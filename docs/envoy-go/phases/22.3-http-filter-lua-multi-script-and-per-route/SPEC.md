# Phase 22.3 SPEC — `envoy.filters.http.lua` (multi-script `SourceCodes` + per-route `LuaPerRoute`)

> **Lifecycle state:** SPEC.md authored; ROADMAP row `22.3` stays `in-progress` (parent row `22` stays `in-progress` per ADR-0106 per-cell SPEC-done annotation; sub-rows `22.1` done, `22.2` done) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase-09..22.2 + phase-22.1 + phase-22.2 sub-phase-SPEC → PLAN precedent. This SPEC is the authoritative input to the 22.3 PLAN.

**Parent:** `docs/envoy-go/phases/22-http-filter-lua/SPEC.md` (the parent master SPEC — §3.1 split surface-mapping [22.3 column] + §4.5 ADR-0125 §(xiv) AMENDMENT-anticipation + §5.1 `Lua` roster + §5.2 `LuaPerRoute` roster + §5.3 DataSource arm roster + §6.3 22.3 forward-pointer PARSE-REJECT arms + §7.3 stat-count delta [22.3 = 0] + §11 7-pin empirical-pin block).

**Predecessors:** `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/BRAINSTORM.md` (12-section dialogue settling the 8 genuinely-open decisions + 4 D-questions carry-forward + the §11 empirical-pin scrape obligations + the anticipated ADR roster; authored at master tip `1ca1645` via squash `0929b3d`) + `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/{SPEC.md, REVIEW.md}` + `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/{SPEC.md, REVIEW.md}` (the predecessor sub-phase SPECs + REVIEWs — load-bearing precedents for SPEC structure + the §11 empirical-pin scrape pattern + the 22.1/22.2 IMPL inheritance state).

**Sub-phase scope (per parent SPEC §3.1 split surface-mapping + 22.3 BRAINSTORM §1.1):** 22.3 lifts the LAST two PARSE-REJECT surfaces in the lua filter — `Lua.SourceCodes` (the named-script map; arm 4 at 22.1/22.2) + `LuaPerRoute` (the 3-arm per-route override oneof; arm 18 at 22.1/22.2) — taking parent BRAINSTORM Q1 envelope D to its FULL conclusion. By 22.3 phase-done, every field in the `Lua` + `LuaPerRoute` v1.32.4 proto rosters is CONSUMED (except the two v1.37.2 binding-gap fields `Lua.clear_route_cache` + `LuaPerRoute.filter_context`, which stay forward-pointers per parent AMEND-12), and the lua filter joins the per-route-capable §9 filter cohort with a structurally-novel 9th canonical per-route pattern (ADR-0125 §(xiv) AMENDMENT body at 22.3 IMPL final Task).

22.3 surface delta (2 proto surfaces + dispatch + transverse decisions):

1. **`Lua.SourceCodes` multi-script map activation** (per §5.1 + 22.3 BRAINSTORM §2.1) — each `(name → DataSource)` entry resolves via the SAME 22.1 4-arm DataSource resolution (Filename + InlineBytes + InlineString + EnvironmentVariable; WatchedDirectory PARSE-REJECT) + compiles to a `*Chunk` at config-load. Named scripts are dispatch TARGETS only; a named script does NOT run on any route unless a `LuaPerRoute.name` references it; `DefaultSourceCode` remains the sole listener-level default.
2. **`LuaPerRoute` 3-arm oneof activation** (per §5.2 + §4) — `disabled:bool` (filter wholly inactive on this route) + `name:string` (string-reference-delegation into the listener-level `SourceCodes` map) + `source_code:*DataSource` (wholesale-override inline script via the SAME 4-arm DataSource resolution). The NEW 9th canonical per-route shape per ADR-0125 §(xiv).
3. **Per-route 3-tier dispatch** (per §4.2 + §11.2 D2 closure) — per-route override → listener-level `DefaultSourceCode` → no-op. Reuses the existing per-route 3-tier resolution from phase 13/14/15 (`internal/filter/http/perroute.go` `BuildPerRouteConfig` + `Resolve` + the ADR-0110 `RegisterPerRouteValidator` single-chokepoint) + extends with the phase-17 8th-canonical `name` string-reference-delegation discipline.
4. **NEW 9th canonical per-route shape** (per §4) — the 3-arm hybrid combines the 5th canonical's `disabled-bool` + the 8th canonical's `string-reference-delegation` + a novel `DataSource`-typed `wholesale-override`. ADR-0125 `§canonical-per-route-roster` grows 8 → 9 via in-place §(xiv) AMENDMENT body at 22.3 IMPL final Task.

Plus transverse decisions (all settled at 22.3 BRAINSTORM, sharpened by the §11 empirical scrape):

