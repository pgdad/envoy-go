# 0094-stats-sink-dogstatsd-batching

Cross-side EXACT differential for the phase 50 dog_statsd
`max_bytes_per_datagram` REAL multi-metric datagram **batching** (ADR-0267),
layered directly on top of the LANDED phase-49 dog_statsd sink WITH TAGS
(`0093-stats-sink-dogstatsd`). Proves that envoy-go (subject, in-process) and
reference Envoy (`contrib-v1.37.2`, Docker), both configured with a
`dog_statsd` `stats_sinks[]` entry carrying an **explicit**
`max_bytes_per_datagram: 200`, agree on:

- the SAME delta-SUM/tag-set/gauge-subset behavior `0093` already proves
  (cloned **verbatim** — unaffected by batching, since batching is a pure
  transport-layer concern applied strictly *after* a line string already
  exists), **plus**
- **at least one datagram observed on each side carried more than one line**
  (`MaxLinesInAnyDatagram() > 1`) — batching actually occurred, not just "the
  cap was configured and had no effect", **and**
- **a deliberately oversized line was always sent alone** in its own datagram
  (`LinesInDatagram(name) == (1, true)`) — no truncation, no drop, no
  special-cased branch.

## The batching design: a deliberately long `backendName` + a short `statPrefix`

`0094` reuses `0093`'s exact same three-counter + one-gauge subset shape, but
re-keys two constants to create a byte-length SPLIT under the configured cap:

- `statPrefix` stays **SHORT** (`"hcm_local"`, unchanged from `0093`) — the
  `envoy.http_conn_manager_prefix`-tagged lines stay small.
- `backendName` is **DELIBERATELY LONG** (a 172-character literal) — the
  `envoy.cluster_name`-tagged lines become guaranteed-oversized.

### The exact byte math (computed, not hand-estimated)

Computed via a throwaway `t.Logf(len(...))` against the REAL production
`formatTagSuffix`/`stats.ExtractTags` code (`internal/statssink/dogstatsd.go`)
at Task 6 authoring time, using a numeric value in the counters'/gauge's
observed range:

| wire line | rendered length |
|---|---|
| `dsdbpfx.cluster.upstream_rq_total:7\|c\|#envoy.cluster_name:<172-char backendName>` | **230 bytes** |
| `dsdbpfx.cluster.membership_total:1\|g\|#envoy.cluster_name:<172-char backendName>` | **229 bytes** |
| `dsdbpfx.http.downstream_rq_total:7\|c\|#envoy.http_conn_manager_prefix:hcm_local` | **78 bytes** |
| `dsdbpfx.http.downstream_rq_xx:7\|c\|#envoy.response_code_class:2,envoy.http_conn_manager_prefix:hcm_local` | **103 bytes** |

With `maxBytesPerDatagram = 200`:

- The two `envoy.cluster_name`-tagged lines (230/229 bytes) **each alone**
  exceed the cap, so they are **always** flushed alone — this holds
  regardless of registry walk order, because `appendLine` flushes any prior
  buffer content before accepting an oversized line into a fresh, empty
  buffer, and the very next line after an oversized one trivially exceeds the
  cap too (forcing another flush before it is appended).
- The two `envoy.http_conn_manager_prefix`-tagged lines (78/103 bytes) are
  comfortably under the cap — even together with the `"\n"` separator
  (`78+1+103 = 182 <= 200`) they fit in ONE datagram, and in practice co-batch
  with each other and the many other short envoy-go/reference self-stat lines
  that flush every interval.

The PLAN's `~160`-character estimate for `backendName` turned out to render as
**172** characters once the exact literal was counted; the resulting 230/229-
byte lines still clear the 200-byte cap with a comfortable margin (no
adjustment to `backendName`'s length was needed — see the driver's package doc
for the full derivation).

**Confirmed live**: the reference observed `MaxLinesInAnyDatagram() == 4`, the
subject observed `== 2` (both `> 1`), and `LinesInDatagram("dsdbpfx.cluster.
upstream_rq_total") == (1, true)` on **both** sides.

## Everything else is the `0093` design, unchanged by batching

The WIRE-name correction (`stats.ExtractTags`), the admin same-name collision
fix (`DeltaSumTagged`, not plain `DeltaSum`), the delta-SUM model + the
post-convergence stability barrier, the two-private-receivers + hard
`Close()` discipline, and the LOCAL `hostGatewayIP` duplicate (import-cycle
avoidance — `runner_test.go` blank-imports this driver from within package
`differential`) are ALL cloned **verbatim** from `0093-stats-sink-dogstatsd`.
See that fixture's `README.md`/`expectations.yaml` for the full design
rationale — none of it changes here, since batching is a pure
transport-layer concern applied strictly *after* a line string already
exists.

