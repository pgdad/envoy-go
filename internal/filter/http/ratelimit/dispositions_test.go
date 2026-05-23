package ratelimit

// dispositions_test.go — TDD coverage for the §4.6 OK/OVER_LIMIT/error
// dispositions per phase-24.1 PLAN Task 7 Step 1. Mandatory tests:
//
//   - TestDispositions_OK_Continue:         OK + RLS request_headers_to_add
//     applied to the request map + ok counter inc + ContinueDecoding.
//   - TestDispositions_OverLimit_429_ByteShape: status from cc.rateLimitedStatus,
//     RawBody body, AMEND-8 header order
//     (x-envoy-ratelimited / RLS / config), over_limit counter inc.
//   - TestDispositions_Error_FailOpen: nil resp + non-nil err with default
//     failure_mode_deny=false → error + failure_mode_allowed counters + Continue.
//   - TestDispositions_Error_FailClosed: nil resp + non-nil err with
//     failure_mode_deny=true → error counter only + SendLocalReply(statusOnError, "", nil).
//   - TestDispositions_GRPC_8_vs_14: documentary — the gRPC code surfaces in
//     the dispositions struct mapping (RESOURCE_EXHAUSTED=8 vs UNAVAILABLE=14
//     per the rate_limited_as_resource_exhausted gate). This is wire-level
//     metadata that the 3-arg SendLocalReply API cannot surface; the test
//     pins the mapping helper rateLimitedAsResourceExhaustedGrpcCode for
//     forward 24.2 consumption.
//
// Note on rc-details: the parent SPEC §4.7 byte-shape specifies the
// "request_rate_limited" / "rate_limiter_error" response_code_details strings.
// The 3-arg envoyhttp SendLocalReply API at internal/filter/http/callbacks.go:35
// does NOT carry an rc-details slot — these strings are ABSENT-BY-API at 24.1
// per the admission_control precedent at admission_control/decode_headers.go:25.
// The string constants are pinned at the package level (Task 7 dispositions.go)
// for forward 24.2 consumption when the API extends.

import (
	"context"
	"net/http"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/stats"
)

// ----------------------------------------------------------------------------
// Helper: makeCountedStats wraps a *filterStats with real *stats.Counter
// instances so assertions can call .Load() to inspect counter values.
// ----------------------------------------------------------------------------

