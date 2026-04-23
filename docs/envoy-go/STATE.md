# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `03-tls`
- **phase-directory:** `docs/envoy-go/phases/03-tls/` — exists; contains `SPEC.md` (approved by spec-document-reviewer subagent per ADR-0004, committed on branch `phase/03-tls-spec` at `4f1c356`) and `PLAN.md` (3367 lines; approved by plan-document-reviewer subagent per ADR-0005 autonomous loop; committed on branch `phase/03-tls-plan` at `9584ce7`).
- **lifecycle-state:** `3` — SPEC.md + PLAN.md exist; implementation incomplete. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 3, the next session runs `superpowers:subagent-driven-development` (recommended per ADR-0005 §4) or `superpowers:executing-plans` to implement the 15-task plan against `docs/envoy-go/phases/03-tls/PLAN.md`, appending to `PROGRESS.md` on each task landing.
- **next-skill:** `superpowers:subagent-driven-development`
- **next-skill-scope:** Execute `docs/envoy-go/phases/03-tls/PLAN.md` (15 tasks, expected ~2600 LoC net change, expected 7 ADRs landing as ADR-0029..ADR-0035). Per ADR-0003: start on a fresh implementation worktree `.worktrees/phase-03-tls-impl` on branch `phase/03-tls-impl` cut from master at the commit that landed the PLAN. Follow the plan's `## Execution preconditions` block verbatim before Task 1 (includes a Task 1 preflight grep for `--concurrency 1` inheritance in `test/differential/harness.go`). TDD discipline (D-3.1): tests first on every task. Each task lands as one commit (ADR-bearing tasks append the ADR to `DECISIONS.md` in the same commit; first-use commit order matches DECISIONS.md file order). Mid-execution split valve: if any task's sub-steps blow past 15 items, split per `BOOTSTRAP_PROMPT.md` §6.2 with an ADR (likely ADR-0036+). Phase-done gates (SPEC §3 / phase-done gate §7.5): (a) fixture 0002 green with byte-exact plaintext + per-cluster `[3,3,3]` distribution per side, (b) fixtures 0000 + 0001 regression-free after ADR-0034 interface split, (c) conformance N/A, (d) `internal/tls.FuzzTLSContextParse` + `internal/bootstrap.FuzzBootstrapLoad` + `internal/filter/tcpproxy.FuzzTcpProxyFilter` clean on CI short budget (30s each, ADR-0018), (e) `go vet`, `golangci-lint run`, `go test ./...` clean, (f) REVIEW.md approved — (f) is the next-next session's responsibility (state 5). At executor session exit after Task 15 lands green, advance STATE to lifecycle-state 4 (`verification-before-completion`) and fast-forward `phase/03-tls-impl` into master per ADR-0003. The plan-authoring session (this one) applied the plan-document-reviewer's four advisory recommendations inline before committing: Task 5 helper stub clarified to a real `anypb.New(inner)` three-liner; Task 7 detReader.Read rewritten with `encoding/binary.LittleEndian.PutUint64` (no `unsafe.Pointer`); Settled §10 #2 forward-references Task 10 Step 3's refinement to pure-function post-handshake dispatch (simpler than the initially-proposed sync.Map shuttle — ADR-0033's Consequences captures the refinement at landing); Task 1 preflight grep for `--concurrency 1` in `harness.go` catches inheritance drift at cold-start rather than at Task 13.
- **last-commit:** 9584ce7
- **last-updated:** 2026-04-23

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
