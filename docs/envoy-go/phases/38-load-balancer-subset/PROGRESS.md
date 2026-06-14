# Phase 38.1 IMPL Progress — `subset` LB (`Cluster.lb_subset_config`)

> **Worktree:** `.worktrees/phase-38-impl` · **Branch:** `phase-38-impl` (from master `c8e8d9b`)
> **Anchored at IMPL-session tip:** `c8e8d9b` (2026-06-13 "next-prompt.txt: add a forward-horizon appendix")

---

## Task Table

| # | Task | Status |
|---|------|--------|
| 1 | First-task baselines/anchors gate + PROGRESS.md | **complete** |
| 2 | `SubsetMatch` / `SubsetValue` / `valueEqual` / `key()` + `ScalarsFromStruct` (`subset.go`) | **complete** |
| 3 | `Endpoint.Metadata` + `extractEndpoints` `envoy.lb` capture | **complete** |
| 4 | Seam-input widening (`Pick` interface + 5 leaves + `WithSubsetMatch` + funnels) | **complete** |
| 5 | `buildLeafLB` extraction | **complete** |
| 6 | `subsetLB` enumeration + build | **complete** |
| 7 | `subsetLB.Pick` (match / fallback / NO_FALLBACK) | **complete** |
| 8 | `manager` `parseLbSubsetConfig` + wrap-after-switch | **complete** |
| 9 | 4 `lb_subsets_*` registrations + counter injection | **complete** |
| 10 | HTTP `metadata_match` producer | **complete** |
| 11 | `0064-lb-subset` differential fixture | **complete** |
| 12 | Deliberate-break liveness + flake check | **complete** |
| 13 | Full differential re-verify gate | **complete** |
| 14 | Completion bundle (ADR-0052 six-gate) | **complete** |

---

## Step 1: Count Anchors (IMPL-session tip `c8e8d9b`)

All counts confirmed against the live worktree — **no surprises, all match expected**.

| Anchor | Expected | Actual | Status |
|--------|----------|--------|--------|
| fixture count (`ls -d test/fixtures/[0-9]* \| wc -l`) | 65 | **65** | OK |
| fixture tail (`ls -d test/fixtures/[0-9]* \| tail -1`) | `test/fixtures/0063-lb-maglev` | **`test/fixtures/0063-lb-maglev`** | OK |
| `BackendKind` tail (`fixture.go:562`) | `TCPThriftResponder BackendKind = 33` | **`TCPThriftResponder BackendKind = 33`** (line 562) | OK |
| fuzzer count (`grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) \| wc -l`) | 42 | **42** | OK |
| `DECISIONS.md` tail (`grep "^## ADR-0" … \| tail -1`) | `## ADR-0238 …` | **`## ADR-0238 …`** | OK (ADR-0239/0240 §Context are DRAFTs in SPEC §13 only) |
| stat surface DOC count (`grep -n "1121" BEHAVIOR_CONTRACT.md`) | lines mentioning 1121 | **lines 995, 4107** (both confirm `1119 → 1121`) | OK |
| `go build ./...` | `BUILD_OK` | **`BUILD_OK`** | OK |
| `go mod tidy -diff` | empty (exit 0) | **empty** | OK (ZERO new dep — AMEND-SS1) |

**Stat surface:** 1121 (a DOC count in `BEHAVIOR_CONTRACT.md` — NOT a programmatic golden; the subset phase will advance it to 1125).

---

## Step 2: As-Built Line Anchors (IMPL-session tip `c8e8d9b`)

All anchors confirmed live. **No drift** from the PLAN's SPEC-era pins (the intervening commits were docs-only `next-prompt.txt` SHA-fills).

### `internal/cluster/loadbalancer.go`

| Symbol | Line |
|--------|------|
| `Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)` (interface method — WIDENS at Task 4) | **21** |
| `var noopRelease = func() {}` (REUSED by subsetLB) | **26** |
| `func (rr *roundRobin) Pick(_ uint64, _ bool)` | **39** |
| `errNoEndpoints` (REUSED) | **41** |

