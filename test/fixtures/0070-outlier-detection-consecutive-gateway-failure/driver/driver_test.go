package driver

import (
	"testing"

	"github.com/esalaine/envoy-go/test/differential/fixture"
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
		{"backend-1:health", 1},
		{"backend-0:foo", 0},
		{"backend-2:/", 2}, // the 503 host's GET / body before ejection
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
// reference_fixture_workload_constant_desync class of bug). The gateway-first
// short-circuit hinges on consec5xxThreshold being set HIGH (so the 5xx detector
// CANNOT fire) and consecGwThreshold being the real ejection trigger.
func TestConstants(t *testing.T) {
	if healthyBackendCount != 2 {
		t.Errorf("healthyBackendCount=%d, want 2 (2 HTTPEcho hosts; the 3rd is the always-503)", healthyBackendCount)
	}
	if backendCount != 3 {
		t.Errorf("backendCount=%d, want 3 (2 HTTPEcho + 1 HTTP503Responder)", backendCount)
	}
	if consecGwThreshold != 5 {
		t.Errorf("consecGwThreshold=%d, want 5", consecGwThreshold)
	}
	if enforcingGwPercent != 100 {
		t.Errorf("enforcingGwPercent=%d, want 100 (enforce every gateway-detected ejection)", enforcingGwPercent)
	}
	// consec5xx is set HIGH so the consecutive_5xx detector CANNOT fire over the
	// eject-drive — the gateway detector ejects first. This is the load-bearing
	// invariant behind the cross-side detected_consecutive_5xx == 0 assertion.
	if consec5xxThreshold <= ejectDriveRequests {
		t.Errorf("consec5xxThreshold=%d must exceed ejectDriveRequests=%d so the 5xx detector cannot fire (gateway-first short-circuit)",
			consec5xxThreshold, ejectDriveRequests)
	}
	if maxEjectionPercent != 100 {
		t.Errorf("maxEjectionPercent=%d, want 100", maxEjectionPercent)
	}
	// The ejection drive must pick the 503 host at least consecGwThreshold times.
	// Under strict round-robin over 3 hosts the 503 host is picked every 3rd
	// request, so ejectDriveRequests must exceed consecGwThreshold*3.
	if ejectDriveRequests <= consecGwThreshold*3 {
		t.Errorf("ejectDriveRequests=%d must exceed consecGwThreshold*3=%d (else the 503 host is not picked enough to eject)",
			ejectDriveRequests, consecGwThreshold*3)
	}
	if n <= 0 {
		t.Errorf("n=%d, want > 0", n)
	}
}

// TestBackendKindAt pins the per-host backend-kind mapping: hosts 0/1 are
// HTTPEcho (healthy), host 2 is the HTTP503Responder (always-503).
func TestBackendKindAt(t *testing.T) {
	d := &odDriver{}
	for i := 0; i < backendCount; i++ {
		got := d.BackendKindAt(i)
		want := fixture.HTTPEcho
		if i == 2 {
			want = fixture.HTTP503Responder
		}
		if got != want {
			t.Errorf("BackendKindAt(%d)=%v, want %v", i, got, want)
		}
	}
}
