package listener

import (
	"context"
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
	if !strings.Contains(err.Error(), "expected exactly one filter_chain") {
		t.Errorf("error %q does not contain %q", err.Error(), "expected exactly one filter_chain")
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
	if !strings.Contains(err.Error(), "filter_chain_match") {
		t.Errorf("error %q does not contain %q", err.Error(), "filter_chain_match")
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
	if !strings.Contains(err.Error(), "transport_socket") {
		t.Errorf("error %q does not contain %q", err.Error(), "transport_socket")
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
