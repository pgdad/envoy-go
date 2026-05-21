# Phase 22.3 — HTTP filter `envoy.filters.http.lua` (multi-script `Lua.SourceCodes` map + `LuaPerRoute` 3-arm per-route override oneof + NEW 9th canonical per-route shape) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the FINAL envelope-D delta for the lua filter — the `Lua.SourceCodes` named-script registry + the `LuaPerRoute` 3-arm per-route override oneof (`disabled` / `name` / `source_code`) + the NEW 9th canonical per-route classification — on top of the 22.1 pragmatic-middle scaffold + the 22.2 full bridge surface, by: consuming `Lua.SourceCodes` (per-name DataSource resolution + content-hash compile into the existing per-listener `internal/lua.CompileCache` + a `name → *Chunk` registry on `*compiledConfig`); REPLACING the arm-18 `validatePerRouteLua` one-liner with the real 3-arm `LuaPerRoute` validator (PGV-mirror arms + per-route `source_code` DataSource gauntlet, fail-fast at HCM-build via the ADR-0110 `RegisterPerRouteValidator` single-chokepoint); wiring per-route 3-tier dispatch into `decode_headers.go` + `encode_headers.go` (`disabled` → both-hooks-skip; `name` → registry lookup with **upstream-parity silent no-op on a dangling name** per AMEND-22.3-1; `source_code` → inline wholesale-override compiled-with-cache-hit at bind; else listener-level `DefaultSourceCode`; else no-op); adding 1 NEW production file `perroute.go` + 1 NEW test file `perroute_test.go`; adding the `FuzzLuaPerRouteConfig` fuzzer (30 → 31) + extending `FuzzLuaConfigParse`'s corpus with `source_codes` seeds; shipping 1 NEW differential fixture `0028-http-lua-multi-script-and-per-route` (29 → 30) with 5 deterministic cross-side scenarios (a)-(e) + 3 boot-reject scenarios (f)-(h); landing the ADR-0193 §Decision + §Consequences body + the ADR-0125 §(xiv) IN-PLACE AMENDMENT body (canonical roster 8 → 9) + the BEHAVIOR_CONTRACT.md edit bundle (0 net-new envoy-go-strict departure records — all 22.3 dispositions are upstream-parity; the bundle is notes + cross-references); recording the §13-R6 *LState-pool benchmark-gate disposition (anticipated WEAK-default STANDS); and flipping **parent ROADMAP row 22 `in-progress → done`** (22.3 is the final sub-phase) at the same atomic-landing commit. **0 net-new stats** (SHARED-vacuous per the 9th canonical; stat count STAYS 107). **0 net-new framework primitives** (CONSUME + DISPATCH only; reuses 22.1's `internal/lua/` + the existing `internal/filter/http/perroute.go` 3-tier resolution).

**Architecture:** 22.3 is a CONSUME + DISPATCH phase — no new `internal/*` packages, no new bridge methods, no new stats, no new HCM plumbing. It adds 1 NEW production file (`internal/filter/http/lua/perroute.go`) holding the `LuaPerRoute` parse helper + the per-route 3-tier `*Chunk`-resolution dispatch, and EXTENDS 4 existing files: `compiled_config.go` (consume `Lua.SourceCodes` into a `sourceCodes map[string]*internallua.Chunk` registry + the per-route override memo + the `source-codes-key-empty` PARSE-REJECT arm + DROP the arm-4 `source_codes`-deferred reject), `lua.go` (REPLACE `validatePerRouteLua` body with the 3-arm validator + DROP the arm-18 one-liner), `decode_headers.go` (replace the `f.cc.chunk == nil` short-circuit with the per-route resolution; run the RESOLVED chunk), `encode_headers.go` (FIX the guard from `f.cc.chunk == nil` to `f.vm == nil` so a route using a per-route override on a default-less listener still fires `envoy_on_response`). The load-bearing dispatch invariant: the lua filter builds the per-stream `*lua.VM` **once** at `DecodeHeaders` (running the per-route-resolved chunk, which defines BOTH `envoy_on_request` AND `envoy_on_response` globals); `EncodeHeaders` reuses that VM. Per-route resolution therefore happens ONCE at decode (selecting the chunk to run); the encode side is gated only by VM-presence (`f.vm == nil` → no script selected → pass through). This is observably equivalent to upstream's per-phase `getPerLuaCodeSetup()` re-resolution because the matched route does not change mid-stream — the same `PerLuaCodeSetup` is returned for both phases upstream too. **D-P1 RESOLVED (option b', compile-with-cache-hit at bind, proto-pointer-memoized):** the per-route `source_code` inline-override is validated (compiled-then-discarded) at HCM-build via the unchanged `func(proto.Message) error` validator; at per-stream bind, `resolveDecodeScript` consults a NEW `cc.perRouteChunks map[*luav3.LuaPerRoute]*internallua.Chunk` memo (guarded by `cc.perRouteMu sync.Mutex`) — on miss it `resolveDataSource(pr.GetSourceCode()) → internallua.CompileScript(src, cc.compileCache)` then memoizes; because `PerRouteConfig.Resolve` returns a STABLE `*LuaPerRoute` pointer per `routeIdx`, all streams on a route share one memo entry → no per-stream DataSource re-read and no per-stream recompile (a true cache HIT after the first stream; the content-hash `CompileCache` additionally dedups identical-content scripts across routes/names). **D-P2 RESOLVED:** boot-reject scenarios (g) `source_codes[name]` DataSource-failure + (h) per-route `source_code` DataSource-failure are CROSS-SIDE (both proxies fail on a bad DataSource); scenario (f) `source_codes` key-empty is REFERENCE-LESS subject-only (envoy-go-strict-defensive — upstream has no PGV map-key rule and accepts an empty `source_codes` key). **D-P3 RESOLVED:** 6 net-new config-load arm-groups (enumerated in §"D-P3 resolution" below); the fuzzer-surfaced-arm question carries to the IMPL fuzzer Task per the 22.1 Task-11 +2-arm precedent. **R6 escape-valve:** per §13-R6 the per-route resolution is an O(1) `Resolve` + memo/registry lookup (NOT a new per-stream VM construction beyond the existing single decode-time VM), so the IMPL benchmark is anticipated WEAK-default STANDS; conditional ADR-0194 fires only if the benchmark crosses 1ms/stream.

**Tech Stack:** Go 1.26.2; `go-control-plane` v0.13.4 (proto pin per ADR-0008; consumes `envoy/extensions/filters/http/lua/v3.Lua.SourceCodes` `map[string]*core.v3.DataSource` [lua.pb.go:125] + `LuaPerRoute` 3-arm `override` oneof — `LuaPerRoute_Disabled` bool / `LuaPerRoute_Name` string / `LuaPerRoute_SourceCode` `*core.v3.DataSource` [lua.pb.go:198-213] + getters `GetDisabled`/`GetName`/`GetSourceCode`/`GetOverride`; `LuaPerRoute.filter_context` ABSENT in this binding per §2.8 — forward-pointer); `github.com/yuin/gopher-lua v1.1.2` (DIRECT dep from 22.1 per ADR-0188; 22.3 adds NO new gopher-lua API surface — multi-script + per-route are Go-side config/dispatch; each selected script runs in the existing per-stream `*lua.LState` exactly as the default script does per SPEC §11.3); stdlib `sync` (the NEW `cc.perRouteMu` guarding the per-route override memo; the existing `CompileCache.mu` is unchanged); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — unchanged); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext upstream backend fixture (the 22.3 per-route tiers are header-mutation/respond scripts — deterministic on the wire; NO 22.2-surface `:timestamp()`/`:httpCall()` non-determinism). **NO new go.mod direct deps at 22.3.**

