package hcm

import (
	"bytes"
	"context"
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
	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/tracing"
)

// *Filter satisfies network.H3Terminal (phase 61.2) — the QUIC listen path
// drives f.ServeH3 via quic-go's http3.Server. This assert fails to compile if
// ServeH3's signature drifts from the interface.
var _ network.H3Terminal = (*Filter)(nil)

// writeH3Reply projects a router.ActionResponse (status + ordered headers +
// body) onto quic-go's http.ResponseWriter. quic-go's http3.Server owns HTTP/3
// framing + QPACK below this seam (exactly as the internal h2 codec hides HTTP/2
// framing below h2.StreamWriter), so this adapter is a pure stdlib projection —
// NOT the HTTP/1.1 wire writer writeH1Reply (which emits a status line + CRLF
// framing wrong for H3). HTTP pseudo-headers (":status", ":path", …) are dropped
// — they are not real response headers. Phase 61.2, ADR-0281.
func writeH3Reply(w http.ResponseWriter, status int, headers filter_http.OrderedHeaders, body []byte) error {
	h := w.Header()
	for _, hf := range headers {
		if strings.HasPrefix(hf.Name, ":") {
			continue // pseudo-header — not a wire response header
		}
		h.Add(hf.Name, hf.Value)
	}
	// Phase 61.3 (ADR-0281 §Consequences deferral, ADR-0282): synthesize the
	// fidelity headers the reference Envoy emits for a direct_response,
	// mirroring the H1 writeStatusReply's Server/Content-Length emission
	// (codec.go) without double-setting an action-supplied value.
	// http.Header.Get is case-insensitive, so this respects any casing the
	// router action already set. Server is synthesized ALWAYS (even for an
	// empty body — Envoy emits server on all responses); Content-Length is
	// synthesized ONLY when the body is non-empty (an empty-body response
	// gets no Content-Length, never Content-Length: 0). Date is intentionally
	// NOT synthesized — it is a timestamp and the differential does not
	// byte-compare headers.
	if h.Get("Server") == "" {
		h.Set("Server", serverHeader())
	}
	if len(body) > 0 && h.Get("Content-Length") == "" {
		h.Set("Content-Length", strconv.Itoa(len(body)))
	}
	w.WriteHeader(status)
	if len(body) > 0 {
		if _, err := w.Write(body); err != nil {
			return err
		}
	}
	return nil
}

// ServeH3 is the quic-go http3.Server handler entry (satisfies the
// http.HandlerFunc-shaped seam that network.H3Terminal wires to the QUIC
// listener). It dispatches one HTTP/3 request through the shared HCM → router →
// filter-chain path, discarding runH3's (status, err) return — the wire
// response is already written to w and the access-log/counters are emitted
// inside runH3. Phase 61.2, ADR-0281.
func (f *Filter) ServeH3(w http.ResponseWriter, r *http.Request) {
	_, _ = f.runH3(r.Context(), w, r)
}

