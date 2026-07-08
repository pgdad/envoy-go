# Phase 53 Implementation Progress

## 11-Task Status Table

| Task | Title | Status |
|------|-------|--------|
| 1 | Baselines/anchors gate + PROGRESS.md | ✓ done |
| 2 | `Endpoint.Priority` dimension + `extractEndpoints` capture | ✓ done |
| 3 | `priority.go`: `tierHealth`/`tierCapacity`/`cascadeLoads` pure functions | ✓ done |
| 4 | `priority.go`: `priorityGroup`/`priorityLB` + construction | ✓ done |
| 5 | `priority.go`: `Pick` implementation | ✓ done |
| 6 | `manager.go` wrap-after-switch + rejects + health-widening | ✓ done |
| 7 | Unit tests: §11.1/§11.4 data tables + AMEND-P1 coverage | ✓ done |
| 8 | `0096` driver: 2-tier×5-host topology + toggle harness | ✓ done |
| 9 | `0096` assertions: arm (a)/(b) hard 100%/0% + stats | ✓ done |
| 10 | `0096` deliberate-break liveness + flake check + `-race` | ✓ done |
| 11 | BEHAVIOR_CONTRACT + ADR-0270 + six-gate | ✓ done |

All 11 tasks of this phase's PLAN are now complete.

---

## Step 1: Count Anchors (Confirmed)

```
fixtures:                  97
last fixture:              test/fixtures/0095-lb-locality-weighted
BackendKind tail:          H2GoawayResponder BackendKind = 38 (line 606)
fuzzer count:              52
DECISIONS tail:            ## ADR-0269
stat-surface DOC count:    1200 (lines 1284, 4730, 4732)
go build:                  BUILD_OK
go mod tidy -diff:         TIDY_EMPTY (exit 0, no changes)
```

**Verdict:** All count anchors confirmed. No drift from PLAN expectations.

---

## Step 2: As-Built Line Anchors (Confirmed)

| Item | Line | File |
|------|------|------|
| `type Endpoint struct` | 42 | internal/cluster/cluster.go |
| `func (e Endpoint) Addr` | 63 | internal/cluster/cluster.go |
| `la := c.GetLoadAssignment()` | 417 | internal/cluster/manager.go |
| `var health *clusterHealth` | 447 | internal/cluster/manager.go |
| `sc := c.GetLbSubsetConfig` | 464 | internal/cluster/manager.go |
| `lwc := c.GetCommonLbConfig().GetLocalityWeightedLbConfig()` | 474 | internal/cluster/manager.go |
| `func buildLeafLB` | 341 | internal/cluster/manager.go |
| `func extractEndpoints` | 814 | internal/cluster/manager.go |
| `Host: sa.GetAddress()` | 834 | internal/cluster/manager.go |
| `cl.lb = lb` | 509 | internal/cluster/manager.go |
| `func registerClusterMetrics` | 112 | internal/cluster/manager.go |
| `type leafFactory` | 131 | internal/cluster/subset.go |
| `func (s *subsetLB) Pick` | 213 | internal/cluster/subset.go |
| `type loadBalancer interface` | 16 | internal/cluster/loadbalancer.go |
| `func (rr *roundRobin) Pick` | 44 | internal/cluster/loadbalancer.go |
| `func newPCGRNG` | 66 | internal/cluster/leastrequest.go |
| `type localityGroup` | 26 | internal/cluster/locality.go |
| `type localityWeightedLB` | 52 | internal/cluster/locality.go |
| `func newLocalityWeightedLB` | 67 | internal/cluster/locality.go |
| `func newLocalityWeightedLBWithRNG` | 85 | internal/cluster/locality.go |
| `func (lw *localityWeightedLB) Pick` | 142 | internal/cluster/locality.go |
| `func newHostHealth` | 44 | internal/cluster/health.go |
| `type clusterHealth struct` | 71 | internal/cluster/health.go |
| `func newClusterHealth` | 83 | internal/cluster/health.go |
| `func (ch *clusterHealth) availableCount` | 142 | internal/cluster/health.go |
| `func (ch *clusterHealth) inPanic` | 165 | internal/cluster/health.go |
| `func (ch *clusterHealth) panicInc` | 181 | internal/cluster/health.go |
| `func parsePanicThreshold` | 431 | internal/cluster/health.go |
| `internal/cluster/priority.go` | ABSENT | (expected — Task 3 creates it) |

**Verdict:** All as-built line anchors confirmed. Every line number matches PLAN citations exactly. No drift detected.

---

## Step 3: Endpoint{} Construction-Site Sweep

### Production sites in `internal/cluster/*.go` (non-test)

