package grpcclient

// grpcclient_test.go — Groups 1+2 test SCAFFOLDING per SPEC §14.1.
//
// Task 2 (this file): Groups 1 (Dialer) + 2 (AuthClient) table-driven test
// cases scaffolded. Assertions describe the eventual real-impl behavior;
// tests FAIL against the Task-2 sentinel stubs (`errTODOTask3`). Group 3
// (Close idempotency) is added by Task 3 alongside the real method bodies.
//
// Test infrastructure (helpers below):
//
//   - `mkAuthPKI(t)` — in-memory CA + leaf cert/key pair (ECDSA P-256). The
//     leaf has CN/SAN "alpha.envoy-go.test" so the dialer's TLS handshake
//     against the in-process server validates cleanly. Modeled on
//     `internal/cluster/dial_h2_test.go`'s `mkH2TestPKI`.
//   - `mkH2ClusterMgr(t, name, port)` — builds a `*cluster.Manager` with a
//     single STATIC cluster wired for HTTP/2 (TLS + ALPN h2 +
//     `http2_protocol_options{}`). Modeled on
//     `internal/filter/hcm/config_test.go`'s `mkH2ClusterManager`.
//   - `mkPlainClusterMgr(t, name, port)` — plaintext STATIC cluster with
//     `UseH2() == false`. Modeled on
//     `internal/filter/hcm/fuzz_test.go`'s `mkOneClusterManagerTB`.
//   - `startTestAuthServer(t, pki, h)` — starts a TLS-fronted `*grpc.Server`
//     on a loopback port with `NextProtos: ["h2"]`; registers `h` (the
//     fake AuthorizationServer) and returns the bound port + a stop func.
//
// FAIL semantics for the Task-2 stubs:
//
//   - Happy-path tests (DialContext / NewAuthClient / Check) expect a
//     non-nil result; the stub returns `errTODOTask3` → FAIL.
//   - PARSE-REJECT tests expect the err string to mention the cluster name;
//     the sentinel `"grpcclient: TODO (Task 3)"` does NOT → FAIL.
//   - Transport-error tests expect a recognizable timeout/cancel/unavailable
//     classifier; the sentinel does NOT match → FAIL.
//
// The Task 3 implementer replaces the method bodies in `grpcclient.go` with
// the real impl; the tests immediately go green (modulo any assertion drift
// the Task 3 implementer also fixes).

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/stats"
)

// ----------------------------------------------------------------------------
// PKI + cluster-manager helpers.
// ----------------------------------------------------------------------------

// authTestPKI carries an in-memory CA + leaf cert/key pair sufficient for a
// TLS handshake between the gRPC dialer (cluster manager-owned TLS) and the
// in-process gRPC auth server. Generated per test (cheap ECDSA P-256).
type authTestPKI struct {
	caPEM       []byte
	caPool      *x509.CertPool
	leafCertPEM []byte
	leafKeyPEM  []byte
}

func mkAuthPKI(t testing.TB) *authTestPKI {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "grpcclient test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("ca cert: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "alpha.envoy-go.test"},
		DNSNames:     []string{"alpha.envoy-go.test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, caTmpl, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("leaf cert: %v", err)
	}
	leafCertPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leafDER})
	leafKeyDER, err := x509.MarshalPKCS8PrivateKey(leafKey)
	if err != nil {
		t.Fatalf("leaf key marshal: %v", err)
	}
	leafKeyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: leafKeyDER})

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	return &authTestPKI{
		caPEM:       caPEM,
		caPool:      pool,
		leafCertPEM: leafCertPEM,
		leafKeyPEM:  leafKeyPEM,
	}
}

// mkH2ClusterMgr builds a *cluster.Manager containing a single STATIC cluster
// `name` listening at 127.0.0.1:port configured for HTTP/2 upstream
// origination (TLS + ALPN h2 + `http2_protocol_options{}`). The CA cert
// from `pki` is inlined as the cluster's `validation_context.trusted_ca`.
// Modeled on `internal/filter/hcm/config_test.go`'s `mkH2ClusterManager`.
func mkH2ClusterMgr(t testing.TB, pki *authTestPKI, name string, port uint32) *cluster.Manager {
	t.Helper()
	ctx := &tlsv3.UpstreamTlsContext{
		Sni: "alpha.envoy-go.test",
		CommonTlsContext: &tlsv3.CommonTlsContext{
			AlpnProtocols: []string{"h2"},
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{
					TrustedCa: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineBytes{InlineBytes: pki.caPEM},
					},
				},
			},
		},
	}
	tsAny, err := anypb.New(ctx)
	if err != nil {
		t.Fatalf("anypb.New(UpstreamTlsContext): %v", err)
	}
	hpoH2 := &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
			},
		},
	}
	hpoAny, err := anypb.New(hpoH2)
	if err != nil {
		t.Fatalf("anypb.New(HttpProtocolOptions): %v", err)
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: name,
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
									},
								}},
							}},
						}},
					}},
				},
				TransportSocket: &corev3.TransportSocket{
					Name:       "envoy.transport_sockets.tls",
					ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: tsAny},
				},
				TypedExtensionProtocolOptions: map[string]*anypb.Any{
					"envoy.extensions.upstreams.http.v3.HttpProtocolOptions": hpoAny,
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager(h2): %v", err)
	}
	return cm
}

// mkPlainClusterMgr builds a *cluster.Manager containing a single plaintext
// STATIC cluster `name` at 127.0.0.1:port with `UseH2() == false`. The
// loopback port is arbitrary; PARSE-REJECT paths never reach the dial step.
// Modeled on `internal/filter/hcm/fuzz_test.go`'s `mkOneClusterManagerTB`.
func mkPlainClusterMgr(t testing.TB, name string, port uint32) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: name,
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
									},
								}},
							}},
						}},
					}},
				},
			}},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager(plain): %v", err)
	}
	return cm
}

// ----------------------------------------------------------------------------
// In-process gRPC auth server.
// ----------------------------------------------------------------------------

// fakeAuthServer implements `authv3.AuthorizationServer`. The Check method:
//
//   - returns `scripted` immediately if non-nil
//   - blocks on `ctx.Done()` if `scripted` is nil (used by timeout / cancel tests)
type fakeAuthServer struct {
	authv3.UnimplementedAuthorizationServer
	scripted *authv3.CheckResponse
}

