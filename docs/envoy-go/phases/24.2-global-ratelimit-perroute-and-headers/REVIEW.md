# Phase 24.2 — HTTP filter `envoy.filters.http.ratelimit` (per-route + X-RateLimit headers) — Review

**Phase id:** `24.2` (SECOND and FINAL sub-phase of the phase-24 PLAN-time ADR-0045 split per ADR-0201; parent row `24 http-filter-global-ratelimit` flips `in-progress → done` AT THIS COMMIT per D-RL18 + the 18/19/22 ROLLUP precedent; **D-hypothesis HELD across the entire phase-24 family-row — ADR-0202 UNCONSUMED at 24.2 phase-done**, next-free stays `ADR-0202`.)
**Slug:** `24.2-global-ratelimit-perroute-and-headers`
**Branch under review:** `phase-24.2-global-ratelimit-perroute-and-headers-impl`
**Range:** branch tip is this Task 8 atomic-landing commit (REVIEW.md + STATE.md re-advance + ROADMAP rows 24.2 + 24 BOTH flipped in the same commit per D-RL18 + BEHAVIOR_CONTRACT.md 24.2 completion bundle + PROGRESS Task 8 entry + Task 7 fuzz_test.go header doc-drift cleanup); **16** task-landing commits on the IMPL branch ahead of master tip `3b17f43` — Pre-Task 0 (`e2334fb`) + Task 1 (`b5df160`) + Task-1 follow-up (`6fd3c5a`) + Task 2 (`9d54a31`) + Task-2 follow-up (`4355e4c`) + Task 3 (`edc4072`) + Task-3 follow-up (`653190d`) + Task 4 (`737b3b1`) + Task 5 (`1e8851f`) + Task-5 stage-close (`8d234ba`) + Task-5 follow-up I-1 (`ce8ca49`) + Task 6 (`2a438f4`) + Task-6 stage-close (`3ea9324`) + Task-6 follow-up (`45a00d9`) + Task 7 (`63d506f`) + this Task 8 atomic landing.
**Parent ROADMAP row:** **row `24 http-filter-global-ratelimit` flipped `in-progress → done` AT THIS COMMIT** (date `2026-05-23`) — the rollup per D-RL18 + the 18/19/22 precedent. Row `24.2 global-ratelimit-perroute-and-headers` ALSO flipped `planned → done` AT THIS COMMIT in the same commit (commit-message body names both transitions for grep-verifiability). The `sub-phases` column for row 24 STAYS `24.1, 24.2` (unchanged).
**PLAN tip SHA:** `81bc2be` (Squash merge phase-24.2-global-ratelimit-perroute-and-headers-plan [ADR-0199 anchored @ Task 3, ADR-0197 amend @ Task 5, ADR-0125 §(xv) @ Task 3]). **SPEC tip:** sub-phase SPEC slice at `24.2-...-SPEC.md` authored at the parent PLAN-stage commit per the in-PLAN-stage SPEC carve-out per ADR-0045. **Parent SPEC tip SHA:** see DECISIONS.md ADR-0201 §Context for the PLAN-time split lineage.
**Reviewer method:** Inline authoring by the implementing session per the PLAN Task 8 direction. Inputs: parent SPEC §15 18-item acceptance UNION + the 24.1 partial items 7/9/15/16 closure inheritance + the 24.2 SPEC slice + PLAN's 9-task structure + the branch diff (15 commits + this REVIEW commit) + PROGRESS.md per-task entries (Pre-Task 0 + Tasks 1-7 + the 4 follow-up entries) + DECISIONS.md ADR-0199 §Decision + §Consequences FULL body + ADR-0197 IN-PLACE §Decision amendment + ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph + BEHAVIOR_CONTRACT.md 24.2 completion bundle (this Task 8) + 24.1 REVIEW.md structural template precedent.
**Links:** [PLAN.md](./PLAN.md) · [SPEC.md](./SPEC.md) · parent [SPEC.md](../24-http-filter-global-ratelimit/SPEC.md) · sibling 24.1 [REVIEW.md](../24.1-global-ratelimit-core-and-route-table/REVIEW.md) · [PROGRESS.md](./PROGRESS.md).
**Six-gate state at HEAD:** all GREEN per Task 8's verification sweep — outputs reproduced verbatim in §7 below.

This review covers the full phase-24.2 surface: the EXTENDED `descriptors.go` engine to all 10 actions (`source_cluster` + `masked_remote_address` with v4=32/v6=128 default prefix masks + `metadata` via the existing `DynamicMetadata()` + the NEW `RouteMetadata()` accessor per the ADR-0165 set-once extension template + `query_parameters` with AMEND-11 default key `"query_param"` SINGULAR + `query_parameter_value_match` with default key `"query_match"`); the NEW `internal/filter/http/ratelimit/compiled_perroute.go` (`RateLimitPerRoute` 10th-canonical TPFC compile + the ADR-0110 single-chokepoint validator + the request-time projection); the NEW `encode.go` + `headers.go` (X-RateLimit DRAFT_VERSION_03 emission on ALL non-fail-closed dispositions + MIN-status across multi-descriptor responses + AMEND-8 wire-order at `[a]+[X-RateLimit]+[b]+[c]` per the Task-5 follow-up I-1 fix); the NEW `stage` multi-stage bucketing path (per parent §4.4 — `bucketRateLimitsByStage` parse-time `[11]bucket` + per-request filter-stage selection); the Axis-B `vh_rate_limits` cross-tier composition table (per §4.3 + AMEND-5 — OVERRIDE/INCLUDE/IGNORE honored at runtime + legacy `RouteAction.include_vh_rate_limits=true` force-include arm); the EXTENDED differential fixture `0032-http-ratelimit` (scenarios (f) `vh_inclusion` + (g) `x_ratelimit_headers` + (d) extended to all 10 actions; NO new fixture dir — stays 35); the EXTENDED `FuzzRateLimitConfigParse` corpus (+15 seeds per D-RL16; project fuzzer count STAYS at 33); the BEHAVIOR_CONTRACT.md 24.2 completion bundle (this Task 8); the 3 ADR landings (ADR-0199 FULL + ADR-0197 IN-PLACE amendment + ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph anchored in ADR-0199 body).

This REVIEW closes phase-24.2's IMPL lifecycle (state 5 → 6) AND closes the entire phase-24 family-row via the parent rollup (D-RL18). It is the final task before squash-merge to master.

---

## 1. Summary

**APPROVED.** All six phase-done gates are GREEN at HEAD per Task 8's verification sweep. The implementation faithfully realizes the SPEC across all 9 PLAN tasks (Pre-Task 0 + Tasks 1-8) plus 4 follow-up commits (Task-1 doc/test review-fix, Task-2 docstring softening, Task-3 LoC/reflect upgrade, Task-5 I-1 AMEND-8 wire-order fix, Task-6 driver.go doc-drift). Phase 24.2 is the SECOND (and FINAL) sub-phase of the phase-24 PLAN-time ADR-0045 split per ADR-0201; **the parent row `24 http-filter-global-ratelimit` flips `in-progress → done` AT THIS COMMIT** per D-RL18 + the 18/19/22 ROLLUP precedent.

**One IMPL-time follow-up CRITICAL fix; zero algorithmic / behavioral deviations from the PLAN.** Four artifact follow-ups landed during IMPL: (1) **Task-1 follow-up** addressing 3 review notes (descent-semantics docstring + non-Struct intermediate test + ROUTE_ENTRY nested + non-string-leaf coverage); (2) **Task-2 follow-up** softening the `compiledConfig.stage` docstring to match `descriptors.go` runtime predicate; (3) **Task-3 follow-up** addressing M-1 code-quality (drop inaccurate LoC numbers in ADR-0199 §Decision) + upgrading `TestCompiledPerRoute_StructShape` to reflect-based field-roster pin; (4) **Task-5 follow-up I-1** the CRITICAL AMEND-8 wire-order fix — the initial Task-5 commit placed X-RateLimit headers AFTER the filter-config `response_headers_to_add` slot [c]; the upstream wire order per parent SPEC §4.7 line 214 is `[a] x-envoy-ratelimited → [X-RateLimit] → [b] RLS response_headers_to_add → [c] filter-config response_headers_to_add`; the follow-up moved X-RateLimit inline at `applyOverLimit` between slot [a] and slot [b]; 1 new test asserting wire order. (5) **Task-6 follow-up** cleaning up driver.go doc-drift (I-1 stale 2-vhost-design comments + I-2 stale arithmetic block). All five fixes are recorded at §3 below.

