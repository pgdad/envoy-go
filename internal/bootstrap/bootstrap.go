package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	fileaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	grpcalv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"
	otlpalv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"

	// Blank-imported so the filter extension's proto descriptor is registered
	// with protoregistry.GlobalTypes, which lets protojson round-trip the
	// typed_config Any without envoy-go interpreting its contents (ADR-0016).
	// Phase 01 fixtures only use tcp_proxy; later phases register additional
	// filters as fixtures introduce them.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	// Phase 04 (HTTP/1.1) registers the HCM network filter, the router HTTP
	// filter, and the route-config proto so protojson round-trips fixtures
	// 0003-* and any future HTTP fixtures without interpreting typed_config.
	// Per ADR-0016 the addition is a registry-population mechanism, not a
	// new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"

	// Phase 05.2 (Task 10) registers the cluster-side HttpProtocolOptions
	// extension proto so protojson round-trips fixture-0004's bootstraps
	// (which carry typed_extension_protocol_options on the cluster) without
	// interpreting typed_config. Per ADR-0016 amendment policy this addition
	// is documented in PROGRESS, not as a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/upstreams/http/v3"
	// Phase 06.2 (Task 7) registers the FileAccessLog extension proto and the
	// StdoutAccessLog (stream) extension proto so protojson round-trips
	// bootstraps carrying HCM access_log[] entries of those types without
	// "type not registered" errors (ADR-0067). Per ADR-0016 amendment policy
	// these additions are documented in PROGRESS, not as a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/stream/v3"

	// Phase 44.1 registers the gRPC-ALS access-logger extension proto so protojson
	// round-trips bootstraps carrying HCM access_log[] HttpGrpcAccessLogConfig
	// entries (ADR-0255). Registered transitively by the grpcalv3 typed import
	// above; the explicit blank-import here mirrors the file/v3 pattern and makes
	// the dependency obvious to bootstrap-side readers. Per ADR-0016 amendment
	// policy this addition is documented in PROGRESS, not as a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/grpc/v3"

	// Phase 45.1 registers the OTLP access-logger extension proto so protojson
	// round-trips bootstraps carrying HCM access_log[] OpenTelemetryAccessLogConfig
	// entries (ADR-0258). Its body/attributes field types transitively pull
	// go.opentelemetry.io/proto/otlp common/v1.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/open_telemetry/v3"

	// Phase 07.1 (Task 20) registers the cors HTTP filter extension proto so
	// protojson round-trips bootstraps that carry
	// `typed_per_filter_config[envoy.filters.http.cors] = CorsPolicy{...}`
	// entries on virtual_hosts / routes (the per-route CORS config form used
	// by the 07.1 differential fixture 0007a-cors). The filter-level Cors
	// message in http_filters[] is registered transitively by the cors filter
	// package itself (cors.go imports `cors/v3`); the explicit blank-import
	// here makes the dependency obvious to bootstrap-side readers and
	// guarantees registration even if the filter package is ever vendored
	// or excluded from a future build. Per ADR-0016 amendment policy this
	// addition is documented in PROGRESS, not as a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/cors/v3"

	// Phase 07.2 (Task 11) registers the tls_inspector listener-filter
	// extension proto so protojson round-trips bootstraps carrying
	// `listener_filters: [{name: envoy.filters.listener.tls_inspector,
	// typed_config: {"@type": ...TlsInspector}}]` (ADR-0079). Without this
	// blank-import protojson errors with "type not registered" at boot
	// because the bootstrap parser walks listener_filters[].typed_config
	// generically. Task 10's accept-loop refactor removed the implicit
	// crypto/tls.GetConfigForClient SNI extraction, so subject bootstraps
	// for SNI-indexed filter chains MUST declare tls_inspector explicitly
	// (mirroring what reference Envoy already required). Per ADR-0016
	// amendment policy this addition is documented in PROGRESS, not as a
	// new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"

	// Phase-26.1 registers the echo + direct_response network-filter extension
	// protos so protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of those types (the 26.1 network
	// read-filter chain). Registered transitively by the echo/directresponse
	// filter packages too; the explicit blank-imports here guarantee resolution
	// in any bootstrap-parsing context. Per ADR-0016 amendment policy, documented
	// in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/direct_response/v3"
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/echo/v3"

	// Phase-27 registers the sni_cluster network-filter extension proto so
	// protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of that type (the 27 sni_cluster
	// read filter). Registered transitively by the snicluster filter package
	// too; the explicit blank-import here guarantees resolution in any
	// bootstrap-parsing context (e.g. the differential harness, which parses
	// the rendered YAML before the binary's filter registry is fully wired).
	// Per ADR-0016 amendment policy, documented in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/sni_cluster/v3"
	// Phase-28.1 registers the zookeeper_proxy network-filter extension proto so
	// protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of that type. Registered
	// transitively by the zookeeperproxy filter package too; the explicit
	// blank-import here guarantees resolution in any bootstrap-parsing context
	// (e.g. the differential harness). Per ADR-0016 amendment policy, documented
	// in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/zookeeper_proxy/v3"
	// Phase-29.1 registers the mongo_proxy network-filter extension proto so
	// protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of that type. Registered
	// transitively by the mongoproxy filter package too; the explicit
	// blank-import here guarantees resolution in any bootstrap-parsing context
	// (e.g. the differential harness). Per ADR-0016 amendment policy, documented
	// in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/mongo_proxy/v3"
	// Phase-31 registers the kafka_broker network-filter extension proto (the
	// project's FIRST /contrib import) so protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of that type. Registered transitively
	// by the kafkabroker filter package too; the explicit blank-import here
	// guarantees resolution in any bootstrap-parsing context (e.g. the differential
	// harness) AND holds the new contrib module dep through `go mod tidy`. Per
	// ADR-0016 amendment policy, documented in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/contrib/envoy/extensions/filters/network/kafka_broker/v3"
	// Phase-32.1 registers the redis_proxy network-filter extension proto so
	// protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of that type. Registered
	// transitively by the redisproxy filter package too; the explicit
	// blank-import here guarantees resolution in any bootstrap-parsing context
	// (e.g. the differential harness). ZERO new go.mod dep (AMEND-R1). Per
	// ADR-0016 amendment policy, documented in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/redis_proxy/v3"
	// Phase-33 registers the thrift_proxy network-filter extension proto so
	// protojson round-trips bootstraps carrying
	// filter_chains[].filters[].typed_config of that type. Registered transitively
	// by the thriftproxy filter package too; the explicit blank-import here
	// guarantees resolution in any bootstrap-parsing context (e.g. the differential
	// harness). The thrift_proxy/v3 subpackage carries both thrift_proxy.pb.go and
	// route.pb.go (one import registers the route-config descriptors too). ZERO new
	// go.mod dep (AMEND-T1 — CORE /envoy v1.32.4). Per ADR-0016 amendment policy,
	// documented in PROGRESS, not a new ADR.
	_ "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
	// Phase 46.1a registers the OpenTelemetry tracer config proto so protojson
	// round-trips bootstraps carrying an HCM tracing.provider OpenTelemetryConfig
	// typed_config (ADR-0260; lifts HttpConnectionManager.tracing from the ADR-0041
	// silent-ignore set).
	_ "github.com/envoyproxy/go-control-plane/envoy/config/trace/v3"

	// Phase 47.1 registers the metrics config proto so protojson round-trips
	// bootstraps carrying a stats_sinks[] metrics_service typed_config (and the
	// sibling StatsdSink, which is STRICT-REJECTED) without "type not registered"
	// errors (ADR-0262; lifts Bootstrap.stats_sinks from the ADR-0064 silent-ignore
	// set). The named metricsconfigv3 import below both registers the descriptor
	// and provides the typed MetricsServiceConfig for the parse arm.
	metricsconfigv3 "github.com/envoyproxy/go-control-plane/envoy/config/metrics/v3"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"gopkg.in/yaml.v3"

	"github.com/esalaine/envoy-go/internal/accesslog"
	"github.com/esalaine/envoy-go/internal/stats"
)

