// Package cluster implements the priority-tier overflow/failover
// load-balancing wrapper (phase 53, ADR-0270).
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

import (
	"math"
	"sort"
)

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
//
//nolint:unused // called from manager.go's wrap-after-switch site (Task 6)
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
