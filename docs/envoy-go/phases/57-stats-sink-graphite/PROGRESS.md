# Phase 57 PROGRESS — `graphite_statsd` stats sink (ADR-0275; row 57 flips `done` at the IMPL six-gate)

> Scaffolded at the PLAN session (2026-07-11, worktree `.worktrees/phase-57-plan`, branch `phase-57-stats-sink-graphite-plan` — the phase-55 bare-name convention). The IMPL session executes `PLAN.md` task-by-task (subagent-driven per `feedback_execution_style`), fills the baseline block at Task 1, logs each task + the Task-9 break outcomes here, and closes it at Task 11.

## Task checklist (mirrors PLAN.md)

- [x] Task 1 — Baselines into this file + the final ADR-0045 re-check
- [x] Task 2 — Graphite parse arm: type URL + dispatch + `parseGraphiteStatsdSinkConfig` + `GraphiteStatsdSinkConfig` + FOUR-sink sibling-reject + explicit-zero reject + stale dog_statsd comment fix [TDD]
- [x] Task 3 — D-GR-BATCHSHARE hoist: shared `appendBatchLine`/`flushBatch` in `udp.go`; dog_statsd tests byte-untouched [refactor]
- [x] Task 4 — `graphite.go`: `GraphiteStatsdSink` + `graphiteTagSuffix` + unit tests + `TestNoNewStat_GraphiteRegistrationGuard` [TDD]
- [x] Task 5 — `main.go` fourth build loop + gate clause
- [x] Task 6 — `FuzzGraphiteStatsdSinkConfigParse` (fuzzers 53 → 54)
- [x] Task 7 — `statsdrecv` additive `;k=v`-in-name graphite-tag extension [TDD]
- [x] Task 8 — `0101-stats-sink-graphite` differential (fixtures 102 → 103) + runner registration + live run
- [x] Task 9 — Deliberate breaks (a, b1, b2, c1, c2) + non-break (d) + 20/20 flake + full-package race [CONTROLLER-EXECUTED]
- [x] Task 10 — +0 stat surface + full 103-dir differential + six-gate
- [x] Task 11 — ADR-0275 body + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 57 `done`) + PROGRESS close + router roll

## D-question dispositions (settled at PLAN — see PLAN.md's D-question block)