const (
	// hcmTypeURL is the TypeURL for HttpConnectionManager, used when walking
	// listener filter chains to find HCM filters during access_log[] parsing.
	hcmTypeURL = "type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager"

	// fileAccessLogTypeURL is the TypeURL for the file access logger
	// (envoy.access_loggers.file). Used in access_log[] typed_config
	// type-switching per ADR-0067.
	fileAccessLogTypeURL = "type.googleapis.com/envoy.extensions.access_loggers.file.v3.FileAccessLog"

	// httpGrpcAccessLogTypeURL is the TypeURL for the gRPC HTTP access logger
	// (envoy.access_loggers.http_grpc). Lifted from the ADR-0041 silent-ignore
	// set at phase 44.1 (ADR-0255).
	httpGrpcAccessLogTypeURL = "type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.HttpGrpcAccessLogConfig"

	// tcpGrpcAccessLogTypeURL is the TypeURL for the TCP gRPC access logger
	// (envoy.access_loggers.tcp_grpc). STRICT-REJECTED at boot (ADR-0080): TCP ALS
	// is unsupported; silently ignoring it would drop a configured user's access
	// logs with no signal (the ADR-0080 anti-silent-divergence rule).
	tcpGrpcAccessLogTypeURL = "type.googleapis.com/envoy.extensions.access_loggers.grpc.v3.TcpGrpcAccessLogConfig"

	// otlpAccessLogTypeURL is the TypeURL for the OpenTelemetry (OTLP) access logger.
	// Lifted from the ADR-0041 silent-ignore set at phase 45.1 (ADR-0258).
	otlpAccessLogTypeURL = "type.googleapis.com/envoy.extensions.access_loggers.open_telemetry.v3.OpenTelemetryAccessLogConfig"

	// alsDefaultBufferSizeBytes is the buffer_size_bytes default applied when the
	// CommonGrpcAccessLogConfig wrapper is absent — the empirically-pinned
	// reference default (AMEND-BUF-2). An explicit present-but-0 value is honored
	// as flush-every-entry and is NOT coerced to this default.
	alsDefaultBufferSizeBytes uint32 = 16384

	// statsFlushIntervalDefault is the stats_flush_interval default (5s) applied
	// when the field is absent or non-positive (ADR-0262 / D-MS-FLUSH).
	statsFlushIntervalDefault = 5 * time.Second
)

