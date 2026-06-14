package hcm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
)

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
//
// Phase 07.1 Task 15: the *Filter backpointer is REMOVED; the access-log
// emit-deferral hook in do() is REMOVED. Per Decision §3.1, access-log
// emit fires from HCM dispatch's chain-completion hook (a single uniform
// site that replaces the four per-action emit-deferral sites from 06.2).
// directResponseAction is now invoked by HCM dispatch via the router
// filter's per-request Action closure (built at config.go's
// buildDirectResponseAction → asRouterAction() bridge).
type directResponseAction struct {
	status   int
	bodyText string

	// extraHeaders carries the route-level response_headers_to_add entries
	// (parsed at config-build time per buildDirectResponseAction). Applied in
	// body() with OVERWRITE_IF_EXISTS_OR_ADD semantics — name-match
	// case-insensitive replace against the 4 default headers; new keys
	// appended in insertion order. Phase 14 fixture 0016 needs this so direct_
	// response routes can set Content-Type to the per-route value (text/html,
	// image/png) that the compressor filter's content_type-match bucket
	// inspects at EncodeHeaders time — without this, envoy-go's directResponse
	// emits a hardcoded `Content-Type: text/plain` that masquerades as
	// "matches the default 8-entry list" and produces a fixture-0016 scenario
	// 2 differential mismatch vs reference Envoy (which emits image/png and
	// correctly hits the content_type_mismatch skip bucket).
	//
	// Simplification vs Envoy's full HeaderValueOption.AppendAction enum: we
	// implement OVERWRITE_IF_EXISTS_OR_ADD uniformly. APPEND_IF_EXISTS_OR_ADD
	// (the proto default) is NOT implemented; fixture YAMLs SHOULD set
	// append_action: OVERWRITE_IF_EXISTS_OR_ADD explicitly so reference Envoy
	// matches envoy-go's semantics. This is a phase-14-Task-14 incremental
	// addition; broader response_headers_to_add support (full enum + multi-
	// header dedup) is deferred to future phases.
	extraHeaders filter_http.OrderedHeaders
}

// body returns the synthesized response in codec-neutral form per SPEC §5.5
// + §13's acceptance check. status is the configured value; headers contain
// Date/Server/Content-Type/Content-Length; the returned body bytes are the
// configured inline_string.
//
// Phase 07.1 Task 19 (I-3 prereq): returns OrderedHeaders so the four headers
// (Content-Type, Content-Length, Server, Date) land on the wire in their
// insertion order — matching writeStatusReply's verbatim shape from phase-04
// (and the writeH2 path's HEADERS-frame deterministic order).
//
// Phase 14 Task 14: extraHeaders (route-level response_headers_to_add) are
// applied AFTER the 4 default headers with OVERWRITE_IF_EXISTS_OR_ADD
// semantics — name-match (case-insensitive) replaces the default value;
// otherwise appends. Required so the compressor filter at EncodeHeaders time
// sees the per-route Content-Type the bootstrap configures (e.g. text/html,
// image/png) rather than the hardcoded text/plain default.
func (a *directResponseAction) body() (status int, headers filter_http.OrderedHeaders, body []byte) {
	bodyBytes := []byte(a.bodyText)
	hdrs := filter_http.OrderedHeaders{
		{Name: "Content-Type", Value: "text/plain"},
		{Name: "Content-Length", Value: strconv.Itoa(len(bodyBytes))},
		{Name: "Server", Value: serverHeader()},
		{Name: "Date", Value: dateHeader()},
	}
	for _, eh := range a.extraHeaders {
		replaced := false
		for i := range hdrs {
			if strings.EqualFold(hdrs[i].Name, eh.Name) {
				hdrs[i].Value = eh.Value
				replaced = true
				break
			}
		}
		if !replaced {
			hdrs = append(hdrs, filter_http.HeaderField{Name: eh.Name, Value: eh.Value})
		}
	}
	return a.status, hdrs, bodyBytes
}

// writeH1 is the H1 adapter — writes HTTP/1.1 wire bytes by delegating to
// writeStatusReply (phase-04 preserved byte-for-byte).
func (a *directResponseAction) writeH1(w io.Writer) error {
	return writeStatusReply(w, a.status, a.bodyText)
}

// writeH2 is the H2 adapter — writes HEADERS (`:status` pseudo first per
// RFC 9113 §8.3, then regular headers in deterministic order) + DATA + END_STREAM
// via the streamWriter. Phase 07.1 Task 19 (I-3 prereq): hdrs is OrderedHeaders;
// .Get is the carrier-friendly accessor mirroring http.Header.Get's
// canonical-name semantics.
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

// do is the legacy direct-call shape preserved for the routeAction interface
// (H2 dispatch + tests still reach it directly). The H1 dispatch path
// post-Task-15 invokes directResponseAction via the chain-mediated path —
// asRouterAction returns a router.Action closure that calls writeH1 and
// surfaces (status, bytesSent, picked={}, err) to HCM dispatch's
// chain-completion hook (where the access-log emit fires per Decision §3.1).
//
// Phase 06.1 Task 11: returns (a.status, error). The configured direct_response
// status is the finalized response code; runConnection Inc's the matching
// downstream_rq_<Nxx> bucket from this return.
//
// Phase 07.1 Task 15: access-log emit-deferral hook REMOVED per Decision §3.1.
func (a *directResponseAction) do(_ context.Context, _ *http.Request, bw *bufio.Writer) (int, error) {
	return a.status, a.writeH1(bw)
}

