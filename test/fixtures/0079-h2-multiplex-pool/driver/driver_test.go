package driver

import "testing"

// TestBackendIdxFromBody_OK: the H2HoldResponder canned body "backend-<idx>:<seg>"
// parses to the embedded idx (the host-attribution signal the held-stream tally
// uses). A GET /mp/0 yields "backend-0:0" (seg = the last path segment).
func TestBackendIdxFromBody_OK(t *testing.T) {
	for _, tc := range []struct {
		body string
		want int
	}{
		{"backend-0:", 0},  // empty seg
		{"backend-0:0", 0}, // GET /mp/0
		{"backend-0:5", 0}, // GET /mp/5
		{"backend-1:health", 1},
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

// TestBackendIdxFromBody_Bad: malformed bodies fail (the tally treats a parse
// failure as a hard error).
func TestBackendIdxFromBody_Bad(t *testing.T) {
	for _, body := range []string{
		"",
		"hello",
		"backend-0",   // no colon
		"backend-x:y", // non-numeric idx
		"released",    // the /__release control-response body
	} {
		if _, err := backendIdxFromBody([]byte(body)); err == nil {
			t.Errorf("body %q: expected error, got nil", body)
		}
	}
}

// TestConstants pins the single-sourced workload constants (guards against the
// reference_fixture_workload_constant_desync class of bug). The asserter's
// fill/multiplex/pend/oversub counts all derive from these.
func TestConstants(t *testing.T) {
	if backendCount != 1 {
		t.Errorf("backendCount=%d, want 1 (one H2HoldResponder host)", backendCount)
	}
	if streamCapMP != 2 {
		t.Errorf("streamCapMP=%d, want 2 (C for the ceil prong)", streamCapMP)
	}
	if heldK != 6 {
		t.Errorf("heldK=%d, want 6 (K fully-overlapping held streams)", heldK)
	}
	// ceil(K/C) = ceil(6/2) = 3 — the EXACT cross-side conn count.
	if expectedConnsMP != 3 {
		t.Errorf("expectedConnsMP=%d, want 3 (ceil(%d/%d))", expectedConnsMP, heldK, streamCapMP)
	}
	// The multiplex proof must be non-vacuous: ceil(K/C) << K.
	if expectedConnsMP >= heldK {
		t.Errorf("expectedConnsMP=%d not << heldK=%d (multiplex proof vacuous)", expectedConnsMP, heldK)
	}
	// The ceil prong's budgets must NOT bind (only C drives growth).
	if maxConnectionsMP < expectedConnsMP {
		t.Errorf("maxConnectionsMP=%d must be >= expectedConnsMP=%d (cap must not bind in the ceil prong)", maxConnectionsMP, expectedConnsMP)
	}
	if maxPendingMP < heldK {
		t.Errorf("maxPendingMP=%d must be >= heldK=%d (queue must not bind in the ceil prong)", maxPendingMP, heldK)
	}
	// The overflow prong's budgets are tight (1 conn, 1 pending slot).
	if streamCapOF != 1 || maxConnectionsOF != 1 || maxPendingOF != 1 {
		t.Errorf("overflow prong budgets = {C:%d, maxConn:%d, maxPend:%d}, want {1,1,1}", streamCapOF, maxConnectionsOF, maxPendingOF)
	}
	if clusterMP != "c_h2mp" {
		t.Errorf("clusterMP=%q, want c_h2mp", clusterMP)
	}
	if clusterOF != "c_h2of" {
		t.Errorf("clusterOF=%q, want c_h2of", clusterOF)
	}
	if refContainerListenerPort != 19168 {
		t.Errorf("refContainerListenerPort=%d, want 19168 (next-free)", refContainerListenerPort)
	}
}
