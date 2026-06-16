# Phase 40.1 Implementation Plan — `outlier_detection.consecutive_5xx`

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land passive upstream health for the single `consecutive_5xx` detector — a host returning N consecutive 5xx on real traffic is ejected from the LB-eligible set for `base_ejection_time`, then lazily returned — over the phase-39 `clusterHealth`/`hostHealth` substrate, byte-identical when no `outlier_detection` is configured.

**Architecture:** A per-request `cluster.RecordUpstreamResult(ep, UpstreamResult)` seam fired from the two LIVE router drivers (`doH1ClusterAction`/`doH2ClusterAction`) feeds an `outlierDetector` that maintains a SEPARATE ejection sub-state on the existing `hostHealth` (distinct from the active-HC `healthy` bit; time-based lazy recovery). A new `available = isHealthy && !isEjected` LB-pick predicate replaces the five leaf `isHealthy` consult sites; the panic denominator moves to a new `availableCount` helper while `membership_healthy` stays active-HC-only. When no `outlier_detection` is set, `isEjected` is always false ⇒ `available == isHealthy`, `availableCount == healthyCount` ⇒ every existing fixture stays byte-identical.

**Tech Stack:** Go; `github.com/envoyproxy/go-control-plane` v1.32.4 (`cluster.v3.OutlierDetection`, Cluster field 19 — already vendored, ZERO new go.mod modules); the internal `stats` registry; the differential harness (`contrib-v1.37.2`, Docker bridge, poll-to-converge + warmup + delta counters).

**Reference docs (read alongside this plan):**
- SPEC: `docs/envoy-go/phases/40.1-outlier-detection-consecutive-5xx/SPEC.md` (the §1.1 AMEND-OD1..OD6 amendments; §3 architecture; §5 proto roster; §6 reject roster; §7 stat surface; §8 fixture; §11 D-OD-* live pins; §13 ADR-0245 §Context).
- BRAINSTORM: `docs/envoy-go/phases/40-outlier-detection/BRAINSTORM.md` (parent charter; the 3-leg split).

---

## D-question resolutions (SPEC §12) — settled at PLAN

These were the open PLAN/IMPL D-questions in SPEC §12. The implementer MUST follow these resolutions (they are baked into the tasks below).

### D-S40.1-1 — house reject wording (§6)
Mirror the `parseHealthChecks` precedent (`cluster: %q: health_check: <reason>`). The four 40.1 reject arms use the prefix `cluster: %q: outlier_detection: ` + the reason (the reason text mirrors the reference PGV wording verbatim where it exists, per ADR-0080):
- `cluster: %q: outlier_detection: max_ejection_percent: value must be less than or equal to 100`
- `cluster: %q: outlier_detection: enforcing_consecutive_5xx: value must be less than or equal to 100`
- `cluster: %q: outlier_detection: interval: value must be greater than 0s`
- `cluster: %q: outlier_detection: base_ejection_time: value must be greater than 0s`

`interval`/`base_ejection_time` are only validated WHEN SET (the proto fields are `*durationpb.Duration`; nil ⇒ default applies, no reject). `max_ejection_percent`/`enforcing_consecutive_5xx` are `*wrapperspb.UInt32Value`; validate only when non-nil.

