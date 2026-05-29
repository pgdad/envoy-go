# Fixture 0039 — http-wasm-perroute-boot-reject

SUBJECT-ONLY boot-reject differential fixture for the
`envoy.filters.http.wasm` HTTP filter at phase 25.3.
Asserts that envoy-go fails-close at config-load when the filter's
`vm_config.environment_variables.key_values` carries MORE than 64
entries (the envoy-go-strict 64-entry cap — PARSE-REJECT arm C per Task
7), while reference Envoy v1.37.2 BOOTS SUCCESSFULLY against the same
config (upstream has NO env_vars entry cap — it accepts arbitrarily many
`environment_variables` entries).

Per phase 25.3 Task 12 + D-25.3-P1 closure at 25.3 IMPL Task 12
first-action.

## Fixture type

SUBJECT-ONLY boot-reject (`BootRejectFixture` +
`SubjectOnlyBootRejectFixture` interfaces at
`test/differential/harness.go`). The runner's `runBootRejectFixture`
branch with the `SubjectOnly() bool` flag dispatch:

1. Renders the reference bootstrap via `ReferenceBootstrap()` (including
   the bind-mount for the .wasm blob via `ReferenceLogMounter`).
2. Starts the reference proxy via `tryStartReferenceProxy`; asserts it
   boots SUCCESSFULLY (cancel returned non-nil; err is nil). Tears down
   the reference proxy.
3. Renders the subject config via `SubjectConfig()`; starts the subject
   via `tryStartSubjectProxy`; asserts it boot-REJECTS (cancel is nil;
   err is non-nil) with the substring
   `"environment_variables exceeds the envoy-go-strict cap"` in stderr.
4. Skips the cross-side request/response phase entirely (this is a
   config-load-only fixture).

## Boot-reject trigger (chosen arm: C — env_vars cap-exceeded)

`envoy.filters.http.wasm` filter config with a
`vm_config.environment_variables.key_values` block carrying 65 entries
(`k0: v0` ... `k64: v64`) — one over the envoy-go-strict 64-entry cap:

```yaml
vm_config:
  vm_id: vm_bootreject_39
  runtime: envoy.wasm.runtime.wazero  # subject-side
  code:
    local:
      filename: /bytecode/probe.wasm
  environment_variables:
    key_values:
      k0: v0
      # ... 65 entries total (64-entry cap + 1) ...
      k64: v64
```

This triggers `parseRejectEnvVarsCapExceeded` per
`internal/filter/http/wasm/compiled_config.go` (the `parseEnvVars` step,
which calls `internalwasm.AssembleEnvVars` — `envVarsMaxEntries = 64`,
`envVarsMaxValueBytes = 4096` per `internal/wasm/env_vars.go`):

```
"wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"
```

## Stderr comparison

| Side | Expected behavior |
|---|---|
| reference Envoy v1.37.2 | BOOTS SUCCESSFULLY. Upstream has NO `environment_variables` entry cap — all 65 entries are accepted. The wasm filter loads `/bytecode/probe.wasm` cleanly + admin /ready returns 200. |
| envoy-go | BOOT-REJECTS at config-load with the byte-stable wording `"wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"` (`parseRejectEnvVarsCapExceeded`). The substring `"environment_variables exceeds the envoy-go-strict cap"` appears verbatim in stderr. |

**Subject-side substring:** `"environment_variables exceeds the
envoy-go-strict cap"` — a literal fragment unique to arm C. Per
`reference_differential_fixture_dispatch_constraint` (one fixture dir =
ONE runner branch), this fixture pins arm C only.

## D-25.3-P1 closure — 2-candidate empirical-scrape disposition

D-25.3-P1 (fixture-0039 single-arm boot-reject finalization) is
finalized empirically at 25.3 IMPL Task 12 first-action by booting BOTH
candidate configs against the reference Docker image
`envoyproxy/envoy:v1.37.2`.

