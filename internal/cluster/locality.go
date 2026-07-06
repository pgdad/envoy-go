package cluster

import "math"

// internal/cluster/locality.go implements the locality-weighted
// load-balancing wrapper (phase 52, ADR-0269).
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
	flat                   loadBalancer // panic-mode + zero-total-weight fallback; spans ALL endpoints
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
