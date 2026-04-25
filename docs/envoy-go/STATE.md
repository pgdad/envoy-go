# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `04-http-1.1`
- **phase-directory:** `docs/envoy-go/phases/04-http-1.1/` — does **not** yet exist (per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3, a phase directory is created only at the moment a phase enters `in-progress`; the next session's first file-system act is `mkdir docs/envoy-go/phases/04-http-1.1/` and that act is also when ROADMAP row 04 flips from `planned` to `in-progress`). Phase 03's directory remains in place at `docs/envoy-go/phases/03-tls/` with `SPEC.md`, `PLAN.md`, `PROGRESS.md`, and `REVIEW.md` (verdict APPROVED WITH FOLLOW-UPS, all four Important items I-1..I-4 landed in `98cc35b`, post-fix verification block in `cbfe275`); it is now closed and read-only history.
- **lifecycle-state:** `1` — phase 04 is in ROADMAP (`planned`, `depends-on: 03`) but its directory does not yet exist. Per `BOOTSTRAP_PROMPT.md` §5 state 1, the next session creates the phase directory, invokes `superpowers:brainstorming` scoped to phase 04, and produces `SPEC.md`. Phase 03 closed at lifecycle-state 6 in this transition: ROADMAP row 03 status flipped `in-progress` → `done` (this commit), `REVIEW.md` (`d45c467`) was APPROVED WITH FOLLOW-UPS with all four Important items landed in `98cc35b` and re-verified in `cbfe275`, and the §5.3 phase-done commit lands eight ADRs (ADR-0029..ADR-0036). The 8 Minor REVIEW findings (M-1..M-8) are deferred to phase 04+ triage in the same form phase-02's REVIEW Minors carried into phase 03 — see `next-skill-scope` for the candidate hygiene list.
- **next-skill:** `superpowers:brainstorming`
- **next-skill-scope:** Brainstorm phase 04 — `HTTP connection manager (HTTP/1.1) + route match + router filter + direct_response`. Per ROADMAP, the differential surface at phase 04 completion is the new HTTP/1.1 routing fixture green plus all pre-existing fixtures (`0000-tcp-echo`, `0001-tcp-proxy-rr`, `0002-tls-tcp`) regression-clean. First file-system acts of the next session: (1) `mkdir docs/envoy-go/phases/04-http-1.1/`; (2) flip ROADMAP row 04 `planned` → `in-progress`; (3) invoke `superpowers:brainstorming` scoped to phase 04 to produce `phases/04-http-1.1/SPEC.md`. The brainstorming session should triage these phase-03 carry-forward Minor findings from `REVIEW.md` (`d45c467`) into the SPEC, absorb them into a doctrine sweep, or re-defer them explicitly: M-1 strictness flag (a stub `Strict` field exists on `*ContextConfig` but is unused), M-2 `applyTLSParams` takes `*ContextConfig` only by reading the params subset, M-3 `dataSourceLoader` micro-helper exists but isn't reused, M-4 ADR-0032 `Cluster.Dial(ctx)` returns `(net.Conn, error)` not `(*ClusterEndpoint, net.Conn, error)`, M-5 listener-manager `GetConfigForClient` allocates one closure per accepted conn, M-6 fixture-0002 PKI-gen Go program lacks a `-help` flag, M-7 `internal/tls/manager.go` `_ = clientCAs` discard suppresses an unused-var warning instead of explicitly building the chain, M-8 SPEC §5.5 `ssl_key_log_file` row says "ignored" without explaining why (audit-loggable). All eight are deferrable per the reviewer's own approval line; none affect correctness of any asserted differential gate. ALSO carry forward as candidates: the phase-02 ADR-0028 cross-link discipline now codified by Minor 8 / ADR-0036 (the BEHAVIOR_CONTRACT cross-link to ADR-0028) — phase 04+ ADRs that introduce determinism pins or scope reductions should mirror that cross-link discipline. Work from a fresh impl worktree: at state 1 the next session may either keep working in `.worktrees/phase-03-tls-impl/` (since the workdir is already useful and the impl branch is now master's HEAD) OR start phase 04 in its own worktree per the project's per-phase-worktree convention (`phase/04-http-1.1-spec`, `phase/04-http-1.1-plan`, `phase/04-http-1.1-impl`); both are acceptable per BOOTSTRAP §5 (no worktree-naming requirement is in the spec). Master and `phase/03-tls-impl` share HEAD again at this commit per ADR-0003.
- **last-commit:** 71a9065 (the lifecycle-state-5 STATE.md update on `phase/03-tls-impl` that closed phase 03 verification post-REVIEW-follow-ups; the §5.3 phase-done commit lands directly on top of it and fast-forwards `master` to its tip per ADR-0003).
- **last-updated:** 2026-04-25

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