---

## Scope check — why phase 22.3 ships as one sub-phase row (ADR-0045 split-gate evaluation: STAY SINGLE-PHASE)

Phase 22 was PRE-SPLIT THREE-way at the parent BRAINSTORM/SPEC commit (`41ccee7`) into `22.1` (DONE `c986419`) + `22.2` (DONE `46183a4`) + `22.3` (THIS PLAN) per ADR-0106 sub-row rollup discipline + parent §1 closure pattern. This PLAN is for the 22.3 sub-phase ONLY; no further nested split per ADR-0106. The PLAN-time re-evaluation of ADR-0045's 25-task / ~1500-LoC split-gate (per BRAINSTORM Q14 deferral + SPEC §3.3 + §1.1 single-phase anticipation):

**Per-component LoC estimate (mirroring the phase-09..22.2 PLAN component-table convention):**

- NEW `internal/filter/http/lua/perroute.go` (`parsePerRouteLua` 3-arm validator helper + `resolveDecodeScript` 3-tier dispatch + the per-route override memo accessor): ~180-280 LoC
- NEW `internal/filter/http/lua/perroute_test.go` (3-arm parse table-driven + 3-tier dispatch + dangling-name silent-no-op + disabled both-hooks-skip + named-script selection + source_code override + memo cache-hit + cross-stream isolation): ~350-550 LoC
- EXTEND `internal/filter/http/lua/compiled_config.go` (`sourceCodes` registry build + per-route override memo fields + `source-codes-key-empty` arm + DROP arm-4 deferral; replace the arm-4 reject with the consume path): ~80-140 LoC delta
- EXTEND `internal/filter/http/lua/compiled_config_test.go` (SourceCodes consume + per-name compile/cache-dedup + key-empty PARSE-REJECT + each-value DataSource gauntlet reuse): ~150-250 LoC delta
- EXTEND `internal/filter/http/lua/lua.go` (REPLACE `validatePerRouteLua` body with `parsePerRouteLua`-delegating validator; DROP the arm-18 one-liner wording): ~20-40 LoC delta
- EXTEND `internal/filter/http/lua/decode_headers.go` (replace the `f.cc.chunk == nil` short-circuit with `chunk, disabled := f.resolveDecodeScript()`; run the resolved chunk): ~25-45 LoC delta
- EXTEND `internal/filter/http/lua/encode_headers.go` (FIX the guard `f.cc.chunk == nil` → drop that clause; gate on `f.vm == nil`): ~3-8 LoC delta
- EXTEND `internal/filter/http/lua/lua_test.go` (filter-struct field assertions for the per-route memo; decode/encode integration for per-route selection; benchmark `BenchmarkPerStream_PerRoute_Resolution` per R6): ~120-220 LoC delta
- EXTEND `internal/filter/http/lua/fuzz_test.go` (+1 `FuzzLuaPerRouteConfig`; `FuzzLuaConfigParse` corpus extended with `source_codes` seeds): ~120-200 LoC delta + ~15-25 corpus seeds
- EXTEND `internal/filter/http/lua/doc.go` (22.3 multi-script + per-route decision summary + AMEND-22.3-1 cross-reference + D-P1/D-P2/D-P3 closures + ADR-0193 + ADR-0125 §(xiv) cross-references): ~40-70 LoC delta
- `cmd/envoy-go/main.go` — UNCHANGED (22.3 extends the SAME lua filter; `RegisterPerRouteValidator` + `Register(lua.TypeURL, lua.New)` calls at the existing alphabetical position STAY UNCHANGED; verify zero delta at the atomic-landing Task)
- `test/differential/fixture/fixture.go` — UNCHANGED (`BackendKind=HTTPLua` from 22.1 STANDS; fixture-0028 reuses it)
- NEW `test/fixtures/0028-http-lua-multi-script-and-per-route/{README.md, envoy.yaml, envoy-go.yaml, expectations.yaml, inputs/driver.go, scripts/*.lua}` (~6-8 scripts; 5 cross-side scenarios + 3 boot-reject): ~900-1400 LoC including driver
- `docs/envoy-go/DECISIONS.md` — ADR-0193 §Decision + §Consequences body + ADR-0125 §(xiv) IN-PLACE AMENDMENT body (canonical roster 8 → 9; the 9th canonical specification + SHARED stat-discipline + 9-shape roster table + lua-row first-use citation): ~300-500 LoC delta
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` — 22.3 edit bundle per §14 (0 net-new departure records; EXTEND `### envoy.filters.http.lua` subsection + 2 upstream-parity NOTES + ADR-0125 §(xiv) cross-reference + convert the `#### Phase 22.2 forward-pointer notes` 22.3-anticipated bullets to landed entries + a NEW `#### Phase 22.3 forward-pointer notes` subsection): ~150-250 LoC delta
- `docs/envoy-go/ROADMAP.md` — row 22.3 flips `in-progress → done`; **parent row 22 flips `in-progress → done`** (final sub-phase); §9 family drops from 4 remaining rows to 3: ~+2 net
- `docs/envoy-go/STATE.md` — rewrite-in-place at the atomic-landing Task
- `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/PROGRESS.md` (NEW; ~7 task entries): ~500-800 LoC
- `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/REVIEW.md` (NEW): ~300-400 LoC

**Production LoC subtotal (excluding docs/tests/fixtures): ~310-520 LoC** (`perroute.go` ~180-280 + `compiled_config.go` delta ~80-140 + `lua.go`/`decode`/`encode` deltas ~50-90). Tests LoC subtotal: ~740-1220. Fixture LoC subtotal: ~900-1400. Doc LoC subtotal: ~1250-1950 (DECISIONS + BEHAVIOR_CONTRACT + PROGRESS + REVIEW). **Total LoC envelope: ~3200-5100.** This sits at the SMALLEST of the three sub-phases (22.1 ~6200-9500; 22.2 ~8500-13500), confirming SPEC §3.3's "~600-900 production-LoC + 1 new production file" anticipation (production+test for the lua package ~1050-1740; the fixture + docs are the bulk).

**Task-count estimate (7 tasks at PLAN time):** Task 0 (PROGRESS preamble + precondition verification) + Tasks 1-6 grouped in 5 tiers (Tier A consume + validate: Task 1 SourceCodes consume, Task 2 per-route validator; Tier B dispatch: Task 3 per-route 3-tier dispatch + decode/encode wiring; Tier C fuzz + bench: Task 4; Tier D differential fixture: Task 5; Tier E atomic landing: Task 6). **7 tasks total — far below ADR-0045's 25-task split-gate.**

