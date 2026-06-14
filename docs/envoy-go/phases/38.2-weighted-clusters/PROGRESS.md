# Phase 38.2 IMPL Progress — `RouteAction.weighted_clusters` routing

> **Worktree:** `.worktrees/phase-38.2-impl` · **Branch:** `phase-38.2-impl` (from master `3a492d2`)
> **Anchored at IMPL-session tip:** `3a492d2` (2026-06-14 "next-prompt.txt: SHA-fill the live tip")

---

## Task Table

| # | Task | Status |
|---|------|--------|
| 1 | First-task baselines/anchors gate + PROGRESS.md | **complete** |
| 2 | `weightedSelector` + RNG + `pick()` (`router_weighted.go`) | **complete** |
| 3 | `mergeRouteSubsetMatch` (`hcm/config.go`) | **complete** |
| 4 | `WeightedCluster` type + `H1`/`H2WeightedClusterAction` (`router_weighted.go`) | **complete** |
| 5 | `weightedClusterRouteAction` bridge (`hcm/actions.go`) | **complete** |
| 6 | `buildRouterAction` `RouteAction_WeightedClusters` switch arm + the SIX-arm reject roster + `buildWeightedRouterAction`/`buildWeightedBridge` | **complete** |
| 7 | Accept-case test coverage | **complete** |
| 8 | `0065-weighted-clusters` fixture | **complete** |
| 9 | Deliberate-break liveness + flake check | **complete** |
| 10 | Full differential re-verify + completion bundle | **complete** |

---

## Step 1: Count Anchors (IMPL-session tip `3a492d2`)

All counts confirmed against the live worktree — **no surprises, all match expected**.

| Anchor | Expected | Actual | Status |
|--------|----------|--------|--------|
| fixture count (`ls -d test/fixtures/[0-9]* \| wc -l`) | 66 | **66** | OK |
| fixture tail (`ls -d test/fixtures/0064-lb-subset`) | `test/fixtures/0064-lb-subset` | **`test/fixtures/0064-lb-subset`** | OK |
| fuzzer count (`grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l`) | 42 | **42** | OK |
| stat surface DOC count (`grep -n "1125" BEHAVIOR_CONTRACT.md`) | lines mentioning 1125 | **lines 1033, 4148** (both confirm `1121 → 1125`) | OK |
| `DECISIONS.md` tail (`grep "^## ADR-02" … \| tail -1`) | `## ADR-0240 …` | **`## ADR-0240 …`** | OK (next-free ADR-0241) |

---

## Step 2: As-Built Line Anchors (IMPL-session tip `3a492d2`)

All anchors confirmed live. **No drift** detected.

### `internal/filter/hcm/config.go`

| Symbol | Line |
|--------|------|
| `func buildAction(...)` | **507** |
| `func buildRouterAction(...)` (builds `clusterRouteAction`; gains `WeightedClusters` arm at Task 6) | **536** |
| `func parseRouteSubsetMatch(...)` (precedent for `mergeRouteSubsetMatch` at Task 3) | **567** |
| `func parseRouteHashPolicies(...)` | **583** |

### `internal/filter/hcm/actions.go`

| Symbol | Line |
|--------|------|
| `type clusterRouteAction struct` | **201** |
| `func (a *clusterRouteAction) do(...)` | **213** |
| `func (a *clusterRouteAction) asRouterAction()` | **236** |
| `func (a *clusterRouteAction) asRouterActionH2()` | **249** |

### `internal/filter/hcm/route.go`

| Symbol | Line |
|--------|------|
| `type routeAction interface` — methods: `do`, `asRouterAction`, `asRouterActionH2` | **63** |

### `internal/filter/http/router/router.go`

| Symbol | Line |
|--------|------|
| `func applyHashKey(...)` | **514** |
| `func H1ClusterAction(c *cluster.Cluster, hps []HashPolicy, subsetMatch cluster.SubsetMatch) Action` | **558** |
| `func doH1ClusterAction(...)` | **588** |
| `type routerAction struct` | **724** |

### `internal/filter/http/router/router_h2.go`

| Symbol | Line |
|--------|------|
| `func H2ClusterAction(c *cluster.Cluster, hps []HashPolicy, subsetMatch cluster.SubsetMatch) H2Action` | **39** |
| `func doH2ClusterAction(...)` | **57** |
| `type routerActionH2 struct` | **219** |

### `internal/cluster/subset.go`

| Symbol | Line |
|--------|------|
| `func NewSubsetMatch(m map[string]SubsetValue) SubsetMatch` | **68** |
| `func (m SubsetMatch) Empty() bool` | **81** |
| `func ScalarsFromStruct(s *structpb.Struct) (map[string]SubsetValue, []string)` | **263** |

### `internal/cluster/leastrequest.go` + `random.go`

| Symbol | File | Line |
|--------|------|------|
| `func newPCGRNG()` (RNG constructor — REUSED by `weightedSelector` at Task 2) | `leastrequest.go` | **63** |
| `func newRandomWithRNG(endpoints []Endpoint, rng func() uint64) *randomLB` (TEMPLATE for weighted pick) | `random.go` | **31** |

---

## Six-gate evidence

*(filled at Task 10 — all gates GREEN; 2026-06-14)*

| Gate | Evidence |
|------|----------|
| (1) `gofmt -l internal/ test/` | **EMPTY** (no formatting drift) |
| (1) `golangci-lint run ./...` | **CLEAN** (no lint findings) |
| (1) `go build ./...` | **OK** (exit 0) |
| (1) `go mod tidy -diff` | **EMPTY** (exit 0 — ZERO new go.mod dep) |
| (2) `go test ./... -count=1` | **ALL GREEN** (~225s; all packages pass including `test/differential` + `test/conformance/h2spec` + `test/conformance/proxy-wasm`) |
| (3) `go test ./test/differential/ -race -short -count=1` | **RACE-CLEAN** (~1.2s; ok `github.com/esalaine/envoy-go/test/differential`) |
| (4) h2spec conformance | **53/53 PASS** — asserted-unaffected by change-scope (38.2 touches ONLY `router_weighted.go` [new] + `hcm/{actions,config}.go` [weighted arm] + `0065` fixture + docs — NO HTTP/2 framing path; the full 67-dir differential is the real guard) |
| (4) proxy-wasm conformance | **10/10 PASS** — asserted-unaffected by change-scope (same reasoning; NO wasm path) |
| (5) fixture count | `ls -d test/fixtures/[0-9]* \| wc -l` = **67** (66 prior + `0065-weighted-clusters`) |
| (5) fuzzer count | `grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l` = **42** (UNCHANGED) |
| (5) stat surface | **1125** (UNCHANGED — ZERO new stat names; see the BEHAVIOR_CONTRACT "Phase 38.2" note) |
| (5) BackendKind tail | **33** (UNCHANGED — `0065` reuses the `0063`/`0064` `HTTPEcho`) |
| (5) DECISIONS.md tail | **ADR-0241** (advanced from ADR-0240 at this IMPL; next-free ADR-0242) |
| (6) docs | BEHAVIOR_CONTRACT + DECISIONS (ADR-0241 IN-PLACE) + STATE + ROADMAP + README + PROGRESS all updated at Task 10 completion bundle commit |
