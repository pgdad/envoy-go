# Phase 43.1 SPEC — connection-pool budgets: the `max_connections` HARD-CAP + the `max_pending_requests` bounded wait-queue — the first leg of the FIFTH-and-FINAL Upstream-robustness-family row

**Lifecycle:** SPEC (lifecycle-state 1 → 2). Predecessor: the phase-43 BRAINSTORM (`docs/envoy-go/phases/43-connection-pooling/BRAINSTORM.md`, commit `19ded383`). This SPEC charters phase **43.1** — the budget substrate + the pending wait-queue (the brainstormed by-concern leg 1), landing the `max_connections` + `max_pending_requests` enforcement phase 41 PARSED-and-registered-emit-0-but-DEFERRED. Counts at SPEC commit UNCHANGED (stat surface **1181** / fixtures **79** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0251**, next-free **ADR-0252**). The §11 D-CP-* empirical pins were EXECUTED IN-SESSION (2026-06-22) live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network`.

---

## 1. Purpose / Mission

Land the upstream connection-pool budget + the **pending wait-queue the synchronous-acquire model lacks** — the piece phase 41 explicitly deferred to this row. A cluster configured with `circuit_breakers{max_connections, max_pending_requests}` caps the number of concurrent upstream connections at `max_connections`; a request that needs a NEW connection when the cap is reached does NOT fail-fast (the deliberate CONTRAST with phase-41's `max_requests`) — it joins a **bounded pending wait-queue** (blocking its own request goroutine), woken when a connection frees; only when the wait-queue itself is full (`max_pending_requests`) is the request **rejected fail-fast** with a `503` + `upstream_rq_pending_overflow`. This is the project's FIRST standing upstream request queue. It EXTENDS the phase-41 `circuitBreaker` per-priority accounting struct (adding the `activeConnections`/`pendingRequests` counters + the two budgets phase 41 parsed-but-stored-no-fields-for) + REUSES the existing H1 LIFO pool (`h1Pool`/`AcquireH1`/`PutIdleH1`) + the `connWithGauge` close path that already drives `upstream_cx_active`.

43.1 is the first of the by-concern 2-leg split (BRAINSTORM Fork 2): **43.1 = the budget substrate + pending wait-queue** (this SPEC; ADR-0252); **43.2 = the H2 multiplex connection pool** (a later SPEC; supersedes ADR-0056; ADR-0253).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 live pins drove these amendments. **The headline (AMEND-CP1):** the BRAINSTORM (inheriting phase-41's framing) assumed `max_connections` PENDS deterministically and `max_pending_requests` bounds that pend cleanly; the live probe revealed the reference's `max_connections` is a **SOFT** breaker (no clean hard cap; `upstream_cx_active` repeatedly EXCEEDED `max_connections` — 1→1 but 2→3, 2→5, 2→8/9 with timing slop) and the `upstream_rq_pending_overflow` 503 count is a timing-sensitive interplay of `max_connections` + `max_pending_requests` + burst arrival (NOT a clean cross-side-reproducible formula). A cross-side **EXACT** differential on connection/overflow counts is therefore INFEASIBLE (precisely why phase 41 deferred these budgets). **User-confirmed strategy (2026-06-22):** envoy-go implements a CLEAN HARD-CAP `max_connections` + a bounded pending wait-queue + fail-fast 503 (a recorded DEPARTURE from the reference's racy soft behavior — arguably more correct), and the differential asserts ROBUST cross-side invariants (the gauges flip; the FACT of overflow; the downstream class) + SUBJECT-SIDE exactness.

- **AMEND-CP1 (the strategy — a CLEAN HARD CAP + bounded queue; a recorded DEPARTURE).** Live finding (D-CP-LIFECYCLE): in `contrib-v1.37.2` (H1 upstream, deterministic raw-hold backend) `max_connections` does NOT hard-cap connections — `cx_active` exceeded `max_connections` with timing slop, while `cx_open`→1 and `upstream_cx_overflow`++ (a soft observability breaker that does NOT itself reject/pend); the request-failing `503` comes from `upstream_rq_pending_overflow` whose count is timing-sensitive (3-of-6, 5-of-6 shifting with race timing). envoy-go instead implements: `max_connections` = a HARD cap on connection CREATION; a request needing a new connection at the cap PENDS in a bounded wait-queue; queue-full ⇒ fail-fast `503` + `upstream_rq_pending_overflow`. The differential asserts the robust deterministic facts (the `cx_open`/`rq_pending_open` gauges flip cross-side; an oversubscribed request gets a `503` on the DOWNSTREAM class cross-side; `upstream_rq_pending_overflow` delta ≥ 1 cross-side) + the SUBJECT-SIDE exact cap/queue behavior (unit + the subject prong of `0078`). The exact cross-side connection/overflow COUNTS are a recorded DEPARTURE (the reference doesn't hard-cap deterministically). *(User-confirmed; the phase-41 SPEC-narrowing precedent.)*
- **AMEND-CP2 (the shared overflow counter — `upstream_rq_pending_overflow`).** Live finding (D-CP-STATS): the connection-pool-overflow `503` increments **`upstream_rq_pending_overflow`** — the SAME counter phase-41's `max_requests` overflow uses (AMEND-CB2); there is NO distinct `upstream_cx_reject`/`upstream_rq_overflow`. envoy-go's queue-full `503` increments `upstream_rq_pending_overflow` (already a LIVE handle from phase 41) + sets the `rq_pending_open` gauge.
- **AMEND-CP3 (stat surface — activate 3 emit-0 + ADD 2 new pending stats).** Live finding (D-CP-STATS): a connection-pooled cluster emits, beyond phase-41's +14 block, **`upstream_rq_pending_active`** (gauge — current pending-queue depth) + **`upstream_rq_pending_total`** (counter — cumulative requests that entered the queue) — names envoy-go does NOT yet register (confirmed via codebase grep). 43.1: (a) ACTIVATE the phase-41 emit-0 handles `circuit_breakers.default.cx_open` + `circuit_breakers.default.rq_pending_open` (gauges) + `upstream_cx_overflow` (counter) — store the handles + drive them LIVE (no NEW names from activation); (b) ADD `upstream_rq_pending_active` + `upstream_rq_pending_total`, scoped to `circuit_breakers` clusters (the phase-41 scoping departure). Surface anticipated **1181 → ~1199** (+2 new names on the existing `0074` CB cluster + the new `0078` CB cluster's full block of ~16); the EXACT figure is a PLAN/IMPL pin (D-CP-STATS). The `remaining_*` gauges STAY deferred (`track_remaining`-gated — the phase-41 posture); the `high.*` subtree STAYS emit-0 (no priority routing).
- **AMEND-CP4 (the overflow response — `503` + `UO`).** Live finding (D-CP-LIFECYCLE): a queue-full overflow yields `503` + `RESPONSE_FLAGS UO` + `upstream_reset_before_response_started{overflow}` — the SAME shape as phase-41's `max_requests` overflow (AMEND-CB4). envoy-go emits the `503` via the existing local-reply path; the `UO` flag is an internal-correctness target (phase-41's recorded departure — no response-flags plumbing); the differential asserts the robust pair (`503` status + `upstream_rq_pending_overflow` delta + the gauges), NOT the access-log line.
- **AMEND-CP5 (defaults — 1024 / 1024).** Live finding (D-CP-PROTO, via `remaining_*` + phase-41 AMEND-CB5): `max_connections` + `max_pending_requests` default 1024. So the budgets are effectively OFF unless explicitly small (the breaker only bites when configured small). envoy-go applies the 1024 default for an absent budget on a `circuit_breakers` cluster.
- **AMEND-CP6 (reject surface — NONE new; NO new fuzzer).** Live finding (D-CP-REJECT): `max_connections`/`max_pending_requests` are existing `Thresholds` fields phase 41 already PARSES + reject-covers (the priority-range + duplicate-priority + retry_budget-percent arms, ADR-0080); 43.1 adds ENFORCEMENT, not parsing ⇒ NO new config-reject arm. The budget `UInt32Value`s stay unbounded (`max_connections: 0` ⇒ a valid all-reject config, the `max_requests:0` precedent). NO new fuzzer (config-parse, not wire-decode); fuzzers STAY **42**.
- **AMEND-CP7 (REUSE `BlockingHoldResponder` 36 — NO new BackendKind).** D-CP-BACKEND: the `0078` differential fills `max_connections` with N held-open connections via the phase-41 `BlockingHoldResponder` (BackendKind 36) + its `/__release` control port — the held connection deterministically occupies a connection slot. NO new BackendKind; tail STAYS **36**. (The 43.2 H2-multiplex differential may need an H2-capable hold backend — a 43.2 obligation, not 43.1.)

### 1.2 ADR continuity + D-disposition at SPEC commit

ADR-0252 (the connection-pool budget/queue architecture — the `activeConnections`/`pendingRequests` extension on the phase-41 `circuitBreaker` struct + the `max_connections` HARD cap at the connection-creation boundary + the bounded pending wait-queue + the queue-full 503 + the stat activation/additions) — §Context DRAFT anchored here (§13); the full §Decision/§Consequences land at the 43.1 IMPL per ADR-0044. DECISIONS tail STAYS ADR-0251 at this SPEC; next-free ADR-0252. (ADR-0253 — the 43.2 H2 multiplex pool, superseding ADR-0056 — is a LATER leg's obligation.) The §10 BRAINSTORM D-CP pins are RESOLVED in §11; the PLAN/IMPL D-questions are §12.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §1.1 amendments)

- **The H2 multiplex connection pool** (`ClientConn` reuse + `max_concurrent_streams` enforcement + GOAWAY rotation; supersedes ADR-0056) — the 43.2 leg (ADR-0253). 43.1's H2 path stays per-request-fresh (`DialH2` + `defer cc.Close()`); each H2 dial is `max_connections`-permit-gated like an H1 pool-miss, but no cross-request reuse yet.
- **Cross-side EXACT connection/overflow counts** (AMEND-CP1) — the reference's soft/racy `max_connections` makes exact cross-side parity infeasible; the exact cap/queue/overflow counts are SUBJECT-SIDE (unit + the `0078` subject prong); a recorded departure.
- **`max_connection_pools` enforcement** — `cx_pool_open` + `upstream_cx_pool_overflow` STAY emit-0 (an edge knob orthogonal to the keystone pair).
- **`per_host_thresholds` (CircuitBreakers field 2)** — silent-ignored (the phase-41 posture).
- **`max_requests_per_connection`** (`CommonHttpProtocolOptions`) — the per-conn request-count rotation knob; deferred.
- **`track_remaining` + the `remaining_*` gauges** — NOT registered (the phase-41 posture).
- **HIGH-priority binding** — blocked on priority routing; the `circuit_breakers.high.*` subtree STAYS emit-0.
- **Connection-pool budgets on non-HTTP (TCP/network) upstreams** — the `Cluster.Dial` chokepoint is shared with tcp_proxy; the 43.1 permit gates connection creation where a `circuitBreaker` is configured, but the differential + focus is HTTP (H1/H2); TCP-cluster budget behavior is incidental + not differential-pinned.
- **Idle-connection timeout / pool drain on idleness; upstream H1 pipelining; the `/clusters` admin pool-size readout.**

---

## 3. The `circuitBreaker` extension + the `max_connections` hard cap + the pending wait-queue + the queue-full 503 (ADR-0252)

### 3.0 Split disposition — leg 1 of the by-concern split; FINAL ADR-0045 re-check at PLAN

43.1 = the `activeConnections`/`pendingRequests` extension + the `max_connections` hard cap + the bounded wait-queue + the queue-full 503 + the stat activation/additions + `0078`. Anticipated ~250–400 prod LoC / ~14–18 tasks (the pending wait-queue concurrency is the load-bearing risk) — under the ADR-0045 hard gate (`> ~25 tasks OR > ~1500 LoC`); the PLAN runs the FINAL split-gate re-check (a leg may sub-split if its as-scoped size warrants — the 42.2a/42.2b precedent; anticipated NO further split).

### 3.1 The per-priority accounting extension

Extend the phase-41 `cbPriority` struct (`internal/cluster/circuitbreaker.go`):
```go
type cbPriority struct {
	maxRequests    int64        // phase 41 — the enforced max_requests budget
	activeRequests atomic.Int64 // phase 41

	maxConnections     int64        // 43.1 — the HARD connection cap (default 1024 — AMEND-CP5)
	maxPendingRequests int64        // 43.1 — the bounded wait-queue cap (default 1024)
	activeConnections  atomic.Int64 // 43.1 — live upstream connections for this priority
	pendingRequests    atomic.Int64 // 43.1 — current pending-queue depth

	rqOpen        *stats.Gauge // phase 41 (LIVE default)
	cxOpen        *stats.Gauge // 43.1 — ACTIVATED (1 while activeConnections >= maxConnections)
	rqPendingOpen *stats.Gauge // 43.1 — ACTIVATED (1 while pendingRequests >= maxPendingRequests)
	// cxPoolOpen/rqRetryOpen stay emit-0 (deferred).
}
```
Cluster-level handles added beside the phase-41 overflow counters: `upstreamCxOverflow` (ACTIVATED), `upstreamRqPendingActive` (NEW gauge), `upstreamRqPendingTotal` (NEW counter). The pending wait-queue itself is a per-priority coordination primitive (a `sync.Cond` or a buffered-channel semaphore — the exact mechanism is a PLAN pin, D-S431-1) carried on the `circuitBreaker`. The struct carries NO per-host state (cluster-wide per-priority accounting — the phase-41 discipline).

### 3.2 The `max_connections` hard cap + the pending wait-queue lifecycle (synchronous, blocking-then-fail-fast)

The enforcement is an overlay on the EXISTING connection-CREATION boundary — `Cluster.Dial` (the raw-dial chokepoint, used by tcp_proxy + `DialH2`) + the `AcquireH1` pool-MISS path (`cluster.go`) — NOT the request-admission point (the phase-41 `max_requests` overlay sits there; `max_connections` is connection-scoped, the deliberate contrast):

1. **Pool HIT needs no permit.** `AcquireH1` returning a pooled idle conn reuses an already-counted connection (idle pooled conns keep `upstream_cx_active` incremented) ⇒ no `max_connections` permit consulted (byte-neutral for the hot keep-alive path).
2. **Connection creation try-acquires a permit (hard cap).** Before dialing a NEW connection (pool-miss / `Dial`), if `c.circuitBreaker != nil`, call `cb.acquireConnOrPend(DEFAULT, ctx)`:
   - atomically: if `activeConnections < maxConnections` ⇒ `activeConnections++`, return `acquired` (proceed to dial; the dial increments `upstream_cx_total`/`upstream_cx_active` as today).
   - else (cap reached) ⇒ set `cxOpen = 1`, `upstreamCxOverflow++` (the soft-signal parity), and ENTER THE WAIT-QUEUE (step 3).
3. **The bounded pending wait-queue.** On cap-reached: if `pendingRequests < maxPendingRequests` ⇒ `pendingRequests++`, `upstreamRqPendingActive++`, `upstreamRqPendingTotal++`, set `rqPendingOpen = 1` (when `pendingRequests >= maxPendingRequests`), then BLOCK the request goroutine on the wait primitive until (a) a connection permit frees ⇒ acquire it + dial, or (b) `ctx` cancels ⇒ release the pending slot + return the context error (the per-attempt/downstream-cancel path). On wake-to-acquire ⇒ `pendingRequests--`, `upstreamRqPendingActive--`, clear `rqPendingOpen` when below cap.
   - else (queue full) ⇒ FAIL-FAST: `upstreamRqPendingOverflow++` (AMEND-CP2), return the overflow sentinel ⇒ the action emits the `503` local-reply IMMEDIATELY (the `router.go` `AcquireH1`-failure `ActionResponse{Status: 503, ...}` shape) with the `UO` flag (AMEND-CP4), NO connection made.
4. **Release wakes a waiter.** A connection permit frees on conn Close (the `connWithGauge` close path — `activeConnections--`, clear `cxOpen` when below cap, signal one waiter). An idle H1 conn returning via `PutIdleH1` ALSO signals a waiter (the waiter can grab the idle conn without a new permit — the wake-on-idle-return seam). The exact wake fairness (FIFO vs LIFO) + the permit-free-vs-idle-return wake coordination + the race-free `acquire`-after-wake are PLAN/IMPL pins (D-S431-1).

`maxConnections`/`maxPendingRequests` are the parsed `Thresholds[DEFAULT]` budgets (or the 1024 default). All counters are DEFAULT-priority (every request is DEFAULT at 43.1). The HIGH budget parses but is never consulted.

### 3.3 Byte-stability

When `c.circuitBreaker == nil` (no `circuit_breakers`), `acquireConnOrPend` is not called (the nil-guard at the connection-creation boundary) ⇒ the dial/pool path is **byte-identical to today** (every existing fixture stays green). The activated + new stats register ONLY on `circuit_breakers` clusters (the phase-41 scoping). The H1 pool-HIT path is unchanged (no permit) ⇒ keep-alive reuse stays byte-stable.

---

## 4. Framework primitives — the budget/queue extension over the phase-41 substrate + 0 new packages + 0 new go.mod deps

- NEW: the `activeConnections`/`pendingRequests` counters + the `maxConnections`/`maxPendingRequests` budgets + the pending wait-queue primitive + `acquireConnOrPend`/`releaseConn`/`wakeWaiter` in `internal/cluster/circuitbreaker.go` (+ possibly a sibling `connpool.go` for the wait-queue — a PLAN call); the activated `cxOpen`/`rqPendingOpen`/`upstreamCxOverflow` handles + the new `upstreamRqPendingActive`/`upstreamRqPendingTotal`; the permit acquire-or-pend at `Cluster.Dial` + the `AcquireH1` pool-miss; the release+wake at the `connWithGauge` close + `PutIdleH1`; the parse of `max_connections`/`max_pending_requests` into `cbPriority` (`parseCircuitBreakers`, currently parses-but-stores-no-field); the overflow 503 local-reply (reuse the phase-41 path).
- REUSED: the phase-41 `circuitBreaker`/`cbPriority` struct + `registerStats` + the `upstream_rq_pending_overflow` LIVE handle + the overflow-503 local-reply shape; the existing H1 LIFO pool (`h1Pool`/`AcquireH1`/`PutIdleH1`); the `connWithGauge` close path (already driving `upstream_cx_active`); the LB Pick/release closure (ADR-0232/0235 — preserved per pooled conn); the `BlockingHoldResponder` BackendKind 36 + `/__release`; the `reference_docker_probe_bridge_network` differential pattern.
- ZERO new Go packages. ZERO new go.mod modules (`cluster.v3.CircuitBreakers` already a dep; `go mod tidy -diff` EMPTY — D-CP-PROTO).

---

## 5. Proto-field roster (per §11 D-CP-PROTO)

`Cluster.circuit_breakers` = `Cluster` field 10 → `cluster.v3.CircuitBreakers` (the phase-41 roster, UNCHANGED). 43.1 changes only the `Thresholds` field DISPOSITION:

| # | Field | Type | 41 disposition | 43.1 disposition |
|---|-------|------|----------------|------------------|
| 2 | `max_connections` | UInt32Value (default 1024) | PARSE-ACCEPT, emit-0 | **ENFORCED — hard cap** (AMEND-CP1); `cx_open` LIVE |
| 3 | `max_pending_requests` | UInt32Value (default 1024) | PARSE-ACCEPT, emit-0 | **ENFORCED — bounded wait-queue** (AMEND-CP1); `rq_pending_open` LIVE |
| 4 | `max_requests` | UInt32Value (default 1024) | ENFORCED (phase 41) | UNCHANGED (phase-41 `max_requests` keystone) |
| 1,5,6,7,8 | priority / max_retries / track_remaining / max_connection_pools / retry_budget | — | (phase-41 dispositions) | UNCHANGED |

`go mod tidy -diff` EMPTY → ZERO new module.

---

## 6. PARSE-REJECT roster (per §11 D-CP-REJECT)

NO new reject arms (AMEND-CP6). `max_connections`/`max_pending_requests` are existing `Thresholds` fields phase 41 already parses + reject-covers (priority-range + duplicate-priority + retry_budget-percent, ADR-0080). The budget `UInt32Value`s stay unbounded (`max_connections: 0`/`max_pending_requests: 0` ⇒ valid configs — 0 connections ⇒ every request overflows once the queue [also 0?] is full; the `max_requests:0` precedent). NO new fuzzer.

---

## 7. Stat surface — activate 3 + add 2 (1181 → ~1199) (per §11 D-CP-STATS + AMEND-CP3)

Scoped to `circuit_breakers` clusters (the phase-41 departure; existing non-CB fixtures unaffected). Two changes:

**A. ACTIVATE 3 phase-41 emit-0 handles (NO new names):**
- `circuit_breakers.default.cx_open` — gauge → LIVE (1 while `activeConnections >= max_connections`).
- `circuit_breakers.default.rq_pending_open` — gauge → LIVE (1 while `pendingRequests >= max_pending_requests`).
- `upstream_cx_overflow` — counter → LIVE (++ when a connection creation hits the `max_connections` cap and the request pends; the soft-signal parity — AMEND-CP1).

**B. ADD 2 new pending-queue names (`cluster.<n>.`), scoped to CB clusters:**
- `upstream_rq_pending_active` — gauge (current pending-queue depth).
- `upstream_rq_pending_total` — counter (cumulative requests that entered the queue).

`upstream_rq_pending_overflow` stays LIVE (the queue-full 503 counter — already wired by phase 41 for `max_requests`; AMEND-CP2). The `high.*` subtree + `cx_pool_open`/`rq_retry_open` + `upstream_cx_pool_overflow` STAY emit-0; the `remaining_*` gauges STAY unregistered (`track_remaining`-gated). Surface anticipated **1181 → ~1199** (+2 new names on the existing `0074` CB cluster + the `0078` cluster's full ~16-name block); the EXACT figure is a PLAN/IMPL pin.

---

## 8. Differential fixture taxonomy (+1: `0078` cross-side robust-invariant + subject-side exact)

### 8.1 `0078-connection-pool-max-connections` (cross-side; REUSE `BlockingHoldResponder` 36)

An HTTP listener → a cluster `c_cp {1 BlockingHoldResponder backend}` with `circuit_breakers: { thresholds: [ { priority: DEFAULT, max_connections: <N>, max_pending_requests: <M> } ] }`, on BOTH the subject and the reference (`contrib-v1.37.2`). The driver (SLEEPLESS — the release-barrier + poll-to-converge pattern, NO `time.Sleep`; sequential-per-side — the shared in-process backend idle between sides):

1. Fire **N concurrent** held requests ⇒ each occupies one connection slot. **POLL `/stats` until `cluster.c_cp.circuit_breakers.default.cx_open == 1` on BOTH sides** (the connections-saturated barrier — robust: the reference reliably sets `cx_open=1` once `cx_active >= max_connections`).
2. **STAGE the pend phase before the overflow phase** (so the subject-exact prong is itself deterministic — concurrent firing of all further requests would race the pend/overflow split even subject-side): fire **M** further held requests ⇒ on the SUBJECT they PEND; **poll the SUBJECT until `circuit_breakers.default.rq_pending_open == 1` + `upstream_rq_pending_active == M`** (the queue-filled barrier — SUBJECT-side exact). THEN fire **J** oversubscribers (J ≥ 1) ⇒ each finds the queue full ⇒ overflows. On the REFERENCE the pend/overflow split is timing-dependent (NOT asserted cross-side — AMEND-CP1 recorded departure); the reference prong only needs the oversubscription heavy enough to guarantee ≥1 overflow (step 3).
3. **Cross-side ROBUST assert:** under the oversubscription, BOTH sides produce ≥1 downstream **503** (asserted on the DOWNSTREAM class — `downstream_rq_2xx`/the 503, per `reference_concurrent_attempt_downstream_class_assertion`, NOT the over-counting upstream class) + `cluster.c_cp.upstream_rq_pending_overflow` delta ≥ 1 + `circuit_breakers.default.cx_open == 1`. Verify `upstream_cx_total > 0` reference-side (path-ran guard).
4. **SUBJECT-side EXACT assert:** `upstream_cx_active` never exceeds N (the hard cap); `upstream_rq_pending_active` peaks at M; exactly J requests overflow (queue-full); `upstream_rq_pending_total == M` EXACTLY (all connections are held until `/__release`, so no pend slot churns mid-test — no hedge).
5. **Release the barrier** (`/__release`) ⇒ the held requests drain (admitted ones complete 2xx) ⇒ poll the gauges back to 0 ⇒ cross-side parity on the final `cx_open == 0` + `rq_pending_open == 0`.

2 `-count=1` deliberate breaks: (A) `max_connections` NOT hard-capped (the permit always granted) ⇒ SUBJECT `upstream_cx_active` exceeds N / no pend ⇒ the subject-exact assert FAILS; (B) the wait-queue NOT bounded (no queue-full 503) ⇒ no overflow ⇒ the cross-side overflow-delta assert FAILS. The constants (N / M / J / convergeDeadline / refContainerListenerPort) single-sourced (`reference_fixture_workload_constant_desync`). 20-run flake gate (the release-barrier ⇒ flake-free). ONE fixture — 43.1 enforces the cx/pending budgets; the H2 multiplex differential (`0079`) is a 43.2 obligation.

### 8.2 NO new BackendKind / NO new fuzzer

REUSE `BlockingHoldResponder` 36 (AMEND-CP7) — BackendKind tail STAYS **36**. `circuit_breakers` is config-parse (no new wire decoder) ⇒ fuzzers STAY **42** (AMEND-CP6).

---

## 9. Behavior-contract delta (the 43.1 bundle; ADR-0052 atomic landing)

A new `### Cluster — connection-pool budgets (max_connections / max_pending_requests)` subsection in BEHAVIOR_CONTRACT.md: the `max_connections` HARD cap on connection creation (try-acquire at the dial boundary; pool-HIT needs no permit); the bounded pending wait-queue (block the request goroutine, woken on permit-free/idle-return; DEFAULT-only); the queue-full fail-fast 503 + `UO` + `upstream_rq_pending_overflow` (the shared counter — AMEND-CP2) + the `cx_open`/`rq_pending_open` gauges; the new `upstream_rq_pending_active`/`upstream_rq_pending_total`; the RECORDED DEPARTURE (AMEND-CP1 — the reference's soft `max_connections`; envoy-go's clean hard cap; exact counts subject-side); byte-identical when no `circuit_breakers`. The stat-surface block advances 1181 → ~1199.

