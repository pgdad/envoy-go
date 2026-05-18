package adaptive_concurrency

// adaptive_concurrency_test.go — Group 9 boot-time factory + stat-name
// regression-guard tests per phase-21 SPEC §6.6 + §14.1 + planner-time D5.
//
// Covers five scenarios:
//
//  1. TestTypeURL_ByteExact — pins the TypeURL constant byte-exact per
//     ADR-0143 SN1 (regression on the constant surfaces immediately).
//  2. TestNew_NilTypedConfig_Error — the boot-time-fail-fast ADR-0072 path.
//  3. TestNew_HappyPath_ReturnsFactory — valid config → non-nil factory;
//     invoking the factory returns HTTPFilter{Decoder: f, Encoder: f}
//     (both non-nil since adaptive_concurrency participates on both sides
//     per SPEC §3.4).
//  4. TestNew_ParseRejectPropagates — invalid config → byte-stable
//     PARSE-REJECT error from buildCompiledConfig propagates verbatim.
//  5. TestStatNames_Equal_Wire — table-driven D5 closure: each of the 7
//     statName* constants byte-exact-matches its wire name.

import (
	"testing"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// 1. TestTypeURL_ByteExact — ADR-0143 SN1 byte-exact pin.
// -----------------------------------------------------------------------------

// TestTypeURL_ByteExact pins the TypeURL constant byte-exact per ADR-0143 SN1.
// The wire-name "type.googleapis.com/envoy.extensions.filters.http.
// adaptive_concurrency.v3.AdaptiveConcurrency" is consumed by the HCM
// filter-chain builder to resolve typed_config Any envelopes against the
// HTTPRegistry's frozen factory map; any drift here surfaces as a runtime
// "no factory registered for type URL" error rather than a build-time
// failure. The compile-time constant assertion below catches the drift at
// `go test` time.
func TestTypeURL_ByteExact(t *testing.T) {
	const want = "type.googleapis.com/envoy.extensions.filters.http.adaptive_concurrency.v3.AdaptiveConcurrency"
	if TypeURL != want {
		t.Errorf("TypeURL drift: got %q; want %q", TypeURL, want)
	}
}

// -----------------------------------------------------------------------------
// 2. TestNew_NilTypedConfig_Error — boot-time-fail-fast (ADR-0072).
// -----------------------------------------------------------------------------

// TestNew_NilTypedConfig_Error verifies the boot-time-fail-fast ADR-0072
// path: a nil typed_config returns (nil, err) with the byte-stable
// "adaptive_concurrency: typed_config required" wording (mirrors the
// equivalent buildCompiledConfig nil-Any path).
func TestNew_NilTypedConfig_Error(t *testing.T) {
	factory, err := New(nil, envoyhttp.FactoryCtx{Stats: stats.NewRegistry(), StatPrefix: "http.test"})
	if err == nil {
		t.Fatal("New(nil, ctx): got nil err; want error")
	}
	if factory != nil {
		t.Errorf("New(nil, ctx): got non-nil factory; want nil")
	}
	const want = "adaptive_concurrency: typed_config required"
	if err.Error() != want {
		t.Errorf("err.Error() = %q; want %q", err.Error(), want)
	}
}

// -----------------------------------------------------------------------------
// 3. TestNew_HappyPath_ReturnsFactory — valid config → BOTH-sides filter.
// -----------------------------------------------------------------------------

// TestNew_HappyPath_ReturnsFactory verifies that New with a valid
// *anypb.Any returns a non-nil FilterInstanceFactory; invoking the factory
// returns an HTTPFilter with Decoder=f AND Encoder=f (both non-nil since
// adaptive_concurrency participates on both sides per SPEC §3.4 + AMEND-6).
// Both Decoder and Encoder point at the SAME *filter instance (the
// *filter struct implements both StreamDecoderFilter and StreamEncoderFilter
// per filter.go's var-block conformance assertions).
func TestNew_HappyPath_ReturnsFactory(t *testing.T) {
	any := toAny(t, validConfig())
	reg := stats.NewRegistry()
	factory, err := New(any, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "http.test"})
	if err != nil {
		t.Fatalf("New: got err=%v; want nil", err)
	}
	if factory == nil {
		t.Fatal("New: got nil factory; want non-nil")
	}
	hf := factory()
	if hf.Name != filterName {
		t.Errorf("HTTPFilter.Name = %q; want %q", hf.Name, filterName)
	}
	if hf.Decoder == nil {
		t.Error("HTTPFilter.Decoder: got nil; want non-nil (both-sides filter per SPEC §3.4)")
	}
	if hf.Encoder == nil {
		t.Error("HTTPFilter.Encoder: got nil; want non-nil (both-sides filter per SPEC §3.4)")
	}
	// Both sides must point at the SAME *filter instance per the var-block
	// conformance assertions at filter.go. Cross-assert by type-asserting
	// and comparing pointers.
	df, ok := hf.Decoder.(*filter)
	if !ok {
		t.Fatalf("HTTPFilter.Decoder type: got %T; want *filter", hf.Decoder)
	}
	ef, ok := hf.Encoder.(*filter)
	if !ok {
		t.Fatalf("HTTPFilter.Encoder type: got %T; want *filter", hf.Encoder)
	}
	if df != ef {
		t.Errorf("Decoder + Encoder point at different *filter instances; want same instance per SPEC §3.4")
	}
	// Per-filter state must be populated.
	if df.cc == nil {
		t.Error("*filter.cc: got nil; want non-nil *compiledConfig")
	}
	if df.controller == nil {
		t.Error("*filter.controller: got nil; want non-nil *gradientController")
	}
	if df.clock == nil {
		t.Error("*filter.clock: got nil; want defaultClock{}")
	}
	// Second invocation of the factory should produce a DIFFERENT *filter
	// instance (per-stream allocation) but the SAME shared *controller pointer
	// (one controller per HCM filter chain mounting the filter).
	hf2 := factory()
	df2 := hf2.Decoder.(*filter)
	if df == df2 {
		t.Error("factory() returned same *filter instance across invocations; want fresh per-stream instance")
	}
	if df.controller != df2.controller {
		t.Error("per-stream *filter holds different *gradientController pointers; want shared per-HCM-instance pointer")
	}
	if df.cc != df2.cc {
		t.Error("per-stream *filter holds different *compiledConfig pointers; want shared pointer")
	}
}