**Total: 18 hits**

- **1 keyed literal (REQUIRES EDITING in Task 2):**
  - `internal/cluster/manager.go:834` — `Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars, Locality: loc, LocalityWeight: weight}` — PRODUCTION construction site, must gain `Priority: priority` field

- **17 bare-zero literals (COMPILE UNCHANGED with Priority=0 default):**
  - `internal/cluster/cluster.go:488` — error return
  - `internal/cluster/cluster.go:517` — error return
  - `internal/cluster/cluster.go:526` — error return
  - `internal/cluster/cluster.go:607` — error return
  - `internal/cluster/cluster.go:617` — error return
  - `internal/cluster/maglev.go:45, 64` — error returns
  - `internal/cluster/leastrequest.go:87, 112` — error returns
  - `internal/cluster/loadbalancer.go:47, 65` — error returns
  - `internal/cluster/subset.go:224` — error return
  - `internal/cluster/h2pool.go:315` — error return
  - `internal/cluster/random.go:43, 59` — error returns
  - `internal/cluster/ringhash.go:135, 157` — error returns

### Test sites in `internal/cluster/*_test.go`

**Total: 80 hits** (mixed zero-value and keyed literals) — ALL compile unchanged.

### Production sites in `internal/filter/` (across all subdirs)

**Total: 44 hits** — ALL are bare-zero literals `cluster.Endpoint{}` — all compile unchanged.

### Verdict

**ZERO-EDIT SITES FINDING CONFIRMED:**

Adding `Priority uint32` to the `Endpoint` struct requires editing **EXACTLY ONE production call site** (`extractEndpoints` in `internal/cluster/manager.go:834`) — every other site (error returns in leaf-policy modules, every test builder, every `internal/filter/` call) is either:
- A bare `Endpoint{}` zero-value literal (the new `Priority` field defaults to 0)
- A keyed literal that never touches the new field (keyed fields explicitly named)

This is a verified finding based on a complete sweep of 101 total `Endpoint{` hits across `internal/cluster/` alone. The third time this exact sweep has been performed (phase 38's `Metadata`, phase 52's `Locality`/`LocalityWeight`, now `Priority`). Per `reference_conn_wrap_method_no_promote`, every new struct field must be enumerated; this phase's discipline confirms zero scaffolding burden beyond the single `extractEndpoints` site.

---

## D-Question Resolutions (SPEC §12)

### D-P-DUP (the duplicate-priority-value corner)

RESOLVED as SPEC's own self-answer, MORE SIMPLY than phase 52's D-LW-DUP (which had to resolve a conflicting WEIGHT via last-write-wins): priority carries no per-group scalar to conflict over, so two `LocalityLbEndpoints` groups declaring the SAME `priority` value simply MERGE their endpoints into the SAME tier — a natural consequence of `newPriorityLBWithRNG`'s `map[uint32][]Endpoint` grouping (Task 4), which appends to the SAME slice keyed by `ep.Priority` regardless of which source group an endpoint came from. This PLAN adds the concrete unit test SPEC leaves implicit: `TestNewPriorityLB_DuplicatePriority_EndpointsMerge` (Task 4) constructs two `Endpoint`s sharing an identical `Priority` value (simulating two source `LocalityLbEndpoints` groups that collapsed to the same tier in `extractEndpoints`'s output) and asserts (a) exactly ONE `priorityGroup` results (not two) and (b) its `endpoints` slice contains BOTH endpoints merged — an unusual, degenerate config shape, low-stakes and untested by either live probe, exactly as SPEC frames it.

### D-P-FACTORY (the `priorityLeafFactory` local-type choice)

RESOLVED per the SPEC's own leaning, CONFIRMED against the actual `buildLeafLB` signature (not assumed): `priorityLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)` is a phase-53-LOCAL type (Task 4), distinct from `subset.go`'s existing `leafFactory func(sub []Endpoint) (loadBalancer, error)` — `subset.go`/`locality.go`'s call sites and the shared `leafFactory` type stay COMPLETELY UNTOUCHED. Verified: `buildLeafLB(c *clusterv3.Cluster, name string, endpoints []Endpoint, health *clusterHealth) (loadBalancer, error)` (`manager.go:341`) already accepts a `health *clusterHealth` as its fourth positional parameter, so the manager.go call-site closure `func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) { return buildLeafLB(c, name, sub, h) }` (Task 6) compiles and forwards CORRECTLY without any adjustment to `buildLeafLB` itself — the SPEC §3.1 sketch's assumption holds exactly as written. A wholesale `leafFactory` refactor (accepting an explicit health parameter for EVERY wrapper, retroactively touching `subset.go`/`locality.go`) is explicitly NOT bundled into this phase — it would touch code outside phase 53's blast radius for no phase-53-required benefit.

