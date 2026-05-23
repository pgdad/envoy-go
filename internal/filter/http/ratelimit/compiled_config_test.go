package ratelimit

import (
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	rlsv3 "github.com/envoyproxy/go-control-plane/envoy/config/ratelimit/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	ratelimitfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ratelimit/v3"
	httppopts "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/stats"
)

// ----------------------------------------------------------------------------
// Test helpers — minimum-viable RateLimit proto baseline + a cluster manager
// stocked with an HTTP/2 RLS cluster so the cluster-load gates pass.
// ----------------------------------------------------------------------------

// rlsClusterName_test is the canonical RLS cluster name reused by the test
// helpers + the cluster-manager-bearing FactoryCtx.
const rlsClusterName_test = "rate_limit_service"

// validRLSGrpcService returns the minimal *corev3.GrpcService pointing at the
// rlsClusterName_test cluster via envoy_grpc — exercises the happy-path of the
// cluster-load gates.
func validRLSGrpcService() *corev3.GrpcService {
	return &corev3.GrpcService{
		TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
			EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{
				ClusterName: rlsClusterName_test,
			},
		},
	}
}

// validRateLimitConfig returns a fully-populated *ratelimitfilterv3.RateLimit
// proto with all required fields set to valid values + nothing extra. Each
// PARSE-REJECT row mutates this baseline.
func validRateLimitConfig() *ratelimitfilterv3.RateLimit {
	return &ratelimitfilterv3.RateLimit{
		Domain:           "test-domain",
		Stage:            0,
		RequestType:      "both",
		FailureModeDeny:  false,
		RateLimitService: &rlsv3.RateLimitServiceConfig{GrpcService: validRLSGrpcService()},
	}
}

// toAnyRL wraps a *ratelimitfilterv3.RateLimit in an *anypb.Any envelope for
// buildCompiledConfig consumption.
func toAnyRL(t *testing.T, c *ratelimitfilterv3.RateLimit) *anypb.Any {
	t.Helper()
	a, err := anypb.New(c)
	if err != nil {
		t.Fatalf("anypb.New(RateLimit): %v", err)
	}
	return a
}

// mkRatelimitH2ClusterMgr builds a *cluster.Manager carrying a single STATIC
// cluster named `name` with HTTP/2 framing enabled (so UseH2() returns true).
// Modeled byte-for-byte on the ext_authz mkExtauthzH2ClusterMgr precedent.
// Loopback port is arbitrary — PARSE-REJECT paths never reach the dial step.
func mkRatelimitH2ClusterMgr(t testing.TB, name string) *cluster.Manager {
	t.Helper()
	hpo := &httppopts.HttpProtocolOptions{
		UpstreamProtocolOptions: &httppopts.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &httppopts.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &httppopts.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &corev3.Http2ProtocolOptions{},
				},
			},
		},
	}
	hpoAny, err := anypb.New(hpo)
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
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 12345},
									},
								}},
							}},
						}},
					}},
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

// mkRatelimitPlainClusterMgr builds a *cluster.Manager carrying a single
// STATIC cluster named `name` with NO HTTP/2 framing (so UseH2() returns
// false). Drives the cluster-must-have-h2 PARSE-REJECT arm.
func mkRatelimitPlainClusterMgr(t testing.TB, name string) *cluster.Manager {
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
										PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: 12345},
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

// ratelimitFactoryCtxWithClusterMgr returns a FactoryCtx with the supplied
// *cluster.Manager (+ fresh *stats.Registry).
func ratelimitFactoryCtxWithClusterMgr(cm *cluster.Manager) envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{
		Stats:          stats.NewRegistry(),
		StatPrefix:     "ingress_http",
		ClusterManager: cm,
	}
}

// ----------------------------------------------------------------------------
// TestBuildCompiledConfig — table-driven §5.1 PARSE-REJECT roster + defaults/
// clamps coverage per PLAN Task 3 Step 1.
// ----------------------------------------------------------------------------

func TestBuildCompiledConfig(t *testing.T) {
	t.Run("PARSE_REJECT", testBuildCompiledConfigParseReject)
	t.Run("Defaults", testBuildCompiledConfigDefaults)
	t.Run("HappyPath", testBuildCompiledConfigHappyPath)
}

