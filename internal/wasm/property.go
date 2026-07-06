// property.go — framework-side proxy_get_property full ~70-path roster per
// 25.2 SPEC §3.1 property.go bullet + §11.4 + AMEND-B4 + R-25.2-4 +
// ADR-0206.
//
// # Design (per AMEND-B4 + cpp-host byte-faithful)
//
// proxy_get_property's wire-contract per proxy-wasm spec README §Serialization
// + cpp-host context.cc:1047-1058: the guest passes a NUL-delimited byte
// path (e.g., `request\0method` for request.method; `metadata\0envoy.foo\0bar`
// for metadata["envoy.foo"]["bar"]) + the host returns (value_bytes,
// WasmResult). Absent values return WasmResult::NotFound (=1) byte-faithful
// to upstream (context.cc:1065/1072/1078/1083/1103/1106/1110).
//
// This file owns the framework-side path-parsing + per-root dispatch +
// value serialization. The consumer-side ABICallbacks (Task 15 at
// internal/filter/http/wasm/abi_callbacks.go) implements the PropertyResolver
// interface declared here — supplying the per-stream context (filter
// callbacks + per-stream filterstate bucket + per-stream dynamicmetadata
// bucket + the per-plugin ADR-0144 DownstreamPrincipal + ADR-0177
// httpclient.Cluster references). The split is:
//
//	framework property.go        owns path-parsing + per-root dispatch +
//	                             serialization
//	consumer abi_callbacks.go    implements PropertyResolver (per-stream
//	                             context plumbing into the framework
//	                             dispatch)
//
// # Roster (per AMEND-B4 + cpp-host context.cc:1040-1115)
//
// ~10 dispatched roots + 4 direct tokens; ~70 documented sub-paths:
//
//	request                 (16): path + url_path + host + scheme + method +
//	                              referer + useragent + id + protocol +
//	                              query + headers (header-by-name) +
//	                              headers_bytes + time + size + total_size +
//	                              duration
//	response                ( 6): code + code_details + flags + grpc_status +
//	                              backend_latency + trailers (trailer-by-name)
//	connection              (13): id + mtls + requested_server_name +
//	                              tls_version + termination_details +
//	                              subject_local_certificate +
//	                              subject_peer_certificate +
//	                              uri_san_local_certificate +
//	                              uri_san_peer_certificate +
//	                              dns_san_local_certificate +
//	                              dns_san_peer_certificate +
//	                              sha256_peer_certificate_digest +
//	                              transport_failure_reason
//	source                  ( 2): address + port
//	destination             ( 2): address + port
//	upstream                (14): address + port + local_address + locality +
//	                              transport_failure_reason +
//	                              request_attempt_count +
//	                              cx_pool_ready_duration + num_endpoints +
//	                              subject_local_certificate +
//	                              subject_peer_certificate +
//	                              uri_san_local_certificate +
//	                              uri_san_peer_certificate +
//	                              dns_san_peer_certificate + tls_version
//	xds                     (12): cluster_name + cluster_metadata +
//	                              route_name + route_metadata +
//	                              virtual_host_name + virtual_host_metadata +
//	                              upstream_host_metadata +
//	                              upstream_host_locality_metadata +
//	                              filter_chain_name + listener_metadata +
//	                              listener_direction + node
//	metadata             (var): metadata.<filter>.<key> via ADR-0190
//	                              dynamicmetadata.Bucket
//	filter_state         (var): filter_state.<key> via NEW
//	                              internal/filterstate per ADR-0207
//	upstream_filter_state(var): upstream_filter_state.<key> — DISTINCT root
//	                              co-equal to filter_state per AMEND-B4
//	wasm.<key>           (var): special — proxies via filter_state then
//	                              upstream_filter_state per cpp-host
//	                              context.cc:987-1019; NO foreign-function
//	                              involvement.
//
// 4 direct tokens (NOT dispatched-root prefixed):
//
//	plugin_name        — direct (no sub-segments)
//	plugin_root_id     — direct
//	plugin_vm_id       — direct
//	connection_id      — direct (also accessible via connection.id)
//
// # Co-consumed primitive mapping (per AMEND-B4)
//
// stream-local accessors handle request + response + source + destination +
// wasm-direct-tokens; the consumer-side ABICallbacks supplies these via the
// per-stream filter callbacks at Task 15.
//
// ADR-0144 DownstreamPrincipal RE-CONSUMED for connection.tls.* sub-paths
// (subject + uri_san + dns_san + sha256 + tls_version + mtls).
//
// ADR-0177 httpclient.Cluster RE-CONSUMED for upstream.{cluster fields} +
// xds.cluster_* sub-paths.
//
// ADR-0190 dynamicmetadata.Bucket RE-CONSUMED for metadata.* + xds.*_metadata
// branches.
//
// NEW internal/filterstate/ per ADR-0207 CONSUMED for filter_state.* +
// upstream_filter_state.* + wasm.<key> proxy branches (consumer #2 — Task 9
// is the producer; phase-22.2 lua MIGRATED at Task 10 is consumer #1).
//
// # Serialization (per spec README §Serialization)
//
// String → raw bytes (no NUL terminator, no length prefix).
// uint64 → 8-byte little-endian.
// []byte → raw bytes (used for already-marshaled metadata payloads).
// bool → single byte (0x00 false; 0x01 true).
//
// The pairs wire format (for "request.headers" without a sub-key) is
// produced by EncodePairs at pairs.go; not exposed at this property.go
// surface — guests use the named-header form ("request.headers.<name>") to
// fetch a single value.
//
// # Absent property → NotFound
//
// Per cpp-host (context.cc:1065/1072/1078/1083/1103/1106/1110) and AMEND-B4
// confirmation: every absent property branch returns WasmResult::NotFound (=1)
// with a nil value byte slice. The consumer-side ABICallbacks.GetProperty at
// 25.1 already returns (nil, false) on absent paths; the 25.2 wire-shape
// pin is unchanged.

