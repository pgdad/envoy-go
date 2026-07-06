package cluster

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pgdad/envoy-go/internal/stats"
)

func TestValueEqual_NumbersAreDoubleValued(t *testing.T) {
	// int 1 ≡ double 1.0 (ProtobufWkt::Value has a single numeric kind).
	one := SubsetValue{Kind: subsetNumber, Num: 1}
	onePointZero := SubsetValue{Kind: subsetNumber, Num: 1.0}
	if !valueEqual(one, onePointZero) {
		t.Error("1 and 1.0 must compare equal (numbers are double-valued)")
	}
	if valueEqual(one, SubsetValue{Kind: subsetNumber, Num: 2}) {
		t.Error("1 and 2 must not compare equal")
	}
}

func TestValueEqual_TypedScalars(t *testing.T) {
	s := SubsetValue{Kind: subsetString, Str: "v1"}
	if valueEqual(s, SubsetValue{Kind: subsetNumber, Num: 1}) {
		t.Error("string v1 must not equal number 1 (typed)")
	}
	if !valueEqual(s, SubsetValue{Kind: subsetString, Str: "v1"}) {
		t.Error("same string must be equal")
	}
	b := SubsetValue{Kind: subsetBool, Bool: true}
	if !valueEqual(b, SubsetValue{Kind: subsetBool, Bool: true}) {
		t.Error("same bool must be equal")
	}
	if valueEqual(b, SubsetValue{Kind: subsetBool, Bool: false}) {
		t.Error("true must not equal false")
	}
}

func TestSubsetMatch_KeyCanonicalIsSortedAndStable(t *testing.T) {
	a := NewSubsetMatch(map[string]SubsetValue{
		"stage":   {Kind: subsetString, Str: "prod"},
		"version": {Kind: subsetNumber, Num: 1},
	})
	b := NewSubsetMatch(map[string]SubsetValue{
		"version": {Kind: subsetNumber, Num: 1.0}, // 1.0 ≡ 1
		"stage":   {Kind: subsetString, Str: "prod"},
	})
	if a.Key() != b.Key() {
		t.Errorf("canonical keys differ under reordering / int-vs-double: %q vs %q", a.Key(), b.Key())
	}
	c := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})
	if a.Key() == c.Key() {
		t.Error("different matches must have different keys")
	}
}

func TestScalarsFromStruct_LowersScalarsRejectsNonScalar(t *testing.T) {
	s, _ := structpb.NewStruct(map[string]any{
		"version": "v1",
		"weight":  float64(7),
		"canary":  true,
		"nested":  map[string]any{"x": 1}, // non-scalar → reported
		"list":    []any{1, 2},            // non-scalar → reported
	})
	scalars, nonScalar := ScalarsFromStruct(s)
	if scalars["version"].Str != "v1" || scalars["weight"].Num != 7 || !scalars["canary"].Bool {
		t.Errorf("scalar lowering wrong: %+v", scalars)
	}
	if _, ok := scalars["nested"]; ok {
		t.Error("nested struct must NOT appear in scalars")
	}
	if len(nonScalar) != 2 {
		t.Errorf("nonScalar keys = %v, want the 2 non-scalar keys (nested, list)", nonScalar)
	}
}

func TestScalarsFromStruct_NilIsEmpty(t *testing.T) {
	scalars, nonScalar := ScalarsFromStruct(nil)
	if len(scalars) != 0 || len(nonScalar) != 0 {
		t.Errorf("nil struct → empty/empty, got %v / %v", scalars, nonScalar)
	}
}

// ---------------------------------------------------------------------------
// Task 6: subsetLB enumeration + build tests
// ---------------------------------------------------------------------------

// rrFactory is a buildLeafLB-shaped factory that builds a roundRobin child for
// tests. Signature matches leafFactory.
func rrFactory(sub []Endpoint) (loadBalancer, error) { return &roundRobin{endpoints: sub}, nil }

// epMD constructs an Endpoint with explicit metadata for test fixtures.
func epMD(host string, port uint32, kv map[string]SubsetValue) Endpoint {
	return Endpoint{Host: host, Port: port, Metadata: kv}
}

