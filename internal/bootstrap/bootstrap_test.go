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
