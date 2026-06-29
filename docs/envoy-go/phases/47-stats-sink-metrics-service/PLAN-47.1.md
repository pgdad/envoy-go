# Phase 47.1 Implementation Plan — the core `metrics_service` stats sink: a NEW `internal/statssink` package (the `stats_flush_interval`-driven `Flusher` walking the frozen `stats.Registry` + the `MetricsServiceSink` over the ALS client-streaming lifecycle + the cumulative/no-labels `Counter`/`Gauge` → `io.prometheus.client.MetricFamily` mapping) + the `MetricsServiceClient` CLIENT-STREAMING typed wrapper over `grpcclient.Dialer` + the `stats_sinks[]`/`stats_flush_interval` parse + strict-reject arms + the post-`Freeze` main wiring + the driver-owned `test/helpers/metricsservice` receiver + the `0089-stats-sink-metrics-service` cross-side EXACT differential + `FuzzStatsSinkConfigParse` — the FIRST `stats_sinks[]` consumer + the FIRST periodic registry-snapshot sink + the FIRST new go.mod module in the Observability family (`github.com/prometheus/client_model v0.6.1`); ANCHORS ADR-0262; row 47 STAYS `in-progress`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). NOTE the execution lesson (`feedback_subagent_autocommit_claudemd`): the global CLAUDE.md makes dispatched subagents AUTO-COMMIT — do NOT fight it; the controller VERIFIES each commit (correct fileset, real non-vacuous tests via `-v` + read assertions, gates green), cleans stray next-task leak files, re-runs the full suite on the FINAL frozen HEAD, does the deliberate-break verification ITSELF, and squashes + pushes at stage-close.

**Goal:** When the bootstrap carries a `metrics_service` `stats_sinks[]` entry, envoy-go dials the configured `grpc_service.envoy_grpc.cluster_name` cluster (via `grpcclient.Dialer`), opens the `MetricsService.StreamMetrics` CLIENT-streaming RPC, and every `stats_flush_interval` (default 5s) snapshots the frozen process-global `stats.Registry` (`Walk`) and streams it as `io.prometheus.client.MetricFamily` protos — the first message carrying `identifier{node}`, each subsequent message carrying only `envoy_metrics`. The mapping is cumulative/no-labels: `Counter` → `COUNTER` absolute, `Gauge` → `GAUGE`, the FULL dotted name (tags inlined), ZERO `LabelPair`s, a flush-time `timestamp_ms`. Proven cross-side EXACT on a deterministic COUNTER name-subset against `contrib-v1.37.2` by the `0089` differential through a driver-owned `MetricsService` receiver. **ANCHORS ADR-0262** (its §Decision/§Consequences body lands atomically here); ROADMAP row 47 (`stats-sink-metrics-service`) **STAYS `in-progress`** (47.1 is NOT the final leg — row 47 flips `done` at the 47.2 IMPL per ADR-0106 + `reference_roadmap_split_phase_row_done`); the Observability family STAYS OPEN.

**Architecture:** ONE new Go package (`internal/statssink`) with three pieces — the `Flusher` (a `time.Ticker`-driven `Registry.Walk` snapshot loop started AFTER `bs.Stats.Freeze()`), the `MetricsServiceSink` (the ALS `GrpcAccessLogSink` bounded-channel + writer-goroutine + identifier-once + reconnect-resend + idempotent-`Close` client-streaming template, SIMPLIFIED to a 1-deep flush handoff with NO size/interval accumulation), and the cumulative/no-labels `Counter`/`Gauge` → `dto.MetricFamily` mapping. PLUS the `MetricsServiceClient` typed wrapper (the `ALSClient` CLIENT-streaming precedent, ADR-0158 — NOT the unary OTLP wrappers), the `stats_sinks[]`/`stats_flush_interval` parse arm + strict-reject arms in `internal/bootstrap`, the post-`Freeze` `Flusher.Start(ctx)` boot wiring in `cmd/envoy-go/main.go`, the driver-owned `test/helpers/metricsservice` gRPC receiver, the `0089` differential, and `FuzzStatsSinkConfigParse`. The FIRST new go.mod module in the Observability family: `github.com/prometheus/client_model v0.6.1` (the `MetricFamily` entry proto referenced by `StreamMetricsMessage.EnvoyMetrics`; the metrics service/config protos resolve at the already-direct `go-control-plane/envoy v1.32.4`). Byte-identical and stat-surface-identical when no `metrics_service` `stats_sinks[]` entry is configured (every non-sink path untouched — the full differential is the regression anchor; the `Flusher`/sink build ONLY when ≥1 `StatsSinkConfig` exists).

