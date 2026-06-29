# Phase 47.1 IMPL — Progress: the core `metrics_service` stats sink

**SPEC:** `docs/envoy-go/phases/47-stats-sink-metrics-service/SPEC-47.1.md`
**PLAN:** `docs/envoy-go/phases/47-stats-sink-metrics-service/PLAN-47.1.md`
**Worktree branch:** `phase-47.1-stats-sink-metrics-service-impl` (off master, isolated worktree)

This leg adds envoy-go's **FIRST stats-export sink**, the **FIRST `stats_sinks[]` consumer**, the
**FIRST periodic frozen-`Registry`-snapshot sink**, and the **FIRST new go.mod module in the
Observability family** (`github.com/prometheus/client_model v0.6.1`). It **ANCHORS ADR-0262**
(§Decision/§Consequences body lands atomically at the completion task). **ROADMAP row 47
(`stats-sink-metrics-service`) STAYS `in-progress`** — 47.1 is NOT the final leg (row 47 flips
`done` at the 47.2 IMPL per ADR-0106 + `reference_roadmap_split_phase_row_done`); the
Observability family STAYS OPEN.

---

## D-MS-SPLIT confirmation — NO sub-split (§3.0 / D-question resolutions)

The core leg is implemented as **ONE new package (`internal/statssink`), 10 tasks, NO 47.1a/47.1b
sub-split.** The three pieces (mapping / sink / flusher) all reuse established templates — the
`Counter`/`Gauge` accessors, the `ALSClient` client-streaming typed wrapper, the
`GrpcAccessLogSink` bounded-channel sink shape — plus the bootstrap parse arm (the
`parseCommonGrpcAccessLogConfig` precedent) and the main wiring (the ALS/OTLP hoist precedent).
The LoC is moderate and the ADR-0045 soft split-gate is NOT tripped. 47.2 (deltas +
tags-as-labels) is a SEPARATE leg chartered by its own brainstorm (anticipated ADR-0263).

---

## Baselines (verbatim outputs — recorded 2026-06-29, this worktree HEAD)

```
$ go build ./... && echo BUILD_OK
BUILD_OK

$ ls -d test/fixtures/*/ | wc -l                                   # expect 90 (tail 0088-tracing-zipkin)
90
$ ls -d test/fixtures/*/ | tail -1
test/fixtures/0088-tracing-zipkin/

$ grep -rh '^func Fuzz' --include='*.go' . | wc -l                 # expect 49
49

$ grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go # expect = 38 (BackendKind tail)
598:	// H2GoawayResponder is a raw-framer in-process h2c (prior-knowledge) responder
606:	H2GoawayResponder BackendKind = 38

$ grep -c 'prometheus/client_model' go.mod go.sum                  # expect 0 0 (NOT present — 47.1 lands it)
go.mod:0
go.sum:0

$ grep 'go-control-plane/envoy' go.mod                             # expect v1.32.4 (already direct)
	github.com/envoyproxy/go-control-plane/envoy v1.32.4

$ grep -rn 'statssink\|MetricsServiceClient\|StatsSinkConfig\|metricsServiceTypeURL' internal/ cmd/ --include='*.go'
(no matches — 47.1 introduces them)
```

**Baseline summary:** stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **90** /
fuzzers **49** / BackendKind **38** / DECISIONS tail **ADR-0261** (next-free **ADR-0262**) /
`client_model` ABSENT (0 0) / `go-control-plane/envoy v1.32.4` already direct / build OK.

---

## Anticipated exit counts (47.1 IMPL complete)