### D-S40.1-2 — `consecutive_5xx == 0` / absent disposition
The proto field is `*wrapperspb.UInt32Value` so absent (nil) and explicit-0 are distinguishable. Follow the reference: **field absent (nil) ⇒ default threshold 5; explicit value 0 ⇒ detector OFF**. Concretely in `parseOutlierDetection`: `if od.GetConsecutive_5Xx() == nil { threshold = 5 } else { threshold = od.GetConsecutive_5Xx().GetValue() }`; a `threshold == 0` records a per-cluster `consec5xxEnabled = false` (the detector's 5xx arm is skipped — no eject ever). Unit-tested in Task 5.

### D-S40.1-3 — `enforcing_consecutive_5xx` roll mechanism
A single uniform "enforce roll": eject (enforce) iff the detector's `enforceRoll() < enforcing_consecutive_5xx`, where `enforceRoll()` returns a uniform value in `[0,100)`. This naturally covers the boundaries: `enforcing == 100` ⇒ roll (0..99) always `< 100` ⇒ always enforce; `enforcing == 0` ⇒ roll never `< 0` ⇒ never enforce (detect-only). For the common deterministic cases (0 and ≥100) the detector SHORT-CIRCUITS without consuming the rng (so `0069` with `enforcing` defaulted to 100 is fully deterministic): `if enforcing >= 100 { enforce = true } else if enforcing == 0 { enforce = false } else { enforce = enforceRoll() < enforcing }`. `enforceRoll` is a field on the detector seeded from `newPCGRNG` (the `randomLB`/maglev precedent), INJECTABLE in unit tests (a deterministic stub) so the intermediate-percentage path is testable. A detect-without-enforce still increments `ejections_detected_consecutive_5xx` but NOT the enforced counters (matches the reference detect-only behavior).

### D-S40.1-4 — `0069` constants single-sourced
All workload constants live in one `const`/`var` block at the top of the `0069` driver (the `0066` precedent, `reference_fixture_workload_constant_desync`): `fixtureName`, `refContainerListenerPort = 19158` (next free; 0068 took 19157), `refAdminPort = 9901`, `healthyBackendCount = 2`, `consec5xxThreshold = 5`, `baseEjectionTime`, `interval`, `maxEjectionPercent = 100`, `n` (load-phase request count), `convergeDeadline`/`convergePoll`, `warmupStable`. The stats assertions and the bootstrap/config builders read these — no hand-rolled duplicate counts.

### D-S40.1-5 — `0069` mixed topology + the `HTTP503Responder` BackendKind  ★ load-bearing
The runner spawns a **single uniform `BackendKind()` for all `BackendCount()` backends** (runner_test.go:178 reads `kind` ONCE before the spawn loop). `0069` needs a mixed `{2×HTTPEcho healthy, 1×always-503}` cluster. Resolution — **honor the SPEC's +1 BackendKind (tail 34 → 35) via a small, general, additive runner capability**: a new OPTIONAL fixture interface
```go
// PerHostBackendKind lets a driver override the backend kind per host index.
// Drivers that do NOT implement it get the uniform BackendKind() for every host.
type PerHostBackendKind interface {
    BackendKindAt(i int) BackendKind
}
```
consulted INSIDE the runner spawn loop (per-index), defaulting to the existing uniform `BackendKind()` when the driver does not implement it (byte-stable for all 70 existing fixtures — none implement it). `0069` implements `BackendKindAt`: hosts 0,1 ⇒ `HTTPEcho`, host 2 ⇒ `HTTP503Responder`, `BackendCount() == 3`. The new `HTTP503Responder` BackendKind 35 gets a runner spawn case `acceptHTTP503Counting(ln, idx)` modeled on `acceptHTTPEchoCounting` (same `backend-<idx>:<seg>` body for host attribution; status line `HTTP/1.1 503 Service Unavailable`). This keeps every SPEC exit-count exact AND routes 0069's 503 host through the standard runner spawn.

**Rejected alternative** (driver-self-spawns the 503 host, the 0066 `deadPort` precedent): zero runner change, but it would make the SPEC's `HTTP503Responder` BackendKind vestigial and break the BackendKind tail 34 → 35 exit count. The per-index capability is preferred because it honors the reviewed SPEC's counts and is reusable by future mixed-topology fixtures.

### D-S40.1-6 — final ADR-0045 split-gate re-check
Anticipated production LoC (below) ≈ **~270–360 LoC** across ~6 prod files; ~14 tasks. Both are comfortably under the ADR-0045 split gate (`> ~25 tasks OR > ~1500 LoC`). **NO FURTHER SPLIT** — 40.1 ships as one flat leg. (40.2/40.3 remain pre-authorized separate legs per the BRAINSTORM.)

---

## File structure

**Production (`internal/`):**
- `internal/cluster/health.go` (MODIFY) — add the 4 ejection fields to `hostHealth`; add `isEjected`/`available`/`availableCount` to `clusterHealth`; split `inPanic` denominator → `availableCount`; thread the optional outlier-stat handles + `now func() time.Time` injection point.
- `internal/cluster/outlier.go` (CREATE) — `outlierConfig` (parsed) + `parseOutlierDetection` + the `outlierDetector` (the `consecutive_5xx` detect/eject/lazy-uneject/cap/enforce-roll logic) + the +5 stat handles.
- `internal/cluster/loadbalancer.go`, `random.go`, `ringhash.go`, `maglev.go`, `leastrequest.go` (MODIFY) — the five leaf `isHealthy → available` consult-site edits.
- `internal/cluster/cluster.go` (MODIFY) — the `outlier *outlierDetector` field on `Cluster` + `UpstreamResult` + `RecordUpstreamResult` (no-op when `outlier == nil`).
- `internal/cluster/manager.go` (MODIFY) — `parseOutlierDetection` call + widen the `clusterHealth`-creation condition to `health_checks OR outlier_detection`; register the +5 stats (in `registerClusterMetrics`); construct + attach the detector.
- `internal/filter/http/router/router.go` (MODIFY, ~:655) + `router_h2.go` (MODIFY, ~:100) — the two seam call sites.

**Test harness (`test/`):**
- `test/differential/fixture/fixture.go` (MODIFY) — `HTTP503Responder BackendKind = 35`; the `PerHostBackendKind` optional interface.
- `test/differential/runner_test.go` (MODIFY) — consult `BackendKindAt` per-index in the spawn loop; the `case fixture.HTTP503Responder` spawn arm; `acceptHTTP503Counting`.
- `test/fixtures/0069-outlier-detection-consecutive-5xx/driver/driver.go` + `driver_test.go` (CREATE) + `expectations.yaml` + `README.md` (CREATE).

**Docs:**
- `docs/envoy-go/DECISIONS.md` (ADR-0245 body), `BEHAVIOR_CONTRACT.md` (passive-health subsection + stat-count 1132 → 1137), `STATE.md`, `ROADMAP.md`, `next-prompt.txt`, the phase `PROGRESS.md` + `README.md`.

---

## Task 1: Baselines + PROGRESS scaffold

**Files:**
- Create: `docs/envoy-go/phases/40.1-outlier-detection-consecutive-5xx/PROGRESS.md`

- [ ] **Step 1: Capture the pre-IMPL six-gate baseline.** Run, and record the outputs in PROGRESS.md:
  - `go build ./...`
  - `go vet ./...`
  - `gofmt -l internal/ test/` (expect empty)
  - `go test ./internal/... 2>&1 | tail -20`
  - `go test ./test/differential/ -count=1 2>&1 | tail -20` (the full 70-dir suite — the byte-stability anchor)
  - Stat-surface count: this project tracks the stat surface as a **documented running total carried forward per phase** (39.2 PROGRESS: "stat surface 1132 → 1132 UNCHANGED"), NOT a runnable command. Record the current total **1132** (the SPEC §14 baseline). The 40.1 exit total is verified **arithmetically (1132 + 5 = 1137)** against the Task 8 registration test (which asserts exactly the 5 new named stats are registered) — there is no count-script to run.
- [ ] **Step 2: Record the baselines + the task checklist** in PROGRESS.md (counts: stat 1132 / fixtures 70 / fuzzers 42 / BackendKind tail 34 / DECISIONS tail ADR-0244, next-free ADR-0245; the anticipated exit deltas from SPEC §14).
- [ ] **Step 3: Commit.**
```bash
git add docs/envoy-go/phases/40.1-outlier-detection-consecutive-5xx/PROGRESS.md
git commit -m "phase 40.1 Task 1: PROGRESS scaffold + pre-IMPL baselines"
```

---

## Task 2: Ejection dimension on `hostHealth` + `isEjected`/`available`/`availableCount`

**Files:**
- Modify: `internal/cluster/health.go`
- Test: `internal/cluster/health_test.go` (or the existing health test file — match the package's test layout)

The new state (DISTINCT from the active-HC `healthy` bit, SPEC §3.2):
```go
// On hostHealth:
ejected        atomic.Bool   // outlier-ejected (separate from healthy)
unejectAtNanos atomic.Int64  // un-eject deadline (UnixNano); 0 when not ejected
ejectCount     atomic.Uint32 // ejections so far (stats + future backoff)
consec5xx      atomic.Uint32 // consecutive 5xx (reset by a 2xx from this host)
```
`clusterHealth` gains an injectable clock + the ejections_active gauge handle (nil-guarded), plus the three methods. The lazy un-eject lives in `isEjected` (SPEC §3.2 — the load-bearing un-eject path, since an ejected host gets no further traffic):
```go
// nowNanos is injectable for deterministic tests; defaults to time.Now().UnixNano.
func (ch *clusterHealth) isEjected(ep Endpoint) bool {
    h, ok := ch.states[ep.Addr()]
    if !ok {
        return false // unknown addr → not ejected (mirrors isHealthy's unknown→true)
    }
    if !h.ejected.Load() {
        return false
    }
    if ch.nowNanos() >= h.unejectAtNanos.Load() {
        // lazy un-eject: clear + decrement the active gauge
        if h.ejected.CompareAndSwap(true, false) && ch.ejectionsActive != nil {
            ch.ejectionsActive.Dec() // or Add(-1) per the stats.Gauge API
        }
        return false
    }
    return true
}

func (ch *clusterHealth) available(ep Endpoint) bool { return ch.isHealthy(ep) && !ch.isEjected(ep) }

func (ch *clusterHealth) availableCount(eps []Endpoint) int {
    n := 0
    for _, ep := range eps {
        if ch.available(ep) {
            n++
        }
    }
    return n
}
```
NOTE: confirm the `stats.Gauge` decrement API (Task 8 wires the real gauge; here the field can be nil and the CAS-guarded decrement must nil-check). Use a `nowNanos func() int64` field on `clusterHealth` defaulting to `func() int64 { return time.Now().UnixNano() }`, set in `newClusterHealth`.

- [ ] **Step 1: Write failing unit tests** in the cluster test package:
  - `TestIsEjected_UnknownAddr` → false.
  - `TestEject_ThenAvailableFalse`: set `ejected=true`, `unejectAtNanos = now+1h` (via the injected clock); `available(ep)` is false; `availableCount` excludes it.
  - `TestLazyUneject`: eject with `unejectAtNanos = now+30s`; advance the injected clock past it; `isEjected` returns false AND clears `ejected` AND (with a stub gauge) decrements once; a second call does not double-decrement (CAS guard).
  - `TestAvailable_NoOutlier`: with no ejection, `available == isHealthy` and `availableCount == healthyCount` (the byte-stable identity).
- [ ] **Step 2: Run → FAIL** (`go test ./internal/cluster/ -run 'TestIsEjected|TestEject|TestLazyUneject|TestAvailable' -v`). Expected: undefined methods/fields.
- [ ] **Step 3: Implement** the fields + the three methods + the injectable clock in `health.go`.
- [ ] **Step 4: Run → PASS.**
- [ ] **Step 5:** `gofmt -l internal/cluster/ && go vet ./internal/cluster/ && golangci-lint run internal/cluster/...` (per `feedback_pertask_gofmt_lint`).
- [ ] **Step 6: Commit** (`phase 40.1 Task 2: ejection dimension on hostHealth + isEjected/available/availableCount`).

---

## Task 3: Move the five leaf LB consult sites `isHealthy → available` + panic denominator → `availableCount` (byte-stability gate)

**Files:**
- Modify: `internal/cluster/loadbalancer.go:61`, `random.go:55`, `ringhash.go:153`, `maglev.go:60`, `leastrequest.go:104` (the per-pick `isHealthy(ep)` calls → `available(ep)`).
- Modify: `internal/cluster/health.go` — `inPanic` denominator `healthyCount → availableCount` (SPEC §3.2); the `membershipHealthy` gauge in `recomputeMembership` STAYS `healthyCount` (active-HC-only).

This is a pure refactor with NO behavior change when no `outlier_detection` is configured (then `available == isHealthy`, `availableCount == healthyCount`). The proof is the full differential suite staying green.

- [ ] **Step 1:** Edit the five leaf `isHealthy(...)` per-pick filter calls to `available(...)`. Leave the `inPanic`/`panicInc` calls as-is at this step EXCEPT change `inPanic`'s internal `healthyCount` → `availableCount` in `health.go`. Do NOT touch `recomputeMembership` (gauge stays `healthyCount`).
- [ ] **Step 2: Run the existing cluster unit tests → PASS unchanged** (`go test ./internal/cluster/ -count=1`). The health-aware LB tests must still pass (with no ejection, `available == isHealthy`).
- [ ] **Step 3: Byte-stability gate — run the FULL differential suite → all 70 GREEN** (`go test ./test/differential/ -count=1`). This is the load-bearing proof that the predicate move is byte-identical. If any fixture flips, STOP and diagnose (a missed `available` vs `isHealthy` asymmetry).
- [ ] **Step 4:** `gofmt -l` + `go vet` + `golangci-lint` on `internal/cluster/...`.
- [ ] **Step 5: Commit** (`phase 40.1 Task 3: move LB pick filter isHealthy→available + panic denominator→availableCount (byte-stable; full 70-dir green)`).

---

## Task 4: `parseOutlierDetection` + the reject roster + registry-creation widening

**Files:**
- Create: `internal/cluster/outlier.go` (the `outlierConfig` struct + `parseOutlierDetection`).
- Modify: `internal/cluster/manager.go` — call `parseOutlierDetection`; widen the `clusterHealth`-creation condition.
- Test: `internal/cluster/outlier_test.go`

The parsed config (consumed by Task 5's detector):
```go
type outlierConfig struct {
    consec5xxEnabled  bool          // false when threshold==0 (D-S40.1-2)
    consecutive5xx    uint32        // threshold (default 5 when field absent)
    baseEjectionTime  time.Duration // default 30s (flat; AMEND-OD2)
    maxEjectionPct    uint32        // default 10 (AMEND-OD3)
    enforcing5xx      uint32        // default 100
    // interval parse-accepted + validated (gt:0s) but role deferred (AMEND-OD1)
}

// parseOutlierDetection returns (nil, nil) when c has no outlier_detection.
// Byte-stable rejects per ADR-0080 (D-S40.1-1 wording).
func parseOutlierDetection(c *clusterv3.Cluster, name string) (*outlierConfig, error)
```
Defaults (SPEC §5, confirmed live): `consecutive_5xx` 5, `interval` 10s, `base_ejection_time` 30s, `max_ejection_percent` 10, `enforcing_consecutive_5xx` 100. Use `GetX() == nil` to distinguish absent-from-explicit for the wrapper/duration fields. All OTHER `outlier_detection` fields are parse-accepted-and-ignored (silent, additive — SPEC §5).

Registry widening in `manager.go` (currently `manager.go:370-373`):
```go
outlierCfg, err := parseOutlierDetection(c, name)
if err != nil { return nil, err }
var health *clusterHealth
if len(hcSpecs) > 0 || outlierCfg != nil {
    health = newClusterHealth(endpoints, parsePanicThreshold(c))
}
```

- [ ] **Step 1: Write failing tests** (`outlier_test.go`): no `outlier_detection` ⇒ `(nil, nil)`; full config parsed with correct defaults; absent `consecutive_5xx` ⇒ threshold 5 + enabled; explicit 0 ⇒ `consec5xxEnabled == false`; the FOUR reject arms (D-S40.1-1) each produce the exact house string (`max_ejection_percent > 100`, `enforcing_consecutive_5xx > 100`, `interval <= 0s`, `base_ejection_time <= 0s`). Use the `parseHealthChecks` reject-test pattern.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `parseOutlierDetection` + the manager widening. Add the manager test (a cluster with only `outlier_detection` now builds a non-nil `health`).
- [ ] **Step 4: Run → PASS** + full `go test ./internal/cluster/ -count=1` green.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 40.1 Task 4: parseOutlierDetection + reject roster + clusterHealth-creation widening`).

---

## Task 5: The `outlierDetector` — `consecutive_5xx` detect / eject / lazy-uneject / cap / enforce-roll

**Files:**
- Modify: `internal/cluster/outlier.go` (the detector type + logic).
- Test: `internal/cluster/outlier_test.go`

```go
type outlierDetector struct {
    cfg        outlierConfig
    health     *clusterHealth // shared registry (ejection lives on hostHealth)
    endpoints  []Endpoint     // for the max_ejection_percent denominator
    enforceRoll func() uint32 // [0,100); injectable; default PCG-seeded
    // +5 stat handles (Task 8 injects; nil-guarded here):
    ejectionsActive            *stats.Gauge
    ejectionsEnforcedTotal     *stats.Counter
    ejectionsOverflow          *stats.Counter
    ejectionsDetected5xx       *stats.Counter
    ejectionsEnforced5xx       *stats.Counter
}

// record applies one upstream result for ep (SPEC §3.3). Called from
// RecordUpstreamResult under the cluster's normal concurrency (per-host
// atomics; the eject decision reads availableCount which is a racy snapshot —
// acceptable, mirrors the reference's best-effort cap).
func (d *outlierDetector) record(ep Endpoint, statusCode int) {
    h, ok := d.health.states[ep.Addr()]
    if !ok { return }
    _ = d.health.isEjected(ep) // fast-path lazy-uneject refresh (not load-bearing; §3.3 step 1)
    is5xx := statusCode >= 500 && statusCode < 600
    if !is5xx {
        h.consec5xx.Store(0) // reset on any non-5xx FROM THIS HOST
        return
    }
    if !d.cfg.consec5xxEnabled { return }
    n := h.consec5xx.Add(1)
    if n < d.cfg.consecutive5xx { return }
    // threshold reached
    if d.ejectionsDetected5xx != nil { d.ejectionsDetected5xx.Inc() }
    if h.ejected.Load() { return } // already ejected
    // enforce roll (D-S40.1-3)
    enforce := d.cfg.enforcing5xx >= 100 || (d.cfg.enforcing5xx != 0 && d.enforceRoll() < d.cfg.enforcing5xx)
    if !enforce { return } // detect-only
    // max_ejection_percent cap (AMEND-OD3): eject iff (ejected+1)*100/total <= cap.
    // ejectedCount(eps) is a new clusterHealth helper added in this task (counts isEjected).
    total := len(d.endpoints)
    if total == 0 || (d.health.ejectedCount(d.endpoints)+1)*100/total > int(d.cfg.maxEjectionPct) {
        if d.ejectionsOverflow != nil { d.ejectionsOverflow.Inc() }
        return
    }
    // eject — CAS the ejected flag so exactly one concurrent goroutine wins the
    // eject + the gauge/counter increments (the consec5xx.Add/threshold check
    // above is not atomic as a unit; a Load-then-Store guard would let two
    // threshold-crossing 5xx both eject + double-increment). (concurrency note below)
    if !h.ejected.CompareAndSwap(false, true) {
        return // another goroutine already ejected this host
    }
    h.unejectAtNanos.Store(d.health.nowNanos() + d.cfg.baseEjectionTime.Nanoseconds())
    h.ejectCount.Add(1)
    if d.ejectionsActive != nil { d.ejectionsActive.Inc() }
    if d.ejectionsEnforcedTotal != nil { d.ejectionsEnforcedTotal.Inc() }   // the double-count
    if d.ejectionsEnforced5xx != nil { d.ejectionsEnforced5xx.Inc() }       //  (AMEND-OD4)
}
```
NOTE — the `h.ejected.Load()` already-ejected guard EARLIER in `record` (before the detected counter) stays as a cheap fast-path; the CAS above is the authoritative once-only gate. The `ejectedCount` cap read is a racy snapshot under concurrency (acceptable — mirrors the reference's best-effort cap; the CAS makes the eject itself exactly-once). Boundary pinned live: 1-of-3 = 33.33% ejects iff cap ≥ 34; cap 33 ⇒ overflow.

- [ ] **Step 1: Write failing unit tests** (deterministic clock + injected `enforceRoll`):
  - eject after exactly N consecutive 5xx (N-1 does not eject); `ejectionsActive`/`ejectionsEnforcedTotal`/`ejectionsEnforced5xx`/`ejectionsDetected5xx` all incremented on the ejecting result (the double-count).
  - a 2xx mid-streak resets `consec5xx` (no eject).
  - `consec5xxEnabled == false` (threshold 0) ⇒ never ejects even on many 5xx.
  - cap overflow: total 3, cap 33 ⇒ the would-be ejection is blocked, `ejectionsOverflow++`, `ejectionsDetected5xx++`, NO `ejected`, NO enforced counters; cap 34 ⇒ ejects.
  - enforce roll: `enforcing 50` with a stub roll returning 49 ⇒ enforce; returning 50 ⇒ detect-only (`ejectionsDetected5xx++` only). `enforcing 0` ⇒ never enforce; `enforcing 100` ⇒ always (roll not consumed).
  - already-ejected host: a further 5xx does not re-eject / re-increment enforced counters.
  - (optional, race-documenting) concurrent eject: two goroutines crossing the threshold for the same host eject it exactly once (the CAS gate) — gauge/enforced counters increment once, not twice.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the detector + the `ejectedCount` helper.
- [ ] **Step 4: Run → PASS.**
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 40.1 Task 5: outlierDetector consecutive_5xx detect/eject/cap/enforce-roll + unit tests`).

---

## Task 6: `RecordUpstreamResult` seam on `Cluster` (no-op when no detector)

**Files:**
- Modify: `internal/cluster/cluster.go` — the `outlier *outlierDetector` field + `UpstreamResult` + `RecordUpstreamResult`.
- Modify: `internal/cluster/manager.go` — construct the detector (when `outlierCfg != nil`) and attach it to the `Cluster`.
- Test: `internal/cluster/cluster_test.go`

```go
// UpstreamResult is one completed upstream request's outcome (SPEC §3.1).
// LocalOriginErr is unread at 40.1 (reserved for 40.2). (ADR-0245)
type UpstreamResult struct {
    StatusCode     int
    LocalOriginErr bool
}

// RecordUpstreamResult feeds one request outcome to the cluster's outlier
// detector. A no-op for clusters without outlier_detection. (ADR-0245)
func (c *Cluster) RecordUpstreamResult(ep Endpoint, r UpstreamResult) {
    if c.outlier == nil {
        return
    }
    c.outlier.record(ep, r.StatusCode)
}
```
Manager construction: after building `health`, when `outlierCfg != nil`, build the detector over `endpoints` + `health`, seed `enforceRoll` from `newPCGRNG`, and set `cl.outlier`. (Stat handles wired in Task 8.) `newPCGRNG()` returns `(func() uint64, error)` (leastrequest.go:66); adapt it to the `[0,100)` `uint32` roll and handle the seed error like maglev (maglev.go:70):
```go
rng, err := newPCGRNG()
if err != nil { return nil, err } // mirror maglev/random: no wrap
det.enforceRoll = func() uint32 { return uint32(rng() % 100) }
```

- [ ] **Step 1: Write failing test:** a `Cluster` with `outlier == nil` ⇒ `RecordUpstreamResult` is a no-op (no panic, no state change); a `Cluster` with a detector ⇒ N consecutive 5xx via `RecordUpstreamResult` ejects the host (`available(ep)` becomes false) — an end-to-end seam test through the public method.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the field + method + manager construction.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 40.1 Task 6: RecordUpstreamResult seam + detector construction`).

---

## Task 7: The two LIVE router call sites

**Files:**
- Modify: `internal/filter/http/router/router.go` (beside `a.cluster.IncStatusClass(resp.StatusCode)` at ~:655).
- Modify: `internal/filter/http/router/router_h2.go` (beside `a.cluster.IncStatusClass(resp.Status)` at ~:100).

★ **Dead-driver trap** (`reference_close_direction_framework_gap` sibling; SPEC AMEND-OD5): the seam goes in the LIVE drivers `doH1ClusterAction` / `doH2ClusterAction` ONLY — NOT the test-only legacy `(*routerAction).do` / `(*routerActionH2).doH2`. At each site `picked` is guaranteed non-zero (it was set from the successful acquire/dial). Add immediately AFTER the existing `IncStatusClass`:
```go
// router.go (doH1ClusterAction, after IncStatusClass(resp.StatusCode)):
a.cluster.RecordUpstreamResult(picked, cluster.UpstreamResult{StatusCode: resp.StatusCode})
// router_h2.go (doH2ClusterAction, after IncStatusClass(resp.Status)):
a.cluster.RecordUpstreamResult(picked, cluster.UpstreamResult{StatusCode: resp.Status})
```
The pre-pick dial/acquire-failure paths (H1 503 `:610-613`, H2 502 `:76-79`) have `picked == Endpoint{}` and are the 40.2 local-origin outcomes — they do NOT call the seam at 40.1. Weighted-cluster routes reuse `do*ClusterAction` → covered for free.

- [ ] **Step 1:** Add the two call sites. (No new unit test here — the seam's behavior is covered by Task 6's end-to-end test + the 0069 fixture's live-traffic ejection in Tasks 10–11, which is also the seam-liveness proof per the dead-driver trap.)
- [ ] **Step 2: Run** `go build ./...` + `go test ./internal/filter/http/router/ -count=1` → PASS.
- [ ] **Step 3:** gofmt/vet/lint on the router package.
- [ ] **Step 4: Commit** (`phase 40.1 Task 7: wire RecordUpstreamResult into the live H1/H2 router success sites`).

---

## Task 8: The +5 stat registrations (scoped to outlier clusters)

**Files:**
- Modify: `internal/cluster/manager.go` (`registerClusterMetrics`, ~:154-161) — register the +5 stats ONLY when the cluster has a detector; inject the handles into both the detector and the `clusterHealth` (the `ejectionsActive` gauge is read by `isEjected`'s lazy un-eject).
- Modify: `internal/cluster/outlier.go` if a small `setStats(...)` helper is cleaner.

Scoped `cluster.<name>.outlier_detection.` (SPEC §7):
1. `ejections_active` — **gauge** (injected into BOTH the detector AND `clusterHealth.ejectionsActive`).
2. `ejections_enforced_total` — counter.
3. `ejections_overflow` — counter.
4. `ejections_detected_consecutive_5xx` — counter.
5. `ejections_enforced_consecutive_5xx` — counter.

Confirm the exact registry prefix from how `health_check.*` stats are scoped (`registerStats(r, prefix)` precedent). Existing fixtures (no detector) register NOTHING new ⇒ stat surface unchanged for them.

- [ ] **Step 1: Write a failing test** asserting that a cluster WITH `outlier_detection` registers exactly these 5 named stats (and a cluster WITHOUT registers none of them). Use the registry-introspection pattern from the health-check stat test.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the conditional registration + handle injection (detector + clusterHealth gauge).
- [ ] **Step 4: Run → PASS** + full `go test ./internal/... -count=1`.
- [ ] **Step 5: Stat-surface count** → expect **1137** (1132 + 5). Record in PROGRESS.
- [ ] **Step 6:** gofmt/vet/lint.
- [ ] **Step 7: Commit** (`phase 40.1 Task 8: +5 outlier_detection stat registrations (1132→1137)`).

---

## Task 9: `HTTP503Responder` BackendKind 35 + per-host kind override (runner)

**Files:**
- Modify: `test/differential/fixture/fixture.go` — `HTTP503Responder BackendKind = 35` (with the doc-comment in the existing style) + the `PerHostBackendKind` optional interface (D-S40.1-5).
- Modify: `test/differential/runner_test.go` — consult `BackendKindAt(i)` per-index in the spawn loop (default to uniform `BackendKind()`); the `case fixture.HTTP503Responder` spawn arm; `acceptHTTP503Counting(ln, idx)`.

`acceptHTTP503Counting` mirrors `acceptHTTPEchoCounting` (runner_test.go:1515) but writes the 503 status line. NOTE the signature has **no `*atomic.Uint64` accept counter** — host attribution is via the `backend-<idx>:` body (the `serveGRPCHealth(ln, bo.idx)` precedent at runner_test.go:946, which also drops the counter); the spawn arm calls `go acceptHTTP503Counting(ln, bo.idx)`:
```go
func acceptHTTP503Counting(ln net.Listener, idx int) {
    for {
        c, err := ln.Accept()
        if err != nil { return }
        go func(c net.Conn) {
            defer func() { _ = c.Close() }()
            br := bufio.NewReader(c)
            req, err := http.ReadRequest(br)
            if err != nil { return }
            _, _ = io.Copy(io.Discard, req.Body); _ = req.Body.Close()
            seg := req.URL.Path
            if i := strings.LastIndex(seg, "/"); i >= 0 && i+1 < len(seg) { seg = seg[i+1:] }
            body := fmt.Sprintf("backend-%d:%s", idx, seg)
            _, _ = fmt.Fprintf(c, "HTTP/1.1 503 Service Unavailable\r\nContent-Type: text/plain\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
        }(c)
    }
}
```
Per-index spawn dispatch (inside the runner spawn loop, replacing the single pre-loop `kind`):
```go
kind := uniformKind // = bk.BackendKind() or the TCPEcho default
if pk, ok := drv.(fixture.PerHostBackendKind); ok { kind = pk.BackendKindAt(i) }
switch kind { ... case fixture.HTTP503Responder: /* listen + go acceptHTTP503Counting(ln, bo.idx) */ }
```

- [ ] **Step 1:** Add the BackendKind constant + the interface + the spawn arm + the helper. (No standalone unit test — exercised by 0069 in Task 10; a `go build ./...` + `go vet ./test/...` gate suffices here. Optionally a tiny test that `acceptHTTP503Counting` returns 503 + the attributed body over a loopback listener.)
- [ ] **Step 2: Run** `go build ./... && go vet ./test/... && go test ./test/differential/ -count=1` → all 70 still GREEN (byte-stable: no existing fixture implements `PerHostBackendKind`, so the per-index path defaults to uniform).
- [ ] **Step 3:** gofmt/vet/lint on `test/...`. Record BackendKind tail **34 → 35**.
- [ ] **Step 4: Commit** (`phase 40.1 Task 9: HTTP503Responder BackendKind 35 + per-host backend-kind override`).

---

## Task 10: The `0069` cross-side fixture

**Files:**
- Create: `test/fixtures/0069-outlier-detection-consecutive-5xx/driver/driver.go`
- Create: `test/fixtures/0069-outlier-detection-consecutive-5xx/driver/driver_test.go` (the `backendIdxFromBody` unit test, per the 0066/0067/0068 precedent — copy the per-fixture helper)
- Create: `test/fixtures/0069-outlier-detection-consecutive-5xx/expectations.yaml`
- Create: `test/fixtures/0069-outlier-detection-consecutive-5xx/README.md`

Model closely on `test/fixtures/0066-health-check-http/driver/driver.go` (the poll-to-converge + warmup + delta-counter template, `reference_health_check_propagation_warmup`). Topology: cluster `c_od`, lb ROUND_ROBIN, 3 endpoints {host0 HTTPEcho, host1 HTTPEcho, host2 HTTP503Responder}, `outlier_detection: { consecutive_5xx: 5, interval: 10s, base_ejection_time: 30s, max_ejection_percent: 100 }`, on BOTH sides. The driver implements `BackendCount() == 3` + `BackendKindAt(i)` (0,1 ⇒ HTTPEcho; 2 ⇒ HTTP503Responder). Reference side STRICT_DNS / host.docker.internal; subject side STATIC / 127.0.0.1 (the 0066 cross-side shape).

The `AssertStats` flow (holds both admin addrs):
1. **Warmup/ejection drive:** send round-robin GET / requests until the 503 host has accrued ≥`consecutive_5xx` consecutive 5xx and been ejected. Because round-robin makes the 503 host's picks deterministic, ~`consecutive_5xx × 3` requests guarantee 5 consecutive picks of host2 are impossible in pure RR (RR interleaves) — IMPORTANT: in strict RR, host2 is hit every 3rd request, so its 5xx are NOT consecutive *globally* but ARE consecutive *per-host* (the detector's `consec5xx` is per-host and only reset by a 2xx FROM host2 — which never comes). So 5 picks of host2 (≈15 RR requests) eject it. Single-source the request counts.
2. **POLL `/stats` on BOTH sides** until `cluster.c_od.outlier_detection.ejections_active == 1` (deadline `convergeDeadline`, every `convergePoll`; NO fixed sleep). Fail clearly on timeout.
3. **Measured load phase:** record the pre-load delta baseline, send `n` GET /; assert (delta) 100% `upstream_rq_2xx`, 0 `upstream_rq_5xx` (the ejected 503 host serves nothing), all served by host0/host1 (body `backend-0:`/`backend-1:`).
4. **Cross-side stat parity:** `outlier_detection.{ejections_active==1, ejections_enforced_total>=1, ejections_enforced_consecutive_5xx>=1, ejections_detected_consecutive_5xx>=1}` on BOTH sides. Do **NOT** assert `ejections_detected_consecutive_gateway_failure` (AMEND-OD4 — envoy-go has no gateway detector; the reference trips it on 503). Verify `upstream_rq_total > 0` reference-side (decode-ran guard).
5. **Recovery arm DEFERRED** (AMEND-OD1) — no un-eject-timing cross-side assertion.

Use `StatsAsserter` (cross-side), NOT `SubjectAsserter` (`reference_differential_asserter_dispatch`). Constants single-sourced (D-S40.1-4).

- [ ] **Step 1:** Write `driver_test.go` (the `backendIdxFromBody` table test) → run → FAIL (helper undefined).
- [ ] **Step 2:** Write `driver.go` (the helper + the full fixture) + `expectations.yaml` + `README.md`.
- [ ] **Step 3:** `go test ./test/fixtures/0069-outlier-detection-consecutive-5xx/driver/ -count=1` (the unit test) → PASS.
- [ ] **Step 4: Run the cross-side fixture** (requires Docker + `contrib-v1.37.2`): `go test ./test/differential/ -run 'TestDifferential/0069' -count=1 -v` (`reference_differential_run_selector` — the `TestDifferential/` prefix is REQUIRED). Expected: PASS, both sides converge to `ejections_active==1`, 100% 2xx in the measured phase.
- [ ] **Step 5:** gofmt/vet/lint. Record fixtures **70 → 71**.
- [ ] **Step 6: Commit** (`phase 40.1 Task 10: 0069 cross-side outlier-detection-consecutive-5xx fixture`).

---

## Task 11: `0069` deliberate breaks + 20-run flake

**Files:** none committed (verification only; the SPEC §8.1 break protocol).

★ Use `-count=1` for EVERY break run (`reference_differential_break_protocol_count1` — go-test caching serves a stale PASS otherwise) and the `TestDifferential/0069` selector.

- [ ] **Step 1: Break (A) — detector no-ops.** Temporarily make `outlierDetector.record` return immediately (or `RecordUpstreamResult` a no-op). Run `go test ./test/differential/ -run 'TestDifferential/0069' -count=1` → MUST FAIL (the subject's `ejections_active` never reaches 1 → the poll times out). This proves the detector is live (not the dead legacy driver). Restore.
- [ ] **Step 2: Break (B) — LB ignores `available`.** Temporarily revert ONE leaf consult site (the round-robin `loadbalancer.go`) `available → isHealthy` (so ejected hosts are still picked). Run → MUST FAIL (the measured phase still routes to the ejected 503 host → 5xx leak / warmup never stabilizes). Restore.
- [ ] **Step 3: Confirm both breaks restored** (`git diff` clean; `go test ./test/differential/ -run 'TestDifferential/0069' -count=1` → PASS).
- [ ] **Step 4: 20-run flake gate:** `for i in $(seq 20); do go test ./test/differential/ -run 'TestDifferential/0069' -count=1 || echo "FAIL $i"; done` → 20/20 PASS (the poll-to-converge + warmup makes it deterministic; if any flake, widen `convergeDeadline`/`warmupStable` per `reference_health_check_propagation_warmup`, NOT a fixed sleep).
- [ ] **Step 5:** Record the break + flake results in PROGRESS. (No commit — verification only.)

---

## Task 12: Full 71-dir differential + six-gate

**Files:** none (verification); update PROGRESS.

- [ ] **Step 1: Six-gate** (ADR-0052): `go build ./...`; `go vet ./...`; `gofmt -l internal/ test/` (empty); `golangci-lint run ./...`; `go test ./internal/... -count=1`; `go test ./test/differential/ -count=1` (ALL 71 GREEN).
- [ ] **Step 2: Stat-surface count → 1137**; fixtures → 71; fuzzers → 42 (unchanged); BackendKind tail → 35. Record all in PROGRESS.
- [ ] **Step 3:** If any gate fails, fix + re-run before proceeding. (No commit unless a fix is needed — then a focused fix commit.)

---

## Task 13: ADR-0245 body + BEHAVIOR_CONTRACT delta

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` — ADR-0245 full entry (§Decision + §Consequences; the §Context is already drafted in SPEC §13 — promote/refine it). DECISIONS tail ADR-0244 → **ADR-0245** (next-free ADR-0246).
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` — the new `### Cluster — passive health (outlier detection)` subsection (SPEC §9): the `consecutive_5xx` detector (eject on N consecutive 5xx; per-host reset-on-2xx); the lazy flat-`base_ejection_time` un-eject (the sweep/backoff departures); the `max_ejection_percent` cap + `ejections_overflow`; the `available = isHealthy && !isEjected` predicate (membership gauge unchanged); the +5 stats (the double-count) + the deferred-stat departures; the seam at the live success sites. Advance the stat-surface block **1132 → 1137**.

- [ ] **Step 1:** Write the ADR-0245 body (§Decision: the seam at the two live success sites; ejection as a separate per-host sub-state; `available` predicate; +5 stats double-count; the AMEND-OD1/OD2 departures recorded for 40.2/40.3. §Consequences: byte-stable when absent; the deferred sweep/backoff; 40.2/40.3 reuse).
- [ ] **Step 2:** Write the BEHAVIOR_CONTRACT subsection + the stat-count bump.
- [ ] **Step 3:** `go build ./...` (docs-only, sanity) — no code change.
- [ ] **Step 4: Commit** (`phase 40.1 Task 13: ADR-0245 body + BEHAVIOR_CONTRACT passive-health subsection (stat 1132→1137)`).

---

## Task 14: Completion bundle

**Files:**
- Modify: `docs/envoy-go/STATE.md` — prepend the `phase 40.1 IMPL done` active-phase block (demote the SPEC block to prior); update lifecycle-state / next-skill / last-commit / last-updated / phase-directory; the count footer → stat **1137** / fixtures **71** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0245** (next-free ADR-0246).
- Modify: `docs/envoy-go/ROADMAP.md` — row 40: append the 40.1-IMPL-DONE note. Row 40 STAYS `in-progress` (40.2 + 40.3 legs pending — per `reference_roadmap_split_phase_row_done`, the row flips `done` only when ALL three legs land; this is NOT the final leg).
- Modify: `docs/envoy-go/phases/40.1-outlier-detection-consecutive-5xx/{PROGRESS.md,README.md}` — final state + exit counts.
- Modify: `next-prompt.txt` — roll the cold-start forward to the phase-40.2 SPEC (the next leg: `consecutive_gateway_failure` + `consecutive_local_origin_failure` + `split_external_local_origin_errors`; threads `UpstreamResult.LocalOriginErr` through the pre-pick failure sites) OR a new subject. SHA-fill after the squash.

- [ ] **Step 1:** Update STATE.md + ROADMAP.md + PROGRESS.md + README.md with the final counts.
- [ ] **Step 2: Final six-gate re-run** (ADR-0052) to confirm the doc edits did not break the build/tests.
- [ ] **Step 3: Commit** the bundle (`phase 40.1 Task 14: completion bundle — STATE/ROADMAP/PROGRESS + next-prompt roll-forward`).
- [ ] **Step 4:** Controller (NOT a subagent — `feedback_subagents_no_push`) squashes the branch, ff-merges to master, pushes, rolls next-prompt with the squash SHA, removes the worktree, deletes the branch (superpowers:finishing-a-development-branch).

---

## Exit deltas (SPEC §14)

| Count | Before (SPEC) | After (IMPL) |
|---|---|---|
| Stat surface | 1132 | **1137** (+5) |
| Fixtures | 70 | **71** (`0069`) |
| Fuzzers | 42 | 42 (unchanged) |
| BackendKind tail | 34 | **35** (`HTTP503Responder`) |
| DECISIONS tail | ADR-0244 | **ADR-0245** (next-free ADR-0246) |
| New Go packages | — | 0 |
| New go.mod modules | — | 0 |

ROADMAP row 40 STAYS `in-progress` (40.2/40.3 pending). The FINAL ADR-0045 split-gate (D-S40.1-6): NO FURTHER SPLIT.