### `internal/cluster/cluster.go`

| Symbol | Line |
|--------|------|
| `type Endpoint struct` (gains `Metadata` at Task 3) | **33** |
| `func (e Endpoint) Addr()` (UNCHANGED — Metadata NOT part of dial identity) | **39** |
| `type hashKeyCtxKey struct{}` (ctx-key TEMPLATE for `WithSubsetMatch`/`subsetMatchFrom`) | **172** |
| `func WithHashKey(ctx context.Context, key uint64) context.Context` | **178** |
| `func hashKeyFrom(ctx context.Context) (uint64, bool)` | **182** |
| `c.lb.Pick(0, false)` — PickEndpoint funnel | **196** |
| `c.lb.Pick(hk, ok)` — Dial funnel | **233** |
| `c.lb.Pick(hk, ok)` — AcquireH1 funnel | **287** |

All THREE `c.lb.Pick(` funnels confirmed (lines 196, 233, 287).

### `internal/cluster/manager.go`

| Symbol | Line |
|--------|------|
| `switch c.GetLbPolicy()` (EXTRACTED to `buildLeafLB` at Task 5) | **251** |
| `case clusterv3.Cluster_ROUND_ROBIN:` | **252** |
| `case clusterv3.Cluster_LEAST_REQUEST:` | **254** |
| `case clusterv3.Cluster_RANDOM:` | **264** |
| `case clusterv3.Cluster_RING_HASH:` | **270** |
| `case clusterv3.Cluster_MAGLEV:` | **280** |
| `unsupported lb_policy` default reject (UNTOUCHED) | **291** |
| `func extractEndpoints(...)` (`Endpoint` construction capture site — Task 3) | **517** |
| `Endpoint{Host:` construction site (gains `Metadata:` field at Task 3) | **533** |
| `func registerClusterMetrics(r *stats.Registry, c *Cluster)` | **99** |
| `if rh, ok := c.lb.(*ringHashLB); ok {` (gauge type-assert precedent) | **110** |
| `if mg, ok := c.lb.(*maglevLB); ok {` (gauge type-assert precedent) | **118** |
| `func parseRingHashLbConfig(...)` (parse precedent for `parseLbSubsetConfig`) | **363** |
| `func parseMaglevLbConfig(...)` (parse precedent) | **407** |

### Leaf `Pick` signatures (all WIDEN at Task 4)

| File | Symbol | Line |
|------|--------|------|
| `loadbalancer.go` | `func (rr *roundRobin) Pick(_ uint64, _ bool)` | **39** |
| `leastrequest.go` | `func (lr *leastRequest) Pick(_ uint64, _ bool)` | **81** |
| `random.go` | `func (r *randomLB) Pick(_ uint64, _ bool)` | **37** |
| `maglev.go` | `func (mg *maglevLB) Pick(hashKey uint64, hasHash bool)` | **39** |
| `ringhash.go` | `func (rh *ringHashLB) Pick(hashKey uint64, hasHash bool)` | **129** |

### `internal/filter/hcm/actions.go`

| Symbol | Line |
|--------|------|
| `type clusterRouteAction struct` (gains `subsetMatch` at Task 10) | **201** |
| `router.H1ClusterAction(a.cluster, a.hashPolicies)` (first funnel — `:213`) | **213** |
| `router.H1ClusterAction(a.cluster, a.hashPolicies)` (second funnel — `:236`) | **236** |
| `router.H2ClusterAction(a.cluster, a.hashPolicies)` (third funnel — `:249`) | **249** |

`doH1ClusterActionDirect` confirmed ABSENT (grep returns empty — no such symbol anywhere in `internal/` or `cmd/`).

### `internal/filter/hcm/config.go`

