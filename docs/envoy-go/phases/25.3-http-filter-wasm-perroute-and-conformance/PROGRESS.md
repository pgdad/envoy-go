# Phase 25.3 — Implementation PROGRESS

> Authoritative input: `docs/envoy-go/phases/25.3-http-filter-wasm-perroute-and-conformance/PLAN.md` (1,043-line PLAN; 15-task TDD task graph across 6 tiers A/B/C/D/E/F). 25.3 SPEC: `.../SPEC.md` (5 AMEND-C wire-shape pins + 7 D-pins + §15 32-item acceptance checklist). 25.3 BRAINSTORM: `.../BRAINSTORM.md` (Q1-Q7). Closest IMPL-execution precedent: `docs/envoy-go/phases/25.2-.../PROGRESS.md` (22-task subagent-driven + two-stage review + 6-gate evidence discipline) + `25.1-.../PROGRESS.md` (17-task).

**Scope.** Phase 25.3 is the **per-route + multi-plugin + reload + env_vars + conformance-harness THIRD-and-FINAL sub-phase** of `envoy.filters.http.wasm` (the NINETEENTH §9 production HTTP filter; parent envelope D 3-way PRE-SPLIT). 15 tasks across 6 tiers: Tier A (Tasks 1-4) `internal/wasm/` core evolution (NEW `registry.go` + raw-vm_id shared-data + NEW `reload.go` + NEW `env_vars.go`); Tier B (Task 5) phase-21 clock MIGRATION onto a unified `internal/clock` superset (F1 reconciliation); Tier C (Tasks 6-9) `internal/filter/http/wasm/` extensions (NEW `perroute.go`; compiled_config lift-5-arms + 3-NEW-arms + registry wiring; stats 128→132; dispatch integration); Tier D (Tasks 10-12) fuzzer FOLD + differential fixtures 0038 + 0039; Tier E (Tasks 13-14) `test/conformance/proxy-wasm/` harness seed + 10 family ports; Tier F (Task 15) atomic landing + ROLLUP close of parent row 25 + §9 family.

**Execution discipline.** `superpowers:subagent-driven-development` — fresh implementer subagent per task + two-stage review (spec compliance, then code quality) between tasks. Each task: failing-test-first → minimal-impl → run-with-expected-output → gates → commit.

**IMPL worktree:** `.worktrees/phase-25.3-http-filter-wasm-perroute-and-conformance-impl`. **IMPL branch:** `phase-25.3-http-filter-wasm-perroute-and-conformance-impl` (branched off master tip `85d39f7`).

---

## Pre-Task — baseline verification (verbatim, from the IMPL worktree root)

### Worktree branch
```
$ git rev-parse --abbrev-ref HEAD
phase-25.3-http-filter-wasm-perroute-and-conformance-impl
```
PASS.

### Master tip
```
$ git log --oneline -3 master
85d39f7 next-prompt.txt: repoint master-tip references to 4afb89a (actual HEAD)
4afb89a next-prompt.txt: rewrite for 25.3 IMPL cold-start (post-25.3-PLAN 1aec3fc/99519f0)
99519f0 phase 25.3 PLAN stage-close: STATE.md SHA-fill (TBD-25.3-PLAN-SQUASH -> 1aec3fc)
```
PASS — branched off `85d39f7` (docs-only repoint past the prompted-anticipated `4afb89a`).

### Toolchain
```
$ go version           → go1.26.2 linux/amd64           (≥ go1.23.0 wazero floor; AMEND-A1)
$ golangci-lint version → v1.64.8                         (ADR-0009 pin)
$ rustc --version       → 1.94.0 (4a4ef493e 2026-03-02)
$ rustup target ...     → wasm32-wasip1 INSTALLED          (offline .wasm build; CI needs no Rust)
$ docker version        → 28.4.0 client / 28.1.1 server    (differential harness only)
```
PASS.

### Baseline gates (build / vet / lint / race-short)
```
$ go build ./...            → exit 0
$ go vet ./...              → exit 0
$ golangci-lint run         → exit 0
$ go test -race -short ./... → exit 0 (all ok / no test files; no failures)
```
PASS — all four code gates GREEN.

### Baseline counts (reconciled vs PLAN)
```
$ grep -rn "^func Fuzz" --include=*_test.go | wc -l       → 35   (PLAN: 35 ✓)
   NOTE: unanchored `grep "func Fuzz"` returns 36 — the 36th is a COMMENT line at
   internal/filter/http/wasm/fuzz_hostcall_test.go:70 (`// grep -rh "^func Fuzz" ...`),
   NOT a fuzz target. The anchored `^func Fuzz` count is the true 35.
$ ls test/fixtures/ | grep -cE '^[0-9]{4}-'               → 37
   NOTE: 0007a-cors + 0007b-iteration-probe have a letter after the 4 digits so they
   are excluded by `^[0-9]{4}-`. Total fixture dirs INCLUDING 0007a/0007b = 39 (the
   "39/39 differential" gate count). 0000-0037 sequential. PLAN baseline 39 ✓.
