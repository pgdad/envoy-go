package thriftproxy

import (
	"testing"

	thrift_proxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/thrift_proxy/v3"
)

func TestParseConfig(t *testing.T) {
	tests := []struct {
		name    string
		msg     *thrift_proxyv3.ThriftProxy
		wantErr string // "" = expect success
	}{
		{"minimal-defaults-ok", &thrift_proxyv3.ThriftProxy{StatPrefix: "t"}, ""},
		{"framed-binary-ok", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Transport: thrift_proxyv3.TransportType_FRAMED, Protocol: thrift_proxyv3.ProtocolType_BINARY}, ""},
		{"passthrough-ok", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", PayloadPassthrough: true}, ""},
		{"stat-prefix-missing", &thrift_proxyv3.ThriftProxy{}, errStatPrefixRequired},
		{"stat-prefix-invalid", &thrift_proxyv3.ThriftProxy{StatPrefix: "bad name!"}, errStatPrefixInvalid},
		{"transport-unsupported", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Transport: thrift_proxyv3.TransportType_UNFRAMED}, errUnsupportedTransport},
		{"transport-header-unsupported", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Transport: thrift_proxyv3.TransportType_HEADER}, errUnsupportedTransport},
		{"protocol-unsupported", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Protocol: thrift_proxyv3.ProtocolType_COMPACT}, errUnsupportedProtocol},
		//nolint:staticcheck // ProtocolType_TWITTER is intentionally exercised to prove the departure-reject covers the deprecated enum value too.
		{"protocol-twitter-unsupported", &thrift_proxyv3.ThriftProxy{StatPrefix: "t", Protocol: thrift_proxyv3.ProtocolType_TWITTER}, errUnsupportedProtocol},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseConfig(tc.msg)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cfg.statPrefix == "" {
					t.Fatalf("statPrefix not set")
				}
				return
			}
			if err == nil || err.Error() != tc.wantErr {
				t.Fatalf("err = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
