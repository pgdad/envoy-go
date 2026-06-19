# Circuit Breakers (`max_requests` keystone) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land per-priority circuit breaking on the `max_requests` budget — a cluster with `circuit_breakers` fails a request fast with a `503` (over-budget) + increments `upstream_rq_pending_overflow` while `circuit_breakers.default.rq_open` reads 1 — with the full per-priority `circuit_breakers.*` stat surface (+14) registered for parity.

**Architecture:** A per-priority `circuitBreaker` counter struct on `Cluster` (new `internal/cluster/circuitbreaker.go`); a synchronous, non-blocking `TryAcquireRequest()` at router admission (`do{H1,H2}ClusterAction`) + a `defer ReleaseRequest()` on completion; fail-fast `503` + `upstream_rq_pending_overflow`++ + `rq_open`=1 on exhaustion. NO background goroutine; byte-identical when no `circuit_breakers` (a nil-guard). Only `max_requests` is enforced (SPEC AMEND-CB1); `max_connections`/`max_pending_requests` register-for-parity-but-defer.

**Tech Stack:** Go; `cluster.v3.CircuitBreakers` (go-control-plane v1.32.4); the `internal/stats` registry; the `test/differential` cross-side harness (reference `envoyproxy/envoy:contrib-v1.37.2` over a Docker bridge).

This PLAN implements `docs/envoy-go/phases/41-circuit-breakers/SPEC.md` (read it first). Counts at PLAN commit UNCHANGED (stat surface **1149** / fixtures **75** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0247**, next-free **ADR-0248**). All `internal/cluster` paths are in package `cluster`; all anchors below are verified against the worktree.

---

## D-question resolutions (SPEC §12) — settled at PLAN

The implementer MUST follow these (baked into the tasks).

### D-S41-1 — house reject wording (§6)
Mirror the `parseOutlierDetection` precedent (`internal/cluster/outlier.go:52-60`, prefix `cluster: %q: outlier_detection: `). The circuit-breaker reject arms use the prefix `cluster: %q: circuit_breakers: ` + the reason (mirroring the reference PGV wording verbatim where it exists, per ADR-0080):
- `cluster: %q: circuit_breakers: priority: value must be one of the defined enum values` — when a `Thresholds.priority` is not `RoutingPriority_DEFAULT`(0) or `RoutingPriority_HIGH`(1).
- `cluster: %q: circuit_breakers: retry_budget: budget_percent: value must be inside range [0, 100]` — when `retry_budget` is set AND `budget_percent.value > 100` (mirrors `PercentValidationError.Value`; validated even though `retry_budget` enforcement is deferred — the 40.1 deferred-but-validated precedent).
- `cluster: %q: circuit_breakers: duplicate threshold priority` — envoy-go's OWN strict reject (ADR-0080) when two `thresholds[]` entries share a priority (the reference silently accepts; envoy-go rejects rather than guess a merge).

Budget `UInt32Value`s are NOT range-validated (unbounded, any value incl. 0 accepted). `per_host_thresholds` is SILENT-IGNORED (not rejected). All arms unit-level (no boot-reject fixture dir).

### D-S41-2 — `max_requests` absent-vs-0 disposition
`max_requests` is `*wrapperspb.UInt32Value`. **Absent (nil) ⇒ the 1024 default** (AMEND-CB5 — the breaker is effectively off for test traffic). **Explicit 0 ⇒ a hard cap of 0** (rejects ALL requests — `activeRequests(0) >= maxRequests(0)` is always true). Concretely: `if v := th.GetMaxRequests(); v == nil { maxRequests = 1024 } else { maxRequests = int64(v.GetValue()) }`. Unit-tested in Task 3 (the `max_requests: 0` reject-all case).

### D-S41-3 — the `UO` response flag (recorded DEPARTURE at 41)
envoy-go has NO access-log response-flag plumbing: `ActionResponse` (`router.go:119-129`) has only `Status`/`Headers`/`Body`/`Close` (no flags field); the access-log `Record` (`accesslog/accesslog.go:29-40`) has no `ResponseFlags` field; the default formatter HARDCODES `RESPONSE_FLAGS` as `"-"` (`accesslog/format.go:41`). So the reference's `UO` flag CANNOT be emitted without new cross-cutting plumbing → **DEFERRED, a recorded departure** at 41. envoy-go emits the `503` STATUS (the observable contract) + `upstream_rq_pending_overflow` + `rq_open`; it does NOT emit the `UO` access-log flag. The `0074` differential asserts the 503-status + stats pair, NEVER the access-log line (fixtures compare `/stats` + response, not log output). Record the departure in the ADR-0248 §Consequences + BEHAVIOR_CONTRACT (Task 11).

