package cluster

import (
	"os"
	"strings"
	"testing"
	"time"

	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	upstreamshttpv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/stats"
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

// mkStaticClusterFromLbEndpoints builds a static cluster from pre-built
// *endpointv3.LbEndpoint values. Unlike mkStaticCluster, the caller constructs
// the LbEndpoint before passing it in, so fields like Metadata can be set.
func mkStaticClusterFromLbEndpoints(name string, lbes ...*endpointv3.LbEndpoint) *clusterv3.Cluster {
	return &clusterv3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
		LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
		LoadAssignment: &endpointv3.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints:   []*endpointv3.LocalityLbEndpoints{{LbEndpoints: lbes}},
		},
	}
}

// ---------------------------------------------------------------------------
// Happy-path tests
// ---------------------------------------------------------------------------

func TestManager_HappyPath_Single(t *testing.T) {
	bs := mkBootstrap(
		mkStaticCluster("c_echo", mkLbEndpoint("127.0.0.1", 8080)),
	)
	m, err := NewManager(bs, stats.NewRegistry())
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
	m, err := NewManager(bs, stats.NewRegistry())
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
	_, err := NewManager(bs, stats.NewRegistry())
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
	_, err := NewManager(bs, stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate cluster") {
		t.Errorf("error %q does not contain %q", err.Error(), "duplicate cluster")
	}
}

// TestNewManager_ClusterNameInvalidChars is the M-1 review-followup regression
// test (REVIEW.md §Findings/Minor; ADR-0065 Consequences (d) carry-forward).
// Cluster names propagate into eight assembled metric names ("cluster.<name>.
// upstream_rq_total" etc.) at registerClusterMetrics; if the assembled name
// contains characters outside the internal/stats nameRE permitted class
// ([a-zA-Z0-9_.] with the [a-zA-Z_] first-char and non-dot last-char rules),
// stats.Registry.NewCounter would panic in checkName per ADR-0059's boot-time
// panic discipline. The contract (mirror of TestParseFilter_StatPrefixInvalidChars
// at internal/filter/hcm/config_test.go:221): NewManager MUST return an
// "invalid cluster name" error and MUST NOT panic. The "0000000000 0" case
// reproduces the verbatim minimized fuzz-seed shape from the ADR-0065
// gate-(d) HCM crasher (12 bytes, literal SP at index 10), demonstrating
// the symmetric vulnerability surface.
func TestNewManager_ClusterNameInvalidChars(t *testing.T) {
	cases := []string{
		"0000000000 0", // verbatim shape of the ADR-0065 fuzz-seed minimized payload
		"foo bar",      // simpler space form
		"foo-bar",      // dash
		"foo:bar",      // colon
		"foo/bar",      // slash
		"foo$bar",      // dollar
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			bs := mkBootstrap(mkStaticCluster(name, mkLbEndpoint("127.0.0.1", 8080)))
			_, err := NewManager(bs, stats.NewRegistry())
			if err == nil {
				t.Fatalf("expected error for cluster name %q, got nil", name)
			}
			if !strings.Contains(err.Error(), "invalid cluster name") {
				t.Errorf("error %q does not contain %q", err.Error(), "invalid cluster name")
			}
		})
	}
}

func TestManager_Error_StrictDNS(t *testing.T) {
	c := mkStaticCluster("c_dns", mkLbEndpoint("127.0.0.1", 8080))
	c.ClusterDiscoveryType = &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STRICT_DNS}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "STRICT_DNS") {
		t.Errorf("error %q does not contain STRICT_DNS", err.Error())
	}
	if !strings.Contains(err.Error(), "only STATIC clusters supported") {
		t.Errorf("error %q does not contain %q", err.Error(), "only STATIC clusters supported")
	}
}

func TestManager_Error_LogicalDNS(t *testing.T) {
	c := mkStaticCluster("c_dns", mkLbEndpoint("127.0.0.1", 8080))
	c.ClusterDiscoveryType = &clusterv3.Cluster_Type{Type: clusterv3.Cluster_LOGICAL_DNS}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "LOGICAL_DNS") {
		t.Errorf("error %q does not contain LOGICAL_DNS", err.Error())
	}
	if !strings.Contains(err.Error(), "only STATIC clusters supported") {
		t.Errorf("error %q does not contain %q", err.Error(), "only STATIC clusters supported")
	}
}

func TestManager_Error_EDS(t *testing.T) {
	c := mkStaticCluster("c_eds", mkLbEndpoint("127.0.0.1", 8080))
	c.ClusterDiscoveryType = &clusterv3.Cluster_Type{Type: clusterv3.Cluster_EDS}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "EDS") {
		t.Errorf("error %q does not contain EDS", err.Error())
	}
	if !strings.Contains(err.Error(), "only STATIC clusters supported") {
		t.Errorf("error %q does not contain %q", err.Error(), "only STATIC clusters supported")
	}
}

func TestManager_Error_OriginalDST(t *testing.T) {
	c := mkStaticCluster("c_orig", mkLbEndpoint("127.0.0.1", 8080))
	c.ClusterDiscoveryType = &clusterv3.Cluster_Type{Type: clusterv3.Cluster_ORIGINAL_DST}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "ORIGINAL_DST") {
		t.Errorf("error %q does not contain ORIGINAL_DST", err.Error())
	}
	if !strings.Contains(err.Error(), "only STATIC clusters supported") {
		t.Errorf("error %q does not contain %q", err.Error(), "only STATIC clusters supported")
	}
}

// ---------------------------------------------------------------------------
// Phase 34 (Task 4) — LEAST_REQUEST acceptance + the §6 reject matrix
// ---------------------------------------------------------------------------

// mkLeastRequest sets the LEAST_REQUEST lb_policy and the
// least_request_lb_config oneof member with the given choice_count.
func mkLeastRequest(name string, cc *wrapperspb.UInt32Value, eps ...*endpointv3.LbEndpoint) *clusterv3.Cluster {
	c := mkStaticCluster(name, eps...)
	c.LbPolicy = clusterv3.Cluster_LEAST_REQUEST
	c.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{
		LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{ChoiceCount: cc},
	}
	return c
}

func TestManager_Accept_LeastRequest_NoConfig(t *testing.T) {
	c := mkStaticCluster("c_lr", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_LEAST_REQUEST // no lb_config → default choice_count 2
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Fatalf("LEAST_REQUEST bare must be accepted (default cc=2): %v", err)
	}
}

func TestManager_Accept_LeastRequest_ChoiceCounts(t *testing.T) {
	for _, cc := range []uint32{2, 100} { // 100 = no clamp (reference parity, AMEND-L3)
		c := mkLeastRequest("c_lr", wrapperspb.UInt32(cc), mkLbEndpoint("127.0.0.1", 8080))
		if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
			t.Errorf("choice_count %d must be accepted: %v", cc, err)
		}
	}
}

func TestManager_Error_LeastRequest_ChoiceCountTooSmall(t *testing.T) {
	for _, cc := range []uint32{0, 1} {
		c := mkLeastRequest("c_lr", wrapperspb.UInt32(cc), mkLbEndpoint("127.0.0.1", 8080))
		_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
		if err == nil {
			t.Fatalf("choice_count %d must be rejected", cc)
		}
		if !strings.Contains(err.Error(), "value must be greater than or equal to 2") {
			t.Errorf("cc=%d: error %q missing PGV-parity substring", cc, err.Error())
		}
	}
}

func TestManager_Error_LeastRequest_BiasUnsupported(t *testing.T) {
	c := mkLeastRequest("c_lr", wrapperspb.UInt32(2), mkLbEndpoint("127.0.0.1", 8080))
	c.GetLeastRequestLbConfig().ActiveRequestBias = &corev3.RuntimeDouble{DefaultValue: 1.5, RuntimeKey: "arb"}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "active_request_bias") {
		t.Errorf("active_request_bias under LEAST_REQUEST must be rejected; got %v", err)
	}
}

