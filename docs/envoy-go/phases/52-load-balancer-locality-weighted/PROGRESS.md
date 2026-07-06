# Phase 52 (`locality-weighted` LB) — IMPL PROGRESS

Worktree: `.worktrees/phase-52-impl`, branch `phase-52-load-balancer-locality-weighted-impl`.
Authoritative spine: `docs/envoy-go/phases/52-load-balancer-locality-weighted/PLAN.md`.

## Task table

| # | Task | Status |
|---|---|---|
| 1 | Baselines/anchors gate + PROGRESS.md | **done** |
| 2 | `Endpoint.Locality`/`LocalityWeight` + `extractEndpoints` per-group capture | **done** |
| 3 | `internal/cluster/locality.go` construction/grouping (TDD part A) | **done** |
| 4 | `localityWeightedLB.Pick` (TDD part B) | **done** |
| 5 | `manager.go` wrap-after-switch: 2 rejects + wrap + health-widening | **done** |
| 6 | Unit tests: formula curve, OPF A/B, AMEND-LW2, panic, both rejects | **done** |
| 7 | `0095` driver: topology + health-check-toggle harness | **done** |
| 8 | `0095` assertions: arm (a)/(b) bands + cross-side stats | **done** |
| 9 | `0095` deliberate breaks + flake-soak + `-race` | **done** |
| 10 | BEHAVIOR_CONTRACT + ADR-0269 + six-gate + ROADMAP note | **done** |

## Step 1 — Count anchors (re-confirmed live against this worktree's HEAD)

```
ls -d test/fixtures/[0-9]* | wc -l                                    → 96   (expected 96 — MATCH)
ls -d test/fixtures/[0-9]* | tail -1                                  → test/fixtures/0094-stats-sink-dogstatsd-batching   (MATCH)
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1  → H2GoawayResponder BackendKind = 38   (MATCH)
grep -rh "^func Fuzz" --include='*.go' . | wc -l                       → 52   (expected 52 — MATCH; reference_fuzzer_count_docs_drift recipe used)
grep "^## ADR-0" docs/envoy-go/DECISIONS.md | tail -1                  → ## ADR-0268 ...   (MATCH; next-free ADR-0269)
grep -n "1200" docs/envoy-go/BEHAVIOR_CONTRACT.md | tail -3            → hits at lines 789, 4699, 4701, all citing surface 1200   (MATCH — doc count, not a golden test)
go build ./...                                                        → BUILD_OK
go mod tidy -diff                                                      → empty, exit 0 → TIDY_EMPTY (ZERO new dep)
```

All count anchors match the PLAN's expectations exactly. No drift.

## Step 2 — As-built line anchors (re-pinned against this worktree's actual HEAD)

Every anchor below was re-verified via a fresh `grep -n`/`Read` against this exact worktree tip. Result: **ZERO drift** — every citation in PLAN.md's "As-built anchors" block (lines 32-39) matches the current HEAD exactly, line-for-line (unusual for a re-check, but expected since the PLAN was authored same-day against this HEAD per its own note).

