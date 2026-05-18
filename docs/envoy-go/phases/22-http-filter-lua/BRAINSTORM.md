# Phase 22 Brainstorm — `envoy.filters.http.lua` (parent row)

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 22 (`http-filter-lua`), the FIFTEENTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, `header_mutation` at phase 10, `local_ratelimit` at phase 11, `csrf` at phase 12, `buffer` at phase 13, `compressor` at phase 14, `bandwidth_limit` at phase 15, `rbac` at phase 16, `jwt_authn` at phase 17, `ext_authz` at phase 18 with its 18.1+18.2 split, `ext_proc` at phase 19 with its 19.1+19.2 split, `oauth2` at phase 20, and `adaptive_concurrency` at phase 21). **Phase 22 is the FIRST §9 family-row to pre-split THREE-way at BRAINSTORM time** (prior splits — phase 05, 06, 07, 08, 18, 19 — all settled at TWO sub-phases either at SPEC time or at PLAN time per ADR-0045 §6). The 3-way pre-split lands as sub-phases `22.1-http-filter-lua-vm-and-headers-bridge` + `22.2-http-filter-lua-full-bridge` + `22.3-http-filter-lua-multi-script-and-per-route`, anchored by the Q2 user decision at BRAINSTORM time.

The next session (lifecycle-state 1 → 2 for phase 22, skill `superpowers:brainstorming` per ADR-0005 scoped to **parent SPEC authoring** per the phase 18 + phase 19 parent-row precedent) authors `docs/envoy-go/phases/22-http-filter-lua/SPEC.md` based on this brainstorm — that parent SPEC is responsible for formalizing the 3-way split surface-mapping + executing the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004. The per-sub-phase SPEC sessions (22.1 / 22.2 / 22.3) follow the parent SPEC; each sub-phase's SPEC lands at its own dedicated session per the phase 18.1 / 18.2 / 19.1 / 19.2 precedent.

**Brainstorm session:** worktree `.worktrees/phase-22-http-filter-lua-brainstorm`, branch `phase-22-http-filter-lua-brainstorm`, branched from master tip `e63feab` (the phase 21 IMPL follow-up STATE.md SHA-fill commit — `phase 21 IMPL follow-up: STATE.md SHA-fill (TBD → 473e970 post-squash)`). The phase 21 squash-merge commit `473e970` and its SHA-fill follow-up `e63feab` are the immediate predecessors on master. `e63feab` is the current master tip.

**Brainstorm mode:** interactive with a live human. The user picked filter selection + each major design decision via a 12-question dialogue (Q1 §9 family-row pick — `lua` chosen from the 4-candidate remaining list `lua / wasm / admission_control / global rate limit`; Q1 MVP envelope — `D. Full upstream parity (SourceCodes map + 3-arm per-route oneof)` chosen from `A. Parse-only / B. Single-script headers-only / C. Single-script full bridge / D. Full upstream parity`; Q2 phase-split — `Pre-split now: 22.1 (envelope B) + 22.2 (envelope C delta) + 22.3 (multi-script + per-route)` chosen from `2-way pre-split / 3-way pre-split / Single phase / Other`; Q3 VM library — `github.com/yuin/gopher-lua` chosen from `gopher-lua / aarzilli/golua / Shopify/go-lua`; Q4 framework-primitive trigger — `NEW internal/lua/ framework primitive at first consumer (extract NOW)` chosen from `in-package with EXTRACT-NOW trigger / extract NOW / in-package no-trigger`; Q5 DataSource scope — `In-package; honor all 4 arms; PARSE-REJECT WatchedDirectory` chosen from `in-package-all-arms / in-package-inline-only / NEW internal/datasource primitive / InlineString-only`; Q6 22.1 bridge scope — `Pragmatic-middle: minimal + :streamInfo() subset + request_handle:respond()` chosen from `Minimal / Pragmatic-middle / Full streamInfo + :connection`; Q7 per-route shape — `NEW 9th canonical: 3-arm hybrid (disabled-bool + string-reference-delegation + DataSource-wholesale-override)` chosen from `NEW-9th / COMPOUND-mapping / EXTENSION-of-5th`; Q8 22.1 stat surface — `Upstream + envoy-go-strict extensions: errors + executions + respond_calls (3 counters; 99 → 102)` chosen from `Upstream-exact / Upstream-plus-extensions / Defer-to-SPEC`; Q9 differential-fixture strategy — `Full cross-side byte-exact for ALL 22.1 scenarios` chosen from `Partial-cross-side / Full-cross-side / REFERENCE-LESS / Defer-to-SPEC`; Q10 ADR-0044 escape-valve hypothesis — `WEAK HOLD: 0-1 extra ADRs at 22.1 IMPL (one-slot escape-valve consumption)` chosen from `STRONG-HOLD / WEAK-HOLD / BREAK / Defer-to-SPEC`; Q11 sub-phase slug convention — `22.1-http-filter-lua-vm-and-headers-bridge / 22.2-http-filter-lua-full-bridge / 22.3-http-filter-lua-multi-script-and-per-route (long-prefix style)` chosen from `Long-prefix / Short-prefix / Lifecycle-style`; Q12 sub-phase directory pre-creation — `Pre-create all 4 directories (parent + 22.1 + 22.2 + 22.3) with placeholder READMEs at this BRAINSTORM` chosen from `Pre-create-all / Pre-create-parent-only`). The §9 family-row continuation is implicit per ADR-0106. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0187, where ADR-0186 + ADR-0187 landed at phase 21 IMPL Tasks 3 + 2 and the ADR-0059 §Decision AMENDMENT body landed at phase 21 IMPL Task 4), and the just-shipped phase 21 + phase 20 + phase 19 + phase 18 + phase 17 + earlier-phase artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §9 and deferred to parent-SPEC-drafting time per the phase 09–21 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/21-http-filter-adaptive-concurrency/BRAINSTORM.md` and `docs/envoy-go/phases/18-http-filter-ext-authz/BRAINSTORM.md` section-for-section, reframed for the lua scope + the 3-way pre-split + the parent-row design discipline. Phase 22 sits in a structurally important position relative to the §9 family: it is the FIRST §9 family-row to (i) **pre-split THREE-way at BRAINSTORM time** (the prior 2-way splits at phases 05 / 06 / 07 / 08 / 18 / 19 settled at SPEC or PLAN time, not BRAINSTORM); (ii) **END the phase-21 ZERO-NEW-framework-primitive streak** by introducing a NEW `internal/lua/` framework primitive at 22.1 first-consumer (per Q4); (iii) **END the FOUR-CONSECUTIVE ADR-0125-skip streak** (phases 18 / 19 / 20 / 21 all skipped) by amending ADR-0125's canonical roster from 8 → 9 entries at phase 22.3 IMPL (per Q7 NEW 9th canonical); (iv) **introduce envoy-go's first third-party Lua VM dependency** (gopher-lua, MIT-licensed pure-Go Lua 5.1 interpreter per Q3) — adds a NEW go.mod direct dependency parallel in spirit to phase-20 oauth2's introduction of golang.org/x/oauth2 + golang.org/x/crypto; (v) **commit to full cross-side byte-exact differential fixture from the first sub-phase** (per Q9) — the project's first cross-side byte-exact fixture without scope-deviation since phase-19.2 (phase-20 oauth2 went REFERENCE-LESS subject-only; phase-21 adaptive_concurrency went REFERENCE-LESS subject-only with the 503-overflow leg DEFERRED as RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION). Sections §§1–11 are decision-bearing prose; §9 enumerates the empirical-pin obligations the parent SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear. NO off-master prebrainstorm-notes branch was authored for phase 22 — this brainstorm cold-started fresh from the §9 heading + the phase 21 just-shipped artefacts per ADR-0106(e). (Note: an off-master `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` branch exists but is unrelated to phase 22 — it was specific to phase 11.)

**Authored:** 2026-05-18.

---

## 1. Mission and scope confirmation (22 only)

ROADMAP row `22 | http-filter-lua | 21 | in-progress | 22.1, 22.2, 22.3 | …` (added by this brainstorm, see §10 + the ROADMAP edit) is the parent row this brainstorm registers as `in-progress` with sub-phase list `22.1, 22.2, 22.3`. The three sub-rows `22.1 | http-filter-lua-vm-and-headers-bridge | 21 | planned | | …`, `22.2 | http-filter-lua-full-bridge | 22.1 | planned | | …`, `22.3 | http-filter-lua-multi-script-and-per-route | 22.2 | planned | | …` are also registered by this brainstorm (per Q11 long-prefix slug convention + Q12 pre-create-all directory convention). Phase 22 is the FIFTEENTH concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 66 — `### HTTP filters family` — is a conceptual umbrella, not a row, per ADR-0106). The phase 21 squash-merge commit `473e970` (with SHA-fill at `e63feab`) is the parent row's `depends-on` anchor.

The HTTP filters family lists candidate filters at `ROADMAP.md` line 66: `Header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit`. After phase 22 lands, the §9 family has 15 row-landings (`cors` at 07.1; `fault` at 09; `header_mutation` at 10; `local_ratelimit` at 11; `csrf` at 12; `buffer` at 13; `compressor` at 14; `bandwidth_limit` at 15; `rbac` at 16; `jwt_authn` at 17; `ext_authz` at 18 with 18.1 + 18.2; `ext_proc` at 19 with 19.1 + 19.2; `oauth2` at 20; `adaptive_concurrency` at 21; `lua` at 22 with 22.1 + 22.2 + 22.3) — 3 candidate rows remain on the roster (`wasm`, `admission_control`, `global rate limit`). The chosen branch + directory + Go-package identifier are aligned per the existing convention: parent branch `phase-22-http-filter-lua-brainstorm` (preserves underscore-equivalent in the lua → lua mapping; lua is a single token), parent directory `22-http-filter-lua/` (per ADR-0106 row-id-plus-slug convention), Go-package identifier `lua` (single token; matches `cors` / `fault` / `buffer` / `csrf` precedent).

Phase 22 is also: (i) the FIRST §9 family-row whose configuration **delegates per-request behavior to operator-authored interpreted scripts** — every prior §9 row had a fully-declarative configuration surface (parse → compile → execute against a fixed algorithm); phase 22 introduces a NEW class of filter where the operator-provided proto-config carries arbitrary Turing-complete script source code that executes at every request. This is structurally distinct from all 14 prior §9 rows and motivates a careful sandbox + per-worker scoping discipline (§7.2 named risk surfaces + §9.3 SPEC-time §10 empirical pin). (ii) the FIRST §9 family-row to introduce a **third-party Lua VM dependency** in envoy-go — the repo currently has zero Lua dependencies; the gopher-lua choice (per Q3) adds `github.com/yuin/gopher-lua` as a NEW direct go.mod dependency at 22.1 + a pinned version reference in `internal/lua/doc.go` per ADR-0008-equivalent discipline (parent SPEC anchors the exact version pin at §9 empirical-pin time). Prior third-party dependency introductions: phase 17 added `github.com/lestrrat-go/jwx/v3` for JWT verification (ADR-0148); phase 20 added `golang.org/x/oauth2` + `golang.org/x/crypto` for OAuth2 flows. Phase 22 is the FIRST to introduce a Turing-complete script interpreter as a dependency. (iii) the FIRST §9 family-row to **pre-split THREE-way at BRAINSTORM time** (per Q2 user decision) — the prior 2-way splits at phases 05 / 06 / 07 / 08 / 18 / 19 all settled at SPEC time (the parent SPEC declared the split via §11.1-style mechanism) or at PLAN time (per ADR-0045 §6 LoC-driven gate). Phase 22's 3-way pre-split at BRAINSTORM time is a notable departure — rationale at §1.4. (iv) the FIRST §9 family-row to introduce a NEW `internal/lua/` framework primitive at first-consumer (per Q4 EXTRACT-NOW choice) — ENDS the phase-21 ZERO-NEW-framework-primitive streak (phase 21 was the FIRST §9 row since phase 14 compressor to introduce zero framework primitives; phase 22 returns to a framework-delta-growth posture). (v) the FIRST §9 family-row to amend ADR-0125's canonical-pattern roster since phase 17 jwt_authn (phases 18 / 19 / 20 / 21 all skipped ADR-0125 amendment — the FOUR-CONSECUTIVE skip streak ends at phase 22.3 IMPL per Q7 NEW 9th canonical). The 9th canonical is structurally distinct from all 8 prior canonicals: a 3-arm hybrid combining the 5th canonical's disabled-bool, the 8th canonical's string-reference-delegation, and a DataSource-typed wholesale-override (§4 + §2.7 + Q7). ADR-0125's roster grows from 8 → 9 at phase 22.3 IMPL.

