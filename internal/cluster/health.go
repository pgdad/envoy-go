package cluster

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/esalaine/envoy-go/internal/stats"
)

// hostHealth is the per-host active-HC state (ADR-0242). healthy starts true
// (hosts begin healthy; the first check applies its result immediately).
type hostHealth struct {
	healthy       atomic.Bool
	consecSuccess atomic.Uint32
	consecFail    atomic.Uint32
}

func newHostHealth() *hostHealth {
	h := &hostHealth{}
	h.healthy.Store(true)
	return h
}

// recordResult applies one probe result. firstCheck transitions immediately;
// thereafter consecutive results gate transitions by the thresholds.
func (h *hostHealth) recordResult(ok bool, unhealthyThreshold, healthyThreshold uint32, firstCheck bool) {
	if ok {
		h.consecFail.Store(0)
		n := h.consecSuccess.Add(1)
		if firstCheck || n >= healthyThreshold {
			h.healthy.Store(true)
		}
	} else {
		h.consecSuccess.Store(0)
		n := h.consecFail.Add(1)
		if firstCheck || n >= unhealthyThreshold {
			h.healthy.Store(false)
		}
	}
}

// clusterHealth is the per-cluster host-health registry + the panic threshold +
// the build-time-injected stat handles. Consulted at Pick by the LB constructs
// (ADR-0243). nil on a Cluster with no health_checks -> the LBs use their fast path.
type clusterHealth struct {
	states            map[string]*hostHealth // keyed by Endpoint.Addr()
	panicThreshold    float64                // healthy fraction below which panic fires (default 0.5; strict <)
	membershipHealthy *stats.Gauge           // membership_healthy (injected at registerClusterMetrics; nil-guarded)
	panicCounter      *stats.Counter         // lb_healthy_panic (injected; nil-guarded)
}

func newClusterHealth(endpoints []Endpoint, panicThreshold float64) *clusterHealth {
	ch := &clusterHealth{states: make(map[string]*hostHealth, len(endpoints)), panicThreshold: panicThreshold}
	for _, ep := range endpoints {
		ch.states[ep.Addr()] = newHostHealth()
	}
	return ch
}

func (ch *clusterHealth) isHealthy(ep Endpoint) bool {
	if h, ok := ch.states[ep.Addr()]; ok {
		return h.healthy.Load()
	}
	return true
}

func (ch *clusterHealth) healthyCount(eps []Endpoint) int {
	n := 0
	for _, ep := range eps {
		if ch.isHealthy(ep) {
			n++
		}
	}
	return n
}

// inPanic reports whether the healthy fraction is strictly below the panic
// threshold (exactly 50% does NOT panic).
func (ch *clusterHealth) inPanic(eps []Endpoint) bool {
	total := len(eps)
	if total == 0 {
		return false
	}
	return float64(ch.healthyCount(eps))/float64(total) < ch.panicThreshold
}

// recomputeMembership Sets the membership_healthy gauge to the current healthy count.
func (ch *clusterHealth) recomputeMembership(eps []Endpoint) {
	if ch.membershipHealthy != nil {
		ch.membershipHealthy.Set(int64(ch.healthyCount(eps)))
	}
}

// panicInc increments lb_healthy_panic (nil-guarded for unit constructions).
func (ch *clusterHealth) panicInc() {
	if ch.panicCounter != nil {
		ch.panicCounter.Inc()
	}
}

type statusRange struct{ start, end int64 } // [start, end) per Int64Range

type httpHealthCheckCfg struct {
	host             string
	path             string
	timeout          time.Duration
	expectedStatuses []statusRange // empty -> default {200,201}
}

func (cfg httpHealthCheckCfg) statusOK(code int) bool {
	ranges := cfg.expectedStatuses
	if len(ranges) == 0 {
		ranges = []statusRange{{200, 201}}
	}
	for _, r := range ranges {
		if int64(code) >= r.start && int64(code) < r.end {
			return true
		}
	}
	return false
}

// probeHTTP dials addr, sends GET path, reports (ok, networkFailure). A fresh
// connection per probe (reuse_connection deferred). networkFailure is a dial/IO
// error (a sub-class of failure); a non-expected status is a non-network failure.
func probeHTTP(addr string, cfg httpHealthCheckCfg) (ok bool, networkFailure bool) {
	d := net.Dialer{Timeout: cfg.timeout}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		return false, true
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(cfg.timeout))
	host := cfg.host
	if host == "" {
		host = addr
	}
	if _, err := fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", cfg.path, host); err != nil {
		return false, true
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: "GET"})
	if err != nil {
		return false, true
	}
	defer func() { _ = resp.Body.Close() }()
	return cfg.statusOK(resp.StatusCode), false
}

