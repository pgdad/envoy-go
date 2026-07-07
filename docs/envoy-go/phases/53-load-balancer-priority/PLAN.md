# Phase 53 Implementation Plan — `priority` LB (`LocalityLbEndpoints.priority`): a tier-overflow/failover WRAPPER over the cluster's `lb_policy` child — the SEVENTH Load-balancing-family construct, the family's SECOND zero-new-`Pick`-parameter wrapper

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `LocalityLbEndpoints.priority` (`envoy.config.endpoint.v3`, field 5 — a plain `uint32` tier number, 0 = highest priority) as a `priorityLB` wrapper that groups a cluster's endpoints by priority tier, computes each tier's effective capacity from its live per-host health fraction (scaled by the EXISTING `ClusterLoadAssignment.Policy.overprovisioning_factor`, a phase-52 dependency reused verbatim), cascades load across tiers in priority order, and — on a genuinely NEW cluster-wide capacity-shortfall condition confirmed by the SPEC's own live probe — falls back to a pure host-count-uniform draw across every host in every tier, ignoring both priority and health entirely. Reuses the EXISTING `loadBalancer.Pick` seam with ZERO new parameters.

**Architecture:** ONE new file `internal/cluster/priority.go` (the `subset.go`/`locality.go` sibling precedent) + one new `Endpoint` field (`Priority uint32` — the THIRD per-endpoint dimension after phase 38's `Metadata` and phase 52's `Locality`/`LocalityWeight`) + an extended `extractEndpoints` (per-`LocalityLbEndpoints`-group capture, a plain scalar with no wrapper-type ambiguity — AMEND-P3) + a THIRD wrap-after-switch site in `manager.go`'s `buildCluster`, placed immediately after the EXISTING `locality_weighted_lb_config` wrap. NO new package, NO new go.mod dependency, NO new producer plane (a cluster-only construct), NO new stat (CONFIRMED zero delta, SPEC §11.5/AMEND-P5 — the family's FOURTH zero-stat-delta LB phase). ONE genuinely new implementation primitive not present in `locality.go`'s shape: `tierHealth`, a panic-DISABLED per-tier health VIEW (AMEND-P1-COROLLARY) — a direct, load-bearing consequence of the SPEC's single most consequential finding (AMEND-P1): the reference's real bypass condition is a cluster-wide capacity-SHORTFALL check, NOT the classic `healthy_panic_threshold`, and NOT an independent per-tier local-panic concept.

**Tech Stack:** Go 1.26.x; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008 — `LocalityLbEndpoints.priority` [a plain `uint32`, field 5] + `ClusterLoadAssignment.Policy.overprovisioning_factor` [already a phase-52 dependency] are BOTH already present in the pinned module — `go mod tidy -diff` stays EMPTY, re-verified live at Task 1); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227 — already live-probed at the SPEC, §11). Reuses `internal/cluster/` (the phase-52 `localityWeightedLB` structural precedent — a wrapper `loadBalancer` owning per-group children built by `buildLeafLB`; the phase-39/40 `clusterHealth`/`hostHealth` model, `health.go`; the `newPCGRNG` crypto-seeded `math/rand/v2` PCG idiom, `leastrequest.go:66-81`, directly reused). ZERO new packages, ZERO new go.mod deps.

## Global Constraints

- Two NEW envoy-go-strict departure rejects, wording VERBATIM per SPEC §6.1 (both confirmed live-probed departures from a reference that MEANINGFULLY COMPOSES both combinations — AMEND-P2):
  1. `cluster: %q: common_lb_config.locality_weighted_lb_config cannot be combined with multi-tier LocalityLbEndpoints.priority`
  2. `cluster: %q: lb_subset_config cannot be combined with multi-tier LocalityLbEndpoints.priority`
