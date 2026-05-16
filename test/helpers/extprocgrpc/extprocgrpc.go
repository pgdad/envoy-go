package extprocgrpc

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// corev3HeaderMap is a local type alias for *corev3.HeaderMap — kept here
// so the lookupPath signature reads cleanly in the package.
type corev3HeaderMap = corev3.HeaderMap

// Server is a minimal in-process scriptable ExternalProcessor.Process bidi-stream
// gRPC server per SPEC §7.4 + planner-time decision D1. Goroutine-safe: the
// per-discriminator scripts + received maps are guarded by an RWMutex so
// concurrent Process / Script / Received calls are race-free under the
// -race detector (see TestServer_ConcurrentClient_NoRace).
//
// Lifecycle:
//   - New(t) binds 127.0.0.1:0 (ephemeral port) + spawns grpcSrv.Serve(lis)
//     in a goroutine + registers t.Cleanup(Stop). The caller reads Addr() to
//     templatize the cluster c_ext_proc port into the fixture yaml configs.
//   - NewAtAddr(addr) binds the caller-supplied `host:port` (a free port
//     pre-allocated via Listen+Close so bootstrap YAMLs can templatize
//     before the gRPC server starts) + spawns grpcSrv.Serve(lis) in a
//     goroutine. No t.Cleanup is registered — the caller manages lifecycle
//     via Server.Stop. Used by the fixture-0022 differential driver.
//   - Script(discriminator, responses...) registers an ORDERED SEQUENCE of
//     *extprocv3.ProcessingResponse for the discriminator (typically the
//     :path of the first ProcessingRequest's request_headers). Per-stage
//     dispatch advances a per-discriminator counter; each successive Recv
//     from the stream triggers Send of the next response in the sequence.
//     The discriminator is captured from the FIRST ProcessingRequest's
//     request_headers `:path` header per planner-time D1.
//   - Process(stream) implements extprocv3.ExternalProcessorServer: Recv-loop
//     accepts per-stage *ProcessingRequest; first Recv extracts :path
//     discriminator; subsequent Recvs advance script counter; per stage Send
//     next ProcessingResponse from the per-discriminator sequence; if
//     ImmediateResponse arm in script → Send + return (closes stream); if
//     script exhausted before stream end → status.Errorf(codes.Internal,
//     "no script remaining for discriminator %q") which the filter under
//     test maps to streamsFailed + dispError per parent §5.P11; if Recv
//     returns io.EOF (client CloseSend) → return nil cleanly.
//   - Received(discriminator) returns the per-discriminator slice of
//     *ProcessingRequest values RECEIVED across all stream lifetimes (drivers
//     use this for post-run content-assertion against the SPEC §6.6
//     hypothesis-mapping table — e.g. attribute envelope content).
//   - Stop() GracefulStops the *grpc.Server; idempotent via sync.Once.
//
// Plaintext h2c — no TLS — per SPEC §7.2 + parent SPEC §8 item 17:
// grpc.NewServer() with no Creds() option; listener is plain net.Listen("tcp", ...).
// The fixture's c_ext_proc cluster uses http2_protocol_options{} without an
// upstream TLS context.
type Server struct {
	extprocv3.UnimplementedExternalProcessorServer

	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server

	mu       sync.RWMutex
	scripts  map[string][]*extprocv3.ProcessingResponse
	received map[string][]*extprocv3.ProcessingRequest

	stopOnce sync.Once
}

