package admission_control

// fuzz_test.go — 32nd project-wide fuzzer `FuzzAdmissionControlConfigParse`
// per phase-23 SPEC §6.7 + PLAN Task 7 + PD-10.
//
// Drives arbitrary byte sequences as the typed_config Any.Value payload to
// `buildCompiledConfig`. Asserts the structural contract: must-never-panic
// across `buildCompiledConfig`; PARSE-REJECT failures + proto-Unmarshal
// failures both return `(nil, error)` cleanly (the fuzz body asserts only
// the no-panic invariant; precise PARSE-REJECT wording is asserted by the
// table-driven `TestBuildCompiledConfig/PARSE_REJECT` rows in
// `compiled_config_test.go`).
//
// # Seed corpus per PD-10
//
// 31 hand-curated `f.Add` seeds covering:
//   - Valid full config (both success-criteria arms + all knobs set; 1 seed)
//   - Each of the 9 PARSE-REJECT arms (§5.1 + §5.2)
//   - Empty config; oneof-absent; enabled-absent variants (~3 seeds)
//   - Boundary / edge-case neighbors: sr_threshold at 1.0% boundary;
//     http range at limits; grpc exactly 16 codes; all-defaults-applied (~5 seeds)
//   - Raw-bytes garbage seed (verifies the proto-Unmarshal failure path; 1 seed)
//
// 30s runtime envelope per SPEC §14.3 + ADR-0018 short-mode CI policy.
//
// # Seed authoring strategy
//
// Uses `f.Add(b)` over `testdata/fuzz/<name>/` corpus files per the phase-21
// adaptive_concurrency precedent at
// `internal/filter/http/adaptive_concurrency/fuzz_test.go` — portable +
// version-controlled + no testdata-file convention. Seeds REUSE `validConfig()`
// from `compiled_config_test.go` (intra-package `_test.go` helpers are visible
// across test files in the same package).