**Three ADR landings (all anticipated per PLAN):**
- **ADR-0199** §Decision + §Consequences FULL — `RateLimitPerRoute` 10th-canonical TPFC compile + the ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph anchored IN-BODY. Landed at Task 3 (`edc4072` + Task-3 follow-up `653190d`).
- **ADR-0197 IN-PLACE §Decision amendment** (the X-RateLimit + remaining-actions slice per ADR-0052) — appended to the 24.1-anchored §Decision body. Landed at Task 5 (`1e8851f` + Task-5 follow-up I-1 AMEND-8 wire-order fix `ce8ca49`).
- **ADR-0125 §(xv) AMENDMENT 9 → 10** — paragraph LANDED INSIDE ADR-0199 body at Task 3 (the anchor for the 10th canonical per ADR-0125 §(xv); ENDING the phase-23 + 24.1 REUSE-by-absence skip streak).

**ADR-0202 UNCONSUMED at 24.2 phase-done — D-hypothesis HELD across the entire phase-24 family-row.** Both byte-confirmation surfaces resolved cleanly:
- **D-RL8 (Task 1 — `metadata` action value-extraction accessor):** the existing `DecoderFilterCallbacks.DynamicMetadata()` accessor satisfies `MetadataSource_DYNAMIC=0` via `Bucket.Get(filterName, topKey)` followed by segmented descent through `*structpb.Value`; the `MetadataSource_ROUTE_ENTRY=1` path required a NEW `RouteMetadata() *corev3.Metadata` accessor added via the ADR-0165 set-once-by-dispatch extension template (chain-field + setter + chain-accessor + decoderCB accessor + HCM-dispatch seed at H1 + H2). Clean extension; **ADR-0202 UNCONSUMED**.
- **D-RL9 (Task 5 — X-RateLimit DRAFT_VERSION_03 byte format):** byte-exact reproduction of the upstream `ratelimit_headers.cc:13-65` byte format pinned at `headers_test.go` against captured upstream headers; verified cross-side byte-exact at fixture `0032-http-ratelimit` scenario (g). **ADR-0202 UNCONSUMED**.

**Next-free ADR: ADR-0202** (UNCHANGED from 24.1 phase-done — no new ADR numbers consumed at 24.2; ADR-0199 + ADR-0197 amend + ADR-0125 §(xv) all consume existing reserved slots).

**ADR-0125 canonical-per-route roster grows 9 → 10 at 24.2 Task 3** (the FIRST roster growth since phase-22.3 / ADR-0193 — ENDING the phase-23 + phase-24.1 REUSE-by-absence skip streak per ADR-0125 §(xv) + ADR-0199 §Decision body).

---

## 2. Parent SPEC §15 acceptance UNION verification (the FULL 18-item set)

Per the parent SPEC §15 directive: "**If the PLAN author splits into 24.1/24.2 (§16), this checklist is the UNION across the sub-phases** (each sub-phase's REVIEW confirms its slice; row-24 flips to `done` only when both sub-rows land — item 17)." This Task 8 REVIEW.md verifies the FULL UNION — the 24.1 partial items 7/9/15/16 CLOSED at this commit + the new items 12/14 LANDED + item 17 doc-state UNION + item 18 end-to-end audit-trail.

### A. Six gates (items 1-6 — FULL at 24.2 HEAD per Gate sweep)

- [x] **Item 1 — Gate A build clean.** **GREEN.** `go build ./...` exits 0 at HEAD. See §7 verbatim output.
- [x] **Item 2 — Gate B vet + lint clean.** **GREEN.** `go vet ./...` exits 0; `golangci-lint run` exits 0; no new lint suppressions across the phase-24.2 surface. See §7 verbatim output.
- [x] **Item 3 — Gate C race clean.** **GREEN.** Initial `go test -race -count=1 ./...` hit the documented multi-listener `freeTCPPort` flake at `0018-http-rbac` (`subject ready: EOF`); re-ran clean in isolation (`ok ... 3.468s`). All 24.2-surface packages race-clean. See §7 verbatim output.
- [x] **Item 4 — Gate D differential 35/35 GREEN.** **GREEN.** Initial run flaked `0020-http-ext-authz-http` (same multi-listener `EOF` flake class); re-ran clean in isolation; a second `-v` full run with the anchored regex showed all 35/35 subtests PASS + `--- PASS: TestDifferential (85.40s)`. The two NEW 24.2 scenario-extensions to `0032-http-ratelimit` ((f) vh_inclusion + (g) x_ratelimit_headers + (d) extended to all 10 actions) PASS on the cross-side runner per Task 6 PROGRESS entry. See §7 verbatim output.
- [x] **Item 5 — Gate E fuzz clean.** **GREEN.** `FuzzRateLimitConfigParse` 46-seed corpus clean (31 seeds from 24.1 + 15 new at 24.2 per D-RL16); 30s live-fuzz clean: 1,784,920 execs in 30s; 10 new-interesting; 0 panics; 0 crashers. Project fuzzer count STAYS at **33** (`find ... -name 'fuzz_test.go' | xargs grep -h '^func Fuzz' | sort -u | wc -l` = 33 — D-RL16 corpus-only extension). See §7 verbatim output.
- [x] **Item 6 — Gate F h2spec 53/53 PASS.** **GREEN.** `go test -v -count=1 ./test/conformance/h2spec/` reports `53 tests, 53 passed, 0 skipped, 0 failed` at the ADR-0051 v1.32.4 pin. PASS at 2.45s. See §7 verbatim output.

### B. Two-directory differential fixture coverage (item 7 — 24.1 partial CLOSED at 24.2)

- [x] **Item 7 — Two-directory differential per §7 (24.1 partial CLOSED).** **GREEN.** `0032-http-ratelimit` extended to 9 scenarios at 24.2: (a) parse_ok + (b) ok_admit + (c) over_limit_reject + (d) descriptor_engine **EXTENDED to all 10 actions** + (e) error_fail_open + (f) **vh_inclusion** (NEW — 3 sub-scenarios f1/f2/f3 covering OVERRIDE / INCLUDE / IGNORE per the §4.3 Axis-B table) + (g) **x_ratelimit_headers** (NEW — cross-side byte-exact DRAFT_VERSION_03 emission) + (h) StatsAsserter. `0033-http-ratelimit-boot-reject` UNCHANGED (`domain`-empty shared boot-reject). Fixture dir count STAYS at 35 (Task 6 extends `0032` in-place per the 24.2 SPEC §16 axis — NO new fixture dir). Subject-side assertions in `StatsAsserter` per project memory `reference_differential_asserter_dispatch.md`. Evidence: Task 6 PROGRESS entry + Task-6 stage-close + follow-up + Gate D GREEN at 35/35.

### C. Cluster-scoped stat-surface verification (item 8 — 24.1 FULL; reaffirmed at 24.2)

- [x] **Item 8 — Cluster-scoped 4-counter stat surface STAYS 110 → 114 at 24.2.** **GREEN.** Per AMEND-1: per-route `RateLimitPerRoute.domain` override is a **descriptor-tier override, NOT a stat namespace**. Stat count STAYS at 114 after phase 24.2 (no new rows; the per-route `domain` does NOT extend the qualifier surface). The 4 stat names (`ok`/`error`/`over_limit`/`failure_mode_allowed`) under `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>` CLUSTER-rooted prefix STAY at the 24.1-landed shape. BEHAVIOR_CONTRACT.md 24.2 completion bundle adds a NEW per-route `domain`-qualifier paragraph documenting this disposition (the per-route `domain` is visible to the RLS service via the `RateLimitRequest.domain` field but does NOT participate in stat naming). Per-route stats SHARED with listener-level remains the 24.1 discipline at 24.2. Evidence: BEHAVIOR_CONTRACT.md `## Stat-name mapping` paragraph at this Task 8 commit + Task 3 PROGRESS entry.

### D. Descriptor-engine fidelity for ALL 10 actions + empty-action-drop (item 9 — 24.1 partial CLOSED)