- The confirmed per-tier capacity formula (SPEC §11.4/AMEND-P4), MUST be implemented exactly, not approximated: `tierCapacity = min(100, overprovisioning_factor × healthy_fraction)` (a PERCENT, 0-100 — priority tiers carry no per-tier configured weight, only order).
- The confirmed CORRECTED cascade (SPEC §11.4/AMEND-P4, REFUTING the naive recursive-fraction reading the BRAINSTORM's hypothesis implied): each tier's assigned load is capped by its OWN effective capacity taken as an ABSOLUTE percent of the REMAINING budget, NOT scaled by a fraction of it — `load_i = min(capacity_i, remaining_budget_before_i)`, `remaining_budget -= load_i`. The two readings coincide at exactly 2 tiers but diverge at 3+ (a decisive 3-tier live probe caught it) — MUST be implemented via the corrected formula, not the naive one.
- The confirmed cluster-wide capacity-SHORTFALL bypass (SPEC §11.1/AMEND-P1 — the phase's single most consequential finding, REFUTING BOTH BRAINSTORM hypotheses): `Σ_i min(100, overprovisioning_factor × healthy_fraction_i) < 100` (STRICT `<` — the boundary scenario, sum EXACTLY 100, confirmed live to NOT bypass) triggers a full bypass to a flat, health-ignoring, host-count-uniform draw across EVERY host in EVERY tier, reusing the EXISTING `lb_healthy_panic` counter.
- NO per-tier "local panic" (AMEND-P1-COROLLARY, a NEW implementation primitive not present in `locality.go`): each tier's child MUST be built against a panic-DISABLED health view (`tierHealth`) so a degraded tier restricts its share to its own healthy hosts rather than internally flattening — confirmed live: a tier at 20% healthy sends its unhealthy hosts ZERO traffic, never spraying across them.
- The flat fallback child MUST be built with a NIL health registry (a deliberate departure from `locality.go`'s flat child, which safely reuses the shared `clusterHealth` because its bypass condition and its flat leaf's own internal check are identical by construction — priorityLB's differently-shaped bypass condition cannot make that guarantee).
- `overprovisioning_factor` default is **140** when the wrapper is ABSENT (nil `*wrapperspb.UInt32Value`) — REUSED verbatim from phase 52's D-LW-OPF0 pattern (the wrapper's PRESENCE, not just its value, is checked before `.GetValue()`).
- `loadBalancer.Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error)` (`loadbalancer.go:16-23`) stays BYTE-FOR-BYTE unchanged — ZERO new pick-input (confirmed §11.7). The ONLY new plumbing is constructor-time (`priorityLeafFactory`'s explicit health-view parameter).
- Stat surface stays **1200 (+0)** — confirmed zero delta (§11.5/AMEND-P5); NO new `registerClusterMetrics` stat handle for priority itself; the health-registry-widening WIDENS a CONDITION on two pre-existing stat names, exactly the D-LW-HEALTHALLOC precedent reapplied (D-P-HEALTHALLOC).
- Every task runs `gofmt -l` + `golangci-lint run` on the touched packages, PER TASK (not just a final gate) — `feedback_pertask_gofmt_lint`.
- Subagents commit LOCAL-ONLY; the controller squash-merges + pushes at stage-close (`feedback_subagents_no_push`).
- Every new differential assertion is proven live via a deliberate break run with `-count=1` (`reference_differential_break_protocol_count1`); targeted runs use `-run 'TestDifferential/0096'`, never `-run '0096'` (`reference_differential_run_selector`).
- `0096`'s two arms are HARD 100%/0% boundary assertions, NOT statistical bands (`reference_differential_band_sigma_margin` does not apply here — failover is a hard boundary by construction, per the SPEC §8.1/BRAINSTORM Q4 choice to avoid the band-margin re-tuning risk phases 35/52 both hit).

---

## Source-of-truth references

- **SPEC:** `docs/envoy-go/phases/53-load-balancer-priority/SPEC.md` — AUTHORITATIVE. §1.1 AMEND-P1..P6 (the live-probe findings, especially AMEND-P1's refutation of both BRAINSTORM hypotheses and AMEND-P4's 3-tier cascade correction); §3.0 the split disposition (single flat row, NO escape valve); §3.1 the `priorityLB` indicative design (`tierHealth`/`tierCapacity`/`cascadeLoads`/`priorityGroup`/`priorityLB`/`newPriorityLB`/`priorityLeafFactory`/`Pick`); §3.2 the seam-REUSE confirmation (zero `Pick` change); §3.3 the manager wrap-after-switch + the health-registry-widening note; §4 the `Endpoint` dimension + `extractEndpoints` extension; §5 the proto roster; §6 the two-reject roster + the non-reject dispositions (incl. D-P-DUP's merge-not-reject disposition); §7 the confirmed-zero stat delta; §8 the `0096` differential design (both arms in ONE fixture dir + the deliberate breaks + the explicit NO-boot-reject-dir disposition, §8.2); §9 the BEHAVIOR_CONTRACT delta shape; §10 the 11-task spine (this PLAN maps 1:1, see the decomposition table below); §11 the D-P1..D-P6 empirical pins (all RESOLVED, none deferred to this PLAN) — §11.1's exact scenario tables ((e)/(g)/(h)/(f)) and §11.4's two 3-tier scenarios are THE decisive data this PLAN's unit tests reproduce exactly; §12 the THREE PLAN-level D-questions this PLAN resolves below (D-P-DUP/D-P-FACTORY/D-P-RETROFIT); §13 the ADR-0270 §Context DRAFT; §14 the exit counts.
- **BRAINSTORM:** `docs/envoy-go/phases/53-load-balancer-priority/BRAINSTORM.md` — the charter (Q-family/Q0/Q2/Q4). The BRAINSTORM's D-P1 panic hypothesis (§2.5, "cluster-wide panic bypasses" OR "per-tier local panic") is REFUTED at the SPEC (AMEND-P1 — a THIRD mechanism, the capacity-sum bypass) and its D-P4 cascade hypothesis (§2.4, an implicit "recursive-fraction" reading) is CORRECTED at 3+ tiers (AMEND-P4) — this PLAN follows the SPEC's corrected design throughout, NOT the BRAINSTORM's original guesses.
- **The phase-52 PLAN** (`docs/envoy-go/phases/52-load-balancer-locality-weighted/PLAN.md`) — the DIRECT STRUCTURAL AND STYLE TEMPLATE this PLAN mirrors section-for-section (Source-of-truth references / Project conventions / D-question resolutions / File Structure / per-task Files+Interfaces+Steps / a final Verification checklist + ADR-0045 re-check). Also the direct precedent for: the wrapper-owns-per-group-children-via-factory shape (`localityWeightedLB` → `priorityLB`), the `Endpoint`-dimension-addition discipline (`Locality`/`LocalityWeight` → `Priority`), the crypto-constructor/injectable-constructor RNG split, and the health-registry-widening pattern (D-LW-HEALTHALLOC → D-P-HEALTHALLOC).
- **As-built anchors** (captured at worktree tip `c6f91418`; RE-CONFIRM at Task 1 — line numbers shift on the IMPL-session tip; every citation below was verified fresh against this exact HEAD while authoring this PLAN, not copied from the SPEC without re-checking):
  - `internal/cluster/cluster.go:42-60` — the `Endpoint{Host, Port, Metadata, Locality, LocalityWeight}` struct (gains `Priority uint32` at Task 2) + `:63-65` (`Addr()` — UNCHANGED, ignores all non-dial fields).
  - `internal/cluster/manager.go:417` (`la := c.GetLoadAssignment()` — ALREADY in scope; the `ClusterLoadAssignment.Policy.overprovisioning_factor` read site is reused verbatim, Task 6) + `:447-450` (`var health *clusterHealth; if len(hcSpecs) > 0 || outlierCfg != nil { health = newClusterHealth(...) }` — the EXISTING nil-health-fast-path convention this phase departs from a SECOND time, following phase 52's own D-LW-HEALTHALLOC precedent) + `:457-460` (`buildLeafLB` call — the CLUSTER_PROVIDED/unsupported-policy reject lives here, `:385-387`, UNTOUCHED) + `:461-469` (the EXISTING `lb_subset_config` wrap: `sc := c.GetLbSubsetConfig(); if sc != nil { lb = newSubsetLB(...) }`) + `:470-508` (the EXISTING phase-52 SECOND wrap-after-switch: `lwc := c.GetCommonLbConfig().GetLocalityWeightedLbConfig(); zac := ...ZoneAwareLbConfig(); switch { ... }` — Task 6 appends the THIRD wrap-after-switch immediately after this block closes, reusing the ALREADY-IN-SCOPE `sc`/`lwc` variables directly, since both were declared via `:=` at function scope and never go out of scope) + `:509-510` (`cl.lb = lb; cl.health = health` — UNCHANGED assignment sites, now potentially seeing a further-widened `health`) + `:341-388` (`buildLeafLB(c, name, endpoints, health) (loadBalancer, error)` — the REUSED, UNCHANGED factory, confirmed matching `priorityLeafFactory`'s exact 4-parameter shape one-for-one: `func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) { return buildLeafLB(c, name, sub, h) }` compiles verbatim, no signature drift) + `:814-841` (`extractEndpoints` — gains the per-group `Priority` capture at Task 2; the single production `Endpoint{...}` construction site is at `:834`) + `:112-203` (`registerClusterMetrics` — UNCHANGED this phase, but its EXISTING `if c.health != nil { ... }` block at `:163-170` now ALSO fires for a priority-tiered cluster with zero `health_checks`, per Task 6's SECOND health-registry-widening — a side effect, not a code change here, exactly mirroring D-LW-HEALTHALLOC).
  - `internal/cluster/locality.go` (full file, 174 lines) — the DIRECT STRUCTURAL PRECEDENT: `:26-31` (`localityGroup` — the `priorityGroup` template) + `:52-59` (`localityWeightedLB` struct — the `priorityLB` template) + `:63-73` (`newLocalityWeightedLB` — the crypto-constructor split template) + `:85-115` (`newLocalityWeightedLBWithRNG` — the grouping/factory-build template) + `:142-174` (`Pick` — the panic-branch/bucket-walk/forwarding template).
  - `internal/cluster/subset.go:131` (`type leafFactory func(sub []Endpoint) (loadBalancer, error)` — the type `priorityLeafFactory` is a LOCAL VARIATION of, per D-P-FACTORY below, NOT a retrofit) + `:213-230` (`subsetLB.Pick` — the exact forwarding shape `priorityLB.Pick` mirrors: `child.Pick(hashKey, hasHash, match, hasMatch)` unchanged).
  - `internal/cluster/loadbalancer.go:16-23` (`type loadBalancer interface { Pick(...) }` — stays BYTE-FOR-BYTE unchanged) + `:35-66` (`roundRobin` struct + `Pick` — the panic-branch + `panicInc()` call-site CONVENTION `priorityLB.Pick` mirrors at its own bypass branch).
  - `internal/cluster/leastrequest.go:66-81` (`newPCGRNG() (func() uint64, error)` — the crypto-seeded, mutex-guarded PCG idiom REUSED verbatim by `newPriorityLB`).
  - `internal/cluster/health.go:22-48` (`hostHealth` + `newHostHealth` — `healthy.Store(true)` default, the D-P-HEALTHALLOC safety argument) + `:71-93` (`clusterHealth` struct — fields `states map[string]*hostHealth`, `panicThreshold float64`, `membershipHealthy *stats.Gauge`, `panicCounter *stats.Counter`, `ejectionsActive *stats.Gauge`, `nowNanos func() int64` — CONFIRMED against the SPEC §3.1 sketch's `&clusterHealth{states: shared.states, panicThreshold: 0, nowNanos: shared.nowNanos}` literal: EVERY field the sketch omits — `membershipHealthy`/`panicCounter`/`ejectionsActive` — defaults to its Go zero value (`nil`), which is EXACTLY the SPEC's own stated intent ("this view never emits stats") + `newClusterHealth`) + `:142-150` (`availableCount(eps []Endpoint) int` — REUSED, called with a TIER's own sub-slice) + `:165-171` (`inPanic(eps []Endpoint) bool` — REUSED unchanged by `tierHealth`'s panic-disabled view, structurally dead since a fraction is never strictly `< 0`) + `:180-185` (`panicInc()` — REUSED, same shared `clusterHealth.panicCounter` handle) + `:429-437` (`parsePanicThreshold` — REUSED unchanged).
  - `docs/envoy-go/BEHAVIOR_CONTRACT.md:1265-1294` — the `### Load balancer — locality-weighted (locality_weighted_lb_config)` section, the STRUCTURAL TEMPLATE Task 11's new subsection mirrors (opening italic intro; wrap-after-switch acceptance; value semantics; departures + coverage boundaries; deferred surface).
  - `docs/envoy-go/DECISIONS.md` tail `## ADR-0269` (locality-weighted LB, `:16445-16467`) — the single-ADR-body landing-shape precedent (§Context/§Decision/§Consequences, ADR-0044 in-place) Task 11's ADR-0270 entry mirrors.
- **Differential harness template:** `test/fixtures/0095-lb-locality-weighted/driver/driver.go` — the poll-to-converge (`pollMembershipHealthy`) + warmup (`warmupUntilStable`) + `scrapeStats` pattern Task 8/9 reuse VERBATIM, PLUS the driver-owned `toggleResponder` idiom (self-managed `net.Listener`, an `atomic.Bool` health flag flipped mid-run) that Task 8 reuses directly for tier-0's 5 hosts (toggled ALL-unhealthy in arm (b), vs. `0095`'s partial 3-of-5 degrade). `test/differential/fixture/fixture.go` — the `Driver`/`StatsAsserter`/`TB`/`BackendKind` interfaces (`HTTPEcho BackendKind = 1`; tail `H2GoawayResponder BackendKind = 38`, UNCHANGED this phase).

## Project conventions honored throughout (memory + ADRs)

- `feedback_execution_style` / `feedback_git_worktrees` — subagent-driven execution; this PLAN was authored in worktree `.worktrees/phase-53-plan`; the IMPL runs in its own fresh worktree.
- `feedback_subagents_no_push` / `feedback_subagent_worktree_path_targeting` — subagents commit LOCAL-ONLY; every path below is repo-root-relative; PROGRESS.md is pinned at `docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md`.
- `feedback_pertask_gofmt_lint` — every task's own gate runs `gofmt -l` + `golangci-lint run` on touched packages.
- `reference_conn_wrap_method_no_promote` (generalized, applied a THIRD time) — every `Endpoint{}` construction site must be checked when the struct grows a field; Task 1/2 ENUMERATE every site (a genuine, verified finding: every existing site is EITHER a bare `Endpoint{}` zero-value literal OR a keyed literal never touching the new field — none require editing except `extractEndpoints` itself).
- `reference_dynamic_stat_name_charset_guard` — N/A this phase (priority derives from validated xDS bootstrap config, not wire-decoded dynamic stat segments); noted for completeness, no guard needed.
- `reference_percent_cap_cross_multiply` (generalized formula-fidelity discipline) — `tierCapacity`/`cascadeLoads` implement the CONFIRMED formulas exactly (Task 3), unit-tested against the SPEC §11.1/§11.4 data tables verbatim (Task 7), not approximated or "simplified."
- `reference_health_check_propagation_warmup` — `0096`'s arm (b) polls `membership_healthy` to convergence THEN runs the K=10-consecutive-200s warmup before measuring (the phase-39/52 pattern).
- `reference_differential_break_protocol_count1` / `reference_differential_run_selector` / `reference_fixture_workload_constant_desync` — Task 10's deliberate breaks; all workload constants (`staticLoadCount`, `degradedLoadCount`, `tier0Hosts`, `tier1Hosts`) are named constants, never re-derived literals.
- `reference_differential_asserter_dispatch` — the cross-side stats prong uses `StatsAsserter` (in-band, holding both admin addrs), NOT `SubjectAsserter` (which only runs on the reference-less path).
- `reference_differential_fixture_dispatch_constraint` — BOTH `0096` arms share ONE fixture dir / ONE runner branch (per SPEC §8.1/§8.4); the composition-reject tests are SUBJECT-SIDE UNIT TESTS in `manager_test.go`, NEVER a separate cross-side boot-reject dir (SPEC §8.2/AMEND-P2 — the reference doesn't reject either combination itself, so there is nothing on the reference side to boot-reject differentially).
- `reference_differential_band_sigma_margin` — deliberately DOES NOT apply here: `0096`'s two arms assert EXACT 100%/0% boundaries (a hard failover proof, not a statistical ratio), per the SPEC/BRAINSTORM's explicit choice to avoid the band-margin re-tuning risk.
- `reference_round_robin_offset_randomized` — N/A to the wrapper's OWN cascade draw (a float64 cumulative-bucket walk over TIERS, not a round-robin offset), but still applies to whichever leaf policy a tier's child happens to be (ROUND_ROBIN in `0096`) — the cross-side assertion is therefore host-count/tier-share based (100%/0% by TIER), never per-host identity within a tier.
- `reference_docker_probe_bridge_network` — `0096`'s driver-owned tier-0 toggle responders bind `0.0.0.0:0` and are addressed via `host.docker.internal` on the reference side (the `0095` addressing precedent, reused verbatim).
- ADR-0270 (the priority-tier policy + the `Endpoint.Priority` dimension + the capacity-sum bypass + the corrected cascade + the composition rejects; §Context DRAFTED at SPEC §13, §Decision/§Consequences land at Task 11) — the SOLE anticipated ADR (a single-ADR reuse-shape phase, the phase-35/37/52 precedent, confirmed by §11.7's zero-seam-change finding). ADR-0243 (the health-aware LB pick — Approach A, build-time-injected health view) — REUSED unchanged; ADR-0269 (`localityWeightedLB`'s wrapper-owns-children-via-factory shape) — the direct structural precedent. ADR-0024 (per-cluster LB-state scope) — UNAMENDED; tier-group state is per-cluster LB-instance state, the same discipline every prior LB construct follows. ADR-0080 (byte-stable reject text — the two VERBATIM strings above). ADR-0052 (the atomic six-gate completion bundle). ADR-0106 (flat family row, no parent rollup). ADR-0045 (the split-gate — FINAL re-check at the end of this PLAN).

## D-question resolutions (SPEC §12)

- **D-P-DUP (the duplicate-priority-value corner)** — RESOLVED as SPEC's own self-answer, MORE SIMPLY than phase 52's D-LW-DUP (which had to resolve a conflicting WEIGHT via last-write-wins): priority carries no per-group scalar to conflict over, so two `LocalityLbEndpoints` groups declaring the SAME `priority` value simply MERGE their endpoints into the SAME tier — a natural consequence of `newPriorityLBWithRNG`'s `map[uint32][]Endpoint` grouping (Task 4), which appends to the SAME slice keyed by `ep.Priority` regardless of which source group an endpoint came from. This PLAN adds the concrete unit test SPEC leaves implicit: `TestNewPriorityLB_DuplicatePriority_EndpointsMerge` (Task 4) constructs two `Endpoint`s sharing an identical `Priority` value (simulating two source `LocalityLbEndpoints` groups that collapsed to the same tier in `extractEndpoints`'s output) and asserts (a) exactly ONE `priorityGroup` results (not two) and (b) its `endpoints` slice contains BOTH endpoints merged — an unusual, degenerate config shape, low-stakes and untested by either live probe, exactly as SPEC frames it.
- **D-P-FACTORY (the `priorityLeafFactory` local-type choice)** — RESOLVED per the SPEC's own leaning, CONFIRMED against the actual `buildLeafLB` signature (not assumed): `priorityLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)` is a phase-53-LOCAL type (Task 4), distinct from `subset.go`'s existing `leafFactory func(sub []Endpoint) (loadBalancer, error)` — `subset.go`/`locality.go`'s call sites and the shared `leafFactory` type stay COMPLETELY UNTOUCHED. Verified: `buildLeafLB(c *clusterv3.Cluster, name string, endpoints []Endpoint, health *clusterHealth) (loadBalancer, error)` (`manager.go:341`) already accepts a `health *clusterHealth` as its fourth positional parameter, so the manager.go call-site closure `func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) { return buildLeafLB(c, name, sub, h) }` (Task 6) compiles and forwards CORRECTLY without any adjustment to `buildLeafLB` itself — the SPEC §3.1 sketch's assumption holds exactly as written. A wholesale `leafFactory` refactor (accepting an explicit health parameter for EVERY wrapper, retroactively touching `subset.go`/`locality.go`) is explicitly NOT bundled into this phase — it would touch code outside phase 53's blast radius for no phase-53-required benefit.
- **D-P-RETROFIT (whether `locality.go`'s own documented child-local-panic coverage boundary should be retroactively fixed with `tierHealth`'s pattern)** — RESOLVED as SPEC's own self-answer: OUT of THIS phase's scope. `locality.go` is phase 52's, already shipped and documented (`BEHAVIOR_CONTRACT.md:1288`, "the child-local-panic coverage boundary (documented, tested, NOT fixed)"); retrofitting it is a candidate LOW-RISK future maintenance item, NOT bundled here to keep phase 53's blast radius to `priority.go` + the manager wrap site only. No task in this PLAN touches `locality.go`.

### Decomposition note (11 tasks, matching SPEC §10's own indicative spine 1:1)

Unlike phase 52 (whose SPEC left a single "build `locality.go`" row that its OWN PLAN had to split into construction-vs-Pick), phase 53's SPEC §10 ALREADY pre-splits the `priority.go` construct into three natural sub-deliverables at rows 3/4/5 (pure helpers → construction/grouping → `Pick`) — this PLAN inherits that split directly, with ONE clarifying note on the reject-arm test placement (mirroring phase 52's own Task 5/6 split precedent):

| SPEC §10 task | This plan | Note |
|---|---|---|
| 1 baselines/anchors gate | Task 1 | 1:1 |
| 2 `Endpoint.Priority` + `extractEndpoints` capture + construction-site sweep | Task 2 | 1:1 |
| 3 `priority.go`: `tierHealth`/`tierCapacity`/`cascadeLoads` pure functions | Task 3 | 1:1 |
| 4 `priority.go`: `priorityGroup`/`priorityLB` + `newPriorityLB`/`-WithRNG` | Task 4 | 1:1 (incl. a placeholder flat-only `Pick`, the phase-52 Task-3 precedent, so Task 4's own tests run against a real `*priorityLB`) |
| 5 `priority.go`: `Pick` | Task 5 | 1:1 |
| 6 `manager.go` wrap-after-switch: `distinctPriorities` + 2 rejects + wrap + health-widening | Task 6 | 1:1. The reject-arm ACCEPTANCE/REJECTION tests are TDD'd INLINE at this task (the phase-52 Task-5 precedent) |
| 7 unit tests: the confirmed §11.1/§11.4 data tables (8 two-tier scenarios + 2 three-tier scenarios) + the AMEND-P1 boundary + the AMEND-P1-COROLLARY per-tier no-local-panic property | Task 7 | 1:1. Adds the REMAINING numeric/behavioral depth SPEC's own Task 7 calls for — the reject-arm tests themselves are ALREADY covered at Task 6, per the note above (mirroring phase 52's identical Task 5/6 split) |
| 8 `0096` driver: 2-tier×5-host topology + health-check-toggle harness | Task 8 | 1:1 |
| 9 `0096` assertions: arm (a)/(b) hard 100%/0% + cross-side stats | Task 9 | 1:1 |
| 10 `0096` deliberate breaks + flake-soak + `-race` | Task 10 | 1:1 |
| 11 BEHAVIOR_CONTRACT + ADR-0270 + six-gate + ROADMAP note | Task 11 | 1:1 (STATE/ROADMAP itself deferred to the controller — see Task 11's note, the phase-52 Task-10 precedent) |

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `internal/cluster/priority.go` | **CREATE** (Tasks 3, 4, 5) | `tierHealth`/`tierCapacity`/`cascadeLoads` (Task 3, pure functions) + `priorityGroup`/`priorityLB`/`priorityLeafFactory` + `newPriorityLB`/`newPriorityLBWithRNG` (Task 4: construction/grouping/merge/sort + a placeholder `Pick`) + the real `Pick` (Task 5: bypass check + cascade draw + delegation) + `var _ loadBalancer = (*priorityLB)(nil)`. |
| `internal/cluster/priority_test.go` | **CREATE** (Tasks 3, 4, 5, 7) | Pure-function tests for `tierHealth`/`tierCapacity`/`cascadeLoads` (Task 3); construction/grouping + D-P-DUP tests via stub factories (Task 4); `Pick` structural tests via stub factories + deterministic RNG (Task 5); the confirmed §11.1/§11.4 data-table depth + the AMEND-P1-COROLLARY per-tier no-local-panic test (Task 7). |
| `internal/cluster/cluster.go` | MODIFY (Task 2) | `Endpoint` gains `Priority uint32`; `Addr()` UNCHANGED. |
| `internal/cluster/manager.go` | MODIFY (Tasks 2, 6) | `extractEndpoints`'s per-group `Priority` capture (Task 2); the `buildCluster` THIRD wrap-after-switch: `distinctPriorities` helper, the two reject arms, the `priorityLB` wrap incl. the health-registry-widening + the `overprovisioning_factor` read (Task 6). |
| `internal/cluster/manager_test.go` | MODIFY (Tasks 2, 6) | `extractEndpoints` capture tests (Task 2); the accept/reject matrix + the health-widening test + the OPF absent/explicit-zero test (Task 6). |
| `test/fixtures/0096-lb-priority/driver/driver.go` | **CREATE** (Tasks 8, 9) | The 2-tier×5-host topology, the driver-owned tier-0 toggle-responder full-failover harness, the bootstrap builders, `AssertStats` (both arms in-band). |
| `test/fixtures/0096-lb-priority/driver/driver_test.go` | **CREATE** (Task 8) | `classifyBody` parse tests + the workload-constant pin test. |
| `test/fixtures/0096-lb-priority/README.md` + `expectations.yaml` | **CREATE** (Task 8); MODIFY (Task 10) | The fixture design doc + differential expectations; Task 10 appends the break-protocol record. |
| `docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md` | **CREATE** (Task 1) | The IMPL progress ledger. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY (Task 11) | The new `### Load balancer — priority (LocalityLbEndpoints.priority)` section. |
| `docs/envoy-go/DECISIONS.md` | MODIFY (Task 11) | The full ADR-0270 entry (§Context + §Decision + §Consequences). |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | MODIFY (Task 11, by the CONTROLLER at stage-close, not by a Task-11 subagent — see Task 11's note) | Active-phase + counts advance; ROADMAP row 53 `in-progress → done`. |

---

## Task 1: Baselines/anchors gate + PROGRESS.md

**Goal:** Re-confirm every count anchor against the IMPL-session tip BEFORE touching code, re-pin the as-built line anchors, confirm the `Endpoint{}` construction-site sweep needs ZERO edits to existing sites, and create the progress ledger. No production code.

**Files:**
- Create: `docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md`

- [ ] **Step 1: Confirm the count anchors via the canonical recipes**

```bash
ls -d test/fixtures/[0-9]* | wc -l                                    # expect 97
ls -d test/fixtures/[0-9]* | sort | tail -1                          # expect test/fixtures/0095-lb-locality-weighted
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1     # expect H2GoawayResponder BackendKind = 38
grep -rh "^func Fuzz" --include='*.go' . | wc -l                       # expect 52 (canonical recipe; reference_fuzzer_count_docs_drift)
grep "^## ADR-0" docs/envoy-go/DECISIONS.md | tail -1                  # expect tail ADR-0269, next-free ADR-0270
grep -n "1200" docs/envoy-go/BEHAVIOR_CONTRACT.md | tail -3            # the stat-surface DOC count, not a golden test
go build ./... && echo BUILD_OK
go mod tidy -diff && echo TIDY_EMPTY                                   # expect exit 0, empty (ZERO new dep)
```
Expected: fixtures **97** (tail `0095-lb-locality-weighted`); BackendKind tail **38**; fuzzers **52**; DECISIONS tail **ADR-0269**; stat surface **1200**; build clean; `go mod tidy -diff` empty.

- [ ] **Step 2: Re-pin the as-built anchors against the IMPL-session tip**

```bash
grep -n "type Endpoint struct" internal/cluster/cluster.go
grep -n "func (e Endpoint) Addr" internal/cluster/cluster.go
grep -n "la := c.GetLoadAssignment()\|var health \*clusterHealth\|sc := c.GetLbSubsetConfig\|lwc := c.GetCommonLbConfig" internal/cluster/manager.go
grep -n "func buildLeafLB\|func extractEndpoints\|Host: sa.GetAddress()\|cl.lb = lb" internal/cluster/manager.go
grep -n "func registerClusterMetrics" internal/cluster/manager.go
grep -n "type leafFactory\|func (s \*subsetLB) Pick" internal/cluster/subset.go
grep -n "type loadBalancer interface\|func (rr \*roundRobin) Pick" internal/cluster/loadbalancer.go
grep -n "func newPCGRNG" internal/cluster/leastrequest.go
grep -n "type localityGroup\|type localityWeightedLB\|func newLocalityWeightedLB\b\|func newLocalityWeightedLBWithRNG\|func (lw \*localityWeightedLB) Pick" internal/cluster/locality.go
grep -n "type clusterHealth struct\|func newHostHealth\|func newClusterHealth\|func (ch \*clusterHealth) availableCount\|func (ch \*clusterHealth) inPanic\|func (ch \*clusterHealth) panicInc\|func parsePanicThreshold" internal/cluster/health.go
test -f internal/cluster/priority.go && echo "WARN priority.go exists" || echo "priority.go ABSENT (expected — Task 3 creates it)"
```
Record the actual line numbers in PROGRESS.md (they should match this PLAN's citations exactly, since this PLAN was authored same-day against this HEAD; a drift here means re-verify every citation in this PLAN before proceeding).

- [ ] **Step 3: Confirm the `Endpoint{}` construction-site sweep needs ZERO edits**

```bash
grep -rn "Endpoint{" internal/cluster/*.go | grep -v _test.go
grep -rn "Endpoint{" internal/cluster/*_test.go | wc -l
grep -rn "cluster\.Endpoint{" --include=*.go internal/filter/ | sort
```
Expected: every hit is EITHER a bare `Endpoint{}` (zero-value error-path literal — unaffected by the new field) OR a KEYED literal (`Endpoint{Host: ..., Port: ...}` / `...Metadata: ...` / `...Locality: ..., LocalityWeight: ...}`) — NEVER a positional literal. The single PRODUCTION construction site with real field values is `internal/cluster/manager.go`'s `extractEndpoints` (`Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars, Locality: loc, LocalityWeight: weight}`). **Record in PROGRESS.md:** because every existing site is keyed, adding `Priority` to the `Endpoint` struct requires editing EXACTLY ONE call site (`extractEndpoints`, Task 2) — every other site (leaf-policy error returns in `ringhash.go`/`random.go`/`maglev.go`/`leastrequest.go`/`loadbalancer.go`/`subset.go`/`h2pool.go`/`cluster.go`, plus every test builder across `internal/cluster/*_test.go`, plus every `cluster.Endpoint{}` site in `internal/filter/hcm/`/`internal/filter/http/router/`) compiles unchanged with `Priority` defaulting to its zero value (`0`, tier 0). This is a VERIFIED finding (101 total `Endpoint{` hits swept in `internal/cluster/` alone at PLAN-authoring time), not an assumption inherited from the SPEC's generic caution — the THIRD time this exact sweep has been performed (phase 38's `Metadata`, phase 52's `Locality`/`LocalityWeight`, now `Priority`).

- [ ] **Step 4: Create PROGRESS.md**

Create `docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md` with: the 11-task table (status column); the count anchors from Step 1; the as-built line anchors from Step 2; the zero-edit-sites finding from Step 3; the D-P-DUP/D-P-FACTORY/D-P-RETROFIT resolutions (copied from this PLAN's D-question section); the ADR-0045 re-check verdict (NO split — 11 tasks, ~200-250 prod LoC, see the FINAL re-check at the end of this PLAN).

- [ ] **Step 5: Commit (LOCAL-ONLY)**

```bash
git add docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md
git commit -m "phase 53 Task 1: baselines gate + PROGRESS.md (fixtures 97 / fuzzers 52 / stat surface 1200 / BackendKind 38 / DECISIONS tail ADR-0269 confirmed; Endpoint{} construction-site sweep needs ZERO edits — every site is a keyed or bare-zero literal; go mod tidy -diff empty)"
```

---

## Task 2: The `Endpoint.Priority` dimension + the `extractEndpoints` per-group capture

**Goal:** `Endpoint` grows `Priority uint32` (the THIRD per-endpoint dimension after phase 38's `Metadata` and phase 52's `Locality`/`LocalityWeight`); `extractEndpoints` captures `group.GetPriority()` (a PLAIN `uint32` — NO wrapper type, unlike `overprovisioning_factor` — AMEND-P3, so "explicitly 0" and "omitted" are indistinguishable at the proto layer and BOTH simply mean tier 0, no `hasX` threading needed) ONCE per `LocalityLbEndpoints` group and stamps every endpoint in that group with the same tier. `Addr()` stays UNCHANGED.

**Files:**
- Modify: `internal/cluster/cluster.go` (the `Endpoint` struct)
- Modify: `internal/cluster/manager.go` (`extractEndpoints`)
- Modify: `internal/cluster/manager_test.go` (new tests)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
func TestExtractEndpoints_CapturesPriorityPerGroup(t *testing.T) {
	c := mkStaticClusterFromGroups("c_pri",
		&endpointv3.LocalityLbEndpoints{
			Priority:    1,
			LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002)},
		},
		&endpointv3.LocalityLbEndpoints{
			// Priority OMITTED — must default to tier 0 (AMEND-P3: a plain
			// uint32, no wrapper type, so absent and explicit-0 are identical).
			LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9003)},
		},
	)
	eps, err := extractEndpoints(c.GetLoadAssignment(), "c_pri")
	if err != nil {
		t.Fatal(err)
	}
	if len(eps) != 3 {
		t.Fatalf("got %d endpoints, want 3", len(eps))
	}
	for _, ep := range eps[:2] {
		if ep.Priority != 1 {
			t.Errorf("ep %+v: Priority = %d, want 1", ep, ep.Priority)
		}
	}
	if eps[2].Priority != 0 {
		t.Errorf("omitted priority must capture as tier 0 (AMEND-P3): got %d", eps[2].Priority)
	}
}

func TestExtractEndpoints_ExplicitPriorityZero_SameAsOmitted(t *testing.T) {
	// AMEND-P3: priority is a plain scalar uint32 (unlike overprovisioning_factor's
	// wrapper type) — an explicit `priority: 0` group and an omitted-priority
	// group are INDISTINGUISHABLE at the proto layer; both land in tier 0.
	c := mkStaticClusterFromGroups("c_pri0",
		&endpointv3.LocalityLbEndpoints{Priority: 0, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001)}},
		&endpointv3.LocalityLbEndpoints{LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9002)}},
	)
	eps, err := extractEndpoints(c.GetLoadAssignment(), "c_pri0")
	if err != nil {
		t.Fatal(err)
	}
	for _, ep := range eps {
		if ep.Priority != 0 {
			t.Errorf("ep %+v: Priority = %d, want 0", ep, ep.Priority)
		}
	}
}

