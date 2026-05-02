# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `08.2-graceful-drain`
- **phase-directory:** `docs/envoy-go/phases/08.2-graceful-drain/` — currently contains the SPEC stub `README.md` from the 08.1 SPEC commit (`1f85b07`) AND the autonomous-brainstorm artefact `BRAINSTORM.md` from this session's commit (named in `last-commit`; SHA filled in a follow-up commit per the phase-04..08.1 SHA-fill convention). The next session (08.2 SPEC drafting) supersedes the stub `README.md` with a full `SPEC.md`; per the stub's own §1, `README.md` becomes read-only history once `SPEC.md` lands. Parent ROADMAP row `08` STAYS `in-progress` (flips at 08.2 phase-done per parent SPEC §5). All earlier phases (00–07.2) remain closed read-only history.
- **lifecycle-state:** `1` — phase 08.2 BRAINSTORM done at the commit named in `last-commit`. Next session is 08.2 SPEC drafting (state 1 → 2) per `superpowers:writing-plans`.
- **next-skill:** `superpowers:writing-plans` — autonomous SPEC-drafting session for 08.2 per ADR-0005's planning-skill discipline (mirrors the 08.1 SPEC session at commit `1f85b07`, the 07.2 SPEC session at commit `bb5f437`, and the 06.2 SPEC pattern). Inputs: `docs/envoy-go/phases/08.2-graceful-drain/BRAINSTORM.md` (this session's autonomous-brainstorm artefact — the authoritative decision log; ADR anchors anticipated at ADR-0091..ADR-0099; 7 empirical-pin obligations enumerated in §11 to be resolved IN-SESSION by the SPEC author against reference Envoy v1.37.2 per ADR-0004); `docs/envoy-go/phases/08.2-graceful-drain/README.md` (the sibling SPEC stub the SPEC supersedes); `docs/envoy-go/phases/08-admin-api-and-drain/{BRAINSTORM.md, SPEC.md}` (parent master context); `docs/envoy-go/phases/08.1-admin-endpoints/{SPEC.md, REVIEW.md}` (just-closed sub-phase architectural foundation — admin-mux scaffold, ADR-0085 LBP-1 constructor-widening pattern, ADR-0088 state-enum coverage that 08.2 amends additively with `DRAINING`).
- **next-skill-scope:** Lifecycle-state 1 → 2 SPEC-drafting session deliverables: (a) `docs/envoy-go/phases/08.2-graceful-drain/SPEC.md` — full SPEC mirroring the 08.1 SPEC shape (§§1–16: Purpose / Non-purposes / Phase-done gates / Deliverables / Architecture / Surface contracts / Fixture / ADRs / Out-of-scope / Carry-forward / Empirical-pin block / Deferred decisions / BEHAVIOR_CONTRACT additions / Testing / Acceptance / References) with the 7 BRAINSTORM §11 empirical-pin obligations resolved IN-SESSION (verbatim Envoy v1.37.2 scrape evidence pinned for: POST `/drain_listeners` body, `/ready` DRAINING body, in-flight HTTP request behavior during drain, POST method-discrimination, new-conn rejection mechanism, header set, SIGTERM-vs-SIGINT + drain timeout default); (b) ROADMAP row 08.2 status flip `planned → in-progress`; (c) `docs/envoy-go/phases/08.2-graceful-drain/README.md` becomes read-only history per the stub's §1. After SPEC, advance STATE to lifecycle-state 2 with `next-skill: superpowers:writing-plans` for 08.2 PLAN drafting.
- **last-commit:** `e7b64ac` — `phase 08.2 brainstorm: graceful-drain BRAINSTORM.md`. Lands the autonomous-brainstorm artefact (1221 lines, 12 sections, 12 Decisions, 9 anticipated ADRs ADR-0091..ADR-0099, 7 empirical-pin obligations) covering the 08.2 design surface — drain-state machine (LIVE/DRAINING/DRAINED), SIGTERM-handler upgrade, POST /drain_listeners admin endpoint, /ready DRAINING-state body extension, /server_info DRAINING transition, listener.Manager.Drain, cluster.Manager.Drain, fixture 0010-graceful-drain, BEHAVIOR_CONTRACT additions. SHA filled in a follow-up commit per the phase-04..08.1 SHA-fill convention.
- **last-updated:** 2026-05-02

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
