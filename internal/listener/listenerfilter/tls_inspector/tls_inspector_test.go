package tls_inspector

import (
	"bufio"
	"context"
	"net"
	"sync"
	"testing"
	"time"

	tls_inspectorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
)

// peekConn is a minimal in-test Peeker that wraps a net.Conn with a
// bufio.Reader sized to the requested peek width. Mirrors the production
// listenerfilter.peekerConn — kept separate here so the test does not depend
// on the parent package's unexported NewPeekerConnSize allocations.
type peekConn struct {
	net.Conn
	br *bufio.Reader
}

func newPeekConn(c net.Conn, size int) *peekConn {
	return &peekConn{Conn: c, br: bufio.NewReaderSize(c, size)}
}

func (p *peekConn) Peek(n int) ([]byte, error) { return p.br.Peek(n) }
func (p *peekConn) Read(b []byte) (int, error) { return p.br.Read(b) }

// feedBytesAsPeeker writes b onto the cli end of a net.Pipe and closes cli
// once written; returns a peekConn over the srv end. Closing cli causes
// bufio.Reader.Peek to terminate with io.EOF after returning the buffered
// bytes, matching the production discipline where Peek(n) returns whatever
// is available when the underlying conn signals EOF.
func feedBytesAsPeeker(t *testing.T, b []byte, peekSize int) (cleanup func(), peeker *peekConn) {
	t.Helper()
	cli, srv := net.Pipe()
	go func() {
		_, _ = cli.Write(b)
		_ = cli.Close()
	}()
	cleanup = func() { _ = srv.Close() }
	return cleanup, newPeekConn(srv, peekSize)
}

func TestInspectWithClientHelloPopulatesInputs(t *testing.T) {
	chBytes := captureClientHelloBytes(t, "foo.example.test", []string{"h2", "http/1.1"})
	cleanup, peeker := feedBytesAsPeeker(t, chBytes, 4096)
	defer cleanup()
	f := &filter{cfg: &config{bufferSize: 4096}}
	defer f.OnDestroy()
	inputs := &listenerfilter.ChainMatchInputs{}
	status, err := f.Inspect(context.Background(), peeker, inputs)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if status != listenerfilter.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if inputs.TransportProtocol != "tls" {
		t.Errorf("TransportProtocol: got %q, want \"tls\"", inputs.TransportProtocol)
	}
	if inputs.ServerName != "foo.example.test" {
		t.Errorf("ServerName: got %q, want \"foo.example.test\"", inputs.ServerName)
	}
	if len(inputs.ApplicationProtocols) != 2 || inputs.ApplicationProtocols[0] != "h2" || inputs.ApplicationProtocols[1] != "http/1.1" {
		t.Errorf("ApplicationProtocols: got %v, want [h2 http/1.1]", inputs.ApplicationProtocols)
	}
}

func TestInspectWithNonTLSPreambleSetsRawBuffer(t *testing.T) {
	cleanup, peeker := feedBytesAsPeeker(t, []byte("GET / HTTP/1.1\r\nHost: example.test\r\n\r\n"), 4096)
	defer cleanup()
	f := &filter{cfg: &config{bufferSize: 4096}}
	defer f.OnDestroy()
	inputs := &listenerfilter.ChainMatchInputs{}
	status, err := f.Inspect(context.Background(), peeker, inputs)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if status != listenerfilter.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if inputs.TransportProtocol != "raw_buffer" {
		t.Errorf("TransportProtocol: got %q, want \"raw_buffer\"", inputs.TransportProtocol)
	}
	if inputs.ServerName != "" {
		t.Errorf("ServerName: got %q, want empty", inputs.ServerName)
	}
	if len(inputs.ApplicationProtocols) != 0 {
		t.Errorf("ApplicationProtocols: got %v, want empty", inputs.ApplicationProtocols)
	}
}

func TestInspectWithEmptyConnectionDoesNotPanic(t *testing.T) {
	cli, srv := net.Pipe()
	_ = cli.Close()
	_ = srv.Close()
	peeker := newPeekConn(srv, 4096)
	f := &filter{cfg: &config{bufferSize: 4096}}
	defer f.OnDestroy()
	inputs := &listenerfilter.ChainMatchInputs{}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Inspect panicked on empty connection: %v", r)
		}
	}()
	status, err := f.Inspect(context.Background(), peeker, inputs)
	if err != nil {
		t.Errorf("Inspect on closed pipe: got err %v, want nil (non-fatal)", err)
	}
	if status != listenerfilter.Continue {
		t.Errorf("status on empty conn: got %v, want Continue", status)
	}
}

func TestNewRoundtripsThroughRegistry(t *testing.T) {
	r := listenerfilter.NewListenerFilterRegistry()
	r.Register(TypeURL, New)
	r.Freeze()
	factoryFn, ok := r.Lookup(TypeURL)
	if !ok {
		t.Fatalf("Lookup(%q): ok=false", TypeURL)
	}
	pb := &tls_inspectorv3.TlsInspector{
		InitialReadBufferSize: wrapperspb.UInt32(2048),
	}
	tc, err := anypb.New(pb)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	instFactory, err := factoryFn(tc, listenerfilter.FactoryCtx{})
	if err != nil {
		t.Fatalf("factory: %v", err)
	}
	inst := instFactory()
	if _, isFilter := inst.(*filter); !isFilter {
		t.Errorf("instance: got %T, want *filter", inst)
	}
	f := inst.(*filter)
	if f.cfg.bufferSize != 2048 {
		t.Errorf("cfg.bufferSize: got %d, want 2048", f.cfg.bufferSize)
	}
}

func TestInspectConcurrentIndependentConnections(t *testing.T) {
	const N = 10
	chBytes := captureClientHelloBytes(t, "foo.example.test", []string{"h2"})
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func(i int) {
			defer wg.Done()
			cleanup, peeker := feedBytesAsPeeker(t, chBytes, 4096)
			defer cleanup()
			f := &filter{cfg: &config{bufferSize: 4096}}
			defer f.OnDestroy()
			inputs := &listenerfilter.ChainMatchInputs{}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			status, err := f.Inspect(ctx, peeker, inputs)
			if err != nil {
				t.Errorf("goroutine %d Inspect: %v", i, err)
				return
			}
			if status != listenerfilter.Continue {
				t.Errorf("goroutine %d status: got %v, want Continue", i, status)
			}
			if inputs.TransportProtocol != "tls" {
				t.Errorf("goroutine %d TransportProtocol: got %q, want \"tls\"", i, inputs.TransportProtocol)
			}
			if inputs.ServerName != "foo.example.test" {
				t.Errorf("goroutine %d ServerName: got %q", i, inputs.ServerName)
			}
		}(i)
	}
	wg.Wait()
}

func TestOnDestroyIsNoOp(t *testing.T) {
	f := &filter{cfg: &config{bufferSize: 4096}}
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("OnDestroy panicked: %v", r)
		}
	}()
	f.OnDestroy()
	f.OnDestroy()
	f.OnDestroy()
}
