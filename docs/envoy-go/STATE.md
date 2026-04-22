# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `00-bootstrap`
- **phase-directory:** `docs/envoy-go/phases/00-bootstrap/`
- **lifecycle-state:** `5` — Verified, not reviewed. Next session runs `superpowers:requesting-code-review`.
- **next-skill:** `superpowers:requesting-code-review`
- **next-skill-scope:** Request code review against PLAN.md, SPEC.md §12 acceptance checklist, doctrine D-3.1–D-3.7, and the phase-done gate (SPEC §3 / §7.5). Produce `docs/envoy-go/phases/00-bootstrap/REVIEW.md` with an explicit approval line or enumerated findings. If REVIEW.md approves, advance lifecycle-state to 6 and hand off to the final-commit step (ROADMAP row 00 → done, STATE advanced to phase 01). If REVIEW.md raises issues, re-enter at lifecycle-state 3 (not 4) per BOOTSTRAP_PROMPT §5.2, scoped to the specific findings.
- **last-commit:** (set by verification commit)
- **last-updated:** 2026-04-22

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
