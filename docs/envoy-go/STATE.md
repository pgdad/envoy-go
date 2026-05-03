# envoy-go State

This file is the single source of truth for "what next" (see `BOOTSTRAP_PROMPT.md` §4.1 invariant 1). Cold-start reads it first. A session must update it before exiting.

---

- **active-phase:** `awaiting next planning` (BOOTSTRAP_PROMPT.md §8 MVP trunk closed at THIS commit; next-phase brainstorm session selects a feature-family row from §9).
- **phase-directory:** N/A — the MVP trunk (phases 00–08) is closed read-only history. The next session creates `docs/envoy-go/phases/<NN-family-slug>/` per §9's family-by-family expansion.
- **lifecycle-state:** `0` for the next phase (per BOOTSTRAP §5 — Phase not yet in ROADMAP.md). The next session's first action: `superpowers:brainstorming` against the §9 family list to pick the next row.
- **next-skill:** `superpowers:brainstorming` — autonomous brainstorm session selecting and scoping the first §9 feature-family phase. Inputs: `BOOTSTRAP_PROMPT.md` §9 (feature-family headings); `docs/envoy-go/ROADMAP.md` (current state); `docs/envoy-go/phases/08.2-graceful-drain/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` (just-closed sub-phase artefacts; the 08.2 brainstorm-and-spec patterns are the structural precedent for §9 family work).
- **next-skill-scope:** Lifecycle-state 0 → 1 deliverables: `docs/envoy-go/phases/<NN-family-slug>/BRAINSTORM.md` + selecting which §9 family to enter first per the user's stated priority. The §9 families: HTTP filters, Network filters, Load balancing, Upstream robustness, HTTP/3 + QUIC, gRPC, xDS / dynamic config, Observability, Runtime + hot restart, WASM host, Deprecated/edge features. The brainstorm scopes the first family as a parent phase (likely needing further splitting per ADR-0045) and adds a row to ROADMAP.md. After BRAINSTORM, advance STATE to lifecycle-state 1 with `next-skill: superpowers:writing-plans` for SPEC drafting.
- **last-commit:** `b33e04f` — `phase 08.2: graceful-drain [ADR-0091..ADR-0099]`. Lands the new internal/drain/ package + drain wiring + POST /drain_listeners + /ready + /server_info DRAINING extensions + listener Accept-loop fast-path + cluster Drain + HCM/TCP-proxy Inc/Dec hooks + cmd/envoy-go SIGTERM-handler upgrade + differential fixture 0010-graceful-drain + BEHAVIOR_CONTRACT umbrella restructure + nine new ADRs ADR-0091..ADR-0099. **ROADMAP rows 08.2 AND 08 BOTH flipped `in-progress → done` at this commit (BOOTSTRAP_PROMPT.md §8 MVP-trunk closure per parent SPEC §5).** SHA filled in a follow-up commit per the phase-04..08.1 SHA-fill convention.
- **last-updated:** 2026-05-03

---

## Lifecycle cross-reference

(unchanged — see SKILL_ROUTING.md and BOOTSTRAP §5.)

## Exit contract for every session

(unchanged.)
