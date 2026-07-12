# Phase 58 PROGRESS — `dog_statsd` explicit-`max_bytes_per_datagram: 0` parity fix (ADR-0276; row 58 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-12, worktree `.worktrees/phase-58-plan`, branch `phase-58-plan`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`), fills the baseline block at Task 1, logs each task + the Task-2 `-count=1` break outcome here, and closes it at Task 6.

## Task checklist (mirrors PLAN.md)

- [ ] Task 1 — Baselines into this file + the final ADR-0045 single-flat-row re-check
- [ ] Task 2 — The reject arm in `parseDogStatsdSinkConfig` + the test CONVERT (delete accept-test, add reject row, keep Absent/512) + 3 `bootstrap.go` doc fixes + the `-count=1` liveness break [TDD; CONTROLLER-EXECUTED break]
- [ ] Task 3 — `FuzzDogStatsdSinkConfigParse` explicit-0 SEED (fuzzers 54 → 54; no new fuzzer)
- [ ] Task 4 — The four `BEHAVIOR_CONTRACT.md` edits (rejects + batching + graphite NOTE flip + consumption summary)
- [ ] Task 5 — +0 stat surface + full 103-dir differential + the six-gate on the frozen HEAD
- [ ] Task 6 — ADR-0276 body + STATE/ROADMAP (row 58 `done`) + sentinel check-(2) re-run + PROGRESS close + router roll

## D-question dispositions (all DISPOSED at the SPEC — SPEC-58 §1.2)

- **D-DZ-REJECTMSG → PINNED** via a live probe (SPEC-58 §11): the reference boot-rejects an explicit `max_bytes_per_datagram: 0` on a `DogStatsdSink` (`Proto constraint validation failed (DogStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0)`, exit 1); envoy-go emits its OWN substring `dog_statsd max_bytes_per_datagram must be greater than 0` (ADR-0080-distinct from graphite's). Two controls isolate the reject to the explicit zero (absent + 512 both validate OK).
- **D-DZ-FIXTURE → NO new fixture** (a boot-reject never reaches the differential runner; fixtures stay 103).
- **D-DZ-TESTROWS → CONVERT-not-ADD** (`TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` asserts the OLD accept behavior; DELETE it, add a reject row to `TestDogStatsdSink_Rejects`; keep `...Absent` + `...Accept512`).
- **D-DZ-FUZZSEED → a SEED to the existing fuzzer** (fuzzers stay 54; `reference_fuzzer_count_docs_drift` — re-count after).
- **D-DZ-DOCSHAPE → the full edit-site roster** (SPEC-58 §12): 3 `bootstrap.go` doc comments + 4 `BEHAVIOR_CONTRACT.md` sites.

## Baselines (filled at IMPL Task 1 — verbatim command outputs)

- `go build ./...`: _(fill at Task 1)_
- fixtures (`ls -d test/fixtures/*/ | wc -l`): _(expect 103, tail `0101-stats-sink-graphite`)_
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): _(expect 54)_
- BackendKind tail (`grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go`): _(expect 38)_
- `go mod tidy -diff`: _(expect EMPTY)_
- dog_statsd reject substring pre-check (`grep -n 'max_bytes_per_datagram must be greater than 0' internal/bootstrap/bootstrap.go`): _(expect ONE hit — graphite :751 — dog_statsd not yet added)_
- stat surface: **1201** (docs-verified; the registration guards enforce +0 — no counting command)
- DECISIONS tail (`grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1`): _(expect ## ADR-0275; next-free ADR-0276)_

## ADR-0045 split disposition (re-confirm at Task 1)

SINGLE FLAT ROW — 6 tasks, margin ~9 under the `~15` ceiling; escape-valve UNCONSUMED. No second subsystem; the graphite arm this row mirrors landed at phase 57.

## Anticipated exit counts (from SPEC-58 §14)

stat surface **1201 (+0)** · fixtures **103 (+0)** · fuzzers **54 (+0)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0276** (next-free ADR-0277) · **+0 go.mod modules, +0 packages**.

## Task log (filled per task at IMPL)

- **Task 1** (`<sha>`): baselines filled above + the ADR-0045 single-flat-row re-check confirmed.
- **Task 2** (`<sha>`): the reject arm + the test convert + 3 doc fixes + the `-count=1` liveness break — _(record the observed break failure line)_.
- **Task 3** (`<sha>`): the `FuzzDogStatsdSinkConfigParse` explicit-0 seed; fuzzers stay 54.
- **Task 4** (`<sha>`): the four `BEHAVIOR_CONTRACT.md` edits.
- **Task 5** (`<sha>`): +0 stat surface + the full 103-dir differential + the six-gate.
- **Task 6** (`<sha>`): ADR-0276 full entry + STATE/ROADMAP (row 58 `done`) + sentinel check-(2) re-run + PROGRESS close + router roll.

## Break log (filled at Task 2 Step 5 — the observed failure line, per `reference_deliberate_break_wrong_assertion`)

- (liveness) reject-arm removal (`-count=1`): _(expect `Load: want error for explicit_max_bytes_per_datagram_zero, got nil` — the converted reject row's `Fatalf` on the `err == nil` precondition; CONFIRM this exact line fires, not an unrelated abort)_. Reverted via `git restore`; tree clean; branch undetached.

## Final counts (Task 6 — re-run baseline commands on the frozen HEAD)

- `ls -d test/fixtures/*/ | wc -l`: _(expect 103, tail `0101-stats-sink-graphite`)_
- `grep -rn '^func Fuzz' --include='*.go' . | wc -l`: _(expect 54)_
- `go mod tidy -diff`: _(expect EMPTY)_
- `go build ./...`: _(expect BUILD_OK)_
- stat surface: **1201** (+0, enforced by the four sink registration guards)
- BackendKind tail: **38** (+0)
- DECISIONS tail: **ADR-0276** (next-free ADR-0277)
- sentinel check-(2): EXACTLY ONE live "candidates:" match (OTLP-metrics + tracing quartet; dog_statsd rolled OUT) ⇒ the sentinel does NOT fire.

**Anticipated vs actual (SPEC-58 §14): MATCH on every count** _(confirm at Task 6)_. Row 58 flips `in-progress` → `done` at the Task-6 commit (the SOLE leg, ADR-0106). The Observability family STAYS OPEN.
