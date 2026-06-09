package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"

	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	bootstrapv3 "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	fileaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
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
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"gopkg.in/yaml.v3"

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
)

// AccessLogConfig is the parsed-but-not-yet-opened representation of one
// envoy.access_loggers.file entry from HCM access_log[]. The sink itself is
// constructed in cmd/envoy-go/main.go after Load returns; this struct carries
// only the parse-time data.
type AccessLogConfig struct {
	Path string
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
	return result, nil
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
	if tc.GetTypeUrl() != fileAccessLogTypeURL {
		// Non-file typed_config (stdout, tcp_grpc, open_telemetry, etc.) —
		// silently ignored per ADR-0041 amendment / ADR-0067.
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
