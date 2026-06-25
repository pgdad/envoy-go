package accesslog

import (
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	dataaccesslogv3 "github.com/envoyproxy/go-control-plane/envoy/data/accesslog/v3"
)

func TestMappingProtocolVersionEnum(t *testing.T) {
	cases := []struct {
		proto string
		want  dataaccesslogv3.HTTPAccessLogEntry_HTTPVersion
	}{
		{"HTTP/1.1", dataaccesslogv3.HTTPAccessLogEntry_HTTP11},
		{"HTTP/2.0", dataaccesslogv3.HTTPAccessLogEntry_HTTP2},
		{"HTTP/1.0", dataaccesslogv3.HTTPAccessLogEntry_HTTP10},
		{"", dataaccesslogv3.HTTPAccessLogEntry_PROTOCOL_UNSPECIFIED},
		{"HTTP/3", dataaccesslogv3.HTTPAccessLogEntry_PROTOCOL_UNSPECIFIED},
		{"garbage", dataaccesslogv3.HTTPAccessLogEntry_PROTOCOL_UNSPECIFIED},
	}
	for _, c := range cases {
		if got := protocolVersionEnum(c.proto); got != c.want {
			t.Errorf("protocolVersionEnum(%q) = %v, want %v", c.proto, got, c.want)
		}
	}
}

func TestMappingRequestMethodEnum(t *testing.T) {
	cases := []struct {
		method string
		want   corev3.RequestMethod
	}{
		{"GET", corev3.RequestMethod_GET},
		{"POST", corev3.RequestMethod_POST},
		{"get", corev3.RequestMethod_GET}, // case-insensitive
		{"", corev3.RequestMethod_METHOD_UNSPECIFIED},
		{"garbage", corev3.RequestMethod_METHOD_UNSPECIFIED},
	}
	for _, c := range cases {
		if got := requestMethodEnum(c.method); got != c.want {
			t.Errorf("requestMethodEnum(%q) = %v, want %v", c.method, got, c.want)
		}
	}
}

func TestMappingBuildHTTPAccessLogEntry(t *testing.T) {
	rec := &Record{
		StartTime:    time.Unix(1700000000, 0),
		Method:       "GET",
		Path:         "/foo",
		Protocol:     "HTTP/1.1",
		ResponseCode: 200,
		BytesSent:    13,
		Duration:     5 * time.Millisecond,
		Authority:    "example.com",
		UserAgent:    "agent/1",
		UpstreamHost: "10.0.0.1:8080",
	}
	e := buildHTTPAccessLogEntry(rec)

	// Deterministic fields.
	if got := e.GetRequest().GetRequestMethod(); got != corev3.RequestMethod_GET {
		t.Errorf("RequestMethod = %v, want GET", got)
	}
	if got := e.GetRequest().GetPath(); got != "/foo" {
		t.Errorf("Path = %q, want /foo", got)
	}
	if got := e.GetRequest().GetAuthority(); got != "example.com" {
		t.Errorf("Authority = %q, want example.com", got)
	}
	if got := e.GetRequest().GetUserAgent(); got != "agent/1" {
		t.Errorf("UserAgent = %q, want agent/1", got)
	}
	if got := e.GetResponse().GetResponseCode().GetValue(); got != 200 {
		t.Errorf("ResponseCode = %d, want 200", got)
	}
	if got := e.GetResponse().GetResponseBodyBytes(); got != 13 {
		t.Errorf("ResponseBodyBytes = %d, want 13", got)
	}
	if got := e.GetProtocolVersion(); got != dataaccesslogv3.HTTPAccessLogEntry_HTTP11 {
		t.Errorf("ProtocolVersion = %v, want HTTP11", got)
	}

	// Non-deterministic fields: populated but values unasserted.
	if e.GetCommonProperties().GetStartTime() == nil {
		t.Error("StartTime is nil, want populated")
	}
	if e.GetCommonProperties().GetDuration() == nil {
		t.Error("Duration is nil, want populated")
	}
	sock := e.GetCommonProperties().GetUpstreamRemoteAddress().GetSocketAddress()
	if sock.GetAddress() != "10.0.0.1" {
		t.Errorf("UpstreamRemoteAddress.Address = %q, want 10.0.0.1", sock.GetAddress())
	}
	if sock.GetPortValue() != 8080 {
		t.Errorf("UpstreamRemoteAddress.PortValue = %d, want 8080", sock.GetPortValue())
	}
}

func TestMappingSocketAddressErrorBranches(t *testing.T) {
	cases := []struct {
		name     string
		hostPort string
	}{
		{"empty", ""},
		{"no colon", "nohost"},
		{"non-numeric port", "host:bad"},
		{"out-of-range port", "host:4294967296"}, // 2^32, overflows ParseUint bitSize=32
	}
	for _, c := range cases {
		if got := socketAddress(c.hostPort); got != nil {
			t.Errorf("socketAddress(%q) = %v, want nil", c.hostPort, got)
		}
	}
}

func TestMappingBuildEmptyUpstreamHost(t *testing.T) {
	rec := &Record{
		StartTime:    time.Unix(1700000000, 0),
		Method:       "GET",
		Path:         "/foo",
		Protocol:     "HTTP/1.1",
		ResponseCode: 200,
		UpstreamHost: "",
	}
	e := buildHTTPAccessLogEntry(rec)
	if addr := e.GetCommonProperties().GetUpstreamRemoteAddress(); addr != nil {
		t.Errorf("UpstreamRemoteAddress = %v, want nil for empty UpstreamHost", addr)
	}
}
