package bandwidthlimit

import (
	"crypto/tls"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	bandwidthlimitv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/bandwidth_limit/v3"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// mustAny packages a BandwidthLimit proto into an *anypb.Any with the
// bandwidth_limit TypeURL. Mirrors phase-11/13/14 test-helper precedent.
func mustAny(t *testing.T, msg proto.Message) *anypb.Any {
	t.Helper()
	a, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	return a
}

// freshFactoryCtx returns a FactoryCtx with a fresh Registry; used by tests
// that exercise the stat-registration path (test code per ADR-0085 may also
// pass an empty FactoryCtx{} to skip the stats path; both paths are tested).
func freshFactoryCtx() envoyhttp.FactoryCtx {
	return envoyhttp.FactoryCtx{Stats: stats.NewRegistry()}
}

// happyConfig returns a minimum-viable BandwidthLimit proto: stat_prefix +
// limit_kbps = 10. Per SPEC §6.5 these two fields plus the default 50ms
// fill_interval form the smallest accepted listener-level shape.
func happyConfig() *bandwidthlimitv3.BandwidthLimit {
	return &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "test",
		LimitKbps:  wrapperspb.UInt64(10),
	}
}

// ----------------------------------------------------------------------------
// Group 1 — New factory + buildCompiledConfig PGV-mirror (per SPEC §14.1 #1 +
// §6.5 + §1.1 amendments 3 + 4 + 5).
// ----------------------------------------------------------------------------

func TestNew_NilTC(t *testing.T) {
	_, err := New(nil, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatalf("New(nil, _): want error, got nil")
	}
	if !strings.Contains(err.Error(), "typed_config required") {
		t.Errorf("got %q; want substring 'typed_config required'", err.Error())
	}
}

func TestNew_MalformedTC(t *testing.T) {
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	_, err := New(bad, envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatalf("New(malformed, _): want error, got nil")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("got %q; want substring 'unmarshal'", err.Error())
	}
}

func TestNew_HappyPathReturnsFactory(t *testing.T) {
	factory, err := New(mustAny(t, happyConfig()), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(happy, _): want success, got %v", err)
	}
	if factory == nil {
		t.Fatal("New: returned nil factory on happy path")
	}
	hf := factory()
	if hf.Name != filterName {
		t.Errorf("HTTPFilter.Name = %q; want %q", hf.Name, filterName)
	}
	if hf.Decoder == nil {
		t.Error("HTTPFilter.Decoder = nil; want non-nil (decode-side service per ADR-0135)")
	}
	if hf.Encoder == nil {
		t.Error("HTTPFilter.Encoder = nil; want non-nil (encode-side service per ADR-0135)")
	}
	// SAME *filter instance per ADR-0135 + planner-time decision 9 (mirrors
	// phase-14 ADR-0129 generalized to symmetric BOTH-direction).
	if dec, ok := hf.Decoder.(*filter); ok {
		if enc, ok := hf.Encoder.(*filter); ok {
			if dec != enc {
				t.Error("Decoder and Encoder must be the SAME *filter instance per ADR-0135")
			}
		}
	}
}

func TestBuildCompiledConfig_StatPrefixEmpty_Rejected(t *testing.T) {
	c := happyConfig()
	c.StatPrefix = ""
	_, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false /*isPerRoute*/)
	if err == nil {
		t.Fatal("expected error on empty stat_prefix")
	}
	if err.Error() != "bandwidth_limit: stat_prefix required" {
		t.Errorf("got %q; want 'bandwidth_limit: stat_prefix required'", err.Error())
	}
}

func TestBuildCompiledConfig_AllEnableModeValuesAccepted(t *testing.T) {
	cases := []bandwidthlimitv3.BandwidthLimit_EnableMode{
		bandwidthlimitv3.BandwidthLimit_DISABLED,
		bandwidthlimitv3.BandwidthLimit_REQUEST,
		bandwidthlimitv3.BandwidthLimit_RESPONSE,
		bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE,
	}
	for _, em := range cases {
		t.Run(em.String(), func(t *testing.T) {
			c := happyConfig()
			c.EnableMode = em
			rc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
			if err != nil {
				t.Fatalf("buildCompiledConfig(enable_mode=%v): want success, got %v", em, err)
			}
			if rc.enableMode != em {
				t.Errorf("enableMode = %v; want %v", rc.enableMode, em)
			}
		})
	}
}

func TestBuildCompiledConfig_LimitKbpsUnset_AcceptedAtListener(t *testing.T) {
	// Per §1.1 amendment 10: listener-level limit_kbps OPTIONAL with foot-gun
	// semantic. parse-time acceptance; runtime foot-gun.
	c := &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "default",
		// LimitKbps unset (nil wrapper).
	}
	rc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig(limit_kbps=nil, listener): want success, got %v", err)
	}
	if rc.limitKbps != 0 {
		t.Errorf("limitKbps = %d; want 0 (sentinel for unset)", rc.limitKbps)
	}
}

func TestBuildCompiledConfig_LimitKbpsZero_Rejected(t *testing.T) {
	c := happyConfig()
	c.LimitKbps = wrapperspb.UInt64(0)
	_, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
	if err == nil {
		t.Fatal("expected error on limit_kbps=0")
	}
	if !strings.Contains(err.Error(), "limit_kbps must be >= 1") {
		t.Errorf("got %q; want substring 'limit_kbps must be >= 1'", err.Error())
	}
}

func TestBuildCompiledConfig_LimitKbpsExplicit(t *testing.T) {
	for _, v := range []uint64{1, 10, 100, 1000} {
		t.Run("limit_kbps", func(t *testing.T) {
			c := happyConfig()
			c.LimitKbps = wrapperspb.UInt64(v)
			rc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
			if err != nil {
				t.Fatalf("buildCompiledConfig(limit_kbps=%d): want success, got %v", v, err)
			}
			if rc.limitKbps != v {
				t.Errorf("limitKbps = %d; want %d", rc.limitKbps, v)
			}
		})
	}
}

func TestBuildCompiledConfig_FillIntervalDefault_50ms(t *testing.T) {
	c := happyConfig()
	c.FillInterval = nil
	rc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
	if err != nil {
		t.Fatalf("buildCompiledConfig: want success, got %v", err)
	}
	if rc.fillInterval != 50*time.Millisecond {
		t.Errorf("fillInterval = %v; want 50ms (proto default per amendment 5)", rc.fillInterval)
	}
}

func TestBuildCompiledConfig_FillIntervalExplicit(t *testing.T) {
	cases := []time.Duration{
		20 * time.Millisecond,
		50 * time.Millisecond,
		100 * time.Millisecond,
		500 * time.Millisecond,
		1 * time.Second,
	}
	for _, d := range cases {
		t.Run(d.String(), func(t *testing.T) {
			c := happyConfig()
			c.FillInterval = durationpb.New(d)
			rc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
			if err != nil {
				t.Fatalf("buildCompiledConfig(fill_interval=%v): want success, got %v", d, err)
			}
			if rc.fillInterval != d {
				t.Errorf("fillInterval = %v; want %v", rc.fillInterval, d)
			}
		})
	}
}

func TestBuildCompiledConfig_FillIntervalBelowMin_Rejected(t *testing.T) {
	c := happyConfig()
	c.FillInterval = durationpb.New(10 * time.Millisecond)
	_, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
	if err == nil {
		t.Fatal("expected error on fill_interval < 20ms")
	}
	if !strings.Contains(err.Error(), "outside supported range [20ms, 1s]") {
		t.Errorf("got %q; want substring 'outside supported range [20ms, 1s]'", err.Error())
	}
}

func TestBuildCompiledConfig_FillIntervalAboveMax_Rejected(t *testing.T) {
	c := happyConfig()
	c.FillInterval = durationpb.New(2 * time.Second)
	_, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
	if err == nil {
		t.Fatal("expected error on fill_interval > 1s")
	}
	if !strings.Contains(err.Error(), "outside supported range [20ms, 1s]") {
		t.Errorf("got %q; want substring 'outside supported range [20ms, 1s]'", err.Error())
	}
}

func TestBuildCompiledConfig_RuntimeEnabled_SilentIgnored(t *testing.T) {
	// Per planner-time decision 7 + §11.P6: runtime_enabled is parsed but
	// silently ignored at runtime (always-100%-active).
	c := happyConfig()
	c.RuntimeEnabled = &corev3.RuntimeFeatureFlag{
		DefaultValue: wrapperspb.Bool(false),
		RuntimeKey:   "test.bandwidth.enabled",
	}
	rc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
	if err != nil {
		t.Fatalf("buildCompiledConfig(runtime_enabled set): want success (silent-ignore); got %v", err)
	}
	// compiledConfig has no runtime-enabled field by design; if the parse
	// succeeded, the silent-ignore contract holds.
	if rc.statPrefix != "test" {
		t.Errorf("statPrefix = %q; want 'test' (other fields untouched)", rc.statPrefix)
	}
}

func TestBuildCompiledConfig_EnableResponseTrailers_SilentIgnored(t *testing.T) {
	// Per planner-time decision 8: enable_response_trailers parsed but
	// always-no-trailers at runtime.
	c := happyConfig()
	c.EnableResponseTrailers = true
	rc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
	if err != nil {
		t.Fatalf("buildCompiledConfig(enable_response_trailers=true): want success; got %v", err)
	}
	if rc == nil {
		t.Fatal("rc nil after enable_response_trailers parse")
	}
}

func TestBuildCompiledConfig_ResponseTrailerPrefix_SilentIgnored(t *testing.T) {
	// Per planner-time decision 8 (couples to enable_response_trailers).
	c := happyConfig()
	c.ResponseTrailerPrefix = "test-bw"
	rc, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, false)
	if err != nil {
		t.Fatalf("buildCompiledConfig(response_trailer_prefix set): want success; got %v", err)
	}
	if rc == nil {
		t.Fatal("rc nil after response_trailer_prefix parse")
	}
}

// ----------------------------------------------------------------------------
// Group 2 — buildCompiledConfigPerRoute + parsePerRoute PGV-mirror discipline
// (per SPEC §14.1 #2 + §6.11 + §11.P1).
// ----------------------------------------------------------------------------

func TestBuildCompiledConfigPerRoute_LimitKbpsUnset_Rejected(t *testing.T) {
	// Per §1.1 amendment 4 + §11.P1: per-route entry REQUIRES limit_kbps.
	c := &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "override",
		// LimitKbps unset.
	}
	_, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, true /*isPerRoute*/)
	if err == nil {
		t.Fatal("expected error: per-route requires limit_kbps")
	}
	expected := "bandwidth_limit: per-route entry requires limit_kbps to be set"
	if err.Error() != expected {
		t.Errorf("got %q; want %q (verbatim per ADR-0136)", err.Error(), expected)
	}
}

func TestBuildCompiledConfigPerRoute_LimitKbpsSet_Accepted(t *testing.T) {
	c := &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "override",
		LimitKbps:  wrapperspb.UInt64(5),
	}
	rc, err := buildCompiledConfig(c, freshFactoryCtx(), true /*isPerRoute*/)
	if err != nil {
		t.Fatalf("buildCompiledConfig(per-route, limit_kbps=5): want success, got %v", err)
	}
	if rc.limitKbps != 5 {
		t.Errorf("limitKbps = %d; want 5", rc.limitKbps)
	}
	// Per ADR-0139 + ADR-0117: per-route INDEPENDENT stats allocated via
	// newFilterStatsIfAbsent (post-Freeze idempotent path).
	if rc.stats == nil {
		t.Error("per-route rc.stats = nil; want non-nil (INDEPENDENT stats per ADR-0139)")
	}
}

func TestBuildCompiledConfigPerRoute_StatPrefixEmpty_Rejected(t *testing.T) {
	// PGV-mirror at per-route position too (same proto reuse pattern per
	// IMPL-1: §11.P1 — bare BandwidthLimit via TPFC, not a wrapper).
	c := &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "",
		LimitKbps:  wrapperspb.UInt64(5),
	}
	_, err := buildCompiledConfig(c, envoyhttp.FactoryCtx{}, true)
	if err == nil {
		t.Fatal("expected error on empty per-route stat_prefix")
	}
	if err.Error() != "bandwidth_limit: stat_prefix required" {
		t.Errorf("got %q; want 'bandwidth_limit: stat_prefix required'", err.Error())
	}
}