### D-S41-4 — the `BlockingHoldResponder` release mechanism + cross-Docker N-in-flight  ★ load-bearing
The in-process backend listener binds `0.0.0.0:0` (`runner_test.go:196`), so it is reachable BOTH from the subject (`127.0.0.1:port`) AND from the reference container (`host.docker.internal:port`) — the proven `0066` cross-side mechanism (`reference_docker_probe_bridge_network`). A SINGLE shared `BlockingHoldResponder` serves both sides. Coordination:
- **The backend holds every `GET /` request** on a per-batch release channel until the driver hits a CONTROL path `GET /__release` on the same listener (the driver reaches it via `127.0.0.1:backendPort` — same machine). `/__release` closes the currently-held batch's channel (releasing them to respond 200) and re-arms for the next batch. This is re-armable: requests arriving after a release block on a fresh channel.
- **The driver tests each side SEQUENTIALLY** (subject fully, then reference) so the shared backend is idle between sides (no cross-side release interference). Per side: fire **N concurrent** `GET /` (held) → **poll the side's `/stats` until `circuit_breakers.default.rq_open == 1`** (confirms N in-flight; NO sleep) → fire the **(N+1)th** `GET /` (the proxy's breaker rejects it → 503 before it reaches the backend) → assert `503` + `upstream_rq_pending_overflow` delta + `rq_open == 1` → hit `GET /__release` → join the N goroutines (all 200) → poll `rq_open` back to 0.

The (N+1)th is deterministic because `max_requests == N` and the N held requests hold exactly N slots (`activeRequests == N >= maxRequests`). No timing window — the release barrier + poll-to-converge provide determinism (`reference_differential_band_sigma_margin`).

### D-S41-5 — `0074` constants single-sourced
One `const`/`var` block at the top of the `0074` driver (`reference_fixture_workload_constant_desync`): `fixtureName`, `refContainerListenerPort` (the NEXT-FREE port — verify via `grep -rh refContainerListenerPort test/fixtures/ | sort -u`; anticipated **19163**, the next after 0073), `refAdminPort = 9901`, `clusterName = "c_cb"`, `maxRequests = 4` (the N), `backendCount = 1`, `convergeDeadline`, `convergePoll`. The concurrency batch size = `maxRequests` (N held) + 1 (the overflow probe). The asserter + bootstrap/config builders read these — no hand-rolled duplicates.

### D-S41-6 — `circuitBreaker.prio[2]` priority-indexing
`prio [2]cbPriority` indexed by `RoutingPriority` (0 = DEFAULT, 1 = HIGH). `parseCircuitBreakers` populates BOTH slots: each `thresholds[]` entry's `priority` selects its slot; an absent priority defaults to DEFAULT(0); a priority-unspecified slot gets the proto defaults (max_requests 1024). Both slots register their full `circuit_breakers.<default|high>.*` stat tree (AMEND-CB3). Enforcement ALWAYS uses `prio[0]` (DEFAULT) — every request is DEFAULT at 41; `prio[1]` (HIGH) parses + registers but `tryAcquire(1)` is never called.

### D-S41-7 — final ADR-0045 split-gate re-check
Anticipated production LoC (below) ≈ **~260–360 LoC** across ~5 prod files + ~2 harness files; **12 tasks**. Both axes comfortably under the gate (`> ~25 tasks OR > ~1500 LoC`). **NO SPLIT** — phase 41 ships as one flat phase.

---

## File structure

**Production (`internal/`):**
- `internal/cluster/circuitbreaker.go` (CREATE) — `circuitBreaker` + `cbPriority` structs; `parseCircuitBreakers(c *clusterv3.Cluster, name string) (*circuitBreaker, error)` (the 3 reject arms + the per-priority budget parse + defaults); `tryAcquire(prio int) bool` / `release(prio int)`; `registerStats(r *stats.Registry, prefix string)` (the +14 registrations).
- `internal/cluster/cluster.go` (MODIFY, ~:124-128) — the `circuitBreaker *circuitBreaker` field on `Cluster`; the `TryAcquireRequest() bool` + `ReleaseRequest()` methods (no-op when `circuitBreaker == nil`).
- `internal/cluster/manager.go` (MODIFY) — the `parseCircuitBreakers` call in `buildCluster` (~:416, beside `parseOutlierDetection`) + attach to the cluster; the scoped `if c.circuitBreaker != nil { c.circuitBreaker.registerStats(r, prefix) }` block in `registerClusterMetrics` (~:173).
- `internal/filter/http/router/router.go` (MODIFY, ~:591) + `router_h2.go` (MODIFY, ~:60) — the admission `TryAcquireRequest` + `defer ReleaseRequest` + the overflow 503.

**Test harness (`test/`):**
- `test/differential/fixture/fixture.go` (MODIFY) — `BlockingHoldResponder BackendKind = 36` (doc-comment in the existing style).
- `test/differential/runner_test.go` (MODIFY) — the `case fixture.BlockingHoldResponder` spawn arm + `acceptBlockingHold(ln, idx)` (the `/` hold + `/__release` control + 200 response).
- `test/fixtures/0074-circuit-breaker-max-requests/driver/driver.go` + `driver_test.go` (CREATE) + `expectations.yaml` + `README.md` (CREATE).

**Docs:**
- `docs/envoy-go/DECISIONS.md` (ADR-0248 body), `BEHAVIOR_CONTRACT.md` (load-shedding subsection + stat-count 1149 → 1163), `STATE.md`, `ROADMAP.md`, `next-prompt.txt`, the phase `PROGRESS.md` + `README.md`.

---

## Task 1: Baselines + PROGRESS scaffold

**Files:**
- Create: `docs/envoy-go/phases/41-circuit-breakers/PROGRESS.md`

