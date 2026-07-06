package accesslog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	dataaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/internal/stats"
)

// fakeStream is a fake AccessLogService_StreamAccessLogsClient. It embeds the
// generated interface (nil) so the grpc.ClientStream methods are satisfied
// structurally; only Send + CloseAndRecv are exercised by the sink.
type fakeStream struct {
	accesslogv3.AccessLogService_StreamAccessLogsClient

	mu             sync.Mutex
	sent           []*accesslogv3.StreamAccessLogsMessage
	closeRecvCount int

	// sendErrs is a per-call error queue: sendErrs[i] (nil == success) is
	// returned by the i-th Send. Beyond the slice, Send succeeds.
	sendErrs []error
	sendIdx  int

	// blockCh, when non-nil, blocks each Send on a receive until the test
	// closes it (used to wedge the writer goroutine for the drop test).
	blockCh chan struct{}
}

func (f *fakeStream) Send(m *accesslogv3.StreamAccessLogsMessage) error {
	if f.blockCh != nil {
		<-f.blockCh
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	var err error
	if f.sendIdx < len(f.sendErrs) {
		err = f.sendErrs[f.sendIdx]
	}
	f.sendIdx++
	if err != nil {
		return err
	}
	// Defensive copy of the LogEntry slice header (buf-reuse contract): the sink
	// reuses buf's backing array across flushes (buf = buf[:0]), so a later
	// batch's append would overwrite this recorded message's entries. The real
	// gRPC Send serializes synchronously and is unaffected; the fake records the
	// message pointer, so it snapshots the slice into a fresh backing array here.
	// The entry pointers themselves are per-record-fresh, so copying them is safe.
	if hl := m.GetHttpLogs(); hl != nil {
		entries := hl.GetLogEntry()
		cp := make([]*dataaccesslogv3.HTTPAccessLogEntry, len(entries))
		copy(cp, entries)
		hl.LogEntry = cp
	}
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeStream) CloseAndRecv() (*accesslogv3.StreamAccessLogsResponse, error) {
	f.mu.Lock()
	f.closeRecvCount++
	f.mu.Unlock()
	return &accesslogv3.StreamAccessLogsResponse{}, nil
}

func (f *fakeStream) messages() []*accesslogv3.StreamAccessLogsMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*accesslogv3.StreamAccessLogsMessage, len(f.sent))
	copy(out, f.sent)
	return out
}

// fakeALSClient is a fake alsClient seam. StreamAccessLogs hands out the same
// fakeStream (optionally erroring per a queue); Close is counted.
type fakeALSClient struct {
	mu         sync.Mutex
	stream     *fakeStream
	streamErrs []error
	streamIdx  int
	openCount  int
	closeCount int
}

func (c *fakeALSClient) StreamAccessLogs(_ context.Context) (accesslogv3.AccessLogService_StreamAccessLogsClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	if c.streamIdx < len(c.streamErrs) {
		err = c.streamErrs[c.streamIdx]
	}
	c.streamIdx++
	c.openCount++
	if err != nil {
		return nil, err
	}
	return c.stream, nil
}

func (c *fakeALSClient) Close() error {
	c.mu.Lock()
	c.closeCount++
	c.mu.Unlock()
	return nil
}

func (c *fakeALSClient) closes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCount
}

func newGrpcTestCounters(t *testing.T) (*stats.Counter, *stats.Counter) {
	t.Helper()
	reg := stats.NewRegistry()
	return reg.NewCounter("test.written"), reg.NewCounter("test.dropped")
}

func testNode() *corev3.Node { return &corev3.Node{Id: "test-node"} }

