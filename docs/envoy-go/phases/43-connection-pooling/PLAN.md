# Connection-Pool Budgets (`max_connections` hard-cap + `max_pending_requests` wait-queue) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the upstream connection-pool budget — a cluster with `circuit_breakers{max_connections, max_pending_requests}` HARD-caps concurrent upstream connections at `max_connections`; a request needing a new connection at the cap joins a bounded pending wait-queue (blocking its own goroutine, woken on conn-Close / idle-return); queue-full fails fast with a `503` + `upstream_rq_pending_overflow`. The project's FIRST standing upstream request queue.

**Architecture:** A per-cluster DEFAULT-priority `connPool` (new `internal/cluster/connpool.go`) carrying the `max_connections`/`max_pending_requests` budgets + a `sync.Mutex`-guarded `activeConns` counter + a FIFO slice of buffered cap-1 wake channels. A connection-creation permit is try-acquired at the connection-CREATION boundary (`Cluster.Dial` raw-dial + the `AcquireH1` pool-MISS; a pool HIT reuses an already-counted conn ⇒ NO permit). At the cap the request PENDS (bounded by `max_pending_requests`), woken via a direct permit handoff on conn-Close (the `connWithGauge` dec closure) or on idle-return (`PutIdleH1` closes-and-wakes when a waiter is queued — routing both wake sources through the single permit mechanism). Queue-full ⇒ a `cluster.IsConnPoolOverflow` sentinel ⇒ a fail-fast `503`. Byte-identical when no `circuit_breakers` (a nil-guard). A recorded DEPARTURE from the reference's soft `max_connections` (AMEND-CP1).

**Tech Stack:** Go (`sync.Mutex` + buffered-channel wake; the 42.2b `-race`-clean single-mutator discipline); `cluster.v3.CircuitBreakers` (go-control-plane); the `internal/stats` registry; the `test/differential` cross-side harness (reference `envoyproxy/envoy:contrib-v1.37.2` over a Docker bridge per `reference_docker_probe_bridge_network`).

This PLAN implements `docs/envoy-go/phases/43-connection-pooling/SPEC.md` (read it first). Counts at PLAN commit UNCHANGED (stat surface **1181** / fixtures **79** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0251**, next-free **ADR-0252**). All `internal/cluster` paths are package `cluster`; all line anchors below are verified against the worktree at `7db26065`.

---

## D-question resolutions (SPEC §12) — settled at PLAN

The implementer MUST follow these (baked into the tasks).

### D-S431-1 — the pending wait-queue concurrency design  ★ THE load-bearing risk

A `connPool` (sibling `connpool.go`) with a **`sync.Mutex` + a FIFO slice of per-waiter buffered (cap-1) channels** — NOT a `sync.Cond` (cannot `select` on `ctx.Done()`) and NOT a pre-allocated permit-token channel (a `max_connections`-sized channel is wasteful at the 1024 default and complicates the idle-return wake). The discipline mirrors the 42.2b hedging collector: **all state transitions under `mu` (single mutator); the only out-of-lock operation is the buffered cap-1 wake send, which never blocks; no loser ever blocks.**

- **Wake fairness: FIFO** (append to tail, hand off from head). Fairness is NOT differential-pinned (the reference is racy); FIFO is the deterministic, starvation-free choice.
- **Permit-free wake (conn-Close):** `releaseConn` hands the freed permit DIRECTLY to the head waiter — `activeConns` is unchanged (the permit transfers; `cx_open` stays set), the waiter is dequeued and signalled, and on wake it KNOWS it owns a reserved permit and dials. With no waiter, `activeConns--` and `cx_open` clears when back under the cap. This direct handoff (all under `mu`) eliminates lost-wakeups and the thundering-herd barge.
- **Idle-return wake (`PutIdleH1`):** routed through the SAME permit mechanism — when a waiter is queued, `PutIdleH1` CLOSES the idle conn (instead of pooling it); its `connWithGauge.Close` runs the dec closure → `releaseConn` → hands a permit to the head waiter, which dials fresh. This avoids a second (conn-handoff) grant type and the H1-pooled-conn-vs-Dial-waiter type mismatch. Cost: one extra dial under contention on a keep-alive workload — unobserved by the differential (the `0078` backend sends `Connection: close`, so `PutIdleH1` is not even hit there; this wake exists for keep-alive correctness — without it a keep-alive workload could stall a waiter forever).
- **Race-free acquire-after-wake:** the woken waiter receives on its cap-1 channel and returns `nil` (permit held). No re-check of the cap (the handoff already reserved the permit).
- **ctx-cancel-while-pending:** the waiter `select`s on `ctx.Done()`. On cancel it re-locks and `removeWaiterLocked`; if STILL queued ⇒ cancelled cleanly (decrement `pending_active`, clear `rq_pending_open`), return `ctx.Err()`. If ALREADY dequeued (a handoff raced the cancel) ⇒ a permit is en route on the buffered channel: drain it (`<-ch`) and `releaseConn()` to give it back (may wake the next waiter), then return `ctx.Err()`. This drain-and-give-back is the load-bearing correctness pattern — without it the raced permit leaks.

Unit tests (Task 3) MUST exercise acquire / pend / wake / queue-full / ctx-cancel under `-race -count=1` (the `reference_differential_break_protocol_count1` caching discipline applies to every `-race` run too).

### D-S431-2 — code placement

- **Sibling `internal/cluster/connpool.go`** houses the `connPool` type + `acquireConnOrPend`/`releaseConn` + helpers. The `circuitBreaker` struct (in `circuitbreaker.go`) gains ONE field `pool *connPool` (DEFAULT-priority only; built whenever `circuit_breakers` is present, since the budgets default to 1024). The `cbPriority` struct is UNCHANGED (it stays the per-priority `max_requests` accounting; the connection budget is connection-scoped, not request-scoped — the deliberate contrast).
- **Shared `Cluster` helpers** (in `cluster.go`) avoid duplicating the permit logic across the two creation sites: `acquireConnSlot(ctx) error` (nil-guarded), `releaseConnSlot()` (the dial-failure-path release), and `connDec(release func()) func()` (composes `upstreamCxActive.Dec()` + the LB `release()` + `pool.releaseConn()` into the single `connWithGauge` dec closure). `Cluster.Dial` and `AcquireH1`'s MISS path each call these (≈4 inserted lines per site); the HIT path returns BEFORE `acquireConnSlot` (no permit).

### D-S431-3 — the EXACT stat-surface arithmetic: 1181 → **1183**

