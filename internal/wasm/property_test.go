// Tests for the framework-side proxy_get_property full ~70-path roster per
// 25.2 SPEC §3.1 property.go bullet + §11.4 + AMEND-B4 + R-25.2-4.
//
// Coverage (per PLAN Task 13):
//
//   - parsePathSegments NUL-delimited edge cases: nominal split; empty path
//     → nil; double-NUL (empty segment) → nil; trailing NUL tolerated;
//     leading NUL (empty first segment) → nil; single segment (direct
//     token) preserved.
//   - ResolveProperty entry: empty path → NotFound; unknown root → NotFound;
//     unknown sub-path within known root → NotFound; absent value within
//     known sub-path → NotFound.
//   - Direct tokens (4): plugin_name + plugin_root_id + plugin_vm_id +
//     connection_id round-trip with mock resolver.
//   - request.* (16 sub-paths): path, url_path, host, scheme, method,
//     referer, headers (byname), headers_bytes, time, id, useragent, size,
//     total_size, duration, protocol, query.
//   - response.* (6 sub-paths): code, code_details, trailers, flags,
//     grpc_status, backend_latency.
//   - connection.* (12 sub-paths + id): mtls, requested_server_name,
//     tls_version, termination_details, subject_local_certificate,
//     subject_peer_certificate, uri_san_local_certificate,
//     uri_san_peer_certificate, dns_san_local_certificate,
//     dns_san_peer_certificate, sha256_peer_certificate_digest,
//     transport_failure_reason, id.
//   - source.* (2 sub-paths): address, port.
//   - destination.* (2 sub-paths): address, port.
//   - upstream.* (14 sub-paths): address, port, local_address, locality,
//     transport_failure_reason, request_attempt_count,
//     cx_pool_ready_duration, num_endpoints, subject_local_certificate,
//     subject_peer_certificate, uri_san_local_certificate,
//     uri_san_peer_certificate, dns_san_peer_certificate, tls_version.
//   - xds.* (12 sub-paths consolidating listener+route+cluster per
//     AMEND-B4): cluster_name, cluster_metadata, route_name, route_metadata,
//     virtual_host_name, virtual_host_metadata, upstream_host_metadata,
//     upstream_host_locality_metadata, filter_chain_name, listener_metadata,
//     listener_direction, node.
//   - metadata.<filter>.<key> branch via ADR-0190 dynamicmetadata.Bucket mock.
//   - filter_state.<key> branch via NEW internal/filterstate.Bucket consumer #2
//     (co-consumed primitive integration round-trip per ADR-0207).
//   - upstream_filter_state.<key> branch DISTINCT root per AMEND-B4.
//   - wasm.<key> proxy class: proxies via filter_state then
//     upstream_filter_state per cpp-host context.cc:987-1019; NO foreign-
//     function involvement.
//   - Serialization round-trips: string → raw bytes; uint64 → 8-byte LE;
//     int64 → 8-byte LE; bool → 1 byte (0 or 1).
//   - Absent-property NotFound byte-faithful across each root.

package wasm

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/esalaine/envoy-go/internal/filterstate"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// -----------------------------------------------------------------------------
// Mock PropertyResolver — table-driven test fixtures wire canned values into
// this struct. nil-bool defaults to (zero, false) per Go's natural zero values
// + the per-method dispatch checks the explicit "have" flag. The Bucket
// fields are wired only when the test needs filter_state / metadata round-trip.
// -----------------------------------------------------------------------------

type mockPropertyResolver struct {
	// request.* (16)
	reqPath           string
	reqPathOK         bool
	reqURLPath        string
	reqURLPathOK      bool
	reqHost           string
	reqHostOK         bool
	reqScheme         string
	reqSchemeOK       bool
	reqMethod         string
	reqMethodOK       bool
	reqReferer        string
	reqRefererOK      bool
	reqUserAgent      string
	reqUserAgentOK    bool
	reqID             string
	reqIDOK           bool
	reqProtocol       string
	reqProtocolOK     bool
	reqQuery          string
	reqQueryOK        bool
	reqHeaders        map[string]string
	reqHeadersBytes   uint64
	reqHeadersBytesOK bool
	reqTime           uint64
	reqTimeOK         bool
	reqSize           uint64
	reqSizeOK         bool
	reqTotalSize      uint64
	reqTotalSizeOK    bool
	reqDuration       uint64
	reqDurationOK     bool

	// response.* (6)
	respCode             uint64
	respCodeOK           bool
	respCodeDetails      string
	respCodeDetailsOK    bool
	respFlags            uint64
	respFlagsOK          bool
	respGrpcStatus       uint64
	respGrpcStatusOK     bool
	respTrailers         map[string]string
	respBackendLatency   uint64
	respBackendLatencyOK bool

	// connection.* (12+id)
	connID                       uint64
	connIDOK                     bool
	connMTLS                     bool
	connMTLSOK                   bool
	connRequestedServerName      string
	connRequestedServerNameOK    bool
	connTLSVersion               string
	connTLSVersionOK             bool
	connTerminationDetails       string
	connTerminationDetailsOK     bool
	connSubjectLocalCert         string
	connSubjectLocalCertOK       bool
	connSubjectPeerCert          string
	connSubjectPeerCertOK        bool
	connURISANLocalCert          string
	connURISANLocalCertOK        bool
	connURISANPeerCert           string
	connURISANPeerCertOK         bool
	connDNSSANLocalCert          string
	connDNSSANLocalCertOK        bool
	connDNSSANPeerCert           string
	connDNSSANPeerCertOK         bool
	connSHA256PeerCertDigest     string
	connSHA256PeerCertDigestOK   bool
	connTransportFailureReason   string
	connTransportFailureReasonOK bool

	// source.* (2)
	srcAddress   string
	srcAddressOK bool
	srcPort      uint64
	srcPortOK    bool

	// destination.* (2)
	dstAddress   string
	dstAddressOK bool
	dstPort      uint64
	dstPortOK    bool

	// upstream.* (14)
	upAddress                  string
	upAddressOK                bool
	upPort                     uint64
	upPortOK                   bool
	upLocalAddress             string
	upLocalAddressOK           bool
	upLocality                 string
	upLocalityOK               bool
	upTransportFailureReason   string
	upTransportFailureReasonOK bool
	upRequestAttemptCount      uint64
	upRequestAttemptCountOK    bool
	upCxPoolReadyDuration      uint64
	upCxPoolReadyDurationOK    bool
	upNumEndpoints             uint64
	upNumEndpointsOK           bool
	upSubjectLocalCert         string
	upSubjectLocalCertOK       bool
	upSubjectPeerCert          string
	upSubjectPeerCertOK        bool
	upURISANLocalCert          string
	upURISANLocalCertOK        bool
	upURISANPeerCert           string
	upURISANPeerCertOK         bool
	upDNSSANPeerCert           string
	upDNSSANPeerCertOK         bool
	upTLSVersion               string
	upTLSVersionOK             bool

	// xds.* (12)
	xdsClusterName                    string
	xdsClusterNameOK                  bool
	xdsClusterMetadata                []byte
	xdsClusterMetadataOK              bool
	xdsRouteName                      string
	xdsRouteNameOK                    bool
	xdsRouteMetadata                  []byte
	xdsRouteMetadataOK                bool
	xdsVirtualHostName                string
	xdsVirtualHostNameOK              bool
	xdsVirtualHostMetadata            []byte
	xdsVirtualHostMetadataOK          bool
	xdsUpstreamHostMetadata           []byte
	xdsUpstreamHostMetadataOK         bool
	xdsUpstreamHostLocalityMetadata   []byte
	xdsUpstreamHostLocalityMetadataOK bool
	xdsFilterChainName                string
	xdsFilterChainNameOK              bool
	xdsListenerMetadata               []byte
	xdsListenerMetadataOK             bool
	xdsListenerDirection              uint64
	xdsListenerDirectionOK            bool
	xdsNode                           []byte
	xdsNodeOK                         bool

	// metadata.* — keyed by (filter, key) tuple per ADR-0190.
	metadata map[string][]byte // key = filterName + "\x1f" + key (US separator)

	// filter_state / upstream_filter_state per ADR-0207.
	filterStateBucket         *filterstate.Bucket
	upstreamFilterStateBucket *filterstate.Bucket

	// Direct tokens (4).
	pluginName   string
	pluginRootID string
	pluginVMID   string
}

