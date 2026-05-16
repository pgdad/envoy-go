package extprocgrpc

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// dialTestClient opens a plaintext h2c grpc.ClientConn to addr, registers it
// for teardown via t.Cleanup, and returns the ExternalProcessorClient.
func dialTestClient(t *testing.T, addr string) extprocv3.ExternalProcessorClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return extprocv3.NewExternalProcessorClient(conn)
}

// mkHeadersReq builds a minimal request_headers ProcessingRequest with the
// supplied `:path` value used as the discriminator key per planner-time D1.
func mkHeadersReq(path string) *extprocv3.ProcessingRequest {
	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_RequestHeaders{
			RequestHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{
					Headers: []*corev3.HeaderValue{
						{Key: ":path", Value: path},
					},
				},
				EndOfStream: true,
			},
		},
	}
}

// mkRespHeadersReq builds a response_headers ProcessingRequest used for the
// post-allow second-stage Recv in the bidi-stream protocol.
func mkRespHeadersReq() *extprocv3.ProcessingRequest {
	return &extprocv3.ProcessingRequest{
		Request: &extprocv3.ProcessingRequest_ResponseHeaders{
			ResponseHeaders: &extprocv3.HttpHeaders{
				Headers: &corev3.HeaderMap{
					Headers: []*corev3.HeaderValue{
						{Key: ":status", Value: "200"},
					},
				},
				EndOfStream: true,
			},
		},
	}
}

// continueHeadersResp builds a ProcessingResponse with an empty
// HeadersResponse.CommonResponse (the canonical "continue" arm for a header
// stage — no mutations, no immediate response).
func continueHeadersResp(stage string) *extprocv3.ProcessingResponse {
	hr := &extprocv3.HeadersResponse{Response: &extprocv3.CommonResponse{}}
	switch stage {
	case "request_headers":
		return &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_RequestHeaders{RequestHeaders: hr},
		}
	case "response_headers":
		return &extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_ResponseHeaders{ResponseHeaders: hr},
		}
	default:
		panic("continueHeadersResp: bad stage " + stage)
	}
}

// TestNew_StartsServerOnEphemeralPort verifies that New binds to an ephemeral
// 127.0.0.1 port and Addr() returns the bound `host:port` string.
func TestNew_StartsServerOnEphemeralPort(t *testing.T) {
	srv := New(t)
	addr := srv.Addr()
	if addr == "" {
		t.Fatal("Addr: empty after New")
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort(%q): %v", addr, err)
	}
	if host != "127.0.0.1" {
		t.Errorf("host: got %q, want %q", host, "127.0.0.1")
	}
	if port == "0" {
		t.Errorf("port: got %q, want non-zero ephemeral", port)
	}
}

// TestNewAtAddr_BindsToSuppliedAddress verifies that NewAtAddr binds the gRPC
// server to the caller-supplied address (allocated upstream via the
// Listen+Close + rebind idiom used by the fixture-0022 differential driver).
func TestNewAtAddr_BindsToSuppliedAddress(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	wantAddr := probe.Addr().String()
	if err := probe.Close(); err != nil {
		t.Fatalf("probe close: %v", err)
	}

	srv, err := NewAtAddr(wantAddr)
	if err != nil {
		t.Fatalf("NewAtAddr(%q): %v", wantAddr, err)
	}
	t.Cleanup(srv.Stop)

	if got := srv.Addr(); got != wantAddr {
		t.Errorf("Addr: got %q, want %q", got, wantAddr)
	}

	// Sanity: rebound server actually serves a Process round-trip.
	srv.Script("/ping", continueHeadersResp("request_headers"))
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream, err := client.Process(ctx)
	if err != nil {
		t.Fatalf("Process(): %v", err)
	}
	if err := stream.Send(mkHeadersReq("/ping")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv: %v", err)
	}
	if resp.GetRequestHeaders() == nil {
		t.Fatal("Recv: request_headers arm = nil")
	}
	_ = stream.CloseSend()
}

// TestNewAtAddr_BindFailureReturnsError verifies that NewAtAddr returns a
// non-nil error when net.Listen fails (here: re-binding the same in-use port).
func TestNewAtAddr_BindFailureReturnsError(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold listen: %v", err)
	}
	defer func() { _ = held.Close() }()

	srv, err := NewAtAddr(held.Addr().String())
	if err == nil {
		srv.Stop()
		t.Fatalf("NewAtAddr(%q): want bind error, got nil", held.Addr().String())
	}
	if srv != nil {
		t.Errorf("NewAtAddr(%q): want nil *Server on error, got %v", held.Addr().String(), srv)
	}
}

