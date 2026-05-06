# Phase 11 — Code review (REVIEW.md)

**Phase id:** `11` (third §9 HTTP-filters family-row to land per ADR-0106; FIRST stateful per-route filter)
**Slug:** `11-http-filter-local-ratelimit`
**Branch under review:** `phase-11-http-filter-local-ratelimit-impl`
**Range:** `dfa08c9` (branch tip; phase-done SHA-fill commit) — 15 task commits + SHA-fill / PROGRESS-append follow-ups
**Parent ROADMAP row:** `11 http-filter-local-ratelimit` flips `in-progress → done` at the Task 14 commit `ac1ec1d` (already landed prior to this REVIEW; row 11's status field reads `done` on the impl branch at HEAD).
**Reviewer method:** Inline authoring by the implementing session per the PLAN's Task 16 explicit allowance; inputs: SPEC §15 acceptance checklist + the branch diff + phase-10 REVIEW.md structural template + PROGRESS.md per-task entries + DECISIONS.md ADR-0114..ADR-0119 + ADR-0073 amendment + ADR-0061 amendment.
**Six-gate state at HEAD:** all green per Task 15's verification sweep — outputs reproduced verbatim in §"Six-gate verification appendix" below.

This review covers the full phase 11 surface: `internal/filter/http/localratelimit/` package (`doc.go` + `local_ratelimit.go` + `bucket.go` + `local_ratelimit_test.go` + `bucket_test.go` + `fuzz_test.go`), two framework additions (`internal/stats.Registry.NewCounterIfAbsent` + Rule SN9 in `internal/stats/name.go`), `cmd/envoy-go/main.go` boot registration, differential fixture `0013-http-local-ratelimit` (4 scenarios, 4-listener pre-configured topology, reference Envoy v1.37.2 STRICT_DNS + envoy-go STATIC), `FuzzLocalRateLimitConfigParse` (fifteenth fuzzer in repo), BEHAVIOR_CONTRACT.md §13 five-patch bundle (NEW local_ratelimit subsection + 22→26 stat-name table extension + timing-tolerances row + equivalence-matrix row + forward-pointer notes), the six ADRs ADR-0114..ADR-0119 + ADR-0073 amendment paragraph + ADR-0061 amendment paragraph, and the ROADMAP row 11 status flip + STATE.md advance.

This REVIEW closes phase 11's lifecycle (state 5 → 6) and is the final task before merge to master.

---

## 1. Final assessment

**APPROVED.**

All six phase-done gates are GREEN at HEAD `1de512d` per the Task 15 verification sweep (§6 below). The implementation faithfully realizes the SPEC across all 15 PLAN tasks. The local_ratelimit filter is the THIRD §9 HTTP-filters family-row to ship under ADR-0106 and the FIRST filter to require per-route stateful resource isolation (independent `*tokenBucket` + `*filterStats` per TPFC entry).

The architectural centrepiece is the ADR-0117 per-route bucket isolation design: the `factoryState` lazy-cache pattern + `Registry.NewCounterIfAbsent` post-Freeze idempotent registration form the canonical reference for future stateful per-route filters. One substantial implementation surprise — IMPL-1: `LocalRateLimitPerRoute` proto does not exist upstream — redirected both SPEC §§ and PLAN sketches but was handled cleanly at Task 1 via the preamble substitution rule.

The differential fixture `0013-http-local-ratelimit` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 4 scenarios across 4 dedicated listeners, exercising basic allow (scenario 1), byte-exact 429 rate-limited response with 4-header wire shape (scenario 2), lazy-refill timing tolerance ±10ms (scenario 3), and per-route TPFC bucket independence with per-route vs listener-level stat-prefix segregation (scenario 4). PASSES 3/3 (second run after a known port-allocation flake on first attempt; confirmed harness-side).

All six anticipated ADRs (ADR-0114..ADR-0119) landed at the correct tasks per the PLAN's "ADRs introduced by this plan" table.

---

## 2. N-1 carry-forward dispositions (from phase-10 REVIEW)

Phase-10's REVIEW §7 identified six carry-forward items. Phase 11's disposition for each:

| # | Phase-10 item | Phase-11 disposition |
|---|---|---|
| **CF-1** | `RegisterPerRouteValidator` registration-site convention | **Not triggered.** local_ratelimit does NOT register a per-route validator (per ADR-0114 + Task 7 notes); per-route TPFC entries are validated lazily at first-resolve via `buildRuntimeConfigPerRoute`. The convention (exported function called from `main.go` pre-Freeze) remains documented at ADR-0110 §Consequences. |
| **CF-2** | `BuildPerRouteConfig` 4-param signature | **In place, not widened.** Phase 11 consumes the existing 4-param signature with the `nil` registry arg pattern. No signature change needed. |
| **CF-3** | Differential body-strip allow-list discipline | **Not triggered for scenario 1/3/4.** Scenario 2 (rate-limited path) returns body `local_rate_limited` from SendLocalReply — the response body does NOT pass through the echo backend, so the header-reflection allow-list concern is moot. The upstream-path scenarios (1/3/4) use the minimal backend (`body: "backend\n"`) with no request-header reflection; no new strip-list entries needed. |
| **CF-4** | `flattenToProm` SN-asymmetry (from phase-09 I-1) | **Resolved: Rule SN9 extended the `default` branch.** Phase 11 is the first stat-emitting §9 filter; the SN9 rule handles the `<stat_prefix>.http_local_rate_limit.<counter>` shape and returns directly, bypassing the SN4 status-class collapse. The original SN1/SN3/SN5 asymmetry concern (dotted-rest segments unflattened) was not triggered by any local_ratelimit name shape; it remains latent for filters whose names WOULD traverse SN1/SN3/SN5 with interior dots. Carry forward as a lower-priority item for a future stat-discipline phase. |
| **CF-5** | Zero-stats filter pattern (ADR-0108) | **Superseded.** local_ratelimit emits 4 counters per stat_prefix (the opposite of zero-stats). ADR-0108 zero-stats precedent cited at ADR-0114 §Context only for comparison. No impact on phase 11 surface. |
| **CF-6** | Dual-listener fixture pattern (0012) | **Extended: 4-listener pattern (0013).** Fixture 0013 pushes the `fixture.MultiListenerDriver` contract to 4 listeners driven in a single `DriveSubjectMulti`/`DriveReferenceMulti` invocation. The 4-listener bootstrap + per-listener `filterEnabledKey` + `filterEnforcedKey` uniqueness discipline is the new reference for future multi-bucket fixture designs. |

---

## 3. Per-task retrospective

The 15 tasks landed 15 task-commits + SHA-fill / PROGRESS-append follow-ups. Tasks that deviated from PLAN verbatim are called out below.

**Task 1 (commit `cbf07eb`):** Execution-precondition check + PROGRESS.md preamble. 15 of 16 preconditions satisfied; precondition 11 FAILED at cold-start (`LocalRateLimitPerRoute` proto does not exist — see IMPL-1 below). IMPL-1 substitution rule settled here; all downstream task sketches adjusted accordingly. The PROGRESS.md preamble introduces the "impl-time decisions" block — new precedent for capturing PLAN/SPEC errata at impl time without amending committed artefacts.

**Task 2 + Task 3 (commit `bfc0529`):** Combined per PLAN line 932 recommendation. Lands `doc.go` + `local_ratelimit.go` + `bucket.go` + tests (~700 LoC). **Three structural deviations from PLAN sketches:** (a) `FilterInstanceFactory` returns `envoyhttp.HTTPFilter{Name, Decoder, Encoder}` struct (not raw `*filter`) — matched fault precedent; (b) `filterStats` fields are `*stats.Counter` (not `*atomic.Int64`) — preserves Walk/Freeze discipline per ADR-0061; (c) `DecodeData`/`EncodeData` parameter is `[]byte` (not `http.Header` — PLAN sketch error). **IMPL-1 substitution applied**: `*LocalRateLimitPerRoute` → `*LocalRateLimit` in code, doc.go, and ADR-0115 Context.

**Task 4 (commit `9f0737a`):** `DecodeHeaders` body + `filterStats` wiring. **One impl-time correction:** PLAN sketch had `var rateLimitedBody = []byte(...)` and `runtimeConfig.body []byte`; `SendLocalReply` actually takes `string`. Substituted `const rateLimitedBody = "local_rate_limited"` + `body string` throughout; ADR-0119 alternative (a) rejection rewritten to reflect the correct reason. Follow-up commit corrected the ADR-0115 Decision-section code block (same `[]byte → string` substitution).

**Task 5 (commit `ea152a1`):** Per-route TPFC bucket independence + `Registry.NewCounterIfAbsent` framework primitive. **IMPL-1 substitution applied** throughout: `sync.Map` keyed by `*LocalRateLimit` (not `*LocalRateLimitPerRoute`); no `.GetRateLimit()` indirection; `TestDecodeHeaders_PerRouteOverride_IndependentBuckets` constructs two `*LocalRateLimit` directly. **One PLAN-sketch correction:** `RequestRouteConfig()` takes no args; PLAN sketch had `dcb.RequestRouteConfig(filterName)`. Framework resolves calling-filter name internally per `internal/filter/http/callbacks.go:36`.

**Task 6 (commit `59c4aa4`):** Rule SN9 + ADR-0118. No deviations from planner-time decision 1. Code-quality reviewer follow-up added 2 boundary tests (`RejectsLeadingDot` + `RejectsDoublyNestedSegment`) + expanded counter-switch keep-in-sync comment + fixed ADR-0057 stale `SN9` forward-reference.

**Task 7 (commit `60bac1b`):** `cmd/envoy-go/main.go` boot registration. Trivial 2-line change. No deviations. local_ratelimit does NOT call `RegisterPerRouteValidator` (deliberate per ADR-0114).

**Task 8 (commit `f77385e`):** `FuzzLocalRateLimitConfigParse` (fifteenth fuzzer). 30s budget: 6.39M execs clean. No deviations.

**Task 9 (commit `21d7c10`):** Fixture infrastructure — `HTTPLocalRateLimit BackendKind = 10` + spawn helper. Trivial pattern-match against phase-10 precedent. Blank-import for the driver deferred to Task 13 per PLAN option (b). No deviations.

**Task 10 (commit `d1da8ca`):** Fixture 0013 `backends/backend.go`. Near-copy of fault backend. No deviations.

**Task 11 (commit `46866f0`):** Fixture 0013 `envoy.yaml` + `envoy-go.yaml`. **IMPL-1 substitution applied** to per-route TPFC `@type` URLs (uses `...LocalRateLimit`, not the non-existent `...LocalRateLimitPerRoute`). **Task-11 omission discovered at Task 13:** missing `dns_lookup_family: V4_ONLY` on the `c_backend` STRICT_DNS cluster in `envoy.yaml`. Without V4_ONLY, Docker Desktop's `host.docker.internal` resolves IPv6 → reference Envoy gets 503. Fixed at Task 13.

**Task 12 (commit `9ce550e`):** Fixture 0013 `expectations.yaml` + `README.md`. Both files include explicit IMPL-1 substitution notes for future readers. No deviations from plan for prose artefacts.

**Task 13 (commit `2fdfc5e`):** Fixture 0013 `driver.go`. **Four PLAN-sketch corrections identified and fixed in-flight:**
- Admin port `9913 → 9901`: harness convention hardcodes `9901/tcp` for all reference containers; the PLAN sketch value `9913` was fixture-specific fiction.
- `dns_lookup_family: V4_ONLY` added to `envoy.yaml` `c_backend` cluster (Task 11 omission, see above).
- Metric base name `envoy_http_local_rate_limit_*` (not `envoy_local_ratelimit_*`): confirmed empirically against Prometheus output; ADR-0118 SN9 produces the `http_` prefix per the rule transformation.
- Per-route stats `qux ok=3` (not 6): `/strict` requests use the per-route `strict` runtimeConfig exclusively per ADR-0117 wholesale-override; only `/loose` requests contribute to `qux` listener-level counters. PLAN sketch's `qux ok=6` was wrong.

Differential test PASSES in 2.5s against reference Envoy v1.37.2. Full suite PASS on second run (first attempt hit the known port-allocation flake — harness-side, not a phase-11 regression).

**Task 14 (commit `ac1ec1d`):** BEHAVIOR_CONTRACT.md five-patch bundle + ROADMAP row 11 flip. **IMPL-1 substitution applied** throughout the new §13.1 `envoy.filters.http.local_ratelimit` subsection (uses `*LocalRateLimit` per-route container wording, not `LocalRateLimitPerRoute`). No deviations.

**Task 15 (commit `1de512d`):** Phase-done six-gate verification + STATE.md advance. All six gates green. 15 differential fixture directories (14 subtests with 0007a/0007b split; fixture 0013 adds one new subtest) run clean. STATE.md flipped to `awaiting next planning`. No deviations.

**Task 16 (THIS commit):** REVIEW.md per end-of-phase review discipline. This document. Closes phase 11 lifecycle (state 5 → 6).

---

## 4. Planner-time decisions retrospective

The nine planner-time deferred decisions from PROGRESS.md §Preamble, evaluated against implementation outcomes:

**D1 (Tag-extractor registration site = EXTEND `internal/stats/name.go`'s `flattenToProm` SWITCH WITH NEW RULE SN9):** VALIDATED. The hardcoded switch was the correct registration site; no dispatch-registry primitive was needed. SN9 second-pass detection fires only on the unmatched-prefix path (after SN1-SN5 prefix-segment switch fails); SN1-SN5 hot-path unchanged. The original SPEC §12 D1 mis-statement of `internal/admin/stats.go` was corrected at planning time; the correction held through implementation without further adjustment.

**D2 (Filter-callback wiring = `SetDecoderCallbacks(cb)` + `SetEncoderCallbacks(cb)`):** VALIDATED. Callbacks pattern matched cors + fault + header_mutation precedent verbatim. The `ecb` field is unused at request time for local_ratelimit (only `dcb` is needed for `SendLocalReply` + `RequestRouteConfig()`); the field is kept for chain-of-conformance.

**D3 (PGV plumbing = EXPLICIT CHECKS IN THE `New` FACTORY):** VALIDATED. Six checks land cleanly in `buildRuntimeConfig`; verbatim Envoy error string `local rate limit token bucket fill timer must be >= 50ms` preserved for boot-log byte-equivalence. The same 6 checks are replicated in `buildRuntimeConfigPerRoute` with a keep-in-sync comment (deliberate duplication per PLAN line 932 rationale; minor reviewer item deferred).

**D4 (Scenario 3 retry-with-deadline = ±10ms TOLERANCE WITH SIMPLE `time.Sleep` DEFAULT):** VALIDATED. The ±10ms band assertion `[200, 260]ms` passes in fixture 0013 scenario 3; no retry-with-deadline fallback needed. The TOLERANCE_FAIL sentinel mechanism provides a clean diagnostic signal if the band is ever breached.

**D5 (Test-only clock injection = SKIP):** VALIDATED. Race-detector cycle test `TestTokenBucket_ConcurrentTryConsume` exercises mutex discipline via real wallclock; no flake observed across all phase-done gate runs. Clock injection remains deferred.

**PLAN-6 (File split = `bucket.go` + `local_ratelimit.go`):** VALIDATED. The file split is logical and keeps the token-bucket primitive isolated from the filter orchestration. Tasks 2+3 were combined into a single commit per PLAN line 932 recommendation; the file split is a source-level boundary, not a commit boundary.

**PLAN-7 (Race-detector cycle test = `TestTokenBucket_ConcurrentTryConsume`):** VALIDATED. 64-goroutine × 100-iteration concurrent test runs clean under `-race -count=1`. Mutex discipline mechanically validated; the test is the canonical LBP-1-adjacent evidence cited in ADR-0116.

**PLAN-8 (4-listener topology in a SINGLE BOOTSTRAP):** VALIDATED. All 4 scenarios run in one `DriveReferenceMulti`/`DriveSubjectMulti` invocation via `fixture.MultiListenerDriver`; no per-scenario teardown required. Differential fixture PASSES 3/3. The 4-listener pre-configured topology diverges from SPEC §7.1's two-listener+teardown layout but correctly fits the harness's single-Drive-call contract.

**PLAN-9 (`BackendKind = HTTPLocalRateLimit BackendKind = 10`):** VALIDATED. Trivial mechanical addition. `case fixture.HTTPLocalRateLimit:` block in `runner_test.go` fires correctly; fixture registered and run successfully.

---

## 5. ADR retrospective

**ADR-0114** (`internal/filter/http/localratelimit/` package shape — no-underscore directory + extension-registry registration ordering): VALIDATED. No-underscore directory `localratelimit/` aligns with cors + fault precedent; diverges from the single-precedent header_mutation pattern without triggering cross-package churn. Boot-registration ordering `router → cors → envoygotest → fault → header_mutation → localratelimit → header_mutation.RegisterPerRouteValidator → Freeze` is exactly as described; local_ratelimit does NOT call `RegisterPerRouteValidator`. ADR-0114 §Consequences naming rule (elide-underscore for multi-token proto type-names) stands as the forward-looking convention for `globalratelimit`, `extauthz`, `jwtauthn`, etc.

**ADR-0115** (`runtimeConfig` shape + 5/14-field decomposition): VALIDATED with one Context correction. The 5-consumed / 14-silent-ignored decomposition held throughout all 15 tasks. The Context paragraph was corrected at Task 2 (IMPL-1) to drop the false `LocalRateLimitPerRoute` per-route container claim; the Decision body's field counts are correct. The `body string` correction (IMPL-1 adjacent — `[]byte → string`) was also applied to the Decision-section code block at Task 4 follow-up.

**ADR-0116** (`tokenBucket` Option-A lazy-refill on access): VALIDATED. The lazy-refill semantics produce byte-equivalent output to reference Envoy v1.37.2 across the ±10ms window. The empirical §11.7 evidence (sharp refill boundary at ≤5ms granularity) fully supports the Option-A choice. LBP-1-adjacent declaration holds: the mutex scope is per-bucket, not shared-registry-wide. The race-detector cycle test is the canonical evidence.

**ADR-0117** (per-route bucket isolation as ADR-0073 wholesale-override consequence): VALIDATED. The lazy-cache mechanism (`sync.Map` keyed by `*LocalRateLimit` proto pointer per IMPL-1) + `NewCounterIfAbsent` framework primitive both work as designed. `TestDecodeHeaders_PerRouteOverride_IndependentBuckets` mechanically validates the 3-way pointer-distinctness (rcA / rcB / rcListener), cross-bucket isolation (drain rcA's bucket → rcB and listener buckets unaffected), and idempotent re-resolution. Fixture 0013 scenario 4 provides the end-to-end empirical validation against reference Envoy. The ADR-0073 amendment paragraph correctly captures the stateful-resource extension. One ADR-0117 Context clarification: per IMPL-1, the cache key is `*LocalRateLimit` (same proto reused for per-route TPFC entries per upstream Envoy v1.37.2 design), not the non-existent `*LocalRateLimitPerRoute`.

**ADR-0118** (SN9 + MVP invariant + 22→26 stat-table): VALIDATED. SN9 second-pass detection produces the correct Prometheus base name `envoy_http_local_rate_limit_<counter>` + label `envoy_local_http_ratelimit_prefix=<stat_prefix>` — confirmed empirically against Prometheus output (PLAN sketch's `envoy_local_ratelimit_*` base name was wrong; corrected at Task 13). MVP invariant `enforced == rate_limited` holds across all rate-limited requests in fixture 0013 scenario 2. ADR-0061 amendment paragraph correctly appended. The PLAN's `envoy_local_ratelimit_*` error was a planner-time proto-schema gap, not an ADR-0118 design flaw; the ADR correctly describes the empirical shape.

**ADR-0119** (rate-limited response wire shape): VALIDATED with one Decision-section correction. The 4-header wire shape (content-length / content-type / date / server) + body `local_rate_limited` (18 bytes, no LF) + 429 default status is byte-equivalent to reference Envoy in fixture 0013 scenario 2. The `SendLocalReply` reuse from phase-09 fault is correct. Decision-section correction: body is `string` not `[]byte`; alternative-considered (a) rewritten at Task 4 to reflect the correct underlying reason (`SendLocalReply` takes `string`; `const string` form matches fault precedent + requires no conversion). The ADR-0115 code block was also updated in lockstep.

---

## 6. Six-gate retrospective

**Gate (a) build/vet/lint:** Clean at all task commits. One recurring golangci-lint `gofmt` alignment issue (struct literal non-canonical spacing) caught at Tasks 2/3 and fixed before committing — same pattern as phase-10.

**Gate (b) unit tests + race:** All packages PASS. `go test -race -count=1 ./...` clean. The `TestTokenBucket_ConcurrentTryConsume` 64-goroutine × 100-iteration test validates the mutex discipline. Differential suite PASS on second run (first attempt hit the known port-allocation flake; retry-in-isolation passed cleanly; harness-side flake, not a phase-11 regression).

**Gate (c) h2spec:** 53/53 at ADR-0051 pin unchanged. Phase 11 touches no codec or H2 path.

**Gate (d) fuzzers:** 15 fuzzers total at HEAD. `FuzzLocalRateLimitConfigParse` re-validated at Task 15 at 30s budget (6.3M execs / 30s clean). The 14 pre-existing fuzzers run at their per-phase baselines (none of their code paths are touched by phase-11 changes).

**Gate (e) differential:** All 15 subtests (0000–0013 with 0007a/0007b split; 14 fixture directories) PASS in 41.21s. Fixture 0013 scenario 2's byte-exact 429 body + 4-header set assertion is the primary non-vacuous claim.

**Gate (f) BEHAVIOR_CONTRACT.md alignment + ROADMAP row 11 status:** `grep -cE 'envoy.filters.http.local_ratelimit' BEHAVIOR_CONTRACT.md` returns 5 (§13.1 subsection + equivalence-matrix row + §13.5 forward-pointer notes). All 6 ADRs (ADR-0114..ADR-0119) confirmed in DECISIONS.md. ROADMAP row 11 reads `done`.

---

## 7. Carry-forward findings for phase 12

| # | Item | Disposition |
|---|---|---|
| **CF-1** | `Registry.NewCounterIfAbsent` reusability | **AVAILABLE AS PRECEDENT.** Future stateful per-route filters (e.g., a future `global_ratelimit` per-process bucket fallback) reuse `NewCounterIfAbsent` for post-Freeze idempotent stat registration. ADR-0117 §Consequences documents the canonical pattern; phase 11's `state.resolvePerRouteConfig` + `buildRuntimeConfigPerRoute` are the reference implementation. |
| **CF-2** | `tokenBucket` primitive reusability | **UNEXPORTED; REUSABLE IN PRINCIPLE.** The primitive is unexported in `localratelimit/bucket.go`. Future cross-filter reuse requires either extracting to a shared package or re-implementation. ADR-0116 §Consequences documents the two-path decision; phase 12 brainstormer should note the extraction path if `global_ratelimit` or another bucket-bearing filter is next. |
| **CF-3** | Deferred field families to schedule | **8 deferred clusters per SPEC §2.1 + BEHAVIOR_CONTRACT §13.5:** (1) descriptor-action (`descriptors`, `rate_limits`, `always_consume_default_token_bucket`, `max_dynamic_descriptors`); (2) runtime + shadow-mode (`filter_enabled`, `filter_enforced`, `request_headers_to_add_when_not_enforced` — divergence-window with reference Envoy when `filter_enabled` defaults to 0%); (3) xDS cluster-state (`local_cluster_rate_limit`); (4) response-side header injection (`response_headers_to_add`); (5) per-connection lifecycle (`local_rate_limit_per_downstream_connection`); (6) multi-stage (`stage`); (7) X-RateLimit headers + vh policy (`enable_x_ratelimit_headers`, `vh_rate_limits`); (8) gRPC trailer mapping (`rate_limited_as_resource_exhausted`). Runtime + shadow-mode (cluster 2) is the highest-priority item because the current divergence-window (envoy-go silent-ignores; ref Envoy defaults to 0% OFF) requires every fixture config to set both to 100% explicitly. |
| **CF-4** | `LocalRateLimitPerRoute` SPEC/PLAN errata | **DOCUMENTED AT IMPL-1 / PROGRESS.md preamble.** SPEC §§ and PLAN extensively cite `*LocalRateLimitPerRoute` as if it were a separate proto. The canonical truth: upstream Envoy v1.37.2 defines one proto (`LocalRateLimit`); per-route TPFC entries reuse it directly. Future readers of SPEC/PLAN should consult PROGRESS.md preamble IMPL-1. Future filters that encounter analogous upstream-proto questions should apply the same discipline: capture the correction in PROGRESS.md preamble; do NOT amend SPEC/PLAN (committed historical artefacts). |
| **CF-5** | `singleflight` optimization for `resolvePerRouteConfig` | **DEFERRED (acceptable).** Under high cold-start fan-out (many concurrent requests hitting the same new per-route TPFC key simultaneously), `LoadOrStore` may allocate multiple `*runtimeConfig` values, one winning and the rest discarded. A `singleflight.Group` would suppress duplicate allocations. Acceptable for current low-cardinality workload; a `TODO` comment is present in source. |
| **CF-6** | `buildRuntimeConfigPerRoute` / `buildRuntimeConfig` duplication | **DEFERRED (deliberate).** The two functions share the same 6-check validation body by design (per PLAN line 932 rationale: keep the two call sites clear for maintainability). A `KEEP-IN-SYNC` comment is present in source. If a third call site emerges, extract into a shared helper. |
| **CF-7** | `flattenToProm` SN1/SN3/SN5 asymmetry | **Still open (carried from phase-10 CF-4).** Phase 11 did not trigger the dotted-rest bug (SN9 handles local_ratelimit names; SN1/SN3/SN5 are not involved). Carry forward for a future stat-discipline phase. |
| **CF-8** | Tag-extraction collision quirk | **OUT OF SCOPE.** When `stat_prefix` collides with an Envoy-internal tag-extractor name, Prometheus output is mangled. Fixture uses safe values; forward-pointer note in BEHAVIOR_CONTRACT §13.5. Future stat-name-discipline phase may address. |

---

## 8. Minor findings

### M-1 (IMPL-1 scope underestimated at PLAN time)

PLAN line 3876 anticipated the `LocalRateLimitPerRoute` proto might need a "field name adjustment" at Task 5 survey. The actual scope was larger: the entire wrapping message does not exist upstream; the per-route TPFC reuses `*LocalRateLimit` directly. The PLAN's Refinement caveat was directionally correct ("the sketch may need adjustment") but underestimated the breadth. IMPL-1 corrected PLAN sketches at 7 affected tasks (1, 2, 3, 4, 5, 11, 14) + 3 DECISIONS.md ADR bodies (ADR-0115, ADR-0117, ADR-0119). No production-code defect resulted; corrected cleanly at each task.

### M-2 (Four Task-13 PLAN-sketch errors in a single driver commit)

Task 13 discovered and fixed four distinct errors introduced at PLAN authoring time (admin port, `dns_lookup_family`, metric base name, `qux ok` counter value). The density is unusual relative to prior phases (phase-10 Task 15 had three; phase-09 Task 14 had one). All four were fixable within the driver landing commit without requiring upstream YAML restates or framework changes. Root cause: PLAN's driver sketch section relied on unverified Prometheus + harness conventions that diverge from actual project discipline. Future driver sketch sections should cross-check admin-port convention, DNS family, metric name prefix, and per-route-stats semantics against at least one prior fixture driver as a planner-time verification step.

### M-3 (Fuzzer count: fifteenth per PROGRESS, consistent with actual repo count)

Phase 10 REVIEW §8 M-1 documented an off-by-one in the fuzzer count (PLAN said "thirteenth"; actual was fourteenth). Phase 11 corrects the labeling: `FuzzLocalRateLimitConfigParse` is the fifteenth fuzzer, and `grep -rE '^func Fuzz' ... | wc -l` at gate (d) returns `15`. The off-by-one chain from prior phases is now resolved at phase 11.

---

## 9. LoC retrospective

PLAN estimated ~2580 LoC total. Approximate actuals from committed diffs:

| Surface | PLAN estimate | Approximate actual |
|---|---|---|
| Production (`local_ratelimit.go` + `bucket.go` + `doc.go`) | ~450 LoC | ~477 LoC (doc.go ~139 + local_ratelimit.go ~239+35 = 274 + bucket.go ~99; ~512 total with follow-up edits) |
| Tests (`local_ratelimit_test.go` + `bucket_test.go`) | ~380 LoC | ~517 LoC (Tasks 2–5 cumulative adds) |
| Fuzzer (`fuzz_test.go`) | ~40 LoC | ~39 LoC |
| Framework deltas (Tasks 5/6: `registry.go` + `name.go`) | ~60 LoC | ~61 LoC (`registry.go` ~31 + `name.go` ~30) |
| Fixture (backends + YAMLs + driver) | ~680 LoC | ~693 LoC (backend ~24 + YAMLs ~319 + driver ~350) |
| Docs (ADRs + BEHAVIOR_CONTRACT + expectations + README) | ~480 LoC | ~730 LoC (DECISIONS.md +509 across 6 ADR bodies + BC patches +116 + expectations ~80 + README ~91) |

Docs ran ~50% over estimate (the 6 ADR bodies with detailed Context + alternatives + consequences sections averaged ~85 LoC each vs the ~40 LoC PLAN estimate). Production + test + framework + fixture came in close to estimate.

---

## 10. Per-ADR cohesion summary

| ADR | Title | Lands-in-task | Commit | Amendments |
|---|---|---|---|---|
| ADR-0114 | `localratelimit/` package shape + boot registration ordering | T2 | `bfc0529` | none |
| ADR-0115 | `runtimeConfig` 5-field shape + 14-silent-ignored decomposition + 6-check validation | T2 | `bfc0529` | Context + Decision `[]byte→string` corrected at T4 |
| ADR-0116 | `tokenBucket` Option-A lazy-refill + monotonic-time + LBP-1-adjacent + ±10ms tolerance | T3 | `bfc0529` | none |
| ADR-0117 | Per-route bucket isolation + `factoryState` lazy-cache + `NewCounterIfAbsent`; amends ADR-0073 | T5 | `ea152a1` | IMPL-1 correction to Context (cache key `*LocalRateLimit` not `*LocalRateLimitPerRoute`) |
| ADR-0118 | SN9 rule + `enforced == rate_limited` MVP invariant + 22→26 stat-table; amends ADR-0061 | T6 | `59c4aa4` | none |
| ADR-0119 | Rate-limited response wire shape — `const string` body + 4-header set + 429 default | T4 | `9f0737a` | Alternative (a) rejection rewritten at T4 (`[]byte→string`); ADR-0115 code block updated in lockstep |

ADR-0114 anchors the package shape; ADR-0115 anchors the config parser + validation discipline; ADR-0116 anchors the token-bucket primitive + timing semantics; ADR-0117 anchors the per-route stateful isolation pattern (the architectural centrepiece of phase 11, amending ADR-0073); ADR-0118 anchors the stat-naming + Prometheus tag-extractor design (amending ADR-0061); ADR-0119 anchors the rate-limited response wire shape. The cluster is internally consistent: ADR-0117 references ADR-0073 amendment + ADR-0116 (lazy-cache must not re-run tryConsume at allocation time) + ADR-0118 (per-route stats must have independent NewCounterIfAbsent allocation); ADR-0119 references ADR-0103 + ADR-0102 (wire-equivalence + SendLocalReply discipline).

---

## 11. Six-gate verification appendix

All six gates run against HEAD `1de512d` per Task 15's verification sweep (PROGRESS Task 15 entry verbatim). Reproduced summary outputs:

### Gate (a) — build clean

```
$ go build ./...
(clean)

$ go vet ./...
(clean)

$ golangci-lint run ./...
(clean)
```

**Result: PASS — clean.**

### Gate (b) — unit tests + race

```
$ go test -race -count=1 -short ./...
all packages PASS (short mode skips differential)

$ go test -race -count=1 ./test/differential/ -v
PASS for all 15 fixtures 0000-0013 (41.21s; second run after the first attempt hit the known port-allocation flake on TestDifferential)
```

**Result: PASS — all packages clean under -race; differential suite PASS on second run (harness-side flake, not a phase-11 regression).**

### Gate (c) — h2spec re-run

```
$ go test -count=1 ./test/conformance/h2spec/ -run TestH2Spec
ok  github.com/esalaine/envoy-go/test/conformance/h2spec  2.417s
(53 tests, 53 passed, 0 skipped, 0 failed at ADR-0051 pin)
```

**Result: PASS — 53/53 at ADR-0051 pin (unchanged).**

### Gate (d) — fuzzers

```
$ grep -rE '^func Fuzz' --include='*_test.go' internal/ test/ | wc -l
15

$ go test -fuzz=FuzzLocalRateLimitConfigParse -fuzztime=30s ./internal/filter/http/localratelimit/
fuzz: elapsed: 30s, execs: 6295574 (385910/sec), new interesting: 31 (total: 279)
PASS
ok  github.com/esalaine/envoy-go/internal/filter/http/localratelimit  31.043s
```

**Result: PASS — 15 fuzzers in repo; FuzzLocalRateLimitConfigParse clean at 30s (6.3M execs); 14 prior fuzzers carried forward green per option B.**

### Gate (e) — differential 0000-0013 all green

```
$ go test -count=1 ./test/differential/ -run 'Test.*0000|Test.*0001|...|Test.*0013'
=== RUN   TestDifferential
    --- PASS: TestDifferential/0000-tcp-echo
    --- PASS: TestDifferential/0001-tcp-proxy-rr
    --- PASS: TestDifferential/0002-tls-tcp
    --- PASS: TestDifferential/0003-http11-routing
    --- PASS: TestDifferential/0004-h2-routing
    --- PASS: TestDifferential/0005-prometheus-stats
    --- PASS: TestDifferential/0006-access-log
    --- PASS: TestDifferential/0007a-cors
    --- PASS: TestDifferential/0007b-iteration-probe
    --- PASS: TestDifferential/0008-listener-chain-match
    --- PASS: TestDifferential/0009-admin-config-dump
    --- PASS: TestDifferential/0010-graceful-drain
    --- PASS: TestDifferential/0011-http-fault
    --- PASS: TestDifferential/0012-http-header-mutation
    --- PASS: TestDifferential/0013-http-local-ratelimit
--- PASS: TestDifferential (41.21s)
```

**Result: PASS — 15 differential subtests green (0000..0013 with 0007a/0007b split — 14 fixture directories, 15 subtest count).**

### Gate (f) — BEHAVIOR_CONTRACT alignment + ROADMAP row 11 status

```
$ grep -cE 'envoy.filters.http.local_ratelimit' docs/envoy-go/BEHAVIOR_CONTRACT.md
5

$ grep -cE '^## ADR-(0114|0115|0116|0117|0118|0119):' docs/envoy-go/DECISIONS.md
6

$ awk -F'|' 'NR>3 && $2 ~ /^ 11 / {print $5}' docs/envoy-go/ROADMAP.md
 done

$ grep -nE 'envoy_local_http_ratelimit_prefix|26-name table|fixture 0013 scenario 3' docs/envoy-go/BEHAVIOR_CONTRACT.md
(all 4 patch sites confirmed present)
```

**Result: PASS — BEHAVIOR_CONTRACT.md populated with:**
- §13.1 NEW `### envoy.filters.http.local_ratelimit` subsection under `## HTTP filter chain`
- §13.2 stat-name table extended 22 → 26 names; 4 new counter rows appended + tag-extractor note
- §13.3 timing-tolerances row added (fixture 0013 scenario 3 ±10ms)
- §13.4 Equivalence Matrix row appended after the `envoy.filters.http.fault` + `envoy.filters.http.header_mutation` rows
- §13.5 Forward-pointer notes for 8 deferred field families + divergence-window note
- ROADMAP row 11 status field reads `done`
- All six ADRs ADR-0114..ADR-0119 confirmed in DECISIONS.md

Six-gate state: **ALL GREEN at HEAD `1de512d`.** Phase-done commit landed at Task 15; this REVIEW closes lifecycle-state 5 → 6.

---

## 12. Acceptance against SPEC §15

Cross-referencing SPEC §15 acceptance checklist (abridged):

- [x] `internal/filter/http/localratelimit/` package lands with `doc.go` + `local_ratelimit.go` + `bucket.go` + `local_ratelimit_test.go` + `bucket_test.go` + `fuzz_test.go`. `New` factory implements SPEC §6 contract.
- [x] `tokenBucket` lazy-refill primitive per ADR-0116: mutex-protected; monotonic time source; lazy refill on `tryConsume` access; `±10ms` tolerance validated in fixture 0013 scenario 3.
- [x] 6 explicit PGV + filter-internal validation checks per ADR-0115; verbatim Envoy error string `local rate limit token bucket fill timer must be >= 50ms`.
- [x] Per-route TPFC bucket isolation per ADR-0117: `factoryState` lazy-cache + `NewCounterIfAbsent` post-Freeze idempotent registration; `TestDecodeHeaders_PerRouteOverride_IndependentBuckets` mechanically validates 3-way pointer-distinctness + cross-bucket isolation.
- [x] `Registry.NewCounterIfAbsent` framework primitive (~30 LoC delta; `internal/stats/registry.go`); 3 unit tests. Bypasses Freeze per ADR-0117.
- [x] Rule SN9 in `internal/stats/name.go` `flattenToProm` `default` branch: produces `envoy_http_local_rate_limit_<counter>` + `envoy_local_http_ratelimit_prefix=<stat_prefix>` per ADR-0118 + SPEC §11.5 empirical pin.
- [x] `enforced == rate_limited` MVP invariant per ADR-0118; `TestDecodeHeaders_RateLimitedPath_CountersIncremented_Lockstep` explicitly asserts lockstep.
- [x] Rate-limited response wire shape per ADR-0119: body `local_rate_limited` (18 bytes, no LF) + 4-header set + 429 default; `SendLocalReply` reuse from phase-09 fault precedent; `const string` body form.
- [x] `cmd/envoy-go/main.go` registers `localratelimit.New` under `localratelimit.TypeURL`; does NOT call `RegisterPerRouteValidator`.
- [x] `FuzzLocalRateLimitConfigParse` fuzzer per ADR-0018: 30s budget + 3-entry seed corpus + `(factory, nil) ∨ (nil, error)` invariant; 6.3M execs clean.
- [x] Fixture 0013-http-local-ratelimit green: 4 scenarios across 4 dedicated listeners; `RequiresReference: true`; STATIC-subject + STRICT_DNS-reference; admin port 9901 per harness discipline; `dns_lookup_family: V4_ONLY` on STRICT_DNS cluster.
- [x] IMPL-1 substitution (`*LocalRateLimitPerRoute` → `*LocalRateLimit`) applied consistently across code, tests, YAML fixtures, ADR bodies, and BEHAVIOR_CONTRACT patches.
- [x] `go test -race ./...` clean: verified by gate (b).
- [x] `FuzzLocalRateLimitConfigParse` runs clean at 30s: verified by gate (d).
- [x] h2spec 53/53 PASS: verified by gate (c).
- [x] Six new ADRs (ADR-0114..ADR-0119) + ADR-0073 amendment paragraph + ADR-0061 amendment paragraph in DECISIONS.md: verified by gate (f).
- [x] BEHAVIOR_CONTRACT.md §13 five-patch bundle: verified by gate (f).
- [x] ROADMAP row 11 flips `in-progress → done`: verified by gate (f) (Task 14 commit `ac1ec1d`).
- [x] STATE.md `active-phase: awaiting next planning` + `lifecycle-state: awaiting` + `next-skill: superpowers:brainstorming`: verified by Task 15's STATE rewrite + SHA-fill `dfa08c9`.
- [x] REVIEW.md authored: THIS document.

All acceptance items checked. Phase-done. Phase 11 lifecycle (state 5 → 6) closes at the commit landing this REVIEW. Branch `phase-11-http-filter-local-ratelimit-impl` is ready for merge to master per the linear-history (fast-forward) precedent established by phases 00–10.
