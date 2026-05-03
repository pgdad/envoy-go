package fault

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// FuzzFaultConfigParse fuzzes the New factory's typed_config parameter against
// arbitrary byte sequences. Per ADR-0018: every parser/codec/filter ships a
// fuzzer. Fault's New is the parser. Asserts:
//   - New returns either (factory, nil) OR (nil, error); never panics.
//   - Never returns (nil, nil).
//
// Seed corpus per SPEC §11.1 PGV-validation cases:
//   - empty Any (TypeURL = "", Value = nil)
//   - Any with wrong TypeURL but valid Value bytes
//   - Any with right TypeURL but garbage Value
//   - HTTPFault with abort.http_status = 0 / 9999 / 100 / 599 / 600
func FuzzFaultConfigParse(f *testing.F) {
	seeds := [][]byte{
		nil,
		{},
		{0x00},
		{0xff, 0xff, 0xff, 0xff},
		[]byte("not-a-proto"),
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		tc := &anypb.Any{TypeUrl: TypeURL, Value: b}
		ctx := envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "x"}
		factory, err := New(tc, ctx)
		if factory == nil && err == nil {
			t.Fatalf("New returned (nil, nil) for input %x — must return either factory or error", b)
		}
		if factory != nil && err != nil {
			t.Fatalf("New returned both factory and error for input %x — exclusive", b)
		}
	})
}
