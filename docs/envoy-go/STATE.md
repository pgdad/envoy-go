# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `01-static-bootstrap-config`
- **phase-directory:** `docs/envoy-go/phases/01-static-bootstrap-config/` — exists; contains `SPEC.md`, `PLAN.md`, and `PROGRESS.md` (17 task entries all SHA-filled + `## Verification` block appended in this session).
- **lifecycle-state:** `5` — fresh-environment verification session re-proved SPEC §3 phase-done gates a–e on `phase/01-static-bootstrap-config-impl` (parent commit `cb02d80`) from the `.worktrees/phase-01-static-bootstrap-config-impl` worktree with `go clean -testcache` before the gated runs. All five gates green: `go vet ./...` clean, `golangci-lint run ./...` clean, `go test ./... -timeout 10m` all `ok`, `go test ./test/differential/... -timeout 5m -v` includes `--- PASS: TestDifferential/0000-tcp-echo (1.16s)`, and `go test ./internal/bootstrap/ -fuzz=FuzzBootstrapLoad -fuzztime=30s -run=^$` final `PASS` with `ok …/internal/bootstrap 31.075s`, no panics, zero new crashes. CI workflow YAML additionally validated per STATE scope: `.github/workflows/ci.yml` parses via `yaml.safe_load`, and `fuzz-bootstrap.needs: lint-vet-test` resolves to an existing job in the same file (as does `differential.needs: lint-vet-test`). Evidence quoted verbatim under `## Verification` in `PROGRESS.md`. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 5, the next session runs `superpowers:requesting-code-review` to obtain gate (f) REVIEW approval.
- **next-skill:** `superpowers:requesting-code-review`
- **next-skill-scope:** Produce `docs/envoy-go/phases/01-static-bootstrap-config/REVIEW.md` — gate (f) of SPEC §3. Scope: the full phase-01 diff on branch `phase/01-static-bootstrap-config-impl` (parent: merge-base with `master`; tip: the Verification-block commit produced in this session, visible in `docs/envoy-go/STATE.md`'s `last-commit` after this session's SHA-fill amend). The reviewer must: (i) walk every SPEC §3 exit criterion and every SPEC §4 deliverable path, flagging absent / divergent files; (ii) spot-check SPEC §5 (architecture) for behavioral deviation; (iii) confirm each ADR referenced in `PROGRESS.md` (ADR-0014 through ADR-0021) is present in `docs/envoy-go/DECISIONS.md`; (iv) NOT re-run gates a–e — cite the `## Verification` block in PROGRESS.md rather than re-executing. Follow whichever REVIEW.md template an earlier phase established (check `docs/envoy-go/phases/00-bootstrap/REVIEW.md` for precedent; if none exists, mint one consistent with BOOTSTRAP_PROMPT §5 step 5). Findings must be severity-classified and end with an explicit approve / request-changes verdict. Exit contract: on `approve`, advance `lifecycle-state` to `6` with `next-skill` set to the phase-commit skill that lands the phase-done exit criteria of SPEC §3 (ROADMAP row 01 → `done`; STATE advanced to `02-tcp-proxy` with `lifecycle-state: 1` and `next-skill: superpowers:brainstorming`; phase-01 merged into `master`). On `request-changes`, set `lifecycle-state: blocked` with a `block-reason` enumerating the required changes (and on resolution loop back to `lifecycle-state: 3`).
- **last-commit:** {FILL-AT-COMMIT}
- **last-updated:** 2026-04-23

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
