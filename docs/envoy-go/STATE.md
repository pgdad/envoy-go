# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `12-http-filter-csrf` — phase 12 brainstorm landed at this commit. ROADMAP row 12 added with status `planned` per BOOTSTRAP_PROMPT.md §4.1 invariant 3 + §5 lifecycle-state-0 → 1 transition. Phase 12 is the FIFTH §9 family-row to enter the lifecycle (after cors @ 07.1 done, fault @ 09 done, header_mutation @ 10 done, local_ratelimit @ 11 done); the §9 family heading at ROADMAP line 56 stays unchanged per ADR-0106(c). Filter selection (`csrf`) was the brainstormer's choice per ADR-0106(b) + the user's interactive Q1 dialogue (alternatives surfaced were buffer, bandwidth_limit, rbac, plus deferred-larger compression/jwt_authn/oauth2/lua candidates).
- **phase-directory:** `docs/envoy-go/phases/12-http-filter-csrf/` — contains `BRAINSTORM.md` (~700 lines) at this commit. SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md authored over phase lifecycle (states 2 → 6). The BRAINSTORM commits 5 design Decisions across §2 (filter package layout / extension-registry registration / 1-consumed-2-deferred MVP envelope / origin-extraction-and-comparison-algorithm-and-method-gate / per-route-TPFC-data-only / stat surface / rejection wire shape), 6 differential-fixture scenarios at §6 (5 in fixture 0014 + 1 GET passthrough deferred to unit tests), 5 anticipated ADRs at §7 (ADR-0120..ADR-0124), 3 inline-deferral items at §8 (`filter_enabled` Runtime-family-coupled; `shadow_enabled` Runtime-family-coupled; StringMatcher non-exact variants entry-level-skip), and 10 empirical-pin obligations at §9 (§11.P1..§11.P10 to be resolved IN-SESSION at SPEC time per ADR-0004). NO sibling-stub was authored per ADR-0106(b). NO off-master prebrainstorm-notes branch was authored (UNLIKE phase 11's pre-existing `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` advisory branch).
- **lifecycle-state:** `phase 12 brainstormed; SPEC.md pending` — phase 12 BRAINSTORM.md authored at this commit; SPEC.md not yet authored. Lifecycle-state-1 → 2 transition (BOOTSTRAP_PROMPT.md §5) is the next session's responsibility. Phase 11's six gates remain green at parent commit `0f3a710`; this commit is docs-only (no code changes; no test re-validation needed).
- **next-skill:** `superpowers:writing-plans` per BOOTSTRAP_PROMPT.md §5 lifecycle-state-2 entry condition (SPEC.md exists, PLAN.md does not — phase 12 enters this state once SPEC.md is authored; the routing convention from phases 09/10/11 is that writing-plans is called once for SPEC drafting, then again for PLAN drafting based on the SPEC). The SPEC author resolves the 10 §9 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.
- **next-skill-scope:** Cold-start scope for the phase 12 SPEC-authoring session. Read first: `docs/envoy-go/phases/12-http-filter-csrf/BRAINSTORM.md` (the authoritative design doc — every load-bearing decision lives there per D-3.4); `docs/envoy-go/ROADMAP.md` row 12 (now `planned`); `docs/envoy-go/BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.local_ratelimit` subsection (the just-shipped phase 11 contract — phase 12 mirrors its structural shape with simplifications); `docs/envoy-go/DECISIONS.md` ADR-0114..ADR-0119 (the 6 phase-11 ADRs — phase 12 numbers ADR-0120..ADR-0124 next-free); `docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md` + `phases/11-http-filter-local-ratelimit/PROGRESS.md` (the most-recent phase artefacts — phase 12 BRAINSTORM is structurally derived from these). The 10 empirical pins at BRAINSTORM §9 (§11.P1..§11.P10) are the SPEC author's IN-SESSION obligation per ADR-0004; the empirical-pin scrape evidence lands verbatim in SPEC §11.
- **last-commit:** TBD (SHA-fill follow-up per the phase-04..11 SHA-fill convention) — `phase 12 BRAINSTORM: http-filter-csrf [planned]`. Adds ROADMAP row 12 with status `planned`; authors `docs/envoy-go/phases/12-http-filter-csrf/BRAINSTORM.md` (~700 lines) capturing the 5-question interactive design dialogue + 5 design Decisions + 6 differential-fixture scenarios + 5 anticipated ADRs + 3 inline-deferral items + 10 empirical-pin obligations. Docs-only commit; no code changes; phase 11's six gates remain green at parent commit `0f3a710`.
- **last-updated:** 2026-05-07

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