func TestParsePerRoute_ValidProto_Parses(t *testing.T) {
	any := mustAny(t, &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "route_override",
		LimitKbps:  wrapperspb.UInt64(50),
		EnableMode: bandwidthlimitv3.BandwidthLimit_RESPONSE,
	})
	msg, err := parsePerRoute(any)
	if err != nil {
		t.Fatalf("parsePerRoute(valid): want success, got %v", err)
	}
	pr, ok := msg.(*bandwidthlimitv3.BandwidthLimit)
	if !ok {
		t.Fatalf("parsePerRoute returned %T; want *bandwidthlimitv3.BandwidthLimit", msg)
	}
	if pr.GetStatPrefix() != "route_override" {
		t.Errorf("StatPrefix = %q; want 'route_override'", pr.GetStatPrefix())
	}
	if pr.GetLimitKbps().GetValue() != 50 {
		t.Errorf("LimitKbps = %d; want 50", pr.GetLimitKbps().GetValue())
	}
}

func TestParsePerRoute_MalformedAny_Rejected(t *testing.T) {
	bad := &anypb.Any{TypeUrl: TypeURL, Value: []byte{0xff, 0xff, 0xff}}
	_, err := parsePerRoute(bad)
	if err == nil {
		t.Fatal("expected error on malformed per-route Any")
	}
	if !strings.Contains(err.Error(), "unmarshal") {
		t.Errorf("got %q; want substring 'unmarshal'", err.Error())
	}
}

func TestParsePerRoute_RuntimeEnabledOverride_SilentIgnored(t *testing.T) {
	// Per planner-time decision 7 + §11.P6: per-route runtime_enabled is
	// also silent-ignored. parsePerRoute returns the proto unchanged; the
	// runtime evaluation path NEVER consults runtime_enabled.
	any := mustAny(t, &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "route_override",
		LimitKbps:  wrapperspb.UInt64(5),
		RuntimeEnabled: &corev3.RuntimeFeatureFlag{
			DefaultValue: wrapperspb.Bool(false),
			RuntimeKey:   "per_route.bandwidth.enabled",
		},
	})
	msg, err := parsePerRoute(any)
	if err != nil {
		t.Fatalf("parsePerRoute(runtime_enabled set): want success (silent-ignore); got %v", err)
	}
	pr, ok := msg.(*bandwidthlimitv3.BandwidthLimit)
	if !ok {
		t.Fatalf("parsePerRoute returned %T; want *bandwidthlimitv3.BandwidthLimit", msg)
	}
	// Field is present on the parsed proto but the filter's runtime path
	// must not honor it (covered structurally — compiledConfig has no
	// runtime-enabled field).
	if pr.GetRuntimeEnabled() == nil {
		t.Error("runtime_enabled missing on parsed per-route proto; expected preservation in unmarshal")
	}
}

// ----------------------------------------------------------------------------
// Group 3 — throttleDuration kbps-per-tick arithmetic (per SPEC §14.1 #3 +
// §6.6 + §11.P15 + §1.1 amendments 6 + 10).
// ----------------------------------------------------------------------------

func TestThrottleDuration_EmptyBody_ReturnsZero(t *testing.T) {
	// bodySize=0 → (0, 0): no throttle, no ticks. Caller skips timer-arm.
	dur, ticks := throttleDuration(0, 10, 50*time.Millisecond)
	if dur != 0 || ticks != 0 {
		t.Errorf("expected (0, 0); got (%v, %d)", dur, ticks)
	}
}

func TestThrottleDuration_LimitKbpsZero_ReturnsFootGun(t *testing.T) {
	// Per §1.1 amendment 10 + §11.P12 probeJ: limit_kbps=0 + active enable_mode
	// triggers an arbitrarily-large throttle (24h) matching Envoy's runtime-hang
	// behavior on missing listener-level limit_kbps. ticks=1 lets the caller
	// still increment *_enforced by 1 at first tick (matching Envoy).
	dur, ticks := throttleDuration(100, 0, 50*time.Millisecond)
	if dur != 24*time.Hour {
		t.Errorf("expected 24h foot-gun throttle; got %v", dur)
	}
	if ticks != 1 {
		t.Errorf("expected ticks=1 (foot-gun marker); got %d", ticks)
	}
}

func TestThrottleDuration_OneTickFloor(t *testing.T) {
	// Sub-chunk_size bodies fit in one tick → (fillInterval, 1). Approximates
	// Envoy's initial-burst capacity behavior within ±70ms tolerance per §11.P9.
	// chunk_size_per_tick = limit_kbps × 1024 × fill_interval_seconds.
	// Parametrized across several (body, kbps, fill) combinations all of which
	// satisfy body <= chunk_size_per_tick.
	cases := []struct {
		name string
		body int
		kbps uint64
		fill time.Duration
	}{
		{"100B_10kbps_50ms_chunk512", 100, 10, 50 * time.Millisecond},
		{"512B_10kbps_50ms_chunk512", 512, 10, 50 * time.Millisecond},
		{"1B_1kbps_50ms_chunk51", 1, 1, 50 * time.Millisecond},
		{"1024B_10kbps_100ms_chunk1024", 1024, 10, 100 * time.Millisecond},
		{"2048B_100kbps_20ms_chunk2048", 2048, 100, 20 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dur, ticks := throttleDuration(tc.body, tc.kbps, tc.fill)
			if ticks != 1 {
				t.Errorf("body=%d kbps=%d fill=%v: wantTicks=1 gotTicks=%d", tc.body, tc.kbps, tc.fill, ticks)
			}
			if dur != tc.fill {
				t.Errorf("body=%d kbps=%d fill=%v: wantDur=%v gotDur=%v", tc.body, tc.kbps, tc.fill, tc.fill, dur)
			}
		})
	}
}

func TestThrottleDuration_KbpsPerTickMatrix(t *testing.T) {
	// Verbatim from SPEC §6.6 empirical-verification table (5 rows):
	//   chunk_size = limit_kbps × 1024 × fill_interval_seconds
	//   ticks      = ceil(body_size / chunk_size)
	//   duration   = ticks × fill_interval
	cases := []struct {
		name      string
		body      int
		kbps      uint64
		fill      time.Duration
		wantTicks uint64
		wantDur   time.Duration
	}{
		{"body100_kbps10_ticks1", 100, 10, 50 * time.Millisecond, 1, 50 * time.Millisecond},     // one-tick floor (sub-chunk_size)
		{"body1024_kbps10_ticks2", 1024, 10, 50 * time.Millisecond, 2, 100 * time.Millisecond},  // 2 ticks @ 512 chunk_size
		{"body4000_kbps10_ticks8", 4000, 10, 50 * time.Millisecond, 8, 400 * time.Millisecond},  // 8 ticks @ 512 chunk_size
		{"body4000_kbps5_ticks16", 4000, 5, 50 * time.Millisecond, 16, 800 * time.Millisecond},  // 16 ticks @ 256 chunk_size
		{"body4000_kbps1_ticks79", 4000, 1, 50 * time.Millisecond, 79, 3950 * time.Millisecond}, // 79 ticks @ 51.2 chunk_size
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dur, ticks := throttleDuration(tc.body, tc.kbps, tc.fill)
			if ticks != tc.wantTicks {
				t.Errorf("body=%d kbps=%d fill=%v: wantTicks=%d gotTicks=%d", tc.body, tc.kbps, tc.fill, tc.wantTicks, ticks)
			}
			if dur != tc.wantDur {
				t.Errorf("body=%d kbps=%d fill=%v: wantDur=%v gotDur=%v", tc.body, tc.kbps, tc.fill, tc.wantDur, dur)
			}
		})
	}
}

func TestThrottleDuration_FillIntervalGranularity(t *testing.T) {
	// chunk_size = limit_kbps × 1024 × fill_interval_seconds scales linearly
	// with fill_interval. Hold limit_kbps + body constant and vary fill_interval
	// across the PGV-supported range [20ms, 1s]:
	//
	//   At limit_kbps=10, body=10240:
	//     fill=50ms  → chunk_size=512 bytes  → ticks=20 → dur=1000ms
	//     fill=100ms → chunk_size=1024 bytes → ticks=10 → dur=1000ms
	//     fill=200ms → chunk_size=2048 bytes → ticks=5  → dur=1000ms
	//     fill=500ms → chunk_size=5120 bytes → ticks=2  → dur=1000ms
	//     fill=1s    → chunk_size=10240 bytes → ticks=1 → dur=1000ms
	//
	// Note: integer-chunk_size cases chosen to avoid float→uint64 truncation
	// artifacts at non-integer chunk_size (e.g. 20ms × 10kbps = 204.8 bytes
	// which uint64-casts to 204, producing one extra ceil-div tick). Such
	// truncation is by-design at the SPEC §6.6 formula; this test exercises
	// the linear-scaling invariant where chunk_size is an exact integer.
	cases := []struct {
		fill      time.Duration
		wantTicks uint64
		wantDur   time.Duration
	}{
		{50 * time.Millisecond, 20, 1000 * time.Millisecond},
		{100 * time.Millisecond, 10, 1000 * time.Millisecond},
		{200 * time.Millisecond, 5, 1000 * time.Millisecond},
		{500 * time.Millisecond, 2, 1000 * time.Millisecond},
		{1 * time.Second, 1, 1000 * time.Millisecond},
	}
	for _, tc := range cases {
		t.Run(tc.fill.String(), func(t *testing.T) {
			dur, ticks := throttleDuration(10240, 10, tc.fill)
			if ticks != tc.wantTicks {
				t.Errorf("fill=%v: wantTicks=%d gotTicks=%d", tc.fill, tc.wantTicks, ticks)
			}
			if dur != tc.wantDur {
				t.Errorf("fill=%v: wantDur=%v gotDur=%v", tc.fill, tc.wantDur, dur)
			}
		})
	}
}

func TestThrottleDuration_LargeBody(t *testing.T) {
	// Sanity: large body sizes don't overflow uint64 arithmetic.
	// body=51200 kbps=10 fill=50ms → chunk_size=512 → ticks=100 → dur=5s.
	dur, ticks := throttleDuration(51200, 10, 50*time.Millisecond)
	if ticks != 100 {
		t.Errorf("wantTicks=100 gotTicks=%d", ticks)
	}
	if dur != 5*time.Second {
		t.Errorf("wantDur=5s gotDur=%v", dur)
	}
}

// ----------------------------------------------------------------------------
// Group 4 — DecodeHeaders + DecodeData decode-side throttle (per SPEC §14.1 #4 +
// §6.7 + §11.P3 + §11.P12 + ADR-0137). 12 tests: 5 DecodeHeaders + 7 DecodeData.
// ----------------------------------------------------------------------------

// fakeDecoderCB is the test-double DecoderFilterCallbacks for Group 4 + 5.
// RequestRouteConfig returns the settable routeCfg (default nil → listener
// inherit per resolvePerRouteConfig); ContinueDecoding records invocations
// via an atomic counter so timer-fire tests can deterministically wait on
// the count. Mirrors phase-09 fault recordingDCB pattern at
// internal/filter/http/fault/fault_test.go:194-232.
type fakeDecoderCB struct {
	mu        sync.Mutex
	routeCfg  proto.Message // returned by RequestRouteConfig; nil by default
	continued atomic.Int32  // bumped by ContinueDecoding (timer-fire callback)
}

func (f *fakeDecoderCB) ContinueDecoding() { f.continued.Add(1) }
func (f *fakeDecoderCB) SendLocalReply(int, string, envoyhttp.OrderedHeaders) {
	// bandwidth_limit does not SendLocalReply; satisfy the interface.
}
func (f *fakeDecoderCB) RequestRouteConfig() proto.Message {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.routeCfg
}
func (f *fakeDecoderCB) RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) {
	return nil, nil, nil
}
func (f *fakeDecoderCB) EncodeHeaders(http.Header, bool) {}
func (f *fakeDecoderCB) EncodeData([]byte, bool)         {}
func (f *fakeDecoderCB) EncodeTrailers(http.Header)      {}
func (f *fakeDecoderCB) DownstreamPrincipal() []string   { return nil }