// metricsServiceTypeURL is the typed_config TypeURL for the metrics_service stats
// sink (envoy.config.metrics.v3.MetricsServiceConfig). It is DERIVED from the
// proto descriptor rather than hard-coded so a proto-package rename is caught at
// build time, not silently (the verify-typeurl-via-descriptor discipline —
// reference_network_filter_typeurl_extensions). A test asserts it equals the
// SPEC §11 live-verified string.
var metricsServiceTypeURL = "type.googleapis.com/" + string((&metricsconfigv3.MetricsServiceConfig{}).ProtoReflect().Descriptor().FullName())

// AccessLogConfig is the parsed-but-not-yet-opened representation of one
// envoy.access_loggers.file entry from HCM access_log[]. The sink itself is
// constructed in cmd/envoy-go/main.go after Load returns; this struct carries
// only the parse-time data.
type AccessLogConfig struct {
	Path string
}

// ALSConfig is the parsed gRPC Access Log Service sink config from one HCM
// access_log[] HttpGrpcAccessLogConfig entry (ADR-0255). The sink is built in
// cmd/envoy-go/main.go after Load returns. buffer_size_bytes and
// buffer_flush_interval are CONSUMED (AMEND-BUF-2, phase 44.2);
// additional_request_headers_to_log / additional_response_headers_to_log are
// now CONSUMED as the header-capture name lists (phase 44.3, AMEND-HDR-1).
// additional_*_trailers_to_log and AccessLog.filter STAY inert (deferred).
type ALSConfig struct {
	ClusterName               string        // common_config.grpc_service.envoy_grpc.cluster_name
	LogName                   string        // common_config.log_name (empty is valid)
	BufferSizeBytes           uint32        // common_config.buffer_size_bytes (default 16384 when wrapper absent; explicit 0 ⇒ flush-every-entry) — AMEND-BUF-2
	BufferFlushInterval       time.Duration // common_config.buffer_flush_interval (default 1s when absent/zero — a NewTicker(<=0) panic-guard) — AMEND-BUF-2
	AdditionalRequestHeaders  []string      // additional_request_headers_to_log (field 2), lowercased at parse — AMEND-HDR-1
	AdditionalResponseHeaders []string      // additional_response_headers_to_log (field 3), lowercased at parse — AMEND-HDR-1
}

// OTLPConfig is the parsed OpenTelemetry (OTLP) access-log sink config from one
// HCM access_log[] OpenTelemetryAccessLogConfig entry (ADR-0258). body/attributes
// are COMPILED at boot (operator templates, AMEND-OPS-2/3); resource_attributes
// are type-validated and stored LITERAL (AMEND-OPS-1); formatters stay
// STRICT-REJECTED; stat_prefix is PARSE-ACCEPT-but-INERT (the fixed stat names
// regardless).
type OTLPConfig struct {
	ClusterName          string        // common_config.grpc_service.envoy_grpc.cluster_name
	LogName              string        // common_config.log_name (empty is valid)
	BufferSizeBytes      uint32        // common_config.buffer_size_bytes (default 16384 when wrapper absent; explicit 0 ⇒ flush-every-entry)
	BufferFlushInterval  time.Duration // common_config.buffer_flush_interval (default 1s when absent/zero)
	DisableBuiltinLabels bool          // disable_builtin_labels (drops all 4 Resource labels wholesale)
	// Body is the compiled body AnyValue template (operator-substituted per record →
	// LogRecord.body); nil when no body is configured (the 45.1 built-in path).
	Body *accesslog.OTLPValueTemplate
	// Attributes are the compiled attributes templates (ordered key→template,
	// operator-substituted per record → LogRecord.attributes); empty when none.
	Attributes []accesslog.OTLPAttrTemplate
	// ResourceAttributes are the LITERAL resource_attributes (validated string/kvlist/
	// array; request operators pass through verbatim — AMEND-OPS-1), emitted as extra
	// Resource.attributes after the 4 built-in labels (surviving disable_builtin_labels
	// — AMEND-OPS-5); nil when none.
	ResourceAttributes []*commonpb.KeyValue
}