// testBuildCompiledConfigParseReject drives each §5.1 PARSE-REJECT arm.
// Each row mutates validRateLimitConfig() to trigger ONE specific arm and
// asserts err.Error() matches (or contains, for the cluster-load arms with a
// %q-quoted cluster-name suffix) the byte-stable wording per ADR-0080.
func testBuildCompiledConfigParseReject(t *testing.T) {
	// h2Ctx is reused for the rows whose triggers fire ABOVE the cluster
	// lookup; plainCtx exercises the !UseH2() gate; nilCtx exercises the
	// "cluster manager not available" gate.
	h2Ctx := ratelimitFactoryCtxWithClusterMgr(mkRatelimitH2ClusterMgr(t, rlsClusterName_test))
	plainCtx := ratelimitFactoryCtxWithClusterMgr(mkRatelimitPlainClusterMgr(t, rlsClusterName_test))
	emptyCtx := ratelimitFactoryCtxWithClusterMgr(mkRatelimitH2ClusterMgr(t, "some_other_cluster"))
	nilCtx := envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "ingress_http"}

	cases := []struct {
		name      string
		ctx       envoyhttp.FactoryCtx
		mutate    func(*ratelimitfilterv3.RateLimit)
		wantSub   string
		wantExact bool // true ⇒ assert err.Error() == wantSub; false ⇒ assert strings.Contains
	}{
		// §5.1 Arm 1: domain empty.
		{
			name:      "Arm01_Domain_Empty",
			ctx:       h2Ctx,
			mutate:    func(c *ratelimitfilterv3.RateLimit) { c.Domain = "" },
			wantSub:   parseRejectDomainRequired,
			wantExact: true,
		},
		// §5.1 Arm 2: rate_limit_service absent.
		{
			name:      "Arm02_RateLimitService_Absent",
			ctx:       h2Ctx,
			mutate:    func(c *ratelimitfilterv3.RateLimit) { c.RateLimitService = nil },
			wantSub:   parseRejectRateLimitServiceRequired,
			wantExact: true,
		},
		// §5.1 Arm 3: stage > 10.
		{
			name:      "Arm03_Stage_TooHigh_11",
			ctx:       h2Ctx,
			mutate:    func(c *ratelimitfilterv3.RateLimit) { c.Stage = 11 },
			wantSub:   parseRejectStageTooHigh,
			wantExact: true,
		},
		{
			name:      "Arm03_Stage_TooHigh_42",
			ctx:       h2Ctx,
			mutate:    func(c *ratelimitfilterv3.RateLimit) { c.Stage = 42 },
			wantSub:   parseRejectStageTooHigh,
			wantExact: true,
		},
		// §5.1 Arm 4: request_type not in {internal,external,both,""}.
		{
			name:      "Arm04_RequestType_Invalid",
			ctx:       h2Ctx,
			mutate:    func(c *ratelimitfilterv3.RateLimit) { c.RequestType = "ingress" },
			wantSub:   parseRejectRequestTypeInvalid,
			wantExact: true,
		},
		{
			name:      "Arm04_RequestType_MixedCase",
			ctx:       h2Ctx,
			mutate:    func(c *ratelimitfilterv3.RateLimit) { c.RequestType = "Internal" },
			wantSub:   parseRejectRequestTypeInvalid,
			wantExact: true,
		},
		// §5.1 Arm 5: response_headers_to_add > 10 items.
		{
			name: "Arm05_ResponseHeadersToAdd_11",
			ctx:  h2Ctx,
			mutate: func(c *ratelimitfilterv3.RateLimit) {
				hdrs := make([]*corev3.HeaderValueOption, 11)
				for i := range hdrs {
					hdrs[i] = &corev3.HeaderValueOption{Header: &corev3.HeaderValue{Key: "x-foo", Value: "v"}}
				}
				c.ResponseHeadersToAdd = hdrs
			},
			wantSub:   parseRejectResponseHeadersTooMany,
			wantExact: true,
		},
		// §5.1 Arm 6: rate_limit_service.grpc_service absent.
		{
			name: "Arm06_GrpcService_Absent",
			ctx:  h2Ctx,
			mutate: func(c *ratelimitfilterv3.RateLimit) {
				c.RateLimitService = &rlsv3.RateLimitServiceConfig{}
			},
			wantSub:   parseRejectGrpcServiceRequired,
			wantExact: true,
		},
		// §5.1 Arm 7: google_grpc arm not supported.
		{
			name: "Arm07_GoogleGrpc_NotSupported",
			ctx:  h2Ctx,
			mutate: func(c *ratelimitfilterv3.RateLimit) {
				c.RateLimitService = &rlsv3.RateLimitServiceConfig{
					GrpcService: &corev3.GrpcService{
						TargetSpecifier: &corev3.GrpcService_GoogleGrpc_{
							GoogleGrpc: &corev3.GrpcService_GoogleGrpc{
								TargetUri:  "ratelimit.example.com:8081",
								StatPrefix: "rls",
							},
						},
					},
				}
			},
			wantSub:   parseRejectGoogleGrpcNotSupported,
			wantExact: true,
		},
		// §5.1 Arm 8: envoy_grpc arm required (no target_specifier set).
		{
			name: "Arm08_EnvoyGrpc_ArmRequired",
			ctx:  h2Ctx,
			mutate: func(c *ratelimitfilterv3.RateLimit) {
				c.RateLimitService = &rlsv3.RateLimitServiceConfig{GrpcService: &corev3.GrpcService{}}
			},
			wantSub:   parseRejectEnvoyGrpcArmRequired,
			wantExact: true,
		},
		// §5.1 Arm 9: envoy_grpc.cluster_name empty.
		{
			name: "Arm09_EnvoyGrpc_ClusterName_Empty",
			ctx:  h2Ctx,
			mutate: func(c *ratelimitfilterv3.RateLimit) {
				c.RateLimitService = &rlsv3.RateLimitServiceConfig{
					GrpcService: &corev3.GrpcService{
						TargetSpecifier: &corev3.GrpcService_EnvoyGrpc_{
							EnvoyGrpc: &corev3.GrpcService_EnvoyGrpc{ClusterName: ""},
						},
					},
				}
			},
			wantSub:   parseRejectEnvoyGrpcClusterNameEmpty,
			wantExact: true,
		},
		// §5.1 Arm 10: cluster manager nil.
		{
			name:      "Arm10_ClusterMgr_Nil",
			ctx:       nilCtx,
			mutate:    func(c *ratelimitfilterv3.RateLimit) {},
			wantSub:   parseRejectClusterManagerNotAvailable,
			wantExact: true,
		},
		// §5.1 Arm 11: cluster unknown.
		{
			name:      "Arm11_Cluster_Unknown",
			ctx:       emptyCtx,
			mutate:    func(c *ratelimitfilterv3.RateLimit) {},
			wantSub:   `ratelimit: rate_limit_service.grpc_service: unknown cluster "rate_limit_service"`,
			wantExact: true,
		},
		// §5.1 Arm 12: cluster must enable HTTP/2.
		{
			name:      "Arm12_Cluster_NotH2",
			ctx:       plainCtx,
			mutate:    func(c *ratelimitfilterv3.RateLimit) {},
			wantSub:   `ratelimit: rate_limit_service.grpc_service: cluster "rate_limit_service" must have http2_protocol_options{} set (gRPC requires HTTP/2)`,
			wantExact: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			c := validRateLimitConfig()
			tc.mutate(c)
			_, err := buildCompiledConfig(toAnyRL(t, c), tc.ctx)
			if err == nil {
				t.Fatalf("buildCompiledConfig: want error, got nil")
			}
			if tc.wantExact {
				if err.Error() != tc.wantSub {
					t.Fatalf("buildCompiledConfig: err mismatch:\n  got: %q\n want: %q", err.Error(), tc.wantSub)
				}
			} else {
				if !strings.Contains(err.Error(), tc.wantSub) {
					t.Fatalf("buildCompiledConfig: err must contain %q; got %q", tc.wantSub, err.Error())
				}
			}
		})
	}
}

