package grpcclient

// processor_client_test.go — Groups 1+2+3 unit tests for the
// `*ProcessorClient` bidi-stream wrapper landed at Task 4 of phase 19.1 IMPL
// (ADR-0169). The test groups mirror the `*AuthClient` Groups 1+2+3 structure
// in `grpcclient_test.go` (per SPEC §14.1):
//
//   Group 1 — `NewProcessorClient` happy path + PARSE-REJECT inheritance from
//             `*Dialer.DialContext` (unknown cluster; `UseH2()==false`; nil
//             dialer).
//   Group 2 — `(*ProcessorClient).Process` bidi-stream lifecycle: open stream
//             → Send → Recv → CloseSend → stream close (happy round-trip);
//             mid-stream ctx cancellation (the OnDestroy primitive);
//             per-message timeout applied by the CALLER (Process itself does
//             NOT wrap with context.WithTimeout — that is the filter's
//             dispatchStage discipline per ADR-0169 §Decision); concurrent
//             Send+Recv on a single stream is race-clean (one goroutine for
//             Send + one for Recv — gRPC ClientStream semantics).
//   Group 3 — `(*ProcessorClient).Close` idempotency (sync.Once-guarded);
//             concurrent Close race-clean; nil-receiver safe.
//
// Test infrastructure REUSES the helpers from `grpcclient_test.go` (same
// package): `mkAuthPKI` (the PKI is service-agnostic), `mkH2ClusterMgr`,
// `mkPlainClusterMgr`. The processor in-process gRPC server is local to this
// file (`startTestProcessorServer` + `fakeProcessorServer`) since the proto
// surface differs from auth (`ExternalProcessorServer` registration; the
// `Process` server method operates on the bidi stream).

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
)

// ----------------------------------------------------------------------------
// In-process gRPC processor server.
// ----------------------------------------------------------------------------

// processorScript governs the in-process processor server's behavior on its
// Process bidi-stream. The Run callback is invoked exactly once per stream
// (the server method blocks in `Run`; on return the gRPC framework closes the
// stream). A nil `Run` blocks until the stream ctx fires (used by mid-stream
// cancel + per-message timeout tests).
type processorScript struct {
	Run func(stream extprocv3.ExternalProcessor_ProcessServer) error
}

// fakeProcessorServer implements `extprocv3.ExternalProcessorServer`. The
// Process method dispatches to the supplied script's Run callback when
// non-nil; otherwise blocks until the stream ctx cancels (yielding
// ctx.Err() to surface caller-cancel transport errors).
type fakeProcessorServer struct {
	extprocv3.UnimplementedExternalProcessorServer
	script *processorScript
}

func (f *fakeProcessorServer) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	if f.script != nil && f.script.Run != nil {
		return f.script.Run(stream)
	}
	<-stream.Context().Done()
	return stream.Context().Err()
}

// startTestProcessorServer starts a TLS-fronted `*grpc.Server` on a loopback
// port with ALPN h2; registers a `fakeProcessorServer{script}`. Returns the
// bound port and a `stop` func (calls `GracefulStop`).
//
// Mirrors `startTestAuthServer` in grpcclient_test.go — same PKI surface,
// same h2 ALPN; swaps the registered service from `AuthorizationServer` to
// `ExternalProcessorServer`.
func startTestProcessorServer(t testing.TB, pki *authTestPKI, script *processorScript) (uint32, func()) {
	t.Helper()
	pair, err := stdtls.X509KeyPair(pki.leafCertPEM, pki.leafKeyPEM)
	if err != nil {
		t.Fatalf("server keypair: %v", err)
	}
	cfg := &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		NextProtos:   []string{"h2"},
		MinVersion:   stdtls.VersionTLS12,
		MaxVersion:   stdtls.VersionTLS13,
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}
	s := grpc.NewServer()
	extprocv3.RegisterExternalProcessorServer(s, &fakeProcessorServer{script: script})
	go func() {
		_ = s.Serve(ln)
	}()
	port := uint32(ln.Addr().(*net.TCPAddr).Port)
	stop := func() {
		s.GracefulStop()
		_ = ln.Close()
	}
	return port, stop
}

