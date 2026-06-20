# Retries (`retry_policy` — the retry loop, phase 42.1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land HTTP retries on `RouteAction.retry_policy` — a `retryExecutor` re-invokes the existing single-attempt driver when the buffered response (or a local-origin failure) matches the parsed `retry_on` subset, up to `num_retries`, sleeping an exponential full-jitter backoff and replaying the buffered body between attempts, capped by the activated phase-41 `retry_budget` slot; with the +5 retry counters (scoped to retry-policy clusters) and the 2 phase-41 stats (`rq_retry_open`/`upstream_rq_retry_overflow`) flipping LIVE.

**Architecture:** A `retryExecutor` in `internal/filter/http/router` WRAPPING `doH1ClusterAction`/`doH2ClusterAction` (which already re-run CB admission `TryAcquireRequest` + the outcome seam `RecordUpstreamResult` per call — so every attempt is correctly accounted). The loop classifies `ActionResponse.Status` (+ a new `localOrigin` signal) against a parsed `retry_on` bitset; on a retriable outcome with attempts + budget remaining it sleeps the backoff, replays the buffered body, and re-invokes the driver. Nil `retryPolicy` ⇒ direct pass-through (BYTE-IDENTICAL). `retry_policy` parsed in `buildRouterAction` (hcm), carried on `clusterRouteAction`, threaded into `H1ClusterAction`/`H2ClusterAction`. The `retry_budget` is activated on the phase-41 `circuitBreaker` (`activeRetries` + `tryAcquireRetry`/`releaseRetry`). ZERO new packages/modules.

**Tech Stack:** Go; `route.v3.RetryPolicy` (go-control-plane v1.32.4 — already vendored); `internal/stats`; the `test/differential` cross-side harness (reference `envoyproxy/envoy:contrib-v1.37.2` over a Docker bridge).

This PLAN implements `docs/envoy-go/phases/42-retries-and-hedging/SPEC.md` (read it first). Counts at PLAN commit UNCHANGED (stat surface **1163** / fixtures **76** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0248**, next-free **ADR-0249**). The module path is `github.com/esalaine/envoy-go`. All anchors below are verified against the worktree at `327acf62`.

---

## D-question resolutions (SPEC §12) — settled at PLAN

The implementer MUST follow these (baked into the tasks).

### D-S42-1 — house reject wording (§6)
Mirror the `parseCircuitBreakers` precedent (`internal/cluster/circuitbreaker.go:45-53`, prefix `cluster: %q: circuit_breakers: `). The retry reject arms use the prefix `route: %q: retry_policy: ` + the reason (mirroring the reference wording where it exists, per ADR-0080). The parse/reject lives inside the `RouteAction_Cluster` arm of `buildRouterAction`, keyed by the cluster name `cs.Cluster` (the `%q`); for the weighted-cluster arm, key it by the route/weighted-action name available there (or thread a `name` param into `buildRouterAction` — `buildRouterAction(r, clusters, vhRetryPolicy)` gains the resolved name). The arms:
- `route: %q: retry_policy: retry_back_off: base_interval must be greater than 0s` — when `retry_back_off` is set AND `base_interval` is absent or ≤ 0 (reference PGV: required + `value must be greater than 0s`).
- `route: %q: retry_policy: max_interval must be greater than or equal to the base_interval` — when `retry_back_off` is set, `max_interval` is set, AND `max_interval < base_interval` (reference RUNTIME boot-reject, live-confirmed `--mode validate`).

NOT rejected: `retry_on` tokens (freeform; unknown SILENTLY IGNORED — no error); `num_retries` (unbounded, any value incl. 0 ⇒ no retries); `retriable_status_codes[]` values (unbounded). All arms unit-level (no boot-reject fixture dir — the 41 precedent).

### D-S42-2 — fuzzer? NO
`retry_policy` is config-parse (no new wire decoder); the parse/reject + the `retry_on` tokenizer are unit-tested (the 40.x/41 precedent). Fuzzers STAY **42**.

### D-S42-3 — the over-cap body guard: NONE needed at the router ★
The HCM ALWAYS buffers the request body to ≤ 1 MiB and an over-cap body synthesizes a **413 inside `RunDecodeData` BEFORE the router runs** (`connection.go:564-567`: "this branch never sees an over-cap body in practice"); the buffered bytes are handed to the router as `req.Body = io.NopCloser(bytes.NewReader(bodyBuf))` (`connection.go:600`). So the retryExecutor's `req.Body` is ALWAYS a finite in-memory reader (or `http.NoBody`) — the executor captures the bytes ONCE (`io.ReadAll`, cheap, already in memory) before the loop and resets `req.Body = io.NopCloser(bytes.NewReader(captured))` before EACH attempt. NO over-cap guard, NO HCM change. (The over-cap-→-413 is the existing HCM behavior; record it as the non-retriable boundary in the ADR/BEHAVIOR_CONTRACT, not new code.)

### D-S42-4 — the `VirtualHost.retry_policy` fallback threading
`retry_policy` resolves route-first, vhost-fallback (the `includeVhRateLimits`/`GetRateLimits` precedent at `config.go:427-446`). In `buildRouteTable` (`config.go:406`), capture the vhost's `vh.GetRetryPolicy()` once; thread it down the EXISTING dispatch chain `buildRouteTable` → `buildAction` (`config.go:507`, the action-arm switch) → `buildRouterAction` (`config.go:536`) as a new `vhRetryPolicy *routev3.RetryPolicy` param on `buildAction`/`buildRouterAction`. `buildRouterAction` computes `effective := r.GetRetryPolicy(); if effective == nil { effective = vhRetryPolicy }`. Absent on BOTH ⇒ `retryPolicy == nil` ⇒ no retry (pass-through). Route-level presence OVERRIDES vhost (no merge). (NOTE: the real dispatcher is `buildAction` — NOT `buildRouteAction`, which does not exist.)