- [x] **Item 9 — Descriptor-engine fidelity (all 10 of 10 actions LANDED at 24.2).** **GREEN.** All 5 remaining actions landed at Task 1 `descriptors.go`: `actionSourceCluster` (chain-seeded `FactoryCtx.NodeServiceCluster` from the Envoy NODE's `service-cluster`) + `actionMaskedRemoteAddress` (v4=32/v6=128 default prefix masks; per-action overrides via UInt32Value fields) + `actionMetadata` (DYNAMIC via `DynamicMetadata()`; ROUTE_ENTRY via the NEW `RouteMetadata()` accessor; segmented `MetadataKey.path` descent through `*structpb.Value`; `default_value` fallback; `skip_if_absent` drop-vs-no-op) + `actionQueryParameters` (default key `"query_param"` SINGULAR per AMEND-11; `query_parameter_name`-driven) + `actionQueryParameterValueMatch` (per-request `QueryParameterMatcher` evaluation). The `actionUnsupportedAt241()` arm is DELETED at Task 1. The empty-action-drop discipline per `router_ratelimit.cc:21-39` continues to fire at the `buildDescriptorForPolicy` level. Stage filter per parent §4.4 landed at Task 2 (`bucketRateLimitsByStage` parse-time `[11]bucket` + per-request filter-stage selection; first-stage OVER_LIMIT short-circuits subsequent stages). Axis-B `vh_rate_limits` cross-tier composition landed at Task 4 (OVERRIDE/INCLUDE/IGNORE per the §4.3 decision table + the legacy `RouteAction.include_vh_rate_limits=true` force-include arm — gated by OVERRIDE arm; IGNORE supersedes the legacy bool). Axis-A early-return on non-empty per-route `rate_limits[]` per AMEND-4 (lands at Task 3 with the per-route TPFC compile). **Item 9 PARTIAL → FULL closure at 24.2.** Evidence: Task 1 + Task 2 + Task 3 + Task 4 PROGRESS entries + Gate C race-clean on `internal/filter/http/ratelimit` + Gate D `0032-http-ratelimit` scenarios (d-extension) + (f-vh_inclusion) GREEN.

### E. PARSE-REJECT roster (item 10 — 24.1 FULL; ratified at 24.2)

- [x] **Item 10 — FULL §5 PARSE-REJECT roster byte-stable.** **GREEN.** Per ADR-0200 (LANDED at 24.1 Task 3): the RATIFIED-from-PGV/config arms (empty `domain`; missing `rate_limit_service`; `stage > 10`; bad `request_type`; >10 response headers) + the 3 envoy-go-strict arms (`disable_key` non-empty; `extension` action; `dynamic_metadata` action). At 24.2 Task 3 the per-route `RateLimitPerRoute` validator (`validatePerRouteRateLimit`) RECURSIVELY invokes `ValidateRouteRateLimits` against the embedded `rate_limits[]` so the FULL §5 roster fires at per-route TPFC compile time. At 24.2 Task 2 the `stage > 10` PARSE-REJECT arm extends to the per-policy granularity (`ValidateRouteRateLimits` REJECTS per-policy stage > 10 with the same byte-stable wording). All boot-reject pinning via the existing `0033-http-ratelimit-boot-reject` fixture (`domain`-empty arm). 0 new departures at 24.2 (the 3 phase-24.1 departure records cover the surface). Evidence: Task 2 + Task 3 PROGRESS entries + Task 7 fuzzer Seeds 43/44 (`stage=5` valid + `stage=11` PARSE-REJECT).

### F. Disposition + reply byte-shape (item 11 — 24.1 FULL; reaffirmed at 24.2)

- [x] **Item 11 — OK/OVER_LIMIT/error dispositions + reply byte-shape.** **GREEN.** 24.1-landed dispositions REMAIN BYTE-EXACT at 24.2:
  - OK: `ok` counter Inc; `ContinueDecoding`. The 24.2 X-RateLimit DRAFT_VERSION_03 emit fires at the encode hook (Task 5).
  - OVER_LIMIT: `over_limit` counter Inc; `SendLocalReply(cc.rateLimitedStatus, string(resp.RawBody), headers)` with the **AMEND-8 wire-order at `[a]+[X-RateLimit]+[b]+[c]`** per the Task-5 follow-up I-1 fix (parent SPEC §4.7 line 214). Status default 429; <400 clamps to 429. `ContinueDecoding` AFTER `SendLocalReply` (the 24.1 Task-7 follow-up wake-up fix REMAINS LIVE at 24.2).
  - error: `error` counter ALWAYS Inc; fail-OPEN (default) `failure_mode_allowed` Inc + `ContinueDecoding` (X-RateLimit DOES emit on fail-OPEN per AMEND-8); fail-CLOSED `SendLocalReply(cc.statusOnError, "", nil)` (nullptr-mutate; X-RateLimit does NOT emit — encode hook does not participate). Default `cc.statusOnError = 500`; <400 clamps to 500.
  - **Task-5 follow-up I-1 AMEND-8 wire-order fix:** the initial Task-5 commit placed X-RateLimit headers AFTER the filter-config `response_headers_to_add` slot [c]. The upstream wire order per parent SPEC §4.7 line 214 is `[a] x-envoy-ratelimited → [X-RateLimit] → [b] RLS response_headers_to_add → [c] filter-config response_headers_to_add`. The follow-up moved X-RateLimit inline at `applyOverLimit` between slot [a] and slot [b]; 1 new test asserting wire order. Evidence: Task 5 + Task-5 follow-up PROGRESS entries + Gate D `0032-http-ratelimit` scenario (g) GREEN.

### G. X-RateLimit header verification (item 12 — NEW at 24.2; FULL at 24.2 HEAD)

- [x] **Item 12 — DRAFT_VERSION_03 X-RateLimit headers per §4.7 + AMEND-8.** **GREEN.** `encode.go` + `headers.go` LANDED at Task 5 (`1e8851f` + follow-up `ce8ca49`). Three response headers emitted on ALL non-fail-closed dispositions when `enable_x_ratelimit_headers == DRAFT_VERSION_03`:
  - `x-ratelimit-limit: <MIN.requests_per_unit>[, <rpu>;w=<window_sec>[;name="<n>"]]...` — MIN selection by `limit_remaining`; quota-policy suffix per descriptor with non-zero window; Unit→seconds: SECOND=1, MINUTE=60, HOUR=3600, DAY=86400, WEEK=604800, MONTH=2592000, YEAR=31536000; UNKNOWN/0 ⇒ no quota-policy segment for that descriptor; `;name=` value quoted per upstream `ratelimit_headers.cc:13-65`.
  - `x-ratelimit-remaining: <MIN.limit_remaining>` — integer-ASCII.
  - `x-ratelimit-reset: <MIN.duration_until_reset.seconds>` — integer-ASCII.

  MIN-selection tie-break: preserve insertion order (= descriptor-list order = action-list order per AMEND-6); FIRST equal-minimum status wins. Fail-closed path does NOT emit X-RateLimit (nullptr-mutate per AMEND-8). Byte-pinned via `headers_test.go` against captured upstream headers; cross-side byte-exact at fixture `0032-http-ratelimit` scenario (g). **D-RL9 byte-confirmation outcome: RESOLVED CLEANLY — ADR-0202 UNCONSUMED.** Evidence: Task 5 PROGRESS entry (`1e8851f`) + Task-5 follow-up I-1 (`ce8ca49`) + Gate D scenario (g) GREEN.

### H. DELTA-1 + DELTA-2 verification (item 13 — 24.1 FULL; reaffirmed at 24.2)

- [x] **Item 13 — DELTA-1 + DELTA-2 + 19 HTTP filters wired.** **GREEN.** Both deltas LANDED at 24.1 (DELTA-1 `RateLimitClient` at 24.1 Task 2 commit `3ee365c`; DELTA-2 HCM route-table `rate_limits` exposure at 24.1 Task 5 commit `7b91ef7`). At 24.2 the DELTA-2 plumbing extends with the NEW `RouteMetadata()` accessor (Task 1; same ADR-0165 set-once-by-dispatch template). 19 HTTP filters wired UNCHANGED at 24.2 (Task 7's corpus-only extension does NOT register a new filter; HTTP filters STAY at 19).

### I. Per-route `RateLimitPerRoute` 10th-canonical verification (item 14 — NEW at 24.2; FULL at 24.2 HEAD)

- [x] **Item 14 — `RateLimitPerRoute` 10th canonical per §5.3 + ADR-0199.** **GREEN.** `compiled_perroute.go` LANDED at Task 3 (`edc4072` + Task-3 follow-up `653190d`): the per-route TPFC compile with the ADR-0110 single-chokepoint validator (`validatePerRouteRateLimit`) + the request-time projection (`compilePerRouteForRequest`). All 4 fields honored:
  - **`vh_rate_limits`** inclusion enum (OVERRIDE/INCLUDE/IGNORE; DEFAULT=OVERRIDE) — Axis-B cross-tier composition lands at Task 4 (`737b3b1`) via the request-time walk;
  - **`rate_limits[]`** route-additional Axis-A (when non-empty ⇒ early-return per AMEND-4);
  - **`override_option`** accepted-but-IGNORED INERT per AMEND-4 (parse-accepted; NEVER consulted at request time; NOT a departure — upstream-parity);
  - **`domain`** descriptor-tier override (when set, descriptor's `domain` is the per-route value; the stat namespace UNCHANGED per AMEND-1).

  **ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph LANDED INSIDE ADR-0199 §Decision body at Task 3** — the FIRST roster growth since phase-22.3 / ADR-0193 — ENDING the phase-23 + 24.1 REUSE-by-absence skip streak. The 10th canonical is the NEW `data-only-with-vh-inclusion-enum` shape that DIVERGES from all 9 prior canonicals. BEHAVIOR_CONTRACT.md `## Per-route canonical patterns cross-reference` extended with the new table row §(xv) + a phase-24.2 cross-reference paragraph. Evidence: Task 3 + Task 4 PROGRESS entries + DECISIONS.md ADR-0199 §Decision body + BEHAVIOR_CONTRACT.md updated table.

### J. ADR landing (item 15 — 24.1 partial CLOSED at 24.2)

- [x] **Item 15 — ALL 4 NEW ADR §Context drafts + §Decision + §Consequences bodies landed across 24.1 + 24.2.** **GREEN.** Phase-24 family-row ADR-landing roster complete:
  - **ADR-0197** §Decision + §Consequences — 24.1 slice at Task 7 (`5c665fa`); **IN-PLACE §Decision amendment** for the X-RateLimit + remaining-actions slice at 24.2 Task 5 (`1e8851f` + follow-up `ce8ca49`).
  - **ADR-0198** §Decision + §Consequences FULL — at 24.1 Task 5 (`7b91ef7`).
  - **ADR-0199** §Decision + §Consequences FULL — at 24.2 Task 3 (`edc4072` + follow-up `653190d`); ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph anchored IN-BODY.
  - **ADR-0200** §Decision + §Consequences FULL — at 24.1 Task 3 (`acbc3d1`).
  - **ADR-0125 §(xv) AMENDMENT 9 → 10** — paragraph LANDED at 24.2 Task 3 (anchored in ADR-0199 body).
  - **ADR-0201** — PLAN-time split ADR (consumed at the PLAN-stage).

  Total: **4 NEW ADRs** (ADR-0197..0200) **+ 1 in-place amendment** (ADR-0197) **+ 1 ADR-0125 amendment** (§(xv)) **+ 1 PLAN-time split ADR** (ADR-0201). All 5 anchored. Item 15 PARTIAL → FULL closure at 24.2. Evidence: `grep -cE '^## ADR-019[7-9]\b'` = 3; `'^## ADR-0200\b'` = 1; `'^## ADR-0201\b'` = 1; `'^## ADR-0202\b'` = 0 (D-hypothesis HELD).

### K. BEHAVIOR_CONTRACT.md completion bundle (item 16 — 24.1 partial CLOSED at 24.2)

- [x] **Item 16 — BEHAVIOR_CONTRACT.md 24.2 completion bundle landed atomically per ADR-0052.** **GREEN.** At this Task 8 commit:
  - **(1) Subsection extension** of `### envoy.filters.http.ratelimit` — added `#### Phase 24.2 completion bundle` paragraph block IN-PLACE (engine completion to all 10 actions including the AMEND-11 default-key roster + the `metadata` accessor disposition + the `destination_cluster` framework-limited disposition + the §4.4 stage multi-bucket discipline + the §4.3 Axis-B `vh_rate_limits` decision table + the X-RateLimit DRAFT_VERSION_03 emission discipline + the per-route `RateLimitPerRoute.domain` override + the `RateLimitPerRoute.override_option` accepted-but-INERT note + the ADR-0202 escape-valve disposition).
  - **(2) Per-route canonical-patterns caption** updated "through phase 24.1" → "through phase 24"; added §(xiv) 9th canonical (lua per phase 22.3) + §(xv) 10th canonical (ratelimit per phase 24.2) rows to the table; added the phase-24.2 cross-reference paragraph documenting the §(xv) AMENDMENT 9 → 10 LANDED at 24.2 Task 3 per ADR-0199.
  - **(3) Response-header allow-list paragraph** — added 3 NEW rows to the `## Header allow-list` table: `x-ratelimit-limit` + `x-ratelimit-remaining` + `x-ratelimit-reset` (set-equal byte-exact per scenario (g)); added the X-RateLimit allow-list note paragraph documenting set-equal-byte-exact disposition + the emission gating + the OVER_LIMIT wire-order at `[a]+[X-RateLimit]+[b]+[c]` per AMEND-8 + the MIN-selection tie-break.
  - **(4) Stat-name `domain`-qualifier paragraph** — added `**Phase 24.2 per-route `domain`-qualifier disposition**` paragraph IN-PLACE inside `## Stat-name mapping` documenting the 110 → 114 STAYS at 114 disposition per AMEND-1 (per-route `domain` is descriptor-tier, NOT a stat namespace; stat namespace UNCHANGED; per-route stats SHARED with listener-level remains the discipline).

  **NO new departure records at 24.2** (the 3 phase-24.1 departure records `disable_key`/`extension`/`dynamic_metadata` cover the surface; `override_option` accepted-but-INERT is upstream-parity, NOT a departure). Departure count STAYS at 18 after phase 24.2. Item 16 PARTIAL → FULL closure at 24.2. Evidence: this Task 8 commit's BEHAVIOR_CONTRACT.md diff.

### L. Doc-state alignment + STATE re-advance (item 17 — NEW at 24.2; FULL at 24.2 HEAD)

- [x] **Item 17 — Doc-state alignment at parent §15 acceptance UNION.** **GREEN.** At this Task 8 commit:
  - **DECISIONS.md** — ADR-0197 IN-PLACE amendment + ADR-0199 FULL + ADR-0125 §(xv) AMENDMENT 9 → 10 all anchored; next-free stays `ADR-0202` (D-hypothesis HELD).
  - **STATE.md** — re-advanced to `awaiting next planning (wasm — final §9 family-row; not yet a ROADMAP row)` lifecycle-state 6 → 0/1; next-skill `superpowers:brainstorming` (to author BRAINSTORM.md for the `wasm` §9 final family-row + append the `wasm` ROADMAP row per the established §9 family-row addition discipline); ADR-0202 UNCONSUMED disposition recorded.
  - **ROADMAP.md** — row 24.2 flipped `planned → done` (date 2026-05-23) AND parent row 24 flipped `in-progress → done` (date 2026-05-23) IN THE SAME COMMIT per D-RL18 + the 18/19/22 ROLLUP precedent. Row 24's `sub-phases` column STAYS `24.1, 24.2` (unchanged). The commit-message body names BOTH transitions for grep-verifiability.
  - **HTTP filters wired** — STAYS at **19** (no new boot-registration at 24.2; ratelimit alphabetical between `oauth2` and `rbac` from 24.1 unchanged).
  - **§9 HTTP-filters family** closes to **1 remaining row: `wasm`** (not yet a ROADMAP row). 17 §9 family-rows landed (phases 7.1 / 9 / 10 / 11 / 12 / 13 / 14 / 15 / 16 / 17 / 18 / 19 / 20 / 21 / 22 / 23 / 24).

  Evidence: STATE.md + ROADMAP.md + BEHAVIOR_CONTRACT.md + DECISIONS.md diffs at this Task 8 commit.

### M. End-to-end audit-trail (item 18 — NEW at 24.2; FULL at 24.2 HEAD)

- [x] **Item 18 — End-to-end audit-trail SPEC → PLAN → PROGRESS → REVIEW chains.** **GREEN.** Phase-24 family-row chain landed:
  - **24.1 chain:** `docs/envoy-go/phases/24.1-global-ratelimit-core-and-route-table/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` ALL present + per-task PROGRESS records map 1:1 to PLAN tasks (Pre-Task 0 + Tasks 1-11 + 2 follow-ups + Task 12 atomic landing).
  - **24.2 chain:** `docs/envoy-go/phases/24.2-global-ratelimit-perroute-and-headers/{SPEC.md, PLAN.md, PROGRESS.md, REVIEW.md}` ALL present + per-task PROGRESS records map 1:1 to PLAN tasks (Pre-Task 0 + Tasks 1-7 + 5 follow-ups + Task 8 atomic landing).
  - **Parent 24 chain:** `docs/envoy-go/phases/24-http-filter-global-ratelimit/{BRAINSTORM.md, SPEC.md}` ALL present (parent SPEC RETAINED + annotated with the split-fired note + the 24.2 phase-done rollup).
  - **D-hypothesis disposition recorded:** ADR-0202 UNCONSUMED at phase-24 phase-done (both D-RL8 + D-RL9 byte-confirmation outcomes recorded in 24.2 PROGRESS Task 1 + Task 5 entries; recorded in STATE.md + this REVIEW.md + the parent ROADMAP row 24's per-cell done annotation).
  - **Six-gate verbatim outputs** captured at this REVIEW.md §7 + the 24.2 PROGRESS.md Task 8 entry.
  - **Per-task commit SHAs** captured at §5 below + the 24.2 PROGRESS.md per-task entries.

  Evidence: directory listings + PROGRESS-to-PLAN cross-references + §7 verbatim outputs at this Task 8 commit.

**Summary:** all 18 of the parent §15 acceptance UNION items GREEN; 0 BLOCKED; 0 GREEN-WITH-NOTED-DEVIATION; 0 remaining PARTIAL annotations (all 24.1 partials CLOSED at 24.2; all 24.2 new items LANDED). Phase-24 family-row CLOSED at this Task 8 commit per the parent §15 acceptance UNION verification + the D-RL18 ROADMAP rollup.

---

## 3. IMPL-time follow-ups (NOT algorithmic deviations from the PLAN)

Five follow-up commits occurred during 24.2 IMPL. NONE is an algorithmic / behavioral deviation from the PLAN; all are recorded here for completeness per the Task 8 instruction.

### Follow-up 1 — Task-1 (`6fd3c5a`) — review-fix bundle (3 notes)

**Planned:** Task 1 lands the 5 remaining `descriptors.go` actions + the AMEND-11 key defaults + the `metadata` accessor (D-RL8 escape-valve target #1).

**What happened:** Reviewer surfaced 3 small notes against the initial Task-1 commit. (I-1) descent-semantics docstring inaccuracy in `actionMetadata`; (I-2) missing test for non-Struct intermediate `*structpb.Value` shapes; (I-3) ROUTE_ENTRY-nested-with-non-string-leaf coverage gap.

**Fix:** Docstring tightened to match the implementation; 2 new test cases (`TestActionMetadata_NonStructIntermediate` + `TestActionMetadata_RouteEntry_NestedNonStringLeaf`). ZERO functional change.

### Follow-up 2 — Task-2 (`4355e4c`) — `compiledConfig.stage` docstring softening

**Planned:** Task 2 lands the multi-stage bucketing path per parent §4.4.

**What happened:** Reviewer surfaced an I-1 mismatch between the `compiledConfig.stage` docstring (asserting "is consulted at request time" exact semantics) and the `descriptors.go` runtime predicate (which is per-policy `stage` against the filter's `stage` field via `bucketRateLimitsByStage`).

**Fix:** Softened docstring wording to "is bucketed at parse time + filtered at request time" matching the actual runtime predicate. ZERO functional change.

### Follow-up 3 — Task-3 (`653190d`) — ADR-0199 §Decision LoC fix + reflect-based struct-shape pin

**Planned:** Task 3 lands `compiled_perroute.go` + ADR-0199 FULL + ADR-0125 §(xv) AMENDMENT 9 → 10.

**What happened:** Reviewer surfaced an M-1 code-quality note — ADR-0199 §Decision text included inaccurate LoC numbers (drift between the §Decision-time draft and the actual code shape). Also: `TestCompiledPerRoute_StructShape` was hand-coded against the field names rather than reflect-based, which would silently miss a field-addition regression.

**Fix:** Dropped the LoC numbers from ADR-0199 §Decision body (the SHAPE is the load-bearing part; LoC is incidental). Upgraded `TestCompiledPerRoute_StructShape` to reflect-based field-roster pin (asserts the EXACT 4-field set of `compiledPerRoute` via `reflect.TypeOf().Field(i).Name` iteration). ZERO functional change.

### Follow-up 4 — Task-5 I-1 (`ce8ca49`) — CRITICAL AMEND-8 wire-order fix

**Planned:** Task 5 lands `encode.go` + `headers.go` X-RateLimit DRAFT_VERSION_03 emission + ADR-0197 IN-PLACE §Decision amendment.

**What happened:** Reviewer surfaced an I-1 wire-order regression in the initial Task-5 commit. The OVER_LIMIT path was emitting X-RateLimit headers AT the encode hook AFTER the filter-config `response_headers_to_add` slot [c] (because the encode hook fires AFTER the `SendLocalReply` headers are assembled). The upstream wire order per parent SPEC §4.7 line 214 is `[a] x-envoy-ratelimited → [X-RateLimit] → [b] RLS response_headers_to_add → [c] filter-config response_headers_to_add`.

**Fix:** Moved X-RateLimit injection INLINE at `applyOverLimit` between slot [a] and slot [b] (NOT at the encode hook for the OVER_LIMIT path). The encode hook still fires for the OK + fail-open paths. 1 new test asserting wire order at `TestApplyOverLimit_XRateLimit_WireOrder`. The other 2 paths (OK + fail-OPEN) use the encode hook and emit X-RateLimit AFTER the request headers — that ordering is unaffected by this fix.

**Consequence:** The OVER_LIMIT X-RateLimit emission wire order is now byte-exact upstream. Cross-side scenario (g) at fixture `0032-http-ratelimit` ratifies the fix. NO change to the algorithmic / behavioral contract of the PLAN — the wire-order assertion was in the SPEC; the fix realigned the IMPL to the SPEC.

### Follow-up 5 — Task-6 (`45a00d9`) — driver.go doc-drift cleanup

**Planned:** Task 6 lands the differential fixture EXTENSIONS to `0032-http-ratelimit` ((f) `vh_inclusion` + (g) `x_ratelimit_headers` + (d) extended to all 10 actions).

**What happened:** Reviewer pass surfaced 2 doc-drift notes in `0032-http-ratelimit/driver.go`. (I-1) stale 2-vhost-design comments from an earlier draft of scenario (f) — the as-implemented design uses 3 sub-scenarios f1/f2/f3 with single-vhost each, NOT a 2-vhost mux; (I-2) stale arithmetic block computing expected counters per the abandoned 2-vhost design.

**Fix:** Docs-only sweep — removed stale 2-vhost-design comments + the stale arithmetic block; the driver.go file-header + setupRLS docstring + driveProxy `AssertStats` prose now consistently describe the as-implemented topology. ZERO functional change.

---

## 4. ADR roster

Three ADR §Decision-touchpoints landed at phase-24.2 IMPL. All landed at their per-Task Lands-in-Tasks per ADR-0044.

| ADR | §Decision / §Consequences disposition | Lands-in-Task | Commit SHA |
|---|---|---|---|
| **ADR-0199** | **§Decision + §Consequences FULL** — `RateLimitPerRoute` 10th-canonical TPFC compile + the ADR-0110 single-chokepoint validator + the request-time projection + the ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph anchored IN-BODY (the FIRST roster growth since phase-22.3 / ADR-0193). The 10th canonical is the NEW `data-only-with-vh-inclusion-enum` shape (4 fields: `vh_rate_limits` enum + `rate_limits[]` Axis-A + `override_option` accepted-but-IGNORED + `domain` descriptor-tier override). | Task 3 | `edc4072` (+ `653190d` follow-up) |
| **ADR-0197 IN-PLACE §Decision amendment** | **In-place amendment per ADR-0052** — the X-RateLimit + remaining-actions slice. The 24.1 §Decision body covered the CORE decision path; the 24.2 amendment closes the X-RateLimit DRAFT_VERSION_03 emission + the AMEND-8 wire-order at `[a]+[X-RateLimit]+[b]+[c]` (with the Task-5 follow-up I-1 fix) + the 5 remaining actions' descriptor production. | Task 5 | `1e8851f` (+ `ce8ca49` follow-up) |
| **ADR-0125 §(xv) AMENDMENT 9 → 10** | **AMENDMENT 9 → 10 paragraph** landed inside ADR-0199 §Decision body — documents the NEW 10th canonical per-route shape `data-only-with-vh-inclusion-enum`; ENDS the phase-23 + 24.1 REUSE-by-absence skip streak. | Task 3 (anchored in ADR-0199 body) | `edc4072` |

**Next-free ADR: ADR-0202** (UNCHANGED from 24.1 phase-done; D-hypothesis HELD across the entire phase-24 family-row; escape valve UNCONSUMED). DECISIONS.md tail at ADR-0201; ADR-0202 absent.

**D-RL8 + D-RL9 byte-confirmation outcomes:**
- **D-RL8 (Task 1):** RESOLVED CLEANLY — the existing `DynamicMetadata()` + the NEW `RouteMetadata()` accessor (ADR-0165 set-once extension template) fit the existing primitives without divergence. ADR-0202 UNCONSUMED.
- **D-RL9 (Task 5):** RESOLVED CLEANLY — byte-exact reproduction of the upstream `ratelimit_headers.cc:13-65` format pinned at `headers_test.go` AND verified cross-side byte-exact at fixture `0032-http-ratelimit` scenario (g). ADR-0202 UNCONSUMED.

---

## 5. Per-Task summary

15 task-landing commits ahead of master tip `3b17f43` + this Task 8 atomic-landing commit = 16 commits total on the IMPL branch.

- **Pre-Task 0 — PROGRESS.md preamble + 12-precondition verification** (`e2334fb`). 12 cold-start preconditions verified verbatim; D-RL8..D-RL18 + D-P1..D-P3 PLAN-author resolutions reproduced in preamble; ADR tail at 0201 (ADR-0199 §Context anchored at parent SPEC per ADR-0044); 3 representative fuzzers spot-checked clean; pre-existing differential 35/35 PASS baseline.
- **Task 1 — `descriptors.go` 5 remaining actions + AMEND-11 + `metadata` accessor (D-RL8)** (`b5df160`). 5 NEW action handlers + the FRAMEWORK delta `RouteMetadata()` accessor (ADR-0165 set-once extension template — chain-field + setter + chain-accessor + decoderCB accessor + HCM-dispatch seed at H1 + H2) + the `FactoryCtx.NodeServiceCluster` field for `source_cluster`. `actionUnsupportedAt241()` DELETED. **D-RL8 byte-confirmation: RESOLVED CLEANLY — ADR-0202 UNCONSUMED.**
- **Task-1 follow-up — review-fix bundle (3 notes)** (`6fd3c5a`). Docstring tightened + 2 new test cases. ZERO functional change.
- **Task 2 — `stage` multi-stage bucketing path (§4.4)** (`9d54a31`). `bucketRateLimitsByStage` parse-time `[11]bucket` + per-request filter-stage selection; first-stage OVER_LIMIT short-circuits subsequent stages; per-policy `stage > 10` PARSE-REJECT extension.
- **Task-2 follow-up — `compiledConfig.stage` docstring softening** (`4355e4c`). Docstring softened to match runtime predicate. ZERO functional change.
- **Task 3 — `compiled_perroute.go` `RateLimitPerRoute` 10th-canonical TPFC compile [ADR-0199 FULL, ADR-0125 §(xv) 9→10]** (`edc4072`). NEW `compiled_perroute.go` + `compiled_perroute_test.go`: `compiledPerRoute` 4-field opaque + `validatePerRouteRateLimit` single-chokepoint validator (recursively invokes `ValidateRouteRateLimits` against embedded `rate_limits[]`) + `compilePerRouteForRequest` request-time projection. TypeURL registered in `internal/filter/hcm/` per ADR-0073. **ADR-0199 §Decision + §Consequences FULL body anchored** + **ADR-0125 §(xv) AMENDMENT 9 → 10 paragraph LANDED INSIDE ADR-0199 body**.
- **Task-3 follow-up — ADR-0199 LoC fix + reflect-based struct-shape pin** (`653190d`). Dropped inaccurate LoC numbers; upgraded `TestCompiledPerRoute_StructShape` to reflect-based pin. ZERO functional change.
- **Task 4 — Axis-B `vh_rate_limits` cross-tier composition table + legacy force-include (§4.3 / AMEND-5)** (`737b3b1`). The §4.3 decision-table fully wired: per-route inclusion enum OVERRIDE/INCLUDE/IGNORE (DEFAULT=OVERRIDE); legacy `RouteAction.include_vh_rate_limits=true` force-include arm honored ONLY when OVERRIDE; IGNORE supersedes the legacy bool; Axis-A early-return on non-empty per-route `rate_limits[]` per AMEND-4.
- **Task 5 — `encode.go` + `headers.go` X-RateLimit DRAFT_VERSION_03 (D-RL9) [ADR-0197 IN-PLACE amendment]** (`1e8851f`). NEW `encode.go` + `headers.go` + `headers_test.go`: 3-header emission (`x-ratelimit-limit/-remaining/-reset`); MIN-status across multi-descriptor responses; quota-policy suffix `;w=<sec>[;name="<n>"]`; Unit→seconds (SECOND=1..YEAR=31536000); byte-pinned against captured upstream headers. **D-RL9 byte-confirmation: RESOLVED CLEANLY — ADR-0202 UNCONSUMED.** **ADR-0197 IN-PLACE §Decision amendment anchored** per ADR-0052.
- **Task-5 stage-close** (`8d234ba`). PROGRESS.md SHA-fill (`TBD-24.2-T5 → 1e8851f`).
- **Task-5 follow-up I-1 — CRITICAL AMEND-8 wire-order fix** (`ce8ca49`). X-RateLimit injection moved INLINE at `applyOverLimit` between slot [a] `x-envoy-ratelimited` and slot [b] RLS `response_headers_to_add` per parent SPEC §4.7 line 214. 1 new test asserting wire order.
- **Task 6 — `0032-http-ratelimit` fixture extensions — (f) `vh_inclusion` + (g) `x_ratelimit_headers` + (d) extended to all 10 actions** (`2a438f4`). Scenario (f) 3 sub-scenarios f1/f2/f3 (OVERRIDE/INCLUDE/IGNORE); scenario (g) cross-side byte-exact X-RateLimit emission; scenario (d) extended from 5 to 10 actions per Task 1. NO new fixture dir; fixture dir count STAYS at 35.
- **Task-6 stage-close** (`3ea9324`). PROGRESS.md SHA-fill (`TBD-24.2-T6 → 2a438f4`).
- **Task-6 follow-up — driver.go doc-drift cleanup** (`45a00d9`). Stale 2-vhost-design comments + stale arithmetic block removed. ZERO functional change.
- **Task 7 — `FuzzRateLimitConfigParse` corpus extension (no new fuzzer)** (`63d506f`). +15 seeds covering: 5 remaining §4 action arms (Seeds 32-36); 6 `RateLimitPerRoute` seeds (Seeds 37-42: 3 `vh_rate_limits` enum arms + 3 `override_option` enum arms + 1 per-route `domain` override); per-policy stage boundary arms (Seeds 43/44: `stage=5` valid + `stage=11` PARSE-REJECT); X-RateLimit toggle combination arm (Seed 45); legacy `RouteAction.include_vh_rate_limits=true` byte shape (Seed 46). Fuzz body 3rd surface added (`validatePerRouteRateLimit` + `compilePerRouteForRequest` via `RateLimitPerRoute` proto Unmarshal). 46-seed corpus clean + 30s clean ~1.78M execs. **Project fuzzer count UNCHANGED at 33** (D-RL16 corpus-only extension).
- **Task 8 — Atomic landing (THIS commit).** Task 7 fuzz_test.go header doc-drift cleanup (3 lines: `45 total → 46 total` / `Seeds 32-45 → Seeds 32-46` / `~14 additional → 15 additional`); 6 phase-done gates verified GREEN with documented flake re-runs; BEHAVIOR_CONTRACT.md 24.2 completion bundle landed atomically (subsection extension + per-route canonical-patterns caption + table extension §(xiv) + §(xv) + cross-reference paragraph + response-header allow-list 3-row extension + stat-name `domain`-qualifier paragraph); STATE.md re-advanced to `awaiting next planning (wasm)` with next-skill `superpowers:brainstorming`; ROADMAP row 24.2 flipped `planned → done` AND parent row 24 flipped `in-progress → done` IN THE SAME COMMIT per D-RL18 + the 18/19/22 ROLLUP precedent; REVIEW.md authored; PROGRESS Task 8 entry appended.

---

## 6. Known limitations + future-work register (24.2 scope)

The phase-24.2 IMPL lands with the following recognized limitations — NONE blocking 24.2 phase-done.

**3 phase-24.2-emergent limitations (all PLAN-anticipated forward-pointers to future phases):**

1. **`destination_cluster` framework-limited disposition (24.1-landed; reaffirmed at 24.2).** The `actionDestinationCluster` resolves at match-time only (NOT at request-time). Upstream Envoy v1.37.2's `Router::ClusterDiscoveryStatus`-aware request-time cluster resolution is NOT implemented at envoy-go MVP. Closes when CDS+xDS dynamic cluster phases land.
2. **`RateLimitPerRoute.override_option` accepted-but-IGNORED INERT per AMEND-4 (NOT a departure).** The upstream proto field is `[#not-implemented-hide:]`-tagged; envoy-go honors the upstream-parity decision — no PARSE-REJECT, no request-time consultation. NOT a departure record (no divergence). Documented at BEHAVIOR_CONTRACT.md 24.2 completion bundle for operator clarity.
3. **Header-matcher + QueryParameter-matcher evaluation NOT pre-compiled (24.1-landed; reaffirmed at 24.2).** Per-request `regexp.Compile` cost is present in the `header_value_match` SafeRegex arm + the analogous `query_parameter_value_match` arm. A future profiling-driven optimization MAY extract pre-compile paths.

**Cross-phase carry-forwards from earlier phases that this phase did NOT touch:** unchanged (per-phase deferral lists from phase-17..23 + 24.1 carry forward).

**24.2 closes ALL 6 phase-24.1-emergent forward-pointers** (X-RateLimit DRAFT_VERSION_03 headers + 5 remaining descriptor actions + `RateLimitPerRoute` 10th canonical + ADR-0125 §(xv) AMENDMENT 9 → 10 + `stage` multi-stage bucketing + Axis-B `vh_rate_limits` cross-tier composition — all LANDED at 24.2).

---

## 7. Six-gate phase-done verification

Verbatim from Task 8's verification sweep. All 6 gates GREEN.

**Gate A — `go build ./...`:**
```
$ go build ./... 2>&1
(empty)
---BUILD-EXIT: 0---
```

**Gate B — `go vet ./...` + `golangci-lint run`:**
```
$ go vet ./... 2>&1
(empty)
---VET-EXIT: 0---

$ golangci-lint run 2>&1
(empty)
---LINT-EXIT: 0---
```

**Gate C — `go test -race -count=1 ./...`:** Initial full-repo run flaked one differential fixture (the documented multi-listener `freeTCPPort` flake class per 22.2 REVIEW §7.4):
```
$ go test -race -count=1 ./... 2>&1 | tail -...
--- FAIL: TestDifferential (84.89s)
    --- FAIL: TestDifferential/0018-http-rbac (1.73s)
        runner_test.go:810: subj start: subject ready: EOF
FAIL
FAIL    github.com/esalaine/envoy-go/test/differential  87.049s
```
Isolated re-run cleared the flake:
```
$ go test -race -count=1 -run 'TestDifferential/0018-http-rbac' ./test/differential/
ok      github.com/esalaine/envoy-go/test/differential  3.468s
---EXIT: 0---
```
GREEN with documented flake disposition. The 24.2 surface is fully race-clean; the 0018 flake is a known-flaky multi-listener fixture unrelated to phase 24.2.

**Gate D — `go test -count=1 -timeout 30m ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])'`** (35/35 anchored regex):

First run flaked one fixture (different one — `0020-http-ext-authz-http`; same `freeTCPPort` flake class):
```
$ go test -count=1 -timeout 30m ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])' 2>&1 | tail -5
--- FAIL: TestDifferential (84.58s)
    --- FAIL: TestDifferential/0020-http-ext-authz-http (1.64s)
        runner_test.go:810: subj start: subject ready: EOF
FAIL
FAIL    github.com/esalaine/envoy-go/test/differential  84.668s
```
Isolated re-run cleared the flake:
```
$ go test -count=1 -timeout 5m -run 'TestDifferential/0020-http-ext-authz-http' ./test/differential/
ok      github.com/esalaine/envoy-go/test/differential  2.060s
---EXIT: 0---
```
A second `-v` full Gate-D run was clean — all 35/35 fixtures PASS in 85.4s:
```
$ go test -count=1 -timeout 30m -v ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])' 2>&1 | tail -...
    --- PASS: TestDifferential/0000-tcp-echo (1.78s)
    --- PASS: TestDifferential/0001-tcp-proxy-rr (1.45s)
    ... (35 subtests total — all PASS)
    --- PASS: TestDifferential/0032-http-ratelimit (1.54s)
    --- PASS: TestDifferential/0033-http-ratelimit-boot-reject (1.36s)
PASS
ok      github.com/esalaine/envoy-go/test/differential  85.480s
--- PASS: TestDifferential (85.40s)
---DIFF-EXIT: 0---
```
All 35/35 fixtures PASS in 85.4s. The two EXTENDED 24.2 scenarios on `0032-http-ratelimit` ((f) `vh_inclusion` + (g) `x_ratelimit_headers` + (d) extended to all 10 actions) are GREEN.

**Gate E — `go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/` (seed corpus) + 30s live fuzz run:**
```
$ go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/ 2>&1 | tail -10
ok      github.com/esalaine/envoy-go/internal/filter/http/ratelimit     0.004s
---FUZZ-SEED-EXIT: 0---

$ go test -run 'XXX_NONE' -fuzz 'FuzzRateLimitConfigParse' -fuzztime 30s ./internal/filter/http/ratelimit/ 2>&1 | tail -15
fuzz: elapsed: 0s, gathering baseline coverage: 0/483 completed
fuzz: elapsed: 3s, gathering baseline coverage: 378/483 completed
fuzz: elapsed: 4s, gathering baseline coverage: 483/483 completed, now fuzzing with 32 workers
fuzz: elapsed: 6s, execs: 250139 (83331/sec), new interesting: 4 (total: 487)
fuzz: elapsed: 9s, execs: 542708 (97499/sec), new interesting: 4 (total: 487)
fuzz: elapsed: 12s, execs: 768226 (75189/sec), new interesting: 5 (total: 488)
fuzz: elapsed: 15s, execs: 958767 (63516/sec), new interesting: 6 (total: 489)
fuzz: elapsed: 18s, execs: 1129080 (56753/sec), new interesting: 8 (total: 491)
fuzz: elapsed: 21s, execs: 1280647 (50524/sec), new interesting: 8 (total: 491)
fuzz: elapsed: 24s, execs: 1511758 (77040/sec), new interesting: 9 (total: 492)
fuzz: elapsed: 27s, execs: 1647714 (45324/sec), new interesting: 9 (total: 492)
fuzz: elapsed: 30s, execs: 1784920 (45732/sec), new interesting: 10 (total: 493)
fuzz: elapsed: 31s, execs: 1784920 (0/sec), new interesting: 10 (total: 493)
PASS
ok      github.com/esalaine/envoy-go/internal/filter/http/ratelimit     31.073s
---FUZZ-30S-EXIT: 0---
```
46-seed corpus clean (31 seeds from 24.1 + 15 new at 24.2 per D-RL16); 1,784,920 execs in 30s; 10 new-interesting; 0 panics; 0 crashers. Project fuzzer count = **33** (D-RL16 corpus-only extension; UNCHANGED from 24.1).

**Gate F — h2spec conformance:**
```
$ go test -v -count=1 ./test/conformance/h2spec/ 2>&1 | tail -25
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
--- PASS: TestH2Spec (2.45s)
PASS
ok      github.com/esalaine/envoy-go/test/conformance/h2spec    2.541s
---H2SPEC-EXIT: 0---
```
53/53 PASS at the ADR-0051 v1.32.4 pin. Phase 24.2 touched no H2 codec path; the PASS confirms zero regression.

---

## 8. Parent-rollup status

**Phase 24.2 (SECOND and FINAL sub-phase of the phase-24 split per ADR-0201) is CLOSED at this Task 8 commit AND the parent row 24 family-row is ALSO CLOSED at this commit per the D-RL18 ROLLUP precedent.** Both transitions land in the SAME commit per the 18/19/22 precedent — the commit-message body names BOTH for grep-verifiability. The §9 HTTP-filters family closes to **1 remaining row: `wasm`** post-phase-24 (the `wasm` row is NOT yet a ROADMAP row — awaiting BRAINSTORM authoring per `superpowers:brainstorming`).

The §9 HTTP-filters family now has **17 landed family-rows** (phases 7.1 / 9 / 10 / 11 / 12 / 13 / 14 / 15 / 16 / 17 / 18 / 19 / 20 / 21 / 22 / 23 / 24) + the not-yet-a-ROADMAP-row final `wasm`.

---

## 9. Lessons learned

**Two-axis composition is more subtle than two independent axes.** Phase-24.2's per-route + Axis-B composition (Task 3 + Task 4) lands a 2-axis system: Axis-A is the per-route `RateLimitPerRoute.rate_limits[]` route-additional policies (when non-empty ⇒ EARLY-RETURN); Axis-B is the inclusion-enum-controlled cross-tier composition of the matched Route's `rate_limits[]` + the VirtualHost's `rate_limits[]`. The Axis-A EARLY-RETURN supersedes Axis-B entirely (per AMEND-4); the per-route inclusion enum (OVERRIDE/INCLUDE/IGNORE) governs Axis-B; the legacy `include_vh_rate_limits=true` bool only fires when OVERRIDE (DEFAULT or explicit); IGNORE supersedes the legacy bool. The 24.2 SPEC §4.3 decision-table captures this; the Task 4 IMPL ratified it via the `0032`(f) cross-side scenarios f1/f2/f3. **Lesson:** any 2-axis composition with a precedence relationship needs a TABLE-driven decision spec (not prose-only); the cross-side fixture validates against the upstream's table interpretation.

**Wire-order assertions must be tested AT the wire-write site, not at the encode-hook.** Phase-24.2's Task-5 follow-up I-1 fix surfaced a subtle wire-order regression: the initial Task-5 commit emitted X-RateLimit at the encode hook (`EncodeHeaders`), but for the OVER_LIMIT path the `SendLocalReply` headers are assembled BEFORE the encode hook fires — so the X-RateLimit headers landed AFTER the filter-config `response_headers_to_add` slot [c], not between slot [a] and slot [b] per the upstream §4.7 wire order. The fix moved X-RateLimit injection INLINE at `applyOverLimit` for the OVER_LIMIT path (the encode hook still fires for OK + fail-OPEN). **Lesson:** wire-order tests should observe the wire (the assembled `[]hpack.HeaderField` slice or the rendered HTTP/1.1 header block) AT the `SendLocalReply` (or equivalent) call site — NOT at the encode hook for paths that bypass the encode hook. The 24.2 follow-up added `TestApplyOverLimit_XRateLimit_WireOrder` asserting the slot order at the `SendLocalReply` call.

**The ADR-0165 set-once-by-dispatch extension template extends cleanly to new accessor families.** Phase-24.2's Task 1 needed a `RouteMetadata()` accessor for the `metadata` action's `MetadataSource_ROUTE_ENTRY=1` path. The ADR-0165 plumbing (chain-field + setter + chain-accessor + decoderCB accessor + HCM-dispatch seed at H1 + H2) extended without divergence — the same template that landed `DownstreamRemoteAddr` at ADR-0165 + the 24.1 DELTA-2 `RouteRateLimits()`/`VirtualHostRateLimits()` pair at ADR-0198. **Lesson:** the ADR-0165 template is a generalizable primitive for any "set-once at HCM-dispatch / read-via-DCB-accessor at filter request-time" capability; future filters needing route-level data exposure should follow the template before considering a new framework primitive.

---

## 10. Forward-pointers carried into next phase (wasm — final §9 family-row)

The next-phase inheritance set per the Task 8 STATE.md advance:

**0 phase-24.2-emergent forward-pointers** (the 3 phase-24.2 known-limitations are CROSS-PHASE forward-pointers, not 24.2-specific deferrals — `destination_cluster` waits on CDS+xDS dynamic cluster phases; `override_option` INERT is upstream-parity and never lands; matcher pre-compile is a profiling-driven optimization).

**Cross-phase carry-forwards from earlier phases that this phase did NOT touch:** unchanged.

**STATE.md post-Task-8 disposition:** `active-phase: awaiting next planning (wasm — final §9 family-row; not yet a ROADMAP row)`; `lifecycle-state: phase 24.2 IMPL done; phase-24 family-row CLOSED at parent rollup; awaiting next planning` (SKILL_ROUTING state 0/1 — no in-flight phase has SPEC.md yet for the next §9 row `wasm`); `next-skill: superpowers:brainstorming` (to author BRAINSTORM.md for the `wasm` §9 final family-row + append the `wasm` ROADMAP row per the established §9 family-row addition discipline); `last-commit: TBD-24.2-IMPL-SQUASH` placeholder for post-squash STATE SHA-fill; `next-free ADR: ADR-0202` (D-hypothesis HELD across the entire phase-24 family-row; escape valve UNCONSUMED).

**§9 family closure trail at 24.2 phase-done:** 17 family-rows landed (phases 7.1 / 9 / 10 / 11 / 12 / 13 / 14 / 15 / 16 / 17 / 18 / 19 / 20 / 21 / 22 / 23 / 24) + 1 remaining row `wasm` (NOT yet a ROADMAP row).

---

## 11. Sign-off

Phase 24.2 is **APPROVED for master squash-merge per project memory `feedback_git_worktrees.md`** + ADR-0003 worktree-isolation discipline + ADR-0005 §Decision 4 worktree-merge discipline. All 6 phase-done gates GREEN at this Task 8 HEAD (Gates A/B/E/F first-run clean; Gates C/D each had one documented multi-listener `freeTCPPort` flake which re-ran clean in isolation per the 22.2 REVIEW §7.4 flake class); all 18 parent §15 acceptance UNION items GREEN (24.1 partial items 7/9/15/16 CLOSED + new items 12/14/17/18 LANDED); 3 ADR §Decision-touchpoints cleanly anchored at their per-Task Lands-in-Tasks per ADR-0044 (ADR-0199 FULL + ADR-0197 IN-PLACE amendment + ADR-0125 §(xv) AMENDMENT 9 → 10); **ADR-0125 canonical-per-route roster grows 9 → 10 at 24.2 Task 3** (the FIRST roster growth since phase-22.3 / ADR-0193 — ENDING the phase-23 + 24.1 REUSE-by-absence skip streak); **D-hypothesis HELD across the entire phase-24 family-row** — ADR-0202 UNCONSUMED at 24.2 phase-done (both D-RL8 metadata accessor at Task 1 + D-RL9 X-RateLimit byte-edge at Task 5 RESOLVED CLEANLY; next-free ADR stays at ADR-0202); **D-RL8..D-RL18 + D-P1..D-P3 PLAN-author dispositions** all confirmed; 33 fuzzers (UNCHANGED; D-RL16 corpus-only extension; 31 → 46 seed corpus) + 35 differential fixtures (UNCHANGED; Task 6 extends `0032` in-place per the 24.2 SPEC §16 axis) GREEN; h2spec 53/53 at ADR-0051 v1.32.4 pin; BEHAVIOR_CONTRACT 24.2 completion bundle landed at this Task 8 commit per ADR-0052 atomic landing (subsection extension + per-route canonical-patterns caption + table extension §(xiv) + §(xv) + cross-reference paragraph + response-header allow-list 3-row extension + stat-name `domain`-qualifier paragraph; **NO new departure records** — count STAYS at 18); **ROADMAP row 24.2 flipped `planned → done` AT THIS COMMIT** AND **parent row 24 flipped `in-progress → done` IN THE SAME COMMIT per D-RL18 + the 18/19/22 ROLLUP precedent** (commit-message body names BOTH for grep-verifiability); STATE.md re-advanced to `awaiting next planning (wasm)` with next-skill `superpowers:brainstorming`; **5 IMPL-time follow-ups recorded** (Task-1 review-fix bundle / Task-2 docstring softening / Task-3 LoC fix + reflect-pin / Task-5 follow-up I-1 CRITICAL AMEND-8 wire-order fix / Task-6 driver.go doc-drift — all ZERO algorithmic / behavioral deviation from the PLAN).

The squash-merge + STATE SHA-fill follow-up + push-to-origin are the user's manual steps after this Task 8 commit lands (per the phase-09..23 + 24.1 squash-merge convention + project memory `feedback_push_to_origin.md`).

**Summary stats:** 9 PLAN tasks (Pre-Task 0 + Tasks 1-7 + Task 8 atomic landing) + 5 follow-up commits + 2 stage-close SHA-fill commits = **16 commits on the IMPL branch ahead of master tip `3b17f43`**. **3 ADR landings** (ADR-0199 FULL + ADR-0197 IN-PLACE amendment + ADR-0125 §(xv) AMENDMENT 9 → 10 anchored IN-BODY of ADR-0199). 33 fuzzers (UNCHANGED from 24.1; D-RL16 corpus-only extension). 35 differential fixtures (UNCHANGED from 24.1; Task 6 extends `0032` in-place). 6 phase-done gates GREEN. 18 envoy-go-strict departures (UNCHANGED from 24.1; `override_option` INERT is upstream-parity, NOT a departure). 19 HTTP filters wired (UNCHANGED from 24.1). 114 stats (UNCHANGED from 24.1; per-route `domain` is descriptor-tier per AMEND-1, NOT a stat namespace). **ZERO new framework primitives at 24.2 production code** (the NEW `RouteMetadata()` accessor at Task 1 extends the EXISTING ADR-0165 set-once-by-dispatch template — same shape as 24.1's DELTA-2 + ADR-0165's `DownstreamRemoteAddr`; NOT a new primitive, an extension of an existing one). **ADR-0125 canonical-per-route roster grows 9 → 10** (the FIRST roster growth since phase-22.3 / ADR-0193 — ENDING the phase-23 + 24.1 REUSE-by-absence skip streak per ADR-0125 §(xv) + ADR-0199). **D-hypothesis HELD across the entire phase-24 family-row — ADR-0202 UNCONSUMED at 24.2 phase-done.**

**End of phase 24.2 review AND end of the phase-24 family-row review. The next session is the phase-done squash-merge + STATE SHA-fill + push-to-origin follow-up per project memory `feedback_git_worktrees.md` + `feedback_push_to_origin.md`; the session AFTER that is the `wasm` §9 final family-row BRAINSTORM authoring per `superpowers:brainstorming`.**
