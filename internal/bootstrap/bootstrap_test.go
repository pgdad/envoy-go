package bootstrap

import (
	"reflect"
	"strings"
	"testing"
	"time"

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

// grpcALSType is the typed_config @type for the gRPC HTTP access logger.
const grpcALSType = "type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig"

// TestBootstrap_ALS_AcceptMinimal verifies a minimal HttpGrpcAccessLogConfig
// (log_name + envoy_grpc.cluster_name) parses into exactly one ALSConfig with
// the cluster name and log name carried through.
func TestBootstrap_ALS_AcceptMinimal(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0], (ALSConfig{ClusterName: "als_cluster", LogName: "mylog", BufferSizeBytes: 16384, BufferFlushInterval: time.Second}); !reflect.DeepEqual(got, want) {
		t.Errorf("ALSConfigs[0]: got %+v, want %+v", got, want)
	}
}

// TestBootstrap_ALS_AcceptEmptyLogName verifies an omitted log_name is valid
// and yields ALSConfig.LogName == "".
func TestBootstrap_ALS_AcceptEmptyLogName(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0], (ALSConfig{ClusterName: "als_cluster", LogName: "", BufferSizeBytes: 16384, BufferFlushInterval: time.Second}); !reflect.DeepEqual(got, want) {
		t.Errorf("ALSConfigs[0]: got %+v, want %+v", got, want)
	}
}

// TestBootstrap_ALS_AcceptBufferFields verifies buffer_size_bytes /
// buffer_flush_interval are CONSUMED at 44.2 (AMEND-BUF-2): an explicit
// 16384 / 1s pair is carried verbatim into the ALSConfig.
func TestBootstrap_ALS_AcceptBufferFields(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster
                        buffer_size_bytes: 16384
                        buffer_flush_interval: 1s`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0].BufferSizeBytes, uint32(16384); got != want {
		t.Errorf("BufferSizeBytes: got %d, want %d", got, want)
	}
	if got, want := bs.ALSConfigs[0].BufferFlushInterval, 1*time.Second; got != want {
		t.Errorf("BufferFlushInterval: got %v, want %v", got, want)
	}
}

// TestBootstrap_ALS_AcceptInertHeaders verifies
// additional_request_headers_to_log parses cleanly (the names are now
// CONSUMED at 44.3; this minimal-shape test only asserts the single-entry
// accept path — the lowercasing/order semantics are covered below).
func TestBootstrap_ALS_AcceptInertHeaders(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster
                      additional_request_headers_to_log: [x-foo]`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
}

// TestBootstrap_ALS_HeadersLowercasedBothPresent verifies both header-name
// lists are lifted into ALSConfig and lowercased at parse (AMEND-HDR-1, 44.3).
func TestBootstrap_ALS_HeadersLowercasedBothPresent(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster
                      additional_request_headers_to_log: ["X-Req-Foo", "x-req-multi"]
                      additional_response_headers_to_log: ["Content-Type"]`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0].AdditionalRequestHeaders, []string{"x-req-foo", "x-req-multi"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AdditionalRequestHeaders: got %v, want %v", got, want)
	}
	if got, want := bs.ALSConfigs[0].AdditionalResponseHeaders, []string{"content-type"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AdditionalResponseHeaders: got %v, want %v", got, want)
	}
}

// TestBootstrap_ALS_HeadersAbsentBoth verifies the no-capture path: with
// neither header list configured both slices stay nil/empty.
func TestBootstrap_ALS_HeadersAbsentBoth(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got := bs.ALSConfigs[0].AdditionalRequestHeaders; len(got) != 0 {
		t.Errorf("AdditionalRequestHeaders: got %v, want empty", got)
	}
	if got := bs.ALSConfigs[0].AdditionalResponseHeaders; len(got) != 0 {
		t.Errorf("AdditionalResponseHeaders: got %v, want empty", got)
	}
}

// TestBootstrap_ALS_HeadersMixedCaseDuplicatePreservedOrder verifies the
// names are lowercased, duplicates are NOT de-duped at parse, and order is
// preserved (the Filter dedups the union, not parse — AMEND-HDR-1, 44.3).
func TestBootstrap_ALS_HeadersMixedCaseDuplicatePreservedOrder(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster
                      additional_request_headers_to_log: ["X-A", "x-a", "X-B"]`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0].AdditionalRequestHeaders, []string{"x-a", "x-a", "x-b"}; !reflect.DeepEqual(got, want) {
		t.Errorf("AdditionalRequestHeaders: got %v, want %v", got, want)
	}
}