- **D-GR-BATCHSHARE → GENERALIZE** (shared free functions in `udp.go`; zero direct test call sites on the old methods, so the dog_statsd tests stay byte-untouched — the SPEC's bias condition holds).
- **D-GR-SPLIT → NO sub-split** (11 tasks, margin 4 under the ADR-0045 `~15` ceiling; escape-valve UNCONSUMED — re-confirm at Task 1).
- **Break-(b) precision → the SPEC §8.1 break-(b) shape is VACUOUS** (`statsdrecv` parses a dog_statsd-formatted line into the identical `(name, tags)` buckets); replaced by (b1) tag-drop → poll-timeout firing and (b2) the UnparsedCount-isolating malformed-line break.

## Baselines (filled at IMPL Task 1 — verbatim command outputs)

- `go build ./...`: BUILD_OK
- fixtures (`ls -d test/fixtures/*/ | wc -l`): 102 (tail `0100-http-tap-bodies`)
- fuzzers (`grep -rn '^func Fuzz' --include='*.go' . | wc -l`): 53
- BackendKind tail: `606:	H2GoawayResponder BackendKind = 38` (confirmed at tail)
- `go mod tidy -diff`: (EMPTY)
- stat surface: **1201** (docs-verified at SPEC/PLAN; no counting command — the registration guards enforce +0)
- DECISIONS tail: **ADR-0274** (line 16571; next-free **ADR-0275** confirmed via `grep -n '^## ADR-02' docs/envoy-go/DECISIONS.md | tail -2`)

## Anticipated exit counts (from SPEC-57 §14)

stat surface **1201 (+0)** · fixtures **103** (`0101-stats-sink-graphite`) · fuzzers **54** (`FuzzGraphiteStatsdSinkConfigParse`) · BackendKind **38 (+0)** · DECISIONS tail **ADR-0275** (next-free ADR-0276) · **+0 go.mod modules, +0 packages**.

## Task log (filled per task at IMPL)

- **Task 1** (`2f2d772d`): baselines filled below + the final ADR-0045 NO-sub-split re-check confirmed (11 tasks, margin 4 under `~15`).
- **Task 2** (`3959af04`, follow-up `cf551e79`): the `graphite_statsd` parse arm — descriptor-derived `envoy.extensions.…` type URL (envoy-go's FIRST typed-extension stat-sink dispatch), `parseUDPSinkAddressAndPrefix` reuse, the NEW explicit-`max_bytes_per_datagram: 0` reject (PGV parity), the FOUR-sink sibling-reject, the stale `bootstrap.go` dog_statsd comment fixed. Follow-up found and extended a THIRD pre-existing test row (`TestDogStatsdSink_Rejects`'s `sibling_unknown_typeurl`) the PLAN had not anticipated.
- **Task 3** (`918f1d85`): D-GR-BATCHSHARE hoist — `appendBatchLine`/`flushBatch` free functions in `udp.go`; zero direct test call sites on the old `DogStatsdSink` methods confirmed, so the phase-50 dog_statsd batching tests stayed byte-untouched (the hoist's own regression proof).
- **Task 4** (`91810369`): `graphite.go`'s `GraphiteStatsdSink` — tags folded into the metric name via the ~10-LoC `graphiteTagSuffix` (`;k=v`, natural order, dotted keys); a FOURTH sink-private `deltaState`; the shared batching pair from Task 3; `TestNoNewStat_GraphiteRegistrationGuard`.
- **Task 5** (`48244234`): `main.go`'s fourth stats-sink build loop + the flusher-gate clause.
- **Task 6** (`1ee82485`): `FuzzGraphiteStatsdSinkConfigParse` — fuzzers 53 → 54, seeded with the explicit-zero and missing-`statsd_specifier` reject arms.
- **Task 7** (`0d2c6302`, fix `fcb6f379`): `statsdrecv`'s additive `;k=v`-in-name tag extension feeding the existing `Tags()`/`DeltaSumTagged` machinery; the fix round changed three new receiver tests from `t.Fatalf` to `t.Errorf` per independent property (`reference_fatalf_makes_assertions_unreachable`).
- **Task 8** (`7ae49490`): the `0101-stats-sink-graphite` differential — merges the `0093` tag/delta-SUM shape with the `0094` batching-proof shape over the graphite `;k=v` wire grammar; fixtures 102 → 103.
- **Task 9** (`76a325de`, CONTROLLER-EXECUTED): all five deliberate breaks + the non-break confirmed each firing its OWN isolated assertion (see the Break log below); the SPEC's own break-(b) shape recorded VACUOUS.
- **Task 10** (`8f0e0339`): +0 stat surface (1201) confirmed via all four sink registration guards; the full 103-dir differential green; the six-gate green.
- **Task 11** (this commit): ADR-0275 full §Decision/§Consequences entry, the BEHAVIOR_CONTRACT graphite_statsd subsection, STATE.md's active-phase advance, ROADMAP row 57 → `done` + the Observability deferred-candidates sentence update, and this PROGRESS close.

## Break log (filled at Task 9 — the observed failure line per break, per `reference_deliberate_break_wrong_assertion`)

- (a) delta break (`batch = s.delta.apply(batch)` disabled): FIRED the delta-family assertion — `runner_test.go:1318: subject: counter "grpfx.cluster.upstream_rq_total" delta-sum = 21, want 7 (== K)` (21 = 7 absolute re-summed over 3 flushes — the overshoot form, per the PLAN's expectation). Reverted via `git restore`; tree clean.
- (b1) tag-drop break (`graphiteTagSuffix` returns `""` unconditionally): FIRED the pollSubset timeout with the describeSubset diagnostic — `runner_test.go:1244: subj drive: statsd receiver: timed out waiting for COUNTER subset delta-SUM == 7 (grpfx.cluster.upstream_rq_total=0(ok=false) grpfx.http.downstream_rq_total=0(ok=false) grpfx.http.downstream_rq_xx=0(ok=false) )` — all three subset names ok=false, proving the `;k=v` embedding + the Task-7 receiver parsing are load-bearing. Reverted.
- (b2) UnparsedCount-isolating break (`s.write(s.prefix + ".break57:1|q")` after flushBatch): FIRED ALONE — `runner_test.go:1318: subject: UnparsedCount() = 5, want 0 (every subject graphite line must parse)`; the subset converged, the barrier passed, every value/tag/gauge/batching assertion passed (single failure line — the isolating break `reference_deliberate_break_wrong_assertion` demands; in (b1) the poll timeout masks this assertion). Reverted.
- (c1) never-batch break (emit closure `s.write(line)` bypassing appendBatchLine): FIRED the multi-line proof ALONE — `runner_test.go:1318: subject: MaxLinesInAnyDatagram() = 1, want > 1 (no batching observed)`; the oversized-alone check (LinesInDatagram==1) still PASSED. Reverted.
- (c2) infinite-cap break (`appendBatchLine(&buf, line, ^uint64(0), s.write)`): FIRED the oversized-alone check ALONE — `runner_test.go:1318: subject: LinesInDatagram("grpfx.cluster.upstream_rq_total") = (17, true), want (1, true) (the long-cluster-name-tagged line must stay alone)`; the multi-line proof (MaxLines>1) still PASSED. Reverted.
- (d) tag-order-reversal NON-break (reversed `graphiteTagSuffix` iteration): the 0101 differential PASSED (`ok ... 5.111s`) — cross-side set-based tag assertions are order-insensitive, proving liveness the OTHER way. Reverted; `go test ./internal/statssink/ -count=1` re-run GREEN (7.300s), confirming the revert restored the exact-literal `TestGraphiteStatsdSink_TwoTagNaturalOrder`.
- SPEC §8.1 break-(b) VACUITY NOTE (recorded per the PLAN): the SPEC's break-(b) shape (emit dog_statsd `|#k:v` instead of `;k=v`) was NOT run — PLAN-time re-derivation proved `statsdrecv.ingestLine` parses a dog_statsd-formatted line into the IDENTICAL `(name, tags)` buckets, so that break fires NOTHING (a green run would falsely suggest liveness). (b1)+(b2) replace it.
- Flake gate: 20/20 consecutive `TestDifferential/0101` runs PASS (zero failures).
- Race gates: `go test ./internal/statssink/ -race -count=1` ok 11.947s (FULL package, `reference_full_suite_race_after_background_mutator`); `go test ./test/helpers/statsdrecv/ -race -count=1` ok 1.257s.
- Every break was a temporary edit to `internal/statssink/graphite.go` only, reverted via `git restore` before the next; `git status --porcelain` clean + branch undetached verified after each.
- 20/20 flake gate: confirmed at Task 9 (see the "Flake gate" line above).
- full-package `-race` (`internal/statssink` + `test/helpers/statsdrecv`): confirmed at Task 9 (see the "Race gates" line above).

## Final counts (Task 11 — re-run baseline commands on the frozen HEAD)

- `ls -d test/fixtures/*/ | wc -l`: **103** (tail `0101-stats-sink-graphite`)
- `grep -rn '^func Fuzz' --include='*.go' . | wc -l`: **54**
- `go mod tidy -diff`: (EMPTY)
- `go build ./...`: BUILD_OK
- stat surface: **1201** (+0, enforced by `TestNoNewStat_GraphiteRegistrationGuard` + the three pre-existing sink registration guards)
- BackendKind tail: **38** (+0)
- DECISIONS tail: **ADR-0275** (next-free **ADR-0276**)

**Anticipated vs actual (SPEC-57 §14): MATCH on every count.** Row 57 flips `in-progress` → `done` at this commit (the SOLE leg, ADR-0106). The Observability family STAYS OPEN; the dog_statsd explicit-`max_bytes_per_datagram: 0` parity gap is recorded as a new deferred candidate in ROADMAP.md.
