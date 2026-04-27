package hcm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"

	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// bad502Body is the local-reply body for upstream-failure 502 emissions per
// SPEC §11.9. The constant is the single source of truth so the H2 router's
// 502 prose cannot drift from any future H1 router 502 prose; phase-04's H1
// 502 currently uses an empty body via writeStatusReply(bw, 502, "") and is
// NOT touched by 05.2 (no regression). The shared-constant pattern future-
// proofs the migration.
const bad502Body = "bad gateway\n"

// errCloseAfterAction is returned by routeAction.do when the action's
// response carried Connection: close (or the equivalent semantic on the
// upstream-routed response). The connection loop checks for this sentinel
// via errors.Is and closes the downstream after the current iteration.
//
// SPEC §10 #3 settled to the sentinel-error mechanism (option (a)). Other
// non-nil errors from do trigger downstream close + log (the connection
// loop handles).
var errCloseAfterAction = errors.New("hcm: action requested connection close")

// directResponseAction synthesizes a local-reply response. Phase 05.1
// factors the writers codec-neutral: body() returns the codec-independent
// payload; writeH1 writes HTTP/1.1 wire bytes (byte-for-byte phase-04
// preserved); writeH2 writes HTTP/2 HEADERS + DATA frames via a streamWriter.
//
// The phase-04 struct field `body string` is renamed to `bodyText` to free
// the name `body` for the codec-neutral method (SPEC §13 + Settled #9).
// All call sites update mechanically; the wire output is unchanged.
//
// Per ADR-0045 + SPEC §5.5.
type directResponseAction struct {
	status   int
	bodyText string
}

// body returns the synthesized response in codec-neutral form per SPEC §5.5
// + §13's acceptance check. status is the configured value; headers contain
// Date/Server/Content-Type/Content-Length; the returned body bytes are the
// configured inline_string.
func (a *directResponseAction) body() (status int, headers http.Header, body []byte) {
	bodyBytes := []byte(a.bodyText)
	hdrs := http.Header{}
	hdrs.Set("Date", dateHeader())
	hdrs.Set("Server", serverHeader())
	hdrs.Set("Content-Type", "text/plain")
	hdrs.Set("Content-Length", strconv.Itoa(len(bodyBytes)))
	return a.status, hdrs, bodyBytes
}

// writeH1 is the H1 adapter — writes HTTP/1.1 wire bytes by delegating to
// writeStatusReply (phase-04 preserved byte-for-byte).
func (a *directResponseAction) writeH1(w io.Writer) error {
	return writeStatusReply(w, a.status, a.bodyText)
}

// writeH2 is the H2 adapter — writes HEADERS (`:status` pseudo first per
// RFC 9113 §8.3, then regular headers in deterministic order) + DATA + END_STREAM
// via the streamWriter.
func (a *directResponseAction) writeH2(sw h2.StreamWriter) error {
	status, hdrs, body := a.body()
	headers := []hpack.HeaderField{
		{Name: ":status", Value: strconv.Itoa(status)},
		{Name: "date", Value: hdrs.Get("Date")},
		{Name: "server", Value: hdrs.Get("Server")},
		{Name: "content-type", Value: hdrs.Get("Content-Type")},
		{Name: "content-length", Value: hdrs.Get("Content-Length")},
	}
	if err := sw.WriteHeaders(headers, false /* body follows */); err != nil {
		return err
	}
	return sw.WriteData(body, true /* end stream */)
}

// do (preserved for the routeAction interface — H1 connection.go calls this
// unchanged). Behaviourally identical to phase-04 because writeH1 == old do.
//
// Phase 06.1 Task 11: returns (a.status, error). The configured direct_response
// status is the finalized response code; runConnection Inc's the matching
// downstream_rq_<Nxx> bucket from this return.
func (a *directResponseAction) do(_ context.Context, _ *http.Request, bw *bufio.Writer) (int, error) {
	return a.status, a.writeH1(bw)
}

// routerAction proxies the request to the named cluster's selected endpoint.
// Per ADR-0039, every routed request opens a fresh upstream connection via
// Cluster.Dial(ctx); no pooling at phase 04. Per-failure-class mapping:
//
//	Cluster.Dial error      → 503 local reply, do returns nil
//	Request.Write error     → 502 local reply, do returns nil
//	http.ReadResponse error → 502 local reply, do returns nil
//	resp.Write error        → propagated up (downstream is broken)
//
// The router does NOT inject x-envoy-*, x-forwarded-*, or x-request-id
// headers (SPEC §2). The upstream sees the unmodified downstream request
// (modulo stdlib's textproto canonicalization on header names).
type routerAction struct {
	cluster *cluster.Cluster
}