// New binds an ephemeral 127.0.0.1 listener and starts an
// extprocv3.ExternalProcessorServer-registered *grpc.Server in a background
// goroutine. The cleanup is registered via t.Cleanup so test functions do
// NOT need to explicitly call Stop (though they may — Stop is idempotent).
//
// New uses testing.TB so it composes equally with *testing.T, *testing.B,
// and *testing.F (the fuzzer harness). For callers that need a
// caller-chosen listener address (notably the fixture-0022 differential
// driver — bootstrap YAMLs bake in a stable processor-cluster endpoint port
// that MUST be allocated before the gRPC server starts), see NewAtAddr.
func New(t testing.TB) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0")
	if err != nil {
		t.Fatalf("extprocgrpc: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// NewAtAddr binds a TCP listener on the caller-supplied `host:port` (e.g.,
// "127.0.0.1:<preAllocatedPort>") and starts an
// extprocv3.ExternalProcessorServer-registered *grpc.Server in a background
// goroutine. Returns the server + nil on success, or nil + a wrapped
// net.Listen / setup error.
//
// Used by differential fixtures whose envoy.yaml / envoy-go.yaml bootstraps
// templatize a baked-in c_ext_proc cluster endpoint that MUST match the
// processor server's bound address — the driver allocates a free port via
// Listen+Close BEFORE the bootstraps render, then NewAtAddr re-binds the
// gRPC server on that exact port. Mirrors phase-18.2 extauthzgrpc.NewAtAddr.
//
// Lifecycle management is the CALLER's responsibility — there is no
// t.Cleanup registration. Tests that want auto-cleanup should use New(t)
// instead.
func NewAtAddr(addr string) (*Server, error) {
	return newServer(addr)
}

// newServer is the shared constructor body used by both New(t) and
// NewAtAddr(addr). It binds a TCP listener on `addr` (which may be
// "127.0.0.1:0" for an ephemeral port), constructs the *Server, registers
// the extprocv3.ExternalProcessorServer impl, and spawns grpcSrv.Serve(lis)
// in a background goroutine. Returns (nil, error) on net.Listen failure.
func newServer(addr string) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}

	s := &Server{
		addr:     lis.Addr().String(),
		lis:      lis,
		grpcSrv:  grpc.NewServer(),
		scripts:  map[string][]*extprocv3.ProcessingResponse{},
		received: map[string][]*extprocv3.ProcessingRequest{},
	}
	extprocv3.RegisterExternalProcessorServer(s.grpcSrv, s)

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
// templatize the cluster c_ext_proc endpoint into envoy.yaml + envoy-go.yaml.
func (s *Server) Addr() string {
	return s.addr
}

// Script registers an ordered sequence of *extprocv3.ProcessingResponse
// values for the supplied discriminator (the :path of the first
// ProcessingRequest's request_headers per planner-time D1). The sequence
// is REPLACED on a second Script call against the same discriminator. The
// internal per-discriminator counter resets to 0 — drivers that re-script
// mid-run get a fresh sequence.
//
// Send order matches Recv order: the Nth ProcessingResponse in the slice
// is sent in response to the Nth ProcessingRequest received on the stream
// (after the discriminator is captured from the first request).
func (s *Server) Script(discriminator string, responses ...*extprocv3.ProcessingResponse) {
	s.mu.Lock()
	s.scripts[discriminator] = responses
	s.mu.Unlock()
}

// Received returns a COPY of all *extprocv3.ProcessingRequest values
// received by the server for the supplied discriminator across all stream
// lifetimes. Used by drivers for post-run content-assertion (per SPEC §6.6
// hypothesis-mapping table — e.g., the attribute envelope content on the
// request_headers and response_headers stages).
//
// Returns nil when the discriminator has no recorded requests.
func (s *Server) Received(discriminator string) []*extprocv3.ProcessingRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.received[discriminator]
	if len(src) == 0 {
		return nil
	}
	out := make([]*extprocv3.ProcessingRequest, len(src))
	copy(out, src)
	return out
}

