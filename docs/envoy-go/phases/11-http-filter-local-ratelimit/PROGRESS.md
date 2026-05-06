# Phase 11 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..10 PROGRESS.md structure.

## Preamble — execution preconditions

Precondition 11 (`LocalRateLimitPerRoute` proto type present in module closure) **did not pass**: `go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/local_ratelimit/v3 LocalRateLimitPerRoute | head -5` returns `doc: no symbol LocalRateLimitPerRoute in package ...@v1.32.4/...`. The PLAN's remediation guidance (try `go mod download` / `go mod tidy` / version bump) was attempted: bumping to v1.37.0 (the highest available version as of 2026-05-05) still produces the same `doc: no symbol` error — `LocalRateLimitPerRoute` does not exist in the go-control-plane Go bindings at any published version. The SPEC (line 1222) references this as a "Reference Envoy v1.37.2 proto" message (1-field: `rate_limit *LocalRateLimit`) but the corresponding Go binding was never generated into go-control-plane. The go.mod was reverted to v1.32.4 (no bump committed) and the working tree is pristine. The impact on subsequent tasks is noted here for the controller: Task 5 (per-route TPFC parsing + ADR-0117) will need to use `LocalRateLimit` directly as the per-route container type (via TPFC `@type: ...LocalRateLimitPerRoute` in the YAML unmarshalling as an `anypb.Any` containing a `LocalRateLimit` body, or define a local shim), since the `LocalRateLimitPerRoute` Go struct is absent. This deviation is recorded per ADR-0044; the remaining 15 preconditions were satisfied at cold-start without deviation.

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

**Commits:** TBD — this task's commit
**Notes:** Created PROGRESS.md; verified all 16 preconditions per PLAN §"Execution preconditions"; phase-11 SPEC + PLAN confirmed present in HEAD; SPEC at 63c88ed; ADR tail at 0113 (next-free 0114); internal/filter/http/localratelimit/ absent (Task 2 lands); SN9 absent (Task 6 lands); fixture.HTTPLocalRateLimit absent (Task 9 lands). Precondition 11 DEVIATION: `LocalRateLimitPerRoute` absent from go-control-plane v1.32.4 (and v1.37.0 — the highest available); controller must resolve before Task 5 proceeds (see preamble deviation paragraph). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention; ADRs land at first-use commit per PLAN's ADR table).
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
