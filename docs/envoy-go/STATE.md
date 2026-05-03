# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `09-http-filter-fault` (first concrete phase under BOOTSTRAP_PROMPT.md §9 HTTP filters family; ROADMAP row 09 is `planned` at this commit; brainstorm scoped to phase 09 is committed at `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md`).
- **phase-directory:** `docs/envoy-go/phases/09-http-filter-fault/` — created at this commit; contains `BRAINSTORM.md` only. The next session creates `SPEC.md`, `PLAN.md`, `PROGRESS.md` per the lifecycle-state machine.
- **lifecycle-state:** `1` for phase 09 (per BOOTSTRAP §5 — Phase in ROADMAP, BRAINSTORM exists, no SPEC yet). The next session's first action: `superpowers:writing-plans` to draft `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` per the brainstorm + execute the §11 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.
- **next-skill:** `superpowers:writing-plans` — SPEC-drafting session for phase 09 fault filter. Inputs: `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md` (§§1-12; §11 enumerates the empirical-pin obligations); `docs/envoy-go/BEHAVIOR_CONTRACT.md` (## HTTP filter chain umbrella + ## Stat-name mapping table to be extended); `docs/envoy-go/ROADMAP.md` row 09 (the phase boundary); `internal/filter/http/` (07.1 framework surface); `internal/filter/http/cors/cors.go` (the cors precedent the SPEC author follows); the just-closed 08.2 SPEC + PLAN + PROGRESS + REVIEW artefacts (the brainstorm-→ SPEC structural precedent).
- **next-skill-scope:** Lifecycle-state 1 → 2 deliverable: `docs/envoy-go/phases/09-http-filter-fault/SPEC.md`. The SPEC author MUST: (a) author 8 ADRs ADR-0100 through ADR-0107 per BRAINSTORM §9; (b) execute §11 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 (8 pins covering abort body shape, delay timing, header-driven edge cases, stat names, per-route wholesale-override, headers-field match semantics); (c) populate `BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.fault` subsection + `## Stat-name mapping` 4-counter extension; (d) define differential fixture `0011-http-fault` topology + 5 scenarios + driver shape; (e) gate the SPEC's task-count + LoC estimate against ADR-0045's surface-split policy (if > ~25 tasks or > ~1500 LoC est., split into 09.1 + 09.2 per BRAINSTORM §1.4 split-readiness map). After SPEC, advance STATE to lifecycle-state 2 with `next-skill: superpowers:writing-plans` for PLAN drafting.
- **last-commit:** `<TBD — fill at the brainstorm-merge commit>` — `phase 09 brainstorm: scope envoy.filters.http.fault as first §9 HTTP filters family phase`. Lands `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md` (12 sections; ~700 lines mirroring the 08.2 BRAINSTORM precedent at a single-filter scope) + adds ROADMAP row 09 (status `planned`) + advances STATE.md to lifecycle-state 1 next-skill writing-plans. SHA filled in a follow-up commit per the phase-04..08.1..08.2 SHA-fill convention.
- **last-updated:** 2026-05-03

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
