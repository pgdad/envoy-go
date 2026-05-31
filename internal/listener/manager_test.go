package listener

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	drv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/direct_response/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	networkrbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/drain"
	filter_http "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filter/http/router"
	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/filter/network/builtins"
	"github.com/esalaine/envoy-go/internal/filter/network/directresponse"
	"github.com/esalaine/envoy-go/internal/filter/network/echo"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter"
	"github.com/esalaine/envoy-go/internal/listener/listenerfilter/tls_inspector"
	"github.com/esalaine/envoy-go/internal/stats"
)

// testHTTPRegistry returns a freshly-allocated, frozen *filter_http.HTTPRegistry
// containing only the router terminal filter. Used by every NewManager*
// call site in this test file post-Task-14 to satisfy the
// "boot-populated, frozen" ADR-0072 contract; tcpproxy-only listeners ignore
// the registry but it is still threaded for uniformity.
func testHTTPRegistry() *filter_http.HTTPRegistry {
	r := filter_http.NewHTTPRegistry()
	r.Register(router.TypeURL, router.New)
	r.Freeze()
	return r
}

// testLFRegistry returns a freshly-allocated, frozen
// *listenerfilter.ListenerFilterRegistry containing only tls_inspector. Used
// by NewManager* call sites post-Task-9 to satisfy the boot-populated,
// frozen ADR-0079 contract; listeners with no listener_filters[] ignore the
// registry but it is still threaded for uniformity. Task 11 will replicate
// this pattern in cmd/envoy-go/main.go.
func testLFRegistry() *listenerfilter.ListenerFilterRegistry {
	r := listenerfilter.NewListenerFilterRegistry()
	r.Register(tls_inspector.TypeURL, tls_inspector.New)
	r.Freeze()
	return r
}

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
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
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

	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Phase-03 (ADR-0033): two plaintext catch-all chains → "catch-all" error or
	// "plaintext listener with multiple filter_chains" error. Either is valid.
	if !strings.Contains(err.Error(), "filter_chain") {
		t.Errorf("error %q does not contain %q", err.Error(), "filter_chain")
	}
}

// TestManager_NonEmptyFilterChainMatch_DestinationPort_Accepted verifies the
// ADR-0078 supersession of ADR-0033 clause 2: destination_port is now an
// accepted FilterChainMatch dimension and parses without error.
func TestManager_NonEmptyFilterChainMatch_DestinationPort_Accepted(t *testing.T) {
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

	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager(destination_port=80): %v (expected nil after ADR-0078)", err)
	}
	if len(mgr.runtimes) != 1 || len(mgr.runtimes[0].chainSpecs) != 1 {
		t.Fatalf("expected 1 runtime with 1 chainSpec, got %d/%d", len(mgr.runtimes), len(mgr.runtimes[0].chainSpecs))
	}
	if got := mgr.runtimes[0].chainSpecs[0].DestinationPort; got != 80 {
		t.Errorf("chainSpecs[0].DestinationPort = %d, want 80", got)
	}
}

// TestManager_Error_TwoFilters: a chain carrying two terminal filters
// [tcp_proxy, tcp_proxy]. Phase-26.2 Task 10 retired the old single-filter rule
// ("expected exactly one filter"); the unified builder now permits multi-filter
// chains but rejects a second terminal with network-filter-multiple-terminals.
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "network-filter-multiple-terminals") {
		t.Errorf("error %q does not contain %q", err.Error(), "network-filter-multiple-terminals")
	}
}

// mkRBACNetworkFilter builds a minimal valid rbac_network filter config (only
// stat_prefix is required; a wholly-inactive engine is a valid passthrough
// read filter). The Name matches upstream's canonical name; resolution is by
// type_url, so Name is cosmetic.
func mkRBACNetworkFilter(t *testing.T) *listenerv3.Filter {
	t.Helper()
	a, err := anypb.New(&networkrbacv3.RBAC{StatPrefix: "lis_rbac"})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return &listenerv3.Filter{Name: "envoy.filters.network.rbac", ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: a}}
}

// TestBuildNetworkChainFactory_RBACThenTCPProxy_MixedChain is the boot-smoke for
// the FIRST production mixed read→terminal network chain: [rbac_network,
// tcp_proxy]. It exercises the SOLE chain-build path (buildNetworkChainFactory)
// fed by a registry populated through builtins.RegisterBuiltins (which now wires
// rbac_network as the 5th built-in), and proves:
//
//  1. the chain BUILDS with no chain-shape reject (no network-filter-terminal-not-last,
//     no network-filter-multiple-terminals) — the 26.2 framework lifted the
//     mixed-chain-unsupported reject; this proves a real consumer flows through it; and
//  2. the chain CLASSIFIES correctly: filters[0] is a read-prefix rbac_network
//     (a network.ReadFilter, NOT a TerminalFilter) and filters[1] is the terminal
//     tcp_proxy (a network.TerminalFilter, last in the chain).
func TestBuildNetworkChainFactory_RBACThenTCPProxy_MixedChain(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	netReg := network.NewRegistry()
	builtins.RegisterBuiltins(netReg, builtins.Deps{ClusterManager: cm, StatsRegistry: stats.NewRegistry()})
	netReg.Freeze()

	filters := []*listenerv3.Filter{mkRBACNetworkFilter(t), mkTcpProxyFilter(t, "c_echo")}
	factory, err := buildNetworkChainFactory("test", filters, netReg, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("mixed [rbac_network, tcp_proxy] chain must build with no chain-shape reject: %v", err)
	}
	if factory == nil {
		t.Fatal("nil netChainFactory for a valid mixed chain")
	}

	// Classify a sample chain: rbac_network is a read-prefix (read, not terminal);
	// tcp_proxy is the single terminal and is last.
	chain := factory()
	if len(chain) != 2 {
		t.Fatalf("chain length = %d; want 2", len(chain))
	}
	if _, isTerminal := chain[0].(network.TerminalFilter); isTerminal {
		t.Error("filters[0] (rbac_network) must NOT be a terminal filter — it is a read-prefix")
	}
	if _, isRead := chain[0].(network.ReadFilter); !isRead {
		t.Error("filters[0] (rbac_network) must be a read filter (read-prefix)")
	}
	if _, isTerminal := chain[1].(network.TerminalFilter); !isTerminal {
		t.Error("filters[1] (tcp_proxy) must be the terminal filter (last in the chain)")
	}
	if _, isRead := chain[1].(network.ReadFilter); isRead {
		t.Error("filters[1] (tcp_proxy) must NOT be a read filter — it is a pure terminal")
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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
				Name: "envoy.filters.network.does_not_exist",
				ConfigType: &listenerv3.Filter_TypedConfig{
					TypedConfig: &anypb.Any{
						TypeUrl: "type.googleapis.com/envoy.extensions.filters.network.does_not_exist.v3.Nope",
						Value:   nil,
					},
				},
			}},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