func makeCountedStats(t *testing.T) *filterStats {
	t.Helper()
	reg := stats.NewRegistry()
	return newFilterStats(reg, "rls_cluster", "")
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_OK_Continue
// ----------------------------------------------------------------------------

// TestDispositions_OK_Continue verifies the OK arm per parent SPEC §4.6 step 1:
//
//  1. ok counter incremented.
//  2. RLS RequestHeadersToAdd applied to the request header map (these go
//     UPSTREAM via the existing http.Header reference per ext_authz
//     applyUpstreamMutations precedent).
//  3. dcb.ContinueDecoding() called (no SendLocalReply).
func TestDispositions_OK_Continue(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain: "test",
		stats:  fs,
	}
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	resp := &ratelimitservicev3.RateLimitResponse{
		OverallCode: ratelimitservicev3.RateLimitResponse_OK,
		RequestHeadersToAdd: []*corev3.HeaderValue{
			{Key: "X-RateLimit-Source", Value: "edge-rls"},
		},
	}

	f.applyDisposition(headers, resp, nil)

	if got := fs.ok.Load(); got != 1 {
		t.Errorf("ok counter: got %d, want 1", got)
	}
	if got := dcb.snapshotContinueCount(); got != 1 {
		t.Errorf("ContinueDecoding: got %d, want 1", got)
	}
	count, _ := dcb.snapshotLocalReply()
	if count != 0 {
		t.Errorf("SendLocalReply: got %d, want 0 (OK disposition)", count)
	}
	if got := headers.Get("X-RateLimit-Source"); got != "edge-rls" {
		t.Errorf("request header X-RateLimit-Source: got %q, want %q (RLS RequestHeadersToAdd applied to upstream request)", got, "edge-rls")
	}

	if got := fs.overLimit.Load(); got != 0 {
		t.Errorf("over_limit counter: got %d, want 0 (OK arm)", got)
	}
	if got := fs.error.Load(); got != 0 {
		t.Errorf("error counter: got %d, want 0 (OK arm)", got)
	}
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_OverLimit_429_ByteShape
// ----------------------------------------------------------------------------

// TestDispositions_OverLimit_429_ByteShape verifies the OVER_LIMIT byte-shape
// per parent SPEC §4.7 + AMEND-8:
//
//  1. over_limit counter incremented.
//  2. SendLocalReply with status cc.rateLimitedStatus (default 429).
//  3. Body = RLS RawBody.
//  4. Headers in AMEND-8 order:
//     [0] x-envoy-ratelimited: true (unless disableXEnvoyRateLimitedHeader)
//     [1..] RLS ResponseHeadersToAdd (in RLS-given order)
//     [k..] config responseHeadersToAdd (in config order)
func TestDispositions_OverLimit_429_ByteShape(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain:            "test",
		stats:             fs,
		rateLimitedStatus: 429,
		responseHeadersToAdd: []headerKV{
			{name: "X-From-Config", value: "config-1"},
			{name: "X-From-Config-2", value: "config-2"},
		},
	}
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	resp := &ratelimitservicev3.RateLimitResponse{
		OverallCode: ratelimitservicev3.RateLimitResponse_OVER_LIMIT,
		RawBody:     []byte("rate limited"),
		ResponseHeadersToAdd: []*corev3.HeaderValue{
			{Key: "X-From-Rls", Value: "rls-1"},
			{Key: "X-Retry-After", Value: "60"},
		},
	}

	f.applyDisposition(headers, resp, nil)

	if got := fs.overLimit.Load(); got != 1 {
		t.Errorf("over_limit counter: got %d, want 1", got)
	}
	if got := fs.ok.Load(); got != 0 {
		t.Errorf("ok counter: got %d, want 0 (OVER_LIMIT arm)", got)
	}
	// OVER_LIMIT must call ContinueDecoding AFTER SendLocalReply to wake the
	// parked dispatch goroutine (chain.go:316-325 parkDecode); the chain's
	// localReplyDone gate ensures the resumed iteration short-circuits without
	// dialing upstream. Mirrors ext_authz extauthz.go:1097-1111 + fault
	// fault.go:299-324 precedents.
	if got := dcb.snapshotContinueCount(); got != 1 {
		t.Errorf("ContinueDecoding: got %d, want 1 (wakes parked dispatch goroutine after SendLocalReply)", got)
	}

	count, args := dcb.snapshotLocalReply()
	if count != 1 {
		t.Fatalf("SendLocalReply: got %d, want 1", count)
	}
	if args.status != 429 {
		t.Errorf("SendLocalReply status: got %d, want 429", args.status)
	}
	if args.body != "rate limited" {
		t.Errorf("SendLocalReply body: got %q, want %q (RLS RawBody)", args.body, "rate limited")
	}

	// AMEND-8 header order assertion.
	want := []envoyhttp.HeaderField{
		{Name: "x-envoy-ratelimited", Value: "true"},
		{Name: "X-From-Rls", Value: "rls-1"},
		{Name: "X-Retry-After", Value: "60"},
		{Name: "X-From-Config", Value: "config-1"},
		{Name: "X-From-Config-2", Value: "config-2"},
	}
	if len(args.headers) != len(want) {
		t.Fatalf("SendLocalReply headers: got %d entries, want %d (entries=%+v)", len(args.headers), len(want), args.headers)
	}
	for i, hf := range args.headers {
		if hf.Name != want[i].Name || hf.Value != want[i].Value {
			t.Errorf("header[%d]: got {%q, %q}, want {%q, %q}", i, hf.Name, hf.Value, want[i].Name, want[i].Value)
		}
	}
}

