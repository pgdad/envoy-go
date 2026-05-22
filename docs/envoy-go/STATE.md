# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `to-be-determined-at-next-session`. Phase `23-http-filter-admission-control` DONE and CANONICAL at this IMPL phase-done commit — DO NOT reopen. Phase 23 was a NEW top-level §9 family-row (`envoy.filters.http.admission_control`), the SIXTEENTH §9 HTTP-filters-family row. **BRAINSTORM done 2026-05-21** (squash-merge `3040a6b`); **SPEC done 2026-05-21** (squash-merge `a64ee71` + SHA-fill `ec68627`); **PLAN done 2026-05-22** (squash-merge `af4a0fe`); **IMPL done 2026-05-22** at this Task 12 commit via `superpowers:subagent-driven-development` (12 + 9a tasks; all 6 phase-done gates GREEN; SPEC §15 16-item acceptance checklist all verified; 3 NEW ADRs landed: ADR-0194 + ADR-0195 + ADR-0196; **D-hypothesis BROKE at Task 9a** — ADR-0196 CONSUMED by the `ResponseStatus()` encode-side accessor; next-free advances to ADR-0197; **phase-23 introduced ONE new encode-side framework primitive**: ADR-0196 `ResponseStatus() int` on `EncoderFilterCallbacks`; **PD-3 health-check arm NOT-MODELED** at MVP — documented deferral; **dead-assertion fix in fixture 0030** — `AssertSubject` was never called on cross-side path; moved to `StatsAsserter`; liveness proven via deliberate-break test; **FIRST ADR-0125-skip since phase-22's roster amendment** — canonical-per-route roster STAYS 9; **18 HTTP filters wired**; **110 stats** (was 107; +3 counters); **15 envoy-go-strict departures** (was 14; +1 RTDS `runtime_key` PARSE-REJECT per ADR-0195); **33 differential fixture dirs** (was 31; +2: `0030-http-admission-control` cross-side + `0031-http-admission-control-boot-reject`); **32 fuzzers** (was 31; +1 `FuzzAdmissionControlConfigParse`); ROADMAP row 23 flipped `in-progress → done` at this commit).
- **phase-directory:** `docs/envoy-go/phases/23-http-filter-admission-control/` (BRAINSTORM.md + SPEC.md + PLAN.md + PROGRESS.md + REVIEW.md all authored; full IMPL lifecycle complete).
- **lifecycle-state:** `phase 23 IMPL done; awaiting next-phase identification` (SKILL_ROUTING state 6). ROADMAP row `23` flipped `in-progress → done` at this commit (per-cell IMPL-done annotation). The §9 HTTP-filters family stands at **2 remaining rows** (`wasm`, `global rate limit`); phase-23 closed the `admission_control` row as planned.
- **next-skill:** `superpowers:brainstorming` (the next-phase initial step per SKILL_ROUTING state 6 + project memory; the user identifies the next §9 family-row or other phase at the next cold-start session).
- **last-commit:** `577be97` (squash-merge of branch `phase-23-http-filter-admission-control-impl` from worktree `.worktrees/phase-23-http-filter-admission-control-impl/`; backfilled by this SHA-fill follow-up per the phase-09..22 IMPL-stage close pattern). Predecessor master tip: `99c8fef` — `next-prompt.txt: repoint master-tip references to 4cd46a8`.
- **last-updated:** 2026-05-22
- **next-free ADR:** `ADR-0197` — **ADVANCED from ADR-0196 to ADR-0197** at phase-23 IMPL (ADR-0196 was CONSUMED at Task 9a by the `ResponseStatus()` encode-side framework accessor; the D-style SPEC §10 hypothesis that ADR-0196 would be UNCONSUMED BROKE; next-free unconsumed = ADR-0197). **ZERO in-place §Decision AMENDMENTs + ZERO ADR-0125 amendments** at phase 23 (REUSE-by-absence per-route; FIRST ADR-0125-skip since phase-22; canonical-per-route roster STAYS 9). DECISIONS.md tail at `ADR-0196` (full §Decision + §Consequences body); ADR-0194 + ADR-0195 + ADR-0196 all CONSUMED; ADR-0197 next-free unconsumed.

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
