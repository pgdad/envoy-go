# Phase 35 Brainstorm — `random` (SECOND Load-balancing-family row; the family STAYS OPEN; the zero-churn seam drop-in)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 35 (`load-balancer-random`), the **SECOND Load-balancing-family row** — landed on the ADR-0232 LB acquire/release seam that phase 34 (`least_request`) built. Phase 35 lands `Cluster.LbPolicy RANDOM` (`envoy.config.cluster.v3`) — the project's **third LB policy** (after the phase-02 `roundRobin` and the phase-34 `leastRequest`). Unlike least_request, RANDOM is a **stateless** policy: it picks a uniformly-random endpoint per `Pick()` and is **insensitive to load**, so it consumes the ADR-0232 seam at ZERO churn — implementing the existing `loadBalancer` interface and returning the shared `noopRelease` (exactly as ADR-0232 §Consequences anticipated for "the non-keyed RANDOM").

The load-bearing facts that shape this brainstorm:

- **The seam already exists and RANDOM is its anticipated zero-churn consumer.** ADR-0232 §Consequences (sharpened at the phase-34 final review) states verbatim: "the RELEASE half of the seam generalizes to every future policy (each returns its own `release func()`, or the shared `noopRelease` when it holds no per-pick state); the non-keyed RANDOM drops in at zero cost (same interface, ignores the counters)." Phase 35 consummates that anticipation. **ZERO seam churn → ZERO seam ADR** (contrast phase 34's TWO ADRs — the seam + the policy). The single ADR is the policy (ADR-0234 anticipated). This mirrors thrift-33's single-ADR-on-reuse precedent (the §9 family's first clean seam reuse).
- **RANDOM has NO config message.** Direct inspection of the v1.32.4 module cache during this brainstorm (the phase-33 §2.1 / phase-34 §2.1 precedent) confirms `Cluster_RANDOM` is `Cluster_LbPolicy` enum value **3** (`config/cluster/v3/cluster.pb.go`) with **NO `Cluster.RandomLbConfig` message** — RANDOM is even simpler than least_request's `choice_count`-bearing `Cluster.LeastRequestLbConfig`. There is **no config-parse arm and no PGV gate at all** at the MVP. (Upstream's `Cluster.RandomLbConfig`-equivalent richness — locality-weighting knobs — lives only on the `load_balancing_policies.random.v3` extension proto via the `Cluster.load_balancing_policy` extension point, the same DEFERRED surface least_request's `selection_method`/`enable_full_scan` live on; §2.1.)
- **No wire decode → the family-level proof-shape novelties CARRY FORWARD.** An LB policy decodes no protocol bytes, so phase 35 adds **NO new fuzzer** (fuzzers STAY **42** — the phase-34 no-fuzzer first repeats) and **NO new BackendKind** (the fixture reuses the plain TCP echo backends of `0001-tcp-proxy-rr` / `0059-lb-least-request`; BackendKind tail STAYS **33**). The differential proof is DISTRIBUTIONAL, not byte-exact: RANDOM is RNG-driven on BOTH sides, so the proof asserts BAND semantics PER SIDE via the EXISTING `DistributionAsserter` (`test/differential/fixture/fixture.go`, the 0001/0003/0059 precedent) — NEVER cross-side-exact (§2.5).
- **The distinguishing proof is ANTI-SKEW.** RANDOM's interesting property is precisely what least_request does NOT have: it ignores load. Fixture `0060-lb-random` REUSES the `0059` held-open-conn skew stimulus but asserts the OPPOSITE outcome — the distribution stays uniform-band PER SIDE *despite* one backend carrying elevated active-conn load (the contrapositive of least_request on the SAME stimulus). This is the strongest true claim that positively separates RANDOM from least_request, and its deliberate-break ("accidentally skews toward/away from load") bites hard (§2.5).

The next sessions author the SPEC then the PLAN then the IMPL (lifecycle-state 1 → 2 for phase 35, skill `superpowers:writing-plans` scoped to **SPEC authoring** — single flat row per Q-split, the kafka-31/thrift-33/least_request-34 precedent). The SPEC executes the §10 empirical-pin obligations (D-R1..D-R6) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network`), and anchors the ADR-0234 §Context draft.

**Brainstorm session:** worktree `.worktrees/phase-35-brainstorm`, branch `phase-35-load-balancer-random-brainstorm`. Substantive predecessor on master: the phase-34 IMPL squash `b086137` (the `leastRequest` policy + the ADR-0232 seam; the Load-balancing family OPENED), with the docs-only SHA-fill routing tips `a450950`/`5815eae`/`f3d32ab` as the literal live tip. Counts at master tip: stat surface **1116**, differential fixtures **61** (tail `0059-lb-least-request`), fuzzers **42** (tail `FuzzThriftDecode`), BackendKind tail **33** (`TCPThriftResponder`), DECISIONS tail **ADR-0233** (next-free **ADR-0234**). ALL counts stay UNCHANGED at this brainstorm.

**Brainstorm mode:** interactive with a live human. The user picked the subject + the differential proof shape via a multi-question dialogue:

- **Q0 subject** — the Load-balancing family STAYS OPEN; phase 35 = **`random`** (`Cluster.LbPolicy RANDOM`, enum value 3 — VERIFIED in the EXISTING core `/envoy v1.32.4` dep, NO config message, §2.1). Picked as the **zero-churn opener** over the seam-extending hash policies (RING_HASH/MAGLEV need a `Pick(hashInput)` widening per ADR-0232 §Consequences — DEFERRED to whichever future phase lands a hash policy). Remaining family candidates after 35: {ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds}.
- **Q1 scope envelope** — **LEAN**: `RANDOM` accepted by `cluster.Manager` (the lb_policy reject TEXT extends its supported-list — a byte-stable-reject touchpoint, blast radius pinned at SPEC D-R2); a stateless `randomLB` picking a uniformly-random endpoint per `Pick()` with the injectable mutex-guarded crypto-seeded `math/rand/v2` PCG (the `leastRequest` RNG posture mirrored exactly). DEFERS: the `load_balancing_policy` extension-point path (where RANDOM's locality-weighting config lives), locality interplay, panic thresholds, and ALL other policies (RING_HASH/MAGLEV/subset/priority stay byte-stable-rejected).
- **Q-seam** — **REUSE the ADR-0232 seam UNCHANGED**: `randomLB` implements the existing `loadBalancer` interface (`Pick() (Endpoint, func(), error)`) returning the shared `noopRelease` (RANDOM holds no per-pick state — it ignores the active counters at zero cost). ZERO seam code, ZERO seam ADR.
- **Q-split sizing** — **SINGLE FLAT ROW 35** (~40–90 prod LoC / ~4–7 tasks, FAR under the ADR-0045 gate — smaller than least_request). NO escape valve needed.
- **Differential proof shape** — the **anti-skew arm**: reuse the `0059` held-open-conn stimulus, assert RANDOM does NOT skew (uniform-band PER SIDE despite the load imbalance) — the contrapositive of least_request.

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0233 — especially ADR-0232 [the LB seam] + ADR-0233 [the least_request policy] + ADR-0024 [the per-cluster RR counter scope]), and the as-built `internal/cluster` package (`loadbalancer.go` / `leastrequest.go` / `manager.go`). Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–34 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/34-load-balancer-least-request/BRAINSTORM.md` section-for-section, reframed for the SECOND family row: a seam REUSE (not a seam build → 1 ADR, not 2), a stateless policy with NO config message (no PGV arm), and an anti-skew differential proof. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-11.

---

## 1. Mission and scope confirmation (35 only)

ROADMAP row `35 | load-balancer-random | 34 | in-progress | | …` (added by this brainstorm) is a **flat top-level Load-balancing-family row** (per ADR-0106 — the family flat-row discipline; NO sub-rows, since Q-split chose a single flat row; sibling family rows are NOT pre-populated). The row's `depends-on` anchor is phase 34 (the last completed phase; substantive predecessor `b086137`).

The Load-balancing family candidate roster at `ROADMAP.md` (§ Feature Families → Load balancing family) immediately BEFORE this brainstorm's registration commit was the post-phase-34 list: `{random, ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds}` (7 candidates). Phase 35 lands **`random`** (this commit marks random IN-PROGRESS). After phase 35 phase-done, **6** family candidates remain: {ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds} — each its own future brainstorm. Branch/directory identifiers: branch `phase-35-load-balancer-random-brainstorm`, directory `35-load-balancer-random/`. **NO new Go package** — the work lands in the EXISTING `internal/cluster` package (a new `random.go` sibling file + the one `manager.go` switch case + the one reject-text line), NOT a new filter package (random is not a filter; §1.5).

Phase 35 is also: (i) the project's **third LB policy** (after `roundRobin` [phase 02] and `leastRequest` [phase 34]) — the `internal/cluster/loadbalancer.go` doc comment (updated at phase 34) reads "Future phases that introduce RANDOM, RING_HASH, MAGLEV, etc. add new types here"; this is the RANDOM phase. (ii) the **FIRST Load-balancing-family row to REUSE the ADR-0232 seam** — the family analogue of thrift-33's clean reuse of the ADR-0230 §9 seam (the build-at-first-consumer/reuse-later logic validated). (iii) a repeat of the phase-34 family-level proof-shape firsts (NO new fuzzer, NO new BackendKind, ZERO stat-surface delta) — now ESTABLISHED for the family, not novel. (iv) **NOT** a seam-touching change at all: `randomLB` consumes the seam exactly as `roundRobin` does (returns `noopRelease`); every existing fixture must pass byte-identically (the seam is unchanged; only a new policy type is added).

### 1.1 What phase 35 delivers as a self-contained whole (envelope: LEAN per Q1)

Phase 35 lands `RANDOM`, as ONE flat row:

1. **The `randomLB` policy** (ADR-0234) — a stateless LB that picks a uniformly-random endpoint per `Pick()` (`rng() % len(endpoints)`; returns `errNoEndpoints` + `noopRelease` on the empty set, mirroring `roundRobin`). The RNG is the injectable mutex-guarded crypto-seeded `math/rand/v2` PCG already built for `leastRequest` (`newPCGRNG` + the `rng func() uint64` field + the `newRandomWithRNG` test seam — `Pick` is concurrent across downstream conns and `math/rand/v2.Rand` is not per-source concurrency-safe; SPEC pins the exact reuse vs. duplication of the RNG helper — D-R5).
2. **Manager acceptance** — `internal/cluster/manager.go` (the lb_policy switch at line ~234) gains a `case clusterv3.Cluster_RANDOM` constructing `randomLB`; the `default` reject TEXT (`manager.go:252`) extends its supported-list from `(supported: ROUND_ROBIN, LEAST_REQUEST)` to `(supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM)` — a **byte-stable-reject touchpoint** whose blast radius is pinned at SPEC (D-R2; the three known sites in §2.6).
3. **The differential fixture `0060-lb-random`** — the anti-skew arm (band-semantics PER SIDE via the existing `DistributionAsserter`: RANDOM stays uniform DESPITE held-conn load) + a cross-side `StatsAsserter` on cluster counters — §6.
4. **The BEHAVIOR_CONTRACT 35 bundle** + the STATE/ROADMAP advance + the row-35 `in-progress → done` flip at the IMPL six-gate (a flat family row — NO parent rollup per ADR-0106).

### 1.2 What phase 35 does NOT deliver (forward to §8)

See §8. Highlights: the `Cluster.load_balancing_policy` extension-point path (where RANDOM's locality-weighting config lives); locality-weighted interplay; panic thresholds; ALL other policies (RING_HASH/MAGLEV/subset/priority — byte-stable-rejected; the hash policies additionally DEFER the ADR-0232 `Pick(hashInput)` widening); any seam change.

### 1.3 Phase-done as the SECOND Load-balancing-family row landing — the family STAYS OPEN

Phase 35 keeps the Load-balancing family OPEN (it opened at phase 34). After phase 35, the family candidate count drops 7 → **6**. The family heading gains a one-line `random DONE` note (the §-family-progress mirror). Sibling rows are NOT pre-populated (ADR-0106). The ADR-0232 seam (built at phase 34) is the family's durable structural asset: phase 35 is its first reuse — `randomLB` ignores the active counters at zero cost, validating the seam's "the non-keyed RANDOM drops in at zero cost" §Consequences claim.

### 1.4 ADR-0045 split readiness — single flat row 35 chosen per Q-split

Per ADR-0045 §6, the split-gate fires at `> ~25 tasks OR > ~1500 LoC`. Phase 35's surface is FAR under it — smaller than least_request (which itself was ~155–255 prod LoC / 9 tasks):

- **The `randomLB` policy** (the uniform pick + the RNG wiring + the manager case + the reject-text change) — ~40–90 prod LoC.
- **Differential/test infra** (the `0060` fixture + the anti-skew band-arm driver + unit tests) — test-side LoC, NOT counted against the gate.

Anticipated **~40–90 prod LoC / ~4–7 tasks** → the **single flat row 35** with NO escape valve (least_request's 34.1/34.2 valve existed because of the seam build; phase 35 builds no seam, so there is nothing to split off). D-R6 re-checks at SPEC.

### 1.5 Seed-stub alignment + package placement

`internal/cluster/loadbalancer.go` IS the seed stub — its doc comment ("Future phases that introduce RANDOM, RING_HASH, MAGLEV, etc. add new types here") names this exact phase. Phase 35 lands INSIDE the existing `internal/cluster` package: the new `randomLB` type in a NEW sibling file `random.go` (the `leastrequest.go` precedent), the acceptance case in `manager.go`, the one reject-text line in `manager.go`. **ZERO new packages** (random is not a filter — there is no `internal/filter/...` analogue), **ZERO new go.mod deps** (the `Cluster.LbPolicy RANDOM` enum is in the EXISTING core `/envoy v1.32.4` dep, already imported by `internal/cluster`; §2.1).

### 1.6 No prebrainstorm-notes branch

No `phase-35-*-prebrainstorm-notes` branch exists. Phase 35 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 35's relationship to the ADR-0232 seam (REUSE, not extend)

Phase 34 built the ADR-0232 seam (the unexported `loadBalancer` interface reshaped to `Pick() (Endpoint, func(), error)` — OPTION C, the exported `Cluster` surface byte-stable). Phase 35 REUSES it UNCHANGED: `randomLB` implements `Pick() (Endpoint, func(), error)` returning `noopRelease`, exactly as `roundRobin` does. NO interface change, NO consumer touch, NO `cluster.go`/`Dial`/`AcquireH1` change. The seam is the family's durable asset; phase 35 is the proof that its RELEASE-half generalization claim holds (ADR-0232 §Consequences). The hash policies (RING_HASH/MAGLEV) are the ones that WILL extend the seam (the PICK-INPUT half — `Pick(hashInput)`); RANDOM does not, which is exactly why it is the zero-churn opener (Q0 rationale).

---

## 2. Design decisions

### 2.1 Subject confirmation: `random` — the v1.32.4 proto surface VERIFIED, NO config message *(Q0 → phase 35 row registered)*

**Decision:** Phase 35 = `Cluster.LbPolicy RANDOM` (`envoy.config.cluster.v3`) — the LEGACY enum path (the same path ROUND_ROBIN + LEAST_REQUEST acceptance uses), NOT the `Cluster.load_balancing_policy` extension point.

**The proto-surface verification (brainstorm finding, the phase-33/34 §2.1 precedent):** direct inspection of the go module cache (`~/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/config/cluster/v3/cluster.pb.go`) during this brainstorm establishes:

```
Cluster_LbPolicy enum (v1.32.4):
  Cluster_ROUND_ROBIN    = 0   (phase 02 — accepted)
  Cluster_LEAST_REQUEST  = 1   (phase 34 — accepted)
  Cluster_RING_HASH      = 2   (rejected; hash policy — needs Pick(hashInput))
  Cluster_RANDOM         = 3   ← THIS PHASE
  Cluster_MAGLEV         = 5   (rejected; hash policy — needs Pick(hashInput))
  Cluster_CLUSTER_PROVIDED          = 6
  Cluster_LOAD_BALANCING_POLICY_CONFIG = 7

Cluster.RandomLbConfig message: ABSENT (no such message exists)
```

There is **NO `Cluster.RandomLbConfig`** on the legacy config surface — RANDOM carries NO config knobs at all (the `lb_config` oneof has members for ring_hash/maglev/least_request/round_robin but NOT random). The only RANDOM-specific configuration upstream exposes (locality-weighting / `LocalityLbConfig`) lives on the SEPARATE `load_balancing_policies.random.v3.Random` extension proto (the `Cluster.load_balancing_policy` extension-point path — present in the v1.32.4 module but NOT the settled config surface; the same DEFERRED path least_request's `selection_method`/`enable_full_scan` live on, phase-34 §2.1). **Consequence:** phase 35 has NO config-parse arm and NO PGV gate — the SIMPLEST policy surface in the family. ZERO new go.mod dep (the EXISTING core `/envoy v1.32.4` dep carries the enum; D-R1 confirms with `go mod tidy`).

**Rationale (the Q0 choice among 7 candidates):** RANDOM is the cheapest next opener because it is the **zero-churn seam drop-in** — ADR-0232 §Consequences names it explicitly ("the non-keyed RANDOM drops in at zero cost"). The hash policies (RING_HASH/MAGLEV) additionally require widening `Pick()` to receive a request-derived hash key (the ADR-0232 PICK-INPUT half — a signature extension the seam anticipates but does not provide at the MVP); they are the seam-EXTENDING phases, deferred to whenever a hash policy is prioritized. subset/locality-weighted/priority/panic-thresholds are MODIFIERS that layer on existing policies, not new selection algorithms — better as later rows. RANDOM accumulates a second family policy at minimal risk and proves the seam's reuse claim.

**Anticipated ADR:** ADR-0234 (the policy ONLY — no seam ADR since the seam is reused unchanged; the thrift-33 single-ADR-on-reuse precedent) — §7. DECISIONS tail STAYS **ADR-0233** at this brainstorm; next-free **ADR-0234**.

### 2.2 Scope envelope: LEAN *(Q1 → single flat row; ADR-0234)*

**Decision:** Deliver: (a) `RANDOM` accepted by `cluster.Manager` (today `internal/cluster/manager.go:252` rejects every lb_policy except ROUND_ROBIN + LEAST_REQUEST; the reject TEXT for the still-rejected policies extends its supported-list — a byte-stable-reject touchpoint, blast radius pinned at SPEC, D-R2); (b) a stateless `randomLB` picking a uniformly-random endpoint per `Pick()` (`rng() % len(endpoints)`; `errNoEndpoints` + `noopRelease` on the empty set). DEFERS (§8): the `load_balancing_policy` extension point (RANDOM's locality-weighting config), locality interplay, panic thresholds, all other policies (RING_HASH/MAGLEV/subset/priority stay rejected with byte-stable text).

**Rationale:** The LEAN envelope isolates exactly one load-bearing piece: the stateless uniform-pick policy in its smallest faithful form. RANDOM has no config to parse and no per-pick state to track, so there is nothing smaller to deliver. What "uniform" means for the picker (full-endpoint-set sampling, no health filtering — there is no active health checking; D-R5 records the boundary) is the only semantic pin, and it mirrors `roundRobin`/`leastRequest`'s full-endpoint-set behavior.

### 2.3 The RNG posture: REUSE the leastRequest injectable PCG *(self-answered; SPEC pins reuse-vs-duplicate, D-R5)*

**Decision:** `randomLB` uses the SAME RNG posture phase 34 built for `leastRequest`: a crypto-seeded `math/rand/v2` PCG, wrapped behind a **mutex-guarded `rng func() uint64`** (Pick is concurrent across downstream conns; `math/rand/v2.Rand` is NOT per-source concurrency-safe), with an injectable test seam (`newRandomWithRNG` — the `newLeastRequestWithRNG` mirror) for deterministic unit tests of the uniform-pick logic. The `crypto/rand` read error threads out of `newRandom` → boot-fail (the `leastRequest` disposition). The SPEC decides whether the `newPCGRNG` helper is shared (extracted, already a package-private helper in `leastrequest.go`) or duplicated (D-R5; anticipated SHARED — it is already package-private in `internal/cluster`).

**Rationale:** RANDOM's entire substance is "uniformly random endpoint" — the RNG IS the policy. Reusing the proven `leastRequest` posture (mutex-guarded, crypto-seeded, injectable) avoids inventing a parallel RNG lifecycle and keeps the deterministic-unit-test discipline identical across the two RNG-driven policies. Note `roundRobin` is the ONLY non-RNG policy; both `leastRequest` and `random` share the PCG posture.

### 2.4 Q-split: single flat row 35, NO escape valve *(Q-split)*

**Decision:** ONE flat row (~40–90 prod LoC / ~4–7 tasks — §1.4), with NO pre-authorized escape valve (least_request's 34.1/34.2 valve existed only to isolate the seam build; phase 35 builds no seam → nothing to split off).

**Rationale:** RANDOM is the smallest possible policy addition — a new type, one manager case, one reject-text line, one fixture. Splitting it would manufacture sub-rows with no independent proof boundary. D-R6 re-checks at SPEC (anticipated NO SPLIT with wide margin).

### 2.5 Differential strategy: anti-skew band + cross-side stats; NO new fuzzer; NO new BackendKind *(Q-differential → fixture envelope §6)*

**Decision:** ONE new fixture `0060-lb-random` reusing the **plain TCP echo backends of `0001-tcp-proxy-rr` / `0059-lb-least-request`** (NO new BackendKind — tail STAYS 33): chain `[tcp_proxy]` on BOTH sides over a multi-endpoint STATIC cluster with `lb_policy: RANDOM`. The **anti-skew arm** — the driver holds open long-lived connections that land on (or are steered to) one backend, elevating its outstanding-active count (the SAME `0059` held-open-conn stimulus); subsequent picks must STAY uniform (RANDOM ignores load — the contrapositive of least_request, which skews AWAY); asserted via the EXISTING `DistributionAsserter` (`test/differential/fixture/fixture.go`, the 0001/0003/0059 precedent) with **BAND semantics PER SIDE** — RANDOM is RNG-driven on both sides, so the assertion is "each side's distribution stays in the uniform band even under the load imbalance," NEVER cross-side-exact; PLUS **(b) cross-side `StatsAsserter`** on cluster counters (`upstream_cx_total` / `upstream_rq_total` per the existing roster; exact cross-equal-vs-per-side disposition pinned at SPEC, D-R4 — anticipated identical to the `0059` disposition: cluster totals cross-equal, per-endpoint spread per-side). **NO new fuzzer** — there is no wire decode anywhere in phase 35 (the enum parse is proto, already fuzzed at the bootstrap surface; there is no config message); fuzzers STAY **42** (the phase-34 no-fuzzer first repeats — now established for the family). Every live assertion proven via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`) — doubly load-bearing for a BAND assertion (a band wide enough to never fail is a dead assertion; the break — "skew toward/away from load" — proves the uniform band bites).

**Rationale:** An LB policy has no bytes to fuzz and no protocol to synthesize — the §9 proof template does not apply (the phase-34 §2.5 reasoning, now established). The anti-skew band proof is the strongest TRUE claim that positively SEPARATES RANDOM from least_request: on the identical held-conn stimulus, least_request (`0059`) skews AWAY from the loaded backend (starvation `c1 <= 12`), while RANDOM (`0060`) stays uniform (every backend in a fair-share band). The deliberate-break is sharp — an implementation that accidentally consulted the active counters (i.e. accidentally implemented least_request) would skew and FAIL the uniform band. Cross-side-exact distribution is IMPOSSIBLE by construction (independent RNGs); band-per-side is the strongest true claim (the `0059` precedent). The fixture is cross-side (it boots the reference with the same RANDOM config), so the `StatsAsserter` prong stays cross-side per `reference_differential_asserter_dispatch`.

### 2.6 Deferred-policy posture: byte-stable reject + the reject-text blast radius *(self-answered; pinned at SPEC, D-R2)*

**Decision:** RING_HASH/MAGLEV (and every other un-implemented lb_policy) STAY rejected at config load with byte-stable text. The current text (`"cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST)"`, `manager.go:252`) names ROUND_ROBIN + LEAST_REQUEST as the supported set; ACCEPTING RANDOM forces a TEXT CHANGE (`… ROUND_ROBIN, LEAST_REQUEST, RANDOM`) — a byte-stable-reject touchpoint. D-R2 pins the blast radius — the EXACT phase-34 three-site pattern, shifted by one policy (verified at brainstorm time):

1. **`internal/cluster/manager.go:252`** — the reject-text string itself.
2. **`internal/cluster/manager_test.go:322`** — `TestManager_Error_UnsupportedLBPolicy` (the phase-34 RETARGET of `TestManager_Error_NonRoundRobinLB`) uses `clusterv3.Cluster_RANDOM` as its reject TRIGGER. RANDOM becoming accepted forces a SECOND retarget — to `Cluster_RING_HASH` (or MAGLEV) — plus the substring assertion `"ROUND_ROBIN, LEAST_REQUEST"` (line ~327) updates to `"ROUND_ROBIN, LEAST_REQUEST, RANDOM"`. (This is the phase-34 "doubly-hit" pattern recurring — the accept of policy N forces the reject test that used N as its trigger to retarget to N+1.)
3. **`docs/envoy-go/BEHAVIOR_CONTRACT.md:899`** — the "NEW lb-policy reject text" entry currently reads "`RANDOM`/`RING_HASH`/`MAGLEV` are rejected … (supported: ROUND_ROBIN, LEAST_REQUEST)"; RANDOM moves OUT of the rejected set (it becomes the third accepted policy) and the supported-set string updates.

NO fixture pins the text → NO boot-reject dir (the phase-34 D-L5 outcome recurs; the cross-side-XOR-boot-reject dispatch constraint per `reference_differential_fixture_dispatch_constraint` would force a separate dir if one were warranted — D-R2 confirms none is).

**Rationale:** The byte-stable-reject discipline (ADR-0080; the §9 D-T4 / phase-34 D-L5 lineage) treats reject text as contract surface: change it ONCE, deliberately, with the blast radius enumerated up front (the three sites above) — never discover the breakage at the six-gate.

### 2.7 Stat surface: anticipated ZERO delta *(self-answered; SPEC pins, D-R4)*

**Decision:** Anticipated stat surface UNCHANGED at **1116** — NO new stat names. random reuses the existing cluster counters (`upstream_cx_total`, `upstream_cx_active`, `upstream_rq_total`, …); the policy changes WHICH endpoint is picked, not what is counted. RANDOM has NO per-endpoint state at all (unlike least_request's LB-internal `atomic.Int64` active counters, which were themselves NOT registry stats). IF the D-R4 probe finds the reference exposing lb-specific cluster stats under RANDOM, the SPEC decides (mirror it, or record an envoy-go-strict departure; anticipated none — RANDOM is the most stat-quiet policy).

**Rationale:** The phase-34 zero-delta hypothesis (the first zero-delta phase) recurs and is now the family expectation. D-R4 is the cheap insurance probe (the `0060` cross-side fixture needs the live reference `/stats` readout anyway).

---

## 3. Framework-survey result — 0 new framework seams + 0 new packages + 0 new go.mod deps (anticipated)

### 3.1 NEW framework seam: NONE *(per Q-seam; the ADR-0232 seam REUSED unchanged)*

Phase 35 builds NO seam — it REUSES the ADR-0232 LB acquire/release seam (`internal/cluster/loadbalancer.go`, the `loadBalancer` interface `Pick() (Endpoint, func(), error)`) UNCHANGED. `randomLB` implements the existing interface and returns the shared `noopRelease` (exactly as `roundRobin` does). This is the family analogue of thrift-33's clean reuse of the ADR-0230 §9 seam — the FIRST reuse of the ADR-0232 seam, validating its "the non-keyed RANDOM drops in at zero cost" §Consequences claim. The hash policies (RING_HASH/MAGLEV) are the seam-EXTENDING future phases (the `Pick(hashInput)` widening — ADR-0232 PICK-INPUT half); RANDOM is NOT.

### 3.2 NEW packages: NONE

The `randomLB` type, the manager acceptance, and the reject-text change all land in the EXISTING `internal/cluster` package (a new `random.go` FILE in the same package — the `leastrequest.go` precedent). random is not a filter — nothing to register in `builtins`, no TypeURL factory, no bootstrap blank-import (the `clusterv3` proto is already imported by `internal/cluster`).

### 3.3 go.mod deps: anticipated ZERO new *(verified at brainstorm; re-pinned at SPEC D-R1)*

`Cluster.LbPolicy RANDOM` (enum value 3) lives in the EXISTING `github.com/envoyproxy/go-control-plane/envoy v1.32.4` dep (`config/cluster/v3/cluster.pb.go` — verified in the module cache during this brainstorm, §2.1; NO config message to import). D-R1 confirms a clean `go mod tidy` and the parse path through the existing `clusterv3` import.

### 3.4 REUSES

- `internal/cluster/` (02/05.2/34) — the `Manager` walk (`manager.go`, the lb_policy switch at line ~234), the ADR-0232 `loadBalancer` interface + the shared `noopRelease` (`loadbalancer.go`), the `leastRequest` RNG helpers (`leastrequest.go` — `newPCGRNG`, the mutex-guarded `rng func() uint64` posture, the `*WithRNG` injectable test seam), the `Cluster.PickEndpoint`/`Dial` pick funnel (UNCHANGED — random consumes it via `noopRelease` exactly as roundRobin does), the ADR-0024 per-cluster state-scope discipline (random has no per-cluster state beyond the endpoint slice + the RNG; ADR-0024 untouched).
- The differential harness — the EXISTING `DistributionAsserter` (`test/differential/fixture/fixture.go`; the 0001/0003/0059 distribution precedent) + `StatsAsserter` + the flat `GET /stats` admin endpoint + the fixture-dispatch/asserter-dispatch/`-count=1` memory constraints — booting the contrib reference image (ADR-0227).
- The plain TCP echo BackendKind (`TCPEcho`, kind-0) of `0001-tcp-proxy-rr` / `0059-lb-least-request` (NO new BackendKind — §2.5).
- `envoy.config.cluster.v3` proto bindings (go-control-plane `/envoy` v1.32.4 — already a dep and already imported by `internal/cluster`; §3.3).

---

## 4. Per-route applicability — none (LB policy is cluster-scoped)

`lb_policy` is a field of `Cluster` — the policy is CLUSTER-scoped, selected per-cluster at bootstrap. There is no `typed_per_filter_config` surface, no per-route override, and no listener interaction. (Upstream's per-route LB overrides — hash policy on routes — belong to the hash-policy rows, not random.) Not applicable to phase 35.

---

## 5. Stat surface hypothesis — anticipated ZERO delta

### 5.1 No new stat names (SPEC pins, D-R4)

random reuses the existing `cluster.<name>.*` counters; the policy changes pick selection, not the stat roster. Anticipated **1116 → 1116**. D-R4 probes the reference's `/stats` under RANDOM for any lb-specific names — if any exist, the SPEC decides mirror-vs-departure (anticipated none).

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The RNG non-equivalence itself (per-side uniform-random streams — a documented behavioral boundary, not a stat departure; the `0059` precedent); the `load_balancing_policy` extension-point config path (DEFERRED — RANDOM's locality-weighting knobs); any reference lb-stat names found at D-R4 and not mirrored. Histograms deferred project-wide (ADR-0060).

---

## 6. Differential fixture envelope — anticipated ONE directory

### 6.1 Fixtures (+1)

- **`0060-lb-random`** (cross-side): chain `[tcp_proxy]` on BOTH sides over a multi-endpoint STATIC cluster with `lb_policy: RANDOM`; the plain TCP echo backends of `0001`/`0059` REUSED. The **anti-skew band arm** — held-open conns elevate one backend's outstanding-active count (the SAME `0059` stimulus); subsequent picks STAY uniform (RANDOM ignores load — the contrapositive of least_request); asserted via the EXISTING `DistributionAsserter` with BAND semantics PER SIDE (RNG-driven — NOT cross-side-exact); PLUS the **cross-side `StatsAsserter`** on cluster counters (cross-equal-vs-per-side disposition per D-R4 — anticipated identical to `0059`). One deliberate-break liveness proof with `-count=1` (skew toward/away from load → fails the uniform band). NO boot-reject dir anticipated (no fixture pins the reject text — the reject lands unit-level in `manager_test.go`; D-R2).

### 6.2 Total

61 → **62**. SPEC pins exact band constants + the establishment-witness protocol (the `0059` hold-N + burst-S template — the held-conn write+read-echo witness per `0059` D-S34-4).

### 6.3 New BackendKind: NONE *(now a family expectation)*

BackendKind tail STAYS **33** (`TCPThriftResponder`). The `0060` fixture reuses the plain TCP echo backends (`TCPEcho`, kind-0). The phase-34 first (an LB phase exercises WHERE connections land, not what the backend speaks) recurs.

### 6.4 New fuzzer: NONE *(now a family expectation)* + no conformance harness

Fuzzers STAY **42** (tail `FuzzThriftDecode`) — phase 35 decodes no wire bytes (§2.5; the phase-34 no-fuzzer first recurs). No new conformance harness; h2spec + proxy-wasm re-run asserted-unaffected at the six-gate (phase 35 adds a policy type but touches no dial-path signature — the seam is unchanged; the six-gate's full 61-dir differential re-verify is the real guard: all 61 existing dirs must stay byte-exact through the new manager case).

---

## 7. Anticipated ADRs — 1 ADR (ADR-0234)

Next-free ADR at master tip is **ADR-0234** (DECISIONS.md tail **ADR-0233** — the least_request policy, ACCEPTED). DECISIONS tail STAYS ADR-0233 at this BRAINSTORM.

- **ADR-0234** *(35 — the random policy)* — the `randomLB` LB: stateless uniform endpoint selection over the full endpoint set (the injectable mutex-guarded crypto-seeded `math/rand/v2` PCG, the `leastRequest` posture); the manager acceptance + the reject-text change + the accept/reject matrix; the seam REUSE (returns `noopRelease`; no seam change → no seam ADR — the thrift-33 single-ADR-on-reuse precedent); the anti-skew band-based differential proof shape; the zero-config / zero-stat-delta / zero-fuzzer / zero-BackendKind firsts (now family expectations).

§Context anchored at SPEC; §Decision/§Consequences body at IMPL per ADR-0044 (next-free after phase 35 ≈ **ADR-0235**). The SPEC/PLAN may surface additional ADRs (each re-checks).

---

## 8. Deferred items

- **The `Cluster.load_balancing_policy` extension point** — the typed-extension LB config path (where upstream's `load_balancing_policies.random.v3.Random` proto with locality-weighting lives). A much larger config seam; a future family row if/when a policy needs it (shared with least_request's deferred `selection_method`/`enable_full_scan` path).
- **Locality-weighted interplay + panic thresholds** — host-set tiering; deferred with the policies that need them.
- **All other policies** — RING_HASH, MAGLEV (the hash policies — each ADDITIONALLY needs the ADR-0232 `Pick(hashInput)` widening), subset LB, locality-weighted LB, priority load balancing, panic thresholds — stay byte-stable-rejected; each a future family row.
- **The weighted/active-load-aware variants** — random is stateless; the active-load-aware family rows reuse the ADR-0232 per-endpoint counters (random ignores them).
- **lb-specific stats / latency histograms** — zero stat delta anticipated (D-R4); histograms deferred project-wide (ADR-0060).
- **Healthy-host filtering beyond the existing no-endpoints error** — the project has no active health checking (the Upstream-robustness family's territory); random samples uniformly over the full endpoint set (D-R5 records the per-side semantics).

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- **02 SPEC (tcp-proxy) deferred-items — "Load-balancer policies other than round-robin"** (`docs/envoy-go/phases/02-tcp-proxy/SPEC.md`: "`LEAST_REQUEST`, `RANDOM`, `RING_HASH`, `MAGLEV`, … → load balancing family") — **PICKED UP here**: random is the SECOND item retired from that phase-02 deferral (after least_request at phase 34); the rest carry forward as the family roster (§1). The phase-02 `loadbalancer.go` doc comment ("Future phases that introduce RANDOM … add new types here") is consummated for RANDOM by this phase.
- **ADR-0232 §Consequences (the LB acquire/release seam)** — its "the non-keyed RANDOM drops in at zero cost (same interface, ignores the counters)" claim is CONSUMMATED here: phase 35 is the seam's first reuse. The seam itself is UNCHANGED. The PICK-INPUT-half widening note (for RING_HASH/MAGLEV) is NOT triggered by random (it is stateless, non-keyed).
- **ADR-0233 (the least_request policy)** — random is the second RNG-driven policy; it REUSES the `leastRequest` injectable mutex-guarded crypto-seeded PCG posture (D-R5; §2.3). No ADR-0233 amendment (random is a sibling policy, not a least_request variant).
- **ADR-0024 (per-cluster RR counter scope)** — UNTOUCHED: random has no per-cluster counter state (the RR counter is roundRobin-only; random has only the endpoint slice + the RNG).
- **The phase-34 BRAINSTORM §8 deferred-items** — "All other policies — RANDOM, RING_HASH, MAGLEV (+ subset/locality/priority/panic) — stay byte-stable-rejected; each a future family row implementing the ADR-0232 seam." Phase 35 retires RANDOM from that list per the post-34 routing.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

The SPEC author executes these IN-SESSION (parallel-subagent fan-out per the 25–34 SPEC precedent) against the contrib reference image (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) + go-control-plane `/envoy` v1.32.4 bindings, using the live-probe precedent (`reference_docker_probe_bridge_network` — bridge network + STRICT_DNS backend hostnames + verify traffic actually ran via `downstream_cx_rx_bytes_total > 0`):

- **D-R1** *(SPEC-BLOCKING for the config surface)* — re-pin the exact v1.32.4 surface: `Cluster.LbPolicy RANDOM` = enum value 3, NO `Cluster.RandomLbConfig` message (§2.1). Confirm a clean `go mod tidy` (ZERO new dep) and the parse path through the existing `clusterv3` import. Confirm there is no `lb_config` oneof member for RANDOM (so a stray `least_request_lb_config`/`ring_hash_lb_config` under a RANDOM cluster is silently ignored — the §6.3 phase-34 silent-ignore-parity disposition; pin the reference's behavior).
- **D-R2** *(SPEC-BLOCKING for the reject contract)* — the accept/reject matrix + the manager reject-text blast radius: confirm the THREE sites (§2.6 — `manager.go:252` + `manager_test.go:322` [the doubly-hit retarget RANDOM → RING_HASH/MAGLEV + the substring assertion] + `BEHAVIOR_CONTRACT.md:899`); the NEW byte-stable text (`… ROUND_ROBIN, LEAST_REQUEST, RANDOM`); confirm no fixture pins the current text → no boot-reject dir; the still-rejected policies' continued-rejection departure (the reference validate-accepts RANDOM/RING_HASH/MAGLEV — record).
- **D-R3** *(SPEC-BLOCKING for the fixture stimulus)* — confirm the reference's RANDOM stays UNIFORM under held-conn load (the anti-skew arm's core claim): apply the `0059` held-open-conn stimulus to a RANDOM cluster and observe the distribution does NOT skew away from the loaded backend (contrast the `0059` least_request skew). Pin the `0060` band constants (the uniform-band width, the hold-N/burst-S workload sizing) + the ≥20-run flake-free protocol + the deliberate-break design (skew toward/away from load). The D-L2 cx-as-rq finding (an open TCP-proxied conn IS one active request) means the held conns DO register as active on the loaded backend — RANDOM simply ignores that, which is exactly the property under test.
- **D-R4** — the stat-surface delta (anticipated ZERO new names — §2.7/§5): scrape the reference `/stats` under RANDOM with live traffic; pin any lb-specific cluster stat names (mirror-vs-departure if found; anticipated none); pin the `0060` `StatsAsserter` counter set + cross-equal-vs-per-side disposition (anticipated identical to `0059`: cluster totals cross-equal, per-endpoint spread per-side).
- **D-R5** — the uniform-sampling/RNG semantics, per-side/unit-level ONLY (RNG never cross-side-comparable): uniform over the full endpoint set (`rng() % n`); healthy-host filtering (none at the MVP — no health checking exists; record the boundary); the RNG source + seeding posture (REUSE the `leastRequest` injectable mutex-guarded crypto-seeded PCG — confirm `newPCGRNG` is sharable as a package-private helper vs. duplicated; anticipated SHARED); the empty-endpoint-set disposition (`errNoEndpoints` + `noopRelease`, the `roundRobin`/`leastRequest` parity).
- **D-R6** — the ADR-0045 envelope re-check at SPEC (LoC/tasks vs the ~40–90 / ~4–7 anticipation; NO escape valve — confirm) + confirm the seam REUSES unchanged (random returns `noopRelease`; no `Pick`-signature change; no consumer touch; ADR-0024 unamended) + the single ADR (ADR-0234, policy only — confirm no seam ADR).

---

## 11. Prior-phase lessons applied

- **Verify the proto surface in the module cache at BRAINSTORM time** (the phase-33/34 §2.1 precedent). Applied: the §2.1 enum-value-3 + NO-config-message verification — which establishes the zero-config / no-PGV-arm shape before the SPEC can assume otherwise.
- **The ADR-0232 seam-reuse claim is load-bearing** (ADR-0232 §Consequences). Applied: RANDOM is the explicitly-named zero-churn drop-in; the Q0 choice (random over the seam-extending hash policies) follows directly from the §Consequences PICK-INPUT-half note.
- **Docker probes need a bridge network** (`reference_docker_probe_bridge_network`). Applied: the D-R3/D-R4 live probes use a shared bridge network + STRICT_DNS backend hostnames + verify traffic ran via `downstream_cx_rx_bytes_total > 0` before trusting any `/stats` readout.
- **Differential break protocol needs `-count=1`** (`reference_differential_break_protocol_count1`). Applied: the `0060` anti-skew arm records a deliberate-break liveness proof with `-count=1` — doubly load-bearing because a BAND assertion can silently go vacuous (a band wide enough to never fail is a dead assertion; the break — skew toward/away from load — proves it bites).
- **Asserter dispatch + liveness** (`reference_differential_asserter_dispatch`). Applied: `0060` is cross-side → the stats prong uses `StatsAsserter`; the distribution prong uses the runner's `DistributionAsserter` hook (driver-side, runs on both paths — the 0001/0003/0059 precedent); every new assertion proven live.
- **The `-run` selector footgun** (`reference_differential_run_selector`). Applied: the SPEC/PLAN/IMPL use `-run 'TestDifferential/0060'` (NOT `-run '0060'`, which matches zero subtests) for the targeted fixture runs.
- **The byte-stable-reject discipline** (ADR-0080; the §9 D-T4 / phase-34 D-L5 lineage). Applied: the manager reject-text change is treated as a contract-surface change with its three-site blast radius enumerated at SPEC (D-R2) — the ONE deliberate text change, with the doubly-hit retarget (RANDOM → RING_HASH/MAGLEV) handled in the same task.
- **The DistributionAsserter precedent** (fixtures `0001`/`0003` RR + `0059` least_request skew). Applied: `0060` reuses the `0059` held-conn skew stimulus but asserts the contrapositive (uniform-stays-uniform) — extending the same hook, not inventing a new assertion seam.
- **Per-task gofmt + golangci-lint** (`feedback_pertask_gofmt_lint`); **subagents commit local-only** (`feedback_subagents_no_push`); **controller squash-merges + pushes at stage-close** (`feedback_push_to_origin`); **work in worktrees** (`feedback_git_worktrees`); **subagent-driven IMPL execution** (`feedback_execution_style`); **pin canonical paths for worktree subagents** (`feedback_subagent_worktree_path_targeting`). Applied at the SPEC/PLAN/IMPL.
- **Wire-format pins** (`reference_wire_format_both_sides_see_same_bytes`) — applied in its GENERALIZED form only: there are no wire bytes here, but the same discipline governs config-parse fidelity (the accept/reject matrix matches the reference's own behavior, D-R2) and the refusal to invent "our band" without probing the reference's actual uniform-under-load behavior (D-R3).

---

## 12. Section closeout

This brainstorm settles: (Q0) phase 35 = `random` (`Cluster.LbPolicy RANDOM`, enum value 3, `envoy.config.cluster.v3` — VERIFIED in the v1.32.4 module cache with NO `Cluster.RandomLbConfig` message; the zero-config simplest family surface), the SECOND Load-balancing-family row — **the family STAYS OPEN**; 6 candidates remain after 35 {ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds}; picked as the **zero-churn seam drop-in** over the seam-extending hash policies (RING_HASH/MAGLEV need the ADR-0232 `Pick(hashInput)` widening — DEFERRED); (Q1) a LEAN envelope — manager acceptance of RANDOM (the reject-text byte-stable touchpoint, three-site blast radius at D-R2) + the stateless uniform-pick `randomLB` (the injectable mutex-guarded crypto-seeded PCG, the `leastRequest` posture); NO config message, NO PGV arm; the `load_balancing_policy` extension point / locality / panic / all-other-policies DEFERRED; (Q-seam) the ADR-0232 seam REUSED UNCHANGED (`randomLB` implements the existing `loadBalancer` interface returning `noopRelease`; ZERO seam code, ZERO seam ADR — the thrift-33 single-ADR-on-reuse precedent; the seam's first reuse, validating its "RANDOM drops in at zero cost" §Consequences claim); (Q-split) a SINGLE FLAT ROW 35 (~40–90 prod LoC / ~4–7 tasks, far under the ADR-0045 gate; NO escape valve — there is no seam build to split off). Self-answered: the differential proof is the ANTI-SKEW arm — fixture `0060-lb-random` reusing the `0059` held-open-conn stimulus but asserting RANDOM STAYS uniform-band PER SIDE despite the load imbalance (the contrapositive of least_request; RNG-driven, never cross-side-exact) + a cross-side `StatsAsserter` prong; **NO new fuzzer** (fuzzers STAY 42) and **NO new BackendKind** (tail STAYS 33; the `0001`/`0059` echo backends reused) — both phase-34 family-level proof-shape firsts now ESTABLISHED as family expectations; stat surface anticipated UNCHANGED at 1116 (D-R4 pins); ZERO new packages (the work lands in `internal/cluster`, a new `random.go` sibling) + ZERO new go.mod deps (verified). Anticipated 1 ADR (ADR-0234 the policy; no seam ADR since reuse; §Context at SPEC, body at IMPL per ADR-0044; DECISIONS tail STAYS ADR-0233 at this brainstorm, next-free ADR-0234), fixtures 61 → 62 anticipated at IMPL (`0060-lb-random`), stat surface 1116 → 1116 anticipated, fuzzers 42 → 42, BackendKind tail 33 → 33. ALL counts UNCHANGED at this brainstorm commit.

The next session authors `docs/envoy-go/phases/35-load-balancer-random/SPEC.md` (`superpowers:writing-plans` scoped to SPEC authoring — single flat row, the kafka-31/thrift-33/least_request-34 precedent), executing the §10 D-R1..D-R6 empirical pins IN-SESSION against the contrib reference Envoy per ADR-0004/ADR-0227 (`reference_docker_probe_bridge_network`), and anchoring the ADR-0234 §Context draft. Per ADR-0106, row 35 registers `in-progress` (flat family row, no sub-phases) at this BRAINSTORM-DONE commit; it flips `in-progress → done` at the phase-35 IMPL six-gate (NO parent rollup).
