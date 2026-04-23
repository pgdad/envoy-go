package tcpproxy

import (
	"strings"
	"testing"

	tcpproxyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
)

// FuzzTcpProxyFilter feeds mutated bytes into NewFilter via an Any wrapper.
// The contract per SPEC §3 gate (d): no panic; every error begins with
// "tcpproxy: " (matches the package's error-prefix discipline).
//
// Note on Go fuzz framework: f.Add(...) seed types and counts must match the
// f.Fuzz(func(t, ...) {...}) parameter list. Here both use (string, []byte) —
// type_url and the inner Any.Value bytes — and the fuzz function reconstructs
// the *anypb.Any from those two scalars on each invocation.
//
// Seed corpus (3 entries per SPEC §4.1):
//  1. Well-formed TcpProxy referencing an extant cluster (canonical happy).
//  2. Wrong type_url (StringValue instead of TcpProxy).
//  3. Malformed proto bytes (random non-proto bytes wrapped in an Any with
//     the correct type_url).
func FuzzTcpProxyFilter(f *testing.F) {
	// Seed 1: well-formed.
	good := &tcpproxyv3.TcpProxy{
		StatPrefix:       "ingress_tcp",
		ClusterSpecifier: &tcpproxyv3.TcpProxy_Cluster{Cluster: "c_echo"},
	}
	goodBytes, err := proto.Marshal(good)
	if err != nil {
		f.Fatalf("seed marshal: %v", err)
	}
	f.Add(TypeURL, goodBytes)

	// Seed 2: wrong type_url.
	f.Add("type.googleapis.com/google.protobuf.StringValue", goodBytes)

	// Seed 3: malformed bytes (non-proto).
	f.Add(TypeURL, []byte{0xff, 0xfe, 0xfd, 0xfc, 0xfb, 0xfa, 0xf9, 0xf8})

	cm := mkClusterMgr(f, "c_echo", "127.0.0.1", 1) // port 1 — never dialed in fuzz; only NewFilter is exercised.

	f.Fuzz(func(t *testing.T, typeURL string, body []byte) {
		a := &anypb.Any{TypeUrl: typeURL, Value: body}
		_, err := NewFilter(a, cm)
		if err == nil {
			return // no error is also acceptable (the input parsed)
		}
		if !strings.HasPrefix(err.Error(), "tcpproxy: ") {
			t.Fatalf("error does not begin with %q: %v", "tcpproxy: ", err)
		}
	})
}