func TestManager_Error_LeastRequest_SlowStartUnsupported(t *testing.T) {
	c := mkLeastRequest("c_lr", wrapperspb.UInt32(2), mkLbEndpoint("127.0.0.1", 8080))
	c.GetLeastRequestLbConfig().SlowStartConfig = &clusterv3.Cluster_SlowStartConfig{}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "slow_start_config") {
		t.Errorf("slow_start_config under LEAST_REQUEST must be rejected; got %v", err)
	}
}

func TestManager_Accept_MismatchedOneof_RoundRobin(t *testing.T) {
	// least_request_lb_config under ROUND_ROBIN → silent-ignore (reference parity, §6.3).
	c := mkStaticCluster("c_rr", mkLbEndpoint("127.0.0.1", 8080)) // LbPolicy ROUND_ROBIN
	c.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{
		LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{ChoiceCount: wrapperspb.UInt32(7)},
	}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("mismatched oneof under ROUND_ROBIN must be silently accepted: %v", err)
	}
}

func TestManager_Error_UnsupportedLBPolicy(t *testing.T) { // RETARGET of TestManager_Error_NonRoundRobinLB
	c := mkStaticCluster("c_x", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_CLUSTER_PROVIDED // MAGLEV now accepted → retarget to the next still-rejected policy (AMEND-M5)
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("CLUSTER_PROVIDED must be rejected")
	}
	if !strings.Contains(err.Error(), "ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV") {
		t.Errorf("error %q missing new supported-set substring (…, MAGLEV)", err.Error())
	}
}

func TestManager_Accept_Random_NoConfig(t *testing.T) {
	c := mkStaticCluster("c_rand", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_RANDOM // RANDOM has no lb_config — bare construction
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Fatalf("RANDOM bare must be accepted: %v", err)
	}
}

func TestManager_Accept_Random_MismatchedOneof(t *testing.T) {
	// A stray least_request_lb_config under RANDOM → silent-ignore (reference parity,
	// SPEC §6.3 / AMEND-R1: the manager never reads the oneof on the RANDOM path).
	c := mkStaticCluster("c_rand", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_RANDOM
	c.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{
		LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{ChoiceCount: wrapperspb.UInt32(7)},
	}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("mismatched oneof under RANDOM must be silently accepted: %v", err)
	}
}

// TestManager_LeastRequest_BootSmoke proves a realistic LEAST_REQUEST bootstrap
// (cc=10, the 0059 config; 3 endpoints) builds a working Manager and that
// PickEndpoint (the immediate-release path) returns a valid in-range endpoint.
func TestManager_LeastRequest_BootSmoke(t *testing.T) {
	c := mkLeastRequest("c_lr", wrapperspb.UInt32(10), // cc=10 (the 0059 config)
		mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	m, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	cl, ok := m.Get("c_lr")
	if !ok {
		t.Fatal("cluster c_lr not found")
	}
	ep, err := cl.PickEndpoint() // exercises the immediate-release path
	if err != nil {
		t.Fatalf("PickEndpoint: %v", err)
	}
	if ep.Port < 9001 || ep.Port > 9003 {
		t.Errorf("picked out-of-range endpoint %v", ep)
	}
}

// TestManager_Random_BootSmoke proves a realistic 3-endpoint RANDOM bootstrap
// builds a working Manager and that PickEndpoint (the immediate-release path)
// returns a valid in-range endpoint (D-S35-4 / SPEC §10 boot smoke).
func TestManager_Random_BootSmoke(t *testing.T) {
	c := mkStaticCluster("c_rand",
		mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_RANDOM
	m, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("boot: %v", err)
	}
	cl, ok := m.Get("c_rand")
	if !ok {
		t.Fatal("cluster c_rand not found")
	}
	for i := 0; i < 10; i++ { // exercises the immediate-release PickEndpoint path
		ep, perr := cl.PickEndpoint()
		if perr != nil {
			t.Fatalf("PickEndpoint: %v", perr)
		}
		if ep.Port < 9001 || ep.Port > 9003 {
			t.Errorf("picked out-of-range endpoint %v", ep)
		}
	}
}

func TestManager_Error_ZeroEndpoints(t *testing.T) {
	// One locality group with empty lb_endpoints.
	c := mkStaticCluster("c_empty" /* no endpoints */)
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
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
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "socket_address") {
		t.Errorf("error %q does not contain socket_address", err.Error())
	}
}

// ---------------------------------------------------------------------------
// Phase-03 TLS cluster tests
// ---------------------------------------------------------------------------

// mkUpstreamTLSTransportSocket builds a corev3.TransportSocket carrying an
// UpstreamTlsContext with the given SNI and inline CA PEM bytes.
func mkUpstreamTLSTransportSocket(t *testing.T, sni string, caPEM []byte) *corev3.TransportSocket {
	t.Helper()
	ctx := &tlsv3.UpstreamTlsContext{
		Sni: sni,
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
	return &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.tls",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
	}
}

func TestNewManager_TLSCluster(t *testing.T) {
	caPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}

	c := mkStaticCluster("c_tls", mkLbEndpoint("10.0.0.1", 443))
	c.TransportSocket = mkUpstreamTLSTransportSocket(t, "alpha.envoy-go.test", caPEM)

	m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManagerWithBaseDir: %v", err)
	}

	got, ok := m.Get("c_tls")
	if !ok {
		t.Fatal("cluster c_tls not found")
	}
	if got.upstreamCfg == nil {
		t.Fatal("upstreamCfg is nil, want non-nil")
	}
	if got.upstreamCfg.ServerName != "alpha.envoy-go.test" {
		t.Errorf("ServerName = %q, want %q", got.upstreamCfg.ServerName, "alpha.envoy-go.test")
	}
	if got.upstreamCfg.RootCAs == nil {
		t.Error("RootCAs is nil, want non-nil")
	}
}

func TestNewManager_TLSCluster_UnknownTransportSocket(t *testing.T) {
	// Use a type_url that is not UpstreamTlsContext (raw_buffer).
	anyMsg, err := anypb.New(&tlsv3.DownstreamTlsContext{})
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	c := mkStaticCluster("c_bad_ts", mkLbEndpoint("10.0.0.1", 443))
	c.TransportSocket = &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.raw_buffer",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
	}
	_, err = NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported transport_socket type_url") {
		t.Errorf("error %q does not contain expected substring", err.Error())
	}
}

func TestNewManager_TLSCluster_MissingTrustedCA(t *testing.T) {
	// UpstreamTlsContext without validation_context.trusted_ca — must error.
	ctx := &tlsv3.UpstreamTlsContext{
		Sni:              "alpha.envoy-go.test",
		CommonTlsContext: &tlsv3.CommonTlsContext{
			// No ValidationContext set — trusted_ca is missing.
		},
	}
	anyMsg, err := anypb.New(ctx)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	c := mkStaticCluster("c_no_ca", mkLbEndpoint("10.0.0.1", 443))
	c.TransportSocket = &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.tls",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
	}
	_, err = NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "trusted_ca") {
		t.Errorf("error %q does not contain %q", err.Error(), "trusted_ca")
	}
}

// ---------------------------------------------------------------------------
// Phase 05.2 — HttpProtocolOptions parsing (Task 10)
// ---------------------------------------------------------------------------

// mkHttpProtocolOptionsAny wraps an upstreamshttpv3.HttpProtocolOptions into
// the anypb.Any that lives in cluster.typed_extension_protocol_options under
// the well-known key.
func mkHttpProtocolOptionsAny(t *testing.T, hpo *upstreamshttpv3.HttpProtocolOptions) *anypb.Any {
	t.Helper()
	a, err := anypb.New(hpo)
	if err != nil {
		t.Fatalf("anypb.New(HttpProtocolOptions): %v", err)
	}
	return a
}

