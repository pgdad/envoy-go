# Phase 25.3 — REVIEW (per `superpowers:requesting-code-review`)

> Authoritative inputs: parent SPEC `docs/envoy-go/phases/25-http-filter-wasm/SPEC.md`; 25.3 SPEC `.../25.3-http-filter-wasm-perroute-and-conformance/SPEC.md` (5 AMEND-C wire-shape pins + 7 D-pins + §15 32-item acceptance checklist); 25.3 PLAN `.../PLAN.md` (15-task TDD task graph across 6 tiers A-F); 25.3 PROGRESS `.../PROGRESS.md` (Pre-Task baseline + Task 7/11/12/14 + bug-fix records + the Task-15 atomic-landing entry). Precedent REVIEW shape: `docs/envoy-go/phases/25.2-.../REVIEW.md`.

## §1 — Reviewer orientation

Phase 25.3 is the **THIRD-and-FINAL sub-phase** of phase 25 (`envoy.filters.http.wasm`), the EIGHTEENTH and FINAL §9 HTTP-filters family-row. It lands per-route wholesale override + multi-plugin VM-sharing + `failure_policy = FAIL_RELOAD` reload + `VmConfig.environment_variables` activation + the `test/conformance/proxy-wasm/` conformance-harness seed. **At 25.3 phase-done the §9 HTTP-filters family CLOSES to 0 remaining rows** — the 25.3 IMPL atomic-landing commit names BOTH `row 25.3 in-progress->done` and `row 25 in-progress->done` per the 18/19/22/24 ROLLUP precedent + ADR-0106.

The 25.3 IMPL session executed 15 tasks (Tier A `internal/wasm/` core evolution Tasks 1-4 — registry + raw-vm_id shared-data + reload state machine + env_vars; Tier B phase-21 clock MIGRATION Task 5; Tier C `internal/filter/http/wasm/` extensions Tasks 6-9 — perroute + compiled_config lift + stats 128→132 + dispatch; Tier D fuzzer FOLD + differential fixtures 0038 + 0039 Tasks 10-12; Tier E conformance harness seed + 10 family ports Tasks 13-14; Tier F atomic landing Task 15) plus 4 IMPL-time production bug fixes surfaced by the fixture-0038 differential.

## §2 — Six-gate phase-done verification

All 6 phase-done gates GREEN at the Task 15 atomic landing per ADR-0052.

- **Gate A — build:** `go build ./...` → clean (exit 0).
- **Gate B — vet + lint:** `go vet ./...` + `golangci-lint run` → clean (exit 0).
- **Gate C — race:** `go test -race -short ./...` → green (exit 0; stable across 3 whole-module runs), including the extended `internal/wasm/` (registry.go + reload.go + env_vars.go) + `internal/filter/http/wasm/` (perroute.go + extended compiled_config.go/stats.go/decode_headers.go) + `internal/clock/` superset + `internal/filter/http/adaptive_concurrency/` (re-pointed onto `internal/clock`).
- **Gate D — differential:** **41/41 GREEN** (`go test ./test/differential/...` → ok, 135.859s) — 0000-0037 pre-existing + NEW 0038-http-wasm-perroute-and-multi-plugin (cross-side + subject-only StatsAsserter arms) + 0039-http-wasm-perroute-boot-reject (subject-only env_vars cap-exceeded).
- **Gate E — fuzz:** **35** project-wide fuzzers (`grep -rn "^func Fuzz" --include=*_test.go | wc -l` == 35). NO 36th — `FuzzWasmPerRouteConfig` RETIRED + FOLDED into `FuzzWasmConfigParse`'s corpus per D-25.3-6.
- **Gate F — h2spec + proxy-wasm:** h2spec **53/53 GREEN** (UNCHANGED — the wasm surface is HCM-internal; the HTTP/2 stack is untouched) + NEW proxy-wasm conformance **10/10 GREEN** (`go test ./test/conformance/proxy-wasm/...`).

Counts at phase-done: stat surface **132** (was 128; +4), fixture dirs **41** (was 39; +2), fuzzers **35** (unchanged), conformance **10/10**, ADR tail **0212** (next-free 0213).

