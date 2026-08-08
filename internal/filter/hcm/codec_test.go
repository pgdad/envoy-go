package hcm

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/http2/hpack"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/filter/hcm/h2"
	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filter/http/router"
)

func TestServerHeader(t *testing.T) {
	if got := serverHeader(); got != "envoy" {
		t.Errorf("serverHeader() = %q, want %q", got, "envoy")
	}
}

func TestDateHeader(t *testing.T) {
	got := dateHeader()
	if _, err := time.Parse(http.TimeFormat, got); err != nil {
		t.Errorf("dateHeader() = %q is not RFC 7231 IMF-fixdate parseable: %v", got, err)
	}
}

func TestWriteStatusReply(t *testing.T) {
	tests := []struct {
		name       string
		status     int
		body       string
		wantStatus string
		wantCLen   string
	}{
		{"200 OK with body", 200, "OK\n", "HTTP/1.1 200 OK\r\n", "Content-Length: 3\r\n"},
		{"400 Bad Request empty body", 400, "", "HTTP/1.1 400 Bad Request\r\n", "Content-Length: 0\r\n"},
		{"404 Not Found", 404, "not found\n", "HTTP/1.1 404 Not Found\r\n", "Content-Length: 10\r\n"},
		{"417 Expectation Failed empty", 417, "", "HTTP/1.1 417 Expectation Failed\r\n", "Content-Length: 0\r\n"},
		{"500 Internal Server Error empty", 500, "", "HTTP/1.1 500 Internal Server Error\r\n", "Content-Length: 0\r\n"},
		{"502 Bad Gateway empty", 502, "", "HTTP/1.1 502 Bad Gateway\r\n", "Content-Length: 0\r\n"},
		{"503 Service Unavailable empty", 503, "", "HTTP/1.1 503 Service Unavailable\r\n", "Content-Length: 0\r\n"},
		{"501 Not Implemented empty", 501, "", "HTTP/1.1 501 Not Implemented\r\n", "Content-Length: 0\r\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeStatusReply(&buf, tc.status, tc.body); err != nil {
				t.Fatalf("writeStatusReply: %v", err)
			}
			out := buf.String()
			if !strings.HasPrefix(out, tc.wantStatus) {
				t.Errorf("status line:\n  got:  %q\n  want prefix: %q", out, tc.wantStatus)
			}
			if !strings.Contains(out, tc.wantCLen) {
				t.Errorf("missing %q in:\n%s", tc.wantCLen, out)
			}
			if !strings.Contains(out, "Server: envoy\r\n") {
				t.Errorf("missing Server header in:\n%s", out)
			}
			if !strings.Contains(out, "Content-Type: text/plain\r\n") {
				t.Errorf("missing Content-Type header in:\n%s", out)
			}
			if !strings.Contains(out, "Date: ") {
				t.Errorf("missing Date header in:\n%s", out)
			}
			if tc.body != "" {
				idx := strings.Index(out, "\r\n\r\n")
				if idx < 0 || out[idx+4:] != tc.body {
					t.Errorf("body mismatch: got %q, want %q", out[idx+4:], tc.body)
				}
			}
		})
	}
}

