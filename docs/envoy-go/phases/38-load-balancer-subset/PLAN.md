# Phase 38.1 Implementation Plan — `subset` LB (`Cluster.lb_subset_config`): a metadata-match endpoint-subset WRAPPER over the cluster's `lb_policy` child — the FIRST LB wrapper, the SECOND seam BUILD, the FIRST per-endpoint metadata dimension

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land `Cluster.lb_subset_config` (`Cluster.LbSubsetConfig`, `envoy.config.cluster.v3`, proto field 22) — the project's FIFTH Load-balancing construct and its FIRST WRAPPER. `lb_subset_config` is NOT a `Cluster.LbPolicy` enum value; its presence WRAPS whatever child the `lb_policy` switch produced, partitioning the endpoint set into precomputed subsets (one per distinct `envoy.lb` endpoint-metadata value-tuple per selector) and delegating each subset to a child `loadBalancer` built from the cluster's `lb_policy`. At pick time it resolves the route's `RouteAction.metadata_match["envoy.lb"]` (carried via ctx) to a subset and delegates; on a miss it applies the cluster-level fallback {NO_FALLBACK / ANY_ENDPOINT / DEFAULT_SUBSET}.

**Architecture:** A new `subsetLB` type (`internal/cluster/subset.go`, same package) wraps a child-LB factory. The existing `buildCluster` `lb_policy` switch is EXTRACTED into a `buildLeafLB(c, name, endpoints)` factory (the same switch over an arbitrary endpoint sub-slice); `subsetLB` calls it per subset + per fallback. The `loadBalancer.Pick(hashKey, hasHash)` seam widens with a SECOND pick-input — the resolved subset match — `Pick(hashKey, hasHash, match SubsetMatch, hasMatch bool)`; the five leaf policies absorb `(match, hasMatch)` behavior-neutrally; `subsetLB` consumes it. `cluster.go` gains `WithSubsetMatch`/`subsetMatchFrom` (the `WithHashKey`/`hashKeyFrom` analogue) and the funnels (`Dial`/`AcquireH1`/`PickEndpoint`) extract both inputs. `Endpoint` grows a `Metadata map[string]SubsetValue` field (the parsed `envoy.lb` scalar map — the FIRST per-endpoint dimension beyond `Host:Port`), captured at `extractEndpoints`. The HTTP router parses `RouteAction.metadata_match["envoy.lb"]` ONCE at config-build into a proto-free `cluster.SubsetMatch` stored on `clusterRouteAction` (route-static — NO per-request fold) and threads it via `WithSubsetMatch` at dispatch. Four mirrored `lb_subsets_*` cluster stats describe the machinery (surface 1121 → 1125). The exported `Cluster` surface stays BYTE-STABLE (OPTION-C — the match rides ctx). NO `Cluster.LbPolicy` reject-text change (subset is not an enum value; the `CLUSTER_PROVIDED + lb_subset_config` combination is ALREADY rejected by envoy-go's pre-existing unsupported-policy gate → ZERO new reject arm).

**Tech Stack:** Go 1.26.x; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008 — `Cluster.LbSubsetConfig` + `LbEndpoint.metadata` + `RouteAction.metadata_match` + `core.Metadata` in the pinned module; ZERO new dep, `go mod tidy -diff` empty); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227, @ `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`). Reuses `internal/cluster/` (the 02/34/35/36/37 Manager + the ADR-0232 `loadBalancer` interface + `noopRelease` + the ADR-0235 `Pick(hashKey, hasHash)` seam + `hashKeyFrom`/`WithHashKey` + the five leaf policies), the HTTP route/producer sites (`internal/filter/hcm/{actions.go,config.go}`, `internal/filter/http/router/{router.go,router_h2.go}` — the ADR-0237 `hash_policy` producer template), the `0062`/`0063` differential harness (the cross-side HTTP-route `StatsAsserter` + `DistributionAsserter` + the `HTTPEcho` backend). ZERO new packages, ZERO new go.mod deps.

---

## Source-of-truth references

- **SPEC:** `docs/envoy-go/phases/38-load-balancer-subset/SPEC.md` — AUTHORITATIVE. §1.1 AMEND-SS1..SS7; §3.0 the split disposition (single flat 38.1 leg; 38.2 pre-authorized); §3.1 the `subsetLB` design (the wrapper + enumeration + fallback — indicative code); §3.2 the seam EXTENSION + the `buildLeafLB` extraction + the `metadata_match` producer; §3.3 the `Endpoint.Metadata` dimension; §5 the proto roster; §6 the reject roster (EMPTY new cluster-arm + the ONE producer-side scalar arm); §7 the +4 `lb_subsets_*` stats + the 33× departure; §8.1 the `0064` fixture design; §10 the ~12–15-task spine; §11 the D-SS1..D-SS7 empirical pins; §12 the D-S38-1..D-S38-6 questions; §13 the ADR-0239/0240 §Context drafts.
- **BRAINSTORM:** `docs/envoy-go/phases/38-load-balancer-subset/BRAINSTORM.md` — the charter (Q0/Q1/Q2). NOTE the BRAINSTORM's "number canonicalization (int vs double)" open question is RESOLVED by SPEC AMEND-SS2 → there is NO int/double distinction in the value model (numbers are double-valued; just `float64`); the BRAINSTORM's "lb_subset_config validation set" anticipation is SUPERSEDED by AMEND-SS1 → the reject surface is EMPTY.
- **As-built anchors** (captured at master tip `ed7f8d7`; re-confirm at Task 1 — line numbers shift on the IMPL-session tip):
  - `internal/cluster/subset.go` — **does NOT yet exist** (Task 2 creates it; the `ringhash.go`/`maglev.go` sibling precedent).
  - `internal/cluster/loadbalancer.go:15-22` — the `loadBalancer` interface (`Pick(hashKey uint64, hasHash bool) (Endpoint, func(), error)`:21 — WIDENS at Task 4) + `noopRelease`:26 (REUSED) + `roundRobin.Pick`:39 (WIDENS) + `errNoEndpoints` (REUSED).
  - `internal/cluster/cluster.go:33-36` (`Endpoint{Host, Port}` — gains `Metadata` at Task 3) + `:39` (`func (e Endpoint) Addr()` — UNCHANGED; `Metadata` is NOT part of the dial identity) + `:172-185` (`hashKeyCtxKey`/`WithHashKey`/`hashKeyFrom` — the ctx-key TEMPLATE for `WithSubsetMatch`/`subsetMatchFrom`) + `:196` (`PickEndpoint` → `c.lb.Pick(0, false)`) + `:232-233` (`Dial`: `hk, ok := hashKeyFrom(ctx)` → `c.lb.Pick(hk, ok)`) + `:286-287` (`AcquireH1`: same shape) — the three funnels widen at Task 4.
  - `internal/cluster/manager.go:251-292` (the `lb_policy` switch — EXTRACTED to `buildLeafLB` at Task 5; `default` reject text at `:291` UNTOUCHED) + `:517-540` (`extractEndpoints` — the `Endpoint` construction site at `:533`; gains the `envoy.lb` capture at Task 3) + `:99-126` (`registerClusterMetrics` — the `*ringHashLB`:110 / `*maglevLB`:118 gauge type-assert blocks; the `*subsetLB` type-assert lands at Task 9) + `:363` (`parseRingHashLbConfig`) + `:407` (`parseMaglevLbConfig`) — the `parseLbSubsetConfig` precedents.
  - `internal/cluster/ringhash.go:60` (`var _ loadBalancer = (*ringHashLB)(nil)`) / `maglev.go:33` (`var _ loadBalancer = (*maglevLB)(nil)`) — the interface-assert pattern `subsetLB` mirrors.
  - The 5 leaf `Pick` signatures (all WIDEN at Task 4): `roundRobin` (`loadbalancer.go:39` — `(_ uint64, _ bool)`), `leastRequest` (`leastrequest.go:81` — `(_ uint64, _ bool)`), `randomLB` (`random.go:37` — `(_ uint64, _ bool)`), `ringHashLB` (`ringhash.go:129` — `(hashKey uint64, hasHash bool)`), `maglevLB` (`maglev.go:39` — `(hashKey uint64, hasHash bool)`).
  - `internal/filter/hcm/actions.go:201-204` (`clusterRouteAction{cluster, hashPolicies}` — gains `subsetMatch` at Task 10).
  - `internal/filter/hcm/config.go:536-556` (`buildRouterAction` — builds `clusterRouteAction`, calls `parseRouteHashPolicies`:551; the non-`Cluster` ClusterSpecifier reject at `:540`) + `:566` (`parseRouteHashPolicies` — the `parseRouteSubsetMatch` precedent).
  - `internal/filter/http/router/router.go:514` (`applyHashKey` → `cluster.WithHashKey`:544 — the producer TEMPLATE) + `:558` (`H1ClusterAction(c, hps)`) + `:588` (`doH1ClusterAction` — applies the hash key at `:597`, `AcquireH1` at `:602`); `internal/filter/http/router/router_h2.go:39` (`H2ClusterAction(c, hps)`).
  - `internal/stats/registry.go:79` (`NewCounter(name) *Counter`) + `:95` (`NewGauge(name) *Gauge`) — REUSED for the 4 `lb_subsets_*` stats (confirm the `*Counter` method set — `Inc`/`Add` — at Task 1).
- **Differential harness / the `0064` template:** `test/fixtures/0063-lb-maglev/driver/driver.go` (the closest template — `clusterName = "c_echo"`, `refContainerListenerPort = 19152`, the `hashValues`/`repeatPerVal`/`totalReqs`/`healthReqs` constants, `AssertDistribution`:307, `AssertStats`:343, the reference STRICT_DNS/`host.docker.internal` vs subject STATIC/`127.0.0.1` bootstrap split, the `HTTPEcho` backend) + `test/differential/fixture/fixture.go` (the `Driver`/`DistributionAsserter`:57/`StatsAsserter`:75 interfaces; `BackendKind` tail `TCPThriftResponder = 33`:562).

## Project conventions honored throughout (memory + ADRs)

