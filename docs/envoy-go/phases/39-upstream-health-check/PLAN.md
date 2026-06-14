# Phase 39.1 Active Health Checks (HTTP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `Cluster.health_checks` HTTP active health checking — the per-host health-state dimension, the cluster-level checker runtime, and the health-aware LB pick (all six constructs) with a panic-threshold fallback — the keystone of the Upstream-robustness family.

**Architecture:** A per-cluster `clusterHealth` registry (`map[string]*hostHealth` keyed by `Endpoint.Addr()`, atomic health bit + consecutive-result counters) seeded at build; a per-cluster × per-`health_check` background goroutine runtime (`interval`/`timeout`-driven HTTP probe reusing the dial path; first-check-immediate then threshold-gated transitions) started post-`Freeze` via `Manager.StartHealthChecks(ctx)` and cancelled+waited in `Manager.Drain()`; a build-time-injected `*clusterHealth` consulted at `Pick` by all six LB constructs (skip unhealthy + re-pick; ring_hash/maglev walk-to-next-healthy; `healthy_panic_threshold` fallback-to-all). `Endpoint` and the `loadBalancer.Pick` signature stay byte-stable; nil `health` = today's behavior (every existing fixture byte-identical).

**Tech Stack:** Go; `internal/cluster` (`health.go` new + the six LB files + `manager.go` + `cluster.go`); `cmd/envoy-go/main.go`; the `internal/stats` registry; the differential harness against `envoyproxy/envoy:contrib-v1.37.2`.

---

## Pre-flight — ADR-0045 split-gate re-check + D-S39 resolutions

### ADR-0045 split-gate FINAL re-check (per SPEC §3.0 / §11.7)

The 39.1 envelope: `health.go` (~250–350 LoC: the registry + the checker runtime + the HTTP probe) + the six-LB health-view consult (~120–180 LoC) + `manager.go`/`cluster.go` wiring (~80–120 LoC) + `main.go` (~5 LoC) + the `0066` fixture (~250 LoC test) ≈ **~500–800 prod LoC / 16 tasks**. Both axes are UNDER the ADR-0045 hard gate (`> ~25 tasks OR > ~1500 LoC`). **Decision: NO FURTHER SPLIT** — 39.1 lands as a single flat leg. (The TCP + gRPC checkers are 39.2, a separate SPEC/PLAN/IMPL.)

### D-S39 resolutions (SPEC §12)

- **D-S39-1 (file placement + health-view shape):** the registry + checker + probe land in a NEW `internal/cluster/health.go`. The health view is a concrete `*clusterHealth` field on each LB construct (NOT an interface — matches the codebase's concrete-struct style; nil = no health checks → behavior-neutral fast path). `buildLeafLB` gains a `health *clusterHealth` param threaded to each construct + to the `subsetLB` factory closure (subset children share the cluster's per-host health).
- **D-S39-2 (pre-first-check routability):** hosts start **HEALTHY**; the first check applies its result immediately (the dead host transitions to unhealthy on its first failed probe — AMEND-HC1; observationally equivalent to the reference after convergence, which is all `0066` asserts; the driver poll-to-converges before the load phase so the startup transient is never observed).
- **D-S39-3 (reject strings):** envoy-go does its OWN parse-reject (no PGV), byte-stable per ADR-0080, with envoy-go wordings (Task 11). The tcp/grpc checker in 39.1 is a config-build DEPARTURE-reject (the reference parse-accepts TCP; envoy-go fail-fast-rejects the unsupported checker — lifted at 39.2).
- **D-S39-4 (`0066` constants):** 2 live + 1 dead host (66% healthy > 50% → asserts FILTERING not panic); `interval: 0.5s`, `timeout: 0.5s`, `unhealthy_threshold: 1` (fast convergence); N=100 requests; the driver polls `/stats` until `membership_healthy == 2` on BOTH sides (30s deadline) before the load phase. The `fixture_workload_constant_desync` guard: N + the host counts are single-sourced.
- **D-S39-5 (test hygiene):** the checker loop takes an injectable tick source (a `<-chan time.Time` param, or a `probeOnce()` method the unit tests call directly) so unit tests are deterministic and leak-free; `Manager.StartHealthChecks(ctx)` derives a cancellable child context, and `Manager.Drain()` cancels it + `wg.Wait()`s (no goroutine leak under `-race`).
- **D-S39-6 (ADR-0045):** single 39.1 leg (above).

## File structure

- **Create `internal/cluster/health.go`** — `hostHealth`, `clusterHealth` (the registry + panic threshold + the `lb_healthy_panic`/`membership_healthy` stat handles + `isHealthy`/`healthyCount`/`inPanic`/`recomputeMembership`), `httpHealthCheckCfg`, `healthChecker` (the runtime + `probeOnce`/`probeHTTP`/`applyResult`), `parseHealthChecks`.
- **Create `internal/cluster/health_test.go`** — the registry-transition property test, the probe test, the parse/reject tests.
- **Modify `internal/cluster/loadbalancer.go`** — `roundRobin` gains a `health *clusterHealth` field + the health-aware `Pick`; a shared `pickHealthAware` helper (or per-construct skip).
- **Modify `internal/cluster/{random.go, leastrequest.go, ringhash.go, maglev.go, subset.go}`** — each construct gains the `health` field + the health-aware skip (ring_hash/maglev walk-to-next-healthy; subset children inherit via the factory).
- **Modify `internal/cluster/manager.go`** — `buildLeafLB` signature + `buildCluster` (seed the registry, parse `healthy_panic_threshold` + `health_checks`, build checkers), `registerClusterMetrics` (the new stats), `StartHealthChecks`/`Drain`.
- **Modify `internal/cluster/cluster.go`** — `Cluster` gains `health *clusterHealth` + `checkers []*healthChecker` + the `hcCancel`/`hcWG` lifecycle fields; a `Cluster.dialProbe`/reuse of the dial path for the probe.
- **Modify `cmd/envoy-go/main.go`** — the `cm.StartHealthChecks(ctx)` boot call after `bs.Stats.Freeze()`.
- **Create `test/fixtures/0066-health-check-http/{driver/driver.go, driver/driver_test.go, expectations.yaml, README.md}`** + register in `test/differential/runner_test.go`.

