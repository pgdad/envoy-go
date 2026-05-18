package adaptive_concurrency

// fuzz_test.go — 27th project-wide fuzzer `FuzzAdaptiveConcurrencyConfigParse`
// per phase-21 SPEC §6.7 + PLAN Task 8 + D6.
//
// Drives arbitrary byte sequences as the typed_config Any.Value payload to
// `buildCompiledConfig`. Asserts the structural contract: must-never-panic
// across `buildCompiledConfig`; PARSE-REJECT failures + proto-Unmarshal
// failures both return `(nil, error)` cleanly (the fuzz body asserts only
// the no-panic invariant; precise PARSE-REJECT wording is asserted by the
// table-driven `TestBuildCompiledConfig/PARSE_REJECT` rows in
// `compiled_config_test.go`).
//
// # Seed corpus per D6
//
// ~29 hand-curated `f.Add` seeds covering:
//   - Valid full Gradient-1 config (1 seed)
//   - Each PARSE-REJECT arm 1-12 × valid-edge-case neighbor (~14 seeds; arm
//     13 fixed_value is STRUCTURALLY UNREACHABLE in v1.32.4 go-control-plane
//     bindings per Task 2 discovery + ADR-0186 §Consequences (d))
//   - Boundary values (interval = 1ns / 1ms / MaxInt64; percentile = 0 / 100;
//     max_concurrency_limit = MaxUint32; ~6 seeds)
//   - Empty config; oneof-absent; nested-message-missing variants (~3 seeds)
//   - Default-applied variants (~3 seeds — wrappers absent so defaults apply)
//   - Raw-bytes garbage seed (verifies the proto-Unmarshal failure path; 1
//     seed via `f.Add([]byte{...})`)
//
// 30s runtime envelope per SPEC §14.3 + ADR-0018 short-mode CI policy.
//
// # Seed authoring strategy
//
// Uses `f.Add(b)` over `testdata/fuzz/<name>/` corpus files per the phase-20
// oauth2 precedent at `internal/filter/http/oauth2/fuzz_test.go` —
// portable + version-controlled + no testdata-file convention. The seeds
// REUSE `validConfig()` from `compiled_config_test.go` (intra-package
// `_test.go` helpers are visible across test files in the same package).

