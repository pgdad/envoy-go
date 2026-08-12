package validate

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
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

// --- Phase 86 (ADR-0308): SDS no-fetch sentinel — end-to-end arms ---------
//
// These exercise validate.Bootstrap against the eleven-shape probe battery
// (PLAN §6): three positive "arm" shapes (cert-only SDS, SDS-bound
// validation_context with a static server cert, SDS-bound
// combined_validation_context), the D-86-N5 dead-SDS-endpoint accept, and
// five negative "n" shapes whose reject must come from the boot SDS
// pre-scan (internal/boot.newSDSProviderAndClient / NewValidateSDSProvider)
// rather than from the tls apply-point's nil-provider guard. n3
// (DELTA_GRPC) is a REGRESSION PIN, not a red: its reject fires inside
// xds.ParseSDSConfig, which commonTLSContextToConfig (internal/tls/config.go)
// calls directly BEFORE ever consulting the provider — so it already
// carried the boot-parity string pre-fix (Task 0 census, confirmed by
// direct execution).

// genServerCertPEM builds a self-signed ServerAuth leaf for cn, mirroring
// internal/boot/boot_sds_e2e_test.go's helper of the same name (that helper
// is package-private to internal/boot and not importable here).
func genServerCertPEM(t *testing.T, cn string) (certPEM, keyPEM []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		DNSNames:     []string{cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("x509.MarshalPKCS8PrivateKey: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}

// deadPort returns a TCP port on 127.0.0.1 guaranteed to have nothing
// listening: a listener is opened and immediately closed. validate mode
// never dials SDS (D-86-N5), so a config whose sds_cluster endpoint names
// this port must still validate cleanly.
func deadPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		t.Fatalf("Listener.Close: %v", err)
	}
	return port
}

// sdsClusterYAML renders the shared upstream `sds_cluster` block. withH2
// controls whether typed_extension_protocol_options{http2_protocol_options}
// is present (n7 omits it to exercise grpcclient.Dialer.DialContext's UseH2
// PARSE-REJECT). port is the cluster's own listed endpoint (n5 supplies a
// dead, closed port — never dialed by validate).
func sdsClusterYAML(withH2 bool, port int) string {
	h2 := ""
	if withH2 {
		h2 = `
      typed_extension_protocol_options:
        envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
          "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
          explicit_http_config:
            http2_protocol_options: {}`
	}
	return fmt.Sprintf(`    - name: sds_cluster
      type: STATIC
      lb_policy: ROUND_ROBIN%s
      load_assignment:
        cluster_name: sds_cluster
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: %d }
`, h2, port)
}

// cBackendYAML renders the shared downstream tcp_proxy target cluster.
const cBackendYAML = `    - name: c_backend
      type: STATIC
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_backend
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`

// sdsCertListenerYAML renders a single listener whose downstream TLS
// certificate is entirely SDS-delivered (arm A's shape): the tls apply-point
// consults the provider (nil pre-fix / the sentinel post-fix), never a
// static tls_certificates entry. secretName/sdsRefCluster/apiType are varied
// by the n1/n3/n4/n5/n6/n7 negative arms below.
func sdsCertListenerYAML(name, secretName, sdsRefCluster, apiType string) string {
	return fmt.Sprintf(`    - name: %s
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                tls_certificate_sds_secret_configs:
                  - name: %s
                    sds_config:
                      resource_api_version: V3
                      api_config_source:
                        api_type: %s
                        transport_api_version: V3
                        grpc_services:
                          - envoy_grpc:
                              cluster_name: %s
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_%s
                cluster: c_backend
`, name, secretName, apiType, sdsRefCluster, name)
}

// sdsSingleArmYAML renders the shared single-listener, SDS-cert-only shape
// used by arm A and the n1/n3/n4/n5/n6/n7 arms (PLAN §6's recipe): withNode
// toggles the node: block (n1), secretName varies (n6), sdsRefCluster is the
// cluster_name the SDS grpc_service points AT (n4: "missing_cluster", never
// defined among static_resources.clusters), apiType varies (n3: DELTA_GRPC),
// sdsClusterH2 toggles the upstream sds_cluster's http2_protocol_options
// (n7: false), sdsClusterPort is sds_cluster's own listed endpoint port (n5:
// a dead, closed port).
func sdsSingleArmYAML(withNode bool, secretName, sdsRefCluster, apiType string, sdsClusterH2 bool, sdsClusterPort int) string {
	node := ""
	if withNode {
		node = "node: { id: envoygo-node, cluster: envoygo-cluster }\n"
	}
	return node + `admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
` + sdsCertListenerYAML("l_sds_tls", secretName, sdsRefCluster, apiType) + `
  clusters:
` + cBackendYAML + sdsClusterYAML(sdsClusterH2, sdsClusterPort)
}

// n2YAML renders n2's cross-context cap shape: TWO listeners, each with its
// own single SDS-bound downstream certificate. newSDSProviderAndClient's
// seen>1 guard (boot.go:192-194) only trips ACROSS filter chains/listeners —
// each individual context's own xds.ParseSDSConfig call (len(configs)!=1)
// never sees more than one entry, so this is a distinct arm from the
// within-one-context "n2-aside" shape retained only as a census datapoint.
func n2YAML() string {
	return `
node: { id: envoygo-node, cluster: envoygo-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
` + sdsCertListenerYAML("l_sds_tls_1", "server_cert_1", "sds_cluster", "GRPC") +
		sdsCertListenerYAML("l_sds_tls_2", "server_cert_2", "sds_cluster", "GRPC") + `
  clusters:
` + cBackendYAML + sdsClusterYAML(true, 0)
}

// armBYAML renders arm B: a static (inline) downstream server certificate
// PLUS an SDS-bound validation_context (mTLS), inline_string-quoted via %q
// so PEM newlines are `\n`-escaped C-style rather than depending on
// hand-counted YAML block-scalar indentation.
func armBYAML(t *testing.T) string {
	certPEM, keyPEM := genServerCertPEM(t, "armb.envoy-go.test")
	return fmt.Sprintf(`
node: { id: envoygo-node, cluster: envoygo-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_sds_mtls
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              require_client_certificate: true
              common_tls_context:
                tls_certificates:
                  - certificate_chain:
                      inline_string: %s
                    private_key:
                      inline_string: %s
                validation_context_sds_secret_config:
                  name: validation_ca
                  sds_config:
                    resource_api_version: V3
                    api_config_source:
                      api_type: GRPC
                      transport_api_version: V3
                      grpc_services:
                        - envoy_grpc:
                            cluster_name: sds_cluster
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_sds_mtls
                cluster: c_backend
  clusters:
`, fmt.Sprintf("%q", string(certPEM)), fmt.Sprintf("%q", string(keyPEM))) + cBackendYAML + sdsClusterYAML(true, 0)
}

// armCYAML renders arm C: an inline downstream server certificate PLUS an
// SDS-bound combined_validation_context (default_validation_context.trusted_ca
// inline + validation_context_sds_secret_config).
func armCYAML(t *testing.T) string {
	leafCert, leafKey := genServerCertPEM(t, "armc-leaf.envoy-go.test")
	caCert, _ := genServerCertPEM(t, "armc-ca.envoy-go.test") // self-signed; only the cert half is used, as trusted_ca
	return fmt.Sprintf(`
node: { id: envoygo-node, cluster: envoygo-cluster }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_sds_cvc
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      filter_chains:
        - transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              require_client_certificate: true
              common_tls_context:
                tls_certificates:
                  - certificate_chain:
                      inline_string: %s
                    private_key:
                      inline_string: %s
                combined_validation_context:
                  default_validation_context:
                    trusted_ca:
                      inline_string: %s
                  validation_context_sds_secret_config:
                    name: validation_ca
                    sds_config:
                      resource_api_version: V3
                      api_config_source:
                        api_type: GRPC
                        transport_api_version: V3
                        grpc_services:
                          - envoy_grpc:
                              cluster_name: sds_cluster
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_sds_cvc
                cluster: c_backend
  clusters:
`, fmt.Sprintf("%q", string(leafCert)), fmt.Sprintf("%q", string(leafKey)), fmt.Sprintf("%q", string(caCert))) + cBackendYAML + sdsClusterYAML(true, 0)
}

func TestBootstrap_SDSArmA_AcceptedWithSentinelProvider(t *testing.T) {
	yaml := sdsSingleArmYAML(true, "server_cert", "sds_cluster", "GRPC", true, 0)
	if err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false); err != nil {
		t.Fatalf("Bootstrap(arm A): got %v, want nil", err)
	}
}