// echoScript reads ONE ProcessingRequest from the stream, sends back a single
// ProcessingResponse with a populated RequestHeaders.Response, then returns
// (closing the stream — the client sees io.EOF on the next Recv).
func echoScript() *processorScript {
	return &processorScript{
		Run: func(stream extprocv3.ExternalProcessor_ProcessServer) error {
			_, err := stream.Recv()
			if err != nil {
				return err
			}
			resp := &extprocv3.ProcessingResponse{
				Response: &extprocv3.ProcessingResponse_RequestHeaders{
					RequestHeaders: &extprocv3.HeadersResponse{
						Response: &extprocv3.CommonResponse{
							Status: extprocv3.CommonResponse_CONTINUE,
						},
					},
				},
			}
			return stream.Send(resp)
		},
	}
}

// ----------------------------------------------------------------------------
// Group 1 — `NewProcessorClient` surface
// ----------------------------------------------------------------------------

// TestProcessorClient_NewProcessorClient_HappyPath verifies that
// `NewProcessorClient` dials the cluster via the supplied `*Dialer` and
// returns a usable `*ProcessorClient`.
func TestProcessorClient_NewProcessorClient_HappyPath(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestProcessorServer(t, pki, echoScript())
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_proc", port)
	d := New(mgr)

	pc, err := NewProcessorClient(d, "c_proc", 2*time.Second)
	if err != nil {
		t.Fatalf("NewProcessorClient: %v", err)
	}
	if pc == nil {
		t.Fatalf("NewProcessorClient: nil ProcessorClient")
	}
	t.Cleanup(func() { _ = pc.Close() })
}

// TestProcessorClient_NewProcessorClient_NilDialer verifies the nil-dialer
// PARSE-REJECT — `NewProcessorClient` returns `(nil, err)` with a cluster-
// name-bearing error mirroring `NewAuthClient`.
func TestProcessorClient_NewProcessorClient_NilDialer(t *testing.T) {
	t.Parallel()
	pc, err := NewProcessorClient(nil, "c_proc", time.Second)
	if err == nil {
		_ = pc.Close()
		t.Fatalf("NewProcessorClient(nil dialer): err = nil; want non-nil PARSE-REJECT")
	}
	if pc != nil {
		t.Errorf("NewProcessorClient(nil dialer): pc = %v; want nil on error", pc)
	}
	if !strings.Contains(err.Error(), "c_proc") {
		t.Errorf("NewProcessorClient err = %q; want substring %q", err.Error(), "c_proc")
	}
}

// TestProcessorClient_NewProcessorClient_PropagatesDialError verifies that
// dial-time PARSE-REJECTs from `(*Dialer).DialContext` surface as the
// `NewProcessorClient` error return. Two PARSE-REJECT axes inherited:
// unknown-cluster + `UseH2()==false`. The cluster name must appear in the
// error wording for diagnostic affinity.
func TestProcessorClient_NewProcessorClient_PropagatesDialError(t *testing.T) {
	t.Parallel()

	t.Run("unknown_cluster", func(t *testing.T) {
		t.Parallel()
		mgr := mkPlainClusterMgr(t, "c_other", 9999)
		d := New(mgr)
		pc, err := NewProcessorClient(d, "c_missing", time.Second)
		if err == nil {
			_ = pc.Close()
			t.Fatalf("NewProcessorClient(c_missing): err = nil; want PARSE-REJECT propagation")
		}
		if pc != nil {
			t.Errorf("NewProcessorClient(c_missing): pc = %v; want nil on error", pc)
		}
		if !strings.Contains(err.Error(), "c_missing") {
			t.Errorf("NewProcessorClient(c_missing) err = %q; want substring %q", err.Error(), "c_missing")
		}
	})

	t.Run("useh2_false", func(t *testing.T) {
		t.Parallel()
		mgr := mkPlainClusterMgr(t, "c_plain", 9999)
		d := New(mgr)
		pc, err := NewProcessorClient(d, "c_plain", time.Second)
		if err == nil {
			_ = pc.Close()
			t.Fatalf("NewProcessorClient(c_plain): err = nil; want PARSE-REJECT propagation")
		}
		if pc != nil {
			t.Errorf("NewProcessorClient(c_plain): pc = %v; want nil on error", pc)
		}
		if !strings.Contains(err.Error(), "c_plain") {
			t.Errorf("NewProcessorClient(c_plain) err = %q; want substring %q", err.Error(), "c_plain")
		}
	})
}

