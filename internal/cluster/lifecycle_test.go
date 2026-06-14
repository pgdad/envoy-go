package cluster

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
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
