package cluster

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/stats"
)

// registeredNames returns the set of metric names currently in the registry.
// The stats package exposes no by-name lookup, but Walk yields every metric in
// registration order (the same path /stats/prometheus uses), so we scrape names
// through it. This is the cleanest existing introspection affordance.
func registeredNames(r *stats.Registry) map[string]bool {
	names := make(map[string]bool)
	r.Walk(func(m stats.Metric) { names[m.Name()] = true })
	return names
}

// mkHTTPHealthCheck returns a valid HTTP health_check with the given timing.
func mkHTTPHealthCheck(path string, interval, timeout time.Duration, unhealthy, healthy uint32) *corev3.HealthCheck {
	hc := baseHC()
	hc.Interval = durationpb.New(interval)
	hc.Timeout = durationpb.New(timeout)
	hc.UnhealthyThreshold = wrapperspb.UInt32(unhealthy)
	hc.HealthyThreshold = wrapperspb.UInt32(healthy)
	return withHTTP(hc, path)
}

func TestRegisterClusterMetrics_HealthStats(t *testing.T) {
	withHC := mkStaticCluster("hc_cluster", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002))
	withHC.HealthChecks = []*corev3.HealthCheck{mkHTTPHealthCheck("/healthz", time.Second, time.Second, 2, 2)}
	noHC := mkStaticCluster("plain_cluster", mkLbEndpoint("127.0.0.1", 9003))

	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(withHC, noHC), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	names := registeredNames(reg)

	want := []string{
		"cluster.hc_cluster.membership_healthy",
		"cluster.hc_cluster.lb_healthy_panic",
		"cluster.hc_cluster.health_check.attempt",
		"cluster.hc_cluster.health_check.success",
		"cluster.hc_cluster.health_check.failure",
		"cluster.hc_cluster.health_check.network_failure",
		"cluster.hc_cluster.health_check.healthy",
	}
	for _, n := range want {
		if !names[n] {
			t.Errorf("expected health stat %q to be registered, but it was not", n)
		}
	}

	// The plain cluster (no health_checks) must NOT have any of the +7 names.
	notWant := []string{
		"cluster.plain_cluster.membership_healthy",
		"cluster.plain_cluster.lb_healthy_panic",
		"cluster.plain_cluster.health_check.attempt",
		"cluster.plain_cluster.health_check.success",
		"cluster.plain_cluster.health_check.failure",
		"cluster.plain_cluster.health_check.network_failure",
		"cluster.plain_cluster.health_check.healthy",
	}
	for _, n := range notWant {
		if names[n] {
			t.Errorf("plain cluster (no health_checks) must NOT register %q", n)
		}
	}

	// membership_healthy starts at the full endpoint count (all hosts healthy).
	cl, ok := m.Get("hc_cluster")
	if !ok {
		t.Fatal("hc_cluster not found in manager")
	}
	if got := cl.health.membershipHealthy.Load(); got != 2 {
		t.Errorf("membership_healthy initial = %d, want 2 (all hosts start healthy)", got)
	}
}

