# Phase 43.2b SPEC — graceful GOAWAY-driven connection rotation: the H2-multiplex-pool drain lifecycle + the behavior-driven `http2.*` reset stats — the SECOND-and-FINAL sub-leg of the SECOND-and-FINAL leg of the FIFTH-and-FINAL Upstream-robustness-family row (ANCHORS ADR-0254)

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-43.2b BRAINSTORM (`docs/envoy-go/phases/43.2-h2-connection-pool/BRAINSTORM-43.2b.md`, commit `b8ec7b4a`). This SPEC charters phase **43.2b** — the graceful GOAWAY-driven connection-rotation leg the 43.2a substrate (ADR-0253) DEFERRED: the codec-visible peer-GOAWAY signal, a per-conn drain-watcher, the admission/eviction extensions (skip draining; close on last stream), lazy replacement via the established MISS path, and the FIRST codec→cluster stat wiring for the behavior-driven `http2.*` reset subset. Counts at SPEC commit UNCHANGED (stat surface **1185** / fixtures **81** / fuzzers **42** documented [actual `^func Fuzz` **43**, `reference_fuzzer_count_docs_drift`] / BackendKind tail **37** / DECISIONS tail **ADR-0253**, next-free **ADR-0254**). The §11 D-H2B-* empirical pins were EXECUTED IN-SESSION (2026-06-24) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Complete the HTTP/2 upstream multiplex pool's lifecycle: when a pooled upstream connection receives a peer GOAWAY, mark it draining, take NO new streams on it, drain its in-flight streams to completion, then close it — and open a replacement lazily on the next demand. This is the Envoy-faithful graceful-rotation behavior the 43.2a substrate (ADR-0253) explicitly deferred (43.2a ships MINIMAL liveness only: `Closed()`-skip + evict-on-`RoundTrip`-error). The codec already OBSERVES a peer GOAWAY (it closes `goawayCh` and finishes streams above `LastStreamID`) but does NOT cancel the conn ctx, so a GOAWAY'd-but-alive conn is INVISIBLE to the 43.2a admission scan (`Closed()` stays false) and keeps taking new streams — the gap 43.2b closes. It SUPERSEDES nothing new (ADR-0253 already superseded ADR-0056); it ANCHORS ADR-0254 and CLOSES the H2-multiplex-pool lifecycle.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live probe (2026-06-24, contrib-v1.37.2 — an H1 downstream → h2c upstream cluster with a raw-framer GOAWAY/RST-emitting backend on a shared bridge) drove these amendments. **The headline (AMEND-H2B-1):** the BRAINSTORM (§2.5) anticipated a behavior-driven `http2.*` subset of FOUR live counters (`goaway_sent`/`rx_reset`/`tx_reset`/`stream_refused_errors`, ~5 names → ~1190). The live probe REVERSES that to TWO: a peer GOAWAY is observed ENTIRELY via the connection-lifecycle counters (`upstream_cx_close_notify` + `upstream_cx_destroy_local`), with **NO `http2.*` counter moving during a rotation** — and the reference has **no `goaway_received` counter at all** at the cluster scope.

