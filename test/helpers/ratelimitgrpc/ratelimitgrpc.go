package ratelimitgrpc

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	commonratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/common/ratelimit/v3"
	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/grpc"
)

// Server is a minimal in-process scriptable RateLimitService gRPC server
// per planner-time decision D-RL5 / AMEND-6 (proto-number-faithful response
// encoding). Goroutine-safe: the per-key `scripts` map is guarded by an
// RWMutex so concurrent ShouldRateLimit / Script calls are race-free under
// the -race detector.
//
// Lifecycle:
//   - New(t) binds 127.0.0.1:0 (ephemeral port) + spawns grpcSrv.Serve(lis)
//     in a goroutine + registers t.Cleanup(Stop). The caller reads Addr()
//     to templatize the rate_limit_service cluster port into the fixture
//     yaml configs.
//   - NewAtAddr(addr) binds the caller-supplied "host:port" (a free port
//     pre-allocated via Listen+Close so bootstrap YAMLs can templatize
//     before the gRPC server starts) + spawns grpcSrv.Serve(lis) in a
//     goroutine. No t.Cleanup is registered — the caller manages lifecycle
//     via Server.Stop. Used by the phase-24.1 fixture-0032 driver.
//   - Script(key, resp) registers a per-key *ratelimitv3.RateLimitResponse.
//     `key` is the canonical descriptor-list string returned by
//     CanonicalKey. Calls to Script after ShouldRateLimit are permitted
//     (RWMutex serializes the writes against in-flight reads).
//   - ShouldRateLimit(ctx, req) implements
//     ratelimitv3.RateLimitServiceServer: computes the canonical key from
//     req.Descriptors, looks up the scripted response, returns it; on
//     no-match returns a default OK response (OverallCode=OK + per-
//     descriptor OK statuses) so unscripted scenarios pass through.
//   - Stop() GracefulStops the *grpc.Server; idempotent via sync.Once.
//
// Plaintext h2c — no TLS — per parent SPEC §7.2: grpc.NewServer() with no
// Creds() option, listener is plain net.Listen("tcp", ...). The fixture's
// rate_limit_service cluster uses http2_protocol_options{} without an
// upstream TLS context.
//
// D-RL5 / AMEND-6 — proto-number-faithful encoding: Scripts that aim to
// drive cross-side byte-exact OVER_LIMIT replies MUST construct the
// *RateLimitResponse via Go-protobuf struct literals setting ONLY the
// fields they intend to emit. Unset optionals (raw_body, dynamic_metadata,
// quota, per-descriptor current_limit / limit_remaining /
// duration_until_reset / quota) are omitted from the wire by Go-protobuf's
// default elision of zero-value scalars + nil-pointer messages — this is
// what gives both the reference Envoy + envoy-go byte-equivalent local
// replies. The fake itself does NOT mutate the scripted response; it
// returns it verbatim.
type Server struct {
	ratelimitv3.UnimplementedRateLimitServiceServer

	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server

	mu      sync.RWMutex
	scripts map[string]*ratelimitv3.RateLimitResponse

	stopOnce sync.Once
}

