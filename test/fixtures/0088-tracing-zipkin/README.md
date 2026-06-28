# 0088-tracing-zipkin

Cross-side differential (subject envoy-go vs reference Envoy `contrib-v1.37.2` in
Docker) for **phase 46.2 Zipkin v2-JSON span export + B3 propagation**. This
fixture proves the SECOND tracing exporter behind the `tracing.Exporter` seam: the
HCM `tracing` block (Zipkin provider, `random_sampling: 100%`) generates one
`SERVER` span named after the request `:authority` per request and POSTs it as a
Zipkin v2 JSON array to a driver-owned HTTP collector, AND continues an inbound B3
trace context.

## Topology

Single-listener plaintext HTTP/1.1. One downstream listener (`l_test`,
`stat_prefix: hcm_local`) routes every request (prefix `/`) to `c_backend` — the
**HTTPFixedBody** backend (`"backend:v1/fixed\n"` = 17 bytes). The HCM carries a
`tracing` block:

```yaml
tracing:
  provider:
    name: envoy.tracers.zipkin
    typed_config:
      "@type": type.googleapis.com/envoy.config.trace.v3.ZipkinConfig
      collector_cluster: c_zipkin_collector
      collector_endpoint: /api/v2/spans
      collector_endpoint_version: HTTP_JSON
      trace_id_128bit: true
      shared_span_context: false
  random_sampling: { value: 100 }
```

`c_zipkin_collector` is a **real** STRICT_DNS (reference) / STATIC (envoy-go)
**plaintext HTTP/1** cluster (NO `http2_protocol_options` — Zipkin v2 JSON POSTs
over HTTP/1.1, unlike the OTLP gRPC h2c collector in fixture 0087) pointing at the
**driver-owned in-process `zipkincollector.Collector`** receiver. The collector is
bound on `0.0.0.0:<port>` before either proxy starts, so the reference container
reaches it at `host.docker.internal:<port>` (per ADR-0010) and envoy-go at
`127.0.0.1:<port>`.

## Capture mechanism — driver-owned HTTP Zipkin collector

`test/helpers/zipkincollector` (`zipkincollector.NewAtAddr`) is a minimal
in-process `net/http` server that accumulates every Zipkin v2 JSON span POSTed to
`/api/v2/spans` across POSTs (de-chunking the body transparently). The driver uses
`Count()` to poll for convergence and `Spans()` to snapshot before `Reset()` for
per-side separation. This is the HTTP analog of the `otlptrace` gRPC receiver used
in fixture 0087.

## Workload — N + M = 12 requests per side

Each side fires, against the proxy under test:

- **N = 8 PLAIN** requests (`GET /trace`, `Host: trace.example`,
  `User-Agent: trace-probe/1`, no inbound trace context). Under
  `random_sampling: 100%` each is a **fresh local sample** — a new random
  trace-id, no parent.
- **M = 4 CONTINUATION** requests additionally carrying a single
  `b3: 0123456789abcdef0123456789abcdef-fedcba9876543210-1` header (the 3-field B3
  single-header form `<traceId>-<spanId>-<sampled>`). The proxy **continues** the
  trace: the exported span carries `traceId == 0123…cdef` (the fixed inbound id);
  under `shared_span_context:false` a fresh span id is generated and
  `parentId == fedcba9876543210` (the inbound span-id becomes the server span's
  parent).

## `shared_span_context: false` — the deliberate choice

The fixture pins `shared_span_context: false` (NOT the Envoy default `true`). This
is the more **discriminating** continuation assertion:

- **`false`** (chosen): the server generates a fresh span id and sets
  `parentId == <inbound span-id>`. The driver asserts `parentId == contSpanID`
  directly.
