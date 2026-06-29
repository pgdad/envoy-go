# 0089-stats-sink-metrics-service

Cross-side EXACT differential for the phase 47.1 core `metrics_service` stats
sink. Proves that envoy-go (subject, in-process) and reference Envoy
(`contrib-v1.37.2`, Docker) both export their stats registry over the
`MetricsService.StreamMetrics` client-streaming gRPC RPC as
`io.prometheus.client.MetricFamily` protos, agreeing on a deterministic COUNTER
name subset and the identifier node.

## Topology

Single-listener plaintext H1 → an `HTTPFixedBody` backend (`c_backend`,
17-byte body), with a **bootstrap-level** `stats_sinks[]` `metrics_service`
entry on BOTH sides:

- `transport_api_version: V3` (the reference HARD-REJECTS a deprecated non-V3
  value — `AMEND-MS-REFERENCE-STRICTER`).
- `grpc_service.envoy_grpc.cluster_name: c_metrics` — an **h2c** cluster
  (`http2_protocol_options: {}`, no TLS) pointing at the driver-owned in-process
  `test/helpers/metricsservice` receiver.
- `stats_flush_interval: 0.5s` — a SHORT interval for fast deterministic
  convergence (vs the 5s default).
- `node: { id, cluster }` — FIXED identically on both sides → the
  `StreamMetricsMessage.identifier.node` (msg #1), cross-side assertable.

The HCM `stat_prefix` (`hcm_local`) and the backend cluster name (`c_backend`)
are FIXED identically on both sides so the mapped dotted stat names match
cross-side.

## Receiver reachability (`reference_docker_probe_bridge_network`)

The driver allocates a stable metrics port, binds the `metricsservice.Server` on
`0.0.0.0:<port>` BEFORE either proxy starts, and templates the address into both
bootstraps: `host.docker.internal:<port>` for the reference container (ADR-0010
bridge alias, STRICT_DNS) and `127.0.0.1:<port>` for the subject (STATIC). Both
sides stream to the SAME receiver.

## Workload + release barrier

Each side fires K=7 deterministic `GET /probe` requests (all 2xx), then POLLS
the receiver until the deterministic COUNTER subset converges to value == 7 on
EACH side — a release barrier (`reference_concurrency_differential_release_barrier`),
NEVER a `time.Sleep`. The receiver is `Reset()` at the start of each side's
drive for clean per-side separation.

## Asserted (cross-side EXACT, deterministic COUNTER NAME SUBSET)

Per `AMEND-MS-HISTOGRAM-PRESENT` this is a name-SUBSET, NOT the whole family set
(the surfaces differ cross-side — envoy-go has no histograms; the reference
does). On BOTH sides, for each of:

- `cluster.c_backend.upstream_rq_total`
- `http.hcm_local.downstream_rq_total`
- `http.hcm_local.downstream_rq_2xx`

the family is present (decode ran — a zero-family pass is structurally
impossible), `type == COUNTER`, and `value == 7 == K`. PLUS the identifier
`node.id == "envoy-go-subject-0089"` and `node.cluster == "envoy-go-differential"`.

## UNasserted

The whole family set / family count (surfaces differ); `metric[].timestamp_ms`
VALUE (non-deterministic — `AMEND-MS-TIMESTAMP-ALWAYS-SET`, presence-only);
`help`; non-deterministic gauges (`server.uptime`, `*_active`, connection
churn); the identifier `user_agent_*`/`extensions[]`; StreamMetrics
message/stream framing + per-message family count
(`reference_streaming_sink_differential_framing`).

## Debug

Set `FIXTURE_0089_DUMP=1` to print the per-side captured subset readings + node
identifier to stderr.