func (f *fakeAuthServer) Check(ctx context.Context, _ *authv3.CheckRequest) (*authv3.CheckResponse, error) {
	if f.scripted != nil {
		return f.scripted, nil
	}
	// Block until the caller cancels; surfaces ctx.Err() (Canceled / DeadlineExceeded).
	<-ctx.Done()
	return nil, ctx.Err()
}

// startTestAuthServer starts a TLS-fronted `*grpc.Server` on a loopback port
// with ALPN h2; registers a `fakeAuthServer{scripted}`. Returns the bound
// port and a `stop` func (calls `GracefulStop`).
func startTestAuthServer(t testing.TB, pki *authTestPKI, scripted *authv3.CheckResponse) (uint32, func()) {
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
	authv3.RegisterAuthorizationServer(s, &fakeAuthServer{scripted: scripted})
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

// ----------------------------------------------------------------------------
// Group 1 — Dialer surface
// ----------------------------------------------------------------------------

// TestDialer_New_ReturnsNonNil verifies `New` returns a non-nil `*Dialer`
// even with a nil cluster manager (it's a stateless wrapper; the nil-mgr
// case ERRORs only when DialContext is called).
func TestDialer_New_ReturnsNonNil(t *testing.T) {
	t.Parallel()
	d := New(nil)
	if d == nil {
		t.Fatalf("New(nil) = nil; want non-nil *Dialer")
	}
}

// TestDialer_DialContext_HappyPath verifies the happy path: an h2-enabled
// cluster (TLS + ALPN h2) produces a usable `*grpc.ClientConn`.
//
// FAIL mode against Task-2 stub: `DialContext` returns `errTODOTask3`.
func TestDialer_DialContext_HappyPath(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestAuthServer(t, pki, &authv3.CheckResponse{})
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_auth", port)
	d := New(mgr)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	conn, err := d.DialContext(ctx, "c_auth")
	if err != nil {
		t.Fatalf("DialContext: %v", err)
	}
	if conn == nil {
		t.Fatalf("DialContext: returned nil ClientConn")
	}
	t.Cleanup(func() { _ = conn.Close() })
}

// TestDialer_DialContext_ParseReject verifies the two PARSE-REJECT paths:
// unknown cluster and `UseH2() == false`. The error wording is impl-defined
// but must include the cluster name to aid diagnostics.
//
// FAIL mode against Task-2 stub: the stub returns `errTODOTask3` which does
// NOT include the cluster name, so the substring assertion fails.
func TestDialer_DialContext_ParseReject(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		clusterName string
		setupMgr    func(testing.TB) *cluster.Manager
		wantSubstr  string // substring expected in err.Error()
	}{
		{
			name:        "unknown_cluster",
			clusterName: "c_does_not_exist",
			setupMgr: func(t testing.TB) *cluster.Manager {
				return mkPlainClusterMgr(t, "c_other", 9999)
			},
			wantSubstr: "c_does_not_exist",
		},
		{
			name:        "useh2_false",
			clusterName: "c_plain",
			setupMgr: func(t testing.TB) *cluster.Manager {
				return mkPlainClusterMgr(t, "c_plain", 9999)
			},
			wantSubstr: "c_plain",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mgr := tc.setupMgr(t)
			d := New(mgr)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conn, err := d.DialContext(ctx, tc.clusterName)
			if err == nil {
				_ = conn.Close()
				t.Fatalf("DialContext(%q): err = nil; want non-nil PARSE-REJECT", tc.clusterName)
			}
			if conn != nil {
				t.Errorf("DialContext(%q): conn = %v; want nil on PARSE-REJECT", tc.clusterName, conn)
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("DialContext(%q) err = %q; want substring %q", tc.clusterName, err.Error(), tc.wantSubstr)
			}
		})
	}
}

// TestDialer_DialContext_Concurrent verifies concurrent `DialContext` calls
// against the same cluster return distinct `*grpc.ClientConn` values (one-
// per-call discipline — the caller owns Close on each). The dialer does
// NOT cache ClientConns; caching is the caller's responsibility (the filter
// caches via `compiledConfig` per ADR-0158).
//
// FAIL mode against Task-2 stub: all goroutines see `errTODOTask3` and the
// distinct-ness check is unreachable.
func TestDialer_DialContext_Concurrent(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestAuthServer(t, pki, &authv3.CheckResponse{})
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_auth", port)
	d := New(mgr)

	const n = 8
	conns := make([]*grpc.ClientConn, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			conns[i], errs[i] = d.DialContext(ctx, "c_auth")
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("DialContext[%d]: %v", i, err)
			continue
		}
		if conns[i] == nil {
			t.Errorf("DialContext[%d]: nil ClientConn", i)
		}
	}
	// One-per-call discipline: each ClientConn pointer must be distinct.
	seen := make(map[*grpc.ClientConn]struct{}, n)
	for i, c := range conns {
		if c == nil {
			continue
		}
		if _, dup := seen[c]; dup {
			t.Errorf("DialContext[%d]: returned a duplicate *grpc.ClientConn pointer; want one-per-call", i)
		}
		seen[c] = struct{}{}
	}
	t.Cleanup(func() {
		for _, c := range conns {
			if c != nil {
				_ = c.Close()
			}
		}
	})
}

// ----------------------------------------------------------------------------
// Group 2 — AuthClient surface
// ----------------------------------------------------------------------------

// TestAuthClient_NewAuthClient_HappyPath verifies that `NewAuthClient`
// dials the cluster via the supplied `*Dialer` and returns a usable
// `*AuthClient`.
//
// FAIL mode against Task-2 stub: `NewAuthClient` returns `(nil, errTODOTask3)`.
func TestAuthClient_NewAuthClient_HappyPath(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestAuthServer(t, pki, &authv3.CheckResponse{})
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_auth", port)
	d := New(mgr)

	ac, err := NewAuthClient(d, "c_auth", 2*time.Second)
	if err != nil {
		t.Fatalf("NewAuthClient: %v", err)
	}
	if ac == nil {
		t.Fatalf("NewAuthClient: nil AuthClient")
	}
	t.Cleanup(func() { _ = ac.Close() })
}

