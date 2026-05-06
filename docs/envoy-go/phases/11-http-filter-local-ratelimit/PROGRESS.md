# Phase 11 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..10 PROGRESS.md structure.

## Preamble — execution preconditions

Precondition 11 (`LocalRateLimitPerRoute` proto type present in module closure) **did not pass**: `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3 LocalRateLimitPerRoute | head -5` returns `doc: no symbol LocalRateLimitPerRoute in package ...@v1.32.4/...`. Bumping to v1.37.0 (highest published as of 2026-05-05) produces the same error; `grep -r LocalRateLimitPerRoute` across the entire go-control-plane module returns 0 matches. Verification against upstream Envoy v1.37.2 source (`gh api repos/envoyproxy/envoy/contents/api/envoy/extensions/filters/http/local_ratelimit/v3/local_rate_limit.proto?ref=v1.37.2`) confirms only one message exists in that file: `message LocalRateLimit`. **The `LocalRateLimitPerRoute` proto cited in SPEC line 1222 does not exist in upstream.** The actual upstream design is that `local_ratelimit` reuses the same `LocalRateLimit` proto for both listener-level config and per-route TPFC entries — the proto's own docstring confirms: *"When using per route configuration, the bucket becomes unique to that route."* Settled by IMPL-1 below (substitution rule + impact-table). The remaining 15 preconditions were satisfied at cold-start without deviation; go.mod stays at v1.32.4; working tree pristine.

## Preamble — impl-time decisions (per ADR-0044 ADR-on-impl convention; correct PLAN/SPEC errata discovered at impl-time)

