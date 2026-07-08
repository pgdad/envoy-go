# Phase 54 Brainstorm — `healthy_panic_threshold` as an independent construct (EIGHTH and FINAL Load-balancing-family row; the family CLOSES at phase-done)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 54 (`load-balancer-panic-threshold`), the **EIGHTH Load-balancing-family row** and the family's LAST deferred candidate. Phase 54 turns `Cluster.CommonLbConfig.healthy_panic_threshold` from a **consumed-but-never-proven** field into a proven, hardened, independently-observable construct: the parse ALREADY exists (`parsePanicThreshold`, `internal/cluster/health.go:483-491` — `envoy.type.v3.Percent`, default 50%, returned as a fraction; `inPanic`'s strict `<` at `health.go:180-186`), and every leaf policy consults it via the shared `panicGate` (`health.go:204-219`, consolidated there by the 2026-07-07 repo-wide maintenance pass) — but **no unit test anywhere passes a non-default threshold through the parse path** (every `newClusterHealth` call in `internal/cluster/*_test.go` hardcodes `0.5`), **no fixture sets the field**, and **the classic threshold-driven panic path has never been differentially proven** (`lb_healthy_panic` has only ever been asserted `== 0` in the health-check fixtures 0066-0068; phase 53's capacity-shortfall bypass reuses the counter but neither `0096` arm triggers it).

The load-bearing facts that shape this brainstorm:

- **The construct exists; the PROOF does not.** Unlike every prior LB phase (which built a new policy/wrapper), phase 54 builds almost no new mechanism — it proves and hardens an existing one. The nearest structural precedent is *not* a wrapper phase but the "prove the existing surface against the reference, fix what the evidence indicts" posture each SPEC's empirical-pin section already takes, promoted to a whole phase.
- **Two panic-scoped defects are already known and deferred, waiting for exactly this phase.** The 2026-07-07 repo-wide maintenance review (`REVIEW_FINDINGS.md`, landed as `9f26f380` — absorbed and fully re-verified at the start of THIS session, including a full 98/98 differential re-run) implemented only behavior-preserving fixes and DEFERRED two cluster findings squarely in this phase's territory: (1) **`lb_healthy_panic` double-increments under locality-weighted panic** — confirmed live in code this session: `localityWeightedLB.Pick` (`locality.go:143-146`) calls `panicInc()` then delegates to its flat fallback child, which was built via the SHARED-health `buildLeafLB` closure (`manager.go:479-481` via `newLocalityWeightedLB`, `locality.go:67`), so the flat leaf's own `panicGate` fires `panicInc()` AGAIN — one panic pick, two increments; (2) the **`locality.go` child-local-panic coverage boundary** — phase 52's accepted-but-unfixed gap (each locality child's leaf evaluates `inPanic` over just ITS OWN endpoint sub-slice), which phase 53 fixed for priority tiers (`tierHealth`, AMEND-P1-COROLLARY) and explicitly flagged as "a candidate FUTURE maintenance item" for `locality.go` (phase-53 PLAN D-P-RETROFIT: "confirmed explicitly OUT of this phase's scope"). Phase 54 is that item's home — gated on its own locality-scoped live probe (D-PT4), per the project's evidence-before-fix discipline.
- **Phase 53's AMEND-P1 finding scopes this phase cleanly.** The reference's bypass condition for a multi-tier-priority cluster is the capacity-sum check `Σ_i min(100, OPF × healthy_fraction_i) < 100`, NOT the classic `healthy_panic_threshold` — the phase-53 SPEC explicitly noted this "is itself a useful precedent for that future row to read first, since it demonstrates the classic 50% threshold is not universally the relevant panic concept once priority tiers are in play." Consequence: the configured threshold's INERTNESS in a multi-tier-priority cluster (per-tier `tierHealth` views are panic-disabled; the shortfall bypass does not read the threshold) is **confirmed-correct reference behavior, not a gap** — phase 54 documents it as a coverage boundary and scopes itself to the FLAT (single-priority) cluster shapes where the classic threshold is the live mechanism.
- **Zero new config surface.** The field is already parsed; the deltas are semantic hardening (the double-increment fix, the locality retrofit, out-of-range validation parity if the reference has it) and PROOF (the differential + unit tests through the parse path). ZERO new `Pick` parameters, ZERO new packages, ZERO new go.mod deps, anticipated ZERO stat-surface delta (the counter names already exist).

The next sessions author the SPEC then the PLAN then the IMPL. The SPEC executes the §10 empirical-pin obligations (D-PT1..D-PT6) IN-SESSION against the contrib reference Envoy (`envoyproxy/envoy:contrib-v1.37.2`, ADR-0227) via the live-probe precedent (`reference_docker_probe_bridge_network`), and anchors the ADR-0271 §Context draft.

