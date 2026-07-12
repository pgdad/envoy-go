# 0101-stats-sink-graphite

Cross-side EXACT differential for the phase 57 `graphite_statsd` sink
(ADR-0275). Proves that envoy-go (subject, in-process) and reference Envoy
(`contrib-v1.37.2`, Docker), both configured with a `graphite_statsd`
`stats_sinks[]` entry carrying an **explicit** `max_bytes_per_datagram: 200`,
agree on:

- the SAME delta-SUM/tag-set/gauge-subset behavior the phase-49 `0093-stats-
  sink-dogstatsd` fixture proves for `dog_statsd` — re-derived here over the
  graphite tag-in-name wire grammar instead of dog_statsd's trailing `|#`
  suffix, **plus**
- **at least one datagram observed on each side carried more than one line**
  (`MaxLinesInAnyDatagram() > 1`) — batching actually occurred (the
  `D-GR-BATCHSHARE` code hoisted verbatim from phase 50's `dog_statsd`
  batching, ADR-0267), **and**
- **a deliberately oversized line was always sent alone** in its own datagram
  (`LinesInDatagram(name) == (1, true)`) — no truncation, no drop, no
  special-cased branch, **and**
- **(NEW, phase 57) the subject's receiver accounted for every ingested line**
  (`UnparsedCount() == 0`, SUBJECT-ONLY) — a correct envoy-go graphite emitter
  produces only `|c`/`|g` lines that the shared receiver can always parse.

## D-GR-FIXTURE: one merged fixture, not two

Phase 57's SPEC decided (D-GR-FIXTURE) that this fixture folds together what
would otherwise have been a separate 0093-shape (tags) fixture and a separate
0094-shape (batching) fixture — both proofs run over the identical workload,
so there is no reason to duplicate the request-firing/polling machinery across
two directories. As a result, fixture numbering after `0100-http-tap-bodies`
skips straight from `0101-stats-sink-graphite` to whatever `0102` is next —
there is no separate `0101`-tags + `0102`-batching pair.

## The graphite tag-in-name grammar (the one novel piece of phase 57)

`graphite_statsd` folds tags **into the metric NAME** as `;envoy.k=v` pairs
preceding the value colon, CONTRAST `dog_statsd`'s trailing `|#k:v,...`
suffix:

```
<prefix>.<residual>[;envoy.k1=v1;envoy.k2=v2...]:<value>|<c|g>
```

A tag-free name carries no `;` at all. Keys use the dotted `envoy.<tag>` form;
values are emitted in `stats.ExtractTags`'s **natural (unsorted)** order — the
reference's two-tag order is the **reverse** of envoy-go's
(`AMEND-GR-TAGORDER`), a documented cross-side coverage boundary. This fixture
asserts tag **SETS** (order-independent `maps.Equal`), never literal order.

On the receive side (Task 7), `statsdrecv.ingestLine` additively parses the
`;k=v` pairs out of the name before the existing `'|'`/`':'` split logic runs,
keying `DeltaSumTagged`/`Tags()` on the **stripped** `<prefix>.<residual>`
name — the SAME lookup keys the `dog_statsd` fixtures use, so the subset names
below are unchanged in shape from `0093`/`0094`.

## The batching design: a deliberately long `backendName` + a short `statPrefix`

Re-derives `0094`'s exact technique over the graphite framing: `statPrefix`
stays **SHORT** (`"hcm_local"`) so the `envoy.http_conn_manager_prefix`-tagged
lines stay small; `backendName` is **DELIBERATELY LONG** (a 171-character
literal) so the `envoy.cluster_name`-tagged lines become guaranteed-oversized.

### The exact byte math (computed, not hand-estimated)

Computed via a throwaway `t.Logf(len(...))` against the REAL production
`graphiteTagSuffix`/`stats.ExtractTags` code
(`internal/statssink/graphite.go`) at Task 8 authoring time, using a numeric
value in the counters'/gauge's observed range:

| wire line | rendered length |
|---|---|
| `grpfx.cluster.upstream_rq_total;envoy.cluster_name=<171-char backendName>:7\|c` | **226 bytes** |
| `grpfx.cluster.membership_total;envoy.cluster_name=<171-char backendName>:1\|g` | **225 bytes** |
| `grpfx.http.downstream_rq_total;envoy.http_conn_manager_prefix=hcm_local:7\|c` | **75 bytes** |
| `grpfx.http.downstream_rq_xx;envoy.response_code_class=2;envoy.http_conn_manager_prefix=hcm_local:7\|c` | **100 bytes** |

With `maxBytesPerDatagram = 200`:

- The two `envoy.cluster_name`-tagged lines (226/225 bytes) **each alone**
  exceed the cap, so they are **always** flushed alone.
- The two `envoy.http_conn_manager_prefix`-tagged lines (75/100 bytes) are
  comfortably under the cap — even together with the `"\n"` separator
  (`75+1+100 = 176 <= 200`) they fit in ONE datagram, and in practice co-batch
  with each other and the many other short envoy-go/reference self-stat lines
  that flush every interval.

The PLAN's `~171`-character `backendName` estimate (mirroring `0094`'s
172-char precedent, minus graphite's slightly smaller tag-fold overhead
relative to dog_statsd's `|#` suffix) needed **no adjustment** — the computed
226/225-byte lines already clear the 200-byte cap with a comfortable margin.

## Everything else is the `0093`/`0094` design, unchanged

The WIRE-name correction (`stats.ExtractTags`), the admin same-name collision
fix (`DeltaSumTagged`, not plain `DeltaSum`), the delta-SUM model + the
post-convergence stability barrier, the two-private-receivers + hard
`Close()` discipline, the `D-GR-BATCHSHARE` shared
`appendBatchLine`/`flushBatch` batching algorithm, and the LOCAL
`hostGatewayIP` duplicate (import-cycle avoidance — `runner_test.go`
blank-imports this driver from within package `differential`) are ALL cloned
**verbatim** from `0094-stats-sink-dogstatsd-batching` (itself cloned from
`0093-stats-sink-dogstatsd`). See those fixtures' `README.md`/
`expectations.yaml` for the full design rationale.

## Topology

Single-listener plaintext H1 → an `HTTPFixedBody` backend (the 171-char
`backendName`, 17-byte body), with a **bootstrap-level** `stats_sinks[]`
`graphite_statsd` entry on BOTH sides:

- `@type: type.googleapis.com/envoy.extensions.stat_sinks.graphite_statsd.v3.GraphiteStatsdSink`
- `address.socket_address: { protocol: UDP, address: <host>, port_value: <port> }` — the driver-owned in-process `statsdrecv` UDP receiver.
- `prefix: grpfx` — baked **identically** on both sides; DISTINCT from
  `0092`/`0093`/`0094`/`0098`'s prefixes.
- `max_bytes_per_datagram: 200` — the batching cap (D-GR-CAP), **honored**
  not strict-rejected.
- `stats_flush_interval: 0.5s` — a SHORT interval for fast deterministic
  convergence (vs the 5s default).

NO `node` field is needed — `graphite_statsd` carries no proxy identifier.

## Workload + release barrier

Each side fires K=7 deterministic `GET /probe` requests (Host:
`graphite.example`, UA: `graphite-probe/1`; all 2xx), then POLLS that side's
private receiver until the deterministic COUNTER subset's per-flush deltas
**SUM** to `== 7` (`DeltaSumTagged`) — a release barrier, NEVER a
`time.Sleep`. After convergence, the driver waits `>= 2` further flushes (the
stability barrier) before snapshotting the delta-SUMs + tag sets + gauge +
gauge tags + the two batching-specific readings + the receiver's
`UnparsedCount()`.

## Asserted (cross-side EXACT, plus one subject-only proof)

A name-SUBSET, NOT the whole line set (the reference emits many more lines
including `|ms` timer histograms that envoy-go lacks). Names are the
**post-extraction, post-tag-strip wire names**, prefix-joined.

