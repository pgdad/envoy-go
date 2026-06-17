# Phase 40.2 Implementation Plan — `outlier_detection`: the gateway + local-origin consecutive detectors + the split accounting switch

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the remaining consecutive passive-outlier detectors over the 40.1 substrate — `consecutive_gateway_failure` (eject on N consecutive gateway-class 5xx `{502,503,504}`), `consecutive_local_origin_failure` (eject on N consecutive connect/reset failures, split=true only), and the `split_external_local_origin_errors` accounting switch — making the 40.1-reserved `UpstreamResult.LocalOriginErr` field live, with NO router-seam re-design and byte-identical behavior for clusters without `outlier_detection`.

**Architecture:** The 40.1 `outlierDetector.record(ep, statusCode)` is extended to `record(ep, statusCode, localOriginErr)` and the 40.1 inline eject is refactored into a reusable `tryEject(enforcing, enforcedCounter) bool` shared by all three detectors. `recordExternal5xx` implements the live-pinned gateway-first ordering (gateway-class 5xx increments BOTH the gateway and 5xx consecutive counters; a gateway *ejection* short-circuits the 5xx detector → `detected_consecutive_5xx` stays 0; a gateway *non-eject* falls through so both counters advance). The `split` switch routes `LocalOriginErr` results: split=true ⇒ a dedicated local-origin detector only; split=false ⇒ mapped to a gateway-class 5xx (the local-reply codes 502/503 are gateway-class). The seam fires at the five router connection-failure sites with `LocalOriginErr: true`; the one framework touch surfaces the picked endpoint on the shared `Cluster.Dial`/`AcquireH1` connect-failure path (today both discard it as `Endpoint{}`). The 40.1 seam (success sites), ejection dimension, `available` predicate, lazy un-eject, and `max_ejection_percent` cap are REUSED UNCHANGED.

**Tech Stack:** Go; `github.com/envoyproxy/go-control-plane` v1.32.4 (`cluster.v3.OutlierDetection` fields 10–14 — already vendored, ZERO new go.mod modules); the internal `stats` registry; the differential harness (`contrib-v1.37.2`, Docker bridge, poll-to-converge + warmup + delta counters).

**Reference docs (read alongside this plan):**
- SPEC: `docs/envoy-go/phases/40.2-outlier-detection-other-consecutive/SPEC.md` (the §1.1 AMEND-OD2-1..6 amendments; §3 architecture; §5 proto roster fields 10–14; §6 reject roster; §7 stat surface +4; §8 `0070`/`0071` fixtures; §11 D-OD2-* live pins; §13 ADR-0246 §Context).
- The 40.1 substrate (the leg this extends): `docs/envoy-go/phases/40.1-outlier-detection-consecutive-5xx/{SPEC,PLAN}.md` + `internal/cluster/outlier.go` (the `record`/`parseOutlierDetection`/`outlierDetector` 40.2 extends) + ADR-0245 in `docs/envoy-go/DECISIONS.md`.

---

## D-question resolutions (SPEC §12) — settled at PLAN

These were the open PLAN/IMPL D-questions in SPEC §12. The implementer MUST follow these (they are baked into the tasks below).

### D-S40.2-1 — house reject wording (§6)
Two new arms, the 40.1 house prefix `cluster: %q: outlier_detection: ` + the reason (mirroring the reference PGV wording verbatim, ADR-0080):
- `cluster: %q: outlier_detection: enforcing_consecutive_gateway_failure: value must be less than or equal to 100`
- `cluster: %q: outlier_detection: enforcing_consecutive_local_origin_failure: value must be less than or equal to 100`

Both are `*wrapperspb.UInt32Value` — validate only when non-nil (`> 100`). The four 40.1 reject arms are UNCHANGED.