- **AMEND-H2B-1 (the rotation observable — connection-lifecycle counters, NOT a goaway stat; NO `goaway_received` counter exists). USER-CONFIRMATION PENDING.** Live finding (D-H2B-ROTATION / D-H2B-GOAWAYSENT): when the upstream backend sends a peer GOAWAY, the reference marks the conn draining (`upstream_cx_close_notify` ++ IMMEDIATELY on receipt) and — for an IDLE conn — closes it promptly (`upstream_cx_destroy` ++, `upstream_cx_destroy_local` ++, `upstream_cx_active` --); for an IN-FLIGHT conn it holds the conn open until the LAST in-flight stream completes, then closes it (`upstream_cx_destroy_local` ++, `upstream_cx_destroy_local_with_active_rq` STAYS 0 — a graceful drain, not a forced kill). A replacement conn opens LAZILY on the next request (`upstream_cx_http2_total` ++, `upstream_cx_total` ++, `upstream_cx_active` back to 1). Throughout a clean rotation, **`http2.goaway_sent` STAYS 0** and **no `http2.*` reset counter moves**. The reference's 23-name `cluster.<name>.http2.*` subtree contains `goaway_sent` but NO `goaway_received` / `goaway_rx` — a received peer GOAWAY is NOT counted by any `http2.*` stat, only by the `upstream_cx_*` close/destroy sequence. ⇒ 43.2b registers NO new GOAWAY-named stat; the rotation is proven via the existing `upstream_cx_http2_total` (43.2a) + the base `upstream_cx_*` lifecycle counters.
- **AMEND-H2B-2 (the reset stat subset — `http2.rx_reset` + `http2.tx_reset` only; +2 → 1187).** Live finding (D-H2B-STATS): of the four brainstorm candidates, only TWO are LIVE-and-cross-side-assertable: `http2.tx_reset` (RST_STREAM the codec SENDS — confirmed: a downstream cancel of a held request drives `http2.tx_reset` ++ alongside `upstream_rq_tx_reset` ++; envoy-go's codec already emits `WriteRSTStream(id, CANCEL)` at `RoundTrip`'s `ctx.Done` site) and `http2.rx_reset` (RST_STREAM the codec RECEIVES — confirmed: a backend RST_STREAM(INTERNAL_ERROR) on an in-flight stream drives `http2.rx_reset` ++ + `upstream_rq_rx_reset` ++ + a downstream 503, conn survives; envoy-go's codec receives RST at `dispatchFrame`'s `RSTStreamFrame` case). 43.2b ADDS exactly these two `http2.*` counters. Surface **1185 → 1187** (+2). Both useH2-gated (registered ONLY on a `useH2()` cluster; non-H2 byte-stable), with a registration test analog to `TestRegisterClusterMetrics_H2Stats`.
- **AMEND-H2B-3 (`goaway_sent` + `stream_refused_errors` DEFERRED — recorded hardening items).** Live finding: (a) `http2.goaway_sent` is NOT incremented by the reference on a drain-close (it stayed 0 through a full local-destroy rotation) — the reference's upstream (client) codec does not count a `goaway_sent` on graceful conn close. envoy-go's `Close()` DOES emit GOAWAY(NO_ERROR), so driving `goaway_sent` from the pool's drain-`Close()` would make envoy-go read non-zero where the reference reads zero — a DIVERGENCE. `goaway_sent` is therefore NOT registered (its only faithful site, `emitGoaway` on a peer protocol violation, is not exercised by the rotation differential ⇒ it would register dead-at-0). (b) `http2.stream_refused_errors` (RST received with REFUSED_STREAM) is REFERENCE-DORMANT: the reference proactively respects the peer's advertised `SETTINGS_MAX_CONCURRENT_STREAMS` AND opens a fresh conn per concurrent cold request (a connect-prefetch), so it essentially never RECEIVES a REFUSED_STREAM — a cross-side EXACT assertion would be vacuous-at-0. Both are recorded hardening items (§2); the `min(local,peer)` avoidance (AMEND-H2-5, inherited-deferred) is the natural home for the `stream_refused_errors` observable when that lands.
- **AMEND-H2B-4 (the differential is cross-side EXACT under the sequenced-barrier drive — no robust fallback needed).** Live finding (D-H2B-EXACT): the rotation, driven as discrete poll-to-converge phases behind release barriers, is deterministic on every count — `upstream_cx_http2_total == 2` after one rotation (the drained conn + its lazy replacement), `upstream_cx_active` back to the steady state, `http2.streams_active` back to 0, and the dedicated reset prongs (`http2.rx_reset == 1` after a backend-RST step, `http2.tx_reset == 1` after a downstream-cancel step). The brainstorm's subject-EXACT + cross-ROBUST fallback (§2.7) is NOT needed: the sequencing quiesces the pool before each trigger, so the GOAWAY lands relative to a settled pool, not a racing one.

### 1.2 ADR continuity + D-disposition at SPEC commit

- **ADR-0254** (next-free) — the 43.2b GOAWAY-rotation architecture; §Context drafted here (§13), §Decision/§Consequences land at the 43.2b IMPL (ADR-0044). ANCHORS the leg that completes the H2-multiplex-pool lifecycle ADR-0253 began.
- D-H2B-STATS / D-H2B-ROTATION / D-H2B-EXACT / D-H2B-GOAWAYSENT: PINNED at this SPEC (§11). D-H2B-{BACKEND, WATCHER-LIFECYCLE, WIRING, SPLIT-FINAL, FUZZER-RECONCILE, BYTESTAB, CXSTATS}: PLAN/IMPL pins (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §1.2 + the §1.1 amendments)

