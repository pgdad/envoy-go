# Phase 38.2 `weighted-clusters` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. One fresh subagent per task + two-stage review (`feedback_execution_style`); subagents commit LOCAL-ONLY (`feedback_subagents_no_push`); the controller squash-merges + pushes at stage-close (`feedback_push_to_origin`). Each task runs `gofmt -l` + `golangci-lint run` on touched packages (`feedback_pertask_gofmt_lint`).

**Goal:** Land `RouteAction.weighted_clusters` routing — a per-request weighted-random cluster-SELECTION primitive (the SECOND `RouteAction.ClusterSpecifier` arm) — + `WeightedCluster.ClusterWeight.metadata_match` (merged over the route-level `RouteAction.metadata_match`, entry precedence) that COMPOSES with the 38.1 subset LB via the unchanged ADR-0239 ctx seam.

**Architecture:** A `weightedSelector` (cumulative weights + a per-action mutex-guarded PCG draw — the `randomLB` model lifted to the router layer) drives a `weightedClusterRouteAction` bridge (the `clusterRouteAction` sibling in `hcm/actions.go`) that holds N entries `{cluster, weight, merged subsetMatch}` + the shared route-level `hashPolicies`. It satisfies the UNCHANGED `routeAction` interface; per request it picks an entry then runs the EXISTING `doH1ClusterAction`/`doH2ClusterAction` with the chosen entry's cluster + subsetMatch — a SELECTION primitive layered ON the 38.1 dispatch (NO `loadBalancer.Pick` change, NO exported-cluster change). The producer `buildWeightedRouterAction` (in `hcm/config.go`) authors the §6 SIX-arm reject roster + the per-entry metadata_match merge; ZERO new stats; the differential is the per-side weight-distribution band `0065-weighted-clusters`.

**Tech Stack:** Go 1.26.x / golangci-lint 1.64.8 (ADR-0009); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227); go-control-plane `/envoy v1.32.4` (the `RouteAction_WeightedClusters` + `WeightedCluster` + `WeightedCluster_ClusterWeight` surface already pinned — ZERO new go.mod dep). Reuses `internal/filter/hcm` + `internal/filter/http/router` + the 38.1 subset seam (`internal/cluster/{subset.go,cluster.go}`) + the `randomLB` RNG model (`internal/cluster/random.go` + `leastrequest.go`).

**SPEC:** `docs/envoy-go/phases/38.2-weighted-clusters/SPEC.md` (this PLAN decomposes SPEC §10; the §11 D-WC1..D-WC6 pins are the empirical authority; the §12 D-WC-IMPL questions are resolved in the Pre-flight below).

---

## Pre-flight — ADR-0045 split-gate re-check + D-WC-IMPL resolutions

### ADR-0045 split-gate FINAL re-check (per SPEC §3.0 / §11.6)

The gate fires at `> ~25 tasks OR > ~1500 production LoC`. As decomposed below:

| Unit | Tasks | Anticipated prod LoC |
|---|---|---|
| `weightedSelector` + RNG + `pick()` (router_weighted.go) | T2 | ~55 |
| `mergeRouteSubsetMatch` helper (hcm/config.go) | T3 | ~20 |
| `WeightedCluster` type + `H1/H2WeightedClusterAction` (router_weighted.go) | T4 | ~70 |
| `weightedClusterRouteAction` bridge (hcm/actions.go) | T5 | ~45 |
| `buildWeightedRouterAction` reject roster + switch arm (hcm/config.go) | T6 | ~60 |
| `buildWeightedRouterAction` accept path (merge + cumulative + build) (hcm/config.go) | T7 | ~40 |
| Fixture / breaks / verify | T8–T10 | test-side, NOT counted |

Net **~290 prod LoC / 10 tasks** — BOTH axes UNDER the gate. **NO FURTHER SPLIT within 38.2** (single flat leg, the SPEC §3.0 disposition holds). The decomposition matches the kafka-31 / least_request-34 / random-35 / ring_hash-36 / maglev-37 / subset-38.1 single-leg precedent.

### D-WC-IMPL resolutions (SPEC §12)

