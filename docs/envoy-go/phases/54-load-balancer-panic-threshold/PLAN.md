# Phase 54 Implementation Plan — `healthy_panic_threshold` as an independent construct (`Cluster.CommonLbConfig.healthy_panic_threshold`) — the EIGHTH and FINAL Load-balancing-family row (the family CLOSES at phase-done)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn `Cluster.CommonLbConfig.healthy_panic_threshold` (`envoy.type.v3.Percent`, default 50%) from a **consumed-but-never-proven** field into a proven, hardened, independently-observable construct. Deliver: the first-ever differential proof (`0097-lb-panic-threshold`, three arms) + FOUR evidence-driven corrections the SPEC's live probes pinned — the integer-truncation comparison-shape fix (AMEND-PT1, the marquee finding), the `lb_healthy_panic` double-increment fix (AMEND-PT2), the `locality.go` child-local-panic retrofit (AMEND-PT3), and out-of-range validation parity (AMEND-PT4).

**Architecture:** ZERO new packages, ZERO new files (except the `0097` fixture dir), ZERO new `Pick` parameters, ZERO new go.mod deps (SPEC §4/§11). All production deltas live in the EXISTING `internal/cluster` package: (a) the panic comparison shape (`health.go` `inPanic`/`parsePanicThreshold` + the `clusterHealth.panicThresholdPercent` stored-type change, integer percent); (b) the removed redundant outer `panicInc()` (`locality.go:144`); (c) the per-locality child health-view retrofit (`locality.go` constructors + the `manager.go` locality closure, reusing the relocated `tierHealth` panic-disabled view primitive + the health-parameterized `healthLeafFactory`); and (d) a new standalone out-of-range parse-reject (byte-stable per ADR-0080). The stat surface stays **1200 → 1200** (a VALUE correction on the existing `lb_healthy_panic` counter, not a name change) — the family's FIFTH zero-stat-delta phase.

