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
	"strconv"
	"time"

	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/cluster"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
)

// h2Dispatcher implements h2.Dispatcher by delegating to f.table.match. Phase
// 07.1 Task 16: Match no longer wraps a routeAction into an action-specific
// adapter — it returns a single chainDispatchAction that, on WriteH2, builds
// the per-stream *FilterChain, drives decode-side iteration, invokes the
// terminal router filter's RunAction, and emits the access-log record from
// the chain-completion hook (Decision §3.1, mirroring connection.go's
// Task-15 H1 rewrite).
//
// On no-match, Match returns a chainDispatchAction whose action+routeIdx
// reflect the synthesized 404 path (no chain is built; the action writes the
// 404 directly to the H2 stream writer + emits the access-log record). This
// matches the H1 path's no-match branch in connection.go.
type h2Dispatcher struct {
	f *Filter
}

func newH2Dispatcher(f *Filter) *h2Dispatcher {
	return &h2Dispatcher{f: f}
}

// Match resolves an incoming *http.Request to an h2.Action. Returns ok=true
// in all cases:
//   - matched route                → chainDispatchAction wrapping the matched
//     entry's H2 action + routeIdx; WriteH2 drives the per-stream chain.
//   - no-match (table miss)        → chainDispatchAction with a synthesized
//     404 H2 action and routeIdx=-1; WriteH2 writes the 404 directly without
//     running any chain (matches the H1 path's no-match branch in
//     connection.go).
//
// ok=false is never returned because the match engine itself cannot fail; a
// Dispatcher returning ok=false would cause the dispatch helper to emit
// RST_STREAM(INTERNAL_ERROR), which is correct for a genuine engine failure
// but not for the expected "no match" case.
//
// Phase 06.1 Task 11: Match is the once-per-request gateway in the H2 path
// (serverStream.dispatch invokes it after END_STREAM). Inc downstream_rq_total
// here per SPEC §12 #1 site (a)'s H2 analog — once-per-request, before
// route-match resolution.
func (d *h2Dispatcher) Match(req *http.Request) (h2.Action, bool) {
	d.f.downstreamRqTotal.Inc()

	entry, routeIdx, ok := d.f.table.match(req)
	if !ok {
		// No matching route — synthesize 404 with empty body. The chain is
		// NOT built; we surface the 404 via a directResponseAction-equivalent
		// closure (writeH2 invocation) so HCM's chain-completion hook +
		// access-log emit fire on a single uniform shape. routeIdx=-1
		// signals "no chain" to chainDispatchAction.WriteH2.
		notFound := &directResponseAction{status: 404, bodyText: ""}
		return &chainDispatchAction{
			f:        d.f,
			action:   notFound.asRouterActionH2(),
			req:      req,
			routeIdx: -1,
			status:   404,
		}, true
	}

	return &chainDispatchAction{
		f:        d.f,
		action:   entry.action.asRouterActionH2(),
		req:      req,
		routeIdx: routeIdx,
	}, true
}

// chainDispatchAction is the single h2.Action implementation that drives the
// per-stream *FilterChain on the H2 path post-Task-16. The Match call
// resolves the matched route's H2 action closure (or a 404 synth on no-match)
// and packages it here; WriteH2 runs the per-request chain machinery.
//
// Lifecycle (mirrors connection.go's H1 dispatchRequest, Task 15):
//  1. Allocate fresh per-request *HTTPFilter instances from f.chainConfig.
//  2. Build chain via filter_http.NewFilterChain(chainHF, f.perRouteConfig);
//     defer chain.Destroy.
//  3. SetRequestCtx(ctx, routeIdx) on the chain.
//  4. Locate the terminal *router.Filter, inject SetH2Action / SetH2Request /
//     SetH2Writer.
//  5. Run RunDecodeHeaders(req.Header, endStream=...). H2 body is buffered
//     fully before dispatch (h2.serverStream snapshots reqBody before
//     spawning the dispatch goroutine), so we feed it to RunDecodeData in a
//     single chunk after decode-headers if non-empty.
//  6. Invoke rf.RunAction(ctx). This dispatches to the H2Action closure
//     (the matched route's writer logic) which calls sw.WriteHeaders /
//     sw.WriteData and surfaces (status, bytesSent, picked, err).
//  7. Read back the captures; emit access-log via f.emitAccessLogH2 — a
//     no-op when status==0 (H2 ctx-cancel sentinel per SPEC §2.1) or when
//     f.accessLog is empty.
//  8. Inc the HCM-scope downstream_rq_<Nxx> counter once per finalized
//     status. status==0 skips the bucket Inc (matches the ctx-cancel
//     "request did not complete" semantics).
//  9. Return the actionErr. serverStream.dispatch reads *h2.Error to emit
//     the matching RST_STREAM(<code>) on the wire.
//
// No-match path (routeIdx == -1): skip chain construction and run the H2Action
// closure directly. This matches the H1 path in connection.go where 404
// catch-all does not build a chain (no route → no per-route config → no
// terminal action machinery to run). HCM-scope downstream_rq_<Nxx> Inc + the
// access-log emit still fire from the same hook.
type chainDispatchAction struct {
	f        *Filter
	action   router.H2Action
	req      *http.Request
	routeIdx int
	// status pinned at construction time; for no-match path only (404). Unused
	// for the matched-route case (status comes from rf.Status() post-RunAction).
	status int
}