- [ ] **Step 1: Capture the pre-IMPL six-gate baseline.** Run, record in PROGRESS.md:
  - `go build ./...`
  - `go vet ./...`
  - `gofmt -l internal/ test/` (expect empty)
  - `go test ./internal/... 2>&1 | tail -20`
  - `go test ./test/differential/ -count=1 2>&1 | tail -20` (the full **75**-dir suite — the byte-stability anchor)
  - Stat surface: tracked as a documented running total (no count script). Record **1149** (SPEC §14 baseline). The 41 exit total is verified ARITHMETICALLY (1149 + 14 = 1163) against the Task 4 registration test.
- [ ] **Step 2: Record baselines + the task checklist** in PROGRESS.md (counts: stat 1149 / fixtures 75 / fuzzers 42 / BackendKind tail 35 / DECISIONS tail ADR-0247, next-free ADR-0248; the anticipated exit deltas from SPEC §14).
- [ ] **Step 3: Commit.**
```bash
git add docs/envoy-go/phases/41-circuit-breakers/PROGRESS.md
git commit -m "phase 41 Task 1: PROGRESS scaffold + pre-IMPL baselines"
```

---

## Task 2: `parseCircuitBreakers` + the reject roster + the `circuitBreaker`/`cbPriority` structs

**Files:**
- Create: `internal/cluster/circuitbreaker.go`
- Test: `internal/cluster/circuitbreaker_test.go`

The structs (SPEC §3.1; `prio` indexed by RoutingPriority — D-S41-6):
```go
package cluster

import (
	"fmt"
	"sync/atomic"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"<module>/internal/stats" // match the existing import path used in outlier.go
)

// circuitBreaker holds per-RoutingPriority concurrency budgets + live counters.
// Nil on Cluster when no circuit_breakers is configured (TryAcquireRequest is a
// pass-through). Phase 41 enforces max_requests only (ADR-0248, AMEND-CB1);
// max_connections/max_pending_requests register-for-parity-but-defer.
type circuitBreaker struct {
	prio [2]cbPriority // [0]=DEFAULT, [1]=HIGH (HIGH parses+registers but never binds)

	// the LIVE cluster-level overflow counter (the SAME counter max_pending_requests
	// would use — there is NO upstream_rq_overflow; AMEND-CB2).
	upstreamRqPendingOverflow *stats.Counter
}

type cbPriority struct {
	maxRequests    int64        // the enforced budget (default 1024 — AMEND-CB5)
	activeRequests atomic.Int64 // in-flight DEFAULT-priority requests
	rqOpen         *stats.Gauge // 1 while activeRequests >= maxRequests (LIVE for DEFAULT)
}
```

