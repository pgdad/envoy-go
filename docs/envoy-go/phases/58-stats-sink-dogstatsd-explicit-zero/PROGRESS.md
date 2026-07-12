# Phase 58 PROGRESS — `dog_statsd` explicit-`max_bytes_per_datagram: 0` parity fix (ADR-0276; row 58 flips `done` at the IMPL six-gate — the SOLE leg)

> Scaffolded at the PLAN session (2026-07-12, worktree `.worktrees/phase-58-plan`, branch `phase-58-plan`). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`), fills the baseline block at Task 1, logs each task + the Task-2 `-count=1` break outcome here, and closes it at Task 6.

## Task checklist (mirrors PLAN.md)

- [x] Task 1 — Baselines into this file + the final ADR-0045 single-flat-row re-check
- [x] Task 2 — The reject arm in `parseDogStatsdSinkConfig` + the test CONVERT (delete accept-test, add reject row, keep Absent/512) + 3 `bootstrap.go` doc fixes + the `-count=1` liveness break [TDD; CONTROLLER-EXECUTED break]
- [x] Task 3 — `FuzzDogStatsdSinkConfigParse` explicit-0 SEED (fuzzers 54 → 54; no new fuzzer)
- [x] Task 4 — The four `BEHAVIOR_CONTRACT.md` edits (rejects + batching + graphite NOTE flip + consumption summary)
- [x] Task 5 — +0 stat surface + full 103-dir differential + the six-gate on the frozen HEAD
- [x] Task 6 — ADR-0276 body + STATE/ROADMAP (row 58 `done`) + sentinel check-(2) re-run + PROGRESS close + router roll

## D-question dispositions (all DISPOSED at the SPEC — SPEC-58 §1.2)

- **D-DZ-REJECTMSG → PINNED** via a live probe (SPEC-58 §11): the reference boot-rejects an explicit `max_bytes_per_datagram: 0` on a `DogStatsdSink` (`Proto constraint validation failed (DogStatsdSinkValidationError.MaxBytesPerDatagram: value must be greater than 0)`, exit 1); envoy-go emits its OWN substring `dog_statsd max_bytes_per_datagram must be greater than 0` (ADR-0080-distinct from graphite's). Two controls isolate the reject to the explicit zero (absent + 512 both validate OK).
- **D-DZ-FIXTURE → NO new fixture** (a boot-reject never reaches the differential runner; fixtures stay 103).
- **D-DZ-TESTROWS → CONVERT-not-ADD** (`TestDogStatsdSink_AcceptMaxBytesPerDatagramZero` asserts the OLD accept behavior; DELETE it, add a reject row to `TestDogStatsdSink_Rejects`; keep `...Absent` + `...Accept512`).
- **D-DZ-FUZZSEED → a SEED to the existing fuzzer** (fuzzers stay 54; `reference_fuzzer_count_docs_drift` — re-count after).
- **D-DZ-DOCSHAPE → the full edit-site roster** (SPEC-58 §12): 3 `bootstrap.go` doc comments + 4 `BEHAVIOR_CONTRACT.md` sites.

## Baselines (filled at IMPL Task 1 — verbatim command outputs)

- `go build ./...`: BUILD_OK
- fixtures (`ls -d test/fixtures/*/ | wc -l`): 103 ✓ (tail `0101-stats-sink-graphite`)
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): 54 ✓
- BackendKind tail (`grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go`): 606	H2GoawayResponder BackendKind = 38 ✓
- `go mod tidy -diff`: (empty) ✓
- dog_statsd reject substring pre-check (`grep -n 'max_bytes_per_datagram must be greater than 0' internal/bootstrap/bootstrap.go`): 751	graphite_statsd only ✓ (dog_statsd not yet added)
- stat surface: **1201** (docs-verified; the registration guards enforce +0 — no counting command)
- DECISIONS tail (`grep -oE '^## ADR-[0-9]+' docs/envoy-go/DECISIONS.md | tail -1`): ## ADR-0275 ✓ (next-free ADR-0276)

## ADR-0045 split disposition (re-confirm at Task 1)

SINGLE FLAT ROW — 6 tasks, margin ~9 under the `~15` ceiling; escape-valve UNCONSUMED. No second subsystem; the graphite arm this row mirrors landed at phase 57.

## Anticipated exit counts (from SPEC-58 §14)

stat surface **1201 (+0)** · fixtures **103 (+0)** · fuzzers **54 (+0)** · BackendKind **38 (+0)** · DECISIONS tail **ADR-0276** (next-free ADR-0277) · **+0 go.mod modules, +0 packages**.

## Task log (filled per task at IMPL)

- **Task 1** (`91e1ba12`): baselines filled above + the ADR-0045 single-flat-row re-check confirmed.
- **Task 2** (`7445c5bd`): the reject arm + the test convert (deleted `…AcceptMaxBytesPerDatagramZero`, added the `explicit_max_bytes_per_datagram_zero` reject row; `…Absent`/`…Accept512` kept) + 3 `bootstrap.go` doc fixes; the `-count=1` liveness break controller-executed — see Break log.
- **Task 3** (`339e65ea`): the `FuzzDogStatsdSinkConfigParse` explicit-0 seed (+ the in-passing 512-comment fix); fuzzers stay 54.
- **Task 4** (`6b6e2177`): the four `BEHAVIOR_CONTRACT.md` edits (rejects [+ "three"→"four" fold] + batching split + graphite NOTE flip + consumption summary EXCEPT→INCLUDING).
- **Task 5** (`07a8261e`): +0 stat surface (four registration guards PASS) + the full 103-dir differential (`ok … 327.702s`, exit 0, byte-stable) + the six-gate (gofmt/golangci-lint/vet/build/`go mod tidy -diff` all clean).
- **Task 6** (this docs commit): ADR-0276 full entry (§Decision/§Consequences on the SPEC §13 §Context) + STATE/ROADMAP (row 58 `done`; deferred-list rolls dog_statsd OUT) + sentinel check-(2) re-run (ONE live match) + PROGRESS close + router roll.

## Break log (filled at Task 2 Step 5 — the observed failure line, per `reference_deliberate_break_wrong_assertion`)

- (liveness) reject-arm removal (`-count=1`): OBSERVED `bootstrap_test.go:2504: Load: want error for explicit_max_bytes_per_datagram_zero, got nil` — the converted reject row's `Fatalf` on the `err == nil` precondition; subtest name `TestDogStatsdSink_Rejects/explicit_max_bytes_per_datagram_zero` CONFIRMED (not an unrelated abort, per `reference_deliberate_break_wrong_assertion`). Reverted via `git restore internal/bootstrap/bootstrap.go`; `go test ./internal/bootstrap/ -count=1` re-verified GREEN; tree clean; branch undetached.

## Final counts (Task 6 — re-run baseline commands on the frozen HEAD)

- `ls -d test/fixtures/*/ | wc -l`: **103** ✓ (tail `0101-stats-sink-graphite`)
- `grep -rn '^func Fuzz' --include='*.go' . | wc -l`: **54** ✓
- `go mod tidy -diff`: EMPTY ✓
- `go build ./...`: BUILD_OK ✓
- stat surface: **1201** (+0, enforced by the four sink registration guards)
- BackendKind tail: **38** (+0)
- DECISIONS tail: **ADR-0276** (next-free ADR-0277)
- sentinel check-(2): EXACTLY ONE live "candidates:" match (OTLP-metrics + tracing quartet; dog_statsd rolled OUT) ⇒ the sentinel does NOT fire.

**Anticipated vs actual (SPEC-58 §14): MATCH on every count** — stat surface 1201 (+0) · fixtures 103 (+0) · fuzzers 54 (+0) · BackendKind 38 (+0) · DECISIONS tail ADR-0276 · +0 packages/modules. Row 58 flips `in-progress` → `done` at the Task-6 commit (the SOLE leg, ADR-0106). The Observability family STAYS OPEN.