---

### Task 1: First-task baselines/anchors gate + PROGRESS.md

**Files:**
- Create: `docs/envoy-go/phases/39-upstream-health-check/PROGRESS.md`

- [ ] **Step 1: Capture the pre-IMPL baselines** (the ADR-0052 atomic-landing anchors)

```bash
cd /home/esa/git/envoy-go
echo "fixtures: $(ls -d test/fixtures/[0-9]* | wc -l)"          # expect 67
echo "fuzzers: $(grep -rh '^func Fuzz' $(find ./internal -name fuzz_test.go) | wc -l)"  # expect 42
grep -c '^## ADR-' docs/envoy-go/DECISIONS.md                    # note the tail (ADR-0241)
grep -nE "Phase 38.2|surface .*1125|STAYS 1125" docs/envoy-go/BEHAVIOR_CONTRACT.md | head -2  # stat surface 1125
go build ./... && echo BUILD_OK
```
Expected: fixtures 67, fuzzers 42, DECISIONS tail ADR-0241, stat surface 1125, BUILD_OK.

- [ ] **Step 2: Create PROGRESS.md** — a task checklist (Tasks 1–16) + a "baselines captured" block (the Step-1 numbers) + an "as-built counts" block (filled at Task 16). Mirror `docs/envoy-go/phases/38.2-weighted-clusters/PROGRESS.md`.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/39-upstream-health-check/PROGRESS.md
git commit -m "phase 39.1 Task 1: baselines/anchors + PROGRESS.md"
```

---

### Task 2: The `hostHealth` registry + transitions (`health.go`)

**Files:**
- Create: `internal/cluster/health.go`
- Create: `internal/cluster/health_test.go`

- [ ] **Step 1: Write the failing test** (the AMEND-HC1 transition property: first-check-immediate, then threshold-gated)

```go
package cluster

import "testing"

