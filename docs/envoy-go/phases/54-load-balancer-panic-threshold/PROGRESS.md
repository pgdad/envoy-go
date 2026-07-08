# Phase 54 Implementation Progress Ledger

## Task Status

| Task | Description | Status |
|---|---|---|
| 1 | Baselines/anchors gate + PROGRESS.md | pending |
| 2 | AMEND-PT1 floor + integer cross-multiply (+ D-PT-STORE type change + call-site sweep) | pending |
| 3 | Parse-path unit hardening (presence semantics through the wired path) | pending |
| 4 | AMEND-PT4 out-of-range reject | pending |
| 5 | AMEND-PT3 locality retrofit (reordered before Task 6) | pending |
| 6 | AMEND-PT2 double-increment fix | pending |
| 7 | `0097` fixture + driver | pending |
| 8 | `0097` deliberate breaks + flake-soak + `-race` | pending |
| 9 | Full 99-dir differential + six gates | pending |
| 10 | ADR-0271 body + BEHAVIOR_CONTRACT delta | pending |
| 11 | Completion bundle (STATE/ROADMAP, row-54 done, family CLOSE, counts) | pending |

---

## Count Anchors (Step 1 — CONFIRMED)

**Expected vs Observed:**
- Fixtures: **98** ✓ (confirmed via `ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l`)
- Tail fixture: **test/fixtures/0096-lb-priority** ✓
- BackendKind tail: **H2GoawayResponder BackendKind = 38** ✓
- Fuzzers: **52** ✓ (confirmed via `grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l`)
- DECISIONS tail: **ADR-0270** ✓ (confirmed via `grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1`)
- Build: **clean** ✓ (confirmed via `go build ./...`)
- `go mod tidy -diff`: **empty** ✓ (ZERO new deps)
- BEHAVIOR_CONTRACT stat surface: **1200** (10 occurrences of "1200" in the file)

---

## As-Built Line Anchors (Step 2 — RE-PINNED)

### `internal/cluster/health.go`
- `type clusterHealth struct` — line **70**
- `func newClusterHealth(endpoints []Endpoint, panicThreshold float64)` — line **82**
- `func (ch *clusterHealth) inPanic(eps []Endpoint) bool` — line **182**
- `func (ch *clusterHealth) panicInc()` — line **198**
- `func (ch *clusterHealth) panicGate(eps []Endpoint) bool` — line **211**
- `func parsePanicThreshold(c *clusterv3.Cluster) float64` — line **485**

### `internal/cluster/priority.go`
- `func tierHealth(shared *clusterHealth) *clusterHealth` — line **46**
- `type priorityLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)` — line **102**
- `func newPriorityLBWithRNG(endpoints []Endpoint, health *clusterHealth, opf uint32, hasOPF bool, factory priorityLeafFactory, rng func() uint64)` — line **154**

### `internal/cluster/subset.go`
- `type leafFactory func(sub []Endpoint) (loadBalancer, error)` — line **131**
- `func newSubsetLB(endpoints []Endpoint, cfg lbSubsetCfg, factory leafFactory)` — line **158**

### `internal/cluster/locality.go`
- `func newLocalityWeightedLB(endpoints []Endpoint, health *clusterHealth, opf uint32, hasOPF bool, factory leafFactory)` — line **67**
- `func newLocalityWeightedLBWithRNG(endpoints []Endpoint, health *clusterHealth, opf uint32, hasOPF bool, factory leafFactory, rng func() uint64)` — line **85**
- `func (lw *localityWeightedLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool)` — line **142**
- `lw.health.panicInc()` — line **144**

### `internal/cluster/loadbalancer.go`
- `type loadBalancer interface` — line **16**
- `type roundRobin struct` — line **35**
- `rr.health.panicGate(rr.endpoints)` — line **49**

