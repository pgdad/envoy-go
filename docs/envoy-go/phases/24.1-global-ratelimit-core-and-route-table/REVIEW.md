# Phase 24.1 — HTTP filter `envoy.filters.http.ratelimit` (CORE decision path + route-table exposure) — Review

**Phase id:** `24.1` (FIRST sub-phase of the phase-24 PLAN-time ADR-0045 split per ADR-0201; parent row `24 http-filter-global-ratelimit` STAYS `in-progress`, closes at 24.2 phase-done per the 18/19/22 rollup precedent; sibling `24.2-global-ratelimit-perroute-and-headers` STAYS `planned` and opens at this commit; **D-hypothesis HELD — ADR-0202 UNCONSUMED at 24.1 phase-done**, next-free stays `ADR-0202`.)
**Slug:** `24.1-global-ratelimit-core-and-route-table`
**Branch under review:** `phase-24.1-global-ratelimit-core-and-route-table-impl`
**Range:** branch tip is this Task 12 commit (REVIEW.md + STATE.md re-advance + ROADMAP row 24.1 flip `in-progress → done` + BEHAVIOR_CONTRACT.md partial bundle + PROGRESS Task 12 entry; **15** task-landing commits on the IMPL branch — Pre-Task 0 + Tasks 1-11 + Task-7 follow-up CRITICAL `ContinueDecoding`-after-`SendLocalReply` wake-up fix + Task-10 doc-fix follow-up + this Task 12 atomic-landing commit, ahead of master tip `e8a8881`). The last-commit SHA-fill on STATE.md is deferred to the post-`wt-merge` follow-up per the phase-09..23 IMPL-stage close pattern (placeholder `TBD-24.1-IMPL-SQUASH` in STATE.md).
**Parent ROADMAP row:** row `24.1 global-ratelimit-core-and-route-table` flipped `in-progress → done` at this Task 12 commit (date `2026-05-23`). Per-cell IMPL-done annotation appended documenting the 15-commit IMPL landing + 3 NEW ADR landings + 6-gate verbatim outputs + SPEC §7 acceptance summary + notable IMPL-time findings. Parent row `24 http-filter-global-ratelimit` STAYS `in-progress`; row `24.2 global-ratelimit-perroute-and-headers` UNCHANGED `planned`.
**PLAN tip SHA:** `1350e69` (Squash merge phase-24.1-global-ratelimit-core-and-route-table-plan [ADR-0197 core, ADR-0198, ADR-0200]). **SPEC tip:** sub-phase SPEC slice at `24.1-...-SPEC.md` was authored at the same PLAN-stage commit per the in-PLAN-stage SPEC carve-out per ADR-0045. **Parent SPEC tip SHA:** see DECISIONS.md ADR-0201 §Context for the PLAN-time split lineage.
**Reviewer method:** Inline authoring by the implementing session per the PLAN Task 12 direction. Inputs: SPEC §7 9-item acceptance checklist (the 24.1 partial subset of the parent §15 UNION) + PLAN's 12-task structure + the branch diff (14 commits + this REVIEW commit) + PROGRESS.md per-task entries (Pre-Task 0 + Tasks 1-11 + 2 follow-up entries) + DECISIONS.md ADR-0197[core] + ADR-0198 + ADR-0200 §Decision + §Consequences full bodies + BEHAVIOR_CONTRACT.md partial bundle (this Task 12) + phase-23 REVIEW.md structural template precedent.
**Links:** [PLAN.md](./PLAN.md) · [SPEC.md](./SPEC.md) · parent [SPEC.md](../24-http-filter-global-ratelimit/SPEC.md) · parent [BRAINSTORM.md](../24-http-filter-global-ratelimit/BRAINSTORM.md) · [PROGRESS.md](./PROGRESS.md).
**Six-gate state at HEAD:** all GREEN per Task 12's verification sweep — outputs reproduced verbatim in §7 below.

