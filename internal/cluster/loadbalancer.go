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
// surface stays byte-stable).
type loadBalancer interface {
	Pick() (Endpoint, func(), error)
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
}

func (rr *roundRobin) Pick() (Endpoint, func(), error) {
	if len(rr.endpoints) == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	i := rr.counter.Add(1) - 1
	return rr.endpoints[int(i)%len(rr.endpoints)], noopRelease, nil
}
