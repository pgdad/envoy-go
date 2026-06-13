# Phase 38 SPEC — `subset` LB (`Cluster.lb_subset_config`): a metadata-match endpoint-subset WRAPPER over the cluster's `lb_policy` child — the FIFTH Load-balancing-family row, the family's FIRST wrapper; the Load-balancing family STAYS OPEN

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (the 38.1 PLAN; SKILL_ROUTING state 2 → 3). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. Phase 38 is a flat Load-balancing-family row WITH a PRE-AUTHORIZED 38.1/38.2 by-plane split (ADR-0106 + ADR-0045; §3.0); this SPEC scopes **38.1** (the HTTP `RouteAction.metadata_match` plane). 38.2 (weighted-cluster routing + `ClusterWeight.metadata_match`) is deferred to its own SPEC/PLAN/IMPL (§2). Phase 38 keeps the Load-balancing family OPEN; 3 candidates remain after 38 (locality-weighted LB, priority load balancing, panic thresholds — all health-gated).

**Goal:** Land `Cluster.lb_subset_config` (`Cluster.LbSubsetConfig`, `envoy.config.cluster.v3`, proto field 22 — AMEND-SS1) — the project's **fifth Load-balancing construct** (after the phase-02 `roundRobin`, phase-34 `leastRequest`, phase-35 `randomLB`, phase-36 `ringHashLB`, phase-37 `maglevLB`) and its **first WRAPPER**. subset LB is **NOT a `Cluster.LbPolicy` enum value** — it is a wrapper triggered by the presence of `lb_subset_config` that partitions the endpoint set into precomputed subsets (one per distinct `envoy.lb` endpoint-metadata value-tuple per selector) and **delegates each subset to a child `loadBalancer` built from the cluster's `lb_policy`**. At pick time it resolves the route's `RouteAction.metadata_match["envoy.lb"]` to a subset and delegates `Pick` to that subset's child; on a miss it applies the cluster-level fallback {NO_FALLBACK / ANY_ENDPOINT / DEFAULT_SUBSET} (AMEND-SS3). In ONE flat 38.1 leg: ZERO new packages (a new `internal/cluster/subset.go` sibling + the `Endpoint.Metadata` field + the `extractEndpoints` capture + the manager wrap-after-switch + the leaf-factory extraction + the seam-input widening in `internal/cluster`; the `RouteAction.metadata_match` producer in the existing `internal/filter/hcm` + `internal/filter/http/router`), ZERO new go.mod deps (the whole surface is in the pinned `/envoy v1.32.4` — AMEND-SS1, `go mod tidy -diff` empty). The differential proof is the STATIC-config NAT-transparent **TRUE cross-side subset-affinity** arm (`0064-lb-subset` — every served backend ∈ the route's expected subset on BOTH sides, the SET-membership property + a fallback arm).

**Architecture:** A new `subsetLB` type (`internal/cluster/subset.go`, same package) wraps a child-LB factory. At construction it enumerates subsets — per configured selector, one subset per distinct value-tuple over the endpoints carrying ALL the selector's `keys` in their `envoy.lb` metadata (AMEND-SS2) — and builds a child `loadBalancer` per subset (and a fallback child for ANY_ENDPOINT / DEFAULT_SUBSET) via the EXTRACTED leaf-LB factory (`buildLeafLB`, the existing `lb_policy` switch lifted to run over an arbitrary endpoint sub-slice). The `loadBalancer.Pick` seam widens with a SECOND pick-input — the resolved subset match — `Pick(hashKey uint64, hasHash bool)` → `Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool)` (AMEND-SS6); the five leaf policies absorb `(match, hasMatch)` as a behavior-neutral widening; `subsetLB.Pick` resolves the match → child → delegates `child.Pick(hashKey, hasHash, …)` (the hashKey passes straight through, so a ring_hash/maglev child routes WITHIN its subset). `cluster.go` gains `WithSubsetMatch`/`subsetMatchFrom` (the `WithHashKey`/`hashKeyFrom` analogue); the HTTP router parses `RouteAction.metadata_match["envoy.lb"]` ONCE at config-build into a proto-free `cluster.SubsetMatch` stored on `clusterRouteAction` (the match is route-static, NOT request-derived — simpler than the `hash_policy` per-request fold) and threads it via `WithSubsetMatch` at dispatch. `Endpoint` grows a `Metadata` field (the parsed `envoy.lb` scalar key→value map — the FIRST per-endpoint dimension beyond `Host:Port`); `extractEndpoints` captures `lbe.GetMetadata().GetFilterMetadata()["envoy.lb"]`. The exported `Cluster` surface stays BYTE-STABLE (OPTION-C — the match rides ctx like the hash key, AMEND-SS6). Four mirrored `lb_subsets_*` cluster stats (`lb_subsets_active` gauge + `lb_subsets_created`/`lb_subsets_selected`/`lb_subsets_fallback` counters) describe the subset machinery (AMEND-SS4; surface 1121 → 1125). NO `Cluster.LbPolicy` reject-text change (subset is not an enum value; AMEND-SS1 — the `CLUSTER_PROVIDED + lb_subset_config` combination the reference rejects is ALREADY rejected by envoy-go's pre-existing unsupported-policy gate, so ZERO new reject arm).

**Tech stack:** Go 1.26.x / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227, @ `sha256:7edd5b0fd763d32c3dfcfd0061f9c2ea63eebd8cdf7f88d974d3adfc99453be8`). go-control-plane **`/envoy v1.32.4`** (ADR-0008 — `Cluster.LbSubsetConfig` + `LbEndpoint.metadata` + `RouteAction.metadata_match` + `core.Metadata` already in the pinned module; **ZERO new go.mod dep**, `go mod tidy -diff` empty — AMEND-SS1). Reuses `internal/cluster/` (the 02/34/35/36/37 Manager + the `extractEndpoints` flatten site + the `lb_policy` switch [EXTRACTED to `buildLeafLB`, then wrapped] + the ADR-0232 `loadBalancer` interface + `noopRelease` + the ADR-0235 `Pick(hashKey, hasHash)` seam + `hashKeyFrom`/`WithHashKey` [EXTENDED with the match pick-input] + the five leaf policies [built per-subset by the factory]), the HTTP route/producer sites (`internal/filter/hcm/{actions.go [clusterRouteAction], config.go [buildRouterAction + the route-build site]}`, `internal/filter/http/router/router.go [the applyHashKey producer the metadata_match producer mirrors]`), the differential harness (the `0062`/`0063` cross-side HTTP-route `StatsAsserter` + `DistributionAsserter` + the `HTTPEcho` backend), upstream Envoy v1.37.2 source (`source/extensions/load_balancing_policies/subset/subset_lb.{h,cc}` + `source/common/protobuf/utility.{h,cc}` [`ValueUtil::equal`/`HashedValue`]) for the algorithm pins. ZERO new packages.

**Authored:** 2026-06-13. **Empirical-pin probe date:** 2026-06-13.

---

## 1. Purpose / Mission