// TestBootstrap_ALS_HeadersWithStrictRejectGoogleGrpc verifies the strict
// reject arms run BEFORE the header reads: a config carrying a header list AND
// google_grpc STILL errors on google_grpc (44.3 adds NO new reject — ADR-0080).
func TestBootstrap_ALS_HeadersWithStrictRejectGoogleGrpc(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          google_grpc:
                            target_uri: 127.0.0.1:50051
                            stat_prefix: als
                      additional_request_headers_to_log: ["X-Req-Foo"]`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for google_grpc, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap:") {
		t.Errorf("error should be bootstrap:-prefixed: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "google_grpc") {
		t.Errorf("error should name google_grpc: %q", err.Error())
	}
}

// TestBootstrap_ALS_AcceptTransportV3 verifies an explicit transport_api_version
// of V3 is accepted.
func TestBootstrap_ALS_AcceptTransportV3(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        transport_api_version: V3
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
}

// TestBootstrap_ALS_AcceptTransportAUTO verifies an omitted transport_api_version
// (AUTO == 0) is accepted.
func TestBootstrap_ALS_AcceptTransportAUTO(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
}

// TestBootstrap_ALS_RejectGoogleGrpc verifies that a google_grpc grpc_service
// is rejected (only envoy_grpc is supported), with a bootstrap:-prefixed error
// naming google_grpc.
func TestBootstrap_ALS_RejectGoogleGrpc(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          google_grpc:
                            target_uri: 127.0.0.1:50051
                            stat_prefix: als`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for google_grpc, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap:") {
		t.Errorf("error should be bootstrap:-prefixed: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "google_grpc") {
		t.Errorf("error should name google_grpc: %q", err.Error())
	}
}

// TestBootstrap_ALS_RejectTransportV2 verifies transport_api_version V2 is
// rejected (envoy-go is V3-only).
func TestBootstrap_ALS_RejectTransportV2(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        transport_api_version: V2
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for transport_api_version V2, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap:") {
		t.Errorf("error should be bootstrap:-prefixed: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "V2") {
		t.Errorf("error should name V2: %q", err.Error())
	}
}

// TestBootstrap_ALS_RejectEmptyCluster verifies an empty envoy_grpc.cluster_name
// is rejected.
func TestBootstrap_ALS_RejectEmptyCluster(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: ""`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for empty cluster_name, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap:") {
		t.Errorf("error should be bootstrap:-prefixed: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "cluster_name") {
		t.Errorf("error should name cluster_name: %q", err.Error())
	}
}

// tcpGrpcALSType is the @type for the TCP gRPC access logger, STRICT-REJECTED at
// boot per ADR-0080 (envoy-go supports HTTP gRPC ALS only).
const tcpGrpcALSType = "type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.TcpGrpcAccessLogConfig"

// TestBootstrap_ALS_RejectTcpGrpc verifies that a TcpGrpcAccessLogConfig is
// STRICT-REJECTED at boot (ADR-0080) rather than silently ignored. The
// common_config is otherwise valid so the ONLY reason to reject is the
// unsupported TCP ALS type.
func TestBootstrap_ALS_RejectTcpGrpc(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.tcp_grpc
                    typed_config:
                      "@type": ` + tcpGrpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for TcpGrpcAccessLogConfig, got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap:") {
		t.Errorf("error should be bootstrap:-prefixed: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "TCP") {
		t.Errorf("error should name TCP ALS as unsupported: %q", err.Error())
	}
}

// TestBootstrap_ALS_CoexistWithFile verifies a file access_log and a gRPC
// access_log in the SAME HCM populate the two parallel slices independently
// (AccessLogConfigs for the file sink, ALSConfigs for the gRPC sink).
func TestBootstrap_ALS_CoexistWithFile(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: /tmp/envoy-access.log
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.AccessLogConfigs); got != 1 {
		t.Errorf("AccessLogConfigs: got %d, want 1", got)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Errorf("ALSConfigs: got %d, want 1", got)
	}
}

// otlpALSType is the @type for the OpenTelemetry (OTLP) access logger. Lifted
// from the ADR-0041 silent-ignore set at phase 45.1 (ADR-0258).
const otlpALSType = "type.googleapis.com/envoy.extensions.access_loggers.open_telemetry.v3.OpenTelemetryAccessLogConfig"

