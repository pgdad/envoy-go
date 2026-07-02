# Phase 49 (stats-sink-dogstatsd) IMPL — PROGRESS

**Stage:** IMPL (lifecycle-state 3 → done). References `docs/envoy-go/phases/49-stats-sink-dogstatsd/SPEC-49.md`. The THIRD `stats_sinks[]` consumer + envoy-go's SECOND UDP datagram seam + the SIXTH Observability-family row — a periodic UDP DogStatsd-line-protocol stats sink WITH TAGS (`stats.ExtractTags` in NATURAL/unsorted order) over the LANDED phase-47/48 Flusher/Sink subsystem. **ANCHORS ADR-0266**; row 49 flips `done` at this IMPL six-gate (the sole leg — ADR-0106; the Observability family STAYS OPEN).

**Plan:** `docs/envoy-go/phases/49-stats-sink-dogstatsd/PLAN-49.md` (9-task TDD spine).
**Worktree branch:** `worktree-phase-49-stats-sink-dogstatsd-impl`. Subagents commit LOCALLY only; the controller squashes + pushes at stage-close.

## Baselines (verified at Task 1, in the worktree)

```
go build ./... && echo BUILD_OK
=> BUILD_OK

ls -d test/fixtures/*/ | wc -l
=> 94

grep -rh '^func Fuzz' --include='*.go' . | wc -l
=> 51

grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go
=> 598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
=> 606:	H2GoawayResponder BackendKind = 38

go mod tidy -diff
=> (empty output — clean)

grep -rn 'DogStatsdSinkConfig\|dogStatsdSinkTypeURL\|parseDogStatsdSinkConfig\|statssink/dogstatsd\|\.Tags(' internal/ cmd/ test/ --include='*.go'
=> (empty output — NONE found; phase 49 introduces them)

grep -c 'statsdSinkTypeURL' internal/bootstrap/bootstrap.go
=> 4
```

Baseline: stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **94** / fuzzers **51** / BackendKind **38** / DECISIONS tail **ADR-0265** (next-free **ADR-0266**). All baselines match the PLAN-49 Task 1 anticipated values exactly.

## D-DSD-SPLIT confirmation (ADR-0045 final re-check)

**NO sub-split — a SINGLE FLAT ROW.** Anticipated ~250–320 prod LoC: `internal/statssink/dogstatsd.go` (~SECOND, independent `*net.UDPConn` writer + DogStatsd-line-with-tags mapping over a SECOND sink-private `deltaState`) + the `dogStatsdSinkTypeURL` parse arm + strict-reject arms in `internal/bootstrap/bootstrap.go` + the `main.go` third build loop + the `test/helpers/statsdrecv` extension (`Tags()` accessor + colon/pipe-split revision). All reuse landed templates (phase-48's `statsd.go`, `delta.go`, `sink.go` idioms, the `0092` driver, `statsdrecv`). Well under the ADR-0045 gate; the escape-valve stays UNCONSUMED. 9 tasks total — one fewer than phase 48, since phase 48's `HostGatewayIP` pattern is DUPLICATED (a local `hostGatewayIP` helper in the 0093 driver, per D-DSD-RECEIVER-WIRING) rather than built fresh.

## Anticipated exit counts

stat **1200** (+0 — D-DSD-STATS-FINAL, no self-stats, no sink cluster) / fixtures **95** (`0093-stats-sink-dogstatsd`) / fuzzers **52** (`FuzzDogStatsdSinkConfigParse`) / BackendKind **38** (driver-owned UDP receiver, no new BackendKind) / DECISIONS **ADR-0266** (next-free ADR-0267); **0 new packages, 0 new go.mod modules**.

## Task checklist

- [x] Task 1: PROGRESS scaffold + baselines + ADR-0045 NO-sub-split re-check (D-DSD-SPLIT)
- [x] Task 2: dog_statsd parse arm (`dogStatsdSinkTypeURL` + `DogStatsdSinkConfig` + three-arm dispatch + `parseDogStatsdSinkConfig` + strict-reject arms, `internal/bootstrap/bootstrap.go`)
- [x] Task 3: `FuzzDogStatsdSinkConfigParse` — the no-panic fuzzer over the dog_statsd parse arm (`internal/bootstrap/dogstatsd_fuzz_test.go`)
- [x] Task 4: `DogStatsdSink` (`internal/statssink/dogstatsd.go`) — SECOND `*net.UDPConn` writer + DogStatsd-line-with-tags mapping over a SECOND sink-private `deltaState` + `stats.ExtractTags` in NATURAL order
- [x] Task 5: Boot wiring — the dog_statsd build loop + flusher-gate generalization (`cmd/envoy-go/main.go`)
- [x] Task 6: Extend `test/helpers/statsdrecv` — colon/pipe-split revision + `Tags()` accessor (+ the `DeltaSumTagged` disambiguation accessor added during Task 7)
- [x] Task 7: `0093-stats-sink-dogstatsd` differential fixture (driver + YAMLs + expectations + README), registered in the runner
- [x] Task 8: +0 stat-surface guard (D-DSD-STATS-FINAL) + full differential + six-gate
- [x] Task 9: ADR-0266 body + BEHAVIOR_CONTRACT + STATE/ROADMAP + PROGRESS close + fuzzer-count reconcile

## Final counts (re-verified at Task 9)

```
go build ./... && echo BUILD_OK
=> BUILD_OK

ls -d test/fixtures/*/ | wc -l
=> 95

grep -rh '^func Fuzz' --include='*.go' . | wc -l
=> 52   (reconciled 51 -> 52: FuzzDogStatsdSinkConfigParse)

go mod tidy -diff
=> (empty output — clean)
```

Final: stat surface **1200** (H2 cluster; non-H2 **1196**, +0 — `TestNoNewStat_DogStatsdRegistrationGuard`) / fixtures **95** (`0093-stats-sink-dogstatsd`) / fuzzers **52** (`FuzzDogStatsdSinkConfigParse`) / BackendKind **38** (driver-owned `test/helpers/statsdrecv` UDP receiver, extended in-place, no new BackendKind) / DECISIONS **ADR-0266** (next-free **ADR-0267**); ZERO new packages, ZERO new go.mod modules.

## Status

**DONE.** All 9 tasks complete. Row 49 (`stats-sink-dogstatsd`) FLIPS `done` (the sole leg — ADR-0106; the Observability family STAYS OPEN). ADR-0266 §Decision/§Consequences landed IN-PLACE in `docs/envoy-go/DECISIONS.md`; `docs/envoy-go/BEHAVIOR_CONTRACT.md` gained the "Stats sinks — the dog_statsd UDP sink with tags" subsection; `docs/envoy-go/STATE.md` and `docs/envoy-go/ROADMAP.md` rolled to reflect the phase-49 IMPL done. NEXT: none chartered — the autonomous-router loop re-opens at the next BRAINSTORM charter.
