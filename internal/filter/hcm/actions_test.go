package hcm

import (
	"bufio"
	"bytes"
	"context"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"golang.org/x/net/http2/hpack"

	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
)

func TestDirectResponseAction_Do(t *testing.T) {
	a := &directResponseAction{status: 200, bodyText: "OK\n"}
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := a.do(context.Background(), &http.Request{}, bw); err != nil {
		t.Fatalf("do: %v", err)
	}
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "HTTP/1.1 200 OK\r\n") {
		t.Errorf("expected 200 OK status line, got: %q", out)
	}
	if !strings.HasSuffix(out, "OK\n") {
		t.Errorf("expected body 'OK\\n' suffix, got: %q", out)
	}
}

func TestDirectResponseWriteH1_GoldenCompat(t *testing.T) {
	a := &directResponseAction{status: 200, bodyText: "OK\n"}
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if err := a.writeH1(bw); err != nil {
		t.Fatalf("writeH1 = %v", err)
	}
	_ = bw.Flush()
	got := regexp.MustCompile(`(?m)^Date: .+$`).ReplaceAllString(buf.String(), "Date: <DATE>")
	wantBytes, err := os.ReadFile("testdata/direct_response_h1.golden")
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(wantBytes) {
		t.Errorf("writeH1 output diverged from phase-04 golden:\nGOT:\n%s\nWANT:\n%s", got, wantBytes)
	}
}

type captureSW struct {
	headerCalls [][]hpack.HeaderField
	dataCalls   [][]byte
	endStream   []bool
}

func (c *captureSW) WriteHeaders(headers []hpack.HeaderField, endStream bool) error {
	c.headerCalls = append(c.headerCalls, headers)
	c.endStream = append(c.endStream, endStream)
	return nil
}
func (c *captureSW) WriteData(b []byte, endStream bool) error {
	c.dataCalls = append(c.dataCalls, append([]byte(nil), b...))
	c.endStream = append(c.endStream, endStream)
	return nil
}

func TestDirectResponseWriteH2_HEADERSThenDATAEndStream(t *testing.T) {
	a := &directResponseAction{status: 200, bodyText: "OK\n"}
	sw := &captureSW{}
	if err := a.writeH2(sw); err != nil {
		t.Fatalf("writeH2 = %v", err)
	}
	if len(sw.headerCalls) != 1 || len(sw.dataCalls) != 1 {
		t.Fatalf("got %d header calls + %d data calls; want 1 + 1", len(sw.headerCalls), len(sw.dataCalls))
	}
	hdrs := sw.headerCalls[0]
	if hdrs[0].Name != ":status" || hdrs[0].Value != "200" {
		t.Errorf("first header = %+v, want :status=200", hdrs[0])
	}
	// Verify regular headers are present and after pseudo-headers.
	wantNames := map[string]bool{"date": false, "server": false, "content-type": false, "content-length": false}
	for _, h := range hdrs[1:] {
		if h.Name[0] == ':' {
			t.Errorf("pseudo-header %q after regular headers (RFC 9113 §8.3 violation)", h.Name)
		}
		if _, want := wantNames[h.Name]; want {
			wantNames[h.Name] = true
		}
	}
	for name, seen := range wantNames {
		if !seen {
			t.Errorf("missing regular header %q", name)
		}
	}
	if string(sw.dataCalls[0]) != "OK\n" {
		t.Errorf("data = %q, want %q", sw.dataCalls[0], "OK\n")
	}
	// END_STREAM must be set on the DATA frame (the last call), not on HEADERS
	// in this test (because there's a body).
	if sw.endStream[0] /* HEADERS endStream */ {
		t.Errorf("HEADERS frame had endStream=true; expected false (body follows)")
	}
	if !sw.endStream[1] /* DATA endStream */ {
		t.Errorf("DATA frame had endStream=false; expected true (last frame)")
	}
}

// ---------------------------------------------------------------------------
// Phase 14 Task 14 — directResponseAction.extraHeaders (response_headers_to_add)
// ---------------------------------------------------------------------------

