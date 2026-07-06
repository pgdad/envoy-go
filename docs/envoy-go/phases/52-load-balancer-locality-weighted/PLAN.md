# Phase 52 Implementation Plan — `locality-weighted` LB (`Cluster.CommonLbConfig.locality_weighted_lb_config`): a health-as-continuous-weight WRAPPER over the cluster's `lb_policy` child — the SIXTH Load-balancing-family construct, the family's cheapest wrapper yet (ZERO new `Pick` parameters)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `Cluster.CommonLbConfig.locality_weighted_lb_config` (`envoy.config.cluster.v3`, oneof field 3 of `CommonLbConfig`) — a `localityWeightedLB` wrapper that partitions a cluster's endpoints by `LocalityLbEndpoints.locality` identity, weights each locality by its configured `load_balancing_weight` degraded by that locality's live per-host healthy fraction (scaled by `ClusterLoadAssignment.Policy.overprovisioning_factor`), and delegates to a per-locality child `loadBalancer` built by the EXISTING policy factory (`buildLeafLB`) — reusing the EXISTING `loadBalancer.Pick` seam with ZERO new parameters.

**Architecture:** ONE new file `internal/cluster/locality.go` (the `subset.go`/`ringhash.go`/`maglev.go` sibling precedent) + two new `Endpoint` fields (`Locality LocalityID`, `LocalityWeight uint32` — the SECOND per-endpoint dimension after phase 38's `Metadata`) + an extended `extractEndpoints` (per-`LocalityLbEndpoints`-group capture) + a SECOND wrap-after-switch site in `manager.go`'s `buildCluster`, placed immediately after the EXISTING `lb_subset_config` wrap. NO new package, NO new go.mod dependency, NO new producer plane (a cluster-only construct — `common_lb_config`/`LocalityLbEndpoints`/`ClusterLoadAssignment.Policy` are all cluster-scoped; nothing attaches to an HTTP route), NO new stat (CONFIRMED zero delta, SPEC §11.5/AMEND-LW5).

**Tech Stack:** Go 1.26.x; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008 — every proto field used here, incl. `Cluster_CommonLbConfig_LocalityWeightedLbConfig`/`Cluster_CommonLbConfig_ZoneAwareLbConfig_`/`LocalityLbEndpoints.Locality`/`LoadBalancingWeight`/`ClusterLoadAssignment_Policy.OverprovisioningFactor`, is ALREADY present in the pinned module — `go mod tidy -diff` stays EMPTY, re-verified live at Task 1); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227 — already live-probed at the SPEC, §11). Reuses `internal/cluster/` (the phase-38 `subsetLB` structural precedent — a wrapper `loadBalancer` owning per-group children built by `buildLeafLB`; the phase-39/40 `clusterHealth`/`hostHealth` model, `health.go`; the `newPCGRNG` crypto-seeded `math/rand/v2` PCG idiom, `leastrequest.go:61-81`, directly reused — matching `random.go`'s posture, NOT the different-package `router_weighted.go`'s independent `newWeightedRNG` duplication). ZERO new packages, ZERO new go.mod deps.

## Global Constraints

- Two NEW envoy-go-strict departure rejects, wording VERBATIM per SPEC §6.1 (both confirmed live-probed departures, §11.1):
  1. `cluster: %q: common_lb_config.zone_aware_lb_config is not supported`
  2. `cluster: %q: lb_subset_config cannot be combined with common_lb_config.locality_weighted_lb_config`
- The confirmed effective-weight formula (SPEC §11.3/AMEND-LW3), MUST be implemented exactly, not approximated: `effective_weight = configured_weight × min(1, (overprovisioning_factor/100) × healthy_fraction)`.
- `overprovisioning_factor` default is **140** when the wrapper is ABSENT (nil `*wrapperspb.UInt32Value`) — confirmed live (§11.3). See the D-LW-OPF0 resolution below for how an EXPLICIT `{value: 0}` differs.
- Cluster-wide panic (the EXISTING `clusterHealth.inPanic` gate, `health.go:165-171`, UNCHANGED) FULLY BYPASSES both health-filtering and locality-weighting — confirmed live (§11.4/AMEND-LW4).
- `loadBalancer.Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error)` (`loadbalancer.go:16-23`) stays BYTE-FOR-BYTE unchanged — ZERO new pick-input (D-LW7, confirmed §11.7).
- Stat surface stays **1200 (+0)** — confirmed zero delta (§11.5/AMEND-LW5); NO new `registerClusterMetrics` stat handle for locality-weighted itself.
- Every task runs `gofmt -l` + `golangci-lint run` on the touched packages, PER TASK (not just a final gate) — `feedback_pertask_gofmt_lint`.
- Subagents commit LOCAL-ONLY; the controller squash-merges + pushes at stage-close (`feedback_subagents_no_push`).
- Every new differential assertion is proven live via a deliberate break run with `-count=1` (`reference_differential_break_protocol_count1`); targeted runs use `-run 'TestDifferential/0095'`, never `-run '0095'` (`reference_differential_run_selector`).

---

## Source-of-truth references

- **SPEC:** `docs/envoy-go/phases/52-load-balancer-locality-weighted/SPEC.md` — AUTHORITATIVE. §1.1 AMEND-LW1..LW5 (the live-probe findings); §3.0 the split disposition (single flat row, NO escape valve); §3.1 the `localityWeightedLB` indicative design; §3.2 the seam-REUSE confirmation (zero `Pick` change); §3.3 the manager wrap-after-switch + the health-registry-widening note; §4 the `Endpoint` dimension + `extractEndpoints` extension + the `overprovisioning_factor` capture; §5 the proto roster; §6 the two-reject roster + the non-reject dispositions; §7 the confirmed-zero stat delta; §8 the `0095` differential design (both arms + the deliberate breaks); §9 the BEHAVIOR_CONTRACT delta shape; §10 the ~10-11-task spine (this PLAN decomposes it 1:1, see the table below); §11 the D-LW1..D-LW8 empirical pins (all RESOLVED, none deferred to this PLAN); §12 the THREE PLAN-level D-questions this PLAN resolves below; §13 the ADR-0269 §Context DRAFT; §14 the exit counts.
- **BRAINSTORM:** `docs/envoy-go/phases/52-load-balancer-locality-weighted/BRAINSTORM.md` — the charter (Q0-Q4); §2.3-2.6 architecture rationale (the `subsetLB` structural precedent chosen over a from-scratch design; the zero-new-`Pick`-parameter insight). The BRAINSTORM's "unset weight defaults to 1" assumption (§2.4) is REFUTED at the SPEC (AMEND-LW2) — this PLAN follows the SPEC's corrected design throughout.
- **The phase-38.1 PLAN** (`docs/envoy-go/phases/38-load-balancer-subset/PLAN.md`) — the STRUCTURAL TEMPLATE this PLAN's section shape mirrors (Source-of-truth references / Project conventions / D-question resolutions / File Structure / per-task Files+Interfaces+Steps / a final Verification checklist). Also the direct precedent for: the `buildLeafLB` extraction (ALREADY LANDED, reused unchanged here), the wrapper-owns-per-group-children-via-factory shape (`subsetLB` → `localityWeightedLB`), and the `Endpoint`-dimension-addition discipline (`Metadata` → `Locality`/`LocalityWeight`).
- **As-built anchors** (captured at worktree tip `8154e55f`; RE-CONFIRM at Task 1 — line numbers shift on the IMPL-session tip; every citation below was verified fresh against this exact HEAD while authoring this PLAN, not copied from the SPEC without re-checking):
  - `internal/cluster/cluster.go:33-40` — the `Endpoint{Host, Port, Metadata}` struct (gains `Locality`/`LocalityWeight` at Task 2) + `:43-45` (`Addr()` — UNCHANGED, ignores both new fields).
  - `internal/cluster/manager.go:417` (`la := c.GetLoadAssignment()` — ALREADY in scope; the `ClusterLoadAssignment.Policy.overprovisioning_factor` read site, Task 5) + `:447-450` (`var health *clusterHealth; if len(hcSpecs) > 0 || outlierCfg != nil { health = newClusterHealth(...) }` — the EXISTING nil-health-fast-path convention that Task 5's health-registry-widening deliberately departs from) + `:457-465` (`buildLeafLB` call + the EXISTING `if sc := c.GetLbSubsetConfig(); sc != nil { lb = newSubsetLB(...) }` wrap — Task 5 hoists `sc` and appends the SECOND wrap-after-switch immediately after) + `:466-467` (`cl.lb = lb; cl.health = health` — UNCHANGED assignment sites, now potentially seeing a widened `health`) + `:341-388` (`buildLeafLB(c, name, endpoints, health) (loadBalancer, error)` — the REUSED, UNCHANGED factory; the CLUSTER_PROVIDED/unsupported-policy reject lives here, `:385-387`) + `:771-795` (`extractEndpoints` — gains the per-group `Locality`/`LoadBalancingWeight` capture at Task 2; the single production `Endpoint{...}` construction site is at `:788`) + `:112-170` (`registerClusterMetrics` — UNCHANGED this phase, but its EXISTING `if c.health != nil { ... membership_healthy ... lb_healthy_panic ... }` block at `:163-169` now ALSO fires for a locality-weighted cluster with zero `health_checks`, per Task 5's widening — a side effect, not a code change here).
  - `internal/cluster/subset.go:140-150` (`subsetLB` struct — the wrapper-owns-per-group-children-via-factory STRUCTURAL PRECEDENT) + `:158-205` (`newSubsetLB` — the enumeration-then-factory-build precedent) + `:213-230` (`subsetLB.Pick` — the exact forwarding shape `localityWeightedLB.Pick` mirrors: `child.Pick(hashKey, hasHash, match, hasMatch)` unchanged) + `:129-131` (`type leafFactory func(sub []Endpoint) (loadBalancer, error)` — REUSED VERBATIM, no new factory type needed).
  - `internal/cluster/loadbalancer.go:16-23` (`type loadBalancer interface { Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) }` — stays BYTE-FOR-BYTE unchanged) + `:44-66` (`roundRobin.Pick` — the panic-branch + `panicInc()` call-site CONVENTION `localityWeightedLB.Pick` mirrors at its own panic branch).
  - `internal/cluster/leastrequest.go:61-81` (`newPCGRNG() (func() uint64, error)` — the crypto-seeded, mutex-guarded PCG idiom REUSED verbatim by `newLocalityWeightedLB`, exactly as `random.go`'s `newRandom` already does) + `:42-59` (`newLeastRequest`/`newLeastRequestWithRNG` — the crypto-constructor/injectable-constructor SPLIT this PLAN's `newLocalityWeightedLB`/`newLocalityWeightedLBWithRNG` mirrors).
  - `internal/cluster/random.go:24-36` (`newRandom`/`newRandomWithRNG` — the SECOND, simpler precedent for the same split, since `randomLB` — like `localityWeightedLB` — holds no per-endpoint mutable counter state).
  - `internal/cluster/health.go:44-48` (`newHostHealth` — `healthy.Store(true)` default, the D-LW-HEALTHALLOC safety argument) + `:83-93` (`newClusterHealth`) + `:142-150` (`availableCount(eps []Endpoint) int` — REUSED, called with a LOCALITY's own sub-slice instead of the whole cluster's) + `:165-171` (`inPanic(eps []Endpoint) bool` — REUSED unchanged, called by the wrapper with `lw.allEndpoints`) + `:180-185` (`panicInc()` — REUSED, same shared `clusterHealth.panicCounter` handle) + `:429-437` (`parsePanicThreshold` — REUSED unchanged).
  - `docs/envoy-go/BEHAVIOR_CONTRACT.md:1232-1263` — the `### Load balancer — subset (lb_subset_config)` section, the STRUCTURAL TEMPLATE Task 10's new subsection mirrors (opening italic ADR-citing intro; wrap-after-switch acceptance; value semantics; departures + coverage boundaries; deferred surface).
  - `docs/envoy-go/DECISIONS.md` tail `## ADR-0268` (bootstrap-validate-mode) — the single-ADR-body landing-shape precedent (§Context/§Decision/§Consequences, ADR-0044 in-place) Task 10's ADR-0269 entry mirrors.
