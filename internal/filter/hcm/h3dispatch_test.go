package hcm

import (
	"bytes"
	"context"
	stdtls "crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"golang.org/x/net/http2/hpack"

	filter_http "github.com/pgdad/envoy-go/internal/filter/http"
)

// mkH3DirectResponseFilter builds a minimal *Filter with a single
// direct_response route at path → (status, bodyText), reusing the H1 test
// harness mkFilterForTable (router-only chain). Shared by the runH3 dispatch
// tests below.
func mkH3DirectResponseFilter(t *testing.T, path string, status int, bodyText string) *Filter {
	t.Helper()
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath(path), action: &directResponseAction{status: status, bodyText: bodyText}},
	}}
	return mkFilterForTable(t, tt)
}

// TestRunH3_GET_DirectResponse verifies a routed GET is dispatched through the
// chain and the direct_response body + status are written to the ResponseWriter.
func TestRunH3_GET_DirectResponse(t *testing.T) {
	f := mkH3DirectResponseFilter(t, "/probe", 200, "h3-ok")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/probe", nil)
	req.TLS = &stdtls.ConnectionState{Version: stdtls.VersionTLS13, HandshakeComplete: true, NegotiatedProtocol: "h3"}
	rec := httptest.NewRecorder()
	status, err := f.runH3(req.Context(), rec, req)
	if err != nil {
		t.Fatalf("runH3: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if rec.Body.String() != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", rec.Body.String())
	}
}

// TestRunH3_NoMatch_404 verifies an unmatched path returns a 404 (mirrors the
// H1 dispatchRequest no-match branch). r.TLS is left nil to exercise the
// unit-test plaintext path — every TLS seeder must nil-tolerate.
func TestRunH3_NoMatch_404(t *testing.T) {
	f := mkH3DirectResponseFilter(t, "/probe", 200, "h3-ok")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/nope", nil)
	rec := httptest.NewRecorder()
	status, err := f.runH3(req.Context(), rec, req)
	if err != nil {
		t.Fatalf("runH3: %v", err)
	}
	if status != 404 {
		t.Errorf("status = %d, want 404", status)
	}
	// The 4xx class counter must have moved for the no-match response.
	if got := f.downstreamRq4xx.Load(); got != 1 {
		t.Errorf("downstream_rq_4xx = %d, want 1", got)
	}
}

// TestRunH3_POST_Body verifies a request WITH a body flows through the
// decode-data loop and the routed direct_response is still written.
func TestRunH3_POST_Body(t *testing.T) {
	f := mkH3DirectResponseFilter(t, "/probe", 200, "h3-ok")
	req := httptest.NewRequest(http.MethodPost, "https://example.test/probe", strings.NewReader("payload-bytes"))
	rec := httptest.NewRecorder()
	status, err := f.runH3(req.Context(), rec, req)
	if err != nil {
		t.Fatalf("runH3: %v", err)
	}
	if status != 200 {
		t.Errorf("status = %d, want 200", status)
	}
	if rec.Body.String() != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", rec.Body.String())
	}
}

// TestServeH3_GET verifies the http.HandlerFunc entry point dispatches through
// runH3 and writes the response (the quic-go http3.Server handler seam).
func TestServeH3_GET(t *testing.T) {
	f := mkH3DirectResponseFilter(t, "/probe", 200, "h3-ok")
	req := httptest.NewRequest(http.MethodGet, "https://example.test/probe", nil)
	rec := httptest.NewRecorder()
	f.ServeH3(rec, req)
	if rec.Result().StatusCode != 200 {
		t.Errorf("status = %d, want 200", rec.Result().StatusCode)
	}
	if rec.Body.String() != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", rec.Body.String())
	}
}

// TestWriteH3Reply_StatusHeadersBody verifies the ActionResponse → ResponseWriter
// projection: status code, response headers, and body are written; HTTP pseudo-
// headers (":status" etc.) are NOT leaked into the response header map.
func TestWriteH3Reply_StatusHeadersBody(t *testing.T) {
	rec := httptest.NewRecorder()
	hdrs := filter_http.OrderedHeaders{
		{Name: "content-type", Value: "text/plain"},
		{Name: "x-custom", Value: "v1"},
		{Name: ":status", Value: "200"}, // pseudo-header — must be dropped
	}
	if err := writeH3Reply(rec, 200, hdrs, []byte("h3-ok")); err != nil {
		t.Fatalf("writeH3Reply: %v", err)
	}
	res := rec.Result()
	if res.StatusCode != 200 {
		t.Errorf("status = %d, want 200", res.StatusCode)
	}
	if got := res.Header.Get("content-type"); got != "text/plain" {
		t.Errorf("content-type = %q, want text/plain", got)
	}
	if got := res.Header.Get("x-custom"); got != "v1" {
		t.Errorf("x-custom = %q, want v1", got)
	}
	if _, leaked := res.Header[":status"]; leaked {
		t.Errorf(":status pseudo-header leaked into the response header map")
	}
	if body := rec.Body.String(); body != "h3-ok" {
		t.Errorf("body = %q, want h3-ok", body)
	}
}