**Tech Stack:** Go 1.26.x; golangci-lint 1.64.8 (ADR-0009); go-control-plane `/envoy` v1.32.4 (ADR-0008 — `envoy.type.v3.Percent` is already reachable via the existing `parsePanicThreshold` consumption; `go mod tidy -diff` stays EMPTY, re-verified live at Task 1); reference Envoy `envoyproxy/envoy:contrib-v1.37.2` (ADR-0227 — already live-probed at the SPEC, §11 — NO new probes at the PLAN or IMPL). Reuses `internal/cluster/`: the `clusterHealth`/`hostHealth` model (`health.go`, ADR-0242/0243); `tierHealth` (`priority.go` → relocated to `health.go`, ADR-0270's panic-disabled-view primitive — its SECOND consumer); the `buildLeafLB` factory closures; and the phase-39/53 differential harness (`toggleResponder` + `pollMembershipHealthy` + `warmupUntilStable` + `scrapeStats`, the `0096-lb-priority` driver reused as the direct template). ZERO new packages, ZERO new go.mod deps.

## Global Constraints

- **The panic comparison shape (AMEND-PT1, SPEC §3.1) — MUST be implemented exactly, not approximated.** The reference panics iff `100.0 × healthy / total < floor(threshold_percent)` (it integer-truncates the configured threshold to a whole percent before comparing). envoy-go mirrors this bit-for-bit by (a) FLOORING the configured value to an integer percent in `parsePanicThreshold`, and (b) comparing via an INTEGER CROSS-MULTIPLY in `inPanic`: panic iff **`100 * availableCount < panicThresholdPercent * total`** (strict `<`, ULP-safe — no float division; `reference_percent_cap_cross_multiply` / `reference_percent_threshold_integer_truncation`). `total == 0 → not panic`. Agrees with the old float form at every integer threshold (50/60/80) any prior test used; corrects the non-integer gap (`60.9%` at `60%` healthy → `300 < 300` false → NO panic, where the pre-fix `0.60 < 0.609` wrongly panicked).
- **The stored representation (D-PT-STORE, SPEC §12).** `clusterHealth.panicThreshold float64` (a fraction) becomes `clusterHealth.panicThresholdPercent int` (a floored integer percent in `[0,100]`). `newClusterHealth`'s second parameter and `parsePanicThreshold`'s return type change from `float64` to `int` accordingly. `int` (not `uint32`) — so the cross-multiply is zero-conversion arithmetic against `availableCount() int` and `len() int`. Range `[0,100]` is guaranteed by the §6 reject (below), so `inPanic` needs no clamp.
- **`parsePanicThreshold` semantics (AMEND-PT1/AMEND-PT5, SPEC §3.1/§11.2):** absent → `50`; else `int(math.Floor(p.GetValue()))`. `{value:0}` disables (floor 0; `100·avail < 0` never true); absent defaults to 50%.
- **The out-of-range reject (AMEND-PT4, SPEC §3.4/§6; ADR-0080) — placement is NOT a free choice (D-PT-REJECT-PLACEMENT).** A new STANDALONE, UNCONDITIONAL validation early in `buildCluster` (before the LB switch), fired for ANY cluster carrying the field — including a PLAIN cluster (no health_checks / no outlier_detection / no wrapper) that never reaches any `parsePanicThreshold` call site (all three are health-guarded). Condition: `healthy_panic_threshold` present AND (`value < 0` OR `value > 100`); exactly `0` and `100` accepted. Byte-stable message (ADR-0080), in the `cluster: %q: …` family: `cluster: %q: common_lb_config.healthy_panic_threshold: value must be inside range [0, 100]`. `NaN` is config-unreachable (`protojson` rejects it for a `double` field).
- **The `lb_healthy_panic` double-increment fix (AMEND-PT2, SPEC §3.2):** DELETE the redundant `lw.health.panicInc()` at `locality.go:144`, keeping the delegation to `lw.flat.Pick`. The flat child (which KEEPS the shared, panic-enabled health) does the single `panicInc()` via its own `panicGate`. Net: exactly ONE increment per pick (`2N → N`). Do NOT nil the flat child (would suppress the increment AND change the zero-total-weight fallback).
- **The `locality.go` child-local-panic retrofit (AMEND-PT3, SPEC §3.3):** each per-locality child is built against a PANIC-DISABLED `tierHealth(health)` view so a single degraded locality never internally flattens (its unhealthy hosts stay at zero); the FLAT fallback child KEEPS the shared, panic-ENABLED health (it alone detects cluster-wide panic + does the increment). This ENTAILS a health-parameterized factory signature change to `newLocalityWeightedLB`/`newLocalityWeightedLBWithRNG` + the `manager.go` locality closure (the phase-53 `priorityLeafFactory`/D-P-FACTORY precedent) — load-bearing, NOT optional.
- **`subsetLB` needs NO change (AMEND-PT3 sub-question / D-PT4, the crux asymmetry).** A subset is its own LB host-set → per-subset panic IS the reference's behavior → the shared-health subset children are already faithful. `subset.go` is UNTOUCHED this phase (a passing regression test locks in that a degraded subset still flattens).
- **`loadBalancer.Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error)` (`loadbalancer.go:22`) stays BYTE-FOR-BYTE unchanged** — ZERO new pick-input (SPEC §3.5/§11.7). No wrapper, no ctx carry, no `Endpoint` dimension, no `Cluster` exported-surface change.
- **Stat surface stays 1200 (+0)** (SPEC §7/§11.5) — the family's FIFTH zero-stat-delta phase; a VALUE correction on `lb_healthy_panic`, adding NO name. NO new `registerClusterMetrics` handle.
- **Counts at IMPL:** fixtures **98 → 99** (`0097-lb-panic-threshold`, a single dir); fuzzers **52 (+0)** (no wire-decode surface); BackendKind tail **38 (+0)** (reuses the toggle-responder pattern); DECISIONS tail **ADR-0270 → ADR-0271** (next-free ADR-0272).
- Every task runs `gofmt -l` + `golangci-lint run` on the touched packages, PER TASK (not just a final gate) — `feedback_pertask_gofmt_lint` — plus `go vet` + `go build ./...`.
- Subagents commit LOCAL-ONLY (`feedback_subagents_no_push`); the controller verifies each commit, re-runs gates on the frozen HEAD, does the deliberate-break verification ITSELF, and squash-merges + pushes at stage-close (`feedback_subagent_autocommit_claudemd` · `feedback_push_to_origin`). Every path below is repo-root-relative; PROGRESS.md is pinned at `docs/envoy-go/phases/54-load-balancer-panic-threshold/PROGRESS.md` (`feedback_subagent_worktree_path_targeting`).
- Every new differential assertion is proven live via a deliberate break run with `-count=1` (`reference_differential_break_protocol_count1`); targeted runs use `-run 'TestDifferential/0097'`, NEVER `-run '0097'` (`reference_differential_run_selector`). Every new UNIT assertion (AMEND-PT2/PT3) is likewise proven live via a scratch-revert that confirms it BITES, then restored (`reference_differential_asserter_dispatch` generalized — "always prove a new assertion is live").
- `0097`'s three arms assert a mix of HARD 0-vs-nonzero (`lb_healthy_panic == N_A` / `== 0`; degraded-host `> 0` / `== 0`) and offset-invariant counts (`reference_round_robin_offset_randomized` — never host identity/sequence). Workload constants are named + internally consistency-tested (`reference_fixture_workload_constant_desync`).

---

## Source-of-truth references

- **SPEC:** `docs/envoy-go/phases/54-load-balancer-panic-threshold/SPEC.md` — AUTHORITATIVE. §1.1 AMEND-PT1..PT6 (the four corrections + the two "already-matches" confirmations); §3.0 the split disposition (single flat row, NO escape valve); §3.1 the integer-truncation comparison-shape fix (the exact as-built line citations + the floor + integer-cross-multiply mirror); §3.2 the double-increment fix; §3.3 the locality retrofit (incl. the load-bearing factory-signature change); §3.4 the out-of-range reject + the STANDALONE-placement requirement; §3.5/§3.6 the seam-reuse + no-new-composition-reject confirmations; §5 the single-field proto roster; §6 the ONE-reject roster + wording discipline; §7 the confirmed-zero stat delta; §8 the `0097` three-arm differential design (§8.1) + the deliberate breaks + the explicit NO-locality/subset-differential-dir disposition (§8.2); §9 the BEHAVIOR_CONTRACT delta shape; §10 the ~9–11-task indicative spine (this PLAN maps it, see the decomposition table); §11 the D-PT1..D-PT6 empirical pins (ALL RESOLVED at the SPEC, none deferred to this PLAN) — §11.1's exact scenario data ((i)/(ii)/(iii)/(iv)) is THE decisive data this PLAN's unit tests reproduce; §12 the FOUR PLAN-level D-questions this PLAN resolves below (D-PT-STORE / D-PT-REJECT-PLACEMENT / D-PT-TIERHEALTH-HOME / D-PT-UNIT-VS-DIFF); §13 the ADR-0271 §Context DRAFT; §14 the exit counts.
- **BRAINSTORM:** `docs/envoy-go/phases/54-load-balancer-panic-threshold/BRAINSTORM.md` — the charter. The BRAINSTORM's D-PT1 comparison-shape anticipation (§2.5 iv, "confirm the comparison shape at fractional boundaries") was REFINED at the SPEC into AMEND-PT1 (the reference integer-TRUNCATES — a genuine divergence, not just a boundary confirmation); its anticipated double-increment fix (a nil-health flat child) was REFINED at AMEND-PT2 (remove the outer increment instead). This PLAN follows the SPEC's corrected designs throughout.
- **The phase-53 PLAN** (`docs/envoy-go/phases/53-load-balancer-priority/PLAN.md`) — the DIRECT STRUCTURAL AND STYLE TEMPLATE this PLAN mirrors (Source-of-truth references / Project conventions / D-question resolutions / File Structure / per-task Files+Interfaces+Steps / a final Verification + ADR-0045 re-check). Its `tierHealth` (`priority.go`, ADR-0270) + `priorityLeafFactory`/D-P-FACTORY health-parameterized-factory pattern are DIRECTLY load-bearing for AMEND-PT3's locality retrofit — this phase reuses (and relocates) both.
- **As-built anchors** (verified fresh against master tip `d48594d7` while authoring this PLAN; RE-CONFIRM at Task 1 — line numbers shift on the IMPL-session tip):
  - `internal/cluster/health.go:70-80` — the `clusterHealth` struct (`panicThreshold float64` at `:72` → becomes `panicThresholdPercent int`, Task 2) + `:82-92` `newClusterHealth(endpoints []Endpoint, panicThreshold float64)` (second param retype, Task 2) + `:180-188` `inPanic` (integer cross-multiply, Task 2) + `:197-202` `panicInc` (UNCHANGED) + `:204-220` `panicGate` (UNCHANGED) + `:483-491` `parsePanicThreshold` (floor + int return, Task 2). NO `"math"` import today (Task 2 adds it).
  - `internal/cluster/locality.go:67-73` `newLocalityWeightedLB(endpoints, health, opf, hasOPF, factory leafFactory)` + `:85-115` `newLocalityWeightedLBWithRNG(..., factory leafFactory, rng)` (BOTH gain the `healthLeafFactory` signature, Task 5; per-locality children via `factory(members, tierHealth(health))`, flat via `factory(endpoints, health)`) + `:103` `child, err := factory(members)` + `:109` `flat, err := factory(endpoints)` (both re-wired, Task 5) + `:142-146` `Pick`'s panic branch (the outer `lw.health.panicInc()` at `:144` DELETED, Task 6).
  - `internal/cluster/priority.go:46-48` `tierHealth(shared *clusterHealth) *clusterHealth` (RELOCATED to `health.go`, Task 5 — the `panicThreshold: 0` literal becomes `panicThresholdPercent: 0` at Task 2, then the whole function moves at Task 5) + `:87-102` `priorityLeafFactory` (RENAMED `healthLeafFactory` + relocated to `loadbalancer.go`, Task 5) + `:131`/`:154` its two consumers (updated to the new name, Task 5) + `:154-187` `newPriorityLBWithRNG` (`tierHealth(health)` call still compiles after the relocation — same package; NO logic change).
  - `internal/cluster/loadbalancer.go:16-23` (`type loadBalancer interface { Pick(...) }` — stays BYTE-FOR-BYTE unchanged) + `:35-61` `roundRobin` (its `rr.health.panicGate(rr.endpoints)` at `:49` is the leaf convention the AMEND-PT2 unit test relies on — a real `roundRobin` child bound to shared health increments the counter exactly once). `healthLeafFactory` is added here at Task 5.
  - `internal/cluster/manager.go:402` `buildCluster(c *clusterv3.Cluster, idx int, baseDir string)` + `:429-436` (`la := c.GetLoadAssignment()` / `extractEndpoints` — the STANDALONE reject is inserted immediately after, Task 4) + `:459-462` the FIRST `parsePanicThreshold` site (health-guarded; type flows through UNCHANGED) + `:476-481` the `lb_subset_config` wrap (`func(sub []Endpoint) { return buildLeafLB(c, name, sub, health) }` — UNCHANGED, subset keeps shared health) + `:486-520` the phase-52 locality wrap (the closure at `:513-515` gains the `h *clusterHealth` param, Task 5; the `parsePanicThreshold` site at `:501` unchanged) + `:521-554` the phase-53 priority wrap (the `:536` `parsePanicThreshold` site + the `:547-549` closure — UNCHANGED reference model for the locality closure change).
  - `internal/cluster/subset.go:129-131` `type leafFactory func(sub []Endpoint) (loadBalancer, error)` + `:158` `newSubsetLB(endpoints, cfg, factory leafFactory)` — UNCHANGED this phase (the asymmetry: subset stays on the health-CLOSING `leafFactory`).
- **Differential harness template:** `test/fixtures/0096-lb-priority/driver/driver.go` — the `toggleResponder`/`newToggleResponder`/`pollMembershipHealthy`/`warmupUntilStable`/`scrapeStats`/`classifyBody` harness Task 7 reuses (adapted from 2 tiers to 3 health-checked clusters). `test/differential/fixture/fixture.go` — the `Driver`/`StatsAsserter`/`TB`/`BackendKind` interfaces (`HTTPEcho BackendKind = 1`; tail `H2GoawayResponder BackendKind = 38`, UNCHANGED). `test/differential/runner_test.go:123` — the blank-import aggregator (the `0096` line) (Task 7 appends the `0097` driver import).

## Project conventions honored throughout (memory + ADRs)

- `feedback_execution_style` / `feedback_git_worktrees` — subagent-driven execution; this PLAN was authored in worktree `.worktrees/phase-54-plan`; the IMPL runs in its own fresh worktree.
- `feedback_subagents_no_push` / `feedback_subagent_worktree_path_targeting` / `feedback_subagent_worktree_detach` — subagents commit LOCAL-ONLY; PROGRESS.md is pinned at `docs/envoy-go/phases/54-load-balancer-panic-threshold/PROGRESS.md`; deliberate-break checks that detach HEAD get a GIT HYGIENE restore (no checkout-sha/amend) + the controller re-verifies the branch each task.
- `feedback_pertask_gofmt_lint` — every task's own gate runs `gofmt -l` + `golangci-lint run` on touched packages.
- `reference_percent_cap_cross_multiply` / `reference_percent_threshold_integer_truncation` — the AMEND-PT1 fix uses the ULP-safe integer cross-multiply (`100*avail < percent*total`), floored-integer threshold; unit-tested at a NON-multiple boundary (`60.9%`/`60%`, `66.7%`/`2-of-3`) where the old float form diverged, not just at round caps.
- `reference_health_check_propagation_warmup` — `0097` polls `membership_healthy` to 3 per cluster THEN runs the warmup-until-K-consecutive-200s gate before measuring (the phase-39/53 pattern; size `warmupStable` per the phase-53 10→60 lesson).
- `reference_membership_total_vs_healthy_gauge` — all three `0097` clusters ARE health-checked, so `membership_healthy` exists both sides (used for the poll-to-converge gate + a deterministic `== 3` per-cluster assertion).
- `reference_round_robin_offset_randomized` — `0097`'s in-panic distribution assertion is offset-invariant (all 5 hosts get `> 0`), NEVER host identity/sequence; the reference randomizes the RR initial offset.
- `reference_differential_break_protocol_count1` / `reference_differential_run_selector` / `reference_fixture_workload_constant_desync` — Task 8's deliberate breaks run `-count=1`, `-run 'TestDifferential/0097'`; all workload constants are named + `TestConstants`-guarded.
- `reference_differential_asserter_dispatch` — `0097`'s cross-side stats prong uses `StatsAsserter` (in-band, holding both admin addrs), NOT `SubjectAsserter`; every new UNIT assertion (AMEND-PT2/PT3) is scratch-revert-proven live.
- `reference_differential_fixture_dispatch_constraint` — `0097` is ONE dir / ONE runner branch (three arms in one boot, SPEC §8.1); the out-of-range reject is a SUBJECT-SIDE UNIT test in `manager_test.go`, NOT a separate cross-side boot-reject dir (SPEC §6 — a scalar range check; `reference_sibling_reject_test_needs_real_typeurl` does NOT apply).
- `reference_docker_probe_bridge_network` — `0097`'s driver-owned toggle responders bind `0.0.0.0:0` and are addressed via `host.docker.internal` on the reference side (the `0095`/`0096` addressing precedent).
- `reference_full_suite_race_after_background_mutator` — Task 9's `-race` gate runs the FULL package (health-check goroutines are background mutators; a `-run`-subset `-race` misses it).
- ADR-0271 (the healthy-panic-threshold construct: proof + the four corrections; §Context DRAFTED at SPEC §13, §Decision/§Consequences land at Task 10) — the SOLE anticipated ADR (SPEC §11.7 confirms zero seam change). ADR-0242/0243 (the health model + the health-aware LB pick) — REUSED unchanged; ADR-0270 (`tierHealth`, the panic-disabled view) — its primitive RELOCATED + REUSED as its second consumer. ADR-0024 (per-cluster LB-state scope) — UNAMENDED. ADR-0080 (byte-stable reject text). ADR-0052 (the atomic six-gate completion bundle). ADR-0106 (flat family row, no parent rollup; the split-phase-completion precedent, `reference_roadmap_split_phase_row_done` — N/A here, a single flat row). ADR-0045 (the split-gate — FINAL re-check at the end of this PLAN). ADR-0044 (ADR §Decision/§Consequences land at IMPL).

## D-question resolutions (SPEC §12)

- **D-PT-STORE (the stored representation of the floored threshold)** — RESOLVED: an INTEGER field. `clusterHealth.panicThreshold float64` (a fraction) → `clusterHealth.panicThresholdPercent int` (a floored integer percent in `[0,100]`). `newClusterHealth`'s second parameter and `parsePanicThreshold`'s return type retype `float64 → int`. Chosen `int` (not `uint32`) so `inPanic`'s cross-multiply — `100*ch.availableCount(eps) < ch.panicThresholdPercent*total` — is zero-conversion arithmetic against `availableCount() int` and `len() int` (SPEC §12's own leaning: "prefer an integer field for clarity and to make the cross-multiply obviously ULP-safe"). Overflow-safe: `100 × availableCount ≤ ~1e6` and `panicThresholdPercent × total ≤ 100 × total ≤ ~1e6` for any realistic host count, far below `int`'s range on every supported platform.
- **D-PT-REJECT-PLACEMENT (where the out-of-range reject fires)** — RESOLVED: a STANDALONE helper `validatePanicThresholdRange(c *clusterv3.Cluster, name string) error` (in `health.go`, next to `parsePanicThreshold`), called UNCONDITIONALLY from `buildCluster` immediately after `extractEndpoints` succeeds (before the health parse + the LB switch). This is REQUIRED, not free latitude: all three `parsePanicThreshold(c)` sites (`manager.go:461/501/536`) are health-guarded (`:461` needs `hcSpecs`/`outlierCfg`; `:501` a `locality_weighted_lb_config`; `:536` a multi-tier `priority`), so folding the range check there would silently MISS a plain cluster (no health_checks, no wrapper) that still carries — and the reference still rejects — an out-of-range value. `parsePanicThreshold` ALSO floors (for the clusters that DO build health); the two concerns are separate. The standalone check runs FIRST and unconditionally, so by the time `parsePanicThreshold` runs the value is guaranteed in `[0,100]` and the floor needs no clamp.
- **D-PT-TIERHEALTH-HOME (`tierHealth`'s home + the factory-type reuse-vs-local-variant)** — RESOLVED in two parts:
  1. **`tierHealth` RELOCATES to `health.go`** (from `priority.go`). It constructs a `clusterHealth` view over the shared per-host `states` map, so `health.go` (which owns `clusterHealth`/`newClusterHealth`) is its natural home now that it has a SECOND consumer (`locality.go`'s retrofit) in a different file. Pure relocation — no logic change (the `panicThreshold: 0` literal already became `panicThresholdPercent: 0` at Task 2).
  2. **`priorityLeafFactory` RENAMES to `healthLeafFactory` and RELOCATES to `loadbalancer.go`** (the neutral LB-construction home, next to the `loadBalancer` interface). Once `localityWeightedLB` ALSO consumes it, a `priority`-specific name on a `localityWeightedLB` parameter would be actively misleading; a neutral name in a neutral home is the DRY-correct outcome (SPEC §12 leaves "the factory-type reuse-vs-local-variant" explicitly open for the PLAN). Both `localityWeightedLB` and `priorityLB` use `healthLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)`. `subset.go`'s `leafFactory func(sub []Endpoint) (loadBalancer, error)` stays COMPLETELY UNTOUCHED (subset children keep shared health via a closing factory — the crux asymmetry). The health-parameterized signature change to `newLocalityWeightedLB`/`newLocalityWeightedLBWithRNG` + the `manager.go:513-515` closure is LOAD-BEARING (not optional).
- **D-PT-UNIT-VS-DIFF (unit vs differential coverage of AMEND-PT2/PT3)** — RESOLVED: the double-increment fix (AMEND-PT2) and the locality retrofit (AMEND-PT3) stay UNIT-proven against the D-PT1(iii)/D-PT4 pinned reference values (delta == N; degraded-locality unhealthy hosts == 0). NO locality/subset differential dir is added — `0097` uses plain, non-wrapped clusters where the panic path is single-increment already (honoring the SPEC §8.2 / Q2 plain-two-cluster-discriminator scope). Each new unit assertion is scratch-revert-proven live (Tasks 5/6).

### Decomposition note (11 tasks; SPEC §10's ~9–11-task spine, with a deliberate 5↔6 reorder)

| SPEC §10 indicative task | This plan | Note |
|---|---|---|
| 1 baselines + PROGRESS | Task 1 | 1:1 |
| 2 AMEND-PT1 floor + integer cross-multiply (+ D-PT-STORE type change + call-site sweep) | Task 2 | 1:1 |
| 3 parse-path unit hardening (presence semantics through the wired path) | Task 3 | 1:1 |
| 4 AMEND-PT4 out-of-range reject | Task 4 | 1:1 (D-PT-REJECT-PLACEMENT resolved: standalone) |
| 6 AMEND-PT3 locality retrofit | **Task 5** | **REORDERED before AMEND-PT2** — the factory-signature change lands ONCE here, so the AMEND-PT2 unit test (Task 6) is written directly against the final `healthLeafFactory` signature (avoids re-editing a just-written test). SPEC §10 explicitly permits reordering ("Tasks 5/6 may fold or split at the PLAN's discretion"). |
| 5 AMEND-PT2 double-increment fix | **Task 6** | REORDERED after AMEND-PT3; the new test uses the Task-5 factory signature + real `roundRobin` flat child. |
| 7 `0097` fixture + driver | Task 7 | 1:1 |
| 8 `0097` deliberate breaks + flake-soak + `-race` | Task 8 | 1:1 |
| 9 full 99-dir differential + six gates | Task 9 | 1:1 |
| 10 ADR-0271 body + BEHAVIOR_CONTRACT delta | Task 10 | 1:1 |
| 11 completion bundle (STATE/ROADMAP, row-54 done, family CLOSE, counts) | Task 11 | 1:1 (by the CONTROLLER at stage-close, the phase-52/53 Task-11 precedent) |

---

## File Structure

| File | Disposition | Responsibility |
|---|---|---|
| `internal/cluster/health.go` | MODIFY (Tasks 2, 4, 5) | `clusterHealth.panicThresholdPercent int` + `newClusterHealth`/`inPanic`/`parsePanicThreshold` retype+floor+cross-multiply (Task 2); the standalone `validatePanicThresholdRange` (Task 4); the relocated `tierHealth` (Task 5). Adds the `"math"` import (Task 2). |
| `internal/cluster/health_test.go` | MODIFY (Tasks 2, 3) | The AMEND-PT1 floor + integer-cross-multiply tests + the `0.5 → 50` sweep + the retyped `TestParsePanicThreshold` (Task 2); the presence-semantics-through-`panicGate` tests (Task 3). |
| `internal/cluster/loadbalancer.go` | MODIFY (Task 5) | `type healthLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)` (renamed+relocated from `priority.go`). |
| `internal/cluster/locality.go` | MODIFY (Tasks 5, 6) | The `healthLeafFactory` signature change + per-locality `tierHealth(health)` children + flat-keeps-shared-health (Task 5); the removed outer `panicInc()` at `Pick` (Task 6). |
| `internal/cluster/locality_test.go` | MODIFY (Tasks 2, 5, 6) | The `0.5 → 50` sweep (Task 2); the factory-literal signature sweep + `trackingFactory` retype + the degraded-locality no-local-panic test (Task 5); the double-increment delta==N test (Task 6). |
| `internal/cluster/priority.go` | MODIFY (Tasks 2, 5) | The `tierHealth` literal `panicThreshold: 0 → panicThresholdPercent: 0` (Task 2); REMOVE `tierHealth` (moved to `health.go`) + rename `priorityLeafFactory → healthLeafFactory` references (Task 5). |
| `internal/cluster/priority_test.go` | MODIFY (Tasks 2, 5) | The `0.5 → 50` sweep + `.panicThreshold → .panicThresholdPercent` (Task 2); the `priorityLeafFactory → healthLeafFactory` references (Task 5). |
| `internal/cluster/manager.go` | MODIFY (Tasks 4, 5) | The `validatePanicThresholdRange` call early in `buildCluster` (Task 4); the locality closure signature change `func(sub []Endpoint) → func(sub []Endpoint, h *clusterHealth)` (Task 5). |
| `internal/cluster/manager_test.go` | MODIFY (Task 4) | The out-of-range accept/reject matrix (incl. the plain-cluster case). |
| `internal/cluster/subset_test.go` | MODIFY (Task 5) | The degraded-subset-still-flattens regression (proving subsetLB unchanged). |
| Other `internal/cluster/*_test.go` (`loadbalancer_test.go`, `leastrequest_test.go`, `random_test.go`, `maglev_test.go`, `outlier_test.go`, `ringhash_test.go`) | MODIFY (Task 2) | The mechanical `newClusterHealth(…, 0.5) → (…, 50)` sweep. |
| `test/fixtures/0097-lb-panic-threshold/driver/driver.go` | **CREATE** (Tasks 7) | The 3-cluster × 5-host topology, the driver-owned toggle-responder degradation harness, the bootstrap builders, `AssertStats` (all three arms in-band). |
| `test/fixtures/0097-lb-panic-threshold/driver/driver_test.go` | **CREATE** (Task 7) | `classifyBody` parse tests + the `TestConstants` workload-constant pin. |
| `test/fixtures/0097-lb-panic-threshold/README.md` + `expectations.yaml` | **CREATE** (Task 7); MODIFY (Task 8) | The fixture design doc + differential expectations; Task 8 appends the break-protocol record. |
| `test/differential/runner_test.go` | MODIFY (Task 7) | Append the `_ "…/0097-lb-panic-threshold/driver"` blank-import. |
| `docs/envoy-go/phases/54-load-balancer-panic-threshold/PROGRESS.md` | **CREATE** (Task 1) | The IMPL progress ledger. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY (Task 10) | The panic-construct behavior delta (SPEC §9). |
| `docs/envoy-go/DECISIONS.md` | MODIFY (Task 10) | The full ADR-0271 entry (§Context + §Decision + §Consequences). |
| `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md` | MODIFY (Task 11, by the CONTROLLER at stage-close) | Active-phase + counts advance; ROADMAP row 54 `in-progress → done`; the Load-balancing family CLOSES. |

---

## Task 1: Baselines/anchors gate + PROGRESS.md

**Goal:** Re-confirm every count anchor against the IMPL-session tip BEFORE touching code, re-pin the as-built line anchors, and create the progress ledger. No production code.

**Files:**
- Create: `docs/envoy-go/phases/54-load-balancer-panic-threshold/PROGRESS.md`

- [ ] **Step 1: Confirm the count anchors via the canonical recipes**

```bash
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                          # expect 98
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | sort | tail -1                 # expect test/fixtures/0096-lb-priority
grep -n "BackendKind = " test/differential/fixture/fixture.go | tail -1    # expect H2GoawayResponder BackendKind = 38
grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l   # expect 52 (reference_fuzzer_count_docs_drift)
grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -1            # expect tail ADR-0270 (next-free ADR-0271)
grep -c "1200" docs/envoy-go/BEHAVIOR_CONTRACT.md                          # the stat-surface DOC count, not a golden test
go build ./... && echo BUILD_OK
go mod tidy -diff && echo TIDY_EMPTY                                       # expect exit 0, empty (ZERO new dep)
```
Expected: fixtures **98** (tail `0096-lb-priority`); BackendKind tail **38**; fuzzers **52**; DECISIONS tail entry **ADR-0270**; build clean; `go mod tidy -diff` empty. (NOTE: `grep -oE 'ADR-0[0-9]+' … | tail` will surface `ADR-0271` as a forward-reference/DRAFT string inside the ADR-0270 body and the SPEC — the actual last ADR HEADER is `## ADR-0270`; use the `^## ADR-` recipe above, not a bare `-o` grep.)

- [ ] **Step 2: Re-pin the as-built anchors against the IMPL-session tip**

```bash
grep -n "type clusterHealth struct\|func newClusterHealth\|func (ch \*clusterHealth) inPanic\|func (ch \*clusterHealth) panicInc\|func (ch \*clusterHealth) panicGate\|func parsePanicThreshold" internal/cluster/health.go
grep -n "func tierHealth\|type priorityLeafFactory\|func newPriorityLBWithRNG" internal/cluster/priority.go
grep -n "type leafFactory\|func newSubsetLB" internal/cluster/subset.go
grep -n "func newLocalityWeightedLB\b\|func newLocalityWeightedLBWithRNG\|func (lw \*localityWeightedLB) Pick\|lw.health.panicInc" internal/cluster/locality.go
grep -n "type loadBalancer interface\|type roundRobin struct\|rr.health.panicGate" internal/cluster/loadbalancer.go
grep -n "func buildCluster\|la := c.GetLoadAssignment()\|endpoints, err := extractEndpoints\|health = newClusterHealth\|return buildLeafLB(c, name, sub" internal/cluster/manager.go
grep -n '0096-lb-priority/driver' test/differential/runner_test.go
```
Record the actual line numbers in PROGRESS.md. A drift here means re-verify every citation in this PLAN before proceeding.

- [ ] **Step 3: Enumerate the `newClusterHealth(…, 0.5)` + `.panicThreshold` + factory-name sweep sets** (so Tasks 2/5 touch exactly these)

```bash
grep -rn "newClusterHealth(" internal/cluster/ --include='*.go' | grep -v "func newClusterHealth"   # 28 call sites; 25 test sites pass 0.5 (the other 3 are manager.go:461/501/536 passing parsePanicThreshold(c))
grep -rn "panicThreshold" internal/cluster/ --include='*.go'
grep -rn "priorityLeafFactory\|leafFactory" internal/cluster/ --include='*.go'
```
**Record in PROGRESS.md:** the exact set of `0.5`-passing test call sites (Task 2 rewrites each to `50`), the `.panicThreshold` field references (Task 2 renames to `.panicThresholdPercent`; the `manager.go:461/501/536` production sites are textually UNCHANGED — the type flows through `parsePanicThreshold`), and the `priorityLeafFactory` reference set (Task 5 renames to `healthLeafFactory`). Note `subset.go`'s `leafFactory` stays untouched.

- [ ] **Step 4: Create PROGRESS.md**

Create `docs/envoy-go/phases/54-load-balancer-panic-threshold/PROGRESS.md` with: the 11-task table (status column); the count anchors from Step 1; the as-built line anchors from Step 2; the sweep sets from Step 3; the four D-PT-* resolutions (copied from this PLAN's D-question section); the FINAL ADR-0045 re-check verdict (NO split — 11 tasks, ~150–260 prod LoC; see the re-check at the end of this PLAN).

- [ ] **Step 5: Commit (LOCAL-ONLY)**

```bash
git add docs/envoy-go/phases/54-load-balancer-panic-threshold/PROGRESS.md
git commit -m "phase 54 Task 1: baselines gate + PROGRESS.md (fixtures 98 / fuzzers 52 / stat surface 1200 / BackendKind 38 / DECISIONS tail ADR-0270 confirmed; newClusterHealth 0.5-site + panicThreshold + priorityLeafFactory sweep sets enumerated; go mod tidy -diff empty)"
```

---

## Task 2: AMEND-PT1 — floor + integer cross-multiply + the D-PT-STORE stored-type change

**Goal:** Mirror the reference's integer-truncation panic condition bit-for-bit: FLOOR the configured threshold to an integer percent (`parsePanicThreshold`), store it as an `int` (`clusterHealth.panicThresholdPercent`), and compare via an integer cross-multiply (`inPanic`). Sweep the `float64 → int` type change through the `newClusterHealth` second parameter, the `tierHealth` literal, and all ~22 test call sites.

**Files:**
- Modify: `internal/cluster/health.go`, `internal/cluster/priority.go`
- Modify: `internal/cluster/health_test.go`, `internal/cluster/priority_test.go`, and the mechanical `0.5`-site test files (`locality_test.go`, `loadbalancer_test.go`, `leastrequest_test.go`, `random_test.go`, `maglev_test.go`, `outlier_test.go`, `ringhash_test.go`)

**Interfaces:**
- Produces: `clusterHealth.panicThresholdPercent int`; `newClusterHealth(endpoints []Endpoint, panicThresholdPercent int) *clusterHealth`; `parsePanicThreshold(c *clusterv3.Cluster) int` (floored percent); `(*clusterHealth).inPanic` = integer cross-multiply. Consumed by Tasks 3/4/5/6.

- [ ] **Step 1: Write the failing tests** (`health_test.go` — replace the existing `TestParsePanicThreshold` at `:477-489` and add the cross-multiply test)

```go
// mkPanicEps builds n plain endpoints (strconv is already imported).
func mkPanicEps(n int) []Endpoint {
	eps := make([]Endpoint, n)
	for i := range eps {
		eps[i] = Endpoint{Host: "h" + strconv.Itoa(i), Port: 1}
	}
	return eps
}

func TestParsePanicThreshold_FloorsToIntegerPercent(t *testing.T) {
	// AMEND-PT1: the reference integer-TRUNCATES (floors) the configured
	// threshold to a whole percent before comparing. parsePanicThreshold now
	// returns an integer percent (D-PT-STORE); floor, NOT round.
	cases := []struct {
		value float64
		want  int
	}{
		{0, 0},     // explicit 0 disables (D-PT2)
		{50, 50},   // integer — unchanged
		{60.9, 60}, // floor(60.9) == 60 (the AMEND-PT1 divergence value)
		{66.7, 66}, // floor, not round-half-up (66.7 -> 66)
		{66.5, 66}, // floor, refuting round (66.5 -> 66)
		{100, 100}, // boundary accepted
	}
	for _, c := range cases {
		cl := &clusterv3.Cluster{
			CommonLbConfig: &clusterv3.Cluster_CommonLbConfig{
				HealthyPanicThreshold: &typev3.Percent{Value: c.value},
			},
		}
		if got := parsePanicThreshold(cl); got != c.want {
			t.Errorf("parsePanicThreshold(%.1f) = %d, want %d", c.value, got, c.want)
		}
	}
}

func TestParsePanicThreshold_AbsentDefaults50(t *testing.T) {
	if got := parsePanicThreshold(&clusterv3.Cluster{}); got != 50 {
		t.Errorf("absent common_lb_config: got %d, want 50", got)
	}
}

func TestInPanic_IntegerCrossMultiply(t *testing.T) {
	// AMEND-PT1: panic iff 100*available < panicThresholdPercent*total (strict <).
	mk := func(healthy, total, thresholdPercent int) bool {
		eps := mkPanicEps(total)
		ch := newClusterHealth(eps, thresholdPercent)
		for i := healthy; i < total; i++ {
			ch.states[eps[i].Addr()].healthy.Store(false)
		}
		return ch.inPanic(eps)
	}
	cases := []struct {
		healthy, total, threshold int
		wantPanic                 bool
		note                      string
	}{
		{3, 5, 60, false, "60% at floor(60.9)=60: 300 < 300 false -> NO panic (the AMEND-PT1 divergence; pre-fix 0.60<0.609 wrongly panicked)"},
		{3, 5, 80, true, "60% at 80: 300 < 400 -> panic"},
		{3, 5, 50, false, "60% at 50: 300 < 250 false -> no panic"},
		{2, 3, 67, true, "66.67% at 67: 200 < 201 -> panic"},
		{2, 3, 66, false, "66.67% at floor(66.7)=66: 200 < 198 false -> no panic"},
		{2, 5, 50, true, "40% at 50: 200 < 250 -> panic (absent-default arm, D-PT1)"},
		{1, 5, 0, false, "threshold 0 disables: 100 < 0 never true -> no panic even at 20%"},
		{0, 0, 50, false, "empty set never panics"},
	}
	for _, c := range cases {
		if got := mk(c.healthy, c.total, c.threshold); got != c.wantPanic {
			t.Errorf("inPanic(healthy=%d,total=%d,threshold=%d) = %v, want %v — %s",
				c.healthy, c.total, c.threshold, got, c.wantPanic, c.note)
		}
	}
}
```

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestParsePanicThreshold|TestInPanic_IntegerCrossMultiply' ./... 2>&1 | head -20
```
Expected: COMPILE FAILURE (`newClusterHealth(eps, thresholdPercent)` passes an `int` to a `float64` parameter; `parsePanicThreshold` returns `float64`, so the `!= c.want` (`int`) comparison also fails to compile).

- [ ] **Step 3: Apply the production change** (`health.go`)

Add `"math"` to the import block. Then:

Replace the `clusterHealth.panicThreshold` field (`:72`):
```go
	panicThresholdPercent int // floored integer percent below which panic fires (default 50; strict <, integer cross-multiply — AMEND-PT1)
```

Replace `newClusterHealth` (`:82-92`):
```go
func newClusterHealth(endpoints []Endpoint, panicThresholdPercent int) *clusterHealth {
	ch := &clusterHealth{
		states:                make(map[string]*hostHealth, len(endpoints)),
		panicThresholdPercent: panicThresholdPercent,
		nowNanos:              func() int64 { return time.Now().UnixNano() },
	}
	for _, ep := range endpoints {
		ch.states[ep.Addr()] = newHostHealth()
	}
	return ch
}
```

Replace `inPanic` (`:180-188`):
```go
// inPanic reports whether the cluster is in panic mode: the healthy PERCENTAGE
// is strictly below the FLOORED integer threshold percent. Mirrors the
// reference EXACTLY via an integer cross-multiply (AMEND-PT1,
// reference_percent_threshold_integer_truncation): panic iff
// 100*availableCount < panicThresholdPercent*total (strict <, ULP-safe — no
// float division). Exactly-at-threshold does NOT panic; total==0 -> not panic.
func (ch *clusterHealth) inPanic(eps []Endpoint) bool {
	total := len(eps)
	if total == 0 {
		return false
	}
	return 100*ch.availableCount(eps) < ch.panicThresholdPercent*total
}
```

Replace `parsePanicThreshold` (`:483-491`):
```go
// parsePanicThreshold reads common_lb_config.healthy_panic_threshold (Percent;
// default 50%) and FLOORS it to an integer percent (AMEND-PT1: the reference
// integer-truncates the threshold toward zero before comparing). The value is
// guaranteed in [0,100] by validatePanicThresholdRange (called earlier in
// buildCluster, Task 4), so no clamp is needed. Returns an integer percent.
func parsePanicThreshold(c *clusterv3.Cluster) int {
	p := c.GetCommonLbConfig().GetHealthyPanicThreshold()
	if p == nil {
		return 50
	}
	return int(math.Floor(p.GetValue()))
}
```

- [ ] **Step 4: Update the `tierHealth` literal** (`priority.go:47`)

Replace `panicThreshold: 0` → `panicThresholdPercent: 0` (and the `:33` doc comment "panic PERMANENTLY DISABLED (panicThreshold: 0…" → "…(panicThresholdPercent: 0…"). This keeps the panic-disabled semantics under the integer form: `100*avail < 0*total` == `100*avail < 0` is never true (`avail ≥ 0`).

- [ ] **Step 5: Sweep the mechanical test call sites**

- Every `newClusterHealth(x, 0.5)` → `newClusterHealth(x, 50)` across `health_test.go`, `locality_test.go`, `loadbalancer_test.go`, `leastrequest_test.go`, `random_test.go`, `maglev_test.go`, `outlier_test.go`, `ringhash_test.go`, `priority_test.go` (ALL 25 test sites from Task 1 Step 3 — a literal "sweep 22" would leave 3 uncompilable `…, 0.5)` sites).
- In `priority_test.go`: `view.panicThreshold` → `view.panicThresholdPercent` (`:128/:129`), `tierHealthViews[0].panicThreshold` → `.panicThresholdPercent` (`:238/:239`), and the `:137` comment "panicThreshold=0 means inPanic can never fire: a fraction is never strictly < 0" → "panicThresholdPercent=0 means inPanic can never fire: 100*avail < 0 is never true".
- In `locality_test.go`: the `:379` comment "SHARED panicThreshold (0.5 default) while the CLUSTER-WIDE fraction…" → update to "SHARED panicThresholdPercent (50 default)…" (doc-only, but avoids describing the removed field name + the pre-fix fractional value).

- [ ] **Step 6: Run to verify they pass**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20
```
Expected: PASS, including the full pre-existing suite (the integer form agrees with the old float form at every integer threshold every prior test used).

- [ ] **Step 7: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/... && go vet ./internal/cluster/... && go build ./...
git add internal/cluster/health.go internal/cluster/priority.go internal/cluster/*_test.go
git commit -m "phase 54 Task 2: AMEND-PT1 floor + integer cross-multiply + D-PT-STORE (panicThreshold float64 fraction -> panicThresholdPercent int; parsePanicThreshold floors; inPanic = 100*avail < percent*total, strict <). Corrects the non-integer over-panic (60.9% at 60% healthy: reference no-panic, envoy-go now agrees); agrees with the old form at every integer threshold. Swept all 25 newClusterHealth 0.5-sites -> 50 + priority_test/locality_test panicThreshold refs"
```

---

## Task 3: Parse-path unit hardening — presence semantics through `parsePanicThreshold → newClusterHealth → panicGate`

**Goal:** Close the "every prior test hardcodes 0.5" gap the SPEC calls out (§1/§10): lock in the boundary strictness (D-PT1 i), disable-at-0 (D-PT2), and absent-defaults-50% (D-PT2) semantics as they flow through the SHARED `panicGate` (the gate every leaf LB consults), not just `inPanic` in isolation. These tests pass against Task 2's implementation (a characterization/coverage task — no new production code).

**Files:**
- Modify: `internal/cluster/health_test.go`

- [ ] **Step 1: Write the tests** (`health_test.go`)

```go
func TestPanicGate_DisableAtZero(t *testing.T) {
	// D-PT2: explicit {value:0} (parsePanicThreshold -> 0) disables panic
	// entirely — even at 20% healthy, panicGate does NOT bypass.
	eps := mkPanicEps(5)
	ch := newClusterHealth(eps, 0)
	for i := 1; i < 5; i++ {
		ch.states[eps[i].Addr()].healthy.Store(false) // 1/5 = 20% healthy
	}
	if ch.panicGate(eps) {
		t.Error("threshold 0 must NEVER panic (disable-at-0, D-PT2), even at 20% healthy")
	}
}

func TestPanicGate_ExactlyAtThreshold_NoPanic(t *testing.T) {
	// D-PT1(i): strict < — exactly at the threshold does NOT panic.
	eps := mkPanicEps(5)
	ch := newClusterHealth(eps, 60)
	for i := 3; i < 5; i++ {
		ch.states[eps[i].Addr()].healthy.Store(false) // 3/5 = exactly 60%
	}
	if ch.panicGate(eps) {
		t.Error("60% healthy at a 60% threshold must NOT panic (strict <, D-PT1 i)")
	}
}

func TestPanicGate_AbsentDefault50_PanicsBelow(t *testing.T) {
	// D-PT2: absent threshold defaults to 50%; 2/5 = 40% < 50% -> panic.
	eps := mkPanicEps(5)
	ch := newClusterHealth(eps, parsePanicThreshold(&clusterv3.Cluster{})) // == 50
	for i := 2; i < 5; i++ {
		ch.states[eps[i].Addr()].healthy.Store(false) // 2/5 = 40%
	}
	if !ch.panicGate(eps) {
		t.Error("40% healthy at the absent-default 50% threshold must panic")
	}
}
```

- [ ] **Step 2: Run to verify they pass** (a coverage task — they exercise Task 2's wired path)

```bash
cd internal/cluster && go test -run 'TestPanicGate_DisableAtZero|TestPanicGate_ExactlyAtThreshold_NoPanic|TestPanicGate_AbsentDefault50_PanicsBelow' ./... -v 2>&1 | tail -20
```
Expected: PASS. (If `mkPanicEps` was not yet present because Tasks 2/3 land out of order, add it here — but Task 2 introduced it.)

- [ ] **Step 3: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/... && go vet ./internal/cluster/...
git add internal/cluster/health_test.go
git commit -m "phase 54 Task 3: parse-path presence-semantics coverage through panicGate (disable-at-0 / exactly-at-threshold strict< / absent-defaults-50%) — closes the every-prior-test-hardcodes-0.5 gap (SPEC §1/§10; D-PT1 i / D-PT2)"
```

---

## Task 4: AMEND-PT4 — the standalone out-of-range `[0,100]` reject (D-PT-REJECT-PLACEMENT; ADR-0080)

**Goal:** Mirror the reference's boot-time PGV rejection of `healthy_panic_threshold.value` outside `[0,100]`. A STANDALONE, UNCONDITIONAL check early in `buildCluster` — fired even for a PLAIN cluster that never reaches any health-guarded `parsePanicThreshold` site.

**Files:**
- Modify: `internal/cluster/health.go` (`validatePanicThresholdRange`), `internal/cluster/manager.go` (the call site)
- Modify: `internal/cluster/manager_test.go`

**Interfaces:**
- Produces: `validatePanicThresholdRange(c *clusterv3.Cluster, name string) error` (nil when absent or in-range; else the byte-stable reject).

- [ ] **Step 1: Write the failing tests** (`manager_test.go`; `strings`/`clusterv3`/`endpointv3` already imported — ADD `typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"`, which `manager_test.go` does NOT yet import)

```go
func TestBuildCluster_RejectsPanicThresholdAbove100(t *testing.T) {
	c := mkStaticCluster("c_pt_hi", mkLbEndpoint("127.0.0.1", 9001))
	c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{HealthyPanicThreshold: &typev3.Percent{Value: 150}}
	_, err := buildCluster(c, 0, "")
	if err == nil || !strings.Contains(err.Error(), "healthy_panic_threshold") {
		t.Fatalf("value 150 must be rejected with a healthy_panic_threshold message; got err = %v", err)
	}
}

func TestBuildCluster_RejectsPanicThresholdNegative(t *testing.T) {
	c := mkStaticCluster("c_pt_lo", mkLbEndpoint("127.0.0.1", 9001))
	c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{HealthyPanicThreshold: &typev3.Percent{Value: -10}}
	_, err := buildCluster(c, 0, "")
	if err == nil || !strings.Contains(err.Error(), "healthy_panic_threshold") {
		t.Fatalf("value -10 must be rejected; got err = %v", err)
	}
}

func TestBuildCluster_AcceptsPanicThresholdBoundaries(t *testing.T) {
	// Exactly 0 and 100 are ACCEPTED (inclusive) — on a PLAIN cluster (no
	// health_checks), proving the check is unconditional yet non-over-eager.
	for _, v := range []float64{0, 100} {
		c := mkStaticCluster("c_pt_ok", mkLbEndpoint("127.0.0.1", 9001))
		c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{HealthyPanicThreshold: &typev3.Percent{Value: v}}
		if _, err := buildCluster(c, 0, ""); err != nil {
			t.Errorf("boundary value %v must be accepted; got err = %v", v, err)
		}
	}
}

func TestBuildCluster_RejectsOutOfRangeOnPlainCluster(t *testing.T) {
	// D-PT-REJECT-PLACEMENT: a PLAIN cluster (no health_checks, no
	// outlier_detection, no wrapper) never reaches parsePanicThreshold, yet the
	// reference rejects an out-of-range value at boot — the STANDALONE check
	// must still fire here (this is why folding into parsePanicThreshold fails).
	c := mkStaticCluster("c_plain_hi", mkLbEndpoint("127.0.0.1", 9001))
	c.CommonLbConfig = &clusterv3.Cluster_CommonLbConfig{HealthyPanicThreshold: &typev3.Percent{Value: 200}}
	if _, err := buildCluster(c, 0, ""); err == nil {
		t.Fatal("out-of-range panic threshold on a plain (no-HC) cluster must still be rejected")
	}
}
```
(If `mkStaticCluster`/`mkLbEndpoint` do not construct a `CommonLbConfig`-carrying cluster directly, set `c.CommonLbConfig` on the returned `*clusterv3.Cluster` as above — confirmed the helpers return a mutable `*clusterv3.Cluster` at Task 1.)

- [ ] **Step 2: Run to verify they fail**

```bash
cd internal/cluster && go test -run 'TestBuildCluster_RejectsPanicThreshold|TestBuildCluster_AcceptsPanicThresholdBoundaries|TestBuildCluster_RejectsOutOfRangeOnPlainCluster' ./... 2>&1 | head -20
```
Expected: FAIL — the reject/accept tests fail because envoy-go currently accepts ANY value (no range check).

- [ ] **Step 3: Add the standalone validator** (`health.go`, next to `parsePanicThreshold`)

```go
// validatePanicThresholdRange rejects an out-of-range healthy_panic_threshold
// at boot, mirroring the reference's PGV [0,100] constraint (AMEND-PT4,
// ADR-0080). STANDALONE + UNCONDITIONAL: it must fire for ANY cluster carrying
// the field, including a plain cluster that never builds a health registry
// (whose parsePanicThreshold call sites are all health-guarded). Exactly 0 and
// 100 are accepted (inclusive). NaN is config-unreachable (protojson rejects a
// NaN double at unmarshal).
func validatePanicThresholdRange(c *clusterv3.Cluster, name string) error {
	p := c.GetCommonLbConfig().GetHealthyPanicThreshold()
	if p == nil {
		return nil
	}
	if v := p.GetValue(); v < 0 || v > 100 {
		return fmt.Errorf("cluster: %q: common_lb_config.healthy_panic_threshold: value must be inside range [0, 100]", name)
	}
	return nil
}
```

- [ ] **Step 4: Call it early in `buildCluster`** (`manager.go`, immediately after the `extractEndpoints` block succeeds — before the health parse + LB switch)

Insert after `endpoints, err := extractEndpoints(la, name)` returns (currently `:433-436`):
```go
	if err := validatePanicThresholdRange(c, name); err != nil {
		return nil, err
	}
```

- [ ] **Step 5: Run to verify they pass**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20
```
Expected: PASS (all four new tests + the full suite).

- [ ] **Step 6: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/... && go vet ./internal/cluster/... && go build ./...
git add internal/cluster/health.go internal/cluster/manager.go internal/cluster/manager_test.go
git commit -m "phase 54 Task 4: AMEND-PT4 out-of-range [0,100] reject (D-PT-REJECT-PLACEMENT: standalone validatePanicThresholdRange, unconditional early in buildCluster — fires for a plain no-HC cluster the three health-guarded parsePanicThreshold sites never reach). Byte-stable message (ADR-0080); 0/100 accepted"
```

---

## Task 5: AMEND-PT3 — the `locality.go` child-local-panic retrofit (relocate `tierHealth`, share `healthLeafFactory`, health-parameterize the constructors)

**Goal:** Build each per-locality child against a PANIC-DISABLED `tierHealth(health)` view so a single degraded locality never internally flattens (its unhealthy hosts stay at zero — D-PT4); keep the FLAT fallback child on the shared, panic-enabled health. This requires the load-bearing factory-signature change (D-PT-TIERHEALTH-HOME): relocate `tierHealth` to `health.go`, rename `priorityLeafFactory → healthLeafFactory` (relocated to `loadbalancer.go`, shared by locality + priority), and thread `h *clusterHealth` through the constructors + the `manager.go` closure. `subsetLB` stays UNTOUCHED (a regression test locks in that a degraded subset still flattens).

**Files:**
- Modify: `internal/cluster/health.go` (relocate `tierHealth` in), `internal/cluster/loadbalancer.go` (add `healthLeafFactory`), `internal/cluster/priority.go` (remove `tierHealth`, rename type refs), `internal/cluster/locality.go` (signature + wiring), `internal/cluster/manager.go` (the locality closure)
- Modify: `internal/cluster/locality_test.go`, `internal/cluster/priority_test.go`, `internal/cluster/subset_test.go`

**Interfaces:**
- Produces: `type healthLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)` (loadbalancer.go); `tierHealth(shared *clusterHealth) *clusterHealth` (health.go). `newLocalityWeightedLB`/`newLocalityWeightedLBWithRNG` now take `factory healthLeafFactory`.

- [ ] **Step 1: Write the failing test** (`locality_test.go`) — the per-locality no-local-panic property. NOTE: `locality_test.go` currently has only `import "testing"`; widen it to a block adding `"strconv"` (this test) and `"github.com/pgdad/envoy-go/internal/stats"` (Task 6's counter test).

```go
func TestPick_DegradedLocality_NoLocalPanic(t *testing.T) {
	// AMEND-PT3 / D-PT4: a single degraded locality (40% healthy) within a
	// cluster that is NOT in cluster-wide panic (70% overall) must NOT locally
	// flatten — its unhealthy hosts receive ZERO traffic. The per-locality
	// child is built against a panic-DISABLED tierHealth view.
	mkLoc := func(region string, n int) []Endpoint {
		eps := make([]Endpoint, n)
		for i := range eps {
			eps[i] = Endpoint{Host: region + strconv.Itoa(i), Port: 1, Locality: LocalityID{Region: region}, LocalityWeight: 1}
		}
		return eps
	}
	epsA := mkLoc("a", 5)
	epsB := mkLoc("b", 5)
	all := append(append([]Endpoint{}, epsA...), epsB...)
	health := newClusterHealth(all, 50)
	// Degrade locality A to 2/5 = 40% (below 50%), cluster-wide 7/10 = 70% >= 50%.
	for _, dead := range []string{"a2:1", "a3:1", "a4:1"} {
		health.states[dead].healthy.Store(false)
	}
	// Real roundRobin children so per-host availability filtering is LIVE.
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		return &roundRobin{endpoints: sub, health: h}, nil
	}
	// rng()==0 -> r==0 -> the FIRST bucket (locality A, encounter order) is
	// always drawn; every pick routes to A's child.
	lw, err := newLocalityWeightedLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for i := 0; i < 100; i++ {
		ep, _, err := lw.Pick(0, false, SubsetMatch{}, false)
		if err != nil {
			t.Fatal(err)
		}
		seen[ep.Addr()]++
	}
	for _, dead := range []string{"a2:1", "a3:1", "a4:1"} {
		if seen[dead] != 0 {
			t.Errorf("degraded locality A must NOT flatten (per-locality panic-disabled): unhealthy host %s got %d picks, want 0", dead, seen[dead])
		}
	}
	if seen["a0:1"]+seen["a1:1"] == 0 {
		t.Error("locality A's healthy hosts must still receive traffic")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

```bash
cd internal/cluster && go test -run 'TestPick_DegradedLocality_NoLocalPanic' ./... 2>&1 | head -20
```
Expected: COMPILE FAILURE (the factory literal `func(sub []Endpoint, h *clusterHealth)` does not match the current `leafFactory func(sub []Endpoint)` parameter of `newLocalityWeightedLBWithRNG`).

- [ ] **Step 3: Relocate `tierHealth` to `health.go`**

Move the `tierHealth` function + its doc comment from `priority.go` (`:30-48`) to `health.go` (place it after `parsePanicThreshold`). Body is unchanged (the literal is already `panicThresholdPercent: 0` from Task 2). Delete it from `priority.go`.

- [ ] **Step 4: Add `healthLeafFactory` to `loadbalancer.go`; rename in `priority.go`**

In `loadbalancer.go` (after the `loadBalancer` interface):
```go
// healthLeafFactory builds a child loadBalancer over an endpoint sub-slice,
// against an EXPLICIT health view h (rather than a value baked into the closure
// at the manager.go call site) — required so a wrapper can hand different
// children different health views: localityWeightedLB/priorityLB give per-group
// children a panic-DISABLED tierHealth(health) view while keeping the flat
// fallback on a different registry. subset.go's leafFactory (no health param)
// is a DISTINCT type — subset children close over the shared health directly.
type healthLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)
```
In `priority.go`: delete the local `priorityLeafFactory` type (`:87-102`) and replace both consumers (`newPriorityLB` `:131`, `newPriorityLBWithRNG` `:154`) `factory priorityLeafFactory` → `factory healthLeafFactory`. In `priority_test.go`: `trackingPriorityFactory() (priorityLeafFactory, …)` → `(healthLeafFactory, …)` (`:178`) + the `:167` comment.

- [ ] **Step 5: Health-parameterize `localityWeightedLB`'s constructors + wiring** (`locality.go`)

Change both constructor signatures `factory leafFactory` → `factory healthLeafFactory` (`:67`, `:85`). In `newLocalityWeightedLBWithRNG`, replace the child/flat builds (`:101-113`):
```go
	tierView := health
	if health != nil {
		tierView = tierHealth(health) // AMEND-PT3: per-locality children never locally panic
	}
	for _, id := range order {
		members := byLocality[id]
		child, err := factory(members, tierView) // panic-DISABLED view
		if err != nil {
			return nil, err
		}
		lw.groups = append(lw.groups, localityGroup{id: id, endpoints: members, weight: weights[id], child: child})
	}
	flat, err := factory(endpoints, health) // KEEPS the shared, panic-ENABLED health (§3.2/§3.3)
	if err != nil {
		return nil, err
	}
	lw.flat = flat
```

- [ ] **Step 6: Update the `manager.go` locality closure** (`:513-515`)

```go
		lw, err := newLocalityWeightedLB(endpoints, health, opf, hasOPF, func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
			return buildLeafLB(c, name, sub, h)
		})
```
(The subset closure at `:478-479` and the priority closure at `:547-549` are UNCHANGED — subset stays on the health-closing `leafFactory`; priority already uses the health-parameterized shape.)

- [ ] **Step 7: Sweep the existing `locality_test.go` factory literals to the new signature**

Every `factory := func(sub []Endpoint) (loadBalancer, error) {…}` and `trackingFactory() (leafFactory, …)` in `locality_test.go` gains the `h *clusterHealth` parameter. The stub-returning factories IGNORE `h` (the stubs assert delegation, not health filtering, so behavior is unchanged): `func(sub []Endpoint, _ *clusterHealth) (loadBalancer, error)`. Update `trackingFactory`'s return type `leafFactory → healthLeafFactory` and its inner literal accordingly. (The full set: the `:13` `trackingFactory` + the inline literals at `:95`, `:129`, `:164`, `:197`, `:227`, `:338`, `:407`.)

- [ ] **Step 8: Add the subset-unchanged regression** (`subset_test.go`)

```go
func TestSubsetLB_DegradedSubset_StillFlattens(t *testing.T) {
	// AMEND-PT3 sub-question / D-PT4 (the crux asymmetry): a subset is its OWN
	// LB host-set, so a degraded subset DOES panic per-subset (unhealthy hosts
	// served). subsetLB is UNCHANGED by phase 54 — its children keep the SHARED
	// (panic-enabled) health, unlike locality's new tierHealth children.
	eps := []Endpoint{
		epMD("h0", 1, map[string]SubsetValue{"v": {Kind: subsetString, Str: "x"}}),
		epMD("h1", 1, map[string]SubsetValue{"v": {Kind: subsetString, Str: "x"}}),
		epMD("h2", 1, map[string]SubsetValue{"v": {Kind: subsetString, Str: "x"}}),
		epMD("h3", 1, map[string]SubsetValue{"v": {Kind: subsetString, Str: "x"}}),
		epMD("h4", 1, map[string]SubsetValue{"v": {Kind: subsetString, Str: "x"}}),
	}
	health := newClusterHealth(eps, 50)
	for _, dead := range []string{"h2:1", "h3:1", "h4:1"} {
		health.states[dead].healthy.Store(false) // 2/5 = 40% < 50%
	}
	// The subset children CLOSE over the shared health (the manager.go:478-479
	// wiring — UNCHANGED this phase; leafFactory, no health parameter).
	factory := func(sub []Endpoint) (loadBalancer, error) {
		return &roundRobin{endpoints: sub, health: health}, nil
	}
	s := newSubsetLB(eps, lbSubsetCfg{fallback: fallbackNoFallback, selectors: [][]string{{"v"}}}, factory)
	match := NewSubsetMatch(map[string]SubsetValue{"v": {Kind: subsetString, Str: "x"}})
	served := 0
	for i := 0; i < 100; i++ {
		ep, _, err := s.Pick(0, false, match, true)
		if err != nil {
			t.Fatal(err)
		}
		if ep.Addr() == "h2:1" || ep.Addr() == "h3:1" || ep.Addr() == "h4:1" {
			served++
		}
	}
	if served == 0 {
		t.Error("a degraded subset must FLATTEN (per-subset panic, D-PT4): its unhealthy hosts must receive traffic, got 0")
	}
}
```

- [ ] **Step 9: Run to verify all pass**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20
```
Expected: PASS (the new retrofit test + the subset regression + the full suite, incl. the swept locality/priority tests).

- [ ] **Step 10: Prove the retrofit assertion is LIVE** (scratch-revert, `reference_differential_asserter_dispatch` generalized)

Temporarily change `locality.go` Step 5's per-locality build back to the shared health (`child, err := factory(members, health)` instead of `tierView`), re-run `TestPick_DegradedLocality_NoLocalPanic` → it MUST FAIL (a degraded locality's child now locally panics → unhealthy hosts served). Restore `tierView`. Record the observed failure in PROGRESS.md. (A GIT HYGIENE note per `feedback_subagent_worktree_detach`: use `git restore`, not checkout-sha/amend.)

- [ ] **Step 11: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/... && go vet ./internal/cluster/... && go build ./...
git add internal/cluster/health.go internal/cluster/loadbalancer.go internal/cluster/locality.go internal/cluster/priority.go internal/cluster/manager.go internal/cluster/locality_test.go internal/cluster/priority_test.go internal/cluster/subset_test.go
git commit -m "phase 54 Task 5: AMEND-PT3 locality child-local-panic retrofit (per-locality children -> panic-disabled tierHealth view; flat child keeps shared health). D-PT-TIERHEALTH-HOME: relocate tierHealth to health.go + rename priorityLeafFactory -> healthLeafFactory (loadbalancer.go, shared by locality+priority) + health-parameterize newLocalityWeightedLB(WithRNG) + the manager locality closure. subsetLB UNCHANGED (degraded-subset-still-flattens regression). Retrofit assertion scratch-revert-proven live"
```

---

## Task 6: AMEND-PT2 — the `lb_healthy_panic` double-increment fix (remove the redundant outer `panicInc()`)

**Goal:** On the locality-weighted cluster-wide panic path, increment `lb_healthy_panic` exactly ONCE per pick (`2N → N`) by DELETING the outer `lw.health.panicInc()` at `locality.go:144` — the shared-health flat child's own `panicGate` provides the single increment.

**Files:**
- Modify: `internal/cluster/locality.go`
- Modify: `internal/cluster/locality_test.go`

- [ ] **Step 1: Write the failing test** (`locality_test.go`) — delta == N, not 2N

```go
func TestPick_LocalityPanic_IncrementsOncePerPick(t *testing.T) {
	// AMEND-PT2 / D-PT1(iii): under cluster-wide panic the locality-weighted LB
	// must increment lb_healthy_panic EXACTLY ONCE per pick (not twice). The
	// flat child (shared, panic-enabled health) does the single increment via
	// its own panicGate; the outer panicInc() is redundant and removed.
	mkLoc := func(region string, n int) []Endpoint {
		eps := make([]Endpoint, n)
		for i := range eps {
			eps[i] = Endpoint{Host: region + strconv.Itoa(i), Port: 1, Locality: LocalityID{Region: region}, LocalityWeight: 1}
		}
		return eps
	}
	all := append(append([]Endpoint{}, mkLoc("a", 2)...), mkLoc("b", 2)...)
	health := newClusterHealth(all, 50)
	reg := stats.NewRegistry()
	health.panicCounter = reg.NewCounter("lb_healthy_panic")
	for _, ep := range all { // 0/4 healthy -> cluster-wide panic
		health.states[ep.Addr()].healthy.Store(false)
	}
	// Real roundRobin children bound to the shared health (the flat child's
	// panicGate is what increments; healthLeafFactory signature from Task 5).
	factory := func(sub []Endpoint, h *clusterHealth) (loadBalancer, error) {
		return &roundRobin{endpoints: sub, health: h}, nil
	}
	lw, err := newLocalityWeightedLBWithRNG(all, health, 140, true, factory, func() uint64 { return 0 })
	if err != nil {
		t.Fatal(err)
	}
	const n = 50
	for i := 0; i < n; i++ {
		if _, _, err := lw.Pick(0, false, SubsetMatch{}, false); err != nil {
			t.Fatal(err)
		}
	}
	if got := health.panicCounter.Load(); got != n {
		t.Errorf("lb_healthy_panic = %d over %d picks, want %d (once per pick, not 2N) — AMEND-PT2", got, n, n)
	}
}
```
(`(*stats.Counter).Load() uint64` is the universal counter-read idiom in the package — e.g. `h2pool_test.go` `.Load()`; there is NO `Registry.Snapshot`. The flat child bound to shared health increments via `roundRobin.Pick`'s `rr.health.panicGate`. This test needs the `locality_test.go` import block widened to add `"strconv"` and `"github.com/pgdad/envoy-go/internal/stats"` — the file currently has only `import "testing"`.)

- [ ] **Step 2: Run to verify it fails**

```bash
cd internal/cluster && go test -run 'TestPick_LocalityPanic_IncrementsOncePerPick' ./... 2>&1 | head -20
```
Expected: FAIL — `lb_healthy_panic = 100 over 50 picks, want 50` (the current double-increment: outer `panicInc()` + the flat child's `panicGate`).

- [ ] **Step 3: Remove the redundant outer increment** (`locality.go:142-146`)

```go
func (lw *localityWeightedLB) Pick(hashKey uint64, hasHash bool, match SubsetMatch, hasMatch bool) (Endpoint, func(), error) {
	if lw.health != nil && lw.health.inPanic(lw.allEndpoints) {
		return lw.flat.Pick(hashKey, hasHash, match, hasMatch) // AMEND-PT2: the flat child's own panicGate does the single increment
	}
	// ... unchanged bucket walk ...
```
Delete the `lw.health.panicInc()` line. Update the `Pick` doc comment (`:127-146`) to note the single-increment delegation.

- [ ] **Step 4: Run to verify it passes**

```bash
cd internal/cluster && go test ./... 2>&1 | tail -20
```
Expected: PASS (delta == N; the full suite, incl. `TestPick_PanicBypassesLocalityWeighting` which asserts flat-child delegation — unaffected by the removal since its stubs don't call `panicGate` and its `panicCounter` is nil).

- [ ] **Step 5: Prove the assertion is LIVE** (scratch-revert)

Temporarily re-insert `lw.health.panicInc()` in the panic branch, re-run `TestPick_LocalityPanic_IncrementsOncePerPick` → it MUST FAIL (`got 100, want 50`). Remove it again. Record in PROGRESS.md.

- [ ] **Step 6: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l internal/cluster/ && golangci-lint run ./internal/cluster/... && go vet ./internal/cluster/... && go build ./...
git add internal/cluster/locality.go internal/cluster/locality_test.go
git commit -m "phase 54 Task 6: AMEND-PT2 lb_healthy_panic double-increment fix (remove the redundant outer panicInc() at localityWeightedLB.Pick; the shared-health flat child's own panicGate provides the single increment -> 2N corrected to N). Delta==N scratch-revert-proven live"
```

---

## Task 7: `0097-lb-panic-threshold` — the three-arm cross-side differential (fixture + driver)

**Goal:** Build the first-ever differential proof of the panic construct: ONE boot, ONE HTTP listener path-routing to THREE health-checked STATIC clusters × 5 hosts, each degraded the SAME 2-of-5 (→ 60% healthy), differing ONLY in `healthy_panic_threshold` (A: 80 → PANICS; B: absent/50 → NO panic; C: 60.9 → floor 60 → NO panic). Reuses the `0096-lb-priority` driver harness as the direct template. BackendKind tail STAYS 38.

**Files:**
- Create: `test/fixtures/0097-lb-panic-threshold/driver/driver.go`, `.../driver/driver_test.go`, `.../README.md`, `.../expectations.yaml`
- Modify: `test/differential/runner_test.go` (blank-import)

**Interfaces:**
- Consumes: `test/differential/fixture/fixture.go` (`Driver`/`StatsAsserter`/`TB`/`BackendKind`); the `0096` harness helpers (`toggleResponder`/`newToggleResponder`/`pollMembershipHealthy`/`warmupUntilStable`/`scrapeStats`, adapted).

- [ ] **Step 1: Scaffold the driver + register it**

Model `driver.go` on `test/fixtures/0096-lb-priority/driver/driver.go` (reuse its `toggleResponder`/`newToggleResponder`/`pollMembershipHealthy`/`warmupUntilStable`/`scrapeStats`/`assertEq` helpers verbatim). Package-level constants (named, `TestConstants`-guarded — `reference_fixture_workload_constant_desync`):
```go
const (
	fixtureName       = "0097-lb-panic-threshold"
	clusterCount      = 3   // c_pt_a (thr 80) / c_pt_b (absent/50) / c_pt_c (thr 60.9)
	hostsPerCluster   = 5
	degradedPerCluster = 2  // -> 3/5 = 60% healthy per cluster
	healthyPerCluster = hostsPerCluster - degradedPerCluster // 3
	loadPerCluster    = 200 // requests driven per cluster path (offset-invariant / count assertions)
)
```
Cluster NAMES are DISTINCT (`c_pt_a`/`c_pt_b`/`c_pt_c` — `reference_admin_interface_wire_name_collision`; per-name stat accessors disambiguate). The driver owns `clusterCount*hostsPerCluster = 15` toggle responders (built once in an `ensureBackends()` method, the `0096` `ensureTier0` precedent — reset-outside-`AssertStats` so both arms/sides see the same degradation), each returning a body identifying its cluster + host index on the DATA path and 200/503 on `/healthz` per its toggle. `BackendKind()` returns `fixture.HTTPEcho` (tail 38 UNCHANGED); the 15 hosts are all driver-owned (no runner-spawned backends needed → `BackendCount()` returns 0, or the minimal count the harness requires — confirm against `0096`'s `BackendCount` contract at implementation). Append to `test/differential/runner_test.go` (after the `0096` import):
```go
	_ "github.com/pgdad/envoy-go/test/fixtures/0097-lb-panic-threshold/driver"
```

- [ ] **Step 2: Bootstrap builders** (`ReferenceBootstrap` / `SubjectConfig`)

ONE HTTP listener (`l_http`) with a route table path-routing `/a → c_pt_a`, `/b → c_pt_b`, `/c → c_pt_c`. Three STATIC clusters, each 5 hosts (the driver-owned toggle-responder ports), each with an active HTTP `health_checks` block (path `/healthz`, `interval`/`timeout` 1s, thresholds 1/1 — fast convergence, the `0096` `healthChecksBlock` precedent) and ROUND_ROBIN. The ONLY per-cluster difference is `common_lb_config.healthy_panic_threshold`: `c_pt_a` `{value: 80}`; `c_pt_b` OMITS `common_lb_config` (the absent/50% default arm); `c_pt_c` `{value: 60.9}`. Reference addressing via `host.docker.internal` (`reference_docker_probe_bridge_network`); the subject via the driver-owned literal ports.

- [ ] **Step 3: `classifyBody` + `driver_test.go`**

`classifyBody(body []byte) (cluster string, host int, err error)` parses the toggle-responder body (e.g. `c_pt_a:3`). `driver_test.go`:
```go
func TestClassifyBody(t *testing.T) {
	cases := []struct{ body, wantCluster string; wantHost int; wantErr bool }{
		{"c_pt_a:0", "c_pt_a", 0, false},
		{"c_pt_c:4", "c_pt_c", 4, false},
		{"garbage", "", 0, true},
		{"", "", 0, true},
	}
	for _, c := range cases {
		gotC, gotH, err := classifyBody([]byte(c.body))
		if (err != nil) != c.wantErr {
			t.Errorf("classifyBody(%q): err=%v wantErr=%v", c.body, err, c.wantErr)
			continue
		}
		if gotC != c.wantCluster || gotH != c.wantHost {
			t.Errorf("classifyBody(%q) = (%q,%d), want (%q,%d)", c.body, gotC, gotH, c.wantCluster, c.wantHost)
		}
	}
}

func TestConstants(t *testing.T) {
	if healthyPerCluster != hostsPerCluster-degradedPerCluster {
		t.Errorf("healthyPerCluster=%d != hostsPerCluster-degradedPerCluster", healthyPerCluster)
	}
	if degradedPerCluster*2 >= hostsPerCluster {
		t.Errorf("degradedPerCluster=%d must keep >50%% healthy for the B/C no-panic arms", degradedPerCluster)
	}
}
```

- [ ] **Step 4: `AssertStats`** (all three arms in-band — the only hook holding both admin addrs)

The sequence (per side, reference + subject):
1. `ensureBackends()`; degrade `degradedPerCluster` hosts in EACH cluster (toggle their `/healthz` to 503).
2. `pollMembershipHealthy(side, adminAddr, "c_pt_a"/"c_pt_b"/"c_pt_c", healthyPerCluster)` → each cluster converges to `membership_healthy == 3`.
3. `warmupUntilStable` per cluster path (exclude the degraded-host bodies until convergence — the `0096` `excludeBodies` mechanism).
4. Drive `loadPerCluster` requests to each of `/a`, `/b`, `/c`; tally per (cluster, host) via `classifyBody`.
5. `scrapeStats` both admin endpoints.

Assertions (BOTH sides; use `assertEq` for exact counts):
```go
// Decode-ran guard first (reference_docker_probe_bridge_network).
// c_pt_a — PANICS (60% < 80): all 5 hosts (incl the 2 degraded) served; lb_healthy_panic == loadPerCluster.
for h := 0; h < hostsPerCluster; h++ {
	if tally["c_pt_a"][h] == 0 {
		t.Errorf("%s c_pt_a host %d got 0 — panic must serve ALL hosts incl unhealthy (offset-invariant)", side, h)
	}
}
assertEq(t, side, st, "cluster.c_pt_a.lb_healthy_panic", loadPerCluster) // once per pick (D-PT1 iii)
// c_pt_b — NO panic (60% >= 50, absent-default arm): the 2 degraded hosts == 0; lb_healthy_panic == 0.
assertEq(t, side, st, "cluster.c_pt_b.lb_healthy_panic", 0)
// (the 2 degraded c_pt_b hosts have tally == 0)
// c_pt_c — NO panic (floor(60.9)=60; 60<60 false — the integer-truncation discriminator): degraded == 0; lb_healthy_panic == 0.
assertEq(t, side, st, "cluster.c_pt_c.lb_healthy_panic", 0)
assertEq(t, side, st, "cluster.c_pt_a.membership_healthy", healthyPerCluster)
assertEq(t, side, st, "cluster.c_pt_b.membership_healthy", healthyPerCluster)
assertEq(t, side, st, "cluster.c_pt_c.membership_healthy", healthyPerCluster)
```
Assert the 2 degraded hosts of `c_pt_b`/`c_pt_c` have `tally == 0` (identify degraded indices via the fixed degrade set, e.g. indices 3 and 4). For `c_pt_a`'s `lb_healthy_panic == loadPerCluster` EXACT assertion, use the SAME per-cluster drive count on both sides (workload-synced). Response-body byte-exact applies ONLY to the fixed `READY\n` Drive stream (the `0096` precedent), NOT the tallied load-phase bodies (a randomized RR — cross-side per-request identity infeasible, `reference_round_robin_offset_randomized`).

- [ ] **Step 5: `README.md` + `expectations.yaml`**

Document the three-arm design (mirroring `0096`'s `expectations.yaml` shape): the topology, the three arms + WHY each panics/doesn't (incl. the `60.9 → floor 60` integer-truncation discriminator), the cross-side deterministic stats, the NOT-exercised note (the locality/subset double-increment + retrofit are UNIT-tested — SPEC §8.2), and the non-additions (NO new BackendKind; NO separate boot-reject dir — the out-of-range reject is a `manager_test.go` unit test).

- [ ] **Step 6: Run the fixture (subject-only smoke, then cross-side)**

```bash
go test ./test/fixtures/0097-lb-panic-threshold/driver/ -run 'TestClassifyBody|TestConstants' -count=1
go test ./test/differential/ -run 'TestDifferential/0097' -count=1 2>&1 | tail -30
```
Expected: the driver unit tests PASS; the cross-side run PASSES (both sides agree). (If a transient `subject ready: EOF` appears, isolate-re-run per `reference_differential_fullsuite_startup_flake` — it is a startup race, not a mismatch.)

- [ ] **Step 7: gofmt + lint + commit (LOCAL-ONLY)**

```bash
gofmt -l test/fixtures/0097-lb-panic-threshold/ && golangci-lint run ./test/... && go vet ./test/... && go build ./...
git add test/fixtures/0097-lb-panic-threshold/ test/differential/runner_test.go
git commit -m "phase 54 Task 7: 0097-lb-panic-threshold three-arm cross-side differential (A thr80 PANICS all-5-hosts / B absent-50 no-panic / C 60.9 floor-60 no-panic — the integer-truncation discriminator). 3 health-checked clusters x 5 hosts, degrade 2/5, poll membership_healthy -> warmup -> drive -> StatsAsserter + per-host tallies. Registered; BackendKind tail STAYS 38 (fixtures 98 -> 99)"
```

---

## Task 8: `0097` deliberate breaks + flake-soak + `-race`

**Goal:** Prove every `0097` assertion is LIVE via the three deliberate breaks (`-count=1`), then a ≥20-run flake-free soak + `-race`.

**Files:**
- Modify: `test/fixtures/0097-lb-panic-threshold/README.md` (append the break-protocol record)

- [ ] **Step 1: Break (a) — hardcode `parsePanicThreshold` to `50`** (ignore the field)

Temporarily change `parsePanicThreshold` to `return 50`. Run `go test ./test/differential/ -run 'TestDifferential/0097' -count=1`. Expected FAIL: `c_pt_a` no longer panics (60% ≥ 50) → its degraded-hosts-`>0` assertion fails AND `cluster.c_pt_a.lb_healthy_panic == loadPerCluster` fails (now 0). Restore. Record the observed failure.

- [ ] **Step 2: Break (b) — skip the degradation step**

Temporarily no-op the per-cluster degradation in `AssertStats`. Run `-count=1`. Expected FAIL: all hosts healthy → `membership_healthy` never reaches 3 (poll-to-converge fails) / `c_pt_a` never panics → A's panic assertions fail. Restore. Record.

- [ ] **Step 3: Break (c) — revert the AMEND-PT1 floor** (compare as a float fraction)

Temporarily change `inPanic` to the pre-fix float form (`float64(availableCount)/float64(total) < float64(panicThresholdPercent)/100.0`) AND `parsePanicThreshold` to `return int(p.GetValue())` WITHOUT floor... — simplest faithful revert: restore the pre-Task-2 float shape locally (fraction threshold + float compare). Run `-count=1`. Expected FAIL: `c_pt_c` now panics (`0.60 < 0.609`) → its `degraded == 0` / `lb_healthy_panic == 0` assertions fail. Restore. Record. (This is THE differential proof of the truncation fix — SPEC §8.1 break (c).)

- [ ] **Step 4: Flake-soak + `-race`**

```bash
go test ./test/differential/ -run 'TestDifferential/0097' -count=20 2>&1 | tail -5   # >=20-run flake-free
go test ./test/differential/ -run 'TestDifferential/0097' -race -count=1 2>&1 | tail -10
```
Expected: 20/20 PASS; `-race` clean. If a run flakes on the warmup/convergence, raise `warmupStable` (the phase-53 10→60 lesson) — NOT the assertion margins (the panic assertions are hard 0-vs-nonzero, not statistical bands).

- [ ] **Step 5: Commit (LOCAL-ONLY)**

```bash
git add test/fixtures/0097-lb-panic-threshold/README.md
git commit -m "phase 54 Task 8: 0097 deliberate breaks (a hardcode-50 / b skip-degrade / c revert-floor -> c_pt_c panics) all bite -count=1; >=20-run flake-free + -race clean. Break (c) is the differential proof of the AMEND-PT1 integer-truncation fix"
```

---

## Task 9: Full 99-dir differential + the six gates

**Goal:** Confirm the whole suite is green on the frozen HEAD (the controller re-runs this at stage-close per `feedback_subagent_autocommit_claudemd`).

- [ ] **Step 1: The six gates**

```bash
go build ./... && echo BUILD_OK
gofmt -l . | grep -v '^$' && echo "GOFMT DRIFT" || echo GOFMT_CLEAN
golangci-lint run ./... 2>&1 | tail -5
go vet ./... 2>&1 | tail -5
go mod tidy -diff && echo TIDY_EMPTY
go test ./... -count=1 2>&1 | tail -20
```
Expected: build clean; gofmt clean; lint clean; vet clean; `go mod tidy -diff` empty; all tests PASS.

- [ ] **Step 2: Full-package `-race`** (`reference_full_suite_race_after_background_mutator` — health-check goroutines are background mutators)

```bash
go test ./internal/cluster/... -race -count=1 2>&1 | tail -10
go test ./test/differential/ -race -count=1 2>&1 | tail -10
```
Expected: `-race` clean across the FULL packages (not a `-run` subset).

- [ ] **Step 3: Full differential suite** (fixtures 99)

```bash
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l   # expect 99
go test ./test/differential/ -count=1 2>&1 | tail -30
```
Expected: 99 dirs; all PASS. (An UNRELATED `subject ready: EOF` on a full-suite run is a startup race — isolate-re-run + full re-run to distinguish from a regression, `reference_differential_fullsuite_startup_flake`.)

- [ ] **Step 4: Commit (LOCAL-ONLY, if any gate produced a fixup)**

```bash
git commit -am "phase 54 Task 9: six gates green + full 99-dir differential + full-package -race clean" || echo "no fixup needed"
```

---

## Task 10: ADR-0271 body + BEHAVIOR_CONTRACT delta (ADR-0044 / ADR-0052)

**Goal:** Land the ADR-0271 §Decision/§Consequences (§Context already DRAFTED at SPEC §13) and the `BEHAVIOR_CONTRACT.md` panic-construct delta, atomically.

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (the full ADR-0271 entry), `docs/envoy-go/BEHAVIOR_CONTRACT.md`

- [ ] **Step 1: ADR-0271** — append after `## ADR-0270`. §Context = SPEC §13's DRAFT (verbatim). §Decision: the floored-integer-threshold + integer-cross-multiply comparison (AMEND-PT1); the `panicThresholdPercent int` stored type (D-PT-STORE); the removed outer `panicInc()` (AMEND-PT2); the `tierHealth`-relocated + `healthLeafFactory`-shared locality retrofit (AMEND-PT3, D-PT-TIERHEALTH-HOME) + the subset-unchanged asymmetry; the standalone out-of-range reject (AMEND-PT4, D-PT-REJECT-PLACEMENT). §Consequences: ZERO new stats/seam/package/dep/fuzzer/BackendKind; the multi-tier-priority INERTNESS coverage boundary (phase-53 AMEND-P1); the degraded-host panic-term coverage boundary (no degraded state, ADR-0242); ADR-0024 UNAMENDED.

- [ ] **Step 2: BEHAVIOR_CONTRACT.md** — add the panic-construct delta (SPEC §9): the strict-`<` FLOORED-integer comparison via integer cross-multiply; disable-at-0 / absent-defaults-50% presence semantics; the in-panic all-hosts distribution; `lb_healthy_panic` once-per-pick (incl. the corrected locality-weighted path); the per-locality no-local-panic guarantee + the per-subset local-panic behavior; the out-of-range `[0,100]` reject; the confirmed-correct INERTNESS of the classic threshold in a multi-tier-priority cluster.

- [ ] **Step 3: Verify DECISIONS tail + commit (LOCAL-ONLY)**

```bash
grep -nE '^## ADR-0[0-9]+' docs/envoy-go/DECISIONS.md | tail -2   # expect ...ADR-0270 then ADR-0271
git add docs/envoy-go/DECISIONS.md docs/envoy-go/BEHAVIOR_CONTRACT.md
git commit -m "phase 54 Task 10: ADR-0271 (healthy_panic_threshold proof + four corrections) §Decision/§Consequences + BEHAVIOR_CONTRACT panic-construct delta (ADR-0044/ADR-0052) — DECISIONS tail ADR-0270 -> ADR-0271"
```

---

## Task 11: Completion bundle — STATE / ROADMAP advance, row-54 `done`, the Load-balancing family CLOSE (CONTROLLER, at stage-close)

**Goal:** Advance the project ledgers and CLOSE the Load-balancing family. Performed by the CONTROLLER at stage-close (NOT a Task-11 subagent — the phase-52/53 Task-11 precedent), AFTER the squash + the six-gate re-run on the frozen HEAD.

**Files:**
- Modify: `docs/envoy-go/STATE.md`, `docs/envoy-go/ROADMAP.md`

- [ ] **Step 1: STATE.md** — active-phase header → phase 54 IMPL done; counts: fixtures **99**, fuzzers **52**, stat surface **1200**, BackendKind **38**, DECISIONS tail **ADR-0271**; append the phase-54 history line.

- [ ] **Step 2: ROADMAP.md** — row 54 `in-progress → done` (lifecycle 2 → 3); the Load-balancing family section CLOSES (its candidate list empties — the FOURTH family to close after HTTP-filters @ 25.3 / Network-filters @ 33 / Upstream-robustness @ 43; NO parent rollup — a single flat row per ADR-0106, the `reference_roadmap_split_phase_row_done` disposition N/A here as this is a single flat row).

- [ ] **Step 3: Final six-gate re-run on the frozen HEAD + squash + push** (CONTROLLER)

```bash
go build ./... && gofmt -l . && golangci-lint run ./... && go vet ./... && go mod tidy -diff && go test ./... -count=1
ls -d test/fixtures/[0-9][0-9][0-9][0-9]* | wc -l                          # 99
grep -rh '^func Fuzz' --include='*.go' --exclude-dir=.worktrees . | wc -l   # 52
```
Then squash-merge the worktree branch to master, push to origin (`feedback_push_to_origin`), remove the worktree, and roll `next-prompt.txt` forward to the phase-54 IMPL → (on IMPL-done) the TERMINATION SENTINEL re-check (phase 54 is the last chartered LB row; re-verify Observability + Operational-tooling deferred lists before any `stop`).

---

## Self-review — spec coverage, placeholders, type consistency

**1. Spec coverage** (SPEC §1.1 AMEND-PT1..PT6 + §3–§8):
- AMEND-PT1 (floor + integer cross-multiply + D-PT-STORE) → Task 2. ✓
- AMEND-PT4 (out-of-range reject, D-PT-REJECT-PLACEMENT) → Task 4. ✓
- AMEND-PT3 (locality retrofit, D-PT-TIERHEALTH-HOME; subset-unchanged) → Task 5. ✓
- AMEND-PT2 (double-increment) → Task 6. ✓
- AMEND-PT5 (presence semantics already match) → Task 3 (locked in). ✓
- AMEND-PT6 (zero stat delta / envelope) → Tasks 9/11 (counts) + the FINAL ADR-0045 re-check. ✓
- `0097` three-arm differential (§8.1) + breaks → Tasks 7/8. ✓
- ADR-0271 + BEHAVIOR_CONTRACT (§9/§13) → Task 10. ✓
- Counts + family CLOSE (§14) → Task 11. ✓

**2. Placeholder scan:** every code step shows complete code or an exact transformation set; no "TBD"/"handle edge cases"/"similar to Task N". The `0097` driver references the `0096` template for reusable harness scaffolding (topology + constants + assertions given in full) — the established phase-53 Task-8/9 practice. ✓

**3. Type consistency:** `panicThresholdPercent int` (Task 2) is consumed by `inPanic` (Task 2), `tierHealth`'s literal (Tasks 2/5), and the D-PT-STORE call-site sweep. `healthLeafFactory func(sub []Endpoint, h *clusterHealth) (loadBalancer, error)` (Task 5) is consumed by `newLocalityWeightedLB`/`newLocalityWeightedLBWithRNG` (Task 5) + `newPriorityLB`/`newPriorityLBWithRNG` (renamed refs, Task 5) + the `manager.go` closure (Task 5). `validatePanicThresholdRange(c, name) error` (Task 4) is called once early in `buildCluster` (Task 4). `subset.go`'s `leafFactory` is UNTOUCHED (the crux asymmetry). Names match across tasks. ✓

## FINAL ADR-0045 split-gate re-check

**Verdict: NO SPLIT.** 11 tasks; ~150–260 prod LoC (health.go comparison-shape + stored-type + validator + relocated tierHealth ≈ 45; loadbalancer.go factory type ≈ 3; locality.go retrofit + removed increment ≈ 18; manager.go reject call + closure ≈ 5; the `0097` driver ≈ 150–200). Comfortably under the ADR-0045 `>~25 tasks OR >~1500 LoC` gate. The family's SMALLEST phase (no new policy, wrapper, package, seam, `Endpoint` dimension, stat, fuzzer, or BackendKind). A single flat row; a SINGLE ADR (ADR-0271, SPEC §11.7's zero-seam-change finding). No second producer plane or subsystem to couple against (SPEC §3.0/§4). The re-check re-confirms the SPEC §3.0 no-split disposition.
