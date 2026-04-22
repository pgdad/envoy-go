package bootstrap

import (
	"strings"
	"testing"
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

func TestFirstListenerSocket_HappyPath(t *testing.T) {
	bs, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	host, port, err := FirstListenerSocket(bs)
	if err != nil {
		t.Fatalf("FirstListenerSocket: %v", err)
	}
	if host != "127.0.0.1" || port != 0 {
		t.Errorf("got %s:%d, want 127.0.0.1:0", host, port)
	}
}

func TestFirstListenerSocket_ZeroListeners(t *testing.T) {
	yaml := `
admin: { address: { socket_address: { address: 127.0.0.1, port_value: 0 } } }
static_resources: { listeners: [], clusters: [] }
`
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, _, err = FirstListenerSocket(bs)
	if err == nil || !strings.Contains(err.Error(), "expected exactly one listener") {
		t.Errorf("err: %v", err)
	}
}

func TestFirstListenerSocket_TwoListeners(t *testing.T) {
	// Two-listener YAML (add a second listener before the existing one).
	yaml := strings.Replace(sampleBootstrap,
		"  listeners:\n    - name: l_tcp",
		"  listeners:\n    - name: l_a\n    - name: l_tcp", 1)
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, _, err = FirstListenerSocket(bs)
	if err == nil || !strings.Contains(err.Error(), "got 2") {
		t.Errorf("err: %v", err)
	}
}

func TestFirstClusterEndpointSocket_HappyPath(t *testing.T) {
	bs, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	host, port, err := FirstClusterEndpointSocket(bs)
	if err != nil {
		t.Fatalf("FirstClusterEndpointSocket: %v", err)
	}
	if host != "127.0.0.1" || port != 0 {
		t.Errorf("got %s:%d, want 127.0.0.1:0", host, port)
	}
}

func TestFirstClusterEndpointSocket_EmptyEndpoints(t *testing.T) {
	yaml := `
admin: { address: { socket_address: { address: 127.0.0.1, port_value: 0 } } }
static_resources:
  listeners:
    - name: l
      address: { socket_address: { address: 127.0.0.1, port_value: 0 } }
  clusters:
    - name: c
      type: STATIC
      load_assignment: { cluster_name: c, endpoints: [] }
`
	bs, err := Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, _, err = FirstClusterEndpointSocket(bs)
	if err == nil {
		t.Fatal("want error for empty endpoints, got nil")
	}
	if !strings.Contains(err.Error(), "endpoints entry") {
		t.Errorf("error should name endpoints: %q", err.Error())
	}
}
