package bandwidthlimit

import (
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	bandwidthlimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/bandwidth_limit/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
)

// FuzzBandwidthLimitConfigParse fuzzes arbitrary byte sequences as the
// typed_config payload to New. Asserts: New returns either (factory, nil) OR
// (nil, error); never panics; never returns (nil, nil). Per ADR-0018's "every
// parser/codec/filter ships a fuzzer" + SPEC §14.3.
//
// 19th fuzzer in the repo (post phase-14's 18th FuzzCompressorConfigParse).
// Seed corpus: 6 valid-config seeds (default-everything; explicit fill_interval;
// explicit limit_kbps=1000; enable_mode=REQUEST_AND_RESPONSE; runtime_enabled
// silent-ignored; response_trailer_prefix silent-ignored) + 4 invalid-config
// seeds (empty bytes → empty BandwidthLimit → stat_prefix required;
// empty stat_prefix; limit_kbps=0 foot-gun; fill_interval=10ms below-min
// bounds rejection). The fuzz engine derives further inputs from these seeds
// at the 30s budget per ADR-0018 short-mode CI policy.
func FuzzBandwidthLimitConfigParse(f *testing.F) {
	// 6 valid-config seeds — each is a well-formed BandwidthLimit proto that
	// New must accept (factory!=nil, err==nil).
	validSeeds := []*bandwidthlimitv3.BandwidthLimit{
		// (a) default-everything: stat_prefix + limit_kbps=10; fill_interval
		// unset → defaults to 50ms per amendment 5 + §11.P5.
		{
			StatPrefix: "seed_a",
			LimitKbps:  wrapperspb.UInt64(10),
		},
		// (b) explicit fill_interval=20ms (lower-bound of [20ms, 1s] per
		// amendment 5).
		{
			StatPrefix:   "seed_b",
			LimitKbps:    wrapperspb.UInt64(10),
			FillInterval: durationpb.New(20 * time.Millisecond),
		},
		// (c) explicit limit_kbps=1000 (mid-range).
		{
			StatPrefix: "seed_c",
			LimitKbps:  wrapperspb.UInt64(1000),
		},
		// (d) enable_mode=REQUEST_AND_RESPONSE (both directions active).
		{
			StatPrefix: "seed_d",
			LimitKbps:  wrapperspb.UInt64(10),
			EnableMode: bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE,
		},
		// (e) runtime_enabled with default_value=false — silent-ignored field
		// per ADR-0040 + ADR-0136 §Decision (ii) + planner-time decision 7.
		{
			StatPrefix: "seed_e",
			LimitKbps:  wrapperspb.UInt64(10),
			RuntimeEnabled: &corev3.RuntimeFeatureFlag{
				DefaultValue: wrapperspb.Bool(false),
				RuntimeKey:   "bandwidth.fuzz.enabled",
			},
		},
		// (f) response_trailer_prefix set — silent-ignored field per ADR-0040
		// + ADR-0136 §Decision (ii) + planner-time decision 8 (couples to
		// enable_response_trailers; parsed but not stored).
		{
			StatPrefix:            "seed_f",
			LimitKbps:             wrapperspb.UInt64(10),
			ResponseTrailerPrefix: "bw-fuzz",
		},
	}
	for i, s := range validSeeds {
		raw, err := proto.Marshal(s)
		if err != nil {
			f.Fatalf("valid seed[%d] marshal: %v", i, err)
		}
		f.Add(raw)
	}

	// 4 invalid-config seeds — each yields (nil, err) without panic via a
	// distinct rejection path:
	//   (1) empty bytes → Unmarshal succeeds with empty BandwidthLimit →
	//       stat_prefix required (per ADR-0136 §Decision (iv) check 1).
	//   (2) explicit empty stat_prefix → same rejection.
	//   (3) limit_kbps=0 → foot-gun rejected per amendment 4 + ADR-0136
	//       §Decision (iv) check 3 ("limit_kbps must be >= 1"; the proto-
	//       wrapped uint64 distinguishes unset (nil) from set-to-zero).
	//   (4) fill_interval=10ms → below-min bounds rejection per amendment 5 +
	//       ADR-0136 §Decision (iv) check 4 ("outside supported range
	//       [20ms, 1s]").
	f.Add([]byte{}) // (1) empty bytes
	{
		raw, err := proto.Marshal(&bandwidthlimitv3.BandwidthLimit{
			StatPrefix: "",
			LimitKbps:  wrapperspb.UInt64(10),
		})
		if err != nil {
			f.Fatalf("invalid seed[2] marshal: %v", err)
		}
		f.Add(raw) // (2) empty stat_prefix
	}
	{
		raw, err := proto.Marshal(&bandwidthlimitv3.BandwidthLimit{
			StatPrefix: "seed_inv3",
			LimitKbps:  wrapperspb.UInt64(0),
		})
		if err != nil {
			f.Fatalf("invalid seed[3] marshal: %v", err)
		}
		f.Add(raw) // (3) limit_kbps=0 foot-gun
	}
	{
		raw, err := proto.Marshal(&bandwidthlimitv3.BandwidthLimit{
			StatPrefix:   "seed_inv4",
			LimitKbps:    wrapperspb.UInt64(10),
			FillInterval: durationpb.New(10 * time.Millisecond),
		})
		if err != nil {
			f.Fatalf("invalid seed[4] marshal: %v", err)
		}
		f.Add(raw) // (4) fill_interval=10ms below-min bounds
	}

	f.Fuzz(func(t *testing.T, raw []byte) {
		// Empty FactoryCtx (no Stats registry) per phase-13 buffer / phase-14
		// compressor precedent: this fuzzer targets the typed_config Any-
		// unmarshal pipeline (parse-rejection contract), not the 14-counter
		// stats-registration path. buildCompiledConfig short-circuits the
		// stats path on ctx.Stats==nil per ADR-0085 nil-tolerance.
		anyMsg := &anypb.Any{TypeUrl: TypeURL, Value: raw}
		factory, err := New(anyMsg, envoyhttp.FactoryCtx{})
		if factory == nil && err == nil {
			t.Fatalf("New returned (nil, nil) — invariant violation; len(raw)=%d", len(raw))
		}
		if factory != nil && err != nil {
			t.Fatalf("New returned (factory, err) — invariant violation: %v", err)
		}
	})
}
