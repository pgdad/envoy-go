# Phase 52 SPEC — `locality-weighted` LB (`Cluster.CommonLbConfig.locality_weighted_lb_config`)

> For agentic workers: this SPEC resolves every BRAINSTORM.md §10 D-LW question (D-LW1..D-LW8) against LIVE evidence from `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227), executed in-session over two parallel probe agents on a Docker bridge network (`reference_docker_probe_bridge_network`). One BRAINSTORM assumption was REFUTED and corrected (§11.2/AMEND-LW2); one was CONFIRMED with the exact formula (§11.3/AMEND-LW3); the anticipated stat-surface delta was REFUTED DOWN to zero (§11.5/AMEND-LW5). The PLAN decomposes §10 into TDD tasks.

**Goal:** land `Cluster.CommonLbConfig.locality_weighted_lb_config` as the SIXTH Load-balancing-family row — a `localityWeightedLB` wrapper that partitions a cluster's endpoints by `LocalityLbEndpoints.locality` identity, weights each locality by its configured `load_balancing_weight` degraded by that locality's live per-host health fraction (scaled by `ClusterLoadAssignment.Policy.overprovisioning_factor`), and delegates to a per-locality child `loadBalancer` built by the existing policy factory — reusing the EXISTING `loadBalancer.Pick` seam with ZERO new parameters.

**Architecture:** a single new file `internal/cluster/locality.go` (the `subset.go` precedent) + two new `Endpoint` fields + an extended `extractEndpoints` + a second wrap-after-switch site in `manager.go`'s `buildCluster`. NO new package, NO new go.mod dependency (confirmed clean, §11.1), NO new producer plane (a cluster-only construct), NO new stat (confirmed, §11.5).

**Tech stack:** unchanged — Go, `internal/cluster`, the existing `clusterHealth`/`hostHealth` model (`health.go`), the existing `newPCGRNG` crypto-seeded `math/rand/v2` PCG idiom (`leastrequest.go:61`, directly REUSED by `random.go` within the same `internal/cluster` package; the SAME idiom is independently DUPLICATED as `newWeightedRNG` in the different-package `internal/filter/http/router/router_weighted.go`, per that file's own doc comment — phase 52's `locality.go` calls the real shared `newPCGRNG`, matching `random.go`'s posture, not the router's duplication).

**Authored:** 2026-07-06.

---

## 1. Purpose / Mission

Phase 52 lands the family's SIXTH LB construct and its FIRST to consume per-host health as a **continuous weight** rather than a binary include/exclude filter. The mechanism: for each distinct locality among a cluster's endpoints, compute an *effective weight* = `configured_weight × min(1, (overprovisioning_factor/100) × healthy_fraction)`, weighted-random-draw a locality by that effective weight, then delegate to a child `loadBalancer` (built by the existing `buildLeafLB` factory) scoped to that locality's endpoints. Cluster-wide panic mode (the EXISTING `clusterHealth.inPanic` gate) fully bypasses this mechanism, falling back to a flat pre-built child spanning every endpoint.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

- **AMEND-LW2 (D-LW2 SPEC-BLOCKING — REFUTATION of the BRAINSTORM's "unset weight defaults to 1" assumption).** ⚠️ The BRAINSTORM (§2.4) assumed an omitted `load_balancing_weight` defaults to `1`. LIVE-PROBED and REFUTED: the reference assigns an omitted-weight locality **ZERO load** (fully excluded) whenever ANY sibling locality in the same `ClusterLoadAssignment` has an explicit weight — confirmed by the go-control-plane proto doc comment itself ("If no weights are specified when locality weighted load balancing is enabled, the locality is assigned no load") AND by a live 900-request test (Locality A weight=2 explicit, Locality B omitted → A received ALL 900 requests, B received 0). Separately, when **every** locality omits the field, the mechanism degrades to a flat round-robin across the FULL endpoint set (a live 900-request test with both localities omitted gave an even ~1:1 split, ~454/446). envoy-go's design UNIFIES both outcomes with ZERO special-casing (§3.1): the proto getter `GetLoadBalancingWeight().GetValue()` already returns `0` for an unset field, so a locality's raw configured weight is captured AS-IS (no default-to-1 substitution); the weighted-draw's cumulative-bucket construction naturally gives a 0-weight locality an EMPTY bucket (the `router_weighted.go:53` "a weight-0 entry has an empty bucket ... never picked" precedent, reused verbatim); and when the SUM of all localities' effective weights is exactly 0 (either because every raw weight is 0, or every nonzero-weight locality is fully unavailable), `Pick` falls back to the SAME flat all-endpoints child used by panic mode — which reproduces the observed "all omitted → flat RR" outcome exactly, with no second code path.
- **AMEND-LW3 (D-LW3 SPEC-BLOCKING — CONFIRMATION of the hypothesized formula, with the overprovisioning factor GENUINELY CONSUMED, not hardcoded).** The BRAINSTORM (§2.4) posed the standard-Envoy-C++-formula as a HYPOTHESIS to verify. LIVE-PROBED and CONFIRMED across 6 data points (100/80/60/40/20/0% healthy on a 2:1-weighted 2-locality cluster) matching the formula `effective_weight = configured_weight × min(1, (OPF/100) × healthy_fraction)` to within <1 percentage point at every point (e.g. 60% healthy: predicted 62.7% share, observed 62.8%; 40% healthy: predicted 52.8%, observed 52.80% EXACT). A direct A/B at 60%-healthy with `overprovisioning_factor` explicitly set to `100` (no plateau) vs the unset default (140, a 71.4% plateau) produced CLEARLY DIFFERENT results (54.0% vs 62.8%) — CONFIRMING envoy-go must actually READ `ClusterLoadAssignment.Policy.overprovisioning_factor` (a `*wrapperspb.UInt32Value`, default 140 when unset/nil) rather than hardcode 140, contrary to the BRAINSTORM's "no approximation acceptable" caution having left this open.
- **AMEND-LW4 (D-LW4 SPEC-BLOCKING — CONFIRMATION of hypothesis (a): panic FULLY bypasses BOTH health-filtering AND locality-weighting).** LIVE-PROBED and CONFIRMED: with both localities degraded to 1/5 healthy each (20% overall, well under the default 50% panic threshold), traffic spread NEAR-EVENLY (46-54 requests per host) across ALL 10 original hosts — healthy AND unhealthy, both localities — with region sums 49.4%/50.6% (NOT the configured 2:1 weight). `lb_healthy_panic` incremented once per `Pick` call while in panic (mirroring the EXISTING `panicInc()` call site inside every leaf policy's own panic branch, e.g. `loadbalancer.go:44-66`). Also reconfirmed: the panic-threshold comparison is STRICT `<` (an EXACT 50% healthy — the `0/5` A-locality data point — did NOT trigger panic), matching the ALREADY-LANDED `clusterHealth.inPanic` (`health.go:165-171`) verbatim — NO change needed there.
- **AMEND-LW5 (D-LW5 — REFUTATION DOWN: the anticipated "small 0-2" stat delta is ZERO).** The BRAINSTORM (§2.8) anticipated a small (0-2) stat-surface delta pending a live probe. LIVE-PROBED and REFUTED DOWN to a confirmed **ZERO**: a full `/stats` scrape (549 lines) of a `locality_weighted_lb_config`-configured, traffic-served cluster found NO dedicated locality-weighted stat group — the only locality/region/zone-named cluster stats in the reference are the PRE-EXISTING, UNRELATED zone-aware-routing counters (`lb_zone_*`, `lb_local_cluster_not_ok` — all read `0` here since zone-aware mode isn't active) plus the standard `membership_*` gauges (unaffected by which locality mode is configured). Locality-weighted LB's only observable effect in the reference is the request-DISTRIBUTION itself, never a dedicated counter. **Stat surface STAYS 1200 (+0)** — phase 52 is a THIRD zero-stat-delta LB-family phase (after the phase-38.2 weighted-clusters leg and the phase-35 random leg), not a "+small" one.
- **AMEND-LW1 (D-LW1 — CONFIRMATION that both planned rejects are genuine envoy-go-strict DEPARTURES, not reference-parity).** LIVE-PROBED: the reference ACCEPTS a cluster configuring BOTH `lb_subset_config` AND `common_lb_config.locality_weighted_lb_config` simultaneously, but SILENTLY OVERRIDES locality weighting once subset load-balancing engages (a live 3:1-weighted, subset-selector-configured cluster produced an EXACT 1:1 split across all subset-matched hosts, ignoring the configured 3:1 weight entirely — `lb_subsets_selected` confirmed subset matching stayed live throughout). Separately, `zone_aware_lb_config` alone is a fully legitimate, independently-working reference feature (confirmed booting and routing correctly). Both confirm the BRAINSTORM's Q1/Q2 rejects (§2.2/§2.6) are DELIBERATE envoy-go-strict departures from the reference's own (silently-confusing) behavior, not gaps in reference support. `go mod tidy` reconfirmed clean (no new dependency).

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail stays **ADR-0268** at this SPEC (docs-only; no ADR body lands until IMPL per ADR-0044). §13 anchors the **ADR-0269 §Context DRAFT** (the SOLE anticipated ADR — the phase-35/37 single-ADR "reuse" shape, confirmed correct: §11.7/D-LW7 confirms ZERO seam change). All 8 BRAINSTORM D-questions (D-LW1..D-LW8) are RESOLVED at this SPEC (§11); none are deferred to PLAN. §12 lists 3 PLAN/IMPL-level (non-empirical) design questions this SPEC leaves open for the PLAN to pin.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

Unchanged from the BRAINSTORM's §8 deferred list, now with live-probe confirmation of WHY each is safe to defer:

- **`zone_aware_lb_config`** — a distinct, fully-functional reference algorithm (LIVE-CONFIRMED, §11.1) requiring the proxy's OWN locality (`Bootstrap.node.locality`, unconsumed project-wide) — EXPLICITLY REJECTED (§6), not silently ignored.
- **Subset-LB composition** (`LbSubsetConfig.locality_weight_aware`/`scale_locality_weight`) — the reference's own behavior when BOTH are configured (silent override, LIVE-CONFIRMED §11.1) is not worth replicating; BOTH-configured is an explicit reject (§6).
- **`LocalityLbEndpoints.priority`/`proximity`** — priority levels stay fully discarded (the separate "priority load balancing" candidate's territory); proximity is unrelated to weighting.
- **`Cluster.CommonLbConfig.update_merge_window`/`ignore_new_hosts_until_first_hc`/`close_connections_on_host_set_change`** — EDS-churn concepts; the project has no dynamic EDS (bootstrap-only static config project-wide). Silently accepted-and-ignored (UNCHANGED posture — these were never read before this phase either; no new reject).
- **`Cluster.CommonLbConfig.consistent_hashing_lb_config`/`override_host_status`** — ring_hash/maglev-specific and health-status-override surfaces, respectively; orthogonal to locality weighting. Silently accepted-and-ignored (unchanged posture).
- **Any locality-specific stat namespace** — CONFIRMED NONE EXISTS (§11.5); nothing to defer here, the anticipated escape valve is moot.
- **The double-declared-locality-identity corner** (two distinct `LocalityLbEndpoints` groups in the SAME `ClusterLoadAssignment` sharing an IDENTICAL `region`/`zone`/`sub_zone` tuple but DIFFERENT `load_balancing_weight` values) — an unusual, essentially degenerate config shape not exercised by either probe; envoy-go's `localityWeightedLB` construction resolves it via last-write-wins in endpoint-encounter order (a documented, self-answered PLAN-level choice, §12).
- **All other Load-balancing-family candidates** — priority load balancing, panic thresholds — stay unimplemented; each a future family row.

---

## 3. The `localityWeightedLB` construct (ADR-0269)

### 3.0 Split disposition — D-LW8 RESOLVED (single flat row; NO escape valve)

Anticipated **~220–320 prod LoC / ~10–11 tasks** (re-confirmed at SPEC-detail level below) — comfortably under the ADR-0045 `>~25 tasks OR >~1500 LoC` gate. NO pre-authorized split (the phase-37 maglev precedent: a single flat row reusing an existing seam, no second producer plane or subsystem to couple against).

### 3.1 The `localityWeightedLB` wrapper (ADR-0269; AMEND-LW2/LW3/LW4)

Indicative shape (the PLAN/IMPL finalizes exact naming/errors):

```go
// internal/cluster/locality.go