- **Differential harness template:** `test/fixtures/0066-health-check-http/driver/driver.go` — the poll-to-converge (`pollMembershipHealthy`) + warmup (`warmupUntilStable`) + delta-baseline (`scrapeStats` before/after) pattern Task 7/8 reuse VERBATIM (per SPEC §8.1's explicit call-out that arm (b) reuses "the phase-39 poll-to-convergence-and-warmup pattern"), PLUS the "driver-owned self-managed `net.Listener`" idiom (`allocDeadPort`) that Task 7's `toggleResponder` generalizes from "permanently closed" to "live and toggleable". `test/differential/fixture/fixture.go` — the `Driver`/`StatsAsserter`/`TB`/`BackendKind` interfaces (`HTTPEcho BackendKind = 1`; tail `H2GoawayResponder BackendKind = 38`, UNCHANGED this phase).

## Project conventions honored throughout (memory + ADRs)

- `feedback_execution_style` / `feedback_git_worktrees` — subagent-driven execution; this PLAN was authored in worktree `.worktrees/phase-52-plan`; the IMPL runs in its own fresh worktree.
- `feedback_subagents_no_push` / `feedback_subagent_worktree_path_targeting` — subagents commit LOCAL-ONLY; every path below is repo-root-relative; PROGRESS.md is pinned at `docs/envoy-go/phases/52-load-balancer-locality-weighted/PROGRESS.md`.
- `feedback_pertask_gofmt_lint` — every task's own gate runs `gofmt -l` + `golangci-lint run` on touched packages.
- `reference_conn_wrap_method_no_promote` (generalized) — every `Endpoint{}` construction site must be checked when the struct grows a field; Task 1/2 ENUMERATE every site (see the D-question-adjacent finding below: all existing sites use KEYED literals, so none require editing — a genuine, verified finding, not an assumption).
- `reference_differential_band_sigma_margin` — `0095`'s two statistical-band assertions (arm (a)/(b) region-share) use a ~5σ margin at n=900 per arm (computed explicitly in Task 7/8, not eyeballed).
- `reference_health_check_propagation_warmup` — both arms of `0095` poll `membership_healthy` to convergence THEN run the K=10-consecutive-non-503 warmup before measuring (the phase-39 pattern, reused for arm (b)'s SECOND convergence point too — a novel double-convergence sequence within one fixture).
- `reference_differential_break_protocol_count1` / `reference_differential_run_selector` / `reference_fixture_workload_constant_desync` — Task 9's deliberate breaks; all workload constants (`staticLoadCount`, `degradedLoadCount`, `regionAHosts`, `degradedAHosts`) are named constants, never re-derived literals.
- `reference_differential_asserter_dispatch` — the cross-side stats prong uses `StatsAsserter` (in-band, holding both admin addrs); no separate `DistributionAsserter` (the region-share bands are computed INSIDE `AssertStats` off the same load-and-tally pass, since arm sequencing — degrade AFTER measuring arm (a) — requires one continuous in-band flow, the `0066`/`0069` precedent).
- `reference_docker_probe_bridge_network` — `0095`'s driver-owned `toggleResponder`s bind `0.0.0.0:0` and are addressed via `host.docker.internal` on the reference side (the `allocDeadPort` addressing precedent, generalized).
- ADR-0269 (the locality-weighted policy + the `Endpoint` dimension + the panic interaction + the `zone_aware_lb_config`/`lb_subset_config` sibling-rejects; §Context DRAFTED at SPEC §13, §Decision/§Consequences land at Task 10) — the SOLE anticipated ADR (a single-ADR reuse-shape phase, the phase-35/37 precedent, confirmed by D-LW7's zero-seam-change finding). ADR-0243 (the health-aware LB pick — Approach A, build-time-injected health view) — REUSED unchanged; ADR-0240 (`subsetLB`'s wrapper-owns-children-via-factory shape) — the direct structural precedent. ADR-0024 (per-cluster LB-state scope) — UNAMENDED; locality-group state is per-cluster LB-instance state, the same discipline every prior LB construct follows. ADR-0080 (byte-stable reject text — the two VERBATIM strings above). ADR-0052 (the atomic six-gate completion bundle). ADR-0106 (flat family row, no parent rollup). ADR-0045 (the split-gate — FINAL re-check at the end of this PLAN).

## D-question resolutions (SPEC §12)

- **D-LW-DUP (the duplicate-locality-identity corner)** — RESOLVED as SPEC's own self-answer: last-write-wins in endpoint-encounter order, realized by `newLocalityWeightedLB`'s `weights[ep.Locality] = ep.LocalityWeight` map-overwrite (Task 3). This PLAN adds the concrete unit test SPEC left implicit: `TestNewLocalityWeightedLB_DuplicateLocality_LastWriteWins` (Task 3) constructs two `Endpoint`s sharing an identical `LocalityID` but different `LocalityWeight` values (simulating two source `LocalityLbEndpoints` groups that collapsed to the same identity in `extractEndpoints`'s output) and asserts (a) the resulting `localityGroup.weight` is the LAST-encountered value and (b) the group's `endpoints` slice contains BOTH endpoints merged (grouping is by identity, not by source-group index) — an unusual, degenerate config shape, low-stakes and untested by either live probe, exactly as SPEC frames it.
- **D-LW-HEALTHALLOC (unconditional `clusterHealth` allocation when `locality_weighted_lb_config` is present, even with zero `health_checks`)** — CONFIRMED safe against the ACTUAL code (not just SPEC's assertion): `newHostHealth` (`health.go:44-48`) defaults `healthy.Store(true)`, so a widened `clusterHealth` with zero active checkers running leaves every host permanently "available" — `availableCount` (`:142-150`) returns `len(eps)` and `inPanic` (`:165-171`) never fires. Verified additionally: `registerClusterMetrics`'s EXISTING `if c.health != nil { ... }` block (`manager.go:163-169`) is UNCONDITIONAL on `hcSpecs` — it will now ALSO register `membership_healthy`/`lb_healthy_panic` for a locality-weighted cluster with NO `health_checks`, a side effect not called out in the SPEC. This is NOT a new stat NAME (both already exist for HC-configured clusters) — it WIDENS the CONDITION under which two pre-existing names appear. Task 5 adds a dedicated unit test proving this (`TestManager_LocalityWeighted_WidensHealthWithoutHealthChecks`) and Task 10's BEHAVIOR_CONTRACT delta documents it as a recorded, minor side effect (not a stat-surface COUNT change, since the fixture and every realistic config pairs `locality_weighted_lb_config` with `health_checks` anyway).
- **D-LW-OPF0 (explicit `overprovisioning_factor: {value: 0}` vs absent)** — RESOLVED with the MORE CORRECT design, not the SPEC's leaning-to-accept-the-departure default: reading `manager.go:417`'s `la := c.GetLoadAssignment()` shows the `*wrapperspb.UInt32Value` pointer (`la.GetPolicy().GetOverprovisioningFactor()`) is ALREADY in scope at the call site BEFORE any `.GetValue()` call — distinguishing "absent" (nil pointer) from "explicit `{value: 0}`" (non-nil pointer, `GetValue()==0`) costs exactly one `!= nil` check, which is free. This PLAN threads `(opf uint32, hasOPF bool)` into `newLocalityWeightedLB` (Task 3) instead of the SPEC's single bare `uint32` parameter: `hasOPF=false` → defaults to 140 (the confirmed reference default, AMEND-LW3); `hasOPF=true, opf=0` → HONORED LITERALLY as 0, which (per the confirmed formula) makes `min(1, 0×frac)==0` for EVERY locality regardless of health — i.e., an explicit `overprovisioning_factor: 0` degrades the ENTIRE mechanism to the flat fallback UNCONDITIONALLY (the AMEND-LW2 "zero total effective weight" unification path fires on every `Pick`, not just during degradation). Task 6 adds `TestPick_ExplicitZeroOPF_AlwaysFallsBackToFlat` proving this distinct-from-140 outcome. One sentence why: the correct behavior was free to implement once the actual call site was read, so there is no reason to accept the documented-departure fallback SPEC left as its default recommendation.

### Decomposition note (10 tasks vs the SPEC's indicative ~10-11)

SPEC §10 lists 10 indicative spine entries. This PLAN maps 1:1 EXCEPT: (a) SPEC's Task 3 (`ClusterLoadAssignment.Policy.overprovisioning_factor` capture) is FOLDED into this PLAN's Task 5 — reading the actual `manager.go` code (Task 1) showed the capture is a single `la.GetPolicy().GetOverprovisioningFactor()` read that only has meaning AT the wrap site (it is threaded straight into `newLocalityWeightedLB`, never used standalone), so a dedicated task would have had no independent deliverable to TDD against; and (b) SPEC's Task 4 (`locality.go`'s full construct) is SPLIT into this PLAN's Tasks 3 (construction/grouping) + 4 (the real `Pick`) — exactly the split SPEC §10's own Task 4 row anticipated ("likely split into 2 tasks if the code is large").

| SPEC §10 task | This plan |
|---|---|
| 1 baselines/anchors gate | Task 1 |
| 2 `Endpoint.Locality`/`LocalityWeight` + `extractEndpoints` capture + construction-site sweep | Task 2 |
| 3 `overprovisioning_factor` capture | **FOLDED into Task 5** (the wrap site is its only consumer) |
| 4 `locality.go`: types + `newLocalityWeightedLB` + `Pick` | **SPLIT into Task 3** (construction/grouping) **+ Task 4** (`Pick`) |
| 5 `manager.go` wrap-after-switch: 2 rejects + wrap + health-widening | Task 5 (also carries the folded Task 3's `overprovisioning_factor` read) |
| 6 unit tests: formula curve, OPF A/B, AMEND-LW2, panic, both rejects | Task 6 (the reject-arm tests themselves are TDD'd inline at Task 5; Task 6 adds the remaining numeric/behavioral depth SPEC's Task 6 calls for) |
| 7 `0095` driver: topology + health-check-toggle harness | Task 7 |
| 8 `0095` assertions: arm (a)/(b) bands + cross-side stats | Task 8 |
| 9 `0095` deliberate breaks + flake-soak + `-race` | Task 9 |
| 10 BEHAVIOR_CONTRACT + ADR-0269 + six-gate + ROADMAP note | Task 10 (STATE/ROADMAP itself deferred to the controller — see Task 10's note) |

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `internal/cluster/locality.go` | **CREATE** (Tasks 3, 4) | `LocalityID` + `localityGroup` + `localityWeightedLB` + `newLocalityWeightedLB`/`newLocalityWeightedLBWithRNG` (Task 3: construction/grouping + a placeholder `Pick`) + the real `Pick` (weighted draw + panic bypass + zero-total fallback) + `effectiveWeight` (Task 4) + `var _ loadBalancer = (*localityWeightedLB)(nil)`. |
| `internal/cluster/locality_test.go` | **CREATE** (Tasks 3, 4, 6) | Construction/grouping tests + D-LW-DUP (Task 3); Pick structural tests via stub factories + deterministic RNG (Task 4); the confirmed effective-weight curve + OPF A/B + D-LW-OPF0 + AMEND-LW2 outcomes + the child-local-panic coverage-boundary test (Task 6). |
| `internal/cluster/cluster.go` | MODIFY (Task 2) | `Endpoint` gains `Locality LocalityID` + `LocalityWeight uint32`; `Addr()` UNCHANGED. |
| `internal/cluster/manager.go` | MODIFY (Tasks 2, 5) | `extractEndpoints`'s per-group `Locality`/`LoadBalancingWeight` capture (Task 2); the `buildCluster` wrap-after-switch: hoisted `sc`, the two reject arms, the `locality_weighted_lb_config` wrap incl. the health-registry-widening + the `overprovisioning_factor` wrapper-presence read (Task 5). |
| `internal/cluster/manager_test.go` | MODIFY (Tasks 2, 5) | `extractEndpoints` capture tests + the `mkStaticClusterFromGroups` helper (Task 2); the accept/reject matrix + the health-widening test (Task 5). |
| `test/fixtures/0095-lb-locality-weighted/driver/driver.go` | **CREATE** (Tasks 7, 8) | The 2-locality×5-host topology, the driver-owned `toggleResponder` health-degradation harness, the bootstrap builders, `AssertStats` (both arms in-band). |
| `test/fixtures/0095-lb-locality-weighted/driver/driver_test.go` | **CREATE** (Task 7) | `classifyBody` parse tests + the workload-constant pin test. |
| `test/fixtures/0095-lb-locality-weighted/README.md` + `expectations.yaml` | **CREATE** (Task 7); MODIFY (Task 9) | The fixture design doc + differential expectations; Task 9 appends the break-protocol record. |
| `docs/envoy-go/phases/52-load-balancer-locality-weighted/PROGRESS.md` | **CREATE** (Task 1) | The IMPL progress ledger. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY (Task 10) | The new `### Load balancer — locality-weighted (locality_weighted_lb_config)` section. |
| `docs/envoy-go/DECISIONS.md` | MODIFY (Task 10) | The full ADR-0269 entry (§Context + §Decision + §Consequences). |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | MODIFY (Task 10, by the CONTROLLER at stage-close, not by a Task-10 subagent — see Task 10's note) | Active-phase + counts advance; ROADMAP row 52 `in-progress → done`. |

---

## Task 1: Baselines/anchors gate + PROGRESS.md

**Goal:** Re-confirm every count anchor against the IMPL-session tip BEFORE touching code, re-pin the as-built line anchors, confirm the `Endpoint{}` construction-site sweep needs ZERO edits to existing sites, and create the progress ledger. No production code.

**Files:**
- Create: `docs/envoy-go/phases/52-load-balancer-locality-weighted/PROGRESS.md`

- [ ] **Step 1: Confirm the count anchors via the canonical recipes**

```bash
ls -d test/fixtures/[0-9]* | wc -l                                    # expect 96
ls -d test/fixtures/[0-9]* | tail -1                                  # expect test/fixtures/0094-stats-sink-dogstatsd-batching
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1     # expect H2GoawayResponder BackendKind = 38
grep -rh "^func Fuzz" --include='*.go' . | wc -l                       # expect 52 (canonical recipe; reference_fuzzer_count_docs_drift)
grep "^## ADR-0" docs/envoy-go/DECISIONS.md | tail -1                  # expect tail ADR-0268, next-free ADR-0269
grep -n "1200" docs/envoy-go/BEHAVIOR_CONTRACT.md | tail -3            # the stat-surface DOC count, not a golden test
go build ./... && echo BUILD_OK
go mod tidy -diff && echo TIDY_EMPTY                                   # expect exit 0, empty (ZERO new dep)
```
Expected: fixtures **96** (tail `0094-stats-sink-dogstatsd-batching`); BackendKind tail **38**; fuzzers **52**; DECISIONS tail **ADR-0268**; stat surface **1200**; build clean; `go mod tidy -diff` empty.

- [ ] **Step 2: Re-pin the as-built anchors against the IMPL-session tip**

```bash
grep -n "type Endpoint struct" internal/cluster/cluster.go
grep -n "func (e Endpoint) Addr" internal/cluster/cluster.go
grep -n "la := c.GetLoadAssignment()\|var health \*clusterHealth\|if sc := c.GetLbSubsetConfig" internal/cluster/manager.go
grep -n "func buildLeafLB\|func extractEndpoints\|Host: sa.GetAddress()" internal/cluster/manager.go
grep -n "func registerClusterMetrics" internal/cluster/manager.go
grep -n "func (s \*subsetLB) Pick\|type leafFactory" internal/cluster/subset.go
grep -n "type loadBalancer interface\|func (rr \*roundRobin) Pick" internal/cluster/loadbalancer.go
grep -n "func newPCGRNG\|func newLeastRequest\b\|func newLeastRequestWithRNG" internal/cluster/leastrequest.go
grep -n "func newRandom\b\|func newRandomWithRNG" internal/cluster/random.go
grep -n "func newHostHealth\|func newClusterHealth\|func (ch \*clusterHealth) availableCount\|func (ch \*clusterHealth) inPanic\|func (ch \*clusterHealth) panicInc\|func parsePanicThreshold" internal/cluster/health.go
test -f internal/cluster/locality.go && echo "WARN locality.go exists" || echo "locality.go ABSENT (expected — Task 3 creates it)"
```
Record the actual line numbers in PROGRESS.md (they should match this PLAN's citations exactly, since this PLAN was authored same-day against this HEAD; a drift here means re-verify every citation in this PLAN before proceeding).

- [ ] **Step 3: Confirm the `Endpoint{}` construction-site sweep needs ZERO edits**

```bash
grep -rn "Endpoint{" internal/cluster/*.go | grep -v _test.go
grep -rn "Endpoint{" internal/cluster/*_test.go
grep -rn "cluster\.Endpoint{" --include=*.go internal/filter/ | sort
```
Expected: every hit is EITHER a bare `Endpoint{}` (zero-value error-path literal — unaffected by new fields) OR a KEYED literal (`Endpoint{Host: ..., Port: ...}` / `Endpoint{Host: ..., Port: ..., Metadata: ...}`) — NEVER a positional literal. The single PRODUCTION construction site with real field values is `internal/cluster/manager.go`'s `extractEndpoints` (`Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars}`). **Record in PROGRESS.md:** because every existing site is keyed, adding `Locality`/`LocalityWeight` to the `Endpoint` struct requires editing EXACTLY ONE call site (`extractEndpoints`, Task 2) — every other site (leaf-policy error returns, test builders across `cluster_test.go`/`maglev_test.go`/`health_test.go`/`loadbalancer_test.go`/`subset_test.go`/`outlier_test.go`/`leastrequest_test.go`/`h2pool_test.go`/`h2pool_coalesce_test.go`/`dial_h2_test.go`/`manager_test.go`, plus every `cluster.Endpoint{}` site in `internal/filter/hcm/`/`internal/filter/http/router/`) compiles unchanged with the two new fields defaulting to their zero values (`LocalityID{}`, `0`). This is a VERIFIED finding, not an assumption inherited from the SPEC's generic caution.

- [ ] **Step 4: Create PROGRESS.md**

Create `docs/envoy-go/phases/52-load-balancer-locality-weighted/PROGRESS.md` with: the 10-task table (status column); the count anchors from Step 1; the as-built line anchors from Step 2; the zero-edit-sites finding from Step 3; the D-LW-DUP/D-LW-HEALTHALLOC/D-LW-OPF0 resolutions (copied from this PLAN's D-question section); the ADR-0045 re-check verdict (NO split — 10 tasks, ~230-330 prod LoC, see the FINAL re-check at the end of this PLAN).

- [ ] **Step 5: Commit (LOCAL-ONLY)**

```bash
git add docs/envoy-go/phases/52-load-balancer-locality-weighted/PROGRESS.md
git commit -m "phase 52 Task 1: baselines gate + PROGRESS.md (fixtures 96 / fuzzers 52 / stat surface 1200 / BackendKind 38 / DECISIONS tail ADR-0268 confirmed; Endpoint{} construction-site sweep needs ZERO edits — every site is a keyed literal; go mod tidy -diff empty)"
```

---

## Task 2: The `Endpoint.Locality`/`Endpoint.LocalityWeight` dimension + the `extractEndpoints` per-group capture

**Goal:** `Endpoint` grows `Locality LocalityID` + `LocalityWeight uint32` (the SECOND per-endpoint dimension after phase 38's `Metadata`); `extractEndpoints` captures `group.GetLocality()` (Region/Zone/SubZone) + `group.GetLoadBalancingWeight().GetValue()` ONCE per `LocalityLbEndpoints` group and stamps every endpoint in that group with the same pair. `Addr()` stays UNCHANGED. `LocalityID` itself is DEFINED here (a tiny proto-free value type with no behavior — `locality.go`, created NEXT in Task 3, owns the LB construct that CONSUMES it; `LocalityID` is placed in `cluster.go` alongside `Endpoint` since it is fundamentally an `Endpoint` dimension, mirroring where `SubsetValue` was NOT placed in `cluster.go` — a deliberate difference: `SubsetValue` has real behavior (`valueEqual`) living in `subset.go` with its consumer, while `LocalityID` is a bare comparable struct with zero methods, cheap to keep next to `Endpoint`).

**Files:**
- Modify: `internal/cluster/cluster.go` (the `Endpoint` struct)
- Modify: `internal/cluster/manager.go` (`extractEndpoints`)
- Modify: `internal/cluster/manager_test.go` (new tests + the `mkStaticClusterFromGroups` helper)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
// mkStaticClusterFromGroups builds a static cluster from pre-built
// *endpointv3.LocalityLbEndpoints groups (unlike mkStaticCluster, which wraps
// every LbEndpoint in a SINGLE group) — needed for locality/weight capture
// tests, which require MULTIPLE distinct groups.
func mkStaticClusterFromGroups(name string, groups ...*endpointv3.LocalityLbEndpoints) *clusterv3.Cluster {
	return &clusterv3.Cluster{
		Name:                 name,
		ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_STATIC},
		LbPolicy:             clusterv3.Cluster_ROUND_ROBIN,
		LoadAssignment: &endpointv3.ClusterLoadAssignment{
			ClusterName: name,
			Endpoints:   groups,
		},
	}
}

func TestExtractEndpoints_CapturesLocalityAndWeightPerGroup(t *testing.T) {
	c := mkStaticClusterFromGroups("c_lw",
		&endpointv3.LocalityLbEndpoints{
			Locality:            &corev3.Locality{Region: "a", Zone: "z1"},
			LoadBalancingWeight: wrapperspb.UInt32(2),
			LbEndpoints:         []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002)},
		},
		&endpointv3.LocalityLbEndpoints{
			Locality:    &corev3.Locality{Region: "b"},
			LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9003)}, // load_balancing_weight OMITTED
		},
	)
	eps, err := extractEndpoints(c.GetLoadAssignment(), "c_lw")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 3 {
		t.Fatalf("got %d endpoints, want 3", len(eps))
	}
	for _, ep := range eps[:2] {
		want := LocalityID{Region: "a", Zone: "z1"}
		if ep.Locality != want {
			t.Errorf("ep %+v: Locality = %+v, want %+v", ep, ep.Locality, want)
		}
		if ep.LocalityWeight != 2 {
			t.Errorf("ep %+v: LocalityWeight = %d, want 2", ep, ep.LocalityWeight)
		}
	}
	if want := (LocalityID{Region: "b"}); eps[2].Locality != want {
		t.Errorf("eps[2].Locality = %+v, want %+v", eps[2].Locality, want)
	}
	if eps[2].LocalityWeight != 0 {
		t.Errorf("omitted load_balancing_weight must capture as 0 (AMEND-LW2, no default-to-1): got %d", eps[2].LocalityWeight)
	}
}

func TestExtractEndpoints_NoLocalityIsZeroValue(t *testing.T) {
	c := mkStaticCluster("c_plain", mkLbEndpoint("127.0.0.1", 9001))
	eps, err := extractEndpoints(c.GetLoadAssignment(), "c_plain")
	if err != nil {
		t.Fatal(err)
	}
	if eps[0].Locality != (LocalityID{}) {
		t.Errorf("no locality set → LocalityID{} zero value, got %+v", eps[0].Locality)
	}
	if eps[0].LocalityWeight != 0 {
		t.Errorf("no locality set → LocalityWeight 0, got %d", eps[0].LocalityWeight)
	}
}

func TestEndpoint_AddrIgnoresLocality(t *testing.T) {
	a := Endpoint{Host: "127.0.0.1", Port: 9001}
	b := Endpoint{Host: "127.0.0.1", Port: 9001, Locality: LocalityID{Region: "a"}, LocalityWeight: 5}
	if a.Addr() != b.Addr() {
		t.Errorf("Addr() must ignore Locality/LocalityWeight: %q vs %q", a.Addr(), b.Addr())
	}
}
```
`manager_test.go` already imports `corev3`, `endpointv3`, and `wrapperspb` (confirmed at Task 1 — no new import needed).

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestExtractEndpoints_CapturesLocalityAndWeightPerGroup|TestExtractEndpoints_NoLocalityIsZeroValue|TestEndpoint_AddrIgnoresLocality' ./... 2>&1 | head -20
```
Expected: COMPILE FAILURE (`LocalityID` / `Endpoint.Locality` / `Endpoint.LocalityWeight` / `mkStaticClusterFromGroups` undefined).

- [ ] **Step 3: Add `LocalityID` + the `Endpoint` fields** (`cluster.go`)

Replace the `Endpoint` struct (currently at `cluster.go:33-40`):

```go
// LocalityID is the proto-free (region, zone, sub_zone) identity captured from
// the owning LocalityLbEndpoints.locality (phase 52). The zero value is a
// valid single group — an endpoint whose group carried no locality at all.
// Comparable (usable as a map key — newLocalityWeightedLB groups endpoints by
// this identity, locality.go, Task 3).
type LocalityID struct {
	Region, Zone, SubZone string
}

// Endpoint is a single upstream socket destination.
type Endpoint struct {
	Host string
	Port uint32
	// Metadata is the parsed envoy.lb scalar key→value namespace (the subset
	// dimension, phase 38). nil when absent. NOT part of the dial identity:
	// Addr() ignores it, so ring_hash/maglev table keys stay "IP:PORT".
	Metadata map[string]SubsetValue
	// Locality is the (region, zone, sub_zone) identity captured from the
	// owning LocalityLbEndpoints group (phase 52; the SECOND per-endpoint
	// dimension after Metadata). Only consulted by localityWeightedLB — every
	// other LB construct ignores it. NOT part of the dial identity.
	Locality LocalityID
	// LocalityWeight is the RAW load_balancing_weight captured from the owning
	// group (0 when the field was unset on that group — AMEND-LW2, NO
	// default-to-1 substitution: the reference assigns an omitted-weight
	// locality ZERO load whenever a sibling locality has an explicit weight).
	// Only consulted by localityWeightedLB.
	LocalityWeight uint32
}
```

- [ ] **Step 4: Extend `extractEndpoints`** (`manager.go:771-795`)

Replace the function body:

```go
func extractEndpoints(la *endpointv3.ClusterLoadAssignment, clusterName string) ([]Endpoint, error) {
	var out []Endpoint
	for gi, group := range la.GetEndpoints() {
		l := group.GetLocality() // nil-safe: (*corev3.Locality)(nil).GetRegion() == ""
		loc := LocalityID{Region: l.GetRegion(), Zone: l.GetZone(), SubZone: l.GetSubZone()}
		weight := group.GetLoadBalancingWeight().GetValue() // 0 when unset — AMEND-LW2, no default
		for ei, lbe := range group.GetLbEndpoints() {
			ep := lbe.GetEndpoint()
			if ep == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d].endpoint is nil", clusterName, gi, ei)
			}
			addr := ep.GetAddress()
			if addr == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d].endpoint.address is nil", clusterName, gi, ei)
			}
			sa := addr.GetSocketAddress()
			if sa == nil {
				return nil, fmt.Errorf("cluster: %q: endpoints[%d].lb_endpoints[%d]: only socket_address endpoints supported", clusterName, gi, ei)
			}
			scalars, _ := ScalarsFromStruct(lbe.GetMetadata().GetFilterMetadata()["envoy.lb"]) // drop non-scalar keys
			out = append(out, Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars, Locality: loc, LocalityWeight: weight})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("cluster: %q: zero endpoints across all locality groups", clusterName)
	}
	return out, nil
}
```

- [ ] **Step 5: Run to verify they pass**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20
```
Expected: PASS, including the full pre-existing suite (the two new fields default to zero values everywhere else — Task 1 Step 3's finding confirmed live).

- [ ] **Step 6: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/cluster.go internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 52 Task 2: Endpoint.Locality/LocalityWeight + extractEndpoints per-group capture (SPEC §4) — the SECOND per-endpoint dimension after Metadata; omitted load_balancing_weight captures as 0 (AMEND-LW2, no default-to-1); ZERO existing Endpoint{} construction sites needed edits (all keyed literals, Task 1 Step 3)"
```

---

## Task 3: `internal/cluster/locality.go` — `localityGroup`/`localityWeightedLB` construction + grouping (TDD, part A)

**Goal:** Create `internal/cluster/locality.go` with the `localityGroup` type, the `localityWeightedLB` struct, `newLocalityWeightedLBWithRNG` (the injectable constructor — groups endpoints by `Locality`, builds one child per group + one flat fallback, both via the caller-bound `leafFactory`) and `newLocalityWeightedLB` (the crypto-seeded production constructor). A PLACEHOLDER `Pick` (delegates unconditionally to `flat`) satisfies the `loadBalancer` interface so this task's tests can run against a real `*localityWeightedLB`; Task 4 replaces it with the real weighted draw.

**Files:**
- Create: `internal/cluster/locality.go`
- Create: `internal/cluster/locality_test.go`

- [ ] **Step 1: Write the failing tests** (`locality_test.go`)

```go
package cluster

import "testing"

// trackingFactory returns a leafFactory that builds ONE stubLB per distinct
// call, recording each call's endpoint sub-slice (by pointer-stable Port sum,
// a cheap fingerprint) so tests can assert what newLocalityWeightedLB built.
type factoryCall struct {
	n   int
	sum uint32 // sum of Port across the sub-slice — a cheap "which slice" fingerprint
}

func trackingFactory() (leafFactory, *[]factoryCall) {
	var calls []factoryCall
	f := func(sub []Endpoint) (loadBalancer, error) {
		var sum uint32
		for _, ep := range sub {
			sum += ep.Port
		}
		calls = append(calls, factoryCall{n: len(sub), sum: sum})
		return &stubLB{ep: Endpoint{Host: "child", Port: sum}}, nil
	}
	return f, &calls
}

func TestNewLocalityWeightedLB_GroupsByLocality(t *testing.T) {
	epsA := []Endpoint{
		{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 2},
		{Host: "a1", Port: 2, Locality: LocalityID{Region: "a"}, LocalityWeight: 2},
	}
	epsB := []Endpoint{{Host: "b0", Port: 3, Locality: LocalityID{Region: "b"}, LocalityWeight: 1}}
	all := append(append([]Endpoint{}, epsA...), epsB...)
	factory, calls := trackingFactory()
	lw, err := newLocalityWeightedLBWithRNG(all, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(lw.groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(lw.groups))
	}
	byRegion := map[string]localityGroup{}
	for _, g := range lw.groups {
		byRegion[g.id.Region] = g
	}
	if g, ok := byRegion["a"]; !ok || len(g.endpoints) != 2 || g.weight != 2 {
		t.Errorf("region a group = %+v, want 2 endpoints weight 2", g)
	}
	if g, ok := byRegion["b"]; !ok || len(g.endpoints) != 1 || g.weight != 1 {
		t.Errorf("region b group = %+v, want 1 endpoint weight 1", g)
	}
	// factory called 3 times: region a (2 eps), region b (1 ep), flat (3 eps).
	if len(*calls) != 3 {
		t.Fatalf("factory calls = %d, want 3 (2 groups + 1 flat)", len(*calls))
	}
	var sawFlat bool
	for _, c := range *calls {
		if c.n == 3 {
			sawFlat = true
		}
	}
	if !sawFlat {
		t.Errorf("no factory call spanned all 3 endpoints (the flat fallback) — calls: %+v", *calls)
	}
}

func TestNewLocalityWeightedLB_DuplicateLocality_LastWriteWins(t *testing.T) {
	// D-LW-DUP: two endpoints sharing an identical LocalityID but DIFFERENT
	// LocalityWeight (simulating two source LocalityLbEndpoints groups that
	// collapsed to the same identity) — the LAST-encountered weight wins, and
	// BOTH endpoints merge into the SAME group (grouping is by identity, not
	// by source-group index).
	eps := []Endpoint{
		{Host: "h1", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 5},
		{Host: "h2", Port: 2, Locality: LocalityID{Region: "a"}, LocalityWeight: 9},
	}
	factory, _ := trackingFactory()
	lw, err := newLocalityWeightedLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(lw.groups) != 1 {
		t.Fatalf("groups = %d, want 1 (both endpoints share LocalityID{Region:a})", len(lw.groups))
	}
	if lw.groups[0].weight != 9 {
		t.Errorf("weight = %d, want 9 (last-write-wins)", lw.groups[0].weight)
	}
	if len(lw.groups[0].endpoints) != 2 {
		t.Errorf("endpoints = %d, want 2 (both merge into the same group)", len(lw.groups[0].endpoints))
	}
}

func TestNewLocalityWeightedLB_FactoryErrorPropagates(t *testing.T) {
	eps := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}}}
	wantErr := errNoEndpoints
	factory := func(sub []Endpoint) (loadBalancer, error) { return nil, wantErr }
	if _, err := newLocalityWeightedLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 }); err != wantErr {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestNewLocalityWeightedLB_OverprovisioningFactor_DefaultsOnAbsent(t *testing.T) {
	factory, _ := trackingFactory()
	lw, err := newLocalityWeightedLBWithRNG([]Endpoint{{Host: "a", Port: 1}}, nil, 0, false /* hasOPF */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if lw.overprovisioningFactor != defaultOverprovisioningFactor {
		t.Errorf("overprovisioningFactor = %d, want %d (absent → default)", lw.overprovisioningFactor, defaultOverprovisioningFactor)
	}
}

func TestNewLocalityWeightedLB_OverprovisioningFactor_HonorsExplicitZero(t *testing.T) {
	// D-LW-OPF0: an EXPLICIT {value: 0} is honored literally, NOT defaulted.
	factory, _ := trackingFactory()
	lw, err := newLocalityWeightedLBWithRNG([]Endpoint{{Host: "a", Port: 1}}, nil, 0, true /* hasOPF */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if lw.overprovisioningFactor != 0 {
		t.Errorf("overprovisioningFactor = %d, want 0 (explicit zero honored, not defaulted)", lw.overprovisioningFactor)
	}
}
```
`stubLB` is already defined in `cluster_test.go` (same package; reused verbatim — Task 1's anchor confirms `cluster_test.go:447-456`).

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestNewLocalityWeightedLB' ./... 2>&1 | head -20
```
Expected: COMPILE FAILURE (`localityGroup`/`newLocalityWeightedLBWithRNG`/`defaultOverprovisioningFactor` undefined).

- [ ] **Step 3: Implement `locality.go`**

```go
// Package cluster: internal/cluster/locality.go implements the
// locality-weighted load-balancing wrapper (phase 52, ADR-0269).
//
// localityWeightedLB partitions a cluster's endpoints by Locality identity
// (LocalityLbEndpoints.locality) and weighted-random-draws a locality per
// Pick, scaled by that locality's live healthy fraction (the confirmed
// AMEND-LW3 formula), delegating to a per-locality child loadBalancer built
// by the EXISTING policy factory (buildLeafLB) — the phase-38 subsetLB
// structural precedent (subset.go), generalized from a route-resolved
// metadata match to a build-time-fixed locality identity. UNLIKE subsetLB,
// this wrapper needs ZERO new Pick parameters (D-LW7): locality selection
// resolves entirely from per-cluster build-time state plus the EXISTING
// clusterHealth, consulted via a new per-locality aggregation read
// (availableCount called with a locality's OWN endpoint sub-slice instead of
// the whole cluster's).
package cluster

// LocalityID is defined in cluster.go (the Endpoint dimension it stamps).

// localityGroup is one distinct locality's endpoint sub-slice + its RAW
// configured weight (0 when load_balancing_weight was unset — AMEND-LW2; NO
// default-to-1 substitution) + its factory-built child loadBalancer.
type localityGroup struct {
	id        LocalityID
	endpoints []Endpoint // this locality's own sub-slice, for per-locality health aggregation
	weight    uint32     // the RAW captured weight; 0 is a valid, meaningful "no load" value
	child     loadBalancer
}

// defaultOverprovisioningFactor is the reference's own default (AMEND-LW3),
// applied when ClusterLoadAssignment.Policy.overprovisioning_factor is
// ABSENT (a nil *wrapperspb.UInt32Value) — NOT when explicitly present with
// value 0 (D-LW-OPF0: manager.go's buildCluster checks the wrapper's
// presence BEFORE calling .GetValue(), a free distinction, and threads
// (opf uint32, hasOPF bool) into newLocalityWeightedLB accordingly).
const defaultOverprovisioningFactor = 140

// localityWeightedLB is a per-cluster WRAPPER load balancer (ADR-0269). Built
// ONCE at cluster construction (locality membership + configured weights
// never change post-boot — no dynamic EDS project-wide); Pick recomputes the
// per-locality EFFECTIVE weight (health-derived) on every call, since health
// state changes live (ADR-0243 precedent — every leaf policy already
// recomputes its own health view per Pick).
//
// REUSES the loadBalancer interface UNCHANGED — ZERO new Pick parameters
// (D-LW7): hashKey/match pass straight through to the chosen locality's
// child, exactly as subsetLB already forwards to ITS children
// (subset.go:213-230).
type localityWeightedLB struct {
	groups                 []localityGroup
	flat                   loadBalancer   // panic-mode + zero-total-weight fallback; spans ALL endpoints
	allEndpoints           []Endpoint
	health                 *clusterHealth // never nil in practice once this wrap fires (manager.go's health-registry-widening); nil-guarded defensively
	overprovisioningFactor uint32         // AMEND-LW3; see defaultOverprovisioningFactor's doc for the absent-vs-explicit-zero distinction
	rng                    func() uint64  // the newPCGRNG (leastrequest.go:61-81) idiom — injectable for deterministic tests
}

var _ loadBalancer = (*localityWeightedLB)(nil)

// newLocalityWeightedLB is the production constructor: seeds a crypto-keyed
// PCG (newPCGRNG, leastrequest.go:61-81 — the random.go/leastrequest.go
// crypto-constructor/injectable-constructor split) then delegates to
// newLocalityWeightedLBWithRNG.
func newLocalityWeightedLB(endpoints []Endpoint, health *clusterHealth, opf uint32, hasOPF bool, factory leafFactory) (*localityWeightedLB, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newLocalityWeightedLBWithRNG(endpoints, health, opf, hasOPF, factory, rng)
}

// newLocalityWeightedLBWithRNG is the injectable constructor used by unit
// tests to supply a deterministic draw sequence (the newRandomWithRNG /
// newLeastRequestWithRNG precedent). It groups endpoints by Locality identity
// (encounter order), builds one child per locality + one flat fallback child
// (both via the caller-bound factory — the buildLeafLB closure, the subsetLB
// precedent), and resolves the overprovisioning_factor absent/explicit-zero
// distinction (D-LW-OPF0). A duplicate locality identity across multiple
// source LocalityLbEndpoints groups resolves last-write-wins on weight, and
// MERGES the duplicate's endpoints into the SAME group (D-LW-DUP — grouping
// is by identity, not source-group index).
func newLocalityWeightedLBWithRNG(endpoints []Endpoint, health *clusterHealth, opf uint32, hasOPF bool, factory leafFactory, rng func() uint64) (*localityWeightedLB, error) {
	overprovisioningFactor := opf
	if !hasOPF {
		overprovisioningFactor = defaultOverprovisioningFactor
	}
	byLocality := map[LocalityID][]Endpoint{}
	weights := map[LocalityID]uint32{}
	var order []LocalityID
	for _, ep := range endpoints {
		if _, seen := byLocality[ep.Locality]; !seen {
			order = append(order, ep.Locality)
		}
		byLocality[ep.Locality] = append(byLocality[ep.Locality], ep)
		weights[ep.Locality] = ep.LocalityWeight // last-write-wins (D-LW-DUP)
	}
	lw := &localityWeightedLB{allEndpoints: endpoints, health: health, overprovisioningFactor: overprovisioningFactor, rng: rng}
	for _, id := range order {
		members := byLocality[id]
		child, err := factory(members)
		if err != nil {
			return nil, err
		}
		lw.groups = append(lw.groups, localityGroup{id: id, endpoints: members, weight: weights[id], child: child})
	}
	flat, err := factory(endpoints)
	if err != nil {
		return nil, err
	}
	lw.flat = flat
	return lw, nil
}

// Pick is completed in Task 4 (the confirmed AMEND-LW3 weighted draw + the
// AMEND-LW4 panic bypass + the AMEND-LW2 zero-total fallback). This
// placeholder unconditionally delegates to the flat fallback so the type
// satisfies loadBalancer and Task 3's construction/grouping tests can run
// against a real *localityWeightedLB.
func (lw *localityWeightedLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) {
	return lw.flat.Pick(hashKey, hasHash, match, hasMatch)
}
```

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run 'TestNewLocalityWeightedLB' ./... -v 2>&1 | tail -30
go build ./... && echo BUILD_OK
```
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/locality.go internal/cluster/locality_test.go
git commit -m "phase 52 Task 3: locality.go — LocalityID grouping + localityWeightedLB construction (newLocalityWeightedLB/-WithRNG), the subsetLB structural precedent generalized; D-LW-DUP last-write-wins + D-LW-OPF0 absent-vs-explicit-zero resolved at the constructor; a placeholder flat-only Pick (Task 4 completes it)"
```

---

## Task 4: `localityWeightedLB.Pick` — the confirmed weighted draw + panic bypass + zero-total fallback (TDD, part B)

**Goal:** Replace Task 3's placeholder `Pick` with the real implementation: cluster-wide panic (AMEND-LW4) bypasses locality weighting entirely; otherwise compute each locality's EFFECTIVE weight via the confirmed AMEND-LW3 formula (extracted as a pure, directly-unit-testable `effectiveWeight` helper), weighted-random-draw a locality (a float64 cumulative-bucket walk — the `router_weighted.go` integer-weight idiom generalized to continuous weights, since effective weight is health-fraction-scaled and therefore NOT an exact integer), and delegate. A zero TOTAL effective weight falls back to the flat child (unifying BOTH AMEND-LW2 outcomes with the SAME fallback path).

**Files:**
- Modify: `internal/cluster/locality.go` (replace the placeholder `Pick`; add `effectiveWeight`)
- Modify: `internal/cluster/locality_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestPick_HealthyLocality_DelegatesToItsOwnChild(t *testing.T) {
	epsA := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 1}}
	epsB := []Endpoint{{Host: "b0", Port: 2, Locality: LocalityID{Region: "b"}, LocalityWeight: 0}} // zero weight → never drawn
	all := append(append([]Endpoint{}, epsA...), epsB...)
	stubs := map[string]*stubLB{}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		key := "flat"
		if len(sub) != len(all) {
			key = sub[0].Locality.Region
		}
		s := &stubLB{}
		stubs[key] = s
		return s, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(all, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := lw.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if stubs["a"].active.Load() != 1 {
		t.Errorf("region a (the only nonzero-weight locality) must have been picked; active = %d", stubs["a"].active.Load())
	}
	if stubs["b"].active.Load() != 0 {
		t.Errorf("region b (weight 0) must NEVER be picked; active = %d", stubs["b"].active.Load())
	}
	if stubs["flat"].active.Load() != 0 {
		t.Errorf("flat fallback must not fire when a nonzero-weight locality exists; active = %d", stubs["flat"].active.Load())
	}
}

func TestPick_PanicBypassesLocalityWeighting(t *testing.T) {
	epsA := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 2}}
	epsB := []Endpoint{{Host: "b0", Port: 2, Locality: LocalityID{Region: "b"}, LocalityWeight: 1}}
	all := append(append([]Endpoint{}, epsA...), epsB...)
	health := newClusterHealth(all, 0.5)
	health.states["a0:1"].healthy.Store(false) // 1/2 = 50%, NOT strictly < 0.5 yet...
	health.states["b0:2"].healthy.Store(false) // ...now 0/2 = 0% < 0.5 → cluster-wide panic
	stubs := map[string]*stubLB{}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		key := "flat"
		if len(sub) != len(all) {
			key = sub[0].Locality.Region
		}
		s := &stubLB{}
		stubs[key] = s
		return s, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := lw.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if stubs["flat"].active.Load() != 1 {
		t.Errorf("cluster-wide panic (0%% healthy) must delegate to flat; flat active = %d", stubs["flat"].active.Load())
	}
	if stubs["a"].active.Load() != 0 || stubs["b"].active.Load() != 0 {
		t.Errorf("panic must bypass per-locality children entirely: a=%d b=%d", stubs["a"].active.Load(), stubs["b"].active.Load())
	}
	if got := health.panicCounter; got != nil {
		t.Errorf("panicCounter is nil-guarded in this unit construction — unexpected non-nil")
	}
}

func TestPick_ZeroTotalEffectiveWeight_FallsBackToFlat(t *testing.T) {
	// AMEND-LW2: every locality's raw weight is 0 (the "all omitted" outcome).
	epsA := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 0}}
	epsB := []Endpoint{{Host: "b0", Port: 2, Locality: LocalityID{Region: "b"}, LocalityWeight: 0}}
	all := append(append([]Endpoint{}, epsA...), epsB...)
	stubs := map[string]*stubLB{}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		key := "flat"
		if len(sub) != len(all) {
			key = sub[0].Locality.Region
		}
		s := &stubLB{}
		stubs[key] = s
		return s, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(all, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, _, err := lw.Pick(0, false, SubsetMatch{}, false); err != nil {
			t.Fatal(err)
		}
	}
	if stubs["flat"].active.Load() != 10 {
		t.Errorf("all-zero-weight → EVERY Pick must fall back to flat; flat active = %d", stubs["flat"].active.Load())
	}
}

