// Unit tests for the ADR-0133 decompress-and-compare helpers landed at Task 11.
// These tests exercise the body-assertion-mode dispatch + the gzip-decompression
// helper + the AE-absence-in-echoed-headers helper in isolation, without
// requiring the differential runner (no Docker / no YAMLs).
//
// The differential pass for fixture 0016 itself lands at Task 14 (counter
// assertions + first green); Task 11 leaves the driver framework + helpers in
// place; YAMLs land at Task 12.
package inputs

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strings"
	"testing"
)

// --- decompressGzip ---

func TestDecompressGzip_RoundTrip(t *testing.T) {
	t.Parallel()
	want := bytes.Repeat([]byte("A"), 1024)
	gz := mustGzip(t, want)
	got, err := decompressGzip(gz)
	if err != nil {
		t.Fatalf("decompressGzip: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("decompressed bytes differ: got %d bytes; want %d bytes", len(got), len(want))
	}
}

func TestDecompressGzip_EmptyPayloadRoundTrip(t *testing.T) {
	t.Parallel()
	gz := mustGzip(t, nil)
	got, err := decompressGzip(gz)
	if err != nil {
		t.Fatalf("decompressGzip: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty plaintext; got %d bytes", len(got))
	}
}

func TestDecompressGzip_InvalidHeaderReturnsError(t *testing.T) {
	t.Parallel()
	_, err := decompressGzip([]byte("not gzip"))
	if err == nil {
		t.Fatalf("expected error on non-gzip input; got nil")
	}
}

// --- assertBodyEquivalent ---

func TestAssertBodyEquivalent_UncompressedByteExactMatch(t *testing.T) {
	t.Parallel()
	body := []byte("hello world")
	eg := scenarioResult{Header: http.Header{}, Body: body}
	en := scenarioResult{Header: http.Header{}, Body: body}
	if err := assertBodyEquivalent(&eg, &en, nil); err != nil {
		t.Fatalf("expected no error; got %v", err)
	}
}

func TestAssertBodyEquivalent_UncompressedByteMismatch(t *testing.T) {
	t.Parallel()
	eg := scenarioResult{Header: http.Header{}, Body: []byte("a")}
	en := scenarioResult{Header: http.Header{}, Body: []byte("b")}
	err := assertBodyEquivalent(&eg, &en, nil)
	if err == nil {
		t.Fatalf("expected error on mismatched uncompressed bodies; got nil")
	}
	if !strings.Contains(err.Error(), "uncompressed bodies differ") {
		t.Fatalf("error did not mention uncompressed mismatch: %v", err)
	}
}

func TestAssertBodyEquivalent_ContentEncodingMismatchFails(t *testing.T) {
	t.Parallel()
	plain := []byte("hello")
	gz := mustGzip(t, plain)
	eg := scenarioResult{Header: makeCE("gzip"), Body: gz}
	en := scenarioResult{Header: http.Header{}, Body: plain}
	err := assertBodyEquivalent(&eg, &en, nil)
	if err == nil {
		t.Fatalf("expected mismatch on CE-divergent sides; got nil")
	}
	if !strings.Contains(err.Error(), "Content-Encoding mismatch") {
		t.Fatalf("error did not mention CE mismatch: %v", err)
	}
}

func TestAssertBodyEquivalent_GzipDecompressedByteExact(t *testing.T) {
	t.Parallel()
	// Both sides gzip the SAME plaintext. The compressed bytes may differ
	// (Go gzip vs libz) — this test simulates that by re-gzipping the SAME
	// plaintext twice; the resulting compressed bytes from the same Go gzip
	// are identical, but the helper is supposed to decompress-and-compare.
	plain := bytes.Repeat([]byte("Z"), 1024)
	gzA := mustGzip(t, plain)
	gzB := mustGzipWithLevel(t, plain, gzip.BestSpeed)
	if bytes.Equal(gzA, gzB) {
		t.Skip("the two gzip outputs happened to be byte-equal; cannot exercise the divergent-compressed-bytes branch")
	}
	eg := scenarioResult{Header: makeCE("gzip"), Body: gzA}
	en := scenarioResult{Header: makeCE("gzip"), Body: gzB}
	if err := assertBodyEquivalent(&eg, &en, plain); err != nil {
		t.Fatalf("expected no error; got %v", err)
	}
}

func TestAssertBodyEquivalent_GzipDecompressedDiffers(t *testing.T) {
	t.Parallel()
	gzA := mustGzip(t, []byte("apple"))
	gzB := mustGzip(t, []byte("orange"))
	eg := scenarioResult{Header: makeCE("gzip"), Body: gzA}
	en := scenarioResult{Header: makeCE("gzip"), Body: gzB}
	err := assertBodyEquivalent(&eg, &en, nil)
	if err == nil {
		t.Fatalf("expected mismatch; got nil")
	}
	if !strings.Contains(err.Error(), "decompressed bodies differ") {
		t.Fatalf("error did not mention decompressed mismatch: %v", err)
	}
}

func TestAssertBodyEquivalent_GzipOriginalPayloadCheck(t *testing.T) {
	t.Parallel()
	plain := []byte("expected")
	other := []byte("actually decompresses to this")
	gz := mustGzip(t, other)
	eg := scenarioResult{Header: makeCE("gzip"), Body: gz}
	en := scenarioResult{Header: makeCE("gzip"), Body: gz}
	err := assertBodyEquivalent(&eg, &en, plain)
	if err == nil {
		t.Fatalf("expected mismatch vs original payload; got nil")
	}
	if !strings.Contains(err.Error(), "original input") {
		t.Fatalf("error did not mention original-input mismatch: %v", err)
	}
}

func TestAssertBodyEquivalent_UnsupportedContentEncoding(t *testing.T) {
	t.Parallel()
	eg := scenarioResult{Header: makeCE("br"), Body: []byte{0xff}}
	en := scenarioResult{Header: makeCE("br"), Body: []byte{0xff}}
	err := assertBodyEquivalent(&eg, &en, nil)
	if err == nil {
		t.Fatalf("expected unsupported-CE error; got nil")
	}
	if !strings.Contains(err.Error(), "unsupported Content-Encoding") {
		t.Fatalf("error did not mention unsupported CE: %v", err)
	}
}

// --- assertNoAcceptEncodingInEchoedBody ---

func TestAssertNoAcceptEncodingInEchoedBody_AbsentOK(t *testing.T) {
	t.Parallel()
	body := []byte(`{"method":"GET","path":"/per-route-rmae","headers":{"host":"example"}}`)
	if err := assertNoAcceptEncodingInEchoedBody(body); err != nil {
		t.Fatalf("expected nil error when Accept-Encoding is absent; got %v", err)
	}
}

func TestAssertNoAcceptEncodingInEchoedBody_PresentFails(t *testing.T) {
	t.Parallel()
	body := []byte(`{"method":"GET","path":"/per-route-rmae","headers":{"accept-encoding":"gzip","host":"example"}}`)
	err := assertNoAcceptEncodingInEchoedBody(body)
	if err == nil {
		t.Fatalf("expected error when Accept-Encoding is present; got nil")
	}
	if !strings.Contains(err.Error(), "NOT stripped") {
		t.Fatalf("error did not mention strip-failure: %v", err)
	}
}

func TestAssertNoAcceptEncodingInEchoedBody_InvalidJSONFails(t *testing.T) {
	t.Parallel()
	err := assertNoAcceptEncodingInEchoedBody([]byte("not-json"))
	if err == nil {
		t.Fatalf("expected JSON-parse error; got nil")
	}
	if !strings.Contains(err.Error(), "parse echoed body") {
		t.Fatalf("error did not mention parse failure: %v", err)
	}
}

// --- varyMatches ---

func TestVaryMatches_EmptyExpectedEmptyActualOK(t *testing.T) {
	t.Parallel()
	if !varyMatches("", "") {
		t.Fatalf("varyMatches: empty/empty should match")
	}
	if !varyMatches("  ", "") {
		t.Fatalf("varyMatches: whitespace-only/empty should match")
	}
}

func TestVaryMatches_EmptyExpectedActualPresentFails(t *testing.T) {
	t.Parallel()
	if varyMatches("Accept-Encoding", "") {
		t.Fatalf("varyMatches: empty-expected but token present should NOT match")
	}
}

func TestVaryMatches_TokenPresenceCaseInsensitive(t *testing.T) {
	t.Parallel()
	if !varyMatches("Accept-Encoding", "Accept-Encoding") {
		t.Fatalf("varyMatches: exact match should hold")
	}
	if !varyMatches("accept-encoding", "Accept-Encoding") {
		t.Fatalf("varyMatches: lowercase actual should match")
	}
	if !varyMatches("ACCEPT-ENCODING", "Accept-Encoding") {
		t.Fatalf("varyMatches: uppercase actual should match")
	}
}

func TestVaryMatches_MultiTokenList(t *testing.T) {
	t.Parallel()
	if !varyMatches("User-Agent, Accept-Encoding, Cookie", "Accept-Encoding") {
		t.Fatalf("varyMatches: multi-token list with target should match")
	}
	if varyMatches("User-Agent, Cookie", "Accept-Encoding") {
		t.Fatalf("varyMatches: list without target should NOT match")
	}
}

// --- test helpers ---

func mustGzip(t *testing.T, in []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(in); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func mustGzipWithLevel(t *testing.T, in []byte, level int) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := gzip.NewWriterLevel(&buf, level)
	if err != nil {
		t.Fatalf("gzip writer level=%d: %v", level, err)
	}
	if _, err := w.Write(in); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func makeCE(value string) http.Header {
	h := http.Header{}
	h.Set("Content-Encoding", value)
	return h
}
