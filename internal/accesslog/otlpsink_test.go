package accesslog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/proto"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	logspb "go.opentelemetry.io/proto/otlp/logs/v1"
)

// fakeOTLPClient is a fake otlpClient seam. Export records every request and can
// be made to error per a call-indexed queue; Close is counted. Mirrors the
// grpcsink fake's buf-reuse defensive-copy discipline.
type fakeOTLPClient struct {
	mu         sync.Mutex
	exported   []*collogspb.ExportLogsServiceRequest
	exportErrs []error
	exportIdx  int
	closeCount int

	// blockCh, when non-nil, blocks each Export on a receive until the test
	// closes it (used to wedge the writer goroutine for the drop test).
	blockCh chan struct{}
}

func (c *fakeOTLPClient) Export(_ context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
	if c.blockCh != nil {
		<-c.blockCh
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	if c.exportIdx < len(c.exportErrs) {
		err = c.exportErrs[c.exportIdx]
	}
	c.exportIdx++
	if err != nil {
		return nil, err
	}
	// Defensive deep copy (buf-reuse contract): the sink wraps its buf slice
	// directly into the request's LogRecords and reuses that backing array across
	// flushes (buf = buf[:0]), so a later batch's append would overwrite this
	// recorded request's records. The real unary Export serializes synchronously
	// and is unaffected; the fake retains the pointer, so it snapshots here.
	cp := proto.Clone(req).(*collogspb.ExportLogsServiceRequest)
	c.exported = append(c.exported, cp)
	return &collogspb.ExportLogsServiceResponse{}, nil
}

func (c *fakeOTLPClient) Close() error {
	c.mu.Lock()
	c.closeCount++
	c.mu.Unlock()
	return nil
}

func (c *fakeOTLPClient) requests() []*collogspb.ExportLogsServiceRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]*collogspb.ExportLogsServiceRequest, len(c.exported))
	copy(out, c.exported)
	return out
}

func (c *fakeOTLPClient) closes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCount
}

// otlpTestNode carries all three node-derived built-in label sources.
func otlpTestNode() *corev3.Node {
	return &corev3.Node{
		Id:       "node-1",
		Cluster:  "cluster-1",
		Locality: &corev3.Locality{Zone: "zone-1"},
	}
}

// otlpRecords sums LogRecords across all recorded requests.
func otlpRecords(reqs []*collogspb.ExportLogsServiceRequest) int {
	n := 0
	for _, r := range reqs {
		for _, rl := range r.GetResourceLogs() {
			for _, sl := range rl.GetScopeLogs() {
				n += len(sl.GetLogRecords())
			}
		}
	}
	return n
}

func TestOTLPSink_SubmitExportsRecord(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 0, time.Hour)

	start := time.Now()
	s.Submit(&Record{StartTime: start, Method: "GET", Path: "/foo", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d Export requests, want 1", len(reqs))
	}
	recs := reqs[0].GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()
	if len(recs) != 1 {
		t.Fatalf("got %d log records, want 1", len(recs))
	}
	if got, want := recs[0].GetTimeUnixNano(), uint64(start.UnixNano()); got != want {
		t.Errorf("TimeUnixNano = %d, want %d", got, want)
	}
	if written.Load() != 1 {
		t.Errorf("logsWritten = %d, want 1", written.Load())
	}
	if dropped.Load() != 0 {
		t.Errorf("logsDropped = %d, want 0", dropped.Load())
	}
}

func TestOTLPSink_BuiltinLabels(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 0, time.Hour)

	s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/foo", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d Export requests, want 1", len(reqs))
	}
	attrs := reqs[0].GetResourceLogs()[0].GetResource().GetAttributes()
	got := map[string]string{}
	for _, kv := range attrs {
		got[kv.GetKey()] = kv.GetValue().GetStringValue()
	}
	want := map[string]string{
		"log_name":     "mylog",
		"zone_name":    "zone-1",
		"cluster_name": "cluster-1",
		"node_name":    "node-1",
	}
	if len(got) != len(want) {
		t.Fatalf("attributes = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("attribute %q = %q, want %q", k, got[k], v)
		}
	}
}

