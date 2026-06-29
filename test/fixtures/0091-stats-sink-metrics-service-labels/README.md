# 0091-stats-sink-metrics-service-labels

Cross-side EXACT differential for the phase 47.2b `emit_tags_as_labels` knob on
the landed 47.1/47.2a `metrics_service` stats sink. Proves that envoy-go
(subject, in-process) and reference Envoy (`contrib-v1.37.2`, Docker), both
configured with `emit_tags_as_labels: true`, split each stat name into a
**residual** dotted name plus `metric[].label[]` LabelPairs over the
`MetricsService.StreamMetrics` client-streaming gRPC RPC, agreeing on a
deterministic COUNTER subset keyed by `{residual name, sorted labels}` whose
**cumulative** value reaches `K`, and on the identifier node.

## Labels-split + cumulative-value model (the key departure from 0090)

Under `emit_tags_as_labels: true` each stat name is split per the statsd SN tag
rules into a **residual** dotted name plus a set of LabelPairs, each keyed by the
Envoy **dotted** tag-name (`envoy.<tag>`). The Counter value stays the
**cumulative absolute** (`== K` after `K` 2xx requests) — the 0089 last-seen
value model, NOT a per-flush delta. There is **no delta-SUM** and **no
post-convergence stability barrier** here (those are 47.2a/0090-only concerns);
the cumulative value needs only first-reach-`K`. This fixture asserts the
last-seen `value == K` via the label-keyed `FamilyWithLabels(name, labels)`
accessor.

## Topology

Single-listener plaintext H1 → an `HTTPFixedBody` backend (`c_backend`,
17-byte body), with a **bootstrap-level** `stats_sinks[]` `metrics_service`
entry on BOTH sides:

- `transport_api_version: V3` (the reference HARD-REJECTS a deprecated non-V3
  value — `AMEND-MS-REFERENCE-STRICTER`).
- `emit_tags_as_labels: true` — the knob under test (ADR-0264).
- `grpc_service.envoy_grpc.cluster_name: c_metrics` — an **h2c** cluster
  (`http2_protocol_options: {}`, no TLS) pointing at the driver-owned in-process
  `test/helpers/metricsservice` receiver.
- `stats_flush_interval: 0.5s` — a SHORT interval for fast deterministic
  convergence (vs the 5s default).
- `node: { id, cluster }` — FIXED identically on both sides → the
  `StreamMetricsMessage.identifier.node` (msg #1), cross-side assertable.

The HCM `stat_prefix` (`hcm_local`) and the backend cluster name (`c_backend`)
are FIXED identically on both sides so the extracted label VALUES match
cross-side.

## Two private per-side receivers + hard `Close()`

The `metrics_service` sink flushes **periodically** — the reference proxy keeps
streaming into its receiver for the whole test, including during the subject's
drive window (`reference_periodic_sink_differential_two_receivers`). A single
shared receiver would let the reference's concurrent flushes contaminate the
subject snapshot (and silently defeat subject-side deliberate breaks). So the
driver starts **two private receivers** — one per side — on two
separately-allocated host ports, each bound on `0.0.0.0:<port>` BEFORE either
proxy starts, and templates the address into each bootstrap:
`host.docker.internal:<refPort>` for the reference container (ADR-0010 bridge
alias, STRICT_DNS) and `127.0.0.1:<subjPort>` for the subject (STATIC). Each side
owns an uncontaminated accumulator, so **no `Reset()`** between sides is needed.
After the subject snapshot BOTH receivers are hard-stopped via `Close()`
(`grpc.Server.Stop`), NOT `Stop()`/`GracefulStop` — the proxies hold their
long-lived `StreamMetrics` streams open, so `GracefulStop` would block until the
test timeout.

## Workload + release barrier

Each side fires K=7 deterministic `GET /probe` requests (all 2xx), then POLLS
that side's private receiver until the deterministic label-keyed COUNTER subset's
last-seen **value** reaches `== 7` (`FamilyWithLabels`), AND the identifier node
has arrived — a release barrier
(`reference_concurrency_differential_release_barrier`), NEVER a `time.Sleep`. The
cumulative-value model needs only first-reach-`K`; there is **no** stability
barrier.

## Label-set ordering normalized

`FamilyWithLabels` compares the queried labels against the received label set in
**sorted key order**, so the LabelPair emission order is not load-bearing
cross-side. The composite key `{residual name, sorted labels}` also keeps apart
families that share a residual name but differ by label value (e.g.
`cluster.upstream_rq_total` for both `c_backend` and `c_metrics`).

## Asserted (cross-side EXACT, deterministic label-keyed COUNTER SUBSET)

Per `AMEND-MS-HISTOGRAM-PRESENT` this is a label-keyed SUBSET, NOT the whole
family set (the surfaces differ cross-side — envoy-go has no histograms; the
reference does). `upstream_cx_total` is excluded — connection reuse makes it
`< K`. On BOTH sides, for each `{residual, labels}`:

- `cluster.upstream_rq_total` `{envoy.cluster_name=c_backend}`
- `http.downstream_rq_total` `{envoy.http_conn_manager_prefix=hcm_local}`
- `http.downstream_rq_xx` `{envoy.http_conn_manager_prefix=hcm_local, envoy.response_code_class=2}`

the family is present (decode ran — a zero-family pass is structurally
impossible), `type == COUNTER`, and the cumulative `value == 7 == K`. The third
is the **2xx two-label SN4 split**: the `response_code_class` tag rule extracts
the `2` digit and rewrites `downstream_rq_2xx` → the residual `downstream_rq_xx`,
leaving a two-label set. PLUS the identifier
`node.id == "envoy-go-subject-0091"` and `node.cluster == "envoy-go-differential"`.

## UNasserted

The whole family set / family count (surfaces differ); `metric[].timestamp_ms`
VALUE (non-deterministic — `AMEND-MS-TIMESTAMP-ALWAYS-SET`, presence-only);
`help`; LABELED gauges (`server.uptime`, `*_active`, connection churn);
the identifier `user_agent_*`/`extensions[]`; StreamMetrics message/stream
framing + per-message family count
(`reference_streaming_sink_differential_framing`); the LabelPair emission order
(normalized via sorted-key comparison).

## Debug

Set `FIXTURE_0091_DUMP=1` to print the per-side captured subset label-keyed
values + node identifier to stderr.
