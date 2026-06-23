# Phase 43.2 Brainstorm — the HTTP/2 multiplex connection pool (the SECOND-and-FINAL leg of the FIFTH-and-FINAL Upstream-robustness-family row; cross-request `ClientConn` reuse + per-conn stream multiplexing — SUPERSEDES ADR-0056)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document settles the **leg-specific** design forks for phase 43.2 (`h2-connection-pool`), the second-and-final leg of ROW 43 (`connection-pooling`). The parent charter — `docs/envoy-go/phases/43-connection-pooling/BRAINSTORM.md` — already pre-authorized the by-concern 2-leg split and chartered 43.2 at a high level (the `ClientConn` reuse + `max_concurrent_streams` enforcement + GOAWAY-driven rotation that ADR-0056 §Consequences named as its superseder). This leg brainstorm resolves the design questions the parent left to leg-time: the **multi-conn pool ambition**, the **stream-budget source**, the **stream-aware pending queue** (the load-bearing concurrency the parent did not specify), the **GOAWAY rotation discipline**, the **stat surface**, and the **ADR-0045 split posture**.

43.2 lands the project's **FIRST cross-request upstream connection reuse for HTTP/2** — it lifts the ADR-0056 `defer cc.Close()` per-request-fresh discipline and amortizes the H2 handshake across requests over a reused, multiplexed `ClientConn` pool. It builds **on top of** the 43.1 connection-pool budget substrate (`max_connections` hard-cap + `max_pending_requests` bounded wait-queue, ADR-0252): each NEW H2 connection consumes one `max_connections` permit; multiplexing a new stream onto an existing connection consumes none. When ROW 43's legs all land, the row flips `done` and the Upstream-robustness family (phases 39–43) CLOSES.

---

## 1. Mission and scope confirmation (43.2 only)

### 1.1 What phase 43.2 delivers (envelope: the H2 multiplex pool)

A per-endpoint pool of reusable, multiplexed upstream `*h2.ClientConn`s replacing the per-request-fresh dial. Concretely:

1. **Per-endpoint `ClientConn` pool** — a new `internal/cluster/h2pool.go` holding `c.h2Pool map[string][]*pooledH2Conn` on `Cluster` (keyed by `ep.Addr()`, guarded by `c.h2PoolMu`), symmetric with the existing per-endpoint `h1Pool`. A `pooledH2Conn` wraps an `*h2.ClientConn` + its in-flight stream count + a `draining` flag.
2. **`max_concurrent_streams` enforcement (peer-driven)** — the per-conn stream cap is the peer's advertised `SETTINGS_MAX_CONCURRENT_STREAMS` (already read into `cc.serverS.MaxConcurrentStreams` at `h2/client.go:~290`). The codec reads it today but does not enforce it on the CLIENT (outbound) side; 43.2 adds the per-conn active-stream count + the admission guard. The **local** half of `min(local, peer)` (`http2_protocol_options.max_concurrent_streams`) is DEFERRED (§8) — recorded in ADR-0253.
3. **The stream-aware pending queue** — when all conns for an endpoint are stream-saturated AND `max_connections` is exhausted, a request PENDS (bounded by `max_pending_requests`), woken on EITHER a stream completing on an existing conn OR a conn closing (the 43.1 permit release). Queue-full ⇒ fail-fast `503` + `upstream_rq_pending_overflow` (the shared 43.1 sentinel/counter).
4. **GOAWAY-driven conn rotation (lazy)** — a conn that has received GOAWAY (the `goawayCh` signal) is skipped by the admission scan and closed when its last in-flight stream finishes; a replacement is opened on-demand. Conns are also evicted on a `RoundTrip` transport error.
5. **Stats** — add `cluster.upstream_cx_http2_total` (+ `cluster.upstream_cx_http2_active`), the Envoy-faithful H2 conn counters; the exact `+N` pinned at SPEC via a live contrib probe (§10 D-H2-STATS).
6. **The router rewire** — `router_h2.go`'s two upstream-call sites (`doH2ClusterAction` + the legacy `doH2`) replace `DialH2(ctx)` + `defer cc.Close()` with `AcquireH2Stream(ctx)` + a `releaseStream` closure.

