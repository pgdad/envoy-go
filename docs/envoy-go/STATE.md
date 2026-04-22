# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `00-bootstrap`
- **phase-directory:** `docs/envoy-go/phases/00-bootstrap/`
- **lifecycle-state:** `2` — SPEC.md exists (approved by `spec-document-reviewer` subagent per ADR-0004), PLAN.md does not
- **next-skill:** `superpowers:writing-plans`
- **next-skill-scope:** produce `docs/envoy-go/phases/00-bootstrap/PLAN.md` implementing the SPEC. Apply §5 step 2 GATE: if PLAN.md exceeds ~25 tasks or ~1500 LoC net change, split phase 00 into sub-phases (§6) and exit.
- **last-commit:** the phase-00 SPEC commit on branch `bootstrap` (and merged into `master` on the same session). Resolve via `git log -1 --format=%H docs/envoy-go/phases/00-bootstrap/SPEC.md` when needed.
- **last-updated:** 2026-04-21

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
