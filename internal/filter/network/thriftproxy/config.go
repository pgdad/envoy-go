package thriftproxy

import (
	"errors"

	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"

	"github.com/pgdad/envoy-go/internal/stats"
)

// PARSE-REJECT arms (ADR-0080 byte-stable; SPEC §6). DO NOT CHANGE these strings.
// errStatPrefixRequired is the ONLY fixture-proven arm (the 0058 boot-reject,
// §6.2); the rest are unit-test-only. errUnsupportedTransport/Protocol are
// envoy-go-strict DEPARTURE rejects (the reference parse-accepts those enum
// values — AMEND-T7), NOT cross-side boot-rejects.
const (
	errStatPrefixRequired   = "thrift_proxy: stat_prefix is required"
	errStatPrefixInvalid    = "thrift_proxy: stat_prefix is not a valid metric name"
	errUnsupportedTransport = "thrift_proxy: unsupported transport (only AUTO_TRANSPORT and FRAMED are supported)"
	errUnsupportedProtocol  = "thrift_proxy: unsupported protocol (only AUTO_PROTOCOL and BINARY are supported)"
)

// compiledConfig is the boot-parsed, per-listener-shared thrift_proxy config
// (ADR-0079 two-step factory). The roster (stats.go) is NOT here — it attaches to
// the filter struct at NewFactory (filter.go).
type compiledConfig struct {
	statPrefix         string
	payloadPassthrough bool
	routes             *routeTable // Task 4; nil-tolerant → all-miss
}

// parseConfig validates + compiles the proto (SPEC §5/§6). Only stat_prefix is a
// hard PGV gate (+ IsValidName-guarded at the config boundary). transport/protocol
// accept {AUTO,FRAMED}×{AUTO,BINARY}; any other DEFINED enum value is an
// envoy-go-strict departure reject. route_config is NOT required (absent → all
// requests routing-miss → local exception). Deferred fields (thrift_filters,
// max_requests_per_connection, trds, access_log, header_keys_preserve_case)
// parse-accept standalone. Pure — no registry.
func parseConfig(msg *thrift_proxyv3.ThriftProxy) (*compiledConfig, error) {
	sp := msg.GetStatPrefix()
	if sp == "" {
		return nil, errors.New(errStatPrefixRequired)
	}
	if !stats.IsValidName(sp) {
		return nil, errors.New(errStatPrefixInvalid)
	}
	switch msg.GetTransport() {
	case thrift_proxyv3.TransportType_AUTO_TRANSPORT, thrift_proxyv3.TransportType_FRAMED:
	default:
		return nil, errors.New(errUnsupportedTransport)
	}
	switch msg.GetProtocol() {
	case thrift_proxyv3.ProtocolType_AUTO_PROTOCOL, thrift_proxyv3.ProtocolType_BINARY:
	default:
		return nil, errors.New(errUnsupportedProtocol)
	}
	rt, err := parseRouteConfig(msg.GetRouteConfig()) // Task 4
	if err != nil {
		return nil, err
	}
	return &compiledConfig{statPrefix: sp, payloadPassthrough: msg.GetPayloadPassthrough(), routes: rt}, nil
}