// TestDispositions_OverLimit_DisableXEnvoyRateLimited verifies the
// disable_x_envoy_ratelimited_header gate per parent SPEC §4.7 +
// compiled_config.go disableXEnvoyRateLimitedHeader: when true the
// x-envoy-ratelimited header is OMITTED (AMEND-8 first slot is dropped).
func TestDispositions_OverLimit_DisableXEnvoyRateLimited(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain:                         "test",
		stats:                          fs,
		rateLimitedStatus:              429,
		disableXEnvoyRateLimitedHeader: true,
	}
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	resp := &ratelimitservicev3.RateLimitResponse{
		OverallCode: ratelimitservicev3.RateLimitResponse_OVER_LIMIT,
	}
	f.applyDisposition(headers, resp, nil)

	_, args := dcb.snapshotLocalReply()
	for _, hf := range args.headers {
		if strings.EqualFold(hf.Name, "x-envoy-ratelimited") {
			t.Errorf("x-envoy-ratelimited header present despite disableXEnvoyRateLimitedHeader=true; headers=%+v", args.headers)
			break
		}
	}
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_Error_FailOpen
// ----------------------------------------------------------------------------

// TestDispositions_Error_FailOpen verifies the error + failure_mode_deny=false
// (DEFAULT) arm per parent SPEC §4.6: the error counter increments AND the
// failure_mode_allowed counter increments AND ContinueDecoding is called
// (request admitted; fail-open).
func TestDispositions_Error_FailOpen(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain:          "test",
		stats:           fs,
		failureModeDeny: false, // explicit; the default
	}
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	f.applyDisposition(headers, nil /* resp */, context.DeadlineExceeded)

	if got := fs.error.Load(); got != 1 {
		t.Errorf("error counter: got %d, want 1", got)
	}
	if got := fs.failureModeAllowed.Load(); got != 1 {
		t.Errorf("failure_mode_allowed counter: got %d, want 1", got)
	}
	if got := dcb.snapshotContinueCount(); got != 1 {
		t.Errorf("ContinueDecoding: got %d, want 1 (fail-open admits)", got)
	}
	count, _ := dcb.snapshotLocalReply()
	if count != 0 {
		t.Errorf("SendLocalReply: got %d, want 0 (fail-open does NOT reject)", count)
	}
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_Error_FailClosed
// ----------------------------------------------------------------------------

// TestDispositions_Error_FailClosed verifies the error + failure_mode_deny=true
// arm per parent SPEC §4.6: the error counter increments (failure_mode_allowed
// does NOT) AND SendLocalReply(statusOnError, "", nil) fires (request rejected).
func TestDispositions_Error_FailClosed(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain:          "test",
		stats:           fs,
		failureModeDeny: true,
		statusOnError:   500,
	}
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	f.applyDisposition(headers, nil, context.DeadlineExceeded)

	if got := fs.error.Load(); got != 1 {
		t.Errorf("error counter: got %d, want 1", got)
	}
	if got := fs.failureModeAllowed.Load(); got != 0 {
		t.Errorf("failure_mode_allowed counter: got %d, want 0 (fail-closed does NOT increment)", got)
	}
	// Fail-closed must call ContinueDecoding AFTER SendLocalReply to wake the
	// parked dispatch goroutine (chain.go:316-325 parkDecode); the chain's
	// localReplyDone gate ensures the resumed iteration short-circuits. Mirrors
	// ext_authz extauthz.go:1146-1156 + fault fault.go:299-324 precedents.
	if got := dcb.snapshotContinueCount(); got != 1 {
		t.Errorf("ContinueDecoding: got %d, want 1 (wakes parked dispatch goroutine after SendLocalReply)", got)
	}
	count, args := dcb.snapshotLocalReply()
	if count != 1 {
		t.Fatalf("SendLocalReply: got %d, want 1", count)
	}
	if args.status != 500 {
		t.Errorf("SendLocalReply status: got %d, want 500", args.status)
	}
	if args.body != "" {
		t.Errorf("SendLocalReply body: got %q, want %q (nullptr-mutate)", args.body, "")
	}
	if args.headers != nil {
		t.Errorf("SendLocalReply headers: got %+v, want nil (nullptr-mutate)", args.headers)
	}
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_GRPC_8_vs_14
// ----------------------------------------------------------------------------

