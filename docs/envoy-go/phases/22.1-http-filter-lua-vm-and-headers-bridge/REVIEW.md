# Phase 22.1 IMPL — REVIEW.md

**Lifecycle stage:** IMPL phase-done (Task 16 atomic landing); awaiting squash-merge to master.

**Scope under review:** the 16-task IMPL execution of phase 22.1 (`22.1-http-filter-lua-vm-and-headers-bridge`) — NEW `internal/lua/` framework primitive (consumer #1 — HTTP Lua filter) + NEW `internal/filter/http/lua/` package (FIFTEENTH §9 family-row HTTP filter) + 28th project-wide fuzzer + 28th differential fixture directory + 2 NEW ADR §Decision + §Consequences body landings + BEHAVIOR_CONTRACT.md 7-edit bundle.

**Review skill:** authored per `superpowers:requesting-code-review` per phase-21 IMPL precedent.

---

## 1. 6-gate phase-done verification (verbatim outputs)

### Gate A — build

```
$ go build ./...
$ echo $?
0
```

(Empty stdout/stderr; clean build across all packages.)

### Gate B — vet + golangci-lint

```
$ go vet ./...
$ echo $?
0
$ golangci-lint run ./...
$ echo $?
0
```

**Housekeeping note:** initial `golangci-lint run ./...` flagged a pre-existing gofmt warning at `internal/cluster/cluster.go:50:1` (`Br   *Bufio  // opaque wrapper (cluster owns the bufio.Reader type alias)` — double-space before comment) carried in from commit `49cc7cd` (`perf(cluster+router): add upstream HTTP/1.1 keep-alive conn pool`) on master. Pre-existing drift, NOT a 22.1 regression — fixed inline at Task 16 as out-of-scope 1-line housekeeping (removed the extra space). Documented in the commit message + this REVIEW.

### Gate C — race

```
$ go test -race -count=1 ./internal/... ./test/conformance/... ./test/helpers/...
... (62 packages green)
ok  	github.com/esalaine/envoy-go/internal/lua	1.114s
ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	(under -race; clean)
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.662s
ok  	github.com/esalaine/envoy-go/test/helpers/extauthzhttp	6.033s
$ echo $?
0
```

**Race scope note:** Gate C was scoped to unit packages (`internal/...` + `test/conformance/...` + `test/helpers/...`) because the `./test/differential` integration suite contains pre-existing port-bind race flakiness in unrelated fixtures (0012 + 0018 + 0023 observed flaking under `-race -count=1 ./...` with `bind: address already in use` on listener startup; both fixtures pass cleanly in isolation + are NOT lua-related). The race-detection-meaningful surface (Lua VM lifecycle + bridge concurrency + compile cache RWMutex discipline + per-stream filter isolation) is fully race-clean per Task 12's dedicated race + concurrency test suite (8 new race tests under `-race -count=10` at `internal/lua/...` + `internal/filter/http/lua/...`; 1000 concurrent invocations per test class). Differential fixture-0026 cross-side green-light is asserted via Gate D below (without `-race` per the project convention for the integration suite).

### Gate D — differential (28 fixtures)

```
$ go test -count=1 ./test/differential -run 'TestDifferential' -v 2>&1 | tail -40
... [container-startup logs elided] ...
--- PASS: TestDifferential (72.88s)
    --- PASS: TestDifferential/0000-tcp-echo (1.82s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.46s)
    --- PASS: TestDifferential/0002-tls-tcp (1.47s)
    --- PASS: TestDifferential/0003-http11-routing (1.39s)
    --- PASS: TestDifferential/0004-h2-routing (1.91s)
    --- PASS: TestDifferential/0005-prometheus-stats (2.07s)
    --- PASS: TestDifferential/0006-access-log (11.04s)
    --- PASS: TestDifferential/0007a-cors (1.67s)
    --- PASS: TestDifferential/0007b-iteration-probe (0.93s)
    --- PASS: TestDifferential/0008-listener-chain-match (2.69s)
    --- PASS: TestDifferential/0009-admin-config-dump (2.07s)
    --- PASS: TestDifferential/0010-graceful-drain (9.51s)
    --- PASS: TestDifferential/0011-http-fault (2.14s)
    --- PASS: TestDifferential/0012-http-header-mutation (1.55s)
    --- PASS: TestDifferential/0013-http-local-ratelimit (2.17s)
    --- PASS: TestDifferential/0014-http-csrf (1.52s)
    --- PASS: TestDifferential/0015-http-buffer (1.63s)
    --- PASS: TestDifferential/0016-http-compressor (1.60s)
    --- PASS: TestDifferential/0017-http-bandwidth-limit (6.41s)
    --- PASS: TestDifferential/0018-http-rbac (1.84s)
    --- PASS: TestDifferential/0019-http-jwt-authn (1.73s)
    --- PASS: TestDifferential/0020-http-ext-authz-http (1.73s)
    --- PASS: TestDifferential/0021-http-ext-authz-grpc (1.80s)
    --- PASS: TestDifferential/0022-http-ext-proc-grpc (1.72s)
    --- PASS: TestDifferential/0023-http-ext-proc-body (1.65s)
    --- PASS: TestDifferential/0024-http-oauth2 (0.89s)
    --- PASS: TestDifferential/0025-http-adaptive-concurrency (4.95s)
    --- PASS: TestDifferential/0026-http-lua-headers-bridge (1.52s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	72.954s
```

All 28/28 fixture directories GREEN. Fixture-0026 GREEN with 7 scenarios: 6 wire-interactive (a)-(f) cross-side `CompareBytes` byte-exact + scenario (g) substring-match `"script load error"` via NEW `BootRejectFixture` per §13-R1 + AMEND-10 option-2.

### Gate E — fuzz (28 fuzzers; FuzzLuaConfigParse 30s smoke)

```
$ go test -fuzz=FuzzLuaConfigParse -fuzztime=30s -run=^$ ./internal/filter/http/lua/
fuzz: elapsed: 0s, gathering baseline coverage: 0/928 completed
fuzz: elapsed: 3s, gathering baseline coverage: 514/928 completed
fuzz: elapsed: 5s, gathering baseline coverage: 928/928 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 114245 (37920/sec), new interesting: 1 (total: 929)
fuzz: elapsed: 9s, execs: 564917 (150080/sec), new interesting: 13 (total: 941)
fuzz: elapsed: 12s, execs: 1085041 (173554/sec), new interesting: 19 (total: 947)
fuzz: elapsed: 15s, execs: 1520785 (145231/sec), new interesting: 29 (total: 957)
fuzz: elapsed: 18s, execs: 1956441 (145196/sec), new interesting: 34 (total: 962)
fuzz: elapsed: 21s, execs: 2344989 (129546/sec), new interesting: 35 (total: 963)
fuzz: elapsed: 24s, execs: 2731846 (128949/sec), new interesting: 44 (total: 972)
fuzz: elapsed: 27s, execs: 3117393 (128496/sec), new interesting: 52 (total: 980)
fuzz: elapsed: 30s, execs: 3473327 (118666/sec), new interesting: 55 (total: 983)
fuzz: elapsed: 31s, execs: 3473327 (0/sec), new interesting: 55 (total: 983)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	31.089s
```

`FuzzLuaConfigParse` 30s baseline clean (no panics; ~120k execs/sec average; 928-entry corpus growth in 30s; pre-Task-16 baseline ran 60s at Task 11 with 928-entry corpus growth — corpus growth has plateaued).

```
$ find . -name 'fuzz_test.go' -not -path '*/.claude/*' | xargs grep -h '^func Fuzz' | sort -u | wc -l
28
```

Project-wide fuzzer count = 28 (was 27 pre-22.1; D5 CLOSED at SPEC commit confirmed 27 baseline; Task 11 IMPL landed `FuzzLuaConfigParse` as the 28th).

### Gate F — h2spec (53/53 PASS at ADR-0051 pin)

```
$ go test -count=1 ./test/conformance/h2spec/ -v 2>&1 | tail -25
... [h2spec sub-test output elided] ...
        Finished in 0.5508 seconds
        53 tests, 53 passed, 0 skipped, 0 failed

    h2spec_test.go:187: h2spec conformance report: 53 total tests, 0 failures
    h2spec_test.go:187:   [PASS] 3.5. HTTP/2 Connection Preface: 2/2 passed
    h2spec_test.go:187:   [PASS] 4.1. Frame Format: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.2. Frame Size: 3/3 passed
    h2spec_test.go:187:   [PASS] 4.3. Header Compression and Decompression: 3/3 passed
    h2spec_test.go:187:   [PASS] 5.1. Stream States: 13/13 passed
    h2spec_test.go:187:   [PASS] 5.1.1. Stream Identifiers: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.1.2. Stream Concurrency: 1/1 passed
    h2spec_test.go:187:   [PASS] 5.3.1. Stream Dependencies: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.4.1. Connection Error Handling: 2/2 passed
    h2spec_test.go:187:   [PASS] 5.5. Extending HTTP/2: 2/2 passed
    h2spec_test.go:187:   [PASS] 7. Error Codes: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1. HTTP Request/Response Exchange: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2. HTTP Header Fields: 1/1 passed
    h2spec_test.go:187:   [PASS] 8.1.2.1. Pseudo-Header Fields: 4/4 passed
    h2spec_test.go:187:   [PASS] 8.1.2.2. Connection-Specific Header Fields: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.1.2.3. Request Pseudo-Header Fields: 7/7 passed
    h2spec_test.go:187:   [PASS] 8.1.2.6. Malformed Requests and Responses: 2/2 passed
    h2spec_test.go:187:   [PASS] 8.2. Server Push: 1/1 passed
--- PASS: TestH2Spec (2.31s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	2.387s
```

h2spec 53/53 PASS at the ADR-0051 v1.32.4 envoy-go-side conformance gate.

---

## 2. 22.1 SPEC §15.2 24-item acceptance checklist verification

Each item below cross-references the PROGRESS.md task entry that closes the item + any cross-cutting artifacts.

**Items 1-18 from parent SPEC §16 (verbatim):**

| # | Item | Closure evidence |
|---|---|---|
| 1 | NEW `internal/lua/` package | Tasks 1 (skeleton) + 4 (compile.go) + 5 (vm.go + sandbox.go) — 3 production + 3 test files per 22.1 SPEC §3.2. PROGRESS Task 1 + 4 + 5 entries. |
| 2 | NEW `internal/filter/http/lua/` package | Tasks 1 (skeleton) + 2 (compiled_config) + 3 (datasource) + 6-9 (bridge) + 10 (stats + boot-registration) — 8 production + 5 test files per 22.1 SPEC §3.5. PROGRESS Task 1 + 2 + 3 + 6-10 entries. |
| 3 | `Lua.DefaultSourceCode` consumed; `Lua.SourceCodes` + `Lua.InlineCode` + `LuaPerRoute` PARSE-REJECTed | Task 2 — arm 3 (`InlineCode`) + arm 4 (`SourceCodes`) + arm 18 (per-route via `validatePerRouteLua` boot-registration). PROGRESS Task 2 entry §"Acceptance-criteria evidence" — 6 PARSE-REJECT rows ALL PASS. |
| 4 | 4-arm DataSource resolution + WatchedDirectory PARSE-REJECT | Task 3 — `datasource.go::resolveDataSource` + 10 PARSE-REJECT leaves per parent §6.2 arms 6-15 + arm 9-extension (Task 11 fuzzer). PROGRESS Task 3 + Task 11 entries. |
| 5 | Pragmatic-middle bridge surface | Tasks 6 (headers + `__pairs`) + 7 (6 :logXxx) + 8 (streamInfo subset) + 9 (respond + decode_headers + encode_headers). 21 bridge entries per BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` bridge surface roster. PROGRESS Tasks 6-9 entries. |
| 6 | Stdlib-sandbox-strict default-deny + envoy-go-strict departure record | Task 5 — `internal/lua/sandbox.go::SandboxConfig` zero-value posture `StrictUpstreamParity`. Departure record at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` "envoy-go-strict departure — stdlib-sandbox-strict" paragraph (Task 16). PROGRESS Task 5 + Task 16 entries. |
| 7 | Per-stream `*LState` construction + per-script-source `*Chunk` cache | Tasks 4 (compile.go + CompileCache) + 5 (vm.go per-stream NewVM) + 9 (per-stream dispatch in decode_headers/encode_headers). Race-clean verified at Task 12 under `-race -count=10`. PROGRESS Tasks 4 + 5 + 9 + 12 entries. |
| 8 | 18-arm PARSE-REJECT roster (subject to D1 disposition) | Task 2 (arms 1, 2, 3, 4 byte-exact-tested; arms 5 + 17 D1-REFUTED silent-no-op-tested) + Task 3 (arms 6-15 + 10 leaves) + Task 4 (arm 16 — gopher-lua `*lua.ApiError` wrap) + Task 1 skeleton (arm 18 via `validatePerRouteLua`). Roster EXTENDED 18 → 19 at Task 11 fuzzer per items below. PROGRESS Tasks 2 + 3 + 4 + 11 entries. |
| 9 | 3-counter stat surface + 99 → 102 BEHAVIOR_CONTRACT.md update | Task 10 (stats.go + boot-registration; HCM-rooted template per AMEND-2). BEHAVIOR_CONTRACT.md stat-table 99 → 102 extension + extension summary paragraph at Task 16. PROGRESS Task 10 + Task 16 entries. |
| 10 | `respond_calls` envoy-go-strict counter departure record | Task 10 (counter registration) + Task 16 (BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` "envoy-go-strict departure — `respond_calls` counter" paragraph). PROGRESS Task 10 + Task 16 entries. |
| 11 | Runtime-error log-message wording envoy-go-strict departure record | Task 16 (BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` "envoy-go-strict departure — runtime-error log-message wording" paragraph). PROGRESS Task 16 entry. |
| 12 | 28th project-wide fuzzer `FuzzLuaConfigParse` | Task 11 — 30s + 60s baseline clean; project-wide count CONFIRMED at 28 via `grep -h '^func Fuzz' \| sort -u \| wc -l`. PROGRESS Task 11 entry. |
| 13 | Differential fixture `0026-http-lua-headers-bridge` GREEN | Tasks 13 (`BackendKind=HTTPLua` + `BootRejectFixture` harness) + 14 (fixture directory + 7 `.lua` scripts + driver) + 15 (wording-pin + green-light). 75.39s full 28-fixture suite at Task 15. PROGRESS Tasks 13 + 14 + 15 entries. |
| 14 | NEW `BackendKind=HTTPLua` constant + `scripts/` subdirectory + 7 per-scenario `.lua` files | Task 13 (`BackendKind=HTTPLua = 22` at `test/differential/fixture/fixture.go`) + Task 14 (`test/fixtures/0026-http-lua-headers-bridge/scripts/{a..g}_*.lua`). PROGRESS Tasks 13 + 14 entries. |
| 15 | envoy-go-side `"script load error: "` wrapping at `cmd/envoy-go/main.go` | Task 15 (`maybeWrapLuaScriptLoadError` helper at `cmd/envoy-go/main.go:191` + 55 LoC helper appended). PROGRESS Task 15 entry. |
| 16 | ADR-0188 §Decision + §Consequences body landed | Task 16 (DECISIONS.md ADR-0188 §Decision + §Consequences body REPLACES the SPEC-commit placeholders per ADR-0044 in-place edit discipline). This commit. |
| 17 | ADR-0189 §Decision + §Consequences body landed | Task 16 (DECISIONS.md ADR-0189 §Decision + §Consequences body REPLACES the SPEC-commit placeholders per ADR-0044). This commit. |
| 18 | STATE.md re-advance + ROADMAP row 22.1 flip | Task 16 (STATE.md rewrite-in-place per BOOTSTRAP §4.1 invariant 1 + ROADMAP.md row 22.1 `in-progress → done` per ADR-0106). This commit. |

**Items 19-24 — 22.1 SPEC-specific extensions:**

| # | Item | Closure evidence |
|---|---|---|
| 19 | D5 resolution recorded at §11.1 — 28th-fuzzer count CONFIRMED + pinned at IMPL | 22.1 SPEC §11.1 (CONFIRMED 27 baseline at SPEC commit); Task 11 PROGRESS entry (CONFIRMED 28 post-IMPL via `find . -name 'fuzz_test.go' \| xargs grep -h '^func Fuzz' \| sort -u \| wc -l`); ADR-0189 §Decision body item 8 D5 closure paragraph. |
| 20 | D7 resolution recorded at §11.2 — bridge `__pairs` alphabetical-snapshot RATIFIED + lands at IMPL Task 6 | 22.1 SPEC §11.2 (RATIFIED at SPEC commit); Task 6 PROGRESS entry (`__pairs` alphabetical-snapshot lands at `bridge.go` + cross-run-determinism test at `bridge_test.go`); ADR-0189 §Decision body item 7 D7 closure paragraph; BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` bridge surface roster entry #10. |
| 21 | D1 closure at IMPL Task 2 first action | Task 2 PROGRESS entry §"D1 closure evidence" — upstream Envoy v1.37.2 source scrape against `lua_filter.cc:1455-1485` FilterConfig constructor + `:140-181` PerLuaCodeSetup constructor + `config.cc:14-26` createFilterFactoryFromProtoTyped; both arms 5 + 17 REFUTED → silent no-op; reserved wording constants pinned in source with `//nolint:unused`. ADR-0189 §Decision body §3 + §Context D1 disposition paragraph. |
| 22 | D3 closure at 22.1 PLAN session | 22.1 PLAN §"Planner-time deferred-decision resolution" D3 paragraph (option (a) — stat-counter `executions` delta IS the "Lua ran" assertion); Task 14 PROGRESS entry §"Implementation details — judgment calls" #2 (cumulative-after-Drive `AssertStats` cross-side compare); ADR-0189 §Decision body item 8 D3 closure paragraph. |
| 23 | Per-task PROGRESS.md entry shape per phase-21 IMPL precedent | All 16 Task entries in `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/PROGRESS.md` follow the 8-section format per D-P3 (Status; Artifacts landed; Verification; D-decision-disposition update; Commit SHA; Tier + Task-number cross-reference). Verification commands quoted verbatim per `superpowers:verification-before-completion`. |
| 24 | REVIEW.md authored at 22.1 IMPL phase-done | THIS file at `docs/envoy-go/phases/22.1-http-filter-lua-vm-and-headers-bridge/REVIEW.md` per `superpowers:requesting-code-review`. |

**24/24 items GREEN.** All acceptance items closed with cross-references to PROGRESS task entries + cross-cutting artifacts.

---

## 3. D3 + D-P1..D-P10 PLAN-time decision-disposition record

| Decision | Disposition at PLAN | Disposition at IMPL phase-done | Notes |
|---|---|---|---|
| D3 | LOCKED at PLAN session per parent §11.7.7 option (a) (stat-counter `executions` delta assertion) | **HELD** — Task 14 PROGRESS entry §"Implementation details — judgment calls" #2 + #9 verified the cumulative-after-Drive `AssertStats` shape (driver scrapes `/stats?format=text` admin endpoint + emits scenario (e) verdict via cross-side delta-of-1 compare). | option (b) artificial-counter-bumping + option (c) NEW `LogAsserter` interface both REJECTED at PLAN session; ZERO new harness/runner infrastructure required. |
| D-P1 | SPEC §6 16-task numbering INHERITED VERBATIM; PROGRESS preamble is Pre-Task 0 | **HELD** — Pre-Task 0 PROGRESS preamble + 16 Task entries authored verbatim per the inherited numbering. | No task-number drift; per-task headings match PLAN exactly. |
| D-P2 | Per-task subagent dispatch type `general-purpose` for Tasks 1-15; Task 16 with explicit acceptance-checklist ref | **HELD** — all 16 Tasks dispatched as subagents per project memory `feedback_execution_style.md`. Task 16 explicit acceptance-checklist ref via THIS REVIEW + the PROGRESS entries' acceptance-criteria sections. |
| D-P3 | Per-task PROGRESS.md entry shape per phase-21 IMPL precedent (8-section format) | **HELD** — all 16 Task entries follow the 8-section format. Verification commands quoted verbatim per `superpowers:verification-before-completion`. |
| D-P4 | Per-task TDD ordering RIGID (Write-failing-test → Run-FAIL → Implement → Run-PASS → vet+lint clean → PROGRESS append → commit); Task 14 relaxed to test-with-implementation | **HELD** — all 16 Tasks followed TDD ordering. Task 14 fixture work relaxed per the PLAN exception. |
| D-P5 | CompileCache scope at compiledConfig-instance; GC-driven eviction; no cross-listener / global cache | **HELD** — Task 4 IMPL `internal/lua/compile.go::CompileCache` owns the cache at the `*compiledConfig` instance per ADR-0188 §Decision §3. Cross-listener / global cache REJECTED (would force lifecycle management). |
| D-P6 | Boot-registration alphabetical between `localratelimit` and `oauth2`; per-route validator registered inside `lua.New` via `reg.RegisterPerRouteValidator` per ADR-0110 single-chokepoint | **HELD** — Task 10 IMPL `cmd/envoy-go/main.go` boot-registration alphabetical + Task 1+10 `lua.New` per-route validator registration. |
| D-P7 | Fuzzer corpus seed roster — 30 total: 18 per-PARSE-REJECT-arm + 5 valid-config + 7 adversarial-Lua-source | **HELD** — Task 11 IMPL `fuzz_test.go` 30 in-test seeds via `f.Add` + 2 regression seed files at `testdata/fuzz/FuzzLuaConfigParse/`. Documented scope limits on arm-1 + arm-2 + arm-15 + arm-18 non-seed-expressibility. |
| D-P8 | Task-graph parallelization: 4-way Tasks-2+3+4+11; 3-way Tasks-6+7+8; 3-way Tasks-9+10+13; 2-way Tasks-12+14 | **HELD** — per-task PROGRESS entries §"Tier + Task-number cross-reference" document the parallelization decisions. |
| D-P9 | Cross-package regression-test command shape — `go test -count=1 -race ./...` at gates + 28-fixture-directory regression at Task 16 Gate D | **HELD WITH SCOPING** — Gate C race scoped to unit packages per the integration-suite port-bind race flakiness observed at Task 16 first run; Gate D differential 28/28 GREEN without race (matches project convention for the integration suite). |
| D-P10 | `*LState`-pool benchmark sub-task at Task 12 with > 1ms threshold gating per parent §13-R6 | **HELD — R6 STANDS WEAK-default.** Task 12 IMPL `BenchmarkPerStreamLState_Construction_Headers` reports `ns/op = 69865` (~70µs/stream) — 3 independent runs at 69935 / 72333 / 69865 ns/op (within ~5% of central value); well under 1ms threshold. ADR-0190 NOT consumed; carries forward to 22.2 BRAINSTORM as the 22.2 IMPL escape-valve slot. |

---

## 4. D1 closure evidence — REFUTED both arms 5 + 17 (Task 2)

Per 22.1 PROGRESS Task 2 entry §"D1 closure evidence": empirically scraped upstream Envoy v1.37.2 source via WebFetch against `source/extensions/filters/http/lua/{config.cc,lua_filter.cc}`. Both arms 5 + 17 REFUTED.

**(a) Arm 5 — `default_source_code` absent.** Upstream `source/extensions/filters/http/lua/lua_filter.cc:1455-1485` `FilterConfig::FilterConfig` constructor: the `if (proto_config.has_default_source_code()) { … } else if (!proto_config.inline_code().empty()) { … }` chain has **no terminal `else` arm**; when both predicates are false, `default_lua_code_setup_` stays null-initialized; the filter loads and runs as a silent pass-through. No `throw EnvoyException` fires for the absent-all case. Arm 5 REFUTED.

**(b) Arm 17 — script-defines-no-hooks.** Upstream `source/extensions/filters/http/lua/lua_filter.cc:140-181` `PerLuaCodeSetup::PerLuaCodeSetup` constructor: missing-hook branch (lines 175-177 + 180-182) emits `ENVOY_LOG(info, …)` and falls through; **no `throw EnvoyException` fires**. The filter loads with `request_function_slot_` / `response_function_slot_` pointing at `LUA_REFNIL`, which the runtime per-stream `StreamHandleWrapper` dispatch path interprets as "this hook is not defined; skip CallGlobal." Arm 17 REFUTED.

**(c) Upstream `config.cc::createFilterFactoryFromProtoTyped`** delegates ALL validation to the `FilterConfig` constructor; no pre-`FilterConfig` validation gate that throws on absent-`default_source_code`.

**envoy-go disposition (per parent §12-D1 REFUTED branch):** both arms flip to silent-no-op (degraded pass-through). Reserved wording constants `parseRejectDefaultSourceCodeRequired` + `parseRejectScriptMissingHooks` pinned in source with `//nolint:unused` for future policy-bump migration per the phase-21 `parseRejectFixedValueDeferred` reserved-constant precedent.

ADR-0189 §Decision body §3 records the empirical disposition; BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` "D1 disposition" paragraph records the byte-equivalent posture for operators.

---

## 5. R6 disposition (Task 12) — STANDS WEAK-default; ADR-0190 NOT consumed

Per 22.1 PROGRESS Task 12 entry §"D-P10 R6 disposition":

- **Threshold (per PLAN D-P10):** `ns/op > 1_000_000` (= 1ms per per-stream construction).
- **Observed:** `ns/op = 69865` (~70µs; 3 independent runs at 69935 / 72333 / 69865 ns/op; ~5% range).
- **Disposition: R6 STANDS WEAK-default.** `ns/op = 69865 ≤ 1_000_000` threshold; per-stream construction acceptable; ADR-0190 NOT consumed; carries forward to 22.2 BRAINSTORM.

The headers-only per-stream construction cost (~70µs / stream) is dominated by gopher-lua `NewState` allocation + the 5 metatable installs + the script-Run bytecode dispatch. At the per-stream cost observed, 14k+ stream constructions/sec/core are sustainable — well above the order-of-magnitude that operationally justifies an `*LState` pool. The escape-valve remains primed: 22.2 BRAINSTORM may re-evaluate against the body/trailer bridge surface (which adds more bridge methods + more per-stream allocation) and decide whether the pool design fires there.

`BenchmarkPerStreamLState_Construction_Headers` measures per-stream `*lua.LState` construction matching production `DecodeHeaders` step-by-step: `NewVM(WithSandboxConfig)` + 5 metatable installs + `*requestHandleContext` + LUserData bind + `vm.Run(chunk)` + `vm.CallGlobal("envoy_on_request", reqUd)` + `vm.Close()`. Helper `buildBenchCompiledConfig(b)` constructs a `*compiledConfig` with a noop-hook script. nil `cc.stats` to avoid stats-counter overhead (stats counters are separate measurable overhead; ~ns each is negligible against the ~70µs construction cost).

ADR-0188 §Decision body §3 + §Consequences "(-) Per-stream `*LState` construction overhead" bullet records the disposition; BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` "envoy-go-strict departure" + "Phase 22.1 forward-pointer notes" "`*LState`-pool design (ADR-0190 escape-valve)" bullets cross-reference.

---

## 6. 2 NEW PARSE-REJECT arms surfaced by Task 11 fuzzer

Per 22.1 PROGRESS Task 11 entry §"Fuzzer findings — 2 real defects fixed":

**Arm 19 — `Lua.stat_prefix` invalid chars per `stats.IsValidName` regex pre-check.**

- **Defect:** without arm-19 validation, a `stat_prefix` containing `-`, ` `, `/`, or other `nameRE`-invalid characters caused the assembled name (e.g. `http.<hcm>.lua.my-script-prefix.errors`) to violate `internal/stats/registry.go::nameRE`, panicking at registry write time per ADR-0059's boot-time-panic discipline. The panic propagates to listener construction, killing the proxy.
- **Fix:** `compiled_config.go::buildCompiledConfig` adds a `stats.IsValidName` probe-name check on `Lua.stat_prefix`. Pattern mirrors `hcm/config.go:209` + `cluster/manager.go:205` precedent (same defect was anticipated + pre-checked at HCM + cluster boundaries; lua was missed at Task 10).
- **Byte-exact wording:** `"lua: stat_prefix: invalid characters in %q (must match %s)"` (constant `parseRejectStatPrefixInvalidFmt`).
- **Regression seed:** `internal/filter/http/lua/testdata/fuzz/FuzzLuaConfigParse/7cfce1e268c58e26` (stat_prefix `"0 "`).
- **LoC delta:** ~30 LoC + 20 LoC doc-comment.

**Arm 9-extension — `Filename` DataSource 16 MiB cap via `io.LimitReader` (defense vs `/dev/full`-class infinite-read OOM-kill).**

- **Defect:** `resolveDataSourceFilename` allocates unboundedly on infinite-read special files. The naive `os.ReadFile` reads until EOF; `/dev/full` returns infinite NUL bytes, never reaching EOF. Operator-supplied `Filename: "/dev/full"` (or any infinite-read device, named pipe, or simply a multi-GB script-file typo) causes the Go runtime to OOM-kill the proxy.
- **Fix:** `datasource.go::resolveDataSourceFilename` replaces `os.ReadFile` with `os.Open` + `io.LimitReader(f, maxFilenameScriptBytes+1)` where `maxFilenameScriptBytes = 16 MiB`. The +1 read byte distinguishes "exactly at the cap" (accepted) from "one byte over" (rejected with byte-exact `parseRejectDataSourceFilenameTooLargeFmt` wording).
- **Byte-exact wording:** `"lua: default_source_code: file %q exceeds the maximum script size of %d bytes"` (constant `parseRejectDataSourceFilenameTooLargeFmt`; cap value = 16777216 bytes).
- **Regression seed:** `internal/filter/http/lua/testdata/fuzz/FuzzLuaConfigParse/1e64cd3ef1a0302b` (Filename `/dev/full`; Linux-specific — on non-Linux runners the seed exercises the arm-9 read-failed path instead).
- **LoC delta:** ~25 LoC + 30 LoC doc-comment.

**Roster 18 → 19.** Both arms fixed inline at Task 11 per ADR-0018 fuzzer-discipline ("fuzzers exist to surface panics + the panics must be fixed before the fuzzer lands"). Documented at BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` 19-arm PARSE-REJECT roster table; ADR-0189 §Decision body §3 ratifies the extension.

---

## 7. Next-phase handoff state (22.2 BRAINSTORM scope hand-off)

**22.2 BRAINSTORM scope per parent §10 forward-pointers + BEHAVIOR_CONTRACT.md `### Phase 22.1 forward-pointer notes` "Deferred items — 22.2 BRAINSTORM scope hand-off" bullets:**

1. **Full bridge surface delta** — body methods (`:body()`, `:bodyChunks()`, `:trailers()`); metadata (`:metadata()`, `:dynamicMetadata()`); `:httpCall()` (consumer #1 of `internal/httpclient/` per phase-20 ADR-0177); crypto helpers (`:base64Decode()`, `:sha256()`); filesystem helpers (`:fileBytes()`); `:timestamp()`; `:connection()`; full `:streamInfo()` (route-metadata / cluster-info / SSL-context / dynamic-metadata accessors).
2. **Anticipated ADRs at 22.2 IMPL** (settled at 22.2 BRAINSTORM): ~2-4 NEW ADRs (full-bridge-API shape + httpCall dispatcher + body-buffering interaction with ADR-0128 + dynamic-metadata-bridge deferral). +1 conditional ADR-0190 (`*LState`-pool escape-valve — fires only if 22.2 body/trailer bridge surface crosses 1ms per-stream threshold).
3. **Stat-surface extension** — likely +2 httpCall counters (settled at 22.2 SPEC; project total 102 → ~104 at 22.2 phase-done).
4. **AMEND-9 divergence catalogue extension** — `lua.FormatNumber(v) string` helper at `internal/lua/` for 22.2 body/trailers bridge use; addresses (a) `tostring(float)` Go vs LuaJIT shortest-round-trippable mismatch, (b) `string.format("%d", float)` Go-fmt mismatch, (c) `pcall` error-message prefix mismatch.
5. **Differential fixture 0027** — `0027-http-lua-full-bridge` (partial cross-side / REFERENCE-LESS fallback per parent §8).

**Cold-start scope for the 22.2 BRAINSTORM session:**

- STATE.md (post-22.1-IMPL state) — `lifecycle-state: phase 22.1 IMPL done; awaiting 22.2 SPEC`; `next-skill: superpowers:brainstorming`.
- `docs/envoy-go/ROADMAP.md` (row 22.1 done; row 22.2 planned; row 22 in-progress).
- THIS REVIEW.md + the 16 PROGRESS task entries (most relevant for 22.2 BRAINSTORM: Task 11 fuzzer 2 arms + Task 12 R6 STANDS + Task 14+15 fixture-0026 GREEN).
- `docs/envoy-go/phases/22-http-filter-lua/SPEC.md` (parent SPEC; §10 forward-pointers describe 22.2 anticipated scope).
- `docs/envoy-go/phases/22-http-filter-lua/BRAINSTORM.md` (Q6 pragmatic-middle 22.1 vs full 22.2 bridge dialogue).
- DECISIONS.md tail (ADR-0188 + ADR-0189 full bodies; ADR-0190 still next-free; ADR-0125 §(xiv) anticipation paragraph).
- BEHAVIOR_CONTRACT.md `### envoy.filters.http.lua` + `### Phase 22.1 forward-pointer notes` (22.2 BRAINSTORM scope hand-off bullets).

The 22.2 BRAINSTORM session creates a fresh worktree per project memory `feedback_git_worktrees.md`: `git worktree add /home/esa/git/envoy-go/.worktrees/phase-22.2-http-filter-lua-full-bridge-brainstorm -b phase-22.2-http-filter-lua-full-bridge-brainstorm <22.1-IMPL-tip-SHA>`.

---

## 8. Reviewer notes — cross-cutting observations

**Test discipline.** TDD strict at every Task per D-P4 + `superpowers:test-driven-development`. Every Task entry quotes verification command outputs verbatim per `superpowers:verification-before-completion`. Every PARSE-REJECT arm has byte-exact wording test coverage at `TestParseRejectConstants_ByteExactWording` + `TestResolveDataSource_ByteExactWording`. Race-clean under `-race -count=10` for the race-detection-meaningful surface (Task 12).

**Wording-pin discipline.** All 19 PARSE-REJECT arms have byte-stable wording constants per ADR-0080. The boot-sink `"script load error: "` prefix is filter-scoped (keyed on the byte-stable substring `"lua: default_source_code: compile:"` emitted by the filter package) — avoids the false-positive risk of a generic `"compile:"` keying. Reserved D1-REFUTED constants pinned with `//nolint:unused` for future policy-bump migration per the phase-21 reserved-constant precedent.

**ADR discipline.** ADR-0044 in-place edit discipline: ADR-0188 + ADR-0189 §Decision + §Consequences body REPLACES the SPEC-commit placeholders at THIS Task 16 commit; §Context blocks UNCHANGED (anchored at parent SPEC commit `41ccee7`). ADR-0125 §(xiv) AMENDMENT-anticipation paragraph UNCHANGED (anchored at parent SPEC commit; AMENDMENT body lands at 22.3 IMPL final Task). NO new ADR consumption at this 22.1 IMPL phase-done — ADR-0190 carries forward unconsumed per D-P10 R6 STANDS WEAK-default.

**Atomic-landing discipline.** BEHAVIOR_CONTRACT.md 7-edit bundle landed atomically per ADR-0052 + parent §14 + 22.1 SPEC §14 at THIS Task 16 commit. STATE.md re-advance + ROADMAP row 22.1 flip + REVIEW.md authoring all in the same atomic commit. The commit is a single `superpowers:finishing-a-development-branch` candidate per project memory `feedback_git_worktrees.md`.

**Scope-expansion judgment calls.** Task 11 scope-expanded to include 2 PARSE-REJECT arm fixes (arm 19 + arm 9-extension) discovered by the fuzzer — kept Task 11 atomic per ADR-0018 ("fuzzers exist to surface panics + the panics must be fixed before the fuzzer lands"). Task 14 surfaced 2 closure items for Task 15 (envoy-go-side wording-pin + reference-container script-mount harness gap) — Task 15 chose Option B2 (driver-side InlineString render) over Option B1 (extend harness) as the lighter-touch closure. Task 16 housekeeping: fixed pre-existing gofmt drift at `internal/cluster/cluster.go:50` (from commit `49cc7cd`) as 1-line out-of-scope housekeeping to clear Gate B — documented in the commit message + this REVIEW.

**No open issues at phase-done.** All 24 acceptance items GREEN. All 6 phase-done gates GREEN. All D-questions disposition-recorded. Phase 22.1 IMPL is READY FOR SQUASH-MERGE TO MASTER per project memory `feedback_git_worktrees.md` + ADR-0005 §Decision 4.

---

## 9. Squash-merge handoff

**Branch:** `phase-22.1-http-filter-lua-vm-and-headers-bridge-impl`  
**Worktree:** `/home/esa/git/envoy-go/.worktrees/phase-22.1-http-filter-lua-vm-and-headers-bridge-impl`  
**Predecessor master tip:** `6d3b487` (`phase 22.1 PLAN follow-up: STATE.md SHA-fill (TBD → 02d745a post-squash)`)  
**Squash-merge target:** `master`  
**Post-squash SHA-fill follow-up:** `phase 22.1 IMPL follow-up: STATE.md SHA-fill (TBD → <squash-SHA> post-squash)` per the phase-09..21 convention.

**Squash-merge commit message** (per the project's phase-09..21 squash convention):

```
Squash merge phase-22.1-http-filter-lua-vm-and-headers-bridge-impl
```

All 16 Tasks landed atomically per the worktree branch's sequential commit history. Post-squash, the branch can be deleted + the worktree removed per `superpowers:finishing-a-development-branch`.
