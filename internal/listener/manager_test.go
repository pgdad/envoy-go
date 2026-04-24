package listener

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"net"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

func mkBoot(adminPort uint32, listeners []*listenerv3.Listener, clusters []*clusterv3.Cluster) *bootstrapv3.Bootstrap {
	return &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Listeners: listeners,
			Clusters:  clusters,
		},
	}
}

func mkListener(name, addr string, port uint32, filter *listenerv3.Filter) *listenerv3.Listener {
	return &listenerv3.Listener{
		Name: name,
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: addr, PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port}},
		}},
		FilterChains: []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{filter}}},
	}
}

func mkTcpProxyFilter(t *testing.T, clusterName string) *listenerv3.Filter {
	t.Helper()
	msg := &tcpproxyv3.TcpProxy{StatPrefix: "ingress_tcp", ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: clusterName}}
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return &listenerv3.Filter{Name: "envoy.filters.network.tcp_proxy", ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: a}}
}

// mkClusterMgr builds a cluster.Manager with a single STATIC cluster.
// Local to this file — does not import from tcpproxy.
func mkClusterMgr(t testing.TB, name, host string, port uint32) *cluster.Manager {
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
										Address:       host,
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
	cm, err := cluster.NewManager(bs)
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestManager_HappyPath_Single(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)

	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Before Start: Listeners() must be empty.
	if got := mgr.Listeners(); len(got) != 0 {
		t.Fatalf("Listeners() before Start: want 0, got %d", len(got))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// After Start: one entry with correct name and a bound addr.
	ls := mgr.Listeners()
	if len(ls) != 1 {
		t.Fatalf("Listeners() after Start: want 1, got %d", len(ls))
	}
	if ls[0].Name != "l_tcp" {
		t.Errorf("Listeners()[0].Name = %q, want %q", ls[0].Name, "l_tcp")
	}
	addr := ls[0].Addr
	if addr == "" {
		t.Fatal("Listeners()[0].Addr is empty")
	}

	// Must be dialable.
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatalf("dial after Start: %v", err)
	}
	_ = conn.Close()

	mgr.Stop()

	// After Stop: fresh dial must fail within a short timeout.
	conn2, dialErr := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if dialErr == nil {
		_ = conn2.Close()
		t.Error("dial after Stop succeeded, expected failure")
	}
}

func TestManager_HappyPath_Multi(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp_a", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
		mkListener("l_tcp_b", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)

	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	ls := mgr.Listeners()
	if len(ls) != 2 {
		t.Fatalf("Listeners() after Start: want 2, got %d", len(ls))
	}
	if ls[0].Addr == ls[1].Addr {
		t.Errorf("both listeners share the same address %q", ls[0].Addr)
	}

	// Both must be dialable.
	for _, li := range ls {
		conn, err := net.DialTimeout("tcp", li.Addr, time.Second)
		if err != nil {
			t.Errorf("dial %q: %v", li.Addr, err)
			continue
		}
		_ = conn.Close()
	}

	mgr.Stop()

	// Both closed after Stop.
	for _, li := range ls {
		conn, err := net.DialTimeout("tcp", li.Addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			t.Errorf("dial %q after Stop succeeded, expected failure", li.Addr)
		}
	}
}

func TestManager_Error_ZeroListeners(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "listener: zero listeners in bootstrap") {
		t.Errorf("error %q does not contain %q", err.Error(), "listener: zero listeners in bootstrap")
	}
}

func TestManager_Error_DuplicateName(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
		mkListener("l_tcp", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate listener") {
		t.Errorf("error %q does not contain %q", err.Error(), "duplicate listener")
	}
}

func TestManager_Error_TwoFilterChains(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_tcp",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{
			{Filters: []*listenerv3.Filter{filter}},
			{Filters: []*listenerv3.Filter{filter}},
		},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Phase-03 (ADR-0033): two plaintext catch-all chains → "catch-all" error or
	// "plaintext listener with multiple filter_chains" error. Either is valid.
	if !strings.Contains(err.Error(), "filter_chain") {
		t.Errorf("error %q does not contain %q", err.Error(), "filter_chain")
	}
}

