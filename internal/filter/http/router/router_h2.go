package router

import (
	"context"
	"errors"
	"strings"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// h2GrantRaceMaxRetries bounds the transparent re-acquire loop in
// doH2ClusterAction for the (effectively unreachable in 43.2a) errH2GrantRaced
// lost-race signal from AcquireH2Stream. A small fixed bound: each iteration
// re-runs the full pick/HIT/dial/pend lifecycle, so a genuine transient race
// resolves on the first re-acquire; the bound only guards against a pathological
// repeat (which cannot occur with the current single-grant-per-conn invariant).
// (phase 43.2a, ADR-0253; Task 7)
const h2GrantRaceMaxRetries = 3

// H2ClusterAction returns an H2Action closure that proxies the per-request
// H2 upstream call to the supplied cluster's selected endpoint. The closure
// delegates to a fresh internal driver (doH2ClusterAction) modeled after the
// byte-preserved routerActionH2.doH2 logic but surfacing (status, bytesSent,
// picked, err) for the chain-mediated dispatch path. HCM h2dispatch.go builds
// one of these per matched route at filter-build time and injects it via
// *Filter.SetH2Action at request-start.
//
// Per SPEC §11.9 / SPEC §2.1:
//   - Cluster.DialH2 error        → 502 local reply, status=502, err=nil
//   - RoundTrip ctx-cancel        → status=0, err=*h2.Error(CANCEL) (sentinel)
//   - RoundTrip protocol error    → 502 local reply, status=502, err=nil
//   - Upstream HTTP status        → forwarded verbatim to downstream sw,
//     status=resp.Status, err=writer-error or nil.
//
// status=0 on the ctx-cancel path is the H2 sentinel per SPEC §2.1 last
// bullet; HCM's chain-completion access-log emit hook skips submission on
// status=0.
func H2ClusterAction(c *cluster.Cluster, hps []HashPolicy, subsetMatch cluster.SubsetMatch, rp *RetryPolicy, hp *HedgePolicy) H2Action {
	a := &routerActionH2{cluster: c, hashPolicies: hps, subsetMatch: subsetMatch, rp: rp, hp: hp}
	return func(ctx context.Context, req h2.H2Request) (ActionResponse, cluster.Endpoint, error) {
		// 42.1 Task 8: when the route carries an effective retry_policy, wrap the
		// single H2 driver call in the retry loop (body replay + the
		// retry/success/limit/backoff increments). nil rp ⇒ the byte-stable
		// single-attempt path (every existing non-retry route — all 76 differential
		// fixtures take this branch). Mirrors H1ClusterAction's switch.
		//
		// 42.2b Task 8: a TRIGGERING hedge_policy wins even when a retry_policy is
		// also present — hedgeExecutorH2 REUSES rp. The hedge branch precedes the
		// retry branch (mirrors H1ClusterAction). nil/non-triggering hp ⇒ byte-stable.
		if a.hp != nil && a.hp.TriggersConcurrency() {
			return hedgeExecutorH2(ctx, a, req)
		}
		if a.rp != nil {
			return retryExecutorH2(ctx, a, req)
		}
		return doH2ClusterAction(ctx, a, req)
	}
}

// doH2ClusterAction runs the per-request H2 upstream-dial dispatch and surfaces
// the logical ActionResponse for the H2Action closure. Phase 07.1 Task 18
// prereq P1: returns ActionResponse instead of writing wire bytes via sw.
// HCM h2dispatch.go's chain-completion path runs the response through the
// encode chain THEN writes HEADERS+DATA frames via sw.
//
// On dial / RoundTrip failure paths, returns an ActionResponse with status=502
// and the canonical bad502Body. On caller-ctx-cancel returns Status=0 + an
// *h2.Error so serverStream.dispatch emits RST_STREAM(CANCEL). On wire-write
// failure (impossible at this layer post-refactor — wire-write moved to
// dispatch), the error is just propagated.
func doH2ClusterAction(ctx context.Context, a *routerActionH2, req h2.H2Request) (ActionResponse, cluster.Endpoint, error) {
	picked := cluster.Endpoint{}

	a.cluster.IncUpstreamRqTotal()

	// Phase 41 (ADR-0248): circuit-breaker max_requests admission (H2). See doH1ClusterAction.
	if !a.cluster.TryAcquireRequest() {
		return ActionResponse{Status: 503, Headers: h2LocalReplyHeaders(), Body: nil}, picked, nil
	}
	defer a.cluster.ReleaseRequest()

	// 36.2: fold the route's hash_policy list into a ring_hash key carried on ctx
	// (cluster.WithHashKey). The H2 request carries no remote addr, so source_ip
	// uses the ctx-carried downstream addr set by HCM dispatch (h2dispatch.go).
	// ADR-0237. 43.2a Task 7.5: AcquireH2Stream now performs a SINGLE ctx-aware
	// LB pick (hash key + subset match) — this ctx key reaches the pool's endpoint
	// selection, restoring hash/subset/source_ip affinity over the multiplex pool.
	ctx, _, _ = applyHashKey(ctx, a.hashPolicies, h2HeaderVal(req), downstreamRemoteAddrFrom(ctx))

	// 38.1: thread the route-static metadata_match onto ctx (cluster.WithSubsetMatch).
	// Route-static (resolved once at config-build, NOT a per-request fold like
	// applyHashKey); threaded verbatim only when non-empty. 43.2a Task 7.5:
	// AcquireH2Stream's single ctx-aware pick consumes this ctx subset match.
	// ADR-0239.
	if !a.subsetMatch.Empty() {
		ctx = cluster.WithSubsetMatch(ctx, a.subsetMatch)
	}

	// 43.2a Task 7 (ADR-0253): acquire a multiplexed stream slot on a POOLED
	// upstream H2 conn (supersedes ADR-0056's per-request fresh dial). The
	// returned release is CALL-ONCE; defer it exactly once on the success path.
	// AcquireH2Stream may (rarely, defensively) return errH2GrantRaced — a
	// granted conn was evicted between the grant and the post-grant lookup. That
	// is a RETRYABLE lost-race, DISTINCT from errConnPoolOverflow (no overflow
	// stat, no 503): a bounded re-acquire is the intended disposition (a fresh
	// acquire re-runs the pick/HIT/dial/pend lifecycle). The bound (h2GrantRace
	// MaxRetries) is defensive; in 43.2a the branch is effectively unreachable
	// (a granted stream-conn has inFlight>0 so it cannot be evicted out from
	// under the grant), but we handle it cleanly so it can never leak to the
	// user as a 502 nor be miscounted as an overflow.
	var (
		cc      *h2.ClientConn
		release func()
		ep      cluster.Endpoint
		err     error
	)
	for attempt := 0; ; attempt++ {
		cc, release, ep, err = a.cluster.AcquireH2Stream(ctx)
		if err == nil || !cluster.IsH2GrantRaced(err) {
			break
		}
		if attempt >= h2GrantRaceMaxRetries {
			// Exhausted the bounded retry on the (effectively unreachable) race.
			// Fall back to the load-shed 503 (NOT a 502 — no upstream connect was
			// attempted; this is a capacity/race condition, not a gateway error).
			return ActionResponse{Status: 503, Headers: h2LocalReplyHeaders()}, picked, nil
		}
	}
	if err != nil {
		if cluster.IsConnPoolOverflow(err) {
			// ADR-0252: connection-pool overflow → 503 (NOT the 502 dial-failure
			// shape; a 502-labeled body would be wrong for a load-shed → empty Body).
			// upstream_rq_pending_overflow already incremented in the pool: NO
			// IncStatusClass (the dedicated overflow counter is the signal), and
			// NOT localOrigin (a load-shed is not a connect failure for retry_on).
			return ActionResponse{Status: 503, Headers: h2LocalReplyHeaders()}, picked, nil
		}
		a.cluster.IncStatusClass(502)
		if !ep.IsZero() { // a host was picked → attribute the local-origin connect failure
			a.cluster.RecordUpstreamResult(ep, cluster.UpstreamResult{StatusCode: 502, LocalOriginErr: true})
		}
		// 42.1 Task 8: localOrigin:true marks this as a router-synthesized DIAL
		// failure (not a proxied upstream 502), so RetryPolicy.matches treats a
		// connect-failure retry_on as retriable here. Mirrors the H1 dial-failure
		// 503 site (doH1ClusterAction sets localOrigin on its synthesized failures).
		return ActionResponse{Status: 502, Body: []byte(bad502Body), Headers: h2LocalReplyHeaders(), localOrigin: true}, picked, nil
	}
	// 43.2a: release the stream slot exactly once when this RoundTrip completes
	// (call-once contract; the slot is returned, promoting the next waiter).
	defer release()
	picked = ep

	resp, err := cc.RoundTrip(ctx, req)
	if err != nil {
		// 84.1 Task 5 (ADR-0306): a LOCALLY-DETECTED malformed response-trailer
		// block is a STREAM fault, not a conn fault and not a gateway error.
		// The reference resets the stream and leaves the upstream connection in
		// service, so this arm sits BEFORE the evict below and returns neither a
		// 502 local reply nor a poisoned-conn eviction. The h2 codec has already
		// RST_STREAM(INTERNAL_ERROR)'d the UPSTREAM stream at the capture site;
		// what is left is to make the DOWNSTREAM stream reset the same way.
		//
		// ⚠️ THE DISCRIMINATOR IS THE SENTINEL, NOT THE CODE. A peer
		// RST_STREAM(INTERNAL_ERROR) finishes the client stream with an
		// identically-coded *h2.Error (h2/client.go's RSTStreamFrame arm) and
		// MUST keep the 502 + evict disposition below. errors.Is against the
		// exported h2.ErrMalformedTrailers sentinel is what separates them;
		// err.Code == h2.ErrInternalError would silently swallow every peer
		// reset into this arm.
		//
		// ⚠️ THE ERROR IS RETURNED UNWRAPPED, DELIBERATELY. serverStream.dispatch
		// reads the RST code via a BARE TYPE ASSERTION — writeErr.(*Error) — so a
		// fmt.Errorf("...: %w", err) wrap would still satisfy errors.Is at every
		// intermediate layer while falling through to dispatch's default and
		// losing the carried code. Return the original value; do not decorate it.
		//
		// Status:0 mirrors the ctx-cancel sentinel one arm down: no response is
		// finalized, HCM's h2dispatch skips the wire-write and the access-log
		// submission, and the reset is signaled purely by the returned *h2.Error.
		// No IncStatusClass / RecordUpstreamResult here for the same reason the
		// ctx-cancel arm books neither: this is not an upstream HTTP outcome. The
		// sent RST_STREAM is already booked at the capture site via the pool's
		// onTxReset hook, which in THIS tree Inc's cluster.<name>.http2.tx_reset
		// (the reference books the same event under a different name; do not go
		// looking for the reference's spelling here).
		if errors.Is(err, h2.ErrMalformedTrailers) {
			return ActionResponse{Status: 0}, picked, err
		}
		// 43.2a Task 7: a RoundTrip transport error means the pooled conn is
		// poisoned — evict it from the pool so a later acquire does not ride a
		// broken conn. The deferred release() still accounts THIS stream's slot;
		// eviction only removes the conn from the reuse set (idempotent + safe
		// with our stream still in-flight: makeRelease re-checks Closed()+inFlight
		// and the gauge dec is permit-conserving / once-only).
		a.cluster.EvictH2ConnOnError(cc, ep)
		// Distinguish caller-side ctx-cancel/deadline (→ stream-scoped CANCEL
		// surfaced upward as *h2.Error so serverStream.dispatch emits
		// RST_STREAM(CANCEL)) from any other error (→ 502 local reply). The
		// ctx-vs-other discrimination is unchanged by the 43.2a pool rewire.
		if ctx.Err() != nil && (errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
			// status=0 → H2 ctx-cancel sentinel per SPEC §2.1 last bullet.
			// HCM's h2dispatch access-log emit skips submission on status==0;
			// serverStream.dispatch reads err.Code == ErrCancel and emits
			// RST_STREAM(CANCEL).
			return ActionResponse{Status: 0}, picked, h2.NewStreamError(h2.ErrCancel, 0, "upstream roundtrip: ctx canceled")
		}
		a.cluster.IncStatusClass(502)
		a.cluster.RecordUpstreamResult(picked, cluster.UpstreamResult{StatusCode: 502, LocalOriginErr: true})
		// 42.1 Task 8: localOrigin:true marks this as a router-synthesized
		// ROUNDTRIP failure (upstream reset/protocol error — NOT a proxied 502),
		// so a connect-failure retry_on matches. NOT set on the ctx-cancel
		// Status:0 return above (a client cancel is never a connect failure).
		return ActionResponse{Status: 502, Body: []byte(bad502Body), Headers: h2LocalReplyHeaders(), localOrigin: true}, picked, nil
	}

	a.cluster.IncStatusClass(resp.Status)
	a.cluster.RecordUpstreamResult(picked, cluster.UpstreamResult{StatusCode: resp.Status})

	// Convert h2 hpack header fields into OrderedHeaders for the chain's encode
	// side. Pseudo-headers (:status etc.) are stripped — the chain works on
	// regular headers; HCM h2dispatch's wire-write emits :status from
	// resp.Status before the regular header set. Phase 07.1 Task 19 (I-3
	// prereq): the carrier is OrderedHeaders so insertion order from the H2
	// HPACK-decoded sequence survives through the chain to the wire (H2 codec
	// preserves wire order on resp.Headers per RFC 9113 §8.1.2 — slice already
	// carries the correct order).
	respHeaders := make(envoyhttp.OrderedHeaders, 0, len(resp.Headers))
	for _, hf := range resp.Headers {
		if strings.HasPrefix(hf.Name, ":") {
			continue
		}
		respHeaders = append(respHeaders, envoyhttp.HeaderField{Name: hf.Name, Value: hf.Value})
	}
	// 84.1 Task 5 (ADR-0306): THE ONE populate site. resp.Trailers is the
	// validated trailing HEADERS block captured by h2.RoundTrip (nil when the
	// upstream sent none — the zero value IS "no trailers", so this assignment
	// is unconditional and the no-trailers control still observes an empty
	// carrier). The fields ride through as hpack.HeaderField, NOT projected
	// onto OrderedHeaders like the leading block above: a trailer section has
	// no pseudo-headers to strip and does not pass through the encode chain.
	// Every other ActionResponse return in this file — and in retry.go /
	// hedge.go / router_weighted.go, which propagate the struct BY VALUE with
	// no edit — leaves Trailers nil.
	return ActionResponse{
		Status:   resp.Status,
		Headers:  respHeaders,
		Body:     resp.Body,
		Trailers: resp.Trailers,
	}, picked, nil
}

// h2LocalReplyHeaders builds the standard local-reply header set for
// synthesized 5xx H2 responses. Mirrors the Header set previously written by
// routerActionH2.write502. Phase 07.1 Task 19 (I-3 prereq): returns
// OrderedHeaders so the action-driven wire-emit path preserves the three-
// header insertion order (Content-Type, Date, Server) on the H2 HEADERS frame.
func h2LocalReplyHeaders() envoyhttp.OrderedHeaders {
	return envoyhttp.OrderedHeaders{
		{Name: "Content-Type", Value: "text/plain"},
		{Name: "Date", Value: dateHeader()},
		{Name: "Server", Value: serverHeader()},
	}
}

// h2HeaderVal returns a codec-agnostic headerVal accessor over the H2 request's
// hpack fields for applyHashKey. req.Headers is []hpack.HeaderField with
// codec-lowercased Name; EqualFold for case-insensitive robustness. Returns
// the FIRST matching value (single-value producer path per D-S362-4; the full
// multi-value fold lives in cluster.HashHeaderValues).
func h2HeaderVal(req h2.H2Request) func(name string) (string, bool) {
	return func(name string) (string, bool) {
		for _, hf := range req.Headers {
			if strings.EqualFold(hf.Name, name) {
				return hf.Value, true
			}
		}
		return "", false
	}
}

// routerActionH2 is the H2-flavored router action. Selected at filter-build
// time when the resolved cluster's UseH2() reports true (per SPEC §5.5 +
// §4.1). The struct mirrors routerAction in shape; the live H2 dispatch path
// (H2ClusterAction → doH2ClusterAction) acquires a multiplexed stream slot on a
// POOLED upstream H2 conn via Cluster.AcquireH2Stream (phase 43.2a, ADR-0253 —
// superseding ADR-0056's per-request fresh dial; the legacy per-request-fresh
// doH2 driver was removed in Task 7 once both fresh-dial sites were gone).
//
// Failure-class mapping per SPEC §11.9:
//
//	Cluster.DialH2 error        → 502 local reply (downstream :status 502 + bad502Body)
//	RoundTrip ctx-cancel        → RST_STREAM(CANCEL) on the downstream
//	RoundTrip protocol error    → 502 local reply
//	Upstream HTTP status (5xx)  → forwarded verbatim (NOT translated)
//
// Per ADR-0058: REQUEST trailers (downstream client → envoy-go) are observed
// and discarded — that half is unchanged, and the discard remains in
// serverStream.recvTrailingHeaders. RESPONSE trailers (upstream → envoy-go)
// are a different story as of phase 84.1: h2.RoundTrip (h2/client.go)
// captures the trailing HEADERS block, validates it against the RFC 9113
// §8.2.2 set, and populates it onto ActionResponse.Trailers at the ONE
// populate site above (:250). The router itself does not decide how the
// block reaches the wire — h2dispatch.go's writeH2Reply conditionally holds
// END_STREAM off the response HEADERS/DATA and emits the trailers as a
// second, trailing HEADERS frame when ActionResponse.Trailers is non-empty;
// with no trailers, END_STREAM still lands on the response HEADERS (bodyless
// case) or the final DATA frame exactly as before.
//
// The live H2 dispatch runs through the H2ClusterAction closure →
// doH2ClusterAction (a package function, not a method). The legacy defensive
// do(...) stub (routeAction-interface shape for an H2-cluster route reached
// on an H1 path) was deleted along with the direct-write do path: variant
// selection at filter-build time guarantees H2-clusters get the H2 closure,
// and the hcm routeAction interface no longer carries a do method.
type routerActionH2 struct {
	cluster      *cluster.Cluster
	hashPolicies []HashPolicy        // 36.2: stored at H2ClusterAction; per-request fold lands in Task 4 (applyHashKey).
	subsetMatch  cluster.SubsetMatch // 38.1: route-static metadata_match threaded onto ctx at dispatch (ADR-0239).
	rp           *RetryPolicy        // 42.1: effective retry_policy; nil when none. Stored here; the retry loop lands in Task 8.
	hp           *HedgePolicy        // 42.2b: effective hedge_policy; nil when none. Read by hedgeExecutorH2 (the concurrent first-acceptable-wins racer); the H1 sibling field is live via hedgeExecutorH1.
}
