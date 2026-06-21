# Phase 42.2b (hedging) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `RouteAction.hedge_policy` (`hedge_on_per_try_timeout` + the `initial_requests`/`additional_request_chance` fan-out) — the project's FIRST concurrent-upstream-attempt control plane: on a per-try-timeout, leave the slow attempt in flight and race a hedge, returning the first attempt to complete acceptably.

**Architecture:** A SEPARATE concurrent `hedgeExecutorH1`/`hedgeExecutorH2` in a new `internal/filter/http/router/hedge.go`, dispatched via the `H{1,2}ClusterAction` closure switch BEFORE the byte-stable `if a.rp != nil` retry branch — ONLY when the route's `hedge_policy.triggersConcurrency()`. Each in-flight attempt is a goroutine (REUSING `do{H1,H2}ClusterAction` + `matches` + the body replay + the CB-admission/outcome/`retry_budget` seams) with a per-attempt HEDGE-TRIGGER timer (NOT the 42.2a cancelling deadline): on `per_try_timeout` it LAUNCHES a budget-counted hedge (`upstream_rq_retry`++, NEVER `upstream_rq_per_try_timeout`) and leaves the original running; a buffered first-acceptable-wins collector returns the first non-retriable result and cancels/drains losers. Byte-identical when no `hedge_policy`.

**Tech Stack:** Go; `route.v3.HedgePolicy` (go-control-plane v1.32.4 — ZERO new module); the live `internal/filter/http/router` retry substrate (ADR-0249/0250); `internal/filter/hcm` parse; the differential harness (`BlockingHoldResponder` BackendKind 36, the `0074` concurrent-poll-`/__release` driver model).

---

## Source-of-truth references

- **SPEC:** `docs/envoy-go/phases/42.2b-hedging/SPEC.md` — §1.1 AMEND-H1..7 (the live SURPRISES), §3 the architecture, §11 the D-H-* pin table, §12 the D-S422B-1..9 questions THIS plan resolves.
- **The as-built substrate (REUSE, do NOT modify the non-hedge path):** `internal/filter/http/router/retry.go` (the 42.1 `retryExecutorH1`/`retryExecutorH2` + the 42.2a per-attempt unit — the `hedgeExecutor` is a SIBLING, it does NOT touch these), `router.go` (the `H1ClusterAction` constructor `:565` + the closure switch `:578` + `doH1ClusterAction` `:603` + the `routerAction` struct `:753`), `router_h2.go` (the `H2ClusterAction` `:39` + switch `:47` + `doH2ClusterAction` `:65` + the `routerActionH2` struct `:246`), `hcm/config.go` (`buildRouterActionWithVH` `:557` + `parseRetryPolicy` `:615`), `hcm/actions.go` (`clusterRouteAction` `:201` + `do` `:214` + `asRouterAction` `:237` + `asRouterActionH2` `:250`), `cluster.go` (`retryStats` `:156` + `EnsureRetryStats` `:254` + the `Inc*` methods `:271-304`).
- **The fixture template:** `test/fixtures/0076-per-try-timeout/` (the cross-side STATIC/STRICT_DNS shape) + `test/fixtures/0074-circuit-breaker-max-requests/driver/driver.go` (the concurrent-fire + poll-`/stats` + `/__release` driver model — `0077` is a HYBRID of these two). Register in `test/differential/runner_test.go` (mirror the `0076` import at `:103`).

## The load-bearing live findings (from SPEC §1.1 — the IMPL MUST honor these)