// testBuildCompiledConfigDefaults drives the AMEND-3 default/clamp coverage:
//   - timeout absent ⇒ 20ms
//   - rate_limited_status absent ⇒ 429
//   - rate_limited_status < 400 ⇒ 429 (clamp)
//   - status_on_error absent ⇒ 500
//   - status_on_error outside [100,511] ⇒ 500 (clamp)
//   - request_type empty ⇒ "both"
//   - request_type "internal" / "external" / "both" honored as-is
//   - response_headers_to_add exactly 10 ⇒ accepted
func testBuildCompiledConfigDefaults(t *testing.T) {
	h2Ctx := ratelimitFactoryCtxWithClusterMgr(mkRatelimitH2ClusterMgr(t, rlsClusterName_test))

	t.Run("Timeout_AbsentDefaults20ms", func(t *testing.T) {
		c := validRateLimitConfig()
		c.Timeout = nil
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.timeout != 20*time.Millisecond {
			t.Fatalf("timeout default: got %v, want 20ms", cc.timeout)
		}
	})

	t.Run("Timeout_HonoredWhenPresent", func(t *testing.T) {
		c := validRateLimitConfig()
		c.Timeout = durationpb.New(75 * time.Millisecond)
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.timeout != 75*time.Millisecond {
			t.Fatalf("timeout: got %v, want 75ms", cc.timeout)
		}
	})

	t.Run("RateLimitedStatus_AbsentDefaults429", func(t *testing.T) {
		c := validRateLimitConfig()
		c.RateLimitedStatus = nil
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.rateLimitedStatus != 429 {
			t.Fatalf("rateLimitedStatus default: got %d, want 429", cc.rateLimitedStatus)
		}
	})

	t.Run("RateLimitedStatus_Below400Clamps429", func(t *testing.T) {
		c := validRateLimitConfig()
		c.RateLimitedStatus = &typev3.HttpStatus{Code: typev3.StatusCode_OK} // 200 < 400
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.rateLimitedStatus != 429 {
			t.Fatalf("rateLimitedStatus 200 clamp: got %d, want 429", cc.rateLimitedStatus)
		}
	})

	t.Run("RateLimitedStatus_HonoredWhen400Plus", func(t *testing.T) {
		c := validRateLimitConfig()
		c.RateLimitedStatus = &typev3.HttpStatus{Code: typev3.StatusCode_ServiceUnavailable} // 503
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.rateLimitedStatus != 503 {
			t.Fatalf("rateLimitedStatus 503: got %d, want 503", cc.rateLimitedStatus)
		}
	})

	t.Run("StatusOnError_AbsentDefaults500", func(t *testing.T) {
		c := validRateLimitConfig()
		c.StatusOnError = nil
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.statusOnError != 500 {
			t.Fatalf("statusOnError default: got %d, want 500", cc.statusOnError)
		}
	})

	t.Run("StatusOnError_OutOfRangeClamps500", func(t *testing.T) {
		c := validRateLimitConfig()
		// The HttpStatus.Code enum may not be able to represent every numeric
		// value; we test the clamp via a value below 100. StatusCode_Empty == 0.
		c.StatusOnError = &typev3.HttpStatus{Code: typev3.StatusCode_Empty}
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.statusOnError != 500 {
			t.Fatalf("statusOnError 0 clamp: got %d, want 500", cc.statusOnError)
		}
	})

	t.Run("StatusOnError_HonoredInRange", func(t *testing.T) {
		c := validRateLimitConfig()
		c.StatusOnError = &typev3.HttpStatus{Code: typev3.StatusCode_BadGateway} // 502
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.statusOnError != 502 {
			t.Fatalf("statusOnError 502: got %d, want 502", cc.statusOnError)
		}
	})

	t.Run("RequestType_EmptyDefaultsBoth", func(t *testing.T) {
		c := validRateLimitConfig()
		c.RequestType = ""
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.requestType != "both" {
			t.Fatalf("requestType default: got %q, want %q", cc.requestType, "both")
		}
	})

	t.Run("RequestType_InternalHonored", func(t *testing.T) {
		c := validRateLimitConfig()
		c.RequestType = "internal"
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.requestType != "internal" {
			t.Fatalf("requestType: got %q, want %q", cc.requestType, "internal")
		}
	})

	t.Run("ResponseHeadersToAdd_ExactlyTenAccepted", func(t *testing.T) {
		c := validRateLimitConfig()
		hdrs := make([]*corev3.HeaderValueOption, 10)
		for i := range hdrs {
			hdrs[i] = &corev3.HeaderValueOption{Header: &corev3.HeaderValue{Key: "x-foo", Value: "v"}}
		}
		c.ResponseHeadersToAdd = hdrs
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if len(cc.responseHeadersToAdd) != 10 {
			t.Fatalf("responseHeadersToAdd len: got %d, want 10", len(cc.responseHeadersToAdd))
		}
	})

	t.Run("RlsClusterName_CapturedFromEnvoyGrpc", func(t *testing.T) {
		c := validRateLimitConfig()
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.rlsClusterName != rlsClusterName_test {
			t.Fatalf("rlsClusterName: got %q, want %q", cc.rlsClusterName, rlsClusterName_test)
		}
	})

	t.Run("EnableXRateLimitHeaders_OFF", func(t *testing.T) {
		c := validRateLimitConfig()
		c.EnableXRatelimitHeaders = ratelimitfilterv3.RateLimit_OFF
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if cc.enableXRateLimitHeaders {
			t.Fatalf("enableXRateLimitHeaders OFF: want false, got true")
		}
	})

	t.Run("EnableXRateLimitHeaders_DRAFT03", func(t *testing.T) {
		c := validRateLimitConfig()
		c.EnableXRatelimitHeaders = ratelimitfilterv3.RateLimit_DRAFT_VERSION_03
		cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
		if err != nil {
			t.Fatalf("buildCompiledConfig: %v", err)
		}
		if !cc.enableXRateLimitHeaders {
			t.Fatalf("enableXRateLimitHeaders DRAFT_VERSION_03: want true, got false")
		}
	})
}

