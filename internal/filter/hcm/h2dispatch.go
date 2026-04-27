// internal/filter/hcm/h2dispatch.go — adapter from hcm package to h2 sub-package.
//
// This file is in package hcm (NOT in package h2), which is the correct
// direction for the one-way import: hcm → h2 only. The h2 package MUST NOT
// import internal/filter/hcm; this file is the seam that resolves the import
// topology per PLAN "Settled SPEC §10 deferred decisions" #10.

package hcm

import (
	"context"
	"log"
	"net/http"

	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// h2Dispatcher implements h2.Dispatcher by delegating to f.table.match.
// Wraps each matched action into an h2.Action implementation.
//
// Phase 06.1 Task 11: holds the *Filter (not just the *routeTable) so the
// per-instance HCM-scope counters can be Inc'd at dispatch entry (per
// SPEC §5.5 + §12 #1 site (a)) and on response status finalization (the
// adapters below carry the Filter reference for the response-class hook).
type h2Dispatcher struct {
	f *Filter
}

func newH2Dispatcher(f *Filter) *h2Dispatcher {
	return &h2Dispatcher{f: f}
}

// Match resolves an incoming *http.Request to an h2.Action. Returns ok=true
// in all cases:
//   - direct_response route        → h2DirectResponseAdapter wrapping the action.
//   - no-match (table miss)        → h2DirectResponseAdapter with a 404 synthesis (empty body, matching phase-04 H1 convention at connection.go).
//   - *routerActionH2 route        → h2RouterActionAdapter wrapping the action (phase 05.2 + SPEC §5.5).
//   - any other route action       → h2RouterActionRejection, which returns ErrInternalError on WriteH2 (SPEC §5.2 step 4c).
//
// ok=false is never returned because the match engine itself cannot fail;
// a Dispatcher returning ok=false would cause the dispatch helper to emit
// RST_STREAM(INTERNAL_ERROR), which is correct for a genuine engine failure
// but not for the expected "no match" case.
//
// Phase 06.1 Task 11: Match is the once-per-request gateway in the H2 path
// (serverStream.dispatch invokes it after END_STREAM). Inc downstream_rq_total
// here per SPEC §12 #1 site (a)'s H2 analog — once-per-request, before
// route-match resolution.
func (d *h2Dispatcher) Match(req *http.Request) (h2.Action, bool) {
	d.f.downstreamRqTotal.Inc()

	entry, ok := d.f.table.match(req)
	if !ok {
		// No matching route — synthesize 404 with empty body (matches phase-04 H1 convention; configured catch-all routes carry their own body).
		return &h2DirectResponseAdapter{a: &directResponseAction{status: 404, bodyText: ""}, f: d.f}, true
	}
	switch a := entry.action.(type) {
	case *directResponseAction:
		return &h2DirectResponseAdapter{a: a, f: d.f}, true
	case *routerActionH2:
		// Phase 05.2: routerActionH2 routes proxy via fresh upstream H2 dial.
		adapter := &h2RouterActionAdapter{a: a, f: d.f}
		// Phase 06.1 Task 11: default-bind doH2Fn to the receiver-bound
		// method so production behavior is unchanged; tests substitute the
		// field to drive the M-9 log-line surface (see h2dispatch_test.go).
		adapter.doH2Fn = a.doH2
		return adapter, true
	default:
		// Non-H2 router action on H2 path (e.g. *routerAction). Variant selection
		// at filter-build time (config.go:buildRouterAction) should prevent this
		// in well-formed bootstraps; defensively return INTERNAL_ERROR + RST.
		return &h2RouterActionRejection{f: d.f}, true
	}
}

// h2DirectResponseAdapter wraps a *directResponseAction as an h2.Action.
// WriteH2 delegates to a.a.writeH2(sw) (the codec-neutral writer factored
// out in 05.1 Task 10). Phase 05.2: WriteH2 ignores ctx + req — direct_response
// synthesizes the reply from its own state.
//
// Phase 06.1 Task 11: f is the parent Filter (held for the response-class
// hook); a.a.status is the finalized status — Inc the matching HCM bucket
// before delegating to writeH2 ("before bytes hit the wire", SPEC §5.5).
type h2DirectResponseAdapter struct {
	a *directResponseAction
	f *Filter
}

func (a *h2DirectResponseAdapter) WriteH2(_ context.Context, _ h2.H2Request, sw h2.StreamWriter) error {
	if c := a.f.downstreamStatusClassCounter(a.a.status); c != nil {
		c.Inc()
	}
	return a.a.writeH2(sw)
}

// h2RouterActionAdapter wraps a *routerActionH2 as an h2.Action. WriteH2
// delegates to a.a.doH2(ctx, req, sw) — see actions.go for the H2 driver.
// Per ADR-0058: trailers are observed but not forwarded by the upstream-side
// codec; the adapter does not surface them to the downstream-side writer.
//
// Phase 06.1 Task 11:
//   - f is the parent Filter held so the adapter can later report the
//     response-class to the HCM-scope counter (see the per-status post-doH2
//     hook in WriteH2 below).
//   - doH2Fn is a function-typed test hook (default-bound to a.a.doH2 by
//     newH2Dispatcher.Match). Tests substitute it to inject sentinel-failing
//     drivers for the M-9 log-line surface (per SPEC §11.4 + §13.1).
//   - The M-9 carry-forward log line ("h2: doH2 error: %v") fires on the
//     doH2 error path before the error propagates upward — addresses the
//     observability gap noted in 05.2 REVIEW M-9.
type h2RouterActionAdapter struct {
	a      *routerActionH2
	f      *Filter
	doH2Fn func(ctx context.Context, req h2.H2Request, sw h2.StreamWriter) (int, error)
}

func (a *h2RouterActionAdapter) WriteH2(ctx context.Context, req h2.H2Request, sw h2.StreamWriter) error {
	fn := a.doH2Fn
	if fn == nil {
		fn = a.a.doH2 // defensive: production path Match always default-binds, but a hand-built adapter (tests) might not.
	}
	status, err := fn(ctx, req, sw)
	// Phase 06.1 Task 11: HCM-scope downstream_rq_<Nxx> Inc on the H2 path
	// per SPEC §5.5 "HCM response hook" row — Inc once per finalized
	// response status. status==0 (ctx-cancel path) skips the bucket Inc
	// since no terminating status was produced.
	if status > 0 {
		if c := a.f.downstreamStatusClassCounter(status); c != nil {
			c.Inc()
		}
	}
	if err != nil {
		// Phase 06.1 Task 11 — M-9 carry-forward (SPEC §13.1). The 05.2
		// REVIEW deferred this to "phase-06 observability when logging /
		// metrics surface lands" (REVIEW.md M-9). One-line log to stderr
		// gives an operator debugging an INTERNAL_ERROR-class RST_STREAM
		// the underlying doH2 error string without spelunking the
		// dispatch carry-error path.
		log.Printf("h2: doH2 error: %v", err)
		return err
	}
	return nil
}

// h2RouterActionRejection is a sentinel h2.Action returned when the matched
// route action is neither a *directResponseAction nor a *routerActionH2.
// WriteH2 returns ErrInternalError so stream.dispatch emits RST_STREAM(INTERNAL_ERROR)
// per SPEC §5.2 step 4c. Phase 05.2: WriteH2 ignores ctx + req (the rejection
// is unconditional).
//
// Phase 06.1 Task 11: f is the parent Filter held for symmetry with the
// other adapters; the rejection is conceptually a 500-class outcome so
// Inc downstream_rq_5xx before returning the INTERNAL_ERROR.
type h2RouterActionRejection struct {
	f *Filter
}

func (r *h2RouterActionRejection) WriteH2(_ context.Context, _ h2.H2Request, _ h2.StreamWriter) error {
	if c := r.f.downstreamStatusClassCounter(500); c != nil {
		c.Inc()
	}
	// Pass stream ID 0 — the dispatch helper reads only the Code field when
	// deciding which ErrCode to use for the RST_STREAM; the stream ID in the
	// error is informational only.
	return h2.NewStreamError(h2.ErrInternalError, 0, "router action on h2 listener (SPEC §5.2 step 4c)")
}
