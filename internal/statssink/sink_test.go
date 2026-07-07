package statssink

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/internal/grpcclient"
)

// The real client satisfies the metricsClient seam. Kept TEST-ONLY so the
// production statssink package stays decoupled from grpcclient (the accesslog
// alsClient-seam precedent).
var _ metricsClient = (*grpcclient.MetricsServiceClient)(nil)

// fakeMetricsStream is a fake MetricsService_StreamMetricsClient. It embeds the
// generated interface (nil) so the grpc.ClientStream methods are satisfied
// structurally; only Send + CloseAndRecv are exercised by the sink (the
// grpcsink_test.go fakeStream precedent).
type fakeMetricsStream struct {
	metricsv3.MetricsService_StreamMetricsClient

	mu             sync.Mutex
	sent           []*metricsv3.StreamMetricsMessage
	closeRecvCount int

	// sendErrs is a per-call error queue: sendErrs[i] (nil == success) is
	// returned by the i-th Send. Beyond the slice, Send succeeds.
	sendErrs []error
	sendIdx  int

	// blockCh, when non-nil, blocks each Send on a receive until the test
	// closes it (used to wedge the writer goroutine for the drop test).
	blockCh chan struct{}

	// sendStarted, when non-nil, receives one signal at the start of each Send
	// (BEFORE the blockCh wait) so a test can observe that the writer goroutine
	// has dequeued a batch and is wedged inside Send. Must be buffered deeply
	// enough for every Send the test triggers.
	sendStarted chan struct{}
}

func (f *fakeMetricsStream) Send(m *metricsv3.StreamMetricsMessage) error {
	if f.sendStarted != nil {
		f.sendStarted <- struct{}{}
	}
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

func (f *fakeMetricsStream) CloseAndRecv() (*metricsv3.StreamMetricsResponse, error) {
	f.mu.Lock()
	f.closeRecvCount++
	f.mu.Unlock()
	return &metricsv3.StreamMetricsResponse{}, nil
}

func (f *fakeMetricsStream) messages() []*metricsv3.StreamMetricsMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*metricsv3.StreamMetricsMessage, len(f.sent))
	copy(out, f.sent)
	return out
}

func (f *fakeMetricsStream) closeRecvCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closeRecvCount
}

// fakeMetricsClient is a fake metricsClient seam. StreamMetrics hands out the
// stream at streamIdx (optionally erroring per a queue); Close is counted.
type fakeMetricsClient struct {
	mu         sync.Mutex
	streams    []*fakeMetricsStream
	streamErrs []error
	streamIdx  int
	openCount  int
	closeCount int
}

func (c *fakeMetricsClient) StreamMetrics(_ context.Context) (metricsv3.MetricsService_StreamMetricsClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var err error
	if c.streamIdx < len(c.streamErrs) {
		err = c.streamErrs[c.streamIdx]
	}
	idx := c.streamIdx
	c.streamIdx++
	c.openCount++
	if err != nil {
		return nil, err
	}
	if idx < len(c.streams) {
		return c.streams[idx], nil
	}
	return c.streams[len(c.streams)-1], nil
}

func (c *fakeMetricsClient) Close() error {
	c.mu.Lock()
	c.closeCount++
	c.mu.Unlock()
	return nil
}

func (c *fakeMetricsClient) opens() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.openCount
}

func (c *fakeMetricsClient) closes() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeCount
}

func testNode() *corev3.Node { return &corev3.Node{Id: "test-node", Cluster: "test-cluster"} }

// fam builds a one-metric COUNTER family with the given name and value (a stand-in
// for snapshot() output; the sink is mapping-agnostic).
func fam(name string, value float64) *dto.MetricFamily {
	return &dto.MetricFamily{
		Name: proto.String(name),
		Type: dto.MetricType_COUNTER.Enum(),
		Metric: []*dto.Metric{{
			Counter: &dto.Counter{Value: proto.Float64(value)},
		}},
	}
}

func batchNames(m *metricsv3.StreamMetricsMessage) []string {
	var out []string
	for _, f := range m.GetEnvoyMetrics() {
		out = append(out, f.GetName())
	}
	return out
}

func TestSink_IdentifierOnce(t *testing.T) {
	stream := &fakeMetricsStream{}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := NewMetricsServiceSink(client, testNode(), false, false)

	batch1 := []*dto.MetricFamily{fam("a.one", 1)}
	batch2 := []*dto.MetricFamily{fam("b.two", 2)}
	s.Submit(batch1)
	s.Submit(batch2)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 2 {
		t.Fatalf("got %d streamed messages, want 2", len(msgs))
	}
	// Message #1: identifier present (node id + cluster), batch1.
	id := msgs[0].GetIdentifier()
	if id == nil {
		t.Fatalf("message[0] identifier = nil, want non-nil")
	}
	if id.GetNode().GetId() != "test-node" {
		t.Errorf("identifier node id = %q, want %q", id.GetNode().GetId(), "test-node")
	}
	if id.GetNode().GetCluster() != "test-cluster" {
		t.Errorf("identifier node cluster = %q, want %q", id.GetNode().GetCluster(), "test-cluster")
	}
	if got := batchNames(msgs[0]); len(got) != 1 || got[0] != "a.one" {
		t.Errorf("message[0] families = %v, want [a.one]", got)
	}
	// Message #2: identifier nil (identifier-once), batch2.
	if msgs[1].GetIdentifier() != nil {
		t.Errorf("message[1] identifier = non-nil, want nil (identifier-once)")
	}
	if got := batchNames(msgs[1]); len(got) != 1 || got[0] != "b.two" {
		t.Errorf("message[1] families = %v, want [b.two]", got)
	}
}

