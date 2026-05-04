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
