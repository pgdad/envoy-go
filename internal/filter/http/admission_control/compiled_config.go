// Package admission_control implements the envoy.filters.http.admission_control
// HTTP filter per phase-23 SPEC. This file lands the per-listener compiledConfig
// struct + the buildCompiledConfig parser body per phase-23 IMPL Task 2 covering
// the 9-arm PARSE-REJECT roster (4 RATIFIED-from-config arms per SPEC §5.1 + 5
// envoy-go-strict runtime_key arms per SPEC §5.2) + the AMEND-4 enabled-absent⇒
// ENABLED default + the http-range + grpc-code precompilation + the sampling_window
// ms/1000 rounding.
//
// # Package name
//
// Package directory + Go-package identifier are both `admission_control`
// (Go-style underscored; matches header_mutation + adaptive_concurrency precedent
// — the §9 filters with a multi-word canonical name; ADR-0114 stylistic license).
//
// # buildCompiledConfig — PARSE-REJECT discipline
//
// 9-arm roster per PD-2 byte-stable wording:
//
//  1. evaluation_criteria oneof absent (§5.1 row 1)
//  2. sr_threshold.default_value < 1.0% (§5.1 row 2)
//  3. http_success_status range invalid (§5.1 row 3)
//  4. grpc_success_status accepts at most 16 codes (§5.1 row 4)
//  5. enabled.runtime_key != "" (§5.2 row 1 + ADR-0195)
//  6. aggression.runtime_key != "" (§5.2 row 2 + ADR-0195)
//  7. sr_threshold.runtime_key != "" (§5.2 row 3 + ADR-0195)
//  8. max_rejection_probability.runtime_key != "" (§5.2 row 4 + ADR-0195)
//  9. rps_threshold.runtime_key != "" (§5.2 row 5 + ADR-0195)
//
// # Default-application semantics (per SPEC §6.1 + AMEND-4)
//
// After every PARSE-REJECT arm passes, defaults are applied to the un-set /
// zero proto fields:
//
//   - enabled absent entirely OR default_value BoolValue absent ⇒ true
//     (filter ON per AMEND-4 INVERSION vs phase-21 — the upstream
//     PROTOBUF_GET_WRAPPED_OR_DEFAULT(..., true) hard-codes true as the
//     fallback when the BoolValue wrapper is absent).
//   - sampling_window absent ⇒ 30s (default per config.cc:18); rounded to
//     whole seconds via integer ms/1000 (mirrors config.cc:33-35).
//   - aggression absent ⇒ 1.0; configured value floored to 1.0 (AMEND-1;
//     mirrors admission_control.cc:57-59 std::max<double>(1.0, configured)).
//   - sr_threshold absent ⇒ 95.0% → fraction 0.95; stored as
//     min(pct,100)/100 (mirrors admission_control.cc:61-64).
//   - rps_threshold absent ⇒ 0 (default per admission_control.cc:32).
//   - max_rejection_probability absent ⇒ 80.0% → fraction 0.80; stored as
//     pct/100 (mirrors admission_control.cc:70-74).
//   - http_criteria absent ⇒ default range {[100,500)} per AMEND-5
//     (success_criteria_evaluator.cc:43 — all codes < 500).
//   - grpc_criteria absent ⇒ default 11-code well-known set per AMEND-5
//     (success_criteria_evaluator.cc:57-69): {0,1,2,3,5,6,7,9,11,12,16}.
//
// # Cross-references
//
//   - ADR-0072 (boot-time-fail-fast)
//   - ADR-0080 (envoy-go-strict departure discipline + byte-stable error wording)
//   - ADR-0195 (RTDS runtime_key deferral PARSE-REJECT — the §Decision + §Consequences
//     bodies anchor at this commit per ADR-0044 in-place edit discipline)
//   - phase-21 adaptive_concurrency precedent:
//     internal/filter/http/adaptive_concurrency/compiled_config.go
//     (same buildCompiledConfig(*anypb.Any) shape; same PARSE-REJECT constant-
//     declaration discipline; same RTDS runtime_key deferral pattern per ADR-0187)
package admission_control

