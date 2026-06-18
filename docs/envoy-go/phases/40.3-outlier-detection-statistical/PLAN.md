# Phase 40.3 Implementation Plan — `outlier_detection`: the statistical detectors (`success_rate` + `failure_percentage`) + the per-interval cross-host aggregation runtime

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the two external statistical passive-outlier detectors over the 40.1/40.2 substrate — `success_rate` (eject hosts below `mean − (success_rate_stdev_factor/1000)×stddev` across the eligible hosts, ejecting-by-default) and `failure_percentage` (eject hosts with `failure% ≥ failure_percentage_threshold`, detect-only-by-default) — both evaluated on a periodic per-cluster background sweep (the project's FIRST outlier goroutine), with NO router-seam re-design and byte-identical behavior for clusters without `outlier_detection`.

**Architecture:** The statistical detectors are SWEEP-driven, distinct from the 40.1/40.2 per-request consecutive detectors which fire synchronously inside `record`. 40.3 adds: (1) windowed per-host `(intervalTotal, intervalSuccess)` `atomic.Uint64` counters on `hostHealth`, fed by the EXISTING `RecordUpstreamResult` seam (which already fires on EVERY completed request — NO router touch); (2) a per-cluster background goroutine (`Manager.StartOutlierDetection(ctx)` boot-start, joined in `Drain` — the 39.1 `StartHealthChecks`/`hcWG`/`hcCancel` lifecycle, mirrored as `odCancel`/`odWG`) that each `interval` snapshots-and-resets every host's window, computes the eligible-host set + the cross-host mean/population-stddev, and ejects via the EXISTING `tryEject` (cap + CAS + the cross-detector double-count). The 40.1/40.2 seam, the ejection dimension on `hostHealth`, the `available` LB-pick predicate, the lazy un-eject, the `max_ejection_percent` cap, and `tryEject` are REUSED UNCHANGED.

**Tech Stack:** Go; `github.com/envoyproxy/go-control-plane` v1.32.4 (`cluster.v3.OutlierDetection` fields 6–9, 16–20, 2 — already vendored, ZERO new go.mod modules); the internal `stats` registry; the differential harness (`contrib-v1.37.2`, Docker bridge, poll-to-converge + warmup + delta counters).

**Reference docs (read alongside this plan):**
- SPEC: `docs/envoy-go/phases/40.3-outlier-detection-statistical/SPEC.md` (the §1.1 AMEND-OD3-1..7 amendments; §3 architecture — the parse extension + the windowed counters + the per-interval sweep + the goroutine lifecycle + the concurrency model; §5 proto roster fields 6–9/16–20/`interval`; §6 six reject arms; §7 stat surface +8; §8 `0072`/`0073` fixtures; §11 D-OD3-* live pins; §12 D-questions; §13 ADR-0247 §Context).
- The 40.1/40.2 substrate (the legs this extends): `docs/envoy-go/phases/40.1-outlier-detection-consecutive-5xx/{SPEC,PLAN}.md` + `docs/envoy-go/phases/40.2-outlier-detection-other-consecutive/{SPEC,PLAN}.md` + `internal/cluster/outlier.go` (the `record`/`parseOutlierDetection`/`outlierDetector`/`tryEject`/`registerStats` this extends) + `internal/cluster/health.go` (`hostHealth` + the lazy un-eject + `healthChecker.run`) + `internal/cluster/manager.go` (`StartHealthChecks`/`Drain`/`hcWG`/`hcCancel`) + ADR-0245/0246 in `docs/envoy-go/DECISIONS.md`.

---

## D-question resolutions (SPEC §12) — settled at PLAN

These were the open PLAN/IMPL D-questions in SPEC §12. The implementer MUST follow these (they are baked into the tasks below).

### D-S40.3-1 — house reject wording (§6)
Six new arms, the 40.1 house prefix `cluster: %q: outlier_detection: ` + the reason (mirroring the reference PGV wording verbatim, ADR-0080). All `*wrapperspb.UInt32Value` except the duration — validate only when the field is non-nil:
- `cluster: %q: outlier_detection: enforcing_success_rate: value must be less than or equal to 100`
- `cluster: %q: outlier_detection: enforcing_failure_percentage: value must be less than or equal to 100`
- `cluster: %q: outlier_detection: failure_percentage_threshold: value must be less than or equal to 100`
- `cluster: %q: outlier_detection: enforcing_local_origin_success_rate: value must be less than or equal to 100`
- `cluster: %q: outlier_detection: enforcing_failure_percentage_local_origin: value must be less than or equal to 100`
- `cluster: %q: outlier_detection: max_ejection_time: value must be greater than 0s`

NO reject arm for `success_rate_stdev_factor` (the reference ACCEPTS 0 — no PGV lower bound; live-confirmed D-OD3-REJECT). A NEGATIVE `max_ejection_time` is already rejected at the duration-parse layer (`Invalid duration: Expected positive duration`); the `<= 0s` arm here catches an explicit `0s`. The existing 40.1/40.2 reject arms are UNCHANGED.

### D-S40.3-2 — windowed-counter placement + snapshot mechanism ★
`intervalTotal atomic.Uint64` + `intervalSuccess atomic.Uint64` on `hostHealth` (alongside the 40.1/40.2 `consec5xx`/`consecGw`/`consecLO`). The per-request `record` does `Add(1)` (write side); the sweep does `Swap(0)` (snapshot+reset side). This is the minimal race-clean atomic form (no ring/double-buffer — a request landing mid-sweep is counted in the next window, matching the reference's interval-boundary semantics, SPEC §3.4). No new lock. Verified race-clean under `-race` in Task 6.

### D-S40.3-3 — the success/failure classification in `record` for the window ★
The window accumulation is added at the TOP of `record`, AFTER the `h, ok := d.health.states[ep.Addr()]` lookup + the `isEjected` lazy-uneject refresh, but BEFORE the localOriginErr/external branching — so it counts on EVERY record path (the local-origin path, the 5xx path, AND the non-5xx-reset path), exactly once per call:
```go
h.intervalTotal.Add(1)
if !localOriginErr && (statusCode < 500 || statusCode >= 600) {
    h.intervalSuccess.Add(1) // success = NOT a local-origin failure AND NOT a 5xx
}
```
A `localOriginErr` is a failure; a 5xx (`500 <= code < 600`) is a failure; everything else is a success. This is split-agnostic for the EXTERNAL statistical detectors (the local-origin-vs-external windowing is the deferred local-origin-statistical axis, SPEC §2). The unknown-addr early return (`if !ok { return }`) correctly counts nothing (no host record).

### D-S40.3-4 — the sweep goroutine lifecycle ★
`Manager.StartOutlierDetection(ctx)` is a SIBLING of `StartHealthChecks` (one goroutine per cluster with `outlier_detection`, launched unconditionally on every outlier cluster — the statistical detectors no-op when `eligible < minimum_hosts`, so a 3-host `0069`/`0070`/`0071` cluster runs a harmless sweep). It uses a PARALLEL `odCancel context.CancelFunc` + `odWG sync.WaitGroup` pair on `Manager` (NOT the active-HC `hcCancel`/`hcWG` — the two runtimes are independent). `Drain` stops BOTH (cancel + join) before the pool close. The boot call site is `cmd/envoy-go/main.go` immediately after `cm.StartHealthChecks(ctx)` (post-Freeze, line 274). The sweep does NOT fire immediately at t=0 (no data yet; unlike `healthChecker.run` which probes immediately) — it ticks every `interval`. No goroutine leak under `-race` (Task 6 test cancels + joins).

### D-S40.3-5 — `0072`/`0073` constants single-sourced ★
Each driver's workload constants live in one `const` block (the `0069` precedent, `reference_fixture_workload_constant_desync`). The load-bearing physics (SPEC §8.1 threshold-positivity + the volume-floor caveat):
- **`0072` topology:** K = **5** healthy `HTTPEcho` + 1 `HTTP503Responder` (6 hosts). With success rates {5×1.0, 1×0.0}: mean = 5/6 ≈ 0.8333, population stddev ≈ 0.3727, threshold = mean − 1.9×stddev ≈ 0.125 > 0 (the bad host's 0.0 < 0.125 ⇒ ejected; the healthy hosts' 1.0 ≥ 0.125 ⇒ kept). Live-proven by the §8.1 probe. K=5 keeps the threshold safely positive at the default `success_rate_stdev_factor` 1900.
- **`minimum_hosts`:** `success_rate_minimum_hosts` (and `failure_percentage_minimum_hosts` for `0072`'s detect-only failure_percentage cross-assertion) set to **2** (≤ the 6 eligible hosts; well below the default 5 so the detectors are guaranteed eligible).
- **`request_volume`:** single-source ONE low `reqVolume = 10` for BOTH `success_rate_request_volume` AND `failure_percentage_request_volume` on `0072` (the §8.1 volume-floor fix: keeping failure_percentage's floor = success_rate's floor makes the `0072` `ejections_detected_failure_percentage >= 1` cross-assertion LIVE — both detectors are eligible at the SAME sweep, so the detect-only failure_percentage detector counts the bad host's 100%-failure before/at the same sweep the success_rate detector ejects it). On `0073` set `failure_percentage_request_volume = reqVolume`.
- **`interval`:** **1s** (short — the sweep cadence; the bad host must accrue ≥ `reqVolume` requests within ONE interval window, then be ejected at the next tick). `base_ejection_time`: **30s** (long — recovery DEFERRED; the host stays ejected through the measured phase). `max_ejection_percent`: **100**.
- **Drive volume:** `ejectDriveRequests = 300` round-robin GET / (the bad host gets ~50 picks — well over `reqVolume` 10 — in a tight loop that completes inside one 1s interval). The consecutive detectors are DISABLED on both fixtures (`consecutive_5xx: 0`, `consecutive_gateway_failure: 0`, `enforcing_consecutive_local_origin_failure` n/a) so ONLY the statistical sweep ejects (isolating the detector under test; `ejections_detected_consecutive_5xx`/`_gateway_failure` stay 0).
- **Convergence:** `convergeDeadline = 30s`, `convergePoll = 200ms`; `warmupStable = 10`, `warmupDeadline = 15s` (the `0069` values).
- **Ports:** `refContainerListenerPort` = **19161** for `0072` (next-free after `0071`'s 19160: 0069=19158, 0070=19159, 0071=19160), **19162** for `0073`. `refAdminPort = 9901`. VERIFY next-free at IMPL (grep the existing fixtures).

★ **Interval-window tuning is flake-gated, not assumption-gated:** if the 20-run flake gate (Task 10) shows non-convergence, the fix is to LOWER `reqVolume` and/or RAISE `interval` (so one interval's drive reliably clears the per-host volume floor) — NOT to weaken an assertion (`reference_health_check_propagation_warmup` philosophy: widen timing, never loosen the cross-side invariant).

### D-S40.3-6 — `0072`/`0073` two dirs vs one folded dir
**TWO separate fixture dirs** (`0072-outlier-detection-success-rate` + `0073-outlier-detection-failure-percentage`) — the `0067`/`0068` + `0070`/`0071` two-dir precedent. Each is its own cross-side runner branch (`reference_differential_fixture_dispatch_constraint` satisfied; both are cross-side, no boot-reject mixing). Cleaner separation of the two single-enforcing-detector configs (AMEND-OD3-3).

### D-S40.3-7 — final ADR-0045 split-gate re-check
Anticipated production LoC (below) ≈ **~170–230 LoC** across ~4 prod files (`outlier.go` the bulk; small `health.go`/`manager.go`/`main.go` edits); ~13 tasks. Both comfortably under the ADR-0045 gate (`> ~25 tasks OR > ~1500 LoC`). **NO FURTHER SPLIT** — 40.3 ships as one flat leg. Landing 40.3 consumes the 3-leg by-detector-class split → ROADMAP row 40 flips `in-progress → done` (`reference_roadmap_split_phase_row_done`; NO parent rollup per ADR-0106).

### D-S40.3-8 — register all 8 statistical stat names (incl. the 4 always-0 local-origin)
Register all 8 (the 40.2 unconditional-registration precedent + the FINAL-leg surface closure): the 4 external (`ejections_{detected,enforced}_{success_rate,failure_percentage}`, LOGIC implemented + driven) + the 4 local-origin (`ejections_{detected,enforced}_{local_origin_success_rate,local_origin_failure_percentage}`, LOGIC DEFERRED, emit 0). **Confirmed no fixture-asserter break:** the `0069`/`0070`/`0071` `StatsAsserter`s cross-assert specific named counters (NOT an exhaustive name-set), so 4 extra 0-valued names are invisible to them; the reference ALSO emits these 4 names at 0 (so cross-side parity holds where any future asserter touches them). The new `0072`/`0073` asserters touch only the external names + the local-origin `enforced` names where they assert `== 0` (which the deferred-logic 0-emit satisfies — a real cross-side match).

---

## File structure

**Production (`internal/` + `cmd/`):**
- `internal/cluster/health.go` (MODIFY) — add `intervalTotal atomic.Uint64` + `intervalSuccess atomic.Uint64` to `hostHealth` (the windowed per-interval request/success counters, alongside the consecutive counters).
- `internal/cluster/outlier.go` (MODIFY) — the bulk: extend `outlierConfig` (the 8 statistical fields + `interval` as load-bearing); extend `parseOutlierDetection` (defaults + the 6 reject arms); add the window accumulation in `record`; add the 8 statistical stat handles to `outlierDetector`; add `evalSuccessRate`/`evalFailurePercentage`/`sweep`/`run`; extend `registerStats` (+8).
- `internal/cluster/manager.go` (MODIFY) — add `odCancel context.CancelFunc` + `odWG sync.WaitGroup` to `Manager`; add `StartOutlierDetection(ctx)`; extend `Drain` to stop+join the outlier runtime.
- `cmd/envoy-go/main.go` (MODIFY) — call `cm.StartOutlierDetection(ctx)` immediately after `cm.StartHealthChecks(ctx)` (post-Freeze, ~line 274).

**Test harness (`test/`):**
- `test/fixtures/0072-outlier-detection-success-rate/{driver/driver.go,driver/driver_test.go,expectations.yaml,README.md}` (CREATE) — success_rate fixture (K=5+1 topology via `PerHostBackendKind`, reuses `HTTP503Responder`).
- `test/fixtures/0073-outlier-detection-failure-percentage/{driver/driver.go,driver/driver_test.go,expectations.yaml,README.md}` (CREATE) — failure_percentage fixture (same topology, `enforcing_success_rate: 0` ⇒ failure_percentage the sole enforcer).

**Docs:**
- `docs/envoy-go/DECISIONS.md` (ADR-0247 body), `BEHAVIOR_CONTRACT.md` (passive-health subsection extension + stat-count 1141 → 1149), `STATE.md`, `ROADMAP.md` (row 40 → `done`), `next-prompt.txt`, the phase `PROGRESS.md` + `README.md`.

---

## Task 1: Baselines + PROGRESS scaffold

**Files:**
- Create: `docs/envoy-go/phases/40.3-outlier-detection-statistical/PROGRESS.md`

- [ ] **Step 1: Capture the pre-IMPL six-gate baseline.** Run, record in PROGRESS.md:
  - `go build ./...`
  - `go vet ./...`
  - `gofmt -l internal/ test/ cmd/` (expect empty)
  - `go test ./internal/... 2>&1 | tail -20`
  - `go test ./test/differential/ -count=1 2>&1 | tail -20` (the full **73**-dir suite — the byte-stability anchor)
  - Stat surface: tracked as a documented running total (no count-script). Record the baseline **1141** (SPEC §14); the 40.3 exit total is verified arithmetically (1141 + 8 = 1149) against the Task 7 registration test.
- [ ] **Step 2: Record the baselines + the task checklist** in PROGRESS.md (counts: stat 1141 / fixtures 73 / fuzzers 42 / BackendKind tail 35 / DECISIONS tail ADR-0246, next-free ADR-0247; the anticipated exit deltas from SPEC §14).
- [ ] **Step 3: Commit.**
```bash
git add docs/envoy-go/phases/40.3-outlier-detection-statistical/PROGRESS.md
git commit -m "phase 40.3 Task 1: PROGRESS scaffold + pre-IMPL baselines"
```

---

## Task 2: `parseOutlierDetection` extension (the 8 statistical fields + `interval`-as-load-bearing) + the 6 reject arms

**Files:**
- Modify: `internal/cluster/outlier.go` — extend `outlierConfig` + `parseOutlierDetection`.
- Test: `internal/cluster/outlier_test.go`

Extend `outlierConfig` (alongside the 40.1/40.2 fields; defaults per SPEC §5, live-confirmed D-OD3-PROTO):
```go
// success_rate detector — sweep-driven; ejects-by-default (enforcing 100).
successRateMinHosts    uint32        // success_rate_minimum_hosts (default 5)
successRateReqVolume   uint32        // success_rate_request_volume (default 100)
successRateStdevFactor uint32        // success_rate_stdev_factor (default 1900; /1000 ⇒ 1.9)
enforcingSuccessRate   uint32        // enforcing_success_rate (default 100)
// failure_percentage detector — sweep-driven; detect-only-by-default (enforcing 0).
failurePctThreshold uint32           // failure_percentage_threshold (default 85)
failurePctMinHosts  uint32           // failure_percentage_minimum_hosts (default 5)
failurePctReqVolume uint32           // failure_percentage_request_volume (default 50)
enforcingFailurePct uint32           // enforcing_failure_percentage (default 0 ⇒ detect-only)
interval            time.Duration    // the sweep cadence (default 10s) — NOW LOAD-BEARING
```

Extend `parseOutlierDetection` — populate the defaults (the 40.1/40.2 `if v := od.GetX(); v != nil { cfg.x = v.GetValue() }` pattern), and make `interval` load-bearing (the field is ALREADY parse-validated `> 0s` at 40.1; now ALSO populate `cfg.interval`, default 10s):
```go
// Defaults (set on the cfg literal or below): successRateMinHosts 5,
// successRateReqVolume 100, successRateStdevFactor 1900, enforcingSuccessRate 100,
// failurePctThreshold 85, failurePctMinHosts 5, failurePctReqVolume 50,
// enforcingFailurePct 0, interval 10s.
cfg.successRateMinHosts = 5
if v := od.GetSuccessRateMinimumHosts(); v != nil {
    cfg.successRateMinHosts = v.GetValue()
}
cfg.successRateReqVolume = 100
if v := od.GetSuccessRateRequestVolume(); v != nil {
    cfg.successRateReqVolume = v.GetValue()
}
cfg.successRateStdevFactor = 1900
if v := od.GetSuccessRateStdevFactor(); v != nil {
    cfg.successRateStdevFactor = v.GetValue() // 0 accepted — no reject
}
cfg.enforcingSuccessRate = 100
if v := od.GetEnforcingSuccessRate(); v != nil {
    cfg.enforcingSuccessRate = v.GetValue()
}
cfg.failurePctThreshold = 85
if v := od.GetFailurePercentageThreshold(); v != nil {
    cfg.failurePctThreshold = v.GetValue()
}
cfg.failurePctMinHosts = 5
if v := od.GetFailurePercentageMinimumHosts(); v != nil {
    cfg.failurePctMinHosts = v.GetValue()
}
cfg.failurePctReqVolume = 50
if v := od.GetFailurePercentageRequestVolume(); v != nil {
    cfg.failurePctReqVolume = v.GetValue()
}
cfg.enforcingFailurePct = 0
if v := od.GetEnforcingFailurePercentage(); v != nil {
    cfg.enforcingFailurePct = v.GetValue()
}
cfg.interval = 10 * time.Second
if d := od.GetInterval(); d != nil {
    cfg.interval = d.AsDuration() // already validated > 0s by the reject arm above
}
```
Add the 6 reject arms (D-S40.3-1) alongside the existing arms (validate only when the field is non-nil). NOTE the proto getter names: `GetEnforcingSuccessRate`, `GetEnforcingFailurePercentage`, `GetFailurePercentageThreshold`, `GetEnforcingLocalOriginSuccessRate`, `GetEnforcingFailurePercentageLocalOrigin`, `GetMaxEjectionTime`:
```go
if v := od.GetEnforcingSuccessRate(); v != nil && v.GetValue() > 100 {
    return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_success_rate: value must be less than or equal to 100", name)
}
if v := od.GetEnforcingFailurePercentage(); v != nil && v.GetValue() > 100 {
    return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_failure_percentage: value must be less than or equal to 100", name)
}
if v := od.GetFailurePercentageThreshold(); v != nil && v.GetValue() > 100 {
    return nil, fmt.Errorf("cluster: %q: outlier_detection: failure_percentage_threshold: value must be less than or equal to 100", name)
}
if v := od.GetEnforcingLocalOriginSuccessRate(); v != nil && v.GetValue() > 100 {
    return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_local_origin_success_rate: value must be less than or equal to 100", name)
}
if v := od.GetEnforcingFailurePercentageLocalOrigin(); v != nil && v.GetValue() > 100 {
    return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_failure_percentage_local_origin: value must be less than or equal to 100", name)
}
if d := od.GetMaxEjectionTime(); d != nil && d.AsDuration() <= 0 {
    return nil, fmt.Errorf("cluster: %q: outlier_detection: max_ejection_time: value must be greater than 0s", name)
}
```

- [ ] **Step 1: Write failing tests** (`outlier_test.go`): a full statistical config parses with the correct values (set all 8 fields + a non-default `interval: 2s` ⇒ `cfg.interval == 2s`); ALL eight statistical fields ABSENT ⇒ the documented defaults (minHosts 5, reqVolume 100, stdevFactor 1900, enforcingSR 100, threshold 85, fpMinHosts 5, fpReqVolume 50, enforcingFP 0, interval 10s); `success_rate_stdev_factor: 0` parses (NO reject) ⇒ `cfg.successRateStdevFactor == 0`; the SIX new reject arms each produce the exact house string (`enforcing_success_rate: 101`, `enforcing_failure_percentage: 101`, `failure_percentage_threshold: 101`, `enforcing_local_origin_success_rate: 101`, `enforcing_failure_percentage_local_origin: 101`, `max_ejection_time: 0s`). Existing 40.1/40.2 parse tests still pass (the new fields default correctly when absent).
- [ ] **Step 2: Run → FAIL** (`go test ./internal/cluster/ -run 'TestParseOutlier' -v`).
- [ ] **Step 3: Implement** the `outlierConfig` fields + the `parseOutlierDetection` extension + the 6 reject arms.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** `gofmt -l internal/cluster/ && go vet ./internal/cluster/ && golangci-lint run internal/cluster/...` (per `feedback_pertask_gofmt_lint`).
- [ ] **Step 6: Commit** (`phase 40.3 Task 2: parseOutlierDetection statistical fields + interval-as-load-bearing + 6 reject arms`).

---

## Task 3: The windowed `(intervalTotal, intervalSuccess)` counters + the `record` classification side-effect

**Files:**
- Modify: `internal/cluster/health.go` — add `intervalTotal`/`intervalSuccess` to `hostHealth`.
- Modify: `internal/cluster/outlier.go` — add the window accumulation at the top of `record`.
- Test: `internal/cluster/outlier_test.go`

Extend `hostHealth` (alongside `consec5xx`/`consecGw`/`consecLO`):
```go
intervalTotal   atomic.Uint64 // requests this interval window (reset at each sweep) (ADR-0247)
intervalSuccess atomic.Uint64 // successful requests this interval window (ADR-0247)
```

Add to `record`, immediately AFTER the `_ = d.health.isEjected(ep)` refresh and BEFORE the `localOriginErr` branch (D-S40.3-3 — counts on EVERY path, exactly once):
```go
// Windowed per-host accumulation for the statistical sweep (ADR-0247). Counts on
// EVERY record path (local-origin, 5xx, and non-5xx). success = NOT a local-origin
// failure AND NOT a 5xx external status. The sweep Swap-resets these each interval.
h.intervalTotal.Add(1)
if !localOriginErr && (statusCode < 500 || statusCode >= 600) {
    h.intervalSuccess.Add(1)
}
```

- [ ] **Step 1: Write failing tests** (`outlier_test.go`): build a detector over 1 host; after `record(ep, 200, false)` ×3 + `record(ep, 503, false)` ×2 + `record(ep, 0, true)` ×1 (local-origin), assert `h.intervalTotal.Load() == 6` and `h.intervalSuccess.Load() == 3` (3 × 2xx are successes; the 2 × 5xx and the 1 × local-origin are failures). Also assert a 4xx (`record(ep, 404, false)`) counts as a success (`< 500`) and a 3xx (`302`) too. (These tests read the atomics directly; the sweep that consumes them is Task 6.)
- [ ] **Step 2: Run → FAIL** (field undefined).
- [ ] **Step 3: Implement** the `hostHealth` fields + the `record` accumulation.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/cluster/ -count=1` (the existing consecutive-detector tests are UNCHANGED — the window accumulation is orthogonal).
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 40.3 Task 3: windowed intervalTotal/intervalSuccess counters + record classification side-effect`).

---

## Task 4: The `success_rate` aggregation (`evalSuccessRate`) + the +2 success_rate stat handles

**Files:**
- Modify: `internal/cluster/outlier.go` — add the `hostWindow` type, `evalSuccessRate`, the +2 success_rate detector handles, and `import "math"`.
- Test: `internal/cluster/outlier_test.go`

Add to the `outlierDetector` struct (the local-origin + failure_percentage handles are added in Task 5/7):
```go
ejectionsDetectedSR *stats.Counter // ejections_detected_success_rate
ejectionsEnforcedSR *stats.Counter // ejections_enforced_success_rate
```

Add the snapshot row type + the success_rate evaluator (callable in isolation for the unit test — the snapshot loop that produces `[]hostWindow` is Task 6):
```go
// hostWindow is one host's snapshotted interval window (the sweep's per-host row).
type hostWindow struct {
    ep      Endpoint
    h       *hostHealth
    total   uint64
    success uint64
}

// evalSuccessRate runs the success_rate detector over the snapshot (SPEC §3.3 step 2):
// collect hosts with total >= success_rate_request_volume (the eligible set); if
// >= success_rate_minimum_hosts eligible, compute the cross-host mean success-rate +
// the POPULATION stddev (divisor N), threshold = mean - (stdev_factor/1000)*stddev;
// eject each eligible host with rate < threshold. A non-positive threshold is a
// benign no-op (no rate in [0,1] is below it). (ADR-0247)
func (d *outlierDetector) evalSuccessRate(snap []hostWindow) {
    var eligible []hostWindow
    for _, w := range snap {
        if w.total >= uint64(d.cfg.successRateReqVolume) {
            eligible = append(eligible, w)
        }
    }
    if len(eligible) < int(d.cfg.successRateMinHosts) {
        return
    }
    rates := make([]float64, len(eligible))
    var sum float64
    for i, w := range eligible {
        rates[i] = float64(w.success) / float64(w.total)
        sum += rates[i]
    }
    mean := sum / float64(len(eligible))
    var variance float64
    for _, r := range rates {
        diff := r - mean
        variance += diff * diff
    }
    variance /= float64(len(eligible)) // POPULATION stddev (divisor N, not N-1)
    threshold := mean - (float64(d.cfg.successRateStdevFactor)/1000.0)*math.Sqrt(variance)
    if threshold <= 0 {
        return // no success rate in [0,1] can be below a non-positive threshold (AMEND-OD3-5)
    }
    for i, w := range eligible {
        if rates[i] < threshold {
            if d.ejectionsDetectedSR != nil {
                d.ejectionsDetectedSR.Inc()
            }
            d.tryEject(w.ep, w.h, d.cfg.enforcingSuccessRate, d.ejectionsEnforcedSR)
        }
    }
}
```

★ **Test-handle provisioning (the 40.2 `withGatewayHandles` precedent, `outlier_test.go:631`):** the unit-test fixture `newDetectorFixture` (`outlier_test.go:333`) allocates ONLY the 5 base/40.1 handles; the SR/FP handles default to nil and the eval methods' `if d.ejectionsXX != nil` guards would make the assertions VACUOUS. Add a chained helper mirroring `withGatewayHandles`:
```go
// withSuccessRateHandles assigns the +2 success_rate stat handles (Task 7
// allocates them in registerStats; unit tests inject directly to observe counts).
func (f detectorFixture) withSuccessRateHandles() detectorFixture {
    reg := stats.NewRegistry()
    f.d.ejectionsDetectedSR = reg.NewCounter("outlier_detection.ejections_detected_success_rate")
    f.d.ejectionsEnforcedSR = reg.NewCounter("outlier_detection.ejections_enforced_success_rate")
    return f
}
```
and chain it (`newDetectorFixture(cfg, eps, panicRoll).withSuccessRateHandles()`) in every Task-4 test that asserts a SR counter value.

- [ ] **Step 1: Write failing tests** (deterministic clock + injected `enforceRoll`; build a detector via `newDetectorFixture(...).withSuccessRateHandles()` with `successRateMinHosts: 2`, `successRateReqVolume: 10`, `successRateStdevFactor: 1900`, `enforcingSuccessRate: 100`, `maxEjectionPct: 100`, hand-build a `[]hostWindow` snapshot). NOTE the negative-threshold case below deliberately sets `successRateMinHosts: 2` so the eligibility gate PASSES (3 ≥ 2) and the `threshold <= 0` branch is the thing under test (not short-circuited by the min-hosts gate):
  - **Ejects the outlier:** snapshot {5 hosts (total 100, success 100), 1 host (total 100, success 0)} → the 0%-success host ejects (`ejectionsDetectedSR`/`ejectionsEnforcedSR`/`ejectionsEnforcedTotal`/`ejectionsActive` bump by 1; the 5 healthy hosts are NOT ejected). This pins the POPULATION-stddev threshold ≈ 0.125 (with sample stddev the threshold would differ — D-S40.3-2).
  - **`minimum_hosts` gate:** snapshot {1 host total 100 success 0, 1 host total 100 success 100} with `successRateMinHosts: 5` → < 5 eligible → NO eject.
  - **`request_volume` gate:** snapshot {6 hosts but the bad host total 5 (< reqVolume 10)} → the bad host is NOT eligible → not even evaluated → NO eject (eligible set excludes it).
  - **Negative-threshold no-op (AMEND-OD3-5):** snapshot {2 hosts success 100%, 1 host success 0%} with `successRateMinHosts: 2` → mean = 2/3 ≈ 0.667, population stddev ≈ 0.471, threshold = 0.667 − 1.9×0.471 ≈ −0.228 < 0 → NO eject (the few-hosts-high-variance case).
  - **`enforcing_success_rate: 0` (detect-only):** the outlier crosses the threshold → `ejectionsDetectedSR` bumps but `ejectionsEnforcedSR == 0` and `ejectionsActive == 0` (detect-only).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the `hostWindow` type + `evalSuccessRate` + the +2 handles + `import "math"`.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 40.3 Task 4: success_rate detector aggregation (population stddev threshold + eligibility gates)`).

---

## Task 5: The `failure_percentage` aggregation (`evalFailurePercentage`) + the +2 failure_percentage stat handles

**Files:**
- Modify: `internal/cluster/outlier.go` — add `evalFailurePercentage` + the +2 failure_percentage detector handles.
- Test: `internal/cluster/outlier_test.go`

Add to the `outlierDetector` struct:
```go
ejectionsDetectedFP *stats.Counter // ejections_detected_failure_percentage
ejectionsEnforcedFP *stats.Counter // ejections_enforced_failure_percentage
```

Add the failure_percentage evaluator (an absolute per-host threshold — NO mean/stddev; integer cross-multiplied `>=` to avoid truncating division, `reference_percent_cap_cross_multiply`):
```go
// evalFailurePercentage runs the failure_percentage detector over the snapshot
// (SPEC §3.3 step 3): collect hosts with total >= failure_percentage_request_volume;
// if >= failure_percentage_minimum_hosts eligible, eject each whose failure% >=
// failure_percentage_threshold. failure% >= threshold is cross-multiplied to
// (total-success)*100 >= threshold*total (integer-exact; no truncation). (ADR-0247)
func (d *outlierDetector) evalFailurePercentage(snap []hostWindow) {
    var eligible []hostWindow
    for _, w := range snap {
        if w.total >= uint64(d.cfg.failurePctReqVolume) {
            eligible = append(eligible, w)
        }
    }
    if len(eligible) < int(d.cfg.failurePctMinHosts) {
        return
    }
    for _, w := range eligible {
        failures := w.total - w.success
        if failures*100 >= uint64(d.cfg.failurePctThreshold)*w.total {
            if d.ejectionsDetectedFP != nil {
                d.ejectionsDetectedFP.Inc()
            }
            d.tryEject(w.ep, w.h, d.cfg.enforcingFailurePct, d.ejectionsEnforcedFP)
        }
    }
}
```

★ **Test-handle provisioning:** add `withFailurePercentageHandles()` (mirroring `withSuccessRateHandles` from Task 4) allocating `ejectionsDetectedFP`/`ejectionsEnforcedFP`, and chain it in every Task-5 test that asserts an FP counter value (else the `if d.ejectionsFP != nil` guards make the assertions vacuous).

- [ ] **Step 1: Write failing tests** (build a detector via `newDetectorFixture(...).withFailurePercentageHandles()` with `failurePctMinHosts: 2`, `failurePctReqVolume: 10`, `failurePctThreshold: 85`, `enforcingFailurePct: 100`, `maxEjectionPct: 100`):
  - **Ejects the failing host:** snapshot {2 hosts total 100 success 100 (0% fail), 1 host total 100 success 0 (100% fail)} → the 100%-fail host ejects (detected/enforced FP + total + active bump by 1; the healthy hosts kept).
  - **`>=` boundary (load-bearing):** snapshot {3 hosts; one host total 100 success 15 ⇒ failure% exactly 85} → ejects (`>=` includes the boundary; a buggy `>` would NOT — this pins the break (B) in Task 10). A host with success 16 (failure% 84) → NOT ejected.
  - **`minimum_hosts` gate:** the same failing host with `failurePctMinHosts: 5` and only 3 eligible → NO eject.
  - **`request_volume` gate:** the failing host with total 5 (< reqVolume 10) → not eligible → NO eject.
  - **`enforcing_failure_percentage: 0` (detect-only, the DEFAULT posture):** the failing host crosses the threshold → `ejectionsDetectedFP` bumps but `ejectionsEnforcedFP == 0` and `ejectionsActive == 0`.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `evalFailurePercentage` + the +2 handles.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 40.3 Task 5: failure_percentage detector aggregation (cross-multiplied >= boundary)`).

---

## Task 6: The per-interval sweep + the background goroutine + the `StartOutlierDetection`/`Drain` lifecycle

**Files:**
- Modify: `internal/cluster/outlier.go` — add `sweep()` (snapshot+reset + both evaluators in order) + `run(ctx)` (the ticker loop).
- Modify: `internal/cluster/manager.go` — add `odCancel`/`odWG` to `Manager`; add `StartOutlierDetection(ctx)`; extend `Drain`.
- Modify: `cmd/envoy-go/main.go` — call `cm.StartOutlierDetection(ctx)` after `cm.StartHealthChecks(ctx)`.
- Test: `internal/cluster/outlier_test.go` (sweep + lifecycle); `internal/cluster/manager_test.go` (the `StartOutlierDetection`/`Drain` join, if a manager test file is the established home — else keep it in `outlier_test.go`).

Add the sweep + goroutine to `outlier.go`:
```go
// sweep is one per-interval aggregation pass (SPEC §3.3): snapshot+reset every
// host's window (atomic Swap-to-0), then run the success_rate then the
// failure_percentage detector over the same snapshot. tryEject's CAS makes a host
// ejectable AT MOST ONCE per sweep (AMEND-OD3-3 — if both detectors flag one host,
// the second tryEject no-ops, so ejections_active counts one ejection per host).
func (d *outlierDetector) sweep() {
    snap := make([]hostWindow, 0, len(d.endpoints))
    for _, ep := range d.endpoints {
        h, ok := d.health.states[ep.Addr()]
        if !ok {
            continue
        }
        _ = d.health.isEjected(ep) // lazy un-eject refresh before the snapshot (SPEC §3.3 step 4)
        snap = append(snap, hostWindow{
            ep:      ep,
            h:       h,
            total:   h.intervalTotal.Swap(0),
            success: h.intervalSuccess.Swap(0),
        })
    }
    d.evalSuccessRate(snap)
    d.evalFailurePercentage(snap)
}

// run is the background sweep loop: tick every interval until ctx is done. Unlike
// healthChecker.run it does NOT sweep immediately at t=0 (no data has accrued yet).
func (d *outlierDetector) run(ctx context.Context) {
    t := time.NewTicker(d.cfg.interval)
    defer t.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-t.C:
            d.sweep()
        }
    }
}
```
(Add `"context"` to `outlier.go`'s imports.)

`manager.go` — add to `Manager`:
```go
// odCancel/odWG mirror hcCancel/hcWG for the passive-outlier sweep runtime
// (StartOutlierDetection; phase 40.3, ADR-0247). Zero-valued until started.
odCancel context.CancelFunc
odWG     sync.WaitGroup
```
Add `StartOutlierDetection` (sibling of `StartHealthChecks`):
```go
// StartOutlierDetection launches the per-interval aggregation sweep for every
// cluster with outlier_detection configured (one goroutine per outlier cluster).
// Call AFTER the stats registry is frozen (post-boot) so the injected handles are
// live. The sweeps stop when ctx is canceled OR Drain() is called. Phase 40.3
// (ADR-0247) — the first outlier background goroutine.
func (m *Manager) StartOutlierDetection(ctx context.Context) {
    odCtx, cancel := context.WithCancel(ctx)
    m.odCancel = cancel
    for _, c := range m.clusters {
        if c.outlier == nil {
            continue
        }
        m.odWG.Add(1)
        go func(d *outlierDetector) {
            defer m.odWG.Done()
            d.run(odCtx)
        }(c.outlier)
    }
}
```
Extend `Drain` (alongside the existing `hcCancel` stop — stop the outlier runtime too, before the pool close):
```go
if m.hcCancel != nil {
    m.hcCancel()
    m.hcWG.Wait()
}
if m.odCancel != nil {
    m.odCancel()
    m.odWG.Wait()
}
for _, c := range m.clusters {
    c.closePool()
}
```
`cmd/envoy-go/main.go` (~line 274, immediately after `cm.StartHealthChecks(ctx)`):
```go
cm.StartHealthChecks(ctx)       // active health checks (phase 39.1)
cm.StartOutlierDetection(ctx)   // passive outlier-detection sweeps (phase 40.3); stop on shutdown ctx + cm.Drain()
```

- [ ] **Step 1: Write failing tests** (`-race`-clean):
  - **`sweep` end-to-end:** build a detector over 6 hosts + a real `clusterHealth`, `record` enough to give 5 hosts 100% success + 1 host 0% success (each ≥ reqVolume), call `sweep()` directly (no goroutine), assert the bad host is ejected (`ejectionsActive == 1`) AND its window was reset (`intervalTotal.Load() == 0` post-sweep).
  - **`sweep` resets every window:** after the sweep, EVERY host's `intervalTotal`/`intervalSuccess` read 0 (Swap-to-0 ran for all).
  - **Goroutine lifecycle / no leak:** start `StartOutlierDetection(ctx)` on a `Manager` with one outlier cluster, `record` traffic, cancel the ctx (or call `Drain`), assert `odWG.Wait()` returns (joins cleanly) and a subsequent `Drain` is idempotent. Run the whole package with `-race`.
  - **Sweep-driven eject via the goroutine:** with a SHORT `interval` (e.g. 20ms) injected into the cfg, start the goroutine, `record` the outlier traffic, poll `ejectionsActive` until it reads 1 within a generous deadline, then cancel. (This proves the ticker fires + the sweep ejects, not just the direct `sweep()` call.)
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `sweep`/`run` + the `Manager` fields + `StartOutlierDetection` + the `Drain` extension + the `main.go` boot call.
- [ ] **Step 4: Run → PASS:** `go build ./...` + `go test ./internal/cluster/ -race -count=1`.
- [ ] **Step 5:** gofmt/vet/lint on `internal/cluster/...` + `cmd/...`.
- [ ] **Step 6: Commit** (`phase 40.3 Task 6: per-interval sweep + StartOutlierDetection/Drain lifecycle (first outlier goroutine)`).

---

## Task 7: The +8 statistical stat registrations (extend `registerStats`) + the 4 local-origin handles

**Files:**
- Modify: `internal/cluster/outlier.go` — add the 4 local-origin handles to the detector struct + extend `registerStats` to allocate + assign all 8.
- Test: `internal/cluster/outlier_test.go` (or the manager stat test).

Add the 4 local-origin handles to the `outlierDetector` struct (logic DEFERRED — emit 0; SPEC §7 / AMEND-OD3-4):
```go
// Local-origin statistical variants (LOGIC DEFERRED — registered for surface
// parity, emit 0; SPEC §2). (ADR-0247)
ejectionsDetectedLOSR *stats.Counter // ejections_detected_local_origin_success_rate
ejectionsEnforcedLOSR *stats.Counter // ejections_enforced_local_origin_success_rate
ejectionsDetectedLOFP *stats.Counter // ejections_detected_local_origin_failure_percentage
ejectionsEnforcedLOFP *stats.Counter // ejections_enforced_local_origin_failure_percentage
```
Extend `registerStats` (alongside the existing 9) — register all 8 UNCONDITIONALLY (D-S40.3-8, the 40.2 precedent):
```go
// +8 statistical detector counters (phase 40.3, ADR-0247) — registered
// UNCONDITIONALLY on any outlier cluster (the reference exposes every
// outlier_detection.* name regardless of configured detectors). The 4 external
// names are driven by the sweep; the 4 local-origin names emit 0 (logic deferred).
d.ejectionsDetectedSR = r.NewCounter(op + "ejections_detected_success_rate")
d.ejectionsEnforcedSR = r.NewCounter(op + "ejections_enforced_success_rate")
d.ejectionsDetectedFP = r.NewCounter(op + "ejections_detected_failure_percentage")
d.ejectionsEnforcedFP = r.NewCounter(op + "ejections_enforced_failure_percentage")
d.ejectionsDetectedLOSR = r.NewCounter(op + "ejections_detected_local_origin_success_rate")
d.ejectionsEnforcedLOSR = r.NewCounter(op + "ejections_enforced_local_origin_success_rate")
d.ejectionsDetectedLOFP = r.NewCounter(op + "ejections_detected_local_origin_failure_percentage")
d.ejectionsEnforcedLOFP = r.NewCounter(op + "ejections_enforced_local_origin_failure_percentage")
```

- [ ] **Step 1: Write a failing test** asserting a cluster WITH `outlier_detection` registers all 17 named outlier stats (the 40.1 five + the 40.2 four + these eight) and a cluster WITHOUT registers none. Extend the existing roster + non-nil checks (do NOT reinvent the counting approach): add the +8 names to the `outlierStatNames()` roster helper — the `func` is at `outlier_test.go:1016`, the `[]string` literal to append to is `:1018-1028` (consumed by `TestRegisterOutlierStats_Present`/`_Absent` at `:1052`) — and extend the all-handles-non-nil block (`outlier_test.go:1059-1069`) with the 8 new detector handles. Also update the stale `9 → 17` count phrasing in the two test doc-comments (`:1032`, `:1078`) and the non-nil block comment (`:1057`). The 4 local-origin names are asserted present (registered) at value 0.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the 4 local-origin handles + the +8 registrations.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/... -count=1`.
- [ ] **Step 5: Stat-surface count → 1149** (1141 + 8). Record in PROGRESS.
- [ ] **Step 6:** gofmt/vet/lint.
- [ ] **Step 7: Commit** (`phase 40.3 Task 7: +8 statistical outlier_detection stat registrations (1141→1149)`).

---

## Task 8: The `0072` cross-side success-rate fixture (K=5+1 topology)

**Files:**
- Create: `test/fixtures/0072-outlier-detection-success-rate/driver/driver.go`
- Create: `test/fixtures/0072-outlier-detection-success-rate/driver/driver_test.go` (the `backendIdxFromBody` table test, the `0069` precedent)
- Create: `test/fixtures/0072-outlier-detection-success-rate/expectations.yaml`
- Create: `test/fixtures/0072-outlier-detection-success-rate/README.md`

Model closely on `test/fixtures/0069-outlier-detection-consecutive-5xx/driver/driver.go`. Topology: cluster `c_sr`, lb ROUND_ROBIN, **6** endpoints {host0..host4 HTTPEcho, host5 HTTP503Responder}, on BOTH sides. The driver implements `BackendCount() == 6` + `BackendKindAt(i)` (0..4 ⇒ HTTPEcho; 5 ⇒ HTTP503Responder) — REUSES the `PerHostBackendKind` interface + `HTTP503Responder` BackendKind 35. Single-source the constants (D-S40.3-5):
```go
const (
    fixtureName              = "0072-outlier-detection-success-rate"
    refContainerListenerPort = 19161 // next-free after 0071's 19160; VERIFY at IMPL
    refAdminPort             = 9901
    backendCount             = 6 // K=5 healthy + 1 always-503
    healthyBackendCount      = 5
    badHostIdx               = 5
    // outlier_detection (single-sourced). Statistical success_rate enforces;
    // failure_percentage detect-only (default enforcing 0). Consecutive detectors
    // DISABLED so ONLY the statistical sweep ejects.
    srMinHosts         = 2                      // success_rate_minimum_hosts (<= 6 eligible)
    fpMinHosts         = 2                      // failure_percentage_minimum_hosts (for the detect-only cross-assert)
    reqVolume          = 10                     // BOTH success_rate_request_volume AND failure_percentage_request_volume (volume-floor fix, §8.1)
    stdevFactor        = 1900                   // threshold ≈ 0.125 > 0 at K=5 (probe-proven)
    interval           = 1 * time.Second        // sweep cadence (short)
    baseEjectionTime   = 30 * time.Second       // recovery DEFERRED — host stays ejected
    maxEjectionPercent = 100
    ejectDriveRequests = 300                    // bad host gets ~50 picks, ≫ reqVolume, within one interval
    n                  = 60                     // measured-phase request count
    convergeDeadline   = 30 * time.Second
    convergePoll       = 200 * time.Millisecond
    warmupStable       = 10
    warmupDeadline     = 15 * time.Second
)
```
The `outlier_detection` YAML block (both sides; consecutive detectors explicitly OFF):
```yaml
      outlier_detection:
        success_rate_minimum_hosts: 2
        success_rate_request_volume: 10
        success_rate_stdev_factor: 1900
        enforcing_success_rate: 100
        failure_percentage_minimum_hosts: 2
        failure_percentage_request_volume: 10
        enforcing_failure_percentage: 0
        consecutive_5xx: 0
        consecutive_gateway_failure: 0
        interval: 1s
        base_ejection_time: 30s
        max_ejection_percent: 100
```

`AssertStats` flow (the `0069` template — eject-drive → poll-converge → warmup → measured load → cross-side parity):
1. **Ejection drive:** round-robin GET / `ejectDriveRequests` (300) times per side (503-tolerant). Within one 1s interval the bad host (host5) accrues ≫ `reqVolume` failures; at the next sweep the success_rate detector ejects it (rate 0.0 < threshold ≈0.125).
2. **POLL `/stats` on BOTH sides** until `cluster.c_sr.outlier_detection.ejections_active == 1` (deadline `convergeDeadline`, NO fixed sleep).
3. **Warmup-until-K-200s** + **measured load phase** (delta counters): 100% `upstream_rq_2xx`, 0 `upstream_rq_5xx`, served by the 5 healthy hosts (NEVER host5).
4. **Cross-side stat parity** (`StatsAsserter`, NOT `SubjectAsserter` — `reference_differential_asserter_dispatch`): `outlier_detection.{ejections_active == 1, ejections_enforced_total == 1, ejections_detected_success_rate >= 1, ejections_enforced_success_rate >= 1}` AND `ejections_enforced_failure_percentage == 0` (failure_percentage detect-only at default enforcing 0 — AMEND-OD3-1, a LIVE `== 0` assertion) AND `ejections_detected_failure_percentage >= 1` (the bad host's 100% failure ≥ 85, eligible at the SAME sweep since `reqVolume` is single-sourced — §8.1 volume-floor fix; this is a LIVE cross-side match) AND `ejections_detected_consecutive_5xx == 0` (the consecutive detectors are OFF — isolates the statistical detector). `ejections_active == 1` confirms the single-enforcing-detector / one-host-one-ejection posture (AMEND-OD3-3). Verify `upstream_rq_total > 0` reference-side. Recovery arm DEFERRED.

- [ ] **Step 1:** Write `driver_test.go` (`backendIdxFromBody` table test, the `0069` precedent) → run → FAIL (helper undefined).
- [ ] **Step 2:** Write `driver.go` + `expectations.yaml` + `README.md`.
- [ ] **Step 3:** `go test ./test/fixtures/0072-outlier-detection-success-rate/driver/ -count=1` (unit) → PASS.
- [ ] **Step 4: Run the cross-side fixture** (Docker + `contrib-v1.37.2`): `go test ./test/differential/ -run 'TestDifferential/0072' -count=1 -v` (`reference_differential_run_selector` — the `TestDifferential/` prefix REQUIRED). Expected PASS, both sides converge `ejections_active==1` via the success_rate detector. If non-convergence: LOWER `reqVolume` / RAISE `interval` (D-S40.3-5 ★), never loosen the assertion.
- [ ] **Step 5:** gofmt/vet/lint. Record fixtures **73 → 74**.
- [ ] **Step 6: Commit** (`phase 40.3 Task 8: 0072 cross-side outlier-detection-success-rate fixture (K=5+1)`).

---

## Task 9: The `0073` cross-side failure-percentage fixture

**Files:**
- Create: `test/fixtures/0073-outlier-detection-failure-percentage/driver/driver.go`
- Create: `test/fixtures/0073-outlier-detection-failure-percentage/driver/driver_test.go`
- Create: `test/fixtures/0073-outlier-detection-failure-percentage/expectations.yaml`
- Create: `test/fixtures/0073-outlier-detection-failure-percentage/README.md`

Model on the `0072` driver (same K=5+1 topology + `PerHostBackendKind` + `HTTP503Responder`). Cluster `c_fp`. The config makes failure_percentage the SOLE enforcer (AMEND-OD3-3): `enforcing_failure_percentage: 100`, `enforcing_success_rate: 0` (success_rate detect-only). `refContainerListenerPort = 19162`. Single-source the constants (mirror `0072`; `failure_percentage_threshold: 85`, `failure_percentage_request_volume: reqVolume`, `failure_percentage_minimum_hosts: 2`):
```yaml
      outlier_detection:
        failure_percentage_threshold: 85
        failure_percentage_minimum_hosts: 2
        failure_percentage_request_volume: 10
        enforcing_failure_percentage: 100
        success_rate_minimum_hosts: 2
        success_rate_request_volume: 10
        enforcing_success_rate: 0
        consecutive_5xx: 0
        consecutive_gateway_failure: 0
        interval: 1s
        base_ejection_time: 30s
        max_ejection_percent: 100
```
(Note: with `enforcing_success_rate: 0` the success_rate detector is detect-only; the bad host's 0% rate may cross the success-rate threshold ⇒ `ejections_detected_success_rate` MAY be ≥ 1 on both sides, but `ejections_enforced_success_rate == 0`. The failure_percentage detector enforces.)

`AssertStats` flow: same drive/poll/warmup/measured-load as `0072`. **Cross-side stat parity** (`StatsAsserter`): `outlier_detection.{ejections_active == 1, ejections_enforced_total == 1, ejections_detected_failure_percentage >= 1, ejections_enforced_failure_percentage >= 1}` AND `ejections_enforced_success_rate == 0` (success_rate detect-only ⇒ a LIVE `== 0` assertion, isolating the failure_percentage enforcer — AMEND-OD3-3) AND `ejections_detected_consecutive_5xx == 0`. Verify `upstream_rq_total > 0` reference-side.

- [ ] **Step 1:** Write `driver_test.go` (`backendIdxFromBody` table test) → run → FAIL.
- [ ] **Step 2:** Write `driver.go` + `expectations.yaml` + `README.md`.
- [ ] **Step 3:** `go test ./test/fixtures/0073-outlier-detection-failure-percentage/driver/ -count=1` (unit) → PASS.
- [ ] **Step 4: Run the cross-side fixture:** `go test ./test/differential/ -run 'TestDifferential/0073' -count=1 -v`. Expected PASS, both sides converge `ejections_active==1` via the failure_percentage detector.
- [ ] **Step 5:** gofmt/vet/lint. Record fixtures **74 → 75**.
- [ ] **Step 6: Commit** (`phase 40.3 Task 9: 0073 cross-side outlier-detection-failure-percentage fixture`).

---

## Task 10: `0072`/`0073` deliberate breaks + 20-run flake

**Files:** none committed (verification only; the SPEC §8.1/§8.2 break protocol).

★ `-count=1` for EVERY break run (`reference_differential_break_protocol_count1`) + the `TestDifferential/<NNNN>` selector.

- [ ] **Step 1: `0072` Break (A) — the success_rate sweep never ejects.** Temporarily make `evalSuccessRate` a no-op (`return` at the top). Run `go test ./test/differential/ -run 'TestDifferential/0072' -count=1` → MUST FAIL (`ejections_active` never reaches 1 — the failure_percentage detector is detect-only, so nothing ejects). Restore.
- [ ] **Step 2: `0072` Break (B) — LB ignores `available`.** Temporarily revert the round-robin leaf consult site (`loadbalancer.go:61`, the 40.1 site) `available → isHealthy`. Run → MUST FAIL (the ejected 503 host stays in rotation → the measured phase leaks 5xx / a host5 body). Restore.
- [ ] **Step 3: `0073` Break (A) — the failure_percentage sweep never ejects.** Temporarily make `evalFailurePercentage` a no-op. Run `go test ./test/differential/ -run 'TestDifferential/0073' -count=1` → MUST FAIL (`ejections_active` never reaches 1 — success_rate is detect-only). Restore.
- [ ] **Step 4: `0073` Break (B) — the failure% boundary comparison is wrong.** Temporarily change `evalFailurePercentage`'s `failures*100 >= threshold*total` to `>` (strict). Run → for a 100%-failure bad host (`100*100 > 85*100` is still true) this does NOT break `0073` — so instead break the per-host-volume gate: temporarily change the eligibility to `w.total > uint64(d.cfg.failurePctReqVolume)*1000` (an unreachable floor) so NO host is eligible → MUST FAIL (the bad host never ejects, never converges). Restore. *(The `>=`-vs-`>` boundary itself is pinned by the Task-5 unit test at failure% exactly 85; the differential break here proves the eligibility gate is load-bearing.)*
- [ ] **Step 5: Confirm all breaks restored** (`git diff` clean; `go test ./test/differential/ -run 'TestDifferential/0072' -count=1` + `-run 'TestDifferential/0073' -count=1` → PASS).
- [ ] **Step 6: 20-run flake gate** for each: `for i in $(seq 20); do go test ./test/differential/ -run 'TestDifferential/0072' -count=1 || echo "FAIL $i"; done` (and `0073`) → 20/20 each. If any flake: LOWER `reqVolume` / RAISE `interval` / widen `convergeDeadline` (D-S40.3-5 ★), NOT a fixed sleep (`reference_health_check_propagation_warmup`).
- [ ] **Step 7:** Record break + flake results in PROGRESS. (No commit — verification only.)

---

## Task 11: Full 75-dir differential + six-gate

**Files:** none (verification + count reconciliation only).

- [ ] **Step 1: Six-gate** (ADR-0052): `go build ./...`; `go vet ./...`; `gofmt -l internal/ test/ cmd/` (empty); `golangci-lint run ./...`; `go test ./internal/... -race -count=1`; `go test ./test/differential/ -count=1` (ALL **75** GREEN — including the unchanged `0069`/`0070`/`0071`, which now run a harmless sweep that emits the +8 names at 0, AMEND-OD3-7).
- [ ] **Step 2: Confirm the `0069`/`0070`/`0071` retro-invariant** (AMEND-OD3-7): those three fixtures pass UNCHANGED (3-host < `minimum_hosts` default 5 → the statistical detectors no-op; the +8 names emit 0 on both sides, not asserted by their `StatsAsserter`s). No edit to them is required — confirm via the full-suite green.
- [ ] **Step 3: Stat-surface count → 1149**; fixtures → 75; fuzzers → 42 (unchanged); BackendKind tail → 35 (unchanged). Record in PROGRESS.
- [ ] **Step 4: Commit** (if any incidental change; otherwise this is a verification gate recorded in PROGRESS — no commit).

---

## Task 12: ADR-0247 body + BEHAVIOR_CONTRACT delta

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` — ADR-0247 full entry (§Decision + §Consequences; promote/refine the §Context already drafted in SPEC §13). DECISIONS tail ADR-0246 → **ADR-0247** (next-free ADR-0248).
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` — extend the `### Cluster — passive health (outlier detection)` subsection (SPEC §9). Advance the stat-surface block **1141 → 1149**.

- [ ] **Step 1:** Write the ADR-0247 body. §Decision: the two external statistical detectors (`success_rate` mean−1.9σ threshold with POPULATION stddev, ejecting-by-default; `failure_percentage` per-host `≥ threshold` cross-multiplied, detect-only-by-default); the per-interval aggregation sweep (snapshot+reset windowed counters → eligible set → eval both detectors in order; `tryEject` CAS once-only per host per sweep); the per-cluster background goroutine + the `StartOutlierDetection`/`odCancel`/`odWG`/`Drain` lifecycle (the first outlier goroutine); the windowed `(intervalTotal, intervalSuccess)` counters fed by the EXISTING seam (NO router touch); the +8 stat registrations (4 external driven + 4 local-origin name-present-logic-deferred); the extended double-count. §Consequences: byte-stable when no `outlier_detection`; the 40.1/40.2 substrate (`tryEject`/seam/ejection-dimension/`available`/lazy-uneject/consecutive-detectors) reused UNCHANGED; the single-enforcing-detector / one-host-one-ejection departure (AMEND-OD3-3, the reference's per-detector double-eject NOT replicated); the deferred local-origin-statistical axis (4 names emit 0) + the 3 legacy-alias departures + the `/clusters`-only readouts; `success_rate_stdev_factor: 0` accepted; the threshold-positivity note; landing 40.3 consumes the 3-leg split → row 40 `done`.
- [ ] **Step 2:** Write the BEHAVIOR_CONTRACT extension + the stat-count bump (1141 → 1149). Record the deferred local-origin-statistical departure, the per-detector-double-eject departure, the 3 legacy-alias departures, and the `success_rate_stdev_factor: 0`-accepted finding.
- [ ] **Step 3:** `go build ./...` (docs-only sanity).
- [ ] **Step 4: Commit** (`phase 40.3 Task 12: ADR-0247 body + BEHAVIOR_CONTRACT passive-health statistical extension (stat 1141→1149)`).

---

## Task 13: Completion bundle + ROADMAP row 40 → `done`

**Files:**
- Modify: `docs/envoy-go/STATE.md` — prepend the `phase 40.3 IMPL done` active-phase block (demote the SPEC block to prior); update lifecycle-state / next-skill / last-commit / last-updated / phase-directory; the count footer → stat **1149** / fixtures **75** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0247** (next-free ADR-0248).
- Modify: `docs/envoy-go/ROADMAP.md` — **row 40 flips `in-progress → done`** (the 40.3 final leg consumes the 3-leg by-detector-class split; `reference_roadmap_split_phase_row_done`; NO parent rollup per ADR-0106). The Upstream-robustness family STAYS OPEN (3 candidates remain: circuit breakers / retries + hedging / per-protocol connection pooling).
- Modify: `docs/envoy-go/phases/40.3-outlier-detection-statistical/{PROGRESS.md,README.md}` — final state + exit counts.
- Modify: `next-prompt.txt` — roll the cold-start forward to a new subject (the phase-40 outlier-detection family is COMPLETE; the next phase is a fresh BRAINSTORM — likely the next Upstream-robustness candidate or a new family row). SHA-fill after the squash.

- [ ] **Step 1:** Update STATE.md + ROADMAP.md (row 40 → `done`) + PROGRESS.md + README.md with the final counts.
- [ ] **Step 2: Final six-gate re-run** (ADR-0052) to confirm the doc edits did not break the build/tests.
- [ ] **Step 3: Commit** the bundle (`phase 40.3 Task 13: completion bundle — STATE/ROADMAP (row 40 done) + next-prompt roll-forward`).
- [ ] **Step 4:** Controller (NOT a subagent — `feedback_subagents_no_push`) squashes the branch, ff-merges to master, pushes, rolls next-prompt with the squash SHA, removes the worktree, deletes the branch (superpowers:finishing-a-development-branch).

---

## Exit deltas (SPEC §14)

| Count | Before (SPEC) | After (IMPL) |
|---|---|---|
| Stat surface | 1141 | **1149** (+8) |
| Fixtures | 73 | **75** (`0072` + `0073`) |
| Fuzzers | 42 | 42 (unchanged) |
| BackendKind tail | 35 | 35 (unchanged — both fixtures reuse `HTTP503Responder` + `PerHostBackendKind`) |
| DECISIONS tail | ADR-0246 | **ADR-0247** (next-free ADR-0248) |
| New Go packages | — | 0 |
| New go.mod modules | — | 0 |

**ROADMAP row 40 flips `in-progress → done`** at the Task-11 six-gate / Task-13 bundle (the 40.3 final leg consumes the 3-leg by-detector-class split). The FINAL ADR-0045 split-gate (D-S40.3-7): NO FURTHER SPLIT.
