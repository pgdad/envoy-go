package driver

import (
	"testing"
)

// TestBackendIdxFromBody_OK: the HTTPEcho canned body "backend-<idx>:<seg>"
// parses to the embedded idx (the host-attribution signal the measured-phase
// tally uses). Only hosts 0/1 are live HTTPEcho backends; host2 is the dead
// allocDeadPort host (it serves nothing — a connect to it is refused).
func TestBackendIdxFromBody_OK(t *testing.T) {
	for _, tc := range []struct {
		body string
		want int
	}{
		{"backend-0:", 0},
		{"backend-1:health", 1},
		{"backend-0:foo", 0},
		{"backend-1:/", 1},
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

// TestAllocDeadPort_Memoized pins the 0066 dead-host mechanism: allocDeadPort
// binds 0.0.0.0:0, captures the assigned port, closes the listener (so the port
// stays unbound → a connect is refused), and MEMOIZES the result so both
// ReferenceBootstrap and SubjectConfig agree on the SAME port number. A non-zero
// stable port across calls is the load-bearing property: a per-call re-alloc
// would point the two sides' dead endpoint at DIFFERENT ports.
func TestAllocDeadPort_Memoized(t *testing.T) {
	d := &odDriver{}
	first := d.allocDeadPort()
	if first <= 0 {
		t.Fatalf("allocDeadPort()=%d, want a positive port", first)
	}
	for i := 0; i < 5; i++ {
		if got := d.allocDeadPort(); got != first {
			t.Fatalf("allocDeadPort() call %d = %d, want memoized %d (both sides must agree on the dead port)", i, got, first)
		}
	}
}

// TestConstants pins the single-sourced workload constants (guards against the
// reference_fixture_workload_constant_desync class of bug). split=true is the
// load-bearing invariant: it routes the dead-host connect-refused (LocalOriginErr)
// to the local-origin detector ONLY, leaving the 5xx/gateway detectors silent
// (detected_consecutive_5xx == 0).
func TestConstants(t *testing.T) {
	if backendCount != 2 {
		t.Errorf("backendCount=%d, want 2 (only the 2 LIVE HTTPEcho hosts are runner-spawned; host2 is the dead allocDeadPort host)", backendCount)
	}
	if healthyBackendCount != 2 {
		t.Errorf("healthyBackendCount=%d, want 2", healthyBackendCount)
	}
	if endpointCount != 3 {
		t.Errorf("endpointCount=%d, want 3 (2 live + 1 dead)", endpointCount)
	}
	if !splitLocalOrigin {
		t.Errorf("splitLocalOrigin=%v, want true (split routes local-origin failures to the LO detector ONLY)", splitLocalOrigin)
	}
	if consecLOThreshold != 5 {
		t.Errorf("consecLOThreshold=%d, want 5", consecLOThreshold)
	}
	if enforcingLOPercent != 100 {
		t.Errorf("enforcingLOPercent=%d, want 100 (enforce every local-origin-detected ejection)", enforcingLOPercent)
	}
	if maxEjectionPercent != 100 {
		t.Errorf("maxEjectionPercent=%d, want 100", maxEjectionPercent)
	}
	// The ejection drive must pick the dead host at least consecLOThreshold times.
	// Under strict round-robin over 3 endpoints the dead host is picked every 3rd
	// request, so ejectDriveRequests must exceed consecLOThreshold*3.
	if ejectDriveRequests <= consecLOThreshold*3 {
		t.Errorf("ejectDriveRequests=%d must exceed consecLOThreshold*3=%d (else the dead host is not picked enough to eject)",
			ejectDriveRequests, consecLOThreshold*3)
	}
	if n <= 0 {
		t.Errorf("n=%d, want > 0", n)
	}
}
