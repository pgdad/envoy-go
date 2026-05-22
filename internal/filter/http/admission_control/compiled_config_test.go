package admission_control

import (
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	acv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/admission_control/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// validConfig returns a fully-populated AdmissionControl proto with every
// field set to a valid value. Each PARSE-REJECT-row test `mutate` closure
// modifies this baseline to drive a specific arm.
//
// Baseline values:
//   - enabled: present, default_value=true, runtime_key=""
//   - evaluation_criteria: SuccessCriteria (both http + grpc arms)
//   - sampling_window: 30s
//   - aggression: 1.0, runtime_key=""
//   - sr_threshold: 95.0 (default_value), runtime_key=""
//   - rps_threshold: 0 (default_value), runtime_key=""
//   - max_rejection_probability: 80.0 (default_value), runtime_key=""
func validConfig() *acv3.AdmissionControl {
	return &acv3.AdmissionControl{
		Enabled: &corev3.RuntimeFeatureFlag{
			DefaultValue: &wrapperspb.BoolValue{Value: true},
			RuntimeKey:   "",
		},
		EvaluationCriteria: &acv3.AdmissionControl_SuccessCriteria_{
			SuccessCriteria: &acv3.AdmissionControl_SuccessCriteria{
				HttpCriteria: &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
					HttpSuccessStatus: []*typev3.Int32Range{
						{Start: 100, End: 500},
					},
				},
				GrpcCriteria: &acv3.AdmissionControl_SuccessCriteria_GrpcCriteria{
					GrpcSuccessStatus: []uint32{0, 1},
				},
			},
		},
		SamplingWindow: durationpb.New(30 * time.Second),
		Aggression: &corev3.RuntimeDouble{
			DefaultValue: 1.0,
			RuntimeKey:   "",
		},
		SrThreshold: &corev3.RuntimePercent{
			DefaultValue: &typev3.Percent{Value: 95.0},
			RuntimeKey:   "",
		},
		RpsThreshold: &corev3.RuntimeUInt32{
			DefaultValue: 0,
			RuntimeKey:   "",
		},
		MaxRejectionProbability: &corev3.RuntimePercent{
			DefaultValue: &typev3.Percent{Value: 80.0},
			RuntimeKey:   "",
		},
	}
}

// toAny wraps the AdmissionControl proto in an *anypb.Any envelope per the
// buildCompiledConfig signature contract.
func toAny(t *testing.T, ac *acv3.AdmissionControl) *anypb.Any {
	t.Helper()
	a, err := anypb.New(ac)
	if err != nil {
		t.Fatalf("anypb.New failed: %v", err)
	}
	return a
}

// -----------------------------------------------------------------------------
// TestBuildCompiledConfig — table-driven PARSE-REJECT roster + happy-path +
// default-applied coverage per phase-23 SPEC §14.1 #8 + #9 + PD-2.
// -----------------------------------------------------------------------------

func TestBuildCompiledConfig(t *testing.T) {
	t.Run("PARSE_REJECT", testBuildCompiledConfigParseReject)
	t.Run("Defaults", testBuildCompiledConfigDefaults)
	t.Run("HappyPath", testBuildCompiledConfigHappyPath)
	t.Run("NilTypedConfig", testBuildCompiledConfigNilTypedConfig)
	t.Run("UnmarshalFailure", testBuildCompiledConfigUnmarshalFailure)
}

