# 0113-stats-sink-otlp-knobs

Cross-side differential for the phase 69 OpenTelemetry (OTLP) metrics stats sink
(`envoy.stat_sinks.open_telemetry`, ADR-0291) — the **KNOB arm**: the three knobs
turned ON together in ONE coherent config. Proves that envoy-go (subject,
in-process) and reference Envoy (`contrib-v1.37.2`, Docker) agree on a
deterministic **full-dotted COUNTER** subset emitted as **DELTA** `Sum`s whose
running sum stabilizes at `K`, with a `prefix` composed into every name and NO
attributes.

| knob | value | effect |
| --- | --- | --- |
| `report_counters_as_deltas` | `true` | DELTA temporality; per-flush counter **deltas** summing to `K` across flushes; `isMonotonic` **retained** |
| `prefix` | `envoytest` | every metric name composed `<prefix>.<base>` (a single dot inserted) |
| `use_tag_extracted_name` | `false` | names are the **FULL DOTTED** stat names, not the tag-extracted residual |
| `emit_tags_as_attributes` | `false` | **NO** `envoy.<tag>` attributes on any datapoint |

The two `*BoolValue` knobs are written as **bare scalars** (`false`, NOT
`{value: false}` — `reference_protojson_wrapper_scalar_not_object`).

## Topology

The 0112 chassis + the four knobs. Single-listener plaintext H1 → an
`HTTPFixedBody` backend (`c_backend`, 17-byte body), with a **bootstrap-level**
`stats_sinks[]` `open_telemetry` entry on BOTH sides:

- `typed_config: envoy.extensions.stat_sinks.open_telemetry.v3.SinkConfig` with
  `report_counters_as_deltas: true`, `prefix: envoytest`,
  `use_tag_extracted_name: false`, `emit_tags_as_attributes: false`.
- `grpc_service.envoy_grpc.cluster_name: c_otlp` — an **h2c** cluster
  (`http2_protocol_options: {}`, no TLS) pointing at the driver-owned in-process
  `test/helpers/otlpmetrics` receiver. `dns_lookup_family: V4_ONLY` on the
  reference side (P-16, the Docker Desktop AAAA gotcha).
- `stats_flush_interval: 0.1s` — a SHORT interval for fast deterministic
  convergence.

The HCM `stat_prefix` (`hcm_local`), the backend cluster name (`c_backend`), and
the metric `prefix` (`envoytest`) are FIXED identically on both sides so the
full-dotted prefixed names match cross-side. Port 10449.

## Two private per-side receivers + hard `Close()`

The sink flushes PERIODICALLY, so the reference keeps Exporting into its receiver
during the subject's drive window. The driver gives each side its OWN private
`otlpmetrics.Server` on a separately-allocated port
(`reference_periodic_sink_differential_two_receivers`) — no `Reset()` between
sides. After the subject snapshot BOTH receivers are hard-stopped via `Close()`.

## The delta-SUM model + the stability barrier

Under `report_counters_as_deltas:true` each COUNTER family carries the per-flush
**delta** (the increment since the previous flush), NOT the cumulative absolute
value; an idle counter's per-flush delta is 0. So the last-seen `value==K` test
(the 0112 cumulative shape) is meaningless here — instead a counter's per-flush
deltas **SUM** across flushes to `K`. The receiver's `DeltaSum(name)` running-sum
accessor accumulates exactly that.

The **stability barrier** (`reference_delta_sink_differential_stability_barrier`):
after the running `DeltaSum` reaches `K`, wait for **≥2 further idle flushes** and
re-read it — it must STILL be `K`. This distinguishes a true DELTA sink from an
absolute one: an absolute emitter re-sends the whole counter value every flush, so
its running sum **overshoots** `K` after the further flushes and the barrier fires.
Without it a first-flush delta is indistinguishable from an absolute value (see
Breaks O + P).

## Why the full-dotted names are unique (name-keyed `DeltaSum` is unambiguous)

With `use_tag_extracted_name:false` the sink emits the FULL DOTTED name, so the
application-backend counter is `envoytest.cluster.c_backend.upstream_rq_total` —
DISTINCT from the OTLP sink cluster's `envoytest.cluster.c_otlp.upstream_rq_total`
and the admin HCM's `envoytest.http.admin.downstream_rq_total`. Unlike 0112 (where
tag extraction collapsed every stat to a colliding RESIDUAL and forced
attribute-qualified selection), here the name alone selects the intended counter,
so `DeltaSum(name)` is unambiguous.

## Workload + asserted subset

K = 7 deterministic `GET /probe` requests/side (all 2xx). Poll each side's
receiver until the full-dotted counter subset's `DeltaSum` converges to `== 7` AND
≥1 Export has arrived (served-this-arm), then ≥2 further flushes (the barrier).

Asserted on BOTH sides (NAMED subsets only,
`reference_stats_sink_emits_used_only`):

| metric | temporality | running DeltaSum | monotonic | attributes |
| --- | --- | --- | --- | --- |
| `envoytest.cluster.c_backend.upstream_rq_total` | DELTA Sum | 7 (stable across ≥2 flushes) | true | none |
| `envoytest.http.hcm_local.downstream_rq_total` | DELTA Sum | 7 (stable) | true | none |
| `envoytest.http.hcm_local.downstream_rq_2xx` | DELTA Sum | 7 (stable) | true | none |
| `envoytest.cluster.c_backend.membership_total` | **Gauge** (absolute) | — | — | none |

Plus: the three `telemetry.sdk.*` resource KEYS present (per-side values
unasserted). The gauge proves `report_counters_as_deltas` applies to **counters
only** — a gauge stays an absolute `Gauge`, never converted to a delta `Sum`.

## NOT asserted

The whole family set/count (surfaces differ — envoy-go has no histograms); the
reference's `StartTimeUnixNano` shape (its µs factor-1000 bug — never cross-side
StartTime equality); the `c_otlp` sink cluster's own counters + `upstream_cx_*`
(dial-unaccounted + the feedback loop, `reference_cluster_sink_dial_unaccounted`);
gauge VALUES (non-deterministic; membership shape differs STATIC vs STRICT_DNS);
OTLP message framing + per-Export metric count + flush cadence
(`reference_streaming_sink_differential_framing`).

## Mixed combos are unit-tested, not fixtured

The mixed combinations (deltas + attributes-ON compose; one-true-one-false naming)
are `Submit`-level unit tests in the T2 matrix (`internal/statssink/otlp_test.go`).
This ONE fixture proves all three knobs together (deltas + prefix + both-false).

## Empirical prefixed-name confirmation

The prefixed full-dotted spellings above were confirmed by running the fixture
with `FIXTURE_0113_DUMP=1`, which logs every received datapoint (name + sorted
attributes + value + type + DeltaSum) on both sides.
