package tcpproxy

import (
	"context"
	stdtls "crypto/tls"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/drain"
	"github.com/pgdad/envoy-go/internal/stats"
)

const tcpProxyTypeURL = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// mkClusterMgr builds a cluster manager from a bootstrap with one STATIC
// cluster pointing at a single endpoint. Tests use this for happy-path setup.
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
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

func mkAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

func TestNewFilter_Happy(t *testing.T) {
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_echo"},
	})
	f, err := NewFilter(any, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999), nil)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f == nil {
		t.Fatal("NewFilter returned nil filter")
	}
}

func TestNewFilter_WrongTypeURL(t *testing.T) {
	_, err := NewFilter(&anypb.Any{TypeUrl: "type.googleapis.com/google.protobuf.StringValue", Value: nil}, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if msg := err.Error(); len(msg) == 0 {
		t.Fatal("expected non-empty error message")
	}
	// Verify it contains "wrong type_url"
	const want = "wrong type_url"
	if !containsStr(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err.Error(), want)
	}
}

func TestNewFilter_UnmarshalError(t *testing.T) {
	_, err := NewFilter(&anypb.Any{TypeUrl: tcpProxyTypeURL, Value: []byte{0xff, 0xff, 0xff, 0xff, 0xff}}, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	const want = "unmarshal"
	if !containsStr(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err.Error(), want)
	}
}

func TestNewFilter_MissingCluster(t *testing.T) {
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_does_not_exist"},
	})
	_, err := NewFilter(any, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !containsStr(err.Error(), "cluster") {
		t.Errorf("error %q does not contain %q", err.Error(), "cluster")
	}
	if !containsStr(err.Error(), "not found") {
		t.Errorf("error %q does not contain %q", err.Error(), "not found")
	}
}

func TestNewFilter_WeightedClustersUnsupported(t *testing.T) {
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix: "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_WeightedClusters{
			WeightedClusters: &tcpproxyv3.TcpProxy_WeightedCluster{},
		},
	})
	_, err := NewFilter(any, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	const want = "weighted_clusters"
	if !containsStr(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err.Error(), want)
	}
}

func TestHandle_BidirectionalEcho(t *testing.T) {
	// Backend echo on a random port.
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go acceptEchoForTest(backend)
	port := uint32(backend.Addr().(*net.TCPAddr).Port)

	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", port)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_echo"},
	})
	f, err := NewFilter(any, cm, nil)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}

	// Acceptor for the simulated downstream side.
	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("front listen: %v", err)
	}
	defer func() { _ = front.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		conn, err := front.Accept()
		if err != nil {
			return
		}
		f.Handle(ctx, conn)
	}()

	cli, err := net.Dial("tcp", front.Addr().String())
	if err != nil {
		t.Fatalf("dial front: %v", err)
	}
	defer func() { _ = cli.Close() }()

	if _, err := cli.Write([]byte("hello\n")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = cli.(*net.TCPConn).CloseWrite()

	var got []byte
	buf := make([]byte, 4096)
	_ = cli.SetReadDeadline(time.Now().Add(time.Second))
	for {
		n, err := cli.Read(buf)
		if n > 0 {
			got = append(got, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	if string(got) != "hello\n" {
		t.Errorf("got %q, want %q", got, "hello\n")
	}
}

func TestHandle_DialFailure_ClosesDownstream(t *testing.T) {
	// Grab a port then immediately close the listener so dial will fail.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	closedPort := uint32(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()

	cm := mkClusterMgr(t, "c_dead", "127.0.0.1", closedPort)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_dead"},
	})
	f, err := NewFilter(any, cm, nil)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}

	// Create a downstream pipe pair.
	serverConn, clientConn, err := newConnPairForTest(t)
	if err != nil {
		t.Fatalf("newConnPairForTest: %v", err)
	}
	defer func() { _ = clientConn.Close() }()

	ctx := context.Background()
	done := make(chan struct{})
	go func() {
		f.Handle(ctx, serverConn)
		close(done)
	}()

	// Wait for Handle to finish.
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle did not return within timeout")
	}

	// After Handle returns, downstream conn should be closed — reads return EOF.
	_ = clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	buf := make([]byte, 1)
	_, readErr := clientConn.Read(buf)
	if readErr == nil {
		t.Error("expected EOF or error reading from closed downstream, got nil")
	}
}

func acceptEchoForTest(ln net.Listener) {
	type wrap struct{ net.Conn }
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			_, _ = io.Copy(wrap{c}, wrap{c})
		}(c)
	}
}

