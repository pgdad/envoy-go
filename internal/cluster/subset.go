package cluster

import (
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/esalaine/envoy-go/internal/stats"
)

// subsetValueKind tags the SubsetValue scalar union. The MVP supports only the
// scalar ProtobufWkt::Value kinds (string, number, bool); list/struct/null are
// not represented (the producer rejects them, extractEndpoints drops them).
type subsetValueKind uint8

const (
	subsetString subsetValueKind = iota
	subsetNumber
	subsetBool
)

// SubsetValue is a proto-free scalar metadata value mirroring the scalar subset
// of ProtobufWkt::Value. Numbers are double-valued (a single numeric kind — int
// 1 ≡ double 1.0; there is no int/double distinction to canonicalize beyond
// float64).
type SubsetValue struct {
	Kind subsetValueKind
	Str  string
	Num  float64
	Bool bool
}

// valueEqual mirrors Envoy's ValueUtil::equal for scalars: cross-kind never
// equal; same-kind by value; numbers compared as float64 (double-valued).
func valueEqual(a, b SubsetValue) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case subsetString:
		return a.Str == b.Str
	case subsetNumber:
		return a.Num == b.Num
	case subsetBool:
		return a.Bool == b.Bool
	}
	return false
}

// subsetKV is one (key → scalar value) pair in a canonicalized SubsetMatch.
type subsetKV struct {
	Key string
	Val SubsetValue
}

// SubsetMatch is the proto-free resolved metadata match — a sorted scalar
// (key → value) vector mirroring Envoy's SubsetMetadata. The producer builds it
// once at config-build from RouteAction.metadata_match["envoy.lb"]; subsetLB
// resolves endpoints/subsets against it via an EXACT key-set + value match.
// The zero value is the empty (no-match) match.
type SubsetMatch struct {
	kvs []subsetKV // sorted by Key ascending; immutable after NewSubsetMatch
}

// NewSubsetMatch builds a canonical (sorted) SubsetMatch from a scalar map.
func NewSubsetMatch(m map[string]SubsetValue) SubsetMatch {
	if len(m) == 0 {
		return SubsetMatch{}
	}
	kvs := make([]subsetKV, 0, len(m))
	for k, v := range m {
		kvs = append(kvs, subsetKV{Key: k, Val: v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].Key < kvs[j].Key })
	return SubsetMatch{kvs: kvs}
}

// Empty reports whether the match carries no keys (a route with no metadata_match).
func (m SubsetMatch) Empty() bool { return len(m.kvs) == 0 }

