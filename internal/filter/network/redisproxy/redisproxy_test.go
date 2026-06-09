package redisproxy

import "testing"

func TestTypeURL_PinnedToUpstreamExtensionsName(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.network.redis_proxy.v3.RedisProxy"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}