- `feedback_execution_style` — subagent-driven execution (the IMPL runs subagent-per-task).
- `feedback_git_worktrees` — this PLAN was authored in worktree `.worktrees/phase-38-plan`; the IMPL runs in its own worktree.
- `feedback_subagents_no_push` — **subagents commit LOCAL-ONLY**; the controller squash-merges + pushes at stage-close.
- `feedback_pertask_gofmt_lint` — **every task** runs `gofmt -l` + `golangci-lint run` on the touched packages (not just `go vet`).
- `feedback_subagent_worktree_path_targeting` / `_detach` — all paths below are repo-root-relative; the IMPL worktree is the canonical checkout; the controller verifies the main checkout stays clean and re-verifies the branch each task (deliberate-break tasks can detach HEAD — restore, never checkout-sha/amend). PROGRESS.md is at the pinned canonical path `docs/envoy-go/phases/38-load-balancer-subset/PROGRESS.md`.
- `reference_conn_wrap_method_no_promote` (generalized) — the NEW `Endpoint.Metadata` field threads through EVERY `Endpoint` construction site (`extractEndpoints` + every test builder), not just the type definition (Task 3 enumerates the sites).
- `reference_differential_break_protocol_count1` — every new differential assertion is proven live by a deliberate-break with `-count=1` (go test caching serves a stale PASS otherwise).
- `reference_differential_asserter_dispatch` — the stats prong uses `StatsAsserter` (cross-side path); the affinity/spread prong uses `DistributionAsserter` (driver-side, runs on both paths). Name each break PRECISELY to avoid a vacuous assertion (Task 12).
- `reference_differential_run_selector` — targeted runs use `-run 'TestDifferential/0064'`, NEVER `-run '0064'` (which matches zero subtests → vacuous green).
- `reference_differential_hash_key_cross_side_infeasible` — INVERTED here: the `metadata_match` is STATIC config (identical on both sides) → NAT-transparent → the subset-affinity SET-membership property holds TRUE cross-side (against each side's OWN known version→idx map). The host-identity obstacle that constrained maglev/ring_hash does NOT bite.
- `reference_fixture_workload_constant_desync` — N/K/totalReqs MUST stay synced with any hand-rolled count slices; go-test caching masks a desync until `-count=1` (Task 11/12 — DERIVE `totalReqs` from the named constants, never a literal).
- `reference_docker_probe_bridge_network` — the `0064` differential needs Docker + the contrib reference image on a bridge network; the controller runs that gate where Docker is present (verify the decode ran: `downstream_cx_rx_bytes_total > 0`).
- ADR-0239 (the subset-match seam-input + the `RouteAction.metadata_match` producer — the seam-BUILD half; §Context DRAFT in SPEC §13) + ADR-0240 (the subset LB policy + the `Endpoint.Metadata` dimension — the policy half; §Context DRAFT in SPEC §13), ADR-0235 (the LB hash-key seam — the seam-input STYLE TEMPLATE), ADR-0237 (the HTTP route producer — the `metadata_match` producer STYLE TEMPLATE), ADR-0232 (the LB acquire/release seam — OPTION-C + `noopRelease`), ADR-0024 (per-cluster LB-state scope — NOTED at SPEC §13: the FIRST LB to Inc stats from `Pick`; the ADR-0240 note records the wrinkle, NOT an amendment), ADR-0044 (ADR §Context at SPEC, body at IMPL, in-place), ADR-0052 (the atomic-landing six-gate), ADR-0080 (byte-stable reject text — the ONE producer-side `router-metadata-match-nonscalar` arm), ADR-0106 (flat family row — NO parent rollup), ADR-0045 (the split-gate — FINAL re-check below), ADR-0004 (the empirical-pin discipline — DONE at SPEC §11), ADR-0227 (the contrib reference image).

## D-question resolutions (SPEC §12)

- **D-S38-1 (file placement):** RESOLVED as anticipated. The `subsetLB` type + the `SubsetMatch`/`SubsetValue` value types + `valueEqual` + `ScalarsFromStruct` + the enumeration/resolution/fallback → NEW `internal/cluster/subset.go` (the `ringhash.go`/`maglev.go` sibling precedent). The `buildLeafLB` extraction + the wrap-after-switch + `parseLbSubsetConfig` + the 4 stat registrations → `manager.go`. The `Endpoint.Metadata` field + `WithSubsetMatch`/`subsetMatchFrom` + the funnel widening → `cluster.go`. The producer (`parseRouteSubsetMatch` + the `clusterRouteAction.subsetMatch` field + the H1/H2 `ClusterAction` constructors + the dispatch thread) → `hcm/config.go` + `hcm/actions.go` + `router/router.go` + `router/router_h2.go`. ZERO new packages.
- **D-S38-2 (the `router-metadata-match-nonscalar` reject wording + the endpoint-side disposition):** RESOLVED — the house-prefixed `router: metadata_match envoy.lb key %q: only scalar values (string, number, bool) are supported` (NO fixture pins it; IMPL-finalizable within the ADR-0080 / §6.2 principle — the `parseRouteHashPolicies` house-prefix precedent). The lowering is shared via `cluster.ScalarsFromStruct(*structpb.Struct) (map[string]SubsetValue, []string)` returning the scalar map + the list of non-scalar keys; the producer REJECTS on a non-empty non-scalar-key list (`router:` message); `extractEndpoints` DROPS them silently (a non-scalar endpoint value can never match a scalar-only route match). `structpb` null/unset → treated as non-scalar (rejected on the route side, dropped on the endpoint side).
- **D-S38-3 (the empty-`keys`-selector + empty-`subset_selectors` disposition):** RESOLVED — ACCEPT (parity — AMEND-SS1). An empty-`keys` selector → a single subset keyed by the empty tuple over ALL endpoints (every host carries "all zero keys"); it is matched only by a route whose `metadata_match` is also empty (which the producer renders as `hasMatch == false` → the fallback path, NOT the empty subset) → in practice a benign no-op subset that inflates the distinct-subset count by ≤1. Empty `subset_selectors: []` → no subsets built; every request takes the fallback path. Both are unit-tested against the live-accepted shape (Task 6/8).
- **D-S38-4 (the `lb_subsets_selected`/`fallback` counter INJECTION mechanics):** RESOLVED — the build-before-register ordering is sound: `buildCluster` builds `cl.lb` (the `subsetLB`, with `numSubsets` computed and `selected`/`fallbackC` nil), THEN `NewManager` calls `registerClusterMetrics(r, c)` which type-asserts `*subsetLB`, allocates the 4 counters/gauge, ASSIGNS the `selected`/`fallbackC` pointers onto the live `subsetLB`, and Sets `active`/`created` to `numSubsets` (×1). The counters are Inc'd from `subsetLB.Pick` thereafter. ADR-0024's per-cluster scope HOLDS (the counters are per-cluster); the new "an LB Inc's its own stats from `Pick`" wrinkle is RECORDED in ADR-0240's §Consequences — NOT a separate ADR-0024 amendment.
- **D-S38-5 (the `0064` final constants + the break protocol):** RESOLVED — 4 endpoints (`version: v1` ×2, `version: v2` ×2) / 2 version subsets / `fallback_policy: ANY_ENDPOINT` / routes `/v1`,`/v2`,`/none`,`/health` / K=16 per route × 3 routed routes + 8 `/health`; `totalReqs` DERIVED. The deliberate-break protocol: drop-the-`metadata_match` (affinity), misroute-the-fallback (fallback + stats), stats-prong-drop — all with `-count=1`; ≥20-run flake check; the `reference_fixture_workload_constant_desync` guard (Tasks 11–12).
- **D-S38-6 (standalone subset property test vs folded):** RESOLVED — FOLDED into Task 6/7's `subset_test.go` as a deterministic property test (random endpoint-metadata sets + selectors → every endpoint lands in exactly the subsets its value-tuple matches; the resolver never panics; the fallback fires per policy; value-equality is double-valued for numbers [int 1 ≡ 1.0]). The `maglev`/`ringhash` "RandomKeysNeverPanicAlwaysValid" folded-property precedent. NOT a `Fuzz*` corpus entry (no untrusted wire input — the metadata derives from validated xDS config; fuzzers STAY 42).
- **ADR-0045 split-gate FINAL re-check:** **NO FURTHER SPLIT within 38.1.** This PLAN decomposes into **14 tasks** (≤ ~25) over **~300–450 production LoC** (`subset.go` ~180–250 + `cluster.go` `Endpoint.Metadata`/`WithSubsetMatch`/funnels ~35 + `manager.go` `buildLeafLB`/wrap/`parseLbSubsetConfig`/`extractEndpoints` capture/4 registrations ~70 + the 5 leaf signature widenings ~15 + the producer ~50; ≤ ~1500). BOTH ADR-0045 axes hold. The seam widening + the leaf-factory extraction + the wrapper + the producer are ONE self-contained whole on the HTTP `metadata_match` plane — NO second subsystem to peel off (38.2's weighted-cluster routing is the pre-authorized peel, deferred to its own SPEC/PLAN/IMPL per §3.0).

### Decomposition note (14 tasks vs the SPEC's indicative ~12–15)

SPEC §10 lists 14 indicative spine entries (Task 13 OPTIONAL property test; Task 14 bundles verify + completion). This PLAN: (a) FOLDS the property test into Tasks 6/7's `subset_test.go` (D-S38-6 — the maglev/ringhash precedent; no standalone task), and (b) SPLITS the SPEC's Task 14 into **Task 13 (verification gate)** and **Task 14 (completion bundle)** — different kinds of work (running gates vs. authoring ADR-0052 docs; the phase-35/36/37 precedent). All other SPEC §10 tasks map 1:1.

| SPEC §10 task | This plan |
|---|---|
| 1 baselines/anchors gate + PROGRESS | Task 1 |
| 2 `SubsetMatch`/`subsetValue`/`valueEqual`/`key()` | Task 2 |
| 3 `Endpoint.Metadata` + `extractEndpoints` capture | Task 3 |
| 4 seam-input widening (interface + 5 leaves + `WithSubsetMatch`/funnels) | Task 4 |
| 5 `buildLeafLB` extraction | Task 5 |
| 6 `subsetLB` enumeration + build | Task 6 (+ folded enumeration property test) |
| 7 `subsetLB.Pick` (match/fallback/NO_FALLBACK) | Task 7 (+ folded resolution property test, D-S38-6) |
| 8 manager `parseLbSubsetConfig` + wrap-after-switch | Task 8 |
| 9 the 4 `lb_subsets_*` registrations | Task 9 |
| 10 the HTTP `metadata_match` producer | Task 10 |
| 11 the `0064-lb-subset` fixture | Task 11 |
| 12 deliberate-break liveness + flake | Task 12 |
| 13 (optional) property test | FOLDED into Tasks 6/7 |
| 14 full differential re-verify | **Task 13** |
| (14, cont.) completion bundle | **Task 14 (split out)** |

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `internal/cluster/subset.go` | **CREATE** (Tasks 2, 6, 7) | `SubsetValue` (scalar union: string/number/bool) + `valueEqual` + `SubsetMatch` (sorted proto-free pairs) + `NewSubsetMatch` + `key()` canonical string + `ScalarsFromStruct` (the shared structpb→scalar lowering — Task 2); `subsetLB` + `newSubsetLB` (enumeration: per selector × value-tuple, a child per subset via the factory; the fallback child — Task 6); `subsetLB.Pick` (match resolution → child delegate → fallback; `selected`/`fallbackC` Inc — Task 7) + the `var _ loadBalancer = (*subsetLB)(nil)` assert. |
| `internal/cluster/subset_test.go` | **CREATE** (Tasks 2, 6, 7) | `valueEqual`/`key()` vectors (int 1 ≡ 1.0; bool; string; sorted-key canonicalization — Task 2); the enumeration tests (subset membership / multi-key / empty-keys / default-match) + the folded enumeration property test (Task 6); the `Pick` tests (affinity / all-5-children delegate / the 3 fallback outcomes / `selected`/`fallbackC` Inc) + the folded resolution property test (Task 7). |
| `internal/cluster/cluster.go` | MODIFY (Tasks 3, 4) | The `Endpoint.Metadata` field (Task 3); `WithSubsetMatch`/`subsetMatchFrom` (the `hashKeyCtxKey` template) + the `Dial`/`AcquireH1`/`PickEndpoint` funnel widening (Task 4). |
| `internal/cluster/loadbalancer.go` + `leastrequest.go` + `random.go` + `ringhash.go` + `maglev.go` | MODIFY (Task 4) | The `loadBalancer.Pick` interface widening + the 5 leaf `Pick` signature widenings (behavior-neutral `_ SubsetMatch, _ bool`). |
| `internal/cluster/manager.go` | MODIFY (Tasks 3, 5, 8, 9) | The `extractEndpoints` `envoy.lb` capture (Task 3); the `buildLeafLB` extraction (Task 5); `parseLbSubsetConfig` + the wrap-after-switch (Task 8); the 4 `lb_subsets_*` registrations + the `*subsetLB` injection in `registerClusterMetrics` (Task 9). |
| `internal/cluster/manager_test.go` | MODIFY (Tasks 3, 5, 8, 9) | The endpoint-metadata capture tests (Task 3); the `buildLeafLB`-behavior-stable tests (Task 5); the §6 accept/reject matrix (Task 8); the registration + injection assertions (Task 9). |
| `internal/filter/hcm/actions.go` | MODIFY (Task 10) | The `clusterRouteAction.subsetMatch` field. |
| `internal/filter/hcm/config.go` | MODIFY (Task 10) | `parseRouteSubsetMatch` (the scalar lower + the non-scalar DEPARTURE-reject) + the `buildRouterAction` wiring. |
| `internal/filter/http/router/router.go` + `router_h2.go` | MODIFY (Task 10) | `H1ClusterAction`/`H2ClusterAction` widen to `(c, hps, subsetMatch)`; `doH1ClusterAction`/`-Direct`/H2 thread `WithSubsetMatch`. |
| `internal/filter/hcm/config_test.go` (+ router tests) | MODIFY (Task 10) | The `parseRouteSubsetMatch` parse + non-scalar-reject + route-static thread tests. |
| `test/fixtures/0064-lb-subset/` | **CREATE** (Task 11) | `driver/driver.go`, `driver/driver_test.go`, `README.md`, `expectations.yaml` (mirroring the `0063` dir layout). |
| `docs/envoy-go/phases/38-load-balancer-subset/PROGRESS.md` | **CREATE** (Task 1) | The IMPL progress ledger. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY (Task 14) | The `subset` subsection + the 4 `lb_subsets_*` stats + the departure/coverage records + the stat-surface doc count 1121 → 1125. |
| `docs/envoy-go/DECISIONS.md` | MODIFY (Task 14) | The full ADR-0239 + ADR-0240 entries (§Context + §Decision + §Consequences; ADR-0044 in-place; tail → ADR-0240). |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | MODIFY (Task 14) | Active-phase + counts advance (fixtures 65 → 66; stat surface 1121 → 1125; DECISIONS tail → ADR-0240); ROADMAP row 38 (38.1 leg) `in-progress → done`. |

---

## Task 1: First-task baselines/anchors gate + PROGRESS.md

**Goal:** Re-confirm every count anchor against the IMPL-session tip BEFORE touching code (the established first-task discipline), re-pin the as-built line anchors, and create the progress ledger. No production code.

**Files:**
- Create: `docs/envoy-go/phases/38-load-balancer-subset/PROGRESS.md`

- [ ] **Step 1: Confirm the count anchors via the canonical recipes**

Run (from repo root):
```bash
ls -d test/fixtures/[0-9]* | wc -l                       # expect 65
ls -d test/fixtures/[0-9]* | tail -1                     # expect test/fixtures/0063-lb-maglev
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1   # expect TCPThriftResponder BackendKind = 33
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l        # expect 42
grep "^## ADR-0" docs/envoy-go/DECISIONS.md | tail -1    # expect tail ADR-0238, next-free ADR-0239
grep -n "1121" docs/envoy-go/BEHAVIOR_CONTRACT.md        # the stat-surface DOC count (no programmatic golden)
go build ./... && echo BUILD_OK
go mod tidy -diff && echo TIDY_EMPTY                     # expect exit 0, empty (ZERO new dep — AMEND-SS1)
```
Expected: fixtures **65** (tail `0063-lb-maglev`), BackendKind tail **33**, fuzzers **42**, stat surface **1121** (a DOC count in BEHAVIOR_CONTRACT.md, NOT a programmatic test — the phase-35/36/37 PROGRESS note), DECISIONS tail **ADR-0238** (the ADR-0239 + ADR-0240 §Context are DRAFTs in SPEC §13 — NOT yet in DECISIONS.md), `go mod tidy -diff` empty.

- [ ] **Step 2: Re-pin the as-built anchors against the IMPL-session tip**

Confirm these line anchors still hold (they shift if the tip moved); record actual line numbers in PROGRESS.md:
```bash
grep -n "Pick(hashKey uint64, hasHash bool)\|noopRelease = func\|errNoEndpoints" internal/cluster/loadbalancer.go   # the seam to widen (Task 4)
grep -n "type Endpoint struct\|func (e Endpoint) Addr\|type hashKeyCtxKey\|func WithHashKey\|func hashKeyFrom" internal/cluster/cluster.go   # the Metadata field + the ctx-key template
grep -n "c.lb.Pick(" internal/cluster/cluster.go                    # the THREE funnels to widen (Dial/AcquireH1/PickEndpoint)
grep -n "switch c.GetLbPolicy()\|case clusterv3.Cluster_\|unsupported lb_policy" internal/cluster/manager.go   # the switch to EXTRACT (Task 5)
grep -n "func extractEndpoints\|Endpoint{Host:" internal/cluster/manager.go    # the Endpoint construction site (Task 3)
grep -n "func registerClusterMetrics\|(\*ringHashLB); ok\|(\*maglevLB); ok" internal/cluster/manager.go   # the gauge-register site + the type-assert precedent (Task 9)
grep -n "func parseRingHashLbConfig\|func parseMaglevLbConfig" internal/cluster/manager.go   # the parse precedent (Task 8)
grep -n "func (.*) Pick(" internal/cluster/loadbalancer.go internal/cluster/leastrequest.go internal/cluster/random.go internal/cluster/ringhash.go internal/cluster/maglev.go   # the 5 leaf Pick sites (Task 4)
grep -n "type clusterRouteAction struct" internal/filter/hcm/actions.go        # the subsetMatch field site (Task 10)
grep -n "func buildRouterAction\|func parseRouteHashPolicies\|RouteAction_Cluster" internal/filter/hcm/config.go   # the producer site (Task 10)
grep -n "func applyHashKey\|func H1ClusterAction\|func doH1ClusterAction\|type routerAction struct\|WithHashKey" internal/filter/http/router/router.go   # the producer template + the dispatch-state struct (Task 10)
grep -n "func H2ClusterAction\|func doH2ClusterAction\|type routerActionH2 struct" internal/filter/http/router/router_h2.go   # the H2 constructor + dispatch + struct (Task 10)
grep -n "ClusterAction(" internal/filter/hcm/actions.go                        # the THREE call sites to widen (Task 10 — expect :213/:236/:249); confirm NO doH1ClusterActionDirect exists anywhere
grep -n "func (r \*Registry) NewCounter\|func (r \*Registry) NewGauge\|func (c \*Counter) Inc\|func (c \*Counter) Add" internal/stats/registry.go   # the stat API (Task 9) — CONFIRM Counter has Inc AND Add (created counter += numSubsets at build)
test -f internal/cluster/subset.go && echo "WARN subset.go exists" || echo "subset.go ABSENT (expected — Task 2 creates it)"
```

- [ ] **Step 3: Confirm the reject surface is EMPTY (AMEND-SS1) + the CLUSTER_PROVIDED pre-coverage**

```bash
grep -rln "unsupported lb_policy" internal/ cmd/                          # expect ONLY internal/cluster/manager.go (UNTOUCHED this phase)
grep -n "ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV" internal/cluster/manager.go   # the supported-list — STAYS UNTOUCHED (subset is not an enum value)
grep -n "func TestManager_Error_UnsupportedLBPolicy" internal/cluster/manager_test.go   # STAYS UNTOUCHED (no doubly-hit retarget — AMEND-SS1)
grep -rln "cluster_specifier.*not supported\|literal cluster name only" internal/filter/hcm/   # the weighted_clusters reject (38.2 territory — UNTOUCHED here)
```
Expected: the production reject string ONLY in `manager.go` and NOT edited this phase; `TestManager_Error_UnsupportedLBPolicy` UNTOUCHED. **Record:** 38.1 adds ZERO new cluster-config reject arm (the only reference reject `CLUSTER_PROVIDED + lb_subset_config` is pre-covered by the unsupported-policy gate); the ONE new arm is the producer-side `router-metadata-match-nonscalar` (unit-level, Task 10).

- [ ] **Step 4: Create PROGRESS.md**

Create `docs/envoy-go/phases/38-load-balancer-subset/PROGRESS.md` with: the 14-task table (status column), the count anchors from Step 1, the as-built line anchors from Step 2, the EMPTY-reject-surface confirmation from Step 3, the D-S38-1..6 resolutions, and the ADR-0045 re-check verdict (NO FURTHER SPLIT; ~300–450 LoC / 14 tasks). Mark Task 1 complete.

- [ ] **Step 5: Commit (LOCAL-ONLY)**

```bash
git add docs/envoy-go/phases/38-load-balancer-subset/PROGRESS.md
git commit -m "phase 38.1 Task 1: baselines gate + PROGRESS.md (fixtures 65 / fuzzers 42 / stat surface 1121 / BackendKind 33 / DECISIONS tail ADR-0238 confirmed; reject surface EMPTY — supported-list + TestManager_Error_UnsupportedLBPolicy UNTOUCHED; go mod tidy -diff empty)"
```

---

## Task 2: `SubsetMatch` / `SubsetValue` / `valueEqual` / `key()` + `ScalarsFromStruct` (`subset.go`)

**Goal:** Create `internal/cluster/subset.go` with the proto-free value types — `SubsetValue` (the scalar union: string / float64 / bool, mirroring the scalar subset of `ProtobufWkt::Value`), `valueEqual` (numbers double-valued — int 1 ≡ 1.0; bool/string typed — mirroring `ValueUtil::equal`, AMEND-SS2), `SubsetMatch` (a sorted proto-free `[]subsetKV`), `NewSubsetMatch` (sorts + canonicalizes), `key()` (the canonical lookup string), and the shared `ScalarsFromStruct` lowering (the structpb→scalar bridge used by BOTH the producer [reject non-scalar] and `extractEndpoints` [drop non-scalar]). TDD-first; NO `subsetLB` yet (Tasks 6–7).

**Files:**
- Create: `internal/cluster/subset.go`
- Create: `internal/cluster/subset_test.go`

- [ ] **Step 1: Write the failing tests** (`subset_test.go`)

```go
package cluster

import (
	"testing"

	"google.golang.org/protobuf/types/known/structpb"
)

func TestValueEqual_NumbersAreDoubleValued(t *testing.T) {
	// AMEND-SS2: int 1 ≡ double 1.0 (ProtobufWkt::Value has a single numeric kind).
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
	// Cross-kind never equal; same-kind by value.
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
	// The canonical key is sort-by-key independent of insertion order; equal
	// matches → equal key; numbers fold int/double.
	a := NewSubsetMatch(map[string]SubsetValue{
		"stage":   {Kind: subsetString, Str: "prod"},
		"version": {Kind: subsetNumber, Num: 1},
	})
	b := NewSubsetMatch(map[string]SubsetValue{
		"version": {Kind: subsetNumber, Num: 1.0}, // 1.0 ≡ 1
		"stage":   {Kind: subsetString, Str: "prod"},
	})
	if a.key() != b.key() {
		t.Errorf("canonical keys differ under reordering / int-vs-double: %q vs %q", a.key(), b.key())
	}
	c := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})
	if a.key() == c.key() {
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
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestValueEqual|TestSubsetMatch|TestScalarsFromStruct' ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`SubsetValue`/`valueEqual`/`SubsetMatch`/`NewSubsetMatch`/`ScalarsFromStruct` undefined).

- [ ] **Step 3: Implement the value types in `subset.go`**

```go
package cluster

import (
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/structpb"
)

// subsetValueKind tags the SubsetValue scalar union. The MVP supports only the
// scalar ProtobufWkt::Value kinds (string, number, bool); list/struct/null are
// not represented (the producer rejects them, extractEndpoints drops them —
// SPEC §6.2 / AMEND-SS2).
type subsetValueKind uint8

const (
	subsetString subsetValueKind = iota
	subsetNumber
	subsetBool
)

// SubsetValue is a proto-free scalar metadata value mirroring the scalar subset
// of ProtobufWkt::Value. Numbers are double-valued (a single numeric kind — int
// 1 ≡ double 1.0; there is no int/double distinction to canonicalize beyond
// float64 — AMEND-SS2).
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
// resolves endpoints/subsets against it via an EXACT key-set + value match
// (AMEND-SS2). The zero value is the empty (no-match) match.
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

// empty reports whether the match carries no keys (a route with no metadata_match).
func (m SubsetMatch) empty() bool { return len(m.kvs) == 0 }

// key is the canonical lookup string (the subset map key). The value-equality
// semantics (numbers double-valued) are preserved by formatting numbers via
// strconv.FormatFloat (1 and 1.0 both → "1"). Keys are kind-tagged so a string
// "1" never collides with the number 1.
func (m SubsetMatch) key() string {
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

// ScalarsFromStruct lowers a structpb.Struct (an envoy.lb metadata namespace) to
// the scalar SubsetValue map, returning the scalar entries plus the list of keys
// whose values were NON-scalar (list / struct / null / unset). Callers decide
// the non-scalar disposition: the route producer REJECTS (router: ... message —
// SPEC §6.2); extractEndpoints DROPS silently (a non-scalar endpoint value can
// never match a scalar-only route match). nil struct → empty/empty.
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
		sort.Strings(nonScalar) // deterministic for a stable reject message
	}
	return scalars, nonScalar
}
```
> Imports: `sort`, `strconv`, `strings` (stdlib) + `google.golang.org/protobuf/types/known/structpb` (transitively present — `go mod tidy -diff` stays empty; confirm in Step 4). NO new go.mod dep.

- [ ] **Step 4: Run to verify they pass + tidy stays empty**

```bash
cd internal/cluster && go test -run 'TestValueEqual|TestSubsetMatch|TestScalarsFromStruct' ./... 2>&1 | tail
cd /home/esa/git/envoy-go && go mod tidy -diff && echo TIDY_EMPTY
```
Expected: PASS; `go mod tidy -diff` empty.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/subset.go internal/cluster/subset_test.go
git commit -m "phase 38.1 Task 2: SubsetMatch/SubsetValue/valueEqual/key() + ScalarsFromStruct (subset.go) — proto-free scalar value types; numbers double-valued (int 1 ≡ 1.0, AMEND-SS2); shared structpb→scalar lowering (producer rejects non-scalar / extractEndpoints drops); ZERO new go.mod dep"
```

---

## Task 3: The `Endpoint.Metadata` dimension + the `extractEndpoints` `envoy.lb` capture

**Goal:** `Endpoint` grows a `Metadata map[string]SubsetValue` field (the parsed `envoy.lb` scalar map — the FIRST per-endpoint dimension beyond `Host:Port`); `extractEndpoints` captures `lbe.GetMetadata().GetFilterMetadata()["envoy.lb"]` via `ScalarsFromStruct` (DROPPING non-scalar values). `Addr()` is UNCHANGED (`Metadata` is NOT part of the dial identity — ring_hash/maglev table keys stay `"IP:PORT"`). Per `reference_conn_wrap_method_no_promote` (generalized): sweep EVERY `Endpoint` construction site.

**Files:**
- Modify: `internal/cluster/cluster.go` (the `Endpoint` struct at `:33`)
- Modify: `internal/cluster/manager.go` (`extractEndpoints` at `:517`, the construction at `:533`)
- Modify: `internal/cluster/manager_test.go` (the capture tests)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
func TestExtractEndpoints_CapturesEnvoyLbScalarMetadata(t *testing.T) {
	// A LbEndpoint carrying envoy.lb {version:"v1", weight:7, canary:true} →
	// Endpoint.Metadata with the 3 scalars; a non-scalar value is DROPPED.
	md, _ := structpb.NewStruct(map[string]any{
		"version": "v1", "weight": float64(7), "canary": true,
		"nested": map[string]any{"x": 1}, // non-scalar → dropped
	})
	lbe := mkLbEndpoint("127.0.0.1", 9001)
	lbe.Metadata = &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"envoy.lb": md}}
	c := mkStaticClusterFromLbEndpoints("c_md", lbe)
	eps, err := extractEndpoints(c.GetLoadAssignment(), "c_md")
	if err != nil {
		t.Fatal(err)
	}
	got := eps[0].Metadata
	if got["version"].Str != "v1" || got["weight"].Num != 7 || !got["canary"].Bool {
		t.Errorf("captured metadata wrong: %+v", got)
	}
	if _, ok := got["nested"]; ok {
		t.Error("non-scalar 'nested' must be dropped")
	}
}

