# Phase 22.3 — `http-filter-lua-multi-script-and-per-route`

**Status:** **22.3 SPEC done at 2026-05-21** — see `./SPEC.md` (16-section authoritative SPEC; ran the ADR-0004 §11 5-pin empirical scrape; resolved D1-D4 IN-SESSION; surfaced **AMEND-22.3-1** [dangling per-route `name` = upstream-parity silent no-op, user-confirmed — REVISES the BRAINSTORM HCM-build-time-PARSE-REJECT disposition; drops arm 7]; anchored the ADR-0193 §Context draft → next-free ADR-0194). The other 7 LOCKED BRAINSTORM decisions STAND. See `./BRAINSTORM.md` for the 8-decision dialogue (decision #3 REVISED per AMEND-22.3-1). Next-skill: `superpowers:writing-plans` scoped to 22.3 PLAN per SKILL_ROUTING state-2 entry (SPEC → PLAN transition per the phase-22.1 + phase-22.2 precedent).

**Parent row:** `22 | http-filter-lua` (status `in-progress` per ROADMAP; closes at 22.3 IMPL phase-done).
**This sub-row:** `22.3 | http-filter-lua-multi-script-and-per-route` (status `in-progress` per ROADMAP at 22.3 BRAINSTORM-done; depends-on `22.2`).

## Anticipated scope (per parent BRAINSTORM §11.3)

Sub-phase 22.3 delivers the multi-script + per-route surface + the NEW 9th canonical per-route shape + the ADR-0125 IN-PLACE AMENDMENT (roster 8 → 9). The anticipated 22.3 surface:

- **`Lua.SourceCodes` named-script map** (consumed): per-name script compilation + cache; per-name dispatch from per-route lookups.
- **`LuaPerRoute` 3-arm oneof** (consumed):
  - `disabled: bool` — Lua filter wholly inactive on this route
  - `name: string` — string-reference into the parent `Lua.SourceCodes` map (named-script dispatch)
  - `source_code: *core.DataSource` — wholesale-override inline script (resolves to a script source string via 22.1's 4-arm DataSource resolution)
- **NEW 9th canonical per-route shape ADR** — anchors the 3-arm hybrid (combines 5th canonical's disabled-bool + 8th canonical's string-reference-delegation + DataSource-typed wholesale-override) + the SHARED stat-discipline (per-route errors charge to listener-level `lua.<config_stat_prefix>.errors`).
- **ADR-0125 IN-PLACE AMENDMENT** — `§canonical-per-route-roster` grows from 8 → 9; the 9th canonical's specification + the lua-row first-use citation. Anchored at 22.3 IMPL final Task per ADR-0044.
- **Per-route 3-tier dispatch** — reuses the existing 3-tier resolution from phase 13/14/15 + extends with the Name string-reference-delegation pattern (the 8th canonical's discipline).
- **Differential fixture `0028-http-lua-multi-script-and-per-route`** — cross-side byte-exact for the deterministic multi-script + per-route scenarios.

## Anticipated artefacts (filled in at this sub-phase's own session-set)

- `BRAINSTORM.md` — (optional) sub-phase BRAINSTORM
- `SPEC.md` — authored at the dedicated 22.3 SPEC session
- `PLAN.md` — authored at the dedicated 22.3 PLAN session
- `PROGRESS.md` — per-task entries authored during 22.3 IMPL session(s)
- `REVIEW.md` — authored at 22.3 IMPL phase-done

## D-hypothesis (provisional; 22.3 BRAINSTORM re-evaluates)

Anticipated 22.3 IMPL ADRs: ~1-2 NEW ADRs (NEW 9th canonical per-route shape + per-route 3-tier dispatch) + 1 IN-PLACE AMENDMENT (ADR-0125 roster 8 → 9). 22.3 BRAINSTORM re-evaluates the hypothesis disposition after the 22.2 IMPL outcomes are known.

## Phase 22 closure at 22.3 IMPL phase-done

Phase 22 (parent row) flips `in-progress → done` at the 22.3 IMPL Task that lands ROADMAP row 22's per-cell IMPL-done annotation. The §9 HTTP-filters family closes from 4 remaining rows (post-phase-21) to 3 remaining rows (post-phase-22): `wasm`, `admission_control`, `global rate limit`.

## Cross-references

- `../22-http-filter-lua/BRAINSTORM.md` — parent BRAINSTORM (load-bearing for the 9th canonical specification + the SHARED stat-discipline rationale)
- `../22.1-http-filter-lua-vm-and-headers-bridge/README.md` — first sub-phase
- `../22.2-http-filter-lua-full-bridge/README.md` — predecessor sub-phase

**This README is descriptive of anticipated scope, not prescriptive of design.**
