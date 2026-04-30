package accesslog

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestDefault_HappyPath_HCMDirect(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 12, 34, 56, 789000000, time.UTC),
		Method:    "GET", Path: "/health", Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 3, Duration: 5 * time.Millisecond,
		Authority: "127.0.0.1:10000", UserAgent: "Go-http-client/1.1",
		UpstreamHost: "", // direct_response → emit literal "-"
	}
	got := Default(rec)
	if !bytes.HasSuffix(got, []byte("\n")) {
		t.Errorf("not LF-terminated: %q", got)
	}
	if bytes.Count(got, []byte("\n")) != 1 {
		t.Errorf("embedded LF: %q", got)
	}
	s := string(got)
	for _, want := range []string{
		`[2026-04-29T12:34:56.789Z]`, `"GET /health HTTP/1.1"`,
		` 200 - - 3 5 - "-" "Go-http-client/1.1" "-" "127.0.0.1:10000" "-"`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("Default missing %q in %q", want, s)
		}
	}
}

func TestDefault_RoutedPath_UpstreamHostFormatted(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method:    "GET", Path: "/api/v1/foo", Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 17, Duration: 12 * time.Millisecond,
		Authority: "127.0.0.1:10000", UserAgent: "Go-http-client/1.1",
		UpstreamHost: "10.0.0.1:8080",
	}
	s := string(Default(rec))
	if !strings.Contains(s, `"10.0.0.1:8080"`) {
		t.Errorf("upstream host missing: %q", s)
	}
}

func TestDefault_QuoteEscaping(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method:    "GET", Path: `/x"y`, Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 0, Duration: 0,
		Authority: `host"with-quote`, UserAgent: `agent"with-quote`,
	}
	s := string(Default(rec))
	if strings.Contains(s, `"agent"with-quote"`) {
		t.Errorf("unescaped quote in UA: %q", s)
	}
	if !strings.Contains(s, `"agent\"with-quote"`) {
		t.Errorf("UA quote not escaped: %q", s)
	}
	if !strings.Contains(s, `/x\"y`) {
		t.Errorf("path quote not escaped: %q", s)
	}
}

// Regression for the gate-(d) verifier finding (verify commit 503c8ee): when
// a quoted-operator value ends with a literal backslash, the closing `"`
// field-delimiter must be preceded by `\\` (escaped backslash), not by a
// single `\` (which round-trip readers and the FuzzAccessLogFormat parseability
// invariant would interpret as an escaped quote — silently swallowing the
// closing field delimiter). Matches reference Envoy
// AccessLogFormatUtils::escapeUtilityValue and RFC 4180 CSV-style escaping.
func TestDefault_BackslashInQuotedField(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method:    "0", Path: "0", Protocol: "0",
		ResponseCode: 200, BytesSent: 42, Duration: 5 * time.Millisecond,
		Authority: "0", UserAgent: "0", UpstreamHost: `\`,
	}
	s := string(Default(rec))
	// The UPSTREAM_HOST field is the last quoted operator. Its body is one
	// backslash. After escape() that backslash must be `\\` so the closing
	// `"` is preceded by `\\` (legit close), not by `\` (looks like `\"`).
	if !strings.HasSuffix(s, `"\\"`+"\n") {
		t.Errorf("UPSTREAM_HOST=`\\` should serialize to `\"\\\\\"`; got tail = %q", s[len(s)-8:])
	}
	if strings.HasSuffix(s, `"\"`+"\n") {
		t.Errorf("UPSTREAM_HOST tail looks like an escaped quote (closing `\"` swallowed): %q", s)
	}
	// Even count of un-escaped quotes (matches the FuzzAccessLogFormat invariant
	// — a `"` is escaped iff preceded by an ODD number of `\`).
	body := []byte(s)
	quoteCount := 0
	for i := 0; i < len(body); i++ {
		if body[i] != '"' {
			continue
		}
		bsCount := 0
		for j := i - 1; j >= 0 && body[j] == '\\'; j-- {
			bsCount++
		}
		if bsCount%2 == 0 {
			quoteCount++
		}
	}
	if quoteCount%2 != 0 {
		t.Errorf("odd un-escaped quote count (%d) in %q", quoteCount, s)
	}
}

// Backslash that does NOT terminate a field still gets escaped to `\\` per the
// RFC 4180 / reference-Envoy convention — verifies the escape catalog is
// consistent across positions in the value, not just at the trailing edge.
func TestDefault_BackslashInMiddleOfField(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method:    "GET", Path: "/", Protocol: "HTTP/1.1",
		ResponseCode: 200,
		UserAgent:    `a\b\c`,
	}
	s := string(Default(rec))
	if !strings.Contains(s, `"a\\b\\c"`) {
		t.Errorf("interior backslashes not doubled; got %q", s)
	}
	if strings.Contains(s, `"a\b\c"`) {
		t.Errorf("interior backslashes still single (un-escaped): %q", s)
	}
}

// Backslash + quote in same field: escape order must produce `\\\"` (backslash
// escapes first, then quote escapes), not `\\"` (which would be parsed as
// escaped-backslash + bare quote = field terminator).
func TestDefault_BackslashThenQuoteInField(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method:    "GET", Path: "/", Protocol: "HTTP/1.1",
		ResponseCode: 200,
		UserAgent:    `\"`,
	}
	s := string(Default(rec))
	if !strings.Contains(s, `"\\\""`) {
		t.Errorf("backslash-quote not escaped to `\\\\\\\"`; got %q", s)
	}
}

func TestDefault_NeverEmbedsLF(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method:    "GET", Path: "/x\ny", Protocol: "HTTP/1.1",
		ResponseCode: 200, Authority: "h\nx", UserAgent: "ua\ny",
	}
	got := Default(rec)
	body := bytes.TrimSuffix(got, []byte{'\n'})
	if bytes.IndexByte(body, '\n') >= 0 {
		t.Errorf("embedded LF in body: %q", got)
	}
}

func TestDefault_EmptyFieldsEmitDash(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method:    "GET", Path: "/", Protocol: "HTTP/1.1",
		ResponseCode: 200,
	}
	s := string(Default(rec))
	if !strings.Contains(s, `"-"`) {
		t.Errorf("empty fields not emitting `-`: %q", s)
	}
}

func TestDefault_StartTimeFormat_RFC3339Ms(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 12, 34, 56, 789012345, time.UTC),
		Method:    "GET", Path: "/", Protocol: "HTTP/1.1", ResponseCode: 200,
	}
	s := string(Default(rec))
	if !strings.HasPrefix(s, `[2026-04-29T12:34:56.789Z]`) {
		t.Errorf("START_TIME format wrong; got prefix %q", s[:30])
	}
}

func TestDefault_DurationMillisecondsRoundedDown(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method:    "GET", Path: "/", Protocol: "HTTP/1.1", ResponseCode: 200,
		Duration: 12_999_999 * time.Nanosecond,
	}
	s := string(Default(rec))
	if !strings.Contains(s, " 12 ") {
		t.Errorf("duration not rounded down to 12: %q", s)
	}
}