### D-P-RETROFIT (whether `locality.go`'s own documented child-local-panic coverage boundary should be retroactively fixed with `tierHealth`'s pattern)

RESOLVED as SPEC's own self-answer: OUT of THIS phase's scope. `locality.go` is phase 52's, already shipped and documented (`BEHAVIOR_CONTRACT.md:1288`, "the child-local-panic coverage boundary (documented, tested, NOT fixed)"); retrofitting it is a candidate LOW-RISK future maintenance item, NOT bundled here to keep phase 53's blast radius to `priority.go` + the manager wrap site only. No task in this PLAN touches `locality.go`.

---

## ADR-0045 Split-Gate Re-Check (FINAL)

**VERDICT: NO SPLIT NEEDED**

This phase decomposes into **11 tasks** (well under the `>~25 tasks` gate) over an estimated **~200-250 production LoC**:
- `priority.go` ~170-190 LoC (`tierHealth`/`tierCapacity`/`cascadeLoads` + `priorityGroup`/`priorityLB`/`priorityLeafFactory` + both constructors + `Pick`)
- `cluster.go`'s `Endpoint.Priority` field addition ~10 LoC
- `manager.go`'s `extractEndpoints` capture (~2 LoC delta)
- `distinctPriorities` helper (~8 LoC)
- wrap-after-switch (~30-35 LoC)

Total well under the `>~1500 LoC` gate.

The wrapper reuses the EXISTING `loadBalancer` interface, the EXISTING `buildLeafLB` factory (confirmed compiling verbatim against `priorityLeafFactory`'s 4-parameter shape, D-P-FACTORY), and the EXISTING `clusterHealth` model (extended only by the pure `tierHealth` view-constructor, itself ~5 LoC) UNCHANGED — there is no second seam to build.

This is the phase-35/37/52 single-flat-row precedent, repeated a FOURTH time: one new file, one new wrap site, zero new packages, zero new producer plane. Slightly CHEAPER than phase 52's ~230-330 LoC / 10 tasks estimate, since `priority.go`'s three-way pure-function split (Task 3) makes the confirmed formulas directly unit-testable without any `Pick`-level stochastic scaffolding — the SPEC's own §1.1 AMEND-P6 anticipated ~230-330/11-12; this PLAN's concrete decomposition lands at the LOW end of that range.

---

## Task 10: `0096` deliberate-break liveness + the ≥20-run flake check (+ a genuine flake fix)

Ran the `0096-lb-priority` cross-side differential live against Docker + the
real `envoyproxy/envoy:contrib-v1.37.2` reference image. Every break below was
run with `-count=1` (never the test cache) via `-run 'TestDifferential/0096'`
(`reference_differential_break_protocol_count1` /
`reference_differential_run_selector`).

### Break (i) — `internal/cluster/manager.go`, `extractEndpoints`

Dropped `Priority: priority` from the `Endpoint{...}` literal (added `_ =
priority` to keep it compiling) — every endpoint stays at tier 0,
`distinctPriorities` reports a single tier, the `priorityLB` wrap never
fires, the cluster falls back to plain ROUND_ROBIN over all 10 hosts.

FAILED as expected (the warmup gate itself times out first, since a 50/50
spread never stabilizes to an all-tier-0 steady state):

```
runner_test.go:1294: arm(a) warmup: subject: data path did not stabilize to 60 consecutive non-degraded 200s within 15s
--- FAIL: TestDifferential (16.94s)
```

`git restore internal/cluster/manager.go` → re-run: PASS.

### Break (ii) — `test/fixtures/0096-lb-priority/driver/driver.go`, `AssertStats`

Commented out `r.SetHealthy(false)` in arm (b)'s tier-0 fail-over loop (added
`_ = r` to keep it compiling) — tier 0 never actually fails its health check.

FAILED as expected (the earliest gate arm (b) hits — `membership_healthy`
never converges down to 5 — but directly caused by the break):

```
runner_test.go:1294: arm(b) converge: reference: cluster.c_pri.membership_healthy did not converge to 5 within 30s (last seen 10)
--- FAIL: TestDifferential (32.30s)
```

Manually reverted (the exact inverse edit, NOT `git restore`, since this file
also carries the flake fix below) → re-run: PASS.

### The ≥20-run flake check found a genuine, pre-existing residual flake — fixed

