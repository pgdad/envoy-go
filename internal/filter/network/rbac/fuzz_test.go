package rbac

import (
	"testing"

	configrbacv3 "github.com/envoyproxy/go-control-plane/envoy/config/rbac/v3"
	networkrbacv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/rbac/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/esalaine/envoy-go/internal/filter/network"
	"github.com/esalaine/envoy-go/internal/stats"
)

// FuzzNetworkRBACConfigParse drives the rbac_network config-parse path with
// arbitrary typed_config bytes — it must never panic; parse errors are fine.
//
// Registry reuse across f.Fuzz iterations: a single NewFactory(stats.NewRegistry())
// is constructed outside the closure and reused on every iteration. This is safe
// because newFilterStats calls NewCounterIfAbsent, which is idempotent (duplicate
// stat_prefix across iterations returns the same counter pointer without panic).
//
// 36th fuzzer in the repo.
func FuzzNetworkRBACConfigParse(f *testing.F) {
	// seed marshals a networkrbacv3.RBAC to its raw proto bytes (the Value field
	// of the Any wrapper), mirroring the direct_response fuzzer's seed shape.
	seed := func(m *networkrbacv3.RBAC) {
		b, _ := proto.Marshal(m)
		f.Add(b)
	}

	// Seed 1: empty message → stat_prefix required PARSE-REJECT.
	seed(&networkrbacv3.RBAC{})
	// Seed 2: valid stat_prefix → successful parse (accept path, counters registered).
	seed(&networkrbacv3.RBAC{StatPrefix: "p"})
	// Seed 3: delay_deny set → PARSE-REJECT (AMEND-A9 unsupported surface).
	seed(&networkrbacv3.RBAC{StatPrefix: "p", DelayDeny: durationpb.New(0)})
	// Seed 4: HTTP-only arm (permission.header) → ProfileL4 PARSE-REJECT at compile.
	seed(&networkrbacv3.RBAC{
		StatPrefix: "p",
		Rules: &configrbacv3.RBAC{
			Action: configrbacv3.RBAC_ALLOW,
			Policies: map[string]*configrbacv3.Policy{
				"x": {
					Permissions: []*configrbacv3.Permission{{Rule: &configrbacv3.Permission_Header{}}},
					Principals:  []*configrbacv3.Principal{{Identifier: &configrbacv3.Principal_Any{Any: true}}},
				},
			},
		},
	})
	// Seed 5: empty bytes → unmarshal produces zero-value → stat_prefix required.
	f.Add([]byte{})
	// Seed 6: garbage bytes → unmarshal error.
	f.Add([]byte{0xff})

	// One factory (and one stats.Registry) reused for the lifetime of the fuzz
	// run. NewCounterIfAbsent is idempotent, so repeated stat_prefix values across
	// iterations do not cause duplicate-registration panics.
	factory := NewFactory(stats.NewRegistry())

	f.Fuzz(func(t *testing.T, body []byte) {
		tc := &anypb.Any{TypeUrl: TypeURL, Value: body}
		// Must not panic regardless of bytes; parse errors are fine.
		fif, err := factory(tc, network.FactoryCtx{})
		if fif == nil && err == nil {
			t.Fatalf("factory returned (nil, nil)")
		}
	})
}
