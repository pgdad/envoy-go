# Phase 22.1 — HTTP filter `envoy.filters.http.lua` (filter scaffold + `internal/lua/` VM primitive + headers bridge + DefaultSourceCode) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the foundational third of `envoy.filters.http.lua` (the FIFTEENTH §9 production HTTP filter) — VM + headers-bridge mode per BRAINSTORM Q6 pragmatic-middle — by shipping the NEW `internal/lua/` framework primitive (3 production + 3 test Go files: `doc.go` + `vm.go` + `compile.go` + `sandbox.go` + `vm_test.go` + `compile_test.go` + `sandbox_test.go`; gopher-lua v1.1.2 VM lifecycle + per-stream `*lua.LState` construction + per-script-source `*Chunk` compile cache + `SandboxConfig` per-stdlib ALLOW/DENY with zero-value `StrictUpstreamParity` posture + bridge-registration `State()` escape-hatch + `WithPanicHandler`/`WithBasePrintSink` VMOptions + panic-wrapper; ADR-0188 §Decision + §Consequences body) + the NEW `internal/filter/http/lua/` package (8 production + 5 test Go files: `doc.go` + `lua.go` + `compiled_config.go` + `datasource.go` + `bridge.go` + `decode_headers.go` + `encode_headers.go` + `stats.go` + 5 test files; 4-arm `DataSource` resolution `Filename`/`InlineBytes`/`InlineString`/`EnvironmentVariable` + 18-arm PARSE-REJECT roster per parent §6.2 + bridge surface 21 entries [2 hooks + 7 headers methods + `__pairs` alphabetical-snapshot per §11 D7 + 6 log methods + 4 `:streamInfo()` methods + 1 `:respond()` with full byte-pin per parent §11.6.7 + AMEND-7 + AMEND-8 encode-side runtime-reject] + 3-counter HCM-rooted stat surface `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.{errors,executions,respond_calls}` per parent §7 + AMEND-2 + AMEND-3; ADR-0189 §Decision + §Consequences body) + the boot-registration insertion at `cmd/envoy-go/main.go` alphabetical between `localratelimit.New` and `oauth2.New` per ADR-0100 §2.2 (17 HTTP filters wired post-22.1; was 16) + the 28th project-wide fuzzer `FuzzLuaConfigParse` (count CONFIRMED at 22.1 SPEC §11.1 D5 closure — `FuzzAccessLogFormat`/`FuzzAdaptiveConcurrencyConfigParse`/.../`FuzzTLSContextParse` = 27 unique pre-22.1; `FuzzLuaConfigParse` is the 28th) + the NEW `BackendKind=HTTPLua` constant at `test/differential/runner_test.go:547` per AMEND-11 + the NEW OPTIONAL `BootRejectFixture` driver interface at `test/differential/harness.go` per parent §13-R1 + AMEND-10 (~80-130 LoC harness delta + `tryStartReferenceProxy` + `tryStartSubjectProxy` variants returning the boot error + stderr buffer; `runner_test.go::runBootRejectFixture` branch ~50 LoC parallel to `runReferenceLessFixture` asserting both sides exit non-zero AND both sides' stderr contains substring `"script load error"`) + the differential fixture `0026-http-lua-headers-bridge` with 7 scenarios (a)-(g) cross-side per parent §8 + this 22.1 SPEC §9 (`a_add_header.lua` + `b_replace_header.lua` + `c_remove_header.lua` + `d_respond.lua` + `e_log_only.lua` + `f_headers_iter.lua` + `g_compile_error.lua` under `test/fixtures/0026-http-lua-headers-bridge/scripts/` per AMEND-11 + parent §8.4 — NOT under `internal/filter/http/lua/scripts/` per parent §16 item 14 footnote typo correction; 6 wire-interactive scenarios (a)-(f) full cross-side byte-exact via existing `CompareBytes`; scenario (g) substring-match via the NEW `BootRejectFixture` interface; scenario (e) per the D3 closure at this PLAN session — locked at parent §11.7.7 RECOMMENDED option (a): the `lua.<prefix>.executions` stat-counter delta IS the "Lua ran" assertion, surfaced via the existing `/stats` admin scrape mechanism per the fixture-0025 inline-snapshot precedent rather than the heavier `LogAsserter` interface option (c) or the artificial counter-bumping option (b)) + the envoy-go-side `"script load error: "` wording-pinning at `cmd/envoy-go/main.go:60-66` boot-reject path per parent §13-W + this 22.1 SPEC §6 Task 15 — with byte-equivalent wire outcomes against reference Envoy v1.37.2 on the 6 wire-interactive fixture-0026 scenarios (a)-(f) + substring-match-equivalent on scenario (g) script-compile-error, modulo the 3 envoy-go-strict documented divergence-windows (stdlib-sandbox-strict per AMEND-1; `respond_calls` envoy-go-strict counter per AMEND-3 corrected from BRAINSTORM 2-record bundle; gopher-lua-vs-LuaJIT runtime-error log-message wording divergence per AMEND-9). **Sub-phase landing (`22.1` ROADMAP row) per parent SPEC §3.1 + BRAINSTORM Q2 PRE-SPLIT discipline** — the 22.1 PLAN closes ROADMAP row `22.1` only at phase-done; parent row `22` STAYS `in-progress` until 22.3 IMPL phase-done (sub-row rollup discipline per ADR-0106 + phase-18.1/18.2 + phase-19.1/19.2 precedent). 22.2 (full bridge delta: `:body`/`:bodyChunks`/`:trailers`/`:metadata`/`:connection`/`:httpCall`/crypto/sha/base64/`:fileBytes`/`:timestamp`/full `:streamInfo`) + 22.3 (multi-script `SourceCodes` map + `LuaPerRoute` 3-arm oneof + NEW 9th canonical per-route shape ADR + ADR-0125 §(xiv) IN-PLACE AMENDMENT roster 8 → 9) are OUT OF SCOPE for 22.1.

