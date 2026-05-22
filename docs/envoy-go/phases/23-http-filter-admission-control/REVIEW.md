# Phase 23 — HTTP filter `envoy.filters.http.admission_control` — Review

**Phase id:** `23` (SIXTEENTH §9 HTTP-filters family-row to land per ADR-0106; closes the row 23 single-row landing settled at the SPEC commit per ADR-0045 — NOT split into 23.1+23.2; **FIRST §9 row since phase-22's roster amendment to skip ADR-0125** (REUSE-by-absence per SPEC §5.4 — no `AdmissionControlPerRoute` proto message in v1.32.4 or v1.37.x; canonical-per-route roster STAYS 9); **D-hypothesis BROKE — ADR-0196 CONSUMED at Task 9a** (PD-5 `:status`-via-header assumption was INVALID; encode-side `ResponseStatus()` framework accessor introduced; REVISES the "ZERO new framework primitives" plan claim → phase-23 introduced ONE new encode-side primitive); **next-free ADR-0197**.)
**Slug:** `23-http-filter-admission-control`
**Branch under review:** `phase-23-http-filter-admission-control-impl`
**Range:** branch tip is this Task 12 commit (REVIEW.md + STATE.md re-advance + ROADMAP row 23 flip + PROGRESS Task 12 entry; 12 task-landing commits at worktree HEAD [Tasks 1-12 + 9a as a separate slot] + their SHA-fill follow-ups). The last-commit SHA-fill on STATE.md is deferred to the post-`wt-merge` follow-up per the phase-09..22 IMPL-stage close pattern.
**Parent ROADMAP row:** row `23 http-filter-admission-control` flipped `in-progress → done` at this Task 12 commit (date `2026-05-22`). Per-cell IMPL-done annotation appended documenting the 12+9a-task IMPL landing + 3 NEW ADR landings + 6-gate verbatim outputs + SPEC §15 acceptance summary + notable IMPL-time findings.
**PLAN tip SHA:** `af4a0fe` (Squash merge phase-23-http-filter-admission-control-plan). **SPEC tip SHA:** `a64ee71` (Squash merge phase-23-http-filter-admission-control-spec). **BRAINSTORM tip SHA:** `3040a6b` (Squash merge phase-23-http-filter-admission-control-brainstorm).
**Reviewer method:** Inline authoring by the implementing session per the PLAN Task 12 direction. Inputs: SPEC §15 16-item acceptance checklist + PLAN §"PD-1..PD-10 decisions" + PLAN's 12-task structure + the branch diff + PROGRESS.md per-task entries (Tasks 1-12 + 9a) + DECISIONS.md ADR-0194 + ADR-0195 + ADR-0196 §Decision + §Consequences full bodies + BEHAVIOR_CONTRACT.md 4-edit bundle (Task 11) + phase-21 REVIEW.md structural template precedent.
**Links:** [PLAN.md](./PLAN.md) · [SPEC.md](./SPEC.md) · [BRAINSTORM.md](./BRAINSTORM.md) · [PROGRESS.md](./PROGRESS.md).
**Six-gate state at HEAD:** all GREEN per Task 12's verification sweep — outputs reproduced verbatim in §7 below.

This review covers the full phase-23 surface: the NEW `internal/filter/http/admission_control/` package (compiled_config.go + controller.go + rand.go + clock.go + stats.go + admission_control.go + decode_headers.go + encode.go + doc.go + fuzz_test.go + the unit-test surface compiled_config_test.go + controller_test.go + rand_test.go + clock_test.go + stats_test.go + admission_control_test.go + encode_test.go); the FRAMEWORK CHANGE at `internal/filter/http/callbacks.go` + `internal/filter/http/chain.go` + `internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go` adding the `ResponseStatus() int` encode-side accessor (ADR-0196; NEW framework primitive set-once by HCM dispatch, read via accessor; mirrors ADR-0165 pattern); the boot-registration at `cmd/envoy-go/main.go` (alphabetical between `adaptive_concurrency` and `bandwidthlimit`; 18 HTTP filters); the NEW differential fixtures `0030-http-admission-control` (cross-side, 4 scenarios) + `0031-http-admission-control-boot-reject`; the 32nd fuzzer `FuzzAdmissionControlConfigParse` (31 corpus seeds; clean at 30s); the BEHAVIOR_CONTRACT.md 4-edit bundle (Task 11); the 3 NEW ADR landings (ADR-0194 + ADR-0195 + ADR-0196).

This REVIEW closes phase-23's IMPL lifecycle (state 5 → 6). It is the final task before merge to master.

---

## 1. Summary

**APPROVED.** All six phase-done gates are GREEN at HEAD per Task 12's verification sweep. The implementation faithfully realizes the SPEC across all 12 + 9a PLAN tasks (Tasks 1-12 + 9a). `envoy.filters.http.admission_control` is the SIXTEENTH §9 HTTP-filters family-row to ship under ADR-0106; it lands the row-23 single-row settled at SPEC commit `a64ee71` per ADR-0045 (NOT split into 23.1+23.2).

**Three IMPL-time deviations from the PLAN** are recorded at §3 below: (1) the D-hypothesis BROKE — ADR-0196 was consumed by the `ResponseStatus()` encode-side framework accessor, REVISING the "ZERO new framework primitives" plan claim; (2) a dead-assertion fix was required in fixture 0030 (AssertSubject was never called on the cross-side path; moved to StatsAsserter); (3) PD-3 health-check arm is NOT-MODELED at MVP.

**Three NEW ADR landings (2 anticipated + 1 unanticipated from the D-hypothesis breaking):**
- **ADR-0194** §Decision + §Consequences FULL — algorithm + package shape + inline Rand/Clock seams + deterministic-regime differential strategy — landed at Task 4.
- **ADR-0195** §Decision + §Consequences FULL — RTDS `runtime_key` deferral PARSE-REJECT (5 arms; `enabled`-absent⇒ENABLED per AMEND-4) — landed at Task 2.
- **ADR-0196** §Decision + §Consequences FULL — encode-side `ResponseStatus()` framework accessor (set-once-by-dispatch / read-via-accessor; mirrors ADR-0165/ADR-0174 pattern; cross-phase-reusable) — landed at Task 9a.

**Next-free ADR: ADR-0197** (ADR-0196 consumed; NOT ADR-0196 as the SPEC §10 D-style hypothesis predicted).

**FIRST ADR-0125-skip since phase-22's roster amendment.** Canonical-per-route roster STAYS 9.

---

## 2. SPEC §15 acceptance verification

Per SPEC §15. All 16 items verified.

### A. Six-gate verification (items 1-6)

- [x] **Item 1 — Gate A build clean.** **GREEN.** `go build ./...` exits 0 at HEAD. See §7 verbatim output.
- [x] **Item 2 — Gate B vet + lint clean.** **GREEN.** `go vet ./...` exits 0; `golangci-lint run` exits 0; no new lint suppressions across the phase-23 surface. See §7 verbatim output.
- [x] **Item 3 — Gate C race clean.** **GREEN.** `go test -race -count=1 ./...` clean repo-wide in a single run (exit 0; `grep -cE "^FAIL|^--- FAIL" /tmp/race-full.log` = 0; 62 `ok` packages). The substantive race-cleanliness is GREEN across the phase-23 packages: `internal/filter/http/admission_control` ok 1.092s; `internal/filter/hcm` ok 1.102s; `internal/filter/hcm/h2` ok 3.544s. See §7 verbatim output.
- [x] **Item 4 — Gate D differential clean.** **GREEN.** `go test -count=1 -timeout=15m ./test/differential/ -run 'TestDifferential'` exits 0 in 81.4s. All 33/33 fixtures PASS (0000-tcp-echo through 0031-http-admission-control-boot-reject). No 0028 freeTCPPort flake on this run (flake class documented at 22.2 REVIEW §7.4 — not a defect). See §7 verbatim output.
- [x] **Item 5 — Gate E fuzz clean.** **GREEN.** `FuzzAdmissionControlConfigParse` seed corpus clean (31 seeds; all PASS); 30s live-fuzz clean: 2,416,440 execs at ~50k/sec; 15 new interesting / 277 total; 0 panics; 0 crashers. Total fuzzer count = **32** (`grep -rE '^func Fuzz' --include='*.go' .` = 32 unique names). See §7 verbatim output.
- [x] **Item 6 — Gate F h2spec clean.** **GREEN.** `go test -v -count=1 ./test/conformance/h2spec/` reports `53 tests, 53 passed, 0 skipped, 0 failed` at the ADR-0051 pin. PASS at 2.30s (first run; no flake this session). See §7 verbatim output.

### B. Fixture coverage (item 7)

- [x] **Item 7 — Two-directory differential per §7.** **GREEN.** `0030-http-admission-control` (4 scenarios: (a) parse_ok / (b) all_admit_healthy CROSS-SIDE byte-exact / (c) stat_surface / (d) pass_through_disabled) + `0031-http-admission-control-boot-reject` (`sr_threshold < 1.0%` shared boot-reject substring). Fixture dir count 31 → 33 confirmed. The (b) all-admit cross-side leg is the load-bearing byte-exact requirement (P_reject=0 ⇒ RNG-independent per AMEND-2; RATIFIED). Evidence: Task 9 + Task 9 follow-up PROGRESS entries + Gate D GREEN at 33/33.

### C. Stat-surface verification (item 8)

- [x] **Item 8 — 3-counter stat surface byte-exact.** **GREEN.** Per ADR-0194 + AMEND-3 + SPEC §11 D1: all 3 stat names anchored in `stats.go` as package-level `const` declarations (D5 compile-time guard per PLAN) + table-driven assertion in `stats_test.go`: `rq_rejected` (counter) + `rq_success` (counter) + `rq_failure` (counter; NOT `rq_error` per AMEND-3). All registered under `http.<HCM_stat_prefix>.admission_control.<stat>` prefix template. Stat surface 107 → 110 names confirmed at BEHAVIOR_CONTRACT.md stat-table extension at Task 11 edit 3. NO gauges (COUNTER-only per AMEND-3). Evidence: Task 3 PROGRESS entry + Task 11 PROGRESS entry + `grep -c "departure count 14 → 15" docs/envoy-go/BEHAVIOR_CONTRACT.md` non-zero match.

### D. Algorithmic-fidelity verification (item 9)

- [x] **Item 9 — Algorithmic fidelity per §14.1 Layer A.** **GREEN.** All Layer A test families landed at Task 4 `controller_test.go`: `TestShouldReject_Boundary_AtKnifeEdge_Admits` + `TestShouldReject_Boundary_OneLessThanKnifeEdge_Rejects` + `TestShouldReject_Boundary_PZero_NeverRejects` (P=0 RNG-independent; 20 r-values all admit) + `TestProbabilityFormula_*` (exponent + sr_threshold-divides + aggression-floor + max_rejection_probability clamp + P=0-floor) + `TestController_FAKE_TIME_Window_*` (per-second bucket rollover + stale-purge via `fakeClock`) + `TestRpsSuppression_*` (gate-2 admits-without-reject) + `TestRecordDiscipline_*` (rejected/disabled not recorded per AMEND-11) + `TestController_Concurrent_*` (race tests). All clean under -race at Gate C. Evidence: Task 4 PROGRESS entry + Task 12 Gate C verification.

### E. PARSE-REJECT roster verification (item 10)

- [x] **Item 10 — PARSE-REJECT roster.** **GREEN.** Per §5.1 + §5.2 + ADR-0195: 4 RATIFIED-from-config arms (oneof-absent; `sr_threshold < 1.0%`; http-range invalid; grpc-codes > 16) + 5 envoy-go-strict `runtime_key` arms (enabled / aggression / sr_threshold / max_rejection_probability / rps_threshold) — all with byte-stable error wording per ADR-0080 + PD-2 + `TestParseRejectConstants_ByteStable` table-driven coverage at `compiled_config_test.go`. 9 arms total. Evidence: Task 2 PROGRESS entry + Task 12 PROGRESS §15 closure.

### F. Reject wire-shape verification (item 11)

- [x] **Item 11 — 503 reject wire shape.** **GREEN.** Per AMEND-7 + D4 + PD-2.503: `f.cb.SendLocalReply(503, "", nil)` — status 503, EMPTY body `""`, nil headers. The `"denied_by_admission_control"` rc-details is NOT surfaceable through the 3-arg `SendLocalReply` API (ABSENT-by-API per PD-2.503); not pinned in tests. `rqRejected.Inc()` at the reject site. Asserted envoy-go-side at `TestRejectLocalReply_ByteShape` in `encode_test.go` (Task 6). Evidence: Task 5 + Task 6 PROGRESS entries.

### G. Enabled-semantics verification (item 12)

- [x] **Item 12 — `enabled` honored-matrix.** **GREEN.** Per §5.3 + AMEND-4: absent ⇒ ENABLED (OPPOSITE of phase-21 adaptive_concurrency); default_value=false ⇒ DISABLED; default_value=true ⇒ ENABLED; `runtime_key` non-empty ⇒ PARSE-REJECT (arm 5 per §5.2). `TestEnabledMatrix_*` 5-case coverage at `compiled_config_test.go`. Evidence: Task 2 PROGRESS entry.

### H. ADR landing (item 13)

- [x] **Item 13 — ADR landings.** **GREEN-WITH-NOTED-DEVIATION.** The SPEC §15 item 13 expected 2 NEW ADR §Context drafts + §Decision + §Consequences bodies (ADR-0194 + ADR-0195). **3** NEW ADRs were consumed at phase-23 IMPL (ADR-0194 + ADR-0195 + ADR-0196). The extra ADR-0196 was unanticipated (the D-hypothesis BROKE at Task 9a when PD-5's `:status`-via-header assumption was found INVALID by differential fixture 0030; see §3 IMPL-time deviation #1). ZERO in-place §Decision AMENDMENTs. ZERO ADR-0125 amendments (canonical-per-route roster STAYS 9). All 3 ADR bodies non-empty with §Decision + §Consequences fully anchored. Evidence: Task 10 PROGRESS entry (ADR final-state audit) + `grep -cE '^## ADR-0194'`, `'^## ADR-0195'`, `'^## ADR-0196'` each return 1; `'^## ADR-0197'` returns 0.

### I. BEHAVIOR_CONTRACT.md edit-bundle (item 14)

- [x] **Item 14 — 4-edit BEHAVIOR_CONTRACT.md bundle landed.** **GREEN.** Atomic landing at Task 11 per ADR-0052: (1) NEW `### envoy.filters.http.admission_control` subsection (filter scope + algorithm + both-sides discipline + PD-3 health-check NOT-MODELED note + reject wire shape + 3-counter stat surface + REUSE-by-absence per-route + ADR-0196 ResponseStatus classification note); (2) RTDS `runtime_key` PARSE-REJECT departure record (departure count 14 → 15); (3) stat-name mapping 107 → 110 table extension; (4) per-route canonical-patterns cross-reference caption update + phase-23 cross-reference paragraph. Evidence: Task 11 PROGRESS entry verbatim grep verifications.

### J. DECISIONS + STATE + ROADMAP advance (item 15)

- [x] **Item 15 — Doc-state alignment.** **GREEN-WITH-NOTED-DEVIATION.** DECISIONS.md: ADR-0194 + ADR-0195 + ADR-0196 full bodies at final state; next-free ADR-0197 (NOT ADR-0196 as the SPEC §10 D-style hypothesis predicted — the hypothesis BROKE; see §3). STATE.md re-advanced at this Task 12 commit. ROADMAP row 23 flipped to `done` at this Task 12 commit (per-cell IMPL-done annotation). 18 HTTP filters wired confirmed. Evidence: this REVIEW.md Task 12 commit.

### K. Audit-trail verification (item 16)

- [x] **Item 16 — End-to-end audit-trail.** **GREEN.** SPEC → PLAN → PROGRESS → REVIEW chain landed (BRAINSTORM `3040a6b`; SPEC `a64ee71`; PLAN `af4a0fe`; PROGRESS has per-task entries Tasks 1-12 + 9a; this REVIEW.md). Per-task PROGRESS records map 1:1 to PLAN tasks (Task 9a is the extra slot from the D-hypothesis breaking; it has its own PROGRESS entry). Each §11 pin + each §12 item recorded. D-hypothesis disposition recorded (ADR-0196 CONSUMED; next-free ADR-0197). Six-gate verbatim outputs at this REVIEW §7.

**Summary:** 14 items GREEN; 0 BLOCKED; 2 GREEN-WITH-NOTED-DEVIATION (item 13 — 3 ADRs consumed vs 2 anticipated [the D-hypothesis BROKE]; item 15 — next-free ADR-0197 not ADR-0196). No GREEN-WITH-DOCUMENTED-SCOPE-DEVIATION items (fixture 0030's 4 scenarios are all fully verified through the `StatsAsserter` cross-side path after the dead-assertion fix at the Task 9 follow-up; the cross-side byte-exact leg is the load-bearing AMEND-2-ratified check). **The D-hypothesis BROKE across the IMPL — ADR-0196 CONSUMED at Task 9a** — the encode-side `ResponseStatus()` framework accessor was required to fix PD-5's invalid `:status`-via-header assumption surfaced by differential fixture 0030. Phase-23 is NOT zero-new-framework-primitives: ONE new encode-side primitive (ADR-0196) was introduced.

---

## 3. IMPL-time deviations from the PLAN

Three deviations from the planned behavior occurred during IMPL. All are recorded here per the Task 12 instruction.

### Deviation 1 — PD-5 invalid → ADR-0196 framework primitive (D-hypothesis BROKE)

**Planned:** Phase-23 would introduce ZERO new `internal/` framework primitives (returning to phase-21's LEAN posture). The D-style SPEC §10 hypothesis predicted ADR-0196 UNCONSUMED at phase-done. PD-5 specified that encode-side HTTP response status was readable via `headers.Get(":status")`, modeled on the phase-14 compressor.

**What happened:** Differential fixture 0030 bring-up (the `all_admit_healthy` (b) cross-side leg at Task 9) FAILED — every HTTP response was misclassified as failure because envoy-go's encode chain does NOT convey the response status to encode-side filters. The `http.Header` map handed to `RunEncodeHeaders(ctx, headers, endStream)` never contains `:status` (the status lives in `resp.Status` at HCM dispatch and is written to the wire status-line separately). PD-5 was INVALID.

**Fix (approved by project owner):** A root-cause framework fix was introduced at Task 9a: `ResponseStatus() int` added to `EncoderFilterCallbacks` (set-once by HCM dispatch via `SetEncodeResponseStatus(status)` in H1/H2/beginLocalReply paths; read via accessor; mirrors the ADR-0165 set-once-by-dispatch / read-via-accessor pattern). ADR-0196 was authored in full. `encode.go`'s HTTP classification path switched from `headers.Get(":status")` to `f.ecb.ResponseStatus()`. 

**Consequence:** Phase-23 introduced ONE new encode-side framework primitive (ADR-0196). The "ZERO new framework primitives" plan claim is REVISED. Next-free ADR advances from the SPEC-time ADR-0196 to ADR-0197. The D-hypothesis **BROKE** — ADR-0196 was consumed, not buffered. **BEHAVIOR_CONTRACT.md §13 `### envoy.filters.http.admission_control` subsection includes the ADR-0196 ResponseStatus classification note.** The accessor is documented as cross-phase-reusable per ADR-0196 §Consequences.

### Deviation 2 — Dead-assertion fix in fixture 0030 (AssertSubject never called on cross-side path)

**Planned:** Fixture 0030 would implement scenarios (a)/(c)/(d) + stat assertions in `SubjectAsserter.AssertSubject`.

**What happened (Task 9 follow-up):** A reviewer pass found that `SubjectAsserter.AssertSubject` was NEVER CALLED on the cross-side runner path. The runner invokes `SubjectAsserter` ONLY on the `runReferenceLessFixture` path (`RequiresReference==false`); fixture 0030 uses `RequiresReference=true` (live `DriveReference` + load-bearing `CompareBytes` for the (b) all_admit_healthy byte-exact leg), so the runner takes the cross-side path which calls `StatsAsserter`, `DistributionAsserter`, etc. — but NOT `SubjectAsserter`. The (a)/(c)/(d) assertions were vacuous/dead.

**Fix:** Removed `AssertSubject` entirely. Implemented `AssertStats(t, refAdminAddr, subjAdminAddr)` (which the cross-side path DOES call at step 10): (c) stat_surface scrapes SUBJECT `/stats/prometheus` and asserts all 3 counters present + `rq_rejected==0` + `rq_failure==0` + **`rq_success > 0`** (positivity confirmed live); (d) pass_through_disabled dials the SUBJECT `l_test_d` disabled listener first, asserts 200, then asserts ALL THREE `hcm_d` counters are 0. Liveness was proven by a deliberate-break test (flipping `rq_success > 0` to `rq_success == 0` on the healthy backend produced a `FAIL` from `runner_test.go:890`).

**Consequence:** The (b) cross-side byte-exact leg (the load-bearing AMEND-2 ratified requirement via `CompareBytes`) was always real and was never affected. The (a)/(c)/(d) assertions are now genuinely exercised via `StatsAsserter`. All 4 logical scenarios per SPEC §7.1 are confirmed live at Gate D.

### Deviation 3 — PD-3 health-check arm NOT-MODELED at MVP

**Planned:** Per SPEC AMEND-4, `DecodeHeaders` would implement the `healthCheck()` short-circuit gate. Per SPEC AMEND-11, health-check requests would not be recorded in the window.

**Disposition (NOT a fix — a documented deferral):** `internal/filter/http/callbacks.go` confirms that `DecoderFilterCallbacks` exposes NO `StreamInfo()`, `HealthCheck()`, or `IsHealthCheck()` accessor. Adding such an accessor would be a NEW framework primitive — violating the ZERO-new-primitive constraint (which was already overridden for ADR-0196 encode-side; the health-check accessor would have been a decode-side addition of additional scope not required for correctness). PD-3 resolution: the `healthCheck()` arm is NOT-MODELED at phase-23 MVP; `DecodeHeaders` implements ONLY the `!f.cc.enabled` pass-through arm. AMEND-11's "health-check requests not recorded" is vacuous at MVP (no health-check gate to bypass; the record discipline still correctly skips rejected + disabled requests). Documented at `decode_headers.go` gate 1 comment + BEHAVIOR_CONTRACT.md `### envoy.filters.http.admission_control` subsection + PROGRESS PD-3 entry. Does NOT consume any ADR.

**Future-phase closure surface:** A future decoder-side `IsHealthCheck()` primitive — either standalone or as part of a broader stream-info accessor family — would enable full AMEND-4 fidelity. The deferral does NOT affect correctness for the primary MVP use case (admission control of non-health-check traffic).

---

## 4. ADR roster

Three ADR §Decision-touchpoints landed at phase-23 IMPL. All landed at their per-Task Lands-in-Tasks per ADR-0044.

| ADR | §Decision / §Consequences disposition | Lands-in-Task | Commit SHA |
|---|---|---|---|
| **ADR-0194** | **§Decision + §Consequences FULL** — admission-control package shape + TypeURL + controller struct + sliding-window mechanics (`std::deque`-of-per-second-buckets per AMEND-6) + rejection-probability formula `max(0, min(max_rej, ((n−s/sr)/(n+1))^(1/aggr)))` per AMEND-1 + integer-modulo decision `(1e4·P) > (r%1e4)` per AMEND-2 + inline `Rand` seam (`Uint64()` not `Float64()`) + inline `Clock` seam (`Now()` only) + classification discipline (HTTP `<500` / gRPC 11-code set / gRPC-trailers per AMEND-5/10) + record discipline per AMEND-11 + 3-counter stat surface per AMEND-3 + deterministic-regime differential strategy | Task 4 | `c05ea6a` |
| **ADR-0195** | **§Decision + §Consequences FULL** — RTDS `runtime_key` deferral PARSE-REJECT (5 arms for enabled / aggression / sr_threshold / max_rejection_probability / rps_threshold; each `"admission_control: <field>.runtime_key is not yet supported; use <field>.default_value"`); `enabled`-absent⇒ENABLED per AMEND-4 (OPPOSITE of phase-21; `PROTOBUF_GET_WRAPPED_OR_DEFAULT(...,true)`); SINGLE envoy-go-strict departure (departure count 14 → 15) | Task 2 | `6ec193f` |
| **ADR-0196** | **§Decision + §Consequences FULL** — encode-side `ResponseStatus() int` framework accessor (set-once-by-HCM-dispatch via `FilterChain.SetEncodeResponseStatus(status)` at H1/H2/beginLocalReply seeding sites; read via `encoderCB.ResponseStatus()`; mirrors ADR-0165 set-once pattern; cross-phase-reusable); REVISES phase-23 "ZERO new framework primitives" claim → ONE new encode-side primitive; PD-5 `:status`-via-header assumption SUPERSEDED; D-hypothesis BROKE | Task 9a | `85e03ba` |

**Next-free ADR: ADR-0197** (ADR-0196 consumed; DECISIONS.md tail advances to ADR-0196 full body; next-free unconsumed = ADR-0197).

**NO ADR-0125 amendment.** ADR-0125's canonical-pattern roster stays unchanged after phase 23 — FIRST §9 row to skip ADR-0125 amendment since phase-22's roster amendment (8 → 9 at 22.3). REUSE-by-absence is the stronger form: the proto absence of `AdmissionControlPerRoute` in v1.32.4/v1.37.x enforces listener-scoped-only via HCM-parse-time PARSE-REJECT per SPEC §5.4 + ADR-0195 §Consequences. Canonical-per-route roster STAYS 9.

---

## 5. Per-Task summary

12 task-landing commits + 9a + their SHA-fill follow-ups at worktree HEAD; this Task 12 REVIEW + STATE/ROADMAP + PROGRESS-append is the final commit.

- **Task 1 — Execution-precondition check + PROGRESS.md preamble** (`3a09611`). 15 cold-start preconditions verified; PD-1..PD-10 reproduced in preamble; ADR tail at 195 (ADR-0194 + ADR-0195 §Context drafts at master per ADR-0044 convention). 3 representative fuzzers (`FuzzAdaptiveConcurrencyConfigParse` + `FuzzLuaConfigParse` + `FuzzBootstrapLoad`) spot-checked at 20s each; all PASS clean. The phase-23-new surface was absent at cold-start. Full pre-existing differential regression baseline PASS in 79.8s.
- **Task 2 — NEW `compiled_config.go` + 9-arm PARSE-REJECT roster + ADR-0195** (`6ec193f` + code-quality follow-up). compiled_config.go + compiled_config_test.go land per ADR-0195. ADR-0195 §Decision + §Consequences full bodies anchored. I1/I2/I3/M1/M3 code-quality follow-up applied.
- **Task 3 — NEW `rand.go` + `clock.go` + `stats.go` seams + test-scope fakes** (`a476be7` + code-quality follow-up). `Rand` interface `Uint64()` (NOT `Float64()` per AMEND-2) + `defaultRand`; `Clock` interface `Now()` + `defaultClock`; `filterStats` 3-counter struct + `newFilterStats`; `fakeRand` + `fakeClock` in test scope. I-2/I-1/M-1/M-2 code-quality follow-up applied.
- **Task 4 — NEW `controller.go` + sliding-window + formula + integer-modulo decision + ADR-0194** (`c05ea6a` + code-quality follow-up). Controller state machine + FAKE-TIME algorithmic-fidelity + 6 Layer A test families per SPEC §14.1 + race tests. ADR-0194 §Decision + §Consequences full bodies anchored. I-1 bucket backing-array growth bug fix + M-1..M-4 test hygiene applied.
- **Task 5 — NEW `admission_control.go` filter struct + `decode_headers.go` gate + reject wire shape** (`526a0cd` + code-quality follow-up). Per-stream filter struct + compile-time interface assertions + 3-gate `DecodeHeaders` order (disabled → RPS-suppression → reject) + 503-empty-body reject per AMEND-7. PD-3 health-check NOT-MODELED disposition confirmed here. Code-quality follow-up applied (RPS test strengthened; stale Task-8 comments removed; EncodeData TODO removed).
- **Task 6 — NEW `encode.go` classification + record discipline** (`ecd6d26` + code-quality follow-up). EncodeHeaders HTTP/gRPC classification (initial PD-5 `:status`-via-header approach; superseded at Task 9a) + EncodeTrailers gRPC-trailers path + record discipline per AMEND-11. Code-quality I1/I2/I3/M1/M2/M3/M4 applied.
- **Task 7 — NEW `fuzz_test.go` 32nd fuzzer + 31 corpus seeds** (`7c206a0` + code-quality follow-up). `FuzzAdmissionControlConfigParse` 31 seeds; clean at 30s with ~2.9M execs; 0 panics. Comment fixes applied.
- **Task 8 — Full filter integration + `doc.go` + boot-registration** (`dd5ab4e`). `doc.go` package doc + `cmd/envoy-go/main.go` boot-registration (alphabetical between `adaptive_concurrency` and `bandwidthlimit`). HTTP filter count: 17 → 18. `grep -c 'httpReg.Register(' cmd/envoy-go/main.go` = 18.
- **Task 9a — Encode-side `ResponseStatus()` framework accessor (root-cause fix; ADR-0196)** (`85e03ba` + code-quality follow-up). PD-5 INVALID; D-hypothesis BROKE. Framework fix: `ResponseStatus() int` added to `EncoderFilterCallbacks` + `FilterChain.encodeResponseStatus` set-once field + `SetEncodeResponseStatus(status)` setter + `encoderCB.ResponseStatus()` accessor + seeded in H1/H2/beginLocalReply before `RunEncodeHeaders`. `encode.go` HTTP classification switched to `f.ecb.ResponseStatus()`. All affected test doubles updated. ADR-0196 full body authored. I-1/I-2/I-3/M-1/M-2/M-3 code-quality follow-up applied. Fixture 0030 (b) cross-side PASSES after fix.
- **Task 9 — Differential fixtures 0030 + 0031 + BackendKind enum + runner switch** (`9622283` + follow-up dead-assertion fix). Depends on Task 9a. Fixture 0030: 4 scenarios; `RequiresReference=true`; cross-side (b) byte-exact. Fixture 0031: boot-reject; `ExpectedBootErrorSubstring() = "cannot be less than 1.0%"`. `fixture.BackendKind HTTPAdmissionControl = 23` enum. Runner switch arm. Follow-up fixed dead `AssertSubject` (never called on cross-side path) → replaced with live `StatsAsserter.AssertStats` with positivity check on `rq_success`; liveness proof via deliberate-break test. Additional reviewer findings (I-1 §7.3 misattribution; I-3 scenario-d 3-counter check; I-4 invalid YAML; M-2/M-3 cleanup) applied.
- **Task 10 — ADR final-state alignment + DECISIONS.md cross-reference audit** (`e212cb8`). 3 NEW ADRs present exactly once; ADR-0197 absent (next-free unconsumed); cross-references clean (all cited ADR-XXXX numbers resolve); stale preamble lines annotated (SUPERSEDED notices inline).
- **Task 11 — BEHAVIOR_CONTRACT.md 4-edit bundle** (`669d349`). Atomic landing per ADR-0052. 4 edits: NEW `### envoy.filters.http.admission_control` subsection + RTDS PARSE-REJECT departure record + stat-table 107→110 extension + per-route cross-reference update.
- **Task 12 — Six-gate phase-done verification + STATE/ROADMAP advance + REVIEW.md** (THIS commit). 6 phase-done gates verified GREEN (all first-run clean this session); STATE.md re-advanced to post-phase-23 state; ROADMAP row 23 flipped `in-progress → done` with per-cell IMPL-done annotation; REVIEW.md authored; PROGRESS Task 12 entry appended.

---

## 6. Known limitations + future-work register

The phase-23 IMPL lands with the following recognized limitations — all are forward-pointers to future phases, NONE blocking phase-done.

**3 phase-23 items:**

1. **RTDS `runtime_key` PARSE-REJECT (departure count 14 → 15)** — DOCUMENTED. Any non-empty `runtime_key` inside the four `Runtime*` wrappers (`enabled` / `aggression` / `sr_threshold` / `max_rejection_probability`) triggers a HCM-parse-time PARSE-REJECT per ADR-0195. Upstream ACCEPTS `runtime_key` (consults the RTDS runtime layer). The static `default_value` is honored. **Future-phase closure surface:** the Runtime/RTDS family phase that brings full RTDS key lookup at runtime for all `Runtime*` wrappers across all phase-23..N filters.

2. **PD-3 health-check arm NOT-MODELED at MVP** — DEFERRED. `DecoderFilterCallbacks` exposes no `IsHealthCheck()` accessor; the `healthCheck()` short-circuit gate from SPEC AMEND-4 is not implemented; AMEND-11 "health-check requests not recorded" is vacuous at MVP. **Future-phase closure surface:** a decoder-side `IsHealthCheck()` primitive as part of a stream-info accessor family; the admission_control filter gate 1 in `decode_headers.go` would then implement the health-check bypass.

3. **ADR-0196 `ResponseStatus()` cross-phase reuse** — FORWARD-POINTER. The `ResponseStatus() int` encode-side accessor was introduced to fix admission_control's classification. It is cross-phase-reusable per ADR-0196 §Consequences. Filters that would benefit: any future encode-side filter that needs the upstream response status code for logic beyond the existing `compressor.go` best-effort `:status`-header bucket. The accessor is available to all filters implementing `StreamEncoderFilter` via `EncoderFilterCallbacks`.

**Cross-phase carry-forwards from earlier phases that this phase did NOT touch:** unchanged (per-phase deferral lists from phase-17 jwt_authn / phase-18.x ext_authz / phase-19.x ext_proc / phase-20 oauth2 / phase-21 adaptive_concurrency / phase-22.x lua carry forward).

---

## 7. Six-gate phase-done verification

Verbatim from Task 12's verification sweep. All 6 gates GREEN.

**Gate A — `go build ./...`:**
```
$ go build ./... 2>&1
(empty)
---BUILD-EXIT: 0---
```

**Gate B — `go vet ./...` + `golangci-lint run`:**
```
$ go vet ./... 2>&1
(empty)
---VET-EXIT: 0---

$ golangci-lint run 2>&1
(empty)
---LINT-EXIT: 0---
```

**Gate C — `go test -race -count=1 ./...`:** Single clean run (exit 0; no retry needed):
```
$ go test -race -count=1 ./... > /tmp/race-full.log 2>&1
RACE-EXIT: 0

$ grep -cE "^FAIL|^--- FAIL" /tmp/race-full.log
0

$ grep "^ok" /tmp/race-full.log | wc -l
62

$ grep "^ok" /tmp/race-full.log | grep -E "admission_control|hcm"
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.102s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.544s
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	1.092s
```
62 packages `ok`; 0 FAIL lines; admission_control + HCM packages race-clean.

**Gate D — `go test -count=1 -timeout=15m ./test/differential/ -run 'TestDifferential'`:**
```
$ go test -count=1 -timeout=15m ./test/differential/ -run 'TestDifferential'
ok  	github.com/esalaine/envoy-go/test/differential	81.440s
---DIFF-EXIT: 0---
```
All 33/33 fixtures PASS (0000-tcp-echo through 0031-http-admission-control-boot-reject) in 81.4s. No 0028 freeTCPPort flake on this run.

**Gate E — `go test -count=1 -run 'FuzzAdmissionControlConfigParse'` (seed corpus) + 30s fuzz run:**
```
$ go test -count=1 -run 'FuzzAdmissionControlConfigParse' ./internal/filter/http/admission_control/ -v 2>&1 | tail -5
    --- PASS: FuzzAdmissionControlConfigParse/seed#29 (0.00s)
    --- PASS: FuzzAdmissionControlConfigParse/seed#30 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	0.004s
---FUZZ-SEED-EXIT: 0---

$ go test -fuzz=FuzzAdmissionControlConfigParse -fuzztime=30s ./internal/filter/http/admission_control/
fuzz: elapsed: 0s, gathering baseline coverage: 0/262 completed
fuzz: elapsed: 2s, gathering baseline coverage: 262/262 completed, now fuzzing with 32 workers
fuzz: elapsed: 3s, execs: 231826 (77267/sec), new interesting: 3 (total: 265)
fuzz: elapsed: 6s, execs: 650362 (139493/sec), new interesting: 5 (total: 267)
fuzz: elapsed: 9s, execs: 1018490 (122708/sec), new interesting: 11 (total: 273)
fuzz: elapsed: 12s, execs: 1323682 (101727/sec), new interesting: 11 (total: 273)
fuzz: elapsed: 15s, execs: 1569208 (81846/sec), new interesting: 13 (total: 275)
fuzz: elapsed: 18s, execs: 1784477 (71752/sec), new interesting: 14 (total: 276)
fuzz: elapsed: 21s, execs: 1966112 (60552/sec), new interesting: 15 (total: 277)
fuzz: elapsed: 24s, execs: 2129752 (54551/sec), new interesting: 15 (total: 277)
fuzz: elapsed: 27s, execs: 2265659 (45292/sec), new interesting: 15 (total: 277)
fuzz: elapsed: 30s, execs: 2416440 (50265/sec), new interesting: 15 (total: 277)
fuzz: elapsed: 31s, execs: 2416440 (0/sec), new interesting: 15 (total: 277)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/admission_control	31.059s
---FUZZ-30S-EXIT: 0---
```
31 seeds gathered at baseline; 2,416,440 execs in 30s; 15 new-interesting; 0 panics; 0 crashers. Total fuzzer count = **32** (`grep -rE '^func Fuzz' --include='*.go' . | wc -l` = 32).

**Gate F — h2spec conformance:**
```
$ go test -v -count=1 ./test/conformance/h2spec/
    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
    h2spec_test.go:187:   [PASS] 3.5. HTTP/2 Connection Preface: 2/2 passed
    h2spec_test.go:187:   [PASS] 4.1. Frame Format: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.2. Frame Size: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.3. Header Compression and Decompression: 3/3 passed
    h2spec_test.go:187:   [PASS] 5.1. Stream States: 13/13 passed
    h2spec_test.go:187:   [PASS] 5.1.1. Stream Identifiers: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.1.2. Stream Concurrency: 1/1 passed
    h2spec_test.go:187:   [PASS] 5.3.1. Stream Dependencies: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.4.1. Connection Error Handling: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.5. Extending HTTP/2: 2/2 passed
    h2spec_test.go:187:   [PASS] 7. Error Codes: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1. HTTP Request/Response Exchange: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2. HTTP Header Fields: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2.1. Pseudo-Header Fields: 4/4 passed
    h2spec_test.go:187:   [PASS] 8.1.2.2. Connection-Specific Header Fields: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
    h2spec_test.go:187:   [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.2. Server Push: 1/1 passed
--- PASS: TestH2Spec (2.30s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.384s
---H2SPEC-EXIT: 0---
```
53/53 PASS at ADR-0051 pin. Phase-23 touched no H2 codec path; the PASS confirms zero regression. Note: `go test -v -count=1 ./test/conformance/h2spec/` is the substantive equivalent of `make test-h2spec` per the phase-20/21 PROGRESS Gate-F precedent.

**ADR-0197 absence verification:**
```
$ grep -cE '^## ADR-0194' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0195' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0196' docs/envoy-go/DECISIONS.md
1
$ grep -cE '^## ADR-0197' docs/envoy-go/DECISIONS.md
0
```
ADR-0194 + ADR-0195 + ADR-0196 present exactly once; ADR-0197 absent (next-free unconsumed).

---

## 8. Parent-rollup status

**Phase 23 (single-row per ADR-0045) is CLOSED at this Task 12 commit.** Per SPEC §3.4 (the SPEC author's single-row settlement; reconfirmed at PLAN time per ADR-0045): row `23 http-filter-admission-control` flips `in-progress → done` AT THIS COMMIT (date `2026-05-22`). No parent-rollup (no ADR-0045 split; row stays single-row at ROADMAP line 69).

The §9 HTTP-filters family now has 16 family-rows landed (phases 7.1 / 9 / 10 / 11 / 12 / 13 / 14 / 15 / 16 / 17 / 18 / 19 / 20 / 21 / 22 / 23). **2 §9 rows remain on the roster post-phase-23**: `wasm` / `global rate limit` per the §9 family closure trail. The next §9 family-row after row 23 closes is to be identified by the user at the next session's cold-start.

---

## 9. Lessons learned

**D-hypothesis breaking is a real risk for encode-side classification.** Phase-23's PD-5 assumed the encode-side HTTP response status was available via `headers.Get(":status")` — a natural extrapolation from the phase-14 compressor. But the compressor read `:status` only as a best-effort optimization (silent no-op when absent), while admission_control needs it for CORE classification. The gap surfaced immediately at the first differential bring-up of fixture 0030. **Lesson:** any encode-side filter that needs the HTTP response status for core logic (not optimization) MUST use a framework accessor seeded at HCM dispatch time, not attempt to read `:status` from the header map. ADR-0196 establishes this pattern; future encode-side filter authors should check ADR-0196 before designing their encode-side status access.

**Dead assertions in cross-side fixtures are invisible without deliberate liveness proofs.** The `AssertSubject` interface was implemented and tests were written, but the cross-side runner path never calls `SubjectAsserter`. Without a deliberate-break test, the dead assertion could have masked coverage gaps indefinitely. **Lesson:** any new `StatsAsserter`, `SubjectAsserter`, etc. implementation in a differential fixture MUST be proven live via a deliberate-break test (set an assertion to the wrong value; confirm the test fails; restore). This was the phase-23 Task 9 follow-up's load-bearing finding.

**The FIRST ADR-0125-skip since phase-22.** Phase-23's REUSE-by-absence (no `AdmissionControlPerRoute` proto message in v1.32.4 or v1.37.x) is the first §9 family-row to skip ADR-0125 amendment since phase-22.3 amended the roster to 9. **Lesson:** the canonical-pattern roster is stable at 9 entries post-phase-22.3; the REUSE-by-absence discipline from phases 18-21 is back in force.

---

## 10. Forward-pointers carried into next phase

The next-phase inheritance set per the Task 12 STATE.md advance:

**3 phase-23-emergent forward-pointers (per §6 above):**
1. RTDS `runtime_key` PARSE-REJECT departure (future-phase closure: Runtime/RTDS family phase for all `Runtime*` wrapper fields).
2. PD-3 health-check arm NOT-MODELED (future-phase closure: decoder-side `IsHealthCheck()` primitive as part of a stream-info accessor family).
3. ADR-0196 `ResponseStatus()` cross-phase reuse (available for any future encode-side filter needing upstream response status for core logic).

**Cross-phase carry-forwards from earlier phases that this phase did NOT touch:** unchanged.

**STATE.md post-Task-12 disposition:** `active-phase: to-be-determined-at-next-session`; `lifecycle-state: phase 23 IMPL done; awaiting next-phase identification`; `next-skill: superpowers:brainstorming` for the next-phase initial step; `last-commit: <TBD>` placeholder for post-squash STATE SHA-fill; `next-free ADR: ADR-0197` (ADR-0196 consumed; D-hypothesis BROKE).

**§9 family closure trail:** 16 family-rows landed. 2 §9 rows remain on the roster: `wasm` / `global rate limit`.

---

## 11. Sign-off

Phase 23 is **APPROVED for master squash-merge per project memory `feedback_git_worktrees.md`** + ADR-0003 worktree-isolation discipline + ADR-0005 §Decision 4 worktree-merge discipline. All 6 phase-done gates GREEN at this Task 12 HEAD (first-run clean; no flakes this session); all 16 SPEC §15 acceptance items verified (14 GREEN + 2 GREEN-WITH-NOTED-DEVIATION: item 13 — 3 ADRs consumed vs 2 anticipated, and item 15 — next-free ADR-0197 not ADR-0196 — both deviations documented with full rationale in §3); 3 ADR §Decision-touchpoints cleanly anchored at their per-Task Lands-in-Tasks per ADR-0044 (2 anticipated NEW ADRs §Decision + §Consequences full bodies — ADR-0194 + ADR-0195 — plus 1 unanticipated NEW ADR from D-hypothesis breaking — ADR-0196); **NO ADR-0125 amendment** (FIRST §9 family-row to REUSE since phase-22's roster amendment — REUSE-by-absence per SPEC §5.4); **D-hypothesis BROKE** — ADR-0044 escape-valve fired at Task 9a (PD-5 `:status`-via-header assumption INVALID; `ResponseStatus()` encode-side framework accessor introduced per ADR-0196; phase-23 is NOT zero-new-framework-primitives: ONE new encode-side primitive; next-free ADR advances to ADR-0197); **PD-1..PD-10 dispositions** all confirmed (PD-3 health-check NOT-MODELED deferral documented; PD-5 superseded by ADR-0196); 32 fuzzers + 33 differential fixtures GREEN; h2spec 53/53 at ADR-0051 pin; BEHAVIOR_CONTRACT 4-edit bundle landed at Task 11 per ADR-0052 atomic landing; **ROADMAP row 23 flipped `in-progress → done` AT THIS COMMIT** (date `2026-05-22`) per the SPEC author's single-row settlement at SPEC commit `a64ee71`; STATE.md re-advanced; **3 IMPL-time deviations from PLAN recorded** (ADR-0196 framework primitive / dead-assertion fix in fixture 0030 / PD-3 health-check NOT-MODELED).

The squash-merge + STATE SHA-fill follow-up + push-to-origin are the user's manual steps after this Task 12 commit lands (per the phase-09..22 squash-merge convention).

**Summary stats:** 12 tasks (Tasks 1-12) + Task 9a = 13 task-landing commits + their SHA-fill follow-ups at worktree HEAD; this Task 12 REVIEW + STATE/ROADMAP + PROGRESS-append is the final commit. 3 NEW ADR §Decision + §Consequences full bodies (ADR-0194 + ADR-0195 + ADR-0196; 2 anticipated + 1 unanticipated from D-hypothesis breaking). 32 fuzzers (31 from phase-22.3 + 1 NEW `FuzzAdmissionControlConfigParse`). 33 differential fixtures (31 from phase-22.3 + 2 NEW `0030-http-admission-control` + `0031-http-admission-control-boot-reject`). 6 phase-done gates GREEN (all first-run clean this session). 15 envoy-go-strict departures (was 14; +1 RTDS `runtime_key` PARSE-REJECT per ADR-0195). 18 HTTP filters wired (was 17; +1 `admission_control`). 110 stats (was 107; +3 counters per AMEND-3). ONE new encode-side framework primitive (ADR-0196 `ResponseStatus()`; REVISES the ZERO-new-primitives plan claim). **FIRST ADR-0125-skip since phase-22's roster amendment** (canonical-per-route roster STAYS 9).

**End of phase 23 review. The next session is the phase-done squash-merge + STATE SHA-fill + push-to-origin follow-up per project memory `feedback_git_worktrees.md` + `feedback_push_to_origin.md`.**