### 1.1 What phase 22 delivers as a self-contained whole (envelope D)

Phase 22 lands `envoy.filters.http.lua` (the canonical Envoy HTTP Lua scripting filter, envelope D — full upstream parity per Q1 user pick: full gopher-lua VM + full Envoy↔Lua bridge surface + named `SourceCodes` map + `LuaPerRoute` 3-arm oneof + ADR-0125 NEW 9th canonical amendment) under the 07.1 framework. The envelope is delivered across THREE sub-phases (per Q2 + Q11 + Q12):

1. **Sub-phase 22.1** (`22.1-http-filter-lua-vm-and-headers-bridge`) — delivers: (a) the NEW `internal/lua/` framework primitive (gopher-lua VM lifecycle + script-compilation cache + sandbox config + bridge-registration interface + per-worker VM scoping); (b) the NEW `internal/filter/http/lua/` package shape (filter struct + parse + DataSource resolution + filter callbacks); (c) the `Lua.DefaultSourceCode` field consumed (top-level script when no per-route override); (d) the 4-arm DataSource resolution (Filename + InlineBytes + InlineString + EnvironmentVariable; WatchedDirectory PARSE-REJECT) per Q5; (e) the pragmatic-middle Envoy↔Lua bridge surface per Q6 (envoy_on_request / envoy_on_response hooks + `request_handle:headers()` / `response_handle:headers()` + headers-object methods + `:logXxx()` log methods + `:streamInfo()` subset + `request_handle:respond()`); (f) the 3-counter stat surface per Q8 (`errors` + `executions` + `respond_calls`; project total 99 → 102; envoy-go-strict departure record at BEHAVIOR_CONTRACT.md); (g) PARSE-REJECT for `SourceCodes` map + `LuaPerRoute` (deferred to 22.3); PARSE-REJECT for `InlineCode` deprecated field (envoy-go-strict departure — upstream supports the deprecated field with a warn log; envoy-go PARSE-REJECTs per the project's deprecated-field-rejection discipline); (h) the 28th project-wide fuzzer `FuzzLuaConfigParse` at the standard ~30-corpus-seed baseline; (i) the differential fixture `0026-http-lua-headers-bridge` — full cross-side byte-exact for ALL scenarios per Q9 (the first cross-side byte-exact fixture without scope-deviation since phase 19.2; relies on Lua errors going to log/stat without wire-body emission per upstream behavior); (j) the BEHAVIOR_CONTRACT.md 7-edit bundle at IMPL final Task per ADR-0052 atomic landing (NEW `### envoy.filters.http.lua` subsection + stat-table 99 → 102 extension + 3-counter envoy-go-strict departure record + 22.1-bridge-scope departure record + `InlineCode`-PARSE-REJECT departure record + NEW `### Phase 22.1 forward-pointer notes` subsection + per-route-canonical cross-reference caption update); (k) STATE.md re-advance to `phase 22.1 IMPL done; awaiting 22.2 SPEC` + ROADMAP row 22.1 flipped `planned → done` with per-cell IMPL-done annotation.

2. **Sub-phase 22.2** (`22.2-http-filter-lua-full-bridge`) — delivers the full bridge-API delta on top of 22.1's pragmatic-middle: (a) `:body()` / `:bodyChunks()` body-access surface (interacts with the phase-13 ADR-0128 decode-side body-buffering primitive); (b) `:trailers()` trailer-access surface; (c) `:metadata()` dynamic-metadata bridge — likely PARSE-REJECT or partial-deferral given the project's dynamic-metadata-deferred discipline across phases 16 / 17 / 18 / 19 / 20 (22.2 SPEC settles); (d) `:connection()` connection-info surface (SSL/TLS access via the phase-03 TLS primitives); (e) `:httpCall()` outbound HTTP call (reuses phase-20 `internal/httpclient/` primitive at 22.2 first co-consumer — validates the phase-20 framework-primitive extraction); (f) the `:respond()` surface extended to async-on-completion (after `:body()` buffering); (g) crypto / base64 / sha helpers + `:fileBytes()` + `:timestamp()` + full `:streamInfo()`; (h) additional stat-surface entries (httpCall counters likely); (i) differential fixture `0027-http-lua-full-bridge` — falls back to REFERENCE-LESS subject-only for non-deterministic httpCall scenarios (per Q9 partial-cross-side fallback for 22.2 specifically). PARSE-REJECT still active on `SourceCodes` map + `LuaPerRoute` at 22.2 (deferred to 22.3).

3. **Sub-phase 22.3** (`22.3-http-filter-lua-multi-script-and-per-route`) — delivers: (a) the `SourceCodes` named-script map (consumed); (b) the `LuaPerRoute` 3-arm oneof (Disabled | Name | SourceCode) consumed; (c) the NEW 9th canonical per-route shape ADR-0125 amendment (roster 8 → 9; anchored at 22.3 IMPL per ADR-0044); (d) the per-route 3-tier dispatch (per-route override → listener-level default → no-op) — re-uses the existing 3-tier resolution from phase 13/14/15 + extends with the Name string-reference-delegation pattern (the 8th canonical's discipline); (e) differential fixture `0028-http-lua-multi-script-and-per-route` — cross-side byte-exact for the multi-script + per-route scenarios that produce deterministic wire output.

### 1.2 What phase 22 does NOT deliver (forward to §8)

See §8 for the explicit deferred-items list. Highlights:

- **WatchedDirectory DataSource arm** (hot-reload of scripts) — PARSE-REJECT at config-load per Q5; deferred to a future Runtime / RTDS / hot-reload family phase.
- **`Lua.InlineCode` deprecated field** — PARSE-REJECT per envoy-go-strict deprecated-field-rejection discipline; the field exists in v1.32.4 + v1.37.x proto bindings but is marked `[deprecated = true]` in the proto IDL.
- **Lua 5.2 / 5.3 / 5.4 dialect features** — gopher-lua is Lua 5.1 (matches upstream Envoy's LuaJIT 5.1 dialect); scripts using 5.2+ features (`goto`, integer subtype, bit32 / utf8 stdlib) get Lua-compile-time errors at script-source compilation.
- **Dynamic-metadata bridge surfaces (`:metadata()` + `:streamInfo():dynamicMetadata()` + `:streamInfo():dynamicTypedMetadata()`)** — settled at 22.2 SPEC; likely PARSE-REJECT or partial-deferral per the project's cross-phase dynamic-metadata-deferral discipline.
- **Async script execution coroutines** — upstream Envoy's Lua filter supports per-script Lua coroutines for yielding during `:httpCall()` and `:body()` await. envoy-go's 22.2 implementation may or may not adopt coroutines (22.2 SPEC settles); alternative is goroutine-resume-on-completion (mirrors phase-09 fault async primitive + phase-18.2 ext_authz_grpc async-resume primitive).
- **Lua-VM-per-worker scoping** — envoy-go's HCM dispatch model (per-stream goroutine; no fixed worker pool) does NOT map 1:1 onto Envoy's per-worker thread model. Per-VM-per-worker is not directly translatable; 22.1 SPEC settles the equivalent scoping discipline (likely: per-script-source compilation cache + per-stream VM execution context).
- **Cross-side byte-exact for the 22.2 + 22.3 non-deterministic scenarios** — `:httpCall()` + `:body()` + `:timestamp()` introduce non-determinism that complicates cross-side byte-comparison; partial cross-side at 22.2 + REFERENCE-LESS fallback for httpCall scenarios.

### 1.3 Phase-done as the FIFTEENTH §9 family-row landing

Phase 22 closes the FIFTEENTH §9 family-row across its 3 sub-phases. The remaining §9 row count drops from 4 to 3 post-phase-22 (`wasm`, `admission_control`, `global rate limit`). Phase 22 also retires the `lua` line item from ROADMAP §9 (`Header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit`).

### 1.4 ADR-0045 split-by-surface readiness — HIGH at BRAINSTORM; 3-way pre-split chosen per Q2

Per ADR-0045 §6, the split-gate fires when `PLAN.md > ~25 tasks OR > ~1500 LoC estimated`. Phase 22's envelope D surface is anticipated to exceed BOTH gates substantially: (a) the NEW `internal/lua/` framework primitive alone (VM lifecycle + script-compilation cache + sandbox config + bridge-registration interface) is ~600-900 LoC; (b) the `internal/filter/http/lua/` package across all 3 sub-phases is anticipated at ~2500-3500 LoC (envelope D matches phase-18 ext_authz scope which landed at ~3200 LoC across 18.1+18.2; the Lua bridge surface is comparable in breadth); (c) the per-sub-phase task counts are anticipated at 14-18 per sub-phase × 3 sub-phases = 42-54 tasks total (vs phase-21 single-row at 14 tasks). Total anticipated LoC ~3500-5000, task count ~42-54 — both far above the ADR-0045 split-gate.

The user picked the 3-way pre-split at Q2 (rather than the 2-way pre-split or single-phase-with-SPEC-time-re-evaluation). The 3-way split axes are natural: 22.1 = VM + DefaultSourceCode + headers-only bridge (envelope-B-equivalent); 22.2 = full bridge delta (envelope-C-delta); 22.3 = multi-script + per-route (envelope-D-delta). Each sub-phase is independently shippable + delivers operator-visible value: 22.1 ships a usable Lua filter (deny-by-header, log-by-route, direct-respond); 22.2 extends with body access + outbound HTTP calls; 22.3 extends with multi-script-per-route operator ergonomics. Each sub-phase is anticipated at ~1200-1500 LoC + ~14-18 tasks — fits cleanly under the ADR-0045 gate per sub-phase.

The 3-way pre-split at BRAINSTORM time is a notable departure from the project's prior split discipline. All prior 2-way splits (phases 05 / 06 / 07 / 08 / 18 / 19) settled at SPEC time (the parent SPEC declared the split via §11.1-style mechanism) or at PLAN time (per ADR-0045 §6 LoC-driven gate). Phase 22 settles at BRAINSTORM time because the envelope D scope is unambiguous + the natural split axes are clear at BRAINSTORM design-decision time — there's no SPEC-time uncertainty to resolve. The parent SPEC will formalize the 3-way split surface-mapping + the per-sub-phase scope boundaries. Per ADR-0106, the parent row registers as `in-progress` with sub-phases listed; each sub-row registers as `planned` until the corresponding sub-phase opens.

### 1.5 Seed-stub alignment + package naming

No seed-stub for lua exists in `internal/filter/http/` (consistent with the §9 family-row pattern; each row creates its own package). Phase 22.1 creates `internal/filter/http/lua/` from scratch + the NEW `internal/lua/` framework primitive package from scratch. Package directory + Go-package identifier are both `lua` (single token; matches `cors` / `fault` / `csrf` / `buffer` / `compressor` / `oauth2` / `rbac` precedent). The framework primitive's directory + Go-package identifier are both `lua` at `internal/lua/` (matches `internal/jwks/` + `internal/jwt/` + `internal/httpclient/` + `internal/sdsfile/` + `internal/grpcclient/` + `internal/matcher/` precedent).

### 1.6 No prebrainstorm-notes branch

No `phase-22-http-filter-lua-prebrainstorm-notes` branch exists. Phase 22 starts cleanly from this BRAINSTORM.md. (The off-master `phase-11-http-filter-local-ratelimit-prebrainstorm-notes` branch noted in project memory `reference_phase_11_local_ratelimit_prebrainstorm.md` is specific to phase 11 and unrelated to phase 22.)

### 1.7 Phase 22's relationship to prior framework deltas + framework-delta accretion shape

Phase 22 **ENDS the phase-21 ZERO-NEW-framework-primitive streak**. The prior §9 row landings carried diverse framework-delta postures:

- Phase 14 compressor — REUSE-only (the FIRST §9 row since phase 11 to introduce zero new framework primitives; pre-phase-21 leanest §9 row)
- Phase 15 bandwidth_limit — REUSE of `internal/ratelimit/` token-bucket primitive
- Phase 16 rbac — NEW `internal/matcher/` (typed matcher framework) + several in-place ADR amendments
- Phase 17 jwt_authn — NEW `internal/jwks/` Fetcher + NEW `internal/jwt/` verifier + extensive matcher reuse
- Phase 18.1 + 18.2 ext_authz — NEW `internal/grpcclient/` framework primitive (gRPC dial pool)
- Phase 19.1 + 19.2 ext_proc — REUSE of the phase-18 `internal/grpcclient/` primitive + extensive matcher reuse
- Phase 20 oauth2 — NEW `internal/httpclient/` + NEW `internal/sdsfile/` framework primitives + IN-PLACE AMENDMENTs to ADR-0150 + ADR-0159
- Phase 21 adaptive_concurrency — ZERO new framework primitives (LEANEST §9 row to date; in-package `Clock` seam stayed in-package per the EXTRACT-NOW-only-when-trigger-fires discipline; in-place §Decision AMENDMENT body to ADR-0059 for the float-valued-gauge int64-encoding convention)
- **Phase 22.1 lua — NEW `internal/lua/` framework primitive at first consumer (per Q4 EXTRACT-NOW choice).** First-of-kind for the project: every prior NEW framework primitive landed at ≥2 anticipated consumers at extraction time (jwks at jwt_authn + future-extauthz reuse; grpcclient at ext_authz + future-ext_proc reuse; httpclient at oauth2 + future-Lua-httpCall reuse; sdsfile at oauth2 + future-SDS-secret-injection reuse; matcher at rbac + future-jwt_authn + future-ext_authz + future-ext_proc reuse). Phase 22.1's `internal/lua/` is extracted at consumer #1 (lua filter only); the future-consumer roster (cluster specifier Lua at `envoy.router.cluster_specifiers.lua`, access logger Lua at `envoy.access_loggers.lua`, string matcher Lua at `envoy.string_matcher.lua`) is speculative — none committed to the ROADMAP.

The 22.1 first-consumer extraction discipline carries higher abstraction-shape revision risk than prior multi-consumer-at-extraction extractions: the `internal/lua/` API may need revision after consumer #2 materializes (e.g., the bridge-registration interface may not generalize cleanly from HTTP filter to cluster specifier). The 22.1 ADR (ADR-0188) anchors the abstraction shape WITH AN EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 — mirrors phase-20 ADR-0159 §Future Work CLOSURE-AT-PHASE-20 pattern but in the opposite direction (anticipating future revision rather than closing prior revision).

This is structurally healthy for the project's accretion shape: phase 21 was a framework-delta-zero row (leanest); phase 22 returns to a framework-delta-growth posture with a measured 1-NEW-primitive + 0-in-place-AMENDMENT-at-22.1 + 1-in-place-AMENDMENT-at-22.3 (ADR-0125 amendment). The next §9 rows (wasm, admission_control, global rate limit) can be sequenced by their own framework-delta weight without coupling: wasm likely introduces a NEW `internal/wasm/` primitive (proxy-wasm ABI + WASM engine); admission_control likely reuses the phase-21 `Clock` seam (which would trigger the EXTRACT-NOW promotion to `internal/clock/`); global rate limit likely reuses `internal/grpcclient/` for the RLS gRPC client.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The brainstorm dialogue settled 12 Q-decisions. Each is anchored here with rationale + the anticipated ADR or REUSE classification.

### 2.1 Scope ambition: envelope D — full upstream parity *(Q1 → ADR-0188 + ADR-0189 + 22.2 ADRs + 22.3 ADRs + ADR-0125 amendment)*

**Decision:** Land the FULL upstream `envoy.filters.http.lua` operator-visible surface across 3 sub-phases — full gopher-lua VM + full Envoy↔Lua bridge surface + named `SourceCodes` map + `LuaPerRoute` 3-arm oneof. NO subset-deferral within envelope D; the only field-level deferrals are: (i) `WatchedDirectory` DataSource arm (PARSE-REJECT per Q5); (ii) `Lua.InlineCode` deprecated field (PARSE-REJECT per envoy-go-strict deprecated-field-rejection discipline).

**Rationale:** Matches phase-18 ext_authz + phase-20 oauth2 precedent of landing the full upstream operator-visible surface for the row's first phase-set. Phase-21 adaptive_concurrency was the only §9 row to defer a structural field (the `enabled.RuntimeFeatureFlag` runtime-coupling); phase 22 has no comparable structural-coupling deferral. The 3-way pre-split (per Q2) allows each sub-phase to deliver a usable operator-visible feature increment without forcing a single-row landing.

**Anticipated ADRs:** ADR-0188 (`internal/lua/` framework primitive — VM lifecycle + script-compilation cache + sandbox config + bridge-registration interface; anchored at 22.1 IMPL); ADR-0189 (lua filter package shape + DataSource resolution + envoy-go-strict stat departures + full-cross-side fixture discipline; anchored at 22.1 IMPL); 22.2 ADRs settled at 22.2 BRAINSTORM (anticipated: full-bridge-API shape + httpCall dispatcher + body-buffering interaction with ADR-0128 + dynamic-metadata-bridge deferral); 22.3 ADRs settled at 22.3 BRAINSTORM (anticipated: NEW 9th canonical per-route shape + per-route 3-tier dispatch + ADR-0125 amendment).

### 2.2 Phase split: 3-way pre-split at BRAINSTORM time *(Q2 → ROADMAP rows 22 + 22.1 + 22.2 + 22.3 registered)*

**Decision:** Pre-split phase 22 into three sub-phases at BRAINSTORM time (rather than declaring split at SPEC time or PLAN time). Sub-phase boundaries follow natural surface axes: 22.1 = VM + DefaultSourceCode + pragmatic-middle bridge (envelope-B-equivalent surface); 22.2 = full bridge delta (envelope-C-delta); 22.3 = multi-script + per-route (envelope-D-delta). Each sub-phase is independently shippable + delivers operator-visible value. ROADMAP registers parent row 22 `in-progress` + sub-rows 22.1 / 22.2 / 22.3 `planned` per ADR-0106. Sub-phase directories pre-created at this BRAINSTORM per Q12.

**Rationale:** Envelope D substantially exceeds the ADR-0045 split-gate (anticipated ~3500-5000 LoC + ~42-54 tasks total). The natural split axes are unambiguous at BRAINSTORM design-decision time; no SPEC-time uncertainty to resolve. The 3-way pre-split at BRAINSTORM is a project FIRST — prior 2-way splits settled at SPEC time (parent SPEC declared via §11.1-style mechanism) or PLAN time (per ADR-0045 §6 LoC-driven gate). Phase 22's BRAINSTORM-time settlement is justified by the envelope-D unambiguity + the natural surface-axis cleanliness.

**Anticipated ADRs:** ROADMAP row registrations (4 new rows: parent 22 + 22.1 + 22.2 + 22.3). No new ADR specifically for the 3-way split decision — ADR-0045 § 6 already covers the split-discipline; the 3-way-at-BRAINSTORM application is a precedent extension not requiring new ADR.

### 2.3 Lua VM library: gopher-lua *(Q3 → ADR-0188 anchor + parent SPEC version pin per ADR-0008-equivalent discipline)*

**Decision:** Embed `github.com/yuin/gopher-lua` as the Lua VM library. Pure Go Lua 5.1 interpreter. Matches upstream Envoy's LuaJIT 5.1 dialect. NO CGO. Added as a NEW direct go.mod dependency at 22.1.

**Rationale:** gopher-lua is the de-facto pure-Go Lua interpreter (used by Caddy, Consul, Hashicorp tooling, etc.); Lua 5.1 dialect matches script-author expectations (Envoy LuaJIT scripts are written for 5.1 semantics); pure-Go aligns with envoy-go's posture (no existing CGO dependencies; introducing CGO at phase 22 would carry portability burden for marginal benefit). The alternative `aarzilli/golua` CGO binding to real Lua C / LuaJIT was rejected on portability cost grounds (musl/glibc, cross-compilation, Docker base-image impact); the alternative `Shopify/go-lua` Lua 5.2 was rejected on dialect-mismatch grounds (Lua 5.2's `goto`, integer subtype, `bit32` / `utf8` stdlib introduce script-author confusion).

**Anticipated ADRs:** ADR-0188 §Decision includes the gopher-lua choice + the version pin discipline (the parent SPEC author anchors the specific version pin per ADR-0008-equivalent discipline; v1.32.4 → v1.37.2 reference Envoy uses LuaJIT 2.1.0-beta3 at the v1.37.2 pin per the upstream Envoy build).

### 2.4 Framework-primitive trigger: NEW `internal/lua/` at first consumer *(Q4 → ADR-0188 §Decision)*

**Decision:** Extract the gopher-lua VM lifecycle as a NEW `internal/lua/` framework primitive at phase 22.1 (consumer #1 — the HTTP Lua filter). The primitive hosts: (a) VM lifecycle (init, sandbox config, panic-recovery, per-stream execution context); (b) script-compilation cache (compile-once-execute-many discipline keyed by the script source's content hash); (c) sandbox config (which gopher-lua stdlib modules to allow / deny — parent SPEC settles); (d) bridge-registration interface (generic mechanism for consumers to register Go callbacks as Lua functions; the HTTP-filter-specific bridge methods like `request_handle:headers()` live in the consumer package). The bridge-registration interface separates the generic primitive (VM lifecycle + registration mechanism) from the consumer-specific bridge methods.

**Rationale:** The Lua VM is a substantially larger surface than phase-21's in-package `Clock` seam — script-compilation caching, sandbox config, per-stream execution context, panic recovery, bridge-registration interface — totaling ~600-900 LoC. Extracting after consumer #2 materializes would require significant refactoring of the entire 22.1 surface. The user picked EXTRACT-NOW at Q4 over the in-package-with-EXTRACT-NOW-trigger approach (which would have matched the phase-21 Clock-seam discipline); the bolder choice ENDS the phase-21 ZERO-NEW-framework-primitive streak but accepts the abstraction-shape revision risk at consumer #2. The 22.1 ADR (ADR-0188) anchors the abstraction shape WITH AN EXPLICIT API-REVISION ALLOWANCE clause for consumer #2.

**Anticipated ADRs:** ADR-0188 §Decision includes the package boundary (`internal/lua/` = generic VM lifecycle + registration mechanism; `internal/filter/http/lua/` = HTTP-filter-specific bridge methods) + the API-REVISION ALLOWANCE clause for consumer #2 (cluster specifier / access logger / string matcher Lua phases). The future-consumer roster is documented as a forward-pointer in §Consequences — none of the future Lua consumers are committed on the ROADMAP at phase-22 BRAINSTORM time; each future-phase BRAINSTORM revisits the `internal/lua/` API shape against its specific needs.

### 2.5 DataSource resolution: in-package, 4 arms + WatchedDirectory PARSE-REJECT *(Q5 → ADR-0189 anchor)*

**Decision:** DataSource resolution lives in-package at `internal/filter/http/lua/datasource.go`. Honor 4 arms at config-load time: (a) `Filename` — read file at config-load; PARSE-REJECT if file is not readable or empty; (b) `InlineBytes` — use bytes directly; PARSE-REJECT if empty; (c) `InlineString` — use string directly; PARSE-REJECT if empty; (d) `EnvironmentVariable` — read env var at config-load; PARSE-REJECT if env var is unset or empty. PARSE-REJECT `WatchedDirectory` (hot-reload deferred to future phase per §8). Resolution happens once at config-load; the resolved script source is compiled to a gopher-lua chunk via `internal/lua/` primitive's compile API + cached on the `*compiledConfig` for the lifetime of the filter instance.

**Rationale:** Matches existing per-filter DataSource handling (jwt_authn at `internal/filter/http/jwtauthn/`; oauth2 at `internal/filter/http/oauth2/`). The alternative — extracting `internal/datasource/` as a NEW framework primitive — was rejected on scope grounds (retrofitting jwt_authn + oauth2 + others is out-of-phase-22-scope; would significantly inflate 22.1 LoC). The 4-arm scope matches envelope D's "full upstream parity" intent (per Q1); the WatchedDirectory deferral mirrors phase-21's `enabled.RuntimeFeatureFlag` deferral (anchor: any field coupling to a runtime / hot-reload subsystem not yet wired in envoy-go).

**Anticipated ADRs:** ADR-0189 §Decision includes the 4-arm DataSource resolution + the WatchedDirectory PARSE-REJECT arm. The WatchedDirectory PARSE-REJECT carries a forward-pointer to the future Runtime / RTDS / hot-reload family phase (parallel to phase-21's ADR-0187 RTDS `enabled.RuntimeFeatureFlag` PARSE-REJECT forward-pointer).

### 2.6 Phase 22.1 bridge-API scope: pragmatic-middle *(Q6 → ADR-0189 anchor)*

**Decision:** Phase 22.1's Envoy↔Lua bridge surface is the pragmatic-middle cut: (a) top-level hooks `envoy_on_request(request_handle)` + `envoy_on_response(response_handle)`; (b) `request_handle:headers()` + `response_handle:headers()` returning headers objects with methods `:get(key)` + `:getAtIndex(key, index)` + `:getNumValues(key)` + `:add(key, value)` + `:append(key, value)` + `:remove(key)` + `:replace(key, value)` + `__pairs` iterator metamethod; (c) `:logTrace(msg)` + `:logDebug(msg)` + `:logInfo(msg)` + `:logWarn(msg)` + `:logErr(msg)` + `:logCritical(msg)` log methods (routed to envoy-go's log subsystem); (d) `:streamInfo()` returning a streamInfo object with the subset `:protocol()` + `:routeName()` + `:downstreamLocalAddress()` + `:downstreamDirectRemoteAddress()` (operators can gate by route, identify the listener-side socket, log per-route diagnostics); (e) `request_handle:respond(headers, body)` for synchronous direct response (operators can enforce policy by short-circuiting). NOT IN 22.1 (deferred to 22.2): `:body()` + `:bodyChunks()` + `:trailers()` + `:metadata()` + `:connection()` + `:httpCall()` + crypto helpers + `:base64Escape()` + `:base64Decode()` + `:sha256()` + `:sha512()` + `:importPublicKey()` + `:verifySignature()` + `:fileBytes()` + `:timestamp()` + full `:streamInfo()` surface (`:upstreamHost()` + `:upstreamCluster()` + `:dynamicMetadata()` + `:dynamicTypedMetadata()` + `:requestedServerName()` + `:filterState()` + `:downstreamSslConnection()`). Deferred methods raise Lua runtime errors when called by a 22.1 script — the Lua-idiomatic disposition (scripts can `pcall` to handle gracefully).

**Rationale:** Phase 22.1's pragmatic-middle gives operators a useful starter feature set (gate by route, log diagnostics, enforce policy via direct response) without bloating 22.1's LoC budget. The minimal-headers-only cut (option a at Q6) was rejected on operator-value grounds (can't gate by route or direct-respond — too thin for operator adoption). The full-streamInfo + `:connection()` cut (option c) was rejected on dynamic-metadata-coupling grounds (`:streamInfo():dynamicMetadata()` pulls forward the project's dynamic-metadata deferral discipline; better to keep that surface in 22.2 where the bridge-API decisions are settled holistically). Mirrors phase-21 "deliver operator-visible core" pragmatic-middle pattern.

**Anticipated ADRs:** ADR-0189 §Decision includes the 22.1 bridge-API roster + the Lua-runtime-error disposition for deferred methods. The Lua-runtime-error disposition is the Lua-idiomatic pattern (raise an error that scripts can catch with `pcall`); the alternative — PARSE-REJECT at config-load by scanning the Lua source AST for unsupported calls — was rejected on idiomatic grounds (Lua is a dynamic language; static analysis of script sources for unsupported method calls is non-idiomatic and brittle).

### 2.7 Per-route shape: NEW 9th canonical *(Q7 → ADR-0125 amendment + 22.3 ADR)*

**Decision:** Classify `LuaPerRoute` as a NEW 9th canonical per-route pattern. `LuaPerRoute` is a 3-arm oneof: (a) `disabled: bool` — disable Lua for this route (matches 5th canonical's disabled-bool arm); (b) `name: string` — string-reference into the parent `Lua.SourceCodes` map (matches 8th canonical's string-reference-delegation pattern); (c) `source_code: *core.DataSource` — wholesale-override inline script (matches 5th canonical's wholesale-override sub-message pattern, but using `DataSource` rather than a parent-config sub-message). The 3-arm hybrid is structurally distinct from all 8 prior canonicals — no prior canonical combines disabled-bool + string-reference-delegation + DataSource-wholesale-override in a single oneof. The 9th canonical's stat-discipline is SHARED (per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors` stat namespace; matches jwt_authn 8th canonical's SHARED discipline because per-route does NOT introduce a separate stat_prefix). ADR-0125's canonical-pattern roster grows from 8 → 9 at phase 22.3 IMPL.

**Rationale:** The 3-arm hybrid is genuinely new — no prior canonical combines all three structural patterns in a single oneof. The COMPOUND-mapping alternative (treating each arm as mapping to an existing canonical without adding a new entry) was rejected on classification-signal grounds (loses the structural-classification signal that the 3-arm-oneof-with-DataSource shape is a new pattern; future Lua-flavor consumers can't compose against a named canonical). The EXTENSION-of-5th alternative (in-place §Decision amendment to ADR-0125's 5th-canonical paragraph) was rejected on misclassification grounds (the 3-arm shape is not a 2-arm-extension; the DataSource type is not a parent-config sub-message; treating it as an extension hides the structural distinctness).

**Anticipated ADRs:** A NEW ADR at 22.3 IMPL anchors the 9th canonical's full specification + the per-route 3-tier dispatch shape (per-route override → listener-level default → no-op). ADR-0125 IN-PLACE AMENDMENT adds the 9th canonical to the roster (§canonical-per-route-roster grows from 8 → 9; matches phase-16 + phase-17 ADR-0125 amendment precedent). The phase-22.3 IMPL Lands-in-Tasks: the new-canonical ADR + the ADR-0125 amendment both anchor at the same Lands-in-Task per ADR-0044 in-place edit discipline.

### 2.8 Phase 22.1 stat surface: 99 → 102 with envoy-go-strict extensions *(Q8 → ADR-0189 anchor + BEHAVIOR_CONTRACT departure record)*

**Decision:** Phase 22.1 exposes 3 counters under the `lua.<config_stat_prefix>.<stat>` prefix template: (a) `errors` (upstream-parity; counter increments per script execution error); (b) `executions` (envoy-go-strict extension; counter increments per script invocation regardless of outcome — gives operators per-script-call visibility); (c) `respond_calls` (envoy-go-strict extension; counter increments per `:respond()` short-circuit invocation — gives operators visibility into how often Lua short-circuits the upstream call). Project stat count 99 → 102 (delta +3). The 2 envoy-go-strict extensions (`executions` + `respond_calls`) require BEHAVIOR_CONTRACT.md envoy-go-strict departure records at the IMPL final Task's 7-edit bundle (parallel to phase-21's RTT-ns-vs-ms + sorted-slice-vs-CircllHist departure records).

**Rationale:** The upstream-exact-only alternative (just `errors` — option a at Q8) was rejected on operator-visibility grounds — the project's stat surface is intentionally LARGER than upstream's where operator value justifies the divergence (phase-19 ext_proc has similar envoy-go-strict additions; phase-21 has the RTT-gauge-in-ns vs upstream-ms departure). The 3-counter cut at 22.1 gives operators the minimum useful telemetry surface (error rate + invocation rate + short-circuit rate); the 22.2 + 22.3 stat-surface additions (httpCall counters at 22.2; SourceCodes-named-script per-name counters possibly at 22.3) are anticipated as forward-pointers but not committed at this BRAINSTORM.

**Anticipated ADRs:** ADR-0189 §Decision includes the 22.1 stat-surface roster + the envoy-go-strict departure justification + the BEHAVIOR_CONTRACT departure-record discipline. The departure records anchor at the 22.1 IMPL final Task's 7-edit bundle per ADR-0052 atomic landing.

### 2.9 Differential-fixture strategy: full cross-side byte-exact at 22.1 *(Q9 → ADR-0189 anchor + parent SPEC §10 gopher-lua-vs-LuaJIT empirical scrape)*

**Decision:** Differential fixture `0026-http-lua-headers-bridge` lands as a FULL cross-side byte-exact fixture for ALL phase 22.1 scenarios. Scenarios are selected to produce deterministic wire output across both gopher-lua and LuaJIT: (a) add-fixed-header scenario (Lua script adds `x-lua-injected: hello` to request + response); (b) replace-header scenario (Lua script replaces `user-agent` header); (c) remove-header scenario (Lua script removes a configured-blocked header); (d) `:respond()` short-circuit scenario (Lua script short-circuits with a deterministic 403 + body); (e) log-only-passthrough scenario (Lua script logs but does not mutate the request — wire output identical to upstream's bypass-through); (f) headers-iteration scenario (Lua script iterates headers via `__pairs` and adds a `x-headers-count: N` header where N is deterministic); (g) script-compile-error scenario (config-load PARSE-REJECT — no wire interaction). Lua runtime errors at script execution go to the `lua.<stat_prefix>.errors` counter + the envoy-go log; the WIRE response is NOT modified (matches upstream Lua filter behavior). The full cross-side byte-exact discipline at 22.1 is the FIRST cross-side byte-exact fixture without scope-deviation since phase 19.2 (phase-20 oauth2 went REFERENCE-LESS subject-only; phase-21 adaptive_concurrency went REFERENCE-LESS subject-only with the 503-overflow leg DEFERRED).

**Rationale:** Phase 22.1's bridge surface (per Q6 pragmatic-middle) is fully deterministic — no `:httpCall()` (deferred to 22.2), no `:body()` (deferred to 22.2), no `:timestamp()` (deferred to 22.2), no random or time-based methods. The wire output is fully deterministic. The risk surface — gopher-lua vs LuaJIT observable divergence — is contained because: (a) Lua runtime errors don't appear on wire (logged + stat increment + continue without mutation); (b) the bridge methods used in fixture scenarios are simple enough that gopher-lua and LuaJIT produce byte-identical observable behavior (header manipulation is deterministic; `:respond()` is deterministic; `__pairs` iteration order in 5.1 is deterministic given the headers-map ordering). The risk is non-zero — gopher-lua and LuaJIT may diverge on edge cases (number formatting in `:logXxx()` messages; string concatenation of mixed types; etc.) — but the fixture scenarios are chosen to avoid those edges. The parent SPEC author validates this at §10 empirical-pin scrape time.

**Anticipated ADRs:** ADR-0189 §Decision includes the full-cross-side fixture discipline + the scenario taxonomy + the gopher-lua-vs-LuaJIT observable-divergence risk-surface acknowledgment. The parent SPEC §10 empirical-pin obligations include a gopher-lua-vs-LuaJIT byte-comparison scrape across all 22.1 bridge methods (validates the fixture's deterministic-output assumption empirically).

### 2.10 ADR-0044 escape-valve hypothesis: WEAK HOLD at 22.1 IMPL *(Q10 → 22.1 IMPL phase-done verification)*

**Decision:** BRAINSTORM-time prediction for phase 22.1 IMPL: WEAK HOLD — 2 anticipated ADRs (ADR-0188 + ADR-0189) land cleanly; 0-1 unanticipated ADR fires from the escape-valve (one-slot consumption). The most likely escape-valve surfaces at 22.1 IMPL: (a) **gopher-lua sandbox discipline** — what stdlib modules to allow / deny (os.*, io.*, debug.* are obvious deny-list candidates; the exact roster may surface IMPL-time discoveries); (b) **per-worker VM scoping** — envoy-go's per-stream goroutine dispatch doesn't map 1:1 onto Envoy's per-worker thread model; the equivalent scoping discipline (per-script-source compile cache + per-stream execution context) may surface IMPL-time refinements; (c) **gopher-lua-vs-LuaJIT observable divergence at fixture-0026** — the parent SPEC §10 scrape may surface divergences that require an envoy-go-strict departure ADR. ZERO-slot buffer post-IMPL (the phase-21 STRENGTHENED two-slot buffer at ADR-0188 + ADR-0189 fully collapses at 22.1 IMPL phase-done).

**Rationale:** Phase 22.1 is a NEW-framework-primitive landing at first-consumer — first-of-kind for the project. The phase-21 STRONG-HOLD discipline (predict zero escape-valve consumption) is too aggressive for a first-of-kind framework-primitive landing where the abstraction shape may surface IMPL-time discoveries. The WEAK HOLD allows for one escape-valve consumption to absorb sandbox / scoping / divergence discoveries without requiring BRAINSTORM-time pre-resolution of those decisions. The BREAK alternative (1-2 extra ADRs predicted) was rejected as over-pessimistic — the named risk surfaces are all amenable to single-ADR resolution.

**Anticipated ADRs:** WEAK-HOLD prediction means the BRAINSTORM-anticipated ADR roster for 22.1 IMPL is: ADR-0188 (NEW `internal/lua/` framework primitive; anchored at the lands-in-task for the primitive's first commit per ADR-0044) + ADR-0189 (lua filter package shape + DataSource resolution + envoy-go-strict stat departure + full-cross-side fixture discipline; anchored at the lands-in-task for the filter package's first commit per ADR-0044). The escape-valve slot ADR-0190 is the WEAK-HOLD-allowance for an unanticipated IMPL-time discovery.

### 2.11 Sub-phase slug naming: long-prefix style *(Q11 → ROADMAP row registrations + worktree path)*

**Decision:** Sub-phase directory slugs use the long-prefix style matching phase-19 precedent: `22.1-http-filter-lua-vm-and-headers-bridge` + `22.2-http-filter-lua-full-bridge` + `22.3-http-filter-lua-multi-script-and-per-route`. Each sub-phase slug carries the full `http-filter-lua` prefix + a descriptive surface-axis suffix. Highly discoverable; clearly subordinates each sub-phase to the parent row's identity.

**Rationale:** Phase-18 used short-prefix style (`18.1-ext-authz-http` + `18.2-ext-authz-grpc`) but the prefix was already short ("ext-authz" is 2 hyphenated words). Phase-19 used long-prefix style (`19.1-http-filter-ext-proc-headers` + `19.2-http-filter-ext-proc-body`) which is consistent with the parent row's slug. Phase 22's parent slug is `http-filter-lua`; the long-prefix style preserves the full identity in the sub-phase slug, matching phase-19. The lifecycle-style alternative (`22.1-lua-mvp` + `22.2-lua-bridge` + `22.3-lua-multi-script`) was rejected on consistency grounds (no prior precedent for lifecycle-style suffixes; all prior splits used surface-axis suffixes).

### 2.12 Sub-phase directory pre-creation: all 4 at this BRAINSTORM *(Q12 → worktree structure)*

**Decision:** Pre-create all 4 phase directories (parent `22-http-filter-lua/` + sub-phases `22.1-http-filter-lua-vm-and-headers-bridge/` + `22.2-http-filter-lua-full-bridge/` + `22.3-http-filter-lua-multi-script-and-per-source/`) at this BRAINSTORM. Each sub-phase dir gets a one-paragraph README pointing back to the parent BRAINSTORM.md + listing the sub-phase's anticipated artefacts. Strongest discoverability; per-sub-phase SPEC sessions just fill in SPEC.md.

**Rationale:** Phase-18 + phase-19 created sub-phase dirs at per-sub-phase SPEC time (not at parent BRAINSTORM). The user picked pre-create-all-at-BRAINSTORM (option a at Q12) over the phase-18/19 precedent (option b) — the user's rationale: highest discoverability + clearest sub-phase scoping signal during the long gap between parent BRAINSTORM and the eventual 22.2 / 22.3 sub-phase SPEC sessions. The pre-created READMEs serve as forward-pointers + scope-boundary documentation; they do NOT pre-empt the per-sub-phase BRAINSTORM-level design decisions (the README is descriptive of anticipated scope, not prescriptive of design).

---

## 3. Framework-survey result — 1 NEW package-level primitive + 0 IN-PLACE AMENDMENTs at 22.1 + 7 REUSES + 1 IN-PLACE AMENDMENT at 22.3 (ADR-0125)

Phase 22 introduces 1 NEW package-level framework primitive at 22.1 (`internal/lua/` per Q4) + 0 in-place ADR amendments at 22.1 + 7 framework REUSES across the envelope D surface + 1 IN-PLACE AMENDMENT to ADR-0125 at 22.3 (NEW 9th canonical per Q7). Phase 22 ENDS the phase-21 ZERO-NEW-framework-primitive streak — the FIRST §9 row since phase 17 (jwt_authn at `internal/jwks/` + `internal/jwt/`) to introduce a NEW framework primitive of substantial scope.

### 3.1 NEW: `internal/lua/` framework primitive *(per Q4 EXTRACT-NOW decision; anchored at ADR-0188; lands at 22.1)*

**Decision:** Extract the gopher-lua VM lifecycle as a NEW `internal/lua/` framework primitive at phase 22.1. Package boundary: `internal/lua/` hosts the GENERIC VM lifecycle (init, sandbox config, panic-recovery, per-stream execution context, script-compilation cache, bridge-registration interface); `internal/filter/http/lua/` hosts the HTTP-FILTER-SPECIFIC bridge methods (`request_handle:headers()`, `response_handle:headers()`, etc.). The bridge-registration interface is the seam between the two: consumers register their Go callbacks as Lua-callable functions via the primitive's API; the primitive doesn't know about HTTP-specific concepts.

The primitive's API shape (provisional; 22.1 SPEC settles):
- `lua.NewVM(opts ...VMOption) *VM` — construct a per-stream VM execution context
- `lua.CompileScript(source []byte, cache *CompileCache) (*Chunk, error)` — compile a script to a reusable chunk
- `lua.NewCompileCache() *CompileCache` — construct a per-filter-instance compile cache keyed by source content hash
- `VM.RegisterBridgeMethod(name string, fn lua.GoFunction)` — register a Go function as a Lua-callable bridge method
- `VM.Run(chunk *Chunk, hooks ...HookFn) error` — execute a chunk with given hooks (envoy_on_request, envoy_on_response)
- `VM.Close()` — release the VM's resources
- `VMOption` types: `WithSandboxConfig(sb SandboxConfig)`, `WithPanicRecovery(rcvr PanicRecoveryFn)`, `WithStderrSink(sink io.Writer)`, etc.

The API-REVISION ALLOWANCE clause per Q4: the primitive's API shape is provisional at consumer #1 (HTTP filter Lua); the second consumer (cluster specifier Lua / access logger Lua / string matcher Lua, whichever materializes first) may require API revision after empirical validation. The 22.1 ADR (ADR-0188) anchors the API shape with an EXPLICIT API-REVISION ALLOWANCE clause referencing the future consumer-#2 phase as the validation event.

### 3.2 REUSE 1: `internal/stats/` Counter support

`internal/stats/registry.go` + `internal/stats/counter.go` already host first-class counter support. The 22.1 3-counter stat surface (`errors` + `executions` + `respond_calls`) is constructed via `Registry.NewCounter`. No framework work. 22.2 + 22.3 stat additions reuse the same primitive.

### 3.3 REUSE 2: HTTPRegistry boot-time registration

`internal/filter/http/registry.go::HTTPRegistry` already supports `Register(typeURL, factory)` + `Freeze` + `Lookup`. Boot wires `lua.New` at `cmd/envoy-go/main.go` between `localratelimit.New` (alphabetical predecessor) and `oauth2.New` (alphabetical successor). The exact insertion position: post-phase-21 the boot has 16 HTTP filters; phase 22.1 adds the 17th — `router → adaptive_concurrency → bandwidthlimit → buffer → compressor → cors → csrf → envoygotest → extauthz → extproc → fault → header_mutation → jwtauthn → localratelimit → lua → oauth2 → rbac → Freeze`. The `lua` insertion is alphabetical between `localratelimit` and `oauth2`. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

### 3.4 REUSE 3: Per-request filter interface (decode/encode hooks)

The existing HCM filter framework already supports per-request filter instances with decodeHeaders / encodeHeaders / decodeData / encodeData / decodeTrailers / encodeTrailers callbacks. Lua filter's `envoy_on_request(request_handle)` hook fires at decodeHeaders; `envoy_on_response(response_handle)` hook fires at encodeHeaders. The `request_handle` / `response_handle` Lua objects are bridge proxies that delegate to the underlying HCM callbacks. No framework extension.

### 3.5 REUSE 4: HCM-parse-time PARSE-REJECT path

The existing HCM parser already rejects unknown type_urls + invalid typed_config bodies at parse time. Lua filter's parse logic adds: PARSE-REJECT `InlineCode` (deprecated field — envoy-go-strict departure), PARSE-REJECT `SourceCodes` + `LuaPerRoute` at 22.1 (deferred to 22.3), PARSE-REJECT `WatchedDirectory` DataSource arm, PARSE-REJECT empty DataSource, PARSE-REJECT unreadable file, PARSE-REJECT unset env var, PARSE-REJECT script-compile-failure (gopher-lua compile error). The exact arm count + wording is settled at parent SPEC time per §10 empirical-pin.

### 3.6 REUSE 5: Per-route 3-tier dispatch (extended at 22.3)

At 22.1 + 22.2: no per-route surface (PARSE-REJECT). At 22.3: the existing 3-tier per-route dispatch from phase 13/14/15 + the string-reference-delegation pattern from phase-17 jwt_authn's 8th canonical compose into the NEW 9th canonical. Per-route resolution: filter callback callback receives the per-route override (if present); the filter's `parsePerRoute` validates the 3-arm oneof + dispatches per the 9th canonical's discipline (Disabled → no-op; Name → lookup in parent `SourceCodes` map + dispatch to named script; SourceCode → use the per-route inline override script).

### 3.7 REUSE 6: Existing fuzzer-corpus framework

`internal/filter/http/fuzz_test.go` already hosts the cross-filter fuzzer registry. Lua adds `FuzzLuaConfigParse` as the 28th project-wide fuzzer at the standard ~30-corpus-seed baseline + the standard PARSE-REJECT arm coverage (per the 22.1 PARSE-REJECT roster).

### 3.8 REUSE 7: Existing differential-fixture framework

`test/fixtures/` + `test/differential/runner_test.go` already host the differential fixture runner. Lua adds: (a) `0026-http-lua-headers-bridge/` at 22.1 (full cross-side byte-exact per Q9); (b) `0027-http-lua-full-bridge/` at 22.2 (partial cross-side / REFERENCE-LESS fallback for non-deterministic scenarios); (c) `0028-http-lua-multi-script-and-per-route/` at 22.3 (cross-side byte-exact for the deterministic multi-script + per-route scenarios). Fixture count: 27 (post-phase-21) → 28 (post-22.1) → 29 (post-22.2) → 30 (post-22.3).

### 3.9 IN-PLACE AMENDMENT at 22.3: ADR-0125 NEW 9th canonical

Per Q7. ADR-0125 §canonical-per-route-roster grows from 8 → 9 at phase 22.3 IMPL. The IN-PLACE AMENDMENT body adds the 9th canonical's specification (3-arm hybrid: disabled-bool + string-reference-delegation + DataSource-wholesale-override) + the SHARED stat-discipline + the structural-distinctness rationale (combines 5th + 8th in a single oneof; structurally distinct from all 8 prior canonicals). Anchored at the 22.3 IMPL final Task per ADR-0044 in-place edit discipline.

---

## 4. Per-route shape — NEW 9th canonical (3-arm hybrid; SHARED stat-discipline)

The `LuaPerRoute` proto defines a 3-arm oneof `override` with arms:
- `disabled: bool` (field 1) — when `true`, the Lua filter is wholly inactive on this route
- `name: string` (field 2) — string-reference into the listener-level `Lua.SourceCodes` map; the named script runs for this route instead of `DefaultSourceCode`
- `source_code: *core.DataSource` (field 3) — inline-override script; this DataSource resolves to a script source that overrides `DefaultSourceCode` for this route

The 3-arm hybrid combines 3 structural patterns from prior canonicals in a single oneof:
- **Disabled-bool arm** matches the 5th canonical's `disabled: true` arm (phase 13 buffer + phase 14 compressor: `BufferPerRoute.oneof override { disabled: true | buffer: BufferPerRoute_PerRouteBufferConfig }`).
- **Name string-reference arm** matches the 8th canonical's string-reference-delegation pattern (phase 17 jwt_authn: `PerRouteConfig.oneof requirement_specifier { disabled: bool | requirement_name: string }` where the string references the listener-level `JwtAuthentication.requirement_map`).
- **SourceCode DataSource-wholesale-override arm** is novel: the wholesale-override uses a `DataSource` type (which resolves to a script source string) rather than a parent-config sub-message. The 5th canonical's wholesale-override arm uses a parent-config sub-message (`BufferPerRoute_PerRouteBufferConfig`); the 9th canonical's wholesale-override arm uses `DataSource` (which is a different proto type, not a parent-config sub-message).

The structural distinctness vs all 8 prior canonicals: no prior canonical combines disabled-bool + string-reference-delegation + DataSource-wholesale-override in a single oneof. The 9th canonical's defining feature: a **3-arm oneof spanning disable-bool + string-reference-delegation + DataSource-wholesale-override**. Future §9 family-rows (or future cross-family rows) whose per-route proto follows the same 3-arm shape compose against the 9th canonical.

The 9th canonical's stat-discipline is SHARED (per-route errors charge to the listener-level `lua.<config_stat_prefix>.errors` stat namespace). Rationale: `LuaPerRoute` has no separate `stat_prefix` field — the listener-level filter's `stat_prefix` is the only stat-prefix-source. Per-route Lua errors are observationally indistinguishable from listener-level errors in the stat surface; the operator differentiates by route-name + the script-source identity (not by per-route stat-prefix). SHARED matches phase-17 jwt_authn 8th canonical (SHARED per ADR-0154) + phase-12 csrf + phase-13 buffer + phase-14 compressor SHARED-stats discipline. DIVERGES from phase-11 local_ratelimit + phase-15 bandwidth_limit + phase-16 rbac INDEPENDENT-stats discipline.

ADR-0125's roster post-phase-22.3 (grown 8 → 9):

| # | Pattern | First-use phase | Stat-discipline |
|---|---|---|---|
| 1 | No-per-route | cors (07.1 original) | n/a |
| 2 | Data-only TPFC bare-message | cors / fault | SHARED-vacuous |
| 3 | Multi-tier all-tier | header_mutation (10) | SHARED |
| 4 | INDEPENDENT-stats stateful | local_ratelimit (11) | INDEPENDENT |
| 5 | Disabled-bool + wholesale-override sub-message | buffer (13) + compressor (14) | SHARED |
| 6 | Bare-message-via-TPFC + code-level-required | bandwidth_limit (15) | INDEPENDENT |
| 7 | Wrapper-with-reserved-field + single optional sub-message | rbac (16) | INDEPENDENT |
| 8 | Oneof with string-reference-delegation + explicit-disable-bool | jwt_authn (17) | SHARED |
| **9** | **3-arm hybrid: disabled-bool + string-reference-delegation + DataSource-wholesale-override** | **lua (22.3)** | **SHARED** |

---

## 5. Stat surface hypothesis

### 5.1 22.1 stat-surface roster (3 counters; project 99 → 102)

Per Q8. Phase 22.1 exposes 3 counters under the `lua.<config_stat_prefix>.<stat>` prefix template:

| Name | Type | Semantics | Source |
|---|---|---|---|
| `errors` | Counter | Script execution errors (gopher-lua runtime errors caught by the panic-recovery wrapper) | Upstream parity |
| `executions` | Counter | Script invocations (every `envoy_on_request` / `envoy_on_response` call; increments regardless of outcome) | envoy-go-strict extension |
| `respond_calls` | Counter | `:respond()` short-circuit invocations | envoy-go-strict extension |

### 5.2 Stat-prefix template

Upstream Envoy publishes the surface under `lua.<config_stat_prefix>.<stat>` (the `Lua.stat_prefix` field qualifies the namespace). The exact template byte-exactness against upstream Envoy v1.37.2 is a **parent-SPEC-time §10 empirical pin** obligation per ADR-0004.

### 5.3 Project stat-count delta

99 → 102 at 22.1 (+3). 22.2 + 22.3 stat additions anticipated as forward-pointers (not committed at this BRAINSTORM):
- 22.2 anticipated additions: httpCall counters (likely `httpcall_total` + `httpcall_failures`) — total +2 if landed
- 22.3 anticipated additions: per-named-script error counters? (SHARED-vacuous discipline likely → +0 per the 9th canonical's SHARED-discipline rationale)
- Total post-phase-22 stat-count: 102 (22.1) + ~2 (22.2 if httpCall counters land) + 0 (22.3 SHARED-vacuous) = ~104

### 5.4 envoy-go-strict departure rationale + BEHAVIOR_CONTRACT.md departure-record discipline

Per Q8. The 2 envoy-go-strict extensions (`executions` + `respond_calls`) are NOT in upstream's stat surface; they are envoy-go-only additions for operator-visibility. The departure rationale: upstream's single `errors` counter gives operators error-rate visibility but lacks per-script invocation-rate + per-script-short-circuit-rate visibility — both operationally useful for capacity planning + policy-enforcement audit. Parallel to phase-19 ext_proc's envoy-go-strict stat additions + phase-21's RTT-gauge-in-ns vs upstream-ms departure.

BEHAVIOR_CONTRACT.md departure records anchor at the 22.1 IMPL final Task's 7-edit bundle per ADR-0052 atomic landing. The 2 records: (a) `executions` counter (envoy-go-only; operator-visibility rationale); (b) `respond_calls` counter (envoy-go-only; operator-visibility rationale). Mirrors phase-21's 2 envoy-go-strict departure records (RTT-ns + sorted-slice-percentile).

---

## 6. Differential-fixture strategy

### 6.1 22.1 fixture-0026: full cross-side byte-exact for ALL scenarios

Per Q9. Fixture `0026-http-lua-headers-bridge` lands as a FULL cross-side byte-exact fixture for ALL phase 22.1 scenarios. Fixture structure: single directory `test/fixtures/0026-http-lua-headers-bridge/` containing the envoy-go subject's `envoy-go.yaml` config + the reference Envoy's `envoy.yaml` config + the differential test runner's invocation entries.

### 6.2 Scenario taxonomy (7 scenarios)

All 7 scenarios produce deterministic wire output across both gopher-lua and LuaJIT:

| # | Name | Lua script behavior | Wire-output assertion |
|---|---|---|---|
| (a) | add-fixed-header | `function envoy_on_request(rh) rh:headers():add("x-lua-injected", "hello") end` | Request header `x-lua-injected: hello` present at upstream |
| (b) | replace-header | `function envoy_on_request(rh) rh:headers():replace("user-agent", "envoy-go-lua/1.0") end` | Request `user-agent: envoy-go-lua/1.0` at upstream |
| (c) | remove-header | `function envoy_on_request(rh) rh:headers():remove("x-blocked") end` | Request without `x-blocked` header at upstream (when input includes it) |
| (d) | respond-shortcircuit | `function envoy_on_request(rh) rh:respond({[":status"]="403"}, "denied") end` | Client receives 403 + body `denied` + no upstream request |
| (e) | log-only-passthrough | `function envoy_on_request(rh) rh:logInfo("lua hit") end` | Request unchanged at upstream + envoy log message |
| (f) | headers-iteration | `function envoy_on_request(rh) local n=0; for k,v in pairs(rh:headers()) do n=n+1 end; rh:headers():add("x-headers-count", tostring(n)) end` | Request header `x-headers-count: N` (N = deterministic) at upstream |
| (g) | script-compile-error | Lua source with intentional syntax error | Config-load PARSE-REJECT; envoy-go subject + reference Envoy both fail to start with byte-exact error logs (parent SPEC §10 empirical-pin validates error-message byte-equivalence) |

### 6.3 gopher-lua vs LuaJIT observable-divergence risk

The risk surface: gopher-lua (Lua 5.1 pure Go) and LuaJIT (Lua 5.1 with JIT extensions) MAY diverge on edge cases — number formatting in `:logXxx()` messages; string concatenation of mixed types; `__pairs` iteration order in 5.1 is documented-non-deterministic but envoy-go's headers-map iteration order is deterministic so the wire output is deterministic; `pcall` error-message format may differ. The fixture scenarios are chosen to avoid those edges. The parent SPEC author validates this empirically at §10 empirical-pin scrape time (gopher-lua-vs-LuaJIT byte-comparison across all 22.1 bridge methods).

If the parent SPEC empirical-pin scrape surfaces an actionable divergence, the parent SPEC author has 3 options: (a) refine the fixture scenarios to avoid the divergent edge; (b) wrap gopher-lua's behavior at the bridge-method level to match LuaJIT (intrusive — adds a compatibility shim per bridge method); (c) document the divergence as an envoy-go-strict departure record at BEHAVIOR_CONTRACT.md + fall back to REFERENCE-LESS for the affected scenarios. Choice (a) is preferred; (b) + (c) carry forward to the parent SPEC's D-decision slate.

### 6.4 22.2 + 22.3 fixtures (forward-pointer)

- **22.2 fixture-0027**: PARTIAL cross-side. `:headers()` + `:streamInfo()` + `:respond()` scenarios continue cross-side byte-exact. `:body()` + `:trailers()` + `:metadata()` scenarios may be cross-side byte-exact (deterministic) or REFERENCE-LESS (if dynamic-metadata-bridge is deferred). `:httpCall()` + `:timestamp()` scenarios fall back to REFERENCE-LESS subject-only (non-deterministic timing + non-deterministic outbound call ordering). 22.2 BRAINSTORM settles the exact taxonomy.
- **22.3 fixture-0028**: cross-side byte-exact for the multi-script + per-route scenarios that produce deterministic wire output. Scenario taxonomy similar to 22.1 but exercises the SourceCodes named-script lookup + the LuaPerRoute 3-arm oneof per-route override resolution. 22.3 BRAINSTORM settles the exact scenario list.

---

## 7. ADR-0044 escape-valve hypothesis

### 7.1 WEAK HOLD at 22.1 IMPL: 0-1 escape-valve consumption *(per Q10)*

BRAINSTORM-time prediction: phase 22.1 IMPL anchors 2 NEW ADRs (ADR-0188 + ADR-0189); 0-1 unanticipated ADR fires from the escape-valve (one-slot consumption from the phase-21 STRENGTHENED two-slot buffer at ADR-0188 + ADR-0189 + ADR-0190 + ADR-0191). The phase-21 STRENGTHENED two-slot buffer collapses to ZERO-slot post-22.1-IMPL (the 2 anticipated ADRs consume both buffer slots; any escape-valve consumes ADR-0190).

### 7.2 Named risk surfaces (3)

1. **gopher-lua sandbox discipline** — Which gopher-lua stdlib modules to allow / deny? Upstream Envoy's LuaJIT sandbox denies `os.*`, `io.*` (except limited write to log), `debug.*`, `package.*` (no module loading); the equivalent gopher-lua sandbox shape may surface IMPL-time refinements (e.g., gopher-lua's stdlib API may not map 1:1 onto LuaJIT's stdlib). The 22.1 IMPL may surface an ADR for the sandbox roster.
2. **Per-worker VM scoping** — envoy-go's per-stream goroutine dispatch doesn't map 1:1 onto Envoy's per-worker thread model. Envoy creates ONE Lua VM per worker thread + reuses the VM across all requests handled by that worker. envoy-go's equivalent: per-script-source compile cache (compile once, share the chunk across all streams) + per-stream VM execution context (each stream constructs a fresh `*lua.LState` from the compile cache; the LState is closed on stream completion). The exact discipline may surface IMPL-time refinements (e.g., gopher-lua's `*LState` construction cost may dictate a pool of pre-constructed LStates rather than per-stream construction).
3. **gopher-lua-vs-LuaJIT observable divergence at fixture-0026** — The parent SPEC §10 scrape may surface divergences requiring an envoy-go-strict departure ADR (e.g., `tostring(1.5)` may produce `"1.5"` in gopher-lua vs `"1.5"` in LuaJIT — most likely same — but edge cases like NaN, Infinity, very-large integers may diverge).

### 7.3 STRENGTHENED two-slot buffer disposition

Phase-21's STRENGTHENED two-slot buffer (ADR-0188 + ADR-0189 both unconsumed at phase-21 IMPL phase-done) carries forward to phase 22. Phase 22.1 IMPL consumes:
- **ADR-0188** (anticipated): NEW `internal/lua/` framework primitive
- **ADR-0189** (anticipated): lua filter package shape + DataSource resolution + envoy-go-strict stat departures + full-cross-side fixture discipline
- **ADR-0190** (escape-valve slot): WEAK-HOLD-allowance for an unanticipated IMPL-time discovery (sandbox / scoping / divergence per §7.2)
- **ADR-0191** (next-free post-22.1-IMPL): unconsumed; carries forward to 22.2 BRAINSTORM as the next-free slot

Post-22.1-IMPL disposition: ZERO-slot buffer (the two-slot buffer fully collapsed at 22.1 IMPL). 22.2 BRAINSTORM re-evaluates the buffer discipline + anticipates 22.2 IMPL ADR consumption.

---

## 8. Deferred items + forward-pointers (15 items)

The full envelope-D scope is delivered across 22.1 + 22.2 + 22.3. Items DEFERRED to future phases (cross-phase boundaries) + items FORWARD-POINTED for future SPEC / IMPL resolution:

1. **WatchedDirectory DataSource arm hot-reload** (Q5 deferral) — PARSE-REJECT at 22.1; deferred to future Runtime / RTDS / hot-reload family phase. The deferral mirrors phase-21 ADR-0187's `enabled.RuntimeFeatureFlag` PARSE-REJECT pattern.
2. **`Lua.InlineCode` deprecated field** (envoy-go-strict deprecated-field-rejection discipline) — PARSE-REJECT at 22.1; never re-enabled in envoy-go (the upstream field is marked `[deprecated = true]` in the proto IDL; envoy-go's discipline is to refuse deprecated fields rather than emit warn logs).
3. **Lua 5.2 / 5.3 / 5.4 dialect features** — gopher-lua is Lua 5.1; scripts using 5.2+ features (`goto`, integer subtype, `bit32` / `utf8` stdlib) get Lua-compile-time errors at script-source compilation; documented in `internal/lua/doc.go` + `internal/filter/http/lua/doc.go`.
4. **Dynamic-metadata bridge surfaces** (`:metadata()` + `:streamInfo():dynamicMetadata()` + `:streamInfo():dynamicTypedMetadata()`) — settled at 22.2 SPEC; likely PARSE-REJECT or partial-deferral per the project's cross-phase dynamic-metadata-deferral discipline (deferred at phases 16 / 17 / 18 / 19 / 20).
5. **Async script execution coroutines** — upstream Envoy's Lua filter supports per-script Lua coroutines for yielding during `:httpCall()` and `:body()` await. envoy-go's 22.2 implementation may or may not adopt coroutines (22.2 SPEC settles); alternative is goroutine-resume-on-completion (mirrors phase-09 fault async primitive + phase-18.2 ext_authz_grpc async-resume primitive).
6. **`:httpCall()` outbound HTTP call surface** (22.2) — reuses phase-20 `internal/httpclient/` framework primitive at first co-consumer (validates the phase-20 extraction).
7. **Body-buffering interaction with ADR-0128** (22.2 `:body()`) — `:body()` + `:bodyChunks()` interact with the phase-13 decode-side body-buffering primitive; 22.2 SPEC settles the exact interaction discipline.
8. **`:connection()` SSL/TLS access** (22.2) — connection-level info bridge; integrates with phase-03 TLS primitives.
9. **Crypto / base64 / sha helpers** (22.2) — `:base64Escape()` + `:base64Decode()` + `:sha256()` + `:sha512()` + `:importPublicKey()` + `:verifySignature()` — likely thin wrappers over Go's `crypto/*` + `encoding/base64`.
10. **`:fileBytes()` file-read helper** (22.2) — security caveats apply (sandbox concern); 22.2 SPEC settles the security discipline.
11. **`:timestamp()` time helper** (22.2) — non-deterministic; fixture cross-side byte-exact challenges for scenarios that include timestamps.
12. **Full `:streamInfo()` surface** (22.2) — `:upstreamHost()` + `:upstreamCluster()` + `:dynamicMetadata()` + `:dynamicTypedMetadata()` + `:requestedServerName()` + `:filterState()` + `:downstreamSslConnection()`.
13. **`SourceCodes` named-script map** (22.3) — multi-script lookup; the 9th canonical's Name arm depends on this surface.
14. **`LuaPerRoute` 3-arm oneof** (22.3) — per-route override; NEW 9th canonical per Q7.
15. **Cluster specifier Lua + access logger Lua + string matcher Lua** (future cross-family phases) — consumers #2/3/4 for the `internal/lua/` framework primitive; each future phase BRAINSTORM revisits the API shape per Q4's API-REVISION ALLOWANCE clause.

---

## 9. Parent-SPEC-time §10 empirical-pin obligations

Per ADR-0004. The parent SPEC author (next session per phase-09..21 precedent) resolves the following empirical pins against reference Envoy v1.37.2 IN-SESSION:

### 9.1 Proto-field roster scrape

Exhaustive proto-field inventory of `envoy.extensions.filters.http.lua.v3.Lua` + `envoy.extensions.filters.http.lua.v3.LuaPerRoute` against the v1.32.4 go-control-plane proto binding + the v1.37.2 reference Envoy upstream proto. Fields-consumed roster + fields-deferred roster (per §1.1 + §8 inventory). Sub-phase mapping: each field assigned to 22.1 / 22.2 / 22.3 per the surface boundary.

### 9.2 gopher-lua vs LuaJIT observable-output divergence

Byte-comparison scrape across all 22.1 bridge methods (headers, `:logXxx()`, `:streamInfo()` subset, `:respond()`). Methodology: write a deterministic Lua script that exercises each bridge method; run it under envoy-go (gopher-lua) and reference Envoy v1.37.2 (LuaJIT); compare wire outputs + log outputs. Document any divergences as parent-SPEC D-decisions (refine fixture scenarios / wrap divergent behavior / document envoy-go-strict departure).

### 9.3 Sandbox config + per-worker VM scoping

Empirically verify the gopher-lua sandbox roster against upstream Envoy's LuaJIT sandbox: which stdlib modules upstream allows / denies (per the upstream source/extensions/filters/http/lua/wrappers.cc); the equivalent gopher-lua roster. Per-worker VM scoping: envoy-go's per-script-source compile cache + per-stream LState construction discipline; empirically verify the LState construction cost (justifies per-stream construction or argues for a pool).

### 9.4 PARSE-REJECT roster build

Build the exhaustive PARSE-REJECT arm roster for `Lua` + `LuaPerRoute` (22.1's PARSE-REJECT roster) per the byte-stable error wording discipline per ADR-0080 + the prior §9 row PARSE-REJECT-roster precedents (phase-18 ext_authz, phase-19 ext_proc, phase-20 oauth2, phase-21 adaptive_concurrency). Anticipated arm count for 22.1: ~12-18 arms (4 DataSource arms × 3 PARSE-REJECT classes per arm + `InlineCode` deprecated + `SourceCodes` map deferral + `LuaPerRoute` map-key deferral + script-compile-failure + sandbox-violation).

### 9.5 Stat-surface byte-exactness

Verify the `lua.<config_stat_prefix>.errors` upstream stat name byte-exactly. Confirm the 22.1 envoy-go-strict extensions (`executions` + `respond_calls`) are NOT in upstream's surface (per Q8 departure justification).

### 9.6 `:respond()` wire-shape byte-exactness

Verify the `:respond(headers, body)` wire shape (status code + body + content-type + content-length) byte-exactly against upstream Envoy v1.37.2's Lua-filter `:respond()` invocation. Specifically: does upstream set `content-length` based on body length? Does upstream set `content-type` from the headers table or default to `application/octet-stream`?

### 9.7 Cross-side comparison test infrastructure for fixture-0026

Verify the differential-fixture runner supports cross-side byte-exact comparison for HTTP responses + envoy logs (the latter for scenario (g) script-compile-error). Identify any runner-infrastructure additions needed; document as parent-SPEC D-decisions.

---

## 10. ROADMAP row registrations

This BRAINSTORM registers 4 NEW rows on the ROADMAP per ADR-0106:

| id | title | depends-on | status | sub-phases | summary |
|---|---|---|---|---|---|
| `22` | `http-filter-lua` | `21` | `in-progress` | `22.1, 22.2, 22.3` | HTTP filter `envoy.filters.http.lua` envelope D. Full upstream parity via 3-way pre-split at BRAINSTORM: 22.1 = NEW `internal/lua/` framework primitive + DefaultSourceCode + pragmatic-middle bridge (headers + log + streamInfo subset + respond) + 4-arm DataSource + 3-counter stat surface + full cross-side fixture-0026; 22.2 = full bridge delta (body + httpCall + crypto + full streamInfo); 22.3 = SourceCodes + LuaPerRoute 3-arm oneof + ADR-0125 NEW 9th canonical amendment. |
| `22.1` | `http-filter-lua-vm-and-headers-bridge` | `21` | `planned` |  | NEW `internal/lua/` (gopher-lua VM lifecycle + script-compile cache + sandbox + bridge-registration); NEW `internal/filter/http/lua/`; DefaultSourceCode; 4-arm DataSource + WatchedDirectory PARSE-REJECT; pragmatic-middle bridge (headers + log + streamInfo subset + respond); 3-counter stat surface (errors + executions + respond_calls); 28th fuzzer; full cross-side fixture-0026; ADR-0188 + ADR-0189; WEAK-HOLD D-hypothesis. |
| `22.2` | `http-filter-lua-full-bridge` | `22.1` | `planned` |  | Full bridge delta on top of 22.1: `:body()` + `:bodyChunks()` + `:trailers()` + `:metadata()` + `:connection()` + `:httpCall()` (reuses `internal/httpclient/`) + crypto/base64/sha helpers + `:fileBytes()` + `:timestamp()` + full `:streamInfo()`. Additional stat additions (httpCall counters likely). Fixture-0027 partial cross-side / REFERENCE-LESS fallback for non-deterministic scenarios. |
| `22.3` | `http-filter-lua-multi-script-and-per-route` | `22.2` | `planned` |  | `SourceCodes` named-script map + `LuaPerRoute` 3-arm oneof + NEW 9th canonical per-route shape (3-arm hybrid: disabled-bool + string-reference-delegation + DataSource-wholesale-override) + ADR-0125 IN-PLACE AMENDMENT (roster 8 → 9). Fixture-0028 cross-side byte-exact for deterministic scenarios. |

The ROADMAP edit appends these 4 rows after row `21 | http-filter-adaptive-concurrency` and before the `### HTTP filters family` heading at ROADMAP line 66.

---

## 11. Sub-phase scope mapping

The full envelope-D scope decomposes into the 3 sub-phases as follows:

### 11.1 Phase 22.1 surface (envelope-B-equivalent core + framework primitive)

**Delivers:**
- NEW `internal/lua/` framework primitive: `VM` + `NewVM` + `CompileScript` + `NewCompileCache` + `Chunk` + `VMOption` + `WithSandboxConfig` + `WithPanicRecovery` + `WithStderrSink` + `*VM.RegisterBridgeMethod` + `*VM.Run` + `*VM.Close`. Anchored at ADR-0188.
- NEW `internal/filter/http/lua/` package: `lua.go` (filter struct + factory + filterStats), `compiled_config.go` (config parse + 4-arm DataSource resolution + PARSE-REJECT roster + script-compile cache key generation), `bridge.go` (HTTP-filter-specific bridge methods — headers + log + streamInfo subset + respond), `decode_headers.go` (envoy_on_request hook firing), `encode_headers.go` (envoy_on_response hook firing), `stats.go` (3-counter stat surface), `doc.go` (package overview + Q1-Q12 decision summary), `lua_test.go` (unit tests; anticipated 1500-2000 LoC), `compiled_config_test.go` (PARSE-REJECT roster table-driven tests), `bridge_test.go` (bridge-method unit tests), `fuzz_test.go` (28th fuzzer `FuzzLuaConfigParse`). Anchored at ADR-0189.
- `Lua.DefaultSourceCode` consumed; `Lua.SourceCodes` + `Lua.InlineCode` PARSE-REJECTed (the InlineCode deprecated field is rejected per envoy-go-strict; the SourceCodes map is rejected as deferred-to-22.3).
- `LuaPerRoute` PARSE-REJECTed at 22.1 (deferred to 22.3).
- `Lua.StatPrefix` consumed (qualifies the 3-counter stat surface namespace).
- 4-arm DataSource resolution + WatchedDirectory PARSE-REJECT per Q5.
- Differential fixture `0026-http-lua-headers-bridge` (full cross-side byte-exact for ALL 7 scenarios per §6.2).
- 28th fuzzer `FuzzLuaConfigParse`.
- BEHAVIOR_CONTRACT.md 7-edit bundle at IMPL final Task: NEW `### envoy.filters.http.lua` subsection + stat-table 99→102 + 3-counter envoy-go-strict departure record + 22.1-bridge-scope departure record + `InlineCode`-PARSE-REJECT departure record + NEW `### Phase 22.1 forward-pointer notes` subsection + per-route-canonical cross-reference caption update.
- STATE.md re-advance at IMPL final Task to `phase 22.1 IMPL done; awaiting 22.2 SPEC` + ROADMAP row 22.1 `planned → done` per ADR-0106.

**Anticipated ADR landings:** ADR-0188 (NEW `internal/lua/` framework primitive) + ADR-0189 (lua filter package shape + DataSource resolution + envoy-go-strict stat departures + full-cross-side fixture discipline). 0-1 escape-valve slot per Q10 WEAK-HOLD.

### 11.2 Phase 22.2 surface (full bridge delta)

**Delivers:** Full bridge-API delta on top of 22.1's pragmatic-middle. Settled in detail at 22.2 BRAINSTORM (next-session-after-22.1-IMPL-phase-done). The anticipated 22.2 surface:
- `:body()` + `:bodyChunks()` body-access surface (interacts with phase-13 ADR-0128 decode-side body-buffering).
- `:trailers()` trailer-access surface.
- `:metadata()` dynamic-metadata bridge (likely PARSE-REJECT or partial-deferral per project dynamic-metadata-deferral discipline).
- `:connection()` connection-info bridge (integrates with phase-03 TLS).
- `:httpCall()` outbound HTTP call (reuses phase-20 `internal/httpclient/` at first co-consumer).
- Crypto / base64 / sha helpers + `:fileBytes()` + `:timestamp()` + full `:streamInfo()`.
- Additional stat-surface entries (httpCall counters likely).
- Differential fixture `0027-http-lua-full-bridge` (partial cross-side / REFERENCE-LESS fallback per §6.4).

**Anticipated ADR landings:** Settled at 22.2 BRAINSTORM. Anticipated: full-bridge-API shape ADR + httpCall dispatcher ADR + dynamic-metadata-bridge deferral ADR (or PARSE-REJECT). Anticipated count: ~2-4 NEW ADRs at 22.2 IMPL.

### 11.3 Phase 22.3 surface (multi-script + per-route + ADR-0125 amendment)

**Delivers:** Multi-script + per-route + 9th canonical. Settled in detail at 22.3 BRAINSTORM (next-session-after-22.2-IMPL-phase-done). The anticipated 22.3 surface:
- `Lua.SourceCodes` named-script map (consumed): per-name script compilation + cache; per-name dispatch.
- `LuaPerRoute` 3-arm oneof (consumed): Disabled | Name | SourceCode per the 9th canonical's 3-tier dispatch.
- NEW 9th canonical per-route shape ADR (anchors the 3-arm hybrid + the SHARED stat-discipline + the structural-distinctness rationale).
- ADR-0125 IN-PLACE AMENDMENT (roster 8 → 9; adds the 9th canonical's specification + the lua-row first-use citation).
- Differential fixture `0028-http-lua-multi-script-and-per-route` (cross-side byte-exact for deterministic scenarios).

**Anticipated ADR landings:** Settled at 22.3 BRAINSTORM. Anticipated: NEW 9th canonical ADR + ADR-0125 IN-PLACE AMENDMENT. Anticipated count: ~1-2 NEW ADRs + 1 IN-PLACE AMENDMENT at 22.3 IMPL.

---

## 12. Document conventions + next-skill handoff

**This BRAINSTORM is complete (lifecycle-state 0 → 1).** The next session (lifecycle-state 1 → 2) authors `docs/envoy-go/phases/22-http-filter-lua/SPEC.md` (the parent SPEC) per the phase 18 + phase 19 parent-row precedent. The parent SPEC formalizes the 3-way split surface-mapping + resolves the §9 empirical-pin obligations against reference Envoy v1.37.2 + Envoy v1.32.4 go-control-plane proto bindings + gopher-lua. The per-sub-phase SPEC sessions (22.1 / 22.2 / 22.3) follow the parent SPEC; each sub-phase's SPEC lands at its own dedicated session per the phase 18.1 / 18.2 / 19.1 / 19.2 precedent.

**STATE.md post-this-BRAINSTORM-squash-merge disposition:**
- `active-phase`: `22-http-filter-lua` (parent row in-progress; sub-phases planned)
- `lifecycle-state`: `phase 22 BRAINSTORM done; awaiting parent SPEC`
- `next-skill`: `superpowers:brainstorming` (scoped to parent SPEC authoring per the phase-18 + phase-19 precedent)
- `last-commit`: `<TBD — SHA-fill follow-up after squash-merge>`
- `next-free ADR`: `ADR-0188` (UNCHANGED at BRAINSTORM; first consumed at 22.1 IMPL per §7.1)

**ROADMAP.md post-this-BRAINSTORM disposition:** 4 NEW rows added (parent 22 + 22.1 + 22.2 + 22.3 per §10). Parent 22 status `in-progress`; sub-rows status `planned`. The depends-on chain: 22.1 ← 21; 22.2 ← 22.1; 22.3 ← 22.2.

**Sub-phase directory pre-creation** (per Q12): 4 directories pre-created at this BRAINSTORM with placeholder READMEs:
- `docs/envoy-go/phases/22-http-filter-lua/` (parent — contains this BRAINSTORM.md + the future parent SPEC.md)
- `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/` (sub-phase 22.1 — README.md placeholder)
- `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/` (sub-phase 22.2 — README.md placeholder)
- `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/` (sub-phase 22.3 — README.md placeholder)

Each sub-phase README points back to this parent BRAINSTORM + lists the anticipated artefacts (SPEC.md + PLAN.md + PROGRESS.md + REVIEW.md authored at the sub-phase's own session).

**End of phase 22 parent BRAINSTORM.**