// TestAuthClient_NewAuthClient_PropagatesDialError verifies that a dial-time
// PARSE-REJECT surfaces as the `NewAuthClient` error return — wrapped or
// passed through. The error must mention the cluster name to aid diagnostics.
//
// FAIL mode against Task-2 stub: `NewAuthClient` returns `errTODOTask3`
// which does NOT mention the cluster name.
func TestAuthClient_NewAuthClient_PropagatesDialError(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_other", 9999) // wrong name → unknown-cluster
	d := New(mgr)

	ac, err := NewAuthClient(d, "c_missing", time.Second)
	if err == nil {
		_ = ac.Close()
		t.Fatalf("NewAuthClient: err = nil; want PARSE-REJECT propagation")
	}
	if ac != nil {
		t.Errorf("NewAuthClient: ac = %v; want nil on error", ac)
	}
	if !strings.Contains(err.Error(), "c_missing") {
		t.Errorf("NewAuthClient err = %q; want substring %q", err.Error(), "c_missing")
	}
}

// TestAuthClient_Check_HappyPath verifies that `Check` returns the scripted
// `*CheckResponse` from the in-process auth server.
//
// FAIL mode against Task-2 stub: `Check` returns `(nil, errTODOTask3)`.
func TestAuthClient_Check_HappyPath(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	scripted := &authv3.CheckResponse{} // zero-value resp — defensive allow per §6.7
	port, stop := startTestAuthServer(t, pki, scripted)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_auth", port)
	d := New(mgr)

	ac, err := NewAuthClient(d, "c_auth", 2*time.Second)
	if err != nil {
		t.Fatalf("NewAuthClient: %v", err)
	}
	t.Cleanup(func() { _ = ac.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := ac.Check(ctx, &authv3.CheckRequest{})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if resp == nil {
		t.Fatalf("Check: nil response")
	}
}

// TestAuthClient_Check_Timeout verifies per-Check `context.WithTimeout`
// applied INSIDE `Check` per SPEC §3.1 + D7/D9. The fake server blocks past
// the configured timeout; `Check` must return a transport error
// (context.DeadlineExceeded / gRPC DeadlineExceeded) WITHOUT mapping it to
// a CheckResponse (the transport-error path is verbatim per D7).
//
// FAIL mode against Task-2 stub: `Check` returns `errTODOTask3` which is
// NOT a recognizable timeout transport error.
func TestAuthClient_Check_Timeout(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	// nil scripted → fake server blocks Check until ctx.Done; the per-Check
	// timeout fires first.
	port, stop := startTestAuthServer(t, pki, nil)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_auth", port)
	d := New(mgr)

	// Very short per-Check timeout so the test runs quickly.
	ac, err := NewAuthClient(d, "c_auth", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("NewAuthClient: %v", err)
	}
	t.Cleanup(func() { _ = ac.Close() })

	// Caller's ctx is generous; the per-Check timeout fires first.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := ac.Check(ctx, &authv3.CheckRequest{})
	if err == nil {
		t.Fatalf("Check: err = nil; want timeout transport error")
	}
	if resp != nil {
		t.Errorf("Check: resp = %v; want nil on timeout error", resp)
	}
	// Transport-error VERBATIM propagation per D7: the err must be a
	// recognizable timeout — either context.DeadlineExceeded directly or a
	// gRPC status with code DeadlineExceeded. The exact shape is impl-defined.
	if !isDeadlineExceededTransportErr(err) {
		t.Errorf("Check err = %v; want DeadlineExceeded transport error", err)
	}
}

// TestAuthClient_Check_CancelHonored verifies the caller's `ctx.Done()` is
// honored per SPEC §14.2 — `OnDestroy`-driven cancellation propagates through
// `context.WithTimeout`'s AND-of-cancellation semantics. The fake server
// blocks indefinitely; the test cancels the caller's ctx mid-flight.
//
// FAIL mode against Task-2 stub: `Check` returns `errTODOTask3` synchronously
// without observing the caller's ctx.
func TestAuthClient_Check_CancelHonored(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestAuthServer(t, pki, nil) // blocks indefinitely
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_auth", port)
	d := New(mgr)

	ac, err := NewAuthClient(d, "c_auth", 10*time.Second) // generous per-Check timeout
	if err != nil {
		t.Fatalf("NewAuthClient: %v", err)
	}
	t.Cleanup(func() { _ = ac.Close() })

	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay; Check must return promptly with the
	// caller-cancel transport error.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	resp, err := ac.Check(ctx, &authv3.CheckRequest{})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("Check: err = nil; want canceled transport error")
	}
	if resp != nil {
		t.Errorf("Check: resp = %v; want nil on cancel", resp)
	}
	// Must return well before the per-Check timeout (10s) — i.e. observe ctx.Done.
	if elapsed > 2*time.Second {
		t.Errorf("Check: did not observe ctx.Done promptly; elapsed=%v", elapsed)
	}
	if !isCanceledTransportErr(err) {
		t.Errorf("Check err = %v; want Canceled transport error", err)
	}
}

// TestAuthClient_Check_TransportErrorVerbatim verifies the D7 transport-
// error-verbatim discipline: gRPC `Unavailable` (server-down) propagates as
// the error return WITHOUT being mapped to a dispError-bearing CheckResponse
// (that's the FILTER's responsibility per SPEC §6.7).
//
// FAIL mode against Task-2 stub: `Check` returns `errTODOTask3` which is not
// a recognizable transport error.
func TestAuthClient_Check_TransportErrorVerbatim(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestAuthServer(t, pki, &authv3.CheckResponse{})
	mgr := mkH2ClusterMgr(t, pki, "c_auth", port)
	d := New(mgr)

	ac, err := NewAuthClient(d, "c_auth", 2*time.Second)
	if err != nil {
		t.Fatalf("NewAuthClient: %v", err)
	}
	t.Cleanup(func() { _ = ac.Close() })

	// Kill the auth server BEFORE the Check call — the next dial sub-channel
	// state transition sees Unavailable.
	stop()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	resp, err := ac.Check(ctx, &authv3.CheckRequest{})
	if err == nil {
		t.Fatalf("Check: err = nil; want Unavailable / transport error")
	}
	if resp != nil {
		t.Errorf("Check: resp = %v; want nil on transport error", resp)
	}
	// The error MUST surface as the error return — Check does NOT synthesize
	// a *CheckResponse to carry it. This is the D7 distinction codified.
}

// ----------------------------------------------------------------------------
// Internal test helpers — transport-error shape classifiers.
//
// These helpers are loose substring/identity classifiers that the Task 3
// impl tightens once the real impl is in place (it may use
// `status.FromError` + `codes.DeadlineExceeded` / `codes.Canceled` /
// `codes.Unavailable` matchers).
// ----------------------------------------------------------------------------

func isDeadlineExceededTransportErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(err.Error(), "DeadlineExceeded") ||
		strings.Contains(err.Error(), "deadline exceeded")
}

func isCanceledTransportErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return true
	}
	return strings.Contains(err.Error(), "Canceled") ||
		strings.Contains(err.Error(), "canceled")
}

// ----------------------------------------------------------------------------
// Group 3 — Close idempotency (Task 3 — real-impl-only)
// ----------------------------------------------------------------------------

// TestAuthClient_Close_Idempotent verifies the sync.Once-guarded Close per
// SPEC §3.1: repeated `Close()` calls return cleanly (the same cached error
// from the first call); the underlying `*grpc.ClientConn.Close` fires at
// most once; a post-Close `Check()` fails with a recognizable closed-conn
// error (the standard gRPC ClientConn closed sentinel).
func TestAuthClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestAuthServer(t, pki, &authv3.CheckResponse{})
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_auth", port)
	d := New(mgr)

	ac, err := NewAuthClient(d, "c_auth", 2*time.Second)
	if err != nil {
		t.Fatalf("NewAuthClient: %v", err)
	}

	// First Close: cached err captured (likely nil — gRPC ClientConn.Close
	// returns nil under normal conditions).
	err1 := ac.Close()
	// Second Close: must return the SAME error as the first (sync.Once-cached).
	err2 := ac.Close()
	// Third Close: still cached.
	err3 := ac.Close()

	// All three must agree.
	if (err1 == nil) != (err2 == nil) || (err2 == nil) != (err3 == nil) {
		t.Errorf("Close idempotency: err1=%v, err2=%v, err3=%v; want all equal", err1, err2, err3)
	}
	if err1 != nil && (err1.Error() != err2.Error() || err2.Error() != err3.Error()) {
		t.Errorf("Close idempotency: err1=%q, err2=%q, err3=%q; want all equal", err1, err2, err3)
	}

	// Post-Close `Check` must surface a closed-conn transport error — gRPC's
	// `*grpc.ClientConn.Invoke` on a closed conn returns a `Canceled` (or
	// similar) status. We don't assert the exact wording, only that the call
	// fails non-nil — the response must be nil per D7.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	resp, callErr := ac.Check(ctx, &authv3.CheckRequest{})
	if callErr == nil {
		t.Errorf("Check after Close: err = nil; want closed-conn transport error")
	}
	if resp != nil {
		t.Errorf("Check after Close: resp = %v; want nil on closed-conn error", resp)
	}
}

// TestAuthClient_Close_ConcurrentRaceClean verifies that N concurrent
// `Close()` invocations under -race produce no race-detector violation; all
// return the SAME error (sync.Once cached); the underlying
// `*grpc.ClientConn.Close` fires at most once. This codifies the D9
// concurrent-Close discipline.
func TestAuthClient_Close_ConcurrentRaceClean(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestAuthServer(t, pki, &authv3.CheckResponse{})
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_auth", port)
	d := New(mgr)

	ac, err := NewAuthClient(d, "c_auth", 2*time.Second)
	if err != nil {
		t.Fatalf("NewAuthClient: %v", err)
	}

	const n = 10
	errs := make([]error, n)
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		i := i
		go func() {
			defer wg.Done()
			errs[i] = ac.Close()
		}()
	}
	wg.Wait()

	// All N goroutines must observe the SAME cached error (whether nil or non-nil).
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

// TestAuthClient_Close_NilSafe verifies that calling `Close()` on a nil
// `*AuthClient` is a no-op returning nil — the public API tolerates nil
// receiver per the D9 idempotency discipline (mirrors `sync.Once`'s own
// nil-tolerance pattern via the early return). This is a small belt-and-
// braces robustness test; the call-site never passes nil in production.
func TestAuthClient_Close_NilSafe(t *testing.T) {
	t.Parallel()
	var ac *AuthClient
	if err := ac.Close(); err != nil {
		t.Errorf("nil AuthClient Close: err = %v; want nil", err)
	}
}

// ----------------------------------------------------------------------------
// In-process gRPC ALS (AccessLogService) server — Task 4 (ALSClient).
//
// Task 9's `test/helpers/accessloggrpc` receiver is NOT yet landed, so the
// StreamAccessLogs smoke test stands up a BARE in-test AccessLogService server
// here: a no-op stub that drains `Recv()` until EOF then `SendAndClose`s an
// empty `StreamAccessLogsResponse`. Mirrors `startTestAuthServer` (TLS-fronted,
// ALPN h2) so the same `mkH2ClusterMgr` builder wires a cluster to it.
// ----------------------------------------------------------------------------

// fakeALSServer implements `accesslogv3.AccessLogServiceServer`. StreamAccessLogs
// drains the client stream until EOF then closes with an empty response.
type fakeALSServer struct {
	accesslogv3.UnimplementedAccessLogServiceServer
}

func (f *fakeALSServer) StreamAccessLogs(stream accesslogv3.AccessLogService_StreamAccessLogsServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&accesslogv3.StreamAccessLogsResponse{})
		}
		if err != nil {
			return err
		}
	}
}

