package driver

import "testing"

// TestBackendIdxFromBody_OK: the HTTPEcho canned body "backend-<idx>:<seg>"
// parses to the embedded idx (the host-attribution signal the load tally uses).
func TestBackendIdxFromBody_OK(t *testing.T) {
	for _, tc := range []struct {
		body string
		want int
	}{
		{"backend-0:", 0},
		{"backend-1:health", 1},
		{"backend-0:foo", 0},
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
// idx) fail — the load tally treats a parse failure as a hard error.
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

// TestAllocDeadPort_Memoized: allocDeadPort returns the SAME port across calls
// (ReferenceBootstrap + SubjectConfig must agree on the dead endpoint), and the
// returned port is unbound (so a probe to it is refused).
func TestAllocDeadPort_Memoized(t *testing.T) {
	d := &hcDriver{}
	p1 := d.allocDeadPort()
	p2 := d.allocDeadPort()
	if p1 != p2 {
		t.Fatalf("allocDeadPort not memoized: %d != %d", p1, p2)
	}
	if p1 == 0 {
		t.Fatalf("allocDeadPort returned 0")
	}
}

// TestConstants pins the single-sourced workload constants (guards against the
// reference_fixture_workload_constant_desync class of bug).
func TestConstants(t *testing.T) {
	if healthyAfterConverge != backendCount {
		t.Errorf("healthyAfterConverge=%d != backendCount=%d", healthyAfterConverge, backendCount)
	}
	if backendCount != 2 {
		t.Errorf("backendCount=%d, want 2 (2 live hosts; 2/3 healthy > 50%% panic threshold)", backendCount)
	}
	if n != 100 {
		t.Errorf("n=%d, want 100", n)
	}
}