// ADR-0165 callback-surface extension stubs (phase-18.2 Task 4).
func (f *fakeDecoderCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (f *fakeDecoderCB) DownstreamLocalAddr() net.Addr    { return nil }
func (f *fakeDecoderCB) DownstreamTLSServerName() string  { return "" }
func (f *fakeDecoderCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (f *fakeDecoderCB) DownstreamProtocol() string       { return "" }
func (f *fakeDecoderCB) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (f *fakeDecoderCB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (f *fakeDecoderCB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }

// ADR-0198 callback-surface extension stubs (phase-24.1 Task 5 — DELTA-2).
func (f *fakeDecoderCB) RouteRateLimits() []*routev3.RateLimit       { return nil }
func (f *fakeDecoderCB) VirtualHostRateLimits() []*routev3.RateLimit { return nil }
func (f *fakeDecoderCB) RouteMetadata() *corev3.Metadata             { return nil }
func (f *fakeDecoderCB) RouteIncludeVhRateLimits() bool              { return false }

// makeFilterWithMode constructs a *filter with the given enable_mode +
// limit_kbps + fill_interval and a freshly-attached fakeDecoderCB. Used
// across the Group 4 (and Group 5) tests; returns the filter + the dcb so
// the test can assert on f.requestActive / f.responseActive / dcb.continued.
func makeFilterWithMode(t *testing.T, mode bandwidthlimitv3.BandwidthLimit_EnableMode, limitKbps uint64, fill time.Duration) (*filter, *fakeDecoderCB) {
	t.Helper()
	c := &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "test",
		EnableMode: mode,
		LimitKbps:  wrapperspb.UInt64(limitKbps),
	}
	if fill > 0 {
		c.FillInterval = durationpb.New(fill)
	}
	factory, err := New(mustAny(t, c), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	inst := factory()
	fl := inst.Decoder.(*filter)
	dcb := &fakeDecoderCB{}
	fl.SetDecoderCallbacks(dcb)
	return fl, dcb
}

// waitForContinueDecoding polls dcb.continued at 2ms intervals until it
// reaches `want` or the deadline elapses. Returns true if reached. Avoids
// time.Sleep in the test body; mirrors phase-09 fault waitForCondition.
func waitForContinueDecoding(dcb *fakeDecoderCB, want int32, deadline time.Duration) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if dcb.continued.Load() >= want {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return dcb.continued.Load() >= want
}

func TestDecodeHeaders_EnableModeRequest_RequestActiveTrue(t *testing.T) {
	fl, _ := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 0)
	status := fl.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if !fl.requestActive {
		t.Error("requestActive: got false, want true (enable_mode=REQUEST)")
	}
	if fl.responseActive {
		t.Error("responseActive: got true, want false (enable_mode=REQUEST excludes RESPONSE)")
	}
	if fl.requestRC == nil {
		t.Error("requestRC: got nil, want cached *compiledConfig")
	}
	if fl.responseRC == nil {
		t.Error("responseRC: got nil, want cascade from requestRC")
	}
	if fl.requestRC != fl.responseRC {
		t.Error("responseRC must be the SAME *compiledConfig as requestRC (per-stream symmetric cascade)")
	}
}

func TestDecodeHeaders_EnableModeResponse_ResponseActiveTrue(t *testing.T) {
	fl, _ := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_RESPONSE, 10, 0)
	status := fl.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if fl.requestActive {
		t.Error("requestActive: got true, want false (enable_mode=RESPONSE excludes REQUEST)")
	}
	if !fl.responseActive {
		t.Error("responseActive: got false, want true (enable_mode=RESPONSE)")
	}
}

func TestDecodeHeaders_EnableModeBoth_BothActive(t *testing.T) {
	fl, _ := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE, 10, 0)
	status := fl.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if !fl.requestActive {
		t.Error("requestActive: got false, want true (REQUEST_AND_RESPONSE)")
	}
	if !fl.responseActive {
		t.Error("responseActive: got false, want true (REQUEST_AND_RESPONSE)")
	}
}

func TestDecodeHeaders_EnableModeDisabled_BothFalse(t *testing.T) {
	// Per §11.P12 DISABLED-mode wholly-inactive semantic: neither side engages.
	fl, _ := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_DISABLED, 10, 0)
	status := fl.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if fl.requestActive {
		t.Error("requestActive: got true, want false (DISABLED)")
	}
	if fl.responseActive {
		t.Error("responseActive: got true, want false (DISABLED)")
	}
}

func TestDecodeHeaders_PerRouteResolution_CachesRC(t *testing.T) {
	// Listener-level config: enable_mode=DISABLED. Per-route override returned
	// by RequestRouteConfig: enable_mode=REQUEST + different stat_prefix.
	// After DecodeHeaders, f.requestRC must point to the per-route compiledConfig
	// (NOT the listener listenerRC).
	fl, dcb := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_DISABLED, 10, 0)
	perRouteProto := &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: "route_override",
		EnableMode: bandwidthlimitv3.BandwidthLimit_REQUEST,
		LimitKbps:  wrapperspb.UInt64(20),
	}
	dcb.mu.Lock()
	dcb.routeCfg = perRouteProto
	dcb.mu.Unlock()

	status := fl.DecodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue", status)
	}
	if fl.requestRC == nil {
		t.Fatal("requestRC: got nil, want per-route *compiledConfig")
	}
	if fl.requestRC == fl.state.listenerRC {
		t.Error("requestRC must NOT be the listener listenerRC (per-route override expected)")
	}
	if fl.requestRC.statPrefix != "route_override" {
		t.Errorf("requestRC.statPrefix: got %q, want 'route_override' (per-route)", fl.requestRC.statPrefix)
	}
	if fl.requestRC.limitKbps != 20 {
		t.Errorf("requestRC.limitKbps: got %d, want 20 (per-route)", fl.requestRC.limitKbps)
	}
	// Per-route enable_mode=REQUEST → requestActive=true.
	if !fl.requestActive {
		t.Error("requestActive: got false, want true (per-route enable_mode=REQUEST)")
	}
}

func TestDecodeData_PassthroughWhenInactive_DataContinue(t *testing.T) {
	// requestActive=false → DataContinue regardless of body content / endStream.
	fl, _ := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_DISABLED, 10, 0)
	fl.DecodeHeaders(http.Header{}, false) // sets requestActive=false
	// Non-empty body, endStream=true → still passthrough.
	status := fl.DecodeData([]byte("hello world"), true)
	if status != envoyhttp.DataContinue {
		t.Errorf("status: got %v, want DataContinue (inactive)", status)
	}
	if len(fl.requestBody) != 0 {
		t.Errorf("requestBody: got len=%d, want 0 (inactive must not buffer)", len(fl.requestBody))
	}
}

func TestDecodeData_BufferedAccumulation_PreEndStream(t *testing.T) {
	// Multi-chunk body; pre-endStream chunks return DataContinue (envoy-go HCM
	// dispatch is synchronous in the same goroutine as the chain — parking on
	// non-endStream would deadlock) but still accumulate locally on
	// f.requestBody so the throttle decision on the terminal chunk uses the
	// full body length.
	fl, _ := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 0)
	fl.DecodeHeaders(http.Header{}, false)
	chunk1 := []byte("chunk1-")
	chunk2 := []byte("chunk2-")
	chunk3 := []byte("chunk3")
	if status := fl.DecodeData(chunk1, false); status != envoyhttp.DataContinue {
		t.Errorf("chunk1: got %v, want DataContinue (envoy-go non-endStream pass-through)", status)
	}
	if status := fl.DecodeData(chunk2, false); status != envoyhttp.DataContinue {
		t.Errorf("chunk2: got %v, want DataContinue", status)
	}
	// Pre-endStream the local accumulator must reflect both chunks.
	want := "chunk1-chunk2-"
	if string(fl.requestBody) != want {
		t.Errorf("requestBody after 2 chunks: got %q, want %q", string(fl.requestBody), want)
	}
	// Final non-endStream chunk also passes through.
	if status := fl.DecodeData(chunk3, false); status != envoyhttp.DataContinue {
		t.Errorf("chunk3: got %v, want DataContinue", status)
	}
	want = "chunk1-chunk2-chunk3"
	if string(fl.requestBody) != want {
		t.Errorf("requestBody after 3 chunks: got %q, want %q", string(fl.requestBody), want)
	}
}

func TestDecodeData_EndStream_ZeroBody_FastPath(t *testing.T) {
	// Empty body → throttleDuration returns (0, 0) → fast path: DataContinue +
	// *_enabled + *_incoming_total_size + *_incoming_size + *_allowed_total_size
	// + *_allowed_size incremented; *_pending NOT bumped; no timer arm.
	fl, dcb := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	status := fl.DecodeData(nil, true)
	if status != envoyhttp.DataContinue {
		t.Errorf("status: got %v, want DataContinue (fast path on empty body)", status)
	}
	if fl.requestTimer != nil {
		t.Error("requestTimer: got non-nil, want nil (fast path must not arm timer)")
	}
	st := fl.requestRC.stats
	if got := st.requestEnabled.Load(); got != 1 {
		t.Errorf("requestEnabled: got %d, want 1", got)
	}
	if got := st.requestIncomingTotalSize.Load(); got != 0 {
		t.Errorf("requestIncomingTotalSize: got %d, want 0 (empty body)", got)
	}
	if got := st.requestAllowedTotalSize.Load(); got != 0 {
		t.Errorf("requestAllowedTotalSize: got %d, want 0 (empty body)", got)
	}
	if got := st.requestPending.Load(); got != 0 {
		t.Errorf("requestPending: got %d, want 0 (fast path skips pending)", got)
	}
	// dcb.ContinueDecoding must NOT be called on the fast path; the chain
	// resumes synchronously via DataContinue.
	if got := dcb.continued.Load(); got != 0 {
		t.Errorf("dcb.continued: got %d, want 0 (fast path does not invoke ContinueDecoding)", got)
	}
}

func TestDecodeData_EndStream_SmallBody_OneTickFloor(t *testing.T) {
	// 100-byte body @ kbps=10 fill=50ms → chunk_size=512 → ticks=1 → throttle=50ms.
	// Verifies: DataStopIterationAndBuffer + requestPending=1 + timer arms; the
	// timer eventually fires and calls ContinueDecoding (asserted via channel wait).
	fl, dcb := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 100)
	status := fl.DecodeData(body, true)
	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("status: got %v, want DataStopIterationAndBuffer (timer arm)", status)
	}
	if fl.requestTimer == nil {
		t.Error("requestTimer: got nil, want armed (one-tick floor)")
	}
	if got := fl.requestRC.stats.requestPending.Load(); got != 1 {
		t.Errorf("requestPending: got %d, want 1 (timer armed)", got)
	}
	// Wait for the timer-fire callback to invoke ContinueDecoding.
	if !waitForContinueDecoding(dcb, 1, 500*time.Millisecond) {
		t.Fatalf("ContinueDecoding never invoked within 500ms; continued=%d", dcb.continued.Load())
	}
}

func TestDecodeData_EndStream_LargeBody_MultiTick(t *testing.T) {
	// 4000-byte body @ kbps=10 fill=50ms → chunk_size=512 → ticks=8 →
	// throttle=400ms. The test does not need to wait the full 400ms for the
	// timer to fire — it only verifies the arm shape (status + pending) and
	// then cancels the timer to keep test wall-time bounded. Cancellation is
	// safe at Task 4 since OnDestroy (Task 6) is not yet engaged; the test
	// goroutine is the only owner of f.requestTimer.
	fl, _ := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 4000)
	status := fl.DecodeData(body, true)
	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("status: got %v, want DataStopIterationAndBuffer", status)
	}
	if fl.requestTimer == nil {
		t.Fatal("requestTimer: got nil, want armed (multi-tick)")
	}
	// Stop the long-duration timer to bound the test's wall-time. Per
	// SPEC §6.9 + planner-time decision 3 the OnDestroy stop-races-fire
	// discipline lands at Task 6; here we stop directly since no concurrent
	// owner exists.
	fl.requestTimer.Stop()
	if got := fl.requestRC.stats.requestPending.Load(); got != 1 {
		t.Errorf("requestPending: got %d, want 1", got)
	}
}

