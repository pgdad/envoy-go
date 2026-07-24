package cluster

// Phase 73 Task 1 — the buildCluster populate LOOP that stamps the OWNING
// CLUSTER's static filter_metadata onto every Endpoint.
//
// These four tests live in their OWN file rather than in manager_test.go
// because manager_test.go is sha256-gated BYTE-UNTOUCHED at T4 and T8; adding
// to it would make its own gate unsatisfiable. Same package, so buildCluster /
// newSubsetLB / subsetLB / parseLbSubsetConfig are all reachable and NO new
// exported symbol is needed.

import (
	"context"
	"io"
	"net"
	"testing"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pgdad/envoy-go/internal/stats"
)

// clusterMetaNS is the NON-"envoy.lb" namespace these tests address. Its whole
// point is that ANY filter_metadata namespace is addressable by a CLUSTER-kind
// tracing custom_tag — "envoy.lb" is NOT a privileged namespace, and the
// phase-38 scalars-only projection cannot serve a NESTED path.
const clusterMetaNS = "envoy.custom.cluster"

// setClusterMeta stamps clusters[].metadata.filter_metadata[ns] on c.
func setClusterMeta(t *testing.T, c *clusterv3.Cluster, ns string, m map[string]any) {
	t.Helper()
	st, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("structpb.NewStruct(%v): %v", m, err)
	}
	if c.Metadata == nil {
		c.Metadata = &corev3.Metadata{}
	}
	if c.Metadata.FilterMetadata == nil {
		c.Metadata.FilterMetadata = map[string]*structpb.Struct{}
	}
	c.Metadata.FilterMetadata[ns] = st
}

// setEndpointMeta stamps lb_endpoints[i].metadata.filter_metadata[ns] on c.
func setEndpointMeta(t *testing.T, c *clusterv3.Cluster, i int, ns string, m map[string]any) {
	t.Helper()
	st, err := structpb.NewStruct(m)
	if err != nil {
		t.Fatalf("structpb.NewStruct(%v): %v", m, err)
	}
	lbe := c.GetLoadAssignment().GetEndpoints()[0].GetLbEndpoints()[i]
	if lbe.Metadata == nil {
		lbe.Metadata = &corev3.Metadata{}
	}
	if lbe.Metadata.FilterMetadata == nil {
		lbe.Metadata.FilterMetadata = map[string]*structpb.Struct{}
	}
	lbe.Metadata.FilterMetadata[ns] = st
}

// nestedString walks v as a StructValue down path and returns the leaf string.
func nestedString(v *structpb.Value, path ...string) (string, bool) {
	if v == nil {
		return "", false
	}
	cur := v
	for _, seg := range path {
		sv := cur.GetStructValue()
		if sv == nil {
			return "", false
		}
		next, ok := sv.GetFields()[seg]
		if !ok {
			return "", false
		}
		cur = next
	}
	s, ok := cur.GetKind().(*structpb.Value_StringValue)
	if !ok {
		return "", false
	}
	return s.StringValue, true
}

// ---------------------------------------------------------------------------
// Test 2 — the populate is LIVE through buildCluster
// ---------------------------------------------------------------------------

