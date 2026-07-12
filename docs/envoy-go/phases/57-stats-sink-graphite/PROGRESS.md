# Phase 57 PROGRESS — `graphite_statsd` stats sink (ADR-0275; row 57 flips `done` at the IMPL six-gate)

> Scaffolded at the PLAN session (2026-07-11, worktree `.worktrees/phase-57-plan`, branch `phase-57-stats-sink-graphite-plan` — the phase-55 bare-name convention). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`), fills the baseline block at Task 1, logs each task + the Task-9 break outcomes here, and closes it at Task 11.

## Task checklist (mirrors PLAN.md)

- [ ] Task 1 — Baselines into this file + the final ADR-0045 re-check
- [ ] Task 2 — Graphite parse arm: type URL + dispatch + `parseGraphiteStatsdSinkConfig` + `GraphiteStatsdSinkConfig` + FOUR-sink sibling-reject + explicit-zero reject + stale dog_statsd comment fix [TDD]
- [ ] Task 3 — D-GR-BATCHSHARE hoist: shared `appendBatchLine`/`flushBatch` in `udp.go`; dog_statsd tests byte-untouched [refactor]
- [ ] Task 4 — `graphite.go`: `GraphiteStatsdSink` + `graphiteTagSuffix` + unit tests + `TestNoNewStat_GraphiteRegistrationGuard` [TDD]
- [ ] Task 5 — `main.go` fourth build loop + gate clause
- [ ] Task 6 — `FuzzGraphiteStatsdSinkConfigParse` (fuzzers 53 → 54)
- [ ] Task 7 — `statsdrecv` additive `;k=v`-in-name graphite-tag extension [TDD]
- [ ] Task 8 — `0101-stats-sink-graphite` differential (fixtures 102 → 103) + runner registration + live run
- [ ] Task 9 — Deliberate breaks (a, b1, b2, c1, c2) + non-break (d) + 20/20 flake + full-package race [CONTROLLER-EXECUTED]
- [ ] Task 10 — +0 stat surface + full 103-dir differential + six-gate
- [ ] Task 11 — ADR-0275 body + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 57 `done`) + PROGRESS close + router roll

## D-question dispositions (settled at PLAN — see PLAN.md's D-question block)

- **D-GR-BATCHSHARE → GENERALIZE** (shared free functions in `udp.go`; zero direct test call sites on the old methods, so the dog_statsd tests stay byte-untouched — the SPEC's bias condition holds).
- **D-GR-SPLIT → NO sub-split** (11 tasks, margin 4 under the ADR-0045 `~15` ceiling; escape-valve UNCONSUMED — re-confirm at Task 1).
- **Break-(b) precision → the SPEC §8.1 break-(b) shape is VACUOUS** (`statsdrecv` parses a dog_statsd-formatted line into the identical `(name, tags)` buckets); replaced by (b1) tag-drop → poll-timeout firing and (b2) the UnparsedCount-isolating malformed-line break.

## Baselines (filled at IMPL Task 1 — verbatim command outputs)

- `go build ./...`: _pending_
- fixtures (`ls -d test/fixtures/*/ | wc -l`): _pending_ (expect 102, tail `0100-http-tap-bodies`)
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): _pending_ (expect 53)
- BackendKind tail: _pending_ (expect 38 `H2GoawayResponder`)
- `go mod tidy -diff`: _pending_ (expect EMPTY)
- stat surface: **1201** (docs-verified at SPEC/PLAN; no counting command — the registration guards enforce +0)
- DECISIONS tail: **ADR-0274** (next-free **ADR-0275**)

## Anticipated exit counts (from SPEC-57 §14)

stat surface **1201 (+0)** · fixtures **103** (`0101-stats-sink-graphite`) · fuzzers **54** (`FuzzGraphiteStatsdSinkConfigParse`) · BackendKind **38 (+0)** · DECISIONS tail **ADR-0275** (next-free ADR-0276) · **+0 go.mod modules, +0 packages**.

## Task log (filled per task at IMPL)

_pending_

## Break log (filled at Task 9 — the observed failure line per break, per `reference_deliberate_break_wrong_assertion`)

- (a) delta break: _pending_
- (b1) tag-drop break: _pending_
- (b2) UnparsedCount-isolating break: _pending_
- (c1) never-batch break: _pending_
- (c2) infinite-cap break: _pending_
- (d) tag-order-reversal NON-break (must PASS the 0101 selector): _pending_
- 20/20 flake gate: _pending_
- full-package `-race` (`internal/statssink` + `test/helpers/statsdrecv`): _pending_