// mkUpstreamTLSTransportSocketWithALPN is the variant of
// mkUpstreamTLSTransportSocket that also sets alpn_protocols on the
// CommonTlsContext.
func mkUpstreamTLSTransportSocketWithALPN(t *testing.T, sni string, caPEM []byte, alpn []string) *corev3.TransportSocket {
	t.Helper()
	ctx := &tlsv3.UpstreamTlsContext{
		Sni: sni,
		CommonTlsContext: &tlsv3.CommonTlsContext{
			AlpnProtocols: alpn,
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
	return &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.tls",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: anyMsg},
	}
}

// hpoExplicitH2 returns an HttpProtocolOptions selecting the
// explicit_http_config.http2_protocol_options{} discriminator (the active
// 05.2 H2 path).
func hpoExplicitH2() *upstreamshttpv3.HttpProtocolOptions {
	return &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{},
			},
		},
	}
}

// hpoExplicitH1 returns an HttpProtocolOptions selecting the
// explicit_http_config.http_protocol_options{} discriminator (the H1 path —
// 05.2 silently honors the discriminator's H1 selection but ignores its
// inner config).
func hpoExplicitH1() *upstreamshttpv3.HttpProtocolOptions {
	return &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_HttpProtocolOptions{},
			},
		},
	}
}

// hpoAutoConfig returns an HttpProtocolOptions selecting the auto_config
// branch (which the 05.2 SPEC narrows to silent-ignore per §5.5).
func hpoAutoConfig() *upstreamshttpv3.HttpProtocolOptions {
	return &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_AutoConfig{
			AutoConfig: &upstreamshttpv3.HttpProtocolOptions_AutoHttpConfig{},
		},
	}
}

func TestBuildCluster_H2Mode_Positive(t *testing.T) {
	caPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	c := mkStaticCluster("c_h2", mkLbEndpoint("10.0.0.1", 443))
	c.TransportSocket = mkUpstreamTLSTransportSocketWithALPN(t, "alpha.envoy-go.test", caPEM, []string{"h2"})
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
	}

	m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManagerWithBaseDir: %v", err)
	}
	got, ok := m.Get("c_h2")
	if !ok {
		t.Fatal("cluster c_h2 not found")
	}
	if !got.UseH2() {
		t.Error("UseH2() = false, want true")
	}
}

// TestExtractH2Mode_PlaintextH2C_NoTransportSocket_Accepted verifies ADR-0166:
// a cluster with http2_protocol_options:{} and NO transport_socket is now
// PERMITTED as plaintext h2c upstream (prior-knowledge per RFC 7540 §3.4).
// Reference Envoy v1.37.2 accepts the same shape; phase-18.2 fixture-0021's
// c_authz_grpc cluster relies on this relaxation. The cluster must build
// successfully with UseH2()=true and upstreamCfg=nil. Renamed and flipped
// from the prior TestBuildCluster_H2Mode_NoTLS error-assertion.
func TestExtractH2Mode_PlaintextH2C_NoTransportSocket_Accepted(t *testing.T) {
	c := mkStaticCluster("c_h2_plaintext", mkLbEndpoint("10.0.0.1", 8080))
	// No TransportSocket — plaintext h2c upstream.
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
	}
	m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManagerWithBaseDir: %v (ADR-0166: plaintext h2c must be permitted)", err)
	}
	got, ok := m.Get("c_h2_plaintext")
	if !ok {
		t.Fatal("cluster c_h2_plaintext not found")
	}
	if !got.UseH2() {
		t.Error("UseH2() = false, want true (plaintext h2c upstream)")
	}
	if got.upstreamCfg != nil {
		t.Errorf("upstreamCfg = %v, want nil (no transport_socket → plaintext h2c)", got.upstreamCfg)
	}
}

// TestExtractH2Mode_TLSH2_TransportSocketWithALPNH2_AcceptedUnchanged is a
// regression guard for the TLS+h2 branch preserved bit-identical by ADR-0166.
// Mirrors TestBuildCluster_H2Mode_Positive — same shape, distinct name so the
// ADR-0166 acceptance-coverage matrix is auditable in one grep.
func TestExtractH2Mode_TLSH2_TransportSocketWithALPNH2_AcceptedUnchanged(t *testing.T) {
	caPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	c := mkStaticCluster("c_h2_tls_alpn_h2", mkLbEndpoint("10.0.0.1", 443))
	c.TransportSocket = mkUpstreamTLSTransportSocketWithALPN(t, "alpha.envoy-go.test", caPEM, []string{"h2"})
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
	}
	m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManagerWithBaseDir: %v (TLS+h2 path must remain bit-identical post-ADR-0166)", err)
	}
	got, ok := m.Get("c_h2_tls_alpn_h2")
	if !ok {
		t.Fatal("cluster c_h2_tls_alpn_h2 not found")
	}
	if !got.UseH2() {
		t.Error("UseH2() = false, want true")
	}
	if got.upstreamCfg == nil {
		t.Error("upstreamCfg = nil, want non-nil (TLS+h2 cluster)")
	}
}

// TestExtractH2Mode_TLSH2_TransportSocketMissingALPNH2_StillRejected verifies
// that when transport_socket IS present, the existing ALPN-h2 enforcement
// remains in force — ADR-0166 relaxes the gate only for transport_socket-
// absent (plaintext h2c) clusters. Mirrors TestBuildCluster_H2Mode_TLSWithoutALPNH2.
func TestExtractH2Mode_TLSH2_TransportSocketMissingALPNH2_StillRejected(t *testing.T) {
	caPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	c := mkStaticCluster("c_h2_tls_no_alpn_h2", mkLbEndpoint("10.0.0.1", 443))
	c.TransportSocket = mkUpstreamTLSTransportSocketWithALPN(t, "alpha.envoy-go.test", caPEM, []string{"http/1.1"})
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
	}
	_, err = NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil (TLS-present + ALPN-missing-h2 must still reject)")
	}
	if !strings.Contains(err.Error(), "alpn_protocols to include") {
		t.Errorf("error %q does not contain %q", err.Error(), "alpn_protocols to include")
	}
	if !strings.Contains(err.Error(), `"h2"`) {
		t.Errorf("error %q does not mention %q", err.Error(), `"h2"`)
	}
}

// ---------------------------------------------------------------------------
// Phase 43.2a Task 2 — Parse max_concurrent_streams → h2MaxConcurrentStreams
// ---------------------------------------------------------------------------

// hpoExplicitH2WithMaxStreams returns an HttpProtocolOptions selecting
// explicit_http_config.http2_protocol_options with max_concurrent_streams set
// to the given value. Used by TestExtractH2Mode_MaxConcurrentStreams.
func hpoExplicitH2WithMaxStreams(v uint32) *upstreamshttpv3.HttpProtocolOptions {
	return &upstreamshttpv3.HttpProtocolOptions{
		UpstreamProtocolOptions: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_{
			ExplicitHttpConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig{
				ProtocolConfig: &upstreamshttpv3.HttpProtocolOptions_ExplicitHttpConfig_Http2ProtocolOptions{
					Http2ProtocolOptions: &corev3.Http2ProtocolOptions{
						MaxConcurrentStreams: wrapperspb.UInt32(v),
					},
				},
			},
		},
	}
}

