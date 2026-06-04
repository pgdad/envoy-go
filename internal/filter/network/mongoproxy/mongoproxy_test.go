package mongoproxy

import "testing"

func TestTypeURL_PinnedToUpstreamExtensionsName(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.network.mongo_proxy.v3.MongoProxy"
	if TypeURL != want {
		t.Fatalf("TypeURL = %q, want %q", TypeURL, want)
	}
}