func TestSubsetLB_EnumeratesOneSubsetPerValueTuple(t *testing.T) {
	eps := []Endpoint{
		epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}),
		epMD("127.0.0.1", 9002, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}),
		epMD("127.0.0.1", 9003, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v2"}}),
	}
	cfg := lbSubsetCfg{fallback: fallbackAnyEndpoint, selectors: [][]string{{"version"}}}
	s := newSubsetLB(eps, cfg, rrFactory)
	if s.numSubsets != 2 { // v1, v2
		t.Errorf("numSubsets = %d, want 2", s.numSubsets)
	}
	v1 := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})
	if _, ok := s.subsets[v1.Key()]; !ok {
		t.Error("missing v1 subset")
	}
}

func TestSubsetLB_HostMissingKeyExcludedFromSelector(t *testing.T) {
	eps := []Endpoint{
		epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}, "zone": {Kind: subsetString, Str: "a"}}),
		epMD("127.0.0.1", 9002, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}), // no zone
	}
	cfg := lbSubsetCfg{fallback: fallbackNoFallback, selectors: [][]string{{"version", "zone"}}}
	s := newSubsetLB(eps, cfg, rrFactory)
	if s.numSubsets != 1 { // only 9001 carries both keys → one {v1,a} subset
		t.Errorf("numSubsets = %d, want 1 (9002 excluded — missing zone)", s.numSubsets)
	}
}

func TestSubsetLB_FallbackChildren(t *testing.T) {
	eps := []Endpoint{
		epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}),
		epMD("127.0.0.1", 9002, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v2"}}),
	}
	sel := [][]string{{"version"}}
	if newSubsetLB(eps, lbSubsetCfg{fallback: fallbackNoFallback, selectors: sel}, rrFactory).fallback != nil {
		t.Error("NO_FALLBACK → nil fallback child")
	}
	if newSubsetLB(eps, lbSubsetCfg{fallback: fallbackAnyEndpoint, selectors: sel}, rrFactory).fallback == nil {
		t.Error("ANY_ENDPOINT → non-nil fallback child")
	}
	def := map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}
	if newSubsetLB(eps, lbSubsetCfg{fallback: fallbackDefaultSubset, defaultSubset: def, selectors: sel}, rrFactory).fallback == nil {
		t.Error("DEFAULT_SUBSET with a matching default → non-nil fallback child")
	}
	zero := map[string]SubsetValue{"version": {Kind: subsetString, Str: "v9"}}
	if newSubsetLB(eps, lbSubsetCfg{fallback: fallbackDefaultSubset, defaultSubset: zero, selectors: sel}, rrFactory).fallback != nil {
		t.Error("DEFAULT_SUBSET matching zero hosts → nil fallback (NO_FALLBACK behavior)")
	}
}

// recordingFactory captures the endpoint slices passed to each child-build call.
// The factory is NOT concurrency-safe; tests use it single-threadedly (newSubsetLB
// is synchronous). The key used to record is caller-assigned (e.g. "fallback" or
// the subset match key); since the factory itself does not know the key, the test
// inspects the recorded slices by index of call order after construction.
type recordingFactory struct {
	calls [][]Endpoint // one entry per factory call, in call order
}

func (r *recordingFactory) factory(sub []Endpoint) (loadBalancer, error) {
	// Copy the slice so the recorded value is not aliased by the caller.
	cp := make([]Endpoint, len(sub))
	copy(cp, sub)
	r.calls = append(r.calls, cp)
	return &roundRobin{endpoints: cp}, nil
}

// hostsInSlice returns the set of "host:port" addresses in a slice.
func hostsInSlice(eps []Endpoint) map[string]bool {
	m := make(map[string]bool, len(eps))
	for _, e := range eps {
		m[e.Addr()] = true
	}
	return m
}

