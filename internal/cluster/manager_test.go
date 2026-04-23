package cluster

import (
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/durationpb"
)

// ---------------------------------------------------------------------------
// Helper builders
// ---------------------------------------------------------------------------

func mkBootstrap(clusters ...*clusterv3.Cluster) *bootstrapv3.Bootstrap {
	return &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{Clusters: clusters},
	}
}

func mkStaticCluster(name string, endpoints ...*endpointv3.LbEndpoint) *clusterv3.Cluster {
	return &clusterv3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
		LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
		LoadAssignment: &endpointv3.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints:   []*endpointv3.LocalityLbEndpoints{{LbEndpoints: endpoints}},
		},
	}
}

func mkLbEndpoint(addr string, port uint32) *endpointv3.LbEndpoint {
	return &endpointv3.LbEndpoint{
		HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
			Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
				SocketAddress: &corev3.SocketAddress{Address: addr, PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port}},
			}},
		}},
	}
}

// ---------------------------------------------------------------------------
// Happy-path tests
// ---------------------------------------------------------------------------

func TestManager_HappyPath_Single(t *testing.T) {
	bs := mkBootstrap(
		mkStaticCluster("c_echo", mkLbEndpoint("127.0.0.1", 8080)),
	)
	m, err := NewManager(bs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	c, ok := m.Get("c_echo")
	if !ok {
		t.Fatal("expected cluster c_echo to be present")
	}
	if c.Name() != "c_echo" {
		t.Errorf("Name() = %q, want %q", c.Name(), "c_echo")
	}
	if c.ConnectTimeout() != 5*time.Second {
		t.Errorf("ConnectTimeout() = %v, want 5s", c.ConnectTimeout())
	}
	ep, err := c.PickEndpoint()
	if err != nil {
		t.Fatalf("PickEndpoint() error: %v", err)
	}
	if ep.Host != "127.0.0.1" || ep.Port != 8080 {
		t.Errorf("PickEndpoint() = %+v, want {Host:127.0.0.1 Port:8080}", ep)
	}
}

func TestManager_HappyPath_Multi(t *testing.T) {
	c1 := mkStaticCluster("c_alpha",
		mkLbEndpoint("10.0.0.1", 9000),
		mkLbEndpoint("10.0.0.2", 9001),
	)
	c2 := mkStaticCluster("c_beta",
		mkLbEndpoint("10.0.1.1", 9100),
		mkLbEndpoint("10.0.1.2", 9101),
	)
	// Set an explicit connect_timeout on c2.
	c2.ConnectTimeout = durationpb.New(time.Second)

	bs := mkBootstrap(c1, c2)
	m, err := NewManager(bs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := m.Get("c_alpha"); !ok {
		t.Error("expected c_alpha to be present")
	}
	got, ok := m.Get("c_beta")
	if !ok {
		t.Fatal("expected c_beta to be present")
	}
	if got.ConnectTimeout() != time.Second {
		t.Errorf("c_beta ConnectTimeout() = %v, want 1s", got.ConnectTimeout())
	}

	// Absent name returns (nil, false).
	if _, ok := m.Get("no_such_cluster"); ok {
		t.Error("Get(absent) should return false")
	}
}

// ---------------------------------------------------------------------------
// Error-path tests
// ---------------------------------------------------------------------------

func TestManager_Error_ZeroClusters(t *testing.T) {
	bs := mkBootstrap() // no clusters
	_, err := NewManager(bs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "cluster: zero clusters") {
		t.Errorf("error %q does not contain %q", err.Error(), "cluster: zero clusters")
	}
}

func TestManager_Error_DuplicateName(t *testing.T) {
	bs := mkBootstrap(
		mkStaticCluster("c_echo", mkLbEndpoint("127.0.0.1", 8080)),
		mkStaticCluster("c_echo", mkLbEndpoint("127.0.0.2", 8081)),
	)
	_, err := NewManager(bs)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate cluster") {
		t.Errorf("error %q does not contain %q", err.Error(), "duplicate cluster")
	}
}

func TestManager_Error_StrictDNS(t *testing.T) {
	c := mkStaticCluster("c_dns", mkLbEndpoint("127.0.0.1", 8080))
	c.ClusterDiscoveryType = &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STRICT_DNS}
	_, err := NewManager(mkBootstrap(c))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "STRICT_DNS") {
		t.Errorf("error %q does not contain STRICT_DNS", err.Error())
	}
	if !strings.Contains(err.Error(), "phase 02 supports only STATIC") {
		t.Errorf("error %q does not contain %q", err.Error(), "phase 02 supports only STATIC")
	}
}

