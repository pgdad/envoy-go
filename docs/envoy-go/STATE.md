# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `24-http-filter-global-ratelimit` — **BRAINSTORM done 2026-05-22 at this commit; awaiting SPEC** (next session, skill `superpowers:writing-plans` scoped to SPEC authoring). Phase 24 is a NEW top-level §9 family-row (`envoy.filters.http.ratelimit`, the GLOBAL rate-limit filter), the SEVENTEENTH §9 HTTP-filters-family row. Phase `23-http-filter-admission-control` DONE and CANONICAL — DO NOT reopen. **Phase-24 BRAINSTORM outcome:** 2 user-decided Q-decisions (Q1 — FULL operator surface, SINGLE PHASE through SPEC with the ADR-0045 split-gate DEFERRED to PLAN time; split-readiness anticipated HIGH; candidate split axis 24.1/24.2 recorded as a planning anchor; Q2 — land the 10 canonical descriptor actions, PARSE-REJECT the `extension` + deprecated `dynamic_metadata` actions) + precedent-settled defaults (TWO framework deltas: a NEW `internal/grpcclient` `RateLimitClient` typed wrapper composing the existing `Dialer` per ADR-0158 [3rd two-tier wrapper] + a NEW HCM route-table capability exposing the matched Route's + VirtualHost's `rate_limits` policies via decoder-callback accessors [FIRST framework exposure of route-level non-TPFC policy data]; RTDS/runtime keying PARSE-REJECT static-only; per-route `RateLimitPerRoute` anticipated NEW **10th canonical** + ADR-0125 amendment 9 → 10 [RE-AMENDS after phase-23's skip]; byte-exact counter stat surface no gauges, 110 → ~114; FULLY-DETERMINISTIC two-directory differential via a SHARED fake gRPC `RateLimitService` dialed by both sides). Anticipated ADRs ADR-0197..ADR-0200; next-free advances 0197 → 0201; ADR-0201 hypothesized UNCONSUMED at phase-done. Fixtures 33 → 35 (`00NN-http-ratelimit` cross-side + `00NN+1-http-ratelimit-boot-reject`, hypothesized 0032+0033); fuzzers 32 → 33 (`FuzzRateLimitConfigParse`); 19 HTTP filters wired post-phase-24. Cross-phase closure pickup: phase-11 local_ratelimit's descriptor-action / X-RateLimit+vh-policy / multi-stage deferral clusters LIFTED at phase 24. §9 family closes from 2 remaining rows to **1 remaining** (`wasm`) post-phase-24.
- **phase-directory:** `docs/envoy-go/phases/24-http-filter-global-ratelimit/` (BRAINSTORM.md authored at this commit; SPEC.md + PLAN.md + PROGRESS.md + REVIEW.md not yet authored). Phase 23 directory `docs/envoy-go/phases/23-http-filter-admission-control/` is complete (full IMPL lifecycle).
- **lifecycle-state:** `phase 24 BRAINSTORM done; awaiting SPEC` (SKILL_ROUTING state 1 → next is `superpowers:writing-plans` scoped to SPEC authoring). ROADMAP row `24` added at this commit with status `in-progress` (stays `in-progress` until phase-24 IMPL phase-done). The §9 HTTP-filters family stands at **1 remaining row** post-phase-24 (`wasm`); phase-23 closed `admission_control`, phase-24 opens `global rate limit`.
- **next-skill:** `superpowers:writing-plans` (scoped to SPEC authoring per the phase-09..23 precedent; the SPEC author executes the BRAINSTORM §10 D1-D7 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004).
- **last-commit:** `<<TBD-fill-post-squash>>` (squash-merge of branch `phase-24-http-filter-global-ratelimit-brainstorm` from worktree `.worktrees/phase-24-http-filter-global-ratelimit-brainstorm/`; SHA backfilled by a follow-up per the phase-09..23 BRAINSTORM-stage close pattern). Predecessor master tip: `88101a7` — `next-prompt.txt: advance to post-phase-23-IMPL cold-start`.
- **last-updated:** 2026-05-22
- **next-free ADR:** `ADR-0197` (UNCHANGED at phase-24 BRAINSTORM — no ADRs consumed at brainstorm-time; ADR-0197..ADR-0200 are ANTICIPATED for phase 24, anchored at the SPEC commit per ADR-0044 §Context-draft discipline). DECISIONS.md tail at `ADR-0196` (full §Decision + §Consequences body); ADR-0194 + ADR-0195 + ADR-0196 all CONSUMED; ADR-0197 next-free unconsumed. Canonical-per-route roster STAYS 9 (phase-24 anticipates the 9 → 10 amendment at SPEC/IMPL via ADR-0199).

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