func TestOTLPSink_DisableBuiltinLabels(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), true, nil, nil, nil, written, dropped, 0, time.Hour)

	start := time.Now()
	s.Submit(&Record{StartTime: start, Method: "GET", Path: "/foo", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d Export requests, want 1", len(reqs))
	}
	if attrs := reqs[0].GetResourceLogs()[0].GetResource().GetAttributes(); len(attrs) != 0 {
		t.Errorf("Resource.Attributes = %v, want empty (disableBuiltinLabels)", attrs)
	}
	recs := reqs[0].GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()
	if len(recs) != 1 || recs[0].GetTimeUnixNano() != uint64(start.UnixNano()) {
		t.Errorf("TimeUnixNano not present with disabled labels; recs=%v", recs)
	}
}

// otlpBatchRecord is the fixed record used by the buffering tests.
func otlpBatchRecord() *Record {
	return &Record{StartTime: time.Now(), Method: "GET", Path: "/buf", ResponseCode: 200}
}

// otlpEntrySize is the serialized byte size of one built LogRecord. The built-in
// LogRecord carries only time_unix_nano, whose varint width grows with the value;
// pinning to a small fixed value keeps it constant so a size threshold splits
// cleanly across records.
func otlpEntrySize() int {
	return proto.Size(buildLogRecord(&Record{StartTime: time.Unix(0, 1)}, nil, nil))
}

// otlpRequestRecords returns the LogRecords of a recorded Export request's single
// ResourceLogs/ScopeLogs.
func otlpRequestRecords(req *collogspb.ExportLogsServiceRequest) []*logspb.LogRecord {
	return req.GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()
}

// waitForOTLPRecords polls the fake until it has recorded >= want LogRecords or
// the deadline elapses (never a bare sleep-then-assert).
func waitForOTLPRecords(t *testing.T, client *fakeOTLPClient, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if otlpRecords(client.requests()) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d records; got %d", want, otlpRecords(client.requests()))
}

