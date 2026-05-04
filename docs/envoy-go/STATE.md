# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `<unset — next session resolves>` — phase 10 done at this commit. ROADMAP row 10 status flipped `in-progress → done` per BOOTSTRAP_PROMPT.md §4.1 invariant 3. Phase 10 was the THIRD §9 family-row to land (after cors @ 07.1, fault @ 09); the §9 family heading at ROADMAP line 56 stays unchanged per ADR-0106. The next §9 family-child (compression / local_ratelimit / jwt_authn / rbac / etc) is brainstormer's choice.
- **phase-directory:** `docs/envoy-go/phases/10-http-filter-header-mutation/` — contains `BRAINSTORM.md` + `SPEC.md` + `PLAN.md` + `PROGRESS.md` + `REVIEW.md` at phase-done. Phase 10 produced ~430 LoC production code (filter impl ~280 + framework deltas ~150), ~370 LoC unit tests, 50 LoC fuzzer (FuzzHeaderMutationConfigParse — thirteenth fuzzer), ~720 LoC fixture (envoy.yaml + envoy-go.yaml + driver + backend + expectations + README), 6 ADRs (ADR-0108..ADR-0113) + ADR-0073 amendment paragraph. All 17 implementation tasks completed (Task 18 REVIEW.md follows at this STATE update).
- **lifecycle-state:** `awaiting next planning` — phase 10 fully closed; six gates green at this commit (build/vet/lint clean; race tests pass — 33 packages, 0 fails; h2spec 53/53 PASS unchanged; 14 fuzzers green at 30s budget; all 13 differential fixtures 0000-0012 PASS in 39.76s; BEHAVIOR_CONTRACT.md populated with §13.1 + §13.4 + §13.5 patches).
- **next-skill:** `superpowers:brainstorming` per ADR-0106 (next §9 family-child cold-starts from the §9 heading + just-shipped phase-10 artefacts; no sibling-stub was authored).
- **next-skill-scope:** Cold-start scope for the next §9 family-child brainstorm session. Read first: `docs/envoy-go/ROADMAP.md` row 10 (now `done`) + the §9 family list at line 58; `docs/envoy-go/BEHAVIOR_CONTRACT.md ## HTTP filter chain ### envoy.filters.http.header_mutation` subsection (the just-shipped contract); `docs/envoy-go/DECISIONS.md` ADR-0108..ADR-0113 (the 6 phase-10 ADRs) + ADR-0073 amendment paragraph + ADR-0110 (multi-tier per-route accessor pattern, reusable by future filters); `docs/envoy-go/phases/10-http-filter-header-mutation/REVIEW.md` (lands in Task 18 follow-up). Per ADR-0106 + the user's persistent preference recorded in `~/.claude/projects/-home-esa-git-envoy-go/memory/MEMORY.md`: the next family-child is the brainstormer's choice from §9 family list. Per user memory: advisory off-master pre-brainstorm notes for `local_ratelimit` exist on branch `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` (pushed to origin) — surface those notes to the brainstormer if/when phase 11 targets `local_ratelimit`.
- **last-commit:** `<TBD — phase-done commit>` — `phase 10: http-filter-header-mutation [ADR-0108, ADR-0109, ADR-0110, ADR-0111, ADR-0112, ADR-0113]`. Lands envoy.filters.http.header_mutation under the 07.1 framework as the THIRD §9 family-row (after cors @ 07.1 and fault @ 09). All six phase-done gates green. Framework deltas: PerRouteConfig.ResolveAllTiers (sibling to Resolve per ADR-0073); DecoderFilterCallbacks.RequestRouteConfigsAllTiers (decoder-only per planner-time decision 1; used from both decode and encode bodies via `f.dcb`); HTTPRegistry.RegisterPerRouteValidator + BuildPerRouteConfig validator hook (eager per-route protected-header validation per planner-time decision 3 + ADR-0111). Differential fixture 0012-http-header-mutation green (4 scenarios). Zero new stats (analogous to cors per ADR-0074); 22-name table UNCHANGED. SHA-fill follow-up commit per phase-04..09 SHA-fill convention.
- **last-updated:** 2026-05-04

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
