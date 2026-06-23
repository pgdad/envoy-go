# Phase 43.2a SPEC — the HTTP/2 upstream multiplex connection pool substrate: local-cap-driven multi-conn growth + the stream-aware pending queue — the FIRST sub-leg of the SECOND-and-FINAL leg of the FIFTH-and-FINAL Upstream-robustness-family row (SUPERSEDES ADR-0056)

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-43.2 BRAINSTORM (`docs/envoy-go/phases/43.2-h2-connection-pool/BRAINSTORM.md`, commit `34de6d14`). This SPEC charters phase **43.2a** — the core H2 multiplex pool substrate (the brainstormed leg, sub-split at SPEC per the ADR-0045 gate; AMEND-H2-4). It lifts the ADR-0056 `defer cc.Close()` per-request-fresh discipline and replaces it with a per-endpoint pool of reusable, multiplexed upstream `*h2.ClientConn`s, layered over the 43.1 connection-pool permit substrate (ADR-0252). Counts at SPEC commit UNCHANGED (stat surface **1183** / fixtures **80** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0252**, next-free **ADR-0253**). The §11 D-H2-* empirical pins were EXECUTED IN-SESSION (2026-06-23) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Land cross-request HTTP/2 connection reuse for upstream clusters: a per-endpoint pool of multiplexed `*h2.ClientConn`s where many concurrent streams ride a small number of connections, the per-connection stream budget is the cluster's own `http2_protocol_options.max_concurrent_streams`, and a new connection is opened (consuming a 43.1 `max_connections` permit) only when the existing connections are stream-saturated. When both stream and connection capacity are exhausted a request PENDS on a stream-aware bounded wait-queue (woken on stream-free OR conn-close); queue-full ⇒ fail-fast 503. This is the project's FIRST cross-request upstream connection reuse for H2 and amortizes the H2 handshake across requests. It SUPERSEDES ADR-0056 (ADR-0253).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live probe (2026-06-23, contrib-v1.37.2) drove these amendments. **The headline (AMEND-H2-1):** the BRAINSTORM chose peer-driven `max_concurrent_streams` enforcement (the local cap deferred); the live probe revealed the reference's multi-conn pooling is driven by the cluster's OWN `http2_protocol_options.max_concurrent_streams` (the local/consumer cap) — NOT the peer's advertised SETTINGS — so the brainstorm's choice is reversed: the local cap is the load-bearing 43.2a budget.