// Implement PropertyResolver on *mockPropertyResolver.
//
// All methods follow the (value, ok) Go-idiomatic pattern; ok=false signals
// absent — translates to WasmResultNotFound at ResolveProperty.

func (m *mockPropertyResolver) RequestPath() (string, bool)    { return m.reqPath, m.reqPathOK }
func (m *mockPropertyResolver) RequestURLPath() (string, bool) { return m.reqURLPath, m.reqURLPathOK }
func (m *mockPropertyResolver) RequestHost() (string, bool)    { return m.reqHost, m.reqHostOK }
func (m *mockPropertyResolver) RequestScheme() (string, bool)  { return m.reqScheme, m.reqSchemeOK }
func (m *mockPropertyResolver) RequestMethod() (string, bool)  { return m.reqMethod, m.reqMethodOK }
func (m *mockPropertyResolver) RequestReferer() (string, bool) { return m.reqReferer, m.reqRefererOK }
func (m *mockPropertyResolver) RequestUserAgent() (string, bool) {
	return m.reqUserAgent, m.reqUserAgentOK
}
func (m *mockPropertyResolver) RequestID() (string, bool) { return m.reqID, m.reqIDOK }
func (m *mockPropertyResolver) RequestProtocol() (string, bool) {
	return m.reqProtocol, m.reqProtocolOK
}
func (m *mockPropertyResolver) RequestQuery() (string, bool) { return m.reqQuery, m.reqQueryOK }

func (m *mockPropertyResolver) RequestHeader(name string) (string, bool) {
	if m.reqHeaders == nil {
		return "", false
	}
	v, ok := m.reqHeaders[name]
	return v, ok
}

func (m *mockPropertyResolver) RequestHeadersBytes() (uint64, bool) {
	return m.reqHeadersBytes, m.reqHeadersBytesOK
}
func (m *mockPropertyResolver) RequestTime() (uint64, bool) { return m.reqTime, m.reqTimeOK }
func (m *mockPropertyResolver) RequestSize() (uint64, bool) { return m.reqSize, m.reqSizeOK }
func (m *mockPropertyResolver) RequestTotalSize() (uint64, bool) {
	return m.reqTotalSize, m.reqTotalSizeOK
}
func (m *mockPropertyResolver) RequestDuration() (uint64, bool) {
	return m.reqDuration, m.reqDurationOK
}

func (m *mockPropertyResolver) ResponseCode() (uint64, bool) { return m.respCode, m.respCodeOK }
func (m *mockPropertyResolver) ResponseCodeDetails() (string, bool) {
	return m.respCodeDetails, m.respCodeDetailsOK
}
func (m *mockPropertyResolver) ResponseFlags() (uint64, bool) { return m.respFlags, m.respFlagsOK }
func (m *mockPropertyResolver) ResponseGrpcStatus() (uint64, bool) {
	return m.respGrpcStatus, m.respGrpcStatusOK
}
func (m *mockPropertyResolver) ResponseBackendLatency() (uint64, bool) {
	return m.respBackendLatency, m.respBackendLatencyOK
}

func (m *mockPropertyResolver) ResponseTrailer(name string) (string, bool) {
	if m.respTrailers == nil {
		return "", false
	}
	v, ok := m.respTrailers[name]
	return v, ok
}

// ResponseHeader satisfies the 25.2 IMPL Task 18 PropertyResolver extension.
// The mock holds no separate response-headers map at this Task; returning
// ("", false) is the table-driven test-fixture default for any path that
// doesn't exercise the named-header surface.
func (m *mockPropertyResolver) ResponseHeader(_ string) (string, bool) {
	return "", false
}

