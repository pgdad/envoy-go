# Phase 22.2 — HTTP filter `envoy.filters.http.lua` (full Envoy↔Lua bridge surface delta + NEW `internal/dynamicmetadata/` + `internal/lua/` 22.2 API extensions + IN-PLACE AMEND `internal/httpclient/`) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended per project memory `feedback_execution_style.md`) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the FULL Envoy↔Lua bridge surface delta on top of 22.1's pragmatic-middle scaffold — taking parent BRAINSTORM Q1 envelope D to its conclusion — by extending the 22.1 `internal/lua/` framework primitive with a coroutine yield/resume API + body-bridge buffer seam (NEW ADR-0191; ADR-0188's API-REVISION ALLOWANCE STAYS scoped to consumer-#2 per Q10 strict scope); landing a NEW `internal/dynamicmetadata/` framework primitive (ADR-0190) that breaks the cross-phase dynamic-metadata deferral discipline at first co-consumer; IN-PLACE AMENDING ADR-0177's `internal/httpclient/` with `Client.ClusterDispatch` for cluster-based `:httpCall()`; landing the NEW `internal/filter/http/lua/` 22.2 package shape extensions (ADR-0192) covering 8 bridge surface families (body / trailers / metadata / connection-SSL / httpCall / crypto / fileBytes+timestamp / streamInfo-full) + IN-PACKAGE `:filterState()` (per Q9 EXTRACT-NOW-only-when-trigger-fires) + 5 NEW envoy-go-strict counters + 3 NEW runtime-rejection arms 20-22 + 11 envoy-go-strict departure records at BEHAVIOR_CONTRACT.md (5 counter + 2 :filterState + 4 D8-classified crypto/fileBytes — `:sha256` + `:sha512` + `:base64Decode` + `:fileBytes` envoy-go-strict per D8 PLAN-scrape closure); extending `FilterChain` with a `tlsConnectionState *tls.ConnectionState` field (lives inside ADR-0192 per Q13 WEAK HOLD); shipping a single mixed-mode differential fixture `0027-http-lua-full-bridge` with ~9-10 deterministic cross-side scenarios + ~3-4 REFERENCE-LESS subject-only scenarios; adding 1-2 NEW project-wide fuzzers (`FuzzLuaBodyBridge` + optionally `FuzzLuaHTTPCallConfig`); and closing the §11 D3 + D5 + D8 SPEC-time carry-forward D-questions at this PLAN commit with empirical-scrape evidence. **22.2's phase-done commit closes ONLY row `22.2`; the parent row `22` stays `in-progress` until 22.3's phase-done per parent SPEC §1 closure pattern + ADR-0106 per-cell rollup discipline + the phase-18.1/18.2 + phase-19.1/19.2 sub-phase precedent.**