- **`http2.goaway_sent` registration** (AMEND-H2B-3) — reference reads 0 on drain-close; envoy-go's `Close()`-emitted GOAWAY would diverge. NOT registered.
- **`http2.stream_refused_errors` registration** (AMEND-H2B-3) — reference-dormant (it avoids receiving REFUSED_STREAM). NOT registered; its observable lands with `min(local,peer)`.
- **`min(local,peer)` proactive `max_concurrent_streams` enforcement** (AMEND-H2-5, inherited-deferred) — 43.2b keeps the 43.2a posture: enforce the LOCAL cap, let a peer-stricter `REFUSED_STREAM` surface via the codec's existing RST path. A recorded hardening item.
- **`nextStreamID` 2^31-exhaustion retirement** (the M-12 / Task-11 follow-up) — needs ~2^31 streams to trigger; undifferentiable. A recorded hardening item.
- `max_requests_per_connection`; idle-connection-timeout eviction; eager/pre-warmed conn creation; `max_connection_pools`; H3/QUIC pooling — all inherited-deferred from 43.2a. No change to the 43.1 `max_connections` / `max_pending_requests` semantics or to the H1 pool.

---

## 3. The drain lifecycle — `internal/cluster/h2pool.go` + `internal/filter/hcm/h2/client.go` (ADR-0254)

### 3.0 Split disposition — sub-leg 43.2b of the 43.2 split; FINAL ADR-0045 re-check at PLAN