// runH3 dispatches one HTTP/3 request through the per-request *FilterChain. It
// is a FAITHFUL DUPLICATION of the H1 dispatchRequest (connection.go) — same
// route-match, same per-request router.Action, same chain.SetX seeder set, same
// pseudo-header injection, same tracing dispatch, same decode-headers/decode-data
// loop + SendLocalReply handling, and the same RunAction → encode-chain tail —
// with three H3 substitutions:
//
//   - the response is written via writeH3Reply(http.ResponseWriter) instead of
//     writeH1Reply(*bufio.Writer);
//   - TLS state is sourced from r.TLS (quic-go populates the *tls.ConnectionState
//     after the QUIC handshake) rather than a net.Conn type-assertion;
//   - SetDownstreamProtocol is "HTTP/3".
//
// Unlike H1 (whose per-request drain Inc/Dec + downstream_rq_total Inc live in
// the runConnection/serveOneRequest caller), runH3 IS the once-per-request entry
// (ServeH3 is invoked per HTTP/3 request by quic-go, the H3 analog of WriteH2),
// so it owns the f.dm Inc/Dec discipline, the downstream_rq_total Inc, and the
// per-response downstream_rq_<Nxx> Inc directly — mirroring h2dispatch.go's
// WriteH2. resp.Close is H1-only and IGNORED on H3 (as on H2). Returns the
// response status + any wire-write/decode error. Phase 61.2, ADR-0281.
func (f *Filter) runH3(ctx context.Context, w http.ResponseWriter, r *http.Request) (int, error) {
	// Per-request drain Inc/Dec (ADR-0096 + ADR-0075). ServeH3 is invoked once
	// per HTTP/3 request, so the deferred Dec fires at request-end (= function
	// return) under all exit paths. The markedInflight sentinel prevents a Dec
	// without a matching Inc. Mirrors WriteH2's per-stream discipline.
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

	// Once-per-request dispatch-entry hook (SPEC §12 #1 site (a); the H3 analog
	// of runConnection's H1 hook / h2Dispatcher.Match's H2 hook). Inc before
	// route-match resolution so every observed request is counted.
	f.downstreamRqTotal.Inc()

	// Frame-local tracing decision pointer, in scope at every emitAccessLogH3
	// call site. Stays nil until the tracing block below runs (pre-Decide emit
	// sites pass nil automatically).
	var traceDecision *tracing.Decision
	start := time.Now()

	entry, routeIdx, ok := f.table.match(r)
	if !ok {
		// 404 no-match: no chain is built (no route → no per-route config → no
		// terminal action). Mirrors dispatchRequest's no-match branch.
		err := writeH3Reply(w, 404, nil, nil)
		f.emitAccessLogH3(r, 404, 0, cluster.Endpoint{}, start, nil, traceDecision)
		if cnt := f.downstreamStatusClassCounter(404); cnt != nil {
			cnt.Inc()
		}
		return 404, err
	}

	// Per-request router.Action from the matched route (H1-flavored — the same
	// asRouterAction() the H1 path uses, NOT asRouterActionH2).
	action := entry.action.asRouterAction()

	// Allocate fresh per-request filter instances from chainConfig (ADR-0071's
	// two-step factory pattern). The terminal entry is the router filter
	// (ValidateChainShape-pinned).
	chainHF := make([]filter_http.HTTPFilter, len(f.chainConfig))
	for i, e := range f.chainConfig {
		chainHF[i] = e.factory()
	}
	chain := filter_http.NewFilterChain(chainHF, f.perRouteConfig)
	chain.SetRequestCtx(ctx, routeIdx)

	// TLS snapshot: r.TLS IS the *tls.ConnectionState (quic-go populates it after
	// the QUIC handshake) — the H3 analog of the H1 downstream.(*stdtls.Conn)
	// ConnectionState() snapshot. nil for the unit-test path that leaves r.TLS
	// unset; every TLS-derived seed below nil-tolerates identically to the H1
	// tlsState != nil guards.
	tlsState := r.TLS
	// Phase 16 Task 6 (ADR-0144): seed the per-stream TLS principal candidates
	// (URI SAN → DNS SAN → Subject CN) BEFORE RunDecodeHeaders. nil for
	// plaintext / non-mTLS / no-client-cert.
	chain.SetTLSPrincipals(extractTLSPrincipals(tlsState))
	// Phase 22.2 Task 6 (ADR-0192): seed the FULL *tls.ConnectionState (gated on
	// HandshakeComplete, symmetric to the H1 path). nil-passthrough otherwise.
	if tlsState != nil && tlsState.HandshakeComplete {
		chain.SetTLSConnectionState(tlsState)
	}
	// Phase 18.2 Task 4 (ADR-0165) + Phase 36.2 (ADR-0237): seed the downstream
	// addrs. r.RemoteAddr is an "ip:port" string (quic-go's UDP remote); parse
	// it to a *net.UDPAddr for the SetDownstreamRemoteAddr seeder (the H3 analog
	// of the H1 *net.TCPAddr) AND seed the ctx string for the source_ip
	// hash_policy producer, which reads the addr off ctx (r.RemoteAddr is not
	// re-derivable inside the router action). No-op when no route configures a
	// source_ip hash_policy.
	if ra := h3RemoteAddr(r.RemoteAddr); ra != nil {
		chain.SetDownstreamRemoteAddr(ra)
	}
	if r.RemoteAddr != "" {
		ctx = router.WithDownstreamRemoteAddr(ctx, r.RemoteAddr)
	}
	// Local addr: quic-go's http3.Server does NOT populate
	// http.LocalAddrContextKey (an stdlib net/http.Server internal), so this is
	// nil in production — a DOCUMENTED boundary (the chain field stays nil; no
	// current consumer reads DownstreamLocalAddr on the H3 path). Read
	// defensively so a future quic-go release that DOES set it lights the seed
	// up automatically.
	if la, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr); ok {
		chain.SetDownstreamLocalAddr(la)
	}
	chain.SetDownstreamProtocol("HTTP/3")
	chain.SetListenerPrincipal(f.listenerPrincipal)
	// Phase 24.1 Task 5 (ADR-0198) + 24.2 (D-RL8 / D-RL11): seed the ratelimit
	// policy slices, route metadata, and the legacy include_vh_rate_limits bool.
	// nil/false-passthrough when absent (zero-regression), identical to H1/H2.
	chain.SetRouteRateLimits(entry.rateLimits)
	chain.SetVirtualHostRateLimits(f.table.vhostRateLimits)
	chain.SetRouteMetadata(entry.metadata)
	chain.SetRouteIncludeVhRateLimits(entry.includeVhRateLimits)
	if tlsState != nil {
		chain.SetDownstreamTLSServerName(tlsState.ServerName)
		if len(tlsState.PeerCertificates) > 0 {
			chain.SetDownstreamTLSPeerCertDER(tlsState.PeerCertificates[0].Raw)
		}
	}
	defer chain.Destroy()

	// Locate the terminal router filter and inject the per-request action + req.
	rf, ok := chainHF[len(chainHF)-1].Decoder.(*router.Filter)
	if !ok {
		log.Printf("hcm: runH3: terminal filter is not *router.Filter (got %T)", chainHF[len(chainHF)-1].Decoder)
		err := writeH3Reply(w, 500, nil, nil)
		f.emitAccessLogH3(r, 500, 0, cluster.Endpoint{}, start, nil, traceDecision)
		if cnt := f.downstreamStatusClassCounter(500); cnt != nil {
			cnt.Inc()
		}
		return 500, err
	}
	rf.SetAction(action)
	rf.SetRequest(r)

	// Inject :method / :authority / :path pseudo-headers onto req.Header so
	// chain-level filters (cors/csrf/rbac) read them without codec-specific
	// surfacing — mirrors dispatchRequest EXACTLY (raw-map access; no wire-emit
	// path iterates req.Header).
	if _, ok := r.Header[":method"]; !ok {
		r.Header[":method"] = []string{r.Method}
	}
	if _, ok := r.Header[":authority"]; !ok && r.Host != "" {
		r.Header[":authority"] = []string{r.Host}
	}
	if _, ok := r.Header[":path"]; !ok && r.URL != nil {
		path := r.RequestURI
		if path == "" {
			path = r.URL.RequestURI()
		}
		r.Header[":path"] = []string{path}
	}

	// Phase 46.1a Task 9: HCM-native request-tracing dispatch — provider-aware
	// extract/inject over req.Header, symmetric to the H1 seam in connection.go.
	if f.tracingConfig != nil {
		var ic tracing.TraceContext
		var okc bool
		if f.tracingConfig.Provider == tracing.ProviderZipkin {
			ic, okc = tracing.ExtractB3(r.Header)
		} else {
			ic, okc = tracing.ExtractTraceparent(r.Header)
		}
		d := tracing.DecideWithContext(r.Header, ic, okc, f.tracingConfig, f.rng)
		r.Header.Set("X-Request-Id", d.RequestID)
		if f.tracingConfig.Provider == tracing.ProviderZipkin {
			tracing.InjectB3(r.Header, d, f.tracingConfig.Zipkin.TraceID128Bit, f.tracingConfig.Zipkin.SharedSpanContext)
		} else {
			tracing.InjectTraceparent(r.Header, d.TraceID, d.SpanID, d.Sample, d.TraceState)
		}
		f.tracingCounters.Record(d.Class)
		traceDecision = &d
	}

	// Decode side: headers → data. endStream on headers is true when the request
	// carries no body (mirror the H1 hasBody rule).
	hasBody := r.Body != nil && r.Body != http.NoBody && r.ContentLength != 0
	endStreamOnHeaders := !hasBody

	if _, err := chain.RunDecodeHeaders(ctx, r.Header, endStreamOnHeaders); err != nil {
		// ctx-cancel or unknown filter status. status==0 skips the access-log
		// emit (matches the H1/H2 sentinel discipline).
		return 0, err
	}

	// SendLocalReply-after-headers: a non-terminal filter (cors etc.) triggered
	// SendLocalReply during DecodeHeaders; the chain already ran the encode chain
	// over the synthesized response. Write it via writeH3Reply and bypass
	// RunAction. Mirrors WriteH2's local-reply path (no Connection:close handling
	// — that is an H1-only wire concern).
	if chain.LocalReplyDone() {
		lrStatus, lrHeaders, lrBody := chain.LocalReplyResponse()
		var werr error
		if lrStatus > 0 {
			werr = writeH3Reply(w, lrStatus, lrHeaders, lrBody)
		}
		f.emitAccessLogH3(r, lrStatus, int64(len(lrBody)), cluster.Endpoint{}, start, lrHeaders, traceDecision)
		if lrStatus > 0 {
			if cnt := f.downstreamStatusClassCounter(lrStatus); cnt != nil {
				cnt.Inc()
			}
		}
		return lrStatus, werr
	}

	if hasBody {
		// Buffer the body bytes as we feed them to the chain so the terminal
		// router action's req.Write(upstream) sees a readable req.Body again —
		// identical to dispatchRequest's Task-22 buffering (without it a routed
		// POST reaches the upstream with headers + zero body). Cap semantics
		// (413 on overflow inside RunDecodeData) are enforced by the chain.
		buf := make([]byte, 32*1024)
		var bodyBuf []byte
		lastEndStreamFired := false
		for {
			n, rerr := r.Body.Read(buf)
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
		// Fire a synthetic empty-terminal chunk if the last Read returned
		// (0, io.EOF) so endStream-finalizing filters observe end-of-stream.
		if !lastEndStreamFired {
			if _, derr := chain.RunDecodeData(ctx, nil, true); derr != nil {
				return 0, derr
			}
		}
		// Restore req.Body for the router's upstream req.Write.
		r.Body = io.NopCloser(bytes.NewReader(bodyBuf))
		// Propagate a filter-injected Content-Length (buffer filter) back onto
		// the request struct fields so req.Write emits it (and not chunked).
		if clStr := r.Header.Get("Content-Length"); clStr != "" && r.ContentLength < 0 {
			if cl, err := strconv.ParseInt(clStr, 10, 64); err == nil {
				r.ContentLength = cl
				r.TransferEncoding = nil
			}
		}
	}

	// SendLocalReply-during-DecodeData: mirror the post-headers branch so the
	// terminal action does NOT run when a filter synthesized a response.
	if chain.LocalReplyDone() {
		lrStatus, lrHeaders, lrBody := chain.LocalReplyResponse()
		var werr error
		if lrStatus > 0 {
			werr = writeH3Reply(w, lrStatus, lrHeaders, lrBody)
		}
		f.emitAccessLogH3(r, lrStatus, int64(len(lrBody)), cluster.Endpoint{}, start, lrHeaders, traceDecision)
		if lrStatus > 0 {
			if cnt := f.downstreamStatusClassCounter(lrStatus); cnt != nil {
				cnt.Inc()
			}
		}
		return lrStatus, werr
	}

	// Terminal-action invocation. Idempotent — if a filter triggered
	// SendLocalReply earlier, rf.ActionRan stays false and the action does not
	// dial upstream (handled by the LocalReplyDone branches above).
	rf.RunAction(ctx)

	resp := rf.Response()
	picked := rf.Picked()
	actionErr := rf.ActionErr()
	status := resp.Status

	// Run the response through the encode chain (reverse declaration order) so
	// encode-side filters mutate headers/body before writeH3Reply. Identical
	// project → reconcile → body-override harvest as dispatchRequest / WriteH2.
	if rf.ActionRan() && status > 0 && actionErr == nil {
		merged := resp.Headers.ToHTTPHeader()
		chain.SetEncodeResponseStatus(status)
		if _, err := chain.RunEncodeHeaders(ctx, merged, len(resp.Body) == 0); err != nil {
			f.emitAccessLogH3(r, status, int64(len(resp.Body)), picked, start, resp.Headers, traceDecision)
			return status, err
		}
		resp.Headers = filter_http.ReconcileOrderedHeaders(resp.Headers, merged)
		if len(resp.Body) > 0 {
			if _, err := chain.RunEncodeData(ctx, resp.Body, true); err != nil {
				f.emitAccessLogH3(r, status, int64(len(resp.Body)), picked, start, resp.Headers, traceDecision)
				return status, err
			}
		}
		if override, ok := chain.EncodeBodyOverride(); ok {
			resp.Body = override
		}
	}

	bytesSent := int64(len(resp.Body))

	// Wire-write the (post-encode-chain) response. resp.Close is H1-only and
	// IGNORED here (as on H2 — HTTP/3 has no connection-close-after-response
	// wire concept at this seam).
	if rf.ActionRan() && actionErr == nil && status > 0 {
		if werr := writeH3Reply(w, resp.Status, resp.Headers, resp.Body); werr != nil {
			actionErr = werr
		}
	}

	// Single uniform access-log + span emit site at chain-completion (no-op when
	// status==0 or f.accessLog is empty; the span block fires for sampled reqs).
	f.emitAccessLogH3(r, status, bytesSent, picked, start, resp.Headers, traceDecision)
	if status > 0 {
		if cnt := f.downstreamStatusClassCounter(status); cnt != nil {
			cnt.Inc()
		}
	}

	return status, actionErr
}

// h3RemoteAddr parses an "ip:port" downstream address string (r.RemoteAddr,
// which quic-go populates from the QUIC connection's UDP remote) into a
// *net.UDPAddr for the chain's SetDownstreamRemoteAddr seeder. HTTP/3 rides
// UDP, so the addr is modeled as a *net.UDPAddr (the H3 analog of the H1
// *net.TCPAddr from net.Conn.RemoteAddr()). Returns nil when the string is
// empty or unparseable (the seeder nil-tolerates); the ctx-string seed
// (router.WithDownstreamRemoteAddr) carries the raw value regardless for the
// source_ip hash_policy producer. Pure string parse — no DNS (the host part is
// an IP literal on the QUIC datagram path).
func h3RemoteAddr(s string) net.Addr {
	if s == "" {
		return nil
	}
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil
	}
	return &net.UDPAddr{IP: ip, Port: port}
}