// startTestALSServer starts a TLS-fronted `*grpc.Server` on a loopback port
// with ALPN h2; registers a `fakeALSServer`. Returns the bound port and a
// `stop` func (calls `GracefulStop`). Mirrors `startTestAuthServer`.
func startTestALSServer(t testing.TB, pki *authTestPKI) (uint32, func()) {
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
	accesslogv3.RegisterAccessLogServiceServer(s, &fakeALSServer{})
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

// ----------------------------------------------------------------------------
// Group 4 — ALSClient surface (Task 4) — the EXACT AuthClient analog for the
// Access Log Service (ADR-0255 / ADR-0158 precedent).
// ----------------------------------------------------------------------------

// TestALSClient_NewALSClient_NilDialer verifies a nil `*Dialer` errors with the
// cluster name named (mirrors NewAuthClient's nil-dialer guard).
func TestALSClient_NewALSClient_NilDialer(t *testing.T) {
	t.Parallel()
	c, err := NewALSClient(nil, "c_als")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewALSClient(nil): err = nil; want non-nil")
	}
	if c != nil {
		t.Errorf("NewALSClient(nil): c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_als") {
		t.Errorf("NewALSClient(nil) err = %q; want substring %q", err.Error(), "c_als")
	}
}

// TestALSClient_NewALSClient_UnknownCluster verifies the DialContext
// unknown-cluster PARSE-REJECT propagates through NewALSClient, naming the
// cluster.
func TestALSClient_NewALSClient_UnknownCluster(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_other", 9999) // wrong name → unknown-cluster
	d := New(mgr)

	c, err := NewALSClient(d, "c_missing")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewALSClient: err = nil; want unknown-cluster PARSE-REJECT")
	}
	if c != nil {
		t.Errorf("NewALSClient: c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_missing") {
		t.Errorf("NewALSClient err = %q; want substring %q", err.Error(), "c_missing")
	}
	if !strings.Contains(err.Error(), "unknown cluster") {
		t.Errorf("NewALSClient err = %q; want substring %q", err.Error(), "unknown cluster")
	}
}

// TestALSClient_NewALSClient_NonH2Cluster verifies a cluster WITHOUT
// http2_protocol_options{} errors via the DialContext UseH2() gate.
func TestALSClient_NewALSClient_NonH2Cluster(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_plain", 9999) // UseH2() == false
	d := New(mgr)

	c, err := NewALSClient(d, "c_plain")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewALSClient: err = nil; want non-H2 PARSE-REJECT")
	}
	if c != nil {
		t.Errorf("NewALSClient: c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_plain") {
		t.Errorf("NewALSClient err = %q; want substring %q", err.Error(), "c_plain")
	}
	if !strings.Contains(err.Error(), "HTTP/2 framing") {
		t.Errorf("NewALSClient err = %q; want substring %q", err.Error(), "HTTP/2 framing")
	}
}

// TestALSClient_Close_Idempotent verifies the sync.Once-guarded Close against a
// valid H2 cluster: repeated Close() returns the same (nil) error, no panic.
func TestALSClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestALSServer(t, pki)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_als", port)
	d := New(mgr)

	c, err := NewALSClient(d, "c_als")
	if err != nil {
		t.Fatalf("NewALSClient: %v", err)
	}

	err1 := c.Close()
	err2 := c.Close()
	err3 := c.Close()
	if (err1 == nil) != (err2 == nil) || (err2 == nil) != (err3 == nil) {
		t.Errorf("Close idempotency: err1=%v, err2=%v, err3=%v; want all equal", err1, err2, err3)
	}
	if err1 != nil && (err1.Error() != err2.Error() || err2.Error() != err3.Error()) {
		t.Errorf("Close idempotency: err1=%q, err2=%q, err3=%q; want all equal", err1, err2, err3)
	}
}