func TestGrpcSink_SubmitStreamsEntry(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 0, time.Hour, nil, nil)

	s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/foo", Protocol: "HTTP/1.1", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d streamed messages, want 1", len(msgs))
	}
	entries := msgs[0].GetHttpLogs().GetLogEntry()
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1", len(entries))
	}
	if got := entries[0].GetRequest().GetPath(); got != "/foo" {
		t.Errorf("entry path = %q, want %q", got, "/foo")
	}
	if got := entries[0].GetRequest().GetRequestMethod(); got != corev3.RequestMethod_GET {
		t.Errorf("entry method = %v, want GET", got)
	}
	if written.Load() != 1 {
		t.Errorf("logsWritten = %d, want 1", written.Load())
	}
	if dropped.Load() != 0 {
		t.Errorf("logsDropped = %d, want 0", dropped.Load())
	}
}

func TestGrpcSink_IdentifierOnce(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 0, time.Hour, nil, nil)

	for i := 0; i < 3; i++ {
		s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/x", Protocol: "HTTP/1.1", ResponseCode: 200})
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 3 {
		t.Fatalf("got %d streamed messages, want 3", len(msgs))
	}
	id := msgs[0].GetIdentifier()
	if id == nil {
		t.Fatalf("first message identifier = nil, want non-nil")
	}
	if id.GetLogName() != "mylog" {
		t.Errorf("identifier log_name = %q, want %q", id.GetLogName(), "mylog")
	}
	if id.GetNode().GetId() != "test-node" {
		t.Errorf("identifier node id = %q, want %q", id.GetNode().GetId(), "test-node")
	}
	for i := 1; i < 3; i++ {
		if msgs[i].GetIdentifier() != nil {
			t.Errorf("message[%d] identifier = non-nil, want nil (identifier-once)", i)
		}
		if msgs[i].GetHttpLogs() == nil {
			t.Errorf("message[%d] http logs = nil, want non-nil", i)
		}
	}
}

func TestGrpcSink_DropNewest(t *testing.T) {
	block := make(chan struct{})
	stream := &fakeStream{blockCh: block}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	s := newGrpcSinkWithCapacity(client, "mylog", testNode(), written, dropped, 0, time.Hour, nil, nil, 1)

	rec := &Record{StartTime: time.Now(), Method: "GET", Path: "/x", Protocol: "HTTP/1.1", ResponseCode: 200}
	for i := 0; i < 100; i++ {
		s.Submit(rec) // must never block even with the writer wedged in Send
	}
	if dropped.Load() == 0 {
		t.Errorf("expected at least one drop with a wedged writer; logsDropped = 0")
	}

	close(block) // release the writer so Close can drain
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestGrpcSink_ReconnectOnSendError(t *testing.T) {
	stream := &fakeStream{sendErrs: []error{errors.New("send boom")}} // first Send errors, then success
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 0, time.Hour, nil, nil)

	s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/foo", Protocol: "HTTP/1.1", ResponseCode: 200})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d streamed messages, want 1 (entry lands after reconnect)", len(msgs))
	}
	if msgs[0].GetIdentifier() == nil {
		t.Errorf("reconnect message identifier = nil, want re-sent identifier")
	}
	if entries := msgs[0].GetHttpLogs().GetLogEntry(); len(entries) != 1 || entries[0].GetRequest().GetPath() != "/foo" {
		t.Errorf("reconnect message entries = %v, want one /foo entry", entries)
	}
	if client.openCount < 2 {
		t.Errorf("openCount = %d, want >= 2 (reconnect re-opened the stream)", client.openCount)
	}
	if written.Load() != 1 {
		t.Errorf("logsWritten = %d, want 1", written.Load())
	}
}

func TestGrpcSink_CloseIdempotent(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 0, time.Hour, nil, nil)

	s.Submit(&Record{StartTime: time.Now(), Method: "GET", Path: "/x", Protocol: "HTTP/1.1", ResponseCode: 200})
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

func TestGrpcSink_NonRecordIgnored(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 0, time.Hour, nil, nil)

	s.Submit("garbage")
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if msgs := stream.messages(); len(msgs) != 0 {
		t.Errorf("got %d streamed messages, want 0", len(msgs))
	}
	if written.Load() != 0 {
		t.Errorf("logsWritten = %d, want 0", written.Load())
	}
}

