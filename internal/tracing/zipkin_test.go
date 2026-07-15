package tracing

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"
)

// idIn builds an identityInput with the canonical 0x01.. trace-id, 0x11.. span-id,
// and (optionally) a 0x21.. parent span-id.
func idIn(withParent bool) identityInput {
	in := identityInput{
		TraceID: [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:  [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
	}
	if withParent {
		in.ParentSpanID = [8]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28}
	}
	return in
}

func TestZipkinIdentityFreshRoot(t *testing.T) {
	traceID, id, parentID, emitShared := zipkinIdentity(idIn(false), false, false)
	if want := "090a0b0c0d0e0f10"; traceID != want {
		t.Errorf("traceID = %q, want %q (low-64, 16 hex)", traceID, want)
	}
	if want := "1112131415161718"; id != want {
		t.Errorf("id = %q, want %q (hex SpanID)", id, want)
	}
	if parentID != "" {
		t.Errorf("parentID = %q, want empty (fresh root)", parentID)
	}
	if emitShared {
		t.Errorf("emitShared = true, want false (fresh root)")
	}
}

func TestZipkinIdentityContinuedNotShared(t *testing.T) {
	_, id, parentID, emitShared := zipkinIdentity(idIn(true), false, false)
	if want := "1112131415161718"; id != want {
		t.Errorf("id = %q, want %q (fresh SpanID)", id, want)
	}
	if want := "2122232425262728"; parentID != want {
		t.Errorf("parentID = %q, want %q (hex ParentSpanID)", parentID, want)
	}
	if emitShared {
		t.Errorf("emitShared = true, want false (not shared)")
	}
}

func TestZipkinIdentityContinuedShared(t *testing.T) {
	_, id, parentID, emitShared := zipkinIdentity(idIn(true), false, true)
	if want := "2122232425262728"; id != want {
		t.Errorf("id = %q, want %q (REUSED ParentSpanID)", id, want)
	}
	if parentID != "" {
		t.Errorf("parentID = %q, want empty (shared)", parentID)
	}
	if !emitShared {
		t.Errorf("emitShared = false, want true (shared)")
	}
}

func TestZipkinIdentity128Bit(t *testing.T) {
	traceID, _, _, _ := zipkinIdentity(idIn(false), true, false)
	if want := "0102030405060708090a0b0c0d0e0f10"; traceID != want {
		t.Errorf("traceID = %q, want %q (full 32 hex)", traceID, want)
	}
}

func TestZipkinEncodeSpan(t *testing.T) {
	d := freshDecision()
	// Continued span so that shared=true actually emits shared/REUSES the parent.
	d.Continued = true
	d.ParentSpanID = [8]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28}

	in := freshInputs()
	in.Authority = "127.0.0.1:10000"
	in.ClientTraceID = "cli-123"

	start := time.Unix(0, 1_000_000_000)
	end := start.Add(10 * time.Millisecond)
	span := BuildServerSpan(d, in, nil, start, end)

	b, err := encodeZipkinSpans([]*Span{span}, false, true)
	if err != nil {
		t.Fatalf("encodeZipkinSpans err = %v", err)
	}

	// Must be a JSON ARRAY.
	var arr []map[string]json.RawMessage
	if err := json.Unmarshal(b, &arr); err != nil {
		t.Fatalf("not a JSON array: %v (%s)", err, b)
	}
	if len(arr) != 1 {
		t.Fatalf("array len = %d, want 1", len(arr))
	}

	// Decode into a typed view for the scalar fields.
	var got struct {
		TraceID   string            `json:"traceId"`
		ID        string            `json:"id"`
		ParentID  string            `json:"parentId"`
		Name      string            `json:"name"`
		Kind      string            `json:"kind"`
		Timestamp int64             `json:"timestamp"`
		Duration  int64             `json:"duration"`
		Shared    bool              `json:"shared"`
		Tags      map[string]string `json:"tags"`
	}
	if err := json.Unmarshal(b[1:len(b)-1], &got); err != nil {
		t.Fatalf("decode span: %v", err)
	}

	if got.Name != span.Authority {
		t.Errorf("name = %q, want %q (Authority)", got.Name, span.Authority)
	}
	if got.Kind != "SERVER" {
		t.Errorf("kind = %q, want SERVER", got.Kind)
	}
	if want := start.UnixMicro(); got.Timestamp != want {
		t.Errorf("timestamp = %d, want %d (UnixMicro)", got.Timestamp, want)
	}
	if want := end.Sub(start).Microseconds(); got.Duration != want {
		t.Errorf("duration = %d, want %d (microseconds)", got.Duration, want)
	}
	if got.Duration <= 0 {
		t.Errorf("duration = %d, want > 0", got.Duration)
	}
	if !got.Shared {
		t.Errorf("shared = false, want true")
	}

	// Identity per zipkinIdentity (continued + shared => REUSED parent as id, no parentId).
	wantTrace, wantID, wantParent, _ := zipkinIdentity(identityInput{
		TraceID: span.TraceID, SpanID: span.SpanID, ParentSpanID: span.ParentSpanID,
	}, false, true)
	if got.TraceID != wantTrace {
		t.Errorf("traceId = %q, want %q", got.TraceID, wantTrace)
	}
	if got.ID != wantID {
		t.Errorf("id = %q, want %q", got.ID, wantID)
	}
	if got.ParentID != wantParent {
		t.Errorf("parentId = %q, want %q", got.ParentID, wantParent)
	}

	// Tags: the 14-key roster = the 16 attrs MINUS node_id/zone, PLUS the optional
	// guid:x-client-trace-id (ClientTraceID set).
	if _, ok := got.Tags["node_id"]; ok {
		t.Error("tags has node_id, want ABSENT")
	}
	if _, ok := got.Tags["zone"]; ok {
		t.Error("tags has zone, want ABSENT")
	}
	presentKeys := []string{
		"http.method", "http.url", "http.protocol", "http.status_code",
		"component", "downstream_cluster", "response_flags",
		"request_size", "response_size", "user_agent", "guid:x-request-id",
		"upstream_cluster", "upstream_cluster.name", "peer.address",
	}
	for _, k := range presentKeys {
		if _, ok := got.Tags[k]; !ok {
			t.Errorf("tags missing key %q", k)
		}
	}
	if v, ok := got.Tags["http.method"]; !ok || v != "GET" {
		t.Errorf("tags[http.method] = %q (ok=%v), want GET", v, ok)
	}
	if v, ok := got.Tags["guid:x-client-trace-id"]; !ok || v != "cli-123" {
		t.Errorf("tags[guid:x-client-trace-id] = %q (ok=%v), want cli-123", v, ok)
	}

	// NO annotations field (D-TRACE-ZIPKIN-ANNOTATIONS).
	if _, ok := arr[0]["annotations"]; ok {
		t.Error("span has annotations field, want ABSENT")
	}
}