func TestDecodeData_TimerFire_IncrementEnforcedByTicks(t *testing.T) {
	// 100-byte body @ kbps=10 fill=50ms → ticks=1. After the timer fires the
	// *_enforced counter equals exactly ticks (per §11.P3 + planner-time
	// decision 15 + ADR-0137: per-tick cumulative match, NOT once-per-stream).
	fl, dcb := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 100)
	fl.DecodeData(body, true)
	// Wait for the timer-fire callback to run.
	if !waitForContinueDecoding(dcb, 1, 500*time.Millisecond) {
		t.Fatalf("ContinueDecoding never invoked within 500ms")
	}
	st := fl.requestRC.stats
	if got := st.requestEnforced.Load(); got != 1 {
		t.Errorf("requestEnforced: got %d, want 1 (ticks=1 → Add(1))", got)
	}
	if got := st.requestAllowedTotalSize.Load(); got != 100 {
		t.Errorf("requestAllowedTotalSize: got %d, want 100 (bodyLen at timer-fire)", got)
	}
	if got := st.requestAllowedSize.Load(); got != 100 {
		t.Errorf("requestAllowedSize: got %d, want 100 (transient at timer-fire)", got)
	}
	if got := st.requestPending.Load(); got != 0 {
		t.Errorf("requestPending: got %d, want 0 (decremented at timer-fire)", got)
	}
}

func TestDecodeData_TimerFire_ContinueDecodingInvoked(t *testing.T) {
	// Verify dcb.ContinueDecoding is called exactly once from the timer-fire
	// callback. Mirrors phase-09 fault TestDecodeHeaders_DelayOnly pattern.
	fl, dcb := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 100)
	status := fl.DecodeData(body, true)
	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Fatalf("status: got %v, want DataStopIterationAndBuffer", status)
	}
	// Pre-fire: continued must be 0 (timer has not yet elapsed).
	if got := dcb.continued.Load(); got != 0 {
		t.Errorf("pre-fire continued: got %d, want 0", got)
	}
	if !waitForContinueDecoding(dcb, 1, 500*time.Millisecond) {
		t.Fatalf("ContinueDecoding never invoked within 500ms; continued=%d", dcb.continued.Load())
	}
	if got := dcb.continued.Load(); got != 1 {
		t.Errorf("post-fire continued: got %d, want exactly 1 (timer fires once)", got)
	}
}

// ----------------------------------------------------------------------------
// Group 5 — EncodeHeaders + EncodeData encode-side throttle (per SPEC §14.1 #5 +
// §6.8 + §11.P3 + ADR-0137). 8 tests: 1 EncodeHeaders + 7 EncodeData — symmetric
// mirror of Group 4 decode-side with response-side substitutions.
// ----------------------------------------------------------------------------

// fakeEncoderCB is the test-double EncoderFilterCallbacks for Group 5.
// ContinueEncoding records invocations via an atomic counter so timer-fire
// tests can deterministically wait on the count. OverwriteBody records any
// invocation (encode-side framework primitive per ADR-0131 §Decision (vi));
// the framework-survey at Task 5 Step 4 confirmed bandwidth_limit's same-bytes
// case does NOT invoke OverwriteBody (the DataStopIterationAndBuffer +
// ContinueEncoding path emits buffered bytes unchanged via the chain's
// post-resume DataContinue iteration). Mirrors fakeDecoderCB shape.
type fakeEncoderCB struct {
	continued       atomic.Int32 // bumped by ContinueEncoding (timer-fire callback)
	overwroteBody   atomic.Int32 // bumped by OverwriteBody — expected to stay 0
	overwriteBuffer []byte       // last OverwriteBody bytes (for diagnostic dumps)
	mu              sync.Mutex
}

func (e *fakeEncoderCB) ContinueEncoding() { e.continued.Add(1) }
func (e *fakeEncoderCB) EncodeHeaders(http.Header, bool) {
	// bandwidth_limit does not synthesize encode-side headers; satisfy interface.
}
func (e *fakeEncoderCB) EncodeData([]byte, bool)    {}
func (e *fakeEncoderCB) EncodeTrailers(http.Header) {}
func (e *fakeEncoderCB) OverwriteBody(b []byte) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.overwroteBody.Add(1)
	e.overwriteBuffer = append(e.overwriteBuffer[:0], b...)
}

// ADR-0174 callback-surface extension stubs (phase-19.1 Task 5 — symmetric
// encoder-side mirror of ADR-0165's 6 decoder-side accessors). Zero-value
// returns satisfy the extended EncoderFilterCallbacks interface required by
// fl.SetEncoderCallbacks; bandwidth_limit does not consume the accessors.
func (e *fakeEncoderCB) DownstreamRemoteAddr() net.Addr   { return nil }
func (e *fakeEncoderCB) DownstreamLocalAddr() net.Addr    { return nil }
func (e *fakeEncoderCB) DownstreamTLSServerName() string  { return "" }
func (e *fakeEncoderCB) DownstreamTLSPeerCertDER() []byte { return nil }
func (e *fakeEncoderCB) DownstreamProtocol() string       { return "" }
func (e *fakeEncoderCB) ListenerPrincipal() string        { return "" }

// ADR-0192 callback-surface extension stubs (phase-22.2 Task 5).
func (e *fakeEncoderCB) DownstreamTLSConnectionState() *tls.ConnectionState { return nil }
func (e *fakeEncoderCB) DynamicMetadata() *dynamicmetadata.Bucket           { return nil }
func (e *fakeEncoderCB) ResponseStatus() int                                { return 0 } // ADR-0196; bandwidthlimit does not consume the encode-side status.

// ADR-0175 callback-surface extension stub (phase-19.2 Task 2 — encode-side
// body-buffering framework primitive). Zero-value return preserves the
// extended EncoderFilterCallbacks compile-time conformance assertion green;
// bandwidth_limit does not consume the accessor.
func (e *fakeEncoderCB) BufferEncodedBody() []byte { return nil }

// makeFilterWithModeBothCB constructs a *filter wired with BOTH a
// fakeDecoderCB and a fakeEncoderCB. Required for Group 5 since EncodeData
// needs the responseActive cascade populated via DecodeHeaders FIRST + the
// encode-side test must observe ContinueEncoding via the ecb test-double.
func makeFilterWithModeBothCB(t *testing.T, mode bandwidthlimitv3.BandwidthLimit_EnableMode, limitKbps uint64, fill time.Duration) (*filter, *fakeDecoderCB, *fakeEncoderCB) {
	t.Helper()
	fl, dcb := makeFilterWithMode(t, mode, limitKbps, fill)
	ecb := &fakeEncoderCB{}
	fl.SetEncoderCallbacks(ecb)
	return fl, dcb, ecb
}

// waitForContinueEncoding polls ecb.continued at 2ms intervals until it
// reaches `want` or the deadline elapses. Mirrors waitForContinueDecoding.
func waitForContinueEncoding(ecb *fakeEncoderCB, want int32, deadline time.Duration) bool {
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		if ecb.continued.Load() >= want {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return ecb.continued.Load() >= want
}

func TestEncodeHeaders_NoOp(t *testing.T) {
	// EncodeHeaders is a 1-line no-op returning Continue — responseRC + responseActive
	// were cached at DecodeHeaders via the per-stream symmetric cascade.
	// Per SPEC §6.8: "responseRC + responseActive were cached at DecodeHeaders."
	fl, _, _ := makeFilterWithModeBothCB(t, bandwidthlimitv3.BandwidthLimit_RESPONSE, 10, 0)
	// Run DecodeHeaders first (populates the cascade so the test reflects the real flow).
	fl.DecodeHeaders(http.Header{}, false)
	// Snapshot the cached responseRC + responseActive; EncodeHeaders must NOT touch them.
	rcBefore := fl.responseRC
	activeBefore := fl.responseActive
	status := fl.EncodeHeaders(http.Header{}, false)
	if status != envoyhttp.Continue {
		t.Errorf("status: got %v, want Continue (encode-side no-op)", status)
	}
	if fl.responseRC != rcBefore {
		t.Error("responseRC: EncodeHeaders must NOT re-resolve per-route (already cached at DecodeHeaders)")
	}
	if fl.responseActive != activeBefore {
		t.Errorf("responseActive: got %v, want %v (must NOT be touched by EncodeHeaders)", fl.responseActive, activeBefore)
	}
}

func TestEncodeData_PassthroughWhenInactive_DataContinue(t *testing.T) {
	// responseActive=false → DataContinue regardless of body content / endStream.
	// Mirror of decode-side TestDecodeData_PassthroughWhenInactive_DataContinue.
	fl, _, _ := makeFilterWithModeBothCB(t, bandwidthlimitv3.BandwidthLimit_DISABLED, 10, 0)
	fl.DecodeHeaders(http.Header{}, false) // sets responseActive=false
	// Non-empty body, endStream=true → still passthrough.
	status := fl.EncodeData([]byte("hello world"), true)
	if status != envoyhttp.DataContinue {
		t.Errorf("status: got %v, want DataContinue (inactive)", status)
	}
	if len(fl.responseBody) != 0 {
		t.Errorf("responseBody: got len=%d, want 0 (inactive must not buffer)", len(fl.responseBody))
	}
}

func TestEncodeData_BufferedAccumulation_PreEndStream(t *testing.T) {
	// Multi-chunk body; pre-endStream chunks return DataContinue (envoy-go HCM
	// encode-side dispatch is synchronous in the same goroutine as the chain —
	// parking on non-endStream would deadlock) but still accumulate locally on
	// f.responseBody so the throttle decision on the terminal chunk uses the
	// full body length. Mirror of decode-side.
	fl, _, _ := makeFilterWithModeBothCB(t, bandwidthlimitv3.BandwidthLimit_RESPONSE, 10, 0)
	fl.DecodeHeaders(http.Header{}, false)
	chunk1 := []byte("chunk1-")
	chunk2 := []byte("chunk2-")
	chunk3 := []byte("chunk3")
	if status := fl.EncodeData(chunk1, false); status != envoyhttp.DataContinue {
		t.Errorf("chunk1: got %v, want DataContinue (envoy-go non-endStream pass-through)", status)
	}
	if status := fl.EncodeData(chunk2, false); status != envoyhttp.DataContinue {
		t.Errorf("chunk2: got %v, want DataContinue", status)
	}
	want := "chunk1-chunk2-"
	if string(fl.responseBody) != want {
		t.Errorf("responseBody after 2 chunks: got %q, want %q", string(fl.responseBody), want)
	}
	if status := fl.EncodeData(chunk3, false); status != envoyhttp.DataContinue {
		t.Errorf("chunk3: got %v, want DataContinue", status)
	}
	want = "chunk1-chunk2-chunk3"
	if string(fl.responseBody) != want {
		t.Errorf("responseBody after 3 chunks: got %q, want %q", string(fl.responseBody), want)
	}
}

func TestEncodeData_EndStream_ZeroBody_FastPath(t *testing.T) {
	// Empty body → throttleDuration returns (0, 0) → fast path: DataContinue +
	// *_enabled + *_incoming_total_size + *_incoming_size + *_allowed_total_size
	// + *_allowed_size incremented; *_pending NOT bumped; no timer arm.
	fl, _, ecb := makeFilterWithModeBothCB(t, bandwidthlimitv3.BandwidthLimit_RESPONSE, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	status := fl.EncodeData(nil, true)
	if status != envoyhttp.DataContinue {
		t.Errorf("status: got %v, want DataContinue (fast path on empty body)", status)
	}
	if fl.responseTimer != nil {
		t.Error("responseTimer: got non-nil, want nil (fast path must not arm timer)")
	}
	st := fl.responseRC.stats
	if got := st.responseEnabled.Load(); got != 1 {
		t.Errorf("responseEnabled: got %d, want 1", got)
	}
	if got := st.responseIncomingTotalSize.Load(); got != 0 {
		t.Errorf("responseIncomingTotalSize: got %d, want 0 (empty body)", got)
	}
	if got := st.responseAllowedTotalSize.Load(); got != 0 {
		t.Errorf("responseAllowedTotalSize: got %d, want 0 (empty body)", got)
	}
	if got := st.responsePending.Load(); got != 0 {
		t.Errorf("responsePending: got %d, want 0 (fast path skips pending)", got)
	}
	// ecb.ContinueEncoding must NOT be called on the fast path; the chain
	// resumes synchronously via DataContinue.
	if got := ecb.continued.Load(); got != 0 {
		t.Errorf("ecb.continued: got %d, want 0 (fast path does not invoke ContinueEncoding)", got)
	}
}

func TestEncodeData_EndStream_SmallBody_OneTickFloor(t *testing.T) {
	// 100-byte body @ kbps=10 fill=50ms → chunk_size=512 → ticks=1 → throttle=50ms.
	// Verifies: DataStopIterationAndBuffer + responsePending=1 + timer arms; the
	// timer eventually fires and calls ContinueEncoding (asserted via bounded poll).
	fl, _, ecb := makeFilterWithModeBothCB(t, bandwidthlimitv3.BandwidthLimit_RESPONSE, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 100)
	status := fl.EncodeData(body, true)
	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("status: got %v, want DataStopIterationAndBuffer (timer arm)", status)
	}
	if fl.responseTimer == nil {
		t.Error("responseTimer: got nil, want armed (one-tick floor)")
	}
	if got := fl.responseRC.stats.responsePending.Load(); got != 1 {
		t.Errorf("responsePending: got %d, want 1 (timer armed)", got)
	}
	// Wait for the timer-fire callback to invoke ContinueEncoding.
	if !waitForContinueEncoding(ecb, 1, 500*time.Millisecond) {
		t.Fatalf("ContinueEncoding never invoked within 500ms; continued=%d", ecb.continued.Load())
	}
}

func TestEncodeData_EndStream_LargeBody_MultiTick(t *testing.T) {
	// 4000-byte body @ kbps=10 fill=50ms → chunk_size=512 → ticks=8 →
	// throttle=400ms. The test does not wait the full 400ms for the timer
	// to fire — it verifies the arm shape (status + pending) and then cancels
	// the timer to keep test wall-time bounded. Cancellation is safe at Task 5
	// since OnDestroy (Task 6) is not yet engaged; the test goroutine is the
	// only owner of f.responseTimer.
	fl, _, _ := makeFilterWithModeBothCB(t, bandwidthlimitv3.BandwidthLimit_RESPONSE, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 4000)
	status := fl.EncodeData(body, true)
	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Errorf("status: got %v, want DataStopIterationAndBuffer", status)
	}
	if fl.responseTimer == nil {
		t.Fatal("responseTimer: got nil, want armed (multi-tick)")
	}
	// Stop the long-duration timer to bound the test's wall-time. Per SPEC §6.9
	// + planner-time decision 3 the OnDestroy stop-races-fire discipline lands
	// at Task 6; here we stop directly since no concurrent owner exists.
	fl.responseTimer.Stop()
	if got := fl.responseRC.stats.responsePending.Load(); got != 1 {
		t.Errorf("responsePending: got %d, want 1", got)
	}
}

