package hcm

import (
	"bufio"
	"bytes"
	"context"
	stdtls "crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/pgdad/envoy-go/internal/cluster"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
	"github.com/pgdad/envoy-go/internal/tracing"
)

// extractTLSPrincipals returns the priority-ordered TLS principal-name
// candidates from the supplied tls.ConnectionState: URI SANs first (highest
// priority — SPIFFE / service identity), then DNS SANs (DNS identity), then
// the Subject DN Common Name as the third-priority fallback. Returns nil
// for handshake-incomplete states OR states carrying no client cert
// (PeerCertificates length == 0; covers plaintext / non-mTLS / no-client-
// cert dispositions per ADR-0143 §Decision (vi) case (c)).
//
// Mirrors Envoy v1.37.2's Principal_Authenticated extraction semantics per
// rbac.pb.go:1432-1438. Per ADR-0144 §Decision (iii) + D11: canonical 3 cert
// fields only — Issuer DN / Serial / fingerprints DEFERRED to a future
// TLS-context-extension phase.
//
// Cross-phase reuse: future HTTP filters (jwt_authn / ext_authz / oauth2 /
// ext_proc) consume the same helper through DecoderFilterCallbacks.
// DownstreamPrincipal() per ADR-0144 §Consequences.
func extractTLSPrincipals(state *stdtls.ConnectionState) []string {
	if state == nil || !state.HandshakeComplete || len(state.PeerCertificates) == 0 {
		return nil
	}
	cert := state.PeerCertificates[0]
	var principals []string
	// URI SANs first (highest priority — SPIFFE / service identity).
	for _, u := range cert.URIs {
		if u == nil {
			continue
		}
		principals = append(principals, u.String())
	}
	// DNS SANs second (DNS identity).
	principals = append(principals, cert.DNSNames...)
	// Subject DN Common Name third (fallback). Empty CN intentionally
	// omitted — an empty-string candidate would otherwise match a Prefix("")
	// matcher unexpectedly.
	if cn := cert.Subject.CommonName; cn != "" {
		principals = append(principals, cn)
	}
	return principals
}

// downstreamTLSPrincipals returns the priority-ordered TLS principal-name
// candidates extracted from the downstream connection's *tls.Conn, or nil
// if the conn is not a *tls.Conn / handshake incomplete / no client cert.
// Wrapper around extractTLSPrincipals that handles the net.Conn type
// assertion. Per ADR-0144 §Decision (iii); used by both H1 (connection.go
// dispatchRequest) and H2 (h2dispatch.go via the per-connection dispatcher)
// paths.
func downstreamTLSPrincipals(downstream net.Conn) []string {
	tc, ok := downstream.(*stdtls.Conn)
	if !ok {
		return nil
	}
	state := tc.ConnectionState()
	return extractTLSPrincipals(&state)
}

// downstreamTLSConnectionState returns the FULL *tls.ConnectionState extracted
// from the downstream connection's *tls.Conn, or nil if:
//   - the conn is nil OR not a *tls.Conn (plaintext listener / test path), OR
//   - the conn IS a *tls.Conn but the handshake has not completed (defensive —
//     production code reaches dispatchRequest only after the codec layer has
//     read at least one byte off the conn, which implies handshake completion
//     for a *tls.Conn; but tests can drive the helper with a pre-handshake
//     state and the bridge surface MUST NOT observe an unverified handshake).
//
// Per ADR-0192 §Decision body anticipation + SPEC §11.5.3 (chain-side
// tlsConnectionState extension lives INSIDE ADR-0192 per Q13 WEAK HOLD).
// Mirrors the downstreamTLSPrincipals plumbing pattern (type-assert *tls.Conn,
// read ConnectionState by value, &state out the address) — same shape, same
// nil-tolerance discipline. Returns a fresh pointer to a heap-allocated copy
// of the conn-state value so the chain field is stable for the request
// lifetime (the *tls.Conn's internal state may continue to evolve on later
// renegotiations / session-resumption events; the chain holds the value
// snapshotted at dispatch time).
//
// Distinct from downstreamTLSPrincipals in ONE semantic axis: this helper
// does NOT gate on len(PeerCertificates)==0. Server-auth-only TLS (no client
// cert) produces a usable *tls.ConnectionState — SNI / cipher suite / version
// are valid even without a client cert. tlsPrincipals (ADR-0144) requires
// mTLS by contrast; tlsConnectionState (ADR-0192) is the full handshake state
// for the 12 lua bridge ssl methods per SPEC §11.5.4.
//
// Used by both H1 (connection.go dispatchRequest) and H2 (h2dispatch.go via
// the per-connection dispatcher) paths — symmetric to downstreamTLSPrincipals.
func downstreamTLSConnectionState(downstream net.Conn) *stdtls.ConnectionState {
	tc, ok := downstream.(*stdtls.Conn)
	if !ok {
		return nil
	}
	state := tc.ConnectionState()
	if !state.HandshakeComplete {
		return nil
	}
	return &state
}

