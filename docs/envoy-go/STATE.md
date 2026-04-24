# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `03-tls`
- **phase-directory:** `docs/envoy-go/phases/03-tls/` — exists; contains `SPEC.md`, `PLAN.md`, and `PROGRESS.md` (all 15 PLAN tasks landed on branch `phase/03-tls-impl` under `.worktrees/phase-03-tls-impl`, plus a fresh-session "Verification — lifecycle-state 4 → 5" section appended in `71068a2` quoting verbatim outputs of every SPEC §3 / BOOTSTRAP §7.5 gate). REVIEW.md does not yet exist.
- **lifecycle-state:** `5` — verified, not yet reviewed. Every SPEC §3 / BOOTSTRAP §7.5 phase-done gate that is in scope at this state was re-run from a fresh session by `superpowers:verification-before-completion` and quoted verbatim into PROGRESS.md (commit `71068a2`): (a) `0002-tls-tcp` PASS 1.27s; (b) `0000-tcp-echo` PASS 1.23s + `0001-tcp-proxy-rr` PASS 1.29s (regression-clean across ADR-0032/0033/0034); (c) N/A — phase 03 ships no conformance suites; (d) `FuzzTLSContextParse` (new this phase) + `FuzzBootstrapLoad` + `FuzzTcpProxyFilter` all PASS at ADR-0018's 30s budget — no crashes, no seed-corpus drift; (e) `go build`, `go vet`, `golangci-lint run`, `go test -count=1 -timeout=10m ./...` all exit 0; (e′) `0002-tls-tcp/pki/gen` re-run produces a byte-identical PEM tree (`git diff --exit-code pki/` exit 0). Differential suite ran against pinned reference Envoy `envoyproxy/envoy@sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (`v1.37.2`, per `docs/envoy-go/ENVOY_TARGET.md` / ADR-0008 — the prior STATE block's "v1.32.4 / ADR-0013" reference was a stale mistake; the actual pin is and has been v1.37.2). Eight phase-03 ADRs (ADR-0029..ADR-0036) landed during impl per PROGRESS task-by-task entries; no new ADRs introduced during verification. (f) `REVIEW.md` approval is the only remaining gate and is the entire scope of state 5. Integration topology unchanged: master and `phase/03-tls-impl` shared HEAD `a6f218f` at the start of this session; after `71068a2` (PROGRESS verification) and the STATE-update commit that lands this block, master must be fast-forwarded again so both branches stay aligned per ADR-0003.
- **next-skill:** `superpowers:requesting-code-review`
- **next-skill-scope:** Produce `docs/envoy-go/phases/03-tls/REVIEW.md` by invoking `superpowers:requesting-code-review` against the full phase-03 implementation diff (master tip prior to phase-03 → impl tip after STATE update). Reviewer should verify: (1) PLAN.md's 15-task→commit mapping in PROGRESS is complete and SHAs are filled; (2) ADR-0029..ADR-0036 are coherent, sequentially numbered without gaps, and each cited in code or PROGRESS where invoked; (3) SPEC §3 gates are evidence-backed (the verification block in PROGRESS quotes verbatim outputs — confirm they were not edited); (4) `BEHAVIOR_CONTRACT.md` TLS subsection is consistent with the implemented downstream/upstream/SNI semantics; (5) ADR-0035's narrowed differential scope (fixture 0002 covers downstream TLS + SNI routing only; upstream TLS is unit-tested only) is reviewer-acceptable for phase 03; (6) fixture 0002 PKI is deterministically reproducible (verified above); (7) no `testdata/fuzz/` seed-corpus pollution from the verification fuzz runs (verified above — clean working tree post-run). If `REVIEW.md` flags issues → re-enter at BOOTSTRAP §5 step 3 (resume implementation + TDD), NOT step 4 (per §5.2). If approved → advance to lifecycle-state 6 for the final phase commit, `ROADMAP.md` status flip to `done`, and STATE handoff to phase 04. Work from the impl worktree; master is fast-forwarded to the impl tip at the close of this state-5 transition so both branches share HEAD.
- **last-commit:** 71068a2
- **last-updated:** 2026-04-24

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