**Counter subset** — on BOTH sides, for each of:

- `grpfx.cluster.upstream_rq_total` — tags `{envoy.cluster_name: <backendName>}`
- `grpfx.http.downstream_rq_total` — tags `{envoy.http_conn_manager_prefix: hcm_local}`
- `grpfx.http.downstream_rq_xx` — tags `{envoy.response_code_class: "2", envoy.http_conn_manager_prefix: hcm_local}`

the name is present (decode ran), `delta-SUM == 7 == K`, and the last-seen tag
SET equals the expected set (order-independent `maps.Equal`).

**Absolute gauge subset** (D-DSD-GAUGE-SUBSET) — on BOTH sides:

- `grpfx.cluster.membership_total == 1`, tags `{envoy.cluster_name: <backendName>}`

**Batching** (`D-GR-BATCHSHARE`) — on BOTH sides:

- `MaxLinesInAnyDatagram() > 1` — at least one multi-line datagram observed.
- `LinesInDatagram("grpfx.cluster.upstream_rq_total") == (1, true)` — the
  deliberately oversized line stayed alone.

**SUBJECT-ONLY** (never cross-side, NEW phase 57):

- subject `UnparsedCount() == 0` — every line the subject's receiver ingested
  parsed cleanly (`reference_framing_break_needs_unparsed_counter`; SPEC-57
  §8.1). The reference legitimately produces unparsed lines (`|ms` timers),
  so its count is not asserted.

## UNasserted

The whole datagram/line set; `|ms` timer lines (histogram side-channel,
reference-only) and the reference's `UnparsedCount()`; non-deterministic
gauges (`server.uptime`, `*_active`, connection churn); per-datagram framing +
line ORDER within a datagram; the EXACT per-flush datagram COUNT or which
specific OTHER lines co-batch with which (only "at least one multi-line
datagram" is asserted, not a precise packing schedule); the literal tag ORDER
on the wire (asserted as an order-independent SET — the reference's order is
the REVERSE of envoy-go's, `AMEND-GR-TAGORDER`); `_NNN` exact-status-code
names (`AMEND-GR-EXACTCODE-SUBSET`, `_xx` class names only);
whole-registry-vs-used-only breadth; the proxy identifier (`graphite_statsd`
has none); flush cadence.

## Byte-math verification (Task 8)

Computed live via a throwaway test in `internal/statssink` (deleted before
commit) calling `stats.ExtractTags` + `graphiteTagSuffix` on the four subset
names with this fixture's exact constants:

```
cluster.upstream_rq_total      len=226  grpfx.cluster.upstream_rq_total;envoy.cluster_name=<171-char>:7|c
cluster.membership_total       len=225  grpfx.cluster.membership_total;envoy.cluster_name=<171-char>:1|g
http.downstream_rq_total       len=75   grpfx.http.downstream_rq_total;envoy.http_conn_manager_prefix=hcm_local:7|c
http.downstream_rq_2xx         len=100  grpfx.http.downstream_rq_xx;envoy.response_code_class=2;envoy.http_conn_manager_prefix=hcm_local:7|c
HCM co-batch total (with separator) = 176
backendName len = 171
```

Matches the PLAN-time estimate exactly (226/225/75/100, co-batch 176 ≤ 200) —
no adjustment to `backendName`'s length was needed.

**Confirmed live**: the reference observed `MaxLinesInAnyDatagram() == 6`, the
subject observed `== 3` (both `> 1`), and `LinesInDatagram("grpfx.cluster.
upstream_rq_total") == (1, true)` on **both** sides. The subject's
`UnparsedCount() == 0`; the reference's was `68` (legitimately non-zero — its
`|ms` timer lines — and, per design, not asserted).

## Debug

Set `FIXTURE_0101_DUMP=1` to print the per-side captured gauge (+ tags),
subset delta-SUMs (+ tags), the batching readings (`MaxLinesInAnyDatagram()` +
`LinesInDatagram` for the cluster-tagged name), and the per-side
`UnparsedCount()` to stderr.
