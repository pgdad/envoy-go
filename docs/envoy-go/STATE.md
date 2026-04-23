# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `02-tcp-proxy`
- **phase-directory:** `docs/envoy-go/phases/02-tcp-proxy/` — exists; contains `SPEC.md`, `PLAN.md`, and `PROGRESS.md` (all 10 PLAN tasks landed plus a `## Verification` section appended in commit `3fc5f15` with fresh-shell re-run evidence for every executable SPEC §3 gate). `REVIEW.md` does not yet exist.
- **lifecycle-state:** `5` — verification complete, not yet reviewed. A fresh `go clean -testcache` + all four of the state-4 command groups ran green on branch `phase/02-tcp-proxy-impl` at parent commit `af59456`: (a) `TestDifferential/0001-tcp-proxy-rr` PASS with byte-exact + AssertDistribution per-proxy; (b) `TestDifferential/0000-tcp-echo` PASS (no regression); (c) vacuously green (no conformance suite applies to phase 02 per SPEC §3 row (c)); (d) `FuzzBootstrapLoad` 31.079s PASS and `FuzzTcpProxyFilter` 31.051s PASS, no panics, no new crashes; (e) `go build ./...` / `go vet ./...` / `golangci-lint run ./...` all `[exit=0]`, `go test ./... -timeout 10m` all `ok`, zero `FAIL`. Verbatim outputs plus a gate-by-gate verdict table are quoted in `PROGRESS.md`'s `## Verification` section. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 5, the next session invokes `superpowers:requesting-code-review` to review the phase against `PLAN.md` + `SPEC.md` and produce `REVIEW.md`.
- **next-skill:** `superpowers:requesting-code-review`
- **next-skill-scope:** Dispatch a reviewer subagent (phase-01 used a general-purpose code reviewer and the same dispatch model is expected here; see phase-01 `REVIEW.md` header for the convention) to review phase 02's full commit range on branch `phase/02-tcp-proxy-impl`. Commit range: from the master-fork point to the current tip of the impl branch — as of STATE-advance time the tip is the STATE commit being landed now (phase-01 precedent: tip includes the STATE-advance commit). The reviewer consumes `docs/envoy-go/phases/02-tcp-proxy/SPEC.md`, `PLAN.md`, `PROGRESS.md` (including the new `## Verification` section), the six new ADRs ADR-0022..ADR-0027 plus ADR-0028 (the mid-Task-10 determinism fix), and all production/test code under `internal/cluster/`, `internal/filter/tcpproxy/`, `internal/listener/`, `cmd/envoy-go/main.go`, `test/helpers/`, `test/differential/`, and `test/fixtures/0001-tcp-proxy-rr/`. Reviewer writes `docs/envoy-go/phases/02-tcp-proxy/REVIEW.md` with verdict (APPROVED / APPROVED-WITH-MINORS / CHANGES-REQUESTED) + per-finding severity. Reviewer does NOT re-run any gate — state 4 already captured those; the review is signal/intent-level per phase-01 precedent. If `REVIEW.md` issues Critical or Important findings, per BOOTSTRAP §5.2 the next session re-enters at state 3 (NOT 4) — new tasks are added to `PROGRESS.md` under a `## Review-feedback iteration N` block, implementation resumes under TDD, and state-4 verification must re-run from scratch before the next review round. If `REVIEW.md` is approved outright, advance STATE to lifecycle-state 6 with `next-skill` pointing at the final phase commit + ROADMAP status flip per §5 state 6.
- **last-commit:** 3fc5f15
- **last-updated:** 2026-04-23

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