// TestOTLPSink_SizeTriggerBatches drives the SIZE trigger mid-life so the fake
// records MULTIPLE Export requests WITHOUT relying on the close-drain flush. It
// also proves the buf-reuse defensive-copy contract: an earlier recorded
// request's records must NOT be corrupted by a later flush reusing the backing
// array (this is what makes the fake's proto.Clone load-bearing).
func TestOTLPSink_SizeTriggerBatches(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)
	// threshold = 2*entrySize+1 ⇒ flush on the 3rd record of each batch. Records
	// are pinned to distinct small fixed timestamps so the serialized size stays
	// constant AND each batch is content-distinguishable (to detect corruption).
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 2*otlpEntrySize()+1, time.Hour)

	const n = 6
	for i := 0; i < n; i++ {
		s.Submit(&Record{StartTime: time.Unix(0, int64(i+1)), Method: "GET", Path: "/buf", ResponseCode: 200})
	}
	// The size trigger must fire BEFORE Close: two full batches of 3 ⇒ 2 Exports.
	waitForOTLPRecords(t, client, n, 5*time.Second)

	reqs := client.requests()
	if len(reqs) < 2 {
		t.Fatalf("got %d Export requests, want >= 2 (size trigger fired mid-life, not close-drain)", len(reqs))
	}
	if got := otlpRecords(reqs); got != n {
		t.Fatalf("total records = %d, want %d", got, n)
	}
	// Buf-reuse contract: the recorded requests must carry the ORIGINAL, distinct
	// timestamps in submit order — not corrupted by a later flush reusing the
	// backing array. Without the fake's defensive copy this assertion fails.
	var seen []uint64
	for _, r := range reqs {
		for _, lr := range otlpRequestRecords(r) {
			seen = append(seen, lr.GetTimeUnixNano())
		}
	}
	if len(seen) != n {
		t.Fatalf("collected %d record timestamps, want %d", len(seen), n)
	}
	for i := 0; i < n; i++ {
		if want := uint64(i + 1); seen[i] != want {
			t.Errorf("record[%d] TimeUnixNano = %d, want %d (buf-reuse corrupted an earlier recorded request)", i, seen[i], want)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if written.Load() != n {
		t.Errorf("logsWritten = %d, want %d (batch-invariant)", written.Load(), n)
	}
}

// TestOTLPSink_TimerTriggerFlushes drives the TIMER trigger: a short interval +
// a large size threshold (never crosses) ⇒ the tick flushes the buffered record
// with NO Close needed.
func TestOTLPSink_TimerTriggerFlushes(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 1<<30, 25*time.Millisecond)

	start := time.Unix(0, 42)
	s.Submit(&Record{StartTime: start, Method: "GET", Path: "/foo", ResponseCode: 200})
	waitForOTLPRecords(t, client, 1, 5*time.Second) // poll until the tick flushes

	reqs := client.requests()
	if got := otlpRecords(reqs); got != 1 {
		t.Fatalf("total records = %d, want 1 (timer-flushed before Close)", got)
	}
	if recs := otlpRequestRecords(reqs[0]); recs[0].GetTimeUnixNano() != uint64(start.UnixNano()) {
		t.Errorf("timer-flushed TimeUnixNano = %d, want %d", recs[0].GetTimeUnixNano(), uint64(start.UnixNano()))
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if written.Load() != 1 {
		t.Errorf("logsWritten = %d, want 1", written.Load())
	}
}

// TestOTLPSink_NoPanicWithParseDefaultInterval pins that a parse-default interval
// (1s) does not trip the time.NewTicker(<=0) panic in run().
func TestOTLPSink_NoPanicWithParseDefaultInterval(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 1<<30, 1*time.Second)

	s.Submit(otlpBatchRecord())
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if written.Load() != 1 {
		t.Errorf("logsWritten = %d, want 1", written.Load())
	}
}

// TestOTLPSink_CloseDrainFlush proves the close-drain flush: a HUGE size + LONG
// interval ⇒ neither the size nor the timer trigger fires; only Close flushes the
// pending buffer, carrying all 3 records in ONE Export.
func TestOTLPSink_CloseDrainFlush(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)
	// HUGE size threshold (never crosses on 3 records) + LONG interval ⇒ only the
	// close-drain flush fires, carrying all 3 in ONE Export.
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 1<<30, time.Hour)

	for i := 0; i < 3; i++ {
		s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/buf", ResponseCode: 200})
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d Export requests, want 1 (single close-drain flush)", len(reqs))
	}
	recs := reqs[0].GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()
	if len(recs) != 3 {
		t.Errorf("batch records = %d, want 3", len(recs))
	}
	if written.Load() != 3 {
		t.Errorf("logsWritten = %d, want 3", written.Load())
	}
}