// StatsSinkConfig is the parsed metrics_service stats-sink config from one
// top-level stats_sinks[] MetricsServiceConfig entry (ADR-0262). The sink itself
// (the MetricsServiceSink + Flusher) is constructed in cmd/envoy-go/main.go after
// Load returns; this struct carries only the parse-time data. The strict-reject
// arms (report_counters_as_deltas / emit_tags_as_labels / non-default
// histogram_emit_mode / non-V3 transport / google_grpc / empty cluster) run in
// parseMetricsServiceConfig before the config is appended (ADR-0080).
type StatsSinkConfig struct {
	ClusterName string // MetricsServiceConfig.grpc_service.envoy_grpc.cluster_name
}

// Bootstrap wraps the parsed Envoy v3 Bootstrap proto together with the
// boot-time *stats.Registry that downstream constructors (cluster/listener/HCM
// managers in Tasks 8–11) register their metrics on. Per the settled SPEC
// §12 #2 decision the Registry lives as a field on this wrapper rather than
// being allocated free-standing in main.go, so future xDS phases that add a
// dynamic config-reload path have a place to thread the Registry through a
// config-update path.
type Bootstrap struct {
	// Proto is the unmarshalled Envoy v3 Bootstrap message.
	Proto *bootstrapv3.Bootstrap
	// Stats is the boot-time metrics Registry. It is allocated by Load and
	// MUST NOT be Frozen at that point — downstream constructors register
	// counters/gauges on it during boot. Task 12 owns the post-construction
	// Freeze call per SPEC §5.4.
	Stats *stats.Registry
	// AccessLogConfigs is the parsed access_log[] file-sink entries from each
	// HCM filter, in registration order across all listeners and HCM filters.
	// Empty when no file-type access_log entries are configured. Per ADR-0067,
	// log_format/format_string/json_format on file-type entries is rejected at
	// parse time. Other typed_config types (stdout, tcp_grpc, open_telemetry)
	// are silently ignored per the ADR-0041 amendment.
	AccessLogConfigs []AccessLogConfig
	// ALSConfigs is the parsed access_log[] gRPC Access Log Service sink entries
	// from each HCM filter, in registration order across all listeners and HCM
	// filters (parallel to AccessLogConfigs, which carries file-sink entries).
	// Empty when no HttpGrpcAccessLogConfig access_log entries are configured.
	// Per ADR-0255/AMEND-BUF-2: transport_api_version V2 and google_grpc are
	// rejected at parse time; envoy_grpc.cluster_name (non-empty), log_name,
	// buffer_size_bytes (default 16384), and buffer_flush_interval (default 1s)
	// are all CONSUMED (phase 44.2); additional_request_headers_to_log /
	// additional_response_headers_to_log are now CONSUMED as the header-capture
	// name lists (phase 44.3, AMEND-HDR-1); additional_*_trailers_to_log and
	// AccessLog.filter stay inert (deferred).
	ALSConfigs []ALSConfig
	// OTLPConfigs is the parsed access_log[] OpenTelemetry (OTLP) access-log sink
	// entries from each HCM filter, in registration order across all listeners and
	// HCM filters (parallel to AccessLogConfigs/ALSConfigs). Empty when no
	// OpenTelemetryAccessLogConfig access_log entries are configured. Per ADR-0258:
	// transport_api_version V2, google_grpc, and an empty envoy_grpc.cluster_name
	// are rejected at parse time (via the shared CommonGrpcAccessLogConfig helper);
	// body/attributes are COMPILED into operator templates and resource_attributes are
	// type-validated + stored literal at phase 45.2 (ADR-0259); formatters stay
	// STRICT-REJECTED; cluster_name, log_name, buffer_size_bytes
	// (default 16384), buffer_flush_interval (default 1s), and disable_builtin_labels
	// are CONSUMED; stat_prefix is read-and-ignored (INERT).
	OTLPConfigs []OTLPConfig
	// StatsSinkConfigs is the parsed top-level stats_sinks[] metrics_service sink
	// entries (ADR-0262), in declaration order. Empty when no metrics_service
	// stats_sinks[] entry is configured. Per ADR-0262/ADR-0080:
	// report_counters_as_deltas:true, emit_tags_as_labels:true, a non-default
	// histogram_emit_mode, a non-V3 transport_api_version, google_grpc, an empty
	// cluster_name, sibling sink TypeURLs, and stats_flush_on_admin are all
	// rejected at parse time. The MetricsServiceSink/Flusher are built in
	// cmd/envoy-go/main.go after Load returns (and ONLY when this slice is
	// non-empty — byte-stability, D-MS-FLUSH-INERT).
	StatsSinkConfigs []StatsSinkConfig
	// FlushInterval is the stats_flush_interval (default 5s when absent/non-positive
	// — D-MS-FLUSH). Parsed-and-stored even when no stats_sinks[] entry exists
	// (inert in that case — D-MS-FLUSH-INERT); the Flusher uses it only when
	// StatsSinkConfigs is non-empty.
	FlushInterval time.Duration
	// ConfigPath is the file path the bootstrap was loaded from. Set by the
	// caller (cmd/envoy-go/main.go) post-Load via bs.ConfigPath = *cfgPath;
	// Load itself leaves this empty (the bootstrap.Load API takes an io.Reader,
	// not a file path, by ADR-0001 design). Phase 08.1's /server_info admin
	// handler reads this for the command_line_options.config_path field.
	// Test code that does not exercise /server_info may leave this field empty.
	ConfigPath string
}