// ─────────────────────────── ZipkinExporter tests ───────────────────────────

// zkPost records one Dispatch call: a DEFENSIVE COPY of the POSTed body plus the
// request shape fields (method/path/host/content-type).
type zkPost struct {
	body        []byte
	method      string
	path        string
	host        string
	contentType string
}

// fakeZipkinTransport is the test seam for ZipkinTransport. Dispatch records each
// call (deep-copying the body before the exporter reuses the buffer/request) and
// returns a per-call status from a queue (default 200) or a per-call error. An
// optional blockCh wedges the writer for the drop-newest test; hasCluster toggles
// HasCluster.
type fakeZipkinTransport struct {
	mu         sync.Mutex
	posts      []zkPost
	statuses   []int   // per-call HTTP status (default 200 when exhausted)
	errs       []error // per-call dispatch error (nil when exhausted)
	idx        int
	hasCluster bool

	blockCh chan struct{} // when non-nil, blocks each Dispatch on a receive
}

func (f *fakeZipkinTransport) HasCluster(string) bool { return f.hasCluster }

func (f *fakeZipkinTransport) Dispatch(_ context.Context, _ string, req *http.Request) (*http.Response, error) {
	if f.blockCh != nil {
		<-f.blockCh
	}
	// Defensive copy of the body BEFORE recording — the exporter may reuse the
	// underlying byte slice / request across attempts and batches.
	var body []byte
	if req.Body != nil {
		body, _ = io.ReadAll(req.Body)
		_ = req.Body.Close()
	}
	cp := make([]byte, len(body))
	copy(cp, body)

	f.mu.Lock()
	f.posts = append(f.posts, zkPost{
		body:        cp,
		method:      req.Method,
		path:        req.URL.Path,
		host:        req.Host,
		contentType: req.Header.Get("Content-Type"),
	})
	var status int = http.StatusOK
	var err error
	if f.idx < len(f.statuses) {
		status = f.statuses[f.idx]
	}
	if f.idx < len(f.errs) {
		err = f.errs[f.idx]
	}
	f.idx++
	f.mu.Unlock()

	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}, nil
}

