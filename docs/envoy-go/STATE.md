# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `02-tcp-proxy`
- **phase-directory:** `docs/envoy-go/phases/02-tcp-proxy/` — exists; contains `SPEC.md`, `PLAN.md`, and `PROGRESS.md` (all 10 PLAN tasks landed, atomic commits per phase-00/01 precedent, final Task-10 all-gates evidence quoted verbatim). REVIEW.md does not yet exist.
- **lifecycle-state:** `4` — implementation complete, not yet reviewed. PLAN.md's 10 tasks all landed on branch `phase/02-tcp-proxy-impl` under `.worktrees/phase-02-tcp-proxy-impl`, each with its atomic main commit + SHA-fill follow-up (phase-01 convention). Six phase-02 ADRs landed per PLAN mapping (ADR-0022 through ADR-0027); one additional ADR-0028 landed mid-Task-10 as fallout from the differential-gate first-run failure (deterministic driver payloads + `--concurrency 1` on the reference Envoy). All six SPEC §3 gates green locally: (a) fixture `0001-tcp-proxy-rr` byte-exact + AssertDistribution [3,3,3] both sides; (b) fixture `0000-tcp-echo` still green; (c) N/A (no conformance suites in phase 02); (d) `FuzzTcpProxyFilter` + `FuzzBootstrapLoad` both clean at ADR-0018's 30s CI budget; (e) `go build`, `go vet`, `golangci-lint run`, `go test -short ./...` all clean; (f) deferred to state 5. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 4, the next session invokes `superpowers:verification-before-completion` to re-run every gate and capture for REVIEW.md.
- **next-skill:** `superpowers:verification-before-completion`
- **next-skill-scope:** Re-run every SPEC §3 phase-done gate from a fresh session, quote every command's verbatim output into the REVIEW.md scaffolding (or a dedicated verification section of PROGRESS.md — BOOTSTRAP §5 state 4 names PROGRESS as the capture target). Commands: `go build ./...`; `go vet ./...`; `golangci-lint run ./...`; `go test ./...`; `go test ./internal/bootstrap/ -run "^TestNothing$" -fuzz=FuzzBootstrapLoad -fuzztime=30s`; `go test ./internal/filter/tcpproxy/ -run "^TestNothing$" -fuzz=FuzzTcpProxyFilter -fuzztime=30s`; `go test ./test/differential/ -v -timeout=10m`. Docker + the Envoy v1.37.2 image (per ADR-0008) required for the differential suite. Work from the impl worktree or a fresh branch at master tip + impl branch fast-forward; ADR-0003 defines the branch strategy. After verification passes, advance STATE to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review` per §5 state 5.
- **last-commit:** f18c24b
- **last-updated:** 2026-04-23

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
