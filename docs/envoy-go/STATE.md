# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `01-static-bootstrap-config`
- **phase-directory:** `docs/envoy-go/phases/01-static-bootstrap-config/` — exists; contains `SPEC.md` and `PLAN.md` as of this commit.
- **lifecycle-state:** `3` — SPEC.md and PLAN.md both exist; PROGRESS.md does not. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 3, the next session runs `superpowers:subagent-driven-development` (per ADR-0005 §4) scoped to executing PLAN.md's 17 tasks in a fresh implementation worktree. Split gate (§6) remains live during execution: if any task's sub-step count exceeds ~10 or net LoC revises past ~1500, stop and split into `01.1`, `01.2`, …
- **next-skill:** `superpowers:subagent-driven-development`
- **next-skill-scope:** Execute `docs/envoy-go/phases/01-static-bootstrap-config/PLAN.md` task-by-task in a fresh worktree on branch `phase/01-static-bootstrap-config-impl` cut off master (per PLAN "Execution preconditions" §1). For each of the 17 tasks: follow its steps in order (TDD where tests exist — failing-test-first, then implementation, then passing test); run the per-task verify command(s) quoted in the task; commit with the exact `git commit -m "..."` string quoted at the task's final step (commit messages end with `[ADR-NNNN]` suffixes where the task lands an ADR); create `PROGRESS.md` during Task 1 and append one entry per task verbatim per the PLAN's "PROGRESS.md conventions" section (Task header `## Task N — <title>` with em-dash, `**Commits:**` / `**Notes:**` / `**Outputs:**` fields, command output quoted verbatim). Every ADR (0012 through 0021) lands in the commit of its first-use task; ADR-0021 explicitly supersedes ADR-0007 without editing ADR-0007 (append-only per D-3.5). On Task 17's completion (all SPEC §3 gates a–e green; gate (f) REVIEW is a later step), advance STATE.md to `lifecycle-state: 4` (verification) with `next-skill: superpowers:verification-before-completion` per ADR-0005 §4, then exit. If during execution the split gate (§6) fires, halt execution, split phase 01 into `01.1/01.2/...` siblings per `BOOTSTRAP_PROMPT.md` §6.2, update ROADMAP parent + child rows, repoint STATE, ADR the split, and exit without committing further tasks. Plan-document-reviewer approved this PLAN on iteration 2 (after fixing the readyAddr-sentinel-parsing bug in Task 12 and an unused-variable footgun in Task 5). Depends-on: phase 00 (done) and phase-01 SPEC (landed on master via `phase/01-static-bootstrap-config-spec`).
- **last-commit:** 82b3cdb
- **last-updated:** 2026-04-22

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