func TestBootstrap_SDSArmB_AcceptedWithSentinelProvider(t *testing.T) {
	if err := Bootstrap(strings.NewReader(armBYAML(t)), t.TempDir(), false); err != nil {
		t.Fatalf("Bootstrap(arm B): got %v, want nil", err)
	}
}

func TestBootstrap_SDSArmC_AcceptedWithSentinelProvider(t *testing.T) {
	if err := Bootstrap(strings.NewReader(armCYAML(t)), t.TempDir(), false); err != nil {
		t.Fatalf("Bootstrap(arm C): got %v, want nil", err)
	}
}

// TestBootstrap_SDSDeadEndpoint_AcceptedWithoutDialing pins D-86-N5: a
// structurally valid SDS-bound config whose sds_cluster endpoint is dead
// (nothing listening) must still validate cleanly, because --mode validate
// never dials or fetches SDS — the reject-classification a genuinely
// unreachable SDS server would trigger at boot/runtime is out of scope for
// validate (PLAN §6 / SPEC Q2's fetch-time exemption).
func TestBootstrap_SDSDeadEndpoint_AcceptedWithoutDialing(t *testing.T) {
	yaml := sdsSingleArmYAML(true, "server_cert", "sds_cluster", "GRPC", true, deadPort(t))
	if err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false); err != nil {
		t.Fatalf("Bootstrap(dead SDS endpoint, n5): got %v, want nil (validate never dials)", err)
	}
}