import (
	"errors"
	"fmt"
	"time"

	acv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/admission_control/v3"
	"google.golang.org/protobuf/types/known/anypb"
)

// compiledConfig is the per-listener compiled admission_control filter
// configuration per phase-23 SPEC §6.1. Populated by buildCompiledConfig at
// HCM-build time per ADR-0072 boot-time-fail-fast.
//
// Field grouping mirrors SPEC §6.1 verbatim for cross-referenceability.
type compiledConfig struct {
	// ---- Outer envelope ----

	// enabled gates the filter on/off per AdmissionControl.enabled.
	// Default TRUE when the proto field is absent per AMEND-4 (the upstream
	// PROTOBUF_GET_WRAPPED_OR_DEFAULT(..., true) hard-coded fallback —
	// OPPOSITE of phase-21 adaptive_concurrency which defaults false).
	enabled bool

	// ---- Numeric knobs (static default_value honored; runtime_key PARSE-REJECTed) ----

	// samplingWindow is the sliding window duration rounded to whole seconds
	// via integer ms/1000 (mirrors config.cc:33-35). Default 30s.
	samplingWindow time.Duration

	// aggression is the exponent applied to the rejection probability formula
	// (per AMEND-1). Floored to 1.0 (mirrors admission_control.cc:57-59
	// std::max<double>(1.0, configured)). Default 1.0.
	aggression float64

	// srThreshold is the success-rate threshold fraction min(pct,100)/100
	// below which the rejection probability becomes non-zero (per AMEND-1).
	// Default 0.95 (= 95.0 / 100.0). Boot-reject if configured < 0.01
	// (i.e. < 1.0%; §5.1 arm 2).
	srThreshold float64

	// rpsThreshold is the average-RPS gate below which the filter never
	// rejects (the suppression gate at admission_control.cc:87-91). Default 0
	// (gate never suppresses unless configured).
	rpsThreshold uint32

	// maxRejectionProbability is the upper-bound fraction pct/100 on the
	// computed P_reject (per AMEND-1). Default 0.80 (= 80.0 / 100.0).
	maxRejectionProbability float64

	// ---- success_criteria (the only evaluation_criteria oneof arm; both sub-arms compiled) ----

	// httpSuccessRanges is the precompiled set of half-open [start,end) HTTP
	// status-code ranges. Default {[100,500)} per AMEND-5.
	httpSuccessRanges []int32Range

	// grpcSuccessCodes is the precompiled set of gRPC status codes considered
	// successful. Default the 11-code well-known set per AMEND-5:
	// {0,1,2,3,5,6,7,9,11,12,16}.
	grpcSuccessCodes map[uint32]struct{}
}

// int32Range is a precompiled half-open [start,end) status-code range.
type int32Range struct {
	start, end int32
}

// isHTTPSuccess reports whether the given HTTP status code is a success per
// the configured httpSuccessRanges (half-open [start,end) semantics per
// AMEND-5 + success_criteria_evaluator.cc:28-44).
func (cc *compiledConfig) isHTTPSuccess(code int) bool {
	for _, r := range cc.httpSuccessRanges {
		if int32(code) >= r.start && int32(code) < r.end {
			return true
		}
	}
	return false
}

// isGRPCSuccess reports whether the given gRPC status code is a success per
// the configured grpcSuccessCodes (membership test per AMEND-5 +
// success_criteria_evaluator.cc:47-70).
func (cc *compiledConfig) isGRPCSuccess(status uint32) bool {
	_, ok := cc.grpcSuccessCodes[status]
	return ok
}

// -----------------------------------------------------------------------------
// PARSE-REJECT byte-stable wordings per PD-2 (PLAN lines, LOCKED).
// -----------------------------------------------------------------------------

