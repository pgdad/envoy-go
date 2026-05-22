package admission_control

// stats_test.go — byte-exact stat-name guards per phase-23 IMPL Task 3 +
// SPEC §6.6 + AMEND-3. Task 3 OWNS these assertions; Task 5's test file does
// NOT re-assert stat names.
//
// Two-layer guard strategy (per stats.go doc-comment):
//  1. The const declarations in stats.go pin byte-exact wire names at build time.
//  2. The TestStatNames_Equal_* tests here pin each const to its expected string
//     literal so that a future refactor cannot silently rename the const alongside
//     the string.
//
// Additionally, TestNewFilterStats_* verifies the constructor wires each counter
// under the correct full dotted name (hcmPrefix + ".admission_control." + name),
// confirming AMEND-3's literal infix "admission_control." and the prefix template
// http.<HCM_stat_prefix>.admission_control.<stat>.

import (
	"strings"
	"testing"

	"github.com/esalaine/envoy-go/internal/stats"
)

// -----------------------------------------------------------------------------
// Byte-exact stat-name constant guards (SPEC §6.6 + AMEND-3).
// Task 3 OWNS these — do NOT duplicate in Task 5 tests.
// -----------------------------------------------------------------------------

// TestStatNames_Equal_RqRejected pins statNameRqRejected to its upstream
// wire name per AMEND-3 (admission_control.h:35-38 COUNTER macro).
func TestStatNames_Equal_RqRejected(t *testing.T) {
	const want = "rq_rejected"
	if statNameRqRejected != want {
		t.Errorf("statNameRqRejected = %q; want %q", statNameRqRejected, want)
	}
}

// TestStatNames_Equal_RqSuccess pins statNameRqSuccess to its upstream
// wire name per AMEND-3.
func TestStatNames_Equal_RqSuccess(t *testing.T) {
	const want = "rq_success"
	if statNameRqSuccess != want {
		t.Errorf("statNameRqSuccess = %q; want %q", statNameRqSuccess, want)
	}
}

// TestStatNames_Equal_RqFailure pins statNameRqFailure to its upstream
// wire name per AMEND-3 (NOTE: rq_failure NOT rq_error — AMEND-3 correction
// of the BRAINSTORM hypothesis; verified via ALL_ADMISSION_CONTROL_STATS macro).
func TestStatNames_Equal_RqFailure(t *testing.T) {
	const want = "rq_failure"
	if statNameRqFailure != want {
		t.Errorf("statNameRqFailure = %q; want %q", statNameRqFailure, want)
	}
}

// TestStatNames_NotRqError guards against a regression to the pre-AMEND-3
// BRAINSTORM name "rq_error". If statNameRqFailure were ever accidentally reset
// to "rq_error" the test above would catch it; this test makes the intent
// explicit for reviewers.
func TestStatNames_NotRqError(t *testing.T) {
	if statNameRqFailure == "rq_error" {
		t.Errorf("statNameRqFailure = %q; must be %q per AMEND-3 (NOT rq_error)", statNameRqFailure, "rq_failure")
	}
}

// TestStatNames_Count verifies there are exactly 3 stat names and that all three
// are distinct — the COUNTER-only roster per AMEND-3 (no gauges, no histograms).
// Distinctness catches a real bug: two consts accidentally set to the same string.
func TestStatNames_Count(t *testing.T) {
	names := []string{statNameRqRejected, statNameRqSuccess, statNameRqFailure}
	const wantCount = 3
	if len(names) != wantCount {
		t.Errorf("stat name count = %d; want exactly %d (COUNTER-only per AMEND-3)", len(names), wantCount)
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if seen[n] {
			t.Errorf("duplicate stat name %q — each const must be distinct", n)
		}
		seen[n] = true
	}
}

// -----------------------------------------------------------------------------
// newFilterStats constructor tests — prefix wiring + full dotted names.
// -----------------------------------------------------------------------------

// TestNewFilterStats_PrefixWiring verifies that newFilterStats registers each
// counter under the full name hcmPrefix+".admission_control."+statName, matching
// the AMEND-3 prefix template http.<HCM_stat_prefix>.admission_control.<stat>.
func TestNewFilterStats_PrefixWiring(t *testing.T) {
	reg := stats.NewRegistry()
	const hcmPrefix = "http.ingress_http"
	fs := newFilterStats(reg, hcmPrefix)

	// Confirm the counters are non-nil (allocated).
	if fs.rqRejected == nil {
		t.Error("rqRejected is nil")
	}
	if fs.rqSuccess == nil {
		t.Error("rqSuccess is nil")
	}
	if fs.rqFailure == nil {
		t.Error("rqFailure is nil")
	}

	// Confirm the full dotted names via counter.Name().
	type nameCase struct {
		got  string
		want string
	}
	cases := []nameCase{
		{fs.rqRejected.Name(), hcmPrefix + ".admission_control." + statNameRqRejected},
		{fs.rqSuccess.Name(), hcmPrefix + ".admission_control." + statNameRqSuccess},
		{fs.rqFailure.Name(), hcmPrefix + ".admission_control." + statNameRqFailure},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("counter name = %q; want %q", tc.got, tc.want)
		}
	}
}

// TestNewFilterStats_InfixLiteral verifies the literal ".admission_control."
// infix (per config.cc:29) is present in each counter name. This is a belt-and-
// suspenders check on top of TestNewFilterStats_PrefixWiring.
func TestNewFilterStats_InfixLiteral(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "http.test")

	const infix = ".admission_control."
	for _, name := range []string{
		fs.rqRejected.Name(),
		fs.rqSuccess.Name(),
		fs.rqFailure.Name(),
	} {
		if !strings.Contains(name, infix) {
			t.Errorf("counter name %q does not contain infix %q", name, infix)
		}
	}
}

// TestNewFilterStats_Inc verifies that counters allocated by newFilterStats
// are functional (Inc works without panicking).
func TestNewFilterStats_Inc(t *testing.T) {
	reg := stats.NewRegistry()
	fs := newFilterStats(reg, "http.test2")

	// Should not panic.
	fs.rqRejected.Inc()
	fs.rqSuccess.Inc()
	fs.rqFailure.Inc()

	if v := fs.rqRejected.Load(); v != 1 {
		t.Errorf("rqRejected after Inc = %d; want 1", v)
	}
	if v := fs.rqSuccess.Load(); v != 1 {
		t.Errorf("rqSuccess after Inc = %d; want 1", v)
	}
	if v := fs.rqFailure.Load(); v != 1 {
		t.Errorf("rqFailure after Inc = %d; want 1", v)
	}
}