// batchRecord is the fixed record used by the buffering tests; every entry has
// identical serialized size, so a size threshold of k*entrySize+1 flushes on the
// (k+1)-th entry.
func batchRecord() *Record {
	return &Record{StartTime: time.Now(), Method: "GET", Path: "/buf", Protocol: "HTTP/1.1", ResponseCode: 200}
}

// entrySize is the serialized byte size of one built HTTPAccessLogEntry for
// batchRecord (constant across entries — all fields identical).
func entrySize() int { return proto.Size(buildHTTPAccessLogEntry(batchRecord(), nil, nil)) }

// totalEntries sums GetLogEntry() lengths across all recorded messages.
func totalEntries(msgs []*accesslogv3.StreamAccessLogsMessage) int {
	n := 0
	for _, m := range msgs {
		n += len(m.GetHttpLogs().GetLogEntry())
	}
	return n
}

// maxEntriesPerMessage is the largest single-message batch observed.
func maxEntriesPerMessage(msgs []*accesslogv3.StreamAccessLogsMessage) int {
	best := 0
	for _, m := range msgs {
		if n := len(m.GetHttpLogs().GetLogEntry()); n > best {
			best = n
		}
	}
	return best
}

// waitForEntries polls the fake until it has recorded >= want entries or the
// deadline elapses (never a bare sleep-then-assert).
func waitForEntries(t *testing.T, stream *fakeStream, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if totalEntries(stream.messages()) >= want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d entries; got %d", want, totalEntries(stream.messages()))
}

func TestGrpcSink_SizeTriggerBatches(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	// threshold = 2*entrySize+1 ⇒ flush on the 3rd entry of each batch.
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 2*entrySize()+1, time.Hour, nil, nil)

	for i := 0; i < 6; i++ {
		s.Submit(batchRecord())
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if got := totalEntries(msgs); got != 6 {
		t.Fatalf("total entries = %d, want 6", got)
	}
	if maxEntriesPerMessage(msgs) < 2 {
		t.Errorf("max entries per message = %d, want >= 2 (size trigger batched)", maxEntriesPerMessage(msgs))
	}
	// logs_written batch-invariant: counts ENTRIES not messages.
	if written.Load() != 6 {
		t.Errorf("logsWritten = %d, want 6", written.Load())
	}
	if len(msgs) >= 6 {
		t.Errorf("messageCount = %d, want < 6 (batched, fewer messages than entries)", len(msgs))
	}
	// identifier-once carries: first flushed message identified, the rest nil.
	if msgs[0].GetIdentifier() == nil {
		t.Errorf("first message identifier = nil, want non-nil")
	}
	for i := 1; i < len(msgs); i++ {
		if msgs[i].GetIdentifier() != nil {
			t.Errorf("message[%d] identifier = non-nil, want nil (identifier-once)", i)
		}
	}
}

func TestGrpcSink_TimerTriggerBatches(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	// HUGE size (never crosses) + SHORT interval ⇒ the timer flushes the batch.
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 1<<30, 50*time.Millisecond, nil, nil)

	const n = 5
	for i := 0; i < n; i++ {
		s.Submit(batchRecord())
	}
	waitForEntries(t, stream, n, 5*time.Second) // poll until the tick flushes

	msgs := stream.messages()
	if got := totalEntries(msgs); got != n {
		t.Fatalf("total entries = %d, want %d", got, n)
	}
	if maxEntriesPerMessage(msgs) < 2 {
		t.Errorf("max entries per message = %d, want >= 2 (timer batched)", maxEntriesPerMessage(msgs))
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if written.Load() != n {
		t.Errorf("logsWritten = %d, want %d", written.Load(), n)
	}
}

