# 0112-stats-sink-otlp

Cross-side differential for the phase 69 OpenTelemetry (OTLP) metrics stats sink
(`envoy.stat_sinks.open_telemetry`, ADR-0291) — the **DEFAULT arm** (all knobs
absent). Proves that envoy-go (subject, in-process) and reference Envoy
(`contrib-v1.37.2`, Docker) both map their stats registry to ONE unary OTLP
`ExportMetricsServiceRequest` per flush and agree on a deterministic COUNTER
residual-name subset: monotonic **CUMULATIVE** `Sum`s with `value == K`,
**tag-extracted RESIDUAL** metric names, tags emitted as `envoy.<tag>`
attributes, and the `telemetry.sdk.*` resource triple.

## Topology

Single-listener plaintext H1 → an `HTTPFixedBody` backend (`c_backend`,
17-byte body), with a **bootstrap-level** `stats_sinks[]` `open_telemetry` entry
on BOTH sides:

- `typed_config: envoy.extensions.stat_sinks.open_telemetry.v3.SinkConfig` with
  **NO knob fields** — the DEFAULT arm: `report_counters_as_deltas` defaults
  FALSE ⇒ CUMULATIVE; `use_tag_extracted_name` and `emit_tags_as_attributes` are
  `*BoolValue` doc-default **TRUE**.
- `grpc_service.envoy_grpc.cluster_name: c_otlp` — an **h2c** cluster
  (`http2_protocol_options: {}`, no TLS) pointing at the driver-owned in-process
  `test/helpers/otlpmetrics` receiver. `dns_lookup_family: V4_ONLY` on the
  reference side (P-16, the Docker Desktop AAAA gotcha).
- `stats_flush_interval: 0.1s` — a SHORT interval for fast deterministic
  convergence.

The HCM `stat_prefix` (`hcm_local`) and the backend cluster name (`c_backend`)
are FIXED identically on both sides so the tag-extracted residual names +
`envoy.<tag>` attribute values match cross-side.

## Two private per-side receivers + hard `Close()`

The sink flushes PERIODICALLY, so the reference keeps Exporting into its receiver
during the subject's drive window. The driver gives each side its OWN private
`otlpmetrics.Server` on a separately-allocated port
(`reference_periodic_sink_differential_two_receivers`) — no `Reset()` between
sides. After the subject snapshot BOTH receivers are hard-stopped via `Close()`.

## The residual-name collision (why the cluster stat is attribute-qualified)

Under `use_tag_extracted_name=true`, tag extraction collapses
`cluster.<name>.upstream_rq_total` → residual `cluster.upstream_rq_total`. BOTH
the application backend cluster (`c_backend`) AND the OTLP sink's OWN gRPC cluster
(`c_otlp`) emit that residual in the SAME Export, differing only in the
`envoy.cluster_name` attribute — and the sink cluster's count grows with every
flush. A name-only lookup is therefore ambiguous, so the driver selects
`c_backend`'s datapoint by its `envoy.cluster_name` attribute (the receiver's
attribute-qualified `Datapoints()` snapshot, added by this task). The
LISTENER-scoped `http.*` residuals do NOT collide (one HCM), so their `value==K`
lookup uses the residual name directly.

## Workload + asserted subset

K = 7 deterministic `GET /probe` requests/side (all 2xx). Poll each side's
receiver until the residual Sum subset converges to `value == 7` AND ≥1 Export has
arrived (served-this-arm), then ≥2 further flushes for the StartTime barrier.

Asserted on BOTH sides (NAMED subsets only,
`reference_stats_sink_emits_used_only`):

| residual | temporality | value | attributes |
| --- | --- | --- | --- |
| `http.downstream_rq_total` | cumulative Sum, monotonic | 7 | `envoy.http_conn_manager_prefix=hcm_local` |
| `http.downstream_rq_xx` | cumulative Sum, monotonic | 7 | `envoy.http_conn_manager_prefix=hcm_local`, `envoy.response_code_class=2` |
| `cluster.upstream_rq_total` (where `envoy.cluster_name=c_backend`) | cumulative Sum, monotonic | 7 | `envoy.cluster_name=c_backend` |

Plus: the three `telemetry.sdk.*` resource KEYS present (per-side values
unasserted); and (subject) the cumulative `StartTimeUnixNano` is ns-magnitude and
CONSTANT across ≥2 further flushes.

## NOT asserted

The whole family set/count (surfaces differ — envoy-go has no histograms); the
reference's `StartTimeUnixNano` shape (its µs factor-1000 bug — never cross-side
StartTime equality); the `c_otlp` sink cluster's own value + `upstream_cx_*`
(dial-unaccounted + the feedback loop,
`reference_cluster_sink_dial_unaccounted`); OTLP message framing + per-Export
metric count + flush cadence (`reference_streaming_sink_differential_framing`).

## Empirical residual-name confirmation

The tag-extracted residual spellings above were confirmed by running the fixture
with `FIXTURE_0112_DUMP=1`, which logs every received datapoint (name + sorted
attributes + value + type) on both sides.
