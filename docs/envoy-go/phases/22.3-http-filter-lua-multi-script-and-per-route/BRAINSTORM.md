# Phase 22.3 Brainstorm — `envoy.filters.http.lua` (multi-script + per-route surface)

**Sub-phase:** `22.3-http-filter-lua-multi-script-and-per-route` (third + final sub-phase of parent 22 per parent BRAINSTORM Q2 PRE-SPLIT)
**Parent row:** `22-http-filter-lua` (status `in-progress` per ROADMAP; parent row flips `in-progress → done` at the 22.3 IMPL Task per parent SPEC §1 closure pattern)
**Predecessor:** `22.2-http-filter-lua-full-bridge` (status `done` at 2026-05-19; landed the full Envoy↔Lua bridge surface delta + NEW `internal/dynamicmetadata/` primitive + `internal/lua/` coroutine + body-bridge extensions; 17 HTTP filters wired; 107 stat names; 30 fuzzers; 29 differential fixture dirs; DECISIONS.md tail at ADR-0192; next-free ADR-0193)
**Successor:** none — 22.3 is the last sub-phase; parent row 22 closes at 22.3 IMPL phase-done
**Authored at:** this BRAINSTORM commit (squash-merged to master per project memory `feedback_git_worktrees.md` + ADR-0005 §Decision 4)

This sub-phase BRAINSTORM is the per-sub-phase BRAINSTORM convention per ADR-0106 + matches the discipline shape of the parent 22 BRAINSTORM (`../22-http-filter-lua/BRAINSTORM.md` — 12 Qs) + the 22.2 BRAINSTORM (`../22.2-http-filter-lua-full-bridge/BRAINSTORM.md` — 14 Qs). Parent BRAINSTORM Q2 settled the 3-way pre-split (22.1 + 22.2 + 22.3); parent BRAINSTORM Q7 settled the NEW 9th canonical per-route classification + the SHARED stat-discipline; parent SPEC §3.1 + §4.5 + §5.2 + §6.3 settled the surface-mapping + the ADR-0125 §(xiv) AMENDMENT-anticipation + the `LuaPerRoute` proto roster + the ~20 forward-pointer PARSE-REJECT arms. **This BRAINSTORM settles the genuinely-open 22.3 decisions the parent left to this session** — the ones that were NOT pre-resolved by Q7 + the parent SPEC forward-pointers.

---

## 1. Mission and scope confirmation (22.3 only)

### 1.1 What 22.3 delivers as a self-contained whole

Phase 22.3 lifts the LAST two PARSE-REJECT surfaces in the lua filter — `Lua.SourceCodes` (the named-script map) + `LuaPerRoute` (the 3-arm per-route override oneof) — taking parent BRAINSTORM Q1 envelope D to its FULL conclusion. By 22.3 phase-done, every field in the `Lua` + `LuaPerRoute` v1.32.4 proto rosters is CONSUMED (except the two v1.37.2 binding-gap fields `Lua.clear_route_cache` + `LuaPerRoute.filter_context`, which stay forward-pointers per AMEND-12), and the lua filter joins the per-route-capable §9 filter cohort with a structurally-novel 9th canonical per-route pattern.

The 22.3 surface delta (2 proto surfaces + the dispatch + the canonical-roster amendment):

- **`Lua.SourceCodes` multi-script map activation** (per §2.1): the `map<string, DataSource>` field 2 is consumed — each entry is a named script whose `DataSource` value resolves (via the SAME 22.1 4-arm DataSource resolution: Filename + InlineBytes + InlineString + EnvironmentVariable; WatchedDirectory PARSE-REJECT) + compiles to a `*Chunk`. Named scripts are dispatch targets for per-route `name` references; they do NOT run on any route by themselves (only `DefaultSourceCode` is the listener-level default).
- **`LuaPerRoute` 3-arm oneof activation** (per §2.2): the `override` oneof (PGV `required`) is consumed at per-route resolve time with three arms — `disabled:bool` (filter wholly inactive on this route) + `name:string` (string-reference-delegation into the listener-level `SourceCodes` map) + `source_code:*DataSource` (wholesale-override inline script). The NEW 9th canonical per-route shape per ADR-0125 §(xiv).
- **Per-route 3-tier dispatch** (per §2.3): per-route override → listener-level `DefaultSourceCode` → no-op. Reuses the existing 3-tier resolution from phase 13/14/15 + extends with the phase-17 jwt_authn 8th-canonical `name` string-reference-delegation discipline.
- **NEW 9th canonical per-route shape** (per §2.4 + §4): the 3-arm hybrid combines the 5th canonical's `disabled-bool` + the 8th canonical's `string-reference-delegation` + a novel `DataSource`-typed `wholesale-override` in a single oneof — structurally distinct from all 8 prior canonicals. ADR-0125's `§canonical-per-route-roster` grows from 8 → 9 via in-place §(xiv) AMENDMENT body at 22.3 IMPL final Task.

Plus transverse decisions:

- **ADR structure** (per §2.5 + Q1): ONE combined NEW ADR (ADR-0193) covers the SourceCodes multi-script activation AND the LuaPerRoute 9th-canonical 3-tier dispatch (one cohesive operator surface — multi-script + per-route are tightly coupled because per-route `name` delegates INTO `SourceCodes`). Plus the ADR-0125 §(xiv) in-place AMENDMENT (no number consumed). Matches 22.1's one-package-shape-ADR economy (ADR-0189).
- **Reserved-name discipline** (per §2.6 + Q2): NONE. `default_source_code` (field 3) + `source_codes` (field 2) are independent proto fields — no `SourceCodes` key can collide with the default. Free-form keys; only the `source-codes-key-empty` PARSE-REJECT arm applies. Matches upstream Envoy (no reserved-name discipline in `lua_filter.cc`). Drops parent SPEC §6.3 forward-pointer arm 3.
- **Name dangling-reference resolution** (per §2.7 + Q5): HCM-build-time cross-resolution — after both listener-level `SourceCodes` + the per-route `LuaPerRoute` are parsed, a per-route `name` that references a key absent from `SourceCodes` PARSE-REJECTs at HCM filter-chain build time (parent SPEC §6.3 arm 7). Fail-fast; mirrors the phase-17 jwt_authn 8th-canonical `provider_name` dangling-reference discipline.
- **Stat surface** (per §2.8 + Q-inherited): 0 net-new counters. SHARED-vacuous per the 9th canonical's SHARED stat-discipline (per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors`; `LuaPerRoute` has no separate `stat_prefix` field). Stat count STAYS 107.
- **Fixture-0028 strategy** (per §2.9 + Q3): single `0028-http-lua-multi-script-and-per-route` directory; multi-scenario cross-side byte-exact for the 5 deterministic tiers (listener-default / per-route-name→named-script / per-route-source_code-override / per-route-disabled / multi-script-selection). 29 → 30 fixture directories.
- **Fuzzer** (per §2.10 + Q4): +1 NEW `FuzzLuaPerRouteConfig` (fuzzes the 3-arm `LuaPerRoute` oneof at parse). SourceCodes-map parse coverage folds into the existing `FuzzLuaConfigParse` seed corpus. 30 → 31 fuzzers.
- **D-hypothesis** (per §2.11 + Q7): WEAK HOLD — 1 combined NEW ADR-0193 + ADR-0125 §(xiv) AMENDMENT land cleanly + 0-1 escape-valve consumption (conditional ADR-0194 — fires only if per-route `*Chunk` lookup + per-route resolution push per-stream construction over the 1 ms gate, re-evaluating R6). Matches 22.1 + 22.2 WEAK-HOLD precedent (both held at 0 escape-valve consumption).
- **Scope shape** (per §2.12 + Q8): single-phase 22.3 at this BRAINSTORM; the PLAN session fires the ADR-0045 split-gate only if estimates exceed (~25 tasks / ~1500 LoC).

