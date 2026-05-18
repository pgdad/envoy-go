// Package adaptive_concurrency implements the envoy.filters.http.
// adaptive_concurrency HTTP filter per phase-21 SPEC. This file lands the
// per-listener compiledConfig struct + the buildCompiledConfig parser body
// per phase-21 IMPL Task 2 covering the 13-arm PARSE-REJECT roster + the
// default-application semantics per SPEC §6.1 + AMEND-4.
//
// # Package name
//
// Package directory + Go-package identifier are both `adaptive_concurrency`
// (Go-style underscored; matches phase-10 header_mutation precedent — the two
// §9 filters with a multi-word canonical name; ADR-0114 stylistic license).
//
// # buildCompiledConfig — PARSE-REJECT discipline
//
// 13-arm roster per planner-time D2 byte-stable wording:
//
//  1. concurrency_controller_config oneof required (§5.1 row 1)
//  2. concurrency_limit_params required (§5.1 row 2)
//  3. min_rtt_calc_params required (§5.1 row 3)
//  4. concurrency_update_interval must be > 0 (§5.1 row 4)
//  5. max_concurrency_limit must be > 0 (§5.1 row 5)
//  6. min_concurrency must be > 0 (§5.1 row 6)
//  7. request_count must be > 0 (§5.1 row 7)
//  8. min_rtt_calc_params.interval must be >= 1ms (§5.1 row 8 + AMEND-1 C3)
//  9. sample_aggregate_percentile must be in [0, 100] (§5.1 row 9)
//  10. jitter must be in [0, 100] (§5.1 row 10)
//  11. buffer must be in [0, 100] (§5.1 row 11)
//  12. enabled.runtime_key is not yet supported (§5.2 row 1 + ADR-0187)
//  13. min_rtt_calc_params.fixed_value is not yet supported (§5.2 row 2 +
//     AMEND-1 C4 + ADR-0186 §Consequences (d)) — STRUCTURALLY UNREACHABLE
//     in v1.32.4 go-control-plane proto bindings (the fixed_value field
//     was added to the MinimumRTTCalculationParams message in a later
//     proto revision; v1.32.4 exposes only Interval / RequestCount /
//     Jitter / MinConcurrency / Buffer). The byte-stable wording constant
//     is preserved here for the proto-bump migration path — when the
//     go-control-plane bumps to v1.37.x the call-site at step 8 will gain
//     the GetFixedValue() != nil branch ahead of the GetInterval() == nil
//     branch (interval-rejection arm 8 lands AFTER the fixed_value-arm 13
//     per the SPEC ordering note).
//
// # Default-application semantics (per SPEC §6.1 + AMEND-4)
//
// After every PARSE-REJECT arm passes, defaults are applied to the un-set /
// zero proto fields:
//
//   - enabled absent OR default_enabled == false (runtime_key == "") ⇒ false
//     (filter OFF per AMEND-4 REFUTATION of BRAINSTORM §2.1 — the upstream
//     RuntimeFeatureFlag.default_value proto-zero is BoolValue{value: false}).
//   - concurrency_limit_exceeded_status absent OR code == 0 ⇒ 503
//   - sample_aggregate_percentile absent ⇒ 0.50 (p50; stored as fraction)
//   - max_concurrency_limit absent ⇒ 1000
//   - min_rtt_calc_params.request_count absent ⇒ 50
//   - min_rtt_calc_params.jitter absent ⇒ 0.15 (15%; fraction)
//   - min_rtt_calc_params.min_concurrency absent ⇒ 3
//   - min_rtt_calc_params.buffer absent ⇒ 0.25 (25%; fraction)
//
// concurrency_update_interval + min_rtt_calc_params.interval are required at
// phase-21 MVP (no defaults — arms 4 + 8 PARSE-REJECT on absent / zero).
//
// # Cross-references
//
//   - ADR-0072 (boot-time-fail-fast)
//   - ADR-0080 (envoy-go-strict departure discipline + byte-stable error wording)
//   - ADR-0186 (Gradient-1 controller — paired ADR at SPEC commit; consumer
//     of the compiledConfig at Task 3 controller materialization)
//   - ADR-0187 (RTDS enabled.RuntimeFeatureFlag deferral PARSE-REJECT — the
//     §Decision + §Consequences bodies anchor at this commit per ADR-0044
//     in-place edit discipline)
//   - phase-20 oauth2 precedent: internal/filter/http/oauth2/compiled_config.go
//     (most-recent buildCompiledConfig template — same byte-stable PARSE-REJECT
//     constant-declaration discipline; this filter is structurally simpler —
//     no matchers, no SDS watchers, no AES key derivation)
package adaptive_concurrency

