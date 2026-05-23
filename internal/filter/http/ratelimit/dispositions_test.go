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