// testBuildCompiledConfigHappyPath drives the fully-populated baseline + spot-
// checks the field roster (the AMEND-3 13-field landing).
func testBuildCompiledConfigHappyPath(t *testing.T) {
	h2Ctx := ratelimitFactoryCtxWithClusterMgr(mkRatelimitH2ClusterMgr(t, rlsClusterName_test))

	c := &ratelimitfilterv3.RateLimit{
		Domain:                         "happy-domain",
		Stage:                          5,
		RequestType:                    "external",
		Timeout:                        durationpb.New(100 * time.Millisecond),
		FailureModeDeny:                true,
		RateLimitedAsResourceExhausted: true,
		RateLimitService:               &rlsv3.RateLimitServiceConfig{GrpcService: validRLSGrpcService()},
		EnableXRatelimitHeaders:        ratelimitfilterv3.RateLimit_DRAFT_VERSION_03,
		DisableXEnvoyRatelimitedHeader: true,
		RateLimitedStatus:              &typev3.HttpStatus{Code: typev3.StatusCode_ServiceUnavailable}, // 503
		ResponseHeadersToAdd: []*corev3.HeaderValueOption{
			{Header: &corev3.HeaderValue{Key: "x-rl-policy", Value: "test"}},
		},
		StatusOnError: &typev3.HttpStatus{Code: typev3.StatusCode_BadGateway}, // 502
		StatPrefix:    "tenant-foo",
	}

	cc, err := buildCompiledConfig(toAnyRL(t, c), h2Ctx)
	if err != nil {
		t.Fatalf("buildCompiledConfig: %v", err)
	}

	// Spot-check all 13 fields per AMEND-3 roster:
	if cc.domain != "happy-domain" {
		t.Errorf("domain: got %q, want %q", cc.domain, "happy-domain")
	}
	if cc.stage != 5 {
		t.Errorf("stage: got %d, want 5", cc.stage)
	}
	if cc.requestType != "external" {
		t.Errorf("requestType: got %q, want %q", cc.requestType, "external")
	}
	if cc.timeout != 100*time.Millisecond {
		t.Errorf("timeout: got %v, want 100ms", cc.timeout)
	}
	if !cc.failureModeDeny {
		t.Errorf("failureModeDeny: want true")
	}
	if !cc.rateLimitedAsResourceExhausted {
		t.Errorf("rateLimitedAsResourceExhausted: want true")
	}
	if cc.rlsClusterName != rlsClusterName_test {
		t.Errorf("rlsClusterName: got %q, want %q", cc.rlsClusterName, rlsClusterName_test)
	}
	if !cc.enableXRateLimitHeaders {
		t.Errorf("enableXRateLimitHeaders: want true (DRAFT_VERSION_03)")
	}
	if !cc.disableXEnvoyRateLimitedHeader {
		t.Errorf("disableXEnvoyRateLimitedHeader: want true")
	}
	if cc.rateLimitedStatus != 503 {
		t.Errorf("rateLimitedStatus: got %d, want 503", cc.rateLimitedStatus)
	}
	if cc.statusOnError != 502 {
		t.Errorf("statusOnError: got %d, want 502", cc.statusOnError)
	}
	if cc.statPrefix != "tenant-foo" {
		t.Errorf("statPrefix: got %q, want %q", cc.statPrefix, "tenant-foo")
	}
	if len(cc.responseHeadersToAdd) != 1 {
		t.Errorf("responseHeadersToAdd len: got %d, want 1", len(cc.responseHeadersToAdd))
	}
}