- `internal/cluster/cluster.go:33-40` — `type Endpoint struct { Host string; Port uint32; Metadata map[string]SubsetValue }` (confirmed exact range). `:43-45` — `func (e Endpoint) Addr() string { return fmt.Sprintf(...) }` (confirmed exact range).
- `internal/cluster/manager.go:417` — `la := c.GetLoadAssignment()` (confirmed).
- `internal/cluster/manager.go:447-450` — `var health *clusterHealth` (447) / `if len(hcSpecs) > 0 || outlierCfg != nil { health = newClusterHealth(endpoints, parsePanicThreshold(c)) }` (448-450) (confirmed exact range).
- `internal/cluster/manager.go:457-465` — `lb, err := buildLeafLB(c, name, endpoints, health)` (457) ... `if sc := c.GetLbSubsetConfig(); sc != nil { lb = newSubsetLB(...) }` (461-465) (confirmed exact range — the `sc :=` line itself is at 461, inside the cited 457-465 span).
- `internal/cluster/manager.go:466-467` — `cl.lb = lb` (466) / `cl.health = health` (467) (confirmed exact range).
- `internal/cluster/manager.go:341-388` — `func buildLeafLB(c *clusterv3.Cluster, name string, endpoints []Endpoint, health *clusterHealth) (loadBalancer, error)` (signature at 341, confirmed start).
- `internal/cluster/manager.go:771-795` — `func extractEndpoints(la *endpointv3.ClusterLoadAssignment, clusterName string) ([]Endpoint, error)` (signature at 771, confirmed start); the single production `Endpoint{...}` construction site is at **`:788`** exactly: `out = append(out, Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars})` — confirmed exact line match.
- `internal/cluster/manager.go:112-170` — `func registerClusterMetrics(r *stats.Registry, c *Cluster)` (signature at 112, confirmed start).
- `internal/cluster/subset.go:140-150` — `subsetLB` struct (confirmed exact range: opens 140, closes 150).
- `internal/cluster/subset.go:158-205` — `func newSubsetLB(endpoints []Endpoint, cfg lbSubsetCfg, factory leafFactory) *subsetLB` (confirmed exact range: opens 158, closes 205).
- `internal/cluster/subset.go:213-230` — `func (s *subsetLB) Pick(...)` (signature at 213, confirmed start).
- `internal/cluster/subset.go:129-131` — `type leafFactory func(sub []Endpoint) (loadBalancer, error)` with its doc comment (confirmed exact range: comment 129-130, type 131).
- `internal/cluster/loadbalancer.go:16-23` — `type loadBalancer interface { Pick(...) (Endpoint, func(), error) }` (signature at 16, confirmed start).
- `internal/cluster/loadbalancer.go:44-66` — `func (rr *roundRobin) Pick(...)` (signature at 44, confirmed start).
- `internal/cluster/leastrequest.go:61-81` — `func newPCGRNG() (func() uint64, error)` incl. doc comment (comment opens 61, func closes 81 — confirmed exact range).
- `internal/cluster/leastrequest.go:42-59` — `func newLeastRequest(endpoints []Endpoint, choiceCount int) (*leastRequest, error)` (42) / `func newLeastRequestWithRNG(endpoints []Endpoint, choiceCount int, rng func() uint64) *leastRequest` (52-59) (confirmed exact range).
- `internal/cluster/random.go:24-36` — `func newRandom(endpoints []Endpoint) (*randomLB, error)` (24) / `func newRandomWithRNG(endpoints []Endpoint, rng func() uint64) *randomLB` (34-36) (confirmed exact range).
- `internal/cluster/health.go:44-48` — `func newHostHealth() *hostHealth` (confirmed exact range).
- `internal/cluster/health.go:83-93` — `func newClusterHealth(endpoints []Endpoint, panicThreshold float64) *clusterHealth` (confirmed exact range).
- `internal/cluster/health.go:142-150` — `func (ch *clusterHealth) availableCount(eps []Endpoint) int` (confirmed exact range).
- `internal/cluster/health.go:165-171` — `func (ch *clusterHealth) inPanic(eps []Endpoint) bool` (confirmed exact range).
- `internal/cluster/health.go:180-185` — `func (ch *clusterHealth) panicInc()` incl. doc comment (comment 180, func 181-185 — confirmed exact range).
- `internal/cluster/health.go:429-437` — `func parsePanicThreshold(c *clusterv3.Cluster) float64` incl. doc comment (comment 429-430, func 431-437 — confirmed exact range).
- `internal/cluster/locality.go` — **confirmed ABSENT** (`test -f` → not found). Expected: Task 3 creates it.

**Verdict: zero line-number drift.** Every citation in PLAN.md's as-built anchors block is safe to use verbatim in Tasks 2-10 without re-adjustment.

## Step 3 — `Endpoint{}` construction-site sweep