import (
	"math"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	adaptive_concurrencyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/adaptive_concurrency/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// adaptiveConcurrencyTypeURL is the type.googleapis.com URL for the v3
// AdaptiveConcurrency proto. Used to construct the *anypb.Any envelope
// passed to buildCompiledConfig. Sourced from the same TypeUrl string used
// in compiled_config_test.go (testBuildCompiledConfigUnmarshalFailure).
const adaptiveConcurrencyTypeURL = "type.googleapis.com/envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency"

// FuzzAdaptiveConcurrencyConfigParse drives arbitrary byte sequences as the
// typed_config Any.Value payload to buildCompiledConfig. 27th project-wide
// fuzzer per phase-21 SPEC §6.7 + PLAN Task 8 + D6.
//
// Must-never-panic across buildCompiledConfig. Tolerates Unmarshal failures
// via the PARSE-REJECT branch (returns (nil, error) cleanly).
func FuzzAdaptiveConcurrencyConfigParse(f *testing.F) {
	addSeed := func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
		f.Helper()
		b, err := proto.Marshal(ac)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(b)
	}

	// -------------------------------------------------------------------------
	// Seed 1: Valid full Gradient-1 config (baseline; happy path).
	// -------------------------------------------------------------------------
	addSeed(validConfig())

	// -------------------------------------------------------------------------
	// Seeds 2-15: PARSE-REJECT arm 1-12 × variant (~14 seeds; arm 13 omitted
	// per the v1.32.4 unreachable note).
	// -------------------------------------------------------------------------

	// Seed 2 — Arm 1: controller oneof absent.
	{
		ac := validConfig()
		ac.ConcurrencyControllerConfig = nil
		addSeed(ac)
	}

	// Seed 3 — Arm 2: concurrency_limit_params absent.
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().ConcurrencyLimitParams = nil
		addSeed(ac)
	}

	// Seed 4 — Arm 3: min_rtt_calc_params absent.
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().MinRttCalcParams = nil
		addSeed(ac)
	}

	// Seed 5 — Arm 4: concurrency_update_interval zero.
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetConcurrencyLimitParams().ConcurrencyUpdateInterval = durationpb.New(0)
		addSeed(ac)
	}

	// Seed 6 — Arm 4: concurrency_update_interval negative.
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetConcurrencyLimitParams().ConcurrencyUpdateInterval = durationpb.New(-1 * time.Second)
		addSeed(ac)
	}

	// Seed 7 — Arm 5: max_concurrency_limit zero.
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetConcurrencyLimitParams().MaxConcurrencyLimit = &wrapperspb.UInt32Value{Value: 0}
		addSeed(ac)
	}

	// Seed 8 — Arm 6: min_concurrency zero.
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetMinRttCalcParams().MinConcurrency = &wrapperspb.UInt32Value{Value: 0}
		addSeed(ac)
	}

	// Seed 9 — Arm 7: request_count zero.
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetMinRttCalcParams().RequestCount = &wrapperspb.UInt32Value{Value: 0}
		addSeed(ac)
	}

	// Seed 10 — Arm 8: min_rtt interval below 1ms (500us).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetMinRttCalcParams().Interval = durationpb.New(500 * time.Microsecond)
		addSeed(ac)
	}

	// Seed 11 — Arm 8: min_rtt interval absent.
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetMinRttCalcParams().Interval = nil
		addSeed(ac)
	}

	// Seed 12 — Arm 9: sample_aggregate_percentile out-of-range (>100).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().SampleAggregatePercentile = &typev3.Percent{Value: 150.0}
		addSeed(ac)
	}

	// Seed 13 — Arm 10: jitter out-of-range (negative).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetMinRttCalcParams().Jitter = &typev3.Percent{Value: -1.0}
		addSeed(ac)
	}

	// Seed 14 — Arm 11: buffer out-of-range (>100).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetMinRttCalcParams().Buffer = &typev3.Percent{Value: 200.0}
		addSeed(ac)
	}

	// Seed 15 — Arm 12: enabled.runtime_key non-empty (RTDS deferred per ADR-0187).
	{
		ac := validConfig()
		ac.Enabled = &corev3.RuntimeFeatureFlag{
			DefaultValue: &wrapperspb.BoolValue{Value: true},
			RuntimeKey:   "adaptive_concurrency.enabled",
		}
		addSeed(ac)
	}

	// -------------------------------------------------------------------------
	// Seeds 16-21: Boundary values (~6 seeds covering numeric extremes).
	// -------------------------------------------------------------------------

	// Seed 16 — concurrency_update_interval = 1ns (minimum positive duration).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetConcurrencyLimitParams().ConcurrencyUpdateInterval = durationpb.New(1 * time.Nanosecond)
		addSeed(ac)
	}

	// Seed 17 — min_rtt interval = 1ms (boundary of arm 8 valid range).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetMinRttCalcParams().Interval = durationpb.New(1 * time.Millisecond)
		addSeed(ac)
	}

	// Seed 18 — max_concurrency_limit = math.MaxUint32.
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetConcurrencyLimitParams().MaxConcurrencyLimit = &wrapperspb.UInt32Value{Value: math.MaxUint32}
		addSeed(ac)
	}

	// Seed 19 — sample_aggregate_percentile = 0.0 (lower boundary).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().SampleAggregatePercentile = &typev3.Percent{Value: 0.0}
		addSeed(ac)
	}

	// Seed 20 — sample_aggregate_percentile = 100.0 (upper boundary).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().SampleAggregatePercentile = &typev3.Percent{Value: 100.0}
		addSeed(ac)
	}

	// Seed 21 — jitter = 0.0 + buffer = 100.0 (range boundaries together).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().GetMinRttCalcParams().Jitter = &typev3.Percent{Value: 0.0}
		ac.GetGradientControllerConfig().GetMinRttCalcParams().Buffer = &typev3.Percent{Value: 100.0}
		addSeed(ac)
	}

	// -------------------------------------------------------------------------
	// Seeds 22-24: Default-applied variants (wrappers absent so defaults fire).
	// -------------------------------------------------------------------------

	// Seed 22 — sample_aggregate_percentile absent (default 0.50 fires).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().SampleAggregatePercentile = nil
		addSeed(ac)
	}

	// Seed 23 — all optional wrappers absent (defaults fire across the board).
	{
		ac := validConfig()
		ac.GetGradientControllerConfig().SampleAggregatePercentile = nil
		ac.GetGradientControllerConfig().GetConcurrencyLimitParams().MaxConcurrencyLimit = nil
		ac.GetGradientControllerConfig().GetMinRttCalcParams().RequestCount = nil
		ac.GetGradientControllerConfig().GetMinRttCalcParams().Jitter = nil
		ac.GetGradientControllerConfig().GetMinRttCalcParams().MinConcurrency = nil
		ac.GetGradientControllerConfig().GetMinRttCalcParams().Buffer = nil
		ac.ConcurrencyLimitExceededStatus = nil
		addSeed(ac)
	}

	// Seed 24 — enabled absent (default OFF per AMEND-4 — REFUTES BRAINSTORM §2.1).
	{
		ac := validConfig()
		ac.Enabled = nil
		addSeed(ac)
	}

	// -------------------------------------------------------------------------
	// Seeds 25-27: Empty / oneof-absent / nested-missing variants.
	// -------------------------------------------------------------------------

	// Seed 25 — Empty AdaptiveConcurrency (oneof unset; arm 1 fires).
	addSeed(&adaptive_concurrencyv3.AdaptiveConcurrency{})

	// Seed 26 — Empty GradientControllerConfig (oneof present, nested absent;
	// arm 2 fires because concurrency_limit_params is nil).
	addSeed(&adaptive_concurrencyv3.AdaptiveConcurrency{
		ConcurrencyControllerConfig: &adaptive_concurrencyv3.AdaptiveConcurrency_GradientControllerConfig{
			GradientControllerConfig: &adaptive_concurrencyv3.GradientControllerConfig{},
		},
	})

	// Seed 27 — Only concurrency_limit_params present (arm 3 fires; nested
	// min_rtt_calc_params absent).
	addSeed(&adaptive_concurrencyv3.AdaptiveConcurrency{
		ConcurrencyControllerConfig: &adaptive_concurrencyv3.AdaptiveConcurrency_GradientControllerConfig{
			GradientControllerConfig: &adaptive_concurrencyv3.GradientControllerConfig{
				ConcurrencyLimitParams: &adaptive_concurrencyv3.GradientControllerConfig_ConcurrencyLimitCalculationParams{
					ConcurrencyUpdateInterval: durationpb.New(100 * time.Millisecond),
				},
			},
		},
	})

	// -------------------------------------------------------------------------
	// Seeds 28-29: envoy-go-strict additional variants.
	// -------------------------------------------------------------------------

	// Seed 28 — enabled present + default_value absent + runtime_key empty
	// (defaults to false per AMEND-4; valid envoy-go-strict happy path).
	{
		ac := validConfig()
		ac.Enabled = &corev3.RuntimeFeatureFlag{
			DefaultValue: nil,
			RuntimeKey:   "",
		}
		addSeed(ac)
	}

	// Seed 29 — concurrency_limit_exceeded_status = 0 code (default 503 fires).
	{
		ac := validConfig()
		ac.ConcurrencyLimitExceededStatus = &typev3.HttpStatus{Code: typev3.StatusCode_Empty}
		addSeed(ac)
	}

	// -------------------------------------------------------------------------
	// Seed 30: Raw garbage bytes — verifies the proto-Unmarshal failure path
	// returns (nil, error) cleanly via the wrapped "typed_config unmarshal"
	// branch in compiled_config.go.
	// -------------------------------------------------------------------------
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0xff})

	// -------------------------------------------------------------------------
	// Fuzz body — must-never-panic structural assertion.
	// -------------------------------------------------------------------------
	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("buildCompiledConfig panicked: %v\nInput: %x", r, data)
			}
		}()
		typedConfig := &anypb.Any{
			TypeUrl: adaptiveConcurrencyTypeURL,
			Value:   data,
		}
		// err is fine (PARSE-REJECT + Unmarshal failure are expected on many
		// random inputs); a panic is not. The two-valued return is discarded
		// because the structural contract assertion is the no-panic invariant.
		_, _ = buildCompiledConfig(typedConfig)
	})
}