// TestDispositions_GRPC_8_vs_14 pins the gRPC-code mapping per parent SPEC
// §4.7: rate_limited_as_resource_exhausted=true ⇒ RESOURCE_EXHAUSTED (8);
// false ⇒ UNAVAILABLE (14). This metadata is NOT surfacable through the 3-arg
// SendLocalReply API at 24.1 — the constant + helper are pinned for forward
// 24.2 consumption (when a gRPC trailer envelope path lands).
func TestDispositions_GRPC_8_vs_14(t *testing.T) {
	if got := rateLimitedAsResourceExhaustedGrpcCode(true); got != grpcCodeResourceExhausted {
		t.Errorf("rateLimitedAsResourceExhaustedGrpcCode(true): got %d, want %d (RESOURCE_EXHAUSTED)", got, grpcCodeResourceExhausted)
	}
	if got := rateLimitedAsResourceExhaustedGrpcCode(false); got != grpcCodeUnavailable {
		t.Errorf("rateLimitedAsResourceExhaustedGrpcCode(false): got %d, want %d (UNAVAILABLE)", got, grpcCodeUnavailable)
	}
	if grpcCodeResourceExhausted != 8 {
		t.Errorf("grpcCodeResourceExhausted: got %d, want 8", grpcCodeResourceExhausted)
	}
	if grpcCodeUnavailable != 14 {
		t.Errorf("grpcCodeUnavailable: got %d, want 14", grpcCodeUnavailable)
	}
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_OverLimit_WakesDispatchGoroutine
// ----------------------------------------------------------------------------

// TestDispositions_OverLimit_WakesDispatchGoroutine pins the critical
// SendLocalReply-then-ContinueDecoding wake-up invariant on the OVER_LIMIT
// path. The dispatch goroutine is parked in parkDecode (chain.go:316-325)
// waiting on decodeResumeCh; SendLocalReply sets c.localReplyDone but does
// NOT push to decodeResumeCh. Without a follow-up ContinueDecoding the parked
// goroutine never wakes until stream-ctx cancellation, causing hangs +
// goroutine leaks under any non-zero RLS over-limit rate. Mirrors the
// ext_authz deny-path precedent at extauthz.go:1097-1111 and the phase-09
// fault filter precedent at fault.go:299-324.
func TestDispositions_OverLimit_WakesDispatchGoroutine(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain:            "test",
		stats:             fs,
		rateLimitedStatus: 429,
	}
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	resp := &ratelimitservicev3.RateLimitResponse{
		OverallCode: ratelimitservicev3.RateLimitResponse_OVER_LIMIT,
	}
	f.applyDisposition(headers, resp, nil)

	count, _ := dcb.snapshotLocalReply()
	if count != 1 {
		t.Fatalf("SendLocalReply: got %d, want 1 (OVER_LIMIT path emits local reply)", count)
	}
	if got := dcb.snapshotContinueCount(); got != 1 {
		t.Errorf("ContinueDecoding: got %d, want 1 (wakes parked dispatch goroutine — without this the HCM dispatch goroutine hangs in parkDecode until stream-ctx cancellation)", got)
	}
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_Error_FailClosed_WakesDispatchGoroutine
// ----------------------------------------------------------------------------

// TestDispositions_Error_FailClosed_WakesDispatchGoroutine pins the critical
// SendLocalReply-then-ContinueDecoding wake-up invariant on the fail-closed
// error path. Same parkDecode-vs-localReplyDone race as the OVER_LIMIT path
// (see TestDispositions_OverLimit_WakesDispatchGoroutine for the full
// rationale). Mirrors the ext_authz failureModeDeny precedent at
// extauthz.go:1146-1156.
func TestDispositions_Error_FailClosed_WakesDispatchGoroutine(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain:          "test",
		stats:           fs,
		failureModeDeny: true,
		statusOnError:   500,
	}
	f := &filter{cc: cc, dcb: dcb}

	headers := http.Header{}
	f.applyDisposition(headers, nil /* resp */, context.DeadlineExceeded)

	count, _ := dcb.snapshotLocalReply()
	if count != 1 {
		t.Fatalf("SendLocalReply: got %d, want 1 (fail-closed path emits local reply)", count)
	}
	if got := dcb.snapshotContinueCount(); got != 1 {
		t.Errorf("ContinueDecoding: got %d, want 1 (wakes parked dispatch goroutine — without this the HCM dispatch goroutine hangs in parkDecode until stream-ctx cancellation)", got)
	}
}

