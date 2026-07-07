# Phase 53 SPEC — `priority` load balancing (`LocalityLbEndpoints.priority`)

> For agentic workers: this SPEC resolves every BRAINSTORM.md §10 D-P question (D-P1..D-P6) against LIVE evidence from `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227), executed in-session over two parallel probe agents on Docker bridge networks (`reference_docker_probe_bridge_network`). **The BRAINSTORM's D-P1 panic hypothesis was REFUTED — the real mechanism is neither of the two readings the BRAINSTORM posed** (§11.1/AMEND-P1, the single most consequential finding of this SPEC). **D-P4's cascade hypothesis was CONFIRMED per-tier but REFUTED at the remainder-aggregation step for 3+ tiers** (§11.4/AMEND-P4). D-P2's composition rejects are CONFIRMED genuine departures — the reference not only accepts both combinations, it MEANINGFULLY COMPOSES them (§11.2/AMEND-P2). D-P5's anticipated small stat delta is REFUTED DOWN to zero (§11.5/AMEND-P5), the family's FOURTH zero-stat-delta LB phase. The PLAN decomposes §10 into TDD tasks.

**Goal:** land `LocalityLbEndpoints.priority` as the SEVENTH Load-balancing-family row — a `priorityLB` wrapper that groups a cluster's endpoints by priority tier, computes each tier's effective capacity from its live per-host health fraction (scaled by the EXISTING `ClusterLoadAssignment.Policy.overprovisioning_factor`, a phase-52 dependency reused verbatim), cascades load across tiers in priority order, and — on a genuinely NEW cluster-wide capacity-shortfall condition discovered by this SPEC's own live probe — falls back to a pure host-count-uniform draw across every host in every tier, ignoring both priority and health entirely. Reuses the EXISTING `loadBalancer.Pick` seam with ZERO new parameters.

**Architecture:** a single new file `internal/cluster/priority.go` (the `locality.go` precedent) + one new `Endpoint` field (`Priority uint32`) + an extended `extractEndpoints` + a THIRD wrap-after-switch site in `manager.go`'s `buildCluster`. NO new package, NO new go.mod dependency (confirmed clean, §11.3), NO new producer plane (a cluster-only construct), NO new stat (confirmed, §11.5). **One genuinely new implementation primitive not present in `locality.go`'s shape:** each tier's own child `loadBalancer` must be built against a PANIC-DISABLED health view (a new `tierHealth` helper, §3.1) — a direct, load-bearing consequence of AMEND-P1's finding that the reference never lets a single degraded tier internally flatten itself; only ONE cluster-wide capacity check governs any bypass at all.

**Tech stack:** unchanged — Go, `internal/cluster`, the existing `clusterHealth`/`hostHealth` model (`health.go`), the existing `newPCGRNG` crypto-seeded `math/rand/v2` PCG idiom (`leastrequest.go:61-81`, directly reused by `locality.go`'s `newLocalityWeightedLB`).

**Authored:** 2026-07-07.

---

## 1. Purpose / Mission

Phase 53 lands the family's SEVENTH LB construct: priority-tier overflow/failover, driven by `LocalityLbEndpoints.priority` (a per-endpoint-group tier number, 0 = highest priority) plus the ALREADY-consumed `ClusterLoadAssignment.Policy.overprovisioning_factor` (phase 52, default 140). The mechanism, as CONFIRMED by this SPEC's live probes: for each priority tier, compute an *effective capacity* = `min(100, overprovisioning_factor × healthy_fraction)` (a percent, 0-100); cascade tiers in priority order, each tier's assigned load capped at `min(its own effective capacity, whatever budget remains after higher tiers)`; weighted-random-draw a tier by its assigned load, then delegate to a per-tier child `loadBalancer` (built by the existing `buildLeafLB` factory) restricted to that tier's OWN healthy hosts. **A single cluster-wide bypass condition — the SUM of every tier's OWN effective capacity falling below 100 — abandons this mechanism entirely**, falling back to a flat, health-ignoring, host-count-uniform draw across every host in every tier (reusing the EXISTING `lb_healthy_panic` stat, confirmed §11.1).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

- **AMEND-P1 (D-P1 SPEC-BLOCKING — REFUTATION of BOTH BRAINSTORM hypotheses; the phase's single most consequential finding).** The BRAINSTORM (§2.5) posed two candidate readings: (a) cluster-wide `healthy_panic_threshold` (aggregate healthy fraction < 50%) bypasses the whole mechanism, mirroring phase 52's shape one layer higher; or (b) panic is computed independently PER TIER. **Neither is correct.** LIVE-PROBED across 8 two-tier scenarios (P0 healthy fraction swept 100%→0% against a fully-healthy P1, plus an asymmetric-degradation control) and two 3-tier scenarios: the reference's actual bypass condition is **`Σ_i min(100, overprovisioning_factor × healthy_fraction_i) < 100`** — the SUM, across ALL tiers, of each tier's OWN capacity-capped-at-100 effective load. When this sum is ≥100 (there is enough spare overprovisioned capacity SOMEWHERE across the tiers to cover 100% of demand), NO bypass fires, REGARDLESS of how low any individual tier's raw healthy fraction is — a tier at 20% healthy (well under the classic 50% panic threshold) correctly restricts its share to its 1 healthy host, with its 4 unhealthy hosts getting **zero** traffic (92/300 to the healthy host, 0/300 to each unhealthy one — decisive per-host evidence, not just an aggregate percentage). When the sum drops below 100, the reference abandons BOTH priority ordering AND per-host health filtering entirely, spreading traffic FLAT and UNIFORM by raw host count across every host in every tier — including hosts that are actively failing their health check (a 2-tier, both-at-20%-healthy scenario gave 32/32/32/32/32 across P0's 5 hosts and 28/28/28/28/28 across P1's 5, a clean 50/50-by-host-count split, with `lb_healthy_panic` incrementing on every one of 300 requests). A 3-tier control (P0=20%, P1=20%, P2=100%: aggregate healthy fraction 46.7%, well UNDER the classic 50% threshold) proved the aggregate-fraction reading FALSE: capacity sum = 28+28+100 = 156 ≥ 100 → **NO bypass** (confirmed `lb_healthy_panic` delta = 0), and the observed split (29.0%/28.7%/42.3%) matched the normal per-tier waterfall almost exactly, not a flat 33/33/33 split. **envoy-go reproduces this bit-for-bit** (§3.1): `priorityLB.Pick` computes the capacity-sum ONCE per Pick, compares against 100 (strict `<`, confirmed by the boundary scenario P0=0%/P1=100% → sum EXACTLY 100 → no bypass, clean 0%/100% split), and on bypass delegates to a `flat` child built with a **NIL health registry** (not the shared `clusterHealth` — see the divergence-risk rationale in §3.1) so it always performs a pure per-host-count-uniform draw ignoring health entirely, reusing the EXISTING `lb_healthy_panic` counter via `health.panicInc()` exactly as `localityWeightedLB.Pick` already does for its own (different) bypass condition.
- **AMEND-P1-COROLLARY (a new implementation primitive, NOT itself an empirical pin, but a DIRECT and NECESSARY consequence of AMEND-P1): no per-tier "local panic."** Phase 52's `localityWeightedLB` builds each locality's child leaf via the shared `buildLeafLB(c, name, members, health)`, where `health` is the SAME `*clusterHealth` object passed to every child — meaning a leaf policy's OWN internal `health.inPanic(itsOwnSubslice)` check (e.g. `roundRobin.Pick`, `loadbalancer.go:44-66`) is evaluated over just THAT locality's own endpoints. Phase 52's own PLAN/IMPL (STATE.md, `phase 52 PLAN done`) explicitly discovered and DOCUMENTED this as an accepted, unfixed coverage boundary ("a locality's own leaf child evaluates its OWN `clusterHealth.inPanic` over its OWN endpoint sub-slice ... verified harmless to the `0095` differential ... not a bug to fix"). **This SPEC's own live probe (AMEND-P1) supplies exactly the evidence phase 52 lacked**: a tier at 20% healthy (well under the classic 50% per-subslice panic threshold) does NOT flatten within itself in the reference — its 4 unhealthy hosts get ZERO traffic, its 1 healthy host gets 100% of the tier's share. Reproducing phase 52's "accept the gap" posture here would mean DELIBERATELY implementing behavior this SPEC has direct, decisive counter-evidence against. The fix costs nothing against the ZERO-new-`Pick`-parameter design goal (that goal is about the `Pick` FUNCTION SIGNATURE, never touched here — this is purely a constructor-time wiring choice): a new `tierHealth(shared *clusterHealth) *clusterHealth` helper (§3.1) returns a VIEW sharing the SAME per-host `states` map (so live health-check results are honored identically) but with `panicThreshold: 0` (so `inPanic` can never fire, since a fraction can never be strictly less than 0). Each tier's child is built via `buildLeafLB(c, name, tierEndpoints, tierHealth(health))` instead of the raw shared `health` — the leaf's own per-host `available()` filtering stays fully live (skip individual unhealthy hosts), but its own internal panic branch is permanently dead code for a priority-tier child. Whether `locality.go` should retroactively adopt the identical pattern is flagged as a candidate FUTURE maintenance item (§12), out of THIS phase's scope (a different file/construct).
- **AMEND-P2 (D-P2 SPEC-BLOCKING — CONFIRMATION that BOTH composition rejects are genuine departures, with a MORE SPECIFIC finding than the BRAINSTORM anticipated: the reference doesn't just accept the combinations, it MEANINGFULLY COMPOSES them).** LIVE-PROBED: a cluster with 2 priority tiers, EACH ALSO carrying distinct localities/weights, PLUS `common_lb_config.locality_weighted_lb_config` configured — `envoy --mode validate` → `configuration OK`; a real boot with 200 requests sent ALL healthy showed **100% of traffic confined to the priority-0 tier** (0 requests reached priority-1), and WITHIN priority-0, the split matched the configured 80/20 locality weights almost exactly (159/41 ≈ 79.5%/20.5%). This directly confirms the reference's natural nesting the BRAINSTORM's §2.2 named as a possibility: **priority selects the active tier FIRST, then locality-weighting divides load WITHIN that tier** — a real, working, THREE-layer composition (priority → locality-weight → leaf) the reference implements and envoy-go deliberately does NOT build (the BRAINSTORM's Q2 choice stands; this SPEC only confirms it is a genuine departure, not a reference-side no-op). A SECOND test — 2 priority tiers PLUS `lb_subset_config` (a metadata-based subset selector) both configured — ALSO validated and booted cleanly; traffic stayed 100% confined to the priority-0 subset-matched endpoint (30/30 requests), and `lb_subsets_created` (33) confirmed the reference builds subset structures PER PRIORITY LEVEL internally. **Both composition rejects (§6) are CONFIRMED envoy-go-strict departures** — exact wording pinned below.
- **AMEND-P3 (D-P3 SPEC-BLOCKING — CONFIRMATION of the proto surface + defaults + a clean go.mod).** Re-confirmed directly from the go-control-plane v1.32.4 module cache: `LocalityLbEndpoints.priority` (`endpoint/v3/endpoint_components.pb.go:356`, field 5) is a **plain `uint32`** — NOT a `*wrapperspb.UInt32Value` (unlike `overprovisioning_factor`, which IS a wrapper type) — so there is NO way to distinguish "explicitly set to 0" from "left absent" at the proto layer; both are simply the Go zero value. This is a MATERIALLY SIMPLER situation than phase 52's D-LW-OPF0 wrinkle (which required threading a `hasOPF bool` precisely because `overprovisioning_factor` IS a wrapper type) — priority needs no such threading; `group.GetPriority()` returning `0` always means "tier 0," full stop, matching the reference's own documented default. Confirmed empirically: a cluster with an entirely omitted `priority` field lands its endpoints in the same tier as an explicit `priority: 0` group. `go mod tidy` in the envoy-go repo reconfirmed a clean no-op (`git status --porcelain go.mod go.sum` empty before/after) — ZERO new go.mod dependency needed; `LocalityLbEndpoints.priority` and `ClusterLoadAssignment.Policy.overprovisioning_factor` are both already reachable through the existing go-control-plane v1.32.4 dependency (the latter already consumed by phase 52).
- **AMEND-P4 (D-P4 SPEC-BLOCKING — CONFIRMATION of the per-tier formula, REFUTATION + CORRECTION of the cascade's remainder-aggregation step for 3+ tiers).** The BRAINSTORM (§2.4) hypothesized a "waterfall": `P0_share = min(100, effective_capacity(P0))`, remainder cascading to P1 by the "identical rule against P1's OWN healthy fraction." This is AMBIGUOUS between two readings once 3+ tiers are involved, and the BRAINSTORM's own hypothesis text (and this SPEC's initial probe framing) implicitly assumed a "recursive-fraction" reading: `P1_share = P1's_effective_capacity × (remaining_budget / 100)`. LIVE-PROBED with a 3-tier decisive test (P0=40% healthy, P1=60% healthy, P2=100% healthy — effective capacities 56/84/100): the recursive-fraction reading predicts `P1_share = 84% × 44% = 36.96%` (leaving P2 with 7.04%); the OBSERVED result was **P0=55.0% (165/300), P1=45.0% (135/300), P2=0.0% (0/300)** — decisively matching a DIFFERENT reading instead: **each tier's assigned load is capped by its OWN effective capacity taken as an ABSOLUTE percent of the total remaining budget, not scaled by a fraction of it** — i.e. `load_i = min(effective_capacity_i, remaining_budget_before_i)`, `remaining_budget -= load_i`. For exactly 2 tiers, the two readings coincide whenever the lower tier's effective capacity is ≥ the remaining budget (true across every 2-tier scenario probed here, which is why the naive formula looked perfect at 2 tiers — see the summary table §11.4) — the divergence ONLY appears at 3+ tiers, exactly where this SPEC's extra probe scenario caught it. A second 3-tier scenario (P0=20%, P1=20%, P2=100% — capacities 28/28/100) confirmed the corrected formula again: predicted 28/28/44, observed 29.0%/28.7%/42.3%.
- **AMEND-P5 (D-P5 — REFUTATION DOWN: the anticipated "small 0-2" stat delta is ZERO — the family's FOURTH zero-stat-delta LB phase).** A full `/stats` scrape of a traffic-served, 2-priority-tier cluster, grepped case-insensitively for "priority," found NO dedicated `cluster.<name>.priority.*` LB/membership stat. The only "priority" hits were the reference's PRE-EXISTING, UNRELATED per-priority circuit-breaker stat buckets (`circuit_breakers.default.*` = priority-0's bucket, `circuit_breakers.high.*` = priority-1's bucket, 5 stats each — CB LIMITS, not LB composition or tier health; orthogonal to this phase, the phase 52 `lb_zone_*`-is-unrelated precedent repeated) plus incidental substring matches of the test cluster's own name. `membership_total`/`membership_healthy` stay SINGLE AGGREGATE gauges per cluster (confirmed NOT split or tagged per priority tier anywhere in `/stats`) — the per-host `priority::N` field is visible ONLY on the `/clusters` admin text endpoint, never as a `/stats` counter/gauge. **Stat surface STAYS 1200 (+0)** — cross-side proof relies entirely on the pre-existing `membership_total`/`membership_healthy` gauges plus per-tier request-share counting in the `0096` driver, exactly as anticipated as the fallback.
- **AMEND-P6 (D-P6 — LoC/task envelope re-check, self-assessed at this SPEC, no live probe needed).** Anticipated **~230–330 prod LoC / ~11-12 tasks** — slightly ABOVE phase 52's ~220-320/10, driven by the `tierHealth` helper (AMEND-P1-COROLLARY) + the corrected two-quantity cascade computation (§3.1's `tierCapacity`/`cascadeLoads`) + TWO composition-reject arms (mirroring phase 52's two, but confirmed via a materially richer probe finding). Comfortably under the ADR-0045 `>~25 tasks OR >~1500 LoC` gate — CONFIRMING §3.0's no-split disposition. No dedicated fuzzer warranted (no wire-decode surface, identical reasoning to phase 52's D-LW8).

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail stays **ADR-0269** at this SPEC (docs-only; no ADR body lands until IMPL per ADR-0044). §13 anchors the **ADR-0270 §Context DRAFT** (the SOLE anticipated ADR — the phase-35/37/52 single-ADR "reuse" shape, confirmed correct: §11.7 confirms ZERO seam change beyond the constructor-time `tierHealth` wiring, which touches no `Pick` signature). All 6 BRAINSTORM D-questions (D-P1..D-P6) are RESOLVED at this SPEC (§11); none are deferred to PLAN. §12 lists the PLAN/IMPL-level (non-empirical) design questions this SPEC leaves open.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

Unchanged from the BRAINSTORM's §8 deferred list, now with live-probe confirmation of WHY each is safe to defer:

- **Composition with `common_lb_config.locality_weighted_lb_config`** — the reference's own behavior (a genuine, working priority-then-locality nesting, LIVE-CONFIRMED §11.2) is not worth replicating without its own dedicated brainstorm (per-tier health scoping interacting with `localityWeightedLB`'s constructor is a real architecture question); BOTH-configured is an explicit reject (§6).
- **Composition with `lb_subset_config`** — the reference ALSO composes this (priority-tier-aware subset structures, LIVE-CONFIRMED §11.2); BOTH-configured is ALSO an explicit reject (§6), mirroring phase 52's own subset-composition-deferral posture.
- **`ClusterLoadAssignment.Policy.weighted_priority_health`** — moot without locality composition (§2.2 of the BRAINSTORM); revisit only if that composition is ever built.
- **`ClusterLoadAssignment.Policy.endpoint_stale_after`** — couples to dynamic EDS host-set churn, which does not exist project-wide (bootstrap-only static config).
- **`ClusterLoadAssignment.Policy.drop_overloads`** — a distinct traffic-shedding/load-shedding concept, unrelated to priority-tier selection.
- **`LocalityLbEndpoints.proximity`** — an unrelated distance-based hint, deferred (same posture as phase 52).
- **Any priority-specific stat namespace beyond the confirmed zero** — CONFIRMED NONE EXISTS (§11.5); nothing to defer here.
- **The reference's per-priority circuit-breaker stat buckets (`circuit_breakers.default`/`.high`)** — a DIFFERENT, PRE-EXISTING reference feature (circuit breaking scoped per priority LEVEL, not per-tier LB composition); orthogonal to this phase, not investigated further (whether envoy-go's own circuit-breaker implementation, if any, should scope per-priority is a candidate for a future circuit-breaker-specific phase, not this one).
- **Whether `locality.go`'s existing per-locality "local panic" coverage boundary should be retroactively fixed with the SAME `tierHealth`-style pattern this phase introduces** — flagged (§12) as a candidate LOW-RISK future maintenance item, explicitly OUT of this phase's scope (a different file/construct; phase 52 already shipped and documented its own posture on this).
- **The family's LAST remaining candidate — panic thresholds as an independent construct** — stays unimplemented; its own future row. (Note: this SPEC's AMEND-P1 finding — that the reference's REAL bypass condition for priority-tier clusters is a capacity-sum check, NOT the classic `healthy_panic_threshold` — is itself a useful precedent for that future row to read first, since it demonstrates the classic 50% threshold is not universally the relevant panic concept once priority tiers are in play.)