func TestOTLPSink_DropNewest(t *testing.T) {
	block := make(chan struct{})
	client := &fakeOTLPClient{blockCh: block}
	written, dropped := newGrpcTestCounters(t)
	s := newOTLPSinkWithCapacity(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 0, time.Hour, 1)

	rec := &Record{StartTime: time.Now(), Method: "GET", Path: "/x", ResponseCode: 200}
	for i := 0; i < 100; i++ {
		s.Submit(rec) // must never block with the writer wedged in Export
	}
	if dropped.Load() == 0 {
		t.Errorf("expected at least one drop with a wedged writer; logsDropped = 0")
	}

	close(block) // release the writer so Close can drain
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestOTLPSink_RetryOnceOnExportError(t *testing.T) {
	client := &fakeOTLPClient{exportErrs: []error{errors.New("export boom")}} // first fails, then succeeds
	written, dropped := newGrpcTestCounters(t)
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 0, time.Hour)

	start := time.Now()
	s.Submit(&Record{StartTime: start, Method: "GET", Path: "/foo", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d Export requests, want 1 (record lands after retry)", len(reqs))
	}
	recs := reqs[0].GetResourceLogs()[0].GetScopeLogs()[0].GetLogRecords()
	if len(recs) != 1 || recs[0].GetTimeUnixNano() != uint64(start.UnixNano()) {
		t.Errorf("retried batch records = %v, want one record", recs)
	}
	if written.Load() != 1 {
		t.Errorf("logsWritten = %d, want 1", written.Load())
	}
}

func TestOTLPSink_SecondFailureDropsBatch(t *testing.T) {
	client := &fakeOTLPClient{exportErrs: []error{errors.New("boom1"), errors.New("boom2")}} // both attempts fail
	written, dropped := newGrpcTestCounters(t)
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 0, time.Hour)

	s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/foo", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if reqs := client.requests(); len(reqs) != 0 {
		t.Errorf("got %d landed Export requests, want 0 (both attempts failed)", len(reqs))
	}
	if written.Load() != 0 {
		t.Errorf("logsWritten = %d, want 0 (dropped batch is logged-not-counted)", written.Load())
	}
	if dropped.Load() != 0 {
		t.Errorf("logsDropped = %d, want 0 (flush-path drops are logged-not-counted)", dropped.Load())
	}
}

func TestOTLPSink_CloseIdempotent(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 0, time.Hour)

	s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/x", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if got := client.closes(); got != 1 {
		t.Errorf("client.Close called %d times, want exactly 1", got)
	}
}

func TestOTLPSink_NonRecordIgnored(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)
	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, nil, nil, nil, written, dropped, 0, time.Hour)

	s.Submit("garbage")
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if reqs := client.requests(); len(reqs) != 0 {
		t.Errorf("got %d Export requests, want 0", len(reqs))
	}
	if written.Load() != 0 {
		t.Errorf("logsWritten = %d, want 0", written.Load())
	}
	if otlpRecords(client.requests()) != 0 {
		t.Errorf("got %d log records, want 0", otlpRecords(client.requests()))
	}
}

// otlpStringValue is a test helper: a literal stringValue AnyValue.
func otlpStringValue(s string) *commonpb.AnyValue {
	return &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: s}}
}

// otlpCompileString is a test helper: compile a %OPERATOR%-templated string leaf
// into an *OTLPValueTemplate (the string arm).
func otlpCompileString(t *testing.T, s string) *OTLPValueTemplate {
	t.Helper()
	v, err := CompileOTLPValue(otlpStringValue(s))
	if err != nil {
		t.Fatalf("CompileOTLPValue(%q): %v", s, err)
	}
	return v
}

