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
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/pgdad/envoy-go/internal/stats"
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
	def := httpHealthCheckCfg{timeout: time.Second}
	if !def.statusOK(200) {
		t.Fatal("default range should accept 200")
	}
	if def.statusOK(201) || def.statusOK(204) {
		t.Fatal("default range is [200,201): 201/204 must be rejected")
	}
	// explicit multi-range.
	cfg := httpHealthCheckCfg{
		timeout:          time.Second,
		expectedStatuses: []statusRange{{200, 300}, {404, 405}},
	}
	if !cfg.statusOK(204) || !cfg.statusOK(404) {
		t.Fatal("204 and 404 should match the explicit ranges")
	}
	if cfg.statusOK(300) || cfg.statusOK(403) {
		t.Fatal("300 and 403 fall outside the explicit ranges")
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
	hc := newHealthChecker(eps, ch, checkerSpec{
		interval:  time.Second,
		unhealthy: 1,
		healthy:   1,
		prober:    httpProber{cfg: httpHealthCheckCfg{path: "/", timeout: time.Second}},
	})
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &clusterv3.Cluster{HealthChecks: []*corev3.HealthCheck{tt.hc}}
			_, _, err := parseHealthChecks(c, "c0")
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
		out, _, err := parseHealthChecks(c, "c0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("want 1 cfg, got %d", len(out))
		}
		got := out[0]
		if got.interval != time.Second {
			t.Fatalf("interval=%v, want 1s", got.interval)
		}
		if got.unhealthy != 2 || got.healthy != 2 {
			t.Fatalf("thresholds unhealthy=%d healthy=%d, want 2/2", got.unhealthy, got.healthy)
		}
		hp, ok := got.prober.(httpProber)
		if !ok {
			t.Fatalf("prober = %T, want httpProber", got.prober)
		}
		if hp.cfg.path != "/h" {
			t.Fatalf("path = %q, want /h", hp.cfg.path)
		}
		if hp.cfg.timeout != time.Second {
			t.Fatalf("timeout=%v, want 1s", hp.cfg.timeout)
		}
		if len(hp.cfg.expectedStatuses) != 1 || hp.cfg.expectedStatuses[0] != (statusRange{200, 201}) {
			t.Fatalf("expectedStatuses = %+v, want [{200 201}]", hp.cfg.expectedStatuses)
		}
	})

	t.Run("no_health_checks", func(t *testing.T) {
		out, _, err := parseHealthChecks(&clusterv3.Cluster{}, "c0")
		if err != nil || out != nil {
			t.Fatalf("empty cluster: out=%+v err=%v, want nil/nil", out, err)
		}
	})
}

// TestTcpProber verifies the connect-only TCP codec.
func TestTcpProber(t *testing.T) {
	// Successful connect: a live listener should yield (true, false).
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	p := tcpProber{timeout: time.Second}
	ok, netFail := p.probe(ln.Addr().String())
	if !ok || netFail {
		t.Fatalf("live listener: want ok=true netFail=false, got ok=%v netFail=%v", ok, netFail)
	}

	// Failed connect: closed port should yield (false, true).
	addr := ln.Addr().String()
	_ = ln.Close()
	ok, netFail = p.probe(addr)
	if ok || !netFail {
		t.Fatalf("closed port: want ok=false netFail=true, got ok=%v netFail=%v", ok, netFail)
	}
}