### 1.2 What phase 43.2 does NOT deliver (forward to §8)

- The **local** `http2_protocol_options.max_concurrent_streams` cap (peer-driven enforcement only at 43.2; `min(local,peer)` collapses to `peer`). DEFERRED — a recorded follow-up in ADR-0253.
- `max_requests_per_connection` (per-conn request budget → conn rotation on request count); H3/QUIC pooling; `max_connection_pools`; eager/pre-warmed connection creation; idle-connection timeout eviction (43.2 evicts on GOAWAY + transport error, not on idle).
- Any change to `max_connections` / `max_pending_requests` semantics (43.1's, reused verbatim) or to the H1 pool.

### 1.3 ADR-0045 split readiness — assessed, decision DEFERRED to SPEC

The full 43.2 envelope (per-endpoint pool management + peer-cap stream admission + the stream-aware pending queue + GOAWAY rotation + the H2 stats + the router rewire + a cross-side differential) is estimated at **≈300–400 prod LoC / ~14+ tasks**, with the stream-aware pending queue as the load-bearing concurrency risk (the 43.1-Task-3 analogue). This is at/near the ADR-0045 split gate. The brainstorm CHARTERS the full leg and **applies the formal split gate at the 43.2 SPEC** (the standard valve; the 42.2a/42.2b precedent). The anticipated split, if taken: **43.2a** = the multiplex pool substrate (per-endpoint pool + peer-cap admission + the stream-aware queue + the router rewire + stats; supersedes ADR-0056) and **43.2b** = GOAWAY rotation + conn eviction-on-error + the drain-close lifecycle + its differential. Row 43 then becomes a 3-leg row and flips `done` only when ALL legs land (`reference_roadmap_split_phase_row_done`).

### 1.4 Package placement + directory convention

Directory: `43.2-h2-connection-pool/` (this leg; the parent-pre-declared sub-directory, the 42.2-hedging precedent). The work lands in the EXISTING `internal/cluster` package (a new `h2pool.go` + the `Cluster.AcquireH2Stream`/`releaseStream` methods; the 43.1 permit seam reused) and the EXISTING `internal/filter/hcm/h2` package (the `ClientConn` liveness predicates + the client-side stream-count accounting) and touches the router (`router_h2.go`). ZERO new Go packages; ZERO new go.mod modules (the `h2` codec + `cluster.v3` are in-tree deps).

### 1.5 Relationship to the 43.1 substrate (a stream pool LAYERED over the conn-permit budget)

43.1 gives 43.2 the conn-creation permit (`acquireConnSlot`/`releaseConn`) + the wait-queue PATTERN (`sync.Mutex` + FIFO buffered-cap-1-channel wake). 43.2 adds a SECOND admission dimension — streams — above the conn dimension. The 43.1 permit gates conn CREATION (each new conn = one `max_connections` permit; conn-Close releases it). The 43.2 stream-aware queue gates stream ADMISSION and is woken on stream-free (no permit movement) as well as conn-close. The two dimensions compose: a request pends only when BOTH all conns are stream-saturated AND `max_connections` is reached.

---

## 2. Design decisions

### 2.1 Subject confirmation: the H2 multiplex pool — the pre-authorized 43.2 leg *(Q-subject)*

Confirmed via `reference_roadmap_split_phase_row_done`: 43.1 (budget substrate + pending wait-queue) LANDED (ADR-0252, commit `c8014f48`); 43.2 (the H2 multiplex pool) is the remaining leg. Row 43 flips `done` only when all its legs land. No new go.mod module. The subject SUPERSEDES ADR-0056 (per-request-fresh upstream H2 dial), as ADR-0056 §Consequences explicitly anticipated.

### 2.2 Ambition: a multi-conn, Envoy-faithful pool *(Q-ambition → Fork: MULTI-CONN)*

**Chosen: the multi-conn pool.** When an endpoint's existing `ClientConn`(s) are saturated at `min(local,peer)` `max_concurrent_streams`, a new request opens an ADDITIONAL `ClientConn` to the same endpoint (consuming a 43.1 `max_connections` permit); if `max_connections` is ALSO exhausted, the request pends on the stream-aware queue; queue-full ⇒ overflow 503. Chosen over **single-conn-per-endpoint multiplex** (rejected: underuses the `max_connections` budget 43.1 built precisely for this, and diverges from the reference's "open more conns when streams saturate" behavior) and **bounded fixed-N pool** (rejected: an arbitrary N that does not compose with the `max_connections` budget). The multi-conn model makes the 43.1 substrate the natural conn-count governor.

