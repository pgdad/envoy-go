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

	"golang.org/x/net/http2/hpack"

	"github.com/esalaine/envoy-go/internal/accesslog"
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
// Phase 06.2 Task 12 — H1 emit-deferral tests
// ---------------------------------------------------------------------------

// TestDirectResponseAction_EmitsAccessLog verifies that directResponseAction.do
// submits exactly one access-log record with the correct ResponseCode and
// BytesSent (== len(bodyText)) when a Filter with a capture sink is wired.
func TestDirectResponseAction_EmitsAccessLog(t *testing.T) {
	cs := &emitCaptureSink{}
	f := &Filter{accessLog: []accesslog.Sink{cs}}
	a := &directResponseAction{status: 200, bodyText: "OK\n", filter: f}

	req, _ := http.NewRequest("GET", "/health", nil)
	req.Proto = "HTTP/1.1"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	if _, err := a.do(context.Background(), req, bw); err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = bw.Flush()

	if len(cs.recs) != 1 {
		t.Fatalf("captured %d records, want 1", len(cs.recs))
	}
	r := cs.recs[0]
	if r.ResponseCode != 200 {
		t.Errorf("ResponseCode = %d, want 200", r.ResponseCode)
	}
	if r.BytesSent != 3 {
		t.Errorf("BytesSent = %d, want 3 (len(\"OK\\n\"))", r.BytesSent)
	}
	if r.UpstreamHost != "" {
		t.Errorf("UpstreamHost = %q, want empty (direct_response)", r.UpstreamHost)
	}
}

// TestDirectResponseAction_NilFilter_DoesNotPanic verifies that
// directResponseAction.do is safe when filter is nil (no sinks wired).
func TestDirectResponseAction_NilFilter_DoesNotPanic(t *testing.T) {
	a := &directResponseAction{status: 404, bodyText: "not found\n", filter: nil}
	req, _ := http.NewRequest("GET", "/missing", nil)
	req.Proto = "HTTP/1.1"
	var buf bytes.Buffer
	bw := bufio.NewWriter(&buf)
	// Must not panic.
	if _, err := a.do(context.Background(), req, bw); err != nil {
		t.Fatalf("do: %v", err)
	}
}
