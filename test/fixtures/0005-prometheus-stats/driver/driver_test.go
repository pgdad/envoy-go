package driver

import (
	"fmt"
	"strings"
	"testing"
)

// -- scrapeAndParse unit tests (against canned Prometheus exposition strings) --

const cannedExposition = `# HELP envoy_cluster_membership_total Number of endpoints in the cluster.
# TYPE envoy_cluster_membership_total gauge
envoy_cluster_membership_total{envoy_cluster_name="c0"} 1
# HELP envoy_cluster_upstream_cx_active Active connections to upstream clusters.
# TYPE envoy_cluster_upstream_cx_active gauge
envoy_cluster_upstream_cx_active{envoy_cluster_name="c0"} 0
# HELP envoy_cluster_upstream_cx_total Total connections established to upstream clusters.
# TYPE envoy_cluster_upstream_cx_total counter
envoy_cluster_upstream_cx_total{envoy_cluster_name="c0"} 3
# HELP envoy_cluster_upstream_rq_total Total requests dispatched to upstream clusters.
# TYPE envoy_cluster_upstream_rq_total counter
envoy_cluster_upstream_rq_total{envoy_cluster_name="c0"} 4
# HELP envoy_cluster_upstream_rq_xx Requests dispatched to upstream clusters, by response code class.
# TYPE envoy_cluster_upstream_rq_xx counter
envoy_cluster_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="c0"} 3
envoy_cluster_upstream_rq_xx{envoy_response_code_class="3",envoy_cluster_name="c0"} 0
envoy_cluster_upstream_rq_xx{envoy_response_code_class="4",envoy_cluster_name="c0"} 0
envoy_cluster_upstream_rq_xx{envoy_response_code_class="5",envoy_cluster_name="c0"} 1
# HELP envoy_http_downstream_rq_total Total requests received by the HTTP connection manager.
# TYPE envoy_http_downstream_rq_total counter
envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="ingress_http"} 5
# HELP envoy_http_downstream_rq_xx Requests received by the HTTP connection manager, by response code class.
# TYPE envoy_http_downstream_rq_xx counter
envoy_http_downstream_rq_xx{envoy_response_code_class="2",envoy_http_conn_manager_prefix="ingress_http"} 3
envoy_http_downstream_rq_xx{envoy_response_code_class="3",envoy_http_conn_manager_prefix="ingress_http"} 0
envoy_http_downstream_rq_xx{envoy_response_code_class="4",envoy_http_conn_manager_prefix="ingress_http"} 1
envoy_http_downstream_rq_xx{envoy_response_code_class="5",envoy_http_conn_manager_prefix="ingress_http"} 1
# HELP envoy_listener_downstream_cx_active Active connections on the listener.
# TYPE envoy_listener_downstream_cx_active gauge
envoy_listener_downstream_cx_active{envoy_listener_address="127_0_0_1_12345"} 0
# HELP envoy_listener_downstream_cx_total Total connections accepted on the listener.
# TYPE envoy_listener_downstream_cx_total counter
envoy_listener_downstream_cx_total{envoy_listener_address="127_0_0_1_12345"} 2
# HELP envoy_server_live 1 if the server is live, 0 otherwise.
# TYPE envoy_server_live gauge
envoy_server_live 1
`

