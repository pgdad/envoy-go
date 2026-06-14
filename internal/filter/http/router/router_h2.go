package router

import (
	"bufio"
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// H2ClusterAction returns an H2Action closure that proxies the per-request
// H2 upstream call to the supplied cluster's selected endpoint. The closure
// delegates to a fresh internal driver (doH2ClusterAction) modeled after the
// byte-preserved routerActionH2.doH2 logic but surfacing (status, bytesSent,
// picked, err) for the chain-mediated dispatch path. HCM h2dispatch.go builds
// one of these per matched route at filter-build time and injects it via
// *Filter.SetH2Action at request-start.
//
// Per SPEC §11.9 / SPEC §2.1:
//   - Cluster.DialH2 error        → 502 local reply, status=502, err=nil
//   - RoundTrip ctx-cancel        → status=0, err=*h2.Error(CANCEL) (sentinel)
//   - RoundTrip protocol error    → 502 local reply, status=502, err=nil
//   - Upstream HTTP status        → forwarded verbatim to downstream sw,
//     status=resp.Status, err=writer-error or nil.
//
// status=0 on the ctx-cancel path is the H2 sentinel per SPEC §2.1 last
// bullet; HCM's chain-completion access-log emit hook skips submission on
// status=0.
func H2ClusterAction(c *cluster.Cluster, hps []HashPolicy, subsetMatch cluster.SubsetMatch) H2Action {
	a := &routerActionH2{cluster: c, hashPolicies: hps, subsetMatch: subsetMatch}
	return func(ctx context.Context, req h2.H2Request) (ActionResponse, cluster.Endpoint, error) {
		return doH2ClusterAction(ctx, a, req)
	}
}

// doH2ClusterAction runs the per-request H2 upstream-dial dispatch and surfaces
// the logical ActionResponse for the H2Action closure. Phase 07.1 Task 18
// prereq P1: returns ActionResponse instead of writing wire bytes via sw.
// HCM h2dispatch.go's chain-completion path runs the response through the
// encode chain THEN writes HEADERS+DATA frames via sw.
//
// On dial / RoundTrip failure paths, returns an ActionResponse with status=502
// and the canonical bad502Body. On caller-ctx-cancel returns Status=0 + an
// *h2.Error so serverStream.dispatch emits RST_STREAM(CANCEL). On wire-write
// failure (impossible at this layer post-refactor — wire-write moved to
// dispatch), the error is just propagated.
func doH2ClusterAction(ctx context.Context, a *routerActionH2, req h2.H2Request) (ActionResponse, cluster.Endpoint, error) {
	picked := cluster.Endpoint{}

	a.cluster.IncUpstreamRqTotal()

	// 36.2: fold the route's hash_policy list into a ring_hash key carried on
	// ctx (cluster.WithHashKey) → ringHashLB.Pick reads it in DialH2. The H2
	// request carries no remote addr, so source_ip uses the ctx-carried
	// downstream addr set by HCM dispatch (h2dispatch.go). ADR-0237.
	ctx, _, _ = applyHashKey(ctx, a.hashPolicies, h2HeaderVal(req), downstreamRemoteAddrFrom(ctx))

	// 38.1: thread the route-static metadata_match onto ctx → subsetLB.Pick
	// reads it in DialH2. Route-static (resolved once at config-build, NOT a
	// per-request fold like applyHashKey); threaded verbatim only when non-empty
	// (an empty match leaves ctx untouched → the subsetLB fallback path). ADR-0239.
	if !a.subsetMatch.Empty() {
		ctx = cluster.WithSubsetMatch(ctx, a.subsetMatch)
	}

	cc, ep, err := a.cluster.DialH2(ctx)
	if err != nil {
		a.cluster.IncStatusClass(502)
		return ActionResponse{Status: 502, Body: []byte(bad502Body), Headers: h2LocalReplyHeaders()}, picked, nil
	}
	defer func() { _ = cc.Close() }()
	picked = ep

	resp, err := cc.RoundTrip(ctx, req)
	if err != nil {
		// Distinguish caller-side ctx-cancel/deadline (→ stream-scoped CANCEL
		// surfaced upward as *h2.Error so serverStream.dispatch emits
		// RST_STREAM(CANCEL)) from any other error (→ 502 local reply). Mirror
		// of routerActionH2.doH2's ctx-vs-other discrimination.
		if ctx.Err() != nil && (errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded)) {
			// status=0 → H2 ctx-cancel sentinel per SPEC §2.1 last bullet.
			// emitAccessLogH2 skips submission on status==0; serverStream.dispatch
			// reads err.Code == ErrCancel and emits RST_STREAM(CANCEL).
			return ActionResponse{Status: 0}, picked, h2.NewStreamError(h2.ErrCancel, 0, "upstream roundtrip: ctx canceled")
		}
		a.cluster.IncStatusClass(502)
		return ActionResponse{Status: 502, Body: []byte(bad502Body), Headers: h2LocalReplyHeaders()}, picked, nil
	}

	a.cluster.IncStatusClass(resp.Status)

	// Convert h2 hpack header fields into OrderedHeaders for the chain's encode
	// side. Pseudo-headers (:status etc.) are stripped — the chain works on
	// regular headers; HCM h2dispatch's wire-write emits :status from
	// resp.Status before the regular header set. Phase 07.1 Task 19 (I-3
	// prereq): the carrier is OrderedHeaders so insertion order from the H2
	// HPACK-decoded sequence survives through the chain to the wire (H2 codec
	// preserves wire order on resp.Headers per RFC 9113 §8.1.2 — slice already
	// carries the correct order).
	respHeaders := make(envoyhttp.OrderedHeaders, 0, len(resp.Headers))
	for _, hf := range resp.Headers {
		if strings.HasPrefix(hf.Name, ":") {
			continue
		}
		respHeaders = append(respHeaders, envoyhttp.HeaderField{Name: hf.Name, Value: hf.Value})
	}
	return ActionResponse{
		Status:  resp.Status,
		Headers: respHeaders,
		Body:    resp.Body,
	}, picked, nil
}

