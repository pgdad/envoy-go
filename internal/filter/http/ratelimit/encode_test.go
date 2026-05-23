package ratelimit

// encode_test.go — TDD coverage for the per-stream X-RateLimit
// DRAFT_VERSION_03 encode-side injection per phase-24.2 PLAN Task 5 (parent
// SPEC §6.6 + AMEND-8 + ADR-0197 X-RateLimit slice). The byte-shape pin
// itself lives in headers_test.go; this file pins the *disposition-aware
// emission discipline* per D-RL12:
//
//   - OK + DRAFT_VERSION_03 ⇒ headers emitted
//   - OVER_LIMIT 429 + DRAFT_VERSION_03 ⇒ headers emitted in AMEND-8 order
//   - fail-OPEN + DRAFT_VERSION_03 ⇒ headers emitted
//   - fail-CLOSED 500 + DRAFT_VERSION_03 ⇒ NO headers (nullptr-mutate path)
//   - OFF (the proto-zero default) ⇒ NO headers on any disposition
//
// The dispositions.go::applyDisposition store-statuses-on-three-arms behavior
// (OK / OVER_LIMIT / fail-OPEN) is co-pinned at dispositions_test.go via the
// TestDispositions_XRateLimit_Stored_OnAllDispositions row.

import (
	"net/http"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	ratelimitservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/protobuf/types/known/durationpb"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// ----------------------------------------------------------------------------
// Test helpers
// ----------------------------------------------------------------------------

// makeOKResponse returns a deterministic *RateLimitResponse with the given
// per-descriptor statuses pre-attached. Two statuses by default — second is
// the MIN (limit_remaining=2).
func makeOKResponseWithStatuses() *ratelimitservicev3.RateLimitResponse {
	return &ratelimitservicev3.RateLimitResponse{
		OverallCode: ratelimitservicev3.RateLimitResponse_OK,
		Statuses: []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
			{
				CurrentLimit: &ratelimitservicev3.RateLimitResponse_RateLimit{
					RequestsPerUnit: 100,
					Unit:            ratelimitservicev3.RateLimitResponse_RateLimit_MINUTE,
					Name:            "global",
				},
				LimitRemaining:     50,
				DurationUntilReset: &durationpb.Duration{Seconds: 30},
			},
			{
				CurrentLimit: &ratelimitservicev3.RateLimitResponse_RateLimit{
					RequestsPerUnit: 10,
					Unit:            ratelimitservicev3.RateLimitResponse_RateLimit_SECOND,
					Name:            "per-ip",
				},
				LimitRemaining:     2,
				DurationUntilReset: &durationpb.Duration{Seconds: 1},
			},
		},
	}
}

// makeOverLimitResponseWithStatuses returns an OVER_LIMIT response with
// pre-attached per-descriptor statuses (the same two-status shape as the OK
// helper but with the OverallCode flipped).
func makeOverLimitResponseWithStatuses() *ratelimitservicev3.RateLimitResponse {
	r := makeOKResponseWithStatuses()
	r.OverallCode = ratelimitservicev3.RateLimitResponse_OVER_LIMIT
	r.RawBody = []byte("over-limit-body")
	return r
}

// expectXRateLimitTriple is a helper assertion: verifies the three
// X-RateLimit headers are present on h with the given byte-exact values.
// Uses http.Header.Get (canonical-case insensitive) since http.Header
// canonicalizes via textproto.CanonicalMIMEHeaderKey on Set.
func expectXRateLimitTriple(t *testing.T, h http.Header, wantLimit, wantRemaining, wantReset string) {
	t.Helper()
	if got := h.Get(headerXRateLimitLimit); got != wantLimit {
		t.Errorf("x-ratelimit-limit:\n  got  %q\n  want %q", got, wantLimit)
	}
	if got := h.Get(headerXRateLimitRemaining); got != wantRemaining {
		t.Errorf("x-ratelimit-remaining:\n  got  %q\n  want %q", got, wantRemaining)
	}
	if got := h.Get(headerXRateLimitReset); got != wantReset {
		t.Errorf("x-ratelimit-reset:\n  got  %q\n  want %q", got, wantReset)
	}
}