## §3 — R8 benchmark gate (Q6 + D-25.3-5)

```
BenchmarkPerStreamContext_Construction_Headers-32    11903540    99.73 ns/op    48 B/op   1 allocs/op
BenchmarkPerStreamModule_Instantiation-32            11582716   101.7  ns/op    48 B/op   1 allocs/op
BenchmarkPerStreamPluginContextLookup-32             11530434   102.6  ns/op    48 B/op   1 allocs/op   (NEW @ 25.3)
BenchmarkPerRouteResolve-32                          1000000000   0.2094 ns/op    0 B/op   0 allocs/op   (NEW @ 25.3)
```

**Disposition: R8 STANDS WEAK-default; ADR-0209 + ADR-0213 escape-valve reserves STAND-UNCONSUMED.** All measurements are WELL under the 1ms (1,000,000 ns/op) per-stream-cost threshold (~9,800× margin on the worst case). The 2 NEW 25.3 supplementary benchmarks per Q6 confirm the multi-plugin VM-sharing (registry-shared `*RootVM` per-stream context creation, ~103 ns/op) + the per-route 3-tier `resolvePerRoute` selection (~0.2 ns/op) add negligible per-request cost. next-free STAYS ADR-0213.

## §4 — The 4 production bugs the differential surfaced + fixed

The headline finding of 25.3 is that the **differential against the real `fail_reload_trap.wasm` guest + the deliberate-break discipline caught 4 production bugs that the unit tests masked**. Each unit test passed because it used a synthetic guest (trapping only in `proxy_on_request_headers`, never poisoning `proxy_on_context_create`) and/or a non-frozen test stats registry; the differential exercised the real frozen-registry + real-guest-trap surface.

- **BUG-1 — frozen-registry per-route stats.** The Task-9 lazy per-route build (`resolveEffective` → `parsePerRouteWasm` → `buildCompiledConfig` → `newFilterStats` → `reg.NewCounter`) ran POST-BOOT, but the stats registry is FROZEN after boot — the subject PANICKED (`registry frozen: cannot register "wasm.plugin_perroute_override.executions" post-boot`) on the first per-route request. FIXED by routing per-route stat registration through `NewCounterIfAbsent` (post-freeze tolerant). The unit `TestDispatch_PerRouteOverrideApplies` passed only because it used a non-frozen test registry; the differential against the frozen production registry caught it. (Fixed at 78d089b.)

- **BUG-3 — guest-trap Close cascade.** A Rust `panic!`/trap inside `proxy_on_request_headers` leaves the proxy-wasm-rust-sdk dispatcher's `RefCell` poisoned (`panic_already_borrowed`). envoy-go caught the request-headers trap but then CONTINUED the stream and dispatched the SAME poisoned instance for `proxy_on_response_headers` + `proxy_on_done`, cascading aborts that destabilized the subject (subsequent requests on the listener degraded). FIXED with a `StreamContext.trapped` guard that abandons further callbacks on a trapped instance. (Fixed at 78d089b.)

- **BUG-4 — ReloadDispatch-before-context-create reorder.** This is the subtle one. As originally ordered in `decode_headers.go` (`resolveEffective` → `initStreamContext`/`proxy_on_context_create` → `ReloadDispatch`), a Failed FAIL_RELOAD VM could NEVER reinstantiate: every post-trap request fail-OPENed at `proxy_on_context_create` (which also trapped on the poisoned instance) BEFORE the reload machine — which owns the only `reinstantiate` primitive — was consulted. The synthetic unit test `TestDispatch_RealTrap_ReloadTripletEngages` could not catch this because its guest trapped only in `proxy_on_request_headers`, not in `proxy_on_context_create`. FIXED by reordering `ReloadDispatch` to run BEFORE `initStreamContext`, so a Failed VM reinstantiates a fresh un-poisoned instance before context-create is attempted; the whole reload-then-context-create dispatch is serialized under `*RootVM.dispatchMu` per D-25.3-P3. (Fixed at c996d3f.)