func TestGrpcSink_FlushOnClose(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	// HUGE size + LONG interval ⇒ neither trigger fires; only Close flushes.
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 1<<30, time.Hour, nil, nil)

	for i := 0; i < 3; i++ {
		s.Submit(batchRecord())
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if got := totalEntries(msgs); got != 3 {
		t.Fatalf("total entries = %d, want 3 (flush-on-close, none lost)", got)
	}
	if maxEntriesPerMessage(msgs) != 3 {
		t.Errorf("max entries per message = %d, want 3 (single final batch)", maxEntriesPerMessage(msgs))
	}
	if written.Load() != 3 {
		t.Errorf("logsWritten = %d, want 3", written.Load())
	}
	if stream.closeRecvCount == 0 {
		t.Errorf("CloseAndRecv not called; the final batch must flush before close")
	}
}

func TestGrpcSink_ZeroSizeFlushesEveryEntry(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	// bufferSizeBytes == 0 ⇒ sum >= 0 always crosses ⇒ flush every entry.
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 0, time.Hour, nil, nil)

	for i := 0; i < 4; i++ {
		s.Submit(batchRecord())
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 4 {
		t.Fatalf("got %d messages, want 4 (flush-every-entry)", len(msgs))
	}
	for i, m := range msgs {
		if got := len(m.GetHttpLogs().GetLogEntry()); got != 1 {
			t.Errorf("message[%d] entries = %d, want exactly 1", i, got)
		}
	}
	if written.Load() != 4 {
		t.Errorf("logsWritten = %d, want 4", written.Load())
	}
}

func TestGrpcSink_ReconnectResendsWholeBatch(t *testing.T) {
	stream := &fakeStream{sendErrs: []error{errors.New("send boom")}} // first batch Send errors, then success
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	// threshold = 2*entrySize+1 ⇒ the 3rd Submit flushes a batch of 3.
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 2*entrySize()+1, time.Hour, nil, nil)

	for i := 0; i < 3; i++ {
		s.Submit(batchRecord())
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want 1 (whole batch lands after reconnect)", len(msgs))
	}
	if got := len(msgs[0].GetHttpLogs().GetLogEntry()); got != 3 {
		t.Errorf("batch entries = %d, want 3 (whole batch resent)", got)
	}
	if msgs[0].GetIdentifier() == nil {
		t.Errorf("reconnect message identifier = nil, want re-attached identifier")
	}
	if client.openCount < 2 {
		t.Errorf("openCount = %d, want >= 2 (reconnect re-opened the stream)", client.openCount)
	}
	if written.Load() != 3 {
		t.Errorf("logsWritten = %d, want 3 (batch counted once, not doubled)", written.Load())
	}
}

// TestGrpcSink_OpenFailureDropsBatch proves a stream-OPEN failure DROPS the
// accumulated batch (logged-not-counted) rather than retaining it — keeping
// memory bounded under a sustained outage (44.1's open-failure-drops policy).
func TestGrpcSink_OpenFailureDropsBatch(t *testing.T) {
	stream := &fakeStream{}
	// First open errors (the first batch's flush) then succeeds (the second's).
	client := &fakeALSClient{stream: stream, streamErrs: []error{errors.New("open boom")}}
	written, dropped := newGrpcTestCounters(t)
	// threshold = 2*entrySize+1 ⇒ flush every 3rd entry.
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 2*entrySize()+1, time.Hour, nil, nil)

	// Identical records ⇒ a clean 3-then-flush split (entrySize constant).
	for i := 0; i < 6; i++ {
		s.Submit(batchRecord())
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	// The open-failed first batch must NOT survive into the second flush: only the
	// 3 second-batch entries land (if buf were retained, we'd see 6 entries — the
	// dropped batch prepended to the second).
	if got := totalEntries(msgs); got != 3 {
		t.Fatalf("total entries = %d, want 3 (open-failed batch dropped, not retained)", got)
	}
	if written.Load() != 3 {
		t.Errorf("logsWritten = %d, want 3 (only the second batch counted; the dropped one not counted)", written.Load())
	}
	// logs_dropped stays channel-full-overflow-only (AMEND-ALS-1): a flush-path
	// drop is logged but NOT counted there.
	if dropped.Load() != 0 {
		t.Errorf("logsDropped = %d, want 0 (flush-path drops are logged-not-counted)", dropped.Load())
	}
}