// h2LocalReplyHeaders builds the standard local-reply header set for
// synthesized 5xx H2 responses. Mirrors the Header set previously written by
// routerActionH2.write502. Phase 07.1 Task 19 (I-3 prereq): returns
// OrderedHeaders so the action-driven wire-emit path preserves the three-
// header insertion order (Content-Type, Date, Server) on the H2 HEADERS frame.
func h2LocalReplyHeaders() envoyhttp.OrderedHeaders {
	return envoyhttp.OrderedHeaders{
		{Name: "Content-Type", Value: "text/plain"},
		{Name: "Date", Value: dateHeader()},
		{Name: "Server", Value: serverHeader()},
	}
}

// emitAccessLogH2 is the H2-flavored variant of (*Filter).emitAccessLog;
// reads pseudo-headers (:method, :path, :authority) and User-Agent from
// H2Request fields. Per SPEC §2.1 last bullet, a zero statusCode is the H2
// ctx-cancel sentinel and skips emission. Migrated verbatim from
// internal/filter/hcm/accesslog_emit.go.
func (f *Filter) emitAccessLogH2(req h2.H2Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time) {
	if statusCode == 0 || len(f.accessLog) == 0 {
		return
	}
	rec := &accesslog.Record{
		StartTime:    start,
		Method:       req.Method,
		Path:         req.Path,
		Protocol:     "HTTP/2.0",
		ResponseCode: statusCode,
		BytesSent:    bytesSent,
		Duration:     time.Since(start),
		Authority:    req.Authority,
		UserAgent:    h2UserAgent(req),
		UpstreamHost: upstreamHostString(picked),
	}
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}

