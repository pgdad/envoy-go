// Package lua implements the envoy.filters.http.lua HTTP filter under
// the 07.1 HTTP filter framework. Phase 22.1: VM + headers-bridge mode
// (foundational third of the FIFTEENTH §9 production HTTP filter; phase
// 22.2 + 22.3 deliver the full envelope D — body / trailers / metadata /
// connection / httpCall / crypto / streamInfo full + multi-script
// SourceCodes + per-route LuaPerRoute).
//
// Anchored at ADR-0189 (§Context drafted at phase 22 parent SPEC
// `41ccee7`; §Decision + §Consequences body lands at 22.1 IMPL Task 16
// per ADR-0044 in-place edit discipline). Consumes the NEW
// internal/lua/ framework primitive at first consumer per ADR-0188 +
// BRAINSTORM Q4 EXTRACT-NOW.
//
// # API surface (per 22.1 SPEC §4.1)
//
//   - TypeURL — byte-exact wire URL
//     "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"
//     per ADR-0143 SN1. Pinned at lua_test.go::TestTypeURL_Matches.
//   - filterName — "envoy.filters.http.lua" per ADR-0070; identifies
//     the filter on the listener config http_filters[].name + the HCM
//     chain dispatch identifier.
//   - New(tc *anypb.Any, ctx envoyhttp.FactoryCtx)
//     (envoyhttp.FilterInstanceFactory, error) — the
//     HTTPFilterFactory registered at boot (Task 10 wires
//     `httpReg.Register(lua.TypeURL, lua.New)` between localratelimit.New
//     and oauth2.New per ADR-0100 §2.2 alphabetical discipline +
//     PLAN D-P6). TASK 1 SKELETON: returns
//     (nil, errors.New("lua: not yet implemented")); FULL IMPL at Task 10.
//   - RegisterPerRouteValidator — exported function called from
//     cmd/envoy-go/main.go BEFORE httpReg.Freeze() per the
//     header_mutation + oauth2 precedent (the registry rejects post-
//     Freeze registrations; New runs during listener construction
//     AFTER Freeze, so it cannot self-register the validator). At
//     22.1 the validator one-liner returns the arm-18 PARSE-REJECT
//     "lua: per-route configuration is not yet supported (lands in
//     phase 22.3)" per ADR-0110 single-chokepoint discipline + parent
//     §6.2 arm 18. 22.3 IMPL replaces the body with the 9th-canonical
//     per-route shape validator.
//
// # BRAINSTORM Q1-Q12 decision summary (per parent BRAINSTORM)
//
//   - Q1 PARENT SCOPE: 22.1 lands envelope-B-equivalent VM + headers-
//     bridge; 22.2 lands body + trailers + httpCall + crypto delta;
//     22.3 lands multi-script SourceCodes + LuaPerRoute. PARENT 3-way
//     split LOCKED at parent BRAINSTORM Q1.
//   - Q2 GOPHER-LUA DEP: github.com/yuin/gopher-lua v1.1.2 pinned;
//     pure-Go Lua 5.1 (matches upstream LuaJIT 5.1 dialect); no CGO;
//     MIT license. ZERO third-party JWT / crypto / proto re-use; the
//     dep is the SINGLE pure-Go-only addition for phase 22.
//   - Q3 ENVELOPE-D BRIDGE: pragmatic-middle headers + log + streamInfo
//     subset + respond at 22.1; defer body / trailers / metadata /
//     connection / httpCall / crypto / fileBytes / timestamp / full
//     streamInfo to 22.2 (deferred methods raise Lua runtime errors
//     via gopher-lua's lua_error — caught by panic-wrapper +
//     increments lua.errors).
//   - Q4 EXTRACT-NOW vs DEFER framework primitive: EXTRACT-NOW per
//     ADR-0188 with EXPLICIT API-REVISION ALLOWANCE clause for
//     consumer #2 (cluster_specifier / access_logger / string_matcher
//     Lua — whichever materializes first).
//   - Q5 SANDBOX-STRICT vs LUAJIT-PARITY: SANDBOX-STRICT per AMEND-1
//     (envoy-go-strict DEPARTURE recorded at BEHAVIOR_CONTRACT.md
//     edit #3 at Task 16). gopher-lua exposes sandbox-breaking arms
//     (os.execute / io.popen / package.* / channel / debug.getupvalue)
//     that envoy-go's per-stream goroutine dispatch model cannot
//     tolerate at operator-trust bounds.
//   - Q6 PRAGMATIC-MIDDLE BRIDGE METHODS: 21 surfaces at 22.1 — 2
//     hooks (envoy_on_request / envoy_on_response) + 7 headers
//     methods + __pairs metamethod + 6 :logXxx + 4 streamInfo subset
//     plus 1 respond. Deferred methods raise lua_error.
//   - Q7 RESPOND BYTE-PIN: full byte-pin per parent §11.6.7 + AMEND-7
//     (status uint32; body []byte; headers ordered via SendLocalReply
//     OrderedHeaders carrier). :respond() at encode-side raises the
//     AMEND-8 runtime error "respond not currently supported in the
//     response path".
//   - Q8 PER-STREAM VM vs POOLED: PER-STREAM (WEAK-default at 22.1
//     per parent §13-R6 + 22.1 SPEC §2.19). Task 12 benchmark gates;
//     if > 1ms ADR-0190 escape-valve fires at Task 16 anchoring a
//     per-script-source *LState pool.
//   - Q9 STAT COUNT: 3 counters (errors + executions upstream-parity
//     per AMEND-3 + respond_calls envoy-go-strict extension per
//     AMEND-3 + BEHAVIOR_CONTRACT.md edit #4 at Task 16). HCM-rooted
//     template "http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>"
//     per parent AMEND-2.
//   - Q10 ADR ESCAPE-VALVE: ADR-0190 reserved for potential per-stream
//     *LState-pool decision (gated by Task 12 benchmark).
//   - Q11 FIXTURE-0026 SCENARIOS: 6 wire-interactive (a)-(f) full
//     cross-side byte-exact via existing CompareBytes; scenario (g)
//     substring-match "script load error" via NEW BootRejectFixture
//     driver interface (per parent §13-R1 + AMEND-10). NEW
//     BackendKind=HTTPLua + scripts/ subdirectory layout per AMEND-11.
//   - Q12 28TH FUZZER: FuzzLuaConfigParse with 30 corpus seeds (18
//     per-PARSE-REJECT-arm + 5 valid-config + 7 adversarial-Lua-source
//     per PLAN D-P7). Project fuzzer count 27 → 28.
//
// # AMEND-1..AMEND-12 cross-references (per parent SPEC §1.1)
//
//   - AMEND-1: stdlib-sandbox-strict default-deny (envoy-go-strict
//     DEPARTURE; §3.3 sandbox roster in internal/lua/sandbox.go +
//     IMPL discipline at Task 5).
//   - AMEND-2: HCM-rooted stat-prefix template (§stats.go at Task 10).
//   - AMEND-3: executions reclassified as upstream-parity; respond_calls
//     ONLY envoy-go-strict (departure record bundle 2 → 1; §8 stats).
//   - AMEND-4: 18-arm PARSE-REJECT roster (§compiled_config.go at Task 2).
//   - AMEND-5: DataSource 10-arm refinement (§datasource.go at Task 3).
//   - AMEND-6: 3 additional baseline arms; sandbox-violation arm
//     RETRACTED (§7 + §compiled_config.go at Task 2).
//   - AMEND-7: :respond() full byte-pin extension (§bridge.go +
//     decode_headers.go at Task 6 + Task 9).
//   - AMEND-8: :respond() from envoy_on_response runtime-reject +
//     :status [200,600) validation (§encode_headers.go at Task 9).
//   - AMEND-9: gopher-lua-vs-LuaJIT divergences (§10 BEHAVIOR_CONTRACT
//     departure record at Task 16).
//   - AMEND-10: fixture-0026 scenario (g) substring-match (§Task 14).
//   - AMEND-11: NEW BackendKind=HTTPLua + scripts/ subdir layout
//     (§Task 13 + Task 14).
//   - AMEND-12: v1.32.4-vs-v1.37.2 binding-gap forward-pointers
//     (silent-drop; no 22.1 IMPL surface).
//
// # D1 / D5 / D7 cross-references (per 22.1 SPEC §11 + §12)
//
//   - D1 — default_source_code absent vs no-op disposition: closes at
//     IMPL Task 2 first action (upstream Envoy v1.37.2 scrape against
//     config.cc::createFilterFactoryFromProtoTyped + Filter constructor);
//     anticipated PARSE-REJECT both arms 5 (default-source-code-required)
//     and 17 (script-missing-required-hooks).
//   - D5 — 28th-fuzzer count CONFIRMED via project-wide grep (Pre-Task 0
//     PC14 evidence); pins ADR-0189 §Decision body + BEHAVIOR_CONTRACT.md
//     §13.4 patch to 28 at IMPL Task 16.
//   - D7 — envoy-go headers-map type EMPIRICALLY-DETERMINED as
//     net/http.Header (= Go map[string][]string; unordered). REFUTES
//     BRAINSTORM hypothesis of insertion-ordered slice-backed; scenario
//     (f) cross-side determinism STILL HOLDS via count-only assertion
//     (the fixture-0026 scenario (f) script counts pairs, doesn't depend
//     on iteration order). NEW bridge __pairs metamethod discipline:
//     snapshot to alphabetically-sorted []struct{k,v string} at __pairs
//     invocation + iterate by integer index (lands at Task 6).
//
// # Per-route discipline (PARSE-REJECT at 22.1; 9th canonical at 22.3)
//
// LuaPerRoute PARSE-REJECTs at any tier (Route / VirtualHost /
// RouteConfiguration / listener-typed_per_filter_config) via
// RegisterPerRouteValidator per ADR-0110 single-chokepoint + parent §6.2
// arm 18 + PLAN D-P6. Wording: "lua: per-route configuration is not yet
// supported (lands in phase 22.3)". 22.3 IMPL replaces the body with the
// NEW 9th-canonical per-route shape validator (3-arm hybrid disabled-bool
// + string-reference-delegation + DataSource-wholesale-override).
// ADR-0125 §(xiv) IN-PLACE AMENDMENT (roster 8 → 9) lands at 22.3 IMPL
// final Task; the AMENDMENT-anticipation paragraph at parent §4.5 +
// DECISIONS.md ADR-0125 §(xiv) STANDS UNCHANGED at 22.1 IMPL.
//
// # File split (per 22.1 SPEC §3.5)
//
// 8 production files + 5 test files. At Task 1 only doc.go + lua.go +
// lua_test.go land. Subsequent tasks add:
//   - Task 2: compiled_config.go + compiled_config_test.go (18-arm
//     PARSE-REJECT roster + D1 closure + valid-config rows).
//   - Task 3: datasource.go + datasource_test.go (4-arm DataSource
//     resolution + WatchedDirectory PARSE-REJECT + empty-oneof
//     PARSE-REJECT + file-read failure paths).
//   - Task 6-8: bridge.go + bridge_test.go (request_handle /
//     response_handle userdata + metatable setup + 7 headers methods
//   - __pairs alphabetical-snapshot + 6 log methods + 4 streamInfo
//     methods + respond byte-pin).
//   - Task 9: decode_headers.go + encode_headers.go (envoy_on_request /
//     envoy_on_response dispatch + respond-state handling).
//   - Task 10: stats.go (3-counter HCM-rooted registration).
//   - Task 11: fuzz_test.go (28th project-wide fuzzer).
//
// # Phase 22.3 — multi-script SourceCodes + per-route LuaPerRoute
//
// Phase 22.3 lands the LAST two PARSE-REJECT surfaces (arm 4 + arm 18)
// + the NEW 9th canonical per-route shape. CONSUME + DISPATCH only:
// 0 new framework primitives, 0 net-new stats (count STAYS 107), 0
// net-new bridge methods, 0 net-new BEHAVIOR_CONTRACT departure records
// (all 22.3 dispositions are upstream-parity).
//
//   - SourceCodes registry (Task 1) — buildCompiledConfig consumes
//     Lua.SourceCodes in SORTED-key order into
//     cc.sourceCodes map[string]*internallua.Chunk (the name → *Chunk
//     registry; nil when the proto has no source_codes); each value
//     resolves via the 22.1 resolveDataSource + compiles via
//     CompileScript into the SHARED per-listener content-hash
//     CompileCache (byte-identical named scripts dedup to one *Chunk).
//     Named scripts are dispatch TARGETS only; default_source_code stays
//     the sole listener default. Sole SourceCodes-key arm:
//     "lua: source_codes: key must be non-empty". The arm-4
//     source_codes-deferred reject is RETIRED.
//   - LuaPerRoute 3-arm validator (Task 2) — NEW perroute.go
//     parsePerRouteLua replaces the arm-18 one-liner: oneof-required /
//     disabled-must-be-true (PGV const:true) / name-min-1-rune (PGV
//     min_len:1) / source_code DataSource gauntlet (compile-to-validate,
//     chunk discarded) + defensive default arm (ADR-0018). lua.go's
//     validatePerRouteLua delegates. 6 net-new config-load arm-groups
//     (D-P3); arms 3 (reserved-name) + 7 (dangling-name) DROPPED.
//   - Per-route 3-tier dispatch (Task 3) — (*filter).resolveDecodeScript:
//     disabled → skip both hooks (no VM built); name → cc.sourceCodes
//     lookup (hit → run; miss → upstream-parity SILENT NO-OP per
//     AMEND-22.3-1); source_code → resolvePerRouteSourceCode memo
//     override; fall through to listener default; else no-op. Matches
//     upstream getPerLuaCodeSetup() precedence.
//   - D-P1(b') no-re-read memo — per-route source_code override compiles
//     with a content-hash cache HIT at bind, proto-pointer-memoized via
//     cc.perRouteChunks map[*luav3.LuaPerRoute]*Chunk guarded by
//     cc.perRouteMu. resolveDecodeScript switches GetOverride() DIRECTLY
//     (does NOT call parsePerRouteLua per stream) so the source_code
//     Filename DataSource is read ONCE per route, never re-read per
//     stream. (The PLAN's literal "call parsePerRouteLua at dispatch"
//     wording would have defeated the no-re-read guarantee; a
//     read-counting Filename test surfaced + pins the correction. See
//     REVIEW.md + PROGRESS.md Task 3.)
//   - Encode-guard fix (Task 3) — encode_headers.go gates on f.vm==nil
//     (the f.cc.chunk==nil clause DROPPED) so a per-route override on a
//     default-less listener still fires envoy_on_response.
//   - 9th canonical (ADR-0125 §(xiv) AMENDMENT, roster 8 → 9) — 3-arm
//     hybrid combining the 5th canonical's disabled-bool + the 8th
//     canonical's string-reference-delegation + a NOVEL DataSource-typed
//     wholesale-override; SHARED stat-discipline (per-route errors charge
//     to listener-level lua.<prefix>.errors; SHARED-vacuous). Lands at
//     22.3 IMPL Task 6 per ADR-0193.
//   - R6 disposition (Task 4) — BenchmarkPerStream_PerRoute_Resolution
//     resolution-only 10.46 ns/op (0 allocs) + per-stream 31.47 ns/op,
//     both ~5 orders of magnitude under the 1ms gate. WEAK-default
//     STANDS; conditional ADR-0194 NOT consumed (STAYS next-free).
//   - Fuzzer (Task 4) — NEW FuzzLuaPerRouteConfig (30 → 31 project-wide);
//     FuzzLuaConfigParse corpus extended with source_codes seeds.
//   - Differential (Task 5) — fixtures 29 → 31 (AUTHORIZED two-directory
//     amendment, NOT 30): 0028 cross-side multi-listener (5 per-route
//     scenarios + dangling-name no-op) + 0029 source_codes-boot-reject.
//     The framework dispatches one branch per directory (cross-side XOR
//     boot-reject), so the SPEC single-fixture shape split into two dirs.
//
// AMEND-22.3-1: a dangling per-route name is an upstream-parity SILENT
// NO-OP at per-stream dispatch, NOT a config-load PARSE-REJECT (mirrors
// upstream perLuaCodeSetup() → nullptr → LUA_REFNIL). No reserved-name
// discipline: default_source_code + source_codes are independent fields.
// Both are upstream-parity → 0 net-new departure records.
//
// 22.3 cross-references: ADR-0193 (NEW combined 22.3 package-shape
// extension §Decision + §Consequences) + ADR-0125 §(xiv) (9th canonical
// roster 8 → 9). Parent row 22 closes at 22.3 IMPL phase-done.
//
// # Cross-references
//
//   - ADR-0188 (NEW internal/lua/ framework primitive; bodies land at
//     Task 16).
//   - ADR-0189 (NEW internal/filter/http/lua/ package shape; bodies
//     land at Task 16).
//   - ADR-0070 (filter-registration convention; filterName below).
//   - ADR-0071 (two-step factory; HTTPFilterFactory return signature).
//   - ADR-0072 (boot-time-fail-fast).
//   - ADR-0110 (per-route validator single-chokepoint).
//   - ADR-0143 SN1 (byte-exact TypeURL pin).
//   - 22 parent SPEC + 22.1 SPEC §3.5 + §4 + §5.
package lua
