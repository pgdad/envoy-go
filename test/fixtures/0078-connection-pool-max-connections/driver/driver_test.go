package driver

import "testing"

// TestBackendIdxFromBody_OK: the BlockingHoldResponder canned body
// "backend-<idx>:<seg>" parses to the embedded idx (the host-attribution signal
// the held-request tally uses). A GET / yields "backend-0:" (empty seg).
func TestBackendIdxFromBody_OK(t *testing.T) {
	for _, tc := range []struct {
		body string
		want int
	}{
		{"backend-0:", 0},    // GET / (empty seg) — the held-request body
		{"backend-0:foo", 0}, // GET /foo
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

// TestBackendIdxFromBody_Bad: malformed bodies (no prefix / no colon / non-numeric
// idx) fail — the tally treats a parse failure as a hard error.
func TestBackendIdxFromBody_Bad(t *testing.T) {
	for _, body := range []string{
		"",
		"hello",
		"backend-0",       // no colon
		"backend-x:y",     // non-numeric idx
		"released",        // the /__release control-response body
		"released-sticky", // the /__release_sticky control-response body
	} {
		if _, err := backendIdxFromBody([]byte(body)); err == nil {
			t.Errorf("body %q: expected error, got nil", body)
		}
	}
}

// TestConstants pins the single-sourced workload constants (guards against the
// reference_fixture_workload_constant_desync class of bug). The asserter's
// fill/pend/oversub counts all derive from N/M/J — these are the single source.
func TestConstants(t *testing.T) {
	if backendCount != 1 {
		t.Errorf("backendCount=%d, want 1 (one BlockingHoldResponder host)", backendCount)
	}
	if maxConnections != 2 {
		t.Errorf("maxConnections=%d, want 2 (N)", maxConnections)
	}
	if maxPendingRequests != 2 {
		t.Errorf("maxPendingRequests=%d, want 2 (M)", maxPendingRequests)
	}
	if oversub != 2 {
		t.Errorf("oversub=%d, want 2 (J)", oversub)
	}
	if clusterName != "c_cp" {
		t.Errorf("clusterName=%q, want c_cp", clusterName)
	}
	if refContainerListenerPort != 19167 {
		t.Errorf("refContainerListenerPort=%d, want 19167 (next-free)", refContainerListenerPort)
	}
	// The reference burst must oversubscribe more heavily than the subject's exact
	// J (the soft breaker needs slack to GUARANTEE >=1 overflow).
	if refOversub <= oversub {
		t.Errorf("refOversub=%d must exceed oversub=%d (reference soft-breaker slack)", refOversub, oversub)
	}
}