// TestNew_NilStats_Error verifies the boot-time-fail-fast nil-stats path:
// ctx.Stats == nil → New returns the byte-stable
// "adaptive_concurrency: ctx.Stats required ..." error. Production callers
// per internal/filter/hcm/config.go always supply a non-nil registry per
// ADR-0061 LBP-1; this test pins the failure mode for misconfigured test
// harnesses.
func TestNew_NilStats_Error(t *testing.T) {
	any := toAny(t, validConfig())
	factory, err := New(any, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("New with nil ctx.Stats: got nil err; want error")
	}
	if factory != nil {
		t.Errorf("New with nil ctx.Stats: got non-nil factory; want nil")
	}
	const want = "adaptive_concurrency: ctx.Stats required (HCM-build-time non-nil per ADR-0061 LBP-1)"
	if err.Error() != want {
		t.Errorf("err.Error() = %q; want %q", err.Error(), want)
	}
}

// -----------------------------------------------------------------------------
// 4. TestNew_ParseRejectPropagates — buildCompiledConfig err propagates verbatim.
// -----------------------------------------------------------------------------

// TestNew_ParseRejectPropagates verifies that an invalid config (arm 1 —
// controller oneof required) returns the byte-stable PARSE-REJECT error
// from buildCompiledConfig verbatim. Pins the wrapping-discipline contract:
// New does NOT additionally wrap the inner buildCompiledConfig error; the
// byte-stable wording surfaces unchanged to the caller (the HCM
// filter-chain builder + onward to the operator-facing boot-time-fail-fast
// log line per ADR-0072).
func TestNew_ParseRejectPropagates(t *testing.T) {
	ac := validConfig()
	ac.ConcurrencyControllerConfig = nil // trigger arm 1
	any := toAny(t, ac)
	reg := stats.NewRegistry()
	factory, err := New(any, envoyhttp.FactoryCtx{Stats: reg, StatPrefix: "http.test"})
	if err == nil {
		t.Fatal("New with arm-1 config: got nil err; want PARSE-REJECT error")
	}
	if factory != nil {
		t.Errorf("New with arm-1 config: got non-nil factory; want nil")
	}
	if err.Error() != parseRejectControllerOneofRequired {
		t.Errorf("err.Error() = %q; want %q (byte-stable PARSE-REJECT wording per D2)",
			err.Error(), parseRejectControllerOneofRequired)
	}
}

// -----------------------------------------------------------------------------
// 5. TestStatNames_Equal_Wire — planner-time D5 closure.
// -----------------------------------------------------------------------------

// TestStatNames_Equal_Wire pins the 7 statName* constants byte-exact against
// their upstream Envoy wire names per phase-21 SPEC §6.6 + AMEND-3 +
// planner-time D5. The constants are consumed by newFilterStats's prefix
// concatenation; a regression that renames the constant alongside the
// string literal (a refactor hazard) is caught by this assertion.
//
// Two-layer guard recorded at stats.go header (1) const declarations pin the
// values at compile time + (2) THIS test pins each constant to its string
// literal — preventing a future refactor from silently renaming the constant
// alongside the string literal.
func TestStatNames_Equal_Wire(t *testing.T) {
	cases := []struct {
		constName string
		constVal  string
		wireName  string
	}{
		{"statNameRqBlocked", statNameRqBlocked, "rq_blocked"},
		{"statNameConcurrencyLimit", statNameConcurrencyLimit, "concurrency_limit"},
		{"statNameGradient", statNameGradient, "gradient"},
		{"statNameBurstQueueSize", statNameBurstQueueSize, "burst_queue_size"},
		{"statNameSampleRTTMsecs", statNameSampleRTTMsecs, "sample_rtt_msecs"},
		{"statNameMinRTTMsecs", statNameMinRTTMsecs, "min_rtt_msecs"},
		{"statNameMinRTTCalculationActive", statNameMinRTTCalculationActive, "min_rtt_calculation_active"},
	}
	for _, c := range cases {
		if c.constVal != c.wireName {
			t.Errorf("stat name drift: %s = %q; want %q", c.constName, c.constVal, c.wireName)
		}
	}
}
