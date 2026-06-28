package zipkincollector

import (
	"bytes"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

// postSpans POSTs the supplied JSON body to <addr>/api/v2/spans and returns the
// response status code, failing the test on a transport error.
func postSpans(t *testing.T, addr string, body []byte) int {
	t.Helper()
	resp, err := http.Post("http://"+addr+"/api/v2/spans", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /api/v2/spans: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// twoSpansJSON is a Zipkin v2 JSON array of two SERVER spans with distinct
// names/traceIds and a tags map, mirroring the encodeZipkinSpans wire shape.
const twoSpansJSON = `[
  {"traceId":"0123456789abcdef","id":"1111111111111111","name":"/a","kind":"SERVER","timestamp":1000,"duration":10,"tags":{"http.method":"GET","http.status_code":"200"}},
  {"traceId":"fedcba9876543210","id":"2222222222222222","parentId":"3333333333333333","name":"/b","kind":"SERVER","timestamp":2000,"duration":20,"tags":{"http.method":"POST"}}
]`

// moreSpansJSON is a second array of two spans to prove cross-POST accumulation.
const moreSpansJSON = `[
  {"traceId":"aaaaaaaaaaaaaaaa","id":"4444444444444444","name":"/c","kind":"SERVER","timestamp":3000,"duration":30,"tags":{}},
  {"traceId":"bbbbbbbbbbbbbbbb","id":"5555555555555555","name":"/d","kind":"SERVER","timestamp":4000,"duration":40,"tags":{}}
]`

// TestNew_StartsServerOnEphemeralPort verifies New binds an ephemeral 127.0.0.1
// port and Addr() returns the bound host:port immediately.
func TestNew_StartsServerOnEphemeralPort(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Stop()

	addr := c.Addr()
	if addr == "" {
		t.Fatal("Addr: empty after New")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	if host != "127.0.0.1" {
		t.Errorf("host: got %q, want %q", host, "127.0.0.1")
	}
	if port == "0" {
		t.Errorf("port: got %q, want non-zero ephemeral", port)
	}
}

// TestPost_AccumulatesAcrossPOSTs POSTs two spans (Count==2, decoded values
// asserted), then a second array of two (Count==4 accumulated).
func TestPost_AccumulatesAcrossPOSTs(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Stop()

	if code := postSpans(t, c.Addr(), []byte(twoSpansJSON)); code != http.StatusAccepted {
		t.Fatalf("first POST status: got %d, want %d", code, http.StatusAccepted)
	}
	if got := c.Count(); got != 2 {
		t.Fatalf("Count after first POST: got %d, want 2", got)
	}

	spans := c.Spans()
	if len(spans) != 2 {
		t.Fatalf("Spans after first POST: got %d, want 2", len(spans))
	}
	// Assert the REAL decoded values of span 0.
	if spans[0].Name != "/a" {
		t.Errorf("spans[0].Name: got %q, want %q", spans[0].Name, "/a")
	}
	if spans[0].TraceID != "0123456789abcdef" {
		t.Errorf("spans[0].TraceID: got %q, want %q", spans[0].TraceID, "0123456789abcdef")
	}
	if spans[0].Kind != "SERVER" {
		t.Errorf("spans[0].Kind: got %q, want %q", spans[0].Kind, "SERVER")
	}
	if got := spans[0].Tags["http.method"]; got != "GET" {
		t.Errorf("spans[0].Tags[http.method]: got %q, want %q", got, "GET")
	}
	if got := spans[0].Tags["http.status_code"]; got != "200" {
		t.Errorf("spans[0].Tags[http.status_code]: got %q, want %q", got, "200")
	}
	// Assert span 1's name/traceId/parentId.
	if spans[1].Name != "/b" {
		t.Errorf("spans[1].Name: got %q, want %q", spans[1].Name, "/b")
	}
	if spans[1].TraceID != "fedcba9876543210" {
		t.Errorf("spans[1].TraceID: got %q, want %q", spans[1].TraceID, "fedcba9876543210")
	}
	if spans[1].ParentID != "3333333333333333" {
		t.Errorf("spans[1].ParentID: got %q, want %q", spans[1].ParentID, "3333333333333333")
	}

	// Second POST accumulates onto the same slice.
	if code := postSpans(t, c.Addr(), []byte(moreSpansJSON)); code != http.StatusAccepted {
		t.Fatalf("second POST status: got %d, want %d", code, http.StatusAccepted)
	}
	if got := c.Count(); got != 4 {
		t.Fatalf("Count after second POST: got %d, want 4 (accumulation)", got)
	}
	spans = c.Spans()
	if len(spans) != 4 {
		t.Fatalf("Spans after second POST: got %d, want 4", len(spans))
	}
	if spans[2].Name != "/c" || spans[3].Name != "/d" {
		t.Errorf("accumulated span names: got %q,%q, want /c,/d", spans[2].Name, spans[3].Name)
	}
}

// TestSpans_ReturnsSnapshotCopy verifies Spans() returns a defensive copy:
// mutating the returned slice must not perturb the collector's accumulation.
func TestSpans_ReturnsSnapshotCopy(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Stop()

	if code := postSpans(t, c.Addr(), []byte(twoSpansJSON)); code != http.StatusAccepted {
		t.Fatalf("POST status: got %d, want %d", code, http.StatusAccepted)
	}
	snap := c.Spans()
	if len(snap) != 2 {
		t.Fatalf("Spans: got %d, want 2", len(snap))
	}
	snap[0].Name = "MUTATED" // mutate the caller copy
	if c.Spans()[0].Name == "MUTATED" {
		t.Error("Spans returned an aliased slice; caller mutation leaked into the collector")
	}
}

// TestReset_ClearsAccumulation verifies Reset() truncates the accumulated spans.
func TestReset_ClearsAccumulation(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Stop()

	if code := postSpans(t, c.Addr(), []byte(twoSpansJSON)); code != http.StatusAccepted {
		t.Fatalf("POST status: got %d, want %d", code, http.StatusAccepted)
	}
	if got := c.Count(); got != 2 {
		t.Fatalf("Count before Reset: got %d, want 2", got)
	}
	c.Reset()
	if got := c.Count(); got != 0 {
		t.Errorf("Count after Reset: got %d, want 0", got)
	}
	if got := c.Spans(); len(got) != 0 {
		t.Errorf("Spans after Reset: got %v, want empty", got)
	}
}

// TestBadBody_DoesNotPanic verifies a malformed body replies a 4xx and does not
// accumulate (graceful handling — the differential only POSTs valid arrays, but
// a malformed body must not panic).
func TestBadBody_DoesNotPanic(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Stop()

	code := postSpans(t, c.Addr(), []byte("not json"))
	if code < 400 || code >= 500 {
		t.Errorf("malformed POST status: got %d, want a 4xx", code)
	}
	if got := c.Count(); got != 0 {
		t.Errorf("Count after malformed POST: got %d, want 0", got)
	}
}

// TestNonPost_Rejected verifies a non-POST method is rejected (4xx) without
// accumulating or panicking.
func TestNonPost_Rejected(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Stop()

	resp, err := http.Get("http://" + c.Addr() + "/api/v2/spans")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode < 400 || resp.StatusCode >= 500 {
		t.Errorf("GET status: got %d, want a 4xx", resp.StatusCode)
	}
	if got := c.Count(); got != 0 {
		t.Errorf("Count after GET: got %d, want 0", got)
	}
}

// TestConcurrentPOSTs_NoRace verifies concurrent POSTs accumulating into the
// shared slice do not trip the race detector while a poller reads
// Count()/Spans() concurrently.
func TestConcurrentPOSTs_NoRace(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Stop()

	const concurrency = 16
	const single = `[{"traceId":"0000000000000001","id":"0000000000000002","name":"/x","kind":"SERVER","timestamp":1,"duration":1,"tags":{}}]`

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			postSpans(t, c.Addr(), []byte(single))
		}()
	}

	done := make(chan struct{})
	deadline := time.After(10 * time.Second)
	go func() {
		defer close(done)
		for {
			if c.Count() >= concurrency {
				return
			}
			select {
			case <-deadline:
				return
			default:
			}
			_ = c.Spans()
			time.Sleep(time.Millisecond)
		}
	}()

	wg.Wait()
	<-done
	if got := c.Count(); got != concurrency {
		t.Errorf("Count: got %d, want %d", got, concurrency)
	}
}

// TestStop_ShutsDown verifies Stop shuts the server down so a subsequent POST
// fails to connect.
func TestStop_ShutsDown(t *testing.T) {
	c, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	addr := c.Addr()
	c.Stop()

	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Post("http://"+addr+"/api/v2/spans", "application/json", bytes.NewReader([]byte(twoSpansJSON)))
	if err == nil {
		_ = resp.Body.Close()
		t.Error("POST after Stop: expected a connection error, got success")
	}
}