// Key is the canonical lookup string (the subset map key). Numbers are formatted
// via strconv.FormatFloat (1 and 1.0 both → "1"). Keys are kind-tagged so a
// string "1" never collides with the number 1.
func (m SubsetMatch) Key() string {
	var b strings.Builder
	for _, kv := range m.kvs {
		b.WriteString(kv.Key)
		b.WriteByte('=')
		switch kv.Val.Kind {
		case subsetString:
			b.WriteString("s:")
			b.WriteString(kv.Val.Str)
		case subsetNumber:
			b.WriteString("n:")
			b.WriteString(strconv.FormatFloat(kv.Val.Num, 'g', -1, 64))
		case subsetBool:
			b.WriteString("b:")
			b.WriteString(strconv.FormatBool(kv.Val.Bool))
		}
		b.WriteByte(';')
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// subsetLB — subset partitioning wrapper (Task 6: enumeration + build).
// Task 7 adds Pick; Task 8 wires into the manager; Task 9 allocates counters.
// ---------------------------------------------------------------------------

// fallbackPolicy mirrors Cluster_LbSubsetConfig_LbSubsetFallbackPolicy.
type fallbackPolicy uint8

const (
	fallbackNoFallback    fallbackPolicy = iota // NO_FALLBACK
	fallbackAnyEndpoint                         // ANY_ENDPOINT
	fallbackDefaultSubset                       // DEFAULT_SUBSET
)

// lbSubsetCfg is the parsed, proto-free lb_subset_config (built by
// parseLbSubsetConfig — Task 8). selectors is one []keys per subset_selector.
type lbSubsetCfg struct {
	fallback      fallbackPolicy
	defaultSubset map[string]SubsetValue // the DEFAULT_SUBSET match (scalars; non-scalar dropped)
	selectors     [][]string             // each selector's keys
}

// leafFactory builds a child loadBalancer over an endpoint sub-slice (bound to
// buildLeafLB(c, name, ·) by the manager).
type leafFactory func(sub []Endpoint) (loadBalancer, error)

// subsetLB is a per-cluster metadata-match WRAPPER load balancer mirroring Envoy
// v1.37.2's SubsetLoadBalancer. It partitions endpoints into precomputed subsets
// (one per distinct envoy.lb value-tuple per selector) and delegates each subset
// to a child built from the cluster's lb_policy. Built once at construction
// (immutable → Pick is lock-free). ADR-0240 (the policy + Endpoint.Metadata);
// ADR-0239 (the subset-match pick-input); ADR-0232 (release — delegates the
// child's release).
type subsetLB struct {
	subsets  map[string]loadBalancer // canonical SubsetMatch key → child over that subset
	fallback loadBalancer            // nil for NO_FALLBACK (or zero-match DEFAULT_SUBSET)
	// selected and fallbackC are written exactly once at cluster build/register time
	// (registerClusterMetrics, Task 9), before any listener serves traffic; the
	// boot ordering (Manager build → register metrics → freeze → listener start)
	// provides the happens-before. Pick only ever reads them (nil-guarded).
	selected   *stats.Counter // +1 on a subset hit (injected at register — Task 9; Inc'd in Task 7)
	fallbackC  *stats.Counter // +1 on the fallback path (injected at register — Task 9; Inc'd in Task 7)
	numSubsets int            // distinct-subset count → active gauge + created counter (×1)
}

var _ loadBalancer = (*subsetLB)(nil)

// newSubsetLB enumerates the subsets and builds a child per subset + the fallback
// child via the factory. A factory error for a non-buildable subset is swallowed
// defensively (the factory is the cluster's own lb_policy, already validated at
// the cluster-level buildLeafLB call in Task 8).
func newSubsetLB(endpoints []Endpoint, cfg lbSubsetCfg, factory leafFactory) *subsetLB {
	s := &subsetLB{subsets: map[string]loadBalancer{}}
	for _, keys := range cfg.selectors {
		groups := map[string][]Endpoint{} // canonical match key → member endpoints
		for _, ep := range endpoints {
			vals, ok := scalarsForKeys(ep.Metadata, keys)
			if !ok {
				continue // host missing a key → excluded from this selector
			}
			k := NewSubsetMatch(vals).Key()
			groups[k] = append(groups[k], ep)
		}
		for k, members := range groups {
			if _, exists := s.subsets[k]; exists {
				continue // a tuple seen via another selector — keep the first
			}
			child, err := factory(members)
			if err != nil {
				continue // defensive — the cluster-level factory already validated
			}
			s.subsets[k] = child
		}
	}
	s.numSubsets = len(s.subsets)

	switch cfg.fallback {
	case fallbackAnyEndpoint:
		if fb, err := factory(endpoints); err == nil {
			s.fallback = fb
		}
	case fallbackDefaultSubset:
		var match []Endpoint
		want := NewSubsetMatch(cfg.defaultSubset)
		for _, ep := range endpoints {
			if endpointMatches(ep, want) {
				match = append(match, ep)
			}
		}
		if len(match) > 0 { // zero-match default → nil fallback ≡ NO_FALLBACK
			if fb, err := factory(match); err == nil {
				s.fallback = fb
			}
		}
	case fallbackNoFallback:
		// s.fallback stays nil
	}
	return s
}

// Pick resolves the ctx-carried subset match to a subset and delegates to its
// child; on a no-match / miss it applies the cluster-level fallback. hashKey/
// hasHash pass straight through to the child (a ring_hash/maglev subset child
// routes WITHIN its subset). selected/fallbackC are Inc'd from here (the FIRST
// LB to touch stats from its own Pick path — injected at register, Task 9;
// nil-guarded for unit constructions). ADR-0240.
func (s *subsetLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) {
	if hasMatch && !match.Empty() {
		if child, ok := s.subsets[match.Key()]; ok {
			if s.selected != nil {
				s.selected.Inc()
			}
			return child.Pick(hashKey, hasHash, SubsetMatch{}, false)
		}
	}
	// No match (route has no metadata_match) OR the match selected no subset → fallback.
	if s.fallback == nil { // NO_FALLBACK (or zero-match DEFAULT_SUBSET)
		return Endpoint{}, noopRelease, errNoEndpoints
	}
	if s.fallbackC != nil {
		s.fallbackC.Inc()
	}
	return s.fallback.Pick(hashKey, hasHash, SubsetMatch{}, false)
}

// scalarsForKeys returns the endpoint's scalar values for exactly the given keys,
// reporting ok==false if the endpoint is missing any key.
func scalarsForKeys(md map[string]SubsetValue, keys []string) (map[string]SubsetValue, bool) {
	out := make(map[string]SubsetValue, len(keys))
	for _, k := range keys {
		v, ok := md[k]
		if !ok {
			return nil, false
		}
		out[k] = v
	}
	return out, true
}

// endpointMatches reports whether the endpoint carries ALL of the match's keys
// with value-equal values (the DEFAULT_SUBSET predicate; numbers double-valued).
func endpointMatches(ep Endpoint, m SubsetMatch) bool {
	for _, kv := range m.kvs {
		v, ok := ep.Metadata[kv.Key]
		if !ok || !valueEqual(v, kv.Val) {
			return false
		}
	}
	return true
}

// ScalarsFromStruct lowers a structpb.Struct (an envoy.lb metadata namespace) to
// the scalar SubsetValue map, returning the scalar entries plus the list of keys
// whose values were NON-scalar (list / struct / null / unset). Callers decide
// the non-scalar disposition: the route producer REJECTS; extractEndpoints DROPS
// silently. nil struct → empty/empty.
func ScalarsFromStruct(s *structpb.Struct) (map[string]SubsetValue, []string) {
	if s == nil || len(s.GetFields()) == 0 {
		return nil, nil
	}
	scalars := make(map[string]SubsetValue, len(s.GetFields()))
	var nonScalar []string
	for k, v := range s.GetFields() {
		switch kind := v.GetKind().(type) {
		case *structpb.Value_StringValue:
			scalars[k] = SubsetValue{Kind: subsetString, Str: kind.StringValue}
		case *structpb.Value_NumberValue:
			scalars[k] = SubsetValue{Kind: subsetNumber, Num: kind.NumberValue}
		case *structpb.Value_BoolValue:
			scalars[k] = SubsetValue{Kind: subsetBool, Bool: kind.BoolValue}
		default: // Value_StructValue / Value_ListValue / Value_NullValue / nil
			nonScalar = append(nonScalar, k)
		}
	}
	if len(nonScalar) > 0 {
		sort.Strings(nonScalar) // deterministic reject message
	}
	return scalars, nonScalar
}
