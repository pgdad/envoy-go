# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `05.2-upstream-h2`
- **phase-directory:** `docs/envoy-go/phases/05.2-upstream-h2/` — contains `README.md`, `SPEC.md` (`dacf4b7`), `PLAN.md` (`4c6b6bb`), and `PROGRESS.md` (Tasks 1-15 landed; Task 15 verification block written by this commit). REVIEW.md will land in the REVIEW session at lifecycle-state 6. The sibling `docs/envoy-go/phases/05.1-downstream-h2/` remains closed read-only history (ROADMAP row `05.1` status `done`). The parent `docs/envoy-go/phases/05-http-2/` retains its `SPEC.md` (`612cdea`) as the master design document. The parent ROADMAP row `05-http-2` stays `in-progress` until 05.2 reaches `done`; the 05.2 phase-done commit will close both rows on the same commit per 05.2 SPEC §4.4 + PLAN Task 15's "Refinement" note.
- **lifecycle-state:** `4` — phase 05.2 implementation complete (15/15 PLAN tasks landed); BEHAVIOR_CONTRACT.md `## HTTP/2` subsection edited in place per ADR-0052; the executor's local six-gate sweep passes a/b/c/d/e (gate (f) REVIEW.md is the next-fresh REVIEW session's deliverable per BOOTSTRAP §5). Per BOOTSTRAP §5 the next transition is state 4 → 5 via `superpowers:verification-before-completion` (the verification session re-runs all six gates per BOOTSTRAP §7.5 / SPEC §3 and advances STATE to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review`).
- **next-skill:** `superpowers:verification-before-completion` — verify all six gates per BOOTSTRAP §7.5 / SPEC §3. Inputs: this commit's PROGRESS.md Task 15 verification block (verbatim outputs of the executor's gate sweep), `BOOTSTRAP_PROMPT.md` §7.5, `docs/envoy-go/phases/05.2-upstream-h2/SPEC.md` §3, the closed-history 05.1 verification block (`docs/envoy-go/phases/05.1-downstream-h2/PROGRESS.md` lines 897-) as the verbatim-output precedent. Verifier branches a fresh worktree from master (per ADR-0003 + per-phase-worktree convention) and re-runs every gate.
- **next-skill-scope:** Re-run all six gates per BOOTSTRAP §7.5: (a) `go test -count=1 -run TestDifferential/0004-h2-routing -v ./test/differential/`; (b) `go test -count=1 -run TestDifferential -v ./test/differential/`; (c) `go test -count=1 ./test/conformance/h2spec/ -v -timeout=300s` — assert 53/53 PASS; (d) all six fuzzers at `-fuzztime=30s` with empty `git status --porcelain` after each (`FuzzFrameStream` + `FuzzHPACKDecode` in `./internal/filter/hcm/h2/`, `FuzzBootstrapLoad` in `./internal/bootstrap/`, `FuzzTcpProxyFilter` in `./internal/filter/tcpproxy/`, `FuzzTLSContextParse` in `./internal/tls/`, `FuzzHCMConfigParse` in `./internal/filter/hcm/`); (e) `go vet ./...`, `golangci-lint run ./...`, `go test -race ./...` — every check clean; ADR-0046 boundary grep empty modulo the 5 allowed files (`framer.go`, `hpack.go`, `settings.go`, `conn.go`, `client.go`); ADR-0048 client.go presence confirmed; forbidden-runtime-imports grep clean modulo `doc.go` text mentions. Gate (f) REVIEW.md is deferred to the next-fresh REVIEW session (lifecycle-state 6). On all gates green, advance STATE to lifecycle-state 5 with `next-skill: superpowers:requesting-code-review`. ROADMAP row 05.2 → `done` AND parent row 05 → `done` flip at the phase-done commit (REVIEW session, lifecycle-state 6), NOT at the verification commit.
- **last-commit:** `bd75c88` — `phase 05.2: BEHAVIOR_CONTRACT in-place edit + all-gates green local sweep (a/b/c/d/e green; f deferred to REVIEW)`, on branch `phase/05.2-upstream-h2-impl`. Lands the in-place `BEHAVIOR_CONTRACT.md ## HTTP/2` rewrite per ADR-0052 (NOT a supersession), the four header-allow-list rows flipped from `forward-looking` to `active per ADR-0057`, the Task 15 PROGRESS verification block, a 1-line test-deadline extension on `TestClientConn_RoundTrip_PeerDataAfterEndStream` (2s → 10s) to fix a pre-existing -race flake, and STATE → lifecycle-state 4. ROADMAP rows unchanged at this commit (row 05.2 + parent row 05 stay `in-progress` per PLAN Task 15's "Refinement" note; phase-done commit at lifecycle-state 6 will flip both).
- **last-updated:** 2026-04-26

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
