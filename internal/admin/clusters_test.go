package admin

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestHandleClusters_HTTPSmoke200Text asserts the handler returns 200 with
// the SPEC §11.6-pinned text/plain Content-Type and a non-empty body. Per
// SPEC §11.2 + ADR-0087.
func TestHandleClusters_HTTPSmoke200Text(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	lm := mustMinimalLM(t, bs, cm)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, lm, nil)
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/clusters")
	if err != nil {
		t.Fatalf("GET /clusters: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "text/plain; charset=UTF-8" {
		t.Errorf("Content-Type: got %q, want %q", ct, "text/plain; charset=UTF-8")
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) == 0 {
		t.Fatal("empty body")
	}
}

// TestHandleClusters_TenClusterLevelLinesPerCluster asserts the §7.3 fixture
// (one cluster c_backend with 2 endpoints) yields exactly 10 + 2*18 = 46
// lines per SPEC §11.2.
func TestHandleClusters_TenClusterLevelLinesPerCluster(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil, nil)
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/clusters")
	if err != nil {
		t.Fatalf("GET /clusters: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	lines := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	if got := len(lines); got != 46 {
		t.Errorf("/clusters total lines: got %d, want 46 (10 cluster-level + 2*18 per-endpoint); body:\n%s", got, body)
	}
}

// TestHandleClusters_ClusterLevelLineFormat asserts the 10 cluster-level
// lines are emitted in the exact order pinned by SPEC §11.2(b). Constants
// 1024 and 3 (envoy.config.cluster.v3.CircuitBreakers.Thresholds defaults)
// + added_via_api::false are emitted unconditionally per ADR-0087.
func TestHandleClusters_ClusterLevelLineFormat(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil, nil)
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/clusters")
	if err != nil {
		t.Fatalf("GET /clusters: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	wantLines := []string{
		"c_backend::observability_name::c_backend",
		"c_backend::default_priority::max_connections::1024",
		"c_backend::default_priority::max_pending_requests::1024",
		"c_backend::default_priority::max_requests::1024",
		"c_backend::default_priority::max_retries::3",
		"c_backend::high_priority::max_connections::1024",
		"c_backend::high_priority::max_pending_requests::1024",
		"c_backend::high_priority::max_requests::1024",
		"c_backend::high_priority::max_retries::3",
		"c_backend::added_via_api::false",
	}
	for _, want := range wantLines {
		if !strings.Contains(bodyStr, want+"\n") {
			t.Errorf("/clusters body missing line %q", want)
		}
	}
}

// TestHandleClusters_PerEndpointLinesAllZeroPlusConstants asserts:
//   - all 8 cx_*/rq_* counter lines emit literal `0` per planner-time
//     decision 8 + ADR-0063 per-endpoint stats deferral
//   - the 10 constant lines are emitted with the §11.2(c)-pinned values
//     (hostname empty; health_flags::healthy; weight::1; region/zone/sub_zone
//     empty; canary::false; priority::0; success_rate::-1;
//     local_origin_success_rate::-1)
func TestHandleClusters_PerEndpointLinesAllZeroPlusConstants(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil, nil)
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/clusters")
	if err != nil {
		t.Fatalf("GET /clusters: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	for _, ep := range []string{"127.0.0.1:18001", "127.0.0.1:18002"} {
		for _, key := range []string{"cx_active", "cx_connect_fail", "cx_total", "rq_active", "rq_error", "rq_success", "rq_timeout", "rq_total"} {
			want := "c_backend::" + ep + "::" + key + "::0\n"
			if !strings.Contains(bodyStr, want) {
				t.Errorf("/clusters body missing per-endpoint zero line %q", want)
			}
		}
		for _, lit := range []string{
			"hostname::",
			"health_flags::healthy",
			"weight::1",
			"region::",
			"zone::",
			"sub_zone::",
			"canary::false",
			"priority::0",
			"success_rate::-1",
			"local_origin_success_rate::-1",
		} {
			want := "c_backend::" + ep + "::" + lit + "\n"
			if !strings.Contains(bodyStr, want) {
				t.Errorf("/clusters body missing per-endpoint constant line %q", want)
			}
		}
	}
}

