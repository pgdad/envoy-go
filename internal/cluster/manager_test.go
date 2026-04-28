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

	"github.com/esalaine/envoy-go/internal/stats"
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

func TestManager_Error_NonRoundRobinLB(t *testing.T) {
	c := mkStaticCluster("c_lr", mkLbEndpoint("127.0.0.1", 8080))
	c.LbPolicy = clusterv3.Cluster_LEAST_REQUEST
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
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

func TestBuildCluster_H2Mode_NoTLS(t *testing.T) {
	c := mkStaticCluster("c_h2_no_tls", mkLbEndpoint("10.0.0.1", 443))
	// No TransportSocket.
	c.TypedExtensionProtocolOptions = map[string]*anypb.Any{
		httpProtocolOptionsKey: mkHttpProtocolOptionsAny(t, hpoExplicitH2()),
	}
	_, err := NewManagerWithBaseDir(mkBootstrap(c), "", stats.NewRegistry())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "requires transport_socket") {
		t.Errorf("error %q does not contain %q", err.Error(), "requires transport_socket")
	}
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
