package cluster

import "sort"

// maglevLB is a per-cluster consistent-hash load balancer implementing the
// Maglev lookup-table algorithm, mirroring Envoy v1.37.2's MaglevTable
// (source/extensions/load_balancing_policies/maglev/maglev_lb.cc). The table is
// a fixed-size []int (each slot an endpoint index) built once at construction:
// hosts sorted by Addr() ascending, each with offset = xxHash64(addr) % M and
// skip = (xxHash64Seed(addr,1) % (M-1)) + 1, populated via (offset+skip*next)%M
// until all M slots fill (terminates: M prime => skip coprime to M => full cycle).
// Pick indexes table[hashKey % M]; with hasHash false it draws a random table
// index (rng() % M — the reference random() mirror). ADR-0238 (the maglev
// policy); ADR-0232 (the RELEASE half — reuses noopRelease unchanged);
// ADR-0235 (the PICK-INPUT-half hash key — reused).
type maglevLB struct {
	endpoints []Endpoint
	table     []int // table[slot] = index into endpoints; len == tableSize
	rng       func() uint64
	tableSize uint64

	// Gauge values computed at build (registered by the manager — AMEND-M4).
	minPerHost uint64 // = floor(M/N) under equal weight
	maxPerHost uint64 // = ceil(M/N)  under equal weight

	// health is the build-time-injected per-cluster health registry (ADR-0243);
	// nil -> the fast path (byte-stable). Set in buildLeafLB after construction.
	health *clusterHealth
}

// maglevCfg carries the parsed maglev policy parameters (the single table_size).
type maglevCfg struct {
	tableSize uint64
}

// var-assert: maglevLB satisfies the loadBalancer interface.
var _ loadBalancer = (*maglevLB)(nil)

// Pick selects table[hashKey % M]. With hasHash false it draws a random table
// index (rng() % M — the Envoy random()-LB fallback; near-uniform, never on the
// differential path). The release func is noopRelease (maglevLB holds no
// per-pick state); non-nil on every path.
func (mg *maglevLB) Pick(hashKey uint64, hasHash bool, _ SubsetMatch, _ bool) (Endpoint, func(), error) {
	if len(mg.endpoints) == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	if !hasHash {
		hashKey = mg.rng()
	}
	slot := hashKey % mg.tableSize
	if mg.health == nil || mg.health.inPanic(mg.endpoints) {
		if mg.health != nil {
			mg.health.panicInc()
		}
		return mg.endpoints[mg.table[slot]], noopRelease, nil
	}
	// Walk the table slots forward from slot to the next healthy host (ADR-0243).
	for k := uint64(0); k < mg.tableSize; k++ {
		idx := mg.table[(slot+k)%mg.tableSize]
		if mg.health.available(mg.endpoints[idx]) {
			return mg.endpoints[idx], noopRelease, nil
		}
	}
	return Endpoint{}, noopRelease, errNoEndpoints
}

// newMaglev builds a maglevLB seeded from a fresh PCG rng. Like newRingHash it
// accepts newPCGRNG's shared seed-error message verbatim (no wrap).
func newMaglev(endpoints []Endpoint, cfg maglevCfg) (*maglevLB, error) {
	rng, err := newPCGRNG()
	if err != nil {
		return nil, err
	}
	return newMaglevWithRNG(endpoints, cfg, rng), nil
}

// newMaglevWithRNG builds the table with an injected rng (tests). The table is
// immutable after construction so Pick is safe for concurrent use.
func newMaglevWithRNG(endpoints []Endpoint, cfg maglevCfg, rng func() uint64) *maglevLB {
	m := cfg.tableSize
	n := len(endpoints)
	if n == 0 {
		return &maglevLB{endpoints: endpoints, rng: rng, tableSize: m}
	}

	// Sort endpoint INDICES by Addr() ascending — Envoy std::sorts the build
	// entries by the key string before populating (maglev_lb.cc:107-120). This
	// determines the build order, hence the table layout (D-M2).
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return endpoints[order[a]].Addr() < endpoints[order[b]].Addr()
	})

	offset := make([]uint64, n)
	skip := make([]uint64, n)
	next := make([]uint64, n)
	count := make([]uint64, n)
	for k, ei := range order {
		key := []byte(endpoints[ei].Addr()) // "IP:PORT" == address()->asString()
		offset[k] = xxHash64(key) % m
		skip[k] = (xxHash64Seed(key, 1) % (m - 1)) + 1
	}

	table := make([]int, m)
	for i := range table {
		table[i] = -1
	}
	var filled uint64
	for filled < m { // equal-weight populate: one slot per host per iteration
		for k := range order {
			if filled == m {
				break
			}
			c := (offset[k] + skip[k]*next[k]) % m
			for table[c] != -1 {
				next[k]++
				c = (offset[k] + skip[k]*next[k]) % m
			}
			table[c] = order[k]
			next[k]++
			count[k]++
			filled++
		}
	}

	minPerHost, maxPerHost := count[0], count[0]
	for _, c := range count[1:] {
		if c < minPerHost {
			minPerHost = c
		}
		if c > maxPerHost {
			maxPerHost = c
		}
	}
	return &maglevLB{
		endpoints:  endpoints,
		table:      table,
		rng:        rng,
		tableSize:  m,
		minPerHost: minPerHost,
		maxPerHost: maxPerHost,
	}
}

// isPrime reports whether n is prime by trial division to sqrt(n). Maglev
// requires a prime table_size so that each per-host skip (in [1, M-1]) is
// coprime to M, giving the populate loop a full M-cycle that fills every slot
// (SPEC §3.1). Adequate for n <= 5000011 (the PGV cap). The reference itself
// rejects a non-prime table_size ("The table size of maglev must be prime
// number", maglev_lb.cc) — envoy-go's reject is reference PARITY (AMEND-M5).
func isPrime(n uint64) bool {
	if n < 2 {
		return false
	}
	if n%2 == 0 {
		return n == 2
	}
	for d := uint64(3); d*d <= n; d += 2 {
		if n%d == 0 {
			return false
		}
	}
	return true
}