const (
	// §5.1 RATIFIED-from-config arms (4)

	// parseRejectEvaluationCriteriaRequired is the §5.1 arm 1 wording for an
	// absent evaluation_criteria oneof (mirrors config.cc:49-50).
	parseRejectEvaluationCriteriaRequired = "admission_control: evaluation_criteria is required"

	// parseRejectSrThresholdTooLow is the §5.1 arm 2 wording for
	// sr_threshold.default_value < 1.0% (mirrors config.cc:25-27).
	parseRejectSrThresholdTooLow = "admission_control: sr_threshold cannot be less than 1.0%"

	// parseRejectHTTPRangeInvalid is the §5.1 arm 3 wording for an invalid
	// http_success_status range (mirrors success_criteria_evaluator.h:31-33).
	parseRejectHTTPRangeInvalid = "admission_control: http_success_status range invalid (must be within [100,600) and start<=end)"

	// parseRejectGRPCCodesExceed16 is the §5.1 arm 4 wording for
	// grpc_success_status list > 16 entries (mirrors
	// success_criteria_evaluator.cc:49-50).
	parseRejectGRPCCodesExceed16 = "admission_control: grpc_success_status accepts at most 16 codes"

	// §5.2 envoy-go-strict runtime_key arms (5)
	// Each arm mirrors "admission_control: <field>.runtime_key is not yet
	// supported; use <field>.default_value" per ADR-0195 + ADR-0080.

	// parseRejectEnabledRuntimeKey is the §5.2 arm for enabled.runtime_key != "".
	parseRejectEnabledRuntimeKey = "admission_control: enabled.runtime_key is not yet supported; use enabled.default_value"

	// parseRejectAggressionRuntimeKey is the §5.2 arm for aggression.runtime_key != "".
	parseRejectAggressionRuntimeKey = "admission_control: aggression.runtime_key is not yet supported; use aggression.default_value"

	// parseRejectSrThresholdRuntimeKey is the §5.2 arm for sr_threshold.runtime_key != "".
	parseRejectSrThresholdRuntimeKey = "admission_control: sr_threshold.runtime_key is not yet supported; use sr_threshold.default_value"

	// parseRejectMaxRejectionProbabilityRuntimeKey is the §5.2 arm for
	// max_rejection_probability.runtime_key != "".
	parseRejectMaxRejectionProbabilityRuntimeKey = "admission_control: max_rejection_probability.runtime_key is not yet supported; use max_rejection_probability.default_value"

	// parseRejectRpsThresholdRuntimeKey is the §5.2 arm for rps_threshold.runtime_key != "".
	parseRejectRpsThresholdRuntimeKey = "admission_control: rps_threshold.runtime_key is not yet supported; use rps_threshold.default_value"
)

// Default-application constants per SPEC §6.1 + AMEND-4 + AMEND-5.
const (
	defaultSamplingWindowSec   int64   = 30  // seconds (config.cc:18)
	defaultAggression          float64 = 1.0 // admission_control.cc:30
	defaultSrThresholdPct      float64 = 95.0
	defaultMaxRejectionProbPct float64 = 80.0
	httpRangeMin               int32   = 100
	httpRangeMax               int32   = 600 // exclusive upper bound for valid end values
	grpcMaxCodes                       = 16
)

// defaultGRPCSuccessCodes is the 11-code well-known gRPC success set per
// AMEND-5 (success_criteria_evaluator.cc:57-69):
//
//	Ok=0, Canceled=1, Unknown=2, InvalidArgument=3, NotFound=5,
//	AlreadyExists=6, Unauthenticated=7, FailedPrecondition=9,
//	OutOfRange=11, PermissionDenied=12, Unimplemented=16
var defaultGRPCSuccessCodes = []uint32{0, 1, 2, 3, 5, 6, 7, 9, 11, 12, 16}

// defaultHTTPSuccessRanges is the default half-open [100,500) range per
// AMEND-5 (success_criteria_evaluator.cc:43 — all codes < 500).
var defaultHTTPSuccessRanges = []int32Range{{start: 100, end: 500}}

