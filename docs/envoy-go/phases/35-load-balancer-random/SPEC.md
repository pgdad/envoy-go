# Phase 35 SPEC — `random` load balancer (`Cluster.LbPolicy RANDOM`): a stateless uniform-pick policy that REUSES the ADR-0232 LB acquire/release seam at ZERO churn — the SECOND Load-balancing-family row; the family STAYS OPEN

> **For agentic workers:** the NEXT lifecycle step is `superpowers:writing-plans` (PLAN authoring; SKILL_ROUTING state 2 → 3). This SPEC is the input to that PLAN. Steps are NOT checkboxes here — the PLAN decomposes §10 into bite-sized TDD tasks. This is a SINGLE flat Load-balancing-family row (directly executable; SPEC → PLAN → IMPL), NOT a parent pre-split, and NO escape valve (there is no seam build to split off — §3.0). Phase 35 keeps the Load-balancing family OPEN (it opened at phase 34); 6 candidates remain after 35.

**Goal:** Land `Cluster.LbPolicy RANDOM` (`envoy.config.cluster.v3`) — the project's **third LB policy** (after the phase-02 `roundRobin` and the phase-34 `leastRequest`) — as a **stateless** uniform-pick LB (`rng() % len(endpoints)`, one draw, INSENSITIVE to load) that REUSES the ADR-0232 LB acquire/release seam UNCHANGED — implementing the existing unexported `loadBalancer` interface (`Pick() (Endpoint, func(), error)`) and returning the shared `noopRelease` (the seam's FIRST reuse, validating its "the non-keyed RANDOM drops in at zero cost" §Consequences claim, AMEND-R6). In ONE flat phase: ZERO new packages (a new `internal/cluster/random.go` sibling), ZERO new go.mod deps (the enum is in the pinned core `/envoy v1.32.4` module — AMEND-R1), ZERO new stat names (AMEND-R4), NO new fuzzer, NO new BackendKind, ZERO seam churn, ZERO config-parse arm, ZERO PGV gate (RANDOM has NO config message — AMEND-R1). The differential proof is the ANTI-SKEW arm: the band-based per-side `DistributionAsserter` on the REUSED `0059` held-conn stimulus asserting RANDOM STAYS uniform despite the load imbalance (the contrapositive of least_request) + a cross-side `StatsAsserter` prong (`0060-lb-random`).

**Architecture:** A new `randomLB` type (`internal/cluster/random.go`, same package) holds only `endpoints []Endpoint` + an injectable `rng func() uint64` — NO `active []atomic.Int64` slice (RANDOM holds no per-pick state, unlike `leastRequest`). `Pick()` draws ONE `rng() % n`, returns `endpoints[i]` + the shared `noopRelease` + nil (empty set → `Endpoint{}, noopRelease, errNoEndpoints` — the `roundRobin`/`leastRequest` parity, AMEND-R5). The RNG is the EXISTING package-private `newPCGRNG()` helper (`leastrequest.go:63` — a mutex-guarded crypto-seeded `math/rand/v2` PCG), called DIRECTLY with NO extraction (it is already shared-ready, AMEND-R5); the injectable `newRandomWithRNG(endpoints, rng)` test seam mirrors `newLeastRequestWithRNG` minus `choiceCount`. `Manager.buildCluster` gains ONE `case clusterv3.Cluster_RANDOM` constructing `randomLB` (NO `parseXxxLbConfig` — RANDOM has no config); the `default` reject TEXT extends its supported-list `(supported: ROUND_ROBIN, LEAST_REQUEST)` → `(..., RANDOM)` — the ONE deliberate byte-stable-reject change (blast radius THREE sites, AMEND-R2). The ADR-0232 seam, `cluster.go`, `Dial`/`AcquireH1`, and every pick-funnel consumer are UNTOUCHED (random consumes the seam exactly as `roundRobin` does).

**Tech stack:** Go 1.26.2 / golangci-lint 1.64.8 (ADR-0009); reference Envoy **`envoyproxy/envoy:contrib-v1.37.2`** (ADR-0227, @ `sha256:7edd5b0f…`). go-control-plane **`/envoy` v1.32.4** (ADR-0008 — `Cluster.LbPolicy RANDOM` enum value 3 is already in the pinned module; **ZERO new go.mod dep**, `go mod tidy -diff` empty — AMEND-R1). Reuses `internal/cluster/` (the 02/05.2/34 Manager + the ADR-0232 seam + the `leastRequest` `newPCGRNG` RNG helper), the differential harness (`DistributionAsserter` + `StatsAsserter` + the `TCPEcho` BackendKind 0 streaming echo + the `0001`/`0059` echo backends), upstream Envoy v1.37.2 source (`source/extensions/load_balancing_policies/random/random_lb.{h,cc}` + `.../common/load_balancer_impl.cc`) for the algorithm pins. ZERO new packages; ZERO `internal/filter/` touches.

**Authored:** 2026-06-11. **Empirical-pin probe date:** 2026-06-11.

---

## 1. Purpose / Mission

Phase 35 lands `random`, the **SECOND Load-balancing-family row** — landed on the ADR-0232 LB acquire/release seam that phase 34 (`least_request`) built. Unlike phase 34, phase 35 builds NO seam: it REUSES the ADR-0232 seam UNCHANGED, the family analogue of thrift-33's clean reuse of the §9 ADR-0230 seam (the build-at-first-consumer / reuse-later logic validated). `randomLB` is the seam's **first reuse** — a stateless policy that ignores the per-endpoint active counters at zero cost (returns `noopRelease`), consummating the ADR-0232 §Consequences anticipation ("the non-keyed RANDOM drops in at zero cost"). Phase 35 is also the project's third LB policy (after `roundRobin` [02] and `leastRequest` [34]); the `loadbalancer.go` doc comment ("Future phases that introduce RANDOM, RING_HASH, MAGLEV, etc. add new types here") names this exact phase.

This SPEC refines the phase-35 BRAINSTORM (`docs/envoy-go/phases/35-load-balancer-random/BRAINSTORM.md`, Q0/Q1/Q-seam/Q-split) against the AS-BUILT `internal/cluster` package + the §11 D-R1..D-R6 empirical pins EXECUTED IN-SESSION (parallel-subagent fan-out) against (1) the live contrib reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (a RANDOM-cluster held-conn distribution probe on a docker bridge network + a `--mode validate` matrix + a full `/stats` name-set diff), (2) go-control-plane `/envoy` v1.32.4 bindings, and (3) upstream Envoy v1.37.2 source (`source/extensions/load_balancing_policies/random/` + the shared base). It anchors the ADR-0234 §Context DRAFT (§13).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The §11 D-R1..D-R6 scrape CONFIRMED every BRAINSTORM anticipation (no refutations — the simplest family surface). The load-bearing pins, each carried into the relevant §§ below:

- **AMEND-R1 (D-R1 — the v1.32.4 RANDOM surface re-pinned; NO config message; the `lb_config` oneof has no `random` member; ZERO new dep).** `Cluster_RANDOM` is `Cluster_LbPolicy` enum value **3** (`cluster.pb.go:125`). There is **NO `Cluster.RandomLbConfig` message** (zero grep hits across the v1.32.4 cluster package) — RANDOM carries NO config knobs at all. The `lb_config` oneof on `Cluster` has FIVE members (`ring_hash_lb_config` 23 / `maglev_lb_config` 52 / `original_dst_lb_config` 34 / `least_request_lb_config` 37 / `round_robin_lb_config` 56) and **NO `random` member** — so a stray `least_request_lb_config`/`ring_hash_lb_config` under a RANDOM cluster is a mismatched-oneof member (nil-safe getters, silently retained — the silent-ignore disposition, D-R2 variant). `go mod tidy -diff` → exit 0, EMPTY; `go build ./...` → OK in the SPEC worktree — **ZERO new go.mod dep**. **Consequence:** phase 35 has NO config-parse arm and NO PGV gate (simpler than least_request's `choice_count`-bearing `parseLeastRequestLbConfig`); the manager case is a bare construction. See §5 / §11.1.
- **AMEND-R2 (D-R2 — the accept/reject matrix pinned live + the blast radius is THREE sites; NO boot-reject dir).** The `--mode validate` matrix (§11.5): `RANDOM` accepted bare; `RANDOM` + a stray `least_request_lb_config` accepted SILENTLY (zero warning/error lines — the mismatched-oneof silent-ignore parity); `RANDOM` + a stray `ring_hash_lb_config` accepted silently; `RING_HASH`/`MAGLEV` validate-accept on the reference (envoy-go's continued rejection is a recorded departure). Blast radius of the reject-text change (full-repo grep): **`internal/cluster/manager.go:252`** (the only production string) + **`internal/cluster/manager_test.go:320` `TestManager_Error_UnsupportedLBPolicy`** (DOUBLY hit — its trigger value `Cluster_RANDOM` becomes accepted, so it must RETARGET to a still-rejected policy [`RING_HASH`/`MAGLEV`] AND it pins the substring `"ROUND_ROBIN, LEAST_REQUEST"` at line 327 → `"ROUND_ROBIN, LEAST_REQUEST, RANDOM"`) + **`docs/envoy-go/BEHAVIOR_CONTRACT.md:899`** (the lb-policy reject-text line — RANDOM moves OUT of the rejected set). **NO fixture/expectations file pins the text** (`grep -rln "ROUND_ROBIN, LEAST_REQUEST" test/` → empty) → no cross-side boot-reject dir is warranted; the reject arm lands UNIT-LEVEL in `manager_test.go` (fixtures 61 → 62 only). See §6.
- **AMEND-R3 (D-R3 SPEC-BLOCKING for the fixture stimulus — RANDOM stays UNIFORM under held-conn load; the anti-skew core claim CONFIRMED LIVE).** With K=4 conns held open (echo-confirmed, then held to elevate active-conn counts on the backends they landed on) + S=60 short round-trips = 64 picks per burst, the reference's per-host distribution stayed UNIFORM (three bursts: {25,21,18} / {19,25,20} / {16,24,24}, mean 21.3, range 16–25 ≈ ±1.4σ binomial) and the LEADING host ROTATED randomly burst-to-burst (be1, be2, be2) — RNG-driven, NOT load-driven. This is the exact contrapositive of phase-34 least_request `0059` (which SKEWED away from the loaded backend): RANDOM ignores the active-conn count entirely, so the heavy-active backend is NOT avoided. The held conns DO register as active on their backends (the D-L2 cx-as-rq finding carries over — an open TCP-proxied conn IS one active request) — RANDOM simply ignores that, which is exactly the property under test. Traffic witnessed via `downstream_cx_rx_bytes_total: 192 (= 3×64)`. See §8.1.
- **AMEND-R4 (D-R4 — ZERO stat-name delta CONFIRMED; surface STAYS 1116; the `0060` StatsAsserter disposition pinned).** Full `/stats` name-set diff RANDOM-vs-ROUND_ROBIN: EMPTY in both directions (455 names each); `/stats/prometheus` carries no random-specific metric (the generic `cluster.<name>.lb_*` family exists identically-at-0 under both policies — never mirrored). envoy-go mirrors NOTHING new. StatsAsserter disposition for `0060` (identical to `0059`): `cluster.<name>.upstream_cx_total` CROSS-EQUAL + `membership_total == 3` cross-equal + `upstream_cx_active == 0` quiesced post-drain cross-equal; `upstream_rq_total` is **PER-SIDE** (reference = conn count [tcp_proxy charges rq-per-cx]; subject = 0 [envoy-go's tcpproxy path never calls `IncUpstreamRqTotal`] — the pre-existing boundary `0059` already pins). See §7 / §8.1.
- **AMEND-R5 (D-R5 — the RANDOM semantics pinned from v1.37.2 source; the RNG REUSE confirmed shared-ready; the empty-set parity).** `RandomLoadBalancer::peekOrChoose` (`random_lb.cc`): ONE draw `random_hash = random()`, then `hosts_to_use[random_hash % hosts_to_use.size()]` — a SINGLE uniformly-random host, NO power-of-two loop, NO active-count consultation, NO tie-break (a single draw has nothing to tie). The class doc is literally "Random load balancer that picks a random host out of all hosts." Healthy-host filtering (priority/healthy-tier/panic) happens UPSTREAM of the pick in the shared `ZoneAwareLoadBalancerBase` (`peekOrChoose` only ever sees a pre-filtered vector); with no health checking it degenerates to all-hosts — envoy-go's boundary (no health checking — the Upstream-robustness family's territory; recorded). `newPCGRNG()` (`leastrequest.go:63`) is ALREADY a package-private `func() (func() uint64, error)` — a new `random.go` in the SAME package calls it DIRECTLY with NO extraction (anticipated SHARED — CONFIRMED). The injectable seam `newRandomWithRNG(endpoints, rng)` mirrors `newLeastRequestWithRNG` minus `choiceCount`. Empty-set disposition: `Endpoint{}, noopRelease, errNoEndpoints` — the `roundRobin`/`leastRequest` parity. See §11.3.
- **AMEND-R6 (D-R6 — the seam REUSES unchanged; ~45–60 prod LoC / ~4–7 tasks; single ADR-0234; ADR-0024 unamended).** The ADR-0232 seam is reused UNCHANGED: `randomLB` returns the shared `noopRelease`, requires NO `Pick`-signature change, NO `loadBalancer` interface change, NO `cluster.go` change, NO consumer touch — the same zero-churn posture `roundRobin` already occupies. ADR-0232 §Consequences pre-authorizes it verbatim (quoted §13). The hash policies (RING_HASH/MAGLEV) are the seam-EXTENDING phases (the `Pick(hashInput)` widening) — RANDOM is on the zero-churn side of that line. Production footprint: a `random.go` (~40–55 LoC — strictly smaller than `leastrequest.go`'s 100: it drops the P2C loop, the `active` slice, the `sync.Once` release closure, and reuses `newPCGRNG` verbatim) + ONE manager case (~5 LoC) + the reject-text line + the `manager_test.go` retarget → **~45–60 prod LoC / ~4–7 tasks**, FAR under the ADR-0045 gate (`> ~25 tasks OR > ~1500 LoC`); NO escape valve. ONE new ADR (ADR-0234, the policy ONLY — no seam ADR since reuse; the thrift-33 single-ADR-on-reuse precedent). ADR-0024 (per-cluster RR counter scope) is UNAMENDED — random holds no per-cluster counter state (only the endpoint slice + the RNG); ADR-0024 §Consequences already names RANDOM as a zero-touch drop-in. See §3 / §11.4.

### 1.2 ADR continuity + D-disposition at SPEC commit

DECISIONS.md tail at master tip is **ADR-0233** (the least_request policy, ACCEPTED); next-free **ADR-0234**. Per the phase-35 routing (next-prompt + STATE + BRAINSTORM §7), the DECISIONS tail **STAYS ADR-0233 at this SPEC** (counts UNCHANGED at the SPEC — contrast the phase-34 SPEC, which seeded its TWO §Context entries into DECISIONS.md and advanced the tail; phase 35 has a single reuse-case ADR with no seam-build to pre-document, so its ADR-0234 §Context is anchored as a DRAFT in §13 and the full DECISIONS.md entry — §Context + §Decision + §Consequences — lands at the phase-35 IMPL per ADR-0044). All six D-R pins are RESOLVED this session (§11); the remaining open items are PLAN/IMPL D-questions (§12).

---

## 2. Non-purposes (deferred; per BRAINSTORM §8 + the §11 amendments)

- **The `Cluster.load_balancing_policy` extension point** — the typed-extension LB config path where RANDOM's locality-weighting config (`load_balancing_policies.random.v3.Random` with `LocalityLbConfig`) lives. A much larger config seam (shared with least_request's deferred `selection_method`/`enable_full_scan` path); a future family row if/when a policy needs it.
- **Locality-weighted interplay + panic thresholds** — the reference's pick-time host-set selection (priority tiers / healthy hosts / panic) happens UPSTREAM of the RANDOM pick (AMEND-R5); envoy-go has no health checking (the Upstream-robustness family's territory) → the RANDOM pick samples over the full endpoint set; the boundary is recorded in BEHAVIOR_CONTRACT at IMPL.
- **All other policies** — RING_HASH, MAGLEV (the hash policies — each ADDITIONALLY needs the ADR-0232 `Pick(hashInput)` widening), subset LB, locality-weighted LB, priority load balancing, panic thresholds — stay rejected with the NEW byte-stable text (§6.2); the reference validate-accepts RANDOM (now retired) and RING_HASH/MAGLEV (a recorded departure, AMEND-R2). Each a future family row.
- **The active-load-aware / weighted variants** — random is stateless; the active-load-aware future rows reuse the ADR-0232 per-endpoint counters (random ignores them at zero cost).
- **lb-specific stats / latency histograms** — zero stat delta CONFIRMED (AMEND-R4); histograms deferred project-wide (ADR-0060).
- **Per-route applicability** — `lb_policy` is a `Cluster` field; cluster-scoped at bootstrap; no `typed_per_filter_config` surface, no per-route override (BRAINSTORM §4). The ADR-0125 roster is untouched.
- **Any seam change** — the ADR-0232 seam is reused UNCHANGED (AMEND-R6); the `Pick(hashInput)` widening belongs to the future hash-policy rows.

---

## 3. The `randomLB` policy on the REUSED ADR-0232 seam (ADR-0234)

### 3.0 Split disposition — D-R6 RESOLVED (single flat phase; NO escape valve)

ADR-0045 split-gate fires at `> ~25 tasks OR > ~1500 production LoC`. Phase-35 surface (§11.4 / AMEND-R6):

| Unit | Anticipated production LoC |
|---|---|
| `random.go`: the `randomLB` type (`endpoints` + `rng`; NO `active` slice), `newRandom`/`newRandomWithRNG` calling the REUSED `newPCGRNG`, `Pick()` (one `rng() % n`, returns `noopRelease`) | ~40–55 |
| `manager.go`: the `case clusterv3.Cluster_RANDOM` construction (~5) + the ONE reject-text line edit | ~5 |
| Consumer / seam touches | **0** (the seam is reused unchanged — AMEND-R6) |
| The `0060` fixture driver + the band asserter + unit tests | test-side LoC, NOT counted |

Net production **~45–60 LoC, ~4–7 tasks** — BOTH axes FAR under the gate (smaller than least_request, which was itself ~155–255 LoC / 9 tasks; phase 35 builds no seam → nothing to split off). **Single flat phase 35 — NO pre-split, NO escape valve.** The PLAN re-checks the gate per ADR-0045 (anticipated NO SPLIT with an order-of-magnitude margin).

### 3.1 The `randomLB` policy (ADR-0234; stateless uniform pick)

`internal/cluster/random.go` (NEW file, same package — D-S35-1 confirms the placement):

```go
// randomLB is a stateless uniform-pick load balancer mirroring Envoy v1.37.2's
// RandomLoadBalancer::peekOrChoose (SPEC §3.1 / AMEND-R5): ONE draw rng()%n,
// pick endpoints[i]; it consults NO active counters and holds NO per-pick state
// (the contrast with leastRequest's P2C). It REUSES the ADR-0232 seam UNCHANGED,
// returning the shared noopRelease (the seam's first reuse — ADR-0232 §Consequences
// "the non-keyed RANDOM drops in at zero cost").
type randomLB struct {
	endpoints []Endpoint
	rng       func() uint64 // injectable for deterministic tests (the upstream mock posture)
}

func newRandom(endpoints []Endpoint) (*randomLB, error) {
	rng, err := newPCGRNG() // REUSED from leastrequest.go — already package-private, shared-ready (AMEND-R5)
	if err != nil {
		return nil, err
	}
	return newRandomWithRNG(endpoints, rng), nil
}

func newRandomWithRNG(endpoints []Endpoint, rng func() uint64) *randomLB {
	return &randomLB{endpoints: endpoints, rng: rng}
}

func (r *randomLB) Pick() (Endpoint, func(), error) {
	n := len(r.endpoints)
	if n == 0 {
		return Endpoint{}, noopRelease, errNoEndpoints // the roundRobin/leastRequest parity (AMEND-R5)
	}
	i := int(r.rng() % uint64(n))
	return r.endpoints[i], noopRelease, nil
}
```

- The struct holds NO `active []atomic.Int64` (RANDOM tracks no per-endpoint load) and NO `sync.Once` release (it returns the shared `noopRelease`) — strictly less state than either existing policy.
- `newPCGRNG()` is REUSED verbatim (it is already a package-private mutex-guarded crypto-seeded `math/rand/v2` PCG — `leastrequest.go:63`; D-S35-2 records the one cosmetic nit: its seed-error string is hard-coded `"cluster: least_request: seed rng"` — the IMPL may give random a flavored message via a thin wrapper, but `newPCGRNG` itself needs no change).
- The crypto/rand read error threads out of `newRandom` → boot-fail (the `leastRequest` disposition); the unit tests inject a deterministic sequence via `newRandomWithRNG` (the upstream mock-RNG posture, AMEND-R5).

### 3.2 The seam REUSE (the ADR-0232 seam UNCHANGED — AMEND-R6)

`randomLB` implements the existing unexported `loadBalancer` interface (`Pick() (Endpoint, func(), error)`, `loadbalancer.go:15`) and returns the shared `noopRelease` (`loadbalancer.go:21`) — exactly as `roundRobin` does. NO interface change, NO `cluster.go`/`Dial`/`AcquireH1`/`connWithGauge` change, NO pick-funnel consumer touch. The seam is the Load-balancing family's durable asset; phase 35 is the proof that its RELEASE-half generalization claim holds. The PICK-INPUT half (the `Pick(hashInput)` widening for RING_HASH/MAGLEV) is NOT triggered by random (it is stateless, non-keyed) — exactly why it is the zero-churn opener (Q0 rationale).

### 3.3 Manager acceptance (ADR-0234)

`manager.go buildCluster`: the `lb_policy` switch (line 234) gains one case; construction is bare (NO config parse — RANDOM has no config message, AMEND-R1):

```go
switch c.GetLbPolicy() {
case clusterv3.Cluster_ROUND_ROBIN:
	cl.lb = &roundRobin{endpoints: endpoints}
case clusterv3.Cluster_LEAST_REQUEST:
	cc, err := parseLeastRequestLbConfig(c, name)
	if err != nil {
		return nil, err
	}
	lb, err := newLeastRequest(endpoints, cc)
	if err != nil {
		return nil, err
	}
	cl.lb = lb
case clusterv3.Cluster_RANDOM: // NEW (phase 35)
	lb, err := newRandom(endpoints)
	if err != nil {
		return nil, err
	}
	cl.lb = lb
default:
	return nil, fmt.Errorf("cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM)", name, c.GetLbPolicy())
}
```

The NEW reject text (`…, RANDOM` appended) is the ONE deliberate byte-stable-reject change (§6.2; blast radius AMEND-R2). A stray `least_request_lb_config`/`ring_hash_lb_config` under a RANDOM cluster is silently ignored (the manager never reads the oneof on the RANDOM path — reference PARITY, §6.3); there is NO `parseRandomLbConfig` (nothing to parse).

---

## 4. Framework primitives — 0 new framework seams + 0 new packages + 0 new go.mod deps

Phase 35's framework delta is ZERO. It REUSES the ADR-0232 LB acquire/release seam UNCHANGED (§3.2 — the seam's first reuse, the family analogue of thrift-33's clean reuse of the §9 ADR-0230 seam). ZERO new packages (the `randomLB` type + the manager acceptance land in the existing `internal/cluster`: a new `random.go` + `manager.go`); ZERO `internal/filter/` touches; ZERO new go.mod deps (AMEND-R1). random is not a filter — no builtins registration, no TypeURL factory, no bootstrap blank-import (the `clusterv3` proto is already imported by `internal/cluster`).

---

## 5. Proto-field roster (per §11.1 D-R1)

All from go-control-plane `/envoy` v1.32.4 (`config/cluster/v3/cluster.pb.go`), verified in the module cache this session (re-confirming the BRAINSTORM §2.1 verification).

### 5.1 `Cluster.LbPolicy` enum values at the guard

| Value | Enum | 35 disposition |
|---|---|---|
| 0 | `ROUND_ROBIN` (proto default; unset ≡ ROUND_ROBIN) | accepted (phase 02) |
| 1 | `LEAST_REQUEST` | accepted (phase 34) |
| **3** | **`RANDOM`** | **accepted (THIS PHASE)** |
| 2 | `RING_HASH` | rejected (hash policy — needs `Pick(hashInput)`; the reference accepts → departure) |
| 5 | `MAGLEV` | rejected (hash policy; the reference accepts → departure) |
| 6 | `CLUSTER_PROVIDED` | rejected |
| 7 | `LOAD_BALANCING_POLICY_CONFIG` | rejected |

### 5.2 NO config message; the `lb_config` oneof (matters for the silent-ignore parity — AMEND-R1)

There is **NO `Cluster.RandomLbConfig`** (zero grep hits). The `lb_config` oneof `Cluster.LbConfig` carries ONE of: `ring_hash_lb_config` (23) / `maglev_lb_config` (52) / `original_dst_lb_config` (34) / `least_request_lb_config` (37) / `round_robin_lb_config` (56) — and **NO `random` member**. So a stray non-random oneof member under a RANDOM cluster is mismatched-but-valid (nil-safe getters): the manager never reads it on the RANDOM path → silently ignored (reference PARITY, §6.3). RANDOM has NO config-parse arm and NO PGV gate at all — the simplest policy surface in the family.

---

## 6. PARSE-REJECT roster (per §11.5 + ADR-0080)

### 6.1 Wording discipline

Per ADR-0080: the reject-text CHANGE at `manager.go:252` is the ONE deliberate contract-surface change this phase (the §9 / phase-34 byte-stable-reject lineage: change it ONCE, with the blast radius enumerated — AMEND-R2: `manager.go:252` + `TestManager_Error_UnsupportedLBPolicy` + `BEHAVIOR_CONTRACT.md:899`; NO fixture pins the text). Verified by a table test at IMPL.

### 6.2 Reject arms (all UNIT-TESTED; no cross-side boot-reject dir — AMEND-R2)

- `cluster-lb-policy-unsupported` — the REPLACEMENT text for the still-rejected policies: `cluster: %q: unsupported lb_policy %s (supported: ROUND_ROBIN, LEAST_REQUEST, RANDOM)`. `TestManager_Error_UnsupportedLBPolicy` (`manager_test.go:320`) RETARGETS its trigger from `Cluster_RANDOM` (now accepted) to `Cluster_RING_HASH` (or `MAGLEV`) and re-pins the new substring `"ROUND_ROBIN, LEAST_REQUEST, RANDOM"` (line 327). An envoy-go-strict DEPARTURE relative to the reference for RING_HASH/MAGLEV (the reference accepts them — recorded in BEHAVIOR_CONTRACT). This is the phase-34 "doubly-hit" pattern recurring (the accept of policy N forces the reject test that used N as its trigger to retarget to N+1).
- There are NO other reject arms for phase 35 — RANDOM has NO config knobs (no `choice_count`-style gate, no `active_request_bias`/`slow_start_config` departure rejects). This is strictly simpler than the phase-34 §6 matrix.

### 6.3 NON-reject dispositions (parity)

- A stray `lb_config` oneof member under `lb_policy: RANDOM` (e.g. `least_request_lb_config` or `ring_hash_lb_config`): **silent-ignore** — reference PARITY (the `--mode validate` matrix accepted both silently, zero warnings — §11.5 variants 2/3) AND behavior-INERT (the manager never reads the oneof on the RANDOM path — there is no `parseRandomLbConfig`; RANDOM's pick is config-free). This keeps the EXISTING under-ROUND_ROBIN / under-LEAST_REQUEST postures byte-stable.
- `lb_policy: RANDOM` with NO lb config: accepted, the bare construction (reference parity — §11.5 variant 1).

---

## 7. Stat surface — ZERO delta (per §11.7 D-R4 + AMEND-R4)

- **NO new stat names.** Surface STAYS **1116**. Live-proven: the full `/stats` name-set diff between RANDOM and ROUND_ROBIN configs is EMPTY both directions (455 names each); `/stats/prometheus` carries no random-specific metric; the reference's `cluster.<name>.lb_*` family exists identically-at-0 under BOTH policies and is NOT mirrored (envoy-go has never mirrored it — unchanged posture). random has NO per-endpoint state at all (unlike least_request's LB-internal `atomic.Int64` counters, which were themselves NOT registry stats).
- **The second anticipated-zero-delta phase** — now a family expectation (the phase-34 first repeats), confirmed empirically.
- The `0060` StatsAsserter set (AMEND-R4): CROSS-EQUAL `cluster.<name>.upstream_cx_total` (= total driver conns; tcp_proxy is 1:1 both sides) + `membership_total` (= 3) + `upstream_cx_active` (= 0 post-drain, quiesced); PER-SIDE `upstream_rq_total` (reference = conn count [rq-per-cx]; subject = 0 [the tcpproxy path never calls `IncUpstreamRqTotal` — the boundary `0059` already pins]).

---

## 8. Differential fixture taxonomy (+1)

Per `reference_differential_fixture_dispatch_constraint`: ONE cross-side dir (no boot-reject dir — AMEND-R2). Per `reference_differential_asserter_dispatch`: the stats prong uses `StatsAsserter` (cross-side path); the distribution prong uses the runner's `DistributionAsserter` hook (driver-side, runs on both paths — the 0001/0003/0059 precedent). Per `reference_differential_run_selector`: targeted runs use `-run 'TestDifferential/0060'` (NOT `-run '0060'`, which matches zero subtests). Every assertion proven live via a deliberate-break with `-count=1` (`reference_differential_break_protocol_count1` — DOUBLY load-bearing here: a band wide enough to never fail is a dead assertion). Numbering continues from `0059`; re-pinned at IMPL Task 1.

### 8.1 `0060-lb-random` (cross-side; the ANTI-SKEW band arm + the stats prong)

Chain `[tcp_proxy]` on BOTH sides (the 0001/0059 shape: reference STRICT_DNS/`host.docker.internal`, subject STATIC/127.0.0.1) over ONE 3-endpoint cluster with `lb_policy: RANDOM` (NO lb config — RANDOM has none). Backends: the plain **`TCPEcho` BackendKind 0** (STREAMING echo — `acceptEchoCounting`, so a held conn CAN confirm end-to-end establishment with a write+read before holding; the `0059` backends REUSED). **NO new BackendKind** (tail STAYS 33).

**The workload (identical per side, sequential — the `0059` stimulus REUSED, the assertion INVERTED):**
1. **Hold phase:** open K=4 connections; on each, write one byte and read the echo (the establishment witness — the upstream dial completed and the active count is held on whatever backend the pick landed on), then KEEP the socket open. (Under RANDOM the held picks self-spread randomly, NOT least-loaded-filling.)
2. **Burst phase:** S=60 sequential short round-trips (`helpers.TCPRoundTrip` — write, half-close, read echo, close).
3. **Drain:** close the 4 held conns.

**The anti-skew band arm (per-side, via `AssertDistribution` on the per-backend accept counts; the 64 total accepts):**
- `c1 + c2 + c3 == 64` (conservation — hard equality; catches any drop/double-count);
- each `cᵢ >= 12` (UNIFORM FLOOR: mean 21.3, ~2.5σ below ≈ 11.9 → floor 12. **Violated by a load-skewing [least_request-like] policy that starves a loaded backend** — the deliberate-break that proves the anti-skew property bites — and by single-host pinning [one host ≈ 0]. Live minima 18/19/16 clear it comfortably);
- each `cᵢ <= 32` (UNIFORM CEILING: mean 21.3, ~2.8σ above ≈ 31.9 → ceiling 32. **Violated by single-host pinning** [one host ≈ 64]. Live maxima 25/25/24 clear it).

Asserted **PER SIDE** (both sides must land in the uniform band; NEVER cross-side-exact — independent RNG streams, the BRAINSTORM band-semantics decision; the `0059` per-side precedent). This is the SECOND band-based `AssertDistribution` use (after `0059` — the interface supports the band unchanged; `0059` legitimized per-side asymmetry). The band brackets RANDOM's binomial variance (σ ≈ √(64·⅓·⅔) ≈ 3.77) at ~2.5–2.8σ — low false-reject risk over RNG draws while still violated by a skew or single-host break. The exact constants (K/S/floor/ceiling) are IMPL-tunable within the pinned principle {conservation + uniform-floor + uniform-ceiling}; margins justified by the live three-burst probe (16–25 around mean 21.3 — §11.2) — D-S35-3 records the tuning protocol. **Note the contrast with `0059`:** `0059` (least_request) asserts STARVATION (`c1 <= 12`) + CONCENTRATION (`c2 >= 16`) — the loaded backend is AVOIDED; `0060` (random) asserts the OPPOSITE — every backend stays in the fair-share band {floor 12, ceiling 32} DESPITE the same held-conn load. The anti-skew property is the positive separation from least_request.

- **Deliberate-break liveness (`-count=1`):** (i) make `randomLB.Pick` consult the active counters and avoid the loaded endpoint (i.e. accidentally implement least_request) → the loaded backend's count drops below the floor 12 → the floor leg FAILS deterministically (the canonical anti-skew break — proves the uniform band bites against load-sensitivity); (ii) pin all picks to `endpoints[0]` (e.g. `i = 0` instead of `rng() % n`) → the two un-picked backends hit 0 < floor and the picked one hits 64 > ceiling → both legs FAIL; (iii) drop a `StatsAsserter` Inc → the stats prong FAILS. Recorded in driver comments + README per the `0030` lesson.

**The stats prong (cross-side `StatsAsserter`, post-drain):** §7's set — cross-equal `upstream_cx_total == 64` + `membership_total == 3` + `upstream_cx_active == 0`; per-side `upstream_rq_total` (ref 64 / subj 0 — AMEND-R4).

### 8.2 NO boot-reject dir (AMEND-R2)

The lb-policy reject arm (the still-rejected RING_HASH/MAGLEV departure) lands UNIT-LEVEL in `manager_test.go` (§6.2): RANDOM with no lb config is valid on both sides (no missing-required-field arm), no fixture pins the old reject text, and the RING_HASH/MAGLEV departure CANNOT be cross-side (the reference accepts them). Fixture count 61 → **62** (`0060-lb-random` only).

### 8.3 NO new BackendKind + NO new fuzzer (now family expectations)

BackendKind tail STAYS **33** (`0060` reuses `TCPEcho` 0 — an LB phase exercises WHERE connections land, not what the backend speaks; the phase-34 first recurs). Fuzzers STAY **42** — phase 35 decodes no wire bytes (the enum parse is proto-level, already fuzz-covered at the bootstrap surface; there is NO config message); the phase-34 no-fuzzer first recurs — DELIBERATE (a manufactured fuzzer over no decoder is vacuous). No new conformance harness; h2spec 53/53 + proxy-wasm 10/10 re-run asserted-unaffected at the six-gate. The six-gate's REAL guard is the full 61-dir differential re-verify: all 61 existing dirs must stay byte-exact through the new manager case (the seam is unchanged → behavior-neutrality is structural).

---

## 9. Behavior-contract delta (the 35 bundle; ADR-0052 atomic landing)

At IMPL final task, `docs/envoy-go/BEHAVIOR_CONTRACT.md` gains:

- A NEW `random` subsection beside the least_request boundary text: the RANDOM acceptance (no config message; bare construction); the uniform-pick semantics (one draw `rng() % n`, no active-count consult — the upstream v1.37.2 mirror; the contrast with least_request's P2C); the per-side RNG non-equivalence (anti-skew band-proven, never cross-side-exact); the healthy-set boundary (no health checking → all-hosts sampling).
- The line-899 reject-text entry updates (RANDOM retired from the rejected set → the third accepted policy; the supported-set string `… ROUND_ROBIN, LEAST_REQUEST, RANDOM`; RING_HASH/MAGLEV stay the recorded departure).
- Departure/coverage records: the mismatched-oneof silent-ignore under RANDOM (parity); NO new fuzzer/BackendKind (now family expectations); stat surface UNCHANGED at 1116 (the second zero-delta phase).

---

## 10. Per-task structure (~4–7 tasks; PLAN decomposes)

Indicative spine for the PLAN (TDD per task; per-task `gofmt -l` + `golangci-lint` on touched pkgs per `feedback_pertask_gofmt_lint`; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`):

| # | Task | SPEC anchor |
|---|---|---|
| 1 | First-task baselines/anchors gate: re-confirm fixtures **61** (tail `0059`) + fuzzers **42** + stat surface **1116** + BackendKind tail **33** + DECISIONS tail **ADR-0233** via the canonical recipes; re-pin the as-built anchors (`loadbalancer.go` interface + `noopRelease`, `leastrequest.go:63` `newPCGRNG`, `manager.go:234` switch + `:252` reject text, `manager_test.go:320` `TestManager_Error_UnsupportedLBPolicy`, the `0059` driver + `acceptEchoCounting`) against the IMPL-session tip; PROGRESS.md created | §11 / §3 |
| 2 | The `randomLB` type: the stateless uniform pick with the injectable RNG (TDD with a deterministic RNG: `rng() % n` selects the indexed endpoint; the empty-set `errNoEndpoints` + `noopRelease` parity; the seam-reuse `noopRelease` return; `newRandom`/`newRandomWithRNG` calling the REUSED `newPCGRNG`) | §3.1 / AMEND-R5 |
| 3 | Manager acceptance: the `case clusterv3.Cluster_RANDOM` bare construction + the NEW reject text + the mismatched-oneof silent-ignore + `TestManager_Error_UnsupportedLBPolicy` retarget (RANDOM → RING_HASH/MAGLEV + the new substring) (TDD: the §6 matrix, byte-stable table test) | §3.3 / §6 |
| 4 | A boot smoke + an in-process anti-skew integration test (deterministic RNG; held picks elevate counters; subsequent RANDOM picks STAY uniform — do NOT avoid the loaded endpoint, the contrast with least_request) | §3.1 / §8.1 |
| 5 | The `0060-lb-random` fixture: driver (hold-4 + burst-60 + drain workload), the anti-skew band `AssertDistribution` (conservation + uniform-floor 12 + uniform-ceiling 32), the `StatsAsserter` prong (cross-equal cx/membership/quiesced + per-side rq) | §8.1 |
| 6 | Band-constant tuning + deliberate-break liveness (`-count=1`): the consult-the-counters break (floor leg — the canonical anti-skew break), the pin-to-endpoints[0] break (floor + ceiling), the stats-prong break; ≥20-run flake check per side (D-S35-3) | §8.1 |
| 7 | Full differential re-verify (the 61 prior dirs byte-exact through the new manager case + `0060` green) + `-race -short` + h2spec/proxy-wasm asserted-unaffected; Completion bundle: BEHAVIOR_CONTRACT 35 bundle (§9) + the ADR-0234 §Context+§Decision+§Consequences (ADR-0044 in-place; tail → ADR-0234) + STATE/ROADMAP row 35 `in-progress → done` (flat family row — NO parent rollup) + the six-gate evidence | §9 / §13 |

The PLAN re-checks the ADR-0045 gate (anticipated NO SPLIT with margin); it may merge/split these indicative tasks (e.g. fold Task 4's boot smoke into Task 3, or split the fixture from its tuning).

---

## 11. SPEC-time empirical-pin block (D-R1..D-R6 — executed IN-SESSION 2026-06-11)

Parallel-subagent fan-out executed this SPEC session per ADR-0004's hard-gate. **Probe date: 2026-06-11.** **Reference source corpus:**

1. **The live `envoyproxy/envoy:contrib-v1.37.2` docker image**: a `tcp_proxy → STRICT_DNS RANDOM` listener on a docker BRIDGE network (`reference_docker_probe_bridge_network`) with THREE streaming-echo backends; a hold-4 + burst-60 distribution probe (×3 bursts) + `/clusters` per-host scrapes + a full `/stats`//`/stats/prometheus` name-set diff RANDOM-vs-ROUND_ROBIN (`downstream_cx_rx_bytes_total > 0` verified); a 5-variant `--mode validate` matrix.
2. **go-control-plane `/envoy` v1.32.4 bindings** at `~/go/pkg/mod/.../envoy@v1.32.4/config/cluster/v3/cluster.pb.go`; `go mod tidy -diff` + `go build ./...` in the SPEC worktree.
3. **Upstream Envoy v1.37.2 source** via raw.githubusercontent.com at tag v1.37.2: `source/extensions/load_balancing_policies/random/random_lb.{h,cc}` + `source/extensions/load_balancing_policies/common/load_balancer_impl.cc`.
4. **envoy-go codebase** at master tip `c5c6c55` (the phase-35 BRAINSTORM squash): `internal/cluster/{loadbalancer,leastrequest,manager,cluster}.go`, `internal/cluster/manager_test.go`, full-repo reject-text greps.

### Summary disposition table (6 pins)

| Pin | Topic | Disposition | AMEND |
|---|---|---|---|
| §11.1 | D-R1 (SPEC-BLOCKING) — proto/surface re-pin + tidy | **CONFIRMED** (enum 3; NO RandomLbConfig; oneof has no random member; ZERO new dep) | R1 |
| §11.2 | D-R3 (SPEC-BLOCKING) — RANDOM uniform under held-conn load | **CONFIRMED LIVE** (3 bursts 16–25 around mean 21.3; leading host rotates randomly; the contrapositive of 0059) | R3 |
| §11.3 | D-R5 — RANDOM sampling/RNG semantics | RESOLVES (one uniform draw; no active-count consult; healthy-set boundary; `newPCGRNG` shared-ready; empty-set parity) | R5 |
| §11.4 | D-R6 — the seam reuse + the ADR-0045 envelope re-check | **REUSE UNCHANGED** (noopRelease; no Pick-signature change; no consumer touch; ADR-0024 unamended; ~45–60 LoC → single flat row, no valve) | R6 |
| §11.5 | D-R2 — accept/reject matrix + blast radius | RESOLVES (the 5-variant validate table; 3-site blast radius; NO fixture pins; mismatched-oneof silent-ignore parity; no boot-reject dir) | R2 |
| §11.7 | D-R4 — stat-surface delta | **ZERO delta CONFIRMED** (empty name-set diff 455==455; no prom delta; the StatsAsserter cross-vs-per-side set pinned) | R4 |

### 11.1 D-R1 (SPEC-BLOCKING) — the RANDOM surface: CONFIRMED

`Cluster_RANDOM Cluster_LbPolicy = 3` (`cluster.pb.go:125`). `grep -rn "RandomLbConfig"` over the v1.32.4 cluster package → ZERO hits — NO config message exists. The `lb_config` oneof wrapper types: `Cluster_RingHashLbConfig_` / `Cluster_MaglevLbConfig_` / `Cluster_OriginalDstLbConfig_` / `Cluster_LeastRequestLbConfig_` / `Cluster_RoundRobinLbConfig_` — FIVE members, NO `random` member → a stray non-random oneof member under a RANDOM cluster is mismatched-but-valid (nil-safe getters; silently retained; the manager never reads it on the RANDOM path → silent-ignore parity, §11.5). `go mod tidy -diff` → exit 0, EMPTY; `go build ./...` → OK (the SPEC worktree untouched). **ZERO new go.mod dep.**

### 11.2 D-R3 (SPEC-BLOCKING) — RANDOM stays UNIFORM under held-conn load: CONFIRMED LIVE

Bridge network `dr3net`, 3 socat streaming-echo backends (be1/be2/be3, STRICT_DNS), `tcp_proxy → lbcluster {RANDOM}`; `tcp.tcp.downstream_cx_rx_bytes_total: 192 (= 3×64)` verified after the bursts. Each burst = K=4 held conns (echo-confirmed, then held to elevate active-conn counts) + S=60 short round-trips = 64 picks. Per-host `cx_total` (== `rq_total`):

| Burst | be1 | be2 | be3 | total | leading host |
|---|---|---|---|---|---|
| 1 | 25 | 21 | 18 | 64 | be1 |
| 2 | 19 | 25 | 20 | 64 | be2 |
| 3 | 16 | 24 | 24 | 64 | be2 |

**Uniform-under-load CONFIRMED.** Per burst, observed range 16–25 around the uniform mean 21.3 (≈ ±1.4σ binomial, σ ≈ 3.77). The LEADING backend ROTATES burst-to-burst (be1 → be2 → be2) — RNG-driven, NOT load-driven. The 4 held conns concentrate active connections on specific backends each burst, yet the 64-pick distribution stays uniform and the heavy-active backend is NOT avoided — the exact contrapositive of phase-34 least_request `0059` (which skewed AWAY from the loaded backend). RANDOM ignores the active-conn count entirely. The proposed `0060` band: conservation `sum == 64` + per-host floor `>= 12` (mean − ~2.5σ; violated by a load-skewing policy starving a loaded backend, and by single-host pinning) + per-host ceiling `<= 32` (mean + ~2.8σ; violated by single-host pinning). §8.1.

### 11.3 D-R5 — the v1.37.2 RANDOM algorithm + the RNG reuse, pinned from source

`RandomLoadBalancer::peekOrChoose` (`random_lb.cc`):

```cpp
HostConstSharedPtr RandomLoadBalancer::peekOrChoose(LoadBalancerContext* context, bool peek) {
  uint64_t random_hash = random(peek);
  const absl::optional<HostsSource> hosts_source = hostSourceToUse(context, random_hash);
  if (!hosts_source) { return nullptr; }
  const HostVector& hosts_to_use = hostSourceToHosts(*hosts_source);
  if (hosts_to_use.empty()) { return nullptr; }
  return hosts_to_use[random_hash % hosts_to_use.size()];
}
```

`chooseHostOnce(context)` → `peekOrChoose(context, false)`. Pins: a SINGLE uniformly-random draw `random_hash`, indexed `random_hash % hosts_to_use.size()` — NO power-of-two loop, NO second draw, NO candidate comparison, NO active-count / load-counter consultation (the decisive contrast with least_request's P2C `lr.active[cand]` strict-`<` minimum); a single draw has NOTHING to tie-break. Healthy-host filtering is entirely UPSTREAM of the pick in the shared `ZoneAwareLoadBalancerBase` (`chooseHostSet` / `hostSourceToUse` panic-threshold logic / `hostSourceToHosts`); `peekOrChoose` only ever sees a pre-filtered `HostVector&`. **Boundary (identical to least_request):** envoy-go does no health checking → the priority/healthy/panic machinery degenerates to all-hosts → RANDOM picks uniformly over the full static endpoint slice (the Upstream-robustness family's territory; recorded at BEHAVIOR_CONTRACT). **RNG reuse:** `newPCGRNG()` (`leastrequest.go:63`) is ALREADY a package-private `func() (func() uint64, error)` (crypto-seeded, mutex-guarded `math/rand/v2` PCG); a new `random.go` in the SAME package calls it DIRECTLY — NO extraction (anticipated SHARED — CONFIRMED). The injectable seam `newRandomWithRNG(endpoints, rng)` mirrors `newLeastRequestWithRNG` minus `choiceCount`. Empty-set: both `roundRobin.Pick` and `leastRequest.Pick` return `Endpoint{}, noopRelease, errNoEndpoints` on `len == 0` — random mirrors this exactly (defense-in-depth; `buildCluster` already rejects zero-endpoint clusters via `extractEndpoints`).

### 11.4 D-R6 — the seam REUSE + the ADR-0045 envelope re-check

The ADR-0232 seam is reused UNCHANGED: `randomLB` implements the existing `loadBalancer` interface (`Pick() (Endpoint, func(), error)`) returning the shared `noopRelease` — NO `Pick`-signature change, NO interface change, NO `cluster.go` change, NO pick-funnel consumer touch (the same zero-churn posture `roundRobin` occupies). ADR-0232 §Consequences pre-authorizes it verbatim (DECISIONS.md:14936): *"the non-keyed RANDOM drops in at zero cost (same interface, ignores the counters)."* The hash-policy widening note (`Pick(hashInput)` for RING_HASH/MAGLEV) is NOT triggered by random (it is stateless, non-keyed). ADR-0024 (per-cluster RR counter scope, DECISIONS.md:591) is UNAMENDED — random holds NO per-cluster counter state (only the endpoint slice + the RNG); ADR-0024 §Consequences already names RANDOM as a zero-touch drop-in ("Future LB policies (LEAST_REQUEST, RANDOM, RING_HASH, MAGLEV …) … No existing code changes when they land."). **LoC:** `random.go` ~40–55 (strictly smaller than `leastrequest.go`'s 100 — it drops the P2C loop, the `active` slice, the `sync.Once` release, and reuses `newPCGRNG`) + the manager case ~5 + the reject-text line + the `manager_test.go` retarget → **~45–60 prod LoC / ~4–7 tasks** — BOTH ADR-0045 legs hold by an order of magnitude. SINGLE FLAT ROW 35; NO escape valve. The PLAN re-checks.

### 11.5 D-R2 — the validate matrix + the blast radius

The 5-variant `--mode validate` table (contrib-v1.37.2; base = tcp_proxy + STATIC cluster, 1 endpoint):

| Variant | Verdict | Reference output (decisive fragment) |
|---|---|---|
| `RANDOM`, no lb config | ACCEPT | `configuration … OK` |
| `RANDOM` + stray `least_request_lb_config: { choice_count: 5 }` (mismatched oneof) | ACCEPT, **silent** | `OK`; zero `[warning]`/`[error]` lines |
| `RANDOM` + stray `ring_hash_lb_config: {}` (mismatched oneof) | ACCEPT, **silent** | `OK`; zero warnings |
| `RING_HASH`, no lb config | ACCEPT | `OK` (envoy-go's continued rejection = recorded departure) |
| `MAGLEV`, no lb config | ACCEPT | `OK` (recorded departure) |

**Blast radius** (full-repo greps): production string — ONLY `manager.go:252` (`grep -rln "unsupported lb_policy" internal/ cmd/` → only manager.go); unit pinner — ONLY `TestManager_Error_UnsupportedLBPolicy` (`manager_test.go:320-330`; trigger `Cluster_RANDOM` at line 322 → must retarget to `RING_HASH`/`MAGLEV`; asserts substring `"ROUND_ROBIN, LEAST_REQUEST"` at line 327 → `"…, RANDOM"`); fixtures — ZERO hits (`grep -rln "ROUND_ROBIN, LEAST_REQUEST" test/` empty); docs — `BEHAVIOR_CONTRACT.md:899` (the reject-text line). NO cross-side boot-reject dir warranted (§8.2).

### 11.6 D-R6 — the ADR-0045 envelope re-check

~45–60 production LoC / ~4–7 tasks (§11.4) — BOTH legs FAR under the gate (`> ~25 tasks OR > ~1500 LoC`). SINGLE FLAT ROW 35; NO escape valve (there is no seam build to split off). The PLAN re-checks.

### 11.7 D-R4 — zero stat delta: CONFIRMED

Full `/stats` after traffic under RANDOM vs ROUND_ROBIN: 455 names each; the sorted name-set `comm -3` diff EMPTY both directions (`comm -23` and `comm -13` both empty). `/stats/prometheus` under RANDOM: `grep -iE 'random|lb_'` → only the pre-existing generic `cluster.<name>.lb_*` family (`lb_healthy_panic`, `lb_recalculate_zone_structures`, `lb_subsets_*`, `lb_zone_routing_*` — all = 0, identical under both policies; never mirrored). Surface STAYS **1116**. The StatsAsserter set (`0060`): cross-equal `upstream_cx_total` (192 ref under the probe burst; = 64 in the fixture) + `membership_total` (= 3) + `upstream_cx_active` (= 0 quiesced post-drain); per-side `upstream_rq_total` (reference = cx_total; subject = 0 — the tcpproxy never increments it). §7.

---

## 12. PLAN / IMPL D-questions (not empirical pins; resolved at PLAN/IMPL)

- **D-S35-1** — the file placement inside `internal/cluster` (anticipated: the new `randomLB` type in a NEW sibling `random.go`; the manager case + reject text in `manager.go`; the retarget in `manager_test.go` — the `leastrequest.go` precedent).
- **D-S35-2** — whether `newRandom` reuses `newPCGRNG` verbatim (the seed-error message reads `"cluster: least_request: seed rng"`) or wraps it for a random-flavored message (anticipated: call `newPCGRNG` directly and accept the shared message, OR add a thin one-line wrapper — PLAN/IMPL picks; `newPCGRNG` itself needs NO change, AMEND-R5).
- **D-S35-3** — the final band constants (floor 12 / ceiling 32 / sum 64 anticipated) + the tuning protocol: N≥20 local repeat runs of `0060` flake-free per side BEFORE landing, plus the three deliberate-break proofs with `-count=1` (§8.1) — a band that cannot fail is a dead assertion (`reference_differential_break_protocol_count1` generalized to BAND assertions, the `0059` lesson).
- **D-S35-4** — whether the in-process anti-skew integration test (Task 4) is a standalone unit test or folded into the fixture's deliberate-break proof (anticipated: a small deterministic-RNG unit test proving RANDOM does NOT avoid a loaded endpoint, distinct from the fixture).
- ADR-0045 split-gate FINAL re-check at PLAN.

---

## 13. ADR continuity — the ADR-0234 §Context DRAFT (anchored here; the full entry lands at the IMPL)

Per the phase-35 routing (next-prompt + STATE + BRAINSTORM §7), the DECISIONS.md tail **STAYS ADR-0233 at this SPEC** (counts UNCHANGED — contrast the phase-34 SPEC, which seeded its TWO §Context entries into DECISIONS.md; phase 35 has a single reuse-case ADR with no seam-build to pre-document). The ADR-0234 §Context is anchored as a DRAFT HERE; the full ADR-0234 entry (§Context + §Decision + §Consequences, status PROPOSED → ACCEPTED) lands at the phase-35 IMPL per ADR-0044.

**ADR-0234 §Context DRAFT (the `random` load-balancing policy):** Phase 35 lands `Cluster.LbPolicy RANDOM` (`envoy.config.cluster.v3`, enum value 3) on the LEGACY enum path (the same path ROUND_ROBIN + LEAST_REQUEST acceptance uses; the `Cluster.load_balancing_policy` extension point stays deferred) — the project's THIRD LB policy and the SECOND Load-balancing-family row. RANDOM has NO config message (`Cluster.RandomLbConfig` does not exist in v1.32.4 — zero config knobs; the simplest family surface) → NO config-parse arm, NO PGV gate. The `randomLB` is a STATELESS uniform-pick LB mirroring upstream v1.37.2's `RandomLoadBalancer::peekOrChoose` EXACTLY: ONE draw `rng() % len(endpoints)`, pick the indexed endpoint — it consults NO active counters and holds NO per-pick state (the contrast with least_request's P2C). It REUSES the ADR-0232 LB acquire/release seam UNCHANGED — implementing the existing unexported `loadBalancer` interface and returning the shared `noopRelease` (the seam's FIRST reuse, validating its "the non-keyed RANDOM drops in at zero cost" §Consequences claim — ZERO seam churn, ZERO seam ADR; the thrift-33 single-ADR-on-reuse precedent). The RNG is the EXISTING package-private `newPCGRNG()` helper (a mutex-guarded crypto-seeded `math/rand/v2` PCG), called directly with NO extraction; the injectable `newRandomWithRNG` test seam mirrors the upstream mock posture. `cluster.Manager` accepts `RANDOM` beside ROUND_ROBIN + LEAST_REQUEST (the ONE deliberate byte-stable-reject text change — `manager.go:252`'s supported-list extends `… ROUND_ROBIN, LEAST_REQUEST` → `…, RANDOM`; blast radius three sites, no fixture pins it — D-R2) and silently ignores a mismatched `lb_config` oneof member under RANDOM (reference PARITY — the manager never reads the oneof on the RANDOM path; behavior-inert). Healthy-host filtering happens upstream of the pick in the reference's shared base (priority/healthy/panic) → with no health checking it degenerates to all-hosts sampling (the Upstream-robustness family's boundary). The differential proof is DISTRIBUTIONAL and ANTI-SKEW (the contrapositive of least_request): fixture `0060-lb-random` — the REUSED `0059` held-conn skew stimulus asserted via the SECOND band-based `AssertDistribution` use to STAY uniform (conservation + uniform-floor + uniform-ceiling legs, PER SIDE — RNG-driven on both sides, never cross-side-exact) DESPITE the load imbalance + a cross-side `StatsAsserter` prong (`upstream_cx_total`/`membership_total`/quiesced-active cross-equal; `upstream_rq_total` per-side); NO new fuzzer (no wire decode) + NO new BackendKind (tail stays 33) + ZERO stat-name delta (surface stays 1116 — the second zero-delta phase, live-confirmed) + ZERO new packages + ZERO new go.mod deps. ADR-0024 (per-cluster RR counter scope) is UNAMENDED (random holds no per-cluster counter state; ADR-0024 §Consequences already names RANDOM as a zero-touch drop-in).

§Decision/§Consequences bodies land at the phase-35 IMPL per ADR-0044 (next-free after phase 35 ≈ **ADR-0235**). The PLAN/IMPL may surface additional ADRs (each re-checks).

---

## 14. Exit — counts + ROADMAP/STATE at SPEC-DONE

At the SPEC-DONE commit (ALL counts UNCHANGED at the SPEC — including the DECISIONS tail; they advance at the IMPL):

- stat surface **1116** (→ **1116** at the IMPL — ZERO delta, AMEND-R4; the second zero-delta phase).
- differential fixtures **61** (→ **62** at the IMPL: `0060-lb-random`; NO boot-reject dir — AMEND-R2).
- fuzzers **42** (→ **42** — NO new fuzzer, deliberate, §8.3).
- BackendKind tail **33** (→ **33** — NO new BackendKind, deliberate, §8.3).
- DECISIONS.md tail **ADR-0233** (STAYS ADR-0233 at this SPEC — the ADR-0234 §Context is a DRAFT in §13; the full ADR-0234 entry lands at the IMPL per ADR-0044; next-free **ADR-0234**).
- ROADMAP row 35 STAYS `in-progress` (it flips `→ done` at the phase-35 IMPL six-gate — a flat family row, NO parent rollup per ADR-0106); the Load-balancing family stays OPEN (6 candidates remain after 35).
- spec-document-reviewer gate applies at this SPEC.
- Next → the **phase-35 PLAN** (`superpowers:writing-plans` — decompose §10 into bite-sized TDD tasks; FINAL ADR-0045 gate re-check).