- **BUG-2 — reload triplet does not engage.** This was the symptom-level read ("the vm_reload triplet does not increment on a real trap") of BUG-3/BUG-4; resolved as a CONSEQUENCE of those two fixes (not a separate fix). Post-fix the triplet engages live, proven via deliberate-break at fixture 0038: req2 within the ~1s backoff window → `vm_reload_backoff=1`; req3 past the window → reinstantiate-recover → `vm_reload_success=1`; `vm_reload_runtime_failure=0` (correct — that counter fires only on a FAILED reload attempt, never reached on the recover path).

Framing: the differential + deliberate-break discipline (per `reference_differential_asserter_dispatch`) is what caught a production code-ORDERING defect (BUG-4) that no fixture-side change (timing, sleep, headers, stat names) could route around — the context-create trap is deterministic regardless of drive timing.

## §5 — Empirical finding: reference v1.37.2 has NO per-route wasm support

Booting fixture-0038's candidate per-route config against the reference Docker image `envoyproxy/envoy:v1.37.2` empirically confirmed AMEND-C1: reference Envoy **boot-rejects** any per-route config for `envoy.filters.http.wasm` (*"The filter envoy.filters.http.wasm doesn't support virtual host or route specific configurations"* — `WasmFilterConfig` overrides only `createFilterFactoryFromProto*`, NOT `createRouteSpecificFilterConfig`). Per-route wasm override is therefore intrinsically an **envoy-go capability surfaced subject-only** in the differential: the reference bootstrap carries no per-route TPFC, and the 0038 `perroute_override_applies` arm asserts the override SUBJECT-ONLY via `wasm.plugin_perroute_override.executions` (proven LIVE via a deliberate-break cycle). This validated ADR-0210's EXPLICIT-NO-NEW-CANONICAL classification (wholesale `Wasm` TPFC override; no `WasmPerRoute`; ADR-0125 STAYS at 10 canonicals). The BRAINSTORM-era "dangling vm_id" boot-reject arm was RETIRED as MOOT (referential VM configs are unimplemented upstream); fixture-0039 shifted to env_vars cap-exceeded (D-25.3-P1).

## §6 — ADR landings

- **ADR-0210** §Decision + §Consequences LANDED (per-route EXPLICIT-NO-NEW-CANONICAL; empirical reference-has-no-per-route-wasm finding; ADR-0125 STAYS at 10; BUG-1 recorded).
- **ADR-0211** §Decision + §Consequences LANDED (multi-plugin VM-sharing registry + FAIL_RELOAD reload state machine + env_vars; BUNDLED per D-25.3-2; records BUG-2/3/4 + the env_vars_cap_exceeded allocate-only note + the 2 NEW departures; 0 NEW ABI surface — ABICallbacks STAYS 25, capability keys STAY 58 per D-25.3-4).
- **ADR-0212** §Decision + §Consequences LANDED (conformance harness seed; in-process `go test` + vendored `.wasm` per D-25.3-P4; 10-of-16 cpp-host family port pin `proxy-wasm-cpp-host@da3ce05d`; 6 deferred families).
- **ADR-0205** §Consequences AMEND + AMENDED 2026-05-29 timestamp (root-VM lifecycle REFINED to per-`(vm_id, vm_configuration, code)` shared `*RootVM` via the registry).
- **ADR-0186** §Consequences RATIFIES clause (i) (the Q5/F1 phase-21 adaptive_concurrency consumer-real migration onto the unified `internal/clock` superset [AfterFunc+Stop] at Task 5 — the EXTRACT-NOW-when-trigger-fires forward-pointer is now consumed at real multi-consumer scope; the reload machine is the third co-consumer).
- ADR tail STAYS at 0212; next-free STAYS ADR-0213. ADR-0209 + ADR-0213 escape-valve reserves STAND-UNCONSUMED.

## §7 — Carried debts + deferred items

RECORDED HERE as carry-forward backlog (NOT load-bearing for 25.3 phase-done; the §9 HTTP-filters family closes regardless):

1. **Q5 debt #2 — phase-22.2 lua filterstate storage migration** STILL deferred past 25.3. The §14.5 non-breaking discipline left the ephemeral `bucketFromMap`/`materializeBucketIntoMap` pattern at `internal/filter/http/lua/filterstate.go`; the deeper storage migration onto `*filterstate.Bucket` is deferred to a consumer-#3-triggered cross-filter migration phase.