func TestWriteStatusReply_UnknownStatusFallsBackToEmptyReason(t *testing.T) {
	var buf bytes.Buffer
	if err := writeStatusReply(&buf, 999, ""); err != nil {
		t.Fatalf("writeStatusReply: %v", err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "HTTP/1.1 999 \r\n") {
		t.Errorf("expected empty reason phrase for unknown status, got:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// Phase 84.1 Task 10 — the H1 BEHAVIORAL non-change regression, paired with an
// H2 positive arm IN THIS FILE.
// ---------------------------------------------------------------------------
//
// ⚠️ WHY THE PAIR IS IN ONE FILE. An H1 "no trailers emitted" assertion is
// green under TWO different worlds: (a) ActionResponse.Trailers is populated
// and the H1 wire writer correctly ignores it — the contract this row lands;
// and (b) Trailers is never populated or never emitted ANYWHERE, i.e. the
// whole feature is dead. A one-sided gate cannot tell those apart
// (reference_one_sided_gate_for_a_two_sided_fix,
// reference_liveness_break_needs_failing_baseline). The H2 positive arm at the
// bottom of this file is the failing baseline: it reddens the moment trailers
// stop reaching the wire, which is exactly world (b). Read the two tests as
// ONE gate.
//
// writeH1Reply's signature is pinned to FOUR arguments below: an ActionResponse
// trailing block is structurally unable to reach the H1 wire writer, so the
// non-change test drives the CALLER (dispatchRequest → writeH1Reply) with a
// Trailers-populated ActionResponse and compares the FULL serialized bytes.

// writeH1Reply stays 4-arg (codec.go). This compile-time pin fails the build if
// a later change widens it with a trailer block, which is the shape phase 84.1
// deliberately did NOT give it (the trailing block is H2-only; H1/H3 ignore).
var _ func(io.Writer, int, filter_http.OrderedHeaders, []byte) error = writeH1Reply

// h1TrailersFixedDate pins the Date header the trailer-carrying action below
// supplies. writeH1Reply synthesizes Date from time.Now() only when the header
// carrier does NOT already carry one, so supplying it makes the serialized
// bytes fully deterministic and the byte-for-byte comparison exact rather than
// clock-dependent. Nothing about the property under test (a trailer section
// appended at the TAIL) depends on the Date value.
const h1TrailersFixedDate = "Mon, 02 Jan 2006 15:04:05 GMT"

// h1TrailersBody is the response body both arms carry. Non-empty on purpose:
// a naive H1 trailer emit would have to append its section AFTER these bytes,
// which is precisely the region a header-map assertion never reaches.
const h1TrailersBody = "hello"

// trailerCarryingAction is a routeAction whose H1-flavored Action
// (asRouterAction — the closure BOTH dispatchRequest and runH3 install into the
// terminal router filter) returns an ActionResponse carrying a caller-chosen
// trailing block. It exists so the H1/H3 non-change tests can drive a
// Trailers-populated ActionResponse all the way to the wire writer without
// standing up an H2 upstream (the production populator is doH2ClusterAction,
// phase 84.1 Task 5).
//
// Distinct from h2dispatch_test.go's trailerStubAction, whose H1 arm
// deliberately returns NO trailers — this one is the H1/H3 counterpart and
// populates the H1-flavored arm precisely so the ignore-path can be observed.
type trailerCarryingAction struct {
	trailers []hpack.HeaderField
}

func (a *trailerCarryingAction) response() router.ActionResponse {
	return router.ActionResponse{
		Status: 200,
		Headers: filter_http.OrderedHeaders{
			{Name: "Content-Type", Value: "application/grpc"},
			{Name: "Content-Length", Value: "5"},
			{Name: "Server", Value: "envoy"},
			{Name: "Date", Value: h1TrailersFixedDate},
		},
		Body:     []byte(h1TrailersBody),
		Trailers: a.trailers,
	}
}

func (a *trailerCarryingAction) asRouterAction() router.Action {
	return func(context.Context, *http.Request) (router.ActionResponse, cluster.Endpoint, error) {
		return a.response(), cluster.Endpoint{}, nil
	}
}

func (a *trailerCarryingAction) asRouterActionH2() router.H2Action {
	return func(context.Context, h2.H2Request) (router.ActionResponse, cluster.Endpoint, error) {
		return a.response(), cluster.Endpoint{}, nil
	}
}

// runH1TrailerDispatch drives one H1 request through dispatchRequest against a
// route whose action returns an ActionResponse with the given trailing block,
// and returns the COMPLETE serialized HTTP/1.1 response bytes.
func runH1TrailerDispatch(t *testing.T, trailers []hpack.HeaderField) []byte {
	t.Helper()
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/grpc"), action: &trailerCarryingAction{trailers: trailers}},
	}}
	f := mkFilterForTable(t, tt)

	req, err := http.NewRequest("POST", "/grpc", nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Proto = "HTTP/1.1"

	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	status, derr := f.dispatchRequest(context.Background(), nil, req, bw)
	if derr != nil {
		t.Fatalf("dispatchRequest: %v", derr)
	}
	if status != 200 {
		t.Fatalf("dispatchRequest status = %d, want 200", status)
	}
	if ferr := bw.Flush(); ferr != nil {
		t.Fatalf("bufio Flush: %v", ferr)
	}
	return buf.Bytes()
}

// TestDispatchRequest_H1_TrailersProduceIdenticalBytes is the phase-84.1
// Task 10 behavioral non-change regression. An ActionResponse carrying a
// non-empty trailing block must serialize on the H1 wire BYTE-FOR-BYTE
// identically to the same response with Trailers nil.
//
// ⚠️ The comparison is over the FULL serialized bytes, not a header subset. An
// H1 trailer section (chunked Transfer-Encoding + a trailer block after the
// zero-length chunk) appears at the TAIL of the response, downstream of every
// header a header-map assertion can see; only a whole-response comparison
// reaches it.
//
// Three independent properties, one t.Errorf each (a t.Fatalf would make the
// later ones dead code):
//  1. the two arms are byte-equal;
//  2. neither arm carries the trailer field names anywhere in its bytes — the
//     stacked control, which catches a break that appends a trailer section to
//     BOTH arms (property 1 alone would stay green on that);
//  3. the trailers arm ends exactly at the body — nothing is appended after it.
func TestDispatchRequest_H1_TrailersProduceIdenticalBytes(t *testing.T) {
	trailerBlock := []hpack.HeaderField{
		{Name: "grpc-status", Value: "0"},
		{Name: "grpc-message", Value: "ok"},
	}

	withTrailers := runH1TrailerDispatch(t, trailerBlock)
	withoutTrailers := runH1TrailerDispatch(t, nil)

	if !bytes.Equal(withTrailers, withoutTrailers) {
		t.Errorf("H1 wire bytes differ when ActionResponse.Trailers is populated.\n with trailers (%d bytes): %q\n  nil trailers (%d bytes): %q",
			len(withTrailers), withTrailers, len(withoutTrailers), withoutTrailers)
	}

	for _, arm := range []struct {
		name  string
		bytes []byte
	}{
		{"trailers-populated", withTrailers},
		{"trailers-nil", withoutTrailers},
	} {
		for _, tf := range trailerBlock {
			if bytes.Contains(arm.bytes, []byte(tf.Name)) {
				t.Errorf("%s arm: H1 wire bytes contain trailer field name %q; H1 must never serialize a trailing block. full response: %q",
					arm.name, tf.Name, arm.bytes)
			}
		}
	}

	if !bytes.HasSuffix(withTrailers, []byte(h1TrailersBody)) {
		t.Errorf("H1 wire bytes do not end at the response body — something was appended after it. full response: %q", withTrailers)
	}
}

// TestH2Dispatch_TrailersReachTheWire is the POSITIVE ARM that makes the H1
// non-change test above two-sided. It is deliberately in THIS FILE.
//
// It drives the H2 dispatch path (h2Dispatcher.Match → chainDispatchAction.
// WriteH2 → writeH2Reply) with an action that populates
// ActionResponse.Trailers, and asserts the trailing HEADERS block actually
// reaches the wire carrying END_STREAM. If the trailing emit is removed, the
// dispatch call site stops forwarding resp.Trailers, or the carrier stops
// being consumed, THIS test reddens while the H1 non-change test above stays
// green — which is exactly the discrimination a one-sided gate lacks.
//
// Compact by design: one cell (body + trailers). The full
// {trailers, no trailers} x {body, no body} matrix lives in
// h2dispatch_test.go's TestWriteH2Reply_FrameSequence (Task 7); this is a
// liveness baseline, not a second copy of it.
func TestH2Dispatch_TrailersReachTheWire(t *testing.T) {
	trailerBlock := []hpack.HeaderField{
		{Name: "grpc-status", Value: "0"},
		{Name: "grpc-message", Value: "ok"},
	}

	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/grpc"), action: &trailerStubAction{body: []byte("hello"), trailers: trailerBlock}},
	}}
	f := newH2DispatchFilter(t, tt, routerOnlyChain(t), nil /* no sinks */)

	disp := newH2Dispatcher(f)
	req, err := http.NewRequest("POST", "/grpc", nil)
	if err != nil {
		t.Fatalf("http.NewRequest: %v", err)
	}
	req.Proto = "HTTP/2.0"
	action, ok := disp.Match(req)
	if !ok {
		t.Fatal("Match returned ok=false; want true on the matched route")
	}

	w := &captureH2Writer{}
	if werr := action.WriteH2(context.Background(), h2.H2Request{Method: "POST", Path: "/grpc", Authority: "localhost"}, w); werr != nil {
		t.Fatalf("WriteH2: %v", werr)
	}

	if got := strings.Join(w.order, ","); got != "headers,data,headers" {
		t.Errorf("H2 frame order = [%s]; want [headers,data,headers] — the trailing HEADERS block is not reaching the wire", got)
	}
	if len(w.headers) != 2 {
		t.Fatalf("HEADERS frame count = %d; want 2 (response block + trailing block)", len(w.headers))
	}
	got := w.headers[1]
	if len(got) != len(trailerBlock) {
		t.Fatalf("trailing block = %v (len %d); want %v (len %d)", got, len(got), trailerBlock, len(trailerBlock))
	}
	for i := range got {
		if got[i] != trailerBlock[i] {
			t.Errorf("trailing block[%d] = %+v; want %+v", i, got[i], trailerBlock[i])
		}
	}
	if n := len(w.endStream); n != 3 {
		t.Fatalf("end_stream flag count = %d (%v); want 3", n, w.endStream)
	}
	if !w.endStream[2] {
		t.Errorf("trailing HEADERS block end_stream = false; want true (it must terminate the stream). full seq %v", w.endStream)
	}
}