// LocalityID is the proto-free (region, zone, sub_zone) identity captured from
// LocalityLbEndpoints.locality. The zero value is a valid single group (an
// endpoint with no locality set at all).
type LocalityID struct {
	Region, Zone, SubZone string
}

// localityGroup is one distinct locality's endpoint sub-slice + its RAW
// configured weight (0 when load_balancing_weight was unset — AMEND-LW2; NO
// default-to-1 substitution) + its factory-built child loadBalancer.
type localityGroup struct {
	id        LocalityID
	endpoints []Endpoint // this locality's own sub-slice, for per-locality health aggregation
	weight    uint32     // the RAW captured weight; 0 is a valid, meaningful "no load" value
	child     loadBalancer
}

// localityWeightedLB partitions a cluster's endpoints by Locality identity and
// weighted-random-draws a locality per Pick, scaled by that locality's live
// healthy fraction (AMEND-LW3's confirmed formula), delegating to a per-locality
// child built by the existing policy factory. Built ONCE at cluster construction
// (locality membership + configured weights never change post-boot — no dynamic
// EDS project-wide); Pick recomputes the per-locality EFFECTIVE weight (health-
// derived) on every call, since health state changes live (ADR-0243 precedent —
// every leaf policy already recomputes its own health view per Pick).
//
// REUSES the loadBalancer interface UNCHANGED — ZERO new Pick parameters
// (D-LW7): hashKey/match pass straight through to the chosen locality's child,
// exactly as subsetLB already forwards to ITS children (subset.go:213-230).
type localityWeightedLB struct {
	groups                 []localityGroup
	flat                    loadBalancer // panic-mode + zero-total-weight fallback; spans ALL endpoints
	allEndpoints            []Endpoint
	health                  *clusterHealth // never nil in practice (see manager.go note in §3.3); nil-guarded defensively
	overprovisioningFactor  uint32         // AMEND-LW3: from ClusterLoadAssignment.Policy.overprovisioning_factor; 140 if 0/unset
	rng                     func() uint64  // the newPCGRNG (leastrequest.go:61) idiom — injectable for deterministic tests
}