func TestExtractEndpoints_NoMetadataIsNilMap(t *testing.T) {
	c := mkStaticCluster("c_plain", mkLbEndpoint("127.0.0.1", 9001))
	eps, err := extractEndpoints(c.GetLoadAssignment(), "c_plain")
	if err != nil {
		t.Fatal(err)
	}
	if eps[0].Metadata != nil {
		t.Errorf("absent envoy.lb metadata → nil map, got %v", eps[0].Metadata)
	}
}

func TestEndpoint_AddrIgnoresMetadata(t *testing.T) {
	// Metadata is NOT part of the dial identity (ring_hash/maglev keys stay IP:PORT).
	a := Endpoint{Host: "127.0.0.1", Port: 9001}
	b := Endpoint{Host: "127.0.0.1", Port: 9001, Metadata: map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}}
	if a.Addr() != b.Addr() {
		t.Errorf("Addr() must ignore Metadata: %q vs %q", a.Addr(), b.Addr())
	}
}
```
> `mkStaticClusterFromLbEndpoints` may need adding if no builder accepts a pre-built `*endpointv3.LbEndpoint` with metadata — check the existing `mkStaticCluster`/`mkLbEndpoint` test helpers first (Task 1 grep) and extend minimally rather than reinventing. `manager_test.go` does NOT currently import `structpb` — ADD the `google.golang.org/protobuf/types/known/structpb` import (and `corev3` if absent) to the test file; the test code uses `structpb.NewStruct`/`corev3.Metadata`.

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestExtractEndpoints|TestEndpoint_AddrIgnoresMetadata' ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`Endpoint.Metadata` field undefined) or test failure.

- [ ] **Step 3: Add the field + the capture**

In `cluster.go` (`Endpoint` at `:33`):
```go
type Endpoint struct {
	Host string
	Port uint32
	// Metadata is the parsed envoy.lb scalar key→value namespace (the subset
	// dimension — ADR-0240). nil when absent. NOT part of the dial identity:
	// Addr() ignores it, so ring_hash/maglev table keys stay "IP:PORT".
	Metadata map[string]SubsetValue
}
```
In `manager.go` `extractEndpoints` (the construction at `:533`):
```go
scalars, _ := ScalarsFromStruct(lbe.GetMetadata().GetFilterMetadata()["envoy.lb"]) // drop non-scalar (AMEND-SS2 / §6.2)
out = append(out, Endpoint{Host: sa.GetAddress(), Port: sa.GetPortValue(), Metadata: scalars})
```
> `ScalarsFromStruct(nil)` returns `(nil, nil)` so the no-metadata path yields a nil `Metadata` map — matching `TestExtractEndpoints_NoMetadataIsNilMap`. The `_` discards the non-scalar key list (endpoint-side drop). Confirm `manager.go` already imports nothing new (`structpb` lives behind the getter chain — no new import in `manager.go`).

- [ ] **Step 4: Run to verify they pass + the full cluster suite is green**

```bash
cd internal/cluster && go test -run 'TestExtractEndpoints|TestEndpoint_AddrIgnoresMetadata' ./... 2>&1 | tail
cd internal/cluster && go test ./... 2>&1 | tail   # the field add must not perturb any existing test
```
Expected: PASS; the existing cluster suite still green (the new field is additive — every existing `Endpoint{...}` literal still compiles with `Metadata` zero/nil).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/cluster.go internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 38.1 Task 3: Endpoint.Metadata dimension + extractEndpoints envoy.lb capture (ADR-0240) — the FIRST per-endpoint dimension beyond Host:Port; scalar capture via ScalarsFromStruct (non-scalar dropped); Addr() unchanged (Metadata not in the dial identity); absent → nil"
```