// selectByServerNameFromMgr is the phase-07.2 Task-10 replacement for the
// pre-refactor getConfigForClientFromMgr helper. It runs the unified
// chain-match algorithm (listenerfilter.SelectChain) over the first
// listenerRuntime's chainSpecs / defaultSpec given an SNI input, and returns
// the per-chain *stdtls.Config of the winning chain. This mirrors the
// production-code path (serveConnection step 5) without requiring a real
// network handshake. Returns (nil, error) if no chain matches.
//
// Pre-refactor the helper extracted rt.tlsCfg.GetConfigForClient — that
// callback no longer exists because chain selection now happens BEFORE the
// handshake (the chain's *stdtls.Config is passed directly to stdtls.Server).
func selectByServerNameFromMgr(t *testing.T, mgr *Manager, sni string) (*stdtls.Config, error) {
	t.Helper()
	if len(mgr.runtimes) == 0 {
		t.Fatal("Manager has no runtimes")
	}
	rt := mgr.runtimes[0]
	inputs := listenerfilter.ChainMatchInputs{ServerName: sni, TransportProtocol: "tls"}
	spec, err := listenerfilter.SelectChain(inputs, rt.chainSpecs, rt.defaultSpec)
	if err != nil {
		// Surface a stable error prefix so existing tests' regex on
		// `^listener:` continues to hold.
		return nil, fmt.Errorf("listener: %q: no filter_chain matches SNI %q", rt.name, sni)
	}
	ci := rt.chainByName[spec.Name]
	if ci == nil {
		t.Fatalf("chainByName[%q] missing (logic bug)", spec.Name)
	}
	return ci.tlsCfg, nil
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
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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
	// Phase-07.2 Task 10: the listener-level tlsCfg field is gone (each chainInfo
	// carries its own per-chain *stdtls.Config). For a plaintext listener every
	// chainInfo's tlsCfg must be nil.
	for name, ci := range rt.chainByName {
		if ci.tlsCfg != nil {
			t.Errorf("plaintext chain %q has non-nil tlsCfg", name)
		}
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
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// alpha SNI → non-nil config.
	cfgAlpha, err := selectByServerNameFromMgr(t, mgr, "alpha.envoy-go.test")
	if err != nil {
		t.Errorf("SelectChain(alpha): unexpected error: %v", err)
	}
	if cfgAlpha == nil {
		t.Error("SelectChain(alpha): expected non-nil *stdtls.Config")
	}

	// beta SNI → non-nil config.
	cfgBeta, err := selectByServerNameFromMgr(t, mgr, "beta.envoy-go.test")
	if err != nil {
		t.Errorf("SelectChain(beta): unexpected error: %v", err)
	}
	if cfgBeta == nil {
		t.Error("SelectChain(beta): expected non-nil *stdtls.Config")
	}

	// The two configs must differ (different chains).
	if cfgAlpha == cfgBeta {
		t.Error("alpha and beta chains returned the same *stdtls.Config pointer")
	}

	// Unmatched SNI → error.
	cfgNone, err := selectByServerNameFromMgr(t, mgr, "gamma.envoy-go.test")
	if err == nil {
		t.Error("SelectChain(gamma): expected error for unmatched SNI, got nil")
	}
	if cfgNone != nil {
		t.Error("SelectChain(gamma): expected nil config for unmatched SNI")
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
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Subdomain matches the wildcard.
	cfg, err := selectByServerNameFromMgr(t, mgr, "foo.envoy-go.test")
	if err != nil {
		t.Errorf("SelectChain(foo.envoy-go.test): unexpected error: %v", err)
	}
	if cfg == nil {
		t.Error("SelectChain(foo.envoy-go.test): expected non-nil config")
	}

	// Bare domain does NOT match the wildcard (*.envoy-go.test requires at least one label prefix).
	cfgBare, err := selectByServerNameFromMgr(t, mgr, "envoy-go.test")
	if err == nil {
		t.Error("SelectChain(envoy-go.test): expected error — bare domain should not match *.envoy-go.test")
	}
	if cfgBare != nil {
		t.Error("SelectChain(envoy-go.test): expected nil config for non-matching SNI")
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
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// alpha.envoy-go.test must match the exact chain (selected by SNI
	// specificity sub-ordering within the equal chain-spec score).
	cfgAlpha, err := selectByServerNameFromMgr(t, mgr, "alpha.envoy-go.test")
	if err != nil {
		t.Fatalf("SelectChain(alpha): %v", err)
	}

	// other.envoy-go.test must match the wildcard chain.
	cfgOther, err := selectByServerNameFromMgr(t, mgr, "other.envoy-go.test")
	if err != nil {
		t.Fatalf("SelectChain(other): %v", err)
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
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	// Known SNI matches its own chain.
	cfgAlpha, err := selectByServerNameFromMgr(t, mgr, "alpha.envoy-go.test")
	if err != nil {
		t.Errorf("SelectChain(alpha): %v", err)
	}
	if cfgAlpha == nil {
		t.Error("SelectChain(alpha): expected non-nil config")
	}

	// Unknown SNI falls through to catch-all.
	cfgUnknown, err := selectByServerNameFromMgr(t, mgr, "unknown.envoy-go.test")
	if err != nil {
		t.Errorf("SelectChain(unknown): unexpected error: %v", err)
	}
	if cfgUnknown == nil {
		t.Error("SelectChain(unknown): expected catch-all config, got nil")
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
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	cfg, err := selectByServerNameFromMgr(t, mgr, "gamma.envoy-go.test")
	if err == nil {
		t.Fatal("SelectChain(gamma): expected error for unmatched SNI, got nil")
	}
	if cfg != nil {
		t.Error("SelectChain(gamma): expected nil config for unmatched SNI")
	}
	// Project error-prefix discipline: every error crossing a package boundary
	// begins with "<package>: ". The selectByServerNameFromMgr helper
	// re-wraps the SelectChain error to preserve the `listener:` prefix the
	// pre-refactor GetConfigForClient callback emitted natively.
	if !strings.HasPrefix(err.Error(), "listener:") {
		t.Errorf("error %q does not start with %q (project error-prefix discipline)", err.Error(), "listener:")
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error for mixed TLS/plaintext chains, got nil")
	}
	if !strings.Contains(err.Error(), "mixed TLS") {
		t.Errorf("error %q does not contain %q", err.Error(), "mixed TLS")
	}
}

// TestParseDefaultFilterChainNoLongerErrors verifies the ADR-0078 supersession
// of ADR-0033 clause 3: default_filter_chain is honored at parse — the
// runtime's defaultSpec/defaultChain are populated.
func TestParseDefaultFilterChainNoLongerErrors(t *testing.T) {
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

	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager(default_filter_chain): %v (expected nil after ADR-0078)", err)
	}
	rt := mgr.runtimes[0]
	if rt.defaultSpec == nil {
		t.Error("listenerRuntime.defaultSpec is nil; expected populated default ChainSpec")
	}
	if rt.defaultChain == nil {
		t.Error("listenerRuntime.defaultChain is nil; expected populated default chainInfo")
	}
}

// TestParseChainSpecAcceptsAllEightDimensions verifies the ADR-0078
// supersession of ADR-0033 clause 2: every chain-match dimension Envoy v1.37.2
// documents is accepted by parseChainSpec without error and surfaces on the
// constructed ChainSpec.
func TestParseChainSpecAcceptsAllEightDimensions(t *testing.T) {
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
		name string
		fcm  *listenerv3.FilterChainMatch
		// check inspects the parsed ChainSpec for the test's dimension.
		check func(t *testing.T, spec *listenerfilter.ChainSpec)
	}{
		{
			name: "destination_port",
			fcm:  &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(80)},
			check: func(t *testing.T, s *listenerfilter.ChainSpec) {
				if s.DestinationPort != 80 {
					t.Errorf("DestinationPort = %d, want 80", s.DestinationPort)
				}
			},
		},
		{
			name: "prefix_ranges",
			fcm: &listenerv3.FilterChainMatch{
				PrefixRanges: []*corev3.CidrRange{{AddressPrefix: "10.0.0.0", PrefixLen: wrapperspb.UInt32(8)}},
			},
			check: func(t *testing.T, s *listenerfilter.ChainSpec) {
				if len(s.PrefixRanges) != 1 {
					t.Errorf("PrefixRanges len = %d, want 1", len(s.PrefixRanges))
				}
			},
		},
		{
			name: "source_prefix_ranges",
			fcm: &listenerv3.FilterChainMatch{
				SourcePrefixRanges: []*corev3.CidrRange{{AddressPrefix: "192.168.0.0", PrefixLen: wrapperspb.UInt32(16)}},
			},
			check: func(t *testing.T, s *listenerfilter.ChainSpec) {
				if len(s.SourcePrefixRanges) != 1 {
					t.Errorf("SourcePrefixRanges len = %d, want 1", len(s.SourcePrefixRanges))
				}
			},
		},
		{
			name: "source_type_LOCAL",
			fcm:  &listenerv3.FilterChainMatch{SourceType: listenerv3.FilterChainMatch_SAME_IP_OR_LOOPBACK},
			check: func(t *testing.T, s *listenerfilter.ChainSpec) {
				if !s.SourceTypeLocal {
					t.Error("SourceTypeLocal = false, want true")
				}
			},
		},
		{
			name: "source_ports",
			fcm:  &listenerv3.FilterChainMatch{SourcePorts: []uint32{8080}},
			check: func(t *testing.T, s *listenerfilter.ChainSpec) {
				if len(s.SourcePorts) != 1 || s.SourcePorts[0] != 8080 {
					t.Errorf("SourcePorts = %v, want [8080]", s.SourcePorts)
				}
			},
		},
		{
			name: "application_protocols",
			fcm:  &listenerv3.FilterChainMatch{ApplicationProtocols: []string{"h2", "http/1.1"}},
			check: func(t *testing.T, s *listenerfilter.ChainSpec) {
				if len(s.ApplicationProtocols) != 2 {
					t.Errorf("ApplicationProtocols len = %d, want 2", len(s.ApplicationProtocols))
				}
			},
		},
		{
			name: "transport_protocol_raw_buffer",
			fcm:  &listenerv3.FilterChainMatch{TransportProtocol: "raw_buffer"},
			check: func(t *testing.T, s *listenerfilter.ChainSpec) {
				if s.TransportProtocol != "raw_buffer" {
					t.Errorf("TransportProtocol = %q, want %q", s.TransportProtocol, "raw_buffer")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			boot := mkBoot(0, []*listenerv3.Listener{makeListener(tc.fcm)}, nil)
			mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
			if err != nil {
				t.Fatalf("NewManager(%s): %v (expected nil)", tc.name, err)
			}
			if len(mgr.runtimes) != 1 || len(mgr.runtimes[0].chainSpecs) != 1 {
				t.Fatalf("expected 1 runtime/1 chainSpec, got %d/%d", len(mgr.runtimes), len(mgr.runtimes[0].chainSpecs))
			}
			tc.check(t, mgr.runtimes[0].chainSpecs[0])
		})
	}
}

// TestParseChainSpecSilentlyIgnoresDirectSourcePrefixRanges verifies that
// `direct_source_prefix_ranges` (the proxy-protocol original-source dimension)
// is silently skipped at parse per SPEC §12 — no error, no field on the
// ChainSpec.
func TestParseChainSpecSilentlyIgnoresDirectSourcePrefixRanges(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_dspr",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			FilterChainMatch: &listenerv3.FilterChainMatch{
				DirectSourcePrefixRanges: []*corev3.CidrRange{{AddressPrefix: "10.0.0.0", PrefixLen: wrapperspb.UInt32(8)}},
			},
			Filters: []*listenerv3.Filter{filter},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v (direct_source_prefix_ranges should be silently skipped)", err)
	}
	spec := mgr.runtimes[0].chainSpecs[0]
	if !spec.Empty {
		t.Errorf("ChainSpec.Empty = false, want true (only direct_source_prefix_ranges set, which is silently skipped)")
	}
}

// TestParseChainSpecRejectsUnknownTransportProtocol verifies that the
// transport_protocol enum domain is enforced at parse: only "tls",
// "raw_buffer", or "" are accepted.
func TestParseChainSpecRejectsUnknownTransportProtocol(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_tp",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			FilterChainMatch: &listenerv3.FilterChainMatch{TransportProtocol: "quic"},
			Filters:          []*listenerv3.Filter{filter},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error for transport_protocol=quic, got nil")
	}
	if !strings.Contains(err.Error(), `transport_protocol "quic"`) {
		t.Errorf("error %q does not name the bad value", err.Error())
	}
}

// TestNewManager_MultiChain_ApplicationProtocols_Accepted verifies that
// application_protocols in filter_chain_match is accepted post-ADR-0078.
// (Phase 03's parse-time error is superseded by ADR-0078 clause 2.)
func TestNewManager_MultiChain_ApplicationProtocols_Accepted(t *testing.T) {
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

	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager(application_protocols): %v (expected nil after ADR-0078)", err)
	}
	if got := mgr.runtimes[0].chainSpecs[0].ApplicationProtocols; len(got) != 2 {
		t.Errorf("chainSpecs[0].ApplicationProtocols = %v, want 2 elements", got)
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
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

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error for unknown transport_socket type_url, got nil")
	}
	// NewDownstreamConfig returns "tls: downstream: unexpected type_url ..."
	if !strings.Contains(err.Error(), "tls:") && !strings.Contains(err.Error(), "type_url") {
		t.Errorf("error %q does not mention tls or type_url", err.Error())
	}
}

// TestNewManager_PlaintextMultiChain_WithSNI_Errors verifies the
// ADR-0078 clause-6 caveat: a plaintext listener with multiple chains where
// at least one chain populates server_names[] is still rejected — SNI cannot
// match on plaintext.
func TestNewManager_PlaintextMultiChain_WithSNI_Errors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")

	l := mkTLSListener("l_pt2", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, nil, filter), // plaintext + SNI
		mkTLSChain(nil, nil, filter),                             // plaintext catch-all
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error for plaintext multi-chain with SNI, got nil")
	}
	if !strings.Contains(err.Error(), "plaintext") {
		t.Errorf("error %q does not mention plaintext SNI constraint", err.Error())
	}
}