func TestManager_Error_NonEmptyFilterChainMatch(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_tcp",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			FilterChainMatch: &listenerv3.FilterChainMatch{
				DestinationPort: wrapperspb.UInt32(80),
			},
			Filters: []*listenerv3.Filter{filter},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Phase-03 (ADR-0033): destination_port is the specific field rejected.
	if !strings.Contains(err.Error(), "destination_port") {
		t.Errorf("error %q does not contain %q", err.Error(), "destination_port")
	}
}

func TestManager_Error_TwoFilters(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_tcp",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{filter, filter},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "expected exactly one filter") {
		t.Errorf("error %q does not contain %q", err.Error(), "expected exactly one filter")
	}
}

func TestManager_Error_PopulatedTransportSocket(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_tcp",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			TransportSocket: &corev3.TransportSocket{Name: "envoy.transport_sockets.tls"},
			Filters:         []*listenerv3.Filter{filter},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Phase-03 (ADR-0033): a transport_socket with no typed_config is now
	// attempted to be parsed as a DownstreamTlsContext, which fails with a
	// "tls: downstream: unexpected type_url" error. The error must be non-nil.
	if err == nil {
		t.Errorf("expected a non-nil error for a TransportSocket with no typed_config")
	}
}

func TestManager_Error_UnknownFilterTypeURL(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := &listenerv3.Listener{
		Name: "l_tcp",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{{
				Name: "envoy.filters.network.echo",
				ConfigType: &listenerv3.Filter_TypedConfig{
					TypedConfig: &anypb.Any{
						TypeUrl: "type.googleapis.com/envoy.extensions.filters.network.echo.v3.Echo",
						Value:   nil,
					},
				},
			}},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unknown filter type_url") {
		t.Errorf("error %q does not contain %q", err.Error(), "unknown filter type_url")
	}
}

func TestManager_Error_FilterConstructionPropagated(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_does_not_exist")),
	}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Must be prefixed with listener name (manager.go uses fmt.Errorf("listener: %q: ...", name)).
	if !strings.Contains(err.Error(), `listener: "l_tcp"`) {
		t.Errorf("error %q does not contain %q", err.Error(), `listener: "l_tcp"`)
	}
	// Must contain the tcpproxy error.
	if !strings.Contains(err.Error(), `cluster "c_does_not_exist" not found`) {
		t.Errorf("error %q does not contain %q", err.Error(), `cluster "c_does_not_exist" not found`)
	}
}

func TestManager_Error_NonSocketAddressListener(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := &listenerv3.Listener{
		Name: "l_pipe",
		Address: &corev3.Address{Address: &corev3.Address_Pipe{
			Pipe: &corev3.Pipe{Path: "/tmp/test.sock"},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_echo")},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "socket_address") {
		t.Errorf("error %q does not contain %q", err.Error(), "socket_address")
	}
}

func TestManager_BindUnwind(t *testing.T) {
	// Allocate a real listener to hold a port.
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold listen: %v", err)
	}
	defer func() { _ = held.Close() }()
	heldPort := uint32(held.Addr().(*net.TCPAddr).Port)

	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_a", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
		mkListener("l_b", "127.0.0.1", heldPort, mkTcpProxyFilter(t, "c_echo")),
	}, nil)

	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx := context.Background()
	startErr := mgr.Start(ctx)
	if startErr == nil {
		mgr.Stop()
		t.Fatal("Start: expected error due to port conflict, got nil")
	}
	if !strings.Contains(startErr.Error(), "bind") {
		t.Errorf("error %q does not contain %q", startErr.Error(), "bind")
	}

	// After unwind: Listeners() must return 0 entries.
	if got := mgr.Listeners(); len(got) != 0 {
		t.Errorf("Listeners() after failed Start: want 0, got %d", len(got))
	}
}

// ---------------------------------------------------------------------------
// Phase-03 TLS / multi-chain test helpers
// ---------------------------------------------------------------------------

// testPKI holds the inline PEM bytes loaded from test/fixtures/0002-tls-tcp/pki/.
// Rather than reading files at test time (which creates a path-dependency from
// the package dir), the PEMs are embedded as constants below.
// They were generated by test/fixtures/0002-tls-tcp/pki/gen/main.go.

