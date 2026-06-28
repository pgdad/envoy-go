# Phase 46.1b Implementation Plan — span emission + OTLP (OpenTelemetry) trace export: the per-request `ingress`/SERVER span model (the concrete 16-attribute roster) + the `OTLPTracesClient` UNARY typed wrapper + the `tracing.OTLPExporter` (the reused 44.2/45.1 bounded-channel + writer-goroutine + size/interval/close batching sink, flushing `TraceService.Export`) + the span-lifecycle wiring (carry the 46.1a `Decision` from the dispatch seam to the `accesslog_emit.go` end-seam) + the 2 tracer-scoped `tracing.opentelemetry.{spans_sent,spans_dropped}` counters + the `log.Fatalf` collector-cluster gate + the driver-owned `test/helpers/otlptrace` receiver + the `0087-tracing-otlp` span differential — the COMPLETING sub-leg of the 46.1 (core+OTLP) by-exporter leg; CLOSES ADR-0260

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). NOTE the 46.1a execution lesson (`feedback_subagent_autocommit_claudemd`): the global CLAUDE.md makes dispatched subagents AUTO-COMMIT — do NOT fight it; the controller VERIFIES each commit (correct fileset, real non-vacuous tests, gates green), cleans stray next-task leak files, and re-runs the full suite on the FINAL frozen HEAD before squashing.

**Goal:** When an HCM carries a `tracing` block with a recognized OpenTelemetry provider, envoy-go builds a per-request **SERVER** span (named the constant `"ingress"`, the concrete 16-attribute built-in roster; start at the 46.1a request-dispatch decision point, end at the access-log emit seam = stream-complete), and EXPORTS the sampled spans to the configured collector over the UNARY `opentelemetry.proto.collector.trace.v1.TraceService.Export` (the reused size/interval/close batching + retry-once + drop-newest sink) — proven cross-side EXACT on the stable per-span payload subset (+ a trace-id continuation prong) against `contrib-v1.37.2` by the `0087-tracing-otlp` differential through a driver-owned OTLP `TraceService` receiver. This is the COMPLETING half of the 46.1 leg: 46.1a built the header-level engine (the sampling/request-id `Decision` + `traceparent` propagation + `x-request-id` generation); 46.1b consumes that `Decision` to emit + export the span. **CLOSES ADR-0260** (its §Decision/§Consequences body lands atomically here).

