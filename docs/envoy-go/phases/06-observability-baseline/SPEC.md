# Phase 06 — Observability Baseline (parent master SPEC)

**Phase id:** `06`
**Slug:** `06-observability-baseline`
**Status:** `in-progress` (SPEC stage; split into `06.1` + `06.2` per ADR-0045 at brainstorm-close — see `docs/envoy-go/phases/06-observability-baseline/BRAINSTORM.md` §1)
**Produced by:** `superpowers:writing-plans` (lifecycle-state 1 → 2; transcribes the brainstorm-close BRAINSTORM.md into formal SPEC shape)
**Depends on:** phase 05 (done at master `75a6bf9`)
**Sub-phases:** `06.1-stats-prometheus`, `06.2-access-log`
**Differential surface at end of phase:** ROADMAP rows `06.1` and `06.2` are both `done`; the parent row `06` flips `in-progress → done` AT THE SAME phase-done commit as `06.2` (mirroring the 05 / 05.1 / 05.2 pattern from `STATE.md`'s closure of phase 05). Cumulatively across the two sub-phases: NEW differential fixtures `0005-prometheus-stats` (06.1) and `0006-access-log` (06.2) are differentially green; pre-existing fixtures `0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`, `0003-http11-routing`, `0004-h2-routing` remain green; the h2spec conformance gate (c) at the ADR-0051 pin is unchanged at 53/53 PASS; one new fuzzer per sub-phase (`FuzzPromTextFormat` in 06.1; access-log-format fuzzer name TBD in 06.2) runs clean at the 30s ADR-0018 budget.

---

## 1. Mission summary

Phase 06 lands envoy-go's observability baseline: a thin internal stats subsystem with a Prometheus scrape adapter (sub-phase 06.1) and an access-log file sink in Envoy's default format (sub-phase 06.2). The brainstorm-close BRAINSTORM.md (§1) split the original ROADMAP row `06` into two sub-phases at planner-time per ADR-0045 + `BOOTSTRAP_PROMPT.md` §6.2; this parent SPEC carries the cross-cutting design that applies to BOTH sub-phases and points downward to each sub-phase's authoritative SPEC for the per-surface detail. The two sub-phases ship in order (06.1 first, 06.2 second per the BRAINSTORM §1 dependency analysis); each has its own differential fixture, its own ADR set, and its own BEHAVIOR_CONTRACT placeholder population.

After phase 06, the project has proven its seventh central engineering claim: *envoy-go emits behaviorally-equivalent operator-grade signals under a defined load — counters and gauges visible at `/stats/prometheus`, access-log records to a configured file sink — making the proxy auditable from outside the process without coupling to any third-party observability dependency.*

---

## 2. Sub-phase scope summary

Per BRAINSTORM §1's split table:

| Sub-phase ID | Title | Scope | Differential fixture |
|---|---|---|---|
| **06.1** | `stats-prometheus` | `internal/stats` package (atomic-counter Registry + Prometheus text-format adapter); `/stats/prometheus` admin endpoint extension; ~17 stat-emit call sites across listener, HCM, cluster; M-9 carry-forward log-line bundled from 05.2 REVIEW. No third-party Prometheus dependency. | `test/differential/0005-prometheus-stats/` |
| **06.2** | `access-log` | `internal/accesslog` package (file sink + Envoy default-format formatter + async writer); HCM access-log emit hooks; `BEHAVIOR_CONTRACT.md ## Access log field mapping` populated. | `test/differential/0006-access-log/` |

The two sub-phases share no production-code surface (stats package vs. access-log package, by construction); their dependency profile is one-directional and weak — 06.2 may optionally emit a buffer-pressure gauge through 06.1's stats Registry, but this is not a required deliverable of 06.2. 06.1 ships first.

The authoritative scope detail lives in each sub-phase's `SPEC.md`:

- `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` (full sub-phase SPEC; this commit drafts it).
- `docs/envoy-go/phases/06.2-access-log/README.md` (sibling SPEC stub; this commit drafts it; superseded by the 06.2 SPEC drafted at 06.2's lifecycle-state 1).

---

## 3. Split rationale

Per BRAINSTORM §1, the original ROADMAP row `06` bundled three deliverables: access log (file sink + Envoy default format) + stats + Prometheus admin endpoint. The brainstorm-close decision was that stats + Prometheus form one coherent unit (atomic-counter Registry → Prometheus text-format exporter walking the Registry; one shared package; one shared admin endpoint) while access-log is a separate filter-chain integration with its own format string, async I/O discipline, and `BEHAVIOR_CONTRACT.md` subsection. Bundling them in one phase risked bloating the SPEC and review surface beyond what the 05.1/05.2 cadence established as reviewable. The 05.1/05.2 precedent — where planner-time-split kept each sub-phase under its own SPEC, PLAN, REVIEW — is the explicit template phase 06 mirrors here.

The split is planner-time (ADR-0045), not mid-execution; the BRAINSTORM session resolved the split before SPEC drafting began. No splitting ADR is anticipated for phase 06 — ADR-0045 already authorizes the discipline; this parent SPEC and the two sub-phase artifacts are the discipline's expression.

---

## 4. Cross-cutting decisions (apply to BOTH 06.1 and 06.2)

### 4.1 BEHAVIOR_CONTRACT.md placeholders

`docs/envoy-go/BEHAVIOR_CONTRACT.md` already has empty `## Stat-name mapping` (line 48) and `## Access log field mapping` (line 56) placeholders, scaffolded at phase 00. Phase 06's two sub-phases each populate one:

- **06.1** populates `## Stat-name mapping` (the 17-name table from BRAINSTORM §2.3 + the SN1-SN8 flattening rules from BRAINSTORM §7.1) plus a new equivalence-matrix row for "Stats output" (per BRAINSTORM §7.3).
- **06.2** populates `## Access log field mapping` (per the 06.2 sub-phase SPEC, which is drafted at lifecycle-state 1 of that sub-phase).

Both populations are in-place edits per the ADR-0052 precedent (the same authorization 05.1's BEHAVIOR_CONTRACT scaffold + 05.2's in-place edit relied on). No new ADR is required to authorize this in-place edit pattern; ADR-0052 is the durable record.

### 4.2 Equivalence-matrix dimensions

BRAINSTORM §7.3 (06.1) introduces a "Stats output" dimension to the equivalence matrix at `BEHAVIOR_CONTRACT.md ## Equivalence Matrix`. 06.2 is expected to introduce or extend an "Access log records" dimension (the seed row already exists in the matrix but is forward-looking). The two new/extended dimensions are landed in their respective sub-phases, NOT in this parent SPEC.

### 4.3 No third-party observability dependencies

Per BRAINSTORM §2.1 (06.1's stats backend architecture) and the implicit symmetry for 06.2's access-log: no third-party Prometheus library (no `prometheus/client_golang`), no third-party access-log library. Both sub-phases own their writers/formatters in-tree. Rationale (per BRAINSTORM §2.1): future Observability-family phases (gRPC ALS, OTLP, statsd, OTel tracing) all need to hook a registry and a formatter — investing in our own thin shapes now is the same architectural choice Envoy made in `source/common/stats/` and `source/common/access_log/` for the same forward-compat reason.

### 4.4 Doctrine inheritance

Both sub-phases honor doctrine `D-3.2` (hybrid implementation stance — codec/wire-format primitives may be vendored; runtime constructs are not), `D-3.3` (differential correctness beats internal fidelity), `D-3.4` (context isolation via written ADRs), `D-3.6` (every phase is a green build), and the `BOOTSTRAP_PROMPT.md` §7.5 six-gate phase-done checklist. Both sub-phases' SPECs specialize §7.5 the way 05.1 and 05.2 did.

### 4.5 Lifecycle-state pattern

Both sub-phases follow the 05.1 / 05.2 lifecycle pattern: BRAINSTORM-close → SPEC.md → PLAN.md → executing-plans → verification-before-completion → REVIEW.md → phase-done commit. ROADMAP row flips for each sub-phase happen per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3 (`planned → in-progress` at the SPEC commit, `→ done` at the phase-done commit).

---

## 5. Phase-done gate (parent)

Per `BOOTSTRAP_PROMPT.md` §7.5 + the parent-rollup discipline established by phase 05 (parent row `05-http-2` flipped to `done` AT THE SAME phase-done commit as `05.2`'s phase-done): **the parent row `06-observability-baseline` is `done` only when both `06.1-stats-prometheus` AND `06.2-access-log` are `done`.** Concretely:

1. ROADMAP row `06.1` flips `planned → in-progress` at 06.1's SPEC commit (per the 06.1 SPEC §3 / `BOOTSTRAP_PROMPT.md` §4.1 invariant 3).
2. ROADMAP row `06.1` flips `in-progress → done` at 06.1's phase-done commit. ROADMAP row `06` and row `06.2` are unchanged at this commit (parent stays `in-progress` because 06.2 is still `planned`; row `06.2` is still `planned` because its SPEC has not been drafted yet).
3. ROADMAP row `06.2` flips `planned → in-progress` at 06.2's SPEC commit.
4. ROADMAP row `06.2` flips `in-progress → done` AT THE SAME COMMIT as the parent row `06`'s `in-progress → done` flip — the 06.2 phase-done commit closes both rows in one operation.

This mirrors the 05 / 05.1 / 05.2 pattern recorded in `STATE.md` at the master `75a6bf9` SHA (05.2's phase-done SHA-fill commit) and in the 05.2 SPEC §4.4 + acceptance-checklist bullet "Row 05 (parent): `in-progress → done` AT THE SAME phase-done commit". The parent SPEC inherits the discipline; the two sub-phase SPECs do the per-row work.

The 06.2 phase-done commit's commit-message body must explicitly name both ROADMAP-row transitions (`06.2 → done` AND `06 → done`) so the rollup is grep-verifiable from `git log`.

The six-gate set per `BOOTSTRAP_PROMPT.md` §7.5 applies to each sub-phase independently. The parent rollup adds nothing beyond the two sub-phases' respective gates being green.

---

## 6. References

- **BRAINSTORM:** `docs/envoy-go/phases/06-observability-baseline/BRAINSTORM.md` (the authoritative design source for both 06.1 and 06.2; this parent SPEC and the sub-phase SPECs distill it into formal SPEC shape).
- **06.1 SPEC:** `docs/envoy-go/phases/06.1-stats-prometheus/SPEC.md` (full sub-phase SPEC; lifecycle-state 2 deliverable for 06.1).
- **06.2 SPEC stub:** `docs/envoy-go/phases/06.2-access-log/README.md` (sibling placeholder; superseded by the 06.2 SPEC drafted at 06.2's lifecycle-state 1).
- **Parent-master-SPEC precedent:** `docs/envoy-go/phases/05-http-2/SPEC.md` (the structural template this SPEC mirrors — terser than a sub-phase SPEC, summarizes both sub-phases, defers per-surface detail to each sub-phase).
- **Sub-phase SPEC precedent:** `docs/envoy-go/phases/05.1-downstream-h2/SPEC.md` and `docs/envoy-go/phases/05.2-upstream-h2/SPEC.md` (the structural templates the 06.1 SPEC mirrors).
- **BOOTSTRAP_PROMPT cross-references:** §5 (Phase Lifecycle State Machine — parent / sub-phase relationship), §6.2 (How to split — planner-time-split discipline), §7.5 (Phase-done gate — six-gate checklist), §4.1 (artifact-layout invariants — ROADMAP row flips at SPEC commit / phase-done commit).
- **ENVOY_TARGET pin:** `docs/envoy-go/ENVOY_TARGET.md` — `envoyproxy/envoy:v1.37.2` at `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`. Both sub-phases' differential fixtures run against this pin. The 06.1 SPEC §"Stat-name mapping rules" cites this SHA in its Rule SN4 empirical-verification gate.
- **DECISIONS.md:** ADR-0045 (planner-time-split discipline), ADR-0051 (h2spec pin SHA — referenced for gate (c) carry-through in both sub-phases), ADR-0052 (BEHAVIOR_CONTRACT in-place edit authorization — both sub-phases inherit). New ADRs anticipated for 06.1 (six per BRAINSTORM §8); the 06.2 SPEC anticipates its own ADRs at its drafting time.
- **ROADMAP.md:** rows `06`, `06.1`, `06.2` per the split landed in this commit's ROADMAP edit.
