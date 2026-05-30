package tcpproxy

import (
	"testing"

	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/filter/network"
)

// Compile-time assertion: *Filter is a network.TerminalFilter (R-T).
var _ network.TerminalFilter = (*Filter)(nil)

func TestNewNetworkFactorySharedInstance(t *testing.T) {
	cm := mkClusterMgr(t, "c", "127.0.0.1", 9999)
	factory := NewNetworkFactory(cm, nil)
	tc, err := anypb.New(&tcpproxyv3.TcpProxy{
		StatPrefix:       "p",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c"},
	})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	mk, err := factory(tc, network.FactoryCtx{})
	if err != nil {
		t.Fatalf("NewNetworkFactory factory err: %v", err)
	}
	a, b := mk(), mk()
	if a != b {
		t.Errorf("tcp_proxy adapter must yield the SAME shared instance per call (conn-stateless terminal); got distinct")
	}
	if _, ok := a.(network.TerminalFilter); !ok {
		t.Errorf("yielded instance is not a network.TerminalFilter")
	}
}

func TestNewNetworkFactoryParseRejectPassthroughByteStable(t *testing.T) {
	cm := mkClusterMgr(t, "c", "127.0.0.1", 9999)
	factory := NewNetworkFactory(cm, nil)
	tc, err := anypb.New(&tcpproxyv3.TcpProxy{ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: ""}})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	_, err = factory(tc, network.FactoryCtx{})
	if err == nil || err.Error() != "tcpproxy: cluster reference is empty" {
		t.Fatalf("parse-reject not surfaced byte-stable through adapter: %v", err)
	}
}