func TestSubsetLB_EnumerationProperty(t *testing.T) {
	// Deterministic property sweep:
	//
	// 5 endpoints with two metadata dimensions: version (v1/v2) and zone (a/b).
	// Not all carry both keys — e5 has neither.
	//
	//   e1: version=v1, zone=a  → in selector["version"]{v1}, selector["version","zone"]{v1,a}
	//   e2: version=v1, zone=b  → in selector["version"]{v1}, selector["version","zone"]{v1,b}
	//   e3: version=v2, zone=a  → in selector["version"]{v2}, selector["version","zone"]{v2,a}
	//   e4: version=v2          → in selector["version"]{v2}, NOT in selector["version","zone"] (missing zone)
	//   e5: (no keys)           → excluded from both selectors
	//
	// Two selectors: [["version"], ["version","zone"]]
	//
	// Expected subsets across both selectors:
	//   key("version"=v1)           → {e1, e2}
	//   key("version"=v2)           → {e3, e4}
	//   key("version"=v1,"zone"=a)  → {e1}
	//   key("version"=v1,"zone"=b)  → {e2}
	//   key("version"=v2,"zone"=a)  → {e3}
	// Total: 5 distinct subsets.

	sv := func(s string) SubsetValue { return SubsetValue{Kind: subsetString, Str: s} }
	e1 := epMD("10.0.0.1", 1001, map[string]SubsetValue{"version": sv("v1"), "zone": sv("a")})
	e2 := epMD("10.0.0.2", 1002, map[string]SubsetValue{"version": sv("v1"), "zone": sv("b")})
	e3 := epMD("10.0.0.3", 1003, map[string]SubsetValue{"version": sv("v2"), "zone": sv("a")})
	e4 := epMD("10.0.0.4", 1004, map[string]SubsetValue{"version": sv("v2")})
	e5 := epMD("10.0.0.5", 1005, nil) // no keys

	allEps := []Endpoint{e1, e2, e3, e4, e5}
	cfg := lbSubsetCfg{
		fallback: fallbackAnyEndpoint,
		selectors: [][]string{
			{"version"},
			{"version", "zone"},
		},
	}

	rec := &recordingFactory{}
	s := newSubsetLB(allEps, cfg, rec.factory)

	// 5 distinct subsets + 1 fallback (ANY_ENDPOINT) = 6 factory calls total.
	if s.numSubsets != 5 {
		t.Errorf("numSubsets = %d, want 5", s.numSubsets)
	}
	if s.fallback == nil {
		t.Error("ANY_ENDPOINT fallback must be non-nil")
	}
	if len(rec.calls) != 6 {
		t.Errorf("factory call count = %d, want 6 (5 subsets + 1 ANY_ENDPOINT fallback)", len(rec.calls))
	}

	// Verify membership of each subset via the subsets map.
	checkSubset := func(t *testing.T, desc string, keys map[string]SubsetValue, wantAddrs []string) {
		t.Helper()
		key := NewSubsetMatch(keys).Key()
		child, ok := s.subsets[key]
		if !ok {
			t.Errorf("%s: subset with key %q not found", desc, key)
			return
		}
		// Extract the endpoint slice from the child (it's a *roundRobin).
		rr, ok := child.(*roundRobin)
		if !ok {
			t.Errorf("%s: child is not *roundRobin", desc)
			return
		}
		got := hostsInSlice(rr.endpoints)
		want := make(map[string]bool, len(wantAddrs))
		for _, a := range wantAddrs {
			want[a] = true
		}
		if len(got) != len(want) {
			t.Errorf("%s: endpoint count = %d, want %d; got=%v want=%v", desc, len(got), len(want), got, want)
			return
		}
		for a := range want {
			if !got[a] {
				t.Errorf("%s: missing endpoint %s; got=%v", desc, a, got)
			}
		}
	}

	checkSubset(t, "version=v1", map[string]SubsetValue{"version": sv("v1")}, []string{"10.0.0.1:1001", "10.0.0.2:1002"})
	checkSubset(t, "version=v2", map[string]SubsetValue{"version": sv("v2")}, []string{"10.0.0.3:1003", "10.0.0.4:1004"})
	checkSubset(t, "version=v1,zone=a", map[string]SubsetValue{"version": sv("v1"), "zone": sv("a")}, []string{"10.0.0.1:1001"})
	checkSubset(t, "version=v1,zone=b", map[string]SubsetValue{"version": sv("v1"), "zone": sv("b")}, []string{"10.0.0.2:1002"})
	checkSubset(t, "version=v2,zone=a", map[string]SubsetValue{"version": sv("v2"), "zone": sv("a")}, []string{"10.0.0.3:1003"})

	// e5 (no keys) must not appear in any subset.
	e5Addr := e5.Addr()
	for key, child := range s.subsets {
		rr, ok := child.(*roundRobin)
		if !ok {
			continue
		}
		for _, ep := range rr.endpoints {
			if ep.Addr() == e5Addr {
				t.Errorf("e5 (no metadata keys) leaked into subset %q", key)
			}
		}
	}

	// e4 (missing zone) must NOT appear in any ["version","zone"] subset.
	e4Addr := e4.Addr()
	for _, keys := range []map[string]SubsetValue{
		{"version": sv("v2"), "zone": sv("a")},
		{"version": sv("v2"), "zone": sv("b")},
	} {
		k := NewSubsetMatch(keys).Key()
		if child, ok := s.subsets[k]; ok {
			rr, ok := child.(*roundRobin)
			if !ok {
				continue
			}
			for _, ep := range rr.endpoints {
				if ep.Addr() == e4Addr {
					t.Errorf("e4 (missing zone) leaked into 2-key subset %q", k)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Task 7: subsetLB.Pick tests (resolution property + stats mutual-exclusion +
// nil-guard + hashKey passthrough).
// ---------------------------------------------------------------------------

func TestSubsetLB_PickResolvesToSubset(t *testing.T) {
	eps := []Endpoint{
		epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}),
		epMD("127.0.0.1", 9002, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v2"}}),
	}
	s := newSubsetLB(eps, lbSubsetCfg{fallback: fallbackNoFallback, selectors: [][]string{{"version"}}}, rrFactory)
	v1 := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})
	ep, _, err := s.Pick(0, false, v1, true)
	if err != nil || ep.Port != 9001 {
		t.Errorf("v1 match must land on 9001: ep=%v err=%v", ep, err)
	}
}

func TestSubsetLB_PickNoFallbackMissIsErrNoEndpoints(t *testing.T) {
	eps := []Endpoint{epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})}
	s := newSubsetLB(eps, lbSubsetCfg{fallback: fallbackNoFallback, selectors: [][]string{{"version"}}}, rrFactory)
	miss := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v9"}})
	_, release, err := s.Pick(0, false, miss, true)
	if err != errNoEndpoints {
		t.Errorf("NO_FALLBACK miss → errNoEndpoints, got %v", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error")
	}
	if _, _, err := s.Pick(0, false, SubsetMatch{}, false); err != errNoEndpoints {
		t.Errorf("no-match + NO_FALLBACK → errNoEndpoints, got %v", err)
	}
}

func TestSubsetLB_PickAnyEndpointFallbackSpreads(t *testing.T) {
	eps := []Endpoint{
		epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}),
		epMD("127.0.0.1", 9002, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v2"}}),
	}
	s := newSubsetLB(eps, lbSubsetCfg{fallback: fallbackAnyEndpoint, selectors: [][]string{{"version"}}}, rrFactory)
	miss := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v9"}})
	seen := map[uint32]bool{}
	// roundRobin starts its counter at 0, so 2 picks cover both hosts; 4 gives a 2× margin.
	for i := 0; i < 4; i++ {
		ep, _, err := s.Pick(0, false, miss, true)
		if err != nil {
			t.Fatal(err)
		}
		seen[ep.Port] = true
	}
	if len(seen) < 2 { // ANY_ENDPOINT round-robins over all hosts
		t.Errorf("ANY_ENDPOINT fallback must spread over all hosts, saw %v", seen)
	}
}

func TestSubsetLB_PickStatsInc(t *testing.T) {
	reg := stats.NewRegistry()
	sel := reg.NewCounter("t.selected")
	fbc := reg.NewCounter("t.fallback")
	eps := []Endpoint{
		epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}),
		epMD("127.0.0.1", 9002, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v2"}}),
	}
	s := newSubsetLB(eps, lbSubsetCfg{fallback: fallbackAnyEndpoint, selectors: [][]string{{"version"}}}, rrFactory)
	s.selected, s.fallbackC = sel, fbc
	if _, _, err := s.Pick(0, false, NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}), true); err != nil { // selected
		t.Fatalf("hit pick failed: %v", err)
	}
	if _, _, err := s.Pick(0, false, NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v9"}}), true); err != nil { // v9 misses all subsets → ANY_ENDPOINT fallback (fallbackC.Inc)
		t.Fatalf("fallback pick failed: %v", err)
	}
	if sel.Load() != 1 || fbc.Load() != 1 {
		t.Errorf("selected/fallback = %d/%d, want 1/1 (mutually exclusive)", sel.Load(), fbc.Load())
	}
}

func TestSubsetLB_PickNilCountersNoPanic(t *testing.T) {
	eps := []Endpoint{epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})}
	s := newSubsetLB(eps, lbSubsetCfg{fallback: fallbackAnyEndpoint, selectors: [][]string{{"version"}}}, rrFactory)
	if _, _, err := s.Pick(0, false, NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}), true); err != nil {
		t.Fatalf("nil-counter Pick must not panic/err: %v", err)
	}
}