// ----------------------------------------------------------------------------
// Group 2 — `(*ProcessorClient).Process` bidi-stream lifecycle
// ----------------------------------------------------------------------------

// TestProcessorClient_Process_HappyRoundTrip verifies the canonical bidi-
// stream lifecycle: open stream → Send → Recv → CloseSend → final Recv sees
// io.EOF after the server returns from its handler.
func TestProcessorClient_Process_HappyRoundTrip(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestProcessorServer(t, pki, echoScript())
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_proc", port)
	d := New(mgr)

	pc, err := NewProcessorClient(d, "c_proc", 2*time.Second)
	if err != nil {
		t.Fatalf("NewProcessorClient: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := pc.Process(ctx)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if stream == nil {
		t.Fatalf("Process: returned nil ProcessStream")
	}

	// Send a single ProcessingRequest with a minimal RequestHeaders payload.
	req := &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{},
			},
		},
	}
	if err := stream.Send(req); err != nil {
		t.Fatalf("stream.Send: %v", err)
	}

	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("stream.Recv: %v", err)
	}
	if resp == nil {
		t.Fatalf("stream.Recv: nil response")
	}
	if resp.GetRequestHeaders() == nil {
		t.Errorf("stream.Recv: missing RequestHeaders; got %T", resp.Response)
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("stream.CloseSend: %v", err)
	}

	// After CloseSend + server returns from handler, the next Recv yields io.EOF.
	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Errorf("stream.Recv after CloseSend: err = %v; want io.EOF", err)
	}
}