func TestSink_ReconnectResend(t *testing.T) {
	// First stream errors on the 2nd Send; second stream succeeds.
	stream1 := &fakeMetricsStream{sendErrs: []error{nil, errors.New("send boom")}}
	stream2 := &fakeMetricsStream{}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream1, stream2}}
	s := NewMetricsServiceSink(client, testNode(), false, false)

	s.Submit([]*dto.MetricFamily{fam("a.one", 1)}) // lands on stream1 (msg #1, identifier)
	s.Submit([]*dto.MetricFamily{fam("b.two", 2)}) // fails on stream1 -> reopen stream2, re-send
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if client.opens() < 2 {
		t.Errorf("openCount = %d, want >= 2 (reconnect re-opened the stream)", client.opens())
	}

	// stream1 took the first batch with the identifier.
	m1 := stream1.messages()
	if len(m1) != 1 {
		t.Fatalf("stream1 got %d messages, want 1", len(m1))
	}
	if m1[0].GetIdentifier() == nil {
		t.Errorf("stream1 message[0] identifier = nil, want non-nil")
	}

	// stream2 re-sent the SECOND batch WITH a re-armed identifier.
	m2 := stream2.messages()
	if len(m2) != 1 {
		t.Fatalf("stream2 got %d messages, want 1 (failed batch re-sent)", len(m2))
	}
	if m2[0].GetIdentifier() == nil {
		t.Errorf("stream2 message[0] identifier = nil, want re-armed identifier")
	}
	if got := batchNames(m2[0]); len(got) != 1 || got[0] != "b.two" {
		t.Errorf("stream2 re-sent families = %v, want [b.two]", got)
	}
}

func TestSink_DropOnFull(t *testing.T) {
	block := make(chan struct{})
	stream := &fakeMetricsStream{blockCh: block}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := newSinkWithCapacity(client, testNode(), false, false, 1)

	// With the writer wedged in Send, the channel fills; Submit must never block.
	for i := 0; i < 100; i++ {
		s.Submit([]*dto.MetricFamily{fam("a.one", 1)}) // never blocks (select-default drop)
	}

	close(block) // release the writer so Close can drain
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestSink_CloseIdempotent(t *testing.T) {
	stream := &fakeMetricsStream{}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := NewMetricsServiceSink(client, testNode(), false, false)

	s.Submit([]*dto.MetricFamily{fam("a.one", 1)})
	if err := s.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if got := client.closes(); got != 1 {
		t.Errorf("client.Close called %d times, want exactly 1", got)
	}
	if stream.closeRecvCalls() == 0 {
		t.Errorf("CloseAndRecv not called; Close must drain the in-flight stream")
	}
}

func TestSink_DeltaMode_RewritesCountersToDeltas(t *testing.T) {
	stream := &fakeMetricsStream{}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := newSinkWithCapacity(client, testNode(), true /*deltas*/, false /*labels*/, 8)

	s.Submit([]*dto.MetricFamily{counterFam("c.rq", 7)})  // first flush -> absolute 7
	s.Submit([]*dto.MetricFamily{counterFam("c.rq", 10)}) // delta 3
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 2 {
		t.Fatalf("got %d streamed messages, want 2", len(msgs))
	}
	if got := msgs[0].GetEnvoyMetrics()[0].GetMetric()[0].GetCounter().GetValue(); got != 7 {
		t.Errorf("message[0] counter = %v, want 7 (first flush absolute)", got)
	}
	if got := msgs[1].GetEnvoyMetrics()[0].GetMetric()[0].GetCounter().GetValue(); got != 3 {
		t.Errorf("message[1] counter = %v, want 3 (per-flush delta)", got)
	}
}

// TestSink_DeltaMode_EnqueueDropDoesNotLatch pins the item-2 fix: the delta
// transform runs in the WRITER goroutine (just before flush), so a batch dropped
// at enqueue (full channel) never latches deltaState — its increments ride the
// next successfully-enqueued flush's delta instead of being silently lost. The
// pre-fix Submit-side apply latched the dropped batch's value, permanently
// undercounting the delta stream.
func TestSink_DeltaMode_EnqueueDropDoesNotLatch(t *testing.T) {
	block := make(chan struct{})
	started := make(chan struct{}, 8)
	stream := &fakeMetricsStream{blockCh: block, sendStarted: started}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := newSinkWithCapacity(client, testNode(), true /*deltas*/, false /*labels*/, 1)

	// Batch A (7): the writer dequeues it, latches 7, and wedges inside Send.
	s.Submit([]*dto.MetricFamily{counterFam("c.rq", 7)})
	<-started // the writer is now wedged in Send and the 1-deep channel is EMPTY

	// Batch B (10) fills the channel; batch C (12) is DROPPED at enqueue. The
	// drop must NOT latch deltaState (the pre-fix bug latched 12 here).
	s.Submit([]*dto.MetricFamily{counterFam("c.rq", 10)})
	s.Submit([]*dto.MetricFamily{counterFam("c.rq", 12)}) // channel full -> dropped

	close(block) // release the writer: A then B flush

	// Wait for B's message so the channel has room for D (Submit never blocks).
	deadline := time.Now().Add(5 * time.Second)
	for len(stream.messages()) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for 2 flushed messages, got %d", len(stream.messages()))
		}
		time.Sleep(time.Millisecond)
	}

	// Batch D (20): its delta must be 20-10=10 (relative to B, the last batch
	// the WRITER saw) — NOT 20-12=8, which would mean the dropped C latched.
	s.Submit([]*dto.MetricFamily{counterFam("c.rq", 20)})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 3 {
		t.Fatalf("got %d streamed messages, want 3 (A, B, D; C dropped)", len(msgs))
	}
	want := []float64{7, 3, 10} // A absolute, B delta 10-7, D delta 20-10
	for i, w := range want {
		if got := msgs[i].GetEnvoyMetrics()[0].GetMetric()[0].GetCounter().GetValue(); got != w {
			t.Errorf("message[%d] counter = %v, want %v", i, got, w)
		}
	}
}

