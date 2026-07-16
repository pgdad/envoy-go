package xds

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	secretv3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/test/helpers/sdsserver"
)

// grpcTestOpener is the in-process StreamOpener test double used by every
// Provider test: it dials addr via a plain insecure grpc.NewClient (no TLS —
// the sdsserver is plaintext h2c) and adapts the typed
// SecretDiscoveryServiceClient.StreamSecrets return to Stream. The typed
// client stream (secretv3.SecretDiscoveryService_StreamSecretsClient) already
// has Send(*DiscoveryRequest) error + Recv() (*DiscoveryResponse, error), so it
// satisfies Stream structurally with no adapter needed.
type grpcTestOpener struct {
	conn *grpc.ClientConn
}

func newGRPCTestOpener(t *testing.T, addr string) *grpcTestOpener {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc.NewClient(%q): %v", addr, err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return &grpcTestOpener{conn: conn}
}

func (o *grpcTestOpener) StreamSecrets(ctx context.Context) (Stream, error) {
	return secretv3.NewSecretDiscoveryServiceClient(o.conn).StreamSecrets(ctx)
}

// closedAddr binds an ephemeral TCP listener, closes it immediately (before any
// Accept), and returns the now-unbound address — connecting to it fails fast
// with "connection refused", simulating an unreachable management server
// without relying on a real connect timeout.
func closedAddr(t *testing.T) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	addr := lis.Addr().String()
	if err := lis.Close(); err != nil {
		t.Fatalf("lis.Close: %v", err)
	}
	return addr
}

func TestProvider_FetchInitialCertificate_Success(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	srv := sdsserver.New(t, sdsserver.WithSecret("server_cert", certPEM, keyPEM))
	opener := newGRPCTestOpener(t, srv.Addr())

	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "server_cert")

	p := NewProvider(opener, Node{ID: "n", Cluster: "c"}, "", 2*time.Second, sdsStats)

	cert, err := p.FetchInitialCertificate(context.Background(), "server_cert")
	if err != nil {
		t.Fatalf("FetchInitialCertificate() unexpected error: %v", err)
	}
	if cert == nil {
		t.Fatal("FetchInitialCertificate() cert = nil, want non-nil")
	}

	if got := sdsStats.updateAttempt.Load(); got != 1 {
		t.Errorf("update_attempt = %d, want 1", got)
	}
	if got := sdsStats.updateSuccess.Load(); got != 1 {
		t.Errorf("update_success = %d, want 1", got)
	}

	reqs := srv.Requests()
	if len(reqs) < 1 {
		t.Fatalf("Requests() = %d, want >= 1", len(reqs))
	}
	if got := reqs[0].GetNode().GetId(); got != "n" {
		t.Errorf("Requests()[0].Node.Id = %q, want %q", got, "n")
	}
}

func TestProvider_FetchInitialCertificate_Timeout(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	srv := sdsserver.New(t, sdsserver.WithSecret("server_cert", certPEM, keyPEM), sdsserver.Silent())
	opener := newGRPCTestOpener(t, srv.Addr())

	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "server_cert")

	p := NewProvider(opener, Node{ID: "n", Cluster: "c"}, "", 200*time.Millisecond, sdsStats)

	cert, err := p.FetchInitialCertificate(context.Background(), "server_cert")
	if err == nil {
		t.Fatal("FetchInitialCertificate() error = nil, want non-nil (deadline)")
	}
	if cert != nil {
		t.Errorf("FetchInitialCertificate() cert = %v, want nil", cert)
	}

	if got := sdsStats.initFetchTimeout.Load(); got != 1 {
		t.Errorf("init_fetch_timeout = %d, want 1", got)
	}
}

func TestProvider_FetchInitialCertificate_MgmtDown(t *testing.T) {
	opener := newGRPCTestOpener(t, closedAddr(t))

	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "server_cert")

	p := NewProvider(opener, Node{ID: "n", Cluster: "c"}, "", 2*time.Second, sdsStats)

	cert, err := p.FetchInitialCertificate(context.Background(), "server_cert")
	if err == nil {
		t.Fatal("FetchInitialCertificate() error = nil, want non-nil (mgmt down)")
	}
	if cert != nil {
		t.Errorf("FetchInitialCertificate() cert = %v, want nil", cert)
	}

	if got := sdsStats.updateFailure.Load(); got != 1 {
		t.Errorf("update_failure = %d, want 1", got)
	}
}

func TestProvider_FetchInitialCertificate_Rejected(t *testing.T) {
	certPEM, keyPEM := selfSignedPEM(t)
	// The server is configured to deliver a Secret named "WRONG_NAME" no matter
	// what resource name the client requests — a mismatched-name reject.
	srv := sdsserver.New(t, sdsserver.WithSecret("WRONG_NAME", certPEM, keyPEM))
	opener := newGRPCTestOpener(t, srv.Addr())

	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "server_cert")

	p := NewProvider(opener, Node{ID: "n", Cluster: "c"}, "", 500*time.Millisecond, sdsStats)

	cert, err := p.FetchInitialCertificate(context.Background(), "server_cert")
	if err == nil {
		t.Fatal("FetchInitialCertificate() error = nil, want non-nil (rejected)")
	}
	if cert != nil {
		t.Errorf("FetchInitialCertificate() cert = %v, want nil", cert)
	}

	if got := sdsStats.updateRejected.Load(); got != 1 {
		t.Errorf("update_rejected = %d, want 1", got)
	}
}