This review covers the full phase-24.1 surface: the NEW `internal/filter/http/ratelimit/` package (8 production files — `ratelimit.go` + `compiled_config.go` + `descriptors.go` + `decode_headers.go` + `dispositions.go` + `stats.go` + `doc.go` + `fuzz_test.go` — plus 7 unit-test files); the NEW `internal/grpcclient/ratelimit_client.go` typed wrapper (DELTA-1; THIRD ADR-0158 unary `AuthClient`-clone after `AuthClient` + `ProcessorClient`); the FRAMEWORK CHANGE at `internal/filter/hcm/route.go` + `internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go` + `internal/filter/hcm/config.go` adding the DELTA-2 route-table `rate_limits` parse/retain/seed path + the `chain.SetRouteRateLimits()`/`SetVirtualHostRateLimits()` set-once-by-dispatch primitives + the `DecoderFilterCallbacks.RouteRateLimits()`/`VirtualHostRateLimits()` accessor pair (mirrors ADR-0165's `DownstreamRemoteAddr` shape); the boot-registration at `cmd/envoy-go/main.go` (alphabetical between `oauth2` and `rbac`; **19 HTTP filters**); the NEW differential fixtures `0032-http-ratelimit` (cross-side; scenarios a/b/c/d-core/e/h) + `0033-http-ratelimit-boot-reject` (domain-empty); the 33rd fuzzer `FuzzRateLimitConfigParse` (seed corpus clean; 30s clean ~1.47M execs); the shared fake `RateLimitService` at `test/helpers/ratelimitgrpc/` + `HTTPGlobalRateLimitGRPC` BackendKind=24 enum (proto-number-faithful per AMEND-6); the BEHAVIOR_CONTRACT.md partial bundle (this Task 12); the 3 NEW ADR landings (ADR-0197[core] + ADR-0198 + ADR-0200).

This REVIEW closes phase-24.1's IMPL lifecycle (state 5 → 6). It is the final task before squash-merge to master. The parent row `24` rollup to `done` happens at the 24.2 phase-done commit per the 18/19/22 split-rollup precedent.

---

## 1. Summary

**APPROVED.** All six phase-done gates are GREEN at HEAD per Task 12's verification sweep. The implementation faithfully realizes the SPEC across all 12 PLAN tasks (Pre-Task 0 + Tasks 1-11 + Task 12 atomic landing) plus 2 follow-up commits (a CRITICAL Task-7 follow-up + a doc-only Task-10 follow-up). Phase 24.1 is the FIRST sub-phase of the phase-24 PLAN-time ADR-0045 split per ADR-0201; the parent row `24 http-filter-global-ratelimit` STAYS `in-progress` and closes at 24.2 phase-done per the 18/19/22 rollup precedent.

**Zero IMPL-time deviations from the PLAN at the algorithmic / behavioral level.** Three small artifact deltas are recorded at §3 below: (1) the Task-7 follow-up CRITICAL `ContinueDecoding`-after-`SendLocalReply` wake-up fix (the async-dispatch parked-decode-goroutine wake-up was missing on OVER_LIMIT + fail-closed paths; root-cause fix mirroring the ext_authz deny-path + phase-09 fault filter precedent); (2) the Task-10 doc-fix follow-up (stale burst/restart references swept across PROGRESS.md, fixture docs, and YAML cross-refs — `setupScripts→setupRLS` renames + expectations.yaml dual-form cross-ref); (3) the X-RateLimit encode-side body STUBBED at 24.1 per D-RL7 (PLAN-anticipated; the encode-side body lands at 24.2 — not a deviation, but worth surfacing for the reviewer-equivalent confirmation).

**Three NEW ADR landings (all anticipated per PLAN):**
- **ADR-0197[core]** §Decision + §Consequences (24.1 slice) — package shape + dispatch + dispositions + boot-registration + 19 HTTP filters. The X-RateLimit + encode-side slice lands at 24.2 (ADR-0197 will be amended in-place at 24.2's encode-side Task per the in-place edit discipline of ADR-0052). Landed at Task 7 (`5c665fa`).
- **ADR-0198** §Decision + §Consequences FULL — DELTA-2 HCM route-table `rate_limits` exposure framework capability + the `RouteRateLimits()`/`VirtualHostRateLimits()` chain-seeded accessor pair + the RAW-PROTO seed type byte-confirmation (D-RL1 resolved). Landed at Task 5 (`7b91ef7`).
- **ADR-0200** §Decision + §Consequences FULL — the FULL §5 PARSE-REJECT roster (the RATIFIED-from-PGV/config arms + the 3 envoy-go-strict departures `disable_key`/`extension`/`dynamic_metadata`). Landed at Task 3 (`acbc3d1`).

**ADR-0202 UNCONSUMED at 24.1 phase-done — D-hypothesis HELD.** The D-RL1 byte-confirmation (parent SPEC §12 item 1 — HIGHEST RISK — the DELTA-2 chain-seed type + accessor return-type) RESOLVED at Task 5 as **RAW-PROTO SEED CONFIRMED**: the raw `[]*routev3.RateLimit` shape fit the existing ADR-0165 `DownstreamRemoteAddr` set-once primitive WITHOUT divergence; no pre-compiled carrier needed; no non-proto type needed. The escape valve did NOT fire. Next-free ADR stays at `ADR-0202`.

**Next-free ADR: ADR-0202** (ADR-0197 + ADR-0198 + ADR-0200 + ADR-0201 [the PLAN-time split ADR] all consumed; ADR-0199 [X-RateLimit + `RateLimitPerRoute` 10th-canonical] stays anchored at the parent SPEC commit as §Context-only, §Decision + §Consequences body lands at 24.2 IMPL).

**SECOND CONSECUTIVE §9 row to DEFER the ADR-0125 roster extension at IMPL final Task** (after phase 23's REUSE-by-absence skip — phase 23 + phase 24.1; the streak ENDS at 24.2 IMPL final Task when the `RateLimitPerRoute` 10th-canonical AMENDMENT body lands per ADR-0125 §(xv) — anchored at the parent SPEC commit per the 22.1→22.3 anticipation→landing precedent).

---

## 2. SPEC §7 acceptance verification (24.1 subset of the parent §15 UNION)

Per the 24.1 SPEC §7 (the 9-item partial subset that maps to the 24.1 slice — items 12 [X-RateLimit] + 14 [`RateLimitPerRoute`] + the remaining slices of items 7/9/15/16 land at 24.2; the parent row-24 `done` flip + the full §15 UNION verification happen at 24.2 phase-done [item 17]).

### A. Six gates (parent §15 items 1-6, scoped to 24.1)

- [x] **Item 1 — Gate A build clean.** **GREEN.** `go build ./...` exits 0 at HEAD. See §7 verbatim output.
- [x] **Item 2 — Gate B vet + lint clean.** **GREEN.** `go vet ./...` exits 0; `golangci-lint run` exits 0; no new lint suppressions across the phase-24.1 surface. See §7 verbatim output.
- [x] **Item 3 — Gate C race clean.** **GREEN.** `go test -race -count=1 ./...` clean for ALL 24.1-packages on the first run (`internal/filter/http/ratelimit` ok 1.083s; `internal/grpcclient` ok 1.191s; `internal/filter/hcm` ok 1.119s; `internal/filter/hcm/h2` ok 3.561s; `test/helpers/ratelimitgrpc` ok 1.061s); 63 packages `ok` repo-wide; 0 FAIL lines on 24.1 surface. The ONE FAIL line is a documented flake at the `-race`-on-differential bring-up (`0025-http-adaptive-concurrency` "subject ready: EOF" — multi-listener bring-up class; per Pre-Task 0 + Task 5 + Task 7 evidence); re-ran clean in isolation (`ok ... 6.045s`). See §7 verbatim output.
- [x] **Item 4 — Gate D differential clean.** **GREEN.** First-run `go test -count=1 -timeout 30m ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])'` exits 0 in 102.4s — all 35/35 fixtures PASS (0000-0033 inclusive incl. NEW `0032-http-ratelimit` cross-side + NEW `0033-http-ratelimit-boot-reject`). A second `-v` confirmation run flaked 2 fixtures (`0020-http-ext-authz-http` + `0023-http-ext-proc-body` — both documented multi-listener "address already in use" / EOF flakes per the well-known flake class); both PASS in isolation re-runs. The two NEW 24.1 fixtures (`0032` + `0033`) PASSED on BOTH runs. See §7 verbatim output.
- [x] **Item 5 — Gate E fuzz clean.** **GREEN.** `FuzzRateLimitConfigParse` seed corpus clean (31 seeds; all PASS); 30s live-fuzz clean: ~1.47M execs at ~50k/sec; 0 panics; 0 crashers. Total fuzzer count = **33** (`find . -name 'fuzz_test.go' -exec grep -h '^func Fuzz' {} \; | sort -u | wc -l` = 33). See §7 verbatim output.
- [x] **Item 6 — Gate F h2spec clean.** **GREEN.** `go test -v -count=1 ./test/conformance/h2spec/` reports `53 tests, 53 passed, 0 skipped, 0 failed` at the ADR-0051 v1.32.4 pin. PASS at 2.97s. See §7 verbatim output.

### B. Two-directory differential fixture coverage (item 7 — partial; f/g + d-extension defer to 24.2)

- [x] **Item 7 — Two-directory differential per §7 (24.1 scenarios).** **GREEN.** `0032-http-ratelimit` (6 scenarios at 24.1: (a) parse_ok / (b) ok_admit cross-side byte-exact / (c) over_limit_reject cross-side byte-exact / (d-core) descriptor_engine 5-core-actions cross-side byte-exact / (e) error_fail_open / (h) stat_surface; **scenarios (f) vh_inclusion + (g) x_ratelimit_headers DEFER to 24.2**; **scenario (d) EXTENSION with the remaining 5 actions DEFER to 24.2**) + `0033-http-ratelimit-boot-reject` (`domain`-empty shared boot-reject substring). Fixture dir count 33 → 35 confirmed. Subject-side assertions in `StatsAsserter` per project memory `reference_differential_asserter_dispatch.md` (NOT `SubjectAsserter` — the cross-side runner path does NOT call `SubjectAsserter`; liveness proven via deliberate-break test at Task 10 per the phase-23 lesson). Evidence: Task 10 + Task 11 PROGRESS entries + Gate D GREEN at 35/35.

### C. Cluster-scoped stat-surface verification (item 8)

- [x] **Item 8 — Cluster-scoped 4-counter stat surface byte-exact.** **GREEN.** Per ADR-0197[core] + AMEND-1 + parent SPEC §11 D2: all 4 stat names anchored in `stats.go` as package-level `const` declarations (D5 compile-time guard per PLAN) + table-driven assertion in `stats_test.go`: `ok` (counter) + `error` (counter) + `over_limit` (counter) + `failure_mode_allowed` (counter). All registered under `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>` **CLUSTER-rooted** prefix template (FIRST cluster-scoped cross-namespace filter-stats surface; LANDS the pattern that ext_authz's `charge_cluster_response_stats` DEFERRED per AMEND-10). Stat surface 110 → 114 names confirmed at BEHAVIOR_CONTRACT.md stat-table extension at this Task 12 commit. NO gauges (COUNTER-only per AMEND-1). Per-route stats SHARED with listener-level at 24.1 (vacuously — `RateLimitPerRoute.domain` override lands at 24.2). Evidence: Task 4 PROGRESS entry + this Task 12 PROGRESS entry + `grep -cE "departure count 17 → 18" docs/envoy-go/BEHAVIOR_CONTRACT.md` non-zero match.

### D. Descriptor-engine fidelity for 5 CORE actions + empty-action-drop (item 9 — partial; remaining 5 actions defer to 24.2)

- [x] **Item 9 — Descriptor-engine fidelity (5 of 10 actions at 24.1).** **GREEN.** All 5 CORE actions landed at Task 6 `descriptors.go`: `actionGenericKey` + `actionRequestHeaders` + `actionRemoteAddress` + `actionDestinationCluster` + `actionHeaderValueMatch` (with the per-request HeaderMatcher evaluation supporting Exact/Prefix/Suffix/Contains/SafeRegex; `Custom` arm UNSUPPORTED-AT-24.1 — falls through false). The remaining 5 actions (`source_cluster`, `masked_remote_address`, `metadata`, `query_parameters`, `query_parameter_value_match`) dispatch to `actionUnsupportedAt241()` which returns `(nil, false, true)` (drop the entire descriptor) — structurally equivalent to "the action is not understood; drop the descriptor". The empty-action-drop discipline per `router_ratelimit.cc:21-39` is honored at the `buildDescriptorForPolicy` level. Test coverage: `descriptors_test.go` exercises all 5 CORE actions + 4 empty-action-drop arms + the per-request HeaderMatcher cases. Evidence: Task 6 PROGRESS entry + Gate C race-clean on `internal/filter/http/ratelimit`. (Item 9 PARTIAL — the remaining 5 actions land at 24.2.)

### E. PARSE-REJECT roster (item 10 — FULL at 24.1)

- [x] **Item 10 — FULL §5 PARSE-REJECT roster byte-stable.** **GREEN.** Per parent SPEC §5.1 + §5.2 + ADR-0200: the RATIFIED-from-PGV/config arms (`domain` empty; `rate_limit_service` missing; `stage > 10`; bad `request_type`; >10 response headers; etc.) + the 3 envoy-go-strict arms (`disable_key` non-empty; `extension` action; `dynamic_metadata` action) — all with byte-stable error wording per ADR-0080 + table-driven coverage at `compiled_config_test.go`. Single-chokepoint validation via `ratelimit.ValidateRouteRateLimits` invoked from HCM `config.go` against each Route's + each VirtualHost's `rate_limits` per ADR-0110. Boot-reject fixture `0033-http-ratelimit-boot-reject` (`domain`-empty) confirms the boot-time arm at the differential gate. Evidence: Task 3 PROGRESS entry + Task 11 PROGRESS entry + Task 12 PROGRESS §7 closure.

### F. Disposition + reply byte-shape (item 11)

- [x] **Item 11 — OK/OVER_LIMIT/error dispositions + reply byte-shape.** **GREEN.** Per parent SPEC §4.7 + AMEND-8 + ADR-0197[core]:
  - OK: `ok` counter Inc; `ContinueDecoding` (no SendLocalReply); pinned at `TestDispositions_OK_*` in `dispositions_test.go`.
  - OVER_LIMIT: `over_limit` counter Inc; `SendLocalReply(cc.rateLimitedStatus, string(resp.RawBody), headers)` with AMEND-8 header order `[x-envoy-ratelimited: true (unless suppressed)] → [RLS response_headers_to_add] → [config response_headers_to_add]`; status from `cc.rateLimitedStatus` (default 429; <400 clamps to 429); `request_rate_limited` rc-details ABSENT-BY-API at 24.1 (3-arg `SendLocalReply` does not surface rc-details; constant pinned for forward-24.2 use); `ContinueDecoding` AFTER `SendLocalReply` to wake the parked dispatch goroutine. Pinned at `TestDispositions_OverLimit_429_ByteShape` + `TestDispositions_OverLimit_DisableXEnvoyRateLimitedHeader_Header_Omitted`.
  - error: `error` counter ALWAYS Inc; fork on `failureModeDeny`: fail-OPEN (default) `failure_mode_allowed` Inc + `ContinueDecoding`; fail-CLOSED `SendLocalReply(cc.statusOnError, "", nil)` (empty body + nil headers — nullptr-mutate shape) + `ContinueDecoding`. Default `cc.statusOnError = 500`; <400 clamps to 500. `rate_limiter_error` rc-details ABSENT-BY-API. Pinned at `TestDispositions_Error_FailOpen_NoReject` + `TestDispositions_Error_FailClosed_500_NullptrMutate`. 
  - **Task-7 follow-up CRITICAL fix:** the `ContinueDecoding()` call AFTER `SendLocalReply` on the OVER_LIMIT + fail-closed paths was missing; the async goroutine's `SendLocalReply` alone sets `c.localReplyDone` but does NOT unblock the parked dispatch goroutine in `parkDecode` (waiting on `decodeResumeCh`). Mirrors ext_authz deny-path + phase-09 fault filter precedent. 2 new tests added (the wake-up invariant at `TestDispositions_OverLimit_ContinueDecoding_AfterSendLocalReply` + the fail-closed analogue). Evidence: Task 7 + Task 7-follow-up PROGRESS entries.

### G. DELTA-1 + DELTA-2 + 19 HTTP filters (item 13)

- [x] **Item 13 — DELTA-1 + DELTA-2 + 19 HTTP filters wired.** **GREEN.** DELTA-1 `internal/grpcclient/ratelimit_client.go` `RateLimitClient` (THIRD ADR-0158 unary `AuthClient`-clone wrapper; composes the existing generic `Dialer` per ADR-0158; `envoy.service.ratelimit.v3` stubs vendored in go-control-plane v1.32.4) — landed at Task 2 with `internal/grpcclient/ratelimit_client_test.go` coverage. DELTA-2 the HCM route-table `rate_limits` exposure — landed at Task 5 via `internal/filter/hcm/route.go` (parse + retain `rateLimits` + `vhostRateLimits` on `routeEntry` + table) + `internal/filter/hcm/config.go` (validate via `ratelimit.ValidateRouteRateLimits` at HCM-build time) + `internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go` (seed via `chain.SetRouteRateLimits()` + `chain.SetVirtualHostRateLimits()` at H1 + H2 dispatch sites) + `internal/filter/http/callbacks.go` (the `RouteRateLimits()` + `VirtualHostRateLimits()` accessor pair on `DecoderFilterCallbacks`) + `internal/filter/http/chain.go` (the set-once-by-dispatch / read-via-accessor primitive mirroring ADR-0165). 19 HTTP filters wired confirmed via `cmd/envoy-go/main.go` boot-registration alphabetical between `oauth2` and `rbac` (the `ratelimit` registration call). Evidence: `grep -c 'httpReg.Register(' cmd/envoy-go/main.go` = 19 (verified at this Task 12 commit per Task 7 PROGRESS entry).

### H. ADR landings (item 15 — partial; ADR-0199 + ADR-0197 [headers slice] defer to 24.2)

- [x] **Item 15 — ADR landings (24.1 partial subset).** **GREEN.** Per the 24.1 PLAN's per-Task Lands-in-Tasks per ADR-0044: ADR-0197[core] §Decision + §Consequences (24.1 slice) landed at Task 7 (`5c665fa`); ADR-0198 §Decision + §Consequences FULL landed at Task 5 (`7b91ef7`); ADR-0200 §Decision + §Consequences FULL landed at Task 3 (`acbc3d1`). ZERO IN-PLACE §Decision AMENDMENTs at 24.1 (the ADR-0197 in-place amendment for the X-RateLimit + encode-side slice lands at 24.2). ZERO ADR-0125 amendments at 24.1 (the 9 → 10 amendment body lands at 24.2 with `RateLimitPerRoute`; the §(xv) AMENDMENT-anticipation paragraph is anchored at the parent SPEC commit). All 3 ADR bodies non-empty with §Decision + §Consequences fully anchored. **D-hypothesis HELD: ADR-0202 UNCONSUMED at 24.1 phase-done.** Evidence: `grep -cE '^## ADR-0197\b'`, `'^## ADR-0198\b'`, `'^## ADR-0200\b'`, `'^## ADR-0201\b'` each return 1; `'^## ADR-0202\b'` returns 0. (Item 15 PARTIAL — ADR-0199 + ADR-0197 in-place amendment for headers slice land at 24.2.)

### I. BEHAVIOR_CONTRACT.md departure records (item 16 — partial; the per-route + X-RateLimit allow-list parts defer to 24.2)

- [x] **Item 16 — BEHAVIOR_CONTRACT.md partial bundle landed.** **GREEN.** Atomic landing at this Task 12 commit per ADR-0052: (1) NEW `### envoy.filters.http.ratelimit` subsection (CORE parts: decode-side request lifecycle + 5-CORE-action descriptor engine table + DELTA-2 route-table exposure + cluster-scoped 4-counter stat surface + X-RateLimit STUBBED forward-pointer to 24.2 + cross-references to ADRs/AMENDs); (2) 3 envoy-go-strict departure records `disable_key` PARSE-REJECT (15 → 16) + `extension` action PARSE-REJECT (16 → 17) + `dynamic_metadata` action PARSE-REJECT (17 → 18); (3) stat-name mapping 110 → 114 table extension (NEW `**ratelimit filter — 4 names**` block + the count-extension paragraph); (4) per-route canonical-patterns cross-reference caption update + phase-24.1 cross-reference paragraph anchoring the ADR-0125 §(xv) 10th-canonical AMENDMENT-anticipation per the 22.1→22.3 anticipation→landing precedent. Evidence: this Task 12 commit grep verifications. **Departure count 15 → 18.** (Item 16 PARTIAL — the per-route `RateLimitPerRoute` allow-list + X-RateLimit header allow-list extensions land at 24.2 with the encode-side body + the 10th-canonical AMENDMENT.)

**Summary:** all 9 of the 24.1 SPEC §7 acceptance items GREEN; 0 BLOCKED; 0 GREEN-WITH-NOTED-DEVIATION; 4 explicit PARTIAL annotations (items 7, 9, 15, 16 — all PLAN-anticipated 24.1/24.2 splits per the SPEC §16 axis). The remaining 24.2 acceptance items (12 X-RateLimit headers + 14 `RateLimitPerRoute` 10th-canonical + the remaining slices of 7/9/15/16 + 17 parent §15 UNION verification with parent row 24 rollup) land at 24.2 phase-done.

---

## 3. IMPL-time artifact deltas (NOT algorithmic deviations from the PLAN)

Three small artifact deltas occurred during IMPL. NONE is an algorithmic / behavioral deviation from the PLAN; all are recorded here for completeness per the Task 12 instruction.

### Delta 1 — Task-7 follow-up CRITICAL `ContinueDecoding`-after-`SendLocalReply` wake-up fix

**Planned:** Task 7 dispatches the async `ShouldRateLimit` call and, on OVER_LIMIT / fail-closed responses, calls `SendLocalReply` from the async goroutine to reject the request.

**What happened (Task-7 follow-up):** A reviewer pass surfaced that `SendLocalReply` alone (called from the async goroutine — outside the dispatch goroutine) sets `c.localReplyDone` but does NOT unblock the parked dispatch goroutine in `parkDecode` (which is waiting on `decodeResumeCh` — `chain.go:316-325`). Without `ContinueDecoding()` after `SendLocalReply`, the dispatch goroutine never wakes; the request hangs until OnDestroy cancels (timeout).

**Fix:** Added an explicit `f.dcb.ContinueDecoding()` call AFTER `f.dcb.SendLocalReply(...)` on BOTH the OVER_LIMIT path (`applyOverLimit`) AND the fail-closed path (`applyError`). 2 new tests added: `TestDispositions_OverLimit_ContinueDecoding_AfterSendLocalReply` + the fail-closed analogue, both asserting `ContinueDecoding` call count = 1. Mirrors the ext_authz deny-path precedent (`extauthz.go:1097-1111` + `extauthz.go:1146-1156`) + the phase-09 fault filter `SendLocalReply+ContinueDecoding` pattern (`fault.go:299-324`). Also deduplicated the ADR-0197 cross-references in the Task-7 commit message body.

**Consequence:** The OVER_LIMIT + fail-closed wire shapes are now correct end-to-end. NO change to the algorithmic / behavioral contract of the PLAN — the dispatch / disposition / reply byte-shape spec is identical; the fix is a framework-mechanics wake-up that the initial Task-7 commit missed. The fix landed in commit `d73226a`.

### Delta 2 — Task-10 doc-fix follow-up (stale burst/restart references)

**Planned:** Task 10 lands the differential fixture `0032-http-ratelimit` (scenarios a/b/c/d-core/e/h).

**What happened (Task-10 follow-up):** A reviewer pass on Task 10's PROGRESS entry + the fixture's `README.md` + the fixture's `expectations.yaml` cross-references surfaced stale text from an earlier draft phrasing (where the ratelimit scenarios were conflated with local_ratelimit token-bucket "burst" / "restart" mechanics). Global ratelimit has NO burst / no restart — the upstream RLS service owns all bucketing logic.

**Fix:** Docs-only sweep — removed stale `burst` / `restart` references across PROGRESS.md + fixture `README.md` files; renamed `setupScripts → setupRLS` for accuracy (the helper-fn sets up the RLS service, not arbitrary scripts); added a dual-form cross-reference in `expectations.yaml` to surface both the metric-name + the descriptor-key forms operators may scrape. The fix landed in commit `993f490`.

**Consequence:** Documentation accuracy restored; ZERO functional change. The 6 scenarios continue to pass at Gate D.

### Delta 3 — X-RateLimit encode-side body STUBBED at 24.1 per D-RL7 (PLAN-anticipated; not a deviation)

**Planned (per D-RL7 + the SPEC §16 split axis):** The encode-side X-RateLimit DRAFT_VERSION_03 response-header injection lands at 24.2. The 24.1 IMPL stores `f.ecb` (the encoder callbacks) at `SetEncoderCallbacks` for forward-24.2-use; the `EncodeHeaders` arm is a pass-through `Continue`.

**Status:** Compile-clean with the `//nolint:unused` annotation as the deliberate placeholder. The `rcDetailsRequestRateLimited` + `rcDetailsRateLimiterError` constants are pinned in `dispositions.go` for forward-24.2 consumption when/if the 3-arg `SendLocalReply` API extends to surface rc-details. The `headerXEnvoyRateLimited` constant is honored at 24.1 (in the OVER_LIMIT path's AMEND-8 header order).

**Consequence:** No deviation — the PLAN explicitly split the X-RateLimit work to 24.2 per the SPEC §16 axis + D-RL7. The 24.1 IMPL faithfully implements the 24.1 slice; 24.2 picks up at the encode-side filter body.

---

## 4. ADR roster

Three ADR §Decision-touchpoints landed at phase-24.1 IMPL. All landed at their per-Task Lands-in-Tasks per ADR-0044.

| ADR | §Decision / §Consequences disposition | Lands-in-Task | Commit SHA |
|---|---|---|---|
| **ADR-0197[core]** | **§Decision + §Consequences (24.1 slice)** — `internal/filter/http/ratelimit/` package shape + `TypeURL` + `New` + per-stream filter struct + `DecodeHeaders` dispatch + OK/OVER_LIMIT/error dispositions + reply byte-shape (AMEND-8 header order; `request_rate_limited`/`rate_limiter_error` rc-details ABSENT-BY-API; nullptr-mutate on fail-closed) + boot-registration (19 HTTP filters; alphabetical between `oauth2` and `rbac`). The X-RateLimit + encode-side slice lands at 24.2 via in-place §Decision amendment. | Task 7 | `5c665fa` (+ `d73226a` follow-up wake-up fix) |
| **ADR-0198** | **§Decision + §Consequences FULL** — DELTA-2 HCM route-table `rate_limits` exposure framework capability: parse + retain `rateLimits` on `routeEntry` + `vhostRateLimits` on `routeTable` at HCM-build time + validation via single-chokepoint `ratelimit.ValidateRouteRateLimits`; chain-seeded `RouteRateLimits()`/`VirtualHostRateLimits()` `DecoderFilterCallbacks` accessor pair via the set-once-by-dispatch primitive mirroring ADR-0165's `DownstreamRemoteAddr`; H1 + H2 dispatch-site seeding at `connection.go` + `h2dispatch.go`. **D-RL1 byte-confirmation: RAW-PROTO SEED CONFIRMED** — raw `[]*routev3.RateLimit` shape fit the existing primitive without divergence; NO pre-compiled carrier needed; NO non-proto type needed. **ADR-0202 UNCONSUMED** at this Task. HIGHEST RISK surface (parent §12 item 1) landed cleanly. | Task 5 | `7b91ef7` |
| **ADR-0200** | **§Decision + §Consequences FULL** — the FULL §5 PARSE-REJECT roster: the RATIFIED-from-PGV/config arms (`domain` empty; `rate_limit_service` missing; `stage > 10`; bad `request_type`; >10 response headers; etc.) + the 3 envoy-go-strict departures (`disable_key` non-empty; `extension` action; `dynamic_metadata` action) — all with byte-stable error wording per ADR-0080. Single-chokepoint validation via `ratelimit.ValidateRouteRateLimits` invoked from HCM `config.go`. Departures 15 → 18 (3rd, 4th, 5th cross-phase envoy-go-strict departures landed in 24.1). | Task 3 | `acbc3d1` |

**Next-free ADR: ADR-0202** (ADR-0197 + ADR-0198 + ADR-0200 + ADR-0201 [PLAN-time split] consumed; ADR-0199 stays as parent-SPEC-anchored §Context-only — body lands at 24.2 IMPL). DECISIONS.md tail at ADR-0201; ADR-0202 absent (D-hypothesis HELD; escape valve UNCONSUMED).

**NO ADR-0125 amendment at 24.1.** The `RateLimitPerRoute` 10th-canonical amendment 9 → 10 lands at 24.2 IMPL final Task per the AMENDMENT-anticipation paragraph anchored at the parent SPEC commit per ADR-0125 §(xv). **SECOND CONSECUTIVE §9 row to DEFER the ADR-0125 roster extension** (after phase 23's REUSE-by-absence skip); the streak ENDS at 24.2 IMPL final Task per the 22.1→22.3 anticipation→landing precedent.

---

## 5. Per-Task summary

14 task-landing commits ahead of master + this Task 12 atomic-landing commit = 15 commits total on the IMPL branch.

- **Pre-Task 0 — PROGRESS.md preamble + 12-precondition verification** (`cbe163e`). 12 cold-start preconditions verified verbatim; D-RL1..D-RL7 + D-P1..D-P3 PLAN-author resolutions reproduced in preamble; ADR tail at 0201 (ADR-0197..0200 §Context drafts anchored at parent SPEC per ADR-0044). 3 representative fuzzers (`FuzzAdmissionControlConfigParse` + `FuzzAdaptiveConcurrencyConfigParse` + `FuzzLuaConfigParse`) spot-checked clean. Pre-existing differential 33/33 PASS baseline.
- **Task 1 — Package skeleton `internal/filter/http/ratelimit/`** (`8649440`). NEW `ratelimit.go` + `ratelimit_test.go` + `doc.go`: TypeURL pin (`type.googleapis.com/envoy.extensions.filters.http.ratelimit.v3.RateLimit` per ADR-0143 SN1) + `filterName` const + per-stream `filter` struct with `SetDecoderCallbacks` / `SetEncoderCallbacks` / `OnDestroy` stubs + `New` factory stub. Compile-clean.
- **Task 2 — DELTA-1 `internal/grpcclient/ratelimit_client.go` `RateLimitClient`** (`3ee365c`). NEW `ratelimit_client.go` + `ratelimit_client_test.go`: unary `AuthClient`-clone wrapper composing the existing generic `Dialer` per ADR-0158; THIRD typed wrapper after `AuthClient` + `ProcessorClient`. `envoy.service.ratelimit.v3` stubs vendored in go-control-plane v1.32.4.
- **Task 3 — `compiled_config.go` + FULL §5 PARSE-REJECT roster + `ValidateRouteRateLimits` + ADR-0200** (`acbc3d1`). NEW `compiled_config.go` + `compiled_config_test.go`: AMEND-3 13-field roster + defaults/clamps (`rate_limited_status` 429 + <400 clamp; `status_on_error` 500 + <400 clamp; `timeout` 20ms; `stage` 0; `failure_mode_deny` false; `disable_x_envoy_ratelimited_header` false; etc.); `ValidateRouteRateLimits` single-chokepoint validation; FULL §5.1 + §5.2 PARSE-REJECT roster with byte-stable wording per ADR-0080; `compileResponseHeaders` pre-canonicalization; `validateGrpcServiceAndResolveCluster` for the RLS cluster reference. ADR-0200 §Decision + §Consequences FULL body anchored.
- **Task 4 — `stats.go` cluster-scoped cross-namespace 4-counter surface (110 → 114)** (`e8a5dd4`). NEW `stats.go` + `stats_test.go`: `filterStats` 4-counter struct (`ok`/`error`/`overLimit`/`failureModeAllowed`) under `cluster.<rls_cluster_name>.ratelimit[.<stat_prefix>].<stat>` CLUSTER-rooted prefix; idempotent registration via `(*stats.Registry).NewCounterIfAbsent` per ADR-0117 + AMEND-10; ADR-0085 nil-tolerance for test scopes without a Registry. FIRST cluster-scoped cross-namespace filter-stats surface (LANDS the AMEND-10 pattern).
- **Task 5 — DELTA-2 HCM route-table `rate_limits` exposure + accessor pair + ADR-0198 (HIGHEST RISK)** (`7b91ef7`). FRAMEWORK CHANGE: `internal/filter/hcm/route.go` + `internal/filter/hcm/connection.go` + `internal/filter/hcm/h2dispatch.go` + `internal/filter/hcm/config.go` + `internal/filter/http/callbacks.go` + `internal/filter/http/chain.go`. Parse + retain `[]*routev3.RateLimit` on `routeEntry` (matched-route Axis-A) + `routeTable.vhostRateLimits` (VirtualHost Axis-B carrier; honored at 24.2). HCM-parse-time validation via `ratelimit.ValidateRouteRateLimits`. H1 + H2 dispatch-site seeding via `chain.SetRouteRateLimits()` + `chain.SetVirtualHostRateLimits()` set-once-by-dispatch primitives mirroring ADR-0165. `DecoderFilterCallbacks.RouteRateLimits()` + `VirtualHostRateLimits()` accessor pair. **D-RL1 byte-confirmation: RAW-PROTO SEED CONFIRMED — ADR-0202 UNCONSUMED.** ADR-0198 §Decision + §Consequences FULL body anchored.
- **Task 6 — `descriptors.go` 5-CORE-action engine** (`2062297`). NEW `descriptors.go` + `descriptors_test.go`: `buildDescriptors` orchestrator + `buildDescriptorForPolicy` per-policy walker + `applyAction` dispatch + the 5 CORE actions (`actionGenericKey` + `actionRequestHeaders` + `actionRemoteAddress` + `actionDestinationCluster` + `actionHeaderValueMatch` with per-request HeaderMatcher Exact/Prefix/Suffix/Contains/SafeRegex evaluation — Custom arm UNSUPPORTED-AT-24.1) + the empty-action-drop discipline + the `actionUnsupportedAt241()` arm for the remaining 5 actions (drops the descriptor) + the Axis-A/OVERRIDE-default walk. `actionUnsupportedAt241` LANDS the remaining 5 actions at 24.2.
- **Task 7 — `decode_headers.go` + `dispositions.go` + full `New` + boot-registration + ADR-0197[core]** (`5c665fa`). NEW `decode_headers.go` + `dispositions.go` + `decode_headers_test.go` + `dispositions_test.go`: build descriptors → `Continue` if empty / async `ShouldRateLimit` dispatch with `StopIteration` + OnDestroy-cancel; OK/OVER_LIMIT/error dispositions per parent SPEC §4.7 + AMEND-8 header order; full `New` factory closure wiring `filterStats` + `RateLimitClient`; `cmd/envoy-go/main.go` boot-registration alphabetical between `oauth2` and `rbac` (18 → 19 HTTP filters). ADR-0197[core] §Decision + §Consequences (24.1 slice) anchored.
- **Task 7 follow-up — CRITICAL `ContinueDecoding`-after-`SendLocalReply` wake-up fix** (`d73226a`). Added explicit `f.dcb.ContinueDecoding()` after `f.dcb.SendLocalReply(...)` on OVER_LIMIT + fail-closed paths (the async goroutine's SendLocalReply alone sets `c.localReplyDone` but does NOT unblock the parked dispatch goroutine in `parkDecode`). 2 new tests asserting the wake-up invariant. Mirrors ext_authz deny-path + phase-09 fault filter precedent. ADR-0197 cross-ref dedup.
- **Task 8 — 33rd fuzzer `FuzzRateLimitConfigParse`** (`952a4c8`). NEW `fuzz_test.go`: 31 seeds (a valid full config; each §5.1 + §5.2 PARSE-REJECT arm; embedded vs route/vhost rate_limits; each action type; empty config). Must-never-panic across `buildCompiledConfig` + descriptor-engine compile. 30s clean at ~1.47M execs at Gate E. Fuzzer count 32 → 33.
- **Task 9 — Shared fake `RateLimitService` + `HTTPGlobalRateLimitGRPC` BackendKind=24** (`33fe8b4`). NEW `test/helpers/ratelimitgrpc/` package: proto-number-faithful fake RLS service per AMEND-6 (hits_addend `UInt64Value` + non-monotonic Unit enum; the fake encodes by proto NUMBER); `HTTPGlobalRateLimitGRPC = 24` BackendKind enum + runner switch arm. Both sides dial THIS fake → full cross-side byte-exact at Gate D.
- **Task 10 — Differential fixture `0032-http-ratelimit` (scenarios a/b/c/d-core/e/h)** (`adafe56` + `993f490` doc-fix follow-up). NEW `test/fixtures/0032-http-ratelimit/`: 6 scenarios — (a) parse_ok + (b) ok_admit cross-side byte-exact + (c) over_limit_reject cross-side byte-exact + (d-core) descriptor_engine 5-CORE-actions cross-side byte-exact + (e) error_fail_open + (h) stat_surface. `StatsAsserter` (NOT `SubjectAsserter`) per `reference_differential_asserter_dispatch.md`; liveness proven live via deliberate-break test per the phase-23 lesson. Scenarios (f) + (g) + (d-extension) DEFER to 24.2 per SPEC §16 axis. Follow-up: docs-only sweep removing stale burst/restart references + `setupScripts→setupRLS` + expectations.yaml dual-form cross-ref.
- **Task 11 — Differential fixture `0033-http-ratelimit-boot-reject` (`domain`-empty)** (`349e4c8`). NEW `test/fixtures/0033-http-ratelimit-boot-reject/`: shared boot-reject substring via the §5.1 `domain`-empty PARSE-REJECT arm; subject-AND-reference both refuse to boot with the matching substring. Fixture dir count 33 → 34 → 35.
- **Task 12 — Six-gate phase-done verification + STATE/ROADMAP advance + BEHAVIOR_CONTRACT partial bundle + REVIEW.md** (THIS commit). 6 phase-done gates verified GREEN; BEHAVIOR_CONTRACT.md partial bundle landed atomically (NEW `### envoy.filters.http.ratelimit` subsection + 3 envoy-go-strict departure records 15→18 + stat-name mapping 110→114 + per-route cross-reference paragraph anchoring ADR-0125 §(xv) anticipation); STATE.md re-advanced to `24.2 awaiting PLAN` with next-skill `superpowers:writing-plans`; ROADMAP row 24.1 flipped `in-progress → done` with per-cell IMPL-done annotation; parent row 24 STAYS `in-progress`; row 24.2 UNCHANGED `planned`; REVIEW.md authored; PROGRESS Task 12 entry appended.

---

## 6. Known limitations + future-work register (24.1 scope)

The phase-24.1 IMPL lands with the following recognized limitations — all are forward-pointers to 24.2 (the planned next sub-phase) or to future phases, NONE blocking 24.1 phase-done.

**6 phase-24.1 items (all PLAN-anticipated forward-pointers to 24.2):**

1. **X-RateLimit DRAFT_VERSION_03 response headers — STUBBED at 24.1 per D-RL7.** Forward-pointer to 24.2: `encode.go` + `headers.go` land at 24.2 with the full DRAFT_VERSION_03 header layout + MIN-status across multi-descriptor responses + the legacy `enable_x_ratelimit_headers=OFF` no-op path. Constants pinned at 24.1 for forward-use.
2. **Remaining 5 descriptor actions — UNSUPPORTED-AT-24.1.** `source_cluster`, `masked_remote_address`, `metadata`, `query_parameters` (key default `"query_param"`), `query_parameter_value_match`. Dispatch returns `(nil, false, true)` (drop the descriptor) — operators MUST scope policies to the 5 CORE actions until 24.2 lands the rest.
3. **`RateLimitPerRoute` — NOT-CONSUMED at 24.1.** Per-route `RateLimitPerRoute` TPFC entries pass HCM-parse-time validation but the runtime arms (Axis-B `vh_rate_limits` traversal + `domain` override + route-additional `rate_limits[]` Axis-A composition) are NOT-CONSUMED at 24.1; the filter only walks the matched Route's `rate_limits[]` (Axis-A subset). LANDS at 24.2 with the NEW 10th canonical + ADR-0125 §(xv) amendment 9 → 10.
4. **`stage` multi-stage bucketing — NOT-CONSUMED at 24.1.** The `stage` field is parsed + clamped to [0, 10] at 24.1 (default 0) but the multi-stage filter-by-stage walk is single-stage at 24.1. LANDS at 24.2 with the full stage iteration.
5. **Axis-B `vh_rate_limits` cross-tier composition — NOT-CONSUMED at 24.1.** The `vhostRateLimits` carrier is seeded onto the chain at H1 + H2 dispatch and accessible via `VirtualHostRateLimits()` accessor at 24.1, but the OVERRIDE/INCLUDE/IGNORE composition discipline lands at 24.2 with the per-route inclusion enum honoring + the legacy `include_vh_rate_limits` force-include arm.
6. **Header-matcher evaluation NOT pre-compiled.** Per-request `regexp.Compile` cost is present in the `header_value_match` SafeRegex arm. 24.2 MAY extract a pre-compile path if profiling surfaces the cost (out of 24.1 scope per the D-RL1 byte-confirmation's RAW-PROTO-only sanction).

**Cross-phase carry-forwards from earlier phases that this phase did NOT touch:** unchanged (per-phase deferral lists from phase-17 jwt_authn / phase-18.x ext_authz / phase-19.x ext_proc / phase-20 oauth2 / phase-21 adaptive_concurrency / phase-22.x lua / phase-23 admission_control carry forward).

---

## 7. Six-gate phase-done verification

Verbatim from Task 12's verification sweep. All 6 gates GREEN.

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

**Gate C — `go test -race -count=1 ./...`:** Single full-repo run (exit code 1 due to the 0025 multi-listener flake; re-ran clean in isolation per the documented flake class):
```
$ go test -race -count=1 ./... > /tmp/race-full.log 2>&1
---RACE-EXIT: 1---

$ grep -cE "^FAIL|^--- FAIL" /tmp/race-full.log
4
(the 4 lines: --- FAIL: TestDifferential, --- FAIL: TestDifferential/0025-http-adaptive-concurrency, FAIL, FAIL ... github.com/.../test/differential — all flow from one root-cause flake at 0025)

$ grep "^ok" /tmp/race-full.log | wc -l
63

$ grep "^ok" /tmp/race-full.log | grep -E "ratelimit|grpcclient|filter/hcm"
ok  	github.com/esalaine/envoy-go/internal/filter/hcm	1.119s
ok  	github.com/esalaine/envoy-go/internal/filter/hcm/h2	3.561s
ok  	github.com/esalaine/envoy-go/internal/filter/http/localratelimit	1.054s
ok  	github.com/esalaine/envoy-go/internal/filter/http/ratelimit	1.083s
ok  	github.com/esalaine/envoy-go/internal/grpcclient	1.191s
ok  	github.com/esalaine/envoy-go/test/helpers/ratelimitgrpc	1.061s
```
63 packages `ok` repo-wide; ALL 24.1 packages race-clean (`internal/filter/http/ratelimit` + `internal/grpcclient` + `internal/filter/hcm` + `internal/filter/hcm/h2` + `test/helpers/ratelimitgrpc`). The single failing line:
```
--- FAIL: TestDifferential/0025-http-adaptive-concurrency (0.87s)
    runner_test.go:741: subj start: subject ready: EOF
```
documented multi-listener `EOF` flake class (per Pre-Task 0 + Task 5 + Task 7 evidence + project memory `reference_differential_fixture_dispatch_constraint.md`). Isolated re-run:
```
$ go test -race -count=1 -timeout 5m -run 'TestDifferential/0025-http-adaptive-concurrency' ./test/differential/
ok  	github.com/esalaine/envoy-go/test/differential	6.045s
---EXIT: 0---
```
GREEN. The 24.1 surface is fully race-clean; the 0025 flake is a known-flaky multi-listener fixture unrelated to phase 24.1.

**Gate D — `go test -count=1 -timeout 30m ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])'`** (35/35 anchored regex per the user's brief; PASS on the first run):
```
$ go test -count=1 -timeout 30m ./test/differential/ -run 'TestDifferential/00(0[0-9]|1[0-9]|2[0-9]|3[0-3])'
ok  	github.com/esalaine/envoy-go/test/differential	102.442s
---DIFF-EXIT: 0---
```
All 35/35 fixtures PASS in 102.4s (0000-tcp-echo through 0033-http-ratelimit-boot-reject inclusive — the regex `3[0-3]` covers 0030-0033). The two NEW 24.1 fixtures (`0032-http-ratelimit` + `0033-http-ratelimit-boot-reject`) both PASS.

A second `-v` confirmation run flaked 2 unrelated fixtures (`0020-http-ext-authz-http` + `0023-http-ext-proc-body` — both documented multi-listener "address already in use" / EOF flakes per the well-known flake class); isolated re-run of 0023 confirms it as a flake:
```
$ go test -count=1 -timeout 5m -run 'TestDifferential/0023-http-ext-proc-body' ./test/differential/
ok  	github.com/esalaine/envoy-go/test/differential	2.066s
---EXIT: 0---
```
The 24.1 NEW fixtures PASSED on BOTH runs. The first-run 35/35 GREEN is load-bearing per the SPEC §6 Gate D acceptance.

**Gate E — `go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/ -v` (seed corpus) + 30s live fuzz run:**
```
$ go test -count=1 -run 'FuzzRateLimitConfigParse' ./internal/filter/http/ratelimit/ -v 2>&1 | tail -10
    --- PASS: FuzzRateLimitConfigParse/seed#23 (0.00s)
    --- PASS: FuzzRateLimitConfigParse/seed#24 (0.00s)
    --- PASS: FuzzRateLimitConfigParse/seed#25 (0.00s)
    --- PASS: FuzzRateLimitConfigParse/seed#26 (0.00s)
    --- PASS: FuzzRateLimitConfigParse/seed#27 (0.00s)
    --- PASS: FuzzRateLimitConfigParse/seed#28 (0.00s)
    --- PASS: FuzzRateLimitConfigParse/seed#29 (0.00s)
    --- PASS: FuzzRateLimitConfigParse/seed#30 (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/ratelimit	0.004s
---FUZZ-SEED-EXIT: 0---

$ go test -fuzz='^FuzzRateLimitConfigParse$' -fuzztime=30s ./internal/filter/http/ratelimit/
fuzz: elapsed: 27s, execs: 1429399 (14638/sec), new interesting: 18 (total: 412)
fuzz: elapsed: 30s, execs: 1466640 (12410/sec), new interesting: 18 (total: 412)
fuzz: elapsed: 31s, execs: 1466640 (0/sec), new interesting: 18 (total: 412)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/ratelimit	31.111s
---FUZZ-30S-EXIT: 0---
```
31 seeds clean at baseline; 1,466,640 execs in 30s; 18 new-interesting; 0 panics; 0 crashers. Total fuzzer count = **33** (`find . -name 'fuzz_test.go' -exec grep -h '^func Fuzz' {} \; | sort -u | wc -l` = 33).

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
--- PASS: TestH2Spec (2.97s)
PASS
ok  	github.com/esalaine/envoy-go/test/conformance/h2spec	3.058s
---H2SPEC-EXIT: 0---
```
53/53 PASS at the ADR-0051 v1.32.4 pin. Phase 24.1 touched no H2 codec path; the PASS confirms zero regression.

**ADR-0202 absence verification:**
```
$ for n in 0197 0198 0199 0200 0201 0202; do echo "ADR-$n: $(grep -cE "^## ADR-$n\b" docs/envoy-go/DECISIONS.md)"; done
ADR-0197: 1
ADR-0198: 1
ADR-0199: 1
ADR-0200: 1
ADR-0201: 1
ADR-0202: 0
```
ADR-0197 + ADR-0198 + ADR-0200 + ADR-0201 (PLAN-time split) consumed; ADR-0199 anchored at parent SPEC (§Context only at 24.1; §Decision + §Consequences body lands at 24.2); ADR-0202 absent (D-hypothesis HELD; escape valve UNCONSUMED).

---

## 8. Parent-rollup status

**Phase 24.1 (FIRST sub-phase of the phase-24 split per ADR-0201) is CLOSED at this Task 12 commit.** The parent row `24 http-filter-global-ratelimit` STAYS `in-progress` per the 18/19/22 rollup precedent — the parent row flips `in-progress → done` at the 24.2 phase-done commit (not at this 24.1 phase-done commit). The 24.2 sub-phase row stays `planned` and opens NOW per the depends-on chain.

The §9 HTTP-filters family currently has 16 family-rows landed (phases 7.1 / 9 / 10 / 11 / 12 / 13 / 14 / 15 / 16 / 17 / 18 / 19 / 20 / 21 / 22 / 23) + the in-flight phase 24 (the parent row stays `in-progress` until 24.2). **At 24.1 phase-done specifically, the §9 row count is STILL 2 remaining (`wasm` + the rollup-pending parent 24)**; the trailing `1 remaining row` lands at the 24.2 phase-done commit which flips parent row 24 to done.

---

## 9. Lessons learned

**Async-dispatch `SendLocalReply` requires explicit `ContinueDecoding()` to wake the parked dispatch goroutine.** Phase-24.1's initial Task-7 commit landed the OVER_LIMIT + fail-closed dispositions with `SendLocalReply` alone, mirroring the synchronous-decode pattern used by the cors + fault + csrf precedents. But the ratelimit dispatch is ASYNC (the response arrives on a non-dispatch goroutine after `ShouldRateLimit` returns), and the dispatch goroutine is parked in `parkDecode` waiting on `decodeResumeCh`. `SendLocalReply` from the async goroutine sets `c.localReplyDone` but does NOT signal `decodeResumeCh` — the dispatch goroutine never wakes. **Lesson:** any filter that dispatches async + returns `StopIteration` MUST follow `SendLocalReply` with `ContinueDecoding()` to wake the parked decode goroutine. The ext_authz deny-path precedent (`extauthz.go:1097-1111`) + the phase-09 fault filter `SendLocalReply+ContinueDecoding` pattern (`fault.go:299-324`) already established this; the Task-7 follow-up restored consistency. Future async-dispatch filter authors should grep for `SendLocalReply` + `StopIteration` co-occurrence + assert `ContinueDecoding` follows.

**The D-RL1 byte-confirmation discipline is the right pattern for HIGH-risk framework-primitive surfaces.** Phase-24.1's HIGHEST risk surface was the DELTA-2 chain-seed type byte-confirmation (parent SPEC §12 item 1). The PLAN reserved ADR-0202 as the escape valve in case the raw `[]*routev3.RateLimit` shape didn't fit the existing ADR-0165 set-once primitive. Task 5 performed the byte-confirmation IN-SESSION against the ADR-0165 plumbing (5 specific file:line citations in the PROGRESS entry); the seed shape fit cleanly; ADR-0202 stayed UNCONSUMED. **Lesson:** for any future HIGH-risk framework-primitive surface, the byte-confirmation discipline (cite existing-primitive file:line; confirm the new surface fits; reserve an escape-valve ADR; record the IN-SESSION verification outcome in the consuming task's PROGRESS) catches the divergence early and surfaces the disposition cleanly. The phase-24.1 outcome (D-hypothesis HELD) demonstrates the discipline works when the byte-confirmation succeeds.

**Differential fixture asserter dispatch — `StatsAsserter` not `SubjectAsserter` on cross-side runner paths.** Project memory `reference_differential_asserter_dispatch.md` codified the lesson learned at phase-23 IMPL: cross-side fixtures (RequiresReference=true) take the cross-side runner path which calls `StatsAsserter.AssertStats` (not `SubjectAsserter.AssertSubject` — the latter is only called on the reference-less path). Phase-24.1's Task 10 honored this from the start; subject-side assertions are in `StatsAsserter`. **Lesson:** the project memory + the per-Task review pass that grep-asserted the right asserter type catches the dead-assertion class proactively; future filter fixtures that need subject-side checks on a cross-side path MUST use `StatsAsserter` and prove liveness via a deliberate-break test.

---

## 10. Forward-pointers carried into next sub-phase (24.2)

The next-sub-phase inheritance set per the Task 12 STATE.md advance:

**6 phase-24.1-emergent forward-pointers (per §6 above) — all PLAN-anticipated 24.2 work:**
1. X-RateLimit DRAFT_VERSION_03 response headers — `encode.go` + `headers.go` land at 24.2.
2. Remaining 5 descriptor actions (`source_cluster` / `masked_remote_address` / `metadata` / `query_parameters` / `query_parameter_value_match`) land at 24.2.
3. `RateLimitPerRoute` 10th canonical + ADR-0125 §(xv) amendment 9 → 10 land at 24.2.
4. `stage` multi-stage bucketing land at 24.2.
5. Axis-B `vh_rate_limits` cross-tier composition (OVERRIDE/INCLUDE/IGNORE + legacy force-include) land at 24.2.
6. Per-request HeaderMatcher pre-compile path is a 24.2-optional optimization if profiling surfaces the cost.

**Cross-phase carry-forwards from earlier phases that this phase did NOT touch:** unchanged.

**STATE.md post-Task-12 disposition:** `active-phase: 24.2-global-ratelimit-perroute-and-headers`; `lifecycle-state: phase 24.1 IMPL done; awaiting 24.2 PLAN` (SKILL_ROUTING state 2 — SPEC.md exists for 24.2, PLAN.md does not); `next-skill: superpowers:writing-plans` (to author the 24.2 PLAN.md from the existing 24.2 SPEC.md); `last-commit: TBD-24.1-IMPL-SQUASH` placeholder for post-squash STATE SHA-fill; `next-free ADR: ADR-0202` (D-hypothesis HELD; escape valve UNCONSUMED).

**§9 family closure trail at 24.1 phase-done:** 16 family-rows landed + the in-flight phase 24 (parent stays `in-progress`); 2 §9 rows remain: `wasm` + the rollup-pending parent 24. The trailing `1 remaining row` lands at the 24.2 phase-done commit which flips parent row 24 to done.

---

## 11. Sign-off

Phase 24.1 is **APPROVED for master squash-merge per project memory `feedback_git_worktrees.md`** + ADR-0003 worktree-isolation discipline + ADR-0005 §Decision 4 worktree-merge discipline. All 6 phase-done gates GREEN at this Task 12 HEAD (Gates A/B/E/F/D first-run clean; Gate C race had the documented 0025 multi-listener flake which re-ran clean in isolation — the 24.1 packages themselves are fully race-clean); all 9 of the 24.1 SPEC §7 acceptance items GREEN (4 explicit PARTIAL annotations are all PLAN-anticipated 24.1/24.2 splits per the SPEC §16 axis); 3 ADR §Decision-touchpoints cleanly anchored at their per-Task Lands-in-Tasks per ADR-0044 (ADR-0197[core] + ADR-0198 + ADR-0200 — all anticipated NEW ADRs §Decision + §Consequences full bodies); **NO ADR-0125 amendment at 24.1** (SECOND CONSECUTIVE §9 row to DEFER the roster extension after phase 23's REUSE-by-absence skip; the streak ENDS at 24.2 IMPL final Task per the AMENDMENT-anticipation paragraph anchored at the parent SPEC commit per ADR-0125 §(xv)); **D-hypothesis HELD** — ADR-0202 UNCONSUMED at 24.1 phase-done (the D-RL1 byte-confirmation RESOLVED as RAW-PROTO SEED CONFIRMED at Task 5; the raw `[]*routev3.RateLimit` shape fit the existing ADR-0165 `DownstreamRemoteAddr` set-once primitive without divergence; next-free ADR stays at ADR-0202); **D-RL1..D-RL7 + D-P1..D-P3 PLAN-author dispositions** all confirmed; 33 fuzzers + 35 differential fixtures GREEN (the two NEW 24.1 fixtures `0032-http-ratelimit` cross-side + `0033-http-ratelimit-boot-reject` both PASS on the first-run); h2spec 53/53 at ADR-0051 v1.32.4 pin; BEHAVIOR_CONTRACT partial bundle landed at this Task 12 commit per ADR-0052 atomic landing (NEW `### envoy.filters.http.ratelimit` subsection CORE slice + 3 envoy-go-strict departure records 15→18 + stat-name mapping 110→114 + per-route cross-reference paragraph with §(xv) AMENDMENT-anticipation); **ROADMAP row 24.1 flipped `in-progress → done` AT THIS COMMIT** (date `2026-05-23`) with per-cell IMPL-done annotation; **parent row 24 STAYS `in-progress`** (closes at 24.2 phase-done per the 18/19/22 rollup precedent); **row 24.2 UNCHANGED `planned`** (opens at this commit); STATE.md re-advanced to `24.2 awaiting PLAN` with next-skill `superpowers:writing-plans`; **3 small artifact deltas recorded** (Task-7 follow-up CRITICAL `ContinueDecoding`-after-`SendLocalReply` wake-up fix / Task-10 doc-fix follow-up / X-RateLimit encode-side body STUBBED at 24.1 per D-RL7 — the latter PLAN-anticipated, not a deviation).

The squash-merge + STATE SHA-fill follow-up + push-to-origin are the user's manual steps after this Task 12 commit lands (per the phase-09..23 squash-merge convention + project memory `feedback_push_to_origin.md`).

**Summary stats:** 12 PLAN tasks (Pre-Task 0 + Tasks 1-11) + 2 follow-up commits + 1 Task 12 atomic-landing = **15 commits on the IMPL branch ahead of master tip `e8a8881`**. 3 NEW ADR §Decision + §Consequences full bodies (ADR-0197[core] + ADR-0198 + ADR-0200 — all anticipated). 33 fuzzers (32 from phase-23 + 1 NEW `FuzzRateLimitConfigParse`). 35 differential fixtures (33 from phase-23 + 2 NEW `0032-http-ratelimit` cross-side + `0033-http-ratelimit-boot-reject`). 6 phase-done gates GREEN. 18 envoy-go-strict departures (was 15; +3: `disable_key` PARSE-REJECT + `extension` action PARSE-REJECT + `dynamic_metadata` action PARSE-REJECT — all per ADR-0200). 19 HTTP filters wired (was 18; +1 `ratelimit`). 114 stats (was 110; +4 counters CLUSTER-rooted per AMEND-1 — FIRST cluster-scoped filter-stats surface). **TWO framework deltas** (DELTA-1 `RateLimitClient` typed wrapper + DELTA-2 HCM route-table `rate_limits` exposure with the `RouteRateLimits()`/`VirtualHostRateLimits()` chain-seeded accessor pair — NOT framework-lean; the highest-risk surface at parent §12 item 1 landed cleanly via the D-RL1 byte-confirmation). **SECOND CONSECUTIVE §9 row to DEFER the ADR-0125 roster extension** (canonical-per-route roster STAYS 9; the 10th-canonical AMENDMENT body lands at 24.2 IMPL final Task per ADR-0125 §(xv)). **D-hypothesis HELD — ADR-0202 UNCONSUMED at 24.1 phase-done.**

**End of phase 24.1 review. The next session is the phase-done squash-merge + STATE SHA-fill + push-to-origin follow-up per project memory `feedback_git_worktrees.md` + `feedback_push_to_origin.md`; the session AFTER that is the 24.2 PLAN authoring per `superpowers:writing-plans`.**