func TestPick_ForwardsHashKeyAndMatchUnchanged(t *testing.T) {
	// D-LW7 / SPEC §3.2: hashKey/match pass straight through to the chosen child.
	eps := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 1}}
	var gotHashKey uint64
	var gotHasHash bool
	var gotMatch SubsetMatch
	var gotHasMatch bool
	factory := func(sub []Endpoint) (loadBalancer, error) {
		return &recordingLB{onPick: func(hk uint64, hh bool, m SubsetMatch, hm bool) {
			gotHashKey, gotHasHash, gotMatch, gotHasMatch = hk, hh, m, hm
		}}, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	wantMatch := NewSubsetMatch(map[string]SubsetValue{"k": {Kind: subsetString, Str: "v"}})
	if _, _, err := lw.Pick(42, true, wantMatch, true); err != nil {
		t.Fatal(err)
	}
	if gotHashKey != 42 || !gotHasHash || gotMatch.Key() != wantMatch.Key() || !gotHasMatch {
		t.Errorf("forwarded (hashKey=%d, hasHash=%v, match=%v, hasMatch=%v), want (42, true, %v, true)", gotHashKey, gotHasHash, gotMatch, gotHasMatch, wantMatch)
	}
}

// recordingLB is a test-only loadBalancer that records its Pick args via a
// caller-supplied callback and returns a fixed zero Endpoint.
type recordingLB struct{ onPick func(uint64, bool, SubsetMatch, bool) }