// testBuildCompiledConfigParseReject drives each of the 9 PARSE-REJECT arms
// per SPEC §5.1 (4 RATIFIED-from-config arms) + §5.2 (5 envoy-go-strict
// runtime_key arms). Each row mutates the baseline validConfig() to trigger
// ONE specific arm and asserts err.Error() matches the byte-stable wording
// exactly per ADR-0080 + PD-2.
func testBuildCompiledConfigParseReject(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*acv3.AdmissionControl)
		wantErrEq string
	}{
		// ---- §5.1 RATIFIED-from-config Arm 1: evaluation_criteria oneof absent ----
		{
			name: "Arm01_EvaluationCriteria_Absent",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.EvaluationCriteria = nil
			},
			wantErrEq: parseRejectEvaluationCriteriaRequired,
		},

		// ---- §5.1 RATIFIED-from-config Arm 2: sr_threshold.default_value < 1.0% ----
		{
			name: "Arm02_SrThreshold_BelowOnePercent",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.SrThreshold = &corev3.RuntimePercent{
					DefaultValue: &typev3.Percent{Value: 0.5},
					RuntimeKey:   "",
				}
			},
			wantErrEq: parseRejectSrThresholdTooLow,
		},
		{
			name: "Arm02_SrThreshold_ExactlyZero",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.SrThreshold = &corev3.RuntimePercent{
					DefaultValue: &typev3.Percent{Value: 0.0},
					RuntimeKey:   "",
				}
			},
			wantErrEq: parseRejectSrThresholdTooLow,
		},

		// ---- §5.1 RATIFIED-from-config Arm 3: http_success_status range invalid ----
		{
			name: "Arm03_HttpRange_StartBelowMin",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.GetSuccessCriteria().HttpCriteria = &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
					HttpSuccessStatus: []*typev3.Int32Range{
						{Start: 99, End: 200},
					},
				}
			},
			wantErrEq: parseRejectHTTPRangeInvalid,
		},
		{
			name: "Arm03_HttpRange_EndAtOrAboveCeiling",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.GetSuccessCriteria().HttpCriteria = &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
					HttpSuccessStatus: []*typev3.Int32Range{
						{Start: 200, End: 601},
					},
				}
			},
			wantErrEq: parseRejectHTTPRangeInvalid,
		},
		{
			name: "Arm03_HttpRange_StartGreaterThanEnd",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.GetSuccessCriteria().HttpCriteria = &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
					HttpSuccessStatus: []*typev3.Int32Range{
						{Start: 400, End: 300},
					},
				}
			},
			wantErrEq: parseRejectHTTPRangeInvalid,
		},

		// ---- §5.1 RATIFIED-from-config Arm 4: grpc_success_status > 16 codes ----
		{
			name: "Arm04_GrpcCodes_MoreThan16",
			mutate: func(ac *acv3.AdmissionControl) {
				codes := make([]uint32, 17)
				for i := range codes {
					codes[i] = uint32(i)
				}
				ac.GetSuccessCriteria().GrpcCriteria = &acv3.AdmissionControl_SuccessCriteria_GrpcCriteria{
					GrpcSuccessStatus: codes,
				}
			},
			wantErrEq: parseRejectGRPCCodesExceed16,
		},
		{
			name: "Arm04_GrpcCodes_Exactly17",
			mutate: func(ac *acv3.AdmissionControl) {
				codes := make([]uint32, 17)
				for i := range codes {
					codes[i] = uint32(i)
				}
				ac.GetSuccessCriteria().GrpcCriteria = &acv3.AdmissionControl_SuccessCriteria_GrpcCriteria{
					GrpcSuccessStatus: codes,
				}
			},
			wantErrEq: parseRejectGRPCCodesExceed16,
		},

		// ---- §5.2 envoy-go-strict Arm 5: enabled.runtime_key != "" ----
		{
			name: "Arm05_EnabledRuntimeKey_DefaultTrue",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: &wrapperspb.BoolValue{Value: true},
					RuntimeKey:   "admission_control.enabled",
				}
			},
			wantErrEq: parseRejectEnabledRuntimeKey,
		},
		{
			name: "Arm05_EnabledRuntimeKey_DefaultFalse",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: &wrapperspb.BoolValue{Value: false},
					RuntimeKey:   "admission_control.enabled",
				}
			},
			wantErrEq: parseRejectEnabledRuntimeKey,
		},

		// ---- §5.2 envoy-go-strict Arm 6: aggression.runtime_key != "" ----
		{
			name: "Arm06_AggressionRuntimeKey",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Aggression = &corev3.RuntimeDouble{
					DefaultValue: 1.0,
					RuntimeKey:   "admission_control.aggression",
				}
			},
			wantErrEq: parseRejectAggressionRuntimeKey,
		},

		// ---- §5.2 envoy-go-strict Arm 7: sr_threshold.runtime_key != "" ----
		{
			name: "Arm07_SrThresholdRuntimeKey",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.SrThreshold = &corev3.RuntimePercent{
					DefaultValue: &typev3.Percent{Value: 95.0},
					RuntimeKey:   "admission_control.sr_threshold",
				}
			},
			wantErrEq: parseRejectSrThresholdRuntimeKey,
		},

		// ---- §5.2 envoy-go-strict Arm 8: max_rejection_probability.runtime_key != "" ----
		{
			name: "Arm08_MaxRejectionProbabilityRuntimeKey",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.MaxRejectionProbability = &corev3.RuntimePercent{
					DefaultValue: &typev3.Percent{Value: 80.0},
					RuntimeKey:   "admission_control.max_rejection_probability",
				}
			},
			wantErrEq: parseRejectMaxRejectionProbabilityRuntimeKey,
		},

		// ---- §5.2 envoy-go-strict Arm 9: rps_threshold.runtime_key != "" ----
		{
			name: "Arm09_RpsThresholdRuntimeKey",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.RpsThreshold = &corev3.RuntimeUInt32{
					DefaultValue: 0,
					RuntimeKey:   "admission_control.rps_threshold",
				}
			},
			wantErrEq: parseRejectRpsThresholdRuntimeKey,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ac := validConfig()
			tc.mutate(ac)
			got, err := buildCompiledConfig(toAny(t, ac))
			if err == nil {
				t.Fatalf("expected PARSE-REJECT error %q, got nil (cc=%+v)", tc.wantErrEq, got)
			}
			if err.Error() != tc.wantErrEq {
				t.Fatalf("error mismatch:\n  want: %q\n   got: %q", tc.wantErrEq, err.Error())
			}
			if got != nil {
				t.Fatalf("expected nil *compiledConfig on PARSE-REJECT, got %+v", got)
			}
		})
	}
}