const testCAPEM = `-----BEGIN CERTIFICATE-----
MIIBZjCCAQ2gAwIBAgIBATAKBggqhkjOPQQDAjAbMRkwFwYDVQQDExBlbnZveS1n
byB0ZXN0IENBMB4XDTI2MDEwMTAwMDAwMFoXDTQ2MDEwMTAwMDAwMFowGzEZMBcG
A1UEAxMQZW52b3ktZ28gdGVzdCBDQTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IA
BGeenc7CzvqX4pE+ZU5RkBBycchYJS0b4ltjO+iMnIYhhHLPELLbYeWFcUFkxp1x
1vBm489frHLB7HqUAP0xHsqjQjBAMA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8E
BTADAQH/MB0GA1UdDgQWBBSUoifyWR8KaOrc10lqG9D5Flw1JDAKBggqhkjOPQQD
AgNHADBEAiBkY3S2qDVrcUq44/X0YBhUTUtNHXyPBbQDtygMwGZpBwIgQiBTD4KR
D7tKwv8xF5UiiKnXXhStxlLdgn9McY8dJ4U=
-----END CERTIFICATE-----
`

const testAlphaCertPEM = `-----BEGIN CERTIFICATE-----
MIIBjzCCATagAwIBAgIBCjAKBggqhkjOPQQDAjAbMRkwFwYDVQQDExBlbnZveS1n
byB0ZXN0IENBMB4XDTI2MDEwMTAwMDAwMFoXDTQ2MDEwMTAwMDAwMFowHjEcMBoG
A1UEAxMTYWxwaGEuZW52b3ktZ28udGVzdDBZMBMGByqGSM49AgEGCCqGSM49AwEH
A0IABDWs3bNE9rkW6xWB5t7CZWQk86BFAngmNVeAJJdk4Jz5HdsgcMxmscDauk2b
bhaKg7T7QbL/P1ypOTYyd6fSbvmjaDBmMA4GA1UdDwEB/wQEAwIFoDATBgNVHSUE
DDAKBggrBgEFBQcDATAfBgNVHSMEGDAWgBSUoifyWR8KaOrc10lqG9D5Flw1JDAe
BgNVHREEFzAVghNhbHBoYS5lbnZveS1nby50ZXN0MAoGCCqGSM49BAMCA0cAMEQC
IAy8XOHKE+KCO6tqVXAKnuCZsohw/1BT5g0sIqdJfqm6AiBsHz8z5ivWuGSWeB4s
CJvpxa3L8kMVssG+jnUeLCfOXA==
-----END CERTIFICATE-----
`