// expectNoXRateLimitTriple asserts NONE of the three X-RateLimit headers
// is present on h. Used for the OFF gate + the fail-CLOSED discipline.
func expectNoXRateLimitTriple(t *testing.T, h http.Header) {
	t.Helper()
	for _, k := range []string{headerXRateLimitLimit, headerXRateLimitRemaining, headerXRateLimitReset} {
		if v := h.Get(k); v != "" {
			t.Errorf("header %q: got %q, want empty (X-RateLimit suppressed)", k, v)
		}
	}
}

// ----------------------------------------------------------------------------
// Test: TestEncodeHeaders_OK_AppliesXRateLimit
// ----------------------------------------------------------------------------

// TestEncodeHeaders_OK_AppliesXRateLimit verifies that on the OK disposition
// with DRAFT_VERSION_03 enabled, EncodeHeaders mutates the response header
// map to carry the three X-RateLimit headers per upstream byte-shape.
//
// Setup: pre-store responseStatuses on the *filter (mirrors what
// dispositions.go::applyOK does live). Then invoke EncodeHeaders directly.
func TestEncodeHeaders_OK_AppliesXRateLimit(t *testing.T) {
	resp := makeOKResponseWithStatuses()
	cc := &compiledConfig{
		domain:                  "test",
		enableXRateLimitHeaders: true,
	}
	f := &filter{cc: cc, responseStatuses: resp.GetStatuses()}

	headers := http.Header{}
	if got := f.EncodeHeaders(headers, true); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", got)
	}

	// MIN status: rpu=10, limit_remaining=2, reset=1 (the second descriptor).
	// Quota-policy iterates BOTH descriptors:
	//   first  → ", 100;w=60;name=\"global\""
	//   second → ", 10;w=1;name=\"per-ip\""
	expectXRateLimitTriple(t, headers,
		`10, 100;w=60;name="global", 10;w=1;name="per-ip"`,
		"2",
		"1",
	)
}

// ----------------------------------------------------------------------------
// Test: TestEncodeHeaders_OverLimit_AppliesXRateLimit
// ----------------------------------------------------------------------------

// TestEncodeHeaders_OverLimit_AppliesXRateLimit verifies the OVER_LIMIT path:
// applyOverLimit triggers SendLocalReply, which causes the chain to enter
// beginLocalReply (chain.go:1214) and synchronously call this filter's
// EncodeHeaders with the local-reply header map (the AMEND-8-ordered headers
// the filter passed to SendLocalReply). The X-RateLimit triple is mutated
// onto that map.
//
// This test exercises the EncodeHeaders body directly (the integration with
// SendLocalReply is covered by the dispositions tests + the fixture-level
// differential coverage at Task 6). The header-map shape mirrors the live
// path: the OVER_LIMIT headers (x-envoy-ratelimited: true + RLS/config-side
// adds) are pre-populated before this filter's EncodeHeaders runs in
// beginLocalReply iteration.
func TestEncodeHeaders_OverLimit_AppliesXRateLimit(t *testing.T) {
	resp := makeOverLimitResponseWithStatuses()
	resp.ResponseHeadersToAdd = []*corev3.HeaderValue{
		{Key: "X-From-Rls", Value: "rls-1"},
	}
	cc := &compiledConfig{
		domain:                  "test",
		enableXRateLimitHeaders: true,
		rateLimitedStatus:       429,
	}
	f := &filter{cc: cc, responseStatuses: resp.GetStatuses()}

	// Pre-populate the header map with the OVER_LIMIT AMEND-8-ordered carrier
	// (mirrors beginLocalReply's `merged := make(http.Header, ...)` + Add()
	// loop at chain.go:1235-1242).
	headers := http.Header{}
	headers.Add("x-envoy-ratelimited", "true")
	headers.Add("X-From-Rls", "rls-1")

	if got := f.EncodeHeaders(headers, true); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", got)
	}

	expectXRateLimitTriple(t, headers,
		`10, 100;w=60;name="global", 10;w=1;name="per-ip"`,
		"2",
		"1",
	)
	// Pre-populated headers preserved.
	if got := headers.Get("x-envoy-ratelimited"); got != "true" {
		t.Errorf("x-envoy-ratelimited: got %q, want %q (pre-populated header preserved)", got, "true")
	}
	if got := headers.Get("X-From-Rls"); got != "rls-1" {
		t.Errorf("X-From-Rls: got %q, want %q", got, "rls-1")
	}
}

