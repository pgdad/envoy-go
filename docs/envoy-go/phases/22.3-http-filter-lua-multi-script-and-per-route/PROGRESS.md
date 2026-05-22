# Phase 22.3 IMPL — HTTP Filter Lua: Multi-Script and Per-Route

## Preamble

**Phase:** 22.3 — HTTP Filter Lua: Multi-Script and Per-Route (IMPL)
**Base commit (worktree base):** `04eba88` (phase 22.3 PLAN follow-up: STATE.md SHA-fill (TBD → 1efd4de post-squash))
**Branch:** `phase-22.3-http-filter-lua-multi-script-and-per-route-impl`

---

## SPEC §15 Acceptance Checklist (closure target)

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
14. Parent row 22 flips `in-progress → done` (final sub-phase) + STATE.md re-advance + ROADMAP row 22.3 flip — Task 6.
15. Per-task PROGRESS.md entries quoting command outputs per verification-before-completion — Tasks 0-6.
16. REVIEW.md authored at phase-done per requesting-code-review — Task 6.

---

## 7-Task Graph

- **Task 0** — PROGRESS.md preamble + precondition verification (THIS task).
- **Task 1** (Tier A) — consume `Lua.SourceCodes` into `sourceCodes map[string]*internallua.Chunk` registry + `perRouteChunks` memo + `perRouteMu`; add `source-codes-key-empty` arm; DROP arm-4 `source_codes`-deferred reject.
- **Task 2** (Tier A) — NEW `perroute.go` `parsePerRouteLua` real 3-arm validator; DROP arm-18 one-liner.
- **Task 3** (Tier B) — per-route 3-tier dispatch (`resolveDecodeScript`) + decode/encode wiring + encode-guard fix.
- **Task 4** (Tier C) — `FuzzLuaPerRouteConfig` (30→31) + `FuzzLuaConfigParse` corpus extension + `BenchmarkPerStream_PerRoute_Resolution` (R6).
- **Task 5** (Tier D) — differential fixture-0028 (29→30): 5 cross-side scenarios (a)-(e) + 3 boot-reject (f)-(h).
- **Task 6** (Tier E, atomic landing) — ADR-0193 §Decision+§Consequences + ADR-0125 §(xiv) AMENDMENT + BEHAVIOR_CONTRACT bundle + doc.go + STATE.md + ROADMAP row 22.3 + parent row 22 closure + REVIEW.md + full-suite green gate.

---

## D-P1 / D-P2 / D-P3 + R6 Dispositions