Phase 38 lands `subset` LB, the **FIFTH Load-balancing-family row** and the family's **FIRST wrapper**. Unlike the five incumbent leaf policies (`roundRobin`/`leastRequest`/`randomLB`/`ringHashLB`/`maglevLB` — each a `Cluster.LbPolicy` enum value), `lb_subset_config` is an orthogonal `Cluster` field that, when present, partitions the host set into precomputed subsets and balances each by the cluster's `lb_policy` as the child. Phase 38 is therefore (i) the project's first LB WRAPPER (owning child `loadBalancer`s, not a sixth leaf), (ii) the FIRST LB construct to extend the `Endpoint` type (the new `Metadata` field — the first per-endpoint dimension beyond `Host:Port`), (iii) the SECOND seam BUILD (a new pick-input — the resolved subset match — after ADR-0235's hash key; contrast phases 35/37, which REUSED the seam), and (iv) a TWO-ADR phase (the seam-input/producer ADR-0239 + the policy ADR-0240 — the phase-36 shape, contrast phases 35/37's single-ADR-on-reuse).

This SPEC refines the phase-38 BRAINSTORM (`docs/envoy-go/phases/38-load-balancer-subset/BRAINSTORM.md`, Q0/Q1/Q2) against the AS-BUILT `internal/cluster` package + the §11 D-SS1..D-SS7 empirical pins EXECUTED IN-SESSION (parallel-subagent fan-out) against (1) the live contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (a `--mode validate` `lb_subset_config` reject matrix + a live subset-cluster `/stats` name-set + affinity + 3-way fallback probe on a docker BRIDGE network per `reference_docker_probe_bridge_network`), (2) go-control-plane `/envoy v1.32.4` bindings, and (3) upstream Envoy v1.37.2 source (`source/extensions/load_balancing_policies/subset/` + `source/common/protobuf/utility.{h,cc}`). It anchors the ADR-0239 + ADR-0240 §Context DRAFTs (§13) and CONSUMES the FINAL 38.1/38.2 ADR-0045 split decision (§3.0).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D-SS1..D-SS7 scrape CONFIRMED the BRAINSTORM's wrapper-not-an-enum architecture and SIMPLIFIED two anticipations (the reject surface is EMPTY, not "the lb_subset_config validation set"; the reference stat magnitude carries a 33× artifact envoy-go must NOT mirror). The load-bearing pins, each carried into the relevant §§ below:

- **AMEND-SS1 (D-SS1 — the v1.32.4 subset surface re-pinned; ZERO new dep; ZERO new reject arm).** `Cluster.LbSubsetConfig` is `lb_subset_config` **proto field 22** (`cluster.pb.go:991`); the message carries `fallback_policy` (`Cluster_LbSubsetConfig_LbSubsetFallbackPolicy`: `NO_FALLBACK=0`/`ANY_ENDPOINT=1`/`DEFAULT_SUBSET=2`), `default_subset` (`*structpb.Struct`, field 2), `subset_selectors` (`[]*LbSubsetSelector`, field 3 — each with `keys []string` [field 1], `single_host_per_subset` [field 4, DEFERRED], `fallback_policy` [field 2, the per-selector enum, DEFERRED], `fallback_keys_subset` [field 3, DEFERRED]), and the cluster flags `locality_weight_aware`/`scale_locality_weight`/`panic_mode_any`/`list_as_any`/`metadata_fallback_policy` (ALL DEFERRED, §2). `LbEndpoint.metadata` is `*core.Metadata` **field 3** (`endpoint_components.pb.go:147`); `RouteAction.metadata_match` is `*core.Metadata` **field 4** (`route_components.pb.go:1718`); `core.Metadata.filter_metadata` is `map[string]*structpb.Struct` field 1 (`base.pb.go:819`) — the `envoy.lb` namespace lives here. **PGV imposes NOTHING beyond three `defined_only` enum-range checks** (`cluster.pb.validate.go:2490`/`4208`): empty selector `keys`, empty `subset_selectors`, duplicate selectors, and `default_subset`/`fallback_policy` mismatches are ALL wire-legal. The live `--mode validate` matrix (14 variants) found **EXACTLY ONE reject**: `lb_subset_config` + `lb_policy: CLUSTER_PROVIDED` → `cluster: LB policy CLUSTER_PROVIDED cannot be combined with lb_subset_config` (a config-init/BOOT reject). **But envoy-go already rejects `CLUSTER_PROVIDED` unconditionally** (it is not in the supported-policy list → the `buildCluster` switch `default` fires "unsupported lb_policy CLUSTER_PROVIDED …" before `lb_subset_config` is ever consulted), so the combination is rejected in OUTCOME (parity) and envoy-go adds **ZERO new reject arm**. The `Cluster.LbPolicy` supported-list + `TestManager_Error_UnsupportedLBPolicy` are **UNTOUCHED** (subset is not an enum value — the phase-34/35/36/37 doubly-hit reject-text-retarget lineage stays BROKEN, §6). `go mod tidy -diff` → exit 0, EMPTY; `go build ./...` → OK — **ZERO new go.mod dep**. See §5 / §6 / §11.1.
- **AMEND-SS2 (D-SS2 SPEC-BLOCKING — the subset enumeration + value-equality pinned; scalar-value MVP).** From `subset_lb.cc` `processSubsets`/`extractSubsetMetadata` at v1.37.2: per selector, only hosts carrying ALL the selector's `keys` in `envoy.lb` participate (a host missing any key is excluded from that selector); one subset per distinct value-TUPLE over the keys. Endpoint values and route `metadata_match` values are compared via `Envoy::HashedValue` (`utility.h`) → `ValueUtil::equal` (`utility.cc`): **numbers are double-valued** (`number_value()` — `ProtobufWkt::Value` has a single numeric kind → int `1` and double `1.0` compare EQUAL — a correction to the BRAINSTORM's "number canonicalization" open question: there is no int/double distinction to canonicalize); bool/string/null are typed direct equality; struct/list recurse. Selector precedence is an **EXACT key-set match** (`findSubset` tree walk — a route's `metadata_match` keys must exactly equal some selector's key set; there is NO partial/longest-prefix ranking); the lookup key is the sorted vector of (key → value) pairs (`SubsetMetadata = std::vector<std::pair<std::string, ProtobufWkt::Value>>`, `subset_lb.h`). The MVP matches SCALAR `envoy.lb` values only (string/number/bool); list/struct values + `list_as_any` DEFERRED (§2). See §3.1 / §11.2.
- **AMEND-SS3 (D-SS3 SPEC-BLOCKING — the three cluster-level fallback outcomes pinned LIVE).** With `fallback_policy: NO_FALLBACK`, a route whose `metadata_match` selects no subset (or a route with no `metadata_match` and a null fallback) returns **HTTP 503 `no healthy upstream`** with response-flag **`UH`** (`upstream_rq_503` increments; `lb_subsets_fallback` stays **0** — a no-host is NOT a fallback). With `ANY_ENDPOINT`, the request balances over the FULL host set via the child policy (200; spreads across all backends; `lb_subsets_fallback` increments). With `DEFAULT_SUBSET` + `default_subset: {…}`, the request balances over the `default_subset`-matching hosts (200; `lb_subsets_fallback` increments); a `default_subset` matching zero hosts degrades to NO_FALLBACK behavior (proto doc + source). The fallback TRIGGER: a route with NO `metadata_match` always uses the fallback policy; a route with a `metadata_match` that matches no subset also falls back. envoy-go maps the NO_FALLBACK no-host to its existing `errNoEndpoints` → the dispatch path's 503 (the no-healthy-upstream parity). See §3.1 / §11.3.
- **AMEND-SS4 (D-SS4 — +4 `lb_subsets_*` stats CONFIRMED LIVE; surface 1121 → 1125; the reference active/created carry a 33× artifact envoy-go does NOT mirror).** The reference emits **7** `lb_subsets_*` cluster stats (`active` gauge; `created`/`removed`/`selected`/`fallback`/`fallback_panic` counters; `single_host_per_subset_duplicate` gauge — confirmed via `/stats?format=prometheus` `# TYPE` lines). envoy-go's 38.1 MVP emits **FOUR** — `lb_subsets_active` (gauge), `lb_subsets_created` (counter), `lb_subsets_selected` (counter), `lb_subsets_fallback` (counter) — and DEFERS `removed` (no EDS churn — the host set is static project-wide), `fallback_panic` (`panic_mode_any` deferred), `single_host_per_subset_duplicate` (`single_host_per_subset` deferred). Increment semantics (live-pinned): `selected` +1 per request that matched a subset; `fallback` +1 per request that took the ANY_ENDPOINT/DEFAULT_SUBSET fallback path (they are MUTUALLY EXCLUSIVE per request; both stay 0 for a NO_FALLBACK no-host); `created`/`active` set to the distinct-subset count at build. ⚠️ **The reference's `active`/`created` magnitude is a 33× artifact** (a calibration sweep found `active == created == 33 × distinct-subset-count`); envoy-go pins the SEMANTICALLY-CORRECT distinct-subset count (×1) — an envoy-go-strict DEPARTURE on magnitude (NOT a parity figure). Consequence for the differential: `selected` + `fallback` are request-counted → **cross-side-equal** (the `0064` `StatsAsserter` cross-equals them); `active`/`created` are NOT cross-equaled (the 33× artifact + the distinct-subset-count departure). Stat surface **1121 → 1125** at the IMPL. See §7 / §11.4.
- **AMEND-SS5 (D-SS5 — the per-subset child honors `lb_policy`; affinity + within-subset spread CONFIRMED LIVE; all five children exercisable).** `subset_lb.cc`: each subset (and each fallback subset) is balanced by the cluster's configured `lb_policy` via the child factory (`PrioritySubsetImpl` → `createLoadBalancer`) over the subset's filtered host set — so a ring_hash/maglev child builds its own ring/table over just the subset's hosts and the request hashKey routes WITHIN the subset. Live: `/v1` requests landed ONLY on v1-tagged backends (be-v1a/be-v1b), `/v2` ONLY on v2-tagged (be-v2a/be-v2b), with the ROUND_ROBIN child spreading within each subset — no cross-subset leakage. envoy-go realizes this by EXTRACTING the `lb_policy` switch into a `buildLeafLB(endpoints)` factory the wrapper calls per subset (§3.2). All five children exercisable (the factory builds any policy over a sub-slice); the per-subset build cost is acceptable for the fixture's small subset count. See §3 / §11.5.
- **AMEND-SS6 (D-SS6 — the seam EXTENDS with a SECOND pick-input + a route-static `metadata_match` producer; exported Cluster byte-stable).** `loadBalancer.Pick(hashKey, hasHash)` widens to `Pick(hashKey, hasHash, match SubsetMatch, hasMatch bool)` (ADR-0239 — the seam-BUILD half, parallel to ADR-0235). The five leaf policies (`roundRobin`/`leastRequest`/`randomLB`/`ringHashLB`/`maglevLB`) absorb `(match, hasMatch)` as a behavior-neutral widening; `subsetLB` consumes it. `cluster.go` gains `WithSubsetMatch`/`subsetMatchFrom`; the `Dial`/`AcquireH1` funnels (`cluster.go:232`/`286`) + the direct-pick `PickEndpoint` (`cluster.go:196`) extract the match alongside the hash key and pass both. The producer parses `RouteAction.metadata_match["envoy.lb"]` ONCE at config-build (`buildRouterAction`, `config.go:536`) into a proto-free `cluster.SubsetMatch` (sorted scalar key→value, DEPARTURE-rejecting unsupported value kinds) stored on `clusterRouteAction`; because the match is ROUTE-STATIC (not request-derived like the `hash_policy` header fold), it is threaded VERBATIM per request via `WithSubsetMatch` in `doH1ClusterAction`/`doH1ClusterActionDirect` — NO per-request computation. The exported `Cluster` surface stays BYTE-STABLE (OPTION-C — `Dial`/`AcquireH1`/`PickEndpoint` already take/derive `ctx`). See §3.2 / §4 / §11.6.
- **AMEND-SS7 (D-SS7 — the `0064` design pinned; ~300–450 prod LoC / ~12–15 tasks → single flat 38.1 leg; no FuzzSubsetResolve; ADR-0024 noted).** The `0064-lb-subset` fixture is a STATIC-config NAT-transparent TRUE cross-side subset-affinity arm (the SET-membership property + a fallback arm; §8). The 38.1 production footprint: `subset.go` (the `subsetLB` type + enumeration + match resolution + fallback) ~150–220 + the `Endpoint.Metadata` field + `extractEndpoints` capture ~15 + the `buildLeafLB` extraction + the wrap-after-switch ~40 + the seam-input widening (interface + 5 leaf signatures + the funnels) ~30 + the `SubsetMatch` type + `WithSubsetMatch`/`subsetMatchFrom` ~30 + the producer (`parseRouteSubsetMatch` + the `clusterRouteAction` field + the H1/H2 `ClusterAction` constructors + the dispatch thread) ~50 + the 4 stat registrations ~15 → **~300–450 prod LoC / ~12–15 tasks**, UNDER the ADR-0045 gate (`> ~25 tasks OR > ~1500 LoC`). **Single flat 38.1 leg; NO further split within 38.1; 38.2 (weighted-cluster routing + `ClusterWeight.metadata_match`) pre-authorized to its own SPEC/PLAN/IMPL (§2).** A `FuzzSubsetResolve` is anticipated NOT warranted (the subset metadata is xDS config validated upstream — no untrusted wire input; a subset-enumeration/resolution PROPERTY test is unit-level, folded into `subset_test.go`). ADR-0024 (per-cluster LB-state scope) is NOTED — the `lb_subsets_selected`/`fallback` counters are the FIRST stats a `loadBalancer` increments from its own `Pick` path (injected at register; §7); the per-cluster scope still holds, the wrinkle is recorded in ADR-0240. See §3.0 / §8 / §11.7.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0238** (the maglev policy, ACCEPTED); next-free **ADR-0239**. Per the phase-38 routing (next-prompt + STATE + BRAINSTORM §7), the DECISIONS tail **STAYS ADR-0238 at this SPEC** (counts UNCHANGED at the SPEC); the ADR-0239 (seam-input + producer) + ADR-0240 (policy + `Endpoint.Metadata`) §Context drafts are anchored in §13 and the full DECISIONS.md entries (§Context + §Decision + §Consequences) land at the phase-38.1 IMPL per ADR-0044 (DECISIONS tail → ADR-0240; next-free after phase 38 ≈ ADR-0241). All seven D-SS pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **The weighted-cluster producer plane → 38.2** — `RouteAction.weighted_clusters` routing (a NEW per-request weighted-cluster-selection primitive — the router resolves a SINGLE `*cluster.Cluster` per route today, `buildRouterAction`/`config.go:540` rejects the `weighted_clusters` ClusterSpecifier) + `WeightedCluster.ClusterWeight.metadata_match` (`*core.Metadata`, field 3 — AMEND-SS1). Its own SPEC/PLAN/IMPL; the pre-authorized by-plane split (§3.0).
- **The tcp_proxy `metadata_match` plane** (`TcpProxy.metadata_match`) — one static match per tcp listener (no per-route richness). Deferred entirely.
- **The per-selector fallback machinery** — `LbSubsetSelector.fallback_policy` {NOT_DEFINED/NO_FALLBACK/ANY_ENDPOINT/DEFAULT_SUBSET/KEYS_SUBSET} + `fallback_keys_subset`. The MVP uses the cluster-level fallback only (the per-selector default `NOT_DEFINED` inherits it — faithful for the common case).
- **`single_host_per_subset`** (+ its `lb_subsets_single_host_per_subset_duplicate` gauge). The reference accepts it at any key arity (AMEND-SS1 variants 11–13) but it is a selection optimization — deferred.
- **`locality_weight_aware` / `scale_locality_weight`** — couples to locality-weighted LB (no locality weights yet).
- **`panic_mode_any`** (+ its `lb_subsets_fallback_panic` counter) — couples to panic thresholds / health (no health field).
- **`list_as_any` + list/struct `envoy.lb` values** — the MVP matches SCALAR values only (string/number/bool — AMEND-SS2); list/struct-valued metadata matching deferred (the producer DEPARTURE-rejects non-scalar values, §6.2).
- **`metadata_fallback_policy`** (`METADATA_NO_FALLBACK=0` / `FALLBACK_LIST=1` — AMEND-SS1) — the multi-metadata fallback. Deferred.
- **`lb_subsets_removed` stat** — no EDS / membership churn project-wide (static host set) → the counter would never increment; deferred (D-SS4).
- **The `Cluster.load_balancing_policy` extension point** — the typed-extension config path (shared with ring_hash's/least_request's/random's/maglev's deferred extension path).
- **All other family policies** — locality-weighted LB, priority load balancing, panic thresholds — stay un-implemented; each a future family row, each gated by the absent health-checking boundary.
- **Healthy-host filtering** — the reference filters subsets to healthy hosts upstream of selection; envoy-go has no active health checking (the Upstream-robustness family's territory) → subsets are built over the full endpoint set.

---

## 3. The `subsetLB` wrapper + the seam EXTENSION (ADR-0239 + ADR-0240)

### 3.0 Split disposition — D-SS7 RESOLVED (single flat 38.1 leg; 38.2 pre-authorized)

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 production LoC`. The pre-authorized by-plane valve (BRAINSTORM §1.4) splits phase 38 because the two producer planes couple DIFFERENT subsystems:

| Leg | Scope | Anticipated prod LoC |
|---|---|---|
| **38.1** (this SPEC) | the `subsetLB` wrapper + the `Endpoint.Metadata` dimension + the `buildLeafLB` extraction + the wrap-after-switch + the seam-input widening + the HTTP `RouteAction.metadata_match` producer + the `0064` fixture | **~300–450** |
| **38.2** (own SPEC) | `RouteAction.weighted_clusters` routing (a NEW per-request routing primitive) + `WeightedCluster.ClusterWeight.metadata_match` | its own envelope |

38.1's surface (§1.1 AMEND-SS7):

| Unit | Anticipated production LoC |
|---|---|
| `subset.go`: the `subsetLB` type (enumeration + match resolution + the cluster-level fallback {NO_FALLBACK/ANY_ENDPOINT/DEFAULT_SUBSET} + the injected stat counters) + the `SubsetMatch` value type + the canonical value-equality | ~180–250 |
| `cluster.go`: the `Endpoint.Metadata` field + `WithSubsetMatch`/`subsetMatchFrom` + the funnel widening (`Dial`/`AcquireH1`/`PickEndpoint`) | ~35 |
| `manager.go`: the `buildLeafLB` extraction + the wrap-after-switch + the `parseLbSubsetConfig` parse + the `extractEndpoints` `envoy.lb` capture + the 4 stat registrations | ~70 |
| `loadbalancer.go` + the 5 leaf files: the behavior-neutral `Pick` signature widening | ~15 |
| `hcm/{actions.go,config.go}` + `router/router.go`: the `metadata_match` producer (`parseRouteSubsetMatch` + the `clusterRouteAction.subsetMatch` field + the H1/H2 `ClusterAction` constructors + the dispatch thread) | ~50 |
| The `0064` fixture driver + asserters + unit/property tests | test-side LoC, NOT counted |

Net production **~300–450 LoC, ~12–15 tasks** — BOTH axes UNDER the gate. **Single flat 38.1 leg — NO further split within 38.1** (the seam widening + producer + wrapper are one self-contained whole on the HTTP `metadata_match` plane). The PLAN re-checks the gate per ADR-0045 (anticipated NO further split). Each leg flips `in-progress → done` at its own IMPL six-gate (NO parent rollup per ADR-0106).

### 3.1 The `subsetLB` wrapper (ADR-0240; AMEND-SS2/SS3/SS5)

`internal/cluster/subset.go` (NEW file, same package — the `ringhash.go`/`maglev.go` precedent). Built ONCE at cluster construction when `lb_subset_config` is present (immutable thereafter → resolution is lock-free). Indicative shape (the PLAN/IMPL finalizes):

```go
// subsetLB is a per-cluster metadata-match WRAPPER load balancer mirroring
// Envoy v1.37.2's SubsetLoadBalancer (source/extensions/load_balancing_policies/
// subset/subset_lb.cc). It partitions the endpoint set into precomputed subsets
// (one per distinct envoy.lb value-tuple per selector) and delegates each subset
// to a child loadBalancer built from the cluster's lb_policy. At Pick it resolves
// the route's metadata_match (carried via ctx) to a subset and delegates; on a
// miss it applies the cluster-level fallback. ADR-0240 (the subset policy +
// Endpoint.Metadata); ADR-0239 (the subset-match pick-input — reused via the
// widened Pick); ADR-0232 (the RELEASE half — delegates the child's release).
type subsetLB struct {
	// subsets keyed by the canonical SubsetMatch key string → child LB over that
	// subset's endpoint sub-slice. Built once; immutable.
	subsets map[string]loadBalancer

	// fallback is the cluster-level fallback child: nil for NO_FALLBACK; a child
	// over ALL endpoints for ANY_ENDPOINT; a child over the default_subset match
	// for DEFAULT_SUBSET (nil when the default matches zero hosts → NO_FALLBACK
	// behavior — AMEND-SS3).
	fallback loadBalancer

	// Injected stat counters (Inc'd from Pick — the FIRST LB to touch stats from
	// its own pick path; registered at registerClusterMetrics — §7 / ADR-0240).
	selected *stats.Counter // +1 on a subset hit
	fallbackC *stats.Counter // +1 on the ANY_ENDPOINT/DEFAULT_SUBSET fallback path

	numSubsets int // the distinct-subset count → active gauge + created counter (×1, NOT the reference 33× — AMEND-SS4)
}

var _ loadBalancer = (*subsetLB)(nil)

// Pick resolves the ctx-carried subset match (threaded by cluster.go's funnel)
// to a subset and delegates to its child; on a miss applies the cluster-level
// fallback. hashKey/hasHash pass straight through to the child (a ring_hash/
// maglev subset child routes within its subset). The (match, hasMatch) the leaf
// policies ignore is what THIS wrapper consumes.
func (s *subsetLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) {
	if hasMatch {
		if child, ok := s.subsets[match.key()]; ok {
			s.selected.Inc()
			return child.Pick(hashKey, hasHash, SubsetMatch{}, false)
		}
	}
	// No match (route has no metadata_match) OR the match selected no subset →
	// the cluster-level fallback (AMEND-SS3).
	if s.fallback == nil { // NO_FALLBACK (or DEFAULT_SUBSET matched zero hosts)
		return Endpoint{}, noopRelease, errNoEndpoints // → dispatch 503 no-healthy-upstream / UH
	}
	s.fallbackC.Inc()
	return s.fallback.Pick(hashKey, hasHash, SubsetMatch{}, false)
}
```

- **Enumeration (AMEND-SS2):** for each selector, traverse the endpoints carrying ALL of the selector's `keys` in their `envoy.lb` `Metadata`; group by the value-tuple over those keys; build one child LB per distinct tuple via `buildLeafLB` (§3.2) over that group's endpoint sub-slice. A host may appear in multiple subsets (across selectors / value-tuples). The subset is keyed by the canonical `SubsetMatch` string (sorted key→value pairs). Empty-`keys` selectors are accepted (parity — AMEND-SS1) and produce a single all-endpoints subset keyed by the empty tuple (or are no-ops if no route can match an empty `metadata_match` — PLAN finalizes the empty-keys disposition; D-S38-3).
- **Match resolution (AMEND-SS2):** `SubsetMatch` is the sorted scalar (key→value) vector the producer built from `RouteAction.metadata_match["envoy.lb"]`; `match.key()` is its canonical string; the subset lookup is an EXACT key-set + value match (numbers double-valued, bool/string typed — `valueEqual`, mirroring `ValueUtil::equal`). No partial/longest-prefix ranking (D-SS2).
- **Fallback (AMEND-SS3):** NO_FALLBACK → `fallback == nil` → `errNoEndpoints` (the existing no-healthy-upstream 503/`UH` path); ANY_ENDPOINT → `fallback` = `buildLeafLB(allEndpoints)`; DEFAULT_SUBSET → `fallback` = `buildLeafLB(endpoints matching default_subset)`, or `nil` if the default matches zero hosts. `selected`/`fallback` counters increment per AMEND-SS4 (mutually exclusive; NO_FALLBACK no-host increments NEITHER).
- The empty-cluster guard is defense-in-depth (`buildCluster` already rejects zero-endpoint clusters via `extractEndpoints`).

### 3.2 The seam EXTENSION + the leaf-LB factory + the `metadata_match` producer (ADR-0239; AMEND-SS5/SS6)

**The seam-input widening (the load-bearing design point).** `loadBalancer.Pick(hashKey uint64, hasHash bool)` → `Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool)`. The five leaf policies (`roundRobin`/`leastRequest`/`randomLB`/`ringHashLB`/`maglevLB`) take the two new params as `_ SubsetMatch, _ bool` (behavior-neutral — the ADR-0235 precedent for the hash-key widening). `cluster.go`'s `Dial`/`AcquireH1` funnels (`:232`/`:286`) and `PickEndpoint` (`:196`) extract `match, hasMatch := subsetMatchFrom(ctx)` alongside `hk, ok := hashKeyFrom(ctx)` and call `c.lb.Pick(hk, ok, match, hasMatch)`. `subsetLB.Pick` consumes `(match, hasMatch)`; it delegates `child.Pick(hashKey, hasHash, SubsetMatch{}, false)` (the child is a leaf → ignores the match; the hashKey passes through, so a ring_hash/maglev subset child routes by key WITHIN the subset). The exported `Cluster` surface stays BYTE-STABLE (OPTION-C — the match rides ctx like the hash key; NO exported-signature change). `WithSubsetMatch(ctx, match)` is exported (the producer lives in the router package); `subsetMatchFrom(ctx)` is package-private. ADR-0239 (the seam-BUILD half, parallel to ADR-0235).

**The leaf-LB factory extraction (AMEND-SS5).** The existing `buildCluster` `lb_policy` switch (`manager.go:251–292`) is EXTRACTED into `buildLeafLB(c *clusterv3.Cluster, name string, endpoints []Endpoint) (loadBalancer, error)` — the same switch, parameterized on an arbitrary endpoint sub-slice. `buildCluster` then:

```go
lb, err := buildLeafLB(c, name, endpoints) // the cluster-level child over ALL endpoints
if err != nil {
	return nil, err // CLUSTER_PROVIDED / LOAD_BALANCING_POLICY_CONFIG / etc. reject HERE (parity — AMEND-SS1)
}
if sc := c.GetLbSubsetConfig(); sc != nil {
	lb, err = newSubsetLB(endpoints, sc, name, func(sub []Endpoint) (loadBalancer, error) {
		return buildLeafLB(c, name, sub) // per-subset + fallback children, same lb_policy/cfg
	})
	if err != nil {
		return nil, err
	}
}
cl.lb = lb
```

This is the **wrap-after-switch** (subset wraps whatever child the `lb_policy` switch produced — NOT a new `case`). Because `buildLeafLB` rejects `CLUSTER_PROVIDED` (and every other unsupported policy) BEFORE the wrap, `lb_subset_config + CLUSTER_PROVIDED` rejects in outcome (parity with the reference's combination reject — AMEND-SS1) with ZERO new reject arm.

**The HTTP `metadata_match` producer (AMEND-SS6).** Mirrors the ADR-0237 `hash_policy` producer but RESOLVED-ONCE (route-static, not request-derived):
- `hcm/config.go buildRouterAction` (`:536`) calls `parseRouteSubsetMatch(r.GetMetadataMatch())` → `cluster.SubsetMatch` (read `filter_metadata["envoy.lb"]`; lower the `*structpb.Struct` to a sorted scalar key→value vector; DEPARTURE-reject non-scalar values — §6.2; nil/absent → an empty (no-match) `SubsetMatch`). Stored on `clusterRouteAction.subsetMatch` alongside `hashPolicies`.
- `router.H1ClusterAction`/`H2ClusterAction` widen to `(c, hps, subsetMatch)`; `doH1ClusterAction`/`doH1ClusterActionDirect` thread it: `ctx = cluster.WithSubsetMatch(ctx, a.subsetMatch)` (only when `subsetMatch` is non-empty — empty → ctx unchanged, the LB's no-match fallback). NO per-request computation (the match is route-static — the contrast with `applyHashKey`'s per-request header fold).
- `cluster.SubsetMatch` is an EXPORTED proto-free type: a sorted `[]struct{ Key string; Val subsetValue }` (scalar value union: string/float64/bool) + a `key()` canonical-string method. The router builds it; `subsetLB` resolves against it; `valueEqual` provides the `ValueUtil::equal` semantics (numbers double-valued).

### 3.3 The `Endpoint.Metadata` dimension + `extractEndpoints` capture (ADR-0240)

`Endpoint` (`cluster.go:33`) grows a `Metadata` field — the parsed `envoy.lb` scalar key→value map (the FIRST per-endpoint dimension beyond `Host:Port`):

```go
type Endpoint struct {
	Host string
	Port uint32
	Metadata map[string]subsetValue // the parsed envoy.lb namespace (scalar values); nil when absent
}
```

`extractEndpoints` (`manager.go:517`) captures `lbe.GetMetadata().GetFilterMetadata()["envoy.lb"]` and lowers it to the scalar map (the same `subsetValue` union the producer uses; non-scalar values silently dropped at the endpoint side OR DEPARTURE-rejected — PLAN finalizes per §6.2, anticipated: drop non-scalar endpoint values, since they can never match a scalar-only route match). The flatten of locality groups is otherwise UNCHANGED (locality/weight/priority still discarded — those belong to the locality-weighted/priority candidates). Per `reference_conn_wrap_method_no_promote` (generalized): the new field threads through EVERY `Endpoint` construction site — `extractEndpoints` + any test builders — not just the type definition (the PLAN enumerates the construction sites). `Addr()` is UNCHANGED (`Metadata` is not part of the dial identity — the ring_hash/maglev table keys stay `"IP:PORT"`).

---

## 4. Framework primitives — a seam EXTENSION + a NEW producer plane + a NEW endpoint dimension + 0 new packages + 0 new go.mod deps

Phase 38.1 EXTENDS the LB pick-input seam with a SECOND pick-input (the subset match — ADR-0239; §3.2), adds a NEW HTTP producer plane (`RouteAction.metadata_match` — ADR-0239), and a NEW endpoint dimension (`Endpoint.Metadata` — ADR-0240). ZERO new packages (the `subsetLB` type + the `SubsetMatch` type + the `Endpoint.Metadata` field + the `buildLeafLB` extraction + the wrap-after-switch + the seam-input widening all land in the EXISTING `internal/cluster`: a new `subset.go` FILE; the producer lands in the EXISTING `internal/filter/hcm` + `internal/filter/http/router`). subset is not a filter — no `builtins` registration, no TypeURL factory, no bootstrap blank-import (`clusterv3`/`routev3`/`corev3`/`structpb` already imported — AMEND-SS1). ZERO new go.mod deps (`go mod tidy -diff` empty — AMEND-SS1). ZERO new hash/codec code (subset matching is value-equality over canonicalized scalars — AMEND-SS2).

---

## 5. Proto-field roster (per §11.1 D-SS1)

All from go-control-plane `/envoy v1.32.4`, verified in the module cache this session.

### 5.1 `Cluster.LbSubsetConfig` (`config/cluster/v3/cluster.pb.go`)

| Field | Proto # | Type | 38.1 disposition |
|---|---|---|---|
| `lb_subset_config` | 22 (on `Cluster`) | `*Cluster_LbSubsetConfig` | parsed; presence triggers the wrap |
| `fallback_policy` | 1 | `Cluster_LbSubsetConfig_LbSubsetFallbackPolicy` (NO_FALLBACK=0/ANY_ENDPOINT=1/DEFAULT_SUBSET=2) | all 3 consumed (AMEND-SS3) |
| `default_subset` | 2 | `*structpb.Struct` | consumed (the DEFAULT_SUBSET match) |
| `subset_selectors` | 3 | `[]*Cluster_LbSubsetConfig_LbSubsetSelector` | consumed (`keys` only) |
| `LbSubsetSelector.keys` | 1 | `[]string` | consumed (the selector's metadata keys) |
| `LbSubsetSelector.single_host_per_subset` | 4 | `bool` | DEFERRED (§2) |
| `LbSubsetSelector.fallback_policy` | 2 | per-selector enum (NOT_DEFINED=0/…/KEYS_SUBSET=4) | DEFERRED (§2) |
| `LbSubsetSelector.fallback_keys_subset` | 3 | `[]string` | DEFERRED (§2) |
| `locality_weight_aware` / `scale_locality_weight` / `panic_mode_any` / `list_as_any` / `metadata_fallback_policy` | 4/5/6/7/8 | bool×4 + enum | DEFERRED (§2) |

PGV (`cluster.pb.validate.go:2490`/`4208`): ONLY three `defined_only` enum-range checks (`fallback_policy`, `metadata_fallback_policy`, per-selector `fallback_policy`). NO `min_items` on `keys`/`subset_selectors`, NO cross-field rule. NOTE the Go struct declares `LbSubsetSelector` fields out of wire order (`keys`,`single_host_per_subset`,`fallback_policy`,`fallback_keys_subset` = wire 1,4,2,3) — key off the getter, not source order.

### 5.2 The producer + endpoint surfaces

| Field | Proto | Type | 38.1 disposition |
|---|---|---|---|
| `LbEndpoint.metadata` | field 3 | `*core.Metadata` | the SUBSET SOURCE — `extractEndpoints` captures `filter_metadata["envoy.lb"]` |
| `RouteAction.metadata_match` | field 4 | `*core.Metadata` | the MATCH producer (38.1) — `buildRouterAction` parses `filter_metadata["envoy.lb"]` |
| `WeightedCluster.ClusterWeight.metadata_match` | field 3 | `*core.Metadata` | DEFERRED → 38.2 |
| `core.Metadata.filter_metadata` | field 1 | `map[string]*structpb.Struct` | the `envoy.lb` namespace key |

`RouteAction.weighted_clusters` (the `RouteAction_WeightedClusters` ClusterSpecifier oneof member → `*WeightedCluster`) EXISTS in v1.32.4 but is unimplemented in envoy-go (`buildRouterAction` rejects non-`Cluster` ClusterSpecifiers) → 38.2.

---

## 6. PARSE-REJECT roster (per §11.1 + ADR-0080)

### 6.1 Wording discipline + the EMPTY new-reject surface

Per ADR-0080: 38.1 changes the parse-reject contract MINIMALLY. The live `--mode validate` matrix (AMEND-SS1 / §11.1) found the reference's ONLY `lb_subset_config` reject is the `CLUSTER_PROVIDED + lb_subset_config` combination — which envoy-go ALREADY rejects (CLUSTER_PROVIDED is an unsupported policy; the `buildCluster` switch `default` fires first). So **38.1 adds ZERO new cluster-config reject arm** and the `Cluster.LbPolicy` supported-list + `TestManager_Error_UnsupportedLBPolicy` are **UNTOUCHED** (the phase-34/35/36/37 doubly-hit reject-text-retarget lineage stays BROKEN — subset is not an enum value). This is CLEANER than maglev (which added 2 reject arms) and than the BRAINSTORM's anticipation (which guessed a `lb_subset_config` validation set — empirically there is none; PGV imposes nothing and the reference accepts empty keys / empty selectors / duplicate selectors / fallback-default mismatches).

### 6.2 The ONE new producer-side reject arm (the scalar-value MVP boundary)

- `router-metadata-match-nonscalar` — a `RouteAction.metadata_match["envoy.lb"]` value that is NOT a scalar (a list or struct value) → `router: metadata_match envoy.lb key %q: only scalar values (string, number, bool) are supported` (the MVP scalar boundary — `list_as_any` + list/struct values DEFERRED, §2). An envoy-go-strict DEPARTURE (the reference accepts list/struct values via `list_as_any`/recursive equality); recorded in BEHAVIOR_CONTRACT. The house `router: …` prefix (the `parseRouteHashPolicies` precedent); no fixture pins the exact bytes (a unit test in the hcm config-build matrix). Anticipated; PLAN/IMPL finalizes the exact wording (D-S38-2). The same scalar boundary on the ENDPOINT side (`extractEndpoints`) anticipated as a silent DROP of non-scalar values (a non-scalar endpoint value can never match a scalar-only route match) — PLAN finalizes.

### 6.3 NON-reject dispositions (parity — AMEND-SS1)

- An empty selector `keys: []`, empty `subset_selectors: []`, duplicate selectors, `default_subset` without `fallback_policy: DEFAULT_SUBSET`, `fallback_policy: DEFAULT_SUBSET` without `default_subset`, and `single_host_per_subset` at any key arity: **ACCEPT** (reference parity — the reference accepts all of these at validate). envoy-go must NOT over-reject them; the MVP handles them benignly (empty keys → an all-endpoints subset or no-op; DEFAULT_SUBSET with no/zero-match default → NO_FALLBACK behavior — AMEND-SS3).
- `lb_subset_config` under `lb_policy: RING_HASH`/`MAGLEV`/`LEAST_REQUEST`/`RANDOM`/`ROUND_ROBIN`: **ACCEPT** + wrap (the subset's children are that policy — AMEND-SS5).
- `lb_subset_config` under `lb_policy: CLUSTER_PROVIDED`/`LOAD_BALANCING_POLICY_CONFIG`: **REJECT** via the pre-existing unsupported-policy gate (the `buildLeafLB` switch `default`) — parity in outcome with the reference's combination reject (different message text; envoy-go's is the more fundamental "policy unimplemented"; a recorded coverage note, NOT a departure).

---

## 7. Stat surface — +4 `lb_subsets_*` stats (per §11.4 D-SS4 + AMEND-SS4)

- **FOUR new stat names.** Surface **1121 → 1125** (at the IMPL). The reference emits SEVEN; envoy-go's 38.1 MVP emits four — `cluster.<name>.lb_subsets_active` (gauge), `cluster.<name>.lb_subsets_created` (counter), `cluster.<name>.lb_subsets_selected` (counter), `cluster.<name>.lb_subsets_fallback` (counter) — and DEFERS `lb_subsets_removed` (no membership churn — static host set), `lb_subsets_fallback_panic` (`panic_mode_any` deferred), `lb_subsets_single_host_per_subset_duplicate` (`single_host_per_subset` deferred). Live-pinned types via `/stats?format=prometheus` `# TYPE` lines.
- **Increment semantics (live):** `selected` +1 on a subset hit; `fallback` +1 on the ANY_ENDPOINT/DEFAULT_SUBSET fallback path (mutually exclusive per request; a NO_FALLBACK no-host increments NEITHER). `created` + `active` set to the distinct-subset count at build. `selected`/`fallback` are Inc'd from `subsetLB.Pick` (the FIRST `loadBalancer` to touch stats from its own pick path — the counters are INJECTED at `registerClusterMetrics` via a `*subsetLB` type-assert, the maglev/ring_hash gauge-block precedent, then Inc'd in `Pick`; ADR-0024's per-cluster scope holds, the wrinkle recorded in ADR-0240 / §13).
- **The 33× artifact (DEPARTURE on magnitude — AMEND-SS4).** A calibration sweep found the reference's `lb_subsets_active`/`created` equal `33 × distinct-subset-count` (a fixed build-side multiplier, stable over time). envoy-go pins the SEMANTICALLY-CORRECT distinct-subset count (×1) — recorded as an envoy-go-strict departure (match the semantics, NOT the 33× figure). Consequence: `active`/`created` are NOT cross-equaled in `0064` (the 33× artifact + the ×1 departure); they are UNIT-asserted at the distinct-subset count.
- **The `0064` `StatsAsserter` set (the `0063` precedent — §8):** cross-equal `cluster.<name>.upstream_cx_total` + `upstream_rq_total` (= the routed request total) + `membership_total` (= the endpoint count) + `upstream_cx_active` (= 0, quiesced) + `lb_subsets_selected` (= the matched-route request count) + `lb_subsets_fallback` (= the fallback-route request count). The `selected`/`fallback` cross-equality is the strong subset-specific prong (request-counted → identical on both sides; the STATIC-config NAT-transparent posture makes them cross-stable, unlike the 33×-artifact gauges).