func TestExtractEndpoints_NoPriorityIsZeroValue(t *testing.T) {
	c := mkStaticCluster("c_plain", mkLbEndpoint("127.0.0.1", 9001))
	eps, err := extractEndpoints(c.GetLoadAssignment(), "c_plain")
	if err != nil {
		t.Fatal(err)
	}
	if eps[0].Priority != 0 {
		t.Errorf("no priority set → Priority 0, got %d", eps[0].Priority)
	}
}

func TestEndpoint_AddrIgnoresPriority(t *testing.T) {
	a := Endpoint{Host: "127.0.0.1", Port: 9001}
	b := Endpoint{Host: "127.0.0.1", Port: 9001, Priority: 7}
	if a.Addr() != b.Addr() {
		t.Errorf("Addr() must ignore Priority: %q vs %q", a.Addr(), b.Addr())
	}
}
```
`manager_test.go` already imports `endpointv3` and defines `mkStaticClusterFromGroups`/`mkStaticCluster`/`mkLbEndpoint` (confirmed at Task 1, phase-52's helper additions — no new import or helper needed).

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestExtractEndpoints_CapturesPriorityPerGroup|TestExtractEndpoints_ExplicitPriorityZero|TestExtractEndpoints_NoPriorityIsZeroValue|TestEndpoint_AddrIgnoresPriority' ./... 2>&1 | head -20
```
Expected: COMPILE FAILURE (`endpointv3.LocalityLbEndpoints{Priority: ...}` compiles fine — it's an existing proto field — but `Endpoint.Priority` is undefined).

- [ ] **Step 3: Add the `Priority` field** (`cluster.go`)

Replace the `Endpoint` struct (currently at `cluster.go:42-60`):

```go
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
	// Priority is the tier number captured from the owning LocalityLbEndpoints
	// group's priority field (phase 53; the THIRD per-endpoint dimension after
	// Metadata and Locality/LocalityWeight). Unlike LocalityWeight/
	// overprovisioning_factor, priority is a PLAIN uint32 proto field (no
	// wrapper type) — 0 is both the explicit-zero AND the omitted value,
	// meaning "highest-priority tier" (AMEND-P3). Only consulted by
	// priorityLB. NOT part of the dial identity.
	Priority uint32
}
```

- [ ] **Step 4: Extend `extractEndpoints`** (`manager.go:814-841`)

Replace the function body:

```go
func extractEndpoints(la *endpointv3.ClusterLoadAssignment, clusterName string) ([]Endpoint, error) {
	var out []Endpoint
	for gi, group := range la.GetEndpoints() {
		l := group.GetLocality() // nil-safe: (*corev3.Locality)(nil).GetRegion() == ""
		loc := LocalityID{Region: l.GetRegion(), Zone: l.GetZone(), SubZone: l.GetSubZone()}
		weight := group.GetLoadBalancingWeight().GetValue() // 0 when unset — AMEND-LW2, no default
		priority := group.GetPriority()                     // plain uint32; 0 == omitted == explicit-zero (AMEND-P3)
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
			out = append(out, Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars, Locality: loc, LocalityWeight: weight, Priority: priority})
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
Expected: PASS, including the full pre-existing suite (the new field defaults to zero everywhere else — Task 1 Step 3's finding confirmed live).

- [ ] **Step 6: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/cluster.go internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 53 Task 2: Endpoint.Priority + extractEndpoints per-group capture (SPEC §4/§5) — the THIRD per-endpoint dimension after Metadata/Locality+LocalityWeight; a plain uint32 proto field, no wrapper-type absent-vs-explicit-zero ambiguity (AMEND-P3); ZERO existing Endpoint{} construction sites needed edits"
```

---

## Task 3: `internal/cluster/priority.go` — `tierHealth`/`tierCapacity`/`cascadeLoads` as pure, independently-testable functions (TDD, part A)

**Goal:** Create `internal/cluster/priority.go` with the three pure functions the rest of the construct is built from: `tierHealth` (a panic-disabled per-tier health VIEW, AMEND-P1-COROLLARY), `tierCapacity` (the confirmed per-tier formula, AMEND-P4), and `cascadeLoads` (the confirmed CORRECTED cascade + the AMEND-P1 capacity-sum bypass quantity, computed together in one pass). None of these touch RNG, factories, or the `loadBalancer` interface — they are proven directly against the SPEC's confirmed numbers with zero stochastic-test risk.

**Files:**
- Create: `internal/cluster/priority.go`
- Create: `internal/cluster/priority_test.go`

- [ ] **Step 1: Write the failing tests** (`priority_test.go`)

```go
package cluster

import "testing"

func TestTierCapacity_MinCapsAt100(t *testing.T) {
	cases := []struct {
		frac float64
		opf  uint32
		want float64
	}{
		{1.00, 140, 100}, // caps at 100 even though 1.00*140=140
		{0.80, 140, 100}, // 0.80*140=112, caps at 100
		{0.60, 140, 84},
		{0.40, 140, 56},
		{0.20, 140, 28},
		{0.00, 140, 0},
		{1.00, 100, 100},
		{0.50, 100, 50}, // OPF=100 has no plateau margin
	}
	for _, c := range cases {
		got := tierCapacity(c.frac, c.opf)
		if diff := got - c.want; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("tierCapacity(%.2f, %d) = %v, want %v", c.frac, c.opf, got, c.want)
		}
	}
}

func TestCascadeLoads_TwoTier_FullyHealthy_AllToP0(t *testing.T) {
	loads, sum := cascadeLoads([]float64{100, 100})
	if loads[0] != 100 || loads[1] != 0 {
		t.Errorf("loads = %v, want [100, 0]", loads)
	}
	if sum != 200 {
		t.Errorf("capacitySum = %v, want 200", sum)
	}
}

func TestCascadeLoads_TwoTier_PartialP0_Cascades(t *testing.T) {
	// SPEC §11.4 scenario d: P0=40% healthy (capacity 56), P1=100% (capacity 100).
	loads, sum := cascadeLoads([]float64{56, 100})
	if diff := loads[0] - 56; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("loads[0] = %v, want 56", loads[0])
	}
	if diff := loads[1] - 44; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("loads[1] = %v, want 44", loads[1])
	}
	if sum != 156 {
		t.Errorf("capacitySum = %v, want 156", sum)
	}
}

func TestCascadeLoads_ExactlyBoundary100_NotABypass(t *testing.T) {
	// SPEC §11.1/§11.4 scenario f: P0=0% (capacity 0), P1=100%. capacitySum ==
	// EXACTLY 100 — the confirmed boundary that does NOT trigger the AMEND-P1
	// bypass (Pick's own "< 100" check, Task 5, is the actual gate; this test
	// only proves cascadeLoads reports the exact sum Pick's comparison needs).
	loads, sum := cascadeLoads([]float64{0, 100})
	if loads[0] != 0 || loads[1] != 100 {
		t.Errorf("loads = %v, want [0, 100]", loads)
	}
	if sum != 100 {
		t.Errorf("capacitySum = %v, want exactly 100 (the boundary)", sum)
	}
}

func TestCascadeLoads_BelowBoundary_BypassCondition(t *testing.T) {
	// SPEC §11.1 scenario g: both tiers at 20% healthy (capacity 28 each).
	// capacitySum = 56 < 100 — Pick's bypass fires on this value (Task 5).
	_, sum := cascadeLoads([]float64{28, 28})
	if sum != 56 {
		t.Errorf("capacitySum = %v, want 56 (< 100 — the bypass condition)", sum)
	}
}

func TestCascadeLoads_ThreeTier_CorrectedNotNaiveRecursive(t *testing.T) {
	// SPEC §11.4 AMEND-P4: the decisive 3-tier probe. P0=40%(cap 56),
	// P1=60%(cap 84), P2=100%(cap 100). The NAIVE recursive-fraction reading
	// predicts P1=84×44%=36.96, P2=63.04 — REFUTED live. The CORRECTED
	// reading (min-against-remaining-budget) predicts P0=56, P1=44, P2=0 —
	// matching the observed P0=55.0%, P1=45.0%, P2=0.0% almost exactly.
	loads, sum := cascadeLoads([]float64{56, 84, 100})
	want := []float64{56, 44, 0}
	for i, w := range want {
		if diff := loads[i] - w; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("loads[%d] = %v, want %v (CORRECTED cascade, NOT the naive-recursive 36.96/63.04 reading)", i, loads[i], w)
		}
	}
	if sum != 240 {
		t.Errorf("capacitySum = %v, want 240 (56+84+100)", sum)
	}
}

func TestCascadeLoads_ThreeTier_SecondScenario(t *testing.T) {
	// SPEC §11.4: P0=20%(cap 28), P1=20%(cap 28), P2=100%(cap 100). CORRECTED
	// prediction: P0=28, P1=28, P2=44 — matching observed 29.0/28.7/42.3.
	loads, sum := cascadeLoads([]float64{28, 28, 100})
	want := []float64{28, 28, 44}
	for i, w := range want {
		if diff := loads[i] - w; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("loads[%d] = %v, want %v", i, loads[i], w)
		}
	}
	if sum != 156 {
		t.Errorf("capacitySum = %v, want 156 (28+28+100)", sum)
	}
}

func TestCascadeLoads_EmptyCapacities(t *testing.T) {
	loads, sum := cascadeLoads(nil)
	if len(loads) != 0 {
		t.Errorf("loads = %v, want empty", loads)
	}
	if sum != 0 {
		t.Errorf("capacitySum = %v, want 0", sum)
	}
}

func TestTierHealth_SharesStatesButDisablesPanic(t *testing.T) {
	ep := Endpoint{Host: "a", Port: 1}
	shared := newClusterHealth([]Endpoint{ep}, 0.5)
	view := tierHealth(shared)
	if view.panicThreshold != 0 {
		t.Errorf("view.panicThreshold = %v, want 0 (panic permanently disabled)", view.panicThreshold)
	}
	// The states map is the SAME map (a reference type) — mutating the shared
	// registry's host state must be visible through the view.
	shared.states[ep.Addr()].healthy.Store(false)
	if view.isHealthy(ep) {
		t.Error("view must observe LIVE health-check results via the shared states map")
	}
	// panicThreshold=0 means inPanic can never fire: a fraction is never
	// strictly < 0, even at 0% available (AMEND-P1-COROLLARY's structural
	// guarantee — this is what makes a tier's own child incapable of
	// internally flattening, Task 7's Pick-level proof).
	if view.inPanic([]Endpoint{ep}) {
		t.Error("tierHealth's view must NEVER report inPanic, even at 0% available")
	}
	// membershipHealthy/panicCounter/ejectionsActive stay nil (this view never
	// emits stats — priorityLB.Pick itself is the sole caller of
	// health.panicInc(), on its own capacity-sum bypass, Task 5).
	if view.membershipHealthy != nil || view.panicCounter != nil || view.ejectionsActive != nil {
		t.Error("tierHealth's view must not carry any stat handles")
	}
}

func TestTierHealth_NowNanosShared(t *testing.T) {
	// nowNanos is shared so a tier's outlier-ejection lazy-uneject check (if
	// ever exercised through this view) uses the SAME injectable clock as the
	// real registry — a unit-test determinism requirement, not a priority-53
	// feature (outlier detection composition with priority tiers is out of
	// scope this phase, matching SPEC's non-purposes list).
	shared := newClusterHealth(nil, 0.5)
	var fixedNow int64 = 42
	shared.nowNanos = func() int64 { return fixedNow }
	view := tierHealth(shared)
	if view.nowNanos() != 42 {
		t.Errorf("view.nowNanos() = %d, want 42 (shared clock)", view.nowNanos())
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestTierCapacity_|TestCascadeLoads_|TestTierHealth_' ./... 2>&1 | head -20
```
Expected: COMPILE FAILURE (`tierCapacity`/`cascadeLoads`/`tierHealth` undefined).

- [ ] **Step 3: Implement `priority.go`**

```go
// Package cluster: internal/cluster/priority.go implements the priority-tier
// overflow/failover load-balancing wrapper (phase 53, ADR-0270).
//
// priorityLB partitions a cluster's endpoints by Priority tier
// (LocalityLbEndpoints.priority) and, on every Pick, computes each tier's
// EFFECTIVE CAPACITY from its live per-host health fraction (scaled by the
// EXISTING overprovisioning_factor, a phase-52 dependency reused verbatim),
// cascades load across tiers in priority order (the CONFIRMED, CORRECTED
// formula — AMEND-P4), and delegates to a per-tier child loadBalancer built
// by the EXISTING policy factory (buildLeafLB) — the phase-52 localityWeightedLB
// structural precedent, generalized from a flat weighted draw across peer
// localities to a CASCADING WATERFALL across ordered tiers. A single
// cluster-wide capacity-SHORTFALL condition (AMEND-P1 — the phase's single
// most consequential finding, REFUTING both BRAINSTORM panic hypotheses)
// bypasses the entire mechanism, falling back to a flat, health-ignoring,
// host-count-uniform draw reusing the EXISTING lb_healthy_panic stat.
//
// UNLIKE localityWeightedLB, each tier's own child is built against a
// PANIC-DISABLED health view (tierHealth, AMEND-P1-COROLLARY) — a genuinely
// new implementation primitive: the reference never lets a single degraded
// tier internally flatten itself; only the ONE cluster-wide capacity check
// governs any bypass at all.
package cluster

import "math"

// tierHealth returns a clusterHealth VIEW over the SAME per-host state as
// shared (per-host available()/isHealthy() honor live health-check results
// identically, since states is a Go map — a reference type — shared, not
// copied) but with panic PERMANENTLY DISABLED (panicThreshold: 0, so
// inPanic can never fire — a fraction is never strictly < 0). AMEND-P1
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
	return &clusterHealth{states: shared.states, panicThreshold: 0, nowNanos: shared.nowNanos}
}

// tierCapacity computes one tier's effective capacity AS A PERCENT (0-100):
// min(100, overprovisioningFactor × healthyFraction) — the SAME primitive as
// locality.go's effectiveWeight (locality.go:117-125), expressed as an
// absolute percent rather than a weight-scaled float (priority tiers carry no
// per-tier configured weight, only order — AMEND-P4).
func tierCapacity(healthyFraction float64, overprovisioningFactor uint32) float64 {
	return math.Min(100, float64(overprovisioningFactor)*healthyFraction)
}

// cascadeLoads computes (a) the AMEND-P1 bypass quantity — capacitySum, the
// UNCAPPED sum of every tier's own tierCapacity, governing the single
// cluster-wide bypass check (Pick, Task 5) — and (b) the AMEND-P4 CORRECTED
// per-tier assigned loads: each tier's load is its OWN capacity capped by
// whatever budget remains after higher tiers (NOT capacity scaled by a
// fraction of the remaining budget — the two readings coincide at 2 tiers
// but diverge at 3+, confirmed by a decisive 3-tier live probe, SPEC §11.4).
func cascadeLoads(capacities []float64) (loads []float64, capacitySum float64) {
	loads = make([]float64, len(capacities))
	remaining := 100.0
	for i, c := range capacities {
		capacitySum += c
		load := math.Min(c, remaining)
		loads[i] = load
		remaining -= load
	}
	return loads, capacitySum
}
```

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run 'TestTierCapacity_|TestCascadeLoads_|TestTierHealth_' ./... -v 2>&1 | tail -40
go build ./... && echo BUILD_OK
```
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/priority.go internal/cluster/priority_test.go
git commit -m "phase 53 Task 3: priority.go — tierHealth (AMEND-P1-COROLLARY panic-disabled view) + tierCapacity (AMEND-P4 per-tier formula) + cascadeLoads (AMEND-P4 corrected waterfall + AMEND-P1 capacity-sum bypass quantity), all pure and directly unit-tested against the confirmed SPEC §11.1/§11.4 numbers"
```

---

## Task 4: `internal/cluster/priority.go` — `priorityGroup`/`priorityLB` construction + grouping (TDD, part B)

**Goal:** Add `priorityGroup`, `priorityLB`, `priorityLeafFactory` (the D-P-FACTORY local-type resolution), `newPriorityLBWithRNG` (the injectable constructor — groups endpoints by `Priority` value with duplicate-tier MERGE per D-P-DUP, sorts tiers ASCENDING, builds one child per tier via a SHARED `tierHealth` view + one `flat` fallback child built with a NIL health registry), and `newPriorityLB` (the crypto-seeded production constructor). A PLACEHOLDER `Pick` (delegates unconditionally to `flat`) satisfies the `loadBalancer` interface so this task's tests can run against a real `*priorityLB`; Task 5 replaces it with the real bypass+cascade logic.

**Files:**
- Modify: `internal/cluster/priority.go`
- Modify: `internal/cluster/priority_test.go`

- [ ] **Step 1: Write the failing tests**

```go
// trackingPriorityFactory returns a priorityLeafFactory that builds ONE
// stubLB per distinct call, recording each call's endpoint sub-slice (by
// port-sum fingerprint) AND the health VIEW it was given (by pointer), so
// tests can assert what newPriorityLB built and which health view each tier
// vs. the flat child received.
type priorityFactoryCall struct {
	n      int
	sum    uint32
	health *clusterHealth
}

func trackingPriorityFactory() (priorityLeafFactory, *[]priorityFactoryCall) {
	var calls []priorityFactoryCall
	f := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		var sum uint32
		for _, ep := range sub {
			sum += ep.Port
		}
		calls = append(calls, priorityFactoryCall{n: len(sub), sum: sum, health: h})
		return &stubLB{ep: Endpoint{Host: "child", Port: sum}}, nil
	}
	return f, &calls
}

func TestNewPriorityLB_GroupsByPriority(t *testing.T) {
	tier0 := []Endpoint{
		{Host: "p0a", Port: 1, Priority: 0},
		{Host: "p0b", Port: 2, Priority: 0},
	}
	tier1 := []Endpoint{{Host: "p1a", Port: 3, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	factory, calls := trackingPriorityFactory()
	shared := newClusterHealth(all, 0.5)
	pr, err := newPriorityLBWithRNG(all, shared, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(pr.groups))
	}
	if pr.groups[0].priority != 0 || pr.groups[1].priority != 1 {
		t.Errorf("groups must be sorted ASCENDING by priority: got [%d, %d]", pr.groups[0].priority, pr.groups[1].priority)
	}
	if len(pr.groups[0].endpoints) != 2 || len(pr.groups[1].endpoints) != 1 {
		t.Errorf("group sizes = [%d, %d], want [2, 1]", len(pr.groups[0].endpoints), len(pr.groups[1].endpoints))
	}
	// factory called 3 times: tier 0 (2 eps), tier 1 (1 ep), flat (3 eps).
	if len(*calls) != 3 {
		t.Fatalf("factory calls = %d, want 3 (2 tiers + 1 flat)", len(*calls))
	}
	var sawFlat bool
	var tierHealthViews []*clusterHealth
	for _, c := range *calls {
		if c.n == 3 {
			sawFlat = true
			if c.health != nil {
				t.Error("the flat child must be built with a NIL health registry (AMEND-P1)")
			}
		} else {
			tierHealthViews = append(tierHealthViews, c.health)
		}
	}
	if !sawFlat {
		t.Errorf("no factory call spanned all 3 endpoints (the flat fallback) — calls: %+v", *calls)
	}
	if len(tierHealthViews) != 2 || tierHealthViews[0] != tierHealthViews[1] {
		t.Error("both tier children must share the SAME tierHealth view instance (ONE shared panic-disabled view, AMEND-P1-COROLLARY)")
	}
	if tierHealthViews[0] == shared {
		t.Error("tier children must NOT receive the raw shared *clusterHealth directly — they must receive tierHealth(shared)'s panic-disabled VIEW")
	}
	if tierHealthViews[0].panicThreshold != 0 {
		t.Errorf("tier children's health view panicThreshold = %v, want 0", tierHealthViews[0].panicThreshold)
	}
}

func TestNewPriorityLB_NilHealth_TierViewAlsoNil(t *testing.T) {
	// A cluster with zero health_checks configured but multi-tier priority
	// still gets a widened, non-nil health at the manager.go call site
	// (D-P-HEALTHALLOC, Task 6) — but this constructor must handle a nil
	// health parameter defensively (unit-test convenience + defense in depth).
	eps := []Endpoint{{Host: "p0a", Port: 1, Priority: 0}, {Host: "p1a", Port: 2, Priority: 1}}
	factory, calls := trackingPriorityFactory()
	_, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range *calls {
		if c.health != nil {
			t.Errorf("with a nil shared health, every factory call (tiers AND flat) must receive nil, got %+v", c)
		}
	}
}

func TestNewPriorityLB_DuplicatePriority_EndpointsMerge(t *testing.T) {
	// D-P-DUP: two endpoints sharing the SAME Priority value (simulating two
	// source LocalityLbEndpoints groups that collapsed to the same tier in
	// extractEndpoints's output) — SIMPLER than locality's D-LW-DUP (no
	// per-group scalar to conflict over): both endpoints simply MERGE into
	// the SAME priorityGroup.
	eps := []Endpoint{
		{Host: "h1", Port: 1, Priority: 0},
		{Host: "h2", Port: 2, Priority: 0},
	}
	factory, _ := trackingPriorityFactory()
	pr, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.groups) != 1 {
		t.Fatalf("groups = %d, want 1 (both endpoints share Priority 0)", len(pr.groups))
	}
	if len(pr.groups[0].endpoints) != 2 {
		t.Errorf("endpoints = %d, want 2 (both merge into the same tier)", len(pr.groups[0].endpoints))
	}
}

func TestNewPriorityLB_SortsAscending_OutOfOrderInput(t *testing.T) {
	// Endpoints arrive with tiers out of numeric order (2, 0, 1) — groups
	// must still come out ASCENDING (cascade order is load-bearing, Task 5).
	eps := []Endpoint{
		{Host: "p2", Port: 1, Priority: 2},
		{Host: "p0", Port: 2, Priority: 0},
		{Host: "p1", Port: 3, Priority: 1},
	}
	factory, _ := trackingPriorityFactory()
	pr, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.groups) != 3 {
		t.Fatalf("groups = %d, want 3", len(pr.groups))
	}
	for i, want := range []uint32{0, 1, 2} {
		if pr.groups[i].priority != want {
			t.Errorf("groups[%d].priority = %d, want %d (ascending)", i, pr.groups[i].priority, want)
		}
	}
}

func TestNewPriorityLB_FactoryErrorPropagates(t *testing.T) {
	eps := []Endpoint{{Host: "a0", Port: 1, Priority: 0}}
	wantErr := errNoEndpoints
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) { return nil, wantErr }
	if _, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 }); err != wantErr {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestNewPriorityLB_OverprovisioningFactor_DefaultsOnAbsent(t *testing.T) {
	factory, _ := trackingPriorityFactory()
	pr, err := newPriorityLBWithRNG([]Endpoint{{Host: "a", Port: 1}}, nil, 0, false /* hasOPF */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if pr.overprovisioningFactor != defaultOverprovisioningFactor {
		t.Errorf("overprovisioningFactor = %d, want %d (absent → default)", pr.overprovisioningFactor, defaultOverprovisioningFactor)
	}
}

func TestNewPriorityLB_OverprovisioningFactor_HonorsExplicitZero(t *testing.T) {
	factory, _ := trackingPriorityFactory()
	pr, err := newPriorityLBWithRNG([]Endpoint{{Host: "a", Port: 1}}, nil, 0, true /* hasOPF, explicit zero */, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if pr.overprovisioningFactor != 0 {
		t.Errorf("overprovisioningFactor = %d, want 0 (explicit zero honored, not defaulted)", pr.overprovisioningFactor)
	}
}
```
`stubLB`/`errNoEndpoints`/`defaultOverprovisioningFactor` are already defined (`cluster_test.go:447-456`, `cluster.go:30`, `locality.go:39` respectively — reused verbatim, same package, no new declaration needed; `defaultOverprovisioningFactor` in particular is the SAME package-level constant `locality.go` already defines, per SPEC §3.1's own note "reused verbatim, not redefined").

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestNewPriorityLB_' ./... 2>&1 | head -30
```
Expected: COMPILE FAILURE (`priorityGroup`/`priorityLB`/`priorityLeafFactory`/`newPriorityLBWithRNG` undefined).

- [ ] **Step 3: Append the construction types + constructors to `priority.go`**

First, widen `priority.go`'s Task-3 import line (`import "math"`) to a block adding `"sort"` (used by `newPriorityLBWithRNG`'s ascending tier sort, below — `math` alone is no longer sufficient):

```go
import (
	"math"
	"sort"
)
```

Then append:

```go
// priorityGroup is one distinct priority tier's endpoint sub-slice + its
// factory-built child loadBalancer (built against a PANIC-DISABLED health
// view — AMEND-P1-COROLLARY, tierHealth above).
type priorityGroup struct {
	priority  uint32
	endpoints []Endpoint // this tier's own sub-slice, for per-tier health aggregation
	child     loadBalancer
}

// priorityLeafFactory is a LOCAL variation on subset.go's leafFactory
// (subset.go:131 — type leafFactory func(sub []Endpoint) (loadBalancer,
// error), reused as-is by locality.go): it accepts the health VIEW to build
// the leaf against as an explicit parameter, rather than a value baked into
// the closure at the manager.go call site — required so newPriorityLB can
// supply the SAME tierHealth(health) view to every tier child and nil to the
// flat child from the SAME factory (D-P-FACTORY; manager.go's
// wrap-after-switch site defines the closure as func(sub []Endpoint, h
// *clusterHealth) (loadBalancer, error) { return buildLeafLB(c, name, sub, h) }
// — h is now a parameter, not captured — confirmed to compile verbatim
// against buildLeafLB's actual 4-parameter signature, Task 6).
//
// This is a PHASE-53-LOCAL type, distinct from subset.go's leafFactory —
// subset.go/locality.go's call sites and the shared leafFactory type stay
// completely untouched (D-P-FACTORY).
type priorityLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)

// priorityLB is a per-cluster WRAPPER load balancer (ADR-0270). Built ONCE at
// cluster construction when the cluster's endpoints span more than one
// distinct Priority value (tier membership never changes post-boot — no
// dynamic EDS project-wide); Pick recomputes each tier's EFFECTIVE capacity
// (health-derived) on every call, since health state changes live (the
// ADR-0243/locality.go precedent).
//
// REUSES the loadBalancer interface UNCHANGED — ZERO new Pick parameters:
// hashKey/match pass straight through to the chosen tier's child, exactly as
// localityWeightedLB already forwards to ITS children (locality.go:142-174).
type priorityLB struct {
	groups                 []priorityGroup // sorted ASCENDING by priority (0 first) — order is load-bearing for the cascade
	flat                   loadBalancer    // the AMEND-P1 capacity-shortfall fallback; built with a NIL health registry (see tierHealth's doc) — spans ALL endpoints, ALL tiers, ignoring health entirely
	allEndpoints           []Endpoint
	health                 *clusterHealth // the REAL, shared cluster health registry — used ONLY for computing each tier's healthy fraction and for panicInc() on bypass; never passed to children directly (tierHealth wraps it first)
	overprovisioningFactor uint32
	rng                    func() uint64 // the newPCGRNG (leastrequest.go:66-81) idiom — injectable for deterministic tests
}

var _ loadBalancer = (*priorityLB)(nil)

// newPriorityLB is the production constructor: seeds a crypto-keyed PCG
// (newPCGRNG, leastrequest.go:66-81 — the locality.go/random.go
// crypto-constructor/injectable-constructor split) then delegates to
// newPriorityLBWithRNG.
func newPriorityLB(endpoints []Endpoint, health *clusterHealth, opf uint32, hasOPF bool, factory priorityLeafFactory) (*priorityLB, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newPriorityLBWithRNG(endpoints, health, opf, hasOPF, factory, rng)
}

// newPriorityLBWithRNG is the injectable constructor used by unit tests to
// supply a deterministic draw sequence (the newRandomWithRNG/
// newLeastRequestWithRNG/newLocalityWeightedLBWithRNG precedent). It groups
// endpoints by Priority value (a duplicate priority across multiple source
// LocalityLbEndpoints groups MERGES into the SAME tier — D-P-DUP, simpler
// than locality's D-LW-DUP since priority carries no per-group scalar to
// conflict over), sorts tiers ASCENDING (cascade order is load-bearing),
// builds ONE shared tierHealth view (AMEND-P1-COROLLARY — every tier child
// gets the IDENTICAL panic-disabled view, never a per-tier copy) and one
// child per tier via the caller-bound factory, plus one flat fallback child
// built with health=nil (AMEND-P1 — the flat child MUST ignore health
// entirely; see tierHealth's doc and priorityLB's own field doc for the
// divergence-risk rationale versus locality.go's flat child), and resolves
// the overprovisioning_factor absent/explicit-zero distinction (D-LW-OPF0's
// exact pattern, reused verbatim for the SAME proto field).
func newPriorityLBWithRNG(endpoints []Endpoint, health *clusterHealth, opf uint32, hasOPF bool, factory priorityLeafFactory, rng func() uint64) (*priorityLB, error) {
	overprovisioningFactor := opf
	if !hasOPF {
		overprovisioningFactor = defaultOverprovisioningFactor // locality.go:39 — the SAME package-level constant, reused verbatim (not redefined)
	}
	byPriority := map[uint32][]Endpoint{}
	var tiers []uint32
	for _, ep := range endpoints {
		if _, seen := byPriority[ep.Priority]; !seen {
			tiers = append(tiers, ep.Priority)
		}
		byPriority[ep.Priority] = append(byPriority[ep.Priority], ep) // merge on duplicate priority (D-P-DUP)
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i] < tiers[j] }) // ASCENDING — cascade order
	p := &priorityLB{allEndpoints: endpoints, health: health, overprovisioningFactor: overprovisioningFactor, rng: rng}
	tierView := health
	if health != nil {
		tierView = tierHealth(health) // AMEND-P1-COROLLARY: ONE shared panic-disabled view for every tier child
	}
	for _, pr := range tiers {
		members := byPriority[pr]
		child, err := factory(members, tierView)
		if err != nil {
			return nil, err
		}
		p.groups = append(p.groups, priorityGroup{priority: pr, endpoints: members, child: child})
	}
	flat, err := factory(endpoints, nil) // AMEND-P1: health=nil — always ignores health entirely
	if err != nil {
		return nil, err
	}
	p.flat = flat
	return p, nil
}