func (r *recordingLB) Pick(hk uint64, hh bool, m SubsetMatch, hm bool) (Endpoint, func(), error) {
	if r.onPick != nil {
		r.onPick(hk, hh, m, hm)
	}
	return Endpoint{}, noopRelease, nil
}

func TestEffectiveWeight_ZeroWeightIsAlwaysZero(t *testing.T) {
	if got := effectiveWeight(0, 1.0, 140); got != 0 {
		t.Errorf("effectiveWeight(0, ...) = %v, want 0", got)
	}
}

func TestEffectiveWeight_FullHealthNoPlateau_IsRawWeight(t *testing.T) {
	// At 100% healthy with the default OPF=140, availability caps at 1.0 —
	// effective weight equals the raw configured weight exactly.
	if got := effectiveWeight(3, 1.0, 140); got != 3 {
		t.Errorf("effectiveWeight(3, 1.0, 140) = %v, want 3", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestPick_|TestEffectiveWeight_' ./... 2>&1 | head -30
```
Expected: FAIL (the placeholder `Pick` always delegates to `flat`, so `TestPick_HealthyLocality_DelegatesToItsOwnChild`/`TestPick_ForwardsHashKeyAndMatchUnchanged` fail; `effectiveWeight` is undefined → compile failure for the last two).

- [ ] **Step 3: Replace the placeholder `Pick`; add `effectiveWeight`** (`locality.go`)

First, insert an import block right after `locality.go`'s `package cluster` line (Task 3's file has none yet):

```go
package cluster

import "math"
```

Then replace the Task 3 placeholder `Pick` method with:

```go
// effectiveWeight computes locality g's AMEND-LW3 effective weight: the RAW
// configured weight scaled by min(1, (overprovisioningFactor/100) ×
// healthyFraction). Extracted as a pure function (no RNG / health-registry
// plumbing) so the confirmed 6-point degradation curve (SPEC §11.3) is
// directly unit-testable (Task 6) without a stochastic Pick-level test.
func effectiveWeight(weight uint32, healthyFraction float64, overprovisioningFactor uint32) float64 {
	availability := math.Min(1.0, (float64(overprovisioningFactor)/100.0)*healthyFraction)
	return float64(weight) * availability
}

// Pick: cluster-wide panic (AMEND-LW4) bypasses locality weighting entirely,
// delegating to the flat child — the SAME clusterHealth.inPanic gate every
// leaf policy already consults (health.go:165-171), evaluated here over
// lw.allEndpoints (the FULL cluster, not a single locality). Otherwise
// compute each locality's EFFECTIVE weight (effectiveWeight, AMEND-LW3),
// weighted-random-draw a locality (a float64 cumulative-bucket walk — the
// router_weighted.go integer-weight idiom generalized to continuous
// weights), and delegate. A zero TOTAL effective weight (every locality's
// raw weight is 0 — AMEND-LW2's "all omitted" case — or every nonzero-weight
// locality is fully unavailable) falls back to the flat child, unifying BOTH
// observed AMEND-LW2 outcomes with the SAME fallback path.
//
// hashKey/hasHash/match/hasMatch pass straight through to whichever child is
// chosen, UNCHANGED (D-LW7) — the identical forwarding shape subsetLB.Pick
// already uses (subset.go:213-230).
func (lw *localityWeightedLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) {
	if lw.health != nil && lw.health.inPanic(lw.allEndpoints) {
		lw.health.panicInc()
		return lw.flat.Pick(hashKey, hasHash, match, hasMatch)
	}
	type bucket struct {
		cum   float64
		child loadBalancer
	}
	buckets := make([]bucket, 0, len(lw.groups))
	var total float64
	for _, g := range lw.groups {
		frac := 1.0
		if lw.health != nil && len(g.endpoints) > 0 {
			frac = float64(lw.health.availableCount(g.endpoints)) / float64(len(g.endpoints))
		}
		eff := effectiveWeight(g.weight, frac, lw.overprovisioningFactor)
		if eff > 0 {
			total += eff
			buckets = append(buckets, bucket{cum: total, child: g.child})
		}
	}
	if total <= 0 {
		return lw.flat.Pick(hashKey, hasHash, match, hasMatch) // AMEND-LW2 unification
	}
	r := (float64(lw.rng()) / float64(math.MaxUint64)) * total
	for _, b := range buckets {
		if r < b.cum {
			return b.child.Pick(hashKey, hasHash, match, hasMatch)
		}
	}
	return buckets[len(buckets)-1].child.Pick(hashKey, hasHash, match, hasMatch) // float rounding guard
}
```

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run 'TestPick_|TestEffectiveWeight_|TestNewLocalityWeightedLB' ./... -v 2>&1 | tail -40
go vet ./... && go build ./... && echo OK
```
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/locality.go internal/cluster/locality_test.go
git commit -m "phase 52 Task 4: localityWeightedLB.Pick — the confirmed AMEND-LW3 effective-weight formula (extracted as effectiveWeight, unit-testable pure) + AMEND-LW4 panic bypass + AMEND-LW2 zero-total flat fallback; D-LW7 hashKey/match forwarding unchanged"
```

---

## Task 5: `manager.go` wrap-after-switch — the two reject arms + the wrap + the health-registry-widening + the `overprovisioning_factor` read

**Goal:** `buildCluster` gains a SECOND wrap-after-switch site, immediately after the EXISTING `lb_subset_config` wrap: hoist `sc := c.GetLbSubsetConfig()` once (avoiding the double getter call the SPEC's indicative sketch left open, §3.3's own note), then a `switch` covering the two new rejects + the `locality_weighted_lb_config` wrap (which allocates `clusterHealth` unconditionally when absent — the health-registry-widening — and reads `ClusterLoadAssignment.Policy.overprovisioning_factor`'s presence/value).

**Files:**
- Modify: `internal/cluster/manager.go` (`buildCluster`, replacing lines 461-467)
- Modify: `internal/cluster/manager_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestManager_Reject_ZoneAwareLbConfig(t *testing.T) {
	c := mkStaticCluster("c_za", mkLbEndpoint("127.0.0.1", 9001))
	c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{
		LocalityConfigSpecifier: &clusterv3.Cluster_CommonLbConfig_ZoneAwareLbConfig_{
			ZoneAwareLbConfig: &clusterv3.Cluster_CommonLbConfig_ZoneAwareLbConfig{},
		},
	}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "common_lb_config.zone_aware_lb_config is not supported") {
		t.Errorf("err = %v, want the zone_aware_lb_config reject", err)
	}
}

func TestManager_Reject_LocalityWeightedWithLbSubsetConfig(t *testing.T) {
	c := mkStaticCluster("c_both", mkLbEndpoint("127.0.0.1", 9001))
	c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{FallbackPolicy: clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT}
	c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{
		LocalityConfigSpecifier: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
			LocalityWeightedLbConfig: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
		},
	}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "lb_subset_config cannot be combined with common_lb_config.locality_weighted_lb_config") {
		t.Errorf("err = %v, want the combined-config reject", err)
	}
}

