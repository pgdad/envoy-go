# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `09-http-filter-fault` (first concrete phase under BOOTSTRAP_PROMPT.md §9 HTTP filters family; ROADMAP row 09 flips `planned → in-progress` AT this commit; SPEC committed at `docs/envoy-go/phases/09-http-filter-fault/SPEC.md`; brainstorm at `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md`).
- **phase-directory:** `docs/envoy-go/phases/09-http-filter-fault/` — contains `BRAINSTORM.md` and `SPEC.md` at this commit. The next session creates `PLAN.md`, `PROGRESS.md` per the lifecycle-state machine.
- **lifecycle-state:** `2` for phase 09 (per BOOTSTRAP §5 — SPEC exists, PLAN does not). The next session's first action: `superpowers:writing-plans` to draft `docs/envoy-go/phases/09-http-filter-fault/PLAN.md` per the SPEC. The SPEC's 1297 lines + the 8 anticipated ADRs (ADR-0100..ADR-0107 per SPEC §8; ADR-0104 repurposed to deferral per §11.5) are the authoritative input for PLAN authoring. Per ADR-0045 surface-split policy, the planner gates the PLAN's task count + LoC estimate; if > ~25 tasks OR > ~1500 LoC est., split into 09.1 + 09.2 per BRAINSTORM §1.4 split-readiness map.
- **next-skill:** `superpowers:writing-plans` — PLAN-drafting session for phase 09 fault filter. Inputs: `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` (THE authoritative source — every PLAN task traces to one or more SPEC sections; 1297 lines, 16 sections, read in full); `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md` (§§1–12; §11.5 amendment explained in SPEC §1.1 — request-header path is DROPPED from MVP scope; ADR-0104 repurposed); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (## HTTP filter chain at line 695 hosts the new `### envoy.filters.http.fault` subsection per SPEC §13.1; ## Stat-name mapping ### 17-name table at line 130 extends to 22 names per §13.2; ## Timing tolerances at line 266 gains a fault-delay-accuracy bullet per §13.3); `internal/filter/http/cors/cors.go` (the cors precedent the PLAN author follows for filter package shape); the just-closed 08.2 PLAN + PROGRESS + REVIEW artefacts (the SPEC-→-PLAN structural precedent).
- **next-skill-scope:** Lifecycle-state 2 → 3 deliverable: `docs/envoy-go/phases/09-http-filter-fault/PLAN.md`. The PLAN author MUST: (a) decompose SPEC §4 deliverables into bite-sized TDD tasks (~15–25 expected per ADR-0045 estimate; if > 25, ADR-0045 split per BRAINSTORM §1.4 → 09.1 + 09.2); (b) anchor each ADR-0100..ADR-0107 to a specific PLAN task per the executing-plans PROGRESS preamble convention; (c) name verification commands for each task per the test-driven-development discipline; (d) include a fixture-build task block for `test/differential/0011-http-fault/` (envoy.yaml + envoy-go.yaml + driver/driver.go + backends/backend.go + expectations.yaml + README.md per SPEC §7); (e) settle the §12 deferred decisions (most are pre-recommended in SPEC §12; the planner just confirms or overrides). After PLAN, advance STATE to lifecycle-state 3 with `next-skill: superpowers:executing-plans` (or subagent-driven-development per the user's persistent preference) for PLAN execution.
- **last-commit:** `TBD` — `phase 09: SPEC for envoy.filters.http.fault [ADR-0100..ADR-0107 anticipated]`. Lands `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` (1297 lines mirroring the 08.2 SPEC precedent) + flips ROADMAP row 09 status `planned → in-progress` + advances STATE.md to lifecycle-state 2 next-skill writing-plans (PLAN.md drafting). The SPEC's §11 empirical-pin block executed IN-SESSION against `envoyproxy/envoy:v1.37.2 @ sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` per ADR-0004; eight pins resolved with three SPEC-amending surprises documented in SPEC §1.1. SHA filled in follow-up commit per the phase-04..08.1..08.2 SHA-fill convention.
- **last-updated:** 2026-05-03

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
