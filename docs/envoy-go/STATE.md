# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `06-observability-baseline`
- **phase-directory:** `docs/envoy-go/phases/06-observability-baseline/` — does not yet exist. The directory is created at phase-06 brainstorm time (the next session). Phase 05.2 is closed; `docs/envoy-go/phases/05.2-upstream-h2/` joins `docs/envoy-go/phases/05.1-downstream-h2/` as closed read-only history. The parent `docs/envoy-go/phases/05-http-2/` retains its `SPEC.md` as the master design document, now closed. ROADMAP rows 05.2 and 05 are both `done` after this commit.
- **lifecycle-state:** `6` — phase 05.2 complete; ROADMAP rows 05.2 + 05 flipped to `done` together by this commit per 05.2 SPEC §4.4 + PLAN Task 15's "Refinement" note. The REVIEW-followup batch on `phase/05.2-upstream-h2-review-followup` (worktree `.worktrees/phase-05.2-upstream-h2-review-followup`, branched from `b9810ad`) closed both REVIEW Importants (I-1 BEHAVIOR_CONTRACT.md 5-cell rewrite at `d8fa1d8`; I-2 H1-path defensive-stub log + unit test at `1d57b31`) plus three optional Minors (M-2/M-4/M-7 at `635f6a3`); `f774d4f` re-verified gates (b)/(c)/(d)/(e) GREEN with the verification block addendum in PROGRESS.md; `5c0f3cc` promoted STATE to lifecycle-state 5; this commit (`TBD`) is the lifecycle-state 6 phase-done commit. Per BOOTSTRAP §5 step 6 → step 1, the next session is phase-06 brainstorm and lifecycle-state resets to `0`-equivalent (cold-start for the new phase).
- **next-skill:** `superpowers:brainstorming` — phase 06 (`observability-baseline`) brainstorm. Per ROADMAP row 06: "Access log (file sink, Envoy default format) + stats + Prometheus admin endpoint. Access log + Prometheus fixtures green." Inputs the brainstorm should consider: (a) the four ADR-0058 / 05.2 carry-forwards: M-4 (`readClientPreface` not ctx-aware → phase-06-or-07-must-consider), M-10 (`SETTINGS_TIMEOUT` absent → phase-06-or-08-must-consider), M-12 (`closedStreams` map unbounded — long-lived-conn hardening, deferred to upstream-robustness family but the planner may bundle if natural), and the seven REVIEW Minors carried forward (M-1 deadline-extension accept-as-is; M-3 ADR-numbering accept-as-is; M-5/M-6/M-8/M-9/M-10 prose/cosmetic — see 05.2 REVIEW.md "Findings"); (b) the phase-04 access-log surface introduced for routed-to-upstream H1 (PROGRESS lines TBD); (c) the SPEC §13 acceptance bullet 14 commit-message-completeness check now satisfied (the 05.2 phase-done commit names all four ADRs).
- **next-skill-scope:** Phase 06 brainstorm session (lifecycle-state 0 → 1 cold-start). Per ADR-0006 + BOOTSTRAP §5: brainstorm with `superpowers:brainstorming` to scope phase 06's deliverables (access log file sink + Envoy default format + stats subsystem + Prometheus admin endpoint per ROADMAP row 06), then advance to lifecycle-state 1 (`superpowers:writing-plans` for SPEC.md authoring) and 2 (PLAN.md authoring). Worktree for the next session: branch from this commit's HEAD per ADR-0003 + per-phase-worktree convention (`.worktrees/phase-06-observability-baseline`).
- **last-commit:** `TBD` — `phase 05.2: phase-done — upstream HTTP/2 lands; ROADMAP rows 05.2 + 05 → done [ADR-0055, ADR-0056, ADR-0057, ADR-0058]`, on branch `phase/05.2-upstream-h2-review-followup`. Flips ROADMAP row 05.2 status `in-progress` → `done` AND parent row 05 (`http-2`) status `in-progress` → `done` together per 05.2 SPEC §4.4 + PLAN Task 15's "Refinement" note. Updates STATE.md `lifecycle-state` 5 → 6 with `active-phase` advanced to `06-observability-baseline` and `next-skill` set to `superpowers:brainstorming` per BOOTSTRAP §5 step 6 → step 1 transition. Names all four ADRs introduced in 05.2 per BOOTSTRAP §5.3: ADR-0055 (flow-control discipline; landed at Task 5 in commit `bef7a1e`), ADR-0056 (per-request fresh upstream H2 dial; landed at Task 9 in commit `344c371`), ADR-0058 (trailers observed but not forwarded + M-4/M-10 carry-forwards; landed at Task 11 in commit `dd30a4c`), ADR-0057 (closes ADR-0035 H2 leg; landed at Task 14 in commit `75d311b`). The commit-time order is non-monotonic (0055 → 0056 → 0058 → 0057) per the topical-vs-commit-order rationale documented in 05.2 PLAN.md "ADRs introduced by this plan" section. SHA-fill follow-up per phase-02..05.1 convention.
- **last-updated:** 2026-04-26

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