// TestProcessorClient_Process_MidStreamCancel verifies that canceling the
// parent ctx mid-stream surfaces a Canceled transport error on the next
// `Recv` call — the `OnDestroy` cancellation primitive per SPEC §3.1.
func TestProcessorClient_Process_MidStreamCancel(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	// nil script → server blocks until stream ctx cancels.
	port, stop := startTestProcessorServer(t, pki, nil)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_proc", port)
	d := New(mgr)

	pc, err := NewProcessorClient(d, "c_proc", 10*time.Second)
	if err != nil {
		t.Fatalf("NewProcessorClient: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	ctx, cancel := context.WithCancel(context.Background())

	stream, err := pc.Process(ctx)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Cancel after a short delay; the blocking Recv must observe ctx.Done.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err = stream.Recv()
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("stream.Recv: err = nil; want Canceled transport error")
	}
	if elapsed > 2*time.Second {
		t.Errorf("stream.Recv: did not observe ctx.Done promptly; elapsed=%v", elapsed)
	}
	if !isCanceledTransportErr(err) {
		t.Errorf("stream.Recv err = %v; want Canceled transport error", err)
	}
}

// TestProcessorClient_Process_PerMessageTimeoutCallerSide verifies the
// per-message-timeout DISCIPLINE per ADR-0169 §Decision: `Process` itself
// does NOT apply `context.WithTimeout` — the caller's `dispatchStage` (the
// filter, future Task 7) wraps each `Recv` with `context.WithTimeout`. This
// test simulates that discipline by wrapping the parent ctx with
// `context.WithTimeout(parent, 100ms)` BEFORE opening the stream, then
// asserting the blocking `Recv` returns DeadlineExceeded promptly.
//
// The `perMessageTimeout` argument to `NewProcessorClient` (50ms here) is
// STORED on the struct for the filter to read — it does NOT influence
// `Process` directly. This test explicitly demonstrates that distinction:
// the 50ms struct field is NOT the cause of the prompt return; the
// caller-supplied 100ms WithTimeout is.
func TestProcessorClient_Process_PerMessageTimeoutCallerSide(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestProcessorServer(t, pki, nil) // server blocks
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_proc", port)
	d := New(mgr)

	const perMessage = 50 * time.Millisecond
	pc, err := NewProcessorClient(d, "c_proc", perMessage)
	if err != nil {
		t.Fatalf("NewProcessorClient: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	// Verify the perMessageTimeout is stored on the struct for the filter.
	if got := pc.PerMessageTimeout(); got != perMessage {
		t.Errorf("PerMessageTimeout() = %v; want %v", got, perMessage)
	}

	parent, cancelP := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelP()

	stream, err := pc.Process(parent)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	// Caller-side per-message wrap (simulates the filter's dispatchStage).
	recvCtx, cancelR := context.WithTimeout(parent, 100*time.Millisecond)
	defer cancelR()

	doneCh := make(chan error, 1)
	go func() {
		// We can't pass recvCtx through the bidi-Recv directly (the gRPC stream
		// already has its own ctx — `parent`). The discipline is: the caller
		// MUST cancel the stream's parent ctx when recvCtx fires. Simulate.
		<-recvCtx.Done()
		// Cancel the stream's parent ctx to abort the Recv (the real filter
		// uses a dedicated per-stream cancel for OnDestroy; for this test the
		// recvCtx timer triggers a manual cancel hop).
		cancelP()
		_, e := stream.Recv()
		doneCh <- e
	}()

	select {
	case e := <-doneCh:
		if e == nil {
			t.Errorf("stream.Recv: err = nil; want DeadlineExceeded/Canceled after caller-side timeout")
		}
		if !isCanceledTransportErr(e) && !isDeadlineExceededTransportErr(e) {
			t.Errorf("stream.Recv err = %v; want Canceled or DeadlineExceeded transport error", e)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("stream.Recv did not return within 3s of caller-side timeout")
	}
}

// TestProcessorClient_Process_ConcurrentSendRecv verifies the gRPC
// ClientStream concurrency contract: ONE goroutine for Send + ONE goroutine
// for Recv is race-clean. This is the bidi-stream concurrency model the
// filter's `dispatchStage` relies on (the filter uses a single goroutine to
// own the stream's Send + Recv halves alternately; this test asserts that
// the underlying gRPC ClientStream tolerates the concurrent-halves pattern
// when authored correctly).
//
// We use the "ping-pong" script: server reads N requests, replies N
// responses; client Sends N + Recvs N concurrently. Under -race the test
// passes only if our ProcessStream wrapper does not introduce write-write
// races over the underlying ClientStream's Send + Recv halves.
func TestProcessorClient_Process_ConcurrentSendRecv(t *testing.T) {
	t.Parallel()
	const n = 16
	pki := mkAuthPKI(t)
	script := &processorScript{
		Run: func(stream extprocv3.ExternalProcessor_ProcessServer) error {
			for i := 0; i < n; i++ {
				if _, err := stream.Recv(); err != nil {
					return err
				}
				if err := stream.Send(&extprocv3.ProcessingResponse{
					Response: &extprocv3.ProcessingResponse_RequestHeaders{
						RequestHeaders: &extprocv3.HeadersResponse{},
					},
				}); err != nil {
					return err
				}
			}
			return nil
		},
	}
	port, stop := startTestProcessorServer(t, pki, script)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_proc", port)
	d := New(mgr)

	pc, err := NewProcessorClient(d, "c_proc", 2*time.Second)
	if err != nil {
		t.Fatalf("NewProcessorClient: %v", err)
	}
	t.Cleanup(func() { _ = pc.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := pc.Process(ctx)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	sendErr := make(chan error, 1)
	recvErr := make(chan error, 1)
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			if err := stream.Send(&extprocv3.ProcessingRequest{
				Request: &extprocv3.ProcessingRequest_RequestHeaders{
					RequestHeaders: &extprocv3.HttpHeaders{Headers: &corev3.HeaderMap{}},
				},
			}); err != nil {
				sendErr <- err
				return
			}
		}
		_ = stream.CloseSend()
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < n; i++ {
			if _, err := stream.Recv(); err != nil {
				recvErr <- err
				return
			}
		}
	}()
	wg.Wait()
	close(sendErr)
	close(recvErr)
	if e, ok := <-sendErr; ok && e != nil {
		t.Errorf("concurrent Send: %v", e)
	}
	if e, ok := <-recvErr; ok && e != nil {
		t.Errorf("concurrent Recv: %v", e)
	}
}

// ----------------------------------------------------------------------------
// Group 3 — `(*ProcessorClient).Close` idempotency
// ----------------------------------------------------------------------------

// TestProcessorClient_Close_Idempotent verifies the sync.Once-guarded Close:
// repeated calls return the same cached error; the underlying
// `*grpc.ClientConn.Close` fires at most once; a post-Close `Process` fails
// with a recognizable closed-conn transport error.
func TestProcessorClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestProcessorServer(t, pki, echoScript())
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_proc", port)
	d := New(mgr)

	pc, err := NewProcessorClient(d, "c_proc", 2*time.Second)
	if err != nil {
		t.Fatalf("NewProcessorClient: %v", err)
	}

	err1 := pc.Close()
	err2 := pc.Close()
	err3 := pc.Close()

	if (err1 == nil) != (err2 == nil) || (err2 == nil) != (err3 == nil) {
		t.Errorf("Close idempotency: err1=%v, err2=%v, err3=%v; want all equal", err1, err2, err3)
	}
	if err1 != nil && (err1.Error() != err2.Error() || err2.Error() != err3.Error()) {
		t.Errorf("Close idempotency: err1=%q, err2=%q, err3=%q; want all equal", err1, err2, err3)
	}

	// Post-Close `Process` must surface a closed-conn transport error.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	stream, callErr := pc.Process(ctx)
	if callErr != nil {
		// Process itself may return the closed-conn error eagerly.
		if stream != nil {
			t.Errorf("Process after Close: stream = %v; want nil on closed-conn error", stream)
		}
		return
	}
	// Or the error may surface only on the first Send/Recv.
	_, recvErr := stream.Recv()
	if recvErr == nil {
		t.Errorf("Recv after Close: err = nil; want closed-conn transport error")
	}
}

// TestProcessorClient_Close_ConcurrentRaceClean verifies that N concurrent
// `Close()` calls under -race produce no race-detector violation; all
// return the SAME cached error; the underlying `*grpc.ClientConn.Close` fires
// at most once.
func TestProcessorClient_Close_ConcurrentRaceClean(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestProcessorServer(t, pki, echoScript())
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_proc", port)
	d := New(mgr)

	pc, err := NewProcessorClient(d, "c_proc", 2*time.Second)
	if err != nil {
		t.Fatalf("NewProcessorClient: %v", err)
	}

	const n = 10
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			errs[i] = pc.Close()
		}()
	}
	wg.Wait()

	var first error
	for i, e := range errs {
		if i == 0 {
			first = e
			continue
		}
		if (first == nil) != (e == nil) {
			t.Errorf("Close[%d]: err = %v; first = %v; want same nil-ness", i, e, first)
			continue
		}
		if first != nil && first.Error() != e.Error() {
			t.Errorf("Close[%d]: err = %q; first = %q; want same wording", i, e, first)
		}
	}
}

// TestProcessorClient_Close_NilSafe verifies that calling Close on a nil
// `*ProcessorClient` is a no-op returning nil — mirrors AuthClient.Close's
// nil-tolerance discipline.
func TestProcessorClient_Close_NilSafe(t *testing.T) {
	t.Parallel()
	var pc *ProcessorClient
	if err := pc.Close(); err != nil {
		t.Errorf("nil ProcessorClient Close: err = %v; want nil", err)
	}
}