// TestExtractH2Mode_MaxConcurrentStreams verifies that buildCluster threads
// http2_protocol_options.max_concurrent_streams onto Cluster.h2MaxConcurrentStreams
// (the local cap; AMEND-H2-1):
//   - explicit max_concurrent_streams: 2  → h2MaxConcurrentStreams == 2
//   - http2_protocol_options{} (no inner) → h2MaxConcurrentStreams == h2DefaultMaxConcurrentStreams (ABSENT path: the h2 != nil guard short-circuits)
//   - http2_protocol_options{max_concurrent_streams: 0} → h2MaxConcurrentStreams == h2DefaultMaxConcurrentStreams (configured-0 path: exercises the v.GetValue() > 0 guard)
//   - absent/0 is treated as the high default (NO reject arm — reference parity)
func TestExtractH2Mode_MaxConcurrentStreams(t *testing.T) {
	t.Run("explicit_cap_2", func(t *testing.T) {
		c := mkStaticCluster("c_h2_cap2", mkLbEndpoint("10.0.0.1", 8080))
		c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2WithMaxStreams(2)),
		}
		m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
		if err != nil {
			t.Fatalf("NewManagerWithBaseDir: %v", err)
		}
		got, ok := m.Get("c_h2_cap2")
		if !ok {
			t.Fatal("cluster c_h2_cap2 not found")
		}
		if got.h2MaxConcurrentStreams != 2 {
			t.Errorf("h2MaxConcurrentStreams = %d, want 2", got.h2MaxConcurrentStreams)
		}
	})

	t.Run("no_cap_defaults_to_high", func(t *testing.T) {
		c := mkStaticCluster("c_h2_nocap", mkLbEndpoint("10.0.0.1", 8080))
		c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
		}
		m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
		if err != nil {
			t.Fatalf("NewManagerWithBaseDir: %v", err)
		}
		got, ok := m.Get("c_h2_nocap")
		if !ok {
			t.Fatal("cluster c_h2_nocap not found")
		}
		if got.h2MaxConcurrentStreams != h2DefaultMaxConcurrentStreams {
			t.Errorf("h2MaxConcurrentStreams = %d, want h2DefaultMaxConcurrentStreams (%d)",
				got.h2MaxConcurrentStreams, h2DefaultMaxConcurrentStreams)
		}
	})

	// configured_zero_defaults_to_high uses a NON-NIL inner *Http2ProtocolOptions
	// carrying max_concurrent_streams: 0 — so the `h2 != nil` guard does NOT
	// short-circuit and the `v.GetValue() > 0` guard is the load-bearing branch
	// that falls a configured 0 through to the high default (AMEND-H2-3; no reject).
	t.Run("configured_zero_defaults_to_high", func(t *testing.T) {
		c := mkStaticCluster("c_h2_cap0", mkLbEndpoint("10.0.0.1", 8080))
		c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2WithMaxStreams(0)),
		}
		m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
		if err != nil {
			t.Fatalf("NewManagerWithBaseDir: %v (configured max_concurrent_streams: 0 must NOT reject — reference parity)", err)
		}
		got, ok := m.Get("c_h2_cap0")
		if !ok {
			t.Fatal("cluster c_h2_cap0 not found")
		}
		if got.h2MaxConcurrentStreams != h2DefaultMaxConcurrentStreams {
			t.Errorf("h2MaxConcurrentStreams = %d, want h2DefaultMaxConcurrentStreams (%d) (configured 0 → high default via the > 0 guard)",
				got.h2MaxConcurrentStreams, h2DefaultMaxConcurrentStreams)
		}
	})
}

func TestBuildCluster_H2Mode_TLSWithoutALPNH2(t *testing.T) {
	caPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	c := mkStaticCluster("c_h2_alpn_mismatch", mkLbEndpoint("10.0.0.1", 443))
	c.TransportSocket = mkUpstreamTLSTransportSocketWithALPN(t, "alpha.envoy-go.test", caPEM, []string{"http/1.1"})
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
	}
	_, err = NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "alpn_protocols to include") {
		t.Errorf("error %q does not contain %q", err.Error(), "alpn_protocols to include")
	}
	if !strings.Contains(err.Error(), `"h2"`) {
		t.Errorf("error %q does not mention %q", err.Error(), `"h2"`)
	}
}

func TestBuildCluster_H2Mode_TLSWithoutALPN(t *testing.T) {
	caPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	c := mkStaticCluster("c_h2_no_alpn", mkLbEndpoint("10.0.0.1", 443))
	c.TransportSocket = mkUpstreamTLSTransportSocketWithALPN(t, "alpha.envoy-go.test", caPEM, nil)
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
	}
	_, err = NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "alpn_protocols to include") {
		t.Errorf("error %q does not contain %q", err.Error(), "alpn_protocols to include")
	}
}

func TestBuildCluster_H1Discriminator_SilentIgnore(t *testing.T) {
	c := mkStaticCluster("c_h1", mkLbEndpoint("10.0.0.1", 8080))
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH1()),
	}
	m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManagerWithBaseDir: %v", err)
	}
	got, ok := m.Get("c_h1")
	if !ok {
		t.Fatal("cluster c_h1 not found")
	}
	if got.UseH2() {
		t.Error("UseH2() = true, want false (H1 discriminator → silent-ignore)")
	}
}

func TestBuildCluster_AutoConfig_SilentIgnore(t *testing.T) {
	c := mkStaticCluster("c_auto", mkLbEndpoint("10.0.0.1", 8080))
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoAutoConfig()),
	}
	m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManagerWithBaseDir: %v (auto_config must be silently ignored at 05.2)", err)
	}
	got, ok := m.Get("c_auto")
	if !ok {
		t.Fatal("cluster c_auto not found")
	}
	if got.UseH2() {
		t.Error("UseH2() = true, want false (auto_config → 05.2 silent-ignore narrowing)")
	}
}

func TestBuildCluster_NoTypedExtension_BaselineFalse(t *testing.T) {
	c := mkStaticCluster("c_baseline", mkLbEndpoint("10.0.0.1", 8080))
	// No TypedExtensionProtocolOptions map.
	m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManagerWithBaseDir: %v", err)
	}
	got, ok := m.Get("c_baseline")
	if !ok {
		t.Fatal("cluster c_baseline not found")
	}
	if got.UseH2() {
		t.Error("UseH2() = true, want false (no typed_extension → phase-04 baseline)")
	}
}

func TestBuildCluster_HttpProtocolOptions_NilUpstreamProtocolOptions(t *testing.T) {
	c := mkStaticCluster("c_empty_hpo", mkLbEndpoint("10.0.0.1", 8080))
	// Empty HttpProtocolOptions{} — no UpstreamProtocolOptions oneof set.
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, &upstreamshttpv3.HttpProtocolOptions{}),
	}
	m, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManagerWithBaseDir: %v (empty HttpProtocolOptions must build cleanly)", err)
	}
	got, ok := m.Get("c_empty_hpo")
	if !ok {
		t.Fatal("cluster c_empty_hpo not found")
	}
	if got.UseH2() {
		t.Error("UseH2() = true, want false (nil UpstreamProtocolOptions → defensive false)")
	}
}

// TestBuildCluster_GrpcHealthCheck verifies the phase-39.2 gRPC H2 requirement:
// a cluster with grpc_health_check and NO HTTP/2 support is rejected at
// config-build time; the same cluster WITH http2_protocol_options is accepted.
func TestBuildCluster_GrpcHealthCheck(t *testing.T) {
	// Helper: a grpc_health_check envelope (valid scalars + grpc checker oneof).
	grpcHC := &corev3.HealthCheck{
		Interval:           durationpb.New(time.Second),
		Timeout:            durationpb.New(time.Second),
		UnhealthyThreshold: wrapperspb.UInt32(2),
		HealthyThreshold:   wrapperspb.UInt32(2),
		HealthChecker: &corev3.HealthCheck_GrpcHealthCheck_{
			GrpcHealthCheck: &corev3.HealthCheck_GrpcHealthCheck{ServiceName: ""},
		},
	}

	// Case 1: grpc_health_check + NO HTTP/2 → must be rejected.
	t.Run("no_h2_rejected", func(t *testing.T) {
		c := mkStaticCluster("c_grpc_hc_noh2", mkLbEndpoint("10.0.0.1", 8080))
		c.HealthChecks = []*corev3.HealthCheck{grpcHC}
		// No TypedExtensionProtocolOptions → extractH2Mode returns false.
		_, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
		if err == nil {
			t.Fatal("expected error for grpc_health_check without HTTP/2, got nil")
		}
		want := "grpc_health_check requires the cluster to support HTTP/2"
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err.Error(), want)
		}
	})

	// Case 2: grpc_health_check + plaintext h2c (http2_protocol_options, no
	// transport_socket — ADR-0166) → must succeed; the reject is conditional.
	t.Run("with_h2_accepted", func(t *testing.T) {
		c := mkStaticCluster("c_grpc_hc_h2", mkLbEndpoint("10.0.0.1", 8080))
		c.HealthChecks = []*corev3.HealthCheck{grpcHC}
		c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
		}
		if _, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry()); err != nil {
			t.Fatalf("grpc_health_check + h2c must be accepted: %v", err)
		}
	})
}