`parseCircuitBreakers` (mirror `parseOutlierDetection`'s shape — returns `(nil, nil)` when absent; the 3 reject arms; D-S41-1/2/6):
```go
// parseCircuitBreakers builds the per-priority budgets from Cluster.circuit_breakers.
// Returns (nil, nil) when absent. Phase 41: max_requests parsed + enforced; the
// other budgets parse-accepted (defaults applied) but enforcement deferred. (ADR-0248)
func parseCircuitBreakers(c *clusterv3.Cluster, name string) (*circuitBreaker, error) {
	cb := c.GetCircuitBreakers()
	if cb == nil {
		return nil, nil
	}
	out := &circuitBreaker{}
	// defaults for any priority with no explicit threshold (AMEND-CB5).
	out.prio[0].maxRequests = 1024
	out.prio[1].maxRequests = 1024
	seen := [2]bool{}
	for _, th := range cb.GetThresholds() {
		p := th.GetPriority()
		if p != corev3.RoutingPriority_DEFAULT && p != corev3.RoutingPriority_HIGH {
			return nil, fmt.Errorf("cluster: %q: circuit_breakers: priority: value must be one of the defined enum values", name)
		}
		idx := int(p) // DEFAULT=0, HIGH=1
		if seen[idx] {
			return nil, fmt.Errorf("cluster: %q: circuit_breakers: duplicate threshold priority", name)
		}
		seen[idx] = true
		// retry_budget enforcement deferred, but validate its percent range if set (D-S41-1).
		if rb := th.GetRetryBudget(); rb != nil && rb.GetBudgetPercent() != nil && rb.GetBudgetPercent().GetValue() > 100 {
			return nil, fmt.Errorf("cluster: %q: circuit_breakers: retry_budget: budget_percent: value must be inside range [0, 100]", name)
		}
		// max_requests: absent ⇒ 1024 default; explicit (incl. 0) ⇒ that value (D-S41-2).
		if v := th.GetMaxRequests(); v != nil {
			out.prio[idx].maxRequests = int64(v.GetValue())
		}
		// max_connections / max_pending_requests / max_retries / max_connection_pools:
		// parse-accepted, enforcement DEFERRED (AMEND-CB1) — no fields stored.
	}
	// per_host_thresholds: silent-ignored (AMEND-CB1).
	return out, nil
}
```
(Resolve the exact `stats` import path + the `corev3` alias by matching `internal/cluster/outlier.go`'s imports.)

- [ ] **Step 1: Write failing tests** in `circuitbreaker_test.go`: (a) absent `circuit_breakers` ⇒ `(nil, nil)`; (b) a DEFAULT threshold with `max_requests: 4` ⇒ `prio[0].maxRequests == 4`, `prio[1].maxRequests == 1024`; (c) absent max_requests ⇒ 1024; (d) explicit `max_requests: 0` ⇒ `prio[0].maxRequests == 0`; (e) `priority: 2` ⇒ the priority reject; (f) two DEFAULT thresholds ⇒ the duplicate reject; (g) `retry_budget.budget_percent: 150` ⇒ the percent reject; (h) `per_host_thresholds` set ⇒ accepted (no error). Build the `*clusterv3.Cluster` inputs in-code (the `outlier_test.go` proto-construction precedent). NOTE: `budget_percent` is a `*typev3.Percent` whose `Value` is a `float64` — construct the (g) input as `&typev3.Percent{Value: 150}` (the production check `rb.GetBudgetPercent().GetValue() > 100` is correct Go: a `float64 > 100` int literal).
- [ ] **Step 2: Run → FAIL** (`parseCircuitBreakers` undefined).
- [ ] **Step 3: Implement** `circuitbreaker.go` (the structs + `parseCircuitBreakers`; leave `tryAcquire`/`release`/`registerStats` as stubs or omit until Task 3/4).
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** gofmt/vet/lint on `internal/cluster/`.
- [ ] **Step 6: Commit** (`phase 41 Task 2: parseCircuitBreakers + reject roster + circuitBreaker struct`).

---

## Task 3: `tryAcquire` / `release` + the `rq_open` gauge + the overflow counter

**Files:**
- Modify: `internal/cluster/circuitbreaker.go`
- Test: `internal/cluster/circuitbreaker_test.go`

The non-blocking try-acquire/release (SPEC §3.2; the LIVE handles `prio[0].rqOpen` + `upstreamRqPendingOverflow` are injected in Task 4 — guard nil for unit testing, or inject test handles):
```go
// tryAcquire reserves a max_requests slot for the given priority. Returns false
// (overflow) when the budget is exhausted, incrementing the overflow counter +
// setting rq_open. (ADR-0248)
func (cb *circuitBreaker) tryAcquire(prio int) bool {
	p := &cb.prio[prio]
	for {
		cur := p.activeRequests.Load()
		if cur >= p.maxRequests {
			if cb.upstreamRqPendingOverflow != nil {
				cb.upstreamRqPendingOverflow.Inc()
			}
			if p.rqOpen != nil {
				p.rqOpen.Set(1)
			}
			return false
		}
		if p.activeRequests.CompareAndSwap(cur, cur+1) {
			if cur+1 >= p.maxRequests && p.rqOpen != nil {
				p.rqOpen.Set(1)
			}
			return true
		}
	}
}

// release returns a slot; clears rq_open when back under budget. (ADR-0248)
func (cb *circuitBreaker) release(prio int) {
	p := &cb.prio[prio]
	if p.activeRequests.Add(-1) < p.maxRequests && p.rqOpen != nil {
		p.rqOpen.Set(0)
	}
}
```

- [ ] **Step 1: Write failing tests** (inject a `*stats.Gauge`/`*stats.Counter` via a tiny registry or set the fields directly): (a) `maxRequests: 2` — two `tryAcquire(0)` return true, the 3rd returns false + `rqOpen==1` + `upstreamRqPendingOverflow==1`; (b) after a `release(0)`, `rqOpen==0` and the next `tryAcquire(0)` returns true; (c) `maxRequests: 0` — the FIRST `tryAcquire(0)` returns false (reject-all, D-S41-2); (d) concurrency: 100 goroutines each `tryAcquire`+(on success)`release` against `maxRequests: 10` never exceed 10 concurrent (use a peak-tracker) and the struct is `-race`-clean.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `tryAcquire`/`release`.
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -race -run CircuitBreaker -count=1`.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 41 Task 3: circuit-breaker tryAcquire/release + rq_open gauge + overflow counter`).

---

## Task 4: The +14 stat registrations (scoped to `circuit_breakers` clusters)

**Files:**
- Modify: `internal/cluster/circuitbreaker.go` — add `registerStats`.
- Modify: `internal/cluster/manager.go` (`registerClusterMetrics`, ~:173) — the scoped block.
- Test: `internal/cluster/circuitbreaker_test.go` (or a manager-level stat test).

`registerStats` — 10 per-priority `*_open` gauges + 4 cluster overflow counters (SPEC §7; only `default.rq_open` + `upstream_rq_pending_overflow` get stored LIVE handles; the other 12 are registered-and-left-at-0):
```go
func (cb *circuitBreaker) registerStats(r *stats.Registry, prefix string) {
	for idx, name := range []string{"default", "high"} {
		gp := prefix + "circuit_breakers." + name + "."
		cb.prio[idx].rqOpen = r.NewGauge(gp + "rq_open") // LIVE for default; high registered but never set
		r.NewGauge(gp + "cx_open")                        // emit-0 (max_connections deferred)
		r.NewGauge(gp + "cx_pool_open")                   // emit-0 (max_connection_pools deferred)
		r.NewGauge(gp + "rq_pending_open")                // emit-0 (max_pending_requests deferred)
		r.NewGauge(gp + "rq_retry_open")                  // emit-0 (max_retries deferred)
	}
	cb.upstreamRqPendingOverflow = r.NewCounter(prefix + "upstream_rq_pending_overflow") // LIVE
	r.NewCounter(prefix + "upstream_cx_overflow")                                        // emit-0
	r.NewCounter(prefix + "upstream_cx_pool_overflow")                                   // emit-0
	r.NewCounter(prefix + "upstream_rq_retry_overflow")                                  // emit-0
}
```
The scoped block in `registerClusterMetrics` (after the `if c.outlier != nil` block, ~:173):
```go
if c.circuitBreaker != nil {
	c.circuitBreaker.registerStats(r, prefix)
}
```

- [ ] **Step 1: Write a failing test** asserting a cluster WITH `circuit_breakers` registers EXACTLY these 14 named stats (the 10 `circuit_breakers.{default,high}.{cx_open,cx_pool_open,rq_open,rq_pending_open,rq_retry_open}` gauges + the 4 `upstream_{cx,cx_pool,rq_pending,rq_retry}_overflow` counters), and a cluster WITHOUT registers NONE of them. Use the registry-introspection pattern from the outlier/health stat test.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `registerStats` + the scoped block.
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5: Stat-surface count → expect 1163** (1149 + 14). Record in PROGRESS.
- [ ] **Step 6:** gofmt/vet/lint.
- [ ] **Step 7: Commit** (`phase 41 Task 4: +14 circuit_breakers stat registrations (1149→1163)`).

---

## Task 5: `Cluster.TryAcquireRequest`/`ReleaseRequest` + the `buildCluster` wiring + byte-stability gate

**Files:**
- Modify: `internal/cluster/cluster.go` (~:124-128) — the `circuitBreaker` field + the two methods.
- Modify: `internal/cluster/manager.go` (`buildCluster`, ~:416) — parse + attach.

On `Cluster` (beside `outlier`):
```go
circuitBreaker *circuitBreaker // nil for clusters with no circuit_breakers
```
```go
// TryAcquireRequest reserves a DEFAULT-priority max_requests slot. Returns false
// (overflow → caller emits 503) when exhausted. No-op true when no circuit_breakers. (ADR-0248)
func (c *Cluster) TryAcquireRequest() bool {
	if c.circuitBreaker == nil {
		return true
	}
	return c.circuitBreaker.tryAcquire(0) // DEFAULT
}

// ReleaseRequest returns the slot acquired by TryAcquireRequest. No-op when no circuit_breakers.
func (c *Cluster) ReleaseRequest() {
	if c.circuitBreaker != nil {
		c.circuitBreaker.release(0)
	}
}
```
In `buildCluster` (beside the `parseOutlierDetection` call, ~:416):
```go
cbCfg, err := parseCircuitBreakers(c, name)
if err != nil {
	return nil, err
}
// ... attach to the constructed cluster: cl.circuitBreaker = cbCfg
```

- [ ] **Step 1: Write a failing test:** a cluster built with `circuit_breakers{max_requests:1}` has non-nil `circuitBreaker` and `TryAcquireRequest()` returns true then false; a cluster without has `TryAcquireRequest()` always true (nil-guard). (Inject the stat handles via the build path or a register call so `rqOpen`/`upstreamRqPendingOverflow` are non-nil.)
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the field, the two methods, the `buildCluster` parse+attach.
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5: BYTE-STABILITY GATE** — `go test ./test/differential/ -count=1` → all **75** still GREEN (no cluster configures `circuit_breakers` yet ⇒ the nil-guard makes the admission path byte-identical). This is the load-bearing regression gate.
- [ ] **Step 6:** gofmt/vet/lint.
- [ ] **Step 7: Commit** (`phase 41 Task 5: Cluster.TryAcquireRequest/ReleaseRequest + buildCluster wiring`).

---

## Task 6: The admission try-acquire + defer-release + the overflow 503

**Files:**
- Modify: `internal/filter/http/router/router.go` (`doH1ClusterAction`, ~:591 — right after `a.cluster.IncUpstreamRqTotal()`).
- Modify: `internal/filter/http/router/router_h2.go` (`doH2ClusterAction`, ~:60 — same spot).

H1 (insert immediately AFTER `a.cluster.IncUpstreamRqTotal()`, BEFORE the `applyHashKey` block):
```go
// Phase 41 (ADR-0248): circuit-breaker max_requests admission. Fail fast with a
// 503 (over-budget) when the cluster's DEFAULT-priority budget is exhausted; the
// release fires on every exit path. No-op (true) when no circuit_breakers.
if !a.cluster.TryAcquireRequest() {
	return ActionResponse{Status: 503, Headers: localReplyHeaders(0), Body: nil}, picked, nil
}
defer a.cluster.ReleaseRequest()
```
H2 (same spot, after `a.cluster.IncUpstreamRqTotal()`; use the H2 local-reply header builder + Status 503):
```go
if !a.cluster.TryAcquireRequest() {
	return ActionResponse{Status: 503, Headers: h2LocalReplyHeaders(), Body: nil}, picked, nil
}
defer a.cluster.ReleaseRequest()
```
NOTE: the overflow path does NOT call `IncStatusClass(503)` (the dedicated `upstream_rq_pending_overflow` counter is the signal; this avoids a speculative `upstream_rq_5xx` cross-side mismatch — the differential does not assert `rq_5xx` for the overflow). The `defer` releases on EVERY exit (success, upstream error, the overflow path is a pre-defer early return so it does NOT double-release). Verify `localReplyHeaders`/`h2LocalReplyHeaders` are in scope (they are — used by the existing 503/502 returns).

- [ ] **Step 1: Write a failing integration-style test** (in the router package, the existing `do{H1,H2}ClusterAction` test pattern, OR rely on the `0074` fixture if the router package has no cluster-backed unit harness — if so, state that in PROGRESS and let Task 8 cover it). Minimal: a router action over a cluster with `circuit_breakers{max_requests:1}` + a backend that blocks; the 2nd concurrent request returns `Status: 503`; both release (the 3rd, after release, succeeds).
- [ ] **Step 2: Run → FAIL** (admission not wired).
- [ ] **Step 3: Implement** the H1 + H2 admission edits.
- [ ] **Step 4: Run → PASS** + `go test ./internal/... -count=1` + the byte-stability gate `go test ./test/differential/ -count=1` (still 75 GREEN — nil-guard).
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 41 Task 6: circuit-breaker admission try-acquire/defer-release + overflow 503 (H1+H2)`).

---

## Task 7: `BlockingHoldResponder` BackendKind 36 + the `/__release` control + the runner spawn arm

**Files:**
- Modify: `test/differential/fixture/fixture.go` — `BlockingHoldResponder BackendKind = 36` (doc-comment in the existing style).
- Modify: `test/differential/runner_test.go` — the `case fixture.BlockingHoldResponder` spawn arm + `acceptBlockingHold(ln, idx)`.

`acceptBlockingHold` (model on `acceptHTTP503Counting` at `runner_test.go:1575`, but HOLD `GET /` until `GET /__release`; re-armable per-batch; D-S41-4). The held-request release uses a shared channel guarded by a mutex; `/__release` swaps in a fresh channel and closes the old one:
```go
func acceptBlockingHold(ln net.Listener, idx int) {
	var mu sync.Mutex
	gate := make(chan struct{}) // closed by /__release to free the current batch
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			br := bufio.NewReader(c)
			req, err := http.ReadRequest(br)
			if err != nil {
				return
			}
			_, _ = io.Copy(io.Discard, req.Body)
			_ = req.Body.Close()
			if req.URL.Path == "/__release" {
				mu.Lock()
				old := gate
				gate = make(chan struct{}) // re-arm for the next batch
				mu.Unlock()
				close(old) // free everyone currently held
				body := "released"
				_, _ = fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
				return
			}
			// a normal request: block until the current batch is released.
			mu.Lock()
			g := gate
			mu.Unlock()
			<-g
			seg := req.URL.Path
			if i := strings.LastIndex(seg, "/"); i >= 0 && i+1 < len(seg) {
				seg = seg[i+1:]
			}
			body := fmt.Sprintf("backend-%d:%s", idx, seg)
			_, _ = fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
		}(c)
	}
}
```
The spawn arm (inside the runner spawn loop, beside `case fixture.HTTP503Responder`):
```go
case fixture.BlockingHoldResponder:
	// listen + go acceptBlockingHold(ln, bo.idx)
