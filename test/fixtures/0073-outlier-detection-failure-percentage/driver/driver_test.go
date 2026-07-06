package driver

import (
	"testing"

	"github.com/pgdad/envoy-go/test/differential/fixture"
)

// TestBackendIdxFromBody_OK: both the HTTPEcho ("backend-<idx>:<seg>") and the
// HTTP503Responder ("backend-<idx>:<seg>") canned bodies parse to the embedded
// idx (the host-attribution signal the measured-phase tally uses).
func TestBackendIdxFromBody_OK(t *testing.T) {
	for _, tc := range []struct {
		body string
		want int
	}{
		{"backend-0:", 0},
		{"backend-4:health", 4},
		{"backend-0:foo", 0},
		{"backend-5:/", 5}, // the 503 host's GET / body before ejection
	} {
		got, err := backendIdxFromBody([]byte(tc.body))
		if err != nil {
			t.Errorf("body %q: unexpected error: %v", tc.body, err)
			continue
		}
		if got != tc.want {
			t.Errorf("body %q: got idx %d, want %d", tc.body, got, tc.want)
		}
	}
}

// TestBackendIdxFromBody_Bad: malformed bodies (no prefix / no colon / non-numeric
// idx) fail — the measured-phase tally treats a parse failure as a hard error.
func TestBackendIdxFromBody_Bad(t *testing.T) {
	for _, body := range []string{
		"",
		"hello",
		"backend-0",   // no colon
		"backend-x:y", // non-numeric idx
		"upstream-0:", // wrong prefix
	} {
		if _, err := backendIdxFromBody([]byte(body)); err == nil {
			t.Errorf("body %q: expected error, got nil", body)
		}
	}
}

// TestConstants pins the single-sourced workload constants (guards against the
// reference_fixture_workload_constant_desync class of bug).
func TestConstants(t *testing.T) {
	if healthyBackendCount != 5 {
		t.Errorf("healthyBackendCount=%d, want 5 (5 HTTPEcho hosts; the 6th is the always-503)", healthyBackendCount)
	}
	if backendCount != 6 {
		t.Errorf("backendCount=%d, want 6 (5 HTTPEcho + 1 HTTP503Responder)", backendCount)
	}
	if badHostIdx != 5 {
		t.Errorf("badHostIdx=%d, want 5 (the last endpoint is the always-503 host)", badHostIdx)
	}
	if maxEjectionPercent != 100 {
		t.Errorf("maxEjectionPercent=%d, want 100", maxEjectionPercent)
	}
	// The failure_percentage detector is the SOLE enforcer here: enforcing_failure_
	// percentage 100, enforcing_success_rate 0 (success_rate detect-only).
	if fpThreshold != 85 {
		t.Errorf("fpThreshold=%d, want 85", fpThreshold)
	}
	// success_rate_minimum_hosts / failure_percentage_minimum_hosts must be
	// satisfiable: with 6 endpoints, all 6 are eligible at the sweep.
	if srMinHosts > backendCount {
		t.Errorf("srMinHosts=%d exceeds backendCount=%d", srMinHosts, backendCount)
	}
	if fpMinHosts > backendCount {
		t.Errorf("fpMinHosts=%d exceeds backendCount=%d (no host eligible → never ejects)", fpMinHosts, backendCount)
	}
	// The bad host must accrue >= reqVolume requests within one interval so it is
	// eligible at the next sweep. Under round-robin over 6 hosts it gets
	// ejectDriveRequests/6 picks; that must comfortably exceed reqVolume.
	if ejectDriveRequests/backendCount <= reqVolume {
		t.Errorf("ejectDriveRequests/backendCount=%d must exceed reqVolume=%d (else the bad host is not eligible for the SR/FP detectors)",
			ejectDriveRequests/backendCount, reqVolume)
	}
	if n <= 0 {
		t.Errorf("n=%d, want > 0", n)
	}
}

// TestBackendKindAt pins the per-host backend-kind mapping: hosts 0..4 are
// HTTPEcho (healthy), host 5 is the HTTP503Responder (always-503).
func TestBackendKindAt(t *testing.T) {
	d := &odDriver{}
	for i := 0; i < backendCount; i++ {
		got := d.BackendKindAt(i)
		want := fixture.HTTPEcho
		if i == badHostIdx {
			want = fixture.HTTP503Responder
		}
		if got != want {
			t.Errorf("BackendKindAt(%d)=%v, want %v", i, got, want)
		}
	}
}
