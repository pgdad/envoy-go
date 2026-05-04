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