1. **AMEND-H1:** a hedged per-try-timeout increments `upstream_rq_retry`, **NEVER** `upstream_rq_per_try_timeout` (which STAYS 0 under hedge mode). The `hedgeExecutor` MUST NOT call `IncUpstreamRqPerTryTimeout`.
2. **AMEND-H2:** the in-flight original WINS (200) even after `num_retries` is exhausted; `upstream_rq_retry_limit_exceeded`=1 means "no more hedges can launch", NOT "final failure" — the request awaits the first acceptable in-flight result.
3. **AMEND-H3:** losers are cancelled + uncounted (`upstream_rq_total=N+1`, `upstream_rq_200=1`). The hedge path NEVER uses the 42.2a cancelling-deadline + race-robust discriminator (so the BRAINSTORM's D-H-RACE hazard cannot occur).
4. **AMEND-H5:** NO new stat — surface STAYS **1181** (a hedge reuses `upstream_rq_retry` + `upstream_rq_retry_backoff_exponential`).
5. **The H1/H2 driver-cancel asymmetry (D-S422B-2):** `doH1ClusterAction` honors only `ctx.Deadline()` (NOT `ctx.Done()` — `router.go:659-661`), so an in-flight H1 LOSER does NOT abort on a pure ctx-cancel; it completes when the upstream responds (the `0077` `/__release` guarantees this). `doH2ClusterAction`'s `cc.RoundTrip(ctx, req)` DOES honor ctx-cancel. The collector MUST use a BUFFERED result channel (no send-block leak) and treat H1 loser-reaping as backend-response-driven (the fixture path) — actively-resetting an H1 loser's socket is a recorded departure (Task 12; a future enhancement).

## D-question resolutions (SPEC §12)

- **D-S422B-1** (placement): a NEW `internal/filter/http/router/hedge.go` (the `hedgeExecutorH1`/`hedgeExecutorH2` + the collector + the `HedgePolicy` type). This OVERRIDES the SPEC §12 D-S422B-1 lean (which leaned toward an extracted shared `runAttempt` helper): REUSE `do{H1,H2}ClusterAction` + `matches` + the body-replay idiom DIRECTLY (same package — NO extracted shared helper; the hedge path's timer+collector shape is different enough that an extracted `runAttempt` would be uglier than the ~25 LoC of intentional per-executor body-replay duplication — the 42.1 H1/H2-separate precedent; the SPEC explicitly delegated the call to the PLAN).
- **D-S422B-2** (collector): a BUFFERED result channel sized `1 + maxHedges` (so no goroutine blocks on send → no leak); the collector `select`s the first `!matches` result, returns it, cancels `hedgeCtx` (the H2 losers abort; the H1 losers complete on backend-response); the drain is implicit (the buffered channel + the cancelled ctx). `-race`-gated. The H1-active-reset gap is a recorded departure (above).
- **D-S422B-3** (acquire-gates-spawn): `TryAcquireRetry()` is called BEFORE the hedge goroutine is spawned; on deny, no spawn + `upstream_rq_retry_overflow` (bumped inside `TryAcquireRetry`). The budget+`num_retries` thus cap the live goroutine count.
- **D-S422B-5** (fan-out verification): the fan-out (`initial_requests>1`) is UNIT-tested in the router package (subject-side; the reference no-ops it). NO `0078` differential dir. Fixtures land at **79** (`0077` only).
- **D-S422B-6** (hedge backoff): a hedge fires IMMEDIATELY on the per-try-timeout (NO backoff sleep — the live timing showed hedges at exact T intervals); it increments `upstream_rq_retry` + `upstream_rq_retry_backoff_exponential` (reference parity) but does NOT `time.Sleep`.
- **D-S422B-9** (`retry_success`): a hedge-win-vs-original-win `upstream_rq_retry_success` distinction is UNPROBED + NONDETERMINISTIC in `0077` (the released held attempts race) → NOT asserted by `0077`. The candidate rule (a NON-original winner increments it) is UNIT-tested with a deterministic two-host harness; an IMPL re-probe (Task 9 prep) confirms the reference rule before the unit test asserts it.
- **D-S422B-7** (`0077` constants): `refContainerListenerPort = 19166` (next-free after `0076`'s 19165); `perTryTimeoutMs = 250`; `numHedges (num_retries) = 3`; H1-only cross-side (the reference downstream is H1) with the H2 hedge path unit-tested.
- **D-S422B-4 / D-S422B-8**: NO new fuzzer (config-parse, unit-tested; fuzzers STAY 42); the FINAL ADR-0045 re-check ⇒ NO split (~280–420 LoC / 13 tasks — both axes under the gate).

---

## Task 1: Baselines + PROGRESS scaffold

**Files:**
- Create: `docs/envoy-go/phases/42.2b-hedging/PROGRESS.md`

- [ ] **Step 1: Capture the GREEN baselines** (from the worktree root). Run + record the outputs:

```bash
go build ./... && go vet ./...
gofmt -l internal/ test/ | tee /tmp/gofmt-base.txt   # expect empty
go test ./internal/filter/http/router/... ./internal/filter/hcm/... ./internal/cluster/... 2>&1 | tail -20
```
Expected: build/vet clean, gofmt empty, all unit tests PASS.

- [ ] **Step 2: Record the count baselines in PROGRESS.md.** stat surface **1181** / fixtures **78** / fuzzers **42** / BackendKind tail **36** / DECISIONS tail **ADR-0250** (next-free **ADR-0251**). Anticipated at IMPL-done: fixtures **79** (`0077`), stat surface **1181 UNCHANGED** (AMEND-H5), DECISIONS **ADR-0251**, fuzzers 42, BackendKind 36, ZERO new packages/modules. List the 13 tasks with checkboxes.

- [ ] **Step 3: Commit**
```bash
git add docs/envoy-go/phases/42.2b-hedging/PROGRESS.md
git commit -m "phase 42.2b Task 1: baselines + PROGRESS scaffold"
```

---

## Task 2: The `HedgePolicy` parsed type + `triggersConcurrency()` + the `initial_requests >= 1` reject

**Files:**
- Create: `internal/filter/http/router/hedge.go`
- Create: `internal/filter/http/router/hedge_test.go`

- [ ] **Step 1: Write the failing test** (`hedge_test.go`):

```go
package router

import "testing"

func TestHedgePolicy_TriggersConcurrency(t *testing.T) {
	cases := []struct {
		name string
		hp   *HedgePolicy
		want bool
	}{
		{"nil", nil, false},
		{"default", &HedgePolicy{InitialRequests: 1}, false},
		{"hedge_on_ptt", &HedgePolicy{InitialRequests: 1, HedgeOnPerTryTimeout: true}, true},
		{"fanout", &HedgePolicy{InitialRequests: 3}, true},
		{"chance", &HedgePolicy{InitialRequests: 1, AdditionalRequestChanceNum: 50, AdditionalRequestChanceDen: 100}, true},
	}
	for _, c := range cases {
		if got := c.hp.triggersConcurrency(); got != c.want {
			t.Errorf("%s: triggersConcurrency()=%v want %v", c.name, got, c.want)
		}
	}
}

func TestNewHedgePolicy_InitialRequestsReject(t *testing.T) {
	if _, err := NewHedgePolicy(0, false, 0, 0); err == nil {
		t.Fatal("initial_requests:0 must reject")
	}
	if _, err := NewHedgePolicy(1, true, 0, 0); err != nil {
		t.Fatalf("initial_requests:1 must accept: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/filter/http/router/ -run TestHedgePolicy -v` → FAIL (undefined `HedgePolicy`).

- [ ] **Step 3: Implement** (`hedge.go`). `additional_request_chance` is stored as a parsed numerator/denominator (the `FractionalPercent` lowered — a deterministic test seam per D-S422B-6/9):

```go
package router

import "errors"

// ErrMsgInitialRequestsBelowOne is the hedge_policy initial_requests reject
// suffix. The hcm parse layer re-emits it with a "route: %q: hedge_policy: "
// prefix; keep both sites referencing this const so they cannot drift.
const ErrMsgInitialRequestsBelowOne = "initial_requests must be greater than or equal to 1"

// HedgePolicy is the parsed, enforced hedging configuration for a route
// (route.v3.HedgePolicy). Only HedgeOnPerTryTimeout is reference-wired; the
// InitialRequests/AdditionalRequestChance fan-out is a richer-than-reference,
// SUBJECT-SIDE-verified departure (SPEC AMEND-H4).
type HedgePolicy struct {
	InitialRequests            uint32 // default 1; PGV gte:1
	HedgeOnPerTryTimeout       bool
	AdditionalRequestChanceNum uint32 // FractionalPercent numerator (0 ⇒ no extra request)
	AdditionalRequestChanceDen uint32 // FractionalPercent denominator (HUNDRED=100, etc.)
}

// NewHedgePolicy validates + builds. initial_requests<1 rejects (the gte:1 PGV).
func NewHedgePolicy(initialRequests uint32, hedgeOnPerTryTimeout bool, chanceNum, chanceDen uint32) (*HedgePolicy, error) {
	if initialRequests < 1 {
		return nil, errors.New(ErrMsgInitialRequestsBelowOne)
	}
	return &HedgePolicy{
		InitialRequests:            initialRequests,
		HedgeOnPerTryTimeout:       hedgeOnPerTryTimeout,
		AdditionalRequestChanceNum: chanceNum,
		AdditionalRequestChanceDen: chanceDen,
	}, nil
}

// triggersConcurrency reports whether this policy launches more than the single
// primary attempt (so the concurrent hedgeExecutor is dispatched). A nil policy
// or a default (initial_requests:1, no chance, no hedge_on_per_try_timeout) ⇒
// false ⇒ the byte-stable 42.1/42.2a path.
func (hp *HedgePolicy) triggersConcurrency() bool {
	if hp == nil {
		return false
	}
	return hp.InitialRequests > 1 || hp.HedgeOnPerTryTimeout || hp.AdditionalRequestChanceNum > 0
}
```

- [ ] **Step 4: Run to verify it passes** — `go test ./internal/filter/http/router/ -run 'TestHedgePolicy|TestNewHedgePolicy' -v` → PASS.

- [ ] **Step 5: Commit** — `git add internal/filter/http/router/hedge.go internal/filter/http/router/hedge_test.go && git commit -m "phase 42.2b Task 2: HedgePolicy type + triggersConcurrency + initial_requests>=1 reject"`

---

## Task 3: `hedgeExecutorH1` skeleton — the `hp` struct field + goroutine + buffered collector + loser cancel (no timer yet)

**Files:**
- Modify: `internal/filter/http/router/router.go` (`routerAction` struct `:753` — add the `hp` field), `internal/filter/http/router/router_h2.go` (`routerActionH2` struct `:246` — add the `hp` field)
- Modify: `internal/filter/http/router/hedge.go`, `internal/filter/http/router/hedge_test.go`

The skeleton launches the PRIMARY attempt as a goroutine feeding a buffered channel + a first-acceptable collector. With `InitialRequests:1` + no hedge trigger fired, it behaves like one driver call (this task does NOT add the per-try timer or the fan-out — those are Tasks 4/6). The executor is package-private + unreferenced until Task 8 wires the dispatch — BUT Go compiles its body regardless, so the `hp` struct field it reads (`a.hp`) MUST be added in THIS task (the field stays nil/unset until Task 8's constructor passes it — harmless, the executor is undispatched). The body uses `io`/`bytes` ⇒ those imports + the `readAndCloseBody` helper are added in this same task/commit, or it will not build.

- [ ] **Step 1: Add the `hp *HedgePolicy` field to BOTH structs** (so `hedgeExecutorH1`/`H2` compile). In `routerAction` (router.go:753) and `routerActionH2` (router_h2.go:246), append:
```go
	hp *HedgePolicy // 42.2b: effective hedge_policy; nil when none. The concurrent hedgeExecutor dispatches on it in Task 8 (closure switch). Set by the widened H{1,2}ClusterAction constructor (Task 8); nil/unset until then.
```

- [ ] **Step 2: Write the failing test** — a primary attempt over a fake driver seam returns the driver's result; a `-race`-clean single-attempt collect:

```go
func TestHedgeExecutorH1_PrimaryOnly(t *testing.T) {
	// hp.InitialRequests==1, no hedge_on_per_try_timeout ⇒ exactly one attempt,
	// returns its result. (Driven through a test routerAction whose cluster +
	// driver path is the existing in-package test harness used by retry_test.go.)
	// Assert: the collector returns the single attempt's ActionResponse, 1 driver call.
}
```
(Model the harness on the existing `retry_test.go` `routerAction`/fake-cluster setup — REUSE it; do NOT invent a new mock.)

- [ ] **Step 3: Run to verify it fails** — `go test ./internal/filter/http/router/ -run TestHedgeExecutorH1 -v` → FAIL (undefined `hedgeExecutorH1`).

- [ ] **Step 4: Implement the skeleton** (`hedge.go`). The import block MUST include `io` + `bytes` (the body replay) — add them now (they are USED in this commit, so no unused-import error). Define `readAndCloseBody` in this same commit (it does NOT yet exist in the package — verified). The buffered channel uses ONE canonical capacity formula `cap := int(hp.InitialRequests) + maxHedges + 1` (I2 — used identically in Tasks 3/4/6; here `maxHedges := 0`, Task 4 sets it from `num_retries`; the formula provably ≥ total goroutines ever launched, so no loser ever blocks on send ⇒ no leak):

```go
import (
	"bytes"
	"context"
	"io"
	"net/http"
	"github.com/esalaine/envoy-go/internal/cluster"
)

// readAndCloseBody buffers + closes a request body (the ≤1MiB ADR-0076 buffer;
// the HCM 413s an over-cap body before the router, so io.ReadAll is safe). The
// replay source for every concurrent attempt. Mirrors retryExecutorH1's idiom.
func readAndCloseBody(b io.ReadCloser) ([]byte, error) {
	defer b.Close()
	return io.ReadAll(b)
}

// attemptResult carries one in-flight attempt's outcome to the collector.
type attemptResult struct {
	resp ActionResponse
	ep   cluster.Endpoint
	err  error
}

// hedgeExecutorH1 runs the concurrent first-acceptable-wins racer for H1 when
// the route carries a triggering hedge_policy. It REUSES doH1ClusterAction (the
// single-attempt driver) per goroutine. The result channel is BUFFERED to
// cap=int(hp.InitialRequests)+maxHedges+1 so no loser goroutine ever blocks on
// send (no leak; the H1 driver honors only ctx.Deadline, not ctx.Done — SPEC
// D-S422B-2). On the first !matches (acceptable) result the collector returns it
// and cancels hedgeCtx (H2 losers abort; H1 losers complete on backend-response).
func hedgeExecutorH1(ctx context.Context, a *routerAction, req *http.Request) (ActionResponse, cluster.Endpoint, error) {
	hp := a.hp
	rp := a.rp // may be nil (a pure-fanout hedge with no retry_policy); guard num_retries below
	maxHedges := 0 // Task 4 sets this to rp.numRetries when hedge_on_per_try_timeout
	initial := int(hp.InitialRequests)

	var body []byte
	if req.Body != nil {
		body, _ = readAndCloseBody(req.Body)
	}

	hedgeCtx, cancelAll := context.WithCancel(ctx)
	defer cancelAll() // reap all in-flight on return (H2 aborts; H1 completes on response)

	results := make(chan attemptResult, initial+maxHedges+1) // the canonical formula (I2)
	launch := func() {
		go func() {
			r := req.Clone(hedgeCtx)
			if body != nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			resp, ep, err := doH1ClusterAction(hedgeCtx, a, r)
			results <- attemptResult{resp, ep, err}
		}()
	}

	inFlight := 0
	for i := 0; i < initial; i++ { // Task 6 makes (initial-1) of these budget-counted
		launch()
		inFlight++
	}

	var last attemptResult
	for inFlight > 0 {
		ar := <-results
		inFlight--
		last = ar
		if rp == nil || !rp.matches(ar.resp.Status, ar.resp.localOrigin) {
			return ar.resp, ar.ep, ar.err // first acceptable wins
		}
	}
	return last.resp, last.ep, last.err // all retriable ⇒ the last
}
```

- [ ] **Step 5: Run to verify it passes** — `go test ./internal/filter/http/router/ -run TestHedgeExecutorH1 -race -v` → PASS.

- [ ] **Step 6: Commit** — `git add -A internal/filter/http/router/ && git commit -m "phase 42.2b Task 3: hedgeExecutorH1 skeleton — the hp struct field + goroutine + buffered first-acceptable collector"`

---

## Task 4: The per-attempt HEDGE-TRIGGER timer + `hedge_on_per_try_timeout` (the AMEND-H1/H2 core)

**Files:**
- Modify: `internal/filter/http/router/hedge.go`
- Modify: `internal/filter/http/router/hedge_test.go`

This is the load-bearing task. On `hedge_on_per_try_timeout`, each in-flight attempt arms a timer at `rp.perTryTimeout`; on firing (and if a hedge slot remains), it LAUNCHES one hedge (`IncUpstreamRqRetry` + `IncUpstreamRqRetryBackoffExponential`, NO `time.Sleep`, **NO `IncUpstreamRqPerTryTimeout`** — AMEND-H1) and leaves the original running. The cap is `num_retries` hedges (`IncUpstreamRqRetryLimitExceeded` ONCE when a timer fires with no slot left, then the request AWAITS the in-flight attempts — AMEND-H2).

- [ ] **Step 1: Write the failing tests** (deterministic, no Docker — a fake driver whose first attempt BLOCKS until a release signal, mirroring the held backend):

```go
func TestHedgeExecutorH1_HedgeOnPerTryTimeout_OriginalWins(t *testing.T) {
	// hp.HedgeOnPerTryTimeout=true, perTryTimeout=50ms, num_retries=3.
	// Driver: each attempt blocks ~150ms then returns 200.
	// Expect: final 200; IncUpstreamRqRetry called 3x; IncUpstreamRqPerTryTimeout
	// called 0x (AMEND-H1); the in-flight original (or first to finish) wins.
}
func TestHedgeExecutorH1_HedgeOnPerTryTimeout_NeverIncrementsPerTryTimeout(t *testing.T) {
	// Drive the all-slow path; assert the cluster's IncUpstreamRqPerTryTimeout
	// counter delta == 0 over the whole hedge run (the load-bearing AMEND-H1).
}
func TestHedgeExecutorH1_HedgeOnPerTryTimeout_LimitExceeded(t *testing.T) {
	// perTryTimeout small + a backend that responds only AFTER all hedges launch;
	// assert IncUpstreamRqRetryLimitExceeded == 1 AND the request still returns 200
	// (AMEND-H2: limit_exceeded != final failure).
}
```
Use a fake cluster recording `Inc*` call counts (extend the `retry_test.go` fake if present, else a minimal stat-recording cluster).

- [ ] **Step 2: Run to verify they fail** → FAIL.

- [ ] **Step 3: Implement** the hedge-trigger timer in `hedgeExecutorH1`. THE CONCURRENCY RULE (I1, the load-bearing design): a `time.AfterFunc` callback runs on its OWN goroutine, so it MUST NOT mutate shared state — it does ONLY a non-blocking channel send. The COLLECTOR LOOP is the SINGLE mutator of `hedgesLaunched`/`limitExceededOnce` and the SINGLE caller of `Inc*`/`TryAcquireRetry`/`launch`. The collector `select`s on BOTH `results` and a `hedgeReq` channel:

```go
maxHedges = 0
if hp.HedgeOnPerTryTimeout && rp != nil {
	maxHedges = rp.numRetries
}
results := make(chan attemptResult, initial+maxHedges+1) // the SAME canonical formula (I2)
hedgeReq := make(chan struct{}, initial+maxHedges+1)     // timer firings funnel here (buffered, never blocks)

// launch starts ONE attempt goroutine. consumedBudget marks a budget-counted
// hedge (NOT the primary) so the goroutine releases its slot on settle (M2).
launch := func(consumedBudget bool) {
	go func() {
		r := req.Clone(hedgeCtx)
		if body != nil { r.Body = io.NopCloser(bytes.NewReader(body)) }
		if hp.HedgeOnPerTryTimeout && rp.perTryTimeout > 0 {
			// arm a per-attempt hedge-trigger; the callback ONLY signals the
			// collector (no shared-state mutation — I1). Stopped on settle.
			t := time.AfterFunc(rp.perTryTimeout, func() {
				select {
				case hedgeReq <- struct{}{}:
				case <-hedgeCtx.Done():
				}
			})
			defer t.Stop()
		}
		resp, ep, err := doH1ClusterAction(hedgeCtx, a, r)
		if consumedBudget { a.cluster.ReleaseRetry() } // release the slot this attempt held (M2)
		results <- attemptResult{resp, ep, err}
	}()
}

hedgesLaunched := 0
limitExceededOnce := false
inFlight := 0
for i := 0; i < initial; i++ { // primary + (initial-1) budget-counted (Task 6); here initial==1
	launch(false)
	inFlight++
}

var last attemptResult
for inFlight > 0 {
	select {
	case ar := <-results:
		inFlight--
		last = ar
		if rp == nil || !rp.matches(ar.resp.Status, ar.resp.localOrigin) {
			return ar.resp, ar.ep, ar.err // first acceptable wins; defer cancelAll reaps losers
		}
	case <-hedgeReq: // a per-try-timeout fired ⇒ launch a hedge (collector is the sole mutator)
		if hedgesLaunched >= maxHedges {
			if !limitExceededOnce { a.cluster.IncUpstreamRqRetryLimitExceeded(); limitExceededOnce = true } // AMEND-H2: "no more hedges", NOT final failure
			continue
		}
		if !a.cluster.TryAcquireRetry() { continue } // budget overflow counted inside (D-S422B-3)
		hedgesLaunched++
		a.cluster.IncUpstreamRqRetry()
		a.cluster.IncUpstreamRqRetryBackoffExponential() // NO time.Sleep (D-S422B-6); NEVER IncUpstreamRqPerTryTimeout (AMEND-H1)
		launch(true)
		inFlight++
	}
}
return last.resp, last.ep, last.err
```

This REPLACES the Task-3 plain `for inFlight>0 { <-results }` loop with the `select`-on-both form. NOTE the `import "time"` addition. The `defer cancelAll()` (Task 3) reaps in-flight losers on return; an H1 loser completes on backend-response (D-S422B-2), an H2 loser aborts on the ctx-cancel (Task 5). `hedgeReq` is buffered to the same capacity so a timer send never blocks even if the collector has already returned.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/filter/http/router/ -run TestHedgeExecutorH1_HedgeOnPerTryTimeout -race -count=1 -v` → PASS.

- [ ] **Step 5: Commit** — `git commit -am "phase 42.2b Task 4: hedge_on_per_try_timeout — the hedge-trigger timer; retry++ not per_try_timeout (AMEND-H1/H2)"`

---

## Task 5: `hedgeExecutorH2` (mirror — the H2 `Status:0`/`localOrigin` + ctx-cancel-aware path)

**Files:**
- Modify: `internal/filter/http/router/hedge.go`, `internal/filter/http/router/hedge_test.go`

`hedgeExecutorH2` MIRRORS `hedgeExecutorH1`'s collector + timer, differing only in: the driver (`doH2ClusterAction` over `h2.H2Request`); the body replay (the `H2Request` body is a buffered `[]byte` field — pass `req` by value per attempt, no `NopCloser`); the `matches` guard (an H2 ctx-cancel returns `Status:0` — `matches(0,false)==false` ⇒ NOT acceptable ⇒ treated as a retriable/cancelled loser, which is correct since the collector cancels losers). H2 `RoundTrip(ctx)` HONORS ctx-cancel, so H2 losers abort promptly (unlike H1 — the asymmetry note).

- [ ] **Step 1: Write the failing tests** — mirror the Task-4 H1 tests for H2; PLUS `TestHedgeExecutorH2_ClientCancelNotMiscounted` (a parent-ctx cancel before any attempt completes returns the `Status:0` loser without an acceptable winner + does NOT increment `upstream_rq_per_try_timeout`).
- [ ] **Step 2: Run to verify they fail** → FAIL.
- [ ] **Step 3: Implement `hedgeExecutorH2`** (the H1 shape with the H2 driver/body/value-copy).
- [ ] **Step 4: Run to verify they pass** — `-race -count=1` → PASS.
- [ ] **Step 5: Commit** — `git commit -am "phase 42.2b Task 5: hedgeExecutorH2 (mirror; H2 ctx-cancel-aware)"`

---

## Task 6: The BUDGET-COUNTED fan-out (`initial_requests` + `additional_request_chance`)

**Files:**
- Modify: `internal/filter/http/router/hedge.go`, `internal/filter/http/router/hedge_test.go`

`initial_requests:N` fires N concurrent attempts up front: 1 PRIMARY (NOT a retry) + (N−1) budget-counted hedges (each `TryAcquireRetry` BEFORE spawn — acquire-gates-spawn, D-S422B-3 — + `IncUpstreamRqRetry` + `IncUpstreamRqRetryBackoffExponential`, capped by `num_retries`/budget). `additional_request_chance` adds ONE more budget-counted attempt iff a deterministic draw succeeds. SUBJECT-SIDE only (the reference no-ops it — AMEND-H4).

- [ ] **Step 1: Write the failing tests:**
```go
func TestHedgeExecutorH1_Fanout_BudgetCounted(t *testing.T) {
	// hp.InitialRequests=3, num_retries=5, ample budget.
	// Expect: 3 concurrent attempts (1 primary + 2 budget-counted) ⇒ IncUpstreamRqRetry==2;
	// first acceptable wins; total driver calls == 3.
}
func TestHedgeExecutorH1_Fanout_AcquireGatesSpawn(t *testing.T) {
	// hp.InitialRequests=5 but a budget that grants only 1 retry ⇒ at most 2 goroutines
	// spawned (1 primary + 1 granted); the overflow does NOT spawn (D-S422B-3).
}
func TestAdditionalRequestChance_Deterministic(t *testing.T) {
	// chanceNum/Den at 0/100 ⇒ never; 100/100 ⇒ always (the unit boundaries).
}
```
- [ ] **Step 2: Run to verify they fail** → FAIL.
- [ ] **Step 3: Implement** — the up-front fan-out loop (1 primary unconditionally; each additional `TryAcquireRetry`-gated spawn) + a `drawAdditional(num, den, seq)` helper (deterministic — seed/seq passed in for testability, NOT `math/rand` at call time). Apply to BOTH executors.
- [ ] **Step 4: Run to verify they pass** — `-race -count=1` → PASS.
- [ ] **Step 5: Commit** — `git commit -am "phase 42.2b Task 6: budget-counted fan-out (initial_requests + additional_request_chance), acquire-gates-spawn"`

---

## Task 7: The `hedge_policy` parse + the vhost fallback + the `clusterRouteAction` carry

**Files:**
- Modify: `internal/filter/hcm/config.go` (`parseHedgePolicy` new + a `fractionalDenominator` helper new; `buildRouteTable` decl `:412` AND its caller `:267`; `buildAction` `:513`; `buildRouterActionWithVH` `:557`; `buildWeightedRouterAction`)
- Modify: `internal/filter/hcm/actions.go` (`clusterRouteAction` `:201` — add the `hedgePolicy` field + the constructor sites set it — but NOT yet the `H{1,2}ClusterAction` signature; that is Task 8)
- Modify: `internal/filter/hcm/config_test.go`

- [ ] **Step 1: Write the failing tests** (`config_test.go`): a route with `hedge_policy{hedge_on_per_try_timeout:true}` parses onto `clusterRouteAction.hedgePolicy`; the `VirtualHost.hedge_policy` fallback applies when the route has none; `initial_requests:0` ⇒ a `route: %q: hedge_policy: initial_requests must be greater than or equal to 1` reject; a route with NEITHER ⇒ `hedgePolicy` nil (byte-stable).

- [ ] **Step 2: Run to verify they fail** → FAIL.

- [ ] **Step 3: Implement.** Thread `vhHedgePolicy *routev3.HedgePolicy` through the call chain — BOTH the `buildRouteTable` decl (`:412`) AND its sole caller (`config.go:267`, today `buildRouteTable(vh.GetRoutes(), clusters, vh.GetRetryPolicy())` → add `, vh.GetHedgePolicy()`; `VirtualHost.GetHedgePolicy()` is a real field-17 accessor) → `buildAction(..., vhRetryPolicy, vhHedgePolicy)` (`:513`) → `buildRouterActionWithVH(..., vhRetryPolicy, vhHedgePolicy)` (`:557`) (+ the weighted arm `buildWeightedRouterAction`). Add a `fractionalDenominator` helper (NONE exists in the package — verified) + `parseHedgePolicy` (mirror `parseRetryPolicy` `:615` — route-level overrides vhost):

```go
// fractionalDenominator lowers a type.v3.FractionalPercent_DenominatorType to
// its integer denominator (HUNDRED=100, TEN_THOUSAND=10000, MILLION=1000000).
func fractionalDenominator(d typev3.FractionalPercent_DenominatorType) uint32 {
	switch d {
	case typev3.FractionalPercent_TEN_THOUSAND:
		return 10000
	case typev3.FractionalPercent_MILLION:
		return 1000000
	default: // HUNDRED (the proto default)
		return 100
	}
}
```

```go
func parseHedgePolicy(r *routev3.RouteAction, vhHedgePolicy *routev3.HedgePolicy, name string) (*router.HedgePolicy, error) {
	eff := r.GetHedgePolicy()
	if eff == nil { eff = vhHedgePolicy }
	if eff == nil { return nil, nil }
	ir := uint32(1)
	if v := eff.GetInitialRequests(); v != nil { ir = v.GetValue() }
	num, den := uint32(0), uint32(100)
	if c := eff.GetAdditionalRequestChance(); c != nil {
		num = c.GetNumerator()
		den = fractionalDenominator(c.GetDenominator()) // HUNDRED=100/TEN_THOUSAND=10000/MILLION=1000000
	}
	hp, err := router.NewHedgePolicy(ir, eff.GetHedgeOnPerTryTimeout(), num, den)
	if err != nil {
		return nil, fmt.Errorf("route: %q: hedge_policy: %s", name, err.Error())
	}
	return hp, nil
}
```
In `buildRouterActionWithVH`, after `parseRetryPolicy`: `hp, err := parseHedgePolicy(r, vhHedgePolicy, cs.Cluster)` (reject-propagate); store `hedgePolicy: hp` on the returned `clusterRouteAction`; and `if hp != nil && hp.triggersConcurrency() { c.EnsureRetryStats() }` (a hedge increments `upstream_rq_retry` even with no retry_policy — AMEND-H5 reuses the retry counters). Add `hedgePolicy *router.HedgePolicy` to the `clusterRouteAction` struct.

- [ ] **Step 4: Run to verify they pass** — `go test ./internal/filter/hcm/ -run 'Hedge' -v` → PASS.

- [ ] **Step 5: Byte-stability gate** — `go test ./internal/filter/hcm/... ./internal/filter/http/router/...` → all PASS (no signature change reached the dispatch yet).

- [ ] **Step 6: Commit** — `git commit -am "phase 42.2b Task 7: hedge_policy parse + vhost fallback + clusterRouteAction.hedgePolicy carry"`

---

## Task 8: Wire the dispatch — widen `H{1,2}ClusterAction` + `routerAction.hp` + the closure-switch hedge branch

**Files:**
- Modify: `internal/filter/http/router/router.go` (`:565` constructor, `:578` switch — the `hp` field already added in Task 3), `internal/filter/http/router/router_h2.go` (`:39`, `:47`), `internal/filter/http/router/router_weighted.go` (`H1WeightedClusterAction` `:104` + its `if a.rp != nil` switch `:116` + the per-entry `&routerAction{...}` build `:107`; `H2WeightedClusterAction` `:126` + switch `:137` + build `:129` — each gains the `hp` param + the parallel hedge branch + sets `hp` on the per-entry action)
- Modify: `internal/filter/hcm/actions.go` (`do` `:214`, `asRouterAction` `:237`, `asRouterActionH2` `:250`, the weighted bridge)
- Modify: the existing `router_test.go`/`config_test.go` call sites (signature churn)

- [ ] **Step 1: Write the failing test** — a route with `hedge_policy{hedge_on_per_try_timeout:true}` dispatched through `H1ClusterAction(...)` routes to `hedgeExecutorH1` (assert via a behavior probe — a held attempt + a hedge fires); a route with nil/non-triggering `hp` routes to the existing path (byte-stable).

- [ ] **Step 2: Run to verify it fails** → FAIL (signature mismatch / no hedge branch).

- [ ] **Step 3: Implement.** Widen `H1ClusterAction(c, hps, subsetMatch, rp, hp *HedgePolicy)` + `H2ClusterAction(...)` (+ the weighted constructors); add `hp *HedgePolicy` to `routerAction`/`routerActionH2`; the closure switch (router.go `:578`, router_h2.go `:47`) gains the hedge branch BEFORE the rp branch:

```go
if a.hp != nil && a.hp.triggersConcurrency() {
	return hedgeExecutorH1(ctx, a, req) // H2: hedgeExecutorH2
}
if a.rp != nil {
	return retryExecutorH1(ctx, a, req)
}
return doH1ClusterAction(ctx, a, req)
```
Update `clusterRouteAction.do`/`asRouterAction`/`asRouterActionH2` + the weighted bridge to pass `a.hedgePolicy`. Update ALL existing call sites (they pass `nil` for the new `hp` — the byte-stable default).

- [ ] **Step 4: Run to verify it passes** — `go test ./internal/filter/http/router/ -run Dispatch -race -v` → PASS.

- [ ] **Step 5: Byte-stability gate** — run the FULL pre-existing unit suite + the 78-dir differential subset spot-check:
```bash
go build ./... && go vet ./... && go test ./internal/... 2>&1 | tail -5
go test ./test/differential/ -run 'TestDifferential/0075|TestDifferential/0076' -count=1 -v 2>&1 | tail -10
```
Expected: all PASS (nil/non-triggering `hp` ⇒ the exact 42.1/42.2a path).

- [ ] **Step 6: Commit** — `git commit -am "phase 42.2b Task 8: wire the hedge dispatch — widen H{1,2}ClusterAction + the closure-switch hedge branch (byte-stable when hp nil)"`

---

## Task 9: The `0077-hedge-on-per-try-timeout` cross-side differential fixture

**Files:**
- Create: `test/fixtures/0077-hedge-on-per-try-timeout/{driver/driver.go,expectations.yaml,README.md}`
- Modify: `test/differential/runner_test.go` (the import block `:103`)

A HYBRID of `0076` (the cross-side single-held-host STATIC/STRICT_DNS config shape) + `0074` (the concurrent-fire + poll-`/stats` + `/__release` driver model — hedging BLOCKS until release, so the request is fired in a goroutine). Cluster `c_hedge` = {1 `BlockingHoldResponder` host}; route `/hedge` with `retry_policy{retry_on:"5xx", num_retries:3, per_try_timeout:0.25s}` + `hedge_policy{hedge_on_per_try_timeout:true}` + **`timeout:0s`** (disable the reference's global route timeout — AMEND-H7; envoy-go has none).

- [ ] **Step 0 (IMPL re-probe, optional but recommended):** re-confirm against `contrib-v1.37.2` that the held-host hedge run yields `upstream_rq_retry==3`/`upstream_rq_per_try_timeout==0`/`upstream_rq_retry_limit_exceeded==1`/`upstream_rq_total==4`/`upstream_rq_200==1` after `/__release` (the SPEC §11 D-H-DIFFERENTIAL shape; also resolve D-S422B-9 — note whether `upstream_rq_retry_success` is 0 or 1). Record in PROGRESS.

- [ ] **Step 1: Write the driver** (model `AssertStats` on `0074`'s concurrent-poll-release):
  1. baseline scrape `/stats`.
  2. fire `GET /hedge` in a goroutine (it BLOCKS — the held attempts never complete pre-release).
  3. poll `adminAddr/stats` (deadline 10s, poll 50ms, NO fixed sleep) until `cluster.c_hedge.upstream_rq_retry - baseline == 3` AND `upstream_rq_retry_limit_exceeded - baseline == 1` (the original + 3 hedges all launched, the cap hit).
  4. `GET /__release` on `127.0.0.1:<backendPort>` ⇒ the held attempts complete 200 ⇒ the blocked `/hedge` returns; join the goroutine; assert downstream status **200**.
  5. final scrape; delta-assert (cross-side EXACT):
     `upstream_rq_retry==3`, `upstream_rq_per_try_timeout==0` (AMEND-H1, the load-bearing), `upstream_rq_retry_limit_exceeded==1`, `upstream_rq_total==4`, `upstream_rq_200==1`, final 200; `ref[upstream_rq_total] > 0` (decode-ran guard). Run SEQUENTIALLY per side (subject fully, then reference).
  Single-source the constants (`refContainerListenerPort=19166`, `perTryTimeoutMs=250`, `numHedges=3`) — `reference_fixture_workload_constant_desync`.

- [ ] **Step 2: Register** in `runner_test.go` (mirror the `0076` import). Run: `go test ./test/differential/ -run 'TestDifferential/0077' -count=1 -v` → PASS (both sides 200, deltas equal).

- [ ] **Step 3: Commit** — `git add -A test/ && git commit -m "phase 42.2b Task 9: 0077-hedge-on-per-try-timeout cross-side fixture (held-host first-acceptable-wins; per_try_timeout==0)"`

---

## Task 10: `0077` deliberate breaks (2) + 20-run flake + `-race`

**Files:** (temporary edits to `hedge.go`, reverted)

- [ ] **Step 1: Break A — neuter the hedge launch.** Temporarily make `fireHedge` a no-op (the per-try-timeout launches NO hedge). Run `go test ./test/differential/ -run 'TestDifferential/0077' -count=1` → MUST FAIL (the poll for `upstream_rq_retry==3` times out / the request hangs to the 10s deadline). Record the failure. **Restore** (`git restore internal/filter/http/router/hedge.go` — GIT HYGIENE per `feedback_subagent_worktree_detach`: NO checkout-sha, NO amend; the controller re-verifies the branch).

- [ ] **Step 2: Break B — the AMEND-H1 proof.** Temporarily add `a.cluster.IncUpstreamRqPerTryTimeout()` inside `fireHedge`. Run `-run 'TestDifferential/0077' -count=1` → MUST FAIL (`upstream_rq_per_try_timeout` delta != 0 cross-side). Record. **Restore.**

- [ ] **Step 3: Flake gate** — `go test ./test/differential/ -run 'TestDifferential/0077' -count=20` → 20/20 PASS. AND `-race -count=1` (the concurrent collector) → PASS.

- [ ] **Step 4: Commit** (PROGRESS update only — the breaks were reverted) — `git commit -am "phase 42.2b Task 10: 0077 deliberate breaks (2) bit + 20/20 flake-free + -race clean"`

---

## Task 11: Full 79-dir six-gate

- [ ] **Step 1: The six gates** (from the worktree root):
```bash
go build ./... && go vet ./...                                  # gate 1-2
gofmt -l internal/ test/                                        # gate 3 (empty)
golangci-lint run internal/filter/http/router/... internal/filter/hcm/... internal/cluster/...  # gate 4 (per feedback_pertask_gofmt_lint)
go test ./internal/... -race -count=1 2>&1 | tail -5            # gate 5
go test ./test/differential/ -count=1 2>&1 | tail -15           # gate 6 (the full 79-dir)
go mod tidy -diff                                               # ZERO (no new module)
```
Expected: ALL GREEN; the full differential is **79 dirs**. (If an UNRELATED dir fails `subject ready: EOF`, isolate-re-run it then re-run the full suite per `reference_differential_fullsuite_startup_flake`.)

- [ ] **Step 2: Commit** — `git commit -am "phase 42.2b Task 11: full 79-dir six-gate GREEN (+ -race)"`

---

## Task 12: ADR-0251 body + BEHAVIOR_CONTRACT hedging sub-block

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (append ADR-0251 §Decision + §Consequences, promote the SPEC §13 §Context DRAFT)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the `### Route — request recovery (retries)` subsection)

- [ ] **Step 1: ADR-0251** (per ADR-0044 — §Context promoted PROPOSED→ACCEPTED; §Decision + §Consequences in-place). Cover: the SEPARATE concurrent `hedgeExecutor` dispatched via the closure switch; the per-attempt HEDGE-TRIGGER timer (NOT the 42.2a cancelling deadline) ⇒ `upstream_rq_retry`++ NEVER `upstream_rq_per_try_timeout` (AMEND-H1); the buffered first-acceptable collector + the H1/H2 loser-cancel asymmetry (H1 honors only `ctx.Deadline`, so H1 losers complete on backend-response — the recorded departure: NO active H1-loser socket reset; H2 `RoundTrip` aborts on cancel); the budget-counted fan-out (subject-side; acquire-gates-spawn); NO new stat (surface STAYS 1181); byte-identical when no `hedge_policy`. DECISIONS tail ADR-0250 → **ADR-0251** (next-free ADR-0252).

- [ ] **Step 2: BEHAVIOR_CONTRACT** — add the `#### Hedging (hedge_on_per_try_timeout + the fan-out) — phase 42.2b (ADR-0251)` sub-block (SPEC §9 wording); update the deferred-retry-surface line (the `HedgePolicy` moves to LANDED) + the row-42-status line; the stat-surface block STAYS **1181** (a "+0 — no new name" note; NO new `Phase 42.2b` stat-count row).

- [ ] **Step 3: Commit** — `git commit -am "phase 42.2b Task 12: ADR-0251 body + BEHAVIOR_CONTRACT hedging sub-block"`

---

## Task 13: Completion bundle — PROGRESS/README + STATE + ROADMAP row 42 → done + next-prompt

**Files:**
- Modify: `docs/envoy-go/phases/42.2b-hedging/PROGRESS.md`; Create: `docs/envoy-go/phases/42.2b-hedging/README.md`
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `next-prompt.txt`

- [ ] **Step 1: Finalize PROGRESS** (all 13 tasks ✓; as-built counts: fixtures 78 → **79**, stat surface **1181 UNCHANGED**, fuzzers 42, BackendKind 36, DECISIONS ADR-0250 → **ADR-0251**, ZERO new packages/modules) + **README.md** (the phase summary). Run the FINAL ADR-0045 split-gate re-check note (NO split — as-built LoC/tasks under the gate).

- [ ] **Step 2: STATE.md** — advance the active-phase to `phase 42.2b (hedging) IMPL done`; demote the SPEC-done block to `prior active-phase`. **ROADMAP.md** row 42 — flip `in-progress → done` (ALL THREE legs 42.1+42.2a+42.2b landed — ADR-0106 + `reference_roadmap_split_phase_row_done`; the Upstream-robustness family then has 1 candidate left {per-protocol connection pooling}). **next-prompt.txt** — roll to a FRESH BRAINSTORM for a NEW subject (the row-41-IMPL precedent: IMPL-done → a fresh BRAINSTORM).

- [ ] **Step 3: Final verification** — `go build ./... && go test ./test/differential/ -count=1 2>&1 | tail -5` (79-dir GREEN). Update any memory if a new cross-cutting lesson emerged (e.g. the H1-loser-cancel asymmetry).

- [ ] **Step 4: Commit** — `git commit -am "phase 42.2b (hedging) IMPL: the concurrent first-acceptable-wins hedge over the 42.2a deadline (ADR-0251); row 42 done"`

---

## Execution notes (for the controller)

- **Subagent-driven** (recommended): fresh implementer per task + the two-stage review (spec-compliance then code-quality). The concurrency tasks (3–6) and the differential (9–10) are the highest-risk — give them the most capable model + insist on `-race -count=1`.
- **`feedback_subagents_no_push`:** subagents commit LOCALLY ONLY; the controller squashes + pushes at stage-close.
- **`feedback_subagent_worktree_detach`:** the Task-10 deliberate breaks use `git restore` (NO checkout-sha, NO amend); the controller re-verifies the branch after each break.
- **`feedback_pertask_gofmt_lint`:** every task runs `gofmt -l` + `golangci-lint` on the touched packages, not just `go vet`.
- **The load-bearing invariant to guard at every review:** the hedge path NEVER calls `IncUpstreamRqPerTryTimeout` (AMEND-H1); the `0077` Break B + the unit test are the proofs. And: byte-identical when `hedge_policy` is nil/non-triggering (the full 78→79-dir gate, the nil-`hp` fall-through).
