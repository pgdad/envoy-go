# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `09-http-filter-fault` (first concrete phase under BOOTSTRAP_PROMPT.md §9 HTTP filters family; ROADMAP row 09 status `in-progress` since SPEC commit `da29807`; SPEC committed at `docs/envoy-go/phases/09-http-filter-fault/SPEC.md`; brainstorm at `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md`; PLAN committed at `docs/envoy-go/phases/09-http-filter-fault/PLAN.md` AT this commit).
- **phase-directory:** `docs/envoy-go/phases/09-http-filter-fault/` — contains `BRAINSTORM.md`, `SPEC.md`, and `PLAN.md` at this commit. The next session creates `PROGRESS.md` (Task 1) per the lifecycle-state machine.
- **lifecycle-state:** `3` for phase 09 (per BOOTSTRAP §5 — SPEC + PLAN exist, PROGRESS does not). The next session's first action: `superpowers:subagent-driven-development` (per the user's persistent preference for subagent-driven over inline execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`) to execute `docs/envoy-go/phases/09-http-filter-fault/PLAN.md` task-by-task. Alternative: `superpowers:executing-plans` for inline execution. The PLAN's 17 tasks decompose SPEC §4 deliverables under TDD discipline; each task lists Files / Precondition / Artifact / Acceptance / numbered Steps / Anchored cross-reference. Production-only LoC estimate ~430 (well below ADR-0045's ~1500 threshold); task count 17 (well below ADR-0045's ~25 threshold).
- **next-skill:** `superpowers:subagent-driven-development` — PLAN-execution session for phase 09 fault filter. Inputs: `docs/envoy-go/phases/09-http-filter-fault/PLAN.md` (THE authoritative implementation source — 3159 lines, 17 tasks, **read Task 1 in full + the headed sections "## Planner-time deferred-decision resolution" + "## ADRs introduced by this plan" + "## Execution preconditions" before starting**); `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` (cross-reference for every task's Anchored line); `docs/envoy-go/phases/09-http-filter-fault/BRAINSTORM.md` (consult when SPEC's "what" needs BRAINSTORM's "why"); `docs/envoy-go/phases/08.2-graceful-drain/{PLAN,PROGRESS,REVIEW}.md` (closed read-only history; the structural precedent for PROGRESS.md per-task entries); `internal/filter/http/cors/cors.go` (the package-shape precedent fault inherits verbatim).
- **next-skill-scope:** Lifecycle-state 3 → 4 (PLAN execution): land Tasks 1–14 (production code + fixture); state stays at 3 throughout. Lifecycle-state 4 → 5 (verification): land Tasks 15–16 (BEHAVIOR_CONTRACT + ADRs + six-gate verification + phase-done commit). Lifecycle-state 5 → 6 (review): land Task 17 (REVIEW.md per requesting-code-review skill). After phase-done, STATE.md flips to `awaiting next planning` per `BOOTSTRAP_PROMPT.md` §5 lifecycle; `next-skill: superpowers:brainstorming` against §9's HTTP filters family for the next family-child (per ADR-0106 — flat top-level rows; no sibling-stub authored).
- **last-commit:** `b963c1b` — `phase 09 PLAN: 17 tasks decomposing SPEC §4 deliverables [ADR-0100..ADR-0107 anchored]`. Lands `docs/envoy-go/phases/09-http-filter-fault/PLAN.md` (3159 lines mirroring the 08.2 PLAN precedent at single-filter scope) + advances STATE.md to lifecycle-state 3 next-skill subagent-driven-development (PLAN execution). The PLAN settles SPEC §12's 10 deferred decisions + 3 PLAN-emerged decisions (FactoryCtx framework extension as ADR-0100 first-use consequence; fixture-path correction `test/fixtures/0011-http-fault/`; BackendKind enum naming). SHA filled in a follow-up commit per the phase-04..08.2 SHA-fill convention.
- **last-updated:** 2026-05-03

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
