package cluster

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/esalaine/envoy-go/internal/stats"
)

func TestHostHealth_Transitions(t *testing.T) {
	h := newHostHealth() // starts HEALTHY
	if !h.healthy.Load() {
		t.Fatal("want initial healthy")
	}
	h.recordResult(false, 2, 2, true) // first check: one failure -> unhealthy immediately
	if h.healthy.Load() {
		t.Fatal("first failure should mark unhealthy immediately")
	}
	h.recordResult(true, 2, 2, false) // recovery needs healthyThreshold(2) successes
	if h.healthy.Load() {
		t.Fatal("one success < healthyThreshold should stay unhealthy")
	}
	h.recordResult(true, 2, 2, false)
	if !h.healthy.Load() {
		t.Fatal("two successes == healthyThreshold should become healthy")
	}
	h.recordResult(false, 2, 2, false) // going down needs unhealthyThreshold(2) failures
	if !h.healthy.Load() {
		t.Fatal("one failure < unhealthyThreshold should stay healthy")
	}
	h.recordResult(false, 2, 2, false)
	if h.healthy.Load() {
		t.Fatal("two failures == unhealthyThreshold should become unhealthy")
	}
}

func TestClusterHealth_View(t *testing.T) {
	eps := []Endpoint{{Host: "10.0.0.1", Port: 80}, {Host: "10.0.0.2", Port: 80}, {Host: "10.0.0.3", Port: 80}}
	ch := newClusterHealth(eps, 0.5)
	if ch.healthyCount(eps) != 3 || ch.inPanic(eps) {
		t.Fatal("all healthy: count 3, no panic")
	}
	ch.states["10.0.0.3:80"].healthy.Store(false) // 2/3 = 66% > 50%
	if ch.healthyCount(eps) != 2 || ch.inPanic(eps) {
		t.Fatal("2/3 healthy (66%): no panic")
	}
	ch.states["10.0.0.2:80"].healthy.Store(false) // 1/3 = 33% < 50% -> panic
	if !ch.inPanic(eps) {
		t.Fatal("1/3 healthy (33%): panic")
	}
	if ch.isHealthy(eps[2]) {
		t.Fatal("ep3 should be unhealthy")
	}
}

func TestClusterHealth_StatHandles(t *testing.T) {
	eps := []Endpoint{{Host: "10.0.0.1", Port: 80}, {Host: "10.0.0.2", Port: 80}}
	ch := newClusterHealth(eps, 0.5)

	// nil-guarded: no panic before the stat handles are injected.
	ch.recomputeMembership(eps)
	ch.panicInc()

	reg := stats.NewRegistry()
	ch.membershipHealthy = reg.NewGauge("membership_healthy")
	ch.panicCounter = reg.NewCounter("lb_healthy_panic")

	ch.recomputeMembership(eps) // both healthy
	if got := ch.membershipHealthy.Load(); got != 2 {
		t.Fatalf("membership_healthy = %d, want 2", got)
	}
	ch.states["10.0.0.2:80"].healthy.Store(false)
	ch.recomputeMembership(eps)
	if got := ch.membershipHealthy.Load(); got != 1 {
		t.Fatalf("membership_healthy = %d, want 1", got)
	}

	ch.panicInc()
	if got := ch.panicCounter.Load(); got != 1 {
		t.Fatalf("lb_healthy_panic = %d, want 1", got)
	}
}

func TestProbeHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(200)
		} else {
			w.WriteHeader(503)
		}
	}))
	defer srv.Close()
	cfg := httpHealthCheckCfg{path: "/healthz", timeout: time.Second, expectedStatuses: []statusRange{{200, 201}}}
	addr := srv.Listener.Addr().String()
	if ok, netErr := probeHTTP(addr, cfg); !ok || netErr {
		t.Fatalf("live 200 should succeed: ok=%v netErr=%v", ok, netErr)
	}
	cfg.path = "/x"
	if ok, netErr := probeHTTP(addr, cfg); ok || netErr {
		t.Fatalf("503 should fail non-network: ok=%v netErr=%v", ok, netErr)
	}
	if ok, netErr := probeHTTP("127.0.0.1:1", cfg); ok || !netErr {
		t.Fatalf("dead addr should be a network failure: ok=%v netErr=%v", ok, netErr)
	}
}

