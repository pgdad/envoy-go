package router

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
)

// ErrMsgMaxIntervalBelowBase is the retry_policy max<base reject suffix. The hcm
// parse layer (parseRetryPolicy) re-emits it with a "route: %q: retry_policy: "
// prefix; keep both sites referencing this const so they cannot drift.
const ErrMsgMaxIntervalBelowBase = "max_interval must be greater than or equal to the base_interval"

// ErrMsgPerTryTimeoutNegative is the retry_policy per_try_timeout reject suffix.
// The hcm parse layer re-emits it with a "route: %q: retry_policy: " prefix;
// keep both sites referencing this const so they cannot drift.
const ErrMsgPerTryTimeoutNegative = "per_try_timeout must not be negative"

// retryOnBits is the enforced subset of Envoy's retry_on conditions (SPEC §5 /
// AMEND-RT1) packed into a bitset. Deferred conditions (retriable-headers,
// envoy-ratelimited, grpc-*) are intentionally absent.
type retryOnBits uint8

const (
	retry5xx          retryOnBits = 1 << iota // any upstream 5xx
	retryGatewayError                         // {502,503,504}
	retryConnectFail                          // local-origin connect failure
	retryStatusCodes                          // status ∈ retriableCodes
	retryReset                                // local-origin reset (a per-try-timeout is a reset; ⊇ connect-failure + timeouts)
)

// RetryPolicy holds the parsed, enforced retry configuration for a route.
type RetryPolicy struct {
	on             retryOnBits
	numRetries     int
	retriableCodes map[int]bool
	baseInterval   time.Duration // default 25ms
	maxInterval    time.Duration // default 10×base
	perTryTimeout  time.Duration // 0 ⇒ no per-attempt bound; <0 rejected at parse
}

// NewRetryPolicy builds a parsed policy. baseInterval/maxInterval are the parsed
// retry_back_off (zero ⇒ defaults: base=25ms, max=10×base per AMEND-RT6).
// Returns an error for the backoff reject arm (D-S42-1: max < base). retry_on is
// freeform (never an error). num_retries==0 ⇒ no retries (threaded verbatim).
//
// A zero baseInterval here means "unset" and gets the 25ms default; the
// base_interval-required reject (when retry_back_off is present but base≤0) is
// enforced in buildRouterAction (Task 4), which sees the proto-presence bit.
func NewRetryPolicy(retryOn string, numRetries uint32, retriableCodes []uint32, baseInterval, maxInterval, perTryTimeout time.Duration) (*RetryPolicy, error) {
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
		return nil, errors.New(ErrMsgMaxIntervalBelowBase)
	}
	if perTryTimeout < 0 {
		return nil, errors.New(ErrMsgPerTryTimeoutNegative)
	}
	rp.perTryTimeout = perTryTimeout // 0 ⇒ no bound (executor skips the child ctx)
	return rp, nil
}

// NumRetries returns the parsed num_retries (the max retry attempts beyond the
// initial try). Exported for the hcm parse test (phase 42.1 Task 4) and the
// retry executors (Tasks 7/8).
func (rp *RetryPolicy) NumRetries() int { return rp.numRetries }

// PerTryTimeout returns the per-attempt deadline (0 ⇒ no bound). Exported for
// the hcm parse test (phase 42.2a Task 4) and the retry executors (Tasks 6/7).
func (rp *RetryPolicy) PerTryTimeout() time.Duration { return rp.perTryTimeout }

// backoff returns the delay before retry attempt n (1-based): full jitter over
// [0, min(maxInterval, baseInterval·2^(n-1))]. Delay-only — never perturbs
// counts (the differential is count-based, so math/rand jitter is fine here).
// The d<=0 guard catches the int64 left-shift OVERFLOW for large n and clamps
// to maxInterval.
func (rp *RetryPolicy) backoff(n int) time.Duration {
	d := rp.baseInterval << uint(n-1)
	if d > rp.maxInterval || d <= 0 {
		d = rp.maxInterval
	}
	return time.Duration(rand.Int63n(int64(d) + 1))
}

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
		case "connect-failure":
			b |= retryConnectFail
		case "reset":
			b |= retryReset
		case "retriable-status-codes":
			b |= retryStatusCodes
		}
	}
	return b
}