func (m *mockPropertyResolver) ConnectionID() (uint64, bool) { return m.connID, m.connIDOK }
func (m *mockPropertyResolver) ConnectionMTLS() (bool, bool) { return m.connMTLS, m.connMTLSOK }
func (m *mockPropertyResolver) ConnectionRequestedServerName() (string, bool) {
	return m.connRequestedServerName, m.connRequestedServerNameOK
}
func (m *mockPropertyResolver) ConnectionTLSVersion() (string, bool) {
	return m.connTLSVersion, m.connTLSVersionOK
}
func (m *mockPropertyResolver) ConnectionTerminationDetails() (string, bool) {
	return m.connTerminationDetails, m.connTerminationDetailsOK
}
func (m *mockPropertyResolver) ConnectionSubjectLocalCertificate() (string, bool) {
	return m.connSubjectLocalCert, m.connSubjectLocalCertOK
}
func (m *mockPropertyResolver) ConnectionSubjectPeerCertificate() (string, bool) {
	return m.connSubjectPeerCert, m.connSubjectPeerCertOK
}
func (m *mockPropertyResolver) ConnectionURISANLocalCertificate() (string, bool) {
	return m.connURISANLocalCert, m.connURISANLocalCertOK
}
func (m *mockPropertyResolver) ConnectionURISANPeerCertificate() (string, bool) {
	return m.connURISANPeerCert, m.connURISANPeerCertOK
}
func (m *mockPropertyResolver) ConnectionDNSSANLocalCertificate() (string, bool) {
	return m.connDNSSANLocalCert, m.connDNSSANLocalCertOK
}
func (m *mockPropertyResolver) ConnectionDNSSANPeerCertificate() (string, bool) {
	return m.connDNSSANPeerCert, m.connDNSSANPeerCertOK
}
func (m *mockPropertyResolver) ConnectionSHA256PeerCertificateDigest() (string, bool) {
	return m.connSHA256PeerCertDigest, m.connSHA256PeerCertDigestOK
}
func (m *mockPropertyResolver) ConnectionTransportFailureReason() (string, bool) {
	return m.connTransportFailureReason, m.connTransportFailureReasonOK
}

func (m *mockPropertyResolver) SourceAddress() (string, bool) { return m.srcAddress, m.srcAddressOK }
func (m *mockPropertyResolver) SourcePort() (uint64, bool)    { return m.srcPort, m.srcPortOK }
func (m *mockPropertyResolver) DestinationAddress() (string, bool) {
	return m.dstAddress, m.dstAddressOK
}
func (m *mockPropertyResolver) DestinationPort() (uint64, bool) { return m.dstPort, m.dstPortOK }

func (m *mockPropertyResolver) UpstreamAddress() (string, bool) { return m.upAddress, m.upAddressOK }
func (m *mockPropertyResolver) UpstreamPort() (uint64, bool)    { return m.upPort, m.upPortOK }
func (m *mockPropertyResolver) UpstreamLocalAddress() (string, bool) {
	return m.upLocalAddress, m.upLocalAddressOK
}
func (m *mockPropertyResolver) UpstreamLocality() (string, bool) {
	return m.upLocality, m.upLocalityOK
}
func (m *mockPropertyResolver) UpstreamTransportFailureReason() (string, bool) {
	return m.upTransportFailureReason, m.upTransportFailureReasonOK
}
func (m *mockPropertyResolver) UpstreamRequestAttemptCount() (uint64, bool) {
	return m.upRequestAttemptCount, m.upRequestAttemptCountOK
}
func (m *mockPropertyResolver) UpstreamCxPoolReadyDuration() (uint64, bool) {
	return m.upCxPoolReadyDuration, m.upCxPoolReadyDurationOK
}
func (m *mockPropertyResolver) UpstreamNumEndpoints() (uint64, bool) {
	return m.upNumEndpoints, m.upNumEndpointsOK
}
func (m *mockPropertyResolver) UpstreamSubjectLocalCertificate() (string, bool) {
	return m.upSubjectLocalCert, m.upSubjectLocalCertOK
}
func (m *mockPropertyResolver) UpstreamSubjectPeerCertificate() (string, bool) {
	return m.upSubjectPeerCert, m.upSubjectPeerCertOK
}
func (m *mockPropertyResolver) UpstreamURISANLocalCertificate() (string, bool) {
	return m.upURISANLocalCert, m.upURISANLocalCertOK
}
func (m *mockPropertyResolver) UpstreamURISANPeerCertificate() (string, bool) {
	return m.upURISANPeerCert, m.upURISANPeerCertOK
}
func (m *mockPropertyResolver) UpstreamDNSSANPeerCertificate() (string, bool) {
	return m.upDNSSANPeerCert, m.upDNSSANPeerCertOK
}
func (m *mockPropertyResolver) UpstreamTLSVersion() (string, bool) {
	return m.upTLSVersion, m.upTLSVersionOK
}

func (m *mockPropertyResolver) XdsClusterName() (string, bool) {
	return m.xdsClusterName, m.xdsClusterNameOK
}
func (m *mockPropertyResolver) XdsClusterMetadata() ([]byte, bool) {
	return m.xdsClusterMetadata, m.xdsClusterMetadataOK
}
func (m *mockPropertyResolver) XdsRouteName() (string, bool) {
	return m.xdsRouteName, m.xdsRouteNameOK
}
func (m *mockPropertyResolver) XdsRouteMetadata() ([]byte, bool) {
	return m.xdsRouteMetadata, m.xdsRouteMetadataOK
}
func (m *mockPropertyResolver) XdsVirtualHostName() (string, bool) {
	return m.xdsVirtualHostName, m.xdsVirtualHostNameOK
}
func (m *mockPropertyResolver) XdsVirtualHostMetadata() ([]byte, bool) {
	return m.xdsVirtualHostMetadata, m.xdsVirtualHostMetadataOK
}
func (m *mockPropertyResolver) XdsUpstreamHostMetadata() ([]byte, bool) {
	return m.xdsUpstreamHostMetadata, m.xdsUpstreamHostMetadataOK
}
func (m *mockPropertyResolver) XdsUpstreamHostLocalityMetadata() ([]byte, bool) {
	return m.xdsUpstreamHostLocalityMetadata, m.xdsUpstreamHostLocalityMetadataOK
}
func (m *mockPropertyResolver) XdsFilterChainName() (string, bool) {
	return m.xdsFilterChainName, m.xdsFilterChainNameOK
}
func (m *mockPropertyResolver) XdsListenerMetadata() ([]byte, bool) {
	return m.xdsListenerMetadata, m.xdsListenerMetadataOK
}
func (m *mockPropertyResolver) XdsListenerDirection() (uint64, bool) {
	return m.xdsListenerDirection, m.xdsListenerDirectionOK
}
func (m *mockPropertyResolver) XdsNode() ([]byte, bool) { return m.xdsNode, m.xdsNodeOK }

