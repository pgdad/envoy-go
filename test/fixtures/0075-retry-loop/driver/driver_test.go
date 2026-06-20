package driver

import (
	"testing"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

// TestBackendIdxFromBody_OK: both the HTTPEcho ("backend-<idx>:<seg>") and the
// HTTP503Responder ("backend-<idx>:<seg>") canned bodies parse to the embedded
// idx (the host-attribution signal). Copied per-fixture (the 0066/0069
// precedent) so the helper stays local to the driver package.
func TestBackendIdxFromBody_OK(t *testing.T) {
	for _, tc := range []struct {
		body string
		want int
	}{
		{"backend-0:", 0},
		{"backend-1:recover", 1},
		{"backend-0:exhaust", 0},
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
// idx) fail.
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
// reference_fixture_workload_constant_desync class of bug — D-S42-6). The driver
// is the single source; this test imports them rather than hand-rolling copies.
func TestConstants(t *testing.T) {
	if backendCount != 2 {
		t.Errorf("backendCount=%d, want 2 (1 HTTP503Responder + 1 HTTPEcho)", backendCount)
	}
	if numRetries != 3 {
		t.Errorf("numRetries=%d, want 3 (the /exhaust num_retries)", numRetries)
	}
	if recoverRetries != 1 {
		t.Errorf("recoverRetries=%d, want 1 (the /recover num_retries; the retry-once-onto-fresh-host invariant)", recoverRetries)
	}
	if clusterExhaust != "c_exhaust" {
		t.Errorf("clusterExhaust=%q, want c_exhaust", clusterExhaust)
	}
	if clusterRecover != "c_recover" {
		t.Errorf("clusterRecover=%q, want c_recover", clusterRecover)
	}
	if statPrefix == "" {
		t.Error("statPrefix must be non-empty (it keys the http.<prefix>.downstream_rq_2xx stat)")
	}
	// recoverReqs is the K recover-arm request count; it must be even (D-S42-6:
	// an even K keeps the round-robin spread balanced regardless of the
	// randomized initial offset) and positive.
	if recoverReqs <= 0 {
		t.Errorf("recoverReqs=%d, want > 0", recoverReqs)
	}
	if recoverReqs%2 != 0 {
		t.Errorf("recoverReqs=%d, want even", recoverReqs)
	}
}

// TestBackendKindAt pins the per-host backend-kind mapping: host 0 is the
// always-503 responder (the exhaustion + recover-via-retry target), host 1 is a
// healthy HTTPEcho (the recover landing host).
func TestBackendKindAt(t *testing.T) {
	d := &retryDriver{}
	for i := 0; i < backendCount; i++ {
		got := d.BackendKindAt(i)
		want := fixture.HTTPEcho
		if i == 0 {
			want = fixture.HTTP503Responder
		}
		if got != want {
			t.Errorf("BackendKindAt(%d)=%v, want %v", i, got, want)
		}
	}
}