func TestManager_Accept_LocalityWeightedLbConfig_WrapsChild(t *testing.T) {
	c := mkStaticClusterFromGroups("c_lw",
		&endpointv3.LocalityLbEndpoints{
			Locality: &corev3.Locality{Region: "a"}, LoadBalancingWeight: wrapperspb.UInt32(2),
			LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001)},
		},
		&endpointv3.LocalityLbEndpoints{
			Locality: &corev3.Locality{Region: "b"}, LoadBalancingWeight: wrapperspb.UInt32(1),
			LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9002)},
		},
	)
	c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{
		LocalityConfigSpecifier: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
			LocalityWeightedLbConfig: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
		},
	}
	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("locality_weighted_lb_config under ROUND_ROBIN must be accepted: %v", err)
	}
	cl, _ := mgr.Get("c_lw")
	if _, ok := cl.lb.(*localityWeightedLB); !ok {
		t.Errorf("lb must be wrapped in *localityWeightedLB, got %T", cl.lb)
	}
}

func TestManager_LocalityWeighted_WidensHealthWithoutHealthChecks(t *testing.T) {
	// D-LW-HEALTHALLOC: no health_checks configured, yet cl.health must be
	// non-nil once locality_weighted_lb_config is present, and
	// registerClusterMetrics must ALSO inject membership_healthy/lb_healthy_panic
	// (the EXISTING unconditional `if c.health != nil` block, manager.go:163-169).
	c := mkStaticCluster("c_lw_nohc", mkLbEndpoint("127.0.0.1", 9001))
	c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{
		LocalityConfigSpecifier: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
			LocalityWeightedLbConfig: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
		},
	}
	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cl, _ := mgr.Get("c_lw_nohc")
	if cl.health == nil {
		t.Fatal("cl.health must be non-nil (the health-registry-widening) even with zero health_checks")
	}
	if cl.health.membershipHealthy == nil {
		t.Error("membership_healthy must be registered even though no health_checks are configured (a widened side effect of D-LW-HEALTHALLOC)")
	}
	if cl.health.panicCounter == nil {
		t.Error("lb_healthy_panic must be registered even though no health_checks are configured")
	}
	if len(cl.checkers) != 0 {
		t.Errorf("checkers = %d, want 0 (no health_checks configured — only the registry widened, not the runtime)", len(cl.checkers))
	}
}