- **AMEND-H2-1 (the strategy — LOCAL-cap-driven multi-conn growth; cross-side EXACT differential). USER-CONFIRMED 2026-06-23.** Live finding (D-H2-EXACT): with the cluster's `http2_protocol_options.max_concurrent_streams: C` set and K fully-overlapping held streams, the reference opens EXACTLY `ceil(K/C)` upstream connections, every trial, zero errors, all 200s (K=5/C=2→3 conns; K=6→3; K=7→4; K=9→5). This is CLEAN, deterministic flow-control enforcement — NOT racy like 43.1's soft `max_connections`. **But** when the cluster cap is left default and only the BACKEND advertises a low `SETTINGS_MAX_CONCURRENT_STREAMS`, the reference packs all streams onto ONE connection (per its own high default) and the backend REFUSES the excess (`503` `REFUSED_STREAM`) — it does NOT reactively open more connections. ⇒ envoy-go's pool keys conn-growth off the LOCAL cap (`http2_protocol_options.max_concurrent_streams`), and the `0079` differential can be cross-side EXACT on connection counts. The local cap, deferred in the BRAINSTORM (§8), is PULLED INTO 43.2a scope.
- **AMEND-H2-2 (stat surface — `upstream_cx_http2_total` counter + `http2.streams_active` gauge; NO `_http2_active`).** Live finding (D-H2-STATS): the reference exposes the protocol-conn split as COUNTERS only — `upstream_cx_http1_total` / `upstream_cx_http2_total` / `upstream_cx_http3_total`. There is **no** `upstream_cx_http2_active`. Live H2 multiplex is tracked by `cluster.<name>.http2.streams_active` (gauge — current concurrent streams across all conns) + the protocol-agnostic `upstream_cx_active`. 43.2a ADDS exactly two shapes: `upstream_cx_http2_total` (counter; ++ per new pooled H2 conn) + `http2.streams_active` (gauge; the live multiplexed-stream count). Surface **1183 → 1185** (+2). The `http2.*` reset/GOAWAY subtree (`http2.rx_reset`, `http2.tx_reset`, `http2.goaway_sent`, `http2.stream_refused_errors`, …) is DEFERRED to 43.2b (the rotation leg).
- **AMEND-H2-3 (default cap — high / single-conn-multiplex).** Live finding (D-H2-DEFAULT): Envoy's default upstream `max_concurrent_streams` is effectively unbounded (≈2^31-1); with no cap configured, 30 concurrent held streams all multiplexed onto ONE upstream conn. envoy-go applies a high default when `http2_protocol_options.max_concurrent_streams` is absent (effectively single-conn multiplex; the pool grows conns only when the cap is configured small). The `0079` differential sets the cap explicitly on BOTH sides.
- **AMEND-H2-4 (the 43.2a/43.2b sub-split — decided at SPEC, per the BRAINSTORM's pre-authorization).** With the local-cap parse pulled in (AMEND-H2-1), the load-bearing stream-aware pending queue, multi-conn growth, the stats, and GOAWAY rotation, the full leg estimates ≈400+ prod LoC / ~16+ tasks — over the soft target, and GOAWAY needs its own differential infra (a GOAWAY-emitting H2 backend). 43.2 sub-splits: **43.2a** (this SPEC) = the core multiplex pool substrate (local-cap-driven growth + stream-aware queue + router rewire + the 2 stats + minimal liveness; supersedes ADR-0056); **43.2b** (future) = graceful GOAWAY rotation + drain lifecycle + the `http2.*` reset/goaway stats + a GOAWAY differential (ADR-0254). Row 43 becomes a 3-leg row (43.1 + 43.2a + 43.2b) and flips `done` only when all land (`reference_roadmap_split_phase_row_done`).
- **AMEND-H2-5 (peer REFUSED_STREAM — local cap only at 43.2a; peer-min a hardening item).** Live finding (D-H2-EXACT/b): if our per-conn stream count reaches a peer that advertises a STRICTER `SETTINGS_MAX_CONCURRENT_STREAMS`, the backend refuses the excess stream (`REFUSED_STREAM` → the codec's existing reset path → 503). 43.2a enforces the LOCAL cap for conn-growth (the reference's behavior) and lets a peer-stricter refusal surface via the codec's existing RST handling. Enforcing `min(local,peer)` to proactively avoid the refusal is a recorded hardening item (§12 / 43.2b consideration); the `0079` differential sets the local cap ≤ the backend's advertised SETTINGS so REFUSED_STREAM does not fire in the EXACT prong.

### 1.2 ADR continuity + D-disposition at SPEC commit

- **ADR-0253** (next-free) — the 43.2a architecture; SUPERSEDES ADR-0056; §Context drafted here (§13), §Decision/§Consequences land at the 43.2a IMPL (ADR-0044).
- 43.2b will anchor **ADR-0254** (GOAWAY rotation) at its own SPEC/IMPL.
- D-H2-STATS / D-H2-DEFAULT / D-H2-EXACT / D-H2-PROTO: PINNED at this SPEC (§11). D-H2-BACKEND / D-H2-SPLIT-FINAL / D-H2-CLOSEDSTREAMS: PLAN/IMPL pins (§12). D-H2-EXACT confirmed the differential is cross-side EXACT (no 43.1-style robust-only departure).

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §1.1 amendments)