var _ loadBalancer = (*localityWeightedLB)(nil)

// newLocalityWeightedLB groups endpoints by Locality identity (encounter
// order), builds one child per locality + one flat fallback child (both via
// the caller-bound factory — the buildLeafLB closure, the subsetLB precedent),
// and seeds a crypto-keyed PCG draw. A duplicate locality identity across
// multiple source LocalityLbEndpoints groups is resolved last-write-wins on
// weight (§12 D-LW-DUP, an untested corner).
func newLocalityWeightedLB(endpoints []Endpoint, health *clusterHealth, overprovisioningFactor uint32, factory leafFactory) (*localityWeightedLB, error) {
	if overprovisioningFactor == 0 {
		overprovisioningFactor = 140 // AMEND-LW3: the reference's own default
	}
	rng, err := newPCGRNG() // leastrequest.go:61 — crypto-seeded math/rand/v2 PCG
	if err != nil {
		return nil, err
	}
	byLocality := map[LocalityID][]Endpoint{}
	weights := map[LocalityID]uint32{}
	var order []LocalityID
	for _, ep := range endpoints {
		if _, seen := byLocality[ep.Locality]; !seen {
			order = append(order, ep.Locality)
		}
		byLocality[ep.Locality] = append(byLocality[ep.Locality], ep)
		weights[ep.Locality] = ep.LocalityWeight // last-write-wins (§12 D-LW-DUP)
	}
	lw := &localityWeightedLB{allEndpoints: endpoints, health: health, overprovisioningFactor: overprovisioningFactor, rng: rng}
	for _, id := range order {
		members := byLocality[id]
		child, err := factory(members)
		if err != nil {
			return nil, err
		}
		lw.groups = append(lw.groups, localityGroup{id: id, endpoints: members, weight: weights[id], child: child})
	}
	flat, err := factory(endpoints)
	if err != nil {
		return nil, err
	}
	lw.flat = flat
	return lw, nil
}