// runConnection drives one downstream HTTP/1.1 connection from acceptance to
// close. The loop reads requests via http.ReadRequest off a bufio.Reader
// over downstream, applies phase-04 out-of-scope guards (Expect:→417,
// Upgrade:→501), dispatches each request through the route table + chain,
// and exits on:
//   - clean EOF from the downstream
//   - any non-EOF parse error (a 400 is sent before close)
//   - phase-04-out-of-scope guard trip (417 or 501 before close)
//   - Connection: close on the request OR the response signaling
//     errCloseAfterAction
//
// On TLS listeners with ALPN, this driver is reached via filter.Handle's
// codec dispatch when the negotiated ALPN is "http/1.1" (or the codec
// is set to HTTP1 explicitly); see ADR-0050 and SPEC §5.4 for the
// dispatch contract.
//
// Per SPEC §5.3 / §5.6 / SPEC §10 #3 settled.
//
// Phase 06.1 Task 11: f's per-instance HCM counters are incremented on
// dispatch entry (downstream_rq_total) and on response status finalization
// (downstream_rq_<Nxx>) per SPEC §5.5 + §12 #1 site (a). The dispatch-entry
// hook fires once per successfully-parsed request; the status-class hook
// fires once per response (including the 400/404/417/501/503/etc. local-
// reply paths so the integer-divide-by-100 discipline observes every
// served response).
//
// Phase 07.1 Task 15: the per-request dispatch is mediated through a
// *filter_http.FilterChain allocated per request from f.chainConfig. The
// chain runs the decode side; the terminal router filter holds the resolved
// route action (built at config.go's buildAction → asRouterAction()) and
// invokes it after decode iteration completes. Access-log emit fires from
// chain-completion (Decision §3.1), reading status / bytes / picked from the
// router filter's per-request capture fields.
func runConnection(ctx context.Context, downstream net.Conn, f *Filter) {
	defer func() { _ = downstream.Close() }()

	br := bufio.NewReader(downstream)
	bw := bufio.NewWriter(downstream)

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		req, err := http.ReadRequest(br)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_ = writeStatusReply(bw, 400, "")
				_ = bw.Flush()
				// Parse-error is a synthesized 400 response — a real served
				// response, so Inc downstream_rq_total + the 4xx class
				// counter to keep observability honest (matches Envoy's
				// "request observed" semantics on the parse-error path).
				f.downstreamRqTotal.Inc()
				if c := f.downstreamStatusClassCounter(400); c != nil {
					c.Inc()
				}
			}
			return
		}

		// Per SPEC §12 #1 site (a): on first byte of request line/headers in
		// connection.go's read loop. http.ReadRequest succeeded → this is the
		// once-per-request dispatch-entry hook.
		f.downstreamRqTotal.Inc()

		// Phase 08.2 Task 9: per-request Inc/Dec inflight per ADR-0096.
		// serveOneRequest wraps the per-request body so defer fires at
		// request-end (not connection-end). Returns keepAlive=false when
		// the connection must be closed after this request.
		//
		// Phase 16 Task 6 (ADR-0144): downstream is threaded through to
		// dispatchRequest so the H1 path can extract TLS principal
		// candidates from the *tls.Conn's ConnectionState and seed them
		// onto the per-stream chain via chain.SetTLSPrincipals before
		// RunDecodeHeaders dispatch.
		if keepAlive := f.serveOneRequest(ctx, downstream, req, bw); !keepAlive {
			return
		}
	}
}