// testBuildCompiledConfigDefaults exercises the default-application semantics
// per SPEC §6.1 + AMEND-4. Each row asserts a specific default is applied
// when the corresponding proto field is absent.
func testBuildCompiledConfigDefaults(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*acv3.AdmissionControl)
		assert func(t *testing.T, cc *compiledConfig)
	}{
		// ---- sampling_window default 30s ----
		{
			name: "SamplingWindow_Absent_Defaults30s",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.SamplingWindow = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.samplingWindow != 30*time.Second {
					t.Fatalf("expected samplingWindow=30s, got %v", cc.samplingWindow)
				}
			},
		},
		{
			name: "SamplingWindow_60s_Preserved",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.SamplingWindow = durationpb.New(60 * time.Second)
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.samplingWindow != 60*time.Second {
					t.Fatalf("expected samplingWindow=60s, got %v", cc.samplingWindow)
				}
			},
		},
		// ms/1000 truncation: 1500ms → 1s
		{
			name: "SamplingWindow_1500ms_Truncated1s",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.SamplingWindow = durationpb.New(1500 * time.Millisecond)
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.samplingWindow != 1*time.Second {
					t.Fatalf("expected samplingWindow=1s (ms/1000 truncation), got %v", cc.samplingWindow)
				}
			},
		},

		// ---- aggression default 1.0 when absent ----
		{
			name: "Aggression_Absent_Defaults1_0",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Aggression = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.aggression != 1.0 {
					t.Fatalf("expected aggression=1.0 (default), got %v", cc.aggression)
				}
			},
		},
		// aggression floored to 1.0 (AMEND-1)
		{
			name: "Aggression_BelowFloor_ClampsTo1_0",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Aggression = &corev3.RuntimeDouble{DefaultValue: 0.5, RuntimeKey: ""}
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.aggression != 1.0 {
					t.Fatalf("expected aggression=1.0 (floor; configured 0.5), got %v", cc.aggression)
				}
			},
		},
		{
			name: "Aggression_2_0_Preserved",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Aggression = &corev3.RuntimeDouble{DefaultValue: 2.0, RuntimeKey: ""}
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.aggression != 2.0 {
					t.Fatalf("expected aggression=2.0, got %v", cc.aggression)
				}
			},
		},

		// ---- sr_threshold default 0.95 when absent ----
		{
			name: "SrThreshold_Absent_Defaults0_95",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.SrThreshold = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.srThreshold != 0.95 {
					t.Fatalf("expected srThreshold=0.95 (default 95%%/100), got %v", cc.srThreshold)
				}
			},
		},
		// sr_threshold=95.0% → fraction 0.95
		{
			name: "SrThreshold_95pct_Fraction",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.SrThreshold = &corev3.RuntimePercent{
					DefaultValue: &typev3.Percent{Value: 95.0},
					RuntimeKey:   "",
				}
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.srThreshold != 0.95 {
					t.Fatalf("expected srThreshold=0.95, got %v", cc.srThreshold)
				}
			},
		},
		// sr_threshold=110% → min(110,100)/100 = 1.0
		{
			name: "SrThreshold_Above100pct_Clamped1_0",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.SrThreshold = &corev3.RuntimePercent{
					DefaultValue: &typev3.Percent{Value: 110.0},
					RuntimeKey:   "",
				}
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.srThreshold != 1.0 {
					t.Fatalf("expected srThreshold=1.0 (min(110,100)/100), got %v", cc.srThreshold)
				}
			},
		},

		// ---- rps_threshold default 0 when absent ----
		{
			name: "RpsThreshold_Absent_Defaults0",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.RpsThreshold = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.rpsThreshold != 0 {
					t.Fatalf("expected rpsThreshold=0 (default), got %v", cc.rpsThreshold)
				}
			},
		},
		{
			name: "RpsThreshold_100_Preserved",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.RpsThreshold = &corev3.RuntimeUInt32{DefaultValue: 100, RuntimeKey: ""}
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.rpsThreshold != 100 {
					t.Fatalf("expected rpsThreshold=100, got %v", cc.rpsThreshold)
				}
			},
		},

		// ---- max_rejection_probability default 0.80 when absent ----
		{
			name: "MaxRejectionProbability_Absent_Defaults0_80",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.MaxRejectionProbability = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.maxRejectionProbability != 0.80 {
					t.Fatalf("expected maxRejectionProbability=0.80 (default 80%%/100), got %v", cc.maxRejectionProbability)
				}
			},
		},
		// max_rejection_probability=50.0% → 0.50
		{
			name: "MaxRejectionProbability_50pct_Fraction",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.MaxRejectionProbability = &corev3.RuntimePercent{
					DefaultValue: &typev3.Percent{Value: 50.0},
					RuntimeKey:   "",
				}
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.maxRejectionProbability != 0.50 {
					t.Fatalf("expected maxRejectionProbability=0.50, got %v", cc.maxRejectionProbability)
				}
			},
		},

		// ---- httpSuccessRanges defaults to {[100,500)} when http_criteria absent ----
		{
			name: "HttpCriteria_Absent_DefaultRange100_500",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.GetSuccessCriteria().HttpCriteria = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if len(cc.httpSuccessRanges) != 1 {
					t.Fatalf("expected 1 default HTTP range, got %d", len(cc.httpSuccessRanges))
				}
				r := cc.httpSuccessRanges[0]
				if r.start != 100 || r.end != 500 {
					t.Fatalf("expected default range [100,500), got [%d,%d)", r.start, r.end)
				}
			},
		},

		// ---- grpcSuccessCodes defaults to the 11-code well-known set per AMEND-5 ----
		{
			name: "GrpcCriteria_Absent_Default11Codes",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.GetSuccessCriteria().GrpcCriteria = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				// The 11-code well-known set per AMEND-5:
				// {0,1,2,3,5,6,7,9,11,12,16}
				wantCodes := []uint32{0, 1, 2, 3, 5, 6, 7, 9, 11, 12, 16}
				if len(cc.grpcSuccessCodes) != len(wantCodes) {
					t.Fatalf("expected %d gRPC success codes, got %d", len(wantCodes), len(cc.grpcSuccessCodes))
				}
				for _, c := range wantCodes {
					if _, ok := cc.grpcSuccessCodes[c]; !ok {
						t.Fatalf("expected gRPC code %d in default set, but not found", c)
					}
				}
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ac := validConfig()
			tc.mutate(ac)
			cc, err := buildCompiledConfig(toAny(t, ac))
			if err != nil {
				t.Fatalf("unexpected PARSE-REJECT error: %v", err)
			}
			if cc == nil {
				t.Fatalf("expected non-nil *compiledConfig on default-applied path")
			}
			tc.assert(t, cc)
		})
	}
}