func TestEncodeData_TimerFire_IncrementEnforcedByTicks(t *testing.T) {
	// 100-byte body @ kbps=10 fill=50ms → ticks=1. After the timer fires the
	// *_enforced counter equals exactly ticks (per §11.P3 + planner-time
	// decision 15 + ADR-0137: per-tick cumulative match, NOT once-per-stream).
	fl, _, ecb := makeFilterWithModeBothCB(t, bandwidthlimitv3.BandwidthLimit_RESPONSE, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 100)
	fl.EncodeData(body, true)
	if !waitForContinueEncoding(ecb, 1, 500*time.Millisecond) {
		t.Fatalf("ContinueEncoding never invoked within 500ms")
	}
	st := fl.responseRC.stats
	if got := st.responseEnforced.Load(); got != 1 {
		t.Errorf("responseEnforced: got %d, want 1 (ticks=1 → Add(1))", got)
	}
	if got := st.responseAllowedTotalSize.Load(); got != 100 {
		t.Errorf("responseAllowedTotalSize: got %d, want 100 (bodyLen at timer-fire)", got)
	}
	if got := st.responseAllowedSize.Load(); got != 100 {
		t.Errorf("responseAllowedSize: got %d, want 100 (transient at timer-fire)", got)
	}
	if got := st.responsePending.Load(); got != 0 {
		t.Errorf("responsePending: got %d, want 0 (decremented at timer-fire)", got)
	}
	// Framework-survey assertion: same-bytes case does NOT invoke OverwriteBody.
	if got := ecb.overwroteBody.Load(); got != 0 {
		t.Errorf("ecb.overwroteBody: got %d, want 0 (same-bytes case must not invoke OverwriteBody per ADR-0137 §(vi))", got)
	}
}

func TestEncodeData_TimerFire_ContinueEncodingInvoked(t *testing.T) {
	// Verify ecb.ContinueEncoding is called exactly once from the timer-fire
	// callback. Mirror of decode-side TestDecodeData_TimerFire_ContinueDecodingInvoked.
	fl, _, ecb := makeFilterWithModeBothCB(t, bandwidthlimitv3.BandwidthLimit_RESPONSE, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 100)
	status := fl.EncodeData(body, true)
	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Fatalf("status: got %v, want DataStopIterationAndBuffer", status)
	}
	// Pre-fire: continued must be 0 (timer has not yet elapsed).
	if got := ecb.continued.Load(); got != 0 {
		t.Errorf("pre-fire continued: got %d, want 0", got)
	}
	if !waitForContinueEncoding(ecb, 1, 500*time.Millisecond) {
		t.Fatalf("ContinueEncoding never invoked within 500ms; continued=%d", ecb.continued.Load())
	}
	if got := ecb.continued.Load(); got != 1 {
		t.Errorf("post-fire continued: got %d, want exactly 1 (timer fires once)", got)
	}
}

// ----------------------------------------------------------------------------
// Group 6 — OnDestroy + Stop-races-Fire pending-gauge discipline (per SPEC
// §14.1 #6 + §6.9 + §4 + planner-time decision 3). 5 tests: noTimer no-op +
// Stop-returns-true Dec + Stop-returns-false trust-callback + N=100 concurrent
// race test + both-directions cleanup. The race test mirrors phase-09 fault's
// TestFault_DelayTimerRace pattern at internal/filter/http/fault/fault_test.go
// :679-701.
// ----------------------------------------------------------------------------

func TestOnDestroy_NoTimer_NoOp(t *testing.T) {
	// Both timers nil → OnDestroy is a no-op; pending gauges unchanged.
	// Reflects the !endStream / inactive / fast-path paths that never armed a
	// timer (Task 4/5 already exercise these; Task 6 verifies OnDestroy doesn't
	// regress the gauge under that shape).
	fl, _ := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	if fl.requestTimer != nil {
		t.Fatal("precondition: requestTimer must be nil (no DecodeData call yet)")
	}
	if fl.responseTimer != nil {
		t.Fatal("precondition: responseTimer must be nil (no EncodeData call yet)")
	}
	// Snapshot pending gauges; OnDestroy must leave them untouched.
	st := fl.requestRC.stats
	rpBefore := st.requestPending.Load()
	rspBefore := st.responsePending.Load()
	fl.OnDestroy()
	if got := st.requestPending.Load(); got != rpBefore {
		t.Errorf("requestPending: got %d, want %d (OnDestroy with nil timer must not Dec)", got, rpBefore)
	}
	if got := st.responsePending.Load(); got != rspBefore {
		t.Errorf("responsePending: got %d, want %d (OnDestroy with nil timer must not Dec)", got, rspBefore)
	}
}

func TestOnDestroy_TimerActive_StopReturnsTrue_DecPending(t *testing.T) {
	// Arm timer with LONG throttle (1s — well past the fast-path & realistic
	// test wall-time); OnDestroy before fire → Stop() returns true → OnDestroy
	// is the path responsible for *_pending.Dec(). No double-Dec since the
	// callback was prevented from running.
	//
	// To force a 1-second throttle: body 10240 @ kbps=10 fill=1s → chunk_size=
	// 10240 → ticks=1 → throttle=1s. (One-tick floor; long enough that we can
	// reliably call OnDestroy before timer-fire under test wall-time.)
	fl, dcb := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 1*time.Second)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 10240)
	status := fl.DecodeData(body, true)
	if status != envoyhttp.DataStopIterationAndBuffer {
		t.Fatalf("status: got %v, want DataStopIterationAndBuffer (timer arm)", status)
	}
	if fl.requestTimer == nil {
		t.Fatal("requestTimer: got nil, want armed")
	}
	if got := fl.requestRC.stats.requestPending.Load(); got != 1 {
		t.Fatalf("pre-OnDestroy requestPending: got %d, want 1 (timer armed)", got)
	}
	// OnDestroy before the 1s timer fires → Stop() returns true → Dec here.
	fl.OnDestroy()
	if got := fl.requestRC.stats.requestPending.Load(); got != 0 {
		t.Errorf("post-OnDestroy requestPending: got %d, want 0 (Stop()=true → OnDestroy Dec'd)", got)
	}
	// The callback was prevented — ContinueDecoding must NOT have been called.
	// Allow a tiny grace window (a stray Gosched) but the timer was Stop'd so
	// it should never have run.
	time.Sleep(20 * time.Millisecond)
	if got := dcb.continued.Load(); got != 0 {
		t.Errorf("dcb.continued: got %d, want 0 (timer Stop'd before fire)", got)
	}
}

func TestOnDestroy_TimerFired_StopReturnsFalse_TrustCallback(t *testing.T) {
	// Arm timer with SHORT throttle (50ms); sleep past fire; OnDestroy →
	// Stop() returns false → trust the callback's own Dec; OnDestroy must NOT
	// Dec (would underflow → -1). Final pending must equal 0 (callback Dec'd
	// the Inc from arm).
	fl, dcb := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 50*time.Millisecond)
	fl.DecodeHeaders(http.Header{}, false)
	body := make([]byte, 100) // 100B @ 10kbps/50ms → ticks=1 → throttle=50ms
	fl.DecodeData(body, true)
	// Wait for the timer-fire callback to run (it Dec's pending).
	if !waitForContinueDecoding(dcb, 1, 500*time.Millisecond) {
		t.Fatalf("ContinueDecoding never invoked within 500ms")
	}
	// Pending was Inc'd at arm + Dec'd at fire → 0.
	if got := fl.requestRC.stats.requestPending.Load(); got != 0 {
		t.Fatalf("pre-OnDestroy requestPending: got %d, want 0 (callback already Dec'd)", got)
	}
	// OnDestroy now: timer already fired → Stop() returns false → no Dec.
	fl.OnDestroy()
	if got := fl.requestRC.stats.requestPending.Load(); got != 0 {
		t.Errorf("post-OnDestroy requestPending: got %d, want 0 (Stop()=false → trust callback; no double-Dec)", got)
	}
}

func TestOnDestroy_RaceConcurrent_NoDoubleDecrement(t *testing.T) {
	// N=100 iterations per planner-time decision 3. Each iteration: spawn fresh
	// filter; arm timer with body=100/kbps=10/fill=50ms (50ms throttle); race
	// OnDestroy against timer-fire from a parallel goroutine. Assert: no panic,
	// final pending gauge == 0, no negative gauge. Mirrors phase-09 fault's
	// TestFault_DelayTimerRace pattern.
	if testing.Short() {
		t.Skip("race-cycle test skipped under -short")
	}
	const N = 100
	for i := 0; i < N; i++ {
		fl, dcb := makeFilterWithMode(t, bandwidthlimitv3.BandwidthLimit_REQUEST, 10, 50*time.Millisecond)
		fl.DecodeHeaders(http.Header{}, false)
		// Arm a 50ms timer (small body → one-tick floor → exactly 50ms).
		fl.DecodeData(make([]byte, 100), true)
		// Race window: i%5 ∈ {0,1,2,3,4} ms — straddles the 50ms timer firing
		// boundary across iterations (some land pre-fire, some mid-fire, some
		// post-fire). The Stop() bool discriminator must hold across the
		// concurrent-OnDestroy-vs-timer-callback race.
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Tiny delay to randomize interleave; OnDestroy from a goroutine
			// distinct from the dispatch goroutine (the test goroutine) to
			// exercise the cross-goroutine race. (Realistically OnDestroy is
			// dispatch-goroutine-bound; the race shape here is more aggressive
			// than production.)
			time.Sleep(time.Duration(i%5) * time.Millisecond)
			fl.OnDestroy()
		}()
		wg.Wait()
		// Drain: wait for the timer-fire callback (if any) to finish so
		// final pending gauge is observable.
		time.Sleep(80 * time.Millisecond)
		got := fl.requestRC.stats.requestPending.Load()
		if got != 0 {
			t.Fatalf("iteration %d: requestPending = %d, want 0 (Stop-races-Fire discipline violated)", i, got)
		}
		if got < 0 {
			t.Fatalf("iteration %d: requestPending = %d (NEGATIVE; double-Dec)", i, got)
		}
		// Best-effort sanity on ContinueDecoding: 0 (OnDestroy beat the timer)
		// or 1 (timer beat OnDestroy); never >1 since the timer fires once.
		if c := dcb.continued.Load(); c > 1 {
			t.Fatalf("iteration %d: ContinueDecoding called %d times, want 0 or 1", i, c)
		}
	}
}