### D-S42-5 — the retry-counter scoping: lazy `EnsureRetryStats()` at Action-build ★ load-bearing
The +5 retry counters (`upstream_rq_retry`/`_retry_success`/`_retry_limit_exceeded`/`_retry_backoff_exponential`/`_retry_backoff_ratelimited`) register ONLY on clusters that are a retry-policy target (a recorded departure from the reference's always-on — keeps every existing fixture's `/stats` byte-stable). Mechanism: `registerClusterMetrics` (`manager.go:112`) stashes the registry + prefix on the `Cluster` (`c.statsReg`, `c.statsPrefix`); `Cluster.EnsureRetryStats()` (idempotent — a nil-guard, config build is single-threaded) allocates the 5 counters on first call. `buildRouterAction` (hcm) calls `cl.EnsureRetryStats()` when the effective `retry_policy != nil`. Multiple retry routes → the same cluster ⇒ the nil-guard registers once. Surface **+5 per retry cluster**; the `0075` fixture has TWO retry clusters (`c_exhaust` + `c_recover`) ⇒ **1163 → 1173**.

### D-S42-6 — `0075` constants single-sourced + the recover-arm stat
One `const`/`var` block at the top of the `0075` driver (`reference_fixture_workload_constant_desync`): `fixtureName`, `refContainerListenerPort = 19164` (the NEXT-FREE — verify via `grep -rhoE 'refContainerListenerPort[[:space:]]*=[[:space:]]*[0-9]+' test/fixtures/ | grep -oE '[0-9]+' | sort -n | tail -1` ⇒ 19163, so 19164), `refAdminPort = 9901`, `clusterExhaust = "c_exhaust"`, `clusterRecover = "c_recover"`, `numRetries = 3` (the exhaustion N), `recoverReqs = 8` (the K — even), `convergeDeadline`, `convergePoll`. The EXHAUSTION arm asserts the cluster counters cross-side EXACT (`c_exhaust.upstream_rq_retry == numRetries`, `_retry_limit_exceeded == 1`, `upstream_rq_total == numRetries+1`, final 503). The RECOVER arm asserts the OFFSET-INVARIANT downstream stat `http.<stat_prefix>.downstream_rq_2xx` delta == `recoverReqs` cross-side (every client recovered — AMEND-RT4; the reference randomizes the RR offset so the exact `upstream_rq_retry` count is NOT cross-side-asserted) + `c_recover.upstream_rq_retry_limit_exceeded == 0` cross-side + subject-side `c_recover.upstream_rq_retry_success == upstream_rq_retry > 0`. (`stat_prefix` is the HCM listener's `stat_prefix` in the bootstrap — single-source it.)

### D-S42-7 — the `retry_budget` activation shape + the `retry_success`/`limit_exceeded` firing conditions
`activeRetries atomic.Int64` lives on the `circuitBreaker` (cluster-level; DEFAULT-priority — every request is DEFAULT). `tryAcquireRetry()`: atomically, if `activeRetries >= max(minRetryConcurrency, ⌈budgetPercent% × prio[0].activeRequests⌉)` ⇒ `upstreamRqRetryOverflow.Inc()` + `prio[0].rqRetryOpen.Set(1)` + return false; else `activeRetries++` + return true. `releaseRetry()`: `activeRetries--`; clear `rqRetryOpen` when back under the cap. The budget is consulted ONLY when `circuitBreaker != nil` AND a `retry_budget` was configured (else `TryAcquireRetry()` returns true — unlimited up to `num_retries`). **`upstream_rq_retry_limit_exceeded`** fires when a request that DID retry exhausts `num_retries` AND the final attempt's outcome was itself retriable (we are in the "retriable + attempt >= numRetries" branch). **`upstream_rq_retry_success`** fires when a request that retried (`attempt > 0`) ends on a NON-retriable outcome (it recovered). Both unit-tested.

### D-S42-8 — the budget-overflow differential: DEFERRED (UNIT-tested at 42.1)
Budget overflow is concurrency-shaped (the live probe needed 120 concurrent for 119 overflows — not a deterministic count cross-side). 42.1 UNIT-tests the activation (Task 6); the deterministic differential core is the `0075` retry loop. NO budget differential arm.

### D-S42-9 — final ADR-0045 split-gate re-check
Anticipated production LoC (below) ≈ **~320–430 LoC** across ~6 prod files + ~2 harness files; **13 tasks**. Both axes comfortably under the gate (`> ~25 tasks OR > ~1500 LoC`). **NO FURTHER SPLIT** of 42.1 — it ships as the first leg; 42.2 (hedging) is the pre-authorized second leg (a later session).

---

## File structure

**Production (`internal/`):**
- `internal/filter/http/router/retry.go` (CREATE) — `RetryPolicy` (parsed: `retryOn` bitset, `numRetries int`, `retriableCodes map[int]bool`, `baseInterval`/`maxInterval time.Duration`); `NewRetryPolicy(retryOn string, numRetries uint32, retriableCodes []uint32, baseInterval, maxInterval time.Duration) (*RetryPolicy, error)` (tokenize + backoff reject); `(*RetryPolicy).matches(status int, localOrigin bool) bool`; `(*RetryPolicy).backoff(attempt int) time.Duration` (exponential full jitter); the `retryExecutorH1`/`retryExecutorH2` loops.
- `internal/filter/http/router/router.go` (MODIFY) — `routerAction.retryPolicy *RetryPolicy`; `H1ClusterAction(..., rp *RetryPolicy)`; the closure runs `retryExecutorH1` when `rp != nil`; `ActionResponse.localOrigin bool` (~:119) set at the synthesized **dial/connection-failure** return sites (~:624/:651/:664). NOT at the CB-overflow 503 (~:597) — an overflow stays `localOrigin=false` so `connect-failure` does not match it (a `5xx`/`gateway-error` retry_on still matches it by its 503 status).
- `internal/filter/http/router/router_h2.go` (MODIFY) — `routerActionH2.retryPolicy`; `H2ClusterAction(..., rp *RetryPolicy)`; `retryExecutorH2`; `localOrigin` at the H2 dial/roundtrip-failure sites (~:88/:107). NOT at the H2 CB-overflow 503 (~:64) nor the ctx-cancel `Status:0` (~:103).
- `internal/filter/http/router/router_weighted.go` (MODIFY) — thread `rp` into the per-entry closures; the wrapping happens at the `do{H1,H2}ClusterAction` call sites (~:110/:123) via the same closure-switch pattern as Task 7/8 (the weighted closures call the `do*` drivers directly, not `H{1,2}ClusterAction`).
- `internal/filter/hcm/actions.go` (MODIFY, :201) — `clusterRouteAction.retryPolicy *router.RetryPolicy`; pass it in `asRouterAction`/`asRouterActionH2` (:236/:249); weighted builder threads it too.
- `internal/filter/hcm/config.go` (MODIFY) — `buildRouterAction` (:536) parses `retry_policy` (+ reject arms) → `router.NewRetryPolicy`, calls `cl.EnsureRetryStats()`; `buildRouteTable` (:406) / `buildAction` (:507) thread the vhost `retry_policy` fallback (D-S42-4).
- `internal/cluster/cluster.go` (MODIFY) — `statsReg`/`statsPrefix`/`retry *retryStats` fields; `EnsureRetryStats()`; the 5 `IncUpstreamRq*` methods; `TryAcquireRetry()`/`ReleaseRetry()` (no-op true when no `circuitBreaker`/`retry_budget`).
- `internal/cluster/circuitbreaker.go` (MODIFY) — `cbPriority.rqRetryOpen *stats.Gauge`; `circuitBreaker.{upstreamRqRetryOverflow *stats.Counter, activeRetries atomic.Int64, budgetPercent float64, minRetryConcurrency int64, hasRetryBudget bool}`; store the DEFAULT `rqRetryOpen` + `upstreamRqRetryOverflow` handles LIVE in `registerStats`; parse `retry_budget` into the fields; `tryAcquireRetry()`/`releaseRetry()`.
- `internal/cluster/manager.go` (MODIFY, `registerClusterMetrics` :112) — stash `c.statsReg = r; c.statsPrefix = prefix`.

**Test harness (`test/`):**
- `test/fixtures/0075-retry-loop/driver/driver.go` + `driver_test.go` + `expectations.yaml` + `README.md` (CREATE). NO `fixture.go`/`runner_test.go` change (REUSE `HTTP503Responder` 35 + `HTTPEcho` 1).

**Docs:** `DECISIONS.md` (ADR-0249 body), `BEHAVIOR_CONTRACT.md` (retries subsection + stat 1163 → 1173), `STATE.md`, `ROADMAP.md`, `next-prompt.txt`, the phase `PROGRESS.md` + `README.md`.

---

## Task 1: Baselines + PROGRESS scaffold

**Files:** Create `docs/envoy-go/phases/42-retries-and-hedging/PROGRESS.md`

- [ ] **Step 1: Capture the pre-IMPL six-gate baseline.** Run + record: `go build ./...`; `go vet ./...`; `gofmt -l internal/ test/` (empty); `go test ./internal/... 2>&1 | tail -20`; `go test ./test/differential/ -count=1 2>&1 | tail -20` (the full **76**-dir suite — the byte-stability anchor). Stat surface: documented running total — record **1163** (SPEC §14). The 42.1 exit is verified ARITHMETICALLY (1163 + 10 = 1173) against the Task 5 registration test.
- [ ] **Step 2: Record baselines + the task checklist** (counts: stat 1163 / fixtures 76 / fuzzers 42 / BackendKind tail 36 / DECISIONS tail ADR-0248, next-free ADR-0249; the SPEC §14 exit deltas).
- [ ] **Step 3: Commit** (`phase 42.1 Task 1: PROGRESS scaffold + pre-IMPL baselines`).

---

## Task 2: `retry_on` tokenizer + bitset + the `matches` classifier

**Files:** Create `internal/filter/http/router/retry.go`; Test `internal/filter/http/router/retry_test.go`

The enforced subset (SPEC §5/AMEND-RT1). `localOrigin` distinguishes a synthesized dial/connection failure from an upstream 5xx RESPONSE (the live `connect-failure` ≠ 502-response finding):
```go
package router

type retryOnBits uint8

const (
	retry5xx          retryOnBits = 1 << iota // any upstream 5xx
	retryGatewayError                         // {502,503,504}
	retryConnectFail                          // local-origin connect/reset failure
	retryStatusCodes                          // status ∈ retriableCodes
)

// parseRetryOn tokenizes the freeform retry_on string. Unknown/deferred tokens
// (retriable-headers, envoy-ratelimited, grpc-*) are SILENTLY IGNORED (no PGV;
// AMEND-RT1). Whitespace/comma separated.
func parseRetryOn(s string) retryOnBits {
	var b retryOnBits
	for _, tok := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' }) {
		switch tok {
		case "5xx":
			b |= retry5xx
		case "gateway-error":
			b |= retryGatewayError
		case "connect-failure", "reset":
			b |= retryConnectFail
		case "retriable-status-codes":
			b |= retryStatusCodes
		}
	}
	return b
}

// matches reports whether an attempt's outcome is retriable. localOrigin marks a
// router-synthesized dial/connection failure (vs an upstream response).
func (rp *RetryPolicy) matches(status int, localOrigin bool) bool {
	if rp.on&retryConnectFail != 0 && localOrigin {
		return true
	}
	if rp.on&retry5xx != 0 && status >= 500 && status <= 599 {
		return true
	}
	if rp.on&retryGatewayError != 0 && (status == 502 || status == 503 || status == 504) {
		return true
	}
	if rp.on&retryStatusCodes != 0 && rp.retriableCodes[status] {
		return true
	}
	return false
}
```
(The `RetryPolicy` struct + `on retryOnBits` + `retriableCodes map[int]bool` fields are declared here; `numRetries`/`baseInterval`/`maxInterval` are added in Task 3.)

- [ ] **Step 1: Write failing tests** in `retry_test.go`: (a) `parseRetryOn("5xx")` sets `retry5xx`; (b) `"gateway-error, connect-failure"` sets both; (c) `"envoy-ratelimited grpc-internal foo"` ⇒ 0 (all ignored); (d) `matches`: `5xx`→503 true, →200 false; `gateway-error`→502 true, →500 false; `connect-failure`→(502, localOrigin=true) true, →(502, localOrigin=false) **false** (the live finding — an upstream 502 is NOT a connect failure); `retriable-status-codes` with `retriableCodes{500:true}`→500 true, →503 false.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the tokenizer + `matches` + the partial `RetryPolicy` struct.
- [ ] **Step 4: Run → PASS** (`go test ./internal/filter/http/router/ -run Retry -count=1`).
- [ ] **Step 5:** gofmt/vet/lint on the router pkg.
- [ ] **Step 6: Commit** (`phase 42.1 Task 2: retry_on tokenizer + matches classifier`).

---

## Task 3: The `RetryPolicy` constructor + the exponential full-jitter backoff + the reject arms

**Files:** Modify `internal/filter/http/router/retry.go`; Test `retry_test.go`

```go
import ("math/rand"; "time"; ...)

type RetryPolicy struct {
	on             retryOnBits
	numRetries     int
	retriableCodes map[int]bool
	baseInterval   time.Duration // default 25ms
	maxInterval    time.Duration // default 10×base
}

// NewRetryPolicy builds a parsed policy. baseInterval/maxInterval are the parsed
// retry_back_off (zero ⇒ defaults). Returns an error for the backoff reject arms
// (D-S42-1). retry_on is freeform (never an error). num_retries==0 ⇒ no retries.
func NewRetryPolicy(retryOn string, numRetries uint32, retriableCodes []uint32, baseInterval, maxInterval time.Duration) (*RetryPolicy, error) {
	rp := &RetryPolicy{on: parseRetryOn(retryOn), numRetries: int(numRetries), retriableCodes: map[int]bool{}}
	for _, c := range retriableCodes {
		rp.retriableCodes[int(c)] = true
	}
	rp.baseInterval = baseInterval
	if rp.baseInterval <= 0 {
		rp.baseInterval = 25 * time.Millisecond // default (AMEND-RT6)
	}
	rp.maxInterval = maxInterval
	if rp.maxInterval <= 0 {
		rp.maxInterval = 10 * rp.baseInterval // default 10×base
	}
	if rp.maxInterval < rp.baseInterval {
		return nil, fmt.Errorf("max_interval must be greater than or equal to the base_interval")
	}
	return rp, nil
}

// backoff returns the delay before retry attempt n (1-based): full jitter over
// [0, min(maxInterval, baseInterval·2^(n-1))]. Delay-only — never perturbs counts.
func (rp *RetryPolicy) backoff(n int) time.Duration {
	d := rp.baseInterval << uint(n-1)
	if d > rp.maxInterval || d <= 0 {
		d = rp.maxInterval
	}
	return time.Duration(rand.Int63n(int64(d) + 1))
}
```
NOTE: the `base_interval`-required/≤0 reject is checked in `buildRouterAction` (it knows whether `retry_back_off` was SET in the proto — a zero `baseInterval` passed here means "unset" and gets the default; an explicitly-set-but-zero base is rejected in hcm before calling this). The `max < base` reject lives here (it sees the resolved values).

- [ ] **Step 1: Write failing tests:** (a) `NewRetryPolicy("5xx", 3, nil, 0, 0)` ⇒ `numRetries==3`, `baseInterval==25ms`, `maxInterval==250ms`; (b) `NewRetryPolicy("5xx", 1, nil, 100ms, 50ms)` ⇒ error (`max < base`); (c) `backoff(n)` for n=1..5 is always in `[0, maxInterval]` and `>= 0` (run 1000×, assert bounds — NOT a fixed value, it's jittered); (d) `num_retries==0` ⇒ `numRetries==0`.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run → PASS** + `go test ./internal/filter/http/router/ -race -run Retry -count=1`.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 42.1 Task 3: RetryPolicy constructor + exponential full-jitter backoff + max<base reject`).

---

## Task 4: The `retry_policy` parse in hcm + the vhost fallback + carry on `clusterRouteAction` + thread the signatures

**Files:** Modify `internal/filter/hcm/config.go` (`buildRouteTable` :406, `buildAction` :507, `buildRouterAction` :536); `internal/filter/hcm/actions.go` (:201, :236, :249); `internal/filter/http/router/router.go` + `router_h2.go` + `router_weighted.go` (the `H{1,2}ClusterAction` signatures); Test `internal/filter/hcm/config_test.go` (or a new retry parse test).

This task wires the PARSE + the signature threading WITHOUT running the executor yet — `routerAction.retryPolicy` is set but the closure still calls `doH{1,2}ClusterAction` directly when nil (Task 7 adds the loop). Byte-stable: no existing fixture sets `retry_policy`.

- `buildRouterAction` (config.go:536) gains a `vhRetryPolicy *routev3.RetryPolicy` param (threaded from `buildRouteTable` via `buildAction` at :507); it computes `eff := r.GetRetryPolicy(); if eff == nil { eff = vhRetryPolicy }`; if `eff != nil`:
  - **num_retries default** (AMEND-RT6): `v := uint32(1); if nr := eff.GetNumRetries(); nr != nil { v = nr.GetValue() }`.
  - extract the backoff (guarded `.AsDuration()` — `*durationpb.Duration.AsDuration()` must not be called on a nil pointer; the getters return nil when unset): `var base, mx time.Duration; if bo := eff.GetRetryBackOff(); bo != nil { bi := bo.GetBaseInterval(); if bi == nil { return nil, fmt.Errorf("route: %q: retry_policy: retry_back_off: base_interval must be greater than 0s", name) }; base = bi.AsDuration(); if base <= 0 { return nil, fmt.Errorf("route: %q: retry_policy: retry_back_off: base_interval must be greater than 0s", name) }; if mi := bo.GetMaxInterval(); mi != nil { mx = mi.AsDuration() } }` (base-required reject — D-S42-1; the `import durationpb "google.golang.org/protobuf/types/known/durationpb"` is already present in hcm).
  - build `rp, err := router.NewRetryPolicy(eff.GetRetryOn(), v, eff.GetRetriableStatusCodes(), base, mx)`; on a non-nil err (the `max<base` arm from `NewRetryPolicy`) wrap it as `return nil, fmt.Errorf("route: %q: retry_policy: max_interval must be greater than or equal to the base_interval", name)`.
  - `cl.EnsureRetryStats()` (D-S42-5); store `rp` on the `clusterRouteAction`.
- `clusterRouteAction.retryPolicy *router.RetryPolicy`; `asRouterAction`/`asRouterActionH2` pass `a.retryPolicy` to `router.H1ClusterAction(a.cluster, a.hashPolicies, a.subsetMatch, a.retryPolicy)` / `H2ClusterAction(...)`.
- `H1ClusterAction`/`H2ClusterAction` gain a trailing `rp *RetryPolicy` param stored on `routerAction`/`routerActionH2`. The weighted builder (`buildWeightedRouterAction`) threads the same `rp` into each entry's `H1ClusterAction`/`H2ClusterAction`.

- [ ] **Step 1: Write a failing test** (`config_test.go`): a route with `retry_policy{retry_on:"5xx", num_retries:3}` ⇒ the built `clusterRouteAction.retryPolicy != nil` with `numRetries==3`; a route with NO retry_policy but a vhost `retry_policy` ⇒ the route inherits it; a route-level retry_policy OVERRIDES the vhost; a route with neither ⇒ `retryPolicy == nil`; a `retry_back_off{base:100ms, max:50ms}` ⇒ the `max<base` reject; a `retry_back_off{base:0}` (explicitly set, base unset) ⇒ the base reject. (Use the proto-construction precedent in the existing hcm config tests.)
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the signature threading + the parse + reject + `EnsureRetryStats` call (stub `EnsureRetryStats` as a no-op until Task 5, or land Task 5 first — see ordering note). Update ALL `H{1,2}ClusterAction` call sites (router.go closure, actions.go, router_weighted.go) to pass the new `rp` arg (`nil` where none).
- [ ] **Step 4: Run → PASS** + `go build ./...` + **BYTE-STABILITY GATE** `go test ./test/differential/ -count=1` (all **76** GREEN — no fixture sets retry_policy; the closure still calls the driver directly).
- [ ] **Step 5:** gofmt/vet/lint on `internal/`.
- [ ] **Step 6: Commit** (`phase 42.1 Task 4: retry_policy parse + vhost fallback + clusterRouteAction carry + H{1,2}ClusterAction threading`).

> **Ordering note:** Task 4 references `cl.EnsureRetryStats()` (Task 5). Land Task 5 BEFORE Task 4's Step 3, or stub `EnsureRetryStats` as a no-op in Task 4 and fill it in Task 5. The subagent controller picks; the PROGRESS records which.

---

## Task 5: The +5 retry counters on `Cluster` (scoped via `EnsureRetryStats`)

**Files:** Modify `internal/cluster/cluster.go`; `internal/cluster/manager.go` (`registerClusterMetrics` :112); Test `internal/cluster/cluster_test.go` (or a manager stat test).

```go
// on Cluster (cluster.go:85 block):
statsReg    *stats.Registry
statsPrefix string
retry       *retryStats

type retryStats struct {
	rq, success, limitExceeded, backoffExp, backoffRL *stats.Counter
}

// EnsureRetryStats registers the +5 retry counters on first call (idempotent —
// config build is single-threaded). Scoped: called only for retry-policy
// clusters (a recorded departure from the reference's always-on). (ADR-0249)
func (c *Cluster) EnsureRetryStats() {
	if c.retry != nil || c.statsReg == nil {
		return
	}
	p := c.statsPrefix
	c.retry = &retryStats{
		rq:            c.statsReg.NewCounter(p + "upstream_rq_retry"),
		success:       c.statsReg.NewCounter(p + "upstream_rq_retry_success"),
		limitExceeded: c.statsReg.NewCounter(p + "upstream_rq_retry_limit_exceeded"),
		backoffExp:    c.statsReg.NewCounter(p + "upstream_rq_retry_backoff_exponential"),
		backoffRL:     c.statsReg.NewCounter(p + "upstream_rq_retry_backoff_ratelimited"), // emit-0
	}
}

func (c *Cluster) IncUpstreamRqRetry()              { if c.retry != nil { c.retry.rq.Inc() } }
func (c *Cluster) IncUpstreamRqRetrySuccess()       { if c.retry != nil { c.retry.success.Inc() } }
func (c *Cluster) IncUpstreamRqRetryLimitExceeded() { if c.retry != nil { c.retry.limitExceeded.Inc() } }
func (c *Cluster) IncUpstreamRqRetryBackoffExponential() { if c.retry != nil { c.retry.backoffExp.Inc() } }
```
In `registerClusterMetrics` (manager.go:113, right after `prefix := ...`): `c.statsReg = r; c.statsPrefix = prefix`.

- [ ] **Step 1: Write a failing test:** a Cluster after `registerClusterMetrics` + `EnsureRetryStats()` registers EXACTLY the 5 `upstream_rq_retry*` counters (registry introspection — the outlier/CB stat-test pattern); a Cluster WITHOUT `EnsureRetryStats()` registers NONE of them; calling `EnsureRetryStats()` twice registers them once (idempotent — same handles, no duplicate-registration panic).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the fields, `EnsureRetryStats`, the Inc methods, the `registerClusterMetrics` stash.
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 42.1 Task 5: +5 retry counters on Cluster via scoped EnsureRetryStats`).

---

## Task 6: The `retry_budget` activation on `circuitBreaker`

**Files:** Modify `internal/cluster/circuitbreaker.go`; `internal/cluster/cluster.go`; Test `internal/cluster/circuitbreaker_test.go`

Add to `cbPriority`: `rqRetryOpen *stats.Gauge`. Add to `circuitBreaker`: `upstreamRqRetryOverflow *stats.Counter`, `activeRetries atomic.Int64`, `budgetPercent float64`, `minRetryConcurrency int64`, `hasRetryBudget bool`. In `parseCircuitBreakers` (the `retry_budget` arm, currently only validating percent at :52): when `rb := th.GetRetryBudget(); rb != nil` AND this is the DEFAULT threshold, set `out.hasRetryBudget = true; out.budgetPercent = pct (default 20); out.minRetryConcurrency = mrc (default 3)` (`budgetPercent` = `rb.GetBudgetPercent().GetValue()` or 20 when nil; `minRetryConcurrency` = `rb.GetMinRetryConcurrency().GetValue()` or 3 when nil). In `registerStats`, STORE the DEFAULT handle + the overflow counter LIVE (currently emit-0):
```go
// in the for-loop: store DEFAULT's rq_retry_open (high stays emit-0)
g := r.NewGauge(gp + "rq_retry_open")
if idx == 0 { cb.prio[0].rqRetryOpen = g }
// after the loop, replace the emit-0 line:
cb.upstreamRqRetryOverflow = r.NewCounter(prefix + "upstream_rq_retry_overflow") // LIVE (was emit-0)
```
```go
// tryAcquireRetry reserves a retry-budget slot (DEFAULT). Returns false (overflow)
// when active retries reach max(minRetryConcurrency, ⌈budgetPercent% × activeRequests⌉).
// No-op true when no retry_budget configured. (ADR-0249, AMEND-RT3)
func (cb *circuitBreaker) tryAcquireRetry() bool {
	if !cb.hasRetryBudget {
		return true
	}
	for {
		active := cb.prio[0].activeRequests.Load()
		cap := cb.minRetryConcurrency
		if byPct := int64((cb.budgetPercent*float64(active) + 99) / 100); byPct > cap { // ceil
			cap = byPct
		}
		cur := cb.activeRetries.Load()
		if cur >= cap {
			cb.upstreamRqRetryOverflow.Inc()
			if cb.prio[0].rqRetryOpen != nil { cb.prio[0].rqRetryOpen.Set(1) }
			return false
		}
		if cb.activeRetries.CompareAndSwap(cur, cur+1) {
			return true
		}
	}
}
func (cb *circuitBreaker) releaseRetry() {
	if !cb.hasRetryBudget { return }
	if cb.activeRetries.Add(-1) < cb.minRetryConcurrency && cb.prio[0].rqRetryOpen != nil {
		cb.prio[0].rqRetryOpen.Set(0)
	}
}
```
On `Cluster` (cluster.go): `TryAcquireRetry() bool { if c.circuitBreaker == nil { return true }; return c.circuitBreaker.tryAcquireRetry() }` + `ReleaseRetry() { if c.circuitBreaker != nil { c.circuitBreaker.releaseRetry() } }`.

- [ ] **Step 1: Write failing tests:** (a) no `retry_budget` ⇒ `tryAcquireRetry()` always true; (b) `retry_budget{budget_percent:0, min_retry_concurrency:1}` (cap=1): the FIRST `tryAcquireRetry` true, the SECOND (no release) false + `upstreamRqRetryOverflow==1` + `rqRetryOpen==1`; after `releaseRetry`, the next true + `rqRetryOpen==0`; (c) the ceil formula: `budget_percent:20, min_retry_concurrency:3` with `prio[0].activeRequests=100` ⇒ cap=`max(3, ceil(20)) = 20`; (d) parse: `retry_budget` absent ⇒ `hasRetryBudget==false`; present ⇒ defaults 20/3 applied. Inject stat handles via `registerStats` or set fields.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run → PASS** + `go test ./internal/cluster/ -race -run CircuitBreaker -count=1`.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 42.1 Task 6: retry_budget activation (activeRetries + tryAcquireRetry/releaseRetry; rq_retry_open + upstream_rq_retry_overflow LIVE)`).

---

## Task 7: The `retryExecutorH1` loop + body capture/replay + the increments

**Files:** Modify `internal/filter/http/router/retry.go` (the executor) + `router.go` (`ActionResponse.localOrigin`, the `H1ClusterAction` closure, the failure-site `localOrigin: true`); Test `retry_test.go` (a fake-driver loop test).

Add `localOrigin bool` to `ActionResponse` (router.go:119) and set it `true` on the synthesized dial/connection-failure returns in `doH1ClusterAction` (the `AcquireH1` err 503 :624, the `req.Write` 502 :651, the `ReadResponse` 502 :664) — NOT on a proxied upstream response, and NOT on the CB-overflow 503 (:597, which stays `localOrigin=false`). The executor:
```go
// retryExecutorH1 wraps doH1ClusterAction with the retry loop. Each iteration is
// one driver call (which itself re-runs TryAcquireRequest + RecordUpstreamResult).
func retryExecutorH1(ctx context.Context, a *routerAction, req *http.Request) (ActionResponse, cluster.Endpoint, error) {
	rp := a.retryPolicy
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body) // already ≤1MiB in-memory (D-S42-3); over-cap is a prior 413
		_ = req.Body.Close()
	}
	var resp ActionResponse
	var ep cluster.Endpoint
	var err error
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if !a.cluster.TryAcquireRetry() { // budget overflow → no retry; counted inside
				return resp, ep, err
			}
			a.cluster.IncUpstreamRqRetry()
			a.cluster.IncUpstreamRqRetryBackoffExponential()
			time.Sleep(rp.backoff(attempt))
		}
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		resp, ep, err = doH1ClusterAction(ctx, a, req)
		if attempt > 0 {
			a.cluster.ReleaseRetry()
		}
		if !rp.matches(resp.Status, resp.localOrigin) {
			if attempt > 0 {
				a.cluster.IncUpstreamRqRetrySuccess() // retried + ended non-retriable = recovered
			}
			return resp, ep, err
		}
		if attempt >= rp.numRetries {
			a.cluster.IncUpstreamRqRetryLimitExceeded()
			return resp, ep, err
		}
	}
}
```
The `H1ClusterAction` closure: `if rp != nil { return retryExecutorH1(ctx, a, req) }; return doH1ClusterAction(ctx, a, req)`.

- [ ] **Step 1: Write a failing test** (router pkg) over a cluster wired to a controllable backend (the existing `do{H1,H2}ClusterAction` test harness, OR a small `net.Listen` echo/503 backend): (a) EXHAUSTION — a single always-503 backend + `retry_on:5xx, num_retries:3` ⇒ final 503, `upstream_rq_retry==3`, `_retry_limit_exceeded==1`, `_retry_backoff_exponential==3`, `upstream_rq_total==4`; (b) RECOVER — a backend that 503s then 200s (or a 2-host cluster) ⇒ final 200, `_retry_success==1`; (c) NON-retriable — a 404 ⇒ no retry, `upstream_rq_retry==0`; (d) BODY REPLAY — a POST with a body, the backend asserts it received the FULL body on the retry attempt; (e) BUDGET — with `retry_budget` cap 0/1 + 2 concurrent failing requests, at least one `upstream_rq_retry_overflow`. (Inject the stat handles via `EnsureRetryStats` + `registerStats`/`registerClusterMetrics`.)
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the `localOrigin` field + sites, the executor, the closure switch.
- [ ] **Step 4: Run → PASS** + `go test ./internal/filter/http/router/ -race -count=1` + **BYTE-STABILITY GATE** `go test ./test/differential/ -count=1` (76 GREEN — `retryPolicy==nil` everywhere).
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 42.1 Task 7: retryExecutorH1 loop + body replay + retry/success/limit/backoff increments`).

---

## Task 8: The `retryExecutorH2` loop + the H2 failure-site `localOrigin` + the weighted path

**Files:** Modify `internal/filter/http/router/router_h2.go` (`ActionResponse.localOrigin` at the H2 failure sites :88/:107; the `H2ClusterAction` closure; `retryExecutorH2`) + `router_weighted.go`; Test `retry_test.go`.

`retryExecutorH2` mirrors H1 but over `doH2ClusterAction(ctx, a, req h2.H2Request)`. Body replay uses the `h2.H2Request` body accessor (snapshot once, reset per attempt — mirror the H1 capture). The H2 ctx-cancel path returns `Status:0` + an `*h2.Error`: `matches(0, false)` is FALSE (0 ∉ 5xx, not localOrigin) ⇒ the executor returns it as-is (NEVER retries a client-cancel — verify by test). Set `localOrigin:true` on the H2 dial/roundtrip 502 returns (:88/:107), NOT on the ctx-cancel `Status:0` return (:103) and NOT on the CB-overflow 503 (:64).

- [ ] **Step 1: Write a failing test** (H2 driver harness, or assert the closure dispatches to the executor): (a) H2 exhaustion (always-502 upstream + `retry_on:gateway-error, num_retries:2`) ⇒ `upstream_rq_retry==2`, `_retry_limit_exceeded==1`; (b) the `Status:0`+`*h2.Error` ctx-cancel is NOT retried (`upstream_rq_retry==0`); (c) the weighted path threads `rp` (a weighted route with `retry_policy` ⇒ each entry's closure runs the executor).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `retryExecutorH2` + the H2 `localOrigin` sites + the closure switch + the weighted threading.
- [ ] **Step 4: Run → PASS** + `go test ./internal/filter/http/router/ -race -count=1` + **BYTE-STABILITY GATE** `go test ./test/differential/ -count=1` (76 GREEN).
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 42.1 Task 8: retryExecutorH2 + H2 localOrigin sites + weighted-cluster retry threading`).

