package directresponse

import (
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	drv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/direct_response/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/filter/network"
)

// FuzzNetworkFilterConfigParse fuzzes arbitrary byte sequences as the
// typed_config payload to direct_response's New. Asserts: New returns either
// (factory, nil) OR (nil, error); never panics; never returns (nil, nil).
// Per ADR-0018 + SPEC §11 / R6.
//
// 35th fuzzer in the repo.
func FuzzNetworkFilterConfigParse(f *testing.F) {
	seed := func(ds *corev3.DataSource) {
		b, _ := proto.Marshal(&drv3.Config{Response: ds})
		f.Add(b)
	}
	seed(&corev3.DataSource{Specifier: &corev3.DataSource_InlineString{InlineString: "x"}})
	seed(&corev3.DataSource{Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte{1}}})
	seed(nil)           // absent response → reject
	f.Add([]byte{})     // empty
	f.Add([]byte{0xff}) // garbage
	f.Add([]byte("not-proto"))

	f.Fuzz(func(t *testing.T, raw []byte) {
		anyMsg := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		fif, err := New(anyMsg, network.FactoryCtx{})
		if fif == nil && err == nil {
			t.Fatalf("New returned (nil, nil)")
		}
		if fif != nil && err != nil {
			t.Fatalf("New returned (factory, error): %v", err)
		}
	})
}
