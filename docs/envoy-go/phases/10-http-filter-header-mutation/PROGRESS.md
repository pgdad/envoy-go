# Phase 10 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..09 PROGRESS.md structure.

## Preamble — execution preconditions

No deviations from PLAN.md's "Execution preconditions" block; all 16 preconditions were satisfied at cold-start.

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The six ADRs anticipated by SPEC §8 (ADR-0108..ADR-0113). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0108** `internal/filter/http/header_mutation/` package shape + boot registration — Task 5 (ADR text + impl), Task 9 (boot registration code)
- **ADR-0109** runtimeConfig shape + 3/1-field decomposition + AppendAction × 4 mapping + keep_empty_value semantics + multi-value collapse/preserve — Task 5
- **ADR-0110** Multi-tier per-route evaluation + ResolveAllTiers + RequestRouteConfigsAllTiers + RegisterPerRouteValidator + accessor-choice discipline + cross-tier algorithm + amends ADR-0073 — Task 7 (ADR text + ADR-0073 amendment paragraph), Tasks 2/3/4 (framework piece commits)
- **ADR-0111** Protected-header set + CONFIG-LOAD-TIME rejection + verbatim error format + EAGER per-route validation lifecycle — Task 5
- **ADR-0112** mutations.query_parameter_mutations[] DEFERRED (per ADR-0040 deferral format) — Task 16
- **ADR-0113** Header-value formatter substitution DEFERRED (per ADR-0040 deferral format) — Task 16

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The eleven planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **`RequestRouteConfigsAllTiers` callback symmetry = DECODER-ONLY** (mirrors cors precedent at cors.go:163; PLAN OVERRIDES SPEC default of "BOTH symmetric"; rationale: cors already calls dcb.RequestRouteConfig from EncodeHeaders so the pattern is in production).
2. **Per-request per-tier cache = SKIP** (sub-microsecond lookup + recompile; revisit only if profiling shows cost).
3. **Per-route protected-header validation lifecycle = EAGER** (HCM-build-time validator hook on *HTTPRegistry; BuildPerRouteConfig signature widens to take registry; ~50 LoC framework delta).
4. **compiledMutationOp slice element type = VALUE-TYPED** (cache locality during apply-loop; read-only after New).
5. **Protected-header set definition = prefix-check on `:` + case-insensitive equality on `host`** (`func isProtectedHeader(name string) bool { if strings.HasPrefix(name, ":") { return true }; return strings.EqualFold(name, "host") }`).
6. **Fuzzer = SHIP** (`FuzzHeaderMutationConfigParse`; ~50 LoC; 30s budget; thirteenth fuzzer; lands in Task 10).
7. **Race-detector cycle test = ADD `TestHeaderMutation_MultiTierConcurrentRequests`** (~30 LoC; lands in Task 8).
8. **applyOps exposure = KEEP unexported** (test via public DecodeHeaders/EncodeHeaders surface).
9. **Per-route-validator integration test fan-out = TABLE-DRIVEN per tier** (RC/VHost/Route × validator-error/no-validator/no-config; ~90 LoC; lands in Task 4).
10. **Fixture path = `test/fixtures/0012-http-header-mutation/`** (NOT `test/differential/0012-http-header-mutation/` per SPEC §4.3 erratum; mirrors 0010/0011 precedents).
11. **Fixture's new BackendKind enum value = `HTTPHeaderMutation BackendKind = 9`** (continues existing naming convention).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** `c9f4e61` — `phase 10: PROGRESS preamble + planner-time decision resolution`
**Notes:** Created PROGRESS.md; verified all 16 preconditions per PLAN §"Execution preconditions"; phase-10 SPEC + 10 PLAN confirmed present in HEAD; SPEC at f339c12; ADR tail at 0107 (next-free 0108); internal/filter/http/header_mutation/ absent (Task 5 lands); ResolveAllTiers absent (Task 2 lands); RequestRouteConfigsAllTiers absent (Task 3 lands); RegisterPerRouteValidator absent (Task 4 lands). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase-10-http-filter-header-mutation-impl
$ go version
go version go1.26.2 linux/amd64
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
107
$ git log -1 --format=%H -- docs/envoy-go/phases/10-http-filter-header-mutation/SPEC.md
f339c1251168d46422cf72a4380a733829bda6ae
```

## Task 2 — PerRouteConfig.ResolveAllTiers framework method

**Commits:** `3eba8df48ef78878a49482a95cad71800c7a5a8d` — `phase 10: framework — PerRouteConfig.ResolveAllTiers per ADR-0110`

**Notes:** Landed `PerRouteConfig.ResolveAllTiers(filterName string, routeIdx int) (route, vhost, rc proto.Message)` in `internal/filter/http/perroute.go`, appended after the existing `Resolve` method. Method reads directly from `p.scopes[routeIdx].route`, `p.scopes[routeIdx].vhost`, and `p.rc` maps without most-specific selection logic; returns the unmerged 3-tuple with nil entries for tiers with no config for filterName. Does not consult or pollute `p.cache` (planner-time decision 2). No ADR text in this task — ADR-0110 text lands in Task 7 at first end-to-end use. 12 test functions added to `perroute_test.go` covering all tier combinations + edge cases (out-of-range routeIdx, absent filterName, cache non-pollution, nil receiver). `go vet ./...` and `golangci-lint run ./internal/filter/http/...` both clean.

**Outputs:**
```
$ go test -race ./internal/filter/http/... -run TestResolveAllTiers -v 2>&1 | grep -E "^(=== RUN|--- (PASS|FAIL)|PASS|FAIL)"
=== RUN   TestResolveAllTiers_AllThreeSet
--- PASS: TestResolveAllTiers_AllThreeSet (0.00s)
=== RUN   TestResolveAllTiers_RouteAndVHostOnly
--- PASS: TestResolveAllTiers_RouteAndVHostOnly (0.00s)
=== RUN   TestResolveAllTiers_RouteAndRCOnly
--- PASS: TestResolveAllTiers_RouteAndRCOnly (0.00s)
=== RUN   TestResolveAllTiers_VHostAndRCOnly
--- PASS: TestResolveAllTiers_VHostAndRCOnly (0.00s)
=== RUN   TestResolveAllTiers_RouteOnly
--- PASS: TestResolveAllTiers_RouteOnly (0.00s)
=== RUN   TestResolveAllTiers_VHostOnly
--- PASS: TestResolveAllTiers_VHostOnly (0.00s)
=== RUN   TestResolveAllTiers_RCOnly
--- PASS: TestResolveAllTiers_RCOnly (0.00s)
=== RUN   TestResolveAllTiers_NoneSet
--- PASS: TestResolveAllTiers_NoneSet (0.00s)
=== RUN   TestResolveAllTiers_RouteIdxOutOfRange
--- PASS: TestResolveAllTiers_RouteIdxOutOfRange (0.00s)
=== RUN   TestResolveAllTiers_FilterNameNotPresent
--- PASS: TestResolveAllTiers_FilterNameNotPresent (0.00s)
=== RUN   TestResolveAllTiers_DoesNotPolluteResolveCache
--- PASS: TestResolveAllTiers_DoesNotPolluteResolveCache (0.00s)
=== RUN   TestResolveAllTiers_NilReceiver
--- PASS: TestResolveAllTiers_NilReceiver (0.00s)
PASS
PASS
PASS
PASS
PASS
```

## Task 3 — DecoderFilterCallbacks.RequestRouteConfigsAllTiers callback + chain wiring

**Commits:** `0551b4366b485728d6eebbb30826433e8f2ce81c` — `phase 10: framework — DecoderFilterCallbacks.RequestRouteConfigsAllTiers per ADR-0110`

**Notes:** Landed `RequestRouteConfigsAllTiers() (route, vhost, rc proto.Message)` on the `DecoderFilterCallbacks` interface in `internal/filter/http/callbacks.go` (after existing `RequestRouteConfig` method, with full ADR-0110 doc-comment covering decoder-only placement rationale and nil-return conditions). Added method body `decoderCB.RequestRouteConfigsAllTiers` in `internal/filter/http/chain.go` (after existing `RequestRouteConfig` method) delegating to `d.c.perRoute.ResolveAllTiers(d.c.filters[d.idx].Name, d.c.routeIdx)` with nil-guard on perRoute. Tests added to `internal/filter/http/callbacks_test.go`: `TestDecoderCB_RequestRouteConfigsAllTiers` (full 3-tier: route="route", vhost="vh", rc="rc") + `TestDecoderCB_RequestRouteConfigsAllTiers_NilPerRoute` (nil perRoute → all-nil return) + `fakeBothSidesFilter` helper capturing `dcb` via `SetDecoderCallbacks`. Mock sweep (Step 7): one affected file — `internal/filter/http/fault/fault_test.go`'s `recordingDCB` needed stub `RequestRouteConfigsAllTiers() (proto.Message, proto.Message, proto.Message) { return nil, nil, nil }`. `fakeDecoderCB` in `callbacks_test.go` also updated. `go vet ./...`, `golangci-lint run ./...`, and full `go test -race -count=1 ./...` all clean.

**Outputs:**
```
=== RUN   TestDecoderCB_RequestRouteConfigsAllTiers
--- PASS: TestDecoderCB_RequestRouteConfigsAllTiers (0.00s)
=== RUN   TestDecoderCB_RequestRouteConfigsAllTiers_NilPerRoute
--- PASS: TestDecoderCB_RequestRouteConfigsAllTiers_NilPerRoute (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	0.002s
```

## Task 4 — HTTPRegistry.RegisterPerRouteValidator + BuildPerRouteConfig integration

**Commits:** `80df1ea6e0e2c74ca5ce682281b39f15e6ad8155` — `phase 10: framework — HTTPRegistry.RegisterPerRouteValidator + BuildPerRouteConfig integration per ADR-0110`

**Notes:** Landed the third and final ADR-0110 framework piece (planner-time decision 3, eager validator lifecycle). Changes:

- `internal/filter/http/registry.go`: added `perRouteValidators map[string]func(proto.Message) error` field to `*HTTPRegistry`; initialised in `NewHTTPRegistry()`; added `RegisterPerRouteValidator(filterName string, validator func(proto.Message) error)` (panic-after-Freeze, mirrors existing `Register` discipline with `r.frozen.Load()` atomic check + RWMutex write lock) + `PerRouteValidator(filterName string) func(proto.Message) error` (nil-safe receiver, RLock, returns nil for unregistered names). Added `"google.golang.org/protobuf/proto"` import.

- `internal/filter/http/perroute.go`: widened `BuildPerRouteConfig` signature from 3-param to 4-param (`reg *HTTPRegistry` as last parameter, optional/nil-safe). Added validator pass after existing `parseMap` calls and before `return out, nil`: iterates `chainNames`, calls `reg.PerRouteValidator(name)`, skips nil validators, checks RC tier then each scope's VHost + Route tiers, returns `fmt.Errorf(...)` with canonical location prefix on first error.

- `internal/filter/hcm/config.go`: widened `buildPerRouteFromHCM` signature to accept `*filter_http.HTTPRegistry`; updated its internal `filter_http.BuildPerRouteConfig(...)` call to pass `httpRegistry`; updated call site at line 208 from `buildPerRouteFromHCM(rc, chainNames)` to `buildPerRouteFromHCM(rc, chainNames, httpRegistry)`.

**Test-only callers swept (`, nil` additions — 19 total):**
- `internal/filter/http/perroute_test.go`: 17 calls updated
- `internal/filter/http/callbacks_test.go`: 1 call updated
- `internal/filter/http/fuzz_test.go`: 1 call updated (used `chain` variable, not `chainNames`, required manual edit)
- `internal/filter/http/envoygotest/filter_test.go`: 1 call updated
- `internal/filter/http/cors/cors_test.go`: 2 calls updated
- `internal/filter/hcm/chain_integration_test.go`: 2 calls updated (used string-literal `[]string{...}`, not `chainNames`, required manual edits)

**New tests:** 4 registry tests in `internal/filter/http/registry_test.go` + 6 perroute validator integration tests in `internal/filter/http/perroute_test.go` = 10 new tests total. One minor deviation from PLAN verbatim: `TestRegistry_PerRouteValidator_LookupNotRegisteredReturnsNil` uses `"got non-nil func"` instead of `%v` formatting on a func value (vet caught `func value, not called` at format-string check).

**Outputs:**
```
$ go test -race ./internal/filter/http/... -run 'TestRegistry_RegisterPerRouteValidator|TestRegistry_PerRouteValidator|TestBuildPerRouteConfig_PerRouteValidator' -v 2>&1 | grep -E '^(=== RUN|--- (PASS|FAIL)|PASS|FAIL|ok)'
=== RUN   TestBuildPerRouteConfig_PerRouteValidator_NilSucceeds
--- PASS: TestBuildPerRouteConfig_PerRouteValidator_NilSucceeds (0.00s)
=== RUN   TestBuildPerRouteConfig_PerRouteValidator_NoValidatorRegisteredSucceeds
--- PASS: TestBuildPerRouteConfig_PerRouteValidator_NoValidatorRegisteredSucceeds (0.00s)
=== RUN   TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnRouteTier
--- PASS: TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnRouteTier (0.00s)
=== RUN   TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnVHostTier
--- PASS: TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnVHostTier (0.00s)
=== RUN   TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnRCTier
--- PASS: TestBuildPerRouteConfig_PerRouteValidator_ValidatorReturnsErrorOnRCTier (0.00s)
=== RUN   TestBuildPerRouteConfig_PerRouteValidator_OnlyConsultedForRegisteredFilters
--- PASS: TestBuildPerRouteConfig_PerRouteValidator_OnlyConsultedForRegisteredFilters (0.00s)
=== RUN   TestRegistry_RegisterPerRouteValidator_BeforeFreezeSucceeds
--- PASS: TestRegistry_RegisterPerRouteValidator_BeforeFreezeSucceeds (0.00s)
=== RUN   TestRegistry_RegisterPerRouteValidator_AfterFreezePanics
--- PASS: TestRegistry_RegisterPerRouteValidator_AfterFreezePanics (0.00s)
=== RUN   TestRegistry_PerRouteValidator_LookupNotRegisteredReturnsNil
--- PASS: TestRegistry_PerRouteValidator_LookupNotRegisteredReturnsNil (0.00s)
=== RUN   TestRegistry_PerRouteValidator_DoesNotConflictWithRegister
--- PASS: TestRegistry_PerRouteValidator_DoesNotConflictWithRegister (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http	(cached)
```

## Task 5 — header_mutation package + parser + protected-header validation [ADR-0108, ADR-0109, ADR-0111]

**Commits:** `7cda566` — `phase 10: header_mutation package + parser + protected-header validation [ADR-0108, ADR-0109, ADR-0111]`

**Notes:** Landed the `internal/filter/http/header_mutation/` package (first real implementation in phase 10). Three files created:

- `internal/filter/http/header_mutation/doc.go` (85 lines): Full package doc-comment with decode-side algorithm (5 steps), encode-side algorithm, concurrency model, public surface, New body discipline, stats discipline, ADR cross-references, and forward-pointers for ADR-0112 + ADR-0113 deferred features.

- `internal/filter/http/header_mutation/header_mutation.go` (253 lines after gofmt): `TypeURL` constant, `filterName` constant, `runtimeConfig` (3-field), `mutationOpKind` (kindRemove/kindAppend), `compiledMutationOp` (value-typed, 5-field per planner-time decision 4), `New` factory, `buildRuntimeConfig`, `compileOps`, `isProtectedHeader`, `validatePerRouteHeaderMutation`, `filter` struct, interface conformance assertions, `SetDecoderCallbacks`/`SetEncoderCallbacks`, STUBBED `DecodeHeaders`/`EncodeHeaders` (return Continue), pass-through `DecodeData`/`EncodeData`/`DecodeTrailers`/`EncodeTrailers`/`OnDestroy`.

- `internal/filter/http/header_mutation/header_mutation_test.go` (278 lines): 11 test functions — `TestNew_NilTC`, `TestNew_MalformedTC`, `TestNew_ProtectedHeader` (table-driven, 10 cases), `TestNew_ProtectedHeader_RemoveAlsoRejected`, `TestNew_HappyPath_ListenerLevelOnly`, `TestRuntimeConfig_FieldExtraction`, `TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored`, `TestCompiledMutationOp_AllAppendActionsParse` (4 sub-cases), `TestCompiledMutationOp_RemoveAndAppend`, `TestNew_RegistersPerRouteValidator`, `TestIsProtectedHeader`.

Three ADRs appended to `docs/envoy-go/DECISIONS.md` (+221 lines, 4794→5015):
- ADR-0108: package shape + boot registration + zero-stats discipline.
- ADR-0109: `runtimeConfig` 3-field shape + `compiledMutationOp` value-typed + AppendAction × 4 + keep_empty_value + multi-value.
- ADR-0111: protected-header set + CONFIG-LOAD-TIME rejection (MAJOR amendment to BRAINSTORM Decision 11) + verbatim error format + eager per-route validation.

**One deviation from PLAN verbatim source:** `TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored` adapted. The PLAN used `{AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD, Append: &corev3.HeaderValue{Key: "q", Value: "v"}}` to construct a `KeyValueMutation`. At impl time, `go doc` revealed `KeyValueMutation` lives in `corev3` (not `mutation_rules/v3`) and has a different struct shape: `Append *KeyValueAppend` + `Remove string` (no `AppendAction` field at top level). The test was adapted to use `&corev3.KeyValueMutation{Remove: "q"}` — a valid minimal construction that still exercises the silent-ignore path (the test purpose is unchanged: verify no error, no ops produced).

`gofmt` was run after initial write — the PLAN's alignment whitespace in `compiledMutationOp` struct literal was not canonical gofmt output; golangci-lint caught this and `gofmt -w` fixed it.

**Outputs:**
```
$ go test -race ./internal/filter/http/header_mutation/... -v 2>&1
=== RUN   TestNew_NilTC
--- PASS: TestNew_NilTC (0.00s)
=== RUN   TestNew_MalformedTC
--- PASS: TestNew_MalformedTC (0.00s)
=== RUN   TestNew_ProtectedHeader
=== RUN   TestNew_ProtectedHeader/method-request
--- PASS: TestNew_ProtectedHeader/method-request (0.00s)
=== RUN   TestNew_ProtectedHeader/path-request
--- PASS: TestNew_ProtectedHeader/path-request (0.00s)
=== RUN   TestNew_ProtectedHeader/authority-request
--- PASS: TestNew_ProtectedHeader/authority-request (0.00s)
=== RUN   TestNew_ProtectedHeader/scheme-request
--- PASS: TestNew_ProtectedHeader/scheme-request (0.00s)
=== RUN   TestNew_ProtectedHeader/status-request
--- PASS: TestNew_ProtectedHeader/status-request (0.00s)
=== RUN   TestNew_ProtectedHeader/host-lower-request
--- PASS: TestNew_ProtectedHeader/host-lower-request (0.00s)
=== RUN   TestNew_ProtectedHeader/host-title-request
--- PASS: TestNew_ProtectedHeader/host-title-request (0.00s)
=== RUN   TestNew_ProtectedHeader/host-upper-request
--- PASS: TestNew_ProtectedHeader/host-upper-request (0.00s)
=== RUN   TestNew_ProtectedHeader/status-response
--- PASS: TestNew_ProtectedHeader/status-response (0.00s)
=== RUN   TestNew_ProtectedHeader/host-response
--- PASS: TestNew_ProtectedHeader/host-response (0.00s)
--- PASS: TestNew_ProtectedHeader (0.00s)
=== RUN   TestNew_ProtectedHeader_RemoveAlsoRejected
--- PASS: TestNew_ProtectedHeader_RemoveAlsoRejected (0.00s)
=== RUN   TestNew_HappyPath_ListenerLevelOnly
--- PASS: TestNew_HappyPath_ListenerLevelOnly (0.00s)
=== RUN   TestRuntimeConfig_FieldExtraction
--- PASS: TestRuntimeConfig_FieldExtraction (0.00s)
=== RUN   TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored
--- PASS: TestRuntimeConfig_QueryParameterMutationsSilentlyIgnored (0.00s)
=== RUN   TestCompiledMutationOp_AllAppendActionsParse
=== RUN   TestCompiledMutationOp_AllAppendActionsParse/APPEND_IF_EXISTS_OR_ADD
--- PASS: TestCompiledMutationOp_AllAppendActionsParse/APPEND_IF_EXISTS_OR_ADD (0.00s)
=== RUN   TestCompiledMutationOp_AllAppendActionsParse/ADD_IF_ABSENT
--- PASS: TestCompiledMutationOp_AllAppendActionsParse/ADD_IF_ABSENT (0.00s)
=== RUN   TestCompiledMutationOp_AllAppendActionsParse/OVERWRITE_IF_EXISTS_OR_ADD
--- PASS: TestCompiledMutationOp_AllAppendActionsParse/OVERWRITE_IF_EXISTS_OR_ADD (0.00s)
=== RUN   TestCompiledMutationOp_AllAppendActionsParse/OVERWRITE_IF_EXISTS
--- PASS: TestCompiledMutationOp_AllAppendActionsParse/OVERWRITE_IF_EXISTS (0.00s)
--- PASS: TestCompiledMutationOp_AllAppendActionsParse (0.00s)
=== RUN   TestCompiledMutationOp_RemoveAndAppend
--- PASS: TestCompiledMutationOp_RemoveAndAppend (0.00s)
=== RUN   TestNew_RegistersPerRouteValidator
--- PASS: TestNew_RegistersPerRouteValidator (0.00s)
=== RUN   TestIsProtectedHeader
--- PASS: TestIsProtectedHeader (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.011s
```

## Task 6 — header_mutation applyOps + applyAppendAction + AppendAction × 4 + keep_empty_value boundary + multi-value tests

**Commits:** `898c3ed` — `phase 10: header_mutation applyOps + AppendAction × 4 + keep_empty_value boundary`

**Notes:** Landed the per-tier mutation-application algorithm (ADR-0109 apply-loop semantics). Two functions added to `internal/filter/http/header_mutation/header_mutation.go` after `validatePerRouteHeaderMutation` and before the `filter` struct:

- `applyOps(headers http.Header, ops []compiledMutationOp)`: iterates the ops slice in proto-declared order, dispatching to `headers.Del(op.headerName)` for `kindRemove` or `applyAppendAction(headers, op)` for `kindAppend`.

- `applyAppendAction(headers http.Header, op compiledMutationOp)`: keep_empty_value=false silent-skip fires FIRST (before the AppendAction switch per §11.2 conclusion (c)). Switch covers all 4 variants: `APPEND_IF_EXISTS_OR_ADD` → `headers.Add` (preserves existing multi-values, per §11.4); `ADD_IF_ABSENT` → `headers.Add` if `headers.Get` is empty; `OVERWRITE_IF_EXISTS_OR_ADD` → `headers.Set` (collapses multi-value to single, per §11.4); `OVERWRITE_IF_EXISTS` → `headers.Set` only if `headers.Get` is non-empty.

`net/http` import added to `header_mutation_test.go` (tests directly construct `http.Header{}`).

15 test functions added (11 top-level + 4 table-driven sub-cases in `TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions`). No ADR landed in this task (ADR-0109 already landed in Task 5).

**Outputs:**
```
$ go test -race ./internal/filter/http/header_mutation/... -v -run TestApplyOps 2>&1
=== RUN   TestApplyOps_AppendIfExistsOrAdd_AbsentTarget
--- PASS: TestApplyOps_AppendIfExistsOrAdd_AbsentTarget (0.00s)
=== RUN   TestApplyOps_AppendIfExistsOrAdd_PresentMultiValue
--- PASS: TestApplyOps_AppendIfExistsOrAdd_PresentMultiValue (0.00s)
=== RUN   TestApplyOps_AddIfAbsent_AbsentTarget
--- PASS: TestApplyOps_AddIfAbsent_AbsentTarget (0.00s)
=== RUN   TestApplyOps_AddIfAbsent_PresentTarget
--- PASS: TestApplyOps_AddIfAbsent_PresentTarget (0.00s)
=== RUN   TestApplyOps_OverwriteIfExistsOrAdd_AbsentTarget
--- PASS: TestApplyOps_OverwriteIfExistsOrAdd_AbsentTarget (0.00s)
=== RUN   TestApplyOps_OverwriteIfExistsOrAdd_PresentMultiValue
--- PASS: TestApplyOps_OverwriteIfExistsOrAdd_PresentMultiValue (0.00s)
=== RUN   TestApplyOps_OverwriteIfExists_AbsentTarget
--- PASS: TestApplyOps_OverwriteIfExists_AbsentTarget (0.00s)
=== RUN   TestApplyOps_OverwriteIfExists_PresentTarget
--- PASS: TestApplyOps_OverwriteIfExists_PresentTarget (0.00s)
=== RUN   TestApplyOps_Remove_PresentTarget
--- PASS: TestApplyOps_Remove_PresentTarget (0.00s)
=== RUN   TestApplyOps_Remove_AbsentTarget
--- PASS: TestApplyOps_Remove_AbsentTarget (0.00s)
=== RUN   TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions
=== RUN   TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions/APPEND_IF_EXISTS_OR_ADD
=== RUN   TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions/ADD_IF_ABSENT
=== RUN   TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions/OVERWRITE_IF_EXISTS_OR_ADD
=== RUN   TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions/OVERWRITE_IF_EXISTS
--- PASS: TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions (0.00s)
    --- PASS: TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions/APPEND_IF_EXISTS_OR_ADD (0.00s)
    --- PASS: TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions/ADD_IF_ABSENT (0.00s)
    --- PASS: TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions/OVERWRITE_IF_EXISTS_OR_ADD (0.00s)
    --- PASS: TestApplyOps_KeepEmptyValueFalse_EmptyValue_AllAppendActions/OVERWRITE_IF_EXISTS (0.00s)
=== RUN   TestApplyOps_KeepEmptyValueTrue_EmptyValue_AppendIfExistsOrAdd
--- PASS: TestApplyOps_KeepEmptyValueTrue_EmptyValue_AppendIfExistsOrAdd (0.00s)
=== RUN   TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_AbsentTarget
--- PASS: TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_AbsentTarget (0.00s)
=== RUN   TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_PresentTarget
--- PASS: TestApplyOps_KeepEmptyValueTrue_EmptyValue_OverwriteIfExists_PresentTarget (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.009s
```

## Task 8 — header_mutation EncodeHeaders symmetric + race-detector cycle test

**Commits:** `6169b09` — `phase 10: header_mutation EncodeHeaders symmetric + race-detector cycle test`

**Notes:** Landed the full `EncodeHeaders` body per SPEC §6.8 and the race-detector cycle test per SPEC §12 deferred decision 7. Two changes to `internal/filter/http/header_mutation/header_mutation.go`:

- `compileForResponse(msg proto.Message) []compiledMutationOp` (method on `*filter`): symmetric to `compileForRequest`; extracts `response_mutations` from the per-route `*HeaderMutationPerRoute` proto. Returns nil for nil input, wrong type, nil mutations, or compile error. Inserted after `compileForRequest`.

- Full `EncodeHeaders` body (replaces the Task 5 stub): applies `f.cfg.responseOps` first; if `f.dcb` is nil returns Continue (listener-only path); calls `f.dcb.RequestRouteConfigsAllTiers()` (DECODER-ONLY per planner-time decision 1 — same callback used for both decode and encode sides, mirrors cors precedent); compiles each tier via `compileForResponse`; applies in flag-controlled order: flag=false → Route→VHost→RC; flag=true → RC→VHost→Route; returns Continue.

No new ADRs in this task (ADR-0109 + ADR-0110 already landed).

4 new test functions added to `header_mutation_test.go` (+ `import "sync"` added): `TestEncodeHeaders_Symmetric`, `TestEncodeHeaders_MultiTier_FlagFalse_ResponseSide`, `TestEncodeHeaders_MultiTier_FlagTrue_ResponseSide`, `TestHeaderMutation_MultiTierConcurrentRequests`. The concurrent test spawns 64 goroutines each constructing a fresh `*filter` from the same shared factory (shared `*runtimeConfig`) and calling `DecodeHeaders` + `EncodeHeaders`; validates the read-only-after-New invariant under the race detector.

One minor formatting deviation: the PLAN's `TestEncodeHeaders_MultiTier_FlagTrue_ResponseSide` struct literal had a trailing comma after `MostSpecificHeaderMutationsWins: true` that triggered a golangci-lint gofmt violation. Fixed by running `gofmt -w` on the test file.

**Outputs:**
```
$ go test -race ./internal/filter/http/header_mutation/... -v -run 'TestEncodeHeaders|TestHeaderMutation_MultiTierConcurrent' 2>&1
=== RUN   TestEncodeHeaders_Symmetric
--- PASS: TestEncodeHeaders_Symmetric (0.00s)
=== RUN   TestEncodeHeaders_MultiTier_FlagFalse_ResponseSide
--- PASS: TestEncodeHeaders_MultiTier_FlagFalse_ResponseSide (0.00s)
=== RUN   TestEncodeHeaders_MultiTier_FlagTrue_ResponseSide
--- PASS: TestEncodeHeaders_MultiTier_FlagTrue_ResponseSide (0.00s)
=== RUN   TestHeaderMutation_MultiTierConcurrentRequests
--- PASS: TestHeaderMutation_MultiTierConcurrentRequests (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	(cached)
```

## Task 7 — header_mutation DecodeHeaders + multi-tier resolution [ADR-0110, ADR-0073 amendment]

**Commits:** `a242b78` — `phase 10: header_mutation DecodeHeaders multi-tier + ADR-0110 + ADR-0073 amendment`

**Notes:** Landed the full `DecodeHeaders` body per SPEC §6.6 + ADR-0110. Two additions to `internal/filter/http/header_mutation/header_mutation.go`:

- `compileForRequest(msg proto.Message) []compiledMutationOp` (method on `*filter`): type-asserts the per-route `proto.Message` to `*HeaderMutationPerRoute`, extracts `request_mutations`, compiles via `compileOps`. Returns nil for nil input, wrong type, nil mutations, or compile error (defensive — the per-route validator at HCM-build time already rejects protected-header violations). Inserted between `applyAppendAction` and `type filter struct`.

- Full `DecodeHeaders` body (replaces the Task 5 stub): applies `f.cfg.requestOps` first; if `f.dcb` is nil returns Continue (listener-only path); calls `f.dcb.RequestRouteConfigsAllTiers()` to get the 3 unmerged per-route messages; compiles each via `compileForRequest`; applies in flag-controlled order: flag=false (default) → Route→VHost→RC (RC applied last, wins overlap); flag=true → RC→VHost→Route (Route applied last, wins overlap); returns Continue.

ADR-0110 appended to `docs/envoy-go/DECISIONS.md`: codifies the 3 framework additions (ResolveAllTiers, RequestRouteConfigsAllTiers DECODER-ONLY, RegisterPerRouteValidator), the per-filter accessor-choice discipline, the cross-tier ordering algorithm, 4 alternatives A/B/C/D with D accepted.

ADR-0073 amendment paragraph appended at the END of the ADR-0073 section (after the "Lands-in-task" paragraph, before the `---` separator): forward-pointer noting most-specific-override is now the DEFAULT model; multi-tier filters use ResolveAllTiers per ADR-0110.

8 new test functions added to `header_mutation_test.go`: `fakeDecoderCB` helper + `mkPerRoute` + `mkFilterFromMutation` + 7 `TestDecodeHeaders_*` tests covering listener-only, route-only, 3-tier flag=false, 3-tier flag=true, 2-of-3 combinations (RouteAndVHost, RouteAndRC, VHostAndRC), and nil-dcb path.

**Outputs:**
```
$ go test -race ./internal/filter/http/header_mutation/... -run TestDecodeHeaders -v 2>&1
=== RUN   TestDecodeHeaders_ListenerLevel_NoPerRoute
--- PASS: TestDecodeHeaders_ListenerLevel_NoPerRoute (0.00s)
=== RUN   TestDecodeHeaders_PerRoute_RouteOnly
--- PASS: TestDecodeHeaders_PerRoute_RouteOnly (0.00s)
=== RUN   TestDecodeHeaders_MultiTier_FlagFalse
--- PASS: TestDecodeHeaders_MultiTier_FlagFalse (0.00s)
=== RUN   TestDecodeHeaders_MultiTier_FlagTrue
--- PASS: TestDecodeHeaders_MultiTier_FlagTrue (0.00s)
=== RUN   TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndVHost
--- PASS: TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndVHost (0.00s)
=== RUN   TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndRC
--- PASS: TestDecodeHeaders_MultiTier_TwoOfThree_RouteAndRC (0.00s)
=== RUN   TestDecodeHeaders_MultiTier_TwoOfThree_VHostAndRC
--- PASS: TestDecodeHeaders_MultiTier_TwoOfThree_VHostAndRC (0.00s)
=== RUN   TestDecodeHeaders_NilDecoderCallbacks_AppliesListenerOnly
--- PASS: TestDecodeHeaders_NilDecoderCallbacks_AppliesListenerOnly (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.011s
```

## Task 9 — cmd/envoy-go/main.go register header_mutation.New

**Commits:** `1246525` — `phase 10: register header_mutation.New under header_mutation.TypeURL`

**Notes:** Landed boot-time registration for header_mutation per ADR-0072 + ADR-0108. Two changes to `cmd/envoy-go/main.go`:

- Import line: added `"github.com/esalaine/envoy-go/internal/filter/http/header_mutation"` to the imports block, alphabetically between `fault` and `router` (lines 28–32).

- Registration line: added `httpReg.Register(header_mutation.TypeURL, header_mutation.New)` in the httpReg.Register block, between `httpReg.Register(fault.TypeURL, fault.New)` and `httpReg.Freeze()` (lines 113–118).

Resulting register block (5 filter registrations + Freeze):
```go
httpReg := filter_http.NewHTTPRegistry()
	httpReg.Register(router.TypeURL, router.New)
	httpReg.Register(cors.TypeURL, cors.New)
	httpReg.Register(envoygotest.TypeURL, envoygotest.New)
	httpReg.Register(fault.TypeURL, fault.New)
	httpReg.Register(header_mutation.TypeURL, header_mutation.New)
	httpReg.Freeze()
```

Build/vet/test all green.

**Outputs:**
```
$ go build ./cmd/envoy-go && go vet ./... && echo "Build and vet OK"
Build and vet OK
$ go test -race -count=1 ./internal/filter/http/header_mutation/... -run 'Test' 2>&1 | tail -5
=== RUN   TestDecodeHeaders_NilDecoderCallbacks_AppliesListenerOnly
--- PASS: TestDecodeHeaders_NilDecoderCallbacks_AppliesListenerOnly (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	1.038s
```

## Task 10 — FuzzHeaderMutationConfigParse (thirteenth fuzzer per ADR-0018)

**Commits:** `4a82931` — `phase 10: FuzzHeaderMutationConfigParse (thirteenth fuzzer per ADR-0018)`

**Notes:** Landed `FuzzHeaderMutationConfigParse` in `internal/filter/http/header_mutation/fuzz_test.go` (37 lines). Fuzzes arbitrary byte sequences as the `tc *anypb.Any` parameter to `New`; asserts `New` returns either `(factory, nil)` OR `(nil, error)`; never panics; never returns `(nil, nil)`. Per ADR-0018's "every parser/codec/filter ships a fuzzer" + SPEC §14.3 planner-time decision 6. Three seed corpus entries: empty TypeURL + empty bytes (invalid); arbitrary bytes under canonical TypeURL (decode error); short proto-wire-format bytes.

30s budget per ADR-0018 short-mode CI policy. Thirteenth fuzzer overall (post-09's twelfth FuzzFaultConfigParse).

**Outputs:**
```
$ go test -fuzz=FuzzHeaderMutationConfigParse -fuzztime=30s ./internal/filter/http/header_mutation/... 2>&1 | tail -10
fuzz: elapsed: 12s, execs: 3120504 (273195/sec), new interesting: 157 (total: 160)
fuzz: elapsed: 15s, execs: 3827812 (235616/sec), new interesting: 169 (total: 172)
fuzz: elapsed: 18s, execs: 4460966 (211225/sec), new interesting: 187 (total: 190)
fuzz: elapsed: 21s, execs: 5143485 (227558/sec), new interesting: 200 (total: 203)
fuzz: elapsed: 24s, execs: 5829933 (228759/sec), new interesting: 207 (total: 210)
fuzz: elapsed: 27s, execs: 6218452 (129505/sec), new interesting: 215 (total: 218)
fuzz: elapsed: 30s, execs: 6545485 (108999/sec), new interesting: 219 (total: 222)
fuzz: elapsed: 31s, execs: 6545485 (0/sec), new interesting: 219 (total: 222)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/header_mutation	31.061s
```

## Task 11 — Fixture infrastructure — HTTPHeaderMutation BackendKind + spawn helper + driver stub

**Commits:** `4078c45` — `phase 10: fixture infrastructure — HTTPHeaderMutation BackendKind + spawn helper + driver stub`

**Notes:** Landed the three fixture-harness infrastructure pieces for fixture 0012. (1) `HTTPHeaderMutation BackendKind = 9` appended after `HTTPFault BackendKind = 8` in `test/differential/fixture/fixture.go` with full doc-comment per task spec. (2) `startHTTPHeaderMutationBackend` spawn helper added to `test/differential/runner_test.go` mirroring `startHTTPFaultBackend` signature exactly (`func(ctx context.Context, repoRoot string, port int) (*exec.Cmd, error)`; error wrapped via `fmt.Errorf("start: %w", err)`). (3) `case fixture.HTTPHeaderMutation:` block added in `runFixture` switch, mirroring the `HTTPFault` case verbatim (bo.port + bo.proc + deferred SIGKILL + waitTCPDial). (4) Blank-import `_ "github.com/esalaine/envoy-go/test/fixtures/0012-http-header-mutation/driver"` appended after the 0011 import. (5) Stub `test/fixtures/0012-http-header-mutation/driver/doc.go` created (package-level doc comment only). No signature deviations from PLAN sketches — actual `startHTTPFaultBackend` matched the sketch's return shape. `go build ./test/differential/... ./test/fixtures/0012-http-header-mutation/...`, `go vet ./...`, `golangci-lint run ./...`, and `go test -race -count=1 ./...` all clean.

**Outputs:**
```
$ go build ./test/differential/... ./test/fixtures/0012-http-header-mutation/...
(no output — clean)
$ go vet ./...
(no output — clean)
$ golangci-lint run ./...
(no output — clean)
$ go test -race -count=1 ./... 2>&1 | tail -5
ok  	github.com/esalaine/envoy-go/test/differential	40.967s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.014s
?   	github.com/esalaine/envoy-go/test/fixtures/0012-http-header-mutation/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.025s
```

## Task 12 — Fixture 0012 backend.go (header echo + multi-value response)

**Commits:** `4910547` — `phase 10: fixture 0012 backend — header echo + multi-value response headers`

**Notes:** Created `test/fixtures/0012-http-header-mutation/backends/backend.go` (51 lines): HTTP server bound to `--port` flag (default 18012), `/` endpoint reflects sorted request headers into response body (one per line), emits single-value `X-Resp-Test: backend-original` + multi-value `X-Multi: alpha` + `X-Multi: beta` headers for OVERWRITE/APPEND testing per SPEC §7.5 + §11.4. Smoke test via curl: returned HTTP 200 OK with sorted headers in body + all expected response headers.

**Outputs:**
```
$ go build ./test/fixtures/0012-http-header-mutation/backends/
(no output — clean)
$ go run ./test/fixtures/0012-http-header-mutation/backends/ --port 18099 &
sleep 0.3
curl -isS http://127.0.0.1:18099/ -H 'X-Probe: yes' | head -30
kill $PID

HTTP/1.1 200 OK
Content-Length: 48
Content-Type: text/plain
X-Multi: alpha
X-Multi: beta
X-Resp-Test: backend-original
Date: Mon, 04 May 2026 17:15:31 GMT

Accept: */*
User-Agent: curl/8.5.0
X-Probe: yes
```

## Task 13 — Fixture 0012 envoy.yaml + envoy-go.yaml dual-listener bootstraps per SPEC §7.4

**Commits:** `237ecad` — `phase 10: fixture 0012 envoy.yaml + envoy-go.yaml dual-listener bootstrap per SPEC §7.4`

**Notes:** Created the two bootstrap files for fixture 0012-http-header-mutation. Templating convention mirrors `test/fixtures/0011-http-fault/` exactly:

- **envoy.yaml** (reference, runs in Docker via runner): cluster `c_backend` uses `STRICT_DNS` + `dns_lookup_family: V4_ONLY`; address template `{{.BackendHost}}:{{.BackendPort}}`; admin/listener ports are FIXED (9912, l_lws :10012, l_mws :10013 — reference side has no dynamic port allocation).
- **envoy-go.yaml** (subject, runs host-side): cluster `c_backend` uses `STATIC` at `127.0.0.1:{{.BackendPort}}`; admin/listener ports use templates `{{.AdminPort}}` / `{{.LwsPort}}` / `{{.MwsPort}}` per the two-listener shape (driver PLAN §Task 15 struct `{AdminPort, LwsPort, MwsPort, BackendPort int}`).

Both files: two listeners `l_lws` (`most_specific_header_mutations_wins: false`) + `l_mws` (`most_specific_header_mutations_wins: true`) with IDENTICAL per-route tier configurations at RC / VirtualHost / Route tiers. Per-route config present at all three tiers for `/multi-tier`; only route tier for `/route-override`; no per-route config for `/listener-only`. Listener-level mutations exercise all 4 AppendActions + Remove + `keep_empty_value` on `l_lws`; abbreviated listener mutations on `l_mws`.

Docker validation: envoy.yaml passed `--mode validate` with templates substituted (BackendHost=host.docker.internal, BackendPort=18012); Envoy accepted 2 listeners and 1 cluster without errors. `go vet ./...`, `golangci-lint run ./...`, and `go test -race -count=1 ./...` all clean.

**Outputs:**
```
$ docker run --rm -v "/tmp/envoy-validate.yaml:/etc/envoy/envoy.yaml" envoyproxy/envoy:v1.37.2 envoy -c /etc/envoy/envoy.yaml --mode validate 2>&1 | tail -5
[2026-05-04 17:18:54.866][1][info][config] [source/server/configuration_impl.cc:132] loading 0 static secret(s)
[2026-05-04 17:18:54.866][1][info][config] [source/server/configuration_impl.cc:138] loading 1 cluster(s)
[2026-05-04 17:18:54.866][1][info][config] [source/server/configuration_impl.cc:148] loading 2 listener(s)
configuration '/etc/envoy/envoy.yaml' OK
[2026-05-04 17:18:54.869][1][info][config] [source/server/configuration_impl.cc:164] loading stats configuration
$ go test -race -count=1 ./... 2>&1 | tail -5
ok  	github.com/esalaine/envoy-go/test/differential	41.836s
ok  	github.com/esalaine/envoy-go/test/differential/fixture	1.032s
?   	github.com/esalaine/envoy-go/test/fixtures/0012-http-header-mutation/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/helpers	1.030s
```