// do drives one upstream H1 round-trip. Phase 06.1 Task 11 wires the cluster-
// scope upstream_rq_total + upstream_rq_<Nxx> counters per SPEC §5.5: total
// Inc's at dispatch entry (once per attempt, BEFORE Dial); the status-class
// counter Inc's after the response code is finalized. On the dial-failure /
// write-failure / read-failure local-reply paths, the synthesized 5xx status
// (503 for dial, 502 for write/read) is the bucket the cluster-scope counter
// reflects per the "5xx Inc lands on the dial-failure local-reply path too"
// annotation in PLAN Task 11.
//
// Returns (statusCode, error). statusCode is the finalized HTTP response code
// (503/502 on local-reply paths; resp.StatusCode on a successful proxy);
// err is the routeAction error (errCloseAfterAction sentinel or a real
// write-side failure).
func (a *routerAction) do(ctx context.Context, req *http.Request, bw *bufio.Writer) (int, error) {
	a.cluster.IncUpstreamRqTotal()

	upstream, err := a.cluster.Dial(ctx)
	if err != nil {
		a.cluster.IncStatusClass(503)
		return 503, writeStatusReply(bw, 503, "")
	}
	defer func() { _ = upstream.Close() }()

	// Propagate the downstream ctx deadline (if any) to the upstream socket
	// so a stalled upstream cannot hold the action past the ctx's deadline
	// during req.Write or http.ReadResponse — both of which are otherwise
	// ctx-unaware (REVIEW.md I-3 from REVIEW.md 04527eb).
	if dl, ok := ctx.Deadline(); ok {
		_ = upstream.SetDeadline(dl)
	}

	if err := req.Write(upstream); err != nil {
		a.cluster.IncStatusClass(502)
		return 502, writeStatusReply(bw, 502, "")
	}

	resp, err := http.ReadResponse(bufio.NewReader(upstream), req)
	if err != nil {
		a.cluster.IncStatusClass(502)
		return 502, writeStatusReply(bw, 502, "")
	}
	defer func() { _ = resp.Body.Close() }()

	a.cluster.IncStatusClass(resp.StatusCode)

	if err := resp.Write(bw); err != nil {
		return resp.StatusCode, err
	}
	// Honor the upstream's Connection: close (and HTTP/1.0 close-by-default)
	// by signaling the connection loop via errCloseAfterAction. http.ReadResponse
	// populates resp.Close from the wire-level signals (Connection: close on
	// HTTP/1.1, default-close on HTTP/1.0). SPEC §5.3 / SPEC §10 #3 settled.
	// REVIEW.md I-1 from REVIEW.md 04527eb.
	if resp.Close {
		return resp.StatusCode, errCloseAfterAction
	}
	return resp.StatusCode, nil
}

// routerActionH2 is the H2-flavored router action. Selected at filter-build
// time when the resolved cluster's UseH2() reports true (per SPEC §5.5 +
// §4.1). The struct mirrors routerAction in shape but consumes a fresh
// upstream H2 conn via Cluster.DialH2 per ADR-0056.
//
// Failure-class mapping per SPEC §11.9:
//
//	Cluster.DialH2 error        → 502 local reply (downstream :status 502 + bad502Body)
//	RoundTrip ctx-cancel        → RST_STREAM(CANCEL) on the downstream
//	RoundTrip protocol error    → 502 local reply
//	Upstream HTTP status (5xx)  → forwarded verbatim (NOT translated)
//
// Per ADR-0058: trailers are observed but NOT forwarded. The upstream-side
// observe-discard is implemented in h2.RoundTrip (the second HEADERS block
// is dropped on the floor); the downstream-side observe-discard is in
// serverStream.recvTrailingHeaders. The router itself emits END_STREAM on
// the response HEADERS or final DATA, never via a trailing HEADERS frame.
//
// Method namespace note: Go disallows two methods with the same name on the
// same receiver, so the H2 driver method is named doH2 (consumed by
// h2RouterActionAdapter.WriteH2 in h2dispatch.go); a separate do(...) method
// exists with the routeAction-interface signature so *routerActionH2 also
// satisfies routeAction (defensive — never reached in well-formed bootstraps;
// see the do method's docstring for the rationale).
type routerActionH2 struct {
	cluster *cluster.Cluster
}

