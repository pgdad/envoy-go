package localratelimit

import (
	"testing"

	"google.golang.org/protobuf/types/known/anypb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
)

// FuzzLocalRateLimitConfigParse fuzzes arbitrary byte sequences as the tc
// *anypb.Any parameter to New. Asserts New returns either (factory, nil)
// OR (nil, error); never panics; never returns (nil, nil).
//
// Per ADR-0018's "every parser/codec/filter ships a fuzzer" + the
// local_ratelimit filter's New factory is a parser. 30s budget per ADR-0018
// short-mode CI policy. Fifteenth fuzzer overall (post phase-10's fourteenth
// FuzzHeaderMutationConfigParse).
func FuzzLocalRateLimitConfigParse(f *testing.F) {
	// Seed corpus: empty TypeURL + empty bytes (invalid Any → decode error or
	// rejection by New).
	f.Add("", []byte{})
	// Seed corpus: arbitrary bytes under the canonical type URL (decode error).
	f.Add(TypeURL, []byte{0xff, 0xff, 0xff})
	// Seed corpus: short proto-wire-format bytes.
	f.Add(TypeURL, []byte{0x08, 0x01})

	f.Fuzz(func(t *testing.T, typeURL string, value []byte) {
		tc := &anypb.Any{TypeUrl: typeURL, Value: value}
		factory, err := New(tc, envoyhttp.FactoryCtx{Stats: stats.NewRegistry()})
		switch {
		case factory == nil && err == nil:
			t.Errorf("New returned (nil, nil); type=%q", typeURL)
		case factory != nil && err != nil:
			t.Errorf("New returned (factory, err); want (factory, nil) or (nil, err); type=%q err=%v", typeURL, err)
		}
	})
}