// -----------------------------------------------------------------------------
// buildCompiledConfig — full proto-parser body per Task 2 + SPEC §6.1.
// -----------------------------------------------------------------------------

// buildCompiledConfig parses the AdmissionControl proto envelope (wrapped in
// an *anypb.Any) + constructs the per-listener compiledConfig per phase-23
// SPEC §6.1 + PD-2 byte-stable PARSE-REJECT wording.
//
// Per ADR-0072 boot-time-fail-fast: any validation failure returns the byte-
// stable error string verbatim (with a single non-byte-stable exception: the
// outer typed_config unmarshal failure path uses a wrapped error since the
// inner proto-decoder error message carries proto-level context useful for
// the operator's first-pass diagnostic — matches the phase-21
// adaptive_concurrency precedent at compiled_config.go).
//
// Arm ordering per SPEC §5.1 + §5.2:
//
//  1. unmarshal Any → *AdmissionControl
//  2. arm 1 — evaluation_criteria oneof required (§5.1)
//  3. arm 2 — sr_threshold.default_value >= 1.0% (§5.1)
//  4. arm 3 — http_success_status ranges valid, if present (§5.1)
//  5. arm 4 — grpc_success_status <= 16 codes, if present (§5.1)
//  6. arm 5 — enabled.runtime_key == "" (§5.2 + ADR-0195)
//  7. arm 6 — aggression.runtime_key == "" (§5.2 + ADR-0195)
//  8. arm 7 — sr_threshold.runtime_key == "" (§5.2 + ADR-0195)
//  9. arm 8 — max_rejection_probability.runtime_key == "" (§5.2 + ADR-0195)
//  10. arm 9 — rps_threshold.runtime_key == "" (§5.2 + ADR-0195)
//  11. apply defaults per SPEC §6.1 + AMEND-4 + AMEND-5
//  12. return populated *compiledConfig
func buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error) {
	if typedConfig == nil {
		return nil, errors.New("admission_control: typed_config required")
	}

	var ac acv3.AdmissionControl
	if err := typedConfig.UnmarshalTo(&ac); err != nil {
		// Non-byte-stable wording — the proto-decoder error carries useful
		// operator-facing context. Matches phase-21 adaptive_concurrency precedent.
		return nil, fmt.Errorf("admission_control: typed_config unmarshal: %w", err)
	}

	// Arm 1: evaluation_criteria oneof required.
	// The only concrete arm in v1.37.2 is SuccessCriteria; GetSuccessCriteria()
	// returns nil when the oneof is absent or set to a different arm type.
	if ac.GetEvaluationCriteria() == nil {
		return nil, errors.New(parseRejectEvaluationCriteriaRequired)
	}

	// Arm 2: sr_threshold.default_value >= 1.0% when present.
	// Check only when sr_threshold is set; absent ⇒ default 95% (> 1%).
	// The boot-reject threshold is the raw percentage value: < 1.0 (i.e. < 1.0%).
	// Mirrors config.cc:25-27 which rejects if default_value.value < 1.0.
	if srt := ac.GetSrThreshold(); srt != nil && srt.GetRuntimeKey() == "" {
		if dv := srt.GetDefaultValue(); dv != nil && dv.GetValue() < 1.0 {
			return nil, errors.New(parseRejectSrThresholdTooLow)
		}
	}

	// Arm 3: http_success_status ranges valid — start ≤ end; [100,600) bounds.
	// Mirrors success_criteria_evaluator.h:31-33. Checked only when http_criteria
	// is present; absent ⇒ default range {[100,500)} (always valid).
	if sc := ac.GetSuccessCriteria(); sc != nil {
		if hc := sc.GetHttpCriteria(); hc != nil {
			for _, r := range hc.GetHttpSuccessStatus() {
				if !isValidHTTPRange(r.GetStart(), r.GetEnd()) {
					return nil, errors.New(parseRejectHTTPRangeInvalid)
				}
			}
		}

		// Arm 4: grpc_success_status <= 16 codes.
		// Mirrors success_criteria_evaluator.cc:49-50.
		if gc := sc.GetGrpcCriteria(); gc != nil {
			if len(gc.GetGrpcSuccessStatus()) > grpcMaxCodes {
				return nil, errors.New(parseRejectGRPCCodesExceed16)
			}
		}
	}

	// Arm 5: enabled.runtime_key == "" required (ADR-0195).
	if en := ac.GetEnabled(); en != nil && en.GetRuntimeKey() != "" {
		return nil, errors.New(parseRejectEnabledRuntimeKey)
	}

	// Arm 6: aggression.runtime_key == "" required (ADR-0195).
	if agg := ac.GetAggression(); agg != nil && agg.GetRuntimeKey() != "" {
		return nil, errors.New(parseRejectAggressionRuntimeKey)
	}

	// Arm 7: sr_threshold.runtime_key == "" required (ADR-0195).
	// NOTE: sr_threshold nil ⇒ no runtime_key check needed (arm 2 already
	// only checks when present with empty runtime_key; here we check the key
	// independent of the default_value, since a set runtime_key must reject).
	if srt := ac.GetSrThreshold(); srt != nil && srt.GetRuntimeKey() != "" {
		return nil, errors.New(parseRejectSrThresholdRuntimeKey)
	}

	// Arm 8: max_rejection_probability.runtime_key == "" required (ADR-0195).
	if mrp := ac.GetMaxRejectionProbability(); mrp != nil && mrp.GetRuntimeKey() != "" {
		return nil, errors.New(parseRejectMaxRejectionProbabilityRuntimeKey)
	}

	// Arm 9: rps_threshold.runtime_key == "" required (ADR-0195).
	if rpt := ac.GetRpsThreshold(); rpt != nil && rpt.GetRuntimeKey() != "" {
		return nil, errors.New(parseRejectRpsThresholdRuntimeKey)
	}

	// All PARSE-REJECT arms passed — populate *compiledConfig with field
	// values + defaults per SPEC §6.1 + AMEND-4.
	cc := &compiledConfig{}

	// enabled (per AMEND-4 + §5.3):
	//   - absent entirely ⇒ true (PROTOBUF_GET_WRAPPED_OR_DEFAULT(...,true);
	//     runtime_protos.h:46 hard-codes true when BoolValue wrapper is absent)
	//   - present, default_value BoolValue absent ⇒ true (same mechanism)
	//   - present, default_value=false, runtime_key="" ⇒ false
	//   - present, default_value=true, runtime_key="" ⇒ true
	//   - runtime_key != "" ⇒ already PARSE-REJECTed above (arm 5)
	cc.enabled = true // AMEND-4 default: true when absent
	if en := ac.GetEnabled(); en != nil {
		if dv := en.GetDefaultValue(); dv != nil {
			cc.enabled = dv.GetValue()
		}
		// If default_value BoolValue is nil but enabled message is present,
		// cc.enabled stays true (the upstream PROTOBUF_GET_WRAPPED_OR_DEFAULT
		// fallback of true applies — the BoolValue wrapper being absent triggers
		// the ,true fallback regardless of whether the outer message is set).
	}

	// sampling_window: absent ⇒ default 30s; rounded to whole seconds via
	// integer ms/1000 (mirrors config.cc:33-35).
	if sw := ac.GetSamplingWindow(); sw != nil {
		ms := sw.AsDuration().Milliseconds()
		cc.samplingWindow = time.Duration(ms/1000) * time.Second
	} else {
		cc.samplingWindow = time.Duration(defaultSamplingWindowSec) * time.Second
	}

	// aggression: absent ⇒ default 1.0; configured value floored to 1.0
	// (per AMEND-1; mirrors admission_control.cc:57-59).
	cc.aggression = defaultAggression
	if agg := ac.GetAggression(); agg != nil {
		v := agg.GetDefaultValue()
		if v < 1.0 {
			v = 1.0
		}
		cc.aggression = v
	}

	// sr_threshold: absent ⇒ default 95.0% → fraction 0.95; stored as
	// min(pct,100)/100 (mirrors admission_control.cc:61-64).
	cc.srThreshold = defaultSrThresholdPct / 100.0
	if srt := ac.GetSrThreshold(); srt != nil {
		if dv := srt.GetDefaultValue(); dv != nil {
			pct := dv.GetValue()
			if pct > 100.0 {
				pct = 100.0
			}
			cc.srThreshold = pct / 100.0
		}
	}

	// rps_threshold: absent ⇒ default 0.
	// rpsThreshold defaults to 0 (zero value)
	if rpt := ac.GetRpsThreshold(); rpt != nil {
		cc.rpsThreshold = rpt.GetDefaultValue()
	}

	// max_rejection_probability: absent ⇒ default 80.0% → fraction 0.80;
	// stored as pct/100 (mirrors admission_control.cc:70-74).
	cc.maxRejectionProbability = defaultMaxRejectionProbPct / 100.0
	if mrp := ac.GetMaxRejectionProbability(); mrp != nil {
		if dv := mrp.GetDefaultValue(); dv != nil {
			cc.maxRejectionProbability = dv.GetValue() / 100.0
		}
	}

	// httpSuccessRanges: precompiled from http_criteria, or default {[100,500)}.
	cc.httpSuccessRanges = buildHTTPRanges(&ac)

	// grpcSuccessCodes: precompiled from grpc_criteria, or default 11-code set.
	cc.grpcSuccessCodes = buildGRPCCodes(&ac)

	return cc, nil
}