func TestGrpcSink_CaptureHeaderNames(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	req := []string{"x-a", "x-b"}
	resp := []string{"content-type"}
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 0, time.Hour, req, resp)
	defer func() { _ = s.Close() }()

	if got := s.CaptureRequestHeaderNames(); len(got) != 2 || got[0] != "x-a" || got[1] != "x-b" {
		t.Errorf("CaptureRequestHeaderNames() = %v, want %v", got, req)
	}
	if got := s.CaptureResponseHeaderNames(); len(got) != 1 || got[0] != "content-type" {
		t.Errorf("CaptureResponseHeaderNames() = %v, want %v", got, resp)
	}

	// The sink satisfies the headerCaptureSink shape (req/resp accessors).
	var _ interface {
		CaptureRequestHeaderNames() []string
		CaptureResponseHeaderNames() []string
	} = s
}

func TestGrpcSink_StreamsFilteredHeaders(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	// This sink captures only x-a (req) + content-type (resp); x-b is in the
	// Record (the emit-hook UNION) but NOT in this sink's subset, so it must be
	// filtered out of the streamed entry.
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 0, time.Hour, []string{"x-a"}, []string{"content-type"})

	s.Submit(&Record{
		StartTime:       time.Now(),
		Method:          "GET",
		Path:            "/foo",
		Protocol:        "HTTP/1.1",
		ResponseCode:    200,
		RequestHeaders:  map[string]string{"x-a": "1", "x-b": "2"},
		ResponseHeaders: map[string]string{"content-type": "text/plain"},
	})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d streamed messages, want 1", len(msgs))
	}
	entries := msgs[0].GetHttpLogs().GetLogEntry()
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1", len(entries))
	}
	reqHdrs := entries[0].GetRequest().GetRequestHeaders()
	if len(reqHdrs) != 1 || reqHdrs["x-a"] != "1" {
		t.Errorf("request_headers = %v, want {x-a:1} (x-b filtered out)", reqHdrs)
	}
	respHdrs := entries[0].GetResponse().GetResponseHeaders()
	if len(respHdrs) != 1 || respHdrs["content-type"] != "text/plain" {
		t.Errorf("response_headers = %v, want {content-type:text/plain}", respHdrs)
	}
}

func TestGrpcSink_NoCaptureHeadersByteStable(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	// No configured names ⇒ proto header maps stay nil even if the Record carried
	// captured maps (byte-identical to the 44.1/44.2 no-capture path).
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 0, time.Hour, nil, nil)

	s.Submit(&Record{
		StartTime:       time.Now(),
		Method:          "GET",
		Path:            "/foo",
		Protocol:        "HTTP/1.1",
		ResponseCode:    200,
		RequestHeaders:  map[string]string{"x-a": "1"},
		ResponseHeaders: map[string]string{"content-type": "text/plain"},
	})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	entries := stream.messages()[0].GetHttpLogs().GetLogEntry()
	if got := entries[0].GetRequest().GetRequestHeaders(); got != nil {
		t.Errorf("request_headers = %v, want nil (no capture configured)", got)
	}
	if got := entries[0].GetResponse().GetResponseHeaders(); got != nil {
		t.Errorf("response_headers = %v, want nil (no capture configured)", got)
	}
}

func TestGrpcSink_NoPanicWithParseDefaultInterval(t *testing.T) {
	stream := &fakeStream{}
	client := &fakeALSClient{stream: stream}
	written, dropped := newGrpcTestCounters(t)
	// 1s is the parse-layer default (the NewTicker panic-guard invariant: a 0
	// here would panic in run()).
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped, 1<<30, 1*time.Second, nil, nil)

	s.Submit(batchRecord())
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if written.Load() != 1 {
		t.Errorf("logsWritten = %d, want 1", written.Load())
	}
}
