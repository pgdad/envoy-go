package oauth2

import (
	"testing"

	"github.com/pgdad/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Group 9 — stat-name byte-exact assertions + 6-counter registration test per
// SPEC §6.11 + §6.12 + planner-time D6 + ADR-0181 + ADR-0143 SN2-reuse.
//
// The 6 byte-exact upstream stat names per phase-20 AMEND-4 + S5 + §20.P8
// REFUTED (BRAINSTORM over-counted at 8). A regression on any of the 6
// `statName*` constants would surface here AND at the registration test
// (which asserts the full HCM-rooted SN2-reuse prefix shape
// `http.<HCM_stat_prefix>.oauth2.<counter>` per ADR-0143).
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// Per-counter byte-exact name assertions (the compile-time guard per D6)
// -----------------------------------------------------------------------------

// TestStatNames_Equal_OauthUnauthorizedRq pins the byte-exact upstream name
// per AMEND-4 + §4.6. A regression here would break wire-name compat with
// reference Envoy v1.37.2.
func TestStatNames_Equal_OauthUnauthorizedRq(t *testing.T) {
	if statNameOauthUnauthorizedRq != "oauth_unauthorized_rq" {
		t.Errorf("statNameOauthUnauthorizedRq: got %q, want %q",
			statNameOauthUnauthorizedRq, "oauth_unauthorized_rq")
	}
}

// TestStatNames_Equal_OauthFailure pins the byte-exact upstream name.
func TestStatNames_Equal_OauthFailure(t *testing.T) {
	if statNameOauthFailure != "oauth_failure" {
		t.Errorf("statNameOauthFailure: got %q, want %q", statNameOauthFailure, "oauth_failure")
	}
}

// TestStatNames_Equal_OauthPassthrough pins the byte-exact upstream name.
func TestStatNames_Equal_OauthPassthrough(t *testing.T) {
	if statNameOauthPassthrough != "oauth_passthrough" {
		t.Errorf("statNameOauthPassthrough: got %q, want %q",
			statNameOauthPassthrough, "oauth_passthrough")
	}
}

// TestStatNames_Equal_OauthSuccess pins the byte-exact upstream name.
func TestStatNames_Equal_OauthSuccess(t *testing.T) {
	if statNameOauthSuccess != "oauth_success" {
		t.Errorf("statNameOauthSuccess: got %q, want %q", statNameOauthSuccess, "oauth_success")
	}
}

// TestStatNames_Equal_OauthRefreshtokenSuccess pins the byte-exact upstream
// name. Note: the upstream wire name uses `refreshtoken` (no underscore
// between "refresh" and "token") — this is INTENTIONAL upstream byte-exact
// reuse; the Go field name uses `Refreshtoken` for parity. A regression here
// would surface as a wire-name mismatch.
func TestStatNames_Equal_OauthRefreshtokenSuccess(t *testing.T) {
	if statNameOauthRefreshtokenSuccess != "oauth_refreshtoken_success" {
		t.Errorf("statNameOauthRefreshtokenSuccess: got %q, want %q",
			statNameOauthRefreshtokenSuccess, "oauth_refreshtoken_success")
	}
}

// TestStatNames_Equal_OauthRefreshtokenFailure pins the byte-exact upstream
// name.
func TestStatNames_Equal_OauthRefreshtokenFailure(t *testing.T) {
	if statNameOauthRefreshtokenFailure != "oauth_refreshtoken_failure" {
		t.Errorf("statNameOauthRefreshtokenFailure: got %q, want %q",
			statNameOauthRefreshtokenFailure, "oauth_refreshtoken_failure")
	}
}

// -----------------------------------------------------------------------------
// 6-counter registration test
// -----------------------------------------------------------------------------

// TestNewFilterStats_Registers6Counters verifies newFilterStats allocates
// exactly 6 counters with the wire-exact HCM-rooted SN2-reuse prefix per
// ADR-0143 + §5.3. Mirrors extauthz Group 2's
// TestFilterStats_ExactlySixCounters_NoExtras shape.
func TestNewFilterStats_Registers6Counters(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "ingress_http")
	if fs == nil {
		t.Fatal("newFilterStats: got nil, want non-nil")
	}
	// Each of the 6 counter handles must be non-nil.
	cases := []struct {
		handle *stats.Counter
		field  string
	}{
		{fs.oauthUnauthorizedRq, "oauthUnauthorizedRq"},
		{fs.oauthFailure, "oauthFailure"},
		{fs.oauthPassthrough, "oauthPassthrough"},
		{fs.oauthSuccess, "oauthSuccess"},
		{fs.oauthRefreshtokenSuccess, "oauthRefreshtokenSuccess"},
		{fs.oauthRefreshtokenFailure, "oauthRefreshtokenFailure"},
	}
	for _, tc := range cases {
		if tc.handle == nil {
			t.Errorf("filterStats.%s is nil", tc.field)
		}
	}

	// Registered names must use the HCM-rooted SN2-reuse prefix per ADR-0143
	// + SPEC §5.3: `http.<HCM_stat_prefix>.oauth2.<counter>`.
	var names []string
	reg.Walk(func(m stats.Metric) {
		names = append(names, m.Name())
	})
	if len(names) != 6 {
		t.Errorf("Registry size = %d; want exactly 6 counters (no extras); got names=%v", len(names), names)
	}
	want := []string{
		"http.ingress_http.oauth2.oauth_unauthorized_rq",
		"http.ingress_http.oauth2.oauth_failure",
		"http.ingress_http.oauth2.oauth_passthrough",
		"http.ingress_http.oauth2.oauth_success",
		"http.ingress_http.oauth2.oauth_refreshtoken_success",
		"http.ingress_http.oauth2.oauth_refreshtoken_failure",
	}
	for _, w := range want {
		found := false
		for _, n := range names {
			if n == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("counter %q not registered; got names=%v", w, names)
		}
	}
}

