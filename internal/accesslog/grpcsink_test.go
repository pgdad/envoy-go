package accesslog

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"

	"github.com/esalaine/envoy-go/internal/stats"
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
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped)

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
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped)

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
	s := newGrpcSinkWithCapacity(client, "mylog", testNode(), written, dropped, 1)

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
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped)

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
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped)

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
	s := NewGrpcAccessLogSink(client, "mylog", testNode(), written, dropped)

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
