# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `03-tls`
- **phase-directory:** `docs/envoy-go/phases/03-tls/` — exists; contains `SPEC.md` (approved by spec-document-reviewer subagent per ADR-0004, committed on branch `phase/03-tls-spec` at `4f1c356`).
- **lifecycle-state:** `2` — SPEC.md exists, PLAN.md does not. Per SKILL_ROUTING.md / BOOTSTRAP_PROMPT §5 state 2, the next session runs `superpowers:writing-plans` scoped to THIS phase, producing `PLAN.md`. GATE: if the draft plan exceeds ~25 tasks OR ~1500 LoC net change, the session splits phase 03 into `03.1`, `03.2`, … per §6, updates ROADMAP + STATE, and exits.
- **next-skill:** `superpowers:writing-plans`
- **next-skill-scope:** Plan `docs/envoy-go/phases/03-tls/PLAN.md` against `docs/envoy-go/phases/03-tls/SPEC.md` (452 lines, ADR-0004-autonomous reviewer-approved). The PLAN must decompose the SPEC's deliverables (SPEC §4) into atomic TDD-friendly tasks that each land as one commit: introduce `internal/tls/` (config.go / datasource.go / params.go / sni.go) with unit + fuzz tests; extend `internal/listener/manager.go` for multi-chain + SNI-routing via `crypto/tls` `GetConfigForClient`; extend `internal/cluster/` with the `Dial(ctx) (net.Conn, error)` abstraction covering plaintext and TLS paths; update `internal/filter/tcpproxy/filter.go` to consume `ctx` via `cluster.Dial(ctx)` (phase-02 REVIEW Minor 4 resolution); split `test/differential/fixture.Driver.Drive` into `DriveReference` + `DriveSubject` and update all three drivers atomically (Minor 6 resolution); land fixture `0002-tls-tcp` (two SNI-indexed chains, two upstream TLS clusters, committed PKI, distribution assertion mirroring fixture 0001's discipline); update `BEHAVIOR_CONTRACT.md` with the new TLS subsection AND append the ADR-0028 cross-reference to the adjacent TCP proxy subsection (Minor 8 resolution); land the 7 ADRs A–G anticipated in SPEC §4.4 (ADR-B **Supersedes: ADR-0025** header mandatory; ADR-F `(informal) Supersedes` the phase-02 fixture.Driver). Expected sequential ADR numbers: `ADR-0029`..`ADR-0035` (phase-02 tail is `ADR-0028` — planner verifies at PLAN write time). The planner must honour SPEC §10's deferred-decision list (resolve each with either a PLAN.md note or defer explicitly) and SPEC §11's risk table (each risk maps to a mitigated task or an ADR). Phase-done gates (SPEC §3): (a) fixture 0002 green with byte-exact plaintext + per-cluster `[3,3,3]` distribution per side, (b) fixtures 0000 + 0001 regression-free (explicit check per the spec reviewer's advisory bullet 5), (c) conformance N/A, (d) `internal/tls.FuzzTLSContextParse` clean + phase-01/phase-02 fuzz targets clean, (e) vet + lint + `go test ./...` clean, (f) REVIEW.md approved. If the planner judges the SPEC requires change, a PLAN-stage ADR must name the SPEC section + delta. The plan-document-reviewer subagent loop (ADR-0005) is the approval gate; up to 3 iterations, else exit blocked. Advisory spec-review recommendations (non-blocking): §10 #11 restates §5.5 table (cosmetic); §5.3 offers two chain-propagation mechanisms and §10 #2 reopens the same — planner locks one; PKI gen-tool drift risk is unmentioned in SPEC §11 (planner may add a Task-level check); `cmd/envoy-go/main_test.go` marked "unchanged" but the planner should verify plaintext-listener regression coverage; SPEC §13 acceptance checklist's "fixtures 0000/0001 green after ADR-F" is implicit — the planner may add an explicit verification step.
- **last-commit:** 4f1c356
- **last-updated:** 2026-04-23

---

## Lifecycle cross-reference

See `SKILL_ROUTING.md` (and `BOOTSTRAP_PROMPT.md` §5) for the full state machine. The `lifecycle-state` field above maps to numbered states 0–6 of that machine. The `next-skill` field is the skill to invoke at the next session's Step D.

## Exit contract for every session

Before exiting, a session MUST:

1. Update `active-phase`, `phase-directory`, `lifecycle-state`, `next-skill`, `next-skill-scope`, `last-commit`, `last-updated` above to reflect the new state.
2. Ensure the repository has no uncommitted state-affecting changes (scaffold, SPEC, PLAN, PROGRESS, REVIEW, or code changes are all either committed or deliberately abandoned per §6 splitting).
3. If exiting due to `blocked` or an ADR'd deviation, set `lifecycle-state` to `blocked` and add a short `block-reason` line below the list.