func (m *mockPropertyResolver) Metadata(filterName, key string) ([]byte, bool) {
	if m.metadata == nil {
		return nil, false
	}
	v, ok := m.metadata[filterName+"\x1f"+key]
	return v, ok
}

func (m *mockPropertyResolver) FilterState() *filterstate.Bucket { return m.filterStateBucket }
func (m *mockPropertyResolver) UpstreamFilterState() *filterstate.Bucket {
	return m.upstreamFilterStateBucket
}

func (m *mockPropertyResolver) PluginName() (string, bool) { return m.pluginName, m.pluginName != "" }
func (m *mockPropertyResolver) PluginRootID() (string, bool) {
	return m.pluginRootID, m.pluginRootID != ""
}
func (m *mockPropertyResolver) PluginVMID() (string, bool) { return m.pluginVMID, m.pluginVMID != "" }

// Compile-time: *mockPropertyResolver satisfies PropertyResolver.
var _ PropertyResolver = (*mockPropertyResolver)(nil)

// stringValueBucketEntry is a minimal FilterStateObject for filter_state.*
// integration tests (returns the stored bytes verbatim on Marshal).
type stringValueBucketEntry struct {
	data []byte
}

func (e *stringValueBucketEntry) Marshal() ([]byte, error) {
	out := make([]byte, len(e.data))
	copy(out, e.data)
	return out, nil
}

func (e *stringValueBucketEntry) Unmarshal(b []byte) error {
	e.data = make([]byte, len(b))
	copy(e.data, b)
	return nil
}

func (e *stringValueBucketEntry) HasData() bool { return len(e.data) > 0 }
func (e *stringValueBucketEntry) StateType() filterstate.StateType {
	return filterstate.StateTypeMutable
}

// encodeU64LE encodes the value as 8-byte little-endian per spec README
// §Serialization.
func encodeU64LE(v uint64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, v)
	return b
}

// encodeBool encodes a bool as a single byte (0 or 1) per spec README.
func encodeBool(v bool) []byte {
	if v {
		return []byte{0x01}
	}
	return []byte{0x00}
}

// -----------------------------------------------------------------------------
// parsePathSegments — NUL-delimited path parsing per §11.4 + cpp-host
// context.cc:1047-1058 host-parsing.
// -----------------------------------------------------------------------------

