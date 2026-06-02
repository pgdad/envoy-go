# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `phase 28.1b PLAN done (next = 28.1b IMPL)` (2026-06-02, this commit) — **phase 28.1b (`network-filter-read-seam-and-zookeeper-requests-proof`), the second/closing half of the user-approved ADR-0045 28.1a/28.1b invoked split, has its PLAN authored + plan-document-reviewer-approved + committed.** The 28.1b PLAN (`docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/PLAN.md`) decomposes the 28.1b SPEC §10 task spine into **10 bite-sized TDD tasks**: (1) first-action baselines/anchors gate; (2) `Buffer.TotalAppended` (**int64** — D-S28.1b-1 PLAN-resolved) + the zookeeperproxy decoder feed re-base, MERGED into one task per the spec-reviewer advisory so the int64 widening is never split across commits (incl. the `fuzz_test.go` mechanical updates the SPEC §3.7 file table missed); (3) `chainRuntime.replayRead` (re-cut BEFORE readconn.go — the SPEC spine order would not compile: readChainConn.Read calls replayRead); (4) `readconn.go` `readChainConn`; (5) the `handleTerminal` read-wrap insertion under the SHARED `len(writeFilters) > 0` predicate + composition/R1/soundness-invariant tests (incl. the load-bearing `newPrefixConn(conn, …)`-not-`rt.conn` nesting change); (6) the §3.6 concurrency race test (**net.Pipe unit shape** — D-S28.1b-2 PLAN-resolved: docker-independent); (7) `0046` re-enable + cross-side GREEN + R4 deliberate-break + fixture README; (8) `0047-zookeeper-boot-reject` (port 15049; the `0044` template); (9) completion bundle — ADR-0221 (both-seams body incl. the 28.2 synchronization forward-pointer) + ADR-0222 bodies + BEHAVIOR_CONTRACT 136 → 337; (10) six-gate + STATE/ROADMAP sub-row advance + next-prompt for the 28.2-SPEC cold-start. ADR-0045 split-gate FINAL re-check: ~80–100 production LoC / 10 tasks → **NO further split**.
- **phase-directory:** `docs/envoy-go/phases/28.1b-network-filter-read-seam-and-zookeeper-requests-proof/` (README.md + SPEC.md + **PLAN.md** at this commit; PROGRESS.md/REVIEW.md land at their lifecycle states). The 28.1 directory (`docs/envoy-go/phases/28.1-network-filter-write-seam-and-zookeeper-requests/`) stays the durable, unmodified record of 28.1a. The parent directory `docs/envoy-go/phases/28-network-filter-zookeeper-proxy/` stays canonical; the 28.2 sub-phase directory is created at the 28.2 SPEC session.
- **lifecycle-state:** `phase 28.1b PLAN done (next = 28.1b IMPL)` — SKILL_ROUTING state 3 (PLAN.md exists, implementation incomplete).
- **next-skill:** `superpowers:subagent-driven-development` (per `feedback_execution_style`) scoped to **the 28.1b IMPL** — execute the PLAN's 10 tasks (TDD per task; per-task `gofmt -l` + `golangci-lint run` per `feedback_pertask_gofmt_lint`; PROGRESS.md append per task; subagents commit LOCAL-ONLY per `feedback_subagents_no_push`; the controller squash-merges + pushes at stage-close per `feedback_push_to_origin`; work in a git worktree per `feedback_git_worktrees`).
- **last-commit:** `eff30e8` — `phase 28.1b PLAN: 10-task TDD decomposition — ADR-0045 final gate NO SPLIT; D-S28.1b-1 resolved (int64); D-S28.1b-2 resolved (net.Pipe unit race test); plan-document-reviewer approved [ADR-0221,ADR-0222]`. Substantive predecessor on master: the 28.1b-SPEC squash `baf02ed`.
- **last-updated:** 2026-06-02
- **next-free ADR:** `ADR-0224` (UNCHANGED at this 28.1b-PLAN commit — DECISIONS.md tail STAYS **ADR-0223**; the 28.1b PLAN consumes NO ADR number; the ADR-0221 (both halves) + ADR-0222 §Decision/§Consequences bodies land IN PLACE at the 28.1b IMPL Task 9, the ADR-0223 body at the 28.2 IMPL — and ADR-0223 MUST carry the 28.1b SPEC §3.6 correlation-structure synchronization forward-pointer; no further phase-28 ADR numbers anticipated).

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