// ----------------------------------------------------------------------------
// Test: TestRcDetailsConstants_ByteStable
// ----------------------------------------------------------------------------

// TestRcDetailsConstants_ByteStable pins the rc-details string constants per
// parent SPEC §4.7. These are PARSED here for byte-stability even though the
// 3-arg SendLocalReply API at 24.1 cannot surface them on the wire (ABSENT-BY-API
// per the admission_control precedent). 24.2 may extend the API.
func TestRcDetailsConstants_ByteStable(t *testing.T) {
	if rcDetailsRequestRateLimited != "request_rate_limited" {
		t.Errorf("rcDetailsRequestRateLimited: got %q, want %q", rcDetailsRequestRateLimited, "request_rate_limited")
	}
	if rcDetailsRateLimiterError != "rate_limiter_error" {
		t.Errorf("rcDetailsRateLimiterError: got %q, want %q", rcDetailsRateLimiterError, "rate_limiter_error")
	}
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_XRateLimit_Stored_OnAllDispositions
// ----------------------------------------------------------------------------

// TestDispositions_XRateLimit_Stored_OnAllDispositions pins the disposition-
// aware store of `f.responseStatuses` per phase-24.2 Task 5 (ADR-0197
// X-RateLimit slice + D-RL12):
//
//   - OK + statuses ⇒ stored
//   - OVER_LIMIT + statuses ⇒ stored
//   - error + fail-OPEN (failOpen=true, resp present with statuses) ⇒ stored
//   - error + fail-CLOSED ⇒ NEVER stored (the nullptr-mutate path; X-RateLimit
//     headers must NOT be emitted per D-RL12)
//
// The byte-shape of the headers themselves is pinned by headers_test.go +
// encode_test.go; this test pins the *cross-arm store discipline* on the
// dispositions side.
func TestDispositions_XRateLimit_Stored_OnAllDispositions(t *testing.T) {
	// Build a deterministic shared statuses slice for the three "store" arms.
	statuses := []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
		{
			CurrentLimit: &ratelimitservicev3.RateLimitResponse_RateLimit{
				RequestsPerUnit: 10,
				Unit:            ratelimitservicev3.RateLimitResponse_RateLimit_SECOND,
				Name:            "per-ip",
			},
			LimitRemaining:     2,
			DurationUntilReset: &durationpb.Duration{Seconds: 1},
		},
	}

	t.Run("OK_stores_statuses", func(t *testing.T) {
		dcb := newFakeRatelimitDCB()
		fs := makeCountedStats(t)
		cc := &compiledConfig{
			domain:                  "test",
			stats:                   fs,
			enableXRateLimitHeaders: true,
		}
		f := &filter{cc: cc, dcb: dcb}

		resp := &ratelimitservicev3.RateLimitResponse{
			OverallCode: ratelimitservicev3.RateLimitResponse_OK,
			Statuses:    statuses,
		}
		f.applyDisposition(http.Header{}, resp, nil)

		if got := len(f.responseStatuses); got != 1 {
			t.Errorf("f.responseStatuses: got len=%d, want 1 (statuses stored on OK)", got)
		}
	})

	t.Run("OVER_LIMIT_stores_statuses", func(t *testing.T) {
		dcb := newFakeRatelimitDCB()
		fs := makeCountedStats(t)
		cc := &compiledConfig{
			domain:                  "test",
			stats:                   fs,
			rateLimitedStatus:       429,
			enableXRateLimitHeaders: true,
		}
		f := &filter{cc: cc, dcb: dcb}

		resp := &ratelimitservicev3.RateLimitResponse{
			OverallCode: ratelimitservicev3.RateLimitResponse_OVER_LIMIT,
			Statuses:    statuses,
		}
		f.applyDisposition(http.Header{}, resp, nil)

		if got := len(f.responseStatuses); got != 1 {
			t.Errorf("f.responseStatuses: got len=%d, want 1 (statuses stored on OVER_LIMIT)", got)
		}
	})

	t.Run("fail_OPEN_stores_statuses_when_resp_present", func(t *testing.T) {
		dcb := newFakeRatelimitDCB()
		fs := makeCountedStats(t)
		cc := &compiledConfig{
			domain:                  "test",
			stats:                   fs,
			failureModeDeny:         false,
			enableXRateLimitHeaders: true,
		}
		f := &filter{cc: cc, dcb: dcb}

		// Partial-success: RLS returned a malformed envelope (UNKNOWN
		// OverallCode) but DID populate statuses. applyDisposition routes
		// through the error arm; on failOpen the statuses are stored so the
		// upstream's response carries X-RateLimit when the fail-open admit
		// reaches the encode phase.
		resp := &ratelimitservicev3.RateLimitResponse{
			OverallCode: ratelimitservicev3.RateLimitResponse_UNKNOWN,
			Statuses:    statuses,
		}
		f.applyDisposition(http.Header{}, resp, nil)

		if got := len(f.responseStatuses); got != 1 {
			t.Errorf("f.responseStatuses: got len=%d, want 1 (statuses stored on fail-OPEN)", got)
		}
	})

	t.Run("fail_CLOSED_does_NOT_store_statuses_D_RL12", func(t *testing.T) {
		dcb := newFakeRatelimitDCB()
		fs := makeCountedStats(t)
		cc := &compiledConfig{
			domain:                  "test",
			stats:                   fs,
			failureModeDeny:         true,
			statusOnError:           500,
			enableXRateLimitHeaders: true,
		}
		f := &filter{cc: cc, dcb: dcb}

		// Even a non-nil response with statuses on the fail-CLOSED path MUST
		// NOT store. The nullptr-mutate discipline per D-RL12: 500 response
		// carries NO X-RateLimit headers.
		resp := &ratelimitservicev3.RateLimitResponse{
			OverallCode: ratelimitservicev3.RateLimitResponse_UNKNOWN,
			Statuses:    statuses,
		}
		f.applyDisposition(http.Header{}, resp, nil)

		if got := len(f.responseStatuses); got != 0 {
			t.Errorf("f.responseStatuses: got len=%d, want 0 (D-RL12 — fail-CLOSED MUST NOT store statuses; would leak X-RateLimit onto the 500 reply)", got)
		}
	})

	t.Run("transport_error_fail_OPEN_nil_resp_no_store", func(t *testing.T) {
		// nil response (transport error) ⇒ no statuses to store on either
		// fail-OPEN OR fail-CLOSED. The non-nil-resp arm above is the only
		// store-on-error path.
		dcb := newFakeRatelimitDCB()
		fs := makeCountedStats(t)
		cc := &compiledConfig{
			domain:                  "test",
			stats:                   fs,
			failureModeDeny:         false,
			enableXRateLimitHeaders: true,
		}
		f := &filter{cc: cc, dcb: dcb}
		f.applyDisposition(http.Header{}, nil, context.DeadlineExceeded)

		if got := len(f.responseStatuses); got != 0 {
			t.Errorf("f.responseStatuses: got len=%d, want 0 (nil resp ⇒ no statuses to store)", got)
		}
	})
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_OverLimit_AMEND8_XRateLimitBetweenXEnvoyAndConfig
// ----------------------------------------------------------------------------

// TestDispositions_OverLimit_AMEND8_XRateLimitBetweenXEnvoyAndConfig pins the
// parent SPEC §4.7 line 214 canonical AMEND-8 wire-order discipline on the
// OVER_LIMIT 429 path: the X-RateLimit DRAFT_VERSION_03 triple
// (x-ratelimit-limit / x-ratelimit-remaining / x-ratelimit-reset) MUST land
// BETWEEN the `x-envoy-ratelimited` slot and the filter-config
// `response_headers_to_add` slot — NOT AFTER the filter-config slot.
//
// # Wire-order regression context (Task 5 follow-up)
//
// Task 5 introduced the X-RateLimit emission via the encode-side hook
// (encode.go::encodeHeaders → headers.Set). On the OVER_LIMIT path that hook
// runs INSIDE chain.go::beginLocalReply → RunEncodeHeaders AFTER the
// OrderedHeaders carrier built at applyOverLimit was merged into a transient
// http.Header. Set() on a NEW canonical key appends at the TAIL via
// ReconcileOrderedHeaders, landing X-RateLimit AFTER filter-config
// response_headers_to_add. The spec-compliance + code-quality reviewers
// flagged this as I-1 wire-order regression vs parent SPEC §4.7 line 214.
//
// # Fix: inline X-RateLimit at applyOverLimit (Option (a))
//
// Construct the X-RateLimit triple inline at applyOverLimit (slot [c-pre])
// BEFORE the filter-config response_headers_to_add loop (slot [c]). The
// encode-side hook (encode.go::encodeHeaders) still runs but its
// `headers.Set` becomes a no-op-set-equal idempotent overwrite (the same
// `buildXRateLimitHeaders` source produces byte-identical values).
//
// # 24.1-baked x-envoy-ratelimited-vs-RLS inversion: OUT OF SCOPE
//
// The existing 24.1 byte-shape emits `x-envoy-ratelimited` BEFORE
// `ResponseHeadersToAdd` (RLS-side). Parent SPEC §4.7 line 214 specifies
// the canonical order is `RLS → x-envoy-ratelimited → X-RateLimit → config`.
// The 24.1-baked `x-envoy-ratelimited`-before-RLS inversion is an inherited
// 24.1 behavior and is OUT OF SCOPE for this Task 5 follow-up; this test
// preserves the inversion verbatim. Only the Task-5-introduced
// X-RateLimit-vs-filter-config slot is being fixed.
func TestDispositions_OverLimit_AMEND8_XRateLimitBetweenXEnvoyAndConfig(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain:                  "test",
		stats:                   fs,
		rateLimitedStatus:       429,
		enableXRateLimitHeaders: true,
		responseHeadersToAdd: []headerKV{
			{name: "X-From-Config", value: "config-1"},
		},
	}
	f := &filter{cc: cc, dcb: dcb}

	resp := &ratelimitservicev3.RateLimitResponse{
		OverallCode: ratelimitservicev3.RateLimitResponse_OVER_LIMIT,
		ResponseHeadersToAdd: []*corev3.HeaderValue{
			{Key: "X-From-Rls", Value: "rls-1"},
		},
		Statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
			{
				CurrentLimit: &ratelimitservicev3.RateLimitResponse_RateLimit{
					RequestsPerUnit: 10,
					Unit:            ratelimitservicev3.RateLimitResponse_RateLimit_SECOND,
				},
				LimitRemaining:     3,
				DurationUntilReset: &durationpb.Duration{Seconds: 7},
			},
		},
	}

	f.applyDisposition(http.Header{}, resp, nil)

	_, args := dcb.snapshotLocalReply()

	// Post-fix wire-order: x-envoy-ratelimited / RLS / X-RateLimit triple /
	// filter-config. The X-RateLimit triple lands BEFORE the filter-config
	// X-From-Config slot (the Task 5 follow-up fix); the 24.1-baked
	// x-envoy-ratelimited-before-RLS inversion is preserved verbatim
	// (out-of-scope per the docstring above).
	want := []envoyhttp.HeaderField{
		{Name: "x-envoy-ratelimited", Value: "true"},
		{Name: "X-From-Rls", Value: "rls-1"},
		{Name: "x-ratelimit-limit", Value: "10, 10;w=1"},
		{Name: "x-ratelimit-remaining", Value: "3"},
		{Name: "x-ratelimit-reset", Value: "7"},
		{Name: "X-From-Config", Value: "config-1"},
	}
	if len(args.headers) != len(want) {
		t.Fatalf("SendLocalReply headers: got %d entries, want %d (entries=%+v)", len(args.headers), len(want), args.headers)
	}
	for i, hf := range args.headers {
		if hf.Name != want[i].Name || hf.Value != want[i].Value {
			t.Errorf("header[%d]: got {%q, %q}, want {%q, %q}", i, hf.Name, hf.Value, want[i].Name, want[i].Value)
		}
	}
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_OverLimit_XRateLimit_OFF_NoInlineEmission
// ----------------------------------------------------------------------------

