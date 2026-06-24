# Phase 43.2b Brainstorm — graceful GOAWAY-driven connection rotation (the SECOND sub-leg of the SECOND-and-FINAL leg of the FIFTH-and-FINAL Upstream-robustness-family row; the H2-multiplex-pool drain lifecycle that ADR-0253 §Consequences DEFERRED — ANCHORS ADR-0254)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document settles the design forks for phase **43.2b** (`h2-connection-pool`, sub-leg 2 of 2), the graceful GOAWAY-driven connection-rotation leg. Its predecessor sub-leg **43.2a** (the H2 multiplex pool substrate — ADR-0253, SUPERSEDES ADR-0056) is **DONE + LANDED** (commit `30b3aad1`); 43.2a ships MINIMAL liveness only (`ClientConn.Closed()`-skip in the admission scan + evict-on-`RoundTrip`-error) and DEFERRED the Envoy-faithful drain lifecycle + the `http2.*` reset/goaway stat subtree + `min(local,peer)` enforcement + `nextStreamID` exhaustion-rotation to this leg (ADR-0253 §Consequences "DEFERRED to 43.2b"). 43.2b is the **LAST** leg of ROW 43 (`connection-pooling`): when it lands, **row 43 flips `done`** and the **Upstream-robustness family (phases 39–43) CLOSES** (`reference_roadmap_split_phase_row_done`).

43.2b builds directly on the 43.2a substrate (`internal/cluster/h2pool.go` + `internal/filter/hcm/h2/client.go`): the per-endpoint `Cluster.h2Pool map[string][]*pooledH2Conn` over the 43.1 permit substrate (ADR-0252), the four-tier `AcquireH2Stream` admission ladder (stream-HIT → connecting-HIT → MISS-dial → PEND), the connect-time dial-coalescing "connecting" tier (Task 9.5), the single ctx-aware LB pick threaded through the dial seam (Task 7.5), and the `releaseStream`→`evictH2ConnLocked` eviction seam. 43.2b extends the eviction seam from the closed-conn case to the **draining** case and adds the first **codec→cluster** stat wiring for the H2 client.

---

## 1. Mission and scope confirmation (43.2b only)

### 1.1 What phase 43.2b delivers (envelope: the drain lifecycle + the reset/goaway stats)

1. **Lazy GOAWAY rotation** — a pooled conn that received a peer GOAWAY (the codec's `goawayCh` signal) is marked draining, SKIPPED by the admission scan, and CLOSED when its last in-flight stream finishes (the in-flight path) OR eagerly when it is idle at GOAWAY time (the idle path); a replacement opens on-demand via the existing MISS path. The 43.2a `releaseStream`→`evictH2ConnLocked` seam (which today fires only on `Closed() && inFlight==0`) is extended to the draining case.
2. **Pool-visible GOAWAY signal + per-conn drain-watcher** — the codec exposes the GOAWAY signal to the pool (`GoneAway()` predicate + `GoneAwayCh()` channel); the pool spawns one drain-watcher goroutine per pooled conn that, on GOAWAY, marks the conn draining and evicts-if-idle (the Envoy-faithful prompt-close of an idle draining conn).
3. **The behavior-driven `http2.*` reset/goaway stat subtree** that 43.2a left unregistered — the subset envoy-go's codec actually drives + this leg's rotation exercises (candidates: `http2.goaway_sent`, `http2.rx_reset`, `http2.tx_reset`, `http2.stream_refused_errors`); the EXACT set + the `+N` to the surface pinned at the 43.2b SPEC via a live `contrib-v1.37.2` probe (`reference_docker_probe_bridge_network`).
4. **The `0080-h2-goaway-rotation` cross-side differential** — an H2 (TLS+ALPN-h2) DOWNSTREAM listener → an h2c upstream cluster with a GOAWAY-capable backend, driving the rotation as sequenced poll-to-converge phases behind release barriers.
5. **ADR-0254** — the GOAWAY-rotation leg (§Context drafted at the 43.2b SPEC; §Decision/§Consequences land at the 43.2b IMPL per ADR-0044).

### 1.2 What phase 43.2b does NOT deliver (deferred → recorded hardening items)

- **`min(local,peer)` proactive `max_concurrent_streams` enforcement** (AMEND-H2-5) — 43.2a enforces the LOCAL cap and lets a peer-stricter `REFUSED_STREAM` surface via the codec's existing RST path. Proactively capping stream admission at `min(local,peer)` to avoid the refusal is DEFERRED — a recorded hardening item (the `http2.stream_refused_errors` counter is still registered + driven by the codec's existing REFUSED_STREAM-reset path, so the observable is captured even though the avoidance is not implemented).
- **`nextStreamID` 2^31-exhaustion retirement** (the M-12 / Task-11 follow-up) — a long-lived pooled conn eventually exhausts the 31-bit client-stream-id space and should retire via GOAWAY before wrapping. DEFERRED (needs ~2^31 streams to trigger; hard to exercise differentially) — a recorded hardening item.
- `max_requests_per_connection` (the per-conn request budget → request-count rotation); idle-connection-timeout eviction; eager/pre-warmed conn creation; `max_connection_pools`; H3/QUIC pooling — all inherited-deferred from 43.2a.
- Any change to the 43.1 `max_connections` / `max_pending_requests` semantics or to the H1 pool.