func (c *chainDispatchAction) WriteH2(ctx context.Context, h2req h2.H2Request, sw h2.StreamWriter) error {
	startTime := time.Now()

	// No-match path: skip chain construction; invoke the synthesized 404
	// action directly + emit access-log. Mirrors H1 connection.go's
	// dispatchRequest no-match branch.
	if c.routeIdx < 0 {
		status, bytesSent, picked, err := c.action(ctx, h2req, sw)
		c.f.emitAccessLogH2(h2req, status, bytesSent, picked, startTime)
		if cnt := c.f.downstreamStatusClassCounter(status); cnt != nil {
			cnt.Inc()
		}
		return err
	}

	// Allocate fresh per-request filter instances from chainConfig. Per
	// ADR-0071's two-step factory pattern: each entry's factory is invoked
	// once per request to allocate a new instance bound to its parsed config.
	chainHF := make([]filter_http.HTTPFilter, len(c.f.chainConfig))
	for i, e := range c.f.chainConfig {
		chainHF[i] = e.factory()
	}
	chain := filter_http.NewFilterChain(chainHF, c.f.perRouteConfig)
	chain.SetRequestCtx(ctx, c.routeIdx)
	defer chain.Destroy()

	// Locate the terminal router filter and inject the per-request H2
	// action + req + writer. ValidateChainShape pins the terminal type_url
	// to router.TypeURL and router.New is the only registered factory for
	// that URL; the cast is defensive against future shape changes only.
	rf, ok := chainHF[len(chainHF)-1].Decoder.(*router.Filter)
	if !ok {
		log.Printf("hcm: h2dispatch: terminal filter is not *router.Filter (got %T)", chainHF[len(chainHF)-1].Decoder)
		// Best-effort 500 synthesis on the wire; mirrors the H1 path's
		// dispatchRequest defensive branch.
		_ = c.f.write500H2(sw)
		c.f.emitAccessLogH2(h2req, 500, 0, cluster.Endpoint{}, startTime)
		if cnt := c.f.downstreamStatusClassCounter(500); cnt != nil {
			cnt.Inc()
		}
		return nil
	}
	rf.SetH2Action(c.action)
	rf.SetH2Request(h2req)
	rf.SetH2Writer(sw)

	// Decode side: headers → data → trailers. H2 body is fully buffered
	// before dispatch (h2.serverStream snapshots reqBody before spawning
	// the dispatch goroutine — see stream.go), so we know if a body is
	// present from h2req.Body length and feed it as a single chunk via
	// RunDecodeData with endStream=true. If the body is empty, RunDecodeHeaders
	// fires with endStream=true. RunDecodeTrailers is not invoked: SPEC §2.1
	// observes-and-discards request trailers in the codec layer (per ADR-0058);
	// the FilterChain does not yet expose RunDecodeTrailers (Task 18 will
	// extend if needed for cors/probe).
	hasBody := len(h2req.Body) > 0
	endStreamOnHeaders := !hasBody

	if _, err := chain.RunDecodeHeaders(ctx, c.req.Header, endStreamOnHeaders); err != nil {
		// ctx-cancel or unknown filter status. status==0 → emitAccessLogH2
		// skips submission per SPEC §2.1 sentinel. Surface the err to
		// serverStream.dispatch which decides RST code from *h2.Error.
		return err
	}

	if hasBody {
		if _, err := chain.RunDecodeData(ctx, h2req.Body, true); err != nil {
			return err
		}
	}

	// Terminal-action invocation. Idempotent — if a non-terminal filter
	// triggered SendLocalReply earlier, the chain transitioned to encode
	// mode and rf.actionRan stays false; the rf.h2Action does NOT run, so
	// the captures stay zero-valued and emitAccessLogH2 / counter Inc skip
	// per their status==0 guards. Task-16 H2 path with router-only chain
	// always reaches here with the action populated.
	rf.RunAction(ctx)

	status := rf.Status()
	bytesSent := rf.BytesSent()
	picked := rf.Picked()
	actionErr := rf.ActionErr()

	// Per Decision §3.1: single uniform access-log emit site at chain-completion.
	// emitAccessLogH2 is a no-op when status==0 (H2 ctx-cancel sentinel per
	// SPEC §2.1 last bullet) or when f.accessLog is empty.
	c.f.emitAccessLogH2(h2req, status, bytesSent, picked, startTime)

	// Phase 06.1 Task 11: HCM-scope downstream_rq_<Nxx> Inc on the H2 path
	// per SPEC §5.5 "HCM response hook" row — Inc once per finalized response
	// status. status==0 (ctx-cancel path) skips the bucket Inc since no
	// terminating status was produced.
	if status > 0 {
		if cnt := c.f.downstreamStatusClassCounter(status); cnt != nil {
			cnt.Inc()
		}
	}

	if actionErr != nil {
		// Phase 06.1 M-9 carry-forward (SPEC §13.1). One-line log gives an
		// operator debugging an INTERNAL_ERROR-class RST_STREAM the underlying
		// action error string without spelunking the dispatch carry-error
		// path. Preserved from the pre-Task-16 h2RouterActionAdapter.
		log.Printf("h2: action error: %v", actionErr)
		return actionErr
	}
	return nil
}

// write500H2 emits a minimal 500 Internal Server Error local reply on the
// H2 stream writer. Used only on the defensive non-*router.Filter terminal
// branch above (unreachable in well-formed bootstraps because ValidateChainShape
// pins the terminal type_url to router.TypeURL). Body is empty to match the
// phase-04 H1 convention for synthesized 500s.
func (f *Filter) write500H2(sw h2.StreamWriter) error {
	hdrs := []hpack.HeaderField{
		{Name: ":status", Value: "500"},
		{Name: "date", Value: dateHeader()},
		{Name: "server", Value: serverHeader()},
		{Name: "content-type", Value: "text/plain"},
		{Name: "content-length", Value: strconv.Itoa(0)},
	}
	if err := sw.WriteHeaders(hdrs, true); err != nil {
		return err
	}
	return nil
}