// serveOneRequest handles one successfully-parsed HTTP/1.1 request on the
// H1 path. It is called from runConnection's loop after http.ReadRequest
// succeeds and downstreamRqTotal.Inc fires. Returns keepAlive=true when the
// connection should be kept alive for the next request, false when the
// connection should be closed.
//
// Phase 08.2 Task 9: the per-request *drain.Manager Inc/Dec pair lives here
// so the deferred Dec fires at request-end (= function return) rather than
// at connection-end. The markedInflight sentinel ensures Dec never fires
// without a matching Inc (per ADR-0096 + ADR-0075). Placement: after
// downstreamRqTotal.Inc (parse succeeded), before the filter chain runs
// (dispatchRequest). Access-log emit fires inside dispatchRequest, so Dec
// occurs after access-log is recorded per the task spec.
func (f *Filter) serveOneRequest(ctx context.Context, downstream net.Conn, req *http.Request, bw *bufio.Writer) (keepAlive bool) {
	var markedInflight bool
	if f.dm != nil {
		f.dm.Inc()
		markedInflight = true
	}
	defer func() {
		if markedInflight {
			f.dm.Dec()
			markedInflight = false
		}
	}()

	if req.Header.Get("Expect") != "" {
		_ = writeStatusReply(bw, 417, "")
		_ = bw.Flush()
		if c := f.downstreamStatusClassCounter(417); c != nil {
			c.Inc()
		}
		drainAndClose(req)
		return false
	}
	if req.Header.Get("Upgrade") != "" || strings.EqualFold(req.Header.Get("Connection"), "Upgrade") {
		_ = writeStatusReply(bw, 501, "")
		_ = bw.Flush()
		if c := f.downstreamStatusClassCounter(501); c != nil {
			c.Inc()
		}
		drainAndClose(req)
		return false
	}

	closeAfterRequest := strings.EqualFold(req.Header.Get("Connection"), "close")
	closeAfterAction := false

	status, actErr := f.dispatchRequest(ctx, downstream, req, bw)
	if errors.Is(actErr, errCloseAfterAction) || errors.Is(actErr, router.ErrCloseAfterAction) {
		closeAfterAction = true
	} else if actErr != nil {
		log.Printf("hcm: action: %v", actErr)
		// Inc the response-class counter for whatever the action
		// finalized before the writer error: status was already set
		// by dispatchRequest, so the integer-divide bucket reflects
		// what HCM produced even when the downstream flush failed.
		if c := f.downstreamStatusClassCounter(status); c != nil {
			c.Inc()
		}
		_ = bw.Flush()
		drainAndClose(req)
		return false
	}

	// Response status finalized (status is set on every code path inside
	// dispatchRequest: 404 catch-all, action result). Inc the bucket per
	// SPEC §5.5 "switch on response status class → downstream_rq_<Nxx>.Inc()
	// once per response". Lives BEFORE bw.Flush per SPEC's "before bytes
	// hit the wire" — Inc'ing post-Flush would skew on flush-error.
	if c := f.downstreamStatusClassCounter(status); c != nil {
		c.Inc()
	}

	if err := bw.Flush(); err != nil {
		drainAndClose(req)
		return false
	}

	drainAndClose(req)

	if closeAfterRequest || closeAfterAction {
		return false
	}
	return true
}