### `internal/cluster/manager.go`
- `func buildCluster(c *clusterv3.Cluster, idx int, baseDir string)` — line **402**
- `la := c.GetLoadAssignment()` — line **429**
- `endpoints, err := extractEndpoints(la, name)` — line **433**
- `health = newClusterHealth(endpoints, parsePanicThreshold(c))` (1st site) — line **461**
- `return buildLeafLB(c, name, sub, health)` — line **479**
- `health = newClusterHealth(endpoints, parsePanicThreshold(c))` (2nd site) — line **501**
- `return buildLeafLB(c, name, sub, health)` — line **514**
- `health = newClusterHealth(endpoints, parsePanicThreshold(c))` (3rd site) — line **536**
- `return buildLeafLB(c, name, sub, h)` — line **548**

### `test/differential/runner_test.go`
- `_ "github.com/pgdad/envoy-go/test/fixtures/0096-lb-priority/driver"` — line **123**

---

## Sweep Sets (Step 3 — ENUMERATED)

### `newClusterHealth(..., 0.5)` Call Sites (Task 2 rewrites to `50`)

**Test sites passing `0.5` (25 total):**
1. `internal/cluster/ringhash_test.go:34`
2. `internal/cluster/priority_test.go:126`
3. `internal/cluster/priority_test.go:158`
4. `internal/cluster/priority_test.go:199`
5. `internal/cluster/priority_test.go:384`
6. `internal/cluster/priority_test.go:428`
7. `internal/cluster/priority_test.go:471`
8. `internal/cluster/priority_test.go:504`
9. `internal/cluster/loadbalancer_test.go:128`
10. `internal/cluster/maglev_test.go:136`
11. `internal/cluster/locality_test.go:160`
12. `internal/cluster/locality_test.go:400`
13. `internal/cluster/random_test.go:88`
14. `internal/cluster/random_test.go:109`
15. `internal/cluster/leastrequest_test.go:142`
16. `internal/cluster/health_test.go:53`
17. `internal/cluster/health_test.go:72`
18. `internal/cluster/health_test.go:154`
19. `internal/cluster/health_test.go:493`
20. `internal/cluster/health_test.go:502`
21. `internal/cluster/health_test.go:521`
22. `internal/cluster/health_test.go:567`
23. `internal/cluster/health_test.go:600`
24. `internal/cluster/health_test.go:618`
25. `internal/cluster/outlier_test.go:496`

**Production sites passing `parsePanicThreshold(c)` (3 total — TEXTUALLY UNCHANGED):**
1. `internal/cluster/manager.go:461`
2. `internal/cluster/manager.go:501`
3. `internal/cluster/manager.go:536`

### `.panicThreshold` Field References (Task 2 renames to `.panicThresholdPercent`)

**Test assertions checking `.panicThreshold`:**
1. `internal/cluster/priority_test.go:128` — `if view.panicThreshold != 0`
2. `internal/cluster/priority_test.go:129` — `t.Errorf("view.panicThreshold = %v, want 0…")`
3. `internal/cluster/priority_test.go:238` — `if tierHealthViews[0].panicThreshold != 0`
4. `internal/cluster/priority_test.go:239` — `t.Errorf("tier children's health view panicThreshold = %v, want 0")`

**Struct definition and comment references:**
5. `internal/cluster/health.go:72` — struct field comment
6. `internal/cluster/priority.go:33` — comment: "panicThreshold: 0"
7. `internal/cluster/priority.go:47` — literal: `panicThreshold: 0`
8. `internal/cluster/locality_test.go:379` — comment: "SHARED panicThreshold (0.5 default)"

### `priorityLeafFactory` / `leafFactory` References (Task 5 renames `priorityLeafFactory` to `healthLeafFactory`)

**`priorityLeafFactory` references (RENAMED):**
1. `internal/cluster/priority.go:87` — comment
2. `internal/cluster/priority.go:88` — comment
3. `internal/cluster/priority.go:99` — comment
4. `internal/cluster/priority.go:100` — comment
5. `internal/cluster/priority.go:102` — type definition
6. `internal/cluster/priority.go:131` — function parameter
7. `internal/cluster/priority.go:154` — function parameter
8. `internal/cluster/priority_test.go:167` — comment
9. `internal/cluster/priority_test.go:178` — function definition