// Process implements extprocv3.ExternalProcessorServer. Per-stream lifecycle:
//
//  1. Recv first ProcessingRequest from the client. The discriminator is
//     extracted from the request_headers stage's :path header — drivers
//     register per-:path script sequences via Script(path, ...).
//  2. Lookup the per-discriminator script sequence. If unregistered,
//     return status.Errorf(codes.Internal, "no script registered for
//     discriminator %q") — the filter under test maps Internal to
//     streamsFailed + dispError per parent §5.P11.
//  3. Send the indexed ProcessingResponse from the sequence.
//  4. If the response carries an ImmediateResponse arm, return nil after
//     Send (closes the stream from server side; the filter detects EOF on
//     its next Recv and applies the immediate-response disposition).
//  5. Loop to step 6.
//  6. Recv next ProcessingRequest. On io.EOF (client CloseSend, e.g.,
//     OnDestroy or stream completion), return nil cleanly. On other Recv
//     errors return them verbatim (filter logs + maps to streamsFailed).
//  7. Advance per-discriminator counter; if exhausted, return
//     status.Errorf(codes.Internal, "no script remaining ...").
//  8. Send next ProcessingResponse; loop back to step 4 (Immediate arm
//     check).
//
// The :path extraction tolerates the absent / empty case — discriminator
// "" lands as a regular script key (drivers that need an empty-path script
// can register one explicitly).
func (s *Server) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	// Recv first request to capture the discriminator.
	firstReq, err := stream.Recv()
	if err != nil {
		if err == io.EOF {
			return nil
		}
		return err
	}
	discriminator := extractPathDiscriminator(firstReq)

	s.recordReceived(discriminator, firstReq)

	idx := 0
	for {
		// Lookup per-discriminator script sequence + bounds check.
		s.mu.RLock()
		sequence := s.scripts[discriminator]
		s.mu.RUnlock()

		if idx >= len(sequence) {
			return status.Errorf(codes.Internal,
				"extprocgrpc: no script remaining for discriminator %q (requested index %d of %d)",
				discriminator, idx, len(sequence))
		}

		resp := sequence[idx]
		idx++

		if err := stream.Send(resp); err != nil {
			return err
		}

		// ImmediateResponse arm → close stream after Send. The filter
		// under test detects the EOF on its next Recv and applies the
		// immediate-response disposition per SPEC §4.
		if resp.GetImmediateResponse() != nil {
			return nil
		}

		// Recv next per-stage request.
		req, err := stream.Recv()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		s.recordReceived(discriminator, req)
	}
}

// recordReceived appends req to the per-discriminator received slice for
// post-run driver assertion. Race-clean via the same mutex protecting
// scripts.
func (s *Server) recordReceived(discriminator string, req *extprocv3.ProcessingRequest) {
	s.mu.Lock()
	s.received[discriminator] = append(s.received[discriminator], req)
	s.mu.Unlock()
}

// extractPathDiscriminator returns the value of the `:path` pseudo-header
// from the first ProcessingRequest. Checks BOTH the request_headers stage
// and the response_headers stage arms (the latter for per-route
// request_header_mode:SKIP cases where the first ProcessingRequest is
// a response_headers stage — see Task 13 fixture 0022 scenario 8).
// Returns "" when neither arm carries a :path header — drivers register
// scripts under the EMPTY discriminator key for those cases.
func extractPathDiscriminator(req *extprocv3.ProcessingRequest) string {
	if req == nil {
		return ""
	}
	if hh := req.GetRequestHeaders(); hh != nil {
		if path := lookupPath(hh.GetHeaders()); path != "" {
			return path
		}
	}
	if hh := req.GetResponseHeaders(); hh != nil {
		if path := lookupPath(hh.GetHeaders()); path != "" {
			return path
		}
	}
	return ""
}

// lookupPath walks a HeaderMap for the `:path` pseudo-header value.
func lookupPath(hm *corev3HeaderMap) string {
	if hm == nil {
		return ""
	}
	for _, h := range hm.GetHeaders() {
		if h.GetKey() == ":path" {
			if rv := h.GetRawValue(); len(rv) > 0 {
				return string(rv)
			}
			return h.GetValue()
		}
	}
	return ""
}

// Stop GracefulStops the *grpc.Server. Idempotent via sync.Once: multiple
// calls are no-ops after the first. Registered as t.Cleanup at New time, so
// callers typically do NOT need to invoke Stop explicitly — the test
// runtime drives it at test end. Drivers that need to observe the
// listener-closed shape (e.g., scenario 5 failure_mode_allow with processor
// down) MAY call Stop early.
func (s *Server) Stop() {
	s.stopOnce.Do(func() {
		s.grpcSrv.GracefulStop()
	})
}