func TestSink_LabelsMode_RewritesNameAndLabels(t *testing.T) {
	stream := &fakeMetricsStream{}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := newSinkWithCapacity(client, testNode(), false /*deltas*/, true /*emitTagsAsLabels*/, 8)

	s.Submit([]*dto.MetricFamily{counterFam("cluster.c_backend.upstream_rq_total", 7)})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 1 {
		t.Fatalf("got %d streamed messages, want 1", len(msgs))
	}
	fam := msgs[0].GetEnvoyMetrics()[0]
	if got := fam.GetName(); got != "cluster.upstream_rq_total" {
		t.Errorf("name = %q, want %q (tag-residual)", got, "cluster.upstream_rq_total")
	}
	labels := fam.GetMetric()[0].GetLabel()
	if len(labels) != 1 {
		t.Fatalf("got %d labels, want 1", len(labels))
	}
	if got := labels[0].GetName(); got != "envoy.cluster_name" {
		t.Errorf("label name = %q, want %q", got, "envoy.cluster_name")
	}
	if got := labels[0].GetValue(); got != "c_backend" {
		t.Errorf("label value = %q, want %q", got, "c_backend")
	}
	if got := fam.GetMetric()[0].GetCounter().GetValue(); got != 7 {
		t.Errorf("counter = %v, want 7 (cumulative — labels mode leaves value unchanged)", got)
	}
}

func TestSink_BothKnobs_DeltaThenLabels(t *testing.T) {
	stream := &fakeMetricsStream{}
	client := &fakeMetricsClient{streams: []*fakeMetricsStream{stream}}
	s := newSinkWithCapacity(client, testNode(), true /*deltas*/, true /*labels*/, 8)

	s.Submit([]*dto.MetricFamily{counterFam("cluster.c_backend.upstream_rq_total", 7)})  // first flush -> delta absolute 7
	s.Submit([]*dto.MetricFamily{counterFam("cluster.c_backend.upstream_rq_total", 10)}) // delta 3
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	msgs := stream.messages()
	if len(msgs) != 2 {
		t.Fatalf("got %d streamed messages, want 2", len(msgs))
	}
	for i, m := range msgs {
		fam := m.GetEnvoyMetrics()[0]
		if got := fam.GetName(); got != "cluster.upstream_rq_total" {
			t.Errorf("message[%d] name = %q, want %q (tag-residual)", i, got, "cluster.upstream_rq_total")
		}
		labels := fam.GetMetric()[0].GetLabel()
		if len(labels) != 1 || labels[0].GetName() != "envoy.cluster_name" || labels[0].GetValue() != "c_backend" {
			t.Errorf("message[%d] labels = %v, want [{envoy.cluster_name c_backend}]", i, labels)
		}
	}
	// Labels apply to the DELTA value: msg0 == 7 (first=absolute), msg1 == 3 (delta).
	if got := msgs[0].GetEnvoyMetrics()[0].GetMetric()[0].GetCounter().GetValue(); got != 7 {
		t.Errorf("message[0] counter = %v, want 7 (delta first=absolute)", got)
	}
	if got := msgs[1].GetEnvoyMetrics()[0].GetMetric()[0].GetCounter().GetValue(); got != 3 {
		t.Errorf("message[1] counter = %v, want 3 (per-flush delta with labels)", got)
	}
}