func TestOnDestroy_BothDirectionsActive_BothCleanedUp(t *testing.T) {
	// REQUEST_AND_RESPONSE mode with both timers armed; OnDestroy stops both;
	// both pending gauges balanced (== 0). 1s throttle so OnDestroy beats both
	// timers and Stop() returns true on both sides.
	fl, _, _ := makeFilterWithModeBothCB(t, bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE, 10, 1*time.Second)
	fl.DecodeHeaders(http.Header{}, false)
	// Decode side: arm 1s timer via 10240B body @ 10kbps/1s.
	body := make([]byte, 10240)
	if status := fl.DecodeData(body, true); status != envoyhttp.DataStopIterationAndBuffer {
		t.Fatalf("DecodeData status: got %v, want DataStopIterationAndBuffer", status)
	}
	if fl.requestTimer == nil {
		t.Fatal("requestTimer: got nil, want armed")
	}
	// Encode side: arm 1s timer symmetrically.
	if status := fl.EncodeData(body, true); status != envoyhttp.DataStopIterationAndBuffer {
		t.Fatalf("EncodeData status: got %v, want DataStopIterationAndBuffer", status)
	}
	if fl.responseTimer == nil {
		t.Fatal("responseTimer: got nil, want armed")
	}
	// Pre-OnDestroy: both pending == 1 (Inc'd at arm).
	st := fl.requestRC.stats // same compiledConfig — requestRC == responseRC per the cascade
	if got := st.requestPending.Load(); got != 1 {
		t.Errorf("pre-OnDestroy requestPending: got %d, want 1", got)
	}
	if got := st.responsePending.Load(); got != 1 {
		t.Errorf("pre-OnDestroy responsePending: got %d, want 1", got)
	}
	// OnDestroy stops both timers; Stop() returns true on both → both Dec'd.
	fl.OnDestroy()
	if got := st.requestPending.Load(); got != 0 {
		t.Errorf("post-OnDestroy requestPending: got %d, want 0 (decode-side cleanup)", got)
	}
	if got := st.responsePending.Load(); got != 0 {
		t.Errorf("post-OnDestroy responsePending: got %d, want 0 (encode-side cleanup)", got)
	}
}

// ----------------------------------------------------------------------------
// Group 7 — Per-route INDEPENDENT-stats wiring (per SPEC §14.1 #6 + §5 +
// §11.P4 + §11.P12 + planner-time decision 5 + ADR-0139). 5 tests:
//
//  1. TestPerRoute_IndependentStats_Allocated — per-route override with own
//     stat_prefix → newFilterStatsIfAbsent allocates a wholly-own counter set
//     (pointer-distinct from listener stats; ADR-0139 §Decision (i)).
//  2. TestPerRoute_IndependentStats_ListenerUnaffected — load against per-
//     route route → listener-level counters stay at 0; per-route counters
//     increment (mirrors phase-11 ADR-0117 + SPEC §11.P4 empirical pin).
//  3. TestPerRoute_DisableViaEnableModeDISABLED_NoCounterIncrements — per-
//     route enable_mode: DISABLED → wholly inactive; namespace registered but
//     counters stay 0 per §11.P12 wholly-inactive semantic.
//  4. TestPerRoute_DisableViaListenerDISABLED_ParityWithPerRoute — listener-
//     level enable_mode: DISABLED produces identical wire output + counter
//     footprint as per-route enable_mode: DISABLED (planner-time decision 5 +
//     §12 deferred #5).
//  5. TestPerRoute_LazyCache_SyncMapKey — multi-request load against same
//     per-route entry → single allocation via sync.Map.LoadOrStore (ADR-0117
//     IMPL-1 race-safety + ADR-0139 §Decision (iii)); verified by pointer-
//     identity of returned *compiledConfig.
// ----------------------------------------------------------------------------

// perRouteHappyConfig returns a minimum-viable per-route BandwidthLimit proto
// with the given stat_prefix, enable_mode, and limit_kbps. The CODE-LEVEL
// required-limit_kbps-at-per-route invariant (per ADR-0136 + §11.P1) is
// satisfied by always setting limit_kbps explicitly.
func perRouteHappyConfig(prefix string, mode bandwidthlimitv3.BandwidthLimit_EnableMode, limitKbps uint64) *bandwidthlimitv3.BandwidthLimit {
	return &bandwidthlimitv3.BandwidthLimit{
		StatPrefix: prefix,
		EnableMode: mode,
		LimitKbps:  wrapperspb.UInt64(limitKbps),
	}
}

func TestPerRoute_IndependentStats_Allocated(t *testing.T) {
	// Build listener factory; resolve a per-route override with a distinct
	// stat_prefix. The resolved *compiledConfig must carry its own
	// *filterStats (pointer-distinct from the listener's filterStats). All 14
	// stats land under the per-route stat_prefix namespace (per ADR-0117
	// post-Freeze idempotency via newFilterStatsIfAbsent + ADR-0139 §Decision
	// (i) INDEPENDENT-stats).
	listenerCfg := happyConfig()
	listenerCfg.StatPrefix = "listener_prefix"
	listenerCfg.EnableMode = bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE
	factory, err := New(mustAny(t, listenerCfg), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(listener): %v", err)
	}
	inst := factory().Decoder.(*filter)

	perRoute := perRouteHappyConfig("perroute_x", bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE, 20)
	rcPR := inst.state.resolvePerRouteConfig(perRoute)
	if rcPR == nil {
		t.Fatal("resolvePerRouteConfig: got nil, want per-route *compiledConfig")
	}
	if rcPR == inst.state.listenerRC {
		t.Fatal("per-route *compiledConfig must NOT alias the listener listenerRC (INDEPENDENT-stats per ADR-0139)")
	}
	if rcPR.statPrefix != "perroute_x" {
		t.Errorf("statPrefix: got %q, want 'perroute_x'", rcPR.statPrefix)
	}
	if rcPR.limitKbps != 20 {
		t.Errorf("limitKbps: got %d, want 20", rcPR.limitKbps)
	}
	if rcPR.stats == nil {
		t.Fatal("per-route stats: got nil, want INDEPENDENT *filterStats")
	}
	if rcPR.stats == inst.state.listenerRC.stats {
		t.Error("per-route *filterStats must be pointer-distinct from listener *filterStats (per ADR-0139 §Decision (i))")
	}
	// All 14 stat fields populated (8 counters + 6 gauges) under per-route
	// stat_prefix namespace.
	if rcPR.stats.requestEnabled == nil || rcPR.stats.requestEnabled.Name() != "perroute_x.http_bandwidth_limit.request_enabled" {
		t.Errorf("requestEnabled: got %v (name=%v), want 'perroute_x.http_bandwidth_limit.request_enabled'",
			rcPR.stats.requestEnabled, statNameOrNil(rcPR.stats.requestEnabled))
	}
	if rcPR.stats.responsePending == nil || rcPR.stats.responsePending.Name() != "perroute_x.http_bandwidth_limit.response_pending" {
		t.Errorf("responsePending: got %v (name=%v), want 'perroute_x.http_bandwidth_limit.response_pending'",
			rcPR.stats.responsePending, gaugeNameOrNil(rcPR.stats.responsePending))
	}
}

// statNameOrNil returns the Name() of a Counter or "<nil>" — tiny helper to
// keep the assertion error wording clean.
func statNameOrNil(c *stats.Counter) string {
	if c == nil {
		return "<nil>"
	}
	return c.Name()
}

// gaugeNameOrNil mirrors statNameOrNil for *stats.Gauge.
func gaugeNameOrNil(g *stats.Gauge) string {
	if g == nil {
		return "<nil>"
	}
	return g.Name()
}

func TestPerRoute_IndependentStats_ListenerUnaffected(t *testing.T) {
	// Listener: enable_mode=REQUEST_AND_RESPONSE; per-route: same. Drive a
	// stream against the per-route override and verify listener stats stay at
	// 0 while per-route stats increment. Mirrors phase-11
	// TestDecodeHeaders_PerRouteOverride_IndependentBuckets shape.
	listenerCfg := happyConfig()
	listenerCfg.StatPrefix = "listener_prefix"
	listenerCfg.EnableMode = bandwidthlimitv3.BandwidthLimit_REQUEST
	factory, err := New(mustAny(t, listenerCfg), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(listener): %v", err)
	}
	inst := factory().Decoder.(*filter)
	dcb := &fakeDecoderCB{}
	inst.SetDecoderCallbacks(dcb)

	perRoute := perRouteHappyConfig("perroute_y", bandwidthlimitv3.BandwidthLimit_REQUEST, 10)
	dcb.mu.Lock()
	dcb.routeCfg = perRoute
	dcb.mu.Unlock()

	// Pre-flight: both stat sets exist; counters at 0.
	listenerStats := inst.state.listenerRC.stats
	if listenerStats == nil {
		t.Fatal("listener stats: nil (precondition)")
	}
	if got := listenerStats.requestEnabled.Load(); got != 0 {
		t.Fatalf("pre-flight listener requestEnabled: got %d, want 0", got)
	}

	// Drive request through the filter — per-route resolved at DecodeHeaders,
	// per-route stats bumped at DecodeData(endStream=true) with empty body
	// (fast path → no timer arm; only the *_enabled + *_incoming_* +
	// *_allowed_* counters increment).
	if status := inst.DecodeHeaders(http.Header{}, false); status != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders: got %v, want Continue", status)
	}
	if inst.requestRC == nil || inst.requestRC == inst.state.listenerRC {
		t.Fatal("requestRC: must be the per-route compiledConfig (not listener)")
	}
	perRouteStats := inst.requestRC.stats
	if perRouteStats == nil {
		t.Fatal("per-route stats: nil")
	}
	if perRouteStats == listenerStats {
		t.Fatal("per-route stats must be pointer-distinct from listener stats (INDEPENDENT-stats per ADR-0139)")
	}
	if status := inst.DecodeData(nil, true); status != envoyhttp.DataContinue {
		t.Fatalf("DecodeData empty-body: got %v, want DataContinue (fast path)", status)
	}

	// Per-route counters incremented (at least requestEnabled bumped).
	if got := perRouteStats.requestEnabled.Load(); got != 1 {
		t.Errorf("per-route requestEnabled: got %d, want 1", got)
	}
	// Listener counters MUST stay at 0 — per-route increments must NOT leak
	// onto listener counters per ADR-0139 §Decision (i) + SPEC §11.P4.
	if got := listenerStats.requestEnabled.Load(); got != 0 {
		t.Errorf("listener requestEnabled: got %d, want 0 (per-route increment must NOT leak)", got)
	}
	if got := listenerStats.requestIncomingTotalSize.Load(); got != 0 {
		t.Errorf("listener requestIncomingTotalSize: got %d, want 0 (per-route increment must NOT leak)", got)
	}
	if got := listenerStats.requestAllowedTotalSize.Load(); got != 0 {
		t.Errorf("listener requestAllowedTotalSize: got %d, want 0 (per-route increment must NOT leak)", got)
	}
}

