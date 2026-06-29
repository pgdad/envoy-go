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

## Two private per-side receivers + hard `Close()`

The `metrics_service` sink flushes **periodically** — the reference proxy keeps
streaming into its receiver for the whole test, including during the subject's
drive window (`reference_periodic_sink_differential_two_receivers`). A single
shared receiver would let the reference's concurrent flushes contaminate the
subject snapshot (and silently defeat subject-side deliberate breaks). So the
driver starts **two private receivers** — one per side — on two
separately-allocated host ports, each bound on `0.0.0.0:<port>` BEFORE either
proxy starts (`reference_docker_probe_bridge_network`), and templates the address
into each bootstrap: `host.docker.internal:<refPort>` for the reference container
(ADR-0010 bridge alias, STRICT_DNS) and `127.0.0.1:<subjPort>` for the subject
(STATIC). Each side owns an uncontaminated accumulator, so **no `Reset()`**
between sides is needed. After the subject snapshot BOTH receivers are
hard-stopped via `Close()` (`grpc.Server.Stop`), NOT `Stop()`/`GracefulStop` —
the proxies hold their long-lived `StreamMetrics` streams open, so `GracefulStop`
would block until the test timeout.

## Workload + release barrier

Each side fires K=7 deterministic `GET /probe` requests (all 2xx), then POLLS
that side's private receiver until the deterministic COUNTER subset converges to
value == 7 on EACH side — a release barrier
(`reference_concurrency_differential_release_barrier`), NEVER a `time.Sleep`.

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