// Pick is completed in Task 5 (the AMEND-P1 capacity-sum bypass + the
// AMEND-P4 corrected cascade draw + delegation). This placeholder
// unconditionally delegates to the flat fallback so the type satisfies
// loadBalancer and this task's construction/grouping tests can run against a
// real *priorityLB.
func (p *priorityLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) {
	return p.flat.Pick(hashKey, hasHash, match, hasMatch)
}
```

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run 'TestNewPriorityLB_' ./... -v 2>&1 | tail -40
go build ./... && echo BUILD_OK
```
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/priority.go internal/cluster/priority_test.go
git commit -m "phase 53 Task 4: priority.go — priorityGroup/priorityLB/priorityLeafFactory + newPriorityLB/-WithRNG construction (D-P-FACTORY local-type resolution; D-P-DUP merge-not-conflict; ASCENDING tier sort; ONE shared tierHealth view per tier + nil-health flat child); a placeholder flat-only Pick (Task 5 completes it)"
```

---

## Task 5: `priorityLB.Pick` — the AMEND-P1 bypass check + the AMEND-P4 corrected cascade draw + delegation (TDD, part C)

**Goal:** Replace Task 4's placeholder `Pick` with the real implementation: compute every tier's effective capacity + the capacity-sum bypass quantity + the corrected per-tier loads (via `tierCapacity`/`cascadeLoads`, Task 3) in one pass; if the capacity sum is strictly below 100, delegate to `flat` (incrementing the shared `lb_healthy_panic` counter); otherwise weighted-random-draw a tier by its assigned load (a float64 cumulative-bucket walk — the `locality.go` idiom) and delegate, forwarding `(hashKey, hasHash, match, hasMatch)` UNCHANGED.

**Files:**
- Modify: `internal/cluster/priority.go` (replace the placeholder `Pick`)
- Modify: `internal/cluster/priority_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestPick_AllHealthy_TwoTier_AlwaysP0(t *testing.T) {
	// No health (frac defaults to 1.0 for every tier) → capacities [100,100],
	// cascade loads [100,0] → tier 1's bucket has zero width and is excluded
	// from the walk entirely — deterministically ALWAYS tier 0, regardless of
	// rng. This is arm (a)'s exact mechanism (SPEC §8.1).
	tier0 := []Endpoint{{Host: "p0", Port: 1, Priority: 0}}
	tier1 := []Endpoint{{Host: "p1", Port: 2, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	stubs := map[uint32]*stubLB{}
	var flat *stubLB
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		if len(sub) == len(all) {
			flat = &stubLB{}
			return flat, nil
		}
		s := &stubLB{}
		stubs[sub[0].Priority] = s
		return s, nil
	}
	pr, err := newPriorityLBWithRNG(all, nil, 140, true, factory, func() uint64 { return math.MaxUint64 }) // rng pinned to the MAX draw — still must land in tier 0 since tier 1's bucket is empty
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if _, _, err := pr.Pick(0, false, SubsetMatch{}, false); err != nil {
			t.Fatal(err)
		}
	}
	if stubs[0].active.Load() != 5 {
		t.Errorf("tier 0 must be picked every time; active = %d", stubs[0].active.Load())
	}
	if stubs[1].active.Load() != 0 {
		t.Errorf("tier 1 (zero remaining budget) must NEVER be picked; active = %d", stubs[1].active.Load())
	}
	if flat.active.Load() != 0 {
		t.Errorf("flat must not fire when capacitySum >= 100; active = %d", flat.active.Load())
	}
}

