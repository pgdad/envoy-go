// internal/filter/hcm/h2dispatch.go — adapter from hcm package to h2 sub-package.
//
// This file is in package hcm (NOT in package h2), which is the correct
// direction for the one-way import: hcm → h2 only. The h2 package MUST NOT
// import internal/filter/hcm; this file is the seam that resolves the import
// topology per PLAN "Settled SPEC §10 deferred decisions" #10.
package hcm

import (
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
//   - direct_response route → h2DirectResponseAdapter wrapping the action.
//   - no-match (table miss) → h2DirectResponseAdapter with a 404 synthesis.
//   - non-direct_response route (e.g. routerAction) → h2RouterActionRejection,
//     which returns ErrInternalError on WriteH2 (SPEC §5.2 step 4c).
//
// ok=false is never returned because the match engine itself cannot fail;
// a Dispatcher returning ok=false would cause the dispatch helper to emit
// RST_STREAM(INTERNAL_ERROR), which is correct for a genuine engine failure
// but not for the expected "no match" case.
func (d *h2Dispatcher) Match(req *http.Request) (h2.Action, bool) {
	entry, ok := d.table.match(req)
	if !ok {
		// No matching route — synthesise 404.
		return &h2DirectResponseAdapter{a: &directResponseAction{status: 404, bodyText: "not found\n"}}, true
	}
	if dr, ok := entry.action.(*directResponseAction); ok {
		return &h2DirectResponseAdapter{a: dr}, true
	}
	// Non-direct_response action on H2 path (e.g. routerAction): return the
	// sentinel that triggers per-stream INTERNAL_ERROR + RST_STREAM in
	// stream.dispatch (SPEC §5.2 step 4c).
	return &h2RouterActionRejection{}, true
}

// h2DirectResponseAdapter wraps a *directResponseAction as an h2.Action.
// WriteH2 delegates to a.a.writeH2(sw) which was factored out in Task 10.
type h2DirectResponseAdapter struct {
	a *directResponseAction
}

func (a *h2DirectResponseAdapter) WriteH2(sw h2.StreamWriter) error {
	return a.a.writeH2(sw)
}

// h2RouterActionRejection is a sentinel h2.Action returned when the matched
// route action is a routerAction (or any non-direct_response action).
// WriteH2 returns ErrInternalError so stream.dispatch emits RST_STREAM(INTERNAL_ERROR)
// per SPEC §5.2 step 4c.
type h2RouterActionRejection struct{}

func (r *h2RouterActionRejection) WriteH2(_ h2.StreamWriter) error {
	// Pass stream ID 0 — the dispatch helper reads only the Code field when
	// deciding which ErrCode to use for the RST_STREAM; the stream ID in the
	// error is informational only.
	return h2.NewStreamError(h2.ErrInternalError, 0, "router action on h2 listener (SPEC §5.2 step 4c)")
}
