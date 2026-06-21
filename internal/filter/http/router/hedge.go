package router

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

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

// TriggersConcurrency reports whether this policy launches more than the single
// primary attempt (so the concurrent hedgeExecutor is dispatched). A nil policy
// or a default (initial_requests:1, no chance, no hedge_on_per_try_timeout) ⇒
// false ⇒ the byte-stable 42.1/42.2a path. Exported (Task 8) so the hcm parse
// layer reuses it directly instead of a duplicate hedgeTriggersConcurrency.
func (hp *HedgePolicy) TriggersConcurrency() bool {
	if hp == nil {
		return false
	}
	return hp.InitialRequests > 1 || hp.HedgeOnPerTryTimeout || hp.AdditionalRequestChanceNum > 0
}

// readAndCloseBody buffers + closes a request body (the ≤1MiB ADR-0076 buffer;
// the HCM 413s an over-cap body before the router, so io.ReadAll is safe). The
// replay source for every concurrent attempt. Mirrors retryExecutorH1's idiom.
func readAndCloseBody(b io.ReadCloser) ([]byte, error) {
	defer func() { _ = b.Close() }()
	return io.ReadAll(b)
}

// additionalChanceSeq is the package-level monotonic source for the
// additional_request_chance deterministic draw. Each executor invocation draws
// one seq via atomic.AddUint64, making the chance decision deterministic-per-
// process (and concurrency-safe) while letting tests call drawAdditional directly
// with an explicit seq.
var additionalChanceSeq uint64