---

## Task 9: The `0075-retry-loop` cross-side fixture

**Files:** Create `test/fixtures/0075-retry-loop/driver/{driver.go,driver_test.go}` + `expectations.yaml` + `README.md`

Model on `test/fixtures/0069-outlier-detection-consecutive-5xx/driver/driver.go` (the per-host-503 + RR + cross-side `StatsAsserter` precedent). Topology: **2 backends** (`BackendKindAt`: 0 ⇒ `HTTP503Responder` (35), 1 ⇒ `HTTPEcho` (1)), `BackendCount()==2`; **2 clusters** in both bootstraps: `c_exhaust` = `[backendPorts[0]]` (503 only), `c_recover` = `[backendPorts[0], backendPorts[1]]` (503+echo, ROUND_ROBIN); **2 routes**: `/exhaust → c_exhaust {retry_policy:{retry_on:"5xx", num_retries:3}}`, `/recover → c_recover {retry_policy:{retry_on:"5xx", num_retries:1}}`. The HCM listener has a fixed `stat_prefix` (single-source it). `--concurrency 1` on the reference (the harness default). Constants single-sourced (D-S42-6).

`AssertStats(t, refAdminAddr, subjAdminAddr)` — drive BOTH sides (the `0069` cross-side pattern; cross-side via `StatsAsserter`, NOT `SubjectAsserter` — `reference_differential_asserter_dispatch`):
```
For each side (addr=listener, adminAddr):
  base := scrapeStats(adminAddr)
  // EXHAUSTION: 1 request, deterministic.
  resp := GET addr/exhaust  → assert resp.StatusCode == 503
  // RECOVER: K=recoverReqs requests; all recover to 200.
  for i in 0..recoverReqs: GET addr/recover → assert 200
  fin := scrapeStats(adminAddr)
  // EXHAUSTION asserts (cross-side EXACT):
  assertDelta(c_exhaust.upstream_rq_retry, numRetries)        // ==3
  assertDelta(c_exhaust.upstream_rq_retry_limit_exceeded, 1)
  assertDelta(c_exhaust.upstream_rq_total, numRetries+1)      // ==4
  assert(c_exhaust.upstream_rq_total > 0)                     // decode-ran guard (ref)
  // RECOVER asserts:
  assertDelta(http.<stat_prefix>.downstream_rq_2xx, recoverReqs)  // CROSS-SIDE, offset-invariant
  assertDelta(c_recover.upstream_rq_retry_limit_exceeded, 0)      // CROSS-SIDE
  // subject-side only (the RR-offset finding — exact retry count not cross-side):
  if side==subject: assert(c_recover.upstream_rq_retry_success == c_recover.upstream_rq_retry && > 0)
```

