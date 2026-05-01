package router

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/cluster"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// TypeURL is the canonical envoy.filters.http.router type URL. Boot wiring
// in cmd/envoy-go/main.go registers New under this key in the HTTPRegistry
// (per ADR-0072) at Task 20.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.router.v3.Router"

// bad502Body is the local-reply body for upstream-failure 502 emissions per
// SPEC §11.9. The constant is the single source of truth so the H2 router's
// 502 prose cannot drift from any future H1 router 502 prose; phase-04's H1
// 502 currently uses an empty body via writeStatusReply(bw, 502, "") and is
// NOT touched by 05.2 (no regression). The shared-constant pattern future-
// proofs the migration. Migrated verbatim from internal/filter/hcm/actions.go
// at phase 07.1 Task 11; the duplication with the hcm-package constant is
// intentional per the PLAN's Task 11 → Task 12 split.
const bad502Body = "bad gateway\n"

// errCloseAfterAction is returned by routerAction.do when the action's
// response carried Connection: close (or the equivalent semantic on the
// upstream-routed response). The connection loop checks for this sentinel
// via errors.Is and closes the downstream after the current iteration.
//
// SPEC §10 #3 settled to the sentinel-error mechanism (option (a)). Other
// non-nil errors from do trigger downstream close + log (the connection
// loop handles).
var errCloseAfterAction = errors.New("router: action requested connection close")

// serverHeader returns the canonical Server header value for HCM-locally-
// generated responses. ADR-0014 (admin /ready precedent) reaffirmed for HCM
// per ADR-0044 / SPEC §10 #12 settled. Duplicated from internal/filter/hcm/codec.go
// at phase 07.1 Task 11 so the router package's locally-synthesized error
// responses do not require a cross-package call.
func serverHeader() string { return "envoy" }

// dateHeader returns the current Date header value formatted as RFC 7231
// IMF-fixdate (e.g. "Sun, 06 Nov 2024 08:49:37 GMT"). Per-response computation
// (no caching) per SPEC §10 #8 settled.
func dateHeader() string { return time.Now().UTC().Format(http.TimeFormat) }

// writeStatusReply writes a complete HTTP/1.1 local-reply response to w. The
// status line uses http.StatusText for the reason phrase; if the status is
// unknown to stdlib, the reason phrase is empty. Headers in fixed order:
// Content-Type, Content-Length, Server, Date. A CRLF blank line then the body.
func writeStatusReply(w io.Writer, status int, body string) error {
	reason := http.StatusText(status)
	if _, err := fmt.Fprintf(w, "HTTP/1.1 %d %s\r\n", status, reason); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w,
		"Content-Type: text/plain\r\nContent-Length: %d\r\nServer: %s\r\nDate: %s\r\n\r\n",
		len(body), serverHeader(), dateHeader()); err != nil {
		return err
	}
	if body != "" {
		if _, err := io.WriteString(w, body); err != nil {
			return err
		}
	}
	return nil
}

// upstreamHostString renders cluster.Endpoint as `host:port` for the access-log
// UPSTREAM_HOST operator. Zero-valued Endpoint (host == "") yields empty
// string; the formatter then emits the literal `-` per accesslog Decision A.
// Duplicated from internal/filter/hcm/accesslog_emit.go.
func upstreamHostString(ep cluster.Endpoint) string {
	if ep.Host == "" {
		return ""
	}
	return ep.Host + ":" + strconv.Itoa(int(ep.Port))
}