---

## 8. Differential fixture taxonomy (+1: `0064` HTTP cross-side subset affinity + fallback)

Per `reference_differential_fixture_dispatch_constraint`: ONE cross-side dir (NO boot-reject dir — §8.2). Per `reference_differential_asserter_dispatch`: the subset-affinity prong uses the runner's `DistributionAsserter` hook (driver-side, runs on both paths — the `0062`/`0063` precedent); the stats prong uses `StatsAsserter` (cross-side path). Per `reference_differential_run_selector`: targeted runs use `-run 'TestDifferential/0064'` (NOT `-run '0064'`). Numbering continues from `0063`; re-pinned at IMPL Task 1.

### 8.1 `0064-lb-subset` (cross-side; HTTP route `metadata_match`; TRUE cross-side subset affinity + a fallback arm)

Chain `[http_connection_manager + router]` on BOTH sides (the `0063` shape: reference STRICT_DNS / `host.docker.internal`, subject STATIC / `127.0.0.1`) over ONE cluster `c_echo` with `lb_policy: ROUND_ROBIN` (the child) + `lb_subset_config: { fallback_policy: ANY_ENDPOINT, subset_selectors: [ { keys: ["version"] } ] }` + **4 endpoints** tagged with distinct `envoy.lb` metadata (`version: v1` ×2, `version: v2` ×2) on BOTH sides. Route table (both sides): `/v1` → `metadata_match {envoy.lb:{version:"v1"}}`, `/v2` → `metadata_match {envoy.lb:{version:"v2"}}`, `/none` → `metadata_match {envoy.lb:{version:"v9"}}` (matches no subset → the ANY_ENDPOINT fallback arm), `/health` → a `direct_response` `inline_string: "OK\n"` (the byte-equiv stream). Backends: the existing **`HTTPEcho`** backend of `0063` REUSED (it embeds its own idx in the body). **NO new BackendKind** (tail STAYS 33). The driver KNOWS which endpoint idx carries which `version` tag (it builds the bootstrap on both sides), so it can assert SET-membership.

