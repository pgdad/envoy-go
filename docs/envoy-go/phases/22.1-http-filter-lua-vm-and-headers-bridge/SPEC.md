# Phase 22.1 SPEC — `envoy.filters.http.lua` (filter scaffold + VM primitive + headers bridge + DefaultSourceCode)

> **Lifecycle state:** SPEC.md authored; ROADMAP row `22.1` flips `planned → in-progress` at this SPEC commit (parent row `22` stays `in-progress` per ADR-0106 per-cell SPEC-done annotation; sub-rows `22.2` + `22.3` stay `planned`) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase-09..21 + phase-18.1 + phase-19.1 precedent. This SPEC is the authoritative input to the 22.1 PLAN.

**Parent:** `docs/envoy-go/phases/22-http-filter-lua/SPEC.md` (the parent master SPEC — carries the cross-cutting design §4 framework primitive, the **full §11 7-pin empirical-pin block** resolved IN-SESSION at the parent SPEC session via parallel-subagent fan-out against v1.37.2 reference Envoy + v1.32.4 go-control-plane proto bindings + `github.com/yuin/gopher-lua`, the §1.1 12-AMEND catalog covering 3 substantive REFUTATIONS + 9 CONFIRMS/EXTENDS findings, the §6 PARSE-REJECT roster (18 arms at 22.1; ~20 forward-pointer arms at 22.3), the §7 stat surface (3 counters under HCM-rooted template), the §8 fixture-0026 disposition (6 wire-interactive scenarios cross-side + scenario (g) substring-match per AMEND-10), the §13 6 RATIFIED-PENDING-IMPL items, and the §14 BEHAVIOR_CONTRACT.md 7-edit bundle anticipation). This sub-phase SPEC details the 22.1 IMPL-task-level surface only; it REFERENCES the parent's §4/§5/§6/§7/§8/§9/§10/§11/§13/§14 rather than repeating them.

**Predecessors:** `docs/envoy-go/phases/22-http-filter-lua/BRAINSTORM.md` (the 12-Q dialogue + envelope D design rationale; the §11 empirical pins are resolved in the parent SPEC §11). NO sub-phase BRAINSTORM (the parent BRAINSTORM + parent SPEC settled enough design — Q1-Q12 locked, AMEND-1..AMEND-12 anchored, §13-R1..R6 RATIFIED-PENDING items scoped, §16 18-item acceptance checklist drafted — that this 22.1 SPEC proceeds directly to SPEC authoring per the next-prompt-permitted skip).

**Sub-phase scope (per parent SPEC §3.1 split surface-mapping):** 22.1 lands the envelope-B-equivalent core + the NEW `internal/lua/` framework primitive at first consumer (per BRAINSTORM Q4 EXTRACT-NOW; ADR-0188) + the NEW `internal/filter/http/lua/` package (ADR-0189). Specifically:

- `Lua.DefaultSourceCode` CONSUMED (4-arm DataSource resolution per parent §5.3 + §6.2 arms 6-15);
- `Lua.SourceCodes` PARSE-REJECT (deferred-to-22.3 per parent §6.2 arm 4);
- `Lua.InlineCode` PARSE-REJECT (envoy-go-strict deprecated-field-rejection per parent §6.2 arm 3 + AMEND-6);
- `Lua.StatPrefix` CONSUMED (qualifies the 3-counter stat namespace per parent §7);
- `Lua.clear_route_cache` v1.32.4 binding-gap forward-pointer (per parent AMEND-12 + §5.4);
- `LuaPerRoute` PARSE-REJECT at any tier (per parent §6.2 arm 18); 22.3 activates;
- Pragmatic-middle Envoy↔Lua bridge surface per BRAINSTORM Q6: `envoy_on_request` + `envoy_on_response` hooks + `request_handle:headers()` + 7 headers-object methods + `__pairs` metamethod (alphabetical-snapshot per §11.2 + §13-R3 — see this SPEC's §11 D7 resolution) + 6 `:logXxx()` methods + `:streamInfo()` 4-method subset (`:protocol`/`:routeName`/`:downstreamLocalAddress`/`:downstreamDirectRemoteAddress`) + `request_handle:respond()` with full byte-pin per parent AMEND-7 + AMEND-8;
- Deferred bridge methods (`:body()` + `:bodyChunks()` + `:trailers()` + `:metadata()` + `:connection()` + `:httpCall()` + crypto/sha/base64 + `:fileBytes()` + `:timestamp()` + full `:streamInfo()` surface) raise Lua runtime errors (Lua-idiomatic disposition per BRAINSTORM Q6);
- Stdlib-sandbox-strict default-deny per parent §4.3 + AMEND-1 (envoy-go-strict DEPARTURE from upstream's bare `luaL_openlibs`-no-neutering posture; required because gopher-lua exposes `os.execute`/`io.popen`/`package.*`/`channel`/`debug.getupvalue` that upstream LuaJIT's per-worker-VM-scoping model can tolerate but envoy-go's per-stream goroutine dispatch cannot);
- 3-counter stat surface (`errors` upstream-parity + `executions` upstream-parity per parent AMEND-3 + `respond_calls` envoy-go-strict) under HCM-rooted template `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` per parent AMEND-2; project stat count 99 → 102;
- 28th project-wide fuzzer `FuzzLuaConfigParse` (count CONFIRMED at SPEC time — see this SPEC's §11 D5 resolution);
- Differential fixture `0026-http-lua-headers-bridge` — 6 wire-interactive scenarios (a)-(f) full cross-side byte-exact via existing `CompareBytes`; scenario (g) substring-match `"script load error"` via NEW OPTIONAL `BootRejectFixture` driver interface (per parent §13-R1 + AMEND-10);
- NEW `BackendKind=HTTPLua` constant addition at `test/differential/runner_test.go:547` per parent AMEND-11;
- NEW `scripts/` subdirectory layout per parent §8.4 + AMEND-11 (exploits DataSource `Filename` arm naturally + adds DataSource-arm coverage for free);
- envoy-go-side `"script load error: "` wording-pinning at `cmd/envoy-go/main.go:60-66` boot-reject path per parent §13-W;
- BEHAVIOR_CONTRACT.md 7-edit bundle at IMPL final Task per ADR-0052 atomic landing (3 envoy-go-strict departure records: stdlib-sandbox-strict per AMEND-1 + `respond_calls` per AMEND-3 + runtime-error log-message wording per AMEND-9).

**22.2 (full Envoy↔Lua bridge delta — body / trailers / metadata / connection / httpCall / crypto / streamInfo full) is OUT OF SCOPE for 22.1.** **22.3 (multi-script `SourceCodes` + per-route `LuaPerRoute` 3-arm oneof + NEW 9th canonical per-route shape ADR + ADR-0125 §(xiv) IN-PLACE AMENDMENT roster 8 → 9) is OUT OF SCOPE for 22.1.**

**ADR continuity:** Phase 21 closed at ADR-0187. Phase 22 parent SPEC anchored §Context drafts for **ADR-0188** (NEW `internal/lua/` framework primitive) + **ADR-0189** (NEW `internal/filter/http/lua/` package shape) per ADR-0044 §Context-draft discipline; their §Decision + §Consequences bodies LAND at this 22.1 IMPL's Lands-in-Tasks per ADR-0044 in-place edit discipline (Task 16 atomic landing per §6 + parent SPEC §14 + §16 acceptance items 16-17). Phase 22 parent SPEC also anchored the **ADR-0125 §(xiv) AMENDMENT-anticipation paragraph** (NEW 9th canonical per-route shape); the AMENDMENT body lands at 22.3 IMPL final Task (NOT 22.1). **At THIS 22.1 SPEC commit: NO NEW ADRs are consumed** — DECISIONS.md tail STAYS at ADR-0189; next-free ADR STAYS at **ADR-0190** (carried forward as the 22.1 IMPL escape-valve slot per BRAINSTORM Q10 WEAK HOLD + parent SPEC §1.2 + §13-R6 RATIFIED-PENDING; anticipated 0-1 consumption from `*LState`-pool benchmark surface).

**Authored:** 2026-05-18.

**Base commit:** `49cc7cd` (master tip at session entry; predecessor `e180c39` parent-SPEC SHA-fill follow-up + intermediate `49cc7cd` out-of-doctrine perf cluster+router conn-pool commit which is orthogonal to phase-22.1 docs scope).

---

## 1. Purpose / Mission

Phase 22.1 lands the foundational `envoy.filters.http.lua` filter in **VM + headers-bridge mode** — the canonical Envoy HTTP Lua scripting filter delegating per-request behavior to operator-authored Lua scripts compiled at config-load and dispatched per-stream into a fresh gopher-lua VM, with the pragmatic-middle bridge surface per BRAINSTORM Q6 (`envoy_on_request`/`envoy_on_response` hooks + `request_handle:headers()` + headers-object methods + `__pairs` metamethod + `:logXxx()` + `:streamInfo()` subset + `request_handle:respond()`) — as the foundational third of the FIFTEENTH §9 production HTTP filter (with 22.2 + 22.3 delivering the full envelope D). It establishes the entire `internal/lua/` framework primitive (gopher-lua VM lifecycle + script-compilation cache + sandbox config + bridge-registration surface; ADR-0188) + the entire `internal/filter/http/lua/` package (filter struct + factory + parse + 4-arm DataSource resolution + bridge methods + stats + 18-arm PARSE-REJECT roster + per-route stub; ADR-0189). The seven architectural primitives:

1. **NEW `internal/lua/` framework primitive** — gopher-lua VM lifecycle (`*VM` type + per-stream `*lua.LState` construction + `*Chunk` compile cache + `SandboxConfig` per-stdlib ALLOW/DENY discipline + bridge-registration surface + panic-wrapper + `BasePrintSink` discipline). EXTRACT-NOW at first consumer per BRAINSTORM Q4 (ENDS the phase-21 ZERO-NEW-framework-primitive streak; FIRST §9 row since phase 17 jwt_authn to introduce a NEW framework primitive of substantial scope). Anchored at ADR-0188 with an EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 (the second `internal/lua/` consumer — cluster_specifier Lua at `envoy.router.cluster_specifiers.lua`, access_logger Lua at `envoy.access_loggers.lua`, or string_matcher Lua at `envoy.string_matcher.lua` — whichever materializes first; the primitive's API shape is provisional at consumer #1 and may require revision after empirical validation at consumer #2 per BRAINSTORM Q4 API-REVISION ALLOWANCE). API surface refined at this SPEC §3.1 + §4.1.

2. **NEW `internal/filter/http/lua/` package** owning the filter implementation. Package directory + Go-package identifier are both `lua` (single token; matches `cors`/`fault`/`csrf`/`buffer`/`compressor`/`oauth2`/`rbac` precedent — no underscore needed). Files: `doc.go` + `lua.go` + `compiled_config.go` + `datasource.go` + `bridge.go` + `decode_headers.go` + `encode_headers.go` + `stats.go` + 5 test files per §3.2 file split. The package exposes `TypeURL` (`"type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"`) + `New` (the `HTTPFilterFactory`) per the cors/fault/.../adaptive_concurrency precedent. ADR-0189 codifies.

3. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 16 HTTP-filter entries after phase 21 — `router.New`, `adaptive_concurrency.New`, `bandwidthlimit.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `extauthz.New`, `extproc.New`, `fault.New`, `header_mutation.New`, `jwtauthn.New`, `localratelimit.New`, `oauth2.New`, `rbac.New` before `httpReg.Freeze()`) gains a seventeenth `httpReg.Register(lua.TypeURL, lua.New)` call before the freeze. Insertion alphabetical per ADR-0100 §2.2: `lua` inserts between `localratelimit` and `oauth2`. Per ADR-0072, registration order does NOT affect runtime behavior; stylistic discipline only.

4. **`Lua` proto parsing + 4-arm in-package DataSource resolution + 18-arm PARSE-REJECT roster** per parent §5.1 + §5.3 + §6.2 + AMEND-4 + AMEND-5 + AMEND-6. Resolution at config-load: `DefaultSourceCode.specifier` oneof dispatched across 4 arms (`Filename` → `os.ReadFile`; `InlineBytes` → verbatim; `InlineString` → byte-cast; `EnvironmentVariable` → `os.LookupEnv`); `WatchedDirectory` sibling field PARSE-REJECTed (deferred to future Runtime/RTDS/hot-reload phase per parent §2.1); empty `specifier` oneof PARSE-REJECTed; per-arm empty-content PARSE-REJECTs (filename name-empty / ENOENT / zero-byte; env-var name-empty / unset / empty-value); resolved bytes fed to gopher-lua `LState.LoadString` via the `internal/lua/` primitive's `CompileScript(src, cache)` API → returns `*Chunk` cached by sha256 content-hash; compile failures surface as PARSE-REJECT arm 16 (`"lua: default_source_code: compile: %w"` wrapping `*lua.ApiError`). The 18-arm roster lives at parent §6.2; this 22.1 SPEC §7 references it verbatim. **D1 — `default_source_code` absent vs no-op disposition (per parent §12-D1)** closes at Task 2 first action (upstream Envoy v1.37.2 scrape against `config.cc::createFilterFactoryFromProtoTyped` + `Filter` constructor); anticipated PARSE-REJECT both arms 5 (`default-source-code-required`) + 17 (`script-missing-required-hooks`). See §12.

5. **Pragmatic-middle bridge surface** per BRAINSTORM Q6 + parent §3.1 + §4.4. The 22.1 bridge exposes 21 surfaces total: `envoy_on_request(rh)` + `envoy_on_response(rh)` hooks (script-author-defined globals; not project-provided — the project provides the `request_handle` userdata they receive); `request_handle:headers()` returns a headers userdata with 7 methods (`:get(name)` + `:getAtIndex(name, idx)` + `:getNumValues(name)` + `:add(name, val)` + `:append(name, val)` + `:remove(name)` + `:replace(name, val)`) + the `__pairs` metamethod (alphabetical-snapshot per §11 D7 resolution); `request_handle:logTrace/:logDebug/:logInfo/:logWarn/:logErr/:logCritical(msg)` (6 log methods wired to project log sink); `request_handle:streamInfo()` returns a streamInfo userdata with 4 methods (`:protocol()` + `:routeName()` + `:downstreamLocalAddress()` + `:downstreamDirectRemoteAddress()`); `request_handle:respond(headers_table, body_string)` short-circuits to local-reply with full byte-pin per parent §11.6.7 + AMEND-7 + `:status` `[200,600)` validation + `envoy_on_response` runtime-reject per AMEND-8. Deferred methods (per parent §2.4-§2.11 + §10) raise Lua runtime errors via gopher-lua's `lua_error` — caught by panic-wrapper, increments `lua.errors`, script aborts (Lua-idiomatic disposition per BRAINSTORM Q6: scripts may `pcall` to handle gracefully).

6. **Filter-callback shape: BOTH `StreamDecoderFilter` AND `StreamEncoderFilter`** (`Decoder: non-nil`; `Encoder: non-nil`). 22.1 mirrors phase-19 ext_proc's both-sides shape — `envoy_on_request` fires at `DecodeHeaders`; `envoy_on_response` fires at `EncodeHeaders`. Static blank-identifier compile-time checks for BOTH interfaces. The decode-side surface: `DecodeHeaders(headers, endStream)` constructs the per-stream `*VM` via `lua.NewVM(opts...)`; sets up the `request_handle` userdata + metatable on `vm.State()`; calls `vm.Run(chunk)` to execute script top-level (defines `envoy_on_request` global); if `vm.HasGlobalFunc("envoy_on_request")` then `vm.CallGlobal("envoy_on_request", reqHandle)`. After CallGlobal returns: if `:respond()` fired, return `StopIteration` + `cb.SendLocalReply(status, body, headers)` per the deferred-state captured on the filter struct; otherwise return `Continue`. `DecodeData` + `DecodeTrailers` pass-through (no body access at 22.1 per §2.2 + §10 forward-pointer to 22.2). The encode-side surface mirrors decode for response_headers; `:respond()` at encode-side raises the byte-exact runtime error `"respond not currently supported in the response path"` per AMEND-8. `OnDestroy` calls `vm.Close()` releasing the `*lua.LState`.

7. **Stdlib-sandbox-strict default-deny** per parent §4.3 + AMEND-1. The `SandboxConfig` zero-value is `StrictUpstreamParity` (DENY `io/os/debug/package/channel`; DENY base-stdlib functions `dofile/loadfile/loadstring/load/require/module/collectgarbage/getfenv/setfenv`; REDIRECT `print` to `BasePrintSink` with default-nil = drop; ALLOW `os.time/os.date/os.clock/os.difftime` only via `AllowOSTimeHelpers`; ALLOW `coroutine` per AMEND-A4 matching upstream `luaL_openlibs`; ALLOW `base-core` + `table` + `string` + `math`). Implementation: NOT `OpenLibs()` (which opens everything indiscriminately) — instead call per-lib `OpenXxx` selectively, then for `AllowBaseFull == false` walk the base-globals table and `LNil` out the denied function names. The sandbox roster table + concrete IMPL discipline live at this SPEC §3.3 + parent §4.3. **envoy-go-strict departure record at BEHAVIOR_CONTRACT.md final-Task 7-edit bundle** per parent §14 edit #3 + §16 acceptance item 6: the stdlib-sandbox-strict posture is documented as an envoy-go-strict departure from upstream's bare-`luaL_openlibs`-no-neutering posture; rationale: gopher-lua's stdlib has sandbox-breaking arms (`os.execute` subprocess, `os.exit` host-process-exit, `io.popen` shell-out, `package.*` filesystem-search loader, `channel` Go-native chan exposure, `debug.getupvalue/setupvalue` cross-closure tampering) that upstream LuaJIT in upstream Envoy's per-worker deployment model assumes operator-trust + per-worker VM scoping bounds — envoy-go's per-stream goroutine dispatch model cannot make the same assumption (any operator's Lua script could compromise the entire envoy-go process; the default-deny posture is the only safe envelope at envoy-go's per-stream dispatch model).

After phase 22.1, the project has the foundational `envoy.filters.http.lua` filter: a both-decode-and-encode-side filter that constructs a per-stream gopher-lua VM at `DecodeHeaders` entry, loads the listener-config-pre-compiled script `*Chunk`, executes script top-level to define `envoy_on_request`/`envoy_on_response` globals, calls each hook against bridge methods at the respective decode + encode entry points, surfaces script execution errors via panic-wrapper → `lua.errors` counter + log, honors `request_handle:respond()` with full byte-pinned local-reply per parent §11.6.7, exposes 21 bridge surfaces (2 hooks + 7 headers methods + `__pairs` + 6 log + 4 streamInfo + 1 respond) under the sandbox-strict default-deny posture, and is OBSERVABLE-OUTCOMES byte-equivalent to reference Envoy v1.37.2 on the 6 wire-interactive fixture-0026 scenarios (a)-(f) + substring-match-equivalent on scenario (g) script-compile-error — modulo the 3 envoy-go-strict documented divergence-windows (stdlib-sandbox-strict per AMEND-1; `respond_calls` envoy-go-strict counter per AMEND-3; gopher-lua-vs-LuaJIT runtime-error log-message wording divergence per AMEND-9). Phase 22.2 then activates the full Envoy↔Lua bridge delta + the body-buffering interaction with ADR-0128 + the `internal/httpclient/` co-consumer at first cross-phase consumption against the same package surface. Phase 22.3 then activates the multi-script `SourceCodes` + the `LuaPerRoute` 3-arm oneof + the NEW 9th canonical per-route shape ADR + the ADR-0125 §(xiv) IN-PLACE AMENDMENT against the same package surface.

### 1.1 Empirical-finding-driven scope (per parent SPEC §1.1)

The 12 §1.1 AMENDs in the parent SPEC are the empirical-finding-driven scope revisions for phase 22. The amendments load-bearing for 22.1: **AMEND-1** (stdlib-sandbox-strict — envoy-go-strict DEPARTURE; §3.3 + §10 below incorporate the departure record), **AMEND-2** (stat-prefix template HCM-rooted — §8 below cross-references), **AMEND-3** (`executions` reclassification as upstream-parity; corrected envoy-go-strict departure bundle 2 → 1 record for `respond_calls` only — §8 + §10 cross-reference), **AMEND-4** (18-arm PARSE-REJECT roster — §7 below), **AMEND-5** (DataSource 10-arm refinement — §7 + §4.2 datasource.go implementation incorporate), **AMEND-6** (3 additional baseline arms; sandbox-violation arm RETRACTED — §7 incorporates), **AMEND-7** (`:respond()` full byte-pin extension — §4.3 bridge respond IMPL incorporates + §9 fixture-0026 scenario (d) IMPL pins), **AMEND-8** (`:respond()` from `envoy_on_response` runtime-reject + `:status` `[200,600)` validation — §4.3 incorporates), **AMEND-9** (gopher-lua-vs-LuaJIT divergences + RECOMMEND-DEPARTURE-RECORD for runtime-error log-message wording — §10 + §14 incorporate), **AMEND-10** (fixture-0026 scenario (g) substring-match scope-narrow — §9 incorporates), **AMEND-11** (NEW `BackendKind=HTTPLua` + `scripts/` subdirectory layout — §9 + §6 Task 13-14 incorporate), **AMEND-12** (v1.32.4 vs v1.37.2 binding-gap forward-pointers — silent-drop disposition; no 22.1 IMPL surface).

This 22.1 SPEC's §3/§4/§7/§8/§9/§10/§14 incorporate all 12 amendments. The 22.1 SPEC author makes NO NEW substantive scope revisions vs the parent SPEC; all design decisions inherit cleanly. The 22.1 SPEC's ADDITIVE contributions:

- **D5 resolution at SPEC time** (per §11 below): 28th-fuzzer count CONFIRMED via project-wide grep. Pins ADR-0189 §Decision body + BEHAVIOR_CONTRACT.md §13.4 patch to **28** at IMPL Task 16 atomic landing.
- **D7 resolution at SPEC time** (per §11 below): envoy-go headers-map type EMPIRICALLY-DETERMINED as `net/http.Header` (= Go `map[string][]string`; unordered). REFUTES BRAINSTORM hypothesis of insertion-ordered slice-backed; scenario (f) cross-side determinism STILL HOLDS via count-only assertion (the fixture-0026 scenario (f) script counts pairs, doesn't depend on iteration order). NEW bridge `__pairs` metamethod discipline RATIFIED at §11.2 sub-pin: snapshot to alphabetically-sorted `[]struct{k,v string}` at `__pairs` invocation + iterate by integer index (matches `net/http.Header.Write` precedent; closes per-run map-iteration non-determinism for script-author debugging).
- **Refined `internal/lua/` API signatures** (per §3.1 + §4.1 below): function-option `VMOption` pattern, `State()`/`HasGlobalFunc()`/`CallGlobal()` split (cleaner than parent SPEC §4.1 sketch's `Run(chunk, hooks ...HookFn)` blob), `WithPanicHandler` / `WithBasePrintSink` naming clarification.
- **Refined `internal/lua/` file split** (per §3.2 below): 3 production files (vm.go + sandbox.go + compile.go) + 3 test files (mirrors compact primitive-package precedent at `internal/jwks/` + `internal/jwt/` + `internal/httpclient/`).

---

## 2. Non-purposes

Phase 22.1 is the first sub-phase of the phase-22 BRAINSTORM-time 3-way pre-split. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land VM + headers-bridge `envoy.filters.http.lua` under the existing 07.1 framework + the 2 NEW ADRs (ADR-0188 NEW `internal/lua/` framework primitive + ADR-0189 NEW `internal/filter/http/lua/` package shape).

- **2.1 Body-access bridge methods OUT OF SCOPE.** `:body()` + `:bodyChunks()` PARSE-REJECT semantically at runtime per the deferred-method-runtime-error discipline (BRAINSTORM Q6). 22.2 activates with the interaction with phase-13 ADR-0128 decode-side body-buffering primitive (the 22.2 SPEC settles the exact interaction discipline, including coroutine-vs-goroutine-resume choice).
- **2.2 Trailer-access bridge methods OUT OF SCOPE.** `:trailers()` raises Lua runtime errors. 22.2 activates.
- **2.3 Dynamic-metadata bridge surfaces OUT OF SCOPE.** `:metadata()` + `:streamInfo():dynamicMetadata()` + `:streamInfo():dynamicTypedMetadata()` raise runtime errors. 22.2 SPEC settles disposition; likely PARSE-REJECT or partial-deferral per the project's cross-phase dynamic-metadata-deferral discipline (deferred at phases 16/17/18/19/20).
- **2.4 Connection-info bridge surface OUT OF SCOPE.** `:connection()` raises runtime errors. 22.2 activates; integrates with phase-03 TLS primitives.
- **2.5 Outbound HTTP call bridge surface OUT OF SCOPE.** `:httpCall()` raises runtime errors. 22.2 activates; REUSES phase-20 `internal/httpclient/` framework primitive at first co-consumer (validates the phase-20 extraction per ADR-0177 §Consequences forward-pointer).
- **2.6 Crypto / base64 / sha helpers OUT OF SCOPE.** `:base64Escape()` + `:base64Decode()` + `:sha256()` + `:sha512()` + `:importPublicKey()` + `:verifySignature()` raise runtime errors. 22.2 activates; likely thin wrappers over Go's `crypto/*` + `encoding/base64`.
- **2.7 File-read helper OUT OF SCOPE.** `:fileBytes()` raises runtime errors. 22.2 activates; security caveats apply (sandbox concern; 22.2 SPEC settles the security discipline).
- **2.8 Time helper OUT OF SCOPE.** `:timestamp()` raises runtime errors. 22.2 activates; non-deterministic, complicates cross-side byte-comparison for scenarios that include timestamps.
- **2.9 Full `:streamInfo()` surface OUT OF SCOPE.** Per BRAINSTORM Q6 pragmatic-middle: `:upstreamHost()` + `:upstreamCluster()` + `:dynamicMetadata()` + `:dynamicTypedMetadata()` + `:requestedServerName()` + `:filterState()` + `:downstreamSslConnection()` raise runtime errors at 22.1. 22.2 activates.
- **2.10 `Lua.SourceCodes` named-script map OUT OF SCOPE.** PARSE-REJECT at 22.1 (parent §6.2 arm 4 `"lua: source_codes map is not yet supported (lands in phase 22.3)"`). 22.3 activates.
- **2.11 `Lua.InlineCode` deprecated field OUT OF SCOPE.** PARSE-REJECT at 22.1 (parent §6.2 arm 3 + envoy-go-strict deprecated-field-rejection discipline + AMEND-6). NEVER re-enabled in envoy-go.
- **2.12 `LuaPerRoute` 3-arm oneof OUT OF SCOPE.** PARSE-REJECT at any tier (parent §6.2 arm 18 via HCM `RegisterPerRouteValidator` hook per ADR-0110 single-chokepoint). 22.3 activates with NEW 9th canonical per-route shape ADR + ADR-0125 §(xiv) IN-PLACE AMENDMENT.
- **2.13 `WatchedDirectory` DataSource sibling field OUT OF SCOPE.** PARSE-REJECT at 22.1 (parent §6.2 arm 7). Deferred to future Runtime/RTDS/hot-reload family phase.
- **2.14 `Lua.clear_route_cache` (v1.37.2 field 5) NEVER-DEFERRED at 22.1.** Per parent AMEND-12 + §5.4 v1.32.4 binding-gap forward-pointer. The field is ABSENT from envoy-go's consumed v1.32.4 binding; activates when go-control-plane bumps to v1.37.x.
- **2.15 `LuaPerRoute.filter_context` (v1.37.2 field 4) NEVER-DEFERRED at 22.1+22.3.** Same disposition; v1.32.4 binding-gap.
- **2.16 Lua 5.2 / 5.3 / 5.4 dialect features NEVER-DEFERRED.** gopher-lua is Lua 5.1 (matches upstream LuaJIT 5.1 dialect); scripts using 5.2+ features (`goto`, integer subtype, `bit32` / `utf8` stdlib) get Lua-compile-time errors at script-source compilation (arm 16 PARSE-REJECT). Documented in `internal/lua/doc.go` + `internal/filter/http/lua/doc.go` at IMPL Task 1.
- **2.17 Async script execution coroutines NEVER-DEFERRED for 22.1.** Upstream Envoy's Lua filter supports per-script Lua coroutines for yielding during `:httpCall()` and `:body()` await. envoy-go's 22.2 implementation may or may not adopt coroutines (22.2 SPEC settles); alternative is goroutine-resume-on-completion (mirrors phase-09 fault async primitive + phase-18.2 ext_authz_grpc async-resume primitive). The `coroutine` stdlib library IS exposed in 22.1's sandbox (per parent AMEND-A4 + §4.3 matching upstream's `luaL_openlibs` opening it) but no 22.1 bridge methods consume coroutines.
- **2.18 No filter-chain ordering surgery.** lua registers as one more entry in the existing extension registry; the HCM filter-chain iteration protocol is unchanged.
- **2.19 No `*LState` pool at 22.1 (WEAK-default per parent §13-R6).** Per-stream `*LState` construction with shared per-script-source `*Chunk` cache. If 22.1 IMPL benchmarks of per-stream construction cost surface > 1ms unacceptable overhead, the ADR-0190 escape-valve consumes (per parent §1.2 + §13-R6) anchoring a "per-script-source `*LState` pool with chunk-pre-loaded entries" decision. Until benchmarked, hold per-stream construction as the WEAK-default. See §13 (RATIFIED-PENDING items).
- **2.20 No `response_code_details` emission** — unchanged from phase-16/17/18/19/20/21; envoy-go's HCM does not surface `response_code_details` to local-reply callers (phase-04 scope). Documented divergence-window joint with prior §9 rows.

---

## 3. Framework primitive (NEW `internal/lua/` + NEW `internal/filter/http/lua/`)

Per parent SPEC §4 + BRAINSTORM Q4 + AMEND-1. The 22.1 SPEC refines the parent SPEC §4.1 API sketch into production signatures + concretizes the file split per §3.2. The 22.1 SPEC author's discretion per the next-prompt-permitted scope:

- The parent SPEC §4.1 sketch is **provisional** ("settled at 22.1 SPEC"). This 22.1 SPEC's §3.1 + §4.1 anchor the production signatures + the cross-cutting discipline (sandbox roster + per-stream lifecycle + bridge-registration seam + panic-wrapper shape).
- Parent SPEC §4.2 (per-stream `*LState` construction + per-script compile cache discipline) STANDS UNCHANGED; this 22.1 SPEC §3.4 implements at Task 4-5.
- Parent SPEC §4.3 (sandbox roster — default-deny per AMEND-1) STANDS UNCHANGED; this 22.1 SPEC §3.3 implements at Task 5 + per-stdlib `OpenXxx` selective + post-walk nil-out.
- Parent SPEC §4.4 (NEW `internal/filter/http/lua/` package file split) STANDS — this 22.1 SPEC §3.2 confirms the 8-production-file + 5-test-file split per the parent's sketch; minor refinement = no change to file count.
- Parent SPEC §4.5 (ADR-0125 §(xiv) AMENDMENT-anticipation paragraph) — UNCHANGED at this 22.1 SPEC commit; the AMENDMENT body lands at 22.3 IMPL final Task.

### 3.1 `internal/lua/` API signatures — refined from parent §4.1 sketch (lands at IMPL Task 1 + Task 4 + Task 5)

Refines parent SPEC §4.1 sketch into production signatures. Key refinements vs parent sketch:

- **`VMOption` pattern changed from sealed interface to function-option** (`type VMOption func(*VM)`) — idiomatic Go option pattern (matches `internal/jwks/Fetcher` `Option`-pattern precedent + `internal/httpclient/` precedent). Easier to extend at consumer-#2 phases without API revision.
- **`Run(chunk, hooks ...HookFn)` split into `Run(chunk) + HasGlobalFunc(name) + CallGlobal(name, args...)`** — cleaner separation of "load + execute top-level" (defines globals like `envoy_on_request`) from "invoke a named global function with args" (the actual hook firing). The parent SPEC's `HookFn func(*VM) error` Go-callback type conflated Go-side callbacks with Lua-side globals; the refined API treats them distinctly.
- **`State() *lua.LState` exposed** — escape-hatch for the filter consumer to set up userdata + metatables (the `request_handle`/`response_handle` userdata patterns mirror upstream Envoy's `wrappers.cc`'s metatable discipline; require direct `*lua.LState` access that a general primitive cannot abstract over without an over-engineered DSL).
- **`WithPanicHandler` renamed from `WithPanicRecovery`** — Go cannot RECOVER from a Lua-side runtime error mid-PCall (gopher-lua's `*lua.ApiError` is returned via PCall's error path, not via Go panic). The handler is invoked AFTER `recover()` in the Go-side panic wrapper for genuine Go panics (e.g., from a bridge method's Go callback panicking); the gopher-lua `*lua.ApiError` path is handled via the explicit Run/CallGlobal error returns.
- **`WithStderrSink` renamed to `WithBasePrintSink` + relocated from SandboxConfig struct field to VMOption** — parent §4.1 sketch (lines 187 + 192 in parent SPEC) placed `BasePrintSink io.Writer` as a SandboxConfig struct field AND `WithStderrSink(sink io.Writer) VMOption` as an option constructor (effectively duplicated). The 22.1 SPEC's refinement: ONE source-of-truth via `WithBasePrintSink(w io.Writer) VMOption` (option only; no SandboxConfig struct field). The sink redirects Lua-script `print(...)` output specifically (the base stdlib's `print` function), NOT a generic stderr stream. Default nil = drop (no stdout leak; envoy-go-strict default). DUAL CHANGE: rename + relocate (avoiding parent §4.1's accidental duplication).
- **`CompileCache` nil-tolerant** per ADR-0085 nil-tolerance discipline. `CompileScript(src, nil)` returns a `*Chunk` without caching; useful for one-shot compilation paths.

Production signatures (lands at IMPL Task 1 + Task 4 + Task 5):

```go
// internal/lua/vm.go — VM lifecycle + options + bridge-registration

package lua

import (
    "io"

    lua "github.com/yuin/gopher-lua"
)

// VM is a per-stream gopher-lua execution context. NOT goroutine-safe.
// Each per-stream filter dispatch constructs a fresh VM via NewVM;
// OnDestroy releases via Close.
type VM struct {
    // unexported fields:
    // state    *lua.LState
    // sandbox  SandboxConfig
    // panicH   PanicHandlerFn
    // printSink io.Writer
}

// VMOption configures VM construction. Function-option pattern.
type VMOption func(*VM)

// WithSandboxConfig sets the per-stdlib ALLOW/DENY posture. Zero value =
// StrictUpstreamParity (DENY io/os/debug/package/channel + base-stdlib
// dangerous globals; ALLOW base-core + table + string + math + coroutine
// + os.time/date/clock/difftime via AllowOSTimeHelpers). See §3.3.
func WithSandboxConfig(sb SandboxConfig) VMOption

// WithPanicHandler sets the Go-panic handler invoked after recover() in
// the VM's panic-wrapper. The handler is invoked with the recovered value.
// NOT for catching Lua-side runtime errors (those return via *lua.ApiError
// from Run/CallGlobal); the handler is for genuine Go panics from bridge
// method Go callbacks.
func WithPanicHandler(h PanicHandlerFn) VMOption

// WithBasePrintSink redirects Lua-script print(...) output. Default nil =
// drop (no stdout leak; envoy-go-strict default).
func WithBasePrintSink(w io.Writer) VMOption

// NewVM constructs a per-stream VM. Applies sandbox config + bridge-method
// registry init + panic-wrapper setup. Caller responsibility: Close at
// OnDestroy. Returns a non-nil *VM.
func NewVM(opts ...VMOption) *VM

// State returns the underlying *lua.LState. ESCAPE-HATCH for filter
// consumers to set up userdata + metatables (request_handle/response_handle
// patterns require direct LState access). Not safe to call after Close.
func (vm *VM) State() *lua.LState

// RegisterGlobalFunc registers a Go function as a Lua-callable global.
// CONVENIENCE for simple globals; userdata + metatables go via State().
func (vm *VM) RegisterGlobalFunc(name string, fn lua.LGFunction)

// Run loads the chunk's *FunctionProto onto this VM's *LState (cheap; no
// recompilation per stream — chunks are bytecode-only, no LState-specific
// state) and executes script top-level (which defines any globals declared
// by the script, e.g., envoy_on_request/envoy_on_response). Returns
// *lua.ApiError on Lua runtime error.
func (vm *VM) Run(chunk *Chunk) error

// HasGlobalFunc returns true if the named global is a callable function.
// Used by the filter to check hook-presence after Run (supports D1
// script-missing-required-hooks PARSE-REJECT — anticipated; closes at
// IMPL Task 2 against upstream Envoy).
func (vm *VM) HasGlobalFunc(name string) bool

// CallGlobal invokes the named global function with args. Returns
// *lua.ApiError on Lua runtime error. The filter consumer pushes userdata
// (e.g., request_handle) via the args slice using lua.LValue (typically
// the LUserData wrapping a Go pointer).
func (vm *VM) CallGlobal(name string, args ...lua.LValue) error

// Close releases the VM's *LState. Idempotent.
func (vm *VM) Close()

// PanicHandlerFn is invoked after recover() in the VM's panic-wrapper.
// recovered is the value returned by recover() (typically the panic value).
type PanicHandlerFn func(recovered any)
```

```go
// internal/lua/compile.go — Chunk + CompileCache

package lua

// Chunk wraps a compiled *FunctionProto safe for cross-VM reuse.
type Chunk struct {
    // unexported fields:
    // proto *lua.FunctionProto
    // hash  [32]byte  // sha256(src) — for cache identity
}

// CompileCache is a content-addressed compile cache, keyed by sha256(src).
// Owned by the *compiledConfig (filter-config-instance scope); GC-driven
// eviction (no manual evict; cache lifetime == compiledConfig lifetime).
// Safe for concurrent read/add via internal sync.RWMutex.
type CompileCache struct {
    // unexported fields:
    // mu    sync.RWMutex
    // store map[[32]byte]*Chunk
}

// NewCompileCache constructs an empty cache. Caller responsibility: keep
// alive for the duration of the dependent compiledConfig.
func NewCompileCache() *CompileCache

// CompileScript compiles src via gopher-lua's parser. If cache is non-nil,
// caches by sha256(src) — subsequent calls with the same src return the
// cached *Chunk without re-parsing. If cache is nil, compiles uncached
// (per ADR-0085 nil-tolerance discipline). Returns *lua.ApiError wrapped
// as a plain error on compile failure.
func CompileScript(src []byte, cache *CompileCache) (*Chunk, error)
```

```go
// internal/lua/sandbox.go — SandboxConfig + per-stdlib ALLOW/DENY

package lua

// SandboxConfig governs which stdlib modules are exposed to scripts.
// Zero value = StrictUpstreamParity per §3.3 + parent SPEC §4.3 +
// AMEND-1 (envoy-go-strict DEPARTURE from upstream's bare luaL_openlibs).
type SandboxConfig struct {
    // AllowBaseFull exposes base stdlib's dangerous globals
    // (dofile/loadfile/loadstring/load/require/module/collectgarbage/
    //  getfenv/setfenv). Default false = strict.
    AllowBaseFull bool

    // AllowIO exposes io.* wholesale (io.open/io.popen — unsandboxed
    // file + subprocess access). Default false = strict.
    AllowIO bool

    // AllowOS exposes os.* wholesale (os.execute/exit/remove/rename/
    // getenv/setlocale/tmpname — subprocess + host-Go-process-exit
    // hazards). Default false = strict; AllowOSTimeHelpers preserves
    // the upstream-parity read-only time arm.
    AllowOS bool

    // AllowOSTimeHelpers exposes only os.time / os.date / os.clock /
    // os.difftime. Independent of AllowOS — if AllowOS is true,
    // AllowOSTimeHelpers is implied. Default false; 22.1 sandbox
    // default still allows time helpers (matches upstream-parity arm).
    AllowOSTimeHelpers bool

    // AllowDebug exposes debug.* wholesale (debug.getupvalue/setupvalue/
    // setfenv — cross-closure tampering; sandbox-breaking). Default
    // false = strict. EXCEPTION: debug.traceback is re-exposed as an
    // INTERNAL-only global for the panic-wrapper's use (NOT in the
    // script's namespace) regardless of this flag.
    AllowDebug bool

    // AllowPackage exposes package.* wholesale (filesystem-search
    // loader loLoaderLua reads .lua from disk). Default false = strict.
    AllowPackage bool

    // AllowChannel exposes gopher-lua's channel stdlib (Go-native chan
    // exposure to Lua). No LuaJIT counterpart; no upstream-parity
    // argument. Default false = strict.
    AllowChannel bool

    // AllowCoroutine exposes the coroutine stdlib. Default true via
    // zero-value-helper applied by NewVM (matches upstream luaL_openlibs
    // opening it; 22.2's :body()/:httpCall() may consume internally).
    AllowCoroutine bool
}
```

**Implementation discipline** (per parent §4.3 + AMEND-1):

- NewVM does NOT call `*lua.LState.OpenLibs()` (which opens everything indiscriminately).
- Instead, NewVM calls the per-lib `OpenXxx` selectively against the resolved `SandboxConfig` (with zero-value defaults applied) → opens only the modules the config permits.
- For `AllowBaseFull == false`: NewVM walks the base globals table after `OpenBase` and `LNil`s out the denied function names (`dofile`/`loadfile`/`loadstring`/`load`/`module`/`require`/`collectgarbage`/`getfenv`/`setfenv`).
- For `AllowOSTimeHelpers && !AllowOS`: NewVM calls `OpenOs` and then nils out the disallowed `os.execute`/`exit`/`remove`/`rename`/`getenv`/`setlocale`/`tmpname` entries on the resulting module table.
- For `AllowCoroutine` (defaulted to true): NewVM calls `OpenCoroutine`.
- For `print`: NewVM rebinds `print` to a Go function that writes to `BasePrintSink` if non-nil, otherwise drops the output.
- For `debug.traceback`: NewVM exposes via an INTERNAL global (e.g., `__envoy_traceback`) used by the panic-wrapper, NOT under `debug.traceback` (since `AllowDebug == false` denies the `debug` table).

### 3.2 `internal/lua/` file split

3 production files + 3 test files (compact primitive-package precedent mirrors `internal/jwks/` + `internal/httpclient/`):

```
internal/lua/
  doc.go            # package overview + AMEND-1 sandbox-strict rationale + AMEND-9
                    # gopher-lua-vs-LuaJIT divergence cross-refs + API surface summary
  vm.go             # VM type + NewVM + VMOption + State + RegisterGlobalFunc +
                    # Run + HasGlobalFunc + CallGlobal + Close + panic-wrapper +
                    # WithSandboxConfig + WithPanicHandler + WithBasePrintSink
  sandbox.go        # SandboxConfig type + per-stdlib OpenXxx selective + post-walk
                    # nil-out per §3.3 implementation discipline
  compile.go        # Chunk + CompileCache + NewCompileCache + CompileScript +
                    # sha256-keyed caching + sync.RWMutex discipline
  vm_test.go        # VM lifecycle + options + RegisterGlobalFunc + Run/HasGlobal/
                    # CallGlobal table-driven + concurrency tests + panic-wrapper
                    # behavior tests
  sandbox_test.go   # per-stdlib ALLOW/DENY exhaustive table-driven; verifies
                    # dofile/loadfile/loadstring/require/io.open/io.popen/os.execute/
                    # os.exit/debug.getupvalue/channel.make/package.path are
                    # nil-or-error post-sandbox-strict construction
  compile_test.go   # NewCompileCache + CompileScript + cache-hit-on-same-content-hash +
                    # cache-miss-on-different-source + concurrent-read/add tests
```

### 3.3 Sandbox roster — default-deny per parent §4.3 + AMEND-1

The 22.1 `SandboxConfig` zero-value posture is STRICT-DEFAULT-DENY. The roster table per parent §4.3 STANDS UNCHANGED at this 22.1 SPEC; reproduced here for in-place readability:

| gopher-lua module | Exposed by `OpenLibs()` | envoy-go 22.1 disposition (zero-value SandboxConfig) | Rationale |
|---|---|---|---|
| `base` | yes (27 funcs) | ALLOW core; DENY `dofile`, `loadfile`, `loadstring`, `load`, `module`, `require`, `collectgarbage`, `getfenv`, `setfenv`; REDIRECT `print` (BasePrintSink; default = drop) | sandbox-breaking arbitrary-code-load + cross-closure tampering |
| `package` | yes | DENY wholesale (do not call `OpenPackage`) | filesystem-search loader `loLoaderLua` reads `.lua` from disk |
| `table` | yes | ALLOW | safe |
| `io` | yes | DENY wholesale | `io.open`/`io.popen` are unsandboxed file/subprocess access |
| `os` | yes | DENY `execute`, `exit`, `remove`, `rename`, `tmpname`, `setlocale`, `getenv`; ALLOW `os.time`, `os.date`, `os.clock`, `os.difftime` (read-only time helpers; preserves upstream-parity time arm) | subprocess + host-Go-process-exit hazards |
| `string` | yes | ALLOW | safe |
| `math` | yes | ALLOW | safe |
| `debug` | yes | DENY wholesale to scripts; INTERNAL re-expose `debug.traceback` via `__envoy_traceback` global for panic-wrapper use | cross-closure tampering — sandbox-breaking |
| `channel` (gopher-lua-specific) | yes | DENY wholesale | No LuaJIT counterpart; exposes Go-native chan |
| `coroutine` | yes | ALLOW per AMEND-A4 | Matches upstream `luaL_openlibs` opening it; 22.2's `:body()`/`:httpCall()` may consume internally |

**Implementation discipline at IMPL Task 5:** per §3.1 production discipline above — NOT `OpenLibs()`; selective `OpenXxx` per-lib + post-walk nil-out for the denied functions. Sandbox test discipline at IMPL Task 5 verifies each denied function is nil-or-runtime-error post-sandbox-strict construction (see §3.2 sandbox_test.go).

**envoy-go-strict departure record at IMPL Task 16** (per parent §14 edit #3 + §16 acceptance item 6): stdlib-sandbox-strict departure recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures section. The departure rationale: gopher-lua exposes sandbox-breaking modules that upstream LuaJIT's per-worker deployment model assumes operator-trust + per-worker VM scoping bounds — envoy-go's per-stream goroutine dispatch model cannot make the same assumption.

### 3.4 Per-stream `*LState` construction + per-script-source `*Chunk` cache (per parent §4.2 + §11.3.4)

Per parent §4.2 + §11.3.4. The 22.1 WEAK-default: per-stream `*LState` construction with shared per-script-source `*Chunk` cache. Each per-stream invocation:

1. The filter's `*compiledConfig` (built at config-load via `buildCompiledConfig` per §4.2) carries the pre-compiled `*Chunk` (compiled once via `lua.CompileScript(src, cfg.compileCache)`).
2. At `DecodeHeaders` entry, the filter calls `vm := lua.NewVM(opts...)`, constructing a fresh `*lua.LState` with the sandbox roster applied (cheap loading of the configured stdlib + nil-ing of denied functions per §3.3).
3. The filter sets up the `request_handle` userdata + metatable on `vm.State()` via gopher-lua's metatable API (LUserData wrapping `*requestHandleContext` Go struct; metatable __index → table of `request_handle:headers()`/`:logXxx()`/`:streamInfo()`/`:respond()` bridge methods).
4. The filter calls `vm.Run(cfg.chunk)` which loads the `*FunctionProto` from `Chunk` onto the new `*LState` (cheap — chunks are bytecode-only, no LState-specific state) and executes script top-level (defining any global functions declared by the script).
5. If `vm.HasGlobalFunc("envoy_on_request")`: the filter calls `vm.CallGlobal("envoy_on_request", reqHandleUserdata)`.
6. If `:respond()` fired during the hook: filter returns `StopIteration` + `cb.SendLocalReply(captured_status, captured_body, captured_headers)`. Otherwise: filter returns `Continue`.
7. `cfg.stats.executions++` after CallGlobal regardless of outcome (matches upstream-parity per AMEND-3).
8. If CallGlobal returned an error: `cfg.stats.errors++` + log via the configured sink + continue (NO wire-side error surface per BRAINSTORM §2.9 — errors don't terminate the stream).
9. The encode-side mirrors decode for `envoy_on_response` + `response_handle` (with `:respond()` raising the AMEND-8 runtime-error string).
10. At `OnDestroy`: the filter calls `vm.Close()` releasing the `*lua.LState` (releases registry + call-stack memory).

**`*LState`-pool design (ESCAPE-VALVE-CANDIDATE per parent §1.2 + §13-R6):** if 22.1 IMPL benchmarks of per-stream construction cost surface unacceptable overhead (e.g., > 1ms per stream at the headers-only bridge surface), the ADR-0190 escape-valve slot anchors a "per-script-source `*LState` pool with chunk-pre-loaded entries" decision. Until benchmarked, hold per-stream construction as the WEAK-default. The pool design (per-script-source pool keying, chunk pre-loading, lifecycle management vs `sync.Pool`'s GC-driven eviction) is non-trivial enough to merit its own ADR if triggered.

### 3.5 `internal/filter/http/lua/` file split (per parent §4.4 STANDS)

Parent SPEC §4.4 file split STANDS UNCHANGED at this 22.1 SPEC. 8 production files + 5 test files:

```
internal/filter/http/lua/
  doc.go                  # package overview + Q1-Q12 BRAINSTORM decision summary +
                          # AMEND-1..AMEND-12 cross-references + D1+D7 cross-refs +
                          # API surface summary (filterStats, compiledConfig, TypeURL)
  lua.go                  # filter struct + factory (HTTPFilterFactory) + filterStats +
                          # TypeURL + filterName + per-route validator registration
  compiled_config.go      # config parse + 18-arm PARSE-REJECT roster per parent §6.2 +
                          # script-compile cache key generation + D1 closure at Task 2
                          # first-action upstream-scrape
  datasource.go           # 4-arm DataSource arm resolution (Filename + InlineBytes +
                          # InlineString + EnvironmentVariable) + WatchedDirectory
                          # PARSE-REJECT + empty-oneof PARSE-REJECT
  bridge.go               # request_handle/response_handle userdata + metatable setup +
                          # 7 headers methods + __pairs metamethod (alphabetical-snapshot
                          # per §11 D7 resolution) + 6 log methods + 4 streamInfo methods +
                          # respond byte-pin per parent §11.6.7
  decode_headers.go       # DecodeHeaders implementation + envoy_on_request hook firing +
                          # respond-state handling (StopIteration + SendLocalReply path)
  encode_headers.go       # EncodeHeaders implementation + envoy_on_response hook firing +
                          # AMEND-8 respond runtime-reject in response-path
  stats.go                # 3-counter stat surface registration under HCM-rooted template
                          # per parent §7 (errors + executions + respond_calls)
  lua_test.go             # filter + factory + filterStats + decode/encode wiring +
                          # per-stream VM lifecycle integration
  compiled_config_test.go # 18-arm PARSE-REJECT table-driven tests per parent §6.2
  datasource_test.go      # 4-arm DataSource resolution + WatchedDirectory PARSE-REJECT +
                          # empty-oneof PARSE-REJECT + file-read failure paths
  bridge_test.go          # all bridge methods + __pairs alphabetical-snapshot verification +
                          # respond byte-pin per parent §11.6.7 + AMEND-8 runtime-reject
  fuzz_test.go            # 28th project-wide fuzzer FuzzLuaConfigParse with ~30 corpus seeds
```

Boot-registration insertion at `cmd/envoy-go/main.go`: alphabetical between `localratelimit.New` and `oauth2.New` per ADR-0100 §2.2 stylistic discipline. 16 HTTP filters wired pre-phase-22.1; **17 post-phase-22.1**. The Go-package identifier is `lua` (single token; matches `cors`/`fault`/`csrf`/`buffer`/`compressor`/`oauth2`/`rbac` precedent).

---

## 4. compiledConfig + code shapes

### 4.1 Public surface

```go
package lua

import (
    "github.com/esalaine/envoy-go/internal/filter/http"  // for HTTPFilterFactory
)

// TypeURL is the canonical Envoy type-URL for the Lua HTTP filter config.
const TypeURL = "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"

// filterName is the canonical Envoy filter name.
const filterName = "envoy.filters.http.lua"

// New is the HTTPFilterFactory registered at boot per ADR-0072.
func New(ctx http.FilterFactoryContext) (http.FilterInstanceFactory, error)
```

`New` parses + validates the `Lua` proto into a `*compiledConfig`, allocates the `filterStats`, compiles the resolved DataSource bytes via `lua.CompileScript`, registers the per-route validator per parent §6.2 arm 18, and returns a `FilterInstanceFactory` closure that produces per-stream `*filter` values. Mirrors the cors/.../adaptive_concurrency factory shape.

### 4.2 `compiledConfig` + `filterStats` shape

```go
package lua

import (
    "github.com/esalaine/envoy-go/internal/lua"
    "github.com/esalaine/envoy-go/internal/stats"
)

// compiledConfig is the immutable post-parse listener-level config.
type compiledConfig struct {
    chunk        *lua.Chunk         // pre-compiled default_source_code (single chunk at 22.1; SourceCodes map adds 22.3)
    compileCache *lua.CompileCache  // chunk-cache holder (kept alive for compiledConfig lifetime; GC-driven eviction)
    sandbox      lua.SandboxConfig  // zero-value at 22.1 = StrictUpstreamParity (no Lua proto knob for sandbox; departs from upstream-parity per AMEND-1)
    stats        *filterStats       // SHARED across listener; no per-route stat at 22.1 (no per-route at 22.1; LuaPerRoute PARSE-REJECTs)
}

// filterStats — 3 counters per parent §7 + AMEND-3.
// All COUNTERS; namespace http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat> (HCM-rooted per AMEND-2).
type filterStats struct {
    errors       *stats.Counter // upstream-parity per AMEND-3 (ALL_LUA_FILTER_STATS macro arm 1)
    executions   *stats.Counter // upstream-parity per AMEND-3 (ALL_LUA_FILTER_STATS macro arm 2)
    respondCalls *stats.Counter // envoy-go-strict extension per AMEND-3; departure record at BEHAVIOR_CONTRACT.md §13.6 row 2 per §14 edit #4
}
```

The `compiledConfig` struct is **mode-agnostic and field-final at 22.1** — 22.2 adds the body-buffering interaction state (likely a `*bodyBufferConfig` field); 22.3 adds the `SourceCodes` map (likely a `map[string]*lua.Chunk` field) + the per-route TPFC dispatch state. 22.1 reserves no fields for the future deltas; field-additive growth at 22.2 + 22.3.

The 3 counters allocate **unconditionally** at `New()` time via `newFilterStats(reg, baseStatPrefix(ctx.StatPrefix))` (mirrors phase-17 jwt_authn + phase-18.1 ext_authz + phase-19.1 ext_proc unconditional-allocation discipline). Per ADR-0085 nil-tolerance: `buildCompiledConfig` guards `if ctx.Stats != nil` before `newFilterStats`.

### 4.3 `DecodeHeaders` body — top-level dispatch

```
DecodeHeaders(headers http.Header, endStream bool):
  1. construct per-stream VM:
       vm := lua.NewVM(
           lua.WithSandboxConfig(cc.sandbox),
           lua.WithPanicHandler(panicLog),
           lua.WithBasePrintSink(nil),  // drop (envoy-go-strict default; no Lua-script-print-to-stdout leak)
       )
       defer-on-OnDestroy: vm.Close()  // captured in filter state for OnDestroy hook
  2. set up request_handle userdata + metatable on vm.State():
       reqUd := vm.State().NewUserData()
       reqUd.Value = &requestHandleContext{
           filter:  f,
           headers: headers,
           cb:      f.dcb,
       }
       L := vm.State()
       reqUd.Metatable = requestHandleMT  // pre-built metatable with __index → bridge methods
  3. execute script top-level (defines envoy_on_request / envoy_on_response globals):
       if err := vm.Run(cc.chunk); err != nil {
         cc.stats.errors++
         logError("lua: script run failed: %v", err)
         return Continue  // continue dispatch despite script error (BRAINSTORM §2.9)
       }
  4. hook-presence check:
       if !vm.HasGlobalFunc("envoy_on_request") {
         return Continue  // hook not defined; pass-through (matches D1 anticipated PARSE-REJECT-at-parse-time disposition closed at Task 2)
       }
  5. invoke envoy_on_request hook:
       cc.stats.executions++  // upstream-parity: increments per invocation, not per-success
       if err := vm.CallGlobal("envoy_on_request", lua.LValue(reqUd)); err != nil {
         cc.stats.errors++
         logError("lua: envoy_on_request failed: %v", err)
         // continue dispatch despite script error
       }
  6. respond-state check:
       if f.respondCaptured != nil {  // :respond() fired
         cc.stats.respondCalls++
         f.dcb.SendLocalReply(
           f.respondCaptured.status,
           f.respondCaptured.body,
           f.respondCaptured.headers,
         )
         return StopIteration  // SendLocalReply continues encode-chain at filter[len-1] per ADR-0075
       }
       return Continue  // no respond; pass through to next filter
```

The `EncodeHeaders` symmetric shape mirrors decode, with:
- `response_handle` userdata + metatable (separate from request_handle; `:respond()` is overridden to raise the AMEND-8 runtime-error string `"respond not currently supported in the response path"`).
- `:status` validation `[200,600)` applies per AMEND-8.
- After CallGlobal, no respond-state handling on encode side (`:respond()` always errors).

The `requestHandleContext` Go struct (per IMPL Task 6 + §6 Task 9 spec):
```go
type requestHandleContext struct {
    filter  *filter          // back-ref for stats + respond-state capture
    headers http.Header      // mutable; bridge methods mutate via http.Header.Set/Add/Del
    cb      http.DecoderFilterCallbacks
}

type responseHandleContext struct {
    filter  *filter
    headers http.Header
    cb      http.EncoderFilterCallbacks
}

type respondState struct {
    status  uint32
    body    []byte
    headers http.Header  // ordered via the SendLocalReply ordered-headers path per types.go OrderedHeaders carrier
}
```

The bridge method implementations (per §6 Task 6-9 IMPL detail) operate on `requestHandleContext` via `lua.CheckUserData(state, 1) → cast → *requestHandleContext` at each method entry point. The `__pairs` metamethod snapshots the headers map into a sorted slice + iterates by integer index (per §11.2 D7 resolution; see §11 below).

---

## 5. Per-route discipline — PARSE-REJECT at 22.1; 9th canonical at 22.3

Per parent SPEC §5.2 + §6.2 arm 18 + AMEND-4. At 22.1, `LuaPerRoute` (the 3-arm oneof `disabled` / `name` / `source_code`) PARSE-REJECTs at any tier (Route / VirtualHost / RouteConfiguration / listener-typed_per_filter_config) via the existing HCM `RegisterPerRouteValidator` hook per phase-20 §5.2 + ADR-0110 single-chokepoint discipline. Wording: `"lua: per-route configuration is not yet supported (lands in phase 22.3)"`.

**22.3 forward-pointer:** the NEW 9th canonical per-route shape (3-arm hybrid `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override`) + the SHARED-stats discipline + the ADR-0125 §(xiv) IN-PLACE AMENDMENT (roster 8 → 9) lands at 22.3 IMPL. The AMENDMENT-anticipation paragraph anchored at parent SPEC commit (parent §4.5) STANDS UNCHANGED at this 22.1 SPEC commit.

**Phase 22.1 lands NO ADR-0125 amendment paragraph + NO per-route IMPL code beyond the PARSE-REJECT chokepoint.** Per-route validator registration at `lua.New` time per IMPL Task 1 (one-line `reg.RegisterPerRouteValidator(filterName, validatePerRouteLua)` registration; the validator function is a one-liner returning the arm-18 error).

---

## 6. Per-task IMPL breakdown (16 tasks across 5 tiers)

Per parent SPEC §3.0 estimate (~14-18 tasks). 16 tasks across 5 tiers. Each task sized for ~150-300 LoC production + tests, fitting cleanly under ADR-0045 per-task gate. Per-task PROGRESS.md entry shape per phase-21 IMPL precedent (one entry per Task at completion, with quoted command outputs per `superpowers:verification-before-completion` discipline). Test-driven discipline per `superpowers:test-driven-development` on every task.

### Tier A — scaffold (Tasks 1-5)

**Task 1: Package skeletons (NEW `internal/lua/` + NEW `internal/filter/http/lua/`).** Creates:
- `internal/lua/doc.go` — package overview + AMEND-1 sandbox-strict rationale + AMEND-9 gopher-lua-vs-LuaJIT divergence cross-refs + API surface summary. Cross-references parent SPEC + this 22.1 SPEC.
- `internal/lua/vm.go` — VM type + skeleton (NewVM + State + Close stubs returning zero values; full IMPL at Task 5). Public surface per §3.1 production signatures.
- `internal/lua/compile.go` — Chunk + CompileCache + NewCompileCache stubs (full IMPL at Task 4).
- `internal/lua/sandbox.go` — SandboxConfig type + per-stdlib defaults (full IMPL at Task 5).
- `internal/lua/vm_test.go` + `compile_test.go` + `sandbox_test.go` — skeleton tests for the type assertions + minimal smoke tests.
- `internal/filter/http/lua/doc.go` — package overview + Q1-Q12 BRAINSTORM decision summary + AMEND-1..AMEND-12 cross-references + D1+D5+D7 cross-refs + API surface summary.
- `internal/filter/http/lua/lua.go` — filter struct + factory (HTTPFilterFactory) skeleton + filterStats stub + TypeURL + filterName + per-route validator registration. Full IMPL at Task 10.
- `internal/filter/http/lua/lua_test.go` — skeleton tests.
- `internal/filter/http/lua/perroute.go` (if needed) — one-liner per-route validator function for arm-18.
- `go.mod` + `go.sum` updates — add `github.com/yuin/gopher-lua v1.1.2` dependency.

Acceptance: `go build ./internal/lua/... ./internal/filter/http/lua/...` clean; package files compile; skeleton tests pass; `go mod tidy` clean.

**Task 2: `compiled_config.go` + 18-arm PARSE-REJECT roster + D1 closure.**
- `internal/filter/http/lua/compiled_config.go` — config parse implementation; 18-arm PARSE-REJECT roster per parent §6.2; byte-stable error wording per parent §6.1.
- `internal/filter/http/lua/compiled_config_test.go` — 18-arm table-driven tests; each arm exercises trigger condition + asserts byte-exact wording match.
- **D1 closure first action**: scrape upstream Envoy v1.37.2 `source/extensions/filters/http/lua/config.cc::createFilterFactoryFromProtoTyped` + `lua_filter.cc::Filter` constructor against the anticipated PARSE-REJECT-both disposition for arms 5 + 17. If anticipated holds: arms 5 + 17 STAND; close D1 at Task 2 PROGRESS.md entry with empirical evidence + cite. If REFUTED (upstream allows no-op): arms 5 + 17 flip to silent no-op (degraded pass-through); update parent §6.2 with the corrected disposition + author ADR-0044 escape-valve at ADR-0190 OR collapse into ADR-0189 §Decision body (likely the latter — D1 disposition is local-IMPL detail, not framework-changing).

Acceptance: 18-arm table-driven tests all pass; D1 PROGRESS.md entry quotes upstream config.cc + lua_filter.cc constructor evidence; ADR-0189 §Decision body draft updated with D1 disposition.

**Task 3: `datasource.go` — 4-arm DataSource resolution.**
- `internal/filter/http/lua/datasource.go` — 4-arm DataSource arm dispatch (Filename / InlineBytes / InlineString / EnvironmentVariable); WatchedDirectory PARSE-REJECT; empty-oneof PARSE-REJECT; per-arm empty-content PARSE-REJECTs.
- `internal/filter/http/lua/datasource_test.go` — 4-arm + 10-rejection-leaf table-driven tests per parent §11.1 + AMEND-5. File-read failures (ENOENT / EACCES / EISDIR) exercised via `t.TempDir()` synthetic files.

Acceptance: 4-arm DataSource resolution returns expected bytes; 10 rejection leaves table-driven tests all pass with byte-exact wording per parent §6.2 arms 6-15.

**Task 4: `internal/lua/compile.go` IMPL — Chunk + CompileCache + CompileScript.**
- Full IMPL of `Chunk` (wraps `*lua.FunctionProto` + sha256 hash); `CompileCache` (sync.RWMutex + map[hash]*Chunk); `NewCompileCache`; `CompileScript(src, cache)` with sha256 content-hash keying + cache hit/miss logic + cache nil-tolerance per ADR-0085.
- `internal/lua/compile_test.go` — cache hit-on-same-content-hash; cache-miss-on-different-source; concurrent-read/add tests via `t.Parallel()` + goroutines; nil-cache behavior verification.

Acceptance: Cache hit/miss behavior correct; concurrent-read tests race-free (`go test -race`); cache nil-tolerance verified.

**Task 5: `internal/lua/vm.go` + `internal/lua/sandbox.go` IMPL — VM lifecycle + sandbox.**
- Full IMPL of NewVM (sandbox-config-driven per-stdlib `OpenXxx` selective + post-walk nil-out per §3.3 implementation discipline); RegisterGlobalFunc; Run (PCall the chunk's FunctionProto onto LState); HasGlobalFunc (GetGlobal name + LFunction-type check); CallGlobal (PCall with args); Close (idempotent state.Close); panic-wrapper (recover() in PCall path; calls PanicHandler; converts to error return); BasePrintSink redirection (rebind `print` global to a Go function that writes to sink-or-drops).
- `internal/lua/sandbox.go` — full per-stdlib disposition table implementation per §3.3.
- `internal/lua/vm_test.go` — VM lifecycle table-driven tests; option application verification; RegisterGlobalFunc behavior; Run / HasGlobalFunc / CallGlobal table-driven tests; panic-wrapper behavior tests; concurrency tests (parallel VM construction + per-VM operation independence).
- `internal/lua/sandbox_test.go` — per-stdlib ALLOW/DENY exhaustive verification per §3.3 roster; verifies `dofile`/`loadfile`/`loadstring`/`load`/`module`/`require`/`io.open`/`io.popen`/`os.execute`/`os.exit`/`debug.getupvalue`/`channel.make`/`package.path` are nil-or-runtime-error post-sandbox-strict construction.

Acceptance: VM lifecycle correct; sandbox roster fully enforced; race tests clean (`go test -race`); panic-wrapper correctly converts Go panics to error returns.

### Tier B — bridge methods (Tasks 6-9)

**Task 6: `bridge.go` headers + `__pairs` (alphabetical-snapshot per §11 D7).**
- `internal/filter/http/lua/bridge.go` — `request_handle` userdata + metatable setup (LUserData wrapping `*requestHandleContext`; metatable __index → table of bridge methods; metatable __pairs → alphabetical-snapshot iterator).
- 7 headers-object methods: `:get(name)` returns first value or nil; `:getAtIndex(name, idx)` returns N-th value or nil (1-indexed per Lua convention); `:getNumValues(name)` returns count; `:add(name, val)` appends; `:append(name, val)` (alias for :add per upstream Envoy semantics); `:remove(name)` deletes; `:replace(name, val)` removes-then-adds.
- `__pairs` metamethod: snapshots `http.Header` map into `[]struct{k,v string}` sorted alphabetically by k (case-insensitive sort via `strings.ToLower` then byte-compare); returns a stateful iterator function that walks the slice by integer index. Discipline per §11 D7 resolution.
- `internal/filter/http/lua/bridge_test.go` — table-driven tests for each headers method; `__pairs` alphabetical-snapshot verification; cross-run determinism test (run __pairs N times against same headers; assert same order).

Acceptance: 7 headers methods byte-compatible with upstream `wrappers.cc` semantics (case-insensitive name lookup; add appends; remove deletes; replace removes-then-adds); `__pairs` alphabetical-deterministic across runs.

**Task 7: `bridge.go` log methods (6 `:logXxx`).**
- `bridge.go` extension — 6 `:logTrace`/`:logDebug`/`:logInfo`/`:logWarn`/`:logErr`/`:logCritical` methods. Each wraps the Go stdlib `"log"` package (the canonical project log sink used by existing filters per `extauthz.go:18` + `extproc.go:26` + `rbac.go:6` + `router_h2.go:7` + `extproc/processor.go:52`) at the corresponding log level. Format: `"lua: <msg>"` prefix preserved across all 6 levels for log-greppability. Log-level mapping (gopher-lua bridge name → Go log call): `:logTrace`/`:logDebug` → `log.Printf("DEBUG lua: %s", msg)`; `:logInfo` → `log.Printf("INFO lua: %s", msg)`; `:logWarn` → `log.Printf("WARN lua: %s", msg)`; `:logErr` → `log.Printf("ERROR lua: %s", msg)`; `:logCritical` → `log.Printf("CRIT lua: %s", msg)`. Conservative mapping to stdlib-`log` (which has no native levels); future log-leveling primitive (if introduced cross-project) replaces verbatim per its own ADR.
- `bridge_test.go` extension — table-driven log-method tests using a captured-log-sink test double; verifies each method calls into the right log level + format.

Acceptance: 6 log methods route to correct log levels; format pin preserved.

**Task 8: `bridge.go` streamInfo subset (4 methods).**
- `bridge.go` extension — `request_handle:streamInfo()` returns a streamInfo userdata; 4 methods on the userdata: `:protocol()` returns "HTTP/1.0" / "HTTP/1.1" / "HTTP/2" / "HTTP/3" depending on stream protocol; `:routeName()` returns the resolved route name string (from filter callback `cb.RouteName()` or equivalent); `:downstreamLocalAddress()` returns "ip:port" formatted local address; `:downstreamDirectRemoteAddress()` returns "ip:port" formatted remote address.
- `bridge_test.go` extension — table-driven tests for each streamInfo method using a test-double `DecoderFilterCallbacks` carrying canned protocol/route/address values.

Acceptance: 4 streamInfo methods return correctly-formatted strings; protocol mapping covers all 4 HTTP versions.

**Task 9: `bridge.go` respond + `decode_headers.go` + `encode_headers.go`.**
- `bridge.go` extension — `request_handle:respond(headers_table, body_string)` implementation per parent §11.6.7 byte-pin: extract `:status` from headers_table (raise byte-exact `":status must be between 200-599"` if outside `[200,600)` per AMEND-8); auto-set `content-length` from body size (if body present); apply `content-type: text/plain` default if headers_table did not supply content-type (per upstream `Utility::prepareLocalReply` at `utility.cc:1241,1273`); capture `respondState` on filter for the decode path to read. `response_handle:respond()` (encode side) raises byte-exact `"respond not currently supported in the response path"` per AMEND-8.
- `internal/filter/http/lua/decode_headers.go` — `DecodeHeaders(headers, endStream)` per §4.3 dispatch; integrates with respond-state to fire `cb.SendLocalReply` if `:respond()` was called.
- `internal/filter/http/lua/encode_headers.go` — `EncodeHeaders(headers, endStream)` symmetric to decode; fires `envoy_on_response` hook; AMEND-8 runtime-reject from response handle.
- `bridge_test.go` extension — `:respond()` full byte-pin verification per parent §11.6.7 (4-tuple {status, content-length: 6, content-type: text/plain, body: "denied"}); `:status` range validation; encode-side `:respond()` runtime-reject byte-exact string verification.
- `lua_test.go` extension — DecodeHeaders + EncodeHeaders integration tests with end-to-end flow (compile script → VM construct → Run → CallGlobal → respond-state check).

Acceptance: respond byte-pin matches parent §11.6.7 verbatim; `:status` validation byte-exact; encode-side runtime-reject byte-exact; decode + encode integration end-to-end correct.

### Tier C — stats + tests (Tasks 10-12)

**Task 10: `stats.go` + boot-registration at `cmd/envoy-go/main.go`.**
- `internal/filter/http/lua/stats.go` — 3-counter stat surface (`errors` + `executions` + `respondCalls`) registration under HCM-rooted template `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` per parent §7.2 + AMEND-2. `New()` time stat registration via `newFilterStats(reg, baseStatPrefix(ctx.StatPrefix))` mirroring jwt_authn + ext_authz + ext_proc + adaptive_concurrency unconditional-allocation discipline; per ADR-0085 nil-tolerance: guard `if ctx.Stats != nil`.
- `cmd/envoy-go/main.go` extension — `httpReg.Register(lua.TypeURL, lua.New)` insertion alphabetical between `localratelimit.New` and `oauth2.New` per ADR-0100 §2.2 (one-line addition + import update).
- `internal/filter/http/lua/lua_test.go` extension — stats registration verification; empty `Lua.stat_prefix` consecutive-dot wire name verification (per AMEND-2 `http.<HCM>.lua..errors` literal); cardinality assertion (3 counters registered per filter instance).

Acceptance: 3 counters registered under correct HCM-rooted template; empty-stat-prefix consecutive-dot wire shape verified; boot-registration insertion alphabetical-correct + cmd/envoy-go builds.

**Task 11: `fuzz_test.go` — 28th project-wide fuzzer `FuzzLuaConfigParse`.**
- `internal/filter/http/lua/fuzz_test.go` — standard ADR-0018 baseline fuzzer. Corpus seeds (~30 total): one seed per PARSE-REJECT arm per parent §6.2 (18 arms × 1 seed each = 18 seeds) + ~5 valid-config seeds (each DataSource arm with valid contents) + ~7 adversarial-Lua-source seeds (syntax errors triggering arm 16 compile failures; sandbox-breaking attempts that should still compile but error at runtime — must-never-panic invariant).
- Must-never-panic invariant per ADR-0018: fuzzer body parses + calls `New()` via test-double `FilterFactoryContext`; no panic regardless of input; PARSE-REJECT errors are expected behavior, not panic.

Acceptance: `FuzzLuaConfigParse` at 30s baseline returns no crashes; corpus seeds exercise all 18 PARSE-REJECT arms + valid + adversarial paths.

**Task 12: Race + concurrency tests at `internal/lua/` + `internal/filter/http/lua/`.**
- `internal/lua/vm_test.go` extension — concurrent-VM-construction tests: N goroutines each call `NewVM(opts...)` + `Run(chunk)` + `CallGlobal(name, args...)` + `Close()` against the same `*Chunk` from the same `*CompileCache`; assert no cross-VM state leak; race-free under `-race`.
- `internal/lua/compile_test.go` extension — concurrent-read/add tests against `CompileCache`: N goroutines mix read (CompileScript with same hash) and add (CompileScript with new content) operations; assert no data race + correct cache contents.
- `internal/filter/http/lua/lua_test.go` extension — concurrent per-stream filter dispatch tests via test-double `DecoderFilterCallbacks`: N goroutines each instantiate a per-stream filter + drive DecodeHeaders + assert per-stream VM independence.

Acceptance: All race tests clean under `go test -race -count=10` (10 iterations to catch flakes); no cross-stream state leak.

### Tier D — differential fixture (Tasks 13-15)

**Task 13: NEW `BackendKind=HTTPLua` + `BootRejectFixture` infrastructure.**
- `test/differential/runner_test.go:547` extension — add `BackendKind=HTTPLua` switch-case mirroring `HTTPCsrf` / `HTTPCompressor` / `HTTPAdaptiveConcurrency` precedent per AMEND-11. ~20 LoC delta.
- `test/differential/harness.go` extension — NEW OPTIONAL `BootRejectFixture` driver interface per parent §13-R1; `tryStartReferenceProxy(ctx, fix) (cancel func(), stderrBuf *bytes.Buffer, err error)` + `tryStartSubjectProxy` variants (paralleling `StartReferenceProxy` / `StartSubjectProxy` but returning the boot error + stderr buffer instead of `t.Fatalf`-ing). ~80-130 LoC delta total per parent §11.7.3.
- `test/differential/runner_test.go` extension — NEW `runBootRejectFixture` branch paralleling `runReferenceLessFixture` at `runner_test.go:1268`. Asserts both sides exit non-zero AND both sides' stderr contains the substring `"script load error"`. ~50 LoC delta.

Acceptance: `BackendKind=HTTPLua` switch-case lands; `BootRejectFixture` interface signature defined + harness `tryStart*` variants implemented + runner `runBootRejectFixture` branch implemented; harness tests pass.

**Task 14: fixture-0026 directory + `scripts/` + 7 `.lua` sources.**
- `test/fixtures/0026-http-lua-headers-bridge/README.md` (~150-250 lines) — scope + 7-scenario table + topology + cross-refs to parent SPEC §8 + this 22.1 SPEC §9 + ADR-0188 + ADR-0189.
- `test/fixtures/0026-http-lua-headers-bridge/envoy.yaml` — reference Envoy bootstrap; single listener + lua filter consuming `Lua.DefaultSourceCode` via `Filename` arm pointing to `scripts/<scenario>.lua`; templated `{{.BackendPort}}`.
- `test/fixtures/0026-http-lua-headers-bridge/envoy-go.yaml` — subject bootstrap; same topology; templated `{{.AdminPort}} {{.ListenerPort}} {{.BackendPort}} {{.FixtureDir}}`.
- `test/fixtures/0026-http-lua-headers-bridge/expectations.yaml` — human-readable declarative scenario expectations (NOT consumed by runner; documentation aid).
- `test/fixtures/0026-http-lua-headers-bridge/inputs/driver.go` (~400-600 LoC) — registered `Driver` impl + `BootRejectFixture` impl for scenario (g); per-scenario probes via `driveProxy` + `emitScenario` + `classifyBody` mirroring fixture-0023's pattern.
- `test/fixtures/0026-http-lua-headers-bridge/scripts/a_add_header.lua` — `function envoy_on_request(rh) rh:headers():add("x-lua-injected", "hello") end`
- `test/fixtures/0026-http-lua-headers-bridge/scripts/b_replace_header.lua` — `function envoy_on_request(rh) rh:headers():replace("user-agent", "envoy-go-lua/1.0") end`
- `test/fixtures/0026-http-lua-headers-bridge/scripts/c_remove_header.lua` — `function envoy_on_request(rh) rh:headers():remove("x-blocked") end`
- `test/fixtures/0026-http-lua-headers-bridge/scripts/d_respond.lua` — `function envoy_on_request(rh) rh:respond({[":status"]="403"}, "denied") end`
- `test/fixtures/0026-http-lua-headers-bridge/scripts/e_log_only.lua` — `function envoy_on_request(rh) rh:logInfo("lua hit") end`
- `test/fixtures/0026-http-lua-headers-bridge/scripts/f_headers_iter.lua` — `function envoy_on_request(rh) local n=0; for k,v in pairs(rh:headers()) do n=n+1 end; rh:headers():add("x-headers-count", tostring(n)) end`
- `test/fixtures/0026-http-lua-headers-bridge/scripts/g_compile_error.lua` — intentional syntax error (e.g., `function envoy_on_request(rh) end syntax-error-here-not-valid-lua`)

Acceptance: fixture-0026 directory layout matches parent §8.4 + this 22.1 SPEC §9; 7 `.lua` files committed; driver.go scenario-probe shape mirrors fixture-0023.

**Task 15: envoy-go-side `"script load error: "` wording-pinning + fixture-0026 green-light.**
- `cmd/envoy-go/main.go` extension — wrap gopher-lua's compile error with `"script load error: "` prefix at boot-reject path (`main.go:60-66` per parent §13-W). ~50 LoC delta (error-wrapping helper + integration with the config-load PARSE-REJECT path that surfaces from `lua.New`'s arm 16). The wrapping ensures the boot-reject stderr contains the literal `"script load error"` substring matching upstream Envoy v1.37.2 per parent §11.7.5.
- Run fixture-0026 via `go test ./test/differential -run TestDifferential/0026`: assert 6 scenarios (a)-(f) full cross-side byte-exact via existing `CompareBytes`; scenario (g) substring-match `"script load error"` via the NEW `BootRejectFixture` interface from Task 13; full fixture GREEN.

Acceptance: fixture-0026 GREEN at IMPL Task 15; envoy-go-side boot-reject stderr contains `"script load error"` for the scenario (g) g_compile_error.lua input; cross-side byte-exact for the 6 wire-interactive scenarios.

### Tier E — atomic landing (Task 16)

**Task 16: BEHAVIOR_CONTRACT.md 7-edit bundle + ADR-0188/0189 §Decision+§Consequences body landings + STATE.md re-advance + ROADMAP row 22.1 per-cell IMPL-done annotation.**

Per ADR-0052 atomic landing discipline + parent SPEC §14 7-edit bundle:

1. **NEW `### envoy.filters.http.lua` subsection** under BEHAVIOR_CONTRACT.md §9 family filter documentation (~80-120 lines). Headers-bridge-focused for 22.1; carries forward-pointers to 22.2 (full bridge) + 22.3 (multi-script + per-route).
2. **Stat-table 99 → 102 extension** under BEHAVIOR_CONTRACT.md `## Stat surface` (the table at line ~340-367). 3 new rows under `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` template — `errors` (counter; upstream-parity per AMEND-3), `executions` (counter; upstream-parity per AMEND-3), `respond_calls` (counter; envoy-go-strict extension per AMEND-3). ~3-line table-row insertion + extension summary paragraph mirroring phase-21's `## Phase 21 extension — 92 → 99 internal names` paragraph at line 367.
3. **envoy-go-strict departure record #1: stdlib-sandbox-strict** (per AMEND-1). NEW row at BEHAVIOR_CONTRACT.md envoy-go-strict departures section. Rationale: per-stream goroutine dispatch model cannot make the per-worker-VM-scoping assumption upstream relies on.
4. **envoy-go-strict departure record #2: `respond_calls` counter** (per AMEND-3 corrected from BRAINSTORM 2-record bundle). NEW row. Operator-visibility for `:respond()` short-circuit rate.
5. **envoy-go-strict departure record #3: runtime-error log-message wording** (per AMEND-9). NEW row. gopher-lua's `[string "chunk"]:line: msg` format diverges from LuaJIT's `chunk:line: msg`; only the envoy log shows the divergent wording.
6. **NEW `### Phase 22.1 forward-pointer notes` subsection** (~30-50 lines). Documents 22.2-anticipated additions (httpCall counters; body-bridge methods; dynamic-metadata-bridge disposition forward-pointer) + 22.3-anticipated additions (NEW 9th canonical per-route shape ADR + ADR-0125 §(xiv) AMENDMENT body + `Lua.SourceCodes` map activation).
7. **Per-route-canonical cross-reference caption update** at the per-route canonical paragraph. 1-line edit (cross-reference to ADR-0125 §(xiv) AMENDMENT-anticipation paragraph).

Plus:

- **ADR-0188 §Decision + §Consequences body landing in DECISIONS.md** per parent SPEC §4.1 API-REVISION-ALLOWANCE clause + this 22.1 SPEC §3.1 production signatures + D5 + D7 resolutions.
- **ADR-0189 §Decision + §Consequences body landing in DECISIONS.md** per parent SPEC §4.4 file split + this 22.1 SPEC §3.5 file split + §4 compiledConfig + filterStats shape + §6.2 18-arm PARSE-REJECT roster + §8 fixture-0026 disposition + §13-R1 BootRejectFixture infrastructure + this 22.1 SPEC §11 D5 + D7 resolutions + Task 2 D1 closure evidence.
- **STATE.md re-advance** to `phase 22.1 IMPL done; awaiting 22.2 SPEC` per BOOTSTRAP §4.1 invariant 1 + 99 → 102 stat count update + 16 → 17 HTTP filter count update + ADR tail advance to ADR-0189 (or ADR-0190 if R6 escape-valve fires) + next-free ADR update.
- **ROADMAP row 22.1 per-cell IMPL-done annotation** per ADR-0106 (flip `planned → done`; record IMPL outcomes — stat count delta, fuzzer count, fixture-0026 GREEN status, ADR-0188 + ADR-0189 §Decision-body-landed evidence, departure records, etc.).
- **ADR-0190 §Context anchor if R6 escape-valve fires** (only if `*LState`-pool benchmark > 1ms threshold per §13-R6); §Decision + §Consequences body lands at the same Task 16 commit per ADR-0044.

Acceptance: All 7 BEHAVIOR_CONTRACT.md edits land in one atomic commit; ADR-0188 + ADR-0189 §Decision + §Consequences bodies complete; STATE.md re-advance reflects post-22.1-IMPL state; ROADMAP row 22.1 flipped done; per-task PROGRESS.md entries complete across all 16 tasks; REVIEW.md authored per `superpowers:requesting-code-review`. Phase 22.1 IMPL phase-done.

---

## 7. PARSE-REJECT roster (cross-reference parent §6 + 22.1-specific row notes)

The parent SPEC §6.2 enumerates the 18-arm 22.1 PARSE-REJECT roster verbatim. This 22.1 SPEC INHERITS the parent §6.2 table without modification and uses the parent table as the authoritative source for IMPL Task 2 (`compiled_config.go` table-driven tests).

**Row-by-row dispositions at 22.1 SPEC commit:**

- **Arms 1-2 (`typed-config-required` + `typed-config-unmarshal`)** — universal-to-every-filter baseline per ADR-0080 byte-stable wording. STAND at 22.1 IMPL.
- **Arm 3 (`inline-code-deprecated-rejected`)** — envoy-go-strict deprecated-field-rejection per AMEND-6. STANDS at 22.1 IMPL. NOT a BEHAVIOR_CONTRACT.md departure record (project-wide deprecated-field-rejection discipline per ADR-0080-cluster references).
- **Arm 4 (`source-codes-deferred-to-22-3`)** — deferred wording per Q1+Q2 envelope D + 3-way split. STANDS at 22.1 IMPL.
- **Arm 5 (`default-source-code-required`)** — subject to §12-D1 disposition. ANTICIPATED PARSE-REJECT at IMPL; D1 closure at Task 2 first action.
- **Arms 6-15 (DataSource 10-arm dispatch + per-arm empty-content checks)** — STAND at 22.1 IMPL per AMEND-5 10-arm refinement.
- **Arm 16 (`script-compile-failed`)** — wraps gopher-lua `*lua.ApiError` per parent §6.2 note. STANDS at 22.1 IMPL.
- **Arm 17 (`script-missing-required-hooks`)** — subject to §12-D1 disposition (same empirical-pin as arm 5). ANTICIPATED PARSE-REJECT at IMPL; D1 closure at Task 2 first action.
- **Arm 18 (`per-route-deferred-to-22-3`)** — via HCM `RegisterPerRouteValidator` hook per phase-20 §5.2 + ADR-0110 single-chokepoint. STANDS at 22.1 IMPL; per-route validator function is one-liner returning the arm-18 error.

**RETRACTED sandbox-violation arm** per parent §6.2 + AMEND-6. gopher-lua does NOT have a static-AST sandbox-scan; deny-list enforced at VM-construction time per §3.3. NO `sandbox-violation` arm in the 22.1 roster.

**22.3 forward-pointer arms** per parent §6.3 — ~20 arms anticipated; settled at 22.3 IMPL. NOT 22.1 scope.

---

## 8. Stat surface (cross-reference parent §7)

The parent SPEC §7 enumerates the 3-counter 22.1 stat surface verbatim:

| # | Internal name | Type | Source | Description |
|---|---|---|---|---|
| 1 | `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.errors` | counter | filter | Script execution errors (panic-wrapper catches; upstream-parity per AMEND-3 + `lua_filter.cc:811`) |
| 2 | `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.executions` | counter | filter | Script invocations (every `envoy_on_request`/`envoy_on_response` call; upstream-parity per AMEND-3 + `lua_filter.cc:872`) |
| 3 | `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.respond_calls` | counter | filter | `:respond()` short-circuit invocations; envoy-go-strict extension per AMEND-3; departure record at BEHAVIOR_CONTRACT.md §13.6 row 2 per §14 edit #4 |

Stat-prefix template `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` per parent §7.2 + AMEND-2 (HCM-rooted; matches phase-09/12/14/16/17/18/19/20/21 dominant §9 pattern). Empty `Lua.stat_prefix` produces literal consecutive-dot names (`http.<HCM>.lua..errors`) mirroring phase-14 compressor empty-`<library>` precedent at BEHAVIOR_CONTRACT.md §line 243.

**Project stat-count delta: 99 → 102 at 22.1 IMPL (+3).** Arithmetic UNCHANGED from BRAINSTORM §5.3 even after AMEND-3 executions reclassification. 22.2 anticipated additions: likely +2 httpCall counters (22.2 SPEC settles). 22.3 anticipated additions: 0 (SHARED-vacuous per the 9th canonical's SHARED-stats discipline).

**1 envoy-go-strict departure record** (corrected from BRAINSTORM 2-record bundle per AMEND-3) — `respond_calls` extension at BEHAVIOR_CONTRACT.md §13.6 row 2 per §14 edit #4.

---

## 9. Differential fixture-0026 (cross-reference parent §8 + 22.1-specific scenario detail)

The parent SPEC §8 enumerates the fixture-0026 disposition verbatim. This 22.1 SPEC INHERITS parent §8.1-§8.5 + concretizes the per-scenario `.lua` source contents + the `BootRejectFixture` driver interface signature.

### 9.1 7-scenario table per parent §8.2

| # | Name | Lua script behavior | Wire-output assertion (cross-side) |
|---|---|---|---|
| (a) | add-fixed-header | `function envoy_on_request(rh) rh:headers():add("x-lua-injected", "hello") end` | Request header `x-lua-injected: hello` present at upstream echobackend (driver asserts substring in reflected JSON body) |
| (b) | replace-header | `function envoy_on_request(rh) rh:headers():replace("user-agent", "envoy-go-lua/1.0") end` | Reflected `user-agent: envoy-go-lua/1.0` |
| (c) | remove-header | `function envoy_on_request(rh) rh:headers():remove("x-blocked") end` | Reflected request without `x-blocked` header |
| (d) | respond-shortcircuit | `function envoy_on_request(rh) rh:respond({[":status"]="403"}, "denied") end` | Client receives full byte-pinned tuple: status `403 Forbidden`; `content-length: 6`; `content-type: text/plain`; body `denied` (6 bytes verbatim, no trailing newline); no upstream request initiated. Per parent §11.6.7 + AMEND-7. |
| (e) | log-only-passthrough | `function envoy_on_request(rh) rh:logInfo("lua hit") end` | Reflected request unchanged at upstream; **stat-counter delta** `lua.<prefix>.executions` increments per probe. Per §12-D3 RECOMMENDED option (a) — stat-counter IS the "Lua ran" assertion; literal log line is supplementary, NOT cross-side asserted. |
| (f) | headers-iteration | `function envoy_on_request(rh) local n=0; for k,v in pairs(rh:headers()) do n=n+1 end; rh:headers():add("x-headers-count", tostring(n)) end` | Reflected header `x-headers-count: N` where N is the probe's request-header count. **Count-only deterministic per §11 D7 resolution** — iteration order is alphabetical via the bridge `__pairs` snapshot, but the script counts only; order-independent assertion. |
| (g) | script-compile-error | Lua source with intentional syntax error (`function envoy_on_request(rh) end syntax-error-here-not-valid-lua`) | **Both sides exit non-zero at config-load + both sides' stderr contains literal substring `"script load error"`** per parent AMEND-10 option 2. NOT byte-exact stderr. Via NEW `BootRejectFixture` driver interface from Task 13. |

### 9.2 Driver impl shape (per parent §11.7.2)

The `inputs/driver.go` impl mirrors fixture-0023's pattern:

```go
type driver struct{ /* ... */ }

func (d *driver) DriveReference(ctx context.Context, ref ProxyAddr) ([]byte, error) {
    return driveProxy(ctx, ref, d.scenarioSet())
}

func (d *driver) DriveSubject(ctx context.Context, subj ProxyAddr) ([]byte, error) {
    return driveProxy(ctx, subj, d.scenarioSet())
}

func driveProxy(ctx context.Context, addr ProxyAddr, scenarios []scenario) ([]byte, error) {
    var buf bytes.Buffer
    for _, sc := range scenarios {
        result := executeScenario(ctx, addr, sc)
        emitScenario(&buf, sc.id, result)
    }
    return buf.Bytes(), nil
}

func emitScenario(buf *bytes.Buffer, id string, r scenarioResult) {
    fmt.Fprintf(buf, "scenario %s status=%d body=%s\n", id, r.status, classifyBody(r.body))
}

// classifyBody insulates from non-substantive divergences (e.g., upstream's
// date: header in reflected JSON; envoy-go's lack thereof) by emitting
// substring-classified body summaries rather than verbatim bytes.
func classifyBody(body []byte) string { /* ... */ }
```

The driver also implements `BootRejectFixture` (per Task 13 + parent §13-R1) for scenario (g):

```go
func (d *driver) BootRejectScript() string {
    // returns path to scripts/g_compile_error.lua relative to fixture dir
    return "scripts/g_compile_error.lua"
}

func (d *driver) ExpectedBootErrorSubstring() string {
    return "script load error"
}
```

### 9.3 envoy.yaml + envoy-go.yaml topology

Single listener at `{{.ListenerPort}}` → HCM with single HTTP route (`/`) → cluster `c_echobackend` targeting the shared `test/helpers/echobackend/cmd/echobackend/` (reflects request headers as JSON body — needed for scenarios (a) + (b) + (c) + (f)). The HCM filter chain contains the `envoy.filters.http.lua` filter consuming `Lua.DefaultSourceCode.Filename` pointing to `{{.FixtureDir}}/scripts/<scenario>.lua` + the terminal `envoy.filters.http.router`.

For scenario (g) g_compile_error.lua: the bootstrap is the same as the other scenarios but the `Filename` points to the intentionally-broken Lua source; the boot-reject path fires at config-load before the listener binds; the test asserts non-zero exit + stderr substring match.

### 9.4 `scripts/` subdirectory rationale (per AMEND-11)

Per parent §8.4 + AMEND-11. The `scripts/` subdirectory exploits the DataSource `Filename` arm naturally (vs all-inline-strings collapsed into the YAML) — adds DataSource-arm coverage for free + improves per-scenario readability. Mirrors the fixture-0019 PKI-subdir pattern. Driver's `SubjectConfig` templates the path as `{{.FixtureDir}}/scripts/<scenario>.lua` per the fixture-0019 precedent.

### 9.5 Backend reuse + `BackendKind=HTTPLua` (per AMEND-11)

NEW `BackendKind` constant `HTTPLua` at `test/differential/runner_test.go:547` per AMEND-11; ~20 LoC switch-case addition mirroring `HTTPCsrf`/`HTTPCompressor`/`HTTPAdaptiveConcurrency` precedent. REUSE shared `test/helpers/echobackend/cmd/echobackend/` (the shared 14+ helper).

---

## 10. Behavior-contract delta (cross-reference parent §9)

The phase-22.1 behavior-contract delta (per parent §9 — high-level semantic changes; the verbatim Markdown patch lives at §14):

1. **Lua-script-driven HTTP filter semantics** — NEW class of filter that delegates per-request behavior to operator-authored interpreted scripts (FIRST §9 row per parent §1). Observable: operator-supplied Lua source compiled at config-load (PARSE-REJECT on compile failure per §7 arm 16); per-request invocation of `envoy_on_request`/`envoy_on_response` hooks against bridge methods.
2. **Stdlib-sandbox-strict envoy-go-strict departure** (per AMEND-1). Recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures section per §14 edit #3.
3. **`respond_calls` envoy-go-strict counter** (per AMEND-3 corrected from BRAINSTORM §5.4 2-record bundle to 1-record bundle). Recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures section per §14 edit #4.
4. **Runtime-error log-message wording divergence** (per AMEND-9 RECOMMEND-DEPARTURE-RECORD). Recorded at BEHAVIOR_CONTRACT.md envoy-go-strict departures section per §14 edit #5.
5. **Scenario (g) substring-match cross-side claim** (per AMEND-10). Recorded at BEHAVIOR_CONTRACT.md phase-22-specific cross-side-equivalence carve-out (parent §13.7).
6. **Header `__pairs` iteration alphabetical-snapshot discipline** (per §11 D7 resolution; refines parent §11.2.3 sub-pin). The bridge `__pairs` metamethod snapshots `http.Header` map into an alphabetically-sorted slice + walks by integer index. Closes per-run map-iteration non-determinism for script-author debugging; matches `net/http.Header.Write` precedent. NOT a BEHAVIOR_CONTRACT.md departure record (this is project-internal discipline — upstream `HeaderMapWrapper::luaPairs` snapshots in HeaderMap-insertion-order; envoy-go snapshots in alphabetical-order; the divergence is observable only to script authors who depend on iteration order — for scenario (f) count-only assertion, the order is irrelevant).

---

## 11. SPEC-time empirical pins (resolved IN-SESSION)

This 22.1 SPEC author closed two of the parent SPEC §12 D-questions IN-SESSION at SPEC drafting time (D5 + D7), per the next-prompt-permitted optional SPEC-time D-question resolution. The remaining D-questions (D1 + D3) carry forward per §12 below.

### 11.1 D5 closure — 28th-fuzzer count CONFIRMED

**Question (per parent §12-D5 + §11.4.5):** BRAINSTORM §3.7 claims `FuzzLuaConfigParse` is the 28th project-wide fuzzer. Current `internal/filter/http/` count is 16. What is the actual project-wide fuzzer count post-phase-21?

**Empirical evidence (executed at this SPEC drafting session):**

```bash
$ find /home/esa/git/envoy-go -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' \
    | xargs grep -h '^func Fuzz' | sort -u | wc -l
27
```

Project-wide unique fuzzer count = **27** in the main tree (excluding stale `.claude/worktrees/phase-09-spec/` duplicates which predate post-phase-09 filter packages).

Per-fuzzer enumeration:
- `FuzzAccessLogFormat` (`internal/accesslog/`)
- `FuzzAdaptiveConcurrencyConfigParse` (`internal/filter/http/adaptive_concurrency/`)
- `FuzzBandwidthLimitConfigParse` (`internal/filter/http/bandwidthlimit/`)
- `FuzzBootstrapLoad` (`internal/bootstrap/`)
- `FuzzBufferConfigParse` (`internal/filter/http/buffer/`)
- `FuzzCheckResponseMapping` (`internal/filter/http/extauthz/` — 2nd in package)
- `FuzzCompressorConfigParse` (`internal/filter/http/compressor/`)
- `FuzzConfigDumpFormat` (`internal/admin/`)
- `FuzzCsrfPolicyConfigParse` (`internal/filter/http/csrf/`)
- `FuzzDrainTransitions` (`internal/drain/`)
- `FuzzExtAuthzConfigParse` (`internal/filter/http/extauthz/`)
- `FuzzExtProcConfigParse` (`internal/filter/http/extproc/`)
- `FuzzFaultConfigParse` (`internal/filter/http/fault/`)
- `FuzzFilterChainMatch` (`internal/listener/listenerfilter/`)
- `FuzzFilterChainParse` (`internal/filter/hcm/`)
- `FuzzFrameStream` (`internal/filter/hcm/h2/`)
- `FuzzHCMConfigParse` (`internal/filter/http/`)
- `FuzzHeaderMutationConfigParse` (`internal/filter/http/header_mutation/`)
- `FuzzHPACKDecode` (`internal/filter/hcm/h2/` — 2nd in package)
- `FuzzJwtAuthnConfigParse` (`internal/filter/http/jwtauthn/`)
- `FuzzLocalRateLimitConfigParse` (`internal/filter/http/localratelimit/`)
- `FuzzOAuth2ConfigParse` (`internal/filter/http/oauth2/`)
- `FuzzProcessingResponseMapping` (`internal/filter/http/extproc/` — 2nd in package)
- `FuzzPromTextFormat` (`internal/stats/`)
- `FuzzRBACConfigParse` (`internal/filter/http/rbac/`)
- `FuzzTcpProxyFilter` (`internal/filter/tcpproxy/`)
- `FuzzTLSContextParse` (`internal/tls/`)

= 27 unique. **`FuzzLuaConfigParse` will be the 28th project-wide fuzzer.**

**Resolution:** BRAINSTORM §3.7 claim CONFIRMED. ADR-0189 §Decision body at IMPL Task 16 + BEHAVIOR_CONTRACT.md §13.4 patch pin to **28**. §13-R4 RATIFIED-PENDING-IMPL-TIME item CLOSED at this SPEC commit (no IMPL-time re-verification needed; the resolution stands).

### 11.2 D7 closure — envoy-go headers-map type EMPIRICALLY-DETERMINED + `__pairs` discipline RATIFIED

**Question (per parent §12-D7 + §11.2.3 + §13-R3):** Per parent §11.2.3 + AMEND-9. Header `__pairs` iteration determinism is bridge-snapshot-driven AND relies on envoy-go's underlying headers-map having insertion-ordered iteration. Verify envoy-go's headers-map type.

**Empirical evidence (executed at this SPEC drafting session):**

The envoy-go filter callbacks pass `net/http.Header` as the headers carrier per `internal/filter/http/types.go:55`:

```go
type StreamDecoderFilter interface {
    DecodeHeaders(headers http.Header, endStream bool) FilterHeadersStatus
    ...
}
```

Where `http.Header` is `type Header map[string][]string` from Go's stdlib `net/http`. **Go map iteration is non-deterministic per language spec** (Go runtime randomizes the iteration start per-process).

The envoy-go codebase explicitly acknowledges this at `internal/filter/http/types.go:86-90`:

```
// Per Task 18 review: the unordered http.Header map cannot preserve the
// SPEC §11.2 verbatim 6-header order on the wire (Go map iteration is
// non-deterministic; net/http's Header.Write emits keys alphabetically
// sorted). HeaderField + OrderedHeaders is the ordered carrier that
// closes that gap.
```

The `OrderedHeaders` carrier `[]HeaderField` is used SPECIFICALLY for `SendLocalReply` wire-emit ordering — NOT for the routine filter-callback path which keeps the unordered `http.Header` map.

**Implication (REFUTES BRAINSTORM hypothesis):** envoy-go's filter-callback headers-map is NOT insertion-ordered. Iteration order is non-deterministic per Go map semantics. The parent SPEC §11.2.3 framing ("envoy-go's underlying headers-map MUST be insertion-ordered for scenario (f) determinism") IS REFUTED by this empirical pin.

**Implication for scenario (f):** the script `local n=0; for k,v in pairs(rh:headers()) do n=n+1 end; rh:headers():add("x-headers-count", tostring(n))` counts entries — the count is order-independent and deterministic across runs. Cross-side byte-exact for `x-headers-count: N` STILL HOLDS because both sides agree on N regardless of iteration order.

**Implication for the bridge `__pairs` metamethod (NEW DISCIPLINE per this 22.1 SPEC):** The bridge `__pairs` metamethod MUST snapshot `http.Header` into a Go slice **sorted alphabetically** at `__pairs` invocation, then walk by integer index. Rationale:

1. **Closes per-run non-determinism** so an operator's debugging is reproducible (a script that uses iteration order for any purpose — e.g., concatenating header names into a string — produces stable output across process restarts).
2. **Matches `net/http.Header.Write`'s emit-order discipline** (project-internal precedent — the stdlib emits headers alphabetically sorted when writing to the wire).
3. **Does not require new insertion-order tracking infrastructure** (would require extending the headers carrier across all filter callbacks; out of 22.1 scope; defers to a hypothetical future phase).
4. **For scenario (f) count-only assertion**, alphabetical-vs-insertion ordering is irrelevant (count is order-independent).

**§13-R3 RATIFIED-PENDING item disposition (REFINED at this SPEC commit):** REFINED from "verify envoy-go's headers-map is insertion-ordered" (parent §13-R3 anticipation) to **"bridge `__pairs` metamethod snapshots `http.Header` into alphabetically-sorted slice + iterates by integer index"**. Lands at IMPL Task 6 + verified at `bridge_test.go` cross-run-determinism test. NOT a BEHAVIOR_CONTRACT.md departure record (this is project-internal discipline — see §10 row 6).

**Resolution:** D7 CLOSED at this SPEC commit. The §13-R3 RATIFIED-PENDING item carries the refined alphabetical-snapshot disposition (replaces the parent's insertion-order-verification framing).

---

## 12. SPEC-time D-questions (carry-forward)

Two of the parent SPEC §12 D-questions remain open after this 22.1 SPEC commit. They carry forward verbatim to their parent-SPEC-anchored resolution points:

### D1 — `default_source_code` absent vs no-op disposition (per parent §12-D1)

**Question (parent §12-D1):** When `Lua.DefaultSourceCode` is unset AND `Lua.SourceCodes` is empty AND `Lua.InlineCode` is empty (post-`InlineCode` PARSE-REJECT), what is upstream Envoy's behavior — PARSE-REJECT (envoy-go's §7 arm 5 disposition) or degraded no-op (silent pass-through)? Same question applies to `script-missing-required-hooks` (§7 arm 17) — does upstream allow a compile-clean hook-less script as a no-op?

**Resolution at:** IMPL **Task 2 first action** (per §6 Task 2 spec). Scrape upstream Envoy v1.37.2 `source/extensions/filters/http/lua/config.cc::createFilterFactoryFromProtoTyped` + `lua_filter.cc::Filter` constructor against the anticipated PARSE-REJECT-both disposition for arms 5 + 17.

**Anticipated answer (BRAINSTORM hypothesis + parent §12-D1 anticipation):** PARSE-REJECT both (upstream cannot operate a filter with no script source). If REFUTED at IMPL: arms 5 + 17 flip to silent no-op (degraded pass-through); ADR-0189 §Decision body records the empirical disposition.

### D3 — scenario (e) `:logInfo()` cross-side assertion shape (per parent §12-D3)

**Question (parent §12-D3):** BRAINSTORM §6.2 wire-output column for scenario (e) states "Request unchanged at upstream + envoy log message". The runner cannot byte-diff log output today (no `LogAsserter` interface beyond `AccessLogAsserter` for access-log files). Three options:
- (a) Drop the "envoy log message" assertion from cross-side scope; rely on `lua.<prefix>.executions` stat counter to confirm the script ran.
- (b) Add `:logInfo()` calls that ALSO bump a counter the driver can scrape (artificial — pollutes the script).
- (c) Introduce a NEW `LogAsserter` interface paralleling `AccessLogAsserter` (heavier infra delta).

**Resolution at:** **22.1 PLAN session** per parent §12-D3 + parent §11.7.7 RECOMMENDED option (a).

**Anticipated answer (parent SPEC §11.7.7 RECOMMENDED):** option (a). The stat-counter delta IS the "Lua ran" assertion; the literal log line is supplementary. Scenario (e)'s wire-output column reads "Request unchanged at upstream; `lua.<prefix>.executions` counter delta = 1 per probe."

### D5 + D7 — CLOSED IN-SESSION at this SPEC commit

D5 + D7 resolved per §11 above. No further IMPL-time action required.

---

## 13. RATIFIED-PENDING items (cross-reference parent §13 + sub-phase-specific)

The parent SPEC §13 enumerates 6 RATIFIED-PENDING-IMPL items + 1 Wording-discipline item (W). This 22.1 SPEC INHERITS all 7 items + REFINES R3 per §11.2 D7 resolution + carries forward R6 escape-valve candidate. Disposition table:

| Item | Parent §13 framing | 22.1 SPEC disposition |
|---|---|---|
| **R1** | Scenario (g) `BootRejectFixture` infra (~80-130 LoC harness + ~50 LoC main.go wrapping) | STANDS. Lands at IMPL Task 13-15 per §6. No ADR. |
| **R2** | Scenario (d) `:respond()` full byte-pin per §11.6.7 | STANDS. Lands at IMPL Task 14 fixture-0026 driver per §9.1. |
| **R3** | envoy-go headers-map insertion-order verification | **REFINED** per §11.2 D7 resolution: envoy-go headers-map IS unordered (Go map); bridge `__pairs` snapshots alphabetically + walks by index. Discipline lands at IMPL Task 6 + verified at `bridge_test.go` cross-run-determinism test. |
| **R4** | 28th-fuzzer count verification | **CLOSED** per §11.1 D5 resolution: count CONFIRMED at 27 → 28 with `FuzzLuaConfigParse`. ADR-0189 §Decision body + BEHAVIOR_CONTRACT.md §13.4 patch pin to 28. |
| **R5** | ADR-0177 `internal/httpclient/` first co-consumer validation | NOT a 22.1 item; settles at 22.2 IMPL (`:httpCall()` task). |
| **R6** | `*LState`-pool benchmark gate | STANDS. Lands at IMPL Task 12 (race tests) benchmark sub-task: measure per-stream `*LState` construction cost at headers-only bridge surface. If < 1ms: WEAK-default per-stream construction STANDS; no ADR-0190 fires. If > 1ms: ADR-0190 escape-valve consumed for `*LState` pool design (§Context + §Decision + §Consequences body all land at the same Task 16 commit per ADR-0044). |
| **W** | Wording-pinning at envoy-go boot-reject path | STANDS. Lands at IMPL Task 15 per §6 + §9 (envoy-go-side `"script load error: "` wrapping at `cmd/envoy-go/main.go:60-66`). |

**New 22.1-SPEC sub-pins:**

- **§11.2 bridge `__pairs` alphabetical-snapshot discipline** — RATIFIED at this SPEC commit; lands at IMPL Task 6 per the REFINED R3 disposition.
- **D1 closure at IMPL Task 2 first action** — RATIFIED-PENDING-IMPL-TIME (anticipated PARSE-REJECT both arms 5 + 17).

---

## 14. BEHAVIOR_CONTRACT.md edit bundle (cross-reference parent §14)

The parent SPEC §14 enumerates the 22.1 IMPL final-Task **7-edit bundle**. This 22.1 SPEC INHERITS the 7-edit bundle verbatim. The bundle lands at IMPL Task 16 per §6 atomic landing:

1. NEW `### envoy.filters.http.lua` subsection (~80-120 lines) — headers-bridge-focused for 22.1; forward-pointers to 22.2 + 22.3.
2. Stat-table 99 → 102 extension under `## Stat surface` — 3 new rows under `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` template; extension summary paragraph.
3. envoy-go-strict departure record #1: stdlib-sandbox-strict (per AMEND-1).
4. envoy-go-strict departure record #2: `respond_calls` counter (per AMEND-3 corrected from BRAINSTORM 2-record bundle).
5. envoy-go-strict departure record #3: runtime-error log-message wording (per AMEND-9).
6. NEW `### Phase 22.1 forward-pointer notes` subsection (~30-50 lines) — 22.2 + 22.3 anticipated additions.
7. Per-route-canonical cross-reference caption update — 1-line edit referencing ADR-0125 §(xiv) AMENDMENT-anticipation paragraph.

22.2 + 22.3 IMPL final-Task bundles anticipated per parent §14 — settled at 22.2 + 22.3 BRAINSTORM/SPEC.

---

## 15. Test surface + 22.1 IMPL acceptance checklist

### 15.1 Test surface (per parent SPEC §15)

The parent SPEC §15 enumerates the 5-layer 22.1 test surface verbatim:

- **Layer A: unit tests at `internal/filter/http/lua/`** — `lua_test.go` (~1500-2000 LoC); `compiled_config_test.go` (18-arm table-driven per parent §6.2); `datasource_test.go` (4-arm + 10-leaf table-driven); `bridge_test.go` (bridge methods + `__pairs` alphabetical-snapshot + respond byte-pin + AMEND-8 runtime-reject).
- **Layer B: unit tests at `internal/lua/`** — `vm_test.go` (per-stream `*LState` construction + RegisterGlobalFunc + Run + Has/CallGlobal + Close); `compile_test.go` (CompileCache + CompileScript); `sandbox_test.go` (per-stdlib-module ALLOW/DENY exhaustive).
- **Layer C: 28th project-wide fuzzer `FuzzLuaConfigParse`** at standard ADR-0018 baseline; ~30 corpus seeds covering all 18 PARSE-REJECT arms + valid + adversarial.
- **Layer D: differential fixture `0026-http-lua-headers-bridge`** per §9 — 6 wire-interactive scenarios cross-side byte-exact via `CompareBytes`; scenario (g) substring-match via `BootRejectFixture`.
- **Layer E: race + concurrency tests** at `internal/lua/` + `internal/filter/http/lua/` — concurrent VM construction; compile-cache concurrent-read/add; per-stream filter dispatch independence.

### 15.2 22.1 IMPL acceptance checklist (parent §16 + sub-phase-specific extensions)

The parent SPEC §16 enumerates 18 items. This 22.1 SPEC EXTENDS with 6 sub-phase-specific items per the next-prompt-permitted scope. The 22.1 IMPL Task 16 atomic landing MUST satisfy ALL of:

**Items 1-18 from parent SPEC §16 (verbatim — see parent SPEC §16):**

1. NEW `internal/lua/` package created with API surface per parent §4.1 + this 22.1 SPEC §3.1 production refinements.
2. NEW `internal/filter/http/lua/` package created with files per parent §4.4 + this 22.1 SPEC §3.5 file split.
3. `Lua.DefaultSourceCode` consumed; `Lua.SourceCodes` + `Lua.InlineCode` + `LuaPerRoute` PARSE-REJECTed per parent §6.2 arms 3 + 4 + 18.
4. 4-arm DataSource resolution + WatchedDirectory PARSE-REJECT per parent §5.3 + §6.2 arms 6-15.
5. Pragmatic-middle bridge surface per BRAINSTORM Q6 + this 22.1 SPEC §1 + §4.3 dispatch shape.
6. Stdlib-sandbox-strict default-deny per parent §4.3 + AMEND-1 + envoy-go-strict departure record at BEHAVIOR_CONTRACT.md per parent §14 edit #3.
7. Per-stream `*LState` construction + per-script-source `*Chunk` cache per parent §4.2 + this 22.1 SPEC §3.4.
8. 18-arm PARSE-REJECT roster per parent §6.2 (subject to §12-D1 disposition for arms 5 + 17 — closed at Task 2 first action per §6).
9. 3-counter stat surface per parent §7 + this 22.1 SPEC §8 — 99 → 102 BEHAVIOR_CONTRACT.md update per §14 edit #2.
10. `respond_calls` envoy-go-strict counter departure record at BEHAVIOR_CONTRACT.md per §14 edit #4 + AMEND-3 (single record — corrected from BRAINSTORM 2-record bundle).
11. Runtime-error log-message wording envoy-go-strict departure record at BEHAVIOR_CONTRACT.md per §14 edit #5 + AMEND-9.
12. 28th project-wide fuzzer `FuzzLuaConfigParse` (count CONFIRMED at this 22.1 SPEC §11 D5 resolution; pin to 28); must-never-panic verified.
13. Differential fixture `0026-http-lua-headers-bridge` GREEN — 6 wire-interactive scenarios (a)-(f) cross-side byte-exact via `CompareBytes`; scenario (g) substring-match via NEW `BootRejectFixture` per §13-R1 + §9.1.
14. NEW `BackendKind=HTTPLua` constant added at `test/differential/runner_test.go:547` per AMEND-11; **`test/fixtures/0026-http-lua-headers-bridge/scripts/`** subdirectory + 7 per-scenario `.lua` files per §9 (canonical fixture-side scripts/ location per parent §8.4; NOT under `internal/filter/http/lua/scripts/` which parent §16 item 14 mistakenly cites — see footnote ⁂).

> ⁂ **Cross-reference correction (parent §16 item 14 typo).** Parent SPEC §16 item 14 reads "`internal/filter/http/lua/scripts/` subdirectory" — that path collides with the production filter package directory and contradicts parent §8.4 + parent AMEND-11 + this 22.1 SPEC §9.4. The CANONICAL location is `test/fixtures/0026-http-lua-headers-bridge/scripts/` (fixture-local, exploiting DataSource `Filename` arm naturally per AMEND-11). The 22.1 SPEC uses the canonical path throughout (§6 Task 14, §9.4, §15.2 item 14); parent §16 item 14's typo silently inherits the corrected interpretation here.
15. envoy-go-side `"script load error: "` wrapping at `cmd/envoy-go/main.go:60-66` boot-reject path per §13-W + §6 Task 15.
16. ADR-0188 §Decision + §Consequences body landed in DECISIONS.md per parent §4.1 + this 22.1 SPEC §3.1 production signatures + ADR-0044 in-place edit discipline.
17. ADR-0189 §Decision + §Consequences body landed in DECISIONS.md per parent §4.4 + this 22.1 SPEC §3.5 file split + §4 compiledConfig + §6.2 18-arm roster + §8 fixture-0026 + §13-R1 BootRejectFixture + D5 + D7 + Task 2 D1 closure evidence + ADR-0044.
18. STATE.md re-advance to `phase 22.1 IMPL done; awaiting 22.2 SPEC` + ROADMAP row 22.1 flipped `planned → done` per ADR-0106 per-cell IMPL-done annotation.

**Items 19-24 — 22.1 SPEC-specific extensions:**

19. **D5 resolution recorded at §11.1** — 28th-fuzzer count CONFIRMED at this SPEC commit; ADR-0189 §Decision body + BEHAVIOR_CONTRACT.md §13.4 patch pin to 28 at IMPL Task 16.
20. **D7 resolution recorded at §11.2** — envoy-go headers-map type EMPIRICALLY-DETERMINED as `net/http.Header` (Go map; unordered); bridge `__pairs` alphabetical-snapshot discipline RATIFIED at this SPEC commit; lands at IMPL Task 6 + verified at `bridge_test.go` cross-run-determinism test.
21. **D1 closure at IMPL Task 2 first action** — upstream Envoy v1.37.2 scrape against `config.cc::createFilterFactoryFromProtoTyped` + `lua_filter.cc::Filter` constructor; PROGRESS.md entry quotes upstream evidence; ADR-0189 §Decision body records the empirical disposition (anticipated PARSE-REJECT both arms 5 + 17).
22. **D3 closure at 22.1 PLAN session** per parent SPEC §12-D3 + §11.7.7 RECOMMENDED option (a) anticipated; PLAN session anchors the option-(a) disposition + scenario (e) cross-side assertion shape.
23. **Per-task PROGRESS.md entry shape per phase-21 IMPL precedent** — 16 entries across all 16 tasks; each entry quotes command outputs per `superpowers:verification-before-completion`; each Task's acceptance criteria from §6 verified before PROGRESS.md entry.
24. **REVIEW.md authored at 22.1 IMPL phase-done** per `superpowers:requesting-code-review` per phase-21 IMPL precedent; per-task review notes + cross-cutting review notes + green-light evidence.

---

## Appendix A — Cross-references to parent SPEC

This 22.1 SPEC cross-references the parent SPEC at `docs/envoy-go/phases/22-http-filter-lua/SPEC.md` for the following content (inherited verbatim; NOT duplicated here):

- **Parent §1** (Mission) — envelope D + 3-way pre-split + 14-fact summary (FIRST §9 row with script-driven semantics + 3-way pre-split + framework primitive + ADR-0125 amendment + third-party Lua VM dependency).
- **Parent §1.1** (12-AMEND catalog) — all 12 AMENDs incorporated into this 22.1 SPEC's §3/§4/§7/§8/§9/§10/§14.
- **Parent §1.2** (STRENGTHENED-or-revised D-hypothesis at SPEC commit) — WEAK HOLD STANDS UNCHANGED; 22.1 IMPL escape-valve at ADR-0190 from `*LState`-pool benchmark surface (§13-R6).
- **Parent §2** (Scope — non-purposes + REUSES-not-consumed) — full 18-item non-purposes catalog; this 22.1 SPEC §2 summarizes for 22.1 surface only.
- **Parent §3** (Sub-phase scope summary) — 3-way split LOCKED at BRAINSTORM Q2; STANDS unchanged at SPEC commit; per-sub-phase scope detail at each sub-phase's SPEC.
- **Parent §4** (Framework primitive) — `internal/lua/` API + `internal/filter/http/lua/` package + sandbox roster + ADR-0125 §(xiv) AMENDMENT-anticipation.
- **Parent §5** (Proto-field roster) — `Lua` 4 fields + `LuaPerRoute` 3-arm oneof + DataSource 4-arm + binding-gap forward-pointers.
- **Parent §6** (PARSE-REJECT roster) — 18-arm 22.1 roster per AMEND-4/-5/-6; 22.3 forward-pointer arms.
- **Parent §7** (Stat surface) — 3 counters under HCM-rooted template per AMEND-2 + AMEND-3.
- **Parent §8** (Differential fixture taxonomy) — 7-scenario fixture-0026 + scope-narrow scenario (g) per AMEND-10 + `BackendKind=HTTPLua` + `scripts/` subdirectory per AMEND-11.
- **Parent §9** (Behavior-contract delta) — 6 high-level semantic changes.
- **Parent §10** (Deferred items + forward-pointers) — 19+ items spanning 22.2/22.3/future-phases.
- **Parent §11** (SPEC-time empirical-pin block) — all 7 pins resolved IN-SESSION at parent SPEC drafting; §11.1-§11.7 full scrape evidence.
- **Parent §12** (SPEC-time D-questions for PLAN-time resolution) — D1/D3/D5/D7 (D5+D7 CLOSED at this 22.1 SPEC §11; D1+D3 carry forward per this 22.1 SPEC §12).
- **Parent §13** (RATIFIED-PENDING-IMPL items) — 6 items + W wording-pinning; this 22.1 SPEC §13 disposition table maps each.
- **Parent §14** (BEHAVIOR_CONTRACT.md edit bundle anticipation) — 7-edit bundle at 22.1 IMPL final Task per ADR-0052.
- **Parent §15** (Test surface) — 5-layer test taxonomy.
- **Parent §16** (22.1 IMPL acceptance checklist) — 18 items; this 22.1 SPEC §15.2 extends with 6 sub-phase-specific items 19-24.

---

## Appendix B — Phase 22.1 ADR landings summary

At THIS 22.1 SPEC commit: **NO NEW ADRs consumed.** DECISIONS.md tail STAYS at ADR-0189. Next-free ADR STAYS at ADR-0190.

At 22.1 IMPL Task 16 atomic landing:

- **ADR-0188 §Decision + §Consequences body** — NEW `internal/lua/` framework primitive. Per parent §4.1 sketch + this 22.1 SPEC §3.1 production signatures + §3.2 file split + §3.3 sandbox roster + §3.4 per-stream lifecycle + EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 per BRAINSTORM Q4.
- **ADR-0189 §Decision + §Consequences body** — NEW `internal/filter/http/lua/` package shape. Per parent §4.4 + this 22.1 SPEC §3.5 file split + §4 compiledConfig + filterStats + §6.2 18-arm PARSE-REJECT roster + §8 fixture-0026 disposition + §11.1 D5 + §11.2 D7 + Task 2 D1 closure evidence + §13-R1 BootRejectFixture infrastructure + §13-W wording-pinning discipline.
- **ADR-0190 §Context + §Decision + §Consequences body (CONDITIONAL — only if R6 escape-valve fires)** — `*LState` pool design. Only consumes if 22.1 IMPL Task 12 benchmark surfaces > 1ms per-stream construction cost. If unconsumed: ADR-0190 carries forward to 22.2 BRAINSTORM as the 22.2 IMPL escape-valve slot per parent §1.2.

At 22.3 IMPL final Task:

- **ADR-0125 §(xiv) IN-PLACE AMENDMENT body** — NEW 9th canonical per-route shape (3-arm hybrid `disabled-bool` + `string-reference-delegation` + `DataSource-wholesale-override`); roster grows 8 → 9. AMENDMENT-anticipation paragraph anchored at parent SPEC §4.5 STANDS UNCHANGED at this 22.1 SPEC commit.

---
