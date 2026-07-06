package hcm

import (
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/stats"
)

// FuzzHCMConfigParse exercises the HCM constructor against arbitrary Any byte
// streams. Asserts: no panic; every error message is hcm:-prefixed.
//
// Per ADR-0018: short-budget (30s in CI; arbitrary local time). Seed corpus
// gives the fuzzer three starting points: one well-formed Any, one truncated
// byte stream, one wrong-type-url Any.
func FuzzHCMConfigParse(f *testing.F) {
	wellFormed := mkHCM(nil)
	f.Add(wellFormed.GetTypeUrl(), wellFormed.GetValue())
	f.Add("type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager", []byte{})
	f.Add("type.googleapis.com/google.protobuf.StringValue", []byte("hello"))

	cm := mkOneClusterManagerTB(f)
	httpReg := testHTTPRegistry()

	f.Fuzz(func(t *testing.T, typeURL string, value []byte) {
		any := &anypb.Any{TypeUrl: typeURL, Value: value}
		// Fresh Registry per iteration so the 5 HCM-scope counters the
		// constructor allocates on the happy path don't collide across fuzz
		// iterations. The httpReg is reused (frozen, no per-iter mutation).
		_, err := NewFilterWithCtxAndSinksAndRegistry(any, cm, ListenerCtx{}, stats.NewRegistry(), nil, httpReg, nil, nil)
		if err != nil && !strings.HasPrefix(err.Error(), "hcm:") {
			t.Errorf("error not hcm:-prefixed: %v", err)
		}
	})
}

// mkOneClusterManagerTB builds a tiny cluster.Manager with one STATIC cluster
// "c_test" at 127.0.0.1:1. Used by the fuzz target (testing.TB lets us pass
// either *testing.T or *testing.F).
func mkOneClusterManagerTB(t testing.TB) *cluster.Manager {
	t.Helper()
	bs := &bootstrapv3.Bootstrap{
		StaticResources: &bootstrapv3.Bootstrap_StaticResources{
			Clusters: []*clusterv3.Cluster{{
				Name:                 "c_test",
				ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
				LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
				ConnectTimeout:       durationpb.New(time.Second),
				LoadAssignment: &endpointv3.ClusterLoadAssignment{
					ClusterName: "c_test",
					Endpoints: []*endpointv3.LocalityLbEndpoints{{
						LbEndpoints: []*endpointv3.LbEndpoint{{
							HostIdentifier: &endpointv3.LbEndpoint_Endpoint{Endpoint: &endpointv3.Endpoint{
								Address: &corev3.Address{Address: &corev3.Address_SocketAddress{
									SocketAddress: &corev3.SocketAddress{
										Address:       "127.0.0.1",
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 1},
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
