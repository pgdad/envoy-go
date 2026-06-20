package router

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/esalaine/envoy-go/internal/cluster"
)

// A deterministic injected RNG lets us hit each entry's cumulative bucket.
func newSeqRNG(vals ...uint64) func() uint64 {
	i := 0
	return func() uint64 { v := vals[i%len(vals)]; i++; return v }
}

func TestWeightedSelector_PickBoundaries(t *testing.T) {
	// weights {50,30,20} → cumulative {50,80,100}, total 100.
	// r in [0,50) → 0 ; [50,80) → 1 ; [80,100) → 2.
	sel := newWeightedSelectorWithRNG([]uint32{50, 30, 20}, newSeqRNG(0, 49, 50, 79, 80, 99))
	want := []int{0, 0, 1, 1, 2, 2}
	for i, w := range want {
		if got := sel.pick(); got != w {
			t.Fatalf("draw %d: pick()=%d want %d", i, got, w)
		}
	}
}

func TestWeightedSelector_ExplicitZeroNeverPicked(t *testing.T) {
	// weights {1,0,1} → cumulative {1,1,2}, total 2. Entry 1 (weight 0) has an
	// empty bucket [1,1) so r<1 → 0, 1<=r<2 → 2; index 1 is unreachable.
	sel := newWeightedSelectorWithRNG([]uint32{1, 0, 1}, newSeqRNG(0, 1))
	if got := sel.pick(); got != 0 {
		t.Fatalf("r=0 pick()=%d want 0", got)
	}
	if got := sel.pick(); got != 2 {
		t.Fatalf("r=1 pick()=%d want 2 (the weight-0 entry must never be picked)", got)
	}
}

func TestWeightedSelector_SingleEntry(t *testing.T) {
	sel := newWeightedSelectorWithRNG([]uint32{7}, newSeqRNG(0, 3, 6))
	for i := 0; i < 3; i++ {
		if got := sel.pick(); got != 0 {
			t.Fatalf("single-entry draw %d pick()=%d want 0", i, got)
		}
	}
}

func TestNewWeightedSelector_DistributionTracksWeights(t *testing.T) {
	// Production RNG (crypto-seeded). Over many draws the empirical distribution
	// must track the weights within a wide band (property check, not a fixture).
	sel, err := NewWeightedSelector([]uint32{50, 30, 20})
	if err != nil {
		t.Fatalf("NewWeightedSelector: %v", err)
	}
	const n = 20000
	counts := make([]int, 3)
	for i := 0; i < n; i++ {
		counts[sel.pick()]++
	}
	checks := []struct {
		idx    int
		loFrac float64
		hiFrac float64
	}{{0, 0.45, 0.55}, {1, 0.25, 0.35}, {2, 0.15, 0.25}}
	for _, c := range checks {
		f := float64(counts[c.idx]) / float64(n)
		if f < c.loFrac || f > c.hiFrac {
			t.Errorf("entry %d frac %.3f outside [%.2f,%.2f]", c.idx, f, c.loFrac, c.hiFrac)
		}
	}
}

// newEchoClusters starts n httptest backends each responding with "backend:<idx>"
// in the body, builds a *cluster.Cluster per backend (via the existing
// singleEndpointCluster helper), and returns:
//   - clusters: slice of *cluster.Cluster, one per backend
//   - bodyDecoder: func([]byte) int — decodes an ActionResponse.Body slice and
//     returns the backend index (0..n-1), or -1 on unrecognized body.
func newEchoClusters(t *testing.T, n int) ([]*cluster.Cluster, func([]byte) int) {
	t.Helper()
	clusters := make([]*cluster.Cluster, n)
	for i := 0; i < n; i++ {
		idx := i // capture
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = fmt.Fprintf(w, "backend:%d", idx)
		}))
		t.Cleanup(srv.Close)
		clusters[i] = singleEndpointCluster(t, srv.Listener.Addr().String())
	}
	bodyDecoder := func(body []byte) int {
		for i := 0; i < n; i++ {
			if string(body) == fmt.Sprintf("backend:%d", i) {
				return i
			}
		}
		return -1
	}
	return clusters, bodyDecoder
}

// mustGET constructs a GET *http.Request for the given path, targeting
// "http://upstream". It is the weighted-cluster test counterpart to the
// inline request-build calls in router_test.go.
func mustGET(t *testing.T, path string) *http.Request {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://upstream"+path, nil)
	if err != nil {
		t.Fatalf("http.NewRequestWithContext: %v", err)
	}
	return req
}

// TestH1WeightedClusterAction_PicksPerSelector builds 3 single-endpoint clusters
// backed by httptest echo servers (body "backend:N"), constructs a
// weightedSelector with a deterministic RNG sequence, and verifies that each
// H1WeightedClusterAction call dispatches to the cluster picked by the selector.
func TestH1WeightedClusterAction_PicksPerSelector(t *testing.T) {
	clusters, bodyDecoder := newEchoClusters(t, 3)
	// RNG sequence 0,0,1,2 with total weight 3 → picks 0,0,1,2.
	sel := newWeightedSelectorWithRNG([]uint32{1, 1, 1}, newSeqRNG(0, 0, 1, 2))
	wcs := []WeightedCluster{
		{Cluster: clusters[0]},
		{Cluster: clusters[1]},
		{Cluster: clusters[2]},
	}
	act := H1WeightedClusterAction(wcs, nil, sel, nil)
	want := []int{0, 0, 1, 2}
	for i, w := range want {
		resp, _, err := act(context.Background(), mustGET(t, "/"))
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		if idx := bodyDecoder(resp.Body); idx != w {
			t.Errorf("draw %d: body=%q landed on backend %d want %d", i, resp.Body, idx, w)
		}
	}
}

// TestH2WeightedClusterAction_CompilesAndNonNil is a compile/smoke test: it
// asserts that H2WeightedClusterAction returns a non-nil H2Action over a
// 1-entry slice. The H2 integration path is exercised by the 0065 differential
// fixture (Task 8); this test only proves the constructor builds without panic.
func TestH2WeightedClusterAction_CompilesAndNonNil(t *testing.T) {
	clusters, _ := newEchoClusters(t, 1)
	sel := newWeightedSelectorWithRNG([]uint32{1}, newSeqRNG(0))
	wcs := []WeightedCluster{{Cluster: clusters[0]}}
	act := H2WeightedClusterAction(wcs, nil, sel, nil)
	if act == nil {
		t.Fatal("H2WeightedClusterAction returned nil H2Action")
	}
}
