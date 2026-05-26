# Fixture 0037 — http-wasm-body-and-advanced-boot-reject

SUBJECT-ONLY boot-reject differential fixture for the
`envoy.filters.http.wasm` HTTP filter at phase 25.2.
Asserts that envoy-go fails-close at config-load when the filter's
`configuration.envoy_go_strict.body_buffer_cap_bytes` field is set to
`0` (PARSE-REJECT arm 19 per 25.2 SPEC §6.2 + Task 14), while
reference Envoy v1.37.2 BOOTS SUCCESSFULLY against the same config
(the unknown `envoy_go_strict` extension key is silently dropped by
upstream's protobuf parser — upstream has no equivalent field).

Per phase 25.2 SPEC §8.2 + D-25.2-P1 closure at 25.2 IMPL Task 21
first-action.

## Fixture type

SUBJECT-ONLY boot-reject (`BootRejectFixture` + `SubjectOnlyBootRejectFixture`
interfaces at `test/differential/harness.go`). The runner's
`runBootRejectFixture` branch with the `SubjectOnly() bool` flag
dispatch:

1. Renders the reference bootstrap via `ReferenceBootstrap()` (including
   the bind-mount for the .wasm blob via `ReferenceLogMounter`).
2. Starts the reference proxy via `tryStartReferenceProxy` (extended
   to accept `hostMounts` at phase 25.2 Task 21); asserts it boots
   SUCCESSFULLY (cancel returned non-nil; err is nil). Tears down the
   reference proxy.
3. Renders the subject config via `SubjectConfig()`; starts the subject
   via `tryStartSubjectProxy`; asserts it boot-REJECTS (cancel is nil;
   err is non-nil) with the substring
   `"envoy_go_strict_body_buffer_cap_bytes"` in its stderr.
4. Skips the cross-side request/response phase entirely (no driving of
   either side; this is a config-load-only fixture).

## Boot-reject trigger (chosen arm: 19)

`envoy.filters.http.wasm` filter config with `configuration` carrying
the envoy-go-strict-only `envoy_go_strict.body_buffer_cap_bytes` field
set to `0`:

```yaml
typed_config:
  "@type": type.googleapis.com/envoy.extensions.filters.http.wasm.v3.Wasm
  config:
    name: plugin_bootreject
    root_id: rootid_bootreject
    vm_config:
      vm_id: vm_bootreject
      runtime: envoy.wasm.runtime.wazero  # subject-side
      code:
        local:
          filename: /bytecode/probe.wasm
    configuration:
      "@type": type.googleapis.com/google.protobuf.Struct
      value:
        envoy_go_strict:
          body_buffer_cap_bytes: 0  # PARSE-REJECT arm 19 trigger
```

This triggers `parseRejectEnvoyGoStrictBodyBufferCapBytesZero` per
`internal/filter/http/wasm/compiled_config.go` line 302:

```
"wasm: config.envoy_go_strict_body_buffer_cap_bytes must be > 0 (envoy-go-strict)"
```

## Stderr comparison

| Side | Expected behavior |
|---|---|
| reference Envoy v1.37.2 | BOOTS SUCCESSFULLY. Upstream's protobuf Struct parser silently drops the unknown `envoy_go_strict` key (there's no upstream extension field by that name). The wasm filter loads `/bytecode/probe.wasm` cleanly + admin /ready returns 200. |
| envoy-go | BOOT-REJECTS at config-load with the byte-stable wording `"wasm: config.envoy_go_strict_body_buffer_cap_bytes must be > 0 (envoy-go-strict)"` (`parseRejectEnvoyGoStrictBodyBufferCapBytesZero` constant per 25.2 SPEC §6.2 arm 19). The substring `"envoy_go_strict_body_buffer_cap_bytes"` appears verbatim in stderr. |

**Subject-side substring:** `"envoy_go_strict_body_buffer_cap_bytes"` —
a 37-character literal fragment unique to arms 19 + 23 (both reference
the same field name). Arm 23's overlarge-cap variant would also match
this substring; here arm 19's `must be > 0` wording is the load-bearing
diagnostic. Per `reference_differential_fixture_dispatch_constraint`
(one fixture dir = ONE runner branch), this fixture pins arm 19 only.

## D-25.2-P1 closure — 6-candidate empirical-scrape disposition

D-25.2-P1 (fixture-0037 single-arm boot-reject finalization) is
finalized empirically at 25.2 IMPL Task 21 first-action by inspecting
the byte-stable wording of each candidate arm at
`internal/filter/http/wasm/compiled_config.go` lines 296-361.

