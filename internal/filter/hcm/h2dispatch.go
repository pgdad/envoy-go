// internal/filter/hcm/h2dispatch.go — adapter from hcm package to h2 sub-package.
//
// This file is in package hcm (NOT in package h2), which is the correct
// direction for the one-way import: hcm → h2 only. The h2 package MUST NOT
// import internal/filter/hcm; this file is the seam that resolves the import
// topology per PLAN "Settled SPEC §10 deferred decisions" #10.

package hcm

import (
	"context"
	"net/http"

	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// h2Dispatcher implements h2.Dispatcher by delegating to *routeTable.match.
// Wraps each matched action into an h2.Action implementation.
type h2Dispatcher struct {
	table *routeTable
}

func newH2Dispatcher(table *routeTable) *h2Dispatcher {
	return &h2Dispatcher{table: table}
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
func (d *h2Dispatcher) Match(req *http.Request) (h2.Action, bool) {
	entry, ok := d.table.match(req)
	if !ok {
		// No matching route — synthesize 404 with empty body (matches phase-04 H1 convention; configured catch-all routes carry their own body).
		return &h2DirectResponseAdapter{a: &directResponseAction{status: 404, bodyText: ""}}, true
	}
	switch a := entry.action.(type) {
	case *directResponseAction:
		return &h2DirectResponseAdapter{a: a}, true
	case *routerActionH2:
		// Phase 05.2: routerActionH2 routes proxy via fresh upstream H2 dial.
		return &h2RouterActionAdapter{a: a}, true
	default:
		// Non-H2 router action on H2 path (e.g. *routerAction). Variant selection
		// at filter-build time (config.go:buildRouterAction) should prevent this
		// in well-formed bootstraps; defensively return INTERNAL_ERROR + RST.
		return &h2RouterActionRejection{}, true
	}
}

// h2DirectResponseAdapter wraps a *directResponseAction as an h2.Action.
// WriteH2 delegates to a.a.writeH2(sw) (the codec-neutral writer factored
// out in 05.1 Task 10). Phase 05.2: WriteH2 ignores ctx + req — direct_response
// synthesizes the reply from its own state.
type h2DirectResponseAdapter struct {
	a *directResponseAction
}

func (a *h2DirectResponseAdapter) WriteH2(_ context.Context, _ h2.H2Request, sw h2.StreamWriter) error {
	return a.a.writeH2(sw)
}

// h2RouterActionAdapter wraps a *routerActionH2 as an h2.Action. WriteH2
// delegates to a.a.doH2(ctx, req, sw) — see actions.go for the H2 driver.
// Per ADR-0058: trailers are observed but not forwarded by the upstream-side
// codec; the adapter does not surface them to the downstream-side writer.
type h2RouterActionAdapter struct {
	a *routerActionH2
}

func (a *h2RouterActionAdapter) WriteH2(ctx context.Context, req h2.H2Request, sw h2.StreamWriter) error {
	return a.a.doH2(ctx, req, sw)
}

// h2RouterActionRejection is a sentinel h2.Action returned when the matched
// route action is neither a *directResponseAction nor a *routerActionH2.
// WriteH2 returns ErrInternalError so stream.dispatch emits RST_STREAM(INTERNAL_ERROR)
// per SPEC §5.2 step 4c. Phase 05.2: WriteH2 ignores ctx + req (the rejection
// is unconditional).
type h2RouterActionRejection struct{}

func (r *h2RouterActionRejection) WriteH2(_ context.Context, _ h2.H2Request, _ h2.StreamWriter) error {
	// Pass stream ID 0 — the dispatch helper reads only the Code field when
	// deciding which ErrCode to use for the RST_STREAM; the stream ID in the
	// error is informational only.
	return h2.NewStreamError(h2.ErrInternalError, 0, "router action on h2 listener (SPEC §5.2 step 4c)")
}