// testBuildCompiledConfigHappyPath exercises the fully-populated baseline +
// asserts every field is captured correctly per SPEC §6.1.
func testBuildCompiledConfigHappyPath(t *testing.T) {
	ac := validConfig()
	cc, err := buildCompiledConfig(toAny(t, ac))
	if err != nil {
		t.Fatalf("unexpected PARSE-REJECT error: %v", err)
	}
	if cc == nil {
		t.Fatalf("expected non-nil *compiledConfig")
	}

	if !cc.enabled {
		t.Errorf("expected enabled=true (baseline default_value=true), got false")
	}
	if cc.samplingWindow != 30*time.Second {
		t.Errorf("expected samplingWindow=30s, got %v", cc.samplingWindow)
	}
	if cc.aggression != 1.0 {
		t.Errorf("expected aggression=1.0, got %v", cc.aggression)
	}
	if cc.srThreshold != 0.95 {
		t.Errorf("expected srThreshold=0.95 (95/100), got %v", cc.srThreshold)
	}
	if cc.rpsThreshold != 0 {
		t.Errorf("expected rpsThreshold=0, got %v", cc.rpsThreshold)
	}
	if cc.maxRejectionProbability != 0.80 {
		t.Errorf("expected maxRejectionProbability=0.80 (80/100), got %v", cc.maxRejectionProbability)
	}

	// HTTP ranges: baseline has one range [100,500)
	if len(cc.httpSuccessRanges) != 1 {
		t.Errorf("expected 1 HTTP success range, got %d", len(cc.httpSuccessRanges))
	} else {
		r := cc.httpSuccessRanges[0]
		if r.start != 100 || r.end != 500 {
			t.Errorf("expected range [100,500), got [%d,%d)", r.start, r.end)
		}
	}

	// gRPC codes: baseline has {0, 1}
	if len(cc.grpcSuccessCodes) != 2 {
		t.Errorf("expected 2 gRPC success codes from baseline, got %d", len(cc.grpcSuccessCodes))
	}
	for _, code := range []uint32{0, 1} {
		if _, ok := cc.grpcSuccessCodes[code]; !ok {
			t.Errorf("expected gRPC code %d in success codes", code)
		}
	}
}

