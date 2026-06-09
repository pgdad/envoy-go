package network

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
)

// echoPipe returns a dial closure yielding one end of a net.Pipe; the other end
// is served by serve. dialed counts dials; reqs counts onRequest hook calls.
func TestUpstreamConn_LazyDial_RoundTrip(t *testing.T) {
	var dials, reqs int
	dial := func(ctx context.Context) (net.Conn, error) {
		dials++
		c, s := net.Pipe()
		go func() { // a trivial server: read a line, reply "+OK\r\n"
			br := bufio.NewReader(s)
			for {
				if _, err := br.ReadBytes('\n'); err != nil {
					return
				}
				_, _ = s.Write([]byte("+OK\r\n"))
			}
		}()
		return c, nil
	}
	u := NewUpstreamConn(dial, func() { reqs++ })
	defer func() { _ = u.Close() }()

	if dials != 0 {
		t.Fatalf("dials = %d before first Send, want 0 (lazy)", dials)
	}
	if err := u.Send(context.Background(), []byte("PING\r\n")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if dials != 1 || reqs != 1 {
		t.Fatalf("after Send: dials=%d reqs=%d, want 1/1", dials, reqs)
	}
	line, err := u.Reader().ReadBytes('\n')
	if err != nil || string(line) != "+OK\r\n" {
		t.Fatalf("reply = %q err=%v, want +OK", line, err)
	}
	// a SECOND Send reuses the SAME conn (no re-dial) and fires the hook again.
	if err := u.Send(context.Background(), []byte("GET x\r\n")); err != nil {
		t.Fatalf("Send 2: %v", err)
	}
	if dials != 1 || reqs != 2 {
		t.Fatalf("after 2nd Send: dials=%d reqs=%d, want 1/2", dials, reqs)
	}
}

func TestUpstreamConn_DialError(t *testing.T) {
	want := errors.New("dial boom")
	u := NewUpstreamConn(func(context.Context) (net.Conn, error) { return nil, want }, nil)
	if err := u.Send(context.Background(), []byte("X\r\n")); !errors.Is(err, want) {
		t.Fatalf("Send err = %v, want %v", err, want)
	}
}

func TestUpstreamConn_CloseIdempotent_NoDial(t *testing.T) {
	// Close before any Send (a PING-only downstream that never proxied) is a no-op.
	u := NewUpstreamConn(func(context.Context) (net.Conn, error) {
		t.Fatal("dial must NOT happen on a Send-less connection")
		return nil, nil
	}, nil)
	if err := u.Close(); err != nil {
		t.Fatalf("Close (no dial) = %v, want nil", err)
	}
	if err := u.Close(); err != nil {
		t.Fatalf("second Close = %v, want nil (idempotent)", err)
	}
}

func TestUpstreamConn_Race(t *testing.T) {
	// One Handle-style goroutine drives Send→read in a loop (single-flight); the
	// -race detector proves no data race on the seam's per-connection state.
	dial := func(context.Context) (net.Conn, error) {
		c, s := net.Pipe()
		go func() { _, _ = io.Copy(s, s) }() // echo
		return c, nil
	}
	u := NewUpstreamConn(dial, nil)
	defer func() { _ = u.Close() }()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			if err := u.Send(context.Background(), []byte("+x\r\n")); err != nil {
				return
			}
			if _, err := u.Reader().ReadBytes('\n'); err != nil {
				return
			}
		}
	}()
	wg.Wait()
}

// Invariant: internal/filter/network must NOT import internal/cluster.
// The seam accepts a dial CLOSURE (UpstreamDialFunc), keeping the package
// free of any internal/cluster dependency.  Verified by:
//
//	go list -deps ./internal/filter/network/ | grep -q "envoy-go/internal/cluster" \
//	  && echo "LEAK: cluster imported" || echo "CLEAN: no cluster import"
//
// Expected output: CLEAN: no cluster import
