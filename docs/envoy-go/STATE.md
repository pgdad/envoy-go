# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `05.2-upstream-h2`
- **phase-directory:** `docs/envoy-go/phases/05.2-upstream-h2/` — contains `README.md`, `SPEC.md` (`dacf4b7`), `PLAN.md` (`4c6b6bb`), `PROGRESS.md` (Tasks 1-15 landed; Task 15 executor verification block at `bd75c88`; lifecycle-state 4 verifier verification block at `b34bd99`; REVIEW-followup verification block addendum written by this commit), and `REVIEW.md` (`b9810ad` — APPROVED WITH FOLLOW-UPS, two Importants + ten Minors). The sibling `docs/envoy-go/phases/05.1-downstream-h2/` remains closed read-only history (ROADMAP row `05.1` status `done`). The parent `docs/envoy-go/phases/05-http-2/` retains its `SPEC.md` (`612cdea`) as the master design document. The parent ROADMAP row `05-http-2` stays `in-progress` until 05.2 reaches `done`; the 05.2 phase-done commit will close both rows on the same commit per 05.2 SPEC §4.4 + PLAN Task 15's "Refinement" note.
- **lifecycle-state:** `5` — REVIEW-followup batch complete and re-verified; SPEC §3 phase-done gates (b)/(c)/(d)/(e) re-run green at HEAD `635f6a3` per the verification block addendum at PROGRESS.md (commit `f774d4f`). The REVIEW (`b9810ad`) returned APPROVED WITH FOLLOW-UPS with two Important findings (I-1 BEHAVIOR_CONTRACT.md rows 40-44 5-cell defect; I-2 routerActionH2.do H1-path observability gap) and ten Minor findings; the followup batch on `phase/05.2-upstream-h2-review-followup` closed both Importants plus three optional Minors (M-2 / M-4 / M-7) across three commits: `d8fa1d8` (I-1 BEHAVIOR_CONTRACT 5-cell rewrite per ADR-0052 in-place edit), `1d57b31` (I-2 Path A — log + unit test for the H1-path defensive stub), `635f6a3` (M-2/M-4/M-7 cosmetics bundle); `f774d4f` re-verified gates (b)/(c)/(d)/(e) green and lifted STATE to lifecycle-state 4. The remaining seven Minors are accepted as-is or carry forward to phase 06 per REVIEW.md Recommendation. Gate (f) closed by REVIEW.md `b9810ad` + the PROGRESS verification block addendum. Per BOOTSTRAP §5 the next transition is state 5 → 6 via the phase-done commit which flips ROADMAP rows 05.2 + 05 to `done` together.
- **next-skill:** `subagent-driven-development` — drive the lifecycle-state 5 → 6 phase-done commit. Required edits in a single commit: (1) flip `docs/envoy-go/ROADMAP.md` row 05.2 status `in-progress` → `done`; (2) flip `docs/envoy-go/ROADMAP.md` row 05 (parent) status `in-progress` → `done`; (3) update STATE.md `lifecycle-state` to `6` (or directly to `0` for phase 06 entry per BOOTSTRAP §5 step 6 → step 1 transition) with `active-phase` advanced to phase 06's directory name (per ROADMAP row 06 — title `observability-baseline`). The commit subject MUST name every ADR introduced in 05.2 per BOOTSTRAP §5.3: `phase 05.2: phase-done — upstream HTTP/2 lands; ROADMAP rows 05.2 + 05 → done [ADR-0055, ADR-0056, ADR-0057, ADR-0058]`.
- **next-skill-scope:** Single phase-done commit on this branch: flip ROADMAP rows 05.2 + 05, update STATE.md lifecycle-state + active-phase, and write the commit message naming all four 05.2 ADRs (ADR-0055/0056/0057/0058) per BOOTSTRAP §5.3. After this commit, phase 05.2 is done; the followup branch stays unmerged per per-phase-worktree convention. Phase 06's brainstorm session inherits the ADR-0058 carry-forward tags (M-4 readClientPreface ctx-awareness, M-10 SETTINGS_TIMEOUT) plus the seven REVIEW-deferred Minors (M-1 deadline-extension accept-as-is, M-3 ADR-numbering accept-as-is, M-5/M-6/M-8/M-9/M-10 prose/cosmetic). Re-entry: this is the terminal lifecycle-state for 05.2; no further state-3 re-entry is anticipated.
- **last-commit:** `TBD` — `phase 05.2 review-followup: STATE.md → lifecycle-state 5 (3 → 4 → 5)`, on branch `phase/05.2-upstream-h2-review-followup`. Promotes lifecycle-state 4 → 5 (re-verification at HEAD `635f6a3` captured in PROGRESS verification block addendum at `f774d4f` — all five non-deferred SPEC §3 gates GREEN). No code or PROGRESS edits; STATE-only commit mirroring the 05.1 `3bb1bb9` precedent. Gate (f) closed by REVIEW.md `b9810ad` + the PROGRESS verification block addendum per BOOTSTRAP §5.2 / REVIEW.md Recommendation Path A. The next commit is the state-6 phase-done commit per BOOTSTRAP §5.3, naming ADR-0055/0056/0057/0058 plus ROADMAP rows 05.2 + 05 → done together. SHA-fill follow-up per phase-02..05.1 convention.
- **last-updated:** 2026-04-26

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