func TestHTTPHealthCheckCfg_StatusOK(t *testing.T) {
	// empty expectedStatuses -> default [200,201): 200 in, 201/204 out.
	def := httpHealthCheckCfg{interval: time.Second, timeout: time.Second, unhealthy: 2, healthy: 2}
	if !def.statusOK(200) {
		t.Fatal("default range should accept 200")
	}
	if def.statusOK(201) || def.statusOK(204) {
		t.Fatal("default range is [200,201): 201/204 must be rejected")
	}
	// explicit multi-range + the config fields carry through.
	cfg := httpHealthCheckCfg{
		interval:         500 * time.Millisecond,
		timeout:          time.Second,
		unhealthy:        3,
		healthy:          1,
		expectedStatuses: []statusRange{{200, 300}, {404, 405}},
	}
	if !cfg.statusOK(204) || !cfg.statusOK(404) {
		t.Fatal("204 and 404 should match the explicit ranges")
	}
	if cfg.statusOK(300) || cfg.statusOK(403) {
		t.Fatal("300 and 403 fall outside the explicit ranges")
	}
	if cfg.interval != 500*time.Millisecond || cfg.unhealthy != 3 || cfg.healthy != 1 {
		t.Fatalf("config fields desynced: interval=%v unhealthy=%d healthy=%d", cfg.interval, cfg.unhealthy, cfg.healthy)
	}
}

// addrEndpoint splits a "host:port" string into an Endpoint (test helper).
func addrEndpoint(hostport string) Endpoint {
	h, p, _ := net.SplitHostPort(hostport)
	n, _ := strconv.Atoi(p)
	return Endpoint{Host: h, Port: uint32(n)}
}

func TestHealthChecker_ProbeOnce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	eps := []Endpoint{addrEndpoint(srv.Listener.Addr().String()), {Host: "127.0.0.1", Port: 1}}
	ch := newClusterHealth(eps, 0.5)
	reg := stats.NewRegistry()
	hc := newHealthChecker(eps, ch, httpHealthCheckCfg{path: "/", timeout: time.Second, unhealthy: 1, healthy: 1})
	hc.registerStats(reg, "cluster.c.")
	hc.probeOnce()
	if !ch.isHealthy(eps[0]) {
		t.Fatal("live host should be healthy after first probe")
	}
	if ch.isHealthy(eps[1]) {
		t.Fatal("dead host should be unhealthy after first probe (first-check-immediate)")
	}
	if hc.attempt.Load() != 2 || hc.success.Load() != 1 || hc.failure.Load() != 1 || hc.networkFailure.Load() != 1 {
		t.Fatalf("stats: attempt=%d success=%d failure=%d net=%d", hc.attempt.Load(), hc.success.Load(), hc.failure.Load(), hc.networkFailure.Load())
	}
}

// baseHC returns a HealthCheck with all required scalars set but NO checker oneof.
func baseHC() *corev3.HealthCheck {
	return &corev3.HealthCheck{
		Interval:           durationpb.New(time.Second),
		Timeout:            durationpb.New(time.Second),
		UnhealthyThreshold: wrapperspb.UInt32(2),
		HealthyThreshold:   wrapperspb.UInt32(2),
	}
}

func withHTTP(hc *corev3.HealthCheck, path string) *corev3.HealthCheck {
	hc.HealthChecker = &corev3.HealthCheck_HttpHealthCheck_{
		HttpHealthCheck: &corev3.HealthCheck_HttpHealthCheck{Path: path},
	}
	return hc
}