---

## 10. Per-task structure (~14–18 tasks; PLAN decomposes)

Anticipated spine: (1) baselines/PROGRESS; (2) parse `max_connections`/`max_pending_requests` into `cbPriority` (defaults 1024) + unit tests; (3) the pending wait-queue primitive (the `sync.Cond`/semaphore design — the load-bearing concurrency piece) + unit tests (`-race`: acquire/pend/wake/queue-full/ctx-cancel); (4) the `activeConnections` hard-cap acquire + `cxOpen`/`upstreamCxOverflow` + unit tests; (5) the activated + new stat registrations/handles (1181→~1199) + unit tests; (6) the permit acquire-or-pend at `Cluster.Dial` + the `AcquireH1` pool-miss (pool-HIT no permit); (7) the release+wake at the `connWithGauge` close + `PutIdleH1`; (8) the queue-full 503-UO local-reply; (9) the `0078` fixture (cross-side robust + subject-side exact); (10) `0078` deliberate-breaks + 20-run flake + `-race`; (11) full 80-dir differential + six-gate; (12) ADR-0252 body + BEHAVIOR_CONTRACT; (13) completion bundle + ROADMAP 43.1 leg → `done`. The PLAN runs the FINAL ADR-0045 split-gate re-check (anticipated NO further split) + pins the wait-queue concurrency design (D-S431-1).