**Tech Stack:** Go; the NEW `internal/statssink` package (`mapping.go` + `sink.go` + `flusher.go`); `internal/grpcclient` (the `MetricsServiceClient` client-streaming wrapper); `internal/bootstrap` (the `stats_sinks[]`/`stats_flush_interval` parse + strict-reject arms + the `config/metrics/v3` blank-import); `cmd/envoy-go/main.go` (the post-Freeze `Flusher.Start`); the driver-owned `test/helpers/metricsservice` h2c gRPC receiver (the `test/helpers/otlptrace` analog); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`, the `0087`/`0088` precedent). The new module is `github.com/prometheus/client_model v0.6.1` (`dto "github.com/prometheus/client_model/go"`); `MetricsServiceConfig`/`StatsSink`/`StreamMetricsMessage` resolve at `go-control-plane/envoy v1.32.4` (already direct).

---

## Orientation — read before Task 1 (the zero-context brief)

You are adding envoy-go's FIRST stats-export sink and FIRST consumer of the bootstrap `stats_sinks[]` field. The substrates are ALL built — the `stats.Registry.Walk` contract, the `grpcclient.Dialer` + the `ALSClient` client-streaming typed-wrapper, the ALS `GrpcAccessLogSink` bounded-channel sink template, and the `cmd/envoy-go/main.go` post-Freeze background-goroutine + LIFO-drain wiring. 47.1 adds a bounded delta: one new package (three pieces), one typed wrapper, one config-parse arm, one main-wiring block, one receiver helper, one differential, one fuzzer, one new go.mod module.

**What ALREADY works (do NOT re-build) — verified at PLAN time (re-confirm line numbers before editing; files evolve):**

- **`internal/stats/registry.go`** — `type Registry struct{ mu sync.RWMutex; metrics []Metric; byName map[string]Metric; frozen atomic.Bool }` (`:60`); `Walk(fn func(Metric))` (`:134` — RLock + iterate `r.metrics` in registration order); `Freeze()` (`:209` — `r.frozen.Store(true)`); `NewCounter(name)` (`:79` — PANICS on duplicate/invalid name/post-freeze); `IsValidName(name) bool` (`:55`). The `Metric` interface (`:30`): `Name() string`, `Type() MetricType`, `Format() string` — **NOTE: `Load()` is NOT on the interface** (it is on the concrete types), so the mapping TYPE-SWITCHES on `*stats.Counter`/`*stats.Gauge`.
- **`internal/stats/counter.go`** — `type Counter struct{ name string; v atomic.Uint64 }`; `Name() string`, `Type() MetricType { return MetricCounter }`, `Load() uint64`. **`internal/stats/gauge.go`** — `type Gauge struct{ name string; v atomic.Int64 }`; `Name() string`, `Type() MetricType { return MetricGauge }`, `Load() int64`. The enum: `MetricCounter MetricType = iota + 1`, `MetricGauge` (registry.go `:16`).
- **`internal/grpcclient/grpcclient.go`** — `type Dialer struct{ mgr *cluster.Manager }` (`:81`); `New(mgr) *Dialer` (`:88`); `DialContext(ctx, clusterName) (*grpc.ClientConn, error)` (`:108` — `mgr.Get` (`unknown cluster` on miss), `clu.UseH2()` gate (`...does not have http2_protocol_options{}...` on miss), `grpc.NewClient("passthrough:///"+clusterName, …insecure…)`). The **`ALSClient` CLIENT-STREAMING template** (`:261`): `type ALSClient struct{ conn *grpc.ClientConn; stub accesslogv3.AccessLogServiceClient; target string; closeOnce sync.Once; closeErr error }`; `NewALSClient(d *Dialer, clusterName string) (*ALSClient, error)` (`:279` — nil-dialer guard → `d.DialContext(context.Background(), clusterName)` → wrap); `StreamAccessLogs(ctx) (…AccessLogService_StreamAccessLogsClient, error)` (`:297`); `Close() error` (`:307` — `sync.Once`-guarded `conn.Close`). **`MetricsServiceClient` mirrors this EXACTLY** (`StreamMetrics` is client-streaming like `StreamAccessLogs`).
- **`internal/accesslog/grpcsink.go`** — the **`MetricsServiceSink` template**. `type GrpcAccessLogSink struct{ ch chan any; client alsClient; node *corev3.Node; …Counter; done chan struct{}; closeOnce sync.Once; closeErr error; lastDropLog atomic.Int64; ctx context.Context; cancel context.CancelFunc; … }` (`:48`); `newGrpcSinkWithCapacity(...)` allocates `ch`, sets `ctx,cancel = context.WithCancel(...)`, `go s.run()` (`:86`); `Submit(r any)` — `select { case s.ch <- r: default: …drop + rate-limited log via lastDropLog… }` (`:121`); `Close() error` — `closeOnce.Do(close(ch) → wait done OR closeDrainGrace → cancel → client.Close())` (`:145`); `run()` — the writer goroutine: lazily open `stream`, send `identifier` ONCE per stream (`sentIdentifier` flag), flush each batch, reopen-once + re-send-identifier + re-send-batch on a `Send` error, drain + `CloseAndRecv` on channel close (`:166`). The `alsClient` seam interface decouples the sink from `grpcclient` for testing (a fake client in `grpcsink_test.go`). 47.1's sink is SIMPLER — no `bufferSizeBytes`/`bufferFlushInterval`/header lists (the Flusher is the trigger; each Submit is one complete batch).
- **`internal/bootstrap/bootstrap.go`** — `Load(r io.Reader) (*Bootstrap, error)` (`:308`): yaml→json→`protojson.Unmarshal` into `bootstrapv3.Bootstrap`, `result := &Bootstrap{Proto: bs, Stats: stats.NewRegistry()}`, then `parseAccessLogConfigs(bs, result)` (`:333`). **TODAY `Bootstrap.stats_sinks` (field 6) + `stats_flush_interval` (field 7) are DROPPED** (the ADR-0064 silent-ignore posture). The TypeURL-const + per-arm dispatch precedent: `parseOneAccessLog` (`:378`) switches `tc.GetTypeUrl()` against consts (`fileAccessLogTypeURL`/`httpGrpcAccessLogTypeURL`/`otlpAccessLogTypeURL`, `:164`); the shared parse helper `parseCommonGrpcAccessLogConfig(common, idx, sinkLabel) (clusterName, logName, bufBytes, flush, err)` (`:465`) rejects `transport_api_version == V2`, requires `envoy_grpc` (rejects `google_grpc`), requires non-empty `cluster_name`. The proto blank-import registry block (`:51`–`:157`, ADR-0016). The bootstrap node: `bs.Proto.GetNode().GetId()`/`.GetCluster()`.
- **`cmd/envoy-go/main.go`** — `dialer := grpcclient.New(cm)` is built UNCONDITIONALLY (`:131`); the ALS/OTLP sink build is a `if len(bs.ALSConfigs) > 0 || len(bs.OTLPConfigs) > 0 { … }` block (`:147`–`:172`) that registers counters + `sinks = append(sinks, …)`; the access-log sinks join a defer-LIFO `for _, s := range sinks { _ = s.Close() }` (`:173`); `bs.Stats.Freeze()` (`:325`); the post-Freeze background goroutines (`cm.StartHealthChecks`/`cm.StartOutlierDetection`) start AFTER Freeze (`:327`+). **The `Flusher.Start(ctx)` must be called AFTER `:325` Freeze** so the `Walk` snapshot is over a frozen registry (ADR-0059).
- **`test/helpers/otlptrace/otlptrace.go`** (212 LoC) — the **`test/helpers/metricsservice` template**: `type Server struct{ coltracepb.UnimplementedTraceServiceServer; addr string; lis net.Listener; grpcSrv *grpc.Server; mu sync.RWMutex; spans []…; stopOnce sync.Once }`; `New(t testing.TB) *Server` (`:72` — `newServer("127.0.0.1:0")` + `t.Cleanup(s.Stop)`); `NewAtAddr(addr) (*Server, error)` (`:92`); `Count() int` (`:155`); `Reset()` (`:181`); `Addr() string` (`:191`); `Stop()` (`:199` — `stopOnce.Do(grpcSrv.GracefulStop)`); the service registration `coltracepb.RegisterTraceServiceServer(s.grpcSrv, s)` in `newServer`. The handler accumulates received data under the mutex.

**The metrics_service wire model (§11 D-MS-* — live-probed against `contrib-v1.37.2` 2026-06-29; all pinned in SPEC-47.1.md):**
- **The mapping** (AMEND-MS-FIELDS-CONFIRMED): every `MetricFamily` carries the FULL DOTTED internal name with tag VALUES inlined (`cluster.backend.upstream_rq_total`, `http.ingress_http.downstream_rq_2xx`), `type` ∈ {`COUNTER`,`GAUGE`} (envoy-go has NO histograms — ADR-0060 — so the reference's `HISTOGRAM` families have no envoy-go analog: **AMEND-MS-HISTOGRAM-PRESENT**), `metric[0].counter.value`/`gauge.value` = the CUMULATIVE ABSOLUTE value (`counter.value:7 == K`), `help` EMPTY, `metric[].timestamp_ms` SET on every metric (flush ms — **AMEND-MS-TIMESTAMP-ALWAYS-SET**, value non-deterministic/UNasserted), `metric[].label` EMPTY (0 labels under `emit_tags_as_labels` unset).
- **The lifecycle** (D-MS-LIFECYCLE): `identifier{node}` on message #1 ONLY (re-armed on a reconnect); one FULL-registry-snapshot batch per `stats_flush_interval` (default 5s); auto-reconnect on receiver close (fresh stream, identifier re-sent). The `identifier.node` carries `id`+`cluster` (envoy-go-settable, cross-side assertable) + impl-specific `user_agent_*` (UNasserted).
- **The reference is STRICTER than tracing** (AMEND-MS-REFERENCE-STRICTER): it HARD-REJECTS a missing/unknown gRPC cluster + a deprecated non-V3 `transport_api_version` (exit 1) ⇒ envoy-go's cluster-exists (`DialContext` gate) + non-V3 rejects are REFERENCE-PARITY. The envoy-go-STRICT rejects (ADR-0080 — the reference BOOTS these) are the knob VALUES (`report_counters_as_deltas:true` / `emit_tags_as_labels:true` / non-default `histogram_emit_mode`), the sibling sink TypeURLs (`reference_strict_reject_sibling_typeurl_gap`), `google_grpc`, an empty `cluster_name`, and `stats_flush_on_admin`.
- **ZERO self-stats** (AMEND-MS-NO-SELF-STATS): the reference registers NO `metrics_service`-scoped stat; the surface delta is **+0** (see D-MS-STATS-FINAL).

### Proto facts (verified at PLAN time; `go-control-plane/envoy v1.32.4` already direct; `client_model` is the SOLE new module)

- **`metricsconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/metrics/v3"`** (the `StatsSink` wrapper + `MetricsServiceConfig`; pulls NO new module — already-present go-control-plane). The TypeURL is `type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig` (SPEC §11 live-verified; **the IMPL MUST re-derive it via the proto descriptor, NOT hard-code blindly** — see Task 6 / `reference_network_filter_typeurl_extensions`). `MetricsServiceConfig` accessors: `GetGrpcService() *corev3.GrpcService` (field 1), `GetReportCountersAsDeltas() *wrapperspb.BoolValue` (2), `GetTransportApiVersion() corev3.ApiVersion` (3), `GetEmitTagsAsLabels() bool` (4), `GetHistogramEmitMode() MetricsServiceConfig_HistogramEmitMode` (5, default `SUMMARY_AND_HISTOGRAM`=0).
- **`metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"`** (the service; pulls `client_model` transitively). `metricsv3.NewMetricsServiceClient(conn)` → stub; `stub.StreamMetrics(ctx) (MetricsService_StreamMetricsClient, error)` (CLIENT-streaming — `Send(*StreamMetricsMessage) error` + `CloseAndRecv() (*StreamMetricsResponse, error)`; FullMethod `/envoy.service.metrics.v3.MetricsService/StreamMetrics`). `metricsv3.StreamMetricsMessage{ Identifier *StreamMetricsMessage_Identifier; EnvoyMetrics []*dto.MetricFamily }`; `metricsv3.StreamMetricsMessage_Identifier{ Node *corev3.Node }`.
- **`dto "github.com/prometheus/client_model/go"`** (the NEW module `github.com/prometheus/client_model v0.6.1` — MVS-resolved, D-MS-MODULE; lands at Task 2 via the first `dto` import + `go get …@v0.6.1 && go mod tidy`). `dto.MetricFamily{ Name *string; Help *string; Type *MetricType; Metric []*Metric }`; `dto.MetricType_COUNTER`/`_GAUGE` (`.Enum()` → `*MetricType`); `dto.Metric{ Label []*LabelPair; Gauge *Gauge; Counter *Counter; TimestampMs *int64 }`; `dto.Counter{ Value *float64 }`; `dto.Gauge{ Value *float64 }`. Pointer fields → use `proto.String`/`proto.Float64`/`proto.Int64` (`google.golang.org/protobuf/proto`).
- `bs.GetStatsSinks() []*metricsconfigv3.StatsSink` (field 6); `bs.GetStatsFlushInterval() *durationpb.Duration` (7); `bs.GetStatsFlushOnAdmin() bool` (the `stats_flush` oneof alternative). `StatsSink.GetTypedConfig() *anypb.Any`; unmarshal via `tc.UnmarshalTo(&metricsconfigv3.MetricsServiceConfig{})`.

### Discipline (honor on EVERY task)