### 2.3 Stream budget: peer-driven now, local cap deferred *(Q-stream-budget → Fork: PEER-NOW-LOCAL-LATER)*

**Chosen: peer-driven enforcement now; the local cap deferred.** The per-conn stream cap = the peer's advertised `SETTINGS_MAX_CONCURRENT_STREAMS` (read into `serverS`), with a sane default when the peer is silent (the exact default pinned at SPEC, D-H2-DEFAULT). The local `http2_protocol_options.max_concurrent_streams` cap (→ `min(local,peer)`) is DEFERRED to a follow-up (§8), recorded in ADR-0253. **Finding that corrected the parent charter's sketch:** `circuit_breakers.max_requests` is ALREADY enforced cluster-wide on both router paths today (the `TryAcquireRequest()`/`ReleaseRequest()` admission wrapping each upstream call in `doH1ClusterAction` / `doH2ClusterAction`; line numbers approximate, symbol-relative) — it is the LIVE concurrent-request cap, NOT emit-0. Reusing it as the per-conn stream cap would double-count the same budget for two semantically distinct purposes. So the per-conn stream cap is peer-driven (a different, per-connection dimension), and `max_requests` stays as the cluster-wide request gate at router admission. The peer cap is also the most controllable knob for a cross-side EXACT differential (set it low on the backend; both sides observe it).

### 2.4 The stream-aware pending queue: woken on stream-free AND conn-close *(Q-stream-wake → Fork: STREAM-AWARE-QUEUE → the load-bearing decision)*

