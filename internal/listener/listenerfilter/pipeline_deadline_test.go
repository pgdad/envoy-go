package listenerfilter

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// peekOnlyFilter mimics tls_inspector's discipline without importing it (this
// package cannot): it Peeks n bytes, IGNORES the error, never consults ctx and
// always returns Continue, nil. Only the pipeline can bound it.
type peekOnlyFilter struct{ n int }

func (f *peekOnlyFilter) Inspect(_ context.Context, p Peeker, _ *ChainMatchInputs) (ListenerFilterStatus, error) {
	_, _ = p.Peek(f.n)
	return Continue, nil
}
func (f *peekOnlyFilter) OnDestroy() {}

// loopbackTCPPair returns a connected (peer, server) pair over REAL loopback
// TCP — never net.Pipe — so the peek reads a kernel socket.
func loopbackTCPPair(t *testing.T) (peer, server net.Conn) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	type res struct {
		c   net.Conn
		err error
	}
	ch := make(chan res, 1)
	go func() {
		c, aerr := ln.Accept()
		ch <- res{c, aerr}
	}()
	peer, err = net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	r := <-ch
	if r.err != nil {
		t.Fatalf("accept: %v", r.err)
	}
	return peer, r.c
}

type runResult struct {
	err     error
	elapsed time.Duration
}

// runPeekPipeline starts Pipeline.Run over a real peekerConn wrapping server
// with one peekOnlyFilter{5}; the result arrives on the returned channel.
func runPeekPipeline(server net.Conn, timeoutMs uint32) (net.Conn, <-chan runResult) {
	pc := NewPeekerConn(server)
	ch := make(chan runResult, 1)
	go func() {
		start := time.Now()
		var p Pipeline
		err := p.Run(context.Background(), []ListenerFilter{&peekOnlyFilter{n: 5}}, AsPeeker(pc), &ChainMatchInputs{}, timeoutMs)
		ch <- runResult{err, time.Since(start)}
	}()
	return pc, ch
}

// readAllBounded reads exactly n bytes from c WITHOUT setting a read
// deadline of its own (that would overwrite a stale one), bounded by a timer.
func readAllBounded(t *testing.T, c net.Conn, n int, bound time.Duration) ([]byte, error) {
	t.Helper()
	type res struct {
		b   []byte
		err error
	}
	ch := make(chan res, 1)
	go func() {
		b := make([]byte, n)
		_, err := io.ReadFull(c, b)
		ch <- res{b, err}
	}()
	select {
	case r := <-ch:
		return r.b, r.err
	case <-time.After(bound):
		_ = c.Close()
		r := <-ch
		return r.b, errors.New("read did not complete within the bound")
	}
}

// TestPipelineRunDeadlineInterruptsSilentPeek: a NON-ctx-aware filter blocked
// in Peek on a silent peer must be interrupted by the pipeline at timeoutMs,
// and Run must report a timeout (an error wrapping context.DeadlineExceeded).
// Bounded: if Run has not returned by 3 s, the peer is closed (which unblocks
// the peek) and the arm FAILS rather than hangs.
func TestPipelineRunDeadlineInterruptsSilentPeek(t *testing.T) {
	peer, server := loopbackTCPPair(t)
	defer func() { _ = peer.Close() }()
	pc, ch := runPeekPipeline(server, 200)
	defer func() { _ = pc.Close() }()
	var r runResult
	select {
	case r = <-ch:
	case <-time.After(3 * time.Second):
		_ = peer.Close()
		r = <-ch
		t.Errorf("deadline enforced: Run did not return within 3s of a 200 ms timeout on a silent peer (it returned only after the peer was closed, at %v)", r.elapsed)
	}
	if !errors.Is(r.err, context.DeadlineExceeded) {
		t.Errorf("timeout classification: Run err = %v, want an error wrapping context.DeadlineExceeded", r.err)
	}
	if r.elapsed < 150*time.Millisecond || r.elapsed > time.Second {
		t.Errorf("deadline window: Run returned after %v, want within [150ms, 1s] of a 200 ms timeout", r.elapsed)
	}
}

