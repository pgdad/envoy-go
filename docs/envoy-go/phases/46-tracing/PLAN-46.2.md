# Phase 46.2 Implementation Plan — the Zipkin span exporter + B3 trace-context propagation: a SECOND `tracing.Exporter` behind the 46.1b seam (a Zipkin **v2 JSON** span encoder POSTed over an HTTP/1 `httpclient.ClusterDispatch` to the `collector_cluster`, NOT gRPC) + provider-type dispatch (OpenTelemetry vs Zipkin) in `NewConfig` + the `ExporterProvider` + the `b3` single-header / `X-B3-*` multi-header extract/continue/inject codec (the W3C `traceparent` analog) + the `Decide`-takes-extracted-context refactor + the `tracing.zipkin.*` tracer-scoped stats + the driver-owned `test/helpers/zipkincollector` HTTP receiver + the `0088-tracing-zipkin` cross-side differential + `FuzzExtractB3` — the FINAL chartered tracing leg; ANCHORS ADR-0261; the six-gate FLIPS ROADMAP row 46 (`tracing`) → `done`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan executes in a FRESH git worktree off master (`feedback_git_worktrees`); subagents commit LOCALLY only (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close (`feedback_push_to_origin`). NOTE the execution lesson (`feedback_subagent_autocommit_claudemd`): the global CLAUDE.md makes dispatched subagents AUTO-COMMIT — do NOT fight it; the controller VERIFIES each commit (correct fileset, real non-vacuous tests via `-v` + read assertions, gates green), cleans stray next-task leak files, re-runs the full suite on the FINAL frozen HEAD, does the deliberate-break verification ITSELF, and squashes + pushes at stage-close.

**Goal:** When an HCM `tracing` block carries a recognized **Zipkin** provider (`envoy.config.trace.v3.ZipkinConfig`), envoy-go runs the SAME provider-agnostic sampling/request-id decision the 46.1a engine produces, builds the SAME per-request SERVER span, but (a) extracts/continues/injects the trace context in **B3** (the `b3` single header AND the `X-B3-*` multi-header set — inject is multi-header) instead of W3C `traceparent`, and (b) EXPORTS each sampled span as a Zipkin **v2 JSON** span over an HTTP/1 `POST <collector_endpoint>` to the `collector_cluster` (via `httpclient.Client.ClusterDispatch`, ADR-0177), instead of OTLP unary gRPC. Proven cross-side EXACT on the stable per-span subset (+ a B3 continuation prong) against `contrib-v1.37.2` by the `0088-tracing-zipkin` differential through a driver-owned HTTP Zipkin collector. **ANCHORS ADR-0261** (its §Decision/§Consequences body lands atomically here); **the six-gate FLIPS ROADMAP row 46 → `done`** (the FINAL chartered tracing leg per ADR-0106 + `reference_roadmap_split_phase_row_done`); the Observability family STAYS OPEN.

**Architecture:** ZERO new Go packages (the Zipkin exporter, the B3 codec, and the v2 JSON encoder all live in the existing `internal/tracing`; the receiver is a test-only `test/helpers/zipkincollector`) and ZERO new go.mod modules (the v2 JSON uses stdlib `encoding/json`; `ZipkinConfig` resolves at the already-imported `github.com/envoyproxy/go-control-plane/envoy/config/trace/v3` — the SAME `tracev3` import `config.go:10` already carries; `go mod tidy -diff` anticipated EMPTY). It composes on substrates already built: the 46.1a/46.1b `tracing.Decide`/`Decision`/`Span`/`Exporter` seam/`ExporterProvider`; the phase-22 `httpclient.Client.ClusterDispatch` HTTP-over-cluster primitive (`httpclient.go:269`, ADR-0177 — already consumed by `internal/wasm/http_call.go` and `internal/filter/network/lua`); the `cluster.Manager` (`Get`/`PickEndpoint`); the HCM request-lifecycle seams (dispatch decision at `connection.go:520`/`h2dispatch.go:421`; span-end + export at `accesslog_emit.go:25`/`:84`). 46.2 makes the EXISTING seams polymorphic: `NewConfig` dispatches OTel-vs-Zipkin; `ExporterProvider` builds an `OTLPExporter` (gRPC) OR a `ZipkinExporter` (HTTP); the HCM dispatch block extracts/injects B3-vs-traceparent keyed by the parsed `ProviderKind`. Byte-identical when no `tracing` provider is configured (every non-tracing path untouched — the full differential is the regression anchor; `tracing.zipkin.*` registers only when ≥1 Zipkin provider is built).

**Tech Stack:** Go; the existing `internal/tracing` package (NEW `b3.go` + `zipkin.go`; MODIFY `config.go`/`decision.go`/`span.go`/`exporter.go`/`stats.go`); `internal/filter/hcm` (the provider-aware extract/inject dispatch + the `Span.Authority` carry); `cmd/envoy-go/main.go` (hoist the `*httpclient.Client`; thread a `ZipkinTransport` adapter into `NewExporterProvider`); the driver-owned `test/helpers/zipkincollector` HTTP/JSON receiver (the `test/helpers/otlptrace` analog, HTTP not gRPC); the Docker-bridge differential harness (`reference_docker_probe_bridge_network`, the `0087` precedent). The Zipkin v2 JSON model is stdlib `encoding/json`; `ZipkinConfig` resolves at `go-control-plane/envoy v1.32.4` (already direct). ZERO new go.mod modules (`go mod tidy -diff` anticipated EMPTY).

---

## Orientation — read before Task 1 (the zero-context brief)

You are adding the SECOND exporter to envoy-go's request-tracing subsystem. The FIRST (OTLP/OpenTelemetry over gRPC) ALREADY EXISTS and is complete (phase 46.1, ADR-0260 CLOSED). The decision engine, span model, `Exporter` seam, and `ExporterProvider` are REUSED WHOLESALE — 46.2 adds a bounded delta: one `Exporter` impl, one propagation codec, one config-parse arm, one span encoder, two stats, one differential, one fuzzer.

**What ALREADY works (do NOT re-build):**
- `internal/tracing/decision.go` — `Decide(h, cfg, rng) Decision` runs the provider-AGNOSTIC sampling/request-id precedence (the `x-request-id` byte-14 reason nibble; the three Percent knobs; incoming-context-continues-authoritatively). The `Decision{Sample, Reason, Class, Continued, TraceID[16], SpanID[8], ParentSpanID[8], TraceState, RequestID}` is its output. **TODAY `Decide` extracts `traceparent` INTERNALLY at `decision.go:64` (`ExtractTraceparent(h)`).** This is the ONE thing 46.2 refactors (Task 3) so a B3 caller can supply an already-extracted context.
- `internal/tracing/span.go` — `BuildServerSpan(d, in SpanInputs, start, end) *Span` assembles the single span (`Name="ingress"` constant, the 16-attr roster as `[]KV`). `toProto()` is OTLP-only. 46.2 adds a provider-neutral `Authority` field + a Zipkin v2 JSON encoder (`zipkin.go`) — the `toProto` analog. The Attrs already store `http.status_code`/`request_size`/`response_size` as STRINGS (Zipkin tags are all strings — no conversion).
- `internal/tracing/exporter.go` — `type Exporter interface { Export(span *Span); Close() error }` (`:56`); `OTLPExporter` (the bounded `chan *Span` + writer-goroutine `run()` + size/interval/close buffer + retry-once + drop-newest + idempotent `sync.Once` `Close`, `:73`); `ExporterProvider` (memoizes one exporter per collector cluster, lazy `sync.Once` tracer-counter register, `:243`). **The `ZipkinExporter` is a SECOND impl behind the SAME `Exporter` interface; the `ExporterProvider` gains a Zipkin construction arm.**
- `internal/tracing/config.go` — `NewConfig(t) (*TracingConfig, error)` parses the HCM `tracing` message; TODAY the provider gate at `:70` accepts ONLY `OpenTelemetryConfig` (`otelTypeName`) and rejects every sibling. 46.2 lifts that gate to "OTel OR Zipkin" + adds the Zipkin-specific strict-reject arms.
- `internal/tracing/stats.go` — the 5 HCM-scoped counters (provider-agnostic, unchanged) + `RegisterTracerCounters` (the OTLP `tracing.opentelemetry.{spans_sent,spans_dropped}` pair). 46.2 adds `RegisterZipkinCounters` (the `tracing.zipkin.{spans_sent,spans_dropped}` pair).
- `internal/httpclient/httpclient.go:269` — `(*Client).ClusterDispatch(ctx, clusterName, *http.Request, *cluster.Manager) (*http.Response, error)`: does `clusterMgr.Get(name)` (returns `errClusterNotFound` on a miss), `PickEndpoint()` (LB-selects, rewrites `URL.Host`), honors the cluster's TLS, runs a stdlib HTTP/1 `Do` with the receiver's timeout/retry. **This is the Zipkin transport.** The `internal/wasm/http_call.go` `HTTPDispatcher{HasCluster(name) bool; Dispatch(ctx, cluster, req)}` adapter (`:119`) is the decoupling-seam precedent we mirror.