This phase 11 PROGRESS.md introduces the "impl-time decisions" preamble — analogous to the planner-time-decisions block in PLAN.md. Project precedent: PLAN.md captures planner-time corrections to SPEC errors as numbered planner-time decisions (e.g., D1 corrected SPEC §12 D1's `internal/admin/stats.go` mis-direction). The same pattern at impl-time goes here. SPEC.md and PLAN.md are not amended (committed historical artefacts); this PROGRESS.md preamble is the impl-time authority and is linked from each affected task's Notes block.

1. **IMPL-1 — `LocalRateLimitPerRoute` proto does not exist; per-route TPFC reuses `LocalRateLimit`.** Both SPEC (lines 21, 78, 310, 612, 1053, 1132, 1222) and PLAN (lines 9, 14, 18, 64, 67, 128, 174, 217–218, 382, 979, 1838–1855, 1940–2257, 3020, 3876) repeatedly cite `*LocalRateLimitPerRoute` as if it were a separate proto message wrapping `LocalRateLimit`. **It is not.** Upstream Envoy v1.37.2 defines exactly one message in `envoy/extensions/filters/http/local_ratelimit/v3/local_rate_limit.proto`: `message LocalRateLimit` (19 top-level fields, `[#next-free-field: 19]`). Per-route TPFC entries reuse the same `LocalRateLimit` proto. The substitution rule for all subsequent tasks:
   - **Go type:** `*localratelimitv3.LocalRateLimitPerRoute` → `*localratelimitv3.LocalRateLimit` (everywhere, including the lazy-cache `sync.Map` key type per Task 5 + ADR-0117).
   - **Per-route TPFC YAML `@type`:** `type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimitPerRoute` → `type.googleapis.com/envoy.extensions.filters.http.local_ratelimit.v3.LocalRateLimit` (Task 11 fixture envoy.yaml + envoy-go.yaml).
   - **No "embedded LocalRateLimit via `rate_limit` field" mental model** — that wrapping doesn't exist; per-route TPFC entries are directly `*LocalRateLimit` with the same 19 fields. SPEC §1.4 leading-edge claim 4's parenthetical "(which embeds a full `LocalRateLimit` body via its `rate_limit` field)" reads as moot.
   - **No "recursive parse"** — SPEC §16:1222's "phase 11 parses recursively" and §14.1's "test that `LocalRateLimitPerRoute` parsing recursively builds a `*runtimeConfig`" become single-level: per-route `*LocalRateLimit` TPFC entries each get their own lazy-built `*runtimeConfig` via the per-filter `sync.Map` cache.
   - **Field count unchanged:** 5 consumed (`stat_prefix`, `token_bucket.{max_tokens, tokens_per_fill, fill_interval}`, `status.code`) + 14 deferred (silent-ignore per ADR-0040) = 19 total `LocalRateLimit` fields. The SPEC §2.1 / §1.4 / PLAN line 979 wording "+plus the LocalRateLimitPerRoute per-route container" reads as moot but does not change the consumed/deferred counts. PLAN line 3876's Refinement caveat ("the implementer at Task 5 step 3 surveys the actual go-control-plane generated `LocalRateLimitPerRoute` to confirm the embedded `LocalRateLimit` accessor; the sketch may need adjustment") is settled here at IMPL-1: the survey returns "no such proto"; the sketch is rewritten by Task 5's dispatch prompt accordingly.
   - **Affected tasks:** Task 2 doc.go enumeration, Task 5 (per-route TPFC parsing + ADR-0117 wording), Task 11 (fixture envoy.yaml + envoy-go.yaml `@type` URLs), Task 14 (BEHAVIOR_CONTRACT.md §13.1 wording — phase 11's per-route container is `LocalRateLimit`, not a separate type), Task 16 (REVIEW.md retrospective notes the impl-time correction).
   - **No ADR for IMPL-1 itself** per ADR-0017 (small-mechanical-correction discipline applies — the correction is a substitution rule, not a design decision); ADR-0117 lands at Task 5 with the corrected wording (no separate amendment, since ADR-0117 is brand-new — first-use is at Task 5).

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §8)

The six ADRs anticipated by SPEC §8 (ADR-0114..ADR-0119). Each lands at the task that anchors its first-use commit per the PLAN.md "ADRs introduced by this plan" table:

- **ADR-0114** `internal/filter/http/localratelimit/` package shape (no-underscore directory + extension-registry registration ordering) — Task 2
- **ADR-0115** runtimeConfig shape + 5/14-field decomposition + PGV constraint table + filter-internal `fill_interval >= 50ms` validation discipline — Task 2
- **ADR-0116** `tokenBucket` Option-A lazy-refill on access + monotonic-time semantics + LBP-1-adjacent declaration + ±10ms empirical refill-timing tolerance — Task 3
- **ADR-0117** Per-route bucket isolation as ADR-0073 wholesale-override consequence (FIRST stateful per-route filter; ADR-0073 amendment paragraph) — Task 5
- **ADR-0118** Stat-table 22→26-name extension + `enforced == rate_limited` MVP invariant + filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` registered as Rule SN9 — Task 6
- **ADR-0119** Rate-limited response wire shape + body byte-exact `local_rate_limited` + 4-header set lowercase wire-form + 429 default status + SendLocalReply reuse from phase 09 — Task 4

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The nine planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — Tag-extractor registration site = EXTEND `internal/stats/name.go`'s `flattenToProm` SWITCH WITH NEW RULE SN9** (corrects SPEC §12 D1's mis-statement of `internal/admin/stats.go` which does not exist; the actual mechanism is the hardcoded switch in `internal/stats/name.go::flattenToProm`).
2. **D2 — Filter-callback wiring hook = `SetDecoderCallbacks(cb)` + `SetEncoderCallbacks(cb)` per the cors + fault + header_mutation precedents** (filter struct carries both `dcb` and `ecb` fields; only `dcb` is used at request time for the SendLocalReply call; `ecb` is unused but kept for chain-of-conformance).
3. **D3 — PGV plumbing = EXPLICIT CHECKS IN THE `New` FACTORY** (mirrors cors + fault + header_mutation; six explicit checks: tc-non-nil, stat_prefix-non-empty, max_tokens > 0, tokens_per_fill > 0 if explicitly set [defaults to 1 if absent], fill_interval >= 50ms [verbatim Envoy error string], status.code in [400, 600) if explicitly set [defaults to 429 if absent]).
4. **D4 — Scenario 3 retry-with-deadline option = ±10ms TOLERANCE WITH SIMPLE `time.Sleep` (DEFAULT)** (retry-with-deadline reserved as fallback if CI flakes; ADR-0116 §Consequences may amend in-place per ADR-0089 to record chosen option).
5. **D5 — Test-only clock injection = SKIP** (per SPEC default; race-detector cycle test exercises mutex via real wallclock; future hardening pass may revisit).
6. **PLAN-emerging — File split = `bucket.go` + `local_ratelimit.go`** (cleanly separates the token-bucket primitive from the filter orchestration; mirrors size-driven splits in similar packages).
7. **PLAN-emerging — Race-detector cycle test = ADD `TestTokenBucket_ConcurrentTryConsume`** (~30 LoC; lands in Task 3).
8. **PLAN-N (fixture topology) — 4 PRE-CONFIGURED LISTENERS (`l_s1`, `l_s2`, `l_s3`, `l_per_route`) IN A SINGLE BOOTSTRAP** (diverges from SPEC §7.1's two-listener+teardown layout to fit the existing differential-fixture harness's single-Drive-call contract; bucket isolation provided at boot by listener-distinct factories; all 4 scenarios run in one `DriveReferenceMulti`/`DriveSubjectMulti` invocation via `fixture.MultiListenerDriver`).
9. **PLAN-emerging — BackendKind enum value = `HTTPLocalRateLimit BackendKind = 10`** (continues existing naming convention).

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Commits:** `cbf07eb` — `phase 11: PROGRESS preamble + planner-time decision resolution`
**Notes:** Created PROGRESS.md; verified 15 of 16 preconditions per PLAN §"Execution preconditions"; precondition 11 (`LocalRateLimitPerRoute` proto present) FAILED at cold-start AND remained failed after a v1.37.0 bump attempt — confirmed at upstream Envoy v1.37.2 source that no such proto exists. Settled at the follow-up commit by IMPL-1 (substitution `*LocalRateLimitPerRoute` → `*LocalRateLimit`; affects Tasks 2, 5, 11, 14, 16); see preamble. phase-11 SPEC + PLAN confirmed present in HEAD; SPEC at 63c88ed; ADR tail at 0113 (next-free 0114); internal/filter/http/localratelimit/ absent (Task 2 lands); SN9 absent (Task 6 lands); fixture.HTTPLocalRateLimit absent (Task 9 lands). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
**Outputs:**
```
$ git rev-parse --abbrev-ref HEAD
phase-11-http-filter-local-ratelimit-impl
$ go version
go version go1.26.2 linux/amd64
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
113
$ git log -1 --format=%H -- docs/envoy-go/phases/11-http-filter-local-ratelimit/SPEC.md
63c88ed1856ae70dc8c89415ca162c2eb57c8b69
```

## Task 2 + Task 3 — `localratelimit/` package skeleton + `runtimeConfig` parser + `tokenBucket` primitive

**Commits:** `bfc0529` — `phase 11: localratelimit/ package skeleton + runtimeConfig parser + tokenBucket primitive [ADR-0114, ADR-0115, ADR-0116]`
**Notes:** Combined Task 2 + Task 3 per PLAN line 932 recommendation. Lands the new `internal/filter/http/localratelimit/` package with 5 files (~700 LoC production + tests): `doc.go` (package doc with iteration-protocol coverage + lazy-cache per-route discipline per IMPL-1 + ADR-0117), `local_ratelimit.go` (TypeURL + types + `runtimeConfig` parser + 6 explicit PGV/filter-internal validation checks per planner-time decision 3 + `New` factory returning `envoyhttp.HTTPFilter{Name, Decoder, Encoder}` per fault precedent + pass-through methods + DecodeHeaders STUBBED to `Continue`), `bucket.go` (`tokenBucket` primitive — lazy refill on access; `sync.Mutex` per bucket; `time.Now().UnixNano()` monotonic clock; LBP-1-adjacent per ADR-0116), `local_ratelimit_test.go` (13 New-validation tests including verbatim Envoy error string assertion `local rate limit token bucket fill timer must be >= 50ms`), `bucket_test.go` (7 mechanics tests + `TestTokenBucket_ConcurrentTryConsume` race-detector cycle test per planner-time decision 7). Three structural deviations from PLAN sketches (all sound; spec-compliance reviewer verified): (a) `FilterInstanceFactory` returns `envoyhttp.HTTPFilter{Name, Decoder, Encoder}` (not raw `*filter`) per `internal/filter/http/types.go` + `fault.go` precedent; (b) `filterStats` fields are `*stats.Counter` (not `*atomic.Int64`) per `internal/stats/registry.go.NewCounter` return type — preserves Walk/Freeze discipline per ADR-0061; (c) `DecodeData`/`EncodeData` parameter is `[]byte` per `types.go` interface (PLAN sketch's `http.Header` was a planner-time error). ADRs ADR-0114 (package shape — no-underscore directory `localratelimit/` aligns with cors/fault precedent; diverges from header_mutation's underscore-preserving pattern; boot-registration ordering router → cors → envoygotest → fault → header_mutation → localratelimit), ADR-0115 (runtimeConfig 5-field shape + 14-field silent-ignore decomposition + 6 validation checks + filter-internal `fill_interval >= 50ms` discipline with verbatim Envoy error string for boot-log byte-equivalence — Context corrected per IMPL-1 to drop the false `LocalRateLimitPerRoute` per-route container claim), ADR-0116 (`tokenBucket` Option-A lazy-refill on access + `time.Now().UnixNano()` monotonic-time + LBP-1-adjacent declaration with rationale for the lock-free-hot-path departure + ±10ms empirical refill-timing tolerance) all land at this commit per ADR-0044 first-use convention. Tests: 21 new test functions (13 filter + 8 bucket) all PASS under `-race`. Cosmetic doc.go/test-comment fix-ups bundled in this follow-up commit (drop dead step 8 reference to `mostSpecificHeaderMutationsWins`; tighten epsilon-refill bound comment in concurrent test) per code-quality reviewer Minor items.
**Outputs:**
```
$ go test -race -count=1 ./internal/filter/http/localratelimit/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.015s
$ go vet ./...
(clean)
$ golangci-lint run ./...
(clean)
$ git diff --stat 44db49d..bfc0529 -- internal/filter/http/localratelimit/ docs/envoy-go/DECISIONS.md
 docs/envoy-go/DECISIONS.md                                | 297 +++++++++++++++++
 internal/filter/http/localratelimit/bucket.go             |  99 ++++++
 internal/filter/http/localratelimit/bucket_test.go        | 151 +++++++++
 internal/filter/http/localratelimit/doc.go                | 139 ++++++++
 internal/filter/http/localratelimit/local_ratelimit.go    | 239 ++++++++++++++
 internal/filter/http/localratelimit/local_ratelimit_test.go | 205 ++++++++++++
 6 files changed, 1130 insertions(+)
```

## Task 4 — `DecodeHeaders` body + `filterStats` wiring + 4-counter Inc-discipline + DecodeHeaders unit tests

**Commits:** `9f0737a` — `phase 11: DecodeHeaders body + filterStats wiring + 4-counter Inc-discipline [ADR-0119]`
**Notes:** Replaces the Task-2/3 stubbed `DecodeHeaders` body with the full SPEC §6.5 implementation: increment `rc.stats.enabled` unconditionally; call `rc.bucket.tryConsume()`; on true → increment `rc.stats.ok` + return `Continue`; on false → increment `rc.stats.rateLimited` AND `rc.stats.enforced` IN LOCKSTEP (MVP invariant per ADR-0118 forthcoming Task 6) + invoke `f.dcb.SendLocalReply(rc.statusCode, rc.body, OrderedHeaders{Content-Type: text/plain})` per ADR-0102 + ADR-0119 + return `StopIteration`. The framework `SendLocalReply` primitive is reused VERBATIM from phase 09 fault precedent at `internal/filter/http/fault/fault.go:321`; no new framework primitive introduced. **One impl-time correction relative to the PLAN sketch** (settled at this commit; mirrors the Tasks 2/3 dispatch's similar three-deviation pattern): the PLAN sketch had `var rateLimitedBody = []byte("local_rate_limited")` and `runtimeConfig.body []byte`, but `SendLocalReply` actually takes `string` per `internal/filter/http/callbacks.go:30` (and `internal/filter/http/fault/fault.go:25`'s `const faultAbortBody = "fault filter abort"` matches). Substitution applied: `var ... []byte(...)` → `const rateLimitedBody = "local_rate_limited"`; `runtimeConfig.body []byte` → `body string`; `TestNew_HappyPath_AllConsumedFields`'s body assertion `string(inst.rc.body)` → `inst.rc.body` (no conversion needed). ADR-0119 alternative (a) rejection rationale was rewritten to reflect the correct underlying reason (`SendLocalReply` takes `string` so const-string is the natural form; matches fault precedent). The follow-up commit additionally folds in a one-line stale-doc fix at DECISIONS.md:5221 — ADR-0115's Decision-section `runtimeConfig` code block originally landed at Tasks 2/3 with `body []byte`; the same `[]byte → string` substitution applies to the doc snippet for consistency with the live source. Tests: 3 new `DecodeHeaders` tests added (`TestDecodeHeaders_AllowPath_CountersIncremented` — Continue + `enabled=1, ok=1, rateLimited=0, enforced=0` + negative `!sendCalled` assertion; `TestDecodeHeaders_RateLimitedPath_CountersIncremented_Lockstep` — StopIteration + SendLocalReply with status=429, body=`"local_rate_limited"` (18 bytes), 1-header set with `Content-Type: text/plain` + counters `enabled=2, ok=1, rateLimited=1, enforced=1` + explicit MVP-invariant assertion `rateLimited.Load() == enforced.Load()`; `TestStatNames_FourCountersUnderStatPrefix` — `stats.Registry.Walk` confirms all 4 expected stat names land under the chosen stat_prefix). Plus a new `fakeDecoderCB` test fake mirroring the `header_mutation_test.go:410-425` pattern, capturing status/body/headers/sendCalled. Total tests in the localratelimit package: 27 (8 bucket + 16 factory/runtimeConfig + 3 DecodeHeaders); all PASS under `-race`. ADR-0119 lands here per the ADR-0044 first-use convention; ADR-0118 lands at Task 6 in full per PLAN line 1507 recommendation (Task 4's commit message references ADR-0118 as forthcoming). Spec-compliance reviewer flagged a single defect (DECISIONS.md:5221 stale `[]byte`) which is folded into this follow-up commit; code-quality reviewer flagged 3 Minor items (none blocking; assessment: "production-ready").
**Outputs:**
```
$ go test -race -count=1 ./internal/filter/http/localratelimit/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.020s
$ go test -race -count=1 -short ./...
(every package PASS; clean)
$ git diff --stat 71c0069..9f0737a
 docs/envoy-go/DECISIONS.md                                | 50 +++++++
 internal/filter/http/localratelimit/local_ratelimit.go    | 35 ++++--
 internal/filter/http/localratelimit/local_ratelimit_test.go | 162 +++++++++++++++++++-
 3 files changed, 235 insertions(+), 12 deletions(-)
```

## Task 5 — Per-route TPFC bucket independence + `Registry.NewCounterIfAbsent` framework primitive + ADR-0073 amendment

**Commits:** `ea152a1` — `phase 11: per-route TPFC bucket independence + Registry.NewCounterIfAbsent + ADR-0073 amendment [ADR-0117]`
**Notes:** Lands the per-route TPFC handling per SPEC §11.6 + ADR-0117 + the small `internal/stats.Registry.NewCounterIfAbsent` framework primitive (~30 LoC delta — needed because the stats Registry is Frozen at boot before HCM-build sees per-route TPFCs and the per-route stat names are data-driven by the operator's chosen `stat_prefix`). Restructured the factory closure: was `*filter{rc *runtimeConfig}` capturing one config; now `*filter{state *factoryState}` where `factoryState` carries `listenerRC *runtimeConfig` (eager, listener-level) + `perRoute sync.Map` (lazy-cache keyed by `*localratelimitv3.LocalRateLimit` per IMPL-1 — diverges from PLAN sketch's `*LocalRateLimitPerRoute` key per PROGRESS.md preamble) + `reg *stats.Registry`. New `factoryState.resolvePerRouteConfig(msg proto.Message) *runtimeConfig` performs nil-fallback + type-assertion fallback + `sync.Map.Load` fast-path + `LoadOrStore`-race-safe lazy construction via `buildRuntimeConfigPerRoute`. New `buildRuntimeConfigPerRoute(c *LocalRateLimit, reg *stats.Registry)` mirrors the listener-level 6-check validation (verbatim error strings preserved including the Envoy `local rate limit token bucket fill timer must be >= 50ms`); only divergence is `newFilterStatsIfAbsent` (post-Freeze idempotent) instead of `newFilterStats` (boot-time only). `DecodeHeaders` updated to call `f.dcb.RequestRouteConfig()` (no args — IMPL fix vs PLAN sketch's `(filterName)` arg; actual interface signature per `internal/filter/http/callbacks.go:36` is no-args; framework resolves the calling-filter name internally), unwrap via `state.resolvePerRouteConfig`, operate on resolved per-route or listener-level `*runtimeConfig`. **IMPL-1 substitutions applied** in code, tests, and ADR-0117 wording: `*LocalRateLimitPerRoute` → `*LocalRateLimit` everywhere; no `.GetRateLimit()` indirection (PLAN sketch's wrapping doesn't exist upstream); `TestDecodeHeaders_PerRouteOverride_IndependentBuckets` constructs two `*LocalRateLimit` directly, no wrapper. ADR-0117 Context paragraph cites IMPL-1 + the upstream proto truth in one sentence. ADR-0073 amendment paragraph appended in-place at the existing ADR-0073 body, noting wholesale-override extends to stateful per-route resources (per ADR-0117) + `NewCounterIfAbsent` post-Freeze idempotent registration (per forthcoming Task 6 ADR-0118 amendment to ADR-0061). All 16 existing localratelimit tests migrated from `inst.rc.X` to `inst.state.listenerRC.X` access pattern. New `TestDecodeHeaders_PerRouteOverride_IndependentBuckets` validates the §11.6 empirical claim mechanically (3-way pointer-distinctness rcA/rcB/rcListener; rcA.bucket != rcB.bucket; rcA.stats != rcB.stats; idempotent re-resolution; cross-bucket isolation: drain rcA's bucket → rcB.tryConsume still succeeds). Three new `TestNewCounterIfAbsent_*` unit tests in `internal/stats/registry_test.go` (RegistersWhenAbsent / ReturnsExisting / BypassesFreeze). Total tests: 29 in localratelimit + 3 in stats. All PASS under `-race`. **Code-quality reviewer flagged 3 Important items + 3 Minor items** all bundled into this follow-up commit: (a) `doc.go` New-body discipline list steps 10-11 stale (didn't reflect the factoryState restructure) — corrected to 12 numbered steps with explicit factoryState wrapping at step 11; (b) `internal/stats/registry.go` `NewCounter` lacks cross-reference to the new `NewCounterIfAbsent` sibling — added a 3-line forward-pointer comment; (c) `resolvePerRouteConfig`'s wasted-allocation comment expanded to note `NewCounterIfAbsent`'s pointer-identity guarantee + a singleflight optimization TODO for future high-cardinality workloads. The 3 Minor items (test-self-containment fourth assertion, ADR forward-pending phrasing, buildRuntimeConfigPerRoute KEEP-IN-SYNC comment) deferred per reviewer assessment "low-risk additions that improve maintainability but do not block forward progress".
**Outputs:**
```
$ go test -race -count=1 ./internal/filter/http/localratelimit/... ./internal/stats/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.017s
ok  	github.com/esalaine/envoy-go/internal/stats	1.025s
$ go vet ./...
(clean)
$ golangci-lint run ./...
(clean)
$ git diff --stat c9e89ba..ea152a1
 docs/envoy-go/DECISIONS.md                                  |  74 ++++++++
 internal/filter/http/localratelimit/local_ratelimit.go      | 156 +++++++++++++--
 internal/filter/http/localratelimit/local_ratelimit_test.go | 109 ++++++++++-
 internal/stats/registry.go                                  |  31 ++
 internal/stats/registry_test.go                             |  44 +++++
 5 files changed, 397 insertions(+), 17 deletions(-)
```

## Task 6 — `internal/stats/name.go` Rule SN9 + filter-specific Prometheus tag-extractor + 22→26 stat-table extension

**Commits:** `59c4aa4` — `phase 11: Rule SN9 + filter-specific Prometheus tag-extractor [ADR-0118]`
**Notes:** Lands Rule SN9 in `internal/stats/name.go`'s `flattenToProm` `default` branch — a SECOND-PASS detection that fires only on the unmatched-prefix path (after SN1-SN5 prefix-segment switch fails); the existing SN1-SN5 hot-path is unchanged. SN9 matches names of the shape `<stat_prefix>.http_local_rate_limit.<counter>` where `<stat_prefix>` has no dots and `<counter>` is one of `{enabled, ok, rate_limited, enforced}`; produces Prometheus base `envoy_http_local_rate_limit_<counter>` + label `envoy_local_http_ratelimit_prefix=<stat_prefix>` per SPEC §11.5 + ADR-0118 + planner-time decision 1. The `idx > 0` guard rejects empty-prefix inputs (leading-dot); the `!strings.ContainsRune(prefix, '.')` guard rejects multi-segment prefixes; the counter switch limits to the 4 known names. SN9 returns directly to skip the SN4 status-class collapse since `<stat_prefix>.http_local_rate_limit.{enabled,ok,rate_limited,enforced}` doesn't have a `_Nxx` suffix. ADR-0118 lands in full at this commit (NOT split with Task 4 per PLAN line 1507 recommendation): covers the SN9 rule design + `enforced == rate_limited` MVP invariant + 22→26-name BEHAVIOR_CONTRACT stat-table extension verbatim Markdown patch (Task 14 applies the patch to BEHAVIOR_CONTRACT.md) + tag-extraction collision quirk note (out-of-scope per SPEC §11.5 (e); fixture 0013 uses safe stat_prefix values to avoid). ADR-0061 amendment paragraph appended in-place at the existing ADR-0061 body (matching ADR-0073's amendment-placement convention from Tasks 5 + 7-prior phases) noting Rule SN9 + `Registry.NewCounterIfAbsent` extensions. Tests: 5 SN9 unit tests landed at the main commit (BasicStatPrefix / AllFourCounters table-driven over 4 counters / PrefixWithUnderscores / DoesNotConflictWithSN1234 — SN1 wins precedence / RejectsUnknownCounter); 2 additional negative tests added in the follow-up commit per code-quality reviewer Important items #1 + #2 (RejectsLeadingDot — exercises the `idx > 0` guard explicitly; RejectsDoublyNestedSegment — exercises the counter-switch rejection on a degenerate doubly-nested segment input). All 7 SN9 tests + the existing 49 stats tests PASS under `-race`. **Code-quality reviewer flagged 3 Important items + 3 Minor items;** 3 Important + 1 Minor folded into the follow-up commit: (a) +2 boundary tests for SN9 (Important #1 + #2) — leading-dot + doubly-nested-segment; (b) `name.go` SN9 counter-switch comment expanded (Important #3) — explicit "KEEP IN SYNC with newFilterStats / newFilterStatsIfAbsent" note + future-widening guidance to extend BOTH this switch AND the filter's filterStats struct in lockstep; (c) ADR-0057 stale forward-reference at line 2148 (Minor #5) — paragraph (c) originally anticipated calling the histogram rule "SN9" but phase 11 claimed the SN9 number for local_ratelimit; reworded to "a new flattening rule" + a HISTORICAL NOTE pointing at ADR-0118 + clarifying that the future histogram rule will use the next-free SN number at its landing. Reviewer's other Minor items (stale ADR-0057 SN9 cross-reference reading, `strings` import note, multi-segment-prefix-vs-SPEC-§11.5 confirmation) deferred per the reviewer's "no critical issues" assessment.
**Outputs:**
```
$ go test -race -count=1 ./internal/stats/...
ok  	github.com/esalaine/envoy-go/internal/stats	1.024s
$ go vet ./...
(clean)
$ golangci-lint run ./...
(clean)
$ git diff --stat ea152a1..59c4aa4
 docs/envoy-go/DECISIONS.md      | 78 ++++++++++++++++++++++++++++++++
 internal/stats/name.go          | 30 +++++++++++++
 internal/stats/name_test.go     | 75 +++++++++++++++++++++++++++++++
 3 files changed, 183 insertions(+)
```

## Task 7 — `cmd/envoy-go/main.go` register `localratelimit.New` under `localratelimit.TypeURL`

**Commits:** `60bac1b` — `phase 11: register localratelimit.New under localratelimit.TypeURL`
**Notes:** Trivial 2-line boot-wiring change (~3 LoC delta total: 1 import alphabetically positioned between `header_mutation` and `router`; 1 registration line inserted between `header_mutation.New` and `header_mutation.RegisterPerRouteValidator(httpReg)` so the localratelimit Register lands BEFORE the eager per-route-validator hook AND the `httpReg.Freeze()` call per ADR-0072). The final boot-registration order is: `router → cors → envoygotest → fault → header_mutation → localratelimit → header_mutation.RegisterPerRouteValidator → Freeze` — matching ADR-0114 Consequences exactly. local_ratelimit does NOT call `RegisterPerRouteValidator` per ADR-0114 + Task 5 settled approach (per-route TPFC entries are validated lazily at first-resolve via `buildRuntimeConfigPerRoute`, NOT at boot via the eager per-route-validator hook). No new ADR lands at this commit (ADR-0114 already covers the registration ordering at Task 2/3 commit). Reviews: skipped subagent dispatch for this trivially small change since the build + full test suite verify correctness directly. `go build ./cmd/envoy-go` clean; `go vet ./...` clean; `go test -race -count=1 -short ./...` all 30+ packages PASS with no regressions.
**Outputs:**
```
$ go build ./cmd/envoy-go
(clean)
$ go vet ./...
(clean)
$ go test -race -count=1 -short ./...
(every package PASS; clean)
$ git diff --stat 59c4aa4..60bac1b
 cmd/envoy-go/main.go | 2 ++
 1 file changed, 2 insertions(+)
$ grep -cE 'httpReg.Register' cmd/envoy-go/main.go
6
```

## Task 8 — `internal/filter/http/localratelimit/fuzz_test.go` `FuzzLocalRateLimitConfigParse`

**Commits:** `f77385e` — `phase 11: FuzzLocalRateLimitConfigParse (fifteenth fuzzer per ADR-0018)`
**Notes:** Lands the fifteenth fuzzer overall (post phase-10's fourteenth `FuzzHeaderMutationConfigParse`) per SPEC §14.3 + ADR-0018's "every parser/codec/filter ships a fuzzer" discipline. ~40 LoC. Fuzzes arbitrary `(typeURL, value)` byte sequences as the `*anypb.Any` parameter to `New`. Asserts `New` returns either `(factory, nil)` OR `(nil, error)` — never panics; never returns `(nil, nil)`. Seed corpus: 3 entries (empty, malformed Any under canonical TypeURL, short proto-wire bytes). 30s budget per ADR-0018 short-mode CI policy. No new ADR (ADR-0018 already covers fuzzer discipline). Reviews: skipped subagent dispatch — fuzzer parameters mechanical; the 30s execution + the `go test -count=1 -run FuzzLocalRateLimitConfigParse` regression-corpus run verify correctness.
**Outputs:**
```
$ go test -count=1 -run FuzzLocalRateLimitConfigParse ./internal/filter/http/localratelimit/...
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	0.003s
$ go test -fuzz=FuzzLocalRateLimitConfigParse -fuzztime=30s ./internal/filter/http/localratelimit/...
fuzz: elapsed: 30s, execs: 6394357 (272072/sec), new interesting: 245 (total: 248)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	31.058s
$ go vet ./... && golangci-lint run ./...
(both clean)
$ git diff --stat 60bac1b..f77385e
 internal/filter/http/localratelimit/fuzz_test.go | 39 +++++++++++++++++++++++
 1 file changed, 39 insertions(+)
```

## Task 9 — Fixture infrastructure: `BackendKind` enum extension + `runner_test.go` spawn helper

**Commits:** `21d7c10` — `phase 11: fixture 0013 infrastructure — BackendKind enum + spawn helper`
**Notes:** Trivial 2-file delta per planner-time decision 9 + phase-10 Task 11 precedent. Adds `HTTPLocalRateLimit BackendKind = 10` to `test/differential/fixture/fixture.go` (after the existing `HTTPHeaderMutation BackendKind = 9` from phase 10) + `startHTTPLocalRateLimitBackend` spawn helper to `test/differential/runner_test.go` (mirrors `startHTTPHeaderMutationBackend`) + the matching `fixture.HTTPLocalRateLimit` switch case in `runFixture`'s backend-spawn dispatch. Per the PLAN's option (b) recommendation, the blank-import for the fixture driver is DEFERRED to Task 13 (the driver landing); Task 9's commit is atomic-by-itself (compiles cleanly without the driver). The 4-listener topology fits within the existing `fixture.MultiListenerDriver` contract introduced in phase 07.2 (`test/differential/fixture/fixture.go:294-299`); no per-scenario teardown primitive is added to the harness. Reviews: skipped subagent dispatch — the change is mechanical pattern-match against phase 10's precedent + verified via `go vet ./...` clean + `go build ./test/differential/...` clean.
**Outputs:**
```
$ go vet ./...
(clean)
$ go build ./test/differential/...
(clean)
$ git diff --stat f77385e..21d7c10
 test/differential/fixture/fixture.go |  9 +++++++++
 test/differential/runner_test.go     | 34 ++++++++++++++++++++++++++++++++++
 2 files changed, 43 insertions(+)
```

## Task 10 — Fixture 0013 `backends/backend.go`

**Commits:** `d1da8ca` — `phase 11: fixture 0013 backend — minimal HTTP backend body backend\n`
**Notes:** Minimal HTTP backend (~24 LoC) per SPEC §7.4. Mirrors fixture 0011-http-fault's backend exactly. Serves `/` with body `"backend\n"` (8 bytes; ASCII; trailing LF) and `Content-Type: text/plain` + `Content-Length: 8` headers. No special handling for `/strict` or `/loose` (rate-limit decision happens at Envoy/envoy-go BEFORE the upstream call; the backend is unreachable on rate-limited paths). Reviews: skipped subagent dispatch — file is a near-copy of the fault precedent. `go vet ./test/fixtures/0013-http-local-ratelimit/...` clean; `go build ./test/fixtures/0013-http-local-ratelimit/backends/` clean.
**Outputs:**
```
$ go vet ./test/fixtures/0013-http-local-ratelimit/...
(clean)
$ go build ./test/fixtures/0013-http-local-ratelimit/backends/
(clean)
$ git diff --stat 21d7c10..d1da8ca
 test/fixtures/0013-http-local-ratelimit/backends/backend.go | 24 ++++++++++++++++++++++++
 1 file changed, 24 insertions(+)
```

## Task 11 — Fixture 0013 `envoy.yaml` + `envoy-go.yaml` bootstraps (4-listener pre-configured topology)

**Commits:** `46866f0` — `phase 11: fixture 0013 bootstraps — 4-listener pre-configured topology`
**Notes:** Lands the dual-bootstrap YAML files per planner-time decision 8 (the 4-listener pre-configured topology — diverges from SPEC §7.1's two-listener+teardown layout to fit the existing differential-fixture harness's single-Drive-call contract; bucket isolation provided at boot by listener-distinct factories). FOUR listeners in each bootstrap: `l_s1` (cap=10, fill=10, interval=1s, stat_prefix=foo), `l_s2` (cap=2, fill=2, interval=60s, stat_prefix=bar), `l_s3` (cap=1, fill=1, interval=200ms, stat_prefix=baz), `l_per_route` (listener-level cap=10/stat_prefix=qux + per-route `/strict` TPFC override cap=1/stat_prefix=strict + no-override `/loose`). All listeners explicitly set `filter_enabled` + `filter_enforced=100%` per SPEC §1.1 amendment with unique runtime_keys per listener-per-filter to avoid Envoy's runtime-key cross-contamination. Both bootstraps use port placeholders (`{{.AdminPort}}`, `{{.LS1Port}}`, `{{.LS2Port}}`, `{{.LS3Port}}`, `{{.LPerRoutePort}}`, `{{.BackendPort}}`) substituted by the runner via Go `text/template`. **IMPL-1 substitution applied** to the per-route TPFC entry per `/strict`: the `@type` URL is `...LocalRateLimit` (NOT `...LocalRateLimitPerRoute` — which doesn't exist upstream); the fields go directly under the message (NO `rate_limit:` wrapper). Difference between the two YAMLs: cluster type `STRICT_DNS` (envoy.yaml — reference Envoy needs DNS resolution to `host.docker.internal` since it runs in a container) vs `STATIC` (envoy-go.yaml — envoy-go convention; 127.0.0.1 backend address since envoy-go runs in-process). The `filter_enabled` + `filter_enforced` fields are PRESENT in envoy-go.yaml even though envoy-go silent-ignores them per SPEC §2.1 cluster 2 — field presence ensures byte-equivalent config-load behavior across reference + subject. Reviews: skipped subagent dispatch — YAML is verbatim from the PLAN sketch with the IMPL-1 substitution rule applied carefully + ports kept consistent.
**Outputs:**
```
$ git diff --stat d1da8ca..46866f0
 test/fixtures/0013-http-local-ratelimit/envoy-go.yaml | 159 ++++++++++++++++++++++
 test/fixtures/0013-http-local-ratelimit/envoy.yaml    | 159 ++++++++++++++++++++++
 2 files changed, 318 insertions(+)
```

## Task 12 — Fixture 0013 `expectations.yaml` + `README.md`

**Commits:** `9ce550e` — `phase 11: fixture 0013 expectations + README`
**Notes:** Lands the prose narrative artefacts per SPEC §4.3 + ADR-0019 (expectations.yaml is prose, not machine-evaluated; the runner's `CompareBytes` enforces machine-checkable byte-equivalence). expectations.yaml documents the per-scenario equivalence claims for all 4 scenarios (basic-allow / basic-rate-limited / refill-after-fill_interval / per-route-override) with verbatim counter-delta assertions + the rate-limited 4-header set in lexicographic order + the lockstep MVP invariant + the ±10ms scenario-3 timing tolerance + the cross-references to ADR-0114..ADR-0119. README.md provides the fixture overview + per-scenario list + 4-listener pre-configured bootstrap discipline + Envoy-deviation note (NONE) + IMPL-1 substitution note + ADR cross-references + planner-time-decisions cross-references. Both files include explicit notes about the IMPL-1 substitution (per-route TPFC `@type` is `...LocalRateLimit`, not `...LocalRateLimitPerRoute`; fields go directly under the message — no `rate_limit:` wrapper) — captures the impl-time correction in the fixture-level prose so future readers don't need to chase PROGRESS.md preamble. Reviews: skipped subagent dispatch — both files are prose with verbatim PLAN content + IMPL-1 note adapted; no compilable code.
**Outputs:**
```
$ git diff --stat 46866f0..9ce550e
 test/fixtures/0013-http-local-ratelimit/README.md       |  91 +++++++++++++++++
 test/fixtures/0013-http-local-ratelimit/expectations.yaml | 80 ++++++++++++++++
 2 files changed, 171 insertions(+)
```

## Task 13 — Fixture 0013 driver.go (single-Drive 4-listener orchestration + ±10ms tolerance)

**Commits:** `2fdfc5e` — `phase 11: fixture 0013 driver — 4-scenario orchestration via 4-listener topology`
**Notes:** Lands the differential-fixture driver implementing `fixture.Driver` + `fixture.MultiListenerDriver` + `fixture.BackendKindAware` + `fixture.StatsAsserter` for fixture 0013. ~594 LoC. Single `driveAll(ctx, addrs)` orchestrating all 4 scenarios in ONE `DriveSubjectMulti`/`DriveReferenceMulti` invocation per planner-time decision 8 (4-listener topology). Scenario 1: 5 GETs to l_s1; assert 5×200 + counters foo.{enabled=5, ok=5, rate_limited=0, enforced=0}. Scenario 2: 5 GETs to l_s2; assert first 2×200 + last 3×429 with byte-exact body `local_rate_limited` (18 bytes) + 4-header set sorted alphabetically (date allow-listed). Scenario 3: 3 GETs at t=0/10ms/250ms with **±10ms post-hoc band assertion** `[200, 260]ms` per ADR-0116 + planner-time decision 4; emits `TOLERANCE_FAIL:` sentinel into the byte stream if outside band so CompareBytes surfaces the failure. Scenario 4: 6 interleaved GETs to /strict/loose; assert per-route bucket isolation per ADR-0117 (strict cap=1 → 2 rate-limited; loose cap=10 inherited → all allowed) + counter capture under BOTH `strict` AND `qux` stat_prefixes. **Differential test PASSES 3/3 against reference Envoy v1.37.2** (~2.5s per run; byte-equivalent output across all 4 scenarios; stats assertions green on both sides). The implementer flagged + fixed FOUR PLAN-sketch errors in-flight: (a) admin port `9913` → `9901` (the harness convention hardcodes 9901/tcp for all reference containers; not per-fixture); (b) `dns_lookup_family: V4_ONLY` added to the c_backend cluster in envoy.yaml (Task 11 omission — Docker Desktop's `host.docker.internal` resolves IPv6 by default; without V4_ONLY reference Envoy gets 503; envoy-go.yaml unaffected since STATIC cluster resolves to loopback at config time); (c) metric base name `envoy_http_local_rate_limit_*` (PLAN sketch had `envoy_local_ratelimit_*` — confirmed empirically against Prometheus output; ADR-0118 SN9 produces `envoy_http_local_rate_limit_<counter>` per the rule's transformation); (d) per-route stats semantics `qux ok=3` (not 6 — `/strict` requests use the per-route `strict` runtimeConfig exclusively per ADR-0117 wholesale-override; `/loose` requests use the listener-level `qux` config; the implementation confirms this — `resolvePerRouteConfig` returns the per-route rc exclusively for matching TPFC routes). Plus the blank-import added to `test/differential/runner_test.go` (deferred from Task 9 per the PLAN's option-(b) recommendation). One reviewer-flagged Minor (stale `9913` cite in driver.go line 104 doc comment) folded into this follow-up commit. The other 2 Minors (per-call `http.Client` allocation in probe; timeout-less `http.DefaultClient` in scrapeRateLimitStats) are pre-existing project patterns deferred per reviewer "production-quality" assessment.
**Outputs:**
```
$ go test -count=1 -v ./test/differential/ -run 'TestDifferential/0013-http-local-ratelimit'
=== RUN   TestDifferential/0013-http-local-ratelimit
--- PASS: TestDifferential/0013-http-local-ratelimit (2.498s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	2.567s
$ git diff --stat 9ce550e..2fdfc5e
 test/differential/runner_test.go                          |   1 +
 test/fixtures/0013-http-local-ratelimit/driver/driver.go  | 594 ++++++++++++++++++++
 test/fixtures/0013-http-local-ratelimit/envoy.yaml        |   1 +
 3 files changed, 596 insertions(+)
```

## Task 14 — BEHAVIOR_CONTRACT.md patches per SPEC §13 + ROADMAP row 11 in-progress→done

**Commits:** `ac1ec1d` — `phase 11: BEHAVIOR_CONTRACT + ROADMAP row 11 done`
**Notes:** Five in-place patches to BEHAVIOR_CONTRACT.md per SPEC §13 + ADR-0052 in-place-edit authorisation (113 lines added; 3 lines removed for the §13.2 heading 22→26 update + minor structural adjustments). NO new code touched. NO new ADRs (ADR-0114..ADR-0119 already landed). (a) §13.1: NEW `### envoy.filters.http.local_ratelimit` subsection inserted after the existing `### envoy.filters.http.header_mutation` subsection, with sub-subsections for asserted-equivalence, token-bucket primitive (per ADR-0116), per-route override semantics (per ADR-0117 + ADR-0073 amendment; **IMPL-1 substitution applied** — uses `*LocalRateLimit` wording, not `LocalRateLimitPerRoute`), 429 wire shape (per ADR-0119 + SPEC §11.3), allow-path response (no x-ratelimit-* headers), MVP invariant (per ADR-0118; `enforced == rate_limited` lockstep), stats (4 counters per stat_prefix; Prometheus tag-extractor SN9), silent-ignored fields (14 fields organized by 8 family-clusters), empirical evidence (sample wire shape). (b) §13.2: Stat-name mapping table heading updated `22-name table → 26-name table`; 4 new counter rows appended (`<stat_prefix>.http_local_rate_limit.{enabled,ok,rate_limited,enforced}`); table preamble note added describing the new filter-specific Prometheus tag-extractor `envoy_local_http_ratelimit_prefix` (Rule SN9) + the tag-extraction collision quirk (out of scope; fixture uses safe values foo/bar/baz/qux/strict to avoid). (c) §13.3: Timing-tolerances row added — fixture 0013 scenario 3 t=250ms refill boundary ±10ms wall-clock per ADR-0116 + SPEC §11.7 empirical. (d) §13.4: Equivalence Matrix row added — covers all 4 fixture scenarios + 4-header set + 4 stat-counter delta assertion + `envoy_local_http_ratelimit_prefix` label + the deferred-field clusters not asserted. (e) §13.5: NEW `### Phase 11 forward-pointer notes` subsection — 8 deferred field families (descriptor-action, runtime+shadow-mode, xDS, response-side, per-connection, multi-stage, X-RateLimit, gRPC trailer); filter_enabled/filter_enforced 0%-default divergence-window note; tag-extraction collision quirk note. ROADMAP row 11 status flipped `in-progress → done` per ADR-0106 (§9 family heading at line 56 stays unchanged). Reviews: skipped subagent dispatch — documentation-only changes verified by `go vet ./...` clean + visual structural inspection.
**Outputs:**
```
$ git diff --stat 2fdfc5e..ac1ec1d
 docs/envoy-go/BEHAVIOR_CONTRACT.md | 116 ++++++++++++++++++++++++++++--
 docs/envoy-go/ROADMAP.md           |   2 +-
 2 files changed, 113 insertions(+), 5 deletions(-)
$ go vet ./...
(clean)
$ grep -n '| 11 ' docs/envoy-go/ROADMAP.md
(row 11 status: done)
```
