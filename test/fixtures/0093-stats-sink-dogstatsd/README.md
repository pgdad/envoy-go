# 0093-stats-sink-dogstatsd

Cross-side EXACT differential for the phase 49 dog_statsd UDP line-protocol
stats sink WITH TAGS. Proves that envoy-go (subject, in-process) and reference
Envoy (`contrib-v1.37.2`, Docker), both configured with a `dog_statsd`
`stats_sinks[]` entry, export each COUNTER family's **per-flush delta** as a
`|c` line (plus an extracted **tag suffix**) and each GAUGE's **absolute**
value as a `|g` line (plus tags), agreeing on a deterministic COUNTER
WIRE-NAME subset whose per-flush deltas **SUM** to `K` and whose extracted tag
**SET** matches, and on an absolute gauge `cluster.membership_total == 1`
tagged with `cluster_name`.

## The WIRE-name correction (the load-bearing diff vs 0092)

UNLIKE the plain statsd sink (`0092-stats-sink-statsd`), dog_statsd applies
`stats.ExtractTags` — the SAME SN1–SN9 + SN4 matcher the Prometheus /
`metrics_service` sinks use — BEFORE formatting the wire line. So the
residual + prefix join differs from 0092's raw dotted name for the "same"
underlying stat:

| dotted stat name | 0092 statsd wire name | 0093 dog_statsd wire name | hoisted tag |
|---|---|---|---|
| `cluster.c_backend.upstream_rq_total` | `sdpfx.cluster.c_backend.upstream_rq_total` | `dsdpfx.cluster.upstream_rq_total` | `envoy.cluster_name=c_backend` |
| `http.hcm_local.downstream_rq_total` | `sdpfx.http.hcm_local.downstream_rq_total` | `dsdpfx.http.downstream_rq_total` | `envoy.http_conn_manager_prefix=hcm_local` |
| `http.hcm_local.downstream_rq_2xx` | `sdpfx.http.hcm_local.downstream_rq_2xx` | `dsdpfx.http.downstream_rq_xx` | `envoy.response_code_class=2`, `envoy.http_conn_manager_prefix=hcm_local` |

The third row is the gotcha: **Rule SN4 rewrites the WIRE NAME** `_2xx` →
`_xx` and hoists the digit to a tag — it is not merely "the same name plus a
tag." A naive "same name as 0092, plus tags" assumption is WRONG; this was
caught at the phase-49 SPEC review and confirmed against a live probe:
`probepfx.http.downstream_rq_xx:...|c|#envoy.response_code_class:2,...`.

## The admin same-name collision (an empirical finding, not anticipated by the PLAN)

The bootstrap's **admin interface** has its own internal `http_conn_manager`
stats (`stat_prefix "admin"`), which — via the SAME SN2 hoist — collapse to
the SAME residual wire names as the test listener's
(`http.downstream_rq_total`, `http.downstream_rq_xx`), differing only in the
`envoy.http_conn_manager_prefix` tag VALUE (`admin` vs `hcm_local`). The
reference container's own admin readiness poll (`wait.ForHTTP("/ready")`,
`harness.go`) is itself one such `admin`-tagged 2xx request, so a plain
per-name `DeltaSum` converges to `K+1` (never `K`) on the reference side — this
was caught running the LIVE differential (it timed out waiting for
`downstream_rq_total`/`downstream_rq_xx` to reach `7`, observing `8` forever).
`0092` never hit this because its RAW (pre-extraction) dotted names
(`sdpfx.http.hcm_local.downstream_rq_total` vs
`sdpfx.http.admin.downstream_rq_total`) never collided.

Fixed with a new, additive `statsdrecv.Server` accessor,
`DeltaSumTagged(name, tags)` — a running sum keyed by BOTH the wire name AND
an EXACT tag-set match, alongside the existing name-only `DeltaSums` map. The
driver uses `DeltaSumTagged` (not `DeltaSum`) for the three counter subset
names; the admin-tagged line accumulates into a different bucket and the
test-listener bucket converges cleanly to `K`. This also makes the tag-drop /
tag-corruption break (b) even more robust: if production ever drops or
mis-tags the suffix, NO line matches the expected exact tag set, so the sum
for that bucket stays `0` forever (a clean, unambiguous FAIL) instead of only
being caught by a separate, potentially racy last-seen-tags comparison.

## Tag order: unsorted, but asserted order-independently

Tags are appended as `|#key1:val1,key2:val2` in the extractor's **natural**
(unsorted, SN4-prepended) order on BOTH sides — the reference does **not**
alphabetically sort dog_statsd tags (`reference_dogstatsd_tag_order_unsorted`).
The cross-side assertion is nonetheless an **order-independent map equality**
(`maps.Equal`) — the differential does not depend on the wire's tag order,
even though envoy-go's `formatTagSuffix` now happens to match the reference's
natural order.

## Delta-SUM model (dog_statsd `|c` counters)

Each dog_statsd flush emits `<prefix>.<residual>:<delta>|c[|#tags]` for every
COUNTER (the per-flush increment since the previous flush, NOT the cumulative
absolute value). A single-window burst makes the first flush's
`delta == absolute == K`, indistinguishable without post-convergence
observation. The **stability barrier** (`awaitFurtherFlushes`) observes
`>= 2` further flushes after the delta-SUM converges to `K`: under correct
deltas the idle counters emit `0` each flush so the SUM stays `K` (PASS); an
absolute sink re-adds the cumulative so the SUM overshoots `K` (FAIL). This is
what makes the deliberate `emit-absolute` / `skip-barrier` breaks catchable
(`reference_delta_sink_differential_stability_barrier`).