package wasm

import (
	"encoding/binary"
	"fmt"

	"github.com/pgdad/envoy-go/internal/filterstate"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// PropertyResolver is the consumer-side seam that supplies per-stream
// context to the framework's per-root dispatch. The consumer is
// internal/filter/http/wasm/abi_callbacks.go at Task 15 — it implements
// each accessor by reading from the per-stream filter state + the
// per-stream filterstate.Bucket + the per-stream dynamicmetadata.Bucket
// + the per-plugin ADR-0144/0177 primitives.
//
// All accessors follow the (value, ok) Go-idiomatic pattern. ok=false
// signals an absent value and translates to WasmResult::NotFound at
// ResolveProperty. ok=true with the zero value of the type is a VALID
// empty payload (e.g., an empty string header value or a zero gauge).
//
// The interface is intentionally fine-grained (one method per sub-path)
// rather than a single GetProperty(path) entry — this keeps the dispatch
// switch statement explicit + lets the type system catch missing
// implementations at consumer-side compile time. The cost is interface
// size (~60 methods); the benefit is that adding a new sub-path requires
// touching both this interface AND the per-root dispatch function, so
// roster drift is impossible.
//
// nolint:revive — exported-method-count exceeds golint's threshold; the
// fine-grained roster is intentional per the design above.
type PropertyResolver interface {
	// request.* (16 sub-paths)
	RequestPath() (string, bool)
	RequestURLPath() (string, bool)
	RequestHost() (string, bool)
	RequestScheme() (string, bool)
	RequestMethod() (string, bool)
	RequestReferer() (string, bool)
	RequestUserAgent() (string, bool)
	RequestID() (string, bool)
	RequestProtocol() (string, bool)
	RequestQuery() (string, bool)
	RequestHeader(name string) (string, bool)
	RequestHeadersBytes() (uint64, bool)
	RequestTime() (uint64, bool)
	RequestSize() (uint64, bool)
	RequestTotalSize() (uint64, bool)
	RequestDuration() (uint64, bool)

	// response.* (7 sub-paths — 6 + named-header accessor; named-header
	// added at 25.2 IMPL Task 18 alongside the cross-side header bridge
	// completion + the cross-side abi_callbacks_test.go pin)
	ResponseCode() (uint64, bool)
	ResponseCodeDetails() (string, bool)
	ResponseFlags() (uint64, bool)
	ResponseGrpcStatus() (uint64, bool)
	ResponseBackendLatency() (uint64, bool)
	ResponseTrailer(name string) (string, bool)
	ResponseHeader(name string) (string, bool)

	// connection.* (13 sub-paths — 12 + id)
	ConnectionID() (uint64, bool)
	ConnectionMTLS() (bool, bool)
	ConnectionRequestedServerName() (string, bool)
	ConnectionTLSVersion() (string, bool)
	ConnectionTerminationDetails() (string, bool)
	ConnectionSubjectLocalCertificate() (string, bool)
	ConnectionSubjectPeerCertificate() (string, bool)
	ConnectionURISANLocalCertificate() (string, bool)
	ConnectionURISANPeerCertificate() (string, bool)
	ConnectionDNSSANLocalCertificate() (string, bool)
	ConnectionDNSSANPeerCertificate() (string, bool)
	ConnectionSHA256PeerCertificateDigest() (string, bool)
	ConnectionTransportFailureReason() (string, bool)

	// source.* (2 sub-paths)
	SourceAddress() (string, bool)
	SourcePort() (uint64, bool)

	// destination.* (2 sub-paths)
	DestinationAddress() (string, bool)
	DestinationPort() (uint64, bool)

	// upstream.* (14 sub-paths)
	UpstreamAddress() (string, bool)
	UpstreamPort() (uint64, bool)
	UpstreamLocalAddress() (string, bool)
	UpstreamLocality() (string, bool)
	UpstreamTransportFailureReason() (string, bool)
	UpstreamRequestAttemptCount() (uint64, bool)
	UpstreamCxPoolReadyDuration() (uint64, bool)
	UpstreamNumEndpoints() (uint64, bool)
	UpstreamSubjectLocalCertificate() (string, bool)
	UpstreamSubjectPeerCertificate() (string, bool)
	UpstreamURISANLocalCertificate() (string, bool)
	UpstreamURISANPeerCertificate() (string, bool)
	UpstreamDNSSANPeerCertificate() (string, bool)
	UpstreamTLSVersion() (string, bool)

	// xds.* (12 sub-paths — consolidates listener+route+cluster per AMEND-B4)
	XdsClusterName() (string, bool)
	XdsClusterMetadata() ([]byte, bool)
	XdsRouteName() (string, bool)
	XdsRouteMetadata() ([]byte, bool)
	XdsVirtualHostName() (string, bool)
	XdsVirtualHostMetadata() ([]byte, bool)
	XdsUpstreamHostMetadata() ([]byte, bool)
	XdsUpstreamHostLocalityMetadata() ([]byte, bool)
	XdsFilterChainName() (string, bool)
	XdsListenerMetadata() ([]byte, bool)
	XdsListenerDirection() (uint64, bool)
	XdsNode() ([]byte, bool)

	// metadata.<filter>.<key> — single accessor; per ADR-0190
	// dynamicmetadata.Bucket. Returns marshaled google.protobuf.Value bytes
	// (the consumer is responsible for the wire-shape — typically protobuf
	// serialized). ok=false → NotFound.
	Metadata(filterName, key string) ([]byte, bool)

	// FilterState returns the per-stream filterstate.Bucket for the
	// filter_state.<key> branch + the downstream half of wasm.<key>. May
	// return nil if no bucket is wired (treated as absent — every
	// filter_state.<key> probe returns NotFound).
	FilterState() *filterstate.Bucket

	// UpstreamFilterState returns the per-stream filterstate.Bucket for
	// the upstream_filter_state.<key> branch + the upstream half of
	// wasm.<key>. DISTINCT root co-equal to filter_state per AMEND-B4.
	// May return nil (treated as absent).
	UpstreamFilterState() *filterstate.Bucket

	// 4 direct tokens (NOT dispatched-root prefixed per AMEND-B4).
	PluginName() (string, bool)
	PluginRootID() (string, bool)
	PluginVMID() (string, bool)
	// connection_id is exposed both as a direct token AND under
	// connection.id; both forms resolve via ConnectionID().
}

// parsePathSegments splits the NUL-delimited byte path into string
// segments per spec README §Serialization + cpp-host context.cc:1047-1058.
//
// Semantics:
//
//   - Empty input → nil (caller translates to NotFound).
//   - Trailing NUL is tolerated (matches cpp-host's tokenizer which
//     terminates iteration when no further non-empty segment remains).
//   - Any EMPTY segment (double-NUL anywhere, leading NUL, etc.) → nil.
//     cpp-host's tokenizer rejects empty segments; envoy-go mirrors the
//     reject byte-faithfully via the nil return (NotFound) — the consumer
//     dispatch treats nil exactly like an unknown path.
//   - Single non-empty segment (e.g., "plugin_name") → 1-element slice.
//     This is the direct-token form per AMEND-B4.
//
// The Go strings.Split(string(b), "\x00") works correctly on the
// little-endian wire format the guest produces; no host-byte-order
// translation needed (envoy-go runs on little-endian targets per
// pairs.go file header).
func parsePathSegments(path []byte) []string {
	if len(path) == 0 {
		return nil
	}
	// Walk byte-by-byte; emit a segment at each NUL boundary. Reject
	// empty segments (including a leading NUL → empty first segment;
	// double-NUL → empty middle segment). Tolerate a trailing NUL
	// (means "the path ends at the prior segment boundary").
	out := make([]string, 0, 4)
	start := 0
	for i := 0; i <= len(path); i++ {
		if i < len(path) && path[i] != 0x00 {
			continue
		}
		// Hit a NUL or the end-of-buffer. The segment runs [start, i).
		if i == start {
			// Empty segment. Tolerate ONLY at the trailing position
			// (when i == len(path) AND we have already emitted at
			// least one segment).
			if i == len(path) && len(out) > 0 {
				return out
			}
			// Otherwise empty segment is a reject — return nil.
			return nil
		}
		out = append(out, string(path[start:i]))
		start = i + 1
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// ResolveProperty is the top-level entry. Parses the NUL-delimited path +
// dispatches to the per-root resolver. Returns (value bytes per spec
// README §Serialization, status code). Absent properties return
// (nil, WasmResultNotFound) byte-faithful to cpp-host.
//
// Per AMEND-B4 + cpp-host context.cc:1040-1115, the dispatch is FIRST on
// the leading segment (the root); the per-root resolver handles the
// remaining segments. The 4 direct tokens (plugin_name, plugin_root_id,
// plugin_vm_id, connection_id) are dispatched on the single-segment form.
func ResolveProperty(resolver PropertyResolver, path []byte) ([]byte, abi.WasmResult) {
	segments := parsePathSegments(path)
	if len(segments) == 0 {
		return nil, abi.WasmResultNotFound
	}

	// Direct tokens — single-segment form per AMEND-B4. These are NOT
	// dispatched roots (no sub-path enumeration); guest passes just the
	// token name with no NUL separator.
	if len(segments) == 1 {
		return resolveDirectToken(segments[0], resolver)
	}

	// ~10 dispatched roots — multi-segment form. Each root takes the
	// segments[1:] tail as its sub-path enumeration.
	root := segments[0]
	tail := segments[1:]
	switch root {
	case "request":
		return resolveRequest(tail, resolver)
	case "response":
		return resolveResponse(tail, resolver)
	case "connection":
		return resolveConnection(tail, resolver)
	case "source":
		return resolveSource(tail, resolver)
	case "destination":
		return resolveDestination(tail, resolver)
	case "upstream":
		return resolveUpstream(tail, resolver)
	case "xds":
		return resolveXds(tail, resolver)
	case "metadata":
		return resolveMetadata(tail, resolver)
	case "filter_state":
		return resolveFilterState(tail, resolver, downstreamBucket)
	case "upstream_filter_state":
		return resolveFilterState(tail, resolver, upstreamBucket)
	case "wasm":
		return resolveWasm(tail, resolver)
	}
	return nil, abi.WasmResultNotFound
}

// resolveDirectToken dispatches the 4 direct tokens per AMEND-B4. The
// single-segment form `plugin_name` resolves to PluginName(); `plugin_root_id`
// to PluginRootID(); `plugin_vm_id` to PluginVMID(); `connection_id` to
// ConnectionID() (also accessible via the connection.id dispatched form).
// Any other single-segment token returns NotFound.
func resolveDirectToken(token string, r PropertyResolver) ([]byte, abi.WasmResult) {
	switch token {
	case "plugin_name":
		return serializeString(r.PluginName())
	case "plugin_root_id":
		return serializeString(r.PluginRootID())
	case "plugin_vm_id":
		return serializeString(r.PluginVMID())
	case "connection_id":
		return serializeUint64(r.ConnectionID())
	}
	return nil, abi.WasmResultNotFound
}

// resolveRequest handles the request.* subtree per AMEND-B4 (16 sub-paths).
func resolveRequest(sub []string, r PropertyResolver) ([]byte, abi.WasmResult) {
	if len(sub) == 0 {
		return nil, abi.WasmResultNotFound
	}
	switch sub[0] {
	case "path":
		return serializeString(r.RequestPath())
	case "url_path":
		return serializeString(r.RequestURLPath())
	case "host":
		return serializeString(r.RequestHost())
	case "scheme":
		return serializeString(r.RequestScheme())
	case "method":
		return serializeString(r.RequestMethod())
	case "referer":
		return serializeString(r.RequestReferer())
	case "useragent":
		return serializeString(r.RequestUserAgent())
	case "id":
		return serializeString(r.RequestID())
	case "protocol":
		return serializeString(r.RequestProtocol())
	case "query":
		return serializeString(r.RequestQuery())
	case "headers":
		// request.headers.<name> form. The bare "request.headers" (no
		// trailing name) would return the full pairs wire format; not
		// exposed at this surface — guests use the named-header form.
		if len(sub) < 2 {
			return nil, abi.WasmResultNotFound
		}
		return serializeString(r.RequestHeader(sub[1]))
	case "headers_bytes":
		return serializeUint64(r.RequestHeadersBytes())
	case "time":
		return serializeUint64(r.RequestTime())
	case "size":
		return serializeUint64(r.RequestSize())
	case "total_size":
		return serializeUint64(r.RequestTotalSize())
	case "duration":
		return serializeUint64(r.RequestDuration())
	}
	return nil, abi.WasmResultNotFound
}

// resolveResponse handles the response.* subtree per AMEND-B4 (6 sub-paths).
func resolveResponse(sub []string, r PropertyResolver) ([]byte, abi.WasmResult) {
	if len(sub) == 0 {
		return nil, abi.WasmResultNotFound
	}
	switch sub[0] {
	case "code":
		return serializeUint64(r.ResponseCode())
	case "code_details":
		return serializeString(r.ResponseCodeDetails())
	case "flags":
		return serializeUint64(r.ResponseFlags())
	case "grpc_status":
		return serializeUint64(r.ResponseGrpcStatus())
	case "backend_latency":
		return serializeUint64(r.ResponseBackendLatency())
	case "trailers":
		if len(sub) < 2 {
			return nil, abi.WasmResultNotFound
		}
		return serializeString(r.ResponseTrailer(sub[1]))
	case "headers":
		// response.headers.<name> form — symmetric to request.headers.<name>
		// per the 25.1 named-header accessor pattern. Added at 25.2 IMPL
		// Task 18 alongside the cross-side header bridge completion.
		if len(sub) < 2 {
			return nil, abi.WasmResultNotFound
		}
		return serializeString(r.ResponseHeader(sub[1]))
	}
	return nil, abi.WasmResultNotFound
}

// resolveConnection handles the connection.* subtree per AMEND-B4 (13
// sub-paths). The TLS-cert sub-paths consume ADR-0144 DownstreamPrincipal
// at the consumer-side ABICallbacks (Task 15).
func resolveConnection(sub []string, r PropertyResolver) ([]byte, abi.WasmResult) {
	if len(sub) == 0 {
		return nil, abi.WasmResultNotFound
	}
	switch sub[0] {
	case "id":
		return serializeUint64(r.ConnectionID())
	case "mtls":
		return serializeBool(r.ConnectionMTLS())
	case "requested_server_name":
		return serializeString(r.ConnectionRequestedServerName())
	case "tls_version":
		return serializeString(r.ConnectionTLSVersion())
	case "termination_details":
		return serializeString(r.ConnectionTerminationDetails())
	case "subject_local_certificate":
		return serializeString(r.ConnectionSubjectLocalCertificate())
	case "subject_peer_certificate":
		return serializeString(r.ConnectionSubjectPeerCertificate())
	case "uri_san_local_certificate":
		return serializeString(r.ConnectionURISANLocalCertificate())
	case "uri_san_peer_certificate":
		return serializeString(r.ConnectionURISANPeerCertificate())
	case "dns_san_local_certificate":
		return serializeString(r.ConnectionDNSSANLocalCertificate())
	case "dns_san_peer_certificate":
		return serializeString(r.ConnectionDNSSANPeerCertificate())
	case "sha256_peer_certificate_digest":
		return serializeString(r.ConnectionSHA256PeerCertificateDigest())
	case "transport_failure_reason":
		return serializeString(r.ConnectionTransportFailureReason())
	}
	return nil, abi.WasmResultNotFound
}

// resolveSource handles the source.* subtree per AMEND-B4 (2 sub-paths).
func resolveSource(sub []string, r PropertyResolver) ([]byte, abi.WasmResult) {
	if len(sub) == 0 {
		return nil, abi.WasmResultNotFound
	}
	switch sub[0] {
	case "address":
		return serializeString(r.SourceAddress())
	case "port":
		return serializeUint64(r.SourcePort())
	}
	return nil, abi.WasmResultNotFound
}

// resolveDestination handles the destination.* subtree per AMEND-B4 (2
// sub-paths).
func resolveDestination(sub []string, r PropertyResolver) ([]byte, abi.WasmResult) {
	if len(sub) == 0 {
		return nil, abi.WasmResultNotFound
	}
	switch sub[0] {
	case "address":
		return serializeString(r.DestinationAddress())
	case "port":
		return serializeUint64(r.DestinationPort())
	}
	return nil, abi.WasmResultNotFound
}

// resolveUpstream handles the upstream.* subtree per AMEND-B4 (14
// sub-paths). The address + port + cluster sub-paths consume ADR-0177
// httpclient.Cluster at the consumer-side ABICallbacks (Task 15); the
// TLS-cert sub-paths consume ADR-0144 DownstreamPrincipal applied to the
// upstream-side connection.
func resolveUpstream(sub []string, r PropertyResolver) ([]byte, abi.WasmResult) {
	if len(sub) == 0 {
		return nil, abi.WasmResultNotFound
	}
	switch sub[0] {
	case "address":
		return serializeString(r.UpstreamAddress())
	case "port":
		return serializeUint64(r.UpstreamPort())
	case "local_address":
		return serializeString(r.UpstreamLocalAddress())
	case "locality":
		return serializeString(r.UpstreamLocality())
	case "transport_failure_reason":
		return serializeString(r.UpstreamTransportFailureReason())
	case "request_attempt_count":
		return serializeUint64(r.UpstreamRequestAttemptCount())
	case "cx_pool_ready_duration":
		return serializeUint64(r.UpstreamCxPoolReadyDuration())
	case "num_endpoints":
		return serializeUint64(r.UpstreamNumEndpoints())
	case "subject_local_certificate":
		return serializeString(r.UpstreamSubjectLocalCertificate())
	case "subject_peer_certificate":
		return serializeString(r.UpstreamSubjectPeerCertificate())
	case "uri_san_local_certificate":
		return serializeString(r.UpstreamURISANLocalCertificate())
	case "uri_san_peer_certificate":
		return serializeString(r.UpstreamURISANPeerCertificate())
	case "dns_san_peer_certificate":
		return serializeString(r.UpstreamDNSSANPeerCertificate())
	case "tls_version":
		return serializeString(r.UpstreamTLSVersion())
	}
	return nil, abi.WasmResultNotFound
}

// resolveXds handles the xds.* subtree per AMEND-B4 (12 sub-paths). Per
// AMEND-B4 substantive refinement, `xds.*` CONSOLIDATES listener + route +
// cluster metadata; there are NO standalone `listener.*` / `route.*` roots.
// The metadata sub-paths consume ADR-0190 dynamicmetadata.Bucket at the
// consumer-side ABICallbacks (Task 15).
func resolveXds(sub []string, r PropertyResolver) ([]byte, abi.WasmResult) {
	if len(sub) == 0 {
		return nil, abi.WasmResultNotFound
	}
	switch sub[0] {
	case "cluster_name":
		return serializeString(r.XdsClusterName())
	case "cluster_metadata":
		return serializeBytes(r.XdsClusterMetadata())
	case "route_name":
		return serializeString(r.XdsRouteName())
	case "route_metadata":
		return serializeBytes(r.XdsRouteMetadata())
	case "virtual_host_name":
		return serializeString(r.XdsVirtualHostName())
	case "virtual_host_metadata":
		return serializeBytes(r.XdsVirtualHostMetadata())
	case "upstream_host_metadata":
		return serializeBytes(r.XdsUpstreamHostMetadata())
	case "upstream_host_locality_metadata":
		return serializeBytes(r.XdsUpstreamHostLocalityMetadata())
	case "filter_chain_name":
		return serializeString(r.XdsFilterChainName())
	case "listener_metadata":
		return serializeBytes(r.XdsListenerMetadata())
	case "listener_direction":
		return serializeUint64(r.XdsListenerDirection())
	case "node":
		return serializeBytes(r.XdsNode())
	}
	return nil, abi.WasmResultNotFound
}

// resolveMetadata handles the metadata.<filter>.<key> branch per
// AMEND-B4. The path shape is `metadata\0<filter>\0<key>`; sub-paths with
// fewer than 2 segments return NotFound. Consumes ADR-0190
// dynamicmetadata.Bucket at the consumer-side ABICallbacks (Task 15).
func resolveMetadata(sub []string, r PropertyResolver) ([]byte, abi.WasmResult) {
	if len(sub) < 2 {
		return nil, abi.WasmResultNotFound
	}
	return serializeBytes(r.Metadata(sub[0], sub[1]))
}

// bucketSelector identifies which of the two filterstate.Bucket roots
// (downstream filter_state vs upstream upstream_filter_state) a
// resolveFilterState call should consult. The wasm.<key> branch
// instead uses resolveWasm which probes BOTH buckets in order.
type bucketSelector int

const (
	downstreamBucket bucketSelector = iota
	upstreamBucket
)

// resolveFilterState handles the filter_state.<key> + upstream_filter_state
// .<key> branches per AMEND-B4 + ADR-0207. Both branches share the same
// dispatch logic — the only difference is WHICH Bucket the resolver hands
// back. The path shape is `<root>\0<key>`; sub-paths with no key return
// NotFound. The FilterStateObject's Marshal() output is the wire bytes per
// spec README §Serialization.
func resolveFilterState(sub []string, r PropertyResolver, which bucketSelector) ([]byte, abi.WasmResult) {
	if len(sub) == 0 {
		return nil, abi.WasmResultNotFound
	}
	var bucket *filterstate.Bucket
	switch which {
	case downstreamBucket:
		bucket = r.FilterState()
	case upstreamBucket:
		bucket = r.UpstreamFilterState()
	}
	if bucket == nil {
		return nil, abi.WasmResultNotFound
	}
	obj, ok := bucket.Get(sub[0])
	if !ok {
		return nil, abi.WasmResultNotFound
	}
	payload, err := obj.Marshal()
	if err != nil {
		// FilterStateObject.Marshal failure → SerializationFailure per
		// spec README §Status enumeration. NOT a NotFound (the entry
		// exists; just couldn't be serialized).
		return nil, abi.WasmResultSerializationFailure
	}
	return payload, abi.WasmResultOk
}

// resolveWasm handles the wasm.<key> proxy class per AMEND-B4 + cpp-host
// context.cc:987-1019. Probes the downstream filter_state Bucket FIRST;
// if absent, falls through to the upstream upstream_filter_state Bucket.
// Returns NotFound only if BOTH buckets lack the key. NO foreign-function
// dispatch involvement.
func resolveWasm(sub []string, r PropertyResolver) ([]byte, abi.WasmResult) {
	if len(sub) == 0 {
		return nil, abi.WasmResultNotFound
	}
	// Try downstream bucket first.
	if bucket := r.FilterState(); bucket != nil {
		if obj, ok := bucket.Get(sub[0]); ok {
			payload, err := obj.Marshal()
			if err != nil {
				return nil, abi.WasmResultSerializationFailure
			}
			return payload, abi.WasmResultOk
		}
	}
	// Fallthrough to upstream bucket.
	if bucket := r.UpstreamFilterState(); bucket != nil {
		if obj, ok := bucket.Get(sub[0]); ok {
			payload, err := obj.Marshal()
			if err != nil {
				return nil, abi.WasmResultSerializationFailure
			}
			return payload, abi.WasmResultOk
		}
	}
	return nil, abi.WasmResultNotFound
}

// -----------------------------------------------------------------------------
// Serialization helpers (per spec README §Serialization).
// -----------------------------------------------------------------------------

// serializeString returns the string bytes verbatim + Ok if ok=true; else
// (nil, NotFound). Empty string with ok=true is a VALID empty payload (the
// returned []byte has len==0 but is non-nil).
func serializeString(v string, ok bool) ([]byte, abi.WasmResult) {
	if !ok {
		return nil, abi.WasmResultNotFound
	}
	return []byte(v), abi.WasmResultOk
}

// serializeUint64 returns the 8-byte little-endian encoding + Ok if
// ok=true; else (nil, NotFound).
func serializeUint64(v uint64, ok bool) ([]byte, abi.WasmResult) {
	if !ok {
		return nil, abi.WasmResultNotFound
	}
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b, abi.WasmResultOk
}

// serializeBool returns the 1-byte encoding (0x00 false; 0x01 true) + Ok
// if ok=true; else (nil, NotFound).
func serializeBool(v, ok bool) ([]byte, abi.WasmResult) {
	if !ok {
		return nil, abi.WasmResultNotFound
	}
	if v {
		return []byte{0x01}, abi.WasmResultOk
	}
	return []byte{0x00}, abi.WasmResultOk
}

// serializeBytes returns the bytes verbatim + Ok if ok=true; else (nil,
// NotFound). Empty []byte with ok=true is a VALID empty payload.
func serializeBytes(v []byte, ok bool) ([]byte, abi.WasmResult) {
	if !ok {
		return nil, abi.WasmResultNotFound
	}
	if v == nil {
		// Preserve "ok with empty payload" semantic — return a non-nil
		// zero-length slice so the caller can distinguish from the
		// nil-on-NotFound return.
		return []byte{}, abi.WasmResultOk
	}
	return v, abi.WasmResultOk
}

// serializeValue is the type-dispatched serializer for cases where the
// caller has an `any` value (e.g., a generic resolver that returns
// `interface{}` rather than a typed accessor). The per-root dispatch
// above uses the typed serialize* helpers directly; this entry is
// exposed for future extensions + for round-trip tests at property_test.go.
//
// Supported types: string, []byte, uint64, bool.
// Unsupported types return (nil, error).
func serializeValue(v any) ([]byte, error) {
	switch x := v.(type) {
	case string:
		return []byte(x), nil
	case []byte:
		if x == nil {
			return []byte{}, nil
		}
		return x, nil
	case uint64:
		b := make([]byte, 8)
		binary.LittleEndian.PutUint64(b, x)
		return b, nil
	case bool:
		if x {
			return []byte{0x01}, nil
		}
		return []byte{0x00}, nil
	}
	return nil, fmt.Errorf("property: serializeValue: unsupported type %T", v)
}