func TestNewManager_MixedPlaintextAndTLSClusters(t *testing.T) {
	caPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}

	// Plaintext cluster.
	plain := mkStaticCluster("c_plain", mkLbEndpoint("10.0.0.1", 8080))

	// TLS cluster.
	tls := mkStaticCluster("c_tls", mkLbEndpoint("10.0.0.2", 443))
	tls.TransportSocket = mkUpstreamTLSTransportSocket(t, "alpha.envoy-go.test", caPEM)

	m, err := NewManagerWithBaseDir(mkBootstrap(plain, tls), "", stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManagerWithBaseDir: %v", err)
	}

	gotPlain, ok := m.Get("c_plain")
	if !ok {
		t.Fatal("cluster c_plain not found")
	}
	if gotPlain.upstreamCfg != nil {
		t.Error("c_plain.upstreamCfg should be nil (plaintext)")
	}

	gotTLS, ok := m.Get("c_tls")
	if !ok {
		t.Fatal("cluster c_tls not found")
	}
	if gotTLS.upstreamCfg == nil {
		t.Fatal("c_tls.upstreamCfg should be non-nil")
	}
	if gotTLS.upstreamCfg.ServerName != "alpha.envoy-go.test" {
		t.Errorf("c_tls.upstreamCfg.ServerName = %q, want %q", gotTLS.upstreamCfg.ServerName, "alpha.envoy-go.test")
	}
}

// ---------------------------------------------------------------------------
// Phase 06.1 Task 8 — 8-metric per-cluster allocation [ADR-0063]
// ---------------------------------------------------------------------------

// TestNewManager_AllocatesEightMetricsPerCluster verifies the cluster-side
// per-cluster metric-allocation loop registers exactly the 8 cluster-scope
// metrics from SPEC §6 on the supplied Registry, stores the metric pointers
// on the Cluster struct, and Sets membership_total to len(endpoints) at
// register time. Per ADR-0063 the metric set is cluster-level only;
// per-endpoint expansion is deferred.
func TestNewManager_AllocatesEightMetricsPerCluster(t *testing.T) {
	bs := mkBootstrap(mkStaticCluster("c0", mkLbEndpoint("127.0.0.1", 9001)))
	r := stats.NewRegistry()
	m, err := NewManager(bs, r)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	c, ok := m.Get("c0")
	if !ok {
		t.Fatal("cluster c0 not found")
	}
	// Each metric pointer must be non-nil.
	if c.upstreamRqTotal == nil ||
		c.upstreamRq2xx == nil || c.upstreamRq3xx == nil ||
		c.upstreamRq4xx == nil || c.upstreamRq5xx == nil ||
		c.upstreamCxTotal == nil ||
		c.upstreamCxActive == nil ||
		c.membershipTotal == nil {
		t.Errorf("expected all 8 metric pointers non-nil; got: %+v", c)
	}
	// membership_total Set to 1 at register time (single endpoint).
	if got := c.membershipTotal.Load(); got != 1 {
		t.Errorf("membershipTotal = %d, want 1", got)
	}
	// Walk: 8 metrics must be visible under cluster.c0.* names.
	var seen []string
	r.Walk(func(m stats.Metric) {
		seen = append(seen, m.Name())
	})
	wantNames := map[string]bool{
		"cluster.c0.upstream_rq_total":  true,
		"cluster.c0.upstream_rq_2xx":    true,
		"cluster.c0.upstream_rq_3xx":    true,
		"cluster.c0.upstream_rq_4xx":    true,
		"cluster.c0.upstream_rq_5xx":    true,
		"cluster.c0.upstream_cx_total":  true,
		"cluster.c0.upstream_cx_active": true,
		"cluster.c0.membership_total":   true,
	}
	for _, n := range seen {
		delete(wantNames, n)
	}
	if len(wantNames) != 0 {
		t.Errorf("missing cluster metrics: %v", wantNames)
	}
}

// ---------------------------------------------------------------------------
// Phase 08.1 (Task 3) — Manager.Clusters() snapshot accessor tests
// ---------------------------------------------------------------------------

func TestManager_Clusters_SnapshotReturnsAllClusters(t *testing.T) {
	bs := mkBootstrap(
		mkStaticCluster("c_a",
			mkLbEndpoint("10.0.0.1", 9000),
			mkLbEndpoint("10.0.0.2", 9001),
		),
		mkStaticCluster("c_b",
			mkLbEndpoint("10.0.1.1", 9100),
			mkLbEndpoint("10.0.1.2", 9101),
		),
	)
	m, err := NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	infos := m.Clusters()
	if len(infos) != 2 {
		t.Fatalf("Clusters() returned %d; want 2", len(infos))
	}
	// Alphabetical-by-name ordering invariant
	if infos[0].Name != "c_a" || infos[1].Name != "c_b" {
		t.Errorf("Clusters() ordering: got [%q, %q]; want [c_a, c_b]", infos[0].Name, infos[1].Name)
	}
	// Per-cluster endpoints populated
	if len(infos[0].Endpoints) != 2 {
		t.Errorf("Clusters()[0].Endpoints: got %d; want 2", len(infos[0].Endpoints))
	}
	if infos[0].Endpoints[0].Address == "" || infos[0].Endpoints[0].Port == 0 {
		t.Errorf("Clusters()[0].Endpoints[0]: empty fields: %+v", infos[0].Endpoints[0])
	}
}

func TestManager_Clusters_FreshlyAllocatedPerCall(t *testing.T) {
	bs := mkBootstrap(
		mkStaticCluster("c_a",
			mkLbEndpoint("10.0.0.1", 9000),
			mkLbEndpoint("10.0.0.2", 9001),
		),
		mkStaticCluster("c_b",
			mkLbEndpoint("10.0.1.1", 9100),
			mkLbEndpoint("10.0.1.2", 9101),
		),
	)
	m, _ := NewManager(bs, stats.NewRegistry())
	a := m.Clusters()
	b := m.Clusters()
	// Different slice headers (snapshot semantics)
	if &a[0] == &b[0] {
		t.Errorf("Clusters() returned aliased slice; expect freshly allocated per call")
	}
	// Mutation of returned slice does not affect manager state
	a[0].Name = "MUTATED"
	a[0].Endpoints[0].Address = "MUTATED"
	c := m.Clusters()
	if c[0].Name == "MUTATED" {
		t.Errorf("mutating Clusters() result affected manager state (Name)")
	}
	if c[0].Endpoints[0].Address == "MUTATED" {
		t.Errorf("mutating Clusters() result affected manager state (Endpoint.Address)")
	}
}

func TestManager_Clusters_EmptyClustersListReturnsEmpty(t *testing.T) {
	// NewManager errors on zero clusters per the existing manager.go contract;
	// this test asserts that IF a Manager could ever have zero clusters,
	// Clusters() returns an empty (non-nil) slice. Constructed directly:
	m := &Manager{clusters: map[string]*Cluster{}}
	if got := m.Clusters(); got == nil || len(got) != 0 {
		t.Errorf("Clusters() on empty manager: got %v; want non-nil empty slice", got)
	}
}

// ---------------------------------------------------------------------------
// Phase 08.2 (Task 4) — Manager.Drain() + Cluster.closePool() [ADR-0096]
// ---------------------------------------------------------------------------

