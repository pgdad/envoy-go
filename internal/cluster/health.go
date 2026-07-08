package cluster

import (
	"bufio"
	"context"
	"fmt"
	"math"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/pgdad/envoy-go/internal/stats"
)

// hostHealth is the per-host active-HC state (ADR-0242). healthy starts true
// (hosts begin healthy; the first check applies its result immediately).
type hostHealth struct {
	healthy       atomic.Bool
	consecSuccess atomic.Uint32
	consecFail    atomic.Uint32

	// Outlier-detection (passive-HC) sub-state, DISTINCT from the active-HC
	// healthy bit above (ADR-0245). A host can be ejected while still
	// active-HC-healthy and vice versa; availability is the AND of both.
	ejected        atomic.Bool   // outlier-ejected (separate from healthy)
	unejectAtNanos atomic.Int64  // un-eject deadline (UnixNano); 0 when not ejected
	consec5xx      atomic.Uint32 // consecutive 5xx (reset by a 2xx from this host)
	consecGw       atomic.Uint32 // consecutive gateway-class 5xx {502,503,504} (ADR-0246)
	consecLO       atomic.Uint32 // consecutive local-origin failures (split=true only) (ADR-0246)

	// Windowed per-host request accounting for the statistical (success_rate /
	// failure_percentage) sweep (ADR-0247). The sweep Swap-resets these each
	// interval; they accumulate on EVERY record path between sweeps.
	intervalTotal   atomic.Uint64 // requests this interval window (reset at each sweep) (ADR-0247)
	intervalSuccess atomic.Uint64 // successful requests this interval window (ADR-0247)
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
	states                map[string]*hostHealth // keyed by Endpoint.Addr()
	panicThresholdPercent int                    // floored integer percent below which panic fires (default 50; strict <, integer cross-multiply — AMEND-PT1)
	membershipHealthy     *stats.Gauge           // membership_healthy (injected at registerClusterMetrics; nil-guarded)
	panicCounter          *stats.Counter         // lb_healthy_panic (injected; nil-guarded)
	ejectionsActive       *stats.Gauge           // outlier_detection.ejections_active (injected at Task 8; nil-guarded)

	// nowNanos is the injectable clock backing the lazy un-eject deadline check
	// (overridden in unit tests for deterministic time). Defaults to wall time.
	nowNanos func() int64
}