- [ ] **Step 1:** Write `driver_test.go` (a `backendIdxFromBody` table test — copy the per-fixture helper, the 0066/0069 precedent) → run → FAIL.
- [ ] **Step 2:** Write `driver.go` (the helper + `Name`, `BackendCount`, `BackendKindAt`, `SubjectListenerName`, `ReferenceBootstrap`, `SubjectConfig`, `ReferenceListenerPort`, `DriveReference`/`DriveSubject`, `ProbeAdmin`, `AssertStats` + the `scrapeStats`/`assertDelta` helpers copied from 0069) + `expectations.yaml` + `README.md`. Single-source the constants (D-S42-6).
- [ ] **Step 3:** `go test ./test/fixtures/0075-retry-loop/driver/ -count=1` (the unit test) → PASS.
- [ ] **Step 4: Run the cross-side fixture** (Docker + `contrib-v1.37.2`): `go test ./test/differential/ -run 'TestDifferential/0075' -count=1 -v` (`reference_differential_run_selector` — the `TestDifferential/` prefix is REQUIRED). Expected PASS: both sides exhaust `/exhaust` to 503 with `upstream_rq_retry==3`, recover all `/recover` to downstream 200.
- [ ] **Step 5:** gofmt/vet/lint. Record fixtures **76 → 77**, stat surface → **1173** (the 2 retry clusters × +5).
- [ ] **Step 6: Commit** (`phase 42.1 Task 9: 0075 cross-side retry-loop fixture (exhaustion-exact + recover-invariant)`).