// TestDispositions_OverLimit_XRateLimit_OFF_NoInlineEmission verifies that
// when enableXRateLimitHeaders is OFF (the proto-zero RateLimit_OFF default),
// the OVER_LIMIT path emits NO X-RateLimit triple — even when the RLS
// response carries non-empty statuses. The Task 5 follow-up inline emission
// at applyOverLimit MUST be gated on cc.enableXRateLimitHeaders.
func TestDispositions_OverLimit_XRateLimit_OFF_NoInlineEmission(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain:                  "test",
		stats:                   fs,
		rateLimitedStatus:       429,
		enableXRateLimitHeaders: false, // OFF
	}
	f := &filter{cc: cc, dcb: dcb}

	resp := &ratelimitservicev3.RateLimitResponse{
		OverallCode: ratelimitservicev3.RateLimitResponse_OVER_LIMIT,
		Statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
			{
				CurrentLimit: &ratelimitservicev3.RateLimitResponse_RateLimit{
					RequestsPerUnit: 10,
					Unit:            ratelimitservicev3.RateLimitResponse_RateLimit_SECOND,
				},
				LimitRemaining:     3,
				DurationUntilReset: &durationpb.Duration{Seconds: 7},
			},
		},
	}

	f.applyDisposition(http.Header{}, resp, nil)

	_, args := dcb.snapshotLocalReply()
	for _, hf := range args.headers {
		switch strings.ToLower(hf.Name) {
		case "x-ratelimit-limit", "x-ratelimit-remaining", "x-ratelimit-reset":
			t.Errorf("X-RateLimit header %q emitted at applyOverLimit despite enableXRateLimitHeaders=false; headers=%+v", hf.Name, args.headers)
		}
	}
}

