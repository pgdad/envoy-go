# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `<next-§9-family-row>` — phase 13 complete as of this commit (lifecycle-state-6 → awaiting next planning). Next phase is the next §9 HTTP filters family-row (compression, jwt_authn, rbac, …); determined by the next brainstorm session consulting the §9 heading + phase 13 just-shipped artefacts per ADR-0106(e). Phase 13 shipped `envoy.filters.http.buffer` — SIXTH §9 family-row; 4 ADRs (ADR-0125, ADR-0126, ADR-0127 v2 updated, ADR-0128 new); 16 differential fixtures + 17 fuzzers green; 6 gates all pass.
- **phase-directory:** `docs/envoy-go/phases/13-http-filter-buffer/` — COMPLETE. Contains BRAINSTORM.md + SPEC.md + PLAN.md + PROGRESS.md (phases 1-12 tasks documented) + REVIEW.md (pending at Task 13). The phase-done commit (this one) closes lifecycle-state-4 → 5 → awaiting-next-planning transition per BOOTSTRAP_PROMPT.md §5.
- **lifecycle-state:** `awaiting next planning` — phase 13 BRAINSTORM.md authored at parent commit `6cf412e`, amended §12 at `3915338`, SPEC.md at `f5d38fa`, PLAN.md at `a8bd93c`, implementation Tasks 1-11 landed over impl session, phase-done Task 12 at this commit. All 6 phase-done gates green (Gate A build/vet/lint; Gate B race-test 36 packages; Gate C h2spec 53/53; Gate D 17 fuzzers 30s; Gate E 16 differential fixtures; Gate F BEHAVIOR_CONTRACT populated). ROADMAP row 13 flipped `in-progress → done`. ADR roster: ADR-0125 + ADR-0126 (Task 2), ADR-0127 v2 (Task 3; in-place updated at Task 12 to retract 100-Continue addendum + reflect Continue/DataContinue landed algorithm), ADR-0128 (NEW at Task 12; framework primitives at connection.go). Task 11 pivot documented: synchronous-HCM deadlock with StopIteration replaced by Continue/DataContinue; connection.go +34 LoC framework deltas for synthetic empty-terminal + CL reconciliation. Task 13 (REVIEW.md) is the sole remaining task before branch merge.
- **next-skill:** `superpowers:brainstorming` — targeting the next §9 HTTP filters family-row (the family heading at ROADMAP line 56 enumerates the remaining members; cold-start per ADR-0106(e) from the §9 heading + phase 13 just-shipped artefacts).
- **next-skill-scope:** Cold-start scope for the next brainstorm session. Read first: `docs/envoy-go/ROADMAP.md` §9 family heading (line 56+) for the family-row ordering; `docs/envoy-go/phases/13-http-filter-buffer/SPEC.md` §1.3 (family-expansion shape) + §1.5 (no prebrainstorm-notes branch) for the cold-start discipline; `docs/envoy-go/BEHAVIOR_CONTRACT.md` `### Phase 13 forward-pointer notes` for the deferred surface from phase 13; `docs/envoy-go/DECISIONS.md` ADR-0106 (flat top-level row; next-free number ADR-0129) for the row-expansion discipline. Task 13 (REVIEW.md per `superpowers:requesting-code-review`) should complete before the next planning session opens a new brainstorm.
- **last-commit:** `TBD` — phase 13 phase-done commit (this commit). Lands: (a) BEHAVIOR_CONTRACT.md 4-edit bundle (Gate F); (b) ROADMAP.md row 13 `in-progress → done`; (c) STATE.md `awaiting next planning`; (d) DECISIONS.md: ADR-0127 v2 in-place update (Context/Decision(i)/Decision(ii)/Decision(v) retraction/Consequences amended) + ADR-0128 NEW framework-primitives ADR; (e) SPEC.md §2.5 + §4 amendments per ADR-0052; (f) PROGRESS.md Task 11 false-claim correction + Task 12 entry. All 6 gates green per Task 12 verbatim outputs.
- **last-updated:** 2026-05-09

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