- **ADR structure** — ONE combined NEW ADR-0193 (multi-script + per-route; one cohesive coupled surface — per-route `name` delegates INTO `SourceCodes`) + the ADR-0125 §(xiv) in-place AMENDMENT (no number consumed). Matches 22.1's one-package-shape-ADR economy (ADR-0189). §Context draft anchors at THIS SPEC commit per ADR-0044.
- **No reserved-name discipline** — `default_source_code` (field 3) + `source_codes` (field 2) are independent proto fields; drops parent SPEC §6.3 forward-pointer arm 3. Matches upstream (per §11.1).
- **Dangling per-route `name` = upstream-parity silent no-op** (per §11.2 + AMEND-22.3-1; REVISES 22.3 BRAINSTORM decision #3 + parent SPEC §6.3 arm 7) — a per-route `name` referencing a key absent from `SourceCodes` resolves to no script and the filter is a no-op for that route, mirroring upstream `perLuaCodeSetup()` returning `nullptr` → `LUA_REFNIL`. **DROPS** parent SPEC §6.3 forward-pointer arm 7 (`per-route-name-not-in-source-codes`). 0 net-new BEHAVIOR_CONTRACT departure records; no HCM cross-resolution plumbing.
- **Stat surface** — 0 net-new counters; SHARED-vacuous per the 9th canonical (per §7 + §11.2). Stat count STAYS 107.
- **Differential fixture-0028** — single `0028-http-lua-multi-script-and-per-route` directory; 5-tier cross-side byte-exact (per §8) + boot-reject scenarios for the genuine config-load PARSE-REJECT arms (per §8.3 D4 closure). 29 → 30 fixture directories.
- **Fuzzer** — +1 NEW `FuzzLuaPerRouteConfig`; `SourceCodes` map parse folds into the existing `FuzzLuaConfigParse` corpus. 30 → 31 fuzzers (baseline 30 confirmed per §11.5).
- **D-hypothesis** — WEAK HOLD; 1 combined NEW ADR-0193 + ADR-0125 §(xiv) AMENDMENT land cleanly + 0-1 escape-valve at conditional ADR-0194 (fires only if R6 *LState-pool gate trips at 22.3 IMPL benchmark; LIKELY STAYS WEAK-default — per §11.2 the per-route resolution is an O(1) dispatch-table lookup, not a new per-stream VM construction).
- **Scope shape** — single-phase 22.3 at SPEC; the PLAN session fires the ADR-0045 split-gate only if estimates exceed (~25 tasks / ~1500 LoC).

**Phase 22.3 closes parent row 22 at 22.3 IMPL phase-done** — the parent row flips `in-progress → done` at the 22.3 IMPL Task. The §9 HTTP-filters family closes from 4 remaining rows (post-phase-21) to 3 remaining (`wasm`, `admission_control`, `global rate limit`).

**ADR continuity:** Phase 22.2 IMPL closed at ADR-0192 §Decision + §Consequences body. **At THIS 22.3 SPEC commit: 1 NEW ADR §Context draft anchors** (ADR-0193) per ADR-0044 §Context-draft discipline. §Decision + §Consequences body LANDS at 22.3 IMPL atomic-landing Task per ADR-0044 in-place edit discipline. **The ADR-0125 §(xiv) AMENDMENT body** lands at 22.3 IMPL final Task (no new ADR number; the anticipation paragraph at parent SPEC commit STANDS UNCHANGED). **Conditional ADR-0194** (escape-valve slot per the WEAK HOLD) anchors AT 22.3 IMPL only if the §13-R6 *LState-pool gate fires. **Next-free ADR after THIS 22.3 SPEC commit: `ADR-0194`** (1 number consumed: ADR-0193 §Context draft).

**Authored:** 2026-05-21.

**Base commit:** `1ca1645` (master tip at session entry; phase 22.3 BRAINSTORM SHA-fill follow-up; predecessor squash `0929b3d` = phase 22.3 BRAINSTORM atomic landing).

---

## 1. Purpose / Mission

Phase 22.3 lands the FINAL envelope-D delta for the lua filter — the `Lua.SourceCodes` named-script registry + the `LuaPerRoute` 3-arm per-route override oneof + the NEW 9th canonical per-route classification — on top of the 22.1 pragmatic-middle + the 22.2 full bridge surface. It is a CONSUME + DISPATCH phase: 0 new framework primitives, 0 new stats, 0 new bridge methods. It consumes 2 already-parsed-then-rejected proto surfaces, reuses the 22.1 DataSource resolution + content-hash compile cache, reuses the existing per-route 3-tier resolution from phase 13/14/15, and reuses the phase-17 8th-canonical `name`-delegation discipline.

After phase 22.3 the project has the FULL `envoy.filters.http.lua` config surface: every v1.32.4 proto field consumed (modulo the two v1.37.2 binding-gap fields); listener-level multi-script registry; per-route override (disabled / named-script / inline wholesale-override); OBSERVABLE-OUTCOMES byte-equivalent to reference Envoy v1.37.2 on the deterministic fixture-0028 scenarios — modulo the inherited 14 envoy-go-strict documented divergence-windows from 22.1 + 22.2 (22.3 adds 0 new departure records per the upstream-parity dispositions in §9).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The 5 §11 empirical pins (executed at this SPEC session via parallel-subagent fan-out against go-control-plane v1.32.4 bindings + upstream Envoy v1.37.2 source + gopher-lua master + the local envoy-go codebase per ADR-0004) generated the following **1 amendment-block entry** load-bearing for 22.3 + **4 D-closures** + **1 baseline confirmation**:

- **AMEND-22.3-1 (dangling per-route `name` disposition — REFUTES 22.3 BRAINSTORM decision #3 + parent SPEC §6.3 forward-pointer arm 7):** Per §11.2 scrape. The 22.3 BRAINSTORM (decision #3 / §2.7) + parent SPEC §6.3 arm 7 specified an **HCM-build-time PARSE-REJECT** for a per-route `name` referencing a key absent from the listener-level `SourceCodes` map, on the stated rationale that it "mirrors the phase-17 jwt_authn 8th-canonical `provider_name` dangling-reference discipline (which fails at config-build time)." The empirical scrape REFUTES this on two counts:
  1. **Upstream Envoy v1.37.2 lua silently no-ops a dangling per-route `name` at request time** (`FilterConfigPerRoute` ctor stores `name_` unvalidated; `FilterConfig::perLuaCodeSetup(name)` returns `nullptr` on a map miss → `function_ref = LUA_REFNIL` → neither `envoy_on_request` nor `envoy_on_response` runs). There is NO config-load failure and NO error string for the dangling-name path — the sole `throw EnvoyException` in the config path is the unrelated `inline_code`+`default_source_code` mutual-exclusion check.
  2. **The jwt_authn precedent it cited uses a SPLIT-SEMANTIC (ADR-0153 / phase-17 §11.P12), and its PER-ROUTE half does NOT fail fast.** `internal/filter/http/jwtauthn/jwtauthn.go` PARSE-REJECTs a dangling **listener-level** `requirement_name`/`provider_name` at boot, but a dangling **per-route** reference is **runtime-resolved** (emit 403, increment denied) — and ADR-0153 §1.1 amendment 6 records this explicitly as upstream-parity that "REFUTES [the phase-17] BRAINSTORM PARSE-REJECT hypothesis." So the per-route analog the 22.3 BRAINSTORM invoked is the runtime-resolve half, NOT the fail-fast half.

  **DISPOSITION (settled IN-SESSION at this SPEC commit):** envoy-go adopts **upstream-parity silent no-op** for a dangling per-route `name`. The per-route resolution looks up `name` in the listener-level `SourceCodes` registry at per-stream dispatch; on a miss, no script runs and the filter is a no-op for that route (lua has no auth-style 403 to emit — it is not an authorization filter). **DROPS parent SPEC §6.3 forward-pointer arm 7** (`per-route-name-not-in-source-codes`) — it never becomes a real config-load arm. **0 net-new BEHAVIOR_CONTRACT departure records** (upstream-parity; no envoy-go-strict divergence). **No new HCM cross-resolution plumbing** (the existing `RegisterPerRouteValidator` single-chokepoint validates per-route configs in isolation — it neither needs nor receives the listener-level `SourceCodes` key-set; the dangling-name lookup-miss is handled at per-stream dispatch). This disposition is consistent with the project's own jwt_authn precedent (ADR-0153) + 22.3 BRAINSTORM decision #2's "upstream-parity over envoy-go-strict-cost; avoid zero-value departure records" principle.

The 4 D-closures (D1 compile-cache keying + D2 chunk-binding + D3 disabled hook-skip + D4 fixture boot-reject) all CONFIRM their BRAINSTORM-recommended dispositions against the empirical evidence (per §11.1 + §11.3 + §11.4). No further AMEND surfaced. The 1 baseline confirmation is the fuzzer count (30 confirmed; 22.3 → 31 per §11.5).

This 22.3 SPEC's §3-§16 incorporate AMEND-22.3-1 + the 4 D-closures. No item carries forward to PLAN as a RATIFIED-PENDING re-scrape (unlike 22.2's §13-R7/R8 crypto+fileBytes follow-ups) — the 22.3 surfaces are config-shape + dispatch, fully resolved by the local-code + upstream scrapes.

### 1.2 ADR continuity + D-hypothesis at 22.3 SPEC commit

Phase 22.2 IMPL closed at ADR-0192 full body. **At THIS 22.3 SPEC commit: 1 NEW ADR §Context draft anchors** per ADR-0044 §Context-draft discipline:

- **ADR-0193 §Context** — NEW combined `internal/filter/http/lua/` 22.3 multi-script + per-route surface (`Lua.SourceCodes` named-script map consume + `LuaPerRoute` 9th-canonical 3-arm oneof + per-route 3-tier dispatch + the upstream-parity dangling-name silent no-op per AMEND-22.3-1 + the no-reserved-name disposition + fixture-0028 + `FuzzLuaPerRouteConfig`). §Context anchored at THIS SPEC commit; §Decision + §Consequences body lands at 22.3 IMPL atomic-landing Task per ADR-0044 in-place edit discipline.

**Next-free ADR after THIS 22.3 SPEC commit: `ADR-0194`** (1 number consumed: ADR-0193 §Context draft). The ADR-0044 escape-valve held in reserve at `ADR-0194` for the WEAK-HOLD conditional consumption surface per the BRAINSTORM Q7 disposition.

**ADR-0125 §(xiv) in-place AMENDMENT body** (canonical roster 8 → 9) lands at 22.3 IMPL final Task per ADR-0044 in-place edit discipline (mirrors the phase-13/14/15/16/17 in-place-amend-at-IMPL precedents at ADR-0125 §(viii)-(xiii)). **No new ADR number consumed.** The AMENDMENT-anticipation paragraph anchored at the parent SPEC commit STANDS UNCHANGED.

**D-hypothesis at 22.3 SPEC commit:** 22.3 BRAINSTORM Q7 WEAK-HOLD predicted 1 combined NEW ADR-0193 + the ADR-0125 §(xiv) AMENDMENT + 0-1 escape-valve consumption (conditional ADR-0194). This SPEC's empirical scrape STRENGTHENS the WEAK-default expectation: §11.2 confirms per-route resolution is an O(1) content-hash/registry lookup (`PerRouteConfig.Resolve` + `CompileCache` map lookup), NOT a new per-stream VM construction — so the per-stream cost delta over the 22.2 baseline (`ns/op = 98157` ~98µs/stream) should be negligible and the §13-R6 *LState-pool gate is unlikely to trip. **SPEC-time disposition: WEAK HOLD STANDS** (UNCHANGED from BRAINSTORM). 1 combined ADR-0193 lands cleanly + the ADR-0125 §(xiv) AMENDMENT + 0-1 escape-valve slot at ADR-0194. The post-22.3-IMPL re-evaluation is the parent-row-22 closure event (no successor sub-phase to carry the buffer).

---

## 2. Non-purposes

Phase 22.3 is the third + final sub-phase of the phase-22 BRAINSTORM-time 3-way pre-split. It does NOT extend the framework beyond the minimum needed to consume the 2 proto surfaces + land the 1 NEW ADR-0193 + the ADR-0125 §(xiv) AMENDMENT.

- **2.1 No new framework primitive.** 0 new `internal/*` packages. 22.3 reuses 22.1's `internal/lua/` (`CompileCache` + `*Chunk`) + the `internal/filter/http/lua/` package shape (ADR-0189 + ADR-0192) + the existing `internal/filter/http/perroute.go` 3-tier resolution. `internal/lua/`'s ADR-0188 EXPLICIT API-REVISION ALLOWANCE clause STAYS scoped to consumer-#2 (UNCHANGED at 22.3 — per-route + multi-script are dispatch-surface extensions, not VM-API revisions).
- **2.2 No new stat.** 0 net-new counters (SHARED-vacuous per the 9th canonical; per §7). Stat count STAYS 107.
- **2.3 No new bridge method.** 22.3 adds NO Lua-callable bridge surface. Per-route override scripts inherit the FULL 22.2 bridge surface unchanged (a named/override script can call `:body()`, `:httpCall()`, `:streamInfo():dynamicMetadata()`, etc. exactly as the listener-default script can).
- **2.4 No `§9` filter row.** `envoy.filters.http.lua` is already the 15th §9 family-row (wired at 22.1; 17 HTTP filters total). 22.3 extends the same filter's config surface; boot-registration alphabetical position between `localratelimit` and `oauth2` UNCHANGED per ADR-0072 + ADR-0100 §2.2.
- **2.5 No reserved-name discipline** (per 22.3 BRAINSTORM decision #2 + §11.1). `default_source_code` + `source_codes` are independent proto fields; a `SourceCodes` key cannot collide with the default. Drops parent SPEC §6.3 forward-pointer arm 3.
- **2.6 No HCM-build-time dangling-name cross-resolution** (per AMEND-22.3-1). A dangling per-route `name` is an upstream-parity silent no-op at per-stream dispatch; there is NO config-load PARSE-REJECT arm 7 and NO new plumbing to thread the listener-level `SourceCodes` key-set into the per-route validator.
- **2.7 `Lua.clear_route_cache` (v1.37.2 field 5) NEVER-DEFERRED.** Per parent AMEND-12 v1.32.4 binding-gap. Field ABSENT from the consumed v1.32.4 binding (confirmed §11.1); activates at the v1.37.x binding-bump phase.
- **2.8 `LuaPerRoute.filter_context` (v1.37.2 field 4) NEVER-DEFERRED.** Same binding-gap disposition (confirmed ABSENT in v1.32.4 per §11.1). 22.3's `LuaPerRoute` PARSE-LIFT does NOT activate `:metadata()` per-route data (the source field does not exist in v1.32.4 bindings); `:metadata()` continues to return the 22.2 callable empty userdata.
- **2.9 `Lua.InlineCode` deprecated field OUT OF SCOPE (PARSE-REJECT UNCHANGED from 22.1).** Never re-enabled per envoy-go-strict deprecated-field-rejection discipline + parent AMEND-6.
- **2.10 `WatchedDirectory` DataSource sibling field OUT OF SCOPE (PARSE-REJECT UNCHANGED).** Applies to BOTH `SourceCodes[name]` values + the per-route `source_code` arm (the same 4-arm DataSource resolution per §5.3). Deferred to a future Runtime/RTDS/hot-reload family phase.
- **2.11 No per-named-script stat cohort.** A `source_codes_<name>_executions` per-name counter was considered + rejected at 22.3 BRAINSTORM §2.8 (operators rarely need per-name counts; the SHARED `executions` counter aggregates all invocations regardless of which named/default/override script ran).
- **2.12 `*LState` pool NEVER-DEFERRED — escape-valve at §13-R6.** Per-stream `*LState` construction with the shared content-hash `*Chunk` cache remains the WEAK-default; the conditional ADR-0194 escape-valve fires only if the 22.3 IMPL benchmark crosses the 1ms per-stream-construction gate (LIKELY STAYS WEAK-default per §1.2 + §11.2).
- **2.13 No filter-chain ordering surgery + no multi-lua-decoder topology.** Per-route applies to a single lua filter instance at a time; 22.3 introduces no in-tree multi-lua-decoder topology. The 22.2 REVIEW.md §6.1 "Continue-on-body-yield" multi-decoder trade-off STAYS deferred (not a 22.3 scope item).
- **2.14 No re-litigation of settled BRAINSTORM/parent decisions** except where the §11 empirical scrape REFUTES an assumption (AMEND-22.3-1 only). The parent BRAINSTORM Q7 9th-canonical classification + parent SPEC §3.1/§4.5/§5.2/§5.3 + the 22.3 BRAINSTORM's other 7 LOCKED decisions STAND.
- **2.15 MVP confirmations (positive consumption assertions for 22.3).** All 22.1 + 22.2 surfaces stay consumed (no regressions). 22.3 adds: `SourceCodes` map consume + named-script compilation; `LuaPerRoute` 3-arm oneof consume + per-route 3-tier dispatch; the 9th canonical classification (ADR-0193 + ADR-0125 §(xiv) AMENDMENT); 1 NEW differential fixture `0028-http-lua-multi-script-and-per-route`; 1 NEW fuzzer `FuzzLuaPerRouteConfig`.

---

## 3. Framework primitive — NONE NEW (CONSUME + DISPATCH; reuses + NEW combined ADR-0193 package-shape extension)

Per parent SPEC §4 + 22.3 BRAINSTORM §3. 22.3 introduces 0 NEW package-level framework primitives + 0 in-place framework-ADR amendments (ADR-0188 / ADR-0189 / ADR-0190 / ADR-0191 / ADR-0192 / ADR-0177 all UNCHANGED) + 1 NEW combined ADR-0193 (package-shape consumption of `SourceCodes` + `LuaPerRoute` + per-route 3-tier dispatch) + 1 IN-PLACE AMENDMENT on ADR-0125 §(xiv) (canonical roster 8 → 9; no number consumed; lands at 22.3 IMPL final Task).

### 3.1 REUSES (no new primitive) — confirmed against local code at §11

| Reused surface | Anchor | 22.3 use | §11 evidence |
|---|---|---|---|
| 22.1 DataSource resolution (4-arm + WatchedDirectory PARSE-REJECT) | ADR-0189 | each `SourceCodes[name]` value + per-route `source_code` arm | §11.1 (proto type identity) |
| 22.1 content-hash compile cache (`internal/lua` `CompileCache`, SHA-256-keyed, per-listener) | ADR-0188/0189 | per-name compilation; identical-content named scripts dedup to one `*Chunk` | §11.3 D1 closure |
| Existing per-route 3-tier resolution (`internal/filter/http/perroute.go` `BuildPerRouteConfig` + `Resolve` + ADR-0110 `RegisterPerRouteValidator`) | phase 13/14/15 + ADR-0110 | per-route dispatch + per-route source_code config-load validation | §11.4 D2 closure |
| Phase-17 jwt_authn 8th-canonical `name`-delegation discipline | ADR-0153 | the `name` → listener-`SourceCodes` lookup (the runtime-resolve half, NOT the listener-level fail-fast half — per AMEND-22.3-1) | §11.2 |
| `internal/dynamicmetadata/` (22.2 ADR-0190) + the full 22.2 bridge surface | ADR-0190/0191/0192 | per-route/named scripts inherit cross-filter visibility + all bridge methods unchanged (no 22.3 action) | n/a |

### 3.2 NEW combined ADR-0193 (package-shape consumption)

Per §1.2 + 22.3 BRAINSTORM §2.5 + §3.2. ONE combined ADR documents the `internal/filter/http/lua/` 22.3 package-shape extensions: `SourceCodes` map consume + named-script compilation + the `name → *Chunk` lookup (content-hash cache; thin label layer) + `LuaPerRoute` 9th-canonical parse + per-route 3-tier dispatch + the upstream-parity dangling-name silent no-op (AMEND-22.3-1) + the no-reserved-name disposition + the fixture-0028 + fuzzer dispositions. §Context block at THIS SPEC commit; §Decision + §Consequences bodies at 22.3 IMPL atomic-landing Task.

### 3.3 `internal/filter/http/lua/` 22.3 file roster

Extends 22.1's + 22.2's roster. 22.3 adds 1 NEW production file + 1 NEW test file; extends a small number of existing files. (Exact file split + LoC estimate finalized at PLAN against the ADR-0045 split-gate; this is the SPEC-anticipated shape.)

```
internal/filter/http/lua/  (22.3 — extends the 22.1 + 22.2 roster)
  doc.go                  # EXTENDED: 22.3 multi-script + per-route decision summary + AMEND-22.3-1 cross-reference
  lua.go                  # EXTENDED: validatePerRouteLua REPLACED — the arm-18 "not yet supported" one-liner
                          # becomes the real 3-arm LuaPerRoute validator (PGV-mirror arms + per-route source_code
                          # DataSource gauntlet via the shared resolution path)
  compiled_config.go      # EXTENDED: consume Lua.SourceCodes (per-name DataSource resolution + content-hash
                          # compile into the existing per-listener CompileCache; name → *Chunk registry on the
                          # *compiledConfig) + the SourceCodes-key-empty PARSE-REJECT arm
  perroute.go             # NEW: LuaPerRoute parse + the 3-tier resolution (disabled → name→registry → source_code
                          # → DefaultSourceCode → no-op) + the per-stream *Chunk binding (O(1) lookup) + the
                          # upstream-parity dangling-name silent no-op
  datasource.go           # UNCHANGED from 22.1 (the 4-arm resolution is reused for both SourceCodes values
                          # + the per-route source_code arm)
  bridge.go / decode_headers.go / encode_headers.go / body.go / ...  # UNCHANGED from 22.2 (no new bridge surface)
  stats.go                # UNCHANGED (0 net-new counters; per-route errors charge to lua.<prefix>.errors)
  perroute_test.go        # NEW: LuaPerRoute parse + 3-tier dispatch + dangling-name silent-no-op + disabled
                          # hook-skip + named-script selection + source_code override unit tests
  compiled_config_test.go # EXTENDED: SourceCodes consume + per-name compile/cache-dedup + key-empty PARSE-REJECT
  fuzz_test.go            # EXTENDED: +1 FuzzLuaPerRouteConfig; FuzzLuaConfigParse corpus extended with source_codes seeds
```

Per-route binding note (§11.4 D2 closure): the per-route `source_code` inline-override DataSource is resolved + compiled at config-load (via the per-route validation chokepoint exercising the DataSource gauntlet, fail-fast on bad source); because the compile cache is content-hash-keyed and lives on the listener `*compiledConfig`, the per-stream binding is a cache HIT (no recompilation). The named-script `name` arm binds via an O(1) `name → *Chunk` registry lookup at per-stream dispatch. The exact storage wiring for the per-route override chunk (validator-side cache population vs filter-construction-time compile-with-cache-hit) is a small PLAN/IMPL detail carried at §12-D-P1 — functionally a no-op on cost given the content-hash cache.

22.3 phase-done production-LoC estimate: ~600-900 (the SMALLEST of the three sub-phases; no new files beyond perroute.go + tests). PLAN does precise estimation against the ADR-0045 split-gate.

---

## 4. Per-route shape — NEW 9th canonical (3-arm hybrid; SHARED stat-discipline)

### 4.1 The 3-arm `override` oneof

The `LuaPerRoute` proto (v1.32.4 binding; confirmed §11.1) defines a 3-arm `override` oneof with PGV `(validate.required) = true` (exactly one arm; validate.go:333-342):

- `disabled: bool` (field 1; PGV `const: true`, validate.go:253-262 "value must equal true") — disable Lua for this route; mirrors the 5th canonical's disabled-bool arm.
- `name: string` (field 2; PGV `min_len: 1`, validate.go:277-286 "value length must be at least 1 runes") — string-reference into the listener-level `Lua.SourceCodes` map; the named script runs for this route instead of `DefaultSourceCode`; mirrors the 8th canonical's string-reference-delegation pattern.
- `source_code: *core.DataSource` (field 3; embedded recursive validation) — wholesale-override inline script using a `DataSource` type rather than a parent-config sub-message (NOVEL — no prior canonical uses `DataSource`-typed wholesale-override).

The 3-arm combination is structurally distinct from all 8 prior canonicals — no prior canonical has all three patterns in one oneof. The two v1.37.2 binding-gap fields stay forward-pointers (per §2.7-§2.8): `LuaPerRoute.filter_context` (field 4; ABSENT in v1.32.4).

### 4.2 Per-route 3-tier dispatch (per §11.2 D2 + §11.4 closures)

Per-route resolution dispatches in 3 tiers, matching upstream `getPerLuaCodeSetup()` precedence (§11.2):

1. **Per-route override present** → resolve the `override` oneof:
   - `disabled` → no-op (skip both `envoy_on_request` + `envoy_on_response` per §11.2 D3 closure; matches upstream `nullptr` for both phases + the phase-13 buffer 5th-canonical disabled early-return pattern).
   - `name` → look up the named `*Chunk` in the listener-level `SourceCodes` registry; if present, run it; **if absent, upstream-parity silent no-op** (per AMEND-22.3-1 — the filter runs no script for this route).
   - `source_code` → run the per-route inline-override `*Chunk` (resolved + compiled at config-load; cache HIT at bind time).
2. **No per-route override** → run the listener-level `DefaultSourceCode` (the 22.1 default behavior).
3. **No per-route override AND no `DefaultSourceCode`** → no-op (existing 22.1 disposition).

The dispatch reuses the existing per-route 3-tier resolution mechanism (`internal/filter/http/perroute.go` `BuildPerRouteConfig` parse+validate at HCM-build time via the ADR-0110 `RegisterPerRouteValidator` single-chokepoint; `PerRouteConfig.Resolve(filterName, routeIdx)` lazy-cached lookup at per-stream dispatch). The `name` → listener-`SourceCodes` lookup happens against the listener-level registry (NOT a per-route-local map), matching upstream + the phase-17 8th-canonical precedent.

### 4.3 9th canonical classification + SHARED stat-discipline

`LuaPerRoute` is the NEW 9th canonical per-route pattern (settled at parent BRAINSTORM Q7; confirmed against the concrete proto-consume surface at this SPEC — no surprises that would re-collapse it to a COMPOUND-mapping or EXTENSION-of-5th). The 9th canonical's stat-discipline is SHARED (per ADR-0154; per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors`; `LuaPerRoute` has no separate `stat_prefix` field — confirmed §11.2 Q6: upstream `FilterConfigPerRoute` holds no `LuaFilterStats` + carries no per-route stat namespace). ADR-0125's `§canonical-per-route-roster` grows 8 → 9 via the in-place §(xiv) AMENDMENT body at 22.3 IMPL final Task.

---

## 5. Proto-field roster (per §11.1 — `Lua.SourceCodes` + `LuaPerRoute` now CONSUMED)

Per §11.1 empirical scrape against `go-control-plane/envoy@v1.32.4/extensions/filters/http/lua/v3/{lua.pb.go, lua.pb.validate.go}`. The 22.3 consumption surface (cross-reference parent SPEC §5.1/§5.2/§5.3):

### 5.1 `Lua` message field roster

| # | Field name | Go type | PGV | 22.3 disposition |
|---|---|---|---|---|
| 1 | `inline_code` (deprecated) | string | none | PARSE-REJECT (UNCHANGED from 22.1) |
| 2 | `source_codes` | `map[string]*v3.DataSource` (lua.pb.go:61) | per-entry embedded DataSource validation; no map-key/size rules | **22.3 CONSUMED** (per-name resolve + compile) |
| 3 | `default_source_code` | `*v3.DataSource` (lua.pb.go:64) | embedded recursive | CONSUMED from 22.1 (UNCHANGED) |
| 4 | `stat_prefix` | string | none | CONSUMED from 22.1 (UNCHANGED) |
| 5 | `clear_route_cache` | — | — | ABSENT in v1.32.4 binding (forward-pointer; per §2.7) |

### 5.2 `LuaPerRoute` message field roster (3-arm oneof)

| # | Field (oneof `override`) | Go wrapper type | Go type | PGV | 22.3 disposition |
|---|---|---|---|---|---|
| 1 | `disabled` | `LuaPerRoute_Disabled` (lua.pb.go:223-227) | bool | `const: true` (validate.go:253-262) | **22.3 CONSUMED** |
| 2 | `name` | `LuaPerRoute_Name` (lua.pb.go:229-233) | string | `min_len: 1` (validate.go:277-286) | **22.3 CONSUMED** |
| 3 | `source_code` | `LuaPerRoute_SourceCode` (lua.pb.go:235-238) | `*v3.DataSource` | embedded recursive (validate.go:301-328) | **22.3 CONSUMED** |
| — | `override` (oneof itself) | — | — | **`(validate.required) = true`** (validate.go:333-342 "value is required") | 22.3 PGV-required at consume time |
| 4 | `filter_context` | — | — | — | ABSENT in v1.32.4 binding (forward-pointer; per §2.8) |

DataSource Go type: `v3.DataSource` from `github.com/envoyproxy/go-control-plane/envoy/config/core/v3` (lua.pb.go:12). The per-route `source_code` arm is the SAME `*v3.DataSource` type as `DefaultSourceCode` + `SourceCodes` values — the resolution code path is shared (forces the FULL 4-arm DataSource roster + WatchedDirectory PARSE-REJECT per §5.3).

### 5.3 DataSource arm roster (reused for BOTH `SourceCodes[name]` values + per-route `source_code`)

UNCHANGED from parent SPEC §5.3 + 22.1's consumption: 4 specifier arms CONSUMED (`filename` + `inline_bytes` + `inline_string` + `environment_variable`) + `watched_directory` sibling-field PARSE-REJECT + empty-specifier-oneof PARSE-REJECT. The resolution + compile path is the 22.1 `resolveDataSource` + `internal/lua` `CompileScript` (content-hash cache).

---

## 6. PARSE-REJECT roster — 22.3 forward-pointer arms (parent §6.3 minus arm 3 + arm 7)

Per parent SPEC §6.3 + 22.3 BRAINSTORM decision #2 (drops arm 3) + AMEND-22.3-1 (drops arm 7). The 22.1 19-arm config-load roster is UNCHANGED; 22.3 LIFTS arm 4 (`source-codes-deferred-to-22-3`) + arm 18 (`per-route-deferred-to-22-3`) from PARSE-REJECT to CONSUMED, and adds the following forward-pointer config-load arms. Wording discipline per ADR-0080 + parent §6.1 (`"lua: <field_path>: <reason>"`; package-private `parseReject*` consts).

| arm-group | trigger | byte-exact wording (provisional; PLAN/IMPL settles) | disposition |
|---|---|---|---|
| `source-codes-each-value-data-source-resolution` (the same DataSource gauntlet per map entry) | each `SourceCodes[name]` value fails the 4-arm DataSource resolution / compile / required-hooks gauntlet (arms 6-17 of the 22.1 roster) | prefix `"lua: source_codes[%q]: ..."` (mirrors the 22.1 `default_source_code:` arms) | KEEP |
| `source-codes-key-empty` | a `SourceCodes` map key is `""` | `"lua: source_codes: key must be non-empty"` | KEEP (envoy-go-strict-as-defensive-mirror per parent §6.2 note; no per-row BEHAVIOR_CONTRACT record) |
| ~~`source-codes-key-duplicate-of-default`~~ | — | — | **DROPPED** (22.3 BRAINSTORM decision #2 — no reserved-name discipline; per §11.1) |
| `per-route-override-oneof-required` | `LuaPerRoute.Override == nil` (PGV-mirror; validate.go:333-342) | `"lua: per-route: override oneof is required"` | KEEP |
| `per-route-disabled-must-be-true` | `LuaPerRoute_Disabled` arm with `false` (PGV-mirror; validate.go:253-262) | `"lua: per-route: disabled must be true (PGV const:true violation; disabled:false is not meaningful)"` | KEEP |
| `per-route-name-min-1-rune` | `LuaPerRoute_Name` arm with `""` (PGV-mirror; validate.go:277-286) | `"lua: per-route: name length must be at least 1 rune"` | KEEP |
| ~~`per-route-name-not-in-source-codes`~~ | — | — | **DROPPED** (AMEND-22.3-1 — upstream-parity silent no-op; per §11.2). NOT a config-load arm; a dangling per-route `name` is a per-stream no-op, not a PARSE-REJECT. |
| `per-route-source-code-each-arm` (the same DataSource gauntlet) | `LuaPerRoute_SourceCode` arm's inline `*DataSource` fails the 4-arm resolution / compile / required-hooks gauntlet | prefix `"lua: per-route: source_code: ..."` | KEEP (validated at HCM-build via the per-route chokepoint, fail-fast; per §11.4) |

**22.3 forward-pointer arm count:** parent SPEC §6.3's ~20 anticipated arms − arm 3 (reserved-name; BRAINSTORM decision #2) − arm 7 (dangling-name; AMEND-22.3-1) = **~18 forward-pointer config-load arms**. Combined 22.1 + 22.3 PARSE-REJECT roster: **~37 arms**. Exact arm enumeration + the per-entry gauntlet count finalized at the 22.3 IMPL atomic-landing Task per the 22.1 Task-11 fuzzer-surfaced-arm precedent (PLAN settles whether the fuzzer surfaces additional config-load arms).

---

## 7. Stat surface — 0 net-new (SHARED-vacuous); stat count STAYS 107

Per parent Q7 + parent SPEC §7.3 + 22.3 BRAINSTORM §2.8 + §11.2 Q6. 0 net-new counters at 22.3. The 9th canonical's SHARED stat-discipline charges per-route errors to the listener-level `lua.<config_stat_prefix>.errors` (no per-route stat namespace — `LuaPerRoute` has no `stat_prefix` field; confirmed upstream `FilterConfigPerRoute` holds no `LuaFilterStats`). No BEHAVIOR_CONTRACT.md stat departure records added at 22.3.

| Phase | Stat count | Delta |
|---|---|---|
| Phase 22.1 phase-done | 102 | +3 (errors + executions + respond_calls) |
| Phase 22.2 phase-done | 107 | +5 (httpcall trio + body_buffered_bytes + coroutine_yields) |
| Phase 22.3 phase-done | 107 | +0 (SHARED-vacuous per 9th canonical) |

The SHARED-stat-discipline rationale is recorded in the ADR-0125 §(xiv) AMENDMENT body (per the 9th canonical's SHARED-stat specification), not as a per-counter departure record.

---

## 8. Differential fixture taxonomy — single fixture-0028, 5-tier cross-side + boot-reject scenarios

### 8.1 Fixture-0028 shape

ONE fixture directory `test/fixtures/0028-http-lua-multi-script-and-per-route/` at 22.3 phase-done. **29 → 30 fixture directories.** Reuses 22.1's `BackendKind=HTTPLua` + `scripts/` subdirectory pattern (no new BackendKind). The deterministic per-route tiers all produce byte-exact wire output (header-mutation + respond scripts) — NO 22.2-surface `:timestamp()`/`:httpCall()` non-determinism.

### 8.2 Deterministic cross-side scenarios (5 tiers; per 22.3 BRAINSTORM §6)

| # | Scenario | Surface | Mode |
|---|---|---|---|
| (a) | listener-default, no per-route | `DefaultSourceCode` runs (22.1 baseline) | cross-side `CompareBytes` |
| (b) | per-route `name` → named script | `LuaPerRoute.name` delegates into `SourceCodes` | cross-side `CompareBytes` |
| (c) | per-route `source_code` wholesale-override | `LuaPerRoute.source_code` inline-override script | cross-side `CompareBytes` |
| (d) | per-route `disabled` | `LuaPerRoute.disabled: true` → filter no-op on route (both hooks skipped) | cross-side `CompareBytes` |
| (e) | multi-script `SourceCodes` selection | two routes select two different named scripts | cross-side `CompareBytes` |

All 5 are fully cross-side byte-exact (no REFERENCE-LESS subject-only scenarios — the 22.3 surfaces are deterministic on the wire, unlike 22.2's httpCall/timestamp).

### 8.3 Boot-reject scenarios (per §11.4 D4 closure)

Per D4: fixture-0028 adds boot-reject scenarios (via the existing `BootRejectFixture` driver — both proxies exit non-zero + stderr substring-match; per §11.4) for the GENUINE config-load PARSE-REJECT arms:

| # | Scenario | Arm | mode |
|---|---|---|---|
| (f) | `source_codes` key-empty | `source-codes-key-empty` | `BootRejectFixture` (subject + reference both boot-reject; substring-match) |
| (g) | `source_codes[name]` DataSource failure (e.g. file-not-found) | `source-codes-each-value-data-source-resolution` | `BootRejectFixture` |
| (h) | per-route `source_code` DataSource failure (e.g. compile error) | `per-route-source-code-each-arm` | `BootRejectFixture` |

**DROPPED:** the BRAINSTORM-anticipated dangling-name boot-reject scenario (parent §6.3 arm 7) — per AMEND-22.3-1 a dangling per-route `name` is an upstream-parity silent no-op, NOT a boot-reject; it is covered instead by a deterministic subject+reference cross-side no-op assertion folded into scenario (b)-adjacent coverage (a route whose `name` is dangling produces the same pass-through wire output on both sides). PLAN scrubs whether the boot-reject scenarios (f)-(h) are cross-side (both proxies reject — likely, since upstream also fails on a bad DataSource) or subject-only (where the key-empty arm is envoy-go-strict-defensive and upstream may accept it — in which case scenario (f) is REFERENCE-LESS subject-only). The exact boot-reject scenario roster + cross-side-vs-subject-only discipline is finalized at PLAN per §12-D-P2.

---

## 9. Behavior-contract delta (cross-reference parent §9 + 22.1/22.2 SPEC + AMEND-22.3-1)

The phase-22.3 behavior-contract delta (vs 22.2 baseline; the verbatim Markdown patch lives at §14):

1. **`Lua.SourceCodes` multi-script registry active** — listener-level named scripts compiled at config-load (content-hash cache; identical-content scripts dedup); dispatch TARGETS for per-route `name`; a named script does NOT run by itself. Upstream-parity (per §11.2 Q1).
2. **`LuaPerRoute` 3-arm per-route override active** — `disabled` / `name` / `source_code`; the NEW 9th canonical per-route shape (ADR-0125 §(xiv) AMENDMENT body at 22.3 IMPL).
3. **Per-route `disabled: true` skips both hooks** — upstream-parity (per §11.2 Q4); the filter is a no-op for the route (constructed per-stream but early-returns; no chain omission).
4. **Dangling per-route `name` = silent no-op** (AMEND-22.3-1) — upstream-parity; a per-route `name` referencing an absent `SourceCodes` key runs no script for that route (no config-load reject, no runtime error). **0 envoy-go-strict departure record** (this is parity, not divergence). Documented at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` 22.3 sub-section as an explicit upstream-parity note (so operators know envoy-go does NOT fail-fast on dangling names — distinguishing it from the jwt_authn listener-level fail-fast).
5. **No reserved-name discipline** (BRAINSTORM decision #2) — `source_codes` keys are free-form; `default_source_code` is independent. Upstream-parity. Documented as an upstream-parity note.
6. **0 net-new stats** — per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors` (SHARED per the 9th canonical). No new counter rows.

**Net BEHAVIOR_CONTRACT departure-record count at 22.3 phase-done: UNCHANGED at 14** (3 from 22.1 + 11 from 22.2). 22.3 adds 0 envoy-go-strict departure records (all 22.3 dispositions are upstream-parity); it adds upstream-parity NOTES + the ADR-0125 §(xiv) cross-reference (per §14).

---

## 10. Deferred items + forward-pointers (cross-reference parent §10 + 22.3 BRAINSTORM §8)

The full envelope-D delivery completes at 22.3 phase-done. Items DEFERRED to future phases (cross-phase boundaries):

1. **`:metadata()` per-route source activation** — future v1.32.4 → v1.37.x binding bump per parent AMEND-12. 22.3's `LuaPerRoute` PARSE-LIFT does NOT activate `:metadata()` data (the `filter_context` source field is v1.37.2-only; ABSENT in v1.32.4 per §11.1).
2. **`Lua.clear_route_cache` (v1.37.2 field 5)** — future binding-bump phase per parent AMEND-12.
3. **Multi-decoder "park-headers-iteration-pending-body" cooperative discipline** — the 22.2 "Continue-on-body-yield" trade-off (22.2 REVIEW.md §6.1); NOT a 22.3 scope item.
4. **`internal/filterstate/` framework primitive extraction** — future; `:filterState()` STAYS IN-PACKAGE (landed at 22.2). Future second filter-state consumer triggers extraction.
5. **Cluster-specifier Lua + access-logger Lua + string-matcher Lua** — future cross-family phases; consumers #2/3/4 for `internal/lua/`; each future phase BRAINSTORM revisits the API shape per ADR-0188's API-REVISION ALLOWANCE (STAYS scoped to consumer-#2; UNCHANGED by 22.3 consumer-#1).
6. **Parent row 22 closure** — at 22.3 IMPL phase-done the parent row flips `in-progress → done`; §9 family closes from 4 remaining rows to 3 (`wasm`, `admission_control`, `global rate limit`).

---

## 11. SPEC-time empirical-pin block — 5 pins resolved IN-SESSION (per ADR-0004)

This block contains the verbatim parallel-subagent-fan-out scrape evidence executed during this 22.3 SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase-22.1 + 22.2 SPEC §11 structure. **Probe date: 2026-05-21.**

**Reference source corpus** (multi-axis verification):

1. **go-control-plane v1.32.4 binding** (local) — `…/envoy@v1.32.4/extensions/filters/http/lua/v3/{lua.pb.go, lua.pb.validate.go}`.
2. **Upstream Envoy v1.37.2 source** (WebFetch) — `source/extensions/filters/http/lua/{lua_filter.cc, lua_filter.h, config.cc}` + `api/.../lua/v3/lua.proto`.
3. **gopher-lua master** (WebFetch) — `state.go`, `compile.go`, `function.go`.
4. **Local envoy-go codebase** — `internal/lua/compile.go`, `internal/filter/http/lua/{compiled_config.go, lua.go}`, `internal/filter/http/perroute.go`, `internal/filter/http/jwtauthn/jwtauthn.go`, `test/differential/harness.go` + `test/fixtures/0026-http-lua-headers-bridge/inputs/driver.go`, `docs/envoy-go/{DECISIONS.md, BEHAVIOR_CONTRACT.md}`.

### Summary disposition table (5 pins → 1 AMEND + 4 D-closures + 1 baseline)

| Pin | Topic | Disposition | Cross-ref |
|---|---|---|---|
| §11.1 | v1.32.4 proto roster (`SourceCodes` + `LuaPerRoute` + PGV) | CONFIRMS parent §5.1/§5.2/§5.3; no-reserved-name confirmed | §5 |
| §11.2 | upstream `source_codes` registry + per-route dispatch + dangling-name + disabled + stats | SURFACES AMEND-22.3-1 (dangling-name); CLOSES D3 (disabled) + Q6 (SHARED stat) | AMEND-22.3-1 + D3 |
| §11.3 | gopher-lua compile/chunk-sharing (D1) | CLOSES D1 → content-hash reuse (chunk safely shared across named scripts) | D1 |
| §11.4 | local per-route mechanism (D2) + jwt_authn precedent + BootRejectFixture (D4) | CLOSES D2 + D4; corroborates AMEND-22.3-1 (jwt_authn split-semantic) | D2 + D4 |
| §11.5 | fuzzer count | 30 confirmed → 31 at 22.3 | §13-R-fuzz |

### §11.1 v1.32.4 proto roster — CONFIRMS parent §5

Per local subagent scrape of `lua.pb.go` + `lua.pb.validate.go`.

- `Lua`: 4 fields. Field 2 `SourceCodes` is `map[string]*v3.DataSource` (lua.pb.go:61, getter :125). Field 3 `DefaultSourceCode` is `*v3.DataSource` (:64). **Field 5 `clear_route_cache` ABSENT** (confirms binding-gap).
- `LuaPerRoute`: 3-arm `override` oneof — `LuaPerRoute_Disabled` (bool, field 1, :223-227), `LuaPerRoute_Name` (string, field 2, :229-233), `LuaPerRoute_SourceCode` (`*v3.DataSource`, field 3, :235-238). **Field 4 `filter_context` ABSENT** (confirms binding-gap).
- PGV: oneof `(validate.required)=true` enforced at validate.go:333-342 ("Override … value is required"); `disabled` const:true at :253-262 ("value must equal true"); `name` min_len:1 at :277-286 ("value length must be at least 1 runes"); `source_code` embedded recursive at :301-328.
- DataSource Go type: `v3.DataSource` from `github.com/envoyproxy/go-control-plane/envoy/config/core/v3` (lua.pb.go:12). The per-route `source_code` is the SAME type as `DefaultSourceCode` + `SourceCodes` values — shared resolution path (forces the full 4-arm roster).
- **No reserved-name discipline** (confirms BRAINSTORM decision #2): `source_codes` (field 2) + `default_source_code` (field 3) are independent fields; no PGV map-key rule; no name addresses the default.

### §11.2 upstream Envoy v1.37.2 `source_codes` + per-route dispatch — SURFACES AMEND-22.3-1; CLOSES D3 + Q6

Per general-purpose subagent scrape of `lua_filter.{cc,h}` + `config.cc` + `lua.proto`.

**§11.2.1 named-script registry (Q1) — compiled at config-load.** `FilterConfig` ctor (`lua_filter.cc` ~468-495) iterates `proto_config.source_codes()`, reads each `DataSource` via `Config::DataSource::read(...)`, compiles each into a `PerLuaCodeSetup` (`std::make_unique<PerLuaCodeSetup>(code, tls)`), and stores in `absl::flat_hash_map<std::string, PerLuaCodeSetupPtr> per_lua_code_setups_map_`. A named script runs ONLY when a per-route `name` references it (never by itself); `default_lua_code_setup_` (from `default_source_code`) is the listener default. **Confirms named-scripts-compiled-at-config-load.**

**§11.2.2 per-route dispatch (Q2) + dangling-name (Q3) — AMEND-22.3-1.** `FilterConfigPerRoute` ctor (`lua_filter.cc` ~497-509): stores `disabled_`, `name_`, and (for `source_code`) eagerly compiles `per_lua_code_setup_ptr_`. For the `name` arm it stores `name_` and early-returns — **NO registry lookup, NO validation at per-route-config construction.** At request time, `FilterConfig::perLuaCodeSetup(name)` (`lua_filter.h` ~284-293) returns the named setup or **`nullptr` on a map miss (NO throw, NO error string)**. The caller sets `function_ref = LUA_REFNIL` → no script runs. **A dangling per-route `name` is a SILENT REQUEST-TIME NO-OP, not a config-load failure.** This REFUTES the 22.3 BRAINSTORM decision #3 + parent SPEC §6.3 arm 7 "HCM-build-time PARSE-REJECT" framing → **AMEND-22.3-1** (disposition: upstream-parity silent no-op; arm 7 DROPPED).

**§11.2.3 disabled hook-skip (Q4) — CLOSES D3 → early-return both hooks.** `getPerLuaCodeSetup()` (`lua_filter.h` ~646-664) returns `nullptr` when `per_route_config_->disabled()`. Both `decodeHeaders` + `encodeHeaders` call `getPerLuaCodeSetup()` independently → `nullptr` → `LUA_REFNIL` for both `requestFunctionRef` + `responseFunctionRef` → **both hooks skipped.** The filter is constructed per-stream but early-returns (NOT omitted from the chain). **Confirms D3 RECOMMENDED option (a).**

**§11.2.4 per-route resolution precedence (Q5).** `getPerLuaCodeSetup()` precedence: (1) `disabled()` → nullptr; (2) `name()` non-empty → `config_->perLuaCodeSetup(name())`; (3) per-route inline `perLuaCodeSetup()` non-null → use it; (4) fall through to listener default `config_->perLuaCodeSetup()`. `encodeHeaders` re-resolves independently. envoy-go's `internal/filter/http/perroute.go` `Resolve` (lazy-cached) is the analog (§11.4).

**§11.2.5 stats (Q6) — SHARED, no per-route namespace.** `FilterConfigPerRoute` takes no stats/scope params + holds no `LuaFilterStats`; `LuaPerRoute` proto has no `stat_prefix` field. Per-route errors charge to the single listener-level `stats_` scope. **Confirms SHARED-vacuous (0 net-new stats).**

### §11.3 gopher-lua compile + chunk-sharing — CLOSES D1 → content-hash reuse

Per general-purpose subagent scrape of gopher-lua `state.go` + `compile.go` + `function.go`.

- Pipeline: `parse.Parse(reader, name) → Compile(stmts, name) (*FunctionProto, error)`; per-state instantiation via `LState.NewFunctionFromProto(proto) *LFunction`.
- `*FunctionProto` is finalized after `Compile` (mutated only DURING compile); `newLFunctionL` only READS the proto + allocates a fresh per-instance `LFunction` (own `Env` + `Upvalues`). **A single `*FunctionProto` is safe to instantiate into multiple `*LState`s sequentially or concurrently.**
- **YES — two `source_codes` entries with byte-identical content can safely share one cached `*FunctionProto`.** No new gopher-lua-vs-LuaJIT divergence from multi-script/per-route dispatch (selection is Go-side config/dispatch; each script runs in its own per-stream `*LState` exactly as a single default script does).

This matches envoy-go's existing 22.1 cache: `internal/lua/compile.go:80-83` `CompileCache{ store map[[32]byte]*Chunk }` keyed by `sha256.Sum256(src)` (`CompileScript` :99-143), held per-listener on the `*compiledConfig` (`compiled_config.go` ~155-160). **D1 CLOSURE: content-hash reuse (option a).** Named `SourceCodes` scripts hash into the SAME cache; the `name → *Chunk` lookup is a thin label layer over the content-hash cache; identical scripts under different names share one compiled chunk; no cache-shape change.

### §11.4 local per-route mechanism + jwt_authn precedent + BootRejectFixture — CLOSES D2 + D4

Per Explore subagent scrape of the local codebase.

**§11.4.1 per-route 3-tier resolution (D2 closure).** `internal/filter/http/perroute.go`: `BuildPerRouteConfig(rcCfg, scopes, chainNames, reg)` parses each scope's `typed_per_filter_config` at HCM-build time + runs the ADR-0110 `RegisterPerRouteValidator` validator per filter per tier (`reg.PerRouteValidator(name)`; error wording `"hcm: route_config: typed_per_filter_config[%q]: %w"`). `PerRouteConfig.Resolve(filterName, routeIdx)` (:140-165) does the lazy-cached 3-tier most-specific lookup (Route > VirtualHost > RouteConfiguration) at per-stream dispatch. The lua filter's current `validatePerRouteLua` (`lua.go` ~225-249) is the arm-18 "not yet supported" one-liner registered via `RegisterPerRouteValidator` BEFORE `Freeze()`. **D2 CLOSURE:** 22.3 REPLACES `validatePerRouteLua` with the real 3-arm validator (PGV-mirror + per-route `source_code` DataSource gauntlet, fail-fast at HCM-build); the per-stream filter binds the resolved `*Chunk` (default / named-from-registry / per-route-source_code) via the existing `Resolve` + content-hash cache O(1) lookup at `DecodeHeaders`/`EncodeHeaders` — matching upstream's request-time `getPerLuaCodeSetup()`. No new per-stream VM construction (corroborates the §1.2 WEAK-HOLD R6 prediction). The phase-13 buffer 5th-canonical `disabled` early-return (Continue without buffering in the hot path) is the disabled-disposition precedent.

**§11.4.2 jwt_authn dangling-reference precedent (corroborates AMEND-22.3-1).** `internal/filter/http/jwtauthn/jwtauthn.go`: listener-level dangling `provider_name` (:513-516) + `requirement_name` (:586-592) PARSE-REJECT at boot (`"provider_name %q not in providers map"` / `"requirement_name %q not in requirement_map"`); BUT per-route dangling reference is **runtime-resolved (emit 403)** per ADR-0153 — the BEHAVIOR_CONTRACT.md `requirement_map` row (:1657) records "referenced by listener-level `RequirementRule.requirement_name` (parse-rejected on dangling name) AND by per-route `PerRouteConfig.requirement_name` (runtime-resolved on dangling name)", and ADR-0153 §1.1 amendment 6 records this split-semantic as upstream-parity that "REFUTES [phase-17] BRAINSTORM PARSE-REJECT hypothesis." So the jwt_authn PER-ROUTE analog is runtime-resolve, NOT config-load fail-fast — the 22.3 BRAINSTORM conflated the listener-level half. **Corroborates AMEND-22.3-1** (lua per-route dangling-name = upstream-parity runtime no-op).

**§11.4.3 BootRejectFixture (D4 closure).** `test/differential/harness.go:312-352` defines the `BootRejectFixture` interface (`BootRejectScript() string` + `ExpectedBootErrorSubstring() string`); the runner spawns BOTH proxies, asserts BOTH exit non-zero, asserts BOTH stderr buffers CONTAIN the substring (case-sensitive). `test/fixtures/0026-http-lua-headers-bridge/inputs/driver.go:444-465` is the 22.1 exemplar (substring `"script load error"`). **D4 CLOSURE:** fixture-0028 CAN add boot-reject scenarios for the genuine config-load PARSE-REJECT arms (key-empty + SourceCodes-DataSource-failure + per-route-source_code-DataSource-failure per §8.3); the dangling-name boot-reject scenario is DROPPED (per AMEND-22.3-1).

### §11.5 fuzzer count

Per Explore subagent: `grep -rh '^func Fuzz' --include='*.go' | sort -u | wc -l` = **30** (confirmed; existing lua fuzzers: `FuzzLuaConfigParse` + `FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig`). 22.3 adds `FuzzLuaPerRouteConfig` → **31**; `SourceCodes` map parse folds into the `FuzzLuaConfigParse` corpus.

---

## 12. SPEC-time D-questions for PLAN-time resolution

The 4 BRAINSTORM D-questions (D1-D4) are CLOSED IN-SESSION at this SPEC commit (per §11.3 D1 + §11.4 D2/D4 + §11.2 D3). The following residual PLAN-time questions remain (small; implementation-shape, not design):

### D-P1 (per §3.3 + §11.4 D2): per-route override `*Chunk` storage wiring

The per-route `source_code` inline-override is resolved + compiled at config-load (fail-fast via the per-route validator exercising the DataSource gauntlet). WHERE the compiled `*Chunk` is stored for the per-stream binding — (a) the validator populates the listener `compileCache` as a side effect (requires the validator to hold a `*CompileCache` reference) vs (b) the per-stream filter compiles-with-cache-hit at bind time (content-hash cache HIT; zero recompile cost) — is a PLAN/IMPL detail. **Anticipated answer:** option (b) compile-with-cache-hit at bind time (simplest; the content-hash cache makes recompilation a cache HIT; keeps the `func(proto.Message) error` validator signature unchanged). RATIFIED-PENDING-PLAN.

### D-P2 (per §8.3 + §11.4 D4): fixture-0028 boot-reject scenario roster + cross-side-vs-subject-only discipline

Whether boot-reject scenarios (f)-(h) are cross-side (both proxies reject — likely for the DataSource-failure arms (g)+(h), since upstream also fails on a bad DataSource) or REFERENCE-LESS subject-only (the key-empty arm (f) is envoy-go-strict-defensive; upstream may accept an empty `source_codes` key). PLAN scrubs the exact roster + per-scenario discipline against a targeted upstream behavior check. **Anticipated answer:** (g)+(h) cross-side; (f) subject-only (envoy-go-strict-defensive). RATIFIED-PENDING-PLAN.

### D-P3 (per §6 + 22.1 Task-11 precedent): config-load arm enumeration + fuzzer-surfaced arms

The exact arm count of the `SourceCodes` + per-route `source_code` DataSource gauntlets (per-entry application) + whether `FuzzLuaPerRouteConfig` / the extended `FuzzLuaConfigParse` corpus surfaces additional config-load arms (per the 22.1 Task-11 +2-arm precedent). PLAN settles the precise roster; IMPL pins the byte-exact wording per ADR-0080. RATIFIED-PENDING-PLAN/IMPL.

---

## 13. RATIFIED-PENDING items (cross-reference parent §13 + 22.1/22.2 SPEC §13)

Items the SPEC anchors as RATIFIED at SPEC commit but pending PLAN/IMPL-time confirmation against the actual envoy-go codebase state.

### Sandbox + perf

- **R6: `*LState`-pool benchmark gate (escape-valve to ADR-0194)** — 22.2 IMPL R6 STANDS WEAK-default at ~98µs/stream (`ns/op = 98157`). 22.3 IMPL benchmark task measures per-stream construction at the multi-script + per-route surface. Per §1.2 + §11.2 + §11.4 the per-route resolution is an O(1) `Resolve` + content-hash cache lookup (NOT a new per-stream VM construction), so the cost delta should be negligible. If `> 1ms`: ADR-0194 escape-valve consumes for `*LState` pool design. If `< 1ms`: WEAK-default carries forward; ADR-0194 stays next-free. **Anticipated: WEAK-default STANDS; ADR-0194 unconsumed.**

### Per-route binding + fixture

- **R-P1 (= §12-D-P1): per-route override `*Chunk` storage wiring** — PLAN settles option (a) validator-side cache population vs (b) compile-with-cache-hit at bind. Anticipated (b).
- **R-P2 (= §12-D-P2): fixture-0028 boot-reject roster + cross-side discipline** — PLAN settles (g)+(h) cross-side / (f) subject-only.

### Fuzzer count + wording

- **R-fuzz: 31st fuzzer count verification** — 22.3 IMPL Task for `FuzzLuaPerRouteConfig`; project-wide grep at IMPL pins exact number (30 → 31). §11.5 PRE-CONFIRMS baseline at 30.
- **W3: byte-stable config-load wording for the 22.3 forward-pointer arms** — `source_codes[%q]:`-prefixed + `per-route:`-prefixed arm wording pinned at the 22.3 IMPL atomic-landing Task per ADR-0080 + parent §6.1.

### Closed in-session

- **D1 + D2 + D3 + D4 — CLOSED IN-SESSION at this SPEC commit** (per §11). AMEND-22.3-1 supersedes the BRAINSTORM decision #3 dangling-name disposition. No further design-time action.

---

## 14. BEHAVIOR_CONTRACT.md edit bundle anticipation (cross-reference parent §14 + 22.2 SPEC §14)

Per ADR-0052 atomic landing + the 22.1/22.2 SPEC §14 precedent. The 22.3 IMPL final-Task edit bundle (NO new envoy-go-strict departure records — all 22.3 dispositions are upstream-parity; the bundle is notes + cross-references):

1. **EXTEND `### envoy.filters.http.lua` subsection** with the 22.3 multi-script + per-route surface (SourceCodes registry + LuaPerRoute 3-arm + 3-tier dispatch + 9th canonical). Cross-reference the 22.1 + 22.2 sub-sections. Convert the `#### Phase 22.2 forward-pointer notes` 22.3-anticipated bullets to landed entries. ~80-120 lines.
2. **Upstream-parity NOTE: dangling per-route `name` silent no-op** (per AMEND-22.3-1) — explicit note that envoy-go does NOT fail-fast on a dangling per-route `name` (matches upstream `LUA_REFNIL`; distinguished from the jwt_authn listener-level fail-fast). NOT a departure record.
3. **Upstream-parity NOTE: no reserved-name discipline** (BRAINSTORM decision #2) — `source_codes` keys free-form; `default_source_code` independent. NOT a departure record.
4. **ADR-0125 §(xiv) cross-reference** — the 9th canonical per-route shape + the SHARED stat-discipline; cross-reference the ADR-0125 AMENDMENT body landing at the same 22.3 IMPL final Task.
5. **Stat surface UNCHANGED at 107** — note 0 net-new (SHARED-vacuous); no stat-table edit.
6. **`#### Phase 22.3 forward-pointer notes` (parent-row-22 closure)** — `:metadata()` per-route source + `clear_route_cache` + `filter_context` binding-gap forward-pointers; the consumer-#2/3/4 `internal/lua/` future phases. ~30-50 lines.

**Departure-record count: UNCHANGED at 14** (3 from 22.1 + 11 from 22.2; 22.3 adds 0).

---

## 15. 22.3 IMPL acceptance checklist (~16 items extending parent §16 + 22.1/22.2 SPEC §15)

The 22.3 IMPL Task(s) that land the multi-script + per-route surface + tests + fixture + ADR landings + STATE.md re-advance + parent-row-22 closure MUST satisfy ALL of:

1. CONSUME `Lua.SourceCodes` per §5.1 — per-name DataSource resolution (reusing `resolveDataSource`) + content-hash compile into the existing per-listener `CompileCache` + the `name → *Chunk` registry on `*compiledConfig`.
2. CONSUME `LuaPerRoute` 3-arm oneof per §5.2 + §4 — REPLACE `validatePerRouteLua` with the real 3-arm validator (PGV-mirror arms + per-route `source_code` DataSource gauntlet, fail-fast at HCM-build via the ADR-0110 chokepoint).
3. Per-route 3-tier dispatch per §4.2 — `disabled` → both-hooks-skip; `name` → registry lookup (silent no-op on miss per AMEND-22.3-1); `source_code` → inline override; fall through to `DefaultSourceCode` → no-op. Reuses `internal/filter/http/perroute.go` `Resolve`.
4. NEW `internal/filter/http/lua/perroute.go` + `perroute_test.go` per §3.3.
5. Config-load PARSE-REJECT arms per §6 — `source-codes-key-empty` + the `source_codes[%q]:` + `per-route: source_code:` DataSource gauntlets + the 3 PGV-mirror per-route arms. Arms 3 + 7 NOT present (dropped per BRAINSTORM decision #2 + AMEND-22.3-1).
6. 0 net-new stats per §7 (SHARED-vacuous); stat count STAYS 107.
7. ADR-0193 §Decision + §Consequences body landed in DECISIONS.md (per §16 + §3.2).
8. ADR-0125 §(xiv) IN-PLACE AMENDMENT body landed (canonical roster 8 → 9; the 9th canonical specification + SHARED stat-discipline + 9-shape roster table + lua-row first-use citation). No new ADR number.
9. CONDITIONAL ADR-0194 §Context + §Decision + §Consequences body — ONLY if the §13-R6 *LState-pool benchmark gate fires (`> 1ms`).
10. NEW fuzzer `FuzzLuaPerRouteConfig` at ADR-0018 must-never-panic baseline; `FuzzLuaConfigParse` corpus extended with `source_codes` seeds; project count 30 → 31.
11. Differential fixture `0028-http-lua-multi-script-and-per-route` GREEN with the 5 deterministic cross-side scenarios (a)-(e) + the boot-reject scenarios (f)-(h) per §8 (cross-side-vs-subject-only per §12-D-P2). 29 → 30 fixture directories.
12. BEHAVIOR_CONTRACT.md edit bundle landed atomically per ADR-0052 + §14 (0 new departure records; notes + ADR-0125 cross-reference; departure count UNCHANGED at 14).
13. R6 *LState-pool gate disposition recorded at the 22.3 IMPL benchmark task (anticipated WEAK-default STANDS).
14. **Parent row 22 flips `in-progress → done`** per ADR-0106 per-cell IMPL-done annotation (22.3 is the final sub-phase); STATE.md re-advance + ROADMAP row 22.3 flipped `in-progress → done`.
15. Per-task PROGRESS.md entries per the phase-21 + 22.1 + 22.2 IMPL precedent; each quotes command outputs per `superpowers:verification-before-completion`.
16. REVIEW.md authored at 22.3 IMPL phase-done per `superpowers:requesting-code-review`.

---

## 16. ADR §Context-draft anchor at 22.3 SPEC commit

Per ADR-0044 §Context-draft discipline. At THIS 22.3 SPEC commit, **1 NEW ADR §Context draft anchors** (ADR-0193) with a full §Context block describing the SPEC-time decision context + the IMPL-time bodies that will land. §Decision + §Consequences bodies land at 22.3 IMPL atomic-landing Task per ADR-0044 in-place edit discipline.

### 16.1 ADR-0193 §Context draft

**Title (provisional):** "NEW combined `internal/filter/http/lua/` 22.3 multi-script + per-route surface — `Lua.SourceCodes` named-script map consume + `LuaPerRoute` 9th-canonical 3-arm oneof + per-route 3-tier dispatch + upstream-parity dangling-`name` silent no-op (AMEND-22.3-1) + no-reserved-name disposition + fixture-0028 5-tier cross-side + `FuzzLuaPerRouteConfig` per 22.3 BRAINSTORM §2 + 22.3 SPEC §3-§11."

**Lands-in:** 22.3 IMPL atomic-landing Task per PLAN.

**Title cross-reference:** Phase-22.1 ADR-0189 (paired predecessor package-shape ADR; 22.3 extends) + ADR-0192 (22.2 package-shape ADR; 22.3 extends) + ADR-0188 (predecessor `internal/lua/` primitive; API-REVISION ALLOWANCE STAYS scoped to consumer-#2) + ADR-0153 (jwt_authn 8th-canonical split-semantic; the per-route runtime-resolve half corroborates AMEND-22.3-1) + ADR-0110 (`RegisterPerRouteValidator` single-chokepoint) + ADR-0125 §(xiv) (AMENDMENT-anticipation paragraph UNCHANGED at this SPEC; body lands at 22.3 IMPL final Task) + ADR-0154 (SHARED stat-discipline) + 22.3 BRAINSTORM §2.1-§2.13 + 22.3 SPEC §3 + §4 + §5 + §6 + §7 + §8 + §11.

---

## Appendix A — Cross-references to parent + 22.1 + 22.2 SPECs + 22.3 BRAINSTORM

This 22.3 SPEC cross-references the following content (inherited verbatim; NOT duplicated here):

- **Parent SPEC §3.1** (split surface-mapping — 22.3 column) + §4.5 (ADR-0125 §(xiv) AMENDMENT-anticipation) + §5.1/§5.2/§5.3 (proto rosters — CONFIRMED at §11.1) + §6.3 (22.3 forward-pointer arms — minus arm 3 + arm 7 per §6) + §7.3 (stat delta 22.3 = 0) + §11 (7-pin block).
- **22.1 SPEC §3** (`internal/lua/` API + content-hash compile cache + `internal/filter/http/lua/` package shape) + §11 (empirical-pin pattern) — REUSED at 22.3.
- **22.2 SPEC §1-§16** (full bridge surface; the 22.3 named/override scripts inherit it unchanged) + §13-R6 (*LState-pool gate carry-forward).
- **22.3 BRAINSTORM §1-§12** (8 LOCKED decisions + 4 D-questions + anticipated ADR roster + §11 scrape obligations) — fully incorporated at this SPEC §1-§16, with decision #3 REVISED per AMEND-22.3-1.

---

## Appendix B — Phase 22.3 ADR landings summary

At THIS 22.3 SPEC commit: **1 NEW ADR §Context draft anchors** (ADR-0193). DECISIONS.md tail advances from ADR-0192 → ADR-0193 §Context. Next-free ADR advances from `ADR-0193` → `ADR-0194`.

At 22.3 IMPL atomic-landing Task per PLAN:

- **ADR-0193 §Decision + §Consequences body** — NEW combined multi-script + per-route package-shape extension. Per §3.2 + §4 + §5 + §6 + §11 + AMEND-22.3-1.
- **ADR-0125 §(xiv) IN-PLACE AMENDMENT body** — NEW 9th canonical per-route shape (roster 8 → 9). No new ADR number. The anticipation paragraph at the parent SPEC commit STANDS UNCHANGED through this SPEC.
- **CONDITIONAL ADR-0194 §Context + §Decision + §Consequences body** — ONLY if the §13-R6 *LState-pool gate fires at the 22.3 IMPL benchmark. If unconsumed: ADR-0194 stays next-free (parent row 22 closes; no successor sub-phase to carry the buffer).

**Next-free ADR after 22.3 SPEC commit:** `ADR-0194`. After 22.3 IMPL: `ADR-0194` (if R6 stands) or `ADR-0195` (if the escape-valve fires).