| Dimension | Baseline | Exit | Δ | Note |
|---|---|---|---|---|
| stat surface | 1200 (non-H2 1196) | 1200 (non-H2 1196) | **+0** | D-MS-STATS-FINAL — NO `metrics_service`-scoped self-stat (matches the reference's +0; sidesteps the self-referential STATS-sink subtlety) |
| fixtures | 90 | **91** | +1 | `0089-stats-sink-metrics-service` (cross-side EXACT, COUNTER name-subset) |
| fuzzers | 49 | **50** | +1 | `FuzzStatsSinkConfigParse` (the `stats_sinks[]`/`MetricsServiceConfig` parse boundary) |
| BackendKind | 38 | **38** | +0 | the `MetricsService` receiver is a driver-owned `test/helpers/metricsservice` server the proxy DIALS, NOT a runner `BackendKind` |
| DECISIONS | ADR-0261 | **ADR-0262** | +1 | ANCHORS ADR-0262 §Decision/§Consequences at the completion task |
| new package | — | **+1** | +1 | `internal/statssink` (`mapping.go` + `sink.go` + `flusher.go`) |
| new go.mod module | — | **+1** | +1 | `github.com/prometheus/client_model v0.6.1` (direct; the FIRST Observability-family module) |

---

## Task checklist

- [x] **Task 1:** Phase scaffolding — PROGRESS-47.1.md + baselines + the final ADR-0045 split re-check (D-MS-SPLIT: NO sub-split)
- [x] **Task 2:** `Counter`/`Gauge` → `MetricFamily` mapping (`internal/statssink/mapping.go`) + land `prometheus/client_model v0.6.1`
- [x] **Task 3:** `MetricsServiceClient` CLIENT-streaming typed wrapper (`internal/grpcclient/grpcclient.go`)
- [x] **Task 4:** `MetricsServiceSink` (`internal/statssink/sink.go`) [`-race`]
- [x] **Task 5:** `Flusher` (`internal/statssink/flusher.go`) [`-race`]
- [x] **Task 6:** bootstrap `stats_sinks[]`/`stats_flush_interval` parse + strict-reject arms + `FuzzStatsSinkConfigParse` (`internal/bootstrap/bootstrap.go`)
- [x] **Task 7:** the post-`Freeze` main wiring (`cmd/envoy-go/main.go`) + `Flusher.Start(ctx)` + the no-new-stat registration guard + byte-stability
- [x] **Task 8:** the driver-owned `test/helpers/metricsservice` receiver
- [x] **Task 9:** the `0089-stats-sink-metrics-service` cross-side EXACT differential (TWO per-side receivers)
- [x] **Task 10:** deliberate breaks + flake-soak + full-package `-race` + the full 91-dir differential + six-gate + completion bundle (ADR-0262, BEHAVIOR_CONTRACT, STATE, ROADMAP row 47 STAYS `in-progress`)

---

## Completion — as-built verification (2026-06-29, controller, frozen HEAD)

### Six-gate (FINAL frozen HEAD)

```
gofmt -l .                          → empty (clean)
golangci-lint run ./...             → exit 0 (clean)
go vet ./...                        → clean
go build ./...                      → BUILD_OK
go test ./... -count=1 (non-diff)   → all packages pass
go test ./test/differential -count=1 (full 91-dir) → ok 278.548s (0 FAIL; no non-sink fixture moved)
go test ./internal/statssink -race -count=1        → ok (FULL package; the Flusher ticker + sink writer are background mutators)
```

### `0089` differential — deliberate breaks (controller-run, `-count=1`, restored via `git restore`)

All FOUR bite on the **subject** side (the two-per-side-receiver isolation makes subject breaks visible — a single shared receiver would have masked them behind the reference's concurrent periodic flushes):

| Break | Edit | Result |
|---|---|---|
| (a) mapping value | counter emits `0` not `Load()` | FAIL — `subj drive: subset == 7 ... =0` (poll timeout) |
| (b) metric type | counter emitted as `GAUGE` (value preserved) | FAIL — `subject: family ... type = GAUGE, want COUNTER` |
| (c) identifier node | emit `node{id:"BREAK-WRONG"}` | FAIL — `subject: identifier node.id = "BREAK-WRONG", want "envoy-go-subject-0089"` |
| (d) disabled flush | `flushOnce` returns before `Submit` | FAIL — `subj drive: ... =0(ok=false), node=false` (decode-did-not-run) |

**Flake-soak:** `0089` 20/20 PASS (`-count=1` each).

### Root-caused corrections (controller, `superpowers:systematic-debugging`)

The Task-9 differential as first authored hung the full 10-min `go test` timeout, then (after the first fix) showed an empty reference identifier. Two root causes, two fixes:

1. **`GracefulStop` deadlock → hard `Close()`.** `metricsservice.Server.Stop()` (GracefulStop) blocks forever waiting for the proxies' long-lived `StreamMetrics` streams to drain (they never do). Added `metricsservice.Server.Close()` (hard `grpc.Server.Stop`) and switched the driver to it — the 0087 `srv.Close()` precedent. (OTLP/Zipkin never hit this: unary Export / HTTP, no long-lived stream.)
2. **Single shared receiver → TWO per-side receivers.** `metrics_service` flushes PERIODICALLY, so the reference proxy keeps streaming into the shared accumulator during the subject's drive window — contaminating the subject snapshot (both 7s) and making subject-side deliberate breaks vacuous. Fixed to two private per-side receivers (one host port each; no `Reset()` between sides; strict per-side assertions; both identifiers captured). The 0087/0088 single-receiver pattern works only because trace export is event-driven (stops when requests stop).

### Final counts (reconciled)

stat surface **1200** (non-H2 **1196**) — D-MS-STATS-FINAL **+0** (`TestNoNewStat_RegistrationGuard` live) · fixtures **91** (tail `0089-stats-sink-metrics-service`) · fuzzers **50** (`FuzzStatsSinkConfigParse`; `^func Fuzz` == 50) · BackendKind **38** (driver-owned receiver) · DECISIONS **ADR-0262** (next-free **ADR-0263**) · **+1 go.mod module** (`github.com/prometheus/client_model v0.6.1`, direct) · **1 new package** (`internal/statssink`) · `go mod tidy -diff` EMPTY.

**ROADMAP row 47 (`stats-sink-metrics-service`) STAYS `in-progress`** (47.1 is the FIRST of two legs; 47.2 [deltas + tags-as-labels, anticipated ADR-0263] is the FINAL leg). The Observability family STAYS OPEN. **NEXT → the 47.2 BRAINSTORM.**
