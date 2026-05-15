package extauthzgrpc

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// dialTestClient opens a plaintext h2c grpc.ClientConn to the supplied address,
// registers it for teardown via t.Cleanup, and returns the AuthorizationClient.
func dialTestClient(t *testing.T, addr string) authv3.AuthorizationClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return authv3.NewAuthorizationClient(conn)
}

// mkCheckRequest builds a minimal *authv3.CheckRequest with the supplied
// `:path` value; used by tests to exercise the per-path script lookup.
func mkCheckRequest(path string) *authv3.CheckRequest {
	return &authv3.CheckRequest{
		Attributes: &authv3.AttributeContext{
			Request: &authv3.AttributeContext_Request{
				Http: &authv3.AttributeContext_HttpRequest{
					Path: path,
				},
			},
		},
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
// Listen+Close + rebind idiom used by the fixture-0021 differential driver).
// The test allocates a free port, closes the listener, immediately rebinds via
// NewAtAddr, and asserts Addr() returns the same `host:port`. A scripted Check
// round-trip confirms the rebound server is serving traffic.
func TestNewAtAddr_BindsToSuppliedAddress(t *testing.T) {
	// Allocate a free port via Listen+Close (the same idiom the fixture-0021
	// driver uses to pin a stable auth-cluster endpoint before bootstrap
	// YAMLs render).
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
		t.Errorf("Addr: got %q, want %q (rebound to a different port)", got, wantAddr)
	}

	// Sanity: the rebound server actually serves Check round-trips.
	srv.Script("/ping", &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: int32(codes.OK)},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{},
		},
	})
	client := dialTestClient(t, srv.Addr())
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	resp, err := client.Check(ctx, mkCheckRequest("/ping"))
	if err != nil {
		t.Fatalf("Check(/ping) on rebound server: %v", err)
	}
	if resp.GetOkResponse() == nil {
		t.Fatalf("Check(/ping): OkResponse arm = nil; want non-nil")
	}
}

// TestNewAtAddr_BindFailureReturnsError verifies that NewAtAddr returns a
// non-nil error when net.Listen fails (here: re-binding the same in-use port).
// The driver relies on this error path to surface bind failures cleanly rather
// than panicking — important when a previous run's TIME_WAIT recycle is
// pending.
func TestNewAtAddr_BindFailureReturnsError(t *testing.T) {
	// Hold a listener on a port, then attempt NewAtAddr on the same port.
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold listen: %v", err)
	}
	defer func() { _ = held.Close() }()

	srv, err := NewAtAddr(held.Addr().String())
	if err == nil {
		// Bind unexpectedly succeeded — clean up the unwanted server.
		srv.Stop()
		t.Fatalf("NewAtAddr(%q): want bind error, got nil", held.Addr().String())
	}
	if srv != nil {
		t.Errorf("NewAtAddr(%q): want nil *Server on error, got %v", held.Addr().String(), srv)
	}
}

// TestServer_Script_ReturnsScripted verifies that a registered script is
// returned at Check by `:path`, and that an unregistered path returns the
// Unavailable status.
func TestServer_Script_ReturnsScripted(t *testing.T) {
	srv := New(t)
	wantResp := &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: int32(codes.OK)},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: []*corev3.HeaderValueOption{
					{
						Header: &corev3.HeaderValue{Key: "x-auth-user", Value: "alice"},
					},
				},
			},
		},
	}
	srv.Script("/allow", wantResp)

	denyResp := &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: int32(codes.PermissionDenied)},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
				Body:   "forbidden",
			},
		},
	}
	srv.Script("/deny", denyResp)

	client := dialTestClient(t, srv.Addr())

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Allow path.
	got, err := client.Check(ctx, mkCheckRequest("/allow"))
	if err != nil {
		t.Fatalf("Check(/allow): %v", err)
	}
	if okResp := got.GetOkResponse(); okResp == nil {
		t.Fatalf("Check(/allow): OkResponse arm = nil; want non-nil")
	} else if len(okResp.GetHeaders()) != 1 || okResp.GetHeaders()[0].GetHeader().GetKey() != "x-auth-user" {
		t.Errorf("Check(/allow): headers = %+v; want one entry x-auth-user", okResp.GetHeaders())
	}

	// Deny path.
	got, err = client.Check(ctx, mkCheckRequest("/deny"))
	if err != nil {
		t.Fatalf("Check(/deny): %v", err)
	}
	if dResp := got.GetDeniedResponse(); dResp == nil {
		t.Fatalf("Check(/deny): DeniedResponse arm = nil; want non-nil")
	} else if dResp.GetBody() != "forbidden" {
		t.Errorf("Check(/deny): body = %q; want %q", dResp.GetBody(), "forbidden")
	}

	// Unregistered path: expect Unavailable.
	_, err = client.Check(ctx, mkCheckRequest("/missing"))
	if err == nil {
		t.Fatal("Check(/missing): want error; got nil")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("Check(/missing): err = %v; want gRPC status", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("Check(/missing): code = %v; want %v", st.Code(), codes.Unavailable)
	}
	if !strings.Contains(st.Message(), "/missing") {
		t.Errorf("Check(/missing): message %q does not mention the unregistered path", st.Message())
	}
}

// TestServer_Stop_Closes verifies that Stop terminates the listener and
// subsequent Check calls fail.
func TestServer_Stop_Closes(t *testing.T) {
	// Construct manually so we can call Stop early; the t.Cleanup registered
	// by New is idempotent against the explicit Stop call.
	srv := New(t)
	srv.Script("/ping", &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: int32(codes.OK)},
	})
	client := dialTestClient(t, srv.Addr())

	// Sanity: pre-Stop the server responds.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := client.Check(ctx, mkCheckRequest("/ping")); err != nil {
		t.Fatalf("pre-Stop Check: %v", err)
	}

	// Stop the server.
	srv.Stop()

	// Post-Stop Check MUST fail. The client may take a brief moment to learn
	// the connection is gone; use a fresh client with a short deadline so the
	// dial detects the closed listener.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel2()
	conn2, dialErr := grpc.NewClient(srv.Addr(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if dialErr != nil {
		// Acceptable: dial reports the closed listener at NewClient time.
		return
	}
	defer func() { _ = conn2.Close() }()
	client2 := authv3.NewAuthorizationClient(conn2)
	if _, err := client2.Check(ctx2, mkCheckRequest("/ping")); err == nil {
		t.Fatal("post-Stop Check: want error (listener closed); got nil")
	}
}

// TestServer_ConcurrentClient_NoRace verifies that concurrent Check calls
// against a single server instance do not trigger the race detector.
func TestServer_ConcurrentClient_NoRace(t *testing.T) {
	srv := New(t)
	srv.Script("/ok", &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: int32(codes.OK)},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{},
		},
	})

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	errs := make([]error, concurrency)
	for i := 0; i < concurrency; i++ {
		i := i
		go func() {
			defer wg.Done()
			client := dialTestClient(t, srv.Addr())
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			resp, err := client.Check(ctx, mkCheckRequest("/ok"))
			if err != nil {
				errs[i] = fmt.Errorf("goroutine[%d] Check: %w", i, err)
				return
			}
			if resp.GetOkResponse() == nil {
				errs[i] = fmt.Errorf("goroutine[%d]: OkResponse arm = nil", i)
			}
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Error(err)
		}
	}
}
