# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `awaiting next planning` — phase 09 (`envoy.filters.http.fault`) is closed at this commit (ROADMAP row 09 status `done` AT this phase-done commit; first §9 family-row landed under BOOTSTRAP_PROMPT.md §9 HTTP filters family per ADR-0106). The next session's planner selects the next §9 family-child via brainstorming (per ADR-0106 — flat top-level rows; the §9 heading at ROADMAP line 56 is an umbrella whose state is implicit and unchanged across family-row landings).
- **phase-directory:** (cleared — no active phase; the next session creates a new `docs/envoy-go/phases/<NN>-<slug>/` directory at brainstorm-merge commit per BOOTSTRAP_PROMPT.md §5 lifecycle-state 0 → 1).
- **lifecycle-state:** `awaiting` — phase 09 closed at lifecycle-state 5 → 6 (REVIEW.md follow-up Task 17 lands the closing review per requesting-code-review skill, but is OUT-OF-BAND of the phase-done commit per the precedent established in phase 08.2). The repo is in BOOTSTRAP_PROMPT.md §5's "between-phases" awaiting state.
- **next-skill:** `superpowers:brainstorming` — the next session brainstorms the next §9 family-child (per ADR-0106 — flat top-level rows; per BRAINSTORM Decision 12). Per the user's persistent preference for subagent-driven over inline execution recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`, the brainstorming session itself runs inline (single agent collaborating with the user on the SPEC space) while the resulting PLAN execution will be subagent-driven.
- **next-skill-scope:** Cold-start brainstorm scope for the next §9 family-child. The §9 HTTP filters family list at `docs/envoy-go/ROADMAP.md` line 58 enumerates the candidates: `Header manipulation, cors [done at 07.1], compression, fault [done at 09], local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit.` cors landed at phase 07.1; fault landed at phase 09. The brainstorming session selects the next family-child from the remaining candidates and produces a single-file BRAINSTORM.md under `docs/envoy-go/phases/<NN>-<slug>/`. Read first: `docs/envoy-go/BOOTSTRAP_PROMPT.md` (§9 family-expansion mandate + §5 lifecycle states 0–1); `docs/envoy-go/ROADMAP.md` (the §9 family list + the row-as-its-own-phase pattern at row 09 — the precedent for the next family-child); `docs/envoy-go/DECISIONS.md` (ADR-0106 family-expansion shape — flat top-level rows; ADR-0001 brainstorm output template); `docs/envoy-go/phases/09-http-filter-fault/{BRAINSTORM,SPEC,PLAN,PROGRESS,REVIEW}.md` (closed read-only history; the structural precedent for the next family-child). The session emits `BRAINSTORM.md` only (lifecycle-state 0 → 1 → 2; SPEC + PLAN follow in subsequent sessions).
- **last-commit:** TBD — `phase 09: http-filter-fault [ADR-0100, ADR-0101, ADR-0102, ADR-0103, ADR-0104, ADR-0105, ADR-0106, ADR-0107]`. Lands `envoy.filters.http.fault` as the SECOND production HTTP filter (after cors at 07.1) and the FIRST top-level row under §9. New `internal/filter/http/fault/` package (~430 LoC); FactoryCtx framework extension (Stats + StatPrefix fields); 5 fault.* stats (4 counters + 1 gauge); 17→22-name BEHAVIOR_CONTRACT extension; new differential fixture `0011-http-fault` (4 scenarios per SPEC §7.1); twelfth fuzzer `FuzzFaultConfigParse`; eight new ADRs ADR-0100..ADR-0107; six gates all green at this commit. SHA filled in a follow-up commit per the phase-04..08.2 SHA-fill convention.
- **last-updated:** 2026-05-03

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