// TestBuildCluster_ClusterMetadataPopulatesEndpoints proves the T1 loop — not
// some other path — makes the OWNING CLUSTER's namespace reachable off a built
// Endpoint, with a NESTED value the phase-38 envoy.lb scalar projection could
// never carry. It also proves the two retentions do NOT leak into each other:
// an lb_endpoints[].metadata value is invisible through ClusterMetaLookup, and
// the cluster value is invisible through MetaLookup. (Break A's target.)
func TestBuildCluster_ClusterMetadataPopulatesEndpoints(t *testing.T) {
	c := mkStaticCluster("c_meta", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002))
	setClusterMeta(t, c, clusterMetaNS, map[string]any{
		"tier": map[string]any{"name": "gold"},
	})
	setEndpointMeta(t, c, 0, clusterMetaNS, map[string]any{
		"tier": map[string]any{"name": "endpoint-only"},
	})

	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, ok := mgr.Get("c_meta")
	if !ok {
		t.Fatalf("Get(c_meta): not found")
	}

	// Every endpoint of the cluster carries the SAME cluster map.
	if len(cl.endpoints) != 2 {
		t.Fatalf("endpoints = %d, want 2", len(cl.endpoints))
	}
	for i, ep := range cl.endpoints {
		v, ok := ep.ClusterMetaLookup(clusterMetaNS)
		if !ok {
			t.Errorf("endpoints[%d].ClusterMetaLookup(%q) ok = false, want true", i, clusterMetaNS)
			continue
		}
		if got, ok := nestedString(v, "tier", "name"); !ok || got != "gold" {
			t.Errorf("endpoints[%d] nested tier.name = %q (ok=%v), want %q", i, got, ok, "gold")
		}
	}

	// The LIVE pick path sees it too.
	ep, err := cl.PickEndpoint()
	if err != nil {
		t.Fatalf("PickEndpoint: %v", err)
	}
	if v, ok := ep.ClusterMetaLookup(clusterMetaNS); !ok {
		t.Errorf("picked endpoint ClusterMetaLookup(%q) ok = false, want true", clusterMetaNS)
	} else if got, _ := nestedString(v, "tier", "name"); got != "gold" {
		t.Errorf("picked endpoint nested tier.name = %q, want %q", got, "gold")
	}

	// endpoints[0] carries an ENDPOINT-level value under the SAME namespace: it
	// must NOT leak into the cluster retention, and vice versa.
	epV, ok := cl.endpoints[0].MetaLookup(clusterMetaNS)
	if !ok {
		t.Errorf("endpoints[0].MetaLookup(%q) ok = false, want true (phase-72 retention still live)", clusterMetaNS)
	} else if got, _ := nestedString(epV, "tier", "name"); got != "endpoint-only" {
		t.Errorf("endpoints[0].MetaLookup nested tier.name = %q, want %q (endpoint source)", got, "endpoint-only")
	}
	clV, ok := cl.endpoints[0].ClusterMetaLookup(clusterMetaNS)
	if !ok {
		t.Errorf("endpoints[0].ClusterMetaLookup(%q) ok = false, want true", clusterMetaNS)
	} else if got, _ := nestedString(clV, "tier", "name"); got != "gold" {
		t.Errorf("endpoints[0].ClusterMetaLookup nested tier.name = %q, want %q (cluster source — endpoint value LEAKED)", got, "gold")
	}
	// endpoints[1] has NO endpoint metadata at all: MetaLookup misses, the
	// cluster retention still resolves.
	if v, ok := cl.endpoints[1].MetaLookup(clusterMetaNS); v != nil || ok {
		t.Errorf("endpoints[1].MetaLookup(%q) = (%v, %v), want (nil, false)", clusterMetaNS, v, ok)
	}
	if _, ok := cl.endpoints[1].ClusterMetaLookup(clusterMetaNS); !ok {
		t.Errorf("endpoints[1].ClusterMetaLookup(%q) ok = false, want true", clusterMetaNS)
	}

	// An absent namespace still misses.
	if v, ok := cl.endpoints[0].ClusterMetaLookup("no.such.ns"); v != nil || ok {
		t.Errorf("ClusterMetaLookup(\"no.such.ns\") = (%v, %v), want (nil, false)", v, ok)
	}
}

// TestBuildCluster_ClusterMetadataAliasesTheProtoMap proves the retention
// ALIASES the already-parsed proto map (zero new allocation per endpoint)
// rather than deep-copying it: mutating the SOURCE map after buildCluster is
// observable through ClusterMetaLookup. A copy would be invisible.
func TestBuildCluster_ClusterMetadataAliasesTheProtoMap(t *testing.T) {
	c := mkStaticCluster("c_alias", mkLbEndpoint("127.0.0.1", 9001))
	setClusterMeta(t, c, clusterMetaNS, map[string]any{"tier": map[string]any{"name": "gold"}})

	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, _ := mgr.Get("c_alias")

	// Mutate the SOURCE proto map (the same map object the field aliases).
	src := c.GetMetadata().GetFilterMetadata()
	mutated, err := structpb.NewStruct(map[string]any{"tier": map[string]any{"name": "silver"}})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	src[clusterMetaNS] = mutated

	v, ok := cl.endpoints[0].ClusterMetaLookup(clusterMetaNS)
	if !ok {
		t.Fatalf("ClusterMetaLookup(%q) ok = false, want true", clusterMetaNS)
	}
	if got, _ := nestedString(v, "tier", "name"); got != "silver" {
		t.Errorf("after mutating the SOURCE map, nested tier.name = %q, want %q — the field is NOT aliasing the proto map", got, "silver")
	}

	// Adding a WHOLE NEW namespace to the source map is visible too — the map
	// header itself is shared, not a shallow copy of its entries.
	extra, err := structpb.NewStruct(map[string]any{"added": "later"})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	src["late.ns"] = extra
	if _, ok := cl.endpoints[0].ClusterMetaLookup("late.ns"); !ok {
		t.Errorf("ClusterMetaLookup(\"late.ns\") ok = false — the map header was COPIED, not aliased")
	}
}