// matches reports whether an attempt's outcome is retriable. localOrigin marks a
// router-synthesized dial/connection failure (vs an upstream response): an
// upstream that RESPONDS 502/503/504 is NOT a connect-failure.
func (rp *RetryPolicy) matches(status int, localOrigin bool) bool {
	if rp.on&(retryConnectFail|retryReset) != 0 && localOrigin {
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

// perTryTimeoutRetriable reports whether a per-try-timeout (a synthesized 504,
// treated as a reset) is retriable under this policy: any 504-status token
// (5xx, gateway-error, retriable-status-codes∋504) OR `reset`. NOT
// connect-failure-alone (a per-try-timeout is a reset, not a connect failure).
func (rp *RetryPolicy) perTryTimeoutRetriable() bool {
	return rp.matches(504, false) || rp.on&retryReset != 0
}

// perTryTimedOutH2 reports whether an H2 attempt's outcome must be booked as a
// per-try timeout — which means Inc'ing upstream_rq_per_try_timeout, REPLACING
// the driver's ActionResponse with a synthesized 504 and DISCARDING its error.
// That is a destructive reclassification, so the predicate has to be exactly
// right; `now` is a parameter (rather than an inline time.Now()) so the
// decision is a pure function of its inputs and can be gated deterministically.
//
// Split out of retryExecutorH2 (phase 84.1 Task 5 fix) because it stopped being
// a predicate over timing alone. It was written when Status:0 had exactly ONE
// producer in doH2ClusterAction — the ctx-cancel CANCEL sentinel, for which a
// 504 is correct. Task 5 added a SECOND Status:0 producer: the locally-detected
// malformed-response-trailers rejection, which must reset the downstream stream
// and must NOT become a gateway error. Without the attemptErr exclusion below, a
// rejection landing at or after the per-try deadline is laundered into a 504
// with err=nil and the *h2.Error carrying the RST code is dropped on the floor —
// the exact pair the row's charter forbids, reached through a code path Task 5
// never touched.
//
// MEASURED, not reasoned — with a DISCRIMINATING probe, which matters because a
// bare 504 count cannot tell a LAUNDERED rejection from a LEGITIMATE per-try
// timeout: evaluating both the old and the new predicate over the SAME sample,
// the unnarrowed form mis-booked 551/2000 requests (27.6%) at a 300µs per-try
// timeout and 33-45/2000 at 400µs, falling to 0 at >=1.2ms. The window is
// SHORT-timeout-heavy and much wider there than a 504-count survey suggests.
// Any NEW Status:0 producer must revisit this list.
//
// The H1 sibling in retryExecutorH1 keeps its inline predicate deliberately: it
// tests `resp.localOrigin` ALONE and H1 has no Status:0 producer at all, so it
// cannot reach this failure mode and sharing a helper would only couple them.
func (rp *RetryPolicy) perTryTimedOutH2(parentCtxErr error, resp ActionResponse, attemptErr error, attemptDeadline, now time.Time) bool {
	if rp.perTryTimeout <= 0 {
		return false // no per-attempt bound configured
	}
	if parentCtxErr != nil {
		return false // a PARENT client-cancel is not a per-try timeout (42.1 invariant)
	}
	if !resp.localOrigin && resp.Status != 0 {
		return false // a clean upstream reply is never misclassified
	}
	if errors.Is(attemptErr, h2.ErrMalformedTrailers) {
		return false // stream-scoped reset, NOT a timeout — see the doc above
	}
	// Deadline elapse, by wall clock. Read BEFORE the caller's explicit cancel(),
	// which would stamp Canceled and lose the signal. This leg (not a ctx-error
	// check) is the load-bearing one: the driver propagates the child deadline
	// onto the upstream socket, so a stalled read can break via socket I/O
	// slightly BEFORE context's timer goroutine stamps Err(), and the wall clock
	// is immune to that lag.
	//
	// A second leg, `errors.Is(attemptCtxErr, context.DeadlineExceeded) || …`,
	// stood here until phase 84.1 Task 5 fix 2 and was deleted as PROVABLY
	// REDUNDANT — it could never be the deciding term. Proof: context.WithTimeout
	// derives its deadline from time.Now().Add(d), which retains the monotonic
	// reading, and Go's runtime timers never fire early. So attemptCtxErr ==
	// DeadlineExceeded implies the timer fired at some t0 >= attemptDeadline; the
	// caller reads attemptCtx.Err() BEFORE time.Now() (Go evaluates call arguments
	// left to right), hence now >= t0 >= attemptDeadline and the wall-clock leg is
	// already true. The converse does NOT hold — which is why the wall-clock leg is
	// the one that survives. It was also UNGATED: deleting it left the whole
	// package green, because every want:true case necessarily has an elapsed
	// deadline. A term that cannot decide anything reads as coverage; it is gone.
	return !now.Before(attemptDeadline)
}

// retryExecutorH1 wraps doH1ClusterAction with the retry loop (phase 42.1 Task 7,
// D-S42-7). Each iteration is ONE driver call — which itself re-runs the phase-41
// max_requests admission (TryAcquireRequest) + RecordUpstreamResult + bumps
// upstream_rq_total — so N attempts == N upstream_rq_total. The retry BUDGET
// (TryAcquireRetry/ReleaseRetry) is SEPARATE from request admission: it only gates
// whether a retry is ALLOWED.
//
// Increment semantics (D-S42-7), all keyed off attempt>0 (a real retry):
//   - upstream_rq_retry + upstream_rq_retry_backoff_exponential fire ONCE per retry,
//     AFTER the budget grant and BEFORE the backoff sleep + driver call.
//   - upstream_rq_retry_limit_exceeded fires when a retriable outcome persists AND
//     attempt>=numRetries (static num_retries-cap exhaustion).
//   - upstream_rq_retry_success fires when attempt>0 AND the outcome is non-retriable
//     (the retried request recovered).
//   - a budget-denied retry (TryAcquireRetry false) bails with the LAST outcome and
//     does NOT count as an upstream_rq_retry (the overflow counter was already
//     bumped inside tryAcquireRetry).
//
// Body capture/replay (D-S42-3): the HCM buffers the request body ≤1MiB in memory
// (over-cap is a prior 413), so io.ReadAll is cheap + safe. The body is reset before
// EVERY attempt (incl. attempt 0). A nil body (GET) skips capture/reset.
func retryExecutorH1(ctx context.Context, a *routerAction, req *http.Request) (ActionResponse, cluster.Endpoint, error) {
	rp := a.rp
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
			if !a.cluster.TryAcquireRetry() { // budget overflow → no retry; overflow counted inside
				return resp, ep, err
			}
			a.cluster.IncUpstreamRqRetry()
			a.cluster.IncUpstreamRqRetryBackoffExponential()
			time.Sleep(rp.backoff(attempt))
		}
		if body != nil {
			req.Body = io.NopCloser(bytes.NewReader(body))
		}
		attemptCtx := ctx
		var cancel context.CancelFunc
		var attemptDeadline time.Time
		if rp.perTryTimeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, rp.perTryTimeout)
			attemptDeadline, _ = attemptCtx.Deadline()
		}
		resp, ep, err = doH1ClusterAction(attemptCtx, a, req)
		// per-try-timeout discrimination (AMEND-PT4): the CHILD attemptCtx deadline
		// fired while the PARENT ctx is still alive ⇒ a per-try-timeout (NOT a client
		// cancel). The driver propagates the child deadline onto the upstream socket
		// (router.go SetDeadline), so a stalled read breaks via the socket I/O timeout
		// — which can return slightly BEFORE context's internal timer goroutine marks
		// attemptCtx.Err() == DeadlineExceeded. We therefore detect the timeout by
		// wall-clock deadline elapse (immune to that timer lag), READ BEFORE the
		// explicit cancel() below (cancel would otherwise stamp Canceled and we'd lose
		// the signal). The local-origin guard scopes this to the synthesized 502/503
		// failure paths a per-try deadline actually produces (a clean upstream response
		// under a long timeout is never misclassified).
		timedOut := rp.perTryTimeout > 0 && ctx.Err() == nil && resp.localOrigin &&
			(errors.Is(attemptCtx.Err(), context.DeadlineExceeded) || !time.Now().Before(attemptDeadline))
		if cancel != nil {
			cancel()
		}
		if attempt > 0 {
			a.cluster.ReleaseRetry()
		}
		if timedOut {
			a.cluster.IncUpstreamRqPerTryTimeout()
			resp = ActionResponse{Status: 504, Headers: localReplyHeaders(0), Body: nil} // synthesized 504, override the driver's 502
			err = nil
			if !rp.perTryTimeoutRetriable() {
				return resp, ep, err
			}
		} else if !rp.matches(resp.Status, resp.localOrigin) {
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

// retryExecutorH2 wraps doH2ClusterAction with the retry loop (phase 42.1 Task 8,
// D-S42-7). It MIRRORS retryExecutorH1's loop shape + increment ledger exactly,
// differing only in the driver signature (doH2ClusterAction over h2.H2Request) and
// the body-replay mechanics. Kept separate from retryExecutorH1 (rather than folded
// into a shared helper) because the driver-signature + body-replay differences make
// a shared abstraction strictly uglier than the ~30 LoC of intentional duplication
// — the Task-7 reviewer's explicit "do not force a premature shared abstraction"
// guidance (carried forward).
//
// Increment semantics are IDENTICAL to H1 (see retryExecutorH1's doc): retry +
// retry_backoff_exponential fire once per real retry after the budget grant and
// before the backoff sleep; retry_limit_exceeded on retriable-at-cap exhaustion;
// retry_success on a retried request that recovered; a budget-denied retry bails
// with the last outcome (the overflow counter was bumped inside TryAcquireRetry).
//
// The ctx-cancel non-retry (the H2 crux): a caller ctx-cancel returns Status:0 +
// an *h2.Error. matches(0, localOrigin=false) is FALSE (0 ∉ 5xx / gateway-error,
// not localOrigin), so the very first iteration returns it as-is — a client cancel
// is NEVER retried (proven by TestRetryExecutorH2_CtxCancelNotRetried). The
// synthesized dial/roundtrip 502s carry localOrigin:true (set in doH2ClusterAction)
// so a connect-failure retry_on still matches THOSE.
//
// Body replay (D-S42-3): the H2Request body is a fully-buffered []byte FIELD (not a
// consume-once io.Reader like *http.Request.Body), so no NopCloser/snapshot dance is
// needed — the field IS the snapshot and survives every attempt unmutated
// (doH2ClusterAction → RoundTrip reads, never rewrites, req.Body). We pass req by
// value per attempt so even a hypothetical future in-driver body mutation cannot
// leak across attempts (defensive; the current driver never mutates it).
func retryExecutorH2(ctx context.Context, a *routerActionH2, req h2.H2Request) (ActionResponse, cluster.Endpoint, error) {
	rp := a.rp
	var resp ActionResponse
	var ep cluster.Endpoint
	var err error
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			if !a.cluster.TryAcquireRetry() { // budget overflow → no retry; overflow counted inside
				return resp, ep, err
			}
			a.cluster.IncUpstreamRqRetry()
			a.cluster.IncUpstreamRqRetryBackoffExponential()
			time.Sleep(rp.backoff(attempt))
		}
		// req is a value type carrying a buffered []byte body — passing the value
		// per attempt re-presents the SAME full body each time (no consume-once
		// reset needed; see the body-replay note in the doc).
		attemptCtx := ctx
		var cancel context.CancelFunc
		var attemptDeadline time.Time
		if rp.perTryTimeout > 0 {
			attemptCtx, cancel = context.WithTimeout(ctx, rp.perTryTimeout)
			attemptDeadline, _ = attemptCtx.Deadline()
		}
		resp, ep, err = doH2ClusterAction(attemptCtx, a, req)
		// per-try-timeout discrimination (AMEND-PT4), H2 form. The predicate lives
		// in perTryTimedOutH2 (phase 84.1 Task 5 fix) — it is a destructive
		// reclassification with four exclusions, one of which distinguishes the two
		// different Status:0 producers doH2ClusterAction now has, so it is gated as
		// a pure function rather than inline. Every argument is read BEFORE the
		// explicit cancel() below, which would stamp Canceled and lose the signal.
		timedOut := rp.perTryTimedOutH2(ctx.Err(), resp, err, attemptDeadline, time.Now())
		if cancel != nil {
			cancel()
		}
		if attempt > 0 {
			a.cluster.ReleaseRetry()
		}
		if timedOut {
			a.cluster.IncUpstreamRqPerTryTimeout()
			resp = ActionResponse{Status: 504, Headers: h2LocalReplyHeaders(), Body: nil} // synthesized 504, override the Status:0/502
			err = nil
			if !rp.perTryTimeoutRetriable() {
				return resp, ep, err
			}
		} else if !rp.matches(resp.Status, resp.localOrigin) {
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
