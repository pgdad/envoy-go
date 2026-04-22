# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `01-static-bootstrap-config`
- **phase-directory:** `docs/envoy-go/phases/01-static-bootstrap-config/` — exists; contains `SPEC.md` as of this commit.
- **lifecycle-state:** `2` — SPEC.md exists, PLAN.md does not. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 2, the next session runs `superpowers:writing-plans` scoped to THIS phase, producing `PLAN.md`. Split gate (§6): if PLAN exceeds ~25 tasks or ~1500 LoC net, split into `01.1`, `01.2`, …
- **next-skill:** `superpowers:writing-plans`
- **next-skill-scope:** Produce `docs/envoy-go/phases/01-static-bootstrap-config/PLAN.md` from the approved SPEC.md. The PLAN must enumerate atomic, TDD-friendly tasks that deliver every SPEC §4 deliverable — new packages `internal/bootstrap/` (with `FuzzBootstrapLoad` per gate (d)) and `internal/admin/` (serving `/ready` only); the rewired `cmd/envoy-go/main.go` + test; `test/differential/harness.go` admin-port wiring; fixture `0000-tcp-echo` evolution (`envoy-go.yaml` rewritten as real Envoy bootstrap YAML, `expectations.yaml` extended with response-status/body/headers applicable, `README.md` refresh, `driver/driver.go` gaining `ProbeAdmin` and a real-bootstrap `SubjectConfig`); new `BEHAVIOR_CONTRACT.md` "Admin API — /ready" subsection; deletion of `cmd/envoy-go/{config,config_test}.go` via a new ADR that explicitly supersedes ADR-0007 per D-3.5; `go.mod`/`go.sum` update for `github.com/envoyproxy/go-control-plane` (proto types only, per D-3.2). Every SPEC §10 deferred decision lands as a new sequential `ADR-NNNN` at planning or implementation time: YAML→proto pipeline shape; go-control-plane version pin; `Server:` header value; pre-init admin response contract; unknown-field handling; node-field semantics; fuzz budget; admin response parser location; main_test.go rewrite vs replacement. Gate (d) applies (first parser/codec in the repo); PLAN must land `FuzzBootstrapLoad` under `internal/bootstrap/fuzz_test.go` with a short-budget CI invocation. Spec-document-reviewer approved the SPEC on iteration 1 with only advisory (non-blocking) recommendations; capture those in `PROGRESS.md` when it is created. Depends-on: phase 00 (done). Branch convention per ADR-0003: this SPEC lands via worktree branch `phase/01-static-bootstrap-config-spec` merged to master; the next session cuts a fresh branch off master for PLAN authoring.
- **last-commit:** c6f9d6c
- **last-updated:** 2026-04-22

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