// ---------------------------------------------------------------------------
// Test 3 — THE SUBSET-LB ASSERTION (Break D's target)
// ---------------------------------------------------------------------------

// TestBuildCluster_ClusterMetadataSurvivesSubsetLB is the row's DISTINGUISHING
// test: the populate loop's PLACEMENT (immediately after the extractEndpoints
// guard, BEFORE ALL LB construction) is load-bearing.
//
// buildLeafLB retains the SLICE HEADER (roundRobin{endpoints: endpoints}) and
// therefore ALIASES the backing array, so the round-robin path would stay green
// even with a misplaced loop. newSubsetLB copies each Endpoint BY VALUE at TWO
// sites — the selector-grouping loop and the fallbackDefaultSubset branch — so
// only a subset-copied Endpoint sees a zeroed field.
//
// ⚠️ Cluster.PickEndpoint() CANNOT be used here: it passes (SubsetMatch{},
// false), so subsetLB.Pick never consults s.subsets and falls through to the
// ANY_ENDPOINT fallback, which is factory(endpoints) over the ORIGINAL
// (aliased) slice — a misplaced loop stays INVISIBLE through it. Only a
// hasMatch=true pick traverses the value-copied groups.
func TestBuildCluster_ClusterMetadataSurvivesSubsetLB(t *testing.T) {
	t.Run("SubsetChild", func(t *testing.T) {
		c := mkStaticCluster("c_sub_meta", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002))
		tag(c, 0, "version", "v1")
		tag(c, 1, "version", "v2")
		c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{
			FallbackPolicy:  clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT,
			SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{"version"}}},
		}
		setClusterMeta(t, c, clusterMetaNS, map[string]any{"tier": map[string]any{"name": "gold"}})

		mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		cl, _ := mgr.Get("c_sub_meta")
		slb, ok := cl.lb.(*subsetLB)
		if !ok {
			t.Fatalf("lb = %T, want *subsetLB (precondition — the subset wrap did not happen)", cl.lb)
		}

		// hasMatch=true + a matching selector value ⇒ the pick traverses
		// s.subsets, whose members were copied BY VALUE at subset.go:168.
		ep, release, err := slb.Pick(0, false, NewSubsetMatch(map[string]SubsetValue{
			"version": {Kind: subsetString, Str: "v1"},
		}), true)
		if err != nil {
			t.Fatalf("subsetLB.Pick(subset child): %v", err)
		}
		release()
		if ep.IsZero() {
			t.Fatalf("subsetLB.Pick returned the zero Endpoint (precondition)")
		}
		v, ok := ep.ClusterMetaLookup(clusterMetaNS)
		if !ok {
			t.Errorf("subset-child endpoint ClusterMetaLookup(%q) ok = false, want true — the populate loop ran AFTER the value-copying LB construction", clusterMetaNS)
		} else if got, _ := nestedString(v, "tier", "name"); got != "gold" {
			t.Errorf("subset-child endpoint nested tier.name = %q, want %q", got, "gold")
		}
	})

	t.Run("SubsetDefaultFallback", func(t *testing.T) {
		// The FOURTH copy site: newSubsetLB's fallbackDefaultSubset branch
		// appends each matching ep BY VALUE into a fresh slice.
		c := mkStaticCluster("c_sub_def", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002))
		tag(c, 0, "version", "v1")
		tag(c, 1, "version", "v2")
		ds, err := structpb.NewStruct(map[string]any{"version": "v1"})
		if err != nil {
			t.Fatalf("structpb.NewStruct: %v", err)
		}
		c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{
			FallbackPolicy: clusterv3.Cluster_LbSubsetConfig_DEFAULT_SUBSET,
			DefaultSubset:  ds,
		}
		setClusterMeta(t, c, clusterMetaNS, map[string]any{"tier": map[string]any{"name": "gold"}})

		mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		cl, _ := mgr.Get("c_sub_def")
		slb, ok := cl.lb.(*subsetLB)
		if !ok {
			t.Fatalf("lb = %T, want *subsetLB (precondition)", cl.lb)
		}
		// No selectors ⇒ s.subsets is empty ⇒ the pick falls to the
		// DEFAULT_SUBSET fallback, built over the value-copied match slice.
		ep, release, err := slb.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatalf("subsetLB.Pick(default fallback): %v", err)
		}
		release()
		if ep.IsZero() {
			t.Fatalf("subsetLB.Pick returned the zero Endpoint (precondition)")
		}
		if _, ok := ep.ClusterMetaLookup(clusterMetaNS); !ok {
			t.Errorf("default-subset fallback endpoint ClusterMetaLookup(%q) ok = false, want true — the populate loop ran AFTER newSubsetLB", clusterMetaNS)
		}
	})

	t.Run("RoundRobinPathStaysGreen", func(t *testing.T) {
		// The CONTROL arm: buildLeafLB aliases the backing array, so this arm
		// cannot discriminate placement. It is here to make that explicit — if
		// this arm ever fires alone, the populate itself (not its placement) is
		// broken.
		c := mkStaticCluster("c_rr_meta", mkLbEndpoint("127.0.0.1", 9001))
		setClusterMeta(t, c, clusterMetaNS, map[string]any{"tier": map[string]any{"name": "gold"}})
		mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
		if err != nil {
			t.Fatalf("NewManager: %v", err)
		}
		cl, _ := mgr.Get("c_rr_meta")
		ep, err := cl.PickEndpoint()
		if err != nil {
			t.Fatalf("PickEndpoint: %v", err)
		}
		if _, ok := ep.ClusterMetaLookup(clusterMetaNS); !ok {
			t.Errorf("round-robin picked endpoint ClusterMetaLookup(%q) ok = false, want true", clusterMetaNS)
		}
	})
}

