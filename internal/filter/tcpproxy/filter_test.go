package tcpproxy

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/internal/cluster"
)

const tcpProxyTypeURL = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// mkClusterMgr builds a cluster manager from a bootstrap with one STATIC
// cluster pointing at a single endpoint. Tests use this for happy-path setup.
func mkClusterMgr(t *testing.T, name, host string, port uint32) *cluster.Manager {
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
	f, err := NewFilter(any, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999))
	if err != nil {
		t.Fatalf("NewFilter: %v", err)
	}
	if f == nil {
		t.Fatal("NewFilter returned nil filter")
	}
}

func TestNewFilter_WrongTypeURL(t *testing.T) {
	_, err := NewFilter(&anypb.Any{TypeUrl: "type.googleapis.com/google.protobuf.StringValue", Value: nil}, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999))
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
	_, err := NewFilter(&anypb.Any{TypeUrl: tcpProxyTypeURL, Value: []byte{0xff, 0xff, 0xff, 0xff, 0xff}}, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999))
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
	_, err := NewFilter(any, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999))
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
	_, err := NewFilter(any, mkClusterMgr(t, "c_echo", "127.0.0.1", 9999))
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
	f, err := NewFilter(any, cm)
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
	f, err := NewFilter(any, cm)
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