// TestWriteH3Reply_EmptyBody verifies a headers-only response (no body) writes
// the status with no panic and an empty body. It also verifies (phase 61.3,
// ADR-0281 §Consequences deferral) that the server: envoy fidelity header IS
// synthesized even for an empty body, but content-length is NOT (the
// reference Envoy omits content-length on a headers-only response — do not
// emit content-length: 0).
func TestWriteH3Reply_EmptyBody(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := writeH3Reply(rec, 204, nil, nil); err != nil {
		t.Fatalf("writeH3Reply: %v", err)
	}
	res := rec.Result()
	if res.StatusCode != 204 {
		t.Errorf("status = %d, want 204", res.StatusCode)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("body len = %d, want 0", rec.Body.Len())
	}
	if got := res.Header.Get("server"); got != "envoy" {
		t.Errorf("server = %q, want envoy", got)
	}
	if got := res.Header.Get("content-length"); got != "" {
		t.Errorf("content-length = %q, want empty (no content-length on empty body)", got)
	}
}

// TestWriteH3Reply_SynthesizesServerAndContentLength verifies the H3 response
// carries the fidelity headers the reference Envoy emits for a direct_response:
// server: envoy and content-length. Phase 61.3 (ADR-0282) — the 61.2 arm was
// minimal (ADR-0281 §Consequences deferred this).
func TestWriteH3Reply_SynthesizesServerAndContentLength(t *testing.T) {
	rec := httptest.NewRecorder()
	if err := writeH3Reply(rec, 200, nil, []byte("OK\n")); err != nil {
		t.Fatalf("writeH3Reply: %v", err)
	}
	res := rec.Result()
	if got := res.Header.Get("server"); got != "envoy" {
		t.Errorf("server = %q, want envoy", got)
	}
	if got := res.Header.Get("content-length"); got != "3" {
		t.Errorf("content-length = %q, want 3", got)
	}
	if rec.Body.String() != "OK\n" {
		t.Errorf("body = %q, want OK\\n", rec.Body.String())
	}
}

// TestWriteH3Reply_ActionSuppliedHeadersNotOverridden verifies that when the
// router action already supplies server/content-length header values, the
// synthesis step respects them (does not double-set / override).
func TestWriteH3Reply_ActionSuppliedHeadersNotOverridden(t *testing.T) {
	rec := httptest.NewRecorder()
	hdrs := filter_http.OrderedHeaders{
		{Name: "server", Value: "custom-server"},
		{Name: "content-length", Value: "99"},
	}
	if err := writeH3Reply(rec, 200, hdrs, []byte("OK\n")); err != nil {
		t.Fatalf("writeH3Reply: %v", err)
	}
	res := rec.Result()
	if got := res.Header.Get("server"); got != "custom-server" {
		t.Errorf("server = %q, want custom-server (action-supplied value must not be overridden)", got)
	}
	if got := res.Header.Get("content-length"); got != "99" {
		t.Errorf("content-length = %q, want 99 (action-supplied value must not be overridden)", got)
	}
}

// ---------------------------------------------------------------------------
// Phase 84.1 Task 11 — the H3 BEHAVIORAL non-change regression, paired with an
// H2 positive arm IN THIS FILE.
// ---------------------------------------------------------------------------
//
// ⚠️ WHY THE PAIR IS IN ONE FILE. An H3 "no Trailer key gained" assertion is
// green under TWO different worlds: (a) ActionResponse.Trailers is populated
// and the H3 projection correctly ignores it — the contract this row lands;
// and (b) Trailers is never populated or never emitted ANYWHERE, i.e. the
// feature is dead. A one-sided gate cannot tell those apart
// (reference_one_sided_gate_for_a_two_sided_fix,
// reference_liveness_break_needs_failing_baseline). The H2 positive arm at the
// bottom of this file is the failing baseline for world (b). Read the two
// tests as ONE gate.

// writeH3Reply stays 4-arg (h3dispatch.go). This compile-time pin fails the
// build if a later change widens it with a trailer block — the shape phase
// 84.1 deliberately did NOT give it (the trailing block is H2-only; H1/H3
// ignore it).
var _ func(http.ResponseWriter, int, filter_http.OrderedHeaders, []byte) error = writeH3Reply

// runH3TrailerDispatch drives one H3 request through runH3 against a route
// whose action returns an ActionResponse with the given trailing block, and
// returns the recorder holding everything the ResponseWriter was told.
// trailerCarryingAction is defined in codec_test.go (same package) — its
// H1-flavored asRouterAction is the closure runH3 installs, exactly as
// dispatchRequest does on H1.
func runH3TrailerDispatch(t *testing.T, trailers []hpack.HeaderField) *httptest.ResponseRecorder {
	t.Helper()
	tt := &routeTable{routes: []routeEntry{
		{match: matchPath("/grpc"), action: &trailerCarryingAction{trailers: trailers}},
	}}
	f := mkFilterForTable(t, tt)

	req := httptest.NewRequest(http.MethodPost, "https://example.test/grpc", nil)
	req.TLS = &stdtls.ConnectionState{Version: stdtls.VersionTLS13, HandshakeComplete: true, NegotiatedProtocol: "h3"}
	rec := httptest.NewRecorder()

	status, err := f.runH3(context.Background(), rec, req)
	if err != nil {
		t.Fatalf("runH3: %v", err)
	}
	if status != 200 {
		t.Fatalf("runH3 status = %d, want 200", status)
	}
	return rec
}

