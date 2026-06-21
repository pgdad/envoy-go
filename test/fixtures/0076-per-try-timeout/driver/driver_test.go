package driver

import (
	"strings"
	"testing"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

// TestConstants pins the single-sourced workload constants (guards against the
// reference_fixture_workload_constant_desync class of bug). The driver is the
// single source; this test imports them rather than hand-rolling copies.
func TestConstants(t *testing.T) {
	if backendCount != 1 {
		t.Errorf("backendCount=%d, want 1 (the single BlockingHoldResponder)", backendCount)
	}
	if numRetries != 3 {
		t.Errorf("numRetries=%d, want 3 (the /ptt num_retries)", numRetries)
	}
	if clusterPtt != "c_ptt" {
		t.Errorf("clusterPtt=%q, want c_ptt", clusterPtt)
	}
	if statPrefix == "" {
		t.Error("statPrefix must be non-empty")
	}
	if perTryTimeout == "" {
		t.Error("perTryTimeout must be non-empty (the per-attempt deadline)")
	}
	if refContainerListenerPort != 19165 {
		t.Errorf("refContainerListenerPort=%d, want 19165 (next-free after 0075's 19164)", refContainerListenerPort)
	}
}

// TestStatKeys pins the cluster-scoped stat key strings the AssertStats delta
// asserts read (all interpolate clusterPtt) so a rename of clusterPtt cannot
// silently desync the keys from the YAML.
func TestStatKeys(t *testing.T) {
	for _, tc := range []struct {
		got, want string
	}{
		{statPerTryTimeout, "cluster.c_ptt.upstream_rq_per_try_timeout"},
		{statRetry, "cluster.c_ptt.upstream_rq_retry"},
		{statRetryLimitExc, "cluster.c_ptt.upstream_rq_retry_limit_exceeded"},
		{statRetrySuccess, "cluster.c_ptt.upstream_rq_retry_success"},
		{statRqTotal, "cluster.c_ptt.upstream_rq_total"},
	} {
		if tc.got != tc.want {
			t.Errorf("stat key = %q, want %q", tc.got, tc.want)
		}
	}
}

// TestConfigBuilders pins that both bootstrap builders emit parseable, coherent
// YAML: the router http_filter (a missing router filter makes requests hang),
// the per_try_timeout + num_retries retry_policy, and the c_ptt cluster.
func TestConfigBuilders(t *testing.T) {
	d := &pttDriver{}
	const backendPort = 40076
	ref := d.ReferenceBootstrap([]int{backendPort})
	subj := d.SubjectConfig(0, 18076, []int{backendPort}, 9076)

	for name, cfg := range map[string]string{"reference": ref, "subject": subj} {
		for _, must := range []string{
			"envoy.filters.http.router", // the router filter (else requests hang)
			"per_try_timeout",
			perTryTimeout,
			clusterPtt,
			"retry_on",
			"num_retries",
		} {
			if !strings.Contains(cfg, must) {
				t.Errorf("%s config missing %q", name, must)
			}
		}
	}
	// The reference reaches the backend over the bridge (host.docker.internal);
	// the subject over loopback (STATIC 127.0.0.1).
	if !strings.Contains(ref, "host.docker.internal") {
		t.Error("reference config must use host.docker.internal (the bridge shape)")
	}
	if !strings.Contains(subj, "STATIC") {
		t.Error("subject config must use a STATIC cluster")
	}
}

// Compile-time interface assertion (mirrors the live registration).
var _ fixture.StatsAsserter = (*pttDriver)(nil)