// TestParseHealthChecks_Tcp covers the tcp_health_check parse arm.
func TestParseHealthChecks_Tcp(t *testing.T) {
	// Plain tcp_health_check (no send/receive) should produce a tcpProber spec.
	t.Run("valid_empty", func(t *testing.T) {
		hc := baseHC()
		hc.HealthChecker = &corev3.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &corev3.HealthCheck_TcpHealthCheck{},
		}
		c := &clusterv3.Cluster{HealthChecks: []*corev3.HealthCheck{hc}}
		out, _, err := parseHealthChecks(c, "c0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(out) != 1 {
			t.Fatalf("want 1 spec, got %d", len(out))
		}
		got := out[0]
		if got.interval != time.Second {
			t.Fatalf("interval=%v, want 1s", got.interval)
		}
		if got.unhealthy != 2 || got.healthy != 2 {
			t.Fatalf("thresholds unhealthy=%d healthy=%d, want 2/2", got.unhealthy, got.healthy)
		}
		tp, ok := got.prober.(tcpProber)
		if !ok {
			t.Fatalf("prober = %T, want tcpProber", got.prober)
		}
		if tp.timeout != time.Second {
			t.Fatalf("timeout=%v, want 1s", tp.timeout)
		}
	})

	// tcp_health_check with send set must be rejected.
	t.Run("send_rejected", func(t *testing.T) {
		hc := baseHC()
		hc.HealthChecker = &corev3.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &corev3.HealthCheck_TcpHealthCheck{
				Send: &corev3.HealthCheck_Payload{},
			},
		}
		c := &clusterv3.Cluster{HealthChecks: []*corev3.HealthCheck{hc}}
		_, _, err := parseHealthChecks(c, "c0")
		if err == nil {
			t.Fatal("want error for tcp send, got nil")
		}
		if !strings.Contains(err.Error(), "tcp_health_check send/receive payload matching is not supported") {
			t.Fatalf("error %q does not contain expected wording", err.Error())
		}
	})

	// tcp_health_check with receive set must be rejected.
	t.Run("receive_rejected", func(t *testing.T) {
		hc := baseHC()
		hc.HealthChecker = &corev3.HealthCheck_TcpHealthCheck_{
			TcpHealthCheck: &corev3.HealthCheck_TcpHealthCheck{
				Receive: []*corev3.HealthCheck_Payload{{}},
			},
		}
		c := &clusterv3.Cluster{HealthChecks: []*corev3.HealthCheck{hc}}
		_, _, err := parseHealthChecks(c, "c0")
		if err == nil {
			t.Fatal("want error for tcp receive, got nil")
		}
		if !strings.Contains(err.Error(), "tcp_health_check send/receive payload matching is not supported") {
			t.Fatalf("error %q does not contain expected wording", err.Error())
		}
	})
}

// newGrpcHealthServer starts an in-process gRPC health server on 127.0.0.1:0,
// sets the given service statuses, and returns its addr + a stop func.
func newGrpcHealthServer(t *testing.T, statuses map[string]grpc_health_v1.HealthCheckResponse_ServingStatus) (addr string, stop func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	hs := health.NewServer()
	for svc, st := range statuses {
		hs.SetServingStatus(svc, st)
	}
	grpc_health_v1.RegisterHealthServer(srv, hs)
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), srv.Stop
}

// TestGrpcProber verifies the gRPC health-check codec: SERVING -> (true,false);
// NOT_SERVING -> (false,false) THE DISCRIMINATOR (reachable, app-unhealthy);
// dead port -> (false,true) network failure.
func TestGrpcProber(t *testing.T) {
	// (a) default service SERVING -> (true, false).
	t.Run("serving", func(t *testing.T) {
		addr, stop := newGrpcHealthServer(t, map[string]grpc_health_v1.HealthCheckResponse_ServingStatus{
			"": grpc_health_v1.HealthCheckResponse_SERVING,
		})
		defer stop()
		ok, netFail := grpcProber{serviceName: "", timeout: 2 * time.Second}.probe(addr)
		if !ok || netFail {
			t.Fatalf("SERVING: want ok=true netFail=false, got ok=%v netFail=%v", ok, netFail)
		}
	})

	// (b) NOT_SERVING discriminator: reachable host, application-unhealthy ->
	// (false, false). networkFailure MUST be false (the keystone assertion).
	t.Run("not_serving_discriminator", func(t *testing.T) {
		addr, stop := newGrpcHealthServer(t, map[string]grpc_health_v1.HealthCheckResponse_ServingStatus{
			"svc.Bad": grpc_health_v1.HealthCheckResponse_NOT_SERVING,
		})
		defer stop()
		ok, netFail := grpcProber{serviceName: "svc.Bad", timeout: 2 * time.Second}.probe(addr)
		if ok {
			t.Fatalf("NOT_SERVING: want ok=false, got ok=true")
		}
		if netFail {
			t.Fatal("NOT_SERVING discriminator: networkFailure MUST be false (reachable host, application-unhealthy)")
		}
	})

	// (c) dead port -> (false, true) network failure.
	t.Run("dead_port", func(t *testing.T) {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("listen: %v", err)
		}
		addr := ln.Addr().String()
		_ = ln.Close()
		ok, netFail := grpcProber{serviceName: "", timeout: time.Second}.probe(addr)
		if ok || !netFail {
			t.Fatalf("dead port: want ok=false netFail=true, got ok=%v netFail=%v", ok, netFail)
		}
	})
}