// prober is one health-check codec: it probes addr and reports (ok, networkFailure).
// networkFailure (a dial/transport error) is a sub-class of failure (both stats
// increment); a non-network failure (e.g. a bad HTTP status / a NOT_SERVING gRPC
// response) sets ok=false, networkFailure=false. (ADR-0244)
type prober interface {
	probe(addr string) (ok, networkFailure bool)
}

// httpProber is the HTTP checker codec — the 39.1 probeHTTP body, behavior-unchanged.
type httpProber struct{ cfg httpHealthCheckCfg }

func (p httpProber) probe(addr string) (ok, networkFailure bool) { return probeHTTP(addr, p.cfg) }

// tcpProber is the connect-only TCP checker codec: a successful TCP connect proves
// liveness; a dial failure (refused/timeout) is a network failure. (ADR-0244)
type tcpProber struct{ timeout time.Duration }

func (p tcpProber) probe(addr string) (ok, networkFailure bool) {
	d := net.Dialer{Timeout: p.timeout}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		return false, true
	}
	_ = conn.Close()
	return true, false
}

// grpcProber is the gRPC checker codec: a unary grpc.health.v1.Health/Check over H2.
// A transport/RPC error (e.g. codes.Unavailable on a refused port) is a network
// failure; a returned non-SERVING status is an application failure (network_failure
// stays 0). (ADR-0244)
type grpcProber struct {
	serviceName string
	timeout     time.Duration
}

func (p grpcProber) probe(addr string) (ok, networkFailure bool) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return false, true
	}
	defer func() { _ = conn.Close() }()
	resp, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{Service: p.serviceName})
	if err != nil {
		return false, true // transport/RPC error (codes.Unavailable on a refused port) = network failure (D-S39.2-3 MVP)
	}
	return resp.GetStatus() == grpc_health_v1.HealthCheckResponse_SERVING, false // NOT_SERVING/other = application failure (network_failure stays 0)
}

// checkerSpec is one parsed health_check: the timing/threshold envelope + the codec.
type checkerSpec struct {
	interval           time.Duration
	unhealthy, healthy uint32
	prober             prober
}

type healthChecker struct {
	endpoints          []Endpoint
	health             *clusterHealth
	interval           time.Duration
	unhealthy, healthy uint32
	prober             prober
	firstDone          map[string]bool

	attempt, success, failure, networkFailure *stats.Counter
	healthyGauge                              *stats.Gauge // health_check.healthy
}

func newHealthChecker(eps []Endpoint, ch *clusterHealth, spec checkerSpec) *healthChecker {
	return &healthChecker{
		endpoints: eps,
		health:    ch,
		interval:  spec.interval,
		unhealthy: spec.unhealthy,
		healthy:   spec.healthy,
		prober:    spec.prober,
		firstDone: make(map[string]bool, len(eps)),
	}
}

func (hc *healthChecker) registerStats(r *stats.Registry, prefix string) {
	hc.attempt = r.NewCounter(prefix + "health_check.attempt")
	hc.success = r.NewCounter(prefix + "health_check.success")
	hc.failure = r.NewCounter(prefix + "health_check.failure")
	hc.networkFailure = r.NewCounter(prefix + "health_check.network_failure")
	hc.healthyGauge = r.NewGauge(prefix + "health_check.healthy")
}

// probeOnce runs one synchronous probe round over every host (the per-tick body
// and the unit-test entry). Stat handles are nil-guarded for bare unit use.
func (hc *healthChecker) probeOnce() {
	for _, ep := range hc.endpoints {
		ok, netFail := hc.prober.probe(ep.Addr())
		hc.applyResult(ep, ok, netFail)
	}
	if hc.healthyGauge != nil {
		hc.healthyGauge.Set(int64(hc.health.healthyCount(hc.endpoints)))
	}
	hc.health.recomputeMembership(hc.endpoints)
}

func (hc *healthChecker) applyResult(ep Endpoint, ok, netFail bool) {
	if hc.attempt != nil {
		hc.attempt.Inc()
		switch {
		case ok:
			hc.success.Inc()
		case netFail:
			hc.failure.Inc()
			hc.networkFailure.Inc()
		default:
			hc.failure.Inc()
		}
	}
	first := !hc.firstDone[ep.Addr()]
	hc.firstDone[ep.Addr()] = true
	if h, exists := hc.health.states[ep.Addr()]; exists {
		h.recordResult(ok, hc.unhealthy, hc.healthy, first)
	}
}

