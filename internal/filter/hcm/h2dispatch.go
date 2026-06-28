// internal/filter/hcm/h2dispatch.go — adapter from hcm package to h2 sub-package.
//
// This file is in package hcm (NOT in package h2), which is the correct
// direction for the one-way import: hcm → h2 only. The h2 package MUST NOT
// import internal/filter/hcm; this file is the seam that resolves the import
// topology per PLAN "Settled SPEC §10 deferred decisions" #10.

package hcm

import (
	"context"
	stdtls "crypto/tls"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/tracing"
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

	// tlsPrincipals is the priority-ordered TLS principal-name candidate
	// slice extracted once at connection build time by runH2 via
	// downstreamTLSPrincipals(downstream). nil for plaintext / non-mTLS /
	// no-client-cert connections. Threaded into every per-stream chain via
	// chainDispatchAction.tlsPrincipals → chain.SetTLSPrincipals before
	// RunDecodeHeaders dispatch. Phase 16 Task 6 (ADR-0144 §Decision (iii)).
	tlsPrincipals []string

	// tlsConnectionState is the FULL *tls.ConnectionState extracted once at
	// connection build time by runH2 via downstreamTLSConnectionState(downstream).
	// nil for plaintext / non-*tls.Conn / pre-handshake connections. Threaded
	// into every per-stream chain via chainDispatchAction.tlsConnectionState →
	// chain.SetTLSConnectionState before RunDecodeHeaders dispatch — symmetric
	// to tlsPrincipals plumbing.
	//
	// Per ADR-0192 §Decision body anticipation + SPEC §11.5.3 (chain-side
	// extension lives INSIDE ADR-0192 per Q13 WEAK HOLD). All H2 streams on
	// this conn share the same connection-snapshot pointer — mirrors the H1
	// path's per-request extraction (the H1 conn is also pinned to a single
	// TLS state for the connection lifetime). Per-conn caching avoids the
	// per-stream ConnectionState() call cost on the many-streams-per-conn
	// H2 shape.
	tlsConnectionState *stdtls.ConnectionState

	// Phase 18.2 Task 4 (ADR-0165): the 4 per-connection callback-surface-
	// extension fields below carry per-stream state captured ONCE at H2
	// connection build time by runH2 — symmetric to tlsPrincipals (the H1
	// connection is also pinned to a single conn-state for the connection
	// lifetime; per-conn caching mirrors that semantic for the H2 path's
	// many-streams-per-conn shape). Threaded into every per-stream chain via
	// chainDispatchAction.{downstreamRemoteAddr/...} → chain.SetX before
	// RunDecodeHeaders dispatch. Zero-value semantics match the chain
	// fields (nil net.Addr / empty string / nil DER bytes for plaintext or
	// no-client-cert connections). The listener-principal is the SAME
	// per-listener string for all conns on this Filter — sourced from
	// f.listenerPrincipal at runH2 time below.
	downstreamRemoteAddr     net.Addr
	downstreamLocalAddr      net.Addr
	downstreamTLSServerName  string
	downstreamTLSPeerCertDER []byte
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
			f:                        d.f,
			action:                   notFound.asRouterActionH2(),
			req:                      req,
			routeIdx:                 -1,
			status:                   404,
			tlsPrincipals:            d.tlsPrincipals,
			tlsConnectionState:       d.tlsConnectionState,
			downstreamRemoteAddr:     d.downstreamRemoteAddr,
			downstreamLocalAddr:      d.downstreamLocalAddr,
			downstreamTLSServerName:  d.downstreamTLSServerName,
			downstreamTLSPeerCertDER: d.downstreamTLSPeerCertDER,
		}, true
	}

	return &chainDispatchAction{
		f:                        d.f,
		action:                   entry.action.asRouterActionH2(),
		req:                      req,
		routeIdx:                 routeIdx,
		tlsPrincipals:            d.tlsPrincipals,
		tlsConnectionState:       d.tlsConnectionState,
		downstreamRemoteAddr:     d.downstreamRemoteAddr,
		downstreamLocalAddr:      d.downstreamLocalAddr,
		downstreamTLSServerName:  d.downstreamTLSServerName,
		downstreamTLSPeerCertDER: d.downstreamTLSPeerCertDER,
		// Phase 24.1 Task 5 (ADR-0198 / DELTA-2): thread the matched route's
		// raw rate_limits[] from the routeEntry into the per-stream dispatch
		// envelope. The vhost-level slice is sourced directly from
		// c.f.table.vhostRateLimits at WriteH2 time (no per-stream snapshot
		// needed — the single-vhost discipline pins it for the Filter
		// lifetime). nil for routes with no rate_limits[] (the zero-regression
		// path; chain accessor returns nil).
		routeRateLimits: entry.rateLimits,
		// Phase 24.2 Task 1 (D-RL8): thread the matched route's metadata
		// for the ratelimit filter's metadata descriptor action under
		// MetadataSource_ROUTE_ENTRY=1. nil-passthrough when the route has
		// no metadata field.
		routeMetadata: entry.metadata,
		// Phase 24.2 Task 4 (D-RL11): thread the matched route's legacy
		// `RouteAction.include_vh_rate_limits` bool for the ratelimit
		// filter's §4.3 Axis-B force-include arm. false-passthrough when
		// the route does not carry the legacy bool.
		routeIncludeVhRateLimits: entry.includeVhRateLimits,
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
	// tlsPrincipals is the priority-ordered TLS principal-name candidate slice
	// from the parent h2Dispatcher (extracted once at connection build time by
	// runH2). nil for plaintext / non-mTLS / no-client-cert connections.
	// Threaded into the per-stream chain via chain.SetTLSPrincipals before
	// RunDecodeHeaders dispatch. Phase 16 Task 6 (ADR-0144 §Decision (iii)).
	tlsPrincipals []string

	// tlsConnectionState is the FULL *tls.ConnectionState snapshot from the
	// parent h2Dispatcher (extracted once at connection build time by runH2 via
	// downstreamTLSConnectionState). nil for plaintext / non-*tls.Conn /
	// pre-handshake conns. Threaded into the per-stream chain via
	// chain.SetTLSConnectionState before RunDecodeHeaders dispatch — symmetric
	// to tlsPrincipals plumbing per ADR-0192 §Decision body anticipation +
	// SPEC §11.5.3.
	tlsConnectionState *stdtls.ConnectionState

	// Phase 18.2 Task 4 (ADR-0165): the 4 per-connection state fields below
	// thread from the parent h2Dispatcher (captured at runH2 connection-build
	// time) into per-stream chain.SetX seeders below in WriteH2. The
	// listener-principal is NOT captured here — it is sourced from f.listenerPrincipal
	// directly (the same per-Filter listener value for every conn on this
	// listener). The downstream-protocol is the H2 canonical "HTTP/2" — also
	// not captured per-conn since it is invariant for the H2 dispatch path.
	downstreamRemoteAddr     net.Addr
	downstreamLocalAddr      net.Addr
	downstreamTLSServerName  string
	downstreamTLSPeerCertDER []byte

	// Phase 24.1 Task 5 (ADR-0198 / DELTA-2): the matched route's raw
	// rate_limits[] threaded from h2Dispatcher.Match's routeEntry capture
	// into chain.SetRouteRateLimits below in WriteH2. The vhost-level slice
	// is NOT captured here — it is sourced from c.f.table.vhostRateLimits
	// directly at WriteH2 time (the single-vhost discipline pins it for the
	// Filter lifetime; no per-stream snapshot needed). nil for routes with
	// no rate_limits[] (the zero-regression path) and for the no-match-route
	// 404 path (the no-chain branch elides the seed entirely).
	routeRateLimits []*routev3.RateLimit

	// Phase 24.2 Task 1 (D-RL8): the matched route's *corev3.Metadata
	// threaded from h2Dispatcher.Match's routeEntry capture into
	// chain.SetRouteMetadata below in WriteH2. Same dispatch-envelope
	// discipline as routeRateLimits above. Consumed by the ratelimit
	// filter's `metadata` descriptor action under MetadataSource_ROUTE_ENTRY=1.
	routeMetadata *corev3.Metadata

	// Phase 24.2 Task 4 (D-RL11): the matched route's legacy
	// `RouteAction.include_vh_rate_limits` bool threaded from
	// h2Dispatcher.Match's routeEntry capture into
	// chain.SetRouteIncludeVhRateLimits below in WriteH2. Same dispatch-
	// envelope discipline as routeRateLimits/routeMetadata above. Consumed by
	// the ratelimit filter's §4.3 Axis-B vhost-walk composition table per
	// parent SPEC §4.3 + AMEND-5 (the legacy force-include override).
	routeIncludeVhRateLimits bool
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

	// Phase 36.2 (ADR-0237): carry the downstream remote addr on the LIVE ctx
	// that threads into BOTH H2 action-invoke points (the no-match
	// c.action(ctx, h2req) below + the matched rf.RunAction(ctx)) → the router
	// action's applyHashKey source_ip producer. h2.H2Request carries no remote
	// addr (D-S362-3), so the source_ip hash_policy reads it from ctx. No-op
	// when no route configures a source_ip hash_policy (value goes unread);
	// guarded for the nil-downstreamRemoteAddr unit-test path.
	if c.downstreamRemoteAddr != nil {
		ctx = router.WithDownstreamRemoteAddr(ctx, c.downstreamRemoteAddr.String())
	}

	// No-match path: skip chain construction; invoke the synthesized 404
	// action directly + emit access-log + write wire bytes. Mirrors H1
	// connection.go's dispatchRequest no-match branch. Phase 07.1 Task 18
	// prereq P1: action returns ActionResponse; we serialize wire bytes here.
	if c.routeIdx < 0 {
		resp, picked, err := c.action(ctx, h2req)
		if err == nil && resp.Status > 0 {
			err = writeH2Reply(sw, resp.Status, resp.Headers, resp.Body)
		}
		c.f.emitAccessLogH2(h2req, resp.Status, int64(len(resp.Body)), picked, startTime, resp.Headers)
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
	// Phase 16 Task 6 (ADR-0144): seed the per-stream TLS principal-name
	// candidates BEFORE RunDecodeHeaders dispatch (symmetric to the H1 path
	// in connection.go's dispatchRequest). The candidates were extracted
	// once at connection build time by runH2 → h2Dispatcher.tlsPrincipals →
	// chainDispatchAction.tlsPrincipals here; nil for plaintext / non-mTLS /
	// no-client-cert connections.
	chain.SetTLSPrincipals(c.tlsPrincipals)
	// Phase 22.2 Task 6 (ADR-0192): seed the per-stream FULL *tls.ConnectionState
	// BEFORE RunDecodeHeaders dispatch — symmetric to SetTLSPrincipals above and
	// to the H1 path in connection.go's dispatchRequest. The snapshot was
	// extracted once at H2 connection-build time by runH2 →
	// h2Dispatcher.tlsConnectionState → chainDispatchAction.tlsConnectionState
	// here; nil for plaintext / non-*tls.Conn / pre-handshake conns. Per
	// SPEC §11.5.3 + ADR-0192 §Decision body anticipation (chain-side extension
	// lives INSIDE ADR-0192 per Q13 WEAK HOLD).
	//
	// dynamicMetadata is NOT seeded here — chain.go's NewFilterChain
	// constructor initializes it at chain build time per ADR-0190 + ADR-0192
	// §Decision body anticipation (chain.go owns the bucket lifecycle). HCM
	// does NOT touch dynamicMetadata directly.
	chain.SetTLSConnectionState(c.tlsConnectionState)
	// Phase 18.2 Task 4 (ADR-0165): seed the 6 per-stream callback-surface
	// extension fields BEFORE RunDecodeHeaders dispatch — symmetric to the
	// H1 path in connection.go. RemoteAddr/LocalAddr/SNI/PeerCertDER come
	// from the per-connection h2Dispatcher capture in runH2 (threaded via
	// chainDispatchAction fields). Protocol is "HTTP/2" canonical for this
	// dispatch path. listener-principal is the per-Filter listener value
	// pre-extracted by the listener manager.
	chain.SetDownstreamRemoteAddr(c.downstreamRemoteAddr)
	chain.SetDownstreamLocalAddr(c.downstreamLocalAddr)
	chain.SetDownstreamTLSServerName(c.downstreamTLSServerName)
	chain.SetDownstreamTLSPeerCertDER(c.downstreamTLSPeerCertDER)
	chain.SetDownstreamProtocol("HTTP/2")
	chain.SetListenerPrincipal(c.f.listenerPrincipal)
	// Phase 24.1 Task 5 (ADR-0198 / DELTA-2): seed the matched route's + the
	// parent vhost's raw []*routev3.RateLimit policy slices onto the per-stream
	// FilterChain BEFORE RunDecodeHeaders. Mirrors the H1 path's seed in
	// connection.go and the ADR-0165 SetDownstreamRemoteAddr set-once-by-
	// dispatch discipline; the single-dispatch-goroutine invariant per ADR-0071
	// applies. nil-passthrough when either slice is empty.
	chain.SetRouteRateLimits(c.routeRateLimits)
	chain.SetVirtualHostRateLimits(c.f.table.vhostRateLimits)
	// Phase 24.2 Task 1 (D-RL8): seed the matched route's *corev3.Metadata
	// onto the chain BEFORE RunDecodeHeaders for the ratelimit filter's
	// `metadata` descriptor action under MetadataSource_ROUTE_ENTRY=1. nil-
	// passthrough when the route has no metadata (zero-regression).
	chain.SetRouteMetadata(c.routeMetadata)
	// Phase 24.2 Task 4 (D-RL11): seed the matched route's legacy
	// `RouteAction.include_vh_rate_limits` bool onto the chain BEFORE
	// RunDecodeHeaders so the ratelimit filter's §4.3 Axis-B composition table
	// can honor the legacy force-include override (per parent SPEC §4.3 +
	// AMEND-5). Mirrors the SetRouteMetadata set-once-by-dispatch discipline;
	// false-passthrough when the route does not carry the legacy bool.
	chain.SetRouteIncludeVhRateLimits(c.routeIncludeVhRateLimits)
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
		c.f.emitAccessLogH2(h2req, 500, 0, cluster.Endpoint{}, startTime, nil)
		if cnt := c.f.downstreamStatusClassCounter(500); cnt != nil {
			cnt.Inc()
		}
		return nil
	}
	rf.SetH2Action(c.action)

	// Phase 46.1a Task 9: HCM-native request-tracing dispatch on the H2 path —
	// symmetric to connection.go's H1 seam. The upstream-forwarded H2 headers
	// live on h2req.Headers (regular headers; pseudo-headers are excluded), so
	// the decision is run over an http.Header view built from them, then
	// x-request-id / traceparent / tracestate are written back onto h2req.Headers.
	// CRITICAL: h2req is received BY VALUE — the mutated copy must be re-set via
	// rf.SetH2Request(h2req) BELOW (a single set after this block) or the upstream
	// RoundTrip would ride the un-mutated value-copy. When no provider is
	// configured the block is skipped and the value passes through unchanged
	// (byte-stable no-tracing path).
	if c.f.tracingConfig != nil {
		view := make(http.Header, len(h2req.Headers)+2)
		for _, hf := range h2req.Headers {
			view.Add(hf.Name, hf.Value)
		}
		d := tracing.Decide(view, c.f.tracingConfig, c.f.rng)
		tracing.InjectTraceparent(view, d.TraceID, d.SpanID, d.Sample, d.TraceState)
		h2req.Headers = upsertH2Header(h2req.Headers, "x-request-id", d.RequestID)
		h2req.Headers = upsertH2Header(h2req.Headers, "traceparent", view.Get("Traceparent"))
		if ts := view.Get("Tracestate"); ts != "" {
			h2req.Headers = upsertH2Header(h2req.Headers, "tracestate", ts)
		}
		c.f.tracingCounters.Record(d.Class)
	}

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

	// Phase 12 Task 11 prereq (csrf filter Host-target): inject the request
	// authority as ":authority" pseudo-header on the headers map so chain-
	// level filters (csrf etc.) can read the Host without codec-specific
	// surfacing. The H2 codec strips :authority into a local var during
	// parseHeadersForRequest (h2/stream.go:399); reflect it back onto
	// c.req.Header so RunDecodeHeaders observes it consistent with the H1
	// path. Same wire-emit safety as ":method": no response-emit path
	// iterates c.req.Header.
	if _, ok := c.req.Header[":authority"]; !ok && c.req.Host != "" {
		c.req.Header[":authority"] = []string{c.req.Host}
	}

	// Phase 16 Task 14: inject the request path as ":path" pseudo-header
	// (symmetric with the H1 connection.go path; same rationale — rbac and
	// similar URL-path-consuming filters observe :path consistently across
	// H1 + H2). H2 already carried :path on the HEADERS frame; the codec
	// strips it into the URL during parseHeadersForRequest. Reflect it back
	// for filter consumption. Same wire-emit safety as :method.
	if _, ok := c.req.Header[":path"]; !ok && c.req.URL != nil {
		path := c.req.RequestURI
		if path == "" {
			path = c.req.URL.RequestURI()
		}
		c.req.Header[":path"] = []string{path}
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
		c.f.emitAccessLogH2(h2req, lrStatus, int64(len(lrBody)), cluster.Endpoint{}, startTime, lrHeaders)
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
		// Seed the encode-side response-status accessor (ADR-0196) so encode-
		// side classifying filters (e.g. admission_control) observe resp.Status,
		// which the encode header map does not carry (:status is not present).
		chain.SetEncodeResponseStatus(status)
		if _, err := chain.RunEncodeHeaders(ctx, merged, len(resp.Body) == 0); err != nil {
			c.f.emitAccessLogH2(h2req, status, int64(len(resp.Body)), picked, startTime, resp.Headers)
			return err
		}
		resp.Headers = filter_http.ReconcileOrderedHeaders(resp.Headers, merged)
		if len(resp.Body) > 0 {
			if _, err := chain.RunEncodeData(ctx, resp.Body, true); err != nil {
				c.f.emitAccessLogH2(h2req, status, int64(len(resp.Body)), picked, startTime, resp.Headers)
				return err
			}
		}
		// Symmetric to connection.go H1 path; same primitive harvest per
		// ADR-0131 §Decision (vi). Substitute resp.Body before writeH2Reply
		// emits HEADERS+DATA frames.
		if override, ok := chain.EncodeBodyOverride(); ok {
			resp.Body = override
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
	c.f.emitAccessLogH2(h2req, status, bytesSent, picked, startTime, resp.Headers)

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

// upsertH2Header sets name=value on an H2 regular-header slice: it drops every
// existing case-insensitive match for name, then appends a single lowercase-name
// field carrying value (RFC 9113 §8.2.1 — H2 header field names are lowercase on
// the wire). Used by the Phase 46.1a tracing seam to write the decided
// x-request-id / traceparent / tracestate onto the upstream-forwarded H2Request
// without disturbing the other headers. The in-place filter (out := fields[:0])
// is safe because the kept-element index never outruns the read index.
func upsertH2Header(fields []hpack.HeaderField, name, value string) []hpack.HeaderField {
	out := fields[:0]
	for _, f := range fields {
		if !strings.EqualFold(f.Name, name) {
			out = append(out, f)
		}
	}
	return append(out, hpack.HeaderField{Name: strings.ToLower(name), Value: value})
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
