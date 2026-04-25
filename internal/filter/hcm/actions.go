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
func (a *directResponseAction) do(_ context.Context, _ *http.Request, bw *bufio.Writer) error {
	return a.writeH1(bw)
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

	// Propagate the downstream ctx deadline (if any) to the upstream socket
	// so a stalled upstream cannot hold the action past the ctx's deadline
	// during req.Write or http.ReadResponse — both of which are otherwise
	// ctx-unaware (REVIEW.md I-3 from REVIEW.md 04527eb).
	if dl, ok := ctx.Deadline(); ok {
		_ = upstream.SetDeadline(dl)
	}

	if err := req.Write(upstream); err != nil {
		return writeStatusReply(bw, 502, "")
	}

	resp, err := http.ReadResponse(bufio.NewReader(upstream), req)
	if err != nil {
		return writeStatusReply(bw, 502, "")
	}
	defer func() { _ = resp.Body.Close() }()

	if err := resp.Write(bw); err != nil {
		return err
	}
	// Honor the upstream's Connection: close (and HTTP/1.0 close-by-default)
	// by signaling the connection loop via errCloseAfterAction. http.ReadResponse
	// populates resp.Close from the wire-level signals (Connection: close on
	// HTTP/1.1, default-close on HTTP/1.0). SPEC §5.3 / SPEC §10 #3 settled.
	// REVIEW.md I-1 from REVIEW.md 04527eb.
	if resp.Close {
		return errCloseAfterAction
	}
	return nil
}