**ADR-0045 split-gate evaluation:** Task-count = 7 (under 25). Production LoC ~310-520 (well under 1500). **Decision: STAY SINGLE-PHASE.** No split into 22.3.1 + 22.3.2. Rationale: (i) 22.3 has tight architectural cohesion — multi-script + per-route are ONE coupled surface (per-route `name` delegates INTO `SourceCodes`); (ii) the only plausible split axis (SourceCodes-consume vs per-route-dispatch) is artificial — the per-route `name` arm cannot be tested without the `SourceCodes` registry it references; (iii) it is the smallest sub-phase by a wide margin; (iv) the task graph has parallelization (Task 1 ∥ Task 2 are file-disjoint up to the dispatch join at Task 3). **Phase 22.3 ships as the single sub-phase row it is — no further nested split.** ROADMAP row 22.3 + parent row 22 BOTH flip `in-progress → done` at the Task 6 atomic-landing commit.

---

## D-P resolutions (settled at THIS PLAN session per SPEC §12 + §13)

### D-P1 (per SPEC §3.3 + §11.4 D2 + §13-R-P1): per-route override `*Chunk` storage wiring — RESOLVED

**Option (b'), compile-with-cache-hit at bind, proto-pointer-memoized.** The per-route `source_code` inline-override is (1) VALIDATED at HCM-build by `parsePerRouteLua` exercising `resolveDataSource` + `internallua.CompileScript(src, nil)` (uncached compile-to-validate; the compiled chunk is discarded — fail-fast on bad source/compile), keeping the `func(proto.Message) error` validator signature UNCHANGED; and (2) BOUND at per-stream dispatch via a NEW `cc.perRouteChunks map[*luav3.LuaPerRoute]*internallua.Chunk` memo (guarded by a NEW `cc.perRouteMu sync.Mutex`). On a memo miss `resolveDecodeScript` calls `resolveDataSource(pr.GetSourceCode())` + `internallua.CompileScript(src, cc.compileCache)` then stores the result keyed by the resolved `*LuaPerRoute` pointer. Because `PerRouteConfig.Resolve(filterName, routeIdx)` caches and returns a STABLE `*LuaPerRoute` pointer per `routeIdx` (`perroute.go:140-165`), all streams matching a route share ONE memo entry → no per-stream DataSource re-read (important for the `Filename` arm, which would otherwise re-read the file every stream) and no per-stream recompile. The content-hash `CompileCache` additionally dedups byte-identical override scripts across distinct routes/names to one `*Chunk`. **Rationale for (b') over the SPEC's plain (b):** plain (b) re-resolves the DataSource every stream — fine for inline arms (trivial) but a per-stream file read for `Filename` per-route overrides; the proto-pointer memo makes it first-stream-only at the cost of one small map + mutex on `*compiledConfig`. **Rationale for not (a):** option (a) (validator populates `cc.compileCache` as a side effect) would require threading a `*CompileCache` into the `func(proto.Message) error` validator — changing the ADR-0110 single-chokepoint validator signature, which the SPEC + AMEND-22.3-1 explicitly preserve.

### D-P2 (per SPEC §8.3 + §11.4 D4 + §13-R-P2): fixture-0028 boot-reject roster + cross-side-vs-subject-only — RESOLVED

- **(g) `source_codes[name]` DataSource failure (e.g. `Filename` ENOENT) → CROSS-SIDE `BootRejectFixture`.** Upstream Envoy v1.37.2 reads each `source_codes` entry's `DataSource` at config-load (`FilterConfig` ctor, SPEC §11.2.1) and FAILS the config-load on a bad DataSource — both proxies boot-reject; substring-match both stderr.
- **(h) per-route `source_code` DataSource failure (e.g. compile error) → CROSS-SIDE `BootRejectFixture`.** Upstream `FilterConfigPerRoute` ctor eagerly compiles the per-route `source_code` (SPEC §11.2.2) and FAILS config-load on a bad source — both proxies boot-reject.
- **(f) `source_codes` key-empty → REFERENCE-LESS subject-only.** envoy-go-strict-defensive: the `source-codes-key-empty` arm is envoy-go's own input-bounds discipline (mirrors the 22.1 §6.2-note + the buffer/csrf defensive-mirror precedent). Upstream has NO PGV map-key rule on `source_codes` (confirmed SPEC §11.1: "no PGV map-key rule") and would ACCEPT an empty key (the empty-keyed script simply becomes unreachable, since no `LuaPerRoute.name` can be `""` — PGV `min_len:1`). So envoy-go boot-rejects where upstream does not → subject-only assertion (assert envoy-go exits non-zero + stderr substring; do NOT run the reference proxy for this scenario). Recorded as the existing envoy-go-strict-defensive posture (NOT a new BEHAVIOR_CONTRACT departure record — defensive-mirror arms carry no per-row record per parent §6.2 note).

The dangling-name boot-reject scenario from the BRAINSTORM is DROPPED (per AMEND-22.3-1 — a dangling per-route `name` is an upstream-parity silent no-op, NOT a boot-reject); it is covered by a deterministic cross-side no-op assertion folded adjacent to scenario (b) (a route whose `name` references an absent key produces identical pass-through wire output on both sides).

### D-P3 (per SPEC §6 + 22.1 Task-11 precedent + §13-W3): config-load arm enumeration + fuzzer-surfaced arms — RESOLVED

