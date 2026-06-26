package accesslog

import (
	"net"
	"strconv"
	"strings"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	dataaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// protocolVersionEnum maps a Record.Protocol string to the structured gRPC ALS
// HTTPVersion enum. The codebase emits "HTTP/1.1" and "HTTP/2.0" (see
// internal/filter/hcm/accesslog_emit.go); "HTTP/1.0" is mapped for completeness.
// Anything else (including HTTP/3, which is not plumbed at 44.1) falls through to
// PROTOCOL_UNSPECIFIED.
func protocolVersionEnum(proto string) dataaccesslogv3.HTTPAccessLogEntry_HTTPVersion {
	switch proto {
	case "HTTP/1.1":
		return dataaccesslogv3.HTTPAccessLogEntry_HTTP11
	case "HTTP/2.0":
		return dataaccesslogv3.HTTPAccessLogEntry_HTTP2
	case "HTTP/1.0":
		return dataaccesslogv3.HTTPAccessLogEntry_HTTP10
	default:
		return dataaccesslogv3.HTTPAccessLogEntry_PROTOCOL_UNSPECIFIED
	}
}

// requestMethodEnum maps an HTTP method string to the core RequestMethod enum
// (case-insensitive). An unknown method maps to METHOD_UNSPECIFIED.
func requestMethodEnum(m string) corev3.RequestMethod {
	if v, ok := corev3.RequestMethod_value[strings.ToUpper(m)]; ok {
		return corev3.RequestMethod(v)
	}
	return corev3.RequestMethod_METHOD_UNSPECIFIED
}

// buildHTTPAccessLogEntry maps the 10-field Record into the structured
// HTTPAccessLogEntry proto streamed by the gRPC ALS sink (AMEND-ALS-2/4). The
// three non-deterministic fields (start_time, duration, upstream_remote_address)
// are populated but left UNasserted cross-side. Path carries the path only
// (AMEND-ALS-2; the reference additionally carries the query string).
//
// reqHdrNames/respHdrNames are the SINK's own configured header names (a subset
// of the emit-hook UNION captured into rec.RequestHeaders/rec.ResponseHeaders).
// Each sink filters the captured union down to its own names so multi-sink
// fan-out stays per-sink correct (D-HDR-SINK-FILTER). Empty name lists OR nil
// Record maps leave the proto header maps unset — byte-identical to the
// 44.1/44.2 no-capture path.
func buildHTTPAccessLogEntry(rec *Record, reqHdrNames, respHdrNames []string) *dataaccesslogv3.HTTPAccessLogEntry {
	e := &dataaccesslogv3.HTTPAccessLogEntry{
		ProtocolVersion: protocolVersionEnum(rec.Protocol),
		Request: &dataaccesslogv3.HTTPRequestProperties{
			RequestMethod: requestMethodEnum(rec.Method),
			Path:          rec.Path,
			Authority:     rec.Authority,
			UserAgent:     rec.UserAgent,
		},
		Response: &dataaccesslogv3.HTTPResponseProperties{
			ResponseCode:      wrapperspb.UInt32(uint32(rec.ResponseCode)),
			ResponseBodyBytes: uint64(rec.BytesSent),
		},
		CommonProperties: &dataaccesslogv3.AccessLogCommon{
			StartTime: timestamppb.New(rec.StartTime),
			Duration:  durationpb.New(rec.Duration),
		},
	}
	if m := filterCaptured(rec.RequestHeaders, reqHdrNames); m != nil {
		e.Request.RequestHeaders = m
	}
	if m := filterCaptured(rec.ResponseHeaders, respHdrNames); m != nil {
		e.Response.ResponseHeaders = m
	}
	if addr := socketAddress(rec.UpstreamHost); addr != nil {
		e.CommonProperties.UpstreamRemoteAddress = addr
	}
	return e
}

// filterCaptured copies the sink's configured names out of the captured map (the
// emit-hook UNION) into a fresh map, omitting names the request/response did not
// carry. Returns nil when names is empty, the captured map is empty/nil, or
// nothing matched — so the proto map stays unset (byte-identical to the
// no-capture path).
func filterCaptured(captured map[string]string, names []string) map[string]string {
	if len(names) == 0 || len(captured) == 0 {
		return nil
	}
	out := make(map[string]string, len(names))
	for _, n := range names {
		if v, ok := captured[n]; ok {
			out[n] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// socketAddress parses a "host:port" string into a core Address. It returns nil
// for an empty or unparseable input (the caller leaves UpstreamRemoteAddress
// unset rather than synthesizing a zero address).
func socketAddress(hostPort string) *corev3.Address {
	if hostPort == "" {
		return nil
	}
	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		return nil
	}
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return nil
	}
	return &corev3.Address{Address: &corev3.Address_SocketAddress{SocketAddress: &corev3.SocketAddress{
		Address:       host,
		PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: uint32(port)},
	}}}
}