## Topology

Single-listener plaintext H1 → an `HTTPFixedBody` backend (the 172-char
`backendName`, 17-byte body), with a **bootstrap-level** `stats_sinks[]`
`dog_statsd` entry on BOTH sides:

- `@type: type.googleapis.com/envoy.config.metrics.v3.DogStatsdSink`
- `address.socket_address: { protocol: UDP, address: <host>, port_value: <port> }` — the driver-owned in-process `statsdrecv` UDP receiver.
- `prefix: dsdbpfx` — baked **identically** on both sides; DISTINCT from
  `0093`'s `dsdpfx` (a different fixture; must not collide if ever run
  against a shared receiver).
- `max_bytes_per_datagram: 200` — the NEW batching cap (ADR-0267), **honored**
  not strict-rejected.
- `stats_flush_interval: 0.5s` — a SHORT interval for fast deterministic
  convergence (vs the 5s default).

NO `node` field is needed — dog_statsd carries no proxy identifier.

## Workload + release barrier

Each side fires K=7 deterministic `GET /probe` requests (Host:
`dogstatsd.example`, UA: `dogstatsd-probe/1`; all 2xx), then POLLS that side's
private receiver until the deterministic COUNTER subset's per-flush deltas
**SUM** to `== 7` (`DeltaSumTagged`) — a release barrier, NEVER a
`time.Sleep`. After convergence, the driver waits `>= 2` further flushes (the
stability barrier) before snapshotting the delta-SUMs + tag sets + gauge +
gauge tags + the two batching-specific readings
(`MaxLinesInAnyDatagram()`/`LinesInDatagram`).

## Asserted (cross-side EXACT)

A name-SUBSET, NOT the whole line set (the reference emits many more lines
including `|ms` timer histograms that envoy-go lacks). Names are the
**post-extraction wire names**, prefix-joined.

**Counter subset** — on BOTH sides, for each of:

- `dsdbpfx.cluster.upstream_rq_total` — tags `{envoy.cluster_name: <backendName>}`
- `dsdbpfx.http.downstream_rq_total` — tags `{envoy.http_conn_manager_prefix: hcm_local}`
- `dsdbpfx.http.downstream_rq_xx` — tags `{envoy.response_code_class: "2", envoy.http_conn_manager_prefix: hcm_local}`

the name is present (decode ran), `delta-SUM == 7 == K`, and the last-seen tag
SET equals the expected set (order-independent `maps.Equal`).

**Absolute gauge subset** (D-DSD-GAUGE-SUBSET) — on BOTH sides:

- `dsdbpfx.cluster.membership_total == 1`, tags `{envoy.cluster_name: <backendName>}`

**Batching (NEW, phase 50, ADR-0267)** — on BOTH sides:

- `MaxLinesInAnyDatagram() > 1` — at least one multi-line datagram observed.
- `LinesInDatagram("dsdbpfx.cluster.upstream_rq_total") == (1, true)` — the
  deliberately oversized line stayed alone.

## UNasserted

The whole datagram/line set; `|ms` timer lines (histogram side-channel,
reference-only); non-deterministic gauges (`server.uptime`, `*_active`,
connection churn); per-datagram framing + line ORDER within a datagram; the
EXACT per-flush datagram COUNT or which specific OTHER lines co-batch with
which (only "at least one multi-line datagram" is asserted, not a precise
packing schedule); the literal tag ORDER on the wire (asserted as an
order-independent SET); the proxy identifier (dog_statsd has none); flush
cadence.

## Deliberate-break verification (Task 6)

Both breaks run with `-count=1` (`reference_differential_break_protocol_count1`)
and were reverted after confirming FAIL:

- **(a) force one-line-per-datagram regardless of cap** — `appendLine`
  unconditionally flushes then appends, skipping the size check entirely.
  FAILS with `MaxLinesInAnyDatagram() = 1, want > 1` — proves the
  batching-occurred assertion is live.
- **(b) silently drop an overflow line** — the overflow branch of `appendLine`
  flushes the buffer but never writes `line` into the fresh one. FAILS (the
  subset counter whose line lands on an overflow boundary undercounts, so the
  delta-SUM poll times out waiting for convergence) — proves the
  value-correctness assertion is live under the NEW batching code path
  specifically.

## Debug

Set `FIXTURE_0094_DUMP=1` to print the per-side captured gauge (+ tags),
subset delta-SUMs (+ tags), and the batching readings
(`MaxLinesInAnyDatagram()` + `LinesInDatagram` for the cluster-tagged name) to
stderr.