2. **Q5 debt #3 — `internal/filterstate/` consumer #3+ API-revision allowance** STILL deferred. Consumer #1 (lua MIGRATES non-breaking) + consumer #2 (wasm) landed at 25.2; the ADR-0207 EXPLICIT API-REVISION ALLOWANCE for consumer #3+ (rbac filter-state read; ext_authz inject; ext_proc pass-through) carries forward.

3. **The 6 deferred proxy-wasm conformance families** — shared_queue, signature_util, wasm (TLS-cache), vm_id_handle, null_vm, fuzz (10-of-16 ported per AMEND-C5). These presuppose the WasmService singleton / cross-VM-queue substrate not implemented at the HTTP-filter scope; documented as forward-pointers in `BOOTSTRAP_PROMPT.md §7.3` + `ENVOY_TARGET.md` + the 25.3 BEHAVIOR_CONTRACT EXTENSION block. Re-evaluated when the broader §9 WASM-host family lands.

4. **`env_vars_cap_exceeded` counter is allocate-only at boot-PARSE-REJECT.** The env_vars cap fires at config-load (`parseEnvVars`) where there is no running per-plugin stats scope to increment a counter; the counter is allocated on the stat surface (so the surface is 132) but is NOT incremented at config-load — consistent with the other 25.2 cap counters being runtime-only. It exists for future runtime use. This is a precise behavioral note, not a bug.

5. **Minor test-isolation note — `TestFilter_RootVM_SharedAcrossStreams_NoCrossStreamLeak`.** The test originally used a fixed plugin name (`plugin_rootvm_concurrent`), which under `-count>1` collides on the process-global arm-26 plugin-config-name claim (the controller observed a `-count=50` flake; the `-count=1` gate is stable). HARDENED at Task 15 with a unique counter-based plugin name (`testVMIDCounter`), with the stat-name lookup derived from it — now stable under `-count=5`. The 4 fixture-0036 cross-side arms a-j (25.2 REVIEW debt #3, Envoy v1.37.2 upstream-buffering 503 parity gap) stay skip-token at 25.3 phase-done per Q4 (deferred to a cross-filter HCM follow-up phase).

## §8 — Green-light evidence summary

**Acceptance to ship:**

- 6 phase-done gates ALL GREEN per §2 (Gates A-F; differential 41/41; h2spec 53/53 + proxy-wasm 10/10).
- R8 benchmark gate STANDS WEAK-default per §3 (all << 1ms; ADR-0209 + ADR-0213 reserves UNCONSUMED).
- 3 NEW ADR §Decision+§Consequences bodies landed (ADR-0210 + ADR-0211 + ADR-0212) + ADR-0205 §Consequences AMEND + ADR-0186 §Consequences RATIFIES per §6.
- BEHAVIOR_CONTRACT.md bundle landed: stat 128 → 132; 2 NEW departure records (#8 reload-floor + #9 env_vars-cap incl. allocate-only note); 25.3 EXTENSION block; 6-deferred-families roster; forward-pointer-notes RESOLVE/RENAME.
- 4 production bugs surfaced by the differential + fixed per §4; empirical reference-no-per-route-wasm finding per §5.
- STATE.md re-advanced to `phase 25.3 IMPL done; §9 HTTP-filters family CLOSED`; next-skill `superpowers:brainstorming` (family closed).
- ROADMAP ROLLUP: sub-row 25.3 + parent row 25 BOTH `in-progress → done` ATOMICALLY; both annotations grep-verifiable in the commit-message body per ADR-0106.
- 15 IMPL tasks landed; PROGRESS.md final Task 15 entry appended; carried debts recorded as backlog per §7.

**Phase 25.3 IMPL ready for squash-merge + push to origin. The §9 HTTP-filters family is CLOSED.**

Next-skill: `superpowers:brainstorming` — the §9 HTTP-filters family is closed; the next un-started §9-line family (Network filters per `BOOTSTRAP_PROMPT.md §9`) is brainstormed as its own phase when it enters `in-progress`.