func TestManager_Error_LogicalDNS(t *testing.T) {
	c := mkStaticCluster("c_dns", mkLbEndpoint("127.0.0.1", 8080))
	c.ClusterDiscoveryType = &clusterv3.Cluster_Type{Type: clusterv3.Cluster_LOGICAL_DNS}
	_, err := NewManager(mkBootstrap(c))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "LOGICAL_DNS") {
		t.Errorf("error %q does not contain LOGICAL_DNS", err.Error())
	}
	if !strings.Contains(err.Error(), "phase 02 supports only STATIC") {
		t.Errorf("error %q does not contain %q", err.Error(), "phase 02 supports only STATIC")
	}
}

func TestManager_Error_EDS(t *testing.T) {
	c := mkStaticCluster("c_eds", mkLbEndpoint("127.0.0.1", 8080))
	c.ClusterDiscoveryType = &clusterv3.Cluster_Type{Type: clusterv3.Cluster_EDS}
	_, err := NewManager(mkBootstrap(c))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "EDS") {
		t.Errorf("error %q does not contain EDS", err.Error())
	}
	if !strings.Contains(err.Error(), "phase 02 supports only STATIC") {
		t.Errorf("error %q does not contain %q", err.Error(), "phase 02 supports only STATIC")
	}
}

func TestManager_Error_OriginalDST(t *testing.T) {
	c := mkStaticCluster("c_orig", mkLbEndpoint("127.0.0.1", 8080))
	c.ClusterDiscoveryType = &clusterv3.Cluster_Type{Type: clusterv3.Cluster_ORIGINAL_DST}
	_, err := NewManager(mkBootstrap(c))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ORIGINAL_DST") {
		t.Errorf("error %q does not contain ORIGINAL_DST", err.Error())
	}
	if !strings.Contains(err.Error(), "phase 02 supports only STATIC") {
		t.Errorf("error %q does not contain %q", err.Error(), "phase 02 supports only STATIC")
	}
}

func TestManager_Error_NonRoundRobinLB(t *testing.T) {
	c := mkStaticCluster("c_lr", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_LEAST_REQUEST
	_, err := NewManager(mkBootstrap(c))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ROUND_ROBIN") {
		t.Errorf("error %q does not contain ROUND_ROBIN", err.Error())
	}
}

func TestManager_Error_ZeroEndpoints(t *testing.T) {
	// One locality group with empty lb_endpoints.
	c := mkStaticCluster("c_empty" /* no endpoints */)
	_, err := NewManager(mkBootstrap(c))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "zero endpoints") {
		t.Errorf("error %q does not contain %q", err.Error(), "zero endpoints")
	}
}

func TestManager_Error_NonSocketAddressEndpoint(t *testing.T) {
	// Build an LbEndpoint that uses Address_Pipe instead of Address_SocketAddress.
	pipeEp := &endpointv3.LbEndpoint{
		HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
			Address: &corev3.Address{Address: &corev3.Address_Pipe{
				Pipe: &corev3.Pipe{Path: "/var/run/echo.sock"},
			}},
		}},
	}
	c := mkStaticCluster("c_pipe", pipeEp)
	_, err := NewManager(mkBootstrap(c))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "socket_address") {
		t.Errorf("error %q does not contain socket_address", err.Error())
	}
}