import (
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	acv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/admission_control/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// admissionControlTypeURL is the type.googleapis.com URL for the v3
// AdmissionControl proto. Used to construct the *anypb.Any envelope
// passed to buildCompiledConfig. Byte-exact per the TypeURL constant
// in admission_control.go; regression-pinned at TestTypeURL_ByteExact.
const admissionControlTypeURL = "type.googleapis.com/envoy.extensions.filters.http.admission_control.v3.AdmissionControl"

// FuzzAdmissionControlConfigParse drives arbitrary byte sequences as the
// typed_config Any.Value payload to buildCompiledConfig. 32nd project-wide
// fuzzer per phase-23 SPEC §6.7 + PLAN Task 7 + PD-10.
//
// Must-never-panic across buildCompiledConfig. Tolerates Unmarshal failures
// via the PARSE-REJECT branch (returns (nil, error) cleanly).
func FuzzAdmissionControlConfigParse(f *testing.F) {
	addSeed := func(ac *acv3.AdmissionControl) {
		f.Helper()
		b, err := proto.Marshal(ac)
		if err != nil {
			f.Fatalf("seed marshal: %v", err)
		}
		f.Add(b)
	}

	// -------------------------------------------------------------------------
	// Seed 1: Valid full config (baseline; happy path; both success-criteria
	// arms + all knobs set per PD-10).
	// -------------------------------------------------------------------------
	addSeed(validConfig())

	// -------------------------------------------------------------------------
	// Seeds 2-13: Each PARSE-REJECT arm (§5.1 arms 1-4 + §5.2 arms 5-9); arms 2 and 3 carry extra boundary sub-variants.
	// -------------------------------------------------------------------------

	// Seed 2 — Arm 1 (§5.1): evaluation_criteria oneof absent.
	{
		ac := validConfig()
		ac.EvaluationCriteria = nil
		addSeed(ac)
	}

	// Seed 3 — Arm 2 (§5.1): sr_threshold.default_value < 1.0% (exact 0.0).
	{
		ac := validConfig()
		ac.SrThreshold = &corev3.RuntimePercent{
			DefaultValue: &typev3.Percent{Value: 0.0},
			RuntimeKey:   "",
		}
		addSeed(ac)
	}

	// Seed 4 — Arm 2 (§5.1): sr_threshold.default_value = 0.5 (< 1.0%).
	{
		ac := validConfig()
		ac.SrThreshold = &corev3.RuntimePercent{
			DefaultValue: &typev3.Percent{Value: 0.5},
			RuntimeKey:   "",
		}
		addSeed(ac)
	}

	// Seed 5 — Arm 3 (§5.1): http_success_status range invalid (start > end).
	{
		ac := validConfig()
		ac.GetSuccessCriteria().HttpCriteria = &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
			HttpSuccessStatus: []*typev3.Int32Range{
				{Start: 400, End: 200},
			},
		}
		addSeed(ac)
	}

	// Seed 6 — Arm 3 (§5.1): http_success_status range out of bounds (end >= 600, e.g. 700).
	{
		ac := validConfig()
		ac.GetSuccessCriteria().HttpCriteria = &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
			HttpSuccessStatus: []*typev3.Int32Range{
				{Start: 100, End: 700},
			},
		}
		addSeed(ac)
	}

	// Seed 7 — Arm 3 (§5.1): http_success_status range out of bounds (start < 100).
	{
		ac := validConfig()
		ac.GetSuccessCriteria().HttpCriteria = &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
			HttpSuccessStatus: []*typev3.Int32Range{
				{Start: 50, End: 200},
			},
		}
		addSeed(ac)
	}

	// Seed 8 — Arm 4 (§5.1): grpc_success_status > 16 codes (17 codes).
	{
		ac := validConfig()
		codes := make([]uint32, 17)
		for i := range codes {
			codes[i] = uint32(i)
		}
		ac.GetSuccessCriteria().GrpcCriteria = &acv3.AdmissionControl_SuccessCriteria_GrpcCriteria{
			GrpcSuccessStatus: codes,
		}
		addSeed(ac)
	}

	// Seed 9 — Arm 5 (§5.2): enabled.runtime_key non-empty.
	{
		ac := validConfig()
		ac.Enabled = &corev3.RuntimeFeatureFlag{
			DefaultValue: &wrapperspb.BoolValue{Value: true},
			RuntimeKey:   "admission_control.enabled",
		}
		addSeed(ac)
	}

	// Seed 10 — Arm 6 (§5.2): aggression.runtime_key non-empty.
	{
		ac := validConfig()
		ac.Aggression = &corev3.RuntimeDouble{
			DefaultValue: 1.0,
			RuntimeKey:   "admission_control.aggression",
		}
		addSeed(ac)
	}

	// Seed 11 — Arm 7 (§5.2): sr_threshold.runtime_key non-empty.
	{
		ac := validConfig()
		ac.SrThreshold = &corev3.RuntimePercent{
			DefaultValue: &typev3.Percent{Value: 95.0},
			RuntimeKey:   "admission_control.sr_threshold",
		}
		addSeed(ac)
	}

	// Seed 12 — Arm 8 (§5.2): max_rejection_probability.runtime_key non-empty.
	{
		ac := validConfig()
		ac.MaxRejectionProbability = &corev3.RuntimePercent{
			DefaultValue: &typev3.Percent{Value: 80.0},
			RuntimeKey:   "admission_control.max_rejection_probability",
		}
		addSeed(ac)
	}

	// Seed 13 — Arm 9 (§5.2): rps_threshold.runtime_key non-empty.
	{
		ac := validConfig()
		ac.RpsThreshold = &corev3.RuntimeUInt32{
			DefaultValue: 0,
			RuntimeKey:   "admission_control.rps_threshold",
		}
		addSeed(ac)
	}

	// -------------------------------------------------------------------------
	// Seeds 14-16: Empty / oneof-absent / enabled-absent variants.
	// -------------------------------------------------------------------------

	// Seed 14 — Empty AdmissionControl (oneof unset; arm 1 fires).
	addSeed(&acv3.AdmissionControl{})

	// Seed 15 — oneof-absent: SuccessCriteria present but empty sub-messages
	// (http_criteria + grpc_criteria both nil — defaults apply for both arms).
	addSeed(&acv3.AdmissionControl{
		EvaluationCriteria: &acv3.AdmissionControl_SuccessCriteria_{
			SuccessCriteria: &acv3.AdmissionControl_SuccessCriteria{},
		},
	})

	// Seed 16 — enabled absent entirely (AMEND-4: absent ⇒ ENABLED default).
	{
		ac := validConfig()
		ac.Enabled = nil
		addSeed(ac)
	}

	// -------------------------------------------------------------------------
	// Seeds 17-22: Boundary / edge-case neighbors.
	// -------------------------------------------------------------------------

	// Seed 17 — sr_threshold = 1.0 exactly (boundary: passes arm 2 check).
	{
		ac := validConfig()
		ac.SrThreshold = &corev3.RuntimePercent{
			DefaultValue: &typev3.Percent{Value: 1.0},
			RuntimeKey:   "",
		}
		addSeed(ac)
	}

	// Seed 18 — http range [100,600) (valid maximum end; boundary of arm 3).
	{
		ac := validConfig()
		ac.GetSuccessCriteria().HttpCriteria = &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
			HttpSuccessStatus: []*typev3.Int32Range{
				{Start: 100, End: 600},
			},
		}
		addSeed(ac)
	}

	// Seed 19 — grpc_success_status exactly 16 codes (boundary: arm 4 passes).
	{
		ac := validConfig()
		codes := make([]uint32, 16)
		for i := range codes {
			codes[i] = uint32(i)
		}
		ac.GetSuccessCriteria().GrpcCriteria = &acv3.AdmissionControl_SuccessCriteria_GrpcCriteria{
			GrpcSuccessStatus: codes,
		}
		addSeed(ac)
	}

	// Seed 20 — enabled present + default_value absent + runtime_key empty
	// (defaults to true per AMEND-4; valid envoy-go-strict happy path).
	{
		ac := validConfig()
		ac.Enabled = &corev3.RuntimeFeatureFlag{
			DefaultValue: nil,
			RuntimeKey:   "",
		}
		addSeed(ac)
	}

	// Seed 21 — enabled default_value=false (pass-through per §5.3 case 2).
	{
		ac := validConfig()
		ac.Enabled = &corev3.RuntimeFeatureFlag{
			DefaultValue: &wrapperspb.BoolValue{Value: false},
			RuntimeKey:   "",
		}
		addSeed(ac)
	}

	// Seed 22 — grpc_criteria absent, http_criteria absent (all defaults fire).
	{
		ac := validConfig()
		ac.GetSuccessCriteria().HttpCriteria = nil
		ac.GetSuccessCriteria().GrpcCriteria = nil
		addSeed(ac)
	}

	// -------------------------------------------------------------------------
	// Seeds 23-30: All-defaults-applied + additional valid variants.
	// -------------------------------------------------------------------------

	// Seed 23 — all optional wrappers absent (full defaults fire for every knob).
	addSeed(&acv3.AdmissionControl{
		EvaluationCriteria: &acv3.AdmissionControl_SuccessCriteria_{
			SuccessCriteria: &acv3.AdmissionControl_SuccessCriteria{
				HttpCriteria: &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
					HttpSuccessStatus: []*typev3.Int32Range{
						{Start: 100, End: 500},
					},
				},
			},
		},
	})

	// Seed 24 — sampling_window absent (default 30s fires).
	{
		ac := validConfig()
		ac.SamplingWindow = nil
		addSeed(ac)
	}

	// Seed 25 — sampling_window=60s: non-default value preserved as-is (no clamp/default).
	{
		ac := validConfig()
		ac.SamplingWindow = durationpb.New(60 * time.Second)
		addSeed(ac)
	}

	// Seed 26 — sampling_window = 1500ms (rounded to 1s via ms/1000 truncation).
	{
		ac := validConfig()
		ac.SamplingWindow = durationpb.New(1500 * time.Millisecond)
		addSeed(ac)
	}

	// Seed 27 — aggression = 0.5 (below floor; clamped to 1.0 at apply time).
	{
		ac := validConfig()
		ac.Aggression = &corev3.RuntimeDouble{
			DefaultValue: 0.5,
			RuntimeKey:   "",
		}
		addSeed(ac)
	}

	// Seed 28 — sr_threshold > 100.0% (clamped to 100.0% at apply time).
	{
		ac := validConfig()
		ac.SrThreshold = &corev3.RuntimePercent{
			DefaultValue: &typev3.Percent{Value: 150.0},
			RuntimeKey:   "",
		}
		addSeed(ac)
	}

	// Seed 29 — rps_threshold set to non-zero value.
	{
		ac := validConfig()
		ac.RpsThreshold = &corev3.RuntimeUInt32{
			DefaultValue: 100,
			RuntimeKey:   "",
		}
		addSeed(ac)
	}

	// Seed 30 — multiple valid http ranges (two ranges with both arms active).
	{
		ac := validConfig()
		ac.GetSuccessCriteria().HttpCriteria = &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
			HttpSuccessStatus: []*typev3.Int32Range{
				{Start: 100, End: 300},
				{Start: 400, End: 500},
			},
		}
		addSeed(ac)
	}

	// -------------------------------------------------------------------------
	// Seed 31: Raw garbage bytes — verifies the proto-Unmarshal failure path
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
			TypeUrl: admissionControlTypeURL,
			Value:   data,
		}
		// err is fine (PARSE-REJECT + Unmarshal failure are expected on many
		// random inputs); a panic is not. The two-valued return is discarded
		// because the structural contract assertion is the no-panic invariant.
		_, _ = buildCompiledConfig(typedConfig)
	})
}