// TestDirectResponseAction_ExtraHeaders_OverwriteContentType asserts that an
// extraHeader keyed `Content-Type` REPLACES the hardcoded `text/plain`
// default (OVERWRITE_IF_EXISTS_OR_ADD semantics). Without this behavior the
// compressor filter at EncodeHeaders time would see `text/plain` (which
// matches the default 8-entry content_type list per SPEC §11.1) on EVERY
// direct_response route — masking the `image/png` content_type_mismatch
// skip path required by fixture 0016 scenario 2.
func TestDirectResponseAction_ExtraHeaders_OverwriteContentType(t *testing.T) {
	a := &directResponseAction{
		status:   200,
		bodyText: "<png-bytes>",
		extraHeaders: filter_http.OrderedHeaders{
			{Name: "Content-Type", Value: "image/png"},
		},
	}
	_, hdrs, _ := a.body()
	if got := hdrs.Get("Content-Type"); got != "image/png" {
		t.Errorf("Content-Type = %q, want %q", got, "image/png")
	}
	// Other default headers preserved.
	if got := hdrs.Get("Content-Length"); got == "" {
		t.Errorf("Content-Length is empty; expected default")
	}
	if got := hdrs.Get("Server"); got == "" {
		t.Errorf("Server is empty; expected default")
	}
}

// TestDirectResponseAction_ExtraHeaders_AppendETag asserts that an
// extraHeader keyed `ETag` (not present in the 4 default headers) is
// APPENDED to the output set. Fixture 0016 scenario 4 depends on this
// (the strong-ETag-strip + compressed-passthrough path requires ETag be
// PRESENT before the compressor's mode-a strip fires).
func TestDirectResponseAction_ExtraHeaders_AppendETag(t *testing.T) {
	a := &directResponseAction{
		status:   200,
		bodyText: "OK\n",
		extraHeaders: filter_http.OrderedHeaders{
			{Name: "ETag", Value: `"abc"`},
		},
	}
	_, hdrs, _ := a.body()
	if got := hdrs.Get("ETag"); got != `"abc"` {
		t.Errorf("ETag = %q, want %q", got, `"abc"`)
	}
	// Original 4 defaults preserved.
	if got := hdrs.Get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type = %q, want default text/plain", got)
	}
}

// TestDirectResponseAction_ExtraHeaders_NilNoop asserts that the historic
// 2-field constructor (status + bodyText, no extraHeaders) is unaffected —
// pre-Task-14 callsites continue to produce identical output.
func TestDirectResponseAction_ExtraHeaders_NilNoop(t *testing.T) {
	a := &directResponseAction{status: 200, bodyText: "OK\n"}
	_, hdrs, _ := a.body()
	if len(hdrs) != 4 {
		t.Errorf("expected 4 default headers; got %d: %+v", len(hdrs), hdrs)
	}
	if got := hdrs.Get("Content-Type"); got != "text/plain" {
		t.Errorf("Content-Type = %q, want text/plain", got)
	}
}