**`leafFactory` references (UNCHANGED in subset.go, REUSED signature in locality.go):**
1. `internal/cluster/subset_test.go:90` — comment
2. `internal/cluster/subset.go:129` — comment
3. `internal/cluster/subset.go:131` — type definition (UNCHANGED)
4. `internal/cluster/subset.go:158` — function parameter (UNCHANGED)
5. `internal/cluster/locality_test.go:5` — comment
6. `internal/cluster/locality_test.go:13` — function definition
7. `internal/cluster/locality.go:67` — function parameter
8. `internal/cluster/locality.go:85` — function parameter

---

## D-PT Question Resolutions (from PLAN §12)

### D-PT-STORE (Stored Representation of Floored Threshold)

**Resolution:** An INTEGER field.
- `clusterHealth.panicThreshold float64` (a fraction) → `clusterHealth.panicThresholdPercent int` (a floored integer percent in `[0,100]`)
- `newClusterHealth`'s second parameter and `parsePanicThreshold`'s return type retype `float64 → int`
- Chosen `int` (not `uint32`) for zero-conversion arithmetic in `inPanic`'s cross-multiply: `100*ch.availableCount(eps) < ch.panicThresholdPercent*total`
- Overflow-safe: `100 × availableCount ≤ ~1e6` and `panicThresholdPercent × total ≤ 100 × total ≤ ~1e6` for any realistic host count

### D-PT-REJECT-PLACEMENT (Out-of-Range Reject Placement)

**Resolution:** A STANDALONE helper `validatePanicThresholdRange(c *clusterv3.Cluster, name string) error` in `health.go`, called UNCONDITIONALLY from `buildCluster` immediately after `extractEndpoints` succeeds (before the health parse + the LB switch).
- **Rationale:** All three `parsePanicThreshold(c)` sites are health-guarded, so folding the range check there would miss a plain cluster (no health_checks, no wrapper) that still carries — and the reference still rejects — an out-of-range value.
- **Condition:** `healthy_panic_threshold` present AND (`value < 0` OR `value > 100`); exactly `0` and `100` accepted
- **Byte-stable message:** `cluster: %q: common_lb_config.healthy_panic_threshold: value must be inside range [0, 100]`

### D-PT-TIERHEALTH-HOME (`tierHealth`'s Home + Factory Type Reuse)

**Resolution (Part 1):** `tierHealth` RELOCATES to `health.go` (from `priority.go`)
- Natural home: `health.go` owns `clusterHealth`/`newClusterHealth`
- SECOND consumer in `locality.go` justifies relocation
- Pure relocation — no logic change (the `panicThreshold: 0` literal already becomes `panicThresholdPercent: 0` at Task 2)

**Resolution (Part 2):** `priorityLeafFactory` RENAMES to `healthLeafFactory` and RELOCATES to `loadbalancer.go`
- Renamed for DRY correctness: once `localityWeightedLB` ALSO consumes it, a `priority`-specific name on a `localityWeightedLB` parameter would be misleading
- Relocated to `loadbalancer.go` (the neutral LB-construction home, next to the `loadBalancer` interface)
- **Signature:** `healthLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)`
- Both `localityWeightedLB` and `priorityLB` use `healthLeafFactory`
- `subset.go`'s `leafFactory func(sub []Endpoint) (loadBalancer, error)` stays COMPLETELY UNTOUCHED (subset children keep shared health via a closing factory — the crux asymmetry)
- **Load-bearing change:** Health-parameterized signature change to `newLocalityWeightedLB`/`newLocalityWeightedLBWithRNG` + the `manager.go:513-515` closure

### D-PT-UNIT-VS-DIFF (Unit vs Differential Coverage of AMEND-PT2/PT3)

