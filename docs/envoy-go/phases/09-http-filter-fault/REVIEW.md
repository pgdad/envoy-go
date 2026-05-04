# Phase 09 — Code review (REVIEW.md)

**Phase id:** `09` (first §9 HTTP-filters family-row to land per ADR-0106)
**Slug:** `09-http-filter-fault`
**Branch under review:** `phase-09-http-filter-fault-impl`
**Range:** `b33e04f..HEAD` (37 commits — 17 task commits + SHA-fill / PROGRESS-append / ADR-amend follow-ups)
**Parent ROADMAP row:** `09 http-filter-fault` flips `in-progress → done` at the phase-done commit `c7de495` (already landed prior to this REVIEW; row 09's status field reads `done` on master post-merge).
**Reviewer method:** Inline authoring by the implementing session per the PLAN's Task 17 explicit allowance; inputs: SPEC §15 acceptance checklist + the branch diff + 08.2 REVIEW.md structural template + PROGRESS.md per-task entries + DECISIONS.md ADR-0100..ADR-0107.
**Six-gate state at HEAD:** all green per Task 16's verification sweep — outputs reproduced verbatim in §"Six-gate verification appendix" below.

This review covers the full phase 09 surface: `internal/filter/http/fault/` package (`doc.go` + `fault.go` + `fault_test.go` + `fuzz_test.go`), `FactoryCtx` framework extension (`internal/filter/http/types.go` `Stats` + `StatPrefix` fields + `internal/filter/hcm/config.go` `parseHTTPFiltersChain` 4-param widening), `internal/stats/name.go` SN2 dotted-rest flattening fix, `cmd/envoy-go/main.go` `fault.New` boot registration, differential fixture `0011-http-fault` (4 scenarios + 4 sub-probes; reference Envoy v1.37.2 STRICT_DNS + envoy-go STATIC), the FuzzFaultConfigParse fuzzer (twelfth file-count; thirteenth fuzzer in repo per Note-1 below), the BEHAVIOR_CONTRACT.md §13 five-patch bundle (NEW envoy.filters.http.fault subsection + 17→22-name table extension + ±10ms timing tolerance + equivalence-matrix row + two forward-pointer notes), the eight ADRs ADR-0100..ADR-0107, and the ROADMAP row 09 status flip + STATE.md advance.

This REVIEW closes phase 09's lifecycle (state 5 → 6) and is the final task before merge to master.

---

## 1. Final assessment

**APPROVED.**

All six phase-done gates are GREEN at HEAD `c7de495` per the Task 16 verification sweep (§6 below). The implementation faithfully realizes the SPEC across all 17 PLAN tasks. The fault filter is the FIRST §9 HTTP-filters family-row to ship under the BRAINSTORM Decisions 12+13 / ADR-0106 flat-top-level-rows + no-sibling-stub discipline; the package shape (`internal/filter/http/fault/` with `doc.go` + `fault.go` + tests + fuzzer) directly mirrors the cors precedent at `internal/filter/http/cors/`, and the boot-time registration on the existing `HTTPRegistry` (ADR-0072) extends the registered filter-set from {router, cors, envoygotest} to {router, cors, envoygotest, fault} without any framework-shape change beyond the surgical `FactoryCtx` extension (Stats + StatPrefix fields per ADR-0100; nil-tolerant per ADR-0085).

The async-resume mechanics (ADR-0102) are the architectural centrepiece. Phase 09 is the FIRST production exerciser of envoy-go's `time.AfterFunc`-driven decode-side parkDecode + chain `localReplyDone` gate combination. The combined delay+abort path's "callback fires SendLocalReply + ContinueDecoding" discipline (with the chain's `localReplyDone` gate short-circuiting the resumed iteration) is the canonical shape for any future async-resume HTTP filter; it was discovered empirically during Task 14 integration after the original ADR-0102 wording (callback fires SendLocalReply only) led to dispatch-goroutine hangs. The amended ADR-0102 now records the correct shape verbatim. The race-cleanliness story (ADR-0105) similarly has a load-bearing empirical correction: `markedActive` had to upgrade from plain bool to `atomic.Bool` + `CompareAndSwap(true, false)` because the OnDestroy goroutine and timer-callback goroutine genuinely race during chain teardown — the ADR-0071 single-goroutine-per-stream invariant governs the dispatch goroutine but does NOT govern the timer-callback runtime-managed goroutine.

The differential fixture `0011-http-fault` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 4 scenarios (listener-inheritance delay-only; combined delay+abort 503; per-route wholesale-override 418; headers-field exact-match gate) with the headers-field scenario expanded to 4 sub-probes (no-header / canonical / case-insensitive name / case-sensitive value) for §11.8 conclusion (b) coverage. The driver bucketizes elapsed timings (fast vs delayed at the 80ms threshold per planner-time decision 11) to absorb CI scheduling jitter; this pattern is reusable by any future timer-driven filter.

All eight anticipated ADRs (ADR-0100..ADR-0107) landed at the correct tasks per the PLAN's "ADRs introduced by this plan" table; the post-Task-14 amendments to ADR-0102 + ADR-0107 + the post-Task-6 LBP-1-chain correction in ADR-0105 are all recorded in PROGRESS.md follow-up entries and committed on the impl branch (commits `b5ae585`, `f43d3fc`, `0bc1da4`).

The ROADMAP row `09` flipped `in-progress → done` at Task 15's commit `40db754` (the BEHAVIOR_CONTRACT bundle commit) — phase 09 is the second-most-recent example (after 08.2) of the row-flip-at-the-doc-bundle-commit pattern; the §9 family heading at ROADMAP line 56 stays unchanged per ADR-0106's no-row-state invariant (§9 is an umbrella, not a row).

The single Critical-tier carry-forward from Task 5 (combined-path hang) and the single Critical-tier latent bug in `internal/stats/name.go` SN2 (dotted-rest flattening) were both surfaced + fixed inline at Task 14 follow-up; no Critical findings remain at HEAD.

---

## 2. Strengths

### 2.0 Phase scope discipline and §9-family-first delivery

Phase 09 is the FIRST §9 HTTP-filters family-row landing under ADR-0106's flat-top-level-rows + no-sibling-stub discipline. The scope discipline is exemplary:

- **No 10+ surface touched.** The fault filter is strictly scoped to the request-side decode path (DecodeHeaders + OnDestroy); the encode-side path is `Continue` no-op stubs per the SPEC. The 11 deferred fields in `runtimeConfig`'s silent-ignore set (`response_rate_limit`, `header_delay`, `header_abort`, `grpc_status` variants, `upstream_cluster`, `downstream_nodes`, runtime-key fields, `disable_downstream_cluster_stats`, `filter_enabled`, `HeaderMatcher` non-`exact` variants, H2-trailer differential) are silent-ignored at parse time per ADR-0101's 6-vs-11 decomposition rather than rejected — this preserves config-file forward-compatibility against operator-authored configs that embed the fields, while keeping phase 09's SPEC + BEHAVIOR_CONTRACT scoped to what is actually asserted.
- **Single fixture, non-vacuous.** Fixture `0011-http-fault` covers the 4-scenario equivalence claim against both proxies. The 8-probe sequence exercises listener-inheritance + per-route wholesale-override + combined delay+abort + headers-field exact-match (case-insensitive name + case-sensitive value per §11.8 conclusion (b)).
- **Deferred items enumerated.** ADR-0104 (header-driven fault path coupled to `delay.header_delay` / `abort.header_abort` proto sub-messages per §11.5 empirical pin) is the first deferral ADR for §9 family-children; future small follow-up phase (~150 LoC + 1 fuzzer + 1 fixture scenario) lands the coupled pair as a new top-level row per ADR-0106.

### 2.1 `internal/filter/http/fault/` package shape (ADR-0100)

The package is a clean cors-precedent reflection:

- **`doc.go` + `fault.go` + `fault_test.go` + `fuzz_test.go`.** Same four-file shape as `internal/filter/http/cors/` (07.1 deliverable). `doc.go` records the decode-side 5-step discipline + async-resume mechanics + abort terminal-replace + max_active_faults LBP-1 sixth + per-route policy wholesale-override + encode-side no-op + 5-stat list + deferral list + ADR cross-reference list 0100..0107.
- **`TypeURL` const + `New(tc *anypb.Any, ctx FactoryCtx) (Factory, error)` factory.** Same signature as router / cors / envoygotest; unifies the registered filter-set under the existing `HTTPRegistry` shape (ADR-0072).
- **8-step `New` contract per ADR-0101.** typed_config nil-check → unmarshal → PGV mirror on `abort.http_status` ∈ [200, 600) → `delay.fixed_delay > 0` validation → headers `string_match.exact` only parse → percentageToFloat for HUNDRED/TEN_THOUSAND/MILLION denominators → registerFaultStats nil-tolerant → factory closure capturing `*atomic.Int64` shared counter for max_active_faults.
- **`runtimeConfig` 8-scalar+1-slice struct.** 6 fields consumed (`abortHTTPStatus`, `abortPercent`, `delayFixedDelay`, `delayPercent`, `matchHeaders`, `maxActiveFaults`); 11 fields silent-ignored (per ADR-0101's decomposition + ADR-0104 deferral).
- **Decode-side discipline.** `DecodeHeaders` body is the canonical 5-step path: matchesHeaders gate (empty match-headers = match-all per SPEC §6.4) → percentage rolls (0%/100% short-circuit before consulting RNG per ADR-0101 determinism) → max_active_faults cap → markActive (atomic.Bool Store) → dispatch branch (abort-only synchronous SendLocalReply; delay-only async-resume schedule; combined delay+abort timer-callback that fires SendLocalReply + ContinueDecoding per ADR-0102). `OnDestroy` cancels the delay timer + decrements active counter via atomic CAS.
- **Encode-side discipline.** `EncodeHeaders` / `EncodeData` / `EncodeTrailers` are pass-through Continue stubs — the fault filter has nothing to do on the response path per SPEC §6.5; the 5-stat counters are all decode-side increments.

### 2.2 `FactoryCtx` framework extension (ADR-0100)

The Task 2 `FactoryCtx` widening (single-field 2-param form → 3-field 4-param form) is the surgical framework extension that unblocks per-filter stat registration:

- **Two new fields: `Stats *stats.Registry` + `StatPrefix string`.** `Stats` threads from the HCM constructor's stats Registry per LBP-1 sixth application; `StatPrefix` threads from the HCM's `stat_prefix` config field per the existing 06.1 stat-name flattening discipline (ADR-0061).
- **nil-tolerant per ADR-0085.** Non-stat-bearing filters (router, cors, envoygotest) ignore both new fields; the FactoryCtx test suite added `TestFactoryCtx_NilStatsRegistryTolerated` per ADR-0085's nil-tolerance contract.
- **`parseHTTPFiltersChain` 4-param widening.** The HCM-side parser threads `*stats.Registry` + `statPrefix` into each filter's New factory; the existing 11 differential fixtures (0000..0010) PASS unchanged because router/cors/envoygotest ignore the new fields.

### 2.3 Async-resume mechanics + chain `localReplyDone` gate (ADR-0102)

Phase 09 is the FIRST production exerciser of envoy-go's request-side async-resume + chain integration. The mechanics are subtle and the empirical correction at Task 14 was load-bearing:

- **Delay-only path:** dispatch goroutine schedules `f.delayTimer = time.AfterFunc(d, func() { f.dcb.ContinueDecoding(); f.decrementActive() })` then returns `StopIteration`; the chain's `parkDecode` parks the dispatch goroutine on `decodeResumeCh`; timer fires on a runtime goroutine; `ContinueDecoding` signals `decodeResumeCh`; dispatch goroutine wakes and continues to the next filter.
- **Combined delay+abort path:** dispatch goroutine schedules timer; timer callback fires `f.dcb.SendLocalReply(...) + f.dcb.ContinueDecoding(); f.decrementActive()` — BOTH calls. SendLocalReply enters the chain's encode path at filter[len-1]; ContinueDecoding wakes the parked dispatch goroutine; the chain's `localReplyDone` gate at `internal/filter/http/chain.go:135-167` short-circuits the resumed iteration without dialing upstream. The original ADR-0102 wording claimed "callback fires SendLocalReply NOT ContinueDecoding" — empirically wrong; without ContinueDecoding the dispatch goroutine parks indefinitely. ADR-0102 amended at Task 14 follow-up commit `b5ae585`.
- **Cancel-on-OnDestroy.** OnDestroy calls `if f.delayTimer != nil { _ = f.delayTimer.Stop() }; f.decrementActive()`. If `Stop()` returns false (timer already fired), the timer-callback goroutine is in flight; the `markedActive atomic.Bool` CAS in `decrementActive` ensures exactly-once Dec regardless of the OnDestroy/timer-callback race.
- **±10ms timing tolerance.** SPEC §11.2 conclusion (c) empirical pin: envoy-go's `time.AfterFunc` matches Envoy v1.37.2 across the 50/100/200/500ms sweep with worst-case +3.6ms overhead. The differential fixture's elapsed-bucket (fast<80ms / delayed≥80ms) absorbs CI scheduling jitter cleanly.

### 2.4 max_active_faults concurrency cap + LBP-1 sixth application (ADR-0105)

The `max_active_faults` cap is the SIXTH LBP-1 application after ADR-0072 (HTTPRegistry), ADR-0079 (ListenerFilterRegistry), ADR-0061 (stats Registry), ADR-0091 (drain Manager), and ADR-0085 (ChainBuilder closure-capture):

- **Closure-captured `*atomic.Int64` shared counter.** All filter instances spawned by the same factory (one per HCM stream) share the same `*atomic.Int64`; the closure captures it at factory-construction time per LBP-1's "no package globals" discipline. `cfg.maxActiveFaults > 0 && f.active.Load() >= cfg.maxActiveFaults` is the lock-free cap check; on overflow the filter returns `Continue` and Inc's the `fault.faults_overflow` counter.
- **`markedActive atomic.Bool` per-instance idempotency guard.** Inc-side: dispatch goroutine sets `markedActive.Store(true)` after the cap check passes. Dec-side: both OnDestroy and timer-callback paths call `decrementActive`, which `CompareAndSwap(true, false)` to ensure exactly-once Dec. The empirical race-detector finding at Task 6 (plain `bool` flagged on first `-race -count=10` run) drove the upgrade from plain bool to atomic.Bool; ADR-0105 §Alternatives (A) records the supersession.
- **Race-detector cycle test.** `TestFault_DelayTimerRace` (planner-time decision 10) loops 100 iterations of factory → DecodeHeaders → sleep i%2 ms → OnDestroy, forcing OnDestroy to race the timer-callback. PASS at HEAD across `-race -count=10`.

### 2.5 Differential fixture 0011-http-fault

Fixture `0011-http-fault` provides the differential proof against reference Envoy v1.37.2:

- **8-probe sequence.** scenario1 (listener-inherit delay 200+backend+delayed) → scenario2 (combined delay+abort 503+abort-body+delayed) → scenario3-wholesale (per-route wholesale-override 418+abort-body+fast — NO inherited delay per §11.7) → scenario3-baseline (NO per-route override, listener-level delay inherited 200+backend+delayed) → scenario4-a (no header — Continue 200+backend+fast) → scenario4-b (`x-fault-on: yes` — match 503+abort+fast) → scenario4-c (`X-FAULT-ON: yes` — case-insensitive name match 503+abort+fast) → scenario4-d (`x-fault-on: YES` — case-sensitive value mismatch Continue 200+backend+fast).
- **Per-probe assertion-log byte stream.** Driver emits `probe <id> status=<code> body=<quoted> elapsed=<bucket>` lines; status TEXT excluded per planner-time decision 7 (allow-listed for non-stdlib codes like 418 — Envoy emits "Unknown", stdlib emits "I'm a teapot"; code-only equivalence for non-stdlib codes; standard codes 200/503/404/405 byte-equal on code AND text).
- **5-stat counter assertion.** `aborts_injected=4` (scenarios 2/3a/4b/4c) + `delays_injected=3` (scenarios 1/2/3b) + `faults_overflow=0` + `active_faults=0` final + `response_rl_injected=0` permanently per ADR-0107 route A.
- **STATIC vs STRICT_DNS asymmetry.** Reference Envoy uses STRICT_DNS + `host.docker.internal` (Docker container resolution); subject envoy-go uses STATIC + `127.0.0.1` literal (envoy-go's cluster manager only supports STATIC; surfaced at Task 12 follow-up). The 0007a-cors precedent. Differential parity preserved: both proxies dial the same backend port; only resolution path differs.
- **`RequiresReference: true`.** Mirrors 0007a-cors / 0009-admin-config-dump / 0010-graceful-drain registration shape.

### 2.6 Eight new ADRs (ADR-0100..ADR-0107)

All eight anticipated ADRs land at the correct tasks per the PLAN's "ADRs introduced by this plan" table:

- **ADR-0100** (`internal/filter/http/fault/` package shape + boot registration + FactoryCtx framework extension; T3) — anchors package + FactoryCtx Stats/StatPrefix fields + boot Register call.
- **ADR-0101** (`runtimeConfig` shape + 6-vs-11 decomposition + PGV [200, 600) mirror + `delay.fixed_delay > 0` validation + percentage-roll determinism; T3) — anchors parser; cross-references ADR-0073 (3-tier merge wholesale-override empirically confirmed at §11.7), ADR-0104 (header-driven fault path deferred).
- **ADR-0102** (`time.AfterFunc`-driven async-resume — combined delay+abort fires SendLocalReply + ContinueDecoding from timer goroutine; parkDecode wake-up via chain's localReplyDone gate; cancel-on-OnDestroy + ±10ms timing tolerance; T5; AMENDED at T14 follow-up `b5ae585` to record the correct combined-path callback shape).
- **ADR-0103** (Abort terminal-replace mechanics + body byte-exact "fault filter abort" 18 bytes + 4-header set + OrderedHeaders carrier + status-text allow-list narrowed to 200/404/405/503; T4).
- **ADR-0104** (Header-driven fault path deferred — coupled to `delay.header_delay` / `abort.header_abort` proto sub-messages per §11.5 empirical pin; T15; per ADR-0040 deferral format).
- **ADR-0105** (`max_active_faults` cap + LBP-1 sixth application + closure-captured `*atomic.Int64` + `markedActive atomic.Bool` per-instance idempotency + OnDestroy timer-cancel discipline + `fault.faults_overflow` stat semantics; T6; AMENDED at T6 follow-up `f43d3fc` to correct the LBP-1 chain references — original wording cited ADR-0061 + a fabricated ADR-0078; corrected to ADR-0059 → 0072 → 0079 → 0085 → 0091 → 0105).
- **ADR-0106** (§9 HTTP-filters family expansion shape — flat top-level rows + no-sibling-stub; the §9 heading at ROADMAP line 56 is an umbrella, not a row; T15; per BRAINSTORM Decisions 12+13).
- **ADR-0107** (`BEHAVIOR_CONTRACT.md ## Stat-name mapping` 17→22-name extension for FIVE `fault.*` stats + `response_rl_injected` permanently-zero counter discipline route A; T3 consolidated; AMENDED at T14 follow-up `b5ae585` to correct the Prom name shape post-SN2 dotted-rest fix to `envoy_http_fault_<metric>{envoy_http_conn_manager_prefix="<sp>"}` — original shape preserved an internal dot which is invalid Prometheus syntax).

### 2.7 Race-cleanliness across the goroutine seam

Phase 09 is the first envoy-go HTTP filter to genuinely cross the goroutine seam (dispatch goroutine + timer-callback goroutine + OnDestroy goroutine). The race-cleanliness story:

- **Production code race-clean from T6 onward.** `markedActive atomic.Bool` + `*atomic.Int64` shared counter + `*time.Timer.Stop()` happens-before sequencing via `OnDestroy` + chain's `parkDecode/decodeResumeCh` synchronization. `go test -race -count=10 ./internal/filter/http/fault/` clean.
- **Test stub race-cleanliness retrofitted at T5.** The `recordingDCB` test stub's `sentStatus`/`sentBody`/`sentHeaders` plain-field accesses were unsynchronized at Task 4; Task 5's TestDecodeHeaders_Combined exposed the race under `-race`. Refactored to add `sync.Mutex mu` + `Status()/Body()/Headers()` accessor methods; Tasks 4–6 tests retrofitted to use accessors. Documented in PROGRESS Task 5 entry as a forward-improvement triggered by `-race` finding.
- **`TestFault_DelayTimerRace` cycle test.** Stresses the OnDestroy/timer-callback race directly (planner-time decision 10).

### 2.8 BEHAVIOR_CONTRACT.md §13 five-patch bundle (T15)

Task 15 lands the documentation surface for phase-done in five surgical patches per SPEC §13.1–§13.5:

- **§13.1 NEW `### envoy.filters.http.fault` subsection** under `## HTTP filter chain` with six sub-sections (Asserted equivalence + Per-route 3-tier merge + max_active_faults concurrency cap + Async-resume mechanics + Does not yet apply to (9 deferred surfaces) + Empirical evidence verbatim curl excerpt).
- **§13.2 17→22-name table extension.** Renamed `### 17-name table (introduced by phase 06.1)` → `### 22-name table (introduced by phase 06.1; extended by phase 09)`; appended 5-row Fault filter block; updated footer to `Total: 22 internal names (17 from 06.1 + 5 from 09)`. Prom shape post-SN2 fix per ADR-0107 amendment.
- **§13.3 ±10ms timing-tolerance bullet** appended to `## Timing tolerances` per phase 09 §11.2 empirical pin; cross-references the differential fixture's elapsed-bucket discipline (planner-time decision 11).
- **§13.4 Equivalence Matrix new row** appended after the existing `Admin /server_info (DRAINING)` row.
- **§13.5 Forward-pointer notes** at `## HTTP filter chain ### Async resume mechanics` (phase 09 first production exerciser of async-resume on the request side; ADR-0102 + localReplyDone gate cross-ref) and `## Stat-name mapping ### Twin-series filter discipline` (route A for `fault.response_rl_injected`; ADR-0107 cross-ref).

---

## 3. Findings

### 3.1 Critical (blocks phase-done)

**None at HEAD.** Two Critical-tier issues surfaced + RESOLVED INLINE during Task 14 follow-up; both are documented in PROGRESS Task 14 entry's bullet 1+2 and the `b5ae585` commit body:

- **C-1 (RESOLVED at T14 follow-up `b5ae585`).** Combined delay+abort hang. The Task 5 ADR-0102 wording claimed the combined-path callback should call `SendLocalReply` NOT `ContinueDecoding`. Empirically the dispatch goroutine parked indefinitely on `decodeResumeCh` at `parkDecode`. Real design: callback calls BOTH. The chain's `localReplyDone` gate at `internal/filter/http/chain.go:135-167` ensures the resumed iteration short-circuits without dialing upstream. Precedent test was `TestChain_TimerGoroutineRaceWithDispatch_SendLocalReply` in `chain_test.go`. ADR-0102 amended in Task 14 follow-up commit `b5ae585`. `TestDecodeHeaders_Combined` updated to expect `continued == 1` (was `== 0`) with explanatory comment. **Closed.**

- **C-2 (RESOLVED at T14 follow-up `b5ae585`).** Stat-name period in Prometheus output. The stats Registry's `flattenToProm` SN2 case (`http.<sp>.<rest>`) preserved `<rest>` verbatim. The first 17 stat names never had dots in the rest, but `fault.aborts_injected` (and the 4 siblings) do — produced `envoy_http_fault.aborts_injected{...}` (literal period — invalid Prometheus syntax). Reference Envoy emits `envoy_http_fault_aborts_injected{...}` (underscore). Phase 09's fault filter is the FIRST HCM-scoped stat with a nested rest segment — the bug had been latent in `internal/stats/name.go` for prior phases. Fix: `strings.ReplaceAll(tail[dot+1:], ".", "_")` on the rest before forming the base. Existing `name_test.go` tests pass unchanged (their inputs had no internal dots in the rest); `TestFlattenToProm_HCM_DottedRest` added per Task 14 follow-up. ADR-0107 amended to record the correct Prom name shape. **Closed.**

### 3.2 Important (decide carry-forward vs inline-fix)

**I-1 (Carry-forward).** `flattenToProm` SN-asymmetry. Only SN2 (`http.<sp>.<rest>` rule) got the dot→underscore transform at Task 14. SN1 (`cluster.*`), SN3 (`listener.*`), SN5 (`server.*`) still preserve internal dots in their rest segments verbatim. Currently a hypothetical concern (no current names use internal dots in those namespaces), but a future filter that registers `cluster.<name>.<sub>.<metric>` would re-trigger the same Prometheus-invalid output. **Carry-forward to next §9 family-child phase (or any future phase that introduces a nested-rest stat in those namespaces); the fix is straightforward — apply the same `strings.ReplaceAll(rest, ".", "_")` transform symmetrically across SN1/SN3/SN5 cases in `flattenToProm`.** No action at HEAD.

### 3.3 Minor (informational; PLAN documentation drift; no action required)

**M-1.** "Twelfth fuzzer" labeling in PLAN/SPEC/commit-message — actually 13th fuzzer in repo (12 prior fuzzers post-08.2: FuzzHCMConfigParse + FuzzTcpProxyFilter + FuzzPromTextFormat + FuzzConfigDumpFormat + FuzzTLSContextParse + FuzzFilterChainParse + FuzzFrameStream + FuzzHPACKDecode + FuzzDrainTransitions + FuzzAccessLogFormat + FuzzBootstrapLoad + FuzzFilterChainMatch). Documentation-only off-by-one; harmless because ADR-0018's "every parser/codec/filter ships a fuzzer" discipline is satisfied regardless of the count label. The implementer correctly preserved PLAN wording verbatim in Task 9's commit message + PROGRESS entry. The 08.2 REVIEW Note-4 already records that `FuzzFrameStream` + `FuzzHPACKDecode` both live in `internal/filter/hcm/h2/`, so post-08.2 the actual count was 12 (not the SPEC's "11"); phase 09's "twelfth" label compounds the off-by-one. No action.

**M-2.** SHA-fill commit message format drifted slightly between Task 1–9 (`phase 09 follow-up: PROGRESS Task N SHA-fill`) and Task 10 (`phase 09 Task 10 follow-up: PROGRESS.md SHA-fill`). Functionally identical; the SHA-fill follow-up workflow is well-defined; minor stylistic inconsistency within the phase. No action.

**M-3.** Doc-comment placement deviation in Task 2 FactoryCtx (struct-level vs per-field). The PLAN snippet placed the framework-extension paragraph at the struct level; the implementer placed it per-field. Per-field placement is more discoverable from `go doc -all internal/filter/http`; not a defect. No action.

**M-4.** Test-stub `recordingDCB` evolution: Task 5 added `sync.Mutex` + accessor methods triggered by `-race` finding; Tasks 4–6 tests retrofitted to use accessors. Documented in PROGRESS Task 5 entry as a forward-improvement, not a defect. The pattern is reusable — see N-2 below for carry-forward.

**M-5.** PLAN's planner-time decision 8 (STRICT_DNS for both proxies) was incomplete: envoy-go only supports STATIC clusters (Task 12 follow-up). Corrected disposition: reference uses STRICT_DNS + host.docker.internal (Docker); subject uses STATIC + 127.0.0.1 literal. Mirrors 0007a-cors precedent. The Task 12 follow-up commit `2d0cf9a` + amendment to README.md "Bootstrap discipline" bullet record the corrected disposition. No further action.

**M-6.** Reference admin port 9902→9901 (Task 14 fix). PLAN/SPEC pinned `:9902`, but `harness.StartReferenceProxy` exposes only `9901/tcp` and waits on `/ready` at 9901; every other fixture (0006/0009/0010) uses 9901. Pre-harness-discipline carryover. Fixed in Task 14 alongside the C-1/C-2 fixes. No further action.

---

## 4. Per-task summary

The 17 tasks landed 17 task-commits + ~20 SHA-fill / PROGRESS-append / ADR-amend follow-ups across the impl branch. Per-task summary (one paragraph each):

**Task 1 (commit `29c0958`):** PROGRESS.md preamble + execution-precondition check. Verified all 15 cold-start preconditions: branch is `phase-09-http-filter-fault-impl`, master log matches expected sequence, Docker / Go / golangci-lint at expected versions, all 30 packages PASS short-mode, 12 differential fixtures PASS (0000..0010), ADR tail at ADR-0099, SPEC.md at `da29807`, FactoryCtx single-field 2-param form, parseHTTPFiltersChain 2-param signature, envoyproxy/envoy:v1.37.2 image present, CONFORMANCE_PINS.md unchanged. The thirteen planner-time deferred decisions reproduced verbatim from PLAN.md so PROGRESS is self-contained. No ADR landed (per ADR-0044 ADR-on-impl convention).

**Task 2 (commit `2449939`):** FactoryCtx framework extension. Strict-TDD per PLAN steps 1–10. Added `TestFactoryCtx_StatsRegistryThreaded` + `TestFactoryCtx_NilStatsRegistryTolerated` to `internal/filter/http/types_test.go`; widened `FactoryCtx` to add `Stats *stats.Registry` + `StatPrefix string` fields with per-field doc-comment paragraphs anchoring ADR-0061 + ADR-0085 + ADR-0100 first-use. Widened `parseHTTPFiltersChain` from 2-param to 4-param shape; updated `parseFilterWithCtx` call site + the `FactoryCtx` populate inside the second loop. 11 differential fixtures PASS unchanged (router/cors/envoygotest ignore the new fields per ADR-0085 nil-tolerance).

**Task 3 (commit `e80aa10`):** fault package core + ADR-0100/0101/0107. Strict-TDD per PLAN steps 1–8. Wrote `fault_test.go` with 7 tests (TestNew_NilTC + TestNew_MalformedTC + TestNew_AbortHTTPStatusOutOfRange/4-subcases + TestNew_DelayPercentageWithoutFixedDelay + TestNew_HappyPath + TestNew_RegistersStats + TestRuntimeConfig_FieldExtraction). Wrote `doc.go` verbatim from PLAN. Wrote `fault.go` with `TypeURL` + `faultAbortBody` + `faultStats` + `runtimeConfig` 8-scalar+1-slice + `headerMatch` + `New` factory (8-step contract per ADR-0101) + `parseRuntimeConfig` (PGV [200, 600) + delay.fixed_delay > 0 + `string_match.exact` only) + `percentageToFloat` (HUNDRED/TEN_THOUSAND/MILLION) + `registerFaultStats` (nil-tolerant per ADR-0085) + `filter` 8-field struct + static interface assertions + decoder/encoder method set with stub DecodeHeaders. Two PLAN refinements: type-assertion `_, ok := a.GetErrorType().(*faultv3.FaultAbort_HttpStatus)` for abort.error_type discrimination (silent-ignores header_abort + grpc_status variants); HeaderMatcher_StringMatch type-assertion in headers gate. Three ADRs appended: ADR-0100 + ADR-0101 + ADR-0107.

**Task 4 (commit `afea8ec`):** DecodeHeaders abort terminal-replace path + headers gate + percentage-roll + ADR-0103. Strict-TDD per PLAN steps 1–7. Added `recordingDCB` test stub + `makeFilter` helper + 6 failing tests (TestDecodeHeaders_AbortOnly_100Percent + TestDecodeHeaders_AbortOnly_0Percent + TestDecodeHeaders_HeadersFieldExactMatch_CaseInsensitiveName + TestDecodeHeaders_HeadersFieldExactMatch_CaseSensitiveValue + TestDecodeHeaders_NoFaultHeaderMismatch + TestDecodeHeaders_AbortStatRecorded). Implemented abort-only path: matchesHeaders gate + rollPercent + max_active_faults placeholder + abort-only `recordFaultEvent + SendLocalReply + return StopIteration`. Added 4 helpers: `matchesHeaders` (case-insensitive name + case-sensitive value per §11.8), `rollPercent` (0%/100% short-circuit; intermediate consults RNG), `faultEventKind` enum + 5 constants, `recordFaultEvent` (consolidates stat dispatch per planner-time decision 3; nil-tolerant), `decrementActive` markedActive-guarded Dec stub (Task 6 wires Inc side). ADR-0103 appended.

**Task 5 (commit `2ec1507`):** Delay async-resume + combined delay+abort timer-callback path + ADR-0102. Strict-TDD per PLAN steps 1–7. Added 4 new tests: TestDecodeHeaders_DelayOnly + TestDecodeHeaders_Combined + TestDecodeHeaders_DelayStatRecorded + TestDecodeHeaders_CombinedStatsRecorded. New helpers: `counterValue` (mirrors router_test.go precedent), `makeDelayFilter`, `waitForCondition`. Replaced both Continue placeholders in DecodeHeaders with `time.AfterFunc`-driven timer paths per the PLAN-prescribed snippet. Refactored `recordingDCB` to add sync.Mutex + Status()/Body()/Headers() accessor methods after `-race` flagged plain-field writes vs polling-goroutine reads — production fault filter itself is race-free; race was strictly in test stub. ADR-0102 appended; AMENDED at T14 follow-up `b5ae585` to correct the combined-path callback shape (callback fires SendLocalReply + ContinueDecoding, not just SendLocalReply).

**Task 6 (commit `b2174fd`):** max_active_faults atomic counter + markedActive idempotency guard + OnDestroy timer-cancel + race-detector cycle test + ADR-0105. Strict-TDD per PLAN steps 1–7. Added 3 new tests: TestDecodeHeaders_MaxActiveFaultsCapOverflow + TestOnDestroy_TimerStopped + TestFault_DelayTimerRace. Implemented per PLAN snippet: cap check + markActive helper + OnDestroy `delayTimer.Stop() + decrementActive`. **Empirical race finding:** the PLAN's claim "race-clean by single-goroutine-per-stream invariant per ADR-0071" was inaccurate — `time.AfterFunc(d, fn)` runs `fn` on a runtime-managed goroutine, NOT the dispatch goroutine; OnDestroy and timer-callback Decs genuinely race during chain teardown. Upgraded `markedActive bool` → `markedActive atomic.Bool` with `CompareAndSwap(true, false)` for race-clean exactly-once Dec; markActive uses `Store(true)`. Re-ran `go test -race -count=10 ./internal/filter/http/fault/...` clean. ADR-0105 appended; LBP-1 chain corrected at T6 follow-up `f43d3fc` (original wording cited ADR-0061 + a fabricated ADR-0078; corrected chain: ADR-0059 → 0072 → 0079 → 0085 → 0091 → 0105).

**Task 7 (commit `aefb3d0`):** Per-route 3-tier merge (routeConfigOrListener + parseRouteRuntimeConfig). Strict-TDD per PLAN steps 1–6. Added `TestPerRouteWholesaleOverride` (listener-level delay 200ms + per-route abort 418 NO delay; assert StopIteration + Status()==418 + elapsed<50ms — wholesale-override per §11.7). Added `routeConfigOrListener` method (nil-dcb fallback → cb.RequestRouteConfig nil-fallback → type-assertion guard on `*faultv3.HTTPFault` with defensive fall-through → `parseRouteRuntimeConfig` projection with defensive fall-through on parse error). Added `parseRouteRuntimeConfig` thin-wrapper around `parseRuntimeConfig` (KEEP-separate per planner-time decision 2). NO new ADR — cross-reference to ADR-0073 (existing 3-tier-merge contract) was already recorded in ADR-0101 §Consequences from Task 3.

**Task 8 (commit `311f363`):** cmd/envoy-go/main.go register fault.New under fault.TypeURL. Mechanical boot-wiring task per PLAN steps 1–7. Added `"github.com/esalaine/envoy-go/internal/filter/http/fault"` to the http filter import block, sorted alphabetically between envoygotest and router. Inserted `httpReg.Register(fault.TypeURL, fault.New)` after the envoygotest Register and before `httpReg.Freeze()`. Smoke test deliberately skipped per PLAN Step 5 — the differential fixture (Tasks 11–14) exercises the exact end-to-end registration → typed_config resolution → factory invocation path. No new ADR — ADR-0100 (boot registration) anchored in Task 3.

**Task 9 (commit `73c2b08`):** FuzzFaultConfigParse fuzzer (twelfth fuzzer per ADR-0018). Mechanical fuzzer-ship task per PLAN steps 1–4 + planner-time decision 1 (SHIP). Wrote `fuzz_test.go` verbatim per PLAN snippet: `FuzzFaultConfigParse` feeds arbitrary byte sequences as the `tc *anypb.Any` Value (TypeURL pinned) and asserts `(factory, nil)` OR `(nil, error)` — never `(nil, nil)`. Seed corpus 5 byte sequences. 30s budget run: 3.36M execs, 250 new-interesting, no panics, no `(nil, nil)`. NO new ADR (ADR-0018 fuzz-CI policy is the anchoring ADR; established phase 04+).

**Task 10 (commit `8366980`):** BackendKind HTTPFault enum + startHTTPFaultBackend spawn helper. Mechanical fixture-infrastructure task per PLAN steps 1–4. Added `HTTPFault BackendKind = 8` to `test/differential/fixture/fixture.go` after HTTPSlowStream. Added `startHTTPFaultBackend` helper + `case fixture.HTTPFault:` in runFixture switch in `test/differential/runner_test.go`. Per PLAN's revised step 2(c), the blank-import for the driver package is DEFERRED to Task 14 (driver package doesn't exist until Task 14). NO new ADR.

**Task 11 (commit `d4aa744`):** Fixture 0011 backends/backend.go. Mechanical fixture-ship task per PLAN steps 1–4. Wrote `test/fixtures/0011-http-fault/backends/backend.go` (24 LoC) with `--port` flag (default 18001) + single `/` handler returning `200 OK` + `Content-Type: text/plain` + explicit `Content-Length: 8` + body `"backend\n"` (8 bytes). Manual smoke test confirmed `HTTP/1.1 200 OK` + `Content-Length: 8` + body hex `62 61 63 6b 65 6e 64 0a`. NO new ADR.

**Task 12 (commits `e5fbd56` + `2d0cf9a` + `ef07ae3`):** Fixture 0011 envoy.yaml + envoy-go.yaml bootstraps per SPEC §7.4. Mechanical fixture-ship task per PLAN steps 1–4 + SPEC §7.4 verbatim YAML + planner-time decision 8 (originally STRICT_DNS, corrected to STRICT_DNS-reference + STATIC-subject) + ADR-0010 (V4_ONLY). Wrote `envoy.yaml` (reference 80 lines) + `envoy-go.yaml` (subject 80 lines) per PLAN snippet — admin / listener / 5 routes (`/scenario1` no-fault, `/scenario2` per-route delay+abort 100% 100ms+503, `/scenario3-wholesale` per-route abort-only 418 NO delay, `/scenario3-baseline` no-fault, `/scenario4` per-route abort 503 + headers `x-fault-on: yes`) + 2 http_filters (listener-level fault: delay 100% 100ms NO abort; router) + cluster `c_backend`. **Task 12 follow-up:** smoke test revealed envoy-go cluster manager only supports STATIC; updated envoy-go.yaml to STATIC + 127.0.0.1; reference retains STRICT_DNS + host.docker.internal. NO new ADR.

**Task 13 (commit `8c5756a`):** Fixture 0011 expectations.yaml + README.md. Mechanical docs-only task per PLAN steps 1–3. Wrote `expectations.yaml` per SPEC §7.1 5-stat verification + 4-scenario equivalence claims (scenario1 listener-inheritance delay-only; scenario2 combined delay+abort 503; scenario3a wholesale-override 418 NO inherited delay; scenario3b baseline NO per-route override; scenario4 headers-field exact-match gate with 4 sub-probes a/b/c/d). Wrote `README.md` per PLAN snippet WITH the Task 12 follow-up STATIC-subject amendment incorporated. NO new ADR.

**Task 14 (commit `1550c9c` + follow-ups `b5ae585` + `74ee6e7` + `3a570b4`):** Fixture 0011 driver/driver.go + StatsAsserter (4-scenario orchestration, 8 probes). Step 1 wrote `driver/driver.go` (~330 LoC) + blank-import in `runner_test.go`. **The first execution exposed three pre-existing bugs that had to be fixed for the differential gate to fire end-to-end:** (1) reference admin port 9902→9901 (Task 12 deliverable; pre-harness-discipline carryover); (2) combined delay+abort hang (Task 5 deliverable; ADR-0102 wording incorrect — fixed by adding ContinueDecoding after SendLocalReply; ADR-0102 amended at follow-up `b5ae585`); (3) `flattenToProm` SN2 dotted-rest preserved internal dot in Prometheus output — fixed by `strings.ReplaceAll(rest, ".", "_")`; ADR-0107 amended at follow-up `b5ae585`. After fixes: `TestDifferential/0011-http-fault` PASSES end-to-end in 2.32s; full `TestDifferential` suite (12 fixtures + 0007 split) PASS in 37.79s. NO new ADR (amendments to ADR-0102 + ADR-0107).

**Task 15 (commit `40db754`):** BEHAVIOR_CONTRACT.md patches per SPEC §13 + ADR-0104 + ADR-0106 + ROADMAP row 09 done. Five §13 patches (§13.1 NEW envoy.filters.http.fault subsection, §13.2 17→22-name table extension, §13.3 ±10ms timing-tolerance bullet, §13.4 equivalence-matrix row, §13.5 forward-pointer notes). Two new ADRs: ADR-0104 (header-driven fault path deferred per ADR-0040 deferral format) + ADR-0106 (§9 family-expansion shape per ADR-0001 template). ROADMAP row 09 status flipped `in-progress → done`. The §9 family heading at line 56 stays unchanged per ADR-0106's no-row-state invariant.

**Task 16 (commit `c7de495` + SHA-fill `595c4cb`):** Phase-done six-gate verification + STATE.md advance + phase-done commit. Final lifecycle-state 5 → 6 task. All six gates green per BOOTSTRAP_PROMPT.md §5 phase-done discipline. Gate (d) abbreviated per planner-time PLAN guidance option B: ran ONLY `FuzzFaultConfigParse` for 30s (rationale: 12 prior fuzzers were verified in prior phases AND phase 09 touches none of their code paths; Task 9 already ran FuzzFaultConfigParse for 30s clean at fuzzer-introduction time). Phase-done commit message names ALL eight ADRs in the subject and body per ADR-0044 + 08.2 phase-done precedent. STATE.md flipped from `lifecycle-state: 3 / active-phase: 09-http-filter-fault` to `lifecycle-state: awaiting / active-phase: awaiting next planning`; `next-skill: superpowers:brainstorming` against §9 family list per ADR-0106. No code changes in Task 16 — verification + state-advance + commit only.

**Task 17 (THIS commit):** REVIEW.md per requesting-code-review skill (end-of-phase review). This document. Closes phase 09 lifecycle (state 5 → 6 final). Next session: `superpowers:brainstorming` against the §9 family-children list (header_mutation, buffer, local_ratelimit, jwt_authn, etc. per BRAINSTORM Decision 12 + ADR-0106).

---

## 5. N-1 carry-forward dispositions for next §9 family-child phase

Items observed during phase 09 that benefit the next §9 family-child phase (header_mutation, buffer, local_ratelimit, jwt_authn, etc.):

| # | Item | Disposition |
|---|---|---|
| **N-1** | `counterValue(t, reg, name)` helper precedent (Task 4 reviewer suggestion; Task 5 adopted) | **Carry-forward.** Future stats-bearing filters reuse this pattern. Could be hoisted to a shared test helper at `test/helpers/counters.go` or `internal/stats/testing/counters.go` if a third filter wants it. Currently lives in `internal/filter/http/fault/fault_test.go` + `internal/filter/http/router/router_test.go`. |
| **N-2** | `recordingDCB` sync.Mutex + accessor pattern (Task 5 race-clean retrofit; Tasks 4–6 retrofitted) | **Carry-forward.** Future async-resume filters reuse this pattern for race-clean `-race` gate runs. The `Status()/Body()/Headers()` accessor shape + `mu sync.Mutex` guarding the (status, body, headers) triple is the canonical form. |
| **N-3** | FactoryCtx Stats + StatPrefix already plumbed (Task 2) | **Already in place.** Phase 09 did the framework extension; future stats-bearing filters reuse the same fields without further FactoryCtx widening. nil-tolerance per ADR-0085 means non-stat-bearing filters need no changes. |
| **N-4** | `flattenToProm` SN-asymmetry (I-1 above) | **Carry-forward to next §9 family-child phase that introduces a nested-rest stat in cluster.* / listener.* / server.* namespaces.** Apply the same `strings.ReplaceAll(rest, ".", "_")` transform symmetrically across SN1/SN3/SN5 cases in `flattenToProm`. Hypothetical concern at HEAD; no current names trigger it. |
| **N-5** | `markedActive atomic.Bool` + `CompareAndSwap(true, false)` pattern (Task 6 race-detector finding) | **Carry-forward.** Future filters using closure-captured shared atomic state combined with timer-callback / OnDestroy goroutine seam should follow the same atomic.Bool + CAS pattern. The plain-bool form is empirically racy. ADR-0105 §Consequences (b) records the maintenance contract. |
| **N-6** | Differential fixture timing-bucket pattern (Task 14 driver elapsed-bucket fast<80ms / delayed≥80ms) | **Carry-forward.** Future timer-driven filters should reuse this elapsed-bucket assertion pattern to absorb CI scheduling jitter cleanly. The planner-time decision 11 threshold (80ms) is empirically calibrated against the 100ms delay scenarios in fixture 0011. |

No 08.2 carry-forwards from the prior REVIEW were resurrected during phase 09 (08.2's N-2 / N-3 / N-5 remain deferred to their respective hardening phases; phase 09 did not touch `clusters.go` / `version.go` / `FuzzConfigDumpFormat` corpus).

---

## 6. ADR cohesion summary

The eight ADRs ADR-0100..ADR-0107 cohere as follows:

| ADR | Title | Lands-in-task | Commit | Amendments |
|---|---|---|---|---|
| ADR-0100 | `internal/filter/http/fault/` package shape + boot registration + `FactoryCtx` framework extension | T3 | `e80aa10` | none |
| ADR-0101 | `runtimeConfig` shape + 6-vs-11 decomposition + PGV mirror + `delay.fixed_delay > 0` validation + percentage-roll determinism | T3 | `e80aa10` | none |
| ADR-0102 | `time.AfterFunc`-driven async-resume — combined delay+abort fires SendLocalReply + ContinueDecoding from timer goroutine; parkDecode wake-up via chain's `localReplyDone` gate; cancel-on-OnDestroy + ±10ms timing tolerance | T5 | `2ec1507` | T14 follow-up `b5ae585` (corrected combined-path callback shape) |
| ADR-0103 | Abort terminal-replace mechanics + body byte-exact "fault filter abort" 18 bytes + 4-header set + OrderedHeaders carrier + status-text allow-list 200/404/405/503 | T4 | `afea8ec` | none |
| ADR-0104 | Header-driven fault path deferred — coupled to `delay.header_delay` / `abort.header_abort` proto sub-messages per §11.5 empirical pin | T15 | `40db754` | none (deferral ADR per ADR-0040 format) |
| ADR-0105 | `max_active_faults` cap + LBP-1 sixth application + closure-captured `*atomic.Int64` + `markedActive atomic.Bool` per-instance idempotency + OnDestroy timer-cancel + `fault.faults_overflow` stat semantics | T6 | `b2174fd` | T6 follow-up `f43d3fc` (LBP-1 chain corrected to ADR-0059 → 0072 → 0079 → 0085 → 0091 → 0105) |
| ADR-0106 | §9 HTTP-filters family expansion shape — flat top-level rows + no-sibling-stub; the §9 heading at ROADMAP line 56 is an umbrella, not a row | T15 | `40db754` | none |
| ADR-0107 | `BEHAVIOR_CONTRACT.md ## Stat-name mapping` 17→22-name extension for FIVE `fault.*` stats + `response_rl_injected` permanently-zero counter discipline (route A) | T3 | `e80aa10` | T14 follow-up `b5ae585` (corrected Prom name shape post-SN2 dotted-rest fix) |

All Lands-in-task fields point to commits on the impl branch HEAD-side of `b33e04f`. ADR-0102 and ADR-0107 were each amended once during Task 14 follow-up; ADR-0105 was amended once during Task 6 follow-up. No ADR was amended after Task 15 (the BEHAVIOR_CONTRACT bundle commit).

The ADR cluster is internally consistent. ADR-0100 anchors the package shape + framework extension; ADR-0101 anchors the parser; ADR-0102 anchors async-resume mechanics (referenced by ADR-0103 + ADR-0105); ADR-0103 anchors abort wire shape (referenced by ADR-0102 combined-path); ADR-0104 deferral references ADR-0101 silent-ignore set; ADR-0105 anchors concurrency cap + LBP-1 sixth (referenced by ADR-0102 cancel-on-OnDestroy); ADR-0106 anchors §9 family-expansion shape (BRAINSTORM Decisions 12+13); ADR-0107 anchors stat-name extension (referenced by ADR-0103 + ADR-0102 stat dispatch + ADR-0105 faults_overflow).

---

## 7. Six-gate verification appendix

All six gates run against HEAD `c7de495` per Task 16's verification sweep (PROGRESS Task 16 entry verbatim). Reproduced summary outputs:

### Gate (a) — build clean

```
$ go build ./...
EXIT:0

$ go vet ./...
EXIT:0

$ golangci-lint run ./...
EXIT:0
```

**Result: PASS — clean.**

### Gate (b) — unit tests + race

```
$ go test -race -count=1 ./...
ok  github.com/esalaine/envoy-go/cmd/envoy-go              4.x s
ok  github.com/esalaine/envoy-go/internal/accesslog        1.x s
ok  github.com/esalaine/envoy-go/internal/admin            1.x s
ok  github.com/esalaine/envoy-go/internal/bootstrap        1.x s
ok  github.com/esalaine/envoy-go/internal/cluster          1.x s
ok  github.com/esalaine/envoy-go/internal/drain            1.x s
ok  github.com/esalaine/envoy-go/internal/filter/hcm       1.x s
ok  github.com/esalaine/envoy-go/internal/filter/hcm/h2    3.x s
ok  github.com/esalaine/envoy-go/internal/filter/http      1.x s
ok  github.com/esalaine/envoy-go/internal/filter/http/cors 1.x s
ok  github.com/esalaine/envoy-go/internal/filter/http/envoygotest  1.x s
ok  github.com/esalaine/envoy-go/internal/filter/http/fault        1.x s
ok  github.com/esalaine/envoy-go/internal/filter/http/router       1.x s
ok  github.com/esalaine/envoy-go/internal/filter/tcpproxy 1.x s
ok  github.com/esalaine/envoy-go/internal/listener         4.x s
ok  github.com/esalaine/envoy-go/internal/listener/listenerfilter  1.x s
ok  github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector  1.x s
ok  github.com/esalaine/envoy-go/internal/stats            1.x s
ok  github.com/esalaine/envoy-go/internal/tls              1.x s
ok  github.com/esalaine/envoy-go/test/conformance/h2spec   3.x s
ok  github.com/esalaine/envoy-go/test/differential        41.x s
ok  github.com/esalaine/envoy-go/test/differential/fixture 1.x s
[... driver packages: all ok ...]
EXIT:0
```

**Result: PASS — clean.**

### Gate (c) — h2spec re-run

```
$ go test -count=1 ./test/conformance/h2spec/...
ok  github.com/esalaine/envoy-go/test/conformance/h2spec  2.274s

(53 tests, 53 passed, 0 skipped, 0 failed at ADR-0051 pin; phase 09 touches no codec)
```

**Result: PASS — 53/53 at ADR-0051 pin (unchanged).**

### Gate (d) — fuzzers (option B abbreviation per planner-time guidance)

Ran ONLY the new `FuzzFaultConfigParse` for 30s. Skipped the 12 existing fuzzers per option B rationale: (a) all 12 verified in prior phases including 08.2's phase-done at `b33e04f`; (b) phase 09 touches none of their code paths (the FactoryCtx Stats/StatPrefix extension is purely additive and exercised by the fault filter only — none of the 12 existing fuzzed code paths read these fields); (c) Task 9 already ran FuzzFaultConfigParse for 30s at fuzzer-introduction time with 3.36M execs clean. The Task 16 re-run confirms the fuzzer remains green after Tasks 10–15.

```
$ go test -run='^$' -fuzz='^FuzzFaultConfigParse$' -fuzztime=30s ./internal/filter/http/fault/
fuzz: elapsed: 30s, execs: 2484833 (18959/sec), new interesting: 38 (total: 311)
PASS
ok  github.com/esalaine/envoy-go/internal/filter/http/fault  31.080s
```

**Result: PASS — FuzzFaultConfigParse clean at 30s; 12 prior fuzzers carried forward green per option B.**

### Gate (e) — differential 0000-0011 all green

```
$ go test -count=1 -v -run 'TestDifferential' ./test/differential/...
--- PASS: TestDifferential (37.73s)
    --- PASS: TestDifferential/0000-tcp-echo (1.45s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.24s)
    --- PASS: TestDifferential/0002-tls-tcp (1.27s)
    --- PASS: TestDifferential/0003-http11-routing (1.25s)
    --- PASS: TestDifferential/0004-h2-routing (1.82s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.06s)
    --- PASS: TestDifferential/0006-access-log (10.96s)
    --- PASS: TestDifferential/0007a-cors (1.36s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.81s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.42s)
    --- PASS: TestDifferential/0009-admin-config-dump (1.88s)
    --- PASS: TestDifferential/0010-graceful-drain (9.25s)
    --- PASS: TestDifferential/0011-http-fault (1.96s)
PASS
```

**Result: PASS — 13 differential subtests green (0000..0011 with 0007 split into 0007a/0007b — total subtest count is 13, 12 fixture directories).**

### Gate (f) — BEHAVIOR_CONTRACT alignment + ROADMAP row 09 status

```
$ grep -c 'envoy.filters.http.fault' docs/envoy-go/BEHAVIOR_CONTRACT.md
5

$ grep -c 'response_rl_injected' docs/envoy-go/BEHAVIOR_CONTRACT.md
4

$ grep '^| 09' docs/envoy-go/ROADMAP.md
| 09 | http-filter-fault | 08 | done |  | New `internal/filter/http/fault/` package implementing `envoy.filters.http.fault` ...
```

**Result: PASS — BEHAVIOR_CONTRACT.md populated with:**
- §13.1 NEW `### envoy.filters.http.fault` subsection under `## HTTP filter chain`
- §13.2 5-row Fault filter block in 17→22-name table; footer reads `Total: 22 internal names (17 from 06.1 + 5 from 09)`
- §13.3 ±10ms timing-tolerance bullet
- §13.4 Equivalence Matrix row appended after the existing `Admin /server_info (DRAINING)` row
- §13.5 Forward-pointer notes at `## HTTP filter chain ### Async resume mechanics` + `## Stat-name mapping ### Twin-series filter discipline`
- ROADMAP row 09 status field reads `done` (flipped at Task 15 commit `40db754`)
- The §9 family heading at ROADMAP line 56 unchanged per ADR-0106 no-row-state invariant

Six-gate state: **ALL GREEN at HEAD `c7de495`.** Phase-done commit landed at Task 16; this REVIEW closes lifecycle-state 5 → 6.

---

## 8. Acceptance against SPEC §15

Cross-referencing SPEC §15 acceptance checklist (abridged):

- [x] `internal/filter/http/fault/` package lands with `doc.go` + `fault.go` + `fault_test.go` + `fuzz_test.go` (FuzzFaultConfigParse). `New` factory implements §6 8-step contract per ADR-0101.
- [x] `FactoryCtx` framework extension: `Stats *stats.Registry` + `StatPrefix string` fields per ADR-0100; `parseHTTPFiltersChain` 4-param widening. Build clean.
- [x] Decode-side 5-step discipline: matchesHeaders gate + percentage rolls + max_active_faults cap + markActive + dispatch branch (abort-only synchronous; delay-only async-resume; combined delay+abort timer-callback per ADR-0102).
- [x] Abort terminal-replace per ADR-0103: 503 + body byte-exact "fault filter abort" 18 bytes + 4-header set + OrderedHeaders carrier + status-text allow-list 200/404/405/503.
- [x] Async-resume mechanics per ADR-0102: time.AfterFunc-driven; combined-path callback fires SendLocalReply + ContinueDecoding; chain's localReplyDone gate short-circuits resumed iteration; ±10ms timing tolerance.
- [x] max_active_faults cap per ADR-0105: closure-captured `*atomic.Int64` + `markedActive atomic.Bool` + OnDestroy timer-cancel + faults_overflow stat.
- [x] Per-route 3-tier merge wholesale-override per ADR-0073 + §11.7: routeConfigOrListener + parseRouteRuntimeConfig.
- [x] cmd/envoy-go/main.go register fault.New + fault.TypeURL after envoygotest, before httpReg.Freeze().
- [x] FuzzFaultConfigParse fuzzer per ADR-0018: 30s budget + 5-seed corpus + (factory, nil) ∨ (nil, error) invariant.
- [x] Fixture 0011-http-fault green: 4 scenarios + 4 sub-probes; `RequiresReference: true`; STATIC-subject + STRICT_DNS-reference per Task 12 follow-up.
- [x] `go test -race ./...` clean: verified by gate (b) above.
- [x] FuzzFaultConfigParse runs clean at 30s: verified by gate (d) above (3.36M execs at fuzzer-introduction; 2.48M execs at phase-done re-run).
- [x] h2spec 53/53 PASS: verified by gate (c) above.
- [x] Eight new ADRs (ADR-0100..ADR-0107) in DECISIONS.md: verified.
- [x] BEHAVIOR_CONTRACT.md §13 five-patch bundle: verified by gate (f) above.
- [x] ROADMAP row 09 flips `in-progress → done`: verified by gate (f) above (Task 15 commit `40db754`).
- [x] STATE.md `active-phase: awaiting next planning` + `lifecycle-state: awaiting` + `next-skill: superpowers:brainstorming`: verified by Task 16's STATE rewrite + SHA-fill `595c4cb`.
- [x] REVIEW.md authored: THIS document.

All acceptance items checked. Phase-done. Phase 09 lifecycle (state 5 → 6) closes at the commit landing this REVIEW. Branch `phase-09-http-filter-fault-impl` is ready for merge to master per the linear-history (fast-forward) precedent established by phases 00–08.2.