// ---------------------------------------------------------------------------
// Test 4 — the byte-unchanged guard
// ---------------------------------------------------------------------------

// TestBuildCluster_Phase38ProjectionUnchangedByClusterMetadata pins that the
// phase-38 envoy.lb scalar projection (manager.go:883-884), defaultSubset
// (manager.go:754) and the phase-72 MetaLookup retention all behave EXACTLY as
// before, with a cluster-level metadata block present.
func TestBuildCluster_Phase38ProjectionUnchangedByClusterMetadata(t *testing.T) {
	c := mkStaticCluster("c_proj", mkLbEndpoint("127.0.0.1", 9001))
	// endpoints[0] carries an envoy.lb namespace with BOTH a scalar key and a
	// NON-scalar (struct) key, plus a separate non-envoy.lb namespace.
	setEndpointMeta(t, c, 0, "envoy.lb", map[string]any{
		"version": "v1",
		"nested":  map[string]any{"drop": "me"},
	})
	setEndpointMeta(t, c, 0, clusterMetaNS, map[string]any{"who": "endpoint"})
	setClusterMeta(t, c, clusterMetaNS, map[string]any{"who": "cluster"})

	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, _ := mgr.Get("c_proj")
	ep := cl.endpoints[0]

	// (a) the envoy.lb SCALAR still lands in Endpoint.Metadata as a SubsetValue.
	sv, ok := ep.Metadata["version"]
	if !ok {
		t.Errorf("Metadata[\"version\"] absent — the phase-38 projection regressed")
	} else if sv.Kind != subsetString || sv.Str != "v1" {
		t.Errorf("Metadata[\"version\"] = %+v, want {Kind:subsetString Str:v1}", sv)
	}
	// (b) the NON-scalar envoy.lb key is still DROPPED from Metadata.
	if _, ok := ep.Metadata["nested"]; ok {
		t.Errorf("Metadata[\"nested\"] present — the non-scalar drop regressed")
	}
	if len(ep.Metadata) != 1 {
		t.Errorf("len(Metadata) = %d, want 1 (scalar only)", len(ep.Metadata))
	}
	// (c) the phase-72 endpoint retention still resolves its OWN source.
	epV, ok := ep.MetaLookup(clusterMetaNS)
	if !ok {
		t.Errorf("MetaLookup(%q) ok = false, want true", clusterMetaNS)
	} else if got := epV.GetStructValue().GetFields()["who"].GetStringValue(); got != "endpoint" {
		t.Errorf("MetaLookup(%q)[who] = %q, want %q", clusterMetaNS, got, "endpoint")
	}
	// The endpoint's envoy.lb namespace is still reachable RAW through
	// MetaLookup, including the non-scalar key the projection dropped.
	lbV, ok := ep.MetaLookup("envoy.lb")
	if !ok {
		t.Errorf("MetaLookup(\"envoy.lb\") ok = false, want true")
	} else if got, _ := nestedString(lbV, "nested", "drop"); got != "me" {
		t.Errorf("MetaLookup(\"envoy.lb\") nested.drop = %q, want %q", got, "me")
	}
	// (d) the phase-73 cluster retention resolves its own source, unaffected.
	clV, ok := ep.ClusterMetaLookup(clusterMetaNS)
	if !ok {
		t.Errorf("ClusterMetaLookup(%q) ok = false, want true", clusterMetaNS)
	} else if got := clV.GetStructValue().GetFields()["who"].GetStringValue(); got != "cluster" {
		t.Errorf("ClusterMetaLookup(%q)[who] = %q, want %q", clusterMetaNS, got, "cluster")
	}
	// (e) the cluster's metadata does NOT enter the subset dimension.
	if _, ok := ep.Metadata["who"]; ok {
		t.Errorf("Metadata[\"who\"] present — cluster metadata leaked into the phase-38 subset projection")
	}

	// (f) defaultSubset (manager.go:754) is unchanged: scalars kept, non-scalars
	// dropped.
	ds, err := structpb.NewStruct(map[string]any{"version": "v1", "nested": map[string]any{"x": "y"}})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	cfg := parseLbSubsetConfig(&clusterv3.Cluster_LbSubsetConfig{
		FallbackPolicy: clusterv3.Cluster_LbSubsetConfig_DEFAULT_SUBSET,
		DefaultSubset:  ds,
	})
	if len(cfg.defaultSubset) != 1 {
		t.Errorf("len(defaultSubset) = %d, want 1 (non-scalar dropped)", len(cfg.defaultSubset))
	}
	if got, ok := cfg.defaultSubset["version"]; !ok || got.Kind != subsetString || got.Str != "v1" {
		t.Errorf("defaultSubset[\"version\"] = %+v (ok=%v), want {Kind:subsetString Str:v1}", got, ok)
	}
}