// TestServer_Script_ReturnsScriptedSequence verifies that a registered
// per-discriminator script returns its per-stage responses in order across
// multiple Recv steps; a second stream against the same discriminator gets
// a fresh sequence (per-stream counter is reset on stream start — drivers
// re-Script between scenarios for full reproducibility).
func TestServer_Script_ReturnsScriptedSequence(t *testing.T) {
	srv := New(t)
	srv.Script("/allow",
		continueHeadersResp("request_headers"),
		continueHeadersResp("response_headers"),
	)

	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.Process(ctx)
	if err != nil {
		t.Fatalf("Process(): %v", err)
	}

	// request_headers stage.
	if err := stream.Send(mkHeadersReq("/allow")); err != nil {
		t.Fatalf("Send request_headers: %v", err)
	}
	resp1, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv 1: %v", err)
	}
	if resp1.GetRequestHeaders() == nil {
		t.Fatalf("Recv 1: request_headers arm = nil; got %+v", resp1)
	}

	// response_headers stage.
	if err := stream.Send(mkRespHeadersReq()); err != nil {
		t.Fatalf("Send response_headers: %v", err)
	}
	resp2, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv 2: %v", err)
	}
	if resp2.GetResponseHeaders() == nil {
		t.Fatalf("Recv 2: response_headers arm = nil; got %+v", resp2)
	}

	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	// Final Recv after CloseSend should observe EOF (server returns nil).
	if _, err := stream.Recv(); err != nil && err != io.EOF {
		t.Errorf("final Recv: got %v; want nil or io.EOF", err)
	}

	// Received-tracking assertion: both per-stage requests recorded under the
	// per-:path discriminator key for post-run driver assertion.
	got := srv.Received("/allow")
	if len(got) != 2 {
		t.Errorf("Received(%q): got %d requests; want 2", "/allow", len(got))
	}
}

// TestServer_Process_ScriptExhaustedReturnsInternal verifies that a stream
// whose Recv outpaces the registered script gets a clean codes.Internal
// status (which the filter under test maps to streamsFailed + dispError per
// parent §5.P11).
func TestServer_Process_ScriptExhaustedReturnsInternal(t *testing.T) {
	srv := New(t)
	srv.Script("/exhaust", continueHeadersResp("request_headers"))

	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.Process(ctx)
	if err != nil {
		t.Fatalf("Process(): %v", err)
	}
	if err := stream.Send(mkHeadersReq("/exhaust")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	// First Recv consumes the only scripted response.
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("first Recv: %v", err)
	}
	// Send a second request that exhausts the script.
	if err := stream.Send(mkRespHeadersReq()); err != nil {
		t.Fatalf("second Send: %v", err)
	}
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("second Recv: want error (script exhausted); got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Recv error: %v; want gRPC status", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("status code: got %v; want %v", st.Code(), codes.Internal)
	}
}

// TestServer_Process_UnregisteredDiscriminatorReturnsInternal verifies that
// a stream whose first request's :path has no registered script gets a
// codes.Internal status on the FIRST Recv (the filter maps to streamsFailed
// + dispError per parent §5.P11).
func TestServer_Process_UnregisteredDiscriminatorReturnsInternal(t *testing.T) {
	srv := New(t)
	// No Script call for /missing.

	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.Process(ctx)
	if err != nil {
		t.Fatalf("Process(): %v", err)
	}
	if err := stream.Send(mkHeadersReq("/missing")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	_, err = stream.Recv()
	if err == nil {
		t.Fatal("Recv: want error (unregistered discriminator); got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Recv error: %v; want gRPC status", err)
	}
	if st.Code() != codes.Internal {
		t.Errorf("status code: got %v; want %v", st.Code(), codes.Internal)
	}
}

// TestServer_Process_BidiHalfClose verifies that client CloseSend after the
// first response cleanly terminates the server's Recv loop (returns nil) and
// the client's subsequent Recv observes io.EOF — the bidi-stream half-close
// lifecycle per the SPEC §7.4 helper API.
func TestServer_Process_BidiHalfClose(t *testing.T) {
	srv := New(t)
	srv.Script("/halfclose", continueHeadersResp("request_headers"))

	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.Process(ctx)
	if err != nil {
		t.Fatalf("Process(): %v", err)
	}
	if err := stream.Send(mkHeadersReq("/halfclose")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv 1: %v", err)
	}
	if resp.GetRequestHeaders() == nil {
		t.Fatal("Recv 1: request_headers arm = nil")
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatalf("CloseSend: %v", err)
	}
	// After CloseSend, the server's Recv returns io.EOF → Process returns nil →
	// the client's next Recv observes io.EOF.
	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Recv after CloseSend: got %v; want io.EOF", err)
	}
}