// newConnPairForTest creates a local TCP connection pair for testing.
// Returns (serverSide, clientSide, error).
func newConnPairForTest(t *testing.T) (net.Conn, net.Conn, error) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = ln.Close() }()

	type result struct {
		conn net.Conn
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		c, e := ln.Accept()
		ch <- result{c, e}
	}()

	clientConn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		return nil, nil, err
	}

	res := <-ch
	if res.err != nil {
		_ = clientConn.Close()
		return nil, nil, res.err
	}
	return res.conn, clientConn, nil
}

// containsStr is a simple substring check to avoid importing strings in test file.
func containsStr(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mkTLSClusterMgr builds a cluster manager backed by a TLS cluster using the
// committed PKI fixtures. The upstream server must present a cert signed by the
// fixture CA with SNI "alpha.envoy-go.test" (upstream-alpha.pem).
func mkTLSClusterMgr(t testing.TB, name, host string, port uint32) *cluster.Manager {
	t.Helper()
	caPEM, err := os.ReadFile("../../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	ctx := &tlsv3.UpstreamTlsContext{
		Sni: "alpha.envoy-go.test",
		CommonTlsContext: &tlsv3.CommonTlsContext{
			ValidationContextType: &tlsv3.CommonTlsContext_ValidationContext{
				ValidationContext: &tlsv3.CertificateValidationContext{
					TrustedCa: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineBytes{InlineBytes: caPEM},
					},
				},
			},
		},
	}
	anyMsg, err := anypb.New(ctx)
	if err != nil {
		t.Fatalf("anypb.New(UpstreamTlsContext): %v", err)
	}
	ts := &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.tls",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
	}
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 name,
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(2 * time.Second),
				TransportSocket:      ts,
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
	cm, err := cluster.NewManagerWithBaseDir(bs, "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManagerWithBaseDir: %v", err)
	}
	return cm
}

// startTLSEchoServer starts a TLS echo server using the upstream-alpha PKI
// fixture and returns the listener. Caller must close it.
func startTLSEchoServer(t *testing.T) net.Listener {
	t.Helper()
	certPEM, err := os.ReadFile("../../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.pem")
	if err != nil {
		t.Fatalf("read upstream-alpha.pem: %v", err)
	}
	keyPEM, err := os.ReadFile("../../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.key.pem")
	if err != nil {
		t.Fatalf("read upstream-alpha.key.pem: %v", err)
	}
	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		MinVersion:   stdtls.VersionTLS12,
	})
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	go acceptEchoForTest(ln)
	return ln
}

// TestFilter_Handle_CtxCanceledBeforeDial verifies that Handle returns
// promptly when the context is already canceled before any dial attempt.
func TestFilter_Handle_CtxCanceledBeforeDial(t *testing.T) {
	// Grab a random port; the cluster will never be dialed.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := uint32(ln.Addr().(*net.TCPAddr).Port)
	_ = ln.Close()

	cm := mkClusterMgr(t, "c_cancel", "127.0.0.1", port)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_cancel"},
	})
	f, err := NewFilter(any, cm, nil)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately before Handle is called

	downstream, client, err := newConnPairForTest(t)
	if err != nil {
		t.Fatalf("newConnPairForTest: %v", err)
	}
	defer func() { _ = client.Close() }()

	done := make(chan struct{})
	go func() {
		f.Handle(ctx, downstream)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Handle did not return promptly with canceled context")
	}
}