Candidate arms per SPEC §6.4: {19, 20, 21, 22, 23, 26}. Disposition:

| Arm | Const + wording | Substring choice | Config trigger | Verdict |
|---|---|---|---|---|
| **19** | `parseRejectEnvoyGoStrictBodyBufferCapBytesZero` = `"wasm: config.envoy_go_strict_body_buffer_cap_bytes must be > 0 (envoy-go-strict)"` | `envoy_go_strict_body_buffer_cap_bytes` (37 chars) | `envoy_go_strict.body_buffer_cap_bytes: 0` (single-field; deterministic) | **CHOSEN** — most distinctive substring, simplest single-field trigger. |
| 20 | `parseRejectEnvoyGoStrictSharedDataValueCapBytesZero` = `"wasm: config.envoy_go_strict_shared_data_value_cap_bytes must be > 0 (envoy-go-strict)"` | `envoy_go_strict_shared_data_value_cap_bytes` (43 chars) | `envoy_go_strict.shared_data_value_cap_bytes: 0` | viable; chose 19 instead (shorter substring + simpler field rationale doc surface) |
| 21 | `parseRejectEnvoyGoStrictSharedDataMaxEntriesZero` = `"wasm: config.envoy_go_strict_shared_data_max_entries must be > 0 (envoy-go-strict)"` | `envoy_go_strict_shared_data_max_entries` (39 chars) | `envoy_go_strict.shared_data_max_entries: 0` | viable; chose 19 instead |
| 22 | `parseRejectEnvoyGoStrictDynamicStatsMaxEntriesZero` = `"wasm: config.envoy_go_strict_dynamic_stats_max_entries must be > 0 (envoy-go-strict)"` | `envoy_go_strict_dynamic_stats_max_entries` (41 chars) | `envoy_go_strict.dynamic_stats_max_entries: 0` | viable; chose 19 instead |
| 23 | `parseRejectEnvoyGoStrictBodyBufferCapBytesOverlarge` = `"wasm: config.envoy_go_strict_body_buffer_cap_bytes %d exceeds 1 GiB ceiling (envoy-go-strict)"` | `exceeds 1 GiB ceiling` (or shares `envoy_go_strict_body_buffer_cap_bytes` with arm 19) | `envoy_go_strict.body_buffer_cap_bytes: 1073741825` (1 GiB + 1) | viable; chose 19 — `must be > 0` is the simpler invariant + the substring `envoy_go_strict_body_buffer_cap_bytes` collides with arm 23's, but pinning arm 19's `must be > 0` wording wins by simplicity of trigger |
| 26 | `parseRejectCrossPluginConfigDuplicatePluginConfigName` = `"wasm: config.name %q is duplicated across PluginConfig entries (per-plugin stat-scope uniqueness; envoy-go-strict)"` | `duplicated across PluginConfig entries` (38 chars) | TWO `Wasm` filter PluginConfigs with the same `name` (multi-listener trigger) | viable but more complex bootstrap (needs two listeners with conflicting names); chose 19 for single-listener simplicity |

**Chosen arm: 19** `envoy-go-strict-body-buffer-cap-bytes-zero` with
substring `"envoy_go_strict_body_buffer_cap_bytes"`. Anticipated answer
per 25.2 SPEC §6.4 + PLAN Task 21 HELD without deviation; rationale
captured above. Per the asymmetry: arm 19 also has the cleanest
"reference Envoy v1.37.2 has no equivalent field" property — every
candidate arm shares this property (all 6 candidates are envoy-go-strict-
only validators), but arm 19's field name `body_buffer_cap_bytes` is
the most-trivially-validatable single-field trigger (any zero suffices;
no need to overlap with a second envoy-go-strict default).

## Runner-branch shape decision

Per PLAN Task 21 + `reference_differential_fixture_dispatch_constraint`:
**chosen approach** — extend the existing `BootRejectFixture` runner
branch with a sibling-interface opt-in (`SubjectOnlyBootRejectFixture`
at `test/differential/harness.go`). Rationale: minimal infrastructure
delta, preserves backwards compatibility with fixtures 0026/0029/0031/
0033/0035 (none of which implement the new sibling interface, so they
default to the existing symmetric boot-reject discipline unchanged).
The runner detects subject-only via type-assertion at
`runBootRejectFixture`; assertion-flow split is the only behavioral
delta. Per PLAN sub-bullet "recommended: extend BootRejectFixture with
subjectOnly: true flag — minimal infrastructure delta".