// testBuildCompiledConfigNilTypedConfig drives the ADR-0072 fail-fast path
// when the typed_config envelope is nil.
func testBuildCompiledConfigNilTypedConfig(t *testing.T) {
	cc, err := buildCompiledConfig(nil)
	if err == nil {
		t.Fatalf("expected error on nil typed_config, got nil (cc=%+v)", cc)
	}
	if cc != nil {
		t.Fatalf("expected nil *compiledConfig on nil typed_config, got %+v", cc)
	}
	const want = "admission_control: typed_config required"
	if err.Error() != want {
		t.Fatalf("error mismatch:\n  want: %q\n   got: %q", want, err.Error())
	}
}

// testBuildCompiledConfigUnmarshalFailure drives the proto-unmarshal-failure
// path. This is the ONE intentional exception to the byte-stable wording
// discipline (per the phase-20 oauth2 precedent); we assert only the prefix.
func testBuildCompiledConfigUnmarshalFailure(t *testing.T) {
	bad := &anypb.Any{
		TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.admission_control.v3.AdmissionControl",
		Value:   []byte{0xff, 0xff, 0xff, 0xff, 0xff},
	}
	cc, err := buildCompiledConfig(bad)
	if err == nil {
		t.Fatalf("expected unmarshal error, got nil (cc=%+v)", cc)
	}
	if cc != nil {
		t.Fatalf("expected nil *compiledConfig on unmarshal failure, got %+v", cc)
	}
	const wantPrefix = "admission_control: typed_config unmarshal:"
	if got := err.Error(); len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("error prefix mismatch:\n  want prefix: %q\n          got: %q", wantPrefix, got)
	}
}

// -----------------------------------------------------------------------------
// TestEnabledMatrix — per SPEC §5.3 + AMEND-4 (absent ⇒ ENABLED — OPPOSITE of
// phase-21 adaptive_concurrency's absent ⇒ disabled).
// -----------------------------------------------------------------------------