| Symbol | Line |
|--------|------|
| `func buildRouterAction(...)` (builds `clusterRouteAction`; calls `parseRouteHashPolicies`) | **536** |
| `*routev3.RouteAction_Cluster` (non-Cluster ClusterSpecifier reject at `:540`) | **540** |
| `func parseRouteHashPolicies(...)` (precedent for `parseRouteSubsetMatch`) | **566** |

### `internal/filter/http/router/router.go`

| Symbol | Line |
|--------|------|
| `func applyHashKey(ctx context.Context, hps []HashPolicy, ...)` (producer TEMPLATE) | **514** |
| `cluster.WithHashKey(ctx, acc)` within `applyHashKey` | **544** |
| `func H1ClusterAction(c *cluster.Cluster, hps []HashPolicy) Action` | **558** |
| `func doH1ClusterAction(ctx context.Context, a *routerAction, req *http.Request)` | **588** |
| `type routerAction struct` | **716** |

### `internal/filter/http/router/router_h2.go`

| Symbol | Line |
|--------|------|
| `func H2ClusterAction(c *cluster.Cluster, hps []HashPolicy) H2Action` | **39** |
| `func doH2ClusterAction(ctx context.Context, a *routerActionH2, req h2.H2Request)` | **57** |
| `type routerActionH2 struct` | **211** |

### `internal/stats/registry.go` and `counter.go`

| Symbol | File | Line |
|--------|------|------|
| `func (r *Registry) NewCounter(name string) *Counter` | `registry.go` | **79** |
| `func (r *Registry) NewGauge(name string) *Gauge` | `registry.go` | **95** |
| `func (c *Counter) Inc()` | `counter.go` | **22** |
| `func (c *Counter) Add(delta uint64)` | `counter.go` | **27** |
| `func (c *Counter) Load() uint64` | `counter.go` | **30** |

**Counter has both `Inc()` AND `Add(delta uint64)` AND `Load() uint64`** — confirmed (Task 9 injects `numSubsets` via `Add`).

### `internal/cluster/subset.go`

**ABSENT** — confirmed `subset.go ABSENT (expected — Task 2 creates it)`.

---

## Step 3: Reject-Surface Confirmation (AMEND-SS1 — EMPTY new cluster-arm)

| Check | Result |
|-------|--------|
| `grep -rln "unsupported lb_policy" internal/ cmd/` | **ONLY `internal/cluster/manager.go`** |
| `grep -n "ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV" manager.go` | **line 291** — supported-list STAYS UNTOUCHED (subset is NOT an enum value) |
| `func TestManager_Error_UnsupportedLBPolicy` in `manager_test.go` | **line 320** — UNTOUCHED (no doubly-hit retarget; the `CLUSTER_PROVIDED + lb_subset_config` combo is pre-covered by this existing gate) |
| `grep -rln "cluster_specifier.*not supported\|literal cluster name only" internal/filter/hcm/` | **`internal/filter/hcm/config.go`** — 38.2 territory (weighted_clusters reject), UNTOUCHED here |

**Verdict:** 38.1 adds **ZERO new cluster-config reject arm**. The only new reject this phase is the producer-side `router-metadata-match-nonscalar` arm (unit-level, Task 10; ADR-0080 / SPEC §6.2).

The `CLUSTER_PROVIDED + lb_subset_config` combination is already rejected by the pre-existing unsupported-policy gate at `manager.go:291` — no new arm needed (AMEND-SS1).

---

## D-question Resolutions (SPEC §12, from PLAN.md)

- **D-S38-1 (file placement):** RESOLVED. `subsetLB` + value types + enumeration/resolution/fallback → NEW `internal/cluster/subset.go`. `buildLeafLB` + wrap-after-switch + `parseLbSubsetConfig` + 4 stat registrations → `manager.go`. `Endpoint.Metadata` + `WithSubsetMatch`/`subsetMatchFrom` + funnel widening → `cluster.go`. Producer (`parseRouteSubsetMatch` + `clusterRouteAction.subsetMatch` + `H1`/`H2ClusterAction` constructors + dispatch thread) → `hcm/config.go` + `hcm/actions.go` + `router/router.go` + `router/router_h2.go`. ZERO new packages.