| Arm | Const + wording | Config trigger | Reference v1.37.2 behavior | Verdict |
|---|---|---|---|---|
| **C — cap-exceeded** | `parseRejectEnvVarsCapExceeded` = `"wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"` | `vm_config.environment_variables.key_values` with 65 entries | **BOOTS SUCCESSFULLY** — admin `/ready` returned 200; log reached `"starting main dispatch loop"`. Upstream has NO entry cap; accepts all 65 entries. | **CHOSEN** — clean subject-only asymmetry (mirrors fixture-0037's "reference has no equivalent constraint"); single-listener; substring `environment_variables exceeds the envoy-go-strict cap` is distinctive. |
| A — key collision | `parseRejectEnvVarsKeyCollision` = `"wasm: config.vm_config.environment_variables: key %q is duplicated across host_env_keys and key_values (all keys must be unique)"` | a key in BOTH `host_env_keys` AND `key_values` | **BOOT-REJECTS** — but with a DIFFERENT byte-stable wording: ``error `Key DUPKEY is duplicated in envoy.extensions.wasm.v3.VmConfig.environment_variables for plugin_b. All the keys must be unique.` ``. SYMMETRIC (both reject) but cross-side substrings diverge. | NOT chosen — arm C is the cleaner subject-only fixture matching the 0037 precedent; one-dir-one-branch forbids carrying both. (A future SYMMETRIC fixture in its own dir could pin arm A, asserting only the subject substring or a per-side substring split — out of scope for 0039.) |

**Chosen arm: C** (env_vars cap-exceeded) with substring
`"environment_variables exceeds the envoy-go-strict cap"`. **Chosen
runner-branch: subject-only** (`SubjectOnlyBootRejectFixture.SubjectOnly()
== true`). This was the ANTICIPATED outcome per the task brief: reference
boots (no cap) -> arm A is subject-only -> mirror fixture-0037.

### Empirical scrape evidence (reference v1.37.2)

Arm C (65 key_values entries):
```
ARM A: /ready=200 BOOTED OK
... all clusters initialized. initializing init manager
... all dependencies initialized. starting workers
... starting main dispatch loop
```

Arm A (key collision):
```
[critical][main] [source/server/server.cc:453] error `Key DUPKEY is
duplicated in envoy.extensions.wasm.v3.VmConfig.environment_variables
for plugin_b. All the keys must be unique.` initializing config ...
exiting
```

## Runner-branch shape decision

Per `reference_differential_fixture_dispatch_constraint`: reuses the
existing `BootRejectFixture` + `SubjectOnlyBootRejectFixture`
sibling-interface dispatch at `test/differential/harness.go` (introduced
at 25.2 Task 21 for fixture-0037). No infrastructure delta — this driver
implements both interfaces + returns `true` from `SubjectOnly()`.

## Bytecode

This fixture is SELF-CONTAINED: it vendors a minimal valid `.wasm` blob
at `bytecode/probe.wasm` (copied from sibling fixture-0038's
`listener_default.wasm` — a proxy-wasm plugin that compiles cleanly
under both V8 (upstream) and wazero (subject)). The reference side
bind-mounts it at `/bytecode/probe.wasm`; without a valid blob upstream
Envoy would fail to boot for an unrelated reason (compile-fail) masking
the asymmetry assertion.

The SUBJECT side never reads the `.wasm` blob — envoy-go's
`buildCompiledConfig` parses `vm_config.environment_variables` (the
`parseEnvVars` arm-C cap validator) before `resolveDataSource` opens the
`.wasm` file; the cap-exceeded reject fires first.

## Bootstrap discipline

Self-contained inline bootstrap (Option B2 per fixture-0029 / 0031 /
0033 / 0035 / 0037 precedent). The arm-C trigger (65 `key_values`
entries) is embedded directly in the bootstrap rendered by
`renderBootRejectBootstrap()` in driver.go.

A minimal `c_unused` upstream cluster (`127.0.0.1:1` — never dialed) is
declared so the cluster manager (which runs BEFORE the listener manager)
does not fail with a zero-endpoint error before the filter PARSE-REJECT
fires. Same ordering sidestep as fixtures 0026 / 0029 / 0031 / 0033 /
0035 / 0037.

The envoy-go subject bootstrap uses `runtime: envoy.wasm.runtime.wazero`
per AMEND-A1; the reference Envoy bootstrap uses
`runtime: envoy.wasm.runtime.v8` per the upstream default.

## This fixture vs fixture-0035 vs fixture-0037 vs fixture-0038

Per `reference_differential_fixture_dispatch_constraint` (one fixture
dir = ONE runner branch):

- **0035** — symmetric boot-reject (BootRejectFixture; both sides fail
  to boot — phase 25.1)
- **0037** — subject-only boot-reject (envoy-go-strict body_buffer_cap
  arm; reference accepts the unknown extension field silently — phase
  25.2)
- **0038** — cross-side perroute + multi-plugin (phase 25.3)
- **0039** — SUBJECT-ONLY boot-reject (env_vars 64-entry cap arm C;
  reference accepts all entries with no cap — phase 25.3 Task 12)

## Cross-references

- 25.3 Task 7 (parseEnvVars + arms A/C; `parseRejectEnvVarsCapExceeded`)
- 25.3 Task 12 (this fixture's authoring discipline + D-25.3-P1 closure
  first-action)
- `internal/filter/http/wasm/compiled_config.go` arm C
  (parseRejectEnvVarsCapExceeded + parseEnvVars fire-site)
- `internal/wasm/env_vars.go` (AssembleEnvVars / ErrEnvVarsCapExceeded;
  envVarsMaxEntries = 64, envVarsMaxValueBytes = 4096)
- `test/differential/harness.go` BootRejectFixture +
  SubjectOnlyBootRejectFixture interfaces
- `test/differential/runner_test.go` runBootRejectFixture branch
- ADR-0008 (`envoyproxy/envoy:v1.37.2` reference Envoy pin)
- project memory `reference_differential_fixture_dispatch_constraint`
  (one fixture dir = ONE runner branch)
- Phase 25.3 fixture 0038 (sibling cross-side fixture + bytecode source)
- Phase 25.2 fixture 0037 (subject-only boot-reject precedent + template)
- Phase 25.1 fixture 0035 (symmetric boot-reject precedent)
