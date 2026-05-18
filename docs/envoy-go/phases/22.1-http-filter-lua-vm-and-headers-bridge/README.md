# Phase 22.1 — `http-filter-lua-vm-and-headers-bridge` (placeholder)

**Status:** Sub-phase directory pre-created at the **phase-22 parent BRAINSTORM** per Q12 (see `../22-http-filter-lua/BRAINSTORM.md`). This sub-phase is **not yet opened** — opening happens at the dedicated 22.1 SPEC session (lifecycle-state 1 → 2 for row 22.1) after the parent SPEC at `../22-http-filter-lua/SPEC.md` lands.

**Parent row:** `22 | http-filter-lua` (status `in-progress` per ROADMAP).
**This sub-row:** `22.1 | http-filter-lua-vm-and-headers-bridge` (status `planned` per ROADMAP; depends-on `21`).
**Depends-on:** master tip post-phase-21-IMPL (parent BRAINSTORM branches from `e63feab`); the parent SPEC at `../22-http-filter-lua/SPEC.md` is the immediate predecessor.

## Anticipated scope (per parent BRAINSTORM §11.1)

Sub-phase 22.1 delivers the envelope-B-equivalent core + the NEW `internal/lua/` framework primitive at first consumer (per parent BRAINSTORM Q4 EXTRACT-NOW decision). Specifically:

- **NEW `internal/lua/` framework primitive** — gopher-lua VM lifecycle + script-compilation cache + sandbox config + bridge-registration interface. Anchored at **ADR-0188**.
- **NEW `internal/filter/http/lua/` package** — filter struct + factory + parse + 4-arm DataSource resolution + bridge methods + stats + per-route stub. Anchored at **ADR-0189**.
- **`Lua.DefaultSourceCode` consumed**; `Lua.SourceCodes` + `Lua.InlineCode` + `LuaPerRoute` PARSE-REJECTed (the InlineCode deprecated field is rejected per envoy-go-strict; `SourceCodes` + `LuaPerRoute` deferred to 22.3).
- **4-arm DataSource resolution** (Filename + InlineBytes + InlineString + EnvironmentVariable) + WatchedDirectory PARSE-REJECT.
- **Pragmatic-middle bridge surface** (per parent BRAINSTORM §2.6 / Q6): `envoy_on_request` + `envoy_on_response` hooks + `:headers()` + headers-object methods + `:logXxx()` + `:streamInfo()` subset (`:protocol` + `:routeName` + `:downstreamLocalAddress` + `:downstreamDirectRemoteAddress`) + `request_handle:respond()`. Deferred methods raise Lua runtime errors.
- **3-counter stat surface** — `errors` + `executions` + `respond_calls` under `lua.<config_stat_prefix>.<stat>`. Project stat count 99 → 102. 2 envoy-go-strict departure records at BEHAVIOR_CONTRACT.md.
- **28th project-wide fuzzer** — `FuzzLuaConfigParse`.
- **Differential fixture `0026-http-lua-headers-bridge`** — **full cross-side byte-exact for ALL 7 scenarios** per parent BRAINSTORM §6.2 (first cross-side byte-exact fixture without scope-deviation since phase 19.2).
- **BEHAVIOR_CONTRACT.md 7-edit bundle** at IMPL final Task per ADR-0052 atomic landing.

## Anticipated artefacts (filled in at this sub-phase's own session-set)

- `SPEC.md` — authored at the dedicated 22.1 SPEC session (next-skill `superpowers:brainstorming` per phase-18.1 / 18.2 precedent).
- `PLAN.md` — authored at the dedicated 22.1 PLAN session.
- `PROGRESS.md` — per-task entries authored during 22.1 IMPL session(s).
- `REVIEW.md` — authored at 22.1 IMPL phase-done per `superpowers:requesting-code-review`.

## D-hypothesis (per parent BRAINSTORM Q10)

WEAK HOLD: 2 anticipated ADRs (ADR-0188 + ADR-0189) land cleanly; 0-1 escape-valve consumption (one-slot consumption from phase-21's STRENGTHENED two-slot buffer). ZERO-slot buffer post-22.1-IMPL.

## Cross-references

- `../22-http-filter-lua/BRAINSTORM.md` — parent BRAINSTORM (load-bearing design doc for envelope D + 3-way split rationale)
- `../22.2-http-filter-lua-full-bridge/README.md` — successor sub-phase
- `../22.3-http-filter-lua-multi-script-and-per-route/README.md` — successor sub-phase

**This README is descriptive of anticipated scope, not prescriptive of design.** The 22.1 BRAINSTORM (if needed; the parent BRAINSTORM may have settled enough decisions to skip a sub-phase BRAINSTORM and proceed directly to SPEC) makes the binding design decisions.