func TestStartHealthChecks_Lifecycle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	host, portStr, err := net.SplitHostPort(srv.Listener.Addr().String())
	if err != nil {
		t.Fatalf("split httptest addr: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse httptest port: %v", err)
	}

	// One live endpoint (the httptest server) + one dead endpoint (127.0.0.1:1).
	cl := mkStaticCluster("hc_cluster",
		mkLbEndpoint(host, uint32(port)),
		mkLbEndpoint("127.0.0.1", 1),
	)
	cl.HealthChecks = []*corev3.HealthCheck{
		mkHTTPHealthCheck("/healthz", 50*time.Millisecond, 200*time.Millisecond, 1, 1),
	}

	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(cl), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	reg.Freeze()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.StartHealthChecks(ctx)

	got, ok := m.Get("hc_cluster")
	if !ok {
		t.Fatal("hc_cluster not found")
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if got.health.healthyCount(got.endpoints) == 1 {
			break // the dead host has been marked unhealthy; the live one stays healthy
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for healthyCount==1; got %d", got.health.healthyCount(got.endpoints))
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Drain must cancel the runtime and join the goroutines (no leak). If the
	// WaitGroup never drains, this blocks and the test times out under `go test`.
	done := make(chan struct{})
	go func() {
		m.Drain()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Drain did not return within 5s (health-check goroutine leak)")
	}
}

// TestStartOutlierDetection_Lifecycle: start the per-cluster sweep goroutine on a
// Manager with one outlier cluster, cancel the ctx, and assert Drain joins cleanly
// (no goroutine leak) and is idempotent (a second Drain returns immediately).
func TestStartOutlierDetection_Lifecycle(t *testing.T) {
	cl := mkStaticCluster("od_cluster",
		mkLbEndpoint("127.0.0.1", 9001),
		mkLbEndpoint("127.0.0.1", 9002),
	)
	cl.OutlierDetection = &clusterv3.OutlierDetection{
		Interval:           durationpb.New(20 * time.Millisecond),
		MaxEjectionPercent: wrapperspb.UInt32(100),
	}

	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(cl), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	reg.Freeze()

	ctx, cancel := context.WithCancel(context.Background())
	m.StartOutlierDetection(ctx)

	got, ok := m.Get("od_cluster")
	if !ok {
		t.Fatal("od_cluster not found")
	}
	// Record some traffic so the sweep has data to snapshot (no eject expected here
	// — the eligibility gates are unmet with 2 hosts and the default minHosts 5).
	for i := 0; i < 50; i++ {
		got.RecordUpstreamResult(got.endpoints[0], UpstreamResult{StatusCode: 200})
	}
	time.Sleep(60 * time.Millisecond) // let the ticker fire a few sweeps

	// Cancel the ctx then Drain — the join must return (no leak).
	cancel()
	done := make(chan struct{})
	go func() {
		m.Drain()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Drain did not return within 5s (outlier-detection goroutine leak)")
	}

	// Idempotent: a second Drain returns immediately.
	done2 := make(chan struct{})
	go func() {
		m.Drain()
		close(done2)
	}()
	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("second Drain did not return within 5s (not idempotent)")
	}
}

// TestStartOutlierDetection_SweepDrivenEject: with a SHORT interval, start the
// sweep goroutine over a 6-host cluster (5 all-success + 1 all-failure), record
// the outlier traffic, and poll ejections_active until it reads 1 — proving the
// ticker fires AND the sweep ejects. Then cancel + Drain.
func TestStartOutlierDetection_SweepDrivenEject(t *testing.T) {
	eps := make([]*endpointv3.LbEndpoint, 6)
	for i := 0; i < 6; i++ {
		eps[i] = mkLbEndpoint("127.0.0.1", uint32(9100+i))
	}
	cl := mkStaticClusterFromLbEndpoints("od_eject_sweep", eps...)
	cl.OutlierDetection = &clusterv3.OutlierDetection{
		Interval:                 durationpb.New(20 * time.Millisecond),
		MaxEjectionPercent:       wrapperspb.UInt32(100),
		SuccessRateMinimumHosts:  wrapperspb.UInt32(5),
		SuccessRateRequestVolume: wrapperspb.UInt32(10),
		EnforcingSuccessRate:     wrapperspb.UInt32(100),
	}

	reg := stats.NewRegistry()
	m, err := NewManager(mkBootstrap(cl), reg)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	reg.Freeze()

	ctx, cancel := context.WithCancel(context.Background())
	m.StartOutlierDetection(ctx)

	got, ok := m.Get("od_eject_sweep")
	if !ok {
		t.Fatal("od_eject_sweep not found")
	}
	// 5 hosts all-success, 1 host all-failure (each over request_volume 10).
	for i := 0; i < 5; i++ {
		for j := 0; j < 50; j++ {
			got.RecordUpstreamResult(got.endpoints[i], UpstreamResult{StatusCode: 200})
		}
	}
	for j := 0; j < 50; j++ {
		got.RecordUpstreamResult(got.endpoints[5], UpstreamResult{StatusCode: 503})
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		if v, found := gaugeValue(reg, "cluster.od_eject_sweep.outlier_detection.ejections_active"); found && v == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the sweep to eject the outlier (ejections_active==1)")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got.health.available(got.endpoints[5]) {
		t.Fatal("the all-failure host should be ejected by the sweep")
	}

	cancel()
	done := make(chan struct{})
	go func() {
		m.Drain()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Drain did not return within 5s (outlier-detection goroutine leak)")
	}
}