// New binds an ephemeral 127.0.0.1 listener and starts a
// ratelimitv3.RateLimitServiceServer-registered *grpc.Server in a
// background goroutine. The cleanup is registered via t.Cleanup so test
// functions do NOT need to explicitly call Stop (though they may — Stop
// is idempotent).
//
// New uses testing.TB so it composes equally with *testing.T, *testing.B,
// and *testing.F (the fuzzer harness). For callers that need a
// caller-chosen listener address (notably the fixture-0032 differential
// driver — bootstrap YAMLs bake in a stable rls-cluster endpoint port
// that MUST be allocated before the gRPC server starts), see NewAtAddr.
func New(t testing.TB) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("ratelimitgrpc: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// NewAtAddr binds a 127.0.0.1 listener on the caller-supplied "host:port"
// (e.g., "127.0.0.1:<preAllocatedPort>") and starts a
// ratelimitv3.RateLimitServiceServer-registered *grpc.Server in a
// background goroutine. Returns the server + nil on success, or nil + a
// wrapped net.Listen / setup error.
//
// Used by differential fixtures whose envoy.yaml / envoy-go.yaml
// bootstraps templatize a baked-in rate_limit_service cluster endpoint
// that MUST match the rls-server's bound address — the driver allocates
// a free port via Listen+Close BEFORE the bootstraps render, then
// NewAtAddr re-binds the gRPC server on that exact port. Mirrors the
// fixture-0021 extauthzgrpc.NewAtAddr idiom.
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
// the ratelimitv3.RateLimitServiceServer impl, and spawns
// grpcSrv.Serve(lis) in a background goroutine. Returns (nil, error) on
// net.Listen failure.
func newServer(addr string) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	s := &Server{
		addr:    lis.Addr().String(),
		lis:     lis,
		grpcSrv: grpc.NewServer(),
		scripts: map[string]*ratelimitv3.RateLimitResponse{},
	}
	ratelimitv3.RegisterRateLimitServiceServer(s.grpcSrv, s)

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
// templatize the rate_limit_service cluster endpoint into envoy.yaml +
// envoy-go.yaml.
func (s *Server) Addr() string {
	return s.addr
}

// Script registers a scripted *ratelimitv3.RateLimitResponse for the
// supplied canonical descriptor-list key. Multiple Script calls against
// the same key overwrite the previous registration.
//
// The key is the value returned by CanonicalKey for the matching
// *ratelimitv3.RateLimitRequest. Tests + fixture drivers seed the script
// map per-scenario; the gRPC server returns the hand-shaped
// *ratelimitv3.RateLimitResponse per scenario.
//
// CRITICAL — D-RL5 / AMEND-6: the *RateLimitResponse passed here MUST be
// constructed by setting ONLY the fields the scenario intends to emit on
// the wire. Unset optional fields (raw_body, dynamic_metadata, quota,
// per-descriptor current_limit / limit_remaining / duration_until_reset
// / quota) are elided by Go-protobuf. Cross-side byte-exact OVER_LIMIT
// replies require this discipline at the call site.
func (s *Server) Script(key string, resp *ratelimitv3.RateLimitResponse) {
	s.mu.Lock()
	s.scripts[key] = resp
	s.mu.Unlock()
}

// ShouldRateLimit implements ratelimitv3.RateLimitServiceServer. It
// computes the canonical key from req.Descriptors via CanonicalKey,
// looks up the per-key scripted response, and returns it.
//
// On no-match (no Script call registered the key) the fake returns a
// default OK response — OverallCode=OK plus a per-descriptor OK status
// slice matching req.Descriptors' length. Unscripted scenarios therefore
// pass through cleanly without dispError attribution at the filter under
// test. Drivers that want a deterministic OVER_LIMIT or error MUST
// Script the exact key before sending the request.
//
// Per AMEND-6 the default OK response sets ONLY OverallCode + Statuses
// (each Statuses[i] sets only Code). All other optional fields stay
// zero-value / nil so Go-protobuf omits them from the wire.
func (s *Server) ShouldRateLimit(_ context.Context, req *ratelimitv3.RateLimitRequest) (*ratelimitv3.RateLimitResponse, error) {
	key := CanonicalKey(req)

	s.mu.RLock()
	resp, ok := s.scripts[key]
	s.mu.RUnlock()

	if ok {
		return resp, nil
	}

	// Default OK — per-descriptor statuses match the request descriptor
	// count so the response shape is well-formed. AMEND-6: only Code is
	// set on each DescriptorStatus; current_limit / limit_remaining /
	// duration_until_reset / quota stay zero-value / nil and are omitted.
	statuses := make([]*ratelimitv3.RateLimitResponse_DescriptorStatus, len(req.GetDescriptors()))
	for i := range statuses {
		statuses[i] = &ratelimitv3.RateLimitResponse_DescriptorStatus{
			Code: ratelimitv3.RateLimitResponse_OK,
		}
	}
	return &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OK,
		Statuses:    statuses,
	}, nil
}

// Stop GracefulStops the *grpc.Server. Idempotent via sync.Once: multiple
// calls are no-ops after the first. Registered as t.Cleanup at New time,
// so callers typically do NOT need to invoke Stop explicitly — the test
// runtime drives it at test end.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		s.grpcSrv.GracefulStop()
	})
}

// CanonicalKey returns the deterministic key the fake derives from a
// *ratelimitv3.RateLimitRequest for Script lookup. Drivers MUST use this
// helper to build the lookup key when seeding the Script map so the key
// the driver writes matches the key the fake reads at ShouldRateLimit
// time.
//
// Format (planner-time decision D-RL5 anchor):
//
//	domain "|" descriptor[0] "|" descriptor[1] ...
//
// where each descriptor is its entries joined as "key=value;key=value;...",
// preserving the descriptor's original entry order (the descriptor engine
// at internal/filter/http/ratelimit/descriptors.go appends entries in
// action-list order per §4 + §4.5; the fake key reflects that order
// verbatim).
//
// The format is INTENTIONALLY simple — no escaping of '=' / ';' / '|' in
// keys or values. Scenarios that need to discriminate descriptors whose
// entries embed those delimiters should use a different action-list shape
// or extend CanonicalKey at that point. For phase 24.1 the 5 CORE actions
// (generic_key / request_headers / remote_address / destination_cluster /
// header_value_match) produce entries whose keys + values are
// well-behaved alphanumeric / dotted-quad / cluster-name strings, so
// no escaping is needed at this scope.
func CanonicalKey(req *ratelimitv3.RateLimitRequest) string {
	if req == nil {
		return ""
	}
	descriptors := req.GetDescriptors()
	parts := make([]string, 0, 1+len(descriptors))
	parts = append(parts, req.GetDomain())
	for _, d := range descriptors {
		parts = append(parts, canonicalDescriptor(d))
	}
	return strings.Join(parts, "|")
}

// canonicalDescriptor renders one RateLimitDescriptor as its entries
// joined "key=value;key=value;..." in the order the descriptor presents
// them. A nil descriptor or a descriptor with no entries renders as the
// empty string — a degenerate-but-well-defined key.
func canonicalDescriptor(d *commonratelimitv3.RateLimitDescriptor) string {
	if d == nil {
		return ""
	}
	entries := d.GetEntries()
	if len(entries) == 0 {
		return ""
	}
	segs := make([]string, len(entries))
	for i, e := range entries {
		segs[i] = e.GetKey() + "=" + e.GetValue()
	}
	return strings.Join(segs, ";")
}
