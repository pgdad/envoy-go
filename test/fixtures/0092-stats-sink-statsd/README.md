# 0092-stats-sink-statsd

Cross-side EXACT differential for the phase 48 statsd UDP line-protocol stats
sink. Proves that envoy-go (subject, in-process) and reference Envoy
(`contrib-v1.37.2`, Docker), both configured with a `statsd` `stats_sinks[]`
entry, export each COUNTER family's **per-flush delta** as a `|c` line and each
GAUGE's **absolute** value as a `|g` line, agreeing on a deterministic COUNTER
name subset whose per-flush deltas **SUM** to `K`, and on an absolute gauge
`cluster.<backend>.membership_total == 1`.

## Delta-SUM model (statsd `|c` counters)

Each statsd flush emits `<prefix>.<name>:<delta>|c` for every COUNTER (the
per-flush increment since the previous flush, NOT the cumulative absolute value).
A single-window burst makes the first flush's `delta == absolute == K`,
indistinguishable without post-convergence observation. The **stability barrier**
(`awaitFurtherFlushes`) observes `>= 2` further flushes after the delta-SUM
converges to `K`: under correct deltas the idle counters emit `0` each flush so
the SUM stays `K` (PASS); an absolute sink re-adds the cumulative so the SUM
overshoots `K` (FAIL). This is what makes the deliberate `emit-absolute` /
`skip-barrier` breaks catchable
(`reference_delta_sink_differential_stability_barrier`).

## Absolute gauge subset (D-SD-GAUGE-SUBSET)

`cluster.<backend>.membership_total` is emitted as an **absolute** `|g` line
each flush (the current gauge value, not a delta). The driver asserts the
captured gauge value `== 1` (one backend endpoint) on BOTH sides.

## Topology

Single-listener plaintext H1 → an `HTTPFixedBody` backend (`c_backend`,
17-byte body), with a **bootstrap-level** `stats_sinks[]` `statsd` entry on
BOTH sides:

- `@type: type.googleapis.com/envoy.config.metrics.v3.StatsdSink`
- `address.socket_address: { protocol: UDP, address: <statsd_host>, port_value: <statsd_port> }` — the driver-owned in-process `statsdrecv` UDP receiver.
- `prefix: sdpfx` — baked **identically** on both sides; all emitted lines are `sdpfx.<name>:<value>|<type>`.
- `stats_flush_interval: 0.5s` — a SHORT interval for fast deterministic convergence (vs the 5s default).

The HCM `stat_prefix` (`hcm_local`) and the backend cluster name (`c_backend`)
are FIXED identically on both sides so the mapped dotted stat names match
cross-side.

NO `node` field is needed — statsd carries no proxy identifier (contrast
`metrics_service` which carries the node in `StreamMetricsMessage.identifier`).

## Reference statsd address: LITERAL `HostGatewayIP`

The reference Envoy container cannot use `host.docker.internal` as the statsd
address because the statsd UDP sink does NOT perform DNS resolution
(`AMEND-SD-REJECT`; `reference_docker_probe_bridge_network`). The driver calls
`differential.HostGatewayIP(ctx)` (Docker `NetworkInspect` on the `"bridge"`
network) to obtain the **LITERAL IP** of the bridge gateway — the host's address
reachable from the reference container. This IP is baked into the reference
bootstrap at `ReferenceBootstrap` time. The receiver is bound on
`0.0.0.0:<refStatsdPort>` so the container can write datagrams to the host.
The subject bootstrap uses `127.0.0.1:<subjStatsdPort>` (loopback).

## Two private per-side receivers + hard `Close()`

The statsd sink flushes **periodically** — the reference proxy keeps sending to
its receiver for the whole test, including during the subject's drive window
(`reference_periodic_sink_differential_two_receivers`). The driver starts **two
private UDP receivers** — one per side — on two separately-allocated host ports,
each bound on `0.0.0.0:<port>` BEFORE either proxy starts. Each side owns an
uncontaminated accumulator (no `Reset()` needed). After the subject snapshot
BOTH receivers are hard-stopped via `Close()` — UDP is connectionless, so
`Close` is unambiguous (no `GracefulStop`-vs-hard-stop distinction; cf. the
`metricsservice` gRPC precedent in 0090 which uses `grpc.Server.Stop`).

## Workload + release barrier

Each side fires K=7 deterministic `GET /probe` requests (all 2xx), then POLLS
that side's private receiver until the deterministic COUNTER subset's per-flush
deltas **SUM** to `== 7` (`DeltaSum`) — a release barrier
(`reference_concurrency_differential_release_barrier`), NEVER a `time.Sleep`.

### Post-convergence stability barrier

After the delta-SUM converges to `K`, the driver waits until the receiver's
`SeenCount(marker) >= base + 2` (each flush increments `SeenCount` for the
marker name, even on a `0`-delta `|c` line — a flush-count signal). Only then
is the snapshot taken. This is the stability barrier that distinguishes correct
delta emission from broken absolute emission.

## Asserted (cross-side EXACT)

A name-SUBSET, NOT the whole line set (the reference emits many more lines
including `|ms` timer histograms that envoy-go lacks). Names are
**prefix-joined**: the receiver keys on the full `<prefix>.<name>` line.

**Counter subset** — on BOTH sides, for each of:

- `sdpfx.cluster.c_backend.upstream_rq_total`
- `sdpfx.http.hcm_local.downstream_rq_total`
- `sdpfx.http.hcm_local.downstream_rq_2xx`

the name is present (decode ran — a zero-datagram pass is structurally
impossible) and `delta-SUM == 7 == K`.

**Absolute gauge subset** (D-SD-GAUGE-SUBSET) — on BOTH sides:

- `sdpfx.cluster.c_backend.membership_total == 1`

## UNasserted

The whole datagram/line set; `|ms` timer lines (histogram side-channel,
reference-only); non-deterministic gauges (`server.uptime`, `*_active`,
connection churn); per-datagram framing + line order + datagram count per
flush; the proxy identifier (statsd has none); flush cadence.

## Debug

Set `FIXTURE_0092_DUMP=1` to print the per-side captured gauge + subset
delta-SUMs to stderr.