// isValidHTTPRange reports whether the half-open [start,end) range is valid:
// start within [100, 600) and end within [100, 600], with start ≤ end.
// Mirrors success_criteria_evaluator.h:31-33.
// Note: end==600 is allowed as the exclusive upper bound (valid: [500,600)).
func isValidHTTPRange(start, end int32) bool {
	return start >= httpRangeMin &&
		end <= httpRangeMax &&
		start <= end
}

// buildHTTPRanges precompiles the http_success_status ranges. Returns the
// default {[100,500)} when http_criteria is absent.
func buildHTTPRanges(ac *acv3.AdmissionControl) []int32Range {
	sc := ac.GetSuccessCriteria()
	if sc == nil {
		return append([]int32Range{}, defaultHTTPSuccessRanges...)
	}
	hc := sc.GetHttpCriteria()
	if hc == nil {
		return append([]int32Range{}, defaultHTTPSuccessRanges...)
	}
	raw := hc.GetHttpSuccessStatus()
	if len(raw) == 0 {
		return append([]int32Range{}, defaultHTTPSuccessRanges...)
	}
	ranges := make([]int32Range, len(raw))
	for i, r := range raw {
		ranges[i] = int32Range{start: r.GetStart(), end: r.GetEnd()}
	}
	return ranges
}

// buildGRPCCodes precompiles the grpc_success_status codes into a set.
// Returns the default 11-code well-known set when grpc_criteria is absent.
func buildGRPCCodes(ac *acv3.AdmissionControl) map[uint32]struct{} {
	sc := ac.GetSuccessCriteria()
	if sc == nil {
		return makeGRPCSet(defaultGRPCSuccessCodes)
	}
	gc := sc.GetGrpcCriteria()
	if gc == nil {
		return makeGRPCSet(defaultGRPCSuccessCodes)
	}
	codes := gc.GetGrpcSuccessStatus()
	if len(codes) == 0 {
		return makeGRPCSet(defaultGRPCSuccessCodes)
	}
	return makeGRPCSet(codes)
}

// makeGRPCSet builds a map[uint32]struct{} membership set from a slice of codes.
func makeGRPCSet(codes []uint32) map[uint32]struct{} {
	m := make(map[uint32]struct{}, len(codes))
	for _, c := range codes {
		m[c] = struct{}{}
	}
	return m
}