// ----------------------------------------------------------------------------
// TestValidateRouteRateLimits — exercises the EXPORTED §5.2 validator (D-RL2)
// called by HCM at Task 5 against route/vhost []*routev3.RateLimit slices.
// ----------------------------------------------------------------------------

func TestValidateRouteRateLimits(t *testing.T) {
	cases := []struct {
		name    string
		rls     []*routev3.RateLimit
		wantErr string
		wantNil bool
	}{
		// Happy path: nil + empty + a benign generic_key action all pass.
		{
			name:    "Nil_Pass",
			rls:     nil,
			wantNil: true,
		},
		{
			name:    "Empty_Pass",
			rls:     []*routev3.RateLimit{},
			wantNil: true,
		},
		{
			name: "GenericKey_Pass",
			rls: []*routev3.RateLimit{{
				Actions: []*routev3.RateLimit_Action{{
					ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
						GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "v"},
					},
				}},
			}},
			wantNil: true,
		},
		// §5.2 Arm 1: disable_key non-empty.
		{
			name: "Arm01_DisableKey_NonEmpty",
			rls: []*routev3.RateLimit{{
				DisableKey: "rl-disable-foo",
			}},
			wantErr: parseRejectRouteRateLimitDisableKey,
		},
		{
			name: "Arm01_DisableKey_NonEmpty_SecondEntry",
			rls: []*routev3.RateLimit{
				{Actions: []*routev3.RateLimit_Action{{ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
					GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "v"},
				}}}},
				{DisableKey: "rl-disable-bar"},
			},
			wantErr: parseRejectRouteRateLimitDisableKey,
		},
		// §5.2 Arm 2: extension descriptor action.
		{
			name: "Arm02_Action_Extension",
			rls: []*routev3.RateLimit{{
				Actions: []*routev3.RateLimit_Action{{
					ActionSpecifier: &routev3.RateLimit_Action_Extension{
						Extension: &corev3.TypedExtensionConfig{Name: "custom-descriptor", TypedConfig: &anypb.Any{}},
					},
				}},
			}},
			wantErr: parseRejectRouteRateLimitActionExtension,
		},
		// §5.2 Arm 3: dynamic_metadata descriptor action (deprecated).
		{
			name: "Arm03_Action_DynamicMetadata",
			rls: []*routev3.RateLimit{{
				Actions: []*routev3.RateLimit_Action{{
					ActionSpecifier: &routev3.RateLimit_Action_DynamicMetadata{
						DynamicMetadata: &routev3.RateLimit_Action_DynamicMetaData{DescriptorKey: "k"},
					},
				}},
			}},
			wantErr: parseRejectRouteRateLimitActionDynamicMetadata,
		},
		// Phase 24.2 Task 2: per-policy `stage > 10` PARSE-REJECT (parent §4.4 +
		// upstream PGV `lte:10`). Mirrors the filter-envelope arm at
		// `buildCompiledConfig` (§5.1 Arm 3); the route-level arm fires at the
		// HCM route-table parse via this validator.
		{
			name: "Stage_TooHigh_PerPolicy_11",
			rls: []*routev3.RateLimit{{
				Stage: wrapperspb.UInt32(11),
				Actions: []*routev3.RateLimit_Action{{
					ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
						GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "v"},
					},
				}},
			}},
			wantErr: parseRejectStageTooHigh,
		},
		{
			name: "Stage_TooHigh_PerPolicy_42",
			rls: []*routev3.RateLimit{{
				Stage: wrapperspb.UInt32(42),
				Actions: []*routev3.RateLimit_Action{{
					ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
						GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "v"},
					},
				}},
			}},
			wantErr: parseRejectStageTooHigh,
		},
		// Happy path: per-policy stage in [0,10] is accepted.
		{
			name: "Stage_AtBound10_Pass",
			rls: []*routev3.RateLimit{{
				Stage: wrapperspb.UInt32(10),
				Actions: []*routev3.RateLimit_Action{{
					ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
						GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "v"},
					},
				}},
			}},
			wantNil: true,
		},
		{
			name: "Stage_AtBound0_Pass",
			rls: []*routev3.RateLimit{{
				Stage: wrapperspb.UInt32(0),
				Actions: []*routev3.RateLimit_Action{{
					ActionSpecifier: &routev3.RateLimit_Action_GenericKey_{
						GenericKey: &routev3.RateLimit_Action_GenericKey{DescriptorValue: "v"},
					},
				}},
			}},
			wantNil: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRouteRateLimits(tc.rls)
			if tc.wantNil {
				if err != nil {
					t.Fatalf("ValidateRouteRateLimits: want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateRouteRateLimits: want err %q, got nil", tc.wantErr)
			}
			if err.Error() != tc.wantErr {
				t.Fatalf("ValidateRouteRateLimits: err mismatch:\n  got: %q\n want: %q", err.Error(), tc.wantErr)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// TestParseRejectConstants_ByteStable pins all §5.1 + §5.2 byte-stable wording
// constants per ADR-0080. NO format-string drift; each constant asserted
// byte-exact against the parent SPEC §5 reference roster.
// ----------------------------------------------------------------------------

func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		// §5.1 RATIFIED-from-PGV / config-validation arms.
		{
			"S51_DomainRequired",
			parseRejectDomainRequired,
			"ratelimit: domain is required",
		},
		{
			"S51_RateLimitServiceRequired",
			parseRejectRateLimitServiceRequired,
			"ratelimit: rate_limit_service is required",
		},
		{
			"S51_StageTooHigh",
			parseRejectStageTooHigh,
			"ratelimit: stage must be <= 10",
		},
		{
			"S51_RequestTypeInvalid",
			parseRejectRequestTypeInvalid,
			"ratelimit: request_type must be one of internal|external|both",
		},
		{
			"S51_ResponseHeadersTooMany",
			parseRejectResponseHeadersTooMany,
			"ratelimit: response_headers_to_add accepts at most 10 items",
		},
		// §5.1 cluster-load arms (REUSE 1; ext_authz buildGRPCCheckFn precedent).
		{
			"S51_GrpcServiceRequired",
			parseRejectGrpcServiceRequired,
			"ratelimit: rate_limit_service.grpc_service is required",
		},
		{
			"S51_GoogleGrpcNotSupported",
			parseRejectGoogleGrpcNotSupported,
			"ratelimit: rate_limit_service.grpc_service: google_grpc arm not supported (envoy-go uses google.golang.org/grpc directly)",
		},
		{
			"S51_EnvoyGrpcArmRequired",
			parseRejectEnvoyGrpcArmRequired,
			"ratelimit: rate_limit_service.grpc_service: envoy_grpc arm required (target_specifier must be set)",
		},
		{
			"S51_EnvoyGrpcClusterNameEmpty",
			parseRejectEnvoyGrpcClusterNameEmpty,
			"ratelimit: rate_limit_service.grpc_service: envoy_grpc.cluster_name must be non-empty",
		},
		{
			"S51_ClusterManagerNotAvailable",
			parseRejectClusterManagerNotAvailable,
			"ratelimit: rate_limit_service.grpc_service: cluster manager not available (FactoryCtx.ClusterManager is nil)",
		},
		// §5.2 envoy-go-strict project-local arms (ADR-0200).
		{
			"S52_RouteRateLimitDisableKey",
			parseRejectRouteRateLimitDisableKey,
			"ratelimit: rate_limits[].disable_key is not yet supported (runtime keying deferred)",
		},
		{
			"S52_RouteRateLimitActionExtension",
			parseRejectRouteRateLimitActionExtension,
			"ratelimit: the 'extension' descriptor action is not yet supported",
		},
		{
			"S52_RouteRateLimitActionDynamicMetadata",
			parseRejectRouteRateLimitActionDynamicMetadata,
			"ratelimit: the deprecated 'dynamic_metadata' descriptor action is not supported; use 'metadata'",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("byte-stable wording drift:\n  const: %q\n   want: %q", tc.got, tc.want)
			}
		})
	}
}
