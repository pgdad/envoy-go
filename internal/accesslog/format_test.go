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
		Method: "GET", Path: "/health", Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 3, Duration: 5 * time.Millisecond,
		Authority: "127.0.0.1:10000", UserAgent: "Go-http-client/1.1",
		UpstreamHost: "",  // direct_response → emit literal "-"
	}
	got := Default(rec)
	if !bytes.HasSuffix(got, []byte("\n")) { t.Errorf("not LF-terminated: %q", got) }
	if bytes.Count(got, []byte("\n")) != 1 { t.Errorf("embedded LF: %q", got) }
	s := string(got)
	for _, want := range []string{
		`[2026-04-29T12:34:56.789Z]`, `"GET /health HTTP/1.1"`,
		` 200 - - 3 5 - "-" "Go-http-client/1.1" "-" "127.0.0.1:10000" "-"`,
	} {
		if !strings.Contains(s, want) { t.Errorf("Default missing %q in %q", want, s) }
	}
}

func TestDefault_RoutedPath_UpstreamHostFormatted(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: "/api/v1/foo", Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 17, Duration: 12 * time.Millisecond,
		Authority: "127.0.0.1:10000", UserAgent: "Go-http-client/1.1",
		UpstreamHost: "10.0.0.1:8080",
	}
	s := string(Default(rec))
	if !strings.Contains(s, `"10.0.0.1:8080"`) { t.Errorf("upstream host missing: %q", s) }
}

func TestDefault_QuoteEscaping(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: `/x"y`, Protocol: "HTTP/1.1",
		ResponseCode: 200, BytesSent: 0, Duration: 0,
		Authority: `host"with-quote`, UserAgent: `agent"with-quote`,
	}
	s := string(Default(rec))
	if strings.Contains(s, `"agent"with-quote"`) { t.Errorf("unescaped quote in UA: %q", s) }
	if !strings.Contains(s, `"agent\"with-quote"`) { t.Errorf("UA quote not escaped: %q", s) }
	if !strings.Contains(s, `/x\"y`) { t.Errorf("path quote not escaped: %q", s) }
}

func TestDefault_NeverEmbedsLF(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: "/x\ny", Protocol: "HTTP/1.1",
		ResponseCode: 200, Authority: "h\nx", UserAgent: "ua\ny",
	}
	got := Default(rec)
	body := bytes.TrimSuffix(got, []byte{'\n'})
	if bytes.IndexByte(body, '\n') >= 0 { t.Errorf("embedded LF in body: %q", got) }
}

func TestDefault_EmptyFieldsEmitDash(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: "/", Protocol: "HTTP/1.1",
		ResponseCode: 200,
	}
	s := string(Default(rec))
	if !strings.Contains(s, `"-"`) { t.Errorf("empty fields not emitting `-`: %q", s) }
}

func TestDefault_StartTimeFormat_RFC3339Ms(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 12, 34, 56, 789012345, time.UTC),
		Method: "GET", Path: "/", Protocol: "HTTP/1.1", ResponseCode: 200,
	}
	s := string(Default(rec))
	if !strings.HasPrefix(s, `[2026-04-29T12:34:56.789Z]`) {
		t.Errorf("START_TIME format wrong; got prefix %q", s[:30])
	}
}

func TestDefault_DurationMillisecondsRoundedDown(t *testing.T) {
	rec := &Record{
		StartTime: time.Date(2026, 4, 29, 0, 0, 0, 0, time.UTC),
		Method: "GET", Path: "/", Protocol: "HTTP/1.1", ResponseCode: 200,
		Duration: 12_999_999 * time.Nanosecond,
	}
	s := string(Default(rec))
	if !strings.Contains(s, " 12 ") { t.Errorf("duration not rounded down to 12: %q", s) }
}