**Brainstorm session:** worktree `.worktrees/phase-54-brainstorm`, branch `phase-54-load-balancer-panic-threshold-brainstorm`. Substantive predecessors on master: the phase-53 IMPL squash `d3767641` (ADR-0270), the repo-wide maintenance pass `9f26f380` (+ merge `4156b747`), and the differential-harness subject-start port hardening `0e9cc680` (this session's own verification-driven fix: probed low-range port block + shared retry with backoff, landed after the maintenance pass's absorption verification root-caused an elevated bind-collision flake rate to environment + harness design, NOT to the pass). Counts at master tip (re-verified fresh this session, NOT trusted from any prior file): stat surface **1200**, differential fixtures **98** (tail `0096-lb-priority`), fuzzers **52**, BackendKind tail **38** (`H2GoawayResponder`; 39 kinds total), DECISIONS tail **ADR-0270** (next-free **ADR-0271**). ALL counts stay UNCHANGED at this brainstorm.

**Brainstorm mode:** interactive with a live human. The user picked the family/candidate and two design questions via a multi-question dialogue (this session):

- **Q-family/Q0 candidate pick** — of the three open families' nine deferred candidates (the "awaiting a human pick" state `next-prompt.txt` was in at cold-start), the user picked **Load-balancing → panic thresholds as an independent construct** — the family's last remaining candidate.
- **Q1 scope** — **Prove + harden** (Recommended, chosen): the first-ever differential proof of a configured threshold + the live semantic pins + the `lb_healthy_panic` double-increment fix + the `locality.go` child-local-panic retrofit (probe-gated), all panic-scoped. The alternatives — minimal prove-only; additionally folding in the `membership_healthy`-ignores-ejections deferred bug (a general health-observability widening); additionally lifting `zone_aware_lb_config.fail_traffic_on_panic` (partially accepting a wholesale-rejected oneof arm) — were presented and declined. §2.2.
- **Q2 differential proof shape** — **the two-cluster discriminator** (Recommended, chosen): ONE fixture dir, ONE boot, TWO clusters differing ONLY in threshold (80% vs default 50%), the SAME degraded health state (2/5 hosts down → 60% healthy) — cluster A panics, cluster B does not. Proves the FIELD is the live discriminator with hard 0-vs-nonzero assertions and a single convergence step. The alternatives (a one-cluster two-health-states shape whose exactly-at-threshold arm rides on cross-side float-comparison parity; both dirs) were presented and declined. §2.3/§6.

Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `ROADMAP.md`, `ENVOY_TARGET.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 .. ADR-0270 — especially ADR-0242/0243 [the phase-39 health-state model + `clusterHealth`] + ADR-0269/0270 [the two wrapper phases whose flat-fallback/child-health wiring this phase corrects and retrofits] + ADR-0080 [byte-stable rejects] + ADR-0106/0045/0044/0052), the as-built `internal/cluster` package (post-maintenance-pass shape: `health.go` [`panicGate`/`inPanic`/`parsePanicThreshold`], `locality.go` [the double-increment + child-local-panic sites], `priority.go` [`tierHealth` — the retrofit's structural template], `manager.go` [the three `parsePanicThreshold` call sites]), and `REVIEW_FINDINGS.md` (the deferred-findings roster). Empirical pins requiring evidence against the contrib reference Envoy are enumerated in §10 and deferred to SPEC-drafting time per the phase 09–53 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/53-load-balancer-priority/BRAINSTORM.md` section-for-section (the family's established shape), adapted for a prove-and-harden phase rather than a new-wrapper phase: no new `Endpoint` dimension, no new wrapper, no new draw formula — instead a proof surface (§6), two evidence-gated corrections (§2.2), and a semantics-pin battery (§10). Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear.

**Authored:** 2026-07-08.

---

## 1. Mission and scope confirmation (54 only)

ROADMAP row `54 | load-balancer-panic-threshold | 53 | in-progress | | …` (added by this brainstorm) is a **flat top-level Load-balancing-family row** (per ADR-0106). The row's `depends-on` anchor is phase 53 (the last completed phase; substantive predecessor `d3767641`, plus the orthogonal maintenance pass `9f26f380` and harness hardening `0e9cc680`).

The Load-balancing family candidate roster at `ROADMAP.md` immediately BEFORE this brainstorm's registration commit was the post-phase-53 list: `{panic thresholds as an independent construct}` (1 candidate). Phase 54 consumes it — **the family's candidate list empties**; the family CLOSES at phase-done (§1.3). Branch/directory identifiers: branch `phase-54-load-balancer-panic-threshold-brainstorm`, directory `54-load-balancer-panic-threshold/`. The work lands in the EXISTING `internal/cluster` package (edits to `health.go`/`locality.go`/`manager.go` + their tests) — NO new file anticipated (the retrofit reuses `priority.go`'s `tierHealth` or a small generalization of it; exact placement a PLAN detail), NO new Go package.

### 1.1 What phase 54 delivers as a self-contained whole (envelope: a single flat row, no pre-authorized split)

1. **The differential proof `0097-lb-panic-threshold`** (§6) — the first fixture (and first test of ANY kind) to set `common_lb_config.healthy_panic_threshold`, and the first differential to drive the classic threshold-panic path at all: the two-cluster discriminator (Q2). Fixtures 98 → **99**.
2. **The `lb_healthy_panic` double-increment fix** — the deferred `REVIEW_FINDINGS.md` cluster finding, confirmed live this session (§2.2). The fix SHAPE is pinned at SPEC by D-PT1's increment-semantics probe (anticipated: the reference increments once per panic pick; anticipated fix: build `localityWeightedLB`'s flat fallback child with a NIL health registry — the exact `priorityLB` precedent, ADR-0270's own documented divergence-risk rationale — OR a panic-disabled view, whichever the pinned semantics dictate). This changes a stat VALUE on an existing (broken) path toward reference parity — precisely the class the maintenance pass deferred to "differential/reference verification", which `0097` supplies: with once-per-pick semantics pinned, the panic arm can assert the counter EXACTLY (== the request count), cross-side.
3. **The `locality.go` child-local-panic retrofit** (probe-gated, D-PT4) — phase 52's accepted coverage boundary, phase 53's D-P-RETROFIT deferral, closed HERE if (and only if) the locality-scoped live probe confirms the reference never lets a single degraded locality internally flatten itself (the AMEND-P1 per-tier evidence, re-established one construct over rather than assumed to transfer). Anticipated mechanism: panic-disabled child health views (the `tierHealth` pattern) for the per-locality children; the flat fallback child's handling folds into deliverable 2.
4. **Out-of-range validation parity** (probe-gated, D-PT3) — `envoy.type.v3.Percent.value` is a plain `float64` in the binding; envoy-go currently accepts ANY value (>100 yields an unreachable threshold >1.0; negative disables). The reference's PGV validation for the field (if any) is probed and mirrored byte-stable (ADR-0080); no reject is invented if the reference accepts.
5. **Unit-test hardening through the REAL parse path** — boundary strictness at exactly-the-threshold (strict `<`, D-PT1), the disable-at-0 convention (explicit `{value: 0}` vs absent-defaults-to-50%, D-PT2), and non-default thresholds threaded through `parsePanicThreshold` → `newClusterHealth` → `panicGate` (closing the every-test-hardcodes-0.5 gap).
6. **The BEHAVIOR_CONTRACT bundle** + the STATE/ROADMAP advance + the row-54 `in-progress → done` flip at the IMPL six-gate + the **Load-balancing family CLOSE** (§1.3).

### 1.2 What phase 54 does NOT deliver (forward to §8)

See §8. Highlights: **`zone_aware_lb_config` stays wholesale-rejected** (including `fail_traffic_on_panic` and `min_cluster_size` — declined at Q1); **the runtime override** (`upstream.healthy_panic_threshold`) stays absent (no Runtime/RTDS family exists); **`membership_healthy`-ignores-ejections** stays a deferred `REVIEW_FINDINGS.md` item (declined at Q1 — a general health-stat concern, not panic-scoped); **per-priority-tier panic threshold semantics** stay as phase 53 pinned them (AMEND-P1 — the threshold is correctly inert in multi-tier clusters); **subset-child panic scoping** is probed for information (D-PT4's sub-question) but only corrected if the evidence indicts it (anticipated: the reference computes panic per subset host-set, so `subsetLB`'s shared-health children may already be faithful — unlike locality's).

### 1.3 Phase-done as the EIGHTH Load-balancing-family row landing — the family CLOSES

Phase 54 consumes the family's LAST candidate. At the phase-54 IMPL six-gate, the Load-balancing family's deferred-candidate list is EMPTY and the family **CLOSES** — the FOURTH family to close (after HTTP-filters @ 25.3, Network-filters @ 33, Upstream-robustness @ 43), leaving Observability (5 candidates) and Operational tooling (3 candidates) as the open families. The family's durable assets: the five leaf policies (34/35/36/37), the subset wrapper (38), the health substrate consumption (39-43 boundary), locality-weighted (52), priority (53), and — from this phase — the proven, hardened panic construct every one of them shares.

### 1.4 No pre-authorized split — a single flat row

No second subsystem exists to couple against. Anticipated **~120–250 prod LoC / ~8–10 tasks** (the smallest LB-family phase yet — no new policy, no new wrapper; the LoC is the two corrections + validation parity + tests), FAR below the ADR-0045 `> ~25 tasks OR > ~1500 LoC` split gate. D-PT6 re-confirms at SPEC.

### 1.5 Seed-stub alignment + package placement

ZERO new packages, ZERO new files anticipated (edits to `health.go` [possible validation reject + any `parsePanicThreshold` presence-semantics adjustment out of D-PT2], `locality.go` [the retrofit + the flat-child health wiring], `manager.go` [reject wiring if D-PT3 warrants], plus `_test.go` siblings and the `0097` fixture dir). ZERO new go.mod deps (`envoy.type.v3.Percent` is already imported transitively via `CommonLbConfig`; D-PT3's `go mod tidy` confirms).

### 1.6 No prebrainstorm-notes branch

No `phase-54-*-prebrainstorm-notes` branch exists. Phase 54 starts cleanly from this BRAINSTORM.md.

### 1.7 Phase 54's relationship to the LB seam (REUSE with ZERO widening — trivially)

`loadBalancer.Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error)` stays byte-for-byte UNCHANGED — this phase does not even add a wrapper, let alone a parameter. All deltas are constructor-time health-view wiring (the phase-53 AMEND-P1-COROLLARY precedent: "purely a constructor-time wiring choice" that "costs nothing against the zero-new-`Pick`-parameter design goal") and parse-time validation.

---

## 2. Design decisions

### 2.1 Subject + scope confirmation: `CommonLbConfig.healthy_panic_threshold` — the v1.32.4 proto surface VERIFIED *(Q-family/Q0 → phase 54 row registered)*

**Decision:** Phase 54 = the classic healthy-panic mechanism as an independent construct: `Cluster.CommonLbConfig.healthy_panic_threshold` (`envoy.type.v3.Percent` — a `float64` `value` field, verified in the v1.32.4 module cache this session) consumed by `parsePanicThreshold` (already landed, phase 39) and gating every leaf policy via `clusterHealth.panicGate`/`inPanic` (strict `<`; default 0.5).

**The as-built consumption map (brainstorm finding, verified this session against post-maintenance-pass HEAD):**

```
internal/cluster/health.go:483-491  parsePanicThreshold — Percent→fraction, nil→0.5 default; NO range validation
internal/cluster/health.go:180-186  inPanic — availableCount/total < panicThreshold (strict <; total==0 → false)
internal/cluster/health.go:204-219  panicGate — nil-health fast path + inPanic→panicInc→bypass (all 5 leaves)
internal/cluster/locality.go:143-146 localityWeightedLB.Pick — cluster-wide inPanic → panicInc → flat child
                                     (flat child SHARES health → the double-increment; children share health
                                      → the child-local-panic boundary)
internal/cluster/priority.go        priorityLB — tierHealth panic-DISABLED views; capacity-sum bypass does
                                     NOT read the threshold (AMEND-P1 — confirmed-correct inertness)
internal/cluster/manager.go:461/501/536 (pre-pass numbering) — the three newClusterHealth(…, parsePanicThreshold(c)) sites
```

**Consequence:** the MVP consumes NOTHING new from the proto surface; it hardens the existing consumption (§1.1). The adjacent `CommonLbConfig` fields stay as they are: `locality_weighted_lb_config` (phase 52), `zone_aware_lb_config` (wholesale reject, unchanged — §8), `update_merge_window`/`ignore_new_hosts_until_first_hc`/`close_connections_on_host_set_change`/`consistent_hashing_lb_config`/`override_host_status` (all deferred, unchanged).

**Anticipated ADRs:** ADR-0271 ONLY (§7).

### 2.2 Scope: prove + harden — the two evidence-gated corrections ride along *(Q1, the user's choice)*

**Decision:** phase 54 bundles the PROOF (the `0097` differential + the parse-path unit tests + the semantic pins) with the two panic-scoped corrections the evidence already (or foreseeably) indicts: the `lb_healthy_panic` double-increment fix (evidence in hand — code inspection this session; reference increment semantics pinned at D-PT1 before the fix shape is chosen) and the `locality.go` child-local-panic retrofit (evidence pending — D-PT4's locality-scoped probe; NOT assumed to transfer from phase 53's per-tier AMEND-P1 finding, though that finding makes the outcome likely). The declined widenings: `membership_healthy`-vs-ejections (not panic-scoped; stays deferred in `REVIEW_FINDINGS.md`), `fail_traffic_on_panic` (would partially lift a wholesale oneof-arm reject — a bigger, riskier architecture question deserving its own dialogue if ever pursued).

**Rationale (the user's Q1 answer):** the double-increment is a panic-OBSERVABILITY bug on exactly the counter this phase's differential asserts — landing the proof without the fix would mean differentially PINNING the wrong value (or weakening the assertion to `>0` forever); landing them together lets `0097` assert the exact reference-parity value. The retrofit is this phase's namesake concern (panic scoping) with a landed structural template (`tierHealth`) and a one-probe evidence gate; deferring it a THIRD time (after phases 52 and 53 both flagged it) when the phase is literally about panic semantics would be evasion, not discipline.

### 2.3 Differential strategy: the two-cluster discriminator — same health state, different threshold *(Q2 → fixture envelope §6)*

**Decision:** ONE fixture dir (`0097-lb-panic-threshold`), ONE boot per side, TWO health-checked clusters differing ONLY in `healthy_panic_threshold`: cluster A explicit **80%**, cluster B **absent** (the 50% default — also proving the default arm through the same fixture). 5 HTTP hosts each (the phase-39/53 toggle-responder pattern); the driver degrades the SAME 2-of-5 in each (60% healthy), polls convergence, then drives N requests per cluster via path-routing. **Cluster A (60% < 80%): panic** — every host INCLUDING the 2 unhealthy receives traffic (per-host `> 0`, offset-invariant per `reference_round_robin_offset_randomized`), `lb_healthy_panic` increments (EXACTLY == N if D-PT1 pins once-per-pick). **Cluster B (60% ≥ 50%): no panic** — the 2 unhealthy hosts receive EXACTLY ZERO, the 3 healthy receive all N, `lb_healthy_panic` stays 0. Hard 0-vs-nonzero assertions, NO statistical bands (the phase-53 Q4 lesson, reapplied). Cross-side `StatsAsserter` (`reference_differential_asserter_dispatch`); deliberate-break liveness with `-count=1` (§6).

**Rationale (the user's Q2 answer):** holding health constant and varying ONLY the threshold proves the FIELD is the live discriminator — the phase's entire point — in one convergence step, with no exactly-at-boundary float-parity flake risk (the declined one-cluster shape's weakness; the boundary is pinned at SPEC via live probe + unit test instead, the cheaper precedent).

### 2.4 Validation parity posture: probe, then mirror — never invent *(self-answered; ADR-0080)*

**Decision:** D-PT3 probes the reference's acceptance of out-of-range `healthy_panic_threshold` values (`>100`, negative, and the degenerate `NaN` if expressible via YAML) at boot. If the reference PGV-rejects, envoy-go mirrors byte-stable (ADR-0080; the phase-49 `reference_sibling_reject_test_needs_real_typeurl` lesson noted for the test design); if the reference accepts (even nonsensically), envoy-go accepts identically — no invented strictness.

### 2.5 Panic semantics: what the pins must nail down *(self-answered envelope; SPEC-BLOCKING, D-PT1/D-PT2/D-PT4)*

**Decision:** the SPEC pins, via live probes: (i) **boundary strictness** — envoy-go implements strict `<` ("exactly 50% does NOT panic"); confirmed or corrected against the reference at a non-trivial boundary (e.g. threshold 60%, 3/5 healthy = exactly 60%); the percent-comparison shape guards against the truncation trap (`reference_percent_cap_cross_multiply`). (ii) **Disable-at-0** — the reference documents `healthy_panic_threshold: {value: 0}` as disabling panic; envoy-go's `parsePanicThreshold` returns 0.0 for an explicit 0 (→ `inPanic` never fires, a fraction is never `< 0`) but ALSO returns 0.5 for ABSENT — the wrapper-presence semantics (`GetHealthyPanicThreshold() == nil` vs explicit zero) are confirmed to match the reference's (the D-LW-OPF0 presence-vs-zero discipline, reapplied to a plain message field). (iii) **The panic pick distribution per policy** — in panic the reference routes over ALL hosts via the policy's own algorithm; envoy-go's leaves do the same (`panicGate`-then-blind-index); `0097` asserts the observable consequence (every host `> 0`), not sequence identity. (iv) **Increment semantics** — which layer increments `lb_healthy_panic`, once per pick (pins the double-increment fix shape AND `0097`'s exact-value assertion). (v) **Locality child scoping** — D-PT4, the retrofit gate (§2.2).

### 2.6 Stat surface: anticipated ZERO delta; a VALUE correction, not a name change *(self-answered; D-PT5)*

**Decision:** anticipated stat surface **1200 → 1200** (the family's FIFTH zero-stat-delta phase). `lb_healthy_panic` and `membership_healthy` already exist (phase 39); no dedicated panic gauge is believed to exist in the reference (D-PT5 confirms by scraping `/stats` under a panicking cluster). The double-increment fix changes the counter's VALUE on the locality-weighted panic path (2N → N) — a semantics correction toward reference parity, differentially pinned by `0097`… noting `0097` itself uses PLAIN (non-locality-weighted) clusters, so the double-increment's differential visibility rides on the leaf/`localityWeightedLB` layering probe in D-PT1; if the plain-cluster path proves single-increment already (likely — `panicGate` fires once per leaf pick), the FIX is proven by unit tests on the locality path plus the D-PT1-pinned reference value, the honest scope of which is settled at SPEC.

---

## 3. Framework-survey result — ZERO seam widening, ZERO new packages, ZERO new deps

### 3.1 Framework seam: UNTOUCHED

No `Pick` signature change, no wrapper, no ctx-carry (§1.7). ADR-0271 covers policy semantics (the proof + the two corrections + validation parity), not a seam.

### 3.2 NEW packages: NONE — edits to existing `internal/cluster` files only (§1.5)

### 3.3 go.mod deps: anticipated ZERO new *(verified at brainstorm; re-pinned at SPEC D-PT3)*

`envoy.type.v3.Percent` ships in the EXISTING `github.com/envoyproxy/go-control-plane/envoy v1.32.4` dep (verified in the module cache this session — `type/v3/percent.pb.go`, `Value float64` field 1) and is already consumed by `parsePanicThreshold`.

### 3.4 REUSES

- `internal/cluster` (39/40/52/53) — `clusterHealth`/`hostHealth` (ADR-0242/0243), `panicGate`/`inPanic`/`parsePanicThreshold` (the constructs under proof), `tierHealth` (`priority.go` — the retrofit's structural template, ADR-0270/AMEND-P1-COROLLARY), the `buildLeafLB` factory closures (the health-view wiring sites).
- The phase-39 health-check substrate + toggle responder — `0097`'s controlled degradation (arm drive), `reference_health_check_propagation_warmup`'s poll-to-convergence gate (NOT a sleep), the `0096` per-host-tally pattern.
- The differential harness — `StatsAsserter`, `-count=1` break protocol, the fixture-dispatch constraint, and this session's own `startSubjectWithRetry`/`freeTCPPortBlock` hardening (`0e9cc680`).
- `envoy.config.cluster.v3` + `envoy.type.v3` (existing deps).

---

## 4. Per-route applicability — NONE (a cluster-only construct)

`CommonLbConfig` is cluster-scoped; there is no per-route/per-listener producer surface. Identical posture to phases 52/53 §4.

---

## 5. Stat surface hypothesis — ZERO delta (D-PT5 confirms); a value-semantics correction on an existing counter

### 5.1 New stat names: NONE anticipated (surface stays **1200**)

### 5.2 envoy-go-strict departure flags (anticipated; SPEC + IMPL pin)

The deferred-flag set: `zone_aware_lb_config` (wholesale reject — PRE-EXISTING, unchanged); the runtime override (absent — no Runtime family; silently N/A, not a reject); any D-PT3 range-validation posture. The double-increment, once fixed, REMOVES a latent value-level divergence rather than adding a departure. Histograms deferred project-wide (ADR-0060).

---

## 6. Differential fixture envelope — ONE directory

### 6.1 Fixtures (+1)

- **`0097-lb-panic-threshold`** (cross-side): one HTTP listener, path-routed to TWO health-checked STATIC clusters × 5 hosts each (driver-owned toggle responders): cluster A `healthy_panic_threshold: {value: 80}`, cluster B field ABSENT (50% default). Drive: degrade 2/5 per cluster → poll `membership_healthy` to 3 per cluster per side (`reference_membership_total_vs_healthy_gauge`: both clusters ARE health-checked, so the gauge exists on both sides) + the warmup gate → N requests per cluster. Assert (per-host backend tallies + `StatsAsserter`): **A** — all 5 hosts `> 0`; `lb_healthy_panic` `> 0` (EXACT `== N` if D-PT1 pins once-per-pick). **B** — the 2 degraded hosts `== 0`; the 3 healthy sum to N; `lb_healthy_panic == 0`. Deliberate breaks with `-count=1`: (a) hardcode the parse to 0.5 ignoring the field → cluster A stops panicking → its unhealthy-hosts-`>0` assertion fails; (b) skip the degradation step → both clusters serve healthy-only → A's panic assertions fail. Workload constants synced per `reference_fixture_workload_constant_desync`; run selector `-run 'TestDifferential/0097'` per `reference_differential_run_selector`; ≥20-run flake-free gate pinned at SPEC.
- **(possible) a boot-reject dir** — ONLY if D-PT3 finds a reference-side range reject worth cross-side proof (a SEPARATE dir per `reference_differential_fixture_dispatch_constraint`); anticipated a unit test suffices (the phase-52/53 reject-test precedent).

### 6.2 Total: 98 → **99** (a single `0097` dir anticipated)

### 6.3 New BackendKind: NONE — reuses the phase-39/53 toggle-responder pattern (driver-owned); tail STAYS **38**

### 6.4 New fuzzer: NONE anticipated + no conformance-harness impact — no wire-decode surface (the phase-52/53 reasoning); D-PT6 confirms. h2spec/proxy-wasm re-run asserted-unaffected at the six-gate.

---

## 7. Anticipated ADRs — 1 ADR (ADR-0271)

Next-free ADR at master tip is **ADR-0271** (DECISIONS tail **ADR-0270**, unchanged at this brainstorm).

- **ADR-0271** *(the healthy-panic-threshold construct: proof + hardening)* — the classic threshold's pinned semantics (boundary strictness, disable-at-0, presence-vs-zero, per-policy panic distribution, increment semantics); the `lb_healthy_panic` double-increment fix; the `locality.go` child-local-panic retrofit (with its D-PT4 evidence, closing the phase-52 boundary phase 53's AMEND-P1-COROLLARY improved on for tiers); the D-PT3 validation posture; the confirmed inertness boundary for multi-tier-priority clusters (AMEND-P1 cross-reference); the `0097` proof shape. §Context anchored at SPEC; §Decision/§Consequences at IMPL per ADR-0044 (next-free after phase 54 ≈ **ADR-0272**).

---

## 8. Deferred items

- **`zone_aware_lb_config`** — stays wholesale-rejected, including `fail_traffic_on_panic` (503-on-panic; declined at Q1 — partially lifting a rejected oneof arm is its own architecture question) and `min_cluster_size`/`routing_enabled` (zone-aware routing needs local-zone awareness envoy-go does not have).
- **The runtime override `upstream.healthy_panic_threshold`** — couples to the absent Runtime/RTDS family; N/A project-wide until such a family opens.
- **`membership_healthy` ignores outlier ejections** — the OTHER deferred `REVIEW_FINDINGS.md` cluster finding (declined at Q1); stays on that roster for a future maintenance or health-observability effort, along with its siblings (`maximum_ring_size` unenforced; sequential health-probe interval stretch; ring/maglev gauges lost under wrapping).
- **Per-priority-tier panic thresholds** — CONFIRMED N/A (phase 53 AMEND-P1: the reference's multi-tier bypass is capacity-sum-based; the classic threshold is correctly inert there). Documented as a coverage boundary, not deferred work.
- **Subset-child panic scoping** — probed for information at D-PT4; corrected ONLY if the evidence indicts envoy-go's current shape (anticipated: the reference computes panic per subset host-set, matching the current shared-health child shape).
- **`CommonLbConfig`'s remaining fields** — `update_merge_window`, `ignore_new_hosts_until_first_hc`, `close_connections_on_host_set_change`, `consistent_hashing_lb_config`, `override_host_status` — unchanged, each its own future candidate if ever chartered.

---

## 9. Cross-references against prior phases' deferred-items lists — closure pickup

- **The phase-52 child-local-panic accepted boundary** (phase-52 PLAN/IMPL; restated at phase-53 SPEC AMEND-P1-COROLLARY as "a candidate FUTURE maintenance item… out of THIS phase's scope" and at phase-53 PLAN D-P-RETROFIT) — **CLOSED here** (probe-gated, D-PT4).
- **The phase-53 SPEC §2 note to this row** ("the AMEND-P1 finding… is itself a useful precedent for that future row to read first") — **honored**: §2.1/§8 scope this phase to flat clusters and document the multi-tier inertness as confirmed-correct.
- **The `REVIEW_FINDINGS.md` deferred cluster roster** (2026-07-07 maintenance pass) — **partial pickup**: the `lb_healthy_panic` double-increment lands here (panic-scoped); the rest stays deferred (§8).
- **The phase-38 §2.1 health-gating note** ("locality-weighted/priority/panic gated by the absent health-checking boundary") — the THIRD and LAST of the three named candidates lands; the gate note fully discharges.
- **The `tierHealth` primitive** (phase 53, ADR-0270) — REUSED as the retrofit's structural template (§2.2/§3.4), its second consumer.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution (empirical pins against the contrib reference Envoy per ADR-0004/ADR-0227)

The SPEC author executes these IN-SESSION against `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227) + go-control-plane `/envoy` v1.32.4 bindings, via the live-probe precedent (`reference_docker_probe_bridge_network`):

- **D-PT1** *(SPEC-BLOCKING — the semantics battery)* — for a FLAT health-checked cluster with a CONFIGURED threshold: (i) boundary strictness at exactly-the-threshold (probe threshold 60% with 3/5 healthy = exactly 60%: panic or not? envoy-go implements strict `<`); (ii) the in-panic pick distribution per leaf policy (all hosts via the policy's own algorithm — confirm with per-host tallies); (iii) `lb_healthy_panic` increment semantics — once per pick? which layer in a locality-weighted cluster? (pins the double-increment fix shape: NIL-health flat child per the `priorityLB` precedent vs a panic-disabled view — and whether `0097`'s panic arm can assert `== N` exactly); (iv) the comparison shape at fractional boundaries (`reference_percent_cap_cross_multiply` — probe a non-multiple boundary).
- **D-PT2** *(SPEC-BLOCKING — presence semantics)* — explicit `{value: 0}` (documented disable) vs ABSENT (50% default) vs explicit `{value: 50}`: confirm envoy-go's `parsePanicThreshold` nil-check matches the reference's presence handling exactly (the D-LW-OPF0 presence-vs-zero discipline).
- **D-PT3** *(validation parity)* — boot-probe out-of-range values (`>100`, negative) against the reference; mirror any reject byte-stable (ADR-0080) or accept identically; confirm clean `go mod tidy` (ZERO new dep).
- **D-PT4** *(SPEC-BLOCKING — the retrofit gate)* — the locality-scoped child-panic probe: a locality-weighted 2-locality cluster, ONE locality degraded below 50% (its own sub-slice fraction) while the CLUSTER-WIDE fraction stays ≥ the threshold — do that locality's unhealthy hosts receive traffic in the reference (child-local flatten) or zero (no local panic — the AMEND-P1 per-tier analog)? Retrofit `locality.go` per the finding. SUB-QUESTION (information-only): the same shape for a `lb_subset_config` cluster (one subset degraded) — is per-subset panic the reference's actual behavior (anticipated yes → envoy-go's shared-health subset children already faithful → no change)?
- **D-PT5** — the stat-surface delta (anticipated ZERO): scrape `/stats` under a panicking cluster; confirm no dedicated panic gauge/namespace exists; pin `0097`'s exact `StatsAsserter` counter set (incl. whether the reference's `lb_healthy_panic` supports an exact-value cross-side assertion given D-PT1(iii)).
- **D-PT6** — the LoC/task envelope re-check (§1.4; anticipated ~120–250 LoC / ~8–10 tasks, far under the ADR-0045 gate) + fuzzer disposition (anticipated NONE — no wire-decode surface).

---

## 11. Prior-phase lessons applied

- **Verify the proto surface in the module cache at BRAINSTORM time** (phases 33..53) — applied: `envoy.type.v3.Percent` (`Value float64`) + the `CommonLbConfig` field set re-verified this session (§2.1/§3.3).
- **Evidence-before-fix / live-probe-over-assumption** (every LB phase's discipline; phase 53's AMEND-P1 refutation as the cautionary exemplar) — applied twice: the retrofit is D-PT4-gated (NOT assumed to transfer from the per-tier finding), and the double-increment fix SHAPE waits for D-PT1(iii)'s pinned increment semantics.
- **Percent-cap comparisons cross-multiply** (`reference_percent_cap_cross_multiply`) — applied: D-PT1(iv) probes a non-multiple boundary before the comparison shape is pinned.
- **Presence-vs-zero wrapper discipline** (D-LW-OPF0, phases 52/53) — applied: D-PT2 treats `{value: 0}` vs absent as distinct probed cases.
- **Hard assertions over bands** (the phase-53 Q4 lesson; `reference_differential_band_sigma_margin` avoided by design) — applied: `0097` is all 0-vs-nonzero/exact-count assertions.
- **RR offset randomized** (`reference_round_robin_offset_randomized`) — applied: `0097` asserts offset-invariant per-host `> 0`/`== 0`/sums, never host identity or sequence.
- **Health-check propagation warmup** (`reference_health_check_propagation_warmup`) + the phase-53 `warmupStable` 10→60 finding — applied: `0097` polls `membership_healthy` convergence + the warmup gate, sized per the 0096 lesson.
- **Admin-interface wire-name collision** (`reference_admin_interface_wire_name_collision`) — noted: `0097`'s two clusters have DISTINCT names, so per-name stat accessors disambiguate naturally.
- **Differential break protocol `-count=1`** + **asserter dispatch** + **run-selector** + **fixture workload-constant desync** + **one-dir-one-branch** — applied per §6.1.
- **Per-task gofmt + golangci-lint** (`feedback_pertask_gofmt_lint`); **subagents commit local-only** (`feedback_subagents_no_push`); **controller squashes + pushes at stage-close** (`feedback_push_to_origin`); **worktrees** (`feedback_git_worktrees`); **subagent-driven IMPL** (`feedback_execution_style`); **path-pinning for worktree subagents** (`feedback_subagent_worktree_path_targeting`/`_detach`) — applied at SPEC/PLAN/IMPL.
- **Full-suite `-race` after a background mutator** (`reference_full_suite_race_after_background_mutator`) — noted for the IMPL six-gate (health-check goroutines are background mutators).

---

## 12. Section closeout

This brainstorm settles: (Q-family/Q0) phase 54 = **`healthy_panic_threshold` as an independent construct** (`Cluster.CommonLbConfig.healthy_panic_threshold`, `envoy.type.v3.Percent` — VERIFIED in the v1.32.4 module cache), the EIGHTH and FINAL Load-balancing-family row — **the family CLOSES at phase-done** (the fourth family to close). (Q1) scope = **prove + harden**: the first-ever proof of the configured threshold (no test of any kind exercises it today) + the semantics-pin battery (D-PT1/D-PT2) + validation parity (D-PT3) + TWO evidence-gated corrections — the `lb_healthy_panic` double-increment fix (`REVIEW_FINDINGS.md` deferred finding, confirmed live in `locality.go` this session; fix shape pinned by D-PT1(iii)) and the `locality.go` child-local-panic retrofit (the phase-52 boundary / phase-53 D-P-RETROFIT deferral, gated on D-PT4's locality-scoped probe, structural template `tierHealth`). Declined: `membership_healthy`-vs-ejections, `fail_traffic_on_panic`. (Q2) differential = **the two-cluster discriminator** `0097-lb-panic-threshold` (ONE dir, TWO clusters differing only in threshold — 80% vs absent/50% — SAME 2-of-5 degradation, hard 0-vs-nonzero + exact-count assertions, no bands). Self-answered: ZERO new `Pick` parameters/packages/deps/BackendKind/fuzzers; stat surface anticipated **1200 → 1200** (the family's FIFTH zero-delta phase; a VALUE correction on the existing `lb_healthy_panic`, not a name change); multi-tier-priority inertness documented as confirmed-correct (AMEND-P1), NOT a gap; NO pre-authorized split (anticipated ~120–250 LoC / ~8–10 tasks — the family's smallest phase). Anticipated **1 ADR** — **ADR-0271**; §Context at SPEC, body at IMPL per ADR-0044; DECISIONS tail STAYS ADR-0270 at this brainstorm. Anticipated moves at the phase-54 IMPL: fixtures 98 → 99 (`0097-lb-panic-threshold`), DECISIONS tail → ADR-0271, stat surface 1200 → 1200, fuzzers 52 → 52, BackendKind tail 38 → 38, **Load-balancing family → CLOSED**. ALL counts UNCHANGED at this brainstorm commit (re-verified fresh this session against post-maintenance-pass HEAD `0e9cc680`).

The next session authors `docs/envoy-go/phases/54-load-balancer-panic-threshold/SPEC.md` (`superpowers:writing-plans` scoped to SPEC authoring — the phase 36/37/38/52/53 precedent), executing the §10 D-PT1..D-PT6 empirical pins IN-SESSION against the contrib reference Envoy per ADR-0004/ADR-0227 (`reference_docker_probe_bridge_network`), anchoring the ADR-0271 §Context draft, and confirming the single-flat-row envelope. Per ADR-0106, row 54 registers `in-progress` (flat family row, no pre-authorized split) at this BRAINSTORM-DONE commit; it flips `in-progress → done` at the phase-54 IMPL six-gate — at which point the **Load-balancing family CLOSES**.