---

## Task 4: The seam-input widening — `loadBalancer.Pick` + the 5 leaves + `WithSubsetMatch`/`subsetMatchFrom` + the funnels

**Goal:** Widen the LB pick-seam with the SECOND pick-input (the subset match — ADR-0239). `loadBalancer.Pick(hashKey, hasHash)` → `Pick(hashKey, hasHash, match SubsetMatch, hasMatch bool)`. The five leaf policies absorb `(match, hasMatch)` as a behavior-neutral widening (the ADR-0235 precedent). `cluster.go` gains `WithSubsetMatch`/`subsetMatchFrom` (the `hashKeyCtxKey` template) and the `Dial`/`AcquireH1`/`PickEndpoint` funnels extract + pass both inputs. The exported `Cluster` surface stays BYTE-STABLE (OPTION-C). This task is a PURE WIDENING — no behavior change (subset still has no consumer until Task 6/7).

**Files:**
- Modify: `internal/cluster/loadbalancer.go` (the interface + `roundRobin.Pick`)
- Modify: `internal/cluster/leastrequest.go`, `random.go`, `ringhash.go`, `maglev.go` (the 4 other leaf `Pick` signatures)
- Modify: `internal/cluster/cluster.go` (`WithSubsetMatch`/`subsetMatchFrom` + the 3 funnels)
- Modify: `internal/cluster/cluster_test.go` / `loadbalancer_test.go` (the ctx-key + behavior-neutral tests)

- [ ] **Step 1: Write the failing tests**

```go
func TestSubsetMatchCtx_RoundTrips(t *testing.T) {
	m := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})
	ctx := WithSubsetMatch(context.Background(), m)
	got, ok := subsetMatchFrom(ctx)
	if !ok {
		t.Fatal("subsetMatchFrom must report ok after WithSubsetMatch")
	}
	if got.key() != m.key() {
		t.Errorf("round-trip mismatch: %q vs %q", got.key(), m.key())
	}
	if _, ok := subsetMatchFrom(context.Background()); ok {
		t.Error("bare ctx must report !ok")
	}
}

func TestLeafPolicies_IgnoreSubsetMatch(t *testing.T) {
	// The widened signature is behavior-neutral for the 5 leaves: passing a match
	// changes nothing (only subsetLB consumes it — Task 7).
	m := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})
	rr := &roundRobin{endpoints: eps(3)}
	a, _, _ := rr.Pick(0, false, SubsetMatch{}, false)
	b, _, _ := rr.Pick(0, false, m, true)
	_ = a
	_ = b // both pick from the full set; no panic, no filtering (RR is stateful — assert no error / valid endpoint)
	ep, _, err := rr.Pick(0, false, m, true)
	if err != nil || ep.Port < 1000 {
		t.Errorf("leaf must ignore the match and return a valid endpoint: ep=%v err=%v", ep, err)
	}
}
```
> Also update EVERY existing `*.Pick(` call site IN TESTS across the cluster package to the new 4-arg form (the compile sweep — the bulk of this task is mechanical). Find them with `grep -rn '\.Pick(' internal/cluster/*_test.go`.

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go build ./... 2>&1 | head
```
Expected: COMPILE FAILURE (the interface/leaf signatures don't yet take 4 args; `WithSubsetMatch`/`subsetMatchFrom` undefined).

- [ ] **Step 3: Widen the interface, the 5 leaves, and add the ctx helpers**

`loadbalancer.go` (the interface at `:21` + `roundRobin.Pick` at `:39`):
```go
type loadBalancer interface {
	// Pick selects an endpoint. hashKey carries a request-derived consistent-hash
	// key when hasHash is true (ring_hash/maglev). match carries the route's
	// resolved metadata subset match when hasMatch is true (subset — ADR-0239);
	// the leaf policies ignore (match, hasMatch), only subsetLB consumes it. The
	// release func is the ADR-0232 RELEASE half (unchanged).
	Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error)
}

func (rr *roundRobin) Pick(_ uint64, _ bool, _ SubsetMatch, _ bool) (Endpoint, func(), error) {
	// ... body unchanged ...
}
```
The 4 other leaves — widen the signature, keep the body. The hash-consuming leaves KEEP their named `hashKey`/`hasHash` and add `_ SubsetMatch, _ bool`:
- `leastrequest.go:81` → `func (lr *leastRequest) Pick(_ uint64, _ bool, _ SubsetMatch, _ bool) (Endpoint, func(), error)`
- `random.go:37` → `func (r *randomLB) Pick(_ uint64, _ bool, _ SubsetMatch, _ bool) (Endpoint, func(), error)`
- `ringhash.go:129` → `func (rh *ringHashLB) Pick(hashKey uint64, hasHash bool, _ SubsetMatch, _ bool) (Endpoint, func(), error)`
- `maglev.go:39` → `func (mg *maglevLB) Pick(hashKey uint64, hasHash bool, _ SubsetMatch, _ bool) (Endpoint, func(), error)`

`cluster.go` (after the `hashKeyCtxKey` block at `:172`):
```go
type subsetMatchCtxKey struct{}

// WithSubsetMatch attaches a resolved route subset match to ctx (the producer
// sets it in the HTTP router; the cluster funnels extract it for subsetLB —
// ADR-0239). The exported Cluster surface stays byte-stable (the match rides
// ctx like the hash key — OPTION-C).
func WithSubsetMatch(ctx context.Context, match SubsetMatch) context.Context {
	return context.WithValue(ctx, subsetMatchCtxKey{}, match)
}

func subsetMatchFrom(ctx context.Context) (SubsetMatch, bool) {
	v, ok := ctx.Value(subsetMatchCtxKey{}).(SubsetMatch)
	return v, ok
}
```
The 3 funnels — `Dial` (`:232-233`), `AcquireH1` (`:286-287`):
```go
hk, ok := hashKeyFrom(ctx)
match, hasMatch := subsetMatchFrom(ctx)
ep, release, err := c.lb.Pick(hk, ok, match, hasMatch)
```
`PickEndpoint` (`:196` — no ctx available, the direct-pick path):
```go
ep, release, err := c.lb.Pick(0, false, SubsetMatch{}, false)
```

- [ ] **Step 4: Run to verify they pass (the whole package builds + green)**

```bash
cd internal/cluster && go build ./... 2>&1 | tail
cd internal/cluster && go test -race ./... 2>&1 | tail   # every leaf + every funnel still behaves identically
cd /home/esa/git/envoy-go && go build ./... 2>&1 | tail   # the whole repo (the interface is package-private; no external caller)
```
Expected: PASS, no race; the whole repo builds (the widening is package-internal — the exported `Cluster.Dial`/`AcquireH1`/`PickEndpoint` signatures are UNCHANGED, so `internal/filter/...` is untouched this task).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/
git commit -m "phase 38.1 Task 4: widen the LB pick-seam with the subset match (ADR-0239) — loadBalancer.Pick gains (match SubsetMatch, hasMatch bool); the 5 leaves absorb it behavior-neutrally (ADR-0235 precedent); WithSubsetMatch/subsetMatchFrom (hashKeyCtxKey template) + the Dial/AcquireH1/PickEndpoint funnels extract both inputs; exported Cluster byte-stable (OPTION-C)"
```

---

## Task 5: The `buildLeafLB` extraction (`manager.go`)

**Goal:** EXTRACT the existing `buildCluster` `lb_policy` switch (`manager.go:251-292`) into a `buildLeafLB(c *clusterv3.Cluster, name string, endpoints []Endpoint) (loadBalancer, error)` factory — the SAME switch, parameterized on an arbitrary endpoint sub-slice — so `subsetLB` can build a child per subset (Task 6). `buildCluster` calls `buildLeafLB(c, name, endpoints)` for the cluster-level child; behavior is byte-stable for every existing policy and `CLUSTER_PROVIDED`/unsupported still reject HERE (before any wrap — AMEND-SS1). NO `subsetLB` wiring yet (Task 8). PURE refactor.

**Files:**
- Modify: `internal/cluster/manager.go` (extract the switch; `buildCluster` calls the factory)
- Modify: `internal/cluster/manager_test.go` (a direct `buildLeafLB` test + the reject-still-fires test)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
func TestBuildLeafLB_BuildsEachPolicyOverSubSlice(t *testing.T) {
	eps := []Endpoint{{Host: "127.0.0.1", Port: 9001}, {Host: "127.0.0.1", Port: 9002}}
	for _, pol := range []clusterv3.Cluster_LbPolicy{
		clusterv3.Cluster_ROUND_ROBIN, clusterv3.Cluster_LEAST_REQUEST,
		clusterv3.Cluster_RANDOM, clusterv3.Cluster_RING_HASH, clusterv3.Cluster_MAGLEV,
	} {
		c := &clusterv3.Cluster{Name: "c", LbPolicy: pol}
		lb, err := buildLeafLB(c, "c", eps)
		if err != nil {
			t.Errorf("buildLeafLB(%v) over sub-slice: %v", pol, err)
			continue
		}
		ep, _, err := lb.Pick(123, true, SubsetMatch{}, false)
		if err != nil || ep.Port < 9001 {
			t.Errorf("buildLeafLB(%v).Pick: ep=%v err=%v", pol, ep, err)
		}
	}
}