**Architecture:** The 22.2 IMPL extends the existing 07.1 HTTP filter framework with **THREE NEW ADR-anchored framework deltas** (ADR-0190 NEW `internal/dynamicmetadata/` primitive; ADR-0191 NEW `internal/lua/` 22.2 API extensions for coroutine yield/resume + body-bridge buffer seam at consumer-#1 scope-expansion; ADR-0192 NEW `internal/filter/http/lua/` 22.2 package shape extensions) + **ONE IN-PLACE §Decision AMENDMENT** (ADR-0177 `Client.ClusterDispatch` cluster-based dispatch with `FactoryCtx.ClusterManager` threading paralleling existing `FactoryCtx.HTTPClient`) + reuses **SIX existing framework primitives load-bearing** (phase-04/05 HTTP framework; phase-07.1 filter framework + chain.go + callbacks.go field extensions per ADR-0144 pattern; phase-13 ADR-0128 decode-side body buffer wrapped by ADR-0191 BodyBuffer interface; phase-20 ADR-0177 `internal/httpclient/` for cluster dispatch; phase-22.1 ADR-0188 `internal/lua/` VM primitive + `__pairs` `installPairsShim` discipline + StrictUpstreamParity sandbox; phase-22.1 ADR-0189 `internal/filter/http/lua/` package shape extended in-place). NEW production files: `internal/dynamicmetadata/{doc.go, dynamicmetadata.go}` + `internal/lua/{coroutine.go, body_buffer.go}` (per D-P5 LOCK at NEW FILES — not in-place APPEND to vm.go) + `internal/filter/http/lua/{body.go, trailers.go, metadata.go, connection.go, ssl.go, httpcall.go, crypto.go, misc.go, filterstate.go}`. EXTEND in-place: `internal/httpclient/httpclient.go` (`ClusterDispatch` + new struct field) + `internal/filter/http/factory.go` (`FactoryCtx.ClusterManager`) + `internal/filter/http/chain.go` (NEW `tlsConnectionState` + `dynamicMetadata` fields + setters) + `internal/filter/http/callbacks.go` (NEW `DownstreamTLSConnectionState()` + `DynamicMetadata()` accessors on both `DecoderFilterCallbacks` + `EncoderFilterCallbacks`) + `internal/filter/hcm/connection.go` (H1 seeds `tlsConnectionState` before `RunDecodeHeaders` symmetric to existing TLS-principals plumbing) + `internal/filter/hcm/h2dispatch.go` (H2 symmetric per ADR-0144 plumbing pattern) + `internal/filter/http/lua/{lua.go, compiled_config.go, bridge.go, streaminfo.go, stats.go, lua_test.go, compiled_config_test.go, bridge_test.go, fuzz_test.go}`. Tests: 7 NEW `*_test.go` files paralleling production files. Single NEW fixture directory `test/fixtures/0027-http-lua-full-bridge/` consuming 22.1's `BackendKind=HTTPLua` + `scripts/` subdirectory pattern (28 → 29 fixture directories) — with the optional NEW `RunSubjectOnlyHTTPLua` driver-helper for REFERENCE-LESS scenarios per §13-R11. **The phase-22.2 SPEC anchored 3 ADR §Context drafts at the SPEC commit (ADR-0190 + ADR-0191 + ADR-0192) per ADR-0044; the 22.2 IMPL lands all 3 §Decision + §Consequences bodies + the ADR-0177 IN-PLACE AMENDMENT body at the final atomic-landing Task per ADR-0044 in-place edit discipline. The conditional ADR-0193 fires ONLY if §13-R6 *LState-pool gate trips (per-stream `*lua.LState` construction at FULL bridge surface exceeds 1ms) OR §13-R9 body-buffer-seam-with-ADR-0128 separation surfaces enough complexity to warrant ADR split from ADR-0192; otherwise next-free ADR-0193 carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot.** Per-stream `*LState` lifecycle at FULL bridge surface = 1 parent `*LState` (script owner; lives for filter-config lifetime via compiled `*Chunk`) + 1 child `*LState` per phase invocation (the coroutine; cheap to construct via `LState.NewThread()`; shares `G`+`Env` with parent; released at stream destroy via the matching `context.CancelFunc` from `NewThread()`). Body-buffer discipline at endStream: defensive copy `lua.LString(string(f.decodedBodyBytes))` (D3 closure at this PLAN session per SPEC §11.3 + §12 RECOMMENDED option (a) — Lua owns the Go string across coroutine yield/resume + HCM dispatch goroutine lifetimes; perf-benchmark task validates ≤1ms sub-MB + ≤100ms 16-MiB-cap-saturated thresholds). Connection-SSL fixture-cert cross-side topology: cert-fingerprint-only (D5 closure at this PLAN session per SPEC §11.5 + §8.3 + §12 RECOMMENDED option (f-B) — script extracts only `:sha256PeerCertificateDigest()`; cross-side asserts hex digest byte-exact via `CompareBytes`; other 11 cert methods exercised in REFERENCE-LESS subject-only scenarios). Crypto + fileBytes upstream-exposure classification: D8 closure at this PLAN session per SPEC §12 + §13-R7 + §13-R8 + AMEND-22.2-2 — targeted re-scrape against upstream Envoy v1.37.2 `PublicKeyWrapper` + `CryptoUtility` + script-global helper registration (`source/extensions/filters/common/lua/{lua.h,lua.cc,wrappers.h,wrappers.cc}` + `source/extensions/filters/http/lua/{lua_filter.h,lua_filter.cc}`); outcome ratifies BEHAVIOR_CONTRACT.md departure-record bundle scale at 11 records (7 baseline = 5 counter + 2 :filterState; 4 D8-classified per PLAN scrape; SPEC §14 conditional placeholder 0-6 expanded to 4). PARSE-REJECT roster STAYS at 19 arms from 22.1 IMPL UNCHANGED at config-load; 3 NEW RUNTIME-REJECT arms 20-22 at runtime via `luaL_error` (`httpcall-cluster-name-required` + `body-size-cap-exceeded` + `crypto-key-format-invalid`) with byte-stable wording pinned per W2.

**Tech Stack:** Go 1.26.2; `go-control-plane` v1.32.4 module (proto pin per ADR-0008; consumes `envoy/extensions/filters/http/lua/v3.Lua` 4-field schema UNCHANGED from 22.1 — no NEW proto fields consumed at 22.2; `:metadata()` per-route source UNCHANGED at v1.32.4 binding-gap per parent AMEND-12 + §11.6 D1 closure); `github.com/yuin/gopher-lua` v1.1.2 (DIRECT module dep from 22.1 per ADR-0188 §Decision; 22.2 EXTENDS API surface — NewThread + Resume + YieldFromBridge per `state.go:1614,2157,2217` + `vm.go:200-210,267-295` + `coroutinelib.go:1-110` empirically pinned at SPEC §11.1 D2 closure); stdlib `crypto/sha256` + `crypto/sha512` + `crypto/x509` + `encoding/base64` (for crypto bridge methods per §3.4); stdlib `crypto/tls` (`*tls.ConnectionState` for connection-SSL bridge; cert wrapper sources from existing phase-03 + ADR-0144 plumbing); stdlib `time` (`time.Now()` for `:timestamp()` non-deterministic wall-clock); stdlib `os` (`os.ReadFile` + `io.LimitReader` for `:fileBytes()` with 16 MiB cap per 22.1 Task 11 cap pattern); stdlib `context` (for `Client.ClusterDispatch` cancellation + child `*LState` cleanup via `NewThread()`'s `context.CancelFunc`); stdlib `google.golang.org/protobuf/types/known/structpb` (for `internal/dynamicmetadata/Bucket` `*structpb.Value` map values); reference Envoy `envoyproxy/envoy:v1.37.2` SHA `c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd` (ADR-0008 + ENVOY_TARGET.md — unchanged); golangci-lint 1.64.8 (ADR-0009 pin); Docker for the differential harness; HTTP/1.1 plaintext downstream + plaintext upstream backend fixture for body/trailers/metadata/crypto/fileBytes/timestamp/streamInfo/httpCall scenarios + TLS downstream fixture (REUSED from phase-18.x cert fixtures or NEW minimal cert per D5 closure scripting) for connection-SSL scenarios — fixture-0027 scenario (f-B) cert-fingerprint-only per D5 closure. **NO new go.mod direct deps at 22.2** (gopher-lua dep from 22.1 STANDS; crypto/tls/x509/sha256/sha512/base64 are stdlib).

---

## Scope check — why phase 22.2 ships as one sub-phase row (it already is the second-of-two split half; PLAN-time split-gate evaluation CONFIRMS single-row)

Phase 22 was SPLIT at parent SPEC `41ccee7` into `22.1-http-filter-lua-vm-and-headers-bridge` (DONE at `c986419`) + `22.2-http-filter-lua-full-bridge` (THIS PLAN) + `22.3-http-filter-lua-multi-script-and-per-route` (planned) per ADR-0106 sub-row rollup discipline + parent §1 closure pattern. This PLAN is for the 22.2 sub-phase ONLY; 22.3 has its own BRAINSTORM/SPEC/PLAN/IMPL session-set per the phase-18.1/18.2 + phase-19.1/19.2 sub-phase precedent. PLAN re-evaluation of ADR-0045's 25-task / 1500-LoC split-gate at THIS PLAN session per BRAINSTORM Q14 deferral:

**Per-component LoC estimate (mirroring the phase-09..22.1 PLAN component-table convention):**

- NEW `internal/dynamicmetadata/` package (3 prod + 2 test files): ~250-400 LoC
- EXTEND `internal/lua/` (coroutine + BodyBuffer APIs; +2 prod files + +2 test files): ~300-500 LoC
- IN-PLACE AMEND `internal/httpclient/` (+ClusterDispatch method + struct field + test): ~80-150 LoC
- EXTEND `internal/filter/http/factory.go` (+ClusterManager field on FactoryCtx): ~10-20 LoC
- EXTEND `internal/filter/http/chain.go` + `callbacks.go` (+tlsConnectionState field + dynamicMetadata field + 4 accessors): ~80-150 LoC
- EXTEND `internal/filter/hcm/connection.go` (H1 seed): ~30-60 LoC
- EXTEND `internal/filter/hcm/h2dispatch.go` (H2 seed): ~30-60 LoC
- NEW `internal/filter/http/lua/body.go` + `body_test.go`: ~250-400 LoC + ~300-500 LoC
- NEW `internal/filter/http/lua/trailers.go` (+ tests merged into `bridge_test.go`): ~120-200 LoC
- NEW `internal/filter/http/lua/metadata.go` + `metadata_test.go`: ~150-250 LoC + ~200-350 LoC
- NEW `internal/filter/http/lua/connection.go` + `ssl.go` + `ssl_test.go`: ~250-400 LoC + ~300-500 LoC
- NEW `internal/filter/http/lua/httpcall.go` + `httpcall_test.go`: ~250-400 LoC + ~300-500 LoC
- NEW `internal/filter/http/lua/crypto.go` + `crypto_test.go`: ~200-350 LoC + ~250-400 LoC
- NEW `internal/filter/http/lua/misc.go` + `misc_test.go` (`:fileBytes` + `:timestamp`): ~120-200 LoC + ~180-300 LoC
- NEW `internal/filter/http/lua/filterstate.go` + `filterstate_test.go`: ~150-250 LoC + ~200-350 LoC
- EXTEND `internal/filter/http/lua/streaminfo.go` (7 NEW methods on top of 22.1's 4-method subset): ~150-250 LoC
- EXTEND `internal/filter/http/lua/bridge.go` (request_handle + response_handle metatable extensions for 22.2 methods + trailers metatable installs): ~150-250 LoC
- EXTEND `internal/filter/http/lua/stats.go` (+5 NEW counters; 3 → 8): ~50-80 LoC
- EXTEND `internal/filter/http/lua/compiled_config.go` (+3 NEW runtime-reject arms 20-22 + body-size-cap parsing): ~80-130 LoC
- EXTEND `internal/filter/http/lua/lua.go` (+ClusterManager wiring + body-buffer accumulation discipline): ~80-150 LoC
- EXTEND `internal/filter/http/lua/decode_headers.go` + `encode_headers.go` (body-bridge wiring + coroutine yield/resume): ~80-150 LoC each
- NEW `internal/filter/http/lua/fuzz_test.go` 29th + 30th fuzzers (`FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig`) + corpus: ~150-250 LoC + ~30-50 corpus seeds
- EXTEND `internal/filter/http/lua/lua_test.go` (race + concurrency tests + body-bridge benchmark + full-bridge LState construction benchmark per D-P10/R6): ~250-450 LoC
- NEW `test/fixtures/0027-http-lua-full-bridge/{README.md, envoy.yaml, envoy-go.yaml, expectations.yaml, inputs/driver.go, scripts/*.lua}` (~13 scripts; ~9-10 deterministic cross-side + 3-4 REFERENCE-LESS): ~1200-1800 LoC including driver
- POTENTIAL EXTEND `test/differential/{harness.go, runner_test.go}` for `RunSubjectOnlyHTTPLua` driver-helper per §13-R11 (if PLAN chooses NEW vs REUSE): ~50-150 LoC OR 0 LoC (REUSE existing REFERENCE-LESS pattern)
- DECISIONS.md: ADR-0190 + ADR-0191 + ADR-0192 §Decision + §Consequences bodies + ADR-0177 IN-PLACE AMENDMENT body + conditional ADR-0193 (if fires): ~600-1100 LoC
- BEHAVIOR_CONTRACT.md: 15 total edits at 22.2 IMPL (11 envoy-go-strict departure records — 5 counters + 2 :filterState + 4 crypto/fileBytes per D8 PLAN scrape closure below — plus 4 non-record edits: items 1+2 EXTEND-subsection + stat-table + item 10 forward-pointer subsection + item 12 D8 disposition paragraph): ~500-800 LoC. SPEC §14 enumerates items 1-12 with item 11 conditional 0-6 → expanded to exactly 4 records per D8 PLAN closure.
- ROADMAP.md row 22.2 PLAN-done annotation then IMPL-done flip: ~+2 net at IMPL
- STATE.md re-advance at final atomic-landing Task: ~rewrite-in-place
- PROGRESS.md (append-only task log; ~20 task entries): ~1000-1400 LoC
- REVIEW.md (final reviewer artifact per `superpowers:requesting-code-review`): ~400-500 LoC

**Production LoC subtotal (excluding docs/tests/fixtures): ~2400-4000 LoC**. Tests LoC subtotal: ~2500-4000 LoC. Fixture LoC subtotal: ~1200-1800 LoC. Doc LoC subtotal: ~2400-3700 LoC (DECISIONS + BEHAVIOR_CONTRACT + PROGRESS + REVIEW). **Total LoC envelope: ~8500-13500.**

**Task-count estimate (20 tasks at PLAN time):** Pre-Task 0 (PROGRESS.md preamble + 17-precondition verification) + Tasks 1-19 grouped in 6 Tiers (Tier A framework primitives Tasks 1-5; Tier B HCM dispatch wire-in Task 6; Tier C bridge surfaces Tasks 7-13; Tier D stats + runtime-rejects + race + fuzz + bench Tasks 14-16; Tier E differential fixture Tasks 17-18; Tier F atomic landing Task 19). The single Tier F atomic-landing Task is renumbered Task 20 for symmetry with 22.1's Task 16. **20 tasks total — well below ADR-0045's 25-task split-gate.**

**ADR-0045 split-gate evaluation (PLAN-time per BRAINSTORM Q14 deferral):** Task-count = 20 (under 25 threshold). Production LoC envelope ~2400-4000 (over 1500 but the LoC gate is "estimated" and historically every §9 row > phase-13 has exceeded 1500; the operational gate is the task-count). **Decision: STAY SINGLE-PHASE.** No split into 22.2.1 + 22.2.2. Rationale: (i) 22.2 has clear architectural cohesion — it IS the full bridge surface delta per parent BRAINSTORM Q1 envelope D + parent §1 closure pattern; (ii) the three plausible split axes (axis-A stream-lifecycle / axis-B framework-vs-package / axis-C deterministic-vs-non-deterministic) per BRAINSTORM §1.4 ALL surface artificial boundaries — body+coroutine primitive landings depend on `:body()` consumer at first use; framework primitives are CONSUMED at first co-consumer within 22.2 package; fixture-0027 is single mixed-mode and cannot be cleanly split; (iii) 22.1 landed 16 tasks at full execution clean — 20 tasks at 22.2 is incrementally larger but within the same operational shape (4 more tasks for 5 bridge surface families beyond 22.1's headers-only); (iv) the 20-task graph has substantial parallelization (4-way at Tier A Tasks 1+2+3+4; 3-way at Tier A Task 5 with Tasks 1-4; 7-way at Tier C Tasks 7+8+9+10+11+12+13 — file-disjoint bridge surfaces; 3-way at Tier D Tasks 14+15+16 — file-disjoint stats/race/fuzz) which keeps the actual IMPL session wall-time manageable. **Phase 22.2 ships as the single sub-phase row it is — no further nested split.**

---

## File structure (decomposition decisions locked in here)

| File | Status | Responsibility |
|---|---|---|
| `internal/dynamicmetadata/doc.go` | NEW | Package doc per SPEC §3.1 + ADR-0190 §Context — package overview; cross-phase dynamic-metadata deferral-break rationale (phases 16/17/18/19/20 deferred independently — 22.2 lands first co-consumer per BRAINSTORM Q3); per-stream `*Bucket` accessor + map structure rationale; lifecycle integration via `FilterChain.dynamicMetadata` field + `DecoderFilterCallbacks.DynamicMetadata()` + `EncoderFilterCallbacks.DynamicMetadata()` accessors; thread-safety contract (per-stream sequential per ADR-0033; NOT goroutine-safe across streams); nil-bucket tolerance per ADR-0085 (`Get` returns `(nil, false)`; `Set` is no-op; `Snapshot` returns nil; `Reset` no-op); API surface summary; ADR-0190 cross-reference. ~50-80 LoC. Lands at Task 1. |
| `internal/dynamicmetadata/dynamicmetadata.go` | NEW | `Bucket` struct per SPEC §3.1 (unexported `m map[string]map[string]*structpb.Value`); `NewBucket() *Bucket`; `(b *Bucket) Get(filterName, key string) (*structpb.Value, bool)`; `(b *Bucket) Set(filterName, key string, value *structpb.Value)`; `(b *Bucket) Snapshot() map[string]map[string]*structpb.Value` (returns a defensive copy per Q9 — Lua bridge consumes for iteration); `(b *Bucket) Reset()` (clears the map). All methods nil-tolerant per ADR-0085. ~80-130 LoC. Lands at Task 1. |
| `internal/dynamicmetadata/dynamicmetadata_test.go` | NEW | Bucket lifecycle table-driven (NewBucket returns non-nil empty; Get on empty returns `(nil, false)`; Set then Get round-trips; Set overwrites existing; Snapshot returns defensive copy — mutating snapshot does not mutate bucket; Reset clears all entries; nil-bucket tolerance for all 4 methods); `structpb.Value` payload variations (null + number_value + string_value + list_value + struct_value); cross-filter key independence (same key under different filterName remains independent). ~150-250 LoC. Lands at Task 1. |
| `internal/dynamicmetadata/bench_test.go` | NEW | Microbenchmarks for `Bucket.Get` + `Bucket.Set` + `Bucket.Snapshot` under per-stream sequential access; informational only (no perf gate at 22.2 PLAN; PLAN may add gate if Task 15 race+bench surfaces concern). ~50-80 LoC. Lands at Task 1. |
| `internal/lua/coroutine.go` | NEW | Coroutine API EXTENSIONS per SPEC §3.2 + ADR-0191 §Context. **D-P5 LOCKS at NEW FILE (not in-place APPEND to vm.go).** Methods: `(vm *VM) NewThread() (*lua.LState, context.CancelFunc)` — wraps `vm.state.NewThread()` (gopher-lua `state.go:1614`) — returns child `*LState` (shares `G`+`Env` with parent) + matching `context.CancelFunc` for child's ctx-derived loop cleanup at stream destroy. `(vm *VM) Resume(child *lua.LState, fn *lua.LFunction, args ...lua.LValue) (lua.ResumeState, error, []lua.LValue)` — wraps `vm.state.Resume(child, fn, args...)` (gopher-lua `state.go:2157`) — returns `ResumeState` (Resume_OK/Error/Yield), error (panic-wrapped per ADR-0188's panic-wrapper integration), and yielded values. `YieldFromBridge(L *lua.LState, args ...lua.LValue) int` — Go-side helper called from bridge LGFunction body: stashes `*LState` in per-stream pending-map (lookup keyed by filter pointer); returns `L.Yield(args...)` which returns `-1` sentinel; gopher-lua `vm.go:200-210` `callGFunction` sees `-1` and calls `switchToParentThread`. Panic-wrapper integration: NewThread + Resume + YieldFromBridge wrap deferred `recover()` paths into `vm.panicH` callback per ADR-0188 §Decision 2 panic-wrapper discipline. ~150-250 LoC. Lands at Task 2. |
| `internal/lua/body_buffer.go` | NEW | `BodyBuffer` interface per SPEC §3.2 + ADR-0191 §Context: `Bytes() []byte` (defensive-copied per D3 closure at endStream — Lua owns the underlying string across coroutine yield/resume) + `Chunks() [][]byte` (chunked iterator support for `:bodyChunks()`) + `EndStream() bool` (terminal-state predicate). NO concrete implementation in this file (the lua bridge implements consumer-side at `internal/filter/http/lua/body.go`). Doc-comment cross-references ADR-0128's HCM-level decode-side `bodyBuf` accumulation primitive (the upstream supplier of the underlying bytes); ADR-0191 §Decision body codifies the seam contract. ~30-60 LoC. Lands at Task 3. |
| `internal/lua/coroutine_test.go` | NEW | Coroutine API table-driven: NewThread returns non-nil child `*LState` + non-nil CancelFunc; child shares globals with parent (Go-registered global functions visible from child); Resume happy path (no yield; Resume_OK + no error); Resume with yield (Resume_Yield + yielded values from `YieldFromBridge`); Resume-after-yield resumes from where Yield returned (multi-step coroutine); CancelFunc invocation cleans up child without leaks (verified via `runtime.NumGoroutine` delta + child-LState closed assertion); panic in bridge LGFunction during Resume wraps via `vm.panicH` callback; race tests N=100 parallel coroutines under `-race`. Bench `BenchmarkCoroutine_NewThread_PerStream` + `BenchmarkCoroutine_Resume_Yield_RoundTrip` informational. ~250-400 LoC. Lands at Task 2 + Task 15 (concurrency tests merged at race-test task). |
| `internal/lua/body_buffer_test.go` | NEW | BodyBuffer interface contract tests via test-double implementation (`mockBodyBuffer` satisfying the interface for upstream-side test pinning). Tests assert interface signature stability + nil-tolerance of consumers reading `Bytes()` / `Chunks()` / `EndStream()`. ~80-150 LoC. Lands at Task 3. |
| `internal/lua/vm.go` | MODIFY (minimal) | EXTENDS the 22.1 file only at panic-wrapper integration touchpoints (if any). NO API REVISIONS to 22.1's `VM` + `VMOption` + `WithSandboxConfig` + `WithPanicHandler` + `WithBasePrintSink` + `NewVM` + `State` + `RegisterGlobalFunc` + `Run` + `HasGlobalFunc` + `CallGlobal` + `Close` + `PanicHandlerFn` per ADR-0188's EXPLICIT API-REVISION ALLOWANCE clause STAYING scoped to consumer-#2 (per Q10 strict scope). **D-P5 LOCKS at NEW FILES (NOT in-place APPEND).** ALL NEW METHODS `NewThread` + `Resume` + `YieldFromBridge` LIVE IN `coroutine.go` per D-P5; ALL NEW interface declarations LIVE IN `body_buffer.go` per D-P5. Net delta on vm.go: ~+0-30 LoC for any minor panic-wrapper integration touchpoints (e.g., exposing a small unexported helper for coroutine.go to consume via package-internal access). Lands at Task 2. |
| `internal/lua/sandbox.go` | UNCHANGED | The 22.1 sandbox roster is FROZEN at 22.2 phase-done. `AllowCoroutine` zero-value-default `true` (per AMEND-A4 at 22.1) STANDS — gopher-lua's `coroutine.create`/`coroutine.yield`/`coroutine.resume` already callable from script. The NEW `internal/lua/` 22.2 coroutine API at `coroutine.go` is the Go-side bridge-author seam, ORTHOGONAL to the script-side coroutine library (which remains script-author-facing per upstream parity). |
| `internal/lua/compile.go` | UNCHANGED | 22.1's `Chunk` + `CompileCache` + `CompileScript` UNCHANGED at 22.2. Cache nil-tolerance per ADR-0085 carries forward unchanged. |
| `internal/httpclient/httpclient.go` | MODIFY | IN-PLACE AMENDMENT per ADR-0044 in-place edit discipline (per SPEC §3.3 + §11.4). NEW method `(c *Client) ClusterDispatch(ctx context.Context, clusterName string, request *http.Request, clusterMgr *cluster.Manager) (*http.Response, error)`. Body: (a) resolves `clusterName` via `clusterMgr.Get(name)` (returns `errClusterNotFound` on miss); (b) selects endpoint via `Cluster.PickEndpoint()`; (c) rewrites `request.URL.Host` to endpoint's `"host:port"` form; (d) honors per-cluster TLS via `cluster.UpstreamTLSConfig()` — constructs a temporary `*http.Client` with the cluster's TLS config + the receiver's `Options.Timeout`/`Options.RetryPolicy`; (e) returns `(*http.Response, error)` per the existing `Do()` contract; (f) retry loop applies identically (per ADR-0177 §Decision retry-loop discipline STATUS-driven; honors `req.Context().Err()` after every sleep). Thread-safe. NO new ADR number consumed (in-place AMENDMENT body lands in DECISIONS.md against existing ADR-0177 anchor; matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). ~80-130 LoC delta. Lands at Task 4 (method body) + Task 19 atomic landing (DECISIONS.md AMENDMENT paragraph + cross-references). |
| `internal/httpclient/httpclient_test.go` | MODIFY | NEW tests for `ClusterDispatch` — cluster-not-found path; endpoint-resolution success; per-cluster TLS-config honored; cluster-LB endpoint-picker called; timeout + retry honored; context cancellation propagates. Test-double `*cluster.Manager` with canned cluster registrations. ~150-250 LoC delta. Lands at Task 4. |
| `internal/filter/http/factory.go` | MODIFY | NEW field on `FactoryCtx` paralleling existing `HTTPClient` field: `ClusterManager *cluster.Manager`. Threading: HCM-level dispatch in `internal/filter/hcm/connection.go` + `h2dispatch.go` injects the per-server `*cluster.Manager` into each `FactoryCtx` at filter-chain construction time. ~10-20 LoC delta. Lands at Task 4. |
| `internal/filter/http/chain.go` | MODIFY | EXTEND `FilterChain` per SPEC §3 + §3.4 + §11.5 + ADR-0192 §Decision body (no separate ADR for chain-side extension per Q13 WEAK HOLD — lives inside ADR-0192). NEW fields: `tlsConnectionState *tls.ConnectionState` (set-once BEFORE `RunDecodeHeaders` per ADR-0071); `dynamicMetadata *dynamicmetadata.Bucket` (initialized at chain construction via `dynamicmetadata.NewBucket()`; reset at OnDestroy via `dynamicMetadata.Reset()` then nil-out). NEW setters: `SetTLSConnectionState(state *tls.ConnectionState)`; `SetDynamicMetadata(b *dynamicmetadata.Bucket)`. ~80-130 LoC delta. Lands at Task 5. |
| `internal/filter/http/callbacks.go` | MODIFY | EXTEND `DecoderFilterCallbacks` + `EncoderFilterCallbacks` per ADR-0192 §Decision body. NEW accessors on BOTH: `DownstreamTLSConnectionState() *tls.ConnectionState` (returns `c.c.tlsConnectionState` per ADR-0144 plumbing pattern; nil-tolerant); `DynamicMetadata() *dynamicmetadata.Bucket` (returns `c.c.dynamicMetadata`; same bucket shared decode+encode per ADR-0033 per-stream sequential dispatch). ~+30-60 LoC delta. Lands at Task 5. |
| `internal/filter/hcm/connection.go` | MODIFY | H1 dispatch seeds `chain.SetTLSConnectionState(downstreamTLSConnectionState(downstreamConn))` after `chain.SetRequestCtx` + before `chain.RunDecodeHeaders` symmetric to existing TLS-principals plumbing per ADR-0144. The `downstreamTLSConnectionState(net.Conn) *tls.ConnectionState` helper is co-located alongside the existing `extractTLSPrincipals` helper (returns `nil` for plaintext / non-TLS / non-handshake-complete connections). `chain.SetDynamicMetadata(dynamicmetadata.NewBucket())` initialization MAY ALTERNATIVELY land inside `chain.go`'s constructor (D-P-X PLAN-time decision below at Tasks 5+6 dispatch). ~30-60 LoC delta. Lands at Task 6. |
| `internal/filter/hcm/h2dispatch.go` | MODIFY | H2 symmetric per ADR-0144 plumbing pattern: extracts TLS connection state once at connection build time + stores on `h2Dispatcher.tlsConnectionState`; dispatcher copies to each `chainDispatchAction.tlsConnectionState` at `Match` time; `WriteH2` calls `chain.SetTLSConnectionState(c.tlsConnectionState)` after `SetRequestCtx` + before `RunDecodeHeaders`. `chain.SetDynamicMetadata` initialization same as H1 (chain.go constructor if D-P-X locks at chain.go). ~30-60 LoC delta. Lands at Task 6. |
| `internal/filter/hcm/connection_test.go` + `h2dispatch_test.go` | MODIFY | NEW tests for tlsConnectionState seeding (TLS-handshake-complete + plaintext + non-mTLS rows; assert `chain.tlsConnectionState` populated symmetric to existing `tlsPrincipals` tests). ~60-120 LoC delta each. Lands at Task 6. |
| `internal/filter/http/lua/doc.go` | MODIFY | EXTEND 22.1's doc.go with 22.2 surface summary: 8 bridge surface families (body / trailers / metadata / connection-SSL / httpCall / crypto / fileBytes+timestamp / streamInfo-full) + 2 envoy-go-strict scope-expansions (dynamic-metadata pulled forward + filter-state in-package); 4 AMEND-22.2-N catalog cross-references; D1/D2/D3/D4/D5/D6/D7/D8 closure cross-references; ADR-0190 + ADR-0191 + ADR-0192 §Decision body cross-references. ~+50-80 LoC delta. Lands at Task 19 (atomic landing). |
| `internal/filter/http/lua/lua.go` | MODIFY | EXTEND `filter` struct with NEW per-stream state for 22.2: `decodedBodyBytes []byte` (body accumulation buffer for `:body()` defensive-copy at endStream per D3 closure); `pendingBodyResume *lua.LState` (per-stream coroutine yield/resume tracking for body-bridge); `pendingHttpCallResume *lua.LState` (same for `:httpCall()` sync); `bodyChunks [][]byte` (chunk list for `:bodyChunks()` iterator); `compiledConfig.maxBodyBufferedBytes int64` (config-time-resolved cap per arm 21). EXTEND `New` factory with `ClusterManager` wiring (consumed by httpcall bridge): `f.clusterMgr = ctx.ClusterManager` per SPEC §3.3. Per-stream child-LState `context.CancelFunc` tracking for cleanup at OnDestroy. ~+80-150 LoC delta. Lands at Task 19 (atomic landing; refactor of 22.1's `lua.go`). |
| `internal/filter/http/lua/compiled_config.go` | MODIFY | EXTEND 22.1's `compiledConfig` + `buildCompiledConfig` per SPEC §6 +3 runtime-reject arms 20-22 + new config field parsing. NEW field `maxBodyBufferedBytes int64` (default 16 MiB = 16 * 1024 * 1024 per 22.1 Task 11 cap pattern; configurable via NEW `Lua.max_body_buffered_bytes_v1` field IF surfaced at SPEC — otherwise hard-coded constant for 22.2 per BRAINSTORM §1.2 conservative-default + parent SPEC §10 forward-pointer; verify at PLAN against SPEC §6 + §10 — currently NOT a config field at SPEC time so hard-coded constant 16 MiB). The 19-arm PARSE-REJECT roster from 22.1 IMPL STAYS UNCHANGED at config-load; arms 20-22 are RUNTIME-REJECTS at script-invocation time via `luaL_error` (NOT in `buildCompiledConfig`). 22.1 reserved wording constants `parseRejectDefaultSourceCodeRequired` + `parseRejectScriptMissingHooks` carry forward unchanged. ~+30-80 LoC delta. Lands at Task 14. |
| `internal/filter/http/lua/bridge.go` | MODIFY | EXTEND 22.1's bridge.go with 22.2 metatable installs + method dispatch for new bridge surface families. request_handle / response_handle metatable EXTENSIONS: `:body()` + `:bodyChunks()` (dispatched to `body.go`); `:trailers()` (dispatched to `trailers.go`); `:metadata()` (dispatched to `metadata.go`); `:connection()` (dispatched to `connection.go`); `:httpCall()` (dispatched to `httpcall.go`); 6 crypto methods (dispatched to `crypto.go`); `:fileBytes()` + `:timestamp()` (dispatched to `misc.go`); 7 new `:streamInfo()` methods (dispatched to `streaminfo.go`); 1 filter-state accessor `:filterState()` via streamInfo userdata. `__pairs` `installPairsShim` discipline from 22.1 EXTENDS to trailers metatable (mirror headers from 22.1 Task 6 alphabetical-snapshot per §11 D7). PublicKeyWrapper userdata (returned by `:importPublicKey`) is a NEW metatable defined here per D8 sub-closure (mimic upstream `PublicKeyWrapper` userdata return scope at `lua_filter.h:415-427`; expose `:get()` method). ~+150-250 LoC delta cumulative across Tasks 7-13. Lands incrementally per task. |
| `internal/filter/http/lua/streaminfo.go` | NEW | EXTRACTED FROM 22.1's bridge.go (was inline in 22.1; now extracted for clarity at 22.2). 11-method `:streamInfo()` surface at 22.2 phase-done: 4 inherited from 22.1 (`:protocol`/`:routeName`/`:downstreamLocalAddress`/`:downstreamDirectRemoteAddress`) + 7 NEW at 22.2 (`:upstreamHost`/`:upstreamCluster`/`:dynamicMetadata`/`:dynamicTypedMetadata`/`:requestedServerName`/`:filterState`/`:downstreamSslConnection`). `:dynamicMetadata()` + `:dynamicTypedMetadata(filterName)` consume `internal/dynamicmetadata/Bucket` via `cb.DynamicMetadata()`. `:filterState()` dispatches to `filterstate.go` (consumer of per-stream string-keyed map). `:downstreamSslConnection()` returns the ssl userdata from `connection.go` symmetric to `:connection():ssl()`. ~150-250 LoC. Lands at Task 13. |
| `internal/filter/http/lua/body.go` | NEW | Body bridge per SPEC §3.4 + §3.4.1 + §11.3 D3 closure (option (a) defensive copy at endStream). `requestHandleBody(L *lua.LState) int` — body method for request_handle: if `len(f.decodedBodyBytes) > maxBodyBufferedBytes` raises arm-21 runtime-reject; else returns `lua.LString(string(f.decodedBodyBytes))` to Lua. If body not yet fully accumulated (endStream not yet fired), call `internal/lua.YieldFromBridge(L, lua.LNil)` to suspend coroutine — return-from-yield value is the eventual body string. `requestHandleBodyChunks(L *lua.LState) int` — `:bodyChunks()` iterator: returns Lua function-value that on each invocation yields next chunk or nil when exhausted; chunks defensive-copied at endStream similarly. Response-side `:body()`/`:bodyChunks()` mirror via `responseHandle*` symbols. Cumulative body-byte counter increments `stats.bodyBufferedBytesTotal`; coroutine yield counter increments `stats.coroutineYieldsTotal` per yield event. Decode-side body accumulation orchestrated by `DecodeData(buf, endStream)` in `decode_headers.go` (extended at Task 7) — appends to `f.decodedBodyBytes`; at endStream signals body-ready then resumes pending coroutine. ~250-400 LoC. Lands at Task 7. |
| `internal/filter/http/lua/body_test.go` | NEW | Body bridge tests: `:body()` returns full bytes; `:body()` over-cap raises arm-21 runtime-reject byte-stable wording per W2; `:bodyChunks()` iterator yields chunks then nil; body-coroutine yield/resume timing (script that calls `:body()` BEFORE endStream yields; resume on endStream returns the bytes); defensive-copy verification (mutating Go-side `decodedBodyBytes` after `:body()` does NOT change the Lua string); `body_buffered_bytes_total` + `coroutine_yields_total` counter assertions. ~300-500 LoC. Lands at Task 7. |
| `internal/filter/http/lua/trailers.go` | NEW | Trailers bridge mirroring 22.1 headers metatable per SPEC §3.4 + §2.2: 8 mutation methods (`:get`/`:getAtIndex`/`:getNumValues`/`:add`/`:append`/`:remove`/`:replace` + 1 inherited from headers — exactly 8 total; verify roster against 22.1 IMPL during Task 8) + `__pairs` reusing 22.1's `installPairsShim` alphabetical-snapshot. Lazy-available: `request_handle:trailers()` returns nil if no trailers (per Q2 + SPEC §2.2). Implementation: extends `bridge.go`'s trailers userdata constructor; metatable installs at filter construction time. Tests merged into `bridge_test.go`. ~120-200 LoC. Lands at Task 8. |
| `internal/filter/http/lua/metadata.go` | NEW | Metadata bridge per SPEC §3.4 + §11.6 D1 closure: `request_handle:metadata()` returns ALWAYS-CALLABLE EMPTY USERDATA (NEVER nil per `MetadataMapWrapper` upstream pattern) at v1.32.4 binding-gap. `:get(k)` returns `lua.LNil`; `pairs(meta)` yields zero iterations. `request_handle:streamInfo():dynamicMetadata()` consumes `internal/dynamicmetadata/Bucket` for cross-filter dynamic metadata: returns Lua-userdata wrapping the bucket; `:get(filterName, key)` returns proto-Value-to-Lua marshaled or nil; `:set(filterName, key, value)` marshals Lua value back to `*structpb.Value` and stores. `request_handle:streamInfo():dynamicTypedMetadata(filterName)` similar but typed. Helpers in metadata.go for `structpb.Value` ↔ Lua-value marshaling. ~150-250 LoC. Lands at Task 9. |
| `internal/filter/http/lua/metadata_test.go` | NEW | Metadata tests: `:metadata()` returns callable empty userdata (NEVER nil); `:metadata():get(k)` returns nil for any k; `pairs(meta)` yields zero iterations; `:streamInfo():dynamicMetadata():set(filterName, key, value)` round-trips with `:get`; cross-filter key independence; `:dynamicTypedMetadata` returns typed structpb value; nil-bucket tolerance via test-double `DecoderFilterCallbacks` that returns nil from `DynamicMetadata()`. ~200-350 LoC. Lands at Task 9. |
| `internal/filter/http/lua/connection.go` | NEW | `:connection()` userdata + `:ssl()` accessor returning ssl userdata (or nil if `cb.DownstreamTLSConnectionState() == nil` — plaintext). ~80-120 LoC. Lands at Task 10. |
| `internal/filter/http/lua/ssl.go` | NEW | ssl userdata + 12 methods per SPEC §3.4 + BRAINSTORM §2.4: `:subject()` (PEM DN); `:sanLocal()`/`:sanPeer()` (SAN list as Lua table); `:validFrom()`/`:expirationPeer()` (ISO-8601 string); `:sessionId()` (hex); `:ciphersuiteId()`/`:tlsVersion()` (numeric); `:urlEncodedPemEncodedPeerCertificate()`/`:urlEncodedPemEncodedPeerCertificateChain()` (URL-encoded PEM); `:sha256PeerCertificateDigest()` (32-byte hex). Each method extracts from the wrapped `*tls.ConnectionState`. nil-tolerant: returns Lua empty string or nil if connection state absent. ~250-400 LoC. Lands at Task 10. |
| `internal/filter/http/lua/ssl_test.go` | NEW | ssl method tests via test-double `DecoderFilterCallbacks` carrying canned `*tls.ConnectionState`. Cross-side cert-fingerprint-only scenario (f-B per D5 closure): canned cert with fixed SAN list + fixed serial; assert `:sha256PeerCertificateDigest()` returns expected hex digest. Tests for nil-tls path (plaintext) — `:connection():ssl()` returns nil; downstream Lua-side nil-check. ~300-500 LoC. Lands at Task 10. |
| `internal/filter/http/lua/httpcall.go` | NEW | `:httpCall(cluster, headers, body, timeout_ms, asynchronous?)` per SPEC §3.4 + §11.7 D6 closure (PURE FIRE-AND-FORGET on `asynchronous=true`). Body: (a) validates `cluster != ""` else raises arm-20 runtime-reject; (b) constructs `*http.Request` from headers (Lua table → `http.Header`) + body (Lua string → bytes reader); (c) sync path: yields coroutine via `internal/lua.YieldFromBridge`; Go-side `f.clusterMgr.ClusterDispatch(ctx, cluster, request, clusterMgr)` (consumes ADR-0177 AMENDMENT); on response resume coroutine with `(response_headers, response_body)`; on error resume with `(nil, err_string)`; (d) async path: spawns goroutine that calls `ClusterDispatch` + discards response/error per upstream `lua_filter.cc:400-416` `noopCallbacks` singleton; returns 0 values to Lua immediately; NO yield. Stats: `httpcall_total++` on every dispatch (sync + async); `httpcall_failures++` SYNC-ONLY on transport/4xx-5xx error; `httpcall_timeouts++` SYNC-ONLY on timeout; `coroutine_yields_total++` on yield event. ~250-400 LoC. Lands at Task 11. |
| `internal/filter/http/lua/httpcall_test.go` | NEW | httpCall tests: empty cluster raises arm-20 runtime-reject byte-stable wording per W2; sync happy path round-trip via test-double `cluster.Manager`; sync timeout increments `httpcall_timeouts`; sync 5xx response increments `httpcall_failures`; async fire-and-forget returns 0 values + does NOT yield + does NOT increment failures/timeouts even on async transport-failure (per AMEND-22.2-3 D6 closure); coroutine yield/resume timing for sync path (script yields on `:httpCall()` then continues with response); `httpcall_total` counter assertions covering both sync + async dispatches. ~300-500 LoC. Lands at Task 11. |
| `internal/filter/http/lua/crypto.go` | NEW | 6 crypto bridge methods per SPEC §3.4 + D8 closure at this PLAN session: `:base64Escape(s)` (upstream-parity per AMEND-22.2-1; `encoding/base64.StdEncoding`); `:base64Decode(s)` (envoy-go-strict per D8 scrape; `encoding/base64.StdEncoding.DecodeString`); `:sha256(s)` (envoy-go-strict per D8; `crypto/sha256.Sum256` returning hex); `:sha512(s)` (envoy-go-strict per D8; `crypto/sha512.Sum512` returning hex); `:importPublicKey(pem)` (upstream-parity per D8 scrape; parses via `crypto/x509.ParsePKIXPublicKey`; returns `PublicKeyWrapper` userdata mimicking upstream `lua_filter.h:415-427` scope — userdata exposes `:get()` returning the key bytes or nil per D8-sub closure); `:verifySignature(hash_algo, pubkey_wrapper, sig, text)` (upstream-parity per D8 scrape; takes the wrapper as 2nd arg matching upstream calling convention; dispatches hash algo via switch on `hash_algo` string per `lua_filter.cc:611` `crypto_util.verifySignature` call). Invalid PEM raises arm-22 runtime-reject byte-stable wording. ~200-350 LoC. Lands at Task 12. |
| `internal/filter/http/lua/crypto_test.go` | NEW | Crypto tests: `:base64Escape` byte-output upstream-parity (test vectors match `absl::Base64Escape` standard-padding); `:base64Decode` round-trip with `:base64Escape`; `:sha256` + `:sha512` byte-output vectors; `:importPublicKey` parses RSA + ECDSA + Ed25519 PEMs; invalid PEM raises arm-22 byte-stable wording per W2; `PublicKeyWrapper:get()` returns key bytes; `:verifySignature` happy path with `SHA256` algo + canned RSA pubkey/sig/text; signature-verification failure returns false; unsupported hash algo string returns false; calling convention pinned (`request_handle:verifySignature(hash, pubkey_wrapper, sig, text)` — 4 args ordered as upstream). ~250-400 LoC. Lands at Task 12. |
| `internal/filter/http/lua/misc.go` | NEW | `:fileBytes(path)` + `:timestamp(unit?)` per SPEC §3.4 + §2.7. `:fileBytes(path)` opens file via `os.Open` + `io.LimitReader(f, maxFilenameScriptBytes+1)` with 16 MiB cap (matches 22.1 Task 11 cap pattern); reads full content; returns as Lua string; arbitrary paths allowed (envoy-go-strict per D8 scrape — `:fileBytes` NOT in upstream at any scope; documented at BEHAVIOR_CONTRACT.md departure record). `:timestamp(unit?)` returns wall-clock per `time.Now()`; default `unit = 'milliseconds'`; supports `'milliseconds'`/`'microseconds'`/`'seconds'`; invalid unit raises Lua runtime error. ~120-200 LoC. Lands at Task 12. |
| `internal/filter/http/lua/misc_test.go` | NEW | `:fileBytes` tests: happy path via `t.TempDir()` synthetic files; over-cap rejection (write file > 16 MiB; assert size-cap runtime-reject); ENOENT path; EACCES path. `:timestamp` tests: returns monotonic-increasing values across rapid calls; unit conversion (`'seconds'` returns ~1000× less than `'milliseconds'`); invalid unit raises runtime error. ~180-300 LoC. Lands at Task 12. |
| `internal/filter/http/lua/filterstate.go` | NEW | `:filterState()` IN-PACKAGE per SPEC §3.4 + §11.8 D4 closure + AMEND-22.2-4. Per-stream `map[string]any` accessor exposed via streamInfo userdata: `:get(name)` returns marshaled Lua value (string→LString; float64/int64→LNumber; bool→LBool; `map[string]any`→LTable recursive; per AMEND-22.2-4 envoy-go-strict typed marshaling vs upstream's always-string-via-serializeAsString); `:set(name, value)` stores after marshaling (envoy-go-strict per AMEND-22.2-4 — upstream is strictly read-only). Per-stream lifecycle (created at filter struct allocation; destroyed at OnDestroy). 2 envoy-go-strict departure records at BEHAVIOR_CONTRACT.md §14 items 8+9. ~150-250 LoC. Lands at Task 13. |
| `internal/filter/http/lua/filterstate_test.go` | NEW | filterState tests: `:get(name)` returns marshaled typed value per LValue conversion table; `:set(name, value)` round-trips with `:get`; cross-stream isolation (verified by spawning N=10 parallel filter instances; each writes/reads independently; no cross-stream leak); `:set` of invalid Lua type raises runtime error; per-stream lifecycle (filter OnDestroy releases the map). ~200-350 LoC. Lands at Task 13. |
| `internal/filter/http/lua/stats.go` | MODIFY | EXTEND 22.1's 3-counter `filterStats` with 5 NEW envoy-go-strict counters per SPEC §7.1: `httpcallTotal *stats.Counter`; `httpcallFailures *stats.Counter`; `httpcallTimeouts *stats.Counter`; `bodyBufferedBytesTotal *stats.Counter`; `coroutineYieldsTotal *stats.Counter`. EXTEND `newFilterStats` factory to construct all 8 counters under HCM-rooted template `http.<HCM_stat_prefix>.lua.<config_stat_prefix>.<stat>` per parent §7.2 + AMEND-2 (TEMPLATE UNCHANGED from 22.1; 5 NEW stat names append-only). NEW package-level const declarations for 5 stat names. `TestStatNames_Equal_*` table-driven assertion EXTENDS to 8 stat names byte-exact. Empty-stat-prefix consecutive-dot wire-name verification CARRIES FORWARD from 22.1 unchanged. ~+50-80 LoC delta. Lands at Task 14. |
| `internal/filter/http/lua/decode_headers.go` | MODIFY | EXTEND 22.1's `DecodeHeaders` per SPEC §4 + §4.1 + ADR-0192 §Decision body: body-bridge wiring (DecodeData accumulation into `f.decodedBodyBytes`; at endStream signal body-ready + resume pending coroutine via `lua.Resume(child, fn, lua.LString(string(f.decodedBodyBytes)))`); trailers-bridge wiring at terminal-state; coroutine yield/resume orchestration; child-LState cleanup at OnDestroy (invokes the `context.CancelFunc` returned from `vm.NewThread()`). ~80-150 LoC delta. Lands at Task 7 + Task 8. |
| `internal/filter/http/lua/encode_headers.go` | MODIFY | EXTEND 22.1's `EncodeHeaders` symmetric to decode for body+trailers on response side. `:respond()` runtime-reject UNCHANGED from 22.1 AMEND-8 byte-exact `"respond not currently supported in the response path"`. ~80-150 LoC delta. Lands at Task 7 + Task 8. |
| `internal/filter/http/lua/lua_test.go` | MODIFY | EXTEND 22.1's `lua_test.go` with: filter struct field assertions for 22.2 (decodedBodyBytes + pendingBodyResume + bodyChunks + pendingHttpCallResume); 8-counter cardinality + `TestStatNames_Equal_*` extension to 8 names byte-exact; DecodeData accumulation tests; OnDestroy CancelFunc invocation tests; race tests N=100 parallel filter dispatches under `-race` per D-P9 + 22.1 ADR-0188 §Decision 4 concurrency discipline. Benchmark `BenchmarkPerStream_FullBridge_LState_Construction` per D-P10 + §13-R6 (re-measures per-stream `*LState` construction cost at FULL bridge surface including body bridge metatable installs + coroutine NewThread call); threshold gate at `ns/op > 1_000_000` (= 1ms) → ADR-0193 escape-valve fires + signal Task 19 atomic landing. Benchmark `BenchmarkBodyBridge_DefensiveCopy_PerStream` per D3 closure (measures defensive-copy overhead at sub-MB body + 16-MiB-cap-saturated body); threshold gate ≤1ms sub-MB + ≤100ms 16-MiB-saturated. ~+250-450 LoC delta. Lands at Task 15. |
| `internal/filter/http/lua/compiled_config_test.go` | MODIFY | EXTEND 22.1's 19-arm PARSE-REJECT table-driven tests. NO new PARSE-REJECT arms at 22.2 config-load (the 19 from 22.1 STAY UNCHANGED). NEW tests for 22.2's `maxBodyBufferedBytes` parsing default of 16 MiB. ~+30-60 LoC delta. Lands at Task 14. |
| `internal/filter/http/lua/bridge_test.go` | MODIFY | EXTEND 22.1's bridge_test.go with: trailers method tests (mirroring headers from 22.1 Task 6 — 8 methods + `__pairs` alphabetical-snapshot cross-run-determinism); request_handle metatable dispatch verification for all NEW 22.2 methods (assert each dispatch routes to correct module); `PublicKeyWrapper` userdata + `:get` method tests. ~+150-300 LoC delta. Lands at Task 8 (trailers) + Task 12 (PublicKeyWrapper). |
| `internal/filter/http/lua/fuzz_test.go` | MODIFY | EXTEND 22.1's `FuzzLuaConfigParse` with 1-2 NEW fuzzers per SPEC §11.9 D7: `FuzzLuaBodyBridge` (fuzzes body-bridge against gopher-lua's coroutine state machine for panics — must-never-panic per ADR-0018; corpus seeds: small/large/empty bodies + over-cap bodies + scripts that yield/resume in pathological patterns; ~15-20 corpus seeds); `FuzzLuaHTTPCallConfig` (fuzzes httpCall config parameters — cluster name fuzz + headers fuzz + body fuzz + timeout fuzz; must-never-panic at runtime; ~10-15 corpus seeds). Project-wide fuzzer count 28 → 29 or 30 per §13-R10. ~150-250 LoC delta. Lands at Task 16. |
| `cmd/envoy-go/main.go` | MODIFY | UNCHANGED at 22.2 (no new HTTP filter wired — 22.2 is the SAME filter extending bridge surface; `httpReg.Register(lua.TypeURL, lua.New)` call at 22.1's alphabetical position STAYS UNCHANGED). NO new boot-registration. Wiring of `ClusterManager` into `FactoryCtx` happens at chain-construction time via `internal/filter/hcm/` plumbing — NOT at `main.go`. Verify zero delta at Task 19 atomic landing. |
| `test/differential/fixture/fixture.go` | UNCHANGED | `BackendKind=HTTPLua=22` from 22.1 STANDS unchanged. Fixture-0027 reuses this BackendKind. |
| `test/differential/runner_test.go` | MAYBE MODIFY | If §13-R11 PLAN-time decision chooses NEW driver-helper (`RunSubjectOnlyHTTPLua`) over REUSE existing REFERENCE-LESS pattern: new switch-case branch ~+12-20 LoC. If REUSE: zero delta. Lands at Task 17 OR Task 18 per the decision. |
| `test/differential/harness.go` | MAYBE MODIFY | Same disposition as runner_test.go per §13-R11. ~+50-150 LoC delta (if NEW) or 0 (if REUSE). |
| `test/fixtures/0027-http-lua-full-bridge/README.md` | NEW | Top-level fixture-directory README — scope + 13-scenario table per SPEC §8.2 (a)-(m) + topology + cross-refs to parent SPEC §8 + 22.2 SPEC §8 + ADR-0190/0191/0192. ~250-400 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/envoy.yaml` | NEW | Reference Envoy bootstrap. Multi-listener (1 listener per scenario or 2 listeners — plaintext-only + TLS for connection-SSL scenario per D5 closure; per-listener filter chain mounting lua filter with `Filename` arm pointing to `scripts/<scenario>.lua`); templated `{{.BackendPort}}` + cert paths for TLS listener. ~400-600 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/envoy-go.yaml` | NEW | Subject bootstrap; same topology; templated. ~400-600 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/expectations.yaml` | NEW | Human-readable declarative scenario expectations (per-scenario behavior pin; documentation aid; NOT consumed by runner). ~200-350 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/inputs/driver.go` | NEW | Registered Driver impl. Per-scenario probes via `driveProxy` + `emitScenario` + `classifyBody` mirroring fixture-0023+0026's pattern. ~9-10 deterministic cross-side scenarios use `CompareBytes`. ~3-4 REFERENCE-LESS subject-only scenarios use the `RunSubjectOnlyHTTPLua` driver helper (NEW OR REUSE per §13-R11). For scenario (f-B) per D5 closure: probe → assert `:sha256PeerCertificateDigest()` returns expected hex. For scenarios (j)+(k) httpCall: probe → assert script invokes upstream cluster + appropriate response observed (sync) OR fire-and-forget completes without observable response (async). For scenario (l) timestamp: probe → emit `scenario l observed_timestamp_unit=<unit>` reference-less. For scenario (m) filterState: probe → emit `scenario m filter_state_round_trip=ok` reference-less. ~800-1200 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/a_body_whole.lua` | NEW | `:body()` whole-buffer scenario. ~10-15 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/b_body_chunks.lua` | NEW | `:bodyChunks()` iterator scenario. ~10-15 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/c_trailers.lua` | NEW | `:trailers()` add+remove scenario. ~10-15 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/d_metadata_empty.lua` | NEW | `:metadata()` empty-userdata at binding-gap scenario. ~10-15 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/e_dynamic_metadata.lua` | NEW | `:streamInfo():dynamicMetadata()` read+write scenario. ~10-20 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/f_connection_ssl_fp.lua` | NEW | `:connection():ssl():sha256PeerCertificateDigest()` fingerprint-only scenario per D5 closure (f-B). ~10 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/g_crypto.lua` | NEW | `:sha256()` + `:base64Escape()` scenario. ~10 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/h_filebytes.lua` | NEW | `:fileBytes()` on fixed-content file scenario. NOTE per D8 closure: `:fileBytes` is envoy-go-strict — this scenario classifies as REFERENCE-LESS subject-only (reference Envoy v1.37.2 cannot execute `:fileBytes` script — would PARSE-REJECT or runtime-reject). PLAN choice: drop scenario h from cross-side to REFERENCE-LESS subject-only roster per D8 outcome. ~10 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/i_streaminfo_upstream.lua` | NEW | `:streamInfo():upstreamHost()` + `:upstreamCluster()` scenario. ~10 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/j_httpcall_sync.lua` | NEW | `:httpCall(cluster, ..., async=nil)` sync scenario (REFERENCE-LESS subject-only). ~15-20 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/k_httpcall_async.lua` | NEW | `:httpCall(cluster, ..., async=true)` fire-and-forget scenario (REFERENCE-LESS subject-only). ~15-20 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/l_timestamp.lua` | NEW | `:timestamp('milliseconds')` wall-clock scenario (REFERENCE-LESS subject-only). ~10 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/scripts/m_filterstate.lua` | NEW | `:streamInfo():filterState():set` + `:get` scenario (REFERENCE-LESS subject-only). ~15-20 LoC. Lands at Task 18. |
| `test/fixtures/0027-http-lua-full-bridge/certs/` | NEW (subdir) | TLS cert + key for the connection-SSL listener per D5 closure. Reuses existing test cert from `test/fixtures/<prior-fixture-with-tls>/certs/` OR generates new minimal cert (decision at Task 17 via subagent). Symlink or copy; document in README. ~0-100 LoC (depends on whether existing cert reused or new). Lands at Task 17. |
| `go.mod` + `go.sum` | UNCHANGED | NO new direct deps at 22.2 (gopher-lua dep from 22.1 STANDS; crypto/tls/x509/sha256/sha512/base64/structpb are stdlib + already-transitive). |
| `docs/envoy-go/DECISIONS.md` | MODIFY | 3 ADR §Decision + §Consequences body landings at Task 19 atomic landing (ADR-0190 NEW `internal/dynamicmetadata/` framework primitive; ADR-0191 NEW `internal/lua/` 22.2 API extensions; ADR-0192 NEW `internal/filter/http/lua/` 22.2 package shape extensions) + 1 IN-PLACE AMENDMENT body landing at Task 19 on ADR-0177 (`Client.ClusterDispatch` cluster-based dispatch) + CONDITIONAL ADR-0193 §Context + §Decision + §Consequences if §13-R6 *LState-pool gate fires OR §13-R9 body-buffer-seam separation fires per Task 15 + Task 7 outcomes. ~+600-1100 LoC delta. Lands at Task 19. |
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | MODIFY | Task 19 **15 total edits** per SPEC §14 12-slot enumeration with item 11 expanded per D8 PLAN closure. Items 1+2 (EXTEND lua subsection + stat-table 102 → 107) + items 3-7 (5 NEW envoy-go-strict counter departure records — httpcall_total/failures/timeouts + body_buffered_bytes_total + coroutine_yields_total) + items 8-9 (2 NEW envoy-go-strict `:filterState` departure records — `:set` mutation surface + `:get` typed marshaling) + item 10 (NEW `### Phase 22.2 forward-pointer notes` subsection) + item 11 expanded to 4 envoy-go-strict crypto/fileBytes records per D8 PLAN closure (`:sha256` + `:sha512` + `:base64Decode` + `:fileBytes`) + item 12 (D8 disposition paragraph). Total: 1+1+5+2+1+4+1 = 15 edits. **11 of those are envoy-go-strict departure records** at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` 22.2 sub-section (5 counter + 2 :filterState + 4 D8); 4 are non-record subsection-level edits. ~+500-800 LoC delta. Lands at Task 19. |
| `docs/envoy-go/ROADMAP.md` | MODIFY | Row 22.2 PLAN-done annotation appended at THIS PLAN squash-merge commit per ADR-0106 per-cell PLAN-done annotation (status STAYS `in-progress`; annotation extends with §-by-§ PLAN outcomes + D3+D5+D8 closures + 20-task split-or-stay disposition + task-count finalization at 20 + ADR-0190+0191+0192 §Context-draft anchor verification + IN-PLACE AMEND ADR-0177 anticipation + conditional ADR-0193 escape-valve disposition); then flips `in-progress → done` at Task 19 IMPL atomic-landing with IMPL-done annotation. Parent row `22` STAYS `in-progress` (per parent §1 closure pattern + ADR-0106 — closes at 22.3 phase-done). Sub-row `22.3` STAYS `planned`. ~+1 net at IMPL Task 19. |
| `docs/envoy-go/STATE.md` | MODIFY | Rewrite-in-place at PLAN squash-merge follow-up commit per BOOTSTRAP §4.1 invariant 1: `lifecycle-state: phase 22.2 PLAN done; awaiting 22.2 IMPL`; `next-skill: superpowers:executing-plans (or superpowers:subagent-driven-development per project memory feedback_execution_style.md) scoped to 22.2 IMPL`; SHA-fill follow-up commit per the phase-09..22.1 PLAN convention. Then rewrite-in-place again at Task 19 IMPL atomic-landing: `lifecycle-state: phase 22.2 IMPL done; awaiting 22.3 BRAINSTORM`; `next-skill: superpowers:brainstorming` scoped to 22.3; `next-free ADR: ADR-0193` UNCHANGED (if neither R6 nor R9 fires) or `ADR-0194` (if one fires); ADR tail advance to ADR-0192 (or ADR-0193 if escape-valve fires); 102 → 107 stat-count update; 28 → 29 fixture-directory update; 28 → 29-30 fuzzer count update; 17 HTTP filters wired UNCHANGED. Per-task SHA-fill follow-up commit per phase-09..22.1 convention. |
| `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` | NEW | Append-only task log per phase-21 + phase-22.1 IMPL precedent + `superpowers:verification-before-completion` discipline; 20 task entries (Pre-Task 0 preamble + Tasks 1-19); each entry quotes command outputs verbatim per D-P3 below. ~1000-1400 LoC. Authored across all IMPL tasks; preamble at Pre-Task 0. |
| `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/REVIEW.md` | NEW | Task 19 reviewer artifact per `superpowers:requesting-code-review` per phase-21 + phase-22.1 IMPL precedent; per-task review notes + cross-cutting review notes + green-light evidence + 25-item acceptance checklist closure per 22.2 SPEC §15 + D-P1..D-P11 PLAN-time decision-disposition record + D3+D5+D8 closure evidence + R6 + R9 + R7 + R8 + R10 + R11 + W2 disposition record. ~400-500 LoC. Lands at Task 19. |

---

## Planner-time deferred-decision resolution (settles SPEC §12 D3 + D5 + D8 + PLAN-emerged decisions)

The 22.2 SPEC §12 carries 3 D-questions forward to PLAN-time: D3 (body-buffer zero-copy lifetime) + D5 (connection-SSL cross-side fixture-cert-topology) + D8 (crypto + fileBytes upstream-exposure-verification). PLAN session ALSO emerges 11 PLAN-time decisions (D-P1..D-P11) modeled on the phase-22.1 + phase-19.1 + phase-21 PLAN precedent. Each decision is a numbered paragraph below.

**1. D3 (per SPEC §11.3 + §12) — body-buffer zero-copy lifetime LOCKED at option (a) defensive copy at endStream per SPEC RECOMMENDED.** Three options were on the SPEC table: (a) defensive copy at endStream (`lua.LString(string(f.decodedBodyBytes))`); (b) zero-copy via `*lua.LUserData` wrapping with finalizer-based GC notification; (c) defensive copy + bounded streaming via `:bodyChunks()`. **LOCKED at option (a).** Rationale: (i) option (a) is the simplest implementation discipline — Lua owns the resulting string across coroutine yield/resume + HCM dispatch goroutine lifetimes; no Lua-side finalizer plumbing required; (ii) option (a) is GC-safe by construction — the underlying `f.decodedBodyBytes` slice may be freed by Go side after `RunDecodeData(endStream=true)` without affecting the Lua string copy; (iii) option (b) zero-copy requires `*lua.LUserData` wrapping + Lua-side `__gc` metamethod registration + post-OnDestroy use-detection — significant complexity for a hypothetical perf gain that may not materialize; (iv) option (c) bounded streaming is a different surface (the `:bodyChunks()` iterator) that lands alongside option (a) — not in lieu of it. PLAN-time perf-validation: Task 15 schedules `BenchmarkBodyBridge_DefensiveCopy_PerStream` at `internal/filter/http/lua/lua_test.go` measuring per-stream body-bridge construction + defensive-copy overhead at sub-MB body + 16-MiB-cap-saturated body. **Threshold gates:** ≤1ms per sub-MB body; ≤100ms per 16-MiB-cap-saturated body. Below both gates → option (a) STANDS at 22.2 phase-done. Either gate exceeded → ADR-0193 escape-valve FIRES at Task 19 atomic landing per §13-R9 body-buffer-seam-with-ADR-0128 separation disposition + revise to option (b) zero-copy via `*lua.LUserData` wrapping. Settles SPEC §12-D3 + §13-R9 disposition. *Anchored: SPEC §11.3 + §12 RECOMMENDED option (a) + this 22.2 PLAN session per the phase-22.1 D3 closure-at-PLAN-session precedent.*

**2. D5 (per SPEC §11.5 + §8.3 + §12) — connection-SSL cross-side fixture-cert-topology LOCKED at option (f-B) cert-fingerprint-only per SPEC RECOMMENDED.** Three options were on the SPEC table: (f-A) full cert-matching cross-side (operationally complex; ~150-300 LoC of fixture-cert plumbing); (f-B) cert-fingerprint-only cross-side (SPEC RECOMMENDED at §8.3); (f-C) drop scenario (f) to REFERENCE-LESS subject-only (loses envelope-D verification for SSL methods). **LOCKED at option (f-B).** Rationale: (i) option (f-B) requires ONLY one ssl method to be byte-identical across sides — `:sha256PeerCertificateDigest()` returns a 32-byte hex digest of the cert's DER encoding; this is computable byte-deterministically from any cert presented on the TLS handshake; (ii) the OTHER 11 ssl methods (`:subject`/`:sanLocal`/`:sanPeer`/`:validFrom`/`:expirationPeer`/`:sessionId`/`:ciphersuiteId`/`:tlsVersion`/`:urlEncodedPemEncodedPeerCertificate`/`:urlEncodedPemEncodedPeerCertificateChain`/`:downstreamSslConnection`) have implementation-specific formatting differences (ISO-8601 timezone vs UTC; URL-encoded PEM ordering; cipher-suite-ID format) that would force option (f-A) into complex cert-equivalence matching across reference Envoy + envoy-go; (iii) option (f-A) was rejected as the ~150-300 LoC of cross-side cert-matching infra is disproportionate to the single byte-exact assertion it would close + would introduce cert-rotation maintenance burden; (iv) option (f-C) was rejected as it loses ALL cross-side envelope-D verification for SSL methods — the 11 other ssl methods are still exercised in REFERENCE-LESS subject-only scenarios. **Fixture cert scripting per Task 17:** REUSE existing TLS cert + key from phase-18.x or phase-19.x fixture-cert directory (or generate a NEW minimal cert via `openssl req -x509 -newkey rsa:2048 -nodes -days 36500 -subj '/CN=fixture-0027'` if no suitable existing cert reuses cleanly — Task 17 subagent decides at IMPL). Cross-side TLS listener (on both reference Envoy + envoy-go) presents the SAME cert; both call `:sha256PeerCertificateDigest()`; both emit identical 32-byte hex digest into the byte-comparison buffer. Fixture-0027 scenario (f) thereby fires as cross-side `CompareBytes`. Other 11 ssl methods exercised in REFERENCE-LESS subject-only scenarios `f2_subject` + `f3_sanlist` + etc. (PLAN-time decision: collapse to a single REFERENCE-LESS subject-only scenario rather than 11 separate scenarios per scope-economy). Settles SPEC §12-D5 + §8.3 disposition. *Anchored: SPEC §11.5 + §8.3 + §12 RECOMMENDED option (f-B) + this 22.2 PLAN session.*

**3. D8 (per SPEC §12 + §13-R7 + §13-R8 + AMEND-22.2-2) — crypto + fileBytes upstream-exposure-verification CLOSED at PLAN session via empirical scrape against upstream Envoy v1.37.2 source.** PLAN session executed targeted re-scrape of upstream Envoy v1.37.2 source against `source/extensions/filters/http/lua/{lua_filter.h,lua_filter.cc,wrappers.h,wrappers.cc}` + `source/extensions/filters/common/lua/{lua.h,lua.cc,wrappers.h,wrappers.cc}` + GitHub code-search across `envoyproxy/envoy` for method names `luaSha256`/`luaSha512`/`luaBase64Decode`/`luaFileBytes`/`luaImportPublicKey`/`luaVerifySignature`. **Classification table (PLAN-time):**

| Method | Found at | Wrapper / scope | Classification | Departure record needed? |
|---|---|---|---|---|
| `:sha256` | NOT FOUND as Lua binding (appears only as string-arg value at `lua_filter.h:303` for `:verifySignature`) | absent — string-argument value only | **envoy-go-strict** | YES |
| `:sha512` | NOT FOUND as Lua binding (same `lua_filter.h:303` comment) | absent — string-argument value only | **envoy-go-strict** | YES |
| `:base64Decode` | NOT FOUND anywhere in `envoyproxy/envoy` repo | absent | **envoy-go-strict** | YES |
| `:importPublicKey` | `lua_filter.h:315` + `lua_filter.cc:637` + `exportedFunctions()` as `"importPublicKey"` | `StreamHandleWrapper` method; returns `PublicKeyWrapper` userdata per `wrappers.h:415-427` | **upstream-parity** | NO |
| `:verifySignature` | `lua_filter.h:303` + `lua_filter.cc:611` + `exportedFunctions()` as `"verifySignature"` | `StreamHandleWrapper` method | **upstream-parity** | NO |
| `:fileBytes` | NOT FOUND anywhere in `envoyproxy/envoy` repo | absent | **envoy-go-strict** | YES |

**Sub-finding (exposure-scope mimicry):** Upstream `:importPublicKey(pem)` does NOT return a raw key — it returns a **`PublicKeyWrapper` userdata** (defined at `wrappers.h:415-427`) exposing only `:get()` returning the key bytes or nil. The matching `:verifySignature(hash_algo, pubkey_wrapper, sig, text)` takes the wrapper as its 2nd argument (NOT raw key bytes). **D8-sub closure:** envoy-go MIMICS upstream's exposure scope — `crypto.go` returns a `PublicKeyWrapper` Lua userdata with `:get()` method; `:verifySignature` takes the wrapper as 2nd arg; calling convention pinned to match upstream byte-exactly. Anti-departure for the calling convention (avoids a calling-convention departure record). **BEHAVIOR_CONTRACT.md edit-count arithmetic at 22.2 IMPL** (canonical figures used uniformly throughout this PLAN): SPEC §14 enumerates 12 numbered edit slots. After D8 PLAN closure, item 11's conditional placeholder (0-6 records) expands to EXACTLY 4 records (`:sha256` + `:sha512` + `:base64Decode` + `:fileBytes` envoy-go-strict). Departure-record count = items 3-9 (= 5 counter records + 2 :filterState records) + item-11-expansion (= 4 D8 records) = **11 envoy-go-strict departure records** at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` 22.2 sub-section. Total edit count = item 1 + item 2 + 7 departure records (items 3-9) + item 10 + 4 D8 records (item-11 expansion) + item 12 = 1+1+7+1+4+1 = **15 total edits at Task 19 atomic landing**. Updates SPEC §14 + ADR-0192 §Decision body anticipation + `### Phase 22.2 forward-pointer notes` subsection at BEHAVIOR_CONTRACT.md. Fixture-0027 scenario (h) `:fileBytes` falls to REFERENCE-LESS subject-only (reference Envoy cannot run `:fileBytes` — would error at runtime per absent-binding). Settles SPEC §12-D8 + §13-R7 + §13-R8 + AMEND-22.2-2. *Anchored: empirical D8 scrape at this 22.2 PLAN session — gh-raw fetch against `https://raw.githubusercontent.com/envoyproxy/envoy/v1.37.2/source/extensions/filters/http/lua/lua_filter.{h,cc}` + `source/extensions/filters/common/lua/{lua.h,lua.cc,wrappers.h,wrappers.cc}` + GitHub code-search.*

**4. D-P1 — SPEC §6 task-numbering convention LOCKED at fresh 20-task numbering (NOT inherited verbatim from 22.1's 16-task numbering).** Settle: this PLAN's Tasks 1-19 + Pre-Task 0 produce 20 task entries grouped in 6 Tiers (Tier A framework primitives 1-5; Tier B HCM dispatch wire-in 6; Tier C bridge surfaces 7-13; Tier D stats + race + fuzz 14-16; Tier E differential fixture 17-18; Tier F atomic landing 19). The 22.2 SPEC does NOT pre-allocate a §6 task breakdown (parent §6 documented arms 1-22 wording — `:6` was a different topical section); this PLAN session allocates the task graph fresh per phase-21 + phase-22.1 PLAN precedent. *Anchored: this 22.2 PLAN session per the phase-21 + phase-22.1 PLAN's PLAN-time-task-graph-allocation precedent.*

**5. D-P2 — Per-task subagent dispatch type LOCKED at `general-purpose` for code Tasks 1-18; Task 19 atomic landing dispatched via `general-purpose` with explicit 25-item acceptance-checklist reference; REVIEW.md authoring at Task 19 final step dispatched via `superpowers:code-reviewer` per `superpowers:requesting-code-review`.** Settle: per project memory `feedback_execution_style.md` (user always wants subagent-driven over inline execution for plans), each Task's IMPL session subagent-dispatches per `superpowers:subagent-driven-development`. Dispatch type per Task: Tasks 1-18 use `general-purpose` agent (Go code work; no specialized agent type matches more precisely); Task 19 uses `general-purpose` with explicit reference to 22.2 SPEC §15 25-item acceptance checklist + BEHAVIOR_CONTRACT.md 15-edit bundle anatomy + ADR-0190 + ADR-0191 + ADR-0192 §Decision + §Consequences body sketches from SPEC §3.1 + §3.2 + §3.4 + §3.5 + §11 + §13 + §16 + the ADR-0177 IN-PLACE AMENDMENT body shape from SPEC §3.3 + §11.4. *Anchored: project memory `feedback_execution_style.md` + phase-09..22.1 + phase-18.1 + phase-19.1 IMPL precedent + `superpowers:subagent-driven-development` skill.*

**6. D-P3 — Per-task PROGRESS.md entry shape LOCKED per phase-21 + phase-22.1 IMPL precedent (8-section format).** Settle: each Task's PROGRESS.md entry contains the following sections in order:
   - **Task ID + title** (matches this PLAN's Task heading verbatim);
   - **Acceptance criteria** (verbatim cross-reference to this PLAN's Task `Acceptance:` stanza);
   - **Files touched** (the precise list from this PLAN's Task heading's `Files:` block);
   - **Verification command outputs** (the exact commands from this PLAN's Task Step bodies' Run-tests-verify-they-pass phase + the verbatim stdout/stderr quoted in fenced code blocks per `superpowers:verification-before-completion` discipline);
   - **Acceptance-criteria evidence** (per-criterion pass/fail with brief reasoning + cross-reference to the verification command output that demonstrates the pass);
   - **D-decision-disposition update** (if the task closes or refines a D-decision — e.g., Task 15 closes D-P10 R6 disposition; the entry records the empirical evidence + the resolved disposition);
   - **Commit SHA** (`git log -1 --format=%H` for the task's commit);
   - **Tier + Task-number cross-reference** (e.g., "Tier C bridge surfaces (Task 8 of 7-13 in tier; Task 8 of 19 overall + Pre-Task 0)").
   *Anchored: phase-21 + phase-22.1 + phase-18.1 + phase-19.1 PROGRESS.md format precedent + `superpowers:verification-before-completion` discipline + this PLAN's per-Task structure.*

**7. D-P4 — Per-task TDD ordering LOCKED at test-first for ALL 19 code Tasks per `superpowers:test-driven-development` rigid discipline; Task 18 fixture-0027 + Task 19 atomic landing relaxed to test-with-implementation.** Settle: every Task that lands production code at IMPL (Tasks 1-17) follows the rigid TDD ordering: (Step 1) write the failing test in the corresponding `*_test.go` file; (Step 2) run the test to verify it fails (compile-error OR assertion-failure with expected error); (Step 3) implement the minimal production code to make the test pass; (Step 4) run the test to verify it passes; (Step 5) run `go build ./... + go vet ./... + golangci-lint run` clean; (Step 6) append PROGRESS.md Task entry per D-P3; (Step 7) commit. Tasks that land bulk fixture material (Task 18 fixture-0027 directory + 13 scripts + YAML configs + driver) follow a relaxed test-with-implementation discipline (the differential fixture IS the integration test; the per-scenario `.lua` source files + the driver impl land together with the per-scenario probe assertions). Task 19 atomic landing follows the relaxed discipline (the 6-gate verification matrix IS the integration test; the BEHAVIOR_CONTRACT.md + DECISIONS.md + STATE.md + ROADMAP.md + REVIEW.md edits land together at the atomic commit). `superpowers:test-driven-development` is RIGID — adherence is mandatory for Tasks 1-17. *Anchored: `superpowers:test-driven-development` rigid discipline + phase-09..22.1 IMPL precedent.*

**8. D-P5 — `internal/lua/` 22.2 file split LOCKED at NEW `coroutine.go` + NEW `body_buffer.go` (NOT in-place APPEND to `vm.go`).** Settle: the 22.2 `internal/lua/` extensions ship as NEW FILES `coroutine.go` (NewThread + Resume + YieldFromBridge) + `body_buffer.go` (BodyBuffer interface) rather than as in-place APPEND to the 22.1's `vm.go`. Rationale: (i) the NEW FILE shape preserves clean ADR-0188 vs ADR-0191 lineage separation (vm.go = ADR-0188 scope; coroutine.go + body_buffer.go = ADR-0191 scope per Q10 strict scope); (ii) file-disjoint test surfaces (coroutine_test.go + body_buffer_test.go) enable parallel subagent dispatch at Tasks 2 + 3; (iii) future consumer-#2 phase that extends `internal/lua/` per ADR-0188 §Decision 5 ALLOWANCE can author ADR-0188-scoped revisions in vm.go without touching ADR-0191's coroutine.go + body_buffer.go. *Anchored: ADR-0188 §Decision 5 ALLOWANCE + ADR-0191 §Context lineage-separation rationale + this PLAN-time emerge.*

**9. D-P6 — Boot-registration position UNCHANGED at 22.2 (no new HTTP filter wired; ClusterManager threaded into FactoryCtx at HCM dispatch).** Settle: `cmd/envoy-go/main.go` ZERO DELTA at 22.2 — `httpReg.Register(lua.TypeURL, lua.New)` call at 22.1's alphabetical position (between `localratelimit.New` and `oauth2.New`) STAYS UNCHANGED. The NEW `ClusterManager` plumbing on `FactoryCtx` (per SPEC §3.3 + D-P-X below) is wired AT HCM-LEVEL DISPATCH (`internal/filter/hcm/connection.go` + `h2dispatch.go`) — NOT at main.go. The per-server `*cluster.Manager` is already available at HCM construction time (consumed by other filters per ADR-0177 §Decision integration paragraph); 22.2 just threads it into `FactoryCtx` for downstream filter consumers. 17 HTTP filters wired post-22.2 UNCHANGED from post-22.1. *Anchored: SPEC §3.3 + ADR-0177 §Decision integration + this PLAN-time emerge.*

**10. D-P7 — Fuzzer count target LOCKED at 30 (29th + 30th: `FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig`) per SPEC §11.9 D7 + §13-R10.** Settle: 22.2 adds 2 NEW fuzzers (not 1) per SPEC §11.9 D7 closure (29-30 anticipated). Both land at Task 16. **`FuzzLuaBodyBridge`** corpus seeds (~15-20 total): empty body (0 bytes); small body (10-100 bytes); medium body (10 KB-100 KB); large body (1 MB-15 MB); over-cap body (17 MB; should runtime-reject); chunked body (multi-call DecodeData accumulation patterns); script-patterns that yield/resume in pathological orderings (call `:body()` multiple times; call before+after endStream; nested coroutines; mid-coroutine OnDestroy). **`FuzzLuaHTTPCallConfig`** corpus seeds (~10-15 total): empty cluster name (should runtime-reject); valid cluster name + valid headers + valid body + valid timeout; missing-cluster fallthrough; transport-failure simulation; oversized headers; oversized body; invalid timeout values; async-flag variations. Both fuzzers must-never-panic per ADR-0018 + 30s clean. Project-wide fuzzer count post-22.2 = **30** (28 from 22.1 + 2 new at 22.2). *Anchored: SPEC §11.9 D7 + §13-R10 + ADR-0018 + this PLAN-time emerge.*

**11. D-P8 — Task graph parallelization LOCKED per PLAN-time emerge.** Settle: after Pre-Task 0 (PROGRESS.md preamble + 17-precondition verification) lands, the 19-task graph allows parallelization at multiple points:

   - **After Pre-Task 0** (PROGRESS.md preamble): Tasks 1 + 2 + 3 + 4 PARALLEL (4-way). NEW packages + framework-primitive extensions + IN-PLACE AMEND on httpclient are file-disjoint.
   - **After Tasks 1 + 2 + 3 + 4**: Task 5 sequential (depends on Task 1's `dynamicmetadata.Bucket` type for the chain.go field — and on Task 4's `FactoryCtx.ClusterManager` for the field).
   - **After Task 5**: Task 6 sequential (depends on Task 5's chain.go field additions; H1 + H2 plumbing).
   - **After Task 6**: Tasks 7 + 8 + 9 + 10 + 11 + 12 + 13 PARALLEL (7-way) — file-disjoint bridge surfaces:
     - Task 7 (`body.go` body bridge + body-bridge decode wire-in)
     - Task 8 (`trailers.go` trailers bridge + trailers metatable installs in bridge.go)
     - Task 9 (`metadata.go` metadata + dynamic-metadata bridge)
     - Task 10 (`connection.go` + `ssl.go` connection-SSL bridge)
     - Task 11 (`httpcall.go` httpCall bridge)
     - Task 12 (`crypto.go` + `misc.go` crypto + fileBytes + timestamp)
     - Task 13 (`streaminfo.go` extension + `filterstate.go` filter-state)
   - **After Tasks 7-13**: Tasks 14 + 15 + 16 PARALLEL (3-way) — file-disjoint:
     - Task 14 (`stats.go` 5 NEW counters + `compiled_config.go` runtime-reject arms 20-22)
     - Task 15 (race + concurrency tests + 2 benchmarks per D-P10)
     - Task 16 (29th + 30th fuzzer)
   - **After Tasks 14-16**: Tasks 17 + 18 PARALLEL (2-way) — cert fixture plumbing decision + fixture-0027 directory authoring:
     - Task 17 (cert fixture scripting per D5; REUSE existing TLS cert from prior fixture OR generate minimal cert)
     - Task 18 (fixture-0027 directory + 13 scripts + YAMLs + driver + R11 disposition for REFERENCE-LESS driver-helper)
   - **Sequential tail**: Task 19 (atomic landing — BEHAVIOR_CONTRACT.md 15-edit bundle + ADR bodies + STATE.md + ROADMAP).

   **Parallel-dispatch opportunities**: 4-way at Tasks 1+2+3+4; 7-way at Tasks 7+8+9+10+11+12+13; 3-way at Tasks 14+15+16; 2-way at Tasks 17+18.

   **Sequential bottlenecks**: Pre-Task-0 → {1,2,3,4}; {1,2,3,4} → 5; 5 → 6; 6 → {7,8,9,10,11,12,13}; {7,8,9,10,11,12,13} → {14,15,16}; {14,15,16} → {17,18}; {17,18} → 19.

   **Shared-file serialization caveat (load-bearing for Tasks 7-13 7-way claim):** `internal/filter/http/lua/bridge.go` is touched by Tasks 7+8+9+10+11+12+13 (each adds ~5-20 LoC of metatable-dispatch registration lines for its surface methods; Task 12 also adds the PublicKeyWrapper userdata metatable). `decode_headers.go` + `encode_headers.go` are touched by Tasks 7+8 (body + trailers decode/encode wiring at terminal-state). The 7-way parallelization claim therefore holds **for the NEW production files (body.go + trailers.go + metadata.go + connection.go + ssl.go + httpcall.go + crypto.go + misc.go + filterstate.go + streaminfo.go) + their NEW test files** (truly file-disjoint), but the **bridge.go + decode_headers.go + encode_headers.go edits are SERIALIZED via the IMPL session orchestrator's merge protocol** — each parallel subagent commits ONLY its NEW files in parallel; the orchestrator then SEQUENTIALLY applies the small bridge.go / decode_headers.go / encode_headers.go method-dispatch deltas from each Task in a single follow-up coordinated commit per Tier C end (one coordinated commit covering all 7 Tasks' bridge.go entries — wired alphabetically per ADR-0100-equivalent ordering discipline), OR each Task commits its bridge.go delta serially after its NEW-files commit while the next Task's NEW-files subagent runs in parallel. The IMPL session per `superpowers:subagent-driven-development` per project memory `feedback_execution_style.md` exploits the file-disjoint parallelism + serializes shared-file edits via the orchestrator's merge protocol. *Anchored: SPEC §3 framework primitive shapes + §6 PARSE/RUNTIME-REJECT roster + §11 empirical-pin closures + this PLAN-time emerge.*

**12. D-P9 — Cross-package regression-test command shape LOCKED per 22.1 D-P9 precedent with race-scoping carry-forward.** Settle: after each task lands its production code, the implementer runs the package-local test command. Race-scoping per 22.1 REVIEW §3 disposition table — `-race` flag scoped to unit packages per integration-suite port-bind race flakiness (0012/0018/0023). At Task 19 Gate D the full regression `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-7])'` runs all 29 fixture directories (the 28 pre-existing — 0000-0026 — plus the new 0027). Per SPEC §15 expected outcome: zero regression. Per-task gates:
   - Tasks 1-3: `go test -count=1 -race ./internal/dynamicmetadata/... ./internal/lua/...`
   - Task 4: `go test -count=1 -race ./internal/httpclient/...`
   - Tasks 5-6: `go test -count=1 -race ./internal/filter/http/... ./internal/filter/hcm/...`
   - Tasks 7-13: `go test -count=1 -race ./internal/filter/http/lua/...`
   - Tasks 14-16: `go test -count=1 -race ./internal/filter/http/lua/...`
   - Task 17-18: `go test -count=1 ./test/differential -run TestDifferential/0027`
   - Task 19: full `go test -count=1 -race ./...` + `go test -count=1 ./test/differential -run 'Test.*00(0[0-9]|1[0-9]|2[0-7])'` (no race for integration suite per 22.1 D-P9 scoping)
   *Anchored: 22.1 D-P9 precedent + 22.1 REVIEW §3 race-scoping refinement + this PLAN-time emerge.*

**13. D-P10 — `*LState`-pool benchmark RE-EVALUATION at FULL bridge surface (Task 15) per SPEC §13-R6 + 22.1 D-P10 carry-forward; threshold gate `ns/op > 1_000_000` (= 1ms) → ADR-0193 escape-valve fires.** Settle: Task 15 (race + concurrency tests) ALSO includes 2 benchmark sub-tasks at `internal/filter/http/lua/lua_test.go`:
   1. **`BenchmarkPerStream_FullBridge_LState_Construction`** — measures per-stream `*lua.LState` construction cost at the FULL bridge surface (constructs N=10000 fresh VMs back-to-back covering the 22.2 metatable installs — request_handle + response_handle + headers + streamInfo + headersIter + trailers + dynamicMetadata + connection + ssl + httpcall + crypto + misc + filterstate + PublicKeyWrapper — plus the parent+child LState pair via NewThread for coroutine support; reports `ns/op` via `b.N` discipline). Threshold gate per SPEC §13-R6: `ns/op > 1_000_000` (= 1ms). 22.1 baseline at headers-only surface measured `ns/op = 69865` (~70µs/stream); 22.2 FULL bridge surface anticipated 200-500µs/stream (3-7× headers-only); SHOULD STAY UNDER 1ms threshold. If `ns/op > 1_000_000`: ADR-0193 escape-valve FIRES at Task 19; ADR-0193 §Context + §Decision + §Consequences body all land at the same Task 19 commit per ADR-0044 anchoring a "per-script-source `*LState` pool with chunk-pre-loaded entries" decision. If `ns/op <= 1_000_000`: the WEAK-default per-stream construction STANDS; no ADR-0193 fires; next-free ADR-0193 carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot.
   2. **`BenchmarkBodyBridge_DefensiveCopy_PerStream`** (per D3 closure above) — measures defensive-copy overhead at sub-MB body + 16-MiB-cap-saturated body. Threshold gates ≤1ms sub-MB + ≤100ms 16-MiB-saturated. Outcomes feed into the §13-R9 disposition (R9 conditional ADR-0193 only fires if R6 stays under but R9 surfaces enough implementation complexity to warrant separate ADR per body-buffer-seam-with-ADR-0128 separation; this is independently evaluable from R6 — R9 may fire at Task 7 body-bridge IMPL via subagent surfacing complexity).
   Both benchmark results quoted verbatim in Task 15 PROGRESS.md entry per D-P3. *Anchored: SPEC §13-R6 + §13-R9 + 22.1 D-P10 carry-forward + 22.1 REVIEW §3 R6 disposition + this PLAN-time emerge.*

**14. D-P11 — REFERENCE-LESS driver-helper for non-deterministic fixture-0027 scenarios LOCKED at REUSE existing `runReferenceLessFixture` pattern (NOT NEW `RunSubjectOnlyHTTPLua` helper).** Settle per §13-R11: fixture-0027's ~3-4 non-deterministic scenarios (j) httpCall sync + (k) httpCall async + (l) timestamp + (m) filterState + (h) fileBytes (per D8 reclassification) consume the existing `runReferenceLessFixture` driver-helper at `test/differential/runner_test.go:1268`. NO NEW driver-helper added. Rationale: (i) the existing pattern handles "subject-side only; emit scenario verdict into byte-comparison buffer" semantics already (precedent: fixture-0025 inline scrape + fixture-0021 grpc-fixture REFERENCE-LESS pattern); (ii) NEW driver-helper would duplicate ~50-150 LoC without semantic gain; (iii) the fixture-0027 driver.go simply DISPATCHES PER-SCENARIO — calls `CompareBytes` for deterministic scenarios + emits subject-only verdicts for non-deterministic scenarios into the same byte-comparison buffer; the existing `BootRejectFixture` driver-helper from 22.1 is ORTHOGONAL (boot-reject is a different lifecycle phase). Task 17 + Task 18 implementer adopts this disposition. Settles SPEC §13-R11 disposition. *Anchored: SPEC §13-R11 + fixture-0021 + fixture-0023 + fixture-0025 + fixture-0026 REFERENCE-LESS-by-pattern precedent + this PLAN-time emerge.*

---

## ADRs introduced/landed by this plan

The 22.2-landing ADRs anticipated by SPEC §16 (ADR-0190 + ADR-0191 + ADR-0192) — **§Context drafts already at the 22.2 SPEC commit `0d6463e`** (re-anchored via SHA-fill follow-up `7b93465`) per ADR-0044 ADR-on-impl convention; **§Decision + §Consequences land at each ADR's Lands-in-Task at 22.2 IMPL atomic-landing Task 19**. The 1 IN-PLACE §Decision AMENDMENT-anticipation paragraph at ADR-0177 (per SPEC §3.3 + §11.4) anchors at the 22.2 SPEC commit; **AMENDMENT body lands at IMPL Task 19** per ADR-0044. PLAN's hypothesis per D-P10 + D3 PLAN-time gate: **conditional ADR-0193 fires only if §13-R6 *LState-pool gate trips at Task 15 OR §13-R9 body-buffer-seam separation surfaces at Task 7** (PLAN-hypothesis: §13-R6 stays under 1ms — 22.1 baseline was 70µs; 22.2 anticipated 200-500µs; §13-R9 stays embedded in ADR-0192; ADR-0193 stays UNCONSUMED at 22.2 phase-done; next-free ADR-0193 carries forward to 22.3 BRAINSTORM as escape-valve slot). NO ADR-0125 §(xiv) AMENDMENT body at 22.2 IMPL (the AMENDMENT-anticipation paragraph anchored at parent SPEC commit STANDS UNCHANGED; body lands at 22.3 IMPL final Task per the 22.3 sub-phase PLAN).

| ADR | Subject (22.2 portion) | Lands-in-task |
|---|---|---|
| **ADR-0190** | NEW `internal/dynamicmetadata/` framework primitive — per-stream `*Bucket` accessor for cross-filter dynamic-metadata read+write at first co-consumer (HTTP Lua filter 22.2's `:streamInfo():dynamicMetadata()` + `:dynamicTypedMetadata(filter_name)`) per phase-22 BRAINSTORM Q3 cross-phase-deferral-break + Q9 EXTRACT-NOW + 22.2 SPEC §3.1 production signatures + §1.6 cross-phase deferral-lift expectation + ADR-0033 per-stream sequential filter dispatch + ADR-0085 nil-bucket tolerance. THIRD §9 framework primitive in two-phase succession (after ADR-0188 + ADR-0189 at 22.1 IMPL). Cross-phase deferral-lift: phases 16/17/18/19/20's BEHAVIOR_CONTRACT.md "deferred" notes carry forward AS-IS until their respective next-touchpoint phases — lift-phases convert "deferred" to "lifted via `internal/dynamicmetadata`". | Task 19 |
| **ADR-0191** | `internal/lua/` 22.2 API extensions for coroutine yield/resume + body-bridge buffer seam at HTTP filter Lua consumer-#1 scope-expansion per phase-22.2 BRAINSTORM Q1 + Q10 strict scope (NEW ADR not in-place AMEND on ADR-0188 — ADR-0188's EXPLICIT API-REVISION ALLOWANCE clause STAYS scoped to consumer-#2) + 22.2 SPEC §3.2 production signatures + §11.1 D2 closure (gopher-lua native LState.NewThread/Yield/Resume) + §11.3 D3 RECOMMENDED option (a) BodyBuffer interface seam consumed at `internal/filter/http/lua/body.go`. Coroutine API: `NewThread() (*lua.LState, context.CancelFunc)` + `Resume(child, fn, args...) (ResumeState, error, []LValue)` + `YieldFromBridge(L, args...) int`. Body-buffer seam: `BodyBuffer interface { Bytes() []byte; Chunks() [][]byte; EndStream() bool }`. Per-stream child-LState lifecycle: 1 parent + 1 child per phase invocation; child's `context.CancelFunc` invoked at stream destroy. | Task 19 |
| **ADR-0192** | `internal/filter/http/lua/` 22.2 package shape extensions — body + trailers + metadata + connection-SSL + httpCall + crypto + fileBytes + timestamp + streamInfo-full + filter-state in-package bridge methods + 5 NEW envoy-go-strict stat counters + 2 NEW envoy-go-strict `:filterState()` divergences (per AMEND-22.2-4) + 4 NEW envoy-go-strict crypto/fileBytes departure records (per D8 closure at this PLAN session — `:sha256`/`:sha512`/`:base64Decode`/`:fileBytes` envoy-go-strict; `:importPublicKey`/`:verifySignature` upstream-parity with calling-convention mimicry) + cross-phase dynamic-metadata deferral-lift expectation (consumer-#1 of ADR-0190) + fixture-0027 mixed-mode discipline + NEW `FilterChain.tlsConnectionState *tls.ConnectionState` field extension (lives inside this ADR per Q13 WEAK HOLD; no separate ADR for chain-side extension) + 3 NEW runtime-reject arms 20-22 byte-stable wording per W2. PARSE-REJECT roster STAYS at 19 from 22.1 IMPL UNCHANGED at config-load. Stat surface 102 → 107 (+5). Fixture directory 28 → 29. Fuzzer count 28 → 30 (+2 per D-P7). | Task 19 |

### IN-PLACE §Decision AMENDMENT (per ADR-0044)

| ADR | AMENDMENT scope | Lands-in-task |
|---|---|---|
| **ADR-0177** | §Decision body gains AMENDMENT paragraph (anticipated at 22.2 SPEC §3.3 + §11.4) documenting NEW method `Client.ClusterDispatch(ctx, clusterName, request, clusterMgr) (*http.Response, error)` for cluster-based dispatch via cluster manager LB + per-cluster TLS + retry inheritance. NEW `FactoryCtx.ClusterManager` field paralleling existing `FactoryCtx.HTTPClient`. R5 RATIFIED-PENDING (parent SPEC §13 + 22.2 SPEC §13-R5) — first co-consumer validation of phase-20's `internal/httpclient/` primitive at 22.2's `:httpCall()` bridge per ADR-0177 §Consequences forward-pointer. NO new ADR number consumed (matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). | Task 19 |

### CONDITIONAL ADR landing (only if §13-R6 *LState-pool gate trips at Task 15 OR §13-R9 body-buffer-seam separation surfaces at Task 7)

| ADR | AMENDMENT scope | Lands-in-task |
|---|---|---|
| **ADR-0193** (CONDITIONAL) | Per-script-source `*LState` pool with chunk-pre-loaded entries (if §13-R6 fires) OR body-bridge buffer seam separation from ADR-0192 with its own §Decision body (if §13-R9 fires). §Context + §Decision + §Consequences body all land at the same Task 19 commit per ADR-0044. If both R6 and R9 stay quiescent: next-free ADR-0193 carries forward to 22.3 BRAINSTORM as the 22.3 IMPL escape-valve slot. | Task 19 (CONDITIONAL) |

> The implementer at Task 19 AUTHORS the 3 ADR §Decision + §Consequences bodies in DECISIONS.md (the §Context drafts are already at the 22.2 SPEC commit per ADR-0044), authors the IN-PLACE AMENDMENT body on ADR-0177, includes the ADRs in the Task 19 commit message, and verifies via `grep -nE '^## ADR-0190' docs/envoy-go/DECISIONS.md` returning the expected single match (similarly for ADR-0191 + ADR-0192; ADR-0177 returns 1 match unchanged but with AMENDMENT paragraph appended). If R6 or R9 escape-valve fires per Task 15 + Task 7, ADR-0193 §Context + §Decision + §Consequences body also land at the same commit.

> **NO in-place ADR-0125 §(xiv) AMENDMENT body at 22.2 IMPL** — the AMENDMENT-anticipation paragraph anchored at parent SPEC commit STANDS UNCHANGED; the AMENDMENT body lands at 22.3 IMPL final Task (per the 22.3 sub-phase PLAN).

> **ADR-0044 escape-valve held in reserve per D-P10 + D3 + §13-R6 + §13-R9** — ADR-0193 is the conditional escape-valve slot; PLAN hypothesis is that NEITHER R6 NOR R9 fires (R6 stays under 1ms per 22.1 baseline scaling; R9 stays embedded in ADR-0192). If at IMPL time a surface DOES warrant a NEW ADR beyond ADR-0193 (highly unlikely per the SPEC-time scrape closure of D1+D2+D4+D6+D7 + this PLAN-time D3+D5+D8 closures), it is ADR-0194 + the PLAN-anchored hypothesis is recorded as falsified in PROGRESS.md.

---

## Task graph (sequential vs parallelizable per D-P8)

The IMPL session subagent-dispatches per `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). Per-task graph (depicted in topological order):

- **Pre-Task 0** (PROGRESS.md preamble + 17-precondition verification) — sequential prerequisite for all subsequent tasks
- **Tasks 1 + 2 + 3 + 4 — PARALLELIZABLE (4-way)** — all depend only on Pre-Task 0:
  - Task 1: NEW `internal/dynamicmetadata/` package (Bucket + Get/Set/Snapshot/Reset)
  - Task 2: EXTEND `internal/lua/` with coroutine API (NewThread + Resume + YieldFromBridge in NEW coroutine.go)
  - Task 3: EXTEND `internal/lua/` with BodyBuffer interface (NEW body_buffer.go)
  - Task 4: IN-PLACE AMEND `internal/httpclient/` with ClusterDispatch + FactoryCtx.ClusterManager field
- **Task 5** (sequential; depends on Tasks 1 + 4): EXTEND `internal/filter/http/chain.go` + `callbacks.go` with `tlsConnectionState` field + `dynamicMetadata` field + 4 accessors
- **Task 6** (sequential; depends on Task 5): EXTEND `internal/filter/hcm/connection.go` (H1) + `h2dispatch.go` (H2) for tlsConnectionState + dynamicMetadata seeding before RunDecodeHeaders
- **Tasks 7 + 8 + 9 + 10 + 11 + 12 + 13 — PARALLELIZABLE (7-way)** — all depend only on Tasks 1-6 (framework primitives + chain.go + HCM wire-in):
  - Task 7: body bridge (`body.go` + decode-side accumulation + coroutine yield/resume)
  - Task 8: trailers bridge (`trailers.go` + bridge.go metatable installs)
  - Task 9: metadata + dynamic-metadata bridge (`metadata.go`)
  - Task 10: connection-SSL bridge (`connection.go` + `ssl.go`)
  - Task 11: httpCall bridge (`httpcall.go`)
  - Task 12: crypto + fileBytes + timestamp bridge (`crypto.go` + `misc.go`)
  - Task 13: streamInfo extension + filter-state bridge (`streaminfo.go` + `filterstate.go`)
- **Tasks 14 + 15 + 16 — PARALLELIZABLE (3-way)** — all depend on Tasks 7-13 (bridge surfaces landed):
  - Task 14: stats + runtime-reject arms 20-22 (`stats.go` + `compiled_config.go`)
  - Task 15: race + concurrency + 2 benchmarks per D-P10 (`lua_test.go` extensions)
  - Task 16: 29th + 30th fuzzers (`fuzz_test.go` extensions)
- **Tasks 17 + 18 — PARALLELIZABLE (2-way)** — both depend on Tasks 7-16:
  - Task 17: cert fixture plumbing for scenario (f-B) per D5 closure (REUSE or generate minimal cert)
  - Task 18: fixture-0027 directory + 13 scripts + YAMLs + driver + R11 REUSE disposition for REFERENCE-LESS pattern
- **Task 19** (sequential tail) — atomic landing: BEHAVIOR_CONTRACT.md 15-edit bundle + 3 ADR §Decision+§Consequences + ADR-0177 IN-PLACE AMENDMENT + conditional ADR-0193 + STATE.md re-advance + ROADMAP row 22.2 IMPL-done + REVIEW.md authoring

**Parallel-dispatch opportunities**: 4-way at Tasks 1+2+3+4; 7-way at Tasks 7+8+9+10+11+12+13; 3-way at Tasks 14+15+16; 2-way at Tasks 17+18. **Sequential bottlenecks**: Pre-Task-0 → {1,2,3,4}; {1,2,3,4} → 5; 5 → 6; 6 → {7,8,9,10,11,12,13}; {7,8,9,10,11,12,13} → {14,15,16}; {14,15,16} → {17,18}; {17,18} → 19.

---

## Execution preconditions

Verify BEFORE any Task starts (sequential prerequisite at Pre-Task 0):

1. **Worktree branch** — `git rev-parse --abbrev-ref HEAD` returns `phase-22.2-http-filter-lua-full-bridge-impl` (the IMPL worktree branch name; matches the worktree-spawn discipline at the cold-start prompt per project memory `feedback_git_worktrees.md`).
2. **Master tail** — `git log --oneline master | head -6` shows: 22.2 PLAN follow-up SHA-fill commit + 22.2 PLAN squash-merge commit + `7b93465 phase 22.2 SPEC follow-up: STATE.md SHA-fill (TBD → 0d6463e post-squash)` + `0d6463e Squash merge phase-22.2-http-filter-lua-full-bridge-spec` + `ac94a92 phase 22.2 BRAINSTORM follow-up: STATE.md SHA-fill (TBD → 6ad3064 post-squash)` + `6ad3064 Squash merge phase-22.2-http-filter-lua-full-bridge-brainstorm`.
3. **Toolchain** — `go version` returns `go1.26.2`; `golangci-lint version` returns `1.64.8`; `docker version` reachable.
4. **DECISIONS.md tail** — `grep -cE '^## ADR-' docs/envoy-go/DECISIONS.md` matches expected count post-22.2-PLAN-SHA-fill commit; highest ADR `## ADR-0192` (§Context draft anchored).
5. **ADR §Context drafts present** — `grep -cE '^## ADR-0190' docs/envoy-go/DECISIONS.md` returns 1; same for ADR-0191 + ADR-0192. Each has §Context body present + §Decision + §Consequences sections empty/TBD per ADR-0044 in-place edit discipline.
6. **NO ADR-0125 §(xiv) AMENDMENT body** — `grep -nE '^### \(xiv\)' docs/envoy-go/DECISIONS.md` returns ANTICIPATION paragraph but NOT amendment body (AMENDMENT body lands at 22.3 IMPL final Task; UNCHANGED at 22.2).
7. **SPEC SHA** — `git log -1 --format=%H -- docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/SPEC.md` returns the 22.2 SPEC commit SHA (`0d6463e` per master tail at this PLAN session).
8. **PLAN SHA** — `git log -1 --format=%H -- docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PLAN.md` returns the 22.2 PLAN squash-merge SHA (TBD at this PLAN session; backfilled at PLAN-done SHA-fill follow-up commit).
9. **Pristine tree** — `git status --porcelain` returns empty (no uncommitted changes).
10. **Pre-existing suite green at `-short` budget** — `go test -count=1 -short ./...` clean.
11. **Pre-existing differential suite green** — `go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-6])'` clean (28 fixtures including 22.1's 0026).
12. **Pre-existing fuzzers run clean at 30s** — `go test -fuzz=FuzzLuaConfigParse -fuzztime=30s ./internal/filter/http/lua/` clean from 22.1 IMPL.
13. **Reference Envoy image present** — `docker image inspect envoyproxy/envoy:v1.37.2` returns valid metadata.
14. **Proto/library packages reachable** — `go doc github.com/yuin/gopher-lua VM` clean; `go doc google.golang.org/protobuf/types/known/structpb Value` clean.
15. **Pre-existing 22.2 framework deltas absent** — `test ! -d internal/dynamicmetadata && echo "ok"`; `test ! -f internal/lua/coroutine.go && echo "ok"`; `test ! -f internal/lua/body_buffer.go && echo "ok"`; `test ! -d test/fixtures/0027-http-lua-full-bridge && echo "ok"`.
16. **OpenSSL available** (for D5 cert scripting if Task 17 generates new cert) — `openssl version` returns valid.
17. **`maybeWrapLuaScriptLoadError` helper at `cmd/envoy-go/main.go` present** (from 22.1 IMPL) — `grep -n maybeWrapLuaScriptLoadError cmd/envoy-go/main.go` returns 1 match.

If all 17 preconditions pass, proceed to Task 1.

---

## Pre-Task 0: PROGRESS.md preamble + 17-precondition verification

**Files:**
- Create: `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md`

This task verifies the `## Execution preconditions` block above and creates PROGRESS.md so subsequent tasks have an append target. Per ADR-0044, ADR-0190 + ADR-0191 + ADR-0192 §Context drafts are at the 22.2 SPEC commit `0d6463e` (re-anchored via SHA-fill follow-up `7b93465`); §Decision + §Consequences bodies authored at Task 19; the ADR-0177 IN-PLACE AMENDMENT body is at Task 19; ADR-0193 is CONDITIONAL (PLAN hypothesis per D-P10 + D3: it does NOT fire at 22.2 IMPL). The PROGRESS preamble ANTICIPATES the 3 NEW ADR §Decision + §Consequences body landings + the 1 IN-PLACE AMENDMENT body landing on ADR-0177 + the conditional ADR-0193 escape-valve disposition (each with its Lands-in-Task anchor reproduced from this PLAN's per-ADR table) and records the 14 planner-time decisions (D3 + D5 + D8 SPEC-time carry-forward closures + D-P1..D-P11 PLAN-emerged).

**Precondition:** Worktree exists at `phase-22.2-http-filter-lua-full-bridge-impl`; branch base is master tip after the 22.2-PLAN SHA-fill follow-up; all 17 preconditions in `## Execution preconditions` above report green.
**Artifact:** `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (new file).
**Acceptance:** all 17 preconditions report green; PROGRESS.md preamble committed; `git log -1 --format=%H -- docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` returns the Pre-Task 0 commit's SHA.

- [ ] **Step 1: Verify each precondition** — run each command from `## Execution preconditions` above and confirm the expected output. Capture each command + verbatim output for the PROGRESS preamble.

- [ ] **Step 2: Author `PROGRESS.md` preamble** — create `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` with: (a) Preamble summarizing the 17-precondition verification (verbatim command outputs captured per `superpowers:verification-before-completion`); (b) the 3-NEW-ADR + 1-IN-PLACE-AMENDMENT + 1-CONDITIONAL-ADR table from `## ADRs introduced/landed by this plan` reproduced verbatim; (c) the 14 planner-time decisions (D3 + D5 + D8 + D-P1..D-P11) reproduced verbatim from `## Planner-time deferred-decision resolution` above; (d) a Pre-Task 0 entry slot for the commit-SHA fill-in.

- [ ] **Step 3: Commit**

```bash
git add docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
git commit -m "phase 22.2 Pre-Task 0: PROGRESS.md preamble + 17-precondition verification"
git log -1 --format=%H -- docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
# expect: a 40-char SHA (Pre-Task 0 commit)
```

---

## Tier A — Framework primitives (Tasks 1-5)

## Task 1: NEW `internal/dynamicmetadata/` package (Bucket + Get/Set/Snapshot/Reset) [ADR-0190]

**Files:**
- Create: `internal/dynamicmetadata/doc.go` (~50-80 LoC)
- Create: `internal/dynamicmetadata/dynamicmetadata.go` (~80-130 LoC)
- Create: `internal/dynamicmetadata/dynamicmetadata_test.go` (~150-250 LoC)
- Create: `internal/dynamicmetadata/bench_test.go` (~50-80 LoC)
- Append: `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (Task 1 entry per D-P3)

Lands the NEW `internal/dynamicmetadata/` framework primitive per SPEC §3.1 + ADR-0190 §Context. Per-stream `*Bucket` accessor + map keyed by `(filterName, key) → *structpb.Value`; `NewBucket`/`Get`/`Set`/`Snapshot`/`Reset` methods nil-tolerant per ADR-0085; per-stream lifecycle per ADR-0033. **PARALLELIZABLE with Tasks 2 + 3 + 4.**

**Precondition:** Pre-Task 0 complete; 17 preconditions green.
**Artifact:** 4 NEW files at `internal/dynamicmetadata/`.
**Acceptance:** `go test -count=1 -race ./internal/dynamicmetadata/...` clean; nil-bucket tolerance verified via table-driven tests; package-level docs cross-reference ADR-0190 §Context; `go build ./...` clean; `go vet ./...` clean; `golangci-lint run` clean.

**Subagent dispatch outline (per D-P2 `general-purpose`):**

> Author Task 1 per the 22.2 PLAN Task 1 + 22.2 SPEC §3.1 + ADR-0190 §Context draft. The NEW package `internal/dynamicmetadata/` ships with 4 files: doc.go + dynamicmetadata.go + dynamicmetadata_test.go + bench_test.go. The `Bucket` struct has unexported `m map[string]map[string]*structpb.Value`. `NewBucket() *Bucket` returns initialized empty bucket. `(b *Bucket) Get(filterName, key string) (*structpb.Value, bool)` looks up the nested map; nil bucket returns `(nil, false)`. `(b *Bucket) Set(filterName, key string, value *structpb.Value)` writes through nested map; auto-initializes inner map; nil bucket is no-op. `(b *Bucket) Snapshot() map[string]map[string]*structpb.Value` returns a defensive copy (mutating snapshot does NOT mutate bucket). `(b *Bucket) Reset()` clears the inner map. All methods nil-tolerant per ADR-0085. Doc.go cross-references SPEC §3.1 + §1.6 + ADR-0190 + ADR-0033 + ADR-0085. Test file is table-driven; bench file has informational micro-benchmarks.

- [ ] **Step 1: Write failing tests in `dynamicmetadata_test.go`** per `superpowers:test-driven-development`. Table-driven `TestBucket_*` covering NewBucket non-nil + empty + Get_on_empty_returns_nil_false + Set_then_Get_roundtrip + Set_overwrites + Snapshot_defensive_copy + Reset_clears + nil_bucket_tolerance (Get/Set/Snapshot/Reset on nil receiver) + structpb.Value payload variations + cross-filter key independence.

- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/dynamicmetadata/... 2>&1 | head -20` → expect FAIL with "no such package" or "Bucket undefined".

- [ ] **Step 3: Author `doc.go` + `dynamicmetadata.go` + `bench_test.go`** per File-structure table rows above + SPEC §3.1 production signatures + ADR-0190 §Context.

- [ ] **Step 4: Run tests to verify they pass** — `go test -count=1 -race ./internal/dynamicmetadata/...` → expect PASS.

- [ ] **Step 5: Verify go build/vet/lint clean** — `go build ./...` + `go vet ./...` + `golangci-lint run` all clean.

- [ ] **Step 6: Append PROGRESS.md Task 1 entry per D-P3.**

- [ ] **Step 7: Commit**

```bash
git add internal/dynamicmetadata/ docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
git commit -m "phase 22.2 Task 1: NEW internal/dynamicmetadata/ framework primitive [ADR-0190]

Lands NEW per-stream *Bucket accessor for cross-filter dynamic-metadata
read+write at first co-consumer per SPEC §3.1 + ADR-0190 §Context. 4 NEW
files: doc.go + dynamicmetadata.go + dynamicmetadata_test.go + bench_test.go.
Bucket exposes Get/Set/Snapshot/Reset; nil-bucket tolerant per ADR-0085;
per-stream sequential per ADR-0033. THIRD §9 framework primitive in
two-phase succession (after ADR-0188 + ADR-0189 at 22.1 IMPL).

ADR-0190 §Decision + §Consequences body lands at Task 19 atomic landing
(this Task 1 commit anchors the primitive code; §Decision body anchors
the ADR formalization)."
```

---

## Task 2: EXTEND `internal/lua/` with coroutine API (NewThread + Resume + YieldFromBridge) [ADR-0191]

**Files:**
- Create: `internal/lua/coroutine.go` (~150-250 LoC)
- Create: `internal/lua/coroutine_test.go` (~250-400 LoC)
- Modify: `internal/lua/vm.go` (panic-wrapper integration touchpoint ONLY; ~+0-30 LoC; NO API revisions to 22.1 surface per Q10 strict scope; ALL NEW methods LIVE IN coroutine.go per D-P5 LOCKED-at-NEW-FILES disposition)
- Append: PROGRESS.md (Task 2 entry per D-P3)

Lands the coroutine API extensions per SPEC §3.2 + ADR-0191 §Context. `NewThread` + `Resume` methods on `*VM` + Go-side helper `YieldFromBridge`. Per-stream child-LState lifecycle = 1 parent (script owner) + 1 child per phase invocation (the coroutine). gopher-lua native via `LState.NewThread()` (state.go:1614) + `LState.Yield()` (returns `-1` sentinel from Go bridge LGFunction) + `LState.Resume()` from Envoy data callback. NO Go-side channel scheduling wrapper (Option B REJECTED per §11.1 D2 closure). Panic-wrapper integration via deferred `recover()` paths into `vm.panicH` callback per ADR-0188 §Decision 2. **PARALLELIZABLE with Tasks 1 + 3 + 4.**

**Precondition:** Pre-Task 0 complete.
**Artifact:** 2 NEW files at `internal/lua/`.
**Acceptance:** `go test -count=1 -race ./internal/lua/...` clean; NewThread returns non-nil child *LState + non-nil CancelFunc; Resume happy + yield-resume round-trip; CancelFunc cleans up child without goroutine leaks; panic-wrapper integration verified via panicH callback; race tests N=100 parallel coroutines clean under `-race`.

**Subagent dispatch outline:**

> Author Task 2 per 22.2 PLAN Task 2 + 22.2 SPEC §3.2 + ADR-0191 §Context + §11.1 D2 closure. NEW file `internal/lua/coroutine.go` contains `(vm *VM) NewThread() (*lua.LState, context.CancelFunc)` (wraps `vm.state.NewThread()`) + `(vm *VM) Resume(child *lua.LState, fn *lua.LFunction, args ...lua.LValue) (lua.ResumeState, error, []lua.LValue)` (wraps `vm.state.Resume(child, fn, args...)`; panic-wrapped via deferred recover into vm.panicH) + `YieldFromBridge(L *lua.LState, args ...lua.LValue) int` (Go-side helper; returns `L.Yield(args...)` which returns `-1` sentinel; gopher-lua vm.go:200-210 callGFunction sees `-1` and calls switchToParentThread; bridge author stashes suspended *LState in per-stream pending-map keyed by filter pointer). NO API REVISIONS to ADR-0188's existing VM surface per Q10 strict scope. Test file covers coroutine API + race + panic-wrapper integration.

- [ ] **Step 1: Write failing tests in `coroutine_test.go`** — TestVM_NewThread_returns_nonNil_child_and_CancelFunc + TestVM_Resume_happy_no_yield + TestVM_Resume_with_YieldFromBridge_roundtrip + TestVM_Resume_after_yield_resumes_from_where_Yield_returned + TestVM_NewThread_CancelFunc_cleans_up_without_leaks (`runtime.NumGoroutine` + child-LState closed assertions) + TestVM_Resume_panic_wraps_via_panicH + TestVM_Coroutine_race_N100_parallel_clean_under_race (skipped if `-race` not set).

- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/lua/... -run 'TestVM_NewThread|TestVM_Resume|TestVM_Coroutine' 2>&1 | head -20` → expect FAIL with "NewThread undefined" or "Resume undefined" or "YieldFromBridge undefined".

- [ ] **Step 3: Author `coroutine.go`** per File-structure table row above + SPEC §3.2 production signatures + ADR-0191 §Context. Integrate panic-wrapper via deferred `recover()` paths.

- [ ] **Step 4: Run tests to verify they pass** — `go test -count=1 -race ./internal/lua/... -run 'TestVM_NewThread|TestVM_Resume|TestVM_Coroutine'` → expect PASS.

- [ ] **Step 5: Verify go build/vet/lint clean.**

- [ ] **Step 6: Append PROGRESS.md Task 2 entry per D-P3.**

- [ ] **Step 7: Commit**

```bash
git add internal/lua/coroutine.go internal/lua/coroutine_test.go docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
git commit -m "phase 22.2 Task 2: internal/lua/ coroutine API extensions [ADR-0191]

Lands NewThread + Resume + YieldFromBridge per SPEC §3.2 + ADR-0191
§Context + §11.1 D2 closure (gopher-lua native LState.NewThread/Yield/
Resume). NEW file coroutine.go preserves clean ADR-0188 vs ADR-0191
lineage separation per Q10 strict scope (no API revisions to 22.1 VM
surface; consumer-#1-scope-expansion lands under NEW ADR not in-place
AMEND on ADR-0188). Per-stream child-LState lifecycle: 1 parent + 1 child
per phase invocation; child's context.CancelFunc invoked at stream destroy.

Race tests N=100 parallel coroutines clean under -race. Panic-wrapper
integration verified via panicH callback. ADR-0191 §Decision + §Consequences
body lands at Task 19 atomic landing."
```

---

## Task 3: EXTEND `internal/lua/` with BodyBuffer interface [ADR-0191]

**Files:**
- Create: `internal/lua/body_buffer.go` (~30-60 LoC)
- Create: `internal/lua/body_buffer_test.go` (~80-150 LoC)
- Append: PROGRESS.md (Task 3 entry per D-P3)

Lands the body-buffer seam interface per SPEC §3.2 + ADR-0191 §Context + §11.3 D3 RECOMMENDED. `BodyBuffer interface { Bytes() []byte; Chunks() [][]byte; EndStream() bool }`. NO concrete implementation here — consumer-side implementation lives at `internal/filter/http/lua/body.go` at Task 7. Seam interface enables future bridge-author flexibility per ADR-0191 §Decision body. **PARALLELIZABLE with Tasks 1 + 2 + 4.**

**Precondition:** Pre-Task 0 complete.
**Artifact:** 2 NEW files at `internal/lua/`.
**Acceptance:** `go test -count=1 ./internal/lua/...` clean (Task 2's coroutine_test merged + new body_buffer_test); interface signature stability + nil-tolerance of consumers reading Bytes()/Chunks()/EndStream(); `go build` + `go vet` + `golangci-lint` clean.

**Subagent dispatch outline:**

> Author Task 3 per 22.2 PLAN Task 3 + 22.2 SPEC §3.2 + ADR-0191 §Context + §11.3 D3 RECOMMENDED option (a) defensive copy at endStream. NEW file `internal/lua/body_buffer.go` declares the interface (3 methods); NO implementation in this file. Test file uses test-double `mockBodyBuffer` satisfying the interface for upstream-side test pinning. Doc-comment cross-references ADR-0128's HCM-level decode-side body-buffer accumulation primitive (the upstream supplier of underlying bytes); cross-references SPEC §11.3 D3 RECOMMENDED + this PLAN's D3 closure paragraph; ADR-0191 §Decision body codifies the seam contract.

- [ ] **Step 1: Write failing tests in `body_buffer_test.go`** — TestBodyBuffer_interface_signature_compiles_with_mock + TestBodyBuffer_nil_tolerance_for_consumers + TestBodyBuffer_mock_Bytes_Chunks_EndStream_return_canned_values.

- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/lua/... -run TestBodyBuffer 2>&1 | head -20` → expect FAIL with "BodyBuffer undefined".

- [ ] **Step 3: Author `body_buffer.go`** per File-structure table row above. Doc-comment cross-references.

- [ ] **Step 4: Run tests to verify they pass** — `go test -count=1 ./internal/lua/... -run TestBodyBuffer` → expect PASS.

- [ ] **Step 5: Verify go build/vet/lint clean.**

- [ ] **Step 6: Append PROGRESS.md Task 3 entry per D-P3.**

- [ ] **Step 7: Commit**

```bash
git add internal/lua/body_buffer.go internal/lua/body_buffer_test.go docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
git commit -m "phase 22.2 Task 3: internal/lua/ BodyBuffer interface seam [ADR-0191]

Lands BodyBuffer interface per SPEC §3.2 + ADR-0191 §Context + §11.3 D3
RECOMMENDED option (a) defensive copy at endStream. 3 methods: Bytes() +
Chunks() + EndStream(). NO concrete implementation here — consumer-side
at internal/filter/http/lua/body.go at Task 7.

Doc-comment cross-references ADR-0128's HCM-level decode-side body-buffer
accumulation primitive (the upstream supplier of underlying bytes).
ADR-0191 §Decision body lands at Task 19 atomic landing."
```

---

## Task 4: IN-PLACE AMEND `internal/httpclient/` with ClusterDispatch + FactoryCtx.ClusterManager [AMEND-ADR-0177]

**Files:**
- Modify: `internal/httpclient/httpclient.go` (~+80-130 LoC delta; NEW method `ClusterDispatch` + NEW struct field for cluster manager threading per ADR-0177 IN-PLACE AMENDMENT)
- Modify: `internal/httpclient/httpclient_test.go` (~+150-250 LoC delta; new tests for ClusterDispatch)
- Modify: `internal/filter/http/factory.go` (~+10-20 LoC delta; NEW `ClusterManager *cluster.Manager` field on `FactoryCtx`)
- Append: PROGRESS.md (Task 4 entry per D-P3)

IN-PLACE AMENDMENT on ADR-0177 per SPEC §3.3 + §11.4 (NO new ADR number consumed — matches phase-17 → phase-18 ADR-0149 → ADR-0150 AMEND precedent). NEW method `(c *Client) ClusterDispatch(ctx, clusterName, request, clusterMgr) (*http.Response, error)` for cluster-based dispatch via cluster manager LB + per-cluster TLS + retry inheritance. NEW `FactoryCtx.ClusterManager` field paralleling existing `FactoryCtx.HTTPClient`. R5 RATIFIED-PENDING (first co-consumer validation of phase-20's `internal/httpclient/` primitive at 22.2's `:httpCall()` bridge). **PARALLELIZABLE with Tasks 1 + 2 + 3.**

**Precondition:** Pre-Task 0 complete.
**Artifact:** 3 modified files.
**Acceptance:** `go test -count=1 -race ./internal/httpclient/...` clean; ClusterDispatch happy + cluster-not-found + endpoint-resolution + per-cluster TLS + retry + timeout + context-cancellation tests pass; FactoryCtx.ClusterManager field present.

**Subagent dispatch outline:**

> Author Task 4 per 22.2 PLAN Task 4 + 22.2 SPEC §3.3 + §11.4. IN-PLACE AMENDMENT body: `(c *Client) ClusterDispatch(ctx, clusterName, request, clusterMgr) (*http.Response, error)` resolves cluster via clusterMgr.Get → endpoint via Cluster.PickEndpoint → rewrites request.URL.Host → honors per-cluster TLS via Cluster.UpstreamTLSConfig() → constructs temporary *http.Client with cluster TLS + receiver's Options.Timeout/RetryPolicy → returns (*http.Response, error). NEW `FactoryCtx.ClusterManager *cluster.Manager` field paralleling existing HTTPClient field. Tests use test-double *cluster.Manager with canned cluster registrations. NO new ADR number consumed; the AMENDMENT body in DECISIONS.md lands at Task 19 atomic landing.

- [ ] **Step 1: Write failing tests in `httpclient_test.go`** — TestClient_ClusterDispatch_cluster_not_found_returns_error + TestClient_ClusterDispatch_endpoint_resolution_success + TestClient_ClusterDispatch_per_cluster_TLS_honored + TestClient_ClusterDispatch_retry_inherits_Options + TestClient_ClusterDispatch_timeout_via_context + TestClient_ClusterDispatch_context_cancellation_propagates + TestFactoryCtx_ClusterManager_field_present.

- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/httpclient/... -run TestClient_ClusterDispatch 2>&1 | head -20` → expect FAIL with "ClusterDispatch undefined".

- [ ] **Step 3: Author IN-PLACE AMENDMENT body in `httpclient.go`** + NEW field on `factory.go::FactoryCtx`. Cross-reference ADR-0177 §Decision AMENDMENT-anticipation paragraph anchored at 22.2 SPEC commit.

- [ ] **Step 4: Run tests to verify they pass** — `go test -count=1 -race ./internal/httpclient/... ./internal/filter/http/...` → expect PASS.

- [ ] **Step 5: Verify go build/vet/lint clean.**

- [ ] **Step 6: Append PROGRESS.md Task 4 entry per D-P3.**

- [ ] **Step 7: Commit**

```bash
git add internal/httpclient/ internal/filter/http/factory.go docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
git commit -m "phase 22.2 Task 4: internal/httpclient/ ClusterDispatch IN-PLACE AMEND [AMEND-ADR-0177]

Lands IN-PLACE AMENDMENT on ADR-0177 per SPEC §3.3 + §11.4: NEW method
Client.ClusterDispatch(ctx, clusterName, request, clusterMgr) for cluster-
based dispatch via cluster manager LB + per-cluster TLS + retry inheritance.
NEW FactoryCtx.ClusterManager field paralleling existing HTTPClient.

R5 RATIFIED at this Task — first co-consumer validation of phase-20's
internal/httpclient/ primitive at 22.2's :httpCall() bridge (consumed at
Task 11). NO new ADR number consumed (matches phase-17 → phase-18
ADR-0149 → ADR-0150 AMEND precedent). ADR-0177 §Decision AMENDMENT body
lands at Task 19 atomic landing."
```

---

## Task 5: EXTEND `internal/filter/http/chain.go` + `callbacks.go` (tlsConnectionState + dynamicMetadata fields + 4 accessors) [ADR-0192]

**Files:**
- Modify: `internal/filter/http/chain.go` (~+80-130 LoC delta; NEW fields + setters)
- Modify: `internal/filter/http/callbacks.go` (~+30-60 LoC delta; NEW accessors on DecoderFilterCallbacks + EncoderFilterCallbacks)
- Modify: `internal/filter/http/chain_test.go` + `callbacks_test.go` (~+150-300 LoC delta cumulative; new tests for fields + accessors)
- Append: PROGRESS.md (Task 5 entry per D-P3)

Extends `FilterChain` per SPEC §3 + §3.4 + §11.5 + ADR-0192 §Decision body (no separate ADR for chain-side extension per Q13 WEAK HOLD — lives inside ADR-0192). NEW fields: `tlsConnectionState *tls.ConnectionState` (set-once BEFORE RunDecodeHeaders per ADR-0071); `dynamicMetadata *dynamicmetadata.Bucket` (initialized at chain construction via NewBucket; reset at OnDestroy). NEW setters + 4 accessors on DecoderFilterCallbacks + EncoderFilterCallbacks. **DEPENDS on Tasks 1 + 4** (Task 1's Bucket type + Task 4's FactoryCtx.ClusterManager — though chain.go itself doesn't consume ClusterManager directly, the testing infrastructure needs them).

**Precondition:** Tasks 1 + 4 complete.
**Artifact:** 4 modified files.
**Acceptance:** `go test -count=1 -race ./internal/filter/http/...` clean; field setters + 4 accessors verified; nil-tolerance (plaintext / non-mTLS / no-bucket) verified.

**Subagent dispatch outline:**

> Author Task 5 per 22.2 PLAN Task 5 + 22.2 SPEC §3 + §3.4 + §11.5 + ADR-0192 §Decision body anticipation. EXTEND FilterChain struct with NEW fields: tlsConnectionState (*tls.ConnectionState) + dynamicMetadata (*dynamicmetadata.Bucket). NEW setters: SetTLSConnectionState(state) + SetDynamicMetadata(b). Modify chain construction to initialize dynamicMetadata via NewBucket(). Modify OnDestroy to Reset() then nil-out the bucket. EXTEND callbacks.go with 4 NEW accessors: DownstreamTLSConnectionState() on decoderCB + encoderCB + DynamicMetadata() on decoderCB + encoderCB. All accessors nil-tolerant (return nil if field unset). Cross-reference ADR-0192 §Decision body anticipation + ADR-0144 plumbing pattern + ADR-0071 set-once discipline.

- [ ] **Step 1: Write failing tests** in `chain_test.go` + `callbacks_test.go` — TestFilterChain_SetTLSConnectionState_roundtrip + TestFilterChain_SetDynamicMetadata_roundtrip + TestFilterChain_OnDestroy_resets_bucket + TestDecoderCB_DownstreamTLSConnectionState_returns_field + TestDecoderCB_DynamicMetadata_returns_field + TestEncoderCB_DownstreamTLSConnectionState_returns_field + TestEncoderCB_DynamicMetadata_returns_field + TestChain_nil_tlsConnectionState_returns_nil_via_accessor + TestChain_nil_dynamicMetadata_returns_nil_via_accessor.

- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/filter/http/... -run 'TestFilterChain_Set|TestDecoderCB|TestEncoderCB|TestChain_nil' 2>&1 | head -20` → expect FAIL.

- [ ] **Step 3: Author the field + setter + accessor additions** in chain.go + callbacks.go.

- [ ] **Step 4: Run tests to verify they pass** — `go test -count=1 -race ./internal/filter/http/...` → expect PASS.

- [ ] **Step 5: Verify go build/vet/lint clean.**

- [ ] **Step 6: Append PROGRESS.md Task 5 entry per D-P3.**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/chain.go internal/filter/http/callbacks.go internal/filter/http/chain_test.go internal/filter/http/callbacks_test.go docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
git commit -m "phase 22.2 Task 5: chain.go + callbacks.go tlsConnectionState + dynamicMetadata fields [ADR-0192]

Lands FilterChain.tlsConnectionState + FilterChain.dynamicMetadata field
extensions per SPEC §3 + §11.5 + ADR-0192 §Decision body anticipation
(lives inside ADR-0192 per Q13 WEAK HOLD — no separate ADR for chain-side
extension). 4 NEW accessors on Decoder+Encoder callbacks: DownstreamTLS
ConnectionState() + DynamicMetadata() each on both sides.

Extends ADR-0144 plumbing pattern (TLS principals → TLS connection state)
+ ADR-0033 per-stream sequential dispatch + ADR-0085 nil-bucket tolerance.
HCM-level seeding (H1 + H2) lands at Task 6."
```

---

## Tier B — HCM dispatch wire-in (Task 6)

## Task 6: EXTEND `internal/filter/hcm/connection.go` (H1) + `h2dispatch.go` (H2) for tlsConnectionState + dynamicMetadata seeding [ADR-0192]

**Files:**
- Modify: `internal/filter/hcm/connection.go` (~+30-60 LoC delta; H1 seed before RunDecodeHeaders)
- Modify: `internal/filter/hcm/h2dispatch.go` (~+30-60 LoC delta; H2 symmetric)
- Modify: `internal/filter/hcm/connection_test.go` + `h2dispatch_test.go` (~+60-120 LoC each)
- Append: PROGRESS.md (Task 6 entry per D-P3)

H1 + H2 dispatch seeds tlsConnectionState + dynamicMetadata before RunDecodeHeaders, symmetric to existing tlsPrincipals plumbing per ADR-0144. **DEPENDS on Task 5** (chain.go fields + setters).

**Precondition:** Tasks 1 + 5 complete.
**Artifact:** 4 modified files.
**Acceptance:** `go test -count=1 -race ./internal/filter/hcm/...` clean; tlsConnectionState seeded on TLS-handshake-complete connections + nil on plaintext + nil on non-mTLS; dynamicMetadata initialized at chain construction; H1 + H2 symmetric.

**Subagent dispatch outline:**

> Author Task 6 per 22.2 PLAN Task 6 + 22.2 SPEC §3 + §11.5 + ADR-0144 plumbing pattern. H1 dispatch (`connection.go::dispatchRequest`): NEW helper `downstreamTLSConnectionState(net.Conn) *tls.ConnectionState` co-located alongside existing `extractTLSPrincipals` helper — returns *tls.ConnectionState for TLS-handshake-complete conn; nil for plaintext/non-mTLS. Call `chain.SetTLSConnectionState(downstreamTLSConnectionState(downstreamConn))` after `chain.SetRequestCtx` + before `chain.RunDecodeHeaders` symmetric to existing tlsPrincipals plumbing. H2 dispatch (`h2dispatch.go::runH2`): extract tlsConnectionState once at connection build time + store on h2Dispatcher.tlsConnectionState; dispatcher copies to each chainDispatchAction.tlsConnectionState at Match time; `WriteH2` calls `chain.SetTLSConnectionState(c.tlsConnectionState)` after SetRequestCtx + before RunDecodeHeaders. dynamicMetadata initialization: lands at chain.go constructor (per D-P-X PLAN-time decision — chain.go owns lifecycle; HCM does NOT touch dynamicMetadata directly). Tests assert seeding correctness for TLS-handshake-complete + plaintext + non-mTLS rows; symmetric H1 + H2 tests.

- [ ] **Step 1: Write failing tests** in `connection_test.go` + `h2dispatch_test.go` — TestConnection_dispatchRequest_seeds_tlsConnectionState_for_TLS_handshake_complete + TestConnection_dispatchRequest_seeds_nil_for_plaintext + TestConnection_dispatchRequest_seeds_nil_for_non_mTLS + TestH2Dispatch_runH2_seeds_tlsConnectionState_symmetric.

- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/filter/hcm/... -run 'TestConnection_dispatchRequest_seeds|TestH2Dispatch_runH2_seeds' 2>&1 | head -20` → expect FAIL with assertion failures.

- [ ] **Step 3: Author the seeding logic** in connection.go + h2dispatch.go. Reuse existing tlsPrincipals plumbing pattern.

- [ ] **Step 4: Run tests to verify they pass** — `go test -count=1 -race ./internal/filter/hcm/... ./internal/filter/http/...` → expect PASS.

- [ ] **Step 5: Verify go build/vet/lint clean.**

- [ ] **Step 6: Append PROGRESS.md Task 6 entry per D-P3.**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/hcm/ docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
git commit -m "phase 22.2 Task 6: HCM tlsConnectionState seeding (H1 + H2) [ADR-0192]

Lands H1 + H2 dispatch seeding of FilterChain.tlsConnectionState per
SPEC §3 + §11.5 + ADR-0144 plumbing pattern. Symmetric to existing
tlsPrincipals plumbing: H1 seeds at dispatchRequest before RunDecodeHeaders;
H2 seeds at WriteH2 after SetRequestCtx. NEW helper downstreamTLSConnection
State co-located alongside extractTLSPrincipals.

dynamicMetadata initialization lives at chain.go constructor (per D-P-X
PLAN-time decision — chain.go owns lifecycle; HCM does NOT touch
dynamicMetadata directly)."
```

---

## Tier C — Bridge surfaces (Tasks 7-13; 7-way parallelizable)


## Task 7: `body.go` body bridge + decode-side accumulation + coroutine yield/resume [ADR-0192]

**Files:**
- Create: `internal/filter/http/lua/body.go` (~250-400 LoC)
- Create: `internal/filter/http/lua/body_test.go` (~300-500 LoC)
- Modify: `internal/filter/http/lua/decode_headers.go` (~+80-150 LoC delta; DecodeData accumulation + endStream-resume orchestration)
- Modify: `internal/filter/http/lua/encode_headers.go` (~+80-150 LoC delta; symmetric for response body)
- Modify: `internal/filter/http/lua/bridge.go` (request_handle + response_handle metatable :body / :bodyChunks method dispatch)
- Append: PROGRESS.md (Task 7 entry per D-P3)

Lands body bridge per SPEC §3.4 + §4.1 + §11.3 D3 closure (option (a) defensive copy at endStream). `:body()` whole-buffer + `:bodyChunks()` chunked iterator. Coroutine yield/resume orchestration via `internal/lua.YieldFromBridge` (consumes Task 2's API). Defensive copy at endStream per D3 closure at this PLAN session. `body_buffered_bytes_total` counter increments cumulative body-byte volume; `coroutine_yields_total` counter increments per yield event. Arm-21 `body-size-cap-exceeded` runtime-reject if `len(f.decodedBodyBytes) > maxBodyBufferedBytes`. **PARALLELIZABLE with Tasks 8-13.**

**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/... -run TestBody` clean; `:body()` returns full bytes + over-cap raises arm-21 + `:bodyChunks()` iterator yields chunks; coroutine yield-before-endStream then resume-with-bytes verified; defensive-copy verification (mutating Go-side `decodedBodyBytes` after `:body()` does NOT change the Lua string); counters increment correctly.

**Subagent dispatch outline:**

> Author Task 7 per 22.2 PLAN Task 7 + 22.2 SPEC §3.4 + §4.1 + §11.3 D3 + ADR-0192 §Decision body anticipation + ADR-0191 BodyBuffer interface (consumed via lua.YieldFromBridge + lua.Resume). NEW file body.go implements `requestHandleBody(L *lua.LState) int` + `requestHandleBodyChunks(L *lua.LState) int` + response-side symmetric. Per the §11.3 D3 closure (defensive copy at endStream): if endStream NOT yet fired, call `lua.YieldFromBridge(L, lua.LNil)` to suspend; on endStream Go-side resumes via `vm.Resume(child, fn, lua.LString(string(f.decodedBodyBytes)))`. If body over-cap raises arm-21 runtime-reject byte-stable wording per W2: `"lua: body: accumulated body exceeds maximum buffered size of %d bytes"`. Stats wiring: stats.bodyBufferedBytesTotal increments cumulative bytes; stats.coroutineYieldsTotal increments per yield. DecodeData modifications: append to f.decodedBodyBytes; at endStream signal body-ready + resume any pending coroutine. EncodeData symmetric for response_handle:body(). Cross-reference §13-R9 disposition (R9 conditional ADR-0193 fires only if implementation surfaces enough complexity to warrant separate ADR — PLAN hypothesis is R9 stays embedded in ADR-0192; surface this evaluation in PROGRESS.md Task 7 entry).

- [ ] **Step 1: Write failing tests in `body_test.go`** — Test_RequestHandleBody_returns_full_bytes + Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording + Test_RequestHandleBodyChunks_iterator_yields_chunks_then_nil + Test_RequestHandleBody_coroutine_yield_before_endStream_then_resume + Test_RequestHandleBody_defensive_copy_verified + Test_body_buffered_bytes_total_counter_increments + Test_coroutine_yields_total_counter_increments + Test_ResponseHandleBody_symmetric.

- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/filter/http/lua/... -run TestBody 2>&1 | head -20` → expect FAIL.

- [ ] **Step 3: Author body.go + DecodeData/EncodeData extensions + bridge.go metatable dispatch.** Evaluate §13-R9 complexity at IMPL time; if implementation surfaces enough complexity to warrant separate ADR, signal Task 19 atomic landing for conditional ADR-0193 via the **R9 signaling protocol**: Task 7 PROGRESS.md entry MUST contain a single sentinel line of the exact form `§13-R9 disposition: STAYS embedded in ADR-0192` OR `§13-R9 disposition: ADR-0193 FIRES`. Task 19 Step 10 greps for the literal substring `§13-R9 disposition: ADR-0193 FIRES` in the PROGRESS.md Task 7 entry to determine whether to land ADR-0193 §Context+§Decision+§Consequences at the atomic-landing commit. Similarly, Task 15 PROGRESS.md entry MUST contain the sentinel `§13-R6 disposition: STANDS WEAK-default at ns/op=<value>` OR `§13-R6 disposition: ADR-0193 FIRES at ns/op=<value>`; Task 19 Step 10 greps for `§13-R6 disposition: ADR-0193 FIRES`. EITHER signal firing fires ADR-0193 landing.

- [ ] **Step 4: Run tests to verify they pass** — `go test -count=1 -race ./internal/filter/http/lua/... -run TestBody` → expect PASS.

- [ ] **Step 5: Verify go build/vet/lint clean.**

- [ ] **Step 6: Append PROGRESS.md Task 7 entry per D-P3 + §13-R9 disposition record (R9 STAYS embedded in ADR-0192 | ADR-0193 escape-valve FIRES).**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/lua/body.go internal/filter/http/lua/body_test.go internal/filter/http/lua/decode_headers.go internal/filter/http/lua/encode_headers.go internal/filter/http/lua/bridge.go docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
git commit -m "phase 22.2 Task 7: body bridge + defensive-copy at endStream [ADR-0192]

Lands :body() + :bodyChunks() body bridge per SPEC §3.4 + §4.1 + §11.3
D3 closure at PLAN session (option (a) defensive copy at endStream).
Coroutine yield/resume orchestration via internal/lua.YieldFromBridge
(consumes Task 2's API) + internal/lua.Resume.

body_buffered_bytes_total counter increments cumulative body-byte volume;
coroutine_yields_total increments per yield event. Arm-21 body-size-cap-
exceeded runtime-reject byte-stable wording per W2.

§13-R9 disposition: <STAYS embedded in ADR-0192 | ADR-0193 escape-valve
FIRES at Task 19 atomic landing>."
```

---

## Task 8: `trailers.go` trailers bridge + bridge.go metatable installs [ADR-0192]

**Files:**
- Create: `internal/filter/http/lua/trailers.go` (~120-200 LoC)
- Modify: `internal/filter/http/lua/bridge.go` (trailers metatable installs + `__pairs` reuse from 22.1 `installPairsShim`)
- Modify: `internal/filter/http/lua/bridge_test.go` (~+150-300 LoC delta for trailers tests)
- Modify: `internal/filter/http/lua/decode_headers.go` + `encode_headers.go` (trailers-bridge wiring at terminal-state)
- Append: PROGRESS.md (Task 8 entry per D-P3)

Lands trailers bridge mirroring 22.1 headers metatable per SPEC §3.4 + §2.2. 8 mutation methods (`:get`/`:getAtIndex`/`:getNumValues`/`:add`/`:append`/`:remove`/`:replace` + 1 inherited) + `__pairs` reusing 22.1's `installPairsShim` alphabetical-snapshot. Lazy-available: `request_handle:trailers()` returns nil if no trailers. **PARALLELIZABLE with Tasks 7, 9-13.**

**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/... -run 'TestTrailers|TestBridge_Trailers'` clean; 8 trailers methods + `__pairs` alphabetical-snapshot + cross-run-determinism; nil-trailers returns nil.

**Subagent dispatch outline:**

> Author Task 8 per 22.2 PLAN Task 8 + 22.2 SPEC §3.4 + §2.2. NEW file trailers.go declares the 8 trailers methods. Reuses 22.1's `installPairsShim` discipline at bridge.go for trailers metatable's `__pairs`. Trailers metatable is conditionally installed at filter-construction time only when trailers are present per Lua-idiomatic lazy-availability. Test file extends 22.1's bridge_test.go with trailers-method tests + `__pairs` cross-run-determinism (alphabetical-snapshot verification across N=100 runs).

- [ ] **Step 1: Write failing tests** in `bridge_test.go` extension — 8 method tests + `__pairs` cross-run-determinism + nil-trailers-returns-nil. (~150-300 LoC additions.)

- [ ] **Step 2: Run tests to verify they fail** → expect FAIL.

- [ ] **Step 3: Author trailers.go + metatable install in bridge.go + decode/encode-headers trailers-bridge wiring at terminal-state.**

- [ ] **Step 4: Run tests to verify they pass** → expect PASS.

- [ ] **Step 5: Verify go build/vet/lint clean.**

- [ ] **Step 6: Append PROGRESS.md Task 8 entry per D-P3.**

- [ ] **Step 7: Commit**

```bash
git add internal/filter/http/lua/trailers.go internal/filter/http/lua/bridge.go internal/filter/http/lua/bridge_test.go internal/filter/http/lua/decode_headers.go internal/filter/http/lua/encode_headers.go docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md
git commit -m "phase 22.2 Task 8: trailers bridge + __pairs alphabetical-snapshot [ADR-0192]

Lands :trailers() mirroring 22.1 headers metatable per SPEC §3.4 + §2.2.
8 mutation methods + __pairs reusing 22.1's installPairsShim alphabetical-
snapshot discipline. Lazy-available (nil if no trailers).

Cross-run determinism verified N=100. Decode/encode-headers trailers-bridge
wiring at terminal-state."
```

---

## Task 9: `metadata.go` metadata + dynamic-metadata bridge [ADR-0192]

**Files:**
- Create: `internal/filter/http/lua/metadata.go` (~150-250 LoC)
- Create: `internal/filter/http/lua/metadata_test.go` (~200-350 LoC)
- Modify: `internal/filter/http/lua/bridge.go` (metadata metatable dispatch)
- Append: PROGRESS.md (Task 9 entry per D-P3)

Lands metadata bridge per SPEC §3.4 + §11.6 D1 closure (callable empty userdata at v1.32.4 binding-gap; NEVER nil per `MetadataMapWrapper` upstream pattern). `:streamInfo():dynamicMetadata()` + `:dynamicTypedMetadata(filterName)` consume `internal/dynamicmetadata/Bucket` from Task 1. structpb.Value ↔ Lua-value marshaling helpers. **PARALLELIZABLE with Tasks 7-8, 10-13.**

**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/... -run TestMetadata` clean; `:metadata()` ALWAYS returns callable empty userdata (NEVER nil); `:dynamicMetadata():get/set` round-trip; cross-filter key independence; nil-bucket tolerance via test-double; structpb.Value marshaling table-driven.

**Subagent dispatch outline:**

> Author Task 9 per 22.2 PLAN Task 9 + 22.2 SPEC §3.4 + §11.6 D1 closure. NEW file metadata.go: `requestHandleMetadata(L)` always returns callable empty userdata per D1 (matches MetadataMapWrapper upstream pattern); `:get(k)` returns lua.LNil; `pairs()` yields zero. `:streamInfo():dynamicMetadata()` returns userdata wrapping internal/dynamicmetadata/Bucket; `:get(filterName, key)` returns proto-Value-to-Lua marshaled OR nil; `:set(filterName, key, value)` marshals Lua value back to *structpb.Value and stores. Helpers for structpb.Value ↔ Lua-value marshaling (null + number + string + list + struct). nil-bucket tolerance per ADR-0085.

- [ ] **Step 1: Write failing tests** — Test_RequestHandleMetadata_returns_callable_empty_userdata + Test_Metadata_get_returns_nil + Test_Metadata_pairs_yields_zero_iterations + Test_DynamicMetadata_set_get_roundtrip + Test_DynamicMetadata_cross_filter_key_independence + Test_DynamicTypedMetadata_returns_typed_value + Test_DynamicMetadata_nil_bucket_tolerance.

- [ ] **Step 2-7:** standard TDD cycle + commit.

```bash
git commit -m "phase 22.2 Task 9: metadata + dynamicMetadata bridge [ADR-0192]

Lands :metadata() callable empty userdata at v1.32.4 binding-gap (NEVER nil
per MetadataMapWrapper upstream pattern; D1 closure at SPEC §11.6).
:streamInfo():dynamicMetadata() + :dynamicTypedMetadata(filterName) consume
internal/dynamicmetadata/Bucket from Task 1. structpb.Value ↔ Lua-value
marshaling helpers covering null/number/string/list/struct."
```

---

## Task 10: `connection.go` + `ssl.go` connection-SSL bridge [ADR-0192]

**Files:**
- Create: `internal/filter/http/lua/connection.go` (~80-120 LoC)
- Create: `internal/filter/http/lua/ssl.go` (~250-400 LoC)
- Create: `internal/filter/http/lua/ssl_test.go` (~300-500 LoC)
- Modify: `internal/filter/http/lua/bridge.go` (connection metatable dispatch)
- Append: PROGRESS.md (Task 10 entry per D-P3)

Lands connection-SSL bridge per SPEC §3.4 + BRAINSTORM §2.4 + D5 closure at this PLAN session (option (f-B) cert-fingerprint-only). `:connection():ssl()` returns ssl userdata wrapping `cb.DownstreamTLSConnectionState()` (nil on plaintext). 12 ssl methods: subject + sanLocal/Peer + validFrom + expirationPeer + sessionId + ciphersuiteId + tlsVersion + urlEncodedPemEncodedPeerCertificate + urlEncodedPemEncodedPeerCertificateChain + sha256PeerCertificateDigest + downstreamSslConnection. **PARALLELIZABLE with Tasks 7-9, 11-13.**

**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/... -run TestSSL` clean; 12 ssl methods exercised via test-double DecoderFilterCallbacks carrying canned *tls.ConnectionState; cross-side cert-fingerprint scenario (f-B per D5 closure) — `:sha256PeerCertificateDigest()` returns expected hex byte-exact; nil-tls path returns Lua nil from `:connection():ssl()`.

**Subagent dispatch outline:**

> Author Task 10 per 22.2 PLAN Task 10 + 22.2 SPEC §3.4 + BRAINSTORM §2.4 + D5 closure. NEW files connection.go + ssl.go. connection.go defines `requestHandleConnection(L)` returning connection userdata with `:ssl()` accessor; ssl userdata wraps *tls.ConnectionState. ssl.go implements 12 methods extracting from wrapped state — subject/sanLocal/sanPeer/validFrom/expirationPeer/sessionId/ciphersuiteId/tlsVersion/urlEncodedPemEncodedPeerCertificate/urlEncodedPemEncodedPeerCertificateChain/sha256PeerCertificateDigest/downstreamSslConnection. nil-tolerant: returns Lua empty string or nil if connection state absent. Cross-side cert-fingerprint scenario (f-B per D5 closure): the byte-exact ssl method `:sha256PeerCertificateDigest()` returns hex digest of cert's DER encoding. The other 11 methods may format implementation-specifically (ISO-8601 timezone; URL-encoded PEM ordering; cipher-suite-ID format) and so are NOT cross-side-byte-exact.

- [ ] **Step 1: Write failing tests** for 12 ssl methods via test-double DecoderFilterCallbacks + canned cert + plaintext-nil-tls path + cross-side fingerprint scenario.

- [ ] **Step 2-7:** standard TDD cycle + commit.

```bash
git commit -m "phase 22.2 Task 10: connection + ssl bridge [ADR-0192]

Lands :connection():ssl() 12-method surface per SPEC §3.4 + BRAINSTORM
§2.4 + D5 closure at this PLAN session (option (f-B) cert-fingerprint-
only cross-side). nil on plaintext / non-TLS / non-handshake-complete
per ADR-0144 plumbing pattern.

:sha256PeerCertificateDigest is byte-exact cross-side (32-byte hex of
cert DER); other 11 methods are subject-only test surface."
```

---

## Task 11: `httpcall.go` httpCall bridge [ADR-0192 + AMEND-ADR-0177 co-consumer]

**Files:**
- Create: `internal/filter/http/lua/httpcall.go` (~250-400 LoC)
- Create: `internal/filter/http/lua/httpcall_test.go` (~300-500 LoC)
- Modify: `internal/filter/http/lua/bridge.go` (httpcall metatable dispatch)
- Append: PROGRESS.md (Task 11 entry per D-P3)

Lands `:httpCall(cluster, headers, body, timeout_ms, asynchronous?)` per SPEC §3.4 + §11.7 D6 closure (PURE FIRE-AND-FORGET on `asynchronous=true`). FIRST CO-CONSUMER of Task 4's ADR-0177 IN-PLACE AMENDMENT (ClusterDispatch). Sync path yields coroutine via `internal/lua.YieldFromBridge`; Go-side dispatches via `f.clusterMgr.ClusterDispatch(ctx, cluster, request, clusterMgr)`; resumes coroutine with `(response_headers, response_body)` or `(nil, err_string)`. Async path spawns fire-and-forget goroutine; returns 0 values to Lua immediately; NO yield. Stats: `httpcall_total++` on every dispatch (sync + async); `httpcall_failures++` SYNC-ONLY on error; `httpcall_timeouts++` SYNC-ONLY on timeout; `coroutine_yields_total++` on yield event. Arm-20 `httpcall-cluster-name-required` runtime-reject if `cluster == ""`. **PARALLELIZABLE with Tasks 7-10, 12-13.**

**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/... -run TestHTTPCall` clean; empty cluster raises arm-20 byte-stable wording per W2; sync happy + timeout + 5xx + transport-failure increments correct counters; async fire-and-forget returns 0 values + does NOT yield + does NOT increment failures/timeouts on async transport-failure (per AMEND-22.2-3 D6 closure); coroutine yield-resume timing for sync path verified.

**Subagent dispatch outline:**

> Author Task 11 per 22.2 PLAN Task 11 + 22.2 SPEC §3.4 + §11.7 D6 closure + AMEND-22.2-3 + Task 4's ADR-0177 IN-PLACE AMENDMENT. NEW file httpcall.go: `requestHandleHttpCall(L *lua.LState) int` validates cluster!="" (else arm-20 runtime-reject); constructs *http.Request from Lua headers table + body string; if asynchronous=true: spawns goroutine that calls f.clusterMgr.ClusterDispatch + discards response/error per upstream lua_filter.cc:400-416 noopCallbacks; returns 0 values to Lua immediately. If asynchronous=nil or false: yields coroutine via YieldFromBridge; Go-side dispatches via ClusterDispatch; on response resumes with (response_headers, response_body); on error resumes with (nil, err_string). Stats wiring: httpcallTotal increments on every dispatch; httpcallFailures + httpcallTimeouts SYNC-ONLY per upstream parity. R5 closure at this Task: first co-consumer of ADR-0177 primitive validated.

- [ ] **Step 1: Write failing tests** for httpcall sync + async + arm-20 + counter wiring + coroutine timing.

- [ ] **Step 2-7:** standard TDD cycle + commit.

```bash
git commit -m "phase 22.2 Task 11: httpCall bridge [ADR-0192 + AMEND-ADR-0177 co-consumer]

Lands :httpCall(cluster, headers, body, timeout_ms, asynchronous?) per
SPEC §3.4 + §11.7 D6 closure (PURE FIRE-AND-FORGET on async=true per
AMEND-22.2-3). FIRST CO-CONSUMER of Task 4's ADR-0177 IN-PLACE AMENDMENT
ClusterDispatch — R5 RATIFIED at this Task.

Sync: coroutine yield + Go-side dispatch + resume with (headers, body)
or (nil, err). Async: fire-and-forget goroutine + 0 values + no yield.
httpcall_total on every dispatch; httpcall_failures/timeouts SYNC-ONLY
per upstream parity. Arm-20 cluster-name-required runtime-reject."
```

---

## Task 12: `crypto.go` + `misc.go` crypto + fileBytes + timestamp bridge [ADR-0192]

**Files:**
- Create: `internal/filter/http/lua/crypto.go` (~200-350 LoC)
- Create: `internal/filter/http/lua/crypto_test.go` (~250-400 LoC)
- Create: `internal/filter/http/lua/misc.go` (~120-200 LoC)
- Create: `internal/filter/http/lua/misc_test.go` (~180-300 LoC)
- Modify: `internal/filter/http/lua/bridge.go` (crypto + misc metatable dispatch + PublicKeyWrapper userdata metatable)
- Append: PROGRESS.md (Task 12 entry per D-P3)

Lands 6 crypto methods + `:fileBytes` + `:timestamp` per SPEC §3.4 + D8 closure at this PLAN session. 2 upstream-parity crypto methods (`:importPublicKey` + `:verifySignature`) MIMIC upstream's PublicKeyWrapper userdata return scope per D8-sub closure; 4 envoy-go-strict (`:sha256` + `:sha512` + `:base64Decode` + `:fileBytes`) document at BEHAVIOR_CONTRACT.md. Arm-22 `crypto-key-format-invalid` runtime-reject if PEM can't parse. `:timestamp(unit?)` non-deterministic wall-clock from `time.Now()`. **PARALLELIZABLE with Tasks 7-11, 13.**

**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/... -run 'TestCrypto|TestMisc'` clean; `:base64Escape` byte-output matches `absl::Base64Escape`; `:base64Decode` round-trips; `:sha256`/`:sha512` byte-output vectors; `:importPublicKey` parses RSA + ECDSA + Ed25519 PEMs; invalid PEM raises arm-22 byte-stable wording per W2; `PublicKeyWrapper:get()` returns key bytes; `:verifySignature(hash_algo, pubkey_wrapper, sig, text)` calling convention pinned to upstream (4 args ordered as upstream `lua_filter.cc:611`); signature-verification failure returns false; unsupported hash algo returns false; `:fileBytes` happy + ENOENT + over-cap (16 MiB+1 byte) + EACCES; `:timestamp` monotonic + unit conversion + invalid unit error.

**Subagent dispatch outline:**

> Author Task 12 per 22.2 PLAN Task 12 + 22.2 SPEC §3.4 + D8 closure at PLAN session. crypto.go: 6 methods. `:base64Escape(s)` → encoding/base64.StdEncoding.EncodeToString (upstream-parity per AMEND-22.2-1; matches absl::Base64Escape). `:base64Decode(s)` → encoding/base64.StdEncoding.DecodeString (envoy-go-strict per D8). `:sha256(s)` → crypto/sha256.Sum256 + hex (envoy-go-strict per D8). `:sha512(s)` → crypto/sha512.Sum512 + hex (envoy-go-strict per D8). `:importPublicKey(pem)` → crypto/x509.ParsePKIXPublicKey (or PKCS1RSA fallback for RSA-only PEMs); returns PublicKeyWrapper userdata MIMICKING upstream wrappers.h:415-427 scope (exposes `:get()` returning key bytes or nil per D8-sub closure). `:verifySignature(hash_algo, pubkey_wrapper, sig, text)` → takes the wrapper as 2nd arg (calling convention pinned to upstream lua_filter.cc:611 `request_handle:verifySignature(hash, pubkey_wrapper, sig, text)` — 4 args); dispatches hash algo via switch on hash_algo string; returns true/false. Invalid PEM raises arm-22 runtime-reject byte-stable wording `"lua: importPublicKey: %w"`. misc.go: `:fileBytes(path)` → os.Open + io.LimitReader(f, maxFilenameScriptBytes+1) with 16 MiB cap (matches 22.1 Task 11 cap pattern); over-cap raises runtime-reject; ENOENT path returns nil string or Lua runtime error per Lua-idiomatic disposition; arbitrary paths allowed (envoy-go-strict per D8 — `:fileBytes` NOT in upstream at any scope). `:timestamp(unit?)` wraps time.Now(); default unit 'milliseconds'; supports 'milliseconds'/'microseconds'/'seconds'; invalid unit raises Lua runtime error.

- [ ] **Step 1: Write failing tests** for 6 crypto methods + PublicKeyWrapper :get + `:verifySignature` calling convention + `:fileBytes` + `:timestamp` + arm-22.

- [ ] **Step 2-7:** standard TDD cycle + commit.

```bash
git commit -m "phase 22.2 Task 12: crypto + misc bridge (fileBytes + timestamp) [ADR-0192]

Lands 6 crypto methods + :fileBytes + :timestamp per SPEC §3.4 + D8
closure at PLAN session. PublicKeyWrapper userdata MIMICS upstream
wrappers.h:415-427 scope (D8-sub closure); :verifySignature calling
convention pinned to upstream lua_filter.cc:611 (4 args).

Classification per D8 PLAN scrape: :importPublicKey + :verifySignature
upstream-parity; :sha256/:sha512/:base64Decode/:fileBytes envoy-go-strict
(4 NEW BEHAVIOR_CONTRACT.md departure records at Task 19; bundle 7 → 11).
Arm-22 crypto-key-format-invalid runtime-reject byte-stable wording."
```

---

## Task 13: `streaminfo.go` extension + `filterstate.go` filter-state bridge [ADR-0192]

**Files:**
- Create: `internal/filter/http/lua/streaminfo.go` (~150-250 LoC)
- Create: `internal/filter/http/lua/filterstate.go` (~150-250 LoC)
- Create: `internal/filter/http/lua/filterstate_test.go` (~200-350 LoC)
- Modify: `internal/filter/http/lua/bridge.go` (extract 22.1's inline streamInfo to streaminfo.go + add 7 NEW methods + filterstate metatable dispatch)
- Append: PROGRESS.md (Task 13 entry per D-P3)

Lands 11-method `:streamInfo()` surface at 22.2 phase-done (4 inherited from 22.1 + 7 NEW). `:filterState()` IN-PACKAGE per SPEC §3.4 + §11.8 D4 closure + AMEND-22.2-4 — 2 envoy-go-strict divergences from upstream (`:set` mutation surface exposed + typed Lua-value marshaling at `:get`). Per-stream `map[string]any` accessor. **PARALLELIZABLE with Tasks 7-12.**

**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/... -run 'TestStreamInfo|TestFilterState'` clean; 11-method streamInfo surface verified; `:filterState():get` + `:set` round-trip; cross-stream isolation (N=10 parallel filter instances; no cross-stream leak); per-stream lifecycle (OnDestroy releases map); Lua value marshaling typed (string→LString; float64/int64→LNumber; bool→LBool; map[string]any→LTable recursive).

**Subagent dispatch outline:**

> Author Task 13 per 22.2 PLAN Task 13 + 22.2 SPEC §3.4 + §11.8 D4 + AMEND-22.2-4. EXTRACT 22.1's inline streamInfo to streaminfo.go for clarity; ADD 7 NEW methods (:upstreamHost/:upstreamCluster/:dynamicMetadata/:dynamicTypedMetadata/:requestedServerName/:filterState/:downstreamSslConnection). `:filterState()` lives at NEW filterstate.go — per-stream string-keyed `map[string]any` accessor; `:get(name)` returns marshaled Lua value per typed conversion table; `:set(name, value)` marshals Lua value back to `any` and stores (envoy-go-strict per AMEND-22.2-4 — upstream is strictly read-only). Per-stream lifecycle (created at filter struct allocation; destroyed at OnDestroy). 2 envoy-go-strict departure records at BEHAVIOR_CONTRACT.md §14 items 8+9 (anticipated; lands at Task 19).

- [ ] **Step 1: Write failing tests** for 11 streamInfo methods + filterstate :get/:set + cross-stream isolation + per-stream lifecycle + marshaling table-driven.

- [ ] **Step 2-7:** standard TDD cycle + commit.

```bash
git commit -m "phase 22.2 Task 13: streamInfo extension (4→11) + filterState bridge [ADR-0192]

Lands 11-method :streamInfo() surface (4 inherited from 22.1 + 7 NEW) +
:filterState() IN-PACKAGE per SPEC §3.4 + §11.8 D4 closure + AMEND-22.2-4
(2 envoy-go-strict divergences: :set mutation exposed + typed Lua-value
marshaling at :get; upstream is strictly read-only + always-string per
serializeAsString()).

Per-stream string-keyed map[string]any accessor. Cross-stream isolation
verified N=10 parallel filter instances. Per-stream lifecycle (OnDestroy
releases). 2 envoy-go-strict departure records at BEHAVIOR_CONTRACT.md
items 8+9 anticipated at Task 19 atomic landing."
```

---

## Tier D — Stats + runtime-rejects + race + fuzz + bench (Tasks 14-16; 3-way parallelizable)

## Task 14: `stats.go` 5 NEW counters + `compiled_config.go` runtime-reject arms 20-22 [ADR-0192]

**Files:**
- Modify: `internal/filter/http/lua/stats.go` (~+50-80 LoC delta; 3 → 8 counters)
- Modify: `internal/filter/http/lua/lua_test.go` (`TestStatNames_Equal_*` extension to 8 names byte-exact; +30-50 LoC)
- Modify: `internal/filter/http/lua/compiled_config.go` (~+30-80 LoC delta; runtime-reject arms 20-22 parsing wiring)
- Modify: `internal/filter/http/lua/compiled_config_test.go` (~+30-60 LoC delta; no new PARSE-REJECT arms at config-load but runtime-reject roster tests via fixture-level)
- Append: PROGRESS.md (Task 14 entry per D-P3)

EXTEND 22.1's 3-counter filterStats with 5 NEW envoy-go-strict counters per SPEC §7.1: `httpcallTotal`/`httpcallFailures`/`httpcallTimeouts`/`bodyBufferedBytesTotal`/`coroutineYieldsTotal`. PARSE-REJECT roster STAYS at 19 arms from 22.1 IMPL UNCHANGED at config-load; 3 NEW RUNTIME-REJECT arms 20-22 at runtime via `luaL_error` from Tasks 7+11+12. **PARALLELIZABLE with Tasks 15-16.**

**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/... -run 'TestStats|TestStatNames_Equal'` clean; 8 counters constructed under HCM-rooted template (TEMPLATE UNCHANGED from 22.1 per AMEND-2); empty-stat-prefix consecutive-dot verification carries forward; `TestStatNames_Equal_*` table-driven assertion extends to 8 stat names byte-exact; 19-arm PARSE-REJECT roster from 22.1 STILL passes (no regressions); runtime-reject arms 20-22 byte-stable wording fired correctly per W2.

**Subagent dispatch outline:**

> Author Task 14 per 22.2 PLAN Task 14 + 22.2 SPEC §7.1 + §7.2 + §6. EXTEND filterStats struct with 5 NEW counters. NEW package-level const declarations for 5 stat names. EXTEND newFilterStats factory + TestStatNames_Equal_* assertion to 8 names byte-exact. EXTEND compiled_config.go with maxBodyBufferedBytes default of 16 MiB (hard-coded constant; NOT a config field at SPEC time). NO new PARSE-REJECT arms at config-load — the 19 from 22.1 STAY UNCHANGED. The 3 runtime-reject arms 20-22 are fired from Tasks 7+11+12 production code; this Task 14 verifies their byte-stable wording is pinned + tested via fixture-level (re-runs Task 11 httpcall arm-20 + Task 7 body arm-21 + Task 12 crypto arm-22 tests).

- [ ] **Step 1: Write failing tests** — TestStatNames_Equal_extended_to_8 + Test_5_new_counters_constructed + Test_runtime_reject_arms_20_22_byte_stable_wording_pinned.

- [ ] **Step 2-7:** standard TDD cycle + commit.

```bash
git commit -m "phase 22.2 Task 14: 5 NEW counters + runtime-reject arms 20-22 [ADR-0192]

EXTENDS 22.1's 3-counter filterStats to 8: + httpcall_total +
httpcall_failures (SYNC-ONLY) + httpcall_timeouts (SYNC-ONLY) +
body_buffered_bytes_total + coroutine_yields_total. Project stat-count
delta 102 → 107 (+5).

19-arm config-load PARSE-REJECT roster UNCHANGED from 22.1. 3 NEW runtime-
reject arms 20-22 byte-stable wording pinned per W2 (fired from Tasks
7+11+12). 5 envoy-go-strict departure records anticipated at Task 19."
```

---

## Task 15: Race + concurrency tests + 2 benchmarks per D-P10 [ADR-0192 + conditional ADR-0193]

**Files:**
- Modify: `internal/filter/http/lua/lua_test.go` (~+250-450 LoC delta; race tests N=100 + 2 benchmarks)
- Modify: `internal/lua/coroutine_test.go` (~+50-100 LoC delta; race tests for coroutine API merged here per D-P9 race-scoping)
- Append: PROGRESS.md (Task 15 entry per D-P3 + benchmark `ns/op` verbatim quote + R6 disposition + R9 cross-check)

Race + concurrency + 2 benchmarks. `BenchmarkPerStream_FullBridge_LState_Construction` measures per-stream `*lua.LState` construction cost at FULL bridge surface per D-P10 + §13-R6 — threshold gate `ns/op > 1_000_000` (= 1ms) → ADR-0193 escape-valve fires at Task 19. `BenchmarkBodyBridge_DefensiveCopy_PerStream` measures defensive-copy overhead per D3 closure — threshold gates ≤1ms sub-MB + ≤100ms 16-MiB-saturated. **PARALLELIZABLE with Tasks 14, 16.**

**Acceptance:** `go test -count=1 -race ./internal/filter/http/lua/... ./internal/lua/...` clean across N=100 parallel filter dispatches; `BenchmarkPerStream_FullBridge_LState_Construction` reports `ns/op` value verbatim; `BenchmarkBodyBridge_DefensiveCopy_PerStream` reports `ns/op` for sub-MB + 16-MiB-saturated; R6 disposition recorded (STANDS WEAK-default | ADR-0193 escape-valve FIRES); R9 disposition cross-checked against Task 7 IMPL outcome.

**Subagent dispatch outline:**

> Author Task 15 per 22.2 PLAN Task 15 + 22.2 SPEC §13-R6 + D-P10 + D3 closure. Race tests: N=100 parallel filter dispatches under `-race`; assert no cross-stream state leak; no goroutine leaks via runtime.NumGoroutine baseline-delta + child-LState CancelFunc invocation tracking. `BenchmarkPerStream_FullBridge_LState_Construction` constructs N=10000 fresh VMs back-to-back covering ALL 22.2 metatable installs (request_handle + response_handle + headers + streamInfo + headersIter + trailers + dynamicMetadata + connection + ssl + httpcall + crypto + misc + filterstate + PublicKeyWrapper) plus parent+child LState pair via NewThread. Reports ns/op via b.N discipline. Threshold gate ns/op > 1_000_000 (= 1ms) → ADR-0193 escape-valve fires + signal Task 19 for ADR-0193 §Context+§Decision+§Consequences landing. 22.1 baseline at headers-only was 69865 ns/op (~70µs); 22.2 anticipated 200-500µs (3-7× headers-only); SHOULD STAY UNDER 1ms. `BenchmarkBodyBridge_DefensiveCopy_PerStream` constructs body-bridge surface + accumulates body bytes + defensive-copies at endStream; runs for sub-MB body (100 KB) + 16-MiB-cap-saturated body. Threshold gates per D3 closure. Outcomes recorded verbatim in PROGRESS.md per `superpowers:verification-before-completion`.

- [ ] **Step 1: Write failing tests** + benchmark setups (skeleton).

- [ ] **Step 2: Run tests to verify they fail** — `go test ./internal/filter/http/lua/...` → expect tests fail or benchmarks compile-error.

- [ ] **Step 3: Author race tests + benchmark bodies.** Subagent runs benchmarks against the FULL 22.2 bridge surface; captures `ns/op` values.

- [ ] **Step 4: Run race + benchmark suite** — `go test -race -count=10 ./internal/filter/http/lua/... ./internal/lua/...` + `go test -bench=BenchmarkPerStream_FullBridge_LState_Construction -benchtime=3s ./internal/filter/http/lua/` + `go test -bench=BenchmarkBodyBridge_DefensiveCopy_PerStream -benchtime=3s ./internal/filter/http/lua/`.

- [ ] **Step 5: Verify go build/vet/lint clean.**

- [ ] **Step 6: Append PROGRESS.md Task 15 entry per D-P3 + benchmark ns/op verbatim quote + R6 disposition sentinel.** PROGRESS.md Task 15 entry MUST contain the exact sentinel line `§13-R6 disposition: STANDS WEAK-default at ns/op=<value>` OR `§13-R6 disposition: ADR-0193 FIRES at ns/op=<value>` per the **R6 signaling protocol** consumed by Task 19 Step 10 (which greps for `§13-R6 disposition: ADR-0193 FIRES` to determine whether to land conditional ADR-0193). Cross-check vs §13-R9 disposition recorded at Task 7 PROGRESS.md entry per the R9 signaling protocol (Task 19 Step 10 also greps for `§13-R9 disposition: ADR-0193 FIRES`).

- [ ] **Step 7: Commit**

```bash
git commit -m "phase 22.2 Task 15: race + 2 benchmarks per D-P10 + D3 [ADR-0192 + cond ADR-0193]

Lands race tests N=100 parallel filter dispatches + 2 benchmarks per
D-P10 + D3 closure at PLAN session. BenchmarkPerStream_FullBridge_LState_
Construction: <ns/op value>. BenchmarkBodyBridge_DefensiveCopy_PerStream:
<sub-MB ns/op> + <16-MiB-saturated ns/op>.

R6 gate per SPEC §13-R6 + D-P10: <STANDS WEAK-default at ns/op < 1ms |
ADR-0193 escape-valve FIRES at Task 19 with §Context+§Decision+§Consequences
landing>. R9 cross-check vs Task 7 outcome: <STAYS embedded in ADR-0192 |
ADR-0193 fires for body-buffer-seam-with-ADR-0128 separation>."
```

---

## Task 16: 29th + 30th project-wide fuzzers (`FuzzLuaBodyBridge` + `FuzzLuaHTTPCallConfig`) per D-P7 [ADR-0192]

**Files:**
- Modify: `internal/filter/http/lua/fuzz_test.go` (~+150-250 LoC delta; 2 NEW fuzzers)
- Create: `internal/filter/http/lua/testdata/fuzz/FuzzLuaBodyBridge/` corpus dir (~15-20 seeds)
- Create: `internal/filter/http/lua/testdata/fuzz/FuzzLuaHTTPCallConfig/` corpus dir (~10-15 seeds)
- Append: PROGRESS.md (Task 16 entry per D-P3 + fuzzer count verification per §13-R10)

Lands 2 NEW project-wide fuzzers per D-P7 + SPEC §11.9 D7 + §13-R10. Project-wide fuzzer count 28 → 30 at 22.2 phase-done. Both fuzzers must-never-panic per ADR-0018; clean at 30s baseline. **PARALLELIZABLE with Tasks 14-15.**

**Acceptance:** `go test -fuzz=FuzzLuaBodyBridge -fuzztime=30s ./internal/filter/http/lua/` clean; `go test -fuzz=FuzzLuaHTTPCallConfig -fuzztime=30s ./internal/filter/http/lua/` clean; corpus seeds per D-P7 roster; project-wide fuzzer count CONFIRMED at 30 via `find . -name 'fuzz_test.go' | xargs grep -h '^func Fuzz' | sort -u | wc -l`.

**Subagent dispatch outline:**

> Author Task 16 per 22.2 PLAN Task 16 + 22.2 SPEC §11.9 D7 + §13-R10 + ADR-0018 + D-P7 corpus roster. `FuzzLuaBodyBridge`: fuzzes body-bridge against gopher-lua's coroutine state machine for panics — must-never-panic. Corpus seeds (~15-20): empty body / small body (10-100 bytes) / medium body (10 KB-100 KB) / large body (1 MB-15 MB) / over-cap body (17 MB; should runtime-reject not panic) / chunked body (multi-call DecodeData) / script-patterns that yield/resume in pathological orderings. `FuzzLuaHTTPCallConfig`: fuzzes httpCall config parameters at PARSE+runtime time — must-never-panic. Corpus seeds (~10-15): empty cluster name / valid cluster + headers + body + timeout / missing-cluster fallthrough / transport-failure simulation / oversized headers / oversized body / invalid timeout values / async-flag variations. Both fuzzers run 30s clean. Project-wide grep CONFIRMS count = 30 (28 from 22.1 + 2 new at 22.2 per D7).

- [ ] **Step 1: Author fuzz_test.go extensions + corpus seed files** (each seed is a test file at testdata/fuzz/<FuzzName>/seed_N).

- [ ] **Step 2: Run smoke fuzz at 5s** — `go test -fuzz=FuzzLuaBodyBridge -fuzztime=5s ./internal/filter/http/lua/` + same for FuzzLuaHTTPCallConfig — expect clean (no panics).

- [ ] **Step 3: Run full 30s baseline** for each fuzzer — expect clean.

- [ ] **Step 4: Verify fuzzer count** — `find . -name 'fuzz_test.go' | xargs grep -h '^func Fuzz' | sort -u | wc -l` → expect 30.

- [ ] **Step 5: Verify go build/vet/lint clean.**

- [ ] **Step 6: Append PROGRESS.md Task 16 entry per D-P3.**

- [ ] **Step 7: Commit**

```bash
git commit -m "phase 22.2 Task 16: 29th + 30th fuzzers (BodyBridge + HTTPCallConfig) [ADR-0192]

Lands FuzzLuaBodyBridge + FuzzLuaHTTPCallConfig per D-P7 + SPEC §11.9 D7 +
§13-R10 + ADR-0018 baseline. Corpus seeds per D-P7 roster (~25-35 total):
~15-20 body-bridge seeds + ~10-15 httpcall-config seeds.

Both fuzzers run 30s clean (no panics). Project-wide fuzzer count CONFIRMED
at 30 (was 28 from 22.1 + 2 new at 22.2)."
```

---

## Tier E — Differential fixture (Tasks 17-18; 2-way parallelizable)

## Task 17: Cert fixture plumbing for scenario (f-B) per D5 closure

**Files:**
- Create: `test/fixtures/0027-http-lua-full-bridge/certs/` (NEW directory; cert + key per D5 closure)
- Possibly modify: existing cert paths if reusing from prior fixture
- Append: PROGRESS.md (Task 17 entry per D-P3)

Lands the TLS cert + key for fixture-0027 scenario (f-B) per D5 closure at this PLAN session. Subagent at IMPL chooses between REUSE existing TLS cert from `test/fixtures/<prior-fixture-with-tls>/certs/` (e.g., phase-03 TLS fixtures or phase-18.x fixtures with TLS-handshake topology) OR generate new minimal cert via `openssl req -x509 -newkey rsa:2048 -nodes -days 36500 -subj '/CN=fixture-0027' -addext 'subjectAltName=DNS:fixture-0027.example.com'`. Cross-side cert presented on both reference Envoy + envoy-go must have IDENTICAL DER encoding so `:sha256PeerCertificateDigest()` returns identical 32-byte hex on both. **PARALLELIZABLE with Task 18.**

**Acceptance:** `test/fixtures/0027-http-lua-full-bridge/certs/` directory present with cert + key; cert + key valid (openssl x509 -in cert.pem -text returns valid output); SAN list documented at fixture README.md.

**Subagent dispatch outline:**

> Author Task 17 per 22.2 PLAN Task 17 + 22.2 SPEC §8.3 + D5 closure (option f-B cert-fingerprint-only). Subagent surveys existing test/fixtures/ for TLS-handshake-topology fixtures (e.g., phase-03 TLS fixtures); evaluates whether existing cert can be REUSED (symlink to fixture-0027/certs/ or copy). If no suitable existing cert: generate new minimal cert via openssl (cmd above). Document cert SAN list + serial in README.md (Task 18). The matching key.pem stays at fixture-0027/certs/; both reference envoy.yaml + envoy-go.yaml mount the same cert+key via volume mount in the differential harness's container topology.

- [ ] **Step 1: Survey existing cert fixtures** + decide REUSE vs NEW.

- [ ] **Step 2: Author/copy cert + key files into certs/ subdirectory.**

- [ ] **Step 3: Verify cert validity** — `openssl x509 -in certs/cert.pem -text -noout` → expect valid output.

- [ ] **Step 4: Append PROGRESS.md Task 17 entry per D-P3 + cert SAN + serial verbatim.**

- [ ] **Step 5: Commit**

```bash
git commit -m "phase 22.2 Task 17: fixture-0027 TLS cert for scenario (f-B) per D5 closure

Lands TLS cert + key at test/fixtures/0027-http-lua-full-bridge/certs/
per 22.2 PLAN D5 closure (option f-B cert-fingerprint-only). Reference
Envoy + envoy-go present IDENTICAL cert; both call :sha256PeerCertificate
Digest() and emit identical 32-byte hex (cross-side CompareBytes).

<REUSE from <prior-fixture> | NEW minimal cert via openssl>. SAN list +
serial documented at fixture README.md (Task 18)."
```

---

## Task 18: fixture-0027 directory + 13 scripts + YAMLs + driver + R11 REUSE disposition

**Files:**
- Create: `test/fixtures/0027-http-lua-full-bridge/README.md` (~250-400 LoC)
- Create: `test/fixtures/0027-http-lua-full-bridge/envoy.yaml` (~400-600 LoC)
- Create: `test/fixtures/0027-http-lua-full-bridge/envoy-go.yaml` (~400-600 LoC)
- Create: `test/fixtures/0027-http-lua-full-bridge/expectations.yaml` (~200-350 LoC)
- Create: `test/fixtures/0027-http-lua-full-bridge/inputs/driver.go` (~800-1200 LoC)
- Create: `test/fixtures/0027-http-lua-full-bridge/scripts/{a_body_whole,b_body_chunks,c_trailers,d_metadata_empty,e_dynamic_metadata,f_connection_ssl_fp,g_crypto,h_filebytes,i_streaminfo_upstream,j_httpcall_sync,k_httpcall_async,l_timestamp,m_filterstate}.lua` (5-20 LoC each)
- Append: PROGRESS.md (Task 18 entry per D-P3 + fixture-0027 fixture-count verification 28 → 29)

Lands fixture-0027 directory + 13 .lua scripts + YAMLs + driver per SPEC §8 + D5 closure (option f-B for scenario (f)) + D-P11 closure (REUSE existing `runReferenceLessFixture` pattern for non-deterministic scenarios). NO NEW driver-helper added per D-P11. Fixture-0027 green-light deferred to Task 19. **PARALLELIZABLE with Task 17.**

**Acceptance:** `go build ./test/...` clean; fixture-0027 directory structure populated; 13 .lua scripts present; multi-listener YAML topology (plaintext listeners for scenarios a/b/c/d/e/g/h/i + 1 TLS listener for scenario f-B); driver implements `Driver` interface; per-scenario probes registered; classifies a-i as deterministic cross-side `CompareBytes` + j-m as REFERENCE-LESS subject-only + h (per D8 reclassification) also REFERENCE-LESS subject-only.

**Subagent dispatch outline:**

> Author Task 18 per 22.2 PLAN Task 18 + 22.2 SPEC §8 + D5 closure + D-P11 closure. Driver mirrors fixture-0026 pattern from 22.1 (registered Driver + per-scenario probes via driveProxy + emitScenario + classifyBody). Scenarios a-g (per D5 (f-B) cert-fingerprint subset + D8 fileBytes-reclassification): cross-side CompareBytes byte-exact for a/b/c/d/e/f (f-B fingerprint subset only)/g/i (9 scenarios). REFERENCE-LESS subject-only: j (httpCall sync) + k (httpCall async) + l (timestamp) + m (filterState) + h (fileBytes per D8 envoy-go-strict — reference Envoy cannot run `:fileBytes` script). Total 9 cross-side + 4 REFERENCE-LESS = 13 scenarios. envoy.yaml + envoy-go.yaml: multi-listener topology (1 listener per scenario + 1 TLS listener for scenario f). expectations.yaml: per-scenario expected behavior verbatim. README.md: fixture overview + 13-scenario table + topology diagram + cross-refs to SPEC §8 + ADR-0190/0191/0192. REUSE existing runReferenceLessFixture per D-P11; NO NEW driver-helper. Fixture-0027 green-light deferred to Task 19 (depends on Tasks 7-13 bridge surfaces all landed cleanly + Tasks 14-16 stats/race/fuzz green).

- [ ] **Step 1: Author 13 .lua script files** verbatim per File-structure table rows (a-m).

- [ ] **Step 2: Author envoy.yaml + envoy-go.yaml** (multi-listener topology; cert mount for scenario f).

- [ ] **Step 3: Author expectations.yaml + README.md + inputs/driver.go.**

- [ ] **Step 4: Verify** `go build ./test/...` clean.

- [ ] **Step 5: Smoke-run fixture-0027** at Task 18 commit time — `go test -count=1 ./test/differential -run TestDifferential/0027` (expect FAIL — bridge surfaces from Tasks 7-13 are GREEN but the full fixture green-light is deferred to Task 19 atomic landing per per-task TDD ordering at fixture-level).

- [ ] **Step 6: Append PROGRESS.md Task 18 entry per D-P3** (note fixture green-light deferred to Task 19; document smoke-run outcome verbatim per `superpowers:verification-before-completion`).

- [ ] **Step 7: Commit**

```bash
git commit -m "phase 22.2 Task 18: fixture-0027 directory + 13 scripts + driver

Lands test/fixtures/0027-http-lua-full-bridge/ per SPEC §8 + D5 closure
(option f-B cert-fingerprint-only) + D-P11 closure (REUSE existing
runReferenceLessFixture; NO NEW driver-helper). 13 scripts a-m: 9
deterministic cross-side (a/b/c/d/e/f-B/g/i + h pending D8 reclassification)
+ 4 REFERENCE-LESS subject-only (j/k/l/m + h per D8 envoy-go-strict).

Fixture green-light deferred to Task 19 atomic landing (depends on all
bridge surfaces from Tasks 7-13 + stats/race/fuzz from Tasks 14-16)."
```

---

## Tier F — Atomic landing (Task 19)

## Task 19: BEHAVIOR_CONTRACT.md 15-edit bundle + ADR-0190/0191/0192 §Decision+§Consequences + ADR-0177 IN-PLACE AMENDMENT + conditional ADR-0193 + STATE.md re-advance + ROADMAP row 22.2 IMPL-done + REVIEW.md [ADR-0190 + ADR-0191 + ADR-0192 + AMEND-ADR-0177 + (cond ADR-0193)]

**Files:**
- Modify: `docs/envoy-go/BEHAVIOR_CONTRACT.md` (15-edit bundle per §14 + D8 PLAN-scrape outcome; ~+500-800 LoC)
- Modify: `docs/envoy-go/DECISIONS.md` (3 ADR §Decision + §Consequences bodies + ADR-0177 IN-PLACE AMENDMENT body + conditional ADR-0193 if fires; ~+600-1100 LoC)
- Modify: `docs/envoy-go/STATE.md` (rewrite-in-place per BOOTSTRAP §4.1 invariant 1)
- Modify: `docs/envoy-go/ROADMAP.md` (row 22.2 `in-progress → done` + per-cell IMPL-done annotation per ADR-0106; parent row `22` UNCHANGED `in-progress`; sub-row `22.3` UNCHANGED `planned`)
- Create: `docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/REVIEW.md` (~400-500 LoC per `superpowers:requesting-code-review`)
- Append: PROGRESS.md (final Task 19 entry; 6-gate outputs captured verbatim)

Atomic landing per ADR-0052 + SPEC §14 15-edit bundle + 3 NEW ADR §Decision + §Consequences body landings + 1 IN-PLACE AMENDMENT on ADR-0177 + conditional ADR-0193 (if §13-R6 OR §13-R9 fires) + STATE/ROADMAP advance + REVIEW.md authoring. **DEPENDS on Tasks 1-18.**

**Subagent dispatch outline (per D-P2 `general-purpose` with explicit reference to 22.2 SPEC §15 + §14 + ADR-0190 + ADR-0191 + ADR-0192 + ADR-0177 IN-PLACE AMENDMENT body shape + REVIEW.md authoring via `superpowers:requesting-code-review`):**

> Author Task 19 per 22.2 PLAN Task 19 + 22.2 SPEC §15 25-item acceptance checklist + §14 15-edit bundle anatomy (10 baseline + D8 4 conditional crypto/fileBytes records + 2 :filterState records = 15 edits at 22.2 IMPL atomic landing per D8 PLAN-scrape (1+1+5+2+1+4+1 = 15)) + ADR-0190 §Decision body sketch (NEW internal/dynamicmetadata/ — Bucket type + 4 methods + per-stream lifecycle + nil-tolerance + cross-phase deferral-lift expectation) + ADR-0191 §Decision body sketch (NEW internal/lua/ 22.2 API extensions — coroutine NewThread/Resume/YieldFromBridge + BodyBuffer interface + per-stream child-LState lifecycle + Q10 strict scope — NEW ADR not in-place AMEND on ADR-0188) + ADR-0192 §Decision body sketch (NEW internal/filter/http/lua/ 22.2 package shape extensions — 8 bridge surface families + 7+4 envoy-go-strict departure records per D8 PLAN scrape + 5 NEW stat counters + 2 :filterState records + 3 runtime-reject arms 20-22 + FilterChain.tlsConnectionState field extension + fixture-0027 mixed-mode discipline) + ADR-0177 IN-PLACE AMENDMENT body shape (NEW Client.ClusterDispatch + NEW FactoryCtx.ClusterManager field + R5 ratified) + REVIEW.md authoring per `superpowers:requesting-code-review` covering 25-item §15 acceptance checklist closure + per-task review notes + cross-cutting review notes + green-light evidence + D-decision-disposition record + 6-gate outputs verbatim. 6 phase-done gates A/B/C/D/E/F outputs captured verbatim in PROGRESS.md final Task 19 entry per `superpowers:verification-before-completion`.

**Steps (15 total):**

- [ ] **Step 1: Gate A — build** (`go build ./...` clean; capture verbatim).

- [ ] **Step 2: Gate B — vet + lint** (`go vet ./...` + `golangci-lint run` clean; no new suppressions; capture verbatim).

- [ ] **Step 3: Gate C — race** (`go test -race -count=1 ./...` clean per D-P9 race-scoping; capture verbatim).

- [ ] **Step 4: Gate D — differential + cross-package regression matrix per D-P9** (`go test -count=1 ./test/differential/ -run 'Test.*00(0[0-9]|1[0-9]|2[0-7])'` clean — all 29 fixture directories green including the new 0027; capture verbatim).

- [ ] **Step 5: Gate E — fuzz** (`go test -fuzz=FuzzLuaBodyBridge -fuzztime=30s ./internal/filter/http/lua/` + `go test -fuzz=FuzzLuaHTTPCallConfig -fuzztime=30s ./internal/filter/http/lua/` clean; verify project-wide fuzzer count = 30 via find/grep oneliner; 28 pre-existing fuzzers re-run clean at 30s per seed).

- [ ] **Step 6: Gate F — h2spec** (`make test-h2spec` 53/53 PASS at ADR-0051 pin; capture verbatim).

- [ ] **Step 7: Author BEHAVIOR_CONTRACT.md 15-edit bundle** per SPEC §14 + D8 PLAN-scrape outcome (10 baseline items 1-10 + item 12 D8 disposition paragraph + 4 NEW conditional crypto/fileBytes departure records per D8 PLAN closure — `:sha256` + `:sha512` + `:base64Decode` + `:fileBytes` envoy-go-strict).

- [ ] **Step 8: Author ADR-0190 + ADR-0191 + ADR-0192 §Decision + §Consequences bodies in DECISIONS.md** per the per-ADR sketches in Task 19 subagent dispatch outline above. Set `Status: Accepted` + `Date: <impl-date>` + `Lands-in: Task 19`.

- [ ] **Step 9: Author IN-PLACE AMENDMENT body on ADR-0177** documenting `Client.ClusterDispatch` + `FactoryCtx.ClusterManager` per Task 4's landing + R5 RATIFIED closure paragraph.

- [ ] **Step 10: Evaluate conditional ADR-0193 landing via R6 + R9 signaling protocols.** Run `grep -n '§13-R6 disposition: ADR-0193 FIRES' docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (greps Task 15's R6 sentinel) AND `grep -n '§13-R9 disposition: ADR-0193 FIRES' docs/envoy-go/phases/22.2-http-filter-lua-full-bridge/PROGRESS.md` (greps Task 7's R9 sentinel). IF EITHER grep returns a match: author ADR-0193 §Context + §Decision + §Consequences body per ADR-0044 (R6 firing → "per-script-source `*LState` pool with chunk-pre-loaded entries" decision; R9 firing → "body-bridge buffer seam separation from ADR-0192 with its own §Decision body"; if BOTH fire → single combined ADR-0193 covering both). IF NEITHER grep returns a match: ADR-0193 stays UNCONSUMED; carries forward to 22.3 BRAINSTORM as escape-valve slot. Document the grep results verbatim in the Task 19 PROGRESS.md entry per `superpowers:verification-before-completion`.

- [ ] **Step 11: Update STATE.md** to post-phase-22.2-IMPL state per BOOTSTRAP §4.1 invariant 1 (active-phase: 22-http-filter-lua parent stays in-progress; lifecycle-state: "phase 22.2 IMPL done; awaiting 22.3 BRAINSTORM"; next-skill: `superpowers:brainstorming` scoped to 22.3; last-commit: `<TBD — SHA-fill follow-up after squash-merge>` placeholder; last-updated: today's date; next-free ADR: ADR-0193 UNCHANGED if neither R6 nor R9 fires OR ADR-0194 if one fires; stat-count delta 102 → 107; fuzzer count 28 → 30; fixture directory count 28 → 29; 17 HTTP filters wired UNCHANGED; verbose summary).

- [ ] **Step 12: Update ROADMAP.md row 22.2** (`in-progress → done` + per-cell IMPL-done annotation per ADR-0106 documenting 19-task landing + 6-gate outputs + SIXTEENTH §9 family-row milestone (sub-phase row count — the §9 row count UNCHANGED at 17 wired filters; the lua filter SAME consumer; 22.2 sub-row done) + 3 NEW ADR landings + 1 IN-PLACE AMENDMENT + conditional ADR-0193 disposition + 25-item §15 acceptance checklist closure + D3+D5+D8 closure record + R5/R6/R7/R8/R9/R10/R11/W2 disposition record; parent row 22 STAYS in-progress; sub-row 22.3 UNCHANGED planned).

- [ ] **Step 13: Author REVIEW.md** per `superpowers:requesting-code-review` — ~400-500 LoC covering 6-gate outputs verbatim + 22.2 SPEC §15 25-item checklist verification with cite-to-PROGRESS-entry per item + D3 + D5 + D8 + D-P1..D-P11 PLAN-time decision-disposition record (which decisions HELD, which were AMENDED at IMPL) + R6 disposition from Task 15 (STANDS WEAK-default OR ADR-0193 escape-valve FIRED) + R9 cross-check from Task 7 (STAYS embedded in ADR-0192 OR ADR-0193 FIRED) + next-phase handoff state (22.3 BRAINSTORM scope hand-off).

- [ ] **Step 14: Append final PROGRESS.md Task 19 entry** with all 6 gate outputs verbatim + 22.2 SPEC §15 25-item closure checklist + D-decision disposition status.

- [ ] **Step 15: Verify nothing left uncommitted** (`git status --porcelain` empty) + **Commit** (Task 19 final IMPL-worktree commit).

**Commit message** (long; abbreviated; see full at the phase-done squash-merge section below):

```bash
git commit -m "phase 22.2 Task 19: atomic landing + 6-gate phase-done verification

All 6 phase-done gates GREEN: A build / B vet+lint / C race / D differential
(29/29 fixture directories incl. new 0027 — 9 scenarios cross-side
byte-exact + 4 REFERENCE-LESS subject-only) / E fuzz (30 fuzzers clean;
30th FuzzLuaHTTPCallConfig + 29th FuzzLuaBodyBridge confirmed) / F h2spec
53/53 PASS.

22.2 SPEC §15 25-item acceptance checklist all GREEN. D3 + D5 + D8 closures
recorded at this PLAN session. D-P1..D-P11 decision-disposition recorded
at IMPL. D-P10 R6 gate: <STANDS WEAK-default | ADR-0193 escape-valve FIRED
with §Decision + §Consequences body landed at this commit>.

BEHAVIOR_CONTRACT.md 15-edit bundle landed atomically per ADR-0052 + SPEC §14.
3 NEW ADRs anchored: ADR-0190 NEW internal/dynamicmetadata/ framework
primitive + ADR-0191 NEW internal/lua/ 22.2 API extensions (coroutine +
BodyBuffer per Q10 strict scope) + ADR-0192 NEW internal/filter/http/lua/
22.2 package shape extensions (8 bridge surface families + 11 envoy-go-
strict departure records per D8 PLAN scrape + FilterChain.tlsConnectionState
field). 1 IN-PLACE AMENDMENT on ADR-0177 (Client.ClusterDispatch + Factory
Ctx.ClusterManager; R5 RATIFIED).

Stat surface 102 → 107 (+5 envoy-go-strict counters). Fixture directories
28 → 29. Fuzzer count 28 → 30. 17 HTTP filters wired UNCHANGED. ADR
tail advance to ADR-0192 (or ADR-0193 if escape-valve fires)."
```

---


## Phase-done squash-merge + push to origin

After Task 19 completes:

1. **Squash-merge to master** (from the master worktree):

```bash
cd /home/esa/git/envoy-go  # the master worktree
git merge --squash phase-22.2-http-filter-lua-full-bridge-impl
# Resolve commit message — body must include the 19-task summary + the 3-NEW-ADR
# (+ 1-IN-PLACE-AMENDMENT on ADR-0177 + CONDITIONAL ADR-0193 if R6 or R9 fired) roster
# + the closes-row-22.2 + parent-row-22-STAYS-in-progress note + the SIXTEENTH-§9-row
# milestone (sub-phase row count — the §9 row count of WIRED FILTERS UNCHANGED at 17;
# the lua filter SAME consumer with full bridge surface delta).
git commit -m "$(cat <<'EOF'
Squash merge phase-22.2-http-filter-lua-full-bridge-impl

Closes ROADMAP row 22.2 (in-progress → done). Parent row 22 STAYS
in-progress until 22.3's phase-done per parent SPEC §1 closure pattern +
ADR-0106 sub-row rollup discipline + phase-18.1/18.2 + phase-19.1/19.2
precedent. Sub-row 22.3 UNCHANGED planned.

19 tasks landed across 6 Tiers (Tier A framework primitives Tasks 1-5;
Tier B HCM dispatch wire-in Task 6; Tier C bridge surfaces Tasks 7-13
7-way parallelizable; Tier D stats + race + fuzz Tasks 14-16 3-way
parallelizable; Tier E differential fixture Tasks 17-18 2-way; Tier F
atomic landing Task 19). 3 NEW ADRs anchored:

ADR-0190 NEW internal/dynamicmetadata/ framework primitive — per-stream
*Bucket accessor for cross-filter dynamic-metadata read+write at first
co-consumer (HTTP Lua filter 22.2's :streamInfo():dynamicMetadata() +
:dynamicTypedMetadata(filter_name)) per phase-22 BRAINSTORM Q3 cross-phase-
deferral-break + Q9 EXTRACT-NOW + 22.2 SPEC §3.1 production signatures.
THIRD §9 framework primitive in two-phase succession (after ADR-0188 +
ADR-0189 at 22.1 IMPL). Cross-phase deferral-lift expectation: phases
16/17/18/19/20's BEHAVIOR_CONTRACT.md "deferred" notes carry forward
AS-IS until their respective next-touchpoint phases.

ADR-0191 NEW internal/lua/ 22.2 API extensions for coroutine yield/resume
+ body-bridge buffer seam at HTTP filter Lua consumer-#1 scope-expansion
per phase-22.2 BRAINSTORM Q1 + Q10 strict scope (NEW ADR not in-place
AMEND on ADR-0188 — ADR-0188's EXPLICIT API-REVISION ALLOWANCE clause
STAYS scoped to consumer-#2 per phase-20 ADR-0177 §Future Work CLOSURE-
AT-PHASE-20 precedent direction-reverse) + 22.2 SPEC §3.2 production
signatures + §11.1 D2 closure (gopher-lua native LState.NewThread/Yield/
Resume) + §11.3 D3 RECOMMENDED option (a) BodyBuffer interface seam.

ADR-0192 NEW internal/filter/http/lua/ 22.2 package shape extensions —
body + trailers + metadata + connection-SSL + httpCall + crypto +
fileBytes + timestamp + streamInfo-full + filter-state in-package bridge
methods + 5 NEW envoy-go-strict stat counters + 2 NEW envoy-go-strict
:filterState divergences (per AMEND-22.2-4) + 4 NEW envoy-go-strict
crypto/fileBytes departure records (per D8 PLAN-scrape closure at 22.2
PLAN session — :sha256/:sha512/:base64Decode/:fileBytes envoy-go-strict;
:importPublicKey/:verifySignature upstream-parity with calling-convention
mimicry of upstream PublicKeyWrapper userdata return scope) + cross-phase
dynamic-metadata deferral-lift expectation (consumer-#1 of ADR-0190) +
fixture-0027 mixed-mode discipline + FilterChain.tlsConnectionState field
extension (lives inside this ADR per Q13 WEAK HOLD; no separate ADR for
chain-side extension).

1 IN-PLACE §Decision AMENDMENT on ADR-0177 (NEW Client.ClusterDispatch +
NEW FactoryCtx.ClusterManager field — first co-consumer of phase-20's
internal/httpclient/ primitive at 22.2's :httpCall() bridge; R5 RATIFIED).

<+ CONDITIONAL ADR-0193 if D-P10 R6 fired (Task 15 benchmark > 1ms)
OR §13-R9 fired (Task 7 body-bridge IMPL surfaced enough complexity):
per-script-source *LState pool with chunk-pre-loaded entries OR body-
buffer-seam-with-ADR-0128 separation; §Context + §Decision + §Consequences
all landed at this commit>.

29th + 30th project-wide fuzzers FuzzLuaBodyBridge + FuzzLuaHTTPCallConfig
clean at 30s per ADR-0018. 29/29 differential fixture directories GREEN
(0000-0027; 9 deterministic cross-side scenarios + 4 REFERENCE-LESS
subject-only scenarios for fixture-0027). All 6 phase-done gates GREEN.
22.2 SPEC §15 25-item acceptance checklist all GREEN.

NO new §9 family-row at 22.2 (the lua filter SAME consumer; 22.2 extends
its bridge surface). 17 HTTP filters wired UNCHANGED. Stat surface
102 → 107 names per SPEC §7 (+5 envoy-go-strict counters: httpcall_total +
httpcall_failures SYNC-ONLY + httpcall_timeouts SYNC-ONLY + body_buffered
_bytes_total + coroutine_yields_total; HCM-rooted template UNCHANGED from
22.1 per AMEND-2). 7-11 envoy-go-strict departures documented per SPEC §14
(7 baseline: 5 counters + 2 :filterState; +4 D8-classified crypto/fileBytes:
:sha256 + :sha512 + :base64Decode + :fileBytes; = 11 total at 22.2 IMPL).

D1 + D2 + D4 + D6 + D7 closed at 22.2 SPEC §11. D3 + D5 + D8 closed at
22.2 PLAN session per SPEC §12 RECOMMENDED + empirical scrape (D8 ran
WebFetch against upstream Envoy v1.37.2 source — 2/6 methods upstream-
parity; 4/6 envoy-go-strict). D-P1..D-P11 PLAN-emerged decisions
disposition recorded at REVIEW.md. R5 RATIFIED at Task 4 (httpclient
co-consumer validation). R6 disposition: <STANDS WEAK-default at
ns/op < 1ms | ADR-0193 escape-valve FIRED>. R7 + R8 closed at PLAN
session (D8 empirical scrape). R9 cross-check vs Task 7 IMPL: <STAYS
embedded in ADR-0192 | ADR-0193 FIRED>. R10 closed at Task 16
(30 fuzzers). R11 closed at D-P11 (REUSE existing runReferenceLessFixture
pattern). W2 closed at Tasks 7+11+12 (runtime-reject arms 20-22 byte-stable
wording pinned).

§13-R5 (httpclient co-consumer) RATIFIED. §13-R6 (LState-pool benchmark)
<STANDS WEAK-default | escape-valve FIRED>. §13-R7 (crypto methods
upstream-exposure) CLOSED at PLAN scrape: 2 upstream-parity (importPublicKey
+ verifySignature) + 3 envoy-go-strict (sha256 + sha512 + base64Decode).
§13-R8 (fileBytes upstream-exposure) CLOSED at PLAN scrape: envoy-go-strict.
§13-R9 (body-buffer-seam-with-ADR-0128 separation) <STAYS embedded in
ADR-0192 | ADR-0193 FIRED>. §13-R10 (fuzzer count) CLOSED at 30. §13-R11
(REFERENCE-LESS driver helper) CLOSED at D-P11 REUSE. §13-W2 (arms 20-22
wording) CLOSED at Tasks 7+11+12.
EOF
)"
```

2. **SHA-fill follow-up** (per the phase-09..22.1 convention):

```bash
# Update STATE.md last-commit field with the real squash SHA (was TBD at Task 19):
# Edit docs/envoy-go/STATE.md replacing "<TBD — SHA-fill follow-up after squash-merge>"
# with the actual squash commit SHA from `git log -1 --format=%H master`.
git add docs/envoy-go/STATE.md
git commit -m "phase 22.2 IMPL follow-up: STATE.md SHA-fill (TBD → <squash SHA> post-squash)"
```

3. **Push to origin** (per project memory `feedback_push_to_origin.md` — always-push-to-origin without asking):

```bash
git push origin master
```

4. **Worktree cleanup** (optional but tidy):

```bash
git worktree remove /home/esa/git/envoy-go/.worktrees/phase-22.2-http-filter-lua-full-bridge-impl
# Keep the branch alive for reference; do NOT delete unless cleanup is explicit
# (matches phase-09..22.1 worktree-cleanup convention)
```

---

## Remember

- **Exact paths always.** All file paths in this PLAN are absolute relative to the repo root (`/home/esa/git/envoy-go/`). Tasks reference paths verbatim.
- **Skill @-references.** Per-task subagent dispatch uses `superpowers:subagent-driven-development` (project memory `feedback_execution_style.md`). TDD discipline per `superpowers:test-driven-development` (RIGID for Tasks 1-17; relaxed for Tasks 18-19). Verification per `superpowers:verification-before-completion` (commands quoted verbatim into PROGRESS.md). Code review per `superpowers:requesting-code-review` (REVIEW.md at Task 19 Step 13). Git worktrees per `superpowers:using-git-worktrees` + project memory `feedback_git_worktrees.md` (always-worktrees; branch base from master tip post-PLAN-SHA-fill).
- **DRY, YAGNI, TDD, frequent commits.** Each Task lands as ONE commit (per `superpowers:writing-plans` discipline + phase-09..22.1 precedent). NO mid-task partial commits. NO half-finished implementations.
- **Always push to origin** after clean local squash-merge + SHA-fill follow-up on master per project memory `feedback_push_to_origin.md`.