- **D-WC-IMPL-1 (file placement + RNG layering).** The `weightedSelector` + the `WeightedCluster` type + `H1/H2WeightedClusterAction` live in a NEW `internal/filter/http/router/router_weighted.go` (the router package owns `doH1/H2ClusterAction`, which the weighted closures call — they MUST be in-package). The `weightedClusterRouteAction` bridge + `buildWeightedRouterAction` + `mergeRouteSubsetMatch` live in `internal/filter/hcm/{actions.go,config.go}` (the `clusterRouteAction`/`buildRouterAction`/`parseRouteSubsetMatch` precedent). **RNG layering:** `cluster.newPCGRNG` is package-private to `internal/cluster`; rather than export a cluster internal for a router-layer need, `router_weighted.go` carries its OWN `newWeightedRNG()` (~12 LoC) mirroring `cluster.newPCGRNG` (crypto-seeded `math/rand/v2` PCG + mutex), plus a `newWeightedSelectorWithRNG(weights, rng)` injectable constructor for deterministic tests (the `random.go` `newRandomWithRNG` precedent). **Per-entry dispatch:** `H1WeightedClusterAction` PRE-BUILDS N `*routerAction` (one per entry) at config-build; the closure picks the index then calls `doH1ClusterAction(ctx, actions[idx], req)` — NO per-request alloc; the `filter` field stays nil exactly as the existing `H1ClusterAction` path (the chain-mediated path never sets it).
- **D-WC-IMPL-2 (reject wordings + deferred-field disposition).** The §6 wordings use the `route action: weighted_clusters …` house prefix (per the SPEC §6 table; ADR-0080 byte-stable, parity in OUTCOME with the reference's divergent text). `cluster_header` set on an entry → REJECT (unsupported — deferred). `request/response_headers_to_*` + `typed_per_filter_config` + `host_rewrite` → SILENT-IGNORE (accepted-but-inert; orthogonal to the selection primitive; the route-level header-mutation scope of prior phases). No fixture pins the rejects — they are unit-level in `config_test.go` (the 38.1 `router-metadata-match-nonscalar` precedent).
- **D-WC-IMPL-3 (`0065` constants + band formula).** ONE weighted route `/w` over 3 weighted clusters `{c_a: 50 → backend0, c_b: 30 → backend1, c_sub: 20 → a 2-endpoint subset cluster (backend2 = `version:v1`, backend3 = `version:v2`)}`, the `c_sub` ClusterWeight carrying `metadata_match {envoy.lb:{version:"v1"}}` (the composition FOLDED into the weighted route — see D-WC-IMPL-4). `N = 500` `GET /w` per side + `8 GET /health` (a `direct_response` byte-equiv stream; NOT backend-routed). Per-backend PER-SIDE band = `mean ± ~4.5σ`, `mean = N·pᵢ`, `σᵢ = √(N·pᵢ·(1−pᵢ))`: backend0 `[200,300]` (mean 250, σ≈11.2), backend1 `[104,196]` (mean 150, σ≈10.25), backend2 `[60,140]` (mean 100, σ≈8.94 — ALL `c_sub` traffic lands on v1), backend3 `== 0` exact (the merged match excludes v2 — the composition affinity). Conservation: the 4 per-backend counts sum to `N` (=500). The `~4.5σ` margin satisfies `reference_differential_band_sigma_margin` (flake-free over ≥20 runs; the breaks land far outside). `totalReqs` + the bands DERIVE from named constants (`reference_fixture_workload_constant_desync`).
- **D-WC-IMPL-4 (composition arm shape).** The composition arm is FOLDED into the weighted route (NOT a separate `/compose` route): `c_sub` is one of the 3 weighted clusters and carries a `ClusterWeight.metadata_match {version:v1}`. The proof is that backend3 (`version:v2`) gets ZERO hits (the merged match selected only the v1 subset WITHIN the chosen `c_sub` cluster) while backend2 (`version:v1`) absorbs all of `c_sub`'s ~20%. This keeps the fixture minimal (4 backends, one route) and the band clean, and proves the merge + the unchanged ADR-0239 seam in one arm.
- **D-WC-IMPL-5 (property-test placement).** The selection-distribution property test FOLDS into `router_weighted_test.go` (Task 2 — the deterministic `pick()` boundary tests + an empirical-distribution-over-many-draws check + the explicit-0-never-picked invariant). NO standalone `FuzzWeightedSelect` (no untrusted wire input — SPEC §8.3).

---

## File structure

| File | Responsibility | Task |
|---|---|---|
| `internal/filter/http/router/router_weighted.go` (NEW) | `weightedSelector` + `newWeightedRNG` + `NewWeightedSelector`/`newWeightedSelectorWithRNG` + `pick()`; the `WeightedCluster` exported entry type; `H1WeightedClusterAction` / `H2WeightedClusterAction` (pre-build N `*routerAction`, pick, dispatch) | T2, T4 |
| `internal/filter/http/router/router_weighted_test.go` (NEW) | selector unit + distribution property tests; the H1/H2 dispatch wiring tests | T2, T4 |
| `internal/filter/hcm/config.go` (MODIFY) | `mergeRouteSubsetMatch` helper; the `buildRouterAction` `RouteAction_WeightedClusters` switch arm; `buildWeightedRouterAction` (the reject roster + merge + build) | T3, T6, T7 |
| `internal/filter/hcm/actions.go` (MODIFY) | the `weightedClusterRouteAction` bridge (the `clusterRouteAction` sibling) | T5 |
| `internal/filter/hcm/config_test.go` (MODIFY) | the producer reject roster + accept-path + merge-precedence unit tests | T3, T6, T7 |
| `test/fixtures/0065-weighted-clusters/{driver/driver.go,expectations.yaml,README.md}` (NEW) | the cross-side per-side weight-distribution band + composition + conservation differential | T8, T9 |
| `docs/envoy-go/{BEHAVIOR_CONTRACT.md,DECISIONS.md,STATE.md,ROADMAP.md}` + the phase dir | the completion bundle | T10 |

---

### Task 1: First-task baselines/anchors gate + PROGRESS.md

**Files:**
- Create: `docs/envoy-go/phases/38.2-weighted-clusters/PROGRESS.md`

- [ ] **Step 1: Re-confirm the project counts via the canonical recipes**

Run (expected values in comments):
```bash
ls -d test/fixtures/[0-9]* | wc -l                                              # 66 (tail 0064-lb-subset)
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l            # 42
grep -c "." /dev/null; grep -n "1125" docs/envoy-go/BEHAVIOR_CONTRACT.md | head # stat surface 1125 ("Phase 38.1 extension" block)
grep -n "^## ADR-02" docs/envoy-go/DECISIONS.md | tail -1                       # ADR-0240 (next-free ADR-0241)
ls -d test/fixtures/0064-lb-subset                                             # tail fixture present
```
Expected: fixtures **66**, fuzzers **42**, stat surface **1125**, DECISIONS tail **ADR-0240**, BackendKind tail **33**.

- [ ] **Step 2: Re-pin the as-built anchors against the IMPL-session tip**

Confirm these exist with the signatures the PLAN's snippets assume (a drift check — if any moved, STOP and reconcile):
```bash
grep -n "func buildRouterAction\|func parseRouteSubsetMatch\|func parseRouteHashPolicies\|func buildAction" internal/filter/hcm/config.go
grep -n "type clusterRouteAction struct\|func (a \*clusterRouteAction)" internal/filter/hcm/actions.go
grep -n "type routeAction interface" internal/filter/hcm/route.go
grep -n "func H1ClusterAction\|func doH1ClusterAction\|type routerAction struct\|func applyHashKey" internal/filter/http/router/router.go
grep -n "func H2ClusterAction\|func doH2ClusterAction\|type routerActionH2 struct" internal/filter/http/router/router_h2.go
grep -n "func NewSubsetMatch\|func ScalarsFromStruct\|func (m SubsetMatch) Empty" internal/cluster/subset.go
grep -n "func newPCGRNG\|func newRandomWithRNG" internal/cluster/*.go
```
Expected: all present (the SPEC §11.5 / §3 anchors). `routeAction` = `{do, asRouterAction, asRouterActionH2}`.

- [ ] **Step 3: Create PROGRESS.md** with the 10-task checklist (mirror `docs/envoy-go/phases/38-load-balancer-subset/PROGRESS.md`'s structure: a task table + a six-gate evidence section to be filled at T10).

- [ ] **Step 4: Commit**

```bash
git add docs/envoy-go/phases/38.2-weighted-clusters/PROGRESS.md
git commit -m "phase 38.2 IMPL Task 1: baselines/anchors gate + PROGRESS.md"
```

---

### Task 2: The `weightedSelector` + RNG + `pick()` (router_weighted.go)

**Files:**
- Create: `internal/filter/http/router/router_weighted.go`
- Create/Test: `internal/filter/http/router/router_weighted_test.go`

- [ ] **Step 1: Write the failing tests**

```go
package router

import "testing"

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
	// Wide ±5% absolute band — this is a smoke property, not the flake-safe σ band.
	checks := []struct {
		idx     int
		loFrac  float64
		hiFrac  float64
	}{{0, 0.45, 0.55}, {1, 0.25, 0.35}, {2, 0.15, 0.25}}
	for _, c := range checks {
		f := float64(counts[c.idx]) / float64(n)
		if f < c.loFrac || f > c.hiFrac {
			t.Errorf("entry %d frac %.3f outside [%.2f,%.2f]", c.idx, f, c.loFrac, c.hiFrac)
		}
	}
}
```

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/filter/http/router/ -run 'TestWeightedSelector|TestNewWeightedSelector' -v`
Expected: FAIL (`undefined: newWeightedSelectorWithRNG` / `NewWeightedSelector`).

- [ ] **Step 3: Implement the selector**

```go
package router

import (
	"crypto/rand"
	"encoding/binary"
	mathrand "math/rand/v2"
	"sync"
)

// weightedSelector draws a per-request entry index by integer weight, mirroring
// upstream Envoy's WeightedClusterEntry (a random value in [0, totalWeight) then
// a cumulative-weight walk) and envoy-go's randomLB (a per-action mutex-guarded
// PCG draw). It is the FIRST per-request RNG model at the router layer (ADR-0241).
// total_weight is the SUM of entry weights — the proto's total_weight is
// deprecated/ignored (SPEC §11.1 / AMEND-WC1).
type weightedSelector struct {
	cumulative  []uint32      // cumulative[i] = Σ weights[0..i]; cumulative[last] == totalWeight
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
```

- [ ] **Step 4: Run to verify PASS**

Run: `go test ./internal/filter/http/router/ -run 'TestWeightedSelector|TestNewWeightedSelector' -count=1 -v`
Expected: PASS (all 4).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/http/router/
golangci-lint run ./internal/filter/http/router/...
git add internal/filter/http/router/router_weighted.go internal/filter/http/router/router_weighted_test.go
git commit -m "phase 38.2 IMPL Task 2: weightedSelector + RNG + pick() (the randomLB model at the router layer)"
```

---

### Task 3: The `mergeRouteSubsetMatch` helper (hcm/config.go)

**Files:**
- Modify: `internal/filter/hcm/config.go`
- Test: `internal/filter/hcm/config_test.go`

- [ ] **Step 1: Write the failing test** — the merge precedence (route-level ⊕ entry, entry wins) + non-scalar reject on either side.

```go
func TestMergeRouteSubsetMatch(t *testing.T) {
	mdLB := func(kv map[string]any) *corev3.Metadata {
		fields := map[string]*structpb.Value{}
		for k, v := range kv {
			val, _ := structpb.NewValue(v)
			fields[k] = val
		}
		return &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{
			"envoy.lb": {Fields: fields},
		}}
	}
	// route {version:v1, stage:prod}, entry {version:v2} → merged {version:v2, stage:prod}.
	route := mdLB(map[string]any{"version": "v1", "stage": "prod"})
	entry := mdLB(map[string]any{"version": "v2"})
	merged, err := mergeRouteSubsetMatch(route, entry)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	want := cluster.NewSubsetMatch(map[string]cluster.SubsetValue{}) // build the expected via the same path
	_ = want
	// Assert via the canonical Key() string (sorted, kind-tagged).
	expect := mergeExpect(t, map[string]any{"version": "v2", "stage": "prod"})
	if merged.Key() != expect.Key() {
		t.Errorf("merged.Key()=%q want %q (entry precedence)", merged.Key(), expect.Key())
	}
	// non-scalar on the entry side rejects.
	bad := &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{
		"envoy.lb": {Fields: map[string]*structpb.Value{
			"list": structpb.NewListValue(&structpb.ListValue{}),
		}},
	}}
	if _, err := mergeRouteSubsetMatch(nil, bad); err == nil {
		t.Errorf("expected non-scalar reject")
	}
}

// mergeExpect builds a SubsetMatch from a scalar map via the production lowering.
func mergeExpect(t *testing.T, kv map[string]any) cluster.SubsetMatch {
	t.Helper()
	fields := map[string]*structpb.Value{}
	for k, v := range kv {
		val, _ := structpb.NewValue(v)
		fields[k] = val
	}
	m, non := cluster.ScalarsFromStruct(&structpb.Struct{Fields: fields})
	if len(non) > 0 {
		t.Fatalf("mergeExpect non-scalar %v", non)
	}
	return cluster.NewSubsetMatch(m)
}
```

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/filter/hcm/ -run TestMergeRouteSubsetMatch -v`
Expected: FAIL (`undefined: mergeRouteSubsetMatch`).

- [ ] **Step 3: Implement the merge helper** (next to `parseRouteSubsetMatch` in `config.go`)

```go
// mergeRouteSubsetMatch merges a route-level RouteAction.metadata_match["envoy.lb"]
// with a per-weighted-cluster ClusterWeight.metadata_match["envoy.lb"], with the
// ENTRY values taking precedence on key collision (SPEC §11.5 / AMEND-WC5 — the
// proto's "values here taking precedence"). Both are route-static → merged ONCE
// at config-build into a proto-free cluster.SubsetMatch threaded verbatim per
// request via the EXISTING ADR-0239 WithSubsetMatch seam (the 38.1 thread, NO
// loadBalancer.Pick change). A non-scalar value on EITHER side rejects (the 38.1
// scalar MVP boundary — the parseRouteSubsetMatch reject, reused).
func mergeRouteSubsetMatch(routeMD, entryMD *corev3.Metadata) (cluster.SubsetMatch, error) {
	routeScalars, routeNon := cluster.ScalarsFromStruct(routeMD.GetFilterMetadata()["envoy.lb"])
	if len(routeNon) > 0 {
		return cluster.SubsetMatch{}, fmt.Errorf("router: metadata_match envoy.lb key %q: only scalar values (string, number, bool) are supported", routeNon[0])
	}
	entryScalars, entryNon := cluster.ScalarsFromStruct(entryMD.GetFilterMetadata()["envoy.lb"])
	if len(entryNon) > 0 {
		return cluster.SubsetMatch{}, fmt.Errorf("router: metadata_match envoy.lb key %q: only scalar values (string, number, bool) are supported", entryNon[0])
	}
	if len(routeScalars) == 0 && len(entryScalars) == 0 {
		return cluster.SubsetMatch{}, nil
	}
	merged := make(map[string]cluster.SubsetValue, len(routeScalars)+len(entryScalars))
	for k, v := range routeScalars {
		merged[k] = v
	}
	for k, v := range entryScalars { // entry precedence
		merged[k] = v
	}
	return cluster.NewSubsetMatch(merged), nil
}
```

- [ ] **Step 4: Run to verify PASS**

Run: `go test ./internal/filter/hcm/ -run TestMergeRouteSubsetMatch -count=1 -v`
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/hcm/ && golangci-lint run ./internal/filter/hcm/...
git add internal/filter/hcm/config.go internal/filter/hcm/config_test.go
git commit -m "phase 38.2 IMPL Task 3: mergeRouteSubsetMatch (route-level ⊕ entry, entry precedence; reuses the 38.1 scalar reject)"
```

---

### Task 4: `WeightedCluster` type + `H1/H2WeightedClusterAction` (router_weighted.go)

**Files:**
- Modify: `internal/filter/http/router/router_weighted.go`
- Test: `internal/filter/http/router/router_weighted_test.go`

- [ ] **Step 1: Write the failing test** — a fake-backed cluster set proves the closure picks per the selector and dispatches to the chosen cluster (the per-backend tally tracks the deterministic RNG). Model on `router_test.go`'s existing cluster-dial test harness (reuse its httptest backend + `cluster.Manager` builder helpers). Assert: with an injected `newSeqRNG` hitting entries {0,0,1,2}, the four requests land on clusters {0,0,1,2} respectively (tally the served backend body idx).

```go
func TestH1WeightedClusterAction_PicksPerSelector(t *testing.T) {
	// Build 3 single-endpoint clusters c0/c1/c2 over 3 httptest backends that
	// echo their index (reuse the router_test.go backend helper). Construct a
	// selector with an injected RNG and verify dispatch lands on the picked one.
	clusters, bodies := newEchoClusters(t, 3) // helper: returns []WeightedCluster-ready *cluster.Cluster + body idx map
	sel := newWeightedSelectorWithRNG([]uint32{1, 1, 1}, newSeqRNG(0, 0, 1, 2))
	wcs := []WeightedCluster{
		{Cluster: clusters[0]}, {Cluster: clusters[1]}, {Cluster: clusters[2]},
	}
	act := H1WeightedClusterAction(wcs, nil, sel)
	want := []int{0, 0, 1, 2}
	for i, w := range want {
		resp, _, err := act(context.Background(), mustGET(t, "/"))
		if err != nil {
			t.Fatalf("draw %d: %v", i, err)
		}
		if idx := bodies(resp.Body); idx != w {
			t.Errorf("draw %d landed on backend %d want %d", i, idx, w)
		}
	}
}
```
(The helper `newEchoClusters` + `mustGET` + `bodies` mirror the existing `router_test.go` cluster-dial scaffolding; if absent, add minimal local helpers.)

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/filter/http/router/ -run TestH1WeightedClusterAction -v`
Expected: FAIL (`undefined: WeightedCluster` / `H1WeightedClusterAction`).

- [ ] **Step 3: Implement the type + constructors** (in `router_weighted.go`)

```go
import "context" // + h2 import for the H2 variant

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
func H1WeightedClusterAction(wcs []WeightedCluster, hps []HashPolicy, sel *weightedSelector) Action {
	actions := make([]*routerAction, len(wcs))
	for i, wc := range wcs {
		actions[i] = &routerAction{cluster: wc.Cluster, hashPolicies: hps, subsetMatch: wc.SubsetMatch}
	}
	return func(ctx context.Context, req *http.Request) (ActionResponse, cluster.Endpoint, error) {
		return doH1ClusterAction(ctx, actions[sel.pick()], req)
	}
}

// H2WeightedClusterAction is the H2 sibling (the H2ClusterAction precedent). The
// selector is SHARED with the H1 constructor (a route is H1 or H2 per listener;
// the caller passes the same *weightedSelector to both — only one path runs).
func H2WeightedClusterAction(wcs []WeightedCluster, hps []HashPolicy, sel *weightedSelector) H2Action {
	actions := make([]*routerActionH2, len(wcs))
	for i, wc := range wcs {
		actions[i] = &routerActionH2{cluster: wc.Cluster, hashPolicies: hps, subsetMatch: wc.SubsetMatch}
	}
	return func(ctx context.Context, req h2.H2Request) (ActionResponse, cluster.Endpoint, error) {
		return doH2ClusterAction(ctx, actions[sel.pick()], req)
	}
}
```

- [ ] **Step 4: Run to verify PASS**

Run: `go test ./internal/filter/http/router/ -run 'TestH1WeightedClusterAction|TestWeightedSelector' -count=1 -v`
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/http/router/ && golangci-lint run ./internal/filter/http/router/...
git add internal/filter/http/router/router_weighted.go internal/filter/http/router/router_weighted_test.go
git commit -m "phase 38.2 IMPL Task 4: WeightedCluster + H1/H2WeightedClusterAction (pick → the existing doH1/H2ClusterAction)"
```

---

### Task 5: The `weightedClusterRouteAction` bridge (hcm/actions.go)

**Files:**
- Modify: `internal/filter/hcm/actions.go`
- Test: `internal/filter/hcm/actions_test.go` (or `config_test.go`)

- [ ] **Step 1: Write the failing test** — the bridge satisfies `routeAction` and its `asRouterAction`/`asRouterActionH2` return non-nil closures; `do` runs the H1 path and writes a reply. (A compile-time `var _ routeAction = (*weightedClusterRouteAction)(nil)` + a smoke that `asRouterAction()` is non-nil over a 1-entry bridge.)

```go
func TestWeightedClusterRouteAction_SatisfiesInterface(t *testing.T) {
	var _ routeAction = (*weightedClusterRouteAction)(nil)
}
```

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/filter/hcm/ -run TestWeightedClusterRouteAction -v`
Expected: FAIL (`undefined: weightedClusterRouteAction`).

- [ ] **Step 3: Implement the bridge** (next to `clusterRouteAction` in `actions.go`)

```go
// weightedClusterRouteAction is the per-request weighted-random cluster-SELECTION
// route action (ADR-0241), the sibling of clusterRouteAction. The producer
// (buildWeightedRouterAction) pre-builds the H1/H2 closures (each carrying the N
// entries + the SHARED selector); the bridge just hands them to the chain-mediated
// dispatch via asRouterAction/asRouterActionH2. The do() arm mirrors
// clusterRouteAction.do (run the H1 closure → write the reply) for the legacy
// direct-call interface.
type weightedClusterRouteAction struct {
	h1 router.Action
	h2 router.H2Action
}

func (a *weightedClusterRouteAction) do(ctx context.Context, req *http.Request, bw *bufio.Writer) (int, error) {
	resp, _, err := a.h1(ctx, req)
	if err != nil {
		return resp.Status, err
	}
	if err := writeStatusReply(bw, resp.Status, string(resp.Body)); err != nil {
		return resp.Status, err
	}
	if resp.Close {
		return resp.Status, errCloseAfterAction
	}
	return resp.Status, nil
}

func (a *weightedClusterRouteAction) asRouterAction() router.Action     { return a.h1 }
func (a *weightedClusterRouteAction) asRouterActionH2() router.H2Action { return a.h2 }
```

- [ ] **Step 4: Run to verify PASS**

Run: `go test ./internal/filter/hcm/ -run TestWeightedClusterRouteAction -count=1 -v`
Expected: PASS (compiles + the interface assertion holds).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/hcm/ && golangci-lint run ./internal/filter/hcm/...
git add internal/filter/hcm/actions.go internal/filter/hcm/*_test.go
git commit -m "phase 38.2 IMPL Task 5: weightedClusterRouteAction bridge (the clusterRouteAction sibling)"
```

---

### Task 6: `buildRouterAction` switch arm + `buildWeightedRouterAction` reject roster (hcm/config.go)

**Files:**
- Modify: `internal/filter/hcm/config.go`
- Test: `internal/filter/hcm/config_test.go`

- [ ] **Step 1: Write the failing tests** — the SIX reject arms (SPEC §6) + the "weighted route is recognized" acceptance. Drive through `buildRouterAction` (so the switch arm is exercised). Build a `*cluster.Manager` with `c_a`/`c_b` present.

```go
func TestBuildWeightedRouterAction_Rejects(t *testing.T) {
	mgr := newTestManager(t, "c_a", "c_b") // helper: a Manager with the named clusters
	w := func(clusters ...*routev3.WeightedCluster_ClusterWeight) *routev3.RouteAction {
		return &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_WeightedClusters{
			WeightedClusters: &routev3.WeightedCluster{Clusters: clusters},
		}}
	}
	cw := func(name string, weight *wrapperspb.UInt32Value) *routev3.WeightedCluster_ClusterWeight {
		return &routev3.WeightedCluster_ClusterWeight{Name: name, Weight: weight}
	}
	u := func(v uint32) *wrapperspb.UInt32Value { return wrapperspb.UInt32(v) }

	cases := []struct {
		name string
		ra   *routev3.RouteAction
		want string // substring of the expected error
	}{
		{"empty-clusters", w(), "weighted_clusters has no clusters"},
		{"name-required", w(cw("", u(1))), "name is required"},
		{"weight-required", w(cw("c_a", nil)), "weight is required"},
		{"sum-zero", w(cw("c_a", u(0)), cw("c_b", u(0))), "total weight must be > 0"},
		{"dangling", w(cw("ghost", u(1))), `cluster "ghost" not found`},
		{"cluster-header", func() *routev3.RouteAction {
			// Name set AND cluster_header set → after the name-presence check passes,
			// the cluster_header-unsupported arm fires (the reference's both-set reject).
			r := w(cw("c_a", u(1)))
			r.GetWeightedClusters().Clusters[0].ClusterHeader = "x-cluster"
			return r
		}(), "cluster_header is not supported"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := buildRouterAction(tc.ra, mgr)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want substring %q", err, tc.want)
			}
		})
	}
}

func TestBuildWeightedRouterAction_AcceptsRecognized(t *testing.T) {
	mgr := newTestManager(t, "c_a", "c_b")
	ra := &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_WeightedClusters{
		WeightedClusters: &routev3.WeightedCluster{Clusters: []*routev3.WeightedCluster_ClusterWeight{
			{Name: "c_a", Weight: wrapperspb.UInt32(50)},
			{Name: "c_b", Weight: wrapperspb.UInt32(50)},
		}},
	}}
	act, err := buildRouterAction(ra, mgr)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if _, ok := act.(*weightedClusterRouteAction); !ok {
		t.Fatalf("got %T want *weightedClusterRouteAction", act)
	}
}
```

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/filter/hcm/ -run TestBuildWeightedRouterAction -v`
Expected: FAIL — the `weighted_clusters` specifier currently hits the `default` `not supported` arm; `buildWeightedRouterAction` is undefined.

- [ ] **Step 3: Wire the switch arm + the reject roster.** Convert `buildRouterAction`'s `GetClusterSpecifier()` type-assert into a `switch` and add the weighted arm; add `buildWeightedRouterAction` with the validation (SPEC §6). The accept-path build is a STUB returning a minimal bridge (Task 7 fleshes it out) — OR implement the full build now (Task 7's content) if preferred; the PLAN keeps validation here and the build in Task 7 for bite-sized review.

```go
// in buildRouterAction — replace the single type-assert with:
	switch cs := r.GetClusterSpecifier().(type) {
	case *routev3.RouteAction_Cluster:
		if cs.Cluster == "" {
			return nil, fmt.Errorf("route action: cluster name is empty")
		}
		c, ok := clusters.Get(cs.Cluster)
		if !ok {
			return nil, fmt.Errorf("route action: cluster %q not found", cs.Cluster)
		}
		hps, err := parseRouteHashPolicies(r.GetHashPolicy())
		if err != nil {
			return nil, err
		}
		sm, err := parseRouteSubsetMatch(r.GetMetadataMatch())
		if err != nil {
			return nil, err
		}
		return &clusterRouteAction{cluster: c, hashPolicies: hps, subsetMatch: sm}, nil
	case *routev3.RouteAction_WeightedClusters:
		return buildWeightedRouterAction(r, cs.WeightedClusters, clusters)
	default:
		return nil, fmt.Errorf("route action: cluster_specifier %T is not supported in phase 04 (literal cluster name or weighted_clusters)", r.GetClusterSpecifier())
	}

// buildWeightedRouterAction lowers a RouteAction.weighted_clusters into a
// weightedClusterRouteAction (ADR-0241). envoy-go does its OWN parse-reject (no
// PGV) — the SPEC §6 SIX-arm roster, in the reference's precedence order. Task 6
// lands the FULL function (validation + merge + build) so the accept test below
// observes a real *weightedClusterRouteAction; Task 7 adds the accept-CASE tests.
func buildWeightedRouterAction(r *routev3.RouteAction, wc *routev3.WeightedCluster, clusters *cluster.Manager) (routeAction, error) {
	entries := wc.GetClusters()
	if len(entries) == 0 {
		return nil, fmt.Errorf("route action: weighted_clusters has no clusters")
	}
	hps, err := parseRouteHashPolicies(r.GetHashPolicy())
	if err != nil {
		return nil, err
	}
	wcs := make([]router.WeightedCluster, 0, len(entries))
	weights := make([]uint32, 0, len(entries))
	for _, cw := range entries {
		// Precedence per SPEC §6 / AMEND-WC3: name presence → cluster_header-unsupported
		// → weight presence → cluster-exists. (Empty name + empty cluster_header → the
		// "name is required" arm — envoy-go's analogue of the reference's
		// "At least one of name or cluster_header need to be specified".)
		name := cw.GetName()
		if name == "" {
			return nil, fmt.Errorf("route action: weighted_clusters cluster: name is required")
		}
		if cw.GetClusterHeader() != "" {
			return nil, fmt.Errorf("route action: weighted_clusters cluster_header is not supported (literal cluster name only)")
		}
		if cw.GetWeight() == nil {
			return nil, fmt.Errorf("route action: weighted_clusters cluster %q: weight is required", name)
		}
		c, ok := clusters.Get(name)
		if !ok {
			return nil, fmt.Errorf("route action: weighted_clusters cluster %q not found", name)
		}
		sm, err := mergeRouteSubsetMatch(r.GetMetadataMatch(), cw.GetMetadataMatch())
		if err != nil {
			return nil, err
		}
		wcs = append(wcs, router.WeightedCluster{Cluster: c, SubsetMatch: sm})
		weights = append(weights, cw.GetWeight().GetValue())
	}
	var total uint32
	for _, w := range weights {
		total += w
	}
	if total == 0 {
		return nil, fmt.Errorf("route action: weighted_clusters total weight must be > 0")
	}
	return buildWeightedBridge(wcs, hps, weights)
}

// buildWeightedBridge constructs the selector (the cumulative weights + a
// crypto-seeded RNG — Σweights > 0 already proven above) and the H1/H2 dispatch
// closures sharing it, returning the weightedClusterRouteAction bridge.
func buildWeightedBridge(wcs []router.WeightedCluster, hps []router.HashPolicy, weights []uint32) (routeAction, error) {
	sel, err := router.NewWeightedSelector(weights)
	if err != nil {
		return nil, fmt.Errorf("route action: weighted_clusters: %w", err) // crypto/rand seed failure → boot-fail
	}
	return &weightedClusterRouteAction{
		h1: router.H1WeightedClusterAction(wcs, hps, sel),
		h2: router.H2WeightedClusterAction(wcs, hps, sel),
	}, nil
}
```

Task 6 lands the FULL `buildWeightedRouterAction` + `buildWeightedBridge` (so `TestBuildWeightedRouterAction_AcceptsRecognized` observes a real `*weightedClusterRouteAction`). Task 7 adds only the accept-CASE tests (explicit-0 / ignored total_weight / merge-precedence end-to-end) — no new production code.

> **Test helpers (ADVISORY-3):** `newTestManager(t, names...)` does NOT exist — the as-built hcm helper is `mkClusterManager(t)` (`config_test.go`, a single hardcoded `c_test` cluster). The executor adds a small variadic/named-cluster manager builder (test-side LoC). Similarly the router-side `newEchoClusters`/`bodies`/`mustGET` (Task 4) extend the as-built `singleEndpointCluster`/`loopbackHTTPEcho` (`router_test.go`). Name and reuse the real primitives; add the minimal new helpers.

- [ ] **Step 4: Run to verify PASS**

Run: `go test ./internal/filter/hcm/ -run TestBuildWeightedRouterAction -count=1 -v`
Expected: PASS (6 rejects + the recognized-accept).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/hcm/ && golangci-lint run ./internal/filter/hcm/...
git add internal/filter/hcm/config.go internal/filter/hcm/config_test.go
git commit -m "phase 38.2 IMPL Task 6: buildRouterAction weighted_clusters arm + the SIX-arm reject roster"
```

---

### Task 7: `buildWeightedRouterAction` accept path — selector + bridge build (hcm/config.go)

**Files:**
- Modify: `internal/filter/hcm/config.go`
- Test: `internal/filter/hcm/config_test.go`

- [ ] **Step 1: Write the failing tests** — the accept path builds a working bridge; the merge precedence is honored end-to-end (a weighted route whose route-level + entry metadata_match merge); an explicit `weight: 0` entry is accepted (sum>0 from the other); a non-matching `total_weight` is ignored.

```go
func TestBuildWeightedRouterAction_AcceptCases(t *testing.T) {
	mgr := newTestManager(t, "c_a", "c_b")
	mk := func(mut func(*routev3.WeightedCluster)) *routev3.RouteAction {
		wc := &routev3.WeightedCluster{Clusters: []*routev3.WeightedCluster_ClusterWeight{
			{Name: "c_a", Weight: wrapperspb.UInt32(50)},
			{Name: "c_b", Weight: wrapperspb.UInt32(50)},
		}}
		if mut != nil {
			mut(wc)
		}
		return &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_WeightedClusters{WeightedClusters: wc}}
	}
	// explicit-0 entry accepts (sum>0).
	if _, err := buildRouterAction(mk(func(wc *routev3.WeightedCluster) {
		wc.Clusters[0].Weight = wrapperspb.UInt32(0)
	}), mgr); err != nil {
		t.Errorf("explicit-0 entry should accept: %v", err)
	}
	// non-matching total_weight ignored.
	if _, err := buildRouterAction(mk(func(wc *routev3.WeightedCluster) {
		wc.TotalWeight = wrapperspb.UInt32(999)
	}), mgr); err != nil {
		t.Errorf("non-matching total_weight should accept (deprecated/ignored): %v", err)
	}
}
```

- [ ] **Step 2: Run to verify FAIL**

Run: `go test ./internal/filter/hcm/ -run TestBuildWeightedRouterAction_AcceptCases -v`
Expected: the new accept-case asserts compile-fail or fail until confirmed (Task 6 already landed the full build, so these should pass once the test compiles — confirm they exercise the explicit-0 / ignored-total_weight / merge paths).

- [ ] **Step 3: No new production code** — Task 6 landed the full `buildWeightedRouterAction` + `buildWeightedBridge`. Task 7 is pure TEST coverage: the explicit-0-entry accept, the ignored-`total_weight` accept, and a merge-precedence end-to-end assertion (a weighted route whose route-level + entry `metadata_match` merge produces the entry-precedence `SubsetMatch` on the built entry — assert via a test seam or by driving a request through the H1 closure to a subset-tagged backend). If any accept case reveals a producer gap, fix it here.

- [ ] **Step 4: Run to verify PASS** + the full hcm package

Run: `go test ./internal/filter/hcm/ -count=1` 
Expected: PASS (the new weighted tests + all existing hcm tests — the single-cluster `RouteAction_Cluster` arm is byte-unchanged).

- [ ] **Step 5: gofmt + lint + commit**

```bash
gofmt -l internal/filter/hcm/ && golangci-lint run ./internal/filter/hcm/...
git add internal/filter/hcm/config.go internal/filter/hcm/config_test.go
git commit -m "phase 38.2 IMPL Task 7: buildWeightedRouterAction accept path — selector + bridge build"
```

---

### Task 8: The `0065-weighted-clusters` differential fixture

**Files:**
- Create: `test/fixtures/0065-weighted-clusters/driver/driver.go`
- Create: `test/fixtures/0065-weighted-clusters/expectations.yaml`
- Create: `test/fixtures/0065-weighted-clusters/README.md`

Model the whole fixture on `test/fixtures/0064-lb-subset/` (the cross-side HTTP-route shape: reference STRICT_DNS / `host.docker.internal`, subject STATIC / `127.0.0.1`; the `HTTPEcho` backend echoing `backend-<idx>`) and the `0060-lb-random/` band-asserter.

- [ ] **Step 1: Write the driver constants + config** (per D-WC-IMPL-3/4)

```go
const (
	backendCount = 4 // backend0=c_a, backend1=c_b, backend2=c_sub/v1, backend3=c_sub/v2
	n            = 500 // GET /w per side (the distribution sample)
	healthReqs   = 8

	// weights {50,30,20}; c_sub's ClusterWeight.metadata_match{version:v1} routes
	// ALL c_sub traffic to backend2 → backend3 (v2) gets ZERO (the composition arm).
	wA, wB, wSub = 50, 30, 20
	// Per-backend PER-SIDE bands: mean = n·p, σ = √(n·p·(1−p)), ~4.5σ margin
	// (reference_differential_band_sigma_margin — flake-free ≥20 runs; breaks land far outside).
	//   backend0 p=.50 mean 250 σ≈11.2 → [200,300]
	//   backend1 p=.30 mean 150 σ≈10.25 → [104,196]
	//   backend2 p=.20 mean 100 σ≈8.94 → [60,140]   (all c_sub → v1)
	//   backend3        mean 0          → ==0         (composition affinity)
	b0Lo, b0Hi = 200, 300
	b1Lo, b1Hi = 104, 196
	b2Lo, b2Hi = 60, 140
	totalReqs  = n // DERIVED; /health is direct_response (no backend)
)
```

The route table (both sides): one `/w` route with `weighted_clusters` over `c_a`(50)/`c_b`(30)/`c_sub`(20, `metadata_match {envoy.lb:{version:v1}}`); `c_sub` is a cluster with `lb_subset_config { fallback_policy: ANY_ENDPOINT, subset_selectors:[{keys:[version]}] }` over backend2(`version:v1`)+backend3(`version:v2`); a `/health` `direct_response` `inline_string:"OK\n"`. (Mirror the `0064` `routeTable` + `ReferenceBootstrap`/`SubjectConfig` builders; add the weighted route + the subset cluster.) **`c_sub`'s `fallback_policy: ANY_ENDPOINT` is load-bearing for break (c):** with the `ClusterWeight.metadata_match` PRESENT, `c_sub` routes only to v1 (backend3==0); when break (c) DROPS the match, the empty match falls through ANY_ENDPOINT → spreads across v1+v2 → backend3 > 0 (the affinity break manifests as a v2 hit, not a 503 — which `NO_FALLBACK` would produce instead).

- [ ] **Step 2: Write the driver drive + asserters**

`DriveReference`/`DriveSubject`: send `n` `GET /w` (tally served `backend-<idx>` bodies) + `healthReqs` `GET /health` (collect for byte-equiv). Return the per-backend tally + the health bytes (the `0064` `drive`/`backendIdxFromBody` precedent).

```go
// AssertDistribution: PER-SIDE weighted band on the per-backend /w accept counts
// (BOTH sides independently — independent RNG; cross-side per-request identity
// infeasible per reference_differential_hash_key_cross_side_infeasible / AMEND-WC2).
func (weightedDriver) AssertDistribution(refCounts, subjCounts []uint64) error {
	for _, sd := range []struct {
		side   string
		counts []uint64
	}{{"reference", refCounts}, {"subject", subjCounts}} {
		if len(sd.counts) != backendCount {
			return fmt.Errorf("%s: expected %d backend counts, got %d", sd.side, backendCount, len(sd.counts))
		}
		band := func(i, lo, hi int) error {
			if int(sd.counts[i]) < lo || int(sd.counts[i]) > hi {
				return fmt.Errorf("%s: backend[%d]=%d outside weighted band [%d,%d] (swapped weights? dropped cluster?)", sd.side, i, sd.counts[i], lo, hi)
			}
			return nil
		}
		for _, e := range []struct{ i, lo, hi int }{{0, b0Lo, b0Hi}, {1, b1Lo, b1Hi}, {2, b2Lo, b2Hi}} {
			if err := band(e.i, e.lo, e.hi); err != nil {
				return err
			}
		}
		if sd.counts[3] != 0 { // backend3 = c_sub/v2: the composition affinity (merged match → v1 only)
			return fmt.Errorf("%s: backend[3] (v2) = %d, want 0 (ClusterWeight.metadata_match{version:v1} must exclude v2)", sd.side, sd.counts[3])
		}
		var sum uint64
		for _, c := range sd.counts {
			sum += c
		}
		if sum != totalReqs {
			return fmt.Errorf("%s: conservation: routed sum %d != %d", sd.side, sum, totalReqs)
		}
	}
	return nil
}

// AssertStats: cross-EQUAL the CONSERVATION sum (the per-cluster split is per-side,
// NOT cross-equaled). Σ cluster.{c_a,c_b,c_sub}.upstream_rq_total == n on each side;
// quiesced upstream_cx_active == 0. ZERO new stat names (AMEND-WC4).
func (weightedDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	// scrape both, assert Σ upstream_rq_total over the 3 clusters == n on each side
	// + each cluster.upstream_cx_active == 0 (the 0064 scrapeStats precedent).
}
```

Add the `var _ fixture.DistributionAsserter = (*weightedDriver)(nil)` + `var _ fixture.StatsAsserter = (*weightedDriver)(nil)` + the `init()` registration (the `0064` precedent).

- [ ] **Step 3: Run the fixture (both sides) to verify PASS**

Run: `go test ./test/differential/ -run 'TestDifferential/0065' -count=1 -v` (per `reference_differential_run_selector` — the `TestDifferential/` prefix, NOT a bare `0065`).
Expected: PASS (reference + subject; the bands hold; backend3==0; conservation==n; `/health` byte-equiv).

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0065-weighted-clusters/
git commit -m "phase 38.2 IMPL Task 8: 0065-weighted-clusters fixture (per-side weight-distribution band + composition + conservation)"
```

---

### Task 9: Deliberate-break liveness + ≥20-run flake

**Files:**
- (temporary, reverted) `internal/filter/hcm/config.go` / the `0065` config; `test/fixtures/0065-weighted-clusters/README.md` (record the breaks)

- [ ] **Step 1: Break (a) — swap the weights.** Edit the `0065` weighted route to `{20,30,50}`. Run `go test ./test/differential/ -run 'TestDifferential/0065' -count=1`. Expected: FAIL (backend0 → ~100 < 200; backend2 → ~250 > 140). REVERT.
- [ ] **Step 2: Break (b) — drop a cluster.** Remove `c_b` from the weighted list. Run `-count=1`. Expected: FAIL (conservation sum ≠ 500; backend1 band fails). REVERT.
- [ ] **Step 3: Break (c) — drop the composition `metadata_match`.** Remove `c_sub`'s `ClusterWeight.metadata_match`. Run `-count=1`. Expected: FAIL (backend3 (v2) > 0 — the merged match no longer excludes v2; backend2 < 60). REVERT.
- [ ] **Step 4: Flake check.** `for i in $(seq 1 20); do go test ./test/differential/ -run 'TestDifferential/0065' -count=1 || echo "FLAKE run $i"; done`. Expected: 20/20 PASS, no FLAKE line (the ~4.5σ band).
- [ ] **Step 5: Record the breaks in the `0065` README** (the `0030`/`0064` lesson — the deliberate-break protocol + the constants-desync guard) and **commit** the README.

```bash
git add test/fixtures/0065-weighted-clusters/README.md
git commit -m "phase 38.2 IMPL Task 9: 0065 deliberate-break liveness (swap-weights/drop-cluster/drop-compose) + 20/20 flake"
```

---

### Task 10: Full differential re-verify + completion bundle

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/phases/38.2-weighted-clusters/{README.md,PROGRESS.md}`

- [ ] **Step 1: The six-gate (ADR-0052).**
```bash
gofmt -l internal/ test/                         # empty
golangci-lint run ./...                           # clean
go build ./...                                     # ok
go mod tidy -diff                                  # exit 0, EMPTY (ZERO new dep — AMEND-WC1)
go test ./... -count=1                             # all green (the 66 prior dirs byte-exact + 0065)
go test ./test/differential/ -race -short -count=1 # race-clean
```
Expected: all green; the full differential is **67** dirs.

- [ ] **Step 2: h2spec / proxy-wasm asserted-unaffected.** 38.2 touches the HTTP route producer + a per-request selector, NOT the h2 framing or wasm path (the wire path is byte-identical when no `weighted_clusters` route is configured). Re-run the conformance gates per the project recipe; expect h2spec 53/53 + proxy-wasm 10/10.

- [ ] **Step 3: BEHAVIOR_CONTRACT delta (SPEC §9).** Add the `### Route action — weighted clusters (weighted_clusters)` subsection (the selection semantics; the merge; the §6 reject roster; the deferred surface) + a NOTE that the stat surface is UNCHANGED (1125; the distribution is observable via the existing per-cluster `upstream_rq_*`; the `max_host_weight` non-relation) + the departure records.

- [ ] **Step 4: ADR-0241 (ADR-0044 in-place).** Promote the SPEC §13 §Context DRAFT into `DECISIONS.md` as `## ADR-0241` with §Context + §Decision + §Consequences (status ACCEPTED); advance the DECISIONS tail to ADR-0241 (next-free ADR-0242). The §Decision records: the weighted-random selection (the cumulative-walk algorithm + the per-action RNG); the metadata_match merge (entry precedence, route-static, the ADR-0239 seam reuse); the SIX rejects; the ZERO-stat finding; the per-side-band differential.

- [ ] **Step 5: STATE / ROADMAP / README / PROGRESS.** STATE active-phase → `phase 38.2 (weighted-clusters) IMPL done`; the counts (fixtures **66 → 67**, DECISIONS tail **ADR-0240 → ADR-0241**, stat surface 1125 / fuzzers 42 / BackendKind 33 UNCHANGED); next-skill → the next Load-balancing candidate or a new family BRAINSTORM. ROADMAP row 38 → the 38.2-IMPL-DONE sub-leg record (row stays `done`; NO parent rollup per ADR-0106). README status 3 → 4 (IMPL done). PROGRESS.md → all 10 tasks complete + the six-gate evidence.

- [ ] **Step 6: Commit the completion bundle**

```bash
git add docs/ test/ internal/
git commit -m "phase 38.2 IMPL: weighted-clusters (RouteAction.weighted_clusters + ClusterWeight.metadata_match) — the per-request cluster-selection primitive; ADR-0241; 0065 fixture; fixtures 66→67"
```

---

## Notes for the executor

- **DRY/YAGNI:** reuse `doH1ClusterAction`/`doH2ClusterAction` UNCHANGED (do NOT fork the dispatch); reuse `parseRouteHashPolicies` + `ScalarsFromStruct` + `NewSubsetMatch` + `cluster.WithSubsetMatch` UNCHANGED; the `randomLB` RNG model is mirrored, not refactored.
- **The single-cluster path is byte-stable:** the `RouteAction_Cluster` arm of `buildRouterAction` keeps its exact behavior (the existing `clusterRouteAction` tests + the 66 prior differential dirs are the guard) — only the type-assert becomes a `switch`.
- **NO `loadBalancer.Pick` change, NO exported-cluster change, NO new stat, NO new fuzzer, NO new BackendKind, NO new go.mod dep** (SPEC §4/§7/§8.3 — assert these explicitly at T10).
- **Deferred-field disposition (D-WC-IMPL-2):** `cluster_header` REJECTS; `request/response_headers_to_*` + `typed_per_filter_config` + `host_rewrite` SILENT-IGNORE (do NOT add accept-but-inert plumbing — just don't read them).
- Reference the SPEC §11 D-WC pins as the empirical authority; do NOT re-probe (ADR-0004 was executed at the SPEC).