### 1.2 What 22.3 does NOT deliver (forward to future / cross-phase)

Items DEFERRED to future phases (cross-phase boundaries):

- **`:metadata()` per-route source activation** — STAYS empty-table at the v1.32.4 binding-gap. 22.3's `LuaPerRoute` PARSE-LIFT does NOT activate `:metadata()` data — the v1.32.4 binding lacks `LuaPerRoute.filter_context` (the field that would carry the per-route metadata source per AMEND-12 + parent SPEC §11.1.4). The `:metadata()` bridge surface (callable; returns empty) activates with real data only at the future v1.32.4 → v1.37.x binding-bump phase. **22.3 does NOT pull `filter_context` forward** (it does not exist in v1.32.4 bindings).
- **`Lua.clear_route_cache` (v1.37.2 field 5; v1.32.4 binding-gap)** — STAYS forward-pointer per AMEND-12 + parent SPEC §5.4. Route-cache invalidation semantics activate at the binding-bump phase.
- **Multi-decoder "park-headers-iteration-pending-body" cooperative discipline** — the "Continue-on-body-yield" trade-off flagged at 22.2 REVIEW.md §6.1 is NOT a 22.3 scope item. Per-route applies to a single lua filter instance at a time; 22.3 introduces no in-tree multi-lua-decoder topology. Deferred to a separate framework phase.
- **`internal/filterstate/` framework primitive extraction** — `:filterState()` STAYS IN-PACKAGE at the lua filter (landed at 22.2 per Q8 + Q9). A future phase that adds a second filter-state consumer extracts the primitive per the EXTRACT-NOW-only-when-trigger-fires lesson.
- **Cluster-specifier Lua + access-logger Lua + string-matcher Lua** — future cross-family phases; consumers #2/3/4 for the `internal/lua/` framework primitive. Each future phase BRAINSTORM revisits the API shape per ADR-0188's EXPLICIT API-REVISION ALLOWANCE clause (STAYS scoped to consumer-#2; UNCHANGED by 22.3 which is still consumer-#1).

### 1.3 22.3's relationship to parent 22 BRAINSTORM Q1 envelope D + Q7 9th-canonical hand-off

Parent BRAINSTORM Q1 settled the ambition: envelope D = full upstream parity by phase-22 phase-done across the 3-way pre-split. Parent BRAINSTORM Q7 settled the per-route classification: NEW 9th canonical (3-arm hybrid). 22.3 takes Q1's final delta (multi-script + per-route) + lands Q7's 9th canonical as a concrete proto-consume + dispatch surface.

22.3 is structurally significant within the §9 family:

1. **ENDS the FOUR-CONSECUTIVE ADR-0125-skip streak** (phases 18 / 19 / 20 / 21 all skipped the canonical-roster amendment) — the 9th canonical lands at 22.3 IMPL via ADR-0125 §(xiv) AMENDMENT, the FIRST canonical-roster growth since phase-17 jwt_authn's 8th canonical.
2. **CLOSES parent row 22** — the parent row flips `in-progress → done` at the 22.3 IMPL Task that lands ROADMAP row 22's per-cell IMPL-done annotation. The §9 HTTP-filters family closes from 4 remaining rows (post-phase-21) to 3 remaining rows (post-phase-22): `wasm`, `admission_control`, `global rate limit`.
3. **NO new framework primitive + NO new stat** — 22.3 is a CONSUME + DISPATCH phase. It consumes 2 already-parsed-then-rejected proto surfaces, reuses the 22.1 DataSource resolution + compile-cache discipline, reuses the existing per-route 3-tier resolution from phase 13/14/15, and reuses the phase-17 8th-canonical `name`-delegation discipline. The framework footprint is unchanged (0 new `internal/*` packages); the only NEW ADR (ADR-0193) documents package-shape consumption + the canonical classification, not a primitive.

### 1.4 ADR-0045 split-by-surface readiness — staying single-phase at BRAINSTORM (Q8)

Per Q8 + parent SPEC §3.0: 22.3 stays as one ROADMAP row at this BRAINSTORM commit. Parent SPEC §3.0 re-estimated each sub-phase at 14-18 tasks / ~1200-1500 LoC after the §11 empirical scrape — well under the ADR-0045 split-gate (~25 tasks / ~1500 LoC). 22.3 is the SMALLEST of the three sub-phases (no new bridge methods; no new framework primitive; no new stats — just 2 proto-surface consumes + dispatch + 1 differential fixture + 1 fuzzer + the canonical amendment). The PLAN session does the precise estimation against the gate; if it exceeds, 22.3 splits at PLAN per the phase-09 → phase-11 + phase-13 split-at-PLAN precedent (ROADMAP + STATE update; BRAINSTORM not invalidated).

Pre-splitting at BRAINSTORM was rejected (Q8 option "Pre-split now") on coupling grounds — the two surfaces are tightly coupled (per-route `name` delegates INTO `SourceCodes`; the dangling-reference resolution requires BOTH surfaces parsed), so there is no clean split axis (a 22.3.1-multi-script + 22.3.2-per-route cut would force the dangling-reference cross-resolution to straddle the sub-phase boundary).

### 1.5 Phase 22.2 IMPL inheritance state

22.3 inherits the following state from 22.2 IMPL (master tip `2821f0d` = `phase 22.2 IMPL follow-up: STATE.md SHA-fill (TBD → 46183a4 post-squash)`):

- **17 HTTP filters wired** — `envoy.filters.http.lua` is the 15th §9 family-row; 22.3 does NOT add a §9 row (extends the same lua filter — per-route + multi-script are config-surface extensions, not new filters). Boot-registration alphabetical position between `localratelimit` and `oauth2` UNCHANGED.
- **107 stat names** — 0 net-new at 22.3 (SHARED-vacuous per the 9th canonical).
- **30 fuzzers** — 22.3 anticipates +1 (`FuzzLuaPerRouteConfig` → 31).
- **29 differential fixture directories** — 22.3 anticipates +1 (`0028-http-lua-multi-script-and-per-route` → 30).
- **DECISIONS.md tail at ADR-0192** with full §Decision + §Consequences bodies; **next-free ADR-0193** carries forward UNCONSUMED from 22.2 IMPL (R6 STANDS WEAK-default at `ns/op = 98157` ~98µs/stream at the FULL 22.2 bridge surface; R9 STAYS embedded in ADR-0192). 22.3 anticipates ADR-0193 consumption for the combined multi-script + per-route ADR (per Q1) + conditional ADR-0194 escape-valve (per Q7).
- **ADR-0125 §(xiv) AMENDMENT-anticipation paragraph** anchored at parent SPEC commit (DECISIONS.md ~line for ADR-0125) STANDS UNCHANGED at 22.3 BRAINSTORM; the AMENDMENT body lands at 22.3 IMPL final Task per ADR-0044 in-place edit discipline.
- **`internal/lua/` framework primitive** anchored at ADR-0188 (+ ADR-0191 22.2 extensions); 22.3 is NOT anticipated to extend it (per-route is dispatch-table mutation + named-script compilation, not VM-API-touching). ADR-0188's API-REVISION ALLOWANCE STAYS scoped to consumer-#2.
- **`internal/filter/http/lua/` package shape** anchored at ADR-0189 (+ ADR-0192 22.2 extensions); 22.3 EXTENDS via NEW ADR-0193 (SourceCodes named-script map consume + LuaPerRoute 9th-canonical parse + per-route 3-tier dispatch).
- **19-arm PARSE-REJECT roster** at `internal/filter/http/lua/` (18 SPEC-time + 1 fuzzer-surfaced at 22.1; UNCHANGED at 22.2 — the 3 NEW 22.2 arms 20-22 are RUNTIME-REJECTs via `luaL_error`, not config-load PARSE-REJECTs). 22.3 lifts arm 4 (`SourceCodes` map) + arm 18 (`LuaPerRoute`) from PARSE-REJECT to CONSUMED, and adds the ~19 forward-pointer config-load arms per parent SPEC §6.3 (minus the dropped reserved-name arm 3 per §2.6).