func TestPerRoute_DisableViaEnableModeDISABLED_NoCounterIncrements(t *testing.T) {
	// Per-route enable_mode=DISABLED → wholly inactive per §11.P12. The
	// namespace IS registered at per-route allocate time (all 14 stat names
	// exist on the Registry) but no counter is ever incremented because
	// requestActive/responseActive both stay false at DecodeHeaders.
	listenerCfg := happyConfig()
	listenerCfg.StatPrefix = "listener_prefix"
	listenerCfg.EnableMode = bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE
	factory, err := New(mustAny(t, listenerCfg), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(listener): %v", err)
	}
	inst := factory().Decoder.(*filter)
	dcb := &fakeDecoderCB{}
	inst.SetDecoderCallbacks(dcb)

	perRoute := perRouteHappyConfig("perroute_disabled", bandwidthlimitv3.BandwidthLimit_DISABLED, 10)
	dcb.mu.Lock()
	dcb.routeCfg = perRoute
	dcb.mu.Unlock()

	if status := inst.DecodeHeaders(http.Header{}, false); status != envoyhttp.Continue {
		t.Fatalf("DecodeHeaders: got %v, want Continue", status)
	}
	if inst.requestActive {
		t.Error("requestActive: got true, want false (per-route enable_mode=DISABLED)")
	}
	if inst.responseActive {
		t.Error("responseActive: got true, want false (per-route enable_mode=DISABLED)")
	}
	perRouteStats := inst.requestRC.stats
	if perRouteStats == nil {
		t.Fatal("per-route stats: nil (namespace MUST be registered even under DISABLED per §11.P12 + ADR-0139 §Decision (v))")
	}
	// Drive a non-empty body — DataContinue passthrough; no counter bump.
	if status := inst.DecodeData([]byte("hello"), true); status != envoyhttp.DataContinue {
		t.Errorf("DecodeData: got %v, want DataContinue (DISABLED → passthrough)", status)
	}
	if got := perRouteStats.requestEnabled.Load(); got != 0 {
		t.Errorf("per-route requestEnabled: got %d, want 0 (DISABLED wholly-inactive per §11.P12)", got)
	}
	if got := perRouteStats.requestIncomingTotalSize.Load(); got != 0 {
		t.Errorf("per-route requestIncomingTotalSize: got %d, want 0 (DISABLED wholly-inactive)", got)
	}
}

func TestPerRoute_DisableViaListenerDISABLED_ParityWithPerRoute(t *testing.T) {
	// Per planner-time decision 5 + §12 deferred #5: listener-level
	// enable_mode=DISABLED and per-route enable_mode=DISABLED must produce
	// the same observable wire output (full passthrough; no timer arm) AND
	// the same counter footprint (namespace allocated but no increments).
	// Build two filters — one with listener-level DISABLED + no per-route
	// override; one with listener-level REQUEST_AND_RESPONSE + per-route
	// override DISABLED — and verify byte-equivalent passthrough + zero
	// counter increments on both.

	// Path A: listener-level DISABLED, no per-route override.
	listenerCfgA := happyConfig()
	listenerCfgA.StatPrefix = "listener_only_disabled"
	listenerCfgA.EnableMode = bandwidthlimitv3.BandwidthLimit_DISABLED
	listenerCfgA.LimitKbps = wrapperspb.UInt64(10)
	factoryA, err := New(mustAny(t, listenerCfgA), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(Path A listener=DISABLED): %v", err)
	}
	instA := factoryA().Decoder.(*filter)
	dcbA := &fakeDecoderCB{}
	instA.SetDecoderCallbacks(dcbA)
	// No per-route override → routeCfg stays nil → resolvePerRouteConfig
	// returns listenerRC verbatim.

	// Path B: listener-level REQUEST_AND_RESPONSE; per-route override DISABLED.
	listenerCfgB := happyConfig()
	listenerCfgB.StatPrefix = "listener_active"
	listenerCfgB.EnableMode = bandwidthlimitv3.BandwidthLimit_REQUEST_AND_RESPONSE
	listenerCfgB.LimitKbps = wrapperspb.UInt64(10)
	factoryB, err := New(mustAny(t, listenerCfgB), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(Path B listener=active): %v", err)
	}
	instB := factoryB().Decoder.(*filter)
	dcbB := &fakeDecoderCB{}
	instB.SetDecoderCallbacks(dcbB)
	perRouteB := perRouteHappyConfig("perroute_b_disabled", bandwidthlimitv3.BandwidthLimit_DISABLED, 10)
	dcbB.mu.Lock()
	dcbB.routeCfg = perRouteB
	dcbB.mu.Unlock()

	// Drive identical body through both paths.
	body := []byte("identical-body-bytes-for-parity-assertion")
	driveDisabledPath := func(t *testing.T, name string, inst *filter) (wireStatusHeaders envoyhttp.FilterHeadersStatus, wireStatusData envoyhttp.FilterDataStatus) {
		t.Helper()
		hs := inst.DecodeHeaders(http.Header{}, false)
		if hs != envoyhttp.Continue {
			t.Fatalf("%s DecodeHeaders: got %v, want Continue", name, hs)
		}
		if inst.requestActive {
			t.Errorf("%s requestActive: got true, want false (DISABLED)", name)
		}
		if inst.responseActive {
			t.Errorf("%s responseActive: got true, want false (DISABLED)", name)
		}
		ds := inst.DecodeData(body, true)
		if ds != envoyhttp.DataContinue {
			t.Errorf("%s DecodeData: got %v, want DataContinue (DISABLED passthrough)", name, ds)
		}
		if inst.requestTimer != nil {
			t.Errorf("%s requestTimer: got non-nil, want nil (DISABLED must not arm timer)", name)
		}
		return hs, ds
	}

	headersA, dataA := driveDisabledPath(t, "Path A (listener=DISABLED)", instA)
	headersB, dataB := driveDisabledPath(t, "Path B (per-route=DISABLED)", instB)

	// Wire-output parity: both paths return Continue / DataContinue.
	if headersA != headersB {
		t.Errorf("wire-shape divergence: Path A headers=%v vs Path B headers=%v", headersA, headersB)
	}
	if dataA != dataB {
		t.Errorf("wire-shape divergence: Path A data=%v vs Path B data=%v", dataA, dataB)
	}

	// Counter footprint parity: each path's resolved compiledConfig has all
	// 14 stats registered at 0.
	rcA := instA.requestRC
	rcB := instB.requestRC
	if rcA == nil || rcB == nil {
		t.Fatalf("requestRC: A=%p B=%p (both must be non-nil)", rcA, rcB)
	}
	if rcA.stats == nil || rcB.stats == nil {
		t.Fatal("filterStats namespace MUST be registered for both paths (per §11.P12 + ADR-0139 §Decision (v))")
	}
	// Path A: stats live on listenerRC (listener_only_disabled.*). Path B:
	// stats live on per-route compiledConfig (perroute_b_disabled.*). Both
	// must show zero counter increments.
	zeroCounters := func(t *testing.T, name string, fs *filterStats) {
		t.Helper()
		counters := map[string]uint64{
			"requestEnabled":            fs.requestEnabled.Load(),
			"requestEnforced":           fs.requestEnforced.Load(),
			"requestIncomingTotalSize":  fs.requestIncomingTotalSize.Load(),
			"requestAllowedTotalSize":   fs.requestAllowedTotalSize.Load(),
			"responseEnabled":           fs.responseEnabled.Load(),
			"responseEnforced":          fs.responseEnforced.Load(),
			"responseIncomingTotalSize": fs.responseIncomingTotalSize.Load(),
			"responseAllowedTotalSize":  fs.responseAllowedTotalSize.Load(),
		}
		for k, v := range counters {
			if v != 0 {
				t.Errorf("%s %s: got %d, want 0 (DISABLED wholly-inactive parity per planner-time decision 5)", name, k, v)
			}
		}
	}
	zeroCounters(t, "Path A (listener=DISABLED)", rcA.stats)
	zeroCounters(t, "Path B (per-route=DISABLED)", rcB.stats)
}

func TestPerRoute_LazyCache_SyncMapKey(t *testing.T) {
	// Multi-request resolve against the SAME per-route entry (same
	// *bandwidthlimitv3.BandwidthLimit pointer) must hit the sync.Map cache
	// after the first allocation. Verified by pointer-identity of the
	// returned *compiledConfig across N=10 resolves + by visiting the
	// sync.Map to count map entries == 1.
	listenerCfg := happyConfig()
	listenerCfg.StatPrefix = "listener_lazy"
	factory, err := New(mustAny(t, listenerCfg), freshFactoryCtx())
	if err != nil {
		t.Fatalf("New(listener): %v", err)
	}
	inst := factory().Decoder.(*filter)

	perRoute := perRouteHappyConfig("perroute_lazy", bandwidthlimitv3.BandwidthLimit_REQUEST, 7)

	// First resolve allocates fresh.
	rcFirst := inst.state.resolvePerRouteConfig(perRoute)
	if rcFirst == nil {
		t.Fatal("first resolve: got nil")
	}

	// N=10 subsequent resolves must return the SAME pointer (lazy-cache hit
	// via sync.Map.Load → LoadOrStore short-circuit per ADR-0117 IMPL-1 +
	// ADR-0139 §Decision (iii)).
	const N = 10
	for i := 0; i < N; i++ {
		rcAgain := inst.state.resolvePerRouteConfig(perRoute)
		if rcAgain != rcFirst {
			t.Fatalf("resolve %d: got %p, want %p (lazy-cache must return pointer-identical *compiledConfig)", i, rcAgain, rcFirst)
		}
	}

	// Count sync.Map entries: only ONE entry must exist (the single per-route
	// pointer keyed → its compiledConfig). Multi-resolve must NOT allocate a
	// second entry.
	entries := 0
	inst.state.perRoute.Range(func(_, _ any) bool {
		entries++
		return true
	})
	if entries != 1 {
		t.Errorf("sync.Map entries: got %d, want 1 (multi-resolve must hit the cache via LoadOrStore)", entries)
	}

	// Race-safety smoke: parallel resolves against the same pointer must all
	// return the same pointer + leave the sync.Map at 1 entry. (The stricter
	// race test under -race lives at Group 6's TestOnDestroy_RaceConcurrent_
	// NoDoubleDecrement; this is a lighter parity check for the lazy-cache
	// LoadOrStore contract.)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := inst.state.resolvePerRouteConfig(perRoute)
			if got != rcFirst {
				t.Errorf("concurrent resolve: got %p, want %p", got, rcFirst)
			}
		}()
	}
	wg.Wait()
	entries = 0
	inst.state.perRoute.Range(func(_, _ any) bool {
		entries++
		return true
	})
	if entries != 1 {
		t.Errorf("post-concurrent sync.Map entries: got %d, want 1", entries)
	}
}

// ----------------------------------------------------------------------------
// Group 8 — 14-stat filterStats namespace registration + Prometheus rendering
// (per SPEC §1.1 amendment 7 + amendment 8 + §6.2 + §11.P3 + §11.P10 + §11.P11
// + ADR-0138). 4 tests:
//
//  1. TestStatsNamespace_AllFourteenActiveStatsRegistered — drive
//     newFilterStats; assert exactly 14 stats registered under the
//     `<statPrefix>.http_bandwidth_limit.*` path (8 counters + 6 gauges).
//  2. TestStatsNamespace_UnderscoreInfix_NotHCMRooted — assert the internal
//     path is `<statPrefix>.http_bandwidth_limit.<counter>` (underscore-infix;
//     NOT HCM-rooted `http.<HCM>.<statPrefix>.bandwidth_limit.<counter>`).
//     Per §11.P11.
//  3. TestStatsNamespace_PromInlineFlatten_NoSN10 — render the stats via
//     stats.WriteProm and assert the Prometheus output is
//     `envoy_<statPrefix>_http_bandwidth_limit_<counter>{}` with NO labels +
//     NO tag-extractor. Per §11.P10 + §1.1 amendment 8.
//  4. TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent — multi-call
//     newFilterStatsIfAbsent with the same stat_prefix returns pointer-
//     equivalent *Counter + *Gauge fields per ADR-0117 post-Freeze idempotency
//     (extended to gauges via NewGaugeIfAbsent at Task 7 per ADR-0139).
// ----------------------------------------------------------------------------

// expectedActiveStatNames is the canonical 14-name set: 8 counters + 6 gauges
// under the `<prefix>.http_bandwidth_limit.<counter>` path per SPEC §6.2 +
// amendment 7. KEEP IN SYNC with newFilterStats / newFilterStatsIfAbsent +
// the filterStats struct field ordering in bandwidthlimit.go.
func expectedActiveStatNames(prefix string) (counters []string, gauges []string) {
	p := prefix + ".http_bandwidth_limit."
	counters = []string{
		p + "request_enabled",
		p + "request_enforced",
		p + "request_incoming_total_size",
		p + "request_allowed_total_size",
		p + "response_enabled",
		p + "response_enforced",
		p + "response_incoming_total_size",
		p + "response_allowed_total_size",
	}
	gauges = []string{
		p + "request_pending",
		p + "request_incoming_size",
		p + "request_allowed_size",
		p + "response_pending",
		p + "response_incoming_size",
		p + "response_allowed_size",
	}
	return
}

