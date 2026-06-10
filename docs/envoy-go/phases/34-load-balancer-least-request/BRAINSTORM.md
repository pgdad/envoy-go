# Phase 34 Brainstorm — `least_request` (FIRST Load-balancing-family row; the Load-balancing family OPENS; the project's first LB-policy extension beyond ROUND_ROBIN)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 34 (`load-balancer-least-request`), the **FIRST Load-balancing-family row** — the row that OPENS the Load-balancing family (the first NEW feature family opened since the §9 Network-filters family closed at phase 33). Phase 34 lands `Cluster.LbPolicy LEAST_REQUEST` (config message `Cluster.LeastRequestLbConfig`, `envoy.config.cluster.v3`) — the project's **first LB policy beyond ROUND_ROBIN** (the phase-02 `roundRobin` has been the sole `loadBalancer` implementation since 2026-04). Unlike every §9 row, the subject is not a filter at all: it is a **cluster-scoped endpoint-selection policy**, and its structural piece is the project's **first framework seam OUTSIDE `internal/filter/network`** — the `loadBalancer` acquire/release extension in `internal/cluster` (§2.3).

The load-bearing facts that shape this brainstorm:

- **No wire decode → a FAMILY-LEVEL proof-shape novelty.** An LB policy decodes no protocol bytes, so phase 34 adds **NO new fuzzer** (fuzzers STAY **42** — the FIRST phase since the fuzzing regime began to add none; recorded explicitly as deliberate, §2.5/§6) and **NO new BackendKind** (the fixture reuses the plain TCP echo backends of `0001-tcp-proxy-rr`; BackendKind tail STAYS **33**). The differential proof is DISTRIBUTIONAL, not byte-exact: P2C is RNG-driven on BOTH sides, so the load-skew arm asserts BAND semantics PER SIDE via the EXISTING `DistributionAsserter` (`test/differential/fixture/fixture.go` line ~57, the fixtures-0001/0003 precedent) — NEVER cross-side-exact (§2.5).
- **The structural piece is the seam, not the policy.** The `loadBalancer` interface (`internal/cluster/loadbalancer.go`, currently stateless `Pick() (Endpoint, error)`, sole impl `roundRobin`) extends to an ACQUIRE/RELEASE shape so the LB can own per-endpoint outstanding-active-request counters — the state least_request (and every future active-load-aware policy) needs. Every future Load-balancing-family row reuses this seam (the ADR-0230 build-at-first-consumer / reuse-later logic, transplanted from the §9 family). Hence **2 anticipated ADRs**: ADR-0232 (the seam) + ADR-0233 (the policy) — §7.
- **The v1.32.4 proto surface was VERIFIED in the go module cache during this brainstorm (the phase-33 §2.1 verification precedent), with a LOUD finding (§2.1):** `Cluster_LeastRequestLbConfig` in `go-control-plane/envoy@v1.32.4/config/cluster/v3/cluster.pb.go` carries EXACTLY THREE knobs — `choice_count` (`UInt32Value`, PGV `>= 2` when set, default 2), `active_request_bias` (`RuntimeDouble`), `slow_start_config` — and **NO `enable_full_scan` and NO `selection_method` enum**. Those two knobs exist ONLY in the SEPARATE `load_balancing_policies.least_request.v3.LeastRequest` extension proto (the `Cluster.load_balancing_policy` extension-point path — present in the v1.32.4 module but NOT the settled config surface). So the Q1 envelope's conditional FULL_SCAN arm ("IFF the v1.32.4 proto + the v1.37.2 reference support it") FAILS its proto leg on the settled surface — the anticipated D-L3 outcome is FULL_SCAN DEFERRED and the deterministic exact-count fixture arm DROPPED (the SPEC makes the final call — §2.2/§10 D-L3).

The next sessions author the SPEC then the PLAN then the IMPL (lifecycle-state 1 → 2 for phase 34, skill `superpowers:writing-plans` scoped to **SPEC authoring** — single flat row per Q-split, the kafka-31/thrift-33 precedent). The SPEC executes the §10 empirical-pin obligations (D-L1..D-L7) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network`), and anchors the ADR-0232 + ADR-0233 §Context drafts.

**Brainstorm session:** worktree `.worktrees/phase-34-brainstorm`, branch `phase-34-load-balancer-least-request-brainstorm`. Substantive predecessor on master: the phase-33 IMPL squash `97aefd6` (the `thrift_proxy` terminal proxy; the §9 family CLOSED), with the CI-fix merge `4252c1b` (PR #1 — test/harness/workflow-only) above it and the docs-only routing tips `6721618`/`e1da5b7` as the literal live tip. Counts at master tip: stat surface **1116**, differential fixtures **60** (tail `0058-thrift-boot-reject`), fuzzers **42** (tail `FuzzThriftDecode`), BackendKind tail **33** (`TCPThriftResponder`), DECISIONS tail **ADR-0231** (next-free **ADR-0232**). ALL counts stay UNCHANGED at this brainstorm.

**Brainstorm mode:** interactive with a live human. The user picked the family + subject and each major design decision via a multi-question dialogue:

- **Q0 family + subject** — the **Load-balancing family** OPENS (the first new family after the §9 closure); first subject = **`least_request`** (`Cluster.LbPolicy LEAST_REQUEST` + `Cluster.LeastRequestLbConfig`, anticipated in the EXISTING core `/envoy v1.32.4` dep — VERIFIED, §2.1). Remaining family candidates after 34: {random, ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds}.
- **Q1 scope envelope** — **LEAN**: `LEAST_REQUEST` accepted by `cluster.Manager` (the manager reject TEXT changes — a byte-stable-reject touchpoint, blast radius pinned at SPEC, D-L5); a `leastRequest` LB implementing upstream's P2C (sample `choice_count` random hosts, pick the fewest-outstanding-active-requests host; `choice_count` default 2, config-parsed + PGV-validated); the FULL_SCAN selection mode ONLY IF both the v1.32.4 proto and the v1.37.2 reference support it (the proto leg already FAILED on the settled surface — §2.1; D-L3 finalizes). DEFERS: `active_request_bias`, `slow_start`, locality interplay, panic thresholds, and ALL other policies (RANDOM/RING_HASH/MAGLEV stay rejected with byte-stable text).
- **Q-seam** — the `loadBalancer` interface extends to an ACQUIRE/RELEASE shape (Pick returns the endpoint plus a release handle, or an equivalent PickAcquire/Release pair — the exact exported surface pinned at SPEC). The LB owns per-endpoint atomic counters; the dial path releases on conn close; `roundRobin` ignores the counters (behavior-neutral). Touches every Pick consumer mechanically but behavior-neutrally (§2.3).
- **Q-split sizing** — **SINGLE FLAT ROW 34** (~300–600 prod LoC / ~10–14 tasks, well under the ADR-0045 gate) + a pre-authorized 34.1-seam / 34.2-policy escape valve (expected unconsumed).

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0231 — especially ADR-0024, the per-cluster RR counter-scope ADR whose territory the seam touches), and the as-built `internal/cluster` package. Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–33 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/33-network-filter-thrift-proxy/BRAINSTORM.md` section-for-section, reframed for a NON-filter subject: a cluster-scoped LB policy with a seam in `internal/cluster` (not `internal/filter/network`), a distributional (not byte-exact) differential proof, and the two family-level firsts (no new fuzzer, no new BackendKind). Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-06-10.

