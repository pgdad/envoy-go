package hcm

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/esalaine/envoy-go/internal/cluster"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
)

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
		if keepAlive := f.serveOneRequest(ctx, req, bw); !keepAlive {
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
func (f *Filter) serveOneRequest(ctx context.Context, req *http.Request, bw *bufio.Writer) (keepAlive bool) {
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

	status, actErr := f.dispatchRequest(ctx, req, bw)
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
func (f *Filter) dispatchRequest(ctx context.Context, req *http.Request, bw *bufio.Writer) (int, error) {
	entry, routeIdx, ok := f.table.match(req)
	if !ok {
		// 404 catch-all: phase-04 byte-preserved synthesis with empty body
		// (matches the connection.go pre-Task-15 wire shape). The chain is
		// NOT allocated for a no-match — there is no route → no per-route
		// config → no terminal action to run; the legacy direct synthesis
		// is the byte-equivalent path. Access-log emit also fires here per
		// Decision §3.1 (the "no-match" terminal state).
		start := time.Now()
		err := writeStatusReply(bw, 404, "")
		f.emitAccessLog(req, 404, 0, cluster.Endpoint{}, start)
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
		// Synthesize 500 + log; the bw write is best-effort.
		log.Printf("hcm: dispatchRequest: terminal filter is not *router.Filter (got %T)", chainHF[len(chainHF)-1].Decoder)
		start := time.Now()
		err := writeStatusReply(bw, 500, "")
		f.emitAccessLog(req, 500, 0, cluster.Endpoint{}, start)
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
		f.emitAccessLog(req, lrStatus, bytesSent, cluster.Endpoint{}, startTime)
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
		f.emitAccessLog(req, lrStatus, bytesSent, cluster.Endpoint{}, startTime)
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
		if _, err := chain.RunEncodeHeaders(ctx, merged, len(resp.Body) == 0); err != nil {
			return status, err
		}
		resp.Headers = filter_http.ReconcileOrderedHeaders(resp.Headers, merged)
		if len(resp.Body) > 0 {
			if _, err := chain.RunEncodeData(ctx, resp.Body, true); err != nil {
				return status, err
			}
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
	// f.accessLog is empty. Calls into the existing accesslog_emit.go body
	// (UNCHANGED at this task; only the call site moves).
	f.emitAccessLog(req, status, bytesSent, picked, startTime)

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