// Load parses r as YAML (upstream Envoy's YAML shape), converts to JSON, and
// unmarshals into an Envoy v3 Bootstrap proto. Unknown fields at any depth
// cause an error (ADR-0016). The phase-01 unsupported surfaces
// dynamic_resources and layered_runtime cause an error even though the proto
// itself defines them. The returned *Bootstrap also carries a freshly
// allocated, non-Frozen *stats.Registry on its `Stats` field (SPEC §12 #2).
//
// Every error returned by Load begins with "bootstrap: ".
func Load(r io.Reader) (*Bootstrap, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: read: %w", err)
	}
	var generic map[string]interface{}
	if err := yaml.Unmarshal(raw, &generic); err != nil {
		return nil, fmt.Errorf("bootstrap: yaml parse: %w", err)
	}
	if generic == nil {
		return nil, fmt.Errorf("bootstrap: empty document")
	}
	if _, ok := generic["dynamic_resources"]; ok {
		return nil, fmt.Errorf("bootstrap: dynamic_resources not supported in phase 01 (see SPEC §2)")
	}
	if _, ok := generic["layered_runtime"]; ok {
		return nil, fmt.Errorf("bootstrap: layered_runtime not supported in phase 01 (see SPEC §2)")
	}
	jsonBytes, err := json.Marshal(generic)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: to json: %w", err)
	}
	bs := &bootstrapv3.Bootstrap{}
	opts := protojson.UnmarshalOptions{DiscardUnknown: false}
	if err := opts.Unmarshal(jsonBytes, bs); err != nil {
		return nil, fmt.Errorf("bootstrap: protojson: %w", err)
	}
	result := &Bootstrap{Proto: bs, Stats: stats.NewRegistry()}
	if err := parseAccessLogConfigs(bs, result); err != nil {
		return nil, err
	}
	if err := parseStatsSinks(bs, result); err != nil {
		return nil, err
	}
	return result, nil
}

// parseStatsSinks reads the top-level stats_flush_interval (default 5s) and the
// stats_sinks[] entries (ADR-0262). stats_flush_interval is ALWAYS stored on
// result.FlushInterval, even with no stats_sinks[] (inert — D-MS-FLUSH-INERT).
// stats_flush_on_admin is STRICT-REJECTED (ADR-0080; envoy-go ships only the
// periodic stats_flush_interval loop). Each stats_sinks[] entry must carry a
// metrics_service typed_config; a sibling sink TypeURL is rejected naming both
// the offending and the supported TypeURL (reference_strict_reject_sibling_typeurl_gap).
func parseStatsSinks(bs *bootstrapv3.Bootstrap, result *Bootstrap) error {
	result.FlushInterval = statsFlushIntervalDefault
	if d := bs.GetStatsFlushInterval(); d != nil {
		if v := d.AsDuration(); v > 0 {
			result.FlushInterval = v
		}
	}
	if bs.GetStatsFlushOnAdmin() {
		return fmt.Errorf("bootstrap: stats_flush_on_admin is not supported (envoy-go ships only the periodic stats_flush_interval sink loop)")
	}
	for i, sink := range bs.GetStatsSinks() {
		tc := sink.GetTypedConfig()
		if tc == nil {
			return fmt.Errorf("bootstrap: stats_sinks[%d]: missing typed_config", i)
		}
		if tc.GetTypeUrl() != metricsServiceTypeURL {
			return fmt.Errorf("bootstrap: stats_sinks[%d]: unsupported sink type %q (envoy-go supports only the metrics_service sink %q)", i, tc.GetTypeUrl(), metricsServiceTypeURL)
		}
		if err := parseMetricsServiceConfig(tc, i, result); err != nil {
			return err
		}
	}
	return nil
}