- **`true`** (default, not chosen): the server **reuses** the inbound span-id as
  its own `id` and sets `shared: true` with no `parentId`. Asserting `id ==
  contSpanID` is weaker (it conflates the server's id with the inbound id).

Choosing `false` lets the continuation prong pin a non-trivial relationship
(server-parent → inbound-span) that exercises the B3 extractor's parent derivation.

## B3 3-field form — parent derivation

The 3-field `b3: <traceId>-<spanId>-<sampled>` (no explicit 4th parent field)
means `spanId` is the upstream caller's current span. Both the reference Zipkin
tracer and envoy-go's `ExtractB3` treat that incoming `spanId` as the **parent** of
the new server span (under `shared:false`). This is the general rule: the incoming
SPAN-ID is ALWAYS our server span's parent (SPEC §11 D-TRACE-ZIPKIN-B3). The
optional 4th field (`parentSpanId`) is the caller's OWN parent — our grandparent —
and is accepted-but-ignored; the server still continues on the incoming span-id.
This fixture uses the 3-field form simply because it is the minimal unambiguous
shape that exercises the continuation parent derivation cross-side.

## `trace_id_128bit: true` — traceId width

`trace_id_128bit: true` makes every emitted `traceId` 32 lowercase hex chars
(128-bit). The continuation requests carry a 128-bit inbound trace-id, so both the
fresh (random) and continuation spans emit a full 32-hex traceId. The driver
asserts `len(traceId) == 32` on every span.

## `random_sampling: 100%` determinism

`random_sampling: { value: 100 }` ensures EVERY request generates a span — the
total span count is deterministic at N+M=12 per side, no sampling flakiness.

## Release barrier — POLL, never sleep

The reference Envoy buffers spans and flushes them on a timer / buffer-fill. The
driver polls `col.Count() >= 12` at 200 ms intervals with a 30 s deadline (the
`reference_concurrency_differential_release_barrier` discipline). A fixed sleep
would be flaky (too short on slow CI) or slow (too long on fast hardware).

## Decode-ran proof + continuation/fresh discrimination

The `Count()` poll guarantees spans > 0 on BOTH sides before asserting — a 0-span
"pass" is structurally impossible. The continuation prong additionally asserts
**exactly M = 4** spans carry `traceId == contTraceID` AND **exactly N = 8** carry
a different (random) trace-id — proving the continuation/fresh split is live, not
vacuous.

## Framing NOT asserted

The cross-side assertion is on the **per-span PAYLOAD aggregated across all POSTs**
— NOT the POST count, per-POST batch sizes, connection count, or flush cadence.
These legitimately vary side-to-side
(`reference_streaming_sink_differential_framing`).

## `upstream_cluster` / `upstream_cluster.name` / `peer.address` — framework gap

envoy-go's upstream cluster name is **not plumbed** to the request-completion seam
where the span is built (the same gap as fixture 0087 and the Lua bridge's
`UpstreamCluster()` which returns `""`). envoy-go emits:

```
upstream_cluster       = ""
upstream_cluster.name  = ""
```

while the reference Envoy emits the real cluster name. `peer.address` is
env-specific. These tag **KEYS** are present on both sides; the **VALUES** differ
and are explicitly **UNasserted cross-side** (the driver asserts KEY presence
only). This is the documented framework gap
(`reference_tracing_upstream_cluster_framework_gap`).

## `localEndpoint` / `node_id` / `zone` NOT asserted

The Zipkin `localEndpoint` (serviceName host/port) is env/impl-specific and not
modeled by the collector's `ReceivedSpan`. The Zipkin encoder **drops** `node_id`
and `zone` from the 16-attr roster (14 tags emitted). The reference MAY emit extra
tags beyond the asserted subset — the driver asserts specific keys/values, NOT
tag-set equality.

## Notes

- One fixture dir = one runner branch
  (`reference_differential_fixture_dispatch_constraint`).
- The Zipkin-provider parse-reject arms NOT cross-side-informative (HTTP_PROTO /
  `split_spans_for_request` / empty `collector_cluster`) are covered by
  `internal/tracing/config_test.go`; the ExporterProvider Zipkin-arm cluster-miss
  boot-reject by `internal/tracing/exporter_test.go` — no fixture dirs for those.
- This fixture is the behavioral proof for the SECOND tracing exporter (Zipkin),
  anchoring ADR-0261 at the phase 46.2 IMPL.
