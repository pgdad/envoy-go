package driver

import "testing"

// TestConstants pins the single-sourced workload constants (guards against the
// reference_fixture_workload_constant_desync class of bug). The six-stage drive's
// expected stat values derive from these.
func TestConstants(t *testing.T) {
	if backendCount != 1 {
		t.Errorf("backendCount=%d, want 1 (one H2GoawayResponder host)", backendCount)
	}
	// C (max_concurrent_streams) is HIGH so it NEVER binds: the rotation is about
	// conn IDENTITY (the GOAWAY drives the multi-conn count), not the local stream
	// cap. A single in-flight stream + the control request share one conn ⇒ C must
	// be >= 2 (and is deliberately well above).
	if streamCap < 2 {
		t.Errorf("streamCap=%d, want >= 2 (C must not bind: one held stream + one control request share a conn)", streamCap)
	}
	if streamCap != 100 {
		t.Errorf("streamCap=%d, want 100 (the pinned non-binding C)", streamCap)
	}
	if cluster != "c_h2gw" {
		t.Errorf("cluster=%q, want c_h2gw", cluster)
	}
	if refContainerListenerPort != 19169 {
		t.Errorf("refContainerListenerPort=%d, want 19169 (next-free after 0079's 19168)", refContainerListenerPort)
	}
}

// TestStatKey pins the cluster-scoped stat-name shape (the asserted counters all
// flow through statKey).
func TestStatKey(t *testing.T) {
	for _, tc := range []struct {
		suffix string
		want   string
	}{
		{"upstream_cx_http2_total", "cluster.c_h2gw.upstream_cx_http2_total"},
		{"upstream_cx_active", "cluster.c_h2gw.upstream_cx_active"},
		{"http2.streams_active", "cluster.c_h2gw.http2.streams_active"},
		{"http2.rx_reset", "cluster.c_h2gw.http2.rx_reset"},
		{"http2.tx_reset", "cluster.c_h2gw.http2.tx_reset"},
	} {
		if got := statKey(tc.suffix); got != tc.want {
			t.Errorf("statKey(%q) = %q, want %q", tc.suffix, got, tc.want)
		}
	}
}