func TestManager_Accept_LocalityWeightedLbConfig_ReadsOverprovisioningFactor(t *testing.T) {
	mk := func(opf *wrapperspb.UInt32Value) *clusterv3.Cluster {
		c := mkStaticCluster("c_opf", mkLbEndpoint("127.0.0.1", 9001))
		c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{
			LocalityConfigSpecifier: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
				LocalityWeightedLbConfig: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
			},
		}
		c.LoadAssignment.Policy = &endpointv3.ClusterLoadAssignment_Policy{OverprovisioningFactor: opf}
		return c
	}
	// absent → defaults to 140.
	mgr, err := NewManager(mkBootstrap(mk(nil)), stats.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	cl, _ := mgr.Get("c_opf")
	lw := cl.lb.(*localityWeightedLB)
	if lw.overprovisioningFactor != 140 {
		t.Errorf("absent overprovisioning_factor: got %d, want 140", lw.overprovisioningFactor)
	}
	// explicit 100 → honored.
	mgr2, err := NewManager(mkBootstrap(mk(wrapperspb.UInt32(100))), stats.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	cl2, _ := mgr2.Get("c_opf")
	if got := cl2.lb.(*localityWeightedLB).overprovisioningFactor; got != 100 {
		t.Errorf("explicit overprovisioning_factor=100: got %d, want 100", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestManager_Reject_ZoneAware|TestManager_Reject_LocalityWeightedWithLbSubsetConfig|TestManager_Accept_LocalityWeighted|TestManager_LocalityWeighted_Widens' ./... 2>&1 | head -30
```
Expected: FAIL (the reject strings never fire; `cl.lb` stays whatever `buildLeafLB` produced, never `*localityWeightedLB`).

- [ ] **Step 3: Implement the wrap-after-switch** (`manager.go`)

Replace lines 461-467 (currently: `if sc := c.GetLbSubsetConfig(); sc != nil { lb = newSubsetLB(...) }` followed by `cl.lb = lb` / `cl.health = health`) with:

```go
	// Phase 38.1 (ADR-0240): the lb_subset_config wrap. sc is HOISTED here (a
	// single c.GetLbSubsetConfig() call, not re-derived below — SPEC §3.3's
	// own note) so the phase-52 combined-config reject can check it cheaply.
	sc := c.GetLbSubsetConfig()
	if sc != nil {
		lb = newSubsetLB(endpoints, parseLbSubsetConfig(sc), func(sub []Endpoint) (loadBalancer, error) {
			return buildLeafLB(c, name, sub, health)
		})
	}
	// Phase 52 (ADR-0269): the SECOND wrap-after-switch site. lwc/zac are the
	// two arms of the SAME CommonLbConfig.LocalityConfigSpecifier oneof — they
	// can never both be non-nil, so their relative case order below is
	// immaterial; lwc-vs-sc IS a real, orthogonal combination (§6.1).
	lwc := c.GetCommonLbConfig().GetLocalityWeightedLbConfig()
	zac := c.GetCommonLbConfig().GetZoneAwareLbConfig()
	switch {
	case lwc != nil && sc != nil:
		return nil, fmt.Errorf("cluster: %q: lb_subset_config cannot be combined with common_lb_config.locality_weighted_lb_config", name)
	case zac != nil:
		return nil, fmt.Errorf("cluster: %q: common_lb_config.zone_aware_lb_config is not supported", name)
	case lwc != nil:
		// D-LW-HEALTHALLOC: a locality-weighted cluster needs
		// clusterHealth.availableCount/inPanic to be CALLABLE even with zero
		// health_checks configured (every host reports available; confirmed
		// safe against newHostHealth's healthy=true default, health.go:44-48).
		// This is a deliberate departure from the nil-health-fast-path
		// convention every OTHER LB construct follows.
		if health == nil {
			health = newClusterHealth(endpoints, parsePanicThreshold(c))
		}
		// D-LW-OPF0: the wrapper's PRESENCE (nil vs non-nil) is checked BEFORE
		// .GetValue() — a free distinction between "absent" (→ 140 default,
		// inside newLocalityWeightedLB) and "explicit {value: 0}" (honored
		// literally).
		opfWrapper := la.GetPolicy().GetOverprovisioningFactor()
		var opf uint32
		hasOPF := opfWrapper != nil
		if hasOPF {
			opf = opfWrapper.GetValue()
		}
		lw, err := newLocalityWeightedLB(endpoints, health, opf, hasOPF, func(sub []Endpoint) (loadBalancer, error) {
			return buildLeafLB(c, name, sub, health)
		})
		if err != nil {
			return nil, err
		}
		lb = lw
	}
	cl.lb = lb
	cl.health = health
```

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test ./... -v 2>&1 | grep -E "FAIL|^--- FAIL" 
go test ./... 2>&1 | tail -10
```
Expected: full package PASS (no regressions on the phase-38 subset tests, since `sc` is now hoisted but behaviorally identical).

- [ ] **Step 5: gofmt + lint + vet + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/... && go vet ./internal/cluster/...
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 52 Task 5: manager.go buildCluster wrap-after-switch (SPEC §3.3/§6) — the zone_aware_lb_config reject + the lb_subset_config-combined reject + the locality_weighted_lb_config wrap (health-registry-widening, D-LW-HEALTHALLOC; overprovisioning_factor absent-vs-explicit-zero, D-LW-OPF0); sc hoisted once above both wrap sites"
```

---

## Task 6: Unit-test depth — the confirmed formula curve, the OPF A/B, D-LW-OPF0's Pick-level consequence, AMEND-LW2 outcomes, and the child-local-panic coverage boundary

**Goal:** Add the numeric/behavioral tests SPEC §10 Task 6 calls for that Tasks 3-5's TDD steps did not already cover: the confirmed 6-point degradation curve (mirroring SPEC §11.3's exact numbers, ±0.01 tolerance), the OPF-100-vs-140 A/B, the Pick-level consequence of an explicit `overprovisioning_factor: 0` (D-LW-OPF0 — the WHOLE mechanism degrades to flat, unconditionally), and a documentation test for a structural finding discovered while writing this PLAN (a locality's own child MAY independently enter ITS OWN panic branch when that locality's local healthy fraction dips below the shared panic threshold, even while the cluster-wide fraction — which governs the WRAPPER's own panic gate — stays above it).

**Files:**
- Modify: `internal/cluster/locality_test.go`

- [ ] **Step 1: Write the tests**

```go
func TestEffectiveWeight_ConfirmedDegradationCurve(t *testing.T) {
	// SPEC §11.3 AMEND-LW3: region A weight=2, region B weight=1 (always 100%
	// healthy), OPF=140 (the reference default). Each row: A's healthy
	// fraction → A's predicted share of the 2-locality draw, matching the
	// live-probed values within the SPEC's own reported <1-percentage-point
	// tolerance.
	const weightA, weightB, opf = 2, 1, 140
	cases := []struct {
		fracA     float64
		wantShare float64
	}{
		{1.00, 0.667},
		{0.80, 0.667}, // ABOVE the 100/140=71.4% plateau threshold — no degradation
		{0.60, 0.627},
		{0.40, 0.528},
		{0.20, 0.359},
		{0.00, 0.000},
	}
	for _, c := range cases {
		effA := effectiveWeight(weightA, c.fracA, opf)
		effB := effectiveWeight(weightB, 1.0, opf)
		total := effA + effB
		var share float64
		if total > 0 {
			share = effA / total
		}
		if diff := share - c.wantShare; diff > 0.01 || diff < -0.01 {
			t.Errorf("fracA=%.2f: share = %.4f, want %.3f (±0.01)", c.fracA, share, c.wantShare)
		}
	}
}

func TestEffectiveWeight_OPF100vs140_AB(t *testing.T) {
	// SPEC §11.3: an identical 60%-healthy state, OPF=140 (default) vs
	// OPF=100 (no plateau margin), must produce CLEARLY DIFFERENT shares —
	// proving the factor is genuinely consumed, not hardcoded.
	const weightA, weightB, frac = 2, 1, 0.60
	share := func(opf uint32) float64 {
		effA := effectiveWeight(weightA, frac, opf)
		effB := effectiveWeight(weightB, 1.0, opf)
		return effA / (effA + effB)
	}
	share140, share100 := share(140), share(100)
	if diff := share140 - share100; diff < 0.05 {
		t.Errorf("share(OPF=140)=%.4f vs share(OPF=100)=%.4f: difference %.4f too small, want a CLEARLY different result (SPEC observed 62.8%% vs 54.0%%)", share140, share100, diff)
	}
	if d := share140 - 0.627; d > 0.01 || d < -0.01 {
		t.Errorf("share(OPF=140) = %.4f, want 0.627 ± 0.01", share140)
	}
	if d := share100 - 0.545; d > 0.01 || d < -0.01 {
		t.Errorf("share(OPF=100) = %.4f, want 0.545 ± 0.01", share100)
	}
}

func TestPick_ExplicitZeroOPF_AlwaysFallsBackToFlat(t *testing.T) {
	// D-LW-OPF0: an EXPLICIT overprovisioning_factor: 0 makes
	// min(1, (0/100)*frac) == 0 for EVERY locality regardless of health
	// (even at 100%-healthy, frac=1.0: 0*1.0==0) — so the ENTIRE mechanism
	// degrades to the flat fallback UNCONDITIONALLY. This is DISTINCT from
	// the hasOPF=false (defaulted-140) case, which does NOT fall back when
	// fully healthy (TestPick_HealthyLocality_DelegatesToItsOwnChild, Task 4).
	eps := []Endpoint{{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 5}}
	stubs := map[string]*stubLB{}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		key := "flat"
		if len(sub) != len(eps) {
			key = sub[0].Locality.Region
		}
		s := &stubLB{}
		stubs[key] = s
		return s, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(eps, nil /* health nil → frac defaults to 1.0 */, 0, true /* hasOPF, explicit zero */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := lw.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if stubs["flat"].active.Load() != 1 {
		t.Errorf("explicit overprovisioning_factor=0 must ALWAYS fall back to flat, even at 100%% health; flat active = %d", stubs["flat"].active.Load())
	}
	if stubs["a"].active.Load() != 0 {
		t.Errorf("region a's own child must never fire when OPF=0; active = %d", stubs["a"].active.Load())
	}
}

func TestPick_ChildLocalityCanLocallyPanicIndependently(t *testing.T) {
	// DOCUMENTATION test (not a phase-52 requirement to fix): a locality's
	// child is built by the UNCHANGED buildLeafLB/roundRobin, which
	// evaluates its OWN clusterHealth.inPanic over ITS OWN endpoint
	// sub-slice (health.go's roundRobin.Pick, loadbalancer.go:44-66) — a
	// DIFFERENT scope than the wrapper's cluster-wide inPanic check
	// (Pick, Task 4). A locality's local healthy fraction can dip below the
	// SHARED panicThreshold (0.5 default) while the CLUSTER-WIDE fraction
	// stays above it, so the wrapper itself never enters panic, yet the
	// CHOSEN locality's own child independently sprays across its own
	// full sub-slice (including unhealthy hosts) once delegated to. This is
	// a structural consequence of reusing buildLeafLB's children UNCHANGED
	// (the identical posture subsetLB's children already have) and is
	// UNTESTED by the 0095 differential (its synthetic backends respond 200
	// on data paths regardless of health-check status, so this local-panic
	// behavior never perturbs the REGION-level share assertions). Recorded
	// here as a coverage boundary, found reading the real roundRobin.Pick
	// code at this PLAN's Task 6 review — not a bug to fix in phase 52
	// (fixing it would require passing a per-locality "panic disabled" view
	// into buildLeafLB's children, contradicting the ZERO-new-Pick-parameter,
	// buildLeafLB-reused-unchanged design, D-LW7).
	region := []Endpoint{
		{Host: "a0", Port: 1, Locality: LocalityID{Region: "a"}, LocalityWeight: 1},
		{Host: "a1", Port: 2, Locality: LocalityID{Region: "a"}, LocalityWeight: 1},
		{Host: "a2", Port: 3, Locality: LocalityID{Region: "a"}, LocalityWeight: 1},
	}
	other := []Endpoint{{Host: "b0", Port: 4, Locality: LocalityID{Region: "b"}, LocalityWeight: 1}}
	all := append(append([]Endpoint{}, region...), other...)
	health := newClusterHealth(all, 0.5)
	health.states["a1:2"].healthy.Store(false)
	health.states["a2:3"].healthy.Store(false) // region a: 1/3 ≈ 33% < 50% (locally panics); cluster-wide: 2/4 = 50%, NOT < 50% (no cluster-wide panic)
	if health.inPanic(all) {
		t.Fatal("test setup invariant broken: cluster-wide must NOT be in panic (2/4 == 50%, strict <)")
	}
	regionAChild := &roundRobin{endpoints: region, health: health}
	factory := func(sub []Endpoint) (loadBalancer, error) {
		if len(sub) == len(region) {
			return regionAChild, nil
		}
		return &roundRobin{endpoints: sub, health: health}, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 }) // rng()==0 → r==0 → the wrapper always picks the FIRST bucket (region a, the only nonzero-remaining-weight locality drawn first in encounter order)
	if err != nil {
		t.Fatal(err)
	}
	// Fully deterministic, not a statistical draw: the wrapper always delegates
	// to regionAChild (rng pinned to 0), and regionAChild's OWN panic branch
	// round-robins via a persistent counter (loadbalancer.go:44-66) that cycles
	// a0,a1,a2,a0,... on every call regardless of health — so an unhealthy host
	// (a1 or a2) is guaranteed within the first 3 draws.
	sawUnhealthyAHost := false
	for i := 0; i < 3; i++ {
		ep, release, err := lw.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatal(err)
		}
		release()
		if ep.Host == "a1" || ep.Host == "a2" {
			sawUnhealthyAHost = true
		}
	}
	if !sawUnhealthyAHost {
		t.Error("region a's local panic branch must surface an unhealthy host within 3 draws (deterministic round-robin cycling) — the documented child-local-panic coverage boundary did not reproduce")
	}
}
```

- [ ] **Step 2: Run**

```bash
cd internal/cluster && go test -run 'TestEffectiveWeight_ConfirmedDegradationCurve|TestEffectiveWeight_OPF100vs140_AB|TestPick_ExplicitZeroOPF|TestPick_ChildLocalityCanLocallyPanic' ./... -v 2>&1 | tail -40
```
Expected: all PASS (the last test is fully deterministic under the pinned `rng()==0` — see its comment).

- [ ] **Step 3: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/locality_test.go
git commit -m "phase 52 Task 6: unit-test depth — the confirmed 6-point AMEND-LW3 degradation curve + the OPF-100-vs-140 A/B (SPEC §11.3, exact live-probed numbers) + D-LW-OPF0's Pick-level unconditional-flat-fallback consequence + a documentation test for the child-local-panic coverage boundary (found reading roundRobin.Pick at this PLAN's Task 6 review)"
```

---

## Task 7: `test/fixtures/0095-lb-locality-weighted` driver — the 2-locality×5-host topology + the toggle-capable health-degradation harness

**Goal:** Register the `0095-lb-locality-weighted` cross-side fixture: cluster `c_lw`, TWO `LocalityLbEndpoints` groups (region "a" weight 2 × 5 hosts, region "b" weight 1 × 5 hosts), an active HTTP health checker (`/healthz`, fast convergence), `common_lb_config.locality_weighted_lb_config: {}`. Region B's 5 hosts are runner-spawned `HTTPEcho` backends (`BackendCount()==5`, always healthy). Region A's 5 hosts are DRIVER-OWNED `toggleResponder`s (self-managed `net.Listener`s — the `0066` `allocDeadPort` precedent generalized from "permanently closed" to "live and toggleable"): each answers `200 "region-a:<idx>"` on any data path and `200`/`503` on `/healthz` depending on an `atomic.Bool` the driver flips mid-run (arm (b)'s controlled-degradation trigger).

**Files:**
- Create: `test/fixtures/0095-lb-locality-weighted/driver/driver.go`
- Create: `test/fixtures/0095-lb-locality-weighted/driver/driver_test.go`
- Create: `test/fixtures/0095-lb-locality-weighted/README.md`
- Create: `test/fixtures/0095-lb-locality-weighted/expectations.yaml`

- [ ] **Step 1: `driver.go` — topology, toggle responder, bootstrap builders, Drive hooks**

```go
// Package driver registers the 0095-lb-locality-weighted cross-side
// differential fixture (phase 52 SPEC §8.1 / PLAN Tasks 7-9).
//
// Cross-side [http_connection_manager + router] fixture over ONE cluster
// c_lw (common_lb_config.locality_weighted_lb_config: {}) with TWO
// LocalityLbEndpoints groups — region "a" (load_balancing_weight: 2, 5
// hosts) and region "b" (load_balancing_weight: 1, 5 hosts) — plus an
// active HTTP health checker (path /healthz, fast convergence).
//
// Region B's 5 hosts are runner-spawned HTTPEcho backends (BackendCount()
// ==5, always healthy). Region A's 5 hosts are DRIVER-OWNED toggleable HTTP
// responders (self-managed net.Listeners, NOT part of the runner's backend
// pool — the 0066 "dead port" precedent generalized to a LIVE, TOGGLEABLE
// host): each answers 200 "region-a:<idx>" on any data path and 200/503 on
// /healthz depending on an atomic healthy flag the driver flips mid-run.
//
// AssertStats drives BOTH arms in-band (the only hook holding both admin
// addrs):
//
//	arm (a) — static ratio (all 10 hosts healthy): poll membership_healthy
//	  ==10 on both sides, warmup, send staticLoadCount requests, assert
//	  region A's share is within a ~5σ band of the confirmed 100%-healthy
//	  formula prediction (66.7%/33.3% — SPEC §11.3).
//	arm (b) — health-degradation shift: toggle 3 of region A's 5
//	  driver-owned hosts to FAIL /healthz, poll membership_healthy==7 on
//	  both sides, re-warmup, send degradedLoadCount MORE requests, assert
//	  the region share matches the confirmed 40%-healthy prediction
//	  (52.8%/47.2%).
//
// Cross-references: phase 52 SPEC §8.1/§11.3; 0066-health-check-http (the
// poll-to-converge + warmup pattern, reused verbatim);
// reference_health_check_propagation_warmup; reference_docker_probe_bridge_
// network (host.docker.internal addressing for BOTH the runner-spawned
// region-B backends AND the driver-owned region-A toggle responders);
// reference_differential_band_sigma_margin (~5σ bands);
// reference_differential_run_selector (-run 'TestDifferential/0095');
// reference_fixture_workload_constant_desync;
// reference_differential_asserter_dispatch (StatsAsserter, cross-side).
package driver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0095-lb-locality-weighted"

	refContainerListenerPort = 19170
	refAdminPort             = 9901

	// backendCount is the number of runner-spawned HTTPEcho backends — region
	// B's 5 ALWAYS-healthy hosts. Region A's 5 hosts are driver-owned (below).
	backendCount = 5

	regionAHosts   = 5
	regionBHosts   = 5
	degradedAHosts = 3 // toggled to failing in arm (b): 2/5 = 40% healthy in region A

	weightA, weightB = 2, 1

	// staticLoadCount / degradedLoadCount are the per-arm request counts —
	// the SPEC §11.3 live-probe count (900), reused for continuity with the
	// confirmed data points (reference_fixture_workload_constant_desync).
	staticLoadCount   = 900
	degradedLoadCount = 900

	// The confirmed AMEND-LW3 predictions (SPEC §11.3) + the ~5σ band
	// half-width at n=900 (reference_differential_band_sigma_margin): std ≈
	// sqrt(n·p·(1-p)); the band below is ~5σ as a percentage-point margin,
	// computed once here (not re-derived per assertion) for auditability.
	staticShareA    = 0.667 // 100%-healthy: 2/(2+1)
	staticBandPct   = 8.0   // percentage points (5σ at n=900,p=0.667 ≈ 7.85pp)
	degradedShareA  = 0.528 // 40%-healthy region A (SPEC §11.3 EXACT match)
	degradedBandPct = 8.5   // percentage points (5σ at n=900,p=0.528 ≈ 8.3pp)

	membershipTotal = regionAHosts + regionBHosts // 10, unaffected by health

	convergeDeadline = 30 * time.Second
	convergePoll     = 200 * time.Millisecond
	warmupStable     = 10
	warmupDeadline   = 15 * time.Second
)

func init() {
	fixture.RegisterFixture(fixtureName, &lwDriver{})
}

// toggleResponder is a driver-owned, self-managed HTTP/1.1 responder for ONE
// region-A host: 200 "region-a:<idx>" on any data path; on /healthz, 200
// while healthy.Load()==true, 503 once SetHealthy(false) has been called
// (arm (b)'s controlled-degradation trigger). Unlike the runner's HTTPEcho
// pool (fixed behavior, spawned/owned by the runner), this is spun up by the
// driver itself — the 0066 allocDeadPort precedent generalized from
// "permanently closed" to "live and toggleable".
type toggleResponder struct {
	idx     int
	ln      net.Listener
	srv     *http.Server
	healthy atomic.Bool
}

func newToggleResponder(idx int) (*toggleResponder, error) {
	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("%s: toggle responder[%d]: listen: %w", fixtureName, idx, err)
	}
	r := &toggleResponder{idx: idx, ln: ln}
	r.healthy.Store(true)
	r.srv = &http.Server{Handler: http.HandlerFunc(r.handle)}
	go func() { _ = r.srv.Serve(ln) }() // best-effort; process-lifetime fixture, no explicit teardown
	return r, nil
}

func (r *toggleResponder) handle(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path == "/healthz" {
		if r.healthy.Load() {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		return
	}
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "region-a:%d", r.idx)
}

func (r *toggleResponder) port() int { return r.ln.Addr().(*net.TCPAddr).Port }

// SetHealthy flips the /healthz response (arm (b)'s controlled-failure trigger).
func (r *toggleResponder) SetHealthy(v bool) { r.healthy.Store(v) }

// lwDriver is STATEFUL: it owns the 5 region-A toggleResponders (built once,
// memoized) and stashes the per-side listener addrs from the Drive hooks so
// AssertStats — the only hook holding both admin addrs — can run both arms.
type lwDriver struct {
	mu           sync.Mutex
	refListener  string
	subjListener string
	regionA      []*toggleResponder
}

func (*lwDriver) BackendCount() int                { return backendCount } // region B only
func (*lwDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (*lwDriver) SubjectListenerName() string      { return "l_http" }
func (*lwDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ensureRegionA builds the 5 region-A toggle responders exactly once
// (memoized — both ReferenceBootstrap and SubjectConfig call it and MUST
// agree on the SAME 5 ports).
func (d *lwDriver) ensureRegionA() []*toggleResponder {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.regionA != nil {
		return d.regionA
	}
	out := make([]*toggleResponder, regionAHosts)
	for i := range out {
		r, err := newToggleResponder(i)
		if err != nil {
			panic(err)
		}
		out[i] = r
	}
	d.regionA = out
	return out
}

const healthChecksBlock = `      health_checks:
        - interval: 0.2s
          timeout: 0.2s
          unhealthy_threshold: 1
          healthy_threshold: 1
          http_health_check:
            path: /healthz`

const commonLbConfigBlock = `      common_lb_config:
        locality_weighted_lb_config: {}`

const routeTable = `                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_lw }`

// localityEndpointsBlock renders the two LocalityLbEndpoints groups for the
// given host addressing scheme over the SAME 10 ports (5 region-A
// toggleResponder ports + 5 region-B runner backend ports).
func localityEndpointsBlock(addr string, aPorts, bPorts []int) string {
	var b strings.Builder
	b.WriteString("      load_assignment:\n        cluster_name: c_lw\n        endpoints:\n")
	fmt.Fprintf(&b, "          - locality: { region: a }\n            load_balancing_weight: %d\n            lb_endpoints:\n", weightA)
	for _, p := range aPorts {
		fmt.Fprintf(&b, "              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }\n", addr, p)
	}
	fmt.Fprintf(&b, "          - locality: { region: b }\n            load_balancing_weight: %d\n            lb_endpoints:\n", weightB)
	for _, p := range bPorts {
		fmt.Fprintf(&b, "              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }\n", addr, p)
	}
	return b.String()
}

func (d *lwDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	regionA := d.ensureRegionA()
	aPorts := make([]int, regionAHosts)
	for i, r := range regionA {
		aPorts[i] = r.port()
	}
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 0.0.0.0, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
%s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_lw
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
%s
%s
`, refAdminPort, refContainerListenerPort, routeTable, healthChecksBlock, commonLbConfigBlock,
		localityEndpointsBlock("host.docker.internal", aPorts, backendPorts))
}

func (d *lwDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	regionA := d.ensureRegionA()
	aPorts := make([]int, regionAHosts)
	for i, r := range regionA {
		aPorts[i] = r.port()
	}
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0095, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_http
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
%s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
  clusters:
    - name: c_lw
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s
%s
%s
`, subjAdminPort, subjListenerPort, routeTable, healthChecksBlock, commonLbConfigBlock,
		localityEndpointsBlock("127.0.0.1", aPorts, backendPorts))
}

func (d *lwDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

func (d *lwDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

func (*lwDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref probe: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj probe: %w", err)
	}
	return refBytes, subjBytes, nil
}

// classifyBody attributes a load response to region "a" ("region-a:<idx>",
// the driver-owned toggleResponders) or region "b" ("backend-<idx>:...", the
// runner-spawned HTTPEcho pool).
func classifyBody(body []byte) (region string, err error) {
	s := string(body)
	switch {
	case strings.HasPrefix(s, "region-a:"):
		return "a", nil
	case strings.HasPrefix(s, "backend-"):
		return "b", nil
	default:
		return "", fmt.Errorf("body %q matches neither region-a: nor backend- prefix", s)
	}
}
```

- [ ] **Step 2: `driver_test.go` — parse + constant-sync tests**

```go
package driver

import "testing"

func TestClassifyBody(t *testing.T) {
	cases := []struct {
		body    string
		want    string
		wantErr bool
	}{
		{"region-a:0", "a", false},
		{"region-a:4", "a", false},
		{"backend-0:health", "b", false},
		{"backend-4:", "b", false},
		{"garbage", "", true},
		{"", "", true},
	}
	for _, c := range cases {
		got, err := classifyBody([]byte(c.body))
		if (err != nil) != c.wantErr {
			t.Errorf("classifyBody(%q): err = %v, wantErr %v", c.body, err, c.wantErr)
			continue
		}
		if got != c.want {
			t.Errorf("classifyBody(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

// TestConstants guards against reference_fixture_workload_constant_desync:
// the topology constants must stay internally consistent.
func TestConstants(t *testing.T) {
	if membershipTotal != regionAHosts+regionBHosts {
		t.Errorf("membershipTotal=%d != regionAHosts+regionBHosts=%d", membershipTotal, regionAHosts+regionBHosts)
	}
	if backendCount != regionBHosts {
		t.Errorf("backendCount=%d must equal regionBHosts=%d (region B is the runner-spawned pool)", backendCount, regionBHosts)
	}
	if degradedAHosts >= regionAHosts {
		t.Errorf("degradedAHosts=%d must be < regionAHosts=%d (some region-A hosts must stay healthy)", degradedAHosts, regionAHosts)
	}
}

func TestToggleResponder_StartsHealthyAndToggles(t *testing.T) {
	r, err := newToggleResponder(0)
	if err != nil {
		t.Fatal(err)
	}
	if !r.healthy.Load() {
		t.Error("toggleResponder must start healthy")
	}
	r.SetHealthy(false)
	if r.healthy.Load() {
		t.Error("SetHealthy(false) must clear the healthy flag")
	}
}
```

- [ ] **Step 3: `README.md` + `expectations.yaml`**

`README.md` (prose mirroring the package doc comment above: topology, the two arms, the toggle-responder mechanism, the band margins and how they were computed). `expectations.yaml` (mirroring `0066`'s shape):

```yaml
# Phase-52 fixture 0095-lb-locality-weighted: differential expectations.
#
# Cross-side [http_connection_manager + router] fixture over ONE cluster
# c_lw (common_lb_config.locality_weighted_lb_config: {}), TWO
# LocalityLbEndpoints groups (region a: weight 2, 5 hosts; region b: weight
# 1, 5 hosts), active HTTP health checking (path /healthz, fast
# convergence). Region B is the runner-spawned HTTPEcho pool; region A is 5
# driver-owned toggleable HTTP responders.
#
# arm (a) — static ratio (all 10 hosts healthy): region A share ≈ 66.7% ±
#   8.0pp (a ~5σ band at n=900 — reference_differential_band_sigma_margin).
# arm (b) — health-degradation shift: 3 of region A's 5 hosts toggled to
#   fail /healthz (2/5 = 40% healthy in region A); region A share shifts to
#   ≈ 52.8% ± 8.5pp — the confirmed AMEND-LW3 40%-healthy prediction
#   (SPEC §11.3, an EXACT match at the live probe).
#
# Cross-side deterministic stats (both sides, both arms):
#   cluster.c_lw.membership_total    == 10 (always — filtering, not removal)
#   cluster.c_lw.membership_healthy  == 10 (arm a) / 7 (arm b, post-degrade)
#   cluster.c_lw.upstream_rq_total   >= 1800 (staticLoadCount + degradedLoadCount)
# Plus the "decode ran" guard: cluster.c_lw.upstream_rq_total > 0 on the
# reference before trusting the readout.
#
# response-body: byte-exact required ONLY over the fixed "READY\n" Drive
# stream (address-independent). The load-phase GET / bodies are tallied for
# the per-side region-share band assertions inside AssertStats, NOT
# byte-compared (a randomized LB policy — cross-side per-request identity is
# infeasible, the 0060/0065 lineage).
#
# Non-additions: NO new BackendKind (region B reuses HTTPEcho; region A is
# driver-owned per reference_differential_fixture_dispatch_constraint); NO
# DistributionAsserter (the region-share bands live in AssertStats, off the
# same load-and-tally pass that must sequence arm (a) BEFORE arm (b)'s
# degradation trigger — reference_differential_asserter_dispatch).
response-body:
  applicable: true
  scope: byte-exact
```

- [ ] **Step 4: Run + gate**

```bash
cd test/fixtures/0095-lb-locality-weighted/driver && go test ./... -v 2>&1 | tail -30
gofmt -l test/fixtures/0095-lb-locality-weighted/
golangci-lint run ./test/fixtures/0095-lb-locality-weighted/...
go build ./...
```
Expected: all PASS; the driver registers without panicking at `init()` (confirm via `go test ./test/differential/... -run 'TestDifferential/0095' -list '.*'` once Task 8 wires the runner — this task alone only proves the unit-level parse/constant tests).

- [ ] **Step 5: Commit (LOCAL-ONLY)**

```bash
git add test/fixtures/0095-lb-locality-weighted/
git commit -m "phase 52 Task 7: 0095-lb-locality-weighted driver scaffolding (SPEC §8.1) — the 2-locality×5-host topology, the driver-owned toggleResponder health-degradation harness (the 0066 allocDeadPort precedent generalized to live+toggleable), bootstrap builders for both sides, README + expectations.yaml"
```

---

## Task 8: `0095` `AssertStats` — arm (a) static-ratio band, arm (b) degraded-ratio band, cross-side stats

**Goal:** Implement the driver's `AssertStats` (the sole hook holding both admin addrs): converge + warmup + load + band-assert for arm (a), THEN toggle 3 of region A's hosts unhealthy, converge + warmup + load + band-assert for arm (b), THEN the cross-side deterministic stats prong.

**Files:**
- Modify: `test/fixtures/0095-lb-locality-weighted/driver/driver.go`

- [ ] **Step 1: Add the poll/warmup/tally/assert helpers + `AssertStats`**

First, add `"strconv"` to `driver.go`'s import block (Task 7 did not need it; `scrapeStats` below does, via `strconv.ParseUint`):

```go
import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)
```

Then append to `driver.go`:

```go
func pollMembershipHealthy(side, adminAddr string, want int) error {
	deadline := time.Now().Add(convergeDeadline)
	var last int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			if v, ok := st["cluster.c_lw.membership_healthy"]; ok {
				last = int64(v)
				if v == uint64(want) {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: cluster.c_lw.membership_healthy did not converge to %d within %s (last seen %d)", side, want, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

type regionTally struct{ a, b int }

func loadAndTally(ctx context.Context, side, addr string, n int) (regionTally, error) {
	var t regionTally
	for i := 0; i < n; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil {
			return t, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return t, fmt.Errorf("%s: GET /[%d]: status %d, want 200", side, i, resp.StatusCode)
		}
		region, err := classifyBody(body)
		if err != nil {
			return t, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if region == "a" {
			t.a++
		} else {
			t.b++
		}
	}
	return t, nil
}

func warmupUntilStable(ctx context.Context, side, addr string) error {
	deadline := time.Now().Add(warmupDeadline)
	consecutive := 0
	for {
		resp, _, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err == nil && resp.StatusCode == http.StatusOK {
			consecutive++
			if consecutive >= warmupStable {
				return nil
			}
		} else {
			consecutive = 0
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: data path did not stabilize to %d consecutive 200s within %s", side, warmupStable, warmupDeadline)
		}
	}
}

// assertShareInBand asserts region A's percentage share of (t.a+t.b) falls
// within ±bandPct percentage points of wantShare (reference_differential_
// band_sigma_margin — a PER-SIDE statistical band, NOT cross-side
// per-request identity: the independent per-request RNG makes cross-side
// identity infeasible for a randomized policy, the 0060/0065 lineage).
func assertShareInBand(t fixture.TB, side string, tally regionTally, wantShare, bandPct float64) {
	t.Helper()
	total := tally.a + tally.b
	if total == 0 {
		t.Errorf("%s: zero total requests tallied", side)
		return
	}
	gotSharePct := 100 * float64(tally.a) / float64(total)
	wantSharePct := 100 * wantShare
	if gotSharePct < wantSharePct-bandPct || gotSharePct > wantSharePct+bandPct {
		t.Errorf("%s: region A share = %.2f%% (a=%d b=%d), want %.1f%% ± %.1fpp", side, gotSharePct, tally.a, tally.b, wantSharePct, bandPct)
	}
}

func (d *lwDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	regionA := d.regionA
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q)", refListener, subjListener)
	}

	// --- arm (a): static ratio, all 10 hosts healthy ---
	if err := pollMembershipHealthy("reference", refAdminAddr, membershipTotal); err != nil {
		t.Fatalf("arm(a) converge: %v", err)
	}
	if err := pollMembershipHealthy("subject", subjAdminAddr, membershipTotal); err != nil {
		t.Fatalf("arm(a) converge: %v", err)
	}
	if err := warmupUntilStable(ctx, "reference", refListener); err != nil {
		t.Fatalf("arm(a) warmup: %v", err)
	}
	if err := warmupUntilStable(ctx, "subject", subjListener); err != nil {
		t.Fatalf("arm(a) warmup: %v", err)
	}
	refStaticTally, err := loadAndTally(ctx, "reference", refListener, staticLoadCount)
	if err != nil {
		t.Fatalf("arm(a) load: %v", err)
	}
	subjStaticTally, err := loadAndTally(ctx, "subject", subjListener, staticLoadCount)
	if err != nil {
		t.Fatalf("arm(a) load: %v", err)
	}
	assertShareInBand(t, "reference/static", refStaticTally, staticShareA, staticBandPct)
	assertShareInBand(t, "subject/static", subjStaticTally, staticShareA, staticBandPct)

	// --- arm (b): degrade 3 of region A's 5 hosts, re-measure the SHIFT ---
	for i := 0; i < degradedAHosts; i++ {
		regionA[i].SetHealthy(false)
	}
	if err := pollMembershipHealthy("reference", refAdminAddr, membershipTotal-degradedAHosts); err != nil {
		t.Fatalf("arm(b) converge: %v", err)
	}
	if err := pollMembershipHealthy("subject", subjAdminAddr, membershipTotal-degradedAHosts); err != nil {
		t.Fatalf("arm(b) converge: %v", err)
	}
	if err := warmupUntilStable(ctx, "reference", refListener); err != nil {
		t.Fatalf("arm(b) warmup: %v", err)
	}
	if err := warmupUntilStable(ctx, "subject", subjListener); err != nil {
		t.Fatalf("arm(b) warmup: %v", err)
	}
	refDegradedTally, err := loadAndTally(ctx, "reference", refListener, degradedLoadCount)
	if err != nil {
		t.Fatalf("arm(b) load: %v", err)
	}
	subjDegradedTally, err := loadAndTally(ctx, "subject", subjListener, degradedLoadCount)
	if err != nil {
		t.Fatalf("arm(b) load: %v", err)
	}
	assertShareInBand(t, "reference/degraded", refDegradedTally, degradedShareA, degradedBandPct)
	assertShareInBand(t, "subject/degraded", subjDegradedTally, degradedShareA, degradedBandPct)

	// --- cross-side deterministic stats ---
	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}
	if ref["cluster.c_lw.upstream_rq_total"] == 0 {
		t.Fatalf("reference did NOT decode: cluster.c_lw.upstream_rq_total == 0")
	}
	for _, sd := range []struct {
		side string
		st   map[string]uint64
	}{{"reference", ref}, {"subject", subj}} {
		assertEq(t, sd.side, sd.st, "cluster.c_lw.membership_total", uint64(membershipTotal))
		assertEq(t, sd.side, sd.st, "cluster.c_lw.membership_healthy", uint64(membershipTotal-degradedAHosts))
		if got := sd.st["cluster.c_lw.upstream_rq_total"]; got < uint64(staticLoadCount+degradedLoadCount) {
			t.Errorf("%s: cluster.c_lw.upstream_rq_total = %d, want >= %d (the measured load alone; convergence/warmup traffic adds more)", sd.side, got, staticLoadCount+degradedLoadCount)
		}
	}
}

func assertEq(t fixture.TB, side string, st map[string]uint64, key string, want uint64) {
	t.Helper()
	v, ok := st[key]
	if !ok {
		t.Errorf("%s: %s ABSENT in /stats", side, key)
		return
	}
	if v != want {
		t.Errorf("%s: %s = %d, want %d", side, key, v, want)
	}
}

// scrapeStats issues GET http://<addr>/stats and parses "name: value" lines
// into a map[name]uint64 (the 0066/0069 scrapeStats, verbatim).
func scrapeStats(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	out := make(map[string]uint64)
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	for {
		nn, rerr := resp.Body.Read(tmp)
		if nn > 0 {
			buf = append(buf, tmp[:nn]...)
		}
		if rerr != nil {
			break
		}
	}
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ": ")
		if idx < 0 {
			continue
		}
		name := line[:idx]
		valStr := strings.TrimSpace(line[idx+2:])
		v, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue
		}
		out[name] = v
	}
	return out, nil
}

var (
	_ fixture.Driver        = (*lwDriver)(nil)
	_ fixture.StatsAsserter = (*lwDriver)(nil)
)
```

- [ ] **Step 2: Run (subject-side path first; cross-side where Docker is present)**

```bash
go test ./test/differential/ -run 'TestDifferential/0095' -count=1 2>&1 | tail -30
# cross-side (controller runs where Docker + the contrib image are present):
#   verify the decode ran: cluster.c_lw.upstream_rq_total > 0 on the reference.
```
Expected: PASS. Confirm via `-run 'TestDifferential/0095'` (`reference_differential_run_selector`).

- [ ] **Step 3: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l test/fixtures/0095-lb-locality-weighted/ && golangci-lint run ./test/...
git add test/fixtures/0095-lb-locality-weighted/driver/driver.go
git commit -m "phase 52 Task 8: 0095 AssertStats — arm (a) static-ratio band (66.7% ±8.0pp) + arm (b) health-degradation shift band (52.8% ±8.5pp, post-toggle re-convergence+re-warmup) + cross-side membership_total/membership_healthy/upstream_rq_total stats; fixtures 96 → 97"
```

---

## Task 9: `0095` deliberate-break liveness (`-count=1`) + the ≥20-run flake check

**Goal:** PROVE both `0095` assertions are LIVE via the two SPEC §8.1 deliberate breaks, each restored after; confirm the workload constants are synced; run a ≥20-run flake check.

**Files:**
- Modify: `test/fixtures/0095-lb-locality-weighted/README.md` (record the break protocol)
- Modify: `docs/envoy-go/phases/52-load-balancer-locality-weighted/PROGRESS.md`

- [ ] **Step 1: Break (i) — skip the `locality_weighted_lb_config` wrap** (defeats arm (a)'s ratio band)

Temporarily comment out the `case lwc != nil:` body's `lb = lw` assignment in `manager.go` (leaving the FLAT `buildLeafLB` output as `cl.lb`, un-wrapped) → the cluster falls back to plain ROUND_ROBIN over all 10 hosts → region A's share collapses to ~50% (5/10 hosts), well outside `[58.7%, 74.7%]` → arm (a)'s `assertShareInBand` MUST fail.

```bash
go test ./test/differential/ -run 'TestDifferential/0095' -count=1 2>&1 | tail   # expect FAIL (arm a band)
```
RESTORE (`git restore` the file — `feedback_subagent_worktree_detach`: never checkout-sha/amend). Re-run → PASS.

- [ ] **Step 2: Break (ii) — freeze the effective-weight computation at the 100%-healthy value** (defeats arm (b)'s shifted band)

Temporarily hardcode `frac := 1.0` unconditionally at the top of `Pick`'s per-group loop in `locality.go` (removing the `if lw.health != nil && len(g.endpoints) > 0 { frac = ... }` branch, so `effectiveWeight` never sees live health) → arm (b)'s post-degradation share stays at ~66.7% instead of shifting to ~52.8%, outside `[44.3%, 61.3%]` → arm (b)'s `assertShareInBand` MUST fail (arm (a), measured BEFORE the break's health-blindness matters at 100% health anyway, still passes — only arm (b) is diagnostic here).

```bash
go test ./test/differential/ -run 'TestDifferential/0095' -count=1 2>&1 | tail   # expect FAIL (arm b band)
```
RESTORE. Re-run → PASS.

- [ ] **Step 3: ≥20-run flake check + the constant-sync guard**

```bash
go test ./test/differential/ -run 'TestDifferential/0095' -count=20 2>&1 | tail   # 20/20 PASS
grep -n "staticLoadCount\|degradedLoadCount\|regionAHosts\|regionBHosts\|degradedAHosts" test/fixtures/0095-lb-locality-weighted/driver/driver.go
```
Expected: 20/20 PASS (a wide ~5σ band at n=900 is intentionally flake-resistant); every workload count is a named constant, never a re-derived literal.

- [ ] **Step 4: `-race`**

```bash
go test ./test/differential/ -run 'TestDifferential/0095' -race -count=1 2>&1 | tail
```
Expected: PASS, no race (the `toggleResponder.healthy` field is an `atomic.Bool`; `lwDriver`'s stashed fields are `sync.Mutex`-guarded).

- [ ] **Step 5: Record + commit (LOCAL-ONLY)**

Record the two breaks (each: the exact edit, the FAILING assertion, the restore) in `README.md` + PROGRESS.md.
```bash
git add test/fixtures/0095-lb-locality-weighted/README.md docs/envoy-go/phases/52-load-balancer-locality-weighted/PROGRESS.md
git commit -m "phase 52 Task 9: 0095 deliberate-break liveness (reference_differential_break_protocol_count1) — skip-the-wrap (arm a band) / freeze-effective-weight-at-100pct-healthy (arm b band), each FAILS under -count=1 then restored; 20/20 flake-free; -race clean"
```

---

## Task 10: Completion bundle — BEHAVIOR_CONTRACT delta + ADR-0269 body + final six-gate

**Goal:** Land the atomic completion bundle: the new `### Load balancer — locality-weighted (locality_weighted_lb_config)` BEHAVIOR_CONTRACT section (mirroring the `### Load balancer — subset` section's shape, `BEHAVIOR_CONTRACT.md:1232-1263`); the full ADR-0269 entry (§Context — promoting the SPEC §13 draft verbatim — + §Decision + §Consequences, ADR-0044 in-place); the final six-gate. **This task's subagent does NOT touch STATE.md/ROADMAP.md** — per this session's controller instructions, those are updated by the controller after reviewing the completed IMPL, not by a task subagent (a deliberate departure from the 38.1 PLAN's Task 14, which bundled STATE/ROADMAP into the same subagent commit — recorded here so the IMPL session doesn't silently diverge from ITS OWN controller's instructions).

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/DECISIONS.md`
- Modify: `docs/envoy-go/phases/52-load-balancer-locality-weighted/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT — the new section**

Add `### Load balancer — locality-weighted (locality_weighted_lb_config)` immediately after the EXISTING `### Load balancer — subset (lb_subset_config)` section (`BEHAVIOR_CONTRACT.md:1232-1263`), covering: the opening italic intro (citing ADR-0269 + the phase-38 `subsetLB` structural precedent + the D-LW7 zero-seam-change framing); the wrap-after-switch acceptance (the SECOND wrap site, hoisted `sc`); the `Endpoint.Locality`/`LocalityWeight` dimension; the confirmed effective-weight formula with the exact AMEND-LW2/LW3/LW4 findings (the omitted-weight-is-zero-load refutation, the confirmed formula + the OPF A/B, the panic full-bypass); the two explicit rejects + their departure framing (§6.1, VERBATIM wording); the confirmed-zero stat delta (§7) PLUS the D-LW-HEALTHALLOC side effect (a locality-weighted cluster with zero `health_checks` now ALSO gets `membership_healthy`/`lb_healthy_panic` registered — a widened CONDITION on two pre-existing stat names, not a new name, not a stat-surface COUNT change); the D-LW-OPF0 resolution (the absent-vs-explicit-zero distinction, MORE correct than the SPEC's leaning-to-accept default); the child-local-panic coverage boundary found at Task 6; the differential proof shape (`0095`'s two arms); the deferred surface list (from SPEC §2: `zone_aware_lb_config`, subset-LB composition, `priority`/`proximity`, the EDS-churn fields, `consistent_hashing_lb_config`/`override_host_status`). Stat-surface doc count STAYS **1200**.

- [ ] **Step 2: DECISIONS — ADR-0269 (ADR-0044 in-place)**

Promote the SPEC §13 §Context DRAFT to the full entry (§Context verbatim from SPEC §13 + §Decision + §Consequences; status ACCEPTED). §Consequences records: the D-LW-HEALTHALLOC health-registry-widening side effect; the D-LW-OPF0 absent-vs-explicit-zero design (a refinement beyond the SPEC's own indicative sketch); the child-local-panic coverage boundary (Task 6's documentation test); ADR-0024 (per-cluster LB-state scope) STAYS UNAMENDED. DECISIONS tail → **ADR-0269**; next-free **ADR-0270**.

- [ ] **Step 3: PROGRESS.md final + the six-gate evidence**

Mark all 10 tasks complete; record the six-gate evidence (build / unit+race / the full 97-dir differential / conformance asserted-unaffected / counts / docs). Confirm the ADR-0045 verdict held (NO split — 10 tasks / ~230-330 LoC, per the FINAL re-check below).

- [ ] **Step 4: The atomic completion commit (LOCAL-ONLY) — then the controller pushes**

```bash
go build ./... && go test ./... 2>&1 | tail   # final green confirmation BEFORE the bundle commit
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/phases/52-load-balancer-locality-weighted/PROGRESS.md
git commit -m "phase 52 IMPL DONE: locality-weighted LB (locality_weighted_lb_config) — the SIXTH Load-balancing-family construct (ADR-0269), the family's first health-as-continuous-weight construct; ZERO new Pick parameters (D-LW7); AMEND-LW2/LW3/LW4 confirmed formula + panic bypass; 2 new envoy-go-strict rejects; stat surface 1200 (+0); 0095 cross-side static-ratio + health-degradation-shift differential (fixtures 96 → 97); fuzzers 52 + BackendKind 38 UNCHANGED"
```
> The controller squash-merges to master, updates STATE.md/ROADMAP.md (row 52 `in-progress → done`, NO parent rollup per ADR-0106), and pushes at stage-close (`feedback_push_to_origin`/`feedback_subagents_no_push`).

---

## FINAL ADR-0045 split-gate re-check

This PLAN decomposes into **10 tasks** (well under the `>~25 tasks` gate) over an estimated **~230-330 production LoC**: `locality.go` ~150-170 LoC (types + both constructors + `effectiveWeight` + `Pick`) + `cluster.go`'s `LocalityID`/`Endpoint` field additions ~15 LoC + `manager.go`'s `extractEndpoints` capture (~8 LoC delta) + the wrap-after-switch (~35-40 LoC) — well under the `>~1500 LoC` gate. **NO split is needed.** The wrapper reuses the EXISTING `loadBalancer` interface, the EXISTING `buildLeafLB` factory, and the EXISTING `clusterHealth` model UNCHANGED — there is no second seam to build (contrast phase 38, which built a NEW `Pick` parameter + a NEW HTTP-route producer, warranting its own by-plane split into 38.1/38.2). This is the phase-35/37 single-flat-row precedent: one new file, one new wrap site, zero new packages, zero new producer plane.

---

## Verification checklist (the ADR-0052 six-gate, at Task 10)

1. **Build:** `go build ./...` green; `go mod tidy -diff` empty (ZERO new dep).
2. **Unit + race:** `go test ./...` + `go test -race -short ./...` green (incl. `locality_test.go`, the widened `manager_test.go`, the `0095` driver's unit-level tests).
3. **Differential:** the full 97-dir suite green (`-count=1`); `0095` liveness-proven (2 breaks, ≥20-run flake-free, `-race` clean); the 96 prior dirs byte-exact (the wrap-after-switch only fires for `locality_weighted_lb_config` clusters; no prior fixture configures it).
4. **Conformance:** h2spec + proxy-wasm asserted-unaffected by change-scope (locality-weighted touches only the cluster LB pick, not HTTP/2 framing or the wasm path); re-run where the harness is present.
5. **Counts:** fixtures 96 → 97 (`0095`); stat surface 1200 UNCHANGED (+0, confirmed §11.5); fuzzers 52 UNCHANGED; BackendKind 38 UNCHANGED; DECISIONS tail ADR-0268 → ADR-0269 (next-free ADR-0270).
6. **Docs:** BEHAVIOR_CONTRACT `### Load balancer — locality-weighted` section (mirroring the subset section's shape) + the ADR-0269 full entry (§Context/§Decision/§Consequences, ADR-0044 in-place). STATE/ROADMAP row 52 `in-progress → done` land at the CONTROLLER's stage-close, not inside Task 10's subagent commit (this PLAN's deliberate departure from the 38.1 PLAN's Task 14 shape — see Task 10's note).