// TestServer_Process_ImmediateResponseStopsStream verifies that an
// ImmediateResponse arm in the script closes the stream immediately after
// Send — the client's subsequent Recv observes io.EOF without an additional
// Send round-trip.
func TestServer_Process_ImmediateResponseStopsStream(t *testing.T) {
	srv := New(t)
	srv.Script("/deny",
		&extprocv3.ProcessingResponse{
			Response: &extprocv3.ProcessingResponse_ImmediateResponse{
				ImmediateResponse: &extprocv3.ImmediateResponse{
					Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
					Body:   []byte("denied"),
				},
			},
		},
	)

	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.Process(ctx)
	if err != nil {
		t.Fatalf("Process(): %v", err)
	}
	if err := stream.Send(mkHeadersReq("/deny")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("Recv 1: %v", err)
	}
	if ir := resp.GetImmediateResponse(); ir == nil {
		t.Fatal("Recv 1: ImmediateResponse arm = nil")
	}
	// Server closes the stream after Send; client Recv observes io.EOF.
	_, err = stream.Recv()
	if !errors.Is(err, io.EOF) {
		t.Errorf("Recv 2: got %v; want io.EOF", err)
	}
}

// TestServer_Stop_Closes verifies that Stop terminates the listener and
// subsequent Process calls fail (either at dial time or stream-init).
func TestServer_Stop_Closes(t *testing.T) {
	srv := New(t)
	srv.Script("/ping", continueHeadersResp("request_headers"))
	client := dialTestClient(t, srv.Addr())

	// Sanity: pre-Stop the server responds.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stream, err := client.Process(ctx)
	if err != nil {
		t.Fatalf("pre-Stop Process: %v", err)
	}
	if err := stream.Send(mkHeadersReq("/ping")); err != nil {
		t.Fatalf("pre-Stop Send: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("pre-Stop Recv: %v", err)
	}
	_ = stream.CloseSend()

	// Stop.
	srv.Stop()

	// Post-Stop: a fresh stream attempt fails.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel2()
	conn2, dialErr := grpc.NewClient(srv.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if dialErr != nil {
		return // dial detects closed listener — acceptable.
	}
	defer func() { _ = conn2.Close() }()
	client2 := extprocv3.NewExternalProcessorClient(conn2)
	stream2, err := client2.Process(ctx2)
	if err != nil {
		return // stream-init detects closed listener — acceptable.
	}
	if err := stream2.Send(mkHeadersReq("/ping")); err == nil {
		if _, recvErr := stream2.Recv(); recvErr == nil {
			t.Fatal("post-Stop Recv: want error (listener closed); got nil")
		}
	}
}

// TestServer_ConcurrentClient_NoRace verifies that two concurrent Process
// streams from independent clients do not trigger the race detector on the
// shared scripts + received maps.
func TestServer_ConcurrentClient_NoRace(t *testing.T) {
	srv := New(t)
	srv.Script("/a", continueHeadersResp("request_headers"))
	srv.Script("/b", continueHeadersResp("request_headers"))

	const concurrency = 16
	var wg sync.WaitGroup
	wg.Add(concurrency)
	errs := make([]error, concurrency)
	for i := 0; i < concurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			path := "/a"
			if i%2 == 1 {
				path = "/b"
			}
			client := dialTestClient(t, srv.Addr())
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			stream, err := client.Process(ctx)
			if err != nil {
				errs[i] = err
				return
			}
			if err := stream.Send(mkHeadersReq(path)); err != nil {
				errs[i] = err
				return
			}
			if _, err := stream.Recv(); err != nil {
				errs[i] = err
				return
			}
			_ = stream.CloseSend()
		}()
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine[%d]: %v", i, err)
		}
	}
	// At least 1 received entry per discriminator (concurrency=16; half each).
	if got := len(srv.Received("/a")); got == 0 {
		t.Errorf("Received(/a): empty; want >=1")
	}
	if got := len(srv.Received("/b")); got == 0 {
		t.Errorf("Received(/b): empty; want >=1")
	}
}

// TestServer_Received_ReturnsCopy verifies that Received returns a COPY of
// the internal slice (mutations via the returned slice do not affect future
// Received() reads).
func TestServer_Received_ReturnsCopy(t *testing.T) {
	srv := New(t)
	srv.Script("/copy", continueHeadersResp("request_headers"))

	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.Process(ctx)
	if err != nil {
		t.Fatalf("Process(): %v", err)
	}
	if err := stream.Send(mkHeadersReq("/copy")); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := stream.Recv(); err != nil {
		t.Fatalf("Recv: %v", err)
	}
	_ = stream.CloseSend()

	got1 := srv.Received("/copy")
	if len(got1) != 1 {
		t.Fatalf("Received: got %d; want 1", len(got1))
	}
	// Mutate the returned slice (zero it out).
	got1[0] = nil

	got2 := srv.Received("/copy")
	if len(got2) != 1 {
		t.Fatalf("Received(repeat): got %d; want 1", len(got2))
	}
	if got2[0] == nil {
		t.Error("Received: caller mutation leaked into internal state")
	}
}