43.2b is chartered here as ONE flat leg (the brainstorm's CORE-ONLY scope decision; §1.3 of the BRAINSTORM). Envelope estimate: ≈150–220 prod LoC / ~8–10 tasks — under the ADR-0045 soft gate. The PLAN does the final re-check (the 43.1 D-S431-7 / 43.2a D-H2-SPLIT-FINAL precedent) but 43.2b ships as one leg. **Row 43 flips `done` at this leg's IMPL** (ALL legs 43.1 + 43.2a + 43.2b landed) ⇒ the Upstream-robustness family (phases 39–43) CLOSES.

### 3.1 The codec-visible GOAWAY signal (`internal/filter/hcm/h2/client.go`)

The codec already closes `goawayCh` in `dispatchFrame`'s `GoAwayFrame` case (and finishes streams above `LastStreamID`) but does NOT `cc.cancel()`, so `Closed()` (`= cc.ctx.Err() != nil`) stays false. 43.2b exposes the signal to the pool WITHOUT cancelling the ctx (an in-flight stream below `LastStreamID` must still complete):

- `GoneAway() bool` — reports whether the peer GOAWAY has been observed (a non-blocking read of `goawayCh`'s closed state, e.g. `select { case <-cc.goawayCh: return true; default: return false }`). The admission scan uses it exactly as it uses `Closed()` today.
- `GoneAwayCh() <-chan struct{}` — returns `cc.goawayCh` for the drain-watcher to `select` on.

No new codec STATE — both are read-only accessors over the existing `goawayCh`. (The two reset counters of §3.4 are injected handles, not new codec state beyond the handle fields.)

### 3.2 Admission + eviction extensions (`internal/cluster/h2pool.go`)

- **Admission skip (draining):** the two admission sites that ride an ESTABLISHED pooled conn — `findStreamHitLocked` (`!pc.cc.Closed() && pc.inFlight < C`) and `h2PromoteLocked`'s stream-slot branch — each add `&& !pc.cc.GoneAway()` so a draining conn takes NO new streams. NO new `pooledH2Conn` field — `draining` is DERIVED from `cc.GoneAway()`, exactly as `Closed()` is derived today (the single-mutator `h2PoolMu` surface is unchanged; D-H2-MUTEX preserved). The connecting tier (`findConnectingHitLocked`) is DELIBERATELY EXCLUDED: a `connectingH2Conn` found there has `cc == nil` (the `cc` field is set only at conversion in `dialAndPool`, AFTER `removeConnectingLocked` drops the entry from the connecting set), and an unestablished conn cannot yet have observed a peer GOAWAY — a `!cn.cc.GoneAway()` guard there would both nil-dereference and be semantically vacuous, so it is omitted (an attacher rides the establishing conn; if THAT conn later drains, the watcher + the stream-HIT skip handle it).
- **Eviction condition (close on last stream):** `makeRelease`'s eager-close check generalizes from `pc.cc.Closed() && pc.inFlight == 0` to `(pc.cc.Closed() || pc.cc.GoneAway()) && pc.inFlight == 0`. The IN-FLIGHT draining path thus closes on the last release (matching the reference: conn held until the last in-flight stream completes, then `cx_destroy_local`); the IDLE draining path is handled by the watcher (§3.3).
- **Lazy replacement (inherited LAZY posture):** no background dial. A skipped draining conn → the next acquire MISSes → dials a replacement via the established MISS path (`upstream_cx_http2_total` ++). Permit + LB-release conservation UNCHANGED — draining-eviction `Close()`s the conn → `connDec` → `releaseConn` frees the `max_connections` permit, exactly as the 43.2a closed-conn case does; `h2PromoteLocked` re-homes the freed permit.

### 3.3 The per-conn drain-watcher goroutine

Spawned at pool insertion — in `dialAndPool`, AFTER `c.h2Pool[addr] = append(c.h2Pool[addr], pc)` (the line that appends the converted `pooledH2Conn`). One goroutine per pooled conn:

```
go func() {
    select {
    case <-cc.GoneAwayCh():   // peer GOAWAY observed
        c.h2PoolMu.Lock()
        if pc := c.findPooledLocked(addr, cc); pc != nil && pc.inFlight == 0 {
            c.evictH2ConnLocked(addr, pc)   // IDLE draining conn → close promptly
            c.h2PromoteLocked(addr)         // the freed permit may satisfy a waiter
        }
        // else: in-flight → the last releaseStream (the §3.2 generalized check) closes it.
        c.h2PoolMu.Unlock()
    case <-cc.ctx.Done():     // conn closed/evicted by another path → exit, no action
    }
}()
```

Rationale (the brainstorm's Q-drain-detection → WATCHER, §2.3): an IDLE conn that receives GOAWAY has no `releaseStream` to ever fire and its ctx is never cancelled, so a poll-on-admission scheme would skip it forever but never close it (a slow conn leak). The watcher closes it promptly. The watcher MUST re-check `findPooledLocked != nil` under `h2PoolMu` before evicting, to dodge the double-evict race with `EvictH2ConnOnError` / `makeRelease` (D-H2B-WATCHER-LIFECYCLE). `cc.ctx.Done()` is the natural "evicted by another path" signal (`Closed()` flips precisely when `cc.cancel()` runs on `Close()`/transport error), so no separate pool-owned done channel is needed; the watcher leaks no goroutine and races no double-evict.

### 3.4 Codec→cluster stat wiring — inject `http2.*` reset-counter handles at dial (the FIRST such wiring)

43.2a drove ALL its stats from the POOL layer (`h2StreamsActiveInc`/`h2CxHTTP2TotalInc` at acquire/release) — the codec was never cluster-aware. But the reset EVENTS happen IN the codec, so 43.2b introduces the FIRST codec→cluster stat wiring: the pool injects the cluster's `http2.rx_reset` + `http2.tx_reset` counter handles into the `ClientConn` at dial (the `dialPooledH2To` → `NewClientConn` seam). The codec increments at its existing event sites:

- `http2.rx_reset` ++ in `dispatchFrame`'s `RSTStreamFrame` case (a received RST_STREAM, the `cs.finish(streamError(...))` site).
- `http2.tx_reset` ++ in `RoundTrip`'s `ctx.Done` case, at `cc.fr.WriteRSTStream(id, http2.ErrCodeCancel)` (a sent RST_STREAM).

The exact handle-passing mechanism (a `NewClientConn` constructor param vs a post-construction setter; whether the kept single-shot `DialH2` path wires the handles or passes nil) is a PLAN detail (D-H2B-WIRING); the SPEC fixes the SEAM (inject at dial; nil-guarded increments so a non-pooled / non-wired conn is safe). Chosen over codec-side atomic accumulators the pool reads-and-deltas (lossy for live reset counters — a counter reflecting every RST cannot be reconstructed from a close-time delta; AMEND/BRAINSTORM §2.6).

### 3.5 Byte-stability

Byte-identical for non-H2 clusters (untouched) and for H2 clusters with no rotation/reset events (the two new counters stay 0; the admission/eviction extensions are no-ops when no conn is `GoneAway()`). The full differential (81-dir today → 82-dir with `0080`) must stay GREEN; the existing H2 fixtures `0004` (probes `/ready`, not the pool) and `0079` (the EXACT multiplex prong — does NOT trigger GOAWAY) are unaffected (D-H2B-BYTESTAB). The drain extensions add NO observable on a no-GOAWAY path.

---

## 4. Framework primitives — 0 new packages + 0 new go.mod deps

- REUSED: the 43.2a pool (`h2Pool`/`pooledH2Conn`/`AcquireH2Stream`/`makeRelease`/`evictH2ConnLocked`/`EvictH2ConnOnError`/`findPooledLocked`/`h2PromoteLocked`); the 43.1 permit seam (`tryAcquireConnSlot`/`releaseConn`/`connDec`) + the `connWithGauge` Close-release; the codec's existing `goawayCh` + RST send/receive sites; the `reference_docker_probe_bridge_network` differential pattern.
- NEW: the `GoneAway()` / `GoneAwayCh()` codec accessors; the per-conn drain-watcher in `dialAndPool`; the `&& !pc.cc.GoneAway()` admission-skip + the `(Closed()||GoneAway())` eviction generalization; the two `http2.rx_reset`/`http2.tx_reset` registrations + the codec→cluster handle injection.
- ZERO new Go packages; ZERO new go.mod modules (`h2` is an in-tree dep; `go mod tidy -diff` anticipated EMPTY).

---

## 5. Proto-field roster — NO new fields (per §11 D-H2B-STATS)

43.2b adds NO proto-field parse. The drain lifecycle is driven entirely by the runtime peer-GOAWAY signal; no `circuit_breakers` / `http2_protocol_options` field is newly read. `go mod tidy -diff` EMPTY.

## 6. PARSE-REJECT roster — NO new reject arm; NO new fuzzer

No new config surface ⇒ no new PARSE-REJECT arm and NO new fuzzer (the drain lifecycle is wire/runtime behavior, not config-parse). **D-H2B-FUZZER-RECONCILE:** 43.2b adds no fuzzer, but as the family-closing leg it is the natural point to reconcile the documented-42-vs-actual-43 `^func Fuzz` drift (`reference_fuzzer_count_docs_drift`) — a PLAN call (reconcile the running total to 43 here, or carry the note forward one more time).

## 7. Stat surface — add 2 (1185 → 1187) (per §11 D-H2B-STATS + AMEND-H2B-2)

- `cluster.<name>.http2.rx_reset` — counter, ++ per RST_STREAM the upstream codec RECEIVES (the `dispatchFrame` `RSTStreamFrame` site). useH2-gated.
- `cluster.<name>.http2.tx_reset` — counter, ++ per RST_STREAM the upstream codec SENDS (the `RoundTrip` `ctx.Done` CANCEL site). useH2-gated.
- NO `http2.goaway_sent` (reference reads 0 on drain-close — AMEND-H2B-3) and NO `http2.stream_refused_errors` (reference-dormant — AMEND-H2B-3). NO `goaway_received` exists in the reference's 23-name `http2.*` subtree (AMEND-H2B-1). The rotation itself reuses the 43.2a `upstream_cx_http2_total` + the base `upstream_cx_*` lifecycle counters. Surface **1185 → 1187** (+2); EXACT figure confirmed against the IMPL registration test (`TestRegisterClusterMetrics_H2Stats` extended; an H2 cluster emits 1187, a non-H2 cluster stays 1183).

## 8. Differential fixture taxonomy (+1: `0080` cross-side EXACT)

### 8.1 `0080-h2-goaway-rotation` (cross-side EXACT — sequenced-barrier)
An H2 (TLS+ALPN-h2) DOWNSTREAM listener (the 0004 PKI shape — MANDATORY: the H2 pool engages ONLY on an H2 downstream, `reference_h2_pool_downstream_codec_gate`; an H1-downstream variant never exercises the pool) → an h2c upstream cluster `c_h2gw` with `http2_protocol_options{ max_concurrent_streams: C }`, on BOTH subject and reference (`contrib-v1.37.2`). A GOAWAY/RST-capable h2c backend (D-H2B-BACKEND — extend `H2HoldResponder` kind 37 with `/__goaway` + `/__rst` gates, vs a new kind 38; PLAN call) holds streams on a gate and emits GOAWAY(NO_ERROR) / RST_STREAM on control. SLEEPLESS (release-barrier + poll-to-converge; sequential-per-side, `reference_concurrency_differential_release_barrier`). Staged drive:

1. **Establish** — fire a held request ⇒ poll until `upstream_cx_http2_total == 1` AND `http2.streams_active == 1`.
2. **Drain (in-flight path)** — barrier ⇒ trigger the backend GOAWAY (honoring the in-flight stream's id) ⇒ poll: the conn STAYS active while its stream is in flight (`upstream_cx_active == 1`, `streams_active == 1`), takes NO new stream (admission-skip), then on release closes (`upstream_cx_active → 0`). A fresh request meanwhile MISSes ⇒ a REPLACEMENT conn (`upstream_cx_http2_total → 2`).
3. **Drain (idle path)** — establish a second conn, let it go idle, trigger its GOAWAY ⇒ poll it closed PROMPTLY (the watcher; `upstream_cx_active` drops without a release driving it).
4. **rx_reset prong** — fire a held request, trigger a backend RST_STREAM on it ⇒ poll `http2.rx_reset == 1` + a downstream 503 (assert the DOWNSTREAM class per `reference_concurrent_attempt_downstream_class_assertion`); the conn survives.
5. **tx_reset prong** — fire a held request, cancel it DOWNSTREAM (close the downstream stream) ⇒ poll `http2.tx_reset == 1` (the codec's CANCEL to the upstream).
6. **Quiesce** — release barrier ⇒ all held drain to 200 ⇒ gauges (`http2.streams_active`, `upstream_cx_active`) poll back to the steady state.

Cross-side EXACT (D-H2B-EXACT — sequenced determinism): `upstream_cx_http2_total`, `http2.streams_active`, `http2.rx_reset == 1`, `http2.tx_reset == 1`. The base `upstream_cx_*` close/destroy lifecycle counters (`cx_close_notify` / `cx_destroy_local`) GUIDE the reference's observed rotation but are asserted cross-side ONLY for the counters envoy-go also emits (D-H2B-CXSTATS — a PLAN/IMPL confirm of which `upstream_cx_*` close/destroy counters envoy-go exposes; the EXACT prong's load-bearing assertions are `upstream_cx_http2_total` + the two reset counters, which both sides emit).

### 8.2 BackendKind / fuzzer
D-H2B-BACKEND: the GOAWAY/RST-capable h2c backend (controllable GOAWAY + RST_STREAM gates over the 43.2a hold machinery) is a PLAN/IMPL deliverable — anticipated either an EXTENSION of `H2HoldResponder` (kind 37; favored — reuses the hold/gate/SETTINGS machinery) OR a new BackendKind **38**. NO new fuzzer (fuzzers STAY 42 documented / 43 actual; reconcile per D-H2B-FUZZER-RECONCILE).

## 9. Behavior-contract delta (the 43.2b bundle; ADR-0254 atomic landing)

Extend the `### Cluster — HTTP/2 upstream multiplex connection pool` subsection (43.2a) with the drain lifecycle: peer-GOAWAY → draining (admission-skip via `GoneAway()`); in-flight drain-close-on-last-stream + idle prompt-close (the per-conn watcher); lazy replacement via the MISS path; the two new behavior-driven `http2.rx_reset`/`http2.tx_reset` counters (the first codec→cluster stat wiring); the rotation observed via `upstream_cx_http2_total` + the `upstream_cx_*` lifecycle counters (NO new GOAWAY stat — AMEND-H2B-1). The stat-surface block advances 1185 → 1187. ADR-0254 lands atomically with this contract delta at the 43.2b IMPL.

## 10. Per-task structure (~8–10 tasks; PLAN decomposes)

PROGRESS+baselines + the final ADR-0045 re-check (D-H2B-SPLIT-FINAL); the `GoneAway()`/`GoneAwayCh()` codec accessors (`-race`); the admission-skip + eviction generalization (`(Closed()||GoneAway()) && inFlight==0`); the per-conn drain-watcher in `dialAndPool` (LOAD-BEARING, `-race -count=1` — the double-evict re-check); the two `http2.rx_reset`/`http2.tx_reset` registrations (1185→1187) + the codec→cluster handle injection + registration test; the GOAWAY/RST-capable backend (D-H2B-BACKEND); the `0080` cross-side EXACT fixture (sequenced barriers); `0080` deliberate breaks (`-count=1`) + 20/20 flake + `-race`; full 82-dir differential + six-gate; ADR-0254 body + BEHAVIOR_CONTRACT; completion bundle (ROADMAP row 43 → done; the Upstream-robustness family CLOSES).

## 11. SPEC-time empirical-pin block (D-H2B-* — executed IN-SESSION 2026-06-24)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared bridge `h2bprobe`; an H1 downstream → h2c upstream cluster `c_h2` with `http2_protocol_options.max_concurrent_streams: 100`; a raw-`golang.org/x/net/http2`-framer h2c backend with `/goaway` + `/rst_last` control gates; sequenced single-shot + `/stats`-delta scrapes; request path verified `upstream_cx_http2_total > 0` + a 200 round-trip).

| Pin | Result |
|-----|--------|
| **D-H2B-STATS** | PINNED (AMEND-H2B-2). The reference's `cluster.<name>.http2.*` subtree = **23 names**; the reset/goaway candidates `goaway_sent` / `rx_reset` / `tx_reset` / `stream_refused_errors` all EXIST. Of these, `tx_reset` (downstream-cancel → CANCEL) and `rx_reset` (backend RST → 503, conn survives) are LIVE + cross-side assertable; envoy-go's codec already has both event sites. 43.2b registers those TWO (+2 → **1187**). `goaway_sent` + `stream_refused_errors` DEFERRED (AMEND-H2B-3). NO `goaway_received` exists. |
| **D-H2B-ROTATION** | PINNED (AMEND-H2B-1). Peer GOAWAY → `upstream_cx_close_notify` ++ immediately (drain mark). IDLE conn ⇒ prompt local close (`upstream_cx_destroy` / `upstream_cx_destroy_local` ++, `upstream_cx_active` --). IN-FLIGHT conn ⇒ held open until the last stream completes, then `upstream_cx_destroy_local` ++ (`_with_active_rq` STAYS 0 — graceful). Replacement is LAZY on the next request (`upstream_cx_http2_total` 1 → 2). NO `http2.*` counter moves during a clean rotation. |
| **D-H2B-EXACT** | PINNED (AMEND-H2B-4). Sequenced-barrier drive ⇒ deterministic: `upstream_cx_http2_total == 2` after one rotation, `upstream_cx_active`/`http2.streams_active` back to steady state, `http2.rx_reset == 1` / `http2.tx_reset == 1` on their dedicated prongs. Cross-side EXACT — NO subject-EXACT + cross-ROBUST fallback needed. |
| **D-H2B-GOAWAYSENT** | PINNED (AMEND-H2B-1/3). `http2.goaway_sent` STAYED 0 through a full reference-initiated drain-close (local destroy) — the reference does NOT count a `goaway_sent` on graceful upstream conn close. A received peer GOAWAY is counted by NO `http2.*` stat (only `upstream_cx_close_notify` + the destroy sequence). envoy-go's `Close()`-emitted GOAWAY must therefore NOT drive a `goaway_sent` counter (it would read non-zero where the reference reads 0). |

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-H2B-BACKEND** — the `0080` GOAWAY/RST-capable backend: extend `H2HoldResponder` (kind 37) with `/__goaway` + `/__rst` gates vs a new kind 38 (§8.2).
- **D-H2B-WATCHER-LIFECYCLE** — the watcher's exact exit conditions: GOAWAY fired → act-then-exit (re-check `findPooledLocked != nil` + `inFlight == 0` under `h2PoolMu` before evicting — the double-evict guard vs `EvictH2ConnOnError`/`makeRelease`); `cc.ctx.Done()` → exit without acting (evicted by another path). No pool-owned done channel; no goroutine leak.
- **D-H2B-WIRING** — the codec→cluster handle-passing mechanism (`NewClientConn` constructor param vs a post-construction setter); whether the kept single-shot `DialH2` path wires the handles or passes nil; nil-guarded increments.
- **D-H2B-SPLIT-FINAL** — the final ADR-0045 re-check at PLAN (43.2b anticipated flat; §3.0).
- **D-H2B-FUZZER-RECONCILE** — reconcile the documented-42-vs-actual-43 `^func Fuzz` drift at this family-closing leg, or carry it forward (§6).
- **D-H2B-BYTESTAB** — confirm `0004` + `0079` stay GREEN under the drain extensions (`0079`'s EXACT prong does not trigger GOAWAY; assertions GOAWAY-agnostic).
- **D-H2B-CXSTATS** — which `upstream_cx_*` close/destroy counters (`cx_close_notify`, `cx_destroy`, `cx_destroy_local`) envoy-go emits, to fix the `0080` cross-side assertion targets (the EXACT prong's load-bearing asserts are `upstream_cx_http2_total` + the two reset counters, which both sides emit regardless).

## 13. ADR continuity — the ADR-0254 §Context DRAFT (anchored here; full entry lands at the 43.2b IMPL)

**ADR-0254 §Context (draft):** ADR-0253 (phase 43.2a) landed the per-endpoint H2 multiplex `ClientConn` pool over the 43.1 permit substrate (ADR-0252) with MINIMAL liveness only (`Closed()`-skip + evict-on-`RoundTrip`-error) and DEFERRED the graceful drain lifecycle to 43.2b. The codec already observes a peer GOAWAY (`goawayCh` closed, streams above `LastStreamID` finished) but does not cancel the conn ctx, so a GOAWAY'd-but-alive conn is invisible to the 43.2a admission scan (`Closed()` stays false) and keeps taking new streams. The 2026-06-24 live probe (D-H2B-ROTATION/STATS/GOAWAYSENT, contrib-v1.37.2) revealed the reference observes a rotation ENTIRELY via the connection-lifecycle counters (`upstream_cx_close_notify` then `upstream_cx_destroy_local`, with the conn held until the last in-flight stream completes) — `http2.goaway_sent` stays 0 and there is no `goaway_received` counter — and that of the brainstorm's four candidate reset/goaway stats only `http2.rx_reset` + `http2.tx_reset` are live-and-cross-side-assertable (AMEND-H2B-1/2/3). 43.2b adds the pool-visible GOAWAY signal (`GoneAway()`/`GoneAwayCh()`), a per-conn drain-watcher goroutine (so an idle draining conn closes promptly), the admission/eviction extensions (skip draining; evict on `(Closed()||GoneAway()) && inFlight==0`), lazy replacement via the established MISS path, and the first codec→cluster stat wiring for `http2.rx_reset` + `http2.tx_reset`. `min(local,peer)` enforcement, `nextStreamID` 2^31-exhaustion retirement, and the `goaway_sent`/`stream_refused_errors` stats are DEFERRED as recorded hardening items. §Decision/§Consequences land at the 43.2b IMPL. ANCHORS ADR-0254; completes the H2-multiplex-pool lifecycle ADR-0253 began.

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at SPEC (docs-only): stat **1185** / fixtures **81** / fuzzers **42** (documented; actual 43) / BackendKind **37** / DECISIONS **ADR-0253** (next-free **ADR-0254**). Anticipated at the 43.2b IMPL: stat **1187** (+2 — `http2.rx_reset` + `http2.tx_reset`; AMEND-H2B-2, REVISED DOWN from the brainstorm's ~1190) / fixtures **82** (`0080-h2-goaway-rotation`) / fuzzers **42** (or 43 if reconciled — D-H2B-FUZZER-RECONCILE) / BackendKind **37** (extend) or **38** (new) — PLAN / DECISIONS **ADR-0254**. ROADMAP row 43 flips **`done`** at the 43.2b IMPL (ALL legs 43.1 + 43.2a + 43.2b landed) ⇒ the **Upstream-robustness family (phases 39–43) CLOSES**. Next → the 43.2b PLAN (`superpowers:writing-plans` — the §12 D-H2B-* PLAN questions).
