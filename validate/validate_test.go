package validate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const sampleValidBootstrap = `
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

func TestBootstrap_ValidConfig_ReturnsNil(t *testing.T) {
	if err := Bootstrap(strings.NewReader(sampleValidBootstrap), t.TempDir(), false); err != nil {
		t.Fatalf("Bootstrap: got %v, want nil", err)
	}
}

// --- REUSED from internal/bootstrap/bootstrap_test.go's Load-level reject arms ---

func TestBootstrap_ReusesLoad_RejectsDynamicResources(t *testing.T) {
	yaml := sampleValidBootstrap + `
dynamic_resources:
  ads_config:
    api_type: GRPC
`
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for dynamic_resources, got nil")
	}
	if !strings.Contains(err.Error(), "dynamic_resources") {
		t.Errorf("error should name dynamic_resources: %q", err.Error())
	}
}

func TestBootstrap_ReusesLoad_AcceptsStaticLayer(t *testing.T) {
	// ⚠️ REPLACES TestBootstrap_ReusesLoad_RejectsLayeredRuntime. Its fixture
	// (name: static_layer + static_layer: {}) is exactly the arm phase 77
	// legalizes, so the old test would have died at t.Fatal.
	yaml := sampleValidBootstrap + `
layered_runtime:
  layers:
    - name: static_layer
      static_layer: {}
`
	if err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false); err != nil {
		t.Fatalf("Bootstrap: got %v, want nil for a static_layer bootstrap", err)
	}
}

func TestBootstrap_ReusesLoad_RejectsRtdsLayer(t *testing.T) {
	// The public-package sibling of the roster. rtds_layer is chosen because it
	// is the arm whose silent acceptance would be WORST — a config asking for
	// DYNAMIC runtime served STATIC values.
	// ⚠️ This asserts the "bootstrap: " prefix, which NEITHER pre-existing
	// validate/ reject test did (R9).
	yaml := sampleValidBootstrap + `
layered_runtime:
  layers:
    - name: L1
      rtds_layer: {name: rtds, rtds_config: {resource_api_version: V3}}
`
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for rtds_layer, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: ") {
		t.Errorf("error prefix: got %q, want to start with %q", err.Error(), "bootstrap: ")
	}
	if !strings.Contains(err.Error(), "rtds_layer") {
		t.Errorf("error should name rtds_layer: %q", err.Error())
	}
}

func TestBootstrap_ReusesLoad_YAMLSyntaxError(t *testing.T) {
	err := Bootstrap(strings.NewReader("not: valid: yaml: at all: :::"), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want yaml parse error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap: yaml parse:") {
		t.Errorf("error prefix: %q", err.Error())
	}
}

func TestBootstrap_ReusesLoad_EmptyDocument(t *testing.T) {
	err := Bootstrap(strings.NewReader(""), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want empty-document error, got nil")
	}
	if !strings.Contains(err.Error(), "empty document") {
		t.Errorf("error: %q", err.Error())
	}
}

// --- NEW: construction-boundary failures bootstrap.Load ALONE cannot catch ---

func TestBootstrap_BadTLSCertPath_FailsAtClusterConstruction(t *testing.T) {
	// trusted_ca only needs to be present (NewUpstreamConfig presence-checks
	// it before commonTLSContextToConfig ever loads certificate_chain/
	// private_key files) — a placeholder string is sufficient, matching
	// cmd/envoy-go/main_test.go's equivalent CLI test. The missing-cert-file
	// error fires before trusted_ca content is ever parsed, so no real CA is
	// needed here.
	yaml := `
node: { id: test-node, cluster: test-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners: []
  clusters:
    - name: c_tls_upstream
      type: STATIC
      connect_timeout: 1s
      transport_socket:
        name: envoy.transport_sockets.tls
        typed_config:
          "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.UpstreamTlsContext
          common_tls_context:
            validation_context:
              trusted_ca:
                inline_string: "unused-placeholder"
            tls_certificates:
              - certificate_chain:
                  filename: does-not-exist-cert.pem
                private_key:
                  filename: does-not-exist-key.pem
      load_assignment:
        cluster_name: c_tls_upstream
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for a nonexistent TLS cert file, got nil")
	}
	if !strings.Contains(err.Error(), "does-not-exist-cert.pem") {
		t.Errorf("error should name the missing file: %q", err.Error())
	}
}