func TestPick_ExactBoundary100_NoBypass_AllToP1(t *testing.T) {
	// SPEC §11.1/§11.4 scenario f: tier 0 FULLY unhealthy (0%), tier 1 fully
	// healthy. capacitySum == EXACTLY 100 — confirmed NOT a bypass (strict <).
	// Cascade loads [0, 100] → tier 0's bucket is empty → always tier 1.
	tier0 := []Endpoint{{Host: "p0", Port: 1, Priority: 0}}
	tier1 := []Endpoint{{Host: "p1", Port: 2, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	health := newClusterHealth(all, 0.5)
	health.states["p0:1"].healthy.Store(false)
	stubs := map[uint32]*stubLB{}
	var flat *stubLB
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		if len(sub) == len(all) {
			flat = &stubLB{}
			return flat, nil
		}
		s := &stubLB{}
		stubs[sub[0].Priority] = s
		return s, nil
	}
	pr, err := newPriorityLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pr.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if stubs[1].active.Load() != 1 {
		t.Error("the exactly-100 boundary must NOT bypass — tier 1 must be picked (all-healthy tier)")
	}
	if flat.active.Load() != 0 {
		t.Error("the exactly-100 boundary must NOT delegate to flat")
	}
	if stubs[0].active.Load() != 0 {
		t.Error("tier 0 (0% healthy, zero remaining budget) must never be picked")
	}
}

func TestPick_CapacityShortfall_BypassesToFlat(t *testing.T) {
	// SPEC §11.1 scenario g: BOTH tiers at 20% healthy (5 hosts each, 1
	// healthy). capacitySum = 28+28 = 56 < 100 → the AMEND-P1 bypass fires.
	mkTier := func(pfx string, priority uint32, healthyCount int) []Endpoint {
		eps := make([]Endpoint, 5)
		for i := range eps {
			eps[i] = Endpoint{Host: fmt.Sprintf("%s%d", pfx, i), Port: uint32(i + 1), Priority: priority}
		}
		return eps
	}
	tier0 := mkTier("t0h", 0, 1)
	tier1 := mkTier("t1h", 1, 1)
	all := append(append([]Endpoint{}, tier0...), tier1...)
	health := newClusterHealth(all, 0.5)
	for _, ep := range tier0[1:] {
		health.states[ep.Addr()].healthy.Store(false)
	}
	for _, ep := range tier1[1:] {
		health.states[ep.Addr()].healthy.Store(false)
	}
	stubs := map[uint32]*stubLB{}
	var flat *stubLB
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		if len(sub) == len(all) {
			flat = &stubLB{}
			return flat, nil
		}
		s := &stubLB{}
		stubs[sub[0].Priority] = s
		return s, nil
	}
	pr, err := newPriorityLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pr.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if flat.active.Load() != 1 {
		t.Errorf("capacitySum=56<100 must bypass to flat; flat active = %d", flat.active.Load())
	}
	if stubs[0].active.Load() != 0 || stubs[1].active.Load() != 0 {
		t.Error("bypass must NOT delegate to either tier's own child")
	}
}

func TestPick_CapacityShortfall_IncrementsPanicCounter(t *testing.T) {
	// The bypass reuses the EXISTING lb_healthy_panic counter (SPEC §7/§11.5
	// — no new stat). Verified via a real *stats.Registry-backed Counter
	// (Counter.Load(), not a Registry-wide snapshot — Registry exposes no
	// such method; Walk(fn) is the only bulk accessor and is overkill for a
	// single known handle).
	reg := stats.NewRegistry()
	tier0 := []Endpoint{{Host: "p0", Port: 1, Priority: 0}}
	tier1 := []Endpoint{{Host: "p1", Port: 2, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	health := newClusterHealth(all, 0.5)
	panicCounter := reg.NewCounter("test.lb_healthy_panic")
	health.panicCounter = panicCounter
	health.states["p0:1"].healthy.Store(false)
	health.states["p1:2"].healthy.Store(false) // both 0% healthy → capacitySum = 0 < 100
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) { return &stubLB{}, nil }
	pr, err := newPriorityLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := pr.Pick(0, false, SubsetMatch{}, false); err != nil {
		t.Fatal(err)
	}
	if got := panicCounter.Load(); got != 1 {
		t.Errorf("lb_healthy_panic = %v, want 1", got)
	}
}

func TestPick_PerTierChild_NeverSpraysUnhealthyHosts(t *testing.T) {
	// AMEND-P1-COROLLARY, the Pick-level structural proof: tier 0 at 20%
	// healthy (1/5) built with a REAL roundRobin child over tierHealth(health)
	// — the confirmed reference property is that a degraded tier NEVER
	// internally flattens: its 4 unhealthy hosts get ZERO traffic, its 1
	// healthy host gets ALL of the tier's share. Tier 1 stays fully healthy
	// (capacitySum = 28+100 = 128 >= 100 — no bypass, matching SPEC §11.1
	// scenario e). rng is pinned so the cascade draw always lands in tier 0's
	// bucket (r < 28).
	tier0 := make([]Endpoint, 5)
	for i := range tier0 {
		tier0[i] = Endpoint{Host: fmt.Sprintf("t0h%d", i), Port: uint32(i + 1), Priority: 0}
	}
	tier1 := []Endpoint{{Host: "t1h0", Port: 100, Priority: 1}}
	all := append(append([]Endpoint{}, tier0...), tier1...)
	health := newClusterHealth(all, 0.5)
	for _, ep := range tier0[1:] {
		health.states[ep.Addr()].healthy.Store(false) // 4 of 5 unhealthy → 20% healthy
	}
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		return &roundRobin{endpoints: sub, health: h}, nil
	}
	pr, err := newPriorityLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 }) // r == 0 → always the first (lowest-cum) bucket, tier 0
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for i := 0; i < 50; i++ {
		ep, release, err := pr.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatal(err)
		}
		release()
		seen[ep.Host]++
	}
	if seen["t0h0"] != 50 {
		t.Errorf("the ONE healthy host in tier 0 must receive ALL 50 picks; seen = %+v", seen)
	}
	for _, unhealthy := range []string{"t0h1", "t0h2", "t0h3", "t0h4"} {
		if seen[unhealthy] != 0 {
			t.Errorf("unhealthy host %q must receive ZERO traffic (AMEND-P1-COROLLARY — no per-tier local panic spray); got %d", unhealthy, seen[unhealthy])
		}
	}
}

func TestPriorityLBPick_ForwardsHashKeyAndMatchUnchanged(t *testing.T) {
	// D-... / SPEC §3.2: hashKey/match pass straight through to the chosen child.
	eps := []Endpoint{{Host: "p0", Port: 1, Priority: 0}}
	var gotHashKey uint64
	var gotHasHash bool
	var gotMatch SubsetMatch
	var gotHasMatch bool
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		return &argRecordingLB{onPick: func(hk uint64, hh bool, m SubsetMatch, hm bool) {
			gotHashKey, gotHasHash, gotMatch, gotHasMatch = hk, hh, m, hm
		}}, nil
	}
	pr, err := newPriorityLBWithRNG(eps, nil, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	wantMatch := NewSubsetMatch(map[string]SubsetValue{"k": {Kind: subsetString, Str: "v"}})
	if _, _, err := pr.Pick(42, true, wantMatch, true); err != nil {
		t.Fatal(err)
	}
	if gotHashKey != 42 || !gotHasHash || gotMatch.Key() != wantMatch.Key() || !gotHasMatch {
		t.Errorf("forwarded (hashKey=%d, hasHash=%v, match=%v, hasMatch=%v), want (42, true, %v, true)", gotHashKey, gotHasHash, gotMatch, gotHasMatch, wantMatch)
	}
}
```
`argRecordingLB` is already defined by phase 52's `locality_test.go` (same package — reused verbatim; NOT named `recordingLB` — that name is already taken by a differently-shaped pre-existing type in `h2pool_test.go`, so `locality.go`'s own PLAN named this one `argRecordingLB` to avoid the collision — confirmed by reading the actual file, not assumed). The test function is named `TestPriorityLBPick_ForwardsHashKeyAndMatchUnchanged` (NOT `TestPick_ForwardsHashKeyAndMatchUnchanged` — that exact name is already taken by `locality_test.go`'s own equivalent test, same package). `fmt`/`math`/`stats` must be added to `priority_test.go`'s import block (`import ("fmt"; "math"; "testing"; "github.com/pgdad/envoy-go/internal/stats")`).

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestPick_' ./... 2>&1 | head -30
```
Expected: FAIL (the placeholder `Pick` always delegates to `flat`, so every non-bypass test fails; the bypass tests coincidentally may look like they pass since flat IS what placeholder always calls, but `TestPick_AllHealthy_TwoTier_AlwaysP0`/`TestPick_ExactBoundary100_NoBypass_AllToP1`/`TestPick_PerTierChild_NeverSpraysUnhealthyHosts`/`TestPriorityLBPick_ForwardsHashKeyAndMatchUnchanged` all fail against the placeholder).

- [ ] **Step 3: Replace the placeholder `Pick`**

```go
// Pick: computes every tier's effective capacity (tierCapacity) and the
// AMEND-P1 capacity-sum bypass quantity + the AMEND-P4 corrected per-tier
// assigned loads (cascadeLoads) in one pass. If capacitySum < 100 (strict —
// AMEND-P1's confirmed boundary: exactly 100 does NOT bypass), delegate to
// the flat child (health-ignoring, host-count-uniform), incrementing the
// EXISTING lb_healthy_panic counter (the reference reuses this SAME stat for
// this mechanism, §11.1/§11.5 — no new counter). Otherwise weighted-random-
// draw a tier by its assigned load (a float64 cumulative-bucket walk, the
// locality.go idiom) and delegate child.Pick(hashKey, hasHash, match,
// hasMatch) UNCHANGED — the identical forwarding shape localityWeightedLB.Pick
// already uses (locality.go:142-174).
func (p *priorityLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) {
	capacities := make([]float64, len(p.groups))
	for i, g := range p.groups {
		frac := 1.0
		if p.health != nil && len(g.endpoints) > 0 {
			frac = float64(p.health.availableCount(g.endpoints)) / float64(len(g.endpoints))
		}
		capacities[i] = tierCapacity(frac, p.overprovisioningFactor)
	}
	loads, capacitySum := cascadeLoads(capacities)
	if capacitySum < 100 {
		if p.health != nil {
			p.health.panicInc()
		}
		return p.flat.Pick(hashKey, hasHash, match, hasMatch)
	}
	type bucket struct {
		cum   float64
		child loadBalancer
	}
	buckets := make([]bucket, 0, len(p.groups))
	var cum float64
	for i, g := range p.groups {
		if loads[i] <= 0 {
			continue
		}
		cum += loads[i]
		buckets = append(buckets, bucket{cum: cum, child: g.child})
	}
	if len(buckets) == 0 { // defensive: capacitySum>=100 guarantees at least one positive load; kept for safety
		return p.flat.Pick(hashKey, hasHash, match, hasMatch)
	}
	r := (float64(p.rng()) / float64(math.MaxUint64)) * cum
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
cd internal/cluster && go test -run 'TestPick_|TestNewPriorityLB_|TestTierCapacity_|TestCascadeLoads_|TestTierHealth_' ./... -v 2>&1 | tail -50
go vet ./... && go build ./... && echo OK
```
Expected: all PASS.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/priority.go internal/cluster/priority_test.go
git commit -m "phase 53 Task 5: priorityLB.Pick — the AMEND-P1 capacity-sum bypass (strict <100, delegates to flat + panicInc) + the AMEND-P4 corrected cascade draw + the AMEND-P1-COROLLARY per-tier no-local-panic property (a degraded tier's own child never sprays its unhealthy hosts); hashKey/match forwarding unchanged"
```

---

## Task 6: `manager.go` wrap-after-switch — `distinctPriorities` + the two reject arms + the wrap + the health-registry-widening + the `overprovisioning_factor` read

**Goal:** `buildCluster` gains a THIRD wrap-after-switch site, immediately after the EXISTING (phase-52) `locality_weighted_lb_config` switch closes: a small `distinctPriorities` helper, then a `switch` covering the two new composition rejects + the `priorityLB` wrap (which allocates `clusterHealth` unconditionally when absent — the SECOND health-registry-widening, D-P-HEALTHALLOC — and reads `ClusterLoadAssignment.Policy.overprovisioning_factor`'s presence/value, reusing `la` which is already in scope).

**Files:**
- Modify: `internal/cluster/manager.go` (`buildCluster`, inserting between the existing switch's closing `}` and `cl.lb = lb`)
- Modify: `internal/cluster/manager_test.go`

- [ ] **Step 1: Write the failing tests**

```go
func TestManager_Reject_PriorityWithLocalityWeighted(t *testing.T) {
	c := mkStaticClusterFromGroups("c_pri_lw",
		&endpointv3.LocalityLbEndpoints{Priority: 0, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001)}},
		&endpointv3.LocalityLbEndpoints{Priority: 1, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9002)}},
	)
	c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{
		LocalityConfigSpecifier: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig_{
			LocalityWeightedLbConfig: &clusterv3.Cluster_CommonLbConfig_LocalityWeightedLbConfig{},
		},
	}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	want := "common_lb_config.locality_weighted_lb_config cannot be combined with multi-tier LocalityLbEndpoints.priority"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want the priority-vs-locality-weighted reject", err)
	}
}