func TestPropertyParsePathSegments(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []string
	}{
		{"empty path", []byte(""), nil},
		{"nil path", nil, nil},
		{"single segment direct token", []byte("plugin_name"), []string{"plugin_name"}},
		{"two segments", []byte("request\x00method"), []string{"request", "method"}},
		{"three segments header byname", []byte("request\x00headers\x00user-agent"), []string{"request", "headers", "user-agent"}},
		{"trailing NUL tolerated", []byte("request\x00method\x00"), []string{"request", "method"}},
		{"double NUL empty segment", []byte("\x00\x00"), nil},
		{"leading NUL empty first segment", []byte("\x00method"), nil},
		{"empty middle segment", []byte("request\x00\x00method"), nil},
		{"filter_state dotted key", []byte("filter_state\x00envoy.lua"), []string{"filter_state", "envoy.lua"}},
		{"metadata 3-tuple", []byte("metadata\x00envoy.foo\x00bar"), []string{"metadata", "envoy.foo", "bar"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePathSegments(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("parsePathSegments(%q) len=%d want=%d (got=%v)", tc.in, len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("parsePathSegments(%q)[%d]=%q want=%q", tc.in, i, got[i], tc.want[i])
				}
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ResolveProperty entry — empty / unknown root / unknown sub-path.
// -----------------------------------------------------------------------------

func TestPropertyResolve_EmptyPath_NotFound(t *testing.T) {
	r := &mockPropertyResolver{}
	val, status := ResolveProperty(r, []byte(""))
	if val != nil {
		t.Fatalf("empty path: got value=%q want nil", val)
	}
	if status != abi.WasmResultNotFound {
		t.Fatalf("empty path: got status=%v want NotFound", status)
	}
}

func TestPropertyResolve_UnknownRoot_NotFound(t *testing.T) {
	r := &mockPropertyResolver{}
	val, status := ResolveProperty(r, []byte("unknown_root\x00sub"))
	if val != nil {
		t.Fatalf("unknown root: got value=%q want nil", val)
	}
	if status != abi.WasmResultNotFound {
		t.Fatalf("unknown root: got status=%v want NotFound", status)
	}
}

func TestPropertyResolve_UnknownSubPath_NotFound(t *testing.T) {
	r := &mockPropertyResolver{
		reqMethod:   "GET",
		reqMethodOK: true,
	}
	val, status := ResolveProperty(r, []byte("request\x00unknown_subpath"))
	if val != nil {
		t.Fatalf("unknown sub-path: got value=%q want nil", val)
	}
	if status != abi.WasmResultNotFound {
		t.Fatalf("unknown sub-path: got status=%v want NotFound", status)
	}
}

func TestPropertyResolve_DoubleNUL_NotFound(t *testing.T) {
	r := &mockPropertyResolver{}
	val, status := ResolveProperty(r, []byte("\x00\x00"))
	if val != nil {
		t.Fatalf("double NUL: got value=%q want nil", val)
	}
	if status != abi.WasmResultNotFound {
		t.Fatalf("double NUL: got status=%v want NotFound", status)
	}
}

func TestPropertyResolve_AbsentSubPath_NotFound(t *testing.T) {
	// Resolver has no canned values — every sub-path returns NotFound.
	r := &mockPropertyResolver{}
	probes := [][]byte{
		[]byte("request\x00method"),
		[]byte("response\x00code"),
		[]byte("connection\x00id"),
		[]byte("source\x00address"),
		[]byte("destination\x00port"),
		[]byte("upstream\x00address"),
		[]byte("xds\x00cluster_name"),
		[]byte("metadata\x00envoy.foo\x00bar"),
		[]byte("filter_state\x00key"),
		[]byte("upstream_filter_state\x00key"),
		[]byte("wasm\x00key"),
		[]byte("plugin_name"),
		[]byte("plugin_root_id"),
		[]byte("plugin_vm_id"),
		[]byte("connection_id"),
	}
	for _, p := range probes {
		val, status := ResolveProperty(r, p)
		if val != nil {
			t.Errorf("path=%q: got value=%q want nil", p, val)
		}
		if status != abi.WasmResultNotFound {
			t.Errorf("path=%q: got status=%v want NotFound", p, status)
		}
	}
}

// -----------------------------------------------------------------------------
// Table-driven ~70 sub-paths per AMEND-B4. Each row exercises one sub-path
// against a resolver that has been populated with the canned value for that
// sub-path only. Absent peers MUST still return NotFound (see
// TestResolveProperty_AbsentSubPath_NotFound).
// -----------------------------------------------------------------------------

type propertyRow struct {
	name       string
	path       []byte
	setup      func(*mockPropertyResolver)
	wantBytes  []byte
	wantStatus abi.WasmResult
}

func TestPropertyResolve_FullRoster(t *testing.T) {
	// Populate a fully-loaded resolver for filter_state / metadata round-trips.
	fsBucket := filterstate.NewBucket()
	if err := fsBucket.Set("envoy.lua", &stringValueBucketEntry{data: []byte("hello-lua")}); err != nil {
		t.Fatalf("filter_state.Set: %v", err)
	}
	upFSBucket := filterstate.NewBucket()
	if err := upFSBucket.Set("envoy.upstream", &stringValueBucketEntry{data: []byte("up-val")}); err != nil {
		t.Fatalf("upstream_filter_state.Set: %v", err)
	}
	// wasm.<key> proxies via filter_state then upstream_filter_state. Test
	// covers BOTH branches: downstream-hit (key on filter_state) and
	// fallthrough-to-upstream (key only on upstream_filter_state).
	wasmBucketDownstream := filterstate.NewBucket()
	if err := wasmBucketDownstream.Set("wasm_downstream_only", &stringValueBucketEntry{data: []byte("ds-val")}); err != nil {
		t.Fatalf("wasm downstream Set: %v", err)
	}
	wasmBucketUpstream := filterstate.NewBucket()
	if err := wasmBucketUpstream.Set("wasm_upstream_only", &stringValueBucketEntry{data: []byte("us-val")}); err != nil {
		t.Fatalf("wasm upstream Set: %v", err)
	}

	rows := []propertyRow{
		// --- request (16 sub-paths) ---
		{"request.path", []byte("request\x00path"), func(r *mockPropertyResolver) { r.reqPath = "/api/v1"; r.reqPathOK = true }, []byte("/api/v1"), abi.WasmResultOk},
		{"request.url_path", []byte("request\x00url_path"), func(r *mockPropertyResolver) { r.reqURLPath = "/api/v1?q=1"; r.reqURLPathOK = true }, []byte("/api/v1?q=1"), abi.WasmResultOk},
		{"request.host", []byte("request\x00host"), func(r *mockPropertyResolver) { r.reqHost = "example.com"; r.reqHostOK = true }, []byte("example.com"), abi.WasmResultOk},
		{"request.scheme", []byte("request\x00scheme"), func(r *mockPropertyResolver) { r.reqScheme = "https"; r.reqSchemeOK = true }, []byte("https"), abi.WasmResultOk},
		{"request.method", []byte("request\x00method"), func(r *mockPropertyResolver) { r.reqMethod = "GET"; r.reqMethodOK = true }, []byte("GET"), abi.WasmResultOk},
		{"request.referer", []byte("request\x00referer"), func(r *mockPropertyResolver) { r.reqReferer = "https://ref"; r.reqRefererOK = true }, []byte("https://ref"), abi.WasmResultOk},
		{"request.useragent", []byte("request\x00useragent"), func(r *mockPropertyResolver) { r.reqUserAgent = "ua/1.0"; r.reqUserAgentOK = true }, []byte("ua/1.0"), abi.WasmResultOk},
		{"request.id", []byte("request\x00id"), func(r *mockPropertyResolver) { r.reqID = "req-abc"; r.reqIDOK = true }, []byte("req-abc"), abi.WasmResultOk},
		{"request.protocol", []byte("request\x00protocol"), func(r *mockPropertyResolver) { r.reqProtocol = "HTTP/2"; r.reqProtocolOK = true }, []byte("HTTP/2"), abi.WasmResultOk},
		{"request.query", []byte("request\x00query"), func(r *mockPropertyResolver) { r.reqQuery = "q=1&r=2"; r.reqQueryOK = true }, []byte("q=1&r=2"), abi.WasmResultOk},
		{"request.headers.byname", []byte("request\x00headers\x00x-foo"), func(r *mockPropertyResolver) { r.reqHeaders = map[string]string{"x-foo": "bar"} }, []byte("bar"), abi.WasmResultOk},
		{"request.headers_bytes", []byte("request\x00headers_bytes"), func(r *mockPropertyResolver) { r.reqHeadersBytes = 1234; r.reqHeadersBytesOK = true }, encodeU64LE(1234), abi.WasmResultOk},
		{"request.time", []byte("request\x00time"), func(r *mockPropertyResolver) { r.reqTime = 1700000000000000000; r.reqTimeOK = true }, encodeU64LE(1700000000000000000), abi.WasmResultOk},
		{"request.size", []byte("request\x00size"), func(r *mockPropertyResolver) { r.reqSize = 42; r.reqSizeOK = true }, encodeU64LE(42), abi.WasmResultOk},
		{"request.total_size", []byte("request\x00total_size"), func(r *mockPropertyResolver) { r.reqTotalSize = 4096; r.reqTotalSizeOK = true }, encodeU64LE(4096), abi.WasmResultOk},
		{"request.duration", []byte("request\x00duration"), func(r *mockPropertyResolver) { r.reqDuration = 250; r.reqDurationOK = true }, encodeU64LE(250), abi.WasmResultOk},

		// --- response (6 sub-paths) ---
		{"response.code", []byte("response\x00code"), func(r *mockPropertyResolver) { r.respCode = 200; r.respCodeOK = true }, encodeU64LE(200), abi.WasmResultOk},
		{"response.code_details", []byte("response\x00code_details"), func(r *mockPropertyResolver) { r.respCodeDetails = "via_upstream"; r.respCodeDetailsOK = true }, []byte("via_upstream"), abi.WasmResultOk},
		{"response.flags", []byte("response\x00flags"), func(r *mockPropertyResolver) { r.respFlags = 0x8; r.respFlagsOK = true }, encodeU64LE(0x8), abi.WasmResultOk},
		{"response.grpc_status", []byte("response\x00grpc_status"), func(r *mockPropertyResolver) { r.respGrpcStatus = 0; r.respGrpcStatusOK = true }, encodeU64LE(0), abi.WasmResultOk},
		{"response.backend_latency", []byte("response\x00backend_latency"), func(r *mockPropertyResolver) { r.respBackendLatency = 1500; r.respBackendLatencyOK = true }, encodeU64LE(1500), abi.WasmResultOk},
		{"response.trailers.byname", []byte("response\x00trailers\x00grpc-status"), func(r *mockPropertyResolver) { r.respTrailers = map[string]string{"grpc-status": "0"} }, []byte("0"), abi.WasmResultOk},

		// --- connection (12 sub-paths + id) ---
		{"connection.id", []byte("connection\x00id"), func(r *mockPropertyResolver) { r.connID = 42; r.connIDOK = true }, encodeU64LE(42), abi.WasmResultOk},
		{"connection.mtls.true", []byte("connection\x00mtls"), func(r *mockPropertyResolver) { r.connMTLS = true; r.connMTLSOK = true }, encodeBool(true), abi.WasmResultOk},
		{"connection.mtls.false", []byte("connection\x00mtls"), func(r *mockPropertyResolver) { r.connMTLS = false; r.connMTLSOK = true }, encodeBool(false), abi.WasmResultOk},
		{"connection.requested_server_name", []byte("connection\x00requested_server_name"), func(r *mockPropertyResolver) {
			r.connRequestedServerName = "sni.example"
			r.connRequestedServerNameOK = true
		}, []byte("sni.example"), abi.WasmResultOk},
		{"connection.tls_version", []byte("connection\x00tls_version"), func(r *mockPropertyResolver) { r.connTLSVersion = "TLSv1.3"; r.connTLSVersionOK = true }, []byte("TLSv1.3"), abi.WasmResultOk},
		{"connection.termination_details", []byte("connection\x00termination_details"), func(r *mockPropertyResolver) { r.connTerminationDetails = "ok"; r.connTerminationDetailsOK = true }, []byte("ok"), abi.WasmResultOk},
		{"connection.subject_local_certificate", []byte("connection\x00subject_local_certificate"), func(r *mockPropertyResolver) { r.connSubjectLocalCert = "CN=server"; r.connSubjectLocalCertOK = true }, []byte("CN=server"), abi.WasmResultOk},
		{"connection.subject_peer_certificate", []byte("connection\x00subject_peer_certificate"), func(r *mockPropertyResolver) { r.connSubjectPeerCert = "CN=client"; r.connSubjectPeerCertOK = true }, []byte("CN=client"), abi.WasmResultOk},
		{"connection.uri_san_local_certificate", []byte("connection\x00uri_san_local_certificate"), func(r *mockPropertyResolver) { r.connURISANLocalCert = "spiffe://srv"; r.connURISANLocalCertOK = true }, []byte("spiffe://srv"), abi.WasmResultOk},
		{"connection.uri_san_peer_certificate", []byte("connection\x00uri_san_peer_certificate"), func(r *mockPropertyResolver) { r.connURISANPeerCert = "spiffe://cli"; r.connURISANPeerCertOK = true }, []byte("spiffe://cli"), abi.WasmResultOk},
		{"connection.dns_san_local_certificate", []byte("connection\x00dns_san_local_certificate"), func(r *mockPropertyResolver) { r.connDNSSANLocalCert = "srv.example"; r.connDNSSANLocalCertOK = true }, []byte("srv.example"), abi.WasmResultOk},
		{"connection.dns_san_peer_certificate", []byte("connection\x00dns_san_peer_certificate"), func(r *mockPropertyResolver) { r.connDNSSANPeerCert = "cli.example"; r.connDNSSANPeerCertOK = true }, []byte("cli.example"), abi.WasmResultOk},
		{"connection.sha256_peer_certificate_digest", []byte("connection\x00sha256_peer_certificate_digest"), func(r *mockPropertyResolver) {
			r.connSHA256PeerCertDigest = "abcd1234"
			r.connSHA256PeerCertDigestOK = true
		}, []byte("abcd1234"), abi.WasmResultOk},
		{"connection.transport_failure_reason", []byte("connection\x00transport_failure_reason"), func(r *mockPropertyResolver) {
			r.connTransportFailureReason = "ssl_error"
			r.connTransportFailureReasonOK = true
		}, []byte("ssl_error"), abi.WasmResultOk},

		// --- source (2 sub-paths) ---
		{"source.address", []byte("source\x00address"), func(r *mockPropertyResolver) { r.srcAddress = "10.0.0.1"; r.srcAddressOK = true }, []byte("10.0.0.1"), abi.WasmResultOk},
		{"source.port", []byte("source\x00port"), func(r *mockPropertyResolver) { r.srcPort = 54321; r.srcPortOK = true }, encodeU64LE(54321), abi.WasmResultOk},

		// --- destination (2 sub-paths) ---
		{"destination.address", []byte("destination\x00address"), func(r *mockPropertyResolver) { r.dstAddress = "10.0.0.2"; r.dstAddressOK = true }, []byte("10.0.0.2"), abi.WasmResultOk},
		{"destination.port", []byte("destination\x00port"), func(r *mockPropertyResolver) { r.dstPort = 8443; r.dstPortOK = true }, encodeU64LE(8443), abi.WasmResultOk},

		// --- upstream (14 sub-paths) ---
		{"upstream.address", []byte("upstream\x00address"), func(r *mockPropertyResolver) { r.upAddress = "10.0.1.1"; r.upAddressOK = true }, []byte("10.0.1.1"), abi.WasmResultOk},
		{"upstream.port", []byte("upstream\x00port"), func(r *mockPropertyResolver) { r.upPort = 9000; r.upPortOK = true }, encodeU64LE(9000), abi.WasmResultOk},
		{"upstream.local_address", []byte("upstream\x00local_address"), func(r *mockPropertyResolver) { r.upLocalAddress = "10.0.0.99"; r.upLocalAddressOK = true }, []byte("10.0.0.99"), abi.WasmResultOk},
		{"upstream.locality", []byte("upstream\x00locality"), func(r *mockPropertyResolver) { r.upLocality = "us-east-2a"; r.upLocalityOK = true }, []byte("us-east-2a"), abi.WasmResultOk},
		{"upstream.transport_failure_reason", []byte("upstream\x00transport_failure_reason"), func(r *mockPropertyResolver) {
			r.upTransportFailureReason = "conn_reset"
			r.upTransportFailureReasonOK = true
		}, []byte("conn_reset"), abi.WasmResultOk},
		{"upstream.request_attempt_count", []byte("upstream\x00request_attempt_count"), func(r *mockPropertyResolver) { r.upRequestAttemptCount = 3; r.upRequestAttemptCountOK = true }, encodeU64LE(3), abi.WasmResultOk},
		{"upstream.cx_pool_ready_duration", []byte("upstream\x00cx_pool_ready_duration"), func(r *mockPropertyResolver) { r.upCxPoolReadyDuration = 12; r.upCxPoolReadyDurationOK = true }, encodeU64LE(12), abi.WasmResultOk},
		{"upstream.num_endpoints", []byte("upstream\x00num_endpoints"), func(r *mockPropertyResolver) { r.upNumEndpoints = 5; r.upNumEndpointsOK = true }, encodeU64LE(5), abi.WasmResultOk},
		{"upstream.subject_local_certificate", []byte("upstream\x00subject_local_certificate"), func(r *mockPropertyResolver) { r.upSubjectLocalCert = "CN=us-srv"; r.upSubjectLocalCertOK = true }, []byte("CN=us-srv"), abi.WasmResultOk},
		{"upstream.subject_peer_certificate", []byte("upstream\x00subject_peer_certificate"), func(r *mockPropertyResolver) { r.upSubjectPeerCert = "CN=us-peer"; r.upSubjectPeerCertOK = true }, []byte("CN=us-peer"), abi.WasmResultOk},
		{"upstream.uri_san_local_certificate", []byte("upstream\x00uri_san_local_certificate"), func(r *mockPropertyResolver) { r.upURISANLocalCert = "spiffe://us-srv"; r.upURISANLocalCertOK = true }, []byte("spiffe://us-srv"), abi.WasmResultOk},
		{"upstream.uri_san_peer_certificate", []byte("upstream\x00uri_san_peer_certificate"), func(r *mockPropertyResolver) { r.upURISANPeerCert = "spiffe://us-peer"; r.upURISANPeerCertOK = true }, []byte("spiffe://us-peer"), abi.WasmResultOk},
		{"upstream.dns_san_peer_certificate", []byte("upstream\x00dns_san_peer_certificate"), func(r *mockPropertyResolver) { r.upDNSSANPeerCert = "us-peer.example"; r.upDNSSANPeerCertOK = true }, []byte("us-peer.example"), abi.WasmResultOk},
		{"upstream.tls_version", []byte("upstream\x00tls_version"), func(r *mockPropertyResolver) { r.upTLSVersion = "TLSv1.2"; r.upTLSVersionOK = true }, []byte("TLSv1.2"), abi.WasmResultOk},

		// --- xds (12 sub-paths consolidating listener+route+cluster per AMEND-B4) ---
		{"xds.cluster_name", []byte("xds\x00cluster_name"), func(r *mockPropertyResolver) { r.xdsClusterName = "edge-cluster"; r.xdsClusterNameOK = true }, []byte("edge-cluster"), abi.WasmResultOk},
		{"xds.cluster_metadata", []byte("xds\x00cluster_metadata"), func(r *mockPropertyResolver) {
			r.xdsClusterMetadata = []byte("\x01\x02\x03")
			r.xdsClusterMetadataOK = true
		}, []byte("\x01\x02\x03"), abi.WasmResultOk},
		{"xds.route_name", []byte("xds\x00route_name"), func(r *mockPropertyResolver) { r.xdsRouteName = "main-route"; r.xdsRouteNameOK = true }, []byte("main-route"), abi.WasmResultOk},
		{"xds.route_metadata", []byte("xds\x00route_metadata"), func(r *mockPropertyResolver) { r.xdsRouteMetadata = []byte("rmeta"); r.xdsRouteMetadataOK = true }, []byte("rmeta"), abi.WasmResultOk},
		{"xds.virtual_host_name", []byte("xds\x00virtual_host_name"), func(r *mockPropertyResolver) { r.xdsVirtualHostName = "vh1"; r.xdsVirtualHostNameOK = true }, []byte("vh1"), abi.WasmResultOk},
		{"xds.virtual_host_metadata", []byte("xds\x00virtual_host_metadata"), func(r *mockPropertyResolver) {
			r.xdsVirtualHostMetadata = []byte("vhmeta")
			r.xdsVirtualHostMetadataOK = true
		}, []byte("vhmeta"), abi.WasmResultOk},
		{"xds.upstream_host_metadata", []byte("xds\x00upstream_host_metadata"), func(r *mockPropertyResolver) {
			r.xdsUpstreamHostMetadata = []byte("uhmeta")
			r.xdsUpstreamHostMetadataOK = true
		}, []byte("uhmeta"), abi.WasmResultOk},
		{"xds.upstream_host_locality_metadata", []byte("xds\x00upstream_host_locality_metadata"), func(r *mockPropertyResolver) {
			r.xdsUpstreamHostLocalityMetadata = []byte("uhlmeta")
			r.xdsUpstreamHostLocalityMetadataOK = true
		}, []byte("uhlmeta"), abi.WasmResultOk},
		{"xds.filter_chain_name", []byte("xds\x00filter_chain_name"), func(r *mockPropertyResolver) { r.xdsFilterChainName = "fc1"; r.xdsFilterChainNameOK = true }, []byte("fc1"), abi.WasmResultOk},
		{"xds.listener_metadata", []byte("xds\x00listener_metadata"), func(r *mockPropertyResolver) { r.xdsListenerMetadata = []byte("lmeta"); r.xdsListenerMetadataOK = true }, []byte("lmeta"), abi.WasmResultOk},
		{"xds.listener_direction", []byte("xds\x00listener_direction"), func(r *mockPropertyResolver) { r.xdsListenerDirection = 1; r.xdsListenerDirectionOK = true }, encodeU64LE(1), abi.WasmResultOk},
		{"xds.node", []byte("xds\x00node"), func(r *mockPropertyResolver) { r.xdsNode = []byte("nodemeta"); r.xdsNodeOK = true }, []byte("nodemeta"), abi.WasmResultOk},

		// --- metadata.<filter>.<key> via ADR-0190 ---
		{"metadata.envoy.foo.bar", []byte("metadata\x00envoy.foo\x00bar"), func(r *mockPropertyResolver) {
			r.metadata = map[string][]byte{"envoy.foo\x1fbar": []byte("md-val")}
		}, []byte("md-val"), abi.WasmResultOk},

		// --- filter_state (via ADR-0207 Bucket) — co-consumed primitive integration ---
		{"filter_state.envoy.lua", []byte("filter_state\x00envoy.lua"), func(r *mockPropertyResolver) {
			r.filterStateBucket = fsBucket
		}, []byte("hello-lua"), abi.WasmResultOk},

		// --- upstream_filter_state (DISTINCT root per AMEND-B4) ---
		{"upstream_filter_state.envoy.upstream", []byte("upstream_filter_state\x00envoy.upstream"), func(r *mockPropertyResolver) {
			r.upstreamFilterStateBucket = upFSBucket
		}, []byte("up-val"), abi.WasmResultOk},

		// --- wasm.<key> proxy class: downstream-hit + fallthrough-to-upstream ---
		{"wasm.<key>.downstream_hit", []byte("wasm\x00wasm_downstream_only"), func(r *mockPropertyResolver) {
			r.filterStateBucket = wasmBucketDownstream
		}, []byte("ds-val"), abi.WasmResultOk},
		{"wasm.<key>.fallthrough_upstream", []byte("wasm\x00wasm_upstream_only"), func(r *mockPropertyResolver) {
			r.upstreamFilterStateBucket = wasmBucketUpstream
		}, []byte("us-val"), abi.WasmResultOk},

		// --- 4 direct tokens ---
		{"plugin_name", []byte("plugin_name"), func(r *mockPropertyResolver) { r.pluginName = "my-plugin" }, []byte("my-plugin"), abi.WasmResultOk},
		{"plugin_root_id", []byte("plugin_root_id"), func(r *mockPropertyResolver) { r.pluginRootID = "root-1" }, []byte("root-1"), abi.WasmResultOk},
		{"plugin_vm_id", []byte("plugin_vm_id"), func(r *mockPropertyResolver) { r.pluginVMID = "vm-xyz" }, []byte("vm-xyz"), abi.WasmResultOk},
		{"connection_id direct token", []byte("connection_id"), func(r *mockPropertyResolver) { r.connID = 99; r.connIDOK = true }, encodeU64LE(99), abi.WasmResultOk},
	}

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			r := &mockPropertyResolver{}
			tc.setup(r)
			val, status := ResolveProperty(r, tc.path)
			if status != tc.wantStatus {
				t.Fatalf("status: got=%v want=%v (val=%q)", status, tc.wantStatus, val)
			}
			if !bytes.Equal(val, tc.wantBytes) {
				t.Fatalf("value: got=%v want=%v", val, tc.wantBytes)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Co-consumed primitive integration round-trip — full filterstate.Bucket
// (Task 9 producer) populated + Get round-trip via filter_state.<key>.
// Verifies the wasm/filterstate seam end-to-end.
// -----------------------------------------------------------------------------

func TestPropertyResolve_FilterStateBucket_IntegrationRoundTrip(t *testing.T) {
	bucket := filterstate.NewBucket()
	payloads := map[string]string{
		"envoy.lua":       "lua-payload",
		"envoy.wasm.test": "wasm-payload",
		"envoy.empty":     "",
	}
	for k, v := range payloads {
		if err := bucket.Set(k, &stringValueBucketEntry{data: []byte(v)}); err != nil {
			t.Fatalf("bucket.Set(%q): %v", k, err)
		}
	}
	r := &mockPropertyResolver{filterStateBucket: bucket}

	for k, wantPayload := range payloads {
		path := []byte("filter_state\x00" + k)
		val, status := ResolveProperty(r, path)
		if status != abi.WasmResultOk {
			t.Errorf("path=%q: status=%v want=Ok", path, status)
			continue
		}
		if string(val) != wantPayload {
			t.Errorf("path=%q: value=%q want=%q", path, val, wantPayload)
		}
	}

	// Absent key → NotFound (byte-faithful to cpp-host).
	val, status := ResolveProperty(r, []byte("filter_state\x00not.there"))
	if status != abi.WasmResultNotFound {
		t.Errorf("absent key: status=%v want=NotFound", status)
	}
	if val != nil {
		t.Errorf("absent key: val=%v want=nil", val)
	}
}

// -----------------------------------------------------------------------------
// serializeValue typed-conversion table.
// -----------------------------------------------------------------------------

func TestPropertySerializeValue(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []byte
	}{
		{"string", "GET", []byte("GET")},
		{"empty string", "", []byte("")},
		{"bytes", []byte{0x01, 0x02, 0x03}, []byte{0x01, 0x02, 0x03}},
		{"nil bytes", []byte(nil), []byte{}},
		{"uint64 zero", uint64(0), encodeU64LE(0)},
		{"uint64 max", uint64(1<<63 + 7), encodeU64LE(1<<63 + 7)},
		{"bool true", true, []byte{0x01}},
		{"bool false", false, []byte{0x00}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := serializeValue(tc.in)
			if err != nil {
				t.Fatalf("serializeValue(%v): err=%v", tc.in, err)
			}
			if !bytes.Equal(got, tc.want) {
				t.Fatalf("serializeValue(%v): got=%v want=%v", tc.in, got, tc.want)
			}
		})
	}
}

func TestPropertySerializeValue_UnsupportedType_Error(t *testing.T) {
	got, err := serializeValue(struct{ X int }{X: 1})
	if err == nil {
		t.Fatalf("serializeValue(struct): got=%v want error", got)
	}
}