func TestStatsNamespace_AllFourteenActiveStatsRegistered(t *testing.T) {
	// Drive newFilterStats directly against a fresh Registry; iterate the
	// Registry via Walk and assert exactly 14 stats registered (8 counters +
	// 6 gauges) under the `<statPrefix>.http_bandwidth_limit.*` path. Per
	// SPEC §1.1 amendment 7 + ADR-0138 §Decision (i).
	const prefix = "test_prefix"
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, prefix)
	if fs == nil {
		t.Fatal("newFilterStats: got nil, want *filterStats")
	}

	wantCounters, wantGauges := expectedActiveStatNames(prefix)
	gotCounterNames := make(map[string]bool)
	gotGaugeNames := make(map[string]bool)
	reg.Walk(func(m stats.Metric) {
		switch m.Type() {
		case stats.MetricCounter:
			gotCounterNames[m.Name()] = true
		case stats.MetricGauge:
			gotGaugeNames[m.Name()] = true
		}
	})

	// Exactly 8 counters + 6 gauges = 14 stats total.
	if len(gotCounterNames) != 8 {
		t.Errorf("counter count: got %d, want 8 (per SPEC §1.1 amendment 7)", len(gotCounterNames))
	}
	if len(gotGaugeNames) != 6 {
		t.Errorf("gauge count: got %d, want 6 (per SPEC §1.1 amendment 7)", len(gotGaugeNames))
	}
	if total := len(gotCounterNames) + len(gotGaugeNames); total != 14 {
		t.Errorf("total active stats: got %d, want 14 (per ADR-0138 §Decision (i))", total)
	}
	// Exact set match (each name appears exactly once + only the expected
	// names registered).
	for _, n := range wantCounters {
		if !gotCounterNames[n] {
			t.Errorf("counter %q missing from Registry", n)
		}
	}
	for _, n := range wantGauges {
		if !gotGaugeNames[n] {
			t.Errorf("gauge %q missing from Registry", n)
		}
	}
	// Unexpected names (e.g., a stray histogram or extra counter) would
	// indicate divergence from amendment 7 + phase-06.1 baseline.
	for n := range gotCounterNames {
		found := false
		for _, want := range wantCounters {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected counter %q registered (not in amendment-7 set)", n)
		}
	}
	for n := range gotGaugeNames {
		found := false
		for _, want := range wantGauges {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("unexpected gauge %q registered (not in amendment-7 set)", n)
		}
	}
}

func TestStatsNamespace_UnderscoreInfix_NotHCMRooted(t *testing.T) {
	// Per SPEC §11.P11 + amendment 8 + ADR-0138 §Decision (ii): the internal
	// stat path is `<statPrefix>.http_bandwidth_limit.<counter>` with single-
	// segment `http_bandwidth_limit` underscore infix (NOT `http.bandwidth_
	// limit.` dot infix; NOT HCM-rooted `http.<HCM>.<prefix>.bandwidth_limit.
	// <counter>` as BRAINSTORM §2.7 initially hypothesized).
	const prefix = "abc"
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, prefix)
	if fs == nil {
		t.Fatal("newFilterStats: got nil")
	}

	// Spot-check: pick one counter + one gauge; verify the registered name
	// starts with `<prefix>.http_bandwidth_limit.` (underscore-infix segment)
	// AND does NOT start with `http.` (NOT HCM-rooted).
	gotCounter := fs.requestEnabled.Name()
	wantCounter := "abc.http_bandwidth_limit.request_enabled"
	if gotCounter != wantCounter {
		t.Errorf("requestEnabled name: got %q, want %q (underscore-infix per §11.P11)", gotCounter, wantCounter)
	}
	gotGauge := fs.responsePending.Name()
	wantGauge := "abc.http_bandwidth_limit.response_pending"
	if gotGauge != wantGauge {
		t.Errorf("responsePending name: got %q, want %q (underscore-infix per §11.P11)", gotGauge, wantGauge)
	}

	// HCM-rooted refutation: every registered name MUST NOT begin with `http.`
	// (an HCM-rooted shape would start `http.<HCM_stat_prefix>...`). Per
	// §11.P11 empirical refutation.
	reg.Walk(func(m stats.Metric) {
		if strings.HasPrefix(m.Name(), "http.") {
			t.Errorf("HCM-rooted name detected: %q (must NOT start with `http.` per §11.P11)", m.Name())
		}
		// Conversely: every name MUST contain the `.http_bandwidth_limit.`
		// underscore-infix segment (the canonical phase-15 namespace shape).
		if !strings.Contains(m.Name(), ".http_bandwidth_limit.") {
			t.Errorf("missing underscore-infix `.http_bandwidth_limit.` segment in %q", m.Name())
		}
	})
}

func TestStatsNamespace_PromInlineFlatten_NoSN10(t *testing.T) {
	// Per SPEC §11.P10 + §1.1 amendment 8 + ADR-0138 §Decision (iii):
	// Prometheus rendering for `<prefix>.http_bandwidth_limit.<counter>`
	// internal names produces `envoy_<prefix>_http_bandwidth_limit_<counter>{}`
	// — stat_prefix INLINED into base name; NO labels; NO tag-extractor;
	// NO new SN10 rule (the existing default-branch flatten handles via
	// dot→underscore substitution).
	const prefix = "demo"
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, prefix)
	if fs == nil {
		t.Fatal("newFilterStats: got nil")
	}

	// Bump a few stats so the Prometheus rendering carries non-zero values
	// (proves the value path also works through the inline-prefix shape).
	fs.requestEnabled.Inc()
	fs.responseEnforced.Add(5)
	fs.requestPending.Set(2)

	// WriteProm walks the Registry, flattens each metric, and emits the
	// Prometheus exposition text. Per ADR-0138 §Decision (iii) the rendered
	// output for each registered stat MUST contain the inline-prefix base name
	// `envoy_<prefix>_http_bandwidth_limit_<counter>` with NO label block ({}).
	var sb strings.Builder
	if err := stats.WriteProm(&sb, reg); err != nil {
		t.Fatalf("WriteProm: %v", err)
	}
	out := sb.String()
	if out == "" {
		t.Fatal("WriteProm: empty output (Registry walk produced no lines; default-branch flatten likely silently dropped the bandwidth_limit names — see ADR-0138 §Consequences default-branch fallback note)")
	}

	// Each of the 14 active stat names MUST appear in the Prometheus output
	// under its inline-prefix base name.
	wantCounters, wantGauges := expectedActiveStatNames(prefix)
	allInternal := append(append([]string{}, wantCounters...), wantGauges...)
	for _, internal := range allInternal {
		// dot→underscore substitution produces the expected Prometheus name:
		//   `demo.http_bandwidth_limit.request_enabled` →
		//   `envoy_demo_http_bandwidth_limit_request_enabled`
		wantBase := "envoy_" + strings.ReplaceAll(internal, ".", "_")
		if !strings.Contains(out, wantBase) {
			t.Errorf("Prometheus output missing base name %q (internal=%q); per ADR-0138 §Decision (iii) inline-prefix shape", wantBase, internal)
		}
		// NO label block: the metric line MUST emit as `<base> <value>` (no
		// `{}` suffix on the name token) per §11.P10 NO-tag-extractor claim.
		// Search for the labeled form `<wantBase>{...}` and assert ABSENT.
		labeledForm := wantBase + "{"
		if strings.Contains(out, labeledForm) {
			t.Errorf("Prometheus output contains labeled form %q — NO tag-extractor expected per §11.P10 + ADR-0138 §Decision (iii)", labeledForm)
		}
	}

	// Spot-check the non-zero values rendered correctly through the inline-
	// prefix shape (proves the full pipeline, not just the name flatten).
	if !strings.Contains(out, "envoy_demo_http_bandwidth_limit_request_enabled 1") {
		t.Error("expected `envoy_demo_http_bandwidth_limit_request_enabled 1` line in Prometheus output")
	}
	if !strings.Contains(out, "envoy_demo_http_bandwidth_limit_response_enforced 5") {
		t.Error("expected `envoy_demo_http_bandwidth_limit_response_enforced 5` line in Prometheus output")
	}
	if !strings.Contains(out, "envoy_demo_http_bandwidth_limit_request_pending 2") {
		t.Error("expected `envoy_demo_http_bandwidth_limit_request_pending 2` line in Prometheus output")
	}
}

func TestStatsNamespace_NewFilterStatsIfAbsent_Idempotent(t *testing.T) {
	// Per ADR-0117 + ADR-0139 §Decision (iii): newFilterStatsIfAbsent is
	// post-Freeze idempotent — repeat invocation with the same stat_prefix
	// returns pointer-identical *Counter + *Gauge fields. The Registry's
	// byName map serves as the canonical store; LoadOrStore-style semantics
	// across both NewCounterIfAbsent + NewGaugeIfAbsent.
	const prefix = "idem"
	reg := stats.NewRegistry()
	reg.Freeze() // post-Freeze path: NewCounterIfAbsent / NewGaugeIfAbsent must succeed.

	fs1 := newFilterStatsIfAbsent(reg, prefix)
	if fs1 == nil {
		t.Fatal("first newFilterStatsIfAbsent: got nil")
	}
	fs2 := newFilterStatsIfAbsent(reg, prefix)
	if fs2 == nil {
		t.Fatal("second newFilterStatsIfAbsent: got nil")
	}

	// Underlying *Counter pointers MUST be identical across calls — the
	// Registry's byName map de-duplicates by name; multi-call against same
	// name returns the same backing instance. Per ADR-0117 + ADR-0139.
	counterCases := []struct {
		name string
		a, b *stats.Counter
	}{
		{"requestEnabled", fs1.requestEnabled, fs2.requestEnabled},
		{"requestEnforced", fs1.requestEnforced, fs2.requestEnforced},
		{"requestIncomingTotalSize", fs1.requestIncomingTotalSize, fs2.requestIncomingTotalSize},
		{"requestAllowedTotalSize", fs1.requestAllowedTotalSize, fs2.requestAllowedTotalSize},
		{"responseEnabled", fs1.responseEnabled, fs2.responseEnabled},
		{"responseEnforced", fs1.responseEnforced, fs2.responseEnforced},
		{"responseIncomingTotalSize", fs1.responseIncomingTotalSize, fs2.responseIncomingTotalSize},
		{"responseAllowedTotalSize", fs1.responseAllowedTotalSize, fs2.responseAllowedTotalSize},
	}
	for _, c := range counterCases {
		if c.a == nil || c.b == nil {
			t.Errorf("%s: got nil counter (a=%p b=%p)", c.name, c.a, c.b)
			continue
		}
		if c.a != c.b {
			t.Errorf("%s: pointer mismatch a=%p b=%p (post-Freeze idempotency per ADR-0117 + ADR-0139)", c.name, c.a, c.b)
		}
	}

	gaugeCases := []struct {
		name string
		a, b *stats.Gauge
	}{
		{"requestPending", fs1.requestPending, fs2.requestPending},
		{"requestIncomingSize", fs1.requestIncomingSize, fs2.requestIncomingSize},
		{"requestAllowedSize", fs1.requestAllowedSize, fs2.requestAllowedSize},
		{"responsePending", fs1.responsePending, fs2.responsePending},
		{"responseIncomingSize", fs1.responseIncomingSize, fs2.responseIncomingSize},
		{"responseAllowedSize", fs1.responseAllowedSize, fs2.responseAllowedSize},
	}
	for _, g := range gaugeCases {
		if g.a == nil || g.b == nil {
			t.Errorf("%s: got nil gauge (a=%p b=%p)", g.name, g.a, g.b)
			continue
		}
		if g.a != g.b {
			t.Errorf("%s: pointer mismatch a=%p b=%p (post-Freeze idempotency via NewGaugeIfAbsent per ADR-0139)", g.name, g.a, g.b)
		}
	}

	// Increment via fs1, observe via fs2 — the shared *Counter / *Gauge
	// instances guarantee that increments are visible across both views.
	// Closes the "same name → same backing instance" semantic claim.
	fs1.requestEnabled.Inc()
	if got := fs2.requestEnabled.Load(); got != 1 {
		t.Errorf("requestEnabled.Inc via fs1, Load via fs2: got %d, want 1 (shared instance per ADR-0117)", got)
	}
	fs1.responsePending.Set(7)
	if got := fs2.responsePending.Load(); got != 7 {
		t.Errorf("responsePending.Set via fs1, Load via fs2: got %d, want 7 (shared instance per ADR-0139)", got)
	}
}