**Resolution:** The double-increment fix (AMEND-PT2) and the locality retrofit (AMEND-PT3) stay UNIT-proven against pinned reference values (delta == N; degraded-locality unhealthy hosts == 0).
- **No locality/subset differential dir added:** `0097` uses plain, non-wrapped clusters where the panic path is single-increment already (honoring SPEC §8.2 / Q2 plain-two-cluster-discriminator scope)
- **Unit proof approach:** Each new assertion is scratch-revert-proven live (Tasks 5/6), confirming the assertion BITES before restoring the code

---

## FINAL ADR-0045 Split-Gate Re-Check Verdict

**Verdict: NO SPLIT.** 11 tasks; ~150–260 prod LoC
- `health.go` comparison-shape + stored-type + validator + relocated tierHealth ≈ 45
- `loadbalancer.go` factory type ≈ 3
- `locality.go` retrofit + removed increment ≈ 18
- `manager.go` reject call + closure ≈ 5
- `0097` driver ≈ 150–200

**Gate status:** Comfortably under the ADR-0045 `>~25 tasks OR >~1500 LoC` gate.

**Justification:**
- The family's SMALLEST phase (no new policy, wrapper, package, seam, `Endpoint` dimension, stat, fuzzer, or BackendKind)
- A single flat row; a SINGLE ADR (ADR-0271, SPEC §11.7's zero-seam-change finding)
- No second producer plane or subsystem to couple against (SPEC §3.0/§4)
- The re-check re-confirms the SPEC §3.0 no-split disposition

---

## Session Information

**IMPL Session Worktree:** `/home/esa/git/envoy-go/.worktrees/phase-54-impl`  
**Branch:** `phase-54-impl`  
**Task 1 Verification Date:** 2026-07-08  
**All anchors CONFIRMED MATCH expected values ✓**

## Task 5 controller scratch-revert liveness proof (Step 10)
- Reverted per-locality child build `factory(members, tierView)` → `factory(members, health)` (+ `_ = tierView`).
- `go test -run TestPick_DegradedLocality_NoLocalPanic -count=1` FAILED: unhealthy hosts a2:1/a3:1/a4:1 each got 20 picks (want 0) — degraded locality flattened. Assertion is LIVE.
- Restored via `git restore internal/cluster/locality.go`; test green again.

## Task 6 controller scratch-revert liveness proof (Step 5)
- Re-inserted `lw.health.panicInc()` in the panic branch of `localityWeightedLB.Pick`.
- `go test -run TestPick_LocalityPanic_IncrementsOncePerPick -count=1` FAILED: `lb_healthy_panic = 100 over 50 picks, want 50` — double-increment reproduced. Assertion is LIVE.
- Restored via `git restore internal/cluster/locality.go`; test green again.

## Task 8 controller deliberate-break verification (0097)
- Break (a) hardcode-50: FAIL (c_pt_a hosts 3/4 got 0; lb_healthy_panic delta 0≠200). LIVE.
- Break (b) skip-degrade: FAIL (c_pt_a membership_healthy did not converge to 3, last seen 5). LIVE.
- Break (c) floor→round (60.9→61): FAIL (c_pt_c panics; /c warmup did not stabilize). LIVE — the integer-truncation differential proof.
- Soak 20/20 flake-free; -race clean. Tree restored clean (0 BREAK markers).

## Task 9 controller full-suite verification
- Six gates: go build / gofmt / go vet / go mod tidy -diff / golangci-lint — ALL clean.
- Non-differential packages: all green.
- Full 99-dir differential: green (re-run; first run flaked on 0081-grpc-access-log waitTCPDial-5s startup race, isolated-passed).
- internal/cluster full-package -race: clean.
- Full differential -race: green (re-run; first run flaked on 0084-otlp-access-log same startup race, isolated-passed). Both flakes = reference_differential_fullsuite_startup_flake, unrelated to phase-54.
