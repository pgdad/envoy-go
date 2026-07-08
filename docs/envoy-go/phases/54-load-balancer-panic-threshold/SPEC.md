# Phase 54 SPEC — `healthy_panic_threshold` as an independent construct (`Cluster.CommonLbConfig.healthy_panic_threshold`)

> For agentic workers: this SPEC resolves every BRAINSTORM.md §10 D-PT question (D-PT1..D-PT6) against LIVE evidence from `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227), executed IN-SESSION on a Docker bridge network (`reference_docker_probe_bridge_network`), ground-truthed via per-host `rq_total` deltas from the admin `/clusters` endpoint under `--concurrency 1`. **The single most consequential finding — NOT anticipated by the BRAINSTORM — is AMEND-PT1: the reference INTEGER-TRUNCATES (floors) the configured threshold to a whole percent before comparing** (`healthy_percent(double) < floor(threshold_percent)`), so envoy-go's current float-fraction comparison OVER-PANICS at any non-integer threshold (a genuine divergence, e.g. `60.9%` threshold at `60%` healthy → reference does NOT panic, envoy-go does). AMEND-PT2 refines the double-increment fix shape (D-PT1(iii)): the clean fix is to REMOVE the redundant outer `panicInc()`, NOT the BRAINSTORM's anticipated nil-health flat child. AMEND-PT3 CONFIRMS the locality retrofit is needed (D-PT4: the reference never lets a single degraded locality internally flatten) AND that the subset analog needs NO change (per-subset panic IS the reference's behavior — the crux asymmetry). AMEND-PT4 CONFIRMS out-of-range values are PGV-rejected at boot (D-PT3). The PLAN decomposes §10 into TDD tasks.

**Goal:** turn `Cluster.CommonLbConfig.healthy_panic_threshold` (`envoy.type.v3.Percent`, default 50%) from a **consumed-but-never-proven** field into a proven, hardened, independently-observable construct — the EIGHTH and FINAL Load-balancing-family row (the family CLOSES at phase-done). Deliver: the first-ever differential proof (`0097-lb-panic-threshold`), the semantics pins, and FOUR evidence-driven corrections — the integer-truncation comparison-shape fix (AMEND-PT1, new), the `lb_healthy_panic` double-increment fix (AMEND-PT2), the `locality.go` child-local-panic retrofit (AMEND-PT3), and out-of-range validation parity (AMEND-PT4).

**Architecture:** ZERO new packages, ZERO new files, ZERO new `Pick` parameters, ZERO new go.mod deps (all confirmed, §4/§11.3). All deltas live in the EXISTING `internal/cluster` package: the panic comparison shape (`health.go` `inPanic`/`parsePanicThreshold`), the double-increment site (`locality.go:143-146`), the per-locality child health-view wiring (`locality.go` constructor, reusing phase-53's `tierHealth` panic-disabled-view primitive), and a new out-of-range parse-reject (byte-stable per ADR-0080). The stat surface stays **1200 → 1200** (a VALUE correction on the existing `lb_healthy_panic` counter, not a name change) — the family's FIFTH zero-stat-delta phase.

**Tech stack:** unchanged — Go, `internal/cluster`, the existing `clusterHealth`/`hostHealth` model (`health.go`, ADR-0242/0243), `tierHealth` (`priority.go`, ADR-0270 — the retrofit's structural template), the `buildLeafLB` factory closures (the health-view wiring sites).

**Authored:** 2026-07-08.

---

## 1. Purpose / Mission

Phase 54 lands the family's EIGHTH and LAST construct — but unlike every prior LB phase (each of which built a new policy or wrapper), it builds almost NO new mechanism. The panic construct already exists end-to-end: `parsePanicThreshold` (`health.go:485-491`, phase 39) reads `common_lb_config.healthy_panic_threshold` (default 50%, returned as a fraction); `inPanic` (`health.go:182-188`) fires when the healthy fraction is strictly below it; and every leaf policy consults it via the shared `panicGate` (`health.go:211-220`). What did NOT exist before this phase: any TEST of a non-default value (every `newClusterHealth` call in `internal/cluster/*_test.go` hardcodes `0.5`), any FIXTURE that sets the field, and any differential drive of the classic threshold-panic path (`lb_healthy_panic` has only ever been asserted `== 0`). Phase 54 supplies the proof — and, in supplying it, the live reference probe (§11) surfaced FOUR concrete divergences/gaps the proof must correct so it pins the RIGHT values.

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

- **AMEND-PT1 (D-PT1(iv) — the phase's single most consequential finding, NOT anticipated by the BRAINSTORM: the reference INTEGER-TRUNCATES the threshold).** The BRAINSTORM (§2.5 iv) anticipated only "confirming the comparison shape at fractional boundaries" (the `reference_percent_cap_cross_multiply` guard). LIVE-PROBED across six non-integer-boundary scenarios, the reference's panic condition is **`100.0 × healthy / total < floor(threshold_percent)`** — the configured `Percent.value` is truncated toward zero to a whole percent (Envoy fetches it via `runtime.getInteger` / a `static_cast<uint64_t>` conversion) BEFORE the comparison, while the healthy percentage stays a real double. Decisive evidence (5-host and 3-host flat health-checked clusters, ROUND_ROBIN): threshold `66.7` at `2/3 = 66.67%` healthy → **NO panic** (floor→66; `66.67 < 66` false); threshold `66.5` → NO panic; threshold `67` → **PANIC** (`66.67 < 67`); threshold `66` → NO panic. **The divergence:** threshold `60.9` at `3/5 = 60%` healthy → reference floor→60 → `60 < 60` false → **NO panic**, but envoy-go's current `float64(available)/float64(total) < value/100.0` computes `0.60 < 0.609` → **PANIC**. envoy-go OVER-panics at every non-integer threshold whose floor the healthy fraction meets. **envoy-go reproduces the reference bit-for-bit** (§3.1) by flooring the configured value to an integer percent and comparing via an INTEGER CROSS-MULTIPLY (the ULP-safe exact mirror, `reference_percent_cap_cross_multiply`): panic iff **`100 × availableCount < floor(value) × total`** (strict `<`; the same-direction integer form of `100.0·healthy/total < floor(value)`). At integer thresholds (50, 80, 60 — every value any prior test or fixture ever used) the old and new forms AGREE, which is exactly why the gap lay dormant. NOTE: the reference's panic percentage also folds in a `degraded_percent` term; envoy-go implements no degraded host state (HEALTHY/UNHEALTHY only, ADR-0242) — a pre-existing, documented coverage boundary (§2), and `0097` uses no degraded hosts.
- **AMEND-PT2 (D-PT1(iii) — the double-increment fix shape, REFINED away from the BRAINSTORM's anticipation).** The BRAINSTORM (§1.1/§2.6) anticipated fixing `lb_healthy_panic`'s locality-weighted double-increment by building `localityWeightedLB`'s flat fallback child with a NIL health registry (the `priorityLB`/ADR-0270 precedent). LIVE-PROBED, the reference increments `lb_healthy_panic` **exactly once per pick** on BOTH a flat cluster (delta = N over N requests) AND a locality-weighted cluster under cluster-wide panic (delta = N, with all hosts flat-uniform). This pins a CLEANER, lower-risk fix than the nil-health child: **REMOVE the redundant `lw.health.panicInc()` at `locality.go:144`** and let the shared-health flat child's OWN `panicGate` provide the single increment. This is correct precisely because `localityWeightedLB`'s bypass condition (`lw.health.inPanic(lw.allEndpoints)`, `locality.go:143`) is IDENTICAL BY CONSTRUCTION to the flat child's own `inPanic` over the same full endpoint set (the flat child is `factory(endpoints)` bound to the shared `clusterHealth`) — so the flat child's `panicGate` fires on exactly the same condition and does the blind-all-hosts pick + the one increment. This is the deliberate CONTRAST with `priorityLB`, whose differently-shaped capacity-sum bypass could NOT guarantee agreement with a leaf's own `inPanic` and therefore required ADR-0270's nil-health flat child. Removing the outer `panicInc()` (rather than nil-ing the flat child) ALSO preserves the current zero-total-effective-weight fallback behavior (that non-panic edge still routes health-filtered through the shared-health flat child — a nil-health child would have silently changed it). With once-per-pick pinned, the observable value corrects `2N → N` toward reference parity.
- **AMEND-PT3 (D-PT4 — the retrofit gate CONFIRMS a fix is needed, AND the crux subset asymmetry).** LIVE-PROBED a locality-weighted 2-locality cluster (locality A at `2/5 = 40%` healthy, locality B `5/5 = 100%`, cluster-wide `7/10 = 70% ≥ 50%` → NO cluster-wide panic): locality A's three UNHEALTHY hosts received **ZERO** traffic; A's two healthy hosts each received `18/100`, B's five healthy `~13/100` each — matching the effective-weight split exactly (`A_eff = 50·min(1, 1.4·0.4) = 28`, `B_eff = 50` → `28/78 ≈ 36%` to A, split over its 2 healthy hosts = 18 each). **The reference does NOT apply per-locality local panic** — a single degraded locality NEVER internally flattens, its unhealthy hosts stay at zero (the phase-53 AMEND-P1 per-tier finding, now re-established one construct over rather than assumed to transfer). envoy-go's current `locality.go` DIVERGES: each locality child is built via the shared `buildLeafLB` closure, so the child leaf evaluates its OWN `inPanic` over just that locality's sub-slice — locality A at 40% < 50% → the child LOCALLY panics → its unhealthy hosts WOULD receive traffic. **The retrofit is CONFIRMED needed** (§3.3): build each per-locality child against a PANIC-DISABLED health view (`tierHealth`, `priority.go`'s ADR-0270 primitive — its SECOND consumer, possibly relocated to `health.go` as a shared primitive, a PLAN detail), so the child's per-host `available()` filtering stays fully live but its internal panic branch is permanently dead for a locality child. **SUB-QUESTION (the crux asymmetry, information-only per BRAINSTORM §8 — RESOLVED: NO subset change).** LIVE-PROBED an `lb_subset_config` cluster (subset X at `2/5 = 40%` healthy, subset Y `100%`, all traffic driven to subset X): X's ALL five hosts INCLUDING its three unhealthy each received `24/120`, `lb_healthy_panic` delta = 120 = N. **The reference DOES panic per-subset** — a subset is its own LB host-set, so panic is evaluated over the subset's own hosts and a degraded subset correctly flattens (unhealthy hosts served, counter incremented once per pick). envoy-go's `subsetLB` builds its per-subset children via the SAME shared-health closure, so each subset child's `inPanic` over its own subset endpoints already matches → **subsetLB is already faithful; NO change** (unlike locality). The architectural reason for the asymmetry: a subset is a full host-set with its own panic evaluation; a locality within a locality-weighted cluster is a sub-mechanism WITHIN one host-set, whose panic is evaluated cluster-wide.
- **AMEND-PT4 (D-PT3 — out-of-range values are PGV-REJECTED at boot).** LIVE-PROBED with `envoy --mode validate`: `healthy_panic_threshold: {value: 150}` → **REJECT** (`CommonLbConfigValidationError.HealthyPanicThreshold … PercentValidationError.Value: value must be inside range [0, 100]`); `{value: -10}` → the SAME reject; `{value: 0}` → `configuration OK`. `envoy.type.v3.Percent.value` carries a PGV `gte:0, lte:100` constraint. envoy-go currently accepts ANY value (`>100` yields an unreachable threshold, negative silently disables). **envoy-go mirrors the reject** (§6): a new parse-time reject when the field is PRESENT and its value is outside `[0, 100]` (envoy-go's own byte-stable message per ADR-0080; a boundary value of exactly `0` or `100` is accepted). `go mod tidy` reconfirmed a clean no-op (`git status --porcelain go.mod go.sum` empty before/after; md5sums identical) — ZERO new dependency; `envoy.type.v3.Percent` is already reachable through the existing go-control-plane v1.32.4 dep (already consumed by `parsePanicThreshold`).
- **AMEND-PT5 (D-PT2 — presence semantics CONFIRMED; envoy-go already matches).** LIVE-PROBED: explicit `{value: 0}` at `1/5 = 20%` healthy (a state that panics at the 50% default) → **NO panic** (floor(0)=0; `20 < 0` false) — only the single healthy host served, the four unhealthy at zero; ABSENT → the 50% default; explicit `{value: 50}` → 50. envoy-go's `parsePanicThreshold` nil-check (`p == nil → 0.5`, else `value/100.0` — a fraction today, becoming a floored integer percent per §3.1) ALREADY matches the reference's presence handling — `{value: 0}` disables panic, absent defaults to 50% (the D-LW-OPF0 presence-vs-zero discipline holds for this plain message field with no change needed beyond the AMEND-PT1 floor).
- **AMEND-PT6 (D-PT5/D-PT6 — stat surface ZERO delta + envelope re-check).** A full `/stats` scrape of a panicking cluster, grepped case-insensitively for "panic," found ONLY `cluster.<n>.lb_healthy_panic` (this phase's counter) and `cluster.<n>.lb_subsets_fallback_panic` (a PRE-EXISTING, subset-fallback-specific counter — the NO_FALLBACK-and-no-match case, orthogonal to `healthy_panic_threshold`; out of scope). NO dedicated panic gauge or namespace exists. **Stat surface STAYS 1200 (+0)** — the fixes correct a VALUE on the existing counter, adding no name. Envelope (§3.0): anticipated **~150–260 prod LoC / ~9–11 tasks** (slightly above the BRAINSTORM's 120–250 for the added AMEND-PT1 floor+cross-multiply correction) — FAR under the ADR-0045 gate. NO fuzzer (no wire-decode surface).

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail stays **ADR-0270** at this SPEC (docs-only; no ADR body lands until IMPL per ADR-0044). §13 anchors the **ADR-0271 §Context DRAFT** (the SOLE anticipated ADR — a single-ADR proof-and-hardening phase; §11.7 confirms ZERO seam change, all deltas are comparison-shape + constructor-time health-view wiring + a parse reject). All six BRAINSTORM D-questions (D-PT1..D-PT6) are RESOLVED at this SPEC (§11); none are deferred to PLAN. §12 lists the PLAN/IMPL-level (non-empirical) design questions this SPEC leaves open.

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

Unchanged from the BRAINSTORM's §8 deferred list, now with live-probe confirmation of WHY each is safe to defer:

- **`zone_aware_lb_config`** — stays WHOLESALE-rejected, including `fail_traffic_on_panic` (503-on-panic; declined at Q1 — partially lifting a rejected oneof arm is its own architecture question) and `min_cluster_size`/`routing_enabled` (zone-aware routing needs local-zone awareness envoy-go does not have).
- **The runtime override `upstream.healthy_panic_threshold`** — couples to the absent Runtime/RTDS family; N/A project-wide until such a family opens. (This is the runtime-`getInteger` key that AMEND-PT1 identified as the reference's truncation vector; the truncation itself is mirrored, but the runtime OVERRIDE stays absent.)
- **`membership_healthy` ignores outlier ejections** — the OTHER deferred `REVIEW_FINDINGS.md` cluster finding (declined at Q1 — a general health-observability concern, not panic-scoped); stays on that roster with its siblings (`maximum_ring_size` unenforced; sequential health-probe interval stretch; ring/maglev gauges lost under wrapping).
- **Per-priority-tier panic thresholds** — CONFIRMED N/A (phase-53 AMEND-P1: the reference's multi-tier bypass is the capacity-sum check `Σ_i min(100, OPF × healthy_fraction_i) < 100`, NOT the classic threshold; the configured threshold is correctly INERT in a multi-tier-priority cluster). Documented as a confirmed-correct coverage boundary, NOT deferred work.
- **Degraded-host state in the panic percentage** — the reference folds `degraded_percent` into the panic comparison; envoy-go models HEALTHY/UNHEALTHY only (ADR-0242, no degraded/draining state). A pre-existing, documented coverage boundary; `0097` uses no degraded hosts, so it is not exercised either side.
- **`CommonLbConfig`'s remaining fields** — `update_merge_window`, `ignore_new_hosts_until_first_hc`, `close_connections_on_host_set_change`, `consistent_hashing_lb_config`, `override_host_status` — unchanged, each its own future candidate if ever chartered.
- **A locality-weighted / subset-panic DIFFERENTIAL fixture** — the double-increment fix (AMEND-PT2) and the locality retrofit (AMEND-PT3) are proven by UNIT tests asserting against the D-PT1(iii)/D-PT4 PINNED reference values (delta == N; degraded-locality unhealthy hosts == 0), NOT a new differential dir — honoring the Q2 two-cluster-discriminator scope (`0097` uses plain, non-wrapped clusters where the panic path is single-increment already). §8/§12.

---

## 3. The panic construct (ADR-0271) — proof + four corrections

### 3.0 Split disposition — D-PT6 RESOLVED (single flat row; NO escape valve)

Anticipated **~150–260 prod LoC / ~9–11 tasks** (§1.1 AMEND-PT6) — comfortably under the ADR-0045 `>~25 tasks OR >~1500 LoC` gate. The family's SMALLEST phase (no new policy, no new wrapper, no new package). NO pre-authorized split (no second producer plane or subsystem to couple against — confirmed by §4).

### 3.1 The integer-truncation comparison-shape fix (AMEND-PT1)

The reference panics iff `100.0 × healthy / total < floor(threshold_percent)` (§1.1 AMEND-PT1). envoy-go's current shape diverges in TWO ways: (a) it does not floor the configured value; (b) it compares float fractions rather than the exact integer form. As-built:

```
health.go:70-86   type clusterHealth struct { … panicThreshold float64 … }   // fraction (value/100)
                  newClusterHealth(endpoints []Endpoint, panicThreshold float64) *clusterHealth
health.go:182-188 inPanic:  float64(ch.availableCount(eps)) / float64(total) < ch.panicThreshold   // strict <
health.go:485-491 parsePanicThreshold:  p == nil → 0.5 ; else p.GetValue() / 100.0                 // NO floor, NO range check
```

**The fix (semantics pinned; exact field naming/typing a PLAN detail):** store the threshold as a FLOORED integer percent in `[0, 100]` and compare via an integer cross-multiply:

- `parsePanicThreshold` floors the configured value: `p == nil → 50`; else `floor(p.GetValue())` (an integer percent; the range `[0,100]` is guaranteed by the §6 reject, so no clamp is needed after validation).
- `inPanic` becomes the ULP-safe exact mirror: `total == 0 → false`; else panic iff **`100 * ch.availableCount(eps) < ch.panicThresholdPercent * total`** (strict `<`, per `reference_percent_cap_cross_multiply`).

This changes the stored type (fraction `float64` → integer percent) and the three `newClusterHealth(endpoints, parsePanicThreshold(c))` call sites (`manager.go:461/501/536`) pass the floored integer instead of the fraction — a mechanical type change, no call-graph change. Boundary behavior is preserved exactly at integer thresholds (strict `<`: exactly-at-threshold does NOT panic — D-PT1(i), the `50`/`60`/`80` cases every prior test used), and now matches the reference at non-integer thresholds too (`60.9` at `60%` → `100·3 = 300 < 60·5 = 300` false → no panic, matching the reference; the pre-fix float form gave `0.60 < 0.609` → panic).

### 3.2 The `lb_healthy_panic` double-increment fix (AMEND-PT2)

```
locality.go:142-146
func (lw *localityWeightedLB) Pick(...) (Endpoint, func(), error) {
    if lw.health != nil && lw.health.inPanic(lw.allEndpoints) {
        lw.health.panicInc()                       // <-- REMOVE: the redundant outer increment
        return lw.flat.Pick(hashKey, hasHash, match, hasMatch)
    }
    …
}
```

**The fix:** delete the `lw.health.panicInc()` call at `locality.go:144`, keeping the delegation. On cluster-wide panic the code delegates to `lw.flat.Pick`; the flat child is `factory(endpoints)` bound to the SHARED `clusterHealth`, so its own `panicGate(endpoints)` evaluates `inPanic(endpoints)` — the SAME condition, TRUE — and performs the single `panicInc()` + the blind-all-hosts pick (the flat-uniform distribution D-PT1(iii) confirmed: all hosts, including unhealthy, served, incrementing once per pick). Net: exactly ONE increment per pick (`2N → N`), matching the reference. The flat child KEEPS its shared health (do NOT nil it) — nil-ing it would suppress BOTH the increment (→ under-count, delta 0) AND change the non-panic zero-total-effective-weight fallback behavior. Unit-tested on the locality-weighted path against the D-PT1(iii)-pinned value (delta == N).

### 3.3 The `locality.go` child-local-panic retrofit (AMEND-PT3)

Each per-locality child is currently built via `factory(members)` (`locality.go:103`), where `factory` = the `buildLeafLB` closure bound to the SHARED `clusterHealth` — so a degraded locality's child leaf sees its OWN sub-slice's `inPanic` and LOCALLY flattens (D-PT4: the reference does NOT). **The fix:** build each per-locality child against a PANIC-DISABLED health VIEW — `tierHealth(shared *clusterHealth) *clusterHealth` (`priority.go`, ADR-0270/AMEND-P1-COROLLARY: a view sharing the SAME per-host `states` map so live health results are honored identically, but with the panic threshold set so `inPanic` can never fire) — reused here as its SECOND consumer (relocating it from `priority.go` to `health.go` as a shared primitive is a PLAN detail). The per-locality child's own per-host `available()` filtering stays fully live (skip individual unhealthy hosts), but its internal panic branch becomes permanently dead — so a single degraded locality never internally flattens, its unhealthy hosts stay at zero (matching D-PT4). **The FLAT fallback child KEEPS the shared (panic-ENABLED) health** — it is the one child that MUST detect cluster-wide panic and do the single increment + blind-all pick (§3.2). So the retrofit is scoped to the per-locality children ONLY; the flat child is untouched. **This requires a real constructor/factory signature change** (not merely a wiring detail): `newLocalityWeightedLB`/`newLocalityWeightedLBWithRNG` and their `factory func(sub []Endpoint) (loadBalancer, error)` closure (`manager.go:513-514`) currently bind BOTH the flat and per-locality children to the ONE shared-`health` closure. To hand per-locality children a `tierHealth(health)` view while the flat child keeps the shared `health`, the constructor adopts the health-PARAMETERIZED factory shape — the phase-53 `priorityLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)` / D-P-FACTORY precedent — and the `manager.go:513-514` closure threads its `h` argument through to `buildLeafLB(c, name, sub, h)`; the constructor then calls it with `tierHealth(health)` for each locality group and `health` for the flat fallback. The exact factory-type reuse-vs-local-variant is a PLAN detail (§12 D-PT-TIERHEALTH-HOME), but the signature change itself is load-bearing and NOT optional. `subsetLB` needs NO analogous change (D-PT4 sub-question: per-subset panic IS the reference's behavior; the shared-health subset children are already faithful — §1.1 AMEND-PT3).

### 3.4 The out-of-range validation reject (AMEND-PT4; ADR-0080)

A new parse-time reject: when `common_lb_config.healthy_panic_threshold` is PRESENT and its `value` is outside `[0, 100]` (i.e. `< 0` or `> 100`; exactly `0` and `100` accepted), `buildCluster` returns an error, mirroring the reference's boot-time PGV rejection (§1.1 AMEND-PT4). envoy-go emits its OWN byte-stable message (ADR-0080; the project does not byte-match Envoy's PGV strings), in the established `fmt.Errorf("cluster: %q: …", name)` shape (`manager.go` reject family). **Placement is NOT a free PLAN choice** — the reject MUST fire for ANY cluster carrying the field, but the three `parsePanicThreshold(c)` call sites (`manager.go:461/501/536`) are ALL health-guarded (`:461` needs `hcSpecs`/`outlierCfg`, `:501` needs a `locality_weighted_lb_config` wrap, `:536` needs a multi-tier `priority`), so a PLAIN cluster (no health-checks, no outlier detection, no wrapper) never reaches them even though it can still carry — and the reference still rejects — an out-of-range value. Therefore the range check must be a STANDALONE validation early in `buildCluster` (before the LB switch), UNCONDITIONAL on `health != nil`; folding it into `parsePanicThreshold` alone would silently miss the plain-cluster case (§12 D-PT-REJECT-PLACEMENT). `NaN` need not be handled: `protojson` rejects `NaN` for a `double` field at unmarshal, so a `NaN` value is config-unreachable (and would evade a `< 0 || > 100` check, since all NaN comparisons are false).

### 3.5 The seam REUSE — ZERO new `Pick` parameters (§11.7)

`loadBalancer.Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error)` stays byte-for-byte UNCHANGED — this phase adds no wrapper and no parameter (§4/§11.7). All deltas are (a) the `inPanic`/`parsePanicThreshold` comparison shape, (b) the removal of one `panicInc()` call, (c) constructor-time per-locality health-view wiring, and (d) a parse reject — none touch the `Pick` signature or the exported `Cluster` surface.

### 3.6 Manager acceptance — NO new composition rejects

No new composition reject arms (contrast phase-53's two). The only new reject is the out-of-range value (§3.4). The three `newClusterHealth(endpoints, parsePanicThreshold(c))` call sites (`manager.go:461/501/536`) are unchanged in structure (the argument's type changes per §3.1). The `membership_healthy`/`lb_healthy_panic` registrations (`manager.go:164`/`:166`) are unchanged.

---

## 4. Framework primitives — 0 new seams + 0 new `Endpoint` dimensions + 0 new packages + 0 new go.mod deps

- **NO new framework seam.** No `Pick` signature change, no wrapper, no ctx-carry (§3.5/§11.7).
- **NO new `Endpoint` dimension.** Unlike phase 52 (`Locality`/`LocalityWeight`) and phase 53 (`Priority`), this phase adds no per-endpoint field — the construct is cluster-scoped (`CommonLbConfig`).
- **NO new package.** Edits to existing `internal/cluster` files (`health.go`, `locality.go`, `manager.go`) + their `_test.go` siblings + the `0097` fixture dir.
- **NO new go.mod dep** (D-PT3 / §11.3 — `go mod tidy -diff` empty; `envoy.type.v3.Percent` already reachable).

---

## 5. Proto-field roster

The construct reads a SINGLE proto field, already consumed since phase 39:

| Field | Type | Location | Disposition |
|---|---|---|---|
| `Cluster.common_lb_config.healthy_panic_threshold` | `envoy.type.v3.Percent` (`{ value: float64 }`, field 1) | `cluster.pb.go` (`CommonLbConfig.HealthyPanicThreshold`) | CONSUMED; default 50% when absent; floored to an integer percent (§3.1); PGV `[0,100]` mirrored as a reject (§3.4) |

No other field is newly read. `envoy.type.v3.Percent.value` is a plain `float64` (verified in the v1.32.4 module cache) with a PGV `gte:0, lte:100` constraint (§1.1 AMEND-PT4).

---

## 6. PARSE-REJECT roster (per §3.4 + ADR-0080)

**ONE new reject arm** — the out-of-range `healthy_panic_threshold.value`:

- **Condition:** `common_lb_config.healthy_panic_threshold` present AND `value < 0` OR `value > 100`.
- **Wording discipline:** envoy-go's own byte-stable message in the `cluster: %q: …` family (exact string a PLAN detail; e.g. `cluster: %q: common_lb_config.healthy_panic_threshold: value must be inside range [0, 100]`), NOT a byte-copy of Envoy's PGV string (ADR-0080). A boundary value of exactly `0` or `100` is ACCEPTED (inclusive).
- **Proof:** a unit test (both a `>100` and a `<0` case, plus the accepted `0`/`100` boundaries), per the phase-52/53 reject-test precedent. A cross-side BOOT-REJECT differential dir is NOT added (a separate dir per `reference_differential_fixture_dispatch_constraint` — a unit test suffices, matching the BRAINSTORM §6.1 anticipation; note the `reference_sibling_reject_test_needs_real_typeurl` lesson does not apply here — this is a scalar range check, not an Any-dispatch reject).

No other new reject. No new producer-side reject. The `zone_aware_lb_config` wholesale reject is UNCHANGED (§2).

---

## 7. Stat surface — CONFIRMED ZERO delta (per §11.5)

**Stat surface STAYS 1200 (+0)** — the family's FIFTH zero-stat-delta phase. `lb_healthy_panic` (phase 39) already exists; the AMEND-PT2 double-increment fix corrects its VALUE on the locality-weighted panic path (`2N → N`), adding NO name. No dedicated panic gauge or namespace exists in the reference (§11.5). `lb_subsets_fallback_panic` is a PRE-EXISTING, subset-fallback-specific counter (out of scope — §1.1 AMEND-PT6). Histograms stay deferred project-wide (ADR-0060).

**envoy-go-strict departure flags** (unchanged set): `zone_aware_lb_config` (wholesale reject, pre-existing); the runtime override (absent — no Runtime family; silently N/A, not a reject); the degraded-host panic term (absent — no degraded state). The double-increment fix REMOVES a latent value-level divergence rather than adding a departure.

---

## 8. Differential fixture taxonomy (+1: `0097-lb-panic-threshold`)

### 8.1 `0097-lb-panic-threshold` (cross-side; three arms, hard 0-vs-nonzero + exact-count assertions)

ONE dir, ONE boot per side. One HTTP listener path-routes to THREE health-checked STATIC clusters × 5 hosts each (driver-owned toggle responders, the phase-39/53 pattern), each degraded the SAME 2-of-5 (→ 60% healthy), differing ONLY in `healthy_panic_threshold`:

- **Cluster A (`healthy_panic_threshold: {value: 80}`) — PANICS** (60% < 80). Assert: all 5 hosts INCLUDING the 2 unhealthy get `rq_total > 0` (offset-invariant per `reference_round_robin_offset_randomized` — never host identity/sequence); `lb_healthy_panic == N_A` EXACT (once-per-pick, D-PT1(iii)).
- **Cluster B (field ABSENT → 50% default) — NO panic** (60% ≥ 50; also proves the default arm). Assert: the 2 degraded hosts `== 0`; the 3 healthy sum to `N_B`; `lb_healthy_panic == 0`.
- **Cluster C (`healthy_panic_threshold: {value: 60.9}`) — NO panic** (floor(60.9)=60; `60 < 60` false — the AMEND-PT1 integer-truncation discriminator). Assert: the 2 degraded hosts `== 0`; `lb_healthy_panic == 0`. This arm FAILS against pre-fix envoy-go (`0.60 < 0.609` → panic → unhealthy served) and PASSES post-fix, matching the reference — the differential proof of the truncation fix. This IS an at-the-floor boundary (`60%` healthy vs `floor(60.9) = 60`), but a DETERMINISTIC integer-equal one: both sides compute `100.0 × healthy / total` multiply-first to an exact `60.0` (D-PT1 confirmed the reference does this), so the cross-side `300 vs 60·5=300 → false` comparison is exact — NOT the arbitrary fractional float-parity boundary the Q2 dialogue declined for a one-cluster shape (whose flake risk came from comparing a float healthy-FRACTION against a float threshold near equality). The integer cross-multiply (§3.1) removes even the exact-`60.0` dependency on envoy-go's side.

Drive: degrade 2/5 per cluster → poll `membership_healthy` to 3 per cluster per side (`reference_membership_total_vs_healthy_gauge`: all three clusters ARE health-checked, so the gauge exists both sides) + the warmup-until-K-consecutive-200s gate (`reference_health_check_propagation_warmup`; size per the phase-53 `warmupStable` 10→60 lesson) → N requests per cluster via path routing. Cross-side `StatsAsserter` (`reference_differential_asserter_dispatch`); per-host backend tallies for the distribution assertions. The three cluster NAMES are DISTINCT (`reference_admin_interface_wire_name_collision` — per-name stat accessors disambiguate naturally). Workload constants synced per `reference_fixture_workload_constant_desync`; run selector `-run 'TestDifferential/0097'` per `reference_differential_run_selector`; ≥20-run flake-free gate.

**Deliberate breaks (`-count=1` per `reference_differential_break_protocol_count1`):**
- (a) hardcode `parsePanicThreshold` to `50` ignoring the field → cluster A stops panicking → A's unhealthy-hosts-`>0` + `lb_healthy_panic > 0` assertions FAIL.
- (b) skip the degradation step → all clusters serve healthy-only → A's panic assertions FAIL.
- (c) revert the AMEND-PT1 floor (compare `value/100.0` as a float) → cluster C panics (`0.60 < 0.609`) → C's `unhealthy == 0` / `lb_healthy_panic == 0` FAIL.

### 8.2 NO locality/subset differential dir (§2/§12)

The AMEND-PT2 double-increment fix and the AMEND-PT3 locality retrofit are proven by UNIT tests against the D-PT1(iii)/D-PT4 pinned reference values, NOT a new differential (honoring the Q2 plain-cluster two-cluster-discriminator scope). `0097` uses non-wrapped clusters where the panic path is single-increment already.

### 8.3 NO new BackendKind + NO new fuzzer

`0097` reuses the phase-39/53 toggle-responder pattern (driver-owned); BackendKind tail STAYS **38**. No wire-decode surface → no fuzzer (fuzzers STAY **52**); h2spec/proxy-wasm asserted-unaffected at the six-gate.

### 8.4 Total

Fixtures **98 → 99** at IMPL (`0097-lb-panic-threshold`). A single dir.

---

## 9. Behavior-contract delta (the phase-54 bundle; ADR-0052 atomic landing)

The `BEHAVIOR_CONTRACT.md` delta lands ATOMICALLY at the IMPL (ADR-0052), documenting: the panic comparison shape (strict `<` at a FLOORED integer threshold via integer cross-multiply — the AMEND-PT1 correction); disable-at-0 and absent-defaults-50% presence semantics; the in-panic all-hosts distribution; `lb_healthy_panic` once-per-pick semantics (incl. the corrected locality-weighted path); the per-locality no-local-panic guarantee (retrofit) and the per-subset local-panic behavior (unchanged); the out-of-range `[0,100]` reject; and the confirmed-correct INERTNESS of the classic threshold in a multi-tier-priority cluster (the phase-53 AMEND-P1 capacity-sum bypass governs there instead). NOT at this SPEC (docs-only per ADR-0044).

---

## 10. Per-task structure (~9–11 tasks; the PLAN decomposes)

Indicative TDD spine (the PLAN finalizes bite-sized steps + exact ordering):

1. Baselines + PROGRESS scaffold (fixtures 98, fuzzers 52, stat surface 1200, DECISIONS tail ADR-0270; `go mod tidy -diff` empty).
2. **AMEND-PT1 floor + integer cross-multiply** — failing unit test at a non-integer boundary (`60.9%` threshold, `60%` healthy → NO panic; `67%`/`2-of-3` → panic; the `reference_percent_cap_cross_multiply` non-multiple boundary) → change `parsePanicThreshold` to floor + `inPanic` to cross-multiply + the stored-type change + the three `newClusterHealth` call sites → green.
3. **Parse-path unit hardening** — non-default thresholds threaded through `parsePanicThreshold → newClusterHealth → panicGate` (closing the every-test-hardcodes-0.5 gap): boundary strictness (exactly-at-threshold, D-PT1(i)), disable-at-0 (D-PT2), absent-defaults-50%.
4. **AMEND-PT4 out-of-range reject** — failing unit test (`{value:150}` and `{value:-10}` rejected; `0`/`100` accepted) → add the range reject (§3.4) → green.
5. **AMEND-PT2 double-increment fix** — failing unit test on the locality-weighted panic path (`lb_healthy_panic` delta == N, not 2N, over N picks under cluster-wide panic) → remove the outer `panicInc()` (`locality.go:144`) → green; confirm the flat-uniform all-hosts distribution unchanged.
6. **AMEND-PT3 locality retrofit** — failing unit test (a degraded locality below 50% while cluster-wide ≥ 50% → its unhealthy hosts receive ZERO, D-PT4) → build per-locality children against `tierHealth` panic-disabled views (relocate/share `tierHealth` as needed) → green; confirm `subsetLB` unchanged (a passing regression test that a degraded subset still flattens — the per-subset panic behavior, D-PT4 sub-question).
7. `0097-lb-panic-threshold` fixture (3 arms) + driver (degrade → poll `membership_healthy` → warmup → drive → StatsAsserter + per-host tallies).
8. `0097` deliberate breaks (a)/(b)/(c) `-count=1` + ≥20-run flake-free gate + `-race`.
9. Full 99-dir differential + the six gates (`go build ./...`, `gofmt -l`, `golangci-lint run`, `go vet`, `go mod tidy -diff` empty, `go test ./... -count=1` incl. `-race -short`; the FULL-package `-race` per `reference_full_suite_race_after_background_mutator` — health-check goroutines are background mutators).
10. ADR-0271 body (§Decision/§Consequences, ADR-0044) + `BEHAVIOR_CONTRACT.md` §9 delta.
11. Completion bundle: STATE/ROADMAP advance, row-54 `in-progress → done`, the **Load-balancing family CLOSE**, counts.

(Tasks 5/6 may fold or split at the PLAN's discretion; the FINAL ADR-0045 re-check re-confirms no split.)

---

## 11. SPEC-time empirical-pin block (D-PT1..D-PT6 — executed IN-SESSION 2026-07-08 against `envoyproxy/envoy:contrib-v1.37.2`, `--concurrency 1`, Docker bridge network, per-host `rq_total` ground truth from `/clusters`, per `reference_docker_probe_bridge_network`)

### Summary disposition table (6 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| D-PT1 | Panic semantics battery (boundary / distribution / increment / comparison shape) | (i)/(ii)/(iii) CONFIRMED; **(iv) NEW DIVERGENCE — integer truncation** | AMEND-PT1 (+ AMEND-PT2 fix-shape refinement) |
| D-PT2 | Presence semantics (disable-at-0 / absent / explicit 50) | CONFIRMED — envoy-go already matches | AMEND-PT5 |
| D-PT3 | Out-of-range validation + go.mod | CONFIRMED — reference PGV-rejects `[0,100]`; go.mod no-op | AMEND-PT4 |
| D-PT4 | Locality retrofit gate + subset sub-question | CONFIRMED needed (locality); NO change (subset) — the crux asymmetry | AMEND-PT3 |
| D-PT5 | Stat surface delta | CONFIRMED ZERO | AMEND-PT6 |
| D-PT6 | LoC/task envelope + fuzzer | CONFIRMED (self-assessed) | AMEND-PT6 |

### 11.1 D-PT1 (SPEC-BLOCKING — the semantics battery)

Setup: reference `contrib-v1.37.2`, `--concurrency 1`, STRICT_DNS flat and locality-weighted health-checked clusters, Python HTTP toggle backends (`/healthz` 200/503 per host), active HTTP health check (`interval`/`timeout` 1s, thresholds 1/1) for fast convergence, ground-truthed via per-host `rq_total` deltas from `/clusters` after `membership_healthy` convergence.

- **(i) Boundary strictness — CONFIRMED strict `<`.** Threshold `60`, `3/5 = 60%` healthy (exactly at threshold) → `lb_healthy_panic` delta = 0; the 2 unhealthy hosts got EXACTLY 0; the 3 healthy split 34/33/33. Exactly-at-threshold does NOT panic — matching envoy-go's strict `<`.
- **(ii) In-panic distribution — CONFIRMED all-hosts via the policy's own algorithm.** Threshold `80`, `3/5 = 60%` healthy → PANIC; ALL 5 hosts INCLUDING the 2 `failed_active_hc` got EXACTLY 20 each (flat round-robin across the full set). Matching envoy-go's `panicGate`-then-blind-index.
- **(iii) Increment semantics — CONFIRMED once per pick, on BOTH layers.** Flat cluster (threshold 80, 60% healthy, 100 requests) → `lb_healthy_panic` delta = 100 = N. Locality-weighted cluster driven to cluster-wide panic (locality A 20% + locality B 40% → `3/10 = 30% < 50%`, 100 requests) → delta = 100 = N, with all 10 hosts flat-uniform (10 each). The reference increments EXACTLY ONCE per pick even on the locality-weighted panic path — so envoy-go's `localityWeightedLB.Pick` outer `panicInc()` + the shared-health flat child's own `panicGate` = 2N is the confirmed double-increment (the `REVIEW_FINDINGS.md` deferred finding), and the fix (§3.2 AMEND-PT2) is to remove the redundant outer increment.
- **(iv) Comparison shape at fractional boundaries — NEW DIVERGENCE: the reference INTEGER-TRUNCATES (floors) the threshold.** `2/3 = 66.67%` healthy: threshold `66.7` → NO panic; `66.5` → NO panic; `66` → NO panic; `67` → PANIC (delta = N). `3/5 = 60%` healthy: threshold `60.9` → NO panic. Model: panic iff `100.0 × healthy / total < floor(threshold_percent)` — the value is truncated toward zero to a whole percent, the healthy percentage stays a real double. envoy-go's float form (`0.60 < 0.609`) over-panics at `60.9`/`60%` where the reference does not. Fix §3.1 (floor + integer cross-multiply). See EVIDENCE in-session; all data points fit `floor` (NOT round — `66.5`/`66.7` both truncate to 66, refuting round-half-up).

### 11.2 D-PT2 (SPEC-BLOCKING — presence semantics) — CONFIRMED, envoy-go already matches

Explicit `{value: 0}`, `1/5 = 20%` healthy (a state that panics at the 50% default) → NO panic (floor(0)=0; `20 < 0` false); only the single healthy host served, the four unhealthy at zero. ABSENT → the 50% default (confirmed via the (i) 60%/threshold-60 no-panic case). explicit `{value: 50}` == 50. envoy-go's `parsePanicThreshold` (`p == nil → 0.5`, else `value`) already matches — `{value:0}` disables, absent defaults to 50% — with no change needed beyond the AMEND-PT1 floor.

### 11.3 D-PT3 (validation parity + go.mod) — CONFIRMED

`envoy --mode validate`: `{value: 150}` → `Proto constraint validation failed … CommonLbConfigValidationError.HealthyPanicThreshold … PercentValidationError.Value: value must be inside range [0, 100]` (REJECT); `{value: -10}` → the SAME reject; `{value: 0}` → `configuration OK`. `envoy.type.v3.Percent.value` carries a PGV `[0,100]`. envoy-go accepts anything today → mirror the reject (§3.4/§6). `go mod tidy` no-op (md5sums identical, `git status --porcelain go.mod go.sum` empty) — ZERO new dep.

### 11.4 D-PT4 (SPEC-BLOCKING — the retrofit gate + subset sub-question) — CONFIRMED (locality: fix; subset: no change)

**Locality** (locality-weighted 2-locality cluster, A `2/5 = 40%` healthy, B `5/5 = 100%`, cluster-wide `7/10 = 70% ≥ 50%` → NO cluster-wide panic, 100 requests): locality A's three UNHEALTHY hosts received ZERO; A's two healthy each `18`, B's five healthy `~13` each — matching the effective-weight split (`A_eff = 50·min(1,1.4·0.4) = 28`, `B_eff = 50` → `28/78 ≈ 36%` to A over its 2 healthy hosts = 18 each). The reference does NOT locally panic → envoy-go's per-locality child-local-panic diverges → RETROFIT (§3.3). **Subset** (`lb_subset_config`, subset X `2/5 = 40%` healthy, subset Y `100%`, all 120 requests driven to subset X): X's ALL five hosts including its three unhealthy each received `24`, `lb_healthy_panic` delta = 120 = N. The reference DOES panic per-subset → envoy-go's shared-health subset children already faithful → NO change. The asymmetry (subset = own host-set → per-subset panic; locality = sub-mechanism within one host-set → cluster-wide panic) is the crux.

### 11.5 D-PT5 (stat surface delta) — CONFIRMED ZERO

`/stats` grep `-i panic` on a served cluster → ONLY `cluster.<n>.lb_healthy_panic` (this phase's counter) + `cluster.<n>.lb_subsets_fallback_panic` (PRE-EXISTING subset-fallback counter, orthogonal). No dedicated panic gauge/namespace. Surface **1200 → 1200**. `0097`'s `StatsAsserter` set: `lb_healthy_panic` (per cluster, exact `== N_A` / `== 0`) + `membership_healthy` (per cluster, `== 3`, deterministic once converged) + per-host `rq_total` tallies (distribution).

### 11.6 D-PT6 (LoC/task envelope + fuzzer) — CONFIRMED (self-assessed)

§10's ~9–11-task / ~150–260 prod-LoC estimate (above the BRAINSTORM's 120–250 for the added AMEND-PT1 correction) is comfortably under the ADR-0045 gate — CONFIRMING §3.0's no-split disposition. No dedicated fuzzer (no wire-decode surface).

### 11.7 The seam-reuse confirmation — CONFIRMED (zero `Pick`-signature change)

All deltas are the comparison shape (§3.1), one removed `panicInc()` call (§3.2), constructor-time per-locality health-view wiring (§3.3), and a parse reject (§3.4). No `Pick` signature change, no `WithX` ctx carry, no interface change, no `Cluster` exported-surface change.

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-PT-STORE** — the exact stored representation of the floored threshold (a `uint64`/`int` percent field on `clusterHealth` vs. keeping a `float64` that holds the floored integer): a PLAN typing choice; §3.1 pins the SEMANTICS (floor + integer cross-multiply). Prefer an integer field for clarity and to make the cross-multiply obviously ULP-safe.
- **D-PT-REJECT-PLACEMENT** — the range reject MUST be a STANDALONE, UNCONDITIONAL validation early in `buildCluster` (§3.4): folding it into `parsePanicThreshold` alone is INSUFFICIENT, because all three `parsePanicThreshold(c)` sites are health-guarded and a plain cluster carrying an out-of-range value would evade them (yet the reference rejects it). The PLAN's only latitude is the exact placement of the standalone check (and whether `parsePanicThreshold` ALSO floors, which it must, for the clusters that do build health).
- **D-PT-TIERHEALTH-HOME** — whether `tierHealth` (currently `priority.go`-local, ADR-0270) is relocated to `health.go` as a shared primitive for its second consumer (the locality retrofit), or referenced in place. A PLAN placement choice; the SEMANTICS (a panic-disabled view over the shared per-host state) are fixed. NOTE the retrofit ALSO entails the health-parameterized-factory signature change to `newLocalityWeightedLB(WithRNG)` + the `manager.go:513-514` closure (§3.3, the D-P-FACTORY precedent) — that change is load-bearing, not optional; only the factory-type reuse-vs-local-variant is open.
- **D-PT-UNIT-VS-DIFF** — the corrections (AMEND-PT2/PT3) are unit-proven against the D-PT1(iii)/D-PT4 pinned values (§8.2). If the PLAN judges the unit coverage thin, a locality-weighted-panic differential arm is a possible addition — but NOT committed here (the Q2 scope is the plain two-cluster discriminator; §2).

---

## 13. ADR continuity — the ADR-0271 §Context DRAFT (anchored here; the full entry lands at the IMPL)

**ADR-0271 §Context DRAFT (the healthy-panic-threshold construct: proof + hardening):** `Cluster.CommonLbConfig.healthy_panic_threshold` (`envoy.type.v3.Percent`, field 1 — a `float64` `value` in `[0,100]`, default 50%) is the classic Envoy panic mechanism: when a cluster's healthy fraction drops below the threshold, the load balancer abandons per-host health filtering and routes blindly across ALL hosts (the "better to send traffic to unhealthy hosts than to overload the few healthy ones" posture), reusing the `lb_healthy_panic` counter. The field has been CONSUMED since phase 39 (`parsePanicThreshold`/`inPanic`/`panicGate`, ADR-0242/0243) but NEVER proven — no test exercised a non-default value, no fixture set it, and the classic threshold-panic path was never differentially driven. Phase 54 — the EIGHTH and FINAL Load-balancing-family row, at which the family CLOSES (the fourth family to close, after HTTP-filters @ 25.3 / Network-filters @ 33 / Upstream-robustness @ 43) — supplies the proof (`0097-lb-panic-threshold`, a three-arm cross-side differential holding health constant at 60% and varying ONLY the threshold: an 80% arm that panics, an absent/50% arm that does not, and a 60.9% arm that does not) and, in doing so, its live reference probe (against `envoyproxy/envoy:contrib-v1.37.2`, ADR-0227, `reference_docker_probe_bridge_network`) surfaced FOUR concrete corrections toward reference parity. **(1) Integer truncation (the phase's most consequential finding, NOT anticipated):** the reference truncates the configured threshold toward zero to a WHOLE percent before comparing (`100.0 × healthy / total < floor(threshold_percent)`), while envoy-go compared float fractions and thus OVER-panicked at every non-integer threshold (e.g. 60.9% at 60% healthy — the reference does not panic, envoy-go did); envoy-go now floors the value and compares via an integer cross-multiply (`100 × available < floor(value) × total`, the ULP-safe exact mirror, `reference_percent_cap_cross_multiply`), agreeing with the old form at the integer thresholds every prior test used and correcting the non-integer gap. **(2) The `lb_healthy_panic` double-increment** on the locality-weighted panic path (`localityWeightedLB.Pick` incremented then delegated to its shared-health flat child, whose own `panicGate` incremented again → 2N) is fixed by REMOVING the redundant outer increment — a cleaner fix than the anticipated nil-health flat child, valid because localityWeightedLB's bypass condition is identical by construction to the flat child's own `inPanic` (the deliberate contrast with ADR-0270's `priorityLB`, whose differently-shaped capacity-sum bypass forced its nil-health child), and preserving the zero-total-weight fallback behavior a nil-health child would have changed; the reference's confirmed once-per-pick semantics let `0097` and the unit tests pin the exact value. **(3) The `locality.go` child-local-panic retrofit** — phase 52's accepted coverage boundary (each per-locality child evaluated `inPanic` over its own sub-slice, so a degraded locality internally flattened) and phase 53's explicit D-P-RETROFIT deferral — is CLOSED here, gated on a locality-scoped live probe that confirmed the reference never lets a single degraded locality flatten (its unhealthy hosts stay at zero), by building each per-locality child against a panic-disabled `tierHealth` view (ADR-0270's primitive, its second consumer); the flat fallback child keeps its shared, panic-enabled health so it alone detects cluster-wide panic and does the single increment. The probe ALSO established the crux asymmetry: a `lb_subset_config` cluster DOES panic per-subset (a subset is its own host-set), so envoy-go's shared-health subset children were already faithful — NO subset change. **(4) Out-of-range validation parity:** the reference PGV-rejects `healthy_panic_threshold.value` outside `[0,100]` at boot; envoy-go, which accepted anything, adds a byte-stable parse reject (ADR-0080). The classic threshold is documented as confirmed-correct INERT in a multi-tier-priority cluster (phase-53 AMEND-P1: the capacity-sum bypass governs there, not the classic threshold — a coverage boundary, not a gap). The stat surface gains ZERO new stats (the fixes correct a VALUE on the existing `lb_healthy_panic`) — the family's FIFTH zero-stat-delta phase. NEEDS ZERO new `Pick` parameters (the seam stays byte-stable), ZERO new `Endpoint` dimension, ZERO new package, ZERO new go.mod dependency, ZERO new BackendKind, ZERO new fuzzer. ADR-0024 (per-cluster LB-state-scope) stays UNAMENDED.

§Decision/§Consequences bodies land at the phase-54 IMPL per ADR-0044 (next-free after phase 54 ≈ **ADR-0272**). The PLAN/IMPL may surface additional ADRs — anticipated NONE (a single-ADR proof-and-hardening phase; §11.7 confirms zero seam change).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

- Stat surface: **1200 (+0)**, CONFIRMED (§11.5) — the family's FIFTH zero-stat-delta phase; a VALUE correction on `lb_healthy_panic`, not a name change.
- Differential fixtures: **98 → 99** at IMPL (`0097-lb-panic-threshold`, a single dir).
- Fuzzers: **52 (+0)** — CONFIRMED, no wire-decode surface.
- BackendKind tail: **38 (+0)** — CONFIRMED, reuses the toggle-responder pattern.
- DECISIONS.md tail: **ADR-0270** at this SPEC (docs-only); → **ADR-0271** at the phase-54 IMPL (next-free ADR-0272).
- ROADMAP row 54: STAYS `in-progress` (lifecycle-state 1 → 2 at this SPEC-DONE commit); flips `done` at the phase-54 IMPL six-gate — at which point the **Load-balancing family CLOSES** (its candidate list empties; NO parent rollup, a single flat row per ADR-0106).
- ZERO new Go packages, ZERO new go.mod modules (re-confirmed, §11.3).
- spec-document-reviewer gate applies at this SPEC.

Next → the **phase-54 PLAN** (`superpowers:writing-plans` — decompose §10 into bite-sized TDD tasks; FINAL ADR-0045 gate re-check, anticipated to re-confirm the no-split disposition of §3.0; resolve the §12 D-PT-* PLAN/IMPL design questions).
