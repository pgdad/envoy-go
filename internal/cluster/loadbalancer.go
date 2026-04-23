package cluster

import "sync/atomic"

// loadBalancer is the unexported per-cluster LB interface. Phase 02 has one
// implementation: roundRobin. Future phases that introduce LEAST_REQUEST,
// RANDOM, RING_HASH, MAGLEV, etc. add new types here.
type loadBalancer interface {
	Pick() (Endpoint, error)
}

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

func (rr *roundRobin) Pick() (Endpoint, error) {
	if len(rr.endpoints) == 0 {
		return Endpoint{}, errNoEndpoints
	}
	i := rr.counter.Add(1) - 1
	return rr.endpoints[int(i)%len(rr.endpoints)], nil
}
