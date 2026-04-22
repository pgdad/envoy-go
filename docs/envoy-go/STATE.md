# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `01-static-bootstrap-config`
- **phase-directory:** `docs/envoy-go/phases/01-static-bootstrap-config/` — exists; contains `SPEC.md`, `PLAN.md`, and `PROGRESS.md` (17 task entries, all SHA-filled) as of this commit.
- **lifecycle-state:** `4` — all 17 PLAN tasks have landed on `phase/01-static-bootstrap-config-impl`; Task 17 captured SPEC §3 phase-done gates a–e as a single green evidence block (`go vet`, `golangci-lint`, `go test ./...`, `go test ./test/differential/... -v`, `go test -fuzz=FuzzBootstrapLoad -fuzztime=30s`) verbatim in `PROGRESS.md`. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 4, the next session runs `superpowers:verification-before-completion` to re-prove those gates in a fresh environment. Gate (f) REVIEW is state-machine step 5 (a separate subsequent session).
- **next-skill:** `superpowers:verification-before-completion`
- **next-skill-scope:** Re-run SPEC §3 phase-done gates a–e AS IF on CI — from a clean shell with fresh module / build / test caches (e.g. `go clean -testcache`), against the `phase/01-static-bootstrap-config-impl` branch in the worktree at `.worktrees/phase-01-static-bootstrap-config-impl`. The five gates are: (1) `go vet ./...`, (2) `golangci-lint run ./...`, (3) `go test ./... -timeout 10m`, (4) `go test ./test/differential/... -timeout 5m -v` (must include `--- PASS: TestDifferential/0000-tcp-echo`), (5) `go test ./internal/bootstrap/ -fuzz=FuzzBootstrapLoad -fuzztime=30s -run=^$` (no panics, zero `new crashes`, final `PASS`). The local-run evidence these must match is already quoted verbatim in `docs/envoy-go/phases/01-static-bootstrap-config/PROGRESS.md` under `## Task 17 — Green local gate sweep (lint/vet/test/differential/fuzz)` — cite that block as the baseline and quote the verification-session outputs into a NEW `## Verification` section appended to the same PROGRESS.md. Additionally verify the CI workflow YAML landed by Task 6 (`.github/workflows/` fuzz-bootstrap job): (a) the YAML parses (e.g. `yq .` or `python3 -c 'import yaml,sys; yaml.safe_load(open(sys.argv[1]))'`) and (b) the fuzz job's `needs:` dependency resolves to an existing job name in the same file. Per SPEC §3, this verification session does NOT run gate (f) REVIEW — REVIEW is state-machine step 5, owned by `superpowers:requesting-code-review` in the session AFTER verification passes. Exit contract: on green verification, advance STATE.md to `lifecycle-state: 5` with `next-skill: superpowers:requesting-code-review`; on failure, set `lifecycle-state: blocked` with a `block-reason` naming the failing gate and its first divergence.
- **last-commit:** f43f66f
- **last-updated:** 2026-04-22

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
