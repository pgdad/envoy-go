# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `00-bootstrap`
- **phase-directory:** `docs/envoy-go/phases/00-bootstrap/`
- **lifecycle-state:** `4` — Implementation complete, not verified. Next session runs `superpowers:verification-before-completion`.
- **next-skill:** `superpowers:verification-before-completion`
- **next-skill-scope:** Run the full phase-done gate (SPEC §3 and §7.5) in verification mode: go build, go vet, golangci-lint run, go test ./... (green), differential fixture (green), quote all command outputs into PROGRESS.md. Reference already-captured Task 16 outputs. Then advance lifecycle-state to 5 and next-skill to `superpowers:requesting-code-review`.
- **last-commit:** 496484b
- **last-updated:** 2026-04-22

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