// parseMetricsServiceConfig parses one metrics_service stats sink typed_config
// and appends a StatsSinkConfig to result.StatsSinkConfigs (ADR-0262). It
// STRICT-REJECTS (ADR-0080): a non-V3 transport_api_version (V2 reference-parity;
// AUTO/V3 accepted — D-MS-APIVERSION), google_grpc (only envoy_grpc supported),
// an empty envoy_grpc.cluster_name, report_counters_as_deltas:true (47.1 emits
// cumulative absolute values — deferred to 47.2), emit_tags_as_labels:true (47.1
// emits the full dotted name with zero labels — deferred to 47.2), and a
// non-default histogram_emit_mode (envoy-go has no histograms — ADR-0060).
func parseMetricsServiceConfig(tc *anypb.Any, idx int, result *Bootstrap) error {
	var msc metricsconfigv3.MetricsServiceConfig
	if err := tc.UnmarshalTo(&msc); err != nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service typed_config: %w", idx, err)
	}
	if v := msc.GetTransportApiVersion(); v != corev3.ApiVersion_AUTO && v != corev3.ApiVersion_V3 {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service transport_api_version %v is not supported (envoy-go is V3-only)", idx, v)
	}
	eg := msc.GetGrpcService().GetEnvoyGrpc()
	if eg == nil {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service requires grpc_service.envoy_grpc (google_grpc is not supported)", idx)
	}
	if eg.GetClusterName() == "" {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service grpc_service.envoy_grpc.cluster_name is required (must be non-empty)", idx)
	}
	if d := msc.GetReportCountersAsDeltas(); d != nil && d.GetValue() {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service report_counters_as_deltas:true is not supported (47.1 emits cumulative absolute values; deferred to 47.2)", idx)
	}
	if msc.GetEmitTagsAsLabels() {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service emit_tags_as_labels:true is not supported (47.1 emits the full dotted name with zero labels; deferred to 47.2)", idx)
	}
	if m := msc.GetHistogramEmitMode(); m != metricsconfigv3.HistogramEmitMode_SUMMARY_AND_HISTOGRAM {
		return fmt.Errorf("bootstrap: stats_sinks[%d]: metrics_service histogram_emit_mode %v is not supported (envoy-go has no histograms)", idx, m)
	}
	result.StatsSinkConfigs = append(result.StatsSinkConfigs, StatsSinkConfig{ClusterName: eg.GetClusterName()})
	return nil
}

