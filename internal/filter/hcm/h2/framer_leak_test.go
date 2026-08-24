package h2

import (
	"bytes"
	"context"
	"log"
	"net"
	"runtime"
	"testing"
	"time"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/hpack"
)

// runOneLeakIteration opens ONE ServerConn over a real loopback socket, drives a
// real request/response over it, tears the whole thing down, and does not return
// until Run has returned.
//
// ⚠️ It deliberately does NOT use conn_test.go's startServerConn. That helper
// registers its listener and its client conn with t.Cleanup, so across 40
// iterations 80 t.Cleanup closures ACCUMULATE and every listener and client conn
// stays open until the test function ends — which is precisely the condition a
// goroutine-leak measurement must not have. This helper owns and closes its own
// resources per iteration.
func runOneLeakIteration(t *testing.T, disp Dispatcher, settings ServerSettings, burstPings int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	runDone := make(chan error, 1)
	go func() {
		serverConn, aerr := ln.Accept()
		if aerr != nil {
			runDone <- aerr
			return
		}
		sc := NewServerConn(ctx, serverConn, disp, settings)
		runDone <- sc.Run()
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		_ = ln.Close()
		t.Fatalf("dial: %v", err)
	}

	// The listener has done its job; close it now rather than at test end so a
	// leaked accept goroutine cannot be mistaken for a leaked reader goroutine.
	_ = ln.Close()

	_ = clientConn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := writeClientPreface(clientConn); err != nil {
		t.Fatalf("preface: %v", err)
	}
	fr := http2.NewFramer(clientConn, clientConn)
	if err := fr.WriteSettings(); err != nil {
		t.Fatalf("settings: %v", err)
	}
	serverAcked, weAcked := false, false
	for !serverAcked || !weAcked {
		f, rerr := fr.ReadFrame()
		if rerr != nil {
			t.Fatalf("handshake: %v", rerr)
		}
		if f.Header().Type == http2.FrameSettings {
			sf := f.(*http2.SettingsFrame)
			if !sf.IsAck() {
				_ = fr.WriteSettingsAck()
				weAcked = true
			} else {
				serverAcked = true
			}
		}
	}

	// A REAL request/response: HEADERS(END_STREAM) out, HEADERS+DATA back. This
	// is what makes the iteration exercise the dispatch path (and its `go fn()`)
	// rather than only the handshake.
	var encBuf bytes.Buffer
	enc := hpack.NewEncoder(&encBuf)
	_ = enc.WriteField(hpack.HeaderField{Name: ":method", Value: "GET"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":path", Value: "/leak"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":scheme", Value: "http"})
	_ = enc.WriteField(hpack.HeaderField{Name: ":authority", Value: "test.local"})
	if err := fr.WriteHeaders(http2.HeadersFrameParam{
		StreamID:      1,
		BlockFragment: encBuf.Bytes(),
		EndHeaders:    true,
		EndStream:     true,
	}); err != nil {
		t.Fatalf("write headers: %v", err)
	}
	for done := false; !done; {
		f, rerr := fr.ReadFrame()
		if rerr != nil {
			t.Fatalf("read response: %v", rerr)
		}
		if df, ok := f.(*http2.DataFrame); ok && df.StreamEnded() {
			done = true
		}
	}

	// Park the reader goroutine OFF the socket before tearing down.
	//
	// This is the load-bearing half of the iteration and it is not decoration.
	// closeReader exists because the reader can be parked in three places and no
	// single mechanism reaches all three (see framer.go): parked in read(2) it is
	// reachable by the deadline stamp AND, incidentally, by conn.Close(); parked
	// SENDING on frameCh or WAITING FOR RELEASE it is reachable ONLY by
	// close(stopCh). A teardown that leaves the reader in read(2) therefore does
	// not test closeReader at all — the outer `defer s.conn.Close()` would clean
	// it up on its own and the guard would stay green with closeReader deleted.
	//
	// Flooding PINGs achieves the needed state deterministically: each one obliges
	// the server to write a PING ACK to a client that has stopped reading, so the
	// socket send buffer fills, Run blocks in that write while HOLDING a frame,
	// and the reader is left waiting for a release token that never comes.
	pingPayload := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
	_ = clientConn.SetWriteDeadline(time.Now().Add(2 * time.Second))
	for i := 0; i < burstPings; i++ {
		if err := fr.WritePing(false, pingPayload); err != nil {
			break // send buffer full in BOTH directions; the state we wanted
		}
	}

	_ = clientConn.Close()
	select {
	case <-runDone:
	case <-time.After(15 * time.Second):
		t.Fatal("ServerConn.Run did not return after the client conn was closed")
	}
	cancel()
}

