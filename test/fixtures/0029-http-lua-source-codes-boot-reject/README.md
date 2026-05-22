# Fixture 0029 — http lua source_codes boot-reject

Phase 22.3 IMPL Task 5 — BOOT-REJECT differential fixture. A `source_codes`
entry carrying a COMPILE-ERROR Lua script must fail-closed at config-load on
BOTH reference Envoy v1.37.2 and envoy-go.

It is modeled EXACTLY on
[`0026-http-lua-headers-bridge`](../0026-http-lua-headers-bridge/)'s
`BootRejectFixture` mechanism. The runner's `runBootRejectFixture` branch
starts BOTH proxies, asserts BOTH fail to boot, and asserts a common substring
appears in BOTH stderr buffers.

## Scenario (g) — source_codes compile-error

The listener `Lua` filter has a VALID no-op `default_source_code` plus a
`source_codes{bad: <compile-error script>}` entry. At config-load BOTH sides
eagerly compile every `source_codes` entry:

- **reference Envoy**: the `FilterConfig` ctor compiles each entry (LuaJIT) →
  parse error → boot reject.
- **envoy-go**: the Task 1 consume path calls `internallua.CompileScript` on
  each entry (gopher-lua) → parse error → boot reject.

This exercises the NEW 22.3 `source_codes` consume path failing closed,
cross-side. It reuses 0026's proven compile-error boot-reject mechanism — the
only structural difference is that the broken script lives in a `source_codes`
entry rather than `default_source_code`.

## Self-contained inline bootstrap (Option B2)

Like 0026, the boot-reject bootstrap is SELF-CONTAINED and embeds the broken
Lua source via the DataSource `inline_string` arm — NOT a host-mounted
`Filename`. `runBootRejectFixture` calls `tryStartReferenceProxy` directly,
which does NOT consult `ReferenceLogMounter`, so a `Filename`-arm bootstrap
would fail with "Invalid path" before the lua filter ever PARSE-REJECTed. The
`envoy.yaml` / `envoy-go.yaml` templates are documentation/symmetry artifacts;
the driver renders the actual bootstrap in `renderBootRejectBootstrap`.
`scripts/bad_compile.lua` is the on-disk symmetry artifact (byte-equivalent to
the inline payload).

## The common stderr substring (verified empirically)

The broken source is:

```lua
function envoy_on_request(handle) end this-is-not-valid-lua-syntax
```

Running both proxies against the source_codes compile-error bootstrap yields:

- reference Envoy (LuaJIT):
  `script load error: [string "function envoy_on_request(handle) end this-is..."]:1: '=' expected near '-'`
- envoy-go (gopher-lua):
  `... lua: source_codes["bad"]: lua compile: lua_filter_chunk line:1(column:43) near '-':   parse error`

The common literal fragment both parsers emit is **`near '-'`** (both point at
the bad `-` token). The upstream `script load error` wrap is NOT shared:
envoy-go's `maybeWrapLuaScriptLoadError` only adds that prefix for the
`default_source_code` arm, NOT for `source_codes`. Hence `near '-'` is the
genuinely-common needle for this case — used as `ExpectedBootErrorSubstring()`.

## Running

```bash
go test ./test/differential/ -run 'TestDifferential/0029' -count=1 -v
```

## NOT built here (covered elsewhere)

- **(f) source_codes key-empty** — subject-only (upstream ACCEPTS an empty
  key, so it cannot be a both-fail `BootRejectFixture`); covered by Task 1 unit
  PARSE-REJECT tests.
- **(h) per-route source_code DataSource-failure** — covered by Task 2 unit
  PARSE-REJECT tests + Task 4 fuzzing.