// asRouterAction returns a router.Action closure that surfaces the
// direct-response shape as a logical ActionResponse for the chain-mediated H1
// dispatch path (Task 15). Phase 07.1 Task 18 prereq P1: wire-bytes
// serialization moved out of the action — HCM dispatch's chain-completion
// path runs the response through the encode chain THEN writes wire bytes.
// The Action closure is now a pure logical-response builder.
func (a *directResponseAction) asRouterAction() router.Action {
	return func(_ context.Context, _ *http.Request) (router.ActionResponse, cluster.Endpoint, error) {
		_, hdrs, body := a.body()
		return router.ActionResponse{
			Status:  a.status,
			Headers: hdrs,
			Body:    body,
		}, cluster.Endpoint{}, nil
	}
}

// asRouterActionH2 returns a router.H2Action closure for the chain-mediated
// H2 dispatch path (Task 16). Same logical-response shape as asRouterAction;
// HCM h2dispatch's chain-completion path runs the encode chain THEN writes
// HEADERS+DATA frames.
func (a *directResponseAction) asRouterActionH2() router.H2Action {
	return func(_ context.Context, _ h2.H2Request) (router.ActionResponse, cluster.Endpoint, error) {
		_, hdrs, body := a.body()
		return router.ActionResponse{
			Status:  a.status,
			Headers: hdrs,
			Body:    body,
		}, cluster.Endpoint{}, nil
	}
}

// clusterRouteAction is the H1 cluster-dial bridge introduced at Task 15.
// It wraps a *cluster.Cluster handle and satisfies the routeAction interface
// by delegating BOTH do() and asRouterAction() to the canonical H1
// upstream-driving logic in internal/filter/http/router. The router-package
// holds the byte-preserved upstream-dial / req.Write / ReadResponse /
// resp.Write loop migrated at Task 11; this bridge is the seam HCM dispatch
// uses to plumb cluster-routed actions into the chain-mediated dispatch path.
//
// Replaces the deleted *routerAction type that lived in actions.go pre-Task-12.
// The H2-side equivalent (clusterRouteActionH2 wrapping routerActionH2) is
// Task 16's territory; the H2 type-switch in h2dispatch.go:62,119 stays
// dangling until Task 16 lands.
type clusterRouteAction struct {
	cluster      *cluster.Cluster
	hashPolicies []router.HashPolicy // 36.2: route RouteAction.hash_policy producer (ADR-0237)
	subsetMatch  cluster.SubsetMatch // 38.1: route-static metadata_match; empty when no metadata_match (ADR-0239)
}

// do invokes the per-request cluster-dial action via the router-package
// closure. Phase 07.1 Task 18 prereq P1: the action no longer takes a
// *bufio.Writer; instead it surfaces an ActionResponse and HCM dispatch
// writes wire bytes. The do method preserves the legacy direct-call shape
// (status int + error) for the routeAction interface; the chain-mediated H1
// path goes through asRouterAction(), not do().
func (a *clusterRouteAction) do(ctx context.Context, req *http.Request, bw *bufio.Writer) (int, error) {
	resp, _, err := router.H1ClusterAction(a.cluster, a.hashPolicies, a.subsetMatch)(ctx, req)
	if err != nil {
		return resp.Status, err
	}
	// The legacy direct-call path emits wire bytes via bw. Phase 07.1's
	// chain-mediated dispatch goes through dispatchRequest which runs the
	// encode chain + does its own wire-write — not this method. Preserved
	// for the routeAction interface only; the HCM dispatch loop never
	// invokes this on the H1 path post-Task-15.
	if err := writeStatusReply(bw, resp.Status, string(resp.Body)); err != nil {
		return resp.Status, err
	}
	if resp.Close {
		return resp.Status, errCloseAfterAction
	}
	return resp.Status, nil
}

// asRouterAction returns the router.Action closure built by the router
// package's H1ClusterAction constructor. This is the seam HCM dispatch's
// chain-mediated H1 path (Task 15 connection.go) plumbs into the terminal
// router filter via *Filter.SetAction.
func (a *clusterRouteAction) asRouterAction() router.Action {
	return router.H1ClusterAction(a.cluster, a.hashPolicies, a.subsetMatch)
}

// asRouterActionH2 returns the router.H2Action closure built by the router
// package's H2ClusterAction constructor. This is the seam HCM h2dispatch.go's
// chain-mediated H2 path (Task 16) plumbs into the terminal router filter via
// *Filter.SetH2Action. The same clusterRouteAction satisfies BOTH H1 and H2
// dispatch paths; HCM dispatch picks which asRouterAction* to invoke based on
// the listener's negotiated codec — H1 listeners go through asRouterAction,
// H2 listeners through asRouterActionH2. This collapses the phase-05.2
// H1/H2 routerAction variant selection into a single bridge type whose
// router-package backend handles both protocols.
func (a *clusterRouteAction) asRouterActionH2() router.H2Action {
	return router.H2ClusterAction(a.cluster, a.hashPolicies, a.subsetMatch)
}