**The workload (identical per side):** for each subset route (`/v1`, `/v2`) send **K=16** `GET` requests; for the fallback route (`/none`) send **K=16** `GET`s; then **8 `GET /health`** round-trips (the `direct_response` byte-equiv stream — `OK\n`, address-independent → byte-equal). `totalReqs` DERIVED from the named constants (`reference_fixture_workload_constant_desync` — never a literal; the cx/rq/`lb_subsets_*` stat expectations track it).

**The affinity + fallback arm (DETERMINISTIC SET-membership, via `AssertDistribution` on the per-backend accept counts — BOTH sides):**
- **BOTH-SIDE subset affinity** — every backend serving a `/v1` request ∈ the v1-tagged endpoint set; every backend serving a `/v2` request ∈ the v2-tagged set (the SET-membership property — 100% deterministic under `metadata_match`; an unsubsetted policy would spread across ALL 4). The STATIC `metadata_match` config is NAT-transparent → this holds TRUE cross-side WITHOUT the host-identity obstacle that constrained maglev/ring_hash (`reference_differential_hash_key_cross_side_infeasible` does NOT bite — the assertion is set-membership against each side's OWN known version→idx map, not cross-side host equality).
- **BOTH-SIDE within-subset spread** — both members of a 2-host subset are hit across K=16 `GET`s (the ROUND_ROBIN child alternates → ≥1 each; robust, no σ-band needed).
- **BOTH-SIDE fallback spread** — `/none` (no matching subset; ANY_ENDPOINT) spreads across ≥2 of the 4 backends (the fallback child is ROUND_ROBIN over ALL endpoints → all 4 hit; assert ≥2 robustly).
- **BOTH-SIDE conservation** — the per-backend counts sum to `totalReqs`.

**The stats prong (cross-side `StatsAsserter`, post-drain):** §7's set — cross-equal `upstream_cx_total` + `upstream_rq_total` (= `totalReqs`) + `membership_total` (= 4) + `upstream_cx_active` (= 0) + `lb_subsets_selected` (= the `/v1`+`/v2` request count = 2·K = 32) + `lb_subsets_fallback` (= the `/none` request count = K = 16). NOT cross-equaled: `lb_subsets_active`/`created` (the 33× reference artifact vs envoy-go's ×1 distinct-subset count — AMEND-SS4; UNIT-asserted instead, value 2 on the subject under this fixture's 2 distinct version subsets).

- **Deliberate-break liveness (`-count=1`, `reference_differential_break_protocol_count1`):** (i) **drop the `metadata_match`** from the `/v1` route → `/v1` requests spread across ALL 4 backends (the no-match fallback) → the affinity leg FAILS (a served backend ∉ the v1 set) — the canonical affinity break; (ii) **misroute the fallback** — give `/none` a `metadata_match {version:"v1"}` → it lands on the v1 subset (not the all-endpoints spread) AND `lb_subsets_fallback` stops incrementing (it becomes a `selected`) → the fallback arm + the stats prong FAIL; (iii) drop a `StatsAsserter` Inc / perturb `lb_subsets_selected`|`fallback` → the stats prong FAILS. Recorded in driver comments + README per the `0030` lesson.

### 8.2 NO boot-reject dir (AMEND-SS1)

There is NO new `lb_subset_config` parse reject (§6 — the only reference reject, `CLUSTER_PROVIDED + lb_subset_config`, is covered by the pre-existing unsupported-policy unit test; the one NEW arm is the producer-side `metadata_match` non-scalar reject, which is unit-level in the hcm config-build matrix). So NO cross-side boot-reject dir. Fixture count 65 → **66** (`0064-lb-subset` only).

### 8.3 NO new BackendKind + NO new fuzzer (family expectations)

BackendKind tail STAYS **33** (`0064` reuses the `0063` `HTTPEcho` — an LB phase exercises WHERE requests land, not what the backend speaks; the phase-34/35/36/37 first recurs even though subset adds a NEW endpoint metadata DIMENSION — the tags live in the bootstrap endpoint config, not the backend process). Fuzzers STAY **42** — subset decodes no wire bytes (the match derives from parsed/validated xDS config, not a wire frame). A subset-ENUMERATION/RESOLUTION property test (random endpoint-metadata sets + selectors → every endpoint lands in the right subsets; the resolver never panics; the fallback fires per policy; value-equality is double-valued for numbers) is a strong UNIT-level candidate FOLDED into `subset_test.go` — D-SS7 decides a standalone `FuzzSubsetResolve` (anticipated NO — no untrusted wire input). No new conformance harness; h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate (subset touches the cluster LB pick + the HTTP route `metadata_match` parse, NOT the h2 framing or the wasm path — the wire path is byte-identical when no `lb_subset_config` cluster is configured; the full 66-dir differential is the real guard).

---

## 9. Behavior-contract delta (the 38.1 bundle; ADR-0052 atomic landing)

At the IMPL final task, `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains:

- A NEW `### Load balancer — subset (lb_subset_config)` subsection: the wrapper-not-an-enum acceptance (presence of `lb_subset_config` wraps the `lb_policy` child); the enumeration semantics (per selector × distinct `envoy.lb` value-tuple; hosts carrying all keys; scalar values only); the value-equality (numbers double-valued — int 1 ≡ 1.0; bool/string typed; exact key-set match); the cluster-level fallback {NO_FALLBACK → 503 `no healthy upstream`/`UH`; ANY_ENDPOINT → full-set spread; DEFAULT_SUBSET → default match, zero-match ≡ NO_FALLBACK}; the per-subset child honoring `lb_policy` (all five children; hashKey routes within a subset); the seam + producer (the widened `Pick`; the route-static `RouteAction.metadata_match` producer); the healthy-set boundary (no health checking → subsets over the full set).
- The 4 `lb_subsets_*` stats added to the `## Stat-name mapping` table (subset-only; `active`/`created` set at build to the distinct-subset count [the 33× reference artifact NOT mirrored — a recorded departure], `selected`/`fallback` Inc'd per request from `subsetLB.Pick`; surface 1121 → 1125).
- Departure/coverage records: the scalar-value-only `metadata_match` MVP boundary (the `router-metadata-match-nonscalar` reject — list/struct values + `list_as_any` deferred); the `active`/`created` 33×-artifact-vs-×1 magnitude departure; the deferred per-selector fallback / `single_host_per_subset` / locality / panic / metadata_fallback flags + the deferred `removed`/`fallback_panic`/`single_host_per_subset_duplicate` stats; the `CLUSTER_PROVIDED + lb_subset_config` reject covered by the pre-existing unsupported-policy gate (parity in outcome, divergent text — a coverage note); the NEW `Endpoint.Metadata` dimension (the first per-endpoint dimension beyond `Host:Port`); NO new fuzzer/BackendKind; the TRUE cross-side subset-affinity SET-membership posture (a strictly cleaner cross-side proof than maglev/ring_hash — STATIC config is NAT-transparent).

---

## 10. Per-task structure (~12–15 tasks; PLAN decomposes)

Indicative spine for the 38.1 PLAN (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`):

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fixtures **65** (tail `0063`) + fuzzers **42** + stat surface **1121** + BackendKind tail **33** + DECISIONS tail **ADR-0238** via the canonical recipes; re-pin the as-built anchors (`loadbalancer.go` interface + `noopRelease`, the 5 leaf `Pick` sites, `cluster.go` `Endpoint`/`Pick` funnel/`hashKeyFrom`/`WithHashKey`, `manager.go` switch + `extractEndpoints` + `registerClusterMetrics`, `hcm/actions.go clusterRouteAction`, `hcm/config.go buildRouterAction`, `router.go applyHashKey`/`H1ClusterAction`, the `0063` driver) against the IMPL-session tip; PROGRESS.md created | §11 / §3 |
| 2 | The `SubsetMatch` value type + `subsetValue` scalar union + `valueEqual` (numbers double-valued; bool/string typed) + `key()` canonical string (TDD: int 1 ≡ 1.0, bool, string, sorted-key canonicalization) | §3.2 / §11.2 |
| 3 | The `Endpoint.Metadata` field + the `extractEndpoints` `envoy.lb` capture (TDD: scalar capture, non-scalar drop, absent → nil; the construction-site sweep per `reference_conn_wrap_method_no_promote`) | §3.3 |
| 4 | The seam-input widening: `loadBalancer.Pick` + the 5 leaf signatures (behavior-neutral) + `WithSubsetMatch`/`subsetMatchFrom` + the `cluster.go` funnel (`Dial`/`AcquireH1`/`PickEndpoint`) (TDD: the leaf policies ignore the match; the funnels thread it) | §3.2 / §11.6 |
| 5 | The `buildLeafLB` extraction (the `lb_policy` switch lifted to run over an arbitrary sub-slice; the cluster-level call unchanged in behavior; CLUSTER_PROVIDED still rejects) (TDD: byte-stable behavior for every existing policy + the reject) | §3.2 / §11.5 |
| 6 | The `subsetLB` enumeration + build: per selector × value-tuple, a child per subset via the factory; the fallback child {NO_FALLBACK nil / ANY_ENDPOINT all / DEFAULT_SUBSET default-match}; the distinct-subset count (TDD: subset membership, multi-key, empty-keys disposition, default-match) | §3.1 / §11.2 |
| 7 | `subsetLB.Pick`: match resolution → child delegate (hashKey passthrough) → fallback {selected/fallback Inc; NO_FALLBACK → errNoEndpoints}; the within-subset spread (TDD: affinity, all-5-children delegate, the 3 fallback outcomes) | §3.1 / §11.3 |
| 8 | Manager acceptance: the `parseLbSubsetConfig` (fallback_policy + default_subset + selectors; the deferred-flag posture) + the wrap-after-switch (`GetLbSubsetConfig()` present → wrap; CLUSTER_PROVIDED rejects via the leaf factory) (TDD: the §6 accept/reject matrix — empty keys/selectors/dups ACCEPT) | §3.2 / §6 |
| 9 | The 4 `lb_subsets_*` registrations: the `*subsetLB` type-assert in `registerClusterMetrics` (active/created Set at build to the ×1 distinct-subset count; selected/fallback counters allocated + INJECTED into the subsetLB) + a boot smoke | §7 |
| 10 | The HTTP `metadata_match` producer: `parseRouteSubsetMatch` (scalar lower + the non-scalar DEPARTURE-reject) + the `clusterRouteAction.subsetMatch` field + the H1/H2 `ClusterAction` constructors + the dispatch thread (`WithSubsetMatch`) (TDD: parse + reject + the route-static thread) | §3.2 / §6.2 |
| 11 | The `0064-lb-subset` fixture: config (HCM+router, `/v1`,`/v2`,`/none`,`/health` routes, the 4-endpoint subset cluster, both sides) + driver (K=16 per route + 8 `/health`) + the affinity/fallback `AssertDistribution` (SET-membership + within-subset spread + fallback spread + conservation) + the `StatsAsserter` prong (cross-equal cx/rq/membership/quiesced + selected/fallback) + expectations.yaml | §8.1 |
| 12 | Deliberate-break liveness (`-count=1`): drop-the-metadata_match (affinity), misroute-the-fallback (fallback + stats), stats-prong drop; ≥20-run flake check (`reference_fixture_workload_constant_desync` — K/totalReqs synced) | §8.1 |
| 13 | (optional) a standalone subset enumeration/resolution property test (no panic; full membership; fallback per policy; value-equality double-valued) — D-SS7 decides standalone vs folded into Task 6/7 | §8.3 |
| 14 | Full differential re-verify (the 65 prior dirs byte-exact through the seam widening + wrap-after-switch + `0064` green) + `-race -short` + h2spec/proxy-wasm asserted-unaffected; Completion bundle: BEHAVIOR_CONTRACT 38.1 bundle (§9) + the ADR-0239 + ADR-0240 §Context+§Decision+§Consequences (ADR-0044 in-place; tail → ADR-0240) + the 4 `lb_subsets_*` stats (surface 1121 → 1125) + STATE/ROADMAP row 38 (38.1 leg) `in-progress → done` (flat family row — NO parent rollup) + the six-gate evidence | §9 / §13 |

The PLAN re-checks the ADR-0045 gate (anticipated NO further split within 38.1); it may merge/split these indicative tasks (e.g. fold Task 2 into Task 6, or split the seam widening from the leaf-factory extraction).

---

## 11. SPEC-time empirical-pin block (D-SS1..D-SS7 — executed IN-SESSION 2026-06-13)

Parallel-subagent fan-out executed this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-13.** **Reference source corpus:**

1. **The live `envoyproxy/envoy:contrib-v1.37.2` docker image**: a 14-variant `--mode validate` `lb_subset_config` reject matrix; a live subset-cluster `/stats` name-set + 3-way fallback + affinity probe on a docker BRIDGE network (`reference_docker_probe_bridge_network`) with four `traefik/whoami` backends (each emitting its identity) + routes carrying `metadata_match` (`upstream_cx_total: 37` / `downstream_cx_rx_bytes_total: 4922` verified live before any readout was trusted).
2. **go-control-plane `/envoy v1.32.4` bindings** at `~/go/pkg/mod/.../envoy@v1.32.4/config/{cluster,endpoint,route,core}/v3/`; `go mod tidy -diff` + `go build ./...` in the SPEC worktree.
3. **Upstream Envoy v1.37.2 source** (via GitHub WebFetch at tag v1.37.2): `source/extensions/load_balancing_policies/subset/subset_lb.{h,cc}` + `source/common/protobuf/utility.{h,cc}` (`ValueUtil::equal`/`HashedValue`) + `api/.../cluster/v3/cluster.proto` + `docs/.../cluster_stats.rst`.
4. **envoy-go codebase** at master tip `01d0d6e` (above the phase-38 BRAINSTORM bundle `fca56f1`): `internal/cluster/{loadbalancer,ringhash,maglev,manager,cluster}.go`, `internal/filter/hcm/{actions,config}.go`, `internal/filter/http/router/router.go`, `test/fixtures/0063-lb-maglev/`.

### Summary disposition table (7 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D-SS1 (SPEC-BLOCKING) — proto/surface + reject contract + tidy | **CONFIRMED + SIMPLIFIED** (field 22; ZERO new dep; ZERO new cluster-reject arm — the only reference reject [CLUSTER_PROVIDED+subset] is pre-covered; supported-list UNTOUCHED) | SS1 |
| §11.2 | D-SS2 (SPEC-BLOCKING) — enumeration + value-equality | **PINNED + CORRECTION** (per-selector × value-tuple; numbers double-valued [int 1 ≡ 1.0 — no canonicalization needed]; exact key-set match) | SS2 |
| §11.3 | D-SS3 (SPEC-BLOCKING) — the 3 fallback outcomes | **CONFIRMED LIVE** (NO_FALLBACK → 503/`UH`, fallback stat 0; ANY_ENDPOINT → full spread; DEFAULT_SUBSET → default match, zero-match ≡ NO_FALLBACK) | SS3 |
| §11.4 | D-SS4 — the stat-surface delta | **CONFIRMED LIVE + DEPARTURE** (+4 `lb_subsets_*`; selected/fallback request-counted cross-equal; active/created carry a 33× reference artifact → envoy-go pins ×1; surface → 1125) | SS4 |
| §11.5 | D-SS5 (SPEC-BLOCKING) — the child-policy interaction | **CONFIRMED LIVE** (per-subset child honors lb_policy; affinity + within-subset spread; all 5 children via the buildLeafLB factory) | SS5 |
| §11.6 | D-SS6 (SPEC-BLOCKING) — the producer + seam | **PINNED** (Pick widens with the match pick-input; the route-static metadata_match producer; exported Cluster byte-stable OPTION-C) | SS6 |
| §11.7 | D-SS7 (SPEC-BLOCKING) — the `0064` design + envelope + split | **RESOLVED** (TRUE cross-side SET-membership affinity + fallback arm; ~300–450 LoC / ~12–15 tasks → single flat 38.1 leg; no FuzzSubsetResolve; ADR-0024 noted) | SS7 |

### 11.1 D-SS1 (SPEC-BLOCKING) — the subset surface + reject contract: CONFIRMED + SIMPLIFIED

**Proto surface (module cache, v1.32.4):** `Cluster.LbSubsetConfig` = `lb_subset_config` proto field **22** (`cluster.pb.go:991`; getter nil on unset). `Cluster_LbSubsetConfig` fields: `FallbackPolicy` (enum `Cluster_LbSubsetConfig_LbSubsetFallbackPolicy`: `NO_FALLBACK=0`/`ANY_ENDPOINT=1`/`DEFAULT_SUBSET=2`, `:321`), `DefaultSubset` (`*structpb.Struct`, field 2), `SubsetSelectors` (`[]*…LbSubsetSelector`, field 3), `LocalityWeightAware`/`ScaleLocalityWeight`/`PanicModeAny`/`ListAsAny` (fields 4–7), `MetadataFallbackPolicy` (enum: `METADATA_NO_FALLBACK=0`/`FALLBACK_LIST=1`, field 8). `LbSubsetSelector`: `Keys []string` (field 1), `SingleHostPerSubset bool` (field 4), `FallbackPolicy` (per-selector enum `NOT_DEFINED=0`/`NO_FALLBACK=1`/`ANY_ENDPOINT=2`/`DEFAULT_SUBSET=3`/`KEYS_SUBSET=4`, field 2), `FallbackKeysSubset []string` (field 3) — **the Go struct declares these out of wire order** (key off the getter). `LbEndpoint.Metadata *core.Metadata` field 3 (`endpoint_components.pb.go:147`). `RouteAction.MetadataMatch *core.Metadata` field 4 (`route_components.pb.go:1718`); `WeightedCluster.ClusterWeight.MetadataMatch` field 3 (`:4035`); `RouteAction_WeightedClusters` ClusterSpecifier oneof EXISTS (38.2). `core.Metadata.FilterMetadata map[string]*structpb.Struct` field 1 (`base.pb.go:819`). **PGV: ONLY three `defined_only` enum-range checks** (`cluster.pb.validate.go:2490`/`4208`) — NO `min_items`, NO cross-field rule. `go mod tidy -diff` exit 0 EMPTY; `go build ./...` OK — **ZERO new go.mod dep**.

**Reject matrix (live `--mode validate`, 14 variants):** the ONLY reject is `lb_subset_config` + `lb_policy: CLUSTER_PROVIDED` → `cluster: LB policy CLUSTER_PROVIDED cannot be combined with lb_subset_config` (a config-init/BOOT reject, `config_validation/server.cc:76` wrapper). ACCEPTED (envoy-go must NOT over-reject): empty selector `keys: []`, empty `subset_selectors: []`, duplicate selectors, `default_subset` without DEFAULT_SUBSET, DEFAULT_SUBSET without `default_subset`, `single_host_per_subset` at any key arity, `lb_subset_config` under RING_HASH/MAGLEV/etc., a route `metadata_match`. **envoy-go consequence:** CLUSTER_PROVIDED is ALREADY unsupported (the `buildLeafLB` switch `default` rejects it before the wrap) → the combination rejects in OUTCOME (parity), ZERO new reject arm, supported-list + `TestManager_Error_UnsupportedLBPolicy` UNTOUCHED.

### 11.2 D-SS2 (SPEC-BLOCKING) — subset enumeration + value-equality: PINNED + CORRECTION

From `subset_lb.cc` (`processSubsets`/`extractSubsetMetadata`/`findSubset`) + `utility.{h,cc}` (`HashedValue`/`ValueUtil::equal`) at v1.37.2:
- **Enumeration:** per selector, only hosts carrying ALL the selector's `keys` in `envoy.lb` participate (a host missing any key → excluded from that selector — `all_kvs.clear(); break;`); one subset per distinct value-TUPLE over the keys; a host may appear in multiple subsets.
- **Value-equality:** endpoint values + route `metadata_match` values wrapped in `Envoy::HashedValue` → `ValueUtil::equal`: **numbers are double-valued** (`ProtobufWkt::Value` has a single numeric kind; `v1.number_value() == v2.number_value()` → int `1` ≡ double `1.0` — ⚠️ CORRECTION to the BRAINSTORM's "number canonicalization (int vs double)" open question: there is no int/double distinction in the value model, nothing to canonicalize beyond using float64); bool/string/null typed direct equality; struct/list recurse (DEFERRED — scalar MVP).
- **Selector precedence:** EXACT key-set match (`findSubset` tree walk — the route's `metadata_match` keys must exactly equal some selector's key set; NO partial/longest-prefix ranking). The lookup key is the sorted (key→value) vector (`SubsetMetadata`).

### 11.3 D-SS3 (SPEC-BLOCKING) — the 3 fallback outcomes: CONFIRMED LIVE

Live probe (4-backend bridge net; the version selector):
- **NO_FALLBACK** + `/none` (no matching subset) → HTTP **503**, body `no healthy upstream`, response-flag **`UH`** (access log `FLAGS=UH CODE=503`), `upstream_rq_503` increments, `lb_subsets_fallback` stays **0** (a no-host is NOT a fallback), `lb_subsets_selected` stays 0.
- **ANY_ENDPOINT** + `/none` → HTTP **200**, spreads across ALL 4 backends, `lb_subsets_fallback` increments per request, `lb_subsets_selected` 0.
- **DEFAULT_SUBSET** (`default_subset: {version:"v1"}`) + `/none` → HTTP **200**, lands ONLY on the v1 subset, `lb_subsets_fallback` increments, `lb_subsets_selected` 0. A `default_subset` matching zero hosts → NO_FALLBACK behavior (proto + source). Source corroboration: NO_FALLBACK → `fallback_subset_ == nullptr` → null host; ANY_ENDPOINT → `subset_any_` (all hosts); DEFAULT_SUBSET → `subset_default_` (the default-match predicate). Fallback TRIGGER: a route with NO `metadata_match` always uses the fallback; a route with a `metadata_match` matching no subset also falls back. envoy-go: NO_FALLBACK no-host → `errNoEndpoints` → the existing 503 no-healthy-upstream path.

### 11.4 D-SS4 — the stat-surface delta: CONFIRMED LIVE (+4) + the 33× artifact

Live `/stats?format=prometheus` (`# TYPE` lines): the reference emits **7** — `lb_subsets_active` (gauge), `lb_subsets_created`/`removed`/`selected`/`fallback`/`fallback_panic` (counters), `lb_subsets_single_host_per_subset_duplicate` (gauge). envoy-go 38.1 emits **4** (`active`/`created`/`selected`/`fallback`); defers `removed` (no churn), `fallback_panic` (panic deferred), `single_host_per_subset_duplicate` (deferred). Increment (live): `selected` +1 per matched request, `fallback` +1 per fallback request (proven mutually-exclusive: a mixed run showed `fallback: 28` + `selected: 10` simultaneously; NO_FALLBACK no-host increments neither). `created`/`active` = the distinct-subset count at build — **but the reference reports `33 × distinct-subset-count`** (calibration: 1 subset→33, 2→66, 3 across 2 selectors→99; stable, not churn). envoy-go pins the SEMANTICALLY-CORRECT ×1 distinct-subset count (a recorded magnitude DEPARTURE; match semantics not the 33× figure). Surface **1121 → 1125**. The `0064` `StatsAsserter` cross-equals `selected`+`fallback` (request-counted, cross-stable) but NOT `active`/`created` (the 33× artifact + the ×1 departure).

### 11.5 D-SS5 (SPEC-BLOCKING) — the child-policy interaction: CONFIRMED LIVE

Source: each subset (+ each fallback subset) is balanced by the cluster's `lb_policy` via the child factory (`PrioritySubsetImpl` → `createLoadBalancer`) over the subset's filtered hosts; a ring_hash/maglev child builds its ring/table over just the subset and the request hash routes WITHIN. Live: `/v1` ×20 → only v1 backends (be-v1a:8/be-v1b:12), `/v2` ×20 → only v2 backends (8/12), no cross-subset leakage, ROUND_ROBIN child spreading within each subset. envoy-go realizes this via the `buildLeafLB(endpoints)` extraction (§3.2) called per subset + per fallback — all five children exercisable over a sub-slice; the small fixture subset count makes per-subset build cost trivial.

### 11.6 D-SS6 (SPEC-BLOCKING) — the producer + seam: PINNED

The seam widens `loadBalancer.Pick(hashKey, hasHash)` → `Pick(hashKey, hasHash, match SubsetMatch, hasMatch bool)` (ADR-0239); the 5 leaf policies absorb `(match, hasMatch)` behavior-neutrally (the ADR-0235 precedent); `subsetLB` consumes it; `cluster.go`'s `Dial`/`AcquireH1`/`PickEndpoint` extract `subsetMatchFrom(ctx)` alongside `hashKeyFrom(ctx)`. The producer parses `RouteAction.metadata_match["envoy.lb"]` ONCE at config-build (`buildRouterAction`) into a proto-free `cluster.SubsetMatch` stored on `clusterRouteAction`; threaded VERBATIM per request via `WithSubsetMatch` (the match is ROUTE-STATIC — NO per-request fold, the contrast with `applyHashKey`). Exported `Cluster` surface byte-stable (OPTION-C — ctx-carried). As-built anchors confirmed: `clusterRouteAction{cluster, hashPolicies}` (`actions.go:201`); `buildRouterAction` → `parseRouteHashPolicies` (`config.go:536/551`); `applyHashKey` + `H1ClusterAction(c, hps)` (`router.go:514/558`); the `Pick` funnels at `cluster.go:196/233/287`.

### 11.7 D-SS7 (SPEC-BLOCKING) — the `0064` design + envelope + split: RESOLVED

The `0064-lb-subset` design (§8): a STATIC-config NAT-transparent TRUE cross-side subset-affinity SET-membership arm (every served backend ∈ the route's expected subset, BOTH sides — strictly cleaner than maglev/ring_hash's host-identity-infeasible modular invariant, because the STATIC `metadata_match` is identical config on both sides and the driver knows each side's version→idx map) + a fallback arm + the cross-side `selected`/`fallback` stats prong + the `/health` byte-equiv stream. Envelope: ~300–450 prod LoC / ~12–15 tasks (§3.0) — UNDER the ADR-0045 gate; **single flat 38.1 leg, NO further split; 38.2 (weighted clusters) pre-authorized to its own SPEC**. No `FuzzSubsetResolve` (no untrusted wire input — a resolution property test is unit-level, folded into `subset_test.go`). ADR-0024 (per-cluster LB-state scope) NOTED: the `lb_subsets_selected`/`fallback` counters are the FIRST stats a `loadBalancer` Inc's from its own `Pick` path (injected at register; the per-cluster scope holds — recorded in ADR-0240). The PLAN re-checks.

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S38-1** — the file placement (anticipated: the `subsetLB` + `SubsetMatch` + `valueEqual` in a NEW `subset.go`; the `buildLeafLB` extraction + the wrap-after-switch + `parseLbSubsetConfig` + the `extractEndpoints` capture + the 4 stat registrations in `manager.go`; the `Endpoint.Metadata` field + `WithSubsetMatch`/`subsetMatchFrom` + the funnel widening in `cluster.go`; the producer in `hcm/config.go`+`actions.go`+`router/router.go` — the `ringhash.go`/ADR-0237 precedent).
- **D-S38-2** — the exact `router-metadata-match-nonscalar` reject wording (anticipated `router: metadata_match envoy.lb key %q: only scalar values (string, number, bool) are supported` — the house prefix; no fixture pins it) AND the endpoint-side non-scalar disposition (anticipated: silent drop in `extractEndpoints`, since a non-scalar endpoint value can never match a scalar-only route match).
- **D-S38-3** — the empty-`keys`-selector + empty-`subset_selectors` disposition (anticipated: accept [parity]; an empty-keys selector → an all-endpoints subset keyed by the empty tuple, or a benign no-op — finalize against a unit test of the live-accepted shape).
- **D-S38-4** — the `lb_subsets_selected`/`fallback` counter INJECTION mechanics (anticipated: allocated in `registerClusterMetrics` via the `*subsetLB` type-assert and stored on the wrapper, Inc'd in `Pick`; confirm the build-before-register ordering [the LB is built in `buildCluster`, the counters allocated in `registerClusterMetrics` — the maglev gauge-Set precedent) is sound for a per-pick counter] and whether ADR-0024 needs an explicit amendment or just the ADR-0240 note.
- **D-S38-5** — the `0064` final constants (4 endpoints / 2 version subsets / K=16 per route / 8 health anticipated) + the deliberate-break protocol (drop-the-metadata_match, misroute-the-fallback, stats-drop) with `-count=1`; ≥20-run flake check; the `reference_fixture_workload_constant_desync` guard.
- **D-S38-6** — whether a standalone subset enumeration/resolution property test ships (Task 13) or folds into Tasks 6/7 (anticipated: a small deterministic property test — full membership / no panic / fallback-per-policy / double-valued equality — distinct from the fixture).
- ADR-0045 split-gate FINAL re-check at PLAN (anticipated NO further split within 38.1).

---

## 13. ADR continuity — the ADR-0239 + ADR-0240 §Context DRAFTs (anchored here; the full entries land at the 38.1 IMPL)

Per the phase-38 routing, the DECISIONS.md tail **STAYS ADR-0238 at this SPEC** (counts UNCHANGED). The ADR-0239 + ADR-0240 §Context are anchored as DRAFTs HERE; the full entries (§Context + §Decision + §Consequences, status PROPOSED → ACCEPTED) land at the phase-38.1 IMPL per ADR-0044 (DECISIONS tail → ADR-0240; next-free after phase 38 ≈ ADR-0241).

**ADR-0239 §Context DRAFT (the subset-match seam-input extension + the `RouteAction.metadata_match` producer — the seam-BUILD half):** Phase 38.1 EXTENDS the LB pick-input seam (the ADR-0235 hash-key seam) with a SECOND pick-input: the resolved subset match. `loadBalancer.Pick(hashKey uint64, hasHash bool)` widens to `Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool)`. The five leaf policies (roundRobin/leastRequest/randomLB/ringHashLB/maglevLB) absorb `(match, hasMatch)` as a behavior-neutral widening (the ADR-0235 precedent for the hash-key widening of the then-four incumbents); only `subsetLB` consumes it (resolving the match to a precomputed subset, then delegating `child.Pick(hashKey, hasHash, …)` with the hashKey passing through so a ring_hash/maglev subset child routes by key WITHIN the subset). `cluster.go` gains `WithSubsetMatch`/`subsetMatchFrom` (the `WithHashKey`/`hashKeyFrom` analogue); the `Dial`/`AcquireH1`/`PickEndpoint` funnels extract the match alongside the hash key and pass both to `Pick`. The exported `Cluster` surface stays BYTE-STABLE (OPTION-C — the match rides ctx; no exported-signature change), validating ADR-0235 §Consequences' "future pick-inputs widen `Pick` behind a byte-stable surface" generality (the SECOND such input). The producer is the HTTP router's `RouteAction.metadata_match["envoy.lb"]`, parsed ONCE at config-build (`buildRouterAction` → `parseRouteSubsetMatch`) into a proto-free `cluster.SubsetMatch` (a sorted scalar key→value vector; list/struct values DEPARTURE-rejected — the scalar MVP) stored on `clusterRouteAction` alongside `hashPolicies`; because the match is ROUTE-STATIC (not request-derived like the `hash_policy` header fold), it is threaded VERBATIM per request via `WithSubsetMatch` with NO per-request computation (the contrast with ADR-0237's `applyHashKey`). This is a seam BUILD → its own ADR, structurally parallel to ADR-0235 (the hash-key seam) and ADR-0237 (the hash-key producer); the policy is the separate ADR-0240. The two-ADR (seam+producer / policy) shape mirrors phase-36 (contrast phases 35/37's single-ADR-on-reuse).

**ADR-0240 §Context DRAFT (the subset LB policy + the `Endpoint.Metadata` dimension — the policy half):** Phase 38.1 lands `Cluster.lb_subset_config` (`Cluster.LbSubsetConfig`, `envoy.config.cluster.v3`, proto field 22) — the project's FIFTH Load-balancing construct and the family's FIRST WRAPPER. Unlike the five incumbent leaf `LbPolicy` enum values, `lb_subset_config` is an orthogonal `Cluster` field; its presence WRAPS whatever child the `lb_policy` switch produced (a wrap-after-switch — NOT a sixth `case`). `subsetLB` mirrors upstream v1.37.2's `SubsetLoadBalancer`: at construction it enumerates subsets (per `subset_selectors[].keys` × distinct `envoy.lb` endpoint-metadata value-tuples — only hosts carrying ALL the selector's keys participate; numbers double-valued [int 1 ≡ 1.0], bool/string typed; scalar values only in the MVP) and builds a child `loadBalancer` per subset via the EXTRACTED `buildLeafLB` factory (the `lb_policy` switch lifted to run over an arbitrary endpoint sub-slice — so all five children are exercisable per subset, honoring the cluster's `lb_policy`). At `Pick` it resolves the route's ctx-carried `metadata_match` (the ADR-0239 seam-input) to a subset via an EXACT key-set lookup and delegates; on a miss it applies the cluster-level fallback {NO_FALLBACK → `errNoEndpoints`/the existing 503 no-healthy-upstream/`UH`; ANY_ENDPOINT → a child over ALL endpoints; DEFAULT_SUBSET → a child over the `default_subset` match, degrading to NO_FALLBACK when the default matches zero hosts}. It REUSES the ADR-0232 release half (the child's release passes through). `Endpoint` grows a `Metadata` field — the parsed `envoy.lb` scalar key→value map captured at `extractEndpoints` — the FIRST per-endpoint dimension beyond `Host:Port` (locality/weight/priority stay discarded — the locality-weighted/priority candidates' territory; `Addr()` is unchanged, so ring_hash/maglev table keys stay `"IP:PORT"`). `cluster.Manager` accepts `lb_subset_config` under any SUPPORTED `lb_policy` (RING_HASH/MAGLEV/LEAST_REQUEST/RANDOM/ROUND_ROBIN) and rejects it under CLUSTER_PROVIDED/LOAD_BALANCING_POLICY_CONFIG via the pre-existing unsupported-policy gate (parity in OUTCOME with the reference's `CLUSTER_PROVIDED cannot be combined with lb_subset_config` reject — divergent message, the policy being unimplemented; ZERO new reject arm, the `LbPolicy` supported-list + `TestManager_Error_UnsupportedLBPolicy` UNTOUCHED — subset is not an enum value, so the phase-34/35/36/37 doubly-hit reject-text-retarget lineage stays broken). The MVP consumes all three cluster-level fallback policies + `default_subset` + the selector `keys`; it DEFERS the per-selector fallback machinery, `single_host_per_subset`, `locality_weight_aware`/`scale_locality_weight`, `panic_mode_any`, `list_as_any` + list/struct values, `metadata_fallback_policy`, and the weighted-cluster (→ 38.2) + tcp_proxy producer planes. Four mirrored `cluster.<name>.lb_subsets_*` stats describe the machinery — `lb_subsets_active` (gauge) + `lb_subsets_created` (counter) Set at build to the distinct-subset count (the reference's 33× magnitude artifact is NOT mirrored — envoy-go pins the semantically-correct ×1 count, a recorded departure) + `lb_subsets_selected` (counter, +1 per subset hit) + `lb_subsets_fallback` (counter, +1 per fallback path); the `selected`/`fallback` counters are the FIRST stats a `loadBalancer` increments from its own `Pick` path (injected at `registerClusterMetrics` via a `*subsetLB` type-assert — the maglev/ring_hash gauge-block precedent; ADR-0024's per-cluster LB-state scope still holds, the new "LB Inc's its own stats" wrinkle recorded here). `lb_subsets_removed`/`fallback_panic`/`single_host_per_subset_duplicate` are DEFERRED (no churn / panic deferred / single_host deferred) — surface 1121 → 1125. The differential proof is the STATIC-config NAT-transparent TRUE cross-side subset-affinity SET-membership arm (`0064-lb-subset` — HTTP routes carrying `metadata_match` over `envoy.lb`-tagged endpoints; every served backend ∈ the route's expected subset on BOTH sides + a fallback arm + cross-side byte-equivalence + the cross-side `selected`/`fallback` stats prong — strictly cleaner than maglev/ring_hash's host-identity-infeasible posture because the STATIC config is NAT-transparent). NO new fuzzer (no wire decode — a resolution property test is unit-level) + NO new BackendKind (tail stays 33; the `0063` `HTTPEcho` reused) + ZERO new packages + ZERO new go.mod deps. Healthy-host filtering happens upstream of subset selection in the reference → with no health checking it degenerates to all-hosts subsets (the Upstream-robustness family's boundary).

§Decision/§Consequences bodies land at the phase-38.1 IMPL per ADR-0044. The PLAN/IMPL may surface additional ADRs (anticipated none for 38.1; 38.2's weighted-cluster routing anticipates ≥1 further ADR at its own SPEC).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

At the SPEC-DONE commit (ALL counts UNCHANGED at the SPEC — including the DECISIONS tail; they advance at the IMPL):

- stat surface **1121** (→ **1125** at the 38.1 IMPL — +4 `lb_subsets_*` stats, AMEND-SS4).
- differential fixtures **65** (→ **66** at the 38.1 IMPL: `0064-lb-subset`; NO boot-reject dir — AMEND-SS1/§8.2).
- fuzzers **42** (→ **42** — NO new fuzzer, deliberate, §8.3).
- BackendKind tail **33** (→ **33** — NO new BackendKind, deliberate, §8.3).
- DECISIONS.md tail **ADR-0238** (STAYS ADR-0238 at this SPEC — the ADR-0239 + ADR-0240 §Context are DRAFTs in §13; the full entries land at the 38.1 IMPL per ADR-0044; next-free **ADR-0239**).
- ROADMAP row 38 STAYS `in-progress` (the 38.1 leg flips `→ done` at the phase-38.1 IMPL six-gate — a flat family row, NO parent rollup per ADR-0106; then 38.2 follows); the Load-balancing family stays OPEN (3 candidates remain after 38).
- spec-document-reviewer gate applies at this SPEC.
- Next → the **phase-38.1 PLAN** (`superpowers:writing-plans` — decompose §10 into bite-sized TDD tasks; FINAL ADR-0045 gate re-check).