The "stat surface" total counts distinct stat-name **shapes** (verified: phase-41 added the 14 `circuit_breakers.*` shapes once → 1149→1163; the `0074` fixture's own cluster contributed NO base-name delta, confirming shape-counting, not per-fixture-instance summing). Therefore:
- **ACTIVATE 3 existing shapes** (`circuit_breakers.default.cx_open`, `circuit_breakers.default.rq_pending_open`, `upstream_cx_overflow`) — already registered emit-0 by phase 41 ⇒ **+0 new shapes** (only handle-storage + live driving change).
- **ADD 2 new shapes** (`upstream_rq_pending_active` gauge, `upstream_rq_pending_total` counter), CB-scoped ⇒ **+2**.
- The new `0078` fixture's CB cluster (`c_cp`) reuses the SAME shapes ⇒ **+0**.

⇒ **1181 → 1183.** (The SPEC's "~1199" was a loose per-fixture-sum over-estimate; the shape-count convention is authoritative.) `circuitBreakerStatNames()` (the registration-test helper, `circuitbreaker_test.go`) grows from 14 → 16 names; `TestRegisterCircuitBreakerStats_Present` asserts all 16. Every existing CB cluster (`0074` is the only one) gains the 2 new names at value 0 (byte-stable cross-side — the reference emits them too per D-CP-STATS; the full-differential gate in Task 10 confirms).

### D-S431-4 — the `UO` response flag (recorded DEPARTURE carry)

envoy-go has no access-log response-flag plumbing (phase-41 D-S41-3: `ActionResponse` has no flags field; `RESPONSE_FLAGS` hardcodes `"-"`). The overflow `503` carries the observable contract (status + `upstream_rq_pending_overflow` + `cx_open`/`rq_pending_open`); the `UO` flag is **DEFERRED, a recorded departure** (ADR-0252 §Consequences + BEHAVIOR_CONTRACT). The `0078` differential NEVER asserts the access-log line.

### D-S431-5 — `0078` constants + staged drive + the sticky-release backend  ★ load-bearing fixture detail

One `const`/`var` block at the top of the `0078` driver (`reference_fixture_workload_constant_desync`): `fixtureName`, `refContainerListenerPort = 19167` (the NEXT-FREE port — verified the max in-use is 19166; re-confirm via `grep -rh refContainerListenerPort test/fixtures/ | grep -o '[0-9]\{4,5\}' | sort -un | tail -1`), `refAdminPort = 9901`, `clusterName = "c_cp"`, `maxConnections = 2` (N), `maxPendingRequests = 2` (M), `oversub = 2` (J), `refOversub = M + J + 4` (reference burst slack — the reference's soft breaker needs heavier oversubscription to GUARANTEE ≥1 overflow), `backendCount = 1`, `convergeDeadline`, `convergePoll`. The asserter + bootstrap/config builders read these — no hand-rolled duplicates.

**The sticky-release requirement (load-bearing):** the M pending waiters, on wake (after the N held conns release), dial FRESH connections to the backend. The phase-41 `acceptBlockingHold` RE-ARMS its gate after `/__release` — so a post-release dial would block on a fresh gate and the drain would STALL (the M backend-held conns never complete ⇒ `cx_open` never returns to 0). Resolution: add a **sticky-release control path `/__release_sticky`** to `acceptBlockingHold` (`runner_test.go`) that permanently opens the gate (all current AND future requests pass immediately). `0074` keeps using the re-armable `/__release` — its behavior is UNCHANGED (the sticky path is additive; BackendKind tail STAYS 36 per AMEND-CP7 — a richer control surface on the existing kind, not a new kind). The `0078` drain hits `/__release_sticky`.

**Staged drive (sequential-per-side; the shared in-process backend idle between sides):**

| Step | SUBJECT (exact) | REFERENCE (robust) |
|---|---|---|
| 1 | Fire **N** concurrent held `GET /` → poll `/stats` until `circuit_breakers.default.cx_open == 1` AND `upstream_cx_active == N` | Fire **N** concurrent held `GET /` → poll until `cx_open == 1` (cx_active may EXCEED N — soft; not asserted) |
| 2 | Fire **M** further held `GET /` (they PEND) → poll until `rq_pending_open == 1` AND `upstream_rq_pending_active == M` | — (skip; the reference pend/overflow split is timing-dependent) |
| 3 | Fire **J** oversubscribers → each finds the queue full ⇒ `503` | Fire **refOversub** concurrent oversubscribers → ≥1 gets `503` |
| 4 (subject-exact) | `upstream_cx_active == N` throughout (never exceeds the cap); `rq_pending_active` peaked at M; exactly J got `503`; `upstream_rq_pending_total == M` EXACTLY (conns held until release ⇒ no pend-slot churn) | — |
| 4 (cross-side robust) | both sides: `cx_open == 1` at saturation; ≥1 downstream-class `503` (observed from the fired oversubscribers' status codes per `reference_concurrent_attempt_downstream_class_assertion` — NOT `upstream_rq_5xx`); `upstream_rq_pending_overflow` delta ≥ 1; reference `upstream_cx_total > 0` (decode-ran guard) | same |
| 5 | `/__release_sticky` → all held + woken requests drain to `200` → poll gauges back to 0 | same → cross-side parity on final `cx_open == 0` + `rq_pending_open == 0` |

### D-S431-6 — `max_connections: 0` / `max_pending_requests: 0` + TCP disposition

`max_connections`/`max_pending_requests` are `*wrapperspb.UInt32Value`. **Absent ⇒ 1024 default** (AMEND-CP5). **Explicit 0 ⇒ that value.** `max_connections: 0` ⇒ `activeConns(0) < 0` is false ⇒ every creation hits the cap branch (`cx_open=1`, `cx_overflow++`); if `max_pending_requests` is also small/0 the request overflows immediately (a valid all-overflow config — the `max_requests:0` precedent). `releaseConn` is never reached at `max_connections:0` (no permit ever acquired). Unit-tested in Task 3 (the `max_connections:0`+`max_pending_requests:0` immediate-overflow case + `max_connections:0`+`max_pending_requests:1` pend-then-ctx-cancel case). **TCP clusters:** `Cluster.Dial` is shared with tcp_proxy/redis/thrift; a `circuit_breakers`-configured TCP cluster DOES engage the permit (incidental), but a TCP overflow surfaces as a dial error → the filter closes the connection (NOT differential-pinned per SPEC §2). No special-casing.

### D-S431-7 — final ADR-0045 split-gate re-check

Anticipated production LoC (below): `connpool.go` ~150 + `circuitbreaker.go` ~25 + `cluster.go` ~40 + router edits ~30 ≈ **~245 prod LoC** across ~5 prod files; **12 tasks**. Both axes comfortably under the gate (`> ~25 tasks OR > ~1500 LoC`). **NO FURTHER SPLIT** — 43.1 ships as one leg (the 42.2a/42.2b precedent did not bind here).

---

## File structure

**Production (`internal/`):**
- `internal/cluster/connpool.go` (CREATE) — the `connPool` struct; `acquireConnOrPend(ctx) error` / `releaseConn()` / `hasWaiter()` + the locked helpers (`removeWaiterLocked`, `clearRqPendingOpenLocked`) + the nil-guarded stat helpers; the `errConnPoolOverflow` sentinel + the exported `IsConnPoolOverflow(err) bool` predicate.
- `internal/cluster/circuitbreaker.go` (MODIFY) — the `pool *connPool` field on `circuitBreaker`; the `max_connections`/`max_pending_requests` parse (defaults 1024) + `out.pool = &connPool{...}` in `parseCircuitBreakers`; the `registerStats` activation (store `cxOpen`/`rqPendingOpen`/`upstreamCxOverflow` into `cb.pool`; add `upstream_rq_pending_active`/`_total`).
- `internal/cluster/cluster.go` (MODIFY) — `acquireConnSlot`/`releaseConnSlot`/`connDec` helpers; the permit overlay in `Dial` (~:401-436) + `AcquireH1` MISS (~:490-518); the `PutIdleH1` close-and-wake (~:528-548).
- `internal/filter/http/router/router.go` (MODIFY) — the `IsConnPoolOverflow` → 503 branch at the `AcquireH1` failure site (~:642) + the legacy `Dial` site (~:813).
- `internal/filter/http/router/router_h2.go` (MODIFY) — the `IsConnPoolOverflow` → 503 branch at the `DialH2` failure site (~:98) + the legacy `doH2` `DialH2` site (~:306).

**Test harness (`test/`):**
- `test/differential/runner_test.go` (MODIFY) — the `/__release_sticky` control path in `acceptBlockingHold` (additive; `/__release` UNCHANGED).
- `test/fixtures/0078-connection-pool-max-connections/driver/driver.go` + `driver_test.go` + `expectations.yaml` + `README.md` (CREATE).

**Docs:**
- `docs/envoy-go/DECISIONS.md` (ADR-0252 body), `BEHAVIOR_CONTRACT.md` (connection-pool subsection + stat-count 1181 → 1183), `STATE.md`, `ROADMAP.md`, `next-prompt.txt`, the phase `PROGRESS.md` + `README.md`.

---

## Task 1: Baselines + PROGRESS scaffold

**Files:**
- Create: `docs/envoy-go/phases/43-connection-pooling/PROGRESS.md`

- [ ] **Step 1: Capture the pre-IMPL six-gate baseline.** Run, record in PROGRESS.md:
  - `go build ./...`
  - `go vet ./...`
  - `gofmt -l internal/ test/` (expect empty)
  - `go test ./internal/... 2>&1 | tail -20`
  - `go test ./test/differential/ -count=1 2>&1 | tail -20` (the full **79**-dir suite — the byte-stability anchor; note the `reference_differential_fullsuite_startup_flake` `subject ready: EOF` possibility — isolate-re-run to distinguish from a regression)
  - Stat surface: tracked as a documented running total. Record **1181** (SPEC §14 baseline). The 43.1 exit total is verified ARITHMETICALLY (1181 + 2 = 1183) against the Task 4 registration test.
- [ ] **Step 2: Record baselines + the 12-task checklist** in PROGRESS.md (counts: stat 1181 / fixtures 79 / fuzzers 42 / BackendKind tail 36 / DECISIONS tail ADR-0251, next-free ADR-0252; the anticipated exit deltas from SPEC §14 + D-S431-3's 1183 pin).
- [ ] **Step 3: Commit.**
```bash
git add docs/envoy-go/phases/43-connection-pooling/PROGRESS.md
git commit -m "phase 43.1 Task 1: PROGRESS scaffold + pre-IMPL baselines"
```

---

## Task 2: Parse `max_connections`/`max_pending_requests` + build the `connPool`

**Files:**
- Modify: `internal/cluster/circuitbreaker.go` — the `pool *connPool` field + the parse.
- Create: `internal/cluster/connpool.go` — the `connPool` struct (fields only this task; methods land in Task 3).
- Test: `internal/cluster/circuitbreaker_test.go`

Add to `circuitBreaker` (after the existing fields):
```go
	// pool is the DEFAULT-priority connection-creation budget + bounded pending
	// wait-queue (ADR-0252). Non-nil for every cluster WITH circuit_breakers
	// (the budgets default to 1024 — AMEND-CP5, so it is effectively off unless
	// configured small). DEFAULT-only: every request is DEFAULT at 43.1.
	pool *connPool
```

The `connPool` struct in `connpool.go` (fields this task; the SPEC §3.1 budgets/counters live here, NOT on `cbPriority` — the D-S431-2 placement decision):
```go
package cluster

import (
	"context"
	"errors"
	"sync"

	"github.com/esalaine/envoy-go/internal/stats"
)

// connPool is the per-cluster DEFAULT-priority connection-creation budget +
// bounded pending wait-queue (ADR-0252). All state transitions happen under mu
// (single-mutator discipline; the 42.2b -race precedent); the only out-of-lock
// operation is the buffered cap-1 wake send, which never blocks. Built for every
// cluster WITH circuit_breakers; effectively off at the 1024 defaults.
type connPool struct {
	maxConnections     int64
	maxPendingRequests int64

	mu          sync.Mutex
	activeConns int64           // live + reserved upstream connections (guarded by mu)
	waiters     []chan struct{} // FIFO; each buffered cap 1; a send hands off a reserved permit

	// stat handles (injected by registerStats in Task 4; nil in primitive unit tests).
	cxOpen                    *stats.Gauge   // 1 while activeConns >= maxConnections
	rqPendingOpen             *stats.Gauge   // 1 while len(waiters) >= maxPendingRequests
	upstreamCxOverflow        *stats.Counter // ++ on each cap-reached connection creation
	upstreamRqPendingActive   *stats.Gauge   // current pending-queue depth
	upstreamRqPendingTotal    *stats.Counter // cumulative requests that entered the queue
	upstreamRqPendingOverflow *stats.Counter // ++ on queue-full (shared name w/ max_requests — AMEND-CP2)
}

// errConnPoolOverflow is returned by acquireConnOrPend when the pending wait-
// queue is full (max_pending_requests reached). Distinct from a real dial error
// so the router routes it to a fail-fast 503 (not 502) WITHOUT double-counting
// the upstream status class. (ADR-0252)
var errConnPoolOverflow = errors.New("cluster: connection pool: max_pending_requests overflow")

// IsConnPoolOverflow reports whether err is the connection-pool wait-queue
// overflow sentinel (checked via errors.Is across the DialH2 "%w" wrap).
// Exported additive predicate — no existing signature changes. (ADR-0252)
func IsConnPoolOverflow(err error) bool { return errors.Is(err, errConnPoolOverflow) }
```

In `parseCircuitBreakers` (read the DEFAULT threshold's budgets; default 1024; build the pool before `return`):
```go
	out := &circuitBreaker{}
	out.prio[0].maxRequests = 1024
	out.prio[1].maxRequests = 1024
	maxConns := int64(1024)   // AMEND-CP5 default
	maxPending := int64(1024) // AMEND-CP5 default
	seen := [2]bool{}
	for _, th := range cb.GetThresholds() {
		// ... existing priority / duplicate / retry_budget / max_requests parse ...
		if idx == 0 { // DEFAULT-only: the connection budget binds DEFAULT (43.1)
			if v := th.GetMaxConnections(); v != nil {
				maxConns = int64(v.GetValue())
			}
			if v := th.GetMaxPendingRequests(); v != nil {
				maxPending = int64(v.GetValue())
			}
		}
	}
	out.pool = &connPool{maxConnections: maxConns, maxPendingRequests: maxPending}
	return out, nil
```

- [ ] **Step 1: Write failing tests** in `circuitbreaker_test.go`: (a) a DEFAULT threshold with `max_connections: 2, max_pending_requests: 3` ⇒ `cb.pool.maxConnections == 2 && cb.pool.maxPendingRequests == 3`; (b) absent budgets ⇒ both default 1024; (c) `max_connections: 0` ⇒ `cb.pool.maxConnections == 0`; (d) a cluster WITH `circuit_breakers` ⇒ `cb.pool != nil`; (e) HIGH-priority budgets are IGNORED by the pool (a HIGH threshold with `max_connections: 7` + a DEFAULT with `max_connections: 2` ⇒ `pool.maxConnections == 2`). Build the `*clusterv3.Cluster` inputs in-code (the `outlier_test.go` precedent; `MaxConnections: wrapperspb.UInt32(2)`).
- [ ] **Step 2: Run → FAIL** (`pool` field / `connPool` undefined).
- [ ] **Step 3: Implement** the field + the struct + the parse.
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** gofmt + `golangci-lint run ./internal/cluster/...` (per `feedback_pertask_gofmt_lint`).
- [ ] **Step 6: Commit** (`phase 43.1 Task 2: parse max_connections/max_pending_requests + build connPool`).

---

## Task 3: The `connPool` wait-queue primitive (acquire / pend / release / ctx-cancel)  ★ LOAD-BEARING

**Files:**
- Modify: `internal/cluster/connpool.go` — the methods.
- Test: `internal/cluster/connpool_test.go` (CREATE)

The methods (D-S431-1; the stat transitions are driven here, nil-guarded so the primitive tests can run without a registry, or inject test handles):
```go
// acquireConnOrPend reserves a connection-creation permit for one new upstream
// connection. Returns:
//   - nil                 → a permit is held; the caller MUST dial and later pair
//                           it with exactly one releaseConn (via the connWithGauge
//                           dec closure, or directly on a post-acquire failure).
//   - errConnPoolOverflow → the wait-queue is full; the caller fails fast (503).
//   - ctx.Err()           → ctx cancelled/expired while pending; no permit held.
// (ADR-0252, D-S431-1)
func (p *connPool) acquireConnOrPend(ctx context.Context) error {
	p.mu.Lock()
	if p.activeConns < p.maxConnections {
		p.activeConns++
		if p.activeConns >= p.maxConnections {
			setGauge(p.cxOpen, 1)
		}
		p.mu.Unlock()
		return nil
	}
	// Cap reached: the soft-signal parity (AMEND-CP1) — cx_open + upstream_cx_overflow.
	setGauge(p.cxOpen, 1)
	incCounter(p.upstreamCxOverflow)
	if int64(len(p.waiters)) >= p.maxPendingRequests {
		p.mu.Unlock()
		incCounter(p.upstreamRqPendingOverflow)
		return errConnPoolOverflow
	}
	ch := make(chan struct{}, 1)
	p.waiters = append(p.waiters, ch)
	incGauge(p.upstreamRqPendingActive)
	incCounter(p.upstreamRqPendingTotal)
	if int64(len(p.waiters)) >= p.maxPendingRequests {
		setGauge(p.rqPendingOpen, 1)
	}
	p.mu.Unlock()

	select {
	case <-ch:
		// A reserved permit was handed off (activeConns already accounts for us).
		return nil
	case <-ctx.Done():
		p.mu.Lock()
		if p.removeWaiterLocked(ch) {
			// Still queued → cancelled cleanly; no permit was handed off.
			decGauge(p.upstreamRqPendingActive)
			p.clearRqPendingOpenLocked()
			p.mu.Unlock()
			return ctx.Err()
		}
		p.mu.Unlock()
		// Already dequeued → a permit is en route on the buffered channel. Drain
		// it and give it back so it is not leaked (may wake the next waiter).
		<-ch
		p.releaseConn()
		return ctx.Err()
	}
}

// releaseConn returns one connection-creation permit. With a queued waiter the
// permit is handed off directly (FIFO) — activeConns is UNCHANGED (the freed
// permit is re-reserved for the waiter; cx_open stays set). With no waiter,
// activeConns-- and cx_open clears when back under the cap. Called from the
// connWithGauge dec closure on conn Close + the ctx-cancel give-back. (ADR-0252)
func (p *connPool) releaseConn() {
	p.mu.Lock()
	if len(p.waiters) > 0 {
		ch := p.waiters[0]
		p.waiters = p.waiters[1:]
		decGauge(p.upstreamRqPendingActive)
		p.clearRqPendingOpenLocked()
		p.mu.Unlock()
		ch <- struct{}{} // buffered cap 1 → never blocks
		return
	}
	p.activeConns--
	if p.activeConns < p.maxConnections {
		setGauge(p.cxOpen, 0)
	}
	p.mu.Unlock()
}

// removeWaiterLocked drops ch from the FIFO if present; reports whether found.
// Caller holds mu. (ADR-0252)
func (p *connPool) removeWaiterLocked(ch chan struct{}) bool {
	for i, w := range p.waiters {
		if w == ch {
			p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
			return true
		}
	}
	return false
}

// clearRqPendingOpenLocked clears rq_pending_open when the queue is back under
// the cap. Caller holds mu. (ADR-0252)
func (p *connPool) clearRqPendingOpenLocked() {
	if int64(len(p.waiters)) < p.maxPendingRequests {
		setGauge(p.rqPendingOpen, 0)
	}
}

// hasWaiter reports whether a request is currently pending. Used by PutIdleH1 to
// choose close-and-wake vs pool. Racy by nature (the truth can change after the
// lock drops) but only a missed-pooling optimization is at stake, never
// correctness. (ADR-0252, D-S431-1)
func (p *connPool) hasWaiter() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.waiters) > 0
}
```

Add the nil-guarded stat helpers at the bottom of `connpool.go` (package-level funcs; reused — confirm `*stats.Gauge` has `Set`/`Inc`/`Dec` and `*stats.Counter` has `Inc`, matching `cluster.go`'s usage):
```go
func setGauge(g *stats.Gauge, v int64) { if g != nil { g.Set(v) } }
func incGauge(g *stats.Gauge)          { if g != nil { g.Inc() } }
func decGauge(g *stats.Gauge)          { if g != nil { g.Dec() } }
func incCounter(c *stats.Counter)      { if c != nil { c.Inc() } }
```

- [ ] **Step 1: Write failing tests** in `connpool_test.go` (inject real `*stats.Gauge`/`*stats.Counter` via `stats.NewRegistry()` so the transitions are asserted):
  - (a) **acquire under cap:** `maxConnections:2` — two `acquireConnOrPend(ctx)` return nil; `activeConns==2`; `cxOpen==1` after the 2nd.
  - (b) **release clears cx_open:** after a `releaseConn`, `activeConns==1`, `cxOpen==0`; the next acquire returns nil + re-sets `cxOpen==1`.
  - (c) **pend + wake (the core):** `maxConnections:1, maxPendingRequests:2` — acquire #1 (nil). Launch #2 in a goroutine (pends: poll `upstreamRqPendingActive==1` + `rqPendingOpen==1` since 1>=... use `maxPendingRequests:1` so 1>=1 ⇒ open). `releaseConn()` → #2's goroutine returns nil (woken) + `pendingActive==0`. Assert `upstreamRqPendingTotal==1`.
  - (d) **queue-full overflow:** `maxConnections:1, maxPendingRequests:1` — acquire #1 (nil); launch #2 (pends, fills the queue); acquire #3 returns `errConnPoolOverflow` + `upstreamRqPendingOverflow==1` (immediate, no block). Release twice → #2 wakes.
  - (e) **ctx-cancel while pending:** `maxConnections:1, maxPendingRequests:5` — acquire #1; launch #2 with a cancellable ctx (pends); cancel the ctx → #2 returns `ctx.Err()`; `pendingActive==0`, `rqPendingOpen==0`, and a subsequent `releaseConn` decrements `activeConns` to 0 (NOT handed to the cancelled waiter — proving the clean removal). THEN a variant proving the drain-and-give-back: race a `releaseConn` against the cancel (loop the ctx-cancel-vs-release ordering) and assert no permit leaks (`activeConns` returns to 0 after all release; no goroutine stuck).
  - (f) **max_connections:0 (D-S431-6):** `maxConnections:0, maxPendingRequests:0` — the FIRST acquire returns `errConnPoolOverflow` immediately (`activeConns(0) < 0` false ⇒ cap branch ⇒ queue 0>=0 full ⇒ overflow). And `maxConnections:0, maxPendingRequests:1` — the first acquire pends; cancel ⇒ `ctx.Err()`.
  - (g) **`-race` concurrency:** `maxConnections:4, maxPendingRequests:1000` — 200 goroutines each `acquireConnOrPend` then (on nil) `releaseConn` after a tiny op; a peak-tracker on `activeConns` (read under a test mutex, or assert via the gauge) NEVER exceeds 4; all 200 complete; `activeConns==0` at the end; no `-race` report.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the methods + helpers.
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -race -run 'ConnPool' -count=1` (the `-count=1` defeats result caching across the iterative break/fix loop — `reference_differential_break_protocol_count1`).
- [ ] **Step 5:** gofmt + `golangci-lint run ./internal/cluster/...`.
- [ ] **Step 6: Commit** (`phase 43.1 Task 3: connPool wait-queue primitive (acquire/pend/release/ctx-cancel) -race-clean`).

---

## Task 4: Activate 3 + add 2 stat registrations (1181 → 1183)

**Files:**
- Modify: `internal/cluster/circuitbreaker.go` (`registerStats`).
- Test: `internal/cluster/circuitbreaker_test.go` (extend `circuitBreakerStatNames()` + the present/absent tests).

Modify `registerStats` to STORE the activated handles into `cb.pool` (DEFAULT only) + ADD the 2 new names:
```go
func (cb *circuitBreaker) registerStats(r *stats.Registry, prefix string) {
	for idx, name := range []string{"default", "high"} {
		gp := prefix + "circuit_breakers." + name + "."
		cb.prio[idx].rqOpen = r.NewGauge(gp + "rq_open")
		cxOpen := r.NewGauge(gp + "cx_open")        // ACTIVATED (was emit-0)
		r.NewGauge(gp + "cx_pool_open")             // emit-0 (max_connection_pools deferred)
		rqPendingOpen := r.NewGauge(gp + "rq_pending_open") // ACTIVATED (was emit-0)
		g := r.NewGauge(gp + "rq_retry_open")
		if idx == 0 { // DEFAULT-only live handles
			cb.pool.cxOpen = cxOpen
			cb.pool.rqPendingOpen = rqPendingOpen
			cb.prio[0].rqRetryOpen = g
		}
	}
	cb.upstreamRqPendingOverflow = r.NewCounter(prefix + "upstream_rq_pending_overflow") // LIVE
	cb.pool.upstreamRqPendingOverflow = cb.upstreamRqPendingOverflow                     // shared (AMEND-CP2)
	cb.pool.upstreamCxOverflow = r.NewCounter(prefix + "upstream_cx_overflow")           // ACTIVATED (was emit-0)
	r.NewCounter(prefix + "upstream_cx_pool_overflow")                                   // emit-0
	cb.upstreamRqRetryOverflow = r.NewCounter(prefix + "upstream_rq_retry_overflow")     // LIVE (phase 42)
	// AMEND-CP3: 2 NEW CB-scoped pending-queue names (the +2 surface delta).
	cb.pool.upstreamRqPendingActive = r.NewGauge(prefix + "upstream_rq_pending_active")
	cb.pool.upstreamRqPendingTotal = r.NewCounter(prefix + "upstream_rq_pending_total")
}
```
(`registerStats` runs only on `circuit_breakers` clusters, where `cb.pool != nil` — Task 2 guarantees it. The scoped call site in `manager.go` `registerClusterMetrics` ~:184 is UNCHANGED.)

- [ ] **Step 1: Extend** `circuitBreakerStatNames()` (the test helper) to append `p+"upstream_rq_pending_active"` + `p+"upstream_rq_pending_total"` (14 → 16 names). Update `TestRegisterCircuitBreakerStats_Present` to also assert `cl.circuitBreaker.pool.cxOpen != nil`, `pool.rqPendingOpen != nil`, `pool.upstreamCxOverflow != nil`, `pool.upstreamRqPendingActive != nil`, `pool.upstreamRqPendingTotal != nil` (the activated + new handles injected). Run → FAIL.
- [ ] **Step 2: Implement** the `registerStats` changes.
- [ ] **Step 3: Run → PASS** (`TestRegisterCircuitBreakerStats_Present`/`_Absent` both green) + `go test ./internal/cluster/ -count=1`.
- [ ] **Step 4: Stat-surface count → expect 1183** (1181 + 2). Record in PROGRESS (the arithmetic verification against the 16-name helper).
- [ ] **Step 5:** gofmt + `golangci-lint run ./internal/cluster/...`.
- [ ] **Step 6: Commit** (`phase 43.1 Task 4: activate cx_open/rq_pending_open/upstream_cx_overflow + add rq_pending_{active,total} (1181→1183)`).

---

## Task 5: The permit overlay at `Dial` + `AcquireH1` MISS + `connDec` + byte-stability

**Files:**
- Modify: `internal/cluster/cluster.go` — the 3 helpers + the `Dial`/`AcquireH1` overlays.

The helpers (add near `TryAcquireRequest`, ~:216):
```go
// acquireConnSlot reserves a connection-creation permit (clusters WITH a connPool).
// Returns nil (no pool, or permit held — caller MUST pair with releaseConnSlot /
// connDec), errConnPoolOverflow (queue full → fail fast 503), or ctx.Err(). (ADR-0252)
func (c *Cluster) acquireConnSlot(ctx context.Context) error {
	if c.circuitBreaker == nil || c.circuitBreaker.pool == nil {
		return nil
	}
	return c.circuitBreaker.pool.acquireConnOrPend(ctx)
}

// releaseConnSlot returns a permit acquired by acquireConnSlot. Called on the
// post-acquire dial/handshake FAILURE paths (the success path composes the
// release into connDec). No-op when no pool. (ADR-0252)
func (c *Cluster) releaseConnSlot() {
	if c.circuitBreaker != nil && c.circuitBreaker.pool != nil {
		c.circuitBreaker.pool.releaseConn()
	}
}

// connDec composes upstream_cx_active.Dec() + the LB release() + (for pooled
// clusters) pool.releaseConn() into the single dec closure connWithGauge runs
// exactly once on Close. The pool release on Close is the conn-Close wake seam
// (hands a permit to the head waiter). (ADR-0252)
func (c *Cluster) connDec(release func()) func() {
	if c.circuitBreaker == nil || c.circuitBreaker.pool == nil {
		return func() { c.upstreamCxActive.Dec(); release() }
	}
	pool := c.circuitBreaker.pool
	return func() { c.upstreamCxActive.Dec(); release(); pool.releaseConn() }
}
```

In `Dial` (after the successful `c.lb.Pick`, BEFORE the dial; release the permit on every post-acquire failure; compose `connDec` on success):
```go
	ep, release, err := c.lb.Pick(hk, ok, match, hasMatch)
	if err != nil {
		return nil, Endpoint{}, err
	}
	// ADR-0252: gate connection CREATION on the max_connections permit (pends in
	// the bounded wait-queue at the cap; errConnPoolOverflow when the queue is full).
	if err := c.acquireConnSlot(ctx); err != nil {
		release()
		return nil, ep, err
	}
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", ep.Addr())
	if err != nil {
		c.releaseConnSlot()
		release()
		return nil, ep, fmt.Errorf("cluster: dial: %w", err)
	}
	var final net.Conn = raw
	if c.upstreamCfg != nil {
		conn := stdtls.Client(raw, c.upstreamCfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			c.releaseConnSlot()
			release()
			return nil, ep, fmt.Errorf("cluster: tls: handshake: %w", err)
		}
		final = conn
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	return &connWithGauge{Conn: final, dec: c.connDec(release)}, ep, nil
```

In `AcquireH1`, the MISS slow path (after `c.h1PoolMu.Unlock()` at ~:490; the HIT path at ~:487 returns BEFORE this ⇒ no permit — byte-neutral keep-alive):
```go
	c.h1PoolMu.Unlock()

	// ADR-0252: pool MISS → gate connection creation on the max_connections permit.
	if err := c.acquireConnSlot(ctx); err != nil {
		release()
		return nil, ep, err
	}
	d := &net.Dialer{Timeout: c.connectTimeout}
	raw, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		c.releaseConnSlot()
		release()
		return nil, ep, fmt.Errorf("cluster: dial: %w", err)
	}
	var final net.Conn = raw
	if c.upstreamCfg != nil {
		conn := stdtls.Client(raw, c.upstreamCfg)
		if err := conn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			c.releaseConnSlot()
			release()
			return nil, ep, fmt.Errorf("cluster: tls: handshake: %w", err)
		}
		final = conn
	}
	c.upstreamCxTotal.Inc()
	c.upstreamCxActive.Inc()
	wrapped := &connWithGauge{Conn: final, dec: c.connDec(release)}
	return &PooledH1Conn{Conn: wrapped, Br: bufio.NewReaderSize(wrapped, 4096), ep: ep}, ep, nil
```

- [ ] **Step 1: Write a failing test** (`cluster_test.go` or `connpool_test.go`): a `Cluster` built (via `NewManager`/`mkBootstrap`) with `circuit_breakers{max_connections:1}` + a backend listener that ACCEPTS-and-holds; `AcquireH1` #1 succeeds; `AcquireH1` #2 (in a goroutine with a short-deadline ctx) returns a ctx-deadline error (it pends then times out — proving the permit gate engaged at the MISS). A cluster WITHOUT `circuit_breakers` never pends (both `AcquireH1` succeed). [If a backend-holding harness is heavy here, defer the full pend-proof to Task 6's integration test and make Task 5's test the byte-stability + nil-guard assertion only — state the choice in PROGRESS.]
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the 3 helpers + the `Dial`/`AcquireH1` overlays.
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -race -count=1`.
- [ ] **Step 5: BYTE-STABILITY GATE** — `go test ./test/differential/ -count=1` → all **79** still GREEN. `0074` now BUILDS a `connPool` (default 1024) + registers the 2 new zero stats + live-drives cx_open (stays 0 at 1024) — the differential must stay green (the reference emits the 2 names too). This is the load-bearing regression gate; an `0074` mismatch here means the new zero-stats or the permit path changed observable behavior — investigate before proceeding.
- [ ] **Step 6:** gofmt + `golangci-lint run ./internal/cluster/...`.
- [ ] **Step 7: Commit** (`phase 43.1 Task 5: permit overlay at Dial + AcquireH1 MISS + connDec compose`).

---

## Task 6: The idle-return wake (`PutIdleH1` close-and-wake) + the pend/wake integration test

**Files:**
- Modify: `internal/cluster/cluster.go` (`PutIdleH1`, ~:528).
- Test: `internal/cluster/connpool_test.go`

In `PutIdleH1` (BEFORE the existing pool-push; the racy `hasWaiter` peek is benign per D-S431-1):
```go
func (c *Cluster) PutIdleH1(p *PooledH1Conn) {
	if p == nil || p.Conn == nil {
		return
	}
	// ADR-0252: when a request is pending on the connection budget, CLOSE this
	// idle conn instead of pooling it — its connWithGauge.Close runs connDec →
	// pool.releaseConn, freeing a permit + waking the head waiter (which dials
	// fresh). Routes the idle-return wake through the single permit mechanism
	// (no direct conn handoff). Racy peek; at worst a missed pooling, never wrong.
	if c.circuitBreaker != nil && c.circuitBreaker.pool != nil && c.circuitBreaker.pool.hasWaiter() {
		_ = p.Conn.Close()
		return
	}
	addr := p.ep.Addr()
	// ... existing pool-push logic unchanged ...
}
```

- [ ] **Step 1: Write a failing integration test** (`connpool_test.go`): build a `Cluster` (via `NewManager`/`mkBootstrap`) with `circuit_breakers{max_connections:2, max_pending_requests:4}` + an in-process backend listener that accepts and holds connections (a tiny `net.Listener` + a per-conn block on a release channel, mirroring `acceptBlockingHold`'s shape but in-package). Drive:
  - `AcquireH1` ×2 (fill the 2 permits; the conns block at the backend).
  - Launch `AcquireH1` #3 + #4 in goroutines (they PEND — poll `cb.pool.upstreamRqPendingActive`/the gauge to `== 2`).
  - Path A (conn-Close wake): `(*PooledH1Conn).Conn.Close()` one held conn → #3 unblocks (its `AcquireH1` returns) — `pendingActive == 1`.
  - Path B (idle-return wake): `PutIdleH1` the OTHER held conn while #4 is pending → #4 unblocks (the conn was closed-and-woke, not pooled) — `pendingActive == 0`.
  - Assert: `cb.pool.upstreamRqPendingTotal == 2`; the cap was never exceeded (a peak tracker on successful concurrent `AcquireH1`s ≤ 2 at any instant); final `activeConns` consistent; no `-race` report; no goroutine leak (all 4 `AcquireH1` calls return).
- [ ] **Step 2: Run → FAIL** (idle-return does not wake yet — #4 stalls).
- [ ] **Step 3: Implement** the `PutIdleH1` close-and-wake.
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -race -run 'ConnPool' -count=1`.
- [ ] **Step 5: BYTE-STABILITY GATE** — `go test ./test/differential/ -count=1` → all **79** GREEN (the `hasWaiter` peek is no-op for non-CB clusters; `0074` never pends so `PutIdleH1` pools as before).
- [ ] **Step 6:** gofmt + `golangci-lint run ./internal/cluster/...`.
- [ ] **Step 7: Commit** (`phase 43.1 Task 6: PutIdleH1 idle-return wake + pend/wake integration test`).

---

## Task 7: The overflow → 503 routing (router H1 + H2)

**Files:**
- Modify: `internal/filter/http/router/router.go` — the `AcquireH1` failure site (~:642) + the legacy `Dial` site (~:813).
- Modify: `internal/filter/http/router/router_h2.go` — the `DialH2` failure site (~:98) + the legacy `doH2` `DialH2` site (~:306).

At the H1 `AcquireH1` failure site (`doH1ClusterAction`, ~:642 — add the overflow branch FIRST):
```go
	pooled, ep, err := a.cluster.AcquireH1(ctx)
	if err != nil {
		if cluster.IsConnPoolOverflow(err) {
			// ADR-0252: connection-pool wait-queue full → fail fast 503.
			// upstream_rq_pending_overflow already incremented in the pool. Mirror
			// the phase-41 max_requests overflow: NO IncStatusClass on the upstream
			// class (the dedicated overflow counter is the signal; avoids a
			// speculative upstream_rq_5xx cross-side mismatch). NOT localOrigin
			// (an overflow is a load-shed, not a connect failure for retry_on).
			return ActionResponse{Status: 503, Headers: localReplyHeaders(0), Body: nil}, picked, nil
		}
		a.cluster.IncStatusClass(503)
		if !ep.IsZero() {
			a.cluster.RecordUpstreamResult(ep, cluster.UpstreamResult{StatusCode: 503, LocalOriginErr: true})
		}
		return ActionResponse{Status: 503, Headers: localReplyHeaders(0), Body: nil, localOrigin: true}, picked, nil
	}
```

At the H2 `DialH2` failure site (`doH2ClusterAction`, ~:98 — the overflow propagates through the `"cluster: dial h2: %w"` wrap, so `errors.Is` works):
```go
	cc, ep, err := a.cluster.DialH2(ctx)
	if err != nil {
		if cluster.IsConnPoolOverflow(err) {
			// ADR-0252: connection-pool overflow → 503 (NOT the 502 dial-failure shape).
			return ActionResponse{Status: 503, Headers: h2LocalReplyHeaders(), Body: nil}, picked, nil
		}
		a.cluster.IncStatusClass(502)
		// ... existing 502 dial-failure path unchanged ...
	}
```

The two legacy sites (`router.go` `do` ~:813 `a.cluster.Dial`; `router_h2.go` `doH2` ~:306 `r.cluster.DialH2`): add the same `if cluster.IsConnPoolOverflow(err)` branch → 503 WITHOUT `IncStatusClass`. (These legacy direct-write paths are not the differential's path — HCM dispatch routes through the `*ClusterAction` pair — but keep them consistent. Confirm they are still reachable via a quick caller grep; if dead, note it and skip.)

- [ ] **Step 1: Write a failing test** (router package, the existing `do{H1,H2}ClusterAction` test pattern): a router action over a cluster with `circuit_breakers{max_connections:1, max_pending_requests:0}` + a backend that ACCEPTS-and-holds; the 1st concurrent request holds the single permit; the 2nd finds the queue full (`max_pending_requests:0`) ⇒ `AcquireH1`/`DialH2` returns `errConnPoolOverflow` ⇒ the action returns `Status: 503` (NOT 502, and NOT a `localOrigin` connect-failure). If the router package lacks a cluster-backed harness, assert the routing via a unit test that injects `errConnPoolOverflow` through a seam OR rely on `0078` (Task 8) for the end-to-end proof — state the choice in PROGRESS.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the 4 router edits (the 2 `*ClusterAction` sites are load-bearing; the 2 legacy sites for consistency).
- [ ] **Step 4: Run → PASS** + `go test ./internal/... -count=1` + the byte-stability gate `go test ./test/differential/ -count=1` (still **79** GREEN — no cluster configures a small `max_connections` yet).
- [ ] **Step 5:** gofmt + `golangci-lint run ./internal/filter/http/router/...`.
- [ ] **Step 6: Commit** (`phase 43.1 Task 7: overflow → 503 routing (H1 AcquireH1 + H2 DialH2 + legacy sites)`).

---

## Task 8: The `0078` cross-side fixture + the sticky-release backend control

**Files:**
- Modify: `test/differential/runner_test.go` — add the `/__release_sticky` control path to `acceptBlockingHold` (additive; `/__release` UNCHANGED — `0074` byte-stable).
- Create: `test/fixtures/0078-connection-pool-max-connections/driver/driver.go`
- Create: `test/fixtures/0078-connection-pool-max-connections/driver/driver_test.go` (the `backendIdxFromBody` table test — the per-fixture helper precedent)
- Create: `test/fixtures/0078-connection-pool-max-connections/expectations.yaml`
- Create: `test/fixtures/0078-connection-pool-max-connections/README.md`

The sticky-release add to `acceptBlockingHold` (a sticky flag the request goroutine checks before blocking on the gate):
```go
func acceptBlockingHold(ln net.Listener, idx int) {
	var mu sync.Mutex
	gate := make(chan struct{})
	sticky := false // once /__release_sticky fires, all requests pass immediately
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			// ... read the request ...
			if req.URL.Path == "/__release_sticky" {
				mu.Lock()
				if !sticky {
					sticky = true
					close(gate) // free everyone currently held + mark sticky
				}
				mu.Unlock()
				// ... 200 "released-sticky" ...
				return
			}
			if req.URL.Path == "/__release" {
				// ... existing re-armable release (UNCHANGED) ...
				return
			}
			// a normal request: pass immediately if sticky, else block on the gate.
			mu.Lock()
			s := sticky
			g := gate
			mu.Unlock()
			if !s {
				<-g
			}
			// ... 200 "backend-<idx>:<seg>" ...
		}(c)
	}
}
```
(Confirm the exact existing `acceptBlockingHold` body at `runner_test.go` and weave the sticky branch in minimally; `0074`'s `/__release` path stays byte-for-byte.)

The driver: model on `test/fixtures/0074-circuit-breaker-max-requests/driver/driver.go` for the cross-side shape (reference STRICT_DNS / `host.docker.internal`; subject STATIC / `127.0.0.1`) + the `scrapeStats`/poll helpers + the `StatsAsserter` interface (cross-side — `reference_differential_asserter_dispatch`, NOT `SubjectAsserter`). Topology: cluster `c_cp`, lb ROUND_ROBIN, **1** `BlockingHoldResponder` endpoint, `circuit_breakers: { thresholds: [ { priority: DEFAULT, max_connections: N, max_pending_requests: M } ] }`, on BOTH sides. Constants single-sourced (D-S431-5). The driver caches `backendPorts` (passed to `ReferenceBootstrap`/`SubjectConfig`) so `AssertStats` can hit `127.0.0.1:<backendPort>/__release_sticky` on loopback for BOTH sides' phases.

`AssertStats(t, refAdminAddr, subjAdminAddr)` runs the staged drive (D-S431-5 table) per side SEQUENTIALLY (subject first — the exact prong; then reference — the robust prong); the shared backend is idle between sides. Assertions:
- **SUBJECT exact:** at the step-2 barrier `upstream_cx_active == N` AND `upstream_rq_pending_active == M`; the J oversubscribers all `503`; after drain `upstream_rq_pending_total == M` EXACTLY; final `cx_open == 0` + `rq_pending_open == 0`.
- **Cross-side robust (both):** `cx_open == 1` at the saturation barrier; ≥1 oversubscriber `503` (observed from the fired requests' status codes — the downstream class per `reference_concurrent_attempt_downstream_class_assertion`); `upstream_rq_pending_overflow` delta ≥ 1; reference `upstream_cx_total > 0`; final `cx_open == 0` + `rq_pending_open == 0` after `/__release_sticky`.

- [ ] **Step 1:** Add the `/__release_sticky` path to `acceptBlockingHold` → `go test ./test/differential/ -run 'TestDifferential/0074' -count=1` → `0074` still GREEN (the sticky path is unused by `0074`; `/__release` unchanged).
- [ ] **Step 2:** Write `driver_test.go` (the `backendIdxFromBody` table test) → run → FAIL (helper undefined).
- [ ] **Step 3:** Write `driver.go` (`BackendCount`, `ReferenceBootstrap`, `SubjectConfig`, `ReferenceListenerPort`, `DriveReference`/`DriveSubject`, `ProbeAdmin`, `AssertStats` with the staged drive) + `expectations.yaml` + `README.md`. Single-source the constants (`refContainerListenerPort = 19167`).
- [ ] **Step 4:** `go test ./test/fixtures/0078-connection-pool-max-connections/driver/ -count=1` (the unit test) → PASS.
- [ ] **Step 5: Run the cross-side fixture** (requires Docker + `contrib-v1.37.2`): `go test ./test/differential/ -run 'TestDifferential/0078' -count=1 -v` (`reference_differential_run_selector` — the `TestDifferential/` prefix is REQUIRED, else ZERO subtests match → vacuous green). Expected: PASS — subject hard-caps at N with the queue at M and J overflows; both sides flip `cx_open`/`rq_pending_open` and produce ≥1 503 + an overflow delta; the sticky release drains both sides to the gauges-at-0 parity.
- [ ] **Step 6:** gofmt + `golangci-lint run ./test/...`. Record fixtures **79 → 80**, BackendKind tail **36 (UNCHANGED — REUSE)**.
- [ ] **Step 7: Commit** (`phase 43.1 Task 8: 0078 connection-pool-max-connections cross-side fixture + sticky-release backend control`).

---

## Task 9: `0078` deliberate breaks + 20-run flake + `-race`

**Files:** none committed (verification only; SPEC §8.1 break protocol).

★ Use `-count=1` for EVERY break run (`reference_differential_break_protocol_count1`) + the `TestDifferential/0078` selector (`reference_differential_run_selector`).

- [ ] **Step 1: Break (A) — `max_connections` NOT hard-capped.** Temporarily make `acquireConnOrPend` always return `nil` (permit always granted, never pends). Run `go test ./test/differential/ -run 'TestDifferential/0078' -count=1` → MUST FAIL (the SUBJECT `upstream_cx_active` exceeds N / no pend ⇒ the subject-exact prong fails). Restore.
- [ ] **Step 2: Break (B) — the wait-queue NOT bounded (no queue-full 503).** Temporarily make the `len(p.waiters) >= p.maxPendingRequests` overflow branch never fire (always pend). Run → MUST FAIL (no oversubscriber gets a 503 ⇒ the cross-side overflow-delta / downstream-503 assert fails; subject may also hang at the J-overflow step — bound it with the convergeDeadline). Restore.
- [ ] **Step 3: Confirm both breaks restored** (`git diff` clean; `go test ./test/differential/ -run 'TestDifferential/0078' -count=1` → PASS). Also run the connPool unit `-race` once more: `go test ./internal/cluster/ -race -run 'ConnPool' -count=1`.
- [ ] **Step 4: 20-run flake gate:** `for i in $(seq 20); do go test ./test/differential/ -run 'TestDifferential/0078' -count=1 || echo "FAIL $i"; done` → 20/20 PASS (the release-barrier + poll-to-converge + sticky-drain make it deterministic; if any flake, widen `convergeDeadline`, NEVER add a fixed sleep — `reference_concurrency_differential_release_barrier`, `reference_differential_band_sigma_margin`).
- [ ] **Step 5:** Record the break + flake results in PROGRESS. (No commit.)

---

## Task 10: Full 80-dir differential + six-gate

**Files:** none (verification); update PROGRESS.

- [ ] **Step 1: Six-gate** (ADR-0052): `go build ./...`; `go vet ./...`; `gofmt -l internal/ test/` (empty); `golangci-lint run ./...`; `go test ./internal/... -count=1`; `go test ./test/differential/ -count=1` (ALL **80** GREEN). The full suite can transiently hit the unrelated `subject ready: EOF` startup flake (`reference_differential_fullsuite_startup_flake`) — isolate-re-run the named dir + a full re-run to distinguish it from a regression.
- [ ] **Step 2: Counts → stat surface 1183; fixtures 80; fuzzers 42 (unchanged); BackendKind tail 36 (unchanged).** Record in PROGRESS.
- [ ] **Step 3:** If any gate fails, fix + re-run before proceeding.

---

## Task 11: ADR-0252 body + BEHAVIOR_CONTRACT delta

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` — the ADR-0252 full entry (§Decision + §Consequences; the §Context is drafted in SPEC §13 — promote/refine it). DECISIONS tail ADR-0251 → **ADR-0252** (next-free ADR-0253).
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the new `### Cluster — connection-pool budgets (max_connections / max_pending_requests)` subsection (SPEC §9). Advance the stat-surface block **1181 → 1183**.

- [ ] **Step 1:** Write the ADR-0252 body. §Decision: the DEFAULT-priority `connPool` (sibling `connpool.go`) extending the phase-41 `circuitBreaker`; the `sync.Mutex` + FIFO buffered-channel wake (D-S431-1); the `max_connections` HARD cap at the connection-creation boundary (`Dial` + `AcquireH1` MISS; pool-HIT no permit — byte-neutral keep-alive); the bounded pending wait-queue (block the goroutine; the conn-Close + idle-return-close wakes; FIFO); the queue-full fail-fast 503 + `upstream_rq_pending_overflow` (the shared counter — AMEND-CP2) + the `cx_open`/`rq_pending_open` gauges + `upstream_cx_overflow`; the 2 new `upstream_rq_pending_active`/`_total` (1181→1183). §Consequences: byte-stable when no `circuit_breakers`; the RECORDED DEPARTURE (AMEND-CP1 — the reference's soft `max_connections`; envoy-go's clean hard cap; exact counts subject-side, robust invariants cross-side); the `UO` flag DEFERRED (D-S431-4 — no plumbing); the idle-return wake's close-instead-of-pool cost (one extra dial under keep-alive contention); the H2-multiplex pool is the 43.2 leg (ADR-0253, supersedes ADR-0056).
- [ ] **Step 2:** Write the BEHAVIOR_CONTRACT subsection (the `max_connections` cap; the bounded wait-queue; the 503/overflow/gauges; the 2 new names; the departure list incl. the UO flag + the soft-vs-hard `max_connections` departure) + the stat-count bump 1181 → 1183.
- [ ] **Step 3:** `go build ./...` (docs-only sanity).
- [ ] **Step 4: Commit** (`phase 43.1 Task 11: ADR-0252 body + BEHAVIOR_CONTRACT connection-pool subsection (stat 1181→1183)`).

---

## Task 12: Completion bundle

**Files:**
- Modify: `docs/envoy-go/phases/43-connection-pooling/PROGRESS.md` (final state + exit-delta table); `docs/envoy-go/phases/43-connection-pooling/README.md` (CREATE — status PLAN-done → IMPL-done); `docs/envoy-go/STATE.md` (active-phase → `phase 43.1 (connection-pooling) IMPL done`; counts → 1183 / 80 / 42 / 36 / ADR-0252); `docs/envoy-go/ROADMAP.md` (row 43 leg 43.1 → `done`; the row STAYS `in-progress` — 43.2 H2-multiplex remains, per `reference_roadmap_split_phase_row_done`: the row flips `done` only once BOTH legs land); `next-prompt.txt` (roll forward to the 43.2 BRAINSTORM — the H2 multiplex pool, supersedes ADR-0056).

- [ ] **Step 1:** Update PROGRESS (the 12-task record + the six-gate evidence + the break/flake results + the exit-delta table).
- [ ] **Step 2:** Write the phase README; update STATE/ROADMAP/next-prompt per the precedent. ROADMAP: leg 43.1 → `done`, row 43 STAYS `in-progress` (43.2 pending); the Upstream-robustness family CLOSES only when BOTH 43.1+43.2 land (ADR-0106 + `reference_roadmap_split_phase_row_done`).
- [ ] **Step 3: Final six-gate re-confirm** + record all exit counts.
- [ ] **Step 4: Commit** (`phase 43.1 Task 12: completion bundle — ROADMAP leg 43.1 done; connection-pool budgets + wait-queue landed`).
- [ ] **Step 5:** The controller squashes the 12 task commits + pushes to origin/master (`feedback_subagents_no_push` — subagents commit LOCALLY only; the controller squashes at stage-close + pushes per `feedback_push_to_origin`).

---

## Exit deltas (SPEC §14)

| Axis | At PLAN | At 43.1 IMPL |
|------|---------|-----------|
| stat surface | 1181 | **1183** (+2: `upstream_rq_pending_active` + `upstream_rq_pending_total`) |
| differential fixtures | 79 | **80** (`0078`) |
| fuzzers | 42 | 42 (unchanged — config-parse, not wire-decode) |
| BackendKind tail | 36 | 36 (unchanged — REUSE `BlockingHoldResponder`; `/__release_sticky` is additive control) |
| DECISIONS tail | ADR-0251 | **ADR-0252** (next-free ADR-0253) |
| new packages / go.mod modules | — | ZERO / ZERO |
| ROADMAP row 43 | in-progress | **in-progress** (leg 43.1 `done`; the row flips `done` only when 43.2 lands) |

Next → the 43.1 IMPL (`superpowers:subagent-driven-development` — fresh implementer per task + two-stage review; insist on `-race -count=1` for Tasks 3/5/6), then the 43.2 (H2 multiplex pool) BRAINSTORM/SPEC/PLAN/IMPL.
