package cluster

import (
	"fmt"
	"sort"
)

// ringHashLB is a per-cluster consistent-hash load balancer implementing the
// Ketama ring algorithm, mirroring Envoy v1.37.2's RingHashLoadBalancer
// (source/extensions/load_balancing_policies/ring_hash/ring_hash_lb.cc). The
// ring is a sorted slice of (hash, endpoint) points built once at construction:
// ceil(minRingSize/N) points per endpoint (equal-weight MVP), each keyed by
// hash("addr:port_i"). Pick binary-searches the first ring point >= the
// request hash key (wrapping past the end); when hasHash is false it draws a
// uniform random ring position from the injected rng. It satisfies the
// loadBalancer interface (the Task-3 widened Pick). ADR-0236 (the ring_hash
// policy decision); ADR-0232 (the RELEASE half — reuses noopRelease unchanged);
// ADR-0235 (the PICK-INPUT-half hash-key extension).
type ringHashLB struct {
	endpoints []Endpoint
	ring      []ringEntry
	rng       func() uint64

	// Gauge values computed at build (registered by the Task-5 manager).
	size       uint64 // total ring points = len(ring)
	minPerHost uint64 // smallest per-host ring-entry count
	maxPerHost uint64 // largest per-host ring-entry count

	// health is the build-time-injected per-cluster health registry (ADR-0243);
	// nil -> the fast path (byte-stable). Set in buildLeafLB after construction.
	health *clusterHealth
}

// ringEntry is one point on the Ketama ring: a hash value and the index of the
// endpoint it maps to (into ringHashLB.endpoints).
type ringEntry struct {
	hash uint64
	ep   int
}

// hashFunc selects the ring-point hash algorithm. Envoy supports XX_HASH
// (default) and MURMUR_HASH_2; this same enum is reused by the Task-5 manager
// parse.
type hashFunc int

const (
	hashXX hashFunc = iota
	hashMurmur
)

// murmurSeed is Envoy's fixed MURMUR_HASH_2 seed for ring construction
// (MurmurHashSeed in ring_hash_lb.cc).
const murmurSeed uint64 = 0xc70f6907

// ringHashCfg carries the parsed ring_hash policy parameters. It is reused by
// the Task-5 manager parse — keep it in this file.
type ringHashCfg struct {
	minRingSize uint64
	maxRingSize uint64
	hashFunc    hashFunc
}

// var-assert: ringHashLB satisfies the loadBalancer interface.
var _ loadBalancer = (*ringHashLB)(nil)

// newRingHash builds a ringHashLB seeded from a fresh PCG rng. D-S36-2: it
// accepts newPCGRNG's shared seed-error message verbatim (no wrap).
func newRingHash(endpoints []Endpoint, cfg ringHashCfg) (*ringHashLB, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newRingHashWithRNG(endpoints, cfg, rng), nil
}

// newRingHashWithRNG builds the ring with an injected rng (tests). The ring is
// immutable after construction so Pick is safe for concurrent use.
func newRingHashWithRNG(endpoints []Endpoint, cfg ringHashCfg, rng func() uint64) *ringHashLB {
	n := len(endpoints)
	if n == 0 {
		return &ringHashLB{endpoints: endpoints, rng: rng}
	}

	// ceil(minRingSize / N) entries per endpoint (equal-weight; D-S36-5).
	entriesPerHost := (cfg.minRingSize + uint64(n) - 1) / uint64(n)

	ring := make([]ringEntry, 0, entriesPerHost*uint64(n))
	perHost := make([]uint64, n)
	for j := 0; j < n; j++ {
		addr := endpoints[j].Addr()
		for i := uint64(0); i < entriesPerHost; i++ {
			key := fmt.Sprintf("%s_%d", addr, i)
			var h uint64
			if cfg.hashFunc == hashXX {
				h = xxHash64([]byte(key))
			} else {
				h = murmurHash2([]byte(key), murmurSeed)
			}
			ring = append(ring, ringEntry{hash: h, ep: j})
			perHost[j]++
		}
	}

	// Stable sort: Envoy keeps insertion order on equal hashes (vanishingly rare).
	sort.SliceStable(ring, func(a, b int) bool { return ring[a].hash < ring[b].hash })

	// Per-host tally is the source of truth for the min/max gauges (equal for
	// equal weight, but the tally is honest if weighting is added later).
	minPerHost, maxPerHost := perHost[0], perHost[0]
	for _, c := range perHost[1:] {
		if c < minPerHost {
			minPerHost = c
		}
		if c > maxPerHost {
			maxPerHost = c
		}
	}

	return &ringHashLB{
		endpoints:  endpoints,
		ring:       ring,
		rng:        rng,
		size:       uint64(len(ring)),
		minPerHost: minPerHost,
		maxPerHost: maxPerHost,
	}
}

// Pick selects the endpoint whose ring point is the first >= hashKey (wrapping
// past the end). With hasHash false it draws a uniform random ring position
// from rng. The release func is noopRelease (ringHashLB holds no per-pick
// state); it is non-nil on every path including the error path.
func (rh *ringHashLB) Pick(hashKey uint64, hasHash bool, _ SubsetMatch, _ bool) (Endpoint, func(), error) {
	if len(rh.endpoints) == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	if !hasHash {
		hashKey = rh.rng()
	}
	m := sort.Search(len(rh.ring), func(i int) bool { return rh.ring[i].hash >= hashKey })
	if m == len(rh.ring) {
		m = 0 // wrap
	}
	if rh.health == nil || rh.health.inPanic(rh.endpoints) {
		if rh.health != nil {
			rh.health.panicInc()
		}
		return rh.endpoints[rh.ring[m].ep], noopRelease, nil
	}
	// Walk the ring forward from m to the next healthy host (ADR-0243).
	for k := 0; k < len(rh.ring); k++ {
		idx := rh.ring[(m+k)%len(rh.ring)].ep
		if rh.health.available(rh.endpoints[idx]) {
			return rh.endpoints[idx], noopRelease, nil
		}
	}
	return Endpoint{}, noopRelease, errNoEndpoints
}
