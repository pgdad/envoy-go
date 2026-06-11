package cluster

// randomLB is a stateless uniform-pick load balancer mirroring Envoy v1.37.2's
// RandomLoadBalancer::peekOrChoose (SPEC §3.1 / AMEND-R5): ONE draw rng()%n,
// pick endpoints[i]; it consults NO active counters and holds NO per-pick state
// (the contrast with leastRequest's P2C). It REUSES the ADR-0232 seam UNCHANGED,
// returning the shared noopRelease (the seam's first reuse — ADR-0232 §Consequences
// "the non-keyed RANDOM drops in at zero cost").
type randomLB struct {
	endpoints []Endpoint
	rng       func() uint64 // injectable for deterministic tests (the upstream mock posture)
}

// newRandom constructs a randomLB over endpoints, REUSING the package-private
// newPCGRNG (leastrequest.go) verbatim — a mutex-guarded crypto-seeded
// math/rand/v2 PCG (AMEND-R5; the helper needs NO change). The crypto/rand read
// error threads out → boot-fail (the leastRequest disposition). NOTE (D-S35-2):
// newPCGRNG's seed-error message reads "cluster: least_request: seed rng" — we
// accept the shared message rather than wrap it; the path is reachable only on a
// crypto/rand failure (effectively unreachable on Linux getrandom).
func newRandom(endpoints []Endpoint) (*randomLB, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newRandomWithRNG(endpoints, rng), nil
}

// newRandomWithRNG is the injectable constructor used by unit tests to supply a
// deterministic draw sequence (mirrors newLeastRequestWithRNG minus choiceCount).
func newRandomWithRNG(endpoints []Endpoint, rng func() uint64) *randomLB {
	return &randomLB{endpoints: endpoints, rng: rng}
}

// Pick implements loadBalancer. One uniformly-random draw; no active-count
// consultation; the shared noopRelease (random holds no per-pick state).
func (r *randomLB) Pick() (Endpoint, func(), error) {
	n := len(r.endpoints)
	if n == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints // the roundRobin/leastRequest parity (AMEND-R5)
	}
	i := int(r.rng() % uint64(n))
	return r.endpoints[i], noopRelease, nil
}