- **TDD** (`superpowers:test-driven-development`): failing-test → run-fail → minimal-impl → run-pass → commit.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): `gofmt -l` (empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`.
- **Worktree hygiene** (`feedback_subagent_worktree_detach`/`_path_targeting`): subagents write to the WORKTREE path; the controller verifies the main checkout stays clean + the branch is undetached after each task.
- **Commit locally only** (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0089'`, NEVER bare `'0089'` (bare matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): 47.1 ADDS background mutators (the `Flusher` ticker goroutine + the `MetricsServiceSink` writer goroutine). A `-run`-subset `-race` MISSES a data race an earlier test's lingering goroutine causes — the `-race` gate MUST run the FULL `internal/statssink` package.
- **Streaming-sink framing** (`reference_streaming_sink_differential_framing` + AMEND-MS-HISTOGRAM-PRESENT): assert the aggregated COUNTER/GAUGE NAME-SUBSET by dotted name, NEVER the whole family set (the surfaces differ cross-side — envoy-go has no histograms) NOR the stream/message framing.
- **Driver-owned receiver** (`reference_differential_grpc_receiver_driver_owned`): the `MetricsService` receiver is a `test/helpers/metricsservice` server the proxy DIALS — NOT a runner `BackendKind` (stays 38).
- **Docker bridge** (`reference_docker_probe_bridge_network`): the receiver must be reachable from the reference container by hostname over a shared bridge (h2c, `http2_protocol_options{}`); verify decode RAN (the receiver's family count > 0) before trusting a green.
- **Release barrier** (`reference_concurrency_differential_release_barrier`): poll the receiver to converge to the post-K values — NEVER a `time.Sleep`.
- **Wire-format both sides** (`reference_wire_format_both_sides_see_same_bytes`): the `MetricFamily` model is shared — the §11 live probe is the wire truth.
- **Dynamic-name charset** (`reference_dynamic_stat_name_charset_guard`): NOT a concern here — the mapped names come from the registry (`IsValidName`-validated at registration), NOT config-derived; no charset guard needed in the mapping.

---

## D-question resolutions (the SPEC §12 D-MS-* PLAN/IMPL pins — settled here)

**D-MS-SPLIT → NO sub-split (one `internal/statssink` package, 10 tasks).** The core leg is moderate LoC: three pieces (mapping/sink/flusher) all reusing established templates (the `Counter`/`Gauge` accessors, the `ALSClient` client-streaming wrapper, the `GrpcAccessLogSink` sink shape) + the bootstrap parse arm (the `parseCommonGrpcAccessLogConfig` precedent) + the main wiring (the ALS/OTLP hoist precedent). A 47.1a/47.1b sub-split is NOT warranted (ADR-0045 soft gate). 47.2 (deltas + tags-as-labels) is a SEPARATE leg chartered by its own brainstorm (anticipated ADR-0263).

**D-MS-CONFIG-HOME → `StatsSinkConfig` lives in `internal/bootstrap`** (mirroring `ALSConfig`/`OTLPConfig`), parsed by `Load`; a top-level `FlushInterval time.Duration` on the `Bootstrap` struct. `internal/statssink` takes PRIMITIVE params (the registry, the interval, the `[]Sink`; the sink takes a `metricsClient` seam + the `*corev3.Node`) so `bootstrap` does NOT import `statssink` — acyclic, the exact ALS/OTLP precedent (`bootstrap → accesslog` never happens; `main.go` wires bootstrap config → the sink). `statssink` imports `stats` + the metrics protos + reaches the gRPC client via a `metricsClient` seam interface (the `alsClient` seam precedent).

**D-MS-SINK-BUFFER → a small bounded channel of `[]*dto.MetricFamily` batches (capacity 8), drop-on-full + rate-limited log (NO counter — D-MS-STATS-FINAL).** The `Flusher` IS the trigger — each `Submit` is one already-complete full-registry snapshot — so the sink does NO size/interval accumulation (UNLIKE the ALS sink's `bufferSizeBytes`/`bufferFlushInterval`). The writer goroutine receives each batch and sends it as ONE `StreamMetricsMessage{EnvoyMetrics: batch}`. Capacity 8 gives slack for a slow stream without unbounded growth; a full channel drops the oldest pending flush (`select … default`) and rate-limit-logs (the `lastDropLog` pattern). 

**D-MS-APIVERSION → accept `transport_api_version` ∈ {`AUTO`(0), `V3`(2)}; reject `V2`(1) and any other value.** `AUTO`→`V3` (the GcpAccessLog-parity semantics; the shared ALS helper rejects only V2). Reference-parity (the reference rejects V2 as deprecated, exit 1).

**D-MS-FLUSH-INERT → no `Flusher` builds without a `metrics_service` sink.** A bare `stats_flush_interval` is parsed-and-stored on `Bootstrap.FlushInterval` (inert) but the `Flusher` constructs ONLY when `len(bs.StatsSinkConfigs) > 0` (byte-stable; no flush loop, no Dialer use). `stats_flush_on_admin` present → boot-reject regardless (ADR-0080; no existing fixture sets it, so byte-stability holds).

**D-MS-STATS-FINAL → +0 (NO new `metrics_service`-scoped stat names; surface stays 1200 / non-H2 1196).** This MATCHES the reference (AMEND-MS-NO-SELF-STATS: zero self-stats) AND sidesteps the self-referential export subtlety unique to a STATS sink (a self-counter registered before Freeze would itself appear in its own next flush). This is a DELIBERATE departure from the 45.1/46.1 exporter-counter convention (`logs_written`/`spans_sent`), justified by (a) the reference's +0 and (b) the self-referential weirdness. Channel-full drops are rate-limit-LOGGED (the ALS `lastDropLog` pattern), NOT counted. A registration test (Task 7) asserts the surface is UNCHANGED (no `statssink`/`metrics_service`-scoped name added).

**D-MS-RECEIVER-WIRING → `test/helpers/metricsservice` built fresh from the `test/helpers/otlptrace` template** (the §11 scratchpad seed `scratchpad/probe/receiver/main.go` is GONE — scratchpad is ephemeral; build from the otlptrace shape). An in-process h2c `metricsv3.MetricsServiceServer` (embed `UnimplementedMetricsServiceServer`; `metricsv3.RegisterMetricsServiceServer`) whose `StreamMetrics(stream)` loops `stream.Recv()` accumulating every received `MetricFamily` across messages keyed by name (LAST-seen value) + capturing the `identifier.node`, then `stream.SendAndClose(&StreamMetricsResponse{})`. Surface: `New(t)`/`NewAtAddr(addr)`/`Family(name) (value float64, typ, ok)`/`Count()`/`Node()`/`Reset()`/`Addr()`/`Stop()`. Shared-bridge + receiver-hostname reachability copies `0087`/`0088` (h2c — `http2_protocol_options{}`).

**D-MS-REJECT-COVERAGE → ALL boot-rejects are SUBJECT unit tests; NO new fixture dirs (fixtures 90 → 91, only `0089`).** The envoy-go-strict rejects (sibling TypeURLs / `report_counters_as_deltas:true` / `emit_tags_as_labels:true` / non-default `histogram_emit_mode` / `google_grpc` / empty `cluster_name` / `stats_flush_on_admin` / non-V3 `transport_api_version`) live in `bootstrap_test.go` (parse-level — the reference BOOTS the strict ones, so they are NOT cross-side-informative; the reference-parity ones are cheaper as units). The missing/non-H2-cluster `log.Fatalf` is covered by a `grpcclient` `MetricsServiceClient` unit test (the `ALSClient` `DialContext`-gate precedent). The 46.2 posture verbatim (`reference_differential_fixture_dispatch_constraint` — a reject dir buys nothing the unit doesn't).

**D-MS-FUZZER → land `FuzzStatsSinkConfigParse` at 47.1 (fuzzers 49 → 50).** The `stats_sinks[]`/`MetricsServiceConfig` parse is an untrusted bootstrap-config boundary (the 45.1 `FuzzOpenTelemetryAccessLogConfig` parse-fuzzer precedent). The `Registry`→`MetricFamily` mapping consumes TRUSTED registry-validated names — NO mapping fuzzer. Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l == 50` at the completion task, reconciling the documented-vs-actual count (`reference_fuzzer_count_docs_drift` — **49 actual** at baseline).

**D-MS-FINAL-FLUSH → NO special final flush at 47.1.** On `ctx.Done()` the `Flusher` stops the ticker and returns; the sink's `Close()` drains the in-flight stream (`CloseAndRecv`) + cancels the stream ctx. A drain-flush is UNasserted (the `0089` differential polls a steady-state converged flush, not a drain), so it adds complexity with no proven cross-side value; deferred to 47.2 if a reference re-probe shows it load-bearing.

---

## File structure (decomposition locked here)

**Production (created):**
- `internal/statssink/mapping.go` — `snapshot(reg *stats.Registry, nowMs int64) []*dto.MetricFamily` (Counter→COUNTER absolute, Gauge→GAUGE, full dotted `Name()`, ZERO `LabelPair`, `TimestampMs` set, no `Help`; type-switch on `*stats.Counter`/`*stats.Gauge`).
- `internal/statssink/sink.go` — `type Sink interface{ Submit(batch []*dto.MetricFamily); Close() error }`; `type MetricsServiceSink struct{...}` (bounded chan + writer goroutine + identifier-once + reconnect-resend + idempotent `Close` — the `GrpcAccessLogSink` template, SIMPLIFIED); `NewMetricsServiceSink(client metricsClient, node *corev3.Node) *MetricsServiceSink` + `newSinkWithCapacity(...)`; the `metricsClient`/`metricsStream` seam interfaces.
- `internal/statssink/flusher.go` — `type Flusher struct{ reg *stats.Registry; interval time.Duration; sinks []Sink; nowMs func() int64 }`; `NewFlusher(reg, interval, sinks) *Flusher`; `Start(ctx context.Context)` (a `time.NewTicker(interval)` loop, `flushOnce()` per tick, stop on `ctx.Done()`); `flushOnce()` (`snapshot(...)` → each `sink.Submit(batch)`).

**Production (modified):**
- `internal/grpcclient/grpcclient.go` — the `metricsv3` import + the `MetricsServiceClient` type + `NewMetricsServiceClient` + `StreamMetrics` + `Close` (the `ALSClient` shape).
- `internal/bootstrap/bootstrap.go` — the `config/metrics/v3` blank-import; the `metricsServiceTypeURL` const; the `StatsSinkConfig` struct + `Bootstrap.StatsSinkConfigs` field + `Bootstrap.FlushInterval` field; `parseStatsSinks(bs, result)` (called from `Load`) + `parseMetricsServiceConfig(tc, idx)` + the strict-reject arms.
- `cmd/envoy-go/main.go` — the metrics-service sink build (Dialer reuse/hoist) + the post-Freeze `Flusher.Start(ctx)` + the `Flusher`/sink `Close` joined to the defer-LIFO.
- `go.mod` / `go.sum` — `github.com/prometheus/client_model v0.6.1` (direct; lands at Task 2 with the first `dto` import).

**Test (created):**
- `internal/statssink/mapping_test.go`, `internal/statssink/sink_test.go` (`-race`), `internal/statssink/flusher_test.go` (`-race`).
- `internal/grpcclient/grpcclient_test.go` (MODIFY: the `MetricsServiceClient` tests).
- `internal/bootstrap/bootstrap_test.go` (MODIFY: the parse-accept + strict-reject arms + the inert-interval byte-stability guard + the no-new-stat registration guard).
- `internal/bootstrap/statssink_fuzz_test.go` (`FuzzStatsSinkConfigParse`).
- `test/helpers/metricsservice/metricsservice.go` (+ `_test.go`).
- `test/fixtures/0089-stats-sink-metrics-service/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`.
- `test/differential/runner_test.go` (MODIFY: blank-import the `0089` driver).

**Docs (completion task):**
- `docs/envoy-go/phases/47-stats-sink-metrics-service/PROGRESS-47.1.md`, `docs/envoy-go/DECISIONS.md` (ADR-0262 §Decision/§Consequences — ANCHORS the leg), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` (**row 47 STAYS `in-progress`**).

---

## Task 1: Phase scaffolding — PROGRESS-47.1.md + baselines + the final ADR-0045 split re-check (D-MS-SPLIT)

**Files:**
- Create: `docs/envoy-go/phases/47-stats-sink-metrics-service/PROGRESS-47.1.md`

- [ ] **Step 1: Record the baseline counts** (verbatim outputs in PROGRESS-47.1.md):
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/*/ | wc -l                                   # expect 90 (tail 0088-tracing-zipkin)
grep -rh '^func Fuzz' --include='*.go' . | wc -l                 # expect 49
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go # expect = 38 (BackendKind tail)
grep -c 'prometheus/client_model' go.mod go.sum                  # expect 0 0 (NOT present — 47.1 lands it)
grep 'go-control-plane/envoy' go.mod                             # expect v1.32.4 (already direct)
grep -rn 'statssink\|MetricsServiceClient\|StatsSinkConfig\|metricsServiceTypeURL' internal/ cmd/ --include='*.go'  # expect: NONE (47.1 introduces them)
```
Baseline: stat surface **1200** (H2 cluster; non-H2 **1196**) / fixtures **90** / fuzzers **49** / BackendKind **38** / DECISIONS tail **ADR-0261** (next-free **ADR-0262**).

- [ ] **Step 2: Write the PROGRESS-47.1.md scaffold** — a header (phase 47.1 IMPL, the SPEC-47.1 reference + the "FIRST stats-export sink + FIRST `stats_sinks[]` consumer + FIRST new go.mod module in the Observability family; ANCHORS ADR-0262; row 47 STAYS in-progress" note, the worktree branch), a task checklist mirroring this plan, the baseline block, the **D-MS-SPLIT confirmation (NO sub-split — §3.0)**, and the anticipated exit counts: stat **1200** (N=0 — D-MS-STATS-FINAL) / fixtures **91** (`0089-stats-sink-metrics-service`) / fuzzers **50** (`FuzzStatsSinkConfigParse`) / BackendKind **38** (driver-owned receiver) / DECISIONS **ADR-0262** / **1 new package** (`internal/statssink`) + **1 new go.mod module** (`prometheus/client_model v0.6.1`).

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/47-stats-sink-metrics-service/PROGRESS-47.1.md
git commit -m "phase 47.1 Task 1: PROGRESS scaffold + baselines + ADR-0045 NO-sub-split re-check (metrics_service stats sink; ANCHORS ADR-0262; row 47 in-progress)"
```

---

## Task 2: The `Counter`/`Gauge` → `MetricFamily` mapping (`internal/statssink/mapping.go`) + the `client_model` module landing [TDD, table-driven]

**Files:**
- Create: `internal/statssink/mapping.go`, `internal/statssink/mapping_test.go`
- Modify: `go.mod` / `go.sum` (land `github.com/prometheus/client_model v0.6.1` — the FIRST `dto` import is here)

The mapping is a PURE function — no gRPC, no goroutine — the cleanest unit to TDD first, and the natural home for the first `dto` import (so `go mod tidy` keeps the module). Creates the NEW `internal/statssink` package.

- [ ] **Step 1: Write the failing table tests** in `mapping_test.go` (use a fresh `stats.NewRegistry()` per sub-test — `NewCounter` PANICS on a duplicate static name):
  - register `c := reg.NewCounter("cluster.backend.upstream_rq_total"); c.Add(7)` + `g := reg.NewGauge("cluster.backend.membership_healthy"); g.Set(3)` (look up the gauge constructor name in `registry.go`), then `fams := snapshot(reg, 1782734137801)`:
    - the COUNTER family: `GetName() == "cluster.backend.upstream_rq_total"`, `GetType() == dto.MetricType_COUNTER`, `GetHelp() == ""`, exactly ONE `Metric`, `Metric[0].GetCounter().GetValue() == 7.0`, `Metric[0].GetTimestampMs() == 1782734137801`, `len(Metric[0].GetLabel()) == 0`.
    - the GAUGE family: `GetType() == dto.MetricType_GAUGE`, `Metric[0].GetGauge().GetValue() == 3.0`, `TimestampMs == 1782734137801`, ZERO labels.
    - registration ORDER preserved (the `Walk` order): the families appear in the order the metrics were registered.
  - an EMPTY registry ⇒ `snapshot(...)` returns an empty (nil or len-0) slice.
  - a negative gauge (`g.Set(-5)`) ⇒ `GetGauge().GetValue() == -5.0` (Gauge is signed).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/statssink/ -run 'TestSnapshot' -count=1` ⇒ FAIL (package/func undefined). (If `go test` errors that `client_model` is missing, that is expected — Step 3 lands it.)

- [ ] **Step 3: Implement** `mapping.go`:
```go
package statssink

import (
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"

	"github.com/esalaine/envoy-go/internal/stats"
)

// snapshot walks the frozen registry and maps each Counter/Gauge to a
// cumulative/no-labels io.prometheus.client.MetricFamily (the metrics_service
// default mapping, ADR-0262): the FULL dotted Name() (tags inlined), the
// absolute Load() value, ZERO LabelPairs (emit_tags_as_labels=false), a
// flush-time TimestampMs, no Help. Histograms have no envoy-go analog
// (ADR-0060), so only COUNTER/GAUGE families are produced.
func snapshot(reg *stats.Registry, nowMs int64) []*dto.MetricFamily {
	var out []*dto.MetricFamily
	reg.Walk(func(m stats.Metric) {
		switch v := m.(type) {
		case *stats.Counter:
			out = append(out, &dto.MetricFamily{
				Name: proto.String(v.Name()),
				Type: dto.MetricType_COUNTER.Enum(),
				Metric: []*dto.Metric{{
					Counter:     &dto.Counter{Value: proto.Float64(float64(v.Load()))},
					TimestampMs: proto.Int64(nowMs),
				}},
			})
		case *stats.Gauge:
			out = append(out, &dto.MetricFamily{
				Name: proto.String(v.Name()),
				Type: dto.MetricType_GAUGE.Enum(),
				Metric: []*dto.Metric{{
					Gauge:       &dto.Gauge{Value: proto.Float64(float64(v.Load()))},
					TimestampMs: proto.Int64(nowMs),
				}},
			})
		}
	})
	return out
}
```

- [ ] **Step 4: Land the module + run the tests**
```bash
go get github.com/prometheus/client_model@v0.6.1
go mod tidy
grep 'prometheus/client_model' go.mod          # expect: in the DIRECT require block, v0.6.1
go mod tidy -diff                              # expect EMPTY (clean)
go test ./internal/statssink/ -run 'TestSnapshot' -count=1   # expect PASS
```
Expected: `go get` + `go mod tidy` add `github.com/prometheus/client_model v0.6.1` to the direct require block (the SOLE new module — the metrics service/config protos resolve at the already-direct `go-control-plane/envoy v1.32.4`); the tests PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/mapping.go internal/statssink/mapping_test.go go.mod go.sum
git commit -m "phase 47.1 Task 2: Counter/Gauge -> MetricFamily mapping (cumulative/no-labels, full dotted name, timestamp_ms) + land prometheus/client_model v0.6.1 (the FIRST Observability-family module; D-MS-MODULE)"
```

---

## Task 3: The `MetricsServiceClient` CLIENT-streaming typed wrapper (`internal/grpcclient/grpcclient.go`) [TDD, the `ALSClient` shape]

**Files:**
- Modify: `internal/grpcclient/grpcclient.go`
- Test: `internal/grpcclient/grpcclient_test.go`

`StreamMetrics` is CLIENT-streaming (`Send` many → `CloseAndRecv` one) — EXACTLY the `ALSClient` `StreamAccessLogs` shape, NOT the unary OTLP wrappers. Mirror `ALSClient` (`grpcclient.go:261`) verbatim.

- [ ] **Step 1: Write the failing tests** in `grpcclient_test.go` (mirror the existing `ALSClient` tests — find them; reuse the test-cluster-manager builders):
  - **nil-dialer**: `NewMetricsServiceClient(nil, "c")` ⇒ `(nil, err)` naming the cluster.
  - **unknown-cluster**: a `Dialer` over a manager with no `c` ⇒ `NewMetricsServiceClient(d, "c")` errors `unknown cluster` (the `DialContext` gate).
  - **non-H2-cluster**: a cluster without `http2_protocol_options{}` ⇒ errors about `http2_protocol_options` (the `UseH2()` gate).
  - **Close-idempotent**: against a valid H2 cluster, `c.Close()` twice returns the same (nil) error — no panic.
  - **StreamMetrics-opens**: against an in-process bare `grpc.NewServer()` + `metricsv3.RegisterMetricsServiceServer(srv, &stub{})` (a no-op `UnimplementedMetricsServiceServer`-embedding stub, OR defer to Task 8's helper), `s, err := c.StreamMetrics(ctx)` returns a non-nil stream + nil error; `s.Send(&metricsv3.StreamMetricsMessage{})` then `s.CloseAndRecv()` returns a non-nil response + nil error.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/grpcclient/ -run 'TestMetricsService' -count=1` ⇒ FAIL (`MetricsServiceClient`/`NewMetricsServiceClient` undefined).

- [ ] **Step 3: Implement** in `grpcclient.go` (add `metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"` to the imports — the FIRST direct `service/metrics/v3` import; resolves at the already-direct go-control-plane), mirroring `ALSClient`:
```go
// ----------------------------------------------------------------------------
// MetricsServiceClient — the typed MetricsService/StreamMetrics CLIENT-STREAMING
// wrapper (ADR-0262). StreamMetrics is client-streaming exactly like ALS
// StreamAccessLogs (Send many -> CloseAndRecv one), so this mirrors ALSClient
// (NOT the unary OTLP wrappers). One *MetricsServiceClient per metrics_service
// sink (cluster_name), owned by the MetricsServiceSink and Close()d at sink close.
type MetricsServiceClient struct {
	conn   *grpc.ClientConn
	stub   metricsv3.MetricsServiceClient
	target string // cluster_name — for logs/errors

	closeOnce sync.Once
	closeErr  error
}

// NewMetricsServiceClient dials the named cluster via d.DialContext and wraps the
// resulting *grpc.ClientConn in a typed MetricsServiceClient. On dial error
// returns (nil, err) verbatim (already cluster-named via DialContext). The
// NewALSClient shape.
func NewMetricsServiceClient(d *Dialer, clusterName string) (*MetricsServiceClient, error) {
	if d == nil {
		return nil, fmt.Errorf("grpcclient: new metrics service client %q: dialer is nil", clusterName)
	}
	conn, err := d.DialContext(context.Background(), clusterName)
	if err != nil {
		return nil, err
	}
	return &MetricsServiceClient{
		conn:   conn,
		stub:   metricsv3.NewMetricsServiceClient(conn),
		target: clusterName,
	}, nil
}

// StreamMetrics opens the client-streaming StreamMetrics RPC. The sink's writer
// goroutine bounds ctx; the returned stream is Send()-many then CloseAndRecv()-once.
func (c *MetricsServiceClient) StreamMetrics(ctx context.Context) (metricsv3.MetricsService_StreamMetricsClient, error) {
	if c == nil || c.stub == nil {
		return nil, errors.New("grpcclient: StreamMetrics: nil MetricsServiceClient / stub")
	}
	return c.stub.StreamMetrics(ctx)
}

// Close releases the underlying *grpc.ClientConn. Idempotent (sync.Once), the
// ALSClient.Close shape.
func (c *MetricsServiceClient) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.conn != nil {
			c.closeErr = c.conn.Close()
		}
	})
	return c.closeErr
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/grpcclient/ -run 'TestMetricsService' -count=1` ⇒ PASS; then `go mod tidy -diff` ⇒ EMPTY (the service proto pulled `client_model` transitively, already direct from Task 2).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/grpcclient/ && golangci-lint run ./internal/grpcclient/... && go vet ./internal/grpcclient/... && go build ./...
git add internal/grpcclient/grpcclient.go internal/grpcclient/grpcclient_test.go
git commit -m "phase 47.1 Task 3: MetricsServiceClient CLIENT-streaming typed wrapper over grpcclient.Dialer (the ALSClient precedent, ADR-0158/ADR-0262)"
```

---

## Task 4: The `MetricsServiceSink` (`internal/statssink/sink.go`) [TDD, `-race`]

**Files:**
- Create: `internal/statssink/sink.go`, `internal/statssink/sink_test.go`

The ALS `GrpcAccessLogSink` client-streaming lifecycle (`internal/accesslog/grpcsink.go`), SIMPLIFIED: a bounded channel of complete batches, a writer goroutine, identifier-once-per-stream, reconnect-resend-once, drop-on-full, idempotent `Close` — but NO size/interval accumulation (the `Flusher` is the trigger; each `Submit` is one full batch). Test against a FAKE `metricsClient` (the `alsClient`-seam precedent).

- [ ] **Step 1: Write the failing tests** in `sink_test.go` (a fake `metricsClient` returning a fake `MetricsService_StreamMetricsClient` recording every `Send`; follow `internal/accesslog/grpcsink_test.go` for how it stubs the generated stream interface — embed the interface + override `Send`/`CloseAndRecv`):
  - **identifier-once**: `Submit(batch1)` then `Submit(batch2)`; after both drain, the fake stream recorded message #1 with a non-nil `Identifier{Node}` (`node.Id`/`node.Cluster` == the configured node) + `EnvoyMetrics == batch1`, and message #2 with `Identifier == nil` + `EnvoyMetrics == batch2`. (Drive draining deterministically — e.g. a small capacity + a `Close()` that waits for the writer, OR a synchronization hook in the fake.)
  - **reconnect-resend**: the fake's first stream errors on the 2nd `Send`; assert the sink opens a SECOND stream, re-sends the `Identifier` on it, and re-sends the failed batch (the `flush` retry-once shape).
  - **drop-on-full**: with a fake whose `Send` blocks, `Submit` MANY batches ⇒ `Submit` never blocks (returns immediately once the channel is full — the `select … default` drop path); no panic.
  - **Close-idempotent**: `Close()` twice ⇒ same (nil) error, no panic; `Close` drains the in-flight stream (`CloseAndRecv` called) + calls `client.Close()`.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/statssink/ -run 'TestSink' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `sink.go` (the `grpcsink.go` template — port `Submit`/`Close`/`run`/`flush`; drop the buffer-accumulation fields):
```go
package statssink

import (
	"context"
	"log"
	"sync"
	"sync/atomic"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
	dto "github.com/prometheus/client_model/go"
)

// Sink consumes one complete MetricFamily batch per flush.
type Sink interface {
	Submit(batch []*dto.MetricFamily)
	Close() error
}

// metricsClient is the gRPC seam (the alsClient-seam precedent) — *grpcclient.
// MetricsServiceClient satisfies it; sink_test.go fakes it.
type metricsClient interface {
	StreamMetrics(ctx context.Context) (metricsv3.MetricsService_StreamMetricsClient, error)
	Close() error
}

const (
	defaultChannelCapacity = 8
	closeDrainGrace        = 2 * time.Second
	dropLogIntervalNanos   = int64(time.Second)
)

type MetricsServiceSink struct {
	ch          chan []*dto.MetricFamily
	client      metricsClient
	node        *corev3.Node
	done        chan struct{}
	closeOnce   sync.Once
	closeErr    error
	lastDropLog atomic.Int64
	ctx         context.Context
	cancel      context.CancelFunc
}

func NewMetricsServiceSink(client metricsClient, node *corev3.Node) *MetricsServiceSink {
	return newSinkWithCapacity(client, node, defaultChannelCapacity)
}

func newSinkWithCapacity(client metricsClient, node *corev3.Node, capacity int) *MetricsServiceSink {
	s := &MetricsServiceSink{
		ch:     make(chan []*dto.MetricFamily, capacity),
		client: client,
		node:   node,
		done:   make(chan struct{}),
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
	go s.run()
	return s
}

func (s *MetricsServiceSink) Submit(batch []*dto.MetricFamily) {
	select {
	case s.ch <- batch:
	default:
		now := time.Now().UnixNano()
		last := s.lastDropLog.Load()
		if now-last >= dropLogIntervalNanos && s.lastDropLog.CompareAndSwap(last, now) {
			log.Printf("statssink: metrics_service channel full, dropping flush batch")
		}
	}
}

func (s *MetricsServiceSink) Close() error {
	s.closeOnce.Do(func() {
		close(s.ch)
		select {
		case <-s.done:
		case <-time.After(closeDrainGrace):
			s.cancel()
			<-s.done
		}
		s.cancel()
		s.closeErr = s.client.Close()
	})
	return s.closeErr
}

// run is the writer goroutine: lazily open the stream, send the identifier once
// per stream, send each batch as one StreamMetricsMessage, reopen-and-resend
// once on a Send error, drain + CloseAndRecv on channel close.
func (s *MetricsServiceSink) run() {
	defer close(s.done)
	var stream metricsv3.MetricsService_StreamMetricsClient
	var sentIdentifier bool

	// flush sends batch on the live stream, opening it (and re-sending the
	// identifier) up to twice (initial + one reconnect). On a second failure the
	// batch is dropped (logged, not counted — D-MS-STATS-FINAL).
	flush := func(batch []*dto.MetricFamily) {
		for attempt := 0; attempt < 2; attempt++ {
			if stream == nil {
				st, err := s.client.StreamMetrics(s.ctx)
				if err != nil {
					log.Printf("statssink: metrics_service stream open: %v", err)
					return
				}
				stream, sentIdentifier = st, false
			}
			msg := &metricsv3.StreamMetricsMessage{EnvoyMetrics: batch}
			if !sentIdentifier {
				msg.Identifier = &metricsv3.StreamMetricsMessage_Identifier{Node: s.node}
			}
			if err := stream.Send(msg); err != nil {
				_, _ = stream.CloseAndRecv()
				stream = nil // reopen on the next attempt
				continue
			}
			sentIdentifier = true
			return
		}
		log.Printf("statssink: metrics_service flush dropped after reconnect")
	}

	for batch := range s.ch {
		flush(batch)
	}
	if stream != nil {
		_, _ = stream.CloseAndRecv()
	}
}
```
NOTE: confirm the generated stream interface name `MetricsService_StreamMetricsClient` and that `msg.Identifier`/`msg.EnvoyMetrics` are the exact field names by reading the generated `service/metrics/v3` package (the orientation pins them; re-verify).

- [ ] **Step 4: Run to verify they pass + race** — `go test ./internal/statssink/ -run 'TestSink' -count=1` ⇒ PASS; then the FULL package race: `go test ./internal/statssink/ -race -count=1` ⇒ PASS (the writer goroutine is a background mutator — `reference_full_suite_race_after_background_mutator`).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/sink.go internal/statssink/sink_test.go
git commit -m "phase 47.1 Task 4: MetricsServiceSink — bounded-channel client-streaming sink (identifier-once + reconnect-resend + drop-on-full + idempotent Close; the GrpcAccessLogSink template, 1-deep handoff; D-MS-SINK-BUFFER)"
```

---

## Task 5: The `Flusher` (`internal/statssink/flusher.go`) [TDD, `-race`]

**Files:**
- Create: `internal/statssink/flusher.go`, `internal/statssink/flusher_test.go`

The NEW bootstrap-level subsystem: a `stats_flush_interval` ticker that `snapshot`s the frozen registry per tick and `Submit`s the batch to each sink. Started AFTER `Freeze` (the `Walk` snapshot is lock-clean — ADR-0059). NO final flush on `ctx.Done()` (D-MS-FINAL-FLUSH).

- [ ] **Step 1: Write the failing tests** in `flusher_test.go`:
  - **flushOnce**: a registry with one counter (`c.Add(5)`) + a fake `Sink` recording every `Submit`; `NewFlusher(reg, time.Hour, []Sink{fake}).flushOnce()` ⇒ the fake received exactly one batch whose single COUNTER family has value 5 (drive `nowMs` via the injectable `f.nowMs` so the timestamp is deterministic).
  - **Start ticks then stops on ctx cancel**: `NewFlusher(reg, 10*time.Millisecond, []Sink{fake})`; run `Start(ctx)` in a goroutine; after a few ticks the fake has `>= 2` batches; `cancel()` ⇒ `Start` returns promptly and the batch count STOPS growing (read it twice with a gap). Use a mutex-guarded fake or atomic counters (the `-race` gate is below).
  - **multi-sink fan-out**: two fake sinks both receive every batch.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/statssink/ -run 'TestFlusher' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `flusher.go`:
```go
package statssink

import (
	"context"
	"time"

	"github.com/esalaine/envoy-go/internal/stats"
)

// Flusher snapshots the frozen process-global Registry every interval and
// Submits the batch to each sink. Started AFTER bs.Stats.Freeze() so Walk is
// lock-clean (ADR-0059). One full-registry snapshot per tick (NOT deltas).
type Flusher struct {
	reg      *stats.Registry
	interval time.Duration
	sinks    []Sink
	nowMs    func() int64 // injectable for tests; defaults to wall-clock ms
}

func NewFlusher(reg *stats.Registry, interval time.Duration, sinks []Sink) *Flusher {
	return &Flusher{
		reg:      reg,
		interval: interval,
		sinks:    sinks,
		nowMs:    func() int64 { return time.Now().UnixMilli() },
	}
}

// Start runs the flush loop until ctx is cancelled. No final flush on Done
// (D-MS-FINAL-FLUSH); the sinks' Close drains any in-flight stream.
func (f *Flusher) Start(ctx context.Context) {
	t := time.NewTicker(f.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			f.flushOnce()
		}
	}
}

func (f *Flusher) flushOnce() {
	batch := snapshot(f.reg, f.nowMs())
	for _, s := range f.sinks {
		s.Submit(batch)
	}
}
```

- [ ] **Step 4: Run to verify they pass + race** — `go test ./internal/statssink/ -run 'TestFlusher' -count=1` ⇒ PASS; then `go test ./internal/statssink/ -race -count=1` ⇒ PASS (FULL package — the ticker goroutine + the sink writer goroutine are background mutators).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/statssink/ && golangci-lint run ./internal/statssink/... && go vet ./internal/statssink/... && go build ./...
git add internal/statssink/flusher.go internal/statssink/flusher_test.go
git commit -m "phase 47.1 Task 5: Flusher — the stats_flush_interval ticker walking the frozen Registry per tick (no final-flush on ctx.Done; D-MS-FINAL-FLUSH)"
```

---

## Task 6: The `stats_sinks[]`/`stats_flush_interval` parse arm + strict-rejects (`internal/bootstrap`) + `FuzzStatsSinkConfigParse` [TDD + fuzz]

**Files:**
- Modify: `internal/bootstrap/bootstrap.go`, `internal/bootstrap/bootstrap_test.go`
- Create: `internal/bootstrap/statssink_fuzz_test.go`

Lift the `metrics_service` `stats_sinks[]` TypeURL from the ADR-0064 silent-ignore set; parse `stats_flush_interval` (default 5s); strict-reject the knob values + sibling sinks + `stats_flush_on_admin` (ADR-0080). `StatsSinkConfig` lives HERE (D-MS-CONFIG-HOME).

- [ ] **Step 1: Write the failing tests** in `bootstrap_test.go` (build a bootstrap YAML/proto with a `stats_sinks[]` entry whose `typed_config` is a `MetricsServiceConfig` via `anypb.New`):
  - **accept**: `metrics_service` with `grpc_service.envoy_grpc.cluster_name:"mc"`, default knobs ⇒ `len(bs.StatsSinkConfigs) == 1`, `StatsSinkConfigs[0].ClusterName == "mc"`; with `stats_flush_interval: 2s` ⇒ `bs.FlushInterval == 2*time.Second`; ABSENT `stats_flush_interval` ⇒ `bs.FlushInterval == 5*time.Second` (D-MS-FLUSH default).
  - **accept `transport_api_version`**: `AUTO` (omitted) ⇒ accept; `V3` ⇒ accept (D-MS-APIVERSION).
  - **strict-rejects** (each its own arm; assert the error message names the offending field/value — `reference_strict_reject_sibling_typeurl_gap`):
    - `report_counters_as_deltas: {value: true}` ⇒ error (`report_counters_as_deltas`); absent/`false` ⇒ accept.
    - `emit_tags_as_labels: true` ⇒ error.
    - `histogram_emit_mode: SUMMARY` (non-default) ⇒ error.
    - `transport_api_version: V2` ⇒ error (reference-parity).
    - `grpc_service.google_grpc{...}` (no `envoy_grpc`) ⇒ error.
    - empty `envoy_grpc.cluster_name` ⇒ error.
    - a SIBLING sink TypeURL (a `StatsdSink` `typed_config`) ⇒ error naming the unsupported TypeUrl + the supported `metrics_service` one.
    - top-level `stats_flush_on_admin: true` ⇒ error (deferred).
  - **inert interval (byte-stability, D-MS-FLUSH-INERT)**: a bootstrap with `stats_flush_interval: 3s` but NO `stats_sinks[]` ⇒ `len(bs.StatsSinkConfigs) == 0` and `bs.FlushInterval == 3*time.Second` (parsed-and-inert; no error).
  - the existing access-log/grpc tests STILL PASS (the new parse pass is additive).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/bootstrap/ -run 'TestStatsSink' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** in `bootstrap.go`:
  - Add the blank-import `_ "github.com/envoyproxy/go-control-plane/envoy/config/metrics/v3"` to the registry block (ADR-0016) so the `MetricsServiceConfig`/`StatsSink` descriptors resolve, AND a named import `metricsconfigv3 "…/config/metrics/v3"` for the typed parse.
  - Derive the TypeURL from the proto descriptor (NOT a blind literal — `reference_network_filter_typeurl_extensions`): `var metricsServiceTypeURL = "type.googleapis.com/" + string((&metricsconfigv3.MetricsServiceConfig{}).ProtoReflect().Descriptor().FullName())`. Add a test asserting `metricsServiceTypeURL == "type.googleapis.com/envoy.config.metrics.v3.MetricsServiceConfig"` (the SPEC §11 live-verified string) — this catches a proto-package rename.
  - Add the parsed struct + the `Bootstrap` fields:
    ```go
    type StatsSinkConfig struct {
        ClusterName string // MetricsServiceConfig.grpc_service.envoy_grpc.cluster_name
    }
    // on Bootstrap: StatsSinkConfigs []StatsSinkConfig ; FlushInterval time.Duration
    ```
  - In `Load`, after `parseAccessLogConfigs(bs, result)`, call `parseStatsSinks(bs, result)`.
  - `parseStatsSinks(bs *bootstrapv3.Bootstrap, result *Bootstrap) error`:
    - `result.FlushInterval = 5 * time.Second; if d := bs.GetStatsFlushInterval(); d != nil { if v := d.AsDuration(); v > 0 { result.FlushInterval = v } }`.
    - `if bs.GetStatsFlushOnAdmin() { return fmt.Errorf("bootstrap: stats_flush_on_admin is not supported (envoy-go ships only the periodic stats_flush_interval sink loop)") }`.
    - for `i, sink := range bs.GetStatsSinks()`: dispatch `tc := sink.GetTypedConfig()`; if `tc == nil` → reject (a stats_sink without typed_config is malformed); if `tc.GetTypeUrl() == metricsServiceTypeURL` → `parseMetricsServiceConfig(tc, i, result)`; ELSE → `return fmt.Errorf("bootstrap: stats_sinks[%d]: unsupported sink type %q (envoy-go supports only the metrics_service sink %q)", i, tc.GetTypeUrl(), metricsServiceTypeURL)` (an EXPLICIT reject naming the offending sibling — NOT a silent slip-through; satisfies `reference_strict_reject_sibling_typeurl_gap`).
  - `parseMetricsServiceConfig(tc *anypb.Any, idx int, result *Bootstrap) error`: `var msc metricsconfigv3.MetricsServiceConfig; if err := tc.UnmarshalTo(&msc); err != nil { return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service typed_config: %w", idx, err) }`, then:
    - `if v := msc.GetTransportApiVersion(); v != corev3.ApiVersion_AUTO && v != corev3.ApiVersion_V3 { return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service transport_api_version %v is not supported (envoy-go is V3-only)", idx, v) }` (D-MS-APIVERSION).
    - `eg := msc.GetGrpcService().GetEnvoyGrpc(); if eg == nil { return …("requires grpc_service.envoy_grpc (google_grpc is not supported)") }; if eg.GetClusterName() == "" { return …("grpc_service.envoy_grpc.cluster_name is required") }`.
    - `if d := msc.GetReportCountersAsDeltas(); d != nil && d.GetValue() { return …("report_counters_as_deltas:true is not supported (47.1 emits cumulative absolute values; deferred to 47.2)") }`.
    - `if msc.GetEmitTagsAsLabels() { return …("emit_tags_as_labels:true is not supported (47.1 emits the full dotted name with zero labels; deferred to 47.2)") }`.
    - `if msc.GetHistogramEmitMode() != metricsconfigv3.HistogramEmitMode_SUMMARY_AND_HISTOGRAM { return …("histogram_emit_mode … is not supported (envoy-go has no histograms)") }`. NOTE: the enum type is the TOP-LEVEL `metricsconfigv3.HistogramEmitMode` (constant `HistogramEmitMode_SUMMARY_AND_HISTOGRAM`), NOT nested under `MetricsServiceConfig` — re-confirm by reading the generated proto.
    - `result.StatsSinkConfigs = append(result.StatsSinkConfigs, StatsSinkConfig{ClusterName: eg.GetClusterName()})`.
  - (Confirm `corev3` is already imported in `bootstrap.go`; the orientation shows `corev3.ApiVersion_V2` already used in `parseCommonGrpcAccessLogConfig`.)

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/bootstrap/ -run 'TestStatsSink' -count=1` ⇒ PASS; then the FULL package `go test ./internal/bootstrap/ -count=1` ⇒ PASS (no regression).

- [ ] **Step 5: Write `FuzzStatsSinkConfigParse`** in `statssink_fuzz_test.go` (the `FuzzOpenTelemetryAccessLogConfig` shape — find it for the harness): seed a handful of valid + malformed bootstrap YAML/JSON documents carrying `stats_sinks[]`/`stats_flush_interval` (including a metrics_service accept, each reject arm, a sibling sink, garbage typed_config bytes); the fuzz body feeds the corpus bytes to `Load(bytes.NewReader(b))` and asserts NO panic (a returned error is fine). Run `go test ./internal/bootstrap/ -run 'FuzzStatsSinkConfigParse' -count=1` (seed pass) ⇒ PASS; then `go test ./internal/bootstrap/ -run '^$' -fuzz 'FuzzStatsSinkConfigParse' -fuzztime 15s` ⇒ no crash.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/bootstrap/ && golangci-lint run ./internal/bootstrap/... && go vet ./internal/bootstrap/... && go build ./... && go mod tidy -diff
git add internal/bootstrap/bootstrap.go internal/bootstrap/bootstrap_test.go internal/bootstrap/statssink_fuzz_test.go
git commit -m "phase 47.1 Task 6: stats_sinks[] metrics_service parse arm + stats_flush_interval (5s default) + strict-rejects (knob values + sibling TypeURLs + google_grpc + empty cluster + non-V3 + stats_flush_on_admin) + FuzzStatsSinkConfigParse (fuzzers 49->50; D-MS-FUZZER/APIVERSION/FLUSH-INERT)"
```

---

## Task 7: Boot wiring (`cmd/envoy-go/main.go`) — post-`Freeze` `Flusher.Start` + LIFO Close + byte-stability + the no-new-stat registration guard

**Files:**
- Modify: `cmd/envoy-go/main.go`
- Modify: `internal/bootstrap/bootstrap_test.go` (the +0 registration guard — OR a small `cmd/envoy-go` test; see Step 3)

main.go is not unit-tested in isolation (the `0089` differential is its behavioral proof); the gate here is build + boot-smoke. The `Flusher.Start(ctx)` MUST run AFTER `bs.Stats.Freeze()` (`:325`).

- [ ] **Step 1: Implement** in `main.go`:
  - The `dialer := grpcclient.New(cm)` already exists UNCONDITIONALLY (`:131`) — REUSE it (no hoist needed; it is built before the sink block).
  - In the access-log sink-build region (near `:147`–`:172`), BUILD the metrics-service sink + Flusher when configured, but do NOT `Start` it yet — stash the `*statssink.Flusher` + a metrics-sink closer slice in the outer scope (e.g. `var statsFlusher *statssink.Flusher` + `var statsSinks []statssink.Sink`):
    ```go
    // Phase 47.1 (ADR-0262): the metrics_service stats sink. Built when a
    // stats_sinks[] metrics_service entry is present; Start()ed AFTER Freeze.
    if len(bs.StatsSinkConfigs) > 0 {
        node := &corev3.Node{Id: bs.Proto.GetNode().GetId(), Cluster: bs.Proto.GetNode().GetCluster()}
        for _, cfg := range bs.StatsSinkConfigs {
            client, err := grpcclient.NewMetricsServiceClient(dialer, cfg.ClusterName)
            if err != nil {
                log.Fatalf("statssink: metrics_service client for cluster %q: %v", cfg.ClusterName, err)
            }
            statsSinks = append(statsSinks, statssink.NewMetricsServiceSink(client, node))
        }
        statsFlusher = statssink.NewFlusher(bs.Stats, bs.FlushInterval, statsSinks)
    }
    ```
    **DO NOT append to the existing `sinks` slice.** That slice is typed `[]accesslog.Sink` (interface `Submit(r any)` + `Close() error`, `internal/accesslog/accesslog.go:18`); `*statssink.MetricsServiceSink` has `Submit(batch []*dto.MetricFamily)`, so it does NOT satisfy `accesslog.Sink` (a Submit-signature mismatch — it will NOT compile in that slice). Instead close the metrics sinks via their OWN defer (they satisfy `io.Closer`): add a dedicated `defer func(){ for _, s := range statsSinks { _ = s.Close() } }()` placed so it runs in the shutdown LIFO (after the Flusher's ctx is cancelled — the `Close` drains the in-flight stream).
  - AFTER `bs.Stats.Freeze()` (`:325`), alongside `cm.StartHealthChecks`/`cm.StartOutlierDetection` (`:327`+), START the flusher on the server context:
    ```go
    if statsFlusher != nil {
        go statsFlusher.Start(ctx) // ctx is the server lifetime context; stops on shutdown
    }
    ```
    (Confirm the server-lifetime `ctx` variable name in main.go; reuse whatever `StartHealthChecks` is handed.)

- [ ] **Step 2: Build + boot-smoke**
```bash
go build ./... && echo BUILD_OK
```
Then two manual boot-smokes: (a) a bootstrap with a `metrics_service` `stats_sinks[]` pointing at a NON-existent cluster ⇒ confirm the `log.Fatalf` fires (the Dialer's unknown-cluster gate at sink build — REFERENCE-PARITY); (b) a bootstrap with NO `stats_sinks[]` ⇒ boots clean and NO `statssink` goroutine starts (byte-stability — `statsFlusher` stays nil).

- [ ] **Step 3: The +0 no-new-stat registration guard** (D-MS-STATS-FINAL). Because the sink registers NO stat, prove the surface is UNCHANGED: in `bootstrap_test.go` (or a `cmd/envoy-go` test) assert that loading + (conceptually) wiring a metrics_service bootstrap adds NO `statssink`/`metrics_service`-scoped counter to `bs.Stats` — i.e. the registry has the SAME names with and without the sink config. The simplest live form: a test that constructs a `MetricsServiceSink` + `Flusher` against a fresh `stats.NewRegistry()` and asserts the registry gains ZERO new metrics (the sink/flusher constructors register nothing). Run it.

- [ ] **Step 4: Per-task gates + commit**
```bash
gofmt -l cmd/envoy-go/ internal/bootstrap/ && golangci-lint run ./cmd/... ./internal/bootstrap/... && go vet ./cmd/... && go build ./...
git add cmd/envoy-go/main.go internal/bootstrap/bootstrap_test.go
git commit -m "phase 47.1 Task 7: boot wiring — per-StatsSinkConfig MetricsServiceSink build (REFERENCE-PARITY log.Fatalf on missing cluster) + post-Freeze Flusher.Start + LIFO Close + byte-stability (no sink => no goroutine) + the +0 no-new-stat guard (D-MS-STATS-FINAL)"
```

---

## Task 8: The driver-owned `test/helpers/metricsservice` receiver [TDD]

**Files:**
- Create: `test/helpers/metricsservice/metricsservice.go`, `test/helpers/metricsservice/metricsservice_test.go`

The `test/helpers/otlptrace` analog — an in-process h2c `MetricsServiceServer` accumulating every received `MetricFamily` (D-MS-RECEIVER-WIRING). NOT a runner `BackendKind` (`reference_differential_grpc_receiver_driver_owned` — BackendKind stays 38). The §11 scratchpad seed is GONE; build from the `otlptrace` shape.

- [ ] **Step 1: Write the failing tests** in `metricsservice_test.go`: `New(t)` starts a server; open a `MetricsServiceClient` (or a bare gRPC stub) to `Addr()`, `StreamMetrics` and `Send` a message with `Identifier{Node{Id:"n",Cluster:"c"}}` + two `MetricFamily`s (`cluster.x.upstream_rq_total`=7 COUNTER, `g`=3 GAUGE), then `CloseAndRecv` ⇒ `Count() == 2`; `Family("cluster.x.upstream_rq_total")` returns `(7.0, COUNTER, true)`; `Node()` returns id="n"/cluster="c"; a SECOND message updating the counter to 9 ⇒ `Family(...)` returns `9.0` (last-seen); `Reset()` clears; `Stop()` shuts down.

- [ ] **Step 2: Run to verify they fail** — `go test ./test/helpers/metricsservice/ -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `metricsservice.go` (the `otlptrace.go` template): a `Server` embedding `metricsv3.UnimplementedMetricsServiceServer` with `addr`/`lis`/`grpcSrv`/`mu sync.RWMutex`/`fams map[string]familyValue`/`node *corev3.Node`/`stopOnce`. `newServer("127.0.0.1:0")` listens, `metricsv3.RegisterMetricsServiceServer(grpcSrv, s)`, serves. `StreamMetrics(stream)` loops `stream.Recv()`: on the first message capture `msg.GetIdentifier().GetNode()`; for each `msg.GetEnvoyMetrics()` family, store `fams[f.GetName()] = {value: f.GetMetric()[0].GetCounter().GetValue() or .GetGauge().GetValue(), typ: f.GetType()}` under the mutex; on `io.EOF` reply `stream.SendAndClose(&metricsv3.StreamMetricsResponse{})`. `New(t)`/`NewAtAddr(addr)`/`Family(name)`/`Count()`/`Node()`/`Reset()`/`Addr()`/`Stop()` mirror `otlptrace`.

- [ ] **Step 4: Run to verify they pass** — `go test ./test/helpers/metricsservice/ -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/metricsservice/ && golangci-lint run ./test/helpers/metricsservice/... && go vet ./test/helpers/metricsservice/... && go build ./...
git add test/helpers/metricsservice/
git commit -m "phase 47.1 Task 8: driver-owned h2c MetricsService receiver (test/helpers/metricsservice — accumulate MetricFamily by name + identifier node; BackendKind stays 38; D-MS-RECEIVER-WIRING)"
```

---

## Task 9: The `0089-stats-sink-metrics-service` cross-side EXACT differential + boot-reject subject unit tests

**Files:**
- Create: `test/fixtures/0089-stats-sink-metrics-service/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the `0089` driver)

COPY the `0087`/`0088` driver-owned-receiver differential shape and adapt: the metrics cluster is h2c (`http2_protocol_options{}`); the receiver is `test/helpers/metricsservice`; a SHORT `stats_flush_interval` (e.g. 500ms) drives fast deterministic convergence. Both sides stream to the SAME receiver over a shared bridge (`reference_docker_probe_bridge_network`).

- [ ] **Step 1: Author `driver/driver.go`** (the `0087` driver shape): start the `metricsservice` receiver on the shared-bridge-reachable addr; fire K=7 deterministic requests through the proxy (all 2xx); POLL the receiver until the deterministic COUNTER subset converges to the post-K values on EACH side — a release barrier, NEVER a `time.Sleep` (`reference_concurrency_differential_release_barrier`). Assertions (cross-side EXACT, on the deterministic NAME SUBSET — AMEND-MS-HISTOGRAM-PRESENT: a name-subset, NOT the whole family set):
  - for `cluster.<backend>.upstream_rq_total`, `http.<stat_prefix>.downstream_rq_total`, `http.<stat_prefix>.downstream_rq_2xx`: present on both sides with `type == COUNTER` and `value == 7 == K` (poll `Family(name)` until value==K to converge).
  - `Node().Id == <configured>` and `Node().Cluster == <configured>` (the identifier on msg #1).
  - **UNasserted**: the whole family set / family count (the surfaces differ cross-side — envoy-go has no histograms); `timestamp_ms` (value non-deterministic; presence-only); `help`; non-deterministic gauges (`server.uptime`, `*_active`, connection churn); the identifier `user_agent_*`/`extensions[]`; message/stream framing + per-message family count (`reference_streaming_sink_differential_framing`).
  - Verify decode RAN (`Count() > 0` on each side) BEFORE asserting — a zero-family green is a false pass (`reference_docker_probe_bridge_network`).

- [ ] **Step 2: Author `envoy.yaml` + `envoy-go.yaml`** — the `0087` shape with: an H1 downstream listener (`stat_prefix` FIXED, identical both sides) → a route to a fixed-body backend (cluster name FIXED, identical both sides); a bootstrap `stats_sinks[]` `metrics_service` entry (`grpc_service.envoy_grpc.cluster_name` → the metrics cluster; `transport_api_version: V3`) + `stats_flush_interval: 0.5s`; a `metrics_cluster` (h2c STRICT_DNS → the shared-bridge receiver hostname:port, `typed_extension_protocol_options` HttpProtocolOptions `explicit_http_config.http2_protocol_options{}`); node `id`/`cluster` FIXED identical both sides. The receiver hostname must be reachable from the reference container.

- [ ] **Step 3: Run the differential** — `go test ./test/differential/ -run 'TestDifferential/0089' -count=1 -v` ⇒ PASS (read the `-v` assertion lines: confirm the three COUNTER subset families == 7 on BOTH sides + the node identifier — NOT a vacuous green). Confirm the receiver got families from BOTH sides.

- [ ] **Step 4: Subject-side reject confirmations** (D-MS-REJECT-COVERAGE — NO new fixture dirs): confirm Task 6 covers the parse rejects (sibling TypeURL / knob values / non-V3 / google_grpc / empty cluster / stats_flush_on_admin) in `bootstrap_test.go`, and Task 3 covers the missing/non-H2-cluster `log.Fatalf` in `grpcclient_test.go`. No `0090`-class dir at 47.1.

- [ ] **Step 5: Blank-import + commit**
```bash
# add: _ "github.com/esalaine/envoy-go/test/fixtures/0089-stats-sink-metrics-service/driver" to runner_test.go
gofmt -l test/ && golangci-lint run ./test/... && go build ./... && go vet ./test/...
git add test/fixtures/0089-stats-sink-metrics-service/ test/differential/runner_test.go
git commit -m "phase 47.1 Task 9: 0089-stats-sink-metrics-service cross-side EXACT differential (poll-to-converge, deterministic COUNTER name-subset == K + node identifier) over a driver-owned MetricsService receiver (fixtures 90->91; D-MS-DIFFERENTIAL)"
```

---

## Task 10: Deliberate breaks + flake-soak + full-package `-race` + the full 91-dir differential + six-gate + docs (ADR-0262, row 47 STAYS in-progress)

**Files:**
- Modify: `docs/envoy-go/phases/47-stats-sink-metrics-service/PROGRESS-47.1.md`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`

- [ ] **Step 1: Deliberate-break verification** (`reference_differential_break_protocol_count1` — `-count=1` on EVERY break; restore via `git restore`, NEVER checkout-sha/amend, `feedback_subagent_worktree_detach`). Prove each `0089` assertion is LIVE: (a) break the mapping value (emit `0` instead of `Load()`) ⇒ the `== K` subset assertion FAILS; (b) break the metric TYPE (emit GAUGE for a counter) ⇒ the `type == COUNTER` assertion FAILS; (c) break the identifier (drop the node) ⇒ the node-id assertion FAILS; (d) disable the flush (return before `Submit`) ⇒ the receiver gets 0 families and the `Count() > 0` decode-ran guard + subset assertion FAIL. Restore after each; re-run `-run 'TestDifferential/0089' -count=1` ⇒ PASS.

- [ ] **Step 2: Flake-soak** — `for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0089' -count=1 || break; done` ⇒ 20/20 PASS (the poll-to-converge is flake-free).

- [ ] **Step 3: Full-package `-race`** — `go test ./internal/statssink/ -race -count=1` ⇒ PASS (the `Flusher` ticker + the `MetricsServiceSink` writer goroutine are background mutators; the FULL package, not a subset — `reference_full_suite_race_after_background_mutator`).

- [ ] **Step 4: The full differential + six-gate** — `gofmt -l .` (empty) + `golangci-lint run ./...` + `go vet ./...` + `go build ./...` + `go test ./... -count=1` + the FULL `go test ./test/differential/ -count=1` (all 91 dirs). A transient unrelated `subject ready: EOF` startup flake is isolatable (`reference_differential_fullsuite_startup_flake`) — isolate-re-run the offending dir to distinguish a regression from the known startup race. Record the verbatim outputs in PROGRESS-47.1.md.

- [ ] **Step 5: Reconcile counts** — `ls -d test/fixtures/*/ | wc -l` == 91 (tail `0089-stats-sink-metrics-service`); `grep -rh '^func Fuzz' --include='*.go' . | wc -l` == 50 (`reference_fuzzer_count_docs_drift` — reconcile the documented running total 49 → 50); the stat surface UNCHANGED at 1200 / non-H2 1196 (D-MS-STATS-FINAL +0); BackendKind 38 (unchanged); `go mod tidy -diff` EMPTY; `grep 'prometheus/client_model' go.mod` ⇒ `v0.6.1` direct (1 new module).

- [ ] **Step 6: ADR-0262 body** — append the ADR-0262 §Decision + §Consequences to `docs/envoy-go/DECISIONS.md` (the SPEC-47.1.md §13 §Context draft is the lead-in; ADR-0044 atomic-landing; status PROPOSED → ACCEPTED). The §Decision: the `internal/statssink` package (the `Flusher` registry-snapshot loop + the `MetricsServiceSink` over the ALS client-streaming template + the cumulative/no-labels `MetricFamily` mapping); the `MetricsServiceClient` typed wrapper; the `stats_sinks[]`/`stats_flush_interval` parse + strict-reject arms; the reference-parity cluster/non-V3 boot-rejects vs the envoy-go-strict knob/sibling/flush-on-admin rejects; +0 self-stats; the FIRST new go.mod module in the Observability family (`prometheus/client_model v0.6.1`) as a §Consequence. The §Consequences: the deferred 47.2 set (`report_counters_as_deltas` / `emit_tags_as_labels` / `histogram_emit_mode` / `stats_flush_on_admin` / sibling sinks); row 47 STAYS `in-progress` (the FINAL leg is 47.2); the Observability family STAYS OPEN. Tail advances **ADR-0261 → ADR-0262**; next-free **ADR-0263** (the anticipated 47.2 anchor).

- [ ] **Step 7: BEHAVIOR_CONTRACT** — add the `### Stats sinks — the metrics_service gRPC sink` subsection (the SPEC §9 contract delta): a bootstrap `stats_sinks[]` `metrics_service` entry makes envoy-go dial the cluster, open `StreamMetrics`, and every `stats_flush_interval` (default 5s) snapshot the frozen `stats.Registry` as `MetricFamily`s (identifier on msg #1; counters → COUNTER absolute, gauges → GAUGE, full dotted name, no labels, a flush-time timestamp_ms); STRICT-REJECT the sibling TypeURLs / `report_counters_as_deltas:true` / `emit_tags_as_labels:true` / non-default `histogram_emit_mode` / non-V3 / `google_grpc` / empty cluster_name / `stats_flush_on_admin`; `log.Fatalf` on a missing/non-H2 metrics cluster (reference-parity); byte-identical + stat-surface-identical (1200) when no metrics_service sink is configured.

- [ ] **Step 8: STATE + ROADMAP** — STATE.md: the active-phase header → "phase 47.1 IMPL done" + the NEXT (the 47.2 BRAINSTORM — deltas + tags-as-labels) + the counts (1200 / 91 / 50 / 38 / ADR-0262). ROADMAP.md: **row 47 (`stats-sink-metrics-service`) STAYS `in-progress`** (47.1 landed; 47.2 is the FINAL leg that flips it `done` per ADR-0106 + `reference_roadmap_split_phase_row_done`); the Observability family STAYS OPEN.

- [ ] **Step 9: Commit**
```bash
git add docs/envoy-go/
git commit -m "phase 47.1 Task 10: ADR-0262 body + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 47 stays in-progress) + full 91-dir differential + six-gate green + count reconcile (1200/91/50/38/ADR-0262; +1 go.mod module)"
```

---

## Exit criteria (controller-verified at stage-close, on the FINAL frozen HEAD)

- Six gates green on the frozen HEAD: `gofmt -l .` empty, `golangci-lint run ./...`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, the FULL `go test ./test/differential/ -count=1` (91 dirs, modulo the documented startup-flake class — isolate-re-run to confirm).
- The deliberate-break verification re-done by the CONTROLLER (not trusted from the subagent logs).
- Full-package `-race` on `internal/statssink` green; `0089` 20/20 flake-soak green.
- Counts: stat **1200** (non-H2 **1196**; D-MS-STATS-FINAL +0) / fixtures **91** (`0089-stats-sink-metrics-service`) / fuzzers **50** (`FuzzStatsSinkConfigParse`) / BackendKind **38** / DECISIONS **ADR-0262**. `go mod tidy -diff` EMPTY; **1 new package** (`internal/statssink`) + **1 new go.mod module** (`github.com/prometheus/client_model v0.6.1`, direct).
- **ROADMAP row 47 (`stats-sink-metrics-service`) == `in-progress`** (47.1 is NOT the final leg); the Observability family STILL OPEN.
- The worktree's commits SQUASHED into one + fast-forwarded onto master + pushed (`feedback_subagents_no_push` · `feedback_push_to_origin`); the worktree removed.