// TestBuildExtraResponseHeaders_OverwriteAction parses a HeaderValueOption
// list with append_action=OVERWRITE_IF_EXISTS_OR_ADD and asserts the
// resulting OrderedHeaders carry the (key, value) pairs in insertion order.
func TestBuildExtraResponseHeaders_OverwriteAction(t *testing.T) {
	opts := []*corev3.HeaderValueOption{
		{
			AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			Header:       &corev3.HeaderValue{Key: "content-type", Value: "image/png"},
		},
		{
			AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			Header:       &corev3.HeaderValue{Key: "etag", Value: `"abc"`},
		},
	}
	got, err := buildExtraResponseHeaders(opts)
	if err != nil {
		t.Fatalf("buildExtraResponseHeaders: %v", err)
	}
	want := filter_http.OrderedHeaders{
		{Name: "content-type", Value: "image/png"},
		{Name: "etag", Value: `"abc"`},
	}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %d, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("entry[%d]: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestBuildExtraResponseHeaders_NilInput returns (nil, nil) per the contract.
func TestBuildExtraResponseHeaders_NilInput(t *testing.T) {
	got, err := buildExtraResponseHeaders(nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != nil {
		t.Errorf("nil input → expected nil out; got %+v", got)
	}
}

// TestBuildExtraResponseHeaders_AppendActionRejected asserts the default
// APPEND_IF_EXISTS_OR_ADD action (proto default = 0) is rejected. Fixture
// YAMLs MUST explicitly set append_action: OVERWRITE_IF_EXISTS_OR_ADD per
// the actions.go directResponseAction GoDoc.
func TestBuildExtraResponseHeaders_AppendActionRejected(t *testing.T) {
	opts := []*corev3.HeaderValueOption{
		{
			AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
			Header:       &corev3.HeaderValue{Key: "x-foo", Value: "bar"},
		},
	}
	if _, err := buildExtraResponseHeaders(opts); err == nil {
		t.Fatalf("expected error on APPEND_IF_EXISTS_OR_ADD; got nil")
	}
}

// TestBuildExtraResponseHeaders_SkipsNilEntries silently skips entries with
// nil Header or empty key (defensive against malformed config — Envoy
// rejects at config-load; envoy-go is permissive but lossy).
func TestBuildExtraResponseHeaders_SkipsNilEntries(t *testing.T) {
	opts := []*corev3.HeaderValueOption{
		nil, // outer nil
		{
			AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			Header:       nil, // inner nil header
		},
		{
			AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			Header:       &corev3.HeaderValue{Key: "", Value: "v"}, // empty key
		},
		{
			AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
			Header:       &corev3.HeaderValue{Key: "x-keep", Value: "kept"},
		},
	}
	got, err := buildExtraResponseHeaders(opts)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(got) != 1 || got[0].Name != "x-keep" || got[0].Value != "kept" {
		t.Errorf("expected 1 surviving entry (x-keep=kept); got %+v", got)
	}
}

// ---------------------------------------------------------------------------
// Phase 38.2 Task 5 — weightedClusterRouteAction bridge (ADR-0241)
// ---------------------------------------------------------------------------

// TestWeightedClusterRouteAction_SatisfiesInterface asserts that
// *weightedClusterRouteAction satisfies the routeAction interface. This is a
// compile-time assertion: if the struct or any method is missing, the test
// fails with "undefined: weightedClusterRouteAction" or an interface mismatch.
func TestWeightedClusterRouteAction_SatisfiesInterface(t *testing.T) {
	var _ routeAction = (*weightedClusterRouteAction)(nil)
}

// ---------------------------------------------------------------------------
// Phase 07.1 Task 15 — emit-deferral migration to chain-completion (Decision §3.1)
// ---------------------------------------------------------------------------
//
// The Phase 06.2 H1 emit-deferral tests (TestDirectResponseAction_EmitsAccessLog,
// TestDirectResponseAction_NilFilter_DoesNotPanic) asserted that
// directResponseAction.do emitted the access log on its deferred site. Per
// Decision §3.1 in the 07.1 PLAN, the access-log emit moves from the four
// per-action emit sites (directResponseAction.do, routerAction.do,
// h2DirectResponseAdapter.WriteH2, routerActionH2.doH2) to a single uniform
// chain-completion site in HCM dispatch. The H1 emit-from-chain-completion
// is asserted by TestDispatchRequest_ChainMediatedAccessLogEmit in
// connection_test.go (the new home of the emit-deferral assertion).
//
// The two pre-Task-15 tests above are DELETED — their assertion shape (action
// owns the emit) is no longer valid. Replacement coverage:
//   - TestDispatchRequest_ChainMediatedAccessLogEmit (connection_test.go) —
//     asserts the chain-mediated emit fires with correct ResponseCode +
//     BytesSent + empty UpstreamHost on a direct_response.
//   - TestDispatchRequest_DirectResponseRunsChain (connection_test.go) —
//     asserts the chain-mediated path produces the byte-equivalent wire
//     output relative to the legacy direct-call shape.