// parseAccessLogConfigs walks the static_resources listeners looking for HCM
// filters, then parses each HCM's access_log[] entries per ADR-0067. File-type
// entries are collected into result.AccessLogConfigs; other typed_config types
// are silently ignored. log_format/format_string/json_format on file-type
// entries produce a fatal error.
func parseAccessLogConfigs(bs *bootstrapv3.Bootstrap, result *Bootstrap) error {
	sr := bs.GetStaticResources()
	if sr == nil {
		return nil
	}
	for _, listener := range sr.GetListeners() {
		for _, fc := range listener.GetFilterChains() {
			for _, f := range fc.GetFilters() {
				tc := f.GetTypedConfig()
				if tc == nil || tc.GetTypeUrl() != hcmTypeURL {
					continue
				}
				hcm := &hcmv3.HttpConnectionManager{}
				if err := proto.Unmarshal(tc.GetValue(), hcm); err != nil {
					return fmt.Errorf("bootstrap: hcm unmarshal: %w", err)
				}
				for i, al := range hcm.GetAccessLog() {
					if err := parseOneAccessLog(al, i, result); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// parseOneAccessLog processes a single AccessLog entry from an HCM filter.
// File-type entries with a valid path are appended to result.AccessLogConfigs.
// File-type entries with log_format/format_string/json_format produce errors.
// Non-file typed_config types are silently ignored per ADR-0041 amendment.
func parseOneAccessLog(al *accesslogv3.AccessLog, idx int, result *Bootstrap) error {
	tc := al.GetTypedConfig()
	if tc == nil {
		// No typed_config — silently ignore.
		return nil
	}
	if tc.GetTypeUrl() == httpGrpcAccessLogTypeURL {
		return parseGrpcAccessLog(tc, idx, result)
	}
	if tc.GetTypeUrl() == tcpGrpcAccessLogTypeURL {
		return fmt.Errorf("bootstrap: access_log[%d]: TCP gRPC ALS (TcpGrpcAccessLogConfig) is not supported (envoy-go supports HTTP gRPC ALS only)", idx)
	}
	if tc.GetTypeUrl() == otlpAccessLogTypeURL {
		return parseOpenTelemetryAccessLog(tc, idx, result)
	}
	if tc.GetTypeUrl() != fileAccessLogTypeURL {
		// Other non-file typed_config (stdout, tcp_grpc, etc.) — silently ignored
		// per ADR-0041 amendment / ADR-0067.
		return nil
	}
	fal := &fileaccesslogv3.FileAccessLog{}
	if err := proto.Unmarshal(tc.GetValue(), fal); err != nil {
		return fmt.Errorf("bootstrap: access_log[%d] unmarshal: %w", idx, err)
	}
	// Reject any custom format fields — ADR-0067 option β.
	switch fal.GetAccessLogFormat().(type) {
	case *fileaccesslogv3.FileAccessLog_LogFormat:
		return fmt.Errorf("bootstrap: unsupported config: access_log[].log_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)")
	case *fileaccesslogv3.FileAccessLog_Format:
		return fmt.Errorf("bootstrap: unsupported config: access_log[].format_string (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)")
	case *fileaccesslogv3.FileAccessLog_JsonFormat:
		return fmt.Errorf("bootstrap: unsupported config: access_log[].json_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)")
	case *fileaccesslogv3.FileAccessLog_TypedJsonFormat:
		return fmt.Errorf("bootstrap: unsupported config: access_log[].json_format (envoy-go ships only the implicit default format in phase 06.2; superseded by a later phase)")
	}
	if fal.GetPath() == "" {
		return fmt.Errorf("bootstrap: access_log[%d]: path is required (must be a non-empty file path)", idx)
	}
	result.AccessLogConfigs = append(result.AccessLogConfigs, AccessLogConfig{Path: fal.GetPath()})
	return nil
}

// parseGrpcAccessLog processes a single HttpGrpcAccessLogConfig access_log entry
// and appends an ALSConfig to result.ALSConfigs (ADR-0255). It STRICT-REJECTS
// transport_api_version V2 (envoy-go is V3-only), google_grpc grpc_service
// (only envoy_grpc is supported), and an empty envoy_grpc.cluster_name.
// buffer_size_bytes and buffer_flush_interval are CONSUMED (AMEND-BUF-2,
// phase 44.2); additional_request_headers_to_log /
// additional_response_headers_to_log are now CONSUMED as the lowercased
// header-capture name lists (phase 44.3, AMEND-HDR-1). The strict-reject arms
// run BEFORE the header reads (ADR-0080; 44.3 adds no new reject).
func parseGrpcAccessLog(tc *anypb.Any, idx int, result *Bootstrap) error {
	cfg := &grpcalv3.HttpGrpcAccessLogConfig{}
	if err := proto.Unmarshal(tc.GetValue(), cfg); err != nil {
		return fmt.Errorf("bootstrap: access_log[%d] grpc unmarshal: %w", idx, err)
	}
	clusterName, logName, bufferSizeBytes, bufferFlushInterval, err := parseCommonGrpcAccessLogConfig(cfg.GetCommonConfig(), idx, "grpc ALS")
	if err != nil {
		return err
	}
	// additional_{request,response}_headers_to_log (fields 2/3, on the OUTER
	// HttpGrpcAccessLogConfig — NOT common_config): the configured names are
	// lowercased once at parse so the capture lookup + the map key are lowercase
	// (AMEND-HDR-1). An absent/empty list ⇒ nil (the no-capture path). Duplicates
	// are NOT de-duped here (idempotent map write; the Filter dedups the union).
	reqHdrs := lowerAll(cfg.GetAdditionalRequestHeadersToLog())
	respHdrs := lowerAll(cfg.GetAdditionalResponseHeadersToLog())
	result.ALSConfigs = append(result.ALSConfigs, ALSConfig{
		ClusterName:               clusterName,
		LogName:                   logName,
		BufferSizeBytes:           bufferSizeBytes,
		BufferFlushInterval:       bufferFlushInterval,
		AdditionalRequestHeaders:  reqHdrs,
		AdditionalResponseHeaders: respHdrs,
	})
	return nil
}

// parseCommonGrpcAccessLogConfig reads the CommonGrpcAccessLogConfig shared by
// HttpGrpcAccessLogConfig (phase 44) and OpenTelemetryAccessLogConfig (phase 45):
// STRICT-REJECTS transport_api_version V2, a google_grpc (non-envoy_grpc)
// grpc_service, and an empty cluster_name (ADR-0080), and applies the
// buffer_size_bytes default (16384 when the wrapper is absent; explicit 0 honored
// as flush-every-entry) + the buffer_flush_interval default (1s when absent/zero —
// a time.NewTicker(<=0) HARD panic-guard, §3.2; any positive explicit value incl.
// sub-second is honored verbatim). sinkLabel is interpolated into the reject
// messages so each arm keeps its own byte-stable wording.
func parseCommonGrpcAccessLogConfig(common *grpcalv3.CommonGrpcAccessLogConfig, idx int, sinkLabel string) (clusterName, logName string, bufBytes uint32, flush time.Duration, err error) {
	if v := common.GetTransportApiVersion(); v == corev3.ApiVersion_V2 { //nolint:staticcheck // SA1019: this arm EXISTS to PARSE-REJECT the deprecated V2 transport; intentional access (ADR-0255, envoy-go is V3-only).
		return "", "", 0, 0, fmt.Errorf("bootstrap: access_log[%d]: %s transport_api_version V2 is not supported (envoy-go is V3-only)", idx, sinkLabel)
	}
	eg := common.GetGrpcService().GetEnvoyGrpc()
	if eg == nil {
		return "", "", 0, 0, fmt.Errorf("bootstrap: access_log[%d]: %s requires grpc_service.envoy_grpc (google_grpc is not supported)", idx, sinkLabel)
	}
	if eg.GetClusterName() == "" {
		return "", "", 0, 0, fmt.Errorf("bootstrap: access_log[%d]: %s grpc_service.envoy_grpc.cluster_name is required (must be non-empty)", idx, sinkLabel)
	}
	// buffer_size_bytes: the WRAPPER pointer is nil when the field is absent ⇒
	// default 16384 (AMEND-BUF-2); an explicit present-but-0 value is honored as
	// flush-every-entry (the size threshold sum >= 0 always fires).
	bufBytes = alsDefaultBufferSizeBytes
	if w := common.GetBufferSizeBytes(); w != nil {
		bufBytes = w.GetValue()
	}
	flush = common.GetBufferFlushInterval().AsDuration()
	if flush <= 0 {
		flush = time.Second
	}
	return eg.GetClusterName(), common.GetLogName(), bufBytes, flush, nil
}

// parseOpenTelemetryAccessLog processes a single OpenTelemetryAccessLogConfig
// access_log entry and appends an OTLPConfig to result.OTLPConfigs (ADR-0258).
// STRICT-REJECTS (ADR-0080): the common_config V2/google_grpc/empty-cluster (via
// the shared helper); a non-empty formatters (out of scope). body + attributes are
// COMPILED into operator templates (unknown-operator / non-string-kvlist-array
// value-type → clean boot error — D-OTLP-2-COMPILE-SITE); resource_attributes are
// type-VALIDATED and stored literally (AMEND-OPS-1). disable_builtin_labels is
// CONSUMED; stat_prefix is read-and-ignored (INERT).
func parseOpenTelemetryAccessLog(tc *anypb.Any, idx int, result *Bootstrap) error {
	cfg := &otlpalv3.OpenTelemetryAccessLogConfig{}
	if err := proto.Unmarshal(tc.GetValue(), cfg); err != nil {
		return fmt.Errorf("bootstrap: access_log[%d] otlp unmarshal: %w", idx, err)
	}
	if len(cfg.GetFormatters()) > 0 {
		return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log formatters (custom formatter extensions) are not supported", idx)
	}
	clusterName, logName, bufBytes, flush, err := parseCommonGrpcAccessLogConfig(cfg.GetCommonConfig(), idx, "OTLP access log")
	if err != nil {
		return err
	}
	// Compile body + attributes (operators substituted per record — AMEND-OPS-2/3/4);
	// the strict-reject (unknown operator / non-string-kvlist-array value-type) is a
	// clean boot error here (D-OTLP-2-COMPILE-SITE).
	bodyTmpl, err := accesslog.CompileOTLPValue(cfg.GetBody())
	if err != nil {
		return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log body: %w", idx, err)
	}
	var attrs []accesslog.OTLPAttrTemplate
	for _, kv := range cfg.GetAttributes().GetValues() {
		vt, err := accesslog.CompileOTLPValue(kv.GetValue())
		if err != nil {
			return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log attributes[%q]: %w", idx, kv.GetKey(), err)
		}
		attrs = append(attrs, accesslog.OTLPAttrTemplate{Key: kv.GetKey(), Value: vt})
	}
	// resource_attributes are LITERAL (no operator substitution — AMEND-OPS-1) but still
	// type-validated (string/kvlist/array only — AMEND-OPS-2).
	resAttrs := cfg.GetResourceAttributes().GetValues()
	for _, kv := range resAttrs {
		if err := accesslog.ValidateOTLPValue(kv.GetValue()); err != nil {
			return fmt.Errorf("bootstrap: access_log[%d]: OTLP access log resource_attributes[%q]: %w", idx, kv.GetKey(), err)
		}
	}
	// stat_prefix (field 6) read-and-ignored (PARSE-ACCEPT-but-INERT).
	result.OTLPConfigs = append(result.OTLPConfigs, OTLPConfig{
		ClusterName:          clusterName,
		LogName:              logName,
		BufferSizeBytes:      bufBytes,
		BufferFlushInterval:  flush,
		DisableBuiltinLabels: cfg.GetDisableBuiltinLabels(),
		Body:                 bodyTmpl,
		Attributes:           attrs,
		ResourceAttributes:   resAttrs,
	})
	return nil
}

// lowerAll returns a new slice with each element lowercased, or nil for an
// empty/nil input (so the no-capture path stays nil — byte-stable).
func lowerAll(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = strings.ToLower(s)
	}
	return out
}

// AdminSocket returns host and port from admin.address.socket_address. Errors
// if admin is missing or the address is not a socket_address.
func AdminSocket(bs *bootstrapv3.Bootstrap) (host string, port uint32, err error) {
	adm := bs.GetAdmin()
	if adm == nil {
		return "", 0, fmt.Errorf("bootstrap: missing admin")
	}
	addr := adm.GetAddress()
	if addr == nil {
		return "", 0, fmt.Errorf("bootstrap: missing admin.address")
	}
	sa := addr.GetSocketAddress()
	if sa == nil {
		return "", 0, fmt.Errorf("bootstrap: admin.address is not a socket_address")
	}
	return sa.GetAddress(), sa.GetPortValue(), nil
}

// Phase-01 `FirstListenerSocket` and `FirstClusterEndpointSocket` helpers
// retired in phase 02 — listener and cluster traversal moved into
// `internal/listener.Manager` and `internal/cluster.Manager` respectively
// (ADR-0022).