// h2UserAgent extracts the User-Agent header value from an H2Request's
// Headers slice (case-insensitive match per RFC 7540 §8.1.2 — header names
// are lowercase in HTTP/2). Returns empty string if absent.
func h2UserAgent(req h2.H2Request) string {
	for _, hf := range req.Headers {
		if strings.EqualFold(hf.Name, "user-agent") {
			return hf.Value
		}
	}
	return ""
}

// h2HeaderVal returns a codec-agnostic headerVal accessor over the H2 request's
// hpack fields for applyHashKey. req.Headers is []hpack.HeaderField with
// codec-lowercased Name; EqualFold mirrors h2UserAgent for robustness. Returns
// the FIRST matching value (single-value producer path per D-S362-4; the full
// multi-value fold lives in cluster.HashHeaderValues).
func h2HeaderVal(req h2.H2Request) func(name string) (string, bool) {
	return func(name string) (string, bool) {
		for _, hf := range req.Headers {
			if strings.EqualFold(hf.Name, name) {
				return hf.Value, true
			}
		}
		return "", false
	}
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
//
// Migrated from internal/filter/hcm/actions.go at phase 07.1 Task 11 with
// signatures preserved byte-for-byte so the byte-preserved tests in
// router_h2_test.go exercise the same shape.
type routerActionH2 struct {
	cluster      *cluster.Cluster
	filter       *Filter             // set post-build by routeTable.bindFilter; nil when no sinks configured.
	hashPolicies []HashPolicy        // 36.2: stored at H2ClusterAction; per-request fold lands in Task 4 (applyHashKey).
	subsetMatch  cluster.SubsetMatch // 38.1: route-static metadata_match threaded onto ctx at dispatch (ADR-0239).
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
	start := time.Now()

	// statusForHCM, bytesSentH2, and picked are captured by the deferred
	// access-log emit so the closure reads the final values. Per SPEC §2.1,
	// a zero statusForHCM (ctx-cancel sentinel) skips emission inside
	// emitAccessLogH2. bytesSentH2 is set to len(resp.Body) on the success
	// path per SPEC §12 #3 option (a).
	statusForHCM := 0
	bytesSentH2 := 0
	picked := cluster.Endpoint{}
	if r.filter != nil {
		defer func() {
			r.filter.emitAccessLogH2(req, statusForHCM, int64(bytesSentH2), picked, start)
		}()
	}

	r.cluster.IncUpstreamRqTotal()

	// 36.2: fold the route's hash_policy list into a ring_hash key carried on
	// ctx (cluster.WithHashKey) → ringHashLB.Pick reads it in DialH2. See
	// doH2ClusterAction for the source_ip ctx-carry rationale. ADR-0237.
	ctx, _, _ = applyHashKey(ctx, r.hashPolicies, h2HeaderVal(req), downstreamRemoteAddrFrom(ctx))

	cc, ep, err := r.cluster.DialH2(ctx)
	if err != nil {
		r.cluster.IncStatusClass(502)
		statusForHCM = 502
		return 502, r.write502(w)
	}
	defer func() { _ = cc.Close() }() // ADR-0056: per-request fresh conn close (analog of phase-04 H1's defer upstream.Close())
	picked = ep

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
			// terminating response status. statusForHCM remains 0 so
			// emitAccessLogH2 skips emission (SPEC §2.1 sentinel).
			return 0, h2.NewStreamError(h2.ErrCancel, 0, "upstream roundtrip: ctx canceled")
		}
		r.cluster.IncStatusClass(502)
		statusForHCM = 502
		return 502, r.write502(w)
	}

	r.cluster.IncStatusClass(resp.Status)
	statusForHCM = resp.Status
	bytesSentH2 = len(resp.Body)

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
	log.Printf("router: routerActionH2.do reached on H1 path — bootstrap misconfiguration; route variant selection should have produced *routerAction, not *routerActionH2 (cluster=%q)", r.cluster.Name())
	return 500, writeStatusReply(bw, 500, "")
}