// ----------------------------------------------------------------------------
// Test: TestEncodeHeaders_FailOpen_AppliesXRateLimit
// ----------------------------------------------------------------------------

// TestEncodeHeaders_FailOpen_AppliesXRateLimit verifies the fail-OPEN
// disposition: dispositions.go::applyError stores statuses (when present —
// the error response may carry statuses from a partial-success RLS) and
// EncodeHeaders applies them. Reference: a fail-OPEN error path admits the
// request; the upstream returns a real response which flows through the
// encode chain. The dispositions store-on-failOpen mirrors upstream which
// emits X-RateLimit if the response carries statuses (the OK+OVER_LIMIT
// paths set them; on transport-error the response is nil and no statuses
// are stored — but in case of partial-success RLS with a non-nil resp the
// statuses ARE applied).
func TestEncodeHeaders_FailOpen_AppliesXRateLimit(t *testing.T) {
	resp := makeOKResponseWithStatuses() // pretend partial-success statuses
	cc := &compiledConfig{
		domain:                  "test",
		enableXRateLimitHeaders: true,
		failureModeDeny:         false,
	}
	f := &filter{cc: cc, responseStatuses: resp.GetStatuses()}

	headers := http.Header{}
	f.EncodeHeaders(headers, true)

	expectXRateLimitTriple(t, headers,
		`10, 100;w=60;name="global", 10;w=1;name="per-ip"`,
		"2",
		"1",
	)
}

// ----------------------------------------------------------------------------
// Test: TestEncodeHeaders_FailClosed_NoXRateLimit
// ----------------------------------------------------------------------------

// TestEncodeHeaders_FailClosed_NoXRateLimit verifies D-RL12: the fail-CLOSED
// 500 path does NOT emit X-RateLimit. The discipline is enforced at
// dispositions.go::applyError(failOpen=false) by NOT storing responseStatuses
// — this test exercises the encode-side resilience: even with
// enableXRateLimitHeaders=true, when responseStatuses is nil EncodeHeaders is
// a clean no-op.
func TestEncodeHeaders_FailClosed_NoXRateLimit(t *testing.T) {
	cc := &compiledConfig{
		domain:                  "test",
		enableXRateLimitHeaders: true,
		failureModeDeny:         true,
		statusOnError:           500,
	}
	// responseStatuses is nil — mirrors dispositions.go::applyError on fail-CLOSED.
	f := &filter{cc: cc, responseStatuses: nil}

	headers := http.Header{}
	if got := f.EncodeHeaders(headers, true); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", got)
	}

	expectNoXRateLimitTriple(t, headers)
}

// ----------------------------------------------------------------------------
// Test: TestEncodeHeaders_OFF_NoEmission
// ----------------------------------------------------------------------------

// TestEncodeHeaders_OFF_NoEmission verifies that with enable_x_ratelimit_headers
// at the proto-zero default (RateLimit_OFF), no X-RateLimit headers are emitted
// on ANY disposition — even if responseStatuses is non-empty (operator forgot
// to enable the gate). The gate is the first-line filter in EncodeHeaders.
func TestEncodeHeaders_OFF_NoEmission(t *testing.T) {
	resp := makeOKResponseWithStatuses()
	cc := &compiledConfig{
		domain:                  "test",
		enableXRateLimitHeaders: false, // OFF
	}
	f := &filter{cc: cc, responseStatuses: resp.GetStatuses()}

	headers := http.Header{}
	if got := f.EncodeHeaders(headers, true); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", got)
	}

	expectNoXRateLimitTriple(t, headers)
}

// ----------------------------------------------------------------------------
// Test: TestEncodeHeaders_NilStatuses_NoEmission
// ----------------------------------------------------------------------------

// TestEncodeHeaders_NilStatuses_NoEmission covers the zero-descriptor short-
// circuit (DecodeHeaders returns Continue without firing the RLS call;
// applyDisposition never runs) AND the nil-resp / no-statuses transport-error
// arm of fail-OPEN. With responseStatuses nil/empty AND
// enableXRateLimitHeaders=true, EncodeHeaders no-ops cleanly.
func TestEncodeHeaders_NilStatuses_NoEmission(t *testing.T) {
	cc := &compiledConfig{
		domain:                  "test",
		enableXRateLimitHeaders: true,
	}
	f := &filter{cc: cc, responseStatuses: nil}

	headers := http.Header{}
	f.EncodeHeaders(headers, true)
	expectNoXRateLimitTriple(t, headers)

	// Empty (non-nil) slice ⇒ same no-op semantic.
	f.responseStatuses = []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{}
	f.EncodeHeaders(headers, true)
	expectNoXRateLimitTriple(t, headers)
}

