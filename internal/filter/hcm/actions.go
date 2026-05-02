package hcm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
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

// asRouterAction returns a router.Action closure wrapping a.writeH1 for the
// chain-mediated H1 dispatch path (Task 15). The closure surfaces
// (status, bytesSent=len(bodyText), picked=zero, err=writeH1 error) so HCM
// dispatch's chain-completion hook can populate the access-log record per
// SPEC §12 #3 + Decision §3.1.
func (a *directResponseAction) asRouterAction() router.Action {
	return func(_ context.Context, _ *http.Request, bw *bufio.Writer) (int, int64, cluster.Endpoint, error) {
		err := a.writeH1(bw)
		return a.status, int64(len(a.bodyText)), cluster.Endpoint{}, err
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
	cluster *cluster.Cluster
}

// do invokes the per-request cluster-dial action via the router-package
// closure and discards the bytesSent/picked surface (the legacy direct-call
// shape returns only status + err per the routeAction interface). HCM
// dispatch post-Task-15 does NOT call do() on the H1 path — it calls
// asRouterAction() and runs the action through the chain — so do()'s return
// is informational here. Preserved to satisfy the routeAction interface
// for symmetry with directResponseAction.
func (a *clusterRouteAction) do(ctx context.Context, req *http.Request, bw *bufio.Writer) (int, error) {
	status, _, _, err := router.H1ClusterAction(a.cluster)(ctx, req, bw)
	return status, err
}

// asRouterAction returns the router.Action closure built by the router
// package's H1ClusterAction constructor. This is the seam HCM dispatch's
// chain-mediated H1 path (Task 15 connection.go) plumbs into the terminal
// router filter via *Filter.SetAction.
func (a *clusterRouteAction) asRouterAction() router.Action {
	return router.H1ClusterAction(a.cluster)
}