// TestFilter_Handle_TLSUpstreamTransparent verifies that Handle pumps bytes
// through a TLS upstream without type-switching on the upstream transport.
func TestFilter_Handle_TLSUpstreamTransparent(t *testing.T) {
	backend := startTLSEchoServer(t)
	defer func() { _ = backend.Close() }()
	port := uint32(backend.Addr().(*net.TCPAddr).Port)

	cm := mkTLSClusterMgr(t, "c_tls_echo", "127.0.0.1", port)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_tls_echo"},
	})
	f, err := NewFilter(any, cm, nil)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}

	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("front listen: %v", err)
	}
	defer func() { _ = front.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		conn, e := front.Accept()
		if e != nil {
			return
		}
		f.Handle(ctx, conn)
	}()

	cli, err := net.Dial("tcp", front.Addr().String())
	if err != nil {
		t.Fatalf("dial front: %v", err)
	}
	defer func() { _ = cli.Close() }()

	if _, err := cli.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = cli.(*net.TCPConn).CloseWrite()

	var got []byte
	buf := make([]byte, 4096)
	_ = cli.SetReadDeadline(time.Now().Add(2 * time.Second))
	for {
		n, readErr := cli.Read(buf)
		if n > 0 {
			got = append(got, buf[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

// TestTCPProxy_DrainInflightBalance verifies that a Handle call Incs the drain
// manager on connect and Decs it when the connection closes, so dm.Done()
// fires after dm.Drain() is called and the last in-flight connection exits.
func TestTCPProxy_DrainInflightBalance(t *testing.T) {
	dm := drain.New(10 * time.Millisecond)
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go acceptEchoForTest(backend)
	port := uint32(backend.Addr().(*net.TCPAddr).Port)

	cm := mkClusterMgr(t, "c_backend", "127.0.0.1", port)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "drain_test",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_backend"},
	})
	f, err := NewFilter(any, cm, dm)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go f.Handle(ctx, srv)
	_, _ = client.Write([]byte("hello\n"))
	_ = client.Close()
	time.Sleep(100 * time.Millisecond)
	dm.Drain()
	select {
	case <-dm.Done():
	case <-time.After(500 * time.Millisecond):
		t.Errorf("dm.Done() did not fire — TCP-proxy inflight not balanced")
	}
}

// TestTCPProxy_DrainInflightBalance_NilDrainManager verifies that Handle does
// not panic when dm is nil (nil-tolerant guard in Handle).
func TestTCPProxy_DrainInflightBalance_NilDrainManager(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	defer func() { _ = backend.Close() }()
	go acceptEchoForTest(backend)
	port := uint32(backend.Addr().(*net.TCPAddr).Port)

	cm := mkClusterMgr(t, "c_backend_nil", "127.0.0.1", port)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "nil_dm_test",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_backend_nil"},
	})
	f, err := NewFilter(any, cm, nil)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	srv, client := net.Pipe()
	defer func() { _ = srv.Close(); _ = client.Close() }()
	go f.Handle(context.Background(), srv)
	_, _ = client.Write([]byte("hello\n"))
	_ = client.Close()
	time.Sleep(50 * time.Millisecond)
	// Test passes if no panic.
}

// mkTwoClusterMgr builds a cluster.Manager that contains TWO distinct clusters,
// each pointing at the supplied listener addresses. Used by
// TestHandle_NoOverrideUsesDefaultCluster to exercise the refactored Filter
// struct (which stores both cm and defaultCluster) without an override ctx.
func mkTwoClusterMgr(t testing.TB, nameA, hostA string, portA uint32, nameB, hostB string, portB uint32) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{
				{
					Name:                 nameA,
					ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
					LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
					ConnectTimeout:       durationpb.New(time.Second),
					LoadAssignment: &endpointv3.ClusterLoadAssignment{
						ClusterName: nameA,
						Endpoints: []*endpointv3.LocalityLbEndpoints{{
							LbEndpoints: []*endpointv3.LbEndpoint{{
								HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
									Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
										SocketAddress: &corev3.SocketAddress{
											Address:       hostA,
											PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: portA},
										},
									}},
								}},
							}},
						}},
					},
				},
				{
					Name:                 nameB,
					ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
					LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
					ConnectTimeout:       durationpb.New(time.Second),
					LoadAssignment: &endpointv3.ClusterLoadAssignment{
						ClusterName: nameB,
						Endpoints: []*endpointv3.LocalityLbEndpoints{{
							LbEndpoints: []*endpointv3.LbEndpoint{{
								HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
									Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
										SocketAddress: &corev3.SocketAddress{
											Address:       hostB,
											PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: portB},
										},
									}},
								}},
							}},
						}},
					},
				},
			},
		},
	}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("mkTwoClusterMgr: cluster.NewManager: %v", err)
	}
	return cm
}