// TestParseHealthChecks_Grpc covers the grpc_health_check parse arm + hasGrpc.
func TestParseHealthChecks_Grpc(t *testing.T) {
	hc := baseHC()
	hc.HealthChecker = &corev3.HealthCheck_GrpcHealthCheck_{
		GrpcHealthCheck: &corev3.HealthCheck_GrpcHealthCheck{ServiceName: "x"},
	}
	c := &clusterv3.Cluster{HealthChecks: []*corev3.HealthCheck{hc}}
	out, hasGrpc, err := parseHealthChecks(c, "c0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasGrpc {
		t.Fatal("want hasGrpc=true for a grpc_health_check")
	}
	if len(out) != 1 {
		t.Fatalf("want 1 spec, got %d", len(out))
	}
	got := out[0]
	if got.interval != time.Second {
		t.Fatalf("interval=%v, want 1s", got.interval)
	}
	if got.unhealthy != 2 || got.healthy != 2 {
		t.Fatalf("thresholds unhealthy=%d healthy=%d, want 2/2", got.unhealthy, got.healthy)
	}
	gp, ok := got.prober.(grpcProber)
	if !ok {
		t.Fatalf("prober = %T, want grpcProber", got.prober)
	}
	if gp.serviceName != "x" {
		t.Fatalf("serviceName = %q, want x", gp.serviceName)
	}
	if gp.timeout != time.Second {
		t.Fatalf("timeout=%v, want 1s", gp.timeout)
	}
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

func TestIsEjected_UnknownAddr(t *testing.T) {
	eps := []Endpoint{{Host: "10.0.0.1", Port: 80}}
	ch := newClusterHealth(eps, 0.5)
	unknown := Endpoint{Host: "10.0.0.9", Port: 80}
	if ch.isEjected(unknown) {
		t.Fatal("unknown addr must not be ejected")
	}
}

func TestEject_ThenAvailableFalse(t *testing.T) {
	eps := []Endpoint{{Host: "10.0.0.1", Port: 80}, {Host: "10.0.0.2", Port: 80}}
	ch := newClusterHealth(eps, 0.5)
	now := int64(1_000_000_000)
	ch.nowNanos = func() int64 { return now }
	h := ch.states["10.0.0.2:80"]
	h.ejected.Store(true)
	h.unejectAtNanos.Store(now + int64(time.Hour))
	if ch.available(eps[1]) {
		t.Fatal("ejected host must be unavailable")
	}
	if !ch.available(eps[0]) {
		t.Fatal("non-ejected healthy host must be available")
	}
	if got := ch.availableCount(eps); got != 1 {
		t.Fatalf("availableCount = %d, want 1 (one ejected)", got)
	}
}

func TestLazyUneject(t *testing.T) {
	eps := []Endpoint{{Host: "10.0.0.1", Port: 80}}
	ch := newClusterHealth(eps, 0.5)
	reg := stats.NewRegistry()
	ch.ejectionsActive = reg.NewGauge("ejections_active")
	base := int64(2_000_000_000)
	uneject := base + int64(30*time.Second)
	now := base
	ch.nowNanos = func() int64 { return now }
	h := ch.states["10.0.0.1:80"]
	h.ejected.Store(true)
	h.unejectAtNanos.Store(uneject)
	ch.ejectionsActive.Inc() // mirror the eject-time increment (done in detector, Task 5)

	// still ejected before the deadline
	if !ch.isEjected(eps[0]) {
		t.Fatal("must stay ejected before unejectAt")
	}
	if ch.ejectionsActive.Load() != 1 {
		t.Fatalf("gauge = %d before deadline, want 1", ch.ejectionsActive.Load())
	}

	// advance past the deadline -> lazy un-eject clears + decrements once
	now = uneject
	if ch.isEjected(eps[0]) {
		t.Fatal("must un-eject at/after unejectAt")
	}
	if h.ejected.Load() {
		t.Fatal("ejected flag must clear on lazy un-eject")
	}
	if ch.ejectionsActive.Load() != 0 {
		t.Fatalf("gauge = %d after un-eject, want 0", ch.ejectionsActive.Load())
	}

	// a second call must NOT double-decrement (CAS guard)
	if ch.isEjected(eps[0]) {
		t.Fatal("stays un-ejected on second call")
	}
	if ch.ejectionsActive.Load() != 0 {
		t.Fatalf("gauge = %d after second isEjected, want 0 (no double-dec)", ch.ejectionsActive.Load())
	}
}

func TestAvailable_NoOutlier(t *testing.T) {
	eps := []Endpoint{{Host: "10.0.0.1", Port: 80}, {Host: "10.0.0.2", Port: 80}, {Host: "10.0.0.3", Port: 80}}
	ch := newClusterHealth(eps, 0.5)
	ch.states["10.0.0.2:80"].healthy.Store(false) // one unhealthy, none ejected
	for _, ep := range eps {
		if ch.available(ep) != ch.isHealthy(ep) {
			t.Fatalf("available(%s)=%v != isHealthy=%v with no ejection", ep.Addr(), ch.available(ep), ch.isHealthy(ep))
		}
	}
	if ch.availableCount(eps) != ch.healthyCount(eps) {
		t.Fatalf("availableCount=%d != healthyCount=%d with no ejection", ch.availableCount(eps), ch.healthyCount(eps))
	}
}