// Filter is the terminal HTTP filter (envoy.filters.http.router) implementing
// envoyhttp.StreamDecoderFilter + envoyhttp.StreamEncoderFilter per ADR-0071.
// It dispatches the resolved route action — cluster dial OR direct_response
// synthesize — driven by the chain's iteration callbacks (Tasks 13–16 wire
// HCM dispatch through to the chain).
//
// At Task 11 the iteration-protocol surface lands as a working skeleton:
// SetDecoderCallbacks/SetEncoderCallbacks store the framework-supplied
// callback structs; DecodeHeaders/Data/Trailers + EncodeHeaders/Data/Trailers
// satisfy the interface. The decode-side dispatch logic that hands off to
// routerAction.do / routerActionH2.doH2 is exercised end-to-end in Tasks
// 15+16 where HCM dispatch wires the chain.
//
// The accessLog field carries the access-log sink slice plumbed in via the
// per-request factory (Task 14 wires the HCM Filter's accessLog through to
// the router instance). Tests (`router_test.go`) exercise the access-log
// emit directly by constructing &Filter{accessLog: []accesslog.Sink{...}}
// — this matches the byte-preserved shape from internal/filter/hcm.
type Filter struct {
	dcb envoyhttp.DecoderFilterCallbacks
	ecb envoyhttp.EncoderFilterCallbacks

	// accessLog holds the configured access-log sinks plumbed through from
	// the HCM Filter (Task 14). Nil when no sinks are configured.
	accessLog []accesslog.Sink
}

// emitAccessLog constructs an accesslog.Record from H1 primitives and submits
// to each sink in f.accessLog. Per SPEC §2.1, a zero statusCode is the H2
// ctx-cancel sentinel and skips emission; H1 path never produces a zero
// statusCode in normal flow, but the guard is uniform across H1+H2 callers.
// Migrated verbatim from internal/filter/hcm/accesslog_emit.go.
func (f *Filter) emitAccessLog(r *http.Request, statusCode int, bytesSent int64, picked cluster.Endpoint, start time.Time) {
	if statusCode == 0 || len(f.accessLog) == 0 {
		return
	}
	rec := &accesslog.Record{
		StartTime:    start,
		Method:       r.Method,
		Path:         r.URL.Path,
		Protocol:     r.Proto,
		ResponseCode: statusCode,
		BytesSent:    bytesSent,
		Duration:     time.Since(start),
		Authority:    r.Host,
		UserAgent:    r.Header.Get("User-Agent"),
		UpstreamHost: upstreamHostString(picked),
	}
	for _, s := range f.accessLog {
		s.Submit(rec)
	}
}

// SetDecoderCallbacks stores the framework-supplied decoder callbacks. The
// terminal filter uses dcb.EncodeHeaders / dcb.EncodeData to dispatch the
// upstream response back through the encode chain (Task 13–16 wire-up).
func (f *Filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }

// SetEncoderCallbacks stores the framework-supplied encoder callbacks.
func (f *Filter) SetEncoderCallbacks(cb envoyhttp.EncoderFilterCallbacks) { f.ecb = cb }

// DecodeHeaders is the entry point for the terminal-filter dispatch. The
// full route-action driving (cluster.Dial → req.Write → ReadResponse → encode)
// is wired by HCM dispatch in Tasks 15+16 (which pass the resolved route
// action + cluster handle through the chain). At Task 11, the skeleton
// returns Continue so the framework's chain.RunDecodeHeaders advances past
// the router as the terminal filter; the byte-preserved tests below
// exercise routerAction.do / routerActionH2.doH2 directly.
func (f *Filter) DecodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}