// TestFramer_ReaderGoroutineDoesNotLeak is the phase-91 goroutine-leak guard for
// the reader goroutine startReader spawns. It mirrors the precedent
// TestH2PoolWatcherEvictRaceNoLeak (internal/cluster/h2pool_test.go): a baseline
// captured BEFORE any connection, a fixed number of full open/close cycles, and a
// POLLED return-to-baseline rather than a single sample.
//
// goleak is deliberately NOT used: it is not a dependency of this module and
// adding it would break this row's +0 go.mod modules envelope. runtime.NumGoroutine
// is what the precedent uses too.
//
// ⚠️ WHY THE SLACK IS 2 AND NOT THE PRECEDENT'S 8. The slack IS the guard;
// pick it too large and the guard passes with the fix deleted. It was DERIVED BY
// MEASUREMENT at this tip, not copied. Over 40 connections, 3 runs each:
//
//   - WITH closeReader intact: delta 0 immediately after the loop and 0 after the
//     poll, every run. The noise floor here is 0, not 5.
//   - WITH (*framer).closeReader gutted to a bare `return`: delta 40 immediately
//     and STILL 40 after a 2 s poll, every run. One permanently parked reader per
//     connection, each blocked waiting for a release token that only close(stopCh)
//     delivers -- so it is a permanent leak, not a transient one a poll erodes.
//
// The discriminating window is therefore 0 vs 40, and 2 is simply the smallest
// slack that still absorbs a transient dispatch goroutine. It is set from the
// NOISE FLOOR, deliberately not from the leak size: a slack of 8 would still have
// caught this particular 40-goroutine leak, but it would silently mask the
// 5-over-40 leak this guard was commissioned against.
//
// ⚠️ THE PING FLOOD IN THE ITERATION HELPER IS LOAD-BEARING AND WAS ALSO
// MEASURED. Re-running the SAME ablation with burstPings = 0 -- i.e. a plain
// request/response followed by a client close -- gives delta 0 on every run. With
// the reader parked in read(2), Run's outer `defer s.conn.Close()` unblocks it on
// its own and closeReader has nothing left to do, so a guard built that way is
// GREEN WITH THE FIX DELETED. The flood is what parks the reader in the one state
// only close(stopCh) can reach.
func TestFramer_ReaderGoroutineDoesNotLeak(t *testing.T) {
	const (
		iters = 40
		slack = 2
	)

	disp := &fixedDispatcher{action: &fixedAction{status: 200, body: "leak-probe-body"}}
	settings := DefaultServerSettings

	baseGoroutines := runtime.NumGoroutine()
	for i := 0; i < iters; i++ {
		runOneLeakIteration(t, disp, settings, 4096)
	}

	// POLL, never a single sample: the dispatch goroutines conn.go launches with
	// `go fn()` in flushPendingDispatch outlive Run's return, so an immediate
	// sample measures scheduler latency rather than a leak.
	pollUntil(t, func() bool { return runtime.NumGoroutine() <= baseGoroutines+slack },
		"the framer reader goroutine must not survive ServerConn.Run: after "+
			"40 connections the goroutine count must return to baseline")

	if testing.Verbose() {
		log.Printf("leak guard: base=%d final=%d delta=%d (slack %d over %d connections)",
			baseGoroutines, runtime.NumGoroutine(), runtime.NumGoroutine()-baseGoroutines, slack, iters)
	}
}
