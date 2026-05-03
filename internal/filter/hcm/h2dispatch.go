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
	"strings"
	"time"

	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
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
// Lifecycle (mirrors connection.go's H1 dispatchRequest, Task 15 + Task 18):
//  1. Allocate fresh per-request *HTTPFilter instances from f.chainConfig.
//  2. Build chain via filter_http.NewFilterChain(chainHF, f.perRouteConfig);
//     defer chain.Destroy.
//  3. SetRequestCtx(ctx, routeIdx) on the chain.
//  4. Locate the terminal *router.Filter, inject SetH2Action / SetH2Request.
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
	// Phase 08.2 Task 9: per-stream Inc/Dec inflight per ADR-0096. WriteH2 is
	// called once per H2 stream (= one request), so the defer fires at stream-
	// end (= function return) ensuring pair-balance under all exit paths. The
	// markedInflight sentinel prevents Dec from firing without a matching Inc
	// (per ADR-0096 + ADR-0075). Access-log emit fires inside WriteH2 at its
	// respective emitAccessLogH2 calls, so Dec occurs after access-log is
	// recorded per the task spec.
	var markedInflight bool
	if c.f.dm != nil {
		c.f.dm.Inc()
		markedInflight = true
	}
	defer func() {
		if markedInflight {
			c.f.dm.Dec()
			markedInflight = false
		}
	}()

	startTime := time.Now()

	// No-match path: skip chain construction; invoke the synthesized 404
	// action directly + emit access-log + write wire bytes. Mirrors H1
	// connection.go's dispatchRequest no-match branch. Phase 07.1 Task 18
	// prereq P1: action returns ActionResponse; we serialize wire bytes here.
	if c.routeIdx < 0 {
		resp, picked, err := c.action(ctx, h2req)
		if err == nil && resp.Status > 0 {
			err = writeH2Reply(sw, resp.Status, resp.Headers, resp.Body)
		}
		c.f.emitAccessLogH2(h2req, resp.Status, int64(len(resp.Body)), picked, startTime)
		if cnt := c.f.downstreamStatusClassCounter(resp.Status); cnt != nil {
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

	// Phase 07.1 Task 18 prereq: inject the request method as ":method"
	// pseudo-header on the headers map so chain-level filters (cors etc.)
	// can read the method without codec-specific surfacing. H2 routes already
	// place :method on the request struct via h2.parseHeadersForRequest;
	// reflect it onto the *http.Request.Header here so RunDecodeHeaders
	// observes it consistent with the H1 path. ":method" is left on
	// c.req.Header for the request lifetime; no wire-emit path observes
	// pseudo-headers (verified via writeH1Reply/writeH2Reply iterating only
	// response headers — c.req.Header is decode-side only). Use raw-map
	// access: http.Header.Get canonicalizes the key via
	// textproto.CanonicalMIMEHeaderKey, which does not preserve the
	// leading colon and would not see the colon-prefixed pseudo-header.
	if _, ok := c.req.Header[":method"]; !ok {
		c.req.Header[":method"] = []string{c.req.Method}
	}

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

	// Phase 07.1 Task 18 prereq P2: SendLocalReply path on the H2 codec.
	// Mirrors the H1 path in connection.go: if a non-terminal filter (cors)
	// triggered SendLocalReply, the chain ran the encode chain over the
	// synthesized response; pull it out via LocalReplyResponse and emit
	// HEADERS+DATA via writeH2Reply.
	if chain.LocalReplyDone() {
		lrStatus, lrHeaders, lrBody := chain.LocalReplyResponse()
		var werr error
		if lrStatus > 0 {
			// Task 18 review fix + Task 19 unification: ordered carrier
			// preserves caller insertion order (e.g. SPEC §11.2 6-header pin
			// from cors.go) on the H2 HEADERS frame.
			werr = writeH2Reply(sw, lrStatus, lrHeaders, lrBody)
		}
		c.f.emitAccessLogH2(h2req, lrStatus, int64(len(lrBody)), cluster.Endpoint{}, startTime)
		if lrStatus > 0 {
			if cnt := c.f.downstreamStatusClassCounter(lrStatus); cnt != nil {
				cnt.Inc()
			}
		}
		return werr
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

	resp := rf.Response()
	picked := rf.Picked()
	actionErr := rf.ActionErr()
	status := resp.Status

	// Phase 07.1 Task 18 prereq P2: run the response through the encode chain
	// so encode-side filters (cors etc.) can mutate headers/body BEFORE the
	// HEADERS+DATA frames hit the wire. Reverse declaration order per SPEC
	// §5.5 + §11.1. Phase 07.1 Task 19 (I-3 prereq): project resp.Headers
	// (OrderedHeaders) → http.Header for the encode chain (which still
	// operates on http.Header per ADR-0071's filter API stability), then
	// reconcile post-encode mutations back onto the OrderedHeaders carrier
	// via filter_http.ReconcileOrderedHeaders. Caller-supplied insertion
	// order survives encode mutations; net-new keys (cors's encode-side
	// allow-origin/allow-credentials/expose-headers append) sort alphabetical
	// after the original carrier.
	if rf.ActionRan() && status > 0 && actionErr == nil {
		merged := resp.Headers.ToHTTPHeader()
		if _, err := chain.RunEncodeHeaders(ctx, merged, len(resp.Body) == 0); err != nil {
			c.f.emitAccessLogH2(h2req, status, int64(len(resp.Body)), picked, startTime)
			return err
		}
		resp.Headers = filter_http.ReconcileOrderedHeaders(resp.Headers, merged)
		if len(resp.Body) > 0 {
			if _, err := chain.RunEncodeData(ctx, resp.Body, true); err != nil {
				c.f.emitAccessLogH2(h2req, status, int64(len(resp.Body)), picked, startTime)
				return err
			}
		}
	}

	// Phase 07.1 Task 18 prereq P2: wire-write the (post-encode-chain) response
	// via writeH2Reply. Skipped on ctx-cancel (status==0) and on the rare
	// SendLocalReply-during-decode path (rf.ActionRan==false; the chain's
	// beginLocalReply wrote the response through the encode chain inline but
	// did NOT emit wire bytes — that is THIS layer's responsibility on H2 too).
	if rf.ActionRan() && actionErr == nil && status > 0 {
		if werr := writeH2Reply(sw, resp.Status, resp.Headers, resp.Body); werr != nil {
			actionErr = werr
		}
	}

	bytesSent := int64(len(resp.Body))

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

// writeH2Reply emits an HTTP/2 response on sw from a pre-built ordered header
// set + body. Phase 07.1 Task 18 prereq P2: the chain-mediated H2 dispatch
// path serializes the action's (post-encode-chain-mutated) response here.
// Phase 07.1 Task 19 (I-3 prereq): unified path — the SendLocalReply path and
// action-driven path BOTH use this single helper (collapsed from
// writeH2Reply{,Ordered} duplication). Iterates headers as a slice so caller-
// supplied insertion order (SPEC §11.2 verbatim 6-header order from cors.go
// on the SendLocalReply path; the H2 codec's wire-order on the upstream path)
// lands on the wire byte-for-byte.
//
// Header emission order:
//  1. :status pseudo-header (RFC 9113 §8.3 — pseudo-headers must come first).
//  2. The headers slice in iteration order (lowercased per RFC 9113 §8.1.2).
//     Content-Length is overridden inline to match len(body).
//  3. date, server defaults appended if absent in the carrier.
//
// HEADERS frame is emitted with end_stream=false (body follows) when
// len(body)>0; end_stream=true when body is empty.
func writeH2Reply(sw h2.StreamWriter, status int, headers filter_http.OrderedHeaders, body []byte) error {
	hf := []hpack.HeaderField{{Name: ":status", Value: strconv.Itoa(status)}}
	hasServer := false
	hasDate := false
	for _, h := range headers {
		if h.Name == "" {
			continue
		}
		ln := strings.ToLower(h.Name)
		val := h.Value
		if ln == "content-length" {
			val = strconv.Itoa(len(body))
		}
		if ln == "server" {
			hasServer = true
		}
		if ln == "date" {
			hasDate = true
		}
		hf = append(hf, hpack.HeaderField{Name: ln, Value: val})
	}
	if !hasServer {
		hf = append(hf, hpack.HeaderField{Name: "server", Value: serverHeader()})
	}
	if !hasDate {
		hf = append(hf, hpack.HeaderField{Name: "date", Value: dateHeader()})
	}
	endStream := len(body) == 0
	if err := sw.WriteHeaders(hf, endStream); err != nil {
		return err
	}
	if !endStream {
		if err := sw.WriteData(body, true); err != nil {
			return err
		}
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