```
(`0074` uses a UNIFORM `BackendKind()` = `BlockingHoldResponder` for its single backend — no `PerHostBackendKind` needed. Confirm the spawn loop reads `BackendKind()` when `PerHostBackendKind` is not implemented.)

- [ ] **Step 1:** Add the BackendKind constant + the spawn arm + `acceptBlockingHold`. (Optional tiny loopback test: a held `GET /` does not return until a concurrent `GET /__release`, then returns 200 + `backend-<idx>:`.)
- [ ] **Step 2: Run** `go build ./... && go vet ./test/... && go test ./test/differential/ -count=1` → all **75** still GREEN (no existing fixture uses `BlockingHoldResponder`).
- [ ] **Step 3:** gofmt/vet/lint on `test/...`. Record BackendKind tail **35 → 36**.
- [ ] **Step 4: Commit** (`phase 41 Task 7: BlockingHoldResponder BackendKind 36 + /__release control + runner spawn arm`).

---

## Task 8: The `0074` cross-side fixture

**Files:**
- Create: `test/fixtures/0074-circuit-breaker-max-requests/driver/driver.go`
- Create: `test/fixtures/0074-circuit-breaker-max-requests/driver/driver_test.go` (the `backendIdxFromBody` unit test — copy the per-fixture helper, the 0066/0069 precedent)
- Create: `test/fixtures/0074-circuit-breaker-max-requests/expectations.yaml`
- Create: `test/fixtures/0074-circuit-breaker-max-requests/README.md`

Model on `test/fixtures/0066-health-check-http/driver/driver.go` for the cross-side shape (reference STRICT_DNS / `host.docker.internal`; subject STATIC / `127.0.0.1`) + the `scrapeStats`/poll helpers. Topology: cluster `c_cb`, lb ROUND_ROBIN, **1** endpoint (`BlockingHoldResponder`), `circuit_breakers: { thresholds: [ { priority: DEFAULT, max_requests: <maxRequests=4> } ] }`, on BOTH sides. The driver implements `BackendCount() == 1` (uniform `BackendKind() == BlockingHoldResponder`) + the `StatsAsserter` interface. Constants single-sourced (D-S41-5).

The `AssertStats(t, refAdminAddr, subjAdminAddr)` flow — run for EACH side SEQUENTIALLY (subject first, then reference; the shared backend is idle between sides — D-S41-4). For a side with listener `addr` + admin `adminAddr`:
```go
// 1. Fire N=maxRequests concurrent GET / (each blocks at BlockingHoldResponder).
var wg sync.WaitGroup
bodies := make([]string, maxRequests)
errs := make([]error, maxRequests)
for i := 0; i < maxRequests; i++ {
	wg.Add(1)
	go func(i int) {
		defer wg.Done()
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil { errs[i] = err; return }
		if resp.StatusCode != 200 { errs[i] = fmt.Errorf("held req %d: status %d", i, resp.StatusCode); return }
		bodies[i] = string(body)
	}(i)
}
// 2. POLL adminAddr /stats until circuit_breakers.default.rq_open == 1 (deadline; NO sleep-as-sync).
//    (confirms all N slots filled — activeRequests==maxRequests.)
// 3. Record the upstream_rq_pending_overflow baseline, then fire the (N+1)th GET / →
//    assert resp.StatusCode == 503 (the proxy rejects it before the backend).
// 4. Re-scrape: assert circuit_breakers.default.rq_open == 1 AND
//    (upstream_rq_pending_overflow - baseline) >= 1. Verify upstream_rq_total > 0 (decode-ran).
// 5. Release: GET addr→ but the RELEASE must hit the BACKEND control port, not the proxy:
//    helpers.HTTPRoundTrip(ctx, "127.0.0.1:"+backendPort, "GET", "/__release", ...).
//    (the driver holds backendPorts from ReferenceBootstrap/SubjectConfig — cache them.)
// 6. wg.Wait() (the N held requests now return 200, bodies "backend-0:"). Poll rq_open → 0.
```
Cross-side assertions (BOTH sides, via `StatsAsserter`): the (N+1)th is `503`; `circuit_breakers.default.rq_open == 1` at saturation; `upstream_rq_pending_overflow` delta `>= 1`; final `rq_open == 0` after release. Do NOT assert the `UO` access-log flag (D-S41-3 departure) or `upstream_rq_5xx` (Task 6 note). Use `StatsAsserter` (cross-side), NOT `SubjectAsserter` (`reference_differential_asserter_dispatch`).

★ The driver caches `backendPorts` (passed to `ReferenceBootstrap`/`SubjectConfig`) so `AssertStats` can hit `127.0.0.1:<backendPort>/__release` — the in-process backend is reachable on loopback from the driver for BOTH sides' phases.

- [ ] **Step 1:** Write `driver_test.go` (the `backendIdxFromBody` table test) → run → FAIL (helper undefined).
- [ ] **Step 2:** Write `driver.go` (the helper + the full fixture: `BackendCount`, `ReferenceBootstrap`, `SubjectConfig`, `ReferenceListenerPort`, `DriveReference`/`DriveSubject`, `ProbeAdmin`, `AssertStats`) + `expectations.yaml` + `README.md`. Single-source the constants.
- [ ] **Step 3:** `go test ./test/fixtures/0074-circuit-breaker-max-requests/driver/ -count=1` (the unit test) → PASS.
- [ ] **Step 4: Run the cross-side fixture** (requires Docker + `contrib-v1.37.2`): `go test ./test/differential/ -run 'TestDifferential/0074' -count=1 -v` (`reference_differential_run_selector` — the `TestDifferential/` prefix is REQUIRED). Expected: PASS — both sides reject the (N+1)th with 503, `rq_open` converges to 1 then 0, the overflow counter increments.
- [ ] **Step 5:** gofmt/vet/lint. Record fixtures **75 → 76**.
- [ ] **Step 6: Commit** (`phase 41 Task 8: 0074 cross-side circuit-breaker-max-requests fixture`).

---

## Task 9: `0074` deliberate breaks + 20-run flake

**Files:** none committed (verification only; the SPEC §8.1 break protocol).

★ Use `-count=1` for EVERY break run (`reference_differential_break_protocol_count1`) + the `TestDifferential/0074` selector.

- [ ] **Step 1: Break (A) — `max_requests` never enforced.** Temporarily make `Cluster.TryAcquireRequest` always return true (or `circuitBreaker.tryAcquire` always true). Run `go test ./test/differential/ -run 'TestDifferential/0074' -count=1` → MUST FAIL (the (N+1)th request is NOT rejected → no 503, no overflow delta; or `rq_open` never reaches 1). Restore.
- [ ] **Step 2: Break (B) — the gauge/counter not wired.** Temporarily make `tryAcquire`'s overflow path skip `upstreamRqPendingOverflow.Inc()` (or skip `rqOpen.Set(1)`). Run → MUST FAIL (the cross-side `upstream_rq_pending_overflow` delta / `rq_open` parity assert fails). Restore.
- [ ] **Step 3: Confirm both breaks restored** (`git diff` clean; `go test ./test/differential/ -run 'TestDifferential/0074' -count=1` → PASS).
- [ ] **Step 4: 20-run flake gate:** `for i in $(seq 20); do go test ./test/differential/ -run 'TestDifferential/0074' -count=1 || echo "FAIL $i"; done` → 20/20 PASS (the release barrier + poll-to-converge make it deterministic; if any flake, widen `convergeDeadline`, NEVER add a fixed sleep — `reference_differential_band_sigma_margin`).
- [ ] **Step 5:** Record the break + flake results in PROGRESS. (No commit.)

---

## Task 10: Full 76-dir differential + six-gate

**Files:** none (verification); update PROGRESS.

- [ ] **Step 1: Six-gate** (ADR-0052): `go build ./...`; `go vet ./...`; `gofmt -l internal/ test/` (empty); `golangci-lint run ./...`; `go test ./internal/... -count=1`; `go test ./test/differential/ -count=1` (ALL **76** GREEN). NOTE the full suite can transiently hit the unrelated `subject ready: EOF` startup flake (`reference_differential_fullsuite_startup_flake`) — isolate-re-run the named dir + a full re-run to distinguish it from a regression.
- [ ] **Step 2: Counts → stat surface 1163; fixtures 76; fuzzers 42 (unchanged); BackendKind tail 36.** Record in PROGRESS.
- [ ] **Step 3:** If any gate fails, fix + re-run before proceeding.

---

## Task 11: ADR-0248 body + BEHAVIOR_CONTRACT delta

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` — the ADR-0248 full entry (§Decision + §Consequences; the §Context is drafted in SPEC §13 — promote/refine it). DECISIONS tail ADR-0247 → **ADR-0248** (next-free ADR-0249).
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the new `### Cluster — load-shedding (circuit breakers)` subsection (SPEC §9). Advance the stat-surface block **1149 → 1163**.