**Architecture:** ZERO new Go packages (the span model + the exporter live in the existing `internal/tracing`; the typed client in `internal/grpcclient`) and ZERO new go.mod modules (the OTLP trace protos `trace/v1` + `collector/trace/v1` resolve at the already-direct `go.opentelemetry.io/proto/otlp v1.0.0`, siblings of the `logs/v1` packages phase 45 consumed). It composes on FOUR already-built substrates: the 46.1a `tracing.Decision` (already carries `TraceID`/`SpanID`/`ParentSpanID`/`TraceState`/`Sample` — the span model's inputs, populated at dispatch); the `OTLPLogsClient` typed-wrapper pattern (`grpcclient.go:319–375`, ADR-0158) for the new `OTLPTracesClient`; the `otlpsink.go` 217-LoC bounded-channel batching sink for the new `OTLPExporter`; and the HCM request-lifecycle seams (the `connection.go:509`/`h2dispatch.go:410` dispatch points where the `Decision` is computed, the `accesslog_emit.go:19`/`:45` emit points where the span ends). The exporter is built by a boot-provided `ExporterProvider` (closing over the hoisted `grpcclient.Dialer` + the shared tracer counters), threaded into the HCM filter via `builtins.Deps`; each `Filter` looks up its OWN exporter by its OWN parsed `cluster_name` (self-contained — no reliance on the access-log sink-matching). Byte-identical when no HCM `tracing` provider is configured (every non-tracing path untouched — the full differential is the regression anchor; the new tracer counters + the exporter goroutine exist only when ≥1 provider is built).

**Tech Stack:** Go; the existing `internal/tracing` package (NEW `span.go` + `exporter.go`); `internal/grpcclient` (NEW `OTLPTracesClient`, mirroring `OTLPLogsClient`); `internal/filter/hcm` (the `Filter.exporter` field + the span-end wiring at `accesslog_emit.go` + the `Decision`-carry plumbing on the H1 call frame / the H2 `chainDispatchAction`); `cmd/envoy-go/main.go` + `internal/filter/network/builtins` (the `ExporterProvider` boot wiring + the `Dialer` hoist + the `log.Fatalf` cluster gate); `go.opentelemetry.io/proto/otlp/trace/v1` + `.../collector/trace/v1` + `.../common/v1` (already direct); the driver-owned `test/helpers/otlptrace` gRPC receiver (mirroring `test/helpers/otlplogs`); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`, the `0084` precedent). ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY).

---

## Orientation — read before Task 1 (the zero-context brief)

You are completing the FIRST request-tracing subsystem of a Go reimplementation of Envoy. The header-level engine ALREADY EXISTS (phase 46.1a, landed): per request, when an HCM has a `tracing` block, `internal/tracing.Decide(req.Header, cfg, rng)` runs the sampling/request-id decision, the HCM filter stamps `x-request-id` + injects the W3C `traceparent`/`tracestate` upstream, and increments the 5 HCM-scoped `http.<stat_prefix>.tracing.*` counters. **What 46.1a does NOT do: build a span or export anything.** The `Decision` it computes is used inline at dispatch and DISCARDED. You are adding the span half: build a per-request SERVER span from that `Decision` (+ the request/response inputs), and ship sampled spans to a collector over gRPC.

**The `Decision` is your input — already complete.** From `internal/tracing/decision.go` (verbatim, as built at 46.1a):
```go
type Decision struct {
    Sample       bool
    Reason       TraceReason   // NoTrace / Sampled / Client (the x-request-id nibble)
    Class        SampleClass   // the HCM-counter class
    Continued    bool
    TraceID      [16]byte      // continued (incoming traceparent) or fresh
    SpanID       [8]byte       // fresh; the upstream traceparent span-id AND the SERVER span's span_id
    ParentSpanID [8]byte       // incoming parent-id (continued) or zero (root)
    TraceState   string        // pass-through
    RequestID    string        // the generated/stamped x-request-id
}
func Decide(h http.Header, cfg *TracingConfig, rng RandSource) Decision
```
Every field the span needs (`TraceID`/`SpanID`/`ParentSpanID`/`TraceState`/`Sample`/`RequestID`) is ALREADY populated. You do NOT touch `decision.go`. The span's `span_id` = `Decision.SpanID`; the span's `parent_span_id` = `Decision.ParentSpanID` (empty for a root); the span's `trace_id` = `Decision.TraceID`. Export the span IFF `Decision.Sample`.

**The span shape (§11 D-TRACE-SPAN — live-probed against `contrib-v1.37.2`, all pinned).** Exactly ONE span per request: `name == "ingress"` (a constant — NOT the cluster name), `kind == SPAN_KIND_SERVER` (=2), `start_time_unix_nano` < `end_time_unix_nano` (both populated; start = request dispatch, end = stream-complete = the access-log emit seam), `parent_span_id` empty for a fresh trace / = the incoming parent-id for a continued trace. Its `attributes` are 16 fixed keys (the deterministic subset is the cross-side-assertable target — §3.4 table). The OTLP envelope: `ExportTraceServiceRequest{ ResourceSpans:[{ Resource{ service.name ← config }, ScopeSpans:[{ Scope{name,version}, Spans:[…] }] }] }`. `ScopeSpans.scope` IS populated (UNLIKE the empty 45.1 `ScopeLogs.scope`) — but the scope name/version + the SDK resource attrs are impl-specific (envoy-go is NOT cpp) and UNasserted; only `Resource`'s `service.name` is cross-side assertable.

**The export path mirrors phase 45.1 exactly.** The OTLP access-log sink (`internal/accesslog/otlpsink.go`, 217 LoC) is your verbatim template: a bounded `chan`, a single writer goroutine (`run()`), a size/interval/close-triggered buffer, ONE `Export` RPC per flush, retry-once-then-drop, an idempotent `sync.Once` `Close()`, and `*_written`/`*_dropped` counters. You build the SAME shape for spans (`tracing.OTLPExporter`), swapping `LogRecord`→`Span` and `ExportLogsServiceRequest`→`ExportTraceServiceRequest`. The typed client `grpcclient.OTLPTracesClient` mirrors `OTLPLogsClient` (`grpcclient.go:339`) — UNARY, no stream lifecycle.

**The differential model (the `0084-otlp-access-log` precedent).** A driver-owned in-process gRPC `TraceService` receiver (`test/helpers/otlptrace`, mirroring `test/helpers/otlplogs`) accumulates every received span. BOTH the reference (Docker `contrib-v1.37.2`) and the subject (in-process envoy-go) export to the SAME receiver over a shared Docker bridge (the receiver hostname must be reachable from the reference container — `reference_docker_probe_bridge_network`, the `0084` wiring verbatim). The driver fires N requests, POLLS the receiver until N spans converge (a release barrier — NEVER a `time.Sleep`, `reference_concurrency_differential_release_barrier`), and asserts the per-span payload AGGREGATED across `Export` calls (NOT the stream/message framing, which legitimately varies side-to-side — `reference_streaming_sink_differential_framing`). The OTLP collector cluster is h2c (`explicit_http_config.http2_protocol_options{}`).

### Key source seams (verified at PLAN time against master `67e85880`; re-confirm line numbers before editing — files evolve)

- **`internal/tracing/decision.go`** — the `Decision` struct (above) + `Decide(...)`. **NOT MODIFIED at 46.1b** (its outputs are the span's inputs; `SpanID`/`ParentSpanID`/`TraceState` already populated). The `SampleClass`/`TraceReason` enums also live here (reuse).
- **`internal/tracing/config.go`** — `TracingConfig{ClientSampling, RandomSampling, OverallSampling float64; ServiceName, ClusterName string}` + `NewConfig(t *hcmv3.HttpConnectionManager_Tracing) (*TracingConfig, error)`. `ServiceName` (← `OpenTelemetryConfig.service_name`) + `ClusterName` (← `envoy_grpc.cluster_name`) were PARSED-and-STORED at 46.1a but UNUSED — 46.1b consumes them (the exporter's `Resource.service.name` + the collector dial). **NOT MODIFIED** (the fields exist).
- **`internal/tracing/stats.go`** — `HCMCounters` (the 5 HCM-scoped counters) + `RegisterHCMCounters`. **MODIFIED at 46.1b:** add `TracerCounters{spansSent, spansDropped *stats.Counter}` + `RegisterTracerCounters(reg) *TracerCounters` (the process-global tracer-scoped pair) + `(*TracerCounters).IncSent(n)`/`IncDropped()`.
- **`internal/filter/hcm/connection.go:509`** (the H1 dispatch seam) — inside `dispatchRequest(ctx, downstream, req, bw) (int, error)` (`:311`–`:613`):
  ```go
  if f.tracingConfig != nil {
      d := tracing.Decide(req.Header, f.tracingConfig, f.rng)
      req.Header.Set("X-Request-Id", d.RequestID)
      tracing.InjectTraceparent(req.Header, d.TraceID, d.SpanID, d.Sample, d.TraceState)
      f.tracingCounters.Record(d.Class)
  }
  ```
  The `d` (`tracing.Decision`) is computed and DISCARDED. **46.1b CAPTURES it** into a `dispatchRequest`-local (`var traceDecision *tracing.Decision`) and passes it to `emitAccessLog` (which is called later in the SAME frame) so the span can be built + ended there.
- **`internal/filter/hcm/h2dispatch.go:410`** (the H2 dispatch seam) — the analogous block builds an `http.Header` view, calls `Decide`, and writes back via `upsertH2Header` + `rf.SetH2Request(h2req)`. The `d` is DISCARDED. **46.1b STORES it** on the `chainDispatchAction` struct (`h2dispatch.go:200`–`:261`, the H2 per-request context object) so `emitAccessLogH2` can read it.
- **`internal/filter/hcm/accesslog_emit.go`** — the SPAN-END seam (stream-complete, AMEND-TRACE-SPANEND-SEAM):
  - H1 (`:19`): `func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders)`.
  - H2 (`:45`): `func (f *Filter) emitAccessLogH2(req h2.H2Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time, respHeaders filter_http.OrderedHeaders)`.
  Both already receive every span input EXCEPT the `Decision`: `statusCode` (→ `http.status_code`; 0 is the ctx-cancel sentinel that SKIPS emission — skip the span too), `bytesSent` (→ `response_size`), `start time.Time` (→ `start_time_unix_nano`; the span's END time is `time.Now()` here), `respHeaders`, `picked cluster.Endpoint` (→ `upstream_cluster`), and the request (`*http.Request` H1 / `h2.H2Request` H2 — `http.method`/`http.url`/`http.protocol`/`user_agent`/`request_size`). **46.1b carries the `Decision` in** (H1: a new trailing param; H2: a `chainDispatchAction` field) and, when `f.exporter != nil && decision != nil && decision.Sample`, builds the span + `f.exporter.Export(span)`. (Confirm at IMPL: `emitAccessLog`/`emitAccessLogH2` are each invoked from exactly the dispatch frame that holds the `Decision`.)
- **`internal/filter/hcm/config.go`** — `type Filter struct` (`:92`) with the 46.1a tracing fields `tracingConfig *tracing.TracingConfig` / `tracingCounters *tracing.HCMCounters` / `rng tracing.RandSource` (`:162`–`:174`); `parseFilterWithCtx` (`:202`–`:338`) calls `tracing.NewConfig(msg.GetTracing())` (`:308`) + conditionally `RegisterHCMCounters` (`:312`). **46.1b adds:** a `exporter tracing.Exporter` Filter field, set in `parseFilterWithCtx` from a boot-provided `ExporterProvider` looked up by `tcfg.ClusterName` (when `tcfg != nil`).
- **`internal/filter/hcm/filter.go`** — `NewFilterWithCtxAndSinksAndRegistry(tc, clusters, lc, registry, accessLogSinks, httpRegistry, dm)` (`:88`) → `parseFilterWithCtx`; `NewNetworkFactory(...)` (`:36`) the boot wrapper. **46.1b adds** the `ExporterProvider` to the boot-closed-over singletons (a new `NewNetworkFactory` param + a new field threaded to `parseFilterWithCtx`).
- **`internal/filter/network/builtins/builtins.go:55`** — `reg.Register(hcm.TypeURL, hcm.NewNetworkFactory(deps.ClusterManager, deps.StatsRegistry, …))`. **46.1b adds** `deps.TracingExporters` to `builtins.Deps` + threads it into `NewNetworkFactory`.
- **`internal/grpcclient/grpcclient.go:319`–`375`** — the `OTLPLogsClient` UNARY typed wrapper (struct + `NewOTLPLogsClient` + `Export` + `Close`, 57 LoC; `Dialer.DialContext` at `:107` with the `UseH2()` gate at `:118`). **The template for `OTLPTracesClient`.** Import alias precedent `collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"` (`:61`) → add `coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"`.
- **`internal/accesslog/otlpsink.go`** (217 LoC) — the bounded-channel + writer-goroutine + size/interval/close-buffer sink (`otlpClient` interface seam `:24`; `OTLPAccessLogSink` struct `:43`; `NewOTLPAccessLogSink`/`newOTLPSinkWithCapacity` `:81`; `Submit` `:111`; `Close` `:135`; `run` `:155`). **The template for `tracing.OTLPExporter`.**
- **`test/helpers/otlplogs/otlplogs.go`** (213 LoC) — the driver-owned OTLP receiver (`UnimplementedLogsServiceServer` `:48`; `RegisterLogsServiceServer` `:112`; `Export` accumulator `:128`; `Records()`/`Count()`/`ResourceAttributes()`/`Reset()`/`Addr()`/`Stop()` poll surface). **The template for `test/helpers/otlptrace`.**
- **`cmd/envoy-go/main.go:129`–`160`** — the `Dialer` hoist (`dialer := grpcclient.New(cm)` `:130`, gated `if len(bs.ALSConfigs) > 0 || len(bs.OTLPConfigs) > 0`), the per-config client+sink build, and the defer-LIFO `Close()` loop (`:156`). The boot order: the sinks build (`:142`–`:154`) BEFORE the listener manager is constructed (`:273`); `cm` exists before both; `bs.Stats.Freeze()` is at `:313` (AFTER listener construction). **46.1b extends** the hoist gate (`|| tracing configured`), builds the `ExporterProvider`, threads it into `builtins.Deps` + the listener manager path, and joins its exporters' `Close()` to the defer-LIFO.
- **`test/fixtures/0084-otlp-access-log/`** — the driver-owned-receiver differential shape (`driver/driver.go` firing N requests + polling the receiver to converge + `AssertStats` cross-side + subject-`/stats`; `envoy.yaml`/`envoy-go.yaml` with the shared-bridge OTLP cluster reachability; the h2c collector cluster). **COPY the directory** (swap the logs receiver for the trace receiver; the bridge/hostname wiring is verbatim).
- **`test/differential/runner_test.go`** — the fixture-driver blank-import block (`_ ".../0086-tracing-request-id/driver"` at the tail). Add the `0087` driver import.

### OTLP trace proto facts (verified at PLAN time; `go.opentelemetry.io/proto/otlp v1.0.0`, already direct)

- `coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"`: `TraceServiceClient` (`NewTraceServiceClient(conn)`), `Export(ctx, *ExportTraceServiceRequest) (*ExportTraceServiceResponse, error)`, RPC `/opentelemetry.proto.collector.trace.v1.TraceService/Export`; server side `RegisterTraceServiceServer` + `UnimplementedTraceServiceServer`. `ExportTraceServiceRequest{ ResourceSpans []*tracepb.ResourceSpans }`.
- `tracepb "go.opentelemetry.io/proto/otlp/trace/v1"`: `ResourceSpans{ Resource *resourcepb.Resource; ScopeSpans []*ScopeSpans; SchemaUrl string }`; `ScopeSpans{ Scope *commonpb.InstrumentationScope; Spans []*Span }`; `Span{ TraceId []byte (16); SpanId []byte (8); TraceState string; ParentSpanId []byte (8); Name string; Kind Span_SpanKind; StartTimeUnixNano uint64; EndTimeUnixNano uint64; Attributes []*commonpb.KeyValue }`; `Span_SPAN_KIND_SERVER Span_SpanKind = 2`.
- `commonpb "go.opentelemetry.io/proto/otlp/common/v1"` (ALREADY used by the logs sink): `KeyValue{ Key string; Value *AnyValue }`; `AnyValue` with `StringValue`/`IntValue` oneof; `InstrumentationScope{ Name, Version string }`.
- `resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"`: `Resource{ Attributes []*commonpb.KeyValue }`. (Confirm the logs sink's resource import path at IMPL and reuse.)
- `go mod tidy -diff` shows NO require change (the trace packages resolve at the already-present module — the same `v1.0.0` the logs packages use).

### Discipline (honor on EVERY task)

- **TDD** (`superpowers:test-driven-development`): each code task is failing-test → run-fail → minimal-impl → run-pass → commit.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): `gofmt -l` (empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`.
- **Worktree hygiene** (`feedback_subagent_worktree_detach`/`_path_targeting`): subagents write to the WORKTREE path; the controller verifies the main checkout stays clean + the branch is undetached after each task.
- **Commit locally only** (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0087'`, NEVER bare `'0087'`.
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1`.
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): 46.1b ADDS a background mutator (the `OTLPExporter` writer goroutine). A `-run`-subset `-race` will MISS a data race an earlier test's lingering goroutine causes — the `-race` gate MUST run the FULL `internal/tracing` package (+ `internal/filter/hcm`), not a subset. This bit the 43.2b final merge gate.
- **Streaming-sink framing** (`reference_streaming_sink_differential_framing`): assert the per-span PAYLOAD aggregated across `Export` calls, NEVER the message/stream framing (it varies side-to-side).
- **Driver-owned receiver** (`reference_differential_grpc_receiver_driver_owned`): the OTLP `TraceService` receiver is a `test/helpers/otlptrace` server the proxy DIALS — NOT a runner `BackendKind` (stays 38).
- **Docker bridge** (`reference_docker_probe_bridge_network`): the receiver must be reachable from the reference container by hostname over a shared bridge; verify decode RAN (`spans_sent > 0` / the receiver's span count > 0) before trusting a green.
- **Release barrier** (`reference_concurrency_differential_release_barrier`): poll the receiver to converge to N spans — never a `time.Sleep`.
- **Wire-format both sides** (`reference_wire_format_both_sides_see_same_bytes`): the OTLP span framing is shared — adopt the reference's `Span`/`ResourceSpans` shapes verbatim.

---

## D-question resolutions (the SPEC §12 D-TRACE-* PLAN/IMPL pins — settled here)

**D-TRACE-SPANEND-PLUMBING → CARRY the 46.1a `tracing.Decision` from the dispatch seam to the `accesslog_emit.go` end-seam; BUILD + SAMPLE + EXPORT the whole span AT the emit seam (no mutable in-flight span handle).** The Explore confirmed there is NO shared per-request object on which the `Decision` already sits. The minimal, allocation-light resolution: the span's START time is already the `start time.Time` the emit seam receives; the span's END time is `time.Now()` at the emit seam; every other span input is at the emit seam EXCEPT the `Decision`. So carry ONLY the small `Decision` value (not a span object that lives across the request) and build the entire span at the end:
- **H1:** `dispatchRequest` (`connection.go:311`) computes `d` at `:509` and calls `emitAccessLog` later in the SAME call frame. Capture `d` into a frame-local `var traceDecision *tracing.Decision` (set `&d` inside the `if f.tracingConfig != nil` block) and add it as a trailing param to `emitAccessLog`. No struct change.
- **H2:** the per-request context is the `chainDispatchAction` struct (`h2dispatch.go:200`–`:261`). Add a `traceDecision *tracing.Decision` field; set it at the `:410` dispatch block; `emitAccessLogH2` reads `action.traceDecision`. (Pass it as a param too, mirroring H1, OR read it off the action — locked at Task 8; default: a param for symmetry with H1.)
- At the emit seam: `if f.exporter != nil && d != nil && d.Sample && statusCode != 0 { span := tracing.BuildServerSpan(*d, <inputs>, end=time.Now()); f.exporter.Export(span) }`. The `statusCode == 0` ctx-cancel sentinel that already skips the access-log emit ALSO skips the span (no span for a cancelled request — matches the access-log's own skip).
- **CRITICAL placement (PLAN-review #1):** both `emitAccessLog` (`accesslog_emit.go:20`) and `emitAccessLogH2` (`:46`) OPEN with `if statusCode == 0 || len(f.accessLog) == 0 { return }`. The span block MUST sit at the FUNCTION HEAD, AHEAD of that early return — guarded ONLY by `statusCode != 0 && f.exporter != nil && d != nil && d.Sample`. A tracing HCM commonly carries NO `access_log` block (the `0086`/`0087` fixtures don't), so `len(f.accessLog) == 0` fires the early return; placing the span "at the end" would make `f.exporter.Export` DEAD CODE (the receiver gets 0 spans, the differential fails, and the deliberate-break (c) is vacuous). The span path reuses the seam's INPUTS but must NOT inherit the access-log-sink gate — only the shared `statusCode == 0` cancel-skip applies.

**D-TRACE-EXPORTER-WIRING → a boot-built `tracing.ExporterProvider` (closing over the hoisted `grpcclient.Dialer` + the shared `*TracerCounters` + the boot node), threaded into the HCM filter via `builtins.Deps`; each `Filter` looks up its OWN exporter by its OWN parsed `cluster_name`; the `log.Fatalf` cluster gate surfaces as a bubbled parse error fatal'd at listener-manager construction.** This is the self-contained resolution that avoids a SECOND bootstrap walk (the parse + reject logic lives only in `tracing.NewConfig`, called from `parseFilterWithCtx`) AND avoids the access-log sink-matching ambiguity (the Filter does NOT need to find "its" sink in a flat list — it asks the provider for the exporter matching its own `tcfg.ClusterName`).
- `internal/tracing` gains:
  ```go
  // Exporter is the span-sink seam (the 46.2 Zipkin exporter is a second impl).
  type Exporter interface { Export(span *Span); Close() error }

  // ExporterProvider memoizes one OTLPExporter per collector cluster_name. Built at
  // boot over the shared Dialer + tracer counters + node; consulted at filter-parse.
  type ExporterProvider struct { /* dialer-ish seam, counters, node, mu, byCluster map */ }
  func (p *ExporterProvider) ExporterFor(clusterName, serviceName string) (Exporter, error)
  func (p *ExporterProvider) CloseAll() error
  ```
  To keep `internal/tracing` from importing `internal/grpcclient` (acyclic-by-convention; the exporter wraps a CLIENT INTERFACE, the 45.1 pattern), the provider takes a small dial seam:
  ```go
  type tracesClientDialer interface {
      NewTracesClient(clusterName string) (otlpTracesClient, error) // otlpTracesClient = {Export(ctx,*ExportTraceServiceRequest)(...); Close() error}
  }
  func NewExporterProvider(d tracesClientDialer, counters *TracerCounters, serviceNameToResource ...) *ExporterProvider
  ```
  `cmd/envoy-go/main.go` supplies the `tracesClientDialer` impl (a 3-line adapter closing over `grpcclient.NewOTLPTracesClient(dialer, name)`). `ExporterFor` lazily dials (the `Dialer.DialContext` unknown-cluster / non-H2 gate returns an error → `ExporterFor` returns it → `parseFilterWithCtx` wraps it `hcm: tracing exporter: %w` → `NewFilterWithCtxAndSinksAndRegistry` → the listener manager → `main.go:274` `log.Fatalf("listener manager: %v", err)`) — a fail-fast boot failure, the AMEND-TRACE-NO-BOOT-REJECT departure (the reference boots permissively). Boot order holds: `cm` exists at `main.go:130`; filter-parse runs inside listener construction (`:273`), so the cluster lookup sees a built cluster manager.
- `builtins.Deps` gains `TracingExporters *tracing.ExporterProvider` (nil ⇒ no tracing wired — but the provider is cheap; build it always). Thread `deps.TracingExporters` → `hcm.NewNetworkFactory(..., deps.TracingExporters)` → the Filter constructor → `parseFilterWithCtx`. This is the SOLE seam: the HCM filter factory closure is created EXACTLY ONCE in `builtins.RegisterBuiltins(netReg, …)` (`main.go:263`); `listener.NewManagerWithBaseDirAndAllowH2C` (`main.go:273`) resolves HCM via the frozen `netReg` and does NOT build the factory itself — so its signature gets NO new param (PLAN-review #2).
- The shared `*TracerCounters` register LAZILY in the provider on the FIRST `ExporterFor` (a `sync.Once`) so a no-tracing boot has NO `tracing.opentelemetry.*` surface (byte-stable). Pre-Freeze registration is satisfied (filter-parse is pre-`:313`).

**D-TRACE-RECEIVER-WIRING → the `0084` precedent verbatim.** `test/helpers/otlptrace` mirrors `test/helpers/otlplogs`: an in-process `coltracepb.TraceServiceServer` (`UnimplementedTraceServiceServer` embed; `RegisterTraceServiceServer`) whose `Export` walks `req.GetResourceSpans()` → `GetScopeSpans()` → `GetSpans()` accumulating every `*Span` (+ the per-`ResourceSpans` `Resource.Attributes`), exposing `Spans()`/`Count()`/`ResourceAttributes()`/`Reset()`/`Addr()`/`Stop()`. The `0087` shared-bridge + receiver-hostname reachability (from the reference container) + the h2c collector cluster COPY `0084` verbatim. The continuation prong needs NO backend-echo helper — the continued trace-id is asserted directly on the RECEIVED span's `trace_id` (cross-side), not via the upstream-injected header.

**D-TRACE-STATS-FINAL → +2 tracer-scoped `tracing.opentelemetry.{spans_sent,spans_dropped}`; surface 1196 → 1198. `timer_flushed` DROPPED.** `spans_sent` += `len(batch)` per successful `Export`; `spans_dropped` ++ per channel-full drop (the `Submit` overflow path) — the 45.1 `logs_written`/`logs_dropped` analog. `timer_flushed` (an Envoy flush-timer artifact) is NOT emitted (the exporter reuses the 45.1 buffer loop — no separate flush counter). Register LAZILY (provider `sync.Once`) so they exist only under a configured provider. Confirm the +2 delta via a registration test, not a brittle absolute.

**D-TRACE-FUZZER → NO new fuzzer at 46.1b; fuzzers STAY 48.** The 46.1a wire-input fuzzers (`FuzzExtractTraceparent` + `FuzzStampRequestID`, the untrusted-header boundary) cover the parse surface. The 46.1b span build + the proto marshal consume TRUSTED internal data (the already-validated `Decision` + the proxy's own request/response state), not untrusted wire input — no new fuzz boundary. Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l == 48` at the completion task (`reference_fuzzer_count_docs_drift`).

**D-TRACE-SPAN-ATTRS (the 16-attr roster sourcing) → build at the emit seam; assert only the DETERMINISTIC subset cross-side.** §3.4 maps each attr to its emit-seam source. The cross-side-assertable subset: `http.method`/`http.url`/`http.protocol`/`http.status_code`/`component`(=`proxy`)/`upstream_cluster`(+`.name`)/`downstream_cluster`(=`-`)/`response_flags`(=`-`)/`request_size`/`response_size`/`user_agent`/`guid:x-request-id`(KEY present, value varies). The `upstream_cluster` name comes from the picked cluster (the same derivation the access-log `%UPSTREAM_CLUSTER%` operator already uses — confirm the accessor on `picked cluster.Endpoint` / the route action at IMPL). `node_id`/`zone` (from the boot node — empty under no node config, UNasserted), `peer.address` (downstream remote IP — UNasserted), and `guid:x-client-trace-id` (conditional) round out the 16; emit them but do NOT assert them.

---

## File structure (decomposition locked here)

**Production (created):**
- `internal/tracing/span.go` — `Span` struct (`TraceID [16]byte`, `SpanID [8]byte`, `ParentSpanID [8]byte`, `Name string`, `Kind`, `Start/End time.Time`, `Attrs []KV`, `TraceState string`, `Resource` service-name) + `KV{Key string; Value string|int64}` + `BuildServerSpan(d Decision, in SpanInputs, end time.Time) *Span` (the 16-attr roster) + `(*Span).toProto() *tracepb.Span`.
- `internal/tracing/exporter.go` — the `Exporter` interface + `OTLPExporter` (bounded `chan *Span` + `run()` writer goroutine + size/interval/close buffer + `buildExportRequest(batch, serviceName) *ExportTraceServiceRequest` + retry-once + drop-newest + idempotent `Close`) + the `otlpTracesClient` interface seam + the `ExporterProvider` + `tracesClientDialer` seam.

**Production (modified):**
- `internal/tracing/stats.go` — `TracerCounters` + `RegisterTracerCounters` + `IncSent`/`IncDropped`.
- `internal/grpcclient/grpcclient.go` — `OTLPTracesClient` (struct + `NewOTLPTracesClient` + `Export` + `Close`) + the `coltracepb` import.
- `internal/filter/hcm/config.go` — the `Filter.exporter tracing.Exporter` field; the `parseFilterWithCtx` exporter lookup (when `tcfg != nil`: `provider.ExporterFor(tcfg.ClusterName, tcfg.ServiceName)`); the `ExporterProvider` threaded onto the parse context.
- `internal/filter/hcm/filter.go` — the `ExporterProvider` `NewNetworkFactory`/constructor param.
- `internal/filter/hcm/connection.go` — capture the H1 `Decision` into a frame-local; pass to `emitAccessLog`.
- `internal/filter/hcm/h2dispatch.go` — store the H2 `Decision` on `chainDispatchAction`; pass to `emitAccessLogH2`.
- `internal/filter/hcm/accesslog_emit.go` — the `Decision` param on both emit fns; build+sample+export the span (guarded `f.exporter != nil && d != nil && d.Sample && statusCode != 0`).
- `internal/filter/network/builtins/builtins.go` — `Deps.TracingExporters` + thread into `NewNetworkFactory`.
- `cmd/envoy-go/main.go` — extend the Dialer-hoist gate; build the `ExporterProvider` (+ the `tracesClientDialer` adapter over `grpcclient.NewOTLPTracesClient`); thread into `builtins.Deps` + the listener manager; join `provider.CloseAll()` to the defer-LIFO.

**Test (created):**
- `internal/tracing/span_test.go`, `internal/tracing/exporter_test.go` (the bounded-channel/flush/retry/drop/Close behavior against a fake `otlpTracesClient` + the `ExporterProvider` boot-reject), `internal/tracing/stats_test.go` (MODIFY: +2 tracer-counter registration).
- `internal/grpcclient/grpcclient_test.go` (MODIFY or add: the `OTLPTracesClient` shape test, mirroring the `OTLPLogsClient` test).
- `internal/filter/hcm/*_test.go` (MODIFY: the exporter lookup + the span-end export + the no-tracing byte-stability guard).
- `test/helpers/otlptrace/otlptrace.go` — the driver-owned receiver.
- `test/fixtures/0087-tracing-otlp/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`.
- `test/differential/runner_test.go` (MODIFY: blank-import the `0087` driver).

**Docs (completion task):**
- `docs/envoy-go/phases/46-tracing/PROGRESS-46.1b.md`, `docs/envoy-go/DECISIONS.md` (ADR-0260 §Decision/§Consequences body — CLOSES the leg), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`.

---

## Task 1: Phase scaffolding — PROGRESS-46.1b.md + baselines

**Files:**
- Create: `docs/envoy-go/phases/46-tracing/PROGRESS-46.1b.md`

- [ ] **Step 1: Record the baseline counts** (verbatim outputs in PROGRESS-46.1b.md):
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                 # expect 88 (tail 0086-tracing-request-id)
grep -rh '^func Fuzz' --include='*.go' . | wc -l                  # expect 48
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go  # expect = 38 (BackendKind tail)
grep -rn 'OTLPTracesClient\|coltracepb\|spans_sent' internal/ --include='*.go'  # expect: NONE (46.1b introduces them)
```
Baseline: stat surface **1196** (H2 cluster; non-H2 **1192**) / fixtures **88** / fuzzers **48** / BackendKind **38** / DECISIONS tail **ADR-0259** (next-free **ADR-0260**).

- [ ] **Step 2: Write the PROGRESS-46.1b.md scaffold** — a header (phase 46.1b IMPL, the SPEC-46.1 reference + the "completing sub-leg of the 46.1 by-exporter leg; CLOSES ADR-0260" note, the worktree branch), a task checklist mirroring this plan, the baseline block, and the anticipated exit counts: stat **1198** (+2 — `tracing.opentelemetry.{spans_sent,spans_dropped}`) / fixtures **89** (`0087-tracing-otlp`) / fuzzers **48** (UNCHANGED — D-TRACE-FUZZER) / BackendKind **38** (driver-owned receiver) / DECISIONS **ADR-0260** (CLOSED) / **0 new packages, 0 new go.mod modules**.

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/46-tracing/PROGRESS-46.1b.md
git commit -m "phase 46.1b Task 1: PROGRESS scaffold + baselines (span emission + OTLP export; CLOSES ADR-0260)"
```

---

## Task 2: The `OTLPTracesClient` UNARY typed wrapper (`grpcclient.go`) [TDD]

**Files:**
- Modify: `internal/grpcclient/grpcclient.go`
- Test: `internal/grpcclient/grpcclient_test.go`

Mirror `OTLPLogsClient` (`:339`) verbatim — the UNARY `Export` has no stream lifecycle.

- [ ] **Step 1: Write the failing tests** in `grpcclient_test.go` (mirror the existing `OTLPLogsClient` test shape):
  - `NewOTLPTracesClient(nil, "c")` ⇒ error (nil dialer).
  - `NewOTLPTracesClient(dialer, "unknown")` against a manager with no `unknown` cluster ⇒ error (the `DialContext` unknown-cluster gate).
  - `NewOTLPTracesClient(dialer, "h1only")` against a non-H2 cluster ⇒ error naming `http2_protocol_options` (the `UseH2()` gate).
  - a `*OTLPTracesClient` built over a real h2c stub `Export`s an `ExportTraceServiceRequest` and returns the response; `Close()` is idempotent (`sync.Once`); `Export` on a nil/closed client errors cleanly. (Reuse the `OTLPLogsClient` test's in-process gRPC server harness, swapping the service.)

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/grpcclient/ -run TestOTLPTraces -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** in `grpcclient.go` (add `coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"` to the import block; mirror `:327`–`:374`):
```go
type OTLPTracesClient struct {
    conn      *grpc.ClientConn
    stub      coltracepb.TraceServiceClient
    target    string // cluster_name — for logs/errors
    closeOnce sync.Once
    closeErr  error
}
func NewOTLPTracesClient(d *Dialer, clusterName string) (*OTLPTracesClient, error) {
    if d == nil { return nil, fmt.Errorf("grpcclient: new OTLP traces client %q: dialer is nil", clusterName) }
    conn, err := d.DialContext(context.Background(), clusterName)
    if err != nil { return nil, err }
    return &OTLPTracesClient{conn: conn, stub: coltracepb.NewTraceServiceClient(conn), target: clusterName}, nil
}
func (c *OTLPTracesClient) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
    if c == nil || c.stub == nil { return nil, errors.New("grpcclient: Export: nil OTLPTracesClient / stub") }
    return c.stub.Export(ctx, req)
}
func (c *OTLPTracesClient) Close() error {
    if c == nil { return nil }
    c.closeOnce.Do(func() { if c.conn != nil { c.closeErr = c.conn.Close() } })
    return c.closeErr
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/grpcclient/ -run TestOTLPTraces -count=1` ⇒ PASS. Then `go build ./... && go mod tidy -diff` ⇒ build OK, diff EMPTY (the trace collector proto resolves at the already-present module).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/grpcclient/ && golangci-lint run ./internal/grpcclient/... && go vet ./internal/grpcclient/... && go build ./...
git add internal/grpcclient/grpcclient.go internal/grpcclient/grpcclient_test.go
git commit -m "phase 46.1b Task 2: OTLPTracesClient UNARY typed wrapper over TraceService.Export (mirrors OTLPLogsClient; ADR-0158; 0 new go.mod modules)"
```

---

## Task 3: The `Span` model + the 16-attribute roster (`span.go`) [TDD]

**Files:**
- Create: `internal/tracing/span.go`, `internal/tracing/span_test.go`

The per-request SERVER span (§11 D-TRACE-SPAN). Built from a `Decision` + the request/response inputs; converts to `*tracepb.Span`.

- [ ] **Step 1: Write the failing tests** in `span_test.go`:
  - `BuildServerSpan(decision, inputs, end)` with a fresh-trace `Decision` (`Continued:false`, known `TraceID`/`SpanID`, `ParentSpanID` zero, `Sample:true`, `RequestID:"…-9…"`) + `SpanInputs{Method:"GET", URL:"http://h/p", Protocol:"HTTP/1.1", StatusCode:200, UserAgent:"ua", RequestSize:0, ResponseSize:11, UpstreamCluster:"c", DownstreamCluster:"-", ResponseFlags:"-", NodeID:"", Zone:"", PeerAddress:"1.2.3.4", ClientTraceID:""}` + `start`/`end` ⇒ a `*Span` with `Name=="ingress"`, `Kind==SPAN_KIND_SERVER`, `start.Before(end)`, `ParentSpanID == zero`, and `Attrs` containing the deterministic keys with the expected values; `guid:x-request-id` KEY present (value == `RequestID`); `guid:x-client-trace-id` ABSENT (empty input).
  - a continued `Decision` (`ParentSpanID` non-zero) ⇒ the span's `ParentSpanID == decision.ParentSpanID`.
  - a `ClientTraceID:"abc"` input ⇒ `guid:x-client-trace-id` attr PRESENT (value `"abc"`).
  - `(*Span).toProto()` ⇒ a `*tracepb.Span` with `TraceId` (16 bytes == `decision.TraceID`), `SpanId` (8 == `SpanID`), `ParentSpanId` (empty slice when zero / 8 bytes when continued), `Name=="ingress"`, `Kind==tracepb.Span_SPAN_KIND_SERVER`, `StartTimeUnixNano==uint64(start.UnixNano())`, `EndTimeUnixNano==uint64(end.UnixNano())`, and the `Attributes []*commonpb.KeyValue` carrying the deterministic subset (string/int values per `KV`). Assert `request_size`/`response_size`/`http.status_code` marshal as INT attrs; the rest as STRING attrs (confirm the reference's value types at IMPL — the §11 dump shows `http.status_code` as a string in the cpp impl; MATCH whatever the `0087` differential's deterministic-subset comparison treats as equal — the receiver compares decoded values, so pick the type the reference uses; if uncertain, store all as STRING and assert against the reference's decoded string form). 

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run TestSpan -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `span.go`:
  - `type KV struct { Key string; Str string; Int int64; IsInt bool }` (a tiny attr holder) + a `commonpb.KeyValue` builder in `toProto`.
  - `type SpanInputs struct { Method, URL, Protocol string; StatusCode int; UserAgent string; RequestSize, ResponseSize int64; UpstreamCluster, DownstreamCluster, ResponseFlags, NodeID, Zone, PeerAddress, ClientTraceID string }`.
  - `type Span struct { TraceID [16]byte; SpanID [8]byte; ParentSpanID [8]byte; Name string; Kind tracepb.Span_SpanKind; Start, End time.Time; TraceState string; ServiceName string; Attrs []KV }`.
  - `BuildServerSpan(d Decision, in SpanInputs, start, end time.Time) *Span`: assemble the 16-attr roster (`http.method`/`http.url`/`http.protocol`/`http.status_code`/`component`=`proxy`/`upstream_cluster`/`upstream_cluster.name`/`downstream_cluster`/`response_flags`/`request_size`/`response_size`/`user_agent`/`guid:x-request-id`=`d.RequestID`/`node_id`/`zone`/`peer.address`; + `guid:x-client-trace-id` IFF `in.ClientTraceID != ""`); `Name="ingress"`, `Kind=SERVER`, `TraceState=d.TraceState`.
  - `(*Span).toProto() *tracepb.Span`: copy ids (a `[]byte` from the arrays; `ParentSpanId` empty when the array is all-zero — root), times → `uint64(UnixNano())`, attrs → `[]*commonpb.KeyValue`.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run TestSpan -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/span.go internal/tracing/span_test.go
git commit -m "phase 46.1b Task 3: the ingress/SERVER Span model + the 16-attr built-in roster + toProto (D-TRACE-SPAN)"
```

---

## Task 4: The `OTLPExporter` — bounded-channel batching sink (`exporter.go`) [TDD, full-package -race]

**Files:**
- Create: `internal/tracing/exporter.go`, `internal/tracing/exporter_test.go`

Mirror `internal/accesslog/otlpsink.go` (217 LoC) verbatim in shape — swap `LogRecord`→`Span`, `ExportLogsServiceRequest`→`ExportTraceServiceRequest`. This task introduces a BACKGROUND GOROUTINE (the `run()` writer) — the `-race` gate must run the FULL package.

- [ ] **Step 1: Write the failing tests** in `exporter_test.go` (against a `fakeTracesClient` recording `Export` calls + a programmable error):
  - `Export(span)` ×K then `Close()` ⇒ the fake received exactly K spans aggregated across `Export` calls (assert the AGGREGATE, not the per-call framing); `spans_sent == K`.
  - size-trigger: with a tiny `bufferSizeBytes`, a batch flushes mid-stream (≥2 `Export` calls for K large spans) — assert the AGGREGATE count, not the call count.
  - interval-trigger: with a short `bufferFlushInterval` + 1 span + no further input ⇒ the span flushes on the timer tick (poll the fake to converge — no `Sleep`-assert).
  - retry-once: the fake errors on attempt 1, succeeds on attempt 2 ⇒ the batch is sent, `spans_sent += len(batch)`; the fake errors TWICE ⇒ the batch is dropped (logged-not-counted), `spans_sent` unchanged, memory bounded (`buf` reset).
  - drop-newest: fill the channel past capacity (a blocked fake) ⇒ overflow spans increment `spans_dropped` (the `Submit` default-case).
  - `Close()` is idempotent (`sync.Once`); a second `Close()` is a no-op; `Close()` drains the pending buffer (a buffered-but-unflushed span reaches the fake before `Close` returns).
  - the flush builds ONE `ExportTraceServiceRequest{ResourceSpans:[{Resource{service.name==<configured>}, ScopeSpans:[{Scope{name,version}, Spans:[batch]}]}]}` — assert `service.name` on the built request + that all batch spans land under the single ScopeSpans.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run TestExporter -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `exporter.go` (the `otlpsink.go` structure verbatim):
  - `type Exporter interface { Export(span *Span); Close() error }`.
  - `type otlpTracesClient interface { Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error); Close() error }` (the `*grpcclient.OTLPTracesClient` satisfies it structurally — no import cycle).
  - `type OTLPExporter struct { ch chan *Span; client otlpTracesClient; serviceName string; scopeName, scopeVersion string; spansSent, spansDropped *stats.Counter; done chan struct{}; closeOnce sync.Once; closeErr error; bufferSizeBytes int; bufferFlushInterval time.Duration; ctx context.Context; cancel context.CancelFunc; lastDropLog atomic.Int64 }`.
  - `NewOTLPExporter(client, serviceName string, counters *TracerCounters, bufferSizeBytes int, bufferFlushInterval time.Duration) *OTLPExporter` (+ a `newOTLPExporterWithCapacity` test variant) → `go s.run()`.
  - `Export(span *Span)`: non-blocking `select { case s.ch <- span: default: s.spansDropped.Inc() + rate-limited log }` (the `Submit` shape).
  - `run()`: the `otlpsink.go:155` loop verbatim — `buf []*tracepb.Span` (convert each `*Span` via `toProto()` at append; or buffer `*Span` and convert at flush — locked: convert at flush so `proto.Size` for the size-trigger measures the wire span), size/interval/close triggers, `flush()` builds the request via `buildExportTraceRequest(buf, serviceName, scopeName, scopeVersion)`, `Export` with retry-once, `spansSent.Add(len(buf))` on success, `buf = buf[:0]`.
  - `buildExportTraceRequest(spans []*tracepb.Span, serviceName, scopeName, scopeVersion string) *coltracepb.ExportTraceServiceRequest`: one `ResourceSpans{Resource:{Attributes:[service.name=serviceName]}, ScopeSpans:[{Scope:{Name:scopeName,Version:scopeVersion}, Spans:spans}]}`.
  - `Close()`: the `otlpsink.go:135` `sync.Once` shutdown (close `ch`, await `done` with a drain grace, `cancel()`, `client.Close()`).

- [ ] **Step 4: Run to verify they pass + the FULL-package -race** — `go test ./internal/tracing/ -run TestExporter -count=1` ⇒ PASS; then `go test ./internal/tracing/ -race -count=1` ⇒ PASS (the full package — the writer goroutine is a background mutator; a subset `-race` would miss a cross-test race, `reference_full_suite_race_after_background_mutator`).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/exporter.go internal/tracing/exporter_test.go
git commit -m "phase 46.1b Task 4: OTLPExporter — bounded-channel + writer-goroutine + size/interval/close batching over TraceService.Export + retry-once + drop-newest + idempotent Close (mirrors otlpsink.go; D-TRACE-OTLP-WIRE)"
```

---

## Task 5: The tracer-scoped counters (`tracing.opentelemetry.{spans_sent,spans_dropped}`) [TDD, +2 → 1198]

**Files:**
- Modify: `internal/tracing/stats.go`, `internal/tracing/stats_test.go`

- [ ] **Step 1: Write the failing test** — `RegisterTracerCounters(reg)` returns a non-nil `*TracerCounters` and the registry gains EXACTLY 2 counters named `tracing.opentelemetry.spans_sent` + `tracing.opentelemetry.spans_dropped` (assert the count DELTA == 2). `IncSent(3)` adds 3 to `spans_sent`; `IncDropped()` increments `spans_dropped`. (These are STATIC names — no dynamic segment, no `IsValidName` guard needed; unlike the HCM-scoped `tracing.*`.) **Test-isolation caution (PLAN-review #6):** `stats.Registry.NewCounter` PANICS on a duplicate name — each sub-test that registers MUST use a FRESH `stats.NewRegistry()` (a second `RegisterTracerCounters` against the same registry panics on the static names).

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/tracing/ -run TestTracerCounters -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** in `stats.go`:
```go
type TracerCounters struct { spansSent, spansDropped *stats.Counter }
func RegisterTracerCounters(reg *stats.Registry) *TracerCounters {
    return &TracerCounters{
        spansSent:    reg.NewCounter("tracing.opentelemetry.spans_sent"),
        spansDropped: reg.NewCounter("tracing.opentelemetry.spans_dropped"),
    }
}
func (c *TracerCounters) IncSent(n int)  { if c != nil { c.spansSent.Add(uint64(n)) } }
func (c *TracerCounters) IncDropped()    { if c != nil { c.spansDropped.Inc() } }
```
(Wire `OTLPExporter`'s `spansSent`/`spansDropped` fields to these in Task 4's struct — adjust Task 4 to take a `*TracerCounters` and call `IncSent`/`IncDropped`, OR hold the two `*stats.Counter` directly; locked: the exporter holds the two `*stats.Counter` extracted from `*TracerCounters` at construction, keeping `run()` allocation-free.)

- [ ] **Step 4: Run to verify it passes** — `go test ./internal/tracing/ -run TestTracerCounters -count=1` ⇒ PASS. Record the +2 surface delta (1196 → 1198) in PROGRESS-46.1b.md.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go build ./...
git add internal/tracing/stats.go internal/tracing/stats_test.go
git commit -m "phase 46.1b Task 5: RegisterTracerCounters — 2 process-global tracing.opentelemetry.{spans_sent,spans_dropped} (+2 → 1198; D-TRACE-STATS-FINAL; timer_flushed dropped)"
```

---

## Task 6: The `ExporterProvider` + the boot-reject cluster gate (`exporter.go`) [TDD]

**Files:**
- Modify: `internal/tracing/exporter.go`
- Test: `internal/tracing/exporter_test.go`

The boot-built memoizer that hands each HCM filter the exporter for its collector cluster (D-TRACE-EXPORTER-WIRING). The cluster-exists / non-H2 gate lives in the injected dialer seam (the `log.Fatalf` departure surfaces as a returned error).

- [ ] **Step 1: Write the failing tests** (against a `fakeDialer` implementing `tracesClientDialer`):
  - `ExporterFor("c", "svc")` on a provider whose dialer returns a fake client ⇒ a non-nil `Exporter`; a SECOND `ExporterFor("c", "svc")` returns the SAME exporter (memoized per cluster — assert pointer identity).
  - `ExporterFor("bad", "svc")` whose dialer returns an error ⇒ `(nil, error)` (the boot-reject — the unknown/non-H2 cluster departure).
  - the tracer counters register LAZILY on the FIRST successful `ExporterFor` (a registry passed to the provider gains the +2 only after the first build; a provider that never builds leaves the surface at 1196 — the byte-stable guard).
  - `CloseAll()` closes every built exporter (idempotent; a second `CloseAll()` is a no-op).
  - **Test-isolation caution (PLAN-review #6):** each sub-test uses a FRESH `stats.NewRegistry()` — the lazy `sync.Once` register guards intra-provider duplicates, but two providers sharing one registry would panic on the static counter names.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run TestExporterProvider -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** in `exporter.go`:
```go
// TracesClient is the EXPORTED span-export seam (PLAN-review #7 — pin this name
// ONCE; main.go's adapter returns it; *grpcclient.OTLPTracesClient satisfies it
// structurally, so internal/tracing never imports internal/grpcclient).
type TracesClient interface {
    Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error)
    Close() error
}
type tracesClientDialer interface { NewTracesClient(clusterName string) (TracesClient, error) }
// (otlpTracesClient from Task 4 == TracesClient; collapse to the one exported name.)

type ExporterProvider struct {
    dialer    tracesClientDialer
    reg       *stats.Registry
    once      sync.Once
    counters  *TracerCounters
    bufBytes  int
    bufFlush  time.Duration
    mu        sync.Mutex
    byCluster map[string]*OTLPExporter
}
func NewExporterProvider(d tracesClientDialer, reg *stats.Registry, bufBytes int, bufFlush time.Duration) *ExporterProvider { … byCluster: map{} … }

func (p *ExporterProvider) ExporterFor(clusterName, serviceName string) (Exporter, error) {
    p.mu.Lock(); defer p.mu.Unlock()
    if e, ok := p.byCluster[clusterName]; ok { return e, nil }
    client, err := p.dialer.NewTracesClient(clusterName) // the unknown/non-H2 cluster gate → error
    if err != nil { return nil, err }
    p.once.Do(func() { p.counters = RegisterTracerCounters(p.reg) }) // lazy: +2 only on first build
    e := NewOTLPExporter(client, serviceName, p.counters, p.bufBytes, p.bufFlush)
    p.byCluster[clusterName] = e
    return e, nil
}
func (p *ExporterProvider) CloseAll() error { … range byCluster → Close() … }
```
(Confirm the buffer defaults — reuse the OTLP-log defaults; a PLAN-acceptable starting point: `bufBytes` 0 ⇒ flush-every-span is too chatty for the differential's poll-to-converge; pick the 45.1 OTLP-log default sizing + a short flush interval so the `0087` poll converges promptly. Lock the exact defaults at IMPL against the `0084` timing.)

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run TestExporterProvider -count=1` ⇒ PASS; then `go test ./internal/tracing/ -race -count=1` ⇒ PASS (the provider mutex + the exporter goroutines).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/exporter.go internal/tracing/exporter_test.go
git commit -m "phase 46.1b Task 6: ExporterProvider — per-cluster memoized OTLPExporter + lazy tracer-counter register + the unknown/non-H2 cluster boot-reject gate (D-TRACE-EXPORTER-WIRING; AMEND-TRACE-NO-BOOT-REJECT departure)"
```

---

## Task 7: Thread the exporter into the HCM `Filter` (`config.go` + `filter.go` + `builtins.go`) [TDD]

**Files:**
- Modify: `internal/filter/hcm/config.go`, `internal/filter/hcm/filter.go`, `internal/filter/network/builtins/builtins.go`
- Test: `internal/filter/hcm/config_test.go`

- [ ] **Step 1: Write the failing tests** in `config_test.go`:
  - **accept**: an HCM with a `tracing` block (OTel provider, cluster `c`, service `svc`) parsed with a provider that returns a fake exporter for `c` ⇒ the `Filter.exporter` is non-nil (the looked-up exporter).
  - **boot-reject**: the SAME HCM parsed with a provider whose `ExporterFor("c", …)` returns an error ⇒ `parseFilterWithCtx` returns an error (`hcm: tracing exporter: …` — the bubbled boot-reject; this is what `main.go` `log.Fatalf`s on). **This is the subject-side boot-reject coverage** (SPEC §8.1 — a unit test, NOT a differential dir).
  - **byte-stable (no tracing)**: an HCM with NO `tracing` block ⇒ `Filter.exporter == nil`, `ExporterFor` never called (assert the fake provider recorded zero calls).
  - **no-provider-wired**: a tracing HCM parsed with a `nil` `ExporterProvider` ⇒ a clean error or a documented skip (locked: a `nil` provider with a configured tracing HCM is a boot misconfiguration — error `hcm: tracing configured but no exporter provider wired`; in production the provider is always built when the Dialer is, so this is defensive).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/filter/hcm/ -run TestParseFilter -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement**:
  - `builtins.go`: add `TracingExporters *tracing.ExporterProvider` to `Deps`; pass `deps.TracingExporters` into `hcm.NewNetworkFactory(...)`.
  - `filter.go`: add the `*tracing.ExporterProvider` param to `NewNetworkFactory` + the Filter constructor; thread to `parseFilterWithCtx`.
  - `config.go`: add the `exporter tracing.Exporter` Filter field; in `parseFilterWithCtx`, after the `tcfg`/`tcounters` block (`:308`–`:317`), when `tcfg != nil`:
    ```go
    if provider == nil {
        return nil, fmt.Errorf("hcm: tracing configured but no exporter provider wired")
    }
    exporter, err := provider.ExporterFor(tcfg.ClusterName, tcfg.ServiceName)
    if err != nil { return nil, fmt.Errorf("hcm: tracing exporter: %w", err) }
    ```
    Set `exporter: exporter` on the `Filter` literal (`:320`–`:338`).
  - **The provider flows through ONE seam only** (PLAN-review #2): the HCM filter factory closure is created EXACTLY ONCE — in `main.go`'s `builtins.RegisterBuiltins(netReg, Deps{… TracingExporters: provider})` (`main.go:263`) → `hcm.NewNetworkFactory(…, provider)`. The listener manager (`listener.NewManagerWithBaseDirAndAllowH2C`, `main.go:273`) resolves HCM via the FROZEN `netReg` it receives — it does NOT build the factory itself, so its signature needs NO new param. Do NOT add a tracing param to `NewManagerWithBaseDirAndAllowH2C` (that would be dead). Existing `NewNetworkFactory` call sites in tests/secondary constructors pass `nil` (the no-tracing default); the manager's thin convenience constructors that build their own `netReg` pass a no-tracing `builtins.Deps` (nil provider) — leave them.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/filter/hcm/ -run TestParseFilter -count=1` ⇒ PASS; `go build ./...` ⇒ OK (all `NewNetworkFactory`/constructor call sites updated).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/filter/hcm/ internal/filter/network/builtins/ && golangci-lint run ./internal/filter/hcm/... ./internal/filter/network/builtins/... && go vet ./internal/filter/hcm/... && go build ./...
git add internal/filter/hcm/config.go internal/filter/hcm/filter.go internal/filter/network/builtins/builtins.go internal/filter/hcm/config_test.go
git commit -m "phase 46.1b Task 7: thread ExporterProvider → Filter.exporter via builtins.Deps + parseFilterWithCtx cluster lookup; the boot-reject bubbles as a parse error (subject-side boot-reject coverage)"
```

---

## Task 8: Span-end wiring — carry the `Decision` to `accesslog_emit.go` + build/sample/export the span (H1+H2) [TDD, byte-stability guard]

**Files:**
- Modify: `internal/filter/hcm/connection.go`, `internal/filter/hcm/h2dispatch.go`, `internal/filter/hcm/accesslog_emit.go`
- Test: `internal/filter/hcm/*_test.go`

- [ ] **Step 1: Write the failing tests** — drive a request through the HCM filter with a tracing config + a FAKE exporter (recording exported spans) + a fake `RandSource` forcing a Sampled decision:
  - **sampled-exports**: a request with no incoming trace headers ⇒ exactly ONE span reaches the fake exporter; its `Name=="ingress"`, `Kind==SERVER`, `start<end`, `TraceID`/`SpanID` == the injected `traceparent`'s ids (the SAME `Decision` drove both the upstream header AND the span — assert the span's `trace_id` matches the upstream `traceparent` trace-id captured from the request).
  - **not-sampled-no-export**: a fake `RandSource` forcing NOT sampled (`RandomSampling:0`) ⇒ NO span exported (the `x-request-id`/`traceparent` still inject — 46.1a — but `decision.Sample==false` ⇒ no span).
  - **continued**: an incoming `Traceparent: 00-<fixed>-<fixedparent>-01` ⇒ the exported span's `trace_id == <fixed>`, `parent_span_id == <fixedparent>`.
  - **cancel-no-span**: a `statusCode == 0` (ctx-cancel) emit ⇒ NO span (matches the access-log's own skip).
  - **byte-stable (no tracing)**: the SAME request through a filter with `tracingConfig==nil`/`exporter==nil` ⇒ NO span, NO exporter call, headers untouched (the regression guard).
  - Cover BOTH H1 (`connection.go`/`emitAccessLog`) and H2 (`h2dispatch.go`/`emitAccessLogH2`).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/filter/hcm/ -run TestSpanEmit -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement**:
  - `connection.go:509`: capture the decision —
    ```go
    var traceDecision *tracing.Decision
    if f.tracingConfig != nil {
        d := tracing.Decide(req.Header, f.tracingConfig, f.rng)
        req.Header.Set("X-Request-Id", d.RequestID)
        tracing.InjectTraceparent(req.Header, d.TraceID, d.SpanID, d.Sample, d.TraceState)
        f.tracingCounters.Record(d.Class)
        traceDecision = &d
    }
    ```
    Pass `traceDecision` to the `emitAccessLog(...)` call(s) in `dispatchRequest`.
  - `h2dispatch.go:410`: set `action.traceDecision = &d` on the `chainDispatchAction` (add the field to the struct `:200`–`:261`); pass to `emitAccessLogH2`.
  - `accesslog_emit.go`: add a trailing `traceDecision *tracing.Decision` param to BOTH `emitAccessLog` (`:19`) and `emitAccessLogH2` (`:45`). **Place the span block at the FUNCTION HEAD — BEFORE the `if statusCode == 0 || len(f.accessLog) == 0 { return }` early return (`:20`/`:46`)** — so it does NOT inherit the access-log-sink gate (PLAN-review #1; the `0087` fixture has no `access_log` block):
    ```go
    if statusCode != 0 && f.exporter != nil && traceDecision != nil && traceDecision.Sample {
        in := tracing.SpanInputs{ /* method/url/protocol/status/ua/sizes/clusters/flags/node/peer/client-trace-id from r|req + statusCode + bytesSent + picked + respHeaders */ }
        f.exporter.Export(tracing.BuildServerSpan(*traceDecision, in, start, time.Now()))
    }
    if statusCode == 0 || len(f.accessLog) == 0 { return } // the EXISTING access-log guard, unchanged
    ```
    Source `UpstreamCluster` from `picked`/the route action (the same derivation `%UPSTREAM_CLUSTER%` uses — confirm the accessor at IMPL); `request_size` from the request content-length/observed body; `response_size` from `bytesSent`; `http.url` from `scheme://authority/path` (H1 from `r`; H2 from the `h2.H2Request` pseudo-headers); `user_agent` from the request UA header; `node_id`/`zone` from the boot node (empty when absent — UNasserted); `peer.address` from `downstream`/the conn remote (UNasserted).
  - **Update ALL emit call sites** (PLAN-review #4): the new trailing `traceDecision` param touches EVERY caller. H1 `dispatchRequest` has 5 `emitAccessLog` sites (`connection.go:323, 443, 554, 655, 733`); H2 `WriteH2` has 6+ `emitAccessLogH2` sites (`h2dispatch.go:305, 387, 497, 543, 549, …`). The POST-Decide sites (`:554`/`:655`/`:733` H1; `:497`/`:543`/`:549` H2) pass `traceDecision` (= `&d`); the PRE-Decide 404/500 sites (`:323`/`:443` H1; `:305`/`:387` H2) run BEFORE the Decide block and pass `nil` (no span — consistent with the documented 404-injection-gap follow-on; the H1 frame-local `var traceDecision *tracing.Decision` is `nil` at those sites by construction). A missed site is a compile error (self-catching), but enumerate them so the subagent updates all in one pass. Re-confirm the exact site line numbers at IMPL.

- [ ] **Step 4: Run to verify they pass + the FULL-package -race** — `go test ./internal/filter/hcm/ -run TestSpanEmit -count=1` ⇒ PASS; then `go test ./internal/filter/hcm/ ./internal/tracing/ -race -count=1` ⇒ PASS (the exporter goroutine is shared across requests — the FULL packages, `reference_full_suite_race_after_background_mutator`).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/filter/hcm/ && golangci-lint run ./internal/filter/hcm/... && go vet ./internal/filter/hcm/... && go build ./...
git add internal/filter/hcm/connection.go internal/filter/hcm/h2dispatch.go internal/filter/hcm/accesslog_emit.go internal/filter/hcm/*_test.go
git commit -m "phase 46.1b Task 8: span-end wiring — carry the Decision to accesslog_emit.go (H1 frame-local / H2 chainDispatchAction); build+sample+export the ingress span at stream-complete; not-sampled/cancel/no-tracing ⇒ no span (AMEND-TRACE-SPANEND-SEAM; byte-stable)"
```

---

## Task 9: The boot wiring — `ExporterProvider` build + `Dialer` hoist + close-LIFO (`main.go`) [TDD where feasible]

**Files:**
- Modify: `cmd/envoy-go/main.go`
- Test: a `cmd/envoy-go` boot smoke test if one exists; else covered by the `0087` differential boot + the Task-6/7 unit tests.

- [ ] **Step 1: Implement** in `main.go`:
  - **HOIST `dialer` to function scope + build the provider UNCONDITIONALLY** (PLAN-review #3). Today `dialer := grpcclient.New(cm)` is scoped INSIDE the `if len(bs.ALSConfigs) > 0 || len(bs.OTLPConfigs) > 0 {` gate (`:130`). The bootstrap exposes NO `bs.TracingConfigs` (the tracing parse lives only in `parseFilterWithCtx`), so main.go cannot gate on "tracing configured." Resolution: lift `dialer := grpcclient.New(cm)` to BEFORE `:129` (build it always — it is cheap and lazy-dialing), keep the ALS/OTLP-log sink builds inside their existing `len(...)>0` guards (reusing the hoisted `dialer`), and build `provider := tracing.NewExporterProvider(tracesDialerAdapter{dialer}, bs.Stats, bufBytes, bufFlush)` UNCONDITIONALLY. The provider is INERT until `ExporterFor` is called during filter-parse (no exporter, no goroutine, no counter), so an always-built provider does NOT perturb the no-tracing stat surface (the lazy `sync.Once` counter register — Task 6 — guarantees it; verified by Step 2).
  - Define the `tracesDialerAdapter` satisfying the EXPORTED seam interface pinned in Task 6 (`tracing.TracesClient` — PLAN-review #7): `type tracesDialerAdapter struct{ d *grpcclient.Dialer }; func (a tracesDialerAdapter) NewTracesClient(c string) (tracing.TracesClient, error) { return grpcclient.NewOTLPTracesClient(a.d, c) }`. The `*grpcclient.OTLPTracesClient` satisfies `tracing.TracesClient` structurally — NO `tracing → grpcclient` import (the acyclic seam). Use the SAME interface name Task 6 commits to (do not invent a second shape).
  - Thread `provider` into `builtins.Deps{TracingExporters: provider}` (`:263`). Do NOT add a param to `listener.NewManagerWithBaseDirAndAllowH2C` (`:273`) — the manager resolves HCM via the frozen `netReg`, which already carries the provider-closed factory (PLAN-review #2).
  - Join `provider.CloseAll()` to the defer-LIFO (`:156` block) so the exporter goroutines flush-then-stop on shutdown (BEFORE the listener manager stops — the writer goroutine is a background mutator).

- [ ] **Step 2: Build + the no-tracing surface guard** — `go build ./...` ⇒ OK. Run the full existing differential subset for a NON-tracing OTLP fixture (e.g. `0084`) to confirm the always-built provider did NOT perturb the access-log path: `go test ./test/differential/ -run 'TestDifferential/0084' -count=1` ⇒ PASS.

- [ ] **Step 3: Per-task gates + commit**
```bash
gofmt -l cmd/envoy-go/ && golangci-lint run ./cmd/... && go vet ./cmd/... && go build ./...
git add cmd/envoy-go/main.go
git commit -m "phase 46.1b Task 9: boot wiring — build the ExporterProvider over the hoisted Dialer + thread via builtins.Deps + the listener manager + join CloseAll to the defer-LIFO (inert until a tracing HCM parses)"
```

---

## Task 10: The driver-owned `test/helpers/otlptrace` receiver [TDD-ish]

**Files:**
- Create: `test/helpers/otlptrace/otlptrace.go`, `test/helpers/otlptrace/otlptrace_test.go`

Mirror `test/helpers/otlplogs/otlplogs.go` (213 LoC) verbatim — swap the logs service for the trace service.

- [ ] **Step 1: Write the failing test** — start a `Server`, dial it as a `TraceService` client, `Export` an `ExportTraceServiceRequest` with K spans across J `ResourceSpans` ⇒ `Count() == K`, `Spans()` returns the K spans (aggregated across the nesting), `ResourceAttributes()` returns the J resource attr-sets, `Reset()` clears, `Addr()` is the bound `host:port`, `Stop()` is idempotent.

- [ ] **Step 2: Run to verify it fails** — `go test ./test/helpers/otlptrace/ -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `otlptrace.go` (the `otlplogs.go` shape):
```go
type Server struct {
    coltracepb.UnimplementedTraceServiceServer
    addr string; lis net.Listener; grpcSrv *grpc.Server
    mu sync.RWMutex; spans []*tracepb.Span; resAttrs [][]*commonpb.KeyValue
    stopOnce sync.Once
}
func (s *Server) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
    s.mu.Lock()
    for _, rs := range req.GetResourceSpans() {
        s.resAttrs = append(s.resAttrs, rs.GetResource().GetAttributes())
        for _, ss := range rs.GetScopeSpans() { s.spans = append(s.spans, ss.GetSpans()...) }
    }
    s.mu.Unlock()
    return &coltracepb.ExportTraceServiceResponse{}, nil
}
// + New()/Start (RegisterTraceServiceServer) + Spans()/Count()/ResourceAttributes()/Reset()/Addr()/Stop()
```

- [ ] **Step 4: Run to verify it passes** — `go test ./test/helpers/otlptrace/ -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/otlptrace/ && golangci-lint run ./test/helpers/otlptrace/... && go build ./...
git add test/helpers/otlptrace/
git commit -m "phase 46.1b Task 10: driver-owned test/helpers/otlptrace TraceService receiver — Export accumulator + Spans/Count/ResourceAttributes/Reset/Addr/Stop poll surface (mirrors otlplogs; BackendKind STAYS 38)"
```

---

## Task 11: The `0087-tracing-otlp` cross-side span differential

**Files:**
- Create: `test/fixtures/0087-tracing-otlp/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the `0087` driver)

Model on `test/fixtures/0084-otlp-access-log/` (the driver-owned-receiver shape + the shared-bridge reachability). One dir, one cross-side assertion branch (the continuation prong rides the same dir — `reference_differential_fixture_dispatch_constraint`).

- [ ] **Step 1: Author the bootstraps** (`envoy.yaml` reference + `envoy-go.yaml` subject): an **H1** downstream listener → a route to a fixed-body backend; the HCM carries a `tracing` block (OTel provider → an OTLP collector cluster pointing at the driver-owned `otlptrace` receiver, h2c via `explicit_http_config.http2_protocol_options{}`, the receiver hostname reachable from the reference container per the `0084` shared-bridge wiring; `random_sampling: { value: 100 }` for deterministic span emission; `service_name: "0087"`; a FIXED `Host` + a query-less path). COPY the `0084` listener/admin/backend/cluster templating; SWAP the access-log OTLP block for the `tracing` block.

- [ ] **Step 2: Author `driver/driver.go`** — copy `0084/driver/driver.go`; `fixtureName = "0087-tracing-otlp"`; `BackendKind()` = the fixed-body backend kind `0084` uses; stand up the `test/helpers/otlptrace` receiver on the shared bridge. Drive: fire **N** (e.g. 8) plain requests + **M** (e.g. 4) requests carrying `Traceparent: 00-<FIXED 32hex>-<FIXED 16hex>-01` against each side; POLL the receiver to converge to N+M spans per side (a release barrier — `reference_concurrency_differential_release_barrier`; never a `Sleep`); snapshot + `Reset()` between sides.

- [ ] **Step 3: Author the assertions** (`AssertStats`, cross-side EXACT on the stable per-span subset — aggregated, NOT framing):
  - span count == N+M (each side); decode-ran proof (count > 0 before asserting).
  - every span: `name=="ingress"`, `kind==SPAN_KIND_SERVER`, `start_time_unix_nano < end_time_unix_nano` (both non-zero), and the deterministic attr subset (`http.method`/`http.url`/`http.protocol`/`http.status_code`/`component=proxy`/`upstream_cluster`(+`.name`)/`downstream_cluster=-`/`response_flags=-`/`request_size`/`response_size`/`user_agent`; the `guid:x-request-id` KEY present) — cross-side EXACT.
  - `ResourceSpans.resource.attributes` carry `service.name == "0087"` (cross-side).
  - continuation prong (the M requests): every resulting span's `trace_id == <the FIXED incoming trace-id>` AND `parent_span_id == <the FIXED incoming parent-id>` (§11 D-TRACE-PROPAGATION).
  - subject `/stats`: `tracing.opentelemetry.spans_sent == N+M`; `tracing.opentelemetry.spans_dropped == 0`.
  - **UNasserted:** the `trace_id`/`span_id` VALUES (except the continuation prong); `start`/`end` time values; `x-request-id`/`peer.address`/`node_id`/`zone` VALUES; the `Export`-call count / per-call batch sizes / connection count (framing — `reference_streaming_sink_differential_framing`); the SDK `telemetry.sdk.*` resource attrs + the `ScopeSpans.scope` name/version (impl-specific — envoy-go is not cpp).
  - `expectations.yaml` minimal (a driver-`AssertStats` fixture, the `0084` shape).

- [ ] **Step 4: Author `README.md`** — the fixture's purpose (span emission + OTLP export), the driver-owned-receiver + shared-bridge capture mechanism, the `random_sampling=100%` determinism note, the continuation prong, the framing-not-asserted note, the SDK/scope-not-asserted note, the decode-ran proof, the one-dir-one-branch note.

- [ ] **Step 5: Register + run isolated.** Add `_ "github.com/esalaine/envoy-go/test/fixtures/0087-tracing-otlp/driver"` to `runner_test.go`'s blank-imports.
Run (`reference_differential_run_selector`): `go test ./test/differential/ -run 'TestDifferential/0087' -count=1` ⇒ PASS (both sides emit N+M `ingress`/SERVER spans with the matching stable subset + continue the fixed trace-id; subject `spans_sent == N+M`).
Confirm: `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` ⇒ **89**.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l test/ && golangci-lint run ./test/... && go build ./...
git add test/fixtures/0087-tracing-otlp/ test/differential/runner_test.go
git commit -m "phase 46.1b Task 11: 0087-tracing-otlp differential — cross-side ingress/SERVER span payload (aggregated, not framing) + trace-id continuation + subject spans_sent via a driver-owned otlptrace receiver on a shared bridge (fixtures 88 → 89)"
```

---

## Task 12: `0087` deliberate-break proofs + flake gate + full 89-dir differential + six-gate + ADR-0260 body + docs (CLOSES the leg)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/46-tracing/PROGRESS-46.1b.md`

- [ ] **Step 1: Deliberate-break proofs** (`-count=1` on EVERY run — `reference_differential_break_protocol_count1`). For EACH, break ONE production line, confirm `0087` FAILS, then `git restore`:
  - (a) Break `BuildServerSpan` to set `Name` to something other than `"ingress"` ⇒ the `name=="ingress"` assertion FAILS.
  - (b) Break `Decide`/the span build to ignore the incoming `traceparent` (always fresh) ⇒ the continuation `trace_id` assertion FAILS on the M-prong.
  - (c) Break the emit-seam export site (skip `f.exporter.Export`) ⇒ the span-count == N+M assertion FAILS (the receiver gets 0 spans) — proves the export site is live.
  - (d) Break `OTLPExporter` `spansSent.Add` ⇒ the subject `spans_sent == N+M` assertion FAILS.
  - (e) Break the sample guard (export even when `!decision.Sample`) — verify with the Task-8 unit test (not-sampled ⇒ no span); note a differential can't prove this negative under `random_sampling=100%`. Record in PROGRESS.
  Run each: `go test ./test/differential/ -run 'TestDifferential/0087' -count=1` ⇒ FAIL, then restore ⇒ PASS. Record in PROGRESS-46.1b.md.

- [ ] **Step 2: Flake gate** — 20 consecutive green runs (the span path has timing — the poll-to-converge must be robust):
```bash
for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0087' -count=1 || { echo "FLAKE at run $i"; break; }; done
```
Expected: 20/20 PASS. (A transient `subject ready: EOF` is the startup-race flake — `reference_differential_fullsuite_startup_flake` — isolate-re-run; NOT an `0087` regression.)

- [ ] **Step 3: The six-gate** (the house completion gate):
```bash
gofmt -l . | tee /dev/stderr | wc -l        # expect 0
golangci-lint run ./...                      # clean
go vet ./...                                  # clean
go build ./...                                # ok
go test ./... -count=1                        # full unit + the 89-dir differential
go test ./internal/tracing/ ./internal/filter/hcm/ -race -count=1   # the exporter goroutine is a background mutator — FULL packages
```
Expected: all green. (The full differential is the byte-stability regression anchor — NO non-tracing fixture moves; the exporter goroutine + the tracer counters exist only under a configured provider.) Confirm `go mod tidy -diff` EMPTY (ZERO new modules).

- [ ] **Step 4: ADR-0260 body** (`DECISIONS.md`) — the §Decision + §Consequences land HERE (ADR-0044; §Context was drafted at SPEC §13). §Decision: the `internal/tracing` engine (the 46.1a `Decide` + the 46.1b span model + the `Exporter` seam + the `OTLPExporter`), the `OTLPTracesClient` UNARY typed wrapper, the `ExporterProvider` boot wiring (per-cluster memoized; the lazy tracer counters; the `log.Fatalf` cluster gate — the AMEND-TRACE-NO-BOOT-REJECT departure), the span-end wiring (carry the `Decision` to `accesslog_emit.go`), the 2 tracer-scoped counters, and the `0087` driver-owned-receiver differential. §Consequences: the row-46 Observability family stays OPEN (the 46.2 Zipkin/B3 leg + `custom_tags`/`spawn_upstream_span`/`http_service`/the `x-envoy-force-trace` force path remain deferred); ZERO new packages/modules; byte-stable when unconfigured. **CLOSES ADR-0260; next-free ADR-0261** (the 46.2 Zipkin+B3 anchor).

- [ ] **Step 5: BEHAVIOR_CONTRACT.md** — EXTEND the 46.1a `### Request tracing` section (or add a `— span emission + OTLP export` subsection): a configured OTel provider builds a per-request `ingress`/SERVER span (the 16 built-in attrs; start at dispatch, end at stream-complete) and EXPORTS sampled spans over the UNARY `TraceService.Export` (the reused size/interval/close batching + retry-once + drop-newest); `log.Fatalf` on a missing/non-H2 collector cluster (the envoy-go-strict departure). Advance the stat-surface block 1196 → 1198 (+2 `tracing.opentelemetry.{spans_sent,spans_dropped}`). Note the 46.1b boundary closure: the no-route-match (404) path injection-gap noted at 46.1a remains a documented follow-on (the span/inject run after route-match).

- [ ] **Step 6: STATE.md + ROADMAP.md + fuzzer reconcile** — STATE active-phase → `phase 46.1b IMPL done`; counts → stat **1198** / fixtures **89** / fuzzers **48** (UNCHANGED) / BackendKind **38** / DECISIONS **ADR-0260** (CLOSED; next-free **ADR-0261**). ROADMAP row 46 STAYS **`in-progress`** (the 46.1 leg is now COMPLETE, but row 46 flips `done` only at the FINAL leg 46.2 IMPL per ADR-0106 + `reference_roadmap_split_phase_row_done`; the Observability family STAYS OPEN); stage-note "46.1 (core+OTLP) COMPLETE [46.1a header engine + 46.1b span+OTLP export]; 46.2 (Zipkin+B3) next." Set the next action → the **46.2 SPEC** (the Zipkin provider + B3 propagation). Reconcile the documented `^func Fuzz` running total stays **48** across STATE/BEHAVIOR_CONTRACT/ROADMAP/PROGRESS (`reference_fuzzer_count_docs_drift`).

- [ ] **Step 7: Commit the completion bundle**
```bash
git add docs/
git commit -m "phase 46.1b (span emission + OTLP export) IMPL: ADR-0260 §Decision/§Consequences body (CLOSES the leg) + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 46 STAYS in-progress; Observability family STAYS OPEN); stat 1198 / fixtures 89 / fuzzers 48 / BackendKind 38 / 0 new packages / 0 new go.mod modules"
```

---

## Final review + handoff

- [ ] **Dispatch the final code-reviewer** over the whole branch diff (the engine→span→exporter→wiring→differential coherence; the byte-stability of the no-tracing path; the exporter goroutine shutdown ordering; the `0087` assertions are live + non-vacuous).
- [ ] **Controller squashes the worktree branch** into ONE atomic commit (subject `phase 46.1b (span emission + OTLP export) IMPL: the ingress/SERVER span model + the 16-attr roster + OTLPTracesClient + OTLPExporter + the span-end wiring + the 2 tracer counters + the 0087 differential — CLOSES ADR-0260`), verifies the main checkout is clean, **pushes to origin** (`feedback_push_to_origin`), and removes the worktree (`superpowers:finishing-a-development-branch`).
- [ ] **Update `next-prompt.txt`** to re-anchor on the 46.1b IMPL squash and route the next session to the **46.2 SPEC** (the Zipkin provider + B3 propagation, ADR-0261; the SECOND exporter behind the `tracing.Exporter` seam; the IMPL of 46.2 flips row 46 → `done` + may CLOSE the Observability family). If 46.2 is the FINAL Observability row, note the family-close at its IMPL.
- [ ] **Counts at IMPL-done (the exit invariant — re-verify, do NOT assume):** stat surface **1198** (H2 cluster; non-H2 **1194** — the +2 tracer counters are process-global, cluster-independent) / fixtures **89** (tail `0087-tracing-otlp`) / fuzzers **48** (UNCHANGED) / BackendKind **38** (driver-owned receiver) / DECISIONS **ADR-0260** (CLOSED; next-free **ADR-0261**). ZERO new Go packages; ZERO new go.mod modules (`go mod tidy -diff` EMPTY).

> **NOTE on the surface figure:** the +2 tracer counters are DYNAMIC (register lazily on the first `ExporterFor`, i.e. only when an HCM `tracing` block is configured + an exporter builds). Assert the +2 DELTA via the Task-5/Task-6 registration tests + the Task-12 six-gate, NOT a brittle absolute. With the 46.1a +5 HCM-scoped `tracing.*`, the full tracing surface is +7 over the pre-46.1 baseline (1191 → 1198) when a tracing HCM is configured.