// TestNewManager_PlaintextMultiChain_NonSNIDimensions_Accepted verifies that a
// plaintext listener with multiple chains differing on non-SNI dimensions is
// now ACCEPTED (ADR-0078 clause-6 partial supersession). The rule was: "no
// multi-chain plaintext"; the rule is now: "no SNI-bearing multi-chain
// plaintext".
func TestNewManager_PlaintextMultiChain_NonSNIDimensions_Accepted(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")

	l := &listenerv3.Listener{
		Name: "l_pt_dst",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{
			{
				FilterChainMatch: &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(8080)},
				Filters:          []*listenerv3.Filter{filter},
			},
			{
				FilterChainMatch: &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(8081)},
				Filters:          []*listenerv3.Filter{filter},
			},
		},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager(plaintext multi-chain on dest_port): %v (expected nil after ADR-0078)", err)
	}
	if len(mgr.runtimes[0].chainSpecs) != 2 {
		t.Errorf("chainSpecs len = %d, want 2", len(mgr.runtimes[0].chainSpecs))
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
	// Phase-07.2 Task 10: SNI is populated only by an explicit tls_inspector
	// listener filter (per ADR-0079). The pre-Task-10 dispatch path used a
	// hard-wired GetConfigForClient callback; the unified path requires the
	// bootstrap to declare the filter so SelectChain sees inputs.ServerName.
	l.ListenerFilters = []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
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

// ---------------------------------------------------------------------------
// Phase-04 HCM registration tests
// ---------------------------------------------------------------------------

// mkRouterAny builds a google.protobuf.Any wrapping an empty Router proto.
func mkRouterAny(t *testing.T) *anypb.Any {
	t.Helper()
	a, err := anypb.New(&routerv3.Router{})
	if err != nil {
		t.Fatal(err)
	}
	return a
}

// mkHCMFilter builds a listenerv3.Filter carrying a minimal valid HCM
// typed_config with a direct_response /health → 200 OK route.
func mkHCMFilter(t *testing.T) *listenerv3.Filter {
	t.Helper()
	hcmProto := &hcmv3.HttpConnectionManager{
		CodecType:  hcmv3.HttpConnectionManager_HTTP1,
		StatPrefix: "ingress_http",
		RouteSpecifier: &hcmv3.HttpConnectionManager_RouteConfig{
			RouteConfig: &routev3.RouteConfiguration{
				VirtualHosts: []*routev3.VirtualHost{{
					Name:    "vh_default",
					Domains: []string{"*"},
					Routes: []*routev3.Route{{
						Match: &routev3.RouteMatch{PathSpecifier: &routev3.RouteMatch_Path{Path: "/health"}},
						Action: &routev3.Route_DirectResponse{DirectResponse: &routev3.DirectResponseAction{
							Status: 200,
							Body:   &corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "OK\n"}},
						}},
					}},
				}},
			},
		},
		HttpFilters: []*hcmv3.HttpFilter{{
			Name:       "envoy.filters.http.router",
			ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: mkRouterAny(t)},
		}},
	}
	a, err := anypb.New(hcmProto)
	if err != nil {
		t.Fatalf("anypb.New HCM: %v", err)
	}
	return &listenerv3.Filter{
		Name:       "envoy.filters.network.http_connection_manager",
		ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: a},
	}
}

// TestNewManager_HCMRegistration verifies that a listener using the
// HCM type_url is accepted by NewManager — the HCM network-filter factory is
// registered into netReg via the builtins seam (Task 9/10).
func TestNewManager_HCMRegistration(t *testing.T) {
	cm := mkClusterMgr(t, "c_test", "127.0.0.1", 1)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_http", "127.0.0.1", 0, mkHCMFilter(t)),
	}, nil)
	if _, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry()); err != nil {
		t.Fatalf("NewManager with HCM listener: %v", err)
	}
}

// mkHCMHTTP2Filter builds a listenerv3.Filter carrying a minimal valid HCM
// typed_config with codec_type=HTTP2 and a direct_response /health route.
// Used by the Task-11 allowH2C plumbing tests.
func mkHCMHTTP2Filter(t *testing.T) *listenerv3.Filter {
	t.Helper()
	hcmProto := &hcmv3.HttpConnectionManager{
		CodecType:  hcmv3.HttpConnectionManager_HTTP2,
		StatPrefix: "ingress_h2",
		RouteSpecifier: &hcmv3.HttpConnectionManager_RouteConfig{
			RouteConfig: &routev3.RouteConfiguration{
				VirtualHosts: []*routev3.VirtualHost{{
					Name:    "vh_default",
					Domains: []string{"*"},
					Routes: []*routev3.Route{{
						Match: &routev3.RouteMatch{PathSpecifier: &routev3.RouteMatch_Path{Path: "/health"}},
						Action: &routev3.Route_DirectResponse{DirectResponse: &routev3.DirectResponseAction{
							Status: 200,
							Body:   &corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "OK\n"}},
						}},
					}},
				}},
			},
		},
		HttpFilters: []*hcmv3.HttpFilter{{
			Name:       "envoy.filters.http.router",
			ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: mkRouterAny(t)},
		}},
	}
	a, err := anypb.New(hcmProto)
	if err != nil {
		t.Fatalf("anypb.New HCM HTTP2: %v", err)
	}
	return &listenerv3.Filter{
		Name:       "envoy.filters.network.http_connection_manager",
		ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: a},
	}
}

// ---------------------------------------------------------------------------
// Phase-05.1 Task 11: --allow-h2c plumbing tests
// ---------------------------------------------------------------------------

// TestNewManagerWithBaseDirAndAllowH2C_HTTP2OnPlaintextWithAllow verifies that
// NewManagerWithBaseDirAndAllowH2C with allowH2C=true accepts a plaintext
// listener with HCM codec_type=HTTP2. The HCM constructor remaps
// HTTP2→AUTO so the phase-04 validator passes; Task 12 replaces the stub.
func TestNewManagerWithBaseDirAndAllowH2C_HTTP2OnPlaintextWithAllow(t *testing.T) {
	cm := mkClusterMgr(t, "c_test", "127.0.0.1", 1)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_h2c", "127.0.0.1", 0, mkHCMHTTP2Filter(t)),
	}, nil)
	m, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", true /* allowH2C */, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err != nil {
		t.Fatalf("NewManagerWithBaseDirAndAllowH2C(allowH2C=true) = %v, want nil", err)
	}
	_ = m
}

// TestNewManagerWithBaseDirAndAllowH2C_HTTP2OnPlaintextWithoutAllow verifies
// that NewManagerWithBaseDirAndAllowH2C with allowH2C=false rejects a plaintext
// listener with HCM codec_type=HTTP2 with an error containing
// "codec_type HTTP2 requires TLS".
//
// SKIP: The validation logic ("codec_type HTTP2 requires TLS") lives in
// hcm.parseFilterWithCtx, which is Task 12's work. Remove this skip in Task 12
// once parseFilterWithCtx enforces the constraint.
func TestNewManagerWithBaseDirAndAllowH2C_HTTP2OnPlaintextWithoutAllow(t *testing.T) {
	cm := mkClusterMgr(t, "c_test", "127.0.0.1", 1)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_h2c", "127.0.0.1", 0, mkHCMHTTP2Filter(t)),
	}, nil)
	_, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false /* no allow */, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err == nil {
		t.Fatal("NewManagerWithBaseDirAndAllowH2C(allowH2C=false) accepted plaintext+HTTP2; want error")
	}
	if !strings.Contains(err.Error(), "codec_type HTTP2 requires TLS") {
		t.Errorf("error = %q, want substring 'codec_type HTTP2 requires TLS'", err.Error())
	}
}

// TestNewManager_BackwardsCompat_DefaultsAllowH2CFalse verifies that the
// existing NewManager + NewManagerWithBaseDir constructors delegate to
// NewManagerWithBaseDirAndAllowH2C with allowH2C=false. A TLS+HTTP2 bootstrap
// still builds correctly (TLS satisfies the requirement; allowH2C is
// irrelevant on the TLS path).
func TestNewManager_BackwardsCompat_DefaultsAllowH2CFalse(t *testing.T) {
	cm := mkClusterMgr(t, "c_test", "127.0.0.1", 1)
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)
	l := mkTLSListener("l_h2tls", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain(nil, tsAlpha, mkHCMHTTP2Filter(t)),
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	m, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager = %v, want nil (TLS+HTTP2 path)", err)
	}
	_ = m
}

// TestNewManager_HCMBuildErrorWrapsAsListenerFilter verifies that a parse
// error from the HCM network-filter factory is wrapped with the standard
// listener prefix plus the per-filter index segment introduced by the unified
// builder: listener: "<name>": filter_chains[<i>]: filters[<j>]: hcm: ...
func TestNewManager_HCMBuildErrorWrapsAsListenerFilter(t *testing.T) {
	// HTTP2 codec_type is the cheapest trigger for an hcm: error.
	hcmProto := &hcmv3.HttpConnectionManager{
		CodecType:  hcmv3.HttpConnectionManager_HTTP2,
		StatPrefix: "x",
	}
	hcmAny, err := anypb.New(hcmProto)
	if err != nil {
		t.Fatal(err)
	}
	cm := mkClusterMgr(t, "c_test", "127.0.0.1", 1)
	bs := mkBoot(0, []*listenerv3.Listener{{
		Name: "l_http",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{{
				Name:       "envoy.filters.network.http_connection_manager",
				ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: hcmAny},
			}},
		}},
	}}, nil)
	_, buildErr := NewManager(bs, cm, stats.NewRegistry(), testHTTPRegistry())
	if buildErr == nil {
		t.Fatal("expected build error, got nil")
	}
	// The error must be wrapped: listener: "l_http": filter_chains[0]: filters[0]: hcm: codec_type HTTP2 ...
	want := `listener: "l_http": filter_chains[0]: filters[0]: hcm: codec_type HTTP2`
	if !strings.Contains(buildErr.Error(), want) {
		t.Errorf("error %q does not contain %q", buildErr.Error(), want)
	}
}