---

## 3. The `priorityLB` construct (ADR-0270)

### 3.0 Split disposition — D-P6 RESOLVED (single flat row; NO escape valve)

Anticipated **~230–330 prod LoC / ~11-12 tasks** (§1.1 AMEND-P6) — comfortably under the ADR-0045 `>~25 tasks OR >~1500 LoC` gate. NO pre-authorized split (the phase-37/52 precedent: a single flat row reusing an existing seam, no second producer plane or subsystem to couple against — confirmed by §4).

### 3.1 The `priorityLB` wrapper (ADR-0270; AMEND-P1/P1-COROLLARY/P4)

Indicative shape (the PLAN/IMPL finalizes exact naming/errors):

```go
// internal/cluster/priority.go

// priorityGroup is one distinct priority tier's endpoint sub-slice + its
// factory-built child loadBalancer (built against a PANIC-DISABLED health
// view — AMEND-P1-COROLLARY, tierHealth below).
type priorityGroup struct {
	priority  uint32
	endpoints []Endpoint // this tier's own sub-slice, for per-tier health aggregation
	child     loadBalancer
}

// tierHealth returns a clusterHealth VIEW over the SAME per-host state as
// shared (per-host available()/isHealthy() honor live health-check results
// identically) but with panic PERMANENTLY DISABLED (panicThreshold: 0, so
// inPanic can never fire — a fraction is never strictly < 0). AMEND-P1
// confirmed the reference applies NO per-tier panic concept: a tier at 20%
// healthy correctly restricts its share to its healthy hosts, never
// internally flattening across its own unhealthy ones. The states map is
// SHARED (not copied) — a Go map is a reference type, and the underlying
// *hostHealth entries are mutated in place by the background health checker,
// so this view always reflects live state. membershipHealthy/panicCounter
// stay nil (this view never emits stats — priorityLB.Pick itself is the SOLE
// caller of health.panicInc(), on its OWN capacity-sum bypass, §below).
func tierHealth(shared *clusterHealth) *clusterHealth {
	return &clusterHealth{states: shared.states, panicThreshold: 0, nowNanos: shared.nowNanos}
}

// tierCapacity computes one tier's effective capacity AS A PERCENT (0-100):
// min(100, overprovisioningFactor × healthyFraction) — the SAME primitive as
// locality.go's effectiveWeight (locality.go:122-125), expressed as an
// absolute percent rather than a weight-scaled float (priority tiers carry no
// per-tier configured weight, only order — AMEND-P4).
func tierCapacity(healthyFraction float64, overprovisioningFactor uint32) float64 {
	return math.Min(100, float64(overprovisioningFactor)*healthyFraction)
}

// cascadeLoads computes (a) the AMEND-P1 bypass quantity — capacitySum, the
// UNCAPPED sum of every tier's own tierCapacity, governing the single
// cluster-wide bypass check — and (b) the AMEND-P4 CORRECTED per-tier
// assigned loads: each tier's load is its OWN capacity capped by whatever
// budget remains after higher tiers (NOT capacity scaled by a fraction of
// the remaining budget — the two readings coincide at 2 tiers but diverge at
// 3+, confirmed by a decisive 3-tier live probe, §11.4).
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
	flat                   loadBalancer    // the AMEND-P1 capacity-shortfall fallback; built with a NIL health registry (see rationale below) — spans ALL endpoints, ALL tiers, ignoring health entirely
	allEndpoints           []Endpoint
	health                 *clusterHealth // the REAL, shared cluster health registry — used ONLY for computing each tier's healthy fraction and for panicInc() on bypass; never passed to children directly (tierHealth wraps it first)
	overprovisioningFactor uint32
	rng                    func() uint64 // the newPCGRNG (leastrequest.go:61-81) idiom — injectable for deterministic tests
}

var _ loadBalancer = (*priorityLB)(nil)

// newPriorityLB groups endpoints by Priority value (a duplicate priority
// across multiple source LocalityLbEndpoints groups MERGES into the SAME
// tier — priority carries no per-group scalar to conflict over, unlike
// locality's weight — §12 D-P-DUP), sorts tiers ASCENDING, builds one child
// per tier via the caller-bound factory (restricted to a PANIC-DISABLED
// health view, tierHealth(health) — AMEND-P1-COROLLARY) + one flat fallback
// child built with a NIL health registry (AMEND-P1: the flat child MUST
// ignore health entirely, not merely reuse the shared clusterHealth's OWN
// internal inPanic — see rationale below), and seeds a crypto-keyed PCG draw.
//
// WHY flat gets health=nil, not the shared clusterHealth (contrast
// locality.go's flat, which safely reuses the shared health because its
// wrapper-level bypass condition — cluster-wide aggregate fraction < 50% —
// is IDENTICAL to the flat leaf's OWN internal inPanic computation over the
// SAME allEndpoints slice, so they can never disagree by construction):
// priorityLB's bypass condition (capacitySum < 100, AMEND-P1) is a
// DIFFERENTLY-SHAPED computation than clusterHealth.inPanic's aggregate
// fraction < panicThreshold — the two CAN disagree in edge cases (e.g. an
// unbalanced-tier-size cluster where one check fires and the other doesn't).
// Building flat with health=nil sidesteps this divergence risk entirely: it
// always performs a pure per-host-count-uniform draw, matching the
// confirmed reference behavior (§11.1's 32/32/32/32/32-and-28/28/28/28/28
// evidence) exactly, regardless of what a hypothetical leaf-internal
// recomputation might conclude.
// opf/hasOPF: the SAME absent-vs-explicit-zero threading phase 52's
// D-LW-OPF0 established for ClusterLoadAssignment.Policy.overprovisioning_factor
// (a *wrapperspb.UInt32Value, per §5 — the wrapper's PRESENCE, not just its
// value, is significant: an explicit {value: 0} must be honored literally,
// collapsing EVERY tier's capacity to 0 and permanently forcing the AMEND-P1
// bypass, rather than silently substituting the 140 default). This is
// REUSED verbatim from locality.go's own newLocalityWeightedLB(endpoints,
// health, opf uint32, hasOPF bool, factory) signature (manager.go:491-499) —
// NOT re-derived; priority.go's manager.go call site must thread the SAME
// opfWrapper-presence check locality.go's wrap site already performs (§3.3).
func newPriorityLB(endpoints []Endpoint, health *clusterHealth, opf uint32, hasOPF bool, factory priorityLeafFactory) (*priorityLB, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newPriorityLBWithRNG(endpoints, health, opf, hasOPF, factory, rng)
}

// priorityLeafFactory is a LOCAL variation on subset.go's leafFactory
// (subset.go:131 — type leafFactory func(sub []Endpoint) (loadBalancer,
// error), reused as-is by locality.go): it accepts the health VIEW to build
// the leaf against as an explicit parameter, rather than a value baked into
// the closure at the manager.go call site — required so newPriorityLB can
// supply tierHealth(health) to tier children and nil to the flat child from
// the SAME factory (§12 D-P-FACTORY; manager.go's wrap-after-switch site
// defines the closure as func(sub []Endpoint, h *clusterHealth)
// (loadBalancer, error) { return buildLeafLB(c, name, sub, h) } — h is now a
// parameter, not captured).
type priorityLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)

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
		byPriority[ep.Priority] = append(byPriority[ep.Priority], ep)
	}
	sort.Slice(tiers, func(i, j int) bool { return tiers[i] < tiers[j] }) // ASCENDING — cascade order (D-P-DUP: merge-by-identity already folded duplicates above)
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

### 3.2 The seam REUSE — ZERO new `Pick` parameters (D-P... confirmed)

`loadBalancer.Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error)` (`loadbalancer.go:16-23`) stays BYTE-FOR-BYTE unchanged. `priorityLB.Pick` forwards all four parameters UNCHANGED to whichever child it delegates to (a tier's child, or the flat fallback) — the identical forwarding shape `localityWeightedLB.Pick` already uses. The ONLY new plumbing is at CONSTRUCTOR time (`priorityLeafFactory`'s explicit `h *clusterHealth` parameter, §3.1) — a build-time wiring choice, not a `Pick`-time one; no `WithX`/`xFrom` ctx-carry is added, for the identical reason phase 52 needed none.

### 3.3 Manager acceptance + composition rejects (ADR-0270; AMEND-P2)

`internal/cluster/manager.go`'s `buildCluster` gains a THIRD wrap-after-switch site, after the existing `lb_subset_config` and `locality_weighted_lb_config` wraps:

```go
priorityTiers := distinctPriorities(endpoints) // a small helper: len(set of ep.Priority across endpoints)
switch {
case len(priorityTiers) > 1 && c.GetCommonLbConfig().GetLocalityWeightedLbConfig() != nil:
	return nil, fmt.Errorf("cluster: %q: common_lb_config.locality_weighted_lb_config cannot be combined with multi-tier LocalityLbEndpoints.priority", name)
case len(priorityTiers) > 1 && c.GetLbSubsetConfig() != nil:
	return nil, fmt.Errorf("cluster: %q: lb_subset_config cannot be combined with multi-tier LocalityLbEndpoints.priority", name)
case len(priorityTiers) > 1:
	if health == nil {
		health = newClusterHealth(endpoints, parsePanicThreshold(c)) // AMEND-P1-COROLLARY-style health-registry-widening — the D-LW-HEALTHALLOC precedent (locality.go/phase 52), reapplied: priorityLB ALWAYS needs a per-tier healthy-fraction view, even with zero health_checks configured (a well-defined 100%-healthy-everywhere fast path)
	}
	// D-LW-OPF0's exact pattern, reused verbatim (NOT re-derived): the
	// wrapper's PRESENCE is checked BEFORE .GetValue() so an explicit
	// {value: 0} is honored literally (forcing every tier's capacity to 0,
	// permanently triggering the AMEND-P1 bypass) rather than silently
	// substituted with the 140 default — locality.go's own wrap site
	// performs the IDENTICAL check for the SAME proto field (manager.go:491-499).
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
```

**The health-registry widening (D-P-HEALTHALLOC — the D-LW-HEALTHALLOC precedent reapplied verbatim).** Exactly mirroring phase 52's own posture: a multi-tier-priority cluster ALWAYS needs `clusterHealth.availableCount`/callable, even with zero `health_checks` configured (a 100%-available-everywhere fast path is well-defined and cheap), so the wrap-after-switch allocates a `clusterHealth` unconditionally when priority tiers are detected. This is the SAME widened-condition side effect phase 52 documented for locality-weighted (`registerClusterMetrics`'s existing `if c.health != nil` block now ALSO fires for a priority-tiered cluster with zero `health_checks`) — a widened CONDITION on the two pre-existing stat names (`membership_healthy`, `lb_healthy_panic`), not a stat-surface COUNT change (§7 confirms the count stays 1200).

---

## 4. Framework primitives — 0 new framework seams + 1 NEW `Endpoint` dimension + 0 new packages + 0 new go.mod deps

- **Seam:** REUSED unchanged (§3.2). NO new context-carry function, NO `Pick` signature change. The ONLY new wiring is constructor-time (`priorityLeafFactory`'s explicit health parameter).
- **`Endpoint` dimension:** grows `Priority uint32` — the THIRD per-endpoint dimension after phase 38's `Metadata` and phase 52's `Locality`/`LocalityWeight` (`cluster.go`). Every `Endpoint{}` construction site (`extractEndpoints` + unit-test builders across `internal/cluster/*_test.go`) must be enumerated and updated at PLAN/IMPL — the phase 38/52 `reference_conn_wrap_method_no_promote`-generalized lesson, applied a THIRD time. `Addr()` is UNCHANGED.
- **`extractEndpoints` extension** (`manager.go:814-841`): captures `group.GetPriority()` per `LocalityLbEndpoints` GROUP (a `uint32`, defaulting to `0` when absent — AMEND-P3, no wrapper-type ambiguity to resolve), stamping every endpoint in that group with the tier. `group.GetLocality()`/`GetLoadBalancingWeight()` stay captured exactly as phase 52 left them (unrelated axis). `group.GetProximity()` stays discarded (unrelated, §2).
- **`ClusterLoadAssignment.Policy.overprovisioning_factor` reuse:** the SAME field phase 52 already reads in `buildCluster` (`la.GetPolicy().GetOverprovisioningFactor().GetValue()`) — threaded to `newPriorityLB` identically to how it is threaded to `newLocalityWeightedLB` (a cluster-wide policy field, not per-tier).
- **NEW packages: NONE.** `priority.go` lands in the EXISTING `internal/cluster` package (the `subset.go`/`locality.go` precedent).
- **NEW go.mod deps: NONE** (re-confirmed at this SPEC, §11.3 — `go mod tidy` stays clean).
- **NEW producer plane: NONE** — `LocalityLbEndpoints.priority` is cluster-scoped (on the cluster's `ClusterLoadAssignment`); unlike phase 38's subset, nothing attaches to an HTTP route.

---

## 5. Proto-field roster (per §11.3 D-P3)

| Field | Proto # | Type | Disposition |
|---|---|---|---|
| `LocalityLbEndpoints.priority` | 5 | `uint32` (plain scalar — NO wrapper type, unlike `overprovisioning_factor`) | THIS PHASE — captured into `Endpoint.Priority`; 0 = default/absent, no ambiguity |
| `LocalityLbEndpoints.locality` | 1 | `*core.v3.Locality` | ALREADY consumed (phase 52) — UNCHANGED |
| `LocalityLbEndpoints.load_balancing_weight` | 3 | `*wrapperspb.UInt32Value` | ALREADY consumed (phase 52) — UNCHANGED |
| `LocalityLbEndpoints.proximity` | 6 | `*wrapperspb.UInt32Value` | STAYS discarded (unrelated) |
| `ClusterLoadAssignment.Policy.overprovisioning_factor` | 3 (of `Policy`) | `*wrapperspb.UInt32Value`, default 140 | ALREADY consumed (phase 52) — REUSED verbatim by `priorityLB` |
| `ClusterLoadAssignment.Policy.weighted_priority_health` | 6 | `bool` | STAYS discarded — moot without locality composition (§2) |
| `ClusterLoadAssignment.Policy.endpoint_stale_after` | 4 | `*durationpb.Duration` | STAYS discarded — EDS-churn N/A project-wide |
| `ClusterLoadAssignment.Policy.drop_overloads` | 2 | `[]*Policy_DropOverload` | STAYS discarded — distinct traffic-shedding concept |

---

## 6. PARSE-REJECT roster (per §11.2 + ADR-0080)

### 6.1 Wording discipline + the new reject surface

Two NEW envoy-go-strict departure rejects (BOTH confirmed by live-probe to be departures from genuine, working reference behavior, NOT reference-parity — §11.2):

1. **BOTH multi-tier `LocalityLbEndpoints.priority` AND `common_lb_config.locality_weighted_lb_config` configured on the same cluster** → `cluster: %q: common_lb_config.locality_weighted_lb_config cannot be combined with multi-tier LocalityLbEndpoints.priority` (the reference accepts and MEANINGFULLY composes them — priority selects the tier, locality-weighting divides load within it — envoy-go over-rejects rather than build the unverified three-layer nesting, a recorded DEPARTURE).
2. **BOTH multi-tier `LocalityLbEndpoints.priority` AND `lb_subset_config` configured on the same cluster** → `cluster: %q: lb_subset_config cannot be combined with multi-tier LocalityLbEndpoints.priority` (the reference accepts and composes — priority-tier-aware subset structures — envoy-go over-rejects, a recorded DEPARTURE).

A single-tier cluster (every endpoint at the SAME `priority`, including the overwhelming common case of an entirely-absent field) triggers NEITHER reject and builds NO `priorityLB` wrapper at all — behavior-neutral for every existing cluster (mirroring how `localityWeightedLB` only engages when its own trigger config is present).

### 6.2 NO new producer-side reject arm

Identical posture to phase 52: `LocalityLbEndpoints.priority` is cluster-scoped with no per-route producer, so there is no NEW reject on the route/producer side beyond the two above.

### 6.3 NON-reject dispositions

- A cluster with endpoints spanning only ONE `priority` value (including all-absent, defaulting to 0) → NOT a reject; no `priorityLB` wrapper built (the overwhelming common case today).
- Multiple `LocalityLbEndpoints` groups declaring the SAME `priority` value → NOT a reject; their endpoints MERGE into one tier (§12 D-P-DUP).
- The four deferred `ClusterLoadAssignment.Policy` fields (§5) → NOT a reject; silently accepted-ignored, UNCHANGED from before this phase.

---

## 7. Stat surface — CONFIRMED ZERO delta (per §11.5 D-P5 + AMEND-P5)

Stat surface **STAYS 1200 (+0)** — the family's FOURTH zero-stat-delta LB phase (after phase 38.2's weighted-clusters leg, phase 35's random leg, and phase 52's locality-weighted). NO `registerClusterMetrics` change beyond the health-registry-widening CONDITION already documented at phase 52 and reapplied here (§3.3's D-P-HEALTHALLOC — a widened condition on the two PRE-EXISTING stat names `membership_healthy`/`lb_healthy_panic`, not a count change). The reference exposes NO dedicated priority-scoped LB stat group (§11.5) — only its own, unrelated per-priority circuit-breaker buckets (`circuit_breakers.default`/`.high`). `lb_healthy_panic` continues to increment from `priorityLB.Pick`'s own bypass branch exactly as `localityWeightedLB.Pick`'s panic branch already does — no new counter, just a new call site of the SAME existing counter handle.

---

## 8. Differential fixture taxonomy (+1: `0096` HTTP cross-side static + full-failover)

### 8.1 `0096-lb-priority` (cross-side; hard 100%/0% boundary assertions + cross-side deterministic stats)

Topology: an HTTP listener routing to a STATIC cluster with TWO `LocalityLbEndpoints` groups at distinct `priority` values (0 and 1), N backend hosts each (mirroring the confirmed-working probe shape: 5 per tier), `health_checks` configured (HTTP `/healthz`, short interval/thresholds for fast convergence, the phase-39 substrate).

- **Arm (a) — static (all healthy):** send ≥300 requests; assert **100%** of traffic lands on priority-0 hosts, **0%** on priority-1 — a hard boundary assertion (capacity sum = 100+100 = 200 ≥ 100, no bypass; the corrected cascade gives P0 the full 100% share, capped at `min(100, remaining=100)`, leaving P1's remaining budget at exactly 0 — §11.1/§11.4 confirm this is the EXACT, not approximate, outcome).
- **Arm (b) — full failover:** fail active health checks on ALL of priority-0's hosts (reusing the phase-39 poll-to-convergence + `reference_health_check_propagation_warmup` warmup gate), THEN assert **100%** of traffic fails over to priority-1, **0%** stays on (the now fully-unhealthy) priority-0 — the confirmed boundary case (capacity sum = 0+100 = 100, exactly at the boundary, CONFIRMED via live probe to NOT trigger the AMEND-P1 bypass — a clean 0%/100% split, not a flattened spray across all 10 hosts).
- **Cross-side deterministic stats:** `membership_total`/`membership_healthy` (both arms) + `upstream_rq_total` (cross-equal total request count, both sides driven identically) via `StatsAsserter` (`reference_differential_asserter_dispatch`).
- **Deliberate breaks (both proven LIVE with `-count=1`, `reference_differential_break_protocol_count1`):** (i) skip the priority-tier capture (leaving flat `buildLeafLB` output, all endpoints merged into one implicit tier) → defeats arm (a)'s 100%/0% split (produces a mixed distribution across all 10 hosts instead); (ii) skip the health-degradation step in arm (b) → defeats the 100% failover assertion (traffic stays on priority-0).
- **NOT exercised by this fixture (deliberately, per the BRAINSTORM's Q4 + this SPEC's own scope discipline):** the AMEND-P1 capacity-shortfall bypass (neither arm's capacity sum ever drops below 100 — both arms sit at 200 and exactly-100 respectively) and the AMEND-P1-COROLLARY per-tier no-local-panic property (neither arm has an asymmetrically PARTIALLY-degraded tier, i.e. some-but-not-all of one tier's hosts unhealthy) are BOTH covered instead by dedicated UNIT tests (§10 Task table) — mirroring how phase 52's zero-total-weight fallback was unit-tested, not fixture-proven, and consistent with the BRAINSTORM's explicit choice to keep the differential to a hard 2-arm proof rather than expand its scope.

### 8.2 NO boot-reject cross-side dir (AMEND-P2)

Both new rejects (§6.1) are envoy-go-strict DEPARTURES from a reference that ACTIVELY COMPOSES both combinations (not merely "accepts and ignores," as phase 52's own rejects were) — there is nothing on the reference side to boot-reject differentially. Per `reference_differential_fixture_dispatch_constraint`, these are SUBJECT-SIDE UNIT TESTS (`manager_test.go`), NOT a cross-side boot-reject fixture dir.

### 8.3 NO new BackendKind + NO new fuzzer (family expectations, reconfirmed)

- **BackendKind:** tail STAYS **38** (`H2GoawayResponder`, 39 kinds 0-indexed). `0096` reuses the plain HTTP backends from `0062`/`0063`/`0064`/`0095` for traffic + the health-check-capable responder from the phase-39/52 precedent for arm (b)'s controlled failure.
- **Fuzzer:** STAYS **52**. No wire decode (priority derives from validated xDS bootstrap config, not untrusted bytes). A tier-grouping/cascade-draw property test (random tier/health combinations → the resolver never panics, the corrected cascade never assigns negative or over-100 loads, an all-tiers-exhausted cluster falls back to flat) is a unit-level candidate FOLDED into `priority_test.go`, not a `Fuzz*` corpus entry (§1.1 AMEND-P6).

### 8.4 Total

97 → **98** (`0096-lb-priority`, cross-side, both arms in ONE fixture dir per `reference_differential_fixture_dispatch_constraint` — both arms exercise the SAME runner branch).

---

## 9. Behavior-contract delta (the phase-53 bundle; ADR-0052 atomic landing)

A new `### Load balancer — priority (LocalityLbEndpoints.priority)` section lands in `docs/envoy-go/BEHAVIOR_CONTRACT.md` immediately after the existing `### Load balancer — locality-weighted (locality_weighted_lb_config)` section, mirroring that section's shape: opening italic intro citing ADR-0270 + the seam-reuse framing; the wrap-after-switch acceptance + the health-registry-widening note; the `Endpoint.Priority` dimension; the CONFIRMED per-tier capacity formula + the CORRECTED cascade + the AMEND-P1 capacity-shortfall bypass (with the exact condition, `Σ min(100, OPF×healthy_fraction_i) < 100`, called out explicitly as NOT the classic `healthy_panic_threshold`); the AMEND-P1-COROLLARY per-tier no-local-panic property; the two explicit composition rejects + their departure framing (noting the reference's OWN natural nesting, for context); the confirmed-zero stat delta; the differential proof shape; the deferred surface list from §2. Landed atomically with the ADR-0270 body at IMPL (ADR-0052's discipline).

---

## 10. Per-task structure (~11-12 tasks; the PLAN decomposes)

| # | Task | SPEC anchor |
|---|---|---|
| 1 | Scaffolding + baseline re-verification (fixtures/fuzzers/stat/BackendKind counts fresh at HEAD) | §11 preconditions |
| 2 | `Endpoint.Priority` field + `extractEndpoints` capture + EVERY `Endpoint{}` construction-site update (test builders across `internal/cluster/*_test.go`) | §4 |
| 3 | `internal/cluster/priority.go`: `tierHealth` + `tierCapacity` + `cascadeLoads` as pure, independently-unit-testable functions | §3.1 |
| 4 | `internal/cluster/priority.go`: `priorityGroup`/`priorityLB` + `newPriorityLB`/`newPriorityLBWithRNG` (grouping + duplicate-priority merge + sort-ascending + tier/flat child construction via `priorityLeafFactory`) | §3.1 |
| 5 | `internal/cluster/priority.go`: `Pick` (the AMEND-P1 bypass check + the AMEND-P4 corrected cascade draw + delegation) | §3.1 |
| 6 | `manager.go` wrap-after-switch: `distinctPriorities` helper, both composition-reject arms, the `priorityLB` wrap (incl. the health-registry-widening note, §3.3), the `priorityLeafFactory` closure change at the call site | §3.3/§6 |
| 7 | Unit tests: `tierCapacity`/`cascadeLoads` against the confirmed data tables (§11.1/§11.4, all 8+ two-tier scenarios and both 3-tier scenarios), the AMEND-P1 bypass boundary (exactly-100 does NOT bypass), the AMEND-P1-COROLLARY per-tier no-local-panic property (a partially-degraded tier restricts to its healthy hosts, never sprays across its unhealthy ones), both reject arms | §11.1/§11.4/§6 |
| 8 | `test/fixtures/0096-lb-priority` driver: the 2-tier×N-host topology, the health-check-toggle harness (reusing/extending the phase-39/52 convergence-poll + warmup helper) | §8.1 |
| 9 | `0096` assertions: arm (a) static 100%/0%, arm (b) full-failover 100%/0%, cross-side `StatsAsserter` | §8.1 |
| 10 | `0096` deliberate breaks (both, `-count=1`) + 20-run flake-soak + `-race` | §8.1 |
| 11 | `docs/envoy-go/BEHAVIOR_CONTRACT.md` delta + ADR-0270 body (§Decision/§Consequences) + final six-gate + ROADMAP row 53 flip | §9/§13 |

Anticipated **~230-330 prod LoC** (the `priority.go` construct ~150-180 LoC + the manager/extractEndpoints deltas ~35-45 LoC + test code separately) — comfortably under the ADR-0045 gate, confirming §3.0's no-split disposition.

---

## 11. SPEC-time empirical-pin block (D-P1..D-P6 — executed IN-SESSION 2026-07-07, two parallel probe agents against `contrib-v1.37.2`, one on the panic/cascade track, one on the composition/surface/stats track, per `reference_docker_probe_bridge_network`)

### Summary disposition table (6 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| D-P1 | Panic/per-tier-health interaction | REFUTED BOTH BRAINSTORM readings; a THIRD mechanism found | AMEND-P1 (+ AMEND-P1-COROLLARY) |
| D-P2 | Composition acceptance + reject wording | CONFIRMED LIVE — reference genuinely COMPOSES both | AMEND-P2 |
| D-P3 | Config surface + defaults + go.mod | CONFIRMED LIVE | AMEND-P3 |
| D-P4 | Effective-load cascade formula | CONFIRMED per-tier; REFUTED + CORRECTED at the remainder step | AMEND-P4 |
| D-P5 | Stat surface delta | REFUTED DOWN (zero, not small) | AMEND-P5 |
| D-P6 | LoC/task envelope + fuzzer | CONFIRMED (self-assessed at SPEC; no live probe needed) | AMEND-P6 |

### 11.1 D-P1 (SPEC-BLOCKING — THE load-bearing empirical unknown) — the panic/per-tier-health interaction: REFUTED (both readings), a THIRD mechanism confirmed

Setup: reference Envoy `contrib-v1.37.2`, **`--concurrency 1`** (methodologically load-bearing — see the surprising-finding note below), a STRICT_DNS 2-tier (then 3-tier) cluster, 5 Python HTTP backends per tier, HTTP active health check on `/healthz` (1s interval, threshold=1 for unambiguous fast convergence), default `healthy_panic_threshold` (50%) and default overprovisioning factor (140%). All measurements: 300 requests per scenario after gauge convergence + settle + a discarded warmup burst, ground-truthed via per-host `rq_total` deltas from the admin `/clusters` endpoint (not response bodies).

**Decisive evidence, the requested (e)-vs-(g) comparison:**
- **(e)** P0=1/5 healthy (20%), P1=5/5 healthy (100%): tier capacities 28% / 100% → capacity SUM = 128 ≥ 100 → **no bypass** (`lb_healthy_panic` delta = **0**). P0's traffic went **100% to the single healthy host (92/300)**; the 4 unhealthy P0 hosts got **0 each**. P1 split evenly (~42 each of its 5 hosts).
- **(g)** P0=1/5 healthy (20%) AND P1=1/5 healthy (20%): capacities 28% / 28% → capacity SUM = 56 < 100 → **bypass fires** (`lb_healthy_panic` delta = **300/300**, every request). Traffic went **flat across ALL 10 hosts by raw host count**: P0's 5 hosts each got ~32 (32,32,32,32,32 — INCLUDING the 4 unhealthy ones); P1's 5 hosts each got ~28 (28,28,28,28,28 — INCLUDING the 4 unhealthy ones). P0=160/300 (53.3%), P1=140/300 (46.7%) — essentially a 50/50 split by host count (5 vs 5), completely disregarding health and the tier-cascade formula that governed every other scenario.

**A disambiguating control ((h): P0=1/5 healthy (20%), P1=4/5 healthy (80%))** rules out "any single tier under 50% triggers global bypass": capacities 28%/100% → sum 128 ≥ 100 → **no bypass** (confirmed, delta=0), normal waterfall applied (P0=79/300=26.3%, entirely to its 1 healthy host; P0's 4 unhealthy hosts and P1's 1 unhealthy host both got 0). Proves a tier being at 20% health does NOT by itself trigger anything — what matters is whether *some other tier* has enough spare capacity to push the cross-tier sum to 100.

**A 3-tier control nails the mechanism precisely:** P0=1/5(20%), P1=1/5(20%), P2=5/5(100%) — capacity sum = 28+28+100=156 ≥ 100 → **no bypass** (delta=0) even though the RAW AGGREGATE healthy-host fraction here is only 7/15=46.7% (BELOW the classic 50% panic threshold!). This is the decisive evidence against the "aggregate-fraction-vs-50%" reading. Observed split: P0=87/300(29.0%), P1=86/300(28.7%), P2=127/300(42.3%) — matches the predicted 28/28/44 cascade almost exactly, zero traffic to any unhealthy host anywhere.

**Boundary check:** scenario (f), P0=0/5(0%), P1=5/5(100%) — capacity sum = 0+100 = EXACTLY 100 → confirmed **NOT** a bypass (clean 0%/100% split, delta=0) — the shortfall condition is strict `<`, not `<=`.

**Confirmed mechanism (envoy-go's implementation target, §3.1):** `Σ_i min(100, overprovisioning_factor × healthy_fraction_i) < 100` triggers a full bypass — priority AND per-host health BOTH ignored, flat host-count-uniform draw across every host in every tier, reusing the EXISTING `lb_healthy_panic` stat. Otherwise, the normal per-tier waterfall applies and **unhealthy hosts get ZERO traffic in every tier**, even at 20% tier health — refuting AMEND-P1-COROLLARY's naive alternative (a per-tier-scoped "local panic" that would flatten within a degraded tier) just as decisively as it refutes the aggregate-50%-threshold reading.

### 11.2 D-P2 (SPEC-BLOCKING) — composition acceptance + reject wording: CONFIRMED LIVE (reference genuinely COMPOSES both)

**Combo A — priority (0/1) + `common_lb_config.locality_weighted_lb_config`:** cluster with priority-0 = 2 localities (region-a weight 80, region-b weight 20), priority-1 = 1 locality (region-c weight 100). `envoy --mode validate` → `configuration OK`; real boot → `/ready` LIVE. 200 requests, all backends healthy: **100% landed on priority-0** (0 reached priority-1); WITHIN priority-0, the split was 159/41 ≈ 79.5%/20.5%, matching the 80/20 weights closely. Confirms the reference's natural "priority selects the tier, locality-weighting divides within it" nesting works and is exercised. Separately: killing priority-0's backend CONTAINERS (no health check configured) did NOT trigger failover — `membership_healthy` stayed at full count, hosts stayed `health_flags::healthy` — connection failures ALONE never mark a STATIC host unhealthy; only active health_checks/outlier_detection do (consistent with the existing phase-39 health-check-propagation-warmup finding — this matters for §3.3's design: priority failover REQUIRES health checking to be configured, matching what the `0096` fixture already assumes).

**Combo B — priority (0/1) + `lb_subset_config`** (subset selector on key `tier`, both endpoints tagged `tier: canary`, route uses `metadata_match`): `envoy --mode validate` → `configuration OK`; booted, `/ready` LIVE. 30/30 requests went to the priority-0 endpoint, 0 to priority-1, even though both matched the SAME subset key. `lb_subsets_created`=33 (subset structures built per priority level internally), `lb_subsets_selected`=30. Confirms subset selection is priority-tier-aware internally, and priority ordering still gates the final pick.

**Both are genuine envoy-go-strict DEPARTURES** (§6.1): the reference accepts AND meaningfully composes both configs; it does not itself reject either combination.

### 11.3 D-P3 (SPEC-BLOCKING) — config surface + defaults + go.mod: CONFIRMED LIVE

- `endpoint/v3/endpoint_components.pb.go:356`: `Priority uint32` (field 5, plain scalar, `protobuf:"varint,5,opt,name=priority,proto3"`) — confirmed NO wrapper/pointer type (unlike `overprovisioning_factor`'s `*wrapperspb.UInt32Value`, `endpoint.pb.go:168`, field 3) — "explicitly 0" and "omitted" are indistinguishable at the proto layer, both the Go zero value.
- Priority-omitted defaults to tier 0: confirmed empirically (an omitted-priority endpoint group lands in the same tier as an explicit `priority: 0` group, per `/clusters` + `/stats` observation) — matches the documented default and proto3 zero-value semantics.
- `go mod tidy` in `/home/esa/git/envoy-go`: `git status --porcelain go.mod go.sum` empty both before and after (md5sums identical) — **confirmed no-op**, ZERO new dependency needed.

### 11.4 D-P4 (SPEC-BLOCKING) — the effective-load cascade formula: CONFIRMED per-tier, REFUTED + CORRECTED at the remainder step

Per-tier formula `capacity_i = min(100, 100 × min(1, overprovisioning_factor/100 × healthy_fraction_i))` (i.e. `min(100, OPF × healthy_fraction_i)` with OPF as a percent-scale number like 140) is CONFIRMED.

Full predicted-vs-observed table (2-tier scenarios, n=300 each, all panic-free unless noted):

| Scenario | P0 healthy | Predicted P0% | Observed P0% (count) | Observed P1% (count) | Within noise? |
|---|---|---|---|---|---|
| a baseline | 5/5=100% | 100.0% | 100.00% (300/300) | 0.00% (0/300) | exact |
| b | 4/5=80% | 100.0% (capped) | 100.00% (300/300) | 0.00% (0/300) | exact |
| c | 3/5=60% | 84.0% | 83.67% (251/300) | 16.33% (49/300) | yes (0.15σ) |
| d | 2/5=40% | 56.0% | 59.33% (178/300) | 40.67% (122/300) | yes (1.17σ) |
| e | 1/5=20% | 28.0% | 30.67% (92/300) | 69.33% (208/300) | yes (1.03σ) |
| f | 0/5=0% | 0.0% | 0.00% (0/300) | 100.00% (300/300) | exact — boundary, no bypass |
| h (P1=80%) | 1/5=20% | 28.0% | 26.33% (79/300) | 73.67% (221/300) | yes (0.64σ) |
| g (bypass) | 1/5=20% | N/A (bypass) | 49.7% flat-by-host-count (160/300) | 50.3% (140/300) | formula does not apply — bypass override, §11.1 |

3-tier scenarios (the AMEND-P4 divergence point):

| Scenario | Naive-recursive reading | CORRECTED (min-against-remaining-budget) reading | Observed |
|---|---|---|---|
| P0=40%,P1=60%,P2=100% (capacities 56/84/100) | P0=56, P1=84×44%=36.96%, P2=7.04% | P0=56, P1=min(84,44)=44, P2=0 | P0=55.0%(165), P1=45.0%(135), **P2=0.0%(0)** — matches CORRECTED |
| P0=20%,P1=20%,P2=100% (capacities 28/28/100) | P0=28, P1=28×72%=20.16%, P2=51.84% | P0=28, P1=min(28,72)=28, P2=44 | P0=29.0%(87), P1=28.7%(86), P2=42.3%(127) — matches CORRECTED |

All 2-tier deviations from the (per-tier-only) formula fall within ~1.2σ of binomial sampling noise at n=300; both 3-tier scenarios decisively distinguish and confirm the CORRECTED cascade over the naive-recursive one — the two readings coincide at exactly 2 tiers (which is why the BRAINSTORM's hypothesis "looked" confirmed there) but diverge at 3+.

### 11.5 D-P5 (SPEC-BLOCKING) — stat surface delta: REFUTED DOWN

A full `/stats` scrape of a traffic-served, 2-priority-tier cluster, grepped case-insensitively for "priority," found ONLY: the reference's pre-existing, UNRELATED per-priority circuit-breaker stat buckets (`cluster.<name>.circuit_breakers.default.*` = priority-0's bucket; `cluster.<name>.circuit_breakers.high.*` = priority-1's bucket; 5 stats each: cx_open/cx_pool_open/rq_open/rq_pending_open/rq_retry_open — CB limits, unrelated to LB composition) plus incidental substring matches of the test cluster's own name. `membership_total`/`membership_healthy` are single AGGREGATE gauges per cluster, confirmed NOT split or tagged per priority tier anywhere in `/stats` (a 3-host, 2-tier cluster reported both as `3`/`3` with no per-tier breakdown). The per-host `priority::N` field is visible ONLY on the `/clusters` admin TEXT endpoint, never as a `/stats` counter/gauge. **Stat surface delta: 0 (confirmed).**

### 11.6 D-P6 — the LoC/task envelope + fuzzer decision: CONFIRMED (self-assessed)

§10's 11-task breakdown and ~230-330 prod LoC estimate are comfortably under the ADR-0045 gate — CONFIRMING §3.0's no-split disposition. No dedicated fuzzer warranted (§8.3) — confirmed, no wire-decode surface anywhere in this construct.

### 11.7 The seam-reuse confirmation: CONFIRMED (zero `Pick`-signature change)

The `priorityLB` sketch (§3.1) confirms `loadBalancer.Pick`'s existing 4-parameter signature is sufficient — the ONLY new plumbing (the `priorityLeafFactory`'s explicit health-view parameter, `tierHealth`/`nil`) is constructor-time wiring, never touching `Pick` itself. No `WithX` ctx carry, no interface change, no `Cluster` exported-surface change.

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-P-DUP** — the duplicate-priority corner (two `LocalityLbEndpoints` groups declaring the SAME `priority` value within one `ClusterLoadAssignment`). Self-answered here: MERGE their endpoints into the SAME tier (grouping is by identity, priority carries no per-group scalar to conflict over — simpler than locality's own D-LW-DUP, which had to resolve a conflicting WEIGHT via last-write-wins). An unusual, essentially degenerate config shape not exercised by either probe (both used one group per distinct priority) — the PLAN documents this as a known, low-stakes, untested assumption, the same posture phase 52 took for D-LW-DUP.
- **D-P-FACTORY** — the `priorityLeafFactory` local type variation (§3.1): whether to introduce this AS a phase-53-local type (this SPEC's choice, minimal blast radius — `subset.go`'s existing `leafFactory` type, reused by `locality.go`, and its call sites stay completely untouched) versus retrofitting `leafFactory` itself to accept an explicit health parameter (which would ALSO let `subset.go`/`locality.go` be refactored to the same shape, but touches code outside this phase's scope for no phase-53-required benefit). The PLAN confirms the local-type choice at implementation time; a wholesale `leafFactory` refactor is explicitly NOT bundled into this phase.
- **D-P-RETROFIT** — whether `locality.go`'s own documented, accepted "child-local-panic coverage boundary" (STATE.md, phase 52 PLAN done) should be retroactively fixed with the SAME `tierHealth`-style pattern this phase introduces, now that this SPEC's live probe supplies concrete evidence that the reference does NOT apply per-group local panic. Self-answered: OUT of THIS phase's scope (a different file/construct — `locality.go` is phase 52's, already shipped and documented; fixing it is a candidate LOW-RISK future maintenance item, not bundled here to keep phase 53's blast radius to `priority.go` + the manager wrap site only).

---

## 13. ADR continuity — the ADR-0270 §Context DRAFT (anchored here; the full entry lands at the IMPL)

**ADR-0270 §Context DRAFT (the `priority` load-balancing policy):** `LocalityLbEndpoints.priority` (`envoy.config.endpoint.v3`, field 5 — a plain `uint32` tier number, 0 = highest priority) drives Envoy's priority-tier overflow/failover mechanism: traffic is drawn from the lowest-numbered tier as long as it has enough live capacity, spilling to the next tier as that capacity degrades — the SEVENTH Load-balancing-family construct, reusing `ClusterLoadAssignment.Policy.overprovisioning_factor` (already a phase-52 dependency, default 140) as the SAME per-group availability primitive phase 52 introduced, applied one axis over (tier instead of locality) via a cascading waterfall rather than a flat weighted draw. Landed as `internal/cluster/priority.go`'s `priorityLB`, a WRAPPER `loadBalancer` (the phase-38/52 `subsetLB`/`localityWeightedLB` structural precedent: per-tier child `loadBalancer`s built by the existing `buildLeafLB` factory, ALL FIVE leaf policies supported uniformly) built once at cluster construction, wrapping whatever child the `lb_policy` switch produced (a THIRD wrap-after-switch site in `manager.go`'s `buildCluster`). Needs **ZERO new `Pick` parameters** — `loadBalancer.Pick(hashKey, hasHash, match, hasMatch)` stays byte-for-byte unchanged; the ONLY new wiring is at construction time. `Endpoint` grows a `Priority uint32` dimension (the THIRD per-endpoint dimension after phase 38's `Metadata` and phase 52's `Locality`/`LocalityWeight`). Live-probed against `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227) and this SPEC's most consequential finding: the BRAINSTORM's two candidate panic/per-tier-health hypotheses were BOTH REFUTED — the reference's real bypass condition is `Σ_i min(100, overprovisioning_factor × healthy_fraction_i) < 100` (a cluster-wide capacity-SHORTFALL check, computed by summing every tier's OWN capacity-capped-at-100 effective load), NOT the classic `healthy_panic_threshold` (a 3-tier control at 46.7% raw aggregate healthy fraction — well under the classic 50% threshold — showed NO bypass, because the capacity sum comfortably exceeded 100) and NOT an independent per-tier local-panic concept (a tier at 20% healthy correctly restricted its share to its own healthy hosts, its unhealthy hosts getting zero traffic — never internally flattening). When the capacity sum DOES drop below 100, the reference abandons BOTH priority ordering and per-host health filtering, spreading traffic FLAT and UNIFORM by raw host count across every host in every tier — reusing its EXISTING `lb_healthy_panic` stat for this mechanism (confirming no new stat is needed). envoy-go reproduces this bit-for-bit: `priorityLB.Pick` computes the capacity sum once, delegates to a HEALTH-NIL flat fallback child on bypass (a deliberate departure from `localityWeightedLB`'s own flat child, which safely reuses the shared cluster health because ITS bypass condition and its flat leaf's own internal check are identical by construction — priorityLB's differently-shaped bypass condition cannot make that same guarantee, so its flat child is built health-nil to avoid any divergence risk), and otherwise cascades load across tiers via a CORRECTED formula (each tier's assigned load capped by its OWN effective capacity as an absolute percent of the REMAINING budget, not scaled by a fraction of it — a 3-tier live probe decisively distinguished and refuted the naive recursive-fraction reading the BRAINSTORM's hypothesis text implied, which coincidentally matches the corrected reading at exactly 2 tiers). A genuinely NEW implementation primitive (`tierHealth`, a panic-disabled per-tier health VIEW sharing the same live per-host state) ensures no individual tier can internally flatten itself — a documented improvement over `locality.go`'s own accepted-but-unfixed "child-local-panic" coverage boundary (phase 52), made possible here because this SPEC's live probe supplies the concrete evidence phase 52 lacked, and costs nothing against the zero-new-`Pick`-parameter design (a constructor-time wiring choice only). TWO envoy-go-strict departure rejects, BOTH confirmed live against a reference that doesn't merely accept but MEANINGFULLY COMPOSES both combinations (priority selecting the tier, then locality-weighting or subset-selection operating within it): a cluster configuring both multi-tier `priority` AND `common_lb_config.locality_weighted_lb_config` is rejected; a cluster configuring both multi-tier `priority` AND `lb_subset_config` is rejected — both deferred, real architecture questions for a future dedicated brainstorm, mirroring phase 52's own subset-composition-deferral posture. The stat surface gains **ZERO new stats** (live-confirmed — no dedicated priority-scoped LB stat group exists in the reference, only its own unrelated per-priority circuit-breaker buckets), the FOURTH zero-stat-delta phase in the Load-balancing family. Proven via the cross-side `0096-lb-priority` differential fixture: a hard 100%/0% static arm plus a hard 100%/0% full-failover arm (reusing the phase-39 poll-to-convergence-and-warmup pattern) — a deliberately narrower proof shape than locality-weighted's banded-ratio pair, since failover is a hard boundary rather than a statistical ratio, avoiding the band-margin re-tuning risk phases 35/52 both hit. NO new package (`internal/cluster`), NO new go.mod dependency, NO new producer plane, NO new BackendKind, NO new fuzzer. ADR-0024 (the per-cluster LB-state-scope discipline) stays UNAMENDED.

§Decision/§Consequences bodies land at the phase-53 IMPL per ADR-0044 (next-free after phase 53 ≈ **ADR-0271**). The PLAN/IMPL may surface additional ADRs — anticipated NONE (a single-ADR reuse-shape phase, the phase-35/37/52 precedent; §11.7 confirms zero seam change).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

- Stat surface: **1200 (+0)**, CONFIRMED (§11.5) — down from the BRAINSTORM's anticipated "small 0-2" delta.
- Differential fixtures: **97 → 98** at IMPL (`0096-lb-priority`).
- Fuzzers: **52 (+0)** — CONFIRMED, no new fuzzer.
- BackendKind tail: **38 (+0)** — CONFIRMED, no new kind.
- DECISIONS.md tail: **ADR-0269** at this SPEC (docs-only); → **ADR-0270** at the phase-53 IMPL (next-free ADR-0271).
- ROADMAP row 53: STAYS `in-progress` (lifecycle-state 1 → 2 at this SPEC-DONE commit); flips `done` at the phase-53 IMPL six-gate (NO parent rollup, a single flat row per ADR-0106).
- ZERO new Go packages, ZERO new go.mod modules (re-confirmed, §11.3).
- spec-document-reviewer gate applies at this SPEC.

Next → the **phase-53 PLAN** (`superpowers:writing-plans` — decompose §10 into bite-sized TDD tasks; FINAL ADR-0045 gate re-check, anticipated to re-confirm the no-split disposition of §3.0).