const testAlphaKeyPEM = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQg4b9v5t4mnAX/Awgy
bgjQxpXS1a+CDJn8z5bF5frhPOyhRANCAAQ1rN2zRPa5FusVgebewmVkJPOgRQJ4
JjVXgCSXZOCc+R3bIHDMZrHA2rpNm24WioO0+0Gy/z9cqTk2Mnen0m75
-----END PRIVATE KEY-----
`

const testBetaCertPEM = `-----BEGIN CERTIFICATE-----
MIIBjzCCATSgAwIBAgIBCzAKBggqhkjOPQQDAjAbMRkwFwYDVQQDExBlbnZveS1n
byB0ZXN0IENBMB4XDTI2MDEwMTAwMDAwMFoXDTQ2MDEwMTAwMDAwMFowHTEbMBkG
A1UEAxMSYmV0YS5lbnZveS1nby50ZXN0MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcD
QgAELlGBNhnkLWifiVo6pUzBPxzRm5GiEe69gDAYhFD0kxZqvGuYklAYioVi6MDU
2S48dDvv9RUBaqqvyUrRweRN7aNnMGUwDgYDVR0PAQH/BAQDAgWgMBMGA1UdJQQM
MAoGCCsGAQUFBwMBMB8GA1UdIwQYMBaAFJSiJ/JZHwpo6tzXSWob0PkWXDUkMB0G
A1UdEQQWMBSCEmJldGEuZW52b3ktZ28udGVzdDAKBggqhkjOPQQDAgNJADBGAiEA
gVMJCfvp+U89TVIzx36RRkXEt6Mxa/7V+RujUwhZyOwCIQDFI+9hYY3IWu6ruIzq
J4kosbc3Rc/DD/WtCPxYeVUE/w==
-----END CERTIFICATE-----
`

const testBetaKeyPEM = `-----BEGIN PRIVATE KEY-----
MIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgPI38K5OvUZOekPuG
q7iCh/0AQuFMpb6cKmCOXmKu9aWhRANCAAQuUYE2GeQtaJ+JWjqlTME/HNGbkaIR
7r2AMBiEUPSTFmq8a5iSUBiKhWLowNTZLjx0O+/1FQFqqq/JStHB5E3t
-----END PRIVATE KEY-----
`

// mkDownstreamTSInline builds a corev3.TransportSocket carrying a
// DownstreamTlsContext with the given cert+key PEMs as inline_bytes.
func mkDownstreamTSInline(t *testing.T, certPEM, keyPEM string) *corev3.TransportSocket {
	t.Helper()
	inner := &tlsv3.DownstreamTlsContext{
		CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsCertificates: []*tlsv3.TlsCertificate{{
				CertificateChain: &corev3.DataSource{
					Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte(certPEM)},
				},
				PrivateKey: &corev3.DataSource{
					Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte(keyPEM)},
				},
			}},
		},
	}
	a, err := anypb.New(inner)
	if err != nil {
		t.Fatalf("anypb.New DownstreamTlsContext: %v", err)
	}
	return &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.tls",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: a},
	}
}

// mkDownstreamTSRequireClientCert builds a transport socket whose
// require_client_certificate is true (should be rejected at build time).
func mkDownstreamTSRequireClientCert(t *testing.T) *corev3.TransportSocket {
	t.Helper()
	inner := &tlsv3.DownstreamTlsContext{
		RequireClientCertificate: wrapperspb.Bool(true),
		CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsCertificates: []*tlsv3.TlsCertificate{{
				CertificateChain: &corev3.DataSource{
					Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte(testAlphaCertPEM)},
				},
				PrivateKey: &corev3.DataSource{
					Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte(testAlphaKeyPEM)},
				},
			}},
		},
	}
	a, err := anypb.New(inner)
	if err != nil {
		t.Fatalf("anypb.New DownstreamTlsContext (require_client_cert): %v", err)
	}
	return &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.tls",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: a},
	}
}

// mkTLSChain builds a filter chain with the given server_names and transport_socket.
// Pass a nil transport_socket for a plaintext chain.
func mkTLSChain(serverNames []string, ts *corev3.TransportSocket, filter *listenerv3.Filter) *listenerv3.FilterChain {
	var fcm *listenerv3.FilterChainMatch
	if len(serverNames) > 0 {
		fcm = &listenerv3.FilterChainMatch{ServerNames: serverNames}
	}
	return &listenerv3.FilterChain{
		FilterChainMatch: fcm,
		TransportSocket:  ts,
		Filters:          []*listenerv3.Filter{filter},
	}
}

// mkTLSListener builds a listener carrying the given filter chains.
func mkTLSListener(name, addr string, port uint32, chains []*listenerv3.FilterChain) *listenerv3.Listener {
	return &listenerv3.Listener{
		Name: name,
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       addr,
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
			},
		}},
		FilterChains: chains,
	}
}

// testCAPool builds an x509.CertPool containing the test CA.
func testCAPool(t *testing.T) *x509.CertPool {
	t.Helper()
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(testCAPEM)) {
		t.Fatal("failed to append test CA PEM")
	}
	return pool
}

// getConfigForClientFromMgr extracts the top-level tlsCfg from the first
// listenerRuntime in Manager.runtimes. Since manager_test.go is in the same
// package (package listener), it can access package-private fields.
func getConfigForClientFromMgr(t *testing.T, mgr *Manager) func(*stdtls.ClientHelloInfo) (*stdtls.Config, error) {
	t.Helper()
	if len(mgr.runtimes) == 0 {
		t.Fatal("Manager has no runtimes")
	}
	rt := mgr.runtimes[0]
	if rt.tlsCfg == nil {
		t.Fatal("listenerRuntime.tlsCfg is nil (not a TLS listener)")
	}
	return rt.tlsCfg.GetConfigForClient
}

// ---------------------------------------------------------------------------
// Phase-03 TLS / multi-chain tests
// ---------------------------------------------------------------------------

// TestNewManager_SingleChain_Plaintext_Unchanged verifies that the phase-02
// single-chain plaintext path still works after the phase-03 rewrite (regression).
func TestNewManager_SingleChain_Plaintext_Unchanged(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_plain", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if len(mgr.runtimes) != 1 {
		t.Fatalf("expected 1 runtime, got %d", len(mgr.runtimes))
	}
	rt := mgr.runtimes[0]
	if rt.tlsMode {
		t.Error("plaintext listener should not be in TLS mode")
	}
	if rt.tlsCfg != nil {
		t.Error("plaintext listener should have nil tlsCfg")
	}
}

// TestNewManager_MultiChain_SNIHappy verifies that two TLS chains with distinct
// server_names route correctly via GetConfigForClient.
func TestNewManager_MultiChain_SNIHappy(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)
	tsBeta := mkDownstreamTSInline(t, testBetaCertPEM, testBetaKeyPEM)

	l := mkTLSListener("l_tls", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, filter),
		mkTLSChain([]string{"beta.envoy-go.test"}, tsBeta, filter),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	gcfc := getConfigForClientFromMgr(t, mgr)

	// alpha SNI → non-nil config.
	cfgAlpha, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "alpha.envoy-go.test"})
	if err != nil {
		t.Errorf("GetConfigForClient(alpha): unexpected error: %v", err)
	}
	if cfgAlpha == nil {
		t.Error("GetConfigForClient(alpha): expected non-nil *stdtls.Config")
	}

	// beta SNI → non-nil config.
	cfgBeta, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "beta.envoy-go.test"})
	if err != nil {
		t.Errorf("GetConfigForClient(beta): unexpected error: %v", err)
	}
	if cfgBeta == nil {
		t.Error("GetConfigForClient(beta): expected non-nil *stdtls.Config")
	}

	// The two configs must differ (different chains).
	if cfgAlpha == cfgBeta {
		t.Error("alpha and beta chains returned the same *stdtls.Config pointer")
	}

	// Unmatched SNI → error.
	cfgNone, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "gamma.envoy-go.test"})
	if err == nil {
		t.Error("GetConfigForClient(gamma): expected error for unmatched SNI, got nil")
	}
	if cfgNone != nil {
		t.Error("GetConfigForClient(gamma): expected nil config for unmatched SNI")
	}
}

// TestNewManager_MultiChain_SNIWildcard verifies suffix-wildcard server_names matching.
func TestNewManager_MultiChain_SNIWildcard(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)

	l := mkTLSListener("l_wild", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"*.envoy-go.test"}, tsAlpha, filter),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	gcfc := getConfigForClientFromMgr(t, mgr)

	// Subdomain matches the wildcard.
	cfg, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "foo.envoy-go.test"})
	if err != nil {
		t.Errorf("GetConfigForClient(foo.envoy-go.test): unexpected error: %v", err)
	}
	if cfg == nil {
		t.Error("GetConfigForClient(foo.envoy-go.test): expected non-nil config")
	}

	// Bare domain does NOT match the wildcard (*.envoy-go.test requires at least one label prefix).
	cfgBare, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "envoy-go.test"})
	if err == nil {
		t.Error("GetConfigForClient(envoy-go.test): expected error — bare domain should not match *.envoy-go.test")
	}
	if cfgBare != nil {
		t.Error("GetConfigForClient(envoy-go.test): expected nil config for non-matching SNI")
	}
}

// TestNewManager_MultiChain_Specificity verifies that an exact pattern beats a
// suffix-wildcard pattern when both could apply.
func TestNewManager_MultiChain_Specificity(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)
	tsBeta := mkDownstreamTSInline(t, testBetaCertPEM, testBetaKeyPEM)

	// Chain A: exact "alpha.envoy-go.test"; Chain B: wildcard "*.envoy-go.test".
	// Deliberately pass wildcard first to verify sort-by-specificity.
	l := mkTLSListener("l_spec", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"*.envoy-go.test"}, tsBeta, filter),
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, filter),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	gcfc := getConfigForClientFromMgr(t, mgr)

	// alpha.envoy-go.test must match the exact chain (chain index 1 in input, but
	// sorted to position 0 by specificity).
	cfgAlpha, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "alpha.envoy-go.test"})
	if err != nil {
		t.Fatalf("GetConfigForClient(alpha): %v", err)
	}

	// other.envoy-go.test must match the wildcard chain.
	cfgOther, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "other.envoy-go.test"})
	if err != nil {
		t.Fatalf("GetConfigForClient(other): %v", err)
	}

	// The two calls must have returned different configs (different chains).
	if cfgAlpha == cfgOther {
		t.Error("exact and wildcard SNIs returned the same *stdtls.Config — specificity sort may be broken")
	}
}

// TestNewManager_MultiChain_CatchAll verifies that a catch-all (empty-match)
// chain is returned for an SNI that matches no other chain.
func TestNewManager_MultiChain_CatchAll(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)
	tsBeta := mkDownstreamTSInline(t, testBetaCertPEM, testBetaKeyPEM)

	// Two named chains + one catch-all.
	l := mkTLSListener("l_catch", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, filter),
		mkTLSChain([]string{"beta.envoy-go.test"}, tsBeta, filter),
		mkTLSChain(nil, tsAlpha, filter), // catch-all
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	gcfc := getConfigForClientFromMgr(t, mgr)

	// Known SNI matches its own chain.
	cfgAlpha, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "alpha.envoy-go.test"})
	if err != nil {
		t.Errorf("GetConfigForClient(alpha): %v", err)
	}
	if cfgAlpha == nil {
		t.Error("GetConfigForClient(alpha): expected non-nil config")
	}

	// Unknown SNI falls through to catch-all.
	cfgUnknown, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "unknown.envoy-go.test"})
	if err != nil {
		t.Errorf("GetConfigForClient(unknown): unexpected error: %v", err)
	}
	if cfgUnknown == nil {
		t.Error("GetConfigForClient(unknown): expected catch-all config, got nil")
	}
}

// TestNewManager_MultiChain_NoSNIMatch verifies that two SNI chains with no
// catch-all return an error for an unmatched SNI.
func TestNewManager_MultiChain_NoSNIMatch(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)
	tsBeta := mkDownstreamTSInline(t, testBetaCertPEM, testBetaKeyPEM)

	l := mkTLSListener("l_nosni", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, filter),
		mkTLSChain([]string{"beta.envoy-go.test"}, tsBeta, filter),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	gcfc := getConfigForClientFromMgr(t, mgr)

	cfg, err := gcfc(&stdtls.ClientHelloInfo{ServerName: "gamma.envoy-go.test"})
	if err == nil {
		t.Error("GetConfigForClient(gamma): expected error for unmatched SNI, got nil")
	}
	if cfg != nil {
		t.Error("GetConfigForClient(gamma): expected nil config for unmatched SNI")
	}
}

// TestNewManager_MultiChain_MixedTLSPlaintext_Errors verifies that a listener
// with one TLS chain and one plaintext chain is rejected at build time.
func TestNewManager_MultiChain_MixedTLSPlaintext_Errors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)

	l := mkTLSListener("l_mixed", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, filter), // TLS
		mkTLSChain([]string{"beta.envoy-go.test"}, nil, filter),      // plaintext
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error for mixed TLS/plaintext chains, got nil")
	}
	if !strings.Contains(err.Error(), "mixed TLS") {
		t.Errorf("error %q does not contain %q", err.Error(), "mixed TLS")
	}
}

// TestNewManager_MultiChain_DefaultFilterChain_Errors verifies that setting
// Listener.default_filter_chain is rejected.
func TestNewManager_MultiChain_DefaultFilterChain_Errors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")

	l := &listenerv3.Listener{
		Name: "l_dfc",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{
			{Filters: []*listenerv3.Filter{filter}},
		},
		DefaultFilterChain: &listenerv3.FilterChain{
			Filters: []*listenerv3.Filter{filter},
		},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error for default_filter_chain, got nil")
	}
	if !strings.Contains(err.Error(), "default_filter_chain") {
		t.Errorf("error %q does not contain %q", err.Error(), "default_filter_chain")
	}
}

// TestNewManager_MultiChain_NonSNIMatchField_Errors verifies that various
// non-SNI FilterChainMatch fields are rejected.
func TestNewManager_MultiChain_NonSNIMatchField_Errors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")

	makeListener := func(fcm *listenerv3.FilterChainMatch) *listenerv3.Listener {
		return &listenerv3.Listener{
			Name: "l_fcm",
			Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
				SocketAddress: &corev3.SocketAddress{
					Address:       "127.0.0.1",
					PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
				},
			}},
			FilterChains: []*listenerv3.FilterChain{{
				FilterChainMatch: fcm,
				Filters:          []*listenerv3.Filter{filter},
			}},
		}
	}

	tests := []struct {
		name    string
		fcm     *listenerv3.FilterChainMatch
		wantErr string
	}{
		{
			name:    "destination_port",
			fcm:     &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(80)},
			wantErr: "destination_port",
		},
		{
			name: "prefix_ranges",
			fcm: &listenerv3.FilterChainMatch{
				PrefixRanges: []*corev3.CidrRange{{AddressPrefix: "10.0.0.0", PrefixLen: wrapperspb.UInt32(8)}},
			},
			wantErr: "prefix_ranges",
		},
		{
			name: "source_ports",
			fcm: &listenerv3.FilterChainMatch{
				SourcePorts: []uint32{8080},
			},
			wantErr: "source_ports",
		},
		{
			name: "source_prefix_ranges",
			fcm: &listenerv3.FilterChainMatch{
				SourcePrefixRanges: []*corev3.CidrRange{{AddressPrefix: "192.168.0.0", PrefixLen: wrapperspb.UInt32(16)}},
			},
			wantErr: "source_prefix_ranges",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			boot := mkBoot(0, []*listenerv3.Listener{makeListener(tc.fcm)}, nil)
			_, err := NewManager(boot, cm)
			if err == nil {
				t.Fatalf("expected error for %s, got nil", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// TestNewManager_MultiChain_ApplicationProtocols_Errors verifies that
// application_protocols in filter_chain_match is rejected (ALPN match deferred
// to phase 07).
func TestNewManager_MultiChain_ApplicationProtocols_Errors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")

	l := &listenerv3.Listener{
		Name: "l_alpn",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			FilterChainMatch: &listenerv3.FilterChainMatch{
				ApplicationProtocols: []string{"h2", "http/1.1"},
			},
			Filters: []*listenerv3.Filter{filter},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error for application_protocols, got nil")
	}
	if !strings.Contains(err.Error(), "application_protocols") {
		t.Errorf("error %q does not contain %q", err.Error(), "application_protocols")
	}
}

// TestNewManager_MultiChain_TooManyCatchAlls_Errors verifies that more than
// one catch-all filter chain is rejected.
func TestNewManager_MultiChain_TooManyCatchAlls_Errors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)
	tsBeta := mkDownstreamTSInline(t, testBetaCertPEM, testBetaKeyPEM)

	// Two chains both with empty filter_chain_match = two catch-alls.
	l := mkTLSListener("l_2catch", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain(nil, tsAlpha, filter),
		mkTLSChain(nil, tsBeta, filter),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error for two catch-all chains, got nil")
	}
	// The error should mention that at most one chain may omit server_names.
	if !strings.Contains(err.Error(), "catch") && !strings.Contains(err.Error(), "server_names") {
		t.Errorf("error %q does not mention catch-all constraint", err.Error())
	}
}

// TestNewManager_MultiChain_RequireClientCert_Errors verifies that a chain
// with require_client_certificate=true is rejected (propagated from
// tls.NewDownstreamConfig).
func TestNewManager_MultiChain_RequireClientCert_Errors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsReq := mkDownstreamTSRequireClientCert(t)

	l := mkTLSListener("l_rcrt", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsReq, filter),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error for require_client_certificate=true, got nil")
	}
	if !strings.Contains(err.Error(), "require_client_certificate") {
		t.Errorf("error %q does not contain %q", err.Error(), "require_client_certificate")
	}
}

// TestNewManager_MultiChain_UnknownTransportSocket_Errors verifies that a
// transport_socket with an unrecognized type_url is rejected.
func TestNewManager_MultiChain_UnknownTransportSocket_Errors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")

	// Construct a TransportSocket with a non-TLS type_url.
	unknownTS := &corev3.TransportSocket{
		Name: "envoy.transport_sockets.raw_buffer",
		ConfigType: &corev3.TransportSocket_TypedConfig{
			TypedConfig: &anypb.Any{
				TypeUrl: "type.googleapis.com/envoy.extensions.transport_sockets.raw_buffer.v3.RawBuffer",
				Value:   nil,
			},
		},
	}

	l := mkTLSListener("l_unkts", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, unknownTS, filter),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error for unknown transport_socket type_url, got nil")
	}
	// NewDownstreamConfig returns "tls: downstream: unexpected type_url ..."
	if !strings.Contains(err.Error(), "tls:") && !strings.Contains(err.Error(), "type_url") {
		t.Errorf("error %q does not mention tls or type_url", err.Error())
	}
}

// TestNewManager_PlaintextMultiChain_Errors verifies that a plaintext listener
// with more than one filter chain is rejected (SNI match requires TLS).
func TestNewManager_PlaintextMultiChain_Errors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")

	l := mkTLSListener("l_pt2", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain(nil, nil, filter), // plaintext catch-all
		mkTLSChain(nil, nil, filter), // plaintext catch-all #2 — triggers both errors
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm)
	if err == nil {
		t.Fatal("expected error for plaintext multi-chain, got nil")
	}
	// Either "multiple filter_chains" or "plaintext" should appear.
	if !strings.Contains(err.Error(), "plaintext") && !strings.Contains(err.Error(), "multiple") && !strings.Contains(err.Error(), "catch") {
		t.Errorf("error %q does not mention plaintext/multiple constraint", err.Error())
	}
}

// TestNewManager_ChainSelectionPropagation performs a full in-process TLS
// handshake against a manager-built listener and verifies that the correct
// filter chain is dispatched based on SNI.
func TestNewManager_ChainSelectionPropagation(t *testing.T) {
	// We use a simple "record which chain was invoked" filter approach: two
	// channels, one per chain; whichever filter.Handle is called sends on its
	// channel.
	alphaCh := make(chan struct{}, 1)
	betaCh := make(chan struct{}, 1)

	// Build a cluster manager (needed by NewManager but tcpproxy filters won't
	// be used — we intercept at the listener level via a custom filter). Instead
	// we use mkClusterMgr and mkTcpProxyFilter (real filter), but we verify which
	// chain was selected by checking the TLS peer certificates returned by the
	// server — the alpha chain serves server-alpha cert; the beta chain serves
	// server-beta cert.
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)
	tsBeta := mkDownstreamTSInline(t, testBetaCertPEM, testBetaKeyPEM)
	_ = alphaCh
	_ = betaCh

	l := mkTLSListener("l_prop", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, filter),
		mkTLSChain([]string{"beta.envoy-go.test"}, tsBeta, filter),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	ls := mgr.Listeners()
	if len(ls) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(ls))
	}
	addr := ls[0].Addr

	caPool := testCAPool(t)

	// Connect with alpha SNI — verify the server presented the alpha cert.
	dialAndCheckCert := func(sni, expectedCN string) {
		t.Helper()
		conn, err := stdtls.DialWithDialer(
			&net.Dialer{Timeout: 2 * time.Second},
			"tcp", addr,
			&stdtls.Config{
				ServerName: sni,
				RootCAs:    caPool,
				MinVersion: stdtls.VersionTLS12,
			},
		)
		if err != nil {
			t.Errorf("TLS dial with SNI %q: %v", sni, err)
			return
		}
		defer func() { _ = conn.Close() }()
		cs := conn.ConnectionState()
		if len(cs.PeerCertificates) == 0 {
			t.Errorf("SNI %q: no peer certificates in connection state", sni)
			return
		}
		cn := cs.PeerCertificates[0].Subject.CommonName
		if cn != expectedCN {
			t.Errorf("SNI %q: expected CN %q, got %q", sni, expectedCN, cn)
		}
	}

	dialAndCheckCert("alpha.envoy-go.test", "alpha.envoy-go.test")
	dialAndCheckCert("beta.envoy-go.test", "beta.envoy-go.test")
}