func TestScrapeAndParse_HappyPath(t *testing.T) {
	s, err := parsePromSnapshot(strings.NewReader(cannedExposition))
	if err != nil {
		t.Fatalf("parsePromSnapshot: %v", err)
	}

	checks := []struct {
		name string
		got  interface{}
		want interface{}
	}{
		{"HCMRqTotal", s.HCMRqTotal, uint64(5)},
		{"HCMRq2xx", s.HCMRq2xx, uint64(3)},
		{"HCMRq3xx", s.HCMRq3xx, uint64(0)},
		{"HCMRq4xx", s.HCMRq4xx, uint64(1)},
		{"HCMRq5xx", s.HCMRq5xx, uint64(1)},
		{"ClusterRqTotal", s.ClusterRqTotal, uint64(4)},
		{"ClusterRq2xx", s.ClusterRq2xx, uint64(3)},
		{"ClusterRq3xx", s.ClusterRq3xx, uint64(0)},
		{"ClusterRq4xx", s.ClusterRq4xx, uint64(0)},
		{"ClusterRq5xx", s.ClusterRq5xx, uint64(1)},
		{"ClusterCxTotal", s.ClusterCxTotal, uint64(3)},
		{"ClusterCxActive", s.ClusterCxActive, int64(0)},
		{"ClusterMembership", s.ClusterMembership, int64(1)},
		{"ListenerCxTotal", s.ListenerCxTotal, uint64(2)},
		{"ListenerCxActive", s.ListenerCxActive, int64(0)},
		{"ServerLive", s.ServerLive, int64(1)},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestScrapeAndParse_IgnoresUnknownNames(t *testing.T) {
	input := `# HELP envoy_cluster_external_upstream_rq_xx Twin metric not in allow-list.
# TYPE envoy_cluster_external_upstream_rq_xx counter
envoy_cluster_external_upstream_rq_xx{envoy_response_code_class="2",envoy_cluster_name="c0"} 99
envoy_http_downstream_rq_total{envoy_http_conn_manager_prefix="ingress_http"} 7
`
	s, err := parsePromSnapshot(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parsePromSnapshot: %v", err)
	}
	// ClusterRq2xx must remain 0 — the twin metric is NOT in the allow-list.
	if s.ClusterRq2xx != 0 {
		t.Errorf("ClusterRq2xx: got %d, want 0 (twin metric must be ignored)", s.ClusterRq2xx)
	}
	// HCMRqTotal must be populated from the ingress_http line.
	if s.HCMRqTotal != 7 {
		t.Errorf("HCMRqTotal: got %d, want 7", s.HCMRqTotal)
	}
}

func TestScrapeAndParse_WrongClusterName(t *testing.T) {
	input := `envoy_cluster_upstream_rq_total{envoy_cluster_name="other"} 42
envoy_cluster_upstream_rq_total{envoy_cluster_name="c0"} 10
`
	s, err := parsePromSnapshot(strings.NewReader(input))
	if err != nil {
		t.Fatalf("parsePromSnapshot: %v", err)
	}
	if s.ClusterRqTotal != 10 {
		t.Errorf("ClusterRqTotal: got %d, want 10 (only c0 label is allowed)", s.ClusterRqTotal)
	}
}

func TestScrapeAndParse_EmptyBody(t *testing.T) {
	s, err := parsePromSnapshot(strings.NewReader(""))
	if err != nil {
		t.Fatalf("parsePromSnapshot on empty body: %v", err)
	}
	// All values must be zero.
	if s.HCMRqTotal != 0 || s.ServerLive != 0 || s.ClusterMembership != 0 {
		t.Errorf("expected all-zero snapshot on empty body, got %+v", s)
	}
}

// -- AssertStatsEquivalence unit tests --

// mockT is a testing.T substitute that collects Errorf/Fatalf calls.
type mockT struct {
	errors []string
	fatals []string
}

func (m *mockT) Errorf(format string, args ...any) {
	m.errors = append(m.errors, fmt.Sprintf(format, args...))
}
func (m *mockT) Fatalf(format string, args ...any) {
	m.fatals = append(m.fatals, fmt.Sprintf(format, args...))
}
func (m *mockT) Helper() {}

func (m *mockT) failed() bool { return len(m.errors) > 0 || len(m.fatals) > 0 }

// makeAfterSnapshot returns a Snapshot as if zeroed before and incremented by
// the expected 5-request deltas. The before snapshot is the zero Snapshot{}.
func makeAfterSnapshot(hcmRqTotal, hcm2xx, hcm4xx, hcm5xx, clRqTotal, cl2xx, cl5xx, clCxTotal, listCxTotal uint64, serverLive, clMembership int64) Snapshot {
	return Snapshot{
		HCMRqTotal:        hcmRqTotal,
		HCMRq2xx:          hcm2xx,
		HCMRq4xx:          hcm4xx,
		HCMRq5xx:          hcm5xx,
		ClusterRqTotal:    clRqTotal,
		ClusterRq2xx:      cl2xx,
		ClusterRq5xx:      cl5xx,
		ClusterCxTotal:    clCxTotal,
		ListenerCxTotal:   listCxTotal,
		ClusterMembership: clMembership,
		ServerLive:        serverLive,
	}
}

func TestAssertStatsEquivalence_Pass(t *testing.T) {
	zero := Snapshot{}
	after := makeAfterSnapshot(5, 3, 1, 1, 4, 3, 1, 2, 1, 1, 1)

	mt := &mockT{}
	AssertStatsEquivalence(mt, zero, after, zero, after)
	if mt.failed() {
		t.Errorf("expected PASS, got errors: %v / fatals: %v", mt.errors, mt.fatals)
	}
}

func TestAssertStatsEquivalence_CounterMismatch(t *testing.T) {
	zero := Snapshot{}
	// Reference: 5 total downstream; subject: 4 (mismatch on downstream_rq_total).
	refAfter := makeAfterSnapshot(5, 3, 1, 1, 4, 3, 1, 1, 1, 1, 1)
	subjAfter := makeAfterSnapshot(4, 3, 1, 1, 4, 3, 1, 1, 1, 1, 1)

	mt := &mockT{}
	AssertStatsEquivalence(mt, zero, refAfter, zero, subjAfter)
	if !mt.failed() {
		t.Error("expected FAIL for counter mismatch, got PASS")
	}
	found := false
	for _, msg := range mt.errors {
		if strings.Contains(msg, "downstream_rq_total") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("error message does not name the diverging metric; errors=%v", mt.errors)
	}
}

func TestAssertStatsEquivalence_GaugeMismatch(t *testing.T) {
	zero := Snapshot{}
	refAfter := makeAfterSnapshot(5, 3, 1, 1, 4, 3, 1, 1, 1, 1, 1) // server.live=1
	// Subject server.live is 0 instead of 1.
	subjAfter := makeAfterSnapshot(5, 3, 1, 1, 4, 3, 1, 1, 1, 0, 1)

	mt := &mockT{}
	AssertStatsEquivalence(mt, zero, refAfter, zero, subjAfter)
	if !mt.failed() {
		t.Error("expected FAIL for gauge mismatch, got PASS")
	}
	found := false
	for _, msg := range mt.errors {
		if strings.Contains(msg, "server.live") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("error message does not name the diverging gauge; errors=%v", mt.errors)
	}
}

func TestAssertStatsEquivalence_DeltaMinViolation(t *testing.T) {
	zero := Snapshot{}
	// cx_total delta is 0 on both sides (below delta_min = 1).
	refAfter := makeAfterSnapshot(5, 3, 1, 1, 4, 3, 1, 0, 0, 1, 1)  // ClusterCxTotal=0, ListenerCxTotal=0
	subjAfter := makeAfterSnapshot(5, 3, 1, 1, 4, 3, 1, 0, 0, 1, 1) // same

	mt := &mockT{}
	AssertStatsEquivalence(mt, zero, refAfter, zero, subjAfter)
	if !mt.failed() {
		t.Error("expected FAIL for delta_min violation, got PASS")
	}
	found := false
	for _, msg := range mt.errors {
		if strings.Contains(msg, "cx_total") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("error message does not mention cx_total; errors=%v", mt.errors)
	}
}