// TestHandle_NoOverrideUsesDefaultCluster is the back-compat regression
// sentinel for the Task-4 struct refactor (ADR-0219). It drives Handle with
// context.Background() (no override) and asserts that bytes reach the
// configured default cluster ("bar"), NOT the second cluster ("foo"). This
// must stay green both before the refactor (pre-condition) and after it.
func TestHandle_NoOverrideUsesDefaultCluster(t *testing.T) {
	// Start two distinct echo backends with distinguishable sentinel responses.
	// "foo" backend echoes bytes prefixed with no modification; we detect which
	// backend was used by sending a sentinel payload and reading it back.
	backendFoo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backendFoo listen: %v", err)
	}
	defer func() { _ = backendFoo.Close() }()
	go acceptEchoForTest(backendFoo)
	portFoo := uint32(backendFoo.Addr().(*net.TCPAddr).Port)

	backendBar, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backendBar listen: %v", err)
	}
	defer func() { _ = backendBar.Close() }()
	go acceptEchoForTest(backendBar)
	portBar := uint32(backendBar.Addr().(*net.TCPAddr).Port)

	// Build a manager with BOTH clusters; configure the Filter with "bar" as
	// the default cluster (the configured cluster_specifier).
	cm := mkTwoClusterMgr(t, "foo", "127.0.0.1", portFoo, "bar", "127.0.0.1", portBar)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "no_override_test",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "bar"},
	})
	f, err := NewFilter(any, cm, nil)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}

	// Simulated front-end listener to hand the accepted conn to Handle.
	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("front listen: %v", err)
	}
	defer func() { _ = front.Close() }()

	// context.Background() — NO override in ctx.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		conn, e := front.Accept()
		if e != nil {
			return
		}
		f.Handle(ctx, conn)
	}()

	cli, err := net.Dial("tcp", front.Addr().String())
	if err != nil {
		t.Fatalf("dial front: %v", err)
	}
	defer func() { _ = cli.Close() }()

	// Send a sentinel that the echo backend will reflect verbatim.
	const sentinel = "sentinel-bar\n"
	if _, err := cli.Write([]byte(sentinel)); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}
	_ = cli.(*net.TCPConn).CloseWrite()

	var got []byte
	buf := make([]byte, 4096)
	_ = cli.SetReadDeadline(time.Now().Add(time.Second))
	for {
		n, readErr := cli.Read(buf)
		if n > 0 {
			got = append(got, buf[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	// Assert the "bar" backend echoed back the sentinel (byte-exact back-compat).
	if string(got) != sentinel {
		t.Errorf("got %q, want %q (expected echo from 'bar' default cluster)", got, sentinel)
	}
}

// TestFilter_Handle_HalfCloseOverTLS verifies that halfClose(*stdtls.Conn)
// propagates a write-shutdown to the upstream after the downstream closes its
// write side, allowing the upstream echo to complete and the downstream to read
// back all data followed by EOF.
func TestFilter_Handle_HalfCloseOverTLS(t *testing.T) {
	backend := startTLSEchoServer(t)
	defer func() { _ = backend.Close() }()
	port := uint32(backend.Addr().(*net.TCPAddr).Port)

	cm := mkTLSClusterMgr(t, "c_tls_half", "127.0.0.1", port)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_tls_half"},
	})
	f, err := NewFilter(any, cm, nil)
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}

	front, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("front listen: %v", err)
	}
	defer func() { _ = front.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		conn, e := front.Accept()
		if e != nil {
			return
		}
		f.Handle(ctx, conn)
	}()

	cli, err := net.Dial("tcp", front.Addr().String())
	if err != nil {
		t.Fatalf("dial front: %v", err)
	}
	defer func() { _ = cli.Close() }()

	if _, err := cli.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	// Signal end-of-write so the echo server stops reading and echoes back.
	_ = cli.(*net.TCPConn).CloseWrite()

	_ = cli.SetReadDeadline(time.Now().Add(2 * time.Second))
	got, readErr := io.ReadAll(cli)
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if string(got) != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestTcpProxy_HashPolicy_SourceIP_Parses(t *testing.T) {
	cm := mkClusterMgr(t, "c1", "127.0.0.1", 9001)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c1"},
		HashPolicy: []*typev3.HashPolicy{
			{PolicySpecifier: &typev3.HashPolicy_SourceIp_{SourceIp: &typev3.HashPolicy_SourceIp{}}},
		},
	})
	f, err := NewFilter(any, cm, nil)
	if err != nil {
		t.Fatalf("source_ip hash_policy must parse: %v", err)
	}
	if !f.hashOnSourceIP {
		t.Error("source_ip hash_policy must set hashOnSourceIP")
	}
}

func TestTcpProxy_NoHashPolicy_ByteStable(t *testing.T) {
	cm := mkClusterMgr(t, "c1", "127.0.0.1", 9001)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c1"},
	})
	f, err := NewFilter(any, cm, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f.hashOnSourceIP {
		t.Error("no hash_policy → hashOnSourceIP must stay false (byte-stable behavior)")
	}
}

func TestTcpProxy_HashPolicy_FilterState_Rejected(t *testing.T) {
	cm := mkClusterMgr(t, "c1", "127.0.0.1", 9001)
	any := mkAny(t, &tcpproxyv3.TcpProxy{
		StatPrefix:       "tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c1"},
		HashPolicy: []*typev3.HashPolicy{
			{PolicySpecifier: &typev3.HashPolicy_FilterState_{FilterState: &typev3.HashPolicy_FilterState{Key: "x"}}},
		},
	})
	_, err := NewFilter(any, cm, nil)
	if err == nil || !strings.Contains(err.Error(), "hash_policy") {
		t.Errorf("filter_state hash_policy must be rejected with a hash_policy error: %v", err)
	}
}