- **D-S38-2 (the `router-metadata-match-nonscalar` reject wording + endpoint-side disposition):** RESOLVED. House-prefixed `router: metadata_match envoy.lb key %q: only scalar values (string, number, bool) are supported`. Lowering shared via `cluster.ScalarsFromStruct(*structpb.Struct)` → scalar map + non-scalar key list. Producer REJECTS on non-empty non-scalar list; `extractEndpoints` DROPS non-scalar silently. `structpb` null/unset → treated as non-scalar (rejected on route side, dropped on endpoint side).

- **D-S38-3 (empty-`keys`-selector + empty-`subset_selectors` disposition):** RESOLVED. ACCEPT (parity — AMEND-SS1). Empty-`keys` selector → single subset keyed by empty tuple over ALL endpoints (benign no-op that inflates distinct-subset count by ≤1). Empty `subset_selectors: []` → no subsets built; every request takes the fallback path. Both unit-tested in Tasks 6/8.

- **D-S38-4 (`lb_subsets_selected`/`fallback` counter INJECTION mechanics):** RESOLVED. Build-before-register ordering sound: `buildCluster` builds `cl.lb` (the `subsetLB`, with `numSubsets` computed and `selected`/`fallbackC` nil), THEN `NewManager` calls `registerClusterMetrics(r, c)` which type-asserts `*subsetLB`, allocates 4 counters/gauge, ASSIGNS `selected`/`fallbackC` pointers onto the live `subsetLB`, and Sets `active`/`created` to `numSubsets`. Counters Inc'd from `subsetLB.Pick` thereafter. ADR-0024 per-cluster scope holds; new "LB Inc's its own stats from Pick" wrinkle recorded in ADR-0240 §Consequences.

- **D-S38-5 (`0064` final constants + break protocol):** RESOLVED. 4 endpoints (`version: v1` ×2, `version: v2` ×2) / 2 version subsets / `fallback_policy: ANY_ENDPOINT` / routes `/v1`, `/v2`, `/none`, `/health` / K=16 per route × 3 routed routes + 8 `/health`; `totalReqs` DERIVED from named constants. Break protocol: drop-the-`metadata_match` (affinity), misroute-the-fallback (fallback + stats), stats-prong-drop — all with `-count=1`; ≥20-run flake check; `reference_fixture_workload_constant_desync` guard (Tasks 11–12).

- **D-S38-6 (standalone subset property test vs folded):** RESOLVED. FOLDED into Tasks 6/7's `subset_test.go` as a deterministic property test (random endpoint-metadata sets + selectors → every endpoint lands in exactly the subsets its value-tuple matches; resolver never panics; fallback fires per policy; value-equality double-valued for numbers). The `maglev`/`ringhash` "RandomKeysNeverPanicAlwaysValid" folded-property precedent. NOT a `Fuzz*` corpus entry (metadata derives from validated xDS config; fuzzers STAY 42).

---

## ADR-0045 Split-Gate Re-check (FINAL)

**NO FURTHER SPLIT within 38.1.** This PLAN decomposes into **14 tasks** (≤25) over **~300–450 production LoC**:
- `subset.go` ~180–250 LoC
- `cluster.go` `Endpoint.Metadata`/`WithSubsetMatch`/funnels ~35 LoC
- `manager.go` `buildLeafLB`/wrap/`parseLbSubsetConfig`/`extractEndpoints`/4 registrations ~70 LoC
- 5 leaf signature widenings ~15 LoC
- producer (`parseRouteSubsetMatch` + threading) ~50 LoC

Total ≤~1500 LoC including tests. Both ADR-0045 axes hold. The seam widening + leaf-factory extraction + wrapper + producer are ONE self-contained whole on the HTTP `metadata_match` plane. 38.2 (weighted-cluster routing) is the pre-authorized peel, deferred per SPEC §3.0.

---

## Task 12: Deliberate-Break Liveness + Flake Check

