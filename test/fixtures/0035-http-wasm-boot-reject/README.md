# Fixture 0035 — http-wasm-boot-reject

BOOT-REJECT differential fixture for the
`envoy.filters.http.wasm` HTTP filter (phase 25.1).
Asserts that BOTH reference Envoy v1.37.2 AND envoy-go fail-close at
config-load when the filter's `vm_config.code.local` field is present
but the `specifier` oneof is unset (§9.2 arm 8 — the cleanest cross-side
substring per D-P6 empirical capture).

## Fixture type

BOOT-REJECT (`BootRejectFixture` interface). The runner's
`runBootRejectFixture` branch:
1. Renders both bootstraps via `ReferenceBootstrap` + `SubjectConfig`.
2. Starts both proxies via `tryStartReferenceProxy` +
   `tryStartSubjectProxy` (NOT the fatal-on-failure variants).
3. Asserts BOTH return non-nil err (boot rejection on both sides).
4. Asserts BOTH captured stderr buffers contain
   `"specifier"` (case-sensitive substring).

## Boot-reject trigger

`envoy.filters.http.wasm` filter config with `vm_config.code.local`
set to an empty `{}` map: the AsyncDataSource.Local oneof IS present
but the inner `specifier` oneof (filename / inline_bytes /
inline_string / environment_variable) is unset, per §9.2 arm 8.

## Stderr comparison (D-P6)

| Side | Stderr excerpt (load-bearing wording) |
|---|---|
| reference Envoy v1.37.2 | `"Proto constraint validation failed (WasmValidationError.Config: embedded message failed validation \| caused by PluginConfigValidationError.VmConfig: embedded message failed validation \| caused by VmConfigValidationError.Code: embedded message failed validation \| caused by AsyncDataSourceValidationError.Local: embedded message failed validation \| caused by field: \"specifier\", reason: is required)"` (PGV `validate.rules.message.required` on `AsyncDataSource.Local.specifier` oneof) |
| envoy-go | `"listener manager: listener: 'l_test_a': filter_chains[0]: hcm: http_filters[0]: factory: wasm: config.vm_config.code.local: specifier oneof required"` (parseRejectDataSourceSpecifierRequired constant per §9.2 row 8) |

**Common substring:** `"specifier"` — a 9-character literal fragment
of the proto oneof name `specifier`. Highly distinctive — does not
collide with any unrelated token in either stderr buffer.

## D-P6 closure

D-P6 (boot-reject common substring selection) is finalized empirically
by running upstream `envoyproxy/envoy:v1.37.2` (ADR-0008) on minimal
configs triggering each candidate arm in {3, 4, 5, 8, 17}:

- Arms 3, 4, 5 (missing PluginConfig / vm_config / vm_config.code)
  all surface upstream as the OPAQUE wrapper string
  `"Unable to create Wasm plugin <plugin_name>"` (extracted at
  `source/extensions/common/wasm/wasm.cc:467`); no field-name detail
  reaches stderr. Common substring vs envoy-go's
  `"wasm: config.vm_config.code is required"` is at best `"Wasm"` /
  `"wasm"` (case-mismatched) or generic `"required"` — neither
  distinctive.
- Arm 8 (`code.local` present + `specifier` oneof unset) trips PGV
  field-level validation BEFORE the wrapper string fires: upstream
  emits the FULL validation-error chain ending in
  `caused by field: "specifier", reason: is required`. envoy-go's
  arm 8 wording carries the same proto oneof name `specifier`
  verbatim. **Selected: arm 8 with common substring `"specifier"`**.
- Arm 17 (compile failure) requires a malformed-but-loadable .wasm
  blob; the wazero compile-error wording vs the upstream V8 compile
  diagnostic share only generic terms like `"compile"` /
  `"WebAssembly"` — distinctive cross-side wording was harder to
  extract than arm 8's PGV chain.

Per project memory `reference_differential_fixture_dispatch_constraint`,
one fixture dir is one runner branch. Arm 8 is the SINGLE-ARM
boot-reject parity covered here; the remaining 17 PARSE-REJECT arms
are exhaustively covered by Task 9's unit tests +
`TestParseRejectConstants_ByteStable` and Task 14's
`FuzzWasmConfigParse` byte-stable parse-error coverage.

## Bootstrap discipline

Self-contained inline bootstrap (Option B2 per fixture-0029 / 0031 /
0033 precedent). The `vm_config.code.local: {}` trigger is embedded
directly in the bootstrap rendered by `renderBootRejectBootstrap()`
(driver.go). No host-mount or file reference is needed — the
boot-reject fires at config-load BEFORE any .wasm bytecode is read.

A minimal `c_unused` cluster (`127.0.0.1:1` — never dialed) is
declared so envoy-go's cluster manager (which runs before the listener
manager) does not fail with a zero-endpoint error before the filter
PARSE-REJECT fires. Same ordering sidestep as fixtures 0026 / 0029 /
0031 / 0033.

The envoy-go subject bootstrap uses `runtime: envoy.wasm.runtime.wazero`
per AMEND-A1; the reference Envoy bootstrap uses
`runtime: envoy.wasm.runtime.v8` per the standard upstream default.
Both runtimes' parse paths trip arm 8 BEFORE any runtime-specific
bytecode loading — the runtime discriminator is parsed but the
specifier-oneof PARSE-REJECT fires inside the AsyncDataSource
validation step (BOTH PGV upstream + envoy-go's compiled_config.go
order this BEFORE the runtime-specific module-load step).

## This fixture vs fixture-0034

Per project memory `reference_differential_fixture_dispatch_constraint`
(one fixture dir = ONE runner branch), the cross-side and boot-reject
surfaces are SEPARATE directories:
- **0034** — cross-side (RequiresReference=true; CompareBytes on
  7 scenarios a..g + StatsAsserter on scenario e)
- **0035** — boot-reject (BootRejectFixture; both sides fail to boot
  on arm-8 `vm_config.code.local: {}` trigger)

## Cross-references

- parent SPEC §9.2 (boot-reject fixture scope)
- 25.1 SPEC §9.2 row 8 (specifier oneof PARSE-REJECT; byte-stable wording)
- 25.1 SPEC §12 D-P6 (boot-reject common substring; empirically settled here)
- 25.1 PLAN Task 16 + D-P6 (boot-reject common stderr substring)
- harness.go `BootRejectFixture` interface
- `runBootRejectFixture` branch (runner_test.go)
- ADR-0008 (`envoyproxy/envoy:v1.37.2` reference Envoy pin)
- Phase 25.1 fixture 0034 (sibling cross-side fixture)
- fixture-0033 (nearest BootRejectFixture precedent; ratelimit)
- fixture-0031 (BootRejectFixture precedent; admission_control)
- fixture-0029 (lua source_codes BootRejectFixture precedent)