---

## Task 10: `0075` deliberate breaks + 20-run flake

**Files:** none committed (verification; SPEC §8 break protocol).

★ `-count=1` for EVERY break run (`reference_differential_break_protocol_count1`) + the `TestDifferential/0075` selector.

- [ ] **Step 1: Break (A) — never retry.** Temporarily make `(*RetryPolicy).matches` always return false. Run `go test ./test/differential/ -run 'TestDifferential/0075' -count=1` → MUST FAIL (the exhaustion arm gets `upstream_rq_retry==0` ≠ 3; the recover arm returns 503s ⇒ `downstream_rq_2xx` ≠ K). Restore.
- [ ] **Step 2: Break (B) — the retry counter not wired.** Temporarily make `IncUpstreamRqRetry` a no-op. Run → MUST FAIL (the cross-side `c_exhaust.upstream_rq_retry` delta asserts 0 ≠ 3). Restore.
- [ ] **Step 3: Confirm both restored** (`git diff` clean; the fixture PASSES).
- [ ] **Step 4: 20-run flake gate:** `for i in $(seq 20); do go test ./test/differential/ -run 'TestDifferential/0075' -count=1 || echo "FAIL $i"; done` → 20/20 PASS (backoff is delay-only + the counts are deterministic; if any flake, it is the unrelated startup race `reference_differential_fullsuite_startup_flake`, NOT a count issue — isolate-re-run; NEVER add a fixed sleep).
- [ ] **Step 5:** Record the break + flake results in PROGRESS. (No commit.)

