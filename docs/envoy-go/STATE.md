# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `06.2-access-log`
- **phase-directory:** `docs/envoy-go/phases/06.2-access-log/` — exists. SPEC.md committed at `4062c65` + `7bbf4a2` reviewer-fixes. PLAN.md committed at `16366f4`. Implementation session (Tasks 1–16) executed on branch `phase/06.2-access-log-impl` per ADR-0003 + ADR-0005 §4 + the user's persistent subagent-driven-execution preference. THIS commit (Task 16) is the implementation session's final commit: BEHAVIOR_CONTRACT.md `## Access log field mapping` placeholder populated per ADR-0052 + SPEC §13.1; six-gate local sweep completed — gates (a)/(b)/(c)/(d)/(e) all GREEN (gate (f) REVIEW.md deferred to the REVIEW session per BOOTSTRAP §5 step 6); lifecycle-state advances 3 → 4. Phase 06.1 remains closed read-only history; the parent `docs/envoy-go/phases/06-observability-baseline/` retains its `BRAINSTORM.md` + parent `SPEC.md` as the master design document (still open until 06.2 phase-done flips parent row 06 at the REVIEW session's phase-done commit per parent SPEC §5). ROADMAP rows 06 + 06.2 stay `in-progress` until that phase-done commit.
- **lifecycle-state:** `4` — SPEC.md, PLAN.md, and all implementation (Tasks 1–16) committed; implementation complete; verification session not yet run. All four ADRs landed per PLAN commit-map: ADR-0066 (Task 2, `76f3ecd`), ADR-0069 (Task 5, `5278161`), ADR-0067 (Task 7, `6949fce`), ADR-0068 (Task 15, `085890d`). The three-tier matrix (7 Tier-E + 3 Tier-F + 5 Tier-S = 15 operators) implemented in fixture 0006-access-log driver and anchored in ADR-0068 — RESP-SVC-TIME demoted from Tier-E to Tier-S during Task 15 because reference Envoy injects X-Envoy-Upstream-Service-Time but envoy-go does not per Decision A. Full differential suite (0000–0006, 7 fixtures) PASS; h2spec 53/53 PASS; all 20 packages `go test -race` PASS; `golangci-lint` clean (4 formatting/errcheck issues found and fixed during closing sweep). `BEHAVIOR_CONTRACT.md ## Access log field mapping` subsection populated in-place per ADR-0052. Lint cleanup committed alongside BEHAVIOR_CONTRACT edit in this Task 16 commit.
- **next-skill:** `superpowers:verification-before-completion` — phase 06.2 verification session (lifecycle-state 4 → 5). The six-gate local sweep was run at Task 16 from the implementation worktree; the verification session re-runs gates (a)–(e) from a fresh checkout per BOOTSTRAP §7.5 to confirm reproducibility before REVIEW.
- **next-skill-scope:** Verify all six gates per BOOTSTRAP §7.5 + SPEC §3 — six-gate local sweep already run at Task 16 (gates a/b/c/d/e GREEN); verification session re-runs from a fresh checkout/worktree. Gate (f) REVIEW.md is the REVIEW session's deliverable (not verification). On verification PASS, advance lifecycle-state 4 → 5 and set next-skill to `superpowers:requesting-code-review`. The REVIEW session (state 5 → 6) writes REVIEW.md and commits the phase-done with subject `phase 06.2: phase-done — access-log lands; ROADMAP rows 06.2 + 06 → done [ADR-0066, ADR-0067, ADR-0068, ADR-0069]` (parent row 06 closes AT THE SAME COMMIT per parent SPEC §5).
- **last-commit:** `TBD — this task's commit` — Task 16: BEHAVIOR_CONTRACT in-place edit + closing all-gates sweep (a/b/c/d/e GREEN; lint cleanup) + STATE lifecycle-state 3 → 4. On branch `phase/06.2-access-log-impl`. SHA-fill follow-up per phase-02..06.1 convention NOT needed per Task 16 instructions.
- **last-updated:** 2026-04-30

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