func newClusterHealth(endpoints []Endpoint, panicThresholdPercent int) *clusterHealth {
	ch := &clusterHealth{
		states:                make(map[string]*hostHealth, len(endpoints)),
		panicThresholdPercent: panicThresholdPercent,
		nowNanos:              func() int64 { return time.Now().UnixNano() },
	}
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

// isEjected reports whether the host is outlier-ejected, performing the
// load-bearing lazy un-eject: once base_ejection_time has elapsed (nowNanos >=
// unejectAtNanos), it clears the ejected flag (CAS-guarded so concurrent scans
// decrement the ejections_active gauge exactly once) and reports false. An
// ejected host receives no further traffic, so this rejoins on the next LB scan
// past it. Unknown addr -> not ejected (mirrors isHealthy's unknown->true).
func (ch *clusterHealth) isEjected(ep Endpoint) bool {
	h, ok := ch.states[ep.Addr()]
	if !ok {
		return false
	}
	return ch.isEjectedState(h)
}

// isEjectedState is the *hostHealth-shaped body of isEjected (the lazy un-eject
// included), shared with available so the pick hot path does a SINGLE registry
// lookup per host instead of one for isHealthy plus one for isEjected.
func (ch *clusterHealth) isEjectedState(h *hostHealth) bool {
	if !h.ejected.Load() {
		return false
	}
	if ch.nowNanos() >= h.unejectAtNanos.Load() {
		if h.ejected.CompareAndSwap(true, false) && ch.ejectionsActive != nil {
			ch.ejectionsActive.Dec()
		}
		return false
	}
	return true
}

// available reports whether the host may receive traffic: active-HC-healthy AND
// not outlier-ejected. Semantically isHealthy(ep) && !isEjected(ep), fused into
// one map lookup (Pick scans call this per host — the hot path). The ordering
// is load-bearing: an unhealthy host short-circuits BEFORE the ejection check,
// so its lazy un-eject side effect stays deferred exactly as before.
func (ch *clusterHealth) available(ep Endpoint) bool {
	h, ok := ch.states[ep.Addr()]
	if !ok {
		return true // unknown addr: healthy (isHealthy) and not ejected (isEjected)
	}
	if !h.healthy.Load() {
		return false
	}
	return !ch.isEjectedState(h)
}

// availableCount is the available-host tally (the panic-denominator successor to
// healthyCount once outlier detection is wired).
func (ch *clusterHealth) availableCount(eps []Endpoint) int {
	n := 0
	for _, ep := range eps {
		if ch.available(ep) {
			n++
		}
	}
	return n
}

// ejectedCount returns the number of currently-ejected hosts among eps.
func (ch *clusterHealth) ejectedCount(eps []Endpoint) int {
	n := 0
	for _, ep := range eps {
		if ch.isEjected(ep) {
			n++
		}
	}
	return n
}

// inPanic reports whether the cluster is in panic mode: the healthy PERCENTAGE
// is strictly below the FLOORED integer threshold percent. Mirrors the
// reference EXACTLY via an integer cross-multiply (AMEND-PT1,
// reference_percent_threshold_integer_truncation): panic iff
// 100*availableCount < panicThresholdPercent*total (strict <, ULP-safe — no
// float division). Exactly-at-threshold does NOT panic; total==0 -> not panic.
func (ch *clusterHealth) inPanic(eps []Endpoint) bool {
	total := len(eps)
	if total == 0 {
		return false
	}
	return 100*ch.availableCount(eps) < ch.panicThresholdPercent*total
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

// panicGate is the shared leaf-LB health gate: it reports whether the pick
// bypasses per-host availability entirely — either no health registry at all
// (nil receiver: a cluster with no health_checks, the byte-stable fast path)
// or cluster-wide panic (blind pick; increments lb_healthy_panic). Every leaf
// Pick calls this before its availability scan; the per-policy indexing
// (counter/draw/ring/table) stays at the call sites so pick sequences are
// bit-identical to the pre-extraction copies. (ADR-0243)
func (ch *clusterHealth) panicGate(eps []Endpoint) bool {
	if ch == nil {
		return true
	}
	if ch.inPanic(eps) {
		ch.panicInc()
		return true
	}
	return false
}

// nextAvailable scans forward from start (wrapping) to the first available
// endpoint index; ok=false when no host is available. Shared by the leaf LBs
// whose scan order is "endpoint index + offset" (random, least_request);
// round_robin re-draws its counter per try and ring_hash/maglev walk ring
// points/table slots, so those keep their own loops. (ADR-0243)
func (ch *clusterHealth) nextAvailable(eps []Endpoint, start int) (int, bool) {
	n := len(eps)
	for k := 0; k < n; k++ {
		j := (start + k) % n
		if ch.available(eps[j]) {
			return j, true
		}
	}
	return 0, false
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
	endpoints   []Endpoint
	health      *clusterHealth
	checkerSpec // the parsed timing/threshold envelope + codec (promoted fields: interval/unhealthy/healthy/prober)
	firstDone   map[string]bool

	attempt, success, failure, networkFailure *stats.Counter
	healthyGauge                              *stats.Gauge // health_check.healthy
}

func newHealthChecker(eps []Endpoint, ch *clusterHealth, spec checkerSpec) *healthChecker {
	return &healthChecker{
		endpoints:   eps,
		health:      ch,
		checkerSpec: spec,
		firstDone:   make(map[string]bool, len(eps)),
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
		// PGV requires gt:0s for interval/timeout (a 0s interval would panic
		// time.NewTicker in healthChecker.run; a 0s timeout dials unbounded).
		if hc.GetInterval().AsDuration() <= 0 {
			return nil, false, fmt.Errorf("cluster: %q: health_check: interval: value must be greater than 0s", name)
		}
		if hc.GetTimeout() == nil {
			return nil, false, fmt.Errorf("cluster: %q: health_check: timeout is required", name)
		}
		if hc.GetTimeout().AsDuration() <= 0 {
			return nil, false, fmt.Errorf("cluster: %q: health_check: timeout: value must be greater than 0s", name)
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

// validatePanicThresholdRange rejects an out-of-range healthy_panic_threshold
// at boot, mirroring the reference's PGV [0,100] constraint (AMEND-PT4,
// ADR-0080). STANDALONE + UNCONDITIONAL: it must fire for ANY cluster carrying
// the field, including a plain cluster that never builds a health registry
// (whose parsePanicThreshold call sites are all health-guarded). Exactly 0 and
// 100 are accepted (inclusive). NaN is config-unreachable (protojson rejects a
// NaN double at unmarshal).
func validatePanicThresholdRange(c *clusterv3.Cluster, name string) error {
	p := c.GetCommonLbConfig().GetHealthyPanicThreshold()
	if p == nil {
		return nil
	}
	if v := p.GetValue(); v < 0 || v > 100 {
		return fmt.Errorf("cluster: %q: common_lb_config.healthy_panic_threshold: value must be inside range [0, 100]", name)
	}
	return nil
}

// parsePanicThreshold reads common_lb_config.healthy_panic_threshold (Percent;
// default 50%) and FLOORS it to an integer percent (AMEND-PT1: the reference
// integer-truncates the threshold toward zero before comparing). The value is
// guaranteed in [0,100] by validatePanicThresholdRange (called earlier in
// buildCluster, Task 4), so no clamp is needed. Returns an integer percent.
func parsePanicThreshold(c *clusterv3.Cluster) int {
	p := c.GetCommonLbConfig().GetHealthyPanicThreshold()
	if p == nil {
		return 50
	}
	return int(math.Floor(p.GetValue()))
}

// tierHealth returns a clusterHealth VIEW over the SAME per-host state as
// shared (per-host available()/isHealthy() honor live health-check results
// identically, since states is a Go map — a reference type — shared, not
// copied) but with panic PERMANENTLY DISABLED (panicThresholdPercent: 0, so
// inPanic can never fire — 100*avail < 0*total is never true). AMEND-P1
// confirmed the reference applies NO per-tier panic concept: a tier at 20%
// healthy correctly restricts its share to its healthy hosts, never
// internally flattening across its own unhealthy ones (AMEND-P1-COROLLARY).
// membershipHealthy/panicCounter/ejectionsActive stay nil (this view never
// emits stats — priorityLB.Pick itself is the SOLE caller of
// health.panicInc(), on its OWN capacity-sum bypass, below). nowNanos is
// shared (not reset) so any outlier-ejection lazy-uneject check evaluated
// through this view uses the identical injectable clock as the real
// registry — outlier-detection composition with priority tiers is out of
// scope this phase (SPEC's non-purposes list), so this is a defensive
// consistency choice, not a tested feature.
func tierHealth(shared *clusterHealth) *clusterHealth {
	return &clusterHealth{states: shared.states, panicThresholdPercent: 0, nowNanos: shared.nowNanos}
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