Ran the three sweep greps specified by the PLAN:
- `grep -rn "Endpoint{" internal/cluster/*.go | grep -v _test.go`
- `grep -rn "Endpoint{" internal/cluster/*_test.go`
- `grep -rn "cluster\.Endpoint{" --include=*.go internal/filter/ | sort`

**Result: every hit is either a bare `Endpoint{}` zero-value literal (error-path returns, unaffected by new fields) or a KEYED literal (`Endpoint{Host: ..., Port: ...}` / `...Metadata: ...`). NOT ONE positional literal exists anywhere in the sweep.**

The single PRODUCTION construction site with real field values is `internal/cluster/manager.go:788` inside `extractEndpoints`:
```go
out = append(out, Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars})
```

Every other site (leaf-policy error-path returns across `cluster.go`, `leastrequest.go`, `random.go`, `h2pool.go`, `loadbalancer.go`, `maglev.go`, `ringhash.go`, `subset.go`; test builders across `cluster_test.go`, `dial_h2_test.go`, `maglev_test.go`, `h2pool_coalesce_test.go`, `manager_test.go`, `h2pool_test.go`, `leastrequest_test.go`, `loadbalancer_test.go`, `outlier_test.go`, `health_test.go`, `subset_test.go`; every `cluster.Endpoint{}` site under `internal/filter/hcm/` and `internal/filter/http/router/`) compiles unchanged once `Endpoint` gains `Locality LocalityID` and `LocalityWeight uint32` — the two new fields default to their zero values (`LocalityID{}`, `0`) at every keyed/bare-literal site.