- [ ] **Step 1:** Write the ADR-0248 body. §Decision: the per-priority `circuitBreaker` counter struct; the synchronous `TryAcquireRequest` at `do{H1,H2}ClusterAction` admission + `defer ReleaseRequest`; the fail-fast 503 + `upstream_rq_pending_overflow` + `rq_open`; the `max_requests`-only enforcement (AMEND-CB1) with cx/pending register-for-parity-but-defer; the +14 stat block (12 emit-0); DEFAULT-only enforce / HIGH register-emit-0. §Consequences: byte-stable when absent; the `UO` access-log flag DEFERRED (no plumbing — D-S41-3); the cx/pending enforcement → the future connection-pooling family row; the reference's max_connections-pends-not-fails + shared-overflow-counter findings recorded.
- [ ] **Step 2:** Write the BEHAVIOR_CONTRACT subsection (the `max_requests` cap; the 503/overflow/`rq_open`; the +14 surface + the emit-0/departure list incl. the UO flag) + the stat-count bump 1149 → 1163.
- [ ] **Step 3:** `go build ./...` (docs-only sanity).
- [ ] **Step 4: Commit** (`phase 41 Task 11: ADR-0248 body + BEHAVIOR_CONTRACT load-shedding subsection (stat 1149→1163)`).