// ----------------------------------------------------------------------------
// Test: TestEncodeHeaders_NoCurrentLimit_NoEmission
// ----------------------------------------------------------------------------

// TestEncodeHeaders_NoCurrentLimit_NoEmission verifies upstream parity for the
// edge case where statuses are non-empty but NO status carries current_limit.
// Upstream returns an empty response-header map (no X-RateLimit added).
// envoy-go's buildXRateLimitHeaders returns ok=false in that case; the
// encode-side hook must NOT inject the headers (would produce wire-meaningful
// "0, 0, 0" values that diverge from upstream).
func TestEncodeHeaders_NoCurrentLimit_NoEmission(t *testing.T) {
	cc := &compiledConfig{
		domain:                  "test",
		enableXRateLimitHeaders: true,
	}
	// Two statuses, neither carrying current_limit (nil sub-message).
	statuses := []*ratelimitservicev3.RateLimitResponse_DescriptorStatus{
		{LimitRemaining: 0},
		{LimitRemaining: 5},
	}
	f := &filter{cc: cc, responseStatuses: statuses}

	headers := http.Header{}
	f.EncodeHeaders(headers, true)
	expectNoXRateLimitTriple(t, headers)
}

// ----------------------------------------------------------------------------
// Test: TestEncodeHeaders_NilCompiledConfig_NoEmission
// ----------------------------------------------------------------------------

// TestEncodeHeaders_NilCompiledConfig_NoEmission is a defensive nil-guard:
// when *filter.cc is nil (test-shape only — production wires cc at New),
// EncodeHeaders no-ops cleanly without panic per ADR-0018 never-panic.
func TestEncodeHeaders_NilCompiledConfig_NoEmission(t *testing.T) {
	f := &filter{cc: nil}
	headers := http.Header{}
	if got := f.EncodeHeaders(headers, true); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", got)
	}
	expectNoXRateLimitTriple(t, headers)
}

// ----------------------------------------------------------------------------
// Test: TestEncodeHeaders_CanonicalCase_Insensitive
// ----------------------------------------------------------------------------

