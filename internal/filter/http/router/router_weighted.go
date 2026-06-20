package router

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	mathrand "math/rand/v2"
	"net/http"
	"sync"

	"github.com/esalaine/envoy-go/internal/cluster"
	"github.com/esalaine/envoy-go/internal/filter/hcm/h2"
)

// weightedSelector draws a per-request entry index by integer weight, mirroring
// upstream Envoy's WeightedClusterEntry (a random value in [0, totalWeight) then
// a cumulative-weight walk) and envoy-go's randomLB (a per-action mutex-guarded
// PCG draw). It is the FIRST per-request RNG model at the router layer (ADR-0241).
// total_weight is the SUM of entry weights — the proto's total_weight is
// deprecated/ignored (SPEC §11.1 / AMEND-WC1).
type weightedSelector struct {
	cumulative  []uint32 // cumulative[i] = Σ weights[0..i]; cumulative[last] == totalWeight
	totalWeight uint32
	rng         func() uint64 // injectable for deterministic tests (the randomLB posture)
}

// NewWeightedSelector builds a selector over the per-entry weights using a
// crypto-seeded production RNG. The crypto/rand read error threads out → the
// caller boot-fails (the randomLB/leastRequest disposition). Callers MUST have
// validated Σweights > 0 (the producer's §6 reject) before calling — a zero
// total would make rng()%0 panic.
func NewWeightedSelector(weights []uint32) (*weightedSelector, error) {
	rng, err := newWeightedRNG()
	if err != nil {
		return nil, err
	}
	return newWeightedSelectorWithRNG(weights, rng), nil
}

// newWeightedSelectorWithRNG is the injectable constructor used by unit tests to
// supply a deterministic draw sequence (the random.go newRandomWithRNG precedent).
func newWeightedSelectorWithRNG(weights []uint32, rng func() uint64) *weightedSelector {
	cum := make([]uint32, len(weights))
	var total uint32
	for i, w := range weights {
		total += w
		cum[i] = total
	}
	return &weightedSelector{cumulative: cum, totalWeight: total, rng: rng}
}

// pick returns the chosen entry index: r = rng() % totalWeight, then the first
// cumulative bucket strictly greater than r. A weight-0 entry has an empty
// bucket (cumulative[i] == cumulative[i-1]) so r < cumulative[i] can never first
// fire on it → it is never picked.
func (s *weightedSelector) pick() int {
	r := uint32(s.rng() % uint64(s.totalWeight))
	for i, c := range s.cumulative {
		if r < c {
			return i
		}
	}
	return len(s.cumulative) - 1 // unreachable: r < totalWeight == cumulative[last]
}

// newWeightedRNG mirrors cluster.newPCGRNG (deliberately duplicated — ~12 LoC —
// to avoid exporting a cluster internal for a router-layer need; D-WC-IMPL-1):
// a math/rand/v2 PCG seeded from two crypto/rand uint64 words, returning a
// MUTEX-GUARDED draw closure (pick is called concurrently across downstream
// connections; math/rand/v2.Rand is not concurrency-safe).
func newWeightedRNG() (func() uint64, error) {
	var seed [16]byte
	if _, err := rand.Read(seed[:]); err != nil {
		return nil, err
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

// WeightedCluster is one resolved entry of a RouteAction.weighted_clusters route:
// a cluster handle + its merged subset match (route-level ⊕ ClusterWeight.metadata_match).
// The weight itself is consumed by the selector (cumulative weights) and is not
// stored here. Exported so the hcm producer (buildWeightedRouterAction) can build
// the entry slice and hand it to the router-package dispatch constructors.
type WeightedCluster struct {
	Cluster     *cluster.Cluster
	SubsetMatch cluster.SubsetMatch
}

// H1WeightedClusterAction returns an Action closure that, per request, picks one
// weighted entry by integer weight (selector.pick — weighted-random, the randomLB
// model) and runs the EXISTING H1 cluster dispatch with the chosen entry's cluster
// + subset match + the shared route-level hashPolicies. It PRE-BUILDS one
// *routerAction per entry at construction (NO per-request alloc; the filter field
// stays nil exactly as H1ClusterAction's chain-mediated path). ADR-0241.
func H1WeightedClusterAction(wcs []WeightedCluster, hps []HashPolicy, sel *weightedSelector, rp *RetryPolicy) Action {
	actions := make([]*routerAction, len(wcs))
	for i, wc := range wcs {
		actions[i] = &routerAction{cluster: wc.Cluster, hashPolicies: hps, subsetMatch: wc.SubsetMatch, rp: rp}
	}
	return func(ctx context.Context, req *http.Request) (ActionResponse, cluster.Endpoint, error) {
		// 42.1 Task 8: the weighted path has its OWN dispatch (it does not route
		// through the H1ClusterAction closure), so the retry switch is added here
		// explicitly. Each per-entry *routerAction carries the shared rp (threaded
		// at construction, Task 4); a non-nil rp runs the retry loop for the picked
		// entry. nil rp ⇒ the byte-stable single-attempt path.
		a := actions[sel.pick()]
		if a.rp != nil {
			return retryExecutorH1(ctx, a, req)
		}
		return doH1ClusterAction(ctx, a, req)
	}
}

// H2WeightedClusterAction is the H2 sibling (the H2ClusterAction precedent). The
// selector is SHARED with the H1 constructor (a route is H1 or H2 per listener;
// the caller passes the same *weightedSelector to both — only one path runs).
func H2WeightedClusterAction(wcs []WeightedCluster, hps []HashPolicy, sel *weightedSelector, rp *RetryPolicy) H2Action {
	actions := make([]*routerActionH2, len(wcs))
	for i, wc := range wcs {
		actions[i] = &routerActionH2{cluster: wc.Cluster, hashPolicies: hps, subsetMatch: wc.SubsetMatch, rp: rp}
	}
	return func(ctx context.Context, req h2.H2Request) (ActionResponse, cluster.Endpoint, error) {
		// 42.1 Task 8: the weighted H2 path has its OWN dispatch (it does not route
		// through the H2ClusterAction closure), so the retry switch is added here
		// explicitly — mirroring the H1 weighted constructor above. The picked
		// per-entry *routerActionH2 carries the shared rp; non-nil ⇒ retry loop.
		a := actions[sel.pick()]
		if a.rp != nil {
			return retryExecutorH2(ctx, a, req)
		}
		return doH2ClusterAction(ctx, a, req)
	}
}