// TestNewFilterStats_EmptyPrefix_FoldsToBarePrefixShape verifies the empty-
// HCM-stat_prefix fold per the baseStatPrefix discipline (mirrors the extauthz
// pattern at ADR-0156): when hcmStatPrefix == "", the counter names use the
// bare `oauth2.<counter>` form to satisfy the Registry's nameRE (forbids
// leading dots / double dots).
func TestNewFilterStats_EmptyPrefix_FoldsToBarePrefixShape(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "")
	if fs == nil {
		t.Fatal("newFilterStats with empty prefix: got nil, want non-nil")
	}
	cases := []struct {
		handle *stats.Counter
		want   string
	}{
		{fs.oauthUnauthorizedRq, "oauth2.oauth_unauthorized_rq"},
		{fs.oauthFailure, "oauth2.oauth_failure"},
		{fs.oauthPassthrough, "oauth2.oauth_passthrough"},
		{fs.oauthSuccess, "oauth2.oauth_success"},
		{fs.oauthRefreshtokenSuccess, "oauth2.oauth_refreshtoken_success"},
		{fs.oauthRefreshtokenFailure, "oauth2.oauth_refreshtoken_failure"},
	}
	for _, tc := range cases {
		if tc.handle == nil {
			t.Errorf("counter for %q: handle is nil with empty prefix", tc.want)
			continue
		}
		if got := tc.handle.Name(); got != tc.want {
			t.Errorf("counter Name() = %q; want %q (bare form, no http. prefix)", got, tc.want)
		}
	}
}

// TestNewFilterStats_IdempotentRegistration verifies that calling
// newFilterStats twice on the same Registry with the same prefix is safe
// (returns the same counter handles). Mirrors the extauthz NewCounterIfAbsent
// pattern per ADR-0156 + the multi-listener-same-prefix footgun avoidance.
func TestNewFilterStats_IdempotentRegistration(t *testing.T) {
	reg := stats.NewRegistry()
	fs1 := newFilterStats(reg, "ingress_http")
	fs2 := newFilterStats(reg, "ingress_http")
	if fs1.oauthSuccess != fs2.oauthSuccess {
		t.Errorf("idempotent registration: distinct counter handles for same name")
	}
	// Registry must still have exactly 6 counters (not 12) after the duplicate
	// registration.
	var count int
	reg.Walk(func(m stats.Metric) { count++ })
	if count != 6 {
		t.Errorf("Registry size after duplicate registration: got %d; want 6", count)
	}
}