**The B3 wire model (§11 D-TRACE-ZIPKIN-B3, live-probed against `contrib-v1.37.2` — all pinned):**
- **EXTRACT** accepts EITHER the single `b3` header `<traceid>-<spanid>-<sampled>[-<parentid>]` (traceid 16-or-32 hex; sampled `0`/`1`/`d`) OR the multi-header `X-B3-TraceId` / `X-B3-SpanId` / `X-B3-ParentSpanId` / `X-B3-Sampled` / `X-B3-Flags`. A continued trace continues authoritatively (its sampled bit bypasses the local caps — the SAME precedence `Decide` already implements; provider-agnostic).
- **INJECT** toward the upstream writes the MULTI-HEADER `X-B3-*` set (NOT the single `b3`): `X-B3-TraceId` (hex, width per `trace_id_128bit`), `X-B3-SpanId` (the server span's outbound span-id), `X-B3-Sampled` (`1`/`0`), and `X-B3-ParentSpanId` ONLY when continued.

**The Zipkin v2 JSON span model (§11 D-TRACE-ZIPKIN-WIRE/IDS/SHARED — all pinned):** the exporter POSTs a JSON ARRAY of span objects, `Content-Type: application/json`, to `<collector_endpoint>` on `collector_cluster`. Each span: `traceId` (16/32 hex per `trace_id_128bit`), `id` (16 hex span-id), `parentId` (conditional — see SHARED), `name` = the request **AUTHORITY** (the `Host`/`:authority` value — NOT the OTLP `"ingress"` constant), `kind:"SERVER"`, `timestamp`/`duration` (int64 **MICRO**seconds), `localEndpoint` (env-specific — UNasserted), `shared:true` (only under `shared_span_context`), and `tags` (14 STRING tags = the 46.1b 16-attr roster MINUS `node_id` + `zone`). `shared_span_context` toggles the continued-span model: **true** (Envoy default) ⇒ the server span REUSES the incoming span-id as its own `id` + emits `"shared":true` + NO `parentId`; **false** ⇒ a CHILD span with a fresh `id` + `parentId` = the incoming span-id.

**The transport posture (§11 D-TRACE-ZIPKIN-TRANSPORT/REJECT — pinned):** the collector cluster is plain HTTP/1 (NO `http2_protocol_options{}` — UNLIKE the OTLP h2c requirement). A non-2xx/error from `ClusterDispatch` is the retry-once-then-drop path. A MISSING collector cluster BOOT-REJECTS in the reference (`envoy.tracers.zipkin: unknown cluster '<name>' initializing config` → exit 1) — UNLIKE the permissive OTLP path. So envoy-go's cluster-exists pre-check at exporter build → a `log.Fatalf` boot failure is REFERENCE-PARITY here, NOT a departure to document.

**The differential model (the `0087-tracing-otlp` precedent).** A driver-owned in-process HTTP Zipkin collector (`test/helpers/zipkincollector`, the `test/helpers/otlptrace` analog but HTTP/JSON not gRPC) accumulates every received span across POSTs. BOTH the reference (Docker `contrib-v1.37.2`) and the subject (in-process envoy-go) POST to the SAME collector over a shared Docker bridge (the collector hostname must be reachable from the reference container — `reference_docker_probe_bridge_network`, the `0087` wiring verbatim — but plaintext HTTP/1, simpler than `0087`'s h2c). The driver fires N requests, POLLS the collector until N spans converge (a release barrier — NEVER a `time.Sleep`, `reference_concurrency_differential_release_barrier`), and asserts the per-span PAYLOAD AGGREGATED across POSTs (NOT the POST/batch framing, which varies side-to-side — `reference_streaming_sink_differential_framing`).

### Key source seams (verified at PLAN time against master `5ea88386`; re-confirm line numbers before editing — files evolve)

- **`internal/tracing/decision.go:61`** — `Decide(h http.Header, cfg *TracingConfig, rng RandSource) Decision`. Reads a fresh `SpanID` (`:63`), THEN `ExtractTraceparent(h)` (`:64`). **MODIFIED at 46.2 (Task 3):** split into `DecideWithContext(h, ic TraceContext, continued bool, cfg, rng) Decision` (the body, taking the already-extracted context) + a thin `Decide` wrapper that extracts `traceparent` then delegates. The rng-call ORDER must be preserved verbatim (SpanID read first — extraction consumes no rng — so OTel/`0086`/`0087` stay byte-stable).
- **`internal/tracing/propagation.go:12`** — `TraceContext{TraceID[16], ParentID[8], Sampled bool, TraceState string}` + `ExtractTraceparent`/`InjectTraceparent`. **REUSED by B3** (the B3 path leaves `TraceState` empty). `b3.go` is the sibling codec.
- **`internal/tracing/span.go:21`** — `SpanInputs{...}` + `Span{TraceID, SpanID, ParentSpanID, Name, Kind, Start, End, TraceState, ServiceName, Attrs}` + `BuildServerSpan` (`:57`). **MODIFIED at 46.2 (Task 4):** add `Authority string` to BOTH `SpanInputs` and `Span` (D-TRACE-ZIPKIN-SPAN-NAME — provider-neutral; the OTLP encoder still emits `Name="ingress"`, the Zipkin encoder uses `Authority`).
- **`internal/tracing/config.go:23`** — `TracingConfig{ClientSampling, RandomSampling, OverallSampling float64; ServiceName, ClusterName string}`; `otelTypeName` (`:35`); `NewConfig` (`:44`). **MODIFIED at 46.2 (Task 5):** add `Provider ProviderKind` + `Zipkin *ZipkinSettings`; add `zipkinTypeName`; dispatch the provider arm.
- **`internal/tracing/exporter.go:56`** — `Exporter` interface; `OTLPExporter` (`:73`); `tracesClientDialer` (`:233`); `ExporterProvider` (`:243`); `ExporterFor(clusterName, serviceName)` (`:275`); `CloseAll` (`:295`). **MODIFIED at 46.2 (Tasks 7–8):** add the `ZipkinExporter` + the `ZipkinTransport` seam (`zipkin.go` carries the exporter; `exporter.go` carries the provider arm); generalize `ExporterFor` to take `*TracingConfig` and dispatch on `Provider`.
- **`internal/tracing/stats.go:51`** — `RegisterTracerCounters` (the OTLP pair). **MODIFIED at 46.2 (Task 6):** add `RegisterZipkinCounters(reg) *ZipkinCounters` (the `tracing.zipkin.{spans_sent,spans_dropped}` pair).
- **`internal/filter/hcm/connection.go:520`** (H1 dispatch) — `if f.tracingConfig != nil { d := tracing.Decide(req.Header, …); … InjectTraceparent(…); … traceDecision = &d }`. **MODIFIED at 46.2 (Task 9):** provider-aware extract (`ExtractB3` vs `ExtractTraceparent`) → `DecideWithContext` → provider-aware inject (`InjectB3` vs `InjectTraceparent`).
- **`internal/filter/hcm/h2dispatch.go:421`** (H2 dispatch) — the analogous block over an `http.Header` view + `upsertH2Header` write-back. **MODIFIED at 46.2 (Task 9):** same provider-aware dispatch; B3 inject writes the `x-b3-*` headers via `upsertH2Header`.
- **`internal/filter/hcm/accesslog_emit.go:25`/`:84`** — `emitAccessLog`/`emitAccessLogH2` already build the span at the FUNCTION HEAD (ahead of the `len(f.accessLog)==0` early-return) guarded by `statusCode != 0 && f.exporter != nil && traceDecision != nil && traceDecision.Sample`. **MODIFIED at 46.2 (Task 9):** set `in.Authority = r.Host` (H1) / `req.Authority` (H2) in the `SpanInputs`.
- **`internal/filter/hcm/config.go:334`** — `exporter, err = provider.ExporterFor(tcfg.ClusterName, tcfg.ServiceName)`. **MODIFIED at 46.2 (Task 7):** `provider.ExporterFor(tcfg)` (the whole config — the provider dispatches on `tcfg.Provider`).
- **`cmd/envoy-go/main.go:101`** (`cm`), **`:130`** (`dialer := grpcclient.New(cm)`), **`:131`** (`tracingProvider := tracing.NewExporterProvider(tracesDialerAdapter{dialer}, bs.Stats, 16384, time.Second)`), **`:263`** (`httpClient := httpclient.New(...)`), **`:382`** (`tracesDialerAdapter`). **MODIFIED at 46.2 (Task 10):** HOIST `httpClient` above `:131`; pass a `zipkinTransportAdapter{httpClient, cm}` (+ `cm` if needed) into `NewExporterProvider`. `CloseAll`/the defer-LIFO are UNCHANGED (the `ZipkinExporter.Close()` joins the same path).
- **`internal/filter/network/builtins/builtins.go:45`** — `Deps.TracingExporters *tracing.ExporterProvider` (already threaded). **UNCHANGED** (the provider type is the same; only its internals + constructor grow).
- **`test/helpers/otlptrace/otlptrace.go`** (212 LoC) — the driver-owned OTLP gRPC receiver (`Export` accumulator + `Spans()`/`Count()`/`Reset()`/`Addr()`/`Stop()`). **The template for `test/helpers/zipkincollector`** (HTTP/1 + JSON not gRPC).
- **`test/fixtures/0087-tracing-otlp/`** — the driver-owned-receiver differential shape (`driver/driver.go` firing N requests + polling to converge + cross-side asserts; `envoy.yaml`/`envoy-go.yaml` with the shared-bridge collector reachability). **COPY the directory** (swap the OTLP h2c collector for the Zipkin plaintext HTTP/1 collector; swap the gRPC receiver for the HTTP/JSON receiver).
- **`test/differential/runner_test.go:114`** — the fixture-driver blank-import tail (`_ ".../0087-tracing-otlp/driver"`). Add the `0088` driver import.

### ZipkinConfig proto facts (verified at PLAN time; `go-control-plane/envoy v1.32.4`, already direct — the SAME `tracev3` import `config.go:10` carries)

- `tracev3 "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"`: `(&tracev3.ZipkinConfig{}).ProtoReflect().Descriptor().FullName()` == `envoy.config.trace.v3.ZipkinConfig` (the `zipkinTypeName`).
- Accessors: `GetCollectorCluster() string` (field 1, required), `GetCollectorEndpoint() string` (2, the POST path), `GetTraceId_128Bit() bool` (3), `GetSharedSpanContext() *wrapperspb.BoolValue` (4 — ABSENT ⇒ default `true`), `GetCollectorEndpointVersion() ZipkinConfig_CollectorEndpointVersion` (5), `GetCollectorHostname() string` (6, the POST `Host`), `GetSplitSpansForRequest() bool` (7).
- Enum `ZipkinConfig_CollectorEndpointVersion`: `ZipkinConfig_DEPRECATED_AND_UNAVAILABLE_DO_NOT_USE`=0, `ZipkinConfig_HTTP_JSON`=1, `ZipkinConfig_HTTP_PROTO`=2, `ZipkinConfig_GRPC`=3. Only `HTTP_JSON`(1) is accepted; 0/2/3 are each their own strict-reject arm.
- `go mod tidy -diff` shows NO require change (`ZipkinConfig` is in the already-present submodule; the v2 JSON is stdlib `encoding/json`).

### Discipline (honor on EVERY task)

- **TDD** (`superpowers:test-driven-development`): each code task is failing-test → run-fail → minimal-impl → run-pass → commit.
- **Per-task gates** (`feedback_pertask_gofmt_lint`): `gofmt -l` (empty) + `golangci-lint run` on the touched packages + `go vet` + `go build ./...`.
- **Worktree hygiene** (`feedback_subagent_worktree_detach`/`_path_targeting`): subagents write to the WORKTREE path; the controller verifies the main checkout stays clean + the branch is undetached after each task.
- **Commit locally only** (`feedback_subagents_no_push`); the controller squashes + pushes at stage-close.
- **Differential selector** (`reference_differential_run_selector`): always `-run 'TestDifferential/0088'`, NEVER bare `'0088'` (bare matches ZERO subtests → vacuous green).
- **Break protocol** (`reference_differential_break_protocol_count1`): every deliberate-break verification AND every `-race` run uses `-count=1` (go-test caching serves a stale PASS otherwise).
- **Full-package race** (`reference_full_suite_race_after_background_mutator`): 46.2 ADDS a background mutator (the `ZipkinExporter` writer goroutine). A `-run`-subset `-race` MISSES a data race an earlier test's lingering goroutine causes — the `-race` gate MUST run the FULL `internal/tracing` package (+ `internal/filter/hcm`).
- **Streaming-sink framing** (`reference_streaming_sink_differential_framing`): assert the per-span PAYLOAD aggregated across POSTs, NEVER the POST/batch framing (it varies side-to-side).
- **Driver-owned receiver** (`reference_differential_grpc_receiver_driver_owned`): the HTTP Zipkin collector is a `test/helpers/zipkincollector` server the proxy POSTs to — NOT a runner `BackendKind` (stays 38).
- **Docker bridge** (`reference_docker_probe_bridge_network`): the collector must be reachable from the reference container by hostname over a shared bridge; verify decode RAN (the collector's span count > 0) before trusting a green.
- **Release barrier** (`reference_concurrency_differential_release_barrier`): poll the collector to converge to N spans — never a `time.Sleep`.
- **Wire-format both sides** (`reference_wire_format_both_sides_see_same_bytes`): the Zipkin span JSON is shared — adopt the reference's v2 model verbatim (the §11 live probe is the wire truth).
- **Framework-gap tags** (`reference_tracing_upstream_cluster_framework_gap`): `upstream_cluster`/`upstream_cluster.name` emit EMPTY (the picked-cluster name is not plumbed to the emit seam) and `peer.address` is env-specific — these stay KEY-PRESENT-only / value-UNasserted cross-side.

---

## D-question resolutions (the SPEC §12 D-TRACE-ZIPKIN-* PLAN/IMPL pins — settled here)

**D-TRACE-ZIPKIN-CONFIG-SHAPE → a `ProviderKind` enum on `TracingConfig` + a nil-able `Zipkin *ZipkinSettings` sub-struct; keep the shared sampling/request-id fields flat; reuse `ClusterName` for the collector cluster so the `ExporterFor` keying is provider-uniform.** The shared `ClientSampling`/`RandomSampling`/`OverallSampling`/`ServiceName` stay flat. Add:
```go
type ProviderKind int
const ( ProviderOTel ProviderKind = iota; ProviderZipkin )

type ZipkinSettings struct {
    CollectorEndpoint string // the POST path (collector_endpoint)
    CollectorHostname string // the POST Host; empty ⇒ the cluster name (collector_hostname)
    TraceID128Bit     bool   // 32-hex traceId when true; 16-hex (low 64) otherwise
    SharedSpanContext bool   // absent ⇒ true (the Envoy default)
}
// TracingConfig grows:
//   Provider ProviderKind
//   Zipkin   *ZipkinSettings // non-nil iff Provider == ProviderZipkin
```
`ClusterName` carries the collector cluster on BOTH arms (OTel `grpc_service.envoy_grpc.cluster_name`; Zipkin `collector_cluster`) so `provider.ExporterFor(tcfg)` keys the memoization map uniformly. `ServiceName` stays empty on the Zipkin arm (`ZipkinConfig` has no `service_name`; the Zipkin `localEndpoint.serviceName` derives from the boot node — UNasserted).

**D-TRACE-ZIPKIN-DECIDE-SEAM → lift the trace-context extraction to the HCM caller; `DecideWithContext` takes the already-extracted `(TraceContext, bool)`; `Decide` stays a thin `traceparent`-extracting wrapper (OTel callers + the existing tests unchanged).** The minimal refactor preserves the rng-call order (SpanID read first):
```go
func DecideWithContext(h http.Header, ic TraceContext, continued bool, cfg *TracingConfig, rng RandSource) Decision { /* the current Decide body, minus the internal ExtractTraceparent */ }
func Decide(h http.Header, cfg *TracingConfig, rng RandSource) Decision {
    ic, ok := ExtractTraceparent(h)
    return DecideWithContext(h, ic, ok, cfg, rng)
}
```
The B3 caller does `ic, ok := tracing.ExtractB3(h)` then `tracing.DecideWithContext(h, ic, ok, cfg, rng)`. `h` is still passed (the `x-request-id`/`x-client-trace-id` reads are propagation-format-agnostic).

**D-TRACE-ZIPKIN-SPAN-NAME → a provider-neutral `Authority` field on `SpanInputs` + `Span`; the OTLP encoder keeps `Name="ingress"`, the Zipkin encoder uses `Authority`.** `BuildServerSpan` sets `Span.Authority = in.Authority` and `Span.Name = "ingress"` (unchanged). `toProto` keeps emitting `Name` ("ingress"). The Zipkin encoder emits `name = s.Authority`. The emit seam sets `in.Authority = r.Host` (H1) / `req.Authority` (H2).

**D-TRACE-ZIPKIN-ID-DERIVE → envoy-go generates `SpanID` independently of `TraceID` (the 46.1a engine already does — `Decide` reads a fresh `SpanID` then a fresh `TraceID`); the root `id`==low64(`traceId`) reference self-consistency is NOT reproduced and NOT asserted (the cross-side `id`/`traceId` VALUE is UNasserted anyway).** Under `trace_id_128bit:false` the Zipkin `traceId` is the LOW 64 bits of `Decision.TraceID` (the last 8 bytes → 16 hex); under `:true` it is the full `[16]byte` (32 hex). The `id` is `Decision.SpanID` (16 hex) — EXCEPT under `shared_span_context:true` + continued, where the server REUSES the incoming span-id (`Decision.ParentSpanID`) as its `id`. A single helper `zipkinIdentity(d, id128, shared) (traceID, id, parentID string, emitShared bool)` centralizes this so the encoder AND `InjectB3` agree on the outbound span identity.

**D-TRACE-ZIPKIN-TRANSPORT-WIRING → a `ZipkinTransport` interface seam (the `wasm.HTTPDispatcher` analog) so `internal/tracing` never imports `internal/httpclient`/`internal/cluster`; `main.go` supplies a `zipkinTransportAdapter{httpClient, cm}`.** Mirroring the 46.1b `TracesClient`-decoupling AND the `wasm.HTTPDispatcher` precedent:
```go
// in internal/tracing — the transport seam (no httpclient/cluster import).
type ZipkinTransport interface {
    HasCluster(name string) bool                                                  // the boot-reject pre-check (clusterMgr.Get)
    Dispatch(ctx context.Context, clusterName string, req *http.Request) (*http.Response, error) // httpClient.ClusterDispatch
}
```
`main.go` supplies `zipkinTransportAdapter{c *httpclient.Client; cm *cluster.Manager}` with `HasCluster(n){ _, ok := a.cm.Get(n); return ok }` and `Dispatch(ctx, n, req){ return a.c.ClusterDispatch(ctx, n, req, a.cm) }`. The `*http.Request` is built per-flush: `POST`, `URL.Path = CollectorEndpoint`, `Host = CollectorHostname‖clusterName`, `Content-Type: application/json`, body = the v2 JSON span array. Retry/timeout reuse the `httpClient`'s `Options` (the 30 s default) — no tracing-specific policy. A non-2xx OR a `Dispatch` error is the retry-once-then-drop path (the `OTLPExporter` retry shape verbatim). `NewExporterProvider` grows a `ZipkinTransport` param (nil-able; only the Zipkin arm consults it).

**D-TRACE-ZIPKIN-RECEIVER-WIRING → the `0087` precedent verbatim, HTTP/1 not gRPC.** `test/helpers/zipkincollector` is an in-process `net/http` server whose handler reads + de-chunks the POST body, `json.Unmarshal`s a `[]zipkinSpan`, and accumulates every span (across POSTs), exposing `Spans()`/`Count()`/`Reset()`/`Addr()`/`Stop()`. The `0088` shared-bridge + collector-hostname reachability (from the reference container) COPIES `0087` — but the collector cluster is plaintext HTTP/1 (NO TLS, NO h2c), simpler than `0087`. The continuation prong asserts the RECEIVED span's `traceId`/`parentId`/`shared` directly (cross-side); the upstream-injected `X-B3-*` is verified by a SUBJECT-side `InjectB3` unit test (Task 2) — NO backend-echo helper needed (the 46.1b precedent).

**D-TRACE-ZIPKIN-STATS-FINAL → +2 tracer-scoped `tracing.zipkin.{spans_sent,spans_dropped}`; surface 1198 → 1200 (non-H2 1194 → 1196). `reports_*`/`timer_flushed` DROPPED.** Mirrors the 46.1b `tracing.opentelemetry.{spans_sent,spans_dropped}` choice exactly: `spans_sent` += `len(batch)` per successful POST; `spans_dropped` ++ per channel-full overflow drop. The reference's `reports_sent`/`reports_failed` (POST-level counters) carry NO cross-side value (the differential asserts payload not POST framing — `reference_streaming_sink_differential_framing`) and are NOT emitted. Register LAZILY (provider `sync.Once` on the Zipkin arm) so a no-Zipkin boot has zero `tracing.zipkin.*` surface. Confirm the +2 delta via a registration test, not a brittle absolute.

**D-TRACE-ZIPKIN-ANNOTATIONS → OMIT the `"ss"` annotation.** Timing-only, no cross-side-deterministic value; the v2 JSON collector tolerates its absence (`annotations` is optional in the model). Emitting it would only add an UNasserted field.

**D-TRACE-ZIPKIN-FUZZER → land `FuzzExtractB3` at Task 2; fuzzers 48 → 49.** The `b3`/`X-B3-*` extraction is a wire-derived untrusted-input boundary (the `FuzzExtractTraceparent` analog). The Zipkin v2 JSON ENCODER consumes TRUSTED internal `Span` data (not untrusted input) — NO encoder fuzzer (the `toProto`-no-fuzzer precedent). Re-verify `grep -rh '^func Fuzz' --include='*.go' . | wc -l == 49` at the completion task, reconciling the documented-vs-actual count (`reference_fuzzer_count_docs_drift`).

---

## File structure (decomposition locked here)

**Production (created):**
- `internal/tracing/b3.go` — `ExtractB3(h http.Header) (TraceContext, bool)` (single `b3` OR multi `X-B3-*`) + `InjectB3(h http.Header, d Decision, id128, shared bool)` (the multi-header `X-B3-*` set).
- `internal/tracing/zipkin.go` — `zipkinSpan` (the v2 JSON struct, `encoding/json` tags) + `encodeZipkinSpans(spans []*Span, id128, shared bool) ([]byte, error)` + `zipkinIdentity(...)` (the shared traceId/id/parentId/shared derivation) + the `ZipkinExporter` (`Exporter` impl) + the `ZipkinTransport` seam.

**Production (modified):**
- `internal/tracing/decision.go` — split `Decide` → `DecideWithContext` + the thin wrapper.
- `internal/tracing/span.go` — `Authority` on `SpanInputs` + `Span`; `BuildServerSpan` sets it.
- `internal/tracing/config.go` — `ProviderKind` + `ZipkinSettings` + `zipkinTypeName`; the provider dispatch + the Zipkin parse + strict-reject arms.
- `internal/tracing/stats.go` — `ZipkinCounters` + `RegisterZipkinCounters`.
- `internal/tracing/exporter.go` — generalize `ExporterFor(*TracingConfig)` + the `ExporterProvider` Zipkin arm (dispatch on `Provider`; the `ZipkinTransport` field; the `HasCluster` boot-reject pre-check; lazy `RegisterZipkinCounters`); `NewExporterProvider` grows the `ZipkinTransport` param.
- `internal/filter/hcm/connection.go` — provider-aware H1 extract/inject + `in.Authority`.
- `internal/filter/hcm/h2dispatch.go` — provider-aware H2 extract/inject + `in.Authority`.
- `internal/filter/hcm/accesslog_emit.go` — set `in.Authority` in both `SpanInputs` builds.
- `internal/filter/hcm/config.go` — `provider.ExporterFor(tcfg)`.
- `cmd/envoy-go/main.go` — hoist `httpClient`; `zipkinTransportAdapter`; thread it into `NewExporterProvider`.

**Test (created):**
- `internal/tracing/b3_test.go`, `internal/tracing/b3_fuzz_test.go` (`FuzzExtractB3`), `internal/tracing/zipkin_test.go` (the encoder + identity + the `ZipkinExporter` bounded-channel/flush/retry/drop/Close against a fake `ZipkinTransport`).
- `internal/tracing/decision_test.go` (MODIFY: a `DecideWithContext` direct test + the `Decide`-wrapper-still-byte-stable test).
- `internal/tracing/config_test.go` (MODIFY: the Zipkin parse + strict-reject arms).
- `internal/tracing/stats_test.go` (MODIFY: +2 `tracing.zipkin.*` registration).
- `internal/tracing/exporter_test.go` (MODIFY: the `ExporterFor(*TracingConfig)` Zipkin-arm dispatch + the `HasCluster` boot-reject + the per-cluster memoize).
- `internal/filter/hcm/*_test.go` (MODIFY: the provider-aware extract/inject + the Authority carry + the no-tracing byte-stability guard).
- `test/helpers/zipkincollector/zipkincollector.go` (+ `_test.go`) — the driver-owned HTTP/JSON receiver.
- `test/fixtures/0088-tracing-zipkin/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`.
- `test/differential/runner_test.go` (MODIFY: blank-import the `0088` driver).

**Docs (completion task):**
- `docs/envoy-go/phases/46-tracing/PROGRESS-46.2.md`, `docs/envoy-go/DECISIONS.md` (ADR-0261 §Decision/§Consequences body — ANCHORS the leg), `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` (**row 46 → `done`**).

---

## Task 1: Phase scaffolding — PROGRESS-46.2.md + baselines

**Files:**
- Create: `docs/envoy-go/phases/46-tracing/PROGRESS-46.2.md`

- [ ] **Step 1: Record the baseline counts** (verbatim outputs in PROGRESS-46.2.md):
```bash
go build ./... && echo BUILD_OK
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                 # expect 89 (tail 0087-tracing-otlp)
grep -rh '^func Fuzz' --include='*.go' . | wc -l                  # expect 48
grep -n 'H2GoawayResponder' test/differential/fixture/fixture.go  # expect = 38 (BackendKind tail)
grep -rn 'ExtractB3\|InjectB3\|ZipkinExporter\|tracing.zipkin\|zipkinTypeName' internal/ --include='*.go'  # expect: NONE (46.2 introduces them)
```
Baseline: stat surface **1198** (H2 cluster; non-H2 **1194**) / fixtures **89** / fuzzers **48** / BackendKind **38** / DECISIONS tail **ADR-0260** (next-free **ADR-0261**).

- [ ] **Step 2: Write the PROGRESS-46.2.md scaffold** — a header (phase 46.2 IMPL, the SPEC-46.2 reference + the "FINAL chartered tracing leg; ANCHORS ADR-0261; flips row 46 done" note, the worktree branch), a task checklist mirroring this plan, the baseline block, the ADR-0045 NO-sub-split note (§3.0), and the anticipated exit counts: stat **1200** (+2 `tracing.zipkin.{spans_sent,spans_dropped}`) / fixtures **90** (`0088-tracing-zipkin`) / fuzzers **49** (`FuzzExtractB3`) / BackendKind **38** (driver-owned receiver) / DECISIONS **ADR-0261** / **0 new packages, 0 new go.mod modules**.

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/46-tracing/PROGRESS-46.2.md
git commit -m "phase 46.2 Task 1: PROGRESS scaffold + baselines (Zipkin exporter + B3; ANCHORS ADR-0261; flips row 46 done)"
```

---

## Task 2: The B3 codec — `ExtractB3` + `InjectB3` + `FuzzExtractB3` (`b3.go`) [TDD]

**Files:**
- Create: `internal/tracing/b3.go`, `internal/tracing/b3_test.go`, `internal/tracing/b3_fuzz_test.go`

The Zipkin analog of the W3C `traceparent` engine (`propagation.go`). Reuses the existing `TraceContext{TraceID, ParentID, Sampled, TraceState}` (the B3 path leaves `TraceState` empty). The `b3`/`X-B3-*` extraction is an UNTRUSTED-input boundary → `FuzzExtractB3`.

- [ ] **Step 1: Write the failing tests** in `b3_test.go` (table-driven):
  - **Single `b3` header** `<traceid>-<spanid>-<sampled>[-<parentid>]`:
    - 64-bit: `b3: "<16hex>-<16hex>-1"` ⇒ `(TraceContext{TraceID: low-8-bytes = the 16hex, ParentID: the spanid 16hex, Sampled: true}, true)`. (The B3 64-bit traceid occupies the LOW 8 bytes of the `[16]byte`; the high 8 stay zero.)
    - 128-bit: `b3: "<32hex>-<16hex>-1-<16hex>"` ⇒ `TraceID` = the full 32hex, `ParentID` = the 4th field (the incoming parent), `Sampled: true`.
    - `sampled=0` ⇒ `Sampled: false`; `sampled=d` (debug) ⇒ `Sampled: true` (debug forces sampled).
    - malformed (wrong hex, wrong field count, empty, all-zero traceid/spanid) ⇒ `(TraceContext{}, false)`.
  - **Multi-header `X-B3-*`** (when `b3` is absent): `X-B3-TraceId`/`X-B3-SpanId`/`X-B3-Sampled` present ⇒ extracted; `X-B3-ParentSpanId` optional → `ParentID`; `X-B3-Flags: 1` (debug) ⇒ `Sampled: true` even when `X-B3-Sampled` absent; missing required field ⇒ `(TraceContext{}, false)`.
  - **`b3` precedence**: when BOTH `b3` and `X-B3-*` are present, the single `b3` wins.
  - **`InjectB3(h, d, id128, shared)`** writes the MULTI-HEADER set (assert via canonical `h.Get("X-B3-TraceId")` etc. — NOT literal map keys; `http.Header.Set`/`Get` canonicalize, so reading back through `Get` stays self-consistent with however the reference cases the wire — `reference_wire_format_both_sides_see_same_bytes`):
    - fresh root (`d.ParentSpanID == zero`), `id128=false`, `sampled=true`: `X-B3-TraceId` = low-64 hex (16 chars), `X-B3-SpanId` = `hex(d.SpanID)` (16 chars), `X-B3-Sampled: 1`, NO `X-B3-ParentSpanId`.
    - continued (`d.ParentSpanID != zero`), `shared=false`: `X-B3-SpanId` = `hex(d.SpanID)` (fresh), `X-B3-ParentSpanId` = `hex(d.ParentSpanID)` (the incoming span-id).
    - continued, `shared=true`: `X-B3-SpanId` = `hex(d.ParentSpanID)` (the REUSED incoming span-id), NO `X-B3-ParentSpanId`.
    - `id128=true`: `X-B3-TraceId` is 32 hex (the full `d.TraceID`).
    - `sampled=false` ⇒ `X-B3-Sampled: 0`.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run 'TestB3' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `b3.go`:
  - `ExtractB3(h http.Header) (TraceContext, bool)`: if `h.Get("B3") != ""` parse the single header (split on `-`; 2/3/4 fields — 2 fields `traceid-spanid` is sampling-deferred but accept with `Sampled:false`; 16-or-32 hex traceid → fill the LOW 8 bytes for 16hex, all 16 for 32hex; the all-zero guards mirror `ExtractTraceparent:49`); else read `X-B3-TraceId`/`X-B3-SpanId`/`X-B3-ParentSpanId`/`X-B3-Sampled`/`X-B3-Flags`. Sampled = (`X-B3-Sampled == "1"`) OR (`b3` sampled field `1`/`d`) OR (`X-B3-Flags == "1"`). Return `(TraceContext{}, false)` on any framing/hex/all-zero failure.
  - `InjectB3(h http.Header, d Decision, id128, shared bool)`: derive `(traceID, spanID, parentID, _)` via `zipkinIdentity` (Task 4 — declare a thin local until Task 4 lands the shared helper; OR land `zipkinIdentity` here and have `zipkin.go` import it — LOCKED: land `zipkinIdentity` in `zipkin.go` at Task 4 and, to keep Task 2 self-contained, implement `InjectB3`'s identity inline here mirroring the same rules, then REPLACE with a `zipkinIdentity` call at Task 4 Step 3 — the Task-4 commit notes the dedup). Write `X-B3-TraceId`/`X-B3-SpanId`/`X-B3-Sampled` always; `X-B3-ParentSpanId` only when `parentID != ""`. Hex is lowercase (`hex.EncodeToString`).
  - `traceIDHex(t [16]byte, id128 bool) string`: `id128` ⇒ `hex(t[:])` (32 chars); else `hex(t[8:])` (16 chars — the low 64). `spanIDHex(s [8]byte) string`: `hex(s[:])`.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run 'TestB3' -count=1` ⇒ PASS.

- [ ] **Step 5: Write `FuzzExtractB3`** in `b3_fuzz_test.go` (the `FuzzExtractTraceparent` shape): seed a handful of valid + malformed `b3`/`X-B3-*` header sets; the fuzz body builds an `http.Header` from the corpus bytes, calls `ExtractB3`, and asserts NO panic + (when `ok`) the returned `TraceID`/`ParentID` are non-zero (the extract contract). Run `go test ./internal/tracing/ -run 'FuzzExtractB3' -count=1` (the seed-corpus pass) ⇒ PASS; then a short `-fuzz`: `go test ./internal/tracing/ -run '^$' -fuzz 'FuzzExtractB3' -fuzztime 15s` ⇒ no crash.

- [ ] **Step 6: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/b3.go internal/tracing/b3_test.go internal/tracing/b3_fuzz_test.go
git commit -m "phase 46.2 Task 2: B3 codec — ExtractB3 (single b3 / multi X-B3-*) + InjectB3 (multi-header) + FuzzExtractB3 (fuzzers 48→49; D-TRACE-ZIPKIN-B3/FUZZER)"
```

---

## Task 3: The `Decide`-takes-extracted-context refactor (`decision.go`) [TDD]

**Files:**
- Modify: `internal/tracing/decision.go`, `internal/tracing/decision_test.go`

Split `Decide` so a B3 caller can supply an already-extracted context (D-TRACE-ZIPKIN-DECIDE-SEAM). The rng-call ORDER is preserved verbatim — OTel/`0086`/`0087` stay byte-stable.

- [ ] **Step 1: Write the failing test** in `decision_test.go`:
  - `DecideWithContext(h, TraceContext{}, false, cfg, rng)` with a fresh-trace path ⇒ identical `Decision` to the existing `Decide(h, cfg, rng)` for the SAME inputs (a no-traceparent header) — assert field-by-field equality (drive both with a deterministic `RandSource` seeded identically; they must produce the SAME `SpanID`/`TraceID`/`Sample`/`RequestID`).
  - `DecideWithContext(h, ic, true, cfg, rng)` with a non-zero `ic` (TraceID + ParentID + Sampled) ⇒ `Continued:true`, `TraceID==ic.TraceID`, `ParentSpanID==ic.ParentID`, `Sample==ic.Sampled`, `Class==NotTraceable` (the continued-authoritative path).
  - The existing `Decide` tests STILL PASS (the wrapper preserves behavior).

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/tracing/ -run 'TestDecide' -count=1` ⇒ FAIL (`DecideWithContext` undefined).

- [ ] **Step 3: Implement** in `decision.go`: rename the current `Decide` BODY to `DecideWithContext(h http.Header, ic TraceContext, continued bool, cfg *TracingConfig, rng RandSource) Decision`, replacing the `if ic, ok := ExtractTraceparent(h); ok {` line with `if continued {` (using the passed `ic`). Keep the leading `_, _ = rng.Read(d.SpanID[:])` FIRST (byte-stability). Add the thin wrapper:
```go
func Decide(h http.Header, cfg *TracingConfig, rng RandSource) Decision {
    ic, ok := ExtractTraceparent(h)
    return DecideWithContext(h, ic, ok, cfg, rng)
}
```

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run 'TestDecide' -count=1` ⇒ PASS. Then the FULL tracing package: `go test ./internal/tracing/ -count=1` ⇒ PASS (no behavior drift).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/decision.go internal/tracing/decision_test.go
git commit -m "phase 46.2 Task 3: DecideWithContext — lift trace-context extraction to the caller; Decide stays a thin traceparent wrapper (byte-stable; D-TRACE-ZIPKIN-DECIDE-SEAM)"
```

---

## Task 4: The Zipkin v2 JSON span encoder + the `Authority` carry (`zipkin.go` + `span.go`) [TDD]

**Files:**
- Modify: `internal/tracing/span.go`, `internal/tracing/span_test.go`
- Create: `internal/tracing/zipkin.go`, `internal/tracing/zipkin_test.go`

The Zipkin analog of `(*Span).toProto` — maps the internal `Span` to the v2 JSON model (§11 D-TRACE-ZIPKIN-WIRE/IDS/SHARED). Adds the provider-neutral `Authority` field (D-TRACE-ZIPKIN-SPAN-NAME).

- [ ] **Step 1: Write the failing tests**:
  - In `span_test.go` (MODIFY): a `SpanInputs{Authority:"127.0.0.1:10000", …}` ⇒ `BuildServerSpan(...).Authority == "127.0.0.1:10000"` and `Name == "ingress"` (BOTH carried); `toProto().Name == "ingress"` (OTLP unchanged).
  - In `zipkin_test.go` (NEW):
    - `zipkinIdentity(d, id128=false, shared=false)` for a fresh root (`ParentSpanID==zero`) ⇒ `traceID` 16-hex (low 64 of `d.TraceID`), `id == hex(d.SpanID)`, `parentID == ""`, `emitShared == false`.
    - continued (`ParentSpanID != zero`), `shared=false` ⇒ `id == hex(d.SpanID)` (fresh), `parentID == hex(d.ParentSpanID)`, `emitShared == false`.
    - continued, `shared=true` ⇒ `id == hex(d.ParentSpanID)` (REUSED), `parentID == ""`, `emitShared == true`.
    - `id128=true` ⇒ `traceID` 32-hex (full).
    - `encodeZipkinSpans([]*Span{span}, id128=false, shared=true)` ⇒ a JSON ARRAY (`[{...}]`); decode it back and assert: `name == span.Authority`, `kind == "SERVER"`, `timestamp == span.Start.UnixMicro()`, `duration == span.End.Sub(span.Start).Microseconds()` (and `> 0` for a non-degenerate span), `shared == true`, `traceId`/`id` per `zipkinIdentity`, and `tags` is a STRING map of the 14 keys = the 16-attr roster MINUS `node_id`/`zone` (assert `node_id`/`zone` ABSENT; assert `http.method`/`http.url`/`http.protocol`/`http.status_code`/`component`/`downstream_cluster`/`response_flags`/`request_size`/`response_size`/`user_agent`/`guid:x-request-id` PRESENT; `upstream_cluster`/`upstream_cluster.name`/`peer.address` PRESENT-as-keys). With a `ClientTraceID` set ⇒ `guid:x-client-trace-id` tag present. NO `annotations` field (D-TRACE-ZIPKIN-ANNOTATIONS).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run 'TestSpan|TestZipkin' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement**:
  - `span.go`: add `Authority string` to `SpanInputs` AND `Span`; in `BuildServerSpan` set `Authority: in.Authority` on the returned `Span` (the `attrs` roster is UNCHANGED — `Authority` is a separate field, NOT a tag).
  - `zipkin.go`:
    - `type zipkinEndpoint struct { ServiceName string \`json:"serviceName,omitempty\"`; … }` (minimal; `localEndpoint` is omitted entirely when serviceName empty — env-specific, UNasserted).
    - `type zipkinSpan struct { TraceID string \`json:"traceId\"`; ID string \`json:"id\"`; ParentID string \`json:"parentId,omitempty\"`; Name string \`json:"name\"`; Kind string \`json:"kind\"`; Timestamp int64 \`json:"timestamp\"`; Duration int64 \`json:"duration\"`; Shared bool \`json:"shared,omitempty\"`; LocalEndpoint *zipkinEndpoint \`json:"localEndpoint,omitempty\"`; Tags map[string]string \`json:"tags\"` }`.
    - `zipkinIdentity(d identityInput, id128, shared bool) (traceID, id, parentID string, emitShared bool)` where `identityInput` is `{TraceID [16]byte; SpanID, ParentSpanID [8]byte}` (so BOTH a `Decision` and a `Span` can supply it — pass the three fields). Logic per the test table. (Replace Task 2's inline `InjectB3` identity with a call to this — update `b3.go`.)
    - `func encodeZipkinSpans(spans []*Span, id128, shared bool) ([]byte, error)`: map each `*Span` → `zipkinSpan` (`Name = s.Authority`, `Kind = "SERVER"`, `Timestamp = s.Start.UnixMicro()`, `Duration = s.End.Sub(s.Start).Microseconds()`, ids via `zipkinIdentity`, `Tags` = the Attrs filtered to drop `node_id`/`zone`, all values `KV.Str`), then `json.Marshal` the slice.
  - Update `b3.go` `InjectB3` to call `zipkinIdentity` (dedup the inline identity).

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run 'TestSpan|TestZipkin|TestB3' -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/span.go internal/tracing/span_test.go internal/tracing/zipkin.go internal/tracing/zipkin_test.go internal/tracing/b3.go
git commit -m "phase 46.2 Task 4: Zipkin v2 JSON span encoder (name=authority, 14 tags, microsecond timing, shared/parentId) + zipkinIdentity + Span.Authority (D-TRACE-ZIPKIN-WIRE/IDS/SHARED/SPAN-NAME)"
```

---

## Task 5: The provider dispatch + the Zipkin `NewConfig` parse arm + strict-rejects (`config.go`) [TDD]

**Files:**
- Modify: `internal/tracing/config.go`, `internal/tracing/config_test.go`

Lift the provider gate from "OTel only" to "OTel OR Zipkin" (D-TRACE-ZIPKIN-CONFIG-SHAPE); add the Zipkin-specific strict-reject arms (ADR-0080).

- [ ] **Step 1: Write the failing tests** in `config_test.go` (build an HCM `tracing` message with a `ZipkinConfig` typed_config via `anypb.New`):
  - a minimal Zipkin config (`collector_cluster:"zk"`, `collector_endpoint:"/api/v2/spans"`, `collector_endpoint_version:HTTP_JSON`) ⇒ `(*TracingConfig{Provider:ProviderZipkin, ClusterName:"zk", Zipkin:&ZipkinSettings{CollectorEndpoint:"/api/v2/spans", SharedSpanContext:true}}, nil)` (note `SharedSpanContext` defaults TRUE when the `*BoolValue` is absent).
  - `trace_id_128bit:true` ⇒ `Zipkin.TraceID128Bit == true`; `shared_span_context:{value:false}` ⇒ `Zipkin.SharedSpanContext == false`; `collector_hostname:"h"` ⇒ `Zipkin.CollectorHostname == "h"`.
  - **strict-rejects** (each its own arm per `reference_strict_reject_sibling_typeurl_gap`): `collector_endpoint_version:HTTP_PROTO` ⇒ error; `:GRPC` ⇒ error; `:DEPRECATED…(0)` ⇒ error; `split_spans_for_request:true` ⇒ error; empty `collector_cluster` ⇒ error.
  - the OTel arm STILL parses unchanged (`Provider == ProviderOTel`, `Zipkin == nil`); the existing OTel tests pass.
  - the SHARED HCM-level rejects (`verbose`/`max_path_tag_length`/`custom_tags`/`spawn_upstream_span`) still fire (they precede the provider dispatch).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run 'TestNewConfig' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** in `config.go`:
  - Add `var zipkinTypeName = (&tracev3.ZipkinConfig{}).ProtoReflect().Descriptor().FullName()` (sibling of `otelTypeName`).
  - Add `ProviderKind` + `ZipkinSettings` (per D-TRACE-ZIPKIN-CONFIG-SHAPE) and grow `TracingConfig` with `Provider` + `Zipkin`.
  - In `NewConfig`, AFTER the shared HCM-level rejects + `tc := p.GetTypedConfig()`, dispatch on `tc.MessageName()`: `otelTypeName` → the EXISTING OTel arm (set `Provider:ProviderOTel`); `zipkinTypeName` → a NEW `parseZipkin(tc)` arm; else the existing "unsupported provider" reject (now naming both supported types).
  - `parseZipkin`: `proto.Unmarshal` into `tracev3.ZipkinConfig`; reject `GetCollectorEndpointVersion() != HTTP_JSON` (three arms: PROTO/GRPC/0 — or a single `!= HTTP_JSON` with the offending value in the message; LOCKED: a single guard `if v := z.GetCollectorEndpointVersion(); v != tracev3.ZipkinConfig_HTTP_JSON { return nil, fmt.Errorf("tracing: zipkin collector_endpoint_version %v unsupported (only HTTP_JSON)", v) }` — the per-value text differs by `v`, satisfying the sibling-gap guard without three literal arms); reject `GetSplitSpansForRequest()`; reject empty `GetCollectorCluster()`. Build `TracingConfig{ClientSampling/RandomSampling/OverallSampling: the shared pct(...), ClusterName: z.GetCollectorCluster(), Provider: ProviderZipkin, Zipkin: &ZipkinSettings{CollectorEndpoint: z.GetCollectorEndpoint(), CollectorHostname: z.GetCollectorHostname(), TraceID128Bit: z.GetTraceId_128Bit(), SharedSpanContext: <shared>}}` where `<shared>` defaults TRUE when absent via a nil-guard that needs NO new import (do NOT name `wrapperspb`): `shared := true; if sv := z.GetSharedSpanContext(); sv != nil { shared = sv.GetValue() }`. (Factor the three `pct(...)` sampling reads so both arms share them.)

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run 'TestNewConfig' -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./... && go mod tidy -diff
git add internal/tracing/config.go internal/tracing/config_test.go
git commit -m "phase 46.2 Task 5: NewConfig provider dispatch (OTel|Zipkin) + the ZipkinConfig parse arm + strict-rejects (collector_endpoint_version!=HTTP_JSON, split_spans_for_request, empty collector_cluster); 0 new go.mod modules (D-TRACE-ZIPKIN-CONFIG-SHAPE)"
```

---

## Task 6: The Zipkin tracer-scoped counters (`tracing.zipkin.{spans_sent,spans_dropped}`) [TDD, +2 → 1200]

**Files:**
- Modify: `internal/tracing/stats.go`, `internal/tracing/stats_test.go`

Mirror `RegisterTracerCounters` exactly (D-TRACE-ZIPKIN-STATS-FINAL).

- [ ] **Step 1: Write the failing test** — `RegisterZipkinCounters(reg)` returns a non-nil `*ZipkinCounters`; the registry gains EXACTLY 2 counters named `tracing.zipkin.spans_sent` + `tracing.zipkin.spans_dropped` (assert the count DELTA == 2). `IncSent(3)` adds 3; `IncDropped()` increments. **Test-isolation caution:** each sub-test that registers uses a FRESH `stats.NewRegistry()` (`NewCounter` PANICS on a duplicate static name).

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/tracing/ -run 'TestZipkinCounters' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** in `stats.go` (the `RegisterTracerCounters` shape verbatim, `opentelemetry`→`zipkin`):
```go
type ZipkinCounters struct{ spansSent, spansDropped *stats.Counter }
func RegisterZipkinCounters(reg *stats.Registry) *ZipkinCounters {
    return &ZipkinCounters{
        spansSent:    reg.NewCounter("tracing.zipkin.spans_sent"),
        spansDropped: reg.NewCounter("tracing.zipkin.spans_dropped"),
    }
}
func (c *ZipkinCounters) IncSent(n int) { if c != nil { c.spansSent.Add(uint64(n)) } }
func (c *ZipkinCounters) IncDropped()   { if c != nil { c.spansDropped.Inc() } }
```

- [ ] **Step 4: Run to verify it passes** — `go test ./internal/tracing/ -run 'TestZipkinCounters' -count=1` ⇒ PASS. Record the +2 surface delta (1198 → 1200) in PROGRESS-46.2.md.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go build ./...
git add internal/tracing/stats.go internal/tracing/stats_test.go
git commit -m "phase 46.2 Task 6: RegisterZipkinCounters — 2 tracing.zipkin.{spans_sent,spans_dropped} (+2 → 1200; D-TRACE-ZIPKIN-STATS-FINAL; reports_*/timer_flushed dropped)"
```

---

## Task 7: The `ZipkinExporter` + the `ZipkinTransport` seam (`zipkin.go`) [TDD, full-package -race]

**Files:**
- Modify: `internal/tracing/zipkin.go`, `internal/tracing/zipkin_test.go`

A SECOND `Exporter` impl behind the existing seam, mirroring `OTLPExporter`'s bounded-channel + writer-goroutine + size/interval/close-buffer + retry-once + drop-newest + idempotent `Close` shape — but flushing a v2 JSON POST over `ZipkinTransport` instead of a gRPC `Export`. This task introduces a BACKGROUND GOROUTINE → the `-race` gate runs the FULL package.

- [ ] **Step 1: Write the failing tests** in `zipkin_test.go` (against a `fakeZipkinTransport` recording POSTed bodies + a programmable error/status + `HasCluster` toggle):
  - `Export(span)` ×K then `Close()` ⇒ the fake received K spans aggregated across POSTs (decode each POST body's JSON array + sum — assert the AGGREGATE, not the POST count); `spans_sent == K`.
  - size-trigger (tiny `bufferSizeBytes`): ≥2 POSTs for K large spans — assert the AGGREGATE.
  - interval-trigger (short flush interval + 1 span) ⇒ the span flushes on the tick (poll the fake to converge — no `Sleep`-assert).
  - retry-once: the fake returns a non-2xx (or an error) on attempt 1, 2xx on attempt 2 ⇒ the batch sends, `spans_sent += len(batch)`; non-2xx TWICE ⇒ dropped (logged-not-counted), `spans_sent` unchanged, buffer reset (memory bounded).
  - drop-newest: fill the channel past capacity (a blocked fake) ⇒ overflow increments `spans_dropped`.
  - `Close()` idempotent (`sync.Once`); drains the pending buffer before returning.
  - the POST request shape: `Method == "POST"`, `URL.Path == CollectorEndpoint`, `Host == CollectorHostname` (or the cluster name when empty), `Content-Type: application/json`.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run 'TestZipkinExporter' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** in `zipkin.go` (the `exporter.go` `OTLPExporter` structure verbatim, swapping the flush):
  - `type ZipkinTransport interface { HasCluster(name string) bool; Dispatch(ctx context.Context, clusterName string, req *http.Request) (*http.Response, error) }`.
  - `type ZipkinExporter struct { ch chan *Span; transport ZipkinTransport; clusterName, endpoint, hostname string; id128, shared bool; spansSent, spansDropped *stats.Counter; done chan struct{}; closeOnce sync.Once; closeErr error; lastDropLog atomic.Int64; bufferSizeBytes int; bufferFlushInterval time.Duration; ctx context.Context; cancel context.CancelFunc }`.
  - `NewZipkinExporter(transport, clusterName, endpoint, hostname string, id128, shared bool, spansSent, spansDropped *stats.Counter, bufferSizeBytes int, bufferFlushInterval time.Duration) *ZipkinExporter` (+ a `…WithCapacity` test variant) → `go e.run()`.
  - `Export(span)`: the `OTLPExporter.Export` drop-newest idiom verbatim.
  - `run()`: the `OTLPExporter.run` loop — buffer `*Span` (size-trigger uses `len(encoded)` measured at flush, OR a rough per-span byte estimate; LOCKED: buffer `*Span` and on the size-check encode incrementally is wasteful — instead accumulate a running `bufBytes` via a cheap per-span estimate `len(s.Authority)+len(s.Attrs)*32` for the trigger, and encode the whole batch ONCE at `flush`). At `flush`: `body, err := encodeZipkinSpans(buf, e.id128, e.shared)`; build `req, _ := http.NewRequestWithContext(e.ctx, http.MethodPost, "http://"+host+e.endpoint, bytes.NewReader(body))` (the `ClusterDispatch` rewrites `URL.Host` to the picked endpoint; the placeholder host carries the `Host` header — set `req.Host = e.hostname` when non-empty, else `e.clusterName`); `req.Header.Set("Content-Type","application/json")`; for `attempt` in 0..1: `resp, err := e.transport.Dispatch(e.ctx, e.clusterName, req)`; success = `err == nil && resp != nil && resp.StatusCode/100 == 2` → drain+close `resp.Body`, `spansSent.Add(len(buf))`, break; else log + (drain/close any resp.Body) + retry. Reset `buf`/`bufBytes` whether sent or dropped.
  - `Close()`: the `OTLPExporter.Close` `sync.Once` shutdown verbatim (no `client.Close()` — the `ZipkinTransport` is shared/stateless; return nil).

- [ ] **Step 4: Run to verify they pass + the FULL-package -race** — `go test ./internal/tracing/ -run 'TestZipkinExporter' -count=1` ⇒ PASS; then `go test ./internal/tracing/ -race -count=1` ⇒ PASS (the FULL package — the writer goroutine is a background mutator; `reference_full_suite_race_after_background_mutator`).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ && golangci-lint run ./internal/tracing/... && go vet ./internal/tracing/... && go build ./...
git add internal/tracing/zipkin.go internal/tracing/zipkin_test.go
git commit -m "phase 46.2 Task 7: ZipkinExporter — bounded-channel + writer-goroutine + v2-JSON POST over ZipkinTransport (httpclient.ClusterDispatch) + retry-once + drop-newest + idempotent Close (D-TRACE-ZIPKIN-WIRE/TRANSPORT-WIRING)"
```

---

## Task 8: The `ExporterProvider` Zipkin arm + the boot-reject gate (`exporter.go`) [TDD]

**Files:**
- Modify: `internal/tracing/exporter.go`, `internal/tracing/exporter_test.go`

Generalize `ExporterFor` to take `*TracingConfig` and dispatch on `Provider`; the Zipkin arm builds a `ZipkinExporter` over the injected `ZipkinTransport`, pre-checking the collector cluster exists (the boot-reject gate, REFERENCE-PARITY — AMEND-ZIPKIN-BOOT-REJECT).

- [ ] **Step 1: Write the failing tests** in `exporter_test.go` (against a `fakeZipkinTransport` with a `HasCluster` toggle + the existing `fakeDialer` for OTel):
  - `ExporterFor(&TracingConfig{Provider:ProviderZipkin, ClusterName:"zk", Zipkin:&ZipkinSettings{...}})` on a provider whose transport `HasCluster("zk")==true` ⇒ a non-nil `Exporter`; a SECOND call returns the SAME pointer (memoized per cluster).
  - `ExporterFor(zipkinCfg)` with `HasCluster("zk")==false` ⇒ `(nil, error)` (the boot-reject — `unknown cluster`).
  - the `tracing.zipkin.*` counters register LAZILY on the FIRST successful Zipkin build (a fresh registry gains +2 only after the first Zipkin `ExporterFor`; a provider that never builds a Zipkin exporter leaves the surface unmoved).
  - the OTel arm STILL works (`ExporterFor(&TracingConfig{Provider:ProviderOTel, ClusterName:"c", ServiceName:"svc"})` ⇒ the OTLP exporter; the `tracing.opentelemetry.*` lazy register unchanged).
  - `CloseAll()` closes every built exporter (mixed OTel + Zipkin); idempotent.
  - **Test-isolation:** each sub-test uses a FRESH `stats.NewRegistry()`.

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/tracing/ -run 'TestExporterProvider' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** in `exporter.go`:
  - `NewExporterProvider` grows a `ZipkinTransport` param (nil-able) + a separate `sync.Once`/`*ZipkinCounters` for the Zipkin counters: `func NewExporterProvider(d tracesClientDialer, zt ZipkinTransport, reg *stats.Registry, bufBytes int, bufFlush time.Duration) *ExporterProvider`. (Store `zt`, `zipkinOnce`, `zipkinCounters`.)
  - Change `ExporterFor(clusterName, serviceName string)` → `ExporterFor(cfg *TracingConfig) (Exporter, error)`; the memoize key stays `cfg.ClusterName`. Dispatch:
    - `ProviderOTel`: the EXISTING OTLP build (`p.dialer.NewTracesClient(cfg.ClusterName)` + lazy `RegisterTracerCounters` + `NewOTLPExporter(..., cfg.ServiceName, ...)`).
    - `ProviderZipkin`: `if p.zt == nil { return nil, fmt.Errorf("tracing: zipkin provider but no transport wired") }`; `if !p.zt.HasCluster(cfg.ClusterName) { return nil, fmt.Errorf("tracing: zipkin: unknown cluster %q", cfg.ClusterName) }` (the boot-reject — reference-parity); `p.zipkinOnce.Do(func(){ p.zipkinCounters = RegisterZipkinCounters(p.reg) })`; `e := NewZipkinExporter(p.zt, cfg.ClusterName, cfg.Zipkin.CollectorEndpoint, cfg.Zipkin.CollectorHostname, cfg.Zipkin.TraceID128Bit, cfg.Zipkin.SharedSpanContext, p.zipkinCounters.spansSent, p.zipkinCounters.spansDropped, p.bufBytes, p.bufFlush)`.
  - Store both `*OTLPExporter` and `*ZipkinExporter` in `byCluster map[string]Exporter` (widen the map value to the interface; `CloseAll` already iterates and calls `Close()`).
  - **Migrate the two call sites in the SAME commit so the whole repo builds** (the `NewExporterProvider` + `ExporterFor` signatures are breaking — without these the per-task `go build ./...` gate fails until Task 10): (1) `internal/filter/hcm/config.go:334` → `exporter, err = provider.ExporterFor(tcfg)` (the whole config; this MOVES the `:334` edit OUT of Task 9 — Task 9 no longer touches `config.go`); (2) `cmd/envoy-go/main.go:131` → `tracing.NewExporterProvider(tracesDialerAdapter{dialer}, nil, bs.Stats, 16384, time.Second)` (a `nil` `ZipkinTransport` placeholder — only the Zipkin arm consults it, and there are NO Zipkin configs until `0088`, so a nil transport builds + boots cleanly; Task 10 swaps `nil` → the real `zipkinTransportAdapter`).

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/tracing/ -run 'TestExporterProvider' -count=1` ⇒ PASS; then `go test ./internal/tracing/ -race -count=1` ⇒ PASS (full package); then `go build ./...` ⇒ OK (the two migrated call sites keep the WHOLE repo green).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/tracing/ internal/filter/hcm/ cmd/envoy-go/ && golangci-lint run ./internal/tracing/... && go vet ./... && go build ./...
git add internal/tracing/exporter.go internal/tracing/exporter_test.go internal/filter/hcm/config.go cmd/envoy-go/main.go
git commit -m "phase 46.2 Task 8: ExporterProvider Zipkin arm — ExporterFor(*TracingConfig) provider dispatch + memoize + lazy tracing.zipkin register + HasCluster boot-reject gate; migrate both call sites (config.go ExporterFor(tcfg) + main.go nil-transport placeholder) so the repo builds (reference-parity; D-TRACE-ZIPKIN-TRANSPORT-WIRING)"
```

---

## Task 9: HCM provider-aware extract/inject + the `Authority` carry + byte-stability (`connection.go`/`h2dispatch.go`/`accesslog_emit.go`/`config.go`) [TDD]

**Files:**
- Modify: `internal/filter/hcm/connection.go`, `internal/filter/hcm/h2dispatch.go`, `internal/filter/hcm/accesslog_emit.go`
- Test: `internal/filter/hcm/*_test.go`

Make the dispatch seam dispatch B3-vs-traceparent by `ProviderKind`; carry the request authority into the span; keep the no-tracing path byte-stable. (NOTE: the `config.go:334` `ExporterFor(tcfg)` migration already landed in Task 8 — Task 9 does NOT touch `config.go`.)

- [ ] **Step 1: Write the failing tests** (extend the existing HCM tracing tests):
  - **H1 Zipkin path:** an HCM `Filter` with a `tracingConfig{Provider:ProviderZipkin, Zipkin:{...}}` + a stub exporter recording spans ⇒ for a request with NO incoming B3, the dispatched upstream request carries `X-B3-TraceId`/`X-B3-SpanId`/`X-B3-Sampled` (NOT `traceparent`); the recorded span's `Authority == r.Host`.
  - **H1 Zipkin continuation:** an incoming `b3: "<trace>-<span>-1"` ⇒ the recorded span's `TraceID` == the incoming trace (continued); the upstream `X-B3-TraceId` == the incoming trace.
  - **H1 OTel path unchanged:** a `Provider:ProviderOTel` config still injects `traceparent` (regression).
  - **H2 Zipkin path:** the analogous `x-b3-*` write-back via `upsertH2Header`; `Authority == req.Authority`.
  - **byte-stability:** no `tracingConfig` ⇒ NO `x-b3-*`/`traceparent`/`x-request-id` added (the existing guard).

- [ ] **Step 2: Run to verify they fail** — `go test ./internal/filter/hcm/ -run 'TestTracing|TestSpan|TestB3' -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement**:
  - `connection.go:520` block → provider-aware:
    ```go
    if f.tracingConfig != nil {
        var ic tracing.TraceContext
        var ok bool
        if f.tracingConfig.Provider == tracing.ProviderZipkin {
            ic, ok = tracing.ExtractB3(req.Header)
        } else {
            ic, ok = tracing.ExtractTraceparent(req.Header)
        }
        d := tracing.DecideWithContext(req.Header, ic, ok, f.tracingConfig, f.rng)
        req.Header.Set("X-Request-Id", d.RequestID)
        if f.tracingConfig.Provider == tracing.ProviderZipkin {
            tracing.InjectB3(req.Header, d, f.tracingConfig.Zipkin.TraceID128Bit, f.tracingConfig.Zipkin.SharedSpanContext)
        } else {
            tracing.InjectTraceparent(req.Header, d.TraceID, d.SpanID, d.Sample, d.TraceState)
        }
        f.tracingCounters.Record(d.Class)
        traceDecision = &d
    }
    ```
  - `h2dispatch.go:421` block → the same dispatch over the `http.Header` view; for Zipkin write the `x-b3-*` headers back via `upsertH2Header` (iterate the view's `X-B3-*` keys, lowercased) instead of `traceparent`/`tracestate`.
  - `accesslog_emit.go`: set `Authority: r.Host` (H1, `:41` `SpanInputs`) and `Authority: req.Authority` (H2, `:101` `SpanInputs`).
  - (`config.go:334` `ExporterFor(tcfg)` already migrated in Task 8 — not touched here.)

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/filter/hcm/ -run 'TestTracing|TestSpan|TestB3' -count=1` ⇒ PASS; then `go test ./internal/filter/hcm/ -count=1` ⇒ PASS (no regression); then `go test ./internal/filter/hcm/ -race -count=1` ⇒ PASS (full package — the exporter goroutine is reachable through the filter).

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l internal/filter/hcm/ && golangci-lint run ./internal/filter/hcm/... && go vet ./internal/filter/hcm/... && go build ./...
git add internal/filter/hcm/connection.go internal/filter/hcm/h2dispatch.go internal/filter/hcm/accesslog_emit.go internal/filter/hcm/
git commit -m "phase 46.2 Task 9: HCM provider-aware extract/inject (B3 vs traceparent by ProviderKind) + Span.Authority carry; no-tracing byte-stable (D-TRACE-ZIPKIN-DECIDE-SEAM)"
```

---

## Task 10: Boot wiring — hoist `httpClient`; thread the `ZipkinTransport` into `NewExporterProvider` (`main.go`) [TDD via build + a smoke boot]

**Files:**
- Modify: `cmd/envoy-go/main.go`

Thread the HTTP transport so the Zipkin arm can dispatch. `CloseAll`/the defer-LIFO are UNCHANGED.

- [ ] **Step 1: Write the failing check** — there is no unit test for `main.go`; the gate is `go build ./...` + a smoke boot. First, ADD a `zipkinTransportAdapter` test in a `cmd/envoy-go` `_test.go` (or assert structurally) that `zipkinTransportAdapter{httpClient, cm}` satisfies `tracing.ZipkinTransport` (a compile-time `var _ tracing.ZipkinTransport = zipkinTransportAdapter{}`). Run `go build ./...` ⇒ FAIL (adapter undefined / `NewExporterProvider` arity mismatch).

- [ ] **Step 2: Implement** in `main.go`:
  - HOIST `httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})` from `:263` to immediately BEFORE the `tracingProvider := …` line at `:131` (it is stateless; the later `:263` reference becomes the hoisted var — remove the duplicate). Confirm no other consumer between `:131` and `:263` needs it earlier (only the listener-manager at `:282` consumes it — fine).
  - Add the adapter (near `tracesDialerAdapter:382`):
    ```go
    type zipkinTransportAdapter struct {
        c  *httpclient.Client
        cm *cluster.Manager
    }
    func (a zipkinTransportAdapter) HasCluster(name string) bool { _, ok := a.cm.Get(name); return ok }
    func (a zipkinTransportAdapter) Dispatch(ctx context.Context, clusterName string, req *http.Request) (*http.Response, error) {
        return a.c.ClusterDispatch(ctx, clusterName, req, a.cm)
    }
    var _ tracing.ZipkinTransport = zipkinTransportAdapter{}
    ```
  - SWAP the `nil` placeholder (landed in Task 8) for the real adapter: `tracingProvider := tracing.NewExporterProvider(tracesDialerAdapter{dialer}, zipkinTransportAdapter{httpClient, cm}, bs.Stats, 16384, time.Second)`.
  - The `defer func() { _ = tracingProvider.CloseAll() }()` (`:168`) is UNCHANGED (the `ZipkinExporter.Close()` joins the same path).

- [ ] **Step 3: Run to verify it builds + boots** — `go build ./... && go vet ./...` ⇒ OK. A smoke boot against a tiny Zipkin-configured bootstrap (reuse the `0088` `envoy-go.yaml` once Task 12 lands; for now assert `go build`). Confirm `go mod tidy -diff` EMPTY.

- [ ] **Step 4: Per-task gates + commit**
```bash
gofmt -l cmd/envoy-go/ && golangci-lint run ./cmd/... && go vet ./cmd/... && go build ./...
git add cmd/envoy-go/main.go cmd/envoy-go/
git commit -m "phase 46.2 Task 10: boot wiring — hoist httpClient + thread zipkinTransportAdapter into NewExporterProvider; CloseAll unchanged (D-TRACE-ZIPKIN-TRANSPORT-WIRING)"
```

---

## Task 11: The driver-owned HTTP Zipkin collector (`test/helpers/zipkincollector`)

**Files:**
- Create: `test/helpers/zipkincollector/zipkincollector.go`, `test/helpers/zipkincollector/zipkincollector_test.go`

The `test/helpers/otlptrace` analog — an in-process `net/http` server accumulating every POSTed v2 JSON span (D-TRACE-ZIPKIN-RECEIVER-WIRING). NOT a runner `BackendKind` (`reference_differential_grpc_receiver_driver_owned` — BackendKind stays 38).

- [ ] **Step 1: Write the failing tests** in `zipkincollector_test.go`: `New()` starts a server; POST a JSON array of 2 spans to `<Addr()>/api/v2/spans` ⇒ `Count() == 2` and `Spans()` returns both decoded (assert `name`/`traceId`/`tags`); a second POST accumulates (`Count() == 4`); `Reset()` clears; `Stop()` shuts down.

- [ ] **Step 2: Run to verify they fail** — `go test ./test/helpers/zipkincollector/ -count=1` ⇒ FAIL.

- [ ] **Step 3: Implement** `zipkincollector.go`: a `Collector` struct `{ srv *http.Server; ln net.Listener; mu sync.Mutex; spans []ReceivedSpan }`; `New() (*Collector, error)` listens on `127.0.0.1:0`, serves a handler that `io.ReadAll`s the body (the stdlib de-chunks transparently), `json.Unmarshal`s into `[]ReceivedSpan` (a struct mirroring `zipkinSpan`'s public fields — `TraceID`/`ID`/`ParentID`/`Name`/`Kind`/`Timestamp`/`Duration`/`Shared`/`Tags`), appends under the mutex, replies `202 Accepted` (the reference collector contract). `Spans()`/`Count()`/`Reset()`/`Addr()`/`Stop()` mirror `test/helpers/otlptrace`.

- [ ] **Step 4: Run to verify they pass** — `go test ./test/helpers/zipkincollector/ -count=1` ⇒ PASS.

- [ ] **Step 5: Per-task gates + commit**
```bash
gofmt -l test/helpers/zipkincollector/ && golangci-lint run ./test/helpers/zipkincollector/... && go vet ./test/helpers/zipkincollector/... && go build ./...
git add test/helpers/zipkincollector/
git commit -m "phase 46.2 Task 11: driver-owned HTTP Zipkin collector (test/helpers/zipkincollector — de-chunk + JSON-decode + accumulate; BackendKind stays 38; D-TRACE-ZIPKIN-RECEIVER-WIRING)"
```

---

## Task 12: The `0088-tracing-zipkin` cross-side EXACT differential + subject unit tests

**Files:**
- Create: `test/fixtures/0088-tracing-zipkin/{driver/driver.go, envoy.yaml, envoy-go.yaml, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the `0088` driver)

COPY `test/fixtures/0087-tracing-otlp/` and adapt: the collector cluster is plaintext HTTP/1 (no h2c); the receiver is the HTTP Zipkin collector; the provider is Zipkin (`random_sampling=100%`, `collector_endpoint_version=HTTP_JSON`, fixed `Host`, `collector_endpoint=/api/v2/spans`, a chosen `trace_id_128bit`/`shared_span_context`). Both sides POST to the SAME driver-owned collector over a shared bridge (`reference_docker_probe_bridge_network`).

- [ ] **Step 1: Author `driver/driver.go`** (the `0087` driver shape): start the `zipkincollector` on the shared-bridge-reachable addr; fire N requests through the proxy; POLL `collector.Count()` until it converges to N (a release barrier — `reference_concurrency_differential_release_barrier`, NEVER a `time.Sleep`); then a sub-batch of M requests carrying an incoming `b3` with a FIXED trace-id (the continuation prong). Assertions (cross-side EXACT, AGGREGATED over received spans — `reference_streaming_sink_differential_framing`):
  - span count == N+M (each side); for every span: `name == <fixed authority>`, `kind == "SERVER"`, `timestamp > 0` + `duration > 0`, and the deterministic VALUE-assertable tags (`http.method`/`http.url`/`http.protocol`/`http.status_code`/`component=proxy`/`downstream_cluster=-`/`response_flags=-`/`request_size`/`response_size`/`user_agent`); the KEY-PRESENT-only tags (`upstream_cluster`/`upstream_cluster.name` [EMPTY — `reference_tracing_upstream_cluster_framework_gap`], `peer.address` [env-specific], `guid:x-request-id` [value varies]); `traceId` width per `trace_id_128bit`.
  - **continuation prong:** the M spans from the `b3`-carrying sub-batch ⇒ every `traceId == <the incoming trace-id>` and (under `shared_span_context:false`) `parentId == <the incoming span-id>` (or under `:true`, `id == <the incoming span-id>` + `shared == true`).
  - **UNasserted:** the `traceId`/`id` VALUES (except continuation); `timestamp`/`duration` VALUES; `localEndpoint`; the POST count / per-POST batch sizes; the `x-request-id`/`peer.address` VALUES.
  - Verify decode RAN (`collector.Count() > 0`) before asserting — a zero-span green is a false pass (`reference_docker_probe_bridge_network`).

- [ ] **Step 2: Author `envoy.yaml` + `envoy-go.yaml`** — the `0087` shape with: an H1 downstream listener → a route to a fixed-body backend; the HCM `tracing` block with the Zipkin provider; a `collector_cluster` (plaintext HTTP/1 STRICT_DNS → the shared-bridge collector hostname:port — NO `http2_protocol_options`). The collector hostname must be reachable from the reference container.

- [ ] **Step 3: Run the differential** — `go test ./test/differential/ -run 'TestDifferential/0088' -count=1 -v` ⇒ PASS (read the `-v` assertion lines: confirm N+M spans, the continuation trace-id match, the SERVER kind — NOT a vacuous green). Confirm the collector received spans from BOTH sides.

- [ ] **Step 4: Subject-side unit tests** (the boot-reject + strict-rejects that are NOT cross-side-informative — both sides `log.Fatalf`/reject): the `HTTP_PROTO`/`split_spans_for_request`/empty-`collector_cluster` parse rejects (Task 5 covers these in `config_test.go` — confirm); the `ExporterProvider` Zipkin-arm cluster-miss boot-reject (Task 8 covers it in `exporter_test.go` — confirm). NO new fixture dirs for these (subject-side units, per the SPEC §8.1 boot-reject posture).

- [ ] **Step 5: Blank-import + commit**
```bash
# add: _ "github.com/esalaine/envoy-go/test/fixtures/0088-tracing-zipkin/driver" to runner_test.go
gofmt -l test/ && golangci-lint run ./test/... && go build ./... && go vet ./test/...
git add test/fixtures/0088-tracing-zipkin/ test/differential/runner_test.go
git commit -m "phase 46.2 Task 12: 0088-tracing-zipkin cross-side EXACT differential (poll-to-converge, stable v2-JSON subset + B3 continuation prong) over a driver-owned HTTP Zipkin collector (fixtures 89→90; D-TRACE-ZIPKIN-DIFFERENTIAL)"
```

---

## Task 13: Deliberate breaks + flake-soak + full-package -race + the full 90-dir differential + six-gate + docs (ADR-0261, row 46 → done)

**Files:**
- Modify: `docs/envoy-go/phases/46-tracing/PROGRESS-46.2.md`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`

- [ ] **Step 1: Deliberate-break verification** (`reference_differential_break_protocol_count1` — `-count=1` on EVERY break; restore via `git restore`, NEVER checkout-sha/amend, `feedback_subagent_worktree_detach`). Prove each `0088` assertion is LIVE: (a) break the span `name` (emit `"ingress"` instead of the authority) ⇒ the name assertion FAILS; (b) break the B3 continuation (drop the incoming trace-id) ⇒ the continuation-prong assertion FAILS; (c) break the SERVER kind ⇒ FAILS; (d) skip the export (return before `Export`) ⇒ the span-count assertion FAILS (the receiver gets 0). Restore after each; re-run `-run 'TestDifferential/0088' -count=1` ⇒ PASS.

- [ ] **Step 2: Flake-soak** — `for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0088' -count=1 || break; done` ⇒ 20/20 PASS (the band/convergence is flake-free).

- [ ] **Step 3: Full-package -race** — `go test ./internal/tracing/ ./internal/filter/hcm/ -race -count=1` ⇒ PASS (the `ZipkinExporter` writer goroutine is a background mutator; the FULL packages, not a subset).

- [ ] **Step 4: The full differential + six-gate** — `gofmt -l .` (empty) + `golangci-lint run ./...` + `go vet ./...` + `go build ./...` + `go test ./... -count=1` + the FULL `go test ./test/differential/ -count=1` (all 90 dirs). A transient unrelated `subject ready: EOF` startup flake is isolatable (`reference_differential_fullsuite_startup_flake`) — isolate-re-run the offending dir to distinguish a regression from the known startup race. Record the verbatim outputs in PROGRESS-46.2.md.

- [ ] **Step 5: Reconcile counts** — `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l` == 90; `grep -rh '^func Fuzz' --include='*.go' . | wc -l` == 49 (`reference_fuzzer_count_docs_drift` — reconcile the documented running total); the stat surface 1198 → 1200 (the +2 `tracing.zipkin.*`); BackendKind 38 (unchanged); `go mod tidy -diff` EMPTY (0 new modules).

- [ ] **Step 6: ADR-0261 body** — append the ADR-0261 §Decision + §Consequences to `docs/envoy-go/DECISIONS.md` (the §13 SPEC-46.2 §Context draft is the lead-in; ADR-0044 atomic-landing). The §Decision: the Zipkin exporter is a SECOND `tracing.Exporter` (v2 JSON over `httpclient.ClusterDispatch`); the B3 codec; the provider dispatch; the `tracing.zipkin.{spans_sent,spans_dropped}` stats; the boot-reject reference-parity; ZERO new packages/modules. The §Consequences: the FINAL chartered tracing leg (row 46 `done`); the Observability family STAYS OPEN; the deferred `HTTP_PROTO`/`GRPC`/`split_spans_for_request`/`custom_tags`/`spawn_upstream_span` set.

- [ ] **Step 7: BEHAVIOR_CONTRACT** — add the `### Request tracing — Zipkin tracing provider + B3 propagation` subsection (the §9 SPEC-46.2 contract delta): the provider-agnostic decision; B3 extract/continue/inject (multi-header inject); the v2 JSON export (`name`=authority, 14 tags, microsecond timing, `shared`/`parentId`); the strict-rejects; the `log.Fatalf` missing-cluster boot-reject (reference-parity); the stat-surface advance 1198 → 1200.

- [ ] **Step 8: STATE + ROADMAP** — STATE.md: the active-phase header → "phase 46.2 IMPL done" + the NEXT (the next ROADMAP row) + the counts (1200 / 90 / 49 / 38 / ADR-0261). ROADMAP.md: **row 46 (`tracing`) FLIPS `done`** ("46.1 + 46.2 COMPLETE") per ADR-0106 + `reference_roadmap_split_phase_row_done`; the Observability family STAYS OPEN (stats sinks + the tap filter remain future rows).

- [ ] **Step 9: Commit**
```bash
git add docs/envoy-go/
git commit -m "phase 46.2 Task 13: ADR-0261 body + BEHAVIOR_CONTRACT + STATE/ROADMAP (row 46 → done) + full 90-dir differential + six-gate green + count reconcile (1200/90/49/38/ADR-0261)"
```

---

## Exit criteria (controller-verified at stage-close, on the FINAL frozen HEAD)

- Six gates green on the frozen HEAD: `gofmt -l .` empty, `golangci-lint run ./...`, `go vet ./...`, `go build ./...`, `go test ./... -count=1`, the FULL `go test ./test/differential/ -count=1` (90 dirs, modulo the documented startup-flake class — isolate-re-run to confirm).
- The deliberate-break verification re-done by the CONTROLLER (not trusted from the subagent logs).
- Counts: stat **1200** (non-H2 **1196**) / fixtures **90** (`0088-tracing-zipkin`) / fuzzers **49** (`FuzzExtractB3`) / BackendKind **38** / DECISIONS **ADR-0261**. `go mod tidy -diff` EMPTY; ZERO new packages.
- **ROADMAP row 46 (`tracing`) == `done`**; the Observability family STILL OPEN.
- The worktree's commits SQUASHED into one + fast-forwarded onto master + pushed (`feedback_subagents_no_push` · `feedback_push_to_origin`); the worktree removed.