Three deliberate breaks applied ONE AT A TIME (temporary; only the README break
table + this evidence are committed), each run with `-count=1` to defeat go-test
caching (`reference_differential_break_protocol_count1`) under selector
`-run 'TestDifferential/0064'` (`reference_differential_run_selector`). Baseline
GREEN confirmed first (2.26s). Docker reachable (Server 28.1.1, Desktop).

| # | break | prong proven | observed `--- FAIL` | restore + re-pass |
|---|-------|--------------|---------------------|-------------------|
| 1 | drop `/v1` `metadata_match` (→ plain `cluster: c_echo` route → ANY_ENDPOINT fallback) | SET-membership affinity (`assertSubsetMembership` `/v1`) | `ref drive: /v1 affinity LEAK: backend[2] served a /v1 request but is not in the subset [0 1] (subset boundary breached)` | `git restore` → re-PASS |
| 2 | misroute fallback (`/none` `version: "v9"` → `"v1"`) | cross-side stats prong | `ref/subj lb_subsets_selected = 48, want 32` AND `ref/subj lb_subsets_fallback = 0, want 16` | `git restore` → re-PASS |
| 3 | perturb expected `lb_subsets_selected` `selectedReqs`(32) → `99` in `AssertStats` | cross-side stats prong | `ref/subj cluster.c_echo.lb_subsets_selected = 32, want 99` | `git restore` → re-PASS |

All three EXPECTED failures observed → every `0064` prong is LIVE (the green fixture
WOULD catch a subset leak, a fallback misroute, and a stat drift). After each restore
`git status` was clean and the branch stayed `phase-38-impl`; the final re-run after
all restores is GREEN (2.26s).

Note on break 2: the `/none` fallback-SPREAD `drive()` leg does NOT fire (the v1
subset `{0,1}` still hits ≥2 backends), but the stats prong catches the misroute
precisely — both `lb_subsets_selected` and `lb_subsets_fallback` are live.

**Flake check:** `go test ./test/differential/ -run 'TestDifferential/0064'
-count=20` → **20/20 PASS** (35.9s, full cross-side incl. reference container boot
per run). `go test ./test/fixtures/0064-lb-subset/driver/ -count=20` → **20/20 PASS**
(the unit asserter logic). The SET-membership/within-subset-spread legs are
DETERMINISTIC (static `metadata_match` + `ROUND_ROBIN` over a 2-host subset over 16
requests); no σ-band.

**Constant-sync confirmation (`reference_fixture_workload_constant_desync`):**
`totalReqs = routes * perRoute` (48), `selectedReqs = 2 * perRoute` (32),
`fallbackReqs = perRoute` (16) are ALL DERIVED from named constants — no literal.
`AssertDistribution` sums the per-backend `sd.counts` slice dynamically against
`totalReqs` (no hand-rolled count slice). gofmt clean over the fixture dir.

## Notes for Later Tasks

- `reference_differential_hash_key_cross_side_infeasible` is **INVERTED** for subset: the `metadata_match` is STATIC config (identical on both sides) → NAT-transparent → subset-affinity SET-membership holds TRUE cross-side. Host-identity obstacle does NOT bite here.
- `reference_fixture_workload_constant_desync` guard: N/K/totalReqs MUST stay synced with any hand-rolled count slices; go-test caching masks desync until `-count=1` (Tasks 11/12 — DERIVE `totalReqs` from named constants, never a literal).
- `reference_differential_run_selector`: targeted runs use `-run 'TestDifferential/0064'`, NEVER `-run '0064'`.
- Counter `Add(delta uint64)` (line 27 in `counter.go`) is the method for injecting `numSubsets` into `lb_subsets_created` + `lb_subsets_active` at Task 9 build time.

---

## Task 13: Full Differential Re-verify Gate