---

## 11. SPEC-time empirical-pin block (D-CP-* — executed IN-SESSION 2026-06-22)

All pins executed live against `envoyproxy/envoy:contrib-v1.37.2` per `reference_docker_probe_bridge_network` (a shared bridge `cp43probe`; a go-httpbin `/delay/N` backend AND a deterministic raw-TCP hold backend; staged concurrent load + `/stats`-delta scrapes; the request path verified `upstream_cx_total>0`).

| Pin | Disposition |
|-----|-------------|
| **D-CP-PROTO** | CONFIRMED. `max_connections`/`max_pending_requests` = the phase-41 `Thresholds` fields 2/3; defaults 1024/1024 (via `remaining_cx`=2/`remaining_pending`=2 at configured 2, `remaining_rq`=1024 at default); `go mod tidy -diff` EMPTY → ZERO new module. |
| **D-CP-LIFECYCLE** | PINNED (the headline — AMEND-CP1). The reference's `max_connections` is SOFT: `upstream_cx_active` EXCEEDED the cap (max_connections=2 → cx_active observed 3,5,8,9 with timing slop), `cx_open`→1 + `upstream_cx_overflow`++, request NOT rejected/pended by it. The `503` comes from `upstream_rq_pending_overflow` whose count is timing-sensitive (3-of-6, 5-of-6 — NOT a clean formula; cross-side EXACT infeasible). `rq_pending_open`=1 iff `pending_active >= max_pending_requests`. ⇒ envoy-go implements a CLEAN HARD CAP + bounded queue (user-confirmed departure); robust cross-side + subject-side-exact differential. |
| **D-CP-STATS** | PINNED. A connection-pooled cluster emits, beyond phase-41's +14, `upstream_rq_pending_active` (gauge) + `upstream_rq_pending_total` (counter) — envoy-go does NOT register them (codebase grep EMPTY). 43.1 activates `cx_open`/`rq_pending_open`/`upstream_cx_overflow` + adds the 2 new names (CB-scoped). `track_remaining` adds `remaining_{cx,pending,rq,retries,cx_pools}` (DEFAULT-only) — STAY deferred. Surface ~1181 → ~1199 (PLAN-pinned exact). |
| **D-CP-REJECT** | PINNED. NO new reject arm (the budgets are existing phase-41-parsed fields; ADR-0080 arms cover); budget values unbounded (0 valid); NO new fuzzer (AMEND-CP6). |
| **D-CP-BACKEND** | PINNED. REUSE `BlockingHoldResponder` 36 + `/__release` (held connection = a deterministic connection slot); NO new BackendKind. The go-httpbin keep-alive/fast-establish was the probe NOISE source; a deterministic raw-hold backend gave stable, reproducible numbers — the `0078` driver mirrors it. |
| **D-CP-DIFFERENTIAL** | PINNED. ONE fixture `0078-connection-pool-max-connections`; cross-side ROBUST (`cx_open`/`rq_pending_open` flip + ≥1 downstream-class 503 + `upstream_rq_pending_overflow` delta) + SUBJECT-side EXACT (hard cap, queue depth, overflow count); 2 deliberate breaks; the shared `BlockingHoldResponder` over the bridge net (§8). |
| **D-CP-H2POOL** | DEFERRED to the 43.2 SPEC (the H2 multiplex pool leg). |

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S431-1** the pending wait-queue concurrency design — the wake primitive (`sync.Cond` vs a buffered-channel semaphore), the wake fairness (FIFO vs LIFO), the permit-free-vs-idle-return wake coordination, the race-free acquire-after-wake, the ctx-cancel-while-pending release; the `-race`-clean discipline (the load-bearing IMPL risk — the 42.2b hedging-collector precedent).
- **D-S431-2** whether the permit logic lives in `Cluster.Dial` + the `AcquireH1` pool-miss as two call sites, or is refactored into a shared `createConn`-with-permit helper; whether a sibling `connpool.go` houses the wait-queue.
- **D-S431-3** the exact surface arithmetic (does `upstream_rq_pending_active`/`_total` register on every CB cluster ⇒ `0074` gains +2; the `0078` block size) — pin 1181 → the exact figure.
- **D-S431-4** the `UO` response-flag emission (the phase-41 D-S41-3 recorded-departure carry; the differential does NOT depend on it).
- **D-S431-5** `0078` constants (N / M / J / convergeDeadline / held-count / refContainerListenerPort) single-sourced; the staged drive order (fill conns → poll `cx_open` → fill queue → poll `rq_pending_open`/`pending_active==M` → fire J oversubscribers); the exact subject-side-exact assertions (cap never exceeded / queue depth M / J overflow).
- **D-S431-6** `max_connections: 0` / `max_pending_requests: 0` disposition (a valid all-overflow config — confirm the queue-full path fires immediately) + whether TCP-cluster `circuit_breakers` engage the permit (incidental; not differential-pinned).
- **D-S431-7** the FINAL ADR-0045 split-gate re-check (anticipated NO further split).