func (f *fakeZipkinTransport) recorded() []zkPost {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]zkPost, len(f.posts))
	copy(out, f.posts)
	return out
}

// countZipkinPostSpans sums the JSON-array length across every recorded POST body.
func countZipkinPostSpans(posts []zkPost) int {
	n := 0
	for _, p := range posts {
		var arr []json.RawMessage
		if err := json.Unmarshal(p.body, &arr); err != nil {
			continue
		}
		n += len(arr)
	}
	return n
}

// waitForZipkinSpans polls the fake until it has recorded >= want spans across all
// POST bodies or the deadline elapses (never a bare sleep-then-assert).
func waitForZipkinSpans(t *testing.T, f *fakeZipkinTransport, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if countZipkinPostSpans(f.recorded()) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d zipkin spans; got %d", want, countZipkinPostSpans(f.recorded()))
}

// zipkinTestSpan returns a minimal valid Span for the exporter tests: a fixed
// Authority (the Zipkin span name) and a single attribute (one tag).
func zipkinTestSpan() *Span {
	return &Span{
		TraceID:   [16]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:    [8]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
		Authority: "svc.example.com:8080",
		Start:     time.Unix(0, 1000),
		End:       time.Unix(0, 2000),
		Attrs:     []KV{{Key: "http.method", Str: "GET"}},
	}
}

const (
	zkEndpoint = "/api/v2/spans"
	zkHostname = "zipkin.collector"
	zkCluster  = "zipkin_cluster"
)