---

## Task 11: Full 77-dir differential + six-gate

**Files:** none (verification); update PROGRESS.

- [ ] **Step 1: Six-gate** (ADR-0052): `go build ./...`; `go vet ./...`; `gofmt -l internal/ test/` (empty); `golangci-lint run ./...`; `go test ./internal/... -count=1`; `go test ./test/differential/ -count=1` (ALL **77** GREEN). Note the unrelated `subject ready: EOF` startup flake (`reference_differential_fullsuite_startup_flake`) — isolate-re-run the named dir + a full re-run to distinguish from a regression.
- [ ] **Step 2: Counts → stat surface 1173; fixtures 77; fuzzers 42; BackendKind tail 36.** Record in PROGRESS.
- [ ] **Step 3:** If any gate fails, fix + re-run before proceeding.

---

## Task 12: ADR-0249 body + BEHAVIOR_CONTRACT delta

**Files:** Modify `docs/envoy-go/DECISIONS.md` (ADR-0249 full entry — §Decision + §Consequences; the §Context is drafted in SPEC §13 — promote/refine it; DECISIONS tail ADR-0248 → **ADR-0249**, next-free ADR-0250); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the new `### Route — request recovery (retries)` subsection, SPEC §9; advance the stat-surface block **1163 → 1173**).