$ stat total (wasm_test.go single-source-of-truth comments) → 128 (PLAN: 128 ✓)
```
PASS — fuzzers 35, fixtures 39, stat 128 all reconcile with the PLAN baseline.

### ADR tail
```
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | ... | tail -1  → 212
```
PASS — tail ADR-0212 (3 §Context drafts 0210/0211/0212 anchored at SPEC); next-free ADR-0213.

### Differential 39/39 + h2spec 53/53
INHERITED-GREEN by docs-only-master-tip argument (25.3 PLAN + the next-prompt repoints
are docs-only; no code changed since the 25.2 phase-done gate that recorded 39/39 + 53/53).
Will be EXERCISED at Tasks 11/12 (differential → 41/41) + Task 15 (full six-gate run).

**Baseline disposition: GREEN. Cleared to begin Task 1.**

---

## Task 7 / D-25.3-P2 — compiled_config.go EXTEND (lift 5 arms + 3 NEW arms)

### D-25.3-P2 closure: 3 NEW PARSE-REJECT constant wordings (byte-stable, pinned)
The provisional wordings were adopted verbatim (no byte-stability reason to adjust):

- **Arm A** (`parseRejectEnvVarsKeyCollision`):
  `"wasm: config.vm_config.environment_variables: key %q is duplicated across host_env_keys and key_values (all keys must be unique)"`
- **Arm B** (`parseRejectFailOpenAndFailurePolicyBothSet`):
  `"wasm: only one of config.fail_open or config.failure_policy can be set"`
- **Arm C** (`parseRejectEnvVarsCapExceeded`):
  `"wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"`

### Arm-count: 24 → 22 (D-25.3-P2)
`TestParseRejectConstants_ByteStable` roster: **24 → 22**. Arithmetic:
`24 (18 from 25.1 + 6 from 25.2) − 5 LIFTED (arms 9, 10, 12, 13, 18) + 3 NEW (A, B, C) = 22`.
The 5 lifted deferral/reserved constants are RETIRED (deleted):
`parseRejectPluginFailurePolicyFailReloadDeferred` (9),
`parseRejectPluginFailOpenDeferred` (10),
`parseRejectVmConfigVmIdDuplicate` (12, reserved),
`parseRejectVmConfigEnvironmentVariablesDeferred` (13),
`parseRejectPerRouteDeferredTo253` (18) + its byte-equal alias
`parseRejectPerRouteUnsupported` in wasm.go.

### Lifted-arm dispositions
- **Arm 9** (failure_policy=FAIL_RELOAD / reload_config): now PARSED + CONSUMED via
  `parseFailurePolicy` → `wasm.FailurePolicy` + reload base interval (from
  `reload_config.backoff.base_interval`). Stored on `compiledConfig.failurePolicy` +
  `.reloadBaseInterval`; the RootVM consumes the interval via `WithReloadConfig`
  (100ms floor / 1s default applied inside `newReloadBackoff`).
- **Arm 10** (fail_open): now mapped to `FailurePolicyFailOpen`. Deprecated proto access
  kept under `//nolint:staticcheck SA1019`.
- **Arm 12** (duplicate vm_id): INVERTED — a duplicate `(vm_id, vm_configuration, code)`
  now SHARES one `*RootVM` via the registry refcount (`wasm.DefaultRegistry.AcquireFor`).
  No reject path.
- **Arm 13** (environment_variables): now PARSED + CONSUMED via `wasm.AssembleEnvVars`
  → fed to the RootVM via `WithRootEnv`.
- **Arm 18** (per-route): LIFTED — `validatePerRouteWasm` now validates the per-route
  Wasm shape (see design decision below).

### Per-route validation design decision (Task 6 review carry-over)
**Decision: validate-only mode** (NOT build-then-discard). Added
`validateWasmConfigShape(tc)` → `buildCompiledConfigImpl(..., validateOnly=true)`.
The validate-only path runs every PARSE-REJECT arm (1-23 + A/B + compile arms 16/17)
but:
  (i)  SKIPS `registerPluginConfigName` (arm 26) — so a legitimate second use of the same
       per-route override name does NOT arm-26-FALSE-reject (the registry is append-only),
  (ii) does NOT call `AcquireFor` / construct a `*RootVM` — no refcount leak, no goroutine,
       no instantiation.
A build-then-discard validator would have leaked BOTH the registry refcount and the
process-wide name claim. `validatePerRouteWasm` type-asserts `proto.Message` → `*wasmv3.Wasm`,
round-trips through `anypb.New`, and calls `validateWasmConfigShape`. Valid → nil;
invalid → SAME byte-stable buildCompiledConfig wording (single source of truth).
The real per-route compiledConfig + VM build is deferred to Task 9 dispatch.

### vm-key byte-source decision
`AcquireFor(vmID, vmConfig, code, factory)` components:
  - `vmID`     = `vm.GetVmId()` (raw string),
  - `vmConfig` = `vm.GetConfiguration().GetValue()` (serialized VmConfig.configuration bytes;
    SAME bytes stored on `cfg.vmConfig` + fed to `RootVM.Configure`),
  - `code`     = the resolved module source bytes `src` (SAME bytes fed to `CompileModule`
    + re-compiled inside `NewRootVM`).

### Release / teardown lifecycle decision
Added `(*compiledConfig).Close()`: calls `wasm.DefaultRegistry.Release(cfg.vmKey)` (refcount--)
then closes the per-config `compileCache`. The compiledConfig NEVER calls `rootVM.Close()`
directly (that would double-close a shared VM still held by another config); the registry
closes the underlying `*RootVM` only at refcount 0. On a registry HIT the freshly-compiled
orphan cache is closed immediately + the SHARED `rootABICallbacks` multiplexer is retrieved
via the NEW `(*RootVM).RegisteredABICallbacks()` accessor (so configs sharing one VM share
one per-stream routing table). Empty `vmKey` (test-double path) skips Release.
**Production teardown is process/listener-lifetime** (no per-config teardown hook exists in
the New factory at 25.3); `Close()` exists for lifecycle correctness + testability, and the
per-route swap path (Task 9) will call it.

### Step E (cross-package enabler)
Added `(*Registry).AcquireFor(vmID, vmConfig, code, factory) (*RootVM, key, error)` to
`internal/wasm/registry.go` + `TestRegistry_AcquireFor_ReusesByComponentsAndReturnsStableKey`.
Added `(*RootVM).RegisteredABICallbacks()` accessor to `internal/wasm/root_vm.go`.

### Deferred seams left for Task 8/9
- Task 8: the 4 NEW stat counters (`vm_reload_*` triplet + `env_vars_cap_exceeded`) +
  `WithReloadStats` wiring + incrementing `env_vars_cap_exceeded` on the arm-C reject path.
  Task 7 lands the arm-C reject only (no counter).
- Task 9: per-route resolution at DecodeHeaders (`resolvePerRoute`) + reload-on-RuntimeError
  dispatch (reads `cfg.failurePolicy`). Task 7 stores `failurePolicy` / `reloadBaseInterval`
  / `vmKey` / `envVars` on `compiledConfig` for those consumers.

### Gates
`go build ./... && go vet ./... && golangci-lint run && go test -race -short ./...` — ALL GREEN.

---

## Task 11 — differential fixture 0038-http-wasm-perroute-and-multi-plugin