// drawAdditional is the DETERMINISTIC additional_request_chance decision given a
// FractionalPercent (num/den) and a sequence number. It does NOT call math/rand.
// Boundary contract (unit-tested): num==0 ⇒ never (false); num==den ⇒ always
// (true). den==0 ⇒ false (guard). A partial num varies across seq but is stable
// per seq.
func drawAdditional(num, den uint32, seq uint64) bool {
	if num == 0 || den == 0 {
		return false
	}
	return (seq % uint64(den)) < uint64(num)
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
	maxHedges := 0
	if hp.HedgeOnPerTryTimeout && rp != nil {
		maxHedges = rp.numRetries // each in-flight attempt may trigger one hedge, up to num_retries
	}
	initial := int(hp.InitialRequests)
	chanceExtra := 0
	if hp.AdditionalRequestChanceNum > 0 {
		chanceExtra = 1 // STRUCTURAL provisioning: one possible extra fan-out attempt
	}

	var body []byte
	if req.Body != nil {
		body, _ = readAndCloseBody(req.Body)
	}

	hedgeCtx, cancelAll := context.WithCancel(ctx)
	defer cancelAll() // reap all in-flight on return (H2 aborts; H1 completes on response)

	// Both channels share the canonical capacity so neither a late timer send nor
	// a loser result-send ever blocks (no goroutine leak) even after the collector
	// has already returned. The max goroutine count is 1 primary + (initial-1)
	// fan-out + chanceExtra + (≤maxHedges per-try-timeout hedges) =
	// initial+chanceExtra+maxHedges, so cap = that + 1 slack never blocks a send.
	results := make(chan attemptResult, initial+chanceExtra+maxHedges+1)
	hedgeReq := make(chan struct{}, initial+chanceExtra+maxHedges+1) // timer firings funnel here (buffered, never blocks)

	// launch starts ONE attempt goroutine. consumedBudget marks a budget-counted
	// hedge (NOT the primary) so the goroutine releases its slot on settle.
	launch := func(consumedBudget bool) {
		go func() {
			r := req.Clone(hedgeCtx)
			if body != nil {
				r.Body = io.NopCloser(bytes.NewReader(body))
			}
			if hp.HedgeOnPerTryTimeout && rp != nil && rp.perTryTimeout > 0 {
				// Arm a per-attempt hedge-trigger. THE CONCURRENCY RULE (I1): the
				// callback runs on its own goroutine, so it ONLY signals the
				// collector via a non-blocking buffered send — it mutates NO shared
				// state and calls NO Inc*/TryAcquireRetry/launch. Stopped on settle.
				timer := time.AfterFunc(rp.perTryTimeout, func() {
					select {
					case hedgeReq <- struct{}{}:
					case <-hedgeCtx.Done():
					}
				})
				defer timer.Stop()
			}
			resp, ep, err := doH1ClusterAction(hedgeCtx, a, r)
			if consumedBudget {
				a.cluster.ReleaseRetry() // release the slot this attempt held
			}
			results <- attemptResult{resp, ep, err}
		}()
	}

	// FAN-OUT (Task 6) — runs SINGLE-THREADED in this executor goroutine DURING
	// SETUP, before the collector loop, so the hedgesLaunched/Inc*/TryAcquireRetry
	// mutations here do not race the launched goroutines (which only send to
	// results). hedgesLaunched consumed here correctly REDUCES the remaining
	// per-try-timeout hedge budget seen by the collector's hedge branch.
	hedgesLaunched := 0
	inFlight := 0
	launch(false) // the PRIMARY attempt (NOT a retry)
	inFlight++
	// (initial-1) budget-counted fan-out attempts + an optional chance attempt.
	// acquire-gates-spawn (D-S422B-3): TryAcquireRetry BEFORE the goroutine; on
	// DENY (overflow counted inside) stop launching further fan-out.
	extra := initial - 1
	if chanceExtra == 1 && drawAdditional(hp.AdditionalRequestChanceNum, hp.AdditionalRequestChanceDen, atomic.AddUint64(&additionalChanceSeq, 1)) {
		extra++ // the deterministic draw succeeded ⇒ one more budget-counted attempt
	}
	for i := 0; i < extra; i++ {
		if !a.cluster.TryAcquireRetry() { // budget overflow counted inside
			break
		}
		hedgesLaunched++
		a.cluster.IncUpstreamRqRetry()
		a.cluster.IncUpstreamRqRetryBackoffExponential() // NO time.Sleep; NEVER IncUpstreamRqPerTryTimeout (AMEND-H1)
		launch(true)
		inFlight++
	}

	// The collector loop is the SOLE mutator of hedgesLaunched/limitExceededOnce
	// and the SOLE caller of Inc*/TryAcquireRetry/launch (THE CONCURRENCY RULE).
	limitExceededOnce := false
	var last attemptResult
	for inFlight > 0 {
		select {
		case ar := <-results:
			inFlight--
			last = ar
			if rp == nil || !rp.matches(ar.resp.Status, ar.resp.localOrigin) {
				return ar.resp, ar.ep, ar.err // first acceptable wins; defer cancelAll reaps losers
			}
		case <-hedgeReq: // a per-try-timeout fired ⇒ launch a hedge (a RETRY, not a per_try_timeout)
			if hedgesLaunched >= maxHedges {
				if !limitExceededOnce { // AMEND-H2: "no more hedges", NOT a final failure — keep waiting
					a.cluster.IncUpstreamRqRetryLimitExceeded()
					limitExceededOnce = true
				}
				continue
			}
			if !a.cluster.TryAcquireRetry() { // budget overflow counted inside
				continue
			}
			hedgesLaunched++
			a.cluster.IncUpstreamRqRetry()
			a.cluster.IncUpstreamRqRetryBackoffExponential() // NO time.Sleep; NEVER IncUpstreamRqPerTryTimeout (AMEND-H1)
			launch(true)
			inFlight++
		}
	}
	return last.resp, last.ep, last.err // all retriable ⇒ the last
}