// ----------------------------------------------------------------------------
// Test: TestDispositions_OverLimit_XRateLimit_NoCurrentLimit_NoInlineEmission
// ----------------------------------------------------------------------------

// TestDispositions_OverLimit_XRateLimit_NoCurrentLimit_NoInlineEmission
// verifies upstream parity for the edge case where statuses exist but NO
// status carries `current_limit` — `buildXRateLimitHeaders` returns ok=false
// and the inline emission MUST suppress (no zero-value "0, 0, 0" leak).
func TestDispositions_OverLimit_XRateLimit_NoCurrentLimit_NoInlineEmission(t *testing.T) {
	dcb := newFakeRatelimitDCB()
	fs := makeCountedStats(t)
	cc := &compiledConfig{
		domain:                  "test",
		stats:                   fs,
		rateLimitedStatus:       429,
		enableXRateLimitHeaders: true,
		responseHeadersToAdd: []headerKV{
			{name: "X-From-Config", value: "config-1"},
		},
	}
	f := &filter{cc: cc, dcb: dcb}

	resp := &ratelimitservicev3.RateLimitResponse{
		OverallCode: ratelimitservicev3.RateLimitResponse_OVER_LIMIT,
		Statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
			// no current_limit → MIN unset → buildXRateLimitHeaders ok=false
			{LimitRemaining: 5},
		},
	}

	f.applyDisposition(http.Header{}, resp, nil)

	_, args := dcb.snapshotLocalReply()
	for _, hf := range args.headers {
		switch strings.ToLower(hf.Name) {
		case "x-ratelimit-limit", "x-ratelimit-remaining", "x-ratelimit-reset":
			t.Errorf("X-RateLimit header %q emitted at applyOverLimit despite no status carrying current_limit; headers=%+v", hf.Name, args.headers)
		}
	}
	// The filter-config X-From-Config slot still emits (the inline X-RateLimit
	// suppression should not affect the post-X-RateLimit filter-config slot).
	foundConfig := false
	for _, hf := range args.headers {
		if hf.Name == "X-From-Config" && hf.Value == "config-1" {
			foundConfig = true
			break
		}
	}
	if !foundConfig {
		t.Errorf("X-From-Config header missing; the inline X-RateLimit suppression should NOT affect the filter-config slot; headers=%+v", args.headers)
	}
}
