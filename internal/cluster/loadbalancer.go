package cluster

import "sync/atomic"

// loadBalancer is the unexported per-cluster LB interface. Phase 02 has one
// implementation: roundRobin; phase 34 adds leastRequest. Future phases that
// introduce RANDOM, RING_HASH, MAGLEV, etc. add new types here.
//
// Pick returns the selected endpoint plus a release func the caller MUST invoke
// exactly once when the picked unit of work completes (conn-producing paths: at
// final conn Close; non-conn paths: immediately). release is always non-nil,
// including on the error path; implementations guard against double-release.
// ADR-0232 (the LB acquire/release seam; OPTION C — the exported Cluster
// surface stays byte-stable). ADR-0235 (the hash-key pick-input extension);
// ADR-0239 (the subset-match pick-input extension).
type loadBalancer interface {
	// Pick selects an endpoint. hashKey carries a request-derived consistent-hash
	// key when hasHash is true (ring_hash/maglev). match carries the route's
	// resolved metadata subset match when hasMatch is true (subset); the leaf
	// policies ignore (match, hasMatch), only subsetLB consumes it. The release
	// func is the ADR-0232 RELEASE half (unchanged).
	Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error)
}

// noopRelease is the shared release for LB policies that hold no per-pick state
// (roundRobin). It is a no-op; the ADR-0024 per-cluster counter is untouched.
var noopRelease = func() {}

// roundRobin is a per-cluster round-robin LB. ADR-0024 codifies the per-cluster
// counter scope decision. The formula i := counter.Add(1) - 1 then mod-index
// makes the first pick endpoints[0]; this property is asserted by unit tests
// and is internal correctness, not a differential equivalence claim (upstream's
// RR is per-worker with randomized starting offset — see BEHAVIOR_CONTRACT
// "## TCP proxy" subsection added by Task 8).
type roundRobin struct {
	endpoints []Endpoint
	counter   atomic.Uint64
	// health is the build-time-injected per-cluster health registry (ADR-0243).
	// nil for a cluster with no health_checks -> the fast path (byte-stable with
	// the pre-39.1 behavior). Set in buildLeafLB AFTER construction.
	health *clusterHealth
}

func (rr *roundRobin) Pick(_ uint64, _ bool, _ SubsetMatch, _ bool) (Endpoint, func(), error) {
	n := len(rr.endpoints)
	if n == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	if rr.health == nil {
		i := rr.counter.Add(1) - 1
		return rr.endpoints[int(i)%n], noopRelease, nil
	}
	if rr.health.inPanic(rr.endpoints) {
		rr.health.panicInc()
		i := rr.counter.Add(1) - 1
		return rr.endpoints[int(i)%n], noopRelease, nil
	}
	for tries := 0; tries < n; tries++ {
		i := rr.counter.Add(1) - 1
		ep := rr.endpoints[int(i)%n]
		if rr.health.available(ep) {
			return ep, noopRelease, nil
		}
	}
	return Endpoint{}, noopRelease, errNoEndpoints
}