// headerDump renders an http.Header as a deterministic, diffable, WHOLE-map
// projection: keys sorted, every value listed.
//
// ⚠️ It walks the map directly rather than calling http.Header.Write, which is
// FAIL-UNSAFE for this assertion: writeSubset silently `continue`s over any key
// that is not a valid header field token, and a trailer-projection key such as
// "Trailer:grpc-status" contains a colon and is therefore invalid. Break B11-a
// (below, in the report) confirmed this empirically — with Header.Write, the
// whole-map comparison stayed GREEN while the response had in fact gained two
// Trailer-prefixed keys. Never dump a header map through a writer that filters.
func headerDump(t *testing.T, h http.Header) string {
	t.Helper()
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	for _, k := range keys {
		fmt.Fprintf(&buf, "%s: %q\n", k, h[k])
	}
	return buf.String()
}

// TestRunH3_TrailersProduceIdenticalResponse is the phase-84.1 Task 11
// behavioral non-change regression. An ActionResponse carrying a non-empty
// trailing block must project onto writeH3Reply's http.ResponseWriter
// identically to the same response with Trailers nil.
//
// Properties, one t.Errorf each (a t.Fatalf would make the later ones dead
// code):
//  1. the WHOLE header map is unchanged (not a per-key subset);
//  2. no "Trailer" key and no "Trailer:"-prefixed key is gained by EITHER arm
//     — the stacked control, which catches a break that declares trailers on
//     both arms (property 1 alone stays green on that);
//  3. the declared-trailer set surfaced by http.Response.Trailer is empty;
//  4. the body bytes and status code are unchanged.
func TestRunH3_TrailersProduceIdenticalResponse(t *testing.T) {
	trailerBlock := []hpack.HeaderField{
		{Name: "grpc-status", Value: "0"},
		{Name: "grpc-message", Value: "ok"},
	}

	with := runH3TrailerDispatch(t, trailerBlock)
	without := runH3TrailerDispatch(t, nil)

	if gotWith, gotWithout := headerDump(t, with.Header()), headerDump(t, without.Header()); gotWith != gotWithout {
		t.Errorf("H3 response headers differ when ActionResponse.Trailers is populated.\n with trailers:\n%s\n  nil trailers:\n%s", gotWith, gotWithout)
	}

	for _, arm := range []struct {
		name string
		rec  *httptest.ResponseRecorder
	}{
		{"trailers-populated", with},
		{"trailers-nil", without},
	} {
		for k := range arm.rec.Header() {
			if strings.EqualFold(k, "Trailer") {
				t.Errorf("%s arm: H3 response gained a %q header (%q); H3 must never declare trailers", arm.name, k, arm.rec.Header().Get(k))
			}
			if strings.HasPrefix(strings.ToLower(k), "trailer:") {
				t.Errorf("%s arm: H3 response gained a Trailer-prefixed key %q; H3 must never emit a trailing block", arm.name, k)
			}
		}
		if tr := arm.rec.Result().Trailer; len(tr) != 0 {
			t.Errorf("%s arm: http.Response.Trailer = %v; want empty", arm.name, tr)
		}
	}

	if !bytes.Equal(with.Body.Bytes(), without.Body.Bytes()) {
		t.Errorf("H3 body bytes differ when Trailers is populated: with = %q, without = %q", with.Body.Bytes(), without.Body.Bytes())
	}
	if with.Code != without.Code {
		t.Errorf("H3 status code differs when Trailers is populated: with = %d, without = %d", with.Code, without.Code)
	}
}

// TestWriteH2Reply_TrailingBlockLiveness is the POSITIVE ARM that makes the H3
// non-change test above two-sided. It is deliberately in THIS FILE.
//
// It drives writeH2Reply — the ONE seam that emits a trailing HEADERS block —
// with a populated trailer slice and asserts the block reaches the wire with
// END_STREAM on it. If the trailing emit is deleted or the carrier stops being
// consumed, THIS test reddens while the H3 non-change test above stays green:
// exactly the discrimination a one-sided gate lacks.
//
// Compact by design: one cell. The full
// {trailers, no trailers} x {body, no body} matrix is Task 7's
// TestWriteH2Reply_FrameSequence in h2dispatch_test.go; this is a liveness
// baseline, not a second copy of it.
func TestWriteH2Reply_TrailingBlockLiveness(t *testing.T) {
	trailerBlock := []hpack.HeaderField{
		{Name: "grpc-status", Value: "0"},
		{Name: "grpc-message", Value: "ok"},
	}
	hdrs := filter_http.OrderedHeaders{{Name: "content-type", Value: "application/grpc"}}

	w := &captureH2Writer{}
	if err := writeH2Reply(w, 200, hdrs, []byte("hello"), trailerBlock); err != nil {
		t.Fatalf("writeH2Reply: %v", err)
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