func TestManager_Drain_ClosesPools(t *testing.T) {
	bs := mkBootstrap(
		mkStaticCluster("c_drain", mkLbEndpoint("127.0.0.1", 9000)),
	)
	m, err := NewManager(bs, stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	// Drain is a best-effort pool close; we assert no panic + idempotency.
	m.Drain()
	// Subsequent Drain calls must be safe (no double-close panics).
	m.Drain()
}

func TestManager_Drain_Idempotent(t *testing.T) {
	bs := mkBootstrap(
		mkStaticCluster("c_drain_idem", mkLbEndpoint("127.0.0.1", 9001)),
	)
	m, _ := NewManager(bs, stats.NewRegistry())
	for i := 0; i < 10; i++ {
		m.Drain()
	}
	// No assertions beyond "did not panic"; closePool stubs may grow more
	// invariants in future hot-restart family work (per SPEC §2.1 deferral).
}

func TestManager_Drain_EmptyClusterList(t *testing.T) {
	m := &Manager{clusters: map[string]*Cluster{}}
	m.Drain() // must not panic on empty map
}

// ---------------------------------------------------------------------------
// Phase 36.1 Task 5: RING_HASH acceptance, gate, and gauge wiring.
// ---------------------------------------------------------------------------

// gaugeValue reads back a gauge by exact name via the scrape-time Walk seam.
func gaugeValue(reg *stats.Registry, name string) (int64, bool) {
	var v int64
	var found bool
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			found = true
			if g, ok := m.(*stats.Gauge); ok {
				v = g.Load()
			}
		}
	})
	return v, found
}

// counterValue reads back a counter by exact name via the scrape-time Walk seam.
// Mirrors gaugeValue; returns (Load(), true) if the metric exists and is a *Counter.
func counterValue(reg *stats.Registry, name string) (uint64, bool) {
	var v uint64
	var found bool
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			found = true
			if c, ok := m.(*stats.Counter); ok {
				v = c.Load()
			}
		}
	})
	return v, found
}

// hasMetric reports whether a metric with the given exact name is registered in reg.
func hasMetric(reg *stats.Registry, name string) bool {
	var found bool
	reg.Walk(func(m stats.Metric) {
		if m.Name() == name {
			found = true
		}
	})
	return found
}

// tag sets lb_endpoint i's envoy.lb metadata to {key: val} (a string scalar).
// Used by subset-stats tests to add envoy.lb metadata to pre-built clusters.
// The cluster must have been built with mkStaticCluster — single locality group
// (Endpoints[0].LbEndpoints[i]).
func tag(c *clusterv3.Cluster, i int, key, val string) {
	lbe := c.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[i]
	md, _ := structpb.NewStruct(map[string]any{key: val})
	lbe.Metadata = &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"envoy.lb": md}}
}

func TestManager_Accept_RingHash_Defaults(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	m, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("RING_HASH bare must be accepted: %v", err)
	}
	if _, ok := m.Get("c_rh"); !ok {
		t.Fatal("cluster c_rh not found")
	}
}

func TestManager_Accept_RingHash_NonDefaultValid(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MinimumRingSize: wrapperspb.UInt64(64), MaximumRingSize: wrapperspb.UInt64(128),
		HashFunction: clusterv3.Cluster_RingHashLbConfig_MURMUR_HASH_2,
	}}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("valid non-default ring_hash_lb_config must be accepted: %v", err)
	}
}

func TestManager_Reject_RingHash_MinOverCap(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MinimumRingSize: wrapperspb.UInt64(9000000)}}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "minimum_ring_size: value must be less than or equal to 8388608") {
		t.Errorf("err = %v, want PGV min-over-cap reject", err)
	}
}

func TestManager_Reject_RingHash_MaxOverCap(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MaximumRingSize: wrapperspb.UInt64(9000000)}}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "maximum_ring_size: value must be less than or equal to 8388608") {
		t.Errorf("err = %v, want PGV max-over-cap reject", err)
	}
}

func TestManager_Reject_RingHash_MinOverMax(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MinimumRingSize: wrapperspb.UInt64(5), MaximumRingSize: wrapperspb.UInt64(2)}}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "ring hash: minimum_ring_size (5) > maximum_ring_size (2)") {
		t.Errorf("err = %v, want runtime min>max reject", err)
	}
}

func TestManager_Accept_RingHash_MismatchedOneof(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	c.LbConfig = &clusterv3.Cluster_LeastRequestLbConfig_{LeastRequestLbConfig: &clusterv3.Cluster_LeastRequestLbConfig{ChoiceCount: wrapperspb.UInt32(7)}}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("mismatched oneof under RING_HASH must be silently accepted (defaults): %v", err)
	}
}

func TestManager_RingHash_RegistersGauges(t *testing.T) {
	c := mkStaticCluster("c_rh", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_RING_HASH
	reg := stats.NewRegistry()
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]int64{
		"cluster.c_rh.ring_hash_lb.size":                1026,
		"cluster.c_rh.ring_hash_lb.min_hashes_per_host": 342,
		"cluster.c_rh.ring_hash_lb.max_hashes_per_host": 342,
	} {
		got, ok := gaugeValue(reg, name)
		if !ok {
			t.Errorf("gauge %q not registered", name)
		} else if got != want {
			t.Errorf("gauge %q = %d, want %d", name, got, want)
		}
	}
}

func TestManager_NonRingHash_NoGauges(t *testing.T) {
	c := mkStaticCluster("c_rr", mkLbEndpoint("127.0.0.1", 9001))
	reg := stats.NewRegistry()
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	if _, ok := gaugeValue(reg, "cluster.c_rr.ring_hash_lb.size"); ok {
		t.Error("ROUND_ROBIN cluster must register NO ring_hash_lb gauges (RING_HASH-only, D-S36-6)")
	}
}

func TestManager_Accept_Maglev_Defaults(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_MAGLEV // no maglev_lb_config → default table_size 65537
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Fatalf("MAGLEV bare must be accepted: %v", err)
	}
}

func TestManager_Accept_Maglev_NonDefaultPrime(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	c.LbConfig = &clusterv3.Cluster_MaglevLbConfig_{MaglevLbConfig: &clusterv3.Cluster_MaglevLbConfig{
		TableSize: wrapperspb.UInt64(127)}} // 127 is prime, <= cap
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("valid prime table_size must be accepted: %v", err)
	}
}

func TestManager_Reject_Maglev_TableSizeOverCap(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	c.LbConfig = &clusterv3.Cluster_MaglevLbConfig_{MaglevLbConfig: &clusterv3.Cluster_MaglevLbConfig{
		TableSize: wrapperspb.UInt64(5000012)}} // > cap
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "table_size: value must be less than or equal to 5000011") {
		t.Errorf("err = %v, want PGV cap reject", err)
	}
}

func TestManager_Reject_Maglev_TableSizeNotPrime(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	c.LbConfig = &clusterv3.Cluster_MaglevLbConfig_{MaglevLbConfig: &clusterv3.Cluster_MaglevLbConfig{
		TableSize: wrapperspb.UInt64(100)}} // composite — the reference rejects (AMEND-M5)
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "must be a prime number") {
		t.Errorf("err = %v, want primality reject", err)
	}
}

func TestManager_Accept_Maglev_MismatchedOneof(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	c.LbConfig = &clusterv3.Cluster_RingHashLbConfig_{RingHashLbConfig: &clusterv3.Cluster_RingHashLbConfig{
		MinimumRingSize: wrapperspb.UInt64(64)}}
	if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
		t.Errorf("mismatched oneof under MAGLEV must be silently accepted (default table_size): %v", err)
	}
}

func TestManager_Maglev_RegistersGauges(t *testing.T) {
	c := mkStaticCluster("c_mg", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002), mkLbEndpoint("127.0.0.1", 9003))
	c.LbPolicy = clusterv3.Cluster_MAGLEV
	reg := stats.NewRegistry()
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	// 65537 slots over 3 hosts → 21845 / 21846 (D-M4); NO maglev_lb.size gauge.
	for name, want := range map[string]int64{
		"cluster.c_mg.maglev_lb.min_entries_per_host": 21845,
		"cluster.c_mg.maglev_lb.max_entries_per_host": 21846,
	} {
		got, ok := gaugeValue(reg, name) // the REUSED ring_hash readback helper
		if !ok {
			t.Errorf("gauge %q not registered", name)
		} else if got != want {
			t.Errorf("gauge %q = %d, want %d", name, got, want)
		}
	}
	if _, ok := gaugeValue(reg, "cluster.c_mg.maglev_lb.size"); ok {
		t.Error("maglev must register NO size gauge (the table size is config-known — D-M4)")
	}
}