### D-S40.2-2 — the `record` reset ordering replicating `detected_consecutive_5xx == 0`-on-gateway-eject ★ load-bearing
Refactor the 40.1 inline eject into `tryEject(ep, h, enforcing, enforcedCounter) bool` (returns true iff it ejected). `recordExternal5xx` runs the gateway detector FIRST; if it EJECTS, return immediately (the 5xx detector's `consec5xx.Add` does not run this call → `detected_consecutive_5xx` never reaches threshold → stays 0, the live pin); if it does NOT eject (detect-only `enforcing_consecutive_gateway_failure` 0, or threshold not reached), FALL THROUGH so `consec5xx.Add` also runs (both consecutive counters advance — the AMEND-OD2-2 invariant; this is the `0069`/`0070` default path where gateway is detect-only and the 5xx detector ejects). No separate "host already ejected ⇒ skip detected" guard is needed beyond `tryEject`'s own `h.ejected.Load()` fast-path — the short-circuit-on-gateway-eject is the mechanism. (The "5xx stays 0 when BOTH thresholds are equal + gateway enforces" case is UNIT-tested in Task 3; in `0070` the fixture sets `consecutive_5xx` HIGH so the 5xx detector cannot fire regardless, and cross-asserts `detected_consecutive_5xx == 0`.)

### D-S40.2-3 — surfacing the picked endpoint on the connect-failure path
Widen `Cluster.AcquireH1` to `(*PooledH1Conn, Endpoint, error)` (mirroring `DialH2`'s `(…, Endpoint, error)`): return the picked `ep` on the inline dial/TLS-failure paths (`cluster.go:374-377` + `:381-385`), `Endpoint{}` on the LB-pick-failure path (`:343-345`, no host selected). Surface `ep` on the shared `Cluster.Dial`'s `DialContext`/handshake-failure returns (`cluster.go:295` + `:303` — change `return nil, Endpoint{}, …` → `return nil, ep, …` on those two paths only, NOT the pick-failure path `:289`), and propagate it through `DialH2` (`dial_h2.go` — change its `c.Dial` error return + its own handshake/clientconn-failure returns from `Endpoint{}` → `ep`; `DialH2`'s signature already returns `Endpoint`, so this is a value change only, no signature change). This is behavior-neutral for existing callers (they ignore the returned `ep` on error). `AcquireH1` has exactly TWO callers — `router.go:610` (live, the seam) + `cluster_test.go` (tests) — so the signature widening is contained; update both. The legacy `(*routerAction).do` (uses `Dial`, not `AcquireH1`)/`(*routerActionH2).doH2` drivers and `DialH2`'s legacy caller (`router_h2.go:271`) stay seam-free (they ignore the propagated `ep`).

★ **`Endpoint` is NOT comparable** — it carries a `Metadata map[string]SubsetValue` field, so `ep != Endpoint{}` does NOT compile. Task 6 MUST add a `func (e Endpoint) IsZero() bool { return e.Host == "" && e.Port == 0 }` helper to `cluster.go` (the pick-failure paths return a literal `Endpoint{}` ⇒ `Host=="" && Port==0` is a correct zero-detector; `Addr()` already ignores `Metadata`), and the router seam guards use `!ep.IsZero()`.

### D-S40.2-4 — the local-origin classification boundary
All connection-level failures map to a single `LocalOriginErr: true` (the recorded simplification, SPEC §2). The post-connect-reset sites ARE in the local-origin class at the MVP: H1 `req.Write` 502 (`router.go:637-640`) + H1 `http.ReadResponse` 502 (`:648-652`) + H2 `RoundTrip` non-ctx-cancel 502 (`router_h2.go:96-98`) all populate `LocalOriginErr: true`, alongside the pure connect failures H1 `AcquireH1` 503 (`:610-614`) + H2 `DialH2` 502 (`:76-80`). The H2 ctx-cancel/deadline sentinel path (`router_h2.go:90-95`) is a DOWNSTREAM cancel → NO seam call.

### D-S40.2-5 — `0070`/`0071` constants single-sourced + the `0069` widening
Each driver's workload constants live in one `const`/`var` block (the `0069` precedent, `reference_fixture_workload_constant_desync`): `fixtureName`, `refContainerListenerPort` (`0070` = **19159**, `0071` = **19160** — next free after `0069`'s 19158; verify next-free at IMPL), `refAdminPort = 9901`, the backend counts, the detector thresholds, `baseEjectionTime`, `interval`, `maxEjectionPercent = 100`, `n`, `convergeDeadline`/`convergePoll`, `warmupStable`. Separately, the EXISTING `0069` driver's `StatsAsserter` is WIDENED in Task 10 to cross-assert `ejections_detected_consecutive_gateway_failure` (AMEND-OD2-1 — at 40.2 envoy-go now emits it on `0069`'s 503s, matching the reference, which already did; the `0069` exclusion comment is removed).

### D-S40.2-6 — `0070`/`0071` two dirs vs one folded dir
**TWO separate fixture dirs** (`0070-outlier-detection-consecutive-gateway-failure` + `0071-outlier-detection-local-origin`) — the `0067`/`0068` two-dir precedent (TCP vs gRPC health-check codecs). Each is its own cross-side runner branch (`reference_differential_fixture_dispatch_constraint` satisfied; both are cross-side, no boot-reject mixing). Cleaner separation of the gateway (HTTP503Responder topology) vs local-origin (dead-host topology) arms.

### D-S40.2-7 — final ADR-0045 split-gate re-check
Anticipated production LoC (below) ≈ **~150–260 LoC** across ~5 prod files (mostly `outlier.go` + the seam threading); ~12 tasks. Both comfortably under the ADR-0045 gate (`> ~25 tasks OR > ~1500 LoC`). **NO FURTHER SPLIT** — 40.2 ships as one flat leg. (40.3 remains the pre-authorized statistical-detector leg.)

---

## File structure

**Production (`internal/`):**
- `internal/cluster/health.go` (MODIFY) — add `consecGw atomic.Uint32` + `consecLO atomic.Uint32` to `hostHealth` (the gateway + local-origin consecutive counters, alongside the 40.1 `consec5xx`).
- `internal/cluster/outlier.go` (MODIFY) — extend `outlierConfig` (the 7 new gateway/local-origin/split fields); extend `parseOutlierDetection` (defaults + the 2 reject arms); refactor the eject into `tryEject`; rewrite `record` → `record(ep, statusCode, localOriginErr)` with `recordExternal5xx` (gateway-first) + `recordLocalOrigin`; add the +4 stat handles + extend `registerStats`.
- `internal/cluster/cluster.go` (MODIFY) — `RecordUpstreamResult` passes `r.LocalOriginErr`; widen `AcquireH1` → `(*PooledH1Conn, Endpoint, error)` + surface `ep` on its dial/TLS-failure paths; surface `ep` on `Cluster.Dial`'s connect/handshake-failure returns.
- `internal/cluster/dial_h2.go` (MODIFY) — propagate `ep` through `DialH2`'s error returns.
- `internal/filter/http/router/router.go` (MODIFY) — consume `AcquireH1`'s new `ep` return; populate `LocalOriginErr: true` at the H1 503/502 failure sites.
- `internal/filter/http/router/router_h2.go` (MODIFY) — populate `LocalOriginErr: true` at the H2 502 failure sites (using `DialH2`'s `ep` / the post-dial `picked`).

**Test harness (`test/`):**
- `test/fixtures/0070-outlier-detection-consecutive-gateway-failure/{driver/driver.go,driver/driver_test.go,expectations.yaml,README.md}` (CREATE) — gateway fixture (reuses `HTTP503Responder` + `PerHostBackendKind`).
- `test/fixtures/0071-outlier-detection-local-origin/{driver/driver.go,driver/driver_test.go,expectations.yaml,README.md}` (CREATE) — local-origin fixture (reuses the `allocDeadPort` dead-host mechanism).
- `test/fixtures/0069-outlier-detection-consecutive-5xx/driver/driver.go` (MODIFY) — widen the `StatsAsserter` to cross-assert `ejections_detected_consecutive_gateway_failure`.

**Docs:**
- `docs/envoy-go/DECISIONS.md` (ADR-0246 body), `BEHAVIOR_CONTRACT.md` (passive-health subsection extension + stat-count 1137 → 1141), `STATE.md`, `ROADMAP.md`, `next-prompt.txt`, the phase `PROGRESS.md` + `README.md`.

---

## Task 1: Baselines + PROGRESS scaffold

**Files:**
- Create: `docs/envoy-go/phases/40.2-outlier-detection-other-consecutive/PROGRESS.md`

- [ ] **Step 1: Capture the pre-IMPL six-gate baseline.** Run, record in PROGRESS.md:
  - `go build ./...`
  - `go vet ./...`
  - `gofmt -l internal/ test/` (expect empty)
  - `go test ./internal/... 2>&1 | tail -20`
  - `go test ./test/differential/ -count=1 2>&1 | tail -20` (the full **71**-dir suite — the byte-stability anchor)
  - Stat surface: tracked as a documented running total (no count-script). Record the baseline **1137** (SPEC §14); the 40.2 exit total is verified arithmetically (1137 + 4 = 1141) against the Task 5 registration test.
- [ ] **Step 2: Record the baselines + the task checklist** in PROGRESS.md (counts: stat 1137 / fixtures 71 / fuzzers 42 / BackendKind tail 35 / DECISIONS tail ADR-0245, next-free ADR-0246; the anticipated exit deltas from SPEC §14).
- [ ] **Step 3: Commit.**
```bash
git add docs/envoy-go/phases/40.2-outlier-detection-other-consecutive/PROGRESS.md
git commit -m "phase 40.2 Task 1: PROGRESS scaffold + pre-IMPL baselines"
```

---

## Task 2: `parseOutlierDetection` extension (gateway/local-origin/split) + the `consecGw`/`consecLO` counters + the 2 reject arms

**Files:**
- Modify: `internal/cluster/health.go` — add `consecGw`/`consecLO` to `hostHealth`.
- Modify: `internal/cluster/outlier.go` — extend `outlierConfig` + `parseOutlierDetection`.
- Test: `internal/cluster/outlier_test.go`

Extend `hostHealth` (alongside the 40.1 `consec5xx`):
```go
consecGw atomic.Uint32 // consecutive gateway-class 5xx {502,503,504} (ADR-0246)
consecLO atomic.Uint32 // consecutive local-origin failures (split=true only) (ADR-0246)
```

Extend `outlierConfig`:
```go
// gateway detector — active-by-default (detect-only by default; AMEND-OD2-1).
consecGwEnabled bool   // false iff consecutive_gateway_failure explicitly 0
consecutiveGw   uint32 // threshold (default 5)
enforcingGw     uint32 // enforce-roll % (default 0 ⇒ detect-only)
// local-origin detector — takes effect only when split is true.
splitLocalOrigin bool   // split_external_local_origin_errors (default false)
consecLOEnabled  bool   // false iff consecutive_local_origin_failure explicitly 0
consecutiveLO    uint32 // threshold (default 5)
enforcingLO      uint32 // enforce-roll % (default 100)
```

Extend `parseOutlierDetection` (defaults SPEC §5, confirmed live; the `consecutive_5xx` absent-vs-explicit-0 pattern from 40.1 applies to the two new thresholds):
```go
// gateway (default 5; active-by-default; enforcing default 0).
if v := od.GetConsecutiveGatewayFailure(); v == nil {
    cfg.consecutiveGw = 5
    cfg.consecGwEnabled = true
} else {
    cfg.consecutiveGw = v.GetValue()
    cfg.consecGwEnabled = cfg.consecutiveGw != 0
}
cfg.enforcingGw = 0 // default
if v := od.GetEnforcingConsecutiveGatewayFailure(); v != nil {
    cfg.enforcingGw = v.GetValue()
}
// split (default false).
cfg.splitLocalOrigin = od.GetSplitExternalLocalOriginErrors()
// local-origin (default 5; enforcing default 100).
if v := od.GetConsecutiveLocalOriginFailure(); v == nil {
    cfg.consecutiveLO = 5
    cfg.consecLOEnabled = true
} else {
    cfg.consecutiveLO = v.GetValue()
    cfg.consecLOEnabled = cfg.consecutiveLO != 0
}
cfg.enforcingLO = 100 // default
if v := od.GetEnforcingConsecutiveLocalOriginFailure(); v != nil {
    cfg.enforcingLO = v.GetValue()
}
```
Add the 2 reject arms (D-S40.2-1) alongside the 40.1 arms (validate only when non-nil):
```go
if v := od.GetEnforcingConsecutiveGatewayFailure(); v != nil && v.GetValue() > 100 {
    return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_consecutive_gateway_failure: value must be less than or equal to 100", name)
}
if v := od.GetEnforcingConsecutiveLocalOriginFailure(); v != nil && v.GetValue() > 100 {
    return nil, fmt.Errorf("cluster: %q: outlier_detection: enforcing_consecutive_local_origin_failure: value must be less than or equal to 100", name)
}
```

- [ ] **Step 1: Write failing tests** (`outlier_test.go`): a full gateway+local-origin+split config parses with correct values; absent `consecutive_gateway_failure` ⇒ threshold 5 + enabled + `enforcingGw == 0`; explicit `consecutive_gateway_failure: 0` ⇒ `consecGwEnabled == false`; `split_external_local_origin_errors: true` ⇒ `splitLocalOrigin == true`; absent `consecutive_local_origin_failure` ⇒ 5 + enabled + `enforcingLO == 100`; the TWO new reject arms (`enforcing_consecutive_gateway_failure: 101` / `enforcing_consecutive_local_origin_failure: 101`) each produce the exact house string. Existing 40.1 parse tests still pass (the new fields default correctly when absent).
- [ ] **Step 2: Run → FAIL** (`go test ./internal/cluster/ -run 'TestParseOutlier' -v`).
- [ ] **Step 3: Implement** the `hostHealth` fields + the `outlierConfig` fields + the `parseOutlierDetection` extension.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** `gofmt -l internal/cluster/ && go vet ./internal/cluster/ && golangci-lint run internal/cluster/...` (per `feedback_pertask_gofmt_lint`).
- [ ] **Step 6: Commit** (`phase 40.2 Task 2: parseOutlierDetection gateway/local-origin/split fields + consecGw/consecLO counters + 2 reject arms`).

---

## Task 3: Refactor the eject into `tryEject` + the gateway-detector path (`recordExternal5xx`, gateway-first)

**Files:**
- Modify: `internal/cluster/outlier.go` — extract `tryEject`; rewrite `record` → `record(ep, statusCode, localOriginErr)` with `recordExternal5xx`; add the +2 gateway stat handles to the detector struct.
- Modify: `internal/cluster/cluster.go` — `RecordUpstreamResult` passes `r.LocalOriginErr`.
- Test: `internal/cluster/outlier_test.go`

Add to the `outlierDetector` struct (alongside the 40.1 handles; local-origin handles added in Task 4):
```go
ejectionsDetectedGw *stats.Counter
ejectionsEnforcedGw *stats.Counter
```

Refactor the 40.1 inline eject (currently `outlier.go:145-172`) into:
```go
// tryEject runs the enforce-roll + max_ejection_percent cap + CAS once-only
// eject for one threshold crossing. Returns true iff THIS call ejected the host.
// The detected counter is the caller's responsibility (Inc'd before tryEject).
// (ADR-0246; refactor of the 40.1 inline eject — REUSED by all three detectors.)
func (d *outlierDetector) tryEject(ep Endpoint, h *hostHealth, enforcing uint32, enforced *stats.Counter) bool {
    if h.ejected.Load() {
        return false // already ejected (fast-path)
    }
    enforce := enforcing >= 100 || (enforcing != 0 && d.enforceRoll() < enforcing)
    if !enforce {
        return false // detect-only
    }
    total := len(d.endpoints)
    // cross-multiplied cap (reference_percent_cap_cross_multiply): eject iff
    // (ejected+1)*100 <= cap*total.
    if total == 0 || (d.health.ejectedCount(d.endpoints)+1)*100 > int(d.cfg.maxEjectionPct)*total {
        if d.ejectionsOverflow != nil {
            d.ejectionsOverflow.Inc()
        }
        return false
    }
    if !h.ejected.CompareAndSwap(false, true) {
        return false
    }
    h.unejectAtNanos.Store(d.health.nowNanos() + d.cfg.baseEjectionTime.Nanoseconds())
    h.ejectCount.Add(1)
    if d.ejectionsActive != nil {
        d.ejectionsActive.Inc()
    }
    if d.ejectionsEnforcedTotal != nil {
        d.ejectionsEnforcedTotal.Inc() // the cross-detector double-count
    }
    if enforced != nil {
        enforced.Inc() // the per-detector enforced counter
    }
    return true
}
```
Rewrite `record` (the gateway path; the local-origin branch is a Task-4 stub returning for now):
```go
func (d *outlierDetector) record(ep Endpoint, statusCode int, localOriginErr bool) {
    h, ok := d.health.states[ep.Addr()]
    if !ok {
        return
    }
    _ = d.health.isEjected(ep) // fast-path lazy-uneject refresh

    if localOriginErr {
        d.recordLocalOrigin(ep, h, statusCode) // Task 4 implements; Task 3 stub: return
        return
    }
    // external HTTP status
    if d.cfg.splitLocalOrigin {
        h.consecLO.Store(0) // a completed external response ⇒ connection succeeded ⇒ reset LO streak
    }
    if statusCode < 500 || statusCode >= 600 {
        h.consec5xx.Store(0)
        h.consecGw.Store(0)
        return
    }
    d.recordExternal5xx(ep, h, statusCode)
}

// recordExternal5xx applies one external 5xx, gateway detector FIRST (AMEND-OD2-2).
func (d *outlierDetector) recordExternal5xx(ep Endpoint, h *hostHealth, statusCode int) {
    gateway := statusCode == 502 || statusCode == 503 || statusCode == 504
    if gateway {
        if d.cfg.consecGwEnabled {
            if h.consecGw.Add(1) >= d.cfg.consecutiveGw {
                if d.ejectionsDetectedGw != nil {
                    d.ejectionsDetectedGw.Inc()
                }
                if d.tryEject(ep, h, d.cfg.enforcingGw, d.ejectionsEnforcedGw) {
                    return // gateway ejected → the 5xx detector does NOT fire this call (detected_5xx stays 0)
                }
            }
        }
    } else {
        h.consecGw.Store(0) // a non-gateway 5xx breaks the gateway streak
    }
    // fall through to the 5xx detector (the 40.1 path, UNCHANGED)
    if !d.cfg.consec5xxEnabled {
        return
    }
    if h.consec5xx.Add(1) >= d.cfg.consecutive5xx {
        if d.ejectionsDetected5xx != nil {
            d.ejectionsDetected5xx.Inc()
        }
        d.tryEject(ep, h, d.cfg.enforcing5xx, d.ejectionsEnforced5xx)
    }
}
```
`RecordUpstreamResult` (`cluster.go`): `c.outlier.record(ep, r.StatusCode)` → `c.outlier.record(ep, r.StatusCode, r.LocalOriginErr)`. Update the existing 40.1 `record(ep, statusCode)` unit-test call sites in `outlier_test.go` to `record(ep, statusCode, false)`.

- [ ] **Step 1: Write failing tests** (deterministic clock + injected `enforceRoll`): gateway eject — N consecutive 503 with `enforcing_consecutive_gateway_failure: 100` ejects + bumps `ejectionsDetectedGw`/`ejectionsEnforcedGw`/`ejectionsEnforcedTotal`/`ejectionsActive` (the double-count) AND leaves `ejectionsDetected5xx == 0` (the gateway-first short-circuit, BOTH thresholds equal); gateway detect-only (`enforcing_consecutive_gateway_failure: 0`, `consecutive_5xx: N` enforcing 100) — a 503 bumps `ejectionsDetectedGw` AND falls through so the 5xx detector ejects (`ejectionsDetected5xx`/`ejectionsEnforced5xx` bump — the `0069` behavior); a non-gateway 5xx (500) resets `consecGw` and counts only via `consec5xx`; a 2xx resets both `consec5xx` and `consecGw`. The 40.1 `consecutive_5xx` tests still pass (updated to the 3-arg `record`).
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `tryEject` + the `record`/`recordExternal5xx` rewrite (with a `recordLocalOrigin` stub that returns) + the `RecordUpstreamResult` 3-arg call + the test-call-site updates.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 40.2 Task 3: tryEject refactor + gateway-detector path (gateway-first ordering)`).

---

## Task 4: The local-origin detector + the `split` branch

**Files:**
- Modify: `internal/cluster/outlier.go` — implement `recordLocalOrigin`; add the +2 local-origin stat handles.
- Test: `internal/cluster/outlier_test.go`

Add to the `outlierDetector` struct:
```go
ejectionsDetectedLO *stats.Counter
ejectionsEnforcedLO *stats.Counter
```
Implement `recordLocalOrigin` (called from `record`'s `localOriginErr` branch):
```go
// recordLocalOrigin applies one local-origin failure (connect/reset). When split
// is enabled it feeds ONLY the local-origin detector; when split is disabled the
// failure is mapped to a gateway-class 5xx (the caller passes the 502/503
// local-reply code, both gateway-class) and fed to the gateway/5xx detectors.
func (d *outlierDetector) recordLocalOrigin(ep Endpoint, h *hostHealth, statusCode int) {
    if !d.cfg.splitLocalOrigin {
        d.recordExternal5xx(ep, h, statusCode) // split=false: count as gateway-class 5xx (AMEND-OD2-3)
        return
    }
    if !d.cfg.consecLOEnabled {
        return
    }
    if h.consecLO.Add(1) >= d.cfg.consecutiveLO {
        if d.ejectionsDetectedLO != nil {
            d.ejectionsDetectedLO.Inc()
        }
        d.tryEject(ep, h, d.cfg.enforcingLO, d.ejectionsEnforcedLO)
    }
}
```
NOTE the `record` external-status branch already resets `consecLO` on a completed external response under split=true (Task 3) — that is the local-origin "success reset" (a response arriving means the connection succeeded).

- [ ] **Step 1: Write failing tests** (deterministic clock + injected roll):
  - split=true: N consecutive `record(ep, 502, true)` ejects via the local-origin detector (`ejectionsDetectedLO`/`ejectionsEnforcedLO`/`ejectionsEnforcedTotal`/`ejectionsActive` bump) AND leaves `ejectionsDetected5xx == 0` AND `ejectionsDetectedGw == 0` (split routes local-origin away from the external detectors).
  - split=true: a successful external response (`record(ep, 200, false)`) mid-streak resets `consecLO` (no eject after fewer than N more local-origin failures).
  - split=false (default): N consecutive `record(ep, 503, true)` ejects via the gateway/5xx detectors (`ejectionsDetected5xx`/gateway bump per the gateway-first ordering; `ejectionsDetectedLO == 0` — the local-origin detector is inactive).
  - split=true with `consecutive_local_origin_failure: 0` ⇒ never ejects on local-origin failures.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** `recordLocalOrigin` (replacing the Task-3 stub) + the +2 handles.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/cluster/ -count=1`.
- [ ] **Step 5:** gofmt/vet/lint.
- [ ] **Step 6: Commit** (`phase 40.2 Task 4: local-origin detector + split_external_local_origin_errors branch`).

---

## Task 5: The +4 stat registrations (extend `registerStats`)

**Files:**
- Modify: `internal/cluster/outlier.go` — extend `registerStats` to allocate + assign the +4 handles.
- Test: `internal/cluster/outlier_test.go` (or the manager stat test).

Extend `registerStats` (currently `outlier.go:100-110`) — register all 4 new handles UNCONDITIONALLY on any outlier cluster (the reference exposes all `outlier_detection.*` names on every outlier cluster regardless of which detectors are configured; D-OD2-STATS):
```go
d.ejectionsDetectedGw = r.NewCounter(op + "ejections_detected_consecutive_gateway_failure")
d.ejectionsEnforcedGw = r.NewCounter(op + "ejections_enforced_consecutive_gateway_failure")
d.ejectionsDetectedLO = r.NewCounter(op + "ejections_detected_consecutive_local_origin_failure")
d.ejectionsEnforcedLO = r.NewCounter(op + "ejections_enforced_consecutive_local_origin_failure")
```

- [ ] **Step 1: Write a failing test** asserting a cluster WITH `outlier_detection` registers the 9 named outlier stats (the 40.1 five + these four) and a cluster WITHOUT registers none. Extend the 40.1 stat-registration test.
- [ ] **Step 2: Run → FAIL.**
- [ ] **Step 3: Implement** the +4 registrations.
- [ ] **Step 4: Run → PASS** + full `go test ./internal/... -count=1`.
- [ ] **Step 5: Stat-surface count → 1141** (1137 + 4). Record in PROGRESS.
- [ ] **Step 6:** gofmt/vet/lint.
- [ ] **Step 7: Commit** (`phase 40.2 Task 5: +4 gateway/local-origin outlier_detection stat registrations (1137→1141)`).

---

## Task 6: The seam — surface the picked endpoint + populate `LocalOriginErr` at the five connection-failure sites

**Files:**
- Modify: `internal/cluster/cluster.go` — widen `AcquireH1` → `(*PooledH1Conn, Endpoint, error)`; surface `ep` on its dial/TLS-failure returns; surface `ep` on `Cluster.Dial`'s connect/handshake-failure returns; add `func (e Endpoint) IsZero() bool` (D-S40.2-3 — `Endpoint` is NOT comparable, has a `Metadata` map).
- Modify: `internal/cluster/dial_h2.go` — propagate `ep` through `DialH2`'s error returns.
- Modify: `internal/cluster/cluster_test.go` — update the `AcquireH1` call site(s) to the 3-return signature.
- Modify: `internal/filter/http/router/router.go` — consume `AcquireH1`'s `ep`; add the seam at the H1 failure sites.
- Modify: `internal/filter/http/router/router_h2.go` — add the seam at the H2 failure sites.

★ **Dead-driver discipline** (SPEC AMEND-OD2-4, the 40.1 AMEND-OD5 sibling): the seam goes in the LIVE drivers `doH1ClusterAction` / `doH2ClusterAction` ONLY — NOT the legacy `(*routerAction).do` / `(*routerActionH2).doH2`. The picked-endpoint surfacing is behavior-neutral for the legacy callers (they ignore the returned `ep`).

`cluster.go` — `Dial` connect/handshake-failure returns (NOT the pick-failure return):
```go
raw, err := d.DialContext(ctx, "tcp", ep.Addr())
if err != nil {
    release()
    return nil, ep, fmt.Errorf("cluster: dial: %w", err) // was: Endpoint{}
}
// ... handshake failure likewise: return nil, ep, fmt.Errorf("cluster: tls: handshake: %w", err)
```
`AcquireH1` — widen to `(*PooledH1Conn, Endpoint, error)`; the LB-pick failure returns `Endpoint{}`; the inline dial/TLS failures return `ep`; the happy path returns `ep` (or the pooled conn's ep). `dial_h2.go` — `DialH2` returns `ep` on `c.Dial`'s error (and its own handshake/clientconn failures use the `ep` from `c.Dial`).

`router.go` (`doH1ClusterAction`):
```go
pooled, ep, err := a.cluster.AcquireH1(ctx)
if err != nil {
    a.cluster.IncStatusClass(503)
    if !ep.IsZero() { // a host was picked → attribute the local-origin connect failure
        a.cluster.RecordUpstreamResult(ep, cluster.UpstreamResult{StatusCode: 503, LocalOriginErr: true})
    }
    return ActionResponse{Status: 503, Headers: localReplyHeaders(0), Body: nil}, picked, nil
}
// ... at req.Write failure (502) and http.ReadResponse failure (502), picked is set:
a.cluster.IncStatusClass(502)
a.cluster.RecordUpstreamResult(picked, cluster.UpstreamResult{StatusCode: 502, LocalOriginErr: true})
return ActionResponse{Status: 502, ...}, picked, nil
```
(`Endpoint` is NOT comparable — `!ep.IsZero()` (the new helper, D-S40.2-3), NOT `ep != Endpoint{}`. The success site at `:655-656` stays UNCHANGED — `LocalOriginErr: false`.)

`router_h2.go` (`doH2ClusterAction`): `DialH2` failure (`:76-80`) — `cc, ep, err := a.cluster.DialH2(ctx)`; on error, if `!ep.IsZero()` call `RecordUpstreamResult(ep, {StatusCode: 502, LocalOriginErr: true})`. `RoundTrip` non-ctx-cancel failure (`:96-98`) — `picked` is set; call `RecordUpstreamResult(picked, {StatusCode: 502, LocalOriginErr: true})`. The ctx-cancel sentinel (`:90-95`) → NO seam call. The success site at `:100-101` stays UNCHANGED.

- [ ] **Step 1: Write failing tests:** in `cluster_test.go`, an `AcquireH1` against an unreachable endpoint returns a non-zero `ep` with the error (the surfaced picked host); an `AcquireH1` against an empty/all-unavailable cluster returns `Endpoint{}` with the error (no host). (The router-level local-origin wiring is exercised end-to-end by `0071` in Task 8; a unit-level router test is optional — the build + the `0071` fixture are the proof, mirroring the 40.1 Task 7 posture.)
- [ ] **Step 2: Run → FAIL** (signature/return mismatch).
- [ ] **Step 3: Implement** the `(Endpoint).IsZero()` helper + the `Dial`/`AcquireH1`/`DialH2` endpoint surfacing + the router failure-site seam calls (guarded by `!ep.IsZero()`) + the `cluster_test.go` call-site update.
- [ ] **Step 4: Run → PASS:** `go build ./...` + `go test ./internal/cluster/ ./internal/filter/http/router/ -count=1`.
- [ ] **Step 5:** gofmt/vet/lint on `internal/cluster/...` + the router package.
- [ ] **Step 6: Commit** (`phase 40.2 Task 6: surface picked endpoint on connect failure + LocalOriginErr seam at the 5 router failure sites`).

---

## Task 7: The `0070` cross-side gateway fixture

**Files:**
- Create: `test/fixtures/0070-outlier-detection-consecutive-gateway-failure/driver/driver.go`
- Create: `test/fixtures/0070-outlier-detection-consecutive-gateway-failure/driver/driver_test.go` (the `backendIdxFromBody` table test, the `0069` precedent)
- Create: `test/fixtures/0070-outlier-detection-consecutive-gateway-failure/expectations.yaml`
- Create: `test/fixtures/0070-outlier-detection-consecutive-gateway-failure/README.md`

Model closely on `test/fixtures/0069-outlier-detection-consecutive-5xx/driver/driver.go`. Topology: cluster `c_gw`, lb ROUND_ROBIN, 3 endpoints {host0 HTTPEcho, host1 HTTPEcho, host2 HTTP503Responder}, `outlier_detection: { consecutive_gateway_failure: 5, enforcing_consecutive_gateway_failure: 100, consecutive_5xx: 100, interval: 10s, base_ejection_time: 30s, max_ejection_percent: 100 }` (consecutive_5xx HIGH so the 5xx detector cannot fire → `detected_consecutive_5xx == 0` regardless of ordering — D-S40.2-2), on BOTH sides. The driver implements `BackendCount() == 3` + `BackendKindAt(i)` (0,1 ⇒ HTTPEcho; 2 ⇒ HTTP503Responder) — REUSES the 40.1 `PerHostBackendKind` interface + `HTTP503Responder` BackendKind 35 (NO new runner change). `refContainerListenerPort = 19159`.

`AssertStats` flow (the `0069` template):
1. **Ejection drive:** round-robin GET / until host2 accrues `consecutive_gateway_failure` (5) consecutive gateway 5xx (per-host streak, reset only by a 2xx FROM host2, which never comes) and is ejected — ~5×3 ≈ 15 requests; single-source the count.
2. **POLL `/stats` on BOTH sides** until `cluster.c_gw.outlier_detection.ejections_active == 1` (deadline `convergeDeadline`, every `convergePoll`; NO fixed sleep).
3. **Warmup-until-K-200s** + **measured load phase** (delta counters): 100% `upstream_rq_2xx`, 0 `upstream_rq_5xx`, served by host0/host1.
4. **Cross-side stat parity** (`StatsAsserter`, NOT `SubjectAsserter` — `reference_differential_asserter_dispatch`): `outlier_detection.{ejections_active==1, ejections_enforced_total>=1, ejections_detected_consecutive_gateway_failure>=1, ejections_enforced_consecutive_gateway_failure>=1}` AND `ejections_detected_consecutive_5xx == 0` AND `ejections_enforced_consecutive_5xx == 0` (the gateway-ejects, 5xx-silent assertion). Verify `upstream_rq_total > 0` reference-side.
5. Recovery arm DEFERRED.

- [ ] **Step 1:** Write `driver_test.go` (`backendIdxFromBody` table test) → run → FAIL (helper undefined).
- [ ] **Step 2:** Write `driver.go` + `expectations.yaml` + `README.md`.
- [ ] **Step 3:** `go test ./test/fixtures/0070-outlier-detection-consecutive-gateway-failure/driver/ -count=1` (unit) → PASS.
- [ ] **Step 4: Run the cross-side fixture** (Docker + `contrib-v1.37.2`): `go test ./test/differential/ -run 'TestDifferential/0070' -count=1 -v` (`reference_differential_run_selector` — the `TestDifferential/` prefix REQUIRED). Expected PASS, both sides converge `ejections_active==1` via the gateway detector.
- [ ] **Step 5:** gofmt/vet/lint. Record fixtures **71 → 72**.
- [ ] **Step 6: Commit** (`phase 40.2 Task 7: 0070 cross-side outlier-detection-consecutive-gateway-failure fixture`).

---

## Task 8: The `0071` cross-side local-origin fixture (split=true; dead host)

**Files:**
- Create: `test/fixtures/0071-outlier-detection-local-origin/driver/driver.go`
- Create: `test/fixtures/0071-outlier-detection-local-origin/driver/driver_test.go`
- Create: `test/fixtures/0071-outlier-detection-local-origin/expectations.yaml`
- Create: `test/fixtures/0071-outlier-detection-local-origin/README.md`

Model on `test/fixtures/0066-health-check-http/driver/driver.go` (the `allocDeadPort` dead-host mechanism — a host:port with NO listener, agreed cross-side; reference via `host.docker.internal:<dead>`, subject via `127.0.0.1:<dead>`). Topology: cluster `c_lo`, lb ROUND_ROBIN, 3 endpoints {host0 HTTPEcho, host1 HTTPEcho, host2 = the dead `allocDeadPort` host}, `outlier_detection: { consecutive_local_origin_failure: 5, enforcing_consecutive_local_origin_failure: 100, split_external_local_origin_errors: true, interval: 10s, base_ejection_time: 30s, max_ejection_percent: 100 }`, on BOTH sides. `BackendCount() == 2` (only the 2 live HTTPEcho backends are runner-spawned; the dead host is the `allocDeadPort` injected endpoint — NOT a runner backend, the `0066` shape; NO `PerHostBackendKind`). `refContainerListenerPort = 19160`.

`AssertStats` flow:
1. **Ejection drive:** round-robin GET / until host2 (the dead host) accrues `consecutive_local_origin_failure` (5) consecutive connect failures and is ejected (each pick of host2 → a connect-refused → `LocalOriginErr: true` → the local-origin detector). ~5×3 ≈ 15 requests; single-source. (Requests to host2 return 503/502 to the client — tolerate them during the drive, like `0066`'s 503-tolerant warmup.)
2. **POLL `/stats` on BOTH sides** until `cluster.c_lo.outlier_detection.ejections_active == 1` (NO fixed sleep).
3. **Warmup + measured load phase** (delta): 100% `upstream_rq_2xx`, 0 `upstream_rq_5xx`, served by host0/host1.
4. **Cross-side stat parity** (`StatsAsserter`): `outlier_detection.{ejections_active==1, ejections_enforced_total>=1, ejections_detected_consecutive_local_origin_failure>=1, ejections_enforced_consecutive_local_origin_failure>=1}` AND `ejections_detected_consecutive_5xx == 0` (split=true routes the local-origin failures away from the 5xx/gateway detectors — AMEND-OD2-3). Verify `upstream_rq_total > 0` reference-side.
5. Recovery arm DEFERRED.

- [ ] **Step 1:** Write `driver_test.go` (`backendIdxFromBody` + an `allocDeadPort`-memoization test, the `0066` precedent) → run → FAIL.
- [ ] **Step 2:** Write `driver.go` + `expectations.yaml` + `README.md`.
- [ ] **Step 3:** `go test ./test/fixtures/0071-outlier-detection-local-origin/driver/ -count=1` (unit) → PASS.
- [ ] **Step 4: Run the cross-side fixture:** `go test ./test/differential/ -run 'TestDifferential/0071' -count=1 -v`. Expected PASS, both sides converge `ejections_active==1` via the local-origin detector.
- [ ] **Step 5:** gofmt/vet/lint. Record fixtures **72 → 73**.
- [ ] **Step 6: Commit** (`phase 40.2 Task 8: 0071 cross-side outlier-detection-local-origin fixture (split=true, dead host)`).

---

## Task 9: `0070`/`0071` deliberate breaks + 20-run flake

**Files:** none committed (verification only; the SPEC §8.1/§8.2 break protocol).

★ `-count=1` for EVERY break run (`reference_differential_break_protocol_count1`) + the `TestDifferential/<NNNN>` selector.

- [ ] **Step 1: `0070` Break (A) — gateway detector no-ops.** Temporarily make `recordExternal5xx`'s gateway arm a no-op (skip the `consecGw`/detect/eject). Run `go test ./test/differential/ -run 'TestDifferential/0070' -count=1` → MUST FAIL (`ejections_active` never reaches 1; the 5xx detector can't fire — `consecutive_5xx` is 100). Restore.
- [ ] **Step 2: `0070` Break (B) — LB ignores `available`.** Temporarily revert the round-robin leaf consult site (`loadbalancer.go:61`, the 40.1 Task 3 site) `available → isHealthy`. Run → MUST FAIL (the ejected 503 host stays in rotation → measured phase leaks 5xx). Restore.
- [ ] **Step 3: `0071` Break (A) — local-origin detector no-ops / `LocalOriginErr` not populated.** Temporarily make `recordLocalOrigin`'s split=true arm a no-op (OR drop the `LocalOriginErr: true` at the H1/H2 failure sites). Run `go test ./test/differential/ -run 'TestDifferential/0071' -count=1` → MUST FAIL (the dead host never ejects). Restore.
- [ ] **Step 4: `0071` Break (B) — picked endpoint not surfaced.** Temporarily revert `AcquireH1`/`Dial` to return `Endpoint{}` on connect failure (so `record`'s unknown-addr guard `if !ok { return }` short-circuits — the zero addr is unknown to `health.states`). Run → MUST FAIL (`consecLO` never increments → the dead host never ejects → never converges). Restore.
- [ ] **Step 5: Confirm all breaks restored** (`git diff` clean; `go test ./test/differential/ -run 'TestDifferential/0070' -count=1` + `-run 'TestDifferential/0071' -count=1` → PASS).
- [ ] **Step 6: 20-run flake gate** for each: `for i in $(seq 20); do go test ./test/differential/ -run 'TestDifferential/0070' -count=1 || echo "FAIL $i"; done` (and `0071`) → 20/20 each (widen `convergeDeadline`/`warmupStable` if any flake, NOT a fixed sleep — `reference_health_check_propagation_warmup`).
- [ ] **Step 7:** Record break + flake results in PROGRESS. (No commit — verification only.)

---

## Task 10: Widen the `0069` `StatsAsserter` + full 73-dir differential + six-gate

**Files:**
- Modify: `test/fixtures/0069-outlier-detection-consecutive-5xx/driver/driver.go` — widen the `StatsAsserter` to cross-assert `cluster.c_od.outlier_detection.ejections_detected_consecutive_gateway_failure >= 1` (AMEND-OD2-1); remove the 40.1 exclusion comment. (Optionally update `0069/README.md` + `expectations.yaml`.)

- [ ] **Step 1:** Widen the `0069` `StatsAsserter` (now that envoy-go emits the gateway-detected counter on `0069`'s 503s, matching the reference). Run `go test ./test/differential/ -run 'TestDifferential/0069' -count=1` → PASS (the gateway-detected counter now matches cross-side).
- [ ] **Step 2: Six-gate** (ADR-0052): `go build ./...`; `go vet ./...`; `gofmt -l internal/ test/` (empty); `golangci-lint run ./...`; `go test ./internal/... -count=1`; `go test ./test/differential/ -count=1` (ALL **73** GREEN).
- [ ] **Step 3: Stat-surface count → 1141**; fixtures → 73; fuzzers → 42 (unchanged); BackendKind tail → 35 (unchanged). Record in PROGRESS.
- [ ] **Step 4: Commit** (`phase 40.2 Task 10: widen 0069 StatsAsserter (gateway-detected) + full 73-dir six-gate green`).

---

## Task 11: ADR-0246 body + BEHAVIOR_CONTRACT delta

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` — ADR-0246 full entry (§Decision + §Consequences; promote/refine the §Context already drafted in SPEC §13). DECISIONS tail ADR-0245 → **ADR-0246** (next-free ADR-0247).
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` — extend the `### Cluster — passive health (outlier detection)` subsection (SPEC §9): the gateway detector (codes `{502,503,504}`; active-by-default, detect-only at enforcing 0; gateway-checked-first + the once-ejected reset + the fall-through); the local-origin detector (split=true only); the `split_external_local_origin_errors` switch; the `LocalOriginErr` seam population at the 5 connection-failure sites + the picked-endpoint-on-connect-failure attribution; the +4 stats + the extended double-count; the all-connection-failures-are-one-local-origin-class simplification + the `0069` gateway-detected widening. Advance the stat-surface block **1137 → 1141**.

- [ ] **Step 1:** Write the ADR-0246 body (§Decision: the two detectors + the split mapping + the gateway-first ordering + the `LocalOriginErr` seam + the `Dial`/`AcquireH1` endpoint-surfacing framework touch + the +4 stats; §Consequences: byte-stable when no `outlier_detection`; 40.1 substrate reused unchanged; the statistical detectors deferred to 40.3; the recorded simplification).
- [ ] **Step 2:** Write the BEHAVIOR_CONTRACT extension + the stat-count bump.
- [ ] **Step 3:** `go build ./...` (docs-only sanity).
- [ ] **Step 4: Commit** (`phase 40.2 Task 11: ADR-0246 body + BEHAVIOR_CONTRACT passive-health extension (stat 1137→1141)`).

---

## Task 12: Completion bundle

**Files:**
- Modify: `docs/envoy-go/STATE.md` — prepend the `phase 40.2 IMPL done` active-phase block (demote the SPEC block to prior); update lifecycle-state / next-skill / last-commit / last-updated / phase-directory; the count footer → stat **1141** / fixtures **73** / fuzzers **42** / BackendKind tail **35** / DECISIONS tail **ADR-0246** (next-free ADR-0247).
- Modify: `docs/envoy-go/ROADMAP.md` — row 40: append the 40.2-IMPL-DONE note. Row 40 STAYS `in-progress` (the 40.3 statistical-detector leg still pending — `reference_roadmap_split_phase_row_done`; the row flips `done` only when ALL three legs land).
- Modify: `docs/envoy-go/phases/40.2-outlier-detection-other-consecutive/{PROGRESS.md,README.md}` — final state + exit counts.
- Modify: `next-prompt.txt` — roll the cold-start forward to the phase-40.3 SPEC (the FINAL leg: `success_rate_*` + `failure_percentage_*` statistical detectors + the per-interval cross-host mean/stddev aggregation runtime — the first new outlier goroutine) OR a new subject. SHA-fill after the squash.

- [ ] **Step 1:** Update STATE.md + ROADMAP.md + PROGRESS.md + README.md with the final counts.
- [ ] **Step 2: Final six-gate re-run** (ADR-0052) to confirm the doc edits did not break the build/tests.
- [ ] **Step 3: Commit** the bundle (`phase 40.2 Task 12: completion bundle — STATE/ROADMAP/PROGRESS + next-prompt roll-forward`).
- [ ] **Step 4:** Controller (NOT a subagent — `feedback_subagents_no_push`) squashes the branch, ff-merges to master, pushes, rolls next-prompt with the squash SHA, removes the worktree, deletes the branch (superpowers:finishing-a-development-branch).

---

## Exit deltas (SPEC §14)

| Count | Before (SPEC) | After (IMPL) |
|---|---|---|
| Stat surface | 1137 | **1141** (+4) |
| Fixtures | 71 | **73** (`0070` + `0071`) |
| Fuzzers | 42 | 42 (unchanged) |
| BackendKind tail | 35 | 35 (unchanged — `0070` reuses `HTTP503Responder`; `0071` reuses `allocDeadPort`) |
| DECISIONS tail | ADR-0245 | **ADR-0246** (next-free ADR-0247) |
| New Go packages | — | 0 |
| New go.mod modules | — | 0 |

ROADMAP row 40 STAYS `in-progress` (40.3 pending). The FINAL ADR-0045 split-gate (D-S40.2-7): NO FURTHER SPLIT.
