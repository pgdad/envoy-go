package hcm

import (
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	tracingv3 "github.com/envoyproxy/go-control-plane/envoy/type/tracing/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

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

	// Phase 59: a custom_tags seed — one accepted `literal` + one rejected
	// `request_header` type. The custom_tags loop runs BEFORE the provider check
	// (config.go), so this seed exercises both the accept-append and a reject arm.
	withCustomTags := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.Tracing = &hcmv3.HttpConnectionManager_Tracing{
			CustomTags: []*tracingv3.CustomTag{
				{Tag: "env", Type: &tracingv3.CustomTag_Literal_{Literal: &tracingv3.CustomTag_Literal{Value: "prod"}}},
				{Tag: "hdr", Type: &tracingv3.CustomTag_RequestHeader{RequestHeader: &tracingv3.CustomTag_Header{Name: "x-req"}}},
			},
		}
	})
	f.Add(withCustomTags.GetTypeUrl(), withCustomTags.GetValue())

	// Phase 62: a request_header custom_tags seed — one accepted `request_header`
	// (name + default) + a mixed literal+request_header config with a duplicate key
	// (exercises the accept arm, the empty-name reject boundary is a unit test, and
	// the first-wins dedup path).
	withReqHeaderTags := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.Tracing = &hcmv3.HttpConnectionManager_Tracing{
			CustomTags: []*tracingv3.CustomTag{
				{Tag: "user", Type: &tracingv3.CustomTag_RequestHeader{RequestHeader: &tracingv3.CustomTag_Header{Name: "x-user", DefaultValue: "anon"}}},
				{Tag: "dup", Type: &tracingv3.CustomTag_Literal_{Literal: &tracingv3.CustomTag_Literal{Value: "L"}}},
				{Tag: "dup", Type: &tracingv3.CustomTag_RequestHeader{RequestHeader: &tracingv3.CustomTag_Header{Name: "x-dup"}}},
			},
		}
	})
	f.Add(withReqHeaderTags.GetTypeUrl(), withReqHeaderTags.GetValue())

	// Phase 63: an environment custom_tags seed — one accepted `environment`
	// (name + default) + a mixed literal+environment config with a duplicate key
	// (exercises the environment accept arm + the first-wins dedup; the empty-name
	// reject boundary is a config_test unit test).
	withEnvTags := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.Tracing = &hcmv3.HttpConnectionManager_Tracing{
			CustomTags: []*tracingv3.CustomTag{
				{Tag: "region", Type: &tracingv3.CustomTag_Environment_{Environment: &tracingv3.CustomTag_Environment{Name: "ENVOY_REGION", DefaultValue: "unknown"}}},
				{Tag: "dup", Type: &tracingv3.CustomTag_Literal_{Literal: &tracingv3.CustomTag_Literal{Value: "L"}}},
				{Tag: "dup", Type: &tracingv3.CustomTag_Environment_{Environment: &tracingv3.CustomTag_Environment{Name: "ENVOY_DUP"}}},
			},
		}
	})
	f.Add(withEnvTags.GetTypeUrl(), withEnvTags.GetValue())

	// Phase 64: a max_path_tag_length seed (incl. an explicit 0) exercises the tracing
	// numeric-knob resolve arm (the GetMaxPathTagLength resolve REPLACED the former
	// reject). The block sets no provider, so it errors at "provider required" AFTER
	// the resolve — the fuzz asserts no-panic + hcm:-prefixed.
	withMaxPathTag := mkHCM(func(h *hcmv3.HttpConnectionManager) {
		h.Tracing = &hcmv3.HttpConnectionManager_Tracing{
			MaxPathTagLength: wrapperspb.UInt32(0),
		}
	})
	f.Add(withMaxPathTag.GetTypeUrl(), withMaxPathTag.GetValue())

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