The sibling-interface approach (vs adding a method directly to
`BootRejectFixture` — which would force every existing boot-reject
fixture to re-implement it) is the strictly-additive non-breaking
choice. Sibling interfaces follow the established pattern of
`ReferenceLogMounter` / `MultiListenerDriver` / `StatsAsserter` /
`ReferenceLessFixture` at `test/differential/fixture/fixture.go`.

## Bytecode reuse

The reference side needs a VALID `.wasm` blob — without one, upstream
Envoy fails to boot for an unrelated reason (file-not-found / compile-
fail) that would mask the asymmetry assertion. We REUSE the
`a_body_read_only.wasm` blob from sibling fixture-0036 at
`test/fixtures/0036-http-wasm-body-and-advanced/bytecode/a_body_read_only.wasm`
— it's a minimal Rust proxy-wasm plugin that compiles cleanly under
both V8 (upstream) and wazero (subject) per Task 20 acceptance.

For the subject side the .wasm blob path is spliced into the bootstrap
identically, but envoy-go's `buildCompiledConfig` PARSE-REJECTs at arm
19 (cap field validator) BEFORE `resolveDataSource` reads the .wasm
file (see ordering at `internal/filter/http/wasm/compiled_config.go`
lines 844-862 — cap validators fire before DataSource resolution).
So even if the .wasm file were absent on the subject side, arm 19
would still fire — but we splice the same path for symmetry of
diagnostic + to avoid masking a future ordering regression.

## Bootstrap discipline

Self-contained inline bootstrap (Option B2 per fixture-0029 / 0031 /
0033 / 0035 precedent). The arm-19 trigger
(`envoy_go_strict.body_buffer_cap_bytes: 0`) is embedded directly in
the bootstrap rendered by `renderBootRejectBootstrap()` in driver.go.

A minimal `c_unused` upstream cluster (`127.0.0.1:1` — never dialed) is
declared so envoy-go's cluster manager (which runs BEFORE the listener
manager) does not fail with a zero-endpoint error before the filter
PARSE-REJECT fires. Same ordering sidestep as fixtures 0026 / 0029 /
0031 / 0033 / 0035.

The envoy-go subject bootstrap uses `runtime: envoy.wasm.runtime.wazero`
per AMEND-A1; the reference Envoy bootstrap uses
`runtime: envoy.wasm.runtime.v8` per the standard upstream default.

## This fixture vs fixture-0035 vs fixture-0036

Per `reference_differential_fixture_dispatch_constraint` (one fixture
dir = ONE runner branch):

- **0035** — symmetric boot-reject (BootRejectFixture; both sides fail
  to boot on arm-8 `vm_config.code.local: {}` trigger — phase 25.1)
- **0036** — cross-side mixed-mode (CompareBytes on 10 deterministic
  scenarios + StatsAsserter on 4 non-deterministic subject-only
  scenarios — phase 25.2 Task 20)
- **0037** — SUBJECT-ONLY boot-reject (BootRejectFixture +
  SubjectOnlyBootRejectFixture; subject fails to boot on arm-19
  envoy-go-strict-only cap-zero trigger; reference accepts the unknown
  extension field silently — phase 25.2 Task 21)

## Cross-references

- 25.2 SPEC §6.2 row 19 (envoy-go-strict-body-buffer-cap-bytes-zero
  PARSE-REJECT; byte-stable wording)
- 25.2 SPEC §6.4 (D-25.2-P1 — fixture-0037 single-arm boot-reject
  finalization)
- 25.2 SPEC §8.2 (fixture-0037 subject-only boot-reject taxonomy)
- 25.2 PLAN Task 21 (this fixture's authoring discipline + D-25.2-P1
  closure first-action)
- `internal/filter/http/wasm/compiled_config.go` arm 19
  (parseRejectEnvoyGoStrictBodyBufferCapBytesZero)
- `test/differential/harness.go` BootRejectFixture +
  SubjectOnlyBootRejectFixture interfaces
- `test/differential/runner_test.go` runBootRejectFixture branch
  (extended with subject-only dispatch at 25.2 Task 21)
- ADR-0008 (`envoyproxy/envoy:v1.37.2` reference Envoy pin)
- ADR-0208 (NEW internal/filter/http/wasm/ 25.2 package extensions —
  the envoy-go-strict config field family lands here)
- project memory `reference_differential_fixture_dispatch_constraint`
  (one fixture dir = ONE runner branch — bit by 22.3 PLAN, now bit by
  25.2 Task 21 for the subject-only boot-reject branch)
- Phase 25.2 fixture 0036 (sibling cross-side mixed-mode fixture +
  bytecode source)
- Phase 25.1 fixture 0035 (sibling symmetric boot-reject precedent)
