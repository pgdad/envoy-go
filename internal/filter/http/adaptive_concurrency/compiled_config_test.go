package adaptive_concurrency

import (
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	adaptive_concurrencyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/adaptive_concurrency/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// validConfig returns a fully-populated AdaptiveConcurrency proto with every
// required field set to a defaults-friendly value. Each PARSE-REJECT-row test
// `mutate` closure modifies this baseline to drive a specific arm.
func validConfig() *adaptive_concurrencyv3.AdaptiveConcurrency {
	return &adaptive_concurrencyv3.AdaptiveConcurrency{
		ConcurrencyControllerConfig: &adaptive_concurrencyv3.AdaptiveConcurrency_GradientControllerConfig{
			GradientControllerConfig: &adaptive_concurrencyv3.GradientControllerConfig{
				SampleAggregatePercentile: &typev3.Percent{Value: 50.0},
				ConcurrencyLimitParams: &adaptive_concurrencyv3.GradientControllerConfig_ConcurrencyLimitCalculationParams{
					MaxConcurrencyLimit:       &wrapperspb.UInt32Value{Value: 1000},
					ConcurrencyUpdateInterval: durationpb.New(100 * time.Millisecond),
				},
				MinRttCalcParams: &adaptive_concurrencyv3.GradientControllerConfig_MinimumRTTCalculationParams{
					Interval:       durationpb.New(30 * time.Second),
					RequestCount:   &wrapperspb.UInt32Value{Value: 50},
					Jitter:         &typev3.Percent{Value: 15.0},
					MinConcurrency: &wrapperspb.UInt32Value{Value: 3},
					Buffer:         &typev3.Percent{Value: 25.0},
				},
			},
		},
		Enabled: &corev3.RuntimeFeatureFlag{
			DefaultValue: &wrapperspb.BoolValue{Value: true},
			RuntimeKey:   "",
		},
		ConcurrencyLimitExceededStatus: &typev3.HttpStatus{
			Code: typev3.StatusCode_ServiceUnavailable, // 503
		},
	}
}

// toAny wraps the AdaptiveConcurrency proto in an *anypb.Any envelope per the
// buildCompiledConfig signature contract.
func toAny(t *testing.T, ac *adaptive_concurrencyv3.AdaptiveConcurrency) *anypb.Any {
	t.Helper()
	any, err := anypb.New(ac)
	if err != nil {
		t.Fatalf("anypb.New failed: %v", err)
	}
	return any
}

// -----------------------------------------------------------------------------
// TestBuildCompiledConfig — table-driven PARSE-REJECT roster + happy-path +
// default-applied coverage per phase-21 SPEC §14.1 + D2 + D5.
// -----------------------------------------------------------------------------

func TestBuildCompiledConfig(t *testing.T) {
	t.Run("PARSE_REJECT", testBuildCompiledConfigParseReject)
	t.Run("Defaults", testBuildCompiledConfigDefaults)
	t.Run("HappyPath", testBuildCompiledConfigHappyPath)
	t.Run("NilTypedConfig", testBuildCompiledConfigNilTypedConfig)
	t.Run("UnmarshalFailure", testBuildCompiledConfigUnmarshalFailure)
}

// testBuildCompiledConfigParseReject drives each of the 12 reachable
// PARSE-REJECT arms (arm 13 fixed_value is structurally unreachable in
// v1.32.4 — see compiled_config.go::parseRejectFixedValueDeferred). Each row
// mutates the baseline validConfig() to trigger ONE specific arm + asserts
// the returned err.Error() matches the byte-stable wording exactly.
func testBuildCompiledConfigParseReject(t *testing.T) {
	cases := []struct {
		name      string
		mutate    func(*adaptive_concurrencyv3.AdaptiveConcurrency)
		wantErrEq string
	}{
		// ---- Arm 1: controller oneof required ----
		{
			name: "Arm01_ControllerOneof_Absent",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.ConcurrencyControllerConfig = nil
			},
			wantErrEq: parseRejectControllerOneofRequired,
		},
		// Note: a `&AdaptiveConcurrency_GradientControllerConfig{GradientControllerConfig: nil}`
		// mutate row would NOT trigger arm 1 reliably — proto Any-round-trip
		// normalizes the nil inner-message to an empty `&GradientControllerConfig{}`,
		// which then surfaces as arm 2 (concurrency_limit_params required) since
		// the empty controller has no nested limit-params. Arm 1's reachable
		// trigger is `ConcurrencyControllerConfig == nil` only.

		// ---- Arm 2: concurrency_limit_params required ----
		{
			name: "Arm02_ConcurrencyLimitParams_Absent",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().ConcurrencyLimitParams = nil
			},
			wantErrEq: parseRejectConcurrencyLimitParamsReq,
		},

		// ---- Arm 3: min_rtt_calc_params required ----
		{
			name: "Arm03_MinRTTCalcParams_Absent",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().MinRttCalcParams = nil
			},
			wantErrEq: parseRejectMinRTTParamsRequired,
		},

		// ---- Arm 4: concurrency_update_interval > 0 ----
		{
			name: "Arm04_ConcurrencyUpdateInterval_Absent",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetConcurrencyLimitParams().ConcurrencyUpdateInterval = nil
			},
			wantErrEq: parseRejectConcurrencyUpdateInterval,
		},
		{
			name: "Arm04_ConcurrencyUpdateInterval_Zero",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetConcurrencyLimitParams().ConcurrencyUpdateInterval = durationpb.New(0)
			},
			wantErrEq: parseRejectConcurrencyUpdateInterval,
		},
		{
			name: "Arm04_ConcurrencyUpdateInterval_Negative",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetConcurrencyLimitParams().ConcurrencyUpdateInterval = durationpb.New(-1 * time.Second)
			},
			wantErrEq: parseRejectConcurrencyUpdateInterval,
		},

		// ---- Arm 5: max_concurrency_limit > 0 when set ----
		{
			name: "Arm05_MaxConcurrencyLimit_Zero",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetConcurrencyLimitParams().MaxConcurrencyLimit = &wrapperspb.UInt32Value{Value: 0}
			},
			wantErrEq: parseRejectMaxConcurrencyLimitZero,
		},

		// ---- Arm 6: min_concurrency > 0 when set ----
		{
			name: "Arm06_MinConcurrency_Zero",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().MinConcurrency = &wrapperspb.UInt32Value{Value: 0}
			},
			wantErrEq: parseRejectMinConcurrencyZero,
		},

		// ---- Arm 7: request_count > 0 when set ----
		{
			name: "Arm07_RequestCount_Zero",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().RequestCount = &wrapperspb.UInt32Value{Value: 0}
			},
			wantErrEq: parseRejectRequestCountZero,
		},

		// ---- Arm 8: min_rtt_calc_params.interval >= 1ms (absent treated
		// as PARSE-REJECT since fixed_value is rejected at v1.37.x; at
		// v1.32.4 fixed_value doesn't exist so interval is the only source). ----
		{
			name: "Arm08_MinRTTInterval_Absent",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().Interval = nil
			},
			wantErrEq: parseRejectMinRTTIntervalTooSmall,
		},
		{
			name: "Arm08_MinRTTInterval_500us",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().Interval = durationpb.New(500 * time.Microsecond)
			},
			wantErrEq: parseRejectMinRTTIntervalTooSmall,
		},
		{
			name: "Arm08_MinRTTInterval_Zero",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().Interval = durationpb.New(0)
			},
			wantErrEq: parseRejectMinRTTIntervalTooSmall,
		},

		// ---- Arm 9: sample_aggregate_percentile in [0, 100] when set ----
		{
			name: "Arm09_SampleAggregatePercentile_Negative",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().SampleAggregatePercentile = &typev3.Percent{Value: -0.1}
			},
			wantErrEq: parseRejectSampleAggregatePercentile,
		},
		{
			name: "Arm09_SampleAggregatePercentile_GreaterThan100",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().SampleAggregatePercentile = &typev3.Percent{Value: 100.5}
			},
			wantErrEq: parseRejectSampleAggregatePercentile,
		},

		// ---- Arm 10: jitter in [0, 100] when set ----
		{
			name: "Arm10_Jitter_Negative",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().Jitter = &typev3.Percent{Value: -1.0}
			},
			wantErrEq: parseRejectJitterRange,
		},
		{
			name: "Arm10_Jitter_GreaterThan100",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().Jitter = &typev3.Percent{Value: 150.0}
			},
			wantErrEq: parseRejectJitterRange,
		},

		// ---- Arm 11: buffer in [0, 100] when set ----
		{
			name: "Arm11_Buffer_Negative",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().Buffer = &typev3.Percent{Value: -5.0}
			},
			wantErrEq: parseRejectBufferRange,
		},
		{
			name: "Arm11_Buffer_GreaterThan100",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().Buffer = &typev3.Percent{Value: 101.0}
			},
			wantErrEq: parseRejectBufferRange,
		},

		// ---- Arm 12: enabled.runtime_key != "" PARSE-REJECT (ADR-0187 cases 4 + 5) ----
		{
			name: "Arm12_EnabledRuntimeKey_DefaultFalse",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: &wrapperspb.BoolValue{Value: false},
					RuntimeKey:   "adaptive_concurrency.enabled",
				}
			},
			wantErrEq: parseRejectEnabledRuntimeKey,
		},
		{
			name: "Arm12_EnabledRuntimeKey_DefaultTrue",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: &wrapperspb.BoolValue{Value: true},
					RuntimeKey:   "adaptive_concurrency.enabled",
				}
			},
			wantErrEq: parseRejectEnabledRuntimeKey,
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
				t.Fatalf("error mismatch:\n  want: %q\n  got:  %q", tc.wantErrEq, err.Error())
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
		mutate func(*adaptive_concurrencyv3.AdaptiveConcurrency)
		assert func(t *testing.T, cc *compiledConfig)
	}{
		// ---- enabled defaults (AMEND-4 — REFUTES BRAINSTORM §2.1) ----
		{
			name: "Enabled_Absent_DefaultsToFalse",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.Enabled = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.enabled {
					t.Fatalf("expected enabled=false (AMEND-4 default OFF when absent), got true")
				}
			},
		},
		{
			name: "Enabled_DefaultValueFalse_StaysFalse",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: &wrapperspb.BoolValue{Value: false},
					RuntimeKey:   "",
				}
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.enabled {
					t.Fatalf("expected enabled=false (default_enabled=false), got true")
				}
			},
		},
		{
			name: "Enabled_DefaultValueAbsent_DefaultsToFalse",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.Enabled = &corev3.RuntimeFeatureFlag{
					DefaultValue: nil,
					RuntimeKey:   "",
				}
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.enabled {
					t.Fatalf("expected enabled=false (default_value absent), got true")
				}
			},
		},

		// ---- concurrency_limit_exceeded_status default 503 ----
		{
			name: "ConcurrencyLimitExceededStatus_Absent_Defaults503",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.ConcurrencyLimitExceededStatus = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.concurrencyLimitExceededStatus != 503 {
					t.Fatalf("expected concurrencyLimitExceededStatus=503, got %d", cc.concurrencyLimitExceededStatus)
				}
			},
		},
		{
			name: "ConcurrencyLimitExceededStatus_CodeZero_Defaults503",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.ConcurrencyLimitExceededStatus = &typev3.HttpStatus{Code: typev3.StatusCode_Empty}
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.concurrencyLimitExceededStatus != 503 {
					t.Fatalf("expected concurrencyLimitExceededStatus=503 (code=0 → default), got %d", cc.concurrencyLimitExceededStatus)
				}
			},
		},

		// ---- sample_aggregate_percentile default 0.50 (p50; fraction) ----
		{
			name: "SampleAggregatePercentile_Absent_DefaultsToHalf",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().SampleAggregatePercentile = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.sampleAggregatePercentile != 0.50 {
					t.Fatalf("expected sampleAggregatePercentile=0.50, got %v", cc.sampleAggregatePercentile)
				}
			},
		},

		// ---- max_concurrency_limit default 1000 ----
		{
			name: "MaxConcurrencyLimit_Absent_Defaults1000",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetConcurrencyLimitParams().MaxConcurrencyLimit = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.maxConcurrencyLimit != 1000 {
					t.Fatalf("expected maxConcurrencyLimit=1000, got %d", cc.maxConcurrencyLimit)
				}
			},
		},

		// ---- min_rtt_calc_params.request_count default 50 ----
		{
			name: "RequestCount_Absent_Defaults50",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().RequestCount = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.minRTTRequestCount != 50 {
					t.Fatalf("expected minRTTRequestCount=50, got %d", cc.minRTTRequestCount)
				}
			},
		},

		// ---- min_rtt_calc_params.jitter default 0.15 (15%; fraction) ----
		{
			name: "Jitter_Absent_Defaults015",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().Jitter = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.minRTTJitterPct != 0.15 {
					t.Fatalf("expected minRTTJitterPct=0.15, got %v", cc.minRTTJitterPct)
				}
			},
		},

		// ---- min_rtt_calc_params.min_concurrency default 3 ----
		{
			name: "MinConcurrency_Absent_Defaults3",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().MinConcurrency = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.minRTTMinConcurrency != 3 {
					t.Fatalf("expected minRTTMinConcurrency=3, got %d", cc.minRTTMinConcurrency)
				}
			},
		},

		// ---- min_rtt_calc_params.buffer default 0.25 (25%; fraction) ----
		{
			name: "Buffer_Absent_Defaults025",
			mutate: func(ac *adaptive_concurrencyv3.AdaptiveConcurrency) {
				ac.GetGradientControllerConfig().GetMinRttCalcParams().Buffer = nil
			},
			assert: func(t *testing.T, cc *compiledConfig) {
				if cc.minRTTBufferPct != 0.25 {
					t.Fatalf("expected minRTTBufferPct=0.25, got %v", cc.minRTTBufferPct)
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
		t.Errorf("expected enabled=true (default_enabled=true in baseline), got false")
	}
	if cc.concurrencyLimitExceededStatus != 503 {
		t.Errorf("expected concurrencyLimitExceededStatus=503, got %d", cc.concurrencyLimitExceededStatus)
	}
	if cc.sampleAggregatePercentile != 0.50 {
		t.Errorf("expected sampleAggregatePercentile=0.50 (50/100 fraction), got %v", cc.sampleAggregatePercentile)
	}
	if cc.maxConcurrencyLimit != 1000 {
		t.Errorf("expected maxConcurrencyLimit=1000, got %d", cc.maxConcurrencyLimit)
	}
	if cc.concurrencyUpdateInterval != 100*time.Millisecond {
		t.Errorf("expected concurrencyUpdateInterval=100ms, got %v", cc.concurrencyUpdateInterval)
	}
	if cc.minRTTCalcInterval != 30*time.Second {
		t.Errorf("expected minRTTCalcInterval=30s, got %v", cc.minRTTCalcInterval)
	}
	if cc.minRTTRequestCount != 50 {
		t.Errorf("expected minRTTRequestCount=50, got %d", cc.minRTTRequestCount)
	}
	if cc.minRTTJitterPct != 0.15 {
		t.Errorf("expected minRTTJitterPct=0.15 (15/100 fraction), got %v", cc.minRTTJitterPct)
	}
	if cc.minRTTMinConcurrency != 3 {
		t.Errorf("expected minRTTMinConcurrency=3, got %d", cc.minRTTMinConcurrency)
	}
	if cc.minRTTBufferPct != 0.25 {
		t.Errorf("expected minRTTBufferPct=0.25 (25/100 fraction), got %v", cc.minRTTBufferPct)
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
	const want = "adaptive_concurrency: typed_config required"
	if err.Error() != want {
		t.Fatalf("error mismatch:\n  want: %q\n  got:  %q", want, err.Error())
	}
}

// testBuildCompiledConfigUnmarshalFailure drives the proto-unmarshal-failure
// path. This is the ONE intentional exception to the byte-stable wording
// discipline (per the phase-20 oauth2 precedent); we assert only the prefix.
func testBuildCompiledConfigUnmarshalFailure(t *testing.T) {
	// Construct an *anypb.Any with the AdaptiveConcurrency TypeUrl but
	// malformed wire bytes — UnmarshalTo will fail.
	bad := &anypb.Any{
		TypeUrl: "type.googleapis.com/envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency",
		Value:   []byte{0xff, 0xff, 0xff, 0xff, 0xff},
	}
	cc, err := buildCompiledConfig(bad)
	if err == nil {
		t.Fatalf("expected unmarshal error, got nil (cc=%+v)", cc)
	}
	if cc != nil {
		t.Fatalf("expected nil *compiledConfig on unmarshal failure, got %+v", cc)
	}
	const wantPrefix = "adaptive_concurrency: typed_config unmarshal:"
	if got := err.Error(); len(got) < len(wantPrefix) || got[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("error prefix mismatch:\n  want prefix: %q\n  got:         %q", wantPrefix, got)
	}
}

// TestParseRejectConstants_ByteStable pins the 13 byte-stable PARSE-REJECT
// wordings (including the structurally-unreachable arm 13 constant for the
// proto-bump migration path). Per planner-time D2: NO format-string drift;
// constants asserted byte-exact against the planner-time D2 reference roster.
func TestParseRejectConstants_ByteStable(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Arm01", parseRejectControllerOneofRequired, "adaptive_concurrency: concurrency_controller_config oneof required"},
		{"Arm02", parseRejectConcurrencyLimitParamsReq, "adaptive_concurrency: concurrency_limit_params required"},
		{"Arm03", parseRejectMinRTTParamsRequired, "adaptive_concurrency: min_rtt_calc_params required"},
		{"Arm04", parseRejectConcurrencyUpdateInterval, "adaptive_concurrency: concurrency_update_interval must be > 0"},
		{"Arm05", parseRejectMaxConcurrencyLimitZero, "adaptive_concurrency: max_concurrency_limit must be > 0"},
		{"Arm06", parseRejectMinConcurrencyZero, "adaptive_concurrency: min_concurrency must be > 0"},
		{"Arm07", parseRejectRequestCountZero, "adaptive_concurrency: request_count must be > 0"},
		{"Arm08", parseRejectMinRTTIntervalTooSmall, "adaptive_concurrency: min_rtt_calc_params.interval must be >= 1ms"},
		{"Arm09", parseRejectSampleAggregatePercentile, "adaptive_concurrency: sample_aggregate_percentile must be in [0, 100]"},
		{"Arm10", parseRejectJitterRange, "adaptive_concurrency: jitter must be in [0, 100]"},
		{"Arm11", parseRejectBufferRange, "adaptive_concurrency: buffer must be in [0, 100]"},
		{"Arm12", parseRejectEnabledRuntimeKey, "adaptive_concurrency: enabled.runtime_key is not yet supported; use enabled.default_enabled"},
		{"Arm13", parseRejectFixedValueDeferred, "adaptive_concurrency: min_rtt_calc_params.fixed_value is not yet supported; use min_rtt_calc_params.interval"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("byte-stable wording drift:\n  const: %q\n  want:  %q", tc.got, tc.want)
			}
		})
	}
}
