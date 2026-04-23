# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `02-tcp-proxy`
- **phase-directory:** `docs/envoy-go/phases/02-tcp-proxy/` — exists; contains `SPEC.md` (committed, reviewer-approved via spec-document-reviewer subagent per ADR-0004) and `PLAN.md` (committed, reviewer-approved via plan-document-reviewer subagent per ADR-0005, iteration 1 approved with four advisory polishes applied).
- **lifecycle-state:** `3` — SPEC.md and PLAN.md exist; PROGRESS.md does not. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 3, the next session invokes `superpowers:subagent-driven-development` (per ADR-0005 §4 default executor preference) to execute `PLAN.md` task-by-task with TDD per `superpowers:test-driven-development` on every task. Each task lands its own atomic commit and appends to `PROGRESS.md` (template per PLAN.md `## PROGRESS.md conventions`). Phase-02 split gate at planning time was inspected (10 tasks under ~25; ~1700 LoC slightly over ~1500) and the no-split decision was justified per BOOTSTRAP §6 with a documented mid-execution split valve at Task 7 (the cutover) — if Task 7 sub-steps blow past ~15 in execution, split into 02.1 (foundation) + 02.2 (cutover) per BOOTSTRAP §6.2 with a new ADR.
- **next-skill:** `superpowers:subagent-driven-development`
- **next-skill-scope:** Execute `docs/envoy-go/phases/02-tcp-proxy/PLAN.md`'s 10 tasks in order, in a fresh worktree on a phase-implementation branch (recommended `.worktrees/phase-02-tcp-proxy-impl` on branch `phase/02-tcp-proxy-impl`, cut from master per ADR-0003) — NOT on `phase/02-tcp-proxy-plan` (this PLAN's authoring branch). Read PLAN's `## Execution preconditions` block first; verify all six items pass before Task 1. Tasks: (1) preconditions + PROGRESS preamble; (2) `internal/cluster` Cluster + Endpoint + round-robin LB [ADR-0024]; (3) `internal/cluster.Manager` build-time materialisation; (4) `internal/filter/tcpproxy` Filter + NewFilter + Handle [ADR-0023, pump lifted verbatim from cmd/envoy-go]; (5) `FuzzTcpProxyFilter` at ADR-0018's 30s budget; (6) `internal/listener.Manager` multi-listener + Start/Stop [ADR-0025]; (7) **atomic cutover** rewiring cmd/envoy-go/main.go + harness per-listener sentinels + FixtureDriver multi-backend interface + bootstrap.First* deletions [ADR-0022, ADR-0026] — see PLAN's "Why this task is atomic" sub-section and the 7a/7b split-valve fallback; (8) BEHAVIOR_CONTRACT TCP proxy subsection; (9) fixture `0001-tcp-proxy-rr` with AssertDistribution [ADR-0027]; (10) all-gates green local run with verbatim PROGRESS quotes. Six new ADRs (0022..0027) land in the named tasks above. Phase 02 ROADMAP row remains `planned` until state-machine step 6 (REVIEW.md approved) — the executor advances STATE to lifecycle-state 4 with `next-skill: superpowers:verification-before-completion` at session-exit.
- **last-commit:** edf1c5f
- **last-updated:** 2026-04-23

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