---

## 1. Mission and scope confirmation (34 only)

ROADMAP row `34 | load-balancer-least-request | 33 | in-progress | | …` (added by this brainstorm) is a **flat top-level Load-balancing-family row** (per ADR-0106 — the family flat-row discipline; NO sub-rows, since Q-split chose a single flat row; sibling family rows are NOT pre-populated). The row's `depends-on` anchor is phase 33 (the last completed phase; substantive predecessor `97aefd6`).

The Load-balancing family candidate roster at `ROADMAP.md` (§ Feature Families → Load balancing family) immediately BEFORE this brainstorm's registration commit was the unopened one-line list: `least_request, random, ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds`. Phase 34 OPENS the family and lands **`least_request`** (this commit adds the family-OPEN paragraph marking least_request IN-PROGRESS). After phase 34 phase-done, **7** family candidates remain: {random, ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds} — each its own future brainstorm. Branch/directory identifiers: branch `phase-34-load-balancer-least-request-brainstorm`, directory `34-load-balancer-least-request/`. **NO new Go package** — the work lands in the EXISTING `internal/cluster` package (`loadbalancer.go` + `manager.go` + `cluster.go`), NOT a new filter package (least_request is not a filter; §1.5).

Phase 34 is also: (i) the project's **first LB-policy extension beyond ROUND_ROBIN** — `internal/cluster/loadbalancer.go` has carried the "Future phases that introduce LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV, etc. add new types here" comment since phase 02; this is that phase. (ii) the carrier of the project's **first framework seam OUTSIDE `internal/filter/network`** — the LB acquire/release extension in `internal/cluster` (§2.3; ADR-0232), the Load-balancing-family analogue of the §9 chain framework: built at the family's first consumer, reused by every subsequent family row. (iii) the **first phase since the fuzzing regime began to add NO fuzzer** and the **first §-differential phase since BackendKinds began to add NO BackendKind** (no wire decode; both firsts deliberate — §2.5/§6). (iv) a **`tcp_proxy`/HTTP-router-agnostic** change: the seam touches every `PickEndpoint`/`Dial` consumer (tcpproxy `filter.go:127`, the HTTP router `router.go:662`, httpclient `httpclient.go:280`, grpcclient, `dial_h2.go`, the ADR-0230 upstream-pool dial closures in redisproxy/thriftproxy) MECHANICALLY but BEHAVIOR-NEUTRALLY (§2.3).

### 1.1 What phase 34 delivers as a self-contained whole (envelope: LEAN per Q1)

Phase 34 lands `LEAST_REQUEST`, as ONE flat row:

1. **The `loadBalancer` acquire/release seam** (`internal/cluster/loadbalancer.go`; ADR-0232) — the interface extends from the stateless `Pick() (Endpoint, error)` to an acquire/release shape: Pick returns the endpoint PLUS a release handle (or an equivalent PickAcquire/Release pair — the exact exported surface pinned at SPEC, D-L7 territory). The LB owns per-endpoint `atomic.Int64` (or equivalent) outstanding-active counters; acquire increments at pick, release decrements when the picked connection closes. The dial path wires release into the conn-close path (the existing `connWithGauge` Close-once wrapper — ADR-0063's Inc-then-wrap, `sync.Once`-guarded — is the anticipated attach-point; pinned at SPEC). `roundRobin` adopts the new shape but IGNORES the counters — provably behavior-neutral (its pick sequence is byte-for-byte unchanged; the phase-02 unit-pinned first-pick-is-`endpoints[0]` property and the `0001`/`0003` fixtures must pass unchanged).
2. **The `leastRequest` LB** (ADR-0233) — upstream's P2C: sample `choice_count` random hosts (default 2), pick the one with the fewest outstanding active requests (tie-break + with/without-replacement sampling semantics pinned at SPEC, D-L6 — per-side/unit-level only; RNG is never cross-side-comparable).
3. **Manager acceptance** — `internal/cluster/manager.go` (the lb_policy guard at line ~216) accepts `LEAST_REQUEST` alongside `ROUND_ROBIN`; parses + PGV-validates `Cluster.LeastRequestLbConfig.choice_count` (`>= 2` when set, default 2 — verified §2.1); the reject TEXT for the still-rejected policies changes ("only ROUND_ROBIN lb_policy supported" can no longer be the message) — a **byte-stable-reject touchpoint** whose blast radius (which unit tests / fixtures pin the current text — `internal/cluster/manager_test.go` is a known pinner, the phase-02 `Error_NonRoundRobinLB` arm) is pinned at SPEC (D-L5).
4. **The FULL_SCAN selection mode** — ONLY IF supported by both the v1.32.4 proto (already REFUTED on the settled `Cluster.LeastRequestLbConfig` surface — §2.1) and the v1.37.2 reference; anticipated DEFERRED at D-L3.
5. **The differential fixture `0059-lb-least-request`** — the slow/held-open-backend skew arm (band-semantics PER SIDE via the existing `DistributionAsserter`) + cross-side `StatsAsserter` on cluster counters (+ the FULL_SCAN deterministic arm only if D-L3 revives it) — §6.
6. **The BEHAVIOR_CONTRACT 34 bundle** + the STATE/ROADMAP advance + the row-34 `in-progress → done` flip at the IMPL six-gate (a flat family row — NO parent rollup per ADR-0106).

### 1.2 What phase 34 does NOT deliver (forward to §8)

See §8. Highlights: `active_request_bias` (the weighted variant — parse-accepted-or-rejected per D-L1/D-L5); `slow_start_config`; FULL_SCAN (anticipated — pending D-L3); the `Cluster.load_balancing_policy` extension-point path (where upstream's `selection_method`/`enable_full_scan` actually live); locality-weighted interplay; panic thresholds; ALL other policies (RANDOM/RING_HASH/MAGLEV/subset/priority — byte-stable-rejected); seam consumption beyond least_request.

### 1.3 Phase-done as the FIRST Load-balancing-family row landing — the family OPENS

Phase 34 OPENS the Load-balancing family (registered at this brainstorm's ROADMAP edit). After phase 34, the family candidate count drops 8 → **7**. The family heading gains an OPEN paragraph (the §9-closure mirror, much shorter); sibling rows are NOT pre-populated (ADR-0106). The seam (ADR-0232) is the family's durable structural asset: random (stateless, trivially ignores the counters), ring_hash/maglev (hash-keyed, ignore the counters), and the active-load-aware future variants all implement the same acquire/release interface.

### 1.4 ADR-0045 split readiness — single flat row 34 chosen per Q-split

Per ADR-0045 §6, the split-gate fires at `> ~25 tasks OR > ~1500 LoC`. Phase 34's surface is anticipated WELL under it:

- **The seam** (interface reshape + per-endpoint counters + release wiring through the dial/close path + `roundRobin` adoption) — ~120–250 prod LoC (mechanical consumer touches included).
- **The `leastRequest` policy** (P2C sampling + config parse/validate + manager acceptance + the reject-text change) — ~120–250 prod LoC.
- **Differential/test infra** (the `0059` fixture + the band-arm driver + unit tests) — test-side LoC, NOT counted against the gate.

Anticipated **~300–600 prod LoC / ~10–14 tasks** → BOTH legs fit comfortably UNDER the gate → the **single flat row 34** with a **pre-authorized 34.1-seam / 34.2-policy escape-valve split** that stays available if the SPEC's empirical estimate or the PLAN's re-check trips the gate (expected unconsumed — the kafka-31/thrift-33 precedent). The split axis, if consumed: 34.1 = the acquire/release seam + `roundRobin` adoption (behavior-neutral, proven by the existing fixtures passing unchanged); 34.2 = the `leastRequest` policy + manager acceptance + the `0059` fixture. D-L7 re-checks at SPEC.

### 1.5 Seed-stub alignment + package placement

`internal/cluster/loadbalancer.go` IS the seed stub — its phase-02 doc comment ("Future phases that introduce LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV, etc. add new types here") names this exact phase. Phase 34 lands INSIDE the existing `internal/cluster` package: the reshaped `loadBalancer` interface + the new `leastRequest` type in `loadbalancer.go` (or a sibling file in the same package), the acceptance/parse change in `manager.go`, the release wiring in `cluster.go`. **ZERO new packages** (least_request is not a filter — there is no `internal/filter/...` analogue; the honest departure from the §9 template's "1 NEW filter package" row), **ZERO new go.mod deps** (the `Cluster.LeastRequestLbConfig` proto is in the EXISTING core `/envoy v1.32.4` dep — verified §2.1).

### 1.6 No prebrainstorm-notes branch

No `phase-34-*-prebrainstorm-notes` branch exists. Phase 34 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 34's relationship to prior framework deltas

Every prior structural seam lives under `internal/filter/` (07.1 HTTP filter framework → 26.1 `internal/filter/network/` read-filter framework → 26.2 `TerminalFilter` → 28.1a `WriteFilter` / 28.1b read seam → 29.3 async halt/resume → 32.1 the ADR-0230 upstream-pool seam). **Phase 34's seam is the FIRST OUTSIDE `internal/filter/network`** — it reshapes the `internal/cluster` `loadBalancer` interface (phase 02, ADR-0024 territory). The seam is the Load-balancing family's analogue of the 26.1 chain framework / the 32.1 pool seam: built at the family's first consumer (least_request), reused by every later family row (the ADR-0230 build-at-first-consumer logic — validated when thrift-33 reused that seam unchanged). ADR-0024 (the per-cluster `atomic.Uint64` RR counter scope) is NOT amended: the RR counter stays per-cluster and untouched; the NEW per-endpoint active counters are LB-owned state in the same per-cluster scope discipline (the SPEC confirms no ADR-0024 amendment is needed — D-L7 territory). The ADR-0230 upstream pools, the tcpproxy dial path, the HTTP router, httpclient, and grpcclient are all MECHANICAL consumers of the reshaped Pick surface — behavior-neutral at this phase (every existing fixture must pass byte-identically).

---

## 2. Design decisions

### 2.1 Subject confirmation: `least_request` — the v1.32.4 proto surface VERIFIED, with a FULL_SCAN-knob finding *(Q0 → phase 34 row registered)*

**Decision:** Phase 34 = `Cluster.LbPolicy LEAST_REQUEST` with config message `Cluster.LeastRequestLbConfig` (`envoy.config.cluster.v3`) — the settled config surface is the LEGACY enum path (the same path the existing ROUND_ROBIN acceptance uses), NOT the `Cluster.load_balancing_policy` extension point.

**The proto-surface verification (brainstorm finding, the phase-33 §2.1 precedent):** direct inspection of the go module cache (`~/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/config/cluster/v3/cluster.pb.go` + `cluster.pb.validate.go`) during this brainstorm establishes:

```
Cluster_LeastRequestLbConfig (v1.32.4 — EXACTLY three fields):
  choice_count         *wrapperspb.UInt32Value      field 1; PGV: value >= 2 when set; default 2 (doc comment)
  active_request_bias  *corev3.RuntimeDouble        field 2; the weighted-RR variant knob — DEFERRED (Q1)
  slow_start_config    *Cluster_SlowStartConfig     field 3; DEFERRED (Q1)
  enable_full_scan     — ABSENT
  selection_method     — ABSENT
```

`enable_full_scan` / `selection_method` (enum `N_CHOICES=0` / `FULL_SCAN=1`) exist ONLY in `envoy@v1.32.4/extensions/load_balancing_policies/least_request/v3/least_request.pb.go` — the `LeastRequest` extension proto consumed via the `Cluster.load_balancing_policy` extension point, a DIFFERENT (and much larger) config surface that is NOT in the phase-34 envelope. **Consequence:** the Q1 conditional FULL_SCAN arm fails its proto leg on the settled surface — anticipated D-L3 outcome: FULL_SCAN DEFERRED, the deterministic exact-count fixture arm DROPPED, fixture `0059` carries the band arm + the stats arm only. The SPEC's D-L3 probe still runs (it pins how the v1.37.2 reference behaves and whether any alternative deterministic arm exists), but the brainstorm records the strong prior. ZERO new go.mod dep regardless (the EXISTING core `/envoy v1.32.4` dep carries `Cluster.LeastRequestLbConfig` — the redis D32-1 / thrift D-T1 posture; D-L1 confirms with `go mod tidy`).

**Rationale:** least_request is the canonical family opener: it is upstream's most-used non-RR policy, it forces the acquire/release seam every active-load-aware policy needs (the family's durable asset), and its config surface is small (one PGV-validated knob at the MVP). The hash policies (ring_hash/maglev) need a hash-key plumbing seam instead; random is trivial but seam-less (it would open the family without building the family's asset); subset/locality/priority/panic are modifiers, not policies — all better as later rows.

**Anticipated ADRs:** ADR-0232 (the seam) + ADR-0233 (the policy) — §7. DECISIONS tail STAYS **ADR-0231** at this brainstorm; next-free **ADR-0232**.

### 2.2 Scope envelope: LEAN *(Q1 → single flat row; ADR-0233)*

**Decision:** Deliver: (a) `LEAST_REQUEST` accepted by `cluster.Manager` (today `internal/cluster/manager.go` line ~216 rejects every lb_policy except ROUND_ROBIN; the reject TEXT for the still-rejected policies necessarily changes — a byte-stable-reject touchpoint, blast radius pinned at SPEC, D-L5); (b) a `leastRequest` LB implementing upstream's P2C — sample `choice_count` random hosts, pick the host with the fewest outstanding active requests; `choice_count` default 2, config-parsed + PGV-validated (`>= 2` when set — §2.1); (c) the FULL_SCAN selection mode ONLY IF both proto legs hold (the v1.32.4 leg already FAILED on the settled surface — §2.1; D-L3 finalizes; anticipated DEFERRED). DEFERS (§8): `active_request_bias` (when unset, upstream treats all-equal-weight hosts via pure P2C — the MVP's case; the weighted variant is a future row/sub-phase), `slow_start_config`, locality interplay, panic thresholds, all other policies (RANDOM/RING_HASH/MAGLEV stay rejected with byte-stable text), the `load_balancing_policy` extension point.

**Rationale:** The LEAN envelope isolates exactly two load-bearing pieces: the seam (the family asset) and the un-weighted P2C (the policy in its smallest faithful form). `active_request_bias` only takes effect when host weights are unequal (the v1.32.4 doc comment, §2.1) — and the project's STATIC clusters at the MVP carry equal weights, so deferring it loses nothing observable. What "active request" means for a TCP-proxied connection in the reference (cx-as-rq?) is the pivotal semantic pin — D-L2, probed with held-open conns — because it determines what the per-endpoint counter counts and what the `0059` skew arm exercises.

### 2.3 The LB acquire/release seam *(Q-seam → §3.1; ADR-0232)*

**Decision:** Extend the `loadBalancer` interface (`internal/cluster/loadbalancer.go`; currently stateless `Pick() (Endpoint, error)`; sole impl `roundRobin`) to an ACQUIRE/RELEASE shape: Pick returns the endpoint PLUS a release handle (or an equivalent PickAcquire/Release pair — the exact exported surface pinned at SPEC). The LB owns per-endpoint atomic outstanding-active counters: acquire increments at pick; the dial path releases (decrements) on conn close — the existing `connWithGauge` Close-once wrapper (`cluster.go` Dial; ADR-0063's Inc-then-wrap with `sync.Once`) is the anticipated release attach-point (no double-release by construction). `roundRobin` adopts the shape but IGNORES the counters — behavior-neutral (pick sequence unchanged; the ADR-0024 per-cluster RR counter untouched). EVERY Pick consumer is touched mechanically: `Cluster.PickEndpoint()` (`cluster.go:173`) and `Cluster.Dial` (`cluster.go:~198` (decl; interior dial/wrap sites ~202/246)) carry the handle internally, so most external consumers (tcpproxy `filter.go:127`, the HTTP router `router.go:662`, grpcclient, `dial_h2.go`, the ADR-0230 pool dial closures) see NO signature change if the SPEC routes the release entirely through Dial's returned conn; the direct `PickEndpoint` consumers (httpclient `httpclient.go:280`, the thriftproxy no-healthy-host probe `filter.go:90`) are the sites where the exact exported surface matters — pinned at SPEC. The release-on-close semantics for un-dialed picks (pick succeeded, dial failed) must be defined (release immediately on dial failure — anticipated; SPEC pins).

**Rationale:** least_request's entire substance is "fewest outstanding active requests" — the counter is the policy. Putting the counters in the LB (not the Cluster, not the conn) keeps the seam reusable: every future active-load-aware family row needs exactly this state, and the stateless policies ignore it at zero cost. Releasing via the existing Close-once wrapper reuses a proven discipline (ADR-0063) instead of inventing a parallel lifecycle. Behavior-neutrality for `roundRobin` makes the seam separately verifiable: if the SPEC/PLAN split (34.1-seam) were consumed, the seam lands proven by the EXISTING fixture suite passing byte-identically.

### 2.4 Q-split: single flat row 34 + pre-authorized 34.1/34.2 escape valve *(Q-split)*

**Decision:** ONE flat row (~300–600 prod LoC / ~10–14 tasks — §1.4), with the pre-authorized 34.1-seam / 34.2-policy escape valve standing unconsumed unless the SPEC (D-L7) or the PLAN re-check trips the ADR-0045 gate.

**Rationale:** Both deliverables are small and the seam is only provable as "reused" once least_request consumes it — splitting by default would manufacture a 34.1 whose only proof is "nothing changed." The escape valve exists because the consumer-touch surface (every Pick consumer) is the one place mechanical LoC could surprise.

### 2.5 Differential strategy: band-based distribution + cross-side stats; NO new fuzzer; NO new BackendKind *(self-answered → fixture envelope §6; a FAMILY-LEVEL proof-shape novelty)*

**Decision:** ONE new fixture `0059-lb-least-request` reusing the **plain TCP echo backends of `0001-tcp-proxy-rr`** (NO new BackendKind — tail STAYS 33): chain `[tcp_proxy]` on BOTH sides over a multi-endpoint STATIC cluster with `lb_policy: LEAST_REQUEST`. Arms: **(a) the load-skew arm** — the driver holds open long-lived connections that land on (or are steered to) one backend, elevating its outstanding-active count; subsequent picks must skew AWAY from the loaded backend; asserted via the EXISTING `DistributionAsserter` (`test/differential/fixture/fixture.go` line ~57; the fixtures-0001/0003 precedent) with **BAND semantics PER SIDE** — P2C is RNG-driven on both sides, so the assertion is "each side's distribution lands in the expected band," NEVER cross-side-exact; **(b) the FULL_SCAN deterministic exact-count arm** — ONLY IF D-L3 revives it (anticipated DROPPED — §2.1); **(c) cross-side `StatsAsserter`** on cluster counters (`upstream_cx_total` / `upstream_rq_total` per the existing roster; exact cross-equal-vs-per-side disposition pinned at SPEC, D-L4). **NO new fuzzer** — there is no wire decode anywhere in phase 34 (config parse is proto/PGV, already fuzzed at the bootstrap surface); fuzzers STAY **42**, the FIRST phase since the fuzzing regime began to add none — recorded as DELIBERATE, not an omission. Every live assertion proven via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1`).

**Rationale:** An LB policy has no bytes to fuzz and no protocol to synthesize — the §9 proof template (byte-equivalence + protocol responder + decoder fuzzer) simply does not apply; pretending otherwise would manufacture a vacuous fuzzer (the `reference_differential_asserter_dispatch` dead-assertion lesson generalized). The distribution-band proof is the honest shape: it is exactly how the project already proves ROUND_ROBIN (fixtures 0001/0003 via `DistributionAsserter`), extended with a load-skew stimulus. Cross-side-exact distribution is IMPOSSIBLE by construction (independent RNGs; upstream's P2C is per-worker random) — band-per-side is the strongest true claim. The fixture is cross-side (it boots the reference with the same LEAST_REQUEST config), so the `StatsAsserter` prong stays cross-side per `reference_differential_asserter_dispatch`.

### 2.6 Deferred-policy posture: byte-stable reject + the reject-text blast radius *(self-answered; pinned at SPEC, D-L5)*

**Decision:** RANDOM/RING_HASH/MAGLEV (and every other un-implemented lb_policy) STAY rejected at config load with byte-stable text. Because the current text ("cluster: %q: only ROUND_ROBIN lb_policy supported; got %s", `manager.go:216`) names ROUND_ROBIN as the only policy, ACCEPTING LEAST_REQUEST forces a TEXT CHANGE — a byte-stable-reject touchpoint. D-L5 pins the blast radius: every unit test (`internal/cluster/manager_test.go` — the phase-02 `Error_NonRoundRobinLB` arm) and any fixture that pins the current text, plus the new accept/reject matrix (LEAST_REQUEST accepted; `choice_count = 0/1` PGV-rejected; `choice_count >= 2` accepted; the deferred knobs' parse-accept-vs-reject disposition matched against the reference's own behavior).

**Rationale:** The byte-stable-reject discipline (the §9 D-T4/D32-x lineage) treats reject text as contract surface: change it ONCE, deliberately, with the blast radius enumerated up front — never discover the breakage at the six-gate.

### 2.7 Stat surface: anticipated ZERO delta *(self-answered; SPEC pins, D-L4)*

**Decision:** Anticipated stat surface UNCHANGED at **1116** — NO new stat names. least_request reuses the existing cluster counters (`upstream_cx_total`, `upstream_cx_active`, `upstream_rq_total`, …); the policy changes WHICH endpoint is picked, not what is counted. IF the D-L4 probe finds the reference exposing lb-specific cluster stats under LEAST_REQUEST (e.g. an `lb_healthy_panic`-adjacent name incremented by the policy), the SPEC decides (mirror it, or record an envoy-go-strict departure). The latency-histogram family stays deferred (ADR-0060).

**Rationale:** A zero-delta stat hypothesis is itself a pin worth making explicit — the first anticipated-zero-delta phase since the stat-surface count began moving every phase. D-L4 is the cheap insurance probe.

---

## 3. Framework-survey result — 1 NEW framework seam (in `internal/cluster`) + 0 new packages + 0 new go.mod deps (anticipated)

### 3.1 NEW framework seam: the LB acquire/release extension *(per Q-seam; ADR-0232)*

The `loadBalancer` interface in `internal/cluster/loadbalancer.go` extends from `Pick() (Endpoint, error)` to the acquire/release shape (§2.3) — **the project's FIRST structural seam OUTSIDE `internal/filter/network`** (every prior seam — 07.1 HTTP chain, 26.1/26.2 network chain + terminal, 28.1a/b write/read, 29.3 halt/resume, 32.1 upstream pool — lives under `internal/filter/`). The LB owns per-endpoint atomic active counters; the dial path (`cluster.go` `Dial` → the `connWithGauge` Close-once wrapper) releases on conn close; pick-without-successful-dial releases immediately (anticipated; SPEC pins). `roundRobin` adopts the shape and ignores the counters (behavior-neutral; ADR-0024 unamended). Every future Load-balancing-family row implements this interface — the ADR-0230 build-at-first-consumer/reuse-later logic transplanted to a new family.

### 3.2 NEW packages: NONE *(the honest departure from the §9 template)*

The §9 rows each added `internal/filter/network/<subject>/`. Phase 34 adds **NO package**: the `leastRequest` type, the reshaped interface, the manager acceptance, and the release wiring all land in the EXISTING `internal/cluster` package (`loadbalancer.go` / `manager.go` / `cluster.go`, possibly a new `leastrequest.go` FILE in the same package). least_request is not a filter — there is nothing to register in `builtins`, no TypeURL factory, no bootstrap blank-import (the `clusterv3` proto is already imported by `internal/cluster`).

### 3.3 go.mod deps: anticipated ZERO new *(verified at brainstorm; re-pinned at SPEC D-L1)*

`Cluster.LeastRequestLbConfig` lives in the EXISTING `github.com/envoyproxy/go-control-plane/envoy v1.32.4` dep (`config/cluster/v3/cluster.pb.go` — verified in the module cache during this brainstorm, §2.1, with the exact three-knob surface + the PGV `choice_count >= 2` constraint recorded). D-L1 confirms a clean `go mod tidy` and re-records the knob roster in the SPEC.

### 3.4 REUSES

- `internal/cluster/` (02/05.2) — the `Manager` walk + PGV validation site (`manager.go`), the `Cluster.PickEndpoint`/`Dial` pick funnel (`cluster.go:~173/~198`), the `connWithGauge` Close-once wrapper (ADR-0063 — the anticipated release attach-point), the ADR-0024 per-cluster state-scope discipline (the new per-endpoint counters follow it; no amendment anticipated).
- Every Pick consumer, mechanically: `internal/filter/tcpproxy/` (`filter.go:127`), `internal/filter/http/router/` (`router.go:662`), `internal/httpclient/` (`httpclient.go:280` — a direct `PickEndpoint` consumer), `internal/grpcclient/`, `internal/cluster/dial_h2.go`, the ADR-0230 upstream-pool dial closures (`redisproxy`/`thriftproxy`) — all behavior-neutral at this phase.
- The differential harness — the EXISTING `DistributionAsserter` (`test/differential/fixture/fixture.go` line ~57; the fixtures-0001/0003 RR-distribution precedent) + `StatsAsserter` + the flat `GET /stats` admin endpoint + the fixture-dispatch/asserter-dispatch/`-count=1` memory constraints — booting the contrib reference image (ADR-0227).
- The plain TCP echo BackendKind of `0001-tcp-proxy-rr` (NO new BackendKind — §2.5).
- `envoy.config.cluster.v3` proto bindings (go-control-plane `/envoy` v1.32.4 — already a dep and already imported by `internal/cluster`; §3.3).

---

## 4. Per-route applicability — none (LB policy is cluster-scoped)

`lb_policy` / `LeastRequestLbConfig` are fields of `Cluster` — the policy is CLUSTER-scoped, selected per-cluster at bootstrap. There is no `typed_per_filter_config` surface, no per-route override, and no listener interaction. (Upstream's per-route LB overrides — e.g. hash policy on routes — belong to the hash-policy rows, not least_request.) Not applicable to phase 34.

---

## 5. Stat surface hypothesis — anticipated ZERO delta

### 5.1 No new stat names (SPEC pins, D-L4)

least_request reuses the existing `cluster.<name>.*` counters; the policy changes pick selection, not the stat roster. Anticipated **1116 → 1116**. D-L4 probes the reference's `/stats` under LEAST_REQUEST for any lb-specific names (e.g. panic/zone-aware families) — if any exist, the SPEC decides mirror-vs-departure.

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The deferred knobs (`active_request_bias` / `slow_start_config` — parse-accepted-or-rejected per D-L5, behavior deferred); any reference lb-stat names found at D-L4 and not mirrored; the RNG non-equivalence itself (per-side P2C streams — a documented behavioral boundary, not a stat departure); histograms deferred (ADR-0060).

---

## 6. Differential fixture envelope — anticipated ONE directory

### 6.1 Fixtures (+1)

- **`0059-lb-least-request`** (cross-side): chain `[tcp_proxy]` on BOTH sides over a multi-endpoint STATIC cluster with `lb_policy: LEAST_REQUEST` (+ `least_request_lb_config.choice_count` exercised); the plain TCP echo backends of `0001-tcp-proxy-rr` REUSED. Arms: (a) the **load-skew band arm** — held-open conns elevate one backend's outstanding-active count; subsequent picks skew away; asserted via the EXISTING `DistributionAsserter` with BAND semantics PER SIDE (RNG-driven — NOT cross-side-exact); (b) the **FULL_SCAN deterministic arm** ONLY IF D-L3 revives it (anticipated DROPPED — the v1.32.4 settled surface has no FULL_SCAN knob, §2.1); (c) the **cross-side `StatsAsserter`** on cluster counters (cross-equal-vs-per-side disposition per D-L4). One deliberate-break liveness proof per arm with `-count=1`. NO boot-reject dir anticipated (the PGV `choice_count` reject lands as a unit-level arm in `manager_test.go`; the SPEC may still add an `00xx-lb-boot-reject` dir if the reference's reject is worth pinning cross-side — D-L5 decides; the cross-side-XOR-boot-reject dispatch constraint would force a separate dir).

### 6.2 Total

60 → **61** (62 only if D-L5 adds a boot-reject dir). SPEC pins exact numbering + arm rosters.

### 6.3 New BackendKind: NONE *(a family-level first)*

BackendKind tail STAYS **33** (`TCPThriftResponder`). The `0059` fixture reuses the plain TCP echo backends (the `0001-tcp-proxy-rr` kind) — an LB phase exercises WHERE connections land, not what the backend speaks. The FIRST differential-bearing phase since BackendKinds began to add none — deliberate.

### 6.4 New fuzzer: NONE *(a project-level first)* + no conformance harness

Fuzzers STAY **42** (tail `FuzzThriftDecode`) — phase 34 decodes no wire bytes (§2.5); the FIRST phase since the fuzzing regime began to add no fuzzer — deliberate, recorded here and at the BEHAVIOR_CONTRACT bundle. No new conformance harness; h2spec + proxy-wasm re-run asserted-unaffected at the six-gate (phase 34 touches the dial path mechanically — the six-gate's full differential re-verify is the real guard: all 60 existing dirs must stay byte-exact through the seam reshape).

---

## 7. Anticipated ADRs — 2 ADRs (ADR-0232 + ADR-0233)

Next-free ADR at master tip is **ADR-0232** (DECISIONS.md tail **ADR-0231** — the thrift_proxy filter, ACCEPTED; the ADR-0209 escape-valve reserve stands unconsumed). DECISIONS tail STAYS ADR-0231 at this BRAINSTORM.

- **ADR-0232** *(34 — the LB acquire/release seam)* — the `loadBalancer` interface extension (Pick + release handle / PickAcquire-Release; per-endpoint LB-owned atomic active counters; release-on-conn-close via the dial path; release-on-dial-failure; `roundRobin` behavior-neutral adoption; ADR-0024 unamended) — the project's first framework seam outside `internal/filter/network`; every future Load-balancing-family row reuses it (the ADR-0230 reuse logic).
- **ADR-0233** *(34 — the least_request policy)* — the `leastRequest` LB: un-weighted P2C over `choice_count` sampled hosts (default 2; PGV `>= 2`), the manager acceptance + the reject-text change + the accept/reject matrix, the FULL_SCAN disposition (anticipated DEFERRED per the v1.32.4 surface), the deferred knobs (`active_request_bias`/`slow_start_config`), and the band-based differential proof shape.

§Context anchored at SPEC; §Decision/§Consequences bodies at IMPL per ADR-0044 (next-free after phase 34 ≈ **ADR-0234**). The SPEC/PLAN may surface additional ADRs (each re-checks).

---

## 8. Deferred items

- **`active_request_bias`** (the weighted P2C variant) — only observable with unequal host weights (the v1.32.4 doc comment, §2.1); the MVP's STATIC clusters are equal-weight. Parse-accept-vs-reject per D-L5. A future family sub-phase (with weighted endpoints).
- **`slow_start_config`** — warm-up weighting; parse disposition per D-L5; a future row (shared with ROUND_ROBIN, whose `RoundRobinLbConfig.slow_start_config` is equally unconsumed).
- **FULL_SCAN / `selection_method`** — NOT expressible on the settled v1.32.4 `Cluster.LeastRequestLbConfig` surface (§2.1); anticipated DEFERRED at D-L3 along with the deterministic fixture arm.
- **The `Cluster.load_balancing_policy` extension point** — the typed-extension LB config path (where upstream's `least_request.v3.LeastRequest` proto with `selection_method` lives). A much larger config seam; a future family row if/when a policy needs it.
- **All other policies** — RANDOM, RING_HASH, MAGLEV (+ subset LB, locality-weighted LB, priority load balancing, panic thresholds as modifiers) — stay byte-stable-rejected; each a future family row implementing the ADR-0232 seam.
- **Locality-weighted interplay + panic thresholds** — host-set tiering; deferred with the policies that need them.
- **lb-specific stats / latency histograms** — zero stat delta anticipated (D-L4); histograms deferred project-wide (ADR-0060).
- **Healthy-host filtering beyond the existing no-endpoints error** — the project has no active health checking (the Upstream-robustness family's territory); P2C samples over the full endpoint set (D-L6 records the per-side semantics).

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- **02 SPEC (tcp-proxy) deferred-items — "Load-balancer policies other than round-robin"** (`docs/envoy-go/phases/02-tcp-proxy/SPEC.md` line ~38: "`LEAST_REQUEST`, `RANDOM`, `RING_HASH`, `MAGLEV`, subset LB, locality-weighted LB, priority LB, panic thresholds → load balancing family") — **PICKED UP here**: least_request is the first item retired from that phase-02 deferral; the rest carry forward as the family roster (§1). The phase-02 `loadbalancer.go` doc comment ("Future phases that introduce LEAST_REQUEST … add new types here") is consummated by this phase.
- **ADR-0024 (per-cluster RR counter scope)** — TOUCHED, not amended: the seam keeps LB state per-cluster (the new per-endpoint counters are fields of the per-cluster LB instance); the RR counter and its unit-pinned first-pick property are untouched (§2.3; the SPEC confirms no amendment — D-L7 territory).
- **ADR-0230 §Consequences (the upstream-pool seam)** — its dial closures are MECHANICAL consumers of the reshaped pick surface (behavior-neutral); the pool seam itself is unchanged. No LB-related deferral existed in ADR-0230.
- **The §9 phase BRAINSTORMs (26–33) §8 lists** — grepped; NO least_request/lb_policy deferral exists in any §9 row's deferred-items list (the §9 family deferred protocol/filter surface only). The phase-02 SPEC is the sole prior deferral source.
- **The phase-33 BRAINSTORM §9 "remaining protocol proxies — NONE"** — confirmed: phase 34 does not reopen §9; it opens a NEW family per the post-33 routing.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

The SPEC author executes these IN-SESSION (parallel-subagent fan-out per the 25–33 SPEC precedent) against the contrib reference image (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) + go-control-plane `/envoy` v1.32.4 bindings, using the live-probe precedent (`reference_docker_probe_bridge_network` — bridge network + STRICT_DNS backend hostnames + verify traffic actually ran via `downstream_cx_rx_bytes_total > 0`):

- **D-L1** *(SPEC-BLOCKING for the config surface)* — re-pin the exact v1.32.4 proto surface of `Cluster.LeastRequestLbConfig` + PGV constraints in the SPEC: the brainstorm verified `choice_count` (`UInt32Value`, PGV `>= 2` when set, default 2) + `active_request_bias` (`RuntimeDouble`) + `slow_start_config`, and the ABSENCE of `enable_full_scan`/`selection_method` (§2.1 — those live only in the `load_balancing_policies.least_request.v3` extension proto). Confirm a clean `go mod tidy` (ZERO new dep) and the parse path through the existing `clusterv3` import.
- **D-L2** *(SPEC-BLOCKING for the counter semantics + the fixture stimulus)* — what "active request" means for TCP-PROXIED connections in the reference (cx-as-rq? — upstream treats a TCP connection as an active request for LB purposes); probe with held-open conns against a LEAST_REQUEST cluster and observe pick skew + `/stats`. This pins WHAT the per-endpoint counter counts (conns vs requests) and the `0059` skew-arm design.
- **D-L3** — FULL_SCAN availability + tie-break semantics: the settled v1.32.4 surface cannot EXPRESS FULL_SCAN (§2.1 — the strong prior is DEFER + drop the deterministic arm); the probe confirms the v1.37.2 reference's default-path behavior and whether ANY deterministic differential arm is salvageable (e.g. choice_count == endpoint-count degeneracy — pin whether that is even defined). Finalize the arm roster for `0059`.
- **D-L4** — the stat-surface delta (anticipated ZERO new names — §2.7/§5): scrape the reference `/stats` under LEAST_REQUEST with live traffic; pin any lb-specific cluster stat names (mirror-vs-departure if found); pin the `0059` `StatsAsserter` counter set + cross-equal-vs-per-side disposition (RNG-driven pick spread makes per-endpoint counters per-side; cluster-total counters anticipated cross-equal).
- **D-L5** — the accept/reject matrix + the manager reject-text blast radius: which existing unit tests/fixtures pin the current `"only ROUND_ROBIN lb_policy supported"` text (`internal/cluster/manager_test.go` — the phase-02 `Error_NonRoundRobinLB` arm — is a known pinner; grep at SPEC for the full set); the NEW byte-stable text for still-rejected policies; the PGV arms (`choice_count` 0/1 rejected, ≥2 accepted, unset → default 2); the deferred-knob (`active_request_bias`/`slow_start_config`) parse-accept-vs-reject disposition matched against the reference's own parse behavior; whether a cross-side boot-reject dir is warranted (§6.1).
- **D-L6** — P2C sampling semantics, per-side/unit-level ONLY (RNG never cross-side-comparable): with-vs-without replacement when sampling `choice_count` hosts; tie-break (first-sampled? random?); behavior when `choice_count >= len(endpoints)`; healthy-host filtering (none at the MVP — no health checking exists; record the boundary); the RNG source + seeding posture for unit-testability (injectable rand — anticipated, for deterministic unit tests of the sampling logic).
- **D-L7** — the ADR-0045 envelope re-check at SPEC (LoC/tasks vs the ~300–600 / ~10–14 anticipation; the 34.1/34.2 escape valve consumed only if tripped) + the exact exported seam surface (release-handle-vs-PickAcquire/Release; the direct-`PickEndpoint`-consumer sites httpclient/thriftproxy; release-on-dial-failure semantics; confirm ADR-0024 needs no amendment).

---

## 11. Prior-phase lessons applied

- **Verify the proto surface in the module cache at BRAINSTORM time** (the phase-33 §2.1 core-/envoy-vs-contrib correction precedent). Applied: the §2.1 three-knob verification — which caught the FULL_SCAN-knob ABSENCE before the SPEC could assume it (the Q1 conditional resolves toward DEFER with evidence, not surprise).
- **Docker probes need a bridge network** (`reference_docker_probe_bridge_network`). Applied: the D-L2/D-L3/D-L4 live probes use a shared bridge network + STRICT_DNS backend hostnames + verify traffic ran via `downstream_cx_rx_bytes_total > 0` before trusting any `/stats` readout.
- **Differential break protocol needs `-count=1`** (`reference_differential_break_protocol_count1`). Applied: every `0059` arm records a deliberate-break liveness proof with `-count=1` — doubly load-bearing here because a BAND assertion can silently go vacuous (a band wide enough to never fail is a dead assertion; the break proves it bites).
- **Asserter dispatch + liveness** (`reference_differential_asserter_dispatch`). Applied: `0059` is cross-side → the stats prong uses `StatsAsserter`; the distribution prong uses the runner's `DistributionAsserter` hook (driver-side, runs on both paths — the fixtures-0001/0003 precedent); every new assertion proven live.
- **The byte-stable-reject discipline** (the §9 D-T4/D32 lineage). Applied: the manager reject-text change is treated as a contract-surface change with its blast radius enumerated at SPEC (D-L5) — the ONE deliberate text change, with every pinning test updated in the same task.
- **The DistributionAsserter precedent** (fixtures `0001-tcp-proxy-rr`/`0003-http11-routing` — RR distribution proven via per-backend accept counts). Applied: `0059` extends the same hook with a skew stimulus + band semantics rather than inventing a new assertion seam.
- **Build-at-first-consumer, reuse-later seam scoping** (ADR-0230, validated by thrift-33's clean reuse). Applied: the ADR-0232 seam is scoped to what least_request needs (per-endpoint active counters + release-on-close) — NOT a speculative weighted/locality/priority framework; future rows extend it when they consume it.
- **Per-task gofmt + golangci-lint** (`feedback_pertask_gofmt_lint`); **subagents commit local-only** (`feedback_subagents_no_push`); **controller squash-merges + pushes at stage-close** (`feedback_push_to_origin`); **work in worktrees** (`feedback_git_worktrees`); **subagent-driven IMPL execution** (`feedback_execution_style`); **pin canonical paths for worktree subagents** (`feedback_subagent_worktree_path_targeting`). Applied at the SPEC/PLAN/IMPL.
- **Wire-format pins** (`reference_wire_format_both_sides_see_same_bytes`) — applied in its GENERALIZED form only: there are no wire bytes here, but the same discipline governs config-parse fidelity (the accept/reject matrix matches the reference's own behavior, D-L5) and the refusal to invent "our band" without probing the reference's actual skew behavior (D-L2).

---

## 12. Section closeout

This brainstorm settles: (Q0) phase 34 = `least_request` (`Cluster.LbPolicy LEAST_REQUEST` + `Cluster.LeastRequestLbConfig`, `envoy.config.cluster.v3` — VERIFIED in the v1.32.4 module cache with EXACTLY three knobs: `choice_count` [PGV ≥ 2, default 2] + `active_request_bias` + `slow_start_config`; NO `enable_full_scan`/`selection_method` on the settled surface — those live only in the `load_balancing_policies.least_request.v3` extension proto), the FIRST Load-balancing-family row — **the Load-balancing family OPENS**; 7 candidates remain after 34 {random, ring_hash, maglev, subset LB, locality-weighted LB, priority load balancing, panic thresholds}; (Q1) a LEAN envelope — manager acceptance of LEAST_REQUEST (the reject-text byte-stable touchpoint, blast radius at D-L5) + the un-weighted P2C `leastRequest` LB (`choice_count` parsed + PGV-validated) + FULL_SCAN only-if-supported (the proto leg already FAILED → anticipated DEFERRED at D-L3); `active_request_bias`/`slow_start`/locality/panic/all-other-policies DEFERRED; (Q-seam) the `loadBalancer` interface extends to ACQUIRE/RELEASE (per-endpoint LB-owned atomic active counters; release on conn close via the dial path's Close-once wrapper; `roundRobin` behavior-neutral; every Pick consumer touched mechanically) — **the project's first framework seam OUTSIDE `internal/filter/network`**, the family's durable asset, reused by every future family row (the ADR-0230 logic); (Q-split) a SINGLE FLAT ROW 34 (~300–600 prod LoC / ~10–14 tasks, well under the ADR-0045 gate) with a PRE-AUTHORIZED 34.1-seam / 34.2-policy escape valve (expected unconsumed). Self-answered: the differential proof is DISTRIBUTIONAL — fixture `0059-lb-least-request` with a held-open-backend skew arm asserted via the EXISTING `DistributionAsserter` with BAND semantics PER SIDE (RNG-driven, never cross-side-exact) + a cross-side `StatsAsserter` prong (+ the FULL_SCAN deterministic arm only if D-L3 revives it); **NO new fuzzer** (fuzzers STAY 42 — the FIRST no-fuzzer phase since the regime began; deliberate) and **NO new BackendKind** (tail STAYS 33; the `0001` echo backends reused) — both family-level proof-shape novelties recorded; stat surface anticipated UNCHANGED at 1116 (D-L4 pins); ZERO new packages (the work lands in `internal/cluster`) + ZERO new go.mod deps (verified). Anticipated 2 ADRs (ADR-0232 the seam + ADR-0233 the policy; §Context at SPEC, bodies at IMPL per ADR-0044; DECISIONS tail STAYS ADR-0231 at this brainstorm, next-free ADR-0232), fixtures 60 → 61 anticipated at IMPL (62 only if D-L5 adds a boot-reject dir), stat surface 1116 → 1116 anticipated, fuzzers 42 → 42, BackendKind tail 33 → 33. ALL counts UNCHANGED at this brainstorm commit.

The next session authors `docs/envoy-go/phases/34-load-balancer-least-request/SPEC.md` (`superpowers:writing-plans` scoped to SPEC authoring — single flat row, the kafka-31/thrift-33 precedent), executing the §10 D-L1..D-L7 empirical pins IN-SESSION against the contrib reference Envoy per ADR-0004/ADR-0227 (`reference_docker_probe_bridge_network`), and anchoring the ADR-0232 + ADR-0233 §Context drafts. Per ADR-0106, row 34 registers `in-progress` (flat family row, no sub-phases) at this BRAINSTORM-DONE commit; it flips `in-progress → done` at the phase-34 IMPL six-gate (NO parent rollup).