func TestEnabledMatrix(t *testing.T) {
	cases := []struct {
		name        string
		mutate      func(*acv3.AdmissionControl)
		wantEnabled bool
		wantErr     string // non-empty means expect PARSE-REJECT
	}{
		// Case 1: enabled absent entirely ⇒ ENABLED (AMEND-4 INVERSION vs phase-21)
		{
			name: "Case1_Absent_ENABLED",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Enabled = nil
			},
			wantEnabled: true,
		},
		// Case 2: present, default_value=false, runtime_key="" ⇒ DISABLED
		{
			name: "Case2_Present_DefaultFalse_DISABLED",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: &wrapperspb.BoolValue{Value: false},
					RuntimeKey:   "",
				}
			},
			wantEnabled: false,
		},
		// Case 3: present, default_value=true, runtime_key="" ⇒ ENABLED
		{
			name: "Case3_Present_DefaultTrue_ENABLED",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: &wrapperspb.BoolValue{Value: true},
					RuntimeKey:   "",
				}
			},
			wantEnabled: true,
		},
		// Case 4: present, runtime_key != "" ⇒ PARSE-REJECT (ADR-0195)
		{
			name: "Case4_RuntimeKey_PARSE_REJECT",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: &wrapperspb.BoolValue{Value: true},
					RuntimeKey:   "ac.enabled",
				}
			},
			wantErr: parseRejectEnabledRuntimeKey,
		},
		// Additional boundary: enabled present, default_value absent (BoolValue wrapper nil)
		// Per AMEND-4 mechanism the `…,true` fallback applies when BoolValue wrapper is nil
		// BUT enabled message itself is present — spec case 1 applies when the message is
		// absent entirely; this sub-case (message present, BoolValue nil) defaults to true
		// via the same `PROTOBUF_GET_WRAPPED_OR_DEFAULT(…, true)` upstream mechanism.
		{
			name: "Case1b_Present_DefaultValueAbsent_ENABLED",
			mutate: func(ac *acv3.AdmissionControl) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: nil,
					RuntimeKey:   "",
				}
			},
			wantEnabled: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			ac := validConfig()
			tc.mutate(ac)
			cc, err := buildCompiledConfig(toAny(t, ac))
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected PARSE-REJECT %q, got nil", tc.wantErr)
				}
				if err.Error() != tc.wantErr {
					t.Fatalf("error mismatch:\n  want: %q\n   got: %q", tc.wantErr, err.Error())
				}
				if cc != nil {
					t.Fatalf("expected nil *compiledConfig on PARSE-REJECT, got %+v", cc)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cc == nil {
				t.Fatalf("expected non-nil *compiledConfig")
			}
			if cc.enabled != tc.wantEnabled {
				t.Fatalf("enabled: want %v, got %v", tc.wantEnabled, cc.enabled)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// TestIsHTTPSuccess + TestIsGRPCSuccess — predicates on *compiledConfig per
// SPEC §4.4 + AMEND-5.
// -----------------------------------------------------------------------------

func TestIsHTTPSuccess(t *testing.T) {
	// Build a compiledConfig with default http ranges [100,500)
	ac := validConfig()
	ac.GetSuccessCriteria().HttpCriteria = nil // use defaults
	cc, err := buildCompiledConfig(toAny(t, ac))
	if err != nil {
		t.Fatalf("unexpected error building compiledConfig: %v", err)
	}

	cases := []struct {
		code int
		want bool
	}{
		{100, true},
		{200, true},
		{404, true},
		{499, true},
		{500, false},
		{503, false},
		{600, false},
		{99, false},
	}
	for _, tc := range cases {
		if got := cc.isHTTPSuccess(tc.code); got != tc.want {
			t.Errorf("isHTTPSuccess(%d): want %v, got %v", tc.code, tc.want, got)
		}
	}

	// Custom configured range [200,300)
	ac2 := validConfig()
	ac2.GetSuccessCriteria().HttpCriteria = &acv3.AdmissionControl_SuccessCriteria_HttpCriteria{
		HttpSuccessStatus: []*typev3.Int32Range{
			{Start: 200, End: 300},
		},
	}
	cc2, err := buildCompiledConfig(toAny(t, ac2))
	if err != nil {
		t.Fatalf("unexpected error building compiledConfig: %v", err)
	}
	if !cc2.isHTTPSuccess(200) {
		t.Errorf("isHTTPSuccess(200) with [200,300): want true, got false")
	}
	if !cc2.isHTTPSuccess(299) {
		t.Errorf("isHTTPSuccess(299) with [200,300): want true, got false")
	}
	if cc2.isHTTPSuccess(300) {
		t.Errorf("isHTTPSuccess(300) with [200,300) [exclusive end]: want false, got true")
	}
	if cc2.isHTTPSuccess(404) {
		t.Errorf("isHTTPSuccess(404) with custom range [200,300): want false, got true")
	}
}

func TestIsGRPCSuccess(t *testing.T) {
	// Build a compiledConfig with default gRPC codes
	ac := validConfig()
	ac.GetSuccessCriteria().GrpcCriteria = nil // use defaults
	cc, err := buildCompiledConfig(toAny(t, ac))
	if err != nil {
		t.Fatalf("unexpected error building compiledConfig: %v", err)
	}

	// The 11-code well-known set per AMEND-5: {0,1,2,3,5,6,7,9,11,12,16}
	wantTrue := []uint32{0, 1, 2, 3, 5, 6, 7, 9, 11, 12, 16}
	for _, c := range wantTrue {
		if !cc.isGRPCSuccess(c) {
			t.Errorf("isGRPCSuccess(%d): want true (default set), got false", c)
		}
	}
	// Code NOT in the well-known set
	wantFalse := []uint32{4, 8, 10, 13, 14, 15, 17}
	for _, c := range wantFalse {
		if cc.isGRPCSuccess(c) {
			t.Errorf("isGRPCSuccess(%d): want false (not in default set), got true", c)
		}
	}

	// Custom configured list {0}
	ac2 := validConfig()
	ac2.GetSuccessCriteria().GrpcCriteria = &acv3.AdmissionControl_SuccessCriteria_GrpcCriteria{
		GrpcSuccessStatus: []uint32{0},
	}
	cc2, err := buildCompiledConfig(toAny(t, ac2))
	if err != nil {
		t.Fatalf("unexpected error building compiledConfig: %v", err)
	}
	if !cc2.isGRPCSuccess(0) {
		t.Errorf("isGRPCSuccess(0) with custom {0}: want true, got false")
	}
	if cc2.isGRPCSuccess(1) {
		t.Errorf("isGRPCSuccess(1) with custom {0}: want false, got true")
	}
}

// -----------------------------------------------------------------------------
// TestParseRejectConstants_ByteStable pins the 9 byte-stable PARSE-REJECT
// wordings per ADR-0080 + PD-2. NO format-string drift; constants asserted
// byte-exact against the PD-2 reference roster.
// -----------------------------------------------------------------------------

func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		// §5.1 RATIFIED-from-config arms (4)
		{
			"Arm01_EvalCriteriaRequired",
			parseRejectEvaluationCriteriaRequired,
			"admission_control: evaluation_criteria is required",
		},
		{
			"Arm02_SrThresholdTooLow",
			parseRejectSrThresholdTooLow,
			"admission_control: sr_threshold cannot be less than 1.0%",
		},
		{
			"Arm03_HttpRangeInvalid",
			parseRejectHTTPRangeInvalid,
			"admission_control: http_success_status range invalid (must be within [100,600) and start<=end)",
		},
		{
			"Arm04_GrpcCodesExceed16",
			parseRejectGRPCCodesExceed16,
			"admission_control: grpc_success_status accepts at most 16 codes",
		},
		// §5.2 envoy-go-strict runtime_key arms (5)
		{
			"Arm05_EnabledRuntimeKey",
			parseRejectEnabledRuntimeKey,
			"admission_control: enabled.runtime_key is not yet supported; use enabled.default_value",
		},
		{
			"Arm06_AggressionRuntimeKey",
			parseRejectAggressionRuntimeKey,
			"admission_control: aggression.runtime_key is not yet supported; use aggression.default_value",
		},
		{
			"Arm07_SrThresholdRuntimeKey",
			parseRejectSrThresholdRuntimeKey,
			"admission_control: sr_threshold.runtime_key is not yet supported; use sr_threshold.default_value",
		},
		{
			"Arm08_MaxRejectionProbabilityRuntimeKey",
			parseRejectMaxRejectionProbabilityRuntimeKey,
			"admission_control: max_rejection_probability.runtime_key is not yet supported; use max_rejection_probability.default_value",
		},
		{
			"Arm09_RpsThresholdRuntimeKey",
			parseRejectRpsThresholdRuntimeKey,
			"admission_control: rps_threshold.runtime_key is not yet supported; use rps_threshold.default_value",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("byte-stable wording drift:\n  const: %q\n   want: %q", tc.got, tc.want)
			}
		})
	}
}
