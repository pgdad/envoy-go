package extauthzgrpc

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"

	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Server is a minimal in-process scriptable Authorization/Check gRPC server
// per SPEC §7.2 + planner-time decision D1. Goroutine-safe: the per-:path
// `scripts` map is guarded by an RWMutex so concurrent Check / Script calls
// are race-free under the -race detector (see TestServer_ConcurrentClient_NoRace).
//
// Lifecycle:
//   - New(t) binds 127.0.0.1:0 (ephemeral port) + spawns grpcSrv.Serve(lis) in
//     a goroutine + registers t.Cleanup(Stop). The caller reads Addr() to
//     templatize the cluster c_authz_grpc port into the fixture yaml configs.
//   - NewAtAddr(addr) binds the caller-supplied `host:port` (a free port
//     pre-allocated via Listen+Close so bootstrap YAMLs can templatize
//     before the gRPC server starts) + spawns grpcSrv.Serve(lis) in a
//     goroutine. No t.Cleanup is registered — the caller manages lifecycle
//     via Server.Stop. Used by the fixture-0021 differential driver.
//   - Script(path, resp) registers a per-:path *authv3.CheckResponse. Calls
//     to Script after Check are permitted (RWMutex serializes the writes
//     against in-flight reads).
//   - Check(ctx, req) implements authv3.AuthorizationServer: looks up the
//     scripted response by req.Attributes.Request.Http.Path; returns it; on
//     no-match returns status.Errorf(codes.Unavailable, ...) which the
//     ext_authz filter maps to dispError per SPEC §5.P10 / §6.7.
//   - Stop() GracefulStops the *grpc.Server; idempotent via sync.Once.
//
// Plaintext h2c — no TLS — per SPEC §7.2: grpc.NewServer() with no Creds()
// option, listener is plain net.Listen("tcp", ...). The fixture's
// c_authz_grpc cluster uses http2_protocol_options{} without an upstream
// TLS context.
type Server struct {
	authv3.UnimplementedAuthorizationServer

	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server

	mu      sync.RWMutex
	scripts map[string]*authv3.CheckResponse

	stopOnce sync.Once
}

// New binds an ephemeral 127.0.0.1 listener and starts an
// authv3.AuthorizationServer-registered *grpc.Server in a background
// goroutine. The cleanup is registered via t.Cleanup so test functions do
// NOT need to explicitly call Stop (though they may — Stop is idempotent).
//
// New uses testing.TB so it composes equally with *testing.T, *testing.B,
// and *testing.F (the fuzzer harness). For callers that need a
// caller-chosen listener address (notably the fixture-0021 differential
// driver — bootstrap YAMLs bake in a stable auth-cluster endpoint port that
// MUST be allocated before the gRPC server starts), see NewAtAddr.
func New(t testing.TB) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("extauthzgrpc: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// NewAtAddr binds a 127.0.0.1 listener on the caller-supplied `host:port`
// (e.g., "127.0.0.1:<preAllocatedPort>") and starts an
// authv3.AuthorizationServer-registered *grpc.Server in a background
// goroutine. Returns the server + nil on success, or nil + a wrapped
// net.Listen / setup error.
//
// Used by differential fixtures whose envoy.yaml / envoy-go.yaml bootstraps
// templatize a baked-in c_authz_grpc cluster endpoint that MUST match the
// auth-server's bound address — the driver allocates a free port via
// Listen+Close BEFORE the bootstraps render, then NewAtAddr re-binds the
// gRPC server on that exact port. Mirrors the 18.1 extauthzhttp.New(ctx,
// addr, script) idiom (caller-chosen-port via address argument).
//
// Lifecycle management is the CALLER's responsibility — there is no
// t.Cleanup registration (the caller may not be a *testing.T-rooted
// context; fixture drivers stop the server explicitly via Server.Stop).
// Tests that want auto-cleanup should use New(t) instead.
func NewAtAddr(addr string) (*Server, error) {
	return newServer(addr)
}

// newServer is the shared constructor body used by both New(t) and
// NewAtAddr(addr). It binds a TCP listener on `addr` (which may be
// "127.0.0.1:0" for an ephemeral port), constructs the *Server, registers
// the authv3.AuthorizationServer impl, and spawns grpcSrv.Serve(lis) in a
// background goroutine. Returns (nil, error) on net.Listen failure.
func newServer(addr string) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	s := &Server{
		addr:    lis.Addr().String(),
		lis:     lis,
		grpcSrv: grpc.NewServer(),
		scripts: map[string]*authv3.CheckResponse{},
	}
	authv3.RegisterAuthorizationServer(s.grpcSrv, s)

	go func() {
		// grpc.Server.Serve returns nil on GracefulStop. Any earlier error
		// (e.g., listener close from a panic path) is discarded — the caller
		// observes lifecycle via Stop's idempotent registration.
		_ = s.grpcSrv.Serve(lis)
	}()

	return s, nil
}

// Addr returns the listener's bound `host:port` string. Load-bearing when
// New allocated an ephemeral port — the fixture driver reads this back to
// templatize the cluster c_authz_grpc endpoint into envoy.yaml + envoy-go.yaml.
func (s *Server) Addr() string {
	return s.addr
}

// Script registers a scripted *authv3.CheckResponse for the supplied `:path`
// discriminator key per planner-time decision D1. Multiple Script calls
// against the same path overwrite the previous registration.
//
// The `:path` discriminator is the value of req.Attributes.Request.Http.Path
// at Check time. Tests + fixture drivers seed the script map per-scenario;
// the gRPC server returns a hand-shaped *authv3.CheckResponse per scenario.
func (s *Server) Script(path string, resp *authv3.CheckResponse) {
	s.mu.Lock()
	s.scripts[path] = resp
	s.mu.Unlock()
}

// Check implements authv3.AuthorizationServer. It looks up the per-:path
// scripted *authv3.CheckResponse and returns it; an unregistered path
// returns codes.Unavailable so the filter under test maps the response to
// dispError per SPEC §5.P10 / §6.7 (the dispError class triggers the
// failure_mode_allow disposition + error-counter increment).
//
// The :path discriminator is sourced from req.Attributes.Request.Http.Path
// (the value the ext_authz filter writes per SPEC §6.6). An empty Path
// (synthetic test stream that bypassed DecodeHeaders) lands the empty-string
// key in the scripts map — drivers that need an "empty path" script can
// register one explicitly via Script("", ...).
func (s *Server) Check(_ context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error) {
	path := req.GetAttributes().GetRequest().GetHttp().GetPath()

	s.mu.RLock()
	resp, ok := s.scripts[path]
	s.mu.RUnlock()

	if !ok {
		return nil, status.Errorf(codes.Unavailable, "extauthzgrpc: no script registered for path %q", path)
	}
	return resp, nil
}

// Stop GracefulStops the *grpc.Server. Idempotent via sync.Once: multiple
// calls are no-ops after the first. Registered as t.Cleanup at New time, so
// callers typically do NOT need to invoke Stop explicitly — the test
// runtime drives it at test end. Tests that need to observe the
// listener-closed shape (e.g. TestServer_Stop_Closes) MAY call Stop early.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		s.grpcSrv.GracefulStop()
	})
}