- [ ] **Step 1:** Write the ADR-0249 body. §Decision: the `retryExecutor` wrapping `doH{1,2}ClusterAction`; the `retry_on` bitset + the `localOrigin` connect-failure signal; the buffered-body replay; the exponential full-jitter backoff; the per-attempt CB-admission/outcome re-run; the `retry_budget` activation (`max(min_retry_concurrency, budget_percent% × active)`; overflow→`upstream_rq_retry_overflow`+`rq_retry_open`, distinct from static-cap `upstream_rq_retry_limit_exceeded`); the +5 scoped retry counters + the 2 phase-41 stats flipping LIVE; the enforced-subset / parse-accept-rest `retry_on` posture. §Consequences: byte-stable when no `retry_policy`; the over-cap body is non-retriable (the HCM 413 — D-S42-3); the `RX`/`URX` access-log flags DEFERRED (no response-flags plumbing — AMEND-RT7); the deferred tokens/headers/per_try_timeout (→ 42.2/future); the single ADR-0249 absorbs the anticipated ADR-0250.
- [ ] **Step 2:** Write the BEHAVIOR_CONTRACT subsection + the stat-count bump 1163 → 1173.
- [ ] **Step 3:** `go build ./...` (docs-only sanity).
- [ ] **Step 4: Commit** (`phase 42.1 Task 12: ADR-0249 body + BEHAVIOR_CONTRACT retries subsection (stat 1163→1173)`).