The FIRST `-count=20` batch (against the Task 9 baseline, before any Task 10
changes) was NOT clean: an intermittent failure at roughly a 1-in-20 to
1-in-60 rate, ALWAYS on the reference side, ALWAYS in arm (a)
(`tally.t1 != 0`, small variable counts: 3, 4, 9, 17, 22, 27, 29/300 observed
across different runs).

Live diagnostic instrumentation (temporary; not committed) conclusively
RULED OUT health-check flapping: `cluster.c_pri.health_check.failure` stayed
at 0 and `membership_healthy` stayed at 10 in every run, including failing
ones. The leaked requests were tightly clustered at the very START of the
300-request load phase (indices 0, 2, 3 in one captured run), immediately
after `warmupUntilStable`'s own K=10-consecutive-success streak had just
completed — i.e., K=10 was occasionally satisfied by luck before the
underlying (still-settling) selection state had genuinely stabilized.

Two other fixes were tried live and DISCARDED (did not reduce the failure
rate over further ≥20-run batches): widening the health-check timeout alone
(made it worse — probes pile up when timeout > interval); widening
interval+timeout proportionally AND separately pinning a long
`dns_refresh_rate` to rule out STRICT_DNS periodic re-resolution (neither
helped).

**Fix:** raised `warmupStable` from 10 to 60 in
`test/fixtures/0096-lb-priority/driver/driver.go` (widening the SAME
mechanism Task 9 built, not a new one). Confirmed clean over 4 further
`-count=20` batches (80 invocations, 80/80 PASS) after the fix landed, plus
both deliberate breaks re-verified against this same fixed state.

Full narrative + all commands/output: `test/fixtures/0096-lb-priority/README.md`
("Task 10" section).

### `-race`

```
go test ./test/differential/ -run 'TestDifferential/0096' -race -count=1 -v
```

PASS, no race.

### Constant-sync guard

`staticLoadCount`, `degradedLoadCount`, `tier0Hosts`, `tier1Hosts` are all
named constants in `driver.go`, referenced everywhere they're used — no
re-derived literals (`reference_fixture_workload_constant_desync`).

### Files changed

- `test/fixtures/0096-lb-priority/driver/driver.go` — `warmupStable: 10 → 60`
  (+ doc comment recording the investigation)
- `test/fixtures/0096-lb-priority/README.md` — Task 10 section
- `docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md` — this section

## Task 11: Completion bundle — BEHAVIOR_CONTRACT + ADR-0270 + final six-gate

The FINAL task of this phase's 11-task PLAN. Lands the atomic completion
bundle: the new `### Load balancer — priority (LocalityLbEndpoints.priority)`
BEHAVIOR_CONTRACT.md section (immediately after the existing locality-weighted
section, mirroring its shape); the full ADR-0270 entry (§Context promoted from
SPEC §13 + §Decision quoting the LANDED `priority.go`/`manager.go` code +
§Consequences, status ACCEPTED); this PROGRESS.md final update. Per this
session's controller instructions (the phase-52 Task-10 precedent),
STATE.md/ROADMAP.md are NOT touched by this task — those land at the
controller's stage-close review.

### The final six-gate (ADR-0052)

1. **Build.**
   ```
   go build ./...          → BUILD_OK
   go mod tidy -diff       → exit 0, empty (ZERO new dep)
   ```
   Both green.

2. **Unit + race.**
   ```
   go test ./... -count=1               → all packages ok, zero FAIL
   go test -race -short ./... -count=1  → all packages ok, zero FAIL
   ```
   Both freshly re-run at Task 11 (redirected straight to log files, not
   piped through `tail`, to rule out any output-truncation artifact): 73
   package result lines in the unit run, 0 occurrences of `^FAIL`/`^--- FAIL`
   in either log; incl. `internal/cluster/priority_test.go`, the widened
   `manager_test.go`, and the `0096` driver's own unit tests (both confirmed
   `ok` under `-race`).

3. **Differential — the FULL 98-dir suite.**
   ```
   go test ./test/differential/ -count=1 -v
   ```
   Run against live Docker + `envoyproxy/envoy:contrib-v1.37.2`; log captured
   at Task 11 time in full and independently re-read end-to-end. Confirmed:
   `ok  	github.com/pgdad/envoy-go/test/differential	309.233s`; exactly
   **98** `--- PASS: TestDifferential/...` lines (one per fixture dir, no
   duplicates, no skips); **zero** real `FAIL`/`--- FAIL` lines anywhere in
   the transcript (the only two substring hits for `FAIL` are unrelated log
   text — `wasm: FAIL_CLOSED 503` — from an unrelated wasm fixture's own
   `failure_policy` logging, not a test result); includes
   `--- PASS: TestDifferential/0096-lb-priority (3.36s)` specifically. All 97
   prior dirs pass alongside the new `0096-lb-priority` (fixture 98) —
   byte-exact/unaffected, confirming the wrap-after-switch only fires for a
   multi-tier-priority cluster. `0096` itself was already separately
   liveness-proven at Task 10 (2 deliberate breaks, 20/20 flake-free after the
   `warmupStable` fix, `-race` clean); this gate re-confirms it live inside
   the full suite alongside every other fixture, with zero new regressions.

