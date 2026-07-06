package boot

import (
	"strings"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/internal/bootstrap"
	"github.com/pgdad/envoy-go/internal/cluster"
	"github.com/pgdad/envoy-go/internal/drain"
	"github.com/pgdad/envoy-go/internal/grpcclient"
	"github.com/pgdad/envoy-go/internal/httpclient"
	"github.com/pgdad/envoy-go/internal/listener"
)

const validYAML = `
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

func mustConstruct(t *testing.T, yaml string) (*listener.Manager, error) {
	t.Helper()
	bs, err := bootstrap.Load(strings.NewReader(yaml))
	if err != nil {
		t.Fatalf("bootstrap.Load: %v", err)
	}
	cm, err := cluster.NewManagerWithBaseDir(bs.Proto, t.TempDir(), bs.Stats)
	if err != nil {
		t.Fatalf("cluster.NewManagerWithBaseDir: %v", err)
	}
	dm := drain.New(30 * time.Second)
	httpClient := httpclient.New(httpclient.Options{Timeout: 30 * time.Second})
	dialer := grpcclient.New(cm)
	tracingProvider := NewTracingProvider(dialer, httpClient, cm, bs.Stats)
	return Construct(bs, cm, t.TempDir(), false, nil, dm, httpClient, tracingProvider)
}

func TestConstruct_ValidBootstrap_ReturnsNilError(t *testing.T) {
	lm, err := mustConstruct(t, validYAML)
	if err != nil {
		t.Fatalf("Construct: got error %v, want nil", err)
	}
	if lm == nil {
		t.Fatal("Construct: got nil *listener.Manager on success, want non-nil")
	}
}

func TestConstruct_LuaCompileFailure_WrapsWithScriptLoadErrorPrefix(t *testing.T) {
	luaYAML := `
node: { id: test-node, cluster: test-cluster }
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
                stat_prefix: hcm_local
                route_config:
                  name: rc
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_echo }
                http_filters:
                  - name: envoy.filters.http.lua
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua
                      default_source_code:
                        inline_string: "this is not ((( valid lua syntax"
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
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
	_, err := mustConstruct(t, luaYAML)
	if err == nil {
		t.Fatal("Construct: want error for invalid Lua syntax, got nil")
	}
	if !strings.Contains(err.Error(), "script load error: ") {
		t.Errorf("error should contain the script-load-error wrap prefix: %q", err.Error())
	}
}