**Conclusion (verified finding, not an assumption inherited from SPEC's generic caution): adding `Locality`/`LocalityWeight` to `Endpoint` (Task 2) requires editing EXACTLY ONE call site — `extractEndpoints` (`manager.go:788`, the whole function 771-795). Zero other `Endpoint{}` sites anywhere in the repo need edits.**

## D-question resolutions (SPEC §12, copied verbatim from PLAN.md)

- **D-LW-DUP (the duplicate-locality-identity corner)** — RESOLVED as SPEC's own self-answer: last-write-wins in endpoint-encounter order, realized by `newLocalityWeightedLB`'s `weights[ep.Locality] = ep.LocalityWeight` map-overwrite (Task 3). This PLAN adds the concrete unit test SPEC left implicit: `TestNewLocalityWeightedLB_DuplicateLocality_LastWriteWins` (Task 3) constructs two `Endpoint`s sharing an identical `LocalityID` but different `LocalityWeight` values (simulating two source `LocalityLbEndpoints` groups that collapsed to the same identity in `extractEndpoints`'s output) and asserts (a) the resulting `localityGroup.weight` is the LAST-encountered value and (b) the group's `endpoints` slice contains BOTH endpoints merged (grouping is by identity, not by source-group index) — an unusual, degenerate config shape, low-stakes and untested by either live probe, exactly as SPEC frames it.
- **D-LW-HEALTHALLOC (unconditional `clusterHealth` allocation when `locality_weighted_lb_config` is present, even with zero `health_checks`)** — CONFIRMED safe against the ACTUAL code (not just SPEC's assertion): `newHostHealth` (`health.go:44-48`) defaults `healthy.Store(true)`, so a widened `clusterHealth` with zero active checkers running leaves every host permanently "available" — `availableCount` (`:142-150`) returns `len(eps)` and `inPanic` (`:165-171`) never fires. Verified additionally: `registerClusterMetrics`'s EXISTING `if c.health != nil { ... }` block (`manager.go:163-169`) is UNCONDITIONAL on `hcSpecs` — it will now ALSO register `membership_healthy`/`lb_healthy_panic` for a locality-weighted cluster with NO `health_checks`, a side effect not called out in the SPEC. This is NOT a new stat NAME (both already exist for HC-configured clusters) — it WIDENS the CONDITION under which two pre-existing names appear. Task 5 adds a dedicated unit test proving this (`TestManager_LocalityWeighted_WidensHealthWithoutHealthChecks`) and Task 10's BEHAVIOR_CONTRACT delta documents it as a recorded, minor side effect (not a stat-surface COUNT change, since the fixture and every realistic config pairs `locality_weighted_lb_config` with `health_checks` anyway).
- **D-LW-OPF0 (explicit `overprovisioning_factor: {value: 0}` vs absent)** — RESOLVED with the MORE CORRECT design, not the SPEC's leaning-to-accept-the-departure default: reading `manager.go:417`'s `la := c.GetLoadAssignment()` shows the `*wrapperspb.UInt32Value` pointer (`la.GetPolicy().GetOverprovisioningFactor()`) is ALREADY in scope at the call site BEFORE any `.GetValue()` call — distinguishing "absent" (nil pointer) from "explicit `{value: 0}`" (non-nil pointer, `GetValue()==0`) costs exactly one `!= nil` check, which is free. This PLAN threads `(opf uint32, hasOPF bool)` into `newLocalityWeightedLB` (Task 3) instead of the SPEC's single bare `uint32` parameter: `hasOPF=false` → defaults to 140 (the confirmed reference default, AMEND-LW3); `hasOPF=true, opf=0` → HONORED LITERALLY as 0, which (per the confirmed formula) makes `min(1, 0×frac)==0` for EVERY locality regardless of health — i.e., an explicit `overprovisioning_factor: 0` degrades the ENTIRE mechanism to the flat fallback UNCONDITIONALLY (the AMEND-LW2 "zero total effective weight" unification path fires on every `Pick`, not just during degradation). Task 6 adds `TestPick_ExplicitZeroOPF_AlwaysFallsBackToFlat` proving this distinct-from-140 outcome. One sentence why: the correct behavior was free to implement once the actual call site was read, so there is no reason to accept the documented-departure fallback SPEC left as its default recommendation.

## ADR-0045 re-check verdict

**NO split.** 10 tasks, ~230-330 estimated production LoC (`locality.go` + `Endpoint` field additions + `extractEndpoints` extension + `manager.go`'s wrap-after-switch site — a single flat family row, matching the phase-38/39/40 precedent scale). Confirmed at Task 1 against the live anchors: the wrapper reuses `buildLeafLB`/`leafFactory`/`clusterHealth`/`newPCGRNG` verbatim (zero new abstractions beyond `locality.go` itself), and needs ZERO new `Pick` parameters (D-LW7) — the smallest-surface-area wrapper in the LB family to date. This PLAN proceeds as a single 10-task IMPL, no escape valve, no parent/child phase split.

## Task 9 — `0095` deliberate-break liveness + ≥20-run flake check + `-race`

Both `0095` `AssertStats` bands proven LIVE via the SPEC §8.1 deliberate
breaks (`reference_differential_break_protocol_count1`); each applied, run,
observed FAILING, then undone with `git restore` (never checkout-sha/amend —
`feedback_subagent_worktree_detach`). Full detail (exact edits + FAILING
output) recorded in `test/fixtures/0095-lb-locality-weighted/README.md`'s new
"Task 9" section; summary:

- **Break (i)** — commented out `manager.go`'s `case lwc != nil:` body's
  `lb = lw` (flat, unwrapped `buildLeafLB` fallback). FAILED: `subject/static:
  region A share = 50.00% (a=450 b=450), want 66.7% ± 8.0pp` (arm (b) fails
  too, as expected — same unwrapped LB). Restored → PASS.
- **Break (ii)** — hardcoded `frac := 1.0` unconditionally in `locality.go`'s
  `Pick` per-group loop (health-blind). FAILED: `subject/degraded: region A
  share = 67.33% (a=606 b=294), want 52.8% ± 8.5pp` (arm (a) unaffected, as
  expected — measured at 100% health where the break is a no-op). Restored →
  PASS.

**A genuine bug found and fixed while running the ≥20-run flake check** (NOT
a flake — deterministic 19/20 failure on the first attempt): `0095`'s
`lwDriver` is a process-lifetime singleton (`init()`-registered), and its 5
region-A `toggleResponder`s are memoized once. Arm (b)'s `SetHealthy(false)`
was never reset, so `-count=N` run 2 onward inherited run 1's degraded health
state; worse, a first-attempt fix (resetting inside `AssertStats`) arrived
too late — Envoy's active health checker locks a host observed-unhealthy-on-
its-first-probe onto the `no_traffic_interval` cadence (default 60s, unset in
this fixture) until cluster traffic flows once, blowing past the 30s
convergence-poll deadline. **Fix:** reset every `toggleResponder` to healthy
inside `ensureRegionA` itself, on every call — before the containers boot, so
both sides see all 10 hosts healthy from their first probe
(`test/fixtures/0095-lb-locality-weighted/driver/driver.go`, `ensureRegionA`).

Post-fix verification (both breaks RE-confirmed live/restored against the
fixed driver, then):
- `-count=20`: **20/20 PASS** (89.4s).
- `-race -count=1`: **PASS**, no data race.
- `gofmt -l` / `go vet` / `golangci-lint run` on the touched packages
  (`internal/cluster`, `test/fixtures/0095-lb-locality-weighted/driver`):
  all clean.

Working tree confirmed clean of the deliberate-break edits before this
task's commit (`git diff HEAD` shows only the `ensureRegionA` fix +
README.md + this PROGRESS.md).

## Known doc-vs-code path label (flagged for Task 10, not fixed here)

This worktree's `go.mod` module line reads:
```
module github.com/pgdad/envoy-go
```
confirming the mechanical module-path rename (`github.com/esalaine/envoy-go` → `github.com/pgdad/envoy-go`) from a separate, already-merged PR is present at this worktree's base. No source file under `internal/cluster` imports its OWN package by path, so this rename has ZERO effect on any code touched by this phase's Tasks 1-9.

However, `docs/envoy-go/DECISIONS.md`'s existing ADR-0268 entry (and likely other older ADR entries) still narrate the OLD path verbatim, e.g.:
> "the new public `github.com/esalaine/envoy-go/validate` package ... the FIRST non-`internal/`/non-`cmd/`/non-`test/` package in this repo"

This is a pre-existing prose/doc artifact, NOT something Task 1-9 of this phase touches or introduces. **Flagged for Task 10** (which adds the new BEHAVIOR_CONTRACT subsection + the ADR-0269 DECISIONS.md entry): when authoring ADR-0269's own §Context/§Decision/§Consequences prose, use the CURRENT `github.com/pgdad/envoy-go` module path if the new entry needs to reference the module path at all (locality-weighted is a cluster-internal construct with no public package, so ADR-0269 likely has no reason to cite a module path either way — but if it does, use the correct current one). Do NOT retroactively fix the OLD ADR-0268 (or earlier) entries' path references as part of this phase — that is out of scope here and not requested by the PLAN.

Resolved at Task 10: ADR-0269's prose is a cluster-internal construct (no public package) and never needed to cite a module import path at all — the flag is moot, no path string appears in the new entry.

## Task 10 — completion bundle: BEHAVIOR_CONTRACT + ADR-0269 + final six-gate

**BEHAVIOR_CONTRACT.md** gained a new `### Load balancer — locality-weighted (locality_weighted_lb_config)` section, inserted immediately after the existing `### Load balancer — subset (lb_subset_config)` section, mirroring that section's shape (italic intro citing ADR-0269 + the seam-reuse framing, then prose paragraphs + a bulleted departures/coverage-boundaries list + a deferred-surface list). It covers the wrap-after-switch acceptance, the `Endpoint.Locality`/`LocalityWeight` dimension, the confirmed AMEND-LW2/LW3/LW4 formula findings, the two VERBATIM reject strings, the confirmed-zero stat delta PLUS the D-LW-HEALTHALLOC condition-widening side effect, the D-LW-OPF0 absent-vs-explicit-zero resolution, the child-local-panic coverage boundary, the `0095` differential proof shape, and the deferred surface. Stat-surface doc count confirmed unchanged at **1200**.

**DECISIONS.md** gained the full **ADR-0269** entry (§Context promoted near-verbatim from SPEC.md §13's draft, adapted to landed-code framing + §Decision grounded in the actual `internal/cluster/locality.go`/`manager.go` code (constructors, `Pick`, the wrap-after-switch, the two reject strings) + §Consequences covering D-LW-HEALTHALLOC, the upgraded D-LW-OPF0 design, the child-local-panic boundary, the confirmed-1200 stat surface, zero new packages/deps, and a prose-only note that ROADMAP row 52 flips `done` at the CONTROLLER's stage-close (not edited by this task). DECISIONS.md tail is now **ADR-0269**; next-free **ADR-0270**.

### ADR-0045 split-gate re-check — HELD

Re-confirmed against the actual landed diff (`git diff master --stat`): **11 files changed, 1947 insertions(+), 4 deletions(-)** across the whole IMPL (production + tests + fixture driver + docs). The core `internal/cluster` PRODUCTION code delta is:

```
internal/cluster/cluster.go   | 24 ++-
internal/cluster/locality.go  | 174 +++++++++++++++ (new file)
internal/cluster/manager.go   | 50 ++++-
```
— **~248 net production LoC**, squarely inside the PLAN's ~220-320 (SPEC's ~220-320 / PLAN's ~230-330) estimate. The larger overall diff total is dominated by test code (`locality_test.go` 436, `manager_test.go` 189) and the `0095` fixture driver (`driver.go` 639, `README.md` 194, `expectations.yaml` 37, `driver_test.go` 56) — expected for a differential-proven LB construct, not a sign of scope creep. **NO split was needed** (well under both the `>~25 tasks` and `>~1500 LoC` ADR-0045 gates); the 10-task decomposition proceeded as planned with zero escape-valve triggers.

### Final six-gate evidence (this task's scope: `internal/cluster`/`validate`/`cmd` + repo-wide static checks; the full 97-dir differential re-verification is the CONTROLLER's separate job)

```
go build ./...                                                → clean, no output
go test ./internal/cluster/... ./validate/... ./cmd/... -count=1
  ok  github.com/pgdad/envoy-go/internal/cluster   3.678s
  ok  github.com/pgdad/envoy-go/validate           0.224s
  ok  github.com/pgdad/envoy-go/cmd/envoy-go        7.036s
gofmt -l .                                                    → empty (repo-wide)
go vet ./...                                                  → clean, no output (repo-wide)
golangci-lint run ./...                                       → clean, no output (repo-wide)
```

All six-gate checks in this task's scope are GREEN. Counts reconfirmed at Task 10 tip: differential fixtures **97** (`ls -d test/fixtures/[0-9]* | wc -l` → 97; tail dir `0095-lb-locality-weighted`); fuzzers **52** (`^func Fuzz` count unchanged); BackendKind tail **38** (`H2GoawayResponder BackendKind = 38`, unchanged); DECISIONS.md tail **ADR-0269** (next-free ADR-0270); stat surface doc references still cite **1200**.

**Docs-only commit scope confirmed:** this task's commit touches only `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/DECISIONS.md`, and this `PROGRESS.md` — no production/test code changed, so the six-gate green above is a confirmation that Task 10 introduced zero regressions, not a re-proof of Tasks 1-9's own work (already gated at their own commits).

**Explicitly NOT touched by this task** (per this session's controller instructions, reiterated at the top of the PLAN's Task 10 section): `STATE.md`, `ROADMAP.md` (row 52 stays `in-progress` until the controller flips it `done` post-review), `next-prompt.txt`. Those are the controller's stage-close job.