func TestManager_NonMaglev_NoGauges(t *testing.T) {
	// the gauges are MAGLEV-only (reference parity) — a ROUND_ROBIN cluster registers none.
	c := mkStaticCluster("c_rr", mkLbEndpoint("127.0.0.1", 9001))
	reg := stats.NewRegistry()
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	if _, ok := gaugeValue(reg, "cluster.c_rr.maglev_lb.min_entries_per_host"); ok {
		t.Error("ROUND_ROBIN cluster must register NO maglev_lb gauges (MAGLEV-only)")
	}
}

// ---------------------------------------------------------------------------
// Phase 38.1 Task 3 — Endpoint.Metadata + extractEndpoints envoy.lb capture
// ---------------------------------------------------------------------------

func TestExtractEndpoints_CapturesEnvoyLbScalarMetadata(t *testing.T) {
	// A LbEndpoint carrying envoy.lb {version:"v1", weight:7, canary:true} →
	// Endpoint.Metadata with the 3 scalars; a non-scalar value is DROPPED.
	md, _ := structpb.NewStruct(map[string]any{
		"version": "v1", "weight": float64(7), "canary": true,
		"nested": map[string]any{"x": 1}, // non-scalar → dropped
	})
	lbe := mkLbEndpoint("127.0.0.1", 9001)
	lbe.Metadata = &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"envoy.lb": md}}
	c := mkStaticClusterFromLbEndpoints("c_md", lbe)
	eps, err := extractEndpoints(c.GetLoadAssignment(), "c_md")
	if err != nil {
		t.Fatal(err)
	}
	got := eps[0].Metadata
	if got["version"].Str != "v1" || got["weight"].Num != 7 || !got["canary"].Bool {
		t.Errorf("captured metadata wrong: %+v", got)
	}
	if _, ok := got["nested"]; ok {
		t.Error("non-scalar 'nested' must be dropped")
	}
}

func TestExtractEndpoints_NoMetadataIsNilMap(t *testing.T) {
	c := mkStaticCluster("c_plain", mkLbEndpoint("127.0.0.1", 9001))
	eps, err := extractEndpoints(c.GetLoadAssignment(), "c_plain")
	if err != nil {
		t.Fatal(err)
	}
	if eps[0].Metadata != nil {
		t.Errorf("absent envoy.lb metadata → nil map, got %v", eps[0].Metadata)
	}
}

func TestEndpoint_AddrIgnoresMetadata(t *testing.T) {
	a := Endpoint{Host: "127.0.0.1", Port: 9001}
	b := Endpoint{Host: "127.0.0.1", Port: 9001, Metadata: map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}}
	if a.Addr() != b.Addr() {
		t.Errorf("Addr() must ignore Metadata: %q vs %q", a.Addr(), b.Addr())
	}
}

// ---------------------------------------------------------------------------
// buildLeafLB factory tests (Task 5)
// ---------------------------------------------------------------------------

func TestBuildLeafLB_BuildsEachPolicyOverSubSlice(t *testing.T) {
	eps := []Endpoint{{Host: "127.0.0.1", Port: 9001}, {Host: "127.0.0.1", Port: 9002}}
	for _, pol := range []clusterv3.Cluster_LbPolicy{
		clusterv3.Cluster_ROUND_ROBIN, clusterv3.Cluster_LEAST_REQUEST,
		clusterv3.Cluster_RANDOM, clusterv3.Cluster_RING_HASH, clusterv3.Cluster_MAGLEV,
	} {
		c := &clusterv3.Cluster{Name: "c", LbPolicy: pol}
		lb, err := buildLeafLB(c, "c", eps[:1], nil)
		if err != nil {
			t.Errorf("buildLeafLB(%v) over sub-slice: %v", pol, err)
			continue
		}
		ep, _, err := lb.Pick(123, true, SubsetMatch{}, false)
		if err != nil || ep.Port != 9001 {
			t.Errorf("buildLeafLB(%v).Pick: ep=%v err=%v, want port 9001", pol, ep, err)
		}
	}
}

func TestBuildLeafLB_RejectsClusterProvided(t *testing.T) {
	// CLUSTER_PROVIDED (and every unsupported policy) rejects in the factory —
	// BEFORE any subset wrap. This is why lb_subset_config + CLUSTER_PROVIDED
	// rejects in outcome with ZERO new reject arm.
	c := &clusterv3.Cluster{Name: "c", LbPolicy: clusterv3.Cluster_CLUSTER_PROVIDED}
	_, err := buildLeafLB(c, "c", []Endpoint{{Host: "127.0.0.1", Port: 9001}}, nil)
	if err == nil || !strings.Contains(err.Error(), "unsupported lb_policy") {
		t.Errorf("err = %v, want unsupported lb_policy reject", err)
	}
}

// ---------------------------------------------------------------------------
// Phase 38.1 Task 8 — parseLbSubsetConfig + wrap-after-switch (§6 matrix)
// ---------------------------------------------------------------------------

func TestManager_Accept_LbSubsetConfig_WrapsChild(t *testing.T) {
	c := mkStaticCluster("c_sub", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002))
	c.LbPolicy = clusterv3.Cluster_ROUND_ROBIN
	c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{
		FallbackPolicy:  clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT,
		SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{"version"}}},
	}
	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("lb_subset_config under ROUND_ROBIN must be accepted: %v", err)
	}
	cl, _ := mgr.Get("c_sub")
	if _, ok := cl.lb.(*subsetLB); !ok {
		t.Errorf("lb must be wrapped in *subsetLB, got %T", cl.lb)
	}
}

func TestManager_Reject_LbSubsetConfig_ClusterProvided(t *testing.T) {
	c := mkStaticCluster("c_sub", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_CLUSTER_PROVIDED
	c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{FallbackPolicy: clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "unsupported lb_policy") {
		t.Errorf("err = %v, want the pre-existing unsupported-policy reject", err)
	}
}

func TestManager_Accept_LbSubsetConfig_BenignShapes(t *testing.T) {
	// empty keys, empty selectors, duplicate selectors, fallback-default mismatch ALL accept.
	base := func() *clusterv3.Cluster {
		c := mkStaticCluster("c_sub", mkLbEndpoint("127.0.0.1", 9001))
		c.LbPolicy = clusterv3.Cluster_ROUND_ROBIN
		return c
	}
	cases := []*clusterv3.Cluster_LbSubsetConfig{
		{SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{}}}},
		{}, // empty subset_selectors + NO_FALLBACK default
		{SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{"v"}}, {Keys: []string{"v"}}}},
		{FallbackPolicy: clusterv3.Cluster_LbSubsetConfig_DEFAULT_SUBSET}, // DEFAULT_SUBSET, no default_subset
	}
	for i, sc := range cases {
		c := base()
		c.LbSubsetConfig = sc
		if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
			t.Errorf("case %d must accept (parity): %v", i, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 38.1 Task 9 — 4 lb_subsets_* stats registration + counter injection
// ---------------------------------------------------------------------------

func TestRegisterClusterMetrics_SubsetStats(t *testing.T) {
	reg := stats.NewRegistry()
	c := mkStaticCluster("c_sub", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002))
	c.LbPolicy = clusterv3.Cluster_ROUND_ROBIN
	tag(c, 0, "version", "v1") // set envoy.lb metadata on lb_endpoint i
	tag(c, 1, "version", "v2")
	c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{
		FallbackPolicy:  clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT,
		SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{"version"}}},
	}
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	// active/created = the distinct-subset count ×1 (NOT 33×)
	if got, ok := gaugeValue(reg, "cluster.c_sub.lb_subsets_active"); !ok || got != 2 {
		t.Errorf("lb_subsets_active: registered=%v value=%d, want registered=true value=2 (×1 distinct-subset count)", ok, got)
	}
	if got, ok := counterValue(reg, "cluster.c_sub.lb_subsets_created"); !ok || got != 2 {
		t.Errorf("lb_subsets_created: registered=%v value=%d, want registered=true value=2", ok, got)
	}
	// selected/fallback are injected onto the live subsetLB wrapper and must be
	// registered (ok==true) and start at zero. The prior code discarded the ok
	// bool, making these checks vacuous when the metric was never registered.
	if s0, ok := counterValue(reg, "cluster.c_sub.lb_subsets_selected"); !ok || s0 != 0 {
		t.Errorf("lb_subsets_selected: registered=%v value=%d, want registered=true value=0", ok, s0)
	}
	if f0, ok := counterValue(reg, "cluster.c_sub.lb_subsets_fallback"); !ok || f0 != 0 {
		t.Errorf("lb_subsets_fallback: registered=%v value=%d, want registered=true value=0", ok, f0)
	}
}