// doH2 drives an upstream H2 round-trip via Cluster.DialH2 + ClientConn.RoundTrip
// per ADR-0056 (per-request fresh dial), writing the response back through
// the H2 stream writer.
//
// Phase 06.1 Task 11 wires the cluster-scope upstream_rq_total +
// upstream_rq_<Nxx> counters per SPEC §5.5 (Increment paths table,
// "routerActionH2.do (H2)" row): total Inc's at dispatch entry (once per
// attempt, BEFORE DialH2); the status-class counter Inc's after the
// upstream response status is finalized. Dial-failure / RoundTrip-failure
// local-reply paths Inc the 5xx (502) bucket so the cluster-scope
// counter reflects "what status-class came out of THIS cluster's dispatch".
// The ctx-cancel path emits a stream-scoped CANCEL — no status is finalized,
// so no class counter Inc (matches "request did not complete" semantics).
//
// Returns (statusForHCM, err). statusForHCM is the wire status the downstream
// H2 client will observe (502 on local-reply paths; resp.Status on a
// successful round-trip; 0 when no status is finalized — i.e. ctx-cancel).
// h2RouterActionAdapter consumes this to Inc the parent Filter's HCM-scope
// downstream_rq_<Nxx> counter on the H2 path per SPEC §5.5 "HCM response
// hook" row.
func (r *routerActionH2) doH2(ctx context.Context, req h2.H2Request, w h2.StreamWriter) (int, error) {
	r.cluster.IncUpstreamRqTotal()

	cc, err := r.cluster.DialH2(ctx)
	if err != nil {
		r.cluster.IncStatusClass(502)
		return 502, r.write502(w)
	}
	defer func() { _ = cc.Close() }() // ADR-0056: per-request fresh conn close (analog of phase-04 H1's defer upstream.Close())

	resp, err := cc.RoundTrip(ctx, req)
	if err != nil {
		// Distinguish: caller-side ctx-cancel/deadline → emit RST(CANCEL) on the
		// downstream stream (the canonical "client gave up" signal); other
		// errors (including upstream-conn-died wrapping cc.ctx.Err()) → 502
		// local reply. We MUST check the caller's ctx specifically; a
		// generic errors.Is(err, context.Canceled) would match cc.ctx.Err()
		// too, mis-categorizing upstream-conn-broken errors as caller-cancel.
		if ctx.Err() != nil && (errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
			// Surface a stream-scoped CANCEL to serverStream.dispatch which
			// emits RST_STREAM(CANCEL) per the dispatch carry-error contract.
			// No upstream_rq_<Nxx> Inc — the request did not produce a
			// terminating response status. Return 0 so the adapter skips
			// the HCM-scope downstream_rq_<Nxx> Inc as well.
			return 0, h2.NewStreamError(h2.ErrCancel, 0, "upstream roundtrip: ctx canceled")
		}
		r.cluster.IncStatusClass(502)
		return 502, r.write502(w)
	}

	r.cluster.IncStatusClass(resp.Status)

	// Forward response: the codec preserves wire order, so resp.Headers already
	// has :status (and any other pseudo-headers) first per RFC 9113 §8.3.
	if err := w.WriteHeaders(resp.Headers, false); err != nil {
		return resp.Status, err // surfaced to serverStream.dispatch which emits RST_STREAM(INTERNAL_ERROR)
	}
	if err := w.WriteData(resp.Body, true); err != nil {
		return resp.Status, err
	}
	return resp.Status, nil
}

// write502 emits a 502 Bad Gateway local-reply via the H2 stream writer.
// The body is the shared bad502Body constant per SPEC §11.9. Date is
// included per SPEC §10 #4. server is "envoy" per ADR-0014. Best-effort:
// any error from the writer is swallowed (conn is broken; nothing useful
// to surface). Always returns nil so dispatch does not RST after the 502.
func (r *routerActionH2) write502(w h2.StreamWriter) error {
	body := []byte(bad502Body)
	hdrs := []hpack.HeaderField{
		{Name: ":status", Value: "502"},
		{Name: "date", Value: dateHeader()},
		{Name: "server", Value: serverHeader()},
		{Name: "content-type", Value: "text/plain"},
		{Name: "content-length", Value: strconv.Itoa(len(body))},
	}
	if err := w.WriteHeaders(hdrs, false); err != nil {
		return nil // best-effort
	}
	_ = w.WriteData(body, true)
	return nil
}

// do (defensive) — *routerActionH2 satisfies the hcm-package routeAction
// interface so the H1 driver's entry.action.do(...) call site does not
// type-fault if an H2-cluster route is ever reached on an H1 path. In
// well-formed bootstraps this is unreachable: variant selection at
// filter-build time guarantees H2-clusters get *routerActionH2 and the
// HCM-level codec dispatch picks the H2 driver for those listeners. If
// somehow reached (invalid bootstrap shape — e.g. a codec_type=AUTO
// listener with alpn_protocols=["h2","http/1.1"] that negotiates
// "http/1.1" to a route pointing at an H2-only cluster), the stub
// writes a 500 status line + logs the misconfiguration so an operator
// debugging the resulting 500 sees the cause without having to grep
// the codec-dispatch path. Per the "Two interfaces, two separate
// decisions" note in PLAN Task 11; closes REVIEW I-2 (observability
// gap on the unreachable defensive stub).
func (r *routerActionH2) do(_ context.Context, _ *http.Request, bw *bufio.Writer) (int, error) {
	log.Printf("hcm: routerActionH2.do reached on H1 path — bootstrap misconfiguration; route variant selection should have produced *routerAction, not *routerActionH2 (cluster=%q)", r.cluster.Name())
	return 500, writeStatusReply(bw, 500, "")
}

// Compile-time interface conformance assertion. *routerActionH2 must satisfy
// routeAction so it can be stored in routeEntry.action and reachable via the
// shared route-table machinery (defensive — never reached on H1 path in
// well-formed bootstraps).
var _ routeAction = (*routerActionH2)(nil)
