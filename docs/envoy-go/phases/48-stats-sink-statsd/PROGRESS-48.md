# Phase 48 (stats-sink-statsd) IMPL — PROGRESS

**Stage:** IMPL (lifecycle-state 3 → done). The SECOND `stats_sinks[]` consumer + envoy-go's FIRST UDP datagram seam + the FIFTH Observability-family row. **ANCHORS ADR-0265**; row 48 flips `done` at this IMPL six-gate (the sole leg — ADR-0106; the Observability family STAYS OPEN).

**Plan:** `docs/envoy-go/phases/48-stats-sink-statsd/PLAN-48.md` (10-task TDD spine).
**Worktree branch:** `worktree-phase-48-stats-sink-statsd-impl` (base `origin/master` @ `02460034`, the PLAN commit). Subagents commit LOCALLY only; the controller squashes + pushes at stage-close.

## Baselines (verified at Task 1, in the worktree)

```
go build ./...                                   => BUILD_OK
ls -d test/fixtures/*/ | wc -l                   => 93   (tail 0091-stats-sink-metrics-service-labels)
grep -rh '^func Fuzz' --include='*.go' . | wc -l => 50
go mod tidy -diff                                => EMPTY (clean)
```
Baseline: stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **93** / fuzzers **50** / BackendKind **38** / DECISIONS tail **ADR-0264** (next-free **ADR-0265**).

## D-SD-SPLIT confirmation (ADR-0045 final re-check)

**NO sub-split — a SINGLE FLAT ROW.** Anticipated ~200–250 prod LoC: `statsd.go` (~80) + the parse arm (~40) + the `main.go` second loop (~12) + the `statsdrecv` receiver (~110) + the `HostGatewayIP` helper (~20). All reuse landed templates (`delta.go`, the `sink.go` idioms, the `0090` driver, the `metricsservice` receiver). Well under the ADR-0045 gate; the 48.1/48.2 escape-valve stays UNCONSUMED.

## Anticipated exit counts

stat **1200** (+0 — no self-stats, no sink cluster; D-SD-STATS-FINAL) / fixtures **94** (`0092-stats-sink-statsd`) / fuzzers **51** (`FuzzStatsdSinkConfigParse`) / BackendKind **38** (driver-owned UDP receiver) / DECISIONS **ADR-0265** (next-free ADR-0266); **0 new packages, 0 new go.mod modules**.

## Task checklist

- [x] Task 1: PROGRESS scaffold + baselines + ADR-0045 NO-sub-split re-check
- [x] Task 2: statsd parse arm (`statsdSinkTypeURL` + `StatsdSinkConfig` + two-arm dispatch + `parseStatsdSinkConfig` + strict-reject)
- [x] Task 3: `FuzzStatsdSinkConfigParse` (50 → 51)
- [x] Task 4: `StatsdSink` (`internal/statssink/statsd.go` — UDP writer + delta + line mapping)
- [x] Task 5: `main.go` statsd build loop + flusher-gate generalization
- [x] Task 6: `test/helpers/statsdrecv` UDP receiver
- [x] Task 7: harness `HostGatewayIP`
- [x] Task 8: `0092-stats-sink-statsd` differential + breaks + 20/20 flake + full-package race
- [x] Task 9: +0 surface guard + full 94-dir differential + six-gate
- [x] Task 10: ADR-0265 body + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 48 done) + fuzzer-count reconcile

## Exit counts (verified at Task 10, in the worktree)

```
go build ./...                                   => BUILD_OK
ls -d test/fixtures/*/ | wc -l                   => 94   (tail 0092-stats-sink-statsd)
grep -rh '^func Fuzz' --include='*.go' . | wc -l => 51   (+ FuzzStatsdSinkConfigParse)
go mod tidy -diff                                => EMPTY (clean)
```
Exit: stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **94** / fuzzers **51** / BackendKind **38** / DECISIONS tail **ADR-0265** (next-free **ADR-0266**). ZERO new packages, ZERO new go.mod modules. Matches the anticipated exit counts exactly.

## TWO IMPL deviations from the PLAN draft (recorded in ADR-0265 §Consequences)

1. **`differential.HostGatewayIP` host-gateway resolution.** The PLAN drafted it as "inspect the Docker `bridge` network's IPAM gateway" — WRONG under Docker Desktop (the bridge IPAM gateway is the VM-internal interface; a reference container sending UDP there reaches nothing). The LANDED helper resolves the host-gateway LITERAL IP via a throwaway `alpine:3` container (`getent hosts host.docker.internal` under `ExtraHosts: host.docker.internal:host-gateway`) — the bridge gateway on native Linux, the VM→host gateway on Docker Desktop; container→host UDP delivery empirically verified. statsd rejects hostnames, so a literal IP is required (AMEND-SD-REJECT).
2. **The `0092` gauge subset uses `cluster.<backend>.membership_total == 1 |g`, NOT `membership_healthy`.** envoy-go registers `membership_healthy` ONLY on clusters WITH `health_checks` (`internal/cluster/manager.go`); the `0092` cluster has none, so the subject never emits it. `membership_total` is registered UNCONDITIONALLY (== endpoint count, value 1) and is cross-side-equal. A noted pre-existing envoy-go behavior (membership_healthy is health-check-gated), not a phase-48 bug.

## Status

**COMPLETE.** Row 48 (`stats-sink-statsd`) flips `done` (the SOLE leg — ADR-0106; NO parent rollup); the Observability family STAYS OPEN. ANCHORS ADR-0265.