func TestBuildLeafLB_RejectsClusterProvided(t *testing.T) {
	// AMEND-SS1: CLUSTER_PROVIDED (and every unsupported policy) rejects in the
	// factory — BEFORE any subset wrap. This is why lb_subset_config +
	// CLUSTER_PROVIDED rejects in outcome with ZERO new reject arm.
	c := &clusterv3.Cluster{Name: "c", LbPolicy: clusterv3.Cluster_CLUSTER_PROVIDED}
	_, err := buildLeafLB(c, "c", []Endpoint{{Host: "127.0.0.1", Port: 9001}})
	if err == nil || !strings.Contains(err.Error(), "unsupported lb_policy") {
		t.Errorf("err = %v, want unsupported lb_policy reject", err)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run TestBuildLeafLB ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`buildLeafLB` undefined).

- [ ] **Step 3: Extract the switch**

In `manager.go`, lift the `switch c.GetLbPolicy()` body (`:251-292`) into a new function, returning the built `loadBalancer` instead of assigning `cl.lb`:
```go
// buildLeafLB constructs the leaf load balancer for the cluster's lb_policy over
// the given endpoint set. Extracted from buildCluster so subsetLB can build a
// child per subset over a filtered sub-slice (AMEND-SS5). CLUSTER_PROVIDED and
// every other unsupported policy reject HERE (the default arm) — before any
// subset wrap, so lb_subset_config + CLUSTER_PROVIDED rejects in outcome with
// ZERO new reject arm (AMEND-SS1).
func buildLeafLB(c *clusterv3.Cluster, name string, endpoints []Endpoint) (loadBalancer, error) {
	switch c.GetLbPolicy() {
	case clusterv3.Cluster_ROUND_ROBIN:
		return &roundRobin{endpoints: endpoints}, nil
	case clusterv3.Cluster_LEAST_REQUEST:
		cc, err := parseLeastRequestLbConfig(c, name)
		if err != nil {
			return nil, err
		}
		return newLeastRequest(endpoints, cc)
	case clusterv3.Cluster_RANDOM:
		return newRandom(endpoints)
	case clusterv3.Cluster_RING_HASH:
		cfg, err := parseRingHashLbConfig(c, name)
		if err != nil {
			return nil, err
		}
		return newRingHash(endpoints, cfg)
	case clusterv3.Cluster_MAGLEV:
		cfg, err := parseMaglevLbConfig(c, name)
		if err != nil {
			return nil, err
		}
		return newMaglev(endpoints, cfg)
	default:
		return nil, fmt.Errorf("cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV)", name, c.GetLbPolicy())
	}
}
```
And `buildCluster` replaces the inline switch with:
```go
lb, err := buildLeafLB(c, name, endpoints)
if err != nil {
	return nil, err
}
cl.lb = lb
```
> Preserve the EXACT reject text (`feedback`/ADR-0080 — byte-stable). The `newLeastRequest`/`newRandom`/`newRingHash`/`newMaglev` constructors already return `(loadBalancer-compatible, error)`; returning them directly is equivalent to the prior assign-then-check. Confirm the `roundRobin` arm returns `&roundRobin{...}, nil` (it had no error path).

- [ ] **Step 4: Run to verify they pass + behavior-stable**

```bash
cd internal/cluster && go test -run TestBuildLeafLB ./... 2>&1 | tail
cd internal/cluster && go test ./... 2>&1 | tail   # every existing accept/reject/parse test must stay green (pure refactor)
```
Expected: PASS; the FULL cluster suite green (the extraction is behavior-preserving — every existing `TestManager_Accept_*`/`TestManager_Reject_*`/`TestManager_Error_UnsupportedLBPolicy` passes unchanged).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 38.1 Task 5: extract buildLeafLB(c, name, endpoints) from the buildCluster lb_policy switch (AMEND-SS5) — same switch over an arbitrary sub-slice so subsetLB builds a child per subset; CLUSTER_PROVIDED/unsupported reject HERE before any wrap (ZERO new reject arm); reject text byte-stable; pure refactor, full suite green"
```

---

## Task 6: The `subsetLB` enumeration + build (`subset.go`)

**Goal:** Implement `subsetLB` + `newSubsetLB` — at construction, enumerate subsets (per selector, one subset per distinct value-tuple over the endpoints carrying ALL the selector's keys — AMEND-SS2), building a child `loadBalancer` per subset via an injected `buildLeafLB`-shaped factory; build the cluster-level fallback child {NO_FALLBACK → nil / ANY_ENDPOINT → child over ALL endpoints / DEFAULT_SUBSET → child over the `default_subset` match, nil if zero-match — AMEND-SS3}; compute `numSubsets` (the distinct-subset count, ×1 — AMEND-SS4). The `selected`/`fallbackC` counters stay nil until Task 9. NO `Pick` yet (Task 7). Fold the enumeration property test (D-S38-6).

**Files:**
- Modify: `internal/cluster/subset.go` (the `subsetLB` type + `newSubsetLB` + the parsed-config struct)
- Modify: `internal/cluster/subset_test.go` (the enumeration tests + the folded property test)

- [ ] **Step 1: Write the failing tests** (`subset_test.go`)

```go
// helper: build a subsetLB with a real buildLeafLB-shaped factory (round_robin)
func rrFactory(sub []Endpoint) (loadBalancer, error) { return &roundRobin{endpoints: sub}, nil }

func epMD(host string, port uint32, kv map[string]SubsetValue) Endpoint {
	return Endpoint{Host: host, Port: port, Metadata: kv}
}

func TestSubsetLB_EnumeratesOneSubsetPerValueTuple(t *testing.T) {
	eps := []Endpoint{
		epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}),
		epMD("127.0.0.1", 9002, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}),
		epMD("127.0.0.1", 9003, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v2"}}),
	}
	cfg := lbSubsetCfg{
		fallback:  fallbackAnyEndpoint,
		selectors: [][]string{{"version"}},
	}
	s := newSubsetLB(eps, cfg, rrFactory)
	if s.numSubsets != 2 { // v1, v2
		t.Errorf("numSubsets = %d, want 2", s.numSubsets)
	}
	// the v1 subset has 2 hosts, v2 has 1
	v1 := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})
	if _, ok := s.subsets[v1.key()]; !ok {
		t.Error("missing v1 subset")
	}
}

func TestSubsetLB_HostMissingKeyExcludedFromSelector(t *testing.T) {
	// AMEND-SS2: a host missing any of a selector's keys is excluded from THAT selector.
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
	// NO_FALLBACK → nil fallback
	if newSubsetLB(eps, lbSubsetCfg{fallback: fallbackNoFallback, selectors: sel}, rrFactory).fallback != nil {
		t.Error("NO_FALLBACK → nil fallback child")
	}
	// ANY_ENDPOINT → non-nil fallback over all hosts
	if newSubsetLB(eps, lbSubsetCfg{fallback: fallbackAnyEndpoint, selectors: sel}, rrFactory).fallback == nil {
		t.Error("ANY_ENDPOINT → non-nil fallback child")
	}
	// DEFAULT_SUBSET {version:v1} → non-nil; zero-match default → nil (≡ NO_FALLBACK)
	def := map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}
	if newSubsetLB(eps, lbSubsetCfg{fallback: fallbackDefaultSubset, defaultSubset: def, selectors: sel}, rrFactory).fallback == nil {
		t.Error("DEFAULT_SUBSET with a matching default → non-nil fallback child")
	}
	zero := map[string]SubsetValue{"version": {Kind: subsetString, Str: "v9"}}
	if newSubsetLB(eps, lbSubsetCfg{fallback: fallbackDefaultSubset, defaultSubset: zero, selectors: sel}, rrFactory).fallback != nil {
		t.Error("DEFAULT_SUBSET matching zero hosts → nil fallback (NO_FALLBACK behavior, AMEND-SS3)")
	}
}

func TestSubsetLB_EnumerationProperty(t *testing.T) {
	// D-S38-6 (folded): a deterministic property sweep — every endpoint lands in
	// exactly the subsets its value-tuple matches; build never panics; numSubsets
	// = the count of distinct tuples present. (Deterministic input, not random
	// seed — reproducible.)
	// ... construct ~5 endpoints across 2 selectors with overlapping tuples;
	// assert each subset's member set is exactly the hosts carrying that tuple,
	// and a host appears in multiple subsets across selectors. ...
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run TestSubsetLB ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`subsetLB`/`newSubsetLB`/`lbSubsetCfg`/`fallback*` undefined).

- [ ] **Step 3: Implement the type + enumeration** (SPEC §3.1)

Add to `subset.go`:
```go
// fallbackPolicy mirrors Cluster_LbSubsetConfig_LbSubsetFallbackPolicy.
type fallbackPolicy uint8

const (
	fallbackNoFallback fallbackPolicy = iota // NO_FALLBACK
	fallbackAnyEndpoint                       // ANY_ENDPOINT
	fallbackDefaultSubset                     // DEFAULT_SUBSET
)

// lbSubsetCfg is the parsed, proto-free lb_subset_config (built by
// parseLbSubsetConfig — Task 8). selectors is one []keys per subset_selector.
type lbSubsetCfg struct {
	fallback      fallbackPolicy
	defaultSubset map[string]SubsetValue // the DEFAULT_SUBSET match (scalars; non-scalar dropped)
	selectors     [][]string             // each selector's keys
}

// leafFactory builds a child loadBalancer over an endpoint sub-slice (bound to
// buildLeafLB(c, name, ·) by the manager — AMEND-SS5).
type leafFactory func(sub []Endpoint) (loadBalancer, error)

// subsetLB is a per-cluster metadata-match WRAPPER load balancer mirroring Envoy
// v1.37.2's SubsetLoadBalancer. It partitions endpoints into precomputed subsets
// (one per distinct envoy.lb value-tuple per selector) and delegates each subset
// to a child built from the cluster's lb_policy. Built once at construction
// (immutable → Pick is lock-free). ADR-0240 (the policy + Endpoint.Metadata);
// ADR-0239 (the subset-match pick-input); ADR-0232 (release — delegates the
// child's release).
type subsetLB struct {
	subsets    map[string]loadBalancer // canonical SubsetMatch key → child over that subset
	fallback   loadBalancer            // nil for NO_FALLBACK (or zero-match DEFAULT_SUBSET)
	selected   *stats.Counter          // +1 on a subset hit (injected at register — Task 9)
	fallbackC  *stats.Counter          // +1 on the fallback path (injected at register — Task 9)
	numSubsets int                     // distinct-subset count → active gauge + created counter (×1)
}

var _ loadBalancer = (*subsetLB)(nil)

// newSubsetLB enumerates the subsets and builds a child per subset + the
// fallback child via the factory. A factory error on any child is swallowed for
// a non-buildable subset is not expected here (the factory is the cluster's own
// lb_policy, already validated at the cluster-level buildLeafLB call — Task 8
// surfaces any factory error at construction); see Task 8 for the error path.
func newSubsetLB(endpoints []Endpoint, cfg lbSubsetCfg, factory leafFactory) *subsetLB {
	s := &subsetLB{subsets: map[string]loadBalancer{}}
	for _, keys := range cfg.selectors {
		groups := map[string][]Endpoint{} // canonical match key → member endpoints
		order := map[string]SubsetMatch{}
		for _, ep := range endpoints {
			vals, ok := scalarsForKeys(ep.Metadata, keys)
			if !ok {
				continue // host missing a key → excluded from this selector (AMEND-SS2)
			}
			m := NewSubsetMatch(vals)
			k := m.key()
			groups[k] = append(groups[k], ep)
			order[k] = m
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
		if len(match) > 0 { // zero-match default → nil fallback ≡ NO_FALLBACK (AMEND-SS3)
			if fb, err := factory(match); err == nil {
				s.fallback = fb
			}
		}
	case fallbackNoFallback:
		// s.fallback stays nil
	}
	return s
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
```
> Imports add `"github.com/<module>/internal/stats"` (the `stats.Counter` type — confirm the exact import path from `manager.go`). Empty-`keys` selector → `scalarsForKeys(md, [])` returns an empty-but-ok map for EVERY endpoint → one subset keyed by the empty tuple over all hosts (D-S38-3; benign). Empty `selectors` → no subsets, `numSubsets == 0`.

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run TestSubsetLB ./... 2>&1 | tail
```
Expected: PASS (the `Pick` tests come in Task 7; this task's tests are enumeration/build only).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/subset.go internal/cluster/subset_test.go
git commit -m "phase 38.1 Task 6: subsetLB enumeration + build (ADR-0240, AMEND-SS2/SS3) — per selector × distinct value-tuple a child via the leaf factory; host missing a key excluded; fallback child {NO_FALLBACK nil / ANY_ENDPOINT all / DEFAULT_SUBSET default-match, zero-match → nil}; numSubsets ×1; folded enumeration property test (D-S38-6)"
```

---

## Task 7: `subsetLB.Pick` — match resolution → child delegate → fallback (`subset.go`)

