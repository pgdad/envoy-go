package cluster

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"fmt"
	mathrand "math/rand/v2"
	"sync"
	"sync/atomic"
)

// leastRequest is an un-weighted power-of-two-choices (P2C) load balancer that
// mirrors Envoy v1.37.2's LeastRequestLoadBalancer::unweightedHostPickNChoices
// (SPEC §3.1 / AMEND-L6): choiceCount independent draws of rng()%n WITH
// replacement (no dedup, no clamp at choiceCount >= n); pick the host with the
// fewest outstanding active counts; STRICT < comparison on the RAW active count;
// the FIRST-DRAWN candidate wins ties. The per-endpoint active counters are
// LB-internal state, NOT registry stats (SPEC §7 — zero stat delta). cx-as-rq
// (AMEND-L2): each pick holds +1 on its endpoint from selection until the
// returned release fires (conn-producing paths: at final conn Close).
//
// The weighted bias formula (weight/(active+1)^bias) belongs to Envoy's EDF
// path, which engages ONLY with unequal host weights or active slow-start —
// the equal-weight static MVP takes pure P2C and active_request_bias never
// enters the computation (AMEND-L6; bias/slow_start are DEPARTURE-rejected at
// parse time — manager.go).
type leastRequest struct {
	endpoints   []Endpoint
	active      []atomic.Int64 // index-aligned with endpoints
	choiceCount int
	rng         func() uint64 // injectable for deterministic tests (the upstream mock posture)
}

// newLeastRequest constructs a leastRequest over endpoints with the given P2C
// choiceCount, seeding a crypto-seeded math/rand/v2 PCG once at construction
// (D-S34-2). The crypto/rand read error is threaded out (effectively unreachable
// on Linux getrandom; boot-fail is the safe disposition). Callers in
// manager.buildCluster surface the error as a cluster build error.
func newLeastRequest(endpoints []Endpoint, choiceCount int) (*leastRequest, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newLeastRequestWithRNG(endpoints, choiceCount, rng), nil
}

// newLeastRequestWithRNG is the injectable constructor used by unit tests to
// supply a deterministic draw sequence (AMEND-L6).
func newLeastRequestWithRNG(endpoints []Endpoint, choiceCount int, rng func() uint64) *leastRequest {
	return &leastRequest{
		endpoints:   endpoints,
		active:      make([]atomic.Int64, len(endpoints)),
		choiceCount: choiceCount,
		rng:         rng,
	}
}

// newPCGRNG seeds a math/rand/v2 PCG from two crypto/rand uint64 words and
// returns a MUTEX-GUARDED draw closure. Pick is called concurrently across
// downstream connections; math/rand/v2.Rand is not per-source concurrency-safe,
// so the mutex serializes draws (cheap; per-cluster contention only). Each
// cluster gets its own independent stream.
func newPCGRNG() (func() uint64, error) {
	var seed [16]byte
	if _, err := cryptorand.Read(seed[:]); err != nil {
		return nil, fmt.Errorf("cluster: least_request: seed rng: %w", err)
	}
	r := mathrand.New(mathrand.NewPCG(
		binary.LittleEndian.Uint64(seed[0:8]),
		binary.LittleEndian.Uint64(seed[8:16]),
	))
	var mu sync.Mutex
	return func() uint64 {
		mu.Lock()
		defer mu.Unlock()
		return r.Uint64()
	}, nil
}

// Pick implements loadBalancer. See the type doc for the P2C semantics.
func (lr *leastRequest) Pick(_ uint64, _ bool, _ SubsetMatch, _ bool) (Endpoint, func(), error) {
	n := len(lr.endpoints)
	if n == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	best := int(lr.rng() % uint64(n))
	bestActive := lr.active[best].Load()
	for i := 1; i < lr.choiceCount; i++ {
		cand := int(lr.rng() % uint64(n))
		candActive := lr.active[cand].Load()
		if candActive < bestActive { // STRICT <: first-drawn keeps ties (AMEND-L6)
			best, bestActive = cand, candActive
		}
	}
	lr.active[best].Add(1)
	var once sync.Once
	release := func() { once.Do(func() { lr.active[best].Add(-1) }) }
	return lr.endpoints[best], release, nil
}
