# Phase 38.2 SPEC — `RouteAction.weighted_clusters` routing + `WeightedCluster.ClusterWeight.metadata_match`: a per-request weighted-random cluster-SELECTION primitive composing with the 38.1 subset LB — the pre-authorized SECOND leg of the phase-38 by-plane split

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (the 38.2 PLAN; SKILL_ROUTING state 2 → 3). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. Phase 38 is a flat Load-balancing-family row WITH a PRE-AUTHORIZED 38.1/38.2 by-plane split (ADR-0106 + ADR-0045; the phase-38 SPEC §3.0). 38.1 (subset LB on the HTTP `RouteAction.metadata_match` plane) LANDED at the phase-38.1 IMPL (squash `4eedf0e`); this SPEC scopes **38.2** (the weighted-cluster routing plane — the pre-authorized second leg, the 36.1/36.2 precedent). Phase 38 keeps the Load-balancing family OPEN; 3 candidates remain after 38.2 (locality-weighted LB, priority load balancing, panic thresholds — all health-gated).

**Goal:** Land `RouteAction.weighted_clusters` routing (the `RouteAction_WeightedClusters` `ClusterSpecifier` oneof member — AMEND-WC1) — a NEW per-request weighted-random cluster-SELECTION primitive — plus `WeightedCluster.ClusterWeight.metadata_match` (proto field 3 — AMEND-WC1), the per-weighted-cluster subset match that COMPOSES with the 38.1 subset LB. The router resolves a SINGLE `*cluster.Cluster` per route TODAY (`buildRouterAction`/`config.go:540` accepts ONLY `RouteAction_Cluster`; `clusterRouteAction`/`actions.go:201` holds one cluster); 38.2 adds the second `ClusterSpecifier` arm: a route may carry N weighted clusters, and per request the router picks ONE by integer weight (weighted-random, mirroring upstream — AMEND-WC2) and EACH `ClusterWeight` may carry its OWN `metadata_match` that MERGES with the route-level `RouteAction.metadata_match` (entry values taking precedence — AMEND-WC1/WC5) and feeds the chosen cluster's subset selector via the EXISTING ADR-0239 ctx seam (`WithSubsetMatch`), UNCHANGED. In ONE flat 38.2 leg: ZERO new packages (a new weighted route-action type + selector in the existing `internal/filter/http/router` + the producer in the existing `internal/filter/hcm`), ZERO new go.mod deps (the whole surface is in the pinned `/envoy v1.32.4` — AMEND-WC1, `go mod tidy -diff` empty). The differential proof is the cross-side **weight-DISTRIBUTION band** arm (`0065-weighted-clusters` — per-request weighted-random selection → a PER-SIDE aggregate distribution over the per-cluster `upstream_rq_total` counters at a flake-safe σ-margin, AMEND-WC2/WC6) + a metadata_match-composition arm (a weighted cluster carrying `metadata_match` → subset affinity within the selected cluster, the 38.1 compose) + cross-side conservation.

**Architecture:** A new `weightedClusterRouteAction` bridge type (`internal/filter/hcm/actions.go`, the `clusterRouteAction` sibling) holds N parsed entries — each `{cluster *cluster.Cluster, weight uint32, subsetMatch cluster.SubsetMatch}` — plus the route-level `hashPolicies` (shared across entries) and a per-action `weightedSelector`. It satisfies the SAME `routeAction` interface (`do`/`asRouterAction`/`asRouterActionH2`, `route.go:63`) by delegating to new router-package constructors `H1WeightedClusterAction`/`H2WeightedClusterAction` (the `H1ClusterAction`/`H2ClusterAction` sibling). The selector mirrors `randomLB` (`internal/cluster/random.go`): a per-action mutex-guarded `newPCGRNG()` draw (`leastrequest.go:63`) `rng() % totalWeight` then a cumulative-weight walk → the chosen entry index (AMEND-WC2). The chosen entry's `cluster` + `subsetMatch` then flow through the EXISTING dispatch (`doH1ClusterAction`/`doH2ClusterAction`) UNCHANGED — `IncUpstreamRqTotal`, `applyHashKey` (route-level hashPolicies), the 38.1 `WithSubsetMatch` thread (when the entry's merged match is non-empty), `AcquireH1`/`DialH2`. The producer parses `weighted_clusters` ONCE at config-build (`buildRouterAction` → a new `buildWeightedRouterAction`): validates the entry list (the §6 reject roster — envoy-go does its OWN parse-reject, NOT PGV), resolves each `name` against the cluster manager, MERGES each entry's `ClusterWeight.metadata_match["envoy.lb"]` over the route-level `RouteAction.metadata_match["envoy.lb"]` (entry precedence) into a proto-free `cluster.SubsetMatch` (route-static — resolved ONCE, threaded VERBATIM per request, NO per-request fold), and computes the cumulative weights. `total_weight` is IGNORED (deprecated upstream — the client uses the sum, AMEND-WC1). The exported `cluster` surface is UNTOUCHED (38.2 is a pure routing-layer primitive — it picks a cluster then reuses the 38.1 seam; NO `loadBalancer.Pick` change). ZERO new stats (AMEND-WC4 — weighted selection is observable purely through the pre-existing per-cluster `upstream_rq_*` counters; surface 1125 → 1125).

**Tech stack:** Go 1.26.x / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227, @ `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`). go-control-plane **`/envoy v1.32.4`** (ADR-0008 — `RouteAction_WeightedClusters` + `WeightedCluster` + `WeightedCluster_ClusterWeight` {`name`/`weight`/`metadata_match`} already in the pinned module; **ZERO new go.mod dep**, `go mod tidy -diff` empty — AMEND-WC1). Reuses `internal/filter/hcm/{actions.go [clusterRouteAction — the bridge precedent], config.go [buildRouterAction + parseRouteSubsetMatch + parseRouteHashPolicies]}`, `internal/filter/http/router/{router.go [H1ClusterAction + doH1ClusterAction + applyHashKey + the routerAction struct], router_h2.go [H2ClusterAction + doH2ClusterAction + routerActionH2]}`, the 38.1 subset seam (`cluster.SubsetMatch`/`NewSubsetMatch`/`ScalarsFromStruct`/`WithSubsetMatch` — UNCHANGED; `internal/cluster/subset.go`+`cluster.go`), the `randomLB` selection model (`internal/cluster/random.go` + `newPCGRNG` `leastrequest.go:63`), the differential harness (the `0060-lb-random` per-side `DistributionAsserter` band precedent + the `0063`/`0064` cross-side HTTP-route `StatsAsserter` + the `HTTPEcho` backend), upstream Envoy v1.37.2 source (`source/common/router/config_impl.{h,cc}` [`WeightedClusterEntry` / `pickWeightedCluster`] + `route_components.proto`) for the algorithm pins. ZERO new packages.

**Authored:** 2026-06-14. **Empirical-pin probe date:** 2026-06-14.

---

## 1. Purpose / Mission

Phase 38.2 lands the **weighted-cluster routing primitive** — the pre-authorized SECOND leg of the phase-38 by-plane split (BRAINSTORM §1.4/§2.3; the phase-38 SPEC §3.0). Where 38.1 added subset LB on the HTTP `RouteAction.metadata_match` plane (one cluster per route, a route-static subset match), 38.2 adds the orthogonal `weighted_clusters` `ClusterSpecifier`: a route fans across N clusters, ONE chosen per request by integer weight. It is therefore (i) the FIRST per-request cluster-SELECTION primitive (every prior route resolves a SINGLE cluster at config-build), (ii) the SECOND `RouteAction.ClusterSpecifier` arm (after `RouteAction_Cluster`), (iii) the FIRST router-layer use of the `randomLB` per-request RNG model OUTSIDE the cluster LB, and (iv) the COMPOSITION point for the 38.1 subset LB — each `ClusterWeight.metadata_match` rides the EXISTING ADR-0239 ctx seam (`WithSubsetMatch`) into the chosen cluster's subset selector, UNCHANGED.

This SPEC refines the phase-38 BRAINSTORM (`docs/envoy-go/phases/38-load-balancer-subset/BRAINSTORM.md`, §1.4/§2.3 — the pre-authorized 38.1/38.2 by-plane split) against the AS-BUILT 38.1 producer + seam (`internal/filter/hcm` + `internal/filter/http/router` + `internal/cluster`) + the §11 D-WC1..D-WC6 empirical pins EXECUTED IN-SESSION (parallel-subagent fan-out) against (1) the live contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (a `--mode validate` `weighted_clusters` reject matrix + a live weighted-route 70/30 traffic-distribution + `/stats` name-set probe on a docker BRIDGE network per `reference_docker_probe_bridge_network`), (2) go-control-plane `/envoy v1.32.4` bindings, and (3) upstream Envoy v1.37.2 route semantics (`config_impl` `WeightedClusterEntry` selection). It anchors the ADR-0241 §Context DRAFT (§13) and CONSUMES the pre-authorized split (the second leg goes straight to its own SPEC per the 36.1/36.2 precedent).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D-WC1..D-WC6 scrape CONFIRMED the BRAINSTORM's "weighted-cluster routing is a new per-request selection primitive" framing and SIMPLIFIED three anticipations: (a) `total_weight` is DEPRECATED (the client uses the weight SUM — no total-vs-sum cross-check, no reject on mismatch); (b) weighted selection adds ZERO new stat surface (observable purely through the pre-existing per-cluster `upstream_rq_*` counters); (c) the selection is per-request weighted-RANDOM (NOT deterministic round-robin-by-weight) → the differential is a PER-SIDE distribution band (never cross-side per-request equality), reusing the `0060-lb-random` band precedent. The load-bearing pins, each carried into the relevant §§ below:

- **AMEND-WC1 (D-WC1 — the v1.32.4 weighted-cluster surface re-pinned; ZERO new dep; `total_weight` deprecated; PGV near-empty).** `RouteAction.weighted_clusters` is the `RouteAction_WeightedClusters` `ClusterSpecifier` oneof member → `*WeightedCluster` (`route_components.pb.go`; `RouteAction.GetClusterSpecifier()` type-switches it alongside the existing `*RouteAction_Cluster`). `WeightedCluster` carries `Clusters []*WeightedCluster_ClusterWeight` (field 1), `TotalWeight *wrapperspb.UInt32Value` (field 3 — **DEPRECATED**: "the client will use the sum of all cluster weights"; ignored for routing math), `RuntimeKeyPrefix string` (field 2, DEFERRED — runtime layer), and `RandomValueSpecifier` oneof → `WeightedCluster_HeaderName` (DEFERRED — header-seeded RNG). `WeightedCluster_ClusterWeight`: `Name string` (field 1), `ClusterHeader string` (field 12, DEFERRED — header-named cluster; mutually exclusive with `Name`), `Weight *wrapperspb.UInt32Value` (field 2 — PRESENCE-REQUIRED, NO range rule), `MetadataMatch *core.Metadata` (field 3 — **MERGES with `RouteAction.metadata_match`, entry values taking precedence**, "filter name should be `envoy.lb`"), `RequestHeadersToAdd`/`RequestHeadersToRemove`/`ResponseHeadersToAdd`/`ResponseHeadersToRemove` (fields 4/9/5/6, DEFERRED — per-weighted header mutation), `TypedPerFilterConfig` + `HostRewriteSpecifier` (DEFERRED). **PGV imposes almost NOTHING**: `WeightedCluster.clusters` has `min_items: 1`; `ClusterWeight.weight` and `ClusterWeight.name` have NO value rule (the `name` `min_len: 1` was REMOVED — proto comment `[#next-major-version: Need to add back the validation rule]`); `request/response_headers_to_*` capped at 1000; `cluster_header`/`*_headers_to_remove`/`host_rewrite_literal` carry only a `^[^\x00\n\r]*$` pattern. **envoy-go does NOT run PGV** (it does its OWN parse-reject, §6) → every reject below is authored explicitly. `go mod tidy -diff` → exit 0, EMPTY; `go build ./...` → OK — **ZERO new go.mod dep**. See §5 / §6 / §11.1.
- **AMEND-WC2 (D-WC2 SPEC-BLOCKING — per-request weighted-RANDOM selection LIVE-confirmed; `rng() % sum` + cumulative walk; per-side independent).** A live 70/30 two-cluster weighted route over 1500 requests produced 1035 A / 465 B (69.0%) with per-500 A-counts 334 / 356 / 345 — a spread of ±~11 around the mean 350, the binomial signature of independent per-request random draws (σ = √(n·p·q) = √(500·0.7·0.3) ≈ 10.25; all observed within ±2σ). This is per-request weighted-RANDOM, NOT deterministic round-robin-by-weight (which would produce near-exact 350/150 every run, σ≈0). Upstream draws a per-request random value in `[0, total_weight)` and walks the cumulative weights (`config_impl` `WeightedClusterEntry`). envoy-go mirrors this with a per-action `newPCGRNG()` (`math/rand/v2` PCG seeded from crypto/rand, mutex-guarded — the `randomLB` model, `random.go`): `idx = walk(cumulativeWeights, rng() % totalWeight)`. **Cross-instance consequence:** two independent instances given the SAME request sequence pick DIFFERENT clusters per request but statistically-matching AGGREGATE distributions → the differential asserts the PER-SIDE aggregate split (counter ratios), NEVER per-request equality (the `0060-lb-random` per-side-band posture; `reference_differential_hash_key_cross_side_infeasible` lineage — cross-side per-request identity is infeasible for a randomized policy). See §3 / §8 / §11.2.
- **AMEND-WC3 (D-WC3 — the accept/reject matrix LIVE-pinned; SIX code-level rejects; `total_weight` mismatch ACCEPTS).** The live `--mode validate` matrix found the weighted-cluster reject contract is CODE-LEVEL (Envoy `Router::ConfigImpl`/`WeightedClusterEntry` construction, NOT PGV beyond the `clusters min_items:1`). The rejects (with the reference's verbatim text, which envoy-go matches in OUTCOME with house wording per ADR-0080): (i) **empty clusters** → PGV `value must contain at least 1 item(s)`; (ii) **sum of weights == 0** → `Sum of weights in the weighted_cluster must be greater than 0.`; (iii) **`weight` wrapper UNSET on an entry** → `Field 'weight' is missing …` (a presence check — because `weight` is a `UInt32Value`, an explicit `0` is a VALID value [accepts] and only total omission rejects); (iv) **both `name` and `cluster_header`** → `Only one of name or cluster_header can be specified`; (v) **neither `name` nor `cluster_header` (incl. `name: ""`)** → `At least one of name or cluster_header need to be specified`; (vi) **dangling cluster name** (a `name` not in the cluster manager) → `route: unknown weighted cluster '<name>'` — caught at config-init, NOT deferred to a runtime 404/503. ACCEPTED (envoy-go must NOT over-reject): a non-matching `total_weight` (deprecation-warns, ignored — uses the sum); an explicit `weight: 0` on an entry (sum>0 from others); a single-entry weighted list; a huge weight (≤ uint32 max); a skewed split; an entry carrying `metadata_match`/`request_headers_to_add`. The reject PRECEDENCE: name/header presence → weight presence → cluster-exists → sum>0. envoy-go authors ALL of these (it has no PGV) — including the cluster_header-set case, which 38.2 rejects as UNSUPPORTED (cluster_header DEFERRED). See §6 / §11.3.
- **AMEND-WC4 (D-WC4 — ZERO new stat surface; observable via the pre-existing per-cluster `upstream_rq_*`; surface 1125 → 1125).** The live `/stats` scrape (663 stat lines) found **NO** `*weighted*` stat anywhere; `grep -i weight` matched only `cluster.<name>.max_host_weight` (an ENDPOINT host-weight gauge present on EVERY cluster, value 1 here — unrelated to the route weight, NOT reflecting the 70/30 config). There is NO per-route/per-vhost weighted-cluster selection counter. The split is observable PURELY through the EXISTING per-cluster counters `cluster.<name>.upstream_rq_total` (1035 / 465 live) + `upstream_rq_2xx`/`upstream_rq_completed` — which envoy-go ALREADY emits (phase 06.1). **38.2 adds ZERO new stat names; surface STAYS 1125.** The differential observes the distribution through these counters (per-side band; cross-side the SUM is conservation-equal). See §7 / §11.4.
- **AMEND-WC5 (D-WC5 — `ClusterWeight.metadata_match` MERGES over the route-level match, entry precedence; route-static; reuses the ADR-0239 seam UNCHANGED).** Per the proto (`route_components.pb.go:4030-4035`): `ClusterWeight.metadata_match` "will be merged with what's provided in `RouteAction.metadata_match`, with values here taking precedence." Both are STATIC route config → the merge happens ONCE at config-build (per entry): `mergedScalars = routeLevelScalars ⊕ entryScalars` (entry wins on key collision) → `cluster.NewSubsetMatch(mergedScalars)` stored on the entry. Per request, the chosen entry's merged `SubsetMatch` is threaded VERBATIM via the EXISTING `cluster.WithSubsetMatch` (ADR-0239) — NO new seam, NO `loadBalancer.Pick` change, NO per-request fold. The match is BEHAVIOR-NEUTRAL on a cluster WITHOUT `lb_subset_config` (the 38.1 leaf-policy widening ignores the ctx match) and COMPOSES with a subset cluster (the merged match selects the subset within the chosen cluster). Non-scalar `envoy.lb` values DEPARTURE-reject via the existing `parseRouteSubsetMatch` path (the 38.1 `router-metadata-match-nonscalar` reject, reused per entry + route-level). See §3.2 / §11.5.
- **AMEND-WC6 (D-WC6 — the `0065` design pinned; ~200–320 prod LoC / ~10–13 tasks → single flat 38.2 leg; no FuzzWeightedSelect; ONE ADR).** The `0065-weighted-clusters` fixture is a cross-side weight-DISTRIBUTION band arm (per-side `DistributionAsserter` over the per-cluster `upstream_rq_total` counts at a ~4–5σ margin per `reference_differential_band_sigma_margin`) + a metadata_match-composition arm + cross-side conservation. The 38.2 production footprint: the `weightedSelector` (cumulative weights + the RNG draw + the walk) ~50–80 + the producer (`buildWeightedRouterAction` + the §6 reject roster + the per-entry metadata_match merge) ~90–130 + the `weightedClusterRouteAction` bridge + the H1/H2 `WeightedClusterAction` constructors + the dispatch reuse ~60–90 → **~200–320 prod LoC / ~10–13 tasks**, UNDER the ADR-0045 gate (`> ~25 tasks OR > ~1500 LoC`). **Single flat 38.2 leg; NO further split.** A `FuzzWeightedSelect` is anticipated NOT warranted (the weights are xDS config validated at build — no untrusted wire input; a selection-distribution PROPERTY test is unit-level, folded into the selector's test). ONE ADR anticipated (ADR-0241 — the weighted-cluster routing primitive + the metadata_match merge composition; §13). See §3.0 / §8 / §11.6.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0240** (the subset LB policy + `Endpoint.Metadata`, ACCEPTED); next-free **ADR-0241**. Per the phase-38.2 routing, the DECISIONS tail **STAYS ADR-0240 at this SPEC** (counts UNCHANGED at the SPEC); the ADR-0241 (the weighted-cluster routing primitive + the metadata_match merge) §Context draft is anchored in §13 and the full DECISIONS.md entry (§Context + §Decision + §Consequences) lands at the phase-38.2 IMPL per ADR-0044 (DECISIONS tail → ADR-0241; next-free after phase 38.2 ≈ ADR-0242). All six D-WC pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **`ClusterWeight.cluster_header`** — the header-named weighted cluster (read the cluster name from a request header; field 12). DEFERRED — the MVP uses literal `name` only (the `buildRouterAction` "literal cluster name only" lineage). A `cluster_header`-bearing entry REJECTS (§6).
- **`ClusterWeight` header mutation** — `request_headers_to_add`/`request_headers_to_remove`/`response_headers_to_add`/`response_headers_to_remove` (per-weighted-cluster header rewriting). DEFERRED — accepted-but-INERT is NOT chosen; the MVP IGNORES them silently (no header mutation), consistent with the route-level header-mutation scope of prior phases. (The PLAN confirms silent-ignore vs reject; anticipated silent-ignore — they are accepted upstream and orthogonal to the selection primitive.)
- **`ClusterWeight.typed_per_filter_config`** — per-weighted-cluster filter config override. DEFERRED (the per-route typed-config plane is its own concern).
- **`ClusterWeight.host_rewrite_literal` / host_rewrite_specifier** — per-weighted-cluster host rewrite. DEFERRED.
- **`WeightedCluster.total_weight`** — DEPRECATED upstream (the client uses the SUM of cluster weights — AMEND-WC1); IGNORED for routing math, NOT cross-checked against the sum, NO reject on mismatch.
- **`WeightedCluster.runtime_key_prefix`** — the runtime-overridable per-cluster weights (the RTDS/runtime layer). DEFERRED (the whole runtime layer is unstarted — ROADMAP §Feature Families "Runtime + hot restart").
- **`WeightedCluster.random_value_specifier` (`header_name`)** — the header-seeded selection RNG (deterministic-by-header selection). DEFERRED — the MVP uses the per-action PCG RNG (per-request random, the `randomLB` model). The header-seeded path would make selection request-deterministic (a future refinement; orthogonal to the MVP distribution).
- **The tcp_proxy `weighted_clusters` plane** (`TcpProxy.weighted_clusters`) — weighted clusters on the L4 tcp_proxy. DEFERRED entirely (the HTTP-router plane is the 38.2 leg).
- **New stat surface** — 38.2 adds NO new stats (AMEND-WC4); a per-route weighted-selection counter is NOT introduced (the reference emits none; the split is observable via the existing per-cluster `upstream_rq_*`).
- **Healthy-cluster awareness** — the reference does not health-gate weighted-cluster selection beyond the per-cluster LB's own behavior; with no active health checking (the Upstream-robustness family's territory) selection is over the configured weights unconditionally.

---

## 3. The weighted-cluster routing primitive + the metadata_match merge (ADR-0241)

### 3.0 Split disposition — D-WC6 RESOLVED (single flat 38.2 leg)

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 production LoC`. 38.2 is the pre-authorized SECOND leg of the phase-38 by-plane split (BRAINSTORM §1.4; the phase-38 SPEC §3.0) — chartered because the weighted-cluster plane couples a DIFFERENT subsystem (per-request HTTP routing selection) from 38.1's cluster-LB subset plane. 38.2's own surface:

| Unit | Anticipated production LoC |
|---|---|
| The `weightedSelector`: the cumulative-weight array + the per-action `newPCGRNG()` draw (`rng() % totalWeight`) + the cumulative walk → entry index (the `randomLB` model) | ~50–80 |
| The producer: `buildWeightedRouterAction` (the §6 reject roster — empty/sum-0/weight-unset/name-xor-header/dangling) + the per-entry `metadata_match` merge over the route level (`ScalarsFromStruct` ⊕ → `NewSubsetMatch`) + the cumulative-weight build | ~90–130 |
| The `weightedClusterRouteAction` bridge (`actions.go`, the `clusterRouteAction` sibling — the entries + the selector; `do`/`asRouterAction`/`asRouterActionH2`) + the `H1WeightedClusterAction`/`H2WeightedClusterAction` router constructors (per-request pick → the EXISTING `doH1ClusterAction`/`doH2ClusterAction`) | ~60–90 |
| The `0065` fixture driver + asserters + the selector unit/property tests | test-side LoC, NOT counted |

Net production **~200–320 LoC, ~10–13 tasks** — BOTH axes UNDER the gate. **Single flat 38.2 leg — NO further split.** The PLAN re-checks the gate per ADR-0045 (anticipated NO further split). The 38.2 leg flips ROADMAP row 38's `done` posture forward by adding the 38.2 sub-leg record (NO parent rollup per ADR-0106 — the flat family row); the Load-balancing family STAYS OPEN (3 candidates remain after 38.2).

### 3.1 The weighted-cluster route action + the per-request selector (ADR-0241; AMEND-WC2)

`internal/filter/hcm/actions.go` gains `weightedClusterRouteAction` (the `clusterRouteAction` sibling). Built ONCE at config-build (`buildWeightedRouterAction`) when the route's `ClusterSpecifier` is `RouteAction_WeightedClusters`; immutable thereafter (the entries + cumulative weights are read-only; only the RNG state mutates, mutex-guarded). Indicative shape (the PLAN/IMPL finalizes):

```go
// weightedClusterRouteAction is the per-request weighted-random cluster-SELECTION
// route action (ADR-0241), the sibling of clusterRouteAction (the single-cluster
// bridge). It holds N entries (each a resolved cluster + its merged subset match)
// + the route-level hashPolicies (shared) + a per-action weightedSelector. At
// dispatch it picks ONE entry by weight (weighted-random — the randomLB model)
// then runs the EXISTING H1/H2 cluster dispatch with the chosen entry's cluster +
// subsetMatch (the 38.1 ADR-0239 seam, threaded UNCHANGED). NO loadBalancer.Pick
// change — 38.2 is a routing-layer primitive that SELECTS a cluster then reuses
// the 38.1 dispatch.
type weightedClusterRouteAction struct {
    entries      []weightedEntry      // immutable after build
    hashPolicies []router.HashPolicy  // route-level RouteAction.hash_policy (shared across entries; 36.2/ADR-0237)
    selector     *weightedSelector    // per-action weighted-random picker (mutex-guarded RNG)
}

type weightedEntry struct {
    cluster     *cluster.Cluster
    weight      uint32               // the per-entry weight (sum > 0; an explicit 0 is legal)
    subsetMatch cluster.SubsetMatch  // route-level metadata_match ⊕ ClusterWeight.metadata_match (entry precedence; 38.1 ADR-0239)
}

// weightedSelector draws a per-request entry index by weight, mirroring upstream's
// WeightedClusterEntry (a random value in [0, totalWeight) then a cumulative walk)
// and envoy-go's randomLB (a per-action mutex-guarded newPCGRNG()). totalWeight is
// the SUM of entry weights (total_weight is deprecated/ignored — AMEND-WC1).
type weightedSelector struct {
    cumulative  []uint32  // running sum; cumulative[i] = Σ weights[0..i]; last == totalWeight
    totalWeight uint32
    rng         func() uint64  // newPCGRNG() — injectable for deterministic tests
}

func (s *weightedSelector) pick() int {
    r := uint32(s.rng() % uint64(s.totalWeight))
    // first cumulative bucket strictly greater than r (linear or binary search)
    for i, c := range s.cumulative {
        if r < c {
            return i
        }
    }
    return len(s.cumulative) - 1 // unreachable when totalWeight == cumulative[last]
}
```

`asRouterAction()` / `asRouterActionH2()` return the router-package constructors `H1WeightedClusterAction(entries, hashPolicies, selector)` / `H2WeightedClusterAction(...)` (the `H1ClusterAction`/`H2ClusterAction` sibling). The constructor's closure, per request, calls `selector.pick()` → the chosen `weightedEntry`, then runs the EXISTING `doH1ClusterAction(ctx, &routerAction{cluster: e.cluster, hashPolicies, subsetMatch: e.subsetMatch}, req)` (the `routerAction` may be pre-built per entry at config-build to avoid a per-request alloc — the PLAN decides). The chosen entry flows through the UNCHANGED dispatch: `IncUpstreamRqTotal` (on the chosen cluster), `applyHashKey` (route-level hashPolicies), `WithSubsetMatch` (when `e.subsetMatch` non-empty — the 38.1 thread), `AcquireH1`/`DialH2`. **The selector RNG is per-action** (one route → one selector); H1 and H2 never share a route, so a single selector per action is correct. The `do(ctx, req, bw)` method (the legacy direct-call interface arm) delegates likewise (picks then runs the chosen entry's H1 dispatch).

### 3.2 The producer + the metadata_match merge (ADR-0241; AMEND-WC1/WC3/WC5)

`buildRouterAction` (`config.go:536`) gains the SECOND `ClusterSpecifier` arm:

```go
switch cs := r.GetClusterSpecifier().(type) {
case *routev3.RouteAction_Cluster:
    // ... the existing single-cluster path (38.1) ...
case *routev3.RouteAction_WeightedClusters:
    return buildWeightedRouterAction(r, cs.WeightedClusters, clusters)
default:
    return nil, fmt.Errorf("route action: cluster_specifier %T is not supported …", r.GetClusterSpecifier())
}
```

`buildWeightedRouterAction(r, wc, clusters)` (the new producer; the `buildRouterAction`/`parseRouteSubsetMatch` style):
1. **Validate the entry list** (the §6 reject roster — envoy-go does its OWN parse-reject, NO PGV): non-empty `wc.GetClusters()`; for each `ClusterWeight`: reject `cluster_header` set (UNSUPPORTED — deferred); require `name` non-empty (the "name required" arm; covers the neither-set + empty-name cases); require `weight` wrapper present (the presence check — `GetWeight() != nil`; an explicit `0` is legal); resolve `name` against `clusters.Get(name)` (reject dangling).
2. **Compute the sum** of entry weights; reject `sum == 0` (`total_weight` is IGNORED — AMEND-WC1).
3. **Merge each entry's `metadata_match`** over the route-level `RouteAction.metadata_match`: `routeScalars, routeNonScalar := ScalarsFromStruct(r.GetMetadataMatch().GetFilterMetadata()["envoy.lb"])`; `entryScalars, entryNonScalar := ScalarsFromStruct(cw.GetMetadataMatch().GetFilterMetadata()["envoy.lb"])`; reject either non-scalar (the 38.1 `router-metadata-match-nonscalar` arm, reused); `merged := union(routeScalars, entryScalars)` with **entry values overriding route values on key collision** (AMEND-WC5); `e.subsetMatch = cluster.NewSubsetMatch(merged)`.
4. **Parse the route-level hashPolicies** ONCE (`parseRouteHashPolicies(r.GetHashPolicy())` — shared across entries; the existing producer).
5. **Build the cumulative weights** + the `weightedSelector` (a `newPCGRNG()`; boot-fail on the crypto/rand error — the `randomLB` precedent) and return `&weightedClusterRouteAction{entries, hashPolicies, selector}`.

Because the entries + merged matches are ROUTE-STATIC (resolved ONCE at build, threaded VERBATIM per request), the per-request hot path is ONLY `selector.pick()` + the existing dispatch — NO per-request proto access, NO per-request match computation (the contrast with the per-request `applyHashKey` header fold; the consistency with the 38.1 route-static `WithSubsetMatch` thread). The exported `cluster` surface is UNTOUCHED (38.2 reuses the 38.1 ctx seam — `WithSubsetMatch` / `subsetMatchFrom` UNCHANGED; NO `loadBalancer.Pick` change).

### 3.3 Composition with the 38.1 subset LB (the seam reuse)

The merged `subsetMatch` on a chosen entry rides the EXISTING `cluster.WithSubsetMatch` ctx seam (ADR-0239) into the chosen cluster's `Dial`/`AcquireH1`/`DialH2` → `subsetMatchFrom(ctx)` → the cluster's `loadBalancer.Pick(hashKey, hasHash, match, hasMatch)`. TWO composition cases, both BEHAVIOR-NEUTRAL by construction:
- **Chosen cluster has NO `lb_subset_config`** — the cluster's leaf policy (roundRobin/leastRequest/randomLB/ringHashLB/maglevLB) ABSORBS `(match, hasMatch)` as the 38.1 behavior-neutral widening and ignores it. The weighted route still routes by weight; the `metadata_match` is inert (harmless). This is the common case for the `0065` distribution arm.
- **Chosen cluster HAS `lb_subset_config`** — the cluster's `subsetLB.Pick` resolves the merged match to a subset and delegates (or applies the cluster-level fallback). The weighted entry's merged `metadata_match` selects the subset WITHIN the chosen cluster — the full 38.1+38.2 composition (the `0065` composition arm). Entry precedence over the route-level match (AMEND-WC5) means an entry can NARROW or OVERRIDE the route's subset selection for its own cluster.

NO new seam, NO `loadBalancer.Pick` change — 38.2 is a pure SELECTION primitive layered ON the 38.1 dispatch.

---

## 4. Framework primitives — a NEW route-action type + a NEW ClusterSpecifier arm + a per-action RNG + 0 new packages + 0 new go.mod deps

- **A new `routeAction` implementation** (`weightedClusterRouteAction`) + a per-request `weightedSelector` — the FIRST per-request cluster-selection primitive; it SELECTS a cluster then reuses the EXISTING per-cluster dispatch.
- **A new `RouteAction.ClusterSpecifier` arm** in `buildRouterAction` (`RouteAction_WeightedClusters` — the SECOND arm after `RouteAction_Cluster`).
- **A per-action `newPCGRNG()` RNG** (the `randomLB` model lifted to the router layer — the FIRST router-layer per-request RNG; mutex-guarded; boot-fail on crypto/rand).
- **The 38.1 ctx seam REUSED UNCHANGED** (`cluster.WithSubsetMatch`/`subsetMatchFrom` + `cluster.SubsetMatch`/`NewSubsetMatch`/`ScalarsFromStruct`) — the per-entry merged match rides the existing thread; NO `loadBalancer.Pick` change, NO exported-cluster change.
- **ZERO new packages** — the action + selector + producer all live in the existing `internal/filter/http/router` + `internal/filter/hcm`.
- **ZERO new go.mod deps** — `RouteAction_WeightedClusters` + `WeightedCluster` + `WeightedCluster_ClusterWeight` are in the pinned `/envoy v1.32.4` (`go mod tidy -diff` empty — AMEND-WC1).
- **ZERO new stat surface** — observable via the existing per-cluster `upstream_rq_*` (AMEND-WC4).

---

## 5. Proto-field roster (per §11.1 D-WC1)

### 5.1 `RouteAction.weighted_clusters` → `WeightedCluster` (`config/route/v3/route_components.pb.go`)

| Field | Wire | Go type | 38.2 disposition |
|---|---|---|---|
| `RouteAction.weighted_clusters` | `ClusterSpecifier` oneof (`RouteAction_WeightedClusters`) | `*WeightedCluster` | **CONSUMED** — the new `buildRouterAction` arm |
| `WeightedCluster.clusters` | field 1, `[]*ClusterWeight` | `[]*WeightedCluster_ClusterWeight` | **CONSUMED** — PGV `min_items: 1`; envoy-go authors the non-empty reject (§6) |
| `WeightedCluster.total_weight` | field 3, `*UInt32Value` | (deprecated) | **IGNORED** — the client uses the weight SUM (AMEND-WC1); NOT cross-checked |
| `WeightedCluster.runtime_key_prefix` | field 2, `string` | (runtime) | DEFERRED → runtime layer |
| `WeightedCluster.random_value_specifier` (`header_name`) | oneof | `*WeightedCluster_HeaderName` | DEFERRED → header-seeded RNG |

### 5.2 `WeightedCluster.ClusterWeight` (`route_components.pb.go:3998`)

| Field | Wire | Go type | 38.2 disposition |
|---|---|---|---|
| `ClusterWeight.name` | field 1, `string` | `Name` | **CONSUMED** — the literal cluster name; NO PGV `min_len` (removed); envoy-go authors the "name required" reject (§6) |
| `ClusterWeight.weight` | field 2, `*UInt32Value` | `Weight` | **CONSUMED** — PRESENCE-required (envoy-go authors the unset reject); NO range rule; explicit `0` legal |
| `ClusterWeight.metadata_match` | field 3, `*core.Metadata` | `MetadataMatch` | **CONSUMED** — MERGES over `RouteAction.metadata_match` (entry precedence — AMEND-WC5); scalar-only (the 38.1 reject reused) |
| `ClusterWeight.cluster_header` | field 12, `string` | `ClusterHeader` | DEFERRED — REJECT if set (UNSUPPORTED; §6) |
| `ClusterWeight.request_headers_to_add`/`_remove` | fields 4/9 | `[]*HeaderValueOption` / `[]string` | DEFERRED — silent-ignore (anticipated; PLAN confirms) |
| `ClusterWeight.response_headers_to_add`/`_remove` | fields 5/6 | `[]*HeaderValueOption` / `[]string` | DEFERRED — silent-ignore |
| `ClusterWeight.typed_per_filter_config` | field 10 | `map` | DEFERRED |
| `ClusterWeight.host_rewrite_literal` (host_rewrite_specifier) | field 11 | `string` | DEFERRED |

`RouteAction_Cluster` (the existing single-cluster arm, 38.1) is UNTOUCHED. `total_weight` deprecated; `cluster_header`/`random_value_specifier`/`runtime_key_prefix` deferred (§2).

---

## 6. PARSE-REJECT roster (per §11.3 + ADR-0080)

Per ADR-0080: envoy-go pins its OWN byte-stable house wording (parity in OUTCOME with the reference's divergent text — envoy-go does NOT run PGV, §11.1). 38.2 adds SIX producer-side reject arms at `buildWeightedRouterAction` (config-build), all UNIT-level (the hcm config-build matrix — the 38.1 `router-metadata-match-nonscalar` precedent; NO boot-reject fixture — §8.2). Anticipated wordings (the `route action:` house prefix; finalized at the IMPL, D-WC-IMPL-2):

| # | Condition | Anticipated envoy-go wording | Reference (parity in outcome) |
|---|---|---|---|
| 1 | `weighted_clusters.clusters` empty/nil | `route action: weighted_clusters has no clusters` | PGV `value must contain at least 1 item(s)` |
| 2 | A `ClusterWeight` has no `name` (and no supported alt) | `route action: weighted_clusters cluster: name is required` | `At least one of name or cluster_header need to be specified` |
| 3 | A `ClusterWeight` sets `cluster_header` | `route action: weighted_clusters cluster_header is not supported (literal cluster name only)` | (`cluster_header` is accepted upstream — an envoy-go-strict DEPARTURE-reject, the deferred-feature boundary) |
| 4 | A `ClusterWeight.weight` wrapper is UNSET | `route action: weighted_clusters cluster %q: weight is required` | `Field 'weight' is missing …` |
| 5 | The SUM of weights == 0 | `route action: weighted_clusters total weight must be > 0` | `Sum of weights in the weighted_cluster must be greater than 0.` |
| 6 | A `ClusterWeight.name` is not in the cluster manager | `route action: weighted_clusters cluster %q not found` | `route: unknown weighted cluster '<name>'` |
| (7) | A `metadata_match` (route-level or entry) `envoy.lb` value is non-scalar | `router: metadata_match envoy.lb key %q: only scalar values (string, number, bool) are supported` (the 38.1 reject, REUSED) | (accepted upstream — the scalar MVP boundary, an existing recorded departure) |

**Reject precedence** (mirror the reference, AMEND-WC3): name presence → `cluster_header`-unsupported → weight presence → cluster-exists → sum>0 → metadata_match scalar. **NON-rejects** (envoy-go must NOT over-reject — AMEND-WC3): a non-matching `total_weight` (ignored, no reject); an explicit `weight: 0` on an entry (legal when the sum>0); a single-entry weighted list; a large weight (≤ uint32 max); a skewed split; an entry carrying `metadata_match`/`request_headers_to_add` (the latter silent-ignored). The `Cluster.LbPolicy` supported-list + `TestManager_Error_*` are UNTOUCHED (38.2 is a routing-layer producer — it does not touch cluster config). NEW `Field 4` / `Route_DirectResponse` arms UNTOUCHED.

---

## 7. Stat surface — ZERO new stats (per §11.4 D-WC4 + AMEND-WC4)

- **ZERO new stat names.** Surface **1125 → 1125** (UNCHANGED at the IMPL). The live `/stats` scrape found NO `*weighted*` stat and NO per-route/per-vhost weighted-selection counter; the reference itself emits none. `cluster.<name>.max_host_weight` is a pre-existing ENDPOINT host-weight gauge (value 1) UNRELATED to the route weight (it appears on every cluster regardless of weighted routing) — NOT a 38.2 surface.
- **The split is observable via the EXISTING per-cluster counters** — `cluster.<name>.upstream_rq_total` (+ `upstream_rq_2xx`/`upstream_rq_completed`/the class counters), which envoy-go ALREADY emits (phase 06.1) and which `weightedClusterRouteAction` increments on the CHOSEN cluster per request via the unchanged `IncUpstreamRqTotal` in `doH1ClusterAction`/`doH2ClusterAction`. The weighted distribution is the per-cluster `upstream_rq_total` ratio.
- **The `0065` `StatsAsserter` set:** the per-cluster `upstream_rq_total` counts are PER-SIDE distribution-band-asserted (NOT cross-side-equal — the independent RNG, AMEND-WC2); the SUM of per-cluster `upstream_rq_total` across the weighted clusters IS cross-side conservation-equal (= `totalReqs` routed through the weighted route) and `StatsAsserter`-cross-equaled. NO subset-specific stats change (the 38.1 `lb_subsets_*` only move when a chosen cluster has `lb_subset_config` — the composition arm; the `0065` composition arm may cross-equal `lb_subsets_selected` on the subset cluster if its selection count is deterministic, else per-side).

---

## 8. Differential fixture taxonomy (+1: `0065` HTTP cross-side weighted distribution + composition)

Per `reference_differential_fixture_dispatch_constraint`: ONE cross-side dir (NO boot-reject dir — §8.2). Per `reference_differential_asserter_dispatch`: the distribution prong uses the runner's `DistributionAsserter` hook (driver-side, runs on both paths — the `0060`/`0062`/`0063` precedent); the conservation/composition stats prong uses `StatsAsserter` (cross-side path). Per `reference_differential_band_sigma_margin`: the per-side band uses ~4–5σ margins (NOT ~2.5σ) to stay flake-free over ≥20 runs while still biting the breaks. Per `reference_differential_run_selector`: targeted runs use `-run 'TestDifferential/0065'` (NOT `-run '0065'`). Per `reference_fixture_workload_constant_desync`: the band + counts DERIVE from named constants (never literals). Numbering continues from `0064`; re-pinned at IMPL Task 1.

### 8.1 `0065-weighted-clusters` (cross-side; HTTP route `weighted_clusters`; per-side weight-distribution band + composition + conservation)

Chain `[http_connection_manager + router]` on BOTH sides (the `0063`/`0064` shape: reference STRICT_DNS / `host.docker.internal`, subject STATIC / `127.0.0.1`) over **3 clusters** `c_a`/`c_b`/`c_c` each → a distinct **`HTTPEcho`** backend (the `0063`/`0064` backend REUSED; it embeds its idx in the body so the driver tallies which cluster served each request). **NO new BackendKind** (tail STAYS 33). Route table (both sides):
- `/w` → `weighted_clusters: { clusters: [ {name: c_a, weight: W_A}, {name: c_b, weight: W_B}, {name: c_c, weight: W_C} ] }` (the distribution arm; anticipated weights e.g. `{50, 30, 20}` or `{1, 2, 1}` — the IMPL pins for a clean band, D-WC-IMPL-3).
- `/compose` → `weighted_clusters` over 2 clusters where ONE (`c_sub`, a 4-endpoint cluster with `lb_subset_config` `version` selectors — the `0064` shape) carries a `ClusterWeight.metadata_match {envoy.lb:{version:"v1"}}` (the composition arm — requests routed to `c_sub` land ONLY on its v1 endpoints; the 38.1 compose). (The IMPL may fold this into the primary cluster set to keep the fixture small — D-WC-IMPL-3.)
- `/health` → a `direct_response` `inline_string: "OK\n"` (the byte-equiv stream — address-independent → byte-equal cross-side).

**The workload (identical per side):** send **N** (anticipated ≥500 — the band needs n large enough for a tight relative σ; D-WC-IMPL-3) `GET /w` requests; send **M** `GET /compose` requests (the composition arm); then **8 `GET /health`** round-trips (the `direct_response` byte-equiv stream). `totalReqs` DERIVED from the named constants.

**The distribution arm (PER-SIDE band, via `AssertDistribution` on the per-cluster accept counts — BOTH sides independently):**
- **PER-SIDE weight tracking** — over N `/w` requests, each cluster's served count ≈ N · (weight / Σweight) within a ~4–5σ band (σ = √(N·p·(1−p)) per cluster, p = weight/Σweight — AMEND-WC2). The assertion is PER-SIDE (the reference and subject draw INDEPENDENT per-request randoms → the per-request picks differ but the aggregate distributions match the weights on each side; `reference_differential_hash_key_cross_side_infeasible` lineage — cross-side per-request identity is infeasible for a randomized policy). Each side's band is computed from its own N + the configured weights.
- **PER-SIDE conservation** — the per-cluster `/w` counts sum to N on each side.
- **The break-biting margin** — the band must be wide enough to never flake over ≥20 runs (~4–5σ) yet narrow enough that a swapped-weights break (e.g. `{50,30,20}` → `{20,30,50}`) lands WELL outside it (the `reference_differential_band_sigma_margin` discipline; e.g. at N=500, p=0.5, σ≈11.2 → a ~4.5σ band ≈ 250 ± 50).

**The composition arm (DETERMINISTIC SET-membership, BOTH sides):** every `/compose` request routed to `c_sub` lands on a v1-tagged endpoint of `c_sub` (the merged `metadata_match` selects the v1 subset WITHIN the chosen cluster — the 38.1 affinity, conditioned on the weighted selection). The STATIC config is NAT-transparent → set-membership holds TRUE cross-side (against each side's own version→idx map). Asserted on the subset of `/compose` requests that hit `c_sub`.

**The stats prong (cross-side `StatsAsserter`, post-drain):** cross-equal the SUM of `cluster.{c_a,c_b,c_c}.upstream_rq_total` (= N, conservation — cross-stable despite the per-side distribution) + `cluster.<each>.upstream_cx_total`/`upstream_cx_active` (= 0, quiesced) + the `/health` byte-equiv. NOT cross-equaled: the individual per-cluster `upstream_rq_total` (the per-side distribution — band-asserted in the distribution arm, not cross-equaled).

- **Deliberate-break liveness (`-count=1`, `reference_differential_break_protocol_count1`):** (i) **swap the weights** (`{50,30,20}` → `{20,30,50}`) → the per-side distribution band FAILS (`c_a`/`c_c` counts land outside their bands) — the canonical distribution break; (ii) **drop a cluster** from the weighted list → conservation FAILS (the sum ≠ N) AND the dropped cluster's band FAILS; (iii) **drop the `ClusterWeight.metadata_match`** from the `/compose` `c_sub` entry → `/compose` spreads across ALL `c_sub` endpoints → the composition set-membership FAILS; (iv) perturb the conservation `StatsAsserter` sum → the stats prong FAILS. Recorded in driver comments + README per the `0030` lesson. ≥20-run flake check (`reference_fixture_workload_constant_desync` — N/M/totalReqs + the band constants synced; go-test caching defeated with `-count=1`).

### 8.2 NO boot-reject dir (AMEND-WC3)

The SIX new `weighted_clusters` rejects (§6) are PRODUCER-side (config-build) and UNIT-level (the hcm config-build matrix — the 38.1 `router-metadata-match-nonscalar` precedent; envoy-go's parse-reject, not a fixture). So NO cross-side boot-reject dir. Fixture count 66 → **67** (`0065-weighted-clusters` only).

### 8.3 NO new BackendKind + NO new fuzzer (family expectations)

BackendKind tail STAYS **33** (`0065` reuses the `0063`/`0064` `HTTPEcho` — a routing phase exercises WHICH cluster requests land in, not what the backend speaks). Fuzzers STAY **42** — weighted selection decodes no wire bytes (the weights derive from parsed/validated xDS config, not a wire frame). A selection-DISTRIBUTION property test (random weight vectors → the picker's empirical distribution tracks the weights; the picker never panics; a sum-0 vector is rejected upstream of the picker; an explicit-0 entry is never picked) is a strong UNIT-level candidate FOLDED into the selector's `_test.go` — D-WC6 decides a standalone `FuzzWeightedSelect` (anticipated NO — no untrusted wire input). No new conformance harness; h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate (38.2 touches the HTTP route producer + a per-request selector, NOT the h2 framing or the wasm path — the wire path is byte-identical when no `weighted_clusters` route is configured; the full 67-dir differential is the real guard).

---

## 9. Behavior-contract delta (the 38.2 bundle; ADR-0052 atomic landing)

At the IMPL final task, `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains:

- A NEW `### Route action — weighted clusters (weighted_clusters)` subsection: the SECOND `ClusterSpecifier` arm acceptance (a route fans across N clusters; ONE chosen per request by weight); the selection semantics (weighted-RANDOM — a per-request random value in `[0, Σweight)` then a cumulative walk; `total_weight` deprecated/ignored — the client uses the sum; an explicit `weight: 0` entry is never selected; per-action PCG RNG — the `randomLB` model); the `ClusterWeight.metadata_match` merge (merged over the route-level `RouteAction.metadata_match`, entry precedence; composes with the 38.1 subset LB via the unchanged ADR-0239 ctx seam; behavior-neutral on a non-subset cluster); the reject roster (the §6 SIX arms); the deferred surface (`cluster_header`/header-mutation/`typed_per_filter_config`/`host_rewrite`/`runtime_key_prefix`/`random_value_specifier.header_name`/tcp_proxy plane).
- NO new stat names (the `## Stat-name mapping` table is UNCHANGED — the distribution is observable via the existing per-cluster `upstream_rq_*`; surface 1125 → 1125; a NOTE records the zero-delta + the `max_host_weight` non-relation).
- Departure/coverage records: the `cluster_header`-unsupported DEPARTURE-reject (a deferred-feature boundary — accepted upstream); the header-mutation/`typed_per_filter_config`/`host_rewrite` silent-ignore (deferred); the `total_weight` ignore (deprecated upstream — parity); the scalar-only `metadata_match` MVP boundary (the reused `router-metadata-match-nonscalar` reject); the per-side-distribution-band differential posture (cross-side per-request identity infeasible for a randomized policy — the `0060`-band lineage); the ZERO-new-stat-surface finding; NO new fuzzer/BackendKind; the FIRST per-request cluster-selection primitive + the FIRST router-layer per-request RNG.

---

## 10. Per-task structure (~10–13 tasks; PLAN decomposes)

Indicative spine for the 38.2 PLAN (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`):

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fixtures **66** (tail `0064`) + fuzzers **42** + stat surface **1125** + BackendKind tail **33** + DECISIONS tail **ADR-0240** via the canonical recipes; re-pin the as-built anchors (`hcm/config.go buildRouterAction` + the `ClusterSpecifier` switch + `parseRouteSubsetMatch` + `parseRouteHashPolicies`; `hcm/actions.go clusterRouteAction`; `router/router.go H1ClusterAction`/`doH1ClusterAction`/`applyHashKey`/`routerAction`; `router/router_h2.go H2ClusterAction`/`doH2ClusterAction`/`routerActionH2`; `cluster/subset.go SubsetMatch`/`NewSubsetMatch`/`ScalarsFromStruct`; `cluster.go WithSubsetMatch`; `cluster/random.go`+`newPCGRNG`; the `0064` driver) against the IMPL-session tip; PROGRESS.md created | §11 / §3 |
| 2 | The `weightedSelector` + the cumulative-weight build + the `pick()` walk (TDD: a deterministic injected RNG hits each entry boundary; an explicit-0 entry is never picked; a single-entry list always picks 0; the empirical distribution over many draws tracks the weights) | §3.1 / §11.2 |
| 3 | `buildWeightedRouterAction` — the reject roster (the §6 SIX arms: empty/name-required/cluster_header-unsupported/weight-required/sum-0/dangling) (TDD: each reject arm + the accepts — explicit-0, single-entry, large weight, non-matching total_weight) | §6 / §11.3 |
| 4 | The per-entry `metadata_match` merge (route-level ⊕ entry, entry precedence) + the reused non-scalar reject (TDD: route-only, entry-only, both [entry wins on collision], neither [empty match], non-scalar reject) | §3.2 / §11.5 |
| 5 | The `weightedClusterRouteAction` bridge + the `H1WeightedClusterAction`/`H2WeightedClusterAction` constructors + the dispatch reuse (per-request pick → the EXISTING `doH1/H2ClusterAction` with the chosen entry's cluster + subsetMatch + the shared hashPolicies) (TDD: the `routeAction` interface is satisfied; the chosen cluster's `IncUpstreamRqTotal` fires; the subsetMatch threads only when non-empty) | §3.1 / §3.3 |
| 6 | The `buildRouterAction` `RouteAction_WeightedClusters` arm wiring (the SECOND `ClusterSpecifier` case → `buildWeightedRouterAction`; the existing single-cluster arm UNTOUCHED) (TDD: a weighted route builds the weighted action; a single-cluster route still builds `clusterRouteAction`; an unknown specifier still rejects) | §3.2 / §5 |
| 7 | The `0065-weighted-clusters` fixture: config (HCM+router, `/w` weighted route over 3 clusters, `/compose` over the subset cluster, `/health`, both sides) + driver (N `/w` + M `/compose` + 8 `/health`) + the per-side distribution `AssertDistribution` band (~4–5σ per cluster) + the composition set-membership arm + the conservation `StatsAsserter` (sum of per-cluster `upstream_rq_total` = N) + expectations.yaml | §8.1 |
| 8 | Deliberate-break liveness (`-count=1`): swap-weights (distribution), drop-a-cluster (conservation), drop-the-compose-metadata_match (composition), stats-sum-perturb; ≥20-run flake check (`reference_fixture_workload_constant_desync` — N/M/band constants synced) | §8.1 |
| 9 | (optional) a standalone weighted-selection distribution property test (the picker's empirical distribution tracks the weights; no panic; explicit-0 never picked) — D-WC6 decides standalone vs folded into Task 2 | §8.3 |
| 10 | Full differential re-verify (the 66 prior dirs byte-exact through the new `ClusterSpecifier` arm + `0065` green) + `-race -short` + h2spec/proxy-wasm asserted-unaffected; Completion bundle: BEHAVIOR_CONTRACT 38.2 bundle (§9) + the ADR-0241 §Context+§Decision+§Consequences (ADR-0044 in-place; tail → ADR-0241) + the ZERO-stat note (surface 1125 UNCHANGED) + STATE/ROADMAP row 38 (the 38.2 sub-leg record) + the six-gate evidence | §9 / §13 |

The PLAN re-checks the ADR-0045 gate (anticipated NO further split within 38.2); it may merge/split these indicative tasks (e.g. fold Task 2 into Task 5, or split the producer reject roster from the merge).

---

## 11. SPEC-time empirical-pin block (D-WC1..D-WC6 — executed IN-SESSION 2026-06-14)

Parallel-subagent fan-out executed this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-14.** **Reference source corpus:**

1. **The live `envoyproxy/envoy:contrib-v1.37.2` docker image**: a `--mode validate` `weighted_clusters` reject matrix (16 variants — empty/sum-0/weight-unset/name-xor-header/dangling/total-mismatch/explicit-0/single/large/skew/metadata/headers); a live 70/30 two-cluster weighted-route traffic-distribution probe (1500 requests over a docker BRIDGE network per `reference_docker_probe_bridge_network` with two `hashicorp/http-echo` backends; `downstream_rq_total: 1500` + both clusters' `upstream_rq_total` nonzero verified live before any readout was trusted) + a `/stats` name-set scrape (663 lines; `grep -i weight` → only `max_host_weight`).
2. **go-control-plane `/envoy v1.32.4` bindings** at `~/go/pkg/mod/.../envoy@v1.32.4/config/route/v3/route_components.{pb,pb.validate}.go` (`WeightedCluster` `:1069` / `RouteAction_WeightedClusters` `:2276` / `WeightedCluster_ClusterWeight` `:3998`; the PGV `validate` at `:1686` [clusters min_items:1] / `:6593` [ClusterWeight — no weight/name value rule]); `go mod tidy -diff` + `go build ./...` in the SPEC worktree.
3. **Upstream Envoy v1.37.2 route semantics** — `source/common/router/config_impl` `WeightedClusterEntry` (the per-request random-in-`[0, total)` + cumulative walk) + `api/.../route/v3/route_components.proto` (the `total_weight` deprecation; the `metadata_match` merge-with-route-level/entry-precedence doc; the `name`/`cluster_header` mutual-exclusion).
4. **envoy-go codebase** at master tip (the phase-38.1 IMPL squash `4eedf0e` + the docs-only SHA-fill tip): `internal/filter/hcm/{actions,config,route}.go`, `internal/filter/http/router/{router,router_h2}.go`, `internal/cluster/{subset,cluster,random,leastrequest}.go`, `test/fixtures/{0060-lb-random,0063-lb-maglev,0064-lb-subset}/`, `test/differential/fixture/fixture.go` (`DistributionAsserter`/`StatsAsserter`).

### Summary disposition table (6 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D-WC1 (SPEC-BLOCKING) — proto/surface + PGV + tidy | **CONFIRMED + SIMPLIFIED** (`RouteAction_WeightedClusters` oneof; `total_weight` deprecated→ignore; `weight` presence-required no range; `name` no min_len; `metadata_match` merges entry-precedence; ZERO new dep; PGV near-empty — envoy-go authors all rejects) | WC1 |
| §11.2 | D-WC2 (SPEC-BLOCKING) — the selection algorithm | **CONFIRMED LIVE** (per-request weighted-RANDOM — `rng() % Σweight` + cumulative walk; live 70/30 → 69.0% over 1500, binomial ±~11/500; per-side independent → per-side band) | WC2 |
| §11.3 | D-WC3 — the accept/reject matrix | **PINNED LIVE** (SIX code-level rejects: empty/sum-0/weight-unset/name-xor-header/dangling [+cluster_header-unsupported]; `total_weight` mismatch ACCEPTS; explicit-0 ACCEPTS) | WC3 |
| §11.4 | D-WC4 — the stat-surface delta | **CONFIRMED LIVE (ZERO)** (no `*weighted*` stat; observable via the existing per-cluster `upstream_rq_*`; `max_host_weight` unrelated; surface STAYS 1125) | WC4 |
| §11.5 | D-WC5 (SPEC-BLOCKING) — the metadata_match merge + seam reuse | **PINNED** (entry merges over route-level, entry precedence; route-static merge at build; rides the UNCHANGED ADR-0239 `WithSubsetMatch` seam; NO `Pick` change) | WC5 |
| §11.6 | D-WC6 (SPEC-BLOCKING) — the `0065` design + envelope + split | **RESOLVED** (per-side weight-distribution band + composition + conservation; ~200–320 LoC / ~10–13 tasks → single flat 38.2 leg; no FuzzWeightedSelect; ONE ADR-0241) | WC6 |

### 11.1 D-WC1 (SPEC-BLOCKING) — the weighted-cluster surface + PGV: CONFIRMED + SIMPLIFIED

**Proto surface (module cache, v1.32.4):** `RouteAction.weighted_clusters` is the `RouteAction_WeightedClusters` `ClusterSpecifier` oneof member (`route_components.pb.go:2276`) → `*WeightedCluster` (`:1069`). `WeightedCluster`: `Clusters []*WeightedCluster_ClusterWeight` (field 1), `TotalWeight *wrapperspb.UInt32Value` (field 3 — marked `Deprecated`: "the client will use the sum of all cluster weights"), `RuntimeKeyPrefix string` (field 2), `RandomValueSpecifier` oneof → `*WeightedCluster_HeaderName` (`:1095`). `WeightedCluster_ClusterWeight` (`:3998`): `Name string` (field 1), `ClusterHeader string` (field 12 — "Only one of name and cluster_header may be specified"), `Weight *wrapperspb.UInt32Value` (field 2), `MetadataMatch *core.Metadata` (field 3 — "merged with what's provided in `RouteAction.metadata_match`, with values here taking precedence … filter name should be `envoy.lb`"), `RequestHeadersToAdd`/`RequestHeadersToRemove` (fields 4/9), `ResponseHeadersToAdd`/`ResponseHeadersToRemove` (fields 5/6), `TypedPerFilterConfig` (field 10), `host_rewrite_specifier` (`HostRewriteLiteral` member field 11). **PGV (`route_components.pb.validate.go`):** `WeightedCluster.validate` (`:1686`) → `len(GetClusters()) < 1` → `Clusters: value must contain at least 1 item(s)`; `total_weight` recursively validated only (NO value rule). `WeightedCluster_ClusterWeight.validate` (`:6593`) → `weight` recursively validated only (NO `gte`/`lte`); `name` NO `min_len` (REMOVED — proto comment `[#next-major-version: Need to add back the validation rule: (validate.rules).string = {min_len: 1}]`); `cluster_header`/`*_headers_to_remove`/`host_rewrite_literal` carry only `^[^\x00\n\r]*$`; `request/response_headers_to_*` max 1000. **envoy-go does NOT run PGV** (grep `\.Validate()`/`\.ValidateAll()` over `internal/` non-test → EMPTY; it does its OWN parse-reject) → §6 authors every reject. `go mod tidy -diff` exit 0 EMPTY; `go build ./...` OK — **ZERO new go.mod dep**.

### 11.2 D-WC2 (SPEC-BLOCKING) — the per-request selection algorithm: CONFIRMED LIVE

Live 70/30 two-cluster weighted route (`{c_a: 70, c_b: 30}`), 1500 requests over a docker bridge net (two `hashicorp/http-echo` backends echoing "A"/"B"):

| Run | A | B | A% |
|---|---|---|---|
| 1 (500) | 334 | 166 | 66.8% |
| 2 (500) | 356 | 144 | 71.2% |
| 3 (500) | 345 | 155 | 69.0% |
| Cumulative (1500) | 1035 | 465 | 69.0% |

Per-500 A-count spread 334/356/345 around the mean 350 (±~11) = the binomial signature of INDEPENDENT per-request random draws (σ = √(500·0.7·0.3) ≈ 10.25; all within ±2σ). **Verdict: per-request weighted-RANDOM, NOT deterministic round-robin-by-weight** (the latter → near-exact 350/150 every run, σ≈0). Upstream draws a per-request random value in `[0, total_weight)` and walks the cumulative weights (`WeightedClusterEntry`). envoy-go mirrors with a per-action `newPCGRNG()` (the `randomLB` model — `math/rand/v2` PCG seeded from crypto/rand, mutex-guarded): `idx = walk(cumulative, uint32(rng() % uint64(Σweight)))`. **Cross-instance:** different per-request picks, statistically-matching aggregate → the differential asserts the PER-SIDE aggregate split, NEVER per-request equality (the `0060`-band posture).

### 11.3 D-WC3 — the accept/reject matrix: PINNED LIVE

Live `--mode validate` (16 variants). Baseline (2 clusters, `{50,50}`, no total_weight) ACCEPTS. **Rejects** (verbatim reference text; envoy-go matches in OUTCOME with house wording, ADR-0080):
- **empty `clusters: []`** → `WeightedClusterValidationError.Clusters: value must contain at least 1 item(s)` (PGV).
- **sum of weights == 0** → `Sum of weights in the weighted_cluster must be greater than 0.` (code-level).
- **`weight` wrapper UNSET on an entry** → `Field 'weight' is missing …` (presence check — a `UInt32Value`, so explicit `0` is a VALID value [ACCEPTS, variant 16] and only total omission rejects).
- **both `name` and `cluster_header`** → `Only one of name or cluster_header can be specified`.
- **neither `name` nor `cluster_header` (incl. `name: ""`)** → `At least one of name or cluster_header need to be specified`.
- **dangling `name`** (not in the cluster manager) → `route: unknown weighted cluster '<name>'` (config-init, NOT a runtime 404/503).

**Accepts** (envoy-go must NOT over-reject): a non-matching `total_weight` (deprecation-warns, ignored — uses the sum=2 even when total_weight=5); an explicit `weight: 0` (sum>0 from others); a single-entry weighted list; a near-uint32-max weight; a `{1000000, 1}` skew; an entry carrying `metadata_match`/`request_headers_to_add`. **Reject precedence:** name/header presence → weight presence → cluster-exists → sum>0. **envoy-go consequence:** authors ALL rejects (no PGV); the `cluster_header`-set case rejects as UNSUPPORTED (cluster_header DEFERRED — §6 arm 3).

### 11.4 D-WC4 — the stat-surface delta: CONFIRMED LIVE (ZERO new)

Live `/stats` scrape (663 lines): `grep -i weighted` → NOTHING (no `*weighted*` stat); `grep -i weight` → only `cluster.c_a.max_host_weight: 1` / `cluster.c_b.max_host_weight: 1` (an ENDPOINT host-weight gauge present on EVERY cluster, value 1 — UNRELATED to the route weight; would appear with a non-weighted single-cluster route too). NO per-route/per-vhost weighted-selection counter. The split is observable PURELY via the EXISTING per-cluster counters: `cluster.c_a.upstream_rq_total: 1035` / `cluster.c_b.upstream_rq_total: 465` (+ `upstream_rq_2xx`/`upstream_rq_completed` matching) + listener `http.ingress_http.downstream_rq_total: 1500`. **38.2 adds ZERO new stat names; surface STAYS 1125.** The `0065` differential observes the distribution through these existing counters.

### 11.5 D-WC5 (SPEC-BLOCKING) — the metadata_match merge + the seam reuse: PINNED

Per the proto doc (`route_components.pb.go:4030-4035`): `ClusterWeight.metadata_match` "will be merged with what's provided in `RouteAction.metadata_match`, with values here taking precedence." Both STATIC route config → the merge is ONCE at config-build per entry: `merged = ScalarsFromStruct(route.metadata_match.envoy.lb) ⊕ ScalarsFromStruct(entry.metadata_match.envoy.lb)` with ENTRY values overriding on key collision → `cluster.NewSubsetMatch(merged)` stored on the entry. Per request, the chosen entry's merged `SubsetMatch` rides the EXISTING `cluster.WithSubsetMatch` ctx seam (ADR-0239) — NO new seam, NO `loadBalancer.Pick` change, NO per-request fold. Behavior-neutral on a non-subset cluster (the 38.1 leaf widening ignores the ctx match); composes with a subset cluster (selects the subset within the chosen cluster). Non-scalar `envoy.lb` values reject via the existing `parseRouteSubsetMatch` path (the 38.1 `router-metadata-match-nonscalar` reject, reused per entry + route-level). As-built anchors confirmed: `parseRouteSubsetMatch` (`config.go:567`) + `ScalarsFromStruct` (`cluster/subset.go`) + `WithSubsetMatch` (`cluster.go`) + the `doH1ClusterAction` thread (`router.go:606`).

### 11.6 D-WC6 (SPEC-BLOCKING) — the `0065` design + envelope + split: RESOLVED

The `0065-weighted-clusters` design (§8): a cross-side PER-SIDE weight-distribution band arm (`AssertDistribution` over the per-cluster `upstream_rq_total` counts at ~4–5σ per `reference_differential_band_sigma_margin`) + a metadata_match-composition arm (subset affinity within a chosen subset cluster — the 38.1 compose) + cross-side conservation (the sum of per-cluster `upstream_rq_total` = N, cross-equal) + the `/health` byte-equiv stream. Envelope: ~200–320 prod LoC / ~10–13 tasks (§3.0) — UNDER the ADR-0045 gate; **single flat 38.2 leg, NO further split**. No `FuzzWeightedSelect` (no untrusted wire input — a selection-distribution property test is unit-level, folded into the selector test). ONE ADR anticipated (ADR-0241 — the weighted-cluster routing primitive + the metadata_match merge composition; §13). The selection RNG reuses the `randomLB` model (`newPCGRNG`, per-action, mutex-guarded). The PLAN re-checks.

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-WC-IMPL-1** — the file placement (anticipated: the `weightedSelector` + the `H1/H2WeightedClusterAction` constructors in a NEW `internal/filter/http/router/router_weighted.go`; the `weightedClusterRouteAction` bridge + `buildWeightedRouterAction` in `hcm/actions.go`+`config.go` — the `clusterRouteAction`/`buildRouterAction` precedent; OR the selector folded into hcm). Confirm whether the per-entry `routerAction` is pre-built at config-build (no per-request alloc) or constructed per request.
- **D-WC-IMPL-2** — the exact §6 reject wordings (anticipated the `route action: weighted_clusters …` house prefix per the table; no fixture pins them — unit-level) AND the deferred-field disposition (anticipated: `cluster_header` REJECTS [unsupported]; `request/response_headers_to_*` + `typed_per_filter_config` + `host_rewrite` SILENT-IGNORE [accepted-but-inert, orthogonal to selection] — confirm silent-ignore vs reject against a unit test of the live-accepted shape).
- **D-WC-IMPL-3** — the `0065` final constants (the cluster count [anticipated 3 for `/w` + a subset cluster for `/compose`, possibly folded] / the weights [a clean band — e.g. `{50,30,20}`] / N [the band needs n large enough for a tight relative σ + flake-safe at ~4–5σ — anticipated ≥500] / M [the composition arm] / 8 health) + the per-cluster band formula (σ = √(N·p·(1−p)), p = weight/Σweight, ~4–5σ margin) + the deliberate-break protocol (swap-weights / drop-a-cluster / drop-the-compose-metadata_match / stats-sum-perturb) with `-count=1`; ≥20-run flake check; the `reference_fixture_workload_constant_desync` guard (N/M/band constants synced to the driver's hand-rolled expectations).
- **D-WC-IMPL-4** — whether the `/compose` composition arm ships as a separate route in `0065` or folds into the `/w` cluster set (anticipated: a dedicated `/compose` route over a subset cluster, kept small); whether the composition arm's `lb_subsets_selected` is cross-equaled (deterministic selection count) or per-side.
- **D-WC-IMPL-5** — whether a standalone weighted-selection property test ships (Task 9) or folds into Task 2 (anticipated: a small deterministic distribution property test — empirical distribution tracks the weights / no panic / explicit-0 never picked — distinct from the fixture).
- ADR-0045 split-gate FINAL re-check at PLAN (anticipated NO further split within 38.2).

---

## 13. ADR continuity — the ADR-0241 §Context DRAFT (anchored here; the full entry lands at the 38.2 IMPL)

Per the phase-38.2 routing, the DECISIONS.md tail **STAYS ADR-0240 at this SPEC** (counts UNCHANGED). The ADR-0241 §Context is anchored as a DRAFT HERE; the full entry (§Context + §Decision + §Consequences, status PROPOSED → ACCEPTED) lands at the phase-38.2 IMPL per ADR-0044 (DECISIONS tail → ADR-0241; next-free after phase 38.2 ≈ ADR-0242).

**ADR-0241 §Context DRAFT (the weighted-cluster routing primitive + the `ClusterWeight.metadata_match` merge composition):** Phase 38.2 lands `RouteAction.weighted_clusters` routing — the project's FIRST per-request cluster-SELECTION primitive — as the pre-authorized SECOND leg of the phase-38 by-plane split (the 36.1/36.2 precedent). Every prior route resolves a SINGLE `*cluster.Cluster` at config-build (`buildRouterAction` accepts ONLY the `RouteAction_Cluster` `ClusterSpecifier`; `clusterRouteAction` holds one cluster); 38.2 adds the SECOND `ClusterSpecifier` arm (`RouteAction_WeightedClusters` → `*WeightedCluster`): a route fans across N weighted clusters, ONE chosen per request by integer weight. The selection is weighted-RANDOM (LIVE-pinned: a per-request random value in `[0, Σweight)` then a cumulative-weight walk → the chosen entry; `total_weight` is DEPRECATED upstream and IGNORED — the client uses the SUM of per-cluster weights; an explicit `weight: 0` entry is legal but never selected). envoy-go realizes the draw with a per-action `weightedSelector` mirroring `randomLB` (a `newPCGRNG()` — `math/rand/v2` PCG seeded from crypto/rand, mutex-guarded; boot-fail on the crypto/rand error) — the FIRST use of the per-request RNG model OUTSIDE the cluster LB (a router-layer RNG). The new `weightedClusterRouteAction` (the `clusterRouteAction` sibling) holds N entries (each a resolved `*cluster.Cluster` + its merged `SubsetMatch` + its weight), the route-level `hashPolicies` (shared across entries — `RouteAction.hash_policy` is route-scoped, NOT per-weighted-cluster), and the selector; it satisfies the UNCHANGED `routeAction` interface and, per request, picks an entry then runs the EXISTING `doH1ClusterAction`/`doH2ClusterAction` with the chosen entry's cluster + subsetMatch (`IncUpstreamRqTotal` on the chosen cluster, `applyHashKey`, the 38.1 `WithSubsetMatch` thread, `AcquireH1`/`DialH2`) — a SELECTION primitive layered ON the existing per-cluster dispatch, NOT a new dispatch path. EACH `WeightedCluster.ClusterWeight` may carry its OWN `metadata_match` (proto field 3) which MERGES over the route-level `RouteAction.metadata_match` with ENTRY values taking precedence (LIVE-pinned from the proto semantics); because both are STATIC route config, the merge happens ONCE at config-build per entry (`ScalarsFromStruct` ⊕ → `cluster.NewSubsetMatch`) and rides the EXISTING ADR-0239 `cluster.WithSubsetMatch` ctx seam UNCHANGED into the chosen cluster's subset selector — NO new seam, NO `loadBalancer.Pick` change, NO exported-cluster change. The merge is BEHAVIOR-NEUTRAL on a non-subset cluster (the 38.1 leaf widening ignores the ctx match) and COMPOSES with a subset cluster (the merged match selects the subset within the chosen cluster — the full 38.1+38.2 compose). The producer (`buildWeightedRouterAction`) authors SIX parse-reject arms (empty clusters / name-required / cluster_header-unsupported / weight-required / sum-0 / dangling-name) — envoy-go does its OWN parse-reject (no PGV); the rejects are UNIT-level (no boot-reject fixture); the reference's verbatim text differs (parity in OUTCOME, ADR-0080). 38.2 adds ZERO new stat surface (LIVE-pinned: the reference emits no `*weighted*` stat; the split is observable purely through the pre-existing per-cluster `upstream_rq_*` counters incremented on the chosen cluster — surface 1125 → 1125). The DEFERRED surface: `cluster_header` (header-named cluster — REJECTED), `request/response_headers_to_*` + `typed_per_filter_config` + `host_rewrite` (per-weighted header/config mutation — silent-ignored), `runtime_key_prefix` (the runtime layer), `random_value_specifier.header_name` (header-seeded RNG), and the tcp_proxy `weighted_clusters` plane. The differential proof is the cross-side PER-SIDE weight-distribution band (`0065-weighted-clusters` — N `/w` requests over 3 weighted clusters → each cluster's `upstream_rq_total` within a ~4–5σ band per side; the per-side posture because the independent per-request RNG makes cross-side per-request identity infeasible for a randomized policy — the `0060-lb-random` lineage) + a metadata_match-composition arm (subset affinity within a chosen subset cluster) + cross-side conservation (the sum of per-cluster `upstream_rq_total` = N) + the `/health` byte-equiv. NO new fuzzer (no untrusted wire decode — a selection-distribution property test is unit-level) + NO new BackendKind (tail stays 33; the `0063`/`0064` `HTTPEcho` reused) + ZERO new packages + ZERO new go.mod deps.

§Decision/§Consequences bodies land at the phase-38.2 IMPL per ADR-0044. The PLAN/IMPL may surface additional ADRs (anticipated none beyond ADR-0241 for 38.2).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

At the SPEC-DONE commit (ALL counts UNCHANGED at the SPEC — including the DECISIONS tail; they advance at the IMPL):

- stat surface **1125** (→ **1125** at the 38.2 IMPL — ZERO new stats, AMEND-WC4; the FIRST LB-family leg to add NO stat surface).
- differential fixtures **66** (→ **67** at the 38.2 IMPL: `0065-weighted-clusters`; NO boot-reject dir — AMEND-WC3/§8.2).
- fuzzers **42** (→ **42** — NO new fuzzer, deliberate, §8.3).
- BackendKind tail **33** (→ **33** — NO new BackendKind, deliberate, §8.3).
- DECISIONS.md tail **ADR-0240** (STAYS ADR-0240 at this SPEC — the ADR-0241 §Context is a DRAFT in §13; the full entry lands at the 38.2 IMPL per ADR-0044; next-free **ADR-0241**).
- ROADMAP row 38 STAYS `done` (a flat family row — the 38.2 sub-leg record is ADDED at the 38.2 IMPL; NO parent rollup per ADR-0106); the Load-balancing family stays OPEN (3 candidates remain after 38.2 — locality-weighted LB, priority load balancing, panic thresholds; all health-gated).
- spec-document-reviewer gate applies at this SPEC.
- Next → the **phase-38.2 PLAN** (`superpowers:writing-plans` — decompose §10 into bite-sized TDD tasks; FINAL ADR-0045 gate re-check).