---

## Task 13: Completion bundle

**Files:** Modify `docs/envoy-go/phases/42-retries-and-hedging/PROGRESS.md` (final state + exit-delta table); CREATE `docs/envoy-go/phases/42-retries-and-hedging/README.md` (status PLAN-done → IMPL-done); `docs/envoy-go/STATE.md` (active-phase → `phase 42.1 (retries-and-hedging) IMPL done`; counts → 1173 / 77 / 42 / 36 / ADR-0249); `docs/envoy-go/ROADMAP.md` (row 42 STAYS `in-progress` — 42.2 hedging is the remaining leg; append the 42.1 IMPL note); `next-prompt.txt` (roll forward to the 42.2 hedging BRAINSTORM).

- [ ] **Step 1:** Update PROGRESS (the 13-task record + the six-gate evidence + the break/flake results + the exit-delta table).
- [ ] **Step 2:** Write the phase README; update STATE/ROADMAP/next-prompt. ROADMAP row 42 STAYS `in-progress` (the split row flips `done` only when BOTH 42.1+42.2 land — ADR-0106 + `reference_roadmap_split_phase_row_done`); the 42.1 IMPL note records the leg landed.
- [ ] **Step 3: Final six-gate re-confirm** + record all exit counts.
- [ ] **Step 4: Commit** (`phase 42.1 Task 13: completion bundle — retries (retry loop) landed; row 42 stays in-progress for 42.2`).
- [ ] **Step 5:** The controller squashes the 13 task commits + pushes to origin/master (`feedback_subagents_no_push` — subagents commit locally only; the controller squashes at stage-close + pushes per `feedback_push_to_origin`).

---

## Exit deltas (SPEC §14)

| Axis | At PLAN | At 42.1 IMPL |
|------|---------|-------------|
| stat surface | 1163 | **1173** (+5 per retry cluster × 2 in `0075`) |
| differential fixtures | 76 | **77** (`0075`) |
| fuzzers | 42 | 42 (unchanged) |
| BackendKind tail | 36 | 36 (REUSE `HTTP503Responder` 35 + `HTTPEcho` 1) |
| DECISIONS tail | ADR-0248 | **ADR-0249** (next-free ADR-0250) |
| phase-41 stats flipped LIVE | emit-0 | `rq_retry_open` + `upstream_rq_retry_overflow` (no surface delta) |
| new packages / go.mod modules | — | ZERO / ZERO |
| ROADMAP row 42 | in-progress | **in-progress** (42.2 hedging remains; flips `done` only when BOTH legs land) |

Next → the phase-42.1 IMPL (`superpowers:subagent-driven-development` — fresh subagent per task + two-stage review).
