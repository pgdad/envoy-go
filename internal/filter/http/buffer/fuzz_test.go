package buffer

import (
	"testing"

	bufferv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/buffer/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// FuzzBufferConfigParse fuzzes arbitrary byte sequences as the typed_config
// payload to New. Asserts: New returns either (factory, nil) OR (nil, error);
// never panics; never returns (nil, nil). Per ADR-0018 + SPEC §14.3.
//
// 17th fuzzer in the repo (post-12's 16th FuzzCsrfPolicyConfigParse).
func FuzzBufferConfigParse(f *testing.F) {
	// Seed corpus (well-formed + intentionally-rejected + malformed).
	for _, v := range []uint32{1, 1024, 1048576, 0, 5242880} {
		bytes, _ := proto.Marshal(&bufferv3.Buffer{MaxRequestBytes: wrapperspb.UInt32(v)})
		f.Add(bytes)
	}
	f.Add([]byte{})                    // empty bytes: Unmarshal failure
	f.Add([]byte{0xff})                // single garbage byte
	f.Add([]byte("not-a-valid-proto")) // printable-string garbage

	f.Fuzz(func(t *testing.T, raw []byte) {
		anyMsg := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		factory, err := New(anyMsg, envoyhttp.FactoryCtx{})
		if factory == nil && err == nil {
			t.Fatalf("New returned (nil, nil) — invariant violation")
		}
		if factory != nil && err != nil {
			t.Fatalf("New returned (factory, error) — invariant violation: %v", err)
		}
	})
}