**Goal:** Implement `subsetLB.Pick(hashKey, hasHash, match, hasMatch)`: on `hasMatch` + a subset hit → `selected.Inc()` + delegate `child.Pick(hashKey, hasHash, SubsetMatch{}, false)` (the hashKey passes through so a ring_hash/maglev subset child routes WITHIN its subset); on no-match / a miss → the fallback {`fallback == nil` → `errNoEndpoints` (the existing 503/`UH` path); else `fallbackC.Inc()` + `fallback.Pick(...)`}. `selected`/`fallbackC` are nil-guarded (they're injected at register — Task 9; a unit `subsetLB` built without injection must not panic). Fold the resolution property test (D-S38-6).

**Files:**
- Modify: `internal/cluster/subset.go` (the `Pick` method)
- Modify: `internal/cluster/subset_test.go` (the `Pick` tests + the folded resolution property test)

- [ ] **Step 1: Write the failing tests** (`subset_test.go`)

```go
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
	// a match selecting no subset
	miss := NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v9"}})
	_, release, err := s.Pick(0, false, miss, true)
	if err != errNoEndpoints {
		t.Errorf("NO_FALLBACK miss → errNoEndpoints, got %v", err)
	}
	if release == nil {
		t.Error("release must be non-nil even on error")
	}
	// a route with NO metadata_match (hasMatch false) also → fallback (nil → errNoEndpoints)
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
	s.Pick(0, false, NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}), true) // selected
	s.Pick(0, false, NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v9"}}), true) // fallback
	if sel.Load() != 1 || fbc.Load() != 1 { // Counter.Load() uint64 (internal/stats/counter.go:30 — there is NO .Value())
		t.Errorf("selected/fallback = %d/%d, want 1/1 (mutually exclusive)", sel.Load(), fbc.Load())
	}
}

func TestSubsetLB_PickNilCountersNoPanic(t *testing.T) {
	// A subsetLB built without register-time injection (a unit construction) must
	// not panic on Pick — the counters are nil-guarded.
	eps := []Endpoint{epMD("127.0.0.1", 9001, map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}})}
	s := newSubsetLB(eps, lbSubsetCfg{fallback: fallbackAnyEndpoint, selectors: [][]string{{"version"}}}, rrFactory)
	if _, _, err := s.Pick(0, false, NewSubsetMatch(map[string]SubsetValue{"version": {Kind: subsetString, Str: "v1"}}), true); err != nil {
		t.Fatalf("nil-counter Pick must not panic/err: %v", err)
	}
}
```
> Confirm the `stats.Counter` readback method name (`.Value()` here — verify against the existing `gaugeValue`/counter-read helper used in `manager_test.go`; use whatever the package exposes).

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run TestSubsetLB_Pick ./... 2>&1 | head
```
Expected: COMPILE FAILURE (`Pick` undefined on `*subsetLB`).

- [ ] **Step 3: Implement `Pick`** (SPEC §3.1)

```go
// Pick resolves the ctx-carried subset match to a subset and delegates to its
// child; on a no-match / miss it applies the cluster-level fallback. hashKey/
// hasHash pass straight through to the child (a ring_hash/maglev subset child
// routes WITHIN its subset). selected/fallbackC are Inc'd from here (the FIRST
// LB to touch stats from its own Pick path — injected at register, Task 9;
// nil-guarded for unit constructions). ADR-0240.
func (s *subsetLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) {
	if hasMatch && !match.empty() {
		if child, ok := s.subsets[match.key()]; ok {
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
```

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run TestSubsetLB -race ./... 2>&1 | tail
```
Expected: PASS, no race (the subsets map is immutable post-build; the counters are atomic).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/subset.go internal/cluster/subset_test.go
git commit -m "phase 38.1 Task 7: subsetLB.Pick (ADR-0240, AMEND-SS3) — match→child delegate (hashKey passthrough so a ring_hash/maglev subset child routes within), fallback {nil→errNoEndpoints; else fallbackC.Inc+delegate}, selected.Inc on a hit (mutually exclusive, nil-guarded); folded resolution property test (D-S38-6)"
```

---

## Task 8: Manager `parseLbSubsetConfig` + the wrap-after-switch

**Goal:** `buildCluster` parses `Cluster.GetLbSubsetConfig()` into `lbSubsetCfg` (`parseLbSubsetConfig` — fallback_policy + default_subset [scalar lower, non-scalar drop] + selector keys; the deferred-flag posture) and, when present, WRAPS the leaf child after the switch: `newSubsetLB(endpoints, cfg, func(sub) { return buildLeafLB(c, name, sub) })`. The §6 accept/reject matrix: `CLUSTER_PROVIDED + lb_subset_config` rejects via the leaf factory (BEFORE the wrap — ZERO new arm); empty keys / empty selectors / duplicate selectors / fallback-default mismatches ALL ACCEPT (AMEND-SS1).

**Files:**
- Modify: `internal/cluster/manager.go` (`parseLbSubsetConfig` + the wrap in `buildCluster`)
- Modify: `internal/cluster/manager_test.go` (the §6 accept/reject matrix)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
func TestManager_Accept_LbSubsetConfig_WrapsChild(t *testing.T) {
	c := mkStaticCluster("c_sub", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002))
	c.LbPolicy = clusterv3.Cluster_ROUND_ROBIN
	c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{
		FallbackPolicy:  clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT,
		SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{"version"}}},
	}
	mgr, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err != nil {
		t.Fatalf("lb_subset_config under ROUND_ROBIN must be accepted: %v", err)
	}
	cl, _ := mgr.Get("c_sub")
	if _, ok := cl.lb.(*subsetLB); !ok {
		t.Errorf("lb must be wrapped in *subsetLB, got %T", cl.lb)
	}
}

func TestManager_Reject_LbSubsetConfig_ClusterProvided(t *testing.T) {
	// AMEND-SS1: parity in OUTCOME via the pre-existing unsupported-policy gate
	// (the leaf factory rejects BEFORE the wrap) — ZERO new reject arm.
	c := mkStaticCluster("c_sub", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_CLUSTER_PROVIDED
	c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{FallbackPolicy: clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT}
	_, err := NewManager(mkBootstrap(c), stats.NewRegistry())
	if err == nil || !strings.Contains(err.Error(), "unsupported lb_policy") {
		t.Errorf("err = %v, want the pre-existing unsupported-policy reject", err)
	}
}

func TestManager_Accept_LbSubsetConfig_BenignShapes(t *testing.T) {
	// AMEND-SS1: empty keys, empty selectors, duplicate selectors, and
	// fallback-default mismatches ALL accept (must NOT over-reject).
	base := func() *clusterv3.Cluster {
		c := mkStaticCluster("c_sub", mkLbEndpoint("127.0.0.1", 9001))
		c.LbPolicy = clusterv3.Cluster_ROUND_ROBIN
		return c
	}
	cases := []*clusterv3.Cluster_LbSubsetConfig{
		{SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{}}}}, // empty keys
		{}, // empty subset_selectors + NO_FALLBACK (default)
		{SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{"v"}}, {Keys: []string{"v"}}}}, // dup
		{FallbackPolicy: clusterv3.Cluster_LbSubsetConfig_DEFAULT_SUBSET}, // DEFAULT_SUBSET with no default_subset
	}
	for i, sc := range cases {
		c := base()
		c.LbSubsetConfig = sc
		if _, err := NewManager(mkBootstrap(c), stats.NewRegistry()); err != nil {
			t.Errorf("case %d must accept (parity): %v", i, err)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestManager_.*LbSubsetConfig' ./... 2>&1 | head
```
Expected: COMPILE/test FAILURE (`parseLbSubsetConfig` undefined / no wrap → not a `*subsetLB`).

- [ ] **Step 3: Implement the parse + the wrap-after-switch**

`parseLbSubsetConfig` (in `manager.go` near the other `parse*LbConfig`):
```go
// parseLbSubsetConfig lowers Cluster.LbSubsetConfig to the proto-free lbSubsetCfg
// (AMEND-SS1/SS2). default_subset scalars are lowered (non-scalar dropped); the
// selector keys are copied; the deferred flags (single_host_per_subset,
// per-selector fallback, locality/panic/list_as_any/metadata_fallback) are
// ignored (§2). Returns the zero cfg if sc is nil.
func parseLbSubsetConfig(sc *clusterv3.Cluster_LbSubsetConfig) lbSubsetCfg {
	cfg := lbSubsetCfg{}
	switch sc.GetFallbackPolicy() {
	case clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT:
		cfg.fallback = fallbackAnyEndpoint
	case clusterv3.Cluster_LbSubsetConfig_DEFAULT_SUBSET:
		cfg.fallback = fallbackDefaultSubset
	default: // NO_FALLBACK
		cfg.fallback = fallbackNoFallback
	}
	if ds := sc.GetDefaultSubset(); ds != nil {
		cfg.defaultSubset, _ = ScalarsFromStruct(ds) // drop non-scalar
	}
	for _, sel := range sc.GetSubsetSelectors() {
		cfg.selectors = append(cfg.selectors, sel.GetKeys())
	}
	return cfg
}
```
The wrap in `buildCluster` (after the Task-5 `lb, err := buildLeafLB(...)` / `cl.lb = lb`):
```go
lb, err := buildLeafLB(c, name, endpoints)
if err != nil {
	return nil, err // CLUSTER_PROVIDED / unsupported reject HERE — before any wrap (AMEND-SS1)
}
if sc := c.GetLbSubsetConfig(); sc != nil {
	lb = newSubsetLB(endpoints, parseLbSubsetConfig(sc), func(sub []Endpoint) (loadBalancer, error) {
		return buildLeafLB(c, name, sub)
	})
}
cl.lb = lb
```

- [ ] **Step 4: Run to verify they pass + the full suite**

```bash
cd internal/cluster && go test -run 'TestManager_.*LbSubsetConfig' ./... 2>&1 | tail
cd internal/cluster && go test ./... 2>&1 | tail   # the existing accept/reject matrix unperturbed
```
Expected: PASS; the full cluster suite green (no non-subset cluster gains a wrap — `GetLbSubsetConfig()` is nil for them).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 38.1 Task 8: parseLbSubsetConfig + wrap-after-switch (AMEND-SS1) — lb_subset_config present → newSubsetLB wraps the buildLeafLB child (factory binds buildLeafLB per subset); CLUSTER_PROVIDED rejects via the leaf factory before the wrap (ZERO new arm); empty keys/selectors/dups/fallback-default mismatches ACCEPT (parity)"
```

---

## Task 9: The 4 `lb_subsets_*` registrations + the counter injection (`registerClusterMetrics`)

**Goal:** Register the 4 `lb_subsets_*` stats in `registerClusterMetrics` via a `*subsetLB` type-assert (the `*ringHashLB`/`*maglevLB` gauge-block precedent): `lb_subsets_active` (gauge) + `lb_subsets_created` (counter) Set/Add to `numSubsets` (×1 — AMEND-SS4; NOT the reference 33×), and `lb_subsets_selected` + `lb_subsets_fallback` (counters) ALLOCATED and INJECTED onto the live `subsetLB` (`s.selected`/`s.fallbackC`) so `Pick` Inc's them (D-S38-4). Surface 1121 → 1125.

**Files:**
- Modify: `internal/cluster/manager.go` (`registerClusterMetrics`)
- Modify: `internal/cluster/manager_test.go` (the registration + injection assertions)

- [ ] **Step 1: Write the failing tests** (`manager_test.go`)

```go
func TestRegisterClusterMetrics_SubsetStats(t *testing.T) {
	reg := stats.NewRegistry()
	c := mkStaticCluster("c_sub", mkLbEndpoint("127.0.0.1", 9001), mkLbEndpoint("127.0.0.1", 9002))
	c.LbPolicy = clusterv3.Cluster_ROUND_ROBIN
	// 2 endpoints tagged v1/v2 → 2 distinct subsets under the version selector
	tag(c, 0, "version", "v1") // helper: set envoy.lb metadata on lb_endpoint i
	tag(c, 1, "version", "v2")
	c.LbSubsetConfig = &clusterv3.Cluster_LbSubsetConfig{
		FallbackPolicy:  clusterv3.Cluster_LbSubsetConfig_ANY_ENDPOINT,
		SubsetSelectors: []*clusterv3.Cluster_LbSubsetConfig_LbSubsetSelector{{Keys: []string{"version"}}},
	}
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	// active/created = the distinct-subset count ×1 (NOT 33× — AMEND-SS4)
	if got, _ := gaugeValue(reg, "cluster.c_sub.lb_subsets_active"); got != 2 { // gaugeValue returns (int64, bool) — manager_test.go:1051
		t.Errorf("lb_subsets_active = %d, want 2 (×1 distinct-subset count)", got)
	}
	if got, _ := counterValue(reg, "cluster.c_sub.lb_subsets_created"); got != 2 {
		t.Errorf("lb_subsets_created = %d, want 2", got)
	}
	// selected/fallback start at 0 and are present (injected, Inc'd by Pick)
	if s0, _ := counterValue(reg, "cluster.c_sub.lb_subsets_selected"); s0 != 0 {
		t.Error("lb_subsets_selected must be registered and start at 0")
	}
	if f0, _ := counterValue(reg, "cluster.c_sub.lb_subsets_fallback"); f0 != 0 {
		t.Error("lb_subsets_fallback must be registered and start at 0")
	}
}

func TestRegisterClusterMetrics_NonSubsetNoSubsetStats(t *testing.T) {
	reg := stats.NewRegistry()
	c := mkStaticCluster("c_plain", mkLbEndpoint("127.0.0.1", 9001))
	c.LbPolicy = clusterv3.Cluster_ROUND_ROBIN // no lb_subset_config
	if _, err := NewManager(mkBootstrap(c), reg); err != nil {
		t.Fatal(err)
	}
	if hasMetric(reg, "cluster.c_plain.lb_subsets_active") {
		t.Error("a non-subset cluster must NOT register lb_subsets_* stats")
	}
}

func TestSubsetLB_PickInjectedStatsIncFromManager(t *testing.T) {
	// End-to-end: build via the manager, drive Pick through the cluster funnel,
	// confirm the INJECTED selected/fallback counters increment.
	// ... NewManager → cl.Get → cl.PickEndpoint won't carry a match (direct path);
	// use a ctx with WithSubsetMatch via a small helper that calls cl.lb.Pick, OR
	// assert at the Pick level that s.selected is the same *Counter the registry holds. ...
}
```
> Readback helpers: `gaugeValue(reg, name) (int64, bool)` EXISTS (`manager_test.go:1051`). `counterValue`/`hasMetric` do NOT exist — ADD them as small test helpers mirroring `gaugeValue` (look the metric up in the registry by name; `Counter.Load() uint64` is the read — `internal/stats/counter.go:30`; `hasMetric` returns the `ok` of the lookup). `tag(c, i, k, v)` is a small test helper setting `lb_endpoints[i].Metadata.FilterMetadata["envoy.lb"]` — add it near `mkLbEndpoint`.

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestRegisterClusterMetrics_Subset|TestRegisterClusterMetrics_NonSubset' ./... 2>&1 | head
```
Expected: test FAILURE (the `lb_subsets_*` metrics are absent).

- [ ] **Step 3: Add the registration + injection block**

In `registerClusterMetrics` (after the `*maglevLB` block at `:118`):
```go
if s, ok := c.lb.(*subsetLB); ok {
	// active/created = the distinct-subset count (×1 — NOT the reference's 33×
	// magnitude artifact, a recorded departure; AMEND-SS4 / ADR-0240).
	r.NewGauge(prefix + "lb_subsets_active").Set(int64(s.numSubsets)) // Gauge.Set takes int64
	r.NewCounter(prefix + "lb_subsets_created").Add(uint64(s.numSubsets)) // Counter.Add takes uint64 (internal/stats/counter.go:27)
	// selected/fallback are Inc'd from subsetLB.Pick — allocate + INJECT onto the
	// live wrapper (the FIRST LB to touch stats from its own Pick path; D-S38-4).
	s.selected = r.NewCounter(prefix + "lb_subsets_selected")
	s.fallbackC = r.NewCounter(prefix + "lb_subsets_fallback")
}
```
> Confirm `*Counter` has `Add(int64)` (Task 1 Step 2 grep). If `created` should be a one-shot set rather than an additive counter, `Add(numSubsets)` once at register is correct (it never increments again — the host set is static). The ordering is sound: `buildCluster` (sets `cl.lb`) runs before `registerClusterMetrics` (D-S38-4).

- [ ] **Step 4: Run to verify they pass**

```bash
cd internal/cluster && go test -run 'TestRegisterClusterMetrics|TestSubsetLB' ./... 2>&1 | tail
```
Expected: PASS.

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/...
git add internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 38.1 Task 9: 4 lb_subsets_* stats (AMEND-SS4, D-S38-4) — *subsetLB type-assert in registerClusterMetrics: active(gauge)/created(counter) Set/Add to numSubsets ×1 (NOT the reference 33× artifact — recorded departure); selected/fallback counters allocated + INJECTED onto the live subsetLB (Inc'd from Pick); surface 1121 → 1125"
```

---

## Task 10: The HTTP `metadata_match` producer

**Goal:** Add the route producer (ADR-0239 — the seam-BUILD's producer half). `parseRouteSubsetMatch(r.GetMetadataMatch())` reads `filter_metadata["envoy.lb"]`, lowers via `cluster.ScalarsFromStruct`, REJECTS a non-scalar value (`router-metadata-match-nonscalar`, D-S38-2), and builds a `cluster.SubsetMatch` stored on `clusterRouteAction.subsetMatch`. `H1ClusterAction`/`H2ClusterAction` widen to `(c, hps, subsetMatch)`; `doH1ClusterAction` (`router.go:588`) + `doH2ClusterAction` (`router_h2.go:57`) thread `cluster.WithSubsetMatch` (ONLY when non-empty — route-static, NO per-request fold). NOTE: there is NO `doH1ClusterActionDirect` — the two dispatch functions are `doH1ClusterAction` + `doH2ClusterAction`, and the three `hcm` call sites are `actions.go:213/236/249`.

**Files:**
- Modify: `internal/filter/hcm/actions.go` (the `subsetMatch` field)
- Modify: `internal/filter/hcm/config.go` (`parseRouteSubsetMatch` + `buildRouterAction` wiring)
- Modify: `internal/filter/http/router/router.go` + `router_h2.go` (the constructors + the thread)
- Modify: `internal/filter/hcm/config_test.go` (+ a router test) (parse + reject + thread tests)

- [ ] **Step 1: Write the failing tests** (`hcm/config_test.go`)

```go
func TestParseRouteSubsetMatch_LowersScalars(t *testing.T) {
	md, _ := structpb.NewStruct(map[string]any{"version": "v1"})
	ra := &routev3.RouteAction{
		ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_echo"},
		MetadataMatch:    &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"envoy.lb": md}},
	}
	// buildRouterAction over a manager that has c_echo → clusterRouteAction.subsetMatch non-empty
	act, err := buildRouterAction(ra, mgrWith("c_echo"))
	if err != nil {
		t.Fatal(err)
	}
	cra := act.(*clusterRouteAction)
	want := cluster.NewSubsetMatch(map[string]cluster.SubsetValue{"version": {Kind: /* subsetString */ 0, Str: "v1"}})
	if cra.subsetMatch.Key() != want.Key() { // export a Key() accessor for tests OR compare via a helper
		t.Errorf("subsetMatch mismatch")
	}
}

func TestParseRouteSubsetMatch_RejectsNonScalar(t *testing.T) {
	md, _ := structpb.NewStruct(map[string]any{"version": map[string]any{"nested": 1}}) // struct value
	ra := &routev3.RouteAction{
		ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_echo"},
		MetadataMatch:    &corev3.Metadata{FilterMetadata: map[string]*structpb.Struct{"envoy.lb": md}},
	}
	_, err := buildRouterAction(ra, mgrWith("c_echo"))
	if err == nil || !strings.Contains(err.Error(), "only scalar values") {
		t.Errorf("err = %v, want the non-scalar reject (router-metadata-match-nonscalar)", err)
	}
}

func TestParseRouteSubsetMatch_NoMetadataIsEmpty(t *testing.T) {
	ra := &routev3.RouteAction{ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: "c_echo"}}
	act, err := buildRouterAction(ra, mgrWith("c_echo"))
	if err != nil {
		t.Fatal(err)
	}
	if !act.(*clusterRouteAction).subsetMatch.Empty() { // export Empty() OR assert via key() == ""
		t.Error("no metadata_match → empty subsetMatch (fallback path)")
	}
}
```
> This needs a tiny exported accessor on `cluster.SubsetMatch` for tests (e.g. `Key() string` + `Empty() bool` — promote the unexported `key()`/`empty()` to exported, OR add exported wrappers; decide at IMPL — exporting `Key()`/`Empty()` is the cleaner call since the producer/tests live in another package). Adjust Task 2's `key()`/`empty()` to exported `Key()`/`Empty()` if so (a one-line rename + update the Task 7 internal callers) — note this in PROGRESS.md as a cross-task touch.

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/filter/hcm && go test -run TestParseRouteSubsetMatch ./... 2>&1 | head
```
Expected: COMPILE/test FAILURE (`subsetMatch` field / `parseRouteSubsetMatch` absent).

- [ ] **Step 3: Implement the producer**

`actions.go` (`clusterRouteAction`):
```go
type clusterRouteAction struct {
	cluster      *cluster.Cluster
	hashPolicies []router.HashPolicy
	subsetMatch  cluster.SubsetMatch // route-static; empty when no metadata_match (ADR-0239)
}
```
`config.go` (`parseRouteSubsetMatch` + the `buildRouterAction` wiring after `parseRouteHashPolicies`):
```go
// parseRouteSubsetMatch lowers RouteAction.metadata_match["envoy.lb"] to a proto-
// free cluster.SubsetMatch (route-static — resolved ONCE at config-build, not per
// request; the contrast with applyHashKey's per-request fold). A non-scalar value
// is rejected (the scalar MVP boundary — list/struct + list_as_any deferred; SPEC
// §6.2). Absent metadata_match → the empty (no-match) SubsetMatch (the LB falls back).
func parseRouteSubsetMatch(md *corev3.Metadata) (cluster.SubsetMatch, error) {
	scalars, nonScalar := cluster.ScalarsFromStruct(md.GetFilterMetadata()["envoy.lb"])
	if len(nonScalar) > 0 {
		return cluster.SubsetMatch{}, fmt.Errorf("router: metadata_match envoy.lb key %q: only scalar values (string, number, bool) are supported", nonScalar[0])
	}
	return cluster.NewSubsetMatch(scalars), nil
}
```
In `buildRouterAction` (after `hps, err := parseRouteHashPolicies(...)`):
```go
sm, err := parseRouteSubsetMatch(r.GetMetadataMatch())
if err != nil {
	return nil, err
}
return &clusterRouteAction{cluster: c, hashPolicies: hps, subsetMatch: sm}, nil
```
`router.go`/`router_h2.go` — widen the constructors + thread the match:
```go
func H1ClusterAction(c *cluster.Cluster, hps []HashPolicy, subsetMatch cluster.SubsetMatch) Action { ... }
func H2ClusterAction(c *cluster.Cluster, hps []HashPolicy, subsetMatch cluster.SubsetMatch) H2Action { ... }
```
In `doH1ClusterAction` (`router.go:588`) and `doH2ClusterAction` (`router_h2.go:57`), after `applyHashKey(...)` and before `AcquireH1`/`Dial`:
```go
if !a.subsetMatch.Empty() {
	ctx = cluster.WithSubsetMatch(ctx, a.subsetMatch)
}
```
> The two dispatch-state structs that carry `hashPolicies` BOTH gain a `subsetMatch cluster.SubsetMatch` field: `routerAction` (`router.go:716`) and `routerActionH2` (`router_h2.go:211`); the `H1ClusterAction`/`H2ClusterAction` constructors populate it. Update ALL THREE `hcm` call sites to pass `cra.subsetMatch`: `actions.go:213` (`router.H1ClusterAction(a.cluster, a.hashPolicies)` → `(a.cluster, a.hashPolicies, a.subsetMatch)`), `:236` (same H1), `:249` (H2). Mirror the `hashPolicies` threading exactly — every site that passes `hashPolicies` also passes `subsetMatch`.

- [ ] **Step 4: Run to verify they pass + the repo builds**

```bash
cd internal/filter/hcm && go test ./... 2>&1 | tail
cd internal/filter/http/router && go test ./... 2>&1 | tail
cd /home/esa/git/envoy-go && go build ./... && echo BUILD_OK
```
Expected: PASS; the whole repo builds (the producer is the last wiring — the seam consumer [subsetLB] and the funnels [Task 4] are ready).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/filter/ && golangci-lint run ./internal/filter/...
git add internal/filter/
git commit -m "phase 38.1 Task 10: the HTTP metadata_match producer (ADR-0239, AMEND-SS6) — parseRouteSubsetMatch lowers RouteAction.metadata_match[envoy.lb] to a route-static cluster.SubsetMatch (non-scalar → router-metadata-match-nonscalar reject, D-S38-2); H1/H2ClusterAction widen to (c, hps, subsetMatch); dispatch threads WithSubsetMatch when non-empty (NO per-request fold)"
```

---

## Task 11: The `0064-lb-subset` differential fixture

**Goal:** Create `test/fixtures/0064-lb-subset/` — a STATIC-config NAT-transparent TRUE cross-side subset-affinity arm (SET-membership + within-subset spread + a fallback arm + the cross-side `selected`/`fallback` stats prong + the `/health` byte-equiv stream), mirroring the `0063` dir layout (SPEC §8.1; D-S38-5). NO new BackendKind (reuse `HTTPEcho`); NO new fuzzer.

**Files:**
- Create: `test/fixtures/0064-lb-subset/driver/driver.go`
- Create: `test/fixtures/0064-lb-subset/driver/driver_test.go`
- Create: `test/fixtures/0064-lb-subset/README.md`
- Create: `test/fixtures/0064-lb-subset/expectations.yaml`

- [ ] **Step 1: Re-pin the fixture number + study the `0063` template**

```bash
ls -d test/fixtures/[0-9]* | tail -1                     # confirm 0063 is the tail → 0064 is next
sed -n '78,100p' test/fixtures/0063-lb-maglev/driver/driver.go   # the constants block
sed -n '300,400p' test/fixtures/0063-lb-maglev/driver/driver.go  # AssertDistribution + AssertStats
```
Record the `0063` constants + the reference/subject bootstrap split (STRICT_DNS/`host.docker.internal` vs STATIC/`127.0.0.1`) in PROGRESS.md.

- [ ] **Step 2: Write the fixture config + driver**

The bootstrap (BOTH sides): chain `[http_connection_manager + router]` over ONE cluster `c_echo` with `lb_policy: ROUND_ROBIN` + `lb_subset_config: { fallback_policy: ANY_ENDPOINT, subset_selectors: [{keys: ["version"]}] }` + 4 endpoints tagged `envoy.lb {version: v1}` ×2 / `{version: v2}` ×2. Routes: `/v1` → `metadata_match {envoy.lb:{version:"v1"}}`; `/v2` → `{version:"v2"}`; `/none` → `{version:"v9"}` (no subset → ANY_ENDPOINT fallback); `/health` → `direct_response {status:200, body:{inline_string:"OK\n"}}`. Reference STRICT_DNS/`host.docker.internal`, subject STATIC/`127.0.0.1` (the `0063` split). Constants:
```go
const (
	clusterName  = "c_echo"
	versionKey   = "version"
	perRoute     = 16          // K — GETs per routed route (/v1, /v2, /none)
	healthReqs   = 8
	// totalReqs DERIVED — never a literal (reference_fixture_workload_constant_desync)
	totalReqs    = 3*perRoute + healthReqs // /v1 + /v2 + /none + /health
)
```
The driver builds BOTH bootstraps (so it KNOWS each side's version→idx map), drives the workload, and collects per-backend accept counts.

- [ ] **Step 3: Implement the assertions** (`AssertDistribution` + `AssertStats`)

`AssertDistribution(refCounts, subjCounts []uint64)` — BOTH sides:
- **Subset affinity (SET-membership):** every backend serving a `/v1` request ∈ the v1 idx set; same for `/v2`. (The driver tracks per-route per-backend counts.)
- **Within-subset spread:** both members of each 2-host subset hit across K=16 (ROUND_ROBIN → ≥1 each).
- **Fallback spread:** `/none` (ANY_ENDPOINT) spreads across ≥2 of the 4 backends.
- **Conservation:** the per-backend counts sum to `totalReqs` (minus the `direct_response` `/health`, which never reaches a backend — account for it explicitly).

`AssertStats(t, refAdminAddr, subjAdminAddr)` — cross-equal (the `0063` `scrapeStats` precedent):
- `cluster.c_echo.upstream_cx_total`, `upstream_rq_total` (= the routed total = `3*perRoute`), `membership_total` (= 4), `upstream_cx_active` (= 0, quiesced), `lb_subsets_selected` (= `/v1`+`/v2` = `2*perRoute` = 32), `lb_subsets_fallback` (= `/none` = `perRoute` = 16).
- NOT cross-equaled: `lb_subsets_active`/`created` (the 33× reference artifact vs envoy-go's ×1 — AMEND-SS4). UNIT-assert the SUBJECT side's `lb_subsets_active == 2` (2 version subsets) separately (NOT cross-side).

- [ ] **Step 4: Run the fixture (subject-side first, then cross-side where Docker is present)**

```bash
# subject-side / driver assertions (no Docker needed for the distribution logic that runs on both paths):
go test ./test/differential/ -run 'TestDifferential/0064' -count=1 2>&1 | tail
# cross-side (controller runs where Docker + the contrib image are present — reference_docker_probe_bridge_network):
#   verify the decode ran: cluster.c_echo.upstream_cx_total > 0 / downstream_cx_rx_bytes_total > 0 on the reference.
```
Expected: PASS. Confirm via `-run 'TestDifferential/0064'` (NOT `-run '0064'` — `reference_differential_run_selector`).

- [ ] **Step 5: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l test/fixtures/0064-lb-subset/ && golangci-lint run ./test/...
git add test/fixtures/0064-lb-subset/
git commit -m "phase 38.1 Task 11: 0064-lb-subset differential fixture (SPEC §8.1, D-S38-5) — STATIC-config NAT-transparent TRUE cross-side subset affinity (SET-membership /v1,/v2 + within-subset spread + ANY_ENDPOINT fallback /none) + cross-side selected/fallback stats prong + /health byte-equiv; reuses HTTPEcho (BackendKind stays 33); fixtures 65 → 66"
```

---

## Task 12: Deliberate-break liveness (`-count=1`) + the ≥20-run flake check

**Goal:** PROVE every `0064` assertion is LIVE via the three deliberate breaks (`reference_differential_break_protocol_count1` — go-test caching serves a stale PASS without `-count=1`), each restored after; confirm the workload constants are synced (`reference_fixture_workload_constant_desync`); run a ≥20-run flake check. No production change — verification only.

**Files:**
- Modify: `test/fixtures/0064-lb-subset/README.md` (record the break protocol per the `0030` lesson)
- Modify: `docs/envoy-go/phases/38-load-balancer-subset/PROGRESS.md` (the liveness evidence)

- [ ] **Step 1: Break (i) — drop the `/v1` `metadata_match`** (affinity leg)

Temporarily remove `metadata_match` from the `/v1` route in the driver's bootstrap builder → `/v1` requests take the ANY_ENDPOINT fallback (spread over ALL 4) → a served backend ∉ the v1 set → the affinity assertion MUST fail.
```bash
go test ./test/differential/ -run 'TestDifferential/0064' -count=1 2>&1 | tail   # expect FAIL (affinity)
```
RESTORE (`git restore` the driver — `feedback_subagent_worktree_detach`: never checkout-sha/amend). Re-run → PASS.

- [ ] **Step 2: Break (ii) — misroute the fallback** (fallback + stats prongs)

Give `/none` a `metadata_match {version:"v1"}` → it lands on the v1 subset (not the all-endpoints spread) AND `lb_subsets_fallback` stops incrementing (becomes a `selected`) → the fallback-spread assertion AND the stats prong (`selected`=48/`fallback`=0 vs expected 32/16) MUST fail.
```bash
go test ./test/differential/ -run 'TestDifferential/0064' -count=1 2>&1 | tail   # expect FAIL (fallback + stats)
```
RESTORE. Re-run → PASS.

- [ ] **Step 3: Break (iii) — perturb a stats expectation** (stats prong)

Temporarily change the expected `lb_subsets_selected` to a wrong value (or drop an Inc in `subsetLB.Pick` — a production break, restored immediately) → the cross-side stats prong MUST fail.
```bash
go test ./test/differential/ -run 'TestDifferential/0064' -count=1 2>&1 | tail   # expect FAIL (stats)
```
RESTORE. Re-run → PASS.

- [ ] **Step 4: ≥20-run flake check + the constant-sync guard**

```bash
go test ./test/differential/ -run 'TestDifferential/0064' -count=20 2>&1 | tail   # 20/20 PASS (deterministic — ROUND_ROBIN child, SET-membership)
grep -n "totalReqs\|perRoute\|healthReqs" test/fixtures/0064-lb-subset/driver/driver.go   # confirm totalReqs DERIVED, no literal count slice desync
```
Expected: 20/20 PASS; `totalReqs` derived from `perRoute`/`healthReqs` (no hand-rolled literal that could desync — `reference_fixture_workload_constant_desync`).

- [ ] **Step 5: Record + commit (LOCAL-ONLY)**

Record the three breaks (each: the exact edit, the FAILING assertion, the restore) in `0064/README.md` + PROGRESS.md (the `0030` lesson — name each break PRECISELY).
```bash
git add test/fixtures/0064-lb-subset/README.md docs/envoy-go/phases/38-load-balancer-subset/PROGRESS.md
git commit -m "phase 38.1 Task 12: 0064 deliberate-break liveness (reference_differential_break_protocol_count1) — drop-metadata_match (affinity) / misroute-fallback (fallback+stats) / stats-perturb, each FAILS under -count=1 then restored; 20/20 flake-free; totalReqs DERIVED (no constant desync)"
```

---

## Task 13: Full differential re-verify + regression gate

**Goal:** Confirm the seam widening + the `buildLeafLB` extraction + the wrap-after-switch + the producer did NOT perturb the 65 prior fixtures, and the full suite + race + conformance are green. No production change — gate only (the controller runs the Docker-dependent legs where Docker is present, per `reference_docker_probe_bridge_network`).

**Files:** none (verification gate; evidence → PROGRESS.md at Task 14).

- [ ] **Step 1: The full unit + race suite**

```bash
cd /home/esa/git/envoy-go && go build ./... && echo BUILD_OK
go test ./... 2>&1 | tail -30
go test -race -short ./... 2>&1 | tail -30
```
Expected: all green; no race.

- [ ] **Step 2: The full 66-dir differential**

```bash
go test ./test/differential/ -count=1 2>&1 | tail -40   # all 66 fixtures (65 prior byte-exact through the seam widening + 0064 green)
```
Expected: 66/66 PASS. The 65 prior dirs are byte-exact (the seam widening is behavior-neutral; the wrap-after-switch only fires for `lb_subset_config` clusters; the producer's `WithSubsetMatch` only sets ctx for routes with `metadata_match` — none of the prior fixtures have one).

- [ ] **Step 3: Conformance asserted-unaffected**

```bash
# h2spec 53/53 + proxy-wasm 10/10 — asserted-unaffected by change-scope (subset touches the cluster LB pick + the
# HTTP route metadata_match parse, NOT the h2 framing or the wasm path; the wire path is byte-identical when no
# lb_subset_config cluster is configured). The controller re-runs where the harness is present.
```
Record the conformance disposition (asserted-unaffected + re-run result if run) in PROGRESS.md.

- [ ] **Step 4: The count anchors at the new tip**

```bash
ls -d test/fixtures/[0-9]* | wc -l                       # expect 66 (0064 added)
grep -rh "^func Fuzz" $(find ./internal -name fuzz_test.go) | wc -l   # expect 42 (UNCHANGED — no new fuzzer)
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1   # expect 33 (UNCHANGED)
go mod tidy -diff && echo TIDY_EMPTY                     # expect empty (ZERO new dep)
```
Expected: fixtures 66, fuzzers 42, BackendKind 33, tidy empty. (The stat-surface doc count 1121 → 1125 lands at Task 14 with the BEHAVIOR_CONTRACT edit.)

- [ ] **Step 5: No commit (gate only)** — record the gate evidence; Task 14 bundles the docs + the final commit.

---

## Task 14: Completion bundle (ADR-0052 atomic landing)

**Goal:** Land the atomic six-gate completion bundle — BEHAVIOR_CONTRACT subset subsection + the 4 `lb_subsets_*` stats + the departure records (the §9 bundle); the full ADR-0239 + ADR-0240 entries (§Context + §Decision + §Consequences, ADR-0044 in-place; DECISIONS tail → ADR-0240); the count advances (fixtures 65 → 66; stat surface 1121 → 1125; DECISIONS tail → ADR-0240); STATE + ROADMAP row 38 (38.1 leg) `in-progress → done` (flat family row — NO parent rollup, ADR-0106).

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`, `docs/envoy-go/DECISIONS.md`, `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`
- Modify: `docs/envoy-go/phases/38-load-balancer-subset/PROGRESS.md`

- [ ] **Step 1: BEHAVIOR_CONTRACT (the §9 bundle)**

Add the `### Load balancer — subset (lb_subset_config)` subsection (the wrapper-not-an-enum acceptance; the enumeration semantics; the value-equality [numbers double-valued; exact key-set]; the 3-way fallback; the per-subset child honoring `lb_policy`; the seam + route-static producer; the healthy-set boundary). Add the 4 `lb_subsets_*` stats to the `## Stat-name mapping` table. Add the departure/coverage records (the scalar-value-only MVP + the `router-metadata-match-nonscalar` reject; the `active`/`created` 33×-vs-×1 magnitude departure; the deferred flags/stats; the `CLUSTER_PROVIDED + lb_subset_config` parity-in-outcome coverage note; the NEW `Endpoint.Metadata` dimension; NO new fuzzer/BackendKind; the TRUE cross-side SET-membership posture). Update the stat-surface doc count **1121 → 1125**.

- [ ] **Step 2: DECISIONS (ADR-0239 + ADR-0240, ADR-0044 in-place)**

Promote the SPEC §13 §Context DRAFTs to the full entries (§Context + §Decision + §Consequences; status ACCEPTED). ADR-0239 (the subset-match seam-input + the `RouteAction.metadata_match` producer). ADR-0240 (the subset LB policy + the `Endpoint.Metadata` dimension; RECORD the ADR-0024 wrinkle — the FIRST LB to Inc stats from `Pick`, per-cluster scope still holds, NOT a separate ADR-0024 amendment — D-S38-4). DECISIONS tail → **ADR-0240**; next-free **ADR-0241**.

- [ ] **Step 3: STATE + ROADMAP**

STATE: active-phase `phase 38.1 (load-balancer-subset) IMPL done`; counts (fixtures 66, stat surface 1125, fuzzers 42, BackendKind 33, DECISIONS tail ADR-0240); lifecycle → next is the phase-38.2 SPEC (weighted-cluster routing + `ClusterWeight.metadata_match`) OR the next Load-balancing-family candidate. ROADMAP: row 38 (38.1 leg) `in-progress → done` (flat family row — NO parent rollup, ADR-0106); the Load-balancing family stays OPEN (3 candidates remain after 38; 38.2 pre-authorized).

- [ ] **Step 4: PROGRESS.md final + the six-gate evidence**

Mark all 14 tasks complete; record the six-gate evidence (build / unit+race / 66-dir differential / conformance asserted-unaffected / counts / docs). Confirm the ADR-0045 verdict held (NO further split — 14 tasks / ~300–450 LoC).

- [ ] **Step 5: The atomic completion commit (LOCAL-ONLY) — then the controller pushes**

```bash
go build ./... && go test ./... 2>&1 | tail   # final green confirmation BEFORE the bundle commit
git add docs/envoy-go/
git commit -m "phase 38.1 IMPL DONE: subset LB (lb_subset_config) — the FIRST LB wrapper (ADR-0239 seam-input+producer / ADR-0240 policy+Endpoint.Metadata); 4 lb_subsets_* stats (surface 1121 → 1125, active/created ×1 NOT the 33× artifact); 0064 TRUE cross-side subset affinity (fixtures 65 → 66); BackendKind 33 + fuzzers 42 UNCHANGED; ROADMAP row 38 (38.1) → done"
```
> The controller squash-merges to master + pushes at stage-close (`feedback_push_to_origin` / `feedback_subagents_no_push`). h2spec + proxy-wasm + the Docker differential legs run where the harness/Docker are present (`reference_docker_probe_bridge_network`).

---

## Verification checklist (the ADR-0052 six-gate, at Task 14)

1. **Build:** `go build ./...` green; `go mod tidy -diff` empty (ZERO new dep).
2. **Unit + race:** `go test ./...` + `go test -race -short ./...` green (incl. the new `subset_test.go` + the widened cluster suite + the producer tests).
3. **Differential:** the full 66-dir suite green (`-count=1`); `0064` liveness-proven (3 breaks); the 65 prior dirs byte-exact.
4. **Conformance:** h2spec 53/53 + proxy-wasm 10/10 asserted-unaffected (re-run where present).
5. **Counts:** fixtures 65 → 66; stat surface 1121 → 1125; fuzzers 42 UNCHANGED; BackendKind 33 UNCHANGED; DECISIONS tail → ADR-0240.
6. **Docs:** BEHAVIOR_CONTRACT subset subsection + stats + departures; ADR-0239 + ADR-0240 full entries; STATE + ROADMAP row 38 (38.1) → done.