func TestManager_Reject_PriorityWithLbSubsetConfig(t *testing.T) {
	c := mkStaticClusterFromGroups("c_pri_sc",
		&endpointv3.LocalityLbEndpoints{Priority: 0, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001)}},
		&endpointv3.LocalityLbEndpoints{Priority: 1, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9002)}},
	)
	c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{FallbackPolicy: clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	want := "lb_subset_config cannot be combined with multi-tier LocalityLbEndpoints.priority"
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("err = %v, want the priority-vs-subset reject", err)
	}
}

func TestManager_Accept_MultiTierPriority_WrapsChild(t *testing.T) {
	c := mkStaticClusterFromGroups("c_pri",
		&endpointv3.LocalityLbEndpoints{Priority: 0, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001)}},
		&endpointv3.LocalityLbEndpoints{Priority: 1, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9002)}},
	)
	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("multi-tier priority under ROUND_ROBIN must be accepted: %v", err)
	}
	cl, _ := mgr.Get("c_pri")
	if _, ok := cl.lb.(*priorityLB); !ok {
		t.Errorf("lb must be wrapped in *priorityLB, got %T", cl.lb)
	}
}

func TestManager_SingleTierPriority_NoWrap(t *testing.T) {
	// The overwhelming common case: every endpoint at the SAME priority
	// (including entirely absent, defaulting to 0) — NOT wrapped (SPEC §6.3).
	c := mkStaticCluster("c_notiered", mkLbEndpoint("127.0.0.1", 9001))
	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	cl, _ := mgr.Get("c_notiered")
	if _, ok := cl.lb.(*priorityLB); ok {
		t.Error("a single-tier cluster must NOT be wrapped in *priorityLB")
	}
}

func TestManager_MultipleGroupsSamePriority_NoWrap(t *testing.T) {
	// D-P-DUP's non-reject sibling (SPEC §6.3): multiple LocalityLbEndpoints
	// groups declaring the SAME priority value merge into one tier — still
	// single-tier overall, still NOT wrapped.
	c := mkStaticClusterFromGroups("c_dup0",
		&endpointv3.LocalityLbEndpoints{Priority: 0, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001)}},
		&endpointv3.LocalityLbEndpoints{Priority: 0, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9002)}},
	)
	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	cl, _ := mgr.Get("c_dup0")
	if _, ok := cl.lb.(*priorityLB); ok {
		t.Error("two groups sharing the SAME priority must merge into one tier — still NOT wrapped")
	}
}

func TestManager_Priority_WidensHealthWithoutHealthChecks(t *testing.T) {
	// D-P-HEALTHALLOC: no health_checks configured, yet cl.health must be
	// non-nil once multi-tier priority is present, and registerClusterMetrics
	// must ALSO inject membership_healthy/lb_healthy_panic (the EXISTING
	// unconditional `if c.health != nil` block, manager.go:163-170).
	c := mkStaticClusterFromGroups("c_pri_nohc",
		&endpointv3.LocalityLbEndpoints{Priority: 0, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001)}},
		&endpointv3.LocalityLbEndpoints{Priority: 1, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9002)}},
	)
	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cl, _ := mgr.Get("c_pri_nohc")
	if cl.health == nil {
		t.Fatal("cl.health must be non-nil (the health-registry-widening) even with zero health_checks")
	}
	if cl.health.membershipHealthy == nil {
		t.Error("membership_healthy must be registered even though no health_checks are configured")
	}
	if cl.health.panicCounter == nil {
		t.Error("lb_healthy_panic must be registered even though no health_checks are configured")
	}
	if len(cl.checkers) != 0 {
		t.Errorf("checkers = %d, want 0 (no health_checks configured — only the registry widened, not the runtime)", len(cl.checkers))
	}
}

func TestManager_Priority_ReadsOverprovisioningFactor(t *testing.T) {
	mk := func(opf *wrapperspb.UInt32Value) *clusterv3.Cluster {
		c := mkStaticClusterFromGroups("c_pri_opf",
			&endpointv3.LocalityLbEndpoints{Priority: 0, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9001)}},
			&endpointv3.LocalityLbEndpoints{Priority: 1, LbEndpoints: []*endpointv3.LbEndpoint{mkLbEndpoint("127.0.0.1", 9002)}},
		)
		c.LoadAssignment.Policy = &endpointv3.ClusterLoadAssignment_Policy{OverprovisioningFactor: opf}
		return c
	}
	// absent → defaults to 140.
	mgr, err := NewManager(mkBootstrap(mk(nil)), stats.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	cl, _ := mgr.Get("c_pri_opf")
	pr := cl.lb.(*priorityLB)
	if pr.overprovisioningFactor != 140 {
		t.Errorf("absent overprovisioning_factor: got %d, want 140", pr.overprovisioningFactor)
	}
	// explicit 100 → honored.
	mgr2, err := NewManager(mkBootstrap(mk(wrapperspb.UInt32(100))), stats.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	cl2, _ := mgr2.Get("c_pri_opf")
	if got := cl2.lb.(*priorityLB).overprovisioningFactor; got != 100 {
		t.Errorf("explicit overprovisioning_factor=100: got %d, want 100", got)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestManager_Reject_Priority|TestManager_Accept_MultiTierPriority|TestManager_SingleTierPriority|TestManager_MultipleGroupsSamePriority|TestManager_Priority_' ./... 2>&1 | head -30
```
Expected: FAIL (the reject strings never fire; `cl.lb` never becomes `*priorityLB`; `priorityLB` type undefined error would NOT occur since Task 4/5 already defined it — this task's failures are purely BEHAVIORAL, not compile errors).

- [ ] **Step 3: Implement the wrap-after-switch** (`manager.go`)

Insert immediately after the EXISTING phase-52 switch's closing `}` (currently `manager.go:508`, right before `cl.lb = lb` / `cl.health = health`, `:509-510`):

```go
	// Phase 53 (ADR-0270): the THIRD wrap-after-switch site. sc/lwc are
	// ALREADY in scope from the two switches above (both declared via := at
	// function scope, never reassigned) — reused directly, not re-derived.
	priorityTiers := distinctPriorities(endpoints)
	switch {
	case len(priorityTiers) > 1 && lwc != nil:
		return nil, fmt.Errorf("cluster: %q: common_lb_config.locality_weighted_lb_config cannot be combined with multi-tier LocalityLbEndpoints.priority", name)
	case len(priorityTiers) > 1 && sc != nil:
		return nil, fmt.Errorf("cluster: %q: lb_subset_config cannot be combined with multi-tier LocalityLbEndpoints.priority", name)
	case len(priorityTiers) > 1:
		// D-P-HEALTHALLOC (the D-LW-HEALTHALLOC precedent reapplied): a
		// priority-tiered cluster ALWAYS needs clusterHealth.availableCount
		// to be CALLABLE, even with zero health_checks configured (a
		// well-defined 100%-healthy-everywhere fast path).
		if health == nil {
			health = newClusterHealth(endpoints, parsePanicThreshold(c))
		}
		// D-LW-OPF0's exact pattern, reused verbatim (NOT re-derived): the
		// wrapper's PRESENCE is checked BEFORE .GetValue() so an explicit
		// {value: 0} is honored literally.
		opfWrapper := la.GetPolicy().GetOverprovisioningFactor()
		var opf uint32
		hasOPF := opfWrapper != nil
		if hasOPF {
			opf = opfWrapper.GetValue()
		}
		pr, err := newPriorityLB(endpoints, health, opf, hasOPF, func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
			return buildLeafLB(c, name, sub, h)
		})
		if err != nil {
			return nil, err
		}
		lb = pr
	}
	cl.lb = lb
	cl.health = health
```

This REPLACES the existing `cl.lb = lb` / `cl.health = health` pair at the end of the phase-52 block (they move to the end of this new switch instead — functionally identical, since the switch's `case`s either return an error or fall through to the SAME two assignment lines).

Then add `distinctPriorities` as a small standalone helper, placed directly above `buildCluster` (`manager.go`, immediately before line 390's `func buildCluster(...)`):

```go
// distinctPriorities returns the set of distinct Endpoint.Priority values
// present in endpoints. Used only for its cardinality — len(...) > 1 gates
// the priorityLB wrap (SPEC §3.3); order is immaterial here (newPriorityLB
// does its own ascending sort at construction, priority.go Task 4).
func distinctPriorities(endpoints []Endpoint) map[uint32]struct{} {
	set := make(map[uint32]struct{}, len(endpoints))
	for _, ep := range endpoints {
		set[ep.Priority] = struct{}{}
	}
	return set
}
```

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test ./... -v 2>&1 | grep -E "FAIL|^--- FAIL"
go test ./... 2>&1 | tail -10
```
Expected: full package PASS (no regressions on the phase-38/52 subset/locality-weighted tests — `sc`/`lwc` are read-only reused, never mutated by the new switch).

- [ ] **Step 5: gofmt + lint + vet + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/... && go vet ./internal/cluster/...
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 53 Task 6: manager.go buildCluster THIRD wrap-after-switch (SPEC §3.3/§6) — distinctPriorities + the locality_weighted_lb_config-combined reject + the lb_subset_config-combined reject + the priorityLB wrap (D-P-HEALTHALLOC health-registry-widening; overprovisioning_factor absent-vs-explicit-zero reused verbatim from D-LW-OPF0); sc/lwc reused from the two prior wrap sites, never re-derived"
```

---

## Task 7: Unit-test depth — the confirmed §11.1/§11.4 data tables (8 two-tier scenarios + 2 three-tier scenarios)

**Goal:** Add the exhaustive numeric-table tests SPEC §10 Task 7 calls for that Tasks 3/5's TDD steps did not already cover in full: EVERY two-tier scenario from SPEC §11.4's table ((a) through (h)) run through the REAL `tierCapacity`→`cascadeLoads` pipeline, plus a documentation note on why the reject-arm tests are NOT duplicated here (already TDD'd at Task 6, the phase-52 Task 5/6 precedent).

**Files:**
- Modify: `internal/cluster/priority_test.go`

- [ ] **Step 1: Write the tests**

```go
func TestPriorityFormula_ConfirmedTwoTierTable(t *testing.T) {
	// SPEC §11.4's full predicted-vs-observed table, scenarios (a)-(h) — this
	// test asserts the EXACT mathematical prediction the CORRECTED formula
	// produces (not the noisy "observed" column, which includes binomial
	// sampling noise at n=300; the differential fixture, Task 9, proves the
	// noisy live-traffic match). tier1 (P1) is ALWAYS 100% healthy except (h).
	const opf = 140
	cases := []struct {
		name        string
		fracP0      float64
		fracP1      float64
		wantCap0    float64
		wantCap1    float64
		wantLoad0   float64
		wantLoad1   float64
		wantSum     float64
	}{
		{"a_baseline", 1.00, 1.00, 100, 100, 100, 0, 200},
		{"b_80pct", 0.80, 1.00, 100, 100, 100, 0, 200}, // capped at 100 despite 0.80*140=112
		{"c_60pct", 0.60, 1.00, 84, 100, 84, 16, 184},
		{"d_40pct", 0.40, 1.00, 56, 100, 56, 44, 156},
		{"e_20pct", 0.20, 1.00, 28, 100, 28, 72, 128},
		{"f_0pct_boundary", 0.00, 1.00, 0, 100, 0, 100, 100}, // exactly 100 — the confirmed boundary
		{"h_20pct_vs_80pct", 0.20, 0.80, 28, 100, 28, 72, 128},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cap0 := tierCapacity(c.fracP0, opf)
			cap1 := tierCapacity(c.fracP1, opf)
			if diff := cap0 - c.wantCap0; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("cap0 = %v, want %v", cap0, c.wantCap0)
			}
			if diff := cap1 - c.wantCap1; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("cap1 = %v, want %v", cap1, c.wantCap1)
			}
			loads, sum := cascadeLoads([]float64{cap0, cap1})
			if diff := loads[0] - c.wantLoad0; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("load0 = %v, want %v", loads[0], c.wantLoad0)
			}
			if diff := loads[1] - c.wantLoad1; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("load1 = %v, want %v", loads[1], c.wantLoad1)
			}
			if diff := sum - c.wantSum; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("capacitySum = %v, want %v", sum, c.wantSum)
			}
		})
	}
}

func TestPriorityFormula_BypassScenario_g(t *testing.T) {
	// SPEC §11.1 scenario (g): BOTH tiers at 20% healthy → capacitySum=56<100
	// — the confirmed AMEND-P1 bypass condition (Pick's own comparison,
	// Task 5, is what actually triggers the bypass; this test isolates the
	// pure-function inputs feeding that comparison).
	cap0 := tierCapacity(0.20, 140)
	cap1 := tierCapacity(0.20, 140)
	_, sum := cascadeLoads([]float64{cap0, cap1})
	if sum != 56 {
		t.Errorf("capacitySum = %v, want 56 (< 100 — triggers the bypass)", sum)
	}
	if !(sum < 100) {
		t.Fatal("test setup invariant broken: sum must be < 100")
	}
}

func TestPriorityFormula_ThreeTier_BothConfirmedScenarios(t *testing.T) {
	// SPEC §11.4's two 3-tier scenarios, run through the REAL
	// tierCapacity→cascadeLoads pipeline end to end (not just cascadeLoads'
	// own unit test at Task 3, which hardcoded the capacities — this test
	// derives them from healthy fractions too, closing the loop).
	cases := []struct {
		name             string
		fracs            []float64
		wantCaps         []float64
		wantLoads        []float64
		wantSum          float64
	}{
		{
			name:      "40_60_100",
			fracs:     []float64{0.40, 0.60, 1.00},
			wantCaps:  []float64{56, 84, 100},
			wantLoads: []float64{56, 44, 0},
			wantSum:   240,
		},
		{
			name:      "20_20_100",
			fracs:     []float64{0.20, 0.20, 1.00},
			wantCaps:  []float64{28, 28, 100},
			wantLoads: []float64{28, 28, 44},
			wantSum:   156,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			caps := make([]float64, len(c.fracs))
			for i, f := range c.fracs {
				caps[i] = tierCapacity(f, 140)
			}
			for i, want := range c.wantCaps {
				if diff := caps[i] - want; diff > 1e-9 || diff < -1e-9 {
					t.Errorf("cap[%d] = %v, want %v", i, caps[i], want)
				}
			}
			loads, sum := cascadeLoads(caps)
			for i, want := range c.wantLoads {
				if diff := loads[i] - want; diff > 1e-9 || diff < -1e-9 {
					t.Errorf("load[%d] = %v, want %v (the CORRECTED cascade — NOT the naive-recursive reading)", i, loads[i], want)
				}
			}
			if diff := sum - c.wantSum; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("capacitySum = %v, want %v", sum, c.wantSum)
			}
		})
	}
}