**Chosen: the H2 pool owns a per-endpoint stream-aware pending queue.** This is the crux the parent charter did not specify. NOTE — this is a SEPARATE queue instance from the 43.1 `connPool.waiters` FIFO (which the SPEC must not overload): the 43.1 queue's wake semantics are permit-handoff-on-conn-close ONLY, whereas the 43.2 H2 queue must additionally wake on stream-free. The H2 queue REUSES the 43.1 wake PATTERN (`sync.Mutex` + FIFO buffered-cap-1 channels) as a distinct per-endpoint structure in `h2pool.go`, it does not reuse the 43.1 `connPool`'s slice. In the multi-conn model a request pends only when all conns exist (`max_connections` reached) AND all are stream-saturated; from there, capacity frees mostly via stream-completion (the 43.1 permit wake fires only on conn-close, which is rare in steady GOAWAY-free load). A waiter woken ONLY by conn-close would STARVE while existing conns cycle streams. So the pool maintains a per-endpoint FIFO of pending requests (mirroring the 43.1 `sync.Mutex` + FIFO buffered-cap-1-channel wake PATTERN) woken on BOTH: (i) a stream completing on an existing conn → the freed slot is handed to the head waiter, which rides the EXISTING conn (no new conn, no permit), and (ii) a conn closing → the 43.1 permit is freed and the woken waiter may dial. The queue is bounded by `max_pending_requests`; queue-full ⇒ fail-fast 503 + `upstream_rq_pending_overflow`. Chosen over **overflow-immediately** (rejected: `max_pending_requests` would not cover the stream-saturation case — diverges from the reference's pending semantics) and **reuse-43.1-queue-as-is** (rejected: the 43.1 queue wakes only on permit/conn-close, causing stream-free starvation). The queue is the load-bearing concurrency unit and gets a 43.1-Task-3-style `-race` unit-test matrix at IMPL.

### 2.5 GOAWAY + error rotation: lazy *(Q-goaway → Fork: LAZY-ROTATION)*

**Chosen: lazy rotation.** New `ClientConn` liveness predicates — `goneAway()` (non-blocking `goawayCh` check) and `closed()` (`ctx.Err() != nil`). The admission scan skips a `goneAway()` conn (marking it `draining`) and, if it finds a `draining && inFlight==0` conn, closes+removes it eagerly (covers the idle-goaway'd-conn case). In-flight streams drain naturally — the codec already finishes streams > `LastStreamID` with the GOAWAY error (`client.go:~452`). A draining conn closes on its last `releaseStream` ⇒ its `connWithGauge.Close`→`connDec`→`releaseConn` releases the `max_connections` permit ⇒ wakes a conn-level waiter. A `RoundTrip` transport error evicts+closes that conn. Replacement conns open on-demand via the MISS path. Chosen over **eager replacement** (rejected: background-dial complexity + a harder-to-test timing surface for marginal latency gain) and **close-all-on-GOAWAY** (rejected: drops in-flight streams ≤ `LastStreamID` the peer promised to finish — loses graceful-drain parity).

### 2.6 Stat surface: add the H2 conn counters *(Q-stats → Fork: ADD-CX-HTTP2)*

**Chosen: add `cluster.upstream_cx_http2_total` (+ `cluster.upstream_cx_http2_active`).** These are the Envoy-faithful H2 connection counters the reference emits, so the cross-side differential can pin them directly. Conn/request counts otherwise reuse `upstream_cx_total/active` + the phase-41 budget. The exact `+N` (whether `_active` is also added, and the precise names/scoping) is pinned at SPEC via a live contrib-v1.37.2 probe (§10 D-H2-STATS) against the reference's emitted set. Anticipated surface delta: **1183 → 1184/1185**.

### 2.7 Request flow (the as-designed admission)

`AcquireH2Stream(ctx) → (cc *h2.ClientConn, releaseStream func(), ep Endpoint, err error)`:
1. **Stream HIT** — under `h2PoolMu`, scan the endpoint's conns for one that is live (`!goneAway() && !closed()`) and `inFlight < cap`; if found, `inFlight++`, return it + `releaseStream`. No permit.
2. **Conn MISS** — `acquireConnSlot` (43.1): permit granted ⇒ dial a fresh `ClientConn` (the `DialH2` internals, refactored for pool reuse), add to the pool, assign the stream; permit would exceed `max_connections` ⇒ the request PENDS on the stream-aware queue (§2.4); queue-full ⇒ 503.
3. **`releaseStream`** — `inFlight--`; if a waiter is queued, hand it the freed slot on THIS conn (no new conn); else if `draining && inFlight==0`, Close the conn (releases the permit → wakes a conn-level waiter).
4. The router consumes the response, then calls `releaseStream` (replacing `defer cc.Close()`).

---

## 3. Framework-survey result — a per-endpoint pool over the EXISTING 43.1 permit seam + the EXISTING h2 codec + 0 new packages + 0 new go.mod modules

- **NEW packages:** NONE.
- **go.mod modules:** anticipated ZERO new (re-pinned at SPEC).
- **REUSES:** the 43.1 `acquireConnSlot`/`releaseConn` permit seam + the `connWithGauge`/`connDec` conn-Close release path (ADR-0252); the 43.1 wait-queue PATTERN (`sync.Mutex` + FIFO buffered-cap-1 wake) for the stream-aware queue; the `h2.ClientConn` multiplex machinery (monotonic `nextStreamID`, concurrent `RoundTrip`, `serverS.MaxConcurrentStreams`, `goawayCh`); the per-endpoint pool shape of `h1Pool`/`PutIdleH1`; the `cluster.IsConnPoolOverflow` sentinel + `upstream_rq_pending_overflow` counter; the router `*ClusterAction` dispatch + the `IsConnPoolOverflow → 503` branch already wired at the H2 site (43.1 Task 7).

---

## 4. Per-route / per-cluster applicability

The pool is a per-cluster, per-endpoint cluster subsystem (not a per-route surface). It engages for any H2-upstream cluster (`http2_protocol_options`). `max_connections`/`max_pending_requests` come from the cluster's `circuit_breakers` (43.1); the per-conn stream cap comes from the peer SETTINGS. Byte-neutral for non-H2 clusters (H1 path unchanged) and for H2 clusters with no `circuit_breakers` (default budgets 1024 — effectively unbounded conn growth, single-stream-cap multiplex).

---

## 5. Stat surface hypothesis — anticipated +1/+2 (`upstream_cx_http2_total` [+ `_active`])

### 5.1 New stat names (SPEC pins, D-H2-STATS)
- `cluster.<name>.upstream_cx_http2_total` (counter) — H2 connections created.
- `cluster.<name>.upstream_cx_http2_active` (gauge) — H2 connections currently open (candidate; confirm the reference emits it at the SPEC probe).
- Anticipated surface: **1183 → 1184/1185**. Exact `+N` pinned at SPEC against the reference's emitted set.

### 5.2 envoy-go-strict departure flags (anticipated)
- None new expected (the overflow 503 reuses the 43.1 `upstream_rq_pending_overflow`; no response-flag plumbing, consistent with the 43.1 `UO`-deferred posture). Re-confirmed at SPEC/IMPL.

---

## 6. Differential fixture envelope — anticipated +1 (`0079`)

### 6.1 `0079-h2-multiplex-pool` (+1)
A cross-side fixture with an H2 backend whose `SETTINGS_MAX_CONCURRENT_STREAMS` is set LOW and controllable, proving: (i) multiplex — many concurrent streams ride FEWER connections than requests (`upstream_cx_http2_total`/`upstream_cx_total` stays small while the downstream request count is high); (ii) the peer stream cap is enforced; (iii) conn-growth up to `max_connections`; (iv) the stream-aware pending queue + overflow→503. Per `reference_max_connections_soft_breaker`, `max_concurrent_streams` is CLEANLY enforced by the H2 flow-control mechanism (not a soft breaker), so this differential may be **cross-side EXACT on stream counts** — unlike 43.1's soft `max_connections`. The concurrency discipline carries forward: a release-barrier hold backend + poll-the-gauge + sequential-per-side, NEVER a `time.Sleep` (`reference_concurrency_differential_release_barrier`); assert the DOWNSTREAM response class, not the over-counting upstream class (`reference_concurrent_attempt_downstream_class_assertion`).

### 6.2 New BackendKind: anticipated ONE (an H2 hold backend)
The 43.1 `BlockingHoldResponder` (kind 36) is H1. The multiplex proof needs an H2-capable hold-and-release backend (an H2 backend that holds streams open on a gate, controllable `SETTINGS_MAX_CONCURRENT_STREAMS`). Whether this is a new BackendKind (37) or an extension of an existing H2 backend is a SPEC/PLAN call (D-H2-BACKEND). The parent brainstorm flagged this (D-CP-BACKEND).

### 6.3 New fuzzer: anticipated NONE (pool management, not a new wire decoder).

---

## 7. Anticipated ADR — ADR-0253 (SUPERSEDES ADR-0056)

ADR-0253 (next-free; reserved at 43.1) records: the per-endpoint H2 multiplex pool; peer-driven `max_concurrent_streams` enforcement (local cap deferred); the stream-aware pending queue (woken on stream-free + conn-close); lazy GOAWAY rotation + error eviction + drain-close-on-last-stream; the `upstream_cx_http2_*` stats; and the **supersession of ADR-0056** (per-request-fresh upstream H2 dial). If the leg sub-splits at SPEC, 43.2a and 43.2b each anchor their ADR per ADR-0044 (ADR-0253 + a possible ADR-0254). The ADR §Context drafts at SPEC; §Decision/§Consequences land at IMPL.

---

## 8. Deferred items

- **Local `http2_protocol_options.max_concurrent_streams` cap** (→ `min(local,peer)`) — peer-driven only at 43.2.
- `max_requests_per_connection` (per-conn request budget + conn rotation on count).
- Idle-connection timeout eviction (43.2 evicts on GOAWAY + transport error only).
- Eager / pre-warmed connection creation; `max_connection_pools`.
- H3/QUIC pooling (a future family's reuse of this pool shape).
- HIGH-priority connection-budget binding (DEFAULT-only, inherited from 43.1).

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- **ADR-0056** (per-request-fresh upstream H2 dial) — SUPERSEDED by 43.2 (the explicit superseder ADR-0056 §Consequences named).
- **Phase 05.2 SPEC §2.1** (upstream H2 stream pooling/multiplexing deferred to the upstream-robustness family) — discharged.
- **M-12** (05.1 REVIEW — `closedStreams` unbounded-growth under long-lived conns) — becomes load-bearing at 43.2 (conns are now long-lived across many request lifetimes); 43.2 must address `closedStreams` bounding (a SPEC item, D-H2-CLOSEDSTREAMS).
- **Phase 41** `max_connections`/`max_pending_requests` deferral — discharged at 43.1; 43.2 reuses the substrate.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

- **D-H2-STATS** — the exact `upstream_cx_http2_*` shapes the reference emits (does it emit `_active`? `_total` only? other H2 cx shapes?) → the precise `+N`. Live probe per `reference_docker_probe_bridge_network`.
- **D-H2-DEFAULT** — the per-conn stream cap when the peer is silent on `SETTINGS_MAX_CONCURRENT_STREAMS` (the codec default vs a pool default; the reference's behavior).
- **D-H2-BACKEND** — the H2 hold-and-release differential backend (new BackendKind vs extend an existing H2 backend; controllable `SETTINGS_MAX_CONCURRENT_STREAMS`).
- **D-H2-CLOSEDSTREAMS** — the `closedStreams` map bounding under long-lived pooled conns (M-12).
- **D-H2-SPLIT** — the formal ADR-0045 split-gate re-check (43.2 flat vs 43.2a/43.2b) once the SPEC scopes the as-designed size.
- **D-H2-EXACT** — confirm the stream-count differential is cross-side EXACT (the clean flow-control enforcement) vs needing a robust prong (the 43.1 soft-breaker contrast).

---

## 11. Prior-phase lessons applied

- `reference_max_connections_soft_breaker` — `max_concurrent_streams` is NOT soft (flow-control enforced); the 43.2 differential can be cross-side EXACT on stream counts (contrast with 43.1).
- `reference_concurrency_differential_release_barrier` + `reference_concurrent_attempt_downstream_class_assertion` — the hold-backend + poll-the-gauge + sequential-per-side discipline; assert the downstream response class.
- `reference_roadmap_split_phase_row_done` — row 43 flips `done` only when ALL legs land (43.1 + 43.2[a/b]).
- `reference_docker_probe_bridge_network` — the live contrib probe for the §10 D-H2-* pins.
- The 43.1 Task-3 precedent — the stream-aware queue is the load-bearing concurrency unit; insist on a `-race -count=1` unit matrix at IMPL.
- `reference_fuzzer_count_docs_drift` — the documented fuzzer running total (42) is off-by-one from the actual 43; reconcile if 43.2 ever touches the fuzzer count (it does not).

---

## 12. Section closeout

The 43.2 leg is chartered: an Envoy-faithful multi-conn H2 multiplex pool over the 43.1 permit substrate, with peer-driven `max_concurrent_streams` enforcement, a stream-aware pending queue (woken on stream-free + conn-close), lazy GOAWAY rotation + error eviction, and the `upstream_cx_http2_*` stats — superseding ADR-0056 (ADR-0253). The ADR-0045 split gate is applied at the 43.2 SPEC (anticipated 43.2a/43.2b). Next session: the 43.2 SPEC (executes the §10 D-H2-* empirical pins in-session against `envoyproxy/envoy:contrib-v1.37.2`; anchors the ADR-0253 §Context draft; applies the split gate).
