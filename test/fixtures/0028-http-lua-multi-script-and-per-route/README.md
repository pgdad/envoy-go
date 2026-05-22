# Fixture 0028 — http lua multi-script and per-route

Phase 22.3 IMPL Task 5 — CROSS-SIDE multi-listener differential fixture
exercising the NEW 22.3 `envoy.filters.http.lua` surface:

- **`source_codes`** — the named-script registry on the listener `Lua` filter.
- **`typed_per_filter_config` `LuaPerRoute`** — the 3-arm per-route override
  oneof: `name` (select a `source_codes` entry), `source_code` (inline/file
  override DataSource), and `disabled` (skip the filter for the route).

It is modeled EXACTLY on
[`0027-http-lua-full-bridge`](../0027-http-lua-full-bridge/): one listener per
scenario via `fixture.MultiListenerDriver`, scripts bind-mounted into the
reference container via `ReferenceHostMounts()`, the shared echobackend that
reflects request headers as JSON, `host.docker.internal` backend reach per
ADR-0010, and byte-exact `CompareBytes` on BOTH proxies.

This fixture does **NOT** implement `BootRejectFixture` — the
`source_codes` compile-error boot-reject is the sibling
[`0029-http-lua-source-codes-boot-reject`](../0029-http-lua-source-codes-boot-reject/).

## Scenarios (6, all CROSS-SIDE byte-exact)

| Scenario | Listener   | Config                                              | Verdict (`x-lua-script`) |
|----------|------------|-----------------------------------------------------|--------------------------|
| a        | `l_test_a` | `default_source_code`; route NO per-route           | `default`                |
| b        | `l_test_b` | `source_codes{named_a,named_b}`+default; name:named_a | `named_a`              |
| b2       | `l_test_b2`| `source_codes`+default; name:ghost (dangling)        | `absent` (silent no-op) |
| c        | `l_test_c` | `default_source_code`; source_code override          | `override`              |
| d        | `l_test_d` | `default_source_code`; disabled:true                 | `absent` (default skipped) |
| e        | `l_test_e` | `source_codes{named_a,named_b}`+default; name:named_b | `named_b`              |

- **(b2) dangling-name** is the AMEND-22.3-1 SILENT NO-OP: a `name` arm that
  references a key NOT in the `source_codes` registry runs nothing (it does
  NOT fall through to the listener default) — upstream-parity pass-through.
- **(d) disabled** skips both hooks; the listener default does NOT run.
- **(b) vs (e)** prove DISTINCT registry entries (`named_a` vs `named_b`) are
  selected from the same `source_codes` map.

## Scripts

All scripts perform deterministic header mutations ONLY (no `:timestamp()`,
`:httpCall()`, or any non-deterministic API) so the reflected JSON — hence the
per-scenario verdict line — is byte-stable cross-side.

| Script         | Mutation                                        |
|----------------|-------------------------------------------------|
| `default.lua`  | `x-lua-script: default`                         |
| `named_a.lua`  | `x-lua-script: named_a`                         |
| `named_b.lua`  | `x-lua-script: named_b`                         |
| `override.lua` | `x-lua-script: override` (per-route source_code)|

## Running

```bash
go test ./test/differential/ -run 'TestDifferential/0028' -count=1 -v
```

Reference Envoy v1.37.2 supports both `source_codes` and `LuaPerRoute`; the
reference and subject bootstraps (`envoy.yaml` / `envoy-go.yaml`) express them
identically.
