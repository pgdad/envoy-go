package hcm

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

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
	filter   *Filter // set post-build by routeTable.bindFilter; nil when no sinks configured.
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
//
// Phase 06.2 Task 12: emits access-log record via a.filter.emitAccessLog on
// return. bytesSent is len(bodyText) — the wire bytes for the inline body per
// SPEC §12 #3 (direct_response writes no upstream bytes; only the local body).
func (a *directResponseAction) do(_ context.Context, req *http.Request, bw *bufio.Writer) (int, error) {
	start := time.Now()
	defer func() {
		if a.filter != nil {
			a.filter.emitAccessLog(req, a.status, int64(len(a.bodyText)), cluster.Endpoint{}, start)
		}
	}()
	return a.status, a.writeH1(bw)
}