// parseHealthChecks validates + converts the cluster's health_checks (http +
// tcp + grpc as of 39.2). Returns nil for a cluster with no health_checks, plus
// hasGrpc: whether any parsed checker is a grpc checker (Task 7 consumes it for
// the must-be-H2 reject). Byte-stable rejects (ADR-0080): the reference's PGV
// requires interval/timeout/thresholds/the checker oneof/non-empty http path;
// envoy-go hand-rolls the equivalents.
func parseHealthChecks(c *clusterv3.Cluster, name string) ([]checkerSpec, bool, error) {
	var out []checkerSpec
	var hasGrpc bool
	for _, hc := range c.GetHealthChecks() {
		if hc.GetInterval() == nil {
			return nil, false, fmt.Errorf("cluster: %q: health_check: interval is required", name)
		}
		if hc.GetTimeout() == nil {
			return nil, false, fmt.Errorf("cluster: %q: health_check: timeout is required", name)
		}
		if hc.GetUnhealthyThreshold() == nil {
			return nil, false, fmt.Errorf("cluster: %q: health_check: unhealthy_threshold is required", name)
		}
		if hc.GetHealthyThreshold() == nil {
			return nil, false, fmt.Errorf("cluster: %q: health_check: healthy_threshold is required", name)
		}
		switch {
		case hc.GetHttpHealthCheck() != nil:
			httpCfg := hc.GetHttpHealthCheck()
			if httpCfg.GetPath() == "" {
				return nil, false, fmt.Errorf("cluster: %q: health_check: http path is required", name)
			}
			cfg := httpHealthCheckCfg{
				host:    httpCfg.GetHost(),
				path:    httpCfg.GetPath(),
				timeout: hc.GetTimeout().AsDuration(),
			}
			for _, r := range httpCfg.GetExpectedStatuses() {
				cfg.expectedStatuses = append(cfg.expectedStatuses, statusRange{r.GetStart(), r.GetEnd()})
			}
			out = append(out, checkerSpec{
				interval:  hc.GetInterval().AsDuration(),
				unhealthy: hc.GetUnhealthyThreshold().GetValue(),
				healthy:   hc.GetHealthyThreshold().GetValue(),
				prober:    httpProber{cfg: cfg},
			})
		case hc.GetTcpHealthCheck() != nil:
			tcp := hc.GetTcpHealthCheck()
			if tcp.GetSend() != nil || len(tcp.GetReceive()) > 0 {
				return nil, false, fmt.Errorf("cluster: %q: health_check: tcp_health_check send/receive payload matching is not supported", name)
			}
			out = append(out, checkerSpec{
				interval:  hc.GetInterval().AsDuration(),
				unhealthy: hc.GetUnhealthyThreshold().GetValue(),
				healthy:   hc.GetHealthyThreshold().GetValue(),
				prober:    tcpProber{timeout: hc.GetTimeout().AsDuration()},
			})
		case hc.GetGrpcHealthCheck() != nil:
			g := hc.GetGrpcHealthCheck()
			// authority/initial_metadata are silent-ignored (D-S39.2-2): no reject,
			// just don't read them.
			out = append(out, checkerSpec{
				interval:  hc.GetInterval().AsDuration(),
				unhealthy: hc.GetUnhealthyThreshold().GetValue(),
				healthy:   hc.GetHealthyThreshold().GetValue(),
				prober:    grpcProber{serviceName: g.GetServiceName(), timeout: hc.GetTimeout().AsDuration()},
			})
			hasGrpc = true
		default:
			return nil, false, fmt.Errorf("cluster: %q: health_check: a health_checker is required", name)
		}
	}
	return out, hasGrpc, nil
}

// parsePanicThreshold reads common_lb_config.healthy_panic_threshold (Percent;
// default 50%). Returns a fraction in [0,1].
func parsePanicThreshold(c *clusterv3.Cluster) float64 {
	p := c.GetCommonLbConfig().GetHealthyPanicThreshold()
	if p == nil {
		return 0.5
	}
	return p.GetValue() / 100.0
}

// run is the background loop: probe immediately, then every interval until ctx done.
func (hc *healthChecker) run(ctx context.Context) {
	hc.probeOnce()
	t := time.NewTicker(hc.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			hc.probeOnce()
		}
	}
}