// TestEncodeHeaders_OverLimit_IdempotentSetEqual_FilterConfigPrePopulated
// pins the no-op-set-equal idempotence discipline introduced at the Task 5
// follow-up I-1 fix (AMEND-8 wire-order — parent SPEC §4.7 line 214).
//
// # Discipline
//
// On the OVER_LIMIT path applyOverLimit emits the X-RateLimit triple inline
// (between `x-envoy-ratelimited` and filter-config response_headers_to_add)
// per the new wire-order. The encode-side `encode.go::encodeHeaders` hook
// then runs synchronously via chain.go::beginLocalReply → RunEncodeHeaders;
// chain.go::beginLocalReply pre-populates the carrier `merged http.Header`
// from the inline-emitted OrderedHeaders (carrier already has X-RateLimit
// AND filter-config response_headers_to_add). The encode hook reads
// f.responseStatuses (set at applyDisposition) → recomputes via
// buildXRateLimitHeaders (the SAME source function) → calls headers.Set
// with byte-identical values. This is the no-op-set-equal idempotence: the
// canonical-form Set on an existing canonical key overwrites with byte-
// equal value, so the wire bytes are unchanged.
//
// # Test shape
//
// Mirror the live OVER_LIMIT carrier shape after beginLocalReply's
// `merged.Add(...)` loop: pre-populate the http.Header with the inline
// AMEND-8 wire-order (x-envoy-ratelimited / RLS / X-RateLimit triple /
// filter-config); invoke EncodeHeaders; verify X-RateLimit values are
// preserved byte-identical AND the filter-config slot is preserved.
func TestEncodeHeaders_OverLimit_IdempotentSetEqual_FilterConfigPrePopulated(t *testing.T) {
	resp := makeOverLimitResponseWithStatuses()
	cc := &compiledConfig{
		domain:                  "test",
		enableXRateLimitHeaders: true,
		rateLimitedStatus:       429,
	}
	f := &filter{cc: cc, responseStatuses: resp.GetStatuses()}

	// Pre-populate the carrier mirroring beginLocalReply's `merged := ...`
	// after the inline-emit OrderedHeaders from applyOverLimit:
	//   [a]    x-envoy-ratelimited
	//   [b]    RLS X-From-Rls
	//   [c-pre] X-RateLimit triple (the post-fix inline slot)
	//   [c]    filter-config X-From-Config
	headers := http.Header{}
	headers.Add("x-envoy-ratelimited", "true")
	headers.Add("X-From-Rls", "rls-1")
	headers.Add(headerXRateLimitLimit, `10, 100;w=60;name="global", 10;w=1;name="per-ip"`)
	headers.Add(headerXRateLimitRemaining, "2")
	headers.Add(headerXRateLimitReset, "1")
	headers.Add("X-From-Config", "config-1")

	if got := f.EncodeHeaders(headers, true); got != envoyhttp.Continue {
		t.Errorf("EncodeHeaders status: got %v, want Continue", got)
	}

	// X-RateLimit values are byte-identical (no-op-set-equal idempotence).
	expectXRateLimitTriple(t, headers,
		`10, 100;w=60;name="global", 10;w=1;name="per-ip"`,
		"2",
		"1",
	)
	// Each X-RateLimit key has a SINGLE value (Set overwrites; the original
	// pre-populated value is replaced by the byte-identical encode-side
	// value — net result: 1 value, byte-equal). This guards against a
	// regression where the encode hook accidentally Adds (appends) rather
	// than Sets (overwrites), which would produce duplicate header values
	// on the wire.
	for _, k := range []string{headerXRateLimitLimit, headerXRateLimitRemaining, headerXRateLimitReset} {
		if got := len(headers.Values(k)); got != 1 {
			t.Errorf("header %q: got %d values, want 1 (no-op-set-equal idempotence — Set must overwrite, not append)", k, got)
		}
	}
	// The filter-config slot survives unchanged — the encode hook only
	// touches the X-RateLimit triple via Set.
	if got := headers.Get("X-From-Config"); got != "config-1" {
		t.Errorf("X-From-Config: got %q, want %q (filter-config slot preserved)", got, "config-1")
	}
	if got := headers.Get("X-From-Rls"); got != "rls-1" {
		t.Errorf("X-From-Rls: got %q, want %q (RLS slot preserved)", got, "rls-1")
	}
	if got := headers.Get("x-envoy-ratelimited"); got != "true" {
		t.Errorf("x-envoy-ratelimited: got %q, want %q (AMEND-8 first slot preserved)", got, "true")
	}
}

// TestEncodeHeaders_CanonicalCase_Insensitive documents the http.Header.Set
// canonical-case behavior: keys written via Set are canonicalized via
// textproto.CanonicalMIMEHeaderKey. The wire header name is case-insensitive
// per RFC 7230 §3.2, so the canonical form ("X-Ratelimit-Limit") is
// semantically equivalent to upstream's "x-ratelimit-limit". The test pins
// the canonical-form retrieval works as expected.
func TestEncodeHeaders_CanonicalCase_Insensitive(t *testing.T) {
	resp := makeOKResponseWithStatuses()
	cc := &compiledConfig{
		domain:                  "test",
		enableXRateLimitHeaders: true,
	}
	f := &filter{cc: cc, responseStatuses: resp.GetStatuses()}

	headers := http.Header{}
	f.EncodeHeaders(headers, true)

	// http.Header.Get is canonical-case insensitive — both lookups succeed.
	if got := headers.Get("x-ratelimit-limit"); got == "" {
		t.Error("headers.Get(\"x-ratelimit-limit\"): got empty, want non-empty (canonical-case insensitive)")
	}
	if got := headers.Get("X-Ratelimit-Limit"); got == "" {
		t.Error("headers.Get(\"X-Ratelimit-Limit\"): got empty, want non-empty (canonical-case form)")
	}
	// Defensive: the underlying map key form is canonical.
	for k := range headers {
		if strings.EqualFold(k, "x-ratelimit-limit") && k != "X-Ratelimit-Limit" {
			t.Logf("note: header stored under non-canonical key %q (http.Header.Set canonicalizes — this should be the canonical form)", k)
		}
	}
}