4. **Conformance.** h2spec + proxy-wasm are asserted-unaffected by this
   phase's change scope — priority-tier LB touches only the cluster LB pick
   path (`internal/cluster`), not HTTP/2 framing or the wasm path. No
   dedicated conformance re-run needed beyond the full differential suite
   above (which already exercises the h2spec/wasm-relevant fixtures
   unchanged).

5. **Counts.**
   - Fixtures: **97 → 98** (`0096-lb-priority`) — confirmed
     (`ls -d test/fixtures/[0-9]* | wc -l` → 98).
   - Stat surface: **1200 UNCHANGED** (+0) — confirmed, the family's FOURTH
     zero-stat-delta LB phase.
   - Fuzzers: **52 UNCHANGED** — confirmed (`grep -rh "^func Fuzz" | wc -l`).
   - BackendKind tail: **38 UNCHANGED** — confirmed (`H2GoawayResponder
     BackendKind = 38`).
   - DECISIONS.md tail: **ADR-0269 → ADR-0270** — confirmed; next-free
     **ADR-0271**.

6. **Docs.** `BEHAVIOR_CONTRACT.md`'s new `### Load balancer — priority
   (LocalityLbEndpoints.priority)` section lands immediately after the
   locality-weighted section, mirroring its shape (opening intro citing
   ADR-0270 + the phase-52 structural precedent; the wrap-after-switch
   acceptance; the `Endpoint.Priority` dimension; the confirmed capacity
   formula + corrected cascade; the AMEND-P1 bypass condition (explicitly NOT
   the classic panic threshold) + AMEND-P1-COROLLARY; the two composition
   rejects (verbatim wording) + AMEND-P2 framing; the confirmed-zero stat
   delta + D-P-HEALTHALLOC; D-P-DUP/D-P-FACTORY/D-P-RETROFIT; the
   differential proof shape; the deferred surface). The full ADR-0270 entry
   (§Context/§Decision/§Consequences) lands in `DECISIONS.md`. STATE.md /
   ROADMAP.md are explicitly OUT of scope for this task's commit (the
   controller updates row 53 `in-progress → done` after review — the
   phase-52 Task-10 precedent).

### FINAL ADR-0045 split-gate re-check (re-confirmed at completion)

> Quoting the PLAN's own FINAL re-check verbatim, re-confirmed now that all 11
> tasks have actually landed:

"This PLAN decomposes into **11 tasks** (well under the `>~25 tasks` gate)
over an estimated **~200-250 production LoC**: `priority.go` ~170-190 LoC
(`tierHealth`/`tierCapacity`/`cascadeLoads` + `priorityGroup`/`priorityLB`/
`priorityLeafFactory` + both constructors + `Pick`) + `cluster.go`'s
`Endpoint.Priority` field addition ~10 LoC + `manager.go`'s `extractEndpoints`
capture (~2 LoC delta) + the `distinctPriorities` helper (~8 LoC) + the
wrap-after-switch (~30-35 LoC) — well under the `>~1500 LoC` gate. **NO split
is needed.** The wrapper reuses the EXISTING `loadBalancer` interface, the
EXISTING `buildLeafLB` factory (confirmed compiling verbatim against
`priorityLeafFactory`'s 4-parameter shape, D-P-FACTORY), and the EXISTING
`clusterHealth` model (extended only by the pure `tierHealth` view-constructor,
itself ~5 LoC) UNCHANGED — there is no second seam to build. This is the
phase-35/37/52 single-flat-row precedent, repeated a FOURTH time: one new
file, one new wrap site, zero new packages, zero new producer plane."

**Verdict re-confirmed:** the landed `priority.go` (240 lines incl. comments)
+ the `cluster.go`/`manager.go` deltas land squarely within this envelope —
11 tasks, no escape valve triggered, exactly as anticipated at both the SPEC
and PLAN stages. NO split occurred.

## Phase 53 complete

All 11 tasks landed. This is the final PROGRESS.md update for this phase;
STATE.md/ROADMAP.md row 53 (`in-progress → done`) is a controller-side edit
made after reviewing this completed IMPL, not part of this task's own commit.
