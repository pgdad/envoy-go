# Phase 55 Brainstorm — the plain-statsd `tcp_cluster_name` transport (the EIGHTH row of the Observability family; a NEW `Sink` sibling over the LANDED phase-48 statsd line-protocol emitter; a SINGLE FLAT ROW, no new package)

> **Lifecycle stage:** BRAINSTORM (lifecycle-state 0 → 1). Docs-only, worktree `.worktrees/phase-55-brainstorm`, branch `phase-55-stats-sink-statsd-tcp-brainstorm` (`feedback_git_worktrees`). Row 55 registers `in-progress` AT this BRAINSTORM commit (the ROADMAP §Schema invariant — NOT pre-populated).
>
> **The pick is already made** (`tcp_cluster_name` — the cheapest remaining deferred Observability candidate, chartered in `next-prompt.txt` after the Load-balancing family closed at phase 54). This BRAINSTORM settles the SCOPE, not the "which row" question.
>
> **User dialogue (5 questions, 2026-07-08):**
> - **Q1 — writer shape → BOUNDED CHANNEL + WRITER GOROUTINE** (the `MetricsServiceSink`/ADR-0262 shape). Phase 48 chose a SYNCHRONOUS `Submit` and its SPEC was explicit that the license for that was a property TCP does not have: *"a UDP `Write` is non-blocking fire-and-forget … a UDP datagram never blocks on a peer"* (`SPEC-48.md:78`). A TCP write blocks, and `Cluster.Dial` blocks for up to `connect_timeout`. The `Flusher` calls `Submit` SERIALLY across all sinks from its single goroutine, so a blocking TCP sink would starve every sibling sink. Accepted cost: the statsd sinks' FIRST background mutator (`reference_full_suite_race_after_background_mutator` applies at the merge gate).
> - **Q2 — dial primitive → `Cluster.Dial(ctx)`** (`internal/cluster/cluster.go:536`), the landed general primitive that `tcp_proxy`/`redis_proxy`/`thrift_proxy` and the `grpcclient` dial-callback already use. Free: LB endpoint pick, `connect_timeout`, upstream TLS, the `max_connections` permit, and `upstream_cx_total`/`upstream_cx_active` accounting. ACCEPTED CONSEQUENCE: a SELF-REFERENCE — the sink reports, over its own connection, the connection counters its own connection produced. The reference routes its `TcpStatsdSink` through its own cluster manager too, so the same self-reference is EXPECTED there; D-TCP-CXSTATS pins it live, and the differential simply does not assert that cluster's `cx` counters.
> - **Q3 — write-error policy → DROP THE BATCH, RECONNECT LAZILY.** This is the ONE place the `MetricsServiceSink` precedent does NOT transfer. That sink reconnects and re-sends the whole batch on a `Send` error, which is safe because gRPC messages are atomic units. A raw TCP byte stream is not: `conn.Write` can return `0 < n < len(buf)`, meaning some statsd lines fully landed before the error. Since the receiver SUMS `|c` deltas, replaying a batch whose prefix already landed silently DOUBLE-COUNTS. So: on ANY write error, `conn.Close()` → `conn = nil` → rate-limit-log → drop the batch. Never replay. The batch's increments are lost (already latched by `delta.apply`) — accepted, and consistent with statsd's lossy-by-design contract and the phase-48 UDP sink's own drop-on-write-error posture. D-TCP-RECONNECT pins that the reference likewise does not replay.
> - **Q4 — differential proof shape → PAYLOAD PARITY CROSS-SIDE + SUBJECT-EXACT `ConnCount() == 1`.** Payload parity alone would pass even if the sink redialed on every single flush — the phase's single most important new behavior (a long-lived connection) would go differentially unproven. But a CROSS-SIDE connection-count assertion bets on the reference's thread model (Envoy's statsd sink is thread-local; under `--concurrency 1` that is *probably* one connection). So: cross-side on the payload, subject-exact on the connection count, and the reference's count is RECORDED but UNASSERTED — upgradeable to cross-side only if D-TCP-CONNCOUNT pins the reference at exactly 1. Precedent: `reference_max_connections_soft_breaker` (subject-exact + cross-side robust).
> - **Q5 — envelope → SINGLE FLAT ROW, ADR-0272.** ~180–230 prod LoC, one new file, one new fixture, one ADR. Matches phase 50 (the direct "lift a strict-reject" precedent, whose Q4 also chose a single flat row) and phase 54 (150–260 LoC, single flat row). ADR-0045 escape-valve UNCONSUMED.

---

## 1. Mission and scope confirmation (55 — a single flat row, a new transport sibling)

### 1.1 What phase 55 delivers as a self-contained whole (statsd over a named cluster)
Lifts the phase-48 STRICT-REJECT at `internal/bootstrap/bootstrap.go:567-568`:

```go
if sd.GetTcpClusterName() != "" {
    return fmt.Errorf("bootstrap: stats_sinks[%d]: statsd tcp_cluster_name is not supported (envoy-go is UDP-only; configure address.socket_address)", idx)
}
```

into a genuine accept-and-honor path: a new `TCPStatsdSink` (`internal/statssink/statsd_tcp.go`) that writes the SAME statsd line protocol phase 48 already emits — `<prefix>.<dotted-name>:<value>|<c|g>` — over a long-lived TCP connection obtained from the named cluster via `Cluster.Dial`, newline-delimited, one write per flush.

### 1.2 What phase 55 does NOT deliver (forward to §8)
Any change to the statsd LINE FORMATTING (`emitStatsdLines`, `udp.go`) or the sink-private always-on `deltaState` (`delta.go`) — the transport is a strictly separate concern applied AFTER line formatting; any change to the landed `StatsdSink` (UDP), `DogStatsdSink`, or `udpWriter`; any `tcp_cluster_name`-equivalent for `dog_statsd` (the `DogStatsdSink` proto oneof has ONLY the `address` member — confirmed at `stats.pb.go:702-703` — so there is no such field to lift); `graphite`/OTLP-metrics sinks; the tap filter; tracing extras.

### 1.3 Phase-done as the EIGHTH Observability-family row landing (family STAYS OPEN)
Row 55 is the EIGHTH Observability-family row (after gRPC-ALS @ 44, OTLP-log @ 45, tracing @ 46, metrics_service @ 47, statsd @ 48, dog_statsd @ 49, dog_statsd-batching @ 50). The family STAYS OPEN — remaining deferred candidates after this row: `graphite`/OTLP-metrics sinks, tracing `custom_tags`/`spawn_upstream_span`/`http_service`/force-trace, the tap filter. NO parent rollup (ADR-0106); row 55 flips `done` at the phase-55 IMPL six-gate.

### 1.4 ADR-0045 split readiness — a SINGLE FLAT ROW (escape-valve unconsumed) *(Q5)*
One new file, one bootstrap parse-arm relaxation, one `main.go` build arm, one `test/helpers` extension, one fixture. ~180–230 prod LoC anticipated — inside the band phase 54 (150–260) and phase 50 occupied as single flat rows. A pre-authorized 55.1/55.2 escape-valve stays UNCONSUMED per ADR-0045 unless the SPEC surfaces unexpected size. The two rejected splits are recorded for the SPEC's re-check: a `55a transport / 55b differential` split would strand a LIFTED strict-reject with no differential proof at 55a's close — exactly the shape ADR-0080 exists to prevent; a `55a sink / 55b reconnect-hardening` split would ship a sink that mishandles a peer restart, for the sake of deferring ~15 LoC.

### 1.5 Seed-stub alignment + package placement
`TCPStatsdSink` lives in a NEW FILE `internal/statssink/statsd_tcp.go` inside the EXISTING `internal/statssink` package. The TCP receiver lives in the EXISTING `test/helpers/statsdrecv` package (extended, NOT replaced — the router's explicit instruction). NO new package, NO new go.mod module.

### 1.6 No prebrainstorm-notes branch
No off-master prebrainstorm-notes branch exists for this row.

### 1.7 Phase 55's relationship to the existing seams (a new transport sibling over the LANDED phase-48 emitter)
REUSES, UNCHANGED: the phase-47 `Flusher`/`Sink` seam (`flusher.go`, `sink.go`); the phase-48 statsd line emitter `emitStatsdLines` (`udp.go:64`); the sink-private always-on `deltaState` (`delta.go`); the `stats_sinks[]` TypeURL dispatch in `parseStatsSinks`; the `Cluster.Dial` primitive (`cluster.go:536`) and its `connWithGauge` Close-once discipline; the `cmd/envoy-go/main.go` sink build loop shape; the `0092-stats-sink-statsd` differential harness shape (two driver-owned per-side receivers + hard `Close()`) as the `0098` template.

NEW: `internal/statssink/statsd_tcp.go` (`TCPStatsdSink`); the `parseStatsdSinkConfig` accept-arm for `tcp_cluster_name` (replacing the reject); a `cmd/envoy-go/main.go` build arm closing over the cluster manager; a TCP listener + `ConnCount()` on `test/helpers/statsdrecv`; the `0098-stats-sink-statsd-tcp` fixture.

---

## 2. Design decisions

### 2.1 Row + subject confirmation: the Observability family continues with the statsd TCP transport *(Q0 → phase 55 row registered)*
The loop RE-OPENED after the Load-balancing family closed at phase 54. `tcp_cluster_name` is the cheapest remaining deferred Observability candidate and has an exact, recent precedent: phase 50 lifted the phase-49 `max_bytes_per_datagram` strict-reject; phase 55 lifts the phase-48 `tcp_cluster_name` strict-reject the same way.

### 2.2 Sink structure: a SIBLING TYPE, not a swappable writer on `StatsdSink` *(self-answered; follows from Q1)*
`StatsdSink` (UDP) and `TCPStatsdSink` are sibling `Sink` implementations, NOT one type with a pluggable transport. This is a direct consequence of Q1: their `Submit` shapes now genuinely differ (synchronous inline write vs. channel-enqueue), so a shared supertype could hoist only `prefix` and `delta` — two fields — while forcing `Submit` down into the transport anyway. The landed `StatsdSink`, `DogStatsdSink`, and `udpWriter` are therefore **NOT TOUCHED AT ALL** by this row (zero blast radius; the UDP path's byte-stability is preserved trivially, not argued).

Sharing happens where it belongs: at `emitStatsdLines` (the line grammar) and `deltaState` (the counter transform), both reused verbatim.

### 2.3 The dial seam: a `func` parameter, so `internal/statssink` does NOT import `internal/cluster` *(self-answered; the landed `metricsClient` precedent)*
`internal/statssink` today imports no cluster code. `sink.go:29-32` establishes the pattern for exactly this situation — a narrow, package-local seam (`metricsClient`) that decouples the PRODUCTION sink from `grpcclient`, keeps the package acyclic, and lets `sink_test.go` fake it (with a TEST-ONLY compile-time assertion pinning that the real client satisfies it).

Apply it here in its simplest form — a function value, since the seam has exactly one method:

```go
// internal/statssink/statsd_tcp.go
func NewTCPStatsdSink(dial func(context.Context) (net.Conn, error), prefix string) *TCPStatsdSink
```

and in `cmd/envoy-go/main.go`, close over the manager and re-look-up per dial exactly as `grpcclient`'s dial-callback does (`grpcclient.go:137`):

```go
if _, ok := cm.Get(cfg.TCPClusterName); !ok {
    log.Fatalf("statssink: statsd tcp_cluster_name %q: unknown cluster", cfg.TCPClusterName)
}
sink := statssink.NewTCPStatsdSink(func(ctx context.Context) (net.Conn, error) {
    cl, ok := cm.Get(cfg.TCPClusterName)
    if !ok {
        return nil, fmt.Errorf("unknown cluster %q", cfg.TCPClusterName)
    }
    c, _, err := cl.Dial(ctx)
    return c, err
}, cfg.Prefix)
```

This is a `func` and not an `interface` because the seam has ONE method with no shared state; a one-method interface would add a name without adding a constraint. The unit tests fake it with a closure returning a `net.Pipe` end or a `*net.TCPConn` to a local listener.

### 2.4 Writer shape: bounded channel + writer goroutine *(Q1 — LOAD-BEARING)*
Mirrors `MetricsServiceSink` (ADR-0262), SIMPLIFIED the same way phase 47 simplified it — each `Submit` is one already-complete batch, so the writer does NO size/interval accumulation:

```go
type TCPStatsdSink struct {
    ch     chan []*dto.MetricFamily // cap defaultChannelCapacity (8); drop-newest
    dial   func(context.Context) (net.Conn, error)
    prefix string
    delta  *deltaState // always non-nil — statsd |c is intrinsically a per-flush delta

    conn net.Conn // OWNED BY run(); lazily dialed, nil ⇒ redial on next flush

    done        chan struct{}
    closeOnce   sync.Once
    closeErr    error
    lastDropLog atomic.Int64
    ctx         context.Context
    cancel      context.CancelFunc
}

func (s *TCPStatsdSink) Submit(batch []*dto.MetricFamily) {
    select {
    case s.ch <- batch:
    default: // drop-newest + rate-limited diagnostic (the accesslog lastDropLog idiom)
    }
}
```

`conn` is touched ONLY by the writer goroutine (`run`), never by `Submit` and never by `Close` except on the deliberate unwedge path (§2.7). `defaultChannelCapacity` and `dropLogIntervalNanos` are reused from `sink.go`.

**Delta is applied in the WRITER, not in `Submit`.** This is the ADR-0263 rule and it is load-bearing here for the same reason: an enqueue-drop must never latch `deltaState`, so the dropped increments ride the next successfully-enqueued flush instead of being silently lost. (Contrast the landed UDP `StatsdSink`, which applies delta in `Submit` — correct there precisely because a synchronous sink has no enqueue step to drop at.)

### 2.5 Framing: newline-delimited, one write per flush *(SPEC pins the exact bytes, D-TCP-LINE — LOAD-BEARING)*
The line grammar is UNCHANGED from phase 48 (`emitStatsdLines`); TCP adds a delimiter because a stream has no datagram boundary. Anticipated: every line `\n`-terminated (including the last), all lines of a flush serialized into ONE buffer and written with ONE `conn.Write`.

This is an ANTICIPATION, not a decision. Per `reference_wire_format_both_sides_see_same_bytes`, the wire is shared and "our frame format" is never a valid deviation: D-TCP-LINE probes the reference's actual bytes (is the last line terminated or separated? is there a trailing newline? one write per flush or per line?) and the IMPL adopts them verbatim. Note that write-granularity is NOT observable to a line-parsing receiver over a stream, so the ASSERTABLE part of this pin is the byte sequence, not the syscall count.

### 2.6 Write-error policy: drop the batch, reconnect lazily, NEVER replay *(Q3 — LOAD-BEARING)*
```go
func (s *TCPStatsdSink) flush(batch []*dto.MetricFamily) {
    if s.conn == nil && !s.redial() {
        return // dial failed: logged, batch dropped
    }
    var b strings.Builder
    emitStatsdLines(batch, func(fam *dto.MetricFamily) (string, string) {
        return s.prefix + "." + fam.GetName(), ""
    }, func(line string) { b.WriteString(line); b.WriteByte('\n') })

    _ = s.conn.SetWriteDeadline(time.Now().Add(writeGrace))
    if _, err := s.conn.Write([]byte(b.String())); err != nil {
        s.conn.Close() // Dec upstream_cx_active + release the LB pick + release the max_connections permit
        s.conn = nil   // next flush redials
        s.logDrop(err) // batch DROPPED — no replay
    }
}
```

Three properties, each deliberate:
1. **`SetWriteDeadline` on every write.** A wedged peer can never stall the writer goroutine unboundedly, which in turn bounds `Close`'s drain by construction rather than by hope. A deadline expiry is an ordinary write error and takes the same drop-and-redial path.
2. **`conn.Close()` on any error is leak-free.** Verified against `cluster.go:359-365`: `connWithGauge.Close()` runs a single `sync.Once` closure that Decs `upstream_cx_active`, releases the LB pick, AND calls `pool.releaseConn()` on the `max_connections` permit. Double-Close is safe (no gauge underflow). So a reconnect loop leaks neither a gauge count nor a circuit-breaker permit.
3. **No replay, ever** — the Q3 rationale. A partial write (`0 < n < len`) means a prefix of the batch's lines already landed; replaying them on a fresh conn double-counts every counter in that prefix, because the receiver SUMS `|c` deltas. Dropping loses those increments (already latched), which is the strictly safer failure and matches statsd's lossy contract.

### 2.7 `Close`: drain with grace, then HARD-close the conn to unwedge *(SPEC/PLAN pin, D-TCP-CLOSE)*
Mirrors `MetricsServiceSink.Close` (`sink.go:135-148`) — `close(ch)` → wait `done` up to `closeDrainGrace` → force → `<-done` — with ONE substantive difference. `MetricsServiceSink` unwedges a stalled `Send` by calling `s.cancel()`, because a gRPC stream is context-bound. **`cancel()` cannot interrupt a blocked `net.Conn.Write`.** The analogous unwedge for a raw conn is `s.conn.Close()`, which makes the in-flight `Write` return `use of closed network connection`. Combined with §2.6's per-write deadline this is belt-and-braces; the PLAN pins which of the two is primary and how `Close` reaches `conn` (which is otherwise writer-goroutine-owned) without a data race — most likely by storing it in an `atomic.Pointer[net.Conn]` or by having `Close` cancel `ctx` and letting a `ctx`-watching helper close it. **This is a genuine `-race` hazard and must not be hand-waved.**

### 2.8 Config validation after the lift: the oneof CLOSES *(ADR-0080; discharges `reference_strict_reject_sibling_typeurl_gap`)*
`StatsdSink`'s `statsd_specifier` oneof has EXACTLY TWO arms — verified at `go-control-plane/envoy@v1.37.0` `config/metrics/v3/stats.pb.go:571-577, 675-691`:

```go
//	*StatsdSink_Address
//	*StatsdSink_TcpClusterName
StatsdSpecifier isStatsdSink_StatsdSpecifier `protobuf_oneof:"statsd_specifier"`
```

So lifting `tcp_cluster_name` **closes** the oneof. There is no third sibling arm left to slip through, which discharges the `reference_strict_reject_sibling_typeurl_gap` hazard AT THE ONEOF LEVEL. (The hazard is discharged, not absent: the TypeURL-level dispatch in `parseStatsSinks` is unchanged and still rejects unknown sink TypeURLs. The two levels are independent.)

The reject arms after the lift:
- **REMOVED:** `sd.GetTcpClusterName() != ""` → error (`bootstrap.go:567-568`). Removed entirely, not narrowed — there is no remaining condition under which this field warrants a parse-time reject. Precedent: phase 50 §2.6 removed the `max_bytes_per_datagram` reject outright.
- **RETAINED (reference-parity):** BOTH oneof arms unset (`statsd_specifier` missing) → reject. Note the load-bearing ORDERING consequence: `parseStatsdSinkConfig`'s comment at `bootstrap.go:556-561` observes that `GetAddress()` returns nil for BOTH a missing oneof AND a `tcp_cluster_name` arm, which is why the `tcp_cluster_name` check runs FIRST. After the lift the same ordering constraint survives with inverted meaning — the `tcp_cluster_name` arm must be DISPATCHED before the nil-`address` reject fires, or a valid TCP config would be rejected as "missing `statsd_specifier`". A regression test must pin this.
- **NEW:** a `tcp_cluster_name` naming a cluster ABSENT from the cluster manager → boot fatal at sink build in `main.go`, NOT at bootstrap parse. This mirrors the LANDED metrics_service precedent exactly: `parseMetricsServiceConfig` does no cluster-existence check; `grpcclient.NewMetricsServiceClient` → `Dialer.DialContext` → `mgr.Get` → `"unknown cluster"` → `log.Fatalf` at `main.go:200-203`. D-TCP-REJECT probes whether the reference rejects at boot too.

### 2.9 Config shape: NOT SETTLED — a PLAN-time question *(D-TCP-CONFIG-SHAPE)*
Two viable shapes, deliberately left open:
- **(a)** extend `StatsdSinkConfig` with `TCPClusterName string` (a tagged union — exactly one of `UDPAddress`/`TCPClusterName` non-empty). One slice, one `main.go` loop with a two-arm branch. Preserves `stats_sinks[]` relative ordering among statsd entries.
- **(b)** add a separate `Bootstrap.StatsdTCPSinkConfigs []StatsdTCPSinkConfig` slice. No tagged union, no discriminator branch; consistent with the landed three-separate-slices shape (`StatsSinkConfigs` / `StatsdSinkConfigs` / `DogStatsdSinkConfigs`), which has ALREADY given up cross-sink-type ordering.

The PLAN decides. Neither affects the wire or the sink.

### 2.10 The `Cluster.Dial` self-reference: documented, not asserted *(Q2; SPEC pins, D-TCP-CXSTATS)*
Because the sink dials through the cluster manager, the stats cluster accrues `cluster.<name>.upstream_cx_total` and `upstream_cx_active` — counters the sink then reports over that very connection. This is stable (one long-lived conn ⇒ `cx_total: 1`, `cx_active: 1`) and is EXPECTED to match the reference, which also routes `TcpStatsdSink` through its own cluster manager. D-TCP-CXSTATS pins it. The `0098` differential does NOT assert those counters either side — they are a property of the transport under test, and asserting them would make the fixture a test of `Cluster.Dial` rather than of the sink.

Note also: the sink's connection consumes one `max_connections` permit on the stats cluster's circuit breaker (ADR-0252). A stats cluster configured with `max_connections: 0` would therefore deadlock the sink's dial. This is inherited reference behavior, not a new envoy-go departure, and needs no new reject — but the SPEC should note it in `BEHAVIOR_CONTRACT.md`.

### 2.11 Receiver-parser risk: a TCP stream is NOT a datagram *(flagged; `reference_line_parser_extension_delimiter_reuse`)*
`statsdrecv.ingest` (`statsdrecv.go:131`) today receives one UDP **datagram** and splits it on `\n`. Every line in a datagram is complete BY CONSTRUCTION — the datagram boundary guarantees it. **A TCP read carries no such guarantee: a single `Read` can return a partial trailing line, and a line can span two reads.** Calling `ingest` on raw TCP read chunks would silently corrupt the accumulator, and — worse — would corrupt it in a way that *looks* like a proxy bug rather than a receiver bug.

The TCP listener must therefore read line-at-a-time through `bufio` (`Scanner`, or `Reader.ReadString('\n')`) with a carried remainder, and discard an incomplete trailing line at EOF. Per the standing lesson, the PLAN task must **trace one concrete split-read example byte-for-byte** before touching the parser — not merely describe the grammar. This is the same trap that bit the phase-49 tag-suffix extension.

Note that `MaxLinesInAnyDatagram`/`LinesInDatagram` (phase-49/50 accessors) are datagram concepts with no TCP meaning; they stay UDP-only and are simply not populated by the TCP path. No conflict, but it must be documented rather than left to a future reader's surprise.

### 2.12 Stat surface hypothesis: zero new self-stats *(self-answered; SPEC pins, D-TCP-STATS)*
A new transport carries no new counters/gauges — the metrics_service/statsd/dog_statsd +0 precedent continues. The `cluster.<name>.upstream_cx_*` counters the sink's own dial increments are PRE-EXISTING stat NAMES (registered for every cluster), so §2.10's self-reference is a new stat INSTANCE, not a new stat name. Anticipated stat surface UNCHANGED at 1200.

---

## 3. Framework-survey result — a new `Sink` sibling; ZERO new packages, ZERO new go.mod modules (55 anticipated)

### 3.1 Framework: no new framework piece
No new flush loop, no new seam, no new typed-client layer. `Flusher` (`flusher.go`) and the `Sink` interface (`sink.go:17-20`) are untouched. `TCPStatsdSink` is the FOURTH `Sink` implementation (after `MetricsServiceSink`, `StatsdSink`, `DogStatsdSink`).

The one genuinely new primitive-USE is `Cluster.Dial` called from OUTSIDE the data plane. That is not a new primitive — `grpcclient`'s dial-callback already does exactly this (`grpcclient.go:137`) — but `TCPStatsdSink` is the first consumer to hold a raw `net.Conn` from a cluster for the process lifetime, so the ADR should record the lifecycle contract (§2.6's Close-once leak-freedom) explicitly.

### 3.2 NEW packages: anticipated NONE.

### 3.3 go.mod modules: anticipated NONE. Pure stdlib (`net`, `bufio`, `strings`, `sync`, `context`). `go mod tidy -diff` anticipated EMPTY.

### 3.4 REUSES
`emitStatsdLines` + `deltaState` (both verbatim); the `Flusher`/`Sink` seam; `defaultChannelCapacity`/`dropLogIntervalNanos`/`closeDrainGrace` constants from `sink.go`; the `MetricsServiceSink` bounded-channel + writer-goroutine + idempotent-`Close` shape (ADR-0262) as the structural template; `Cluster.Dial` + `connWithGauge`'s Close-once discipline; the `parseStatsSinks` TypeURL dispatch; the `0092` differential harness shape; `test/helpers/statsdrecv` (extended in-place with a TCP listener + `ConnCount()`, per §2.11).

---

## 4. Bootstrap-level applicability — the `stats_sinks[]` surface (NOT per-listener), same `StatsdSink` entry
No change to WHERE this config lives — still the top-level `Bootstrap.stats_sinks[]` `StatsdSink` entry, just the OTHER arm of its `statsd_specifier` oneof now honored instead of rejected. A `tcp_cluster_name` sink additionally requires the named cluster to exist in `static_resources.clusters[]` (§2.8's new boot-fatal arm).

---

## 5. Stat surface hypothesis — zero new self-stats (55)

### 5.1 Stat names (SPEC pins, D-TCP-STATS)
Anticipated NONE. See §2.12 for why the `upstream_cx_*` self-reference is an instance, not a name.

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)
- REMOVED: the phase-48 `tcp_cluster_name`-set strict-reject (§2.8) — removed outright, not narrowed.
- NEW: an unknown-cluster `tcp_cluster_name` → boot fatal at sink build (the metrics_service precedent). Whether this is a DEPARTURE or reference-parity is D-TCP-REJECT.
- UNCHANGED: the missing-`statsd_specifier` reject; `socket_address.protocol` accepted-and-ignored (UDP arm only); unknown sink TypeURL rejects.

### 5.3 Anticipated surface arithmetic
Stat surface **1200 → 1200** (+0 anticipated). SPEC/IMPL pin.

---

## 6. Differential fixture envelope — anticipated ONE NEW directory `0098-stats-sink-statsd-tcp`

### 6.1 Fixture
`0098-stats-sink-statsd-tcp`. Clones the `0092-stats-sink-statsd` cross-side shape:
- **Receivers:** TWO driver-owned per-side TCP receivers (`test/helpers/statsdrecv`, extended — NOT a BackendKind, per `reference_differential_grpc_receiver_driver_owned`), both bound before either proxy starts, each with its own uncontaminated accumulator, both hard-`Close()`d after the subject snapshot — NOT `GracefulStop` (`reference_periodic_sink_differential_two_receivers`: a periodic sink's reference-side flushes contaminate the subject snapshot if a single shared receiver is used, producing VACUOUS breaks).
- **Reachability:** the reference's `c_statsd` cluster endpoint is the host-gateway literal IP (`differential.HostGatewayIP`, `reference_host_gateway_ip_docker_desktop`); the subject's is `127.0.0.1`. Note the address moves from `stats_sinks[].address` (where `0092` put it) into `clusters[].load_assignment` — same lesson, new location.
- **BackendCount:** the `c_backend` `HTTPFixedBody` backend satisfies the `>= 1` requirement (`reference_differential_backendcount_min_one`).
- **Assertions (Q4).** Cross-side, both snapshots: the counter subset `sdpfx.cluster.c_backend.upstream_rq_total` / `sdpfx.http.hcm_local.downstream_rq_total` / `sdpfx.http.hcm_local.downstream_rq_2xx` present with `|c` delta-SUM `== K` (K=7, the `0092` constant), STILL `== K` after a `>= 2`-further-flush stability barrier (`reference_delta_sink_differential_stability_barrier` — a burst-all-requests delta sink cannot otherwise tell a delta from an absolute, since the first flush's delta EQUALS the absolute); plus the absolute gauge `sdpfx.cluster.c_backend.membership_total == 1` (`reference_membership_total_vs_healthy_gauge` — `membership_healthy` is registered only on health-checked clusters).
- **Assertions, subject-side ONLY:** `subjRecv.ConnCount() == 1` (one long-lived connection across the whole run — no per-flush redial, no reconnect churn). `refRecv.ConnCount()` is RECORDED and logged but UNASSERTED pending D-TCP-CONNCOUNT.
- **UNASSERTED, both sides:** `cluster.c_statsd.upstream_cx_*` (§2.10); the whole line set; `|ms` timers; non-deterministic gauges; flush cadence; write granularity.

### 6.2 Deliberate breaks (3, each `-count=1` per `reference_differential_break_protocol_count1`)
- **(a) Redial on every flush** (drop the `conn` cache): subject `ConnCount() == 1` fails. Proves the Q4 subject-exact assertion is LIVE.
- **(b) Omit the `\n` line terminator:** the receiver's line parser sees one giant concatenated line; the counter-subset lookups miss; the delta-sum assertions fail. Proves the framing is load-bearing.
- **(c) Emit ABSOLUTE counter values instead of deltas:** the `>= 2`-flush stability barrier fails. Per `reference_delta_sink_differential_stability_barrier`, break (c) PASSES without the barrier — so the break must be run against the barrier-bearing driver, and the PLAN must additionally verify that removing the barrier would have masked it.

Differential discipline carries: `-run 'TestDifferential/0098-stats-sink-statsd-tcp'` NEVER bare (`reference_differential_run_selector` — a bare `-run '0098'` matches ZERO subtests and reports a vacuous green); Docker bridge network for the reference (`reference_docker_probe_bridge_network`); a `subject ready: EOF` failure on an UNRELATED fixture in a full-suite run is the known startup flake, not a regression — isolate-re-run to tell them apart (`reference_differential_fullsuite_startup_flake`).

### 6.3 Total
fixtures **99 → 100** (`0098-stats-sink-statsd-tcp`).

### 6.4 New BackendKind: anticipated NONE (driver-owned TCP receiver; BackendKind stays 38).

### 6.5 New fuzzer: anticipated NONE (D-TCP-FUZZER, SPEC pins)
`tcp_cluster_name` is an EXISTING field on an EXISTING parse arm. The landed `FuzzStatsdSinkConfigParse` (`internal/bootstrap/statsd_fuzz_test.go:13`) already drives arbitrary bytes through this same message and ALREADY carries a `tcp_cluster_name` seed (`statsd_fuzz_test.go:47-52`, currently labelled `// tcp_cluster_name (reject)`). This row does not add a dispatch arm; it flips that seed's expected outcome from reject to accept. Same for the `statssink_fuzz_test.go:110-115` seed. Fuzzers anticipated to STAY AT 52. Note `reference_fuzzer_count_docs_drift`: the documented running total has been off-by-one before — the IMPL must reconcile `grep -c '^func Fuzz'` against the doc, not trust the doc.

---

## 7. Anticipated ADRs — 1 at the phase-55 IMPL: ADR-0272 (the statsd TCP transport)
ADR-0272 (ACCEPTED — the plain-statsd `tcp_cluster_name` transport lifted from the phase-48 strict-reject; a new `TCPStatsdSink` `Sink` sibling; the bounded-channel writer-goroutine shape; the drop-never-replay write-error policy; the `Cluster.Dial`-from-outside-the-data-plane lifecycle contract). §Context drafted at the SPEC, §Decision/§Consequences landed in-place at the IMPL per ADR-0044. NO seam ADR (the `Flusher`/`Sink` seam, the line emitter, and `deltaState` are all reused unchanged). next-free after 55: **ADR-0273**.

---

## 8. Deferred items
- `graphite` / `open_telemetry`-metrics sinks (each its own future deferred Observability row).
- Timers / `|ms` lines (gated by the absent-histogram boundary — ADR-0060, unchanged from prior rows).
- The tap filter + tracing `custom_tags` / `spawn_upstream_span` / `http_service` / force-trace (unrelated Observability-family candidates, unchanged from the phase-50 deferred list).
- ANY `tcp_cluster_name`-equivalent for `dog_statsd` — confirmed ABSENT from the proto (`stats.pb.go:702-703`: the `DogStatsdSink` oneof has only the `address` member). Not a candidate for this pattern.
- Newline-batched packing for the UDP `StatsdSink` (phase-48 §2's unconsumed IMPL micro-decision). Orthogonal to this row; the TCP path is newline-delimited by necessity, the UDP path by choice-not-yet-made.
- A `max_connections`-exempt dial path for stats/observability connections (Q2 option C). Requires surgery on the shared `Cluster.Dial` primitive for one caller; defer unless D-TCP-CXSTATS shows the reference exempts it.

---

## 9. Cross-references against prior phases' deferred-items lists — pickup
Picks up the phase-48 ROADMAP row's / BRAINSTORM §8's deferred candidate: *"`tcp_cluster_name` transport (statsd-over-named-cluster — its own future sub-leg/row; reuses the cluster/dispatch seam)"* (`48-stats-sink-statsd/BRAINSTORM.md:117`), chartered as phase 55 on 2026-07-08. The phase-48 note's prediction that it "reuses the cluster/dispatch seam" is CONFIRMED (§2.3, `Cluster.Dial`). The remaining deferred Observability candidates (`graphite`/OTLP-metrics, tracing-extras, tap) carry forward UNbrainstormed.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227 — `reference_docker_probe_bridge_network`)
- **D-TCP-LINE (LOAD-BEARING):** the exact wire bytes. Is every line `\n`-TERMINATED (including the last), or are lines `\n`-SEPARATED with no trailing newline? Does the reference write once per flush or once per line? Per `reference_wire_format_both_sides_see_same_bytes` we adopt the reference's framing verbatim — "our frame format" is never a valid deviation. Probe with a byte-exact capture, not a line-parsing receiver.
- **D-TCP-DELTA:** confirm the reference's TCP statsd sink emits COUNTER families as per-flush DELTAS (`|c`) and GAUGE families as ABSOLUTE (`|g`) — i.e. identical to its UDP sink, and identical to phase 48's landed `deltaState`. Expected yes; a NO would invalidate §2.4's delta reuse and the whole `0098` assertion shape.
- **D-TCP-CONNCOUNT:** how many TCP connections does the reference open to the stats cluster under `--concurrency 1`? Envoy's statsd sink is thread-local, so the answer may be 1 (worker only) or 2 (main + worker). Decides whether Q4's `ConnCount()` assertion can be UPGRADED from subject-exact to cross-side-exact.
- **D-TCP-CXSTATS:** does the reference's stats cluster accrue `upstream_cx_total` / `upstream_cx_active` from the sink's own connection (§2.10's self-reference)? Confirms Q2's dial primitive is faithful. Also probe whether the reference's stats-cluster connection consumes a `max_connections` permit.
- **D-TCP-RECONNECT:** on a receiver restart mid-run, does the reference redial? Does it REPLAY the batch it failed to write (which would contradict Q3), or drop it? Confirms §2.6.
- **D-TCP-REJECT:** the reference's boot behavior for (i) `tcp_cluster_name` naming an UNKNOWN cluster, (ii) both oneof arms unset, (iii) a `tcp_cluster_name` pointing at a cluster that exists but is unreachable (expect: boots fine, sink retries). Determines whether §2.8's new boot-fatal arm is reference-parity or an envoy-go-strict departure (ADR-0080).
- **D-TCP-STATS:** confirm +0 new self-stats (§2.12).
- **D-TCP-CLOSE (PLAN):** the race-free mechanism by which `Close` reaches the writer-goroutine-owned `conn` to unwedge a blocked `Write` (§2.7). `atomic.Pointer` vs. a `ctx`-watching closer goroutine vs. relying solely on the per-write deadline. Must be `-race`-proven, not argued.
- **D-TCP-CONFIG-SHAPE (PLAN):** tagged-union field on `StatsdSinkConfig` vs. a separate `StatsdTCPSinkConfigs` slice (§2.9).
- **D-TCP-FUZZER:** confirm no new fuzzer (§6.5); reconcile the documented count against `grep -c '^func Fuzz'` per `reference_fuzzer_count_docs_drift`.

---

## 11. Prior-phase lessons applied
- `reference_full_suite_race_after_background_mutator` — **the sharpest one for this row.** `TCPStatsdSink` is the statsd sinks' FIRST background mutator. A `-run`-subset `-race` will MISS the class of failure it introduces; the merge gate needs a FULL-package `-race` on `internal/statssink` AND `test/differential`.
- `reference_line_parser_extension_delimiter_reuse` — a TCP stream is not a datagram; `ingest`'s `\n`-split assumption does not survive the transport change (§2.11). Trace a concrete split-read example before editing `statsdrecv`.
- `reference_delta_sink_differential_stability_barrier` — the `0098` driver needs the `>= 2`-further-flush barrier; break (c) passes without it (§6.2).
- `reference_periodic_sink_differential_two_receivers` — TWO per-side receivers + hard `Close()`, never one shared receiver, never `GracefulStop`.
- `reference_host_gateway_ip_docker_desktop` — the reference reaches the host receiver at the host-gateway literal IP; here it lives in `clusters[].load_assignment`, not `stats_sinks[].address`.
- `reference_wire_format_both_sides_see_same_bytes` — D-TCP-LINE adopts the reference's framing verbatim; a self-invented frame format is never valid (§2.5).
- `reference_strict_reject_sibling_typeurl_gap` — lifting one reject needs the siblings checked; here the oneof has exactly two arms so lifting CLOSES it (§2.8), and the independent TypeURL-level dispatch is unchanged.
- `reference_differential_backendcount_min_one` — `c_backend` satisfies the `>= 1` requirement for an otherwise-driver-owned fixture.
- `reference_membership_total_vs_healthy_gauge` — assert `membership_total` (unconditional), not `membership_healthy` (health-checked clusters only), on the no-HC `c_backend`.
- `reference_max_connections_soft_breaker` — the subject-exact + cross-side-robust assertion split (Q4), rather than betting on the reference's internals.
- `reference_admin_interface_wire_name_collision` — a watch-item: `0098` reuses `0092`'s `hcm_local` stat_prefix and exact-name `DeltaSum` lookups; re-verify no admin-listener HCM collapses onto the same wire name.
- `reference_differential_run_selector` / `reference_differential_break_protocol_count1` — `-run 'TestDifferential/0098-stats-sink-statsd-tcp'` never bare; `-count=1` on every break.
- `reference_differential_fullsuite_startup_flake` — a `subject ready: EOF` on an unrelated fixture is the harness startup race, not a regression; isolate-re-run to discriminate.
- `reference_fuzzer_count_docs_drift` — reconcile the fuzzer count against the source, not the doc (§6.5).
- `reference_docker_probe_bridge_network` — the SPEC probes run on a shared Docker bridge with a decode-ran proof.
- `feedback_execution_style` / `feedback_git_worktrees` / `feedback_subagents_no_push` / `feedback_subagent_autocommit_claudemd` / `feedback_pertask_gofmt_lint` — subagent-driven IMPL in a fresh worktree; subagents commit locally only; the controller verifies each commit, re-runs gates on the frozen HEAD, does deliberate-break verification ITSELF, and squashes + pushes at stage-close.

---

## 12. Section closeout
- **Subject:** the `StatsdSink.tcp_cluster_name` oneof arm — statsd line protocol over a long-lived TCP connection to a NAMED CLUSTER, lifted from the phase-48 strict-reject at `bootstrap.go:567-568`.
- **Q1 writer shape:** BOUNDED CHANNEL + WRITER GOROUTINE (the `MetricsServiceSink`/ADR-0262 shape). Phase 48's synchronous `Submit` was licensed by UDP's never-blocks property, which TCP does not have.
- **Q2 dial primitive:** `Cluster.Dial(ctx)` — the landed general primitive. Accepted consequence: a stable, reference-expected `upstream_cx_*` self-reference on the stats cluster (D-TCP-CXSTATS pins; unasserted in the differential).
- **Q3 write-error policy:** DROP the batch, reconnect lazily, NEVER replay. A partial TCP write means a replay double-counts summed `|c` deltas — the one place the `MetricsServiceSink` precedent does not transfer.
- **Q4 differential proof shape:** payload parity CROSS-SIDE + `ConnCount() == 1` SUBJECT-EXACT (the reference's count recorded but unasserted, pending D-TCP-CONNCOUNT). Payload parity ALONE would pass even under a redial-every-flush sink.
- **Q5 envelope:** SINGLE FLAT ROW (ADR-0045 escape-valve unconsumed; both candidate splits rejected with reasons at §1.4).
- **Scope:** remove the `bootstrap.go:567-568` `tcp_cluster_name` strict-reject + dispatch the oneof's second arm (§2.8, ordering-sensitive) + a new `internal/statssink/statsd_tcp.go` `TCPStatsdSink` (bounded channel, writer goroutine, `func` dial seam, per-write deadline, drop-never-replay, hard-close unwedge) + a `cmd/envoy-go/main.go` build arm with the unknown-cluster boot fatal + a TCP listener and `ConnCount()` on `test/helpers/statsdrecv` (line-at-a-time, remainder-carrying) + the `0098-stats-sink-statsd-tcp` cross-side differential.
- **Untouched:** `Flusher`, `Sink`, `deltaState`, `emitStatsdLines`, `StatsdSink` (UDP), `DogStatsdSink`, `udpWriter`.
- **Anticipated counts:** stat **1200** (+0) / fixtures **100** (`0098`) / fuzzers **52** (+0, D-TCP-FUZZER pins) / BackendKind **38** (+0) / DECISIONS **ADR-0272** (next-free ADR-0273); ZERO new packages, ZERO new go.mod modules, ZERO new `Pick` params.
- **Load-bearing SPEC probes:** D-TCP-LINE (the exact wire bytes) + D-TCP-DELTA (delta semantics survive the transport) + D-TCP-CONNCOUNT (upgradeable to a cross-side assertion?) + D-TCP-RECONNECT (confirm no replay) + D-TCP-REJECT (is the unknown-cluster boot fatal reference-parity?).
- **Load-bearing PLAN pins:** D-TCP-CLOSE (the race-free unwedge, `-race`-proven not argued) + D-TCP-CONFIG-SHAPE.
- **Row 55** registers `in-progress` at this BRAINSTORM commit; flips `done` at the phase-55 IMPL six-gate (NO parent rollup — ADR-0106). The Observability FAMILY STAYS OPEN (`graphite`/OTLP-metrics sinks, tracing extras, the tap filter remain deferred).
- **Next → the phase-55 SPEC** (`SPEC.md` — execute the §10 D-TCP-* live pins against `envoyproxy/envoy:contrib-v1.37.2`; anchor the ADR-0272 §Context draft).