**6 net-new config-load arm-groups at 22.3** (combined 22.1 19-arm roster − arm 4 lifted + these 6 = the 22.3 surface; arms 3 + 7 DROPPED per BRAINSTORM decision #2 + AMEND-22.3-1):

1. `source-codes-key-empty` — a `SourceCodes` map key is `""`. Wording (provisional; IMPL pins per ADR-0080 + W3): `"lua: source_codes: key must be non-empty"`. (envoy-go-strict-defensive; subject-only fixture coverage per D-P2.)
2. `source-codes-each-value-data-source-resolution` — each `SourceCodes[name]` value runs the EXISTING 22.1 `resolveDataSource` 10-leaf gauntlet (arms 6-15) + `CompileScript` (arm 16), with the per-entry error prefix `"lua: source_codes[%q]: ..."` (mirrors the 22.1 `default_source_code:` arms; the per-entry leaves are REUSED, not net-new wording — only the `source_codes[%q]:` prefix is new).
3. `per-route-override-oneof-required` — `LuaPerRoute.GetOverride() == nil` (PGV-mirror; validate.go:333-342). Wording: `"lua: per-route: override oneof is required"`.
4. `per-route-disabled-must-be-true` — `LuaPerRoute_Disabled` arm with `false` (PGV-mirror; validate.go:253-262). Wording: `"lua: per-route: disabled must be true (PGV const:true violation)"`.
5. `per-route-name-min-1-rune` — `LuaPerRoute_Name` arm with `""` (PGV-mirror; validate.go:277-286). Wording: `"lua: per-route: name length must be at least 1 rune"`.
6. `per-route-source-code-each-arm` — the `LuaPerRoute_SourceCode` arm's inline `*DataSource` runs the SAME `resolveDataSource` gauntlet + `CompileScript`, with the prefix `"lua: per-route: source_code: ..."` (validated fail-fast at HCM-build via the ADR-0110 chokepoint).

**Fuzzer-surfaced arms:** per the 22.1 Task-11 +2-arm precedent (the `stat_prefix`-invalid arm 19 + the `filename-too-large` arm both surfaced from `FuzzLuaConfigParse` first-run), the `FuzzLuaPerRouteConfig` fuzzer at Task 4 MAY surface additional config-load arms (e.g. a pathological per-route `source_code` DataSource path triggering an unhandled panic). The PLAN settles the 6 core arm-groups above; any fuzzer-surfaced additions land at Task 4 with byte-stable wording pinned per ADR-0080 + W3. RATIFIED-PENDING-IMPL at Task 4.

### R6 (per SPEC §13-R6): *LState-pool benchmark gate — anticipated WEAK-default STANDS

The per-route resolution is an O(1) `Resolve` (cached map lookup) + a memo/registry lookup — NOT a new per-stream VM construction (the single decode-time VM is unchanged from 22.2). Task 4 adds `BenchmarkPerStream_PerRoute_Resolution` measuring the per-stream construction + resolution cost at the multi-script + per-route surface; the disposition is recorded at Task 6. If `> 1ms`: conditional ADR-0194 escape-valve consumes for a `*LState`-pool design (next-free ADR-0194). If `< 1ms` (anticipated): WEAK-default carries forward; ADR-0194 stays next-free (parent row 22 closes; no successor sub-phase to carry the buffer).

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/filter/http/lua/perroute.go` | NEW | The 22.3 per-route surface. (1) `parsePerRouteLua(perRoute proto.Message) (*luav3.LuaPerRoute, error)` — type-asserts to `*luav3.LuaPerRoute`; switches on `GetOverride()` for the 3 PGV-mirror arms (arm-group 3 `nil`-oneof, arm-group 4 `disabled==false`, arm-group 5 `name==""`); for the `source_code` arm runs `resolveDataSource(pr.GetSourceCode())` + `internallua.CompileScript(src, nil)` (compile-to-validate, discard) → arm-group 6 fail-fast. Returns the validated `*LuaPerRoute` (or error). (2) `validatePerRouteLua(m proto.Message) error` MOVES here from lua.go (or stays in lua.go delegating to `parsePerRouteLua` — IMPL chooses; the byte-exact wording is the contract). (3) `(f *filter) resolveDecodeScript() (chunk *internallua.Chunk, disabled bool)` — the 3-tier dispatch: resolve per-route via `f.dcb.RequestRouteConfig()`; nil → listener default `f.cc.chunk` (tier 2/3); non-nil → `parsePerRouteLua` → `disabled` arm returns `(nil, true)`; `name` arm returns `(cc.sourceCodes[name], false)` — a miss returns `(nil, false)` = upstream-parity silent no-op per AMEND-22.3-1; `source_code` arm returns the memoized override chunk via the `cc.perRouteChunks` memo (D-P1 b'). nil-tolerant on `f.dcb`. ~180-280 LoC. Lands at Tasks 2-3. |
| `internal/filter/http/lua/perroute_test.go` | NEW | `parsePerRouteLua` 3-arm table-driven (each PGV-mirror arm byte-exact wording + the `source_code` DataSource gauntlet reuse: ENOENT / compile-error / empty / WatchedDirectory-reject); `resolveDecodeScript` 3-tier dispatch (default-when-no-perroute; named-script selection; `source_code` override; `disabled` → `(nil,true)`; **dangling-name → `(nil,false)` silent no-op**); memo cache-hit (two streams on the same route share one `*Chunk` pointer — assert pointer-equality + that the file is read once via a read-counting `t.TempDir()` fixture or a hash-stable inline arm); cross-stream isolation (N parallel filters on distinct routes resolve independently); nil-`dcb` tolerance. ~350-550 LoC. Lands at Tasks 2-3. |
| `internal/filter/http/lua/compiled_config.go` | MODIFY | (1) NEW `*compiledConfig` fields: `sourceCodes map[string]*internallua.Chunk` (the `name → *Chunk` registry; nil when the proto has no `source_codes`) + `perRouteChunks map[*luav3.LuaPerRoute]*internallua.Chunk` (the D-P1 b' per-route override memo; lazily allocated) + `perRouteMu sync.Mutex` (guards `perRouteChunks`). (2) REPLACE the arm-4 `if len(m.GetSourceCodes()) > 0 { return ...parseRejectSourceCodesDeferred }` reject with the consume path: iterate `m.GetSourceCodes()` in sorted-key order (deterministic compile order), reject empty keys (arm-group 1 `source-codes-key-empty`), `resolveDataSource` each value (arm-group 2; prefix `source_codes[%q]:`), `CompileScript(src, compileCache)` each into the SHARED cache, populate `sourceCodes[name]`. (3) RETIRE the `parseRejectSourceCodesDeferred` const (the arm is lifted) — keep it as a reserved-verbatim const ONLY if a regression test still references it; otherwise delete + sweep the test. NEW consts `parseRejectSourceCodesKeyEmpty` + `wrapParseRejectSourceCodesValueFmt`. ~80-140 LoC delta. Lands at Task 1. |
| `internal/filter/http/lua/compiled_config_test.go` | MODIFY | NEW rows: `source_codes` single-entry consume (registry populated; chunk compiles); multi-entry consume (2 entries → 2 registry chunks); identical-content entries under 2 names dedup to ONE `*Chunk` pointer (content-hash cache); key-empty PARSE-REJECT byte-exact (`source-codes-key-empty`); each-value DataSource gauntlet reuse (ENOENT / compile-error / WatchedDirectory-reject with the `source_codes[%q]:` prefix); the arm-4-deferred test row is DELETED (arm lifted). ~150-250 LoC delta. Lands at Task 1. |
| `internal/filter/http/lua/lua.go` | MODIFY | REPLACE the `validatePerRouteLua` one-liner body (`return errors.New("lua: per-route configuration is not yet supported (lands in phase 22.3)")`) with a body that delegates to `parsePerRouteLua` (`_, err := parsePerRouteLua(m); return err`). The `RegisterPerRouteValidator` wiring is UNCHANGED (the chokepoint registration call in cmd/envoy-go/main.go stays). Update the doc-comment (drop the "22.3 IMPL replaces" forward-pointer; now landed). ~20-40 LoC delta. Lands at Task 2. |
| `internal/filter/http/lua/decode_headers.go` | MODIFY | REPLACE Step 1 (`if f.cc == nil \|\| f.cc.chunk == nil { return Continue }`) with: keep `if f.cc == nil { return Continue }`, then `chunk, disabled := f.resolveDecodeScript(); if disabled \|\| chunk == nil { return Continue }`. Replace the `f.vm.Run(f.cc.chunk)` call (Step 5) with `f.vm.Run(chunk)` — run the per-route-RESOLVED chunk (default / named / override). The disabled + nil-chunk early-returns happen BEFORE VM construction (Step 2) — matching upstream's `nullptr` + the buffer 5th-canonical passthrough (no VM built → encode-side `f.vm == nil` guard naturally skips `envoy_on_response` too). ~25-45 LoC delta. Lands at Task 3. |
| `internal/filter/http/lua/encode_headers.go` | MODIFY | FIX the Step 1 guard: change `if f.cc == nil \|\| f.cc.chunk == nil \|\| f.vm == nil` to `if f.cc == nil \|\| f.vm == nil`. The `f.cc.chunk == nil` clause is a 22.1/22.2 default-chunk-only artifact; with per-route overrides a route may select a chunk on a default-less listener (`f.cc.chunk == nil` but `f.vm != nil` because decode built+ran the override). `f.vm != nil` is the correct gate (decode selected + ran a script → encode tries `envoy_on_response`). ~3-8 LoC delta. Lands at Task 3. |
| `internal/filter/http/lua/lua_test.go` | MODIFY | NEW: per-route memo field assertions on the `*compiledConfig` (memo nil before first per-route bind; populated after); decode→encode integration for per-route selection (a route with `source_code` override defining `envoy_on_response` fires it at encode despite `f.cc.chunk == nil` — regression-pins the encode-guard fix); `BenchmarkPerStream_PerRoute_Resolution` per R6 (measures per-stream VM construction + per-route resolution at the multi-script + per-route surface; threshold gate `ns/op > 1_000_000` → signal Task 6 ADR-0194 escape-valve). ~120-220 LoC delta. Lands at Task 3 (integration) + Task 4 (benchmark). |
| `internal/filter/http/lua/fuzz_test.go` | MODIFY | NEW `FuzzLuaPerRouteConfig` (fuzzes `*LuaPerRoute` proto bytes through `parsePerRouteLua`; must-never-panic per ADR-0018; corpus: each PGV-mirror arm + valid disabled/name/source_code + adversarial DataSource paths; ~10-15 seeds). EXTEND `FuzzLuaConfigParse` corpus with `source_codes` map seeds (single-entry / multi-entry / empty-key / bad-value; ~5-10 seeds). Surface + pin any additional config-load arms per D-P3. Project fuzzer count 30 → 31. ~120-200 LoC delta + corpus. Lands at Task 4. |
| `internal/filter/http/lua/doc.go` | MODIFY | EXTEND with the 22.3 multi-script + per-route surface summary: `SourceCodes` registry + `LuaPerRoute` 3-arm + 3-tier dispatch + the 9th canonical + AMEND-22.3-1 (dangling-name silent no-op) + D-P1/D-P2/D-P3 closures + ADR-0193 + ADR-0125 §(xiv) cross-references. ~40-70 LoC delta. Lands at Task 6 (atomic landing). |
| `cmd/envoy-go/main.go` | UNCHANGED | 22.3 extends the SAME lua filter; the `RegisterPerRouteValidator` + `Register(lua.TypeURL, lua.New)` calls STAY at the existing alphabetical position. Verify zero delta at Task 6. |
| `test/differential/fixture/fixture.go` | UNCHANGED | `BackendKind=HTTPLua` from 22.1 STANDS; fixture-0028 reuses it. |
| `test/fixtures/0028-http-lua-multi-script-and-per-route/README.md` | NEW | Scope + 5+3 scenario table + topology + cross-refs to SPEC §8 + ADR-0193 + ADR-0125 §(xiv). ~120-200 LoC. Lands at Task 5. |
| `test/fixtures/0028-http-lua-multi-script-and-per-route/envoy.yaml` | NEW | Reference Envoy bootstrap: 1 listener; `Lua` filter with `source_codes` (2 named scripts) + `default_source_code`; route table with per-route `LuaPerRoute` typed_per_filter_config (name-delegation / source_code-override / disabled / dangling-name routes). Templated `{{.BackendPort}}`. ~180-280 LoC. Lands at Task 5. |
| `test/fixtures/0028-http-lua-multi-script-and-per-route/envoy-go.yaml` | NEW | Subject bootstrap; same topology; templated `{{.AdminPort}} {{.ListenerPort}} {{.BackendPort}} {{.FixtureDir}}`. ~180-280 LoC. Lands at Task 5. |
| `test/fixtures/0028-http-lua-multi-script-and-per-route/expectations.yaml` | NEW | Human-readable scenario expectations (NOT consumed by runner). ~80-150 LoC. Lands at Task 5. |
| `test/fixtures/0028-http-lua-multi-script-and-per-route/inputs/driver.go` | NEW | Registered `Driver` impl + `BootRejectFixture` impl(s) for scenarios (f)-(h); per-scenario probes (a)-(e) via the existing `driveProxy`/`emitScenario` pattern; scenario (b) covers both a valid `name`-delegation route AND a dangling-name no-op route. ~350-550 LoC. Lands at Task 5. |
| `test/fixtures/0028-http-lua-multi-script-and-per-route/scripts/*.lua` | NEW | ~6-8 deterministic header-mutation scripts: `default.lua` (listener default), `named_a.lua` + `named_b.lua` (the 2 `source_codes` entries), `override.lua` (per-route source_code), + a compile-error script for boot-reject (h), + a bad-datasource reference for (g). Each ~3-8 LoC. Lands at Task 5. |
| `docs/envoy-go/DECISIONS.md` | MODIFY | ADR-0193 §Decision + §Consequences body (NEW combined multi-script + per-route package-shape extension; per SPEC §3.2 + §4 + §5 + §6 + §11 + AMEND-22.3-1 + D-P1/D-P2/D-P3) + ADR-0125 §(xiv) IN-PLACE AMENDMENT body (canonical roster 8 → 9; the 9th canonical 3-arm-hybrid specification + SHARED stat-discipline + 9-shape roster table + lua-row first-use citation; NO new ADR number). The ADR-0193 §Context draft anchored at the SPEC commit is EXTENDED in place (not rewritten). ~300-500 LoC delta. Lands at Task 6. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY | 22.3 edit bundle per SPEC §14 (0 net-new departure records): EXTEND `### envoy.filters.http.lua` subsection with the multi-script + per-route surface; 2 upstream-parity NOTES (dangling-name silent no-op per AMEND-22.3-1; no reserved-name discipline); ADR-0125 §(xiv) cross-reference; convert the `#### Phase 22.2 forward-pointer notes` 22.3-anticipated bullets to landed entries; NEW `#### Phase 22.3 forward-pointer notes` subsection (`:metadata()` per-route source + `clear_route_cache` + `filter_context` binding-gap forward-pointers; parent-row-22 closure). Departure-record count UNCHANGED at 14. ~150-250 LoC delta. Lands at Task 6. |
| `docs/envoy-go/ROADMAP.md` | MODIFY | Row 22.3 flips `in-progress → done`; **parent row 22 flips `in-progress → done`** (final sub-phase; per-cell IMPL-done annotation per ADR-0106); §9 HTTP-filters family closes from 4 remaining rows to 3 (`wasm`, `admission_control`, `global rate limit`). ~+2 net. Lands at Task 6. |
| `docs/envoy-go/STATE.md` | MODIFY | Rewrite-in-place at Task 6: `lifecycle-state: phase 22.3 IMPL done; phase 22 (parent) done; awaiting next-phase BRAINSTORM`; stat count STAYS 107; 17 HTTP filters UNCHANGED; 30 fuzzers; fuzzer count 30 → 31; 29 → 30 fixture directories; ADR tail at ADR-0193 full body (+ ADR-0125 §(xiv) amended); next-free `ADR-0194` (if R6 stands) or `ADR-0195` (if escape-valve fires); SHA-fill follow-up commit per the phase-09..22.2 convention. |
| `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/PROGRESS.md` | NEW | Append-only task log per the phase-22.1/22.2 IMPL precedent + `superpowers:verification-before-completion`; 7 task entries; each quotes command outputs verbatim. ~500-800 LoC. |
| `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/REVIEW.md` | NEW | Task 6 reviewer artifact per `superpowers:requesting-code-review`; per-task review + the SPEC §15 ~16-item acceptance-checklist closure. ~300-400 LoC. |

---

## Task 0: PROGRESS.md preamble + precondition verification

**Files:**
- Create: `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/PROGRESS.md`

- [ ] **Step 1: Verify the worktree base is green (build/vet/lint)**

Run from the 22.3 PLAN IMPL worktree root:
```bash
go build ./... && go vet ./... && golangci-lint run
```
Expected: clean (exit 0). Record the output in PROGRESS.md.

- [ ] **Step 2: Verify the lua package + per-route framework tests pass**

```bash
go test ./internal/filter/http/lua/... ./internal/lua/... ./internal/filter/http/ -count=1
```
Expected: PASS. Record.

- [ ] **Step 3: Verify the fuzzer baseline (30) + fixture baseline (29)**

```bash
grep -rh '^func Fuzz' --include='*.go' . | sort -u | wc -l   # expect 30
ls -d test/fixtures/00*/ | wc -l                              # expect 29
```
Record both counts in PROGRESS.md as the pre-22.3 baseline.

- [ ] **Step 4: Write the PROGRESS.md preamble**

Header: phase 22.3 IMPL; base commit (worktree base SHA); the SPEC §15 acceptance-checklist (~16 items) reproduced as the closure target; the 7-task graph; the D-P1/D-P2/D-P3 + R6 dispositions from this PLAN. Commit:
```bash
git add docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/PROGRESS.md
git commit -m "phase 22.3 IMPL Task 0: PROGRESS.md preamble + precondition verification"
```

---

## Task 1: Consume `Lua.SourceCodes` — named-script registry + key-empty arm (Tier A)

**Files:**
- Modify: `internal/filter/http/lua/compiled_config.go`
- Modify: `internal/filter/http/lua/compiled_config_test.go`

- [ ] **Step 1: Write the failing tests**

In `compiled_config_test.go`, add table rows + dedicated tests:
- `source_codes` single-entry (`InlineString` arm) → `cc.sourceCodes["a"]` non-nil, compiles.
- 2 entries → 2 distinct registry chunks.
- 2 entries with byte-identical content under 2 names → `cc.sourceCodes["x"] == cc.sourceCodes["y"]` (same `*Chunk` pointer; content-hash dedup).
- key-empty (`source_codes[""]`) → PARSE-REJECT byte-exact `"lua: source_codes: key must be non-empty"`.
- bad value (`Filename` ENOENT) → PARSE-REJECT with prefix `"lua: source_codes[\"a\"]: "`.
- DELETE the existing arm-4 `parseRejectSourceCodesDeferred` test row.

- [ ] **Step 2: Run to verify the new tests fail**

```bash
go test ./internal/filter/http/lua/ -run 'TestBuildCompiledConfig.*SourceCodes' -count=1
```
Expected: FAIL (registry field + consume path not yet implemented; arm-4 still rejects).

- [ ] **Step 3: Implement the consume path**

In `compiled_config.go`: add the `sourceCodes`/`perRouteChunks`/`perRouteMu` fields to `compiledConfig`; add consts `parseRejectSourceCodesKeyEmpty = "lua: source_codes: key must be non-empty"` + `wrapParseRejectSourceCodesValueFmt = "lua: source_codes[%q]: %w"`; REPLACE the arm-4 reject block with: iterate `m.GetSourceCodes()` in sorted-key order, reject `""` keys, `resolveDataSource` + `CompileScript(src, compileCache)` each value (wrap errors via the `source_codes[%q]:` prefix), populate `cc.sourceCodes`. Retire `parseRejectSourceCodesDeferred` (delete the const + sweep any reference; if a fuzzer corpus seed referenced the arm-4 wording, replace it with a `source_codes`-consume seed at Task 4).

- [ ] **Step 4: Run to verify tests pass + full package green**

```bash
go test ./internal/filter/http/lua/ -count=1 && go vet ./internal/filter/http/lua/ && golangci-lint run ./internal/filter/http/lua/
```
Expected: PASS + clean.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/lua/compiled_config.go internal/filter/http/lua/compiled_config_test.go
git commit -m "phase 22.3 IMPL Task 1: consume Lua.SourceCodes into name→*Chunk registry + key-empty PARSE-REJECT"
```

---

## Task 2: 3-arm `LuaPerRoute` validator (Tier A)

**Files:**
- Create: `internal/filter/http/lua/perroute.go`
- Create: `internal/filter/http/lua/perroute_test.go`
- Modify: `internal/filter/http/lua/lua.go`
- Modify: `internal/filter/http/lua/compiled_config.go` (retire `parseRejectPerRouteDeferred`)
- Modify: `internal/filter/http/lua/lua_test.go` (sweep the `validatePerRouteLua`-returns-deferred assertion)
- Modify: `internal/filter/http/lua/compiled_config_test.go` (sweep the `Arm04`/`Arm18` const-table row)

> **Deferred-const test sweep (do NOT skip):** retiring `parseRejectSourceCodesDeferred` (Task 1) + `parseRejectPerRouteDeferred` (Task 2) breaks existing tests that pin those wordings — they will fail to compile/pass until swept. Known pin sites at PLAN-authoring time: `compiled_config_test.go` arm-4 rows + the `Arm04`/`Arm18` byte-exact const-table test; `lua_test.go` the `validatePerRouteLua` deferred-assertion test. Replace the arm-4/arm-18 deferred assertions with the new consume/validate behavior; do NOT merely delete the rows if doing so drops byte-exact-wording coverage — re-point them at the new arm wordings.

- [ ] **Step 1: Write the failing tests**

In `perroute_test.go`, table-driven `parsePerRouteLua` tests:
- `disabled: true` → no error.
- `disabled: false` → byte-exact `"lua: per-route: disabled must be true (PGV const:true violation)"`.
- `name: "a"` → no error.
- `name: ""` → byte-exact `"lua: per-route: name length must be at least 1 rune"`.
- `source_code` valid `InlineString` → no error (compiles-to-validate).
- `source_code` ENOENT `Filename` → error with prefix `"lua: per-route: source_code: "`.
- `source_code` compile-error → error with prefix `"lua: per-route: source_code: "`.
- `source_code` WatchedDirectory → reject (gauntlet reuse).
- nil-oneof (`&LuaPerRoute{}`) → byte-exact `"lua: per-route: override oneof is required"`.
- wrong type (non-`*LuaPerRoute` proto.Message) → type-assert error.

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/filter/http/lua/ -run TestParsePerRouteLua -count=1
```
Expected: FAIL (`parsePerRouteLua` undefined).

- [ ] **Step 3: Implement `parsePerRouteLua` + delegate the validator**

In `perroute.go`: add `parsePerRouteLua(perRoute proto.Message) (*luav3.LuaPerRoute, error)` with the 4 arm-groups (oneof-required / disabled-must-be-true / name-min-1 / source_code-gauntlet via `resolveDataSource` + `CompileScript(src, nil)`). Add the 4 byte-exact wording consts (per D-P3). In `lua.go`: REPLACE the `validatePerRouteLua` body with `func validatePerRouteLua(m proto.Message) error { _, err := parsePerRouteLua(m); return err }`; update the doc-comment (drop the arm-18 forward-pointer; now landed). Remove the now-unused `parseRejectPerRouteDeferred` const from `compiled_config.go` (or keep reserved-verbatim if a test pins it — sweep accordingly).

- [ ] **Step 4: Run to verify pass + green**

```bash
go test ./internal/filter/http/lua/ -count=1 && golangci-lint run ./internal/filter/http/lua/
```
Expected: PASS + clean.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/lua/perroute.go internal/filter/http/lua/perroute_test.go internal/filter/http/lua/lua.go internal/filter/http/lua/compiled_config.go
git commit -m "phase 22.3 IMPL Task 2: replace arm-18 one-liner with real 3-arm LuaPerRoute validator"
```

---

## Task 3: Per-route 3-tier dispatch + decode/encode wiring (Tier B)

**Files:**
- Modify: `internal/filter/http/lua/perroute.go` (add `resolveDecodeScript`)
- Modify: `internal/filter/http/lua/decode_headers.go`
- Modify: `internal/filter/http/lua/encode_headers.go`
- Modify: `internal/filter/http/lua/perroute_test.go`
- Modify: `internal/filter/http/lua/lua_test.go`

- [ ] **Step 1: Write the failing tests**

In `perroute_test.go`, `resolveDecodeScript` tests via a test-double `DecoderFilterCallbacks` whose `RequestRouteConfig()` returns canned `*LuaPerRoute` values:
- no per-route (nil) → returns `(cc.chunk, false)` (listener default).
- `name: "named_a"` present in registry → `(cc.sourceCodes["named_a"], false)`.
- **`name: "ghost"` absent from registry → `(nil, false)` (dangling-name silent no-op per AMEND-22.3-1).**
- `source_code` override → `(<compiled override chunk>, false)`; second call on the SAME `*LuaPerRoute` pointer returns the SAME `*Chunk` pointer (memo hit; assert the `Filename` source is read only once via a read-counting temp file).
- `disabled: true` → `(nil, true)`.
- no per-route AND `cc.chunk == nil` → `(nil, false)`.

In `lua_test.go`: decode→encode integration — a route with a `source_code` override that defines `envoy_on_response` (and `cc.chunk == nil` listener) fires `envoy_on_response` at `EncodeHeaders` (regression-pins the encode-guard fix); a `disabled` route builds NO VM and skips BOTH hooks.

- [ ] **Step 2: Run to verify they fail**

```bash
go test ./internal/filter/http/lua/ -run 'TestResolveDecodeScript|TestPerRoute.*Encode|TestPerRoute.*Disabled' -count=1
```
Expected: FAIL.

- [ ] **Step 3: Implement the dispatch + wiring**

In `perroute.go`: add `(f *filter) resolveDecodeScript() (*internallua.Chunk, bool)` — the 3-tier dispatch (default / name→registry-with-silent-no-op-on-miss / source_code→memo) using `f.dcb.RequestRouteConfig()` + the `cc.perRouteChunks`/`cc.perRouteMu` memo (D-P1 b'); nil-tolerant on `f.dcb`. In `decode_headers.go`: replace the Step-1 short-circuit with `if f.cc == nil { return Continue }; chunk, disabled := f.resolveDecodeScript(); if disabled || chunk == nil { return Continue }`; replace `f.vm.Run(f.cc.chunk)` with `f.vm.Run(chunk)`. In `encode_headers.go`: change the guard `f.cc == nil || f.cc.chunk == nil || f.vm == nil` → `f.cc == nil || f.vm == nil`.

- [ ] **Step 4: Run the full lua package + per-route framework + race**

```bash
go test ./internal/filter/http/lua/ ./internal/filter/http/ -count=1
go test -race ./internal/filter/http/lua/ -count=1
golangci-lint run ./internal/filter/http/lua/
```
Expected: PASS + clean + race-free.

- [ ] **Step 5: Commit**

```bash
git add internal/filter/http/lua/perroute.go internal/filter/http/lua/perroute_test.go internal/filter/http/lua/decode_headers.go internal/filter/http/lua/encode_headers.go internal/filter/http/lua/lua_test.go
git commit -m "phase 22.3 IMPL Task 3: per-route 3-tier dispatch + decode/encode wiring (dangling-name silent no-op; encode-guard fix)"
```

---

## Task 4: `FuzzLuaPerRouteConfig` + corpus extension + R6 benchmark (Tier C)

**Files:**
- Modify: `internal/filter/http/lua/fuzz_test.go`
- Modify: `internal/filter/http/lua/lua_test.go` (benchmark)
- Create: `internal/filter/http/lua/testdata/fuzz/FuzzLuaPerRouteConfig/` (corpus seeds)

- [ ] **Step 1: Write the fuzzer + benchmark**

`FuzzLuaPerRouteConfig`: marshal fuzzed bytes into `*luav3.LuaPerRoute`, run through `parsePerRouteLua` — must never panic (ADR-0018). Seeds: each PGV-mirror arm + valid disabled/name/source_code + adversarial DataSource. EXTEND `FuzzLuaConfigParse` corpus with `source_codes` map seeds. Add `BenchmarkPerStream_PerRoute_Resolution` to `lua_test.go` (per-stream VM construct + per-route resolve at the multi-script + per-route surface).

- [ ] **Step 2: Run the fuzzers briefly + the benchmark**

```bash
go test ./internal/filter/http/lua/ -run 'FuzzLuaPerRouteConfig|FuzzLuaConfigParse' -count=1
go test ./internal/filter/http/lua/ -fuzz FuzzLuaPerRouteConfig -fuzztime 30s
go test ./internal/filter/http/lua/ -bench BenchmarkPerStream_PerRoute_Resolution -run '^$' -benchmem
```
Expected: seed corpus PASS; 30s fuzz no new crashers; benchmark prints `ns/op`. If the fuzzer surfaces a config-load arm/panic, ADD the byte-stable PARSE-REJECT arm (per D-P3) + a regression test + re-run. Record the `ns/op` for the R6 disposition (anticipated `< 1ms`).

- [ ] **Step 3: Verify the project fuzzer count is 31**

```bash
grep -rh '^func Fuzz' --include='*.go' . | sort -u | wc -l   # expect 31
```

- [ ] **Step 4: Commit**

```bash
git add internal/filter/http/lua/fuzz_test.go internal/filter/http/lua/lua_test.go internal/filter/http/lua/testdata/fuzz/
git commit -m "phase 22.3 IMPL Task 4: FuzzLuaPerRouteConfig (30→31) + source_codes corpus + R6 benchmark"
```

---

## Task 5: Differential fixture-0028 (Tier D)

**Files:**
- Create: `test/fixtures/0028-http-lua-multi-script-and-per-route/{README.md, envoy.yaml, envoy-go.yaml, expectations.yaml, inputs/driver.go}`
- Create: `test/fixtures/0028-http-lua-multi-script-and-per-route/scripts/*.lua`

- [ ] **Step 1: Author the fixture + scripts + driver**

5 deterministic cross-side scenarios (a) listener-default, (b) per-route `name`→named (+ a dangling-name no-op sub-route), (c) per-route `source_code` override, (d) per-route `disabled`, (e) multi-script selection (two routes → two named scripts) — all `CompareBytes`. 3 boot-reject scenarios (f) key-empty (subject-only per D-P2), (g) `source_codes[name]` DataSource-failure (cross-side), (h) per-route `source_code` DataSource-failure (cross-side) via `BootRejectFixture`. Deterministic header-mutation scripts only (no `:timestamp()`/`:httpCall()`).

- [ ] **Step 2: Run the differential fixture (both proxies via Docker)**

```bash
go test ./test/differential/ -run 'Fixture.*0028' -count=1
```
Expected: all 5 cross-side scenarios byte-exact GREEN; all 3 boot-reject scenarios assert correctly (f subject-only; g+h cross-side). Iterate on YAML/scripts until green; quote the run output in PROGRESS.md.

- [ ] **Step 3: Verify the fixture-directory count is 30**

```bash
ls -d test/fixtures/00*/ | wc -l   # expect 30
```

- [ ] **Step 4: Commit**

```bash
git add test/fixtures/0028-http-lua-multi-script-and-per-route/
git commit -m "phase 22.3 IMPL Task 5: differential fixture-0028 (5 cross-side + 3 boot-reject scenarios; 29→30)"
```

---

## Task 6: Atomic landing — ADR bodies + BEHAVIOR_CONTRACT + docs + STATE/ROADMAP + parent-row-22 closure + REVIEW (Tier E)

**Files:**
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0193 §Decision+§Consequences; ADR-0125 §(xiv) AMENDMENT body)
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md`
- Modify: `internal/filter/http/lua/doc.go`
- Modify: `docs/envoy-go/ROADMAP.md`, `docs/envoy-go/STATE.md`
- Modify: `docs/envoy-go/phases/22.3-.../PROGRESS.md`
- Create: `docs/envoy-go/phases/22.3-.../REVIEW.md`

- [ ] **Step 1: Land the ADR bodies**

Extend the ADR-0193 §Context draft in place with §Decision + §Consequences (per SPEC §16.1 + §3.2 + AMEND-22.3-1 + D-P1/D-P2/D-P3 + R6 disposition). Land the ADR-0125 §(xiv) IN-PLACE AMENDMENT body (canonical roster 8 → 9; the 9th canonical 3-arm-hybrid specification + SHARED stat-discipline + the 9-shape roster table + the lua-row first-use citation). NO new ADR number consumed (ADR-0194 stays next-free unless R6 fired at Task 4 — if it did, also land ADR-0194 here per §15 item 9).

- [ ] **Step 2: Land the BEHAVIOR_CONTRACT.md edit bundle + doc.go**

EXTEND `### envoy.filters.http.lua` with the multi-script + per-route surface; add the 2 upstream-parity NOTES (dangling-name silent no-op; no reserved-name) + the ADR-0125 §(xiv) cross-reference; convert the `#### Phase 22.2 forward-pointer notes` 22.3-bullets to landed entries; add `#### Phase 22.3 forward-pointer notes`. Departure-record count UNCHANGED at 14. Extend `doc.go`.

- [ ] **Step 3: Update STATE.md + ROADMAP.md (parent-row-22 closure)**

ROADMAP: row 22.3 `in-progress → done`; **parent row 22 `in-progress → done`** (§9 family 4 → 3 remaining). STATE: lifecycle-state `phase 22.3 IMPL done; phase 22 parent done`; next-skill `superpowers:brainstorming` (next phase); fuzzer 30 → 31; fixtures 29 → 30; ADR tail ADR-0193 full body; next-free `ADR-0194` (or `ADR-0195` if R6 fired); stat count STAYS 107; 17 HTTP filters UNCHANGED.

- [ ] **Step 4: Author REVIEW.md + finalize PROGRESS.md**

REVIEW.md per `superpowers:requesting-code-review`: per-task review + the SPEC §15 ~16-item acceptance-checklist closure (with command-output evidence). Finalize PROGRESS.md Task 6 entry.

- [ ] **Step 5: Full-suite green gate (per `superpowers:verification-before-completion`)**

```bash
go build ./... && go vet ./... && golangci-lint run
go test ./... -count=1
go test -race ./internal/filter/http/lua/ -count=1
go test ./test/differential/ -run 'Fixture.*0028' -count=1
# h2spec unchanged at ADR-0051 pin (no HTTP-core change at 22.3) — spot-check per project convention
git diff --stat HEAD~6   # confirm cmd/envoy-go/main.go + fixture.go ZERO delta
```
Expected: all green; main.go + fixture.go zero delta. Quote outputs in PROGRESS.md + REVIEW.md.

- [ ] **Step 6: Commit the atomic landing**

```bash
git add docs/ internal/filter/http/lua/doc.go
git commit -m "phase 22.3 IMPL Task 6: ADR-0193 + ADR-0125 §(xiv) bodies + BEHAVIOR_CONTRACT + STATE/ROADMAP parent-row-22 closure + REVIEW"
```

---

## Post-IMPL integration (per project memory `feedback_git_worktrees.md` + ADR-0005 + `feedback_push_to_origin.md`)

After Task 6 green: squash-merge the IMPL branch to master per the phase-09..22.2 convention; SHA-fill follow-up commit backfilling the STATE.md `last-commit` placeholder (`TBD → <squash-SHA> post-squash`); push to origin. (This is the IMPL-session integration; THIS PLAN session itself only authors PLAN.md + the STATE.md PLAN-done transition + README status-line, which squash-merges separately per the SPEC→PLAN→IMPL session split.)

---

## Acceptance criteria (SPEC §15 ~16-item checklist — the IMPL closure target)

1. CONSUME `Lua.SourceCodes` (per-name DataSource resolve + content-hash compile into the shared `CompileCache` + `name → *Chunk` registry) — Task 1.
2. CONSUME `LuaPerRoute` 3-arm oneof — REPLACE `validatePerRouteLua` with the real validator — Task 2.
3. Per-route 3-tier dispatch (`disabled` both-hooks-skip; `name` registry lookup with silent no-op on miss; `source_code` override; fall through to default; else no-op) — Task 3.
4. NEW `perroute.go` + `perroute_test.go` — Tasks 2-3.
5. Config-load PARSE-REJECT arms (6 arm-groups per D-P3; arms 3 + 7 NOT present) — Tasks 1-2.
6. 0 net-new stats (SHARED-vacuous); stat count STAYS 107 — verified Task 6.
7. ADR-0193 §Decision + §Consequences body landed — Task 6.
8. ADR-0125 §(xiv) IN-PLACE AMENDMENT body landed (roster 8 → 9; no new ADR number) — Task 6.
9. CONDITIONAL ADR-0194 — ONLY if the R6 benchmark gate fires (`> 1ms`) — Task 4 measure / Task 6 land.
10. NEW `FuzzLuaPerRouteConfig` + `FuzzLuaConfigParse` corpus extension; project count 30 → 31 — Task 4.
11. Differential fixture-0028 GREEN (5 cross-side + 3 boot-reject); 29 → 30 — Task 5.
12. BEHAVIOR_CONTRACT.md edit bundle (0 new departure records; departure count UNCHANGED at 14) — Task 6.
13. R6 *LState-pool gate disposition recorded (anticipated WEAK-default STANDS) — Task 4 / Task 6.
14. **Parent row 22 flips `in-progress → done`** (final sub-phase) + STATE.md re-advance + ROADMAP row 22.3 flip — Task 6.
15. Per-task PROGRESS.md entries quoting command outputs per `superpowers:verification-before-completion` — Tasks 0-6.
16. REVIEW.md authored at phase-done per `superpowers:requesting-code-review` — Task 6.