// ---------------------------------------------------------------------------
// Test 5 — the pool-HIT provenance test
// ---------------------------------------------------------------------------

// TestAcquireH1_PoolHitRetainsClusterMetadata drives a REAL H1 pool MISS →
// PutIdleH1 → HIT and asserts the cluster retention still resolves off the
// Endpoint the HIT returns. On a hit AcquireH1 returns the POOLED conn's
// dial-time ep (p.ep), not the fresh pick — so a pool path that reconstructed
// the Endpoint would zero the field on reuse while every fresh-dial test stayed
// green.
func TestAcquireH1_PoolHitRetainsClusterMetadata(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(io.Discard, conn) }()
		}
	}()

	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	port := uint32(0)
	for _, b := range portStr {
		port = port*10 + uint32(b-'0')
	}

	c := mkStaticCluster("c_pool_meta", mkLbEndpoint(host, port))
	setClusterMeta(t, c, clusterMetaNS, map[string]any{"tier": map[string]any{"name": "gold"}})
	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cl, _ := mgr.Get("c_pool_meta")

	// MISS → fresh dial.
	p1, epMiss, err := cl.AcquireH1(context.Background())
	if err != nil {
		t.Fatalf("AcquireH1 (miss): %v", err)
	}
	if v, ok := epMiss.ClusterMetaLookup(clusterMetaNS); !ok {
		t.Errorf("pool MISS endpoint ClusterMetaLookup(%q) ok = false, want true", clusterMetaNS)
	} else if got, _ := nestedString(v, "tier", "name"); got != "gold" {
		t.Errorf("pool MISS endpoint nested tier.name = %q, want %q", got, "gold")
	}

	cl.PutIdleH1(p1)

	// HIT → the pooled conn's dial-time ep.
	p2, epHit, err := cl.AcquireH1(context.Background())
	if err != nil {
		t.Fatalf("AcquireH1 (hit): %v", err)
	}
	defer func() { _ = p2.Conn.Close() }()
	if p2 != p1 {
		t.Fatalf("second AcquireH1 was not a pool HIT (got a different *PooledH1Conn)")
	}
	if v, ok := epHit.ClusterMetaLookup(clusterMetaNS); !ok {
		t.Errorf("pool HIT endpoint ClusterMetaLookup(%q) ok = false, want true", clusterMetaNS)
	} else if got, _ := nestedString(v, "tier", "name"); got != "gold" {
		t.Errorf("pool HIT endpoint nested tier.name = %q, want %q", got, "gold")
	}
	// The pooled conn's own retained ep carries it too.
	if _, ok := p2.ep.ClusterMetaLookup(clusterMetaNS); !ok {
		t.Errorf("pooled conn ep ClusterMetaLookup(%q) ok = false, want true", clusterMetaNS)
	}
}