- **Graceful GOAWAY-driven rotation** + the drain-close-on-last-stream lifecycle + the `http2.*` reset/goaway stats — **43.2b** (AMEND-H2-4). 43.2a includes only MINIMAL liveness: skip a `closed()` conn (`ctx.Err() != nil`) and evict+close a conn on a `RoundTrip` transport error.
- **`min(local,peer)` proactive enforcement** (AMEND-H2-5) — 43.2a enforces the local cap; peer-stricter refusals surface via the existing RST path.
- `max_requests_per_connection` (the `upstream_cx_max_requests` counter the reference already emits at 0); idle-connection timeout eviction; eager/pre-warmed conn creation; `max_connection_pools`; H3/QUIC pooling; HIGH-priority binding (DEFAULT-only, inherited from 43.1).
- Cross-side EXACT counts remain feasible HERE (contrast 43.1's soft-breaker departure) — no robustness-prong departure for the stream/conn counts.

---

## 3. The H2 multiplex pool — `internal/cluster/h2pool.go` (ADR-0253)

### 3.0 Split disposition — sub-leg 43.2a of the 43.2 split; FINAL ADR-0045 re-check at PLAN

43.2a is chartered here; the PLAN does the final ADR-0045 re-check (the 43.1 D-S431-7 precedent) and may further decompose tasks but ships 43.2a as one leg. 43.2b (GOAWAY rotation) is a separate cycle.

### 3.1 Structures (new `internal/cluster/h2pool.go`, package `cluster`)

- `c.h2Pool map[string][]*pooledH2Conn` on `Cluster` (keyed by `ep.Addr()`), guarded by `c.h2PoolMu sync.Mutex` — symmetric with the existing per-endpoint `h1Pool`/`h1PoolMu`.
- `pooledH2Conn { cc *h2.ClientConn; inFlight int }` — admission accounting lives in the pool under `h2PoolMu` (not on the conn).
- The per-endpoint stream-aware pending queue: a per-`Cluster` (or per-endpoint) FIFO of buffered cap-1 wake channels, mirroring the 43.1 `connPool` wake PATTERN (`sync.Mutex` + FIFO; the only out-of-lock op is the buffered cap-1 send). **A SEPARATE structure from the 43.1 `connPool.waiters`** (AMEND/BRAINSTORM §2.4): the 43.1 queue wakes on conn-permit-release only; the H2 queue must additionally wake on stream-free. Bounded by `max_pending_requests` (the 43.1 budget); queue-full ⇒ the `cluster.IsConnPoolOverflow` sentinel ⇒ fail-fast 503 + `upstream_rq_pending_overflow`.
- The per-conn stream cap = the cluster's `http2_protocol_options.max_concurrent_streams` (AMEND-H2-1), parsed onto `Cluster` (a new field, e.g. `h2MaxConcurrentStreams int64`, default high — AMEND-H2-3). `0` / absent ⇒ the high default (single-conn multiplex).

### 3.2 `h2.ClientConn` additions (`internal/filter/hcm/h2/client.go`)

- `closed() bool` — `cc.ctx.Err() != nil` (the conn's frame loop has torn down). Used by the admission scan to skip a dead conn.
- (43.2a needs the in-flight count only at the POOL layer; the pool increments/decrements `pooledH2Conn.inFlight` under `h2PoolMu`. No new conn-level stream counter required. `goneAway()` is a 43.2b add.)

### 3.3 The admission lifecycle — `AcquireH2Stream` / `releaseStream`

`(c *Cluster) AcquireH2Stream(ctx) (cc *h2.ClientConn, releaseStream func(), ep Endpoint, err error)`:
1. **Stream HIT** — under `h2PoolMu`, scan the endpoint's conns for one that is `!closed()` and `inFlight < h2MaxConcurrentStreams`; if found, `inFlight++`, unlock, return it + `releaseStream`. NO permit.
2. **Conn MISS** — `acquireConnSlot(ctx)` (43.1): permit granted ⇒ dial a fresh `ClientConn` (the `DialH2` internals, refactored for pool reuse — §3.4), add to the pool, `inFlight=1`, return; permit would exceed `max_connections` ⇒ PEND on the stream-aware queue (§3.5); queue-full ⇒ `errConnPoolOverflow` ⇒ the router's existing `IsConnPoolOverflow → 503` branch (43.1 Task 7).
3. **`releaseStream`** — under `h2PoolMu`, `inFlight--`; if a waiter is queued, hand it the freed slot on THIS conn (`inFlight++` back for the waiter; wake it — no new conn, no permit); else if the conn is `closed()` (hard-error eviction) AND `inFlight==0`, remove+`Close()` it (releases the 43.1 permit → wakes a conn-level waiter). The router calls `releaseStream` after consuming the response (replacing `defer cc.Close()`).
4. **RoundTrip transport error** — the router/pool evicts the conn: remove from the pool + `Close()` (releases the permit). In-flight streams on a hard-errored conn fail via the codec's existing RST/ctx path.

### 3.4 `DialH2` refactor

Factor the dial-internals of the current `DialH2` (the `Cluster.Dial` → `connWithGauge` assert → TLS/ALPN-or-h2c branch → `h2.NewClientConn`) into a pool-callable helper so `AcquireH2Stream`'s MISS path dials identically (preserving the 43.1 permit acquisition inside `Cluster.Dial` and the `connWithGauge.Close`→`connDec`→`releaseConn` permit release on conn-Close). The legacy single-shot `DialH2` remains for any non-pooled caller (or is removed if dead — a PLAN reachability check).

### 3.5 The stream-aware pending queue (the load-bearing concurrency)

A request that MISSes and cannot get a permit PENDS: append a buffered cap-1 channel to the per-endpoint FIFO (bounded by `max_pending_requests`; over-bound ⇒ overflow 503), then `select { <-ch ; <-ctx.Done() }`. Woken by EITHER: (a) `releaseStream` handing a freed STREAM slot on an existing conn (the waiter rides that conn, no permit), or (b) a conn-Close releasing a 43.1 permit (the woken waiter dials). ctx-cancel-while-pending re-locks + removes the waiter (the 43.1 drain-and-give-back discipline if a wake raced the cancel). Single-mutator discipline; `-race`-clean unit matrix at IMPL (the 43.1 Task-3 precedent).

### 3.6 Byte-stability

Byte-identical for non-H2 clusters (the H1 path untouched) and for H2 clusters with no `circuit_breakers` AND no `max_concurrent_streams` (default budgets 1024 / high stream cap ⇒ single-conn multiplex, effectively unbounded — the pre-43.2 observable behavior except the conn is now REUSED across requests instead of per-request-fresh). The router rewire replaces `defer cc.Close()` with pooled reuse: the OBSERVABLE change is fewer `upstream_cx_total` (conn reuse) — which is the point and is what the `0079` differential asserts; the full 80-dir differential must stay GREEN for all non-43.2 fixtures (the H2 fixtures `0004`/others now reuse conns — verify their assertions are conn-count-agnostic or update them).

---

## 4. Framework primitives — the pool over the 43.1 permit seam + the in-tree h2 codec + 0 new packages + 0 new go.mod deps

- REUSED: the 43.1 `acquireConnSlot`/`releaseConn`/`connDec` permit seam + the `connWithGauge` conn-Close release (ADR-0252); the 43.1 wake PATTERN for the stream-aware queue; the `h2.ClientConn` multiplex machinery (monotonic `nextStreamID`, concurrent `RoundTrip`, `serverS`/`goawayCh`); the per-endpoint pool shape of `h1Pool`; the `IsConnPoolOverflow`/`upstream_rq_pending_overflow` overflow path + the router `IsConnPoolOverflow → 503` branch; the `reference_docker_probe_bridge_network` differential pattern.
- NEW: `internal/cluster/h2pool.go` (the pool + queue); the `h2MaxConcurrentStreams` parse on `Cluster`; the `ClientConn.closed()` predicate; the `router_h2.go` rewire.
- ZERO new Go packages; ZERO new go.mod modules (the `h2` codec + `cluster.v3`/`upstreams.http.v3` are in-tree deps; `go mod tidy -diff` EMPTY — D-H2-PROTO).

---

## 5. Proto-field roster (per §11 D-H2-PROTO)

| # | Field | Type | Pre-43.2a | 43.2a |
|---|-------|------|-----------|-------|
| 1 | `HttpProtocolOptions.explicit_http_config.http2_protocol_options` | message | PARSE (mode = H2 discriminator only) | PARSE + read `max_concurrent_streams` |
| 2 | `http2_protocol_options.max_concurrent_streams` | UInt32Value (default high — AMEND-H2-3) | IGNORED | **PARSE-ACCEPT → the per-conn stream cap (AMEND-H2-1)** |
| 3 | `circuit_breakers.max_connections` / `max_pending_requests` | UInt32Value (default 1024) | ENFORCED (43.1) | REUSED (conn-permit + pending-queue bound) |

`go mod tidy -diff` EMPTY (D-H2-PROTO).

## 6. PARSE-REJECT roster (per §11 D-H2-REJECT)

`max_concurrent_streams` is a `UInt32Value` with no documented bound that envoy-go must reject (the reference accepts any value; `0`/absent ⇒ the high default per AMEND-H2-3). Anticipated NO new reject arm (a PLAN/IMPL confirm, D-H2-REJECT). NO new fuzzer (config-parse, not wire-decode); fuzzers STAY **42** (note: the documented running total 42 is off-by-one from the actual `^func Fuzz` count 43 — `reference_fuzzer_count_docs_drift`; 43.2a adds none, so the figure is carried unchanged).

## 7. Stat surface — add 2 (1183 → 1185) (per §11 D-H2-STATS + AMEND-H2-2)

- `cluster.<name>.upstream_cx_http2_total` — counter, ++ per new pooled H2 conn (the reference's H2-conn counter).
- `cluster.<name>.http2.streams_active` — gauge, the current multiplexed-stream count across the cluster's H2 conns.
- NO `upstream_cx_http2_active` (does not exist in the reference — AMEND-H2-2). The `http2.*` reset/goaway subtree STAYS unregistered (43.2b). `upstream_cx_total`/`upstream_cx_active` continue to count H2 conns (already driven by `Cluster.Dial`). Surface **1183 → 1185** (+2); EXACT figure confirmed against the IMPL registration test.

## 8. Differential fixture taxonomy (+1: `0079` cross-side EXACT)

### 8.1 `0079-h2-multiplex-pool` (cross-side EXACT)
An HTTP/1.1 downstream listener → an H2-upstream cluster `c_h2mp` with `http2_protocol_options{ max_concurrent_streams: C }` + `circuit_breakers{ max_connections: K_max, max_pending_requests: M }`, on BOTH the subject and the reference (`contrib-v1.37.2`). An H2 hold-and-release backend (D-H2-BACKEND — a new BackendKind 37 OR an extension of the 0004 H2 backend; PLAN call) holds streams open on a gate and advertises `SETTINGS_MAX_CONCURRENT_STREAMS ≥ C` (so the LOCAL cap binds, not the peer — AMEND-H2-1/H2-5). SLEEPLESS (release-barrier + poll-to-converge; sequential-per-side). Staged drive:
1. Fire K fully-overlapping held `GET /` ⇒ poll `/stats` until the conn count converges — assert `upstream_cx_total == ceil(K/C)` AND `http2.streams_active == K` (cross-side EXACT — the clean ceil; AMEND-H2-1) AND `upstream_cx_http2_total == ceil(K/C)`.
2. (multiplex proof) `upstream_cx_total` ≪ K (few conns, many streams).
3. (pending/overflow proof, if `K_max < ceil(K/C)`) further held requests PEND (poll `upstream_rq_pending_active`), then J oversubscribers ⇒ queue-full ⇒ downstream `503` + `upstream_rq_pending_overflow` delta (assert the DOWNSTREAM class per `reference_concurrent_attempt_downstream_class_assertion`).
4. Release barrier ⇒ all held drain to 200 ⇒ gauges (`http2.streams_active`, `upstream_rq_pending_active`) poll back to 0.

The stream/conn counts are cross-side EXACT (D-H2-EXACT — clean flow-control), NOT a 43.1-style robust prong.

### 8.2 BackendKind / fuzzer
D-H2-BACKEND: the H2 hold-and-release backend (controllable SETTINGS; holds streams on a gate) is a PLAN/IMPL deliverable — anticipated a NEW BackendKind **37** (the 43.1 `BlockingHoldResponder` 36 is H1-only). NO new fuzzer (fuzzers STAY 42).

## 9. Behavior-contract delta (the 43.2a bundle; ADR-0253 atomic landing)

A new `### Cluster — HTTP/2 upstream multiplex connection pool` subsection in BEHAVIOR_CONTRACT.md: per-endpoint `ClientConn` reuse; the local-cap (`http2_protocol_options.max_concurrent_streams`)-driven per-conn stream budget + `ceil`-driven multi-conn growth (AMEND-H2-1); the stream-aware pending queue (woken on stream-free OR conn-close; bounded by `max_pending_requests`; queue-full 503); minimal liveness (closed-skip + evict-on-error); the 2 new stats (`upstream_cx_http2_total` + `http2.streams_active`); the SUPERSESSION of ADR-0056; byte-stable when no H2 / no small caps (modulo conn-reuse reducing `upstream_cx_total`). The stat-surface block advances 1183 → 1185.

## 10. Per-task structure (~12–15 tasks; PLAN decomposes)

PROGRESS+baselines; parse `max_concurrent_streams` (+ the `Cluster` field, default high); the `h2pool.go` structures + `ClientConn.closed()`; the stream-aware pending-queue primitive (LOAD-BEARING, `-race -count=1`); `AcquireH2Stream`/`releaseStream` + the `DialH2` refactor; the stats (1183→1185) + registration test; the `router_h2.go` rewire (2 sites) + byte-stability gate; the `0079` H2 hold backend (BackendKind 37) + fixture; `0079` deliberate breaks + flake + `-race`; full 81-dir differential + six-gate; ADR-0253 body + BEHAVIOR_CONTRACT; completion bundle (ROADMAP leg 43.2a → done; row 43 STAYS in-progress — 43.2b pending).

## 11. SPEC-time empirical-pin block (D-H2-* — executed IN-SESSION 2026-06-23)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared bridge `h2probe43`; a Go h2c hold-backend with controllable `SETTINGS_MAX_CONCURRENT_STREAMS`; a concurrent held-stream burst driver; `/stats`-delta scrapes; request path verified `upstream_cx_total>0` + backend `proto=HTTP/2.0`).

| Pin | Result |
|-----|--------|
| **D-H2-PROTO** | CONFIRMED. `max_concurrent_streams` lives on `http2_protocol_options` (read at the existing `manager.go` H2-mode parse site); `go mod tidy -diff` EMPTY → ZERO new module. |
| **D-H2-STATS** | PINNED (AMEND-H2-2). Protocol-conn split is COUNTERS only (`upstream_cx_http1_total`/`http2_total`/`http3_total`); NO `upstream_cx_http2_active`. Live multiplex = `http2.streams_active` (gauge). 43.2a adds `upstream_cx_http2_total` + `http2.streams_active` (+2 → 1185). The `http2.*` reset/goaway subtree (24 names) is 43.2b. `upstream_cx_max_requests` already emitted at 0 (max_requests_per_connection deferred). |
| **D-H2-EXACT** | PINNED (the headline — AMEND-H2-1). Multi-conn growth keys off the cluster's OWN `max_concurrent_streams` (`ceil(K/C)` deterministic, zero errors, cross-side EXACT-able). Peer-SETTINGS-only ⇒ pack-onto-one-conn + REFUSED_STREAM 503, NO reactive reconnect. ⇒ local-cap-driven design; EXACT differential feasible. |
| **D-H2-DEFAULT** | PINNED (AMEND-H2-3). Default upstream `max_concurrent_streams` effectively unbounded (≈2^31-1) ⇒ single-conn multiplex (30 streams / 1 conn). envoy-go applies a high default for absent/0. |
| **D-H2-REFUSED** | PINNED (AMEND-H2-5). Peer-stricter SETTINGS ⇒ `REFUSED_STREAM` → 503 (no reconnect). 43.2a enforces the local cap; peer-min is a hardening item. |

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-H2-BACKEND** — the `0079` H2 hold-and-release backend: a new BackendKind 37 (controllable SETTINGS + gate-held streams) vs an extension of the 0004 H2 backend.
- **D-H2-SPLIT-FINAL** — the final ADR-0045 re-check at PLAN (43.2a flat; the 43.1 D-S431-7 precedent).
- **D-H2-CLOSEDSTREAMS** — the `closedStreams` map bounding under now-long-lived pooled conns (M-12; may be a 43.2a hardening or a 43.2b item).
- **D-H2-LEGACY** — whether the single-shot `DialH2` + the legacy `doH2` site are still reachable post-rewire (remove if dead).
- **D-H2-BYTESTAB** — confirm the existing H2 fixtures (`0004` etc.) stay GREEN under conn-reuse (assertions conn-count-agnostic or updated).
- **D-H2-PEERMIN** — whether to enforce `min(local,peer)` in 43.2a (avoid REFUSED_STREAM) or defer (AMEND-H2-5).
- **D-H2-MUTEX** — the PLAN must make the single-mutator invariant explicit: `pooledH2Conn.inFlight` is guarded SOLELY by `h2PoolMu` (the load-bearing concurrency surface; the 43.1 Task-3 single-mutator discipline). No conn-level stream counter; the pool mutex is the only guard.
- **D-H2-EVICTORDER** — the PLAN must specify the `releaseStream` ordering when a conn is BOTH `closed()` (hard-error) AND has a queued waiter: waiter-handoff takes precedence ONLY on a LIVE conn; for a `closed()` conn the waiter must fall through to a dial/permit path (never hand a waiter a dead conn). Eager close+remove of a `closed() && inFlight==0` conn.
- **D-H2-LEGACY** (sharpened) — BOTH router H2 sites must be rewired off `DialH2`+`defer cc.Close()` (the live `doH2ClusterAction` AND the legacy `doH2` at `router_h2.go:~290`, which still carries the `ADR-0056: per-request fresh conn close` comment) — OR the legacy site proven dead and removed. The ADR-0056 supersession is INCOMPLETE in code until both per-request-fresh sites are gone.

## 13. ADR continuity — the ADR-0253 §Context DRAFT (anchored here; full entry lands at the 43.2a IMPL)

**ADR-0253 §Context (draft):** ADR-0056 deferred upstream H2 stream pooling to the Upstream-robustness family and named that family as its superseder. Phase 43.1 landed the connection-pool budget substrate (ADR-0252). The 43.2 BRAINSTORM chartered the H2 multiplex pool; the 2026-06-23 live probe (D-H2-EXACT) revealed the reference's multi-conn pooling is driven by the cluster's own `http2_protocol_options.max_concurrent_streams` (deterministic `ceil(K/C)`), not the peer SETTINGS — reversing the brainstorm's peer-driven choice (AMEND-H2-1, user-confirmed). 43.2a implements a per-endpoint multiplex `ClientConn` pool over the 43.1 permit substrate, local-cap-driven, with a stream-aware pending queue; it SUPERSEDES ADR-0056. GOAWAY rotation + the reset/goaway stats are the 43.2b leg (ADR-0254). §Decision/§Consequences land at the 43.2a IMPL.

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at SPEC (docs-only): stat **1183** / fixtures **80** / fuzzers **42** / BackendKind **36** / DECISIONS **ADR-0252** (next-free **ADR-0253**). Anticipated at 43.2a IMPL: stat **1185** (+2) / fixtures **81** (`0079`) / fuzzers **42** / BackendKind **37** (the H2 hold backend) / DECISIONS **ADR-0253**. ROADMAP row 43 STAYS `in-progress` (43.1 + 43.2a done; 43.2b pending). Next → the 43.2a PLAN.