func TestParseHealthChecks(t *testing.T) {
	tcpHC := baseHC()
	tcpHC.HealthChecker = &corev3.HealthCheck_TcpHealthCheck_{TcpHealthCheck: &corev3.HealthCheck_TcpHealthCheck{}}
	grpcHC := baseHC()
	grpcHC.HealthChecker = &corev3.HealthCheck_GrpcHealthCheck_{GrpcHealthCheck: &corev3.HealthCheck_GrpcHealthCheck{}}

	noInterval := withHTTP(baseHC(), "/h")
	noInterval.Interval = nil
	noTimeout := withHTTP(baseHC(), "/h")
	noTimeout.Timeout = nil
	noUnhealthy := withHTTP(baseHC(), "/h")
	noUnhealthy.UnhealthyThreshold = nil
	noHealthy := withHTTP(baseHC(), "/h")
	noHealthy.HealthyThreshold = nil

	tests := []struct {
		name    string
		hc      *corev3.HealthCheck
		wantErr string
	}{
		{"no_checker", baseHC(), "health_check: a health_checker is required"},
		{"empty_path", withHTTP(baseHC(), ""), "health_check: http path is required"},
		{"no_interval", noInterval, "health_check: interval is required"},
		{"no_timeout", noTimeout, "health_check: timeout is required"},
		{"no_unhealthy_threshold", noUnhealthy, "health_check: unhealthy_threshold is required"},
		{"no_healthy_threshold", noHealthy, "health_check: healthy_threshold is required"},
		{"tcp_unsupported", tcpHC, "health_check: only http_health_check is supported"},
		{"grpc_unsupported", grpcHC, "health_check: only http_health_check is supported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clusterv3.Cluster{HealthChecks: []*corev3.HealthCheck{tt.hc}}
			_, err := parseHealthChecks(c, "c0")
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error %q does not contain %q", err.Error(), tt.wantErr)
			}
		})
	}

	t.Run("valid", func(t *testing.T) {
		hc := baseHC()
		hc.HealthChecker = &corev3.HealthCheck_HttpHealthCheck_{
			HttpHealthCheck: &corev3.HealthCheck_HttpHealthCheck{
				Path:             "/h",
				ExpectedStatuses: []*typev3.Int64Range{{Start: 200, End: 201}},
			},
		}
		c := &clusterv3.Cluster{HealthChecks: []*corev3.HealthCheck{hc}}
		out, err := parseHealthChecks(c, "c0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("want 1 cfg, got %d", len(out))
		}
		got := out[0]
		if got.path != "/h" {
			t.Fatalf("path = %q, want /h", got.path)
		}
		if got.interval != time.Second || got.timeout != time.Second {
			t.Fatalf("interval=%v timeout=%v, want 1s/1s", got.interval, got.timeout)
		}
		if got.unhealthy != 2 || got.healthy != 2 {
			t.Fatalf("thresholds unhealthy=%d healthy=%d, want 2/2", got.unhealthy, got.healthy)
		}
		if len(got.expectedStatuses) != 1 || got.expectedStatuses[0] != (statusRange{200, 201}) {
			t.Fatalf("expectedStatuses = %+v, want [{200 201}]", got.expectedStatuses)
		}
	})

	t.Run("no_health_checks", func(t *testing.T) {
		out, err := parseHealthChecks(&clusterv3.Cluster{}, "c0")
		if err != nil || out != nil {
			t.Fatalf("empty cluster: out=%+v err=%v, want nil/nil", out, err)
		}
	})
}

func TestParsePanicThreshold(t *testing.T) {
	if got := parsePanicThreshold(&clusterv3.Cluster{}); got != 0.5 {
		t.Fatalf("nil common_lb_config: got %v, want 0.5", got)
	}
	c := &clusterv3.Cluster{
		CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
			HealthyPanicThreshold: &typev3.Percent{Value: 70},
		},
	}
	if got := parsePanicThreshold(c); got != 0.7 {
		t.Fatalf("Percent{70}: got %v, want 0.7", got)
	}
}