### 1.6 Cross-phase dynamic-metadata deferral discipline — already broken at 22.2 (no 22.3 action)

Per the 22.2 BRAINSTORM §1.6 + REVIEW.md §8: 22.2 already broke the cross-phase dynamic-metadata deferral discipline by landing the `internal/dynamicmetadata/` primitive. 22.3 takes NO action on this — the per-route surfaces inherit the cross-filter visibility from the existing primitive (per-route override scripts can call `:streamInfo():dynamicMetadata()` exactly as the listener-default script can). 22.3 should NOT defer dynamic-metadata anew (the primitive exists at 22.2 phase-done).

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

### 2.1 `Lua.SourceCodes` multi-script map activation *(→ ADR-0193)*

**Decision:** Consume the `Lua.SourceCodes` `map<string, DataSource>` field. Each entry `(name → DataSource)` resolves its `DataSource` value via the SAME 22.1 4-arm resolution (`resolveDataSource`: Filename + InlineBytes + InlineString + EnvironmentVariable; WatchedDirectory PARSE-REJECT; empty-specifier-oneof PARSE-REJECT) + compiles to a `*Chunk` at config-load time. The named scripts populate a `name → *Chunk` (or `name → resolved-source` per the compile-cache disposition deferred to SPEC — see §2.13 D1) lookup consumed by per-route `name` references. Named scripts are dispatch TARGETS only — a named script does NOT execute on any route unless a `LuaPerRoute.name` arm references it; `DefaultSourceCode` remains the sole listener-level default.

**Rationale:** This is the upstream Envoy semantic (`source_codes` is a named-script registry; routes opt in via `LuaPerRoute.name`). The alternative — running all named scripts on every route — was never on the table (it contradicts the upstream registry semantic + would be a gross perf regression). Per-name compile-at-config-load (vs lazy compile-at-first-dispatch) matches the 22.1 `DefaultSourceCode` compile-at-config-load discipline (fail-fast on compile error per the existing `script-compile-failure` PARSE-REJECT arm).

**Anticipated ADRs:** ADR-0193 §Decision documents the SourceCodes map consume shape — per-name DataSource resolution reuse + per-name compilation + the `name → chunk` lookup structure + the interaction with the 22.1 compile cache (the exact cache keying deferred to 22.3 SPEC per §2.13 D1).

### 2.2 `LuaPerRoute` 3-arm oneof activation *(→ ADR-0193 + ADR-0125 §(xiv) AMENDMENT)*

**Decision:** Consume the `LuaPerRoute.override` oneof (PGV `(validate.required) = true`) with three arms per parent SPEC §5.2:

- `disabled` (bool; PGV `const: true`) — the lua filter is wholly inactive on this route (both `envoy_on_request` + `envoy_on_response` hooks skipped; no-op). PARSE-REJECT `disabled: false` (PGV-mirror per parent SPEC §6.3 arm 5).
- `name` (string; PGV `min_len: 1`) — string-reference-delegation into the listener-level `Lua.SourceCodes` map; the named script runs for this route instead of `DefaultSourceCode`. PARSE-REJECT empty name (PGV-mirror arm 6) + PARSE-REJECT dangling name (cross-resolution arm 7 per §2.7).
- `source_code` (`*core.DataSource`) — wholesale-override inline script; resolves via the SAME 22.1 4-arm DataSource resolution. PARSE-REJECT per the 8-arm DataSource gauntlet (arm 8) with wording prefix `"lua: per-route: source_code: ..."`.

**`source_code` arm DataSource scope (self-answered against precedent; confirmed at this BRAINSTORM):** the per-route `source_code` arm honors the FULL 4-arm DataSource roster (Filename + InlineBytes + InlineString + EnvironmentVariable) + WatchedDirectory PARSE-REJECT — FORCED by proto type identity (`source_code` is the same `*core.DataSource` type as `DefaultSourceCode`; the resolution code path is shared). No restriction; no envoy-go-strict cut.

**Rationale:** The 3-arm consume is the direct realization of parent BRAINSTORM Q7's 9th-canonical classification. The COMPOUND-mapping + EXTENSION-of-5th alternatives were already rejected at parent Q7 (classification-signal + misclassification grounds); 22.3 does not re-litigate. The `disabled: true`-means-skip-both-hooks semantic matches upstream `lua_filter.cc` per-route disable.

**Anticipated ADRs:** ADR-0193 §Decision documents the `LuaPerRoute` parse + per-arm validation + the per-route resolution dispatch. ADR-0125 §(xiv) in-place AMENDMENT body (lands at 22.3 IMPL final Task per ADR-0044) adds the 9th canonical's specification + the lua-row first-use citation; ADR-0125's `§canonical-per-route-roster` grows 8 → 9.

### 2.3 Per-route 3-tier dispatch *(→ ADR-0193)*

**Decision:** Per-route resolution dispatches in 3 tiers per parent BRAINSTORM §11.3(d) + parent SPEC §3.1:

1. **Per-route override present** → resolve the `override` oneof: `disabled` → no-op (skip both hooks); `name` → look up the named `*Chunk` in the listener-level `SourceCodes` registry + run it; `source_code` → run the per-route inline-override `*Chunk`.
2. **No per-route override** → run the listener-level `DefaultSourceCode` (the 22.1 default behavior).
3. **No per-route override AND no `DefaultSourceCode`** → no-op (existing 22.1 disposition).