func TestHostHealth_Transitions(t *testing.T) {
	h := newHostHealth() // starts HEALTHY (D-S39-2)
	if !h.healthy.Load() {
		t.Fatal("want initial healthy")
	}
	// first check immediate: one failure → unhealthy (threshold bypassed on first transition)
	h.recordResult(false /*ok*/, 2 /*unhealthyThreshold*/, 2 /*healthyThreshold*/, true /*firstCheck*/)
	if h.healthy.Load() {
		t.Fatal("first failure should mark unhealthy immediately")
	}
	// recovery needs healthyThreshold(2) consecutive successes
	h.recordResult(true, 2, 2, false)
	if h.healthy.Load() {
		t.Fatal("one success < healthyThreshold should stay unhealthy")
	}
	h.recordResult(true, 2, 2, false)
	if !h.healthy.Load() {
		t.Fatal("two successes == healthyThreshold should become healthy")
	}
	// going down needs unhealthyThreshold(2) consecutive failures (post-first-check)
	h.recordResult(false, 2, 2, false)
	if !h.healthy.Load() {
		t.Fatal("one failure < unhealthyThreshold should stay healthy")
	}
	h.recordResult(false, 2, 2, false)
	if h.healthy.Load() {
		t.Fatal("two failures == unhealthyThreshold should become unhealthy")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/cluster/ -run TestHostHealth_Transitions -count=1`
Expected: FAIL (undefined `newHostHealth`).

- [ ] **Step 3: Implement** (in `health.go`)

```go
package cluster

import "sync/atomic"

// hostHealth is the per-host active-HC state (ADR-0242). healthy starts true
// (D-S39-2: hosts begin healthy; the first check applies its result immediately).
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

// recordResult applies one probe result. firstCheck transitions immediately
// (AMEND-HC1); thereafter consecutive results gate transitions by the thresholds.
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
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./internal/cluster/ -run TestHostHealth_Transitions -count=1`
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit** (every task ends with this — `feedback_pertask_gofmt_lint`)

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/health.go internal/cluster/health_test.go
git commit -m "phase 39.1 Task 2: hostHealth registry + first-check-immediate transitions"
```

---

### Task 3: The `clusterHealth` view (registry + panic threshold + accessors)

**Files:**
- Modify: `internal/cluster/health.go`
- Modify: `internal/cluster/health_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestClusterHealth_View(t *testing.T) {
	eps := []Endpoint{{Host: "10.0.0.1", Port: 80}, {Host: "10.0.0.2", Port: 80}, {Host: "10.0.0.3", Port: 80}}
	ch := newClusterHealth(eps, 0.5) // panic threshold 50%
	if ch.healthyCount(eps) != 3 || ch.inPanic(eps) {
		t.Fatal("all healthy: count 3, no panic")
	}
	ch.states["10.0.0.3:80"].healthy.Store(false) // 2/3 healthy = 66% > 50%
	if ch.healthyCount(eps) != 2 || ch.inPanic(eps) {
		t.Fatal("2/3 healthy (66%): no panic, filtering")
	}
	ch.states["10.0.0.2:80"].healthy.Store(false) // 1/3 = 33% < 50% → panic
	if !ch.inPanic(eps) {
		t.Fatal("1/3 healthy (33%): panic")
	}
	if ch.isHealthy(eps[2]) {
		t.Fatal("ep3 should be unhealthy")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `go test ./internal/cluster/ -run TestClusterHealth_View -count=1` → FAIL.

- [ ] **Step 3: Implement** (in `health.go`)

```go
import "github.com/esalaine/envoy-go/internal/stats"

// clusterHealth is the per-cluster host-health registry + the panic threshold +
// the build-time-injected stat handles. Consulted at Pick by the LB constructs
// (ADR-0243). nil on a Cluster with no health_checks → the LBs use their fast path.
type clusterHealth struct {
	states         map[string]*hostHealth // keyed by Endpoint.Addr()
	panicThreshold float64                // healthy fraction below which panic fires (default 0.5; strict <)
	// injected at registerClusterMetrics (Task 10); nil-guarded:
	membershipHealthy *stats.Gauge   // membership_healthy
	panicCounter      *stats.Counter // lb_healthy_panic
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
	return true // unknown host (defensive) → healthy
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
// threshold (AMEND-HC5: strict <; exactly 50% does NOT panic).
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
```

- [ ] **Step 4: Run to verify it passes** — PASS.
- [ ] **Step 5: gofmt + lint + commit** — `git commit -m "phase 39.1 Task 3: clusterHealth view + strict-< panic threshold"`

---

### Task 4: The HTTP probe codec (`probeHTTP`)

**Files:**
- Modify: `internal/cluster/health.go`
- Modify: `internal/cluster/health_test.go`

- [ ] **Step 1: Write the failing test** (a probe against a live httptest server + a dead address)

```go
import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
	cfg.path = "/x" // 503 → not in expected → failure (not a network failure)
	if ok, netErr := probeHTTP(addr, cfg); ok || netErr {
		t.Fatalf("503 should fail non-network: ok=%v netErr=%v", ok, netErr)
	}
	if ok, netErr := probeHTTP("127.0.0.1:1", cfg); ok || !netErr {
		t.Fatalf("dead addr should be a network failure: ok=%v netErr=%v", ok, netErr)
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL (undefined `probeHTTP`).

- [ ] **Step 3: Implement** (in `health.go`) — a minimal H1 GET probe (no pool; a fresh conn per probe; `codec_client_type` H1 only in 39.1)

```go
import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"
)

type statusRange struct{ start, end int64 } // [start, end) per Int64Range

type httpHealthCheckCfg struct {
	host             string
	path             string
	interval         time.Duration
	timeout          time.Duration
	unhealthy        uint32
	healthy          uint32
	expectedStatuses []statusRange // empty → default {200,201}
}

func (cfg httpHealthCheckCfg) statusOK(code int) bool {
	ranges := cfg.expectedStatuses
	if len(ranges) == 0 {
		ranges = []statusRange{{200, 201}} // AMEND-HC2 default 200
	}
	for _, r := range ranges {
		if int64(code) >= r.start && int64(code) < r.end {
			return true
		}
	}
	return false
}

// probeHTTP dials addr, sends GET path, and reports (ok, networkFailure).
// networkFailure is true on dial/IO error (a sub-class of failure); a non-expected
// status is a non-network failure. A fresh connection per probe (reuse_connection
// deferred — §2).
func probeHTTP(addr string, cfg httpHealthCheckCfg) (ok bool, networkFailure bool) {
	d := net.Dialer{Timeout: cfg.timeout}
	conn, err := d.Dial("tcp", addr)
	if err != nil {
		return false, true
	}
	defer conn.Close()
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
	defer resp.Body.Close()
	return cfg.statusOK(resp.StatusCode), false
}
```

- [ ] **Step 4: Run to verify it passes** — PASS.
- [ ] **Step 5: gofmt + lint + commit** — `git commit -m "phase 39.1 Task 4: HTTP probe codec (GET path + expected_statuses)"`

---

### Task 5: The `healthChecker` runtime (probeOnce + applyResult)

**Files:**
- Modify: `internal/cluster/health.go`
- Modify: `internal/cluster/health_test.go`

- [ ] **Step 1: Write the failing test** (a deterministic single-tick probe round over a live + a dead host; assert transitions + stats; NO real timer)

```go
func TestHealthChecker_ProbeOnce(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer srv.Close()
	live := srv.Listener.Addr().String()
	eps := []Endpoint{addrEndpoint(live), {Host: "127.0.0.1", Port: 1}}
	ch := newClusterHealth(eps, 0.5)
	reg := stats.NewRegistry()
	hc := newHealthChecker(eps, ch, httpHealthCheckCfg{path: "/", timeout: time.Second, unhealthy: 1, healthy: 1})
	hc.registerStats(reg, "cluster.c.")
	hc.probeOnce() // one synchronous round over all hosts
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
```
(Add a small `addrEndpoint(host:port string) Endpoint` test helper that splits host/port.)

- [ ] **Step 2: Run to verify it fails** — FAIL.

- [ ] **Step 3: Implement** (in `health.go`)

```go
import "context"

type healthChecker struct {
	endpoints []Endpoint
	health    *clusterHealth
	cfg       httpHealthCheckCfg
	firstDone map[string]bool // per-host: has the first check completed (for AMEND-HC1)
	// stats (injected via registerStats at register time; Task 10):
	attempt, success, failure, networkFailure *stats.Counter
	healthyGauge                              *stats.Gauge // health_check.healthy
}

func newHealthChecker(eps []Endpoint, ch *clusterHealth, cfg httpHealthCheckCfg) *healthChecker {
	return &healthChecker{endpoints: eps, health: ch, cfg: cfg, firstDone: make(map[string]bool, len(eps))}
}

func (hc *healthChecker) registerStats(r *stats.Registry, prefix string) {
	hc.attempt = r.NewCounter(prefix + "health_check.attempt")
	hc.success = r.NewCounter(prefix + "health_check.success")
	hc.failure = r.NewCounter(prefix + "health_check.failure")
	hc.networkFailure = r.NewCounter(prefix + "health_check.network_failure")
	hc.healthyGauge = r.NewGauge(prefix + "health_check.healthy")
}

// probeOnce runs one synchronous probe round over every host (the unit-test +
// per-tick body). Concurrency-safe stat handles; nil-guarded for bare unit use.
func (hc *healthChecker) probeOnce() {
	for _, ep := range hc.endpoints {
		ok, netFail := probeHTTP(ep.Addr(), hc.cfg)
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
		h.recordResult(ok, hc.cfg.unhealthy, hc.cfg.healthy, first)
	}
}

// run is the background loop: probe immediately, then every interval until ctx done.
func (hc *healthChecker) run(ctx context.Context) {
	hc.probeOnce() // first check at startup (AMEND-HC3)
	t := time.NewTicker(hc.cfg.interval)
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
```
(`networkFailure` is also counted under `failure` per AMEND-HC4 — `network_failure` is a sub-class. Verify against the reference at IMPL; the §11.1 probe showed `failure: 1, network_failure: 1` for one dead-host failure → they co-increment.)

- [ ] **Step 4: Run to verify it passes** — PASS.
- [ ] **Step 5: gofmt + lint + commit** — `git commit -m "phase 39.1 Task 5: healthChecker runtime (probeOnce + applyResult + stats)"`

---

### Task 6: Parse `health_checks` + `healthy_panic_threshold` + the reject roster (`manager.go`)

**Files:**
- Modify: `internal/cluster/manager.go`
- Modify: `internal/cluster/health.go` (`parseHealthChecks`)
- Modify: `internal/cluster/manager_test.go`

- [ ] **Step 1: Write the failing tests** (the §6 rejects + a valid parse + the panic-threshold default)

```go
// in manager_test.go — drive buildCluster via NewManager with a minimal bootstrap.
func TestHealthCheck_Rejects(t *testing.T) {
	cases := []struct{ name, want string; cfg string }{
		{"no_checker", "health_check: a health_checker is required", `[{timeout: 1s, interval: 1s, unhealthy_threshold: 2, healthy_threshold: 2}]`},
		{"empty_path", "health_check: http path is required", `[{timeout: 1s, interval: 1s, unhealthy_threshold: 2, healthy_threshold: 2, http_health_check: {path: ""}}]`},
		{"no_interval", "health_check: interval is required", `[{timeout: 1s, unhealthy_threshold: 2, healthy_threshold: 2, http_health_check: {path: /}}]`},
		{"no_timeout", "health_check: timeout is required", `[{interval: 1s, unhealthy_threshold: 2, healthy_threshold: 2, http_health_check: {path: /}}]`},
		{"tcp_unsupported", "health_check: only http_health_check is supported", `[{timeout: 1s, interval: 1s, unhealthy_threshold: 2, healthy_threshold: 2, tcp_health_check: {}}]`},
	}
	// ... build a bootstrap with each health_checks cfg, assert NewManager errors with want.
}
```
(Use the existing `manager_test.go` bootstrap-builder helper; the exact wordings are envoy-go's own byte-stable strings — D-S39-3; the IMPL may refine to match the reference more closely, recorded in BEHAVIOR_CONTRACT.)

- [ ] **Step 2: Run to verify it fails** — FAIL.

- [ ] **Step 3: Implement** `parseHealthChecks` (health.go) + wire into `buildCluster`

```go
// parseHealthChecks validates + converts the cluster's health_checks (HTTP only
// in 39.1). Returns nil for a cluster with no health_checks. Byte-stable rejects
// per ADR-0080 (D-S39-3). The reference's PGV requires interval/timeout/thresholds/
// the checker oneof/non-empty path; envoy-go hand-rolls the equivalents.
func parseHealthChecks(c *clusterv3.Cluster, name string) ([]httpHealthCheckCfg, error) {
	var out []httpHealthCheckCfg
	for _, hc := range c.GetHealthChecks() {
		if hc.GetInterval() == nil {
			return nil, fmt.Errorf("cluster: %q: health_check: interval is required", name)
		}
		if hc.GetTimeout() == nil {
			return nil, fmt.Errorf("cluster: %q: health_check: timeout is required", name)
		}
		http := hc.GetHttpHealthCheck()
		switch {
		case http == nil && (hc.GetTcpHealthCheck() != nil || hc.GetGrpcHealthCheck() != nil):
			return nil, fmt.Errorf("cluster: %q: health_check: only http_health_check is supported", name) // tcp/grpc → 39.2
		case http == nil:
			return nil, fmt.Errorf("cluster: %q: health_check: a health_checker is required", name)
		}
		if http.GetPath() == "" {
			return nil, fmt.Errorf("cluster: %q: health_check: http path is required", name)
		}
		cfg := httpHealthCheckCfg{
			host:     http.GetHost(),
			path:     http.GetPath(),
			interval: hc.GetInterval().AsDuration(),
			timeout:  hc.GetTimeout().AsDuration(),
			unhealthy: defUint(hc.GetUnhealthyThreshold(), 0), // 0 → required; see below
			healthy:   defUint(hc.GetHealthyThreshold(), 0),
		}
		if hc.GetUnhealthyThreshold() == nil {
			return nil, fmt.Errorf("cluster: %q: health_check: unhealthy_threshold is required", name)
		}
		if hc.GetHealthyThreshold() == nil {
			return nil, fmt.Errorf("cluster: %q: health_check: healthy_threshold is required", name)
		}
		for _, r := range http.GetExpectedStatuses() {
			cfg.expectedStatuses = append(cfg.expectedStatuses, statusRange{r.GetStart(), r.GetEnd()})
		}
		out = append(out, cfg)
	}
	return out, nil
}

func defUint(v *wrapperspb.UInt32Value, d uint32) uint32 {
	if v == nil {
		return d
	}
	return v.GetValue()
}

// parsePanicThreshold reads common_lb_config.healthy_panic_threshold (Percent;
// default 50% — AMEND-HC5). Returns a fraction in [0,1].
func parsePanicThreshold(c *clusterv3.Cluster) float64 {
	p := c.GetCommonLbConfig().GetHealthyPanicThreshold()
	if p == nil {
		return 0.5
	}
	return p.GetValue() / 100.0
}
```
Wire into `buildCluster` (after `extractEndpoints`, before `buildLeafLB`):
```go
hcCfgs, err := parseHealthChecks(c, name)
if err != nil {
	return nil, err
}
var health *clusterHealth
if len(hcCfgs) > 0 {
	health = newClusterHealth(endpoints, parsePanicThreshold(c))
}
// ... pass `health` to buildLeafLB (Task 7); build cl.checkers from hcCfgs + cl.health = health.
```

- [ ] **Step 4: Run to verify it passes** — PASS.
- [ ] **Step 5: gofmt + lint + commit** — `git commit -m "phase 39.1 Task 6: parse health_checks + panic threshold + reject roster"`

---

### Task 7: Thread `health` through `buildLeafLB` + the six constructs (byte-stable, nil = today)

**Files:**
- Modify: `internal/cluster/manager.go` (`buildLeafLB` signature + the two call sites)
- Modify: `internal/cluster/{loadbalancer.go, random.go, leastrequest.go, ringhash.go, maglev.go, subset.go}` (add the `health *clusterHealth` field; constructors accept it)
- Modify: the LB constructor tests (pass nil)

- [ ] **Step 1: Write the failing test** (all existing LB unit tests still pass with nil health; a new struct-field presence test)

```go
func TestBuildLeafLB_NilHealthByteStable(t *testing.T) {
	eps := []Endpoint{{Host: "10.0.0.1", Port: 80}, {Host: "10.0.0.2", Port: 80}}
	lb := &roundRobin{endpoints: eps} // health nil
	for i := 0; i < 4; i++ {
		ep, _, err := lb.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatal(err)
		}
		_ = ep
	}
	// nil health → today's behavior, no panic; existing rr ordering test still passes.
}
```

- [ ] **Step 2: Run to verify it fails** (after you change `buildLeafLB`'s signature, the compile breaks the call sites) — `go build ./...` → FAIL until wired.

- [ ] **Step 3: Implement** — `buildLeafLB(c *clusterv3.Cluster, name string, endpoints []Endpoint, health *clusterHealth) (loadBalancer, error)`; each construct gains a `health *clusterHealth` field set in its constructor (e.g. `&roundRobin{endpoints: endpoints, health: health}`, `newLeastRequest(endpoints, cc, health)`, `newRandom(endpoints, health)`, `newRingHash(endpoints, cfg, health)`, `newMaglev(endpoints, cfg, health)`). The `subsetLB` factory closure passes the same `health` to children:
```go
lb, err := buildLeafLB(c, name, endpoints, health)
// subset wrap:
factory := func(sub []Endpoint) (loadBalancer, error) { return buildLeafLB(c, name, sub, health) }
```
Keep `Pick` UNCHANGED in this task (the field is added but not yet consulted) — behavior byte-stable.

- [ ] **Step 4: Run to verify it passes** — `go build ./... && go test ./internal/cluster/ -count=1` → PASS (all existing LB tests green; nil health is inert).
- [ ] **Step 5: gofmt + lint + commit** — `git commit -m "phase 39.1 Task 7: thread *clusterHealth into buildLeafLB + the six constructs (inert)"`

---

### Task 8: Health-aware pick — `roundRobin` + `randomLB` + `leastRequest` (skip + panic)

**Files:**
- Modify: `internal/cluster/{loadbalancer.go, random.go, leastrequest.go}`
- Modify: their `_test.go`

- [ ] **Step 1: Write the failing test** (rr skips the unhealthy host; panic routes to all)

```go
func TestRoundRobin_HealthAware(t *testing.T) {
	eps := []Endpoint{{Host: "10.0.0.1", Port: 80}, {Host: "10.0.0.2", Port: 80}, {Host: "10.0.0.3", Port: 80}}
	ch := newClusterHealth(eps, 0.5)
	ch.states["10.0.0.2:80"].healthy.Store(false) // 2/3 healthy (66% > 50%) → filter, no panic
	rr := &roundRobin{endpoints: eps, health: ch}
	for i := 0; i < 30; i++ {
		ep, _, err := rr.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatal(err)
		}
		if ep.Host == "10.0.0.2" {
			t.Fatal("unhealthy host must never be picked when filtering")
		}
	}
	// panic: 1/3 healthy (33% < 50%) → route to all (incl unhealthy), lb_healthy_panic Inc
	reg := stats.NewRegistry()
	ch.panicCounter = reg.NewCounter("c.lb_healthy_panic")
	ch.states["10.0.0.3:80"].healthy.Store(false)
	seen := map[string]bool{}
	for i := 0; i < 30; i++ {
		ep, _, _ := rr.Pick(0, false, SubsetMatch{}, false)
		seen[ep.Host] = true
	}
	if !seen["10.0.0.2"] || !seen["10.0.0.3"] {
		t.Fatal("panic mode must route to all hosts")
	}
	if ch.panicCounter.Load() == 0 {
		t.Fatal("lb_healthy_panic should increment in panic mode")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL.

- [ ] **Step 3: Implement** the health-aware `roundRobin.Pick` (and mirror for `randomLB` re-draw + `leastRequest` filter-then-P2C)

```go
func (rr *roundRobin) Pick(_ uint64, _ bool, _ SubsetMatch, _ bool) (Endpoint, func(), error) {
	n := len(rr.endpoints)
	if n == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	if rr.health == nil { // fast path — today's behavior, byte-identical
		i := rr.counter.Add(1) - 1
		return rr.endpoints[int(i)%n], noopRelease, nil
	}
	if rr.health.inPanic(rr.endpoints) { // panic → route to all
		rr.health.panicInc()
		i := rr.counter.Add(1) - 1
		return rr.endpoints[int(i)%n], noopRelease, nil
	}
	for tries := 0; tries < n; tries++ { // skip unhealthy
		i := rr.counter.Add(1) - 1
		ep := rr.endpoints[int(i)%n]
		if rr.health.isHealthy(ep) {
			return ep, noopRelease, nil
		}
	}
	return Endpoint{}, noopRelease, errNoEndpoints // unreachable when !inPanic (≥threshold healthy)
}
```
Add `func (ch *clusterHealth) panicInc() { if ch.panicCounter != nil { ch.panicCounter.Inc() } }`. `randomLB.Pick`: same shape, re-draw up to `n` tries. `leastRequest.Pick`: when `health != nil && !inPanic`, sample `choice_count` among the HEALTHY hosts only (filter then P2C); panic → over all.

- [ ] **Step 4: Run to verify it passes** — PASS (+ existing rr/random/leastRequest tests still green with nil health).
- [ ] **Step 5: gofmt + lint + commit** — `git commit -m "phase 39.1 Task 8: health-aware pick — round_robin/random/least_request (skip + panic)"`

---

### Task 9: Health-aware pick — `ringHashLB` + `maglevLB` walk-to-next-healthy + `subsetLB` inheritance

**Files:**
- Modify: `internal/cluster/{ringhash.go, maglev.go}`
- Modify: their `_test.go`

- [ ] **Step 1: Write the failing test** (a given key whose primary host is unhealthy walks to the next healthy host; key→host stability preserved when all healthy)

```go
func TestRingHash_HealthAware_WalkToNextHealthy(t *testing.T) {
	eps := makeEndpoints(4) // helper
	ch := newClusterHealth(eps, 0.5)
	// use a real ring config (an all-zero ringHashCfg yields an empty ring); add a
	// health param to newRingHash in Task 7, or construct + set rh.health = ch here.
	cfg := ringHashCfg{minRingSize: 1024, maxRingSize: 8388608, hashFunc: hashXX} // match parseRingHashLbConfig defaults
	rh := newRingHash(eps, cfg)
	rh.health = ch
	// pick the primary host for a fixed key (all healthy):
	primary, _, _ := rh.Pick(12345, true, SubsetMatch{}, false)
	// mark primary unhealthy → same key must now resolve to a DIFFERENT, healthy host:
	ch.states[primary.Addr()].healthy.Store(false)
	got, _, _ := rh.Pick(12345, true, SubsetMatch{}, false)
	if got.Addr() == primary.Addr() {
		t.Fatal("unhealthy primary must be walked past")
	}
	if !ch.isHealthy(got) {
		t.Fatal("walked-to host must be healthy")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL.

- [ ] **Step 3: Implement** — in `ringHashLB.Pick`, after computing the primary ring position, if `health != nil && !inPanic`, walk forward through ring entries to the next healthy host (bounded by the ring size); panic → return the primary host. Mirror in `maglevLB.Pick`: from `table[hash%M]`, if unhealthy, walk `table[(hash+k)%M]` for k=1.. until a healthy host or full loop; panic → the primary. `subsetLB` needs NO change — its children are the leaf constructs, already health-aware via the Task-7 factory threading (the child's Pick filters within the subset).

- [ ] **Step 4: Run to verify it passes** — PASS (+ existing ring/maglev affinity tests still green with nil health: key→host stability unchanged).
- [ ] **Step 5: gofmt + lint + commit** — `git commit -m "phase 39.1 Task 9: health-aware pick — ring_hash/maglev walk-to-next-healthy; subset inherits"`

---

### Task 10: Stat registrations + injection (`registerClusterMetrics`)

**Files:**
- Modify: `internal/cluster/manager.go` (`registerClusterMetrics`)
- Modify: `internal/cluster/cluster.go` (the `health`/`checkers` fields)
- Modify: `internal/cluster/manager_test.go`

- [ ] **Step 1: Write the failing test** (a cluster with health_checks registers the +7 stats; a cluster without does NOT)

```go
func TestRegisterClusterMetrics_HealthStats(t *testing.T) {
	reg := stats.NewRegistry()
	// build a cluster with health_checks → assert the 7 names exist:
	want := []string{"cluster.c.health_check.attempt", "cluster.c.health_check.success",
		"cluster.c.health_check.failure", "cluster.c.health_check.network_failure",
		"cluster.c.health_check.healthy", "cluster.c.membership_healthy", "cluster.c.lb_healthy_panic"}
	// ... NewManager(bootstrap-with-health-checks) over reg; assert each name registered.
	// a cluster WITHOUT health_checks must NOT register them.
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL.

- [ ] **Step 3: Implement** — in `registerClusterMetrics`, after the existing block:
```go
if c.health != nil {
	c.health.membershipHealthy = r.NewGauge(prefix + "membership_healthy")
	c.health.membershipHealthy.Set(int64(len(c.endpoints))) // all start healthy (D-S39-2)
	c.health.panicCounter = r.NewCounter(prefix + "lb_healthy_panic")
	for _, hc := range c.checkers {
		hc.registerStats(r, prefix) // health_check.{attempt,success,failure,network_failure,healthy}
	}
}
```
(Per-checker stats: in 39.1 a cluster has ≤1 health_check in the fixture; if multiple, the reference aggregates under one `health_check.*` namespace — for the MVP assume ≤1 checker per cluster, or share one stat set across checkers. Pin at IMPL; `0066` uses one.) Add `Cluster.health *clusterHealth` + `Cluster.checkers []*healthChecker` fields in cluster.go.

- [ ] **Step 4: Run to verify it passes** — PASS. Verify the stat-surface delta:
```bash
# build a 1-cluster-with-health-checks bootstrap, scrape, count the new names → +7
```
- [ ] **Step 5: gofmt + lint + commit** — `git commit -m "phase 39.1 Task 10: register +7 health stats (scoped to health-checked clusters)"`

---

### Task 11: The lifecycle — `Manager.StartHealthChecks(ctx)` + `Drain` + `main.go` boot

**Files:**
- Modify: `internal/cluster/manager.go` (`StartHealthChecks`, `Drain`)
- Modify: `internal/cluster/cluster.go` (the `Manager` lifecycle fields)
- Modify: `cmd/envoy-go/main.go`
- Modify: `internal/cluster/manager_test.go`

- [ ] **Step 1: Write the failing test** (StartHealthChecks launches checkers; Drain cancels + waits with no goroutine leak)

```go
func TestStartHealthChecks_Lifecycle(t *testing.T) {
	// build a manager with a cluster pointing at a live httptest backend + a dead host
	// StartHealthChecks(ctx); poll until membership_healthy converges; Drain(); assert goroutines exited.
	// Run under -race to catch leaks.
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL (undefined `StartHealthChecks`).

- [ ] **Step 3: Implement** — `Manager` gains `hcCancel context.CancelFunc` + `hcWG sync.WaitGroup`:
```go
func (m *Manager) StartHealthChecks(ctx context.Context) {
	hcCtx, cancel := context.WithCancel(ctx)
	m.hcCancel = cancel
	for _, c := range m.clusters {
		for _, hc := range c.checkers {
			m.hcWG.Add(1)
			go func(hc *healthChecker) { defer m.hcWG.Done(); hc.run(hcCtx) }(hc)
		}
	}
}
```
Extend `Drain()`:
```go
func (m *Manager) Drain() {
	if m.hcCancel != nil {
		m.hcCancel()
		m.hcWG.Wait()
	}
	for _, c := range m.clusters {
		c.closePool()
	}
}
```
In `cmd/envoy-go/main.go`, after `bs.Stats.Freeze()` (line ~272):
```go
cm.StartHealthChecks(ctx) // ctx = the signal.NotifyContext; checkers stop on SIGTERM + Drain
```

- [ ] **Step 4: Run to verify it passes** — `go test ./internal/cluster/ -run TestStartHealthChecks_Lifecycle -race -count=1` → PASS, no leak.
- [ ] **Step 5: gofmt + lint + commit** — `git commit -m "phase 39.1 Task 11: StartHealthChecks/Drain lifecycle + main.go boot call"`

---

### Task 12: The `0066-health-check-http` differential fixture

**Files:**
- Create: `test/fixtures/0066-health-check-http/{driver/driver.go, driver/driver_test.go, expectations.yaml, README.md}`
- Modify: `test/differential/runner_test.go` (blank-import the 0066 driver)

- [ ] **Step 1: Author the fixture** (per D-S39-4): a cluster `c_hc` (ROUND_ROBIN) over 2 live HTTP backends + 1 dead unbound host, `health_checks: [{http_health_check{path:/health}, interval 0.5s, timeout 0.5s, unhealthy_threshold 1, healthy_threshold 1}]`; an HTTP listener → `c_hc`. **CLUSTER TYPE — the two-bootstrap split (the `0065` precedent):** envoy-go's `buildCluster` (`manager.go:278-280`) supports ONLY `type: STATIC`, while the reference (real Envoy) side uses `STRICT_DNS` with backend hostnames (per `reference_docker_probe_bridge_network`). So author TWO bootstraps: the **subject (envoy-go) cluster = STATIC** with `127.0.0.x:port` endpoints (2 live + 1 dead unbound), the **reference cluster = STRICT_DNS** with the bridge-network backend hostnames + one dead hostname — exactly the `0065-weighted-clusters` split (STRICT_DNS reference at `driver.go:181-201`, STATIC subject at `:254+`). The driver: (1) poll `/stats` on BOTH sides until `cluster.c_hc.membership_healthy == 2` (30s deadline); (2) send N=100 requests; (3) assert 100% served by the 2 live backends (the dead host filtered) on BOTH sides; (4) `StatsAsserter`: cross-side `health_check.{attempt,success,failure}` + `membership_healthy == 2` + `upstream_rq_total` cross-equal. Mirror `test/fixtures/0065-weighted-clusters/driver/driver.go` structure (the two-bootstrap split included).

- [ ] **Step 2: Run the fixture (subject + reference)**

Run: `cd test/differential && go test -run 'TestDifferential/0066' -count=1 -v` (uses the Docker bridge per `reference_docker_probe_bridge_network`).
Expected: PASS both sides.

- [ ] **Step 3: README.md** — the fixture intent + the deliberate-break table (Task 13).
- [ ] **Step 4: Commit** — `git commit -m "phase 39.1 Task 12: 0066-health-check-http poll-to-converge differential"`

---

### Task 13: Deliberate-break liveness + ≥20-run flake

**Files:** (no production change — temporary edits reverted)

- [ ] **Step 1: Break (a)** — drop the health filter (force `rr.health = nil` path or skip `isHealthy`): rebuild, run `-count=1` → the 100%-live assertion FAILS (traffic leaks to the dead host). Revert.
- [ ] **Step 2: Break (b)** — invert the poll predicate (`membership_healthy == 3`): run `-count=1` → the poll never converges (times out). Revert.
- [ ] **Step 3: Break (c)** — drop the `lb_healthy_panic`/`membership_healthy` registration: run `-count=1` → the StatsAsserter FAILS. Revert.
- [ ] **Step 4: Flake** — `for i in $(seq 20); do go test -run 'TestDifferential/0066' -count=1 ./test/differential/ || break; done` → 20/20 PASS. (Honor `reference_differential_break_protocol_count1` + `reference_differential_run_selector`.)
- [ ] **Step 5: Commit** — record the break table in `0066/README.md`; `git commit -m "phase 39.1 Task 13: 0066 deliberate-break liveness + 20/20 flake"`

---

### Task 14: Full differential re-verify gate

- [ ] **Step 1: Full suite** — `cd test/differential && go test -count=1 ./...` → all 68 dirs PASS (the existing 67 byte-identical — health-aware pick is inert when no `health_checks`; the new `0066`).
- [ ] **Step 2: Six-gate** — `gofmt -l internal/ cmd/`; `golangci-lint run ./...`; `go build ./...`; `go mod tidy -diff` (EMPTY — AMEND-HC8); `go test ./... -count=1`; `go test -race -short ./...`.
- [ ] **Step 3: Conformance** — h2spec 53/53 + proxy-wasm 10/10 asserted-unaffected (change-scope: `internal/cluster/*` + `cmd/envoy-go/main.go` + the `0066` fixture — no HTTP framing / wasm path).
- [ ] **Step 4: Commit** (if any gofmt/lint drift fixed) — `git commit -m "phase 39.1 Task 14: full 68-dir differential + six-gate green"`

---

### Task 15: ADR bodies + BEHAVIOR_CONTRACT + DECISIONS

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0242 + ADR-0243 §Decision/§Consequences bodies, in-place per ADR-0044; tail ADR-0241 → ADR-0243, next-free ADR-0244)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (the `### Cluster — active health checks` subsection + the stat-surface 1125 → 1132 note + the §2 deferred + the tcp/grpc-39.1 departure records)

- [ ] **Step 1: Write ADR-0242 + ADR-0243 bodies** (promote the SPEC §13 §Context drafts; add §Decision/§Consequences).
- [ ] **Step 2: Write the BEHAVIOR_CONTRACT delta** (the host health-state dimension; the HTTP checker; the health-aware pick + panic; the §6 rejects; the +7 stats; the deferred departures).
- [ ] **Step 3: Commit** — `git commit -m "phase 39.1 Task 15: ADR-0242/0243 bodies + BEHAVIOR_CONTRACT delta"`

---

### Task 16: Completion bundle (counts + STATE/ROADMAP/README/PROGRESS)

**Files:**
- Modify: `docs/envoy-go/STATE.md` (active-phase → phase 39.1 IMPL done), `ROADMAP.md` (row 39 39.1-leg `in-progress → done`), `docs/envoy-go/phases/39-upstream-health-check/README.md` (status 2 → 3/4), `PROGRESS.md` (all tasks done + six-gate evidence + as-built counts)

- [ ] **Step 1: Verify the as-built counts** — fixtures 67 → 68; fuzzers 42; stat surface 1125 → 1132 (+7); BackendKind 33; DECISIONS tail ADR-0241 → ADR-0243.
```bash
echo "fixtures: $(ls -d test/fixtures/[0-9]* | wc -l)"  # 68
grep -c '^## ADR-' docs/envoy-go/DECISIONS.md            # tail ADR-0243
```
- [ ] **Step 2: Update the bundle** (STATE/ROADMAP/README/PROGRESS) — row 39 (39.1 leg) flips `in-progress → done`; the family stays OPEN (39.2 TCP+gRPC pending; 4 family candidates after 39).
- [ ] **Step 3: Final six-gate re-run** (ADR-0052 atomic landing) + commit — `git commit -m "phase 39.1 Task 16: completion bundle (fixtures 68, stat 1132, ADR-0243); row 39.1 done"`

---

## Notes for the executor

- **Work in a fresh worktree** (`feedback_git_worktrees`); subagents commit LOCAL-only (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Pin the canonical paths (`feedback_subagent_worktree_path_targeting`/`_detach`).
- **Per-task `gofmt -l` + `golangci-lint`** on touched packages (`feedback_pertask_gofmt_lint`) — not just `go vet`.
- **Behavior-neutrality is the safety net:** nil `health` (every existing fixture) MUST be byte-identical. Task 7 lands the field inert; Tasks 8–9 only branch when `health != nil`. The full 68-dir differential (Task 14) is the real guard.
- **`-count=1` everywhere** for break/flake (`reference_differential_break_protocol_count1`); `-run 'TestDifferential/0066'` (`reference_differential_run_selector`); single-source the `0066` constants (`reference_fixture_workload_constant_desync`).
- **The first-check / no_traffic_interval timing:** the dead host fails its first probe at startup → converges fast; the driver poll-to-converges (no fixed sleep). If `membership_healthy` stalls, check the checker goroutine started (post-`Freeze`) and the probe path/`expected_statuses`.
- **gRPC dep (39.2, not now):** `google.golang.org/grpc v1.70.0` is already a direct dep → 39.2 imports `grpc_health_v1` directly (AMEND-HC8).
