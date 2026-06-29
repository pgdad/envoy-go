# Phase 47.2a IMPL — Progress: `report_counters_as_deltas=true` (the per-sink last-flush Counter DELTA state)

**SPEC:** `docs/envoy-go/phases/47-stats-sink-metrics-service/SPEC-47.2a.md`
**PLAN:** `docs/envoy-go/phases/47-stats-sink-metrics-service/PLAN-47.2a.md`
**Worktree branch:** `phase-47.2a-report-counters-as-deltas` (off master, isolated worktree)

This leg is the **FIRST of the two FINAL sub-legs** of the FOURTH Observability-family row. It lifts the
`internal/bootstrap/bootstrap.go:446` `report_counters_as_deltas:true` strict-reject (reference-parity-accept),
adds a `StatsSinkConfig.ReportCountersAsDeltas bool`, a per-sink `deltaState` (`map[string]uint64`) delta
transform in a NEW `internal/statssink/delta.go` applied in `MetricsServiceSink.Submit` (Counter-only;
Gauges absolute; first=absolute; idle=0; no shared-slice mutation), threads the bool through
`cmd/envoy-go/main.go`, extends the driver-owned `test/helpers/metricsservice` receiver with a per-name
delta-SUM surface (`FamilySum`), and proves it with the `0090-stats-sink-metrics-service-deltas`
cross-side EXACT delta-SUM differential. It **ANCHORS ADR-0263** (§Decision/§Consequences body lands
atomically at the completion task per ADR-0044). **ROADMAP row 47 (`stats-sink-metrics-service`) STAYS
`in-progress`** — 47.2a is NOT the final sub-leg (row 47 flips `done` at the 47.2b IMPL per ADR-0106 +
`reference_roadmap_split_phase_row_done`); the Observability family STAYS OPEN.

---

## D-MS-SPLIT-2 — FINAL ADR-0045 re-check: NO sub-split (§3.0 / Task 1 Step 2)

The realized 47.2a scope is ~150 LoC of new logic: bootstrap ~4 lines (lift one reject arm + one bool field +
set-on-append); the new `delta.go` ~45 lines (`deltaState` + `apply`); `sink.go` ~6 lines (a `delta *deltaState`
field + a bool ctor param + the `Submit` branch); `main.go` ~1 line (the thread); the receiver ~12 lines (a
parallel `sums map[string]float64` + `FamilySum` + the `StreamMetrics`/`Reset` updates); the `0090` fixture a
mechanical clone of the authoritative `0089/driver/driver.go` with the `value==K` assertion replaced by the
delta-`FamilySum`==K invariant. This is a single focused change — a 47.2a-i/47.2a-ii sub-split would be
over-decomposition (BRAINSTORM-47.2 §1.4 expectation; SPEC §3.0). **D-MS-SPLIT-2 RESOLVED: NO further split.**

---

## Baselines (verbatim outputs — recorded 2026-06-29, this worktree HEAD `75c66cd7`)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/*/ | wc -l                                   # expect 91 (tail 0089-stats-sink-metrics-service)
91
$ ls -d test/fixtures/*/ | tail -1
test/fixtures/0089-stats-sink-metrics-service/

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l                 # expect 50
50

$ grep -n 'H2GoawayResponder BackendKind' test/differential/fixture/fixture.go   # BackendKind tail = 38
606:	H2GoawayResponder BackendKind = 38

$ grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1    # tail ADR-0262 (next-free ADR-0263)
## ADR-0262

$ grep 'prometheus/client_model' go.mod                            # already direct (landed at 47.1)
	github.com/prometheus/client_model v0.6.1

$ go test ./internal/statssink/ ./internal/bootstrap/ ./test/helpers/metricsservice/ -count=1
ok  	github.com/esalaine/envoy-go/internal/statssink
ok  	github.com/esalaine/envoy-go/internal/bootstrap
ok  	github.com/esalaine/envoy-go/test/helpers/metricsservice
```

**Baseline summary:** stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **91** /
fuzzers **50** / BackendKind **38** / DECISIONS tail **ADR-0262** (next-free **ADR-0263**) /
`client_model v0.6.1` already direct / build OK / the three touched packages green.

**Anticipated at IMPL exit:** stat **1200 UNCHANGED** (+0) / fixtures **92** (`0090-stats-sink-metrics-service-deltas`) /
fuzzers **50** / BackendKind **38** / DECISIONS **ADR-0263** / NO new package / NO new go.mod module.

---

## Task ledger

- [x] **Task 1** — Baselines + PROGRESS scaffold + FINAL ADR-0045 re-check (D-MS-SPLIT-2: no sub-split). (controller)
- [ ] **Task 2** — Lift the `report_counters_as_deltas:true` reject + add `StatsSinkConfig.ReportCountersAsDeltas`.
- [ ] **Task 3** — The per-sink Counter delta transform (`internal/statssink/delta.go`).
- [ ] **Task 4** — Thread the bool into `MetricsServiceSink` (apply in `Submit`).
- [ ] **Task 5** — Boot wiring (`cmd/envoy-go/main.go`).
- [ ] **Task 6** — The receiver delta-SUM surface (`FamilySum`).
- [ ] **Task 7** — The `0090-stats-sink-metrics-service-deltas` cross-side EXACT fixture (+ register in runner_test.go).
- [ ] **Task 8** — `0090` deliberate breaks + flake-soak + full-package `-race` (controller).

### Task 8 root-cause finding (deliberate-break sensitivity — FIXED at Task 7b)

The first deliberate-break attempt (break (a): emit absolute instead of delta) **PASSED instead of failing** — the
`0090` differential was structurally blind to it. Root cause (confirmed via `FIXTURE_0090_DUMP`): the 7 requests
all complete within a single 500ms flush window, so the FIRST post-request flush emits `delta = 7−0 = 7`
(correct) which is IDENTICAL to the broken `absolute = cur = 7`; the convergence poll (`FamilySum==7`) snapshots
at the instant the SUM first reaches 7, before any second flush could reveal the divergence (correct idle flush
adds 0; broken absolute re-adds the cumulative). The delta/absolute distinction only manifests across multiple
flush windows OR in post-convergence stability.

**Fix (Task 7b):** a **post-convergence stability barrier** — after the per-name delta-SUM reaches K, observe ≥2
further flushes (a release barrier on the flush ticker via a new `metricsservice.Server.Messages()` counter) and
confirm the SUM is STILL K. Under deltas the now-idle counters emit a 0 delta so the SUM stays K; an
absolute/un-latched sink re-adds the cumulative every flush so the SUM overshoots K → assertion fails. This
catches breaks (a) emit-absolute and (b) skip-the-latch DETERMINISTICALLY, independent of request timing. The
production `delta.go` was NOT changed — only the test fixture's break-sensitivity. (See memory
`reference_delta_sink_differential_stability_barrier`.)
- [ ] **Task 9** — Full 92-dir differential + six-gate + ADR-0263 body + BEHAVIOR_CONTRACT + STATE/ROADMAP.