// dispatchRequest runs one request through the per-request *FilterChain. Per
// PLAN Task 15 + Decision §3.1:
//
//  1. Match the route. If no match: synthesize 404 directly (the chain has
//     not been built yet; we don't run any filter machinery for a no-match).
//  2. Build the per-request router.Action from the matched route's action
//     via routeAction.asRouterAction().
//  3. Allocate a fresh *FilterChain from f.chainConfig (one fresh instance
//     per chainEntry's factory). Inject the action + writer + request into
//     the terminal router filter. Set the request ctx + routeIdx on the
//     chain.
//  4. Run RunDecodeHeaders(req.Header, endStream=...). If the request has a
//     body, stream it into RunDecodeData. If trailers, run RunDecodeTrailers.
//  5. Invoke the router filter's RunAction (the terminal-action invocation
//     logically sits "after" the decode chain). The action writes the
//     response wire bytes through bw and surfaces (status, bytesSent, picked,
//     err) on the router filter.
//  6. Emit the access-log record from the router filter's captured outcome
//     (Decision §3.1's single uniform site).
//
// Returns (status, err) so runConnection can Inc the downstream_rq_<Nxx>
// counter and detect the close-after-action sentinel. status is meaningful
// even when err is non-nil (the action populates status before the writer
// error path).
func (f *Filter) dispatchRequest(ctx context.Context, downstream net.Conn, req *http.Request, bw *bufio.Writer) (int, error) {
	// Phase 46.1b Task 8: frame-local tracing decision pointer. Declared at the
	// TOP of dispatchRequest so it is in scope at every emitAccessLog call site.
	// Stays nil until the tracing block below runs (pre-Decide emit sites pass nil
	// automatically; post-Decide sites receive the captured &d).
	var traceDecision *tracing.Decision

	entry, routeIdx, ok := f.table.match(req)
	if !ok {
		// 404 catch-all: phase-04 byte-preserved synthesis with empty body
		// (matches the connection.go pre-Task-15 wire shape). The chain is
		// NOT allocated for a no-match — there is no route → no per-route
		// config → no terminal action to run; the legacy direct synthesis
		// is the byte-equivalent path. Access-log emit also fires here per
		// Decision §3.1 (the "no-match" terminal state). traceDecision is
		// nil here (PRE-Decide path) — no span for a 404 no-match.
		start := time.Now()
		err := writeStatusReply(bw, 404, "")
		f.emitAccessLog(req, 404, 0, cluster.Endpoint{}, start, nil, traceDecision, nil)
		return 404, err
	}

	// Build the per-request router.Action closure from the matched route's
	// action shape. Both directResponseAction + clusterRouteAction satisfy
	// asRouterAction(); the closure encapsulates the writeH1 / cluster-dial
	// logic so the terminal router filter can invoke it without knowing the
	// action variant.
	action := entry.action.asRouterAction()

	// Allocate fresh per-request filter instances from chainConfig. The
	// chainConfig is the resolved http_filters[] in declaration order; each
	// entry's factory is invoked once per request to allocate a new instance
	// (per ADR-0071's two-step factory pattern). The terminal entry is the
	// router filter (validated at parseFilterWithCtx → ValidateChainShape).
	chainHF := make([]filter_http.HTTPFilter, len(f.chainConfig))
	for i, e := range f.chainConfig {
		chainHF[i] = e.factory()
	}
	chain := filter_http.NewFilterChain(chainHF, f.perRouteConfig)
	chain.SetRequestCtx(ctx, routeIdx)
	// Snapshot the downstream TLS connection state ONCE per request. The three
	// TLS-derived chain seeds below (principals, full conn-state, SNI/peer-cert)
	// previously each type-asserted the conn and copied the large
	// tls.ConnectionState struct (re-walking PeerCertificates) — one snapshot
	// derives all three with identical values. nil for plaintext / non-*tls.Conn
	// (including the nil-downstream unit-test path).
	var tlsState *stdtls.ConnectionState
	if tc, ok := downstream.(*stdtls.Conn); ok {
		state := tc.ConnectionState()
		tlsState = &state
	}
	// Phase 16 Task 6 (ADR-0144): seed the per-stream TLS principal-name
	// candidates BEFORE RunDecodeHeaders dispatch so decode-side filters
	// (rbac, future jwt_authn / ext_authz / oauth2 / ext_proc) reading
	// dcb.DownstreamPrincipal() observe the priority-ordered URI SAN →
	// DNS SAN → Subject DN CN list. Returns nil for plaintext / non-mTLS /
	// no-client-cert connections (the chain field stays nil; the accessor
	// returns nil per ADR-0143 §Decision (vi) case (c)).
	chain.SetTLSPrincipals(extractTLSPrincipals(tlsState))
	// Phase 22.2 Task 6 (ADR-0192): seed the per-stream FULL *tls.ConnectionState
	// BEFORE RunDecodeHeaders dispatch — symmetric to SetTLSPrincipals above per
	// SPEC §11.5.3 + ADR-0144 plumbing-pattern extension. The state powers the
	// phase-22.2 lua bridge's 12 ssl methods (PEM-encoded certs, cipher suite,
	// TLS version, etc. per SPEC §11.5.4) and is observable from all decode +
	// encode filters via {decoder,encoder}CB.DownstreamTLSConnectionState().
	// Stays nil for plaintext / non-*tls.Conn / pre-handshake — the documented
	// nil-passthrough so the chain field stays nil and consumers nil-tolerate
	// per ADR-0085 (same HandshakeComplete gate as the downstreamTLSConnectionState
	// helper the H2 path uses). Per Q13 WEAK HOLD the chain-side extension lives
	// INSIDE ADR-0192; no separate ADR.
	//
	// dynamicMetadata is NOT seeded here — chain.go's NewFilterChain
	// constructor initializes it at chain build time per ADR-0190 + ADR-0192
	// §Decision body anticipation (chain.go owns the bucket lifecycle). HCM
	// does NOT touch dynamicMetadata directly.
	if tlsState != nil && tlsState.HandshakeComplete {
		chain.SetTLSConnectionState(tlsState)
	}
	// Phase 18.2 Task 4 (ADR-0165): seed the 6 per-stream callback-surface
	// extension fields BEFORE RunDecodeHeaders dispatch — the cross-phase
	// reusable framework primitives for ext_authz gRPC-mode AttributeContext
	// population (and future ext_proc / global_ratelimit filters). All 6
	// mirror the SetTLSPrincipals seed-and-read discipline; the
	// listener-principal is sourced from f.listenerPrincipal (pre-extracted
	// by the listener manager at chain-build time from the chain's leaf TLS
	// cert via extractListenerPrincipal — empty for plaintext chains). The
	// downstream-protocol is the H1 canonical "HTTP/1.1".
	//
	// Nil-downstream guard: chain-dispatch unit tests
	// (TestDispatchRequest_ChainInvocationOrder etc.) pass downstream=nil
	// because they exercise the chain machinery directly without a real
	// listener — the chain fields stay zero (nil net.Addr, "" SNI / cert
	// DER) per the zero-value semantics documented on the chain struct.
	if downstream != nil {
		chain.SetDownstreamRemoteAddr(downstream.RemoteAddr())
		chain.SetDownstreamLocalAddr(downstream.LocalAddr())
		// Phase 36.2 (ADR-0237): carry the downstream remote addr on the LIVE
		// ctx that threads into rf.RunAction(ctx) → the router action's
		// applyHashKey source_ip producer. *http.Request.RemoteAddr is NOT
		// populated on the HCM codec path (D-S362-3), so the source_ip
		// hash_policy reads the addr from ctx instead. No-op when no route
		// configures a source_ip hash_policy (ctx value simply goes unread).
		ctx = router.WithDownstreamRemoteAddr(ctx, downstream.RemoteAddr().String())
	}
	chain.SetDownstreamProtocol("HTTP/1.1")
	chain.SetListenerPrincipal(f.listenerPrincipal)
	// Phase 24.1 Task 5 (ADR-0198 / DELTA-2): seed the matched route's + the
	// parent vhost's raw []*routev3.RateLimit policy slices onto the per-stream
	// FilterChain BEFORE RunDecodeHeaders. Mirrors the ADR-0165
	// SetDownstreamRemoteAddr et al. set-once-by-dispatch discipline; the
	// single-dispatch-goroutine invariant per ADR-0071 applies. nil-passthrough
	// when either slice is empty — the chain accessor returns nil and the
	// ratelimit filter's engine reads zero policies (zero-regression path).
	chain.SetRouteRateLimits(entry.rateLimits)
	chain.SetVirtualHostRateLimits(f.table.vhostRateLimits)
	// Phase 24.2 Task 1 (D-RL8): seed the matched route's *corev3.Metadata onto
	// the chain BEFORE RunDecodeHeaders so the ratelimit filter's `metadata`
	// descriptor action (MetadataSource_ROUTE_ENTRY=1) can read it through
	// decoderCB.RouteMetadata(). Mirrors the SetRouteRateLimits / SetX set-
	// once-by-dispatch discipline; nil-passthrough when the route has no
	// metadata (zero-regression).
	chain.SetRouteMetadata(entry.metadata)
	// Phase 24.2 Task 4 (D-RL11): seed the matched route's legacy
	// `RouteAction.include_vh_rate_limits` bool onto the chain BEFORE
	// RunDecodeHeaders so the ratelimit filter's §4.3 Axis-B composition table
	// can honor the legacy force-include override (per parent SPEC §4.3 +
	// AMEND-5). Mirrors the SetRouteMetadata set-once-by-dispatch discipline;
	// false-passthrough when the route does not carry the legacy bool.
	chain.SetRouteIncludeVhRateLimits(entry.includeVhRateLimits)
	if tlsState != nil {
		chain.SetDownstreamTLSServerName(tlsState.ServerName)
		if len(tlsState.PeerCertificates) > 0 {
			chain.SetDownstreamTLSPeerCertDER(tlsState.PeerCertificates[0].Raw)
		}
	}
	defer chain.Destroy()

	// Locate the terminal router filter instance and inject the per-request
	// action + req + bw. The chain validation guarantees the last entry is
	// the router filter; we cast its Decoder side to *router.Filter (the
	// concrete type returned by router.New's per-instance factory) and set
	// the Action / Request / Writer fields.
	rf, ok := chainHF[len(chainHF)-1].Decoder.(*router.Filter)
	if !ok {
		// Defensive: should never happen in well-formed bootstraps because
		// ValidateChainShape pins the terminal type_url to router.TypeURL,
		// and router.New is the only registered factory for that URL.
		// Synthesize 500 + log; the bw write is best-effort. traceDecision
		// is nil here (PRE-Decide path) — no span for this synthetic 500.
		log.Printf("hcm: dispatchRequest: terminal filter is not *router.Filter (got %T)", chainHF[len(chainHF)-1].Decoder)
		start := time.Now()
		err := writeStatusReply(bw, 500, "")
		f.emitAccessLog(req, 500, 0, cluster.Endpoint{}, start, nil, traceDecision, chain.DynamicMetadata().Get)
		return 500, err
	}
	rf.SetAction(action)
	rf.SetRequest(req)

	startTime := time.Now()

	// Phase 07.1 Task 18 prereq: inject the request method as ":method"
	// pseudo-header on the headers map so chain-level filters (cors etc.)
	// can read the method without codec-specific surfacing. ":method" is
	// left on req.Header for the request lifetime; no wire-emit path
	// observes pseudo-headers (verified via writeH1Reply/writeH2Reply
	// iterating only response headers — req.Header is decode-side only).
	// Use raw-map access: http.Header.Get canonicalizes the key via
	// textproto.CanonicalMIMEHeaderKey, which does not preserve the
	// leading colon and would not see the colon-prefixed pseudo-header.
	if _, ok := req.Header[":method"]; !ok {
		req.Header[":method"] = []string{req.Method}
	}

	// Phase 12 Task 11 prereq (csrf filter Host-target): inject the request
	// authority as ":authority" pseudo-header on the headers map so chain-
	// level filters (csrf etc.) can read the Host without codec-specific
	// surfacing. http.ReadRequest strips the Host header off req.Header and
	// stores it on req.Host (per stdlib documentation), so a filter reading
	// req.Header.Get("Host") OR req.Header.Get(":authority") would otherwise
	// see "" on the H1 path. We mirror the H2 codec's :authority population
	// (h2/stream.go:341 case ":authority":) so chain-level filters observe
	// a consistent request-Host signal across both H1 and H2. Same wire-emit
	// safety as ":method": no response-emit path iterates req.Header.
	if _, ok := req.Header[":authority"]; !ok && req.Host != "" {
		req.Header[":authority"] = []string{req.Host}
	}

	// Phase 16 Task 14: inject the request path as ":path" pseudo-header on
	// the headers map so chain-level filters (rbac etc.) can read the URL
	// path without codec-specific surfacing. http.ReadRequest parses the H1
	// request-line and exposes the path via req.URL.Path; we mirror it onto
	// req.Header[":path"] symmetric with the :method + :authority
	// injection. Same wire-emit safety: no response-emit path iterates
	// req.Header.
	if _, ok := req.Header[":path"]; !ok && req.URL != nil {
		// RequestURI preserves the raw path + query as it appeared on the
		// wire (mirrors H2 codec's :path discipline per RFC 7540 §8.1.2.3);
		// fall back to URL.Path when RequestURI is empty (server-side parsed
		// requests carry RequestURI; client-side-constructed requests may
		// have only URL.Path populated).
		path := req.RequestURI
		if path == "" {
			path = req.URL.RequestURI()
		}
		req.Header[":path"] = []string{path}
	}

	// Phase 46.1a Task 9: HCM-native request-tracing dispatch. When a `tracing`
	// provider is configured (f.tracingConfig != nil), run the sampling/request-id
	// decision over the inbound headers, stamp-or-generate x-request-id, inject the
	// W3C traceparent (+ tracestate), and Inc the matching HCM tracing.* counter.
	// The mutation lands on req.Header BEFORE the decode chain + the terminal
	// router action — so the decision propagates UPSTREAM via req.Write (revising
	// the router.go:763 "router injects no x-request-id/traceparent" invariant:
	// the HCM tracing engine, not the router, does the injection). When no
	// provider is configured this whole block is skipped — the no-tracing path
	// stays byte-stable (no x-request-id / traceparent added; no counter moves).
	//
	// Phase 46.1b Task 8: capture the decision into the frame-local pointer so
	// the post-Decide emitAccessLog call sites below can pass it to the span-emit
	// block. The pointer stays nil when no tracing is configured (byte-stable).
	if f.tracingConfig != nil {
		// Phase 46.2 Task 9: provider-aware extract/inject (D-TRACE-ZIPKIN-DECIDE-SEAM).
		// Zipkin reads/writes the B3 multi-header set; OTel reads/writes the W3C
		// traceparent. The DecideWithContext seam runs the shared §11 precedence over
		// whichever inbound context the provider extracted.
		var ic tracing.TraceContext
		var ok bool
		if f.tracingConfig.Provider == tracing.ProviderZipkin {
			ic, ok = tracing.ExtractB3(req.Header)
		} else {
			ic, ok = tracing.ExtractTraceparent(req.Header)
		}
		d := tracing.DecideWithContext(req.Header, ic, ok, f.tracingConfig, f.rng)
		req.Header.Set("X-Request-Id", d.RequestID)
		if f.tracingConfig.Provider == tracing.ProviderZipkin {
			tracing.InjectB3(req.Header, d, f.tracingConfig.Zipkin.TraceID128Bit, f.tracingConfig.Zipkin.SharedSpanContext)
		} else {
			tracing.InjectTraceparent(req.Header, d.TraceID, d.SpanID, d.Sample, d.TraceState)
		}
		f.tracingCounters.Record(d.Class)
		traceDecision = &d
	}

	// Decode side: headers → data → trailers. endStream on RunDecodeHeaders is
	// true when the request has no body (req.Body == nil || req.Body ==
	// http.NoBody || req.ContentLength == 0). Per ADR-0076 the chain may
	// transition to a 413 local-reply on decode-side body overflow; we
	// handle that path below by reading chain state.
	hasBody := req.Body != nil && req.Body != http.NoBody && req.ContentLength != 0
	// HTTP/1.1 request trailers via stdlib http.Request.Trailer are populated
	// only after the body has been fully read with chunked transfer-encoding;
	// the Phase-04..07.1 fixture set does not exercise H1 trailers, and the
	// FilterChain does not yet expose a RunDecodeTrailers method (Task 18 will
	// add it for the cors/envoygotest filters). For Task 15 we DO NOT branch
	// on req.Trailer; endStream lands on the headers (no body) or on the last
	// data chunk. This matches the byte-preserved phase-04 H1 wire output.
	endStreamOnHeaders := !hasBody

	if _, err := chain.RunDecodeHeaders(ctx, req.Header, endStreamOnHeaders); err != nil {
		// ctx-cancel or unknown filter status. status==0 skips the access-log
		// emit (matches the H2 ctx-cancel sentinel discipline per SPEC §2.1).
		return 0, err
	}

	// Phase 07.1 Task 18 prereq P2: SendLocalReply path. If a non-terminal
	// filter (e.g. cors) called dcb.SendLocalReply during decode, the chain
	// has already run the encode chain over the synthesized response inside
	// beginLocalReply (per ADR-0075). Pull the (post-mutation) response shape
	// out of the chain via LocalReplyResponse and write wire bytes via
	// writeH1Reply. Bypasses RunAction (the terminal action does NOT run on
	// the local-reply path).
	if chain.LocalReplyDone() {
		lrStatus, lrHeaders, lrBody := chain.LocalReplyResponse()
		bytesSent := int64(len(lrBody))
		var werr error
		if lrStatus > 0 {
			// Task 18 review fix + Task 19 unification: SendLocalReply path
			// uses the unified ordered helper so SPEC §11.2's verbatim
			// 6-header order survives on the wire (alphabetical sort via
			// http.Header.Write would lose the §11.2 order).
			werr = writeH1Reply(bw, lrStatus, lrHeaders, lrBody)
		}
		// Phase 46.1b Task 8: POST-Decide site — pass traceDecision so the
		// span block in emitAccessLog fires when sampling is active.
		f.emitAccessLog(req, lrStatus, bytesSent, cluster.Endpoint{}, startTime, lrHeaders, traceDecision, chain.DynamicMetadata().Get)
		// Honor any user-supplied Connection: close on the local-reply
		// headers (the 413 overflow path sets this; cors preflight does not).
		if strings.EqualFold(lrHeaders.Get("Connection"), "close") {
			if werr == nil {
				werr = errCloseAfterAction
			}
		}
		return lrStatus, werr
	}

	if hasBody {
		// Phase 07.1 Task 22: buffer the body bytes as we feed them to the
		// chain so the terminal router action's `req.Write(upstream)` sees a
		// readable req.Body again. Without this, the body bytes were drained
		// into RunDecodeData and the upstream got headers + zero body bytes
		// despite a non-zero Content-Length, causing the upstream backend to
		// hang waiting for body bytes that never came (manifesting as 502 on
		// every POST request — exposed by fixture 0007b's modes 3 + 5 when
		// they POST a body and either return 200 (mode 3) or 418 (mode 5)).
		//
		// The buffering is unconditional in this branch — every POST/PUT
		// request with a body goes through chain.RunDecodeData (so any
		// filter's DecodeData callback fires) AND the buffered bytes go
		// to the upstream via the restored req.Body. Phase 04 streamed the
		// body directly through req.Write; Task 15 introduced the chain
		// drain; Task 22 closes the gap.
		//
		// Buffer cap is filter_http.FilterBufferLimitBytes (1 MiB per
		// ADR-0076) — overflow synthesizes a 413 inside RunDecodeData (per
		// the §11 #3 empirical pin), so this branch never sees an
		// over-cap body in practice.
		buf := make([]byte, 32*1024)
		var bodyBuf []byte
		lastEndStreamFired := false
		for {
			n, rerr := req.Body.Read(buf)
			endStreamOnData := rerr != nil
			if n > 0 {
				bodyBuf = append(bodyBuf, buf[:n]...)
				if _, derr := chain.RunDecodeData(ctx, buf[:n], endStreamOnData); derr != nil {
					return 0, derr
				}
				if endStreamOnData {
					lastEndStreamFired = true
				}
			}
			if rerr != nil {
				break
			}
		}
		// If the final Read returned (0, io.EOF) — no data chunk with
		// endStream=true was ever sent to the chain — fire a synthetic
		// empty-terminal chunk so filters that finalize on endStream (e.g.
		// the buffer filter's maybeAddContentLength) observe end-of-stream.
		if !lastEndStreamFired {
			if _, derr := chain.RunDecodeData(ctx, nil, true); derr != nil {
				return 0, derr
			}
		}
		// Restore req.Body so the downstream router's req.Write(upstream)
		// can stream the body to the upstream cluster. Use a bytes.Reader
		// wrapped in io.NopCloser since req.Write reads whatever Reader
		// req.Body wraps.
		req.Body = io.NopCloser(bytes.NewReader(bodyBuf))

		// Phase 13 / buffer filter CL-injection reconciliation: req.Write uses
		// req.ContentLength and req.TransferEncoding (not req.Header) to decide
		// whether to emit Content-Length or Transfer-Encoding: chunked on the
		// wire. The buffer filter's maybeAddContentLength mutates req.Header
		// (which it receives as the DecodeHeaders headers argument), so we
		// propagate the mutation back to the request struct fields here so
		// req.Write sends the injected Content-Length (and not chunked) to the
		// upstream backend.
		//
		// Condition: Content-Length was set by a filter AND Transfer-Encoding was
		// dropped by the same filter (both mutations are atomic in
		// maybeAddContentLength per buffer_filter.cc:91-97). No-op for requests
		// that already had a Content-Length (req.ContentLength >= 0 stays).
		if clStr := req.Header.Get("Content-Length"); clStr != "" && req.ContentLength < 0 {
			if cl, err := strconv.ParseInt(clStr, 10, 64); err == nil {
				req.ContentLength = cl
				req.TransferEncoding = nil
			}
		}
	}

	// Phase 07.1 Task 22: SendLocalReply path on DecodeData. If a non-terminal
	// filter (e.g. envoygotest in local-reply-decode-data mode) called
	// dcb.SendLocalReply during the body loop above, the chain transitioned
	// to encode mode and the encode chain ran synchronously inside
	// beginLocalReply. We mirror the post-RunDecodeHeaders branch (handled at
	// line ~268) here so the terminal action does NOT dial the upstream
	// cluster — the synthesized response shape is already on the chain.
	if chain.LocalReplyDone() {
		lrStatus, lrHeaders, lrBody := chain.LocalReplyResponse()
		bytesSent := int64(len(lrBody))
		var werr error
		if lrStatus > 0 {
			werr = writeH1Reply(bw, lrStatus, lrHeaders, lrBody)
		}
		// Phase 46.1b Task 8: POST-Decide site — pass traceDecision.
		f.emitAccessLog(req, lrStatus, bytesSent, cluster.Endpoint{}, startTime, lrHeaders, traceDecision, chain.DynamicMetadata().Get)
		if strings.EqualFold(lrHeaders.Get("Connection"), "close") {
			if werr == nil {
				werr = errCloseAfterAction
			}
		}
		return lrStatus, werr
	}

	// Terminal-action invocation: the router filter's RunAction surfaces the
	// logical ActionResponse (Phase 07.1 Task 18 prereq P1). Idempotent — if
	// a non-terminal filter triggered SendLocalReply earlier in the decode
	// chain, the chain transitioned to encode mode and the terminal action
	// is NOT invoked (rf.ActionRan stays false). For the Task-15 H1 path
	// with router-only chain, RunAction always fires.
	rf.RunAction(ctx)

	resp := rf.Response()
	picked := rf.Picked()
	actionErr := rf.ActionErr()

	// Phase 07.1 Task 18 prereq P2: run the response through the encode chain
	// so encode-side filters (cors etc.) can mutate headers/body BEFORE the
	// wire-write fires. RunEncodeHeaders iterates filters in REVERSE
	// declaration order per SPEC §5.5 + §11.1; cors's EncodeHeaders mutates
	// headers in place via the http.Header map. Phase 07.1 Task 19 (I-3
	// prereq): project resp.Headers (OrderedHeaders) → http.Header for the
	// encode chain, then reconcile post-encode mutations back via
	// filter_http.ReconcileOrderedHeaders. Caller-supplied insertion order
	// survives encode-chain mutations; net-new keys (cors's encode-side
	// 3-header append on the actual-request path) sort alphabetical after the
	// original carrier.
	status := resp.Status
	if rf.ActionRan() && status > 0 && actionErr == nil {
		merged := resp.Headers.ToHTTPHeader()
		// Seed the encode-side response-status accessor (ADR-0196) so encode-
		// side classifying filters (e.g. admission_control) observe resp.Status,
		// which the encode header map does not carry (:status is not present).
		chain.SetEncodeResponseStatus(status)
		if _, err := chain.RunEncodeHeaders(ctx, merged, len(resp.Body) == 0); err != nil {
			return status, err
		}
		resp.Headers = filter_http.ReconcileOrderedHeaders(resp.Headers, merged)
		if len(resp.Body) > 0 {
			if _, err := chain.RunEncodeData(ctx, resp.Body, true); err != nil {
				return status, err
			}
		}
		// After chain.RunEncodeData returns, harvest any encode-body override.
		// Per ADR-0131 §Decision (vi): filters that compress / transform the
		// response body register the new bytes via cb.OverwriteBody(b); the
		// chain stores them on c.encodeBodyOverride; HCM substitutes resp.Body
		// before writeH1Reply consumes it. writeH1Reply rewrites Content-Length
		// unconditionally per codec.go, so substituting bytes here is the
		// load-bearing wire-shape mutation point.
		if override, ok := chain.EncodeBodyOverride(); ok {
			resp.Body = override
		}
	}

	bytesSent := int64(len(resp.Body))

	// Phase 07.1 Task 18 prereq P2: wire-write the response via writeH1Reply
	// (the unified ordered helper post-Task-19 — same path as the SendLocalReply
	// branch above). The action's surfaced response (post-encode-chain) is the
	// source of truth for status + headers + body.
	if rf.ActionRan() && actionErr == nil && status > 0 {
		if werr := writeH1Reply(bw, resp.Status, resp.Headers, resp.Body); werr != nil {
			actionErr = werr
		} else if resp.Close {
			actionErr = errCloseAfterAction
		}
	}

	// Per Decision §3.1: single uniform access-log emit site at chain-completion.
	// emitAccessLog is a no-op when status==0 (ctx-cancel sentinel) or when
	// f.accessLog is empty. Phase 46.1b Task 8: POST-Decide site — pass
	// traceDecision so the span block fires for sampled requests.
	f.emitAccessLog(req, status, bytesSent, picked, startTime, resp.Headers, traceDecision, chain.DynamicMetadata().Get)

	return status, actionErr
}

// drainAndClose discards any unread request body bytes and closes the body.
// Without this, the next iteration's http.ReadRequest would read stale body
// bytes as the request line.
func drainAndClose(req *http.Request) {
	if req.Body != nil {
		_, _ = io.Copy(io.Discard, req.Body)
		_ = req.Body.Close()
	}
}