**Architecture:** The 22.1 IMPL adds TWO new packages (`internal/lua/` 3-production-file framework primitive + `internal/filter/http/lua/` 8-production-file consumer package) + extends 3 existing files (`cmd/envoy-go/main.go` +1 register + +1 import per ADR-0100 §2.2; `test/differential/fixture/fixture.go` +1 `HTTPLua BackendKind = 22` enum value + dispatcher metadata; `test/differential/runner_test.go` +blank-import + `BackendKind=HTTPLua` switch-case + NEW `runBootRejectFixture` branch) + adds 1 new harness primitive (`test/differential/harness.go` +`BootRejectFixture` interface + `tryStartReferenceProxy`/`tryStartSubjectProxy` variants ~80-130 LoC delta per parent §11.7.3) + adds 1 new fixture directory (`test/fixtures/0026-http-lua-headers-bridge/` with `README.md` + `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `inputs/driver.go` + `scripts/` subdirectory containing 7 `.lua` source files per AMEND-11 + this 22.1 SPEC §9.4 + parent §8.4) + wraps the gopher-lua compile error with `"script load error: "` prefix at `cmd/envoy-go/main.go:60-66` boot-reject path per parent §13-W. The NEW `internal/lua/` framework primitive follows the compact primitive-package precedent at `internal/jwks/` + `internal/httpclient/` (3 production files + 3 test files; refined from parent SPEC §4.1 sketch per this 22.1 SPEC §3.1: `VMOption` function-option pattern instead of sealed interface; `Run(chunk) + HasGlobalFunc(name) + CallGlobal(name, args...)` split from parent's `Run(chunk, hooks ...HookFn)` blob; `State() *lua.LState` escape-hatch exposed for userdata setup; `WithPanicHandler` renamed from `WithPanicRecovery`; `WithBasePrintSink` renamed from `WithStderrSink` AND relocated from `SandboxConfig` struct field to `VMOption` — dual change avoiding parent §4.1 sketch's accidental duplication). The NEW `internal/filter/http/lua/` package follows the multi-file split per parent §4.4 + this 22.1 SPEC §3.5 (8 production + 5 test files; `lua` single-token Go package identifier matching `cors`/`fault`/`csrf`/`buffer`/`compressor`/`oauth2`/`rbac` precedent — no underscore needed). The filter shape: BOTH `StreamDecoderFilter` AND `StreamEncoderFilter` (`Decoder: non-nil`; `Encoder: non-nil`) — `envoy_on_request` fires at `DecodeHeaders`; `envoy_on_response` fires at `EncodeHeaders`; per-stream `*VM` constructed at `DecodeHeaders` entry via `lua.NewVM(opts...)`; `request_handle`/`response_handle` userdata + metatable set up on `vm.State()` via gopher-lua's `LUserData`/metatable API; `vm.Run(cc.chunk)` executes script top-level (defines globals); `vm.HasGlobalFunc("envoy_on_request")` hook-presence check; `vm.CallGlobal("envoy_on_request", reqHandleUserdata)` hook invocation; respond-state check → `cb.SendLocalReply(captured)` per parent §11.6.7 byte-pin if `:respond()` fired; `OnDestroy` calls `vm.Close()` releasing the `*lua.LState`. The compiled-config holds: `chunk *lua.Chunk` (pre-compiled DefaultSourceCode — single chunk at 22.1; SourceCodes map adds 22.3) + `compileCache *lua.CompileCache` (kept alive for compiledConfig lifetime; GC-driven eviction; no per-route override at 22.1) + `sandbox lua.SandboxConfig` (zero-value at 22.1 = `StrictUpstreamParity` per AMEND-1; no Lua proto knob exposes sandbox configuration — envoy-go-strict departure recorded at BEHAVIOR_CONTRACT.md §13.6 row 1 per parent §14 edit #3) + `stats *filterStats` (3 counters shared across listener; no per-route stat at 22.1 since `LuaPerRoute` PARSE-REJECTs). The 3 counters allocate unconditionally at `New()` time via `newFilterStats(reg, baseStatPrefix(ctx.StatPrefix))` mirroring phase-17 jwt_authn + phase-18.1 ext_authz + phase-19.1 ext_proc + phase-20 oauth2 + phase-21 adaptive_concurrency unconditional-allocation discipline; per ADR-0085 nil-tolerance: `buildCompiledConfig` guards `if ctx.Stats != nil` before `newFilterStats`. The stdlib-sandbox-strict default-deny implementation discipline per this 22.1 SPEC §3.3 + parent §4.3 + AMEND-1: `NewVM` does NOT call `*lua.LState.OpenLibs()` (which opens everything indiscriminately) — instead `NewVM` calls per-lib `OpenXxx` selectively against the resolved `SandboxConfig` (with zero-value defaults applied) → opens only the modules the config permits; for `AllowBaseFull == false`, walks the base globals table after `OpenBase` and `LNil`s out the denied function names (`dofile`/`loadfile`/`loadstring`/`load`/`module`/`require`/`collectgarbage`/`getfenv`/`setfenv`); for `AllowOSTimeHelpers && !AllowOS`, calls `OpenOs` then nils out `os.execute`/`exit`/`remove`/`rename`/`getenv`/`setlocale`/`tmpname` entries on the resulting module table; for `AllowCoroutine` (zero-value-defaulted to true per AMEND-A4 matching upstream `luaL_openlibs`), calls `OpenCoroutine`; for `print`, rebinds `print` to a Go function that writes to `BasePrintSink` if non-nil, otherwise drops the output; for `debug.traceback`, exposes via an INTERNAL global (e.g., `__envoy_traceback`) used by the panic-wrapper, NOT under `debug.traceback`. The bridge `__pairs` metamethod (per parent §11.2 + this 22.1 SPEC §11.2 D7 resolution): snapshots `net/http.Header` map (Go `map[string][]string`; empirically `internal/filter/http/types.go:55` carrier type per the SPEC's D7 closure) into a `[]struct{k,v string}` sorted alphabetically by k (case-insensitive sort via `strings.ToLower` then byte-compare; matches `net/http.Header.Write`'s emit-order discipline) + returns a stateful iterator function that walks the slice by integer index — closes per-run map-iteration non-determinism for script-author debugging without requiring new insertion-order infrastructure across all filter callbacks. The 6 `:logXxx` methods wrap the Go stdlib `"log"` package at the corresponding log level (the canonical project log sink per `extauthz.go:18` + `extproc.go:26` + `rbac.go:6` + `router_h2.go:7` + `extproc/processor.go:52`); format pin `"<LEVEL> lua: <msg>"` preserved across all 6 levels (`:logTrace`/`:logDebug` → `log.Printf("DEBUG lua: %s", msg)`; `:logInfo` → `log.Printf("INFO lua: %s", msg)`; `:logWarn` → `log.Printf("WARN lua: %s", msg)`; `:logErr` → `log.Printf("ERROR lua: %s", msg)`; `:logCritical` → `log.Printf("CRIT lua: %s", msg)`). The `:respond()` byte-pin per parent §11.6.7 + AMEND-7: extract `:status` from headers_table (raise byte-exact `":status must be between 200-599"` if outside `[200,600)` per AMEND-8); auto-set `content-length` from body size when body non-empty; apply `content-type: text/plain` default if headers_table did not supply content-type (per upstream `Utility::prepareLocalReply` at `utility.cc:1241,1273`); capture `respondState` on filter for the decode path to read at the post-CallGlobal phase. `response_handle:respond()` raises byte-exact `"respond not currently supported in the response path"` per AMEND-8. The per-stream `*LState` construction WEAK-default per parent §13-R6 STANDS at 22.1 IMPL Task 12 benchmark sub-task; if benchmark surfaces > 1ms unacceptable overhead, the ADR-0190 escape-valve slot anchors a "per-script-source `*LState` pool with chunk-pre-loaded entries" decision (§Context + §Decision + §Consequences body all land at the same Task 16 commit per ADR-0044). The differential fixture-0026 driver implements the OPTIONAL `BootRejectFixture` interface for scenario (g) (`BootRejectScript() string` returns `"scripts/g_compile_error.lua"` relative to fixture dir; `ExpectedBootErrorSubstring() string` returns `"script load error"`); the runner's NEW `runBootRejectFixture` branch starts BOTH reference Envoy AND envoy-go with the broken script + asserts both sides exit non-zero AND both sides' stderr contains the literal substring `"script load error"`. The 22.1 SPEC §6 16-task breakdown across 5 tiers is the load-bearing input to this PLAN; each Task corresponds 1:1 to a PLAN entry below (Tier A scaffold Tasks 1-5; Tier B bridge methods Tasks 6-9; Tier C stats + tests Tasks 10-12; Tier D differential fixture Tasks 13-15; Tier E atomic landing Task 16). The 16 tasks comfortably fit ADR-0045's 25-task split-gate; the LoC envelope per parent §3.0 estimate (~3000-4000 production+test+fixture IMPL) sits well above the ~1500 LoC PLAN-size soft-gate (the PLAN gate is about PLAN.md size, not IMPL LoC; the IMPL LoC sizing per Task is settled at the 22.1 SPEC §6).

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008; `envoy/extensions/filters/http/lua/v3` for the `Lua` proto + `LuaPerRoute` proto; `envoy/config/core/v3` for `DataSource` 4-arm + `WatchedDirectory` sibling); **NEW direct dependency** `github.com/yuin/gopher-lua v1.1.2` (pure-Go Lua 5.1 interpreter; MIT-licensed; matches upstream Envoy's LuaJIT 5.1 dialect; NO CGO — fits envoy-go's pure-Go portability constraint per ADR-0008) — `go.mod` + `go.sum` updates at Task 1 first action; stdlib `crypto/sha256` for the `CompileCache` content-hash key (32-byte sha256 of script source); stdlib `os` (`os.ReadFile` for the `DataSource.Filename` arm; `os.LookupEnv` for the `DataSource.EnvironmentVariable` arm); stdlib `sort` + `strings` (for the `__pairs` alphabetical-snapshot — `sort.Slice` + `strings.ToLower` then byte-compare); stdlib `sync` (`sync.RWMutex` for the `CompileCache` concurrent-read-add discipline); stdlib `log` (the 6 `:logXxx` bridge methods wrap `"log"` package per the existing filter project precedent); stdlib `bytes` + `io` (for the BootRejectFixture stderr-buffer capture); stdlib `fmt` (for the wire-error wrappings + `"script load error: %v"` boot-reject wording); stdlib `context` (per-stream context threading; no new contracts vs prior phases); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — unchanged); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext upstream backend fixture (NO TLS surface at phase-22.1).

---

## Scope check — why phase 22.1 ships as one sub-phase row (settled at parent BRAINSTORM Q2 PRE-SPLIT discipline)

Phase 22 was PRE-SPLIT THREE-way at the parent BRAINSTORM commit per BRAINSTORM Q2 (envelope D delivered across 22.1 + 22.2 + 22.3 — `22.1-http-filter-lua-vm-and-headers-bridge` foundational third; `22.2-http-filter-lua-full-bridge` full Envoy↔Lua bridge delta; `22.3-http-filter-lua-multi-script-and-per-route` SourceCodes map + LuaPerRoute 3-arm oneof + NEW 9th canonical per-route shape + ADR-0125 §(xiv) AMENDMENT). This PLAN is for the 22.1 sub-phase ONLY; no further nested split per ADR-0106 (sub-sub-phase splits are structurally awkward; matches phase-18.1 + phase-19.1 sub-phase PLAN precedent). The 22.2 + 22.3 sibling stubs at `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/README.md` + `docs/envoy-go/phases/22.3-http-filter-lua-multi-script-and-per-route/README.md` document the deferred surface; the full 22.2 + 22.3 SPECs are drafted at each sub-phase's lifecycle-state 1 after 22.1 phase-done.

The PLAN-time re-evaluation per `superpowers:writing-plans` GATE + ADR-0045 §6 confirms single-sub-phase landing:

- **Task count: 16** — comfortably under the ADR-0045 25-task split-gate. Matches the 22.1 SPEC §6 16-task breakdown verbatim (Tier A 1-5 + Tier B 6-9 + Tier C 10-12 + Tier D 13-15 + Tier E 16).
- **LoC: ~3000-4000 production+test+fixture+docs** (per parent §3.0 estimate; ~1500-2000 LoC `internal/filter/http/lua/` production + tests per parent §15 Layer A; ~600-900 LoC `internal/lua/` production + tests per this 22.1 SPEC §3.2; ~400-700 LoC fixture-0026 + scripts + harness deltas; ~200-400 LoC BEHAVIOR_CONTRACT.md + ADR-0188 + ADR-0189 + STATE.md + ROADMAP.md + PROGRESS.md + REVIEW.md). The LoC sits well above the ~1500 LoC PLAN-size soft-gate, but the PLAN gate is about PLAN.md size (this PLAN at ~1000-1200 LoC sits below the soft-gate), not IMPL LoC — the IMPL LoC sizing per Task is settled at the 22.1 SPEC §6 + this PLAN's per-task File-structure rows refine.
- **Phase 22.1 ships as the single sub-phase row it is** — no further nested split. The 22.1 phase-done squash-merge **CLOSES row 22.1** (in-progress → done) at the same commit; parent row `22` STAYS `in-progress` until 22.3 IMPL phase-done per the sub-row rollup discipline per ADR-0106 + phase-18.1 + phase-18.2 + phase-19.1 + phase-19.2 precedent.

Net change estimate for 22.1 (mirroring the phase-09..21 + phase-18.1 + phase-19.1 PLAN component-table convention):

- `internal/lua/doc.go` ~30-50 (package overview + AMEND-1 sandbox-strict rationale + AMEND-9 gopher-lua-vs-LuaJIT divergence cross-refs + API surface summary; lands at Task 1)
- `internal/lua/vm.go` ~250-350 cumulative (Task 1 skeleton ~80; Task 5 full IMPL ~+170-270 — `NewVM` + `VMOption` + `RegisterGlobalFunc` + `State` + `Run` + `HasGlobalFunc` + `CallGlobal` + `Close` + panic-wrapper + sandbox-config-driven per-stdlib `OpenXxx` selective + post-walk nil-out per §3.3 + `BasePrintSink` redirection)
- `internal/lua/compile.go` ~80-130 (`Chunk` + `CompileCache` + `NewCompileCache` + `CompileScript` + sha256-keyed cache + `sync.RWMutex` + cache nil-tolerance per ADR-0085; lands at Task 4)
- `internal/lua/sandbox.go` ~120-180 (`SandboxConfig` type + per-stdlib `Allow*` defaults + roster-driven `OpenXxx` selective dispatch + denied-function nil-out logic; lands at Task 5)
- `internal/lua/vm_test.go` ~250-400 (Task 5 lifecycle table-driven + option application + `RegisterGlobalFunc` behavior + `Run`/`HasGlobalFunc`/`CallGlobal` + panic-wrapper behavior; Task 12 +concurrency tests N goroutines × NewVM/Run/CallGlobal/Close against same `*Chunk`)
- `internal/lua/compile_test.go` ~150-220 (Task 4 cache hit-on-same-content-hash + cache-miss-on-different-source + nil-cache tolerance; Task 12 +concurrent-read/add tests)
- `internal/lua/sandbox_test.go` ~250-400 (Task 5 per-stdlib ALLOW/DENY exhaustive; verifies `dofile`/`loadfile`/`loadstring`/`load`/`module`/`require`/`io.open`/`io.popen`/`os.execute`/`os.exit`/`debug.getupvalue`/`channel.make`/`package.path` are nil-or-runtime-error post-sandbox-strict construction)
- `internal/filter/http/lua/doc.go` ~50-80 (package overview + Q1-Q12 BRAINSTORM summary + AMEND-1..AMEND-12 cross-refs + D1+D5+D7 cross-refs + API surface; lands at Task 1)
- `internal/filter/http/lua/lua.go` ~150-220 cumulative (Task 1 skeleton ~80 — filter struct + `New` factory stub + `filterStats` stub + `TypeURL` + `filterName` + per-route validator registration; Task 10 +70-140 — `newFilterStats` body + full `New` body wiring; further integration extends across Tasks 6+9+10)
- `internal/filter/http/lua/compiled_config.go` ~250-380 (`compiledConfig` struct + `buildCompiledConfig` + full 18-arm PARSE-REJECT roster per parent §6.2 with byte-stable error wording per parent §6.1; lands at Task 2 with the D1 closure first-action upstream-scrape)
- `internal/filter/http/lua/datasource.go` ~150-220 (4-arm `DataSource` dispatch + `WatchedDirectory` PARSE-REJECT + empty-oneof PARSE-REJECT + per-arm empty-content PARSE-REJECTs per parent §5.3 + AMEND-5 10-arm refinement; lands at Task 3)
- `internal/filter/http/lua/bridge.go` ~400-600 cumulative (Task 6 headers + `__pairs` ~150-200; Task 7 log methods ~80-100; Task 8 streamInfo subset ~80-100; Task 9 respond byte-pin + encode-side runtime-reject ~80-130)
- `internal/filter/http/lua/decode_headers.go` ~100-150 (Task 9 — `DecodeHeaders` per §4.3 dispatch + respond-state integration + `cb.SendLocalReply` firing path)
- `internal/filter/http/lua/encode_headers.go` ~80-120 (Task 9 — `EncodeHeaders` symmetric to decode + AMEND-8 runtime-reject from response_handle)
- `internal/filter/http/lua/stats.go` ~80-120 (Task 10 — 3-counter stat surface + HCM-rooted template + boot-registration metadata)
- `internal/filter/http/lua/lua_test.go` ~400-700 cumulative (Task 1 skeleton; Task 9 +decode/encode integration; Task 10 +stats registration + empty-stat-prefix consecutive-dot verification + cardinality assertion; Task 12 +concurrent per-stream filter dispatch tests via test-double `DecoderFilterCallbacks`)
- `internal/filter/http/lua/compiled_config_test.go` ~400-600 (Task 2 — 18-arm table-driven PARSE-REJECT tests with byte-exact wording assertions per parent §6.2 + the D1-closure upstream-scrape evidence quoted in Task 2 PROGRESS.md entry)
- `internal/filter/http/lua/datasource_test.go` ~300-450 (Task 3 — 4-arm + 10-rejection-leaf table-driven; file-read failures ENOENT/EACCES/EISDIR via `t.TempDir()` synthetic files)
- `internal/filter/http/lua/bridge_test.go` ~600-900 (Task 6 headers + `__pairs` alphabetical-snapshot + cross-run-determinism; Task 7 log-level routing; Task 8 streamInfo subset; Task 9 respond byte-pin per parent §11.6.7 + `:status` range validation + AMEND-8 runtime-reject byte-exact)
- `internal/filter/http/lua/fuzz_test.go` ~80-130 (Task 11 — 28th fuzzer `FuzzLuaConfigParse` per ADR-0018 baseline + corpus seeds)
- `internal/filter/http/lua/testdata/fuzz/FuzzLuaConfigParse/` (Task 11 — corpus seeds ~30 total covering all 18 PARSE-REJECT arms + valid + adversarial)
- `cmd/envoy-go/main.go` ~+2 LoC + +1 import (`lua "github.com/esalaine/envoy-go/internal/filter/http/lua"` alphabetical-among-imports; `httpReg.Register(lua.TypeURL, lua.New)` inserted alphabetical between `localratelimit.New` and `oauth2.New` per ADR-0100 §2.2) + Task 15 `"script load error: "` wrapping ~+30-50 LoC at the boot-reject path
- `test/differential/fixture/fixture.go` ~+15 (NEW `BackendKind=HTTPLua = 22` enum value after `HTTPAdaptiveConcurrency = 21`; dispatcher metadata mirroring AMEND-11)
- `test/differential/runner_test.go` ~+12 (blank import for `internal/filter/http/lua`; switch-case for `HTTPLua`) + Task 13 +~50 LoC `runBootRejectFixture` branch
- `test/differential/harness.go` ~+80-130 (Task 13 — NEW OPTIONAL `BootRejectFixture` driver interface + `tryStartReferenceProxy(ctx, fix) (cancel func(), stderrBuf *bytes.Buffer, err error)` + `tryStartSubjectProxy` variants per parent §11.7.3)
- `test/fixtures/0026-http-lua-headers-bridge/README.md` ~150-250 (Task 14)
- `test/fixtures/0026-http-lua-headers-bridge/envoy.yaml` ~150-250 (Task 14)
- `test/fixtures/0026-http-lua-headers-bridge/envoy-go.yaml` ~150-250 (Task 14)
- `test/fixtures/0026-http-lua-headers-bridge/expectations.yaml` ~100-180 (Task 14; human-readable; NOT consumed by runner)
- `test/fixtures/0026-http-lua-headers-bridge/inputs/driver.go` ~400-600 (Task 14 — registered `Driver` impl + `BootRejectFixture` impl + per-scenario probes + `/stats` admin scrape for scenario (e) D3 executions-counter delta)
- `test/fixtures/0026-http-lua-headers-bridge/scripts/a_add_header.lua` ~5 (Task 14)
- `test/fixtures/0026-http-lua-headers-bridge/scripts/b_replace_header.lua` ~5 (Task 14)
- `test/fixtures/0026-http-lua-headers-bridge/scripts/c_remove_header.lua` ~5 (Task 14)
- `test/fixtures/0026-http-lua-headers-bridge/scripts/d_respond.lua` ~5 (Task 14)
- `test/fixtures/0026-http-lua-headers-bridge/scripts/e_log_only.lua` ~5 (Task 14)
- `test/fixtures/0026-http-lua-headers-bridge/scripts/f_headers_iter.lua` ~10 (Task 14)
- `test/fixtures/0026-http-lua-headers-bridge/scripts/g_compile_error.lua` ~5 (Task 14; intentionally broken Lua)
- `go.mod` + `go.sum` ~+5 LoC delta (NEW `github.com/yuin/gopher-lua v1.1.2` direct dep; transitive deps if any; Task 1)
- `docs/envoy-go/DECISIONS.md` — 2 ADR §Decision + §Consequences bodies anchored at Task 16 (ADR-0188 + ADR-0189; CONDITIONAL ADR-0190 only if R6 escape-valve fires); ~+400-600 LoC delta
- `docs/envoy-go/BEHAVIOR_CONTRACT.md` ~+250-400 LoC (Task 16 7-edit bundle per parent §14 + this 22.1 SPEC §14)
- `docs/envoy-go/ROADMAP.md` row 22.1 flips `in-progress → done` at Task 16; per-cell IMPL-done annotation; parent row `22` UNCHANGED; sub-rows `22.2` + `22.3` UNCHANGED `planned`; ~+1 net
- `docs/envoy-go/STATE.md` rewrite-in-place at Task 16
- `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md` (NEW) ~700-1000 across 16 task entries
- `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/REVIEW.md` (NEW) ~300-400

**Production code: ~2200-3300 LoC** (`internal/lua/` ~600-900 + `internal/filter/http/lua/` ~1500-2200 + harness delta ~80-130 + boot-reg ~30-50 + enum +15 + runner-switch +12) **+ ~2100-3300 LoC tests** + ~900-1500 LoC fixture-0026 (including 7 `.lua` scripts ~40 LoC) + ~1000-1400 LoC docs ≈ **~6200-9500 LoC total**. **Task count: 16** — comfortably under the ADR-0045 25-task split-gate.

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/lua/doc.go` | NEW | Package doc per 22.1 SPEC §3.2 — package overview; AMEND-1 sandbox-strict rationale (envoy-go-strict DEPARTURE from upstream's bare-`luaL_openlibs` posture; per-stream goroutine dispatch model cannot make per-worker-VM-scoping assumption); AMEND-9 gopher-lua-vs-LuaJIT runtime-error-log-message-wording divergence cross-reference; AMEND-A4 coroutine ALLOWED matching upstream cross-reference; API surface summary (`VM` + `VMOption` + `NewVM` + `State`/`RegisterGlobalFunc`/`Run`/`HasGlobalFunc`/`CallGlobal`/`Close` + `Chunk` + `CompileCache` + `NewCompileCache` + `CompileScript` + `SandboxConfig` + `PanicHandlerFn` + zero-value `StrictUpstreamParity` defaults); ADR-0188 cross-reference. ~30-50 LoC. Lands at Task 1. |
| `internal/lua/vm.go` | NEW | `VM` type per 22.1 SPEC §3.1 production signatures (`VM` struct with unexported `state *lua.LState` + `sandbox SandboxConfig` + `panicH PanicHandlerFn` + `printSink io.Writer`); `VMOption` function-option pattern; `WithSandboxConfig(sb SandboxConfig) VMOption`; `WithPanicHandler(h PanicHandlerFn) VMOption`; `WithBasePrintSink(w io.Writer) VMOption`; `NewVM(opts ...VMOption) *VM` (sandbox-config-driven per-stdlib `OpenXxx` selective + post-walk nil-out per §3.3 implementation discipline + panic-wrapper setup + `print`-redirect rebinding + INTERNAL `__envoy_traceback` global registration for panic-wrapper's use); `State() *lua.LState` escape-hatch (for filter consumers to set up userdata + metatables); `RegisterGlobalFunc(name string, fn lua.LGFunction)` convenience; `Run(chunk *Chunk) error` (loads `*FunctionProto` onto `*LState` + PCalls top-level; returns `*lua.ApiError` on Lua-runtime error); `HasGlobalFunc(name string) bool` (GetGlobal name + LFunction-type check); `CallGlobal(name string, args ...lua.LValue) error` (PCall with args; returns `*lua.ApiError`); `Close()` (idempotent `state.Close`); `PanicHandlerFn func(recovered any)`. Task 1 lands SKELETON (type + stubs returning zero values); Task 5 lands FULL IMPL. ~250-350 LoC cumulative. |
| `internal/lua/compile.go` | NEW | `Chunk` struct per 22.1 SPEC §3.1 (`proto *lua.FunctionProto` + `hash [32]byte` sha256(src)); `CompileCache` type (`sync.RWMutex` + `store map[[32]byte]*Chunk`); `NewCompileCache() *CompileCache`; `CompileScript(src []byte, cache *CompileCache) (*Chunk, error)` (sha256 content-hash key; cache hit returns existing; cache miss compiles via gopher-lua parser; cache nil-tolerance per ADR-0085 = compiles uncached). Returns `*lua.ApiError` wrapped as plain error on compile failure (the `lua.New(opts).LoadString(string(src))` path + `lua.LState.NewFunctionFromProto` discipline). Lands at Task 4 full IMPL. ~80-130 LoC. |
| `internal/lua/sandbox.go` | NEW | `SandboxConfig` struct per 22.1 SPEC §3.1 (8 `Allow*` fields: `AllowBaseFull`/`AllowIO`/`AllowOS`/`AllowOSTimeHelpers`/`AllowDebug`/`AllowPackage`/`AllowChannel`/`AllowCoroutine`; zero-value posture = `StrictUpstreamParity` per AMEND-1 — `AllowOSTimeHelpers` zero-valued false BUT NewVM's zero-value helper sets it to true to match upstream-parity time-helpers arm; `AllowCoroutine` zero-valued false BUT NewVM's zero-value helper sets it to true matching upstream `luaL_openlibs`); per-stdlib `OpenXxx` selective dispatch helpers (NOT `OpenLibs()`); base-globals post-walk nil-out logic for the 9 denied base-globals (`dofile`/`loadfile`/`loadstring`/`load`/`module`/`require`/`collectgarbage`/`getfenv`/`setfenv`); os-module post-walk nil-out logic for the 7 denied os entries when `AllowOSTimeHelpers && !AllowOS`. Lands at Task 5. ~120-180 LoC. |
| `internal/lua/vm_test.go` | NEW | VM lifecycle table-driven (NewVM/Close idempotency; double-Close); option application verification (`WithSandboxConfig` + `WithPanicHandler` + `WithBasePrintSink` independently); `RegisterGlobalFunc` behavior (Go callback exposed as Lua-callable; verifies arg marshaling); `Run` happy + compile-clean script + script-error path (`*lua.ApiError` returned); `HasGlobalFunc` true/false table-driven (defined / undefined / non-function global); `CallGlobal` happy + arg push + return-value handling; panic-wrapper behavior tests (Go panic in bridge callback → recover() invokes `PanicHandlerFn` → converts to error return; the recovered value is passed through). Task 5 lands the bulk; Task 12 adds concurrency tests (N goroutines each call NewVM/Run/CallGlobal/Close against the same `*Chunk` from the same `*CompileCache`; assert no cross-VM state leak; race-free under `-race`). ~250-400 LoC cumulative. |
| `internal/lua/compile_test.go` | NEW | `NewCompileCache` returns non-nil empty cache; `CompileScript` happy paths (valid Lua source); `CompileScript` cache-hit-on-same-content-hash (same `[]byte` → same `*Chunk` pointer); `CompileScript` cache-miss-on-different-source (different `[]byte` → different `*Chunk` pointer); `CompileScript(src, nil)` nil-cache behavior (compiles uncached, no caching side effects); `CompileScript` compile-error path (`*lua.ApiError` wrapped as plain `error`). Task 4 lands the bulk; Task 12 adds `TestCompileCache_ConcurrentReadAdd_*` (N goroutines mix CompileScript with same hash and CompileScript with new content; assert no data race + correct cache contents) under `t.Parallel()`. ~150-220 LoC cumulative. |
| `internal/lua/sandbox_test.go` | NEW | Per-stdlib ALLOW/DENY exhaustive table-driven per 22.1 SPEC §3.3 roster: verify `dofile`/`loadfile`/`loadstring`/`load`/`module`/`require`/`collectgarbage`/`getfenv`/`setfenv` are nil-or-runtime-error post-sandbox-strict construction (AllowBaseFull=false default); `io.open`/`io.popen` nil-or-runtime-error (AllowIO=false default; `io` module itself NOT opened); `os.execute`/`os.exit`/`os.remove`/`os.rename`/`os.getenv`/`os.setlocale`/`os.tmpname` nil-or-runtime-error (AllowOS=false; AllowOSTimeHelpers=true default → `os.time`/`os.date`/`os.clock`/`os.difftime` ARE callable); `debug.getupvalue`/`debug.setupvalue`/`debug.setfenv` nil-or-runtime-error (AllowDebug=false default; `debug` module itself NOT opened); `package.path`/`package.cpath`/`package.loaded` nil-or-runtime-error (AllowPackage=false default); `channel.make`/`channel.send`/`channel.receive` nil-or-runtime-error (AllowChannel=false default); `coroutine.create`/`coroutine.yield`/`coroutine.resume` ARE callable (AllowCoroutine=true default). Plus opt-in tests verifying that enabling each flag exposes the corresponding module. Lands at Task 5. ~250-400 LoC. |
| `internal/filter/http/lua/doc.go` | NEW | Package doc per 22.1 SPEC §3.5 — package overview; Q1-Q12 BRAINSTORM decision summary (envelope D + 3-way pre-split + gopher-lua + EXTRACT-NOW framework primitive + 4-arm DataSource + pragmatic-middle bridge + NEW 9th canonical at 22.3 + 3-counter stats + full cross-side fixture + WEAK HOLD escape-valve + long-prefix slugs + pre-create-all directories); AMEND-1..AMEND-12 cross-references; D1 + D5 + D7 cross-references; API surface summary (`TypeURL` + `New` factory); ADR-0189 cross-reference. ~50-80 LoC. Lands at Task 1. |
| `internal/filter/http/lua/lua.go` | NEW | Filter struct + `New` factory (`HTTPFilterFactory` per ADR-0072); `filterStats` struct (3 counters); `TypeURL = "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"`; `filterName = "envoy.filters.http.lua"`; per-route validator registration (one-liner returning the arm-18 PARSE-REJECT). Filter struct holds the per-stream state: `cc *compiledConfig` + `dcb DecoderFilterCallbacks` + `ecb EncoderFilterCallbacks` + `vm *lua.VM` + `respondCaptured *respondState`. `var _ StreamDecoderFilter = (*filter)(nil)` + `var _ StreamEncoderFilter = (*filter)(nil)` compile-time assertions. Task 1 lands SKELETON; Task 10 lands FULL `newFilterStats` body + `New` factory body wiring (cross-references Task 6 + Task 9). ~150-220 LoC cumulative. |
| `internal/filter/http/lua/compiled_config.go` | NEW | `compiledConfig` struct per 22.1 SPEC §4.2 (`chunk *lua.Chunk` + `compileCache *lua.CompileCache` + `sandbox lua.SandboxConfig` + `stats *filterStats`); `buildCompiledConfig(ctx, raw *luav3.Lua) (*compiledConfig, error)` full body covering 18-arm PARSE-REJECT roster per parent §6.2 with byte-stable error wording per parent §6.1: arms 1-2 universal `typed-config-*`; arm 3 `inline-code-deprecated-rejected` (`"lua: inline_code is deprecated; use default_source_code (envoy-go-strict)"`); arm 4 `source-codes-deferred-to-22-3` (`"lua: source_codes map is not yet supported (lands in phase 22.3)"`); arm 5 `default-source-code-required` (`"lua: default_source_code is required"` — anticipated PARSE-REJECT per D1 closure at Task 2 first action); arms 6-15 DataSource 10-arm dispatch + per-arm empty-content checks (delegated to `datasource.go` per Task 3); arm 16 `script-compile-failed` (`"lua: default_source_code: compile: %w"` wrapping `*lua.ApiError`); arm 17 `script-missing-required-hooks` (`"lua: script defines neither envoy_on_request nor envoy_on_response"` — anticipated PARSE-REJECT per D1 closure at Task 2 first action; subject to upstream-scrape REFUTE → flip to silent no-op); arm 18 `per-route-deferred-to-22-3` (registered via `RegisterPerRouteValidator`). Defaults applied per parent §6.1 (`stat_prefix` empty → consecutive-dot template; `sandbox` zero-value = StrictUpstreamParity). Lands at Task 2. ~250-380 LoC. |
| `internal/filter/http/lua/datasource.go` | NEW | 4-arm `DataSource` arm dispatch per parent §5.3 + AMEND-5 10-arm refinement: `Filename` → `os.ReadFile`; `InlineBytes` → verbatim; `InlineString` → byte-cast; `EnvironmentVariable` → `os.LookupEnv`; `WatchedDirectory` → PARSE-REJECT (deferred to future Runtime/RTDS family phase); empty-oneof → PARSE-REJECT; per-arm empty-content PARSE-REJECTs (filename name-empty / ENOENT / zero-byte; env-var name-empty / unset / empty-value). `resolveDataSource(ds *corev3.DataSource) ([]byte, error)` returns `([]byte, error)` for downstream `CompileScript` consumption. Byte-stable error wording per parent §6.2 arms 6-15. Lands at Task 3. ~150-220 LoC. |
| `internal/filter/http/lua/bridge.go` | NEW | `request_handle`/`response_handle` userdata + metatable setup. 7 headers-object methods (`:get`/`:getAtIndex`/`:getNumValues`/`:add`/`:append`/`:remove`/`:replace`); `__pairs` metamethod alphabetical-snapshot per §11 D7 (snapshots `net/http.Header` map into `[]struct{k,v string}` sorted alphabetically by k via `strings.ToLower` then byte-compare; returns stateful iterator walking by integer index). 6 log methods (`:logTrace`/`:logDebug`/`:logInfo`/`:logWarn`/`:logErr`/`:logCritical` wrapping `"log"` package). `streamInfo` userdata + 4 methods (`:protocol`/`:routeName`/`:downstreamLocalAddress`/`:downstreamDirectRemoteAddress`). `:respond(headers_table, body_string)` per parent §11.6.7 byte-pin + AMEND-7 (extract `:status` from headers_table; validate `[200,600)` per AMEND-8 byte-exact `":status must be between 200-599"`; auto-set `content-length`; apply `content-type: text/plain` default if not supplied; capture `respondState` on filter). `response_handle:respond()` raises byte-exact `"respond not currently supported in the response path"` per AMEND-8. `requestHandleContext`/`responseHandleContext`/`respondState` Go structs per 22.1 SPEC §4.3. Task 6 lands headers + `__pairs`; Task 7 lands log; Task 8 lands streamInfo; Task 9 lands respond + encode-side reject. ~400-600 LoC cumulative. |
| `internal/filter/http/lua/decode_headers.go` | NEW | `DecodeHeaders(headers http.Header, endStream bool)` per 22.1 SPEC §4.3 dispatch: construct per-stream `*VM`; set up `request_handle` userdata + metatable on `vm.State()`; `vm.Run(cc.chunk)` (executes top-level; if errors, `cc.stats.errors++` + log; continue); `vm.HasGlobalFunc("envoy_on_request")` (if not defined, pass-through); `cc.stats.executions++`; `vm.CallGlobal("envoy_on_request", reqUd)` (if errors, `cc.stats.errors++` + log); respond-state check (if `f.respondCaptured != nil`, `cc.stats.respondCalls++` + `cb.SendLocalReply(captured.status, captured.body, captured.headers)` + return `StopIteration`); else return `Continue`. `OnDestroy` calls `vm.Close()`. Lands at Task 9. ~100-150 LoC. |
| `internal/filter/http/lua/encode_headers.go` | NEW | `EncodeHeaders(headers http.Header, endStream bool)` symmetric to decode for `envoy_on_response` + `response_handle`; `:respond()` raises AMEND-8 runtime-error string at the bridge layer (response_handle has its own metatable mapping `:respond` to the rejection callback). Lands at Task 9. ~80-120 LoC. |
| `internal/filter/http/lua/stats.go` | NEW | 3-counter `filterStats` struct per parent §7 + AMEND-3 (`errors *stats.Counter` upstream-parity per AMEND-3 + `lua_filter.cc:811`; `executions *stats.Counter` upstream-parity per AMEND-3 + `lua_filter.cc:872`; `respondCalls *stats.Counter` envoy-go-strict per AMEND-3 corrected); `newFilterStats(reg *stats.Registry, hcmStatPrefix string, configStatPrefix string) *filterStats` constructs via `reg.NewCounter` under HCM-rooted template `http.<hcmStatPrefix>.lua.<configStatPrefix>.<stat>` per parent §7.2 + AMEND-2. Package-level const declarations for the 3 stat names (`statNameErrors = "errors"`; `statNameExecutions = "executions"`; `statNameRespondCalls = "respond_calls"`); table-driven `TestStatNames_Equal_*` test in `lua_test.go` asserts the 3 constants byte-exact against the wire-expected names per ADR-0143 SN2-reuse. Lands at Task 10. ~80-120 LoC. |
| `internal/filter/http/lua/lua_test.go` | NEW | Filter + factory integration tests (`TypeURL` constant assertion; `New` happy path with each DataSource arm); `filterStats` 3-counter registration + cardinality assertion + empty-stat-prefix consecutive-dot wire name verification (per AMEND-2 `http.<HCM>.lua..errors` literal); `TestStatNames_Equal_*` table-driven 3-stat-name byte-exact assertion; DecodeHeaders + EncodeHeaders integration end-to-end (compile script → VM construct → Run → CallGlobal → respond-state check) via test-double `DecoderFilterCallbacks` + `EncoderFilterCallbacks`. Task 1 lands skeleton; Task 9 + Task 10 + Task 12 extend across decode/encode + stats + concurrency. ~400-700 LoC cumulative. |
| `internal/filter/http/lua/compiled_config_test.go` | NEW | 18-arm table-driven PARSE-REJECT tests per parent §6.2: each row is `{name string, configMutator func(*luav3.Lua), wantErrSubstring string}`. ~20-25 rows covering arms 1-18 + 5-10 valid-config rows (each DataSource arm with valid contents). Each PARSE-REJECT row asserts `err != nil && err.Error() == "<expected wording>"`. The D1 closure first-action upstream-scrape evidence quoted in Task 2 PROGRESS.md entry; tests for arms 5 + 17 honor the resolved disposition (PARSE-REJECT anticipated; if REFUTED at upstream-scrape, flip to expected-silent-no-op test). Lands at Task 2. ~400-600 LoC. |
| `internal/filter/http/lua/datasource_test.go` | NEW | 4-arm `DataSource` resolution + 10-rejection-leaf table-driven per parent §11.1 + AMEND-5: `Filename` happy + ENOENT + EACCES + EISDIR + zero-byte + name-empty (via `t.TempDir()` synthetic files); `InlineBytes` happy + zero-byte; `InlineString` happy + empty-string; `EnvironmentVariable` happy + unset + empty-value + name-empty; `WatchedDirectory` PARSE-REJECT; empty-oneof PARSE-REJECT. Byte-exact error wording assertions per parent §6.2 arms 6-15. Lands at Task 3. ~300-450 LoC. |
| `internal/filter/http/lua/bridge_test.go` | NEW | 7 headers-method tests (case-insensitive name lookup; `:get` returns first value or nil; `:getAtIndex` 1-indexed; `:getNumValues` count; `:add`/`:append` appends; `:remove` deletes; `:replace` removes-then-adds). `__pairs` alphabetical-snapshot verification + cross-run-determinism test (run `__pairs` N times against same headers; assert same order each time). 6 log-method tests via captured-log-sink test double (verifies each method routes to correct log level + format pin). 4 streamInfo-method tests via test-double `DecoderFilterCallbacks` carrying canned protocol/route/address values. `:respond()` full byte-pin verification per parent §11.6.7 4-tuple `{status: 403, content-length: 6, content-type: text/plain, body: "denied"}`. `:status` range validation byte-exact `":status must be between 200-599"`. encode-side `:respond()` runtime-reject byte-exact `"respond not currently supported in the response path"`. Task 6 lands bulk; Task 7 + Task 8 + Task 9 extend. ~600-900 LoC cumulative. |
| `internal/filter/http/lua/fuzz_test.go` | NEW | 28th project-wide fuzzer `FuzzLuaConfigParse` per ADR-0018 baseline. Must-never-panic across `New()`. Corpus seeds ~30 total: one seed per PARSE-REJECT arm per parent §6.2 (18 seeds) + ~5 valid-config seeds (each DataSource arm valid) + ~7 adversarial-Lua-source seeds (syntax errors triggering arm 16; sandbox-breaking attempts that compile-clean but error at runtime — must-never-panic). Lands at Task 11. ~80-130 LoC + `testdata/fuzz/FuzzLuaConfigParse/` corpus directory. |
| `cmd/envoy-go/main.go` | MODIFY | +1 LoC + +1 import + Task 15 ~+30-50 LoC. Task 10: add import `lua "github.com/esalaine/envoy-go/internal/filter/http/lua"` (alphabetical-among-imports; between `localratelimit` and `oauth2`); add `httpReg.Register(lua.TypeURL, lua.New)` alphabetical between `localratelimit.New` and `oauth2.New` per ADR-0100 §2.2 (17 HTTP filters wired post-22.1; was 16). Task 15: wrap gopher-lua compile error with `"script load error: %v"` prefix at boot-reject path (`main.go:60-66` per parent §13-W) ensuring boot-reject stderr contains the literal substring `"script load error"` matching upstream Envoy v1.37.2 per parent §11.7.5. |
| `test/differential/fixture/fixture.go` | MODIFY | +1 enum value `HTTPLua BackendKind = 22` (after `HTTPAdaptiveConcurrency = 21`); ~+15 LoC including doc-comment per existing BackendKind comment style. Lands at Task 13. |
| `test/differential/runner_test.go` | MODIFY | +blank import for `internal/filter/http/lua`; +switch-case for `HTTPLua` (~+12 LoC); +`runBootRejectFixture` branch ~+50 LoC paralleling `runReferenceLessFixture` at `runner_test.go:1268`. Lands at Task 13. |
| `test/differential/harness.go` | MODIFY | NEW OPTIONAL `BootRejectFixture` driver interface per parent §13-R1 + this 22.1 SPEC §9.2 (`BootRejectScript() string` + `ExpectedBootErrorSubstring() string`); `tryStartReferenceProxy(ctx, fix) (cancel func(), stderrBuf *bytes.Buffer, err error)` + `tryStartSubjectProxy` variants paralleling existing `StartReferenceProxy`/`StartSubjectProxy` but returning boot error + stderr buffer instead of `t.Fatalf`-ing. ~80-130 LoC delta per parent §11.7.3. Lands at Task 13. |
| `test/fixtures/0026-http-lua-headers-bridge/README.md` | NEW | Top-level fixture-directory README — scope + 7-scenario table + topology + cross-refs to parent SPEC §8 + this 22.1 SPEC §9 + ADR-0188 + ADR-0189. ~150-250 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/envoy.yaml` | NEW | Reference Envoy bootstrap; single listener + lua filter consuming `Lua.DefaultSourceCode` via `Filename` arm pointing to `scripts/<scenario>.lua`; templated `{{.BackendPort}}`. ~150-250 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/envoy-go.yaml` | NEW | Subject bootstrap; same topology; templated `{{.AdminPort}} {{.ListenerPort}} {{.BackendPort}} {{.FixtureDir}}`. ~150-250 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/expectations.yaml` | NEW | Human-readable declarative scenario expectations (NOT consumed by runner; documentation aid). ~100-180 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/inputs/driver.go` | NEW | Registered `Driver` impl + `BootRejectFixture` impl for scenario (g); per-scenario probes via `driveProxy` + `emitScenario` + `classifyBody` mirroring fixture-0023's pattern; for scenario (e) executes the request then scrapes `/stats` (per the D3 PLAN-session closure at option (a) + the fixture-0025 inline `/stats` scrape precedent) emitting `scenario e executions_delta=N` into the byte-comparison buffer; assert cross-side `executions_delta=1` per probe. ~400-600 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/scripts/a_add_header.lua` | NEW | `function envoy_on_request(rh) rh:headers():add("x-lua-injected", "hello") end`. ~5 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/scripts/b_replace_header.lua` | NEW | `function envoy_on_request(rh) rh:headers():replace("user-agent", "envoy-go-lua/1.0") end`. ~5 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/scripts/c_remove_header.lua` | NEW | `function envoy_on_request(rh) rh:headers():remove("x-blocked") end`. ~5 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/scripts/d_respond.lua` | NEW | `function envoy_on_request(rh) rh:respond({[":status"]="403"}, "denied") end`. ~5 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/scripts/e_log_only.lua` | NEW | `function envoy_on_request(rh) rh:logInfo("lua hit") end`. ~5 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/scripts/f_headers_iter.lua` | NEW | `function envoy_on_request(rh) local n=0; for k,v in pairs(rh:headers()) do n=n+1 end; rh:headers():add("x-headers-count", tostring(n)) end`. ~10 LoC. Lands at Task 14. |
| `test/fixtures/0026-http-lua-headers-bridge/scripts/g_compile_error.lua` | NEW | Intentional Lua syntax error (e.g., `function envoy_on_request(rh) end this-is-not-valid-lua-syntax`). ~5 LoC. Lands at Task 14; consumed by `BootRejectFixture` driver impl. |
| `go.mod` + `go.sum` | MODIFY | +`github.com/yuin/gopher-lua v1.1.2` direct dep; transitive deps if any; `go mod tidy` clean. Lands at Task 1. |
| `docs/envoy-go/DECISIONS.md` | MODIFY | 2 ADR §Decision + §Consequences bodies anchored at Task 16 (ADR-0188 — NEW `internal/lua/` framework primitive per parent §4.1 + this 22.1 SPEC §3.1 production signatures + §3.2 file split + §3.3 sandbox roster + §3.4 per-stream lifecycle + EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 per BRAINSTORM Q4; ADR-0189 — NEW `internal/filter/http/lua/` package shape per parent §4.4 + this 22.1 SPEC §3.5 file split + §4 compiledConfig + §6.2 18-arm PARSE-REJECT roster + §8 fixture-0026 disposition + §11.1 D5 + §11.2 D7 + Task 2 D1 closure evidence + §13-R1 BootRejectFixture infrastructure + §13-W wording-pinning discipline). CONDITIONAL ADR-0190 §Context + §Decision + §Consequences body only if R6 escape-valve fires per Task 12 benchmark > 1ms threshold. ~+400-600 LoC delta. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY | Task 16 7-edit bundle per parent §14 + this 22.1 SPEC §14: (1) NEW `### envoy.filters.http.lua` subsection ~80-120 LoC; (2) Stat-table 99 → 102 extension under `## Stat surface` + extension summary paragraph; (3) envoy-go-strict departure record #1: stdlib-sandbox-strict per AMEND-1; (4) envoy-go-strict departure record #2: `respond_calls` counter per AMEND-3; (5) envoy-go-strict departure record #3: runtime-error log-message wording per AMEND-9; (6) NEW `### Phase 22.1 forward-pointer notes` subsection ~30-50 LoC; (7) Per-route-canonical cross-reference caption update 1-line edit. ~+250-400 LoC delta. |
| `docs/envoy-go/ROADMAP.md` | MODIFY | Row 22.1 flips `in-progress → done` at Task 16; per-cell IMPL-done annotation; parent row `22` STAYS `in-progress`; sub-rows `22.2` + `22.3` UNCHANGED `planned`. ~+1 net. |
| `docs/envoy-go/STATE.md` | MODIFY | Rewrite-in-place at Task 16: `lifecycle-state: phase 22.1 IMPL done; awaiting 22.2 SPEC`; `next-skill: superpowers:brainstorming` (22.2 BRAINSTORM scoped to 22.2 sub-phase); `next-free ADR: ADR-0190` (UNCHANGED if R6 escape-valve does NOT fire) or `ADR-0191` (if fires); 99 → 102 stat-count update; 16 → 17 HTTP filter count update; ADR tail advance to ADR-0189 (or ADR-0190 if escape-valve fires); 27 → 28 fuzzer count; per-task SHA-fill follow-up commit per phase-09..21 convention. |
| `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md` | NEW | Append-only task log per phase-21 IMPL precedent + `superpowers:verification-before-completion` discipline; 16 task entries; each entry quotes command outputs verbatim + records acceptance-criteria evidence per task. ~700-1000 LoC. |
| `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/REVIEW.md` | NEW | Task 16 reviewer artifact per `superpowers:requesting-code-review` per phase-21 IMPL precedent; per-task review notes + cross-cutting review notes + green-light evidence + 24-item acceptance checklist closure per this 22.1 SPEC §15.2. ~300-400 LoC. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 + PLAN-emerged decisions)

The planner is required by 22.1 SPEC §12 to settle the D3 RATIFIED-PENDING-PLAN-TIME item at this PLAN session (per parent §12-D3 + §11.7.7 RECOMMENDED option (a) anticipated); the D1 RATIFIED-PENDING-IMPL-TIME item closes at IMPL Task 2 first action per this 22.1 SPEC §12-D1 (PLAN does NOT resolve D1 — that's IMPL-time empirical scrape). D5 + D7 already CLOSED at the 22.1 SPEC commit per §11.1 + §11.2. Additional PLAN-emerged decisions D-P1..D-P10 settle here.

1. **D3 — scenario (e) `:logInfo()` cross-side assertion shape — LOCKED at OPTION (A) STAT-COUNTER DELTA per parent §11.7.7 RECOMMENDED.** The 22.1 SPEC §12-D3 carries forward to this PLAN session per parent §12-D3 anchored resolution point. Three options were on the table: (a) drop the "envoy log message" assertion from cross-side scope + rely on `lua.<prefix>.executions` stat-counter delta to confirm the script ran; (b) add `:logInfo()` calls that ALSO bump a counter the driver can scrape (artificial — pollutes the script); (c) introduce a NEW `LogAsserter` interface paralleling `AccessLogAsserter` (heavier infra delta). **LOCKED at option (a).** Rationale: (i) option (a) requires ZERO new harness/runner infrastructure — the existing `/stats` admin scrape mechanism (fixture-0025's inline `scrapeStats(ctx, adminAddr)` + `emitStatsSnapshot(&buf, statsBody)` per `test/fixtures/0025-http-adaptive-concurrency/inputs/driver.go:149-153,236-272` lines) supplies the byte-comparison content directly; (ii) the cross-side determinism HOLDS because both reference Envoy AND envoy-go expose `lua.<prefix>.executions` under their respective `/stats` endpoints in byte-equivalent form per parent AMEND-3 `executions` upstream-parity reclassification; (iii) option (b) was rejected as it pollutes the script with non-functional assertions visible to script authors (sets a bad precedent for 22.2 + 22.3 scripts that need cleaner test shapes); (iv) option (c) was rejected as the `LogAsserter` infra delta (~150-300 LoC harness extension + runner integration + per-fixture wiring) is disproportionate to the single assertion it would close at 22.1 (the existing 27 fixtures all live without a log-comparison primitive). Scenario (e) `e_log_only.lua` wire-output column reads: **"Request unchanged at upstream; `lua.<prefix>.executions` counter delta = 1 per probe."** The fixture-0026 driver implementation: probe the request → scrape `/stats?format=text` admin endpoint → diff pre-probe vs post-probe → emit `scenario e executions_delta=N` into the byte-comparison buffer; cross-side assertion `executions_delta=1` per probe. Settles parent §12-D3 + 22.1 SPEC §12 D3 + §15.2 item 22 + this PLAN's File-structure `driver.go` row. *Anchored: parent §11.7.7 RECOMMENDED option (a) + this 22.1 PLAN session per 22.1 SPEC §12-D3 carry-forward anchor + fixture-0025 inline `/stats` scrape precedent.*

2. **D-P1 — SPEC §6 16-task numbering INHERITED VERBATIM; PROGRESS.md preamble + precondition check is "Pre-Task 0" (NOT a renumbered Task 1).** Settle: this PLAN's Tasks 1-16 map 1:1 to the 22.1 SPEC §6 16-task breakdown per the cold-start prompt's explicit instruction ("Inherit 22.1 SPEC §6 16-task breakdown verbatim. The PLAN does NOT re-decide task structure"). The PROGRESS.md preamble + 15-precondition verification (which the phase-21 PLAN names Task 1 because that SPEC did not pre-allocate task numbers in §6) is here labeled **Pre-Task 0** + executed at IMPL session cold-start before SPEC §6 Task 1 begins. This preserves the SPEC §6 inheritance discipline while honoring the precedent's PROGRESS.md-preamble-at-the-start ritual. *Anchored: 22.1 PLAN cold-start prompt verbatim + phase-21 PROGRESS.md ritual precedent.*

3. **D-P2 — Per-task subagent dispatch type LOCKED at `general-purpose` for code Tasks 1-15; `superpowers:writing-skills` NOT applicable; `superpowers:code-reviewer` (NOT a generic agent type) NOT needed mid-IMPL; Task 16 atomic landing dispatched via `general-purpose` with explicit acceptance-checklist reference.** Settle: per project memory `feedback_execution_style.md` (user always wants subagent-driven over inline execution for plans), each Task's IMPL session subagent-dispatches per `superpowers:subagent-driven-development`. Dispatch type per Task: Tasks 1-15 use `general-purpose` agent (Go code work; no specialized agent type matches more precisely); Task 16 uses `general-purpose` with explicit reference to 22.1 SPEC §15.2 24-item acceptance checklist + the BEHAVIOR_CONTRACT.md 7-edit bundle anatomy + the ADR-0188 + ADR-0189 §Decision + §Consequences body sketches from this 22.1 SPEC §3.1 + §3.5 + §13. REVIEW.md at IMPL Task 16 final step dispatched via `superpowers:code-reviewer` per `superpowers:requesting-code-review`. *Anchored: project memory `feedback_execution_style.md` + phase-09..21 + phase-18.1 + phase-19.1 IMPL precedent + `superpowers:subagent-driven-development` skill.*

4. **D-P3 — Per-task PROGRESS.md entry shape LOCKED per phase-21 IMPL precedent.** Settle: each Task's PROGRESS.md entry contains the following sections in order:
   - **Task ID + title** (matches the SPEC §6 task ID + title verbatim + this PLAN's task heading);
   - **Acceptance criteria** (verbatim cross-reference to 22.1 SPEC §6's "Acceptance:" line for the task + this PLAN's Task heading's `Acceptance:` line);
   - **Files touched** (the precise list from this PLAN's Task heading's `Files:` block);
   - **Verification command outputs** (the exact commands from this PLAN's Task Step bodies' Run-tests-verify-they-pass phase + the verbatim stdout/stderr quoted in fenced code blocks per `superpowers:verification-before-completion` discipline);
   - **Acceptance-criteria evidence** (per-criterion pass/fail with brief reasoning + cross-reference to the verification command output that demonstrates the pass);
   - **D-decision-disposition update** (if the task closes or refines a D-decision — e.g., Task 2 closes D1; the entry records the empirical evidence + the resolved disposition);
   - **Commit SHA** (`git log -1 --format=%H` for the task's commit);
   - **Tier + Task-number cross-reference** (e.g., "Tier A scaffold (Task 2 of 5 in tier; Task 2 of 16 overall)").
   *Anchored: phase-21 + phase-18.1 + phase-19.1 PROGRESS.md format precedent + `superpowers:verification-before-completion` discipline + this PLAN's per-Task structure.*

5. **D-P4 — Per-task TDD ordering LOCKED at test-first for ALL 16 Tasks per `superpowers:test-driven-development` rigid discipline.** Settle: every Task that lands production code at IMPL (Tasks 1-15; Task 16 is the atomic-landing meta-task with no new production code beyond doc edits) follows the rigid TDD ordering: (Step 1) write the failing test in the corresponding `*_test.go` file; (Step 2) run the test to verify it fails (compile-error OR assertion-failure with expected error); (Step 3) implement the minimal production code to make the test pass; (Step 4) run the test to verify it passes; (Step 5) run `go build ./... + go vet ./... + golangci-lint run` clean; (Step 6) append PROGRESS.md Task entry per D-P3; (Step 7) commit. Tasks that land bulk fixture material (Task 14 fixture-0026 directory + scripts + YAML configs) follow a relaxed test-with-implementation discipline (the differential fixture IS the integration test; the per-scenario `.lua` source files + the driver impl land together with the per-scenario probe assertions). The Skill's documentation classifies it as RIGID — adherence is mandatory, NOT advisory. *Anchored: `superpowers:test-driven-development` rigid discipline + phase-09..21 IMPL precedent.*

6. **D-P5 — `CompileCache` scope LOCKED at `compiledConfig`-instance (not cross-stream global; not cross-listener global).** Settle: the `*lua.CompileCache` is owned by the `*compiledConfig` (filter-config-instance scope; one cache per listener filter-chain mounting a lua filter); GC-driven eviction (the cache lifetime equals the `compiledConfig` lifetime; eviction happens when the listener drains + the `compiledConfig` is released). NO cross-listener / cross-process global cache. Rationale: (i) 22.1 has a single chunk per `compiledConfig` (the `Lua.DefaultSourceCode`); the cache's primary purpose is to forward-pin the API shape for 22.3 when `SourceCodes` map adds multiple chunks per listener; (ii) cross-listener sharing is unsafe (different listeners may have different sandbox configs in future phases — though at 22.1 all share the StrictUpstreamParity zero-value); (iii) `sync.RWMutex` discipline at the cache level scales to N concurrent compile-script calls per stream per phase 22.3 SourceCodes; (iv) GC-driven eviction matches the project's existing `sync.Pool`-free precedent (no manual evict; cache lifetime == compiledConfig lifetime — listener drain releases). *Anchored: 22.1 SPEC §3.1 + §3.4 + this PLAN-time emerge.*

7. **D-P6 — Boot-registration position LOCKED at alphabetical between `localratelimit.New` and `oauth2.New` per ADR-0100 §2.2.** Settle: `cmd/envoy-go/main.go` gains the `httpReg.Register(lua.TypeURL, lua.New)` call alphabetically between `localratelimit.New` and `oauth2.New` per ADR-0100 §2.2 (the Go-package identifier `lua` sorts after `localratelimit` and before `oauth2`). The 17th `httpReg.Register` call after phase 21's 16. Per ADR-0072 + ADR-0100 §2.2 — registration order does not affect runtime behavior; stylistic discipline only. The matching `import lua "github.com/esalaine/envoy-go/internal/filter/http/lua"` inserts alphabetically among the filter-package imports. **NO `RegisterPerRouteValidator` call delegation** at boot — instead, the per-route validator is registered via `reg.RegisterPerRouteValidator(filterName, validatePerRouteLua)` from inside `lua.New` itself per the parent §5.2 + ADR-0110 single-chokepoint discipline (matches phase-10 header_mutation + phase-20 oauth2 + phase-19.1 ext_proc precedent); the validator function is a one-liner returning the arm-18 PARSE-REJECT (`"lua: per-route configuration is not yet supported (lands in phase 22.3)"`). *Anchored: 22.1 SPEC §6 Task 10 + ADR-0100 §2.2 + ADR-0072 + ADR-0110 + this PLAN-time emerge.*

8. **D-P7 — Fuzzer corpus seed roster for `FuzzLuaConfigParse` LOCKED per 22.1 SPEC §6 Task 11 + parent §15 Layer C.** Settle: corpus seeds at `internal/filter/http/lua/testdata/fuzz/FuzzLuaConfigParse/` covering:
   - **Per PARSE-REJECT arm** (18 seeds; 1 seed per arm) — one fixture triggering each of the 18 PARSE-REJECT arms from parent §6.2;
   - **Valid-config seeds** (5 seeds; 1 per DataSource arm + 1 with explicit `stat_prefix`) — `DefaultSourceCode.Filename` valid path; `DefaultSourceCode.InlineBytes` valid Lua; `DefaultSourceCode.InlineString` valid Lua; `DefaultSourceCode.EnvironmentVariable` valid var name (assumes test env sets it); valid config with non-empty `stat_prefix`;
   - **Adversarial-Lua-source seeds** (7 seeds) — syntax errors triggering arm 16 (`function envoy_on_request` no-end; broken parens; unicode-bom-edge-cases); sandbox-breaking attempts that compile-clean but should error at runtime (NOT panic — `dofile("/etc/passwd")`; `io.popen("ls")`; `os.execute("rm /")`; `require("syscall")`; `debug.getupvalue(envoy_on_request, 1)`).

   Total corpus floor: ~30 seeds. Must-never-panic across `New()` per ADR-0018. Clean at 30s per seed. *Anchored: 22.1 SPEC §6 Task 11 + parent §15 Layer C + ADR-0018 + this PLAN-time emerge.*

9. **D-P8 — Task graph parallelization LOCKED per planner-time emerge.** Settle: after Pre-Task 0 (PROGRESS.md preamble + precondition check) lands, the 16-task graph allows parallelization at multiple points:

   - **After Task 1** (package skeletons): Tasks 2 (`compiled_config.go`) + 3 (`datasource.go`) + 4 (`compile.go`) + 11 (`fuzz_test.go`) can run in PARALLEL — each depends only on Task 1's skeleton files + the gopher-lua dep being available. NOTE: Task 11 ALSO depends on Task 2's `buildCompiledConfig` being non-skeleton; the fuzzer needs a fuzzable target. So Task 11 can start (write the fuzzer skeleton) but the actual `go test -fuzz=FuzzLuaConfigParse -fuzztime=30s` clean-run depends on Task 2 finishing.
   - **After Task 4** (`compile.go` IMPL): Task 5 (`vm.go` + `sandbox.go` IMPL) can run.
   - **After Task 5** (`vm.go` lifecycle + sandbox IMPL): Tasks 6 (headers + `__pairs`) + 7 (log) + 8 (streamInfo) can run in PARALLEL — each bridge method group is file-disjoint within `bridge.go` (separate function bodies + separate test bodies in `bridge_test.go`); the IMPL session can dispatch 3 concurrent subagents each appending their bridge surface to `bridge.go` + `bridge_test.go` then merging.
   - **After Tasks 6 + 7 + 8**: Task 9 (`respond` + `decode_headers.go` + `encode_headers.go`) + Task 10 (`stats.go` + boot-reg) + Task 13 (`BackendKind=HTTPLua` + `BootRejectFixture` harness) can run in PARALLEL — file-disjoint surfaces.
   - **After Tasks 9 + 10 + 13**: Task 12 (race + concurrency tests) + Task 14 (fixture-0026 directory + scripts) can run in PARALLEL.
   - **After Tasks 12 + 14**: Task 15 (envoy-go-side wording-pinning + fixture-0026 green-light) lands.
   - **Sequential tail**: Task 16 (atomic landing — BEHAVIOR_CONTRACT.md + ADR bodies + STATE.md + ROADMAP).

   **Parallel-dispatch opportunities**: 4-way at Tasks 2+3+4+11; 3-way at Tasks 6+7+8; 3-way at Tasks 9+10+13; 2-way at Tasks 12+14. **Sequential bottlenecks**: Task 1 → {2,3,4,11}; Task 4 → Task 5; Task 5 → {6,7,8}; {6,7,8} → {9,10,13}; {9,10,13} → {12,14}; {12,14} → Task 15; Task 15 → Task 16. The IMPL session per `superpowers:subagent-driven-development` per project memory `feedback_execution_style.md` exploits these parallel opportunities. *Anchored: 22.1 SPEC §6 5-tier breakdown + this PLAN-time emerge.*

10. **D-P9 — Cross-package regression-test command shape LOCKED.** Settle: after each task lands its production code, the implementer runs the package-local test command (`go test -count=1 -race ./internal/lua/... ./internal/filter/http/lua/...` for Tasks 1-12; `go test -count=1 ./test/differential -run TestDifferential/0026` for Tasks 13-15; full `go test -count=1 -race ./...` at Task 16 final gate). At Task 16 Gate D the full regression `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-6])'` runs all 28 fixture directories (the 27 pre-existing — 0000-0025 + 0007a/b sub-fixtures — plus the new 0026). Per 22.1 SPEC §15 expected outcome: zero regression. *Anchored: 22.1 SPEC §15 Layer E + phase-21 D4 precedent + this PLAN-time emerge.*

11. **D-P10 — `*LState`-pool benchmark sub-task LOCKED at Task 12 with explicit > 1ms threshold gating per parent §13-R6.** Settle: Task 12 (race + concurrency tests) ALSO includes a benchmark sub-task `BenchmarkPerStreamLState_Construction_Headers` at `internal/filter/http/lua/lua_test.go` measuring per-stream `*lua.LState` construction cost at the headers-only bridge surface (constructs N=10000 fresh VMs back-to-back; reports `ns/op` via `b.N` discipline). The threshold gate per parent §13-R6: if `ns/op > 1_000_000` (= 1ms), the ADR-0190 escape-valve FIRES at Task 16; ADR-0190 §Context + §Decision + §Consequences body all land at the same Task 16 commit per ADR-0044 anchoring a "per-script-source `*LState` pool with chunk-pre-loaded entries" decision. If `ns/op <= 1_000_000`, the WEAK-default per-stream construction STANDS; no ADR-0190 fires; next-free ADR-0190 stays UNCONSUMED carried forward to 22.2 BRAINSTORM. The benchmark result quoted verbatim in Task 12 PROGRESS.md entry. *Anchored: parent §13-R6 + this 22.1 SPEC §13 R6 STANDS + 22.1 SPEC §2.19 + this PLAN-time emerge.*

---

## ADRs introduced/landed by this plan

The 22.1 IMPL lands 2 ADR §Decision + §Consequences bodies at Task 16 atomic landing per ADR-0044 (the §Context drafts already anchored at parent SPEC commit `41ccee7` per parent §4.1 + §4.4); 1 CONDITIONAL ADR landing at Task 16 only if R6 escape-valve fires per D-P10. **NO new ADRs consumed at any task before Task 16.** The ADR-0125 §(xiv) AMENDMENT-anticipation paragraph anchored at parent SPEC commit STANDS UNCHANGED at this 22.1 PLAN commit; the AMENDMENT body lands at 22.3 IMPL final Task (NOT 22.1).

| ADR | Subject (22.1 portion) | Lands-in-Task |
|---|---|---|
| **ADR-0188** | NEW `internal/lua/` framework primitive — gopher-lua v1.1.2 VM lifecycle + per-stream `*LState` construction + per-script-source `*Chunk` compile cache + `SandboxConfig` per-stdlib ALLOW/DENY zero-value `StrictUpstreamParity` posture per AMEND-1 + bridge-registration `State()` escape-hatch + `WithPanicHandler` + `WithBasePrintSink` VMOptions + panic-wrapper + EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 per BRAINSTORM Q4 (the second `internal/lua/` consumer at one of cluster_specifier Lua / access_logger Lua / string_matcher Lua may require API revision after empirical validation at consumer #2). Refined production signatures per this 22.1 SPEC §3.1 vs parent §4.1 sketch (`VMOption` function-option pattern; `Run/HasGlobalFunc/CallGlobal` split; `State()` escape-hatch; `WithPanicHandler`/`WithBasePrintSink` naming + relocation). 3 production + 3 test files per §3.2. | Task 16 |
| **ADR-0189** | NEW `internal/filter/http/lua/` package shape — 8 production + 5 test files per parent §4.4 + this 22.1 SPEC §3.5; `compiledConfig` + 3-counter `filterStats` shape per §4; 18-arm PARSE-REJECT roster per parent §6.2; 4-arm `DataSource` resolution per parent §5.3; pragmatic-middle bridge surface 21 entries per BRAINSTORM Q6; full byte-pin `:respond()` per parent §11.6.7 + AMEND-7; AMEND-8 encode-side runtime-reject; 3-counter HCM-rooted stat surface per AMEND-2 + AMEND-3; `__pairs` alphabetical-snapshot per §11 D7; fixture-0026 disposition per §8 + this 22.1 SPEC §9 + scenario (g) substring-match per AMEND-10 + the D3 PLAN-session closure at option (a) stat-counter delta per this PLAN's D3; NEW `BootRejectFixture` driver interface per §13-R1; envoy-go-side `"script load error: "` wording-pinning per §13-W; per-route 5th-canonical PARSE-REJECT at any tier per arm 18 + ADR-0110 single-chokepoint; Task 2 D1 closure evidence (anticipated PARSE-REJECT both arms 5 + 17; subject to upstream-scrape REFUTE). | Task 16 |

### CONDITIONAL ADR landing (only if R6 escape-valve fires per D-P10)

| ADR | AMENDMENT scope | Lands-in-Task |
|---|---|---|
| **ADR-0190** (CONDITIONAL) | Per-script-source `*LState` pool with chunk-pre-loaded entries — anchors only if Task 12 `BenchmarkPerStreamLState_Construction_Headers` reports `ns/op > 1_000_000` (= 1ms threshold per parent §13-R6 + this 22.1 SPEC §2.19 + this PLAN's D-P10). §Context + §Decision + §Consequences body all land at the same Task 16 commit per ADR-0044. If unconsumed: next-free ADR-0190 carries forward to 22.2 BRAINSTORM as the 22.2 IMPL escape-valve slot. | Task 16 (CONDITIONAL) |

The implementer at Task 16 AUTHORS the 2 ADR §Decision + §Consequences bodies in DECISIONS.md (the §Context drafts are already at the parent SPEC commit per ADR-0044), includes the ADRs in the Task 16 commit message, and verifies via `grep -nE '^## ADR-0188' docs/envoy-go/DECISIONS.md` returning the expected single match (similarly for ADR-0189). If R6 escape-valve fires per D-P10, ADR-0190 §Context body also lands at the same commit.

**NO in-place ADR-0125 amendment at this PLAN commit + at Task 16** — the AMENDMENT-anticipation paragraph anchored at parent SPEC commit STANDS UNCHANGED; the AMENDMENT body lands at 22.3 IMPL final Task (per the 22.3 sub-phase PLAN).

**ADR-0044 escape-valve held in reserve per D-P10** — `ADR-0190` is the conditional escape-valve slot; the 22.1 SPEC's WEAK HOLD per §1.2 + §13-R6 STANDS UNCHANGED at this PLAN commit. If at IMPL time a surface DOES warrant a new ADR beyond ADR-0190 (highly unlikely per the SPEC-time scrape closure of D5 + D7), it is ADR-0191 + the SPEC-anchored hypothesis is recorded as falsified in PROGRESS.md.

---

## Task graph (sequential vs parallelizable per D-P8)

The IMPL session subagent-dispatches per `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). Per-task dependency graph:

- **Pre-Task 0** (PROGRESS.md preamble + 15-precondition verification) — sequential prerequisite for everything; sets up the append-only log.
- **Task 1** (package skeletons + go.mod update) — sequential prerequisite for Tasks 2-16; lands the package directories + gopher-lua dep.
- **Tasks 2, 3, 4, 11** — **PARALLELIZABLE** (4-way) after Task 1; each depends only on Task 1's skeleton:
  - **Task 2** — `compiled_config.go` + 18-arm PARSE-REJECT roster + D1 closure first-action.
  - **Task 3** — `datasource.go` 4-arm resolution + 10-leaf PARSE-REJECTs.
  - **Task 4** — `internal/lua/compile.go` full IMPL — `Chunk` + `CompileCache` + `CompileScript`.
  - **Task 11** — `fuzz_test.go` 28th fuzzer skeleton (corpus seeds land alongside; full clean-run depends on Task 2's `buildCompiledConfig` being non-skeleton).
- **Task 5** (`internal/lua/vm.go` + `sandbox.go` full IMPL — VM lifecycle + sandbox) — depends on Task 4.
- **Tasks 6, 7, 8** — **PARALLELIZABLE** (3-way) after Task 5; each is a file-disjoint bridge-method group within `bridge.go` + `bridge_test.go`:
  - **Task 6** — headers + `__pairs` alphabetical-snapshot per §11 D7.
  - **Task 7** — log methods (6 `:logXxx`).
  - **Task 8** — streamInfo subset (4 methods).
- **Tasks 9, 10, 13** — **PARALLELIZABLE** (3-way) after Tasks 6+7+8; file-disjoint surfaces:
  - **Task 9** — `respond` + `decode_headers.go` + `encode_headers.go`.
  - **Task 10** — `stats.go` + boot-registration at `cmd/envoy-go/main.go`.
  - **Task 13** — `BackendKind=HTTPLua` + `BootRejectFixture` harness infrastructure.
- **Tasks 12, 14** — **PARALLELIZABLE** (2-way) after Tasks 9+10+13:
  - **Task 12** — race + concurrency tests at `internal/lua/` + `internal/filter/http/lua/` + the `BenchmarkPerStreamLState_Construction_Headers` benchmark sub-task per D-P10 (gates ADR-0190 escape-valve at Task 16).
  - **Task 14** — fixture-0026 directory + scripts + driver impl.
- **Task 15** (envoy-go-side `"script load error: "` wording-pinning + fixture-0026 green-light) — depends on Tasks 12 + 14.
- **Task 16** (atomic landing — BEHAVIOR_CONTRACT.md 7-edit bundle + ADR-0188 + ADR-0189 + STATE.md + ROADMAP + CONDITIONAL ADR-0190 if R6 fires + REVIEW.md authoring per `superpowers:requesting-code-review`) — depends on everything.

**Parallel-dispatch opportunities**: 4-way at Tasks 2+3+4+11; 3-way at Tasks 6+7+8; 3-way at Tasks 9+10+13; 2-way at Tasks 12+14. **Sequential bottlenecks**: Pre-Task 0 → Task 1 → {2,3,4,11}; Task 4 → Task 5; Task 5 → {6,7,8}; {6,7,8} → {9,10,13}; {9,10,13} → {12,14}; {12,14} → Task 15 → Task 16.

---

## Execution preconditions

Before Pre-Task 0 the implementer cold-starts and verifies. **Worktree spawn discipline:** the IMPL session runs on a fresh worktree branched off the PLAN tip per ADR-0003 + the per-phase-worktree convention (project memory `feedback_git_worktrees.md`). The expected sequence (executed by the orchestrating session before invoking the IMPL session, OR by the IMPL session at cold-start if standalone):

```bash
# From the master worktree (or any non-conflicting worktree):
git worktree add /home/esa/git/envoy-go/.worktrees/phase-22.1-http-filter-lua-vm-and-headers-bridge-impl \
                 -b phase-22.1-http-filter-lua-vm-and-headers-bridge-impl <PLAN-tip-SHA>
cd /home/esa/git/envoy-go/.worktrees/phase-22.1-http-filter-lua-vm-and-headers-bridge-impl
```

where `<PLAN-tip-SHA>` is the master tip after the PLAN.md squash-merge commit + its SHA-fill follow-up.

The 15 preconditions verified at Pre-Task 0 cold-start:

1. **Worktree branch.** `git rev-parse --abbrev-ref HEAD` returns `phase-22.1-http-filter-lua-vm-and-headers-bridge-impl`. If only a SPEC-stage or PLAN-stage worktree is present, branch a fresh impl worktree from master HEAD per ADR-0003.
2. **Master tail.** `git log --oneline master | head -8` shows the phase-22.1-PLAN.md squash commit + its SHA-fill follow-up at the head, with the phase-22.1-SPEC.md squash commit `a7021aa` + its SHA-fill follow-up `b16abc5` immediately before. If not, resync via `git fetch origin master && git pull --ff-only`.
3. **Toolchain.** `go version` reports `go1.26.2` or newer; `golangci-lint version` reports `1.64.8` (ADR-0009 pin); `docker version` reports both client + server.
4. **DECISIONS.md tail.** `grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1` returns `189` (ADR-0189 — the highest ADR anchored as of master tip per the phase-22.1 SPEC commit). Higher → another phase landed concurrently; re-verify next-free numbers.
5. **ADR §Context drafts present.** `grep -cE '^## ADR-0188' docs/envoy-go/DECISIONS.md` returns `1` (ADR-0188 §Context already at parent SPEC commit `41ccee7` per ADR-0044). Same for ADR-0189. `grep -nE '^## ADR-0190' docs/envoy-go/DECISIONS.md` returns 0 (ADR-0190 stays unconsumed UNLESS D-P10 R6 escape-valve fires at Task 12).
6. **ADR-0125 §(xiv) AMENDMENT-anticipation paragraph present.** `grep -nE 'Amendment \(per phase 22\.3\)' docs/envoy-go/DECISIONS.md` returns ≥1 match in the ADR-0125 body block — confirms the parent-SPEC-time AMENDMENT-anticipation paragraph anchored. The AMENDMENT body lands at 22.3 IMPL final Task (NOT 22.1).
7. **NO 22.3-bound code at this 22.1 worktree.** Per BOOTSTRAP §4.1 invariant 2 — phase-22.3 surfaces (`Lua.SourceCodes` map activation; `LuaPerRoute` 3-arm oneof IMPL; NEW 9th canonical per-route shape ADR) MUST NOT land at 22.1 IMPL. If a 22.3-surface partial implementation has been started, halt + escalate to user.
8. **Parent SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/22-http-filter-lua/SPEC.md` returns `41ccee7` (or descendant). If different, re-read parent SPEC.
9. **22.1 SPEC SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/SPEC.md` returns `a7021aa` (or descendant). If different, re-read 22.1 SPEC.
10. **22.1 PLAN SHA.** `git log -1 --format=%H -- docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PLAN.md` returns the PLAN commit's SHA. If earlier than the SPEC, PLAN has been amended — re-read PLAN.
11. **Pristine tree.** `git status --porcelain` returns empty.
12. **Pre-existing suite green at `-short` budget.** `go test -count=1 -short ./...` returns clean.
13. **Pre-existing differential suite green.** `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-5])'` returns every fixture 0000-0025 (27 directories counting `0007a + 0007b` separately) PASS — the regression baseline. Phase 22.1 adds the 22nd `BackendKind` enum value + the 28th-by-directory fixture (`0026-http-lua-headers-bridge` per Task 14).
14. **Pre-existing fuzzers run clean at 30s.** The 27 fuzzers from phases 02-21 run clean. Phase 22.1 adds the 28th (`FuzzLuaConfigParse` per Task 11). Quick smoke: `find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l` returns `27`.
15. **Pre-existing `internal/lua/` + `internal/filter/http/lua/` directories + `test/fixtures/0026-http-lua-headers-bridge/` directory + `BackendKind=HTTPLua` enum value do NOT exist.** `test ! -d internal/lua && test ! -d internal/filter/http/lua && test ! -d test/fixtures/0026-http-lua-headers-bridge && ! grep -q 'HTTPLua' test/differential/fixture/fixture.go && echo "ok: phase-22.1-new-surfaces absent"` returns success.

If all 15 preconditions pass, proceed to Pre-Task 0 (PROGRESS.md preamble) + Task 1.

---

## Pre-Task 0: PROGRESS.md preamble + 15-precondition verification

**Files:**
- Create: `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md`

This pre-task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, ADR-0188 + ADR-0189 §Context drafts are at the parent SPEC commit `41ccee7`; ADR-0190 is CONDITIONAL (PLAN hypothesis per D-P10: it does NOT fire unless Task 12 benchmark surfaces > 1ms threshold). The PROGRESS preamble ANTICIPATES the 2 NEW ADR landings at Task 16 + records the 10 PLAN-time decisions D3 + D-P1..D-P10.

Pre-Task 0 is NOT a SPEC §6 numbered task — the SPEC §6 16-task breakdown begins at Task 1 (package skeletons). Per D-P1, the SPEC §6 numbering is inherited verbatim; PROGRESS.md preamble + precondition verification is the ritual prefix.

**Precondition:** worktree exists at `phase-22.1-http-filter-lua-vm-and-headers-bridge-impl`; branch base is master tip after the 22.1 PLAN.md SHA-fill follow-up; all 15 preconditions report green.
**Artifact:** `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md` (new file).
**Acceptance:** all 15 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md` returns the Pre-Task 0 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md` with: (a) Preamble summarizing the 15-precondition verification (verbatim command outputs captured); (b) the 2-NEW-ADR table + CONDITIONAL ADR-0190 row from `## ADRs introduced/landed by this plan` reproduced verbatim; (c) the 10 PLAN-time decisions D3 + D-P1..D-P10 reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) a Pre-Task 0 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Pre-Task 0: PROGRESS.md preamble + 15-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
# expect: a 40-char SHA (Pre-Task 0 commit)
```

---

## Tier A — scaffold (Tasks 1-5)

## Task 1: Package skeletons (NEW `internal/lua/` + NEW `internal/filter/http/lua/`) + `go.mod` gopher-lua dep

**Files:**
- Create: `internal/lua/doc.go` (~30-50 LoC)
- Create: `internal/lua/vm.go` (skeleton ~80 LoC; type + stubs returning zero values; full IMPL at Task 5)
- Create: `internal/lua/compile.go` (skeleton ~30 LoC; types + stubs; full IMPL at Task 4)
- Create: `internal/lua/sandbox.go` (skeleton ~40 LoC; `SandboxConfig` type + per-stdlib defaults; full IMPL at Task 5)
- Create: `internal/lua/vm_test.go` + `compile_test.go` + `sandbox_test.go` (skeleton tests; type assertions + minimal smoke tests)
- Create: `internal/filter/http/lua/doc.go` (~50-80 LoC)
- Create: `internal/filter/http/lua/lua.go` (skeleton ~80 LoC; filter struct + factory + filterStats stub + TypeURL + filterName + per-route validator registration; full IMPL at Task 10)
- Create: `internal/filter/http/lua/lua_test.go` (skeleton tests)
- Modify: `go.mod` + `go.sum` — add `github.com/yuin/gopher-lua v1.1.2` direct dep
- Append: `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md` (Task 1 entry per D-P3)

This task lands the package directory skeletons + the gopher-lua dep. Per 22.1 SPEC §6 Task 1: `go build ./internal/lua/... ./internal/filter/http/lua/...` clean; package files compile; skeleton tests pass; `go mod tidy` clean. **Sequential prerequisite for Tasks 2-16.**

**Precondition:** Pre-Task 0 complete; all 15 preconditions green; gopher-lua v1.1.2 is the pinned version per 22.1 SPEC §6 Task 1.
**Artifact:** Both package directories with skeleton files; gopher-lua dep added; `go build` + skeleton tests pass.
**Acceptance:** `go build ./internal/lua/... ./internal/filter/http/lua/...` clean; `go vet ./...` clean; `golangci-lint run ./internal/lua/... ./internal/filter/http/lua/...` clean; `go test -count=1 ./internal/lua/... ./internal/filter/http/lua/...` skeleton tests pass; `go mod tidy` clean (no orphaned modules); `go.sum` includes gopher-lua entries.

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author the Task 1 skeleton files at the 8 listed paths per the 22.1 PLAN Task 1 + 22.1 SPEC §3.1 + §3.2 + §3.5 production signatures. The package directories are NEW (both `internal/lua/` and `internal/filter/http/lua/`). Add `github.com/yuin/gopher-lua v1.1.2` to go.mod as a direct dep; run `go mod tidy`. Each skeleton file should compile cleanly + the skeleton tests should pass with type-assertion-only verification. Cross-reference the 22.1 SPEC §3.1 production signatures verbatim in `internal/lua/vm.go` (types defined with stub method bodies returning zero values); cross-reference 22.1 SPEC §3.5 file split in `internal/filter/http/lua/doc.go`. Per-route validator registration in `lua.go` is a one-liner (`reg.RegisterPerRouteValidator(filterName, validatePerRouteLua)` where `validatePerRouteLua` is a one-liner returning `errors.New("lua: per-route configuration is not yet supported (lands in phase 22.3)")`). Commit per the Step 8 commit message template. Quote `go build`, `go vet`, `golangci-lint`, `go test`, `go mod tidy` outputs verbatim in PROGRESS.md Task 1 entry per D-P3.

- [ ] **Step 1: Add gopher-lua dep to go.mod**

```bash
go get github.com/yuin/gopher-lua@v1.1.2
go mod tidy
```

- [ ] **Step 2: Author `internal/lua/doc.go`** per 22.1 SPEC §3.2 — package overview + AMEND-1 sandbox-strict rationale + AMEND-9 gopher-lua-vs-LuaJIT divergence cross-refs + AMEND-A4 coroutine cross-reference + API surface summary + ADR-0188 cross-reference.

- [ ] **Step 3: Author `internal/lua/vm.go` SKELETON** per 22.1 SPEC §3.1 production signatures — `VM` struct + `VMOption` type + `NewVM(opts ...VMOption) *VM` returning zero `&VM{}` + `WithSandboxConfig` + `WithPanicHandler` + `WithBasePrintSink` returning trivial closures + `State() *lua.LState` stub returning `nil` + `RegisterGlobalFunc` stub no-op + `Run` + `HasGlobalFunc` + `CallGlobal` stubs returning nil + `Close` stub no-op + `PanicHandlerFn` type. ALL public surface declared with stub bodies; full IMPL at Task 5.

- [ ] **Step 4: Author `internal/lua/compile.go` SKELETON** per 22.1 SPEC §3.1 — `Chunk` struct (unexported fields) + `CompileCache` struct (unexported fields) + `NewCompileCache` returning `&CompileCache{store: map[[32]byte]*Chunk{}}` + `CompileScript(src []byte, cache *CompileCache) (*Chunk, error)` stub returning `&Chunk{}, nil`. Full IMPL at Task 4.

- [ ] **Step 5: Author `internal/lua/sandbox.go` SKELETON** per 22.1 SPEC §3.1 — `SandboxConfig` struct with 8 `Allow*` fields + per-stdlib defaults helper functions (`applyZeroValueDefaults(sb SandboxConfig) SandboxConfig` setting `AllowCoroutine` + `AllowOSTimeHelpers` to true zero-valued case). Full IMPL at Task 5.

- [ ] **Step 6: Author skeleton tests** — `internal/lua/vm_test.go` (type-assertion `var _ = NewVM(); var _ = (*VM).Close; var _ = (*VM).State`); `compile_test.go` (`TestNewCompileCache_ReturnsNonNil`; `TestCompileScript_NilCache_DoesNotPanic`); `sandbox_test.go` (`TestSandboxConfig_ZeroValue_AllowsCoroutineDefault` placeholder).

- [ ] **Step 7: Author `internal/filter/http/lua/doc.go`** per 22.1 SPEC §3.5 — package overview + BRAINSTORM Q1-Q12 summary + AMEND-1..AMEND-12 cross-refs + D1 + D5 + D7 cross-refs + API surface summary + ADR-0189 cross-reference.

- [ ] **Step 8: Author `internal/filter/http/lua/lua.go` SKELETON** — `TypeURL = "type.googleapis.com/envoy.extensions.filters.http.lua.v3.Lua"`; `filterName = "envoy.filters.http.lua"`; `filterStats` struct (3 `*stats.Counter` fields stub); `New(ctx http.FilterFactoryContext) (http.FilterInstanceFactory, error)` stub returning `nil, errors.New("not yet implemented")` (full IMPL at Task 10); `validatePerRouteLua(msg proto.Message) error` one-liner returning the arm-18 PARSE-REJECT; filter struct + compile-time interface assertions `var _ http.StreamDecoderFilter = (*filter)(nil)` + `var _ http.StreamEncoderFilter = (*filter)(nil)` (the filter struct method bodies are stubs at Task 1; full IMPL at Tasks 9 + 10).

- [ ] **Step 9: Author `internal/filter/http/lua/lua_test.go`** — `TestTypeURL_Matches` (asserts the constant); `TestNew_NotYetImplemented` (asserts the Task-1-stub error).

- [ ] **Step 10: Verify `go build ./internal/lua/... ./internal/filter/http/lua/...` + `go vet ./...` + `golangci-lint run ./internal/lua/... ./internal/filter/http/lua/...` clean.**

- [ ] **Step 11: Verify skeleton tests pass**

```bash
go test -count=1 ./internal/lua/... ./internal/filter/http/lua/...
# Expect: PASS — skeleton tests
go mod tidy
# Expect: clean output (no orphaned modules)
```

- [ ] **Step 12: Append PROGRESS.md Task 1 entry** per D-P3.

- [ ] **Step 13: Commit**

```bash
git add internal/lua/ internal/filter/http/lua/ go.mod go.sum \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 1: package skeletons + gopher-lua v1.1.2 dep

Creates NEW internal/lua/ framework primitive skeleton (3 production +
3 test files; doc.go + vm.go + compile.go + sandbox.go + tests; full IMPL
at Tasks 4 + 5) and NEW internal/filter/http/lua/ package skeleton
(doc.go + lua.go + lua_test.go; full IMPL across Tasks 2-15) per 22.1
SPEC §3.1 + §3.2 + §3.5 production signatures.

NEW direct dep github.com/yuin/gopher-lua v1.1.2 (pure-Go Lua 5.1; MIT;
matches upstream LuaJIT 5.1 dialect; no CGO). go.mod + go.sum updated;
go mod tidy clean.

Skeleton tests pass; go build + go vet + golangci-lint all clean.
Sequential prerequisite for Tasks 2-16 satisfied."
```

---

## Task 2: `compiled_config.go` + 18-arm PARSE-REJECT roster + D1 closure first-action

**Files:**
- Create: `internal/filter/http/lua/compiled_config.go` (~250-380 LoC)
- Create: `internal/filter/http/lua/compiled_config_test.go` (~400-600 LoC; 18-arm table-driven + valid-config rows)
- Append: `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md` (Task 2 entry per D-P3, including D1 closure evidence)

This task lands the `compiledConfig` struct + the `buildCompiledConfig` parser + the full 18-arm PARSE-REJECT roster per parent §6.2 with byte-stable error wording per parent §6.1. **PARALLELIZABLE with Tasks 3 + 4 + 11** per D-P8 (file-disjoint surfaces; depend only on Task 1).

**D1 closure first action**: scrape upstream Envoy v1.37.2 `source/extensions/filters/http/lua/config.cc::createFilterFactoryFromProtoTyped` + `lua_filter.cc::Filter` constructor against the anticipated PARSE-REJECT-both disposition for arms 5 (`default-source-code-required`) + 17 (`script-missing-required-hooks`). If anticipated holds: arms 5 + 17 STAND; close D1 at this Task's PROGRESS.md entry with empirical evidence quoted + cite. If REFUTED (upstream allows no-op): arms 5 + 17 flip to silent no-op (degraded pass-through); update parent §6.2 + ADR-0189 §Decision body with corrected disposition.

**Precondition:** Task 1 complete.
**Artifact:** `compiled_config.go` with full PARSE-REJECT roster; D1 closure recorded in PROGRESS.md.
**Acceptance:** `go build ./internal/filter/http/lua/...` clean; `go vet ./...` clean; `golangci-lint run` clean; `go test -count=1 ./internal/filter/http/lua/... -run 'TestBuildCompiledConfig'` clean (~25-30 PARSE-REJECT rows + valid-config rows all pass); D1 closure evidence quoted in PROGRESS.md Task 2 entry with upstream `config.cc` + `lua_filter.cc` line citations.

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 2 per the 22.1 PLAN Task 2 + 22.1 SPEC §6 Task 2 + parent §6.2 18-arm PARSE-REJECT roster verbatim. First action: scrape upstream Envoy v1.37.2 at `source/extensions/filters/http/lua/config.cc::createFilterFactoryFromProtoTyped` + `source/extensions/filters/http/lua/lua_filter.cc::Filter` constructor to empirically resolve D1 (the `default_source_code` absent + `script-missing-required-hooks` disposition; anticipated PARSE-REJECT both arms 5 + 17). Quote the upstream evidence verbatim in PROGRESS.md Task 2 entry with line citations; cross-reference the resolved disposition in `compiled_config.go` doc-comments. Then author `buildCompiledConfig` covering all 18 arms with byte-stable error wording per parent §6.1. Test file is table-driven with one row per arm + 5-10 valid-config rows (each DataSource arm with valid contents). Cross-reference parent §6.2 in both production and test code doc-comments.

- [ ] **Step 1: Scrape upstream Envoy v1.37.2 for D1 closure** — fetch `config.cc::createFilterFactoryFromProtoTyped` + `lua_filter.cc::Filter` constructor source; identify the upstream disposition for (a) `default_source_code` absent path (PARSE-REJECT vs silent no-op) + (b) script-missing-required-hooks path (PARSE-REJECT vs silent no-op). Capture evidence verbatim for PROGRESS.md Task 2 entry.

- [ ] **Step 2: Write failing tests** in `internal/filter/http/lua/compiled_config_test.go`. Table-driven format: each row is `{name string, configMutator func(*luav3.Lua), wantErrSubstring string}`. ~20-25 PARSE-REJECT rows + 5-10 valid-config rows. Cover arms 1-18 from parent §6.2 with byte-exact error wording.

- [ ] **Step 3: Run tests to verify they fail**

```bash
go test ./internal/filter/http/lua/... -run TestBuildCompiledConfig 2>&1 | head -20
# Expect: FAIL with "buildCompiledConfig not implemented" or similar
```

- [ ] **Step 4: Author `internal/filter/http/lua/compiled_config.go`** per the File-structure table row above + 22.1 SPEC §4.2 + parent §6.2 + parent §6.1. The `compiledConfig` struct shape per 22.1 SPEC §4.2 verbatim; `buildCompiledConfig` body covers each of the 18 PARSE-REJECT arms; defaults applied per parent §6.1 (`stat_prefix` empty → consecutive-dot template; `sandbox` zero-value = StrictUpstreamParity).

- [ ] **Step 5: Run tests to verify they pass**

```bash
go test -count=1 ./internal/filter/http/lua/... -run TestBuildCompiledConfig
# Expect: PASS — 25-30 PARSE-REJECT rows + 5-10 valid-config rows
```

- [ ] **Step 6: Verify `go build ./...` + `go vet ./...` + `golangci-lint run` clean.**

- [ ] **Step 7: Append PROGRESS.md Task 2 entry** per D-P3 + D1 closure evidence (the upstream scrape result with `config.cc` + `lua_filter.cc` line citations + the resolved disposition for arms 5 + 17 + the cross-reference to ADR-0189 §Decision body anchor at Task 16).

- [ ] **Step 8: Commit**

```bash
git add internal/filter/http/lua/compiled_config.go \
        internal/filter/http/lua/compiled_config_test.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 2: compiled_config.go + 18-arm PARSE-REJECT roster + D1 closure

Lands compiledConfig struct + buildCompiledConfig with the full 18-arm
PARSE-REJECT roster per parent §6.2 with byte-stable error wording per
parent §6.1. D1 closed empirically against upstream Envoy v1.37.2 (per
22.1 SPEC §12-D1 anchored resolution point): anticipated PARSE-REJECT
both arms 5 (default-source-code-required) + 17 (script-missing-required-
hooks) HELD/REFUTED [pick one based on scrape result]. ADR-0189 §Decision
body sketch updated.

20-25 PARSE-REJECT table-driven tests + 5-10 valid-config tests pass."
```

---

## Task 3: `datasource.go` — 4-arm DataSource resolution

**Files:**
- Create: `internal/filter/http/lua/datasource.go` (~150-220 LoC)
- Create: `internal/filter/http/lua/datasource_test.go` (~300-450 LoC; 4-arm + 10-rejection-leaf table-driven)
- Append: PROGRESS.md (Task 3 entry per D-P3)

This task lands the 4-arm `DataSource` arm dispatch (`Filename` / `InlineBytes` / `InlineString` / `EnvironmentVariable`) per parent §5.3 + AMEND-5 10-arm refinement; `WatchedDirectory` PARSE-REJECT; empty-oneof PARSE-REJECT; per-arm empty-content PARSE-REJECTs. **PARALLELIZABLE with Tasks 2 + 4 + 11** per D-P8 (file-disjoint).

**Precondition:** Task 1 complete.
**Artifact:** `datasource.go` + 10-leaf table-driven tests.
**Acceptance:** `go build` + `go vet` + `golangci-lint` clean; `go test -count=1 ./internal/filter/http/lua/... -run 'TestResolveDataSource'` clean (4 valid + 10 rejection-leaf rows all pass with byte-exact wording per parent §6.2 arms 6-15).

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 3 per the 22.1 PLAN Task 3 + 22.1 SPEC §6 Task 3 + parent §5.3 + parent §6.2 arms 6-15 + AMEND-5 10-arm refinement. The 4-arm DataSource dispatch: `Filename` → `os.ReadFile`; `InlineBytes` → verbatim; `InlineString` → byte-cast; `EnvironmentVariable` → `os.LookupEnv`; `WatchedDirectory` → PARSE-REJECT (deferred to future Runtime/RTDS family phase); empty-oneof → PARSE-REJECT; per-arm empty-content PARSE-REJECTs (filename name-empty / ENOENT / zero-byte; env-var name-empty / unset / empty-value). File-read failures (ENOENT / EACCES / EISDIR) exercised via `t.TempDir()` synthetic files. Byte-exact error wording per parent §6.2 arms 6-15.

- [ ] **Step 1: Write failing tests** in `internal/filter/http/lua/datasource_test.go` covering all 4 arms + 10 rejection leaves per parent §6.2 + AMEND-5.

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `internal/filter/http/lua/datasource.go`** per the File-structure table row above + 22.1 SPEC §6 Task 3 + parent §5.3.

- [ ] **Step 4: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/filter/http/lua/... -run TestResolveDataSource
# Expect: PASS — 4 valid arms + 10 rejection leaves
```

- [ ] **Step 5: Verify `go build` + `go vet` + `golangci-lint` clean.**

- [ ] **Step 6: Append PROGRESS.md Task 3 entry** per D-P3.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/lua/datasource.go \
        internal/filter/http/lua/datasource_test.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 3: datasource.go + 4-arm DataSource resolution

Lands 4-arm DataSource dispatch (Filename/InlineBytes/InlineString/
EnvironmentVariable) per parent §5.3 + AMEND-5 10-arm refinement.
WatchedDirectory PARSE-REJECTed (deferred to future Runtime/RTDS phase).
Per-arm empty-content PARSE-REJECTs per parent §6.2 arms 6-15 byte-exact
wording. 10-leaf table-driven tests + 4 valid-arm tests pass."
```

---

## Task 4: `internal/lua/compile.go` IMPL — Chunk + CompileCache + CompileScript

**Files:**
- Modify: `internal/lua/compile.go` (full IMPL ~80-130 LoC total; replaces Task 1 skeleton)
- Modify: `internal/lua/compile_test.go` (~150-220 LoC cumulative; Task 4 adds bulk)
- Append: PROGRESS.md (Task 4 entry per D-P3)

This task lands the full `Chunk` + `CompileCache` + `CompileScript` IMPL per 22.1 SPEC §3.1 + the cache nil-tolerance per ADR-0085. **PARALLELIZABLE with Tasks 2 + 3 + 11** per D-P8 (file-disjoint).

**Precondition:** Task 1 complete.
**Artifact:** `compile.go` full IMPL + cache hit/miss + nil-tolerance tests.
**Acceptance:** `go test -count=1 ./internal/lua/... -run 'TestCompile|TestNewCompileCache'` clean; cache hit-on-same-content-hash + cache-miss-on-different-source + nil-cache verified; concurrent-read/add tests deferred to Task 12.

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 4 full IMPL per 22.1 SPEC §3.1 production signatures verbatim. `Chunk` wraps `*lua.FunctionProto` + `[32]byte` sha256 hash. `CompileCache` uses `sync.RWMutex` + `map[[32]byte]*Chunk`. `CompileScript(src, cache)` computes sha256(src); cache hit returns existing `*Chunk`; cache miss compiles via gopher-lua parser (`lua.NewState` + `LoadString` + extract `*FunctionProto` via `FunctionProto` field on the loaded LFunction); if cache non-nil, stores under hash. Cache nil-tolerance per ADR-0085: `CompileScript(src, nil)` compiles uncached + returns `*Chunk` without caching. Concurrent tests deferred to Task 12.

- [ ] **Step 1: Write failing tests** for cache hit/miss + nil-cache nil-tolerance + content-hash identity in `internal/lua/compile_test.go`.

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Replace `internal/lua/compile.go` skeleton with full IMPL** per 22.1 SPEC §3.1.

- [ ] **Step 4: Run tests to verify they pass.**

- [ ] **Step 5: Verify `go build` + `go vet` + `golangci-lint` clean.**

- [ ] **Step 6: Append PROGRESS.md Task 4 entry** per D-P3.

- [ ] **Step 7: Commit**

```bash
git add internal/lua/compile.go internal/lua/compile_test.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 4: internal/lua/compile.go full IMPL + CompileCache

Lands Chunk wrapper + CompileCache (sync.RWMutex-guarded sha256-keyed
content-addressed cache) + CompileScript per 22.1 SPEC §3.1. Cache nil-
tolerance per ADR-0085. Cache hit/miss + content-hash identity + nil-
cache tests pass. Concurrent-read/add tests at Task 12."
```

---

## Task 5: `internal/lua/vm.go` + `internal/lua/sandbox.go` IMPL — VM lifecycle + sandbox

**Files:**
- Modify: `internal/lua/vm.go` (full IMPL ~250-350 LoC; replaces Task 1 skeleton)
- Modify: `internal/lua/sandbox.go` (full IMPL ~120-180 LoC; replaces Task 1 skeleton)
- Modify: `internal/lua/vm_test.go` (~250-400 LoC cumulative; Task 5 adds bulk)
- Modify: `internal/lua/sandbox_test.go` (~250-400 LoC; full bulk at Task 5)
- Append: PROGRESS.md (Task 5 entry per D-P3)

This task lands the full VM lifecycle + sandbox roster IMPL per 22.1 SPEC §3.1 + §3.3. **Depends on Task 4** (the `*Chunk` type from compile.go is consumed by `Run(chunk *Chunk)`).

**Precondition:** Tasks 1 + 4 complete.
**Artifact:** `vm.go` + `sandbox.go` full IMPL + exhaustive per-stdlib ALLOW/DENY tests.
**Acceptance:** `go test -count=1 ./internal/lua/...` clean; sandbox roster fully enforced per §3.3 (sandbox_test.go verifies each denied function is nil-or-runtime-error post-sandbox-strict construction); panic-wrapper correctly converts Go panics to error returns; race tests deferred to Task 12.

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 5 full IMPL per 22.1 SPEC §3.1 production signatures + §3.3 sandbox roster + AMEND-1 default-deny posture. `NewVM(opts ...VMOption) *VM` constructs `*lua.LState` via `lua.NewState(lua.Options{...})`; applies `VMOption`s; resolves the `SandboxConfig` (zero-value-defaulted via `applyZeroValueDefaults` from sandbox.go — `AllowCoroutine` + `AllowOSTimeHelpers` true zero-valued); calls per-stdlib `OpenXxx` selectively (NOT `OpenLibs()`) per §3.3 + sandbox.go; for `AllowBaseFull == false` walks base globals and `LNil`s out the 9 denied functions; for `AllowOSTimeHelpers && !AllowOS` calls `OpenOs` then nils out 7 denied entries; for `AllowCoroutine == true` calls `OpenCoroutine`; for `print` rebinds to a Go function writing to `BasePrintSink` if non-nil otherwise drops; exposes INTERNAL `__envoy_traceback` global for panic-wrapper. `Run(chunk)` calls `vm.state.Push(lua.LFunction{Proto: chunk.proto})` + `PCall(0, lua.MultRet, nil)`; returns `*lua.ApiError` wrapped. `HasGlobalFunc(name)` does `GetGlobal(name)` + checks `LFunction` type. `CallGlobal(name, args...)` does `GetGlobal(name)` + `PCall(len(args), lua.MultRet, nil)` with args pushed. `Close()` calls `vm.state.Close()` idempotently. Panic-wrapper: a deferred recover() in `Run`/`CallGlobal` paths invokes `PanicHandlerFn` if set + converts the recovered value to an error return.

- [ ] **Step 1: Write failing tests** in `internal/lua/vm_test.go` + `sandbox_test.go` per the File-structure table rows above.

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Replace `internal/lua/vm.go` + `internal/lua/sandbox.go` skeletons with full IMPL** per 22.1 SPEC §3.1 + §3.3.

- [ ] **Step 4: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/lua/...
# Expect: PASS — VM lifecycle + sandbox roster + panic-wrapper
```

- [ ] **Step 5: Verify `go build` + `go vet` + `golangci-lint` clean.**

- [ ] **Step 6: Append PROGRESS.md Task 5 entry** per D-P3.

- [ ] **Step 7: Commit**

```bash
git add internal/lua/vm.go internal/lua/sandbox.go \
        internal/lua/vm_test.go internal/lua/sandbox_test.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 5: internal/lua/vm.go + sandbox.go full IMPL

Lands full VM lifecycle (NewVM + per-stdlib selective OpenXxx + post-walk
nil-out of denied base + os entries + print-redirect + INTERNAL
__envoy_traceback for panic-wrapper + Run + HasGlobalFunc + CallGlobal +
Close + panic-wrapper recovering Go panics) per 22.1 SPEC §3.1 + §3.3
StrictUpstreamParity default-deny posture per AMEND-1.

Sandbox roster fully enforced: dofile/loadfile/loadstring/load/module/
require/collectgarbage/getfenv/setfenv/io.open/io.popen/os.execute/exit/
remove/rename/getenv/setlocale/tmpname/debug.getupvalue/setupvalue/setfenv/
package.path/cpath/loaded/channel.make/send/receive all nil-or-runtime-error
post-sandbox-strict construction. coroutine + os.time/date/clock/difftime
ARE callable (AllowCoroutine + AllowOSTimeHelpers zero-value-defaulted true)."
```

---

## Tier B — bridge methods (Tasks 6-9)

## Task 6: `bridge.go` headers + `__pairs` alphabetical-snapshot per §11 D7

**Files:**
- Create: `internal/filter/http/lua/bridge.go` (Task 6 contribution ~150-200 LoC; full file grows to ~400-600 LoC by Task 9)
- Create: `internal/filter/http/lua/bridge_test.go` (Task 6 contribution ~250-350 LoC; full file grows to ~600-900 LoC by Task 9)
- Append: PROGRESS.md (Task 6 entry per D-P3)

This task lands the `request_handle`/`response_handle` userdata + metatable setup + 7 headers-object methods + the `__pairs` alphabetical-snapshot metamethod per this 22.1 SPEC §11.2 D7 resolution. **PARALLELIZABLE with Tasks 7 + 8** per D-P8 (each bridge-method group is file-disjoint within `bridge.go` — separate function bodies + separate test bodies in `bridge_test.go`; concurrent subagents append their bridge surface then merge).

**Precondition:** Task 5 complete (the `*VM` lifecycle is consumed by the userdata setup).
**Artifact:** Headers bridge + `__pairs` discipline + cross-run-determinism test.
**Acceptance:** 7 headers methods byte-compatible with upstream `wrappers.cc` semantics (case-insensitive name lookup; `:add` appends; `:remove` deletes; `:replace` removes-then-adds); `__pairs` alphabetical-deterministic across N=100 runs; `go test -count=1 ./internal/filter/http/lua/... -run 'TestBridge_Headers|TestBridge_Pairs'` clean.

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 6 per the 22.1 PLAN Task 6 + 22.1 SPEC §6 Task 6 + parent §11.2 + this 22.1 SPEC §11.2 D7 resolution. The `request_handle` userdata wraps `*requestHandleContext` (per 22.1 SPEC §4.3); metatable `__index` → table of bridge methods; metatable `__pairs` → alphabetical-snapshot iterator. 7 headers methods: `:get(name)` returns first value or nil; `:getAtIndex(name, idx)` returns N-th value or nil (1-indexed per Lua convention); `:getNumValues(name)` returns count; `:add(name, val)` appends via `http.Header.Add`; `:append(name, val)` alias for `:add` per upstream Envoy semantics; `:remove(name)` calls `http.Header.Del`; `:replace(name, val)` removes-then-adds. `__pairs` metamethod snapshots `http.Header` map into `[]struct{k,v string}` sorted alphabetically by k (case-insensitive sort via `strings.ToLower` then byte-compare; matches `net/http.Header.Write` emit-order discipline); returns a stateful iterator function that walks the slice by integer index — closes per-run map-iteration non-determinism for script-author debugging. Cross-run-determinism test: run `__pairs` N=100 times against same headers + assert byte-identical iteration order each time.

- [ ] **Step 1: Write failing tests** in `internal/filter/http/lua/bridge_test.go` per the File-structure table row above. Cover 7 headers methods + `__pairs` alphabetical-snapshot + cross-run determinism (N=100 iterations).

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `internal/filter/http/lua/bridge.go` Task 6 contribution** — `requestHandleContext`/`responseHandleContext` structs per 22.1 SPEC §4.3; `installRequestHandleMetatable(L *lua.LState) *lua.LTable` returning the metatable with `__index` → method table + `__pairs` → alphabetical-snapshot iterator; 7 headers methods.

- [ ] **Step 4: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/filter/http/lua/... -run 'TestBridge_Headers|TestBridge_Pairs'
# Expect: PASS — 7 headers + __pairs determinism
```

- [ ] **Step 5: Verify `go build` + `go vet` + `golangci-lint` clean.**

- [ ] **Step 6: Append PROGRESS.md Task 6 entry** per D-P3.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/lua/bridge.go internal/filter/http/lua/bridge_test.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 6: bridge.go headers + __pairs alphabetical-snapshot

Lands request_handle/response_handle userdata + metatable + 7 headers
methods (:get/:getAtIndex/:getNumValues/:add/:append/:remove/:replace)
byte-compatible with upstream wrappers.cc semantics. __pairs metamethod
snapshots net/http.Header (Go map[string][]string per types.go:55
empirically-determined at 22.1 SPEC §11.2 D7 closure) into alphabetically-
sorted slice via strings.ToLower-then-byte-compare; iterates by integer
index — closes per-run map-iteration non-determinism for script-author
debugging without insertion-order infrastructure across filter callbacks.

§13-R3 RATIFIED-PENDING-IMPL-TIME item (REFINED at 22.1 SPEC §11.2 D7)
CLOSED at this Task. Cross-run determinism verified across N=100 runs."
```

---

## Task 7: `bridge.go` log methods (6 `:logXxx`)

**Files:**
- Modify: `internal/filter/http/lua/bridge.go` (Task 7 contribution ~80-100 LoC)
- Modify: `internal/filter/http/lua/bridge_test.go` (Task 7 contribution ~120-180 LoC)
- Append: PROGRESS.md (Task 7 entry per D-P3)

This task lands the 6 `:logTrace`/`:logDebug`/`:logInfo`/`:logWarn`/`:logErr`/`:logCritical` methods wrapping the Go stdlib `"log"` package. **PARALLELIZABLE with Tasks 6 + 8** per D-P8 (file-disjoint within `bridge.go` separate function bodies).

**Precondition:** Task 5 complete; Task 6 complete OR concurrent with merge-on-completion.
**Artifact:** 6 log methods + table-driven log-level routing test.
**Acceptance:** 6 log methods route to correct log levels; format pin `"<LEVEL> lua: <msg>"` preserved across all 6 levels; `go test -count=1 ./internal/filter/http/lua/... -run 'TestBridge_Log'` clean.

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 7 per the 22.1 PLAN Task 7 + 22.1 SPEC §6 Task 7. The 6 log methods wrap the Go stdlib `"log"` package at the corresponding log level (the canonical project log sink per `extauthz.go:18` + `extproc.go:26` + `rbac.go:6` + `router_h2.go:7` + `extproc/processor.go:52`). Format: `"<LEVEL> lua: <msg>"` prefix preserved across all 6 levels for log-greppability. Log-level mapping: `:logTrace` + `:logDebug` → `log.Printf("DEBUG lua: %s", msg)`; `:logInfo` → `log.Printf("INFO lua: %s", msg)`; `:logWarn` → `log.Printf("WARN lua: %s", msg)`; `:logErr` → `log.Printf("ERROR lua: %s", msg)`; `:logCritical` → `log.Printf("CRIT lua: %s", msg)`. Conservative mapping to stdlib-`log` (which has no native levels); future log-leveling primitive (if introduced cross-project) replaces verbatim per its own ADR. Test uses a captured-log-sink test double via `log.SetOutput(buf)` + `defer log.SetOutput(os.Stderr)`; verifies each method calls into the right log level + format.

- [ ] **Step 1: Write failing tests** in `bridge_test.go` covering 6 log methods using `log.SetOutput(buf)` captured-sink discipline.

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `bridge.go` Task 7 contribution** — 6 log methods.

- [ ] **Step 4: Run tests to verify they pass.**

- [ ] **Step 5: Verify `go build` + `go vet` + `golangci-lint` clean.**

- [ ] **Step 6: Append PROGRESS.md Task 7 entry** per D-P3.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/lua/bridge.go internal/filter/http/lua/bridge_test.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 7: bridge.go 6 :logXxx methods

Lands 6 :logTrace/:logDebug/:logInfo/:logWarn/:logErr/:logCritical
methods wrapping Go stdlib log.Printf at corresponding LEVEL prefix.
Format pin '<LEVEL> lua: <msg>' preserved across all 6 levels for
log-greppability. Conservative mapping to stdlib-log (no native levels);
future log-leveling primitive replaces verbatim per its own ADR.

Table-driven log-level routing test via log.SetOutput(buf) captured-sink
verification passes."
```

---

## Task 8: `bridge.go` streamInfo subset (4 methods)

**Files:**
- Modify: `internal/filter/http/lua/bridge.go` (Task 8 contribution ~80-100 LoC)
- Modify: `internal/filter/http/lua/bridge_test.go` (Task 8 contribution ~120-180 LoC)
- Append: PROGRESS.md (Task 8 entry per D-P3)

This task lands the `:streamInfo()` userdata + 4 methods (`:protocol` + `:routeName` + `:downstreamLocalAddress` + `:downstreamDirectRemoteAddress`). **PARALLELIZABLE with Tasks 6 + 7** per D-P8 (file-disjoint within `bridge.go`).

**Precondition:** Task 5 complete; Tasks 6 + 7 complete OR concurrent with merge-on-completion.
**Artifact:** 4 streamInfo methods + canned-value test double.
**Acceptance:** 4 streamInfo methods return correctly-formatted strings; protocol mapping covers all 4 HTTP versions; `go test -count=1 ./internal/filter/http/lua/... -run 'TestBridge_StreamInfo'` clean.

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 8 per 22.1 PLAN Task 8 + 22.1 SPEC §6 Task 8. `request_handle:streamInfo()` returns a streamInfo userdata; 4 methods on the userdata: `:protocol()` returns "HTTP/1.0" / "HTTP/1.1" / "HTTP/2" / "HTTP/3" depending on stream protocol; `:routeName()` returns the resolved route name string (from filter callback `cb.RouteName()` or equivalent); `:downstreamLocalAddress()` returns "ip:port" formatted local address; `:downstreamDirectRemoteAddress()` returns "ip:port" formatted remote address. Test uses a test-double `DecoderFilterCallbacks` carrying canned protocol/route/address values.

- [ ] **Step 1: Write failing tests** in `bridge_test.go` covering 4 streamInfo methods via test-double `DecoderFilterCallbacks`.

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `bridge.go` Task 8 contribution** — `streamInfo` userdata + 4 methods.

- [ ] **Step 4: Run tests to verify they pass.**

- [ ] **Step 5: Verify `go build` + `go vet` + `golangci-lint` clean.**

- [ ] **Step 6: Append PROGRESS.md Task 8 entry** per D-P3.

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/lua/bridge.go internal/filter/http/lua/bridge_test.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 8: bridge.go streamInfo subset (4 methods)

Lands :streamInfo() userdata + 4 methods (:protocol/:routeName/
:downstreamLocalAddress/:downstreamDirectRemoteAddress) per BRAINSTORM
Q6 pragmatic-middle bridge surface scope. Test uses test-double
DecoderFilterCallbacks carrying canned protocol/route/address values."
```

---

## Task 9: `bridge.go` respond + `decode_headers.go` + `encode_headers.go`

**Files:**
- Modify: `internal/filter/http/lua/bridge.go` (Task 9 contribution ~80-130 LoC; full file ~400-600 LoC at this point)
- Create: `internal/filter/http/lua/decode_headers.go` (~100-150 LoC)
- Create: `internal/filter/http/lua/encode_headers.go` (~80-120 LoC)
- Modify: `internal/filter/http/lua/bridge_test.go` (Task 9 contribution ~150-250 LoC)
- Modify: `internal/filter/http/lua/lua_test.go` (Task 9 +decode/encode integration tests ~150-250 LoC)
- Append: PROGRESS.md (Task 9 entry per D-P3)

This task lands `request_handle:respond` byte-pin per parent §11.6.7 + AMEND-7 + AMEND-8 + `decode_headers.go` + `encode_headers.go` per 22.1 SPEC §4.3. **PARALLELIZABLE with Tasks 10 + 13** per D-P8 (file-disjoint surfaces).

**Precondition:** Tasks 6 + 7 + 8 complete (the headers + log + streamInfo bridge surfaces exist).
**Artifact:** Respond byte-pin + decode/encode dispatch + AMEND-8 runtime-reject.
**Acceptance:** Respond byte-pin matches parent §11.6.7 verbatim (4-tuple `{status: 403, content-length: 6, content-type: text/plain, body: "denied"}`); `:status` validation byte-exact `":status must be between 200-599"`; encode-side `:respond()` runtime-reject byte-exact `"respond not currently supported in the response path"`; decode + encode integration end-to-end correct; `go test -count=1 ./internal/filter/http/lua/...` clean.

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 9 per 22.1 PLAN Task 9 + 22.1 SPEC §6 Task 9. `request_handle:respond(headers_table, body_string)`: extract `:status` from headers_table (raise byte-exact `":status must be between 200-599"` if outside `[200,600)` per AMEND-8); auto-set `content-length` from body size when body non-empty; apply `content-type: text/plain` default if headers_table did not supply content-type (per upstream `Utility::prepareLocalReply` at `utility.cc:1241,1273`); capture `respondState` on filter for the decode path to read. `response_handle:respond()` raises byte-exact `"respond not currently supported in the response path"` per AMEND-8. `DecodeHeaders` body per 22.1 SPEC §4.3: construct per-stream `*VM`; set up `request_handle` userdata + metatable; `vm.Run(cc.chunk)` (errors → `cc.stats.errors++` + log + continue); `vm.HasGlobalFunc("envoy_on_request")` check; `cc.stats.executions++`; `vm.CallGlobal(...)` (errors → `cc.stats.errors++` + log + continue); respond-state check → `cc.stats.respondCalls++` + `cb.SendLocalReply(captured)` + return `StopIteration`; else return `Continue`. `EncodeHeaders` symmetric for `envoy_on_response` + response_handle. `OnDestroy` calls `vm.Close()`.

- [ ] **Step 1: Write failing tests** for respond byte-pin + encode-side reject + decode/encode integration in `bridge_test.go` + `lua_test.go`.

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `bridge.go` Task 9 contribution** (respond + encode-side reject).

- [ ] **Step 4: Author `internal/filter/http/lua/decode_headers.go`** per 22.1 SPEC §4.3.

- [ ] **Step 5: Author `internal/filter/http/lua/encode_headers.go`** symmetric to decode.

- [ ] **Step 6: Run tests to verify they pass.**

- [ ] **Step 7: Verify `go build` + `go vet` + `golangci-lint` clean.**

- [ ] **Step 8: Append PROGRESS.md Task 9 entry** per D-P3.

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/lua/bridge.go \
        internal/filter/http/lua/decode_headers.go \
        internal/filter/http/lua/encode_headers.go \
        internal/filter/http/lua/bridge_test.go \
        internal/filter/http/lua/lua_test.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 9: bridge.go respond + decode_headers.go + encode_headers.go

Lands request_handle:respond per parent §11.6.7 byte-pin + AMEND-7 (full
4-tuple: status 200..599 validated per AMEND-8 byte-exact wording;
content-length auto-set from body size; content-type text/plain default
per upstream Utility::prepareLocalReply at utility.cc:1241,1273; captures
respondState on filter for decode path).

response_handle:respond raises byte-exact 'respond not currently supported
in the response path' per AMEND-8.

decode_headers.go: per-stream VM construction + envoy_on_request hook
firing + respond-state SendLocalReply on captured per 22.1 SPEC §4.3.
encode_headers.go: symmetric for envoy_on_response."
```

---

## Tier C — stats + tests (Tasks 10-12)

## Task 10: `stats.go` + boot-registration at `cmd/envoy-go/main.go` + `lua.go` full `New` body

**Files:**
- Create: `internal/filter/http/lua/stats.go` (~80-120 LoC)
- Modify: `internal/filter/http/lua/lua.go` (Task 10 contribution ~70-140 LoC; `New` full body + `newFilterStats` body)
- Modify: `internal/filter/http/lua/lua_test.go` (+stats registration + empty-stat-prefix consecutive-dot + cardinality + `TestStatNames_Equal_*` ~80-150 LoC)
- Modify: `cmd/envoy-go/main.go` (~+2 LoC + +1 import; alphabetical between `localratelimit` and `oauth2`)
- Append: PROGRESS.md (Task 10 entry per D-P3)

This task lands the 3-counter stat surface + boot-registration + the full `New` factory body wiring. **PARALLELIZABLE with Tasks 9 + 13** per D-P8 (file-disjoint).

**Precondition:** Tasks 6 + 7 + 8 complete; Task 9 complete OR concurrent with the integration sub-task; Task 4 + Task 5 complete (compileScript + NewVM consumed by `New`).
**Artifact:** 3-counter stats + boot-reg insertion + functional `New` factory.
**Acceptance:** 3 counters registered under correct HCM-rooted template `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>`; empty `Lua.stat_prefix` consecutive-dot wire name verified (per AMEND-2 `http.<HCM>.lua..errors` literal); cardinality assertion (3 counters registered per filter instance); boot-registration insertion alphabetical-correct; `cmd/envoy-go` builds; 17 HTTP filters wired post-Task-10 (was 16).

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 10 per 22.1 PLAN Task 10 + 22.1 SPEC §6 Task 10 + parent §7 + AMEND-2 + AMEND-3. `filterStats` 3-counter struct (`errors`/`executions`/`respondCalls`) per 22.1 SPEC §4.2; `newFilterStats(reg *stats.Registry, hcmStatPrefix string, configStatPrefix string) *filterStats` constructs via `reg.NewCounter` under template `http.<hcmStatPrefix>.lua.<configStatPrefix>.<stat>` per parent §7.2 + AMEND-2. Package-level const declarations for the 3 stat names per D-P6 + ADR-0143 SN2-reuse. `TestStatNames_Equal_*` table-driven byte-exact assertion. Empty-stat-prefix consecutive-dot verification (`http.<HCM>.lua..errors` literal). `New()` factory body wiring per ADR-0085 nil-tolerance (`if ctx.Stats != nil` guard); registers per-route validator inside `lua.New` per D-P6 + ADR-0110 single-chokepoint. `cmd/envoy-go/main.go` boot-reg insertion alphabetical between `localratelimit.New` and `oauth2.New`.

- [ ] **Step 1: Write failing tests** for stats registration + cardinality + empty-stat-prefix + `TestStatNames_Equal_*` in `lua_test.go`.

- [ ] **Step 2: Run tests to verify they fail.**

- [ ] **Step 3: Author `stats.go`** per 22.1 SPEC §4.2 + parent §7.

- [ ] **Step 4: Author `lua.go` Task 10 contribution** — `New` full body wiring (calls `buildCompiledConfig` from Task 2 + `resolveDataSource` from Task 3 + `CompileScript` from Task 4 + `newFilterStats` from this Task + registers per-route validator); returns `FilterInstanceFactory` closure producing per-stream `*filter`.

- [ ] **Step 5: Modify `cmd/envoy-go/main.go`** — add import `lua "github.com/esalaine/envoy-go/internal/filter/http/lua"` alphabetical-among-imports; add `httpReg.Register(lua.TypeURL, lua.New)` alphabetical between `localratelimit.New` and `oauth2.New` per ADR-0100 §2.2.

- [ ] **Step 6: Run tests to verify they pass.**

```bash
go test -count=1 ./internal/filter/http/lua/...
# Expect: PASS — stats + factory + integration
go build ./cmd/envoy-go/
# Expect: clean — 17 HTTP filters wired post-Task-10
```

- [ ] **Step 7: Verify `go build` + `go vet` + `golangci-lint` clean across all packages.**

- [ ] **Step 8: Append PROGRESS.md Task 10 entry** per D-P3.

- [ ] **Step 9: Commit**

```bash
git add internal/filter/http/lua/stats.go internal/filter/http/lua/lua.go \
        internal/filter/http/lua/lua_test.go cmd/envoy-go/main.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 10: stats.go + boot-registration + lua.New full body

Lands 3-counter filterStats (errors + executions upstream-parity per
AMEND-3 + respond_calls envoy-go-strict per AMEND-3 corrected) under
HCM-rooted template http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>
per parent §7.2 + AMEND-2. Empty Lua.stat_prefix produces literal
consecutive-dot wire names (http.<HCM>.lua..errors) mirroring phase-14
compressor empty-<library> precedent.

lua.New full body wiring: buildCompiledConfig + resolveDataSource +
CompileScript + newFilterStats + per-route validator registration
(via reg.RegisterPerRouteValidator per ADR-0110 single-chokepoint).

cmd/envoy-go/main.go: httpReg.Register(lua.TypeURL, lua.New) inserted
alphabetical between localratelimit.New and oauth2.New per ADR-0100 §2.2.
17 HTTP filters wired post-Task-10 (was 16)."
```

---

## Task 11: `fuzz_test.go` — 28th project-wide fuzzer `FuzzLuaConfigParse`

**Files:**
- Create: `internal/filter/http/lua/fuzz_test.go` (~80-130 LoC)
- Create: `internal/filter/http/lua/testdata/fuzz/FuzzLuaConfigParse/` (corpus seeds ~30 per D-P7)
- Append: PROGRESS.md (Task 11 entry per D-P3)

This task lands the 28th project-wide fuzzer per ADR-0018 baseline. **PARALLELIZABLE with Tasks 2 + 3 + 4** per D-P8 (skeleton lands early; full clean-run depends on Task 2's `buildCompiledConfig` being non-skeleton).

**Precondition:** Task 1 complete (for the skeleton fuzz_test.go); Task 2 complete (for the clean 30s fuzz run against non-skeleton `buildCompiledConfig`).
**Artifact:** Fuzzer + 30-seed corpus.
**Acceptance:** `FuzzLuaConfigParse` at 30s baseline returns no crashes; corpus seeds exercise all 18 PARSE-REJECT arms + 5 valid + 7 adversarial paths per D-P7; must-never-panic invariant per ADR-0018 verified; fuzzer count CONFIRMED at 28 (was 27 per 22.1 SPEC §11.1 D5 closure).

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 11 per 22.1 PLAN Task 11 + D-P7 corpus seed roster. Standard ADR-0018 baseline fuzzer: fuzzer body parses + calls `New()` via test-double `FilterFactoryContext`; no panic regardless of input; PARSE-REJECT errors are expected behavior, not panic. Corpus seeds at `internal/filter/http/lua/testdata/fuzz/FuzzLuaConfigParse/` per D-P7 (30 total: 18 per-PARSE-REJECT-arm + 5 valid-config + 7 adversarial-Lua-source). Run `go test -fuzz=FuzzLuaConfigParse -fuzztime=30s` clean (no panics).

- [ ] **Step 1: Author `fuzz_test.go`** per ADR-0018 baseline. Must-never-panic across `New()`.

- [ ] **Step 2: Author corpus seeds** per D-P7 roster — 30 total.

- [ ] **Step 3: Run fuzzer for 30s smoke**

```bash
go test -fuzz=FuzzLuaConfigParse -fuzztime=30s ./internal/filter/http/lua/
# Expect: clean (no panics)
```

- [ ] **Step 4: Verify fuzzer count**

```bash
find . -name 'fuzz_test.go' -not -path '*/.worktrees/*' -not -path '*/.claude/*' \
    | xargs grep -h '^func Fuzz' | sort -u | wc -l
# Expect: 28
```

- [ ] **Step 5: Append PROGRESS.md Task 11 entry** per D-P3.

- [ ] **Step 6: Commit**

```bash
git add internal/filter/http/lua/fuzz_test.go \
        internal/filter/http/lua/testdata/fuzz/FuzzLuaConfigParse/ \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 11: 28th fuzzer FuzzLuaConfigParse + corpus

Lands FuzzLuaConfigParse per ADR-0018 baseline. Standard must-never-panic
across New(). Corpus seeds per D-P7 roster (30 total): 18 per-PARSE-REJECT-
arm + 5 valid-config (one per DataSource arm + one with explicit stat_prefix)
+ 7 adversarial-Lua-source (syntax errors triggering arm 16; sandbox-
breaking attempts that compile-clean but error at runtime).

Project-wide fuzzer count CONFIRMED at 28 (was 27 per 22.1 SPEC §11.1
D5 closure). Clean at 30s baseline; no panics."
```

---

## Task 12: Race + concurrency tests + `*LState`-pool benchmark sub-task per D-P10

**Files:**
- Modify: `internal/lua/vm_test.go` (+concurrency tests ~100-150 LoC)
- Modify: `internal/lua/compile_test.go` (+`TestCompileCache_ConcurrentReadAdd_*` ~80-120 LoC)
- Modify: `internal/filter/http/lua/lua_test.go` (+per-stream-filter-dispatch race tests ~100-150 LoC; +`BenchmarkPerStreamLState_Construction_Headers` ~30-50 LoC)
- Append: PROGRESS.md (Task 12 entry per D-P3 + benchmark `ns/op` quote + R6 disposition per D-P10)

This task lands the race + concurrency test surface + the `*LState`-pool benchmark sub-task. **PARALLELIZABLE with Task 14** per D-P8 (file-disjoint).

**Precondition:** Tasks 5 + 9 + 10 complete (the VM + filter integration + stats surfaces being tested concurrently).
**Artifact:** All race tests clean under `go test -race -count=10` (10 iterations to catch flakes); benchmark `ns/op` quoted; R6 disposition resolved per D-P10 (escape-valve fires if `ns/op > 1_000_000` = 1ms).
**Acceptance:** `go test -race -count=10 ./internal/lua/... ./internal/filter/http/lua/...` clean; no cross-stream state leak; `BenchmarkPerStreamLState_Construction_Headers` reports `ns/op` under 1_000_000 (R6 stays unconsumed) OR over (ADR-0190 escape-valve fires at Task 16).

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 12 per 22.1 PLAN Task 12 + 22.1 SPEC §6 Task 12 + D-P10 benchmark gate. Concurrent-VM-construction tests: N=100 goroutines each call `NewVM(opts...)` + `Run(chunk)` + `CallGlobal(name, args...)` + `Close()` against the same `*Chunk` from the same `*CompileCache`; assert no cross-VM state leak; race-free under `-race`. CompileCache concurrent-read/add: N=100 goroutines mix read (CompileScript with same hash) and add (CompileScript with new content); assert no data race + correct cache contents. Per-stream filter dispatch race tests via test-double `DecoderFilterCallbacks`. Benchmark: `BenchmarkPerStreamLState_Construction_Headers` measures per-stream `*lua.LState` construction cost at headers-only bridge surface; report `ns/op` via standard `b.N` discipline. Per D-P10 gate: if `ns/op > 1_000_000` (= 1ms), record R6 escape-valve fires + signal ADR-0190 firing for Task 16; else WEAK-default per-stream construction STANDS.

- [ ] **Step 1: Write failing tests** for concurrent VM construction + concurrent cache + per-stream-filter race.

- [ ] **Step 2: Author benchmark** `BenchmarkPerStreamLState_Construction_Headers` in `lua_test.go`.

- [ ] **Step 3: Run tests + benchmark to verify they pass**

```bash
go test -race -count=10 ./internal/lua/... ./internal/filter/http/lua/...
# Expect: PASS — race-clean across 10 iterations
go test -bench=BenchmarkPerStreamLState_Construction_Headers -benchtime=3s ./internal/filter/http/lua/
# Expect: ns/op output; capture for PROGRESS.md
```

- [ ] **Step 4: Verify `go build` + `go vet` + `golangci-lint` clean.**

- [ ] **Step 5: Append PROGRESS.md Task 12 entry** per D-P3 — quote benchmark output verbatim + record R6 disposition (escape-valve fires if `ns/op > 1_000_000` else WEAK-default STANDS).

- [ ] **Step 6: Commit**

```bash
git add internal/lua/vm_test.go internal/lua/compile_test.go \
        internal/filter/http/lua/lua_test.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 12: race + concurrency tests + LState-pool benchmark

Lands concurrent-VM-construction tests (N=100 goroutines against same
*Chunk + *CompileCache); CompileCache concurrent-read/add (N=100 mixed
read/add); per-stream filter dispatch race tests via test-double
DecoderFilterCallbacks. All race tests clean under go test -race -count=10.

BenchmarkPerStreamLState_Construction_Headers: <ns/op value>. Per D-P10
R6 gate: <STANDS WEAK-default | ADR-0190 escape-valve FIRES at Task 16>."
```

---

## Tier D — differential fixture (Tasks 13-15)

## Task 13: NEW `BackendKind=HTTPLua` + `BootRejectFixture` infrastructure

**Files:**
- Modify: `test/differential/fixture/fixture.go` (+1 enum value `HTTPLua = 22` + doc-comment; ~+15 LoC)
- Modify: `test/differential/runner_test.go` (+blank import + switch-case for `HTTPLua` ~+12 LoC; + `runBootRejectFixture` branch ~+50 LoC)
- Modify: `test/differential/harness.go` (NEW `BootRejectFixture` interface + `tryStartReferenceProxy`/`tryStartSubjectProxy` variants ~+80-130 LoC)
- Append: PROGRESS.md (Task 13 entry per D-P3)

This task lands the differential-harness infrastructure for fixture-0026: NEW `BackendKind=HTTPLua` constant + the OPTIONAL `BootRejectFixture` driver interface + the new `runBootRejectFixture` runner branch. **PARALLELIZABLE with Tasks 9 + 10** per D-P8 (file-disjoint).

**Precondition:** None beyond Task 1 (the harness deltas are file-disjoint from the production package surfaces).
**Artifact:** Harness infrastructure for fixture-0026 + scenario (g) boot-reject path.
**Acceptance:** `BackendKind=HTTPLua` switch-case lands; `BootRejectFixture` interface signature defined + harness `tryStart*` variants implemented + runner `runBootRejectFixture` branch implemented; harness tests pass; no regression in existing 27 fixture directories (`go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-5])'` clean).

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 13 per 22.1 PLAN Task 13 + 22.1 SPEC §6 Task 13 + parent §13-R1 + §11.7.3. NEW `BackendKind=HTTPLua = 22` enum value at `test/differential/fixture/fixture.go` after `HTTPAdaptiveConcurrency = 21` (matches AMEND-11 prescription). NEW OPTIONAL `BootRejectFixture` driver interface at `test/differential/harness.go`: `BootRejectScript() string` returns path to broken script relative to fixture dir; `ExpectedBootErrorSubstring() string` returns substring to assert in stderr. NEW `tryStartReferenceProxy(ctx, fix) (cancel func(), stderrBuf *bytes.Buffer, err error)` + `tryStartSubjectProxy` variants paralleling existing `StartReferenceProxy`/`StartSubjectProxy` but returning boot error + stderr buffer instead of `t.Fatalf`-ing. NEW `runBootRejectFixture` branch at `runner_test.go` parallel to `runReferenceLessFixture` at `runner_test.go:1268`; asserts both sides exit non-zero AND both sides' stderr contains substring `ExpectedBootErrorSubstring()`. Regression check: existing 27 fixtures stay green.

- [ ] **Step 1: Write failing tests** for `BootRejectFixture` interface contract in `test/differential/harness_test.go` (if exists) or inline test in `harness.go`.

- [ ] **Step 2: Author `BackendKind=HTTPLua`** at `test/differential/fixture/fixture.go`.

- [ ] **Step 3: Author `BootRejectFixture` interface + `tryStart*` variants** at `test/differential/harness.go`.

- [ ] **Step 4: Author `runBootRejectFixture` branch** at `test/differential/runner_test.go`.

- [ ] **Step 5: Verify `go build` + `go vet` + `golangci-lint` clean + existing 27 fixtures still green**

```bash
go build ./test/differential/...
go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-5])'
# Expect: PASS — all 27 pre-existing fixtures still green
```

- [ ] **Step 6: Append PROGRESS.md Task 13 entry** per D-P3.

- [ ] **Step 7: Commit**

```bash
git add test/differential/fixture/fixture.go test/differential/runner_test.go \
        test/differential/harness.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 13: BackendKind=HTTPLua + BootRejectFixture infra

Lands NEW BackendKind=HTTPLua=22 enum value per AMEND-11. NEW OPTIONAL
BootRejectFixture driver interface (BootRejectScript + ExpectedBootError
Substring) per parent §13-R1; tryStartReferenceProxy + tryStartSubjectProxy
variants returning boot error + stderr buffer per parent §11.7.3. NEW
runBootRejectFixture runner branch paralleling runReferenceLessFixture;
asserts both sides exit non-zero + stderr contains expected substring.

Pre-existing 27 fixtures stay GREEN — no regression in harness changes."
```

---

## Task 14: fixture-0026 directory + `scripts/` subdirectory + 7 `.lua` sources + driver

**Files:**
- Create: `test/fixtures/0026-http-lua-headers-bridge/README.md` (~150-250 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/envoy.yaml` (~150-250 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/envoy-go.yaml` (~150-250 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/expectations.yaml` (~100-180 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/inputs/driver.go` (~400-600 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/scripts/a_add_header.lua` (~5 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/scripts/b_replace_header.lua` (~5 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/scripts/c_remove_header.lua` (~5 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/scripts/d_respond.lua` (~5 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/scripts/e_log_only.lua` (~5 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/scripts/f_headers_iter.lua` (~10 LoC)
- Create: `test/fixtures/0026-http-lua-headers-bridge/scripts/g_compile_error.lua` (~5 LoC; intentionally broken)
- Append: PROGRESS.md (Task 14 entry per D-P3)

This task lands the fixture-0026 directory + the 7 `.lua` script files + the driver impl per this 22.1 SPEC §9.4. **PARALLELIZABLE with Task 12** per D-P8 (file-disjoint).

**Precondition:** Tasks 9 + 10 + 13 complete (the production filter must work end-to-end + the harness must support the BackendKind + boot-reject branch).
**Artifact:** Fixture-0026 directory + 7 Lua scripts + driver + per-scenario probes.
**Acceptance:** Fixture-0026 directory layout matches parent §8.4 + this 22.1 SPEC §9; 7 `.lua` files committed verbatim per 22.1 SPEC §9.1 7-scenario table; driver.go scenario-probe shape mirrors fixture-0023's pattern; scenario (e) probe scrapes `/stats?format=text` + emits `scenario e executions_delta=N` per D3 closure at this PLAN; **fixture-0026 GREEN deferred to Task 15** (the green-light depends on envoy-go-side `"script load error: "` wording-pinning at Task 15 + the production filter being complete).

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 14 per 22.1 PLAN Task 14 + 22.1 SPEC §6 Task 14 + this 22.1 SPEC §9. Fixture-0026 directory layout per AMEND-11 + parent §8.4. 7 `.lua` script files verbatim per 22.1 SPEC §9.1 (the `function envoy_on_request(rh) rh:headers():add(...) end` patterns). Driver impl mirrors fixture-0023's pattern: registered `Driver` impl + `BootRejectFixture` impl (`BootRejectScript() string` returns `"scripts/g_compile_error.lua"`; `ExpectedBootErrorSubstring() string` returns `"script load error"`); per-scenario probes via `driveProxy` + `emitScenario` + `classifyBody`. Scenario (e) per D3 closure at this PLAN's D3 + option (a) stat-counter delta: probe the request → scrape `/stats?format=text` admin endpoint pre + post → diff → emit `scenario e executions_delta=N` into byte-comparison buffer (uses the existing fixture-0025 inline scrape pattern at `test/fixtures/0025-http-adaptive-concurrency/inputs/driver.go:149-153,236-272`). Fixture-0026 green-light deferred to Task 15.

- [ ] **Step 1: Create directory structure**

```bash
mkdir -p test/fixtures/0026-http-lua-headers-bridge/inputs \
         test/fixtures/0026-http-lua-headers-bridge/scripts
```

- [ ] **Step 2: Author 7 `.lua` script files** verbatim per 22.1 SPEC §9.1.

- [ ] **Step 3: Author `envoy.yaml`** — reference Envoy bootstrap with single listener + lua filter consuming `Lua.DefaultSourceCode` via `Filename` arm pointing to `scripts/<scenario>.lua`; templated `{{.BackendPort}}`.

- [ ] **Step 4: Author `envoy-go.yaml`** — subject bootstrap with same topology; templated `{{.AdminPort}} {{.ListenerPort}} {{.BackendPort}} {{.FixtureDir}}`.

- [ ] **Step 5: Author `expectations.yaml`** — human-readable scenario expectations; documentation aid.

- [ ] **Step 6: Author `inputs/driver.go`** — registered `Driver` impl + `BootRejectFixture` impl per this 22.1 SPEC §9.2; per-scenario probes; scenario (e) per D3 closure inline `/stats` scrape.

- [ ] **Step 7: Author `README.md`** — scope + 7-scenario table + topology + cross-refs.

- [ ] **Step 8: Verify `go build ./test/...` clean.**

- [ ] **Step 9: Append PROGRESS.md Task 14 entry** per D-P3 — note that fixture green-light is deferred to Task 15.

- [ ] **Step 10: Commit**

```bash
git add test/fixtures/0026-http-lua-headers-bridge/ \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 14: fixture-0026 directory + 7 .lua scripts + driver

Lands test/fixtures/0026-http-lua-headers-bridge/ directory per AMEND-11
+ parent §8.4 + this 22.1 SPEC §9.4. 7 .lua script files verbatim per
22.1 SPEC §9.1 (a_add_header + b_replace_header + c_remove_header +
d_respond + e_log_only + f_headers_iter + g_compile_error). Driver impl
mirrors fixture-0023 pattern + implements BootRejectFixture (returns
scripts/g_compile_error.lua + 'script load error' substring).

Scenario (e) per D3 closure at PLAN session (locked at parent §11.7.7
RECOMMENDED option (a) — stat-counter executions delta IS the 'Lua ran'
assertion): driver scrapes /stats?format=text pre + post per probe;
emits 'scenario e executions_delta=N' into byte-comparison buffer
(reuses fixture-0025 inline scrape pattern at driver.go:149-153,236-272).

Fixture green-light deferred to Task 15 (depends on envoy-go-side
'script load error: ' wording-pinning at cmd/envoy-go/main.go:60-66
per parent §13-W)."
```

---

## Task 15: envoy-go-side `"script load error: "` wording-pinning + fixture-0026 green-light

**Files:**
- Modify: `cmd/envoy-go/main.go` (~+30-50 LoC at boot-reject path; wraps gopher-lua compile error with `"script load error: "` prefix per parent §13-W)
- Append: PROGRESS.md (Task 15 entry per D-P3 + fixture-0026 GREEN evidence)

This task wraps the gopher-lua compile error at envoy-go's boot-reject path with the `"script load error: "` prefix matching upstream Envoy v1.37.2 per parent §11.7.5; lights up fixture-0026 green per 22.1 SPEC §6 Task 15.

**Precondition:** Tasks 12 + 14 complete (the production filter is fully implemented + the fixture is in place + the harness supports the boot-reject branch).
**Artifact:** Boot-reject wording-pinning at main.go + fixture-0026 GREEN.
**Acceptance:** envoy-go-side boot-reject stderr contains literal substring `"script load error"` for the scenario (g) `g_compile_error.lua` input; `go test -count=1 ./test/differential -run 'TestDifferential/0026'` GREEN (6 scenarios (a)-(f) full cross-side byte-exact via `CompareBytes`; scenario (g) substring-match via `BootRejectFixture`).

**Subagent dispatch outline** (per D-P2 `general-purpose`):

> Author Task 15 per 22.1 PLAN Task 15 + 22.1 SPEC §6 Task 15 + parent §13-W. Wrap gopher-lua compile error with `"script load error: "` prefix at boot-reject path. Inspect `cmd/envoy-go/main.go:60-66` for the existing config-load PARSE-REJECT path that surfaces from `lua.New`'s arm 16 (`"lua: default_source_code: compile: %w"` wrapping `*lua.ApiError`); add a wrapping helper at the boot-reject path that adds `"script load error: "` prefix when the error originates from the lua filter's arm 16 compile failure. Ensure boot-reject stderr contains the literal substring `"script load error"` matching upstream Envoy v1.37.2 per parent §11.7.5. Then run fixture-0026 via `go test ./test/differential -run TestDifferential/0026` and assert 6 wire-interactive scenarios (a)-(f) full cross-side byte-exact via existing `CompareBytes` + scenario (g) substring-match via `BootRejectFixture` from Task 13.

- [ ] **Step 1: Inspect `cmd/envoy-go/main.go:60-66`** for existing boot-reject path.

- [ ] **Step 2: Author the `"script load error: "` wrapping helper** at `cmd/envoy-go/main.go` per parent §13-W.

- [ ] **Step 3: Verify `go build` + `go vet` + `golangci-lint` clean.**

- [ ] **Step 4: Run fixture-0026**

```bash
go test -count=1 ./test/differential -run 'TestDifferential/0026'
# Expect: PASS — 6 cross-side byte-exact + scenario (g) substring-match
```

- [ ] **Step 5: Append PROGRESS.md Task 15 entry** per D-P3 — quote fixture-0026 output verbatim; record GREEN evidence; cross-reference 22.1 SPEC §15.2 item 13.

- [ ] **Step 6: Commit**

```bash
git add cmd/envoy-go/main.go \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md
git commit -m "phase 22.1 Task 15: 'script load error: ' wording-pinning + fixture-0026 GREEN

Wraps gopher-lua compile error with 'script load error: ' prefix at
envoy-go boot-reject path (cmd/envoy-go/main.go:60-66) per parent §13-W
+ §11.7.5 ensuring boot-reject stderr contains literal substring matching
upstream Envoy v1.37.2.

Fixture-0026 GREEN: 6 scenarios (a)-(f) full cross-side byte-exact via
CompareBytes; scenario (g) substring-match via BootRejectFixture from
Task 13. 28-by-directory fixture count post-Task-15 (was 27 pre-22.1
counting 0007a + 0007b separately)."
```

---

## Tier E — atomic landing (Task 16)

## Task 16: BEHAVIOR_CONTRACT.md 7-edit bundle + ADR-0188/0189 §Decision+§Consequences + STATE.md re-advance + ROADMAP row 22.1 IMPL-done + REVIEW.md

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (7-edit bundle per parent §14 + this 22.1 SPEC §14; ~+250-400 LoC)
- Modify: `docs/envoy-go/DECISIONS.md` (ADR-0188 + ADR-0189 §Decision + §Consequences body landings; ~+400-600 LoC; CONDITIONAL ADR-0190 if R6 escape-valve fires per D-P10)
- Modify: `docs/envoy-go/STATE.md` (rewrite-in-place per BOOTSTRAP §4.1 invariant 1)
- Modify: `docs/envoy-go/ROADMAP.md` (row 22.1 flips `in-progress → done` + per-cell IMPL-done annotation per ADR-0106; parent row `22` UNCHANGED `in-progress`; sub-rows `22.2` + `22.3` UNCHANGED `planned`)
- Create: `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/REVIEW.md` (~300-400 LoC; per `superpowers:requesting-code-review`)
- Append: PROGRESS.md (Task 16 entry per D-P3 — final task; 6-gate outputs captured verbatim)

This task lands the atomic landing per ADR-0052 atomic landing discipline + parent SPEC §14 7-edit bundle + the 2 ADR §Decision + §Consequences body landings + the STATE/ROADMAP advance + the REVIEW.md authoring. **Depends on Tasks 1-15** (consumes all prior surfaces; closes the 22.1 SPEC §15.2 24-item acceptance checklist).

**Precondition:** Tasks 1-15 complete; all 6 phase-done gates A/B/C/D/E/F GREEN.
**Artifact:** All 7 BEHAVIOR_CONTRACT.md edits + 2 ADR bodies + STATE.md + ROADMAP + REVIEW.md; phase 22.1 IMPL phase-done ready for squash-merge.
**Acceptance:** All 7 BEHAVIOR_CONTRACT.md edits land in one atomic commit; ADR-0188 + ADR-0189 §Decision + §Consequences bodies complete + grep-verified; STATE.md re-advance reflects post-22.1-IMPL state (`lifecycle-state: phase 22.1 IMPL done; awaiting 22.2 SPEC`; `next-skill: superpowers:brainstorming` scoped to 22.2; `next-free ADR: ADR-0190` UNCHANGED if R6 escape-valve does NOT fire OR `ADR-0191` if fires; 99 → 102 stat count; 16 → 17 HTTP filter count; 27 → 28 fuzzer count; ADR tail advance to ADR-0189 or ADR-0190); ROADMAP row 22.1 flipped `in-progress → done`; per-task PROGRESS.md entries complete across all 16 tasks + Pre-Task 0; REVIEW.md authored per `superpowers:requesting-code-review`; all 24 items from 22.1 SPEC §15.2 acceptance checklist closed.

**Subagent dispatch outline** (per D-P2 `general-purpose` with explicit reference to 22.1 SPEC §15.2 + parent §14 + ADR-0188 + ADR-0189):

> Author Task 16 atomic landing per 22.1 PLAN Task 16 + 22.1 SPEC §6 Task 16 + parent SPEC §14 + ADR-0052 atomic landing discipline. The 7 BEHAVIOR_CONTRACT.md edits per parent §14: (1) NEW `### envoy.filters.http.lua` subsection ~80-120 lines headers-bridge-focused for 22.1 + forward-pointers to 22.2 + 22.3; (2) Stat-table 99 → 102 extension under `## Stat surface` (3 new rows under `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` template) + extension summary paragraph mirroring phase-21's `## Phase 21 extension — 92 → 99 internal names` paragraph; (3) envoy-go-strict departure record #1: stdlib-sandbox-strict per AMEND-1; (4) envoy-go-strict departure record #2: `respond_calls` counter per AMEND-3 corrected from BRAINSTORM 2-record bundle; (5) envoy-go-strict departure record #3: runtime-error log-message wording per AMEND-9; (6) NEW `### Phase 22.1 forward-pointer notes` subsection ~30-50 lines (22.2 + 22.3 anticipated additions); (7) Per-route-canonical cross-reference caption update — 1-line edit referencing ADR-0125 §(xiv) AMENDMENT-anticipation paragraph. Plus: ADR-0188 §Decision + §Consequences body landing (extends parent SPEC commit §Context per ADR-0044; covers `internal/lua/` API surface per 22.1 SPEC §3.1 + sandbox roster per §3.3 + per-stream lifecycle per §3.4 + EXPLICIT API-REVISION ALLOWANCE clause for consumer #2 per BRAINSTORM Q4); ADR-0189 §Decision + §Consequences body landing (extends parent SPEC commit §Context per ADR-0044; covers `internal/filter/http/lua/` package shape per 22.1 SPEC §3.5 + §4 + §6.2 18-arm PARSE-REJECT roster + §8 fixture-0026 disposition + §11.1 D5 + §11.2 D7 + Task 2 D1 closure evidence + §13-R1 BootRejectFixture infrastructure + §13-W wording-pinning discipline); CONDITIONAL ADR-0190 §Context + §Decision + §Consequences if D-P10 R6 escape-valve fires (per-script-source `*LState` pool design). STATE.md re-advance per BOOTSTRAP §4.1 invariant 1. ROADMAP row 22.1 flip per ADR-0106. REVIEW.md per `superpowers:requesting-code-review` covering 24-item 22.1 SPEC §15.2 acceptance checklist + per-task review notes + cross-cutting review notes + green-light evidence + D-decision-disposition record. 6 phase-done gates A/B/C/D/E/F outputs captured verbatim in PROGRESS.md final Task 16 entry.

- [ ] **Step 1: Gate A — build** — `go build ./...` clean. Capture output verbatim in PROGRESS.md.

- [ ] **Step 2: Gate B — vet + lint** — `go vet ./...` + `golangci-lint run` clean; no new suppressions. Capture output verbatim.

- [ ] **Step 3: Gate C — race** — `go test -race -count=1 ./...` clean; zero data-race violations across all packages including the new `internal/lua/` + `internal/filter/http/lua/` race tests per D-P10. Capture output verbatim.

- [ ] **Step 4: Gate D — differential + cross-package regression matrix per D-P9** — `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-6])'` clean (all 28 fixture directories GREEN: 27 pre-existing + new 0026). Capture output verbatim.

- [ ] **Step 5: Gate E — fuzz** — `go test -fuzz=FuzzLuaConfigParse -fuzztime=30s ./internal/filter/http/lua/` clean (no panics). 27 pre-existing fuzzers re-run clean at 30s per seed via per-package iteration. Capture output verbatim. Verify project-wide fuzzer count = 28 via the find/grep oneliner.

- [ ] **Step 6: Gate F — h2spec** — `make test-h2spec` 53/53 PASS at ADR-0051 pin. Capture output verbatim.

- [ ] **Step 7: Author BEHAVIOR_CONTRACT.md 7-edit bundle** per parent §14 + this 22.1 SPEC §14.

- [ ] **Step 8: Author ADR-0188 + ADR-0189 §Decision + §Consequences bodies in DECISIONS.md** per parent §4.1 + §4.4 + this 22.1 SPEC §3.1 + §3.5 + §4 + §6.2 + §8 + §11 + §13.

- [ ] **Step 9: IF D-P10 R6 escape-valve fires (per Task 12 benchmark > 1ms threshold)**: author ADR-0190 §Context + §Decision + §Consequences body per ADR-0044 anchoring per-script-source `*LState` pool with chunk-pre-loaded entries.

- [ ] **Step 10: Update STATE.md** to post-phase-22.1-IMPL state per BOOTSTRAP §4.1 invariant 1:
  - `active-phase`: `22-http-filter-lua` (parent row stays in-progress)
  - `lifecycle-state`: `phase 22.1 IMPL done; awaiting 22.2 SPEC`
  - `next-skill`: `superpowers:brainstorming` (the 22.2 BRAINSTORM scoped to 22.2 sub-phase per parent BRAINSTORM Q2 PRE-SPLIT + ADR-0106)
  - `last-commit`: `<TBD — SHA-fill follow-up after squash-merge>` placeholder
  - `last-updated`: today's date
  - `next-free ADR`: `ADR-0190` (UNCHANGED if D-P10 R6 does NOT fire) OR `ADR-0191` (if fires)
  - Verbose summary: 16 tasks landed; 2 NEW ADRs anchored (ADR-0188 + ADR-0189) + CONDITIONAL ADR-0190 if R6 fires; 28th fuzzer FuzzLuaConfigParse clean; 28/28 differential fixture directories green; all 6 phase-done gates green; SPEC §15.2 24 items all GREEN; 99 → 102 stat count; 16 → 17 HTTP filter count; 27 → 28 fuzzer count; D1 closure evidence + D3 PLAN-session closure recorded.

- [ ] **Step 11: Update ROADMAP.md row 22.1** — status flips `in-progress → done`; per-cell IMPL-done annotation appended per ADR-0106 documenting the 16-task IMPL landing + 6-gate outputs + the FIFTEENTH §9 family-row milestone + the FIRST §9 row to introduce a third-party Lua VM dependency (gopher-lua v1.1.2) + the NEW `internal/lua/` framework primitive milestone + the SPEC §15.2 24-item acceptance + the D1/D3/D5/D7 disposition record. Parent row `22` STAYS `in-progress`. Sub-rows `22.2` + `22.3` UNCHANGED `planned`.

- [ ] **Step 12: Author REVIEW.md** per `superpowers:requesting-code-review` — ~300-400 LoC reviewer artifact covering: the 6-gate outputs verbatim; the 22.1 SPEC §15.2 24-item checklist verification with cite-to-PROGRESS-entry per item; the D3 + D-P1..D-P10 PLAN-time decision-disposition record (which decisions HELD, which were AMENDED at IMPL); the D1 closure evidence from Task 2; the R6 disposition from Task 12 (STANDS WEAK-default OR ADR-0190 escape-valve FIRED); the next-phase handoff state (22.2 BRAINSTORM scope hand-off).

- [ ] **Step 13: Append final PROGRESS.md Task 16 entry** with all 6 gate outputs verbatim + the 22.1 SPEC §15.2 24-item closure checklist + the D-decision disposition status.

- [ ] **Step 14: Verify nothing left uncommitted**

```bash
git status --porcelain
# Expect: empty
```

- [ ] **Step 15: Commit (Task 16 final IMPL-worktree commit)**

```bash
git add docs/envoy-go/BEHAVIOR_CONTRACT.md \
        docs/envoy-go/DECISIONS.md \
        docs/envoy-go/STATE.md \
        docs/envoy-go/ROADMAP.md \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md \
        docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/REVIEW.md
git commit -m "phase 22.1 Task 16: atomic landing + 6-gate phase-done verification

All 6 phase-done gates GREEN: A build / B vet+lint / C race / D differential
(28/28 fixture directories incl. new 0026 — 6 scenarios cross-side
byte-exact + scenario (g) substring-match via BootRejectFixture) /
E fuzz (28 fuzzers clean; 28th FuzzLuaConfigParse confirmed per 22.1 SPEC
§11.1 D5 closure) / F h2spec 53/53 PASS.

22.1 SPEC §15.2 24-item acceptance checklist all GREEN. D1 closure
recorded at Task 2 PROGRESS entry (anticipated PARSE-REJECT both arms
5 + 17 HELD/REFUTED against upstream Envoy v1.37.2 scrape). D3 closure
recorded at this PLAN session (option (a) stat-counter executions delta
LOCKED). D5 + D7 closure recorded at 22.1 SPEC §11. D-P10 R6 gate:
<STANDS WEAK-default | ADR-0190 escape-valve FIRED with §Decision +
§Consequences body landed at this commit>.

BEHAVIOR_CONTRACT.md 7-edit bundle landed atomically per ADR-0052 +
parent §14 + this 22.1 SPEC §14. ADR-0188 §Decision + §Consequences
body anchored (NEW internal/lua/ framework primitive — gopher-lua v1.1.2
VM lifecycle + per-stream LState + per-script-source Chunk cache +
SandboxConfig zero-value StrictUpstreamParity posture per AMEND-1 +
EXPLICIT API-REVISION ALLOWANCE for consumer #2 per BRAINSTORM Q4).
ADR-0189 §Decision + §Consequences body anchored (NEW
internal/filter/http/lua/ package shape — 8 production + 5 test files;
compiledConfig + 3-counter filterStats; 18-arm PARSE-REJECT roster;
4-arm DataSource; pragmatic-middle bridge 21 entries; full byte-pin
:respond per parent §11.6.7 + AMEND-7 + AMEND-8 encode-side reject;
__pairs alphabetical-snapshot per §11 D7; fixture-0026 disposition +
NEW BootRejectFixture per §13-R1; envoy-go-side 'script load error: '
wording-pinning per §13-W).

FIFTEENTH §9 family-row landed. FIRST §9 row to introduce third-party
Lua VM dependency (gopher-lua v1.1.2; pure-Go Lua 5.1; MIT; no CGO).
ENDS the phase-21 ZERO-NEW-framework-primitive streak by introducing
internal/lua/ at first consumer. ROADMAP row 22.1 flipped in-progress
-> done; parent row 22 STAYS in-progress until 22.3 phase-done; sub-rows
22.2 + 22.3 UNCHANGED planned. STATE.md re-advanced to post-22.1-IMPL
state. REVIEW.md authored per superpowers:requesting-code-review."
```

---

## Phase-done squash-merge + push to origin

After Task 16 completes:

1. **Squash-merge to master** (from the master worktree):

```bash
cd /home/esa/git/envoy-go  # the master worktree
git merge --squash phase-22.1-http-filter-lua-vm-and-headers-bridge-impl
# Resolve commit message — body must include the 16-task summary + the 2-NEW-ADR
# (+ CONDITIONAL ADR-0190 if R6 fired) roster + the closes-row-22.1 + FIFTEENTH-§9-row
# + FIRST-third-party-Lua-VM-dep milestone + the parent-row-22-STAYS-in-progress note.
git commit -m "$(cat <<'EOF'
Squash merge phase-22.1-http-filter-lua-vm-and-headers-bridge-impl

Closes ROADMAP row 22.1 (in-progress → done) — FIFTEENTH §9 family-row
foundational third (parent row 22 STAYS in-progress until 22.3 phase-done;
sub-rows 22.2 + 22.3 UNCHANGED planned per ADR-0106 sub-row rollup
discipline + phase-18.1/18.2 + phase-19.1/19.2 precedent).

16 tasks landed. 2 NEW ADRs anchored (ADR-0188 NEW internal/lua/
framework primitive — gopher-lua v1.1.2 VM lifecycle + per-stream LState
+ per-script-source Chunk cache + StrictUpstreamParity sandbox per AMEND-1
+ EXPLICIT API-REVISION ALLOWANCE for consumer #2 per BRAINSTORM Q4;
ADR-0189 NEW internal/filter/http/lua/ package shape — 8 prod + 5 test
files; compiledConfig + 3-counter filterStats; 18-arm PARSE-REJECT;
4-arm DataSource; pragmatic-middle bridge 21 entries; full byte-pin
:respond per parent §11.6.7 + AMEND-7 + AMEND-8; __pairs alphabetical-
snapshot per §11 D7; fixture-0026 + NEW BootRejectFixture per §13-R1;
envoy-go-side 'script load error: ' wording-pinning per §13-W).
<+ CONDITIONAL ADR-0190 if D-P10 R6 fired: per-script-source *LState pool
with chunk-pre-loaded entries — only if Task 12 benchmark surfaced > 1ms
per-stream construction cost>.

28th fuzzer FuzzLuaConfigParse clean at 30s. 28/28 differential fixture
directories GREEN (0000-0026; 6 scenarios cross-side byte-exact for
fixture-0026 + scenario (g) substring-match via NEW BootRejectFixture).
All 6 phase-done gates GREEN. 22.1 SPEC §15.2 24-item acceptance
checklist all GREEN.

FIRST §9 row to introduce third-party Lua VM dependency (gopher-lua
v1.1.2; pure-Go Lua 5.1; MIT; no CGO; matches upstream LuaJIT 5.1
dialect). ENDS phase-21 ZERO-NEW-framework-primitive streak — first §9
row since phase 17 jwt_authn to introduce NEW framework primitive of
substantial scope. 17 HTTP filters wired post-22.1 (was 16). Stat surface
99 → 102 names per AMEND-3 (errors + executions upstream-parity per
AMEND-3 + respond_calls envoy-go-strict per AMEND-3 corrected; HCM-rooted
template http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat> per
AMEND-2).

Three envoy-go-strict departures documented per parent §14 + this 22.1
SPEC §14 edits 3 + 4 + 5: stdlib-sandbox-strict per AMEND-1 (per-stream
goroutine dispatch model cannot make per-worker-VM-scoping assumption);
respond_calls counter per AMEND-3 (operator-visibility for :respond
short-circuit rate); runtime-error log-message wording per AMEND-9
(gopher-lua's [string "chunk"]:line: msg format diverges from LuaJIT's
chunk:line: msg).

D1 closed at IMPL Task 2 first action (upstream Envoy v1.37.2 scrape
against config.cc::createFilterFactoryFromProtoTyped + lua_filter.cc::
Filter constructor; arms 5 + 17 disposition resolved). D3 closed at
22.1 PLAN session (locked at parent §11.7.7 RECOMMENDED option (a) —
lua.<prefix>.executions stat-counter delta IS the 'Lua ran' assertion
via existing /stats admin scrape mechanism per fixture-0025 inline
scrape precedent). D5 + D7 closed at 22.1 SPEC §11 (28th-fuzzer count
CONFIRMED at 27 → 28; envoy-go headers-map EMPIRICALLY net/http.Header
unordered; bridge __pairs RATIFIED alphabetical-snapshot).

§13-R1 (BootRejectFixture infra), §13-R2 (:respond byte-pin), §13-R3
(REFINED __pairs alphabetical-snapshot), §13-R4 (28th fuzzer), §13-W
(envoy-go-side wording-pinning) — ALL CLOSED. §13-R5 (httpclient/
co-consumer) settles at 22.2. §13-R6 (LState-pool benchmark) <STANDS
WEAK-default | ADR-0190 escape-valve FIRED>.
EOF
)"
```

2. **SHA-fill follow-up** (per the phase-09..21 convention):

```bash
# Update STATE.md last-commit field with the real squash SHA (was TBD at Task 16):
# Edit docs/envoy-go/STATE.md replacing "<TBD — SHA-fill follow-up after squash-merge>"
# with the actual squash commit SHA from `git log -1 --format=%H master`.
git add docs/envoy-go/STATE.md
git commit -m "phase 22.1 IMPL follow-up: STATE.md SHA-fill (TBD → <squash SHA> post-squash)"
```

3. **Push to origin** (per project memory `feedback_push_to_origin.md` — always-push-to-origin without asking):

```bash
git push origin master
```

4. **Worktree cleanup** (optional but tidy):

```bash
git worktree remove /home/esa/git/envoy-go/.worktrees/phase-22.1-http-filter-lua-vm-and-headers-bridge-impl
# Keep the branch alive for reference; do NOT delete unless cleanup is explicit
```

---

## Remember

- Exact file paths always.
- Complete code shapes are in the 22.1 SPEC §3.1 + §3.5 + §4 + §6 references — the PLAN points to SPEC §6 rather than reproducing the full code (per the SPEC-vs-PLAN division of labor); the per-Task File-structure table rows + per-Task Step bodies above describe the IMPL surface in implementer-actionable detail.
- Exact commands with expected output for each Step.
- Reference relevant skills with @ syntax where applicable: `@superpowers:subagent-driven-development` (recommended IMPL execution per project memory `feedback_execution_style.md`), `@superpowers:executing-plans` (alternative inline), `@superpowers:systematic-debugging` (when race-test flakes surface at Task 12), `@superpowers:test-driven-development` (every code task is Write-failing-test → Run-FAIL → Implement → Run-PASS → Commit per D-P4), `@superpowers:requesting-code-review` (Task 16 REVIEW.md), `@superpowers:verification-before-completion` (the 6 phase-done gates at Task 16 + per-Task PROGRESS.md entry quoted command outputs per D-P3).
- DRY, YAGNI, TDD, frequent commits.
