package hcm

import (
	"bufio"
	"context"
	"errors"
	"net/http"

	"github.com/esalaine/envoy-go/internal/cluster"
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

// directResponseAction synthesizes a local-reply HTTP/1.1 response. body is
// the inline_string contents. status must be in [100, 599] (validated at
// config-parse time per SPEC §2). direct_response participates in keep-alive;
// it never returns errCloseAfterAction.
type directResponseAction struct {
	status int
	body   string
}

func (a *directResponseAction) do(_ context.Context, _ *http.Request, bw *bufio.Writer) error {
	return writeStatusReply(bw, a.status, a.body)
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

func (a *routerAction) do(ctx context.Context, req *http.Request, bw *bufio.Writer) error {
	upstream, err := a.cluster.Dial(ctx)
	if err != nil {
		return writeStatusReply(bw, 503, "")
	}
	defer func() { _ = upstream.Close() }()

	if err := req.Write(upstream); err != nil {
		return writeStatusReply(bw, 502, "")
	}

	resp, err := http.ReadResponse(bufio.NewReader(upstream), req)
	if err != nil {
		return writeStatusReply(bw, 502, "")
	}
	defer func() { _ = resp.Body.Close() }()

	return resp.Write(bw)
}