// TestBootstrap_OTLP table-drives the OpenTelemetryAccessLogConfig parse arm
// (phase 45.1, ADR-0258): the accept cases assert exactly one OTLPConfig with
// the expected field values; the reject cases assert a bootstrap:-prefixed error
// naming the offending field plus the "OTLP access log" sink label and the
// access_log[%d] index. The shared parseCommonGrpcAccessLogConfig helper carries
// the V2/google_grpc/empty-cluster strict rejects (the existing
// TestBootstrap_ALS_Reject* tests are the byte-stability green-guard for the
// "grpc ALS" wording the same helper produces with sinkLabel="grpc ALS").
func TestBootstrap_OTLP(t *testing.T) {
	minimalCommon := `
                      common_config:
                        log_name: otel
                        grpc_service:
                          envoy_grpc:
                            cluster_name: otlp_cluster`
	tests := []struct {
		name    string
		block   string
		wantErr bool
		errSubs []string
		wantCfg *OTLPConfig
	}{
		{
			name: "accept-minimal",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + minimalCommon,
			wantCfg: &OTLPConfig{ClusterName: "otlp_cluster", LogName: "otel", BufferSizeBytes: 16384, BufferFlushInterval: time.Second, DisableBuiltinLabels: false},
		},
		{
			name: "accept-empty-log_name",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      common_config:
                        grpc_service:
                          envoy_grpc:
                            cluster_name: otlp_cluster`,
			wantCfg: &OTLPConfig{ClusterName: "otlp_cluster", LogName: "", BufferSizeBytes: 16384, BufferFlushInterval: time.Second},
		},
		{
			name: "accept-disable_builtin_labels",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      disable_builtin_labels: true` + minimalCommon,
			wantCfg: &OTLPConfig{ClusterName: "otlp_cluster", LogName: "otel", BufferSizeBytes: 16384, BufferFlushInterval: time.Second, DisableBuiltinLabels: true},
		},
		{
			name: "accept-buffer-fields",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      common_config:
                        log_name: otel
                        grpc_service:
                          envoy_grpc:
                            cluster_name: otlp_cluster
                        buffer_size_bytes: 8192
                        buffer_flush_interval: 2s`,
			wantCfg: &OTLPConfig{ClusterName: "otlp_cluster", LogName: "otel", BufferSizeBytes: 8192, BufferFlushInterval: 2 * time.Second},
		},
		{
			name: "accept-buffer-zero",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      common_config:
                        log_name: otel
                        grpc_service:
                          envoy_grpc:
                            cluster_name: otlp_cluster
                        buffer_size_bytes: 0`,
			wantCfg: &OTLPConfig{ClusterName: "otlp_cluster", LogName: "otel", BufferSizeBytes: 0, BufferFlushInterval: time.Second},
		},
		{
			name:    "accept-flush-default",
			block:   "\n                  - name: envoy.access_loggers.open_telemetry\n                    typed_config:\n                      \"@type\": " + otlpALSType + minimalCommon,
			wantCfg: &OTLPConfig{ClusterName: "otlp_cluster", LogName: "otel", BufferSizeBytes: 16384, BufferFlushInterval: time.Second},
		},
		{
			name: "accept-stat_prefix-inert",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      stat_prefix: myprefix` + minimalCommon,
			wantCfg: &OTLPConfig{ClusterName: "otlp_cluster", LogName: "otel", BufferSizeBytes: 16384, BufferFlushInterval: time.Second},
		},
		{
			name: "accept-transport-V3",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      common_config:
                        log_name: otel
                        transport_api_version: V3
                        grpc_service:
                          envoy_grpc:
                            cluster_name: otlp_cluster`,
			wantCfg: &OTLPConfig{ClusterName: "otlp_cluster", LogName: "otel", BufferSizeBytes: 16384, BufferFlushInterval: time.Second},
		},
		{
			name: "accept-transport-AUTO",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + minimalCommon,
			wantCfg: &OTLPConfig{ClusterName: "otlp_cluster", LogName: "otel", BufferSizeBytes: 16384, BufferFlushInterval: time.Second},
		},
		{
			name: "reject-google_grpc",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      common_config:
                        log_name: otel
                        grpc_service:
                          google_grpc:
                            target_uri: 127.0.0.1:50051
                            stat_prefix: otlp`,
			wantErr: true,
			errSubs: []string{"google_grpc", "OTLP access log", "access_log[0]"},
		},
		{
			name: "reject-transport-V2",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      common_config:
                        log_name: otel
                        transport_api_version: V2
                        grpc_service:
                          envoy_grpc:
                            cluster_name: otlp_cluster`,
			wantErr: true,
			errSubs: []string{"OTLP access log", "V2"},
		},
		{
			name: "reject-empty-cluster",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      common_config:
                        log_name: otel
                        grpc_service:
                          envoy_grpc:
                            cluster_name: ""`,
			wantErr: true,
			errSubs: []string{"OTLP access log", "cluster_name"},
		},
		{
			name: "reject-body",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      body:
                        string_value: "custom"` + minimalCommon,
			wantErr: true,
			errSubs: []string{"OTLP access log", "body"},
		},
		{
			name: "reject-attributes",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      attributes:
                        values:
                          - key: "k"
                            value: { string_value: "v" }` + minimalCommon,
			wantErr: true,
			errSubs: []string{"OTLP access log", "attributes"},
		},
		{
			name: "reject-resource_attributes",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      resource_attributes:
                        values:
                          - key: "k"
                            value: { string_value: "v" }` + minimalCommon,
			wantErr: true,
			errSubs: []string{"OTLP access log", "resource_attributes"},
		},
		{
			name: "reject-formatters",
			block: `
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      formatters:
                        - name: some.formatter` + minimalCommon,
			wantErr: true,
			errSubs: []string{"OTLP access log", "formatters"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bs, err := Load(strings.NewReader(hcmWithAccessLog(tt.block)))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("Load: want error, got nil")
				}
				if !strings.HasPrefix(err.Error(), "bootstrap:") {
					t.Errorf("error should be bootstrap:-prefixed: %q", err.Error())
				}
				for _, sub := range tt.errSubs {
					if !strings.Contains(err.Error(), sub) {
						t.Errorf("error should contain %q: %q", sub, err.Error())
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := len(bs.OTLPConfigs); got != 1 {
				t.Fatalf("OTLPConfigs: got %d, want 1", got)
			}
			if tt.wantCfg != nil {
				if got, want := bs.OTLPConfigs[0], *tt.wantCfg; !reflect.DeepEqual(got, want) {
					t.Errorf("OTLPConfigs[0]: got %+v, want %+v", got, want)
				}
			}
		})
	}
}

// TestBootstrap_OTLP_CoexistWithFileAndGrpc verifies a file + a gRPC + an OTLP
// access_log in the SAME HCM populate the three parallel slices independently.
func TestBootstrap_OTLP_CoexistWithFileAndGrpc(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.file
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog
                      path: /tmp/envoy-access.log
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster
                  - name: envoy.access_loggers.open_telemetry
                    typed_config:
                      "@type": ` + otlpALSType + `
                      common_config:
                        log_name: otel
                        grpc_service:
                          envoy_grpc:
                            cluster_name: otlp_cluster`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.AccessLogConfigs); got != 1 {
		t.Errorf("AccessLogConfigs: got %d, want 1", got)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Errorf("ALSConfigs: got %d, want 1", got)
	}
	if got := len(bs.OTLPConfigs); got != 1 {
		t.Errorf("OTLPConfigs: got %d, want 1", got)
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

// TestBootstrap_RoundTrips_CorsPerRouteConfig exercises the phase-07.1
// (Task 20) blank-import of `envoy/extensions/filters/http/cors/v3`: a
// minimal HCM bootstrap with a virtual_host carrying
// `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{...}` must
// load cleanly via Load (no protojson "type not registered" error) AND
// survive a protojson.Marshal round-trip. Without the cors/v3 blank import,
// protojson would reject the CorsPolicy Any at unmarshal time (ADR-0016).
//
// The shape mirrors the per-route configuration form used by the phase-07.1
// 0007a-cors differential fixture (Task 21) but is kept minimal here.
func TestBootstrap_RoundTrips_CorsPerRouteConfig(t *testing.T) {
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
                      typed_per_filter_config:
                        envoy.filters.http.cors:
                          "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.CorsPolicy
                          allow_origin_string_match:
                            - exact: "https://example.com"
                          allow_methods: "GET, POST"
                          allow_headers: "x-custom-header"
                          allow_credentials: true
                      routes:
                        - match: { path: "/health" }
                          direct_response:
                            status: 200
                            body: { inline_string: "OK\n" }
                http_filters:
                  - name: envoy.filters.http.cors
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.cors.v3.Cors
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
	// Round-trip via protojson — would error if the CorsPolicy proto descriptor
	// (or the Cors filter-level descriptor) were unregistered.
	if _, err := protojson.Marshal(bs.Proto); err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
}

// TestBootstrap_RoundTrips_TLSInspectorListenerFilter exercises the
// phase-07.2 (Task 11) blank-import of
// `envoy/extensions/filters/listener/tls_inspector/v3`: a minimal bootstrap
// carrying `listener_filters: [{name: envoy.filters.listener.tls_inspector,
// typed_config: TlsInspector}]` must load cleanly via Load (no protojson
// "type not registered" error) AND survive a protojson.Marshal round-trip.
//
// Without the tls_inspector v3 blank import, protojson would reject the
// TlsInspector Any at unmarshal time (ADR-0016). The shape is the listener
// filter declaration the unified pre-handshake dispatch path (ADR-0079)
// requires for SNI extraction, mirroring fixture-0002 / fixture-0008.
func TestBootstrap_RoundTrips_TLSInspectorListenerFilter(t *testing.T) {
	yamlSrc := `
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: 0 }
static_resources:
  listeners:
    - name: l_tls
      address:
        socket_address: { address: 127.0.0.1, port_value: 0 }
      listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
      filter_chains:
        - filter_chain_match:
            server_names: ["alpha.example.test"]
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: ingress_tls
                cluster: c_alpha
  clusters:
    - name: c_alpha
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c_alpha
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: 127.0.0.1, port_value: 0 }
`
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	listeners := bs.Proto.GetStaticResources().GetListeners()
	if got := len(listeners); got != 1 {
		t.Fatalf("listeners: got %d, want 1", got)
	}
	lfs := listeners[0].GetListenerFilters()
	if got := len(lfs); got != 1 {
		t.Fatalf("listener_filters: got %d, want 1", got)
	}
	if got, want := lfs[0].GetName(), "envoy.filters.listener.tls_inspector"; got != want {
		t.Errorf("listener_filters[0].name: got %q, want %q", got, want)
	}
	tc := lfs[0].GetTypedConfig()
	if tc == nil {
		t.Fatal("listener_filters[0].typed_config: nil")
	}
	const want = "type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector"
	if got := tc.GetTypeUrl(); got != want {
		t.Errorf("listener_filters[0].typed_config.@type: got %q, want %q", got, want)
	}
	// Round-trip via protojson — would error if the TlsInspector proto
	// descriptor were unregistered.
	if _, err := protojson.Marshal(bs.Proto); err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
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

// TestBootstrap_ALS_Buffer_DefaultBoth verifies that when buffer_size_bytes and
// buffer_flush_interval are both absent (no wrapper, no duration), the
// AMEND-BUF-2 defaults are applied: BufferSizeBytes == 16384 and
// BufferFlushInterval == 1s.
func TestBootstrap_ALS_Buffer_DefaultBoth(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0].BufferSizeBytes, uint32(16384); got != want {
		t.Errorf("BufferSizeBytes: got %d, want %d (AMEND-BUF-2 default)", got, want)
	}
	if got, want := bs.ALSConfigs[0].BufferFlushInterval, 1*time.Second; got != want {
		t.Errorf("BufferFlushInterval: got %v, want %v (AMEND-BUF-2 default)", got, want)
	}
}

// TestBootstrap_ALS_Buffer_ExplicitSize verifies that an explicit
// buffer_size_bytes value is honored verbatim (not replaced by the default).
func TestBootstrap_ALS_Buffer_ExplicitSize(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster
                        buffer_size_bytes: 4096`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0].BufferSizeBytes, uint32(4096); got != want {
		t.Errorf("BufferSizeBytes: got %d, want %d", got, want)
	}
}

// TestBootstrap_ALS_Buffer_ExplicitZeroSize verifies that buffer_size_bytes: 0
// (UInt32Value wrapper PRESENT with value 0) is honored as flush-every-entry —
// NOT coerced to the 16384 default. An explicit present-but-zero wrapper is
// semantically distinct from an absent wrapper (nil pointer) per AMEND-BUF-2.
func TestBootstrap_ALS_Buffer_ExplicitZeroSize(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster
                        buffer_size_bytes: 0`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0].BufferSizeBytes, uint32(0); got != want {
		t.Errorf("BufferSizeBytes: got %d, want %d (explicit 0 must be honored, not coerced to default)", got, want)
	}
}

// TestBootstrap_ALS_Buffer_ExplicitIntervalSubsecond verifies a positive
// sub-second buffer_flush_interval is honored verbatim (200ms).
func TestBootstrap_ALS_Buffer_ExplicitIntervalSubsecond(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster
                        buffer_flush_interval: 0.2s`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0].BufferFlushInterval, 200*time.Millisecond; got != want {
		t.Errorf("BufferFlushInterval: got %v, want %v", got, want)
	}
}

// TestBootstrap_ALS_Buffer_IntervalZeroCoerced verifies that
// buffer_flush_interval: 0s (Duration present but zero) is coerced to the 1s
// default — a time.NewTicker(0) panic-guard (§3.2 AMEND-BUF-2).
func TestBootstrap_ALS_Buffer_IntervalZeroCoerced(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster
                        buffer_flush_interval: 0s`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0].BufferFlushInterval, 1*time.Second; got != want {
		t.Errorf("BufferFlushInterval: got %v, want %v (0s must be coerced to 1s panic-guard)", got, want)
	}
}

// TestBootstrap_ALS_Buffer_IntervalAbsentCoerced verifies that an absent
// buffer_flush_interval (nil *durationpb.Duration → AsDuration() == 0 → coerce)
// produces the 1s default.
func TestBootstrap_ALS_Buffer_IntervalAbsentCoerced(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          envoy_grpc:
                            cluster_name: als_cluster`)
	bs, err := Load(strings.NewReader(yamlSrc))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(bs.ALSConfigs); got != 1 {
		t.Fatalf("ALSConfigs: got %d, want 1", got)
	}
	if got, want := bs.ALSConfigs[0].BufferFlushInterval, 1*time.Second; got != want {
		t.Errorf("BufferFlushInterval: got %v, want %v (absent interval must be coerced to 1s)", got, want)
	}
}

// TestBootstrap_ALS_Buffer_WithStrictReject verifies that a config with
// buffer_size_bytes set AND google_grpc still produces an error for google_grpc.
// The reject arms run BEFORE the buffer reads; the buffer fields add no new
// accept-path that bypasses a reject.
func TestBootstrap_ALS_Buffer_WithStrictReject(t *testing.T) {
	yamlSrc := hcmWithAccessLog(`
                  - name: envoy.access_loggers.http_grpc
                    typed_config:
                      "@type": ` + grpcALSType + `
                      common_config:
                        log_name: mylog
                        grpc_service:
                          google_grpc:
                            target_uri: 127.0.0.1:50051
                            stat_prefix: als
                        buffer_size_bytes: 4096`)
	_, err := Load(strings.NewReader(yamlSrc))
	if err == nil {
		t.Fatal("Load: want error for google_grpc (even with buffer_size_bytes set), got nil")
	}
	if !strings.HasPrefix(err.Error(), "bootstrap:") {
		t.Errorf("error should be bootstrap:-prefixed: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "google_grpc") {
		t.Errorf("error should name google_grpc: %q", err.Error())
	}
}

func TestBootstrap_ConfigPathFieldExistsAndDefaultsEmpty(t *testing.T) {
	const minimalBootstrap = `admin:
  address:
    socket_address: {address: 127.0.0.1, port_value: 9901}
static_resources:
  listeners: []
  clusters: []
`
	bs, err := Load(strings.NewReader(minimalBootstrap))
	if err == nil {
		// minimal bootstrap should error (zero clusters), but if it loads,
		// assert ConfigPath is empty
		if bs.ConfigPath != "" {
			t.Errorf("ConfigPath after Load: got %q, want \"\"", bs.ConfigPath)
		}
	}
	// Construct directly and assert the field is settable.
	b := &Bootstrap{ConfigPath: "/tmp/envoy.yaml"}
	if b.ConfigPath != "/tmp/envoy.yaml" {
		t.Errorf("ConfigPath after struct-literal set: got %q, want %q", b.ConfigPath, "/tmp/envoy.yaml")
	}
}