// DecodeData is the body-side counterpart to DecodeHeaders. Routing dispatch
// runs once end_stream is observed (Tasks 15+16).
func (f *Filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is the trailer-side counterpart to DecodeData.
func (f *Filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// EncodeHeaders is the encode-side pass-through; the terminal filter does
// not modify upstream response headers.
func (f *Filter) EncodeHeaders(_ http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	return envoyhttp.Continue
}

// EncodeData is the encode-side body pass-through.
func (f *Filter) EncodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// EncodeTrailers is the encode-side trailer pass-through.
func (f *Filter) EncodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// OnDestroy releases per-request state. The router's per-instance state is
// callback-pointer + access-log slice; both are GC-tracked references with
// no cleanup required.
func (f *Filter) OnDestroy() {}

// New is the HTTPFilterFactory exposed at boot via filter.HTTPRegistry.Register
// (Task 20 wires the boot registration). The router proto carries no fields
// envoy-go consumes at Task 11 — every field in envoy.extensions.filters.http.router.v3.Router
// is in the silent-ignore set inherited from ADR-0040. tc may be nil (default
// router config); a non-nil tc is unmarshaled-and-discarded only to honor the
// ADR-0072 "factories validate typed_config shape" contract; payload fields
// remain silently ignored at this phase.
func New(_ *anypb.Any, _ envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
	return func() envoyhttp.HTTPFilter {
		f := &Filter{}
		return envoyhttp.HTTPFilter{
			Name:    "envoy.filters.http.router",
			Decoder: f,
			Encoder: f,
		}
	}, nil
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
//
// Migrated from internal/filter/hcm/actions.go at phase 07.1 Task 11 with
// signatures preserved byte-for-byte so the byte-preserved tests in
// router_test.go exercise the same shape (`a.do(ctx, req, bw)`).
type routerAction struct {
	cluster *cluster.Cluster
	filter  *Filter // set post-build by routeTable.bindFilter; nil when no sinks configured.
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
	start := time.Now()

	// bytesSent counts only the response body bytes for BYTES_SENT per SPEC
	// §12 #3 option (a). We do NOT wrap bw in a byteCounterWriter because
	// resp.Write writes the full HTTP/1.1 wire bytes (status line + headers +
	// body), which would inflate the count. Instead, we read the body into a
	// buffer, record its length, replace resp.Body with the buffered reader, and
	// let resp.Write drain the buffer through bw directly.
	bytesSent := int64(0)

	// statusCode and picked are captured by the deferred access-log emit so
	// the closure always reads the final values after all writes have completed.
	statusCode := 0
	picked := cluster.Endpoint{}
	if a.filter != nil {
		defer func() { a.filter.emitAccessLog(req, statusCode, bytesSent, picked, start) }()
	}

	a.cluster.IncUpstreamRqTotal()

	upstream, ep, err := a.cluster.Dial(ctx)
	if err != nil {
		a.cluster.IncStatusClass(503)
		statusCode = 503
		return 503, writeStatusReply(bw, 503, "")
	}
	defer func() { _ = upstream.Close() }()
	picked = ep

	// Propagate the downstream ctx deadline (if any) to the upstream socket
	// so a stalled upstream cannot hold the action past the ctx's deadline
	// during req.Write or http.ReadResponse — both of which are otherwise
	// ctx-unaware (REVIEW.md I-3 from REVIEW.md 04527eb).
	if dl, ok := ctx.Deadline(); ok {
		_ = upstream.SetDeadline(dl)
	}

	if err := req.Write(upstream); err != nil {
		a.cluster.IncStatusClass(502)
		statusCode = 502
		return 502, writeStatusReply(bw, 502, "")
	}

	resp, err := http.ReadResponse(bufio.NewReader(upstream), req)
	if err != nil {
		a.cluster.IncStatusClass(502)
		statusCode = 502
		return 502, writeStatusReply(bw, 502, "")
	}
	defer func() { _ = resp.Body.Close() }()

	a.cluster.IncStatusClass(resp.StatusCode)
	statusCode = resp.StatusCode

	// Read the entire response body to count body bytes for BYTES_SENT.
	// Replace resp.Body with a bytes.Reader so resp.Write drains the same
	// bytes downstream without a second upstream read.
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	bytesSent = int64(len(bodyBytes))
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

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

// Compile-time interface conformance assertions. *Filter must satisfy both
// StreamDecoderFilter + StreamEncoderFilter so the chain framework can drive
// it as a both-sides filter (terminal: decode dispatches; encode forwards).
var (
	_ envoyhttp.StreamDecoderFilter = (*Filter)(nil)
	_ envoyhttp.StreamEncoderFilter = (*Filter)(nil)
	_ envoyhttp.HTTPFilterFactory   = New
)