// Pick: cluster-wide panic (AMEND-LW4) bypasses locality weighting entirely,
// delegating to the flat child. Otherwise compute each locality's EFFECTIVE
// weight (AMEND-LW3's confirmed formula), weighted-random-draw a locality
// (a float64 cumulative-bucket walk — the router_weighted.go integer-weight
// idiom generalized to continuous weights, since effective weight is
// health-fraction-scaled), and delegate. A zero TOTAL effective weight (every
// locality's raw weight is 0 — AMEND-LW2's "all omitted" case — or every
// nonzero-weight locality is fully unavailable) falls back to the flat child,
// unifying BOTH observed AMEND-LW2 outcomes with the SAME fallback path.
func (lw *localityWeightedLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) {
	if lw.health != nil && lw.health.inPanic(lw.allEndpoints) {
		lw.health.panicInc()
		return lw.flat.Pick(hashKey, hasHash, match, hasMatch)
	}
	type bucket struct {
		cum   float64
		child loadBalancer
	}
	buckets := make([]bucket, 0, len(lw.groups))
	var total float64
	for _, g := range lw.groups {
		frac := 1.0
		if lw.health != nil && len(g.endpoints) > 0 {
			frac = float64(lw.health.availableCount(g.endpoints)) / float64(len(g.endpoints))
		}
		availability := math.Min(1.0, (float64(lw.overprovisioningFactor)/100.0)*frac)
		eff := float64(g.weight) * availability
		if eff > 0 {
			total += eff
			buckets = append(buckets, bucket{cum: total, child: g.child})
		}
	}
	if total <= 0 {
		return lw.flat.Pick(hashKey, hasHash, match, hasMatch) // AMEND-LW2 unification
	}
	r := (float64(lw.rng()) / float64(math.MaxUint64)) * total
	for _, b := range buckets {
		if r < b.cum {
			return b.child.Pick(hashKey, hasHash, match, hasMatch)
		}
	}
	return buckets[len(buckets)-1].child.Pick(hashKey, hasHash, match, hasMatch) // float rounding guard
}
```

### 3.2 The seam REUSE — ZERO new `Pick` parameters (D-LW7 CONFIRMED)

`loadBalancer.Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error)` (`loadbalancer.go:16-23`) stays BYTE-FOR-BYTE unchanged. `localityWeightedLB.Pick` forwards all four parameters UNCHANGED to whichever child it delegates to (a locality's child, or the flat fallback) — the identical forwarding shape `subsetLB.Pick` already uses (`subset.go:213-230`). NO `WithX`/`xFrom` ctx-carry is added (contrast ADR-0235's `WithHashKey`/ADR-0239's `WithSubsetMatch`) — locality selection needs no per-request producer input, resolving entirely from per-cluster build-time state (`Endpoint.Locality`/`Endpoint.LocalityWeight`) plus the EXISTING `clusterHealth` consulted via a NEW per-locality aggregation read (`ch.availableCount(g.endpoints)` — an EXISTING method, `health.go:142-150`, called with a locality's OWN sub-slice instead of the whole cluster's).

### 3.3 Manager acceptance + the `CommonLbConfig` gate (ADR-0269; AMEND-LW1)

`internal/cluster/manager.go`'s `buildCluster` gains a SECOND wrap-after-switch site, immediately after the existing `lb_subset_config` wrap (`manager.go:461-465`):

```go
lwc := c.GetCommonLbConfig().GetLocalityWeightedLbConfig()
zac := c.GetCommonLbConfig().GetZoneAwareLbConfig()
switch {
case lwc != nil && c.GetLbSubsetConfig() != nil: // re-derives the SAME sc the subset wrap above already checked (a cheap getter, no side effects) — the PLAN may instead hoist a single `sc := c.GetLbSubsetConfig()` above BOTH wrap sites to avoid the double call
	return nil, fmt.Errorf("cluster: %q: lb_subset_config cannot be combined with common_lb_config.locality_weighted_lb_config", name)
case zac != nil:
	return nil, fmt.Errorf("cluster: %q: common_lb_config.zone_aware_lb_config is not supported", name)
case lwc != nil:
	if health == nil {
		health = newClusterHealth(endpoints, parsePanicThreshold(c)) // a locality-weighted cluster ALWAYS needs the health/panic view, even with no health_checks configured (100%-healthy fast path — availableCount/inPanic over an all-healthy set is well-defined and cheap)
	}
	opf := la.GetPolicy().GetOverprovisioningFactor().GetValue() // AMEND-LW3; 0 when unset -> newLocalityWeightedLB defaults to 140
	lw, err := newLocalityWeightedLB(endpoints, health, opf, func(sub []Endpoint) (loadBalancer, error) {
		return buildLeafLB(c, name, sub, health)
	})
	if err != nil {
		return nil, err
	}
	lb = lw
}
```

**The health-registry widening (a small, deliberate departure from today's nil-health-means-fast-path convention).** Every OTHER LB construct treats `health == nil` (no `health_checks` configured) as a legitimate all-healthy fast path with ZERO health-registry allocation. `localityWeightedLB` is DIFFERENT: it needs `clusterHealth.availableCount`/`inPanic` to be CALLABLE even when no health_checks exist (a locality-weighted cluster with no active health checking is still well-defined — every host reports available, `availableCount == len`, `inPanic` never fires), so the wrap-after-switch ALLOCATES a `clusterHealth` unconditionally when `locality_weighted_lb_config` is present, even if `hcSpecs`/`outlierCfg` are both empty. This is confirmed SAFE by `newClusterHealth`'s own contract (`health.go:83-93` — `hostHealth` starts `healthy=true`) and by `Pick`'s existing `lw.health != nil` nil-guard (defensive; in practice always non-nil once this wrap fires). §12 D-LW-HEALTHALLOC records this as a PLAN-confirmed (not empirically probed) design point.

---

## 4. Framework primitives — 0 new framework seams + 1 NEW `Endpoint` dimension + 0 new packages + 0 new go.mod deps

- **Seam:** REUSED unchanged (§3.2). NO new context-carry function, NO `Pick` signature change.
- **`Endpoint` dimension:** grows `Locality LocalityID` + `LocalityWeight uint32` — the SECOND per-endpoint dimension after phase 38's `Metadata` (`cluster.go:33-40`). `Addr()` is UNCHANGED (ring_hash/maglev table keys inside a locality child stay `"IP:PORT"`, identical to how subset children key their rings — `reference_conn_wrap_method_no_promote`, generalized: every `Endpoint{}` construction site — `extractEndpoints` + unit-test builders across `internal/cluster/*_test.go` — must be enumerated and updated at PLAN/IMPL).
- **`extractEndpoints` extension** (`manager.go:771-795`): captures `group.GetLocality()` (Region/Zone/SubZone) + `group.GetLoadBalancingWeight().GetValue()` (0 when unset — AMEND-LW2, NO defaulting) per `LocalityLbEndpoints` GROUP, stamping every endpoint in that group with the same `Locality`/`LocalityWeight` pair. `group.GetPriority()` stays discarded (unchanged from phase 38 — couples to the separate priority-LB candidate).
- **`ClusterLoadAssignment.Policy.overprovisioning_factor` capture:** read ONCE in `buildCluster` from `la.GetPolicy().GetOverprovisioningFactor().GetValue()` (`la` is the already-in-scope `*endpointv3.ClusterLoadAssignment`) and threaded to `newLocalityWeightedLB` — NOT captured on `Endpoint` (it is a cluster-wide policy field, not a per-locality one).
- **NEW packages: NONE.** `locality.go` lands in the EXISTING `internal/cluster` package (the `subset.go`/`ringhash.go`/`maglev.go` precedent).
- **NEW go.mod deps: NONE** (re-confirmed at this SPEC, §11.1 — `go mod tidy` stays clean; every proto field is in the already-imported go-control-plane v1.32.4 dep).
- **NEW producer plane: NONE** — `common_lb_config`/`LocalityLbEndpoints`/`ClusterLoadAssignment.Policy` are ALL cluster-scoped; unlike phase 38, nothing attaches to an HTTP route.

---

## 5. Proto-field roster (per §11.1 D-LW1)

| Field | Proto # | Type | Disposition |
|---|---|---|---|
| `Cluster.CommonLbConfig.healthy_panic_threshold` | 1 | `*type.v3.Percent` | ALREADY consumed (`parsePanicThreshold`, `health.go:429-437`) — UNCHANGED |
| `Cluster.CommonLbConfig.zone_aware_lb_config` | 2 (oneof) | `*Cluster_CommonLbConfig_ZoneAwareLbConfig` | NEW EXPLICIT REJECT (§6) |
| `Cluster.CommonLbConfig.locality_weighted_lb_config` | 3 (oneof) | `*Cluster_CommonLbConfig_LocalityWeightedLbConfig` (confirmed EMPTY marker — zero data fields) | THIS PHASE — presence triggers the wrap |
| `Cluster.CommonLbConfig.update_merge_window` | 4 | `*durationpb.Duration` | Silently accepted-ignored (unchanged; EDS-churn N/A) |
| `Cluster.CommonLbConfig.ignore_new_hosts_until_first_hc` | 5 | `bool` | Silently accepted-ignored (unchanged) |
| `Cluster.CommonLbConfig.close_connections_on_host_set_change` | 6 | `bool` | Silently accepted-ignored (unchanged) |
| `Cluster.CommonLbConfig.consistent_hashing_lb_config` | 7 | `*Cluster_CommonLbConfig_ConsistentHashingLbConfig` | Silently accepted-ignored (unchanged) |
| `Cluster.CommonLbConfig.override_host_status` | 8 | `*core.v3.HealthStatusSet` | Silently accepted-ignored (unchanged) |
| `LocalityLbEndpoints.locality` | 1 | `*core.v3.Locality` (`region`/`zone`/`sub_zone` strings) | THIS PHASE — captured into `Endpoint.Locality` |
| `LocalityLbEndpoints.lb_endpoints` | 2 | `[]*LbEndpoint` | ALREADY consumed (unchanged) |
| `LocalityLbEndpoints.load_balancing_weight` | 3 | `*wrapperspb.UInt32Value` | THIS PHASE — captured into `Endpoint.LocalityWeight` (0 if unset, NO default — AMEND-LW2) |
| `LocalityLbEndpoints.priority` | 5 | `uint32` | STAYS discarded (the priority-LB candidate's territory) |
| `LocalityLbEndpoints.proximity` | 6 | `*wrapperspb.UInt32Value` | STAYS discarded (unrelated) |
| `ClusterLoadAssignment.Policy.overprovisioning_factor` | 3 (of `Policy`) | `*wrapperspb.UInt32Value`, default 140 | THIS PHASE — CONSUMED (AMEND-LW3), read once per cluster build |

---

## 6. PARSE-REJECT roster (per §11.1 + ADR-0080)

### 6.1 Wording discipline + the new reject surface

Two NEW envoy-go-strict departure rejects (BOTH confirmed by live-probe to be departures, not reference-parity — §11.1):

1. **`zone_aware_lb_config` configured** → `cluster: %q: common_lb_config.zone_aware_lb_config is not supported` (a distinct, unimplemented algorithm; the reference accepts and runs it — envoy-go over-rejects, a recorded DEPARTURE).
2. **BOTH `lb_subset_config` AND `locality_weighted_lb_config` configured on the same cluster** → `cluster: %q: lb_subset_config cannot be combined with common_lb_config.locality_weighted_lb_config` (the reference accepts and silently overrides locality weighting — envoy-go over-rejects rather than replicate the confusing silent-override, a recorded DEPARTURE).

### 6.2 NO new producer-side reject arm

Unlike subset's `RouteAction.metadata_match` scalar-only reject, phase 52 has no per-route producer — there is no NEW reject on the endpoint/locality side beyond the two above. A malformed `Locality` (e.g. all three fields empty) is NOT rejected — the zero-value `LocalityID{}` is a valid single locality group (mirroring how the reference treats an absent `locality` field as its own implicit group).

### 6.3 NON-reject dispositions (parity — AMEND-LW1/LW2)

- An omitted `load_balancing_weight` on SOME (not all) localities → NOT a reject; the locality is assigned zero load (AMEND-LW2, PARITY with the reference's own documented + observed behavior).
- An omitted `load_balancing_weight` on ALL localities → NOT a reject; degrades to flat RR (AMEND-LW2, PARITY).
- `overprovisioning_factor` set to any value (including `0`, treated as "unset" per the wrapper's own default substitution — matching how the reference's proto-generated default-140 annotation works on an explicit `{value: 0}` vs a nil pointer; D-LW-OPF0 in §12 flags this as an untested corner since neither probe set an explicit `0`) → NOT a reject, always accepted.
- The FIVE deferred `CommonLbConfig` fields (§5) → NOT a reject; silently accepted-ignored, UNCHANGED from before this phase (they were never read either way).

---

## 7. Stat surface — CONFIRMED ZERO delta (per §11.5 D-LW5 + AMEND-LW5)

Stat surface **STAYS 1200 (+0)**. NO `registerClusterMetrics` change — `localityWeightedLB` injects NO stat handles (contrast `subsetLB`'s 4 `lb_subsets_*` injections, `manager.go:147-155`). The reference exposes NO dedicated locality-weighted stat group; the mechanism is observable ONLY through the request-distribution itself and the pre-existing `membership_total`/`membership_healthy` gauges (already emitted, `reference_membership_total_vs_healthy_gauge` — unaffected by which locality mode is active). `lb_healthy_panic` (already emitted, `health.go:181-185`) continues to increment from `localityWeightedLB.Pick`'s own panic branch exactly as every leaf policy's panic branch already does — no new counter, just a new call site of the SAME existing counter handle (shared across the whole cluster via `clusterHealth.panicCounter`).

---

## 8. Differential fixture taxonomy (+1: `0095` HTTP cross-side weight-ratio + health-degradation shift)

### 8.1 `0095-lb-locality-weighted` (cross-side; per-side statistical bands + cross-side deterministic stats)

Topology (mirroring the confirmed-working probe shape exactly, to minimize risk): an HTTP listener routing to a STATIC cluster with `common_lb_config.locality_weighted_lb_config: {}`, TWO localities (`region: a` / `region: b`) × 5 backend hosts each (10 total), `load_balancing_weight` 2 (region a) / 1 (region b), `health_checks` configured (HTTP `/healthz`, short interval/thresholds for fast convergence), `dns_refresh_rate` pinned long (per the probe's own environment-gotcha finding — avoids spurious health flicker on STRICT_DNS re-resolution; N/A for envoy-go's STATIC-only cluster model but the REFERENCE side of the differential DOES need this pin if it uses STRICT_DNS backends — the driver design confirms which discovery type the differential harness's backends use, D-LW6/§12).

- **Arm (a) — static ratio (all healthy):** send ≥500 requests; assert per-region request SHARE within a banded margin of the confirmed formula's 100%-healthy prediction (66.7% / 33.3%) — a PER-SIDE band (`reference_differential_band_sigma_margin`, ~4-5σ over ≥20 runs), NOT cross-side per-request identity (the independent per-request RNG makes cross-side identity infeasible for a randomized policy, the `0060`-lb-random/`0065`-weighted-clusters precedent).
- **Arm (b) — health-degradation shift:** fail health checks on 3 of region-a's 5 hosts (→ 40% healthy in region a), poll `/stats`'s `membership_healthy` to convergence THEN run the K=10-consecutive-non-degraded-host warmup (`reference_health_check_propagation_warmup`), snapshot a POST-WARMUP baseline, send ≥500 more requests, and assert the DELTA per-region share matches the confirmed formula's 40%-healthy prediction (52.8% / 47.2%) within the same band — the load-bearing NOVEL proof (a continuous-weight shift, not a set-membership flip).
- **Cross-side deterministic stats:** `membership_total`/`membership_healthy` (both arms) + `upstream_rq_total` (cross-equal total request count, both sides driven identically) via `StatsAsserter` (`reference_differential_asserter_dispatch`).
- **Deliberate breaks (both proven LIVE with `-count=1`, `reference_differential_break_protocol_count1`):** (i) skip the `locality_weighted_lb_config` wrap (leaving flat `buildLeafLB` output) → defeats arm (a)'s ratio band (produces ~50/50, not ~67/33); (ii) freeze the effective-weight computation at the 100%-healthy value (never re-read live health) → defeats arm (b)'s shifted-ratio band.

### 8.2 NO boot-reject cross-side dir (AMEND-LW1)

Both new rejects (§6.1) are envoy-go-strict DEPARTURES with no reference-side counterpart to differentially compare against (the reference accepts both configurations) — per `reference_differential_fixture_dispatch_constraint`, these are SUBJECT-SIDE UNIT TESTS (`manager_test.go`), NOT a cross-side boot-reject fixture dir (there is nothing on the reference side to boot-reject).

### 8.3 NO new BackendKind + NO new fuzzer (family expectations, reconfirmed)

- **BackendKind:** tail STAYS **38** (`H2GoawayResponder`, 39 kinds 0-indexed). `0095` reuses the plain HTTP backends from `0062`/`0063`/`0064` for traffic + a health-check-capable responder (the phase-39 `0066` `BlockingHoldResponder`-adjacent precedent, or a simple toggle-able HTTP responder) for arm (b)'s controlled failure — driver-owned, not a new `BackendKind` (health-check responders are already driver-owned per existing convention).
- **Fuzzer:** STAYS **52**. No wire decode (locality/weight derive from validated xDS bootstrap config, not untrusted bytes). A locality-grouping/weighted-draw property test (random locality/weight/health combos → the resolver never panics, a zero-weight locality never gets picked, an all-zero-weight cluster falls back to flat) is a unit-level candidate FOLDED into `locality_test.go`, not a `Fuzz*` corpus entry.

### 8.4 Total

96 → **97** (`0095-lb-locality-weighted`, cross-side, both arms in ONE fixture dir per `reference_differential_fixture_dispatch_constraint` — both arms exercise the SAME runner branch [cross-side traffic], unlike a boot-reject fixture which would need its own dir).

---

## 9. Behavior-contract delta (the phase-52 bundle; ADR-0052 atomic landing)

A new `### Load balancer — locality-weighted (locality_weighted_lb_config)` section lands in `docs/envoy-go/BEHAVIOR_CONTRACT.md` immediately after the existing `### Load balancer — subset (lb_subset_config)` section (mirroring that section's shape — opening italic intro citing ADR-0269 + the seam-reuse framing, then: the wrap-after-switch acceptance; the `Endpoint.Locality`/`LocalityWeight` dimension; the confirmed effective-weight formula with the exact AMEND-LW2/LW3/LW4 findings; the two explicit rejects + their departure framing; the confirmed-zero stat delta; the differential proof shape; the deferred surface list from §2). Landed atomically with the ADR-0269 body at IMPL (ADR-0052's discipline — the BEHAVIOR_CONTRACT delta and the ADR body land together, not separately).

---

## 10. Per-task structure (~10-11 tasks; the PLAN decomposes)

| # | Task | SPEC anchor |
|---|---|---|
| 1 | Scaffolding + baseline re-verification (fixtures/fuzzers/stat/BackendKind counts fresh at HEAD) | §11 preconditions |
| 2 | `Endpoint.Locality`/`Endpoint.LocalityWeight` fields + `extractEndpoints` capture + EVERY `Endpoint{}` construction-site update (test builders across `internal/cluster/*_test.go`) | §4 |
| 3 | `ClusterLoadAssignment.Policy.overprovisioning_factor` capture in `buildCluster` | §4/§5 |
| 4 | `internal/cluster/locality.go`: `LocalityID`/`localityGroup`/`localityWeightedLB` + `newLocalityWeightedLB` + `Pick` (the confirmed formula + panic bypass + zero-total fallback) | §3.1 |
| 5 | `manager.go` wrap-after-switch: the `zone_aware_lb_config` reject, the both-configs reject, the `locality_weighted_lb_config` wrap (incl. the health-registry-widening note, §3.3) | §3.3/§6 |
| 6 | Unit tests: the effective-weight formula (health-fraction × OPF, the 6-point curve + the OPF-100-vs-140 A/B, mirroring the live-probed data), the AMEND-LW2 zero-load + all-omitted-flat-fallback outcomes, the panic full-bypass, both reject arms | §11.3/§11.2/§11.4/§6 |
| 7 | `test/fixtures/0095-lb-locality-weighted` driver: the 2-locality×5-host topology, the health-check-toggle harness (reusing/extending the phase-39 convergence-poll + warmup helper) | §8.1 |
| 8 | `0095` assertions: arm (a) static-ratio band, arm (b) degraded-ratio band, cross-side `StatsAsserter` | §8.1 |
| 9 | `0095` deliberate breaks (both, `-count=1`) + 20-run flake-soak + `-race` | §8.1 |
| 10 | `docs/envoy-go/BEHAVIOR_CONTRACT.md` delta + ADR-0269 body (§Decision/§Consequences) + final six-gate + ROADMAP row 52 flip | §9/§13 |

Anticipated **~220-320 prod LoC** (the `locality.go` construct ~120-160 LoC + the manager/extractEndpoints deltas ~30-40 LoC + test code separately) — well under the ADR-0045 gate, confirming §3.0's no-split disposition.

---

## 11. SPEC-time empirical-pin block (D-LW1..D-LW8 — executed IN-SESSION 2026-07-06, two parallel probe agents against `contrib-v1.37.2` on a Docker bridge network per `reference_docker_probe_bridge_network`)

### Summary disposition table (8 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| D-LW1 | Config surface + composition acceptance | CONFIRMED LIVE | AMEND-LW1 |
| D-LW2 | Weight default/mixed-presence rule | REFUTED + CORRECTED | AMEND-LW2 |
| D-LW3 | Effective-weight formula | CONFIRMED LIVE | AMEND-LW3 |
| D-LW4 | Panic-mode interaction | CONFIRMED LIVE (hypothesis (a)) | AMEND-LW4 |
| D-LW5 | Stat surface delta | REFUTED DOWN (zero, not small) | AMEND-LW5 |
| D-LW6 | Differential harness design | RESOLVED (§8) | — |
| D-LW7 | Seam-reuse confirmation | CONFIRMED (zero change) | — |
| D-LW8 | LoC/task envelope + fuzzer | CONFIRMED (~220-320 LoC / 10-11 tasks; no fuzzer) | — |

### 11.1 D-LW1 (SPEC-BLOCKING) — config surface + reject contract: CONFIRMED LIVE

Proto surface re-pinned exact (§5 table) directly from the go-control-plane v1.32.4 module cache. `Cluster_CommonLbConfig_LocalityWeightedLbConfig` confirmed a truly EMPTY marker message (state/sizeCache/unknownFields only). LIVE boot test: a cluster with BOTH `lb_subset_config` (a `version` selector) and `common_lb_config.locality_weighted_lb_config` set, weights 3:1 across 2 localities, ALL endpoints/route tagged to match the SAME subset — `envoy --mode validate` returned `configuration OK`; a real boot served 400 requests with the result `regionA=200, regionB=200` (an EXACT 1:1 split, NOT the configured 3:1) while `lb_subsets_selected: 400` / `lb_subsets_fallback: 0` confirmed subset resolution stayed live the entire time. **The reference silently overrides/ignores locality weighting once subset load-balancing engages — it does NOT reject the combination.** Separately, `common_lb_config.zone_aware_lb_config` alone (`routing_enabled: 100`, `min_cluster_size: 1`) booted and routed correctly across 20 requests — a fully legitimate, independently-functioning reference feature. `go mod tidy -diff` reconfirmed EMPTY (`git status --porcelain go.mod go.sum` empty before/after). **Confirms both of phase 52's planned rejects (§6.1) are envoy-go-strict DEPARTURES** — the reference accepts both configurations; envoy-go deliberately rejects them for a cleaner, honest failure mode instead of replicating the reference's confusing silent-override / building an unimplemented separate algorithm.

### 11.2 D-LW2 (SPEC-BLOCKING) — the weight default/mixed-presence rule: REFUTED + CORRECTED

⚠️ **REFUTATION of the BRAINSTORM's D-LW2 assumption** ("the reference default is `1` per the proto doc comment... confirm live"). LIVE-PROBED: a 2-locality×2-host cluster (region A `load_balancing_weight: 2` explicit, region B's field OMITTED entirely), `locality_weighted_lb_config: {}`, 900 requests sent. Result: region A received **900/900** requests (both A hosts ~450 each); region B received **0/900** (both B hosts exactly 0). The reference does NOT default the omitted weight to 1 (which would predict a ~2:1, i.e. ~600/300, split) — it excludes the locality ENTIRELY. This matches the go-control-plane proto doc comment on `load_balancing_weight`, quoted verbatim: *"If no weights are specified when locality weighted load balancing is enabled, the locality is assigned no load."* A SEPARATE test with BOTH localities' weights omitted (same topology, 900 requests) gave an even ~1:1 split (227/227/223/223 across the 4 hosts, region totals ≈454/446) — i.e. `locality_weighted_lb_config` degrades to a FLAT round-robin across the WHOLE endpoint set when no locality anywhere declares a weight, rather than "both excluded" (which would serve zero traffic, an absurd outcome the reference correctly avoids). **envoy-go's design (§3.1) reproduces BOTH outcomes from ONE mechanism with ZERO special-casing:** capture the raw `GetValue()` (0 when unset, no substitution) as each locality's weight; a 0-weight locality's cumulative bucket is empty (never drawn, mirroring the ALREADY-LANDED `router_weighted.go:53` "a weight-0 entry... is never picked" comment); and when the TOTAL effective weight across all localities is exactly 0 (either because every raw weight is 0, or because every nonzero-weight locality's health degraded to 0), `Pick` falls back to the flat all-endpoints child — which reproduces the observed "all omitted → flat RR" result exactly.

### 11.3 D-LW3 (SPEC-BLOCKING) — the effective-weight formula: CONFIRMED LIVE

The hypothesized formula `effective_weight = configured_weight × min(1, (overprovisioning_factor/100) × healthy_fraction)` was LIVE-PROBED across 6 data points on a 2:1-weighted, 2-locality×5-host cluster (region A's healthy count varied 5/5 → 0/5, region B held fully healthy throughout), each measurement taken as a DELTA over a post-convergence-and-warmup baseline (`reference_health_check_propagation_warmup`):

| A healthy-fraction | observed A-share | predicted A-share (OPF=140, the reference's own default) | match |
|---|---|---|---|
| 100% (5/5) | 67.00% | 66.7% | yes |
| 80% (4/5) | 66.00% | 66.7% (no degradation predicted — ABOVE the 71.4% plateau threshold) | yes |
| 60% (3/5) | 62.80% | 62.7% | yes |
| 40% (2/5) | 52.80% | 52.8% | EXACT |
| 20% (1/5) | 36.00% | 35.9% | yes |
| 0% (0/5) | 0.00% | 0% | EXACT |

`overprovisioning_factor` default confirmed **140** (go-control-plane `endpoint.pb.go:154-168` doc comment, quoted verbatim: *"With the default value 140(1.4), Envoy doesn't consider a priority level or a locality unhealthy until their percentage of healthy hosts drops below 72%"* — i.e. the plateau threshold is `100/OPF`). A direct A/B AT THE SAME 60%-healthy state — `overprovisioning_factor` explicit `140` (default) vs explicit `100` — produced 62.8% vs 54.0% respectively (predicted 62.7% vs 54.5%), a CLEARLY DIFFERENT result at an IDENTICAL health state, **proving envoy-go must genuinely READ this field, not hardcode 140.** No panic contamination observed at any data point (`lb_healthy_panic` stayed 0 throughout, including the 0/5-A point where overall health was exactly 5/10 = 50% — confirming the panic comparison is STRICT `<`, matching the ALREADY-LANDED `clusterHealth.inPanic`, `health.go:165-171`, verbatim).

### 11.4 D-LW4 (SPEC-BLOCKING) — the panic-mode interaction: CONFIRMED LIVE (hypothesis (a))

With BOTH localities degraded to 1/5 healthy each (2/10 = 20% overall, well under the default 50% panic threshold), post-convergence traffic (500 requests) spread NEAR-EVENLY across ALL 10 ORIGINAL hosts — healthy and unhealthy, both localities alike (per-host deltas 46-54, no discernible bias) — with region sums 247 (49.4%) / 253 (50.6%), essentially 50:50 and NOT the configured 2:1 weight. `lb_healthy_panic` incremented to 1300 over the 500-request measurement (confirming the counter increments per internal `Pick` call, consistent with the EXISTING `panicInc()` call convention inside every leaf policy's panic branch — e.g. `loadbalancer.go:44-66` — which envoy-go's `localityWeightedLB.Pick` reuses at its OWN panic branch, §3.1). **Confirms hypothesis (a): cluster-wide panic FULLY bypasses BOTH health-filtering AND locality-weighting**, not merely one or the other.

### 11.5 D-LW5 (SPEC-BLOCKING) — stat surface delta: REFUTED DOWN

A full `/stats` scrape (549 lines) of a traffic-served, `locality_weighted_lb_config`-active cluster, grepped case-insensitively for "local"/"region"/"zone" at cluster scope, found ONLY: the pre-existing, UNRELATED zone-aware-routing stat group (`lb_zone_cluster_too_small`/`lb_zone_no_capacity_left`/`lb_zone_routing_all_directly`/`lb_zone_routing_cross_zone`/`lb_zone_routing_sampled`/`lb_local_cluster_not_ok`/`lb_recalculate_zone_structures` — ALL reading `0`, confirmed separately to populate meaningfully only when `zone_aware_lb_config` is actually active) plus the standard `membership_*` gauges (unaffected by locality mode). **NEGATIVE FINDING, confirmed: NO dedicated locality-weighted-specific stat exists in the reference.** Locality-weighted LB's only externally-observable effect is the request-distribution itself, plus the per-host `region`/`zone`/`sub_zone`/`weight` fields already exposed on the admin `/clusters` page regardless of which locality LB mode is configured (pre-existing, unrelated to this phase). **Stat surface delta: 0 (confirmed, refuting the BRAINSTORM's anticipated "small 0-2").**

### 11.6 D-LW6 — the differential harness design: RESOLVED

Resolved directly by the confirmed formula (§11.3) + the confirmed panic behavior (§11.4): the `0095` fixture (§8.1) reuses the EXACT topology that was live-probed working (2 localities × 5 hosts, 2:1 weight) for both a static-ratio arm and a health-degradation arm targeting the SAME confirmed data points (100%-healthy and 40%-healthy), giving high confidence the differential's predicted bands are achievable and non-flaky (they are the SAME numbers already empirically observed against the live reference during this SPEC).

### 11.7 D-LW7 — the seam-reuse confirmation: CONFIRMED (zero change)

The `localityWeightedLB` sketch (§3.1) confirms `loadBalancer.Pick`'s existing 4-parameter signature is sufficient — the wrapper needs no additional input beyond its own per-cluster build-time state (`groups`, `flat`, `health`, `overprovisioningFactor`) and forwards `(hashKey, hasHash, match, hasMatch)` unchanged to whichever child it delegates to, exactly mirroring `subsetLB`'s existing forwarding shape (`subset.go:213-230`). No `WithX ctx` carry, no interface change, no `Cluster` exported-surface change.

### 11.8 D-LW8 — the LoC/task envelope + fuzzer decision: CONFIRMED

§10's task breakdown (10 tasks) and LoC estimate (~220-320 prod LoC) are comfortably under the ADR-0045 gate — CONFIRMING §3.0's no-split disposition. No dedicated fuzzer warranted (§8.3) — confirmed, no wire-decode surface exists anywhere in this construct.

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-LW-DUP** — the double-declared-locality-identity corner (two `LocalityLbEndpoints` groups sharing an identical `region`/`zone`/`sub_zone` tuple but declaring DIFFERENT `load_balancing_weight` values within the SAME `ClusterLoadAssignment`). Self-answered here: last-write-wins in endpoint-encounter order (§3.1's `weights[ep.Locality] = ep.LocalityWeight` map-overwrite). An unusual, essentially degenerate config shape not exercised by either probe (both probes used ONE `LocalityLbEndpoints` group per distinct locality, the realistic/idiomatic shape) — the PLAN documents this as a known, low-stakes, untested assumption rather than re-probing it.
- **D-LW-HEALTHALLOC** — confirming the §3.3 health-registry-widening design (allocating `clusterHealth` unconditionally when `locality_weighted_lb_config` is present, even with zero `health_checks` configured) compiles and behaves correctly against the EXISTING `clusterHealth`/`hostHealth` code paths with zero active checkers running (i.e. every host stays `healthy=true` forever, `availableCount == len`, `inPanic` never fires) — a code-reading confirmation at PLAN time, not a live probe (the mechanism is a direct, already-well-understood consequence of `newHostHealth`'s `healthy.Store(true)` default, `health.go:44-48`).
- **D-LW-OPF0** — whether an EXPLICIT `overprovisioning_factor: {value: 0}` (as opposed to the field being entirely absent/nil) should be treated identically to "unset → 140" (envoy-go's planned behavior, §6.3) or should instead be honored literally as `0` (meaning EVERY locality with ANY unhealthy host immediately drops to 0 effective weight, since `min(1, 0×frac) = 0` whenever `frac < 1`). Neither probe set an explicit `0` (both probes used "unset" and an explicit non-zero `100`). The PLAN pins this via a targeted code-read of the go-control-plane wrapper semantics (a `*wrapperspb.UInt32Value{Value: 0}` is a valid, PRESENT zero — distinguishable from nil via `!= nil` before calling `.GetValue()`) — envoy-go's `la.GetPolicy().GetOverprovisioningFactor().GetValue()` idiom (§3.3) currently CANNOT distinguish "absent" from "explicit 0" (both read as Go zero-value `0`), so the CURRENT design treats them IDENTICALLY (both → default-substituted to 140 inside `newLocalityWeightedLB`). This is flagged as a KNOWN, minor departure risk (an explicit `overprovisioning_factor: 0` config, if anyone ever writes one, would behave as 140 in envoy-go rather than a literal 0) — the PLAN decides whether to thread the WRAPPER pointer through (distinguishing nil from `{value:0}`) to close this gap, or accept it as documented.

---

## 13. ADR continuity — the ADR-0269 §Context DRAFT (anchored here; the full entry lands at the IMPL)

**ADR-0269 §Context DRAFT (the `locality-weighted` load-balancing policy):** Envoy's `Cluster.CommonLbConfig.locality_weighted_lb_config` (an empty marker message toggling a distinct weighting MODE, `envoy.config.cluster.v3`) weights traffic across a cluster's endpoint localities (`LocalityLbEndpoints.locality` — region/zone/sub_zone) by each locality's configured `load_balancing_weight`, scaled by that locality's live healthy-host fraction and an `overprovisioning_factor` (`ClusterLoadAssignment.Policy.overprovisioning_factor`, default 140/1.4) — the SIXTH Load-balancing-family construct and the FIRST to consume the phase-39/40 per-host health state as a CONTINUOUS weight rather than a binary include/exclude filter. Landed as `internal/cluster/locality.go`'s `localityWeightedLB`, a WRAPPER `loadBalancer` (the phase-38 `subsetLB` structural precedent: a construct owning per-group child `loadBalancer`s built by the existing `buildLeafLB` factory, one per distinct locality, ALL FIVE leaf policies supported uniformly via factory reuse) built once at cluster construction and WRAPPING whatever child the `lb_policy` switch produced (a SECOND wrap-after-switch site in `manager.go`'s `buildCluster`, alongside the phase-38 `lb_subset_config` wrap). UNLIKE subsetLB, this wrapper needs **ZERO new `Pick` parameters** — `loadBalancer.Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool)` (`loadbalancer.go:16-23`) stays byte-for-byte unchanged; locality selection resolves entirely from per-cluster build-time state (`Endpoint` grows a `Locality LocalityID` + `LocalityWeight uint32` dimension, the SECOND per-endpoint dimension after phase 38's `Metadata`) plus the EXISTING `clusterHealth` state (ADR-0242/0243), consulted via a NEW per-locality aggregation read (`clusterHealth.availableCount` called with a locality's OWN endpoint sub-slice) — the wrapper forwards `(hashKey, hasHash, match, hasMatch)` UNCHANGED to whichever child it delegates to, mirroring `subsetLB`'s existing forwarding shape exactly. Live-probed against `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227) and CONFIRMED: the effective-weight formula `configured_weight × min(1, (overprovisioning_factor/100) × healthy_fraction)` (6-point curve match + an OPF-100-vs-140 A/B proving the factor must be genuinely read, not hardcoded); cluster-wide panic (the EXISTING `clusterHealth.inPanic` gate, unmodified) FULLY BYPASSES locality weighting AND health-filtering, delegating to a pre-built flat all-endpoints child; an omitted `load_balancing_weight` is assigned ZERO load when sibling localities have an explicit weight (REFUTING the natural "defaults to 1" assumption — matching the reference's own documented "the locality is assigned no load" behavior), and when NO locality anywhere declares a weight the mechanism degrades to flat round-robin — BOTH outcomes reproduced by ONE unified "zero total effective weight → flat-child fallback" rule with no special-casing. TWO envoy-go-strict departure rejects, both CONFIRMED live against the reference (which accepts both configurations but behaves confusingly): a cluster configuring the sibling `zone_aware_lb_config` oneof arm (a distinct, unimplemented local-zone-percentage-redirect algorithm) is REJECTED rather than silently falling through, per this project's established sibling-reject discipline for a oneof arm lifted out of a shared silent-ignore set; a cluster configuring BOTH `lb_subset_config` and `locality_weighted_lb_config` is REJECTED rather than replicating the reference's own silent locality-weight override once subset engages. The stat surface gains **ZERO new stats** (live-confirmed — no dedicated locality-weighted stat group exists in the reference; REFUTING the BRAINSTORM's anticipated small delta), the THIRD zero-stat-delta phase in the Load-balancing family. Proven via the cross-side `0095-lb-locality-weighted` differential fixture: a per-side statistical-band static weight-ratio arm PLUS a health-degradation shift arm (failing checks on a controlled subset of one locality's hosts, reusing the phase-39 poll-to-convergence-and-warmup pattern, then re-measuring the shifted share) — the load-bearing novel proof, since this construct is the family's first to weight continuously rather than filter binarily. NO new package (`internal/cluster`), NO new go.mod dependency, NO new producer plane (a cluster-only construct — the first LB phase since round_robin with zero per-route surface), NO new BackendKind, NO new fuzzer. ADR-0024 (the per-cluster LB-state-scope discipline) stays UNAMENDED — locality-group state is per-cluster LB-instance state, the same discipline every prior LB construct already follows.

§Decision/§Consequences bodies land at the phase-52 IMPL per ADR-0044 (next-free after phase 52 ≈ **ADR-0270**). The PLAN/IMPL may surface additional ADRs — anticipated NONE (a single-ADR reuse-shape phase, the phase-35/37 precedent; §11.7 confirms zero seam change).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

- Stat surface: **1200 (+0)**, CONFIRMED (§11.5) — down from the BRAINSTORM's anticipated "small 0-2" delta.
- Differential fixtures: **96 → 97** at IMPL (`0095-lb-locality-weighted`).
- Fuzzers: **52 (+0)** — CONFIRMED, no new fuzzer.
- BackendKind tail: **38 (+0)** — CONFIRMED, no new kind.
- DECISIONS.md tail: **ADR-0268** at this SPEC (docs-only); → **ADR-0269** at the phase-52 IMPL (next-free ADR-0270).
- ROADMAP row 52: STAYS `in-progress` (lifecycle-state 1 → 2 at this SPEC-DONE commit); flips `done` at the phase-52 IMPL six-gate (NO parent rollup, a single flat row per ADR-0106).
- ZERO new Go packages, ZERO new go.mod modules (re-confirmed, §11.1).
- spec-document-reviewer gate applies at this SPEC.

Next → the **phase-52 PLAN** (`superpowers:writing-plans` — decompose §10 into bite-sized TDD tasks; FINAL ADR-0045 gate re-check, anticipated to re-confirm the no-split disposition of §3.0).
