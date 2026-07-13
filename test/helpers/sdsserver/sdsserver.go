package sdsserver

import (
	"fmt"
	"io"
	"net"
	"sync"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	secretv3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
)

// Server is a minimal in-process SecretDiscoveryService gRPC server that delivers
// a configured Secret on the first StreamSecrets request and records every received
// DiscoveryRequest (goroutine-safe). Driver-owned (reference_differential_grpc_receiver_driver_owned)
// — the client DIALS it; it is NOT a runner BackendKind. Plaintext h2c (no TLS).
type Server struct {
	secretv3.UnimplementedSecretDiscoveryServiceServer

	addr    string
	lis     net.Listener
	grpcSrv *grpc.Server

	secretName string
	certPEM    []byte
	keyPEM     []byte
	silent     bool // when true, never Send a response (drives the client's initial-fetch timeout)

	mu       sync.RWMutex
	requests []*discoveryv3.DiscoveryRequest

	stopOnce sync.Once
}

// Option configures a Server at New time.
type Option func(*Server)

// WithSecret configures the delivered Secret{name, tls_certificate{inline PEM}}.
func WithSecret(name string, certPEM, keyPEM []byte) Option {
	return func(s *Server) { s.secretName = name; s.certPEM = certPEM; s.keyPEM = keyPEM }
}

// Silent makes the server accept the stream but never Send a response — used to
// drive the client's initial_fetch_timeout path (Provider test).
func Silent() Option { return func(s *Server) { s.silent = true } }

// New binds an ephemeral 127.0.0.1 listener + starts the server in a goroutine,
// registering t.Cleanup(Stop). Read Addr() to dial.
func New(t testing.TB, opts ...Option) *Server {
	t.Helper()
	s, err := newServer("127.0.0.1:0", opts...)
	if err != nil {
		t.Fatalf("sdsserver: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

// NewAtAddr binds the caller-supplied host:port (e.g. "0.0.0.0:<preAllocatedPort>"
// so a Docker reference-Envoy can dial the host) and starts the server. No
// t.Cleanup — the CALLER (a fixture driver) owns the lifecycle via Server.Stop.
// Mirrors metricsservice.NewAtAddr.
func NewAtAddr(addr string, opts ...Option) (*Server, error) {
	return newServer(addr, opts...)
}

func newServer(addr string, opts ...Option) (*Server, error) {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen %s: %w", addr, err)
	}
	s := &Server{addr: lis.Addr().String(), lis: lis, grpcSrv: grpc.NewServer()}
	for _, o := range opts {
		o(s)
	}
	secretv3.RegisterSecretDiscoveryServiceServer(s.grpcSrv, s)
	go func() { _ = s.grpcSrv.Serve(lis) }()
	return s, nil
}

// Addr returns the listener's bound `host:port` string.
func (s *Server) Addr() string { return s.addr }

// StreamSecrets records each received request and, on the first one (unless
// Silent), Sends a DiscoveryResponse carrying the configured Secret, then keeps
// draining (the client's ACK) until the stream closes.
func (s *Server) StreamSecrets(stream secretv3.SecretDiscoveryService_StreamSecretsServer) error {
	first := true
	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		s.mu.Lock()
		s.requests = append(s.requests, req)
		s.mu.Unlock()
		if first && !s.silent {
			first = false
			resp, berr := s.buildResponse(req.GetResourceNames())
			if berr != nil {
				return berr
			}
			if err := stream.Send(resp); err != nil {
				return err
			}
		}
	}
}

func (s *Server) buildResponse(names []string) (*discoveryv3.DiscoveryResponse, error) {
	sec := &tlsv3.Secret{
		Name: s.secretName,
		Type: &tlsv3.Secret_TlsCertificate{TlsCertificate: &tlsv3.TlsCertificate{
			CertificateChain: &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: s.certPEM}},
			PrivateKey:       &corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: s.keyPEM}},
		}},
	}
	any, err := anypb.New(sec)
	if err != nil {
		return nil, err
	}
	return &discoveryv3.DiscoveryResponse{
		VersionInfo: "v1",
		Nonce:       "nonce-1",
		TypeUrl:     "type.googleapis.com/" + string(sec.ProtoReflect().Descriptor().FullName()),
		Resources:   []*anypb.Any{any},
	}, nil
}

// Requests returns a snapshot copy of the received DiscoveryRequests (arrival order).
func (s *Server) Requests() []*discoveryv3.DiscoveryRequest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*discoveryv3.DiscoveryRequest, len(s.requests))
	copy(out, s.requests)
	return out
}

// Stop GracefulStops the server; idempotent via sync.Once.
func (s *Server) Stop() { s.stopOnce.Do(s.grpcSrv.GracefulStop) }

// Close is the immediate hard-stop variant of Stop (GracefulStop) for callers
// (e.g. a differential fixture driver) that want deterministic teardown
// without waiting on a still-open proxy StreamSecrets stream. Like
// metricsservice's StreamMetrics, SDS's StreamSecrets is long-lived — the
// proxy keeps the stream open indefinitely awaiting future secret rotations —
// so GracefulStop would block until the test timeout. Close calls
// grpc.Server.Stop (cancels in-flight calls, returns at once) and shares the
// same sync.Once as Stop so the two are idempotent and mutually exclusive.
func (s *Server) Close() { s.stopOnce.Do(s.grpcSrv.Stop) }