import (
	"errors"
	"fmt"
	"time"

	adaptive_concurrencyv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/adaptive_concurrency/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

// compiledConfig is the per-listener compiled adaptive_concurrency filter
// configuration per phase-21 SPEC §6.1. Populated by buildCompiledConfig at
// HCM-build time per ADR-0072 boot-time-fail-fast.
//
// Field grouping mirrors SPEC §6.1 verbatim for cross-referenceability.
type compiledConfig struct {
	// ---- Outer envelope ----

	// enabled gates the filter on/off per AdaptiveConcurrency.enabled.
	// Default FALSE when the proto field is absent per AMEND-4 (the upstream
	// RuntimeFeatureFlag.default_value proto-zero is BoolValue{value: false}).
	// REFUTES BRAINSTORM §2.1's "absent enabled = ON" hypothesis.
	enabled bool

	// concurrencyLimitExceededStatus is the wire status code returned on the
	// 503-overflow leg per AMEND-6. Default 503 when the proto field is absent
	// or the .code field is zero.
	concurrencyLimitExceededStatus uint32

	// ---- GradientControllerConfig sub-surface ----
	//
	// The only oneof arm at v1.37.2 per AMEND-1 C1 (no separate
	// gradient_controller.proto exists). The proto field is named
	// concurrency_limit_params per AMEND-1 C1 (renamed from
	// concurrency_limit_calculation_params).

	// sampleAggregatePercentile is the [0.0, 1.0] fraction used when
	// summarizing aggregated samples for minRTT recalc + concurrency update
	// per gradient_controller.cc:176-182. Default 0.50 (p50). Stored as
	// fraction (the proto carries [0, 100]; converted via /100).
	sampleAggregatePercentile float64

	// maxConcurrencyLimit is the upper bound on the calculated concurrency
	// limit per gradient_controller.cc:198-206. Default 1000.
	maxConcurrencyLimit uint32

	// concurrencyUpdateInterval is the period between concurrency-update ticks
	// per SPEC §4.2. Required at MVP (PARSE-REJECT if absent OR <= 0).
	concurrencyUpdateInterval time.Duration

	// ---- min_rtt_calc_params sub-block ----
	//
	// Per AMEND-1 C3 + C4: the fixed_value alternative arm is PARSE-REJECTed
	// per §5.3 + ADR-0186 §Consequences (d); interval is required at MVP
	// (PGV gte 1ms when set + envoy-go enforces presence since fixed_value
	// is rejected).

	// minRTTCalcInterval is the time between minRTT recalc windows per SPEC
	// §4.5. PGV >= 1ms when set; required at phase-21 (fixed_value
	// PARSE-REJECTed at arm 13 per AMEND-1 C4).
	minRTTCalcInterval time.Duration

	// minRTTRequestCount is the number of samples per minRTT window per
	// gradient_controller.cc:281-283. Default 50.
	minRTTRequestCount uint32

	// minRTTJitterPct is the [0.0, 1.0] fraction of the interval used as
	// additive jitter on the next-interval delay per AMEND-2 C2 +
	// gradient_controller.cc:152-160. Default 0.15 (15%). Stored as fraction.
	minRTTJitterPct float64

	// minRTTMinConcurrency is the concurrency limit pinned during the minRTT
	// sampling window per gradient_controller.cc:55-92. Default 3.
	minRTTMinConcurrency uint32

	// minRTTBufferPct is the [0.0, 1.0] fraction added to the measured minRTT
	// when computing the gradient per gradient_controller.cc:190-192. Default
	// 0.25 (25%). Stored as fraction.
	minRTTBufferPct float64
}

// -----------------------------------------------------------------------------
// PARSE-REJECT byte-stable wordings per planner-time D2 (PLAN lines 119-133).
// -----------------------------------------------------------------------------

const (
	parseRejectControllerOneofRequired   = "adaptive_concurrency: concurrency_controller_config oneof required"
	parseRejectConcurrencyLimitParamsReq = "adaptive_concurrency: concurrency_limit_params required"
	parseRejectMinRTTParamsRequired      = "adaptive_concurrency: min_rtt_calc_params required"
	parseRejectConcurrencyUpdateInterval = "adaptive_concurrency: concurrency_update_interval must be > 0"
	parseRejectMaxConcurrencyLimitZero   = "adaptive_concurrency: max_concurrency_limit must be > 0"
	parseRejectMinConcurrencyZero        = "adaptive_concurrency: min_concurrency must be > 0"
	parseRejectRequestCountZero          = "adaptive_concurrency: request_count must be > 0"
	parseRejectMinRTTIntervalTooSmall    = "adaptive_concurrency: min_rtt_calc_params.interval must be >= 1ms"
	parseRejectSampleAggregatePercentile = "adaptive_concurrency: sample_aggregate_percentile must be in [0, 100]"
	parseRejectJitterRange               = "adaptive_concurrency: jitter must be in [0, 100]"
	parseRejectBufferRange               = "adaptive_concurrency: buffer must be in [0, 100]"
	parseRejectEnabledRuntimeKey         = "adaptive_concurrency: enabled.runtime_key is not yet supported; use enabled.default_enabled"

	// parseRejectFixedValueDeferred is the byte-stable wording for arm 13 per
	// AMEND-1 C4 + ADR-0186 §Consequences (d). STRUCTURALLY UNREACHABLE in
	// v1.32.4 go-control-plane bindings (the fixed_value field is not exposed
	// on GradientControllerConfig_MinimumRTTCalculationParams in v1.32.4).
	// The constant is preserved verbatim for the proto-bump migration path —
	// when go-control-plane bumps to v1.37.x the buildCompiledConfig body will
	// gain a `if mrp.GetFixedValue() != nil { return nil, errors.New(
	// parseRejectFixedValueDeferred) }` arm preceding the interval-rejection
	// arm 8 per the SPEC ordering invariant (fixed_value check BEFORE
	// interval == nil check).
	parseRejectFixedValueDeferred = "adaptive_concurrency: min_rtt_calc_params.fixed_value is not yet supported; use min_rtt_calc_params.interval"
)

// Default-application constants per SPEC §6.1 + AMEND-4.
const (
	defaultConcurrencyLimitExceededStatus uint32  = 503
	defaultSampleAggregatePercentile      float64 = 0.50
	defaultMaxConcurrencyLimit            uint32  = 1000
	defaultMinRTTRequestCount             uint32  = 50
	defaultMinRTTJitterPct                float64 = 0.15
	defaultMinRTTMinConcurrency           uint32  = 3
	defaultMinRTTBufferPct                float64 = 0.25
)

// -----------------------------------------------------------------------------
// buildCompiledConfig — full proto-parser body per Task 2 + SPEC §6.1.
// -----------------------------------------------------------------------------

// buildCompiledConfig parses the AdaptiveConcurrency proto envelope (wrapped
// in an *anypb.Any) + constructs the per-listener compiledConfig per phase-21
// SPEC §6.1 + planner-time D2 byte-stable PARSE-REJECT wording.
//
// Per ADR-0072 boot-time-fail-fast: any validation failure returns the byte-
// stable error string verbatim (with a single non-byte-stable exception: the
// outer typed_config unmarshal failure path uses a wrapped error since the
// inner proto-decoder error message carries proto-level context useful for
// the operator's first-pass diagnostic — this matches the phase-20 oauth2
// precedent at internal/filter/http/oauth2/compiled_config.go).
//
// Arm ordering per SPEC §5.1 + §5.2 (with one deterministic deviation for
// the fixed_value-vs-interval pair when the proto exposes fixed_value — see
// the comment on parseRejectFixedValueDeferred):
//
//  1. unmarshal Any → *AdaptiveConcurrency
//  2. arm 1 — controller oneof required
//  3. arm 2 — concurrency_limit_params required
//  4. arm 3 — min_rtt_calc_params required
//  5. arm 4 — concurrency_update_interval > 0
//  6. arm 5 — max_concurrency_limit > 0 when set
//  7. arm 6 — min_concurrency > 0 when set
//  8. arm 7 — request_count > 0 when set
//  9. arm 13 — fixed_value PARSE-REJECT (unreachable in v1.32.4; preserved
//     in source for the proto-bump migration path)
//  10. arm 8 — interval >= 1ms (interval is required at MVP since fixed_value
//     is rejected; treat absent as PARSE-REJECT too)
//  11. arm 9 — sample_aggregate_percentile in [0, 100] when set
//  12. arm 10 — jitter in [0, 100] when set
//  13. arm 11 — buffer in [0, 100] when set
//  14. arm 12 — enabled.runtime_key == "" required
//  15. apply defaults per SPEC §6.1 + AMEND-4
//  16. return populated *compiledConfig
func buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error) {
	if typedConfig == nil {
		return nil, errors.New("adaptive_concurrency: typed_config required")
	}

	var ac adaptive_concurrencyv3.AdaptiveConcurrency
	if err := typedConfig.UnmarshalTo(&ac); err != nil {
		// Non-byte-stable wording — the proto-decoder error carries useful
		// operator-facing context (e.g. the failing field's wire-tag). Matches
		// phase-20 oauth2 precedent at compiled_config.go::extractOAuth2Config.
		return nil, fmt.Errorf("adaptive_concurrency: typed_config unmarshal: %w", err)
	}

	// Arm 1: concurrency_controller_config oneof required.
	if ac.GetConcurrencyControllerConfig() == nil {
		return nil, errors.New(parseRejectControllerOneofRequired)
	}
	// The only oneof arm at v1.32.4 + v1.37.x is gradient_controller_config
	// per AMEND-1 C1. GetGradientControllerConfig() returns nil if the active
	// arm is some other (future) controller type — collapse to the same
	// "controller required" wording per the oneof-required-message contract.
	gc := ac.GetGradientControllerConfig()
	if gc == nil {
		return nil, errors.New(parseRejectControllerOneofRequired)
	}

	// Arm 2: concurrency_limit_params required.
	clp := gc.GetConcurrencyLimitParams()
	if clp == nil {
		return nil, errors.New(parseRejectConcurrencyLimitParamsReq)
	}

	// Arm 3: min_rtt_calc_params required.
	mrp := gc.GetMinRttCalcParams()
	if mrp == nil {
		return nil, errors.New(parseRejectMinRTTParamsRequired)
	}

	// Arm 4: concurrency_update_interval must be > 0 (absent OR <= 0).
	cui := clp.GetConcurrencyUpdateInterval()
	if cui == nil || cui.AsDuration() <= 0 {
		return nil, errors.New(parseRejectConcurrencyUpdateInterval)
	}

	// Arm 5: max_concurrency_limit must be > 0 when set (absent ⇒ default).
	if mcl := clp.GetMaxConcurrencyLimit(); mcl != nil && mcl.GetValue() == 0 {
		return nil, errors.New(parseRejectMaxConcurrencyLimitZero)
	}

	// Arm 6: min_concurrency must be > 0 when set (absent ⇒ default).
	if mc := mrp.GetMinConcurrency(); mc != nil && mc.GetValue() == 0 {
		return nil, errors.New(parseRejectMinConcurrencyZero)
	}

	// Arm 7: request_count must be > 0 when set (absent ⇒ default).
	if rc := mrp.GetRequestCount(); rc != nil && rc.GetValue() == 0 {
		return nil, errors.New(parseRejectRequestCountZero)
	}

	// Arm 13: fixed_value PARSE-REJECT (per AMEND-1 C4 + ADR-0186 §Consequences
	// (d)). STRUCTURALLY UNREACHABLE in v1.32.4 — the GetFixedValue() method
	// does not exist on GradientControllerConfig_MinimumRTTCalculationParams
	// at this proto revision. The constant parseRejectFixedValueDeferred is
	// preserved verbatim for the future proto-bump migration; when the field
	// surfaces, insert the rejection branch HERE (before arm 8) per the SPEC
	// ordering invariant: fixed_value check BEFORE interval == nil check.
	//
	// Forward-pointer wiring sketch (commented for migration):
	//   if mrp.GetFixedValue() != nil {
	//       return nil, errors.New(parseRejectFixedValueDeferred)
	//   }

	// Arm 8: min_rtt_calc_params.interval must be >= 1ms.
	//
	// Treat absent interval as PARSE-REJECT too — at v1.32.4 fixed_value is
	// not on the proto so interval is the ONLY source of the minRTT recalc
	// trigger; absent interval ⇒ no minRTT source ⇒ filter cannot function.
	// Per the SPEC §5.1 row 8 wording the absent-arm collapses to the same
	// byte-stable "interval must be >= 1ms" message (the interval IS the
	// required field at phase-21 MVP per AMEND-1 C4).
	interval := mrp.GetInterval()
	if interval == nil || interval.AsDuration() < time.Millisecond {
		return nil, errors.New(parseRejectMinRTTIntervalTooSmall)
	}

	// Arm 9: sample_aggregate_percentile in [0, 100] when set.
	if sap := gc.GetSampleAggregatePercentile(); sap != nil {
		v := sap.GetValue()
		if v < 0 || v > 100 {
			return nil, errors.New(parseRejectSampleAggregatePercentile)
		}
	}

	// Arm 10: jitter in [0, 100] when set.
	if j := mrp.GetJitter(); j != nil {
		v := j.GetValue()
		if v < 0 || v > 100 {
			return nil, errors.New(parseRejectJitterRange)
		}
	}

	// Arm 11: buffer in [0, 100] when set.
	if b := mrp.GetBuffer(); b != nil {
		v := b.GetValue()
		if v < 0 || v > 100 {
			return nil, errors.New(parseRejectBufferRange)
		}
	}

	// Arm 12: enabled.runtime_key == "" required (RTDS deferred per ADR-0187).
	// Per the 5-case behavioral matrix in ADR-0187 §Context: cases 4 + 5
	// (runtime_key != "") PARSE-REJECT here; cases 1 + 2 + 3 are honored at
	// the default-application step below.
	if en := ac.GetEnabled(); en != nil && en.GetRuntimeKey() != "" {
		return nil, errors.New(parseRejectEnabledRuntimeKey)
	}

	// All PARSE-REJECT arms passed — populate *compiledConfig with field
	// values + defaults per SPEC §6.1 + AMEND-4.
	cc := &compiledConfig{}

	// enabled (per AMEND-4): absent OR default_enabled == false ⇒ filter OFF;
	// default_enabled == true ⇒ filter ON. The runtime_key != "" path has
	// already been PARSE-REJECTed above (arm 12).
	if en := ac.GetEnabled(); en != nil {
		if dv := en.GetDefaultValue(); dv != nil {
			cc.enabled = dv.GetValue()
		}
	}

	// concurrency_limit_exceeded_status (per AMEND-6 + SPEC §6.1 default 503):
	// absent OR .code == 0 ⇒ default 503. Note the proto's HttpStatus.code
	// field is the StatusCode enum (int32-typed); we cast to uint32 for the
	// wire-shape uint32-typed envoy-go contract.
	if hs := ac.GetConcurrencyLimitExceededStatus(); hs != nil {
		if code := int32(hs.GetCode()); code != 0 {
			cc.concurrencyLimitExceededStatus = uint32(code)
		}
	}
	if cc.concurrencyLimitExceededStatus == 0 {
		cc.concurrencyLimitExceededStatus = defaultConcurrencyLimitExceededStatus
	}

	// sample_aggregate_percentile (per SPEC §6.1 default 0.50 = p50). Stored
	// as fraction in [0.0, 1.0]; the proto carries [0, 100] (the type.v3.
	// Percent.value field is a double in [0, 100]).
	if sap := gc.GetSampleAggregatePercentile(); sap != nil {
		cc.sampleAggregatePercentile = sap.GetValue() / 100.0
	} else {
		cc.sampleAggregatePercentile = defaultSampleAggregatePercentile
	}

	// max_concurrency_limit (per SPEC §6.1 default 1000).
	if mcl := clp.GetMaxConcurrencyLimit(); mcl != nil {
		cc.maxConcurrencyLimit = mcl.GetValue()
	} else {
		cc.maxConcurrencyLimit = defaultMaxConcurrencyLimit
	}

	// concurrency_update_interval (no default; required by arm 4).
	cc.concurrencyUpdateInterval = cui.AsDuration()

	// min_rtt_calc_params.interval (no default; required by arm 8).
	cc.minRTTCalcInterval = interval.AsDuration()

	// min_rtt_calc_params.request_count (per SPEC §6.1 default 50).
	if rc := mrp.GetRequestCount(); rc != nil {
		cc.minRTTRequestCount = rc.GetValue()
	} else {
		cc.minRTTRequestCount = defaultMinRTTRequestCount
	}

	// min_rtt_calc_params.jitter (per SPEC §6.1 default 0.15 = 15%). Stored
	// as fraction in [0.0, 1.0].
	if j := mrp.GetJitter(); j != nil {
		cc.minRTTJitterPct = j.GetValue() / 100.0
	} else {
		cc.minRTTJitterPct = defaultMinRTTJitterPct
	}

	// min_rtt_calc_params.min_concurrency (per SPEC §6.1 default 3).
	if mc := mrp.GetMinConcurrency(); mc != nil {
		cc.minRTTMinConcurrency = mc.GetValue()
	} else {
		cc.minRTTMinConcurrency = defaultMinRTTMinConcurrency
	}

	// min_rtt_calc_params.buffer (per SPEC §6.1 default 0.25 = 25%). Stored
	// as fraction in [0.0, 1.0].
	if b := mrp.GetBuffer(); b != nil {
		cc.minRTTBufferPct = b.GetValue() / 100.0
	} else {
		cc.minRTTBufferPct = defaultMinRTTBufferPct
	}

	return cc, nil
}
