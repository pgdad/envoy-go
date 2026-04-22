# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `00-bootstrap`
- **phase-directory:** `docs/envoy-go/phases/00-bootstrap/`
- **lifecycle-state:** `3` — SPEC.md and PLAN.md exist (PLAN.md approved across six iterations of the `plan-document-reviewer` subagent loop per ADR-0005); implementation incomplete
- **next-skill:** `superpowers:subagent-driven-development` (per ADR-0005, the user's standing preference for execution; the executing session may override only with an ADR)
- **next-skill-scope:** execute the 16 tasks in `docs/envoy-go/phases/00-bootstrap/PLAN.md` task-by-task with `superpowers:test-driven-development` discipline per task. Follow PLAN's "Execution preconditions" section first: create worktree `.worktrees/phase-00-bootstrap-impl` on a new branch `phase/00-bootstrap-impl` off `master`, verify Docker / Go 1.23 / golangci-lint, then enter Task 1. Append to `docs/envoy-go/phases/00-bootstrap/PROGRESS.md` after every task. The plan's tasks 4 and 10–13 require Docker; task 16 wants a green CI run (or a documented local equivalent if no remote is configured).
- **last-commit:** the phase-00 PLAN commit on branch `phase/00-bootstrap-plan` (fast-forwarded into `master` at session exit per ADR-0003). Resolve via `git log -1 --format=%H docs/envoy-go/phases/00-bootstrap/PLAN.md` when needed.
- **last-updated:** 2026-04-21

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
