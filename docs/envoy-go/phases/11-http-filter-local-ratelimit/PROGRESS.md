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
