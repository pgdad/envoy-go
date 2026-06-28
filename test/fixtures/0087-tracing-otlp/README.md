# 0087-tracing-otlp

Cross-side differential (subject envoy-go vs reference Envoy `contrib-v1.37.2` in
Docker) for **phase 46.1b span emission + OTLP export**. This fixture proves the
complete span pipeline: the HCM `tracing` block (OpenTelemetry provider,
`random_sampling: 100%`) generates one `ingress`/`SPAN_KIND_SERVER` OTLP span per
request and exports it to a driver-owned receiver via h2c gRPC.

## Topology

Single-listener plaintext HTTP/1.1. One downstream listener (`l_test`,
`stat_prefix: hcm_local`) routes every request (prefix `/`) to `c_backend` — the
**HTTPFixedBody** backend (`"backend:v1/fixed\n"` = 17 bytes). The HCM carries a
`tracing` block:

```yaml
tracing:
  provider:
    name: envoy.tracers.opentelemetry
    typed_config:
      "@type": type.googleapis.com/envoy.config.trace.v3.OpenTelemetryConfig
      grpc_service:
        envoy_grpc: { cluster_name: c_otlp_collector }
      service_name: "0087"
  random_sampling: { value: 100 }
```

`c_otlp_collector` is a **real** STRICT_DNS (reference) / STATIC (envoy-go) h2c
cluster pointing at the **driver-owned in-process `otlptrace.Server`** receiver.
The receiver is bound on `0.0.0.0:<port>` before either proxy starts, so the
reference container can reach it at `host.docker.internal:<port>` (per ADR-0010)
and envoy-go at `127.0.0.1:<port>`.

## Capture mechanism — driver-owned OTLP TraceService receiver

`test/helpers/otlptrace` (`otlptrace.NewAtAddr`) is a minimal in-process
`coltracepb.TraceServiceServer` that accumulates every `*tracepb.Span` across
all `Export` calls (flattening the `ResourceSpans/ScopeSpans` nesting). The driver
uses `Count()` to poll for convergence and `Spans()`/`ResourceAttributes()` to
snapshot before `Reset()` for per-side separation. This is the trace counterpart of
the `otlplogs` receiver used in fixture 0084.

## Workload — N + M = 12 requests per side

Each side fires, against the proxy under test:

- **N = 8 PLAIN** requests (`GET /trace`, `Host: trace.example`,
  `User-Agent: trace-probe/1`, no inbound trace context). Under
  `random_sampling: 100%` the decision is deterministic: each is a **fresh local
  sample** — a new trace-id, no parent span id.
- **M = 4 CONTINUATION** requests additionally carrying
  `Traceparent: 00-aaaa…aaaa-bbbb…bbbb-01` (a FIXED, non-zero inbound W3C trace
  context). The proxy **continues** the trace: the exported span carries
  `trace_id == aaaa…aaaa` (the fixed inbound id) and
  `parent_span_id == bbbb…bbbb` (the fixed inbound parent span id).

## `random_sampling: 100%` determinism

Setting `random_sampling: { value: 100 }` ensures that EVERY request generates a
span. This makes the total span count deterministic at N+M=12 per side — no
sampling-based flakiness.

## Release barrier — POLL, never sleep

The reference Envoy buffers spans and flushes them on a timer / buffer-fill. The
driver polls `srv.Count() >= 12` at 200ms intervals with a 30s deadline (the
`reference_concurrency_differential_release_barrier` discipline). A fixed sleep
would be flaky (too short on slow CI) or slow (too long on fast hardware).

## Decode-ran proof

The `Count()` poll guarantees spans > 0 on BOTH sides before asserting. A
0-span "pass" (failed export, misconfigured cluster) is structurally impossible.

## Framing NOT asserted

The cross-side assertion is on the **per-span PAYLOAD aggregated across all
`Export` calls** — NOT the `Export`-call count, per-call batch sizes, connection
count, or flush cadence. These legitimately vary side-to-side
(`reference_streaming_sink_differential_framing`).

## Continuation prong — trace-id invariant

The M=4 continuation spans are identified by `trace_id == contTraceID` bytes
(`aaaa…aaaa`). For each, the driver asserts `parent_span_id == contParentID`
(`bbbb…bbbb`) — the cross-side EXACT continuation invariant: both sides CONTINUE
the inbound trace and set the parent span id to the inbound parent.

## `upstream_cluster` / `upstream_cluster.name` — framework-gap deviation

envoy-go's upstream cluster name is **not plumbed** to the request-completion
seam where the span is built (the same gap as the Lua bridge's `UpstreamCluster()`
which returns `""`). envoy-go therefore emits:

```
upstream_cluster       = ""
upstream_cluster.name  = ""
```

while the reference Envoy emits `"c_backend"`. The attribute **KEYS** are present
on both sides; the **VALUES** differ and are explicitly **UNasserted cross-side**.
The subject-side driver only asserts KEY presence (not value). This deviation is
intentional and documented as a known framework gap — it will be addressed in a
future phase that plumbs the cluster name through the request context.

## Attribute value-type parity (`http.status_code`, `request_size`, `response_size`)

envoy-go's `span.go` currently emits `http.status_code`, `request_size`, and
`response_size` as `AnyValue_IntValue` (INT). The reference Envoy cpp OTel tracer
may emit these as `AnyValue_StringValue` (STRING). The driver uses **normalized
comparison** (`assertAttrNormalized`) — converting both INT and STRING to their
decimal string form before comparison — so the assertion is type-agnostic and the
differential passes regardless of which form either side uses. If you observe that
the reference uses STRING, adjust `internal/tracing/span.go` to match.

## SDK / scope NOT asserted

`telemetry.sdk.*` resource attributes and `ScopeSpans.scope.name`/`version` are
impl-specific (envoy-go is not a cpp SDK). These are intentionally UNasserted.

## Notes

- One fixture dir = one runner branch
  (`reference_differential_fixture_dispatch_constraint`).
- This fixture CLOSES ADR-0260 at IMPL (the completing sub-leg of row 46 —
  span emission + OTLP export, following 46.1a header engine).