// vcFakeOpener returns a Stream serving a canned validation_context response —
// keeping the FetchInitialValidationContext tests independent of the sdsserver
// extension (which does not learn WithValidationContext until a later task).
type vcFakeOpener struct {
	resp    *discoveryv3.DiscoveryResponse
	openErr error
	block   bool // when true, Recv blocks until ctx cancels (drives the timeout path)
	ctx     context.Context
}

func (o *vcFakeOpener) StreamSecrets(ctx context.Context) (Stream, error) {
	if o.openErr != nil {
		return nil, o.openErr
	}
	o.ctx = ctx
	return &vcFakeStream{o: o}, nil
}

type vcFakeStream struct{ o *vcFakeOpener }

func (s *vcFakeStream) Send(*discoveryv3.DiscoveryRequest) error { return nil }

func (s *vcFakeStream) Recv() (*discoveryv3.DiscoveryResponse, error) {
	if s.o.block {
		<-s.o.ctx.Done()
		return nil, s.o.ctx.Err()
	}
	return s.o.resp, nil
}

func TestProvider_FetchInitialValidationContext_Success(t *testing.T) {
	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "validation_ca")
	op := &vcFakeOpener{resp: &discoveryv3.DiscoveryResponse{
		VersionInfo: "v1", Nonce: "n1",
		Resources: []*anypb.Any{validVCSecretAny(t, "validation_ca")},
	}}

	p := NewProvider(op, Node{ID: "n", Cluster: "c"}, "", time.Second, sdsStats)

	pool, err := p.FetchInitialValidationContext(context.Background(), "validation_ca")
	if err != nil {
		t.Fatalf("FetchInitialValidationContext() unexpected error: %v", err)
	}
	if pool == nil {
		t.Error("FetchInitialValidationContext() pool = nil, want non-nil")
	}

	if got := sdsStats.updateAttempt.Load(); got != 1 {
		t.Errorf("update_attempt = %d, want 1", got)
	}
	if got := sdsStats.updateSuccess.Load(); got != 1 {
		t.Errorf("update_success = %d, want 1", got)
	}
}

func TestProvider_FetchInitialValidationContext_Timeout(t *testing.T) {
	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "validation_ca")

	p := NewProvider(&vcFakeOpener{block: true}, Node{ID: "n", Cluster: "c"}, "", 50*time.Millisecond, sdsStats)

	pool, err := p.FetchInitialValidationContext(context.Background(), "validation_ca")
	if err == nil {
		t.Fatal("FetchInitialValidationContext() error = nil, want non-nil (deadline)")
	}
	if pool != nil {
		t.Errorf("FetchInitialValidationContext() pool = %v, want nil", pool)
	}
	if !strings.Contains(err.Error(), "initial fetch timed out") {
		t.Errorf("error = %q, want it to mention the initial-fetch timeout", err.Error())
	}

	if got := sdsStats.initFetchTimeout.Load(); got != 1 {
		t.Errorf("init_fetch_timeout = %d, want 1", got)
	}
}

func TestProvider_FetchInitialValidationContext_MgmtDown(t *testing.T) {
	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "validation_ca")

	p := NewProvider(&vcFakeOpener{openErr: errors.New("dial refused")}, Node{ID: "n", Cluster: "c"}, "", time.Second, sdsStats)

	pool, err := p.FetchInitialValidationContext(context.Background(), "validation_ca")
	if err == nil {
		t.Fatal("FetchInitialValidationContext() error = nil, want non-nil (mgmt down)")
	}
	if pool != nil {
		t.Errorf("FetchInitialValidationContext() pool = %v, want nil", pool)
	}
	if !strings.Contains(err.Error(), "open stream") {
		t.Errorf("error = %q, want it to mention open stream", err.Error())
	}

	if got := sdsStats.updateAttempt.Load(); got != 1 {
		t.Errorf("update_attempt = %d, want 1 (counted unconditionally, before the opener)", got)
	}
	if got := sdsStats.updateFailure.Load(); got != 1 {
		t.Errorf("update_failure = %d, want 1", got)
	}
}

func TestProvider_FetchInitialValidationContext_Rejected(t *testing.T) {
	reg := stats.NewRegistry()
	sdsStats := RegisterSDSStats(reg, "validation_ca")
	// A tls_certificate served where a validation_context was requested -> rejected.
	op := &vcFakeOpener{resp: &discoveryv3.DiscoveryResponse{
		VersionInfo: "v1", Nonce: "n1",
		Resources: []*anypb.Any{validSecretAny(t, "validation_ca")},
	}}

	p := NewProvider(op, Node{ID: "n", Cluster: "c"}, "", time.Second, sdsStats)

	pool, err := p.FetchInitialValidationContext(context.Background(), "validation_ca")
	if err == nil {
		t.Fatal("FetchInitialValidationContext() error = nil, want non-nil (rejected)")
	}
	if pool != nil {
		t.Errorf("FetchInitialValidationContext() pool = %v, want nil", pool)
	}

	if got := sdsStats.updateRejected.Load(); got != 1 {
		t.Errorf("update_rejected = %d, want 1 (a validation failure classifies as rejected, not failure)", got)
	}
	if got := sdsStats.updateFailure.Load(); got != 0 {
		t.Errorf("update_failure = %d, want 0 (a reject must NOT also count as a failure)", got)
	}
}