// hedgeExecutorH2 is the H2 twin of hedgeExecutorH1: the concurrent
// first-acceptable-wins racer for H2 when the route carries a triggering
// hedge_policy. It MIRRORS hedgeExecutorH1's collector + hedge-trigger timer
// EXACTLY, differing only in the driver (doH2ClusterAction over a value-type
// h2.H2Request), the body replay (h2.H2Request.Body is a buffered []byte FIELD —
// the value IS the snapshot, so we pass req by value per attempt with NO
// Clone/NopCloser dance), and the action type (*routerActionH2).
//
// The H2 cancel difference (the H2 crux): doH2ClusterAction's cc.RoundTrip
// HONORS ctx.Done() (H1 honors only ctx.Deadline), so an H2 loser ABORTS
// promptly on cancelAll(), returning the Status:0 CANCEL sentinel + an *h2.Error.
// The collector's existing !rp.matches(status, localOrigin) guard handles this
// correctly: matches(0,false)==false, so a Status:0 is returned as final — which
// is exactly right for the client-cancel case (a client cancel returns Status:0,
// is NEVER a per_try_timeout). Losers only reach Status:0 AFTER a winner returned
// (collector already exited) or on a parent-ctx cancel — both correct.
//
// Kept separate from hedgeExecutorH1 (rather than folded into a shared helper)
// for the same reason retryExecutorH2 is: the driver-signature + body-replay
// differences make a shared abstraction strictly uglier than the intentional
// duplication (the package already accepts H1/H2 executor duplication over a
// forced shared abstraction — see retryExecutorH2's doc comment).
//
// THE CONCURRENCY RULE (mirrored from H1): the AfterFunc callback is a pure
// non-blocking hedgeReq send (+ a <-hedgeCtx.Done() escape); the collector loop
// is the SOLE mutator of hedgesLaunched/limitExceededOnce and the SOLE caller of
// Inc*/TryAcquireRetry/launch. AMEND-H1: a hedge increments upstream_rq_retry +
// upstream_rq_retry_backoff_exponential only (no time.Sleep) and NEVER calls
// IncUpstreamRqPerTryTimeout. AMEND-H2: limit_exceeded-once is "no more hedges",
// not a final failure — the first acceptable in-flight result still wins.
func hedgeExecutorH2(ctx context.Context, a *routerActionH2, req h2.H2Request) (ActionResponse, cluster.Endpoint, error) {
	hp := a.hp
	rp := a.rp // may be nil (a pure-fanout hedge with no retry_policy); guard num_retries below
	maxHedges := 0
	if hp.HedgeOnPerTryTimeout && rp != nil {
		maxHedges = rp.numRetries // each in-flight attempt may trigger one hedge, up to num_retries
	}
	initial := int(hp.InitialRequests)
	chanceExtra := 0
	if hp.AdditionalRequestChanceNum > 0 {
		chanceExtra = 1 // STRUCTURAL provisioning: one possible extra fan-out attempt
	}

	hedgeCtx, cancelAll := context.WithCancel(ctx)
	defer cancelAll() // reap all in-flight on return (H2 losers abort on ctx.Done)

	// Both channels share the canonical capacity so neither a late timer send nor
	// a loser result-send ever blocks (no goroutine leak) even after the collector
	// has already returned. The max goroutine count is 1 primary + (initial-1)
	// fan-out + chanceExtra + (≤maxHedges per-try-timeout hedges) =
	// initial+chanceExtra+maxHedges, so cap = that + 1 slack never blocks a send.
	results := make(chan attemptResult, initial+chanceExtra+maxHedges+1)
	hedgeReq := make(chan struct{}, initial+chanceExtra+maxHedges+1) // timer firings funnel here (buffered, never blocks)

	// launch starts ONE attempt goroutine. consumedBudget marks a budget-counted
	// hedge (NOT the primary) so the goroutine releases its slot on settle. req is
	// captured by value per goroutine — the buffered []byte body field re-presents
	// in full each attempt (no Clone/NopCloser snapshot dance; see the doc).
	launch := func(consumedBudget bool) {
		go func() {
			if hp.HedgeOnPerTryTimeout && rp != nil && rp.perTryTimeout > 0 {
				// Arm a per-attempt hedge-trigger. THE CONCURRENCY RULE (I1): the
				// callback runs on its own goroutine, so it ONLY signals the
				// collector via a non-blocking buffered send — it mutates NO shared
				// state and calls NO Inc*/TryAcquireRetry/launch. Stopped on settle.
				timer := time.AfterFunc(rp.perTryTimeout, func() {
					select {
					case hedgeReq <- struct{}{}:
					case <-hedgeCtx.Done():
					}
				})
				defer timer.Stop()
			}
			resp, ep, err := doH2ClusterAction(hedgeCtx, a, req) // req by value: the body field is the snapshot
			if consumedBudget {
				a.cluster.ReleaseRetry() // release the slot this attempt held
			}
			results <- attemptResult{resp, ep, err}
		}()
	}

	// FAN-OUT (Task 6) — runs SINGLE-THREADED in this executor goroutine DURING
	// SETUP, before the collector loop, so the hedgesLaunched/Inc*/TryAcquireRetry
	// mutations here do not race the launched goroutines (which only send to
	// results). hedgesLaunched consumed here correctly REDUCES the remaining
	// per-try-timeout hedge budget seen by the collector's hedge branch.
	hedgesLaunched := 0
	inFlight := 0
	launch(false) // the PRIMARY attempt (NOT a retry)
	inFlight++
	// (initial-1) budget-counted fan-out attempts + an optional chance attempt.
	// acquire-gates-spawn (D-S422B-3): TryAcquireRetry BEFORE the goroutine; on
	// DENY (overflow counted inside) stop launching further fan-out.
	extra := initial - 1
	if chanceExtra == 1 && drawAdditional(hp.AdditionalRequestChanceNum, hp.AdditionalRequestChanceDen, atomic.AddUint64(&additionalChanceSeq, 1)) {
		extra++ // the deterministic draw succeeded ⇒ one more budget-counted attempt
	}
	for i := 0; i < extra; i++ {
		if !a.cluster.TryAcquireRetry() { // budget overflow counted inside
			break
		}
		hedgesLaunched++
		a.cluster.IncUpstreamRqRetry()
		a.cluster.IncUpstreamRqRetryBackoffExponential() // NO time.Sleep; NEVER IncUpstreamRqPerTryTimeout (AMEND-H1)
		launch(true)
		inFlight++
	}

	// The collector loop is the SOLE mutator of hedgesLaunched/limitExceededOnce
	// and the SOLE caller of Inc*/TryAcquireRetry/launch (THE CONCURRENCY RULE).
	limitExceededOnce := false
	var last attemptResult
	for inFlight > 0 {
		select {
		case ar := <-results:
			inFlight--
			last = ar
			if rp == nil || !rp.matches(ar.resp.Status, ar.resp.localOrigin) {
				return ar.resp, ar.ep, ar.err // first acceptable wins; defer cancelAll reaps losers
			}
		case <-hedgeReq: // a per-try-timeout fired ⇒ launch a hedge (a RETRY, not a per_try_timeout)
			if hedgesLaunched >= maxHedges {
				if !limitExceededOnce { // AMEND-H2: "no more hedges", NOT a final failure — keep waiting
					a.cluster.IncUpstreamRqRetryLimitExceeded()
					limitExceededOnce = true
				}
				continue
			}
			if !a.cluster.TryAcquireRetry() { // budget overflow counted inside
				continue
			}
			hedgesLaunched++
			a.cluster.IncUpstreamRqRetry()
			a.cluster.IncUpstreamRqRetryBackoffExponential() // NO time.Sleep; NEVER IncUpstreamRqPerTryTimeout (AMEND-H1)
			launch(true)
			inFlight++
		}
	}
	return last.resp, last.ep, last.err // all retriable ⇒ the last
}
