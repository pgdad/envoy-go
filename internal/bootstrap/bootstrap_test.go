package bootstrap

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"
)

const sampleBootstrap = `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tcp
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tcp
                cluster: c_echo
  clusters:
    - name: c_echo
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`

func TestLoad_HappyPath(t *testing.T) {
	bs, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if bs == nil {
		t.Fatal("Load returned nil bootstrap with nil error")
	}
	if got, want := bs.GetNode().GetId(), "test-node"; got != want {
		t.Errorf("node.id: got %q, want %q", got, want)
	}
	if got := bs.GetStaticResources(); got == nil {
		t.Fatal("static_resources missing")
	}
	if n := len(bs.GetStaticResources().GetListeners()); n != 1 {
		t.Errorf("listeners: got %d, want 1", n)
	}
	if n := len(bs.GetStaticResources().GetClusters()); n != 1 {
		t.Errorf("clusters: got %d, want 1", n)
	}
}

func TestLoad_RejectsDynamicResources(t *testing.T) {
	yaml := sampleBootstrap + `
dynamic_resources:
  ads_config:
    api_type: GRPC
`
	_, err := Load(strings.NewReader(yaml))
	if err == nil {
		t.Fatal("Load: want error for dynamic_resources, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: ") {
		t.Errorf("error prefix: got %q, want to start with \"bootstrap: \"", err.Error())
	}
	if !strings.Contains(err.Error(), "dynamic_resources") {
		t.Errorf("error should name dynamic_resources: %q", err.Error())
	}
}

func TestLoad_RejectsLayeredRuntime(t *testing.T) {
	yaml := sampleBootstrap + `
layered_runtime:
  layers:
    - name: static_layer
      static_layer: {}
`
	_, err := Load(strings.NewReader(yaml))
	if err == nil {
		t.Fatal("Load: want error for layered_runtime, got nil")
	}
	if !strings.Contains(err.Error(), "layered_runtime") {
		t.Errorf("error should name layered_runtime: %q", err.Error())
	}
}

func TestLoad_YAMLSyntaxError(t *testing.T) {
	_, err := Load(strings.NewReader("not: valid: yaml: at all: :::"))
	if err == nil {
		t.Fatal("Load: want yaml parse error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: yaml parse:") {
		t.Errorf("error prefix: %q", err.Error())
	}
}

func TestLoad_UnknownTopLevelField(t *testing.T) {
	yaml := sampleBootstrap + "\nnot_a_real_field: 42\n"
	_, err := Load(strings.NewReader(yaml))
	if err == nil {
		t.Fatal("Load: want unknown-field error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: protojson:") {
		t.Errorf("error prefix: %q (expected protojson rejection)", err.Error())
	}
}

func TestLoad_EmptyDocument(t *testing.T) {
	_, err := Load(strings.NewReader(""))
	if err == nil {
		t.Fatal("Load: want empty-doc error, got nil")
	}
	if !strings.Contains(err.Error(), "empty document") {
		t.Errorf("error: %q", err.Error())
	}
}

func TestAdminSocket_HappyPath(t *testing.T) {
	bs, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	host, port, err := AdminSocket(bs)
	if err != nil {
		t.Fatalf("AdminSocket: %v", err)
	}
	if host != "127.0.0.1" {
		t.Errorf("host: got %q, want 127.0.0.1", host)
	}
	if port != 0 {
		t.Errorf("port: got %d, want 0", port)
	}
}

func TestAdminSocket_MissingAdmin(t *testing.T) {
	yaml := `
static_resources:
  listeners: []
  clusters: []
`
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, _, err = AdminSocket(bs)
	if err == nil {
		t.Fatal("want error for missing admin, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: ") {
		t.Errorf("prefix: %q", err.Error())
	}
}

// TestFirstListenerSocket_* and TestFirstClusterEndpointSocket_* retired in
// phase 02 — the `First*` helpers themselves are gone (ADR-0022). Listener and
// cluster bootstrap traversal is now covered by `internal/listener` and
// `internal/cluster` manager tests.

// TestBootstrap_RoundTrips_FixtureFour_Shape exercises the phase-05.2 (Task 10)
// blank import of `envoy/extensions/upstreams/http/v3`: a minimal bootstrap
// carrying a cluster with `typed_extension_protocol_options.HttpProtocolOptions
// = explicit_http_config { http2_protocol_options {} }` plus a TLS
// transport_socket and ALPN ["h2"] must load cleanly via Load (no protojson
// "type not registered" error) AND survive a protojson.Marshal round-trip.
//
// The shape mirrors fixture 0004's `c_h2_backend` cluster (per SPEC §4.2) but
// is kept minimal here — fixture 0004 itself lives in test/fixtures/0004-* and
// is materialized in Task 13.
func TestBootstrap_RoundTrips_FixtureFour_Shape(t *testing.T) {
	yamlSrc := `
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  clusters:
    - name: c_h2_backend
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_h2_backend
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 10.0.0.1, port_value: 443 }
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
          sni: backend.envoy-go.test
          common_tls_context:
            alpn_protocols: ["h2"]
      typed_extension_protocol_options:
        envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
          "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
          explicit_http_config:
            http2_protocol_options: {}
`
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	clusters := bs.GetStaticResources().GetClusters()
	if len(clusters) != 1 {
		t.Fatalf("clusters: got %d, want 1", len(clusters))
	}
	c := clusters[0]
	if got, want := c.GetName(), "c_h2_backend"; got != want {
		t.Errorf("cluster.name: got %q, want %q", got, want)
	}
	tepo := c.GetTypedExtensionProtocolOptions()
	if tepo == nil {
		t.Fatal("typed_extension_protocol_options: nil; expected map with HttpProtocolOptions key")
	}
	hpoAny, ok := tepo["envoy.extensions.upstreams.http.v3.HttpProtocolOptions"]
	if !ok {
		t.Fatal("typed_extension_protocol_options: missing HttpProtocolOptions key")
	}
	if hpoAny.GetTypeUrl() != "type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions" {
		t.Errorf("typed_extension type_url: got %q", hpoAny.GetTypeUrl())
	}
	// Round-trip via protojson — would error if the proto descriptor were
	// unregistered.
	if _, err := protojson.Marshal(bs); err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
}

func TestLoad_HCMRoundTrip(t *testing.T) {
	yamlSrc := `
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.GetStaticResources().GetListeners()); got != 1 {
		t.Fatalf("listeners: got %d, want 1", got)
	}
	if _, err := protojson.Marshal(bs); err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
}