// NOTE on reject-arm test placement: the both-locality_weighted_lb_config
// and both-lb_subset_config composition rejects (SPEC §6.1) are TDD'd
// INLINE at Task 6 (TestManager_Reject_PriorityWithLocalityWeighted /
// TestManager_Reject_PriorityWithLbSubsetConfig, manager_test.go) — the
// phase-52 Task 5/6 precedent (locality-weighted's own PLAN made the
// identical split). They are NOT duplicated here.
```

- [ ] **Step 2: Run**

```bash
cd internal/cluster && go test -run 'TestPriorityFormula_' ./... -v 2>&1 | tail -60
```
Expected: all PASS.

- [ ] **Step 3: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/priority_test.go
git commit -m "phase 53 Task 7: unit-test depth — the confirmed SPEC §11.4 two-tier table (scenarios a-h) + the §11.1 scenario (g) bypass isolation + both §11.4 three-tier scenarios, run end-to-end through tierCapacity->cascadeLoads (exact, not just the noisy live-probe observed values); reject-arm tests already covered at Task 6"
```

---

## Task 8: `test/fixtures/0096-lb-priority` driver — the 2-tier×5-host topology + the full-failover health-check-toggle harness

**Goal:** Register the `0096-lb-priority` cross-side fixture: cluster `c_pri`, TWO `LocalityLbEndpoints` groups (`priority: 0` × 5 hosts, `priority: 1` × 5 hosts), an active HTTP health checker (`/healthz`, fast convergence). Tier 1's 5 hosts are runner-spawned `HTTPEcho` backends (`BackendCount()==5`, always healthy). Tier 0's 5 hosts are DRIVER-OWNED `toggleResponder`s (the `0095` precedent, reused directly): each answers `200 "tier0:<idx>"` on any data path and `200`/`503` on `/healthz` depending on an `atomic.Bool` the driver flips mid-run — arm (b) flips ALL 5 (not a partial 3-of-5 like `0095`'s degradation shift, since this fixture proves a HARD 100%/0% failover, not a statistical shift).

**Files:**
- Create: `test/fixtures/0096-lb-priority/driver/driver.go`
- Create: `test/fixtures/0096-lb-priority/driver/driver_test.go`
- Create: `test/fixtures/0096-lb-priority/README.md`
- Create: `test/fixtures/0096-lb-priority/expectations.yaml`

- [ ] **Step 1: `driver.go` — topology, toggle responder, bootstrap builders, Drive hooks**

```go
// Package driver registers the 0096-lb-priority cross-side differential
// fixture (phase 53 SPEC §8.1 / PLAN Tasks 8-10).
//
// Cross-side [http_connection_manager + router] fixture over ONE cluster
// c_pri with TWO LocalityLbEndpoints groups at distinct priority values (0
// and 1), 5 hosts each, plus an active HTTP health checker (path /healthz,
// fast convergence).
//
// Tier 1's 5 hosts are runner-spawned HTTPEcho backends (BackendCount()==5,
// always healthy). Tier 0's 5 hosts are DRIVER-OWNED toggleable HTTP
// responders (the 0095-lb-locality-weighted precedent, reused directly):
// each answers 200 "tier0:<idx>" on any data path and 200/503 on /healthz
// depending on an atomic healthy flag the driver flips mid-run.
//
// AssertStats drives BOTH arms in-band (the only hook holding both admin
// addrs):
//
//	arm (a) — static (all 10 hosts healthy): poll membership_healthy==10 on
//	  both sides, warmup, send staticLoadCount requests, assert a HARD
//	  100%/0% split — ALL traffic on tier 0, NONE on tier 1 (capacitySum =
//	  100+100 = 200 >= 100, no bypass; SPEC §8.1/§11.1 scenario (a)).
//	arm (b) — full failover: fail ALL 5 of tier 0's hosts' /healthz, poll
//	  membership_healthy==5 on both sides, re-warmup, send
//	  degradedLoadCount MORE requests, assert the split flips to a HARD
//	  0%/100% — capacitySum = 0+100 = EXACTLY 100, the confirmed boundary
//	  that does NOT trigger the AMEND-P1 capacity-shortfall bypass (SPEC
//	  §11.1 scenario (f)).
//
// Cross-references: phase 53 SPEC §8.1/§11.1/§11.4;
// 0095-lb-locality-weighted (the toggleResponder + poll/warmup harness,
// reused verbatim); reference_health_check_propagation_warmup;
// reference_docker_probe_bridge_network (host.docker.internal addressing);
// reference_differential_run_selector (-run 'TestDifferential/0096');
// reference_fixture_workload_constant_desync;
// reference_differential_asserter_dispatch (StatsAsserter, cross-side);
// reference_differential_fixture_dispatch_constraint (both arms in ONE
// fixture dir — no separate boot-reject dir, SPEC §8.2/AMEND-P2).
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

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0096-lb-priority"

	refContainerListenerPort = 19171
	refAdminPort             = 9901

	// backendCount is the number of runner-spawned HTTPEcho backends — tier
	// 1's 5 ALWAYS-healthy hosts. Tier 0's 5 hosts are driver-owned (below).
	backendCount = 5

	tier0Hosts = 5
	tier1Hosts = 5

	// staticLoadCount / degradedLoadCount are the per-arm request counts —
	// the SPEC §8.1 "≥300 requests" convention, matching the live-probe
	// scenario counts (reference_fixture_workload_constant_desync).
	staticLoadCount   = 300
	degradedLoadCount = 300

	membershipTotal = tier0Hosts + tier1Hosts // 10, unaffected by health

	convergeDeadline = 30 * time.Second
	convergePoll     = 200 * time.Millisecond
	warmupStable     = 10
	warmupDeadline   = 15 * time.Second
)

func init() {
	fixture.RegisterFixture(fixtureName, &priDriver{})
}

// toggleResponder is a driver-owned, self-managed HTTP/1.1 responder for ONE
// tier-0 host: 200 "tier0:<idx>" on any data path; on /healthz, 200 while
// healthy.Load()==true, 503 once SetHealthy(false) has been called (arm
// (b)'s controlled-degradation trigger). The 0095-lb-locality-weighted
// precedent, reused directly (identical shape; only the response-body prefix
// changes: "tier0:" here vs. "region-a:" there).
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
	fmt.Fprintf(w, "tier0:%d", r.idx)
}

func (r *toggleResponder) port() int { return r.ln.Addr().(*net.TCPAddr).Port }

// SetHealthy flips the /healthz response (arm (b)'s controlled-failure trigger).
func (r *toggleResponder) SetHealthy(v bool) { r.healthy.Store(v) }

// priDriver is STATEFUL: it owns the 5 tier-0 toggleResponders (built once,
// memoized) and stashes the per-side listener addrs from the Drive hooks so
// AssertStats — the only hook holding both admin addrs — can run both arms.
type priDriver struct {
	mu           sync.Mutex
	refListener  string
	subjListener string
	tier0        []*toggleResponder
}

func (*priDriver) BackendCount() int                { return backendCount } // tier 1 only
func (*priDriver) BackendKind() fixture.BackendKind { return fixture.HTTPEcho }
func (*priDriver) SubjectListenerName() string      { return "l_http" }
func (*priDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ensureTier0 builds the 5 tier-0 toggle responders exactly once (memoized —
// both ReferenceBootstrap and SubjectConfig call it and MUST agree on the
// SAME 5 ports).
func (d *priDriver) ensureTier0() []*toggleResponder {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.tier0 != nil {
		return d.tier0
	}
	out := make([]*toggleResponder, tier0Hosts)
	for i := range out {
		r, err := newToggleResponder(i)
		if err != nil {
			panic(err)
		}
		out[i] = r
	}
	d.tier0 = out
	return out
}

const healthChecksBlock = `      health_checks:
        - interval: 0.2s
          timeout: 0.2s
          unhealthy_threshold: 1
          healthy_threshold: 1
          http_health_check:
            path: /healthz`

const routeTable = `                      routes:
                        - match: { prefix: "/" }
                          route: { cluster: c_pri }`

// priorityEndpointsBlock renders the two LocalityLbEndpoints groups (distinct
// priority values 0 and 1) for the given host addressing scheme over the SAME
// 10 ports (5 tier-0 toggleResponder ports + 5 tier-1 runner backend ports).
func priorityEndpointsBlock(addr string, tier0Ports, tier1Ports []int) string {
	var b strings.Builder
	b.WriteString("      load_assignment:\n        cluster_name: c_pri\n        endpoints:\n")
	b.WriteString("          - priority: 0\n            lb_endpoints:\n")
	for _, p := range tier0Ports {
		fmt.Fprintf(&b, "              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }\n", addr, p)
	}
	b.WriteString("          - priority: 1\n            lb_endpoints:\n")
	for _, p := range tier1Ports {
		fmt.Fprintf(&b, "              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }\n", addr, p)
	}
	return b.String()
}

func (d *priDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	tier0 := d.ensureTier0()
	t0Ports := make([]int, tier0Hosts)
	for i, r := range tier0 {
		t0Ports[i] = r.port()
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
    - name: c_pri
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
%s
%s
`, refAdminPort, refContainerListenerPort, routeTable, healthChecksBlock,
		priorityEndpointsBlock("host.docker.internal", t0Ports, backendPorts))
}

func (d *priDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	tier0 := d.ensureTier0()
	t0Ports := make([]int, tier0Hosts)
	for i, r := range tier0 {
		t0Ports[i] = r.port()
	}
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0096, cluster: envoy-go-differential }
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
    - name: c_pri
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s
%s
`, subjAdminPort, subjListenerPort, routeTable, healthChecksBlock,
		priorityEndpointsBlock("127.0.0.1", t0Ports, backendPorts))
}

func (d *priDriver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

func (d *priDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

func (*priDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// classifyBody attributes a load response to tier "0" ("tier0:<idx>", the
// driver-owned toggleResponders) or tier "1" ("backend-<idx>:...", the
// runner-spawned HTTPEcho pool).
func classifyBody(body []byte) (tier string, err error) {
	s := string(body)
	switch {
	case strings.HasPrefix(s, "tier0:"):
		return "0", nil
	case strings.HasPrefix(s, "backend-"):
		return "1", nil
	default:
		return "", fmt.Errorf("body %q matches neither tier0: nor backend- prefix", s)
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
		{"tier0:0", "0", false},
		{"tier0:4", "0", false},
		{"backend-0:health", "1", false},
		{"backend-4:", "1", false},
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
	if membershipTotal != tier0Hosts+tier1Hosts {
		t.Errorf("membershipTotal=%d != tier0Hosts+tier1Hosts=%d", membershipTotal, tier0Hosts+tier1Hosts)
	}
	if backendCount != tier1Hosts {
		t.Errorf("backendCount=%d must equal tier1Hosts=%d (tier 1 is the runner-spawned pool)", backendCount, tier1Hosts)
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

`README.md` (prose mirroring the package doc comment above: topology, the two arms, the toggle-responder mechanism, why the assertions are HARD boundaries not bands). `expectations.yaml` (mirroring `0095`'s shape):

```yaml
# Phase-53 fixture 0096-lb-priority: differential expectations.
#
# Cross-side [http_connection_manager + router] fixture over ONE cluster
# c_pri with TWO LocalityLbEndpoints groups at distinct priority values (0
# and 1), 5 hosts each, active HTTP health checking (path /healthz, fast
# convergence). Tier 1 is the runner-spawned HTTPEcho pool; tier 0 is 5
# driver-owned toggleable HTTP responders.
#
# arm (a) — static (all 10 hosts healthy): HARD 100%/0% split — ALL traffic
#   on tier 0, NONE on tier 1 (capacitySum=200>=100, no bypass).
# arm (b) — full failover: ALL 5 of tier 0's hosts fail /healthz; HARD
#   0%/100% split — capacitySum=0+100=EXACTLY 100, the confirmed boundary
#   that does NOT trigger the AMEND-P1 bypass (a hard failover, not a
#   flattened spray across all 10 hosts).
#
# Cross-side deterministic stats (both sides, both arms):
#   cluster.c_pri.membership_total    == 10 (always — filtering, not removal)
#   cluster.c_pri.membership_healthy  == 10 (arm a) / 5 (arm b, post-failover)
#   cluster.c_pri.upstream_rq_total   >= 600 (staticLoadCount + degradedLoadCount)
# Plus the "decode ran" guard: cluster.c_pri.upstream_rq_total > 0 on the
# reference before trusting the readout.
#
# response-body: byte-exact required ONLY over the fixed "READY\n" Drive
# stream (address-independent). The load-phase GET / bodies are tallied for
# the per-side HARD tier-split assertions inside AssertStats, NOT
# byte-compared (a randomized LB policy — cross-side per-request identity is
# infeasible, the 0060/0065/0095 lineage).
#
# NOT exercised by this fixture (deliberately, per SPEC §8.1's own scope
# discipline): the AMEND-P1 capacity-shortfall bypass (neither arm's capacity
# sum ever drops below 100) and the AMEND-P1-COROLLARY per-tier
# no-local-panic property (neither arm has an asymmetrically
# PARTIALLY-degraded tier) — BOTH covered instead by dedicated UNIT tests
# (internal/cluster/priority_test.go, Tasks 5/7).
#
# Non-additions: NO new BackendKind (tier 1 reuses HTTPEcho; tier 0 is
# driver-owned per reference_differential_fixture_dispatch_constraint); NO
# separate boot-reject cross-side dir (SPEC §8.2/AMEND-P2 — both composition
# rejects are envoy-go-strict-only departures, unit-tested at
# internal/cluster/manager_test.go, Task 6).
response-body:
  applicable: true
  scope: byte-exact
```

- [ ] **Step 4: Run + gate**

```bash
cd test/fixtures/0096-lb-priority/driver && go test ./... -v 2>&1 | tail -30
gofmt -l test/fixtures/0096-lb-priority/
golangci-lint run ./test/fixtures/0096-lb-priority/...
go build ./...
```
Expected: all PASS; the driver registers without panicking at `init()`.

- [ ] **Step 5: Commit (LOCAL-ONLY)**

```bash
git add test/fixtures/0096-lb-priority/
git commit -m "phase 53 Task 8: 0096-lb-priority driver scaffolding (SPEC §8.1) — the 2-tier×5-host topology, the driver-owned toggleResponder full-failover harness (the 0095 precedent, reused directly — ALL 5 of tier 0 toggled, not a partial degrade), bootstrap builders for both sides, README + expectations.yaml"
```

---

## Task 9: `0096` `AssertStats` — arm (a) hard 100%/0%, arm (b) hard 0%/100% failover, cross-side stats

**Goal:** Implement the driver's `AssertStats` (the sole hook holding both admin addrs): converge + warmup + load + HARD-split-assert for arm (a), THEN fail ALL of tier 0's hosts, converge + warmup + load + HARD-split-assert for arm (b), THEN the cross-side deterministic stats prong.

**Files:**
- Modify: `test/fixtures/0096-lb-priority/driver/driver.go`

- [ ] **Step 1: Add the poll/warmup/tally/assert helpers + `AssertStats`**

First, add `"strconv"` to `driver.go`'s import block (Task 8 deliberately did NOT import it — nothing in Task 8's own code uses it; this task's `scrapeStats` is the first consumer, via `strconv.ParseUint`):

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

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)
```

Append to `driver.go`:

```go
func pollMembershipHealthy(side, adminAddr string, want int) error {
	deadline := time.Now().Add(convergeDeadline)
	var last int64 = -1
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			if v, ok := st["cluster.c_pri.membership_healthy"]; ok {
				last = int64(v)
				if v == uint64(want) {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: cluster.c_pri.membership_healthy did not converge to %d within %s (last seen %d)", side, want, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

type tierTally struct{ t0, t1 int }

func loadAndTally(ctx context.Context, side, addr string, n int) (tierTally, error) {
	var t tierTally
	for i := 0; i < n; i++ {
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", "/", nil, nil)
		if err != nil {
			return t, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if resp.StatusCode != http.StatusOK {
			return t, fmt.Errorf("%s: GET /[%d]: status %d, want 200", side, i, resp.StatusCode)
		}
		tier, err := classifyBody(body)
		if err != nil {
			return t, fmt.Errorf("%s: GET /[%d]: %w", side, i, err)
		}
		if tier == "0" {
			t.t0++
		} else {
			t.t1++
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

// assertHardSplit asserts a HARD tier boundary (not a statistical band —
// SPEC §8.1's failover proof is exact by construction, since the losing
// tier's cascade-load bucket is EMPTY, not merely small): wantAll0 selects
// which tier must receive 100% (true → tier 0 gets everything, tier 1 gets
// nothing; false → the reverse, arm (b)'s failover outcome).
func assertHardSplit(t fixture.TB, side string, tally tierTally, wantAll0 bool) {
	t.Helper()
	if wantAll0 {
		if tally.t1 != 0 {
			t.Errorf("%s: tier 1 must receive ZERO traffic (arm a, tier 0 fully healthy); got t0=%d t1=%d", side, tally.t0, tally.t1)
		}
		if tally.t0 == 0 {
			t.Errorf("%s: tier 0 must receive ALL traffic (arm a); got t0=%d t1=%d", side, tally.t0, tally.t1)
		}
		return
	}
	if tally.t0 != 0 {
		t.Errorf("%s: tier 0 must receive ZERO traffic (arm b, fully failed over); got t0=%d t1=%d", side, tally.t0, tally.t1)
	}
	if tally.t1 == 0 {
		t.Errorf("%s: tier 1 must receive ALL traffic (arm b, the failover target); got t0=%d t1=%d", side, tally.t0, tally.t1)
	}
}

func (d *priDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	ctx := context.Background()

	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	tier0 := d.tier0
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q)", refListener, subjListener)
	}

	// --- arm (a): static, all 10 hosts healthy — HARD 100%/0% ---
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
	assertHardSplit(t, "reference/static", refStaticTally, true)
	assertHardSplit(t, "subject/static", subjStaticTally, true)

	// --- arm (b): fail ALL of tier 0, re-measure the HARD failover ---
	for _, r := range tier0 {
		r.SetHealthy(false)
	}
	if err := pollMembershipHealthy("reference", refAdminAddr, membershipTotal-tier0Hosts); err != nil {
		t.Fatalf("arm(b) converge: %v", err)
	}
	if err := pollMembershipHealthy("subject", subjAdminAddr, membershipTotal-tier0Hosts); err != nil {
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
	assertHardSplit(t, "reference/failover", refDegradedTally, false)
	assertHardSplit(t, "subject/failover", subjDegradedTally, false)

	// --- cross-side deterministic stats ---
	ref, err := scrapeStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref /stats: %v", err)
	}
	subj, err := scrapeStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj /stats: %v", err)
	}
	if ref["cluster.c_pri.upstream_rq_total"] == 0 {
		t.Fatalf("reference did NOT decode: cluster.c_pri.upstream_rq_total == 0")
	}
	for _, sd := range []struct {
		side string
		st   map[string]uint64
	}{{"reference", ref}, {"subject", subj}} {
		assertEq(t, sd.side, sd.st, "cluster.c_pri.membership_total", uint64(membershipTotal))
		assertEq(t, sd.side, sd.st, "cluster.c_pri.membership_healthy", uint64(membershipTotal-tier0Hosts))
		if got := sd.st["cluster.c_pri.upstream_rq_total"]; got < uint64(staticLoadCount+degradedLoadCount) {
			t.Errorf("%s: cluster.c_pri.upstream_rq_total = %d, want >= %d (the measured load alone; convergence/warmup traffic adds more)", sd.side, got, staticLoadCount+degradedLoadCount)
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
// into a map[name]uint64 (the 0066/0095 scrapeStats, verbatim).
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
	_ fixture.Driver        = (*priDriver)(nil)
	_ fixture.StatsAsserter = (*priDriver)(nil)
)
```

- [ ] **Step 2: Run (subject-side path first; cross-side where Docker is present)**

```bash
go test ./test/differential/ -run 'TestDifferential/0096' -count=1 2>&1 | tail -30
# cross-side (controller runs where Docker + the contrib image are present):
#   verify the decode ran: cluster.c_pri.upstream_rq_total > 0 on the reference.
```
Expected: PASS. Confirm via `-run 'TestDifferential/0096'` (`reference_differential_run_selector`).

- [ ] **Step 3: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l test/fixtures/0096-lb-priority/ && golangci-lint run ./test/...
git add test/fixtures/0096-lb-priority/driver/driver.go
git commit -m "phase 53 Task 9: 0096 AssertStats — arm (a) HARD 100%/0% static split + arm (b) HARD 0%/100% full-failover split (post-toggle re-convergence+re-warmup) + cross-side membership_total/membership_healthy/upstream_rq_total stats; fixtures 97 → 98"
```

---

## Task 10: `0096` deliberate-break liveness (`-count=1`) + the ≥20-run flake check

**Goal:** PROVE both `0096` assertions are LIVE via the two SPEC §8.1 deliberate breaks, each restored after; confirm the workload constants are synced; run a ≥20-run flake check.

**Files:**
- Modify: `test/fixtures/0096-lb-priority/README.md` (record the break protocol)
- Modify: `docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md`

- [ ] **Step 1: Break (i) — skip the priority-tier capture** (defeats arm (a)'s hard split)

Temporarily change `extractEndpoints`'s `Endpoint{...}` construction in `manager.go` to omit `Priority: priority` (leaving every endpoint at the zero-value tier 0) → `distinctPriorities` reports a single tier → the `priorityLB` wrap NEVER fires (`len(priorityTiers) > 1` is false) → the cluster falls back to plain ROUND_ROBIN over all 10 hosts → arm (a)'s traffic spreads roughly 50/50 across tier 0 and tier 1 hosts instead of 100%/0% → `assertHardSplit`'s `tally.t1 != 0` check MUST fail.

```bash
go test ./test/differential/ -run 'TestDifferential/0096' -count=1 2>&1 | tail   # expect FAIL (arm a hard split)
```
RESTORE (`git restore` the file — `feedback_subagent_worktree_detach`: never checkout-sha/amend). Re-run → PASS.

- [ ] **Step 2: Break (ii) — skip the health-degradation step in arm (b)** (defeats the failover split)

Temporarily comment out the `for _, r := range tier0 { r.SetHealthy(false) }` loop in `driver.go`'s `AssertStats` → tier 0 never actually fails its health check → traffic stays 100%/0% (unchanged from arm (a)) instead of flipping to 0%/100% → `assertHardSplit`'s `tally.t0 != 0` check (arm b, `wantAll0=false`) MUST fail.

```bash
go test ./test/differential/ -run 'TestDifferential/0096' -count=1 2>&1 | tail   # expect FAIL (arm b hard split)
```
RESTORE. Re-run → PASS.

- [ ] **Step 3: ≥20-run flake check + the constant-sync guard**

```bash
go test ./test/differential/ -run 'TestDifferential/0096' -count=20 2>&1 | tail   # 20/20 PASS
grep -n "staticLoadCount\|degradedLoadCount\|tier0Hosts\|tier1Hosts" test/fixtures/0096-lb-priority/driver/driver.go
```
Expected: 20/20 PASS (a HARD boundary assertion is inherently flake-resistant — no statistical band to mistune, per the SPEC/BRAINSTORM's Q4 choice); every workload count is a named constant, never a re-derived literal.

- [ ] **Step 4: `-race`**

```bash
go test ./test/differential/ -run 'TestDifferential/0096' -race -count=1 2>&1 | tail
```
Expected: PASS, no race (the `toggleResponder.healthy` field is an `atomic.Bool`; `priDriver`'s stashed fields are `sync.Mutex`-guarded).

- [ ] **Step 5: Record + commit (LOCAL-ONLY)**

Record the two breaks (each: the exact edit, the FAILING assertion, the restore) in `README.md` + PROGRESS.md.
```bash
git add test/fixtures/0096-lb-priority/README.md docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md
git commit -m "phase 53 Task 10: 0096 deliberate-break liveness (reference_differential_break_protocol_count1) — skip-the-priority-capture (arm a hard split) / skip-the-failover-toggle (arm b hard split), each FAILS under -count=1 then restored; 20/20 flake-free; -race clean"
```

---

## Task 11: Completion bundle — BEHAVIOR_CONTRACT delta + ADR-0270 body + final six-gate

**Goal:** Land the atomic completion bundle: the new `### Load balancer — priority (LocalityLbEndpoints.priority)` BEHAVIOR_CONTRACT section (mirroring the `### Load balancer — locality-weighted` section's shape, `BEHAVIOR_CONTRACT.md:1265-1294`); the full ADR-0270 entry (§Context — promoting the SPEC §13 draft verbatim — + §Decision + §Consequences, ADR-0044 in-place); the final six-gate. **This task's subagent does NOT touch STATE.md/ROADMAP.md** — per this session's controller instructions, those are updated by the controller after reviewing the completed IMPL, not by a task subagent (the phase-52 Task 10 precedent — a deliberate departure from earlier phases that bundled STATE/ROADMAP into the same subagent commit, recorded here so the IMPL session doesn't silently diverge from ITS OWN controller's instructions).

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `docs/envoy-go/DECISIONS.md`
- Modify: `docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT — the new section**

Add `### Load balancer — priority (LocalityLbEndpoints.priority)` immediately after the EXISTING `### Load balancer — locality-weighted (locality_weighted_lb_config)` section (`BEHAVIOR_CONTRACT.md:1265-1294`), covering: the opening italic intro (citing ADR-0270 + the phase-52 `localityWeightedLB` structural precedent + the zero-seam-change framing); the wrap-after-switch acceptance (the THIRD wrap site, reusing the already-in-scope `sc`/`lwc`); the `Endpoint.Priority` dimension (a plain scalar, no wrapper-type ambiguity — AMEND-P3); the confirmed per-tier capacity formula + the CORRECTED cascade (AMEND-P4, with the exact naive-vs-corrected divergence at 3+ tiers spelled out); the AMEND-P1 capacity-shortfall bypass (the exact condition `Σ min(100, OPF×healthy_fraction_i) < 100`, called out explicitly as NOT the classic `healthy_panic_threshold` — the phase's single most consequential finding, REFUTING both BRAINSTORM hypotheses); the AMEND-P1-COROLLARY per-tier no-local-panic property (`tierHealth`, a genuinely NEW primitive vs. `locality.go`); the two explicit composition rejects + their departure framing (§6.1 VERBATIM wording, noting the reference's OWN natural priority-then-locality/priority-then-subset nesting, AMEND-P2); the confirmed-zero stat delta (§7) PLUS the D-P-HEALTHALLOC side effect (a priority-tiered cluster with zero `health_checks` now ALSO gets `membership_healthy`/`lb_healthy_panic` registered — a SECOND widened CONDITION on the same two pre-existing stat names D-LW-HEALTHALLOC already widened, not a new name, not a stat-surface COUNT change); the D-P-DUP/D-P-FACTORY/D-P-RETROFIT resolutions; the differential proof shape (`0096`'s two HARD-boundary arms, contrasted with `0095`'s statistical bands — a deliberately narrower proof shape since failover is a hard boundary); the deferred surface list (from SPEC §2: `weighted_priority_health`, `endpoint_stale_after`, `drop_overloads`, `proximity`, the reference's own unrelated per-priority circuit-breaker stat buckets, the family's last remaining candidate — panic thresholds as an independent construct). Stat-surface doc count STAYS **1200**.

- [ ] **Step 2: DECISIONS — ADR-0270 (ADR-0044 in-place)**

Promote the SPEC §13 §Context DRAFT to the full entry (§Context verbatim from SPEC §13 + §Decision + §Consequences; status ACCEPTED). §Decision records (verified against the LANDED code, the ADR-0269 precedent's own citation discipline): `internal/cluster/priority.go`'s `tierHealth`/`tierCapacity`/`cascadeLoads`/`priorityGroup`/`priorityLB`/`priorityLeafFactory`/`newPriorityLB`/`newPriorityLBWithRNG`/`Pick` — each function's actual signature, quoted verbatim from the landed file, exactly as ADR-0269's §Decision quoted `locality.go`'s; `manager.go`'s THIRD wrap-after-switch (the `distinctPriorities` helper + the two reject arms + the wrap, quoted verbatim); the two reject error strings (verbatim). §Consequences records: the D-P-HEALTHALLOC health-registry-widening side effect (the SECOND widening of the SAME two stat names, after D-LW-HEALTHALLOC); the D-P-FACTORY local-type resolution (NOT a `leafFactory` retrofit); the D-P-RETROFIT out-of-scope disposition (an explicit forward-pointer to `locality.go`'s own still-undocumented-as-fixed coverage boundary); ADR-0024 (per-cluster LB-state scope) STAYS UNAMENDED. DECISIONS tail → **ADR-0270**; next-free **ADR-0271**.

- [ ] **Step 3: PROGRESS.md final + the six-gate evidence**

Mark all 11 tasks complete; record the six-gate evidence (build / unit+race / the full 98-dir differential / conformance asserted-unaffected / counts / docs). Confirm the ADR-0045 verdict held (NO split — 11 tasks / ~200-250 LoC, per the FINAL re-check below).

- [ ] **Step 4: The atomic completion commit (LOCAL-ONLY) — then the controller pushes**

```bash
go build ./... && go test ./... 2>&1 | tail   # final green confirmation BEFORE the bundle commit
git add docs/envoy-go/BEHAVIOR_CONTRACT.md docs/envoy-go/DECISIONS.md docs/envoy-go/phases/53-load-balancer-priority/PROGRESS.md
git commit -m "phase 53 IMPL DONE: priority LB (LocalityLbEndpoints.priority) — the SEVENTH Load-balancing-family construct (ADR-0270), the family's SECOND zero-new-Pick-parameter wrapper; AMEND-P1 capacity-sum bypass (REFUTES both BRAINSTORM panic hypotheses) + AMEND-P4 corrected 3+-tier cascade + AMEND-P1-COROLLARY per-tier no-local-panic (tierHealth); 2 new envoy-go-strict composition rejects (AMEND-P2 confirms genuine reference composition); stat surface 1200 (+0, the family's FOURTH zero-stat-delta phase); 0096 cross-side hard-100%/0%-static + hard-0%/100%-failover differential (fixtures 97 → 98); fuzzers 52 + BackendKind 38 UNCHANGED"
```
> The controller squash-merges to master, updates STATE.md/ROADMAP.md (row 53 `in-progress → done`, NO parent rollup per ADR-0106), and pushes at stage-close (`feedback_push_to_origin`/`feedback_subagents_no_push`).

---

## FINAL ADR-0045 split-gate re-check

This PLAN decomposes into **11 tasks** (well under the `>~25 tasks` gate) over an estimated **~200-250 production LoC**: `priority.go` ~170-190 LoC (`tierHealth`/`tierCapacity`/`cascadeLoads` + `priorityGroup`/`priorityLB`/`priorityLeafFactory` + both constructors + `Pick`) + `cluster.go`'s `Endpoint.Priority` field addition ~10 LoC + `manager.go`'s `extractEndpoints` capture (~2 LoC delta) + the `distinctPriorities` helper (~8 LoC) + the wrap-after-switch (~30-35 LoC) — well under the `>~1500 LoC` gate. **NO split is needed.** The wrapper reuses the EXISTING `loadBalancer` interface, the EXISTING `buildLeafLB` factory (confirmed compiling verbatim against `priorityLeafFactory`'s 4-parameter shape, D-P-FACTORY), and the EXISTING `clusterHealth` model (extended only by the pure `tierHealth` view-constructor, itself ~5 LoC) UNCHANGED — there is no second seam to build. This is the phase-35/37/52 single-flat-row precedent, repeated a FOURTH time: one new file, one new wrap site, zero new packages, zero new producer plane. Slightly CHEAPER than phase 52's ~230-330 LoC / 10 tasks estimate, since `priority.go`'s three-way pure-function split (Task 3) makes the confirmed formulas directly unit-testable without any `Pick`-level stochastic scaffolding — the SPEC's own §1.1 AMEND-P6 anticipated ~230-330/11-12; this PLAN's concrete decomposition lands at the LOW end of that range.

---

## Verification checklist (the ADR-0052 six-gate, at Task 11)

1. **Build:** `go build ./...` green; `go mod tidy -diff` empty (ZERO new dep).
2. **Unit + race:** `go test ./...` + `go test -race -short ./...` green (incl. `priority_test.go`, the widened `manager_test.go`, the `0096` driver's unit-level tests).
3. **Differential:** the full 98-dir suite green (`-count=1`); `0096` liveness-proven (2 breaks, ≥20-run flake-free, `-race` clean); the 97 prior dirs byte-exact (the wrap-after-switch only fires for a multi-tier-priority cluster; no prior fixture configures `LocalityLbEndpoints.priority`).
4. **Conformance:** h2spec + proxy-wasm asserted-unaffected by change-scope (priority-tier LB touches only the cluster LB pick, not HTTP/2 framing or the wasm path); re-run where the harness is present.
5. **Counts:** fixtures 97 → 98 (`0096`); stat surface 1200 UNCHANGED (+0, confirmed §11.5 — the family's FOURTH zero-stat-delta phase); fuzzers 52 UNCHANGED; BackendKind 38 UNCHANGED; DECISIONS tail ADR-0269 → ADR-0270 (next-free ADR-0271).
6. **Docs:** BEHAVIOR_CONTRACT `### Load balancer — priority` section (mirroring the locality-weighted section's shape) + the ADR-0270 full entry (§Context/§Decision/§Consequences, ADR-0044 in-place). STATE/ROADMAP row 53 `in-progress → done` land at the CONTROLLER's stage-close, not inside Task 11's subagent commit (this PLAN's departure mirrors the phase-52 Task-10 precedent exactly).