// TestOTLPSink_BodyAttributesResourceEndToEnd threads a compiled body, one compiled
// attribute, and a literal resource_attribute through the sink and asserts they land
// on the exported record/resource (body operator-substituted, attribute
// operator-substituted, resource literal appended AFTER the 4 built-ins).
func TestOTLPSink_BodyAttributesResourceEndToEnd(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)

	body := otlpCompileString(t, "%REQ(:METHOD)% %RESPONSE_CODE%")
	attrs := []OTLPAttrTemplate{{Key: "m", Value: otlpCompileString(t, "%REQ(:METHOD)%")}}
	resourceAttrs := []*commonpb.KeyValue{{Key: "svc", Value: otlpStringValue("x")}}

	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), false, body, attrs, resourceAttrs, written, dropped, 0, time.Hour)

	start := time.Now()
	s.Submit(&Record{StartTime: start, Method: "GET", Path: "/foo", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d Export requests, want 1", len(reqs))
	}
	rl := reqs[0].GetResourceLogs()[0]
	recs := rl.GetScopeLogs()[0].GetLogRecords()
	if len(recs) != 1 {
		t.Fatalf("got %d log records, want 1", len(recs))
	}
	if got := recs[0].GetBody().GetStringValue(); got != "GET 200" {
		t.Errorf("Body.StringValue = %q, want %q", got, "GET 200")
	}
	if recs[0].GetTimeUnixNano() != uint64(start.UnixNano()) {
		t.Errorf("TimeUnixNano = %d, want %d", recs[0].GetTimeUnixNano(), uint64(start.UnixNano()))
	}
	recAttrs := map[string]string{}
	for _, kv := range recs[0].GetAttributes() {
		recAttrs[kv.GetKey()] = kv.GetValue().GetStringValue()
	}
	if recAttrs["m"] != "GET" || len(recAttrs) != 1 {
		t.Errorf("record attributes = %v, want {m:GET}", recAttrs)
	}

	resAttrs := map[string]string{}
	for _, kv := range rl.GetResource().GetAttributes() {
		resAttrs[kv.GetKey()] = kv.GetValue().GetStringValue()
	}
	want := map[string]string{
		"log_name":     "mylog",
		"zone_name":    "zone-1",
		"cluster_name": "cluster-1",
		"node_name":    "node-1",
		"svc":          "x",
	}
	if len(resAttrs) != len(want) {
		t.Fatalf("resource attributes = %v, want %v", resAttrs, want)
	}
	for k, v := range want {
		if resAttrs[k] != v {
			t.Errorf("resource attribute %q = %q, want %q", k, resAttrs[k], v)
		}
	}
}

// TestOTLPSink_DisableBuiltinResourceAttrsSurvive pins AMEND-OPS-5: with
// disableBuiltinLabels the 4 built-in Resource labels drop but the literal
// resource_attributes SURVIVE, and the templated body/attributes still land on the
// record.
func TestOTLPSink_DisableBuiltinResourceAttrsSurvive(t *testing.T) {
	client := &fakeOTLPClient{}
	written, dropped := newGrpcTestCounters(t)

	body := otlpCompileString(t, "%REQ(:METHOD)% %RESPONSE_CODE%")
	attrs := []OTLPAttrTemplate{{Key: "m", Value: otlpCompileString(t, "%REQ(:METHOD)%")}}
	resourceAttrs := []*commonpb.KeyValue{{Key: "svc", Value: otlpStringValue("x")}}

	s := NewOTLPAccessLogSink(client, "mylog", otlpTestNode(), true, body, attrs, resourceAttrs, written, dropped, 0, time.Hour)

	s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/foo", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reqs := client.requests()
	if len(reqs) != 1 {
		t.Fatalf("got %d Export requests, want 1", len(reqs))
	}
	rl := reqs[0].GetResourceLogs()[0]
	resAttrs := rl.GetResource().GetAttributes()
	if len(resAttrs) != 1 || resAttrs[0].GetKey() != "svc" || resAttrs[0].GetValue().GetStringValue() != "x" {
		t.Errorf("resource attributes = %v, want just {svc:x} (AMEND-OPS-5)", resAttrs)
	}
	recs := rl.GetScopeLogs()[0].GetLogRecords()
	if len(recs) != 1 {
		t.Fatalf("got %d log records, want 1", len(recs))
	}
	if got := recs[0].GetBody().GetStringValue(); got != "GET 200" {
		t.Errorf("Body.StringValue = %q, want %q", got, "GET 200")
	}
	recAttrs := map[string]string{}
	for _, kv := range recs[0].GetAttributes() {
		recAttrs[kv.GetKey()] = kv.GetValue().GetStringValue()
	}
	if recAttrs["m"] != "GET" || len(recAttrs) != 1 {
		t.Errorf("record attributes = %v, want {m:GET}", recAttrs)
	}
}
