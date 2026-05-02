# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `08.2-graceful-drain`
- **phase-directory:** `docs/envoy-go/phases/08.2-graceful-drain/` — currently contains the SPEC stub `README.md` from the 08.1 SPEC commit (`1f85b07`). The 08.2 BRAINSTORM session populates this directory with `BRAINSTORM.md` per `superpowers:brainstorming`. Phase 08.1 (sub-phase) closes at THIS commit; the parent ROADMAP row `08` STAYS `in-progress` (flips at 08.2 phase-done per parent SPEC §5). All earlier phases (00–07.2) remain closed read-only history.
- **lifecycle-state:** `0` — phase 08.1 phase-done at the commit named in `last-commit`. Next session is 08.2 BRAINSTORM (state 0 → 1) per `superpowers:brainstorming`.
- **next-skill:** `superpowers:brainstorming` — autonomous brainstorm session for 08.2 (per ADR-0004's hard-gate discipline). Inputs: `docs/envoy-go/phases/08-admin-api-and-drain/{SPEC.md, BRAINSTORM.md}` (parent master SPEC + master BRAINSTORM context); `docs/envoy-go/phases/08.2-graceful-drain/README.md` (sibling SPEC stub from the 08.1 SPEC commit `1f85b07`); `docs/envoy-go/phases/08.1-admin-endpoints/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (just-closed sub-phase artefacts; the 08.2 brainstorm builds on 08.1's admin-mux scaffold + ADR-0085's constructor-widening pattern + ADR-0088's `LIVE`/`PRE_INITIALIZING` state-enum coverage that 08.2 extends with `DRAINING`).
- **next-skill-scope:** Lifecycle-state 0 → 1 BRAINSTORM session deliverables: `docs/envoy-go/phases/08.2-graceful-drain/BRAINSTORM.md` — full autonomous-brainstorm artefact mirroring 06.2 + 07.2 BRAINSTORM shape; populates §1 mission + §2 design dimensions + §3 surface inventory + §4 carry-forward + §5 per-endpoint flow + §6 contract surface + §7 fixture design + §8 testing + §9 ADR anticipation + §10 carry-forward + §11 empirical-pin obligations (per ADR-0004 hard gate). After BRAINSTORM, advance STATE to lifecycle-state 1 with `next-skill: superpowers:writing-plans` for 08.2 SPEC drafting.
- **last-commit:** `70e6a65` — `phase 08.1: admin-endpoints [ADR-0084, ADR-0085, ADR-0086, ADR-0087, ADR-0088, ADR-0089, ADR-0090]`. Lands the four new admin endpoints + the constructor widening + the differential fixture 0009 + the BEHAVIOR_CONTRACT umbrella restructure + ROADMAP row 08.1 done flip + the seven new ADRs. SHA filled in a follow-up commit per the phase-04..07.2 SHA-fill convention.
- **last-updated:** 2026-05-02

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