- **D-P1 RESOLVED** → option (b'), compile-with-cache-hit at bind, proto-pointer-memoized via a NEW `cc.perRouteChunks map[*luav3.LuaPerRoute]*internallua.Chunk` memo guarded by `cc.perRouteMu sync.Mutex`. Validator signature stays `func(proto.Message) error`.
- **D-P2 RESOLVED** → fixture-0028 boot-reject roster: (g) `source_codes[name]` DataSource-failure + (h) per-route `source_code` DataSource-failure are CROSS-SIDE; (f) `source_codes` key-empty is REFERENCE-LESS subject-only (envoy-go-strict-defensive).
- **D-P3 RESOLVED** → 6 net-new config-load arm-groups: (1) source-codes-key-empty + (2) source-codes-each-value-data-source-resolution + (3) per-route-override-oneof-required + (4) per-route-disabled-must-be-true + (5) per-route-name-min-1-rune + (6) per-route-source-code-each-arm. Arms 3 (reserved-name) + 7 (dangling-name) DROPPED.
- **R6** → anticipated WEAK-default STANDS: per-route resolution is O(1); `BenchmarkPerStream_PerRoute_Resolution` at Task 4; conditional ADR-0194 fires only if `> 1ms`.
- **AMEND-22.3-1**: a dangling per-route `name` is an upstream-parity SILENT NO-OP at per-stream dispatch, NOT a config-load PARSE-REJECT.

---

## Task Log

---

### Task 0 — PROGRESS.md preamble + precondition verification

**Status:** DONE

#### Step 1: Build / Vet / Lint

```
$ go build ./... && go vet ./... && golangci-lint run
(no output — all clean, exit 0)
```

Result: GREEN — build, vet, and lint all pass with no output.

#### Step 2: Lua package + per-route framework tests

```
$ go test ./internal/filter/http/lua/... ./internal/lua/... ./internal/filter/http/ -count=1
ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	1.311s
ok  	github.com/esalaine/envoy-go/internal/lua	0.097s
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.258s
```

Result: GREEN — all packages PASS.

#### Step 3: Fuzzer baseline + fixture baseline

```
$ grep -rh '^func Fuzz' --include='*.go' . | sort -u | wc -l
30

$ ls -d test/fixtures/00*/ | wc -l
29
```

Fuzz functions (30 total, sorted):
```
func FuzzAccessLogFormat(f *testing.F)
func FuzzAdaptiveConcurrencyConfigParse(f *testing.F)
func FuzzBandwidthLimitConfigParse(f *testing.F)
func FuzzBootstrapLoad(f *testing.F)
func FuzzBufferConfigParse(f *testing.F)
func FuzzCheckResponseMapping(f *testing.F)
func FuzzCompressorConfigParse(f *testing.F)
func FuzzConfigDumpFormat(f *testing.F)
func FuzzCsrfPolicyConfigParse(f *testing.F)
func FuzzDrainTransitions(f *testing.F)
func FuzzExtAuthzConfigParse(f *testing.F)
func FuzzExtProcConfigParse(f *testing.F)
func FuzzFaultConfigParse(f *testing.F)
func FuzzFilterChainMatch(f *testing.F)
func FuzzFilterChainParse(f *testing.F)
func FuzzFrameStream(f *testing.F)
func FuzzHCMConfigParse(f *testing.F)
func FuzzHeaderMutationConfigParse(f *testing.F)
func FuzzHPACKDecode(f *testing.F)
func FuzzJwtAuthnConfigParse(f *testing.F)
func FuzzLocalRateLimitConfigParse(f *testing.F)
func FuzzLuaBodyBridge(f *testing.F)
func FuzzLuaConfigParse(f *testing.F)
func FuzzLuaHTTPCallConfig(f *testing.F)
func FuzzOAuth2ConfigParse(f *testing.F)
func FuzzProcessingResponseMapping(f *testing.F)
func FuzzPromTextFormat(f *testing.F)
func FuzzRBACConfigParse(f *testing.F)
func FuzzTcpProxyFilter(f *testing.F)
func FuzzTLSContextParse(f *testing.F)
```

Fixture directories (29 total):
```
test/fixtures/0000-tcp-echo/
test/fixtures/0001-tcp-proxy-rr/
test/fixtures/0002-tls-tcp/
test/fixtures/0003-http11-routing/
test/fixtures/0004-h2-routing/
test/fixtures/0005-prometheus-stats/
test/fixtures/0006-access-log/
test/fixtures/0007a-cors/
test/fixtures/0007b-iteration-probe/
test/fixtures/0008-listener-chain-match/
test/fixtures/0009-admin-config-dump/
test/fixtures/0010-graceful-drain/
test/fixtures/0011-http-fault/
test/fixtures/0012-http-header-mutation/
test/fixtures/0013-http-local-ratelimit/
test/fixtures/0014-http-csrf/
test/fixtures/0015-http-buffer/
test/fixtures/0016-http-compressor/
test/fixtures/0017-http-bandwidth-limit/
test/fixtures/0018-http-rbac/
test/fixtures/0019-http-jwt-authn/
test/fixtures/0020-http-ext-authz-http/
test/fixtures/0021-http-ext-authz-grpc/
test/fixtures/0022-http-ext-proc-grpc/
test/fixtures/0023-http-ext-proc-body/
test/fixtures/0024-http-oauth2/
test/fixtures/0025-http-adaptive-concurrency/
test/fixtures/0026-http-lua-headers-bridge/
test/fixtures/0027-http-lua-full-bridge/
```

**Pre-22.3 baselines confirmed:**
- Fuzzer count: **30** (expected 30) ✓
- Fixture count: **29** (expected 29) ✓

All preconditions GREEN. Proceeding with PROGRESS.md commit.

Task 0 commit: `544e33c`.

---

### Task 1 — consume `Lua.SourceCodes` into name→*Chunk registry + key-empty arm (Tier A)

**Commit:** `91b3102` (implementer `5289c37` + code-review follow-up amended in place).

Implemented (subagent-driven, TDD-first):
- NEW `*compiledConfig` fields: `sourceCodes map[string]*internallua.Chunk` (the `name → *Chunk` registry; nil when proto has no `source_codes`), plus `perRouteChunks map[*luav3.LuaPerRoute]*internallua.Chunk` + `perRouteMu sync.Mutex` (pre-declared for Task 3, unused this task).
- NEW consts `parseRejectSourceCodesKeyEmpty = "lua: source_codes: key must be non-empty"` + `wrapParseRejectSourceCodesValueFmt = "lua: source_codes[%q]: %w"`.
- REPLACED the arm-4 `source_codes`-deferred reject with the consume path: sorted-key iteration (deterministic compile order), empty-key reject, `resolveDataSource` + `CompileScript(src, compileCache)` each value into the SHARED per-listener cache (content-hash dedup), populate `cc.sourceCodes`. Retired `parseRejectSourceCodesDeferred`.
- Tests: single-entry consume, 2-distinct-chunks, byte-identical-content-dedup (same `*Chunk` pointer), key-empty byte-exact reject, bad-value (`Filename` ENOENT) prefix reject, `source_codes`-only-no-default happy path (`cc.chunk==nil` + `cc.sourceCodes` populated). Deleted the arm-4 deferred test rows; repointed the byte-exact const-table.

**Two-stage review:** spec-compliance ✅; code-quality found 1 Important (`wrapParseRejectSourceCodesValueFmt` missing from the byte-exact wording-pin table — ADR-0080) + 2 actionable Minors (stale file-level comment; missing `source_codes`-only happy-path test). All fixed and re-verified green.

**Verification:** `go test ./internal/filter/http/lua/ -count=1` → ok; `go vet` + `golangci-lint run ./internal/filter/http/lua/` → clean.

---

### Task 2 — replace arm-18 one-liner with real 3-arm `LuaPerRoute` validator (Tier A)

**Commit:** `2fc50b8` (implementer `074f605` + code-review follow-up amended in place).

Implemented (subagent-driven, TDD-first):
- NEW `internal/filter/http/lua/perroute.go` with `parsePerRouteLua(proto.Message) (*luav3.LuaPerRoute, error)`: type-assert → `GetOverride()==nil` → disabled-must-be-true → name-min-1 → source_code gauntlet (`resolveDataSource` + `CompileScript(src, nil)` compile-to-validate, discard), plus a defensive `default` arm (ADR-0018 never-panic).
- 4 byte-exact wording consts: `parseRejectPerRouteOneofRequired` = `"lua: per-route: override oneof is required"`; `parseRejectPerRouteDisabledFalse` = `"lua: per-route: disabled must be true (PGV const:true violation)"`; `parseRejectPerRouteNameEmpty` = `"lua: per-route: name length must be at least 1 rune"`; `wrapParseRejectPerRouteSourceCodeFmt` = `"lua: per-route: source_code: %w"`.
- `validatePerRouteLua` in `lua.go` now delegates: `{ _, err := parsePerRouteLua(m); return err }`. Retired `parseRejectPerRouteDeferred`; swept all references (compiled_config.go const + roster comment, compiled_config_test.go Arm18 row → 4 new pin rows, lua_test.go test).
- NEW `perroute_test.go` table-driven (10 cases) + return-value + byte-exact-const pins.

**Two-stage review:** spec-compliance ✅; code-quality found 2 Important (two stale doc-comments in `lua.go` the plan required updating) + 3 Minors (`Fmt`-suffix on a verb-less const → renamed; `fmt.Errorf("%s",...)` → `errors.New(...)` per package convention; missing defensive `default` in the oneof switch → added). All fixed and re-verified green.

**Verification:** `go test ./internal/filter/http/lua/ -count=1` → ok; `golangci-lint` → clean.

---

### Task 3 — per-route 3-tier dispatch + decode/encode wiring (Tier B)

**Commit:** `8490d32` (implementer `eee1f97` + code-review follow-up + a bug-fix amended in place).

Implemented (subagent-driven, TDD-first):
- NEW `(f *filter) resolveDecodeScript() (*internallua.Chunk, bool)` in `perroute.go` — the 3-tier dispatch. Matched the **buffer filter** per-route retrieval idiom (`f.dcb.RequestRouteConfig()` → `PerRouteConfig.Resolve`, stable `*LuaPerRoute` pointer per routeIdx). Plus `resolvePerRouteSourceCode` memo helper (D-P1 b'): `cc.perRouteMu`-guarded `cc.perRouteChunks` map keyed by `*LuaPerRoute` pointer, compiled into the SHARED `cc.compileCache`.
- `decode_headers.go`: short-circuit now `if f.cc == nil { return Continue }` then `chunk, disabled := f.resolveDecodeScript(); if disabled || chunk == nil { return Continue }`; runs the RESOLVED `chunk` (not `f.cc.chunk`). Disabled/nil-chunk return BEFORE VM construction → no VM built.
- `encode_headers.go`: **encode-guard fix** — `f.cc == nil || f.cc.chunk == nil || f.vm == nil` → `f.cc == nil || f.vm == nil` (so a per-route override on a default-less listener still fires `envoy_on_response`).
- Removed the now-stale `//nolint:unused` directives on `perRouteChunks`/`perRouteMu` (live as of this task).
- Tests: 9 `TestResolveDecodeScript_*` (nil-dcb/nil-route/nil-listener-chunk/named-hit/dangling-miss-silent-no-op/disabled/source-code/memo-hit-InlineString/memo-hit-Filename-not-reread) + 2 integration (`...DefaultLessListener_EncodeFires` regression-pins the encode-guard fix; `...Disabled_BuildsNoVM_SkipsBothHooks`).

**PLAN DIVERGENCE (recorded per `superpowers:executing-plans`):** The PLAN's File-structure table described `resolveDecodeScript` as "non-nil → `parsePerRouteLua` → …". The initial implementation followed that literally and called the full validator `parsePerRouteLua` on every per-stream dispatch — which for a `source_code` `Filename` arm re-read the file from disk **every stream** (re-running `resolveDataSource` purely to re-validate, discarding the chunk), BEFORE the memo was consulted. This **defeated the D-P1(b') "read+compile the Filename DataSource ONCE per route, never re-read per stream" guarantee**. The code-review-mandated read-counting `Filename` test (PLAN Task 3 Step 1: "the file is read once via a read-counting temp file") surfaced it. **Resolution:** the binding intent is the D-P1(b') no-re-read guarantee; the literal "call `parsePerRouteLua` at dispatch" wording is the error. `resolveDecodeScript` was rewritten to type-assert to `*luav3.LuaPerRoute` and switch on `GetOverride()` DIRECTLY (no per-stream re-validation), letting the memo own the single read. `parsePerRouteLua` is UNCHANGED and still the HCM-build validator (per-route config is already validated at HCM-build, so dispatch-time re-validation was redundant). The direct switch is semantically equivalent to the old path for the disabled-false (both → listener default) and dangling-name (both → `(nil,false)`) edge cases. Carries to REVIEW.md.

**Two-stage review + re-review:** spec-compliance ✅; code-quality found 0 Critical/Important + 2 Minors (stale nolint directives → removed; memo-hit test under-pinned the no-re-read claim → strengthened with the Filename test, which surfaced the bug above). After the bug-fix, a focused re-review confirmed all 9 dispatch tuples preserved + no-re-read guaranteed + scope clean.

**Verification:** `go test ./internal/filter/http/lua/ ./internal/filter/http/ -count=1` → ok; `go test -race ./internal/filter/http/lua/ -count=1` → ok (race-free); `golangci-lint` → clean.

---

### Task 4 — `FuzzLuaPerRouteConfig` (30→31) + corpus extension + R6 benchmark (Tier C)

**Commit:** `8a63a9a` (+ docstring nit `37e8d20`).

Implemented (subagent-driven):
- NEW `FuzzLuaPerRouteConfig` in `fuzz_test.go` — unmarshal fuzzed bytes → `*luav3.LuaPerRoute` (skip on unmarshal failure) → `parsePerRouteLua`, `recover()`-trap asserting MUST-NEVER-PANIC (ADR-0018; error returns are fine). 15 inline `f.Add` seeds, each mapping to a distinct validator branch / `resolveDataSource` gauntlet leaf (PGV-mirror arms + adversarial DataSource paths) + 1 raw-garbage seed. Reuses the existing fuzzer idiom exactly (closure `addSeed` helper, same recover style).
- EXTENDED `FuzzLuaConfigParse` corpus with 6 `source_codes` map seeds (single/multi-entry, empty-key, compile-error value, nil-specifier value, source_codes+default combined).
- NEW `BenchmarkPerStream_PerRoute_Resolution` in `lua_test.go` — two sub-benchmarks reusing the Task-3 test-doubles; VM construction excluded (covered by the existing `BenchmarkPerStream_FullBridge_LState_Construction`).

**30s fuzz:** 1.27–1.72M execs, NO crasher, no testdata crasher file written. NO D-P3 fuzzer-surfaced-arm triggered — `parsePerRouteLua` is panic-free across all explored inputs.

**R6 DISPOSITION (load-bearing for Task 6):** benchmark numbers —
```
BenchmarkPerStream_PerRoute_Resolution/resolution-only   10.46 ns/op    0 B/op   0 allocs/op
BenchmarkPerStream_PerRoute_Resolution/per-stream        31.47 ns/op   48 B/op   2 allocs/op
```
Both ~5 orders of magnitude under the 1 ms (1,000,000 ns) R6 gate. **Anticipated WEAK-default STANDS** — per-route resolution is O(1) (warm-memo hot path is zero-alloc). **Conditional ADR-0194 does NOT fire; next-free ADR STAYS ADR-0194.**

**Two-stage review:** spec-compliance ✅ (benchmark + no-crasher independently re-verified); code-quality APPROVED (0 Critical/Important; only a cosmetic stale const-name docstring in `perroute.go:82` from the Task-2 rename → fixed at `37e8d20`).

**Verification:** fuzzer count `grep '^func Fuzz' | sort -u | wc -l` → **31**; seed corpus `go test -count=1` → ok; `go vet` + `golangci-lint` → clean.

---

### Task 5 — differential fixtures (Tier D)

**Commit:** `ff0d02b`.

**PLAN DIVERGENCE — fixture count 29→31 (TWO directories), AUTHORIZED by the project owner.** The PLAN's Task 5 specified a SINGLE fixture-0028 hosting "5 cross-side + 3 boot-reject" scenarios (29→30). Implementation surfaced a hard framework constraint: the differential runner (`runFixture`) dispatches **exactly ONE branch per fixture directory** — a fixture is EITHER a cross-side wire fixture (like 0027: `MultiListenerDriver` + `CompareBytes` + reference-less subject-only scenarios) OR a boot-reject fixture (like 0026: `BootRejectFixture`, one config-load-failure, both-proxies-fail), never both (the boot-reject branch is a top-level early `return` before the cross-side path). Neither the 22.1 (0026) nor 22.2 (0027) precedent actually hosts both in one directory. Additionally, `runBootRejectFixture` is hardcoded cross-side (asserts BOTH proxies fail), so the PLAN's subject-only (f) key-empty scenario (upstream ACCEPTS an empty key) cannot be a `BootRejectFixture` at all. **Escalated to the project owner** (the choice changed scope + the "29→30" invariant + SPEC §15 item 11). **Decision: TWO directories** (0028 cross-side + 0029 boot-reject; 29→**31**).

**fixture-0028-http-lua-multi-script-and-per-route** (CROSS-SIDE multi-listener, 0027 pattern; does NOT implement `BootRejectFixture`): 6 listeners, all byte-exact `CompareBytes` GREEN —
- (a) `l_test_a` listener-default → `x-lua-script=default`
- (b) `l_test_b` `LuaPerRoute{name: named_a}` → `named_a`
- (b2) `l_test_b2` `LuaPerRoute{name: ghost}` (dangling) → header ABSENT (silent no-op, AMEND-22.3-1)
- (c) `l_test_c` `LuaPerRoute{source_code: override.lua}` → `override`
- (d) `l_test_d` `LuaPerRoute{disabled: true}` on a default-bearing listener → ABSENT (both hooks skipped)
- (e) `l_test_e` `LuaPerRoute{name: named_b}` → `named_b` (distinct registry key vs (b))

Scripts (`default/named_a/named_b/override.lua`) each set a DISTINCT `x-lua-script` value (deterministic `headers():add` only; no non-deterministic API) — selecting the wrong script would diverge byte-exact, so selection is genuinely tested. An independent spec-review confirmed NON-trivial passes: hit-vs-no-op distinguishable, dangling-name distinct from default, disabled distinct from default.

**fixture-0029-http-lua-source-codes-boot-reject** (BOOT-REJECT, 0026 pattern): a `source_codes{bad}` entry carrying a compile-error script (the NEW 22.3 consume path, NOT `default_source_code`) → BOTH reference Envoy v1.37.2 and envoy-go fail closed at config-load. Common stderr substring determined EMPIRICALLY by running both: **`near '-'`** (the `script load error` wrap is not shared for the source_codes arm). A valid no-op `default_source_code` + an unused valid cluster isolate the source_codes compile-reject as the failure cause.

**SCENARIOS COVERED ELSEWHERE (not built as differential scenarios per the framework constraint):** (f) `source_codes` key-empty (subject-only — upstream accepts empty keys; covered byte-exact by Task 1's `source-codes-key-empty` unit test) + (h) per-route `source_code` DataSource-failure (covered by Task 2's per-route source_code gauntlet unit test). The PLAN's D-P2 cross-side/subject-only roster is realized at the unit + (g) differential layer.

**runner_test.go:** +2 blank-import registration lines for the two new `inputs` packages — the established per-fixture discovery discipline (identical to the 0025/0026/0027 lines); NOT a framework change. `test/differential/fixture/fixture.go` + `cmd/envoy-go/main.go` ZERO delta (verified).

**Verification:** `go test ./test/differential/ -run 'Differential/(0028|0029)' -count=1` → ok (3.87s); both subtests RAN (not skipped). Fixture-directory count `ls -d test/fixtures/00*/ | wc -l` → **31**. (One transient `subject ready: EOF` admin-probe startup race on a combined run — a pre-existing harness flake, not a fixture defect; passed clean on re-run.)

---

### Task 6 — Atomic landing: ADR-0193 + ADR-0125 §(xiv) bodies + BEHAVIOR_CONTRACT + STATE/ROADMAP parent-row-22 closure + REVIEW (Tier E)

**Status:** DONE

**Artifacts landed (all in the single atomic Task 6 commit per ADR-0052):**
- **DECISIONS.md ADR-0193 §Decision + §Consequences body** — Status `Proposed → Accepted`; §Context UNCHANGED from the SPEC anchor `e72af4c`. §Decision (i)-(x): 22.3 file roster + SourceCodes content-hash registry consume + the real 3-arm `parsePerRouteLua` validator + the per-route 3-tier dispatch with the **D-P1(b') no-re-read realization** (PLAN-literal-wording correction documented) + the encode-guard fix + AMEND-22.3-1 dangling-name + no-reserved-name + the **D-P2 two-directory fixture split (29 → 31)** + the fuzzer + R6 WEAK-default + 0 net-new stats + the ADR-0125 §(xiv) landing pointer. §Consequences: full config surface + O(1) zero-alloc resolution + no-re-read read-counting test + 0 departure records + stat 107 + 17 filters + roster 8 → 9 + parent-row-22 closure + the unit-layer (f)/(h) coverage trade-off + the multi-listener port-race known limitation + the binding-gap forward-pointers.
- **DECISIONS.md ADR-0125 §(xiv) IN-PLACE AMENDMENT body** — the `**(xiv)**` clause (9th canonical: 3-arm hybrid disabled-bool + string-reference-delegation + DataSource-wholesale-override; SHARED stat-discipline; dangling-name silent-no-op per AMEND-22.3-1 vs jwt_authn 403) + the updated 9-shape roster table + the lua-row first-use citation (ADR-0193). Roster grows 8 → 9. NO new ADR number.
- **BEHAVIOR_CONTRACT.md** — NEW `#### Phase 22.3 multi-script + per-route surface delta` (4 surface families + 2 upstream-parity NOTES [dangling-name silent no-op + no-reserved-name] + ADR-0125 §(xiv) cross-reference + stat-107 + the differential-coverage paragraph) + NEW `#### Phase 22.3 forward-pointer notes` (parent-row-22 closure + binding-gap + consumer-#2 forward-pointers); the `#### Phase 22.2 forward-pointer notes` 22.3-anticipated bullets CONVERTED to LANDED entries. **Departure-record count UNCHANGED at 14** (verified: 0 new departure-record markers added).
- **doc.go** — NEW `# Phase 22.3 — multi-script SourceCodes + per-route LuaPerRoute` section (SourceCodes registry + LuaPerRoute 3-arm + 3-tier dispatch + D-P1(b') no-re-read + encode-guard fix + 9th canonical + R6 + fuzzer + differential + AMEND-22.3-1 + ADR-0193 + ADR-0125 §(xiv) cross-refs).
- **ROADMAP.md** — row 22.3 `in-progress → done` (date 2026-05-21 + IMPL annotation); **parent row 22 `in-progress → done`** (final sub-phase; §9 family 4 → 3 remaining: `wasm`, `admission_control`, `global rate limit`).
- **STATE.md** — rewrite-in-place: `phase 22.3 IMPL done; phase 22 (parent) done; awaiting next-phase BRAINSTORM`; next-skill `superpowers:brainstorming`; fuzzer 30 → 31; fixtures 29 → 31 (flagged as the authorized two-directory amendment); stat 107; 17 filters; ADR tail at ADR-0193 full body + ADR-0125 §(xiv) amended; next-free ADR-0194 (R6 WEAK-default STANDS); `last-commit: TBD → <squash-SHA> post-squash`; last-updated 2026-05-21.
- **REVIEW.md** — NEW at `docs/envoy-go/phases/22.3-.../REVIEW.md` per `superpowers:requesting-code-review`: green-gate verbatim outputs + SPEC §15 16-item closure + the TWO PLAN divergences (Task 3 D-P1(b') dispatch correction + Task 5 two-directory fixture amendment) + R6 disposition + per-task review summary.

**Full-suite green gate (per `superpowers:verification-before-completion` — verbatim):**

```
$ go build ./... && go vet ./... && golangci-lint run
BUILD_EXIT=0 ; VET_EXIT=0 ; LINT_EXIT=0      (all empty stdout/stderr — clean)

$ go test $(go list ./... | grep -v /test/differential) -count=1
... ok (all packages); h2spec 2.993s ; TEST_EXIT=0

$ go test -race ./internal/filter/http/lua/ -count=1
ok  	github.com/esalaine/envoy-go/internal/filter/http/lua	2.940s   ; RACE_EXIT=0

$ go test ./test/differential/ -run 'Differential/(0026|0027|0028|0029)' -count=1
# combined run: 0026 + 0029 GREEN; 0027 + 0028 transient `bind: address already in use`
# (freeTCPPort multi-listener port-race per 22.2 REVIEW §7.4 — NOT a defect).
# Isolated re-runs: 0026 ok ; 0027 ok (3/3) ; 0028 ok (3/4; 1 port-race) ; 0029 ok.

$ git diff --stat 04eba88 HEAD -- cmd/envoy-go/main.go test/differential/fixture/fixture.go
(no output — empty = ZERO delta = PASS)
```

**Acceptance evidence:** SPEC §15 16/16 GREEN (item 9 correctly NOT-fired — R6 WEAK-default; item 11 the authorized two-directory amendment). Departure count 14 (verified 0 new markers). Next-free ADR STAYS ADR-0194. main.go + fixture.go ZERO delta. Parent row 22 CLOSED.

**Self-review:** the two multi-listener fixtures (0027 + 0028) exhibit the documented pre-existing `freeTCPPort` port-allocation race only in combined/back-to-back runs; each passes clean in isolation, and the failure cause is always `bind: address already in use`, never a byte-divergence or compile-reject mismatch. This is the 22.2 REVIEW §7.4 item 2 known limitation, not a 22.3 regression.

**Hand-off:** parent row 22 is CLOSED; next-skill `superpowers:brainstorming` scoped to the next §9 phase (`wasm` / `admission_control` / `global rate limit`). Squash-merge to master + push to origin per project memory `feedback_git_worktrees.md` + `feedback_push_to_origin.md`; SHA-fill follow-up backfills the STATE.md `last-commit` placeholder.