func TestRegisterClusterMetrics_NonSubsetNoSubsetStats(t *testing.T) {
	reg := stats.NewRegistry()
	c := mkStaticCluster("c_plain", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_ROUND_ROBIN // no lb_subset_config
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	// All four lb_subsets_* names must be absent for a non-subset cluster so that
	// a regression registering even a subset of them is caught (fuller coverage).
	for _, suffix := range []string{"lb_subsets_active", "lb_subsets_created", "lb_subsets_selected", "lb_subsets_fallback"} {
		name := "cluster.c_plain." + suffix
		if hasMetric(reg, name) {
			t.Errorf("a non-subset cluster must NOT register %s", name)
		}
	}
}

// ---------------------------------------------------------------------------
// Phase 43.2a Task 6 — register upstream_cx_http2_total + http2.streams_active
// (useH2-gated; non-H2 clusters stay byte-stable; AMEND-H2-2)
// ---------------------------------------------------------------------------

// TestRegisterClusterMetrics_H2Stats verifies that registerClusterMetrics
// registers exactly the 2 H2 pool stats on H2-upstream clusters (useH2-gated)
// and registers NEITHER on non-H2 clusters (byte-stability).
//
// H2 cluster: upstream_cx_http2_total (counter) + http2.streams_active (gauge)
// MUST be present, and upstream_cx_http2_active MUST be absent (AMEND-H2-2 —
// it does not exist in the reference). Non-H2 cluster: NEITHER name is present.
func TestRegisterClusterMetrics_H2Stats(t *testing.T) {
	t.Run("h2_cluster_has_two_stats", func(t *testing.T) {
		// Build a plaintext h2c cluster (ADR-0166: no transport_socket needed).
		c := mkStaticCluster("c_h2s", mkLbEndpoint("10.0.0.1", 8080))
		c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
			httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
		}
		reg := stats.NewRegistry()
		m, err := NewManagerWithBaseDir(mkBootstrap(c), "", reg)
		if err != nil {
			t.Fatalf("NewManagerWithBaseDir: %v", err)
		}
		cl, ok := m.Get("c_h2s")
		if !ok {
			t.Fatal("cluster c_h2s not found")
		}

		// The two H2 handles must be bound on the cluster struct.
		if cl.upstreamCxHTTP2Total == nil {
			t.Error("upstreamCxHTTP2Total is nil; want non-nil after registerClusterMetrics (H2 cluster)")
		}
		if cl.http2StreamsActive == nil {
			t.Error("http2StreamsActive is nil; want non-nil after registerClusterMetrics (H2 cluster)")
		}

		// The two names must be present in the registry.
		if !hasMetric(reg, "cluster.c_h2s.upstream_cx_http2_total") {
			t.Error("cluster.c_h2s.upstream_cx_http2_total not registered (H2 cluster)")
		}
		if !hasMetric(reg, "cluster.c_h2s.http2.streams_active") {
			t.Error("cluster.c_h2s.http2.streams_active not registered (H2 cluster)")
		}

		// upstream_cx_http2_active MUST NOT be registered (AMEND-H2-2 — it does
		// not exist in the reference; there is no _http2_active name).
		if hasMetric(reg, "cluster.c_h2s.upstream_cx_http2_active") {
			t.Error("cluster.c_h2s.upstream_cx_http2_active registered but must NOT exist (AMEND-H2-2)")
		}

		// Phase 43.2b (ADR-0254, AMEND-H2B-2): the 2 reset counters
		// http2.rx_reset + http2.tx_reset are useH2-gated and wired from the
		// codec via h2.WithResetHooks at dial. Handles bound + names present.
		if cl.http2RxReset == nil {
			t.Error("http2RxReset is nil; want non-nil after registerClusterMetrics (H2 cluster)")
		}
		if cl.http2TxReset == nil {
			t.Error("http2TxReset is nil; want non-nil after registerClusterMetrics (H2 cluster)")
		}
		if !hasMetric(reg, "cluster.c_h2s.http2.rx_reset") {
			t.Error("cluster.c_h2s.http2.rx_reset not registered (H2 cluster)")
		}
		if !hasMetric(reg, "cluster.c_h2s.http2.tx_reset") {
			t.Error("cluster.c_h2s.http2.tx_reset not registered (H2 cluster)")
		}
	})

	t.Run("non_h2_cluster_has_no_h2_stats", func(t *testing.T) {
		// A plain non-H2 cluster must not register either H2 stat name
		// (byte-stability: non-H2 fixture stat counts are unchanged).
		c := mkStaticCluster("c_h1s", mkLbEndpoint("10.0.0.1", 8080))
		// No TypedExtensionProtocolOptions → useH2 = false.
		reg := stats.NewRegistry()
		m, err := NewManagerWithBaseDir(mkBootstrap(c), "", reg)
		if err != nil {
			t.Fatalf("NewManagerWithBaseDir: %v", err)
		}
		for _, name := range []string{
			"cluster.c_h1s.upstream_cx_http2_total",
			"cluster.c_h1s.http2.streams_active",
			"cluster.c_h1s.upstream_cx_http2_active",
			// Phase 43.2b (ADR-0254): the reset counters are useH2-gated too.
			"cluster.c_h1s.http2.rx_reset",
			"cluster.c_h1s.http2.tx_reset",
		} {
			if hasMetric(reg, name) {
				t.Errorf("non-H2 cluster must NOT register %q (byte-stability)", name)
			}
		}
		// The handles must stay nil on a non-H2 cluster.
		cl, ok := m.Get("c_h1s")
		if !ok {
			t.Fatal("cluster c_h1s not found")
		}
		if cl.http2RxReset != nil {
			t.Error("http2RxReset must be nil on a non-H2 cluster (byte-stability)")
		}
		if cl.http2TxReset != nil {
			t.Error("http2TxReset must be nil on a non-H2 cluster (byte-stability)")
		}
	})
}

func TestSubsetLB_InjectedCountersIncFromManager(t *testing.T) {
	// End-to-end: build via the manager, drive the cluster's lb.Pick through a
	// match, confirm the INJECTED selected/fallback counters increment in the registry.
	reg := stats.NewRegistry()
	c := mkStaticCluster("c_sub2", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002))
	c.LbPolicy = clusterv3.Cluster_ROUND_ROBIN
	tag(c, 0, "version", "v1")
	tag(c, 1, "version", "v2")
	c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{
		FallbackPolicy:  clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT,
		SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{"version"}}},
	}
	mgr, err := NewManager(mkBootstrap(c), reg)
	if err != nil {
		t.Fatal(err)
	}
	cl, _ := mgr.Get("c_sub2")
	// a subset hit
	if _, _, err := cl.lb.Pick(0, false, NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}), true); err != nil {
		t.Fatalf("Pick (subset hit): %v", err)
	}
	// a fallback (v9 misses — ANY_ENDPOINT fallback picks one of the 2 endpoints)
	if _, _, err := cl.lb.Pick(0, false, NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v9"}}), true); err != nil {
		t.Fatalf("Pick (fallback): %v", err)
	}
	if got, _ := counterValue(reg, "cluster.c_sub2.lb_subsets_selected"); got != 1 {
		t.Errorf("selected = %d, want 1 (injection works end-to-end)", got)
	}
	if got, _ := counterValue(reg, "cluster.c_sub2.lb_subsets_fallback"); got != 1 {
		t.Errorf("fallback = %d, want 1", got)
	}
}