// --- Negative arms: message-transition (pre-fix: masked by the arm-A
// nil-provider string; post-fix: the boot pre-scan's own byte-identical
// boot-parity string) plus n3's regression pin. ---

func TestBootstrap_SDSMissingNode_RejectsWithBootParityString(t *testing.T) {
	yaml := sdsSingleArmYAML(false, "server_cert", "sds_cluster", "GRPC", true, 0)
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap(n1 missing node): want error, got nil")
	}
	if !strings.Contains(err.Error(), "xds: sds: node.id and node.cluster are required for SDS") {
		t.Errorf("Bootstrap(n1 missing node): got %q, want it to contain the boot node-requirement string", err.Error())
	}
}

func TestBootstrap_SDSCrossContextCap_RejectsWithBootParityString(t *testing.T) {
	err := Bootstrap(strings.NewReader(n2YAML()), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap(n2 cross-context cap): want error, got nil")
	}
	if !strings.Contains(err.Error(), "xds: sds: multiple SDS-bound downstream TLS contexts unsupported (MVP takes one)") {
		t.Errorf("Bootstrap(n2 cross-context cap): got %q, want it to contain the boot one-secret-cap string", err.Error())
	}
}

// TestBootstrap_SDSDeltaGRPC_RegressionPin is n3 (PLAN §6): api_type
// DELTA_GRPC. UNLIKE n1/n2/n4/n6/n7 this is NOT a message-transition arm:
// xds.ParseSDSConfig runs directly inside commonTLSContextToConfig
// (internal/tls/config.go), BEFORE that function ever consults the
// provider, so this reject already carried the boot-parity string pre-fix
// (Task 0 census, confirmed by direct execution — census.log's "=== n3
// (rc=1) ===" line already reads the DELTA_GRPC string, not the masking
// arm-A string). Pinned here as a regression guard, not claimed as a red.
func TestBootstrap_SDSDeltaGRPC_RegressionPin(t *testing.T) {
	yaml := sdsSingleArmYAML(true, "server_cert", "sds_cluster", "DELTA_GRPC", true, 0)
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap(n3 DELTA_GRPC): want error, got nil")
	}
	if !strings.Contains(err.Error(), "xds: sds: DELTA_GRPC api_type unsupported (only GRPC / State-of-the-World)") {
		t.Errorf("Bootstrap(n3 DELTA_GRPC): got %q, want it to contain the ParseSDSConfig DELTA_GRPC string", err.Error())
	}
}

func TestBootstrap_SDSUnknownCluster_RejectsWithBootParityString(t *testing.T) {
	yaml := sdsSingleArmYAML(true, "server_cert", "missing_cluster", "GRPC", true, 0)
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap(n4 unknown cluster): want error, got nil")
	}
	if !strings.Contains(err.Error(), `xds: sds: dial cluster "missing_cluster"`) {
		t.Errorf("Bootstrap(n4 unknown cluster): got %q, want it to contain the verbatim `dial cluster` prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "unknown cluster") {
		t.Errorf("Bootstrap(n4 unknown cluster): got %q, want it to name the unknown-cluster failure", err.Error())
	}
}

func TestBootstrap_SDSInvalidSecretName_RejectsWithBootParityString(t *testing.T) {
	yaml := sdsSingleArmYAML(true, "bad/name", "sds_cluster", "GRPC", true, 0)
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap(n6 invalid secret name): want error, got nil")
	}
	if !strings.Contains(err.Error(), `xds: sds: invalid secret name: "bad/name"`) {
		t.Errorf("Bootstrap(n6 invalid secret name): got %q, want it to contain the boot secret-name-charset string", err.Error())
	}
}

func TestBootstrap_SDSMissingHTTP2Options_RejectsWithBootParityString(t *testing.T) {
	yaml := sdsSingleArmYAML(true, "server_cert", "sds_cluster", "GRPC", false, 0)
	err := Bootstrap(strings.NewReader(yaml), t.TempDir(), false)
	if err == nil {
		t.Fatal("Bootstrap(n7 missing http2_protocol_options): want error, got nil")
	}
	if !strings.Contains(err.Error(), `xds: sds: dial cluster "sds_cluster"`) {
		t.Errorf("Bootstrap(n7 missing http2_protocol_options): got %q, want it to contain the verbatim `dial cluster` prefix", err.Error())
	}
	if !strings.Contains(err.Error(), "does not have http2_protocol_options{} set") {
		t.Errorf("Bootstrap(n7 missing http2_protocol_options): got %q, want it to name the missing-h2 failure", err.Error())
	}
}