The full 66-dir differential ran GREEN end-to-end under `-count=1` (cache-defeated): **exit 0, ~223s**. The 65 prior dirs are byte-identical through the new `Pick(hashKey, hasHash, match, hasMatch)` signature + the wrap-after-switch (the five leaf policies absorb the widened pick-input behaviour-neutrally; the wire path is byte-identical when no `lb_subset_config` cluster is configured). `0064-lb-subset` is the 66th dir (tail).

---

## Task 14: Completion Bundle — the ADR-0052 Six-Gate

The documentation/state-only atomic landing. NO production code changes (Tasks 1–13 built + committed all production code). The six-gate evidence:

**Gate (1) — build + tidy.** `go build ./...` → **BUILD_OK**. `go mod tidy -diff` → **EMPTY** (ZERO new go.mod dep — the `Cluster.LbSubsetConfig` / `LbEndpoint.metadata` / `RouteAction.metadata_match` proto fields are in the pinned `/envoy v1.32.4`).

**Gate (2) — unit suite.** `go vet ./...` → **VET_OK**. `go test -race -short ./internal/... ./cmd/...` → **all green** (incl. `internal/cluster` `subset_test.go` — the folded subset-resolution property test; `internal/filter/hcm` the `router-metadata-match-nonscalar` reject; `internal/filter/http/router` + `router_h2` the dispatch thread). (The full `go test ./...` includes the Docker differential package; the controller confirmed the full 66-dir differential green out-of-band — see Gate 3.)

**Gate (3) — full differential + liveness.** The full **66-dir** differential GREEN `-count=1` (exit 0, ~223s — Task 13). `0064-lb-subset` liveness-proven via **3 deliberate breaks** (Task 12): drop-`metadata_match` → SET-membership affinity FAILS; misroute-the-fallback → cross-side `selected`/`fallback` stats prong FAILS; stats-perturb → cross-side stats prong FAILS; each `git restore`d + re-PASS; 20/20 flake-free.

**Gate (4) — conformance.** h2spec **53/53** + proxy-wasm **10/10** asserted-unaffected by change-scope construction (subset touches `internal/cluster/*` + the HTTP route `metadata_match` parse + the `0064` fixture/docs — NO HTTP/2 or proxy-wasm path; the wire path is byte-identical when no `lb_subset_config` cluster is configured; the full 66-dir differential is the real guard).

**Gate (5) — counts.** fixtures **65 → 66** (tail `0064-lb-subset`; `ls -d test/fixtures/[0-9]* | wc -l` = 66) / stat surface **1121 → 1125** (+4 `lb_subsets_*`) / fuzzers **42** (UNCHANGED) / BackendKind tail **33** (UNCHANGED) / DECISIONS tail **ADR-0238 → ADR-0240** (ADR-0239 seam+producer / ADR-0240 policy+`Endpoint.Metadata`, both ACCEPTED in-place per ADR-0044; next-free ADR-0241).

**Gate (6) — docs.** `BEHAVIOR_CONTRACT.md` — the NEW `### Load balancer — subset (lb_subset_config)` subsection (after maglev) + the 4 `lb_subsets_*` stat-table rows + the explanatory paragraph + the departure/coverage records + the "Phase 38.1 extension — 1121 → 1125" stat-surface block. `DECISIONS.md` — ADR-0239 + ADR-0240 PROMOTED from the SPEC §13 §Context drafts (PROPOSED → ACCEPTED, bodies IN-PLACE per ADR-0044). `STATE.md` — active-phase / lifecycle-state / next-skill / last-commit / last-updated / next-free ADR + the Project-counts block. `ROADMAP.md` — row 38 status `in-progress → done`, date `2026-06-14`, the "Next →" sentence REPLACED with the IMPL-DONE note.

**ADR-0045 verdict HELD — NO further split.** The PLAN's 14 tasks / ~300–450 prod LoC executed as one self-contained whole on the HTTP `metadata_match` plane; both ADR-0045 axes (`> ~25 tasks OR > ~1500 LoC`) stayed well under the gate. The 38.2 weighted-cluster leg is the pre-authorized peel (deferred to its own SPEC).