// TestPipelineRunDeadlineClearedOnSuccess: a peer that sends in time lets Run
// return nil, and no read deadline survives Run: a later Read of further peer
// bytes, well past the 200 ms budget, succeeds.
func TestPipelineRunDeadlineClearedOnSuccess(t *testing.T) {
	peer, server := loopbackTCPPair(t)
	defer func() { _ = peer.Close() }()
	pc, ch := runPeekPipeline(server, 200)
	defer func() { _ = pc.Close() }()
	start := time.Now()
	time.Sleep(20 * time.Millisecond)
	if _, err := peer.Write([]byte("hello")); err != nil {
		t.Fatalf("peer write: %v", err)
	}
	select {
	case r := <-ch:
		if r.err != nil {
			t.Errorf("success path: Run err = %v after the peer sent 5 bytes at 20 ms, want nil", r.err)
		}
	case <-time.After(3 * time.Second):
		_ = peer.Close()
		<-ch
		t.Fatalf("success path: Run did not return within 3s although the peer sent 5 bytes at 20 ms")
	}
	time.Sleep(time.Until(start.Add(400 * time.Millisecond)))
	if _, err := peer.Write([]byte("more")); err != nil {
		t.Fatalf("peer write (after the budget): %v", err)
	}
	got, err := readAllBounded(t, pc, 9, 3*time.Second)
	if err != nil || string(got) != "hellomore" {
		t.Errorf("no stale deadline (success path): Read after the 200 ms budget got %q err=%v, want \"hellomore\"", got, err)
	}
}

// TestPipelineRunDeadlineClearedAfterTimeout: after the pipeline's deadline
// FIRED (Run returned a timeout), the socket must be usable again — a
// fall-through connection (continue_on_listener_filters_timeout=true) is
// read by the chain. A Read of bytes the peer sends afterwards succeeds.
// Bounded: if Run has not returned by 1 s, the peer's send unblocks it and
// the arm FAILS.
func TestPipelineRunDeadlineClearedAfterTimeout(t *testing.T) {
	peer, server := loopbackTCPPair(t)
	defer func() { _ = peer.Close() }()
	pc, ch := runPeekPipeline(server, 200)
	defer func() { _ = pc.Close() }()
	select {
	case r := <-ch:
		if !errors.Is(r.err, context.DeadlineExceeded) {
			t.Errorf("timeout classification: Run err = %v, want an error wrapping context.DeadlineExceeded", r.err)
		}
		if _, err := peer.Write([]byte("hello")); err != nil {
			t.Fatalf("peer write: %v", err)
		}
	case <-time.After(time.Second):
		if _, err := peer.Write([]byte("hello")); err != nil {
			t.Fatalf("peer write: %v", err)
		}
		r := <-ch
		t.Errorf("deadline enforced: Run did not return within 1s of a 200 ms timeout on a silent peer (returned after the peer sent, at %v)", r.elapsed)
	}
	got, err := readAllBounded(t, pc, 5, 3*time.Second)
	if err != nil || string(got) != "hello" {
		t.Errorf("no stale deadline (timeout path): Read after the fired deadline got %q err=%v, want \"hello\"", got, err)
	}
}

// TestPipelineRunZeroTimeoutHoldsSilentPeek: timeoutMs 0 establishes no
// deadline, so a silent peer holds Run past 400 ms (2x a 200 ms budget); Run
// returns nil only once the peer closes.
func TestPipelineRunZeroTimeoutHoldsSilentPeek(t *testing.T) {
	peer, server := loopbackTCPPair(t)
	pc, ch := runPeekPipeline(server, 0)
	defer func() { _ = pc.Close() }()
	select {
	case r := <-ch:
		t.Errorf("zero disables: Run returned after %v (err=%v) with timeoutMs 0 and a silent peer, want still blocked at 400 ms", r.elapsed, r.err)
		_ = peer.Close()
		return
	case <-time.After(400 * time.Millisecond):
	}
	_ = peer.Close()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Errorf("zero disables: Run err = %v after the peer closed, want nil (no deadline, a non-ctx-aware filter)", r.err)
		}
	case <-time.After(3 * time.Second):
		t.Errorf("zero disables: Run did not return within 3s of the peer closing")
	}
}
