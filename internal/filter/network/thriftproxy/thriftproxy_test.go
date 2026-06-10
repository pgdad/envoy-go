package thriftproxy

import "testing"

func TestTypeURL(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.network.thrift_proxy.v3.ThriftProxy"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}