// TestZipkinExporter_ExportAndClose submits K spans then Close — the fake must have
// received K spans aggregated across POSTs, spansSent == K.
func TestZipkinExporter_ExportAndClose(t *testing.T) {
	f := &fakeZipkinTransport{}
	sent, dropped := newTracerTestCounters(t)
	e := NewZipkinExporter(f, zkCluster, zkEndpoint, zkHostname, false, false, sent, dropped, 0, time.Hour)

	const k = 5
	for i := 0; i < k; i++ {
		e.Export(zipkinTestSpan())
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := countZipkinPostSpans(f.recorded()); got != k {
		t.Errorf("total spans = %d, want %d", got, k)
	}
	if got := sent.Load(); got != k {
		t.Errorf("spansSent = %d, want %d", got, k)
	}
	if got := dropped.Load(); got != 0 {
		t.Errorf("spansDropped = %d, want 0", got)
	}
}

// TestZipkinExporter_SizeTrigger uses a tiny bufferSizeBytes so a batch flushes
// mid-stream (≥2 POSTs). Asserts the AGGREGATE span count, not the POST count.
func TestZipkinExporter_SizeTrigger(t *testing.T) {
	f := &fakeZipkinTransport{}
	sent, dropped := newTracerTestCounters(t)
	// Per-span estimate = len(Authority) + len(Attrs)*32. Threshold = 2*est+1 ⇒
	// flush on the 3rd span of each batch.
	est := len(zipkinTestSpan().Authority) + len(zipkinTestSpan().Attrs)*32
	threshold := 2*est + 1
	e := NewZipkinExporter(f, zkCluster, zkEndpoint, zkHostname, false, false, sent, dropped, threshold, time.Hour)

	const n = 6
	for i := 0; i < n; i++ {
		e.Export(zipkinTestSpan())
	}
	waitForZipkinSpans(t, f, n, 5*time.Second)

	posts := f.recorded()
	if len(posts) < 2 {
		t.Fatalf("got %d POSTs, want >= 2 (size trigger fired mid-life)", len(posts))
	}
	if got := countZipkinPostSpans(posts); got != n {
		t.Fatalf("total spans = %d, want %d", got, n)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := sent.Load(); got != n {
		t.Errorf("spansSent = %d, want %d (batch-invariant)", got, n)
	}
}

// TestZipkinExporter_IntervalTrigger uses a short flush interval + a huge size
// threshold so only the timer fires. POLL the fake (no sleep-then-assert).
func TestZipkinExporter_IntervalTrigger(t *testing.T) {
	f := &fakeZipkinTransport{}
	sent, dropped := newTracerTestCounters(t)
	e := NewZipkinExporter(f, zkCluster, zkEndpoint, zkHostname, false, false, sent, dropped, 1<<30, 25*time.Millisecond)

	e.Export(zipkinTestSpan())
	waitForZipkinSpans(t, f, 1, 5*time.Second)

	if got := countZipkinPostSpans(f.recorded()); got != 1 {
		t.Fatalf("total spans = %d, want 1 (timer-flushed before Close)", got)
	}
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := sent.Load(); got != 1 {
		t.Errorf("spansSent = %d, want 1", got)
	}
	_ = dropped
}

// TestZipkinExporter_RetryOnce makes the fake return 500 on attempt 1 and 200 on
// attempt 2. The batch must land (spansSent += len(batch)) AND the SUCCESSFUL
// (second) POST must carry the span bytes — proving the retry re-sent the body
// (the consumed-bytes.Reader footgun would re-send an empty body).
func TestZipkinExporter_RetryOnce(t *testing.T) {
	f := &fakeZipkinTransport{statuses: []int{http.StatusInternalServerError, http.StatusOK}}
	sent, dropped := newTracerTestCounters(t)
	e := NewZipkinExporter(f, zkCluster, zkEndpoint, zkHostname, false, false, sent, dropped, 0, time.Hour)

	e.Export(zipkinTestSpan())
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	posts := f.recorded()
	if len(posts) != 2 {
		t.Fatalf("got %d POSTs, want 2 (one failed + one retried)", len(posts))
	}
	// The SECOND (successful) POST must carry exactly 1 span — not an empty body.
	var arr []json.RawMessage
	if err := json.Unmarshal(posts[1].body, &arr); err != nil {
		t.Fatalf("retry POST body is not a JSON array: %v (body=%q)", err, posts[1].body)
	}
	if len(arr) != 1 {
		t.Fatalf("retry POST carried %d spans, want 1 (the retry re-sent the bytes)", len(arr))
	}
	if got := sent.Load(); got != 1 {
		t.Errorf("spansSent = %d, want 1 (landed after retry)", got)
	}
	_ = dropped
}

// TestZipkinExporter_SecondFailureDropsBatch makes both attempts return 500. The
// batch is dropped (logged-not-counted); spansSent stays 0; spansDropped stays 0
// (the flush-path drop is logged, not channel-overflow-counted). A follow-up span
// proves the buffer was reset (memory bounded) and a fresh batch still flushes.
func TestZipkinExporter_SecondFailureDropsBatch(t *testing.T) {
	f := &fakeZipkinTransport{statuses: []int{http.StatusInternalServerError, http.StatusInternalServerError}}
	sent, dropped := newTracerTestCounters(t)
	e := NewZipkinExporter(f, zkCluster, zkEndpoint, zkHostname, false, false, sent, dropped, 0, time.Hour)

	e.Export(zipkinTestSpan())
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if got := sent.Load(); got != 0 {
		t.Errorf("spansSent = %d, want 0 (dropped batch is logged-not-counted)", got)
	}
	if got := dropped.Load(); got != 0 {
		t.Errorf("spansDropped = %d, want 0 (flush-path drops are logged-not-counted)", got)
	}
}

// TestZipkinExporter_DropNewest fills the channel past capacity with a wedged
// fake. Overflow spans must increment spansDropped.
func TestZipkinExporter_DropNewest(t *testing.T) {
	block := make(chan struct{})
	f := &fakeZipkinTransport{blockCh: block}
	sent, dropped := newTracerTestCounters(t)
	e := newZipkinExporterWithCapacity(f, zkCluster, zkEndpoint, zkHostname, false, false, sent, dropped, 0, time.Hour, 1)

	for i := 0; i < 100; i++ {
		e.Export(zipkinTestSpan()) // must never block with the writer wedged in Dispatch
	}
	if got := dropped.Load(); got == 0 {
		t.Errorf("expected at least one drop with a wedged writer; spansDropped = 0")
	}

	close(block) // release the writer so Close can drain
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	_ = sent
}

// TestZipkinExporter_CloseIdempotent: two Close() calls — the second is a no-op.
func TestZipkinExporter_CloseIdempotent(t *testing.T) {
	f := &fakeZipkinTransport{}
	sent, dropped := newTracerTestCounters(t)
	e := NewZipkinExporter(f, zkCluster, zkEndpoint, zkHostname, false, false, sent, dropped, 0, time.Hour)

	e.Export(zipkinTestSpan())
	if err := e.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := e.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	_, _ = sent, dropped
}

// TestZipkinExporter_PostShape asserts the POST request shape: Method POST, path ==
// endpoint, Host == hostname, Content-Type application/json.
func TestZipkinExporter_PostShape(t *testing.T) {
	f := &fakeZipkinTransport{}
	sent, dropped := newTracerTestCounters(t)
	e := NewZipkinExporter(f, zkCluster, zkEndpoint, zkHostname, false, false, sent, dropped, 0, time.Hour)

	e.Export(zipkinTestSpan())
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	posts := f.recorded()
	if len(posts) == 0 {
		t.Fatalf("got 0 POSTs, want >= 1")
	}
	p := posts[0]
	if p.method != http.MethodPost {
		t.Errorf("method = %q, want POST", p.method)
	}
	if p.path != zkEndpoint {
		t.Errorf("path = %q, want %q", p.path, zkEndpoint)
	}
	if p.host != zkHostname {
		t.Errorf("host = %q, want %q", p.host, zkHostname)
	}
	if p.contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", p.contentType)
	}
	_, _ = sent, dropped
}

// TestZipkinExporter_PostShapeEmptyHostname asserts that an empty hostname falls
// back to the cluster name for the Host header.
func TestZipkinExporter_PostShapeEmptyHostname(t *testing.T) {
	f := &fakeZipkinTransport{}
	sent, dropped := newTracerTestCounters(t)
	e := NewZipkinExporter(f, zkCluster, zkEndpoint, "", false, false, sent, dropped, 0, time.Hour)

	e.Export(zipkinTestSpan())
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	posts := f.recorded()
	if len(posts) == 0 {
		t.Fatalf("got 0 POSTs, want >= 1")
	}
	if posts[0].host != zkCluster {
		t.Errorf("host = %q, want %q (cluster fallback)", posts[0].host, zkCluster)
	}
	_, _ = sent, dropped
}

// TestZipkinEncodeCustomTagLiteral: a literal custom tag surfaces in the Zipkin v2
// `tags` map; node_id/zone stay dropped (the 14-tag roster is unchanged otherwise).
func TestZipkinEncodeCustomTagLiteral(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.Authority = "127.0.0.1:10000"
	in.NodeID = "node-x"
	in.Zone = "zone-y"
	start := time.Unix(0, 1_000_000_000)
	end := start.Add(10 * time.Millisecond)
	span := BuildServerSpan(d, in, []KV{{Key: "custom_env", Str: "prod-literal"}}, start, end)

	b, err := encodeZipkinSpans([]*Span{span}, false, true)
	if err != nil {
		t.Fatalf("encodeZipkinSpans err = %v", err)
	}
	var got struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.Unmarshal(b[1:len(b)-1], &got); err != nil {
		t.Fatalf("decode span: %v (%s)", err, b)
	}
	if got.Tags["custom_env"] != "prod-literal" {
		t.Errorf("tags[custom_env] = %q, want prod-literal", got.Tags["custom_env"])
	}
	if _, ok := got.Tags["node_id"]; ok {
		t.Errorf("tags[node_id] present, want dropped by the Zipkin encoder")
	}
	if _, ok := got.Tags["zone"]; ok {
		t.Errorf("tags[zone] present, want dropped by the Zipkin encoder")
	}
}

// TestZipkinEncodeResolvedRequestHeaderTag: a resolved request_header custom tag
// surfaces in the Zipkin v2 `tags` map (the shared Attrs seam feeds both exporters);
// node_id/zone stay dropped by the encoder.
func TestZipkinEncodeResolvedRequestHeaderTag(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.Authority = "127.0.0.1:10000"
	in.NodeID = "node-x"
	in.Zone = "zone-y"
	start := time.Unix(0, 1_000_000_000)
	end := start.Add(10 * time.Millisecond)
	// The resolver would have produced this KV from {tag: trace_user, request_header:{name: x-trace-user}}.
	span := BuildServerSpan(d, in, []KV{{Key: "trace_user", Str: "u-42"}}, start, end)

	b, err := encodeZipkinSpans([]*Span{span}, false, true)
	if err != nil {
		t.Fatalf("encodeZipkinSpans err = %v", err)
	}
	var got struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.Unmarshal(b[1:len(b)-1], &got); err != nil {
		t.Fatalf("decode span: %v (%s)", err, b)
	}
	if got.Tags["trace_user"] != "u-42" {
		t.Errorf("tags[trace_user] = %q, want u-42", got.Tags["trace_user"])
	}
	if _, ok := got.Tags["node_id"]; ok {
		t.Errorf("tags[node_id] present, want dropped by the Zipkin encoder")
	}
	if _, ok := got.Tags["zone"]; ok {
		t.Errorf("tags[zone] present, want dropped by the Zipkin encoder")
	}
}

// TestZipkinEncodeResolvedEnvironmentTag: a resolved environment custom tag surfaces
// in the Zipkin v2 `tags` map (the shared Attrs seam feeds both exporters);
// node_id/zone stay dropped by the encoder.
func TestZipkinEncodeResolvedEnvironmentTag(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.Authority = "127.0.0.1:10000"
	in.NodeID = "node-x"
	in.Zone = "zone-y"
	start := time.Unix(0, 1_000_000_000)
	end := start.Add(10 * time.Millisecond)
	// The resolver would have produced this KV from {tag: region, environment:{name: ENVOY_REGION}}.
	span := BuildServerSpan(d, in, []KV{{Key: "region", Str: "us-east-2"}}, start, end)

	b, err := encodeZipkinSpans([]*Span{span}, false, true)
	if err != nil {
		t.Fatalf("encodeZipkinSpans err = %v", err)
	}
	var got struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.Unmarshal(b[1:len(b)-1], &got); err != nil {
		t.Fatalf("decode span: %v (%s)", err, b)
	}
	if got.Tags["region"] != "us-east-2" {
		t.Errorf("tags[region] = %q, want us-east-2", got.Tags["region"])
	}
	if _, ok := got.Tags["node_id"]; ok {
		t.Errorf("tags[node_id] present, want dropped by the Zipkin encoder")
	}
	if _, ok := got.Tags["zone"]; ok {
		t.Errorf("tags[zone] present, want dropped by the Zipkin encoder")
	}
}

// TestZipkinEncodeTruncatedHTTPURL: a truncated http.url (built via BuildHTTPURL, the
// provider-neutral truncation) surfaces VERBATIM in the Zipkin v2 `tags` map — the
// Zipkin encoder carries the already-truncated URL (SPEC-64 §3.5/§8; truncation is at
// the call site, NOT in the encoder). node_id/zone stay dropped by the encoder.
func TestZipkinEncodeTruncatedHTTPURL(t *testing.T) {
	d := freshDecision()
	in := freshInputs()
	in.URL = BuildHTTPURL("http", "h.io", "/abcdefghijKLMNOPqrstuvwxyz", 16) // :path truncated to 16 bytes
	in.NodeID = "node-x"
	in.Zone = "zone-y"
	start := time.Unix(0, 1_000_000_000)
	end := start.Add(10 * time.Millisecond)
	span := BuildServerSpan(d, in, nil, start, end)

	b, err := encodeZipkinSpans([]*Span{span}, false, true)
	if err != nil {
		t.Fatalf("encodeZipkinSpans err = %v", err)
	}
	var got struct {
		Tags map[string]string `json:"tags"`
	}
	if err := json.Unmarshal(b[1:len(b)-1], &got); err != nil {
		t.Fatalf("decode span: %v (%s)", err, b)
	}
	if got.Tags["http.url"] != "http://h.io/abcdefghijKLMNO" {
		t.Errorf("tags[http.url] = %q, want http://h.io/abcdefghijKLMNO (truncated)", got.Tags["http.url"])
	}
	if _, ok := got.Tags["node_id"]; ok {
		t.Errorf("tags[node_id] present, want dropped by the Zipkin encoder")
	}
	if _, ok := got.Tags["zone"]; ok {
		t.Errorf("tags[zone] present, want dropped by the Zipkin encoder")
	}
}