---

## 13. ADR continuity — the ADR-0252 §Context DRAFT (anchored here; full entry lands at the 43.1 IMPL)

**ADR-0252 §Context (draft).** Phase 41 (circuit breakers) landed the `max_requests` keystone fail-fast and PARSED-but-DEFERRED `max_connections` + `max_pending_requests` enforcement, recording that the reference's `max_connections` is a soft throttle that pends rather than 503s, and that `max_pending_requests` needs the connection-pool queue envoy-go's synchronous-acquire-per-request model lacks — both deferred to the per-protocol connection-pooling row. Phase 43.1 is that row's first by-concern leg: it builds the pending wait-queue (the project's first standing upstream request queue) and the `max_connections` cap. The §11 live pins (2026-06-22, `contrib-v1.37.2`) then revealed that the reference's `max_connections` does NOT cleanly hard-cap (connections exceed the budget with timing slop; `cx_open`+`upstream_cx_overflow` are soft signals) and the `upstream_rq_pending_overflow` 503 count is timing-sensitive — so a cross-side EXACT differential is infeasible. The user-confirmed design: envoy-go implements a CLEAN HARD CAP on connection creation (the deliberate, more-correct departure) + a bounded pending wait-queue that blocks the request goroutine (woken on connection-free/idle-return) + a fail-fast 503 (`upstream_rq_pending_overflow`, the shared counter) on queue-full; the budget/queue EXTENDS the phase-41 `circuitBreaker` per-priority struct (`activeConnections`/`pendingRequests` + the two budgets); the H1 pool-HIT path needs no permit (byte-neutral keep-alive); the cx/pending gauges activate LIVE + `upstream_rq_pending_active`/`_total` are added; byte-identical when no `circuit_breakers`. The differential asserts robust cross-side invariants (gauges flip; ≥1 downstream-class 503; overflow delta) + subject-side exact cap/queue behavior; the exact cross-side counts are a recorded departure. §Decision + §Consequences land at the 43.1 IMPL.

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

Counts UNCHANGED at this SPEC: stat surface **1181** / fixtures **79** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0251** (next-free **ADR-0252**). ROADMAP row 43 STAYS `in-progress` (the 43.1 SPEC note appended). Anticipated at the 43.1 IMPL: fixtures 79 → 80 (`0078`), BackendKind tail 36 (UNCHANGED — REUSE `BlockingHoldResponder`), DECISIONS tail ADR-0251 → ADR-0252 (next-free ADR-0253), stat surface 1181 → ~1199 (PLAN-pinned), fuzzers 42 (UNCHANGED), ZERO new packages + ZERO new go.mod modules. Next → the 43.1 PLAN (`superpowers:writing-plans`), then the 43.1 IMPL, then the 43.2 (H2 multiplex pool) BRAINSTORM/SPEC/PLAN/IMPL.