func TestBootstrap_LuaCompileFailure_FailsAtListenerConstruction(t *testing.T) {
	yaml := `
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
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for invalid Lua syntax, got nil")
	}
	if !strings.Contains(err.Error(), "script load error: ") {
		t.Errorf("error should contain the script-load-error wrap prefix: %q", err.Error())
	}
}

// TestBootstrap_UnknownHTTPFilterTypeURL_FailsAtListenerConstruction pins that
// listener construction rejects an HTTP filter chain entry whose type_url
// does not resolve in httpReg. Per
// reference_sibling_reject_test_needs_real_typeurl, protojson resolves the
// Any's @type against the REAL proto registry before dispatch ever runs, so
// the type_url must name a message that really exists (and is linked into
// this binary) — it just must not be one httpbuiltins.RegisterBuiltins
// registers as an HTTP filter. filter_http.ValidateChainShape (see
// internal/filter/http/chain_shape.go) dispatches on the entry's TypeURL,
// not its Name — confirmed by reading registry.Lookup(typeURL) and its sole
// caller, internal/filter/hcm/config.go's
// ValidateChainShape(entries, httpRegistry, routerFilterName, router.TypeURL)
// call. So this fixture reuses network/tcp_proxy's real, resolvable
// TcpProxy type_url (registered as a NETWORK filter, never an HTTP one)
// under a non-router filter name, with a separate, correctly-shaped router
// entry last in the chain — isolating the unknown-type_url dispatch failure
// from the unrelated "last entry must be router" chain-shape rule.
func TestBootstrap_UnknownHTTPFilterTypeURL_FailsAtListenerConstruction(t *testing.T) {
	yaml := `
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
                  - name: envoy.filters.http.totally_unregistered_filter
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                      stat_prefix: ingress_tcp
                      cluster: c_echo
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
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap: want error for an unregistered filter type_url, got nil")
	}
	if !strings.Contains(err.Error(), "unknown type_url") {
		t.Errorf("error should name the unknown type_url failure: %q", err.Error())
	}
}

// --- BootstrapFile ---

func TestBootstrapFile_ValidConfig_ReturnsNil(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bootstrap.yaml")
	if err := os.WriteFile(path, []byte(sampleValidBootstrap), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := BootstrapFile(path); err != nil {
		t.Fatalf("BootstrapFile: got %v, want nil", err)
	}
}

func TestBootstrapFile_MissingFile_ReturnsError(t *testing.T) {
	if err := BootstrapFile(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Fatal("BootstrapFile: want error for a missing file, got nil")
	}
}

// --- Tracing-enabled config must not leak the exporter's background goroutine ---

// tracingEnabledBootstrap carries an HCM `tracing` block (Zipkin provider),
// exercising boot.Construct's tracing.ExporterProvider.Get path — which
// spawns a background `go e.run()` goroutine (internal/tracing/zipkin.go)
// for the exporter. c_zipkin_collector never needs to be reachable: building
// the ZipkinExporter only requires the cluster to exist in the manager
// (tracing.ZipkinTransport.HasCluster), not a live dial.
const tracingEnabledBootstrap = `
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
                tracing:
                  provider:
                    name: envoy.tracers.zipkin
                    typed_config:
                      "@type": type.googleapis.com/envoy.config.trace.v3.ZipkinConfig
                      collector_cluster: c_zipkin_collector
                      collector_endpoint: /api/v2/spans
                      collector_endpoint_version: HTTP_JSON
                      trace_id_128bit: true
                      shared_span_context: false
                  random_sampling:
                    value: 100
                route_config:
                  name: rc
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_echo }
                http_filters:
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
    - name: c_zipkin_collector
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_zipkin_collector
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`

// TestBootstrap_TracingEnabledConfig_DoesNotLeakGoroutine pins the fix for
// the finding that validate.Bootstrap left the tracing exporter's background
// goroutine (and, for OTLP, its gRPC ClientConn) running after returning —
// contradicting the package doc comment's promise that no background loop
// is started. Bootstrap must CloseAll() the tracing.ExporterProvider it
// builds before returning, regardless of whether Construct succeeds.
func TestBootstrap_TracingEnabledConfig_DoesNotLeakGoroutine(t *testing.T) {
	// Baseline: let any leftover goroutines from prior subtests/GC settle.
	baseline := settledGoroutineCount(t)

	if err := Bootstrap(strings.NewReader(tracingEnabledBootstrap), t.TempDir(), false); err != nil {
		t.Fatalf("Bootstrap: got %v, want nil", err)
	}

	after := settledGoroutineCount(t)
	if after > baseline {
		t.Errorf("goroutine count after Bootstrap: got %d, want <= baseline %d (exporter goroutine leaked)", after, baseline)
	}
}

// settledGoroutineCount samples runtime.NumGoroutine() repeatedly over a
// short window, returning the minimum observed count once it stabilizes.
// Goroutine counts are inherently noisy (GC workers, timers, test runner
// bookkeeping), so a bare single-sample equality check would flake; this
// polls a few times and settles on the lowest reading, giving any
// short-lived goroutines time to exit.
func settledGoroutineCount(t *testing.T) int {
	t.Helper()
	runtime.GC()
	min := runtime.NumGoroutine()
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		runtime.GC()
		if n := runtime.NumGoroutine(); n < min {
			min = n
		}
	}
	return min
}