---

## Task 12: Completion bundle

**Files:**
- Modify: `docs/envoy-go/phases/41-circuit-breakers/PROGRESS.md` (final state + exit-delta table); `docs/envoy-go/phases/41-circuit-breakers/README.md` (CREATE — status PLAN-done → IMPL-done); `docs/envoy-go/STATE.md` (active-phase → `phase 41 (circuit-breakers) IMPL done`; counts → 1163 / 76 / 42 / 36 / ADR-0248); `docs/envoy-go/ROADMAP.md` (row 41 `in-progress → done`); `next-prompt.txt` (roll forward to a FRESH BRAINSTORM — the Upstream-robustness family stays open, 2 candidates remain).

- [ ] **Step 1:** Update PROGRESS (the 12-task record + the six-gate evidence + the break/flake results + the exit-delta table).
- [ ] **Step 2:** Write the phase README; update STATE/ROADMAP/next-prompt per the precedent (row 41 → `done` — a flat un-split family row, NO parent rollup per ADR-0106; the family stays open).
- [ ] **Step 3: Final six-gate re-confirm** + record all exit counts.
- [ ] **Step 4: Commit** (`phase 41 Task 12: completion bundle — ROADMAP row 41 done; circuit breakers (max_requests) landed`).
- [ ] **Step 5:** The controller squashes the 12 task commits + pushes to origin/master (`feedback_subagents_no_push` — subagents commit locally only; the controller squashes at stage-close + pushes per `feedback_push_to_origin`).

---

## Exit deltas (SPEC §14)

| Axis | At PLAN | At 41 IMPL |
|------|---------|-----------|
| stat surface | 1149 | **1163** (+14) |
| differential fixtures | 75 | **76** (`0074`) |
| fuzzers | 42 | 42 (unchanged) |
| BackendKind tail | 35 | **36** (`BlockingHoldResponder`) |
| DECISIONS tail | ADR-0247 | **ADR-0248** (next-free ADR-0249) |
| new packages / go.mod modules | — | ZERO / ZERO |
| ROADMAP row 41 | in-progress | **done** (flat un-split family row; NO parent rollup) |

Next → the phase-41 IMPL (`superpowers:subagent-driven-development` — fresh subagent per task + two-stage review).