STATUS: **DONE** — all 4 arms GREEN with NO skip guards (BUG-1/2/3 fixed at
78d089b, BUG-4 fixed at c996d3f). The default
`go test ./test/differential/ -run TestDifferential/0038 -v` PASSES against
Docker reference Envoy v1.37.2 vs envoy-go. The reload triplet engages: the
BUG-4 fix (ReloadDispatch reordered BEFORE initStreamContext at c996d3f) lets
a Failed FAIL_RELOAD VM reinstantiate a fresh un-poisoned instance before
proxy_on_context_create, so req2 records `vm_reload_backoff=1` (blocked within
the ~1s window) and req3 records `vm_reload_success=1` (reinstantiate recovers
→ 200). All 4 subject-side StatsAsserter arms (override executions, multi_a +
multi_b executions, reload backoff + success) PROVEN LIVE via deliberate-break
cycles. Skip guards (`FIXTURE_0038_SKIP_OVERRIDE` / `FIXTURE_0038_SKIP_RELOAD`)
REMOVED — all arms run unconditionally. Fixture-dir count 39→40 (0038 present).

The BLOCKED finding below (BUG-4) is RESOLVED at c996d3f and retained for the
record.

Fixture authored: `test/fixtures/0038-http-wasm-perroute-and-multi-plugin/`
(README, envoy.yaml, envoy-go.yaml, expectations.yaml, inputs/driver.go,
scripts/{perroute_override,listener_default,shared_data_combined,fail_reload_trap}
+ vendored bytecode/*.wasm). New `BackendKind=HTTPWasmPerRoute=27` +
runner_test.go blank import + switch-case. NOT COMMITTED (per the task rule:
do not commit a fixture that has not run fully green).

### Arm results

| arm | kind | result |
|---|---|---|
| `perroute_listener_default` | cross-side CompareBytes | **GREEN** — `x-wasm-variant=listener` identical on ref + subject |
| `multiplugin_shared_data` | cross-side CompareBytes + subject StatsAsserter | **GREEN** — `x-shared-count=2` identical on both sides (DISCRIMINATING vm_id-sharing proof: two wasm filters share one vm_id → one shared-data namespace → CAS counter reaches 2; a non-shared namespace would read 1). Subject: `wasm.plugin_multi_a.executions>=1` + `wasm.plugin_multi_b.executions>=1` (both filters in the shared-VM chain dispatched). `wasm.wazero.created=3` (distinct VMs; vm_shared shared by the two multi plugins). |
| `perroute_override_applies` | subject StatsAsserter | **GREEN** (post-78d089b) — subject `/override` returns `x-wasm-variant=override` (override applied; was BUG-1, FIXED). `wasm.plugin_perroute_override.executions>=1` (the per-route override VM dispatched its guest). |
| `reload_fail_reload_recovers` | subject StatsAsserter | **GREEN** (post-c996d3f BUG-4 fix) — req1 traps (503 + arms reload); req2 within the ~1s backoff → `vm_reload_backoff=1`; sleep 1.3s; req3 no-trigger → reinstantiate → `vm_reload_success=1` + 200. `vm_reload_runtime_failure=0` (correct: that counter fires only on a FAILED reload ATTEMPT, never reached). `hcm_reload` rq_xx: two 5xx (req1+req2 traps) + one 2xx (req3 recovers). |

Verified GREEN subset (post-78d089b RE-RUN, reload disabled via the driver's
`FIXTURE_0038_SKIP_RELOAD=1` guard ONLY — override is now GREEN with NO guard):
`--- PASS: TestDifferential/0038-http-wasm-perroute-and-multi-plugin`. Drive
stream (both sides identical): `perroute_override subject-only` /
`perroute_listener x-wasm-variant=listener` / `multiplugin x-shared-count=2` /
`reload subject-only`. Subject `/override x-wasm-variant=override`.

### BUG-4 — FAIL_RELOAD reload machine is unreachable when the guest trap poisons proxy_on_context_create (RESOLVED at c996d3f)

RESOLUTION: fixed at c996d3f by reordering `ReloadDispatch` to run BEFORE
`initStreamContext` in `internal/filter/http/wasm/decode_headers.go`, so a
Failed FAIL_RELOAD VM is reinstantiated (fresh un-poisoned instance) before
`proxy_on_context_create` is attempted. Post-fix differential RE-RUN (NO skip
guards): `wasm.plugin_reload.vm_reload_backoff=1`,
`wasm.plugin_reload.vm_reload_success=1`,
`wasm.plugin_reload.vm_reload_runtime_failure=0`,
`wasm.plugin_reload.executions=2` (req1 trap + req3 recover). Original finding
retained below.


The 78d089b fix resolved BUG-1 (frozen-registry per-route → override arm now
GREEN) + BUG-3 (the *Close* teardown cascade). It did NOT — and the unit
`TestDispatch_RealTrap_ReloadTripletEngages` could not have — caught the
following ORDERING defect, because that test's synthetic `buildTrappingProxyWasm`
traps ONLY in `proxy_on_request_headers`, NOT in `proxy_on_context_create`.

With the REAL `fail_reload_trap.wasm` guest (built from the proxy-wasm-rust-sdk),
a trap in `proxy_on_request_headers` leaves the dispatcher's `RefCell`
poisoned (`panic_already_borrowed`). The poison persists on the shared RootVM
instance, so on the NEXT request `proxy_on_context_create` ALSO traps.

Production `internal/filter/http/wasm/decode_headers.go` orders the dispatch:

```
1. resolveEffective            (line 133)
2. initStreamContext           (line 147)  → RootVM.NewStreamContext → proxy_on_context_create
   └─ on error: fail-OPEN, `return Continue`   (line 157)   ← req2/req3 die HERE
3. ReloadDispatch              (line 169)  → reinstantiate fresh instance + vm_reload triplet
4. CallProxyOnRequestHeaders   (line 195)
```

Drive sequence outcome (subject `/stats/prometheus`, RE-RUN with NO skip guards):

- `wasm.plugin_reload.executions = 1`  — only req1 ever reached the guest
  header dispatch. req2 + req3 trapped at `proxy_on_context_create` in step 2
  and fail-OPENed at line 157, NEVER reaching `ReloadDispatch` (step 3).
- `wasm.plugin_reload.envoy_go_failures = 4`  — req1 header trap + req1
  response-headers trap + req2 + req3 context-create traps.
- `wasm.plugin_reload.vm_reload_backoff = 0`, `vm_reload_success = 0`,
  `vm_reload_runtime_failure = 0`  — the reload machine NEVER advanced past
  Failed because `ReloadDispatch` was never invoked for req2/req3.

ROOT CAUSE: for FAIL_RELOAD to recover, `ReloadDispatch` (which owns
`rv.reinstantiate`, the ONLY primitive that swaps in a fresh un-poisoned
instance) must run BEFORE `initStreamContext`, so a Failed VM is reinstantiated
*before* `proxy_on_context_create` is attempted. As ordered today, a poisoned
instance can never be reinstantiated: every post-trap request fails-OPEN at
context-create before the reload machine is consulted.

FIX (out of this task's scope — requires editing internal/ production code):
reorder so `ReloadDispatch` precedes `initStreamContext` (consult the reload
machine on `eff.rootVM` before constructing the per-stream context), OR have
`initStreamContext`/`NewStreamContext` route a context-create trap on a
FAIL_RELOAD VM into `NoteReloadRuntimeError` + the reload disposition instead
of an unconditional fail-OPEN. NO fixture-side change (timing, sleep, headers,
stat names) can route around a production code-ordering defect — the
context-create trap is deterministic regardless of drive timing.

This is a SEPARATE finding from the original BUG-2 (which read the symptom as
"the triplet does not engage on a real trap"). BUG-2's verification was a unit
test that bypassed the poisoned-context-create ordering; the differential
against the real guest exposes that the ordering — not the reload machine
itself — is the blocker.

### BUG-1 — per-route wholesale Wasm override crashes the subject (frozen stats registry)

Reference Envoy **v1.37.2 does NOT support per-route config for
`envoy.filters.http.wasm`** at all (boot rejects: *"The filter
envoy.filters.http.wasm doesn't support virtual host or route specific
configurations"*). So per-route override is intrinsically an envoy-go-only
capability (NOT cross-side) against this pin — handled by making the
reference bootstrap carry no per-route TPFC and asserting the override
SUBJECT-ONLY via `wasm.plugin_perroute_override.executions`.

BUT the subject (envoy-go) PANICS when the per-route override is resolved at
the first request: the Task-9 lazy per-route build (`resolveEffective` →
`parsePerRouteWasm` → `buildCompiledConfig` → `newFilterStats` →
`reg.NewCounter`) runs POST-BOOT, but the stats registry is FROZEN after boot:

```
panic: stats: registry frozen: cannot register "wasm.plugin_perroute_override.executions" post-boot
  internal/stats.(*Registry).checkFrozenLocked
  internal/filter/http/wasm.newFilterStats
  internal/filter/http/wasm.parsePerRouteWasm   (perroute.go:97)
  internal/filter/http/wasm.(*compiledConfig).resolveEffective   (compiled_config.go:1332)
  internal/filter/http/wasm.(*filter).DecodeHeaders   (decode_headers.go:133)
```

The per-route override path (Task 6/9) needs to either (a) pre-build +
pre-register per-route override configs at HCM-build time / boot (before the
freeze) via the `RegisterPerRouteValidator`/`BuildPerRouteConfig` seam, or
(b) use `NewCounterIfAbsent` + a post-freeze-tolerant stats path for lazily
built per-route configs. The `TestDispatch_PerRouteOverrideApplies` unit test
passes ONLY because it uses a non-frozen test registry; production (frozen)
crashes.

### BUG-2 — FAIL_RELOAD reload triplet does not engage on a real guest trap

The reload listener uses `failure_policy: FAIL_RELOAD` (parsed correctly:
`FailurePolicy_FAIL_RELOAD == 1` in go-control-plane v1.37.0 — the subject log
confirms `failure_policy=1`). The guest (`fail_reload_trap.wasm`) panics
(wasm trap) on request header `x-trigger-trap: 1`. Drive sequence: req1 trap /
req2 trap within ~1s backoff / sleep 1.3s / req3 no-trigger. Expected: the
vm_reload triplet progresses (req2 → `vm_reload_backoff`; req3 →
`vm_reload_success`). OBSERVED: `vm_reload_backoff=0`, `vm_reload_success=0`,
`vm_reload_runtime_failure=0` — the reload state machine never reports
Backoff/Attempt. `wasm.plugin_reload.envoy_go.failures=5` (the traps were
counted as generic failures). The trapping request itself does get a 503
(`FAIL_CLOSED 503 ... failure_policy=1` in the subject log + one 5xx in the
hcm_reload stats), but the NEXT-request reload arming
(`NoteReloadRuntimeError` → Failed → ReloadDispatch→Backoff/Attempt) does not
produce the triplet increments. (NOTE: the original driver also asserted
`vm_reload_runtime_failure>=1`, which is itself INCORRECT — that counter only
fires on a FAILED reload ATTEMPT, not on the trap-arming or the
recover-success path; the assertion was corrected to backoff+success, but
those are still 0.)

### BUG-3 — a guest trap poisons the shared wasm instance (cascade panics)

A Rust `panic!`/trap inside `proxy_on_request_headers` leaves the
proxy-wasm-rust-sdk dispatcher's `RefCell` borrowed (poisoned). envoy-go
catches the request-headers trap but then CONTINUES the stream and dispatches
the SAME (poisoned) instance for `proxy_on_response_headers` + `proxy_on_done`,
which abort with `core::cell::panic_already_borrowed`. This destabilizes the
subject (subsequent requests on the listener are affected; the stats scrape
intermittently sees a degraded process). A guest trap should abandon /
reinstantiate the instance before any further callback on that stream, and the
trap should stop the filter chain (no response-path dispatch into the trapped
instance).

### Other findings (worked around in the fixture, but noteworthy)

- **`get_plugin_configuration` traps in the guest**: envoy-go at 25.3
  RECOGNIZES but does NOT route the `PluginConfiguration` (and
  `VmConfiguration`) buffer types — `proxy_get_buffer_bytes(PluginConfiguration)`
  returns `BadArgument` (body_bridge.go `activatedBufferType`), which the
  proxy-wasm-rust-sdk `get_plugin_configuration()` turns into a guest trap
  (`get_buffer` panics). The fixture's multi-plugin guest was redesigned to
  NOT read plugin configuration (role-free symmetric CAS counter) to avoid
  this; a guest that reads its plugin configuration would trap at on_configure.
- **Duplicate HTTP-filter `name` rejected**: envoy-go rejects two filters
  named `envoy.filters.http.wasm` in one chain (*"duplicate filter name"*);
  reference v1.37.2 tolerates it. The multi-plugin chain uses distinct filter
  ENTRY names (`wasm_multi_a` / `wasm_multi_b`) with the same `@type` — works
  on both sides.
- **Driver stat-name normalization**: the `/stats/prometheus` scrape renders
  wire names (`wasm.plugin_x.executions`) in prometheus form
  (`envoy_wasm_plugin_x_executions`; `.`/`-`→`_`, `envoy_` prefix). The driver
  normalizes accordingly (`promNormalize`).

### Deliberate-break liveness (per reference_differential_asserter_dispatch)

The multi-plugin subject arm (`wasm.plugin_multi_a.executions` /
`wasm.plugin_multi_b.executions`) was PROVEN LIVE during the stat-name-format
debugging: with the WRONG (dotted) lookup name the assertion FAILED
(`executions = 0 (found=false)`); with the corrected `promNormalize`-based
lookup it PASSED (`found=true`, value 1) — a natural deliberate-break/restore
cycle.

POST-78d089b deliberate-break cycles (RE-RUN against the real reference,
reload arm skipped via `FIXTURE_0038_SKIP_RELOAD=1`):

- **`perroute_override_applies` (PROVEN LIVE):** threshold raised from `< 1`
  to `< 999` in `AssertStats` → re-run → `--- FAIL: ... perroute_override_applies:
  wasm.plugin_perroute_override.executions = 1 (found=true); want >= 1` →
  threshold restored to `< 1` → re-run → PASS. The assertion fires on the
  real, live counter (the per-route override VM dispatched its guest).
- **`multiplugin_shared_data` (RE-CONFIRMED LIVE):** `plugin_multi_a.executions`
  threshold raised `< 1` → `< 999` → re-run → `--- FAIL: ... multiplugin_shared_data:
  wasm.plugin_multi_a.executions = 1 (found=true); want >= 1` → restored →
  re-run → PASS. Still live post-fix.
POST-c996d3f (BUG-4 fix) deliberate-break cycles — FINAL, all arms run with
NO skip guards against Docker reference Envoy v1.37.2:

- **`reload_fail_reload_recovers` / `vm_reload_backoff` (PROVEN LIVE):**
  threshold raised `< 1` → `< 999` in `AssertStats` → re-run →
  `--- FAIL: ... reload_fail_reload_recovers: vm_reload_backoff = 1
  (found=true); want >= 1 (req2 blocked within the backoff window)` → restored
  to `< 1` → re-run → PASS. The req2 backoff increment is real + live.
- **`reload_fail_reload_recovers` / `vm_reload_success` (PROVEN LIVE):**
  threshold raised `< 1` → `< 999` → re-run →
  `--- FAIL: ... reload_fail_reload_recovers: vm_reload_success = 1
  (found=true); want >= 1 (req3 past the window reloaded successfully)` →
  restored → re-run → PASS. The req3 reinstantiate-recover increment is real +
  live.
- **`perroute_override_applies` (RE-CONFIRMED LIVE):** `executions` threshold
  raised `< 1` → `< 999` → re-run → `--- FAIL: ... perroute_override_applies:
  wasm.plugin_perroute_override.executions = 1 (found=true); want >= 1` →
  restored → re-run → PASS. Still live with all guards removed.
- **`multiplugin_shared_data` (RE-CONFIRMED LIVE):** both
  `wasm.plugin_multi_a.executions` and `wasm.plugin_multi_b.executions`
  thresholds independently raised `< 1` → `< 999` → re-run → each FAILed
  (`= 1 (found=true); want >= 1`) → restored → re-run → PASS. Both filters of
  the shared-VM chain still dispatch live with all guards removed.

All four subject-side StatsAsserter arms are LIVE. Skip guards removed; the
default differential run exercises all arms.

## Task 12 — differential fixture 0039-http-wasm-perroute-boot-reject

NEW single-arm SUBJECT-ONLY boot-reject differential fixture
`test/fixtures/0039-http-wasm-perroute-boot-reject`. Fixture-dir count
40 → 41.

### D-25.3-P1 closure — empirical reference-boot scrape

First-action: booted BOTH candidate configs against the reference Docker
image `envoyproxy/envoy:v1.37.2` to choose the arm + runner-branch (per
`reference_differential_fixture_dispatch_constraint`: one fixture dir =
ONE runner branch).

| Candidate arm | Config trigger | Reference v1.37.2 observed behavior | Branch implied |
|---|---|---|---|
| **C — env_vars cap-exceeded** (CHOSEN) | `vm_config.environment_variables.key_values` with 65 entries (64-entry cap + 1) | **BOOTS SUCCESSFULLY** — admin `/ready` returned `200`; log reached `all dependencies initialized. starting workers` + `starting main dispatch loop`. Upstream Envoy has NO env_vars entry cap; all 65 entries accepted. | **subject-only** (envoy-go rejects, reference boots) |
| A — env_vars key-collision | a key in BOTH `host_env_keys` AND `key_values` | **BOOT-REJECTS** — `[critical][main] error `Key DUPKEY is duplicated in envoy.extensions.wasm.v3.VmConfig.environment_variables for plugin_b. All the keys must be unique.` initializing config ... exiting`. Reference rejects too, but with a DIFFERENT byte-stable wording than envoy-go's `parseRejectEnvVarsKeyCollision`. | symmetric (cross-side substrings diverge) |

**Chosen arm:** C — env_vars cap-exceeded.
**Chosen substring:** `"environment_variables exceeds the envoy-go-strict cap"`
(verbatim fragment of `parseRejectEnvVarsCapExceeded` =
`"wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)"`).
**Chosen runner-branch:** subject-only
(`SubjectOnlyBootRejectFixture.SubjectOnly() == true`) — mirrors
fixture-0037; reuses the existing harness boot-reject dispatch with NO
infrastructure delta.

This was the ANTICIPATED outcome per the task brief (reference boots
arm C with no cap ⇒ arm C is subject-only ⇒ mirror fixture-0037). Arm A
was NOT chosen: although both sides reject it (symmetric), the one-dir-
one-branch constraint forbids carrying two arms in one dir, and arm C is
the cleaner subject-only fixture matching the 0037 precedent. (A future
symmetric fixture in its own dir could pin arm A.)

### Differential result

`go test ./test/differential/ -run 'TestDifferential/0039' -v` → PASS
(2.12s). Reference container boots (admin `/ready=200`) then torn down;
subject envoy-go boot-rejects with:
`listener manager: listener: "l_test_a": filter_chains[0]: hcm: http_filters[0]: factory: wasm: config.vm_config.environment_variables exceeds the envoy-go-strict cap (max 64 entries, max 4096 bytes per value)`
— contains the asserted substring. No production code modified.

### Self-contained bytecode

`bytecode/probe.wasm` vendored (copied from fixture-0038's
`listener_default.wasm`) so 0039 is self-contained; bind-mounted into the
reference container at `/bytecode/probe.wasm`. The subject never reads it
(arm C fires at `parseEnvVars` before `resolveDataSource`).

## Task 14 Batch A — conformance families + deliberate-break liveness

Ported 5 of the 10 conformance families into the in-process proxy-wasm
harness (`test/conformance/proxy-wasm/`): each = a vendored Rust guest
`.wasm` (`sources/<crate>/` → `bytecode/<crate>.wasm`, built offline with
`cargo build --release --target wasm32-wasip1 --offline`, SDK
proxy-wasm-rust-sdk =0.2.4) + a Go `run` func in `families_test.go`
registered into `conformanceFamilies` via a test-scope `init()` (keeps the
production global an empty literal). All blobs export
`proxy_abi_version_0_2_1` (AMEND-A6).

`go test ./test/conformance/proxy-wasm/... -run TestProxyWasmConformance -v`
→ PASS (logging, stop_iteration{pause,continue}, shared_data, pairs_util,
endianness). build / vet / golangci-lint clean.

### Harness-helper extensions (`conformance.go`)

The Task-13 scaffold anticipated `recordingABICallbacks` would need
extending; Batch A added:

- `logForward io.Writer` + `Log` forwarder — the host bridge routes
  `proxy_log` → `ABICallbacks.Log` (NOT → the RootVM log sink), so the
  recording callback now formats each guest log as `[wasm <level>] <msg>\n`
  (mirroring `RootVM.LogProxy`) and writes it to the captured-log buffer.
  Wired by `newConformanceRootVM` to the same `syncBuffer` as
  `WithRootLogSink`. (`logLevelName` helper added — `logLevelString` is
  package-private to internal/wasm.)
- `reqHeaders` seed + `SeedRequestHeaders` / `GetHeaderMap` returning the
  seed for `HttpRequestHeaders` — feeds a known request-header map to the
  guest through the `proxy_get_header_map_pairs` (pairs wire format) path.
- `writtenRespHeaders` capture in `ReplaceHeaderMapValue` +
  `WrittenResponseHeader` accessor — records the guest's
  `set_http_response_header` writes (which the SDK routes through
  `proxy_replace_header_map_value`).

### Per-family behavior, host-observable assertion, deliberate-break cycle

| family | guest behavior | host-observable assertion | break → FAIL | restore → PASS |
| --- | --- | --- | --- | --- |
| logging | `hostcalls::log` at trace/debug/info/warn/error on request headers | `assertLogContains` for each `[wasm <lvl>] conformance-logging <lvl>-msg` | assert `NONEXISTENT-msg` → FAIL | ✓ |
| stop_iteration | two crates: `Action::Pause` and `Action::Continue` from `proxy_on_request_headers` | `CallProxyOnRequestHeaders` returns `ProxyActionPause` / `ProxyActionContinue` | pause variant expect `Continue` → FAIL | ✓ |
| shared_data | `on_vm_start`: set(k,"v1",cas=0)→get→CAS-match set("v2")→stale-cas set("v3",cas=1) | `GetSharedData(k)` = ("v2", cas=2) (stale write rejected) | expect value "v3" → FAIL | ✓ |
| pairs_util | `on_request_headers`: `get_map` decode → write x-pairs-count + x-pairs-echo | seed 4 req headers → `WrittenResponseHeader` count="4", echo="probe-value-42" | expect echo "wrong-value" → FAIL | ✓ |
| endianness | `on_vm_start`: write 0x01020304u32/0x0102030405060708u64 LE bytes to shared-data | `GetSharedData` raw bytes == LE byte order | expect big-endian u32 → FAIL | ✓ |

All five break→FAIL / restore→PASS cycles were run and confirmed (the
`families_test.go` was restored byte-identical after each — verified via
`diff` against a pre-break backup; final full run green).

No production code (`internal/`) modified. No production bugs surfaced — every
family's correct host-observable behavior was assertable through the existing
exported RootVM / StreamContext / ABICallbacks surface.

### Notes for Batch B (remaining 5 families)

exports, security, runtime, wasm_vm, bytecode_util. The `endianness` /
`shared_data` crates demonstrate the `on_vm_start`-only pattern (no stream
needed; observe via `GetSharedData`). `exports` can assert via
`RootVM.HasGlobalFunc` / `CompileModule` accepting the abi-version export;
`security` (OOB memory → `WasmResultInvalidMemoryAccess`) needs a guest that
passes a bad pointer to a hostcall — the host bridge already returns
`InvalidMemoryAccess` on `readString`/`Memory()` failures (see
`registration.go` proxy_log arm); `runtime` exercises the
vm_start/configure/context-create lifecycle ordering; `wasm_vm` needs two
`NewStreamContext` calls to prove per-stream isolation; `bytecode_util` likely
asserts at the `CompileModule` boundary (abi-version sentinel) rather than
needing a new guest. The harness's `recordingABICallbacks` seed/capture
extensions are reusable for any Batch-B family needing seeded host state.

## Task 14 Batch B — conformance families exports/security/runtime/wasm_vm/bytecode_util + deliberate-break liveness (10/10 green)

Ported the remaining 5 families into the harness, completing the 10-family
conformance gate (R-25.3-6). Four new vendored Rust guests
(`sources/exports`, `sources/security`, `sources/runtime`, `sources/wasm_vm`
→ `bytecode/<crate>.wasm`, built offline `cargo build --release --target
wasm32-wasip1 --offline`, SDK proxy-wasm-rust-sdk =0.2.4, all exporting
`proxy_abi_version_0_2_1`); `bytecode_util` needs NO guest — it asserts at the
`CompileModule` boundary using hand-crafted minimal wasm modules built with an
in-file DSL (mirroring the package-private builders in
`internal/wasm/compile_test.go`). Go `run` funcs live in `families_b_test.go`,
registered via a second test-scope `init()` append.

`go test ./test/conformance/proxy-wasm/... -v` → ALL 10 families PASS
(Batch A: logging, stop_iteration{pause,continue}, shared_data, pairs_util,
endianness; Batch B: exports, security{allowed,denied}, runtime, wasm_vm,
bytecode_util{v0_2_1_compiles,wrong_abi_rejected,missing_abi_rejected}).
build / vet / golangci-lint clean.

### Harness-helper extensions (`conformance.go`)

- `conformanceSandboxDenying(deny ...string)` — returns the permissive
  conformance sandbox with named caps removed; the `security` family passes it
  via `WithRootSandboxConfig` in the `extra` opts (applied AFTER the harness
  default, so it overrides) to drive the same guest under a restricted gate.
- `assertLogNotContains` — negative log assertion (security "time-ok" absent
  under the denied sandbox).
- `ResetWrittenResponseHeaders` on `recordingABICallbacks` — the `wasm_vm`
  family drives several streams against the SAME RootVM/callbacks and reads
  `x-stream-count` after each drive; it resets between drives so each read
  reflects only the latest stream's write.

### Per-family behavior, host-observable assertion, deliberate-break cycle

| family | guest behavior | host-observable assertion | break → FAIL | restore → PASS |
| --- | --- | --- | --- | --- |
| exports | `on_vm_start`: `std::env::var` (environ), `SystemTime::now` (clock_time_get), raw `random_get` → write each to shared-data | `WithRootEnv` seed round-trips byte-faithfully; clock nanos non-zero; random errno=0 + 16-byte non-all-zero buffer | expect env value `+"-BREAK"` → FAIL | ✓ |
| security | `on_request_headers`: `proxy_log("log-ok")` then `get_current_time` then `log("time-ok")`; SDK panics(→trap) on deny sentinel | allowed: `CallProxyOnRequestHeaders` nil + log "log-ok"+"time-ok"; denied (`proxy_get_current_time_nanoseconds` removed): non-nil err (trap) + "log-ok" present, "time-ok" absent | denied: invert `err==nil`→`err!=nil` → FAIL | ✓ |
| runtime | `on_request_headers`: `unreachable!()` (panic=abort → wasm `unreachable` trap); own fresh RootVM | `CallProxyOnRequestHeaders` returns non-nil error (trap surfaces) | invert `err==nil`→`err!=nil` → FAIL | ✓ |
| wasm_vm | `on_request_headers`: per-Filter `calls` counter → write `x-stream-count`; two streams | distinct `ContextID()`s; A→"1","2"; B→"1" (NOT "3" — isolated) | assert B == "3" (leaked) → FAIL | ✓ |
| bytecode_util | hand-crafted modules at `CompileModule` boundary (no guest) | v0.2.1 sentinel compiles; v0.1.0 + missing-sentinel fail wrapping `ErrUnsupportedAbiVersion` | wrong_abi: invert `err==nil`→`err!=nil` → FAIL | ✓ |

All five break→FAIL / restore→PASS cycles were run and confirmed; the file was
restored after each (final full run green, 10/10 families PASS).

### Finding (NOT a production bug)

The proxy-wasm-rust-sdk =0.2.4 `hostcalls::get_current_time()` PANICS
("unexpected status") on any non-`Ok` host result — it has NO graceful `Err`
path for the deny sentinel (`WasmResult::InternalFailure`). So a DENIED 25.1
active hostcall surfaces to the guest as a wasm TRAP, not a recoverable error.
The `security` family therefore asserts the gate via the trap (denied →
`CallProxyOnRequestHeaders` non-nil error, with the always-allowed `proxy_log`
"log-ok" line landing FIRST proving the gate is per-capability). This is SDK
behavior, not an envoy-go defect — the host correctly returned the deny
sentinel (the 25.1 gate-at-call-site discipline), and the host runtime
correctly surfaced the resulting guest trap.

No production code (`internal/`) modified. No production bugs surfaced — every
family's correct host-observable behavior was assertable through the existing
exported RootVM / StreamContext / ABICallbacks / CompileModule surface.

## Task 15 — atomic landing (ADR bodies + BEHAVIOR_CONTRACT bundle + R8 benchmarks + STATE/ROADMAP ROLLUP)

STATUS: **DONE** — the §9 HTTP-filters family CLOSES at this commit (parent row 25
+ sub-row 25.3 BOTH flip `in-progress → done` ATOMICALLY per the 18/19/22/24 ROLLUP
precedent; 0 remaining §9 HTTP-filters rows).

### R8 benchmark disposition (Q6 + D-25.3-5 SAME 1ms threshold)

`go test ./internal/filter/http/wasm/ -run '^$' -bench 'PerStream|PerRoute' -benchmem`
on AMD Ryzen 9 9950X3D:

```
BenchmarkPerStreamContext_Construction_Headers-32    11903540    99.73 ns/op    48 B/op   1 allocs/op
BenchmarkPerStreamModule_Instantiation-32            11582716   101.7  ns/op    48 B/op   1 allocs/op
BenchmarkPerStreamPluginContextLookup-32             11530434   102.6  ns/op    48 B/op   1 allocs/op   (NEW @ 25.3)
BenchmarkPerRouteResolve-32                          1000000000   0.2094 ns/op    0 B/op   0 allocs/op   (NEW @ 25.3)
```

**Disposition: R8 STANDS WEAK-default.** All benchmarks WELL under the 1ms (1,000,000
ns/op) per-stream-cost threshold (~9,800× margin on the worst case). The 2 NEW 25.3
supplementary benchmarks (`BenchmarkPerStreamPluginContextLookup` measuring per-stream
context creation on a registry-shared `*RootVM` per ADR-0211; `BenchmarkPerRouteResolve`
measuring the 3-tier `resolvePerRoute` selection per ADR-0210) confirm the multi-plugin
VM-sharing + per-route surfaces add negligible per-request cost. **ADR-0209 + ADR-0213
escape-valve reserves STAND-UNCONSUMED**; next-free STAYS ADR-0213. (Had any benchmark
exceeded 1ms, the ADR-0209/0213 escape-valve would have fired in this same commit — it
did not.)

### FINAL six-gate phase-done evidence (verbatim, per ADR-0052 atomic-record discipline)

- **Gate A** `go build ./...` → clean (exit 0).
- **Gate B** `go vet ./...` + `golangci-lint run` → clean (exit 0).
- **Gate C** `go test -race -short ./...` → green (exit 0; confirmed stable across 3 whole-module runs).
- **Gate D** differential → **41/41 GREEN** (`go test ./test/differential/...` → ok, 135.859s; includes NEW 0038 + 0039).
- **Gate E** fuzzers → **35** (`grep -rn "^func Fuzz" --include=*_test.go | wc -l` == 35; corpus extended at Task 10; no 36th per D-25.3-6 FOLD).
- **Gate F** h2spec → **53/53 GREEN** (`go test ./test/conformance/h2spec/...` → ok) + NEW `go test ./test/conformance/proxy-wasm/...` → **10/10 GREEN** (all 10 conformance families).
- fixture dirs: **41** (was 39; +0038 +0039); stat surface: **132** (was 128; +4 at Task 8).

Task-15 re-run confirmation (this session, after the doc/benchmark/test edits — only
docs + benchmarks + doc-comment sweep + the dispatch_test.go hardening were touched, no
production behavior changed): Gate A clean; Gate B vet + golangci-lint clean; Gate C
`go test -race -short ./...` green (incl. `internal/filter/http/wasm` ok + the hardened
`TestFilter_RootVM_SharedAcrossStreams_NoCrossStreamLeak` stable under `-count=5`);
conformance `go test ./test/conformance/proxy-wasm/...` → 10/10 ok. Counts re-verified:
fuzzers 35, fixture dirs 41, conformance families 10. Gate D 41/41 + Gate F h2spec 53/53
TRUSTED from the controller's confirmed evidence (no production .go behavior changed at
Task 15 beyond doc-comments).

### ADR landings

- **ADR-0210** §Decision + §Consequences LANDED — per-route = EXPLICIT-NO-NEW-CANONICAL
  (wholesale `Wasm` TPFC; no `WasmPerRoute`; ADR-0125 STAYS at 10 canonicals). Empirical
  finding recorded: reference Envoy v1.37.2 has NO per-route wasm support → per-route is an
  envoy-go capability surfaced subject-only in the differential (fixture 0038).
- **ADR-0211** §Decision + §Consequences LANDED — multi-plugin VM-sharing (process-global
  `Sha256(vm_id‖vm_configuration‖code)`-keyed registry + refcount; raw-vm_id shared-data
  scope) + FAIL_RELOAD reload state machine ({Running,Reloading,Failed} + base_interval-only
  backoff + 100ms floor + RuntimeError-gating + the dispatchMu-serialized
  ReloadDispatch-before-context-create ordering) + env_vars activation (collision-reject +
  64/4KiB cap + WASI environ feed) — BUNDLED per D-25.3-2. Records the 4 differential-surfaced
  bugs (BUG-1..4) + the env_vars_cap_exceeded allocate-only note + the 2 NEW departures.
- **ADR-0212** §Decision + §Consequences LANDED — conformance harness seed (in-process
  `go test` + vendored `.wasm`; NO Docker/Rust-in-CI per D-25.3-P4; 10-of-16 cpp-host family
  port pin `proxy-wasm-cpp-host@da3ce05d`; 6 deferred families).
- **ADR-0205** §Consequences AMEND LANDED + AMENDED 2026-05-29 timestamp in §Status — the
  root-VM lifecycle REFINED to per-`(vm_id, vm_configuration, code)` shared `*RootVM` via the
  registry.
- **ADR-0186** §Consequences RATIFIES clause (i) LANDED — the Q5/F1 phase-21
  adaptive_concurrency consumer-real migration onto the unified `internal/clock` superset
  (AfterFunc+Stop) done at Task 5.
- ADR tail STAYS at 0212; next-free STAYS ADR-0213. ADR-0209/0213 reserves STAND-UNCONSUMED.

### BEHAVIOR_CONTRACT.md bundle

Stat-table 128 → 132 (the 4 NEW counter rows: vm_reload_success / vm_reload_runtime_failure
/ vm_reload_backoff Group-C upstream-parity + env_vars_cap_exceeded envoy-go-strict); the
25.3 EXTENSION block (per-route wholesale-override subject-only; multi-plugin vm_id-shared
VM + raw-vm_id shared-data; FAIL_RELOAD reload machine; env_vars); 2 NEW envoy-go-strict
departure records (#8 reload-floor; #9 env_vars-cap incl. the precise allocate-only-at-boot
note); the 6-deferred-conformance-families roster; RESOLVE/RENAME of the Phase 25.2
forward-pointer notes to 25.3 (all 25.3 hand-off items marked RESOLVED). BOOTSTRAP_PROMPT.md
§7.3 + ENVOY_TARGET.md updated with the 6 deferred families.

### Doc-staleness sweep

`compiled_config.go` `compiledConfig.stats` doc updated "14 counters at 25.2" → "18 counters
at 25.3"; `stats.go` filterStats/newFilterStats headers updated 5/14 → 18 (current-state
claims; historical AMEND-A2 origin annotations preserved) + the false "no per-route stats at
25.1; per-route PARSE-REJECTs" claim corrected to note per-route IS supported at 25.3 via
NewCounterIfAbsent (BUG-1 fix); the "10 wrapper methods" RootStatsRecorder compile-guard
comment clarified (the interface roster STAYS at 10 — the 4 NEW 25.3 wrappers are
filter-package-only, NOT in the interface, so the count is accurate). Hardened
`TestFilter_RootVM_SharedAcrossStreams_NoCrossStreamLeak` against the -count>1 arm-26
collision (unique counter-based plugin name; stat-name lookup derives from it) — now stable
under `-count=5`.

### STATE / ROADMAP ROLLUP

STATE.md re-advanced: active-phase + lifecycle-state → `phase 25.3 IMPL done; §9
HTTP-filters family CLOSED`; next-skill → `superpowers:brainstorming` (family closed; next
§9-line family = Network filters); last-commit → 57c7c4d (SHA-fill at
stage-close); next-free ADR-0213. ROADMAP: sub-row 25.3 `in-progress → done` + parent row 25
`in-progress → done` flipped ATOMICALLY; BOTH lifecycle annotations placed in the
commit-message body for grep-verifiability per ADR-0106 + the 18/19/22/24 ROLLUP precedent.

### REVIEW.md authored

`docs/envoy-go/phases/25.3-.../REVIEW.md` authored (modeled on the 25.2 REVIEW shape):
15-task execution retrospective + the 4 production bugs the differential surfaced + fixed +
the empirical finding (reference v1.37.2 has no per-route wasm) + the carried debts + the 6
deferred conformance families + the env_vars_cap_exceeded allocate-only note + the
test-isolation note.