The dispatch reuses the existing per-route 3-tier resolution mechanism from phase 13/14/15 (the project's `typed_per_filter_config` → per-route-config resolution path) + extends with the phase-17 jwt_authn 8th-canonical `name` string-reference-delegation discipline (the `name` → registry lookup happens against the listener-level `SourceCodes` map, NOT a per-route-local map).

**Rationale:** Reuse over reinvention — the project already has a battle-tested 3-tier per-route resolution; 22.3 plugs the lua filter's 9th-canonical resolution into it. The `name`-delegation-into-listener-`SourceCodes` (vs a per-route-local script registry) matches both upstream Envoy + the phase-17 8th-canonical precedent (where per-route `provider_name` references the listener-level provider registry).

**Anticipated ADRs:** ADR-0193 §Decision documents the 3-tier dispatch + the `typed_per_filter_config` wiring (reuse of the existing per-route mechanism) + the per-route `*filter` construction discipline (which `*Chunk` the per-stream filter binds to at resolve time).

### 2.4 NEW 9th canonical per-route shape *(→ ADR-0193 anchors classification + ADR-0125 §(xiv) AMENDMENT records roster growth)*

**Decision:** Classify `LuaPerRoute` as the NEW 9th canonical per-route pattern per parent BRAINSTORM Q7. The 3-arm hybrid combines three structural patterns from prior canonicals in a single `override` oneof:

- `disabled-bool` (mirrors the 5th canonical's disabled-bool arm)
- `string-reference-delegation` (mirrors the 8th canonical's `name`-into-registry pattern)
- `DataSource-typed wholesale-override` (NOVEL — no prior canonical uses a `core.DataSource`-typed wholesale-override arm; the 5th canonical's wholesale-override uses a parent-config sub-message, not a `DataSource`)

The 3-arm combination is structurally distinct from all 8 prior canonicals — no prior canonical has all three patterns in one oneof. The 9th canonical's stat-discipline is SHARED (per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors`; matches phase-17 jwt_authn 8th-canonical SHARED per ADR-0154 — applies because `LuaPerRoute` has no separate `stat_prefix` field).

**Rationale:** Settled at parent Q7; 22.3 does not re-decide the classification. This BRAINSTORM confirms the classification holds against the concrete proto-consume surface (no surprises surfaced that would re-collapse the 9th canonical to a COMPOUND-mapping or an EXTENSION-of-5th).

**Anticipated ADRs:** NEW ADR-0193 anchors the classification statement (cross-referencing ADR-0125 §(xiv)); the ADR-0125 §(xiv) in-place AMENDMENT body lands the full 9-shape roster table + the per-row first-use citation at 22.3 IMPL final Task.

### 2.5 ADR structure: ONE combined NEW ADR-0193 *(Q1 → ADR-0193 + ADR-0125 §(xiv) AMENDMENT)*

**Decision:** ONE combined NEW ADR (ADR-0193) covers BOTH the `SourceCodes` multi-script activation AND the `LuaPerRoute` 9th-canonical 3-tier dispatch — they are one cohesive operator surface (per-route `name` delegates INTO `SourceCodes`; the dangling-reference cross-resolution requires both). Plus the ADR-0125 §(xiv) in-place AMENDMENT (no new number consumed per ADR-0044). Title (provisional): "`internal/filter/http/lua/` 22.3 multi-script + per-route surface — `Lua.SourceCodes` named-script map consume + `LuaPerRoute` 9th-canonical 3-arm oneof + per-route 3-tier dispatch".

**Rationale:** The 2-separate-ADRs alternative (Q1 option 2) was rejected on coupling-+-economy grounds — splitting SourceCodes from LuaPerRoute would force cross-references between two ADRs for a single tightly-coupled surface (the dangling-reference resolution spans both), and would consume ADR-0193 + ADR-0194 where ADR-0194 is more valuably held as the conditional escape-valve slot (per §2.11). The defer-to-SPEC alternative (Q1 option 3) was rejected because the ADR count is a design-shape decision the BRAINSTORM is well-positioned to settle (the surfaces are known + pre-classified). Matches 22.1's one-package-shape-ADR economy (ADR-0189 covered the entire 22.1 filter package shape + DataSource resolution + stat departures + fixture discipline in one ADR).

**Anticipated ADRs:** ADR-0193 §Context block authored at 22.3 SPEC commit per ADR-0044 §Context-draft discipline; §Decision + §Consequences bodies anchored at 22.3 IMPL atomic-landing Task. ADR-0125 §(xiv) AMENDMENT body lands at the same 22.3 IMPL final Task.

### 2.6 Reserved-name discipline: NONE *(Q2 → drops parent SPEC §6.3 forward-pointer arm 3)*

**Decision:** NO reserved-name collision discipline for `SourceCodes` map keys. `default_source_code` (field 3) + `source_codes` (field 2) are independent proto fields — a `SourceCodes` key cannot collide with the default (the default is not addressed by name). Map keys are free-form; only the `source-codes-key-empty` PARSE-REJECT arm (parent SPEC §6.3 arm 2) applies. **Drops parent SPEC §6.3 forward-pointer arm 3** (`source-codes-key-duplicate-of-default`) — it never becomes a real arm.

**Rationale:** The reserve-a-name alternative (Q2 option 2) was rejected on upstream-parity + cost grounds — upstream Envoy `lua_filter.cc` has NO reserved-name discipline (named scripts + default are independent), so reserving a name would be an envoy-go-strict departure requiring a BEHAVIOR_CONTRACT.md departure record for zero operator-protection value (there is no collision to protect against). The defer-to-SPEC alternative (Q2 option 3) is unnecessary — the proto-field independence is already established at parent SPEC §5.1 (the two fields are separate; no name addresses the default), so the SPEC scrape would only confirm the no-reserved-name disposition.

**Anticipated ADRs:** ADR-0193 §Decision documents the no-reserved-name disposition (cross-referencing the parent SPEC §6.3 arm-3 drop). The PARSE-REJECT roster note: combined 22.1+22.3 config-load roster is ~19 forward-pointer arms (parent SPEC §6.3's ~20 minus the dropped arm 3) on top of the 19-arm 22.1 roster.

### 2.7 Name dangling-reference resolution: HCM-build-time cross-resolution *(Q5 → ADR-0193)*

**Decision:** A per-route `LuaPerRoute.name` that references a key ABSENT from the listener-level `Lua.SourceCodes` map PARSE-REJECTs at HCM filter-chain build time (parent SPEC §6.3 arm 7) — AFTER both the listener-level `SourceCodes` map + the per-route `LuaPerRoute` are parsed. Wording: `"lua: per-route: name %q is not defined in source_codes"`. Fail-fast at config load.

**Rationale:** The request-time-lookup alternative (Q5 option 2) was rejected on fail-fast + precedent grounds — deferring the dangling-name failure to traffic time diverges from the phase-17 jwt_authn 8th-canonical `provider_name` dangling-reference discipline (which fails at config-build time) + loses the operator's config-load-time error signal (a typo'd `name` would silently no-op or runtime-error on first matching request). HCM-build-time (vs pure parse-time) is REQUIRED because the cross-resolution needs BOTH the listener-level `SourceCodes` map (parsed from the `Lua` config) + the per-route `LuaPerRoute` (parsed from `typed_per_filter_config`) — these are parsed at different points; the cross-check can only run once both are available, i.e., at HCM filter-chain build time.

**Anticipated ADRs:** ADR-0193 §Decision documents the HCM-build-time cross-resolution timing + the dangling-reference PARSE-REJECT arm wording.

### 2.8 Stat surface: 0 net-new (SHARED-vacuous) *(inherited from parent Q7 + SPEC §7.3)*

**Decision:** 0 net-new counters at 22.3. The 9th canonical's stat-discipline is SHARED (per ADR-0154 + parent BRAINSTORM Q7 + parent SPEC §7.3) — per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors`; `LuaPerRoute` has no separate `stat_prefix` field, so there is no per-route stat namespace. Stat count STAYS 107.

**Rationale:** Settled at parent Q7 + SPEC §7.3 (which explicitly notes "22.3 anticipated additions: 0 (SHARED-vacuous)"). 22.3 does not re-decide. A per-named-script counter cohort (e.g., `source_codes_<name>_executions`) was considered + rejected at parent BRAINSTORM §2.8 forward-pointer (operators rarely need per-name execution counts; the SHARED `executions` counter already aggregates all script invocations regardless of which named/default/override script ran).

**Anticipated ADRs:** None for stats (0 net-new). The SHARED-stat-discipline rationale is recorded in the ADR-0125 §(xiv) AMENDMENT body (per the 9th canonical's SHARED-stat specification).

### 2.9 Differential fixture: single fixture-0028, multi-scenario cross-side byte-exact *(Q3 → ADR-0193)*

**Decision:** ONE fixture directory `test/fixtures/0028-http-lua-multi-script-and-per-route/` with multi-scenario cross-side byte-exact (`CompareBytes`) coverage for the 5 deterministic per-route tiers:

| Scenario | Surface | Mode |
|---|---|---|
| (a) listener-default, no per-route | `DefaultSourceCode` runs (22.1 baseline behavior) | cross-side `CompareBytes` |
| (b) per-route `name` → named script | `LuaPerRoute.name` delegates into `SourceCodes` | cross-side `CompareBytes` |
| (c) per-route `source_code` wholesale-override | `LuaPerRoute.source_code` inline-override script | cross-side `CompareBytes` |
| (d) per-route `disabled` | `LuaPerRoute.disabled: true` → filter no-op on route | cross-side `CompareBytes` |
| (e) multi-script `SourceCodes` selection | two routes select two different named scripts | cross-side `CompareBytes` |

29 → 30 fixture directories. Reuses the 22.1 `BackendKind=HTTPLua` + `scripts/` subdirectory pattern (no new BackendKind). The deterministic scenarios all produce byte-exact wire output (header mutations / direct responses driven by the dispatched script) — no `:timestamp()` / `:httpCall()` non-determinism (those are 22.2-surface; 22.3 scenarios use header-mutation + respond scripts).

**Rationale:** The minimal-cross-side alternative (Q3 option 2) was rejected on envelope-D-verification grounds — 22.3 closes the parent row; the per-route dispatch is the headline deliverable and deserves full cross-side byte-exact verification across all 5 tiers (a 1-2 scenario cut would leave the multi-script-selection + source_code-override tiers verified only at the unit level, losing the cross-side envelope-D signal for the row-closing phase). The defer-taxonomy-to-SPEC alternative (Q3 option 3) is partially honored — the BRAINSTORM commits to the single-directory + cross-side discipline + the 5-tier scenario sketch; the 22.3 SPEC scrubs the exact scenario roster (may add boot-reject scenarios for the NEW config-load PARSE-REJECT arms: dangling-name + key-empty + per-route-source_code-DataSource-failure).

**Anticipated ADRs:** ADR-0193 §Decision documents the fixture-0028 scenario taxonomy + the per-scenario cross-side discipline. Fixture-0028 is fully cross-side byte-exact (no REFERENCE-LESS subject-only scenarios — the 22.3 surfaces are all deterministic on the wire, unlike 22.2's httpCall/timestamp).

### 2.10 Fuzzer: +1 NEW `FuzzLuaPerRouteConfig` *(Q4 → 30 → 31)*

**Decision:** Add ONE NEW fuzzer `FuzzLuaPerRouteConfig` (fuzzes the 3-arm `LuaPerRoute` oneof at parse — all three arms + the empty-oneof + the per-arm PGV violations + the `source_code` DataSource arm). The `SourceCodes` map parse coverage FOLDS INTO the existing `FuzzLuaConfigParse` seed corpus (extend with `source_codes` map entries + named-script DataSource arms). Project fuzzer count 30 → 31. ADR-0018 must-never-panic baseline.

**Rationale:** The two-new-fuzzers alternative (Q4 option 2 — separate `FuzzLuaSourceCodesConfig`) was rejected on fuzzer-economy grounds — the `SourceCodes` map is parsed by the SAME config-parse entry point as the rest of the `Lua` message (it is field 2 of `Lua`); extending the existing `FuzzLuaConfigParse` corpus with `source_codes` seeds covers the map parse without a separate fuzzer. `LuaPerRoute` warrants its own fuzzer because it is a DISTINCT proto message (parsed from `typed_per_filter_config`, not the `Lua` message) with its own parse entry point + the dangling-reference cross-resolution. The none-new alternative (Q4 option 3) was rejected because `LuaPerRoute`'s distinct parse entry point + the 3-arm oneof's wider parse-state-space warrant a dedicated fuzzer (the dangling-reference resolution + the `source_code` DataSource gauntlet are non-trivial parse surfaces).

**Anticipated ADRs:** ADR-0193 §Decision (or the 22.3 SPEC §13-R-equivalent) documents the fuzzer-count delta + the corpus-seed roster for `FuzzLuaPerRouteConfig` + the `FuzzLuaConfigParse` corpus extension.

### 2.11 D-hypothesis: WEAK HOLD *(Q7 → 1 NEW ADR-0193 + AMENDMENT + 0-1 escape-valve at ADR-0194)*

**Decision:** BRAINSTORM-time prediction for 22.3 IMPL: WEAK HOLD — 1 combined NEW ADR-0193 (multi-script + 9th canonical + 3-tier dispatch) + the ADR-0125 §(xiv) in-place AMENDMENT (no number consumed) land cleanly + 0-1 escape-valve consumption (conditional ADR-0194). The most likely escape-valve surface at 22.3 IMPL: the **R6 *LState-pool gate re-evaluation** — per-route `*Chunk` lookup + per-route resolution add a small per-stream cost on top of the 22.2 baseline (`ns/op = 98157` ~98µs/stream); if the per-route resolution path pushes per-stream construction over the 1 ms threshold, ADR-0194 fires at 22.3 IMPL with the `*LState`-pool design (per 22.2 REVIEW.md §5 R6 disposition carry-forward). LIKELY STAYS WEAK-default — per-route resolution is a dispatch-table lookup (O(1) map lookup for `name` → `*Chunk`), not a new per-stream VM construction; the per-stream cost delta should be negligible.

**Rationale:** The STRONG-HOLD alternative (Q7 option 2 — exactly 1 ADR, no escape-valve) was rejected on prudence grounds — although per-route is mechanically simple, the conditional escape-valve costs nothing to reserve (ADR-0194 stays next-free if unconsumed) and matches the 22.1 + 22.2 WEAK-HOLD precedent (both held at 0 consumption but reserved the slot). The BREAK alternative (Q7 option 3 — 2-3 NEW ADRs) was rejected because it contradicts the Q1 "1 combined ADR" choice + over-provisions ADR slots for a CONSUME phase with no new framework primitive.

**Anticipated ADRs:** WEAK-HOLD prediction means the 22.3 IMPL anticipated ADR roster is: ADR-0193 (NEW combined multi-script + per-route) + ADR-0125 §(xiv) AMENDMENT (no number) + conditional ADR-0194 (escape-valve — fires only if R6 *LState-pool gate trips at 22.3 IMPL benchmark Task). Next-free ADR after 22.3 IMPL: ADR-0194 (if escape-valve unconsumed) or ADR-0195 (if consumed).

### 2.12 Scope shape: single-phase at BRAINSTORM *(Q8 → no split at this BRAINSTORM commit)*

**Decision:** 22.3 stays as one ROADMAP row at this BRAINSTORM commit. The PLAN session does the precise estimation against the ADR-0045 split-gate (~25 tasks / ~1500 LoC); if it exceeds, 22.3 splits at PLAN per the phase-09 → phase-11 + phase-13 split-at-PLAN precedent. BRAINSTORM artefacts (this file) are NOT invalidated by a future PLAN-time split.

**Rationale:** Per §1.4 — 22.3 is the smallest sub-phase (parent SPEC §3.0 estimate 14-18 tasks / ~1200-1500 LoC; no new bridge methods / primitives / stats). Pre-split-now (Q8 option 2) rejected on coupling grounds (no clean axis — per-route `name` delegates into `SourceCodes`; dangling-reference resolution straddles any split). The defer-to-PLAN disposition is the same posture as 22.2 Q14.

**Anticipated ADRs:** None at BRAINSTORM. If 22.3 splits at PLAN, an ADR documenting the PLAN-time split may anchor (confirm at PLAN time per the phase-13 precedent).

### 2.13 BRAINSTORM-time open questions for 22.3 SPEC-time resolution

#### D1 (per §2.1): per-name compile-cache keying

How do named `SourceCodes` scripts integrate with the 22.1 per-script compile cache? Two candidate dispositions: **(a) content-hash-keyed reuse** — each `SourceCodes[name]` DataSource resolves to source bytes that hash into the SAME 22.1 compile cache (keyed by content hash per ADR-0188/0189); the `name` is an operator-facing lookup label → `*Chunk`; identical scripts under different names share one compiled chunk; no cache-shape change. **(b) name-keyed separate cache** — a separate `name → *Chunk` map keyed by `SourceCodes` key; simpler lookup but loses dedup + adds a second cache structure. The 22.3 SPEC §11 empirical scrape settles (recommended disposition: (a) content-hash reuse — preserves the 22.1 cache discipline + dedups identical scripts; the `name → *Chunk` lookup is a thin label layer over the content-hash cache). 22.3 commits to per-name compilation at config-load (per §2.1); only the cache keying is deferred.

#### D2 (per §2.3): per-route `*filter` chunk-binding discipline

At per-route resolve time, which `*Chunk` does the per-stream `*filter` bind to, and when? Candidate: the per-route resolution (existing 3-tier mechanism) yields the resolved `*Chunk` (default / named / override) at HCM filter-chain build time per route; the per-stream `*filter` binds to the resolved chunk at stream construction. The 22.3 SPEC settles the exact binding point (HCM-build-time per-route resolution vs per-stream resolution) + the interaction with the per-stream `*LState` construction (the R6 escape-valve re-evaluation surface per §2.11).

#### D3 (per §2.2): `disabled: true` hook-skip semantics

`LuaPerRoute.disabled: true` → the lua filter is a no-op on the route. The 22.3 SPEC settles whether "no-op" means (a) the per-stream `*filter` is constructed but both hooks early-return (cheap; matches the existing per-route-disable pattern), or (b) the filter is omitted from the per-route chain entirely (requires HCM-chain-construction-level per-route filter omission — heavier; may not match the existing per-route mechanism). Recommended: (a) early-return both hooks (reuses the existing per-route-disable disposition from phase 13/14/15).

#### D4 (per §2.9): fixture-0028 boot-reject scenario additions

The 22.3 SPEC scrubs whether fixture-0028 adds boot-reject scenarios for the NEW config-load PARSE-REJECT arms (dangling-name arm 7 + key-empty arm 2 + per-route-source_code-DataSource-failure arm 8) — analogous to 22.1 fixture-0026 scenario (g) `BootRejectFixture`. These would be subject-side-only boot-reject assertions (the reference Envoy may accept configs envoy-go PARSE-REJECTs, or vice versa — settled at SPEC scrape).

---

## 3. Framework-survey result — 0 NEW primitives + 0 in-place AMENDs + 1 NEW combined ADR + 1 IN-PLACE AMENDMENT (ADR-0125 §(xiv))

Phase 22.3 introduces 0 NEW package-level framework primitives (a CONSUME + DISPATCH phase — see §1.3) + 0 in-place ADR amendments on framework-primitive ADRs (ADR-0188 / ADR-0189 / ADR-0190 / ADR-0191 / ADR-0192 / ADR-0177 all UNCHANGED) + 1 NEW combined ADR-0193 (package-shape consumption of `SourceCodes` + `LuaPerRoute` + per-route 3-tier dispatch per §2.5) + 1 IN-PLACE AMENDMENT on ADR-0125 §(xiv) (canonical roster 8 → 9, no number consumed, lands at 22.3 IMPL final Task per ADR-0044).

### 3.1 REUSES (no new primitive)

- **22.1 DataSource resolution** (`resolveDataSource`: 4-arm + WatchedDirectory PARSE-REJECT) — reused for both `SourceCodes[name]` values + the per-route `source_code` arm.
- **22.1 per-script compile cache** (ADR-0188/0189) — reused for named-script compilation (exact keying deferred to SPEC per §2.13 D1).
- **Existing per-route 3-tier resolution** (phase 13/14/15 `typed_per_filter_config` mechanism) — reused for the per-route dispatch.
- **Phase-17 jwt_authn 8th-canonical `name`-delegation discipline** — reused for the `name` → listener-`SourceCodes` lookup + the dangling-reference cross-resolution.
- **`internal/dynamicmetadata/` (22.2 ADR-0190)** — per-route override scripts inherit cross-filter dynamic-metadata visibility (no 22.3 action; see §1.6).

### 3.2 NEW combined ADR-0193 (package-shape consumption)

Per §2.5. ONE combined ADR documents the `internal/filter/http/lua/` 22.3 package-shape extensions: `SourceCodes` map consume + named-script compilation + the `name → *Chunk` lookup + `LuaPerRoute` 9th-canonical parse + per-route 3-tier dispatch + the HCM-build-time dangling-reference cross-resolution + the no-reserved-name disposition + the fixture-0028 + fuzzer dispositions. §Context block at 22.3 SPEC commit; §Decision + §Consequences bodies at 22.3 IMPL atomic-landing Task.

### 3.3 IN-PLACE AMENDMENT on ADR-0125 §(xiv) (canonical roster 8 → 9)

Per §2.4 + parent SPEC §4.5. The AMENDMENT body adds the 9th canonical's full specification (3-arm hybrid: disabled-bool + string-reference-delegation + DataSource-wholesale-override) + the SHARED stat-discipline + the structural-distinctness rationale + the updated 9-shape roster table + the lua-row first-use citation. Lands at 22.3 IMPL final Task per ADR-0044 in-place edit discipline (mirrors the phase-13/14/15/16/17 in-place-amend-at-IMPL precedents at ADR-0125 §(viii)-(xiii)). No new ADR number consumed.

---

## 4. Per-route shape — NEW 9th canonical (3-arm hybrid; SHARED stat-discipline)

The `LuaPerRoute` proto (v1.32.4 binding) defines a 3-arm `override` oneof (PGV `(validate.required) = true`):

- `disabled: bool` (field 1; PGV `const: true`) — disable Lua for this route (mirrors 5th canonical's disabled-bool arm).
- `name: string` (field 2; PGV `min_len: 1`) — string-reference into the listener-level `Lua.SourceCodes` map; the named script runs for this route instead of `DefaultSourceCode` (mirrors 8th canonical's string-reference-delegation pattern).
- `source_code: *core.DataSource` (field 3) — wholesale-override inline script using a `DataSource` type rather than a parent-config sub-message (NOVEL — no prior canonical uses `DataSource`-typed wholesale-override).

The 3-arm hybrid combines 3 structural patterns from prior canonicals in a single oneof — structurally distinct from all 8 prior canonicals (no prior canonical has all three). The 9th canonical's stat-discipline is SHARED (per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors`; `LuaPerRoute` has no separate `stat_prefix` field; matches phase-17 jwt_authn 8th-canonical SHARED per ADR-0154). ADR-0125's `§canonical-per-route-roster` grows from 8 → 9 at phase 22.3 IMPL.

The two v1.37.2 binding-gap fields on the per-route message stay forward-pointers: `LuaPerRoute.filter_context` (field 4; `google.protobuf.Struct`; the per-route `:metadata()` source — activates at the v1.37.x binding-bump phase per AMEND-12). 22.3's `LuaPerRoute` PARSE-LIFT does NOT activate `:metadata()` data (the source field does not exist in v1.32.4 bindings).

---

## 5. Stat surface — 0 net-new (SHARED-vacuous)

Per §2.8 + parent Q7 + parent SPEC §7.3. 0 net-new counters at 22.3; stat count STAYS 107. The 9th canonical's SHARED stat-discipline charges per-route errors to the listener-level `lua.<config_stat_prefix>.errors` (no per-route stat namespace — `LuaPerRoute` has no `stat_prefix` field). No BEHAVIOR_CONTRACT.md stat departure records added at 22.3 (the SHARED-stat rationale is recorded in the ADR-0125 §(xiv) AMENDMENT body).

| Phase | Stat count | Delta |
|---|---|---|
| Phase 22.1 phase-done | 102 | +3 (errors + executions + respond_calls) |
| Phase 22.2 phase-done | 107 | +5 (httpcall trio + body_buffered_bytes + coroutine_yields) |
| Phase 22.3 phase-done | 107 | +0 (SHARED-vacuous per 9th canonical) |

---

## 6. Differential fixture strategy — single fixture-0028, 5-tier cross-side byte-exact *(per §2.9)*

ONE fixture directory `test/fixtures/0028-http-lua-multi-script-and-per-route/` at 22.3 phase-done (29 → 30). Multi-scenario cross-side byte-exact (`CompareBytes`) for the 5 deterministic per-route tiers (a)-(e) per §2.9. Reuses 22.1's `BackendKind=HTTPLua` + `scripts/` subdirectory pattern (no new BackendKind). All scenarios deterministic on the wire (header-mutation + respond scripts; no `:timestamp()` / `:httpCall()` non-determinism). The 22.3 SPEC scrubs the exact scenario roster + may add boot-reject scenarios for the NEW config-load PARSE-REJECT arms (per §2.13 D4).

---

## 7. Anticipated ADRs — 1 combined NEW ADR-0193 + 1 IN-PLACE AMENDMENT (ADR-0125 §(xiv)) + conditional ADR-0194 *(per Q1 + Q7)*

### 7.1 ADR-0193 — NEW combined `internal/filter/http/lua/` 22.3 multi-script + per-route surface

Per §2.5 + §3.2. ONE combined ADR for the SourceCodes multi-script activation + the LuaPerRoute 9th-canonical 3-tier dispatch (one cohesive coupled surface). §Context block at 22.3 SPEC commit per ADR-0044 §Context-draft discipline; §Decision + §Consequences bodies at 22.3 IMPL atomic-landing Task.

**Lands-in:** 22.3 IMPL atomic-landing Task.
**Title (provisional):** "`internal/filter/http/lua/` 22.3 multi-script + per-route surface — `Lua.SourceCodes` named-script map consume + `LuaPerRoute` 9th-canonical 3-arm oneof + per-route 3-tier dispatch + HCM-build-time dangling-reference cross-resolution"

### 7.2 IN-PLACE AMENDMENT on ADR-0125 §(xiv) — canonical roster 8 → 9

Per §2.4 + §3.3 + parent SPEC §4.5. In-place AMENDMENT body on ADR-0125 §Decision at 22.3 IMPL final Task per ADR-0044 in-place edit discipline. No new ADR number consumed. Adds the 9th canonical specification + the SHARED stat-discipline + the structural-distinctness rationale + the 9-shape roster table + the lua-row first-use citation.

### 7.3 Conditional ADR-0194 — escape-valve slot per WEAK HOLD prediction *(per Q7)*

Per §2.11. Fires only if the R6 *LState-pool gate trips at the 22.3 IMPL benchmark Task (per-route `*Chunk` lookup + per-route resolution push per-stream construction over the 1 ms threshold). LIKELY STAYS UNCONSUMED (per-route resolution is an O(1) dispatch-table lookup, not a new per-stream VM construction). If R6 STANDS WEAK-default again at 22.3 IMPL + no other unanticipated landings, ADR-0194 stays next-free.

### 7.4 D-hypothesis prediction summary

| Hypothesis | NEW ADRs | In-place AMEND | Conditional | Net consumption |
|---|---|---|---|---|
| WEAK HOLD (chosen) | 1 (ADR-0193) | 1 (ADR-0125 §(xiv)) | 0-1 (ADR-0194) | 1-2 NEW |
| STRONG HOLD (rejected) | 1 (ADR-0193) | 1 (ADR-0125 §(xiv)) | 0 | 1 NEW |
| BREAK (rejected) | 2-3 | 1 (ADR-0125 §(xiv)) | 1 | 3-4 NEW |

Next-free ADR after 22.3 BRAINSTORM commit: UNCHANGED (ADR-0193 stays next-free per ADR-0044 §Context-draft discipline; NO ADR consumption at BRAINSTORM commit). ADR consumption happens at the 22.3 IMPL atomic-landing Task.

---

## 8. Deferred items (~5 items; forward to future / cross-phase)

1. **`:metadata()` per-route source activation** — future v1.32.4 → v1.37.x binding bump per AMEND-12 + parent SPEC §10 items 16-17. 22.3's `LuaPerRoute` PARSE-LIFT does NOT activate `:metadata()` data (the `filter_context` source field is v1.37.2-only).
2. **`Lua.clear_route_cache` (v1.37.2 field 5; v1.32.4 binding-gap)** — future binding-bump phase per AMEND-12 + parent SPEC §5.4.
3. **Multi-decoder "park-headers-iteration-pending-body" cooperative discipline** — the 22.2 "Continue-on-body-yield" trade-off (22.2 REVIEW.md §6.1); NOT a 22.3 scope item (per-route applies to a single lua filter instance at a time; no in-tree multi-lua-decoder topology at 22.3). Deferred to a separate framework phase.
4. **`internal/filterstate/` framework primitive extraction** — future; `:filterState()` STAYS IN-PACKAGE (landed at 22.2). Future second filter-state consumer triggers extraction.
5. **Cluster-specifier Lua + access-logger Lua + string-matcher Lua** — future cross-family phases; consumers #2/3/4 for `internal/lua/`; each future phase BRAINSTORM revisits the API shape per ADR-0188's API-REVISION ALLOWANCE clause (STAYS scoped to consumer-#2; UNCHANGED by 22.3 consumer-#1).

---

## 9. Cross-references against parent BRAINSTORM Q-decisions + 22.2 IMPL forward-pointers — closure pickup

### 9.1 Parent BRAINSTORM Q-decisions inherited

| Parent Q | Decision | 22.3 disposition |
|---|---|---|
| Q1 (envelope D) | Full upstream parity by phase-22 phase-done | 22.3 lands the FINAL delta (multi-script + per-route); envelope D complete at 22.3 phase-done (per §1.1) |
| Q2 (3-way pre-split) | 22.1 + 22.2 + 22.3 | 22.3 is the third + final sub-phase; closes parent row 22 (per §1.3) |
| Q5 (4-arm DataSource) | 4 arms + WatchedDirectory PARSE-REJECT | REUSED for both `SourceCodes[name]` values + per-route `source_code` arm (per §2.1 + §2.2) |
| Q7 (NEW 9th canonical) | LuaPerRoute 3-arm hybrid; SHARED stat | LANDED at 22.3 (per §2.2 + §2.4 + §4); ADR-0125 §(xiv) AMENDMENT body at 22.3 IMPL final Task |
| Q8 (stat surface) | errors + executions + respond_calls at 22.1 | 0 net-new at 22.3 (SHARED-vacuous per §2.8 + §5) |
| Q9 (full cross-side fixture) | Full cross-side byte-exact | fixture-0028 5-tier cross-side byte-exact (per §2.9 + §6) |
| Q10 (WEAK HOLD escape-valve) | 0-1 escape-valve | 22.3 uses WEAK HOLD again (per §2.11) |

### 9.2 22.2 IMPL forward-pointers picked up

Per 22.2 REVIEW.md §8 + BEHAVIOR_CONTRACT.md `#### Phase 22.2 forward-pointer notes`:

- **`Lua.SourceCodes` multi-script map activation** — LANDED at 22.3 (per §2.1); arm 4 PARSE-REJECT lifts.
- **`LuaPerRoute` 3-arm oneof override** — LANDED at 22.3 (per §2.2); arm 18 PARSE-REJECT lifts; NEW 9th canonical per ADR-0125 §(xiv).
- **Per-route 3-tier dispatch** — LANDED at 22.3 (per §2.3); settled at 22.3 SPEC for the exact binding point (per §2.13 D2).
- **NEW 9th canonical per-route shape ADR** — folded into the combined ADR-0193 (per §2.5); the standalone-ADR anticipation re-scoped to the combined ADR per Q1.
- **ADR-0125 §(xiv) AMENDMENT body** — lands at 22.3 IMPL final Task (per §3.3); the anticipation paragraph at parent SPEC commit STANDS UNCHANGED.
- **Conditional ADR-0193 forward** — RE-PURPOSED at 22.3: ADR-0193 is now the COMBINED multi-script + per-route ADR (per Q1), NOT the *LState-pool escape-valve. The escape-valve slot shifts to conditional ADR-0194 (per §2.11). The R6 *LState-pool gate re-evaluation happens at the 22.3 IMPL benchmark Task; if it trips, ADR-0194 fires.
- **Multi-decoder "park-headers-iteration-pending-body" discipline** — STAYS deferred (NOT a 22.3 scope item per 22.2 REVIEW.md §8 item 7 + §1.2 above).

### 9.3 Post-22.3 forward-pointers (parent row 22 closure)

22.3 is the last sub-phase. At 22.3 IMPL phase-done:

- Parent row 22 flips `in-progress → done` (per-cell IMPL-done annotation at the 22.3 IMPL Task).
- §9 HTTP-filters family closes from 4 remaining rows to 3 (`wasm`, `admission_control`, `global rate limit`).
- The v1.32.4 → v1.37.x binding-bump phase (future) activates `:metadata()` per-route data + `Lua.clear_route_cache` + `LuaPerRoute.filter_context` (the binding-gap forward-pointers).
- The `internal/lua/` framework primitive's consumer-#2/3/4 phases (cluster-specifier / access-logger / string-matcher Lua) revisit the API per ADR-0188's ALLOWANCE.

---

## 10. BRAINSTORM-time open questions for 22.3 SPEC-time resolution (D1-D4)

Consolidated from §2.13:

- **D1 — per-name compile-cache keying** (content-hash reuse vs name-keyed; recommended content-hash reuse).
- **D2 — per-route `*filter` chunk-binding discipline** (HCM-build-time per-route resolution vs per-stream resolution; interacts with R6 escape-valve).
- **D3 — `disabled: true` hook-skip semantics** (early-return both hooks vs per-route filter omission; recommended early-return).
- **D4 — fixture-0028 boot-reject scenario additions** (dangling-name + key-empty + per-route-source_code-DataSource-failure boot-reject scenarios).

Plus the 22.3 SPEC §11 empirical-pin scrape obligations (mirrors the 22.1 + 22.2 SPEC §11 pattern):

- Upstream Envoy v1.37.2 `source_codes` + `LuaPerRoute` consume semantics (named-script registry + per-route dispatch + dangling-reference behavior) against `lua_filter.cc` + `config.cc`.
- gopher-lua-vs-LuaJIT named-script + per-route dispatch observable divergences (likely none — dispatch is config-surface, not script-surface).
- PARSE-REJECT arm wording byte-exactness for the ~19 forward-pointer arms (parent SPEC §6.3 minus the dropped reserved-name arm 3).
- Fuzzer count verification post-22.3 (30 + 1 = 31 via `grep -h '^func Fuzz' | sort -u | wc -l`).

---

## 11. Phase-21 + parent-22 + 22.1 + 22.2 BRAINSTORM lessons applied

- **Phase-21 EXTRACT-NOW-only-when-trigger-fires lesson** — applied at §1.3 + §3.1: 0 new framework primitives at 22.3 (CONSUME + DISPATCH phase; reuses 22.1 DataSource resolution + compile cache + existing per-route 3-tier resolution + phase-17 8th-canonical name-delegation).
- **Phase-17 jwt_authn 8th-canonical string-reference-delegation discipline** — applied at §2.2 + §2.7: `name` → listener-`SourceCodes` lookup + HCM-build-time dangling-reference cross-resolution mirrors the jwt_authn `provider_name` discipline.
- **ADR-0154 SHARED-stat discipline** — applied at §2.8 + §5: per-route errors charge to the listener-level stat namespace (no per-route stat_prefix).
- **Parent 22 BRAINSTORM Q7 9th-canonical classification** — applied at §2.4 + §4: confirmed against the concrete proto-consume surface (no re-collapse to COMPOUND-mapping or EXTENSION-of-5th).
- **22.1 + 22.2 WEAK-HOLD precedent** — applied at §2.11: WEAK HOLD chosen (both predecessors held at 0 escape-valve consumption; 22.3's escape-valve slot at ADR-0194 likely stays unconsumed).
- **22.1 one-package-shape-ADR economy (ADR-0189)** — applied at §2.5: ONE combined ADR-0193 for the coupled multi-script + per-route surface (vs 2 separate ADRs).
- **Upstream-parity over envoy-go-strict-cost discipline** — applied at §2.6: no reserved-name discipline (matches upstream; avoids a zero-value BEHAVIOR_CONTRACT departure record).
- **Phase-09 → Phase-11 + Phase-13 split-at-PLAN precedent** — applied at §2.12: 22.3 stays single-phase at BRAINSTORM; PLAN decides split per ADR-0045 gate.
- **22.1 + 22.2 SPEC §11 empirical-pin pattern** — applied at §10: 22.3 SPEC §11 will scrape upstream `source_codes` + `LuaPerRoute` consume semantics + dangling-reference behavior + PARSE-REJECT arm wording + fuzzer count.

---

## 12. Section closeout

Phase 22.3 BRAINSTORM settles the genuinely-open 22.3 decisions the parent left to this session (the ones NOT pre-resolved by parent Q7 + the parent SPEC forward-pointers). Outcomes:

- FINAL envelope-D delta per parent BRAINSTORM Q1: `Lua.SourceCodes` + `LuaPerRoute` consumed; envelope D complete at 22.3 phase-done; parent row 22 closes.
- NEW 9th canonical per-route shape (3-arm hybrid; SHARED stat-discipline) per parent Q7 — LANDED at 22.3; ADR-0125 roster grows 8 → 9 via §(xiv) AMENDMENT body at 22.3 IMPL final Task.
- 0 NEW framework primitives + 0 in-place framework-ADR amendments (CONSUME + DISPATCH phase) + 1 combined NEW ADR-0193 + 1 IN-PLACE AMENDMENT on ADR-0125 §(xiv) + conditional ADR-0194 (escape-valve).
- No reserved-name discipline (drops parent SPEC §6.3 forward-pointer arm 3); HCM-build-time dangling-reference cross-resolution (fail-fast; mirrors jwt_authn 8th canonical).
- 0 net-new stats (SHARED-vacuous; stat count STAYS 107).
- 1 NEW differential fixture (`0028-http-lua-multi-script-and-per-route`) 5-tier cross-side byte-exact (29 → 30).
- 1 NEW fuzzer (`FuzzLuaPerRouteConfig`; 30 → 31); `SourceCodes` parse folds into existing `FuzzLuaConfigParse`.
- 4 D-questions (D1-D4) carry forward to 22.3 SPEC + the §11 empirical-pin scrape obligations.
- Single-phase 22.3 at BRAINSTORM; PLAN decides split per ADR-0045 gate.

**Next-skill:** `superpowers:brainstorming` (scoped to 22.3 SPEC per SKILL_ROUTING state-1 entry "Phase in ROADMAP, directory exists, SPEC.md does not exist → superpowers:brainstorming scoped to THIS phase → output: SPEC.md") — the next session authors `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/SPEC.md` (the BRAINSTORM → SPEC two-session split per the phase-22.1 + phase-22.2 sub-phase precedent).

**Squash-merge handoff:** this BRAINSTORM session lands all 22.3 BRAINSTORM artefacts (NEW BRAINSTORM.md + README.md status-line update + STATE.md lifecycle 0→1 update + ROADMAP row 22.3 planned→in-progress update) atomically via one squash-merge commit per project memory `feedback_git_worktrees.md` + ADR-0005 §Decision 4. Post-squash SHA-fill follow-up commit per the phase-09..22.2 convention. Pushed to origin per project memory `feedback_push_to_origin.md`.

**No ADR consumption at this BRAINSTORM commit.** ADR-0193 stays next-free; ADR-0188 + ADR-0189 + ADR-0190 + ADR-0191 + ADR-0192 + ADR-0177 + ADR-0125 §(xiv) UNCHANGED. ADR consumption happens at 22.3 IMPL atomic-landing Task per Q1 + Q7.