// TestALSClient_StreamAccessLogs_ReturnsStream verifies that against a valid H2
// cluster wired to the in-test AccessLogService server, StreamAccessLogs opens
// a non-nil client-streaming RPC whose Send succeeds.
func TestALSClient_StreamAccessLogs_ReturnsStream(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestALSServer(t, pki)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_als", port)
	d := New(mgr)

	c, err := NewALSClient(d, "c_als")
	if err != nil {
		t.Fatalf("NewALSClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := c.StreamAccessLogs(ctx)
	if err != nil {
		t.Fatalf("StreamAccessLogs: %v", err)
	}
	if stream == nil {
		t.Fatalf("StreamAccessLogs: nil stream")
	}
	if err := stream.Send(&accesslogv3.StreamAccessLogsMessage{}); err != nil {
		t.Fatalf("stream.Send: %v", err)
	}
	if _, err := stream.CloseAndRecv(); err != nil {
		t.Fatalf("stream.CloseAndRecv: %v", err)
	}
}

// TestALSClient_Close_NilSafe verifies Close() on a nil *ALSClient is a no-op
// returning nil (mirrors AuthClient.Close nil-tolerance).
func TestALSClient_Close_NilSafe(t *testing.T) {
	t.Parallel()
	var c *ALSClient
	if err := c.Close(); err != nil {
		t.Errorf("nil ALSClient Close: err = %v; want nil", err)
	}
}

// ----------------------------------------------------------------------------
// In-process gRPC OTLP (LogsService) server — Task 4 (OTLPLogsClient).
//
// The OTLP `Export` is a plain UNARY RPC (no stream lifecycle), so the smoke
// test stands up a BARE in-test LogsService server here: a no-op stub that
// returns an empty `ExportLogsServiceResponse`. Mirrors `startTestALSServer`
// (TLS-fronted, ALPN h2) so the same `mkH2ClusterMgr` builder wires a cluster
// to it.
// ----------------------------------------------------------------------------

// fakeOTLPLogsServer implements `collogspb.LogsServiceServer`. Export returns an
// empty response with no error.
type fakeOTLPLogsServer struct {
	collogspb.UnimplementedLogsServiceServer
}

func (f *fakeOTLPLogsServer) Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
	return &collogspb.ExportLogsServiceResponse{}, nil
}

// startTestOTLPLogsServer starts a TLS-fronted `*grpc.Server` on a loopback port
// with ALPN h2; registers a `fakeOTLPLogsServer`. Returns the bound port and a
// `stop` func (calls `GracefulStop`). Mirrors `startTestALSServer`.
func startTestOTLPLogsServer(t testing.TB, pki *authTestPKI) (uint32, func()) {
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
	collogspb.RegisterLogsServiceServer(s, &fakeOTLPLogsServer{})
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

// ----------------------------------------------------------------------------
// Group 4b — OTLPLogsClient surface (Task 4) — the EXACT ALSClient analog for
// the OTLP LogsService, but UNARY (ADR-0258 / ADR-0255 precedent).
// ----------------------------------------------------------------------------

// TestOTLPLogsClient_New_NilDialer verifies a nil `*Dialer` errors with the
// cluster name named (mirrors NewALSClient's nil-dialer guard).
func TestOTLPLogsClient_New_NilDialer(t *testing.T) {
	t.Parallel()
	c, err := NewOTLPLogsClient(nil, "c_otlp")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewOTLPLogsClient(nil): err = nil; want non-nil")
	}
	if c != nil {
		t.Errorf("NewOTLPLogsClient(nil): c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_otlp") {
		t.Errorf("NewOTLPLogsClient(nil) err = %q; want substring %q", err.Error(), "c_otlp")
	}
}

// TestOTLPLogsClient_New_UnknownCluster verifies the DialContext unknown-cluster
// PARSE-REJECT propagates through NewOTLPLogsClient, naming the cluster.
func TestOTLPLogsClient_New_UnknownCluster(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_other", 9999) // wrong name → unknown-cluster
	d := New(mgr)

	c, err := NewOTLPLogsClient(d, "c_missing")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewOTLPLogsClient: err = nil; want unknown-cluster PARSE-REJECT")
	}
	if c != nil {
		t.Errorf("NewOTLPLogsClient: c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_missing") {
		t.Errorf("NewOTLPLogsClient err = %q; want substring %q", err.Error(), "c_missing")
	}
	if !strings.Contains(err.Error(), "unknown cluster") {
		t.Errorf("NewOTLPLogsClient err = %q; want substring %q", err.Error(), "unknown cluster")
	}
}

// TestOTLPLogsClient_New_NonH2Cluster verifies a cluster WITHOUT
// http2_protocol_options{} errors via the DialContext UseH2() gate.
func TestOTLPLogsClient_New_NonH2Cluster(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_plain", 9999) // UseH2() == false
	d := New(mgr)

	c, err := NewOTLPLogsClient(d, "c_plain")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewOTLPLogsClient: err = nil; want non-H2 PARSE-REJECT")
	}
	if c != nil {
		t.Errorf("NewOTLPLogsClient: c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_plain") {
		t.Errorf("NewOTLPLogsClient err = %q; want substring %q", err.Error(), "c_plain")
	}
	if !strings.Contains(err.Error(), "HTTP/2 framing") {
		t.Errorf("NewOTLPLogsClient err = %q; want substring %q", err.Error(), "HTTP/2 framing")
	}
}

// TestOTLPLogsClient_Close_Idempotent verifies the sync.Once-guarded Close
// against a valid H2 cluster: repeated Close() returns the same (nil) error, no
// panic.
func TestOTLPLogsClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestOTLPLogsServer(t, pki)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_otlp", port)
	d := New(mgr)

	c, err := NewOTLPLogsClient(d, "c_otlp")
	if err != nil {
		t.Fatalf("NewOTLPLogsClient: %v", err)
	}

	err1 := c.Close()
	err2 := c.Close()
	err3 := c.Close()
	if (err1 == nil) != (err2 == nil) || (err2 == nil) != (err3 == nil) {
		t.Errorf("Close idempotency: err1=%v, err2=%v, err3=%v; want all equal", err1, err2, err3)
	}
	if err1 != nil && (err1.Error() != err2.Error() || err2.Error() != err3.Error()) {
		t.Errorf("Close idempotency: err1=%q, err2=%q, err3=%q; want all equal", err1, err2, err3)
	}
}

// TestOTLPLogsClient_Export_RoundTrips verifies that against a valid H2 cluster
// wired to the in-test LogsService server, Export returns a non-nil response and
// nil error.
func TestOTLPLogsClient_Export_RoundTrips(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestOTLPLogsServer(t, pki)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_otlp", port)
	d := New(mgr)

	c, err := NewOTLPLogsClient(d, "c_otlp")
	if err != nil {
		t.Fatalf("NewOTLPLogsClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := c.Export(ctx, &collogspb.ExportLogsServiceRequest{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if resp == nil {
		t.Fatalf("Export: nil response")
	}
}

// TestOTLPLogsClient_Close_NilSafe verifies Close() on a nil *OTLPLogsClient is
// a no-op returning nil (mirrors ALSClient.Close nil-tolerance).
func TestOTLPLogsClient_Close_NilSafe(t *testing.T) {
	t.Parallel()
	var c *OTLPLogsClient
	if err := c.Close(); err != nil {
		t.Errorf("nil OTLPLogsClient Close: err = %v; want nil", err)
	}
}

// ----------------------------------------------------------------------------
// In-process gRPC OTLP (TraceService) server — Task 2 (OTLPTracesClient).
//
// The OTLP TraceService `Export` is a plain UNARY RPC (no stream lifecycle),
// so the smoke test stands up a BARE in-test TraceService server: a no-op stub
// that returns an empty `ExportTraceServiceResponse`. Mirrors
// `startTestOTLPLogsServer` (TLS-fronted, ALPN h2) so the same
// `mkH2ClusterMgr` builder wires a cluster to it.
// ----------------------------------------------------------------------------

// fakeOTLPTracesServer implements `coltracepb.TraceServiceServer`. Export
// returns an empty response with no error.
type fakeOTLPTracesServer struct {
	coltracepb.UnimplementedTraceServiceServer
}

func (f *fakeOTLPTracesServer) Export(_ context.Context, _ *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

// startTestOTLPTracesServer starts a TLS-fronted `*grpc.Server` on a loopback
// port with ALPN h2; registers a `fakeOTLPTracesServer`. Returns the bound
// port and a `stop` func (calls `GracefulStop`). Mirrors
// `startTestOTLPLogsServer`.
func startTestOTLPTracesServer(t testing.TB, pki *authTestPKI) (uint32, func()) {
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
	coltracepb.RegisterTraceServiceServer(s, &fakeOTLPTracesServer{})
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

// ----------------------------------------------------------------------------
// Group 4c — OTLPTracesClient surface (Task 2) — the EXACT OTLPLogsClient
// analog for the OTLP TraceService, UNARY (ADR-0260 / ADR-0258 precedent).
// ----------------------------------------------------------------------------

// TestOTLPTracesClient_New_NilDialer verifies a nil `*Dialer` errors with the
// cluster name named (mirrors NewOTLPLogsClient's nil-dialer guard).
func TestOTLPTracesClient_New_NilDialer(t *testing.T) {
	t.Parallel()
	c, err := NewOTLPTracesClient(nil, "c_traces")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewOTLPTracesClient(nil): err = nil; want non-nil")
	}
	if c != nil {
		t.Errorf("NewOTLPTracesClient(nil): c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_traces") {
		t.Errorf("NewOTLPTracesClient(nil) err = %q; want substring %q", err.Error(), "c_traces")
	}
}

// TestOTLPTracesClient_New_UnknownCluster verifies the DialContext
// unknown-cluster PARSE-REJECT propagates through NewOTLPTracesClient, naming
// the cluster.
func TestOTLPTracesClient_New_UnknownCluster(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_other", 9999) // wrong name → unknown-cluster
	d := New(mgr)

	c, err := NewOTLPTracesClient(d, "c_missing")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewOTLPTracesClient: err = nil; want unknown-cluster PARSE-REJECT")
	}
	if c != nil {
		t.Errorf("NewOTLPTracesClient: c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_missing") {
		t.Errorf("NewOTLPTracesClient err = %q; want substring %q", err.Error(), "c_missing")
	}
	if !strings.Contains(err.Error(), "unknown cluster") {
		t.Errorf("NewOTLPTracesClient err = %q; want substring %q", err.Error(), "unknown cluster")
	}
}

// TestOTLPTracesClient_New_NonH2Cluster verifies a cluster WITHOUT
// http2_protocol_options{} errors via the DialContext UseH2() gate.
func TestOTLPTracesClient_New_NonH2Cluster(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_plain", 9999) // UseH2() == false
	d := New(mgr)

	c, err := NewOTLPTracesClient(d, "c_plain")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewOTLPTracesClient: err = nil; want non-H2 PARSE-REJECT")
	}
	if c != nil {
		t.Errorf("NewOTLPTracesClient: c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_plain") {
		t.Errorf("NewOTLPTracesClient err = %q; want substring %q", err.Error(), "c_plain")
	}
	if !strings.Contains(err.Error(), "http2_protocol_options") {
		t.Errorf("NewOTLPTracesClient err = %q; want substring %q", err.Error(), "http2_protocol_options")
	}
}

// TestOTLPTracesClient_Close_Idempotent verifies the sync.Once-guarded Close
// against a valid H2 cluster: repeated Close() returns the same (nil) error,
// no panic.
func TestOTLPTracesClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestOTLPTracesServer(t, pki)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_traces", port)
	d := New(mgr)

	c, err := NewOTLPTracesClient(d, "c_traces")
	if err != nil {
		t.Fatalf("NewOTLPTracesClient: %v", err)
	}

	err1 := c.Close()
	err2 := c.Close()
	err3 := c.Close()
	if (err1 == nil) != (err2 == nil) || (err2 == nil) != (err3 == nil) {
		t.Errorf("Close idempotency: err1=%v, err2=%v, err3=%v; want all equal", err1, err2, err3)
	}
	if err1 != nil && (err1.Error() != err2.Error() || err2.Error() != err3.Error()) {
		t.Errorf("Close idempotency: err1=%q, err2=%q, err3=%q; want all equal", err1, err2, err3)
	}
}

// TestOTLPTracesClient_Export_RoundTrips verifies that against a valid H2
// cluster wired to the in-test TraceService server, Export returns a non-nil
// response and nil error.
func TestOTLPTracesClient_Export_RoundTrips(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestOTLPTracesServer(t, pki)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_traces", port)
	d := New(mgr)

	c, err := NewOTLPTracesClient(d, "c_traces")
	if err != nil {
		t.Fatalf("NewOTLPTracesClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := c.Export(ctx, &coltracepb.ExportTraceServiceRequest{})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if resp == nil {
		t.Fatalf("Export: nil response")
	}
}

// TestOTLPTracesClient_Close_NilSafe verifies Close() on a nil
// *OTLPTracesClient is a no-op returning nil (mirrors OTLPLogsClient.Close
// nil-tolerance).
func TestOTLPTracesClient_Close_NilSafe(t *testing.T) {
	t.Parallel()
	var c *OTLPTracesClient
	if err := c.Close(); err != nil {
		t.Errorf("nil OTLPTracesClient Close: err = %v; want nil", err)
	}
}

// TestOTLPTracesClient_Export_NilClientErrors verifies Export on a nil
// *OTLPTracesClient returns a non-nil error cleanly (no panic).
func TestOTLPTracesClient_Export_NilClientErrors(t *testing.T) {
	t.Parallel()
	var c *OTLPTracesClient
	ctx := context.Background()
	resp, err := c.Export(ctx, &coltracepb.ExportTraceServiceRequest{})
	if err == nil {
		t.Errorf("nil OTLPTracesClient Export: err = nil; want non-nil")
	}
	if resp != nil {
		t.Errorf("nil OTLPTracesClient Export: resp = %v; want nil", resp)
	}
}

// ----------------------------------------------------------------------------
// In-process gRPC MetricsService (StreamMetrics) server — Task 3
// (MetricsServiceClient).
//
// Task 8's `test/helpers/metricsservice` receiver is NOT yet landed, so the
// StreamMetrics smoke test stands up a BARE in-test MetricsService server here:
// a no-op stub that drains `Recv()` until EOF then `SendAndClose`s an empty
// `StreamMetricsResponse`. Mirrors `fakeALSServer`/`startTestALSServer` (TLS-
// fronted, ALPN h2) so the same `mkH2ClusterMgr` builder wires a cluster to it.
// ----------------------------------------------------------------------------

// fakeMetricsServer implements `metricsv3.MetricsServiceServer`. StreamMetrics
// drains the client stream until EOF then closes with an empty response.
type fakeMetricsServer struct {
	metricsv3.UnimplementedMetricsServiceServer
}

func (f *fakeMetricsServer) StreamMetrics(stream metricsv3.MetricsService_StreamMetricsServer) error {
	for {
		_, err := stream.Recv()
		if err == io.EOF {
			return stream.SendAndClose(&metricsv3.StreamMetricsResponse{})
		}
		if err != nil {
			return err
		}
	}
}

// startTestMetricsServer starts a TLS-fronted `*grpc.Server` on a loopback port
// with ALPN h2; registers a `fakeMetricsServer`. Returns the bound port and a
// `stop` func (calls `GracefulStop`). Mirrors `startTestALSServer`.
func startTestMetricsServer(t testing.TB, pki *authTestPKI) (uint32, func()) {
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
	metricsv3.RegisterMetricsServiceServer(s, &fakeMetricsServer{})
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

// ----------------------------------------------------------------------------
// Group 5 — MetricsServiceClient surface (Task 3) — the EXACT ALSClient analog
// for the metrics_service StreamMetrics CLIENT-streaming RPC (ADR-0262 /
// ADR-0158 precedent).
// ----------------------------------------------------------------------------

// TestMetricsServiceClient_New_NilDialer verifies a nil `*Dialer` errors with
// the cluster name named (mirrors NewALSClient's nil-dialer guard).
func TestMetricsServiceClient_New_NilDialer(t *testing.T) {
	t.Parallel()
	c, err := NewMetricsServiceClient(nil, "c_ms")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewMetricsServiceClient(nil): err = nil; want non-nil")
	}
	if c != nil {
		t.Errorf("NewMetricsServiceClient(nil): c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_ms") {
		t.Errorf("NewMetricsServiceClient(nil) err = %q; want substring %q", err.Error(), "c_ms")
	}
}

// TestMetricsServiceClient_New_UnknownCluster verifies the DialContext
// unknown-cluster PARSE-REJECT propagates through NewMetricsServiceClient,
// naming the cluster.
func TestMetricsServiceClient_New_UnknownCluster(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_other", 9999) // wrong name → unknown-cluster
	d := New(mgr)

	c, err := NewMetricsServiceClient(d, "c_missing")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewMetricsServiceClient: err = nil; want unknown-cluster PARSE-REJECT")
	}
	if c != nil {
		t.Errorf("NewMetricsServiceClient: c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_missing") {
		t.Errorf("NewMetricsServiceClient err = %q; want substring %q", err.Error(), "c_missing")
	}
	if !strings.Contains(err.Error(), "unknown cluster") {
		t.Errorf("NewMetricsServiceClient err = %q; want substring %q", err.Error(), "unknown cluster")
	}
}

// TestMetricsServiceClient_New_NonH2Cluster verifies a cluster WITHOUT
// http2_protocol_options{} errors via the DialContext UseH2() gate.
func TestMetricsServiceClient_New_NonH2Cluster(t *testing.T) {
	t.Parallel()
	mgr := mkPlainClusterMgr(t, "c_plain", 9999) // UseH2() == false
	d := New(mgr)

	c, err := NewMetricsServiceClient(d, "c_plain")
	if err == nil {
		_ = c.Close()
		t.Fatalf("NewMetricsServiceClient: err = nil; want non-H2 PARSE-REJECT")
	}
	if c != nil {
		t.Errorf("NewMetricsServiceClient: c = %v; want nil on error", c)
	}
	if !strings.Contains(err.Error(), "c_plain") {
		t.Errorf("NewMetricsServiceClient err = %q; want substring %q", err.Error(), "c_plain")
	}
	if !strings.Contains(err.Error(), "HTTP/2 framing") {
		t.Errorf("NewMetricsServiceClient err = %q; want substring %q", err.Error(), "HTTP/2 framing")
	}
}

// TestMetricsServiceClient_Close_Idempotent verifies the sync.Once-guarded
// Close against a valid H2 cluster: repeated Close() returns the same (nil)
// error, no panic.
func TestMetricsServiceClient_Close_Idempotent(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestMetricsServer(t, pki)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_ms", port)
	d := New(mgr)

	c, err := NewMetricsServiceClient(d, "c_ms")
	if err != nil {
		t.Fatalf("NewMetricsServiceClient: %v", err)
	}

	err1 := c.Close()
	err2 := c.Close()
	err3 := c.Close()
	if (err1 == nil) != (err2 == nil) || (err2 == nil) != (err3 == nil) {
		t.Errorf("Close idempotency: err1=%v, err2=%v, err3=%v; want all equal", err1, err2, err3)
	}
	if err1 != nil && (err1.Error() != err2.Error() || err2.Error() != err3.Error()) {
		t.Errorf("Close idempotency: err1=%q, err2=%q, err3=%q; want all equal", err1, err2, err3)
	}
}

// TestMetricsServiceClient_StreamMetrics_ReturnsStream verifies that against a
// valid H2 cluster wired to the in-test MetricsService server, StreamMetrics
// opens a non-nil client-streaming RPC whose Send + CloseAndRecv succeed.
func TestMetricsServiceClient_StreamMetrics_ReturnsStream(t *testing.T) {
	t.Parallel()
	pki := mkAuthPKI(t)
	port, stop := startTestMetricsServer(t, pki)
	t.Cleanup(stop)
	mgr := mkH2ClusterMgr(t, pki, "c_ms", port)
	d := New(mgr)

	c, err := NewMetricsServiceClient(d, "c_ms")
	if err != nil {
		t.Fatalf("NewMetricsServiceClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := c.StreamMetrics(ctx)
	if err != nil {
		t.Fatalf("StreamMetrics: %v", err)
	}
	if stream == nil {
		t.Fatalf("StreamMetrics: nil stream")
	}
	if err := stream.Send(&metricsv3.StreamMetricsMessage{}); err != nil {
		t.Fatalf("stream.Send: %v", err)
	}
	resp, err := stream.CloseAndRecv()
	if err != nil {
		t.Fatalf("stream.CloseAndRecv: %v", err)
	}
	if resp == nil {
		t.Fatalf("stream.CloseAndRecv: nil response")
	}
}

// TestMetricsServiceClient_Close_NilSafe verifies Close() on a nil
// *MetricsServiceClient is a no-op returning nil (mirrors ALSClient.Close
// nil-tolerance).
func TestMetricsServiceClient_Close_NilSafe(t *testing.T) {
	t.Parallel()
	var c *MetricsServiceClient
	if err := c.Close(); err != nil {
		t.Errorf("nil MetricsServiceClient Close: err = %v; want nil", err)
	}
}