// TestListenerManager_AllocatesTwoMetricsPerListener verifies the listener-side
// per-listener metric-allocation loop registers exactly the 2 listener-scope
// metrics from SPEC §6 (`listener.<addr>.downstream_cx_total` counter,
// `listener.<addr>.downstream_cx_active` gauge) on the supplied Registry at
// boot time. The address segment is the bound (post-resolve) bind address
// normalized per the planner-pinned `strings.NewReplacer(":", "_", ".", "_")`
// shape so that the resulting name has exactly three top-level dot-segments
// (`listener.<addr>.<rest>`) — necessary for `flattenToProm`'s SN3 single-dot
// split to extract the address as a label.
//
// PLAN-deviation note: the PLAN's example test code walks the Registry directly
// after NewManager. The implementation registers at Start time (post-bind)
// instead, because (a) SPEC §6's <addr> is the BIND address — pre-bind the
// configured port may be 0 ("OS-pick"), and (b) two listeners with configured
// `:0` would collide on the same registered name pre-resolve. The test calls
// Start before walking — which is also what the differential fixture (Task 14)
// does, so the test mirrors the production scrape ordering.
func TestListenerManager_AllocatesTwoMetricsPerListener(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_h1", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	r := stats.NewRegistry()
	lm, err := NewManager(boot, cm, r, testHTTPRegistry())
	if err != nil {
		t.Fatalf("listener.NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := lm.Start(ctx); err != nil {
		t.Fatalf("listener.Start: %v", err)
	}
	defer lm.Stop()
	// Walk the registry; the listener.<addr>.downstream_cx_{total,active}
	// names must be present.
	var seen []string
	r.Walk(func(m stats.Metric) { seen = append(seen, m.Name()) })
	wantSubstr := []string{".downstream_cx_total", ".downstream_cx_active"}
	for _, w := range wantSubstr {
		found := false
		for _, n := range seen {
			if strings.HasPrefix(n, "listener.") && strings.HasSuffix(n, w) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing listener.<addr>%s metric (seen=%v)", w, seen)
		}
	}
}

// ---------------------------------------------------------------------------
// Phase-07.2 Task 9: listener_filters[] + ADR-0078 + ambiguous-selection tests
// ---------------------------------------------------------------------------

// mkTLSInspectorFilter returns a listenerv3.ListenerFilter wrapping an empty
// (default-config) tls_inspector typed_config. Used by Task-9 tests that
// exercise listener_filters[] resolution through the registry.
//
// The Any is constructed directly with the canonical tls_inspector type_url
// and a nil Value. The tls_inspector parser tolerates a nil typed_config
// Value (it returns the default config — see internal/listener/listenerfilter/tls_inspector/proto.go).
func mkTLSInspectorFilter(_ *testing.T) *listenerv3.ListenerFilter {
	return &listenerv3.ListenerFilter{
		Name: "envoy.filters.listener.tls_inspector",
		ConfigType: &listenerv3.ListenerFilter_TypedConfig{
			TypedConfig: &anypb.Any{TypeUrl: tls_inspector.TypeURL},
		},
	}
}

// TestParseListenerFiltersResolvesViaRegistry verifies that a Listener
// carrying `listener_filters: [{tls_inspector}]` is resolved through the
// threaded *ListenerFilterRegistry — the per-connection FilterInstanceFactory
// is captured on listenerRuntime.listenerFilterFactories.
func TestParseListenerFiltersResolvesViaRegistry(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_lf",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:    []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{filter}}},
		ListenerFilters: []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := len(mgr.runtimes[0].listenerFilterFactories); got != 1 {
		t.Errorf("listenerFilterFactories len = %d, want 1", got)
	}
}

// TestParseListenerFiltersUnknownTypeURLErrors verifies that an unknown
// listener-filter type_url errors with the documented message format.
func TestParseListenerFiltersUnknownTypeURLErrors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	bogus := &anypb.Any{TypeUrl: "type.googleapis.com/foo", Value: nil}
	l := &listenerv3.Listener{
		Name: "name",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{filter}}},
		ListenerFilters: []*listenerv3.ListenerFilter{{
			Name:       "envoy.filters.listener.unknown",
			ConfigType: &listenerv3.ListenerFilter_TypedConfig{TypedConfig: bogus},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err == nil {
		t.Fatal("expected error for unknown listener-filter type_url, got nil")
	}
	want := `listener: "name": listener_filters[0]: unknown filter type_url "type.googleapis.com/foo"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err.Error(), want)
	}
}

// TestParseListenerFiltersTimeoutInRange verifies that a 5s
// listener_filters_timeout parses to lfTimeoutMs=5000.
func TestParseListenerFiltersTimeoutInRange(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_to",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:           []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{filter}}},
		ListenerFiltersTimeout: durationpb.New(5 * time.Second),
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := mgr.runtimes[0].lfTimeoutMs; got != 5000 {
		t.Errorf("lfTimeoutMs = %d, want 5000", got)
	}
}

// TestParseListenerFiltersTimeoutDefault verifies the unset/nil
// listener_filters_timeout defaults to 15000ms (ADR-0082).
func TestParseListenerFiltersTimeoutDefault(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_def", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if got := mgr.runtimes[0].lfTimeoutMs; got != 15000 {
		t.Errorf("lfTimeoutMs default = %d, want 15000", got)
	}
}

// TestParseListenerFiltersTimeoutBelowFloorErrors verifies that a 500ms
// listener_filters_timeout errors with the [1s, 60s] envelope message.
func TestParseListenerFiltersTimeoutBelowFloorErrors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "name",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:           []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{filter}}},
		ListenerFiltersTimeout: durationpb.New(500 * time.Millisecond),
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error for listener_filters_timeout=500ms, got nil")
	}
	want := `listener: "name": listener_filters_timeout 500ms is outside the supported [1s, 60s] envelope`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err.Error(), want)
	}
}

// TestParseListenerFiltersTimeoutAboveCapErrors verifies that a 90s
// listener_filters_timeout errors with the same envelope-violation format.
func TestParseListenerFiltersTimeoutAboveCapErrors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "name",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:           []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{filter}}},
		ListenerFiltersTimeout: durationpb.New(90 * time.Second),
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error for listener_filters_timeout=90s, got nil")
	}
	want := `listener: "name": listener_filters_timeout 1m30s is outside the supported [1s, 60s] envelope`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err.Error(), want)
	}
}

// TestParseChainSpecMixedTLSPreserved verifies that ADR-0033 clause 5
// (preserved by ADR-0078): mixed TLS + plaintext WITHIN filter_chains[] is
// rejected unless server_names disambiguate. The phase-03 test already
// covers the mixed-TLS error message; this test re-asserts the post-ADR-0078
// preservation.
func TestParseChainSpecMixedTLSPreserved(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)

	l := mkTLSListener("l_mix", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, filter), // TLS
		mkTLSChain([]string{"beta.envoy-go.test"}, nil, filter),      // plaintext
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error for mixed TLS+plaintext in filter_chains[], got nil")
	}
	if !strings.Contains(err.Error(), "mixed TLS") {
		t.Errorf("error %q does not contain %q", err.Error(), "mixed TLS")
	}
}

// TestIdenticalFilterChainsErrorWithAmbiguousSelection verifies the
// ADR-0081 final-tie rule: two chains with identical filter_chain_match
// shapes error at NewManager-build with an `ambiguous selection` message.
func TestIdenticalFilterChainsErrorWithAmbiguousSelection(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	// Two chains differing on terminal-filter only, identical match shape
	// (DestinationPort=8080).
	fcm := &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(8080)}
	l := &listenerv3.Listener{
		Name: "l_ambig",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{
			{FilterChainMatch: fcm, Filters: []*listenerv3.Filter{filter}},
			{FilterChainMatch: fcm, Filters: []*listenerv3.Filter{filter}},
		},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error for identical filter_chain_match, got nil")
	}
	if !strings.Contains(err.Error(), "ambiguous selection") {
		t.Errorf("error %q does not contain %q", err.Error(), "ambiguous selection")
	}
}

// TestParseDefaultFilterChain_Plaintext_WithTLSFilterChain verifies the
// ADR-0080 + ADR-0078 clause-5 caveat: default_filter_chain may have an
// independent TLS posture from filter_chains[]. Specifically a TLS
// filter_chains[] entry + a plaintext default_filter_chain coexist.
func TestParseDefaultFilterChain_Plaintext_WithTLSFilterChain(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)

	l := &listenerv3.Listener{
		Name: "l_tls_plus_plain_default",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{
			mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, filter),
		},
		DefaultFilterChain: &listenerv3.FilterChain{
			Filters: []*listenerv3.Filter{filter}, // plaintext (no transport_socket)
		},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v (TLS chains + plaintext default coexistence per ADR-0080)", err)
	}
	if mgr.runtimes[0].defaultChain == nil {
		t.Error("defaultChain not populated")
	}
	if mgr.runtimes[0].defaultChain != nil && mgr.runtimes[0].defaultChain.tlsCfg != nil {
		t.Error("defaultChain.tlsCfg should be nil (plaintext default)")
	}
}

// TestIdenticalFilterChainsAmbiguousSelectionDetectedDespiteSlicePermutation
// is a code-review-driven follow-up on Task 9: chainSpecKey must canonicalize
// (sort) every multi-element ChainSpec field before serializing, because
// chainmatch.matches() is set-based on every multi-element dimension
// (sniMatchAny / alpnMatchAny / portInAny / ipInAny). Without canonicalization,
// two chains differing only in declared slice order would produce different
// keys → boot-time duplicate detection misses them → at runtime SelectChain
// hits the ambiguous-tie path on the first matching connection (worse failure
// mode than a boot-time error). Each subtest exercises one multi-element
// dimension's permutation and asserts it is detected as a duplicate at boot.
//
// SNI / ALPN cases use TLS chains because plaintext+SNI on a multi-chain
// listener is rejected earlier (not the path under test); other cases stay
// plaintext because they do not depend on a TLS-only dimension.
func TestIdenticalFilterChainsAmbiguousSelectionDetectedDespiteSlicePermutation(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)

	type plainCase struct {
		fcmA *listenerv3.FilterChainMatch
		fcmB *listenerv3.FilterChainMatch
	}
	plainCases := map[string]plainCase{
		"SourcePorts permutation": {
			fcmA: &listenerv3.FilterChainMatch{SourcePorts: []uint32{1000, 2000}},
			fcmB: &listenerv3.FilterChainMatch{SourcePorts: []uint32{2000, 1000}},
		},
		"PrefixRanges permutation": {
			fcmA: &listenerv3.FilterChainMatch{PrefixRanges: []*corev3.CidrRange{
				{AddressPrefix: "10.0.0.0", PrefixLen: wrapperspb.UInt32(8)},
				{AddressPrefix: "192.168.0.0", PrefixLen: wrapperspb.UInt32(16)},
			}},
			fcmB: &listenerv3.FilterChainMatch{PrefixRanges: []*corev3.CidrRange{
				{AddressPrefix: "192.168.0.0", PrefixLen: wrapperspb.UInt32(16)},
				{AddressPrefix: "10.0.0.0", PrefixLen: wrapperspb.UInt32(8)},
			}},
		},
		"SourcePrefixRanges permutation": {
			fcmA: &listenerv3.FilterChainMatch{SourcePrefixRanges: []*corev3.CidrRange{
				{AddressPrefix: "10.0.0.0", PrefixLen: wrapperspb.UInt32(8)},
				{AddressPrefix: "172.16.0.0", PrefixLen: wrapperspb.UInt32(12)},
			}},
			fcmB: &listenerv3.FilterChainMatch{SourcePrefixRanges: []*corev3.CidrRange{
				{AddressPrefix: "172.16.0.0", PrefixLen: wrapperspb.UInt32(12)},
				{AddressPrefix: "10.0.0.0", PrefixLen: wrapperspb.UInt32(8)},
			}},
		},
	}
	for name, tc := range plainCases {
		t.Run(name, func(t *testing.T) {
			l := &listenerv3.Listener{
				Name: "l_perm",
				Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
					SocketAddress: &corev3.SocketAddress{
						Address:       "127.0.0.1",
						PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
					},
				}},
				FilterChains: []*listenerv3.FilterChain{
					{FilterChainMatch: tc.fcmA, Filters: []*listenerv3.Filter{filter}},
					{FilterChainMatch: tc.fcmB, Filters: []*listenerv3.Filter{filter}},
				},
			}
			boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
			_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
			if err == nil {
				t.Fatalf("expected ambiguous-selection error for permuted %s, got nil", name)
			}
			if !strings.Contains(err.Error(), "ambiguous selection") {
				t.Errorf("error %q does not contain %q (permuted %s)", err.Error(), "ambiguous selection", name)
			}
		})
	}

	type tlsCase struct {
		fcmA *listenerv3.FilterChainMatch
		fcmB *listenerv3.FilterChainMatch
	}
	tlsCases := map[string]tlsCase{
		"ServerNames permutation": {
			fcmA: &listenerv3.FilterChainMatch{ServerNames: []string{"a.example", "b.example"}},
			fcmB: &listenerv3.FilterChainMatch{ServerNames: []string{"b.example", "a.example"}},
		},
		"ApplicationProtocols permutation": {
			fcmA: &listenerv3.FilterChainMatch{ApplicationProtocols: []string{"h2", "http/1.1"}},
			fcmB: &listenerv3.FilterChainMatch{ApplicationProtocols: []string{"http/1.1", "h2"}},
		},
	}
	for name, tc := range tlsCases {
		t.Run(name, func(t *testing.T) {
			l := &listenerv3.Listener{
				Name: "l_perm_tls",
				Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
					SocketAddress: &corev3.SocketAddress{
						Address:       "127.0.0.1",
						PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
					},
				}},
				FilterChains: []*listenerv3.FilterChain{
					{FilterChainMatch: tc.fcmA, TransportSocket: tsAlpha, Filters: []*listenerv3.Filter{filter}},
					{FilterChainMatch: tc.fcmB, TransportSocket: tsAlpha, Filters: []*listenerv3.Filter{filter}},
				},
			}
			boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
			_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
			if err == nil {
				t.Fatalf("expected ambiguous-selection error for permuted %s, got nil", name)
			}
			if !strings.Contains(err.Error(), "ambiguous selection") {
				t.Errorf("error %q does not contain %q (permuted %s)", err.Error(), "ambiguous selection", name)
			}
		})
	}
}

// TestParseDefaultFilterChainBuildErrorIsSinglePrefixed asserts the
// default_filter_chain build error carries the
// `listener: %q: default_filter_chain: ` prefix exactly once. Phase-26.2
// Task 10 retired the old buildTerminalFilter path (and its
// errUnwrapFilterChain double-prefix peeler); buildNetworkChainFactory now
// emits the full single prefix directly via dfcNetPrefix. This test exercises
// the path with a default_filter_chain carrying two terminal filters
// ([tcp_proxy, tcp_proxy] → network-filter-multiple-terminals reject) and
// asserts the prefix appears exactly once.
func TestParseDefaultFilterChainBuildErrorIsSinglePrefixed(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_dfc_two",
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
			// Two terminal filters → network-filter-multiple-terminals reject.
			Filters: []*listenerv3.Filter{filter, filter},
		},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err == nil {
		t.Fatal("expected error from default_filter_chain with two terminal filters, got nil")
	}
	const marker = "default_filter_chain: "
	if got := strings.Count(err.Error(), marker); got != 1 {
		t.Errorf("error %q contains %q %d times, want exactly 1 (single-prefix regression)", err.Error(), marker, got)
	}
	if !strings.Contains(err.Error(), "network-filter-multiple-terminals") {
		t.Errorf("error %q does not contain the expected reject text", err.Error())
	}
}

// TestParseListenerFiltersNilRegistryErrors is a code-review-driven follow-up
// on Task 9: a Listener carrying listener_filters[] but compiled against a
// nil ListenerFilterRegistry must error with the documented message — the
// path is otherwise reachable but untested.
func TestParseListenerFiltersNilRegistryErrors(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	l := &listenerv3.Listener{
		Name: "l_nilreg",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains:    []*listenerv3.FilterChain{{Filters: []*listenerv3.Filter{filter}}},
		ListenerFilters: []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	_, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), nil, nil, nil, testNetRegistryWithTerminals(t, cm))
	if err == nil {
		t.Fatal("expected error for non-empty listener_filters[] with nil lfRegistry, got nil")
	}
	want := `listener: "l_nilreg": listener_filters[] is non-empty but no listener-filter registry was supplied`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error %q does not contain %q", err.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// Phase-07.2 Task 10 — unified pre/post-handshake dispatch
// ---------------------------------------------------------------------------

// startTaggedBackend launches a tiny TCP "echo+tag" backend on 127.0.0.1:0.
// Each accepted connection has the supplied tag byte sent immediately; the
// remainder of the connection echoes whatever the client sends back. The
// caller-side dialer reads the first byte to identify which backend (and thus
// which filter_chain) handled the connection.
func startTaggedBackend(t *testing.T, tag byte) (addr *net.TCPAddr, cleanup func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("backend listen: %v", err)
	}
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			go func(conn net.Conn) {
				defer func() { _ = conn.Close() }()
				_, _ = conn.Write([]byte{tag})
				buf := make([]byte, 256)
				for {
					if _, rerr := conn.Read(buf); rerr != nil {
						return
					}
				}
			}(c)
		}
	}()
	return ln.Addr().(*net.TCPAddr), func() { _ = ln.Close() }
}

// readByteWithTimeout reads exactly one byte from c with a deadline; returns
// the byte or fails the test on timeout / read error.
func readByteWithTimeout(t *testing.T, c net.Conn, d time.Duration) byte {
	t.Helper()
	if err := c.SetReadDeadline(time.Now().Add(d)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 1)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n != 1 {
		t.Fatalf("read: got %d bytes, want 1", n)
	}
	return buf[0]
}

// twoClusterMgr builds a cluster.Manager with TWO STATIC clusters pointing at
// two distinct host:port pairs (typically two tagged backends). Used by the
// Task-10 unified-dispatch tests to attribute a connection to its filter_chain
// via the tag byte the upstream backend sends on accept.
func twoClusterMgr(t *testing.T, names []string, hosts []string, ports []uint32) *cluster.Manager {
	t.Helper()
	if len(names) != 2 || len(hosts) != 2 || len(ports) != 2 {
		t.Fatalf("twoClusterMgr: want 2 entries, got %d/%d/%d", len(names), len(hosts), len(ports))
	}
	clusters := make([]*clusterv3.Cluster, 2)
	for i := range names {
		clusters[i] = &clusterv3.Cluster{
			Name:                 names[i],
			ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
			LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
			ConnectTimeout:       durationpb.New(time.Second),
			LoadAssignment: &endpointv3.ClusterLoadAssignment{
				ClusterName: names[i],
				Endpoints: []*endpointv3.LocalityLbEndpoints{{
					LbEndpoints: []*endpointv3.LbEndpoint{{
						HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
							Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
								SocketAddress: &corev3.SocketAddress{
									Address:       hosts[i],
									PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: ports[i]},
								},
							}},
						}},
					}},
				}},
			},
		}
	}
	bs := &bootstrapv3.Bootstrap{StaticResources: &bootstrapv3.Bootstrap_StaticResources{Clusters: clusters}}
	cm, err := cluster.NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("cluster.NewManager: %v", err)
	}
	return cm
}

// TestUnifiedDispatchPlaintextChainSelectByDestPort verifies the Task-10
// unified dispatch path: a plaintext listener with two filter_chains that
// differ only on `destination_port` routes connections by the listener-side
// local port the OS assigned. To exercise the dimension, we build TWO
// listeners (one per port) sharing a single Manager but each carries the
// 2-chain shape — chain matching the listener's bound port goes to backend A;
// the other chain (port-mismatched) is unreachable. We assert tag bytes A vs B
// flow through the correct chain.
//
// Test design note: a single listener can only be bound to one port, so
// "destination_port" semantically narrows to "this listener's port" — the
// chain whose destination_port equals the bound port matches; the other chain
// is dead code. We exploit that by binding two listeners with port=0 (OS
// picks) and consulting the resolved port post-bind, then re-running with
// hard-coded port mocks where the chain-match dimension actually filters.
//
// SIMPLER DESIGN ADOPTED: bind one listener with port=0, then verify the
// chain whose destination_port == resolved.Port wins; the other chain (set to
// resolved.Port+1) is the loser. This exercises the dimension at chain-match
// time without requiring two real binds.
func TestUnifiedDispatchPlaintextChainSelectByDestPort(t *testing.T) {
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	addrB, cleanB := startTaggedBackend(t, 'B')
	defer cleanB()

	cm := twoClusterMgr(t,
		[]string{"c_a", "c_b"},
		[]string{"127.0.0.1", "127.0.0.1"},
		[]uint32{uint32(addrA.Port), uint32(addrB.Port)},
	)

	// Phase 1: bind the listener with port=0 to learn the resolved port. We
	// can't pre-populate destination_port on the chain spec until we know it.
	probeLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	resolvedPort := uint32(probeLn.Addr().(*net.TCPAddr).Port)
	_ = probeLn.Close()

	// Build the listener bootstrap with two chains:
	//   chain A: destination_port = resolvedPort   → cluster c_a (tag 'A')
	//   chain B: destination_port = resolvedPort+1 → cluster c_b (tag 'B') [loser]
	chainAFilter := mkTcpProxyFilter(t, "c_a")
	chainBFilter := mkTcpProxyFilter(t, "c_b")
	l := &listenerv3.Listener{
		Name: "l_destport",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: resolvedPort},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{
			{
				FilterChainMatch: &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(resolvedPort)},
				Filters:          []*listenerv3.Filter{chainAFilter},
			},
			{
				FilterChainMatch: &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(resolvedPort + 1)},
				Filters:          []*listenerv3.Filter{chainBFilter},
			},
		},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	infos := mgr.Listeners()
	if len(infos) != 1 {
		t.Fatalf("Listeners: want 1, got %d", len(infos))
	}

	// Dial → expect the 'A' tag byte (chain A's backend).
	conn, err := net.DialTimeout("tcp", infos[0].Addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	got := readByteWithTimeout(t, conn, 2*time.Second)
	if got != 'A' {
		t.Errorf("tag byte = %q, want 'A' (chain A's backend; chain-match by destination_port broken)", got)
	}
}

// TestUnifiedDispatchTLSWithSNI verifies the Task-10 unified dispatch path
// for TLS: a TLS listener with a `tls_inspector` listener filter populates
// `inputs.ServerName` from the ClientHello, then chain-match selects on
// `server_names`, then the handshake runs against the selected chain's TLS
// config. We assert two distinct SNIs route to two distinct upstream
// backends (tag 'A' vs tag 'B').
func TestUnifiedDispatchTLSWithSNI(t *testing.T) {
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	addrB, cleanB := startTaggedBackend(t, 'B')
	defer cleanB()

	cm := twoClusterMgr(t,
		[]string{"c_a", "c_b"},
		[]string{"127.0.0.1", "127.0.0.1"},
		[]uint32{uint32(addrA.Port), uint32(addrB.Port)},
	)

	chainAFilter := mkTcpProxyFilter(t, "c_a")
	chainBFilter := mkTcpProxyFilter(t, "c_b")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)
	tsBeta := mkDownstreamTSInline(t, testBetaCertPEM, testBetaKeyPEM)

	l := mkTLSListener("l_sni_unified", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, chainAFilter),
		mkTLSChain([]string{"beta.envoy-go.test"}, tsBeta, chainBFilter),
	})
	l.ListenerFilters = []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	addr := mgr.Listeners()[0].Addr
	caPool := testCAPool(t)

	dialAndCheckTag := func(sni string, wantTag byte) {
		t.Helper()
		conn, derr := stdtls.DialWithDialer(
			&net.Dialer{Timeout: 2 * time.Second},
			"tcp", addr,
			&stdtls.Config{
				ServerName: sni,
				RootCAs:    caPool,
				MinVersion: stdtls.VersionTLS12,
			},
		)
		if derr != nil {
			t.Fatalf("TLS dial SNI=%q: %v", sni, derr)
		}
		defer func() { _ = conn.Close() }()
		got := readByteWithTimeout(t, conn, 2*time.Second)
		if got != wantTag {
			t.Errorf("SNI=%q tag = %q, want %q", sni, got, wantTag)
		}
		// Sanity: server cert CN must match SNI.
		if cn := conn.ConnectionState().PeerCertificates[0].Subject.CommonName; cn != sni {
			t.Errorf("SNI=%q: peer cert CN = %q, want SNI", sni, cn)
		}
	}
	dialAndCheckTag("alpha.envoy-go.test", 'A')
	dialAndCheckTag("beta.envoy-go.test", 'B')
}

// TestUnifiedDispatchDefaultFilterChainFallback verifies the Task-10 unified
// dispatch path's `default_filter_chain` fallback: a listener with
// `filter_chains[]` matching only on a specific dest_port + a
// `default_filter_chain` routes a connection on a different (non-matching)
// dimension into the default. Because a single listener binds one port, we
// exercise the fallback by setting the specific chain's destination_port to a
// value the listener was NOT bound on (resolvedPort+1), forcing the
// matchless connection into the default chain.
func TestUnifiedDispatchDefaultFilterChainFallback(t *testing.T) {
	addrSpecific, cleanS := startTaggedBackend(t, 'S')
	defer cleanS()
	addrDefault, cleanD := startTaggedBackend(t, 'D')
	defer cleanD()

	cm := twoClusterMgr(t,
		[]string{"c_specific", "c_default"},
		[]string{"127.0.0.1", "127.0.0.1"},
		[]uint32{uint32(addrSpecific.Port), uint32(addrDefault.Port)},
	)

	probeLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("probe listen: %v", err)
	}
	resolvedPort := uint32(probeLn.Addr().(*net.TCPAddr).Port)
	_ = probeLn.Close()

	specificFilter := mkTcpProxyFilter(t, "c_specific")
	defaultFilter := mkTcpProxyFilter(t, "c_default")
	l := &listenerv3.Listener{
		Name: "l_default_fallback",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: resolvedPort},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{
			{
				FilterChainMatch: &listenerv3.FilterChainMatch{DestinationPort: wrapperspb.UInt32(resolvedPort + 1)},
				Filters:          []*listenerv3.Filter{specificFilter},
			},
		},
		DefaultFilterChain: &listenerv3.FilterChain{
			Filters: []*listenerv3.Filter{defaultFilter},
		},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	conn, err := net.DialTimeout("tcp", mgr.Listeners()[0].Addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	got := readByteWithTimeout(t, conn, 2*time.Second)
	if got != 'D' {
		t.Errorf("tag byte = %q, want 'D' (default_filter_chain fallback broken)", got)
	}
}

// slowListenerFilter is a stub ListenerFilter that blocks in Inspect for
// `block` duration before returning Continue. Used to exercise the
// listener-filter pipeline timeout behavior in Task-10 unified dispatch.
type slowListenerFilter struct{ block time.Duration }

func (s *slowListenerFilter) Inspect(ctx context.Context, _ listenerfilter.Peeker, _ *listenerfilter.ChainMatchInputs) (listenerfilter.ListenerFilterStatus, error) {
	select {
	case <-time.After(s.block):
		return listenerfilter.Continue, nil
	case <-ctx.Done():
		return listenerfilter.Continue, ctx.Err()
	}
}
func (s *slowListenerFilter) OnDestroy() {}

// installSlowListenerFilter overwrites rt.listenerFilterFactories with a
// single factory that emits slowListenerFilter{block} per connection. Used
// after NewManager has built the real factories so the Task-10 timeout test
// can swap a deterministically-slow stand-in for tls_inspector. The Pipeline
// (per ADR-0079) has lfTimeoutMs already set; the test re-points the
// factories slice directly because the Manager has no public swap-API.
func installSlowListenerFilter(rt *listenerRuntime, block time.Duration) {
	rt.listenerFilterFactories = []listenerfilter.FilterInstanceFactory{
		func() listenerfilter.ListenerFilter { return &slowListenerFilter{block: block} },
	}
}

// TestUnifiedDispatchListenerFilterTimeoutAbortsConnection verifies that the
// Task-10 unified dispatch aborts the connection when a listener filter
// exceeds the pipeline timeout AND `continue_on_listener_filters_timeout`
// defaults to false (per ADR-0082). We install a slowListenerFilter that
// sleeps 2s and a 1s pipeline timeout; the dialer's Read should return EOF
// (conn closed by the listener) within ~2s.
func TestUnifiedDispatchListenerFilterTimeoutAbortsConnection(t *testing.T) {
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	filter := mkTcpProxyFilter(t, "c_a")

	l := &listenerv3.Listener{
		Name: "l_lf_timeout_abort",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{
			{Filters: []*listenerv3.Filter{filter}},
		},
		// 1s pipeline timeout (ADR-0082 floor); slowListenerFilter blocks 2s.
		ListenerFiltersTimeout: durationpb.New(1 * time.Second),
		// continue_on_listener_filters_timeout defaults to false (zero value).
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// Swap the (empty) listener-filter factory slice for a slow stand-in BEFORE
	// Start so the accept loop sees the override on the first connection.
	installSlowListenerFilter(mgr.runtimes[0], 2*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	conn, err := net.DialTimeout("tcp", mgr.Listeners()[0].Addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// The listener should close the conn after the pipeline aborts (~1s).
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline: %v", err)
	}
	buf := make([]byte, 1)
	n, rerr := conn.Read(buf)
	if rerr == nil && n > 0 {
		t.Errorf("expected conn closed by listener (timeout abort), got %d bytes %q", n, buf[:n])
	}
	// The exact error is platform-dependent (EOF, ECONNRESET, …); any non-nil
	// err with n==0 is acceptable evidence the listener aborted.
}

// TestUnifiedDispatchListenerFilterTimeoutContinue verifies that the Task-10
// unified dispatch falls through to chain-match on partial inputs when a
// listener filter times out AND `continue_on_listener_filters_timeout=true`
// (per ADR-0082). The catch-all chain matches every connection regardless of
// inputs, so the connection proceeds to its terminal filter and the dialer
// receives the tag byte.
func TestUnifiedDispatchListenerFilterTimeoutContinue(t *testing.T) {
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	filter := mkTcpProxyFilter(t, "c_a")

	l := &listenerv3.Listener{
		Name: "l_lf_timeout_continue",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{
				Address:       "127.0.0.1",
				PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0},
			},
		}},
		FilterChains: []*listenerv3.FilterChain{
			{Filters: []*listenerv3.Filter{filter}},
		},
		ListenerFiltersTimeout:           durationpb.New(1 * time.Second),
		ContinueOnListenerFiltersTimeout: true,
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	installSlowListenerFilter(mgr.runtimes[0], 2*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	conn, err := net.DialTimeout("tcp", mgr.Listeners()[0].Addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()
	// Allow a bit more than the pipeline timeout for the post-timeout
	// dispatch to deliver the tag byte from the upstream backend.
	got := readByteWithTimeout(t, conn, 4*time.Second)
	if got != 'A' {
		t.Errorf("tag byte = %q, want 'A' (continue_on_listener_filters_timeout=true should keep going)", got)
	}
}

// ---------------------------------------------------------------------------
// Phase-08.2 Task 5 — listener.Manager.Drain() + Accept-loop fast-path
// ---------------------------------------------------------------------------

func TestManager_Drain(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	dm := drain.New(10 * time.Millisecond)
	m, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), dm, nil, testNetRegistryWithTerminals(t, cm))
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if dm.IsDraining() {
		t.Errorf("IsDraining() pre-Drain: got true, want false")
	}
	m.Drain()
	if !dm.IsDraining() {
		t.Errorf("IsDraining() post-Drain: got false, want true")
	}
}

func TestManager_DrainIdempotent(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	dm := drain.New(10 * time.Millisecond)
	m, _ := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), dm, nil, testNetRegistryWithTerminals(t, cm))
	m.Drain()
	m.Drain()
	m.Drain()
	if !dm.IsDraining() {
		t.Errorf("IsDraining() post-multi-Drain: got false, want true")
	}
}

func TestManager_AcceptDuringDrainClosesConn(t *testing.T) {
	// Boot a listener; trigger Drain; dial the listener; assert the conn is
	// closed without filter-chain dispatch (i.e., empty read / EOF).
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp_drain", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	dm := drain.New(1 * time.Hour)
	m, _ := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), dm, nil, testNetRegistryWithTerminals(t, cm))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Stop()
	addr := m.Listeners()[0].Addr
	m.Drain() // fast-path activates
	// Dial; expect handshake + immediate FIN (empty body / EOF on first read).
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = c.Close() }()
	_ = c.SetReadDeadline(time.Now().Add(1 * time.Second))
	buf := make([]byte, 64)
	n, err := c.Read(buf)
	if err != io.EOF || n != 0 {
		t.Errorf("expected EOF (accept-then-FIN); got n=%d err=%v", n, err)
	}
}

func TestManager_StopAfterDrain(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	dm := drain.New(10 * time.Millisecond)
	m, _ := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), dm, nil, testNetRegistryWithTerminals(t, cm))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = m.Start(ctx)
	m.Drain() // sets dm.IsDraining=true
	m.Stop()  // closes listening sockets (post-drain teardown)
	m.Stop()  // idempotent
}

// ---------------------------------------------------------------------------
// Phase 26.1 (Task 10/11): network read-filter-chain dual-dispatch
// ---------------------------------------------------------------------------

// mkEchoNetFilter builds a *listenerv3.Filter whose typed_config is the
// network echo filter (empty body — echo has no fields).
func mkEchoNetFilter(t *testing.T) *listenerv3.Filter {
	t.Helper()
	a := &anypb.Any{TypeUrl: echo.TypeURL}
	return &listenerv3.Filter{Name: "envoy.filters.network.echo", ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: a}}
}

// mkDirectResponseNetFilter builds a *listenerv3.Filter whose typed_config is
// the network direct_response filter with an inline_string response body.
func mkDirectResponseNetFilter(t *testing.T, body string) *listenerv3.Filter {
	t.Helper()
	msg := &drv3.Config{Response: &corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: body}}}
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New(direct_response): %v", err)
	}
	return &listenerv3.Filter{Name: "envoy.filters.network.direct_response", ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: a}}
}

// testNetRegistry returns a freshly-allocated, frozen *network.Registry with
// echo + direct_response registered. Mirrors the testHTTPRegistry / testLFRegistry
// boot-populated-then-frozen pattern.
func testNetRegistry() *network.Registry {
	r := network.NewRegistry()
	r.Register(echo.TypeURL, echo.New)
	r.Register(directresponse.TypeURL, directresponse.New)
	r.Freeze()
	return r
}

// firstChain returns the single built chainInfo of mgr's first runtime (the
// catch-all/default chain a single-chain bootstrap produces). Post-Task-10 every
// chain carries a non-nil .netChainFactory (netReg is the SOLE registry); tests
// assert on it to verify the unified chain build.
func firstChain(t *testing.T, mgr *Manager) *chainInfo {
	t.Helper()
	if len(mgr.runtimes) == 0 {
		t.Fatal("Manager has no runtimes")
	}
	rt := mgr.runtimes[0]
	if len(rt.chainSpecs) != 1 {
		t.Fatalf("expected exactly 1 chainSpec, got %d", len(rt.chainSpecs))
	}
	ci := rt.chainByName[rt.chainSpecs[0].Name]
	if ci == nil {
		t.Fatalf("chainByName[%q] missing (logic bug)", rt.chainSpecs[0].Name)
	}
	return ci
}

// testNetRegistryWithTerminals returns a frozen *network.Registry with all four
// built-in network filters (echo, direct_response, tcp_proxy, HCM) registered
// via the Task-9 builtins seam (builtins.RegisterBuiltins). This is the SOLE
// registry the unified chain builder resolves every filter against post-Task-10;
// the shared DRY helper for terminal-filter test bootstraps.
func testNetRegistryWithTerminals(t *testing.T, cm *cluster.Manager) *network.Registry {
	t.Helper()
	r := network.NewRegistry()
	builtins.RegisterBuiltins(r, builtins.Deps{ClusterManager: cm, StatsRegistry: stats.NewRegistry(), HTTPRegistry: testHTTPRegistry()})
	r.Freeze()
	return r
}

// TestBuildChainPureTerminalThroughNetReg: with tcp_proxy registered in netReg,
// a [tcp_proxy] chain builds through the unified chain builder (filters[0]
// resolves in netReg → netChainFactory set).
func TestBuildChainPureTerminalThroughNetReg(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp_net", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)

	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err != nil {
		t.Fatalf("NewManager([tcp_proxy] via netReg): %v", err)
	}
	ci := firstChain(t, mgr)
	if ci.netChainFactory == nil {
		t.Error("netChainFactory is nil; expected new chain path for tcp_proxy filters[0]")
	}
	if got := len(ci.netChainFactory()); got != 1 {
		t.Errorf("netChainFactory() len = %d, want 1", got)
	}
}

// TestBuildChainMixedReadTerminalNowValid: [echo, tcp_proxy] — the 26.1
// mixed-chain reject is LIFTED. echo is a read filter, tcp_proxy a terminal-last
// → valid [read*, terminal?] shape → builds.
func TestBuildChainMixedReadTerminalNowValid(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := &listenerv3.Listener{
		Name: "l_mixed_valid",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{mkEchoNetFilter(t), mkTcpProxyFilter(t, "c_echo")},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err != nil {
		t.Fatalf("NewManager([echo, tcp_proxy]): %v (mixed read+terminal chain must now build)", err)
	}
	ci := firstChain(t, mgr)
	if ci.netChainFactory == nil {
		t.Fatal("netChainFactory is nil; expected new chain path for [echo, tcp_proxy]")
	}
	if got := len(ci.netChainFactory()); got != 2 {
		t.Errorf("netChainFactory() len = %d, want 2 (echo read + tcp_proxy terminal)", got)
	}
}

// TestBuildChainTerminalNotLastRejected: [tcp_proxy, echo] — a terminal filter
// not at the tail → boot-reject containing "network-filter-terminal-not-last".
func TestBuildChainTerminalNotLastRejected(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := &listenerv3.Listener{
		Name: "l_terminal_not_last",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_echo"), mkEchoNetFilter(t)},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err == nil {
		t.Fatal("NewManager([tcp_proxy, echo]) returned nil error; expected terminal-not-last boot-reject")
	}
	if !strings.Contains(err.Error(), "network-filter-terminal-not-last") {
		t.Errorf("error = %q; want it to mention network-filter-terminal-not-last", err.Error())
	}
}

// TestBuildChainMultipleTerminalsRejected: [tcp_proxy, hcm] — two terminal
// filters → boot-reject containing "network-filter-multiple-terminals".
func TestBuildChainMultipleTerminalsRejected(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := &listenerv3.Listener{
		Name: "l_multi_terminal",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{mkTcpProxyFilter(t, "c_echo"), mkHCMFilter(t)},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err == nil {
		t.Fatal("NewManager([tcp_proxy, hcm]) returned nil error; expected multiple-terminals boot-reject")
	}
	if !strings.Contains(err.Error(), "network-filter-multiple-terminals") {
		t.Errorf("error = %q; want it to mention network-filter-multiple-terminals", err.Error())
	}
}

func TestBuildNewPathChainWhenFilterInNetReg(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_net_echo", "127.0.0.1", 0, mkEchoNetFilter(t)),
	}, nil)

	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistry())
	if err != nil {
		t.Fatalf("NewManager(net echo chain): %v", err)
	}
	ci := firstChain(t, mgr)
	if ci.netChainFactory == nil {
		t.Error("netChainFactory is nil; expected new read-filter-chain path for echo filters[0]")
	}
	// The factory must allocate fresh instances per call (one per connection).
	a, b := ci.netChainFactory(), ci.netChainFactory()
	if len(a) != 1 || len(b) != 1 {
		t.Fatalf("netChainFactory returned len %d/%d, want 1/1", len(a), len(b))
	}
	if &a[0] == &b[0] {
		t.Error("netChainFactory returned the same backing array across calls (instances not fresh)")
	}
}

// TestNetChainLaterIndexNetRegMiss: with a netReg holding only echo+dr (NOT
// tcp_proxy), the chain [echo, tcp_proxy] resolves filters[0]=echo in netReg
// (committing to the net-chain path) but tcp_proxy at index 1 misses netReg.
// Phase-26.2 Task 7 LIFTED the 26.1 mixed-chain reject; a later-index miss is
// now the UNIFIED unknown-type-url form — byte-identical to the old terminal
// path's wording.
func TestNetChainLaterIndexNetRegMiss(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	// chain = [echo (in netReg), tcp_proxy (NOT in this netReg)].
	l := &listenerv3.Listener{
		Name: "l_later_miss",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{
			Filters: []*listenerv3.Filter{mkEchoNetFilter(t), mkTcpProxyFilter(t, "c_echo")},
		}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)

	_, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistry())
	if err == nil {
		t.Fatal("NewManager(later-index miss) returned nil error; expected unknown-type-url boot-reject")
	}
	if !strings.Contains(err.Error(), "unknown filter type_url") {
		t.Errorf("error = %q; want unified unknown-type-url wording", err.Error())
	}
	if strings.Contains(err.Error(), "network-filter-mixed-chain-unsupported") {
		t.Errorf("error = %q; the 26.1 mixed-chain reject must be DELETED", err.Error())
	}
}

// TestOldTerminalRegistryRetired: Task 10 retired the old terminal-filter path
// (buildTerminalFilter / filterRegistry). netReg is now the SOLE registry, so a
// chain whose filters[0] is NOT in netReg no longer falls back to a terminal
// path — it is the unified unknown-type-url boot-reject.
func TestOldTerminalRegistryRetired(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)

	// A netReg holding only echo+dr (NOT tcp_proxy); a [tcp_proxy] chain's
	// filters[0] misses netReg → unified unknown-type-url reject (no old path).
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp_miss", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	_, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistry())
	if err == nil {
		t.Fatal("NewManager(netReg without tcp_proxy, [tcp_proxy]) returned nil; expected unknown-type-url boot-reject (old terminal path retired)")
	}
	if !strings.Contains(err.Error(), "unknown filter type_url") {
		t.Errorf("error = %q; want unified unknown-type-url wording", err.Error())
	}

	// With all four built-ins registered (builtins seam), the same [tcp_proxy]
	// chain dispatches correctly through the unified netReg path.
	boot2 := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp_ok", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	mgr2, err := NewManagerWithBaseDirAndAllowH2C(boot2, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err != nil {
		t.Fatalf("NewManager(builtins netReg, [tcp_proxy]): %v", err)
	}
	ci2 := firstChain(t, mgr2)
	if ci2.netChainFactory == nil {
		t.Error("netChainFactory is nil; expected unified chain path for tcp_proxy via builtins netReg")
	}
}

// TestNilNetRegRejectsFilterChain: netReg == nil + a filter chain → boot error.
// Post-Task-10 there is no old terminal path to fall back to; a nil registry is
// a clear boot error (R-U).
func TestNilNetRegRejectsFilterChain(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_tcp_nilreg", "127.0.0.1", 0, mkTcpProxyFilter(t, "c_echo")),
	}, nil)
	_, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, nil)
	if err == nil {
		t.Fatal("NewManager(nil netReg, [tcp_proxy]) returned nil error; expected a boot error (no old terminal path)")
	}
	if !strings.Contains(err.Error(), "no network-filter registry") {
		t.Errorf("error = %q; want a clear nil-registry boot error", err.Error())
	}
}

// TestBuildChainUnknownTypeWordingPreserved (R-S / D-26.2-6): a filters[0]
// type_url in NEITHER built-in resolves through the unified builder (now the
// SOLE path; buildTerminalFilter deleted) to the EXISTING wording byte-for-byte:
// "...: unknown filter type_url %q".
func TestBuildChainUnknownTypeWordingPreserved(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	const badType = "type.googleapis.com/envoy.extensions.filters.network.does_not_exist.v3.Nope"
	bad := &listenerv3.Filter{
		Name:       "envoy.filters.network.does_not_exist",
		ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: &anypb.Any{TypeUrl: badType}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{
		mkListener("l_unknown", "127.0.0.1", 0, bad),
	}, nil)
	_, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm))
	if err == nil {
		t.Fatal("NewManager(unknown filters[0] type_url) returned nil error; expected unknown-type-url boot-reject")
	}
	want := `unknown filter type_url "` + badType + `"`
	if !strings.Contains(err.Error(), want) {
		t.Errorf("error = %q; want it to contain byte-stable wording %q", err.Error(), want)
	}
}

// startNetChainListener boots + starts a single-listener manager whose only
// filter_chain is the supplied network read-filter chain, and returns the
// bound address. The manager is Stop()'d via t.Cleanup.
func startNetChainListener(t *testing.T, filters ...*listenerv3.Filter) string {
	t.Helper()
	cm := mkClusterMgr(t, "c_unused", "127.0.0.1", 9999)
	l := &listenerv3.Listener{
		Name: "l_net",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{Filters: filters}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistry())
	if err != nil {
		t.Fatalf("NewManager(net chain): %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { mgr.Stop(); cancel() })
	ls := mgr.Listeners()
	if len(ls) != 1 || ls[0].Addr == "" {
		t.Fatalf("expected 1 bound listener, got %+v", ls)
	}
	return ls[0].Addr
}

func TestServeReadFilterChainEcho(t *testing.T) {
	addr := startNetChainListener(t, mkEchoNetFilter(t))

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Write SEVERAL SEPARATE payloads over the same connection, reading each
	// echo back before sending the next. Upstream echo re-iterates the chain on
	// every socket read, so EVERY write must be echoed — not just the first.
	// (Regression: the old sticky-halt chain swallowed writes 2+ after the first
	// StopIteration, so this read-back of write #2 timed out.)
	for _, want := range [][]byte{
		[]byte("hello echo 26.1"),
		[]byte("second write"),
		[]byte("third write"),
	} {
		if _, err := conn.Write(want); err != nil {
			t.Fatalf("write %q: %v", want, err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		got := make([]byte, len(want))
		if _, err := io.ReadFull(conn, got); err != nil {
			t.Fatalf("read echoed bytes for %q: %v", want, err)
		}
		if string(got) != string(want) {
			t.Errorf("echo = %q, want %q", got, want)
		}
	}

	// Half-close the write side → server observes EOF and closes its side. A
	// subsequent read on the connection must hit EOF.
	if tc, ok := conn.(*net.TCPConn); ok {
		if err := tc.CloseWrite(); err != nil {
			t.Fatalf("CloseWrite: %v", err)
		}
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Read(make([]byte, 8)); err != io.EOF {
		t.Errorf("after half-close, read err = %v, want io.EOF (server closed)", err)
	}
}

func TestServeReadFilterChainDirectResponse(t *testing.T) {
	const body = "go away (direct_response 26.1)"
	addr := startNetChainListener(t, mkDirectResponseNetFilter(t, body))

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// direct_response writes the body in OnNewConnection then closes; the client
	// reads the body followed by EOF without sending anything.
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != body {
		t.Errorf("direct_response body = %q, want %q", got, body)
	}
}

// ---------------------------------------------------------------------------
// Phase-26.2 Task 8 — serveConnection step-7 → unified serveNetworkChain.
//
// These four dispatch-shape tests drive REAL localhost connections through a
// Manager whose netReg has ALL FOUR filters registered (echo, direct_response,
// tcp_proxy, HCM). Because filters[0] resolves in netReg for every chain, EVERY
// connection dispatches through the NEW unified serveNetworkChain path (rather
// than the transitional old selected.filter.Handle branch, which only runs for
// nil-netChainFactory chains — deleted in Task 10):
//   - tcp_proxy → pure-terminal: immediate HandleTerminal (R3 byte-identical to
//     the retired selected.filter.Handle path — L4 pump to an upstream backend).
//   - HCM       → pure-terminal: HandleTerminal drives an HTTP/1 round-trip.
//   - echo      → pure-read: the 26.1 read loop, unchanged.
//   - direct_response → pure-read: static body then close, unchanged.
// ---------------------------------------------------------------------------

// startNetChainListenerWithReg boots + starts a single-listener Manager whose
// only filter_chain is the supplied network filter chain, threading the given
// cluster.Manager and netReg so terminal filters (tcp_proxy / HCM) resolve in
// netReg and dispatch through serveNetworkChain. Returns the bound address.
func startNetChainListenerWithReg(t *testing.T, cm *cluster.Manager, netReg *network.Registry, filters ...*listenerv3.Filter) string {
	t.Helper()
	l := &listenerv3.Listener{
		Name: "l_net",
		Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
			SocketAddress: &corev3.SocketAddress{Address: "127.0.0.1", PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 0}},
		}},
		FilterChains: []*listenerv3.FilterChain{{Filters: filters}},
	}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, netReg)
	if err != nil {
		t.Fatalf("NewManager(net chain w/ terminals): %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { mgr.Stop(); cancel() })
	ls := mgr.Listeners()
	if len(ls) != 1 || ls[0].Addr == "" {
		t.Fatalf("expected 1 bound listener, got %+v", ls)
	}
	return ls[0].Addr
}

// TestServeNetworkChainTCPProxy: a [tcp_proxy] chain with tcp_proxy registered
// in netReg dispatches through serveNetworkChain as a PURE-TERMINAL chain
// (immediate HandleTerminal). R3: byte-identical to the pre-migration terminal
// path — the L4 pump round-trips a tagged byte from the upstream backend.
func TestServeNetworkChainTCPProxy(t *testing.T) {
	addrA, cleanA := startTaggedBackend(t, 'A')
	defer cleanA()
	cm := mkClusterMgr(t, "c_a", "127.0.0.1", uint32(addrA.Port))
	netReg := testNetRegistryWithTerminals(t, cm)

	addr := startNetChainListenerWithReg(t, cm, netReg, mkTcpProxyFilter(t, "c_a"))

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// The upstream echo backend writes its tag byte on accept; the L4 pump must
	// deliver it downstream through the tcp_proxy terminal.
	if got := readByteWithTimeout(t, conn, 4*time.Second); got != 'A' {
		t.Errorf("tag byte = %q, want 'A' (tcp_proxy terminal via serveNetworkChain)", got)
	}
}

// TestServeNetworkChainHCM: a [hcm] chain with HCM registered in netReg
// dispatches through serveNetworkChain as a PURE-TERMINAL chain. R3: an HTTP/1
// request to the /health route round-trips a 200 response.
func TestServeNetworkChainHCM(t *testing.T) {
	cm := mkClusterMgr(t, "c_unused", "127.0.0.1", 9999)
	netReg := testNetRegistryWithTerminals(t, cm)

	addr := startNetChainListenerWithReg(t, cm, netReg, mkHCMFilter(t))

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("GET /health HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n")); err != nil {
		t.Fatalf("write request: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp := string(got)
	if !strings.Contains(resp, "200") {
		t.Errorf("HCM response = %q, want a 200 status line (HCM terminal via serveNetworkChain)", resp)
	}
	if !strings.Contains(resp, "OK") {
		t.Errorf("HCM response = %q, want the /health body 'OK'", resp)
	}
}

// TestServeNetworkChainEchoStillReadLoop: with echo registered in netReg, the
// [echo] chain dispatches through serveNetworkChain as a PURE-READ chain — the
// 26.1 read loop is unchanged, so bytes are echoed back.
func TestServeNetworkChainEchoStillReadLoop(t *testing.T) {
	cm := mkClusterMgr(t, "c_unused", "127.0.0.1", 9999)
	netReg := testNetRegistryWithTerminals(t, cm)

	addr := startNetChainListenerWithReg(t, cm, netReg, mkEchoNetFilter(t))

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	want := []byte("echo via serveNetworkChain")
	if _, err := conn.Write(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	got := make([]byte, len(want))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatalf("read echoed bytes: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("echo = %q, want %q", got, want)
	}
}

// TestServeNetworkChainDirectResponse: with direct_response registered in
// netReg, the [direct_response] chain dispatches through serveNetworkChain as a
// PURE-READ chain — static body written in OnNewConnection, then close. The
// 26.1 path is unchanged.
func TestServeNetworkChainDirectResponse(t *testing.T) {
	const body = "go away (direct_response via serveNetworkChain)"
	cm := mkClusterMgr(t, "c_unused", "127.0.0.1", 9999)
	netReg := testNetRegistryWithTerminals(t, cm)

	addr := startNetChainListenerWithReg(t, cm, netReg, mkDirectResponseNetFilter(t, body))

	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	got, err := io.ReadAll(conn)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != body {
		t.Errorf("direct_response body = %q, want %q", got, body)
	}
}
