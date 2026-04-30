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
	if got, want := bs.Proto.GetNode().GetId(), "test-node"; got != want {
		t.Errorf("node.id: got %q, want %q", got, want)
	}
	if got := bs.Proto.GetStaticResources(); got == nil {
		t.Fatal("static_resources missing")
	}
	if n := len(bs.Proto.GetStaticResources().GetListeners()); n != 1 {
		t.Errorf("listeners: got %d, want 1", n)
	}
	if n := len(bs.Proto.GetStaticResources().GetClusters()); n != 1 {
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
	host, port, err := AdminSocket(bs.Proto)
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
	_, _, err = AdminSocket(bs.Proto)
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
	clusters := bs.Proto.GetStaticResources().GetClusters()
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
	if _, err := protojson.Marshal(bs.Proto); err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
}

// TestLoad_AllocatesStatsRegistry asserts that Load returns a Bootstrap
// wrapper carrying an allocated, non-Frozen *stats.Registry. Per the settled
// SPEC §12 #2 decision the Registry lives as a field on the Bootstrap wrapper
// (not as a free-standing alloc in main.go), so future xDS phases that add a
// dynamic config-reload path have a place to thread the Registry through.
// The Registry MUST NOT be Frozen yet — downstream cluster/listener/HCM
// constructors (Tasks 8–11) register metrics on it before Task 12's
// post-construction Freeze call.
func TestLoad_AllocatesStatsRegistry(t *testing.T) {
	bs, err := Load(strings.NewReader(sampleBootstrap))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if bs.Stats == nil {
		t.Fatal("Bootstrap.Stats is nil; expected an allocated *stats.Registry")
	}
	// The Registry MUST NOT be Frozen yet (downstream constructors register).
	// (Hyphens are not legal in SN-validated names — use dotted form.)
	c := bs.Stats.NewCounter("test.field_not_frozen")
	if c == nil {
		t.Fatal("NewCounter on Bootstrap.Stats returned nil")
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
	if got := len(bs.Proto.GetStaticResources().GetListeners()); got != 1 {
		t.Fatalf("listeners: got %d, want 1", got)
	}
	if _, err := protojson.Marshal(bs.Proto); err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
}

// hcmWithAccessLog builds a complete bootstrap YAML string containing a single
// HCM listener with the given accessLogBlock embedded as the access_log[]
// section. If accessLogBlock is empty, the access_log field is omitted.
func hcmWithAccessLog(accessLogBlock string) string {
	accessLogField := ""
	if accessLogBlock != "" {
		accessLogField = "\n                access_log:\n" + accessLogBlock
	}
	return `
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
                stat_prefix: ingress_http` + accessLogField + `
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
}

// TestBootstrap_AccessLog_FileType_PathRequired verifies that a file-type
// access_log entry with a valid path is parsed, returned in AccessLogConfigs,
// and the Path field is set correctly.
func TestBootstrap_AccessLog_FileType_PathRequired(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: /tmp/envoy-access.log`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.AccessLogConfigs); got != 1 {
		t.Fatalf("AccessLogConfigs: got %d, want 1", got)
	}
	if got, want := bs.AccessLogConfigs[0].Path, "/tmp/envoy-access.log"; got != want {
		t.Errorf("AccessLogConfigs[0].Path: got %q, want %q", got, want)
	}
}

// TestBootstrap_AccessLog_RejectLogFormat verifies that a file-type
// access_log entry with log_format set is rejected with the correct error.
func TestBootstrap_AccessLog_RejectLogFormat(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: /tmp/envoy-access.log
                      log_format:
                        text_format_source:
                          inline_string: "[%START_TIME%] %REQ(:METHOD)%\n"`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for log_format, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported config: access_log[].log_format") {
		t.Errorf("error should contain 'unsupported config: access_log[].log_format': %q", err.Error())
	}
}

// TestBootstrap_AccessLog_RejectJSONFormat verifies that a file-type
// access_log entry with json_format set is rejected with the correct error.
func TestBootstrap_AccessLog_RejectJSONFormat(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: /tmp/envoy-access.log
                      json_format:
                        timestamp: "%START_TIME%"`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for json_format, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported config: access_log[].json_format") {
		t.Errorf("error should contain 'unsupported config: access_log[].json_format': %q", err.Error())
	}
}

// TestBootstrap_AccessLog_RejectFormatString verifies that a file-type
// access_log entry with the deprecated format (format_string) set is rejected
// with the correct error. The deprecated proto field is the oneof
// FileAccessLog_Format (proto field name "format").
func TestBootstrap_AccessLog_RejectFormatString(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: /tmp/envoy-access.log
                      format: "[%START_TIME%] %REQ(:METHOD)% %REQ(X-FORWARDED-FOR)%\n"`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for format_string, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported config: access_log[].format_string") {
		t.Errorf("error should contain 'unsupported config: access_log[].format_string': %q", err.Error())
	}
}

// TestBootstrap_AccessLog_PathEmptyRejects verifies that a file-type
// access_log entry with an empty path is rejected.
func TestBootstrap_AccessLog_PathEmptyRejects(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: ""`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for empty path, got nil")
	}
	if !strings.Contains(err.Error(), "path is required") {
		t.Errorf("error should contain 'path is required': %q", err.Error())
	}
}

// TestBootstrap_AccessLog_StdoutSilentlyIgnored verifies that a stdout-type
// access_log entry is silently ignored — no error and no AccessLogConfigs entry.
func TestBootstrap_AccessLog_StdoutSilentlyIgnored(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.stdout
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.stream.v3.StdoutAccessLog`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.AccessLogConfigs); got != 0 {
		t.Errorf("AccessLogConfigs: got %d, want 0 (stdout should be silently ignored)", got)
	}
}

// TestBootstrap_AccessLog_NoEntriesIsValid verifies that an HCM with no
// access_log entries (missing field) parses successfully with an empty
// AccessLogConfigs slice.
func TestBootstrap_AccessLog_NoEntriesIsValid(t *testing.T) {
	bs, err := Load(strings.NewReader(hcmWithAccessLog("")))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.AccessLogConfigs); got != 0 {
		t.Errorf("AccessLogConfigs: got %d, want 0", got)
	}
}

// TestBootstrap_AccessLog_TwoFileEntries verifies that two file-type entries
// are both parsed and returned in registration order.
func TestBootstrap_AccessLog_TwoFileEntries(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: /tmp/envoy-access-1.log
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: /tmp/envoy-access-2.log`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.AccessLogConfigs); got != 2 {
		t.Fatalf("AccessLogConfigs: got %d, want 2", got)
	}
	if got, want := bs.AccessLogConfigs[0].Path, "/tmp/envoy-access-1.log"; got != want {
		t.Errorf("AccessLogConfigs[0].Path: got %q, want %q", got, want)
	}
	if got, want := bs.AccessLogConfigs[1].Path, "/tmp/envoy-access-2.log"; got != want {
		t.Errorf("AccessLogConfigs[1].Path: got %q, want %q", got, want)
	}
}
