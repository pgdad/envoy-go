# Phase 55 SPEC — the plain-statsd `tcp_cluster_name` transport (`StatsdSink.statsd_specifier.tcp_cluster_name`)

> **Lifecycle stage:** SPEC (lifecycle-state 1 → 2). Docs-only, worktree `.worktrees/phase-55-spec`, branch `phase-55-stats-sink-statsd-tcp-spec`. Row 55 STAYS `in-progress`.
>
> **The BRAINSTORM's five settled questions (Q1..Q5) are the input.** The §11 empirical pins were EXECUTED IN-SESSION (2026-07-08) against `envoyproxy/envoy:contrib-v1.37.2` (ADR-0004/ADR-0227) on a Docker bridge network with a driver-owned byte-logging TCP receiver, per `reference_docker_probe_bridge_network`. **They CONFIRM four pins and AMEND three BRAINSTORM anticipations — one of which (AMEND-TCP-RESUME) refutes the stated PREMISE of the Q3 user decision and was re-decided by the human in-session.**

---

## 1. Purpose / Mission

Lift the phase-48 STRICT-REJECT at `internal/bootstrap/bootstrap.go:567-568` into a genuine accept-and-honor path: a new `TCPStatsdSink` that emits the UNCHANGED phase-48 statsd line protocol over a long-lived, newline-delimited TCP connection obtained from a named cluster. The EIGHTH Observability-family row; the family STAYS OPEN.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

Four pins CONFIRMED as anticipated; **three AMENDMENTS**, listed by consequence:

- **AMEND-TCP-RESUME (LOAD-BEARING; refutes the Q3 PREMISE).** The BRAINSTORM asserted, and told the human, that the reference "buffers into the conn and drops the buffer on connection failure, it does not replay." **The probe shows the exact opposite.** Across a 30-second receiver outage the reference ACCUMULATED every unwritten flush snapshot and delivered ALL of them, concatenated, on reconnect: 45 snapshots total, `sdpfx.server.uptime` running `5,6,…,32,…` with no gap, `sdpfx.http.hcm_local.downstream_rq_total` deltas `[0,4,0,0,0,0,0,0]` summing EXACTLY to the 4 requests driven during the outage. **Nothing lost. Nothing duplicated.** Q3's dichotomy (*replay double-counts* vs *drop loses*) was a FALSE DICHOTOMY: because `conn.Write` returns `n`, a third option exists — retain the unwritten suffix REALIGNED to the last complete line boundary `≤ n`. The human, presented with this evidence, re-decided Q3 → **bounded accumulate + line-aligned resume** (§3.5).
- **AMEND-TCP-CXSTATS (refutes the Q2 anticipation; ACTIVATES the pre-authorized Q2 option (c)).** The BRAINSTORM anticipated that, because the reference routes `TcpStatsdSink` through its own cluster manager, the stats cluster would accrue `upstream_cx_total`/`upstream_cx_active` exactly as envoy-go's `Cluster.Dial` does. **It does not.** The reference's stats cluster reports `upstream_cx_total: 0` and `upstream_cx_active: 0` while `upstream_cx_tx_bytes_total: 16511` — and with `circuit_breakers.thresholds[].max_connections: 0` on the stats cluster it STILL connects and STILL flushes (`circuit_breakers.default.cx_open: 0`, `upstream_cx_overflow: 0`). The statsd connection bypasses conn-pool accounting AND the `max_connections` permit. The BRAINSTORM pre-authorized this branch verbatim ("Defer unless D-TCP-CXSTATS shows the reference exempts it") — it does, so envoy-go gets an UNACCOUNTED dial (§3.2) rather than reusing `Cluster.Dial`.
- **AMEND-TCP-NODE (NEW; anticipated by nothing).** The reference REFUSES TO BOOT a `tcp_cluster_name` statsd sink unless BOTH `node.id` and `node.cluster` are set — either alone fails with the identical message. The UDP `address` sink boots with NO node at all (control probe). A new byte-stable reject arm (§3.7, §6).

Two further findings that shape the differential but change no envoy-go behavior:

- **AMEND-TCP-CONNCOUNT (strengthens Q4's rationale).** The reference opens **1** connection with zero traffic and **2** under load; connection #2 carries **only `|ms` timer lines** (35 of them; 6 distinct timer names). It is the worker-thread sink, connecting lazily on its first histogram emission. envoy-go has NO histograms (ADR-0060) and therefore can NEVER open it. So a cross-side `ConnCount` equality is infeasible **by the histogram boundary** — a structural reason, not the thread-model *uncertainty* Q4 hedged against. Q4's subject-exact assertion STANDS, on firmer ground.
- **AMEND-TCP-USEDONLY (NEW; differential-shaping).** The reference emits ONLY *used* stats: `cluster.c_statsd.upstream_cx_total` sits at `0` in the registry (`/stats`) yet appears in NO emitted line, whereas `cluster.c_statsd.membership_change:0|c` — a counter that WAS incremented once and now has delta 0 — IS emitted every flush. envoy-go's `snapshot()` walks the whole frozen registry and emits every registered stat. The emitted line SET therefore differs cross-side by construction. This does not change envoy-go (the `0092` fixture already asserts a NAMED SUBSET, never the whole set) but it forecloses any whole-set assertion in `0098` and explains why `upstream_cx_total` must be asserted SUBJECT-side only (§8.1).

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail stays **ADR-0271** at this SPEC (docs-only; no ADR body lands until IMPL per ADR-0044). §13 anchors the **ADR-0272 §Context DRAFT** (the SOLE anticipated ADR). Seven of the nine BRAINSTORM §10 D-questions are RESOLVED here (§11); **D-TCP-CLOSE** and **D-TCP-CONFIG-SHAPE** remain PLAN-level per the router, joined by two new PLAN questions the amendments raise (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §1.1 amendments)

- Timers / `|ms` lines. The reference emits 6 distinct timer names; envoy-go emits none (ADR-0060, the histogram boundary). UNCHANGED coverage boundary, now measured rather than assumed.
- Any change to the landed `StatsdSink` (UDP), `DogStatsdSink`, `udpWriter`, `Flusher`, `Sink`, `deltaState`, or `emitStatsdLines`.
- A `tcp_cluster_name` analog for `dog_statsd` — the `DogStatsdSink` oneof has ONLY the `address` member (`stats.pb.go:702-703`). Confirmed absent, not deferred.
- An UNBOUNDED accumulate buffer (the reference's actual behavior — §11.5 measured ≥45 snapshots / ~200 KB with no cap reached). envoy-go bounds it (§3.5); a DOCUMENTED, deliberate departure consistent with the tree's own backpressure posture (`MetricsServiceSink`'s cap-8 drop-newest channel).
- `graphite`/OTLP-metrics sinks; tracing extras; the tap filter.
- Restoring `upstream_cx_tx_bytes_total` (the ONE stats-cluster counter the reference does move). envoy-go does not register that stat NAME for any cluster (verified: no `"upstream_cx_tx_bytes_total"` anywhere in `internal/`), so there is nothing to diverge from and nothing to add. Out of scope.

---

## 3. The TCP statsd transport (ADR-0272)

### 3.0 Split disposition — ADR-0045 re-check: SINGLE FLAT ROW (escape valve stays available)

The amendments GREW the envelope from the BRAINSTORM's ~180–230 prod LoC to **~250–330** (`statsd_tcp.go` ~200; the unaccounted cluster dial ~40; bootstrap ~40; `main.go` ~20). Still ONE subsystem, ONE new file, ONE ADR, ONE fixture. **NO SPLIT.** The pre-authorized 55.1/55.2 escape valve stays UNCONSUMED but is explicitly re-armed: **if the PLAN's decomposition exceeds ~14 tasks, revisit.**

### 3.1 Writer shape — bounded channel + writer goroutine *(Q1, unchanged)*

`Submit` non-blocking-sends onto a cap-`defaultChannelCapacity` (8) channel; on a full channel the batch is dropped and a rate-limited diagnostic emitted (the `sink.go` `lastDropLog` idiom). A single writer goroutine owns the dial, the `pending` buffer, and the write. The `Flusher` NEVER blocks. This is the `MetricsServiceSink`/ADR-0262 shape; phase 48's SYNCHRONOUS `Submit` was licensed by UDP's never-blocks property (`SPEC-48.md:78`), which TCP lacks.

`delta.apply` runs **in the writer**, not in `Submit` (the ADR-0263 rule) — so a channel-full enqueue drop never latches `deltaState`; the dropped increments ride the next enqueued flush.

### 3.2 The dial — UNACCOUNTED, not `Cluster.Dial` *(AMEND-TCP-CXSTATS; Q2 option (c), pre-authorized)*

`Cluster.Dial` (`cluster.go:536`) does five things: LB pick (holding the release until conn Close), `acquireConnSlot` (the `max_connections` permit, ADR-0252), TCP dial under `connect_timeout`, TLS handshake, then `upstreamCxTotal.Inc()` + `upstreamCxActive.Inc()` and a `connWithGauge` wrap. **The reference's stats connection does the pick, the timeout, and the TLS — and NONE of the accounting.**

envoy-go therefore gains a NARROW, exported, stats-sink-scoped dial on `*Cluster` (working name `DialSink`; the exact name is a PLAN question, D-TCP-DIALNAME):

```go
// DialSink dials one endpoint of c for a stats sink. Unlike Dial it takes NO
// max_connections permit and increments NO upstream_cx_* counters — mirroring
// the reference, whose statsd TCP connection bypasses conn-pool accounting
// (SPEC §11.4). It DOES honor the LB pick, connect_timeout, and upstream TLS.
func (c *Cluster) DialSink(ctx context.Context) (net.Conn, error)
```

Implementation shape (PLAN pins the details): `c.PickEndpoint()` (which releases the pick immediately — its documented contract, `cluster.go:498-506`; least_request load-invisibility is acceptable and already accepted for every `PickEndpoint` caller) → TCP dial to `ep.Addr()` bounded by `c.ConnectTimeout()` → TLS handshake bounded by `ctx` when `c.UpstreamTLSConfig() != nil` → return the BARE `net.Conn` (no `connWithGauge` wrap; there is no gauge to Dec and no permit to release).

**`internal/statssink` does NOT import `internal/cluster`.** `NewTCPStatsdSink` takes a `func(context.Context) (net.Conn, error)` seam; `cmd/envoy-go/main.go` closes over the manager and re-looks-up the cluster per dial, exactly as `grpcclient.go:137` does:

```go
statssink.NewTCPStatsdSink(func(ctx context.Context) (net.Conn, error) {
    cl, ok := cm.Get(name)
    if !ok { return nil, fmt.Errorf("unknown cluster %q", name) }
    return cl.DialSink(ctx)
}, cfg.Prefix)
```

A `func` and not a one-method interface: the seam has one method and no shared state.

### 3.3 Framing — `\n`-TERMINATED, one write per flush *(D-TCP-LINE, CONFIRMED)*

Every emitted line, **including the last of a flush**, is `\n`-TERMINATED (not `\n`-separated): every captured write ends with `0x0A` and no write contains `\n\n`. A steady-state periodic flush is **ONE** `conn.Write` of the whole snapshot (99 lines / ~4650 bytes in the probe). Per `reference_wire_format_both_sides_see_same_bytes` envoy-go adopts this verbatim:

```
<prefix>.<dotted-name>:<value>|c\n     COUNTER (per-flush delta)
<prefix>.<dotted-name>:<value>|g\n     GAUGE   (absolute)
```

Reuse `emitStatsdLines` (`udp.go:58`) unchanged, with the StatsdSink `nameAndTags` closure (`prefix + "." + name`, empty tag suffix) and an `emit` that appends `line + "\n"` to `pending`.

**Write granularity is NOT observable to a line-parsing receiver over a stream** and is therefore NOT asserted; the byte SEQUENCE is.

### 3.4 Delta semantics — UNCHANGED from the UDP sink *(D-TCP-DELTA, CONFIRMED)*

COUNTER families carry the per-flush DELTA with `|c`; GAUGE families the ABSOLUTE value with `|g`. Probed: `downstream_rq_total` deltas `[2,5,0,0,0,0]` over 7 requests (SUM == 7); `membership_total` and `server.live` hold constant at their absolute values across all flushes.

**A counter with a ZERO delta is still emitted** (`sdpfx.cluster.c_statsd.membership_change:0|c` appears in every flush). envoy-go's `deltaState.apply` already emits every COUNTER family every flush, delta 0 included. No change. (Contrast AMEND-TCP-USEDONLY: the reference omits *never-incremented* counters entirely — a set difference, not a value difference.)

The landed `delta.go` is reused **verbatim**, a sink-private always-on `deltaState`. No knob.

### 3.5 Error policy — bounded accumulate + line-aligned resume *(AMEND-TCP-RESUME; Q3 RE-DECIDED)*

The writer owns a `pending []byte` of UNWRITTEN bytes, accumulated ACROSS flushes:

```go
func (s *TCPStatsdSink) flush(batch []*dto.MetricFamily) {
    emitStatsdLines(s.delta.apply(batch), s.nameAndTags, func(line string) {
        s.pending = append(append(s.pending, line...), '\n')
    })
    if len(s.pending) > maxPendingBytes {
        s.pending = dropOldestLines(s.pending, maxPendingBytes) // + rate-limited log
    }
    if s.conn == nil && !s.redial() {
        return // dial failed: pending RETAINED, retried next flush
    }
    n, err := s.conn.Write(s.pending)
    if err == nil {
        s.pending = s.pending[:0]
        return
    }
    s.conn.Close()
    s.conn = nil
    // Resume at a LINE boundary: complete lines that landed are never re-sent;
    // the partially-written line is re-sent WHOLE on the next connection.
    s.pending = s.pending[bytes.LastIndexByte(s.pending[:n], '\n')+1:]
    s.logDrop(err)
}
```

**Why the line-boundary realignment is exactly right, and not merely conservative.** A `Write` that returns `0 < n < len(pending)` with a non-nil error means bytes `[0,n)` may have reached the peer. Those bytes end mid-line. The dead connection's receiver, parsing a stream, MUST discard its incomplete trailing line at EOF (§3.9 — the same discipline `statsdrecv`'s TCP listener must implement). So:

- complete lines within `[0,n)` were delivered exactly once and are NOT re-sent → **no duplication**;
- the straddling line was discarded by the old receiver and is re-sent whole on the fresh connection → **no loss**.

`bytes.LastIndexByte(pending[:n], '\n')+1` is precisely the start of that straddling line (and equals `n` when `n` already lands on a boundary, correctly retaining nothing). Go's blocking `net.Conn.Write` loops internally, so `err == nil` implies `n == len(pending)`; a non-nil error is the ONLY partial-write path.

**Bounded, unlike the reference.** `maxPendingBytes` caps the buffer; on overflow the OLDEST whole lines are dropped, with a rate-limited log. The reference is effectively unbounded (§11.5: ≥45 snapshots / ~200 KB retained with no cap reached). This is a DELIBERATE, DOCUMENTED departure — an unbounded in-process buffer is a memory-growth hazard the tree already refuses elsewhere (`MetricsServiceSink`'s cap-8 drop-newest channel). The exact value of `maxPendingBytes` is a PLAN question (D-TCP-PENDING-CAP).

### 3.6 `Close` — drain, then HARD-close to unwedge *(D-TCP-CLOSE — PLAN)*

Mirrors `MetricsServiceSink.Close` (`sink.go:135-148`): `close(ch)` → wait `done` up to `closeDrainGrace` → force → `<-done`. ONE substantive difference: `cancel()` cannot interrupt a blocked `net.Conn.Write` (a gRPC stream is context-bound; a raw conn is not). The unwedge is `conn.Close()`. Combined with a per-write `SetWriteDeadline` this is belt-and-braces. **`conn` is writer-goroutine-owned, so `Close` reaching it is a genuine `-race` hazard** — the mechanism (an `atomic.Pointer[net.Conn]`, a ctx-watching closer, or relying solely on the deadline) is a PLAN pin and must be `-race`-PROVEN, not argued.

### 3.7 The node requirement *(AMEND-TCP-NODE; ADR-0080)*

The reference rejects at boot:

```
tcp statsd: node 'id' and 'cluster' are required. Set it either in 'node' config
or via --service-node and --service-cluster options.
```

Probed: `node.id` alone → reject; `node.cluster` alone → reject; both → boots. The UDP `address` sink with NO node → **boots** (control). So the requirement is TCP-SPECIFIC.

envoy-go mirrors it with a byte-stable reject. `bs.Proto.GetNode().GetId()` / `.GetCluster()` are already read at `main.go:155` (`minNode`), so the data is in hand. Whether the check lives in `parseStatsdSinkConfig` (via `result.Proto.GetNode()`) or at the `main.go` sink-build site is a PLAN question (D-TCP-NODE-PLACEMENT); parse-time is preferred so the existing `FuzzStatsdSinkConfigParse` covers it.

Neither `node.id` nor `node.cluster` appears anywhere in the emitted line stream — the requirement is a validation, not a naming input. (Verified: the probe's `prefix: sdpfx` alone drives every line name.)

### 3.8 The reject roster after the lift — the oneof CLOSES

`StatsdSink.statsd_specifier` has EXACTLY TWO arms (`stats.pb.go:571-577, 675-691`), so lifting `tcp_cluster_name` **closes** the oneof; there is no third sibling left, discharging `reference_strict_reject_sibling_typeurl_gap` at the oneof level. (The independent TypeURL-level `parseStatsSinks` dispatch is unchanged and still rejects unknown sink TypeURLs.) See §6 for the full roster.

**The ordering constraint survives the lift with INVERTED meaning.** `bootstrap.go:556-561` records that `GetAddress()` returns nil for BOTH a missing oneof AND a `tcp_cluster_name` arm, which is why the reject ran FIRST. After the lift, the `tcp_cluster_name` arm must be **dispatched** before the nil-`address` reject fires, or a valid TCP config is rejected as "missing `statsd_specifier`". A regression test MUST pin this.

### 3.9 The receiver is a DATAGRAM parser and must become a STREAM parser *(`reference_line_parser_extension_delimiter_reuse`)*

`statsdrecv.ingest` (`statsdrecv.go:131`) splits a UDP **datagram** on `\n`; every line is complete BY CONSTRUCTION. **A TCP read carries no such guarantee — this is measured, not hypothetical:** the probe's first post-reconnect write was ~200 KB and arrived at the receiver in `recv()` chunks capped at 65536 bytes, splitting lines mid-token. The TCP listener must read line-at-a-time through `bufio` with a carried remainder and DISCARD an incomplete trailing line at EOF (which §3.5's correctness argument depends on). The PLAN task must trace one concrete split-read example byte-for-byte.

`MaxLinesInAnyDatagram` / `LinesInDatagram` are datagram concepts; the TCP path leaves them unpopulated. Documented, not silently divergent.

---

## 4. Framework primitives — 1 new cluster method, 0 new seams, 0 new packages, 0 new go.mod deps

| Primitive | Disposition |
|---|---|
| `Flusher` / `Sink` seam | REUSED unchanged |
| `emitStatsdLines`, `deltaState` | REUSED verbatim |
| `StatsdSink` (UDP), `DogStatsdSink`, `udpWriter` | UNTOUCHED |
| `Cluster.DialSink` | **NEW** (§3.2) — the only production surface added outside `statssink` |
| `internal/statssink/statsd_tcp.go` | **NEW file**, existing package |
| `test/helpers/statsdrecv` | EXTENDED (TCP listener + `ConnCount()`), not replaced |
| new Go package / go.mod module | **NONE** (`go mod tidy -diff` anticipated EMPTY) |

---

## 5. Proto-field roster

| Field | Disposition |
|---|---|
| `StatsdSink.statsd_specifier.tcp_cluster_name` | **CONSUMED** (this row) |
| `StatsdSink.statsd_specifier.address` | consumed at phase 48, UNCHANGED |
| `StatsdSink.prefix` | consumed at phase 48, UNCHANGED (default `envoy`) |
| `Bootstrap.node.id` / `.cluster` | **READ** as a TCP-sink boot precondition (§3.7) |

---

## 6. PARSE-REJECT roster (ADR-0080)

| Condition | Before | After | Parity? |
|---|---|---|---|
| `tcp_cluster_name` set | REJECT (`bootstrap.go:567-568`) | **ACCEPT** | — (the lift) |
| `statsd_specifier` unset (both arms) | REJECT | REJECT (unchanged) | **PARITY** — reference: `Proto constraint validation failed (field: "statsd_specifier", reason: is required)` |
| `tcp_cluster_name` names an unknown cluster | n/a | **REJECT (boot fatal)** | **PARITY** — reference: `tcp statsd: unknown cluster 'c_nonexistent'` |
| `tcp_cluster_name` set + `node.id` or `node.cluster` missing | n/a | **REJECT (NEW)** | **PARITY** — §3.7 |
| `tcp_cluster_name` names an existing but UNREACHABLE cluster | n/a | ACCEPT (boot OK; sink retries) | **PARITY** — probed |
| UDP `address` + no node | ACCEPT | ACCEPT (unchanged) | **PARITY** — control probe |
| unknown sink TypeURL | REJECT | REJECT (unchanged) | — |

The unknown-cluster reject is reference-PARITY, **not** an envoy-go-strict departure — correcting the BRAINSTORM §5.2's "whether this is a DEPARTURE or reference-parity is D-TCP-REJECT."

---

## 7. Stat surface — CONFIRMED ZERO delta

**1200 → 1200 (+0).** The statsd sink registers ZERO self-stats in the reference (probed: no `sink`- or `statsd`-scoped stat name in any emitted line, `/stats` confirms). And per AMEND-TCP-CXSTATS, envoy-go's `DialSink` increments NOTHING — so unlike the BRAINSTORM's §2.12 reasoning (which argued the `upstream_cx_*` self-reference was "a new stat INSTANCE, not a new NAME"), there is now **no self-reference at all**. The stronger statement holds: the stats cluster's `upstream_cx_total`/`upstream_cx_active` stay at **0** on both sides.

---

## 8. Differential fixture taxonomy (+1: `0098-stats-sink-statsd-tcp`)

### 8.1 `0098-stats-sink-statsd-tcp` (cross-side payload parity + subject-exact transport assertions)

Clones the `0092-stats-sink-statsd` shape. K = 7 deterministic `GET /probe` requests per side.

- **Receivers:** TWO driver-owned per-side TCP receivers (`test/helpers/statsdrecv`, extended), bound before either proxy starts, each with its own accumulator, both hard-`Close()`d after the subject snapshot — never `GracefulStop` (`reference_periodic_sink_differential_two_receivers`).
- **Reachability:** the reference's `c_statsd` cluster endpoint is the host-gateway literal IP (`differential.HostGatewayIP`, `reference_host_gateway_ip_docker_desktop`); the subject's is `127.0.0.1`. The address now lives in `clusters[].load_assignment`, not `stats_sinks[].address`.
- **Node:** both bootstraps MUST set `node: {id, cluster}` (§3.7), else the reference will not boot.
- **BackendCount:** `c_backend` satisfies `≥ 1` (`reference_differential_backendcount_min_one`).

**CROSS-SIDE assertions** (both snapshots):
- `sdpfx.cluster.c_backend.upstream_rq_total`, `sdpfx.http.hcm_local.downstream_rq_total`, `sdpfx.http.hcm_local.downstream_rq_2xx`: present, `|c` delta-SUM `== 7`, and STILL `== 7` after a `≥2`-further-flush stability barrier (`reference_delta_sink_differential_stability_barrier` — a burst-all-requests delta sink cannot otherwise tell a delta from an absolute).
- `sdpfx.cluster.c_backend.membership_total == 1` (absolute gauge; `reference_membership_total_vs_healthy_gauge`).

**SUBJECT-EXACT assertions** (reference RECORDED, not asserted):
- `subjRecv.ConnCount() == 1` — one long-lived connection; no per-flush redial. Reference is **2** (main + worker-timer sink); cross-side equality is infeasible by the histogram boundary (AMEND-TCP-CONNCOUNT), not by uncertainty.
- `sdpfx.cluster.c_statsd.upstream_cx_total == 0` — proves `DialSink` takes the UNACCOUNTED path (AMEND-TCP-CXSTATS). Subject-only because the reference never emits this line at all (AMEND-TCP-USEDONLY: unused counters are omitted).

**UNASSERTED, both sides:** the whole line set (AMEND-TCP-USEDONLY makes it structurally different); `|ms` timers; non-deterministic gauges; flush cadence; write granularity.

### 8.2 Deliberate breaks (4; each `-count=1` per `reference_differential_break_protocol_count1`)

- **(a)** Redial on every flush (drop the conn cache) → subject `ConnCount() == 1` fails.
- **(b)** Emit `\n`-SEPARATED instead of `\n`-TERMINATED (drop the final terminator) → the last line of each flush concatenates with the first of the next; the counter-subset lookups miss.
- **(c)** Emit ABSOLUTE counter values instead of deltas → the `≥2`-flush stability barrier fails. **This break PASSES without the barrier** — the PLAN must additionally verify that removing the barrier masks it.
- **(d)** Use `Cluster.Dial` instead of `DialSink` → subject `cluster.c_statsd.upstream_cx_total` becomes `1`; the subject-exact assertion fails. Proves AMEND-TCP-CXSTATS is live.

The line-aligned resume (§3.5) is **UNIT-proven**, not differentially proven: the `0098` receiver never dies. A unit test drives a fake dialer whose conn accepts `n` bytes then errors mid-line, and asserts (i) the straddling line is re-sent whole on the next conn, (ii) complete lines already written are NOT re-sent, (iii) the delivered line multiset across both conns equals the emitted multiset exactly.

### 8.3 NO new BackendKind (driver-owned receiver; stays 38) + NO new fuzzer (stays 52)

The landed `FuzzStatsdSinkConfigParse` (`statsd_fuzz_test.go:13`) already drives this message and already carries a `tcp_cluster_name` seed (`:47-52`, labelled `// tcp_cluster_name (reject)`); this row flips that seed's expected outcome and adds a node-missing seed. No new dispatch arm ⇒ no new fuzzer. Reconcile the documented count against `grep -c '^func Fuzz'` per `reference_fuzzer_count_docs_drift`.

### 8.4 Total

fixtures **99 → 100**.

---

## 9. Behavior-contract delta (the phase-55 bundle; ADR-0052 atomic landing)

`BEHAVIOR_CONTRACT.md` gains: the statsd TCP transport (framing, delta semantics, one long-lived conn); the node precondition; the unaccounted-dial semantics (no `upstream_cx_*`, no `max_connections` permit) and its consequence that a `max_connections`-capped stats cluster does NOT throttle the sink; the bounded-`pending` departure from the reference's unbounded accumulate; the `|ms` and used-only line-set coverage boundaries.

---

## 10. Per-task structure (~12–14 tasks; the PLAN decomposes)

1. `Cluster.DialSink` + unit tests (unaccounted: no permit, no `upstream_cx_*`; honors TLS + `connect_timeout`).
2. `statsdrecv` TCP listener (stream parser, carried remainder, discard incomplete trailing line at EOF) + `ConnCount()` + unit tests incl. a forced mid-line split-read.
3. Bootstrap: lift the `tcp_cluster_name` reject; dispatch the oneof arm BEFORE the nil-`address` reject (§3.8 regression test).
4. Bootstrap: the node-required reject (§3.7) + fuzz seeds.
5. `TCPStatsdSink` skeleton: struct, `func` dial seam, bounded channel, writer goroutine, `Submit`, delta-in-writer.
6. Serialization into `pending` via `emitStatsdLines` (`\n`-TERMINATED).
7. Write path + line-aligned resume (§3.5) + unit tests (the mid-line partial-write case).
8. `maxPendingBytes` cap + `dropOldestLines` + rate-limited log.
9. `Close`: drain/grace/unwedge; the `-race`-proven `conn` handoff (D-TCP-CLOSE).
10. `main.go` build arm + unknown-cluster boot fatal.
11. Fixture `0098` bootstraps (both sides, with `node`) + driver.
12. The 4 deliberate breaks, controller-reperformed.
13. ADR-0272 body + `BEHAVIOR_CONTRACT.md` delta.
14. Six-gate completion bundle (full-package `-race`, §11.8).

---

## 11. SPEC-time empirical-pin block — executed IN-SESSION 2026-07-08 against `envoyproxy/envoy:contrib-v1.37.2`, `--concurrency 1`, Docker bridge network, driver-owned byte-logging TCP receiver

### Summary disposition table (9 pins)

| Pin | Disposition |
|---|---|
| D-TCP-LINE | **CONFIRMED** — `\n`-TERMINATED incl. last; one write per periodic flush |
| D-TCP-DELTA | **CONFIRMED** — `\|c` per-flush delta, `\|g` absolute; zero-delta counters still emitted |
| D-TCP-CONNCOUNT | **AMENDED** — 1 (no traffic) / 2 (under load); conn #2 is `\|ms`-only |
| D-TCP-CXSTATS | **AMENDED** — NO `upstream_cx_total`/`active`, NO `max_connections` permit |
| D-TCP-RECONNECT | **AMENDED** — accumulate-and-deliver, NOT drop; nothing lost or duplicated |
| D-TCP-REJECT | **CONFIRMED + EXTENDED** — unknown cluster rejects (parity); **node.id/cluster required (NEW)** |
| D-TCP-STATS | **CONFIRMED** — zero sink self-stats |
| D-TCP-CLOSE | **DEFERRED to PLAN** (per router) |
| D-TCP-CONFIG-SHAPE | **DEFERRED to PLAN** (per router) |

### 11.1 D-TCP-LINE — CONFIRMED

Every captured write ends `0x0A`; no write contains `\n\n`. Steady-state periodic flush = ONE write, 99 lines, ~4650 bytes. First write of a fresh conn = `sdpfx.server.initialization_time_ms:6|ms\n` (41 bytes, `\n`-terminated).

### 11.2 D-TCP-DELTA — CONFIRMED

`sdpfx.http.hcm_local.downstream_rq_total` deltas across flushes: `2, 5, 0, 0, 0, 0` over K=7 requests → SUM = 7. Same shape for `downstream_rq_5xx` and `upstream_cx_connect_fail`. Gauges (`membership_total`, `server.live`) hold constant. `membership_change:0|c` proves zero-delta counters are still emitted.

### 11.3 D-TCP-CONNCOUNT — AMENDED

0 requests over 7 s → **1 ACCEPT**. 7 requests → **2 ACCEPTs**. Suffix census: conn #1 `{ms: 1, c: 220, g: 268}`, conn #2 `{ms: 35}`. Conn #2 carries only the 5 worker-thread timer names. envoy-go emits no histograms ⇒ 1 conn always.

### 11.4 D-TCP-CXSTATS — AMENDED (LOAD-BEARING)

`/stats?filter=cluster.c_statsd`: `upstream_cx_total: 0`, `upstream_cx_active: 0`, `upstream_cx_tx_bytes_total: 16511`. With `max_connections: 0` on the stats cluster the reference STILL connects and flushes; `circuit_breakers.default.cx_open: 0`, `upstream_cx_overflow: 0`. Contrast `c_backend` (a data-plane cluster), whose `upstream_cx_connect_fail: 3` moves normally. envoy-go registers no `upstream_cx_tx_bytes_total` at all.

### 11.5 D-TCP-RECONNECT — AMENDED (LOAD-BEARING; refutes the Q3 premise)

Short outage (3 requests before, receiver killed, 4 requests during, receiver restarted): the reference redialed once and delivered ONE 37044-byte write containing **8 concatenated flush snapshots** (`server.uptime` = 6,7,…,13), with `downstream_rq_total` deltas `[0,4,0,0,0,0,0,0]` — SUM = 4, exactly the outage-era requests. Cross-receiver total = 7 = K. Long outage (30 s): **45 snapshots** delivered, earliest retained `uptime=5`, no gap, no cap reached; the first post-reconnect write exceeded the receiver's 65536-byte `recv()` buffer and was chunked mid-line.

⇒ The reference ACCUMULATES unwritten snapshots and delivers all of them. Nothing lost; nothing duplicated. Q3 re-decided by the human → §3.5.

### 11.6 D-TCP-REJECT — CONFIRMED + EXTENDED

| Arm | Reference behavior |
|---|---|
| `tcp_cluster_name: c_nonexistent` | boot REJECT: `tcp statsd: unknown cluster 'c_nonexistent'` |
| `statsd_specifier` absent | boot REJECT: `Proto constraint validation failed (field: "statsd_specifier", reason: is required)` |
| existing but unreachable cluster | **BOOTS**; sink retries |
| TCP sink, `node.id` only | boot REJECT: `tcp statsd: node 'id' and 'cluster' are required.…` |
| TCP sink, `node.cluster` only | boot REJECT (identical message) |
| TCP sink, both node fields | BOOTS |
| **UDP sink, no node (control)** | **BOOTS** ⇒ the node requirement is TCP-specific |

Incidental: a UDP `address` with a HOSTNAME is rejected (`malformed IP address: recv55`) — the statsd UDP `socket_address` must be an IP literal, matching envoy-go's existing `StatsdSinkConfig.UDPAddress` doc-comment. No change.

### 11.7 D-TCP-STATS — CONFIRMED ZERO

No `sink`- or `statsd`-scoped stat name appears in any emitted line or in `/stats`. Stat surface stays 1200.

### 11.8 The `-race` obligation

`TCPStatsdSink` is the statsd sinks' FIRST background mutator. Per `reference_full_suite_race_after_background_mutator`, a `-run`-subset `-race` will MISS the class of failure it introduces. The merge gate runs a FULL-package `-race` on `internal/statssink`, `internal/cluster`, and `test/differential`.

---

## 12. PLAN / IMPL D-questions (not empirical pins)

- **D-TCP-CLOSE (LOAD-BEARING).** The race-free mechanism by which `Close` reaches the writer-goroutine-owned `conn` to unwedge a blocked `Write`. `cancel()` cannot interrupt a raw `net.Conn`. Must be `-race`-PROVEN, not argued.
- **D-TCP-CONFIG-SHAPE.** A tagged-union `TCPClusterName` field on `StatsdSinkConfig` vs. a separate `Bootstrap.StatsdTCPSinkConfigs` slice.
- **D-TCP-PENDING-CAP (NEW).** The value of `maxPendingBytes`, and whether `dropOldestLines` drops whole LINES or whole SNAPSHOTS. (The reference is unbounded, so there is no value to mirror — this is an envoy-go choice, §3.5.)
- **D-TCP-NODE-PLACEMENT (NEW).** Whether the node-required reject lives in `parseStatsdSinkConfig` (via `result.Proto.GetNode()`, so `FuzzStatsdSinkConfigParse` covers it — PREFERRED) or at the `main.go` sink-build site.
- **D-TCP-DIALNAME (NEW).** The exported name and exact signature of the unaccounted cluster dial (`DialSink`? `DialUnaccounted`?), and whether it shares a private helper with `Dial`.

---

## 13. ADR continuity — the ADR-0272 §Context DRAFT (anchored here; the full entry lands at the IMPL per ADR-0044)

> **ADR-0272 — the plain-statsd `tcp_cluster_name` transport (§Context DRAFT).**
>
> Phase 48 (ADR-0265) landed a UDP-only statsd sink and STRICT-REJECTED `tcp_cluster_name` (ADR-0080), deferring the TCP transport to a future row. Phase 55 consumes that deferral, as phase 50 (ADR-0267) consumed phase 49's `max_bytes_per_datagram` deferral.
>
> The line protocol is unchanged; the TRANSPORT is not. Three properties of TCP that UDP lacks forced design changes the BRAINSTORM only partly anticipated, and a live probe against `envoyproxy/envoy:contrib-v1.37.2` corrected it on three counts. (i) A TCP write BLOCKS, so phase 48's synchronous `Submit` — explicitly licensed by "a UDP datagram never blocks on a peer" — is unavailable; the sink adopts the `MetricsServiceSink` bounded-channel + writer-goroutine shape (ADR-0262) and becomes the statsd sinks' first background mutator. (ii) `tcp_cluster_name` names a CLUSTER, so the write path goes through the cluster manager; but the reference's statsd connection BYPASSES conn-pool accounting (`upstream_cx_total: 0`, `upstream_cx_active: 0`) and the `max_connections` permit, so `Cluster.Dial` is NOT reusable and a narrow unaccounted `Cluster.DialSink` is introduced. (iii) A TCP peer can vanish mid-flush. The reference does NOT drop the failed flush — it accumulates every unwritten snapshot and delivers all of them on reconnect (45 snapshots over a 30-second outage, nothing lost or duplicated). Because `conn.Write` reports `n`, envoy-go achieves the same exactness by retaining the unwritten suffix REALIGNED to the last complete line boundary `≤ n`: complete lines that landed are never re-sent, and the one straddling line — which the dead connection's stream parser discards at EOF — is re-sent whole. envoy-go BOUNDS the buffer where the reference does not, a deliberate departure consistent with the tree's backpressure posture.
>
> The reference additionally requires `node.id` and `node.cluster` for a TCP statsd sink (but not for UDP), and rejects an unknown `tcp_cluster_name` at boot; envoy-go mirrors both.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

| Surface | At SPEC (docs-only) | Anticipated at IMPL |
|---|---|---|
| stat surface | 1200 | **1200 (+0)** |
| fixtures | 99 | **100** (`0098-stats-sink-statsd-tcp`) |
| fuzzers | 52 | **52 (+0)** |
| BackendKind tail | 38 | **38 (+0)** |
| DECISIONS tail | ADR-0271 | **ADR-0272** (next-free ADR-0273) |
| new Go packages | 0 | **0** |
| new go.mod modules | 0 | **0** |

Row 55 STAYS `in-progress`. **Next → the phase-55 PLAN** (decompose §10 into a bite-sized TDD spine; resolve the five §12 D-questions; FINAL ADR-0045 split-gate re-check against the grown ~250–330 LoC envelope).