// TestHandleClusters_BodyExactByteLayout asserts the full §7.3-fixture body
// is byte-equal to the SPEC §11.2 verbatim line set (with cx_/rq_ counters
// emitting `0` per planner-time decision 8). This is the differential
// comparator's primary pre-check before tolerance application — byte-equality
// modulo the cx_/rq_ tolerance per ADR-0087.
func TestHandleClusters_BodyExactByteLayout(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil, nil)
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/clusters")
	if err != nil {
		t.Fatalf("GET /clusters: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)

	want := `c_backend::observability_name::c_backend
c_backend::default_priority::max_connections::1024
c_backend::default_priority::max_pending_requests::1024
c_backend::default_priority::max_requests::1024
c_backend::default_priority::max_retries::3
c_backend::high_priority::max_connections::1024
c_backend::high_priority::max_pending_requests::1024
c_backend::high_priority::max_requests::1024
c_backend::high_priority::max_retries::3
c_backend::added_via_api::false
c_backend::127.0.0.1:18001::cx_active::0
c_backend::127.0.0.1:18001::cx_connect_fail::0
c_backend::127.0.0.1:18001::cx_total::0
c_backend::127.0.0.1:18001::rq_active::0
c_backend::127.0.0.1:18001::rq_error::0
c_backend::127.0.0.1:18001::rq_success::0
c_backend::127.0.0.1:18001::rq_timeout::0
c_backend::127.0.0.1:18001::rq_total::0
c_backend::127.0.0.1:18001::hostname::
c_backend::127.0.0.1:18001::health_flags::healthy
c_backend::127.0.0.1:18001::weight::1
c_backend::127.0.0.1:18001::region::
c_backend::127.0.0.1:18001::zone::
c_backend::127.0.0.1:18001::sub_zone::
c_backend::127.0.0.1:18001::canary::false
c_backend::127.0.0.1:18001::priority::0
c_backend::127.0.0.1:18001::success_rate::-1
c_backend::127.0.0.1:18001::local_origin_success_rate::-1
c_backend::127.0.0.1:18002::cx_active::0
c_backend::127.0.0.1:18002::cx_connect_fail::0
c_backend::127.0.0.1:18002::cx_total::0
c_backend::127.0.0.1:18002::rq_active::0
c_backend::127.0.0.1:18002::rq_error::0
c_backend::127.0.0.1:18002::rq_success::0
c_backend::127.0.0.1:18002::rq_timeout::0
c_backend::127.0.0.1:18002::rq_total::0
c_backend::127.0.0.1:18002::hostname::
c_backend::127.0.0.1:18002::health_flags::healthy
c_backend::127.0.0.1:18002::weight::1
c_backend::127.0.0.1:18002::region::
c_backend::127.0.0.1:18002::zone::
c_backend::127.0.0.1:18002::sub_zone::
c_backend::127.0.0.1:18002::canary::false
c_backend::127.0.0.1:18002::priority::0
c_backend::127.0.0.1:18002::success_rate::-1
c_backend::127.0.0.1:18002::local_origin_success_rate::-1
`
	if !bytes.Equal(body, []byte(want)) {
		t.Errorf("/clusters body byte-mismatch.\n--- got ---\n%s\n--- want ---\n%s", body, want)
	}
}

// TestHandleClusters_EndpointDeclarationOrderPreserved asserts endpoints are
// emitted in their bootstrap-declared order (NOT alphabetical or address-
// sorted) per SPEC §11.2(c) + §6.2. The §7.3 fixture declares :18001 then
// :18002; the body must surface :18001's block before :18002's block.
func TestHandleClusters_EndpointDeclarationOrderPreserved(t *testing.T) {
	bs := mustMinimalBs(t)
	cm := mustMinimalCM(t, bs)
	s := New("127.0.0.1:0", bs.Stats, bs, cm, nil, nil)
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/clusters")
	if err != nil {
		t.Fatalf("GET /clusters: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	bodyStr := string(body)

	idx18001 := strings.Index(bodyStr, "c_backend::127.0.0.1:18001::cx_active::0")
	idx18002 := strings.Index(bodyStr, "c_backend::127.0.0.1:18002::cx_active::0")
	if idx18001 < 0 || idx18002 < 0 {
		t.Fatalf("/clusters body missing endpoint anchor lines: idx18001=%d, idx18002=%d", idx18001, idx18002)
	}
	if idx18001 >= idx18002 {
		t.Errorf("/clusters body endpoint order reversed: :18001 should precede :18002 (idx18001=%d, idx18002=%d)", idx18001, idx18002)
	}
}

// TestHandleClusters_NilManagerEmitsEmptyBody asserts the defensive nil-cm
// path (test code that constructs admin.New with cm=nil per ADR-0085's
// nil-tolerated test convention) emits a 200 + empty body rather than
// panicking.
func TestHandleClusters_NilManagerEmitsEmptyBody(t *testing.T) {
	bs := mustMinimalBs(t)
	s := New("127.0.0.1:0", bs.Stats, bs, nil, nil, nil)
	addr, err := s.Start()
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = s.Close() }()
	time.Sleep(10 * time.Millisecond)

	resp, err := http.Get("http://" + addr + "/clusters")
	if err != nil {
		t.Fatalf("GET /clusters: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("status: got %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if len(body) != 0 {
		t.Errorf("body: got %q, want empty", body)
	}
}
