# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `10-http-filter-header-mutation` — BRAINSTORM.md landed at this commit (ROADMAP row 10 added with status `planned`; phase directory `docs/envoy-go/phases/10-http-filter-header-mutation/` created with `BRAINSTORM.md` only). Phase 10 is the THIRD §9 family-row to enter `planned` status (after cors @ 07.1 done, fault @ 09 done). Per ADR-0106 flat-top-level-rows discipline, phase 10 lands as a top-level row, NOT a sub-phase of any §9 parent.
- **phase-directory:** `docs/envoy-go/phases/10-http-filter-header-mutation/` — contains `BRAINSTORM.md` only at this commit. SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md follow in subsequent sessions per the phase-09 precedent.
- **lifecycle-state:** `brainstorm-complete` — BRAINSTORM.md is the single output of this session (project's adapted lifecycle-state 0 → 1 → 2 advanced this session by appending ROADMAP row 10 + creating the phase directory + landing BRAINSTORM.md, mirroring phase 09's brainstorm-merge pattern). The next session's input is BRAINSTORM.md + the §9 empirical-pin block (BRAINSTORM §9) executed IN-SESSION against reference Envoy v1.37.2 per ADR-0004.
- **next-skill:** `superpowers:writing-plans` per ADR-0005 routing — but the next session's WORK is to author SPEC.md (per the phase 09 precedent: BRAINSTORM → SPEC → PLAN → impl → review). The skill name is `writing-plans` for routing purposes, but the session's deliverable is SPEC.md (NOT PLAN.md; PLAN.md is the session-after-next). Per the user's persistent preference for subagent-driven over inline execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`, the SPEC-authoring session itself runs inline (single agent collaborating on SPEC text + executing the §9 empirical-pin scrapes) while the subsequent PLAN execution will be subagent-driven.
- **next-skill-scope:** Cold-start scope for the SPEC-authoring session. Read first: `docs/envoy-go/phases/10-http-filter-header-mutation/BRAINSTORM.md` (full); `docs/envoy-go/phases/09-http-filter-fault/SPEC.md` (structural precedent — 1305 lines; mirror its §1..§N + §11 empirical-pin section + §15 acceptance checklist shape); `docs/envoy-go/DECISIONS.md` ADR-0004 (autonomous-brainstorming adaptation; spec-document-reviewer subagent loop discipline), ADR-0005 (autonomous-planning adaptation), ADR-0040 (deferral-ADR format for §8 deferrals), ADR-0072 (HTTP filter registration discipline), ADR-0073 (per-route 3-tier most-specific-override; phase 10 amends, does not supersede), ADR-0074 (cors precedent), ADR-0100 (fault package shape; mirror), ADR-0101 (fault runtimeConfig parser; mirror), ADR-0106 (§9 flat-top-level-rows discipline; phase 10 lands as row 10, not a sub-phase). Read `internal/filter/http/perroute.go:103–128` (existing `Resolve` implementation; phase 10 adds `ResolveAllTiers` sibling) + `cmd/envoy-go/main.go:112–116` (registration site). The session emits SPEC.md only (lifecycle-state 2 → 3 in the project's adapted lifecycle; PLAN.md follows in subsequent session). Empirical pins to resolve IN-SESSION: BRAINSTORM §9 enumerates five (P1 protected-header set, P2 keep_empty_value boundary, P3 stats verification, P4 AppendAction × 4 multi-valued behavior, P5 most_specific_header_mutations_wins evaluation order).
- **last-commit:** `ad7c129` — `phase 10 brainstorm: http-filter-header-mutation [ROADMAP row 10; BRAINSTORM.md]`. Lands `BRAINSTORM.md` (756 lines) for phase 10 (`http-filter-header-mutation`), the THIRD concrete §9 HTTP filters family-row (after cors @ 07.1 and fault @ 09); adds ROADMAP row 10 with status `planned` and `depends-on=09`; advances STATE.md lifecycle 0 → 2; anticipates ADRs ADR-0108..ADR-0114 (~7 ADRs). The §9 family heading at `ROADMAP.md` line 56 stays unchanged per ADR-0106(c). The brainstorm worktree branch `phase-10-http-filter-header-mutation-brainstorm` (branched from master tip `3066c72`) ff-merged into master; SHA filled in this follow-up commit per the phase-04..09 SHA-fill convention. Per user preference, master pushed to origin after merge.
- **last-updated:** 2026-05-04

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