### 1.3 ADR-0045 split posture — 43.2b ships as ONE leg (no further split)

Scoped to **core only** (the user's brainstorm decision): GOAWAY rotation + the behavior-driven `http2.*` stats; BOTH `min(local,peer)` (3) and `nextStreamID`-exhaustion retirement (4) deferred. The remaining envelope (the `GoneAway()` codec seam + the drain-watcher + the admission/eviction extensions + the codec→cluster stat wiring + the registration test + the `0080` differential + the ADR/contract bundle) is estimated at **≈150–220 prod LoC / ~8–10 tasks** — comfortably UNDER the ADR-0045 soft split gate. The PLAN does the final ADR-0045 re-check (the 43.1 D-S431-7 / 43.2a precedent) but 43.2b ships as one leg. Row 43 flips `done` at this leg's IMPL.

### 1.4 Package placement + directory convention

Directory: `43.2-h2-connection-pool/` (this sub-leg lives in the SAME directory as 43.2a; the doc is `BRAINSTORM-43.2b.md` to avoid clobbering the parent `BRAINSTORM.md`; the 43.2b SPEC/PLAN/PROGRESS will be `*-43.2b.md` siblings). The work lands in the EXISTING `internal/cluster` package (the `h2pool.go` drain extensions + the drain-watcher) and the EXISTING `internal/filter/hcm/h2` package (the `GoneAway()`/`GoneAwayCh()` accessors + the reset/goaway stat event sites) and touches `internal/cluster/cluster.go` (the new stat registrations). ZERO new Go packages; ZERO new go.mod modules (the `h2` codec is an in-tree dep; `go mod tidy -diff` anticipated EMPTY).

### 1.5 Relationship to the 43.2a substrate (the drain lifecycle layered over the established pool)

43.2a gave 43.2b: the per-endpoint pool + the four-tier admission ladder + the connecting tier + the eviction seam (`makeRelease`→`evictH2ConnLocked`) + the permit/LB-release conservation discipline + the `http2.streams_active` gauge + the `upstream_cx_http2_total` counter. 43.2b adds a THIRD admission-skip dimension (draining, alongside `Closed()` and stream-saturation), a per-conn watcher that proactively evicts idle draining conns, and the reset/goaway stat accounting. Permit + LB-release conservation are PRESERVED verbatim: draining-eviction Closes the conn → `connDec` → `releaseConn` frees the `max_connections` permit (exactly as the closed-conn case does today); `h2PromoteLocked` re-homes the freed permit to a queued waiter (which dials the replacement) or to the next dial.

---

## 2. Design decisions

### 2.1 Subject confirmation: GOAWAY rotation — the pre-authorized 43.2b sub-leg *(Q-subject)*

Confirmed via ADR-0253 §Consequences ("DEFERRED to 43.2b (ADR-0254): graceful GOAWAY-driven conn rotation + the drain-close-on-last-stream lifecycle + the `http2.*` reset/goaway stats … + `min(local,peer)` … + the `nextStreamID` 2^31 exhaustion-rotation") and `reference_roadmap_split_phase_row_done` (row 43 is the 3-leg row 43.1 + 43.2a + 43.2b; 43.1 + 43.2a DONE; 43.2b is the LAST). No new go.mod module. The subject COMPLETES the H2-multiplex-pool lifecycle that ADR-0253 began.

### 2.2 Scope: core only — GOAWAY rotation + the behavior-driven stats *(Q-scope → Fork: CORE-ONLY)*

**Chosen: core only.** GOAWAY rotation + the behavior-driven `http2.*` reset/goaway stat subset. Chosen over **core + both candidates** (rejected: adding `min(local,peer)` (3) + `nextStreamID`-exhaustion retirement (4) risks tripping the ADR-0045 soft task-count gate and a needless further sub-split of an already-final leg) and over **core + one candidate** (rejected: (4) needs ~2^31 streams to trigger — undifferentiable; (3)'s enforcement is hardening, while its OBSERVABLE — the `stream_refused_errors` counter — is captured by the stats subset regardless). Core-only keeps 43.2b a single clean cycle and closes the family fastest. (3) and (4) become recorded hardening items (§1.2).

### 2.3 Drain detection: a per-conn drain-watcher goroutine *(Q-drain-detection → Fork: WATCHER)*

**Chosen: a per-conn drain-watcher goroutine.** The codec already closes `goawayCh` and finishes streams above `LastStreamID` on a peer GOAWAY (`client.go` `dispatchFrame`'s `GoAwayFrame` case), but it does NOT cancel the conn ctx — so `Closed()` stays false and the 43.2a admission scan keeps handing new streams to the draining conn. 43.2b spawns, at pool insertion (in `dialAndPool`, after the conn is appended to `h2Pool[addr]`), one watcher goroutine that `select`s on the conn's GOAWAY signal vs a conn-done signal; on GOAWAY it locks `h2PoolMu`, and if the conn is still pooled with `inFlight==0` it evicts-if-idle (`evictH2ConnLocked` + `h2PromoteLocked`), else leaves it for the last `releaseStream`. The watcher exits cleanly on either branch (no goroutine leak — it also returns when the conn is closed/evicted by another path).

Chosen over **poll-on-admission + release-time only** (rejected: an IDLE pooled conn that receives GOAWAY has no `releaseStream` to ever fire and its ctx is never canceled, so a poll-only scheme would skip it forever but never close it — a slow conn leak; mitigating it with an endpoint sweep on every release is more code than the watcher and races the same way) and over a **codec `onGoAway` callback** (rejected: inverts the pool→codec layering — the codec would call into a pool-supplied closure and run `h2PoolMu` work from the readLoop goroutine; the watcher keeps the codec cluster-agnostic for the *control* path). The watcher is a new pattern (the 43.2a pool has no per-conn goroutines) but the codec already runs a readLoop goroutine per conn, so the per-conn-goroutine cost is already paid. This is the Envoy-faithful behavior: an idle draining conn closes promptly; an in-flight draining conn drains its promised streams then closes.

### 2.4 Admission + eviction extensions *(the load-bearing mechanism)*

- **Admission skip:** `findStreamHitLocked` / `findConnectingHitLocked` (and `h2PromoteLocked`'s stream-slot branch) add `&& !pc.cc.GoneAway()` so a draining conn takes NO new streams. No new `pooledH2Conn` struct field — `draining` is DERIVED from `cc.GoneAway()`, exactly as `Closed()` is used today (keeps the single-mutator `h2PoolMu` surface unchanged; D-H2-MUTEX preserved).
- **Eviction condition:** `makeRelease`'s eager-close check generalizes from `pc.cc.Closed() && pc.inFlight==0` to `(pc.cc.Closed() || pc.cc.GoneAway()) && pc.inFlight==0`. The in-flight draining path thus closes on the last release; the idle draining path is handled by the watcher (§2.3).
- **Lazy replacement** (Fork: LAZY, inherited from the 43.2a substrate + its "no eager/pre-warm" non-purpose): no background dial. A skipped draining conn → the next acquire MISSes → dials a replacement via the established MISS path. Permit + LB-release conservation UNCHANGED.
- `goaway_sent` increments when we `Close()` the drained conn — `Close()` (`client.go`) already emits GOAWAY(NO_ERROR) with the highest allocated stream id.

### 2.5 Stat surface: a behavior-driven `http2.*` subset *(Q-stats-scope → Fork: BEHAVIOR-SUBSET)*

**Chosen: register only the `http2.*` counters envoy-go's codec actually drives + this leg's rotation exercises.** Candidate set: `http2.goaway_sent` (GOAWAYs we emit — on `Close`/`emitGoaway`), `http2.rx_reset` (RST_STREAM received — `dispatchFrame`'s `RSTStreamFrame` case), `http2.tx_reset` (RST_STREAM we emit — e.g. the CANCEL on ctx cancel), `http2.stream_refused_errors` (RST with REFUSED_STREAM received — the AMEND-H2-5 observable, captured even though `min(local,peer)` avoidance is deferred). The EXACT set + final `+N` is pinned at the SPEC live probe (`contrib-v1.37.2`). Anticipated surface delta: **1185 → ~1190**.

Chosen over **the full `http2.*` subtree mirror** (rejected: the reference emits ~24 `http2.*` names — flood-protection, messaging-error, header-overflow, trailers, tx_flush_timeout, keepalive_timeout — but envoy-go's codec implements none of those paths, so ~20 would register dead-at-0, cutting against the project's "prove every new assertion is live" discipline — `reference_differential_asserter_dispatch`) and over **minimal — `goaway_sent` only** (rejected: under-mirrors the reference's reset accounting the codec already has the events for; `rx_reset`/`tx_reset` are cheap, live, and directly observable). Every registered counter is LIVE and differentially assertable. All counters are useH2-gated (registered ONLY on a `useH2()` cluster; non-H2 byte-stable), with a registration test analog to `TestRegisterClusterMetrics_H2Stats`.

### 2.6 Codec→cluster stat wiring: inject cluster stat handles into the `ClientConn` at dial *(Q-stat-wiring → Fork: INJECT-HANDLES)*

**Chosen: the pool injects the cluster's `http2.*` counter handles into the `ClientConn` at dial.** 43.2a drove ALL its stats from the POOL layer (`h2StreamsActiveInc`/`h2CxHTTP2TotalInc` at acquire/release) — the codec was never aware of the cluster. But the reset/goaway EVENTS happen IN the codec (`dispatchFrame`, `emitGoaway`, `Close`), so 43.2b introduces the FIRST codec→cluster stat wiring for the H2 client: when the pool dials (`dialPooledH2To` → the `h2ConnFromDialed` finalizer → `NewClientConn`), it passes the cluster's `http2.*` stat handles; the codec increments at each event site. Chosen over **codec-side atomic accumulators the pool reads-and-deltas at conn Close/drain** (rejected: lossy and awkward for live counters — a counter that must reflect every RST cannot be reconstructed from a close-time delta; injection is the precise wiring). The exact handle-passing mechanism (constructor param vs a setter) is a PLAN detail; the brainstorm fixes the seam.

### 2.7 The differential: sequenced-barrier EXACT, attempt first *(Q-differential-strategy → Fork: SEQUENCED-EXACT-FIRST)*

**Chosen: drive the rotation as discrete poll-to-converge phases behind release barriers, targeting cross-side EXACT.** A new `0080-h2-goaway-rotation` fixture: an H2 (TLS+ALPN-h2) DOWNSTREAM listener (the 0004 PKI shape — MANDATORY: the H2 pool engages ONLY on an H2 downstream, `reference_h2_pool_downstream_codec_gate`; an H1-downstream variant never exercises the pool) → an h2c upstream cluster with a GOAWAY-capable backend, on BOTH the subject and the reference. Staged drive (SLEEPLESS — release-barrier + poll-to-converge; sequential-per-side, `reference_concurrency_differential_release_barrier`):

1. Fire held streams ⇒ poll the conn/`streams_active` counts to converge.
2. Barrier ⇒ trigger the backend GOAWAY ⇒ poll the drained conn OUT of admission (no new streams ride it) + poll the `http2.*` deltas (`goaway_sent`/reset counters move).
3. Fire a fresh request ⇒ poll the REPLACEMENT conn IN (`upstream_cx_http2_total` increments beyond the drained conn).
4. Release barrier ⇒ all held drain to 200 ⇒ gauges (`http2.streams_active`, `upstream_cx_active`) poll back to the expected steady state.

Chosen over **subject-EXACT + cross-ROBUST upfront** (the 43.1 soft-breaker posture — rejected as the DEFAULT but RETAINED as the fallback: if the SPEC probe shows the GOAWAY-vs-next-request timing races across the Docker boundary for a particular count, that count falls back to subject-EXACT + cross-ROBUST, the rest stay EXACT) and over **behavioral-only, no count EXACT** (rejected: too loose — the sequenced barriers make most counts deterministic, like 43.2a's sequenced `ceil` prong). The sequencing is what buys determinism: each step settles before the next is driven, so the GOAWAY lands relative to a quiesced pool, not a racing one. Final feasibility per count confirmed at the SPEC live probe.

### 2.8 The GOAWAY-capable backend *(deferred to PLAN — D-H2B-BACKEND)*

The `0080` backend must emit a GOAWAY on a gate (after some streams are established). Two options, a PLAN/IMPL deliverable decision (the D-H2-BACKEND precedent — resolved at PLAN for 43.2a): **extend `H2HoldResponder` (BackendKind 37)** — the 43.2a h2c hold-and-release backend already has re-armable `/__release` + sticky gates + `SETTINGS_MAX_CONCURRENT_STREAMS: 1000` — by adding a `/__goaway` gate that triggers a GOAWAY emission on the held conn; **vs a new BackendKind 38** (a dedicated GOAWAY-emitting h2c backend). Extending 37 reuses the hold/gate/SETTINGS machinery (favored); a new 38 is cleaner-separated but more code. Recorded as a PLAN D-question; the brainstorm does not pre-decide (BackendKind tail stays 37 OR advances to 38 at IMPL).

---

## 3. SPEC-time empirical pins (D-H2B-* — to execute live at the 43.2b SPEC)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared bridge; a Go h2c GOAWAY-emitting backend; sequenced held-stream + GOAWAY-trigger drive; `/stats`-delta scrapes; request path verified `upstream_cx_total>0` + backend `proto=HTTP/2.0`).

| Pin | Question to resolve at SPEC |
|-----|------------------------------|
| **D-H2B-STATS** | The EXACT `http2.*` reset/goaway names the reference emits at the cluster scope + which envoy-go drives live (the behavior-driven subset §2.5) + the final `+N` to the surface (anticipated 1185 → ~1190). Confirm `http2.goaway_sent` / `http2.rx_reset` / `http2.tx_reset` / `http2.stream_refused_errors` exist + their exact spellings/scopes. |
| **D-H2B-ROTATION** | The reference's observable rotation behavior on a peer GOAWAY: does it open a replacement conn lazily (on the next request) and what does the `upstream_cx_*` / `cx_destroy` / `cx_active` sequence look like — to size the §2.7 sequenced assertions. |
| **D-H2B-EXACT** | Per-count cross-side determinism under the sequenced-barrier drive: which counts are cross-side EXACT vs need the subject-EXACT + cross-ROBUST fallback (§2.7). |
| **D-H2B-GOAWAYSENT** | Whether `goaway_sent` is driven by OUR Close-emitted GOAWAY (envoy-go's `Close()` emits GOAWAY(NO_ERROR)) and whether the reference counts a received-GOAWAY anywhere at the cluster scope (Envoy historically has no `goaway_received` cluster counter — confirm). |

## 4. PLAN / IMPL D-questions (not empirical pins)

- **D-H2B-BACKEND** — the `0080` GOAWAY-capable backend: extend `H2HoldResponder` (kind 37) with a `/__goaway` gate vs a new kind 38 (§2.8).
- **D-H2B-WATCHER-LIFECYCLE** — the PLAN/SPEC must specify the drain-watcher's exit conditions precisely (GOAWAY fired → act-then-exit; conn closed/evicted by another path → exit without acting) and the conn-done signal it selects on. `cc.ctx.Done()` is the natural "evicted by another path" signal — `Closed()` flips precisely when `cc.cancel()` runs (on `Close()`/transport error), so `select { case <-cc.GoneAwayCh(): … ; case <-cc.ctx.Done(): return }` covers both without a separate pool-owned done channel. The watcher MUST re-check the conn is still pooled (`findPooledLocked != nil`) under `h2PoolMu` before evicting, to avoid a double-evict race with `EvictH2ConnOnError` / `makeRelease`. So no watcher goroutine leaks and no double-evict races `h2PoolMu`.
- **D-H2B-WIRING** — the codec→cluster stat handle-passing mechanism (constructor param to `NewClientConn` vs a post-construction setter); whether the single-shot `DialH2` path (kept at 43.2a) also wires the handles or passes nil.
- **D-H2B-SPLIT-FINAL** — the final ADR-0045 re-check at PLAN (43.2b anticipated flat; §1.3).
- **D-H2B-FUZZER-RECONCILE** — 43.2b adds NO fuzzer (no new wire decoder), but it is the family-closing leg — a natural point to reconcile the documented-42-vs-actual-43 `^func Fuzz` drift (`reference_fuzzer_count_docs_drift`). PLAN call: reconcile the doc to 43 here, or carry the note forward.
- **D-H2B-BYTESTAB** — confirm the existing H2 fixtures (`0004`, `0079`) stay GREEN under the drain extensions (the `0079` EXACT prong does not trigger GOAWAY, so it must be unaffected; assertions GOAWAY-agnostic).

## 5. ADR continuity — ADR-0254 §Context DRAFT (anchored here; §Context lands at the 43.2b SPEC, full entry at IMPL)

**ADR-0254 §Context (draft):** ADR-0253 (phase 43.2a) landed the per-endpoint H2 multiplex `ClientConn` pool over the 43.1 permit substrate with MINIMAL liveness only (`Closed()`-skip + evict-on-`RoundTrip`-error) and DEFERRED the graceful drain lifecycle to 43.2b. The codec already observes a peer GOAWAY (`goawayCh` closed, streams above `LastStreamID` finished) but does not cancel the conn ctx, so a GOAWAY'd-but-alive conn is invisible to the 43.2a admission scan (`Closed()` stays false) and keeps taking new streams. 43.2b adds the pool-visible GOAWAY signal (`GoneAway()`/`GoneAwayCh()`), a per-conn drain-watcher goroutine (so an idle draining conn closes promptly), the admission/eviction extensions (skip draining; evict on `(Closed()||GoneAway()) && inFlight==0`), lazy replacement via the established MISS path, and the first codec→cluster stat wiring for the behavior-driven `http2.*` reset/goaway subset. `min(local,peer)` enforcement and `nextStreamID` 2^31-exhaustion retirement are DEFERRED as recorded hardening items. §Decision/§Consequences land at the 43.2b IMPL. ANCHORS ADR-0254; completes the H2-multiplex-pool lifecycle ADR-0253 began.

## 6. Exit — counts + ROADMAP/STATE at brainstorm-done

Counts UNCHANGED at brainstorm (docs-only): stat **1185** / fixtures **81** / fuzzers **42** (documented; actual `^func Fuzz` 43) / BackendKind tail **37** / DECISIONS tail **ADR-0253** (next-free **ADR-0254**). Anticipated at the 43.2b IMPL: stat **~1190** (+~5, the behavior-driven `http2.*` subset; EXACT `+N` pinned at SPEC) / fixtures **82** (`0080-h2-goaway-rotation`) / fuzzers **42** (or 43 if reconciled — D-H2B-FUZZER-RECONCILE) / BackendKind **37** (extend) or **38** (new) — PLAN / DECISIONS **ADR-0254**. ROADMAP row 43 flips **`done`** at the 43.2b IMPL (ALL legs 43.1 + 43.2a + 43.2b landed) ⇒ the **Upstream-robustness family (phases 39–43) CLOSES**. Next → the 43.2b SPEC (`superpowers:brainstorming` predecessor → the SPEC live-probe block D-H2B-*).