## Absolute gauge subset (D-DSD-GAUGE-SUBSET)

`cluster.membership_total` is emitted as an **absolute** `|g` line each flush
(the current gauge value, not a delta), tagged with `envoy.cluster_name`. The
driver asserts the captured gauge value `== 1` (one backend endpoint) AND its
tag set on BOTH sides. `membership_total` (not `membership_healthy`) is used
because this cluster carries no `health_checks`
(`reference_membership_total_vs_healthy_gauge`, the 0092 precedent).

## Topology

Single-listener plaintext H1 → an `HTTPFixedBody` backend (`c_backend`,
17-byte body), with a **bootstrap-level** `stats_sinks[]` `dog_statsd` entry on
BOTH sides:

- `@type: type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink`
- `address.socket_address: { protocol: UDP, address: <host>, port_value: <port> }` — the driver-owned in-process `statsdrecv` UDP receiver.
- `prefix: dsdpfx` — baked **identically** on both sides; DISTINCT from `0092`'s `sdpfx` (a different sink; coexistence not tested here, but the prefixes must not collide if both sinks were ever combined).
- `stats_flush_interval: 0.5s` — a SHORT interval for fast deterministic convergence (vs the 5s default).

The HCM `stat_prefix` (`hcm_local`) and the backend cluster name (`c_backend`)
are FIXED identically on both sides so the extracted tag VALUES match
cross-side.

NO `node` field is needed — dog_statsd carries no proxy identifier (contrast
`metrics_service` which carries the node in `StreamMetricsMessage.identifier`).

## Reference dog_statsd address: LITERAL `HostGatewayIP`

The reference Envoy container cannot use `host.docker.internal` as the
dog_statsd address because the dog_statsd UDP sink does NOT perform DNS
resolution — the statsd sink precedent (`AMEND-SD-REJECT`;
`reference_docker_probe_bridge_network`). The driver resolves the **LITERAL
IP** of the Docker host-gateway via a throwaway container (its own local
`hostGatewayIP`, copied verbatim from the `0092` driver rather than importing
`test/differential`'s exported `HostGatewayIP` — that would risk an import
cycle, since `runner_test.go` blank-imports this driver from within package
`differential`). This IP is baked into the reference bootstrap at
`ReferenceBootstrap` time. The receiver is bound on
`0.0.0.0:<refStatsdPort>` so the container can write datagrams to the host.
The subject bootstrap uses `127.0.0.1:<subjStatsdPort>` (loopback).

## Two private per-side receivers + hard `Close()`

The dog_statsd sink flushes **periodically** — the reference proxy keeps
sending to its receiver for the whole test, including during the subject's
drive window (`reference_periodic_sink_differential_two_receivers`). The
driver starts **two private UDP receivers** — one per side — on two
separately-allocated host ports, each bound on `0.0.0.0:<port>` BEFORE either
proxy starts. Each side owns an uncontaminated accumulator (no `Reset()`
needed). After the subject snapshot BOTH receivers are hard-stopped via
`Close()` — UDP is connectionless, so `Close` is unambiguous (no
`GracefulStop`-vs-hard-stop distinction).

## Workload + release barrier

Each side fires K=7 deterministic `GET /probe` requests (Host:
`dogstatsd.example`, UA: `dogstatsd-probe/1`; all 2xx), then POLLS that side's
private receiver until the deterministic COUNTER subset's per-flush deltas
**SUM** to `== 7` (`DeltaSum`) — a release barrier
(`reference_concurrency_differential_release_barrier`), NEVER a `time.Sleep`.

### Post-convergence stability barrier

After the delta-SUM converges to `K`, the driver waits until the receiver's
`SeenCount(marker) >= base + 2` (each flush increments `SeenCount` for the
marker name, even on a `0`-delta `|c` line — a flush-count signal). Only then
is the snapshot taken (subset delta-SUMs + tag sets + gauge + gauge tags).
This is the stability barrier that distinguishes correct delta emission from
broken absolute emission.

## Asserted (cross-side EXACT)

A name-SUBSET, NOT the whole line set (the reference emits many more lines
including `|ms` timer histograms that envoy-go lacks). Names are the
**post-extraction wire names**, prefix-joined: the receiver keys on the full
`<prefix>.<residual>` line.

**Counter subset** — on BOTH sides, for each of:

- `dsdpfx.cluster.upstream_rq_total` — tags `{envoy.cluster_name: c_backend}`
- `dsdpfx.http.downstream_rq_total` — tags `{envoy.http_conn_manager_prefix: hcm_local}`
- `dsdpfx.http.downstream_rq_xx` — tags `{envoy.response_code_class: "2", envoy.http_conn_manager_prefix: hcm_local}`

the name is present (decode ran — a zero-datagram pass is structurally
impossible), `delta-SUM == 7 == K`, and the last-seen tag SET equals the
expected set (order-independent `maps.Equal`).

**Absolute gauge subset** (D-DSD-GAUGE-SUBSET) — on BOTH sides:

- `dsdpfx.cluster.membership_total == 1`, tags `{envoy.cluster_name: c_backend}`

## UNasserted

The whole datagram/line set; `|ms` timer lines (histogram side-channel,
reference-only, envoy-go has no histograms); non-deterministic gauges
(`server.uptime`, `*_active`, connection churn); per-datagram framing + line
order + datagram count per flush; the literal tag ORDER on the wire (asserted
as an order-independent SET even though production now matches the
reference's natural order); the proxy identifier (dog_statsd has none); flush
cadence.

## Debug

Set `FIXTURE_0093_DUMP=1` to print the per-side captured gauge (+ tags) and
subset delta-SUMs (+ tags) to stderr.
