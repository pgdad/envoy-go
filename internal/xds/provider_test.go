package xds

import (
	"context"
	"net"
	"testing"
	"time"

	secretv3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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
