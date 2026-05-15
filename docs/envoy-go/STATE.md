# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `18.2-ext-authz-grpc` — phase 18.1 IMPL session COMPLETE (state-4 entry per ADR-0005 §Decision 4). The phase-18.1 IMPL session executed the full 15-task PLAN: Tasks 1–13 authored the `internal/filter/http/extauthz/` package (HTTP service mode), differential fixture `0020-http-ext-authz-http` (7 scenarios, 21/21 fixtures green), and all 7 ADR §Decision + §Consequences bodies (ADR-0156/0157/0159/0160/0161/0162/0163); Task 14 applied the BEHAVIOR_CONTRACT 6-patch bundle + ROADMAP flip + STATE advance + 6-gate phase-done verification; Task 15 REVIEW.md is the final task of this IMPL session. ROADMAP row `18.1` is now `done` (2026-05-15). The next session is the phase-18.2 SPEC-authoring session.
- **phase-directory:** `docs/envoy-go/phases/18-http-filter-ext-authz/` (BRAINSTORM.md + parent master SPEC.md) + `docs/envoy-go/phases/18.1-ext-authz-http/` (SPEC.md + PLAN.md + PROGRESS.md + REVIEW.md; fully closed) + `docs/envoy-go/phases/18.2-ext-authz-grpc/` (sibling stub README.md; full SPEC.md pending — drafted at 18.2's lifecycle-state 1).
- **lifecycle-state:** `phase 18.1 done; phase 18.2 SPEC pending` — phase 18.1 IMPL is complete; all 6 phase-done gates passed; ROADMAP row 18.1 flipped `done`; next-free ADR is ADR-0165. The phase-18.2 SPEC session is the next lifecycle step (state-1 entry for phase 18.2: `docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md` does not yet exist → SPEC-authoring session per ADR-0005 §Decision 4 + BOOTSTRAP_PROMPT.md §5 state-1 → `superpowers:brainstorming` scoped to SPEC authoring).
- **next-skill:** `superpowers:brainstorming` (SCOPED to SPEC authoring for phase 18.2) — the next session authors `docs/envoy-go/phases/18.2-ext-authz-grpc/SPEC.md` based on the parent SPEC + the 18.1 implementation surface. Per BOOTSTRAP_PROMPT.md §5 state-1: "SPEC.md does not exist → `superpowers:brainstorming` (scoped to THIS phase)"; per phase-18 BRAINSTORM→SPEC precedent (STATE.md at commit `854fa2c`: `next-skill: superpowers:brainstorming (SCOPED to SPEC authoring)`). Cold-start scope: read `docs/envoy-go/STATE.md` (this file) + `docs/envoy-go/phases/18-http-filter-ext-authz/SPEC.md` (parent master SPEC — §§ covering gRPC mode, ADR-0158/0160-gRPC/0161-gRPC, the 18.2 scope boundaries) + `docs/envoy-go/phases/18.1-ext-authz-http/SPEC.md` (18.1 SPEC — §3 framework survey, the ADR-0159 disposition (b), the HTTP-mode portions of ADR-0160/0161 already landed) + `docs/envoy-go/DECISIONS.md` (tail at ADR-0164; next-free ADR-0165; ADR-0157 §Decision for grpc_service PARSE-REJECT amendment; ADR-0158 §Context draft; ADR-0160/0161 §Context drafts — gRPC-mode portions land at 18.2) + `docs/envoy-go/ROADMAP.md` (row 18.2 `planned` → will flip `in-progress` at SPEC commit; row 18 `in-progress`, closes when 18.2 is `done`) + `docs/envoy-go/BEHAVIOR_CONTRACT.md` (ext_authz subsection §13.1 with gRPC forward-pointer; §13.7 HTTP outbound auth-check note; `## Per-route canonical patterns cross-reference`).
- **last-commit:** `<TBD>` — the phase-18.1 IMPL squash-merge SHA is not yet known (recorded as `<TBD>` per the phase-09..17 SHA-fill follow-up precedent; a follow-up commit will fill it in post-squash to `<actual-SHA>`).
- **last-updated:** 2026-05-15
- **next-free ADR:** `ADR-0165` — phase 18.1 IMPL landed 7 ADRs (ADR-0156/0157/0159/0160/0161/0162/0163; §Decision + §Consequences bodies authored at impl-time Tasks 2/2/3/4/5/6/7 per ADR-0044; §Context drafts had already landed at SPEC commit `308e9b6`). ADR-0044 escape-valve NOT triggered (0 impl-time-unanticipated ADRs — the anticipated async-resume race-guard surface D4 → `mu`/`done` guard + `context.WithCancel` sufficed; ADR-0165 NOT authored). ADR-0158 §Context draft is ALREADY present (authored at SPEC commit per ADR-0044 ADR-on-impl convention for the gRPC-client framework primitive); its §Decision + §Consequences land at 18.2 IMPL. ADR tail is at `ADR-0164`; next-free is `ADR-0165`.

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
