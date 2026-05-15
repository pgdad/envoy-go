# Phase 18.1 — Code review (REVIEW.md)

**Phase id:** `18.1` (ELEVENTH §9 HTTP-filters family-row to land per ADR-0106; FIRST §9 row to REUSE an ADR-0125 canonical rather than extend the roster — 5th canonical, per ADR-0163; FIRST §9 row with a per-request cancellable outbound call; FIFTH §9 row to ship pure decode-side after csrf@12, buffer@13, rbac@16, jwt_authn@17; ADR-0044 escape-valve NOT triggered — ADR-0165 NOT authored; 7 SPEC-anticipated ADRs ADR-0156/0157/0159/0160/0161/0162/0163)
**Slug:** `18.1-ext-authz-http`
**Branch under review:** `phase-18.1-ext-authz-http-impl`
**Range:** branch tip `6175ec5` (Task 14 review-fix at this SHA; REVIEW.md is the final Task 15 commit). 14 task-landing commits (Tasks 1–14) + multiple in-task follow-up and review-fix commits at worktree HEAD; phase-done six-gate verification at `9dd79ca`. Lifecycle-state advance landed at the Task 14 STATE.md edit; the last-commit SHA-fill is deferred to the post-`wt-merge` follow-up per the phase-09..17 close pattern.
**Parent ROADMAP row:** `18.1 ext-authz-http` flipped `in-progress → done` at Task 14 commit `9dd79ca` (date `2026-05-15`). Parent row `18` remains `in-progress` — it closes only when 18.2 is `done` per the parent SPEC §2.
**Reviewer method:** Inline authoring by the implementing session — inputs: SPEC §15 15-claim acceptance checklist + PLAN's 15-task structure + the branch diff + PROGRESS.md per-task entries (Tasks 1–14 with verbatim outputs) + DECISIONS.md ADR-0156..ADR-0163 + BEHAVIOR_CONTRACT §13 6-edit bundle + phase-17 REVIEW.md structural template.
**Six-gate state at HEAD:** all GREEN per Task 14's verification sweep — outputs reproduced verbatim in §4 below.

This review covers the full phase-18.1 surface: the NEW `internal/filter/http/extauthz/` package (`doc.go` 192 + `extauthz.go` 1087 + `check.go` 411 + `attributes.go` 435 = **2125 LoC production**; `extauthz_test.go` + `fuzz_test.go` test surface); the NEW `test/helpers/extauthzhttp/` test-helper (`doc.go` 25 + `extauthzhttp.go` ~155 = ~180 LoC; `extauthzhttp_test.go`); boot registration at `cmd/envoy-go/main.go` (+3 LoC; alphabetical between `envoygotest` and `fault` per ADR-0156); differential fixture `0020-http-ext-authz-http` (7 scenarios; three-listener topology; `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` + `inputs/driver.go`); fixture-runner wiring at `test/differential/fixture/fixture.go` + `test/differential/runner_test.go`; `FuzzExtAuthzConfigParse` (the 22nd fuzzer); the 7 SPEC-anticipated ADRs ADR-0156/0157/0159/0160/0161/0162/0163; the BEHAVIOR_CONTRACT 6-edit bundle; the ROADMAP row 18.1 status flip + STATE.md advance to lifecycle-state-4.

This REVIEW closes phase 18.1's lifecycle (state-4; the last-commit SHA-fill follow-up on master is the only remaining mechanical step) and is the final task before merge to master.

---

## 1. Phase summary

**APPROVED.** All six phase-done gates are GREEN at HEAD `6175ec5` (verification sweep at `9dd79ca` per Task 14; Task 14 review-fix `6175ec5` is documentation-only — STATE.md `next-skill` correction — and carries no gate-affecting changes). The implementation faithfully realizes the SPEC across all 15 PLAN tasks. ext_authz (HTTP mode) is the ELEVENTH §9 HTTP-filters family-row to ship under ADR-0106.

The architectural centerpiece is the full decode-side external-authorization gate: `DecodeHeaders` resolves the per-route config (5th-canonical REUSE, SHARED-stats), optionally buffers the request body via the phase-13 ADR-0128 primitive, builds the HTTP `AuthorizationRequest` (request-side header filtering through `allowed_headers`/`disallowed_headers`, `headers_to_add` static additions, `path_prefix` prepend), fires an async outbound POST to the configured auth service via the thin `httpAuthClient` (ADR-0159 disposition (b)), and on resume applies the mode-agnostic `{allow, deny, error}` disposition — allow-path upstream header injection + optional `ClearRouteCache`; deny-path `SendLocalReply` with the auth service's verbatim status/body/`allowed_client_headers`-filtered headers; error-path `failure_mode_allow`/`status_on_error` posture. The per-request `context.Context` (cancelled at `OnDestroy`) makes the in-flight POST return promptly; the `mu`/`done` guard prevents callback-touches after stream teardown.

The differential fixture `0020-http-ext-authz-http` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 7 scenarios across a three-listener topology (l_test_a: allow/deny/body/per-route disabled/per-route check_settings; l_test_b: error→`status_on_error`; l_test_c: `failure_mode_allow`). All 7 PASS at byte-exact body/status + cross-side counter-delta equivalence on the 5 reachable counters.

Seven ADRs landed (ADR-0156/0157/0159/0160/0161/0162/0163), all SPEC-anticipated per ADR-0044's standard discipline. **The ADR-0044 escape-valve was NOT triggered** — the `mu`/`done` guard + `context.WithCancel` (planner-time decision D4) sufficed; ADR-0165 was NOT authored. Next-free ADR remains ADR-0165.

**Notable discovery at Task 13:** Two missing `ContinueDecoding()` calls after `SendLocalReply` (deny path + error→`status_on_error` path). The phase-09 fault filter (`fault.go:321-324`) is the canonical `SendLocalReply + ContinueDecoding()` precedent; the phase-18.1 impl missed it in the initial `applyDisposition`/`applyErrorPosture` authors. Fixed at Task 13 iteration 1 before the fixture passed. Unit tests updated (`TestDecodeHeaders_AsyncDeny_SendLocalReply` + `TestDecodeHeaders_AsyncError_FailureModeAllow_False`: `want 0` → `want 1` for ContinueDecoding call count). See §7 notable lessons.

---

## 2. ADR roster

Each of the seven anticipated ADRs ADR-0156..ADR-0163 (ADR-0158 is 18.2), evaluated for whether the §Decision body held up under implementation and fixture exercise:

**ADR-0156** (`internal/filter/http/extauthz/` package shape — single-token directory + DECODER-only `HTTPFilter` (`Encoder: nil`) + 6-base-counter `filterStats` registered unconditionally at `New()` time + deny-path `SendLocalReply` mechanism + boot-registration alphabetical between `envoygotest` and `fault`; Lands-in: Task 2): **VALIDATED.** Single-token directory `extauthz/` per ADR-0114 (matches `localratelimit/`, `jwtauthn/`). DECODER-only shape verified — `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` compile-time assertion; `Encoder: nil`. 6-counter `filterStats` unconditional allocation verified at Group 2 stats sub-group tests (Task 8). Boot-registration ordering: `... envoygotest → extauthz → fault ...` (`grep -cE 'httpReg\.Register' cmd/envoy-go/main.go` returns 13 post-Task-10).

**ADR-0157** (`compiledConfig` shape + `services`-oneof dual-mode dispatch (a `checkFn` closure; `grpc_service` arm PARSE-REJECTs in 18.1, §Decision amended at 18.2 IMPL) + consumed-vs-deferred field discipline + error-posture fields + `transport_api_version` V3-only PARSE-REJECT + empty-`services` factory rejection + the §5.P10 error-classification boundary; Lands-in: Task 2): **VALIDATED.** `services`-oneof dispatch confirmed — `http_service` builds the real `checkFn`; `grpc_service` PARSE-REJECTs (`"grpc_service mode not yet supported (lands in phase 18.2)"`); empty PARSE-REJECTs (`"services oneof must be set"`). `transport_api_version` non-V3 PARSE-REJECTs per ADR-0008. The mode-agnostic `compiledConfig` struct shape is field-final at 18.1 (18.2 adds no fields, only supplies the gRPC `checkFn` constructor). ADR-0157's §Decision is NOT yet amended — the gRPC arm amendment lands at 18.2 IMPL per the SPEC §10 anchor.

**ADR-0159** (HTTP-outbound auth-check framework primitive — thin ext_authz-local `httpAuthClient`; disposition (b) per SPEC §3.1; `httpAuthClient` wrapping `*http.Client` + timeout + `path_prefix`; ZERO retry (D2); composing-against but NOT reusing the phase-17 `internal/jwks/Fetcher` outbound-HTTP structure; the (a)-vs-(b) record + the oauth2-triggers-`internal/httpclient/`-generalization forward-pointer; Lands-in: Task 3): **VALIDATED.** `httpAuthClient` in `check.go` confirmed thin (no cache, no retry, no async-refresh). The closure threads `http.NewRequestWithContext(ctx, ...)` so `OnDestroy`'s `callCancel()` aborts the in-flight call promptly. The (a)-vs-(b) record and the oauth2 forward-pointer are in ADR-0159 §Decision. BEHAVIOR_CONTRACT §13.7 carries the `## HTTP outbound auth-check framework note` per D10.

**ADR-0160** (`AuthorizationRequest` builder — `headers_to_add` + `path_prefix` prepend + top-level `allowed_headers`/`disallowed_headers` request-side filtering + deprecated `AuthorizationRequest.allowed_headers` honored-if-present; HTTP-mode portion; Lands-in: Task 4): **VALIDATED.** `buildAuthRequest` in `attributes.go` (hs-independent per the Task 4 Option B ruling; `headersToAdd []headerKV` pre-compiled at config-load time at Task 9 review-fix). Pseudo-header strip added in Task 9 review-fix (Fix 1 side-discovery: Go's `net/http` rejects `:` prefixed headers). D6 confirmed (deprecated `AuthorizationRequest.allowed_headers` #1 honored-if-present — field is deprecated-but-present in go-control-plane v1.32.4 proto). D5 confirmed (`google_re2` arm honored; other engine arms PARSE-REJECT).

**ADR-0161** (bidirectional header-mutation discipline — `AuthorizationResponse.{allowed_upstream_headers, allowed_upstream_headers_to_append, allowed_client_headers}` compilation + allow-path upstream injection + deny-path downstream `allowed_client_headers`-filtered emission + `validate_mutations` gating → `invalid` counter + deny-path header-set construction (`text/plain` fallback, decision-headers-first ordering) + `allowed_client_headers_on_success` deferral; HTTP-mode portion; Lands-in: Task 5): **VALIDATED.** Response-side matcher compilation in `check.go`'s `buildHTTPCheckFn` at config-load time. `applyUpstreamMutations` applies `upstreamSet` (overwrite) then `upstreamApp` (append). Deny-path header ordering: decision headers first, framework housekeeping after — confirmed byte-equivalent to reference Envoy at Task 13 §18.P11 closure. `validate_mutations` → `dispInvalid` → `invalid` counter + error posture verified. `allowed_client_headers_on_success` DEFERRED (documented divergence-window).

**ADR-0162** (request-body inclusion — `with_request_body{max_request_bytes, allow_partial_message, pack_as_bytes}` + ADR-0128 decode-side body-buffering reuse + `allow_partial_message:false` over-limit → `SendLocalReply(413, "Payload Too Large", {connection: close})`, auth NOT called, NO counter increments + the `DecodeHeaders`-`Continue`/`DecodeData`-`StopIterationAndBuffer` interaction; Lands-in: Task 6): **VALIDATED.** `effectiveWithRequestBody(pr)` implements the 3-tier precedence (per-route `disable_request_body_buffering` → nil; per-route `with_request_body` → per-route override; listener-level fallback). Over-limit 413 path emits `connection: close` per §5.P5 and increments ZERO ext_authz counters — load-bearing invariant tested at `TestDecodeData_OverLimit_NoCounterIncrements`. ADR-0128 `Continue`-not-`StopIteration` discipline confirmed (ADR-0076 + connection.go synchronous HCM dispatch constraint — returning `StopIteration` from `DecodeHeaders` for body buffering would deadlock). `pack_as_bytes` parsed and stored for 18.2 gRPC-mode no-op in 18.1 HTTP-mode. ext_authz is the THIRD ADR-0128 consumer (after phase-13 buffer + phase-15 bandwidth_limit).

**ADR-0163** (per-route 5th-canonical REUSE classification — NO ADR-0125 amendment paragraph; `disabled` arm PGV `const: true`; `override` oneof PGV-required; `check_settings` narrower-override merge; SHARED-stats; `sync.Map` lazy-cache identity; 6-counter stat surface `http.<HCM_stat_prefix>.ext_authz.*` SN2-reuse; RATIFIED-PENDING-IMPL-TIME §18.P6 + §18.P7 closed at Task 8; Lands-in: Task 7): **VALIDATED.** NO `**(xiv)**` amendment paragraph (`grep -cE '^\*\*(xiv)\*\*' docs/envoy-go/DECISIONS.md` = 0 confirmed at Tasks 1, 7, 14). SHARED-stats: no per-route `*filterStats` — all routes share the listener-level `compiledConfig.stats`. `sync.Map` `LoadOrStore` pointer-identity keyed by `*ExtAuthzPerRoute` proto pointer per ADR-0117 + ADR-0125 §(v). §18.P6 + §18.P7 RATIFIED at Task 8 empirical scrape (SN2-reuse confirmed verbatim — `envoy_http_ext_authz_<counter>{envoy_http_conn_manager_prefix="ingress_http"}`; no new SN-flattening rule needed; no ADR-0163 amendment required). The 5th-canonical REUSE at ext_authz marks the FIRST §9 row since phase-13 buffer (which INTRODUCED the 5th canonical) to REUSE it without extension.

---

## 3. Empirical pins outcome

All 13 parent SPEC §5 empirical pins were resolved IN-SESSION at SPEC drafting per ADR-0004 (probe date 2026-05-14; reference Envoy `envoyproxy/envoy:v1.37.2` + go-control-plane v1.32.4). The parent SPEC §5 is the authoritative per-pin reference. The 18.1-load-bearing pins and their impl-time outcomes:

**§18.P1 (RATIFIED-AND-EXTENDED)** — 28-field `ExtAuthz` proto roster; `services` oneof NOT PGV-required (factory rejects empty); top-level `allowed_headers`/`disallowed_headers`; deprecated `AuthorizationRequest.allowed_headers`. Held through implementation — all 28 fields parsed or silently-ignored per the consumed-vs-deferred discipline. D6 confirmed (`AuthorizationRequest.allowed_headers` deprecated-but-honored at impl-time).

**§18.P2 (RATIFIED)** — `ExtAuthzPerRoute{oneof{disabled(bool, const: true) | check_settings(CheckSettings)}}`; oneof PGV-required; PGV wrinkles. Held — `parsePerRoute` PGV-mirror PARSE-REJECTs `disabled: false` and empty `override`; `check_settings` XOR `disable_request_body_buffering` / `with_request_body` enforced. 5th-canonical-REUSE classification confirmed; NO ADR-0125 §(xiv).

**§18.P5 (RATIFIED)** — `allow_partial_message:false` + over-limit → local 413 + `connection: close`, auth NOT called. Held — `TestDecodeData_OverLimit_NoCounterIncrements` + `TestDecodeData_OverLimit_AllowPartialFalse_413` confirm zero counter increments and the 413 wire shape.

**§18.P6 (REFINED; RATIFIED-PENDING-IMPL-TIME → RATIFIED at Task 8)** — 6-counter stat surface; `disabled` STRUCTURALLY UNREACHABLE under MVP (NOT incremented by per-route disable). CLOSED RATIFIED at Task 8 empirical scrape: reference Envoy v1.37.2 `/stats/prometheus` renders the 6 MVP counters + 5 deferred-feature extras; SN2-reuse confirmed verbatim; no ADR-0163 amendment required. `disabled` stays at 0 for the fixture lifetime (confirmed `TestFilterStats_DisabledCounter_RegisteredButZero`).

**§18.P7 (RATIFIED; RATIFIED-PENDING-IMPL-TIME → RATIFIED at Task 8)** — Prometheus SN2-reuse namespace flattening: `envoy_http_ext_authz_<counter>{envoy_http_conn_manager_prefix="<stat_prefix>"}`. CLOSED RATIFIED at Task 8 empirical scrape (same run as §18.P6). Counter-delta cross-side equivalence confirmed at Task 13 fixture scrape.

**§18.P8 (RATIFIED-AND-EXTENDED)** — `ListStringMatcher` → `StringMatcher` exact/prefix/suffix/contains/`safe_regex`/`custom` + `ignore_case`; top-level `ExtAuthz.allowed_headers`/`disallowed_headers` are the live fields. Held — `compileStringMatcherList` in `attributes.go` supports all non-`custom` arms; `custom` PARSE-REJECTs envoy-go-strict. D5 confirmed (`google_re2` honored; others PARSE-REJECT).

**§18.P9 (PARTIAL → DEFER)** — `allowed_client_headers_on_success` decode-side infeasibility. Held — field parses (silent-ignore); divergence-window documented at BEHAVIOR_CONTRACT §13.4 + `doc.go`.

**§18.P10 (RATIFIED)** — error-classification boundary: transport/timeout/connect failure/unrecognized HTTP status → error → `status_on_error`; HTTP 200 → allow; HTTP 401/403 → deny. Held in `check.go` `mapHTTPResponseWithMatchers` — confirmed at fixture scenarios 2 (deny-403), 3 (error-connect-refused), 4 (error-connect-refused + `failure_mode_allow:true`).

**§18.P11 (RATIFIED; RATIFIED-PENDING-IMPL-TIME → RATIFIED at Task 13)** — deny-path header ordering: auth-supplied/decision headers first, framework housekeeping (`content-length`, `date`, `server: envoy`) after; `content-type: text/plain` fallback; body verbatim; no `x-envoy-*` added. CLOSED RATIFIED at Task 13 fixture-harness differential diff (scenario 2 byte-stream comparison; reference Envoy and envoy-go both emit `x-authz-denied` before `content-type`/`content-length`/`date`/`server`).

**§18.P12 (RATIFIED)** — `filter_enabled`/`filter_enabled_metadata`/`deny_at_disable` all default to no-op when unset; fixture configs need NO explicit settings. Held — the fixture exercises all scenarios without these fields; `disabled` counter remains at 0 throughout (STRUCTURALLY UNREACHABLE under MVP).

**RATIFIED-PENDING-IMPL-TIME tally:** 3 pins were RATIFIED-PENDING at SPEC time → all 3 CLOSED during 18.1 IMPL:
- §18.P6 → RATIFIED at Task 8 empirical scrape
- §18.P7 → RATIFIED at Task 8 empirical scrape (same run)
- §18.P11 → RATIFIED at Task 13 fixture-harness differential diff

---

## 4. Gate-by-gate evidence

Verbatim from PROGRESS.md Task 14 outputs. All 6 gates GREEN at `9dd79ca`:

**Gate A — build + vet + lint clean:**
```
$ go build ./...        # exit 0
$ go vet ./...          # exit 0
$ golangci-lint run     # exit 0 (0 lines output)
```
(Lint debt addressed during Tasks 1–13: misspell `cancelled`→`canceled` in `check.go`/`extauthz.go`/`extauthz_test.go`/`extauthzhttp.go`; unused `buildDispatchFilter` removed; `//nolint:staticcheck` on deprecated `ar.GetAllowedHeaders()` + `corev3.ApiVersion_V2`.)

**Gate B — race-test pass across all packages:**
```
$ go test -race -count=1 $(go list ./... | grep -v './test/differential')   # exit 0
$ go test -race -count=1 ./test/differential/ -run TestDifferential         # exit 0
```

**Gate C — h2spec 53/53 PASS at ADR-0051 pin:**
```
$ go test -count=1 -run TestH2Spec ./test/conformance/h2spec/
53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec
```
(Phase 18.1 introduces no H2 wire-shape changes; gate unchanged at the ADR-0051 pin.)

**Gate D — 22 fuzzers GREEN at 30s each:**
```
PASS FuzzExtAuthzConfigParse   # the 22nd fuzzer — 3.7M+ execs in 30s, 0 crashes
PASS [21 pre-existing fuzzers] # FuzzPromTextFormat re-ran clean in isolation per phase-17 precedent
=== DONE: 22/22 PASS
```

**Gate E — 21 differential fixtures 0000–0020 PASS:**
```
$ go test -count=1 ./test/differential/ -run 'TestDifferential/000[0-9]'   ok 47.543s
$ go test -count=1 ./test/differential/ -run 'TestDifferential/001[0-9]'   ok 50.170s
$ go test -count=1 ./test/differential/ -run 'TestDifferential/0020'       ok 5.032s
```

**Gate F — BEHAVIOR_CONTRACT.md 6-edit bundle landed:**
```
$ grep -c "envoy.filters.http.ext_authz" docs/envoy-go/BEHAVIOR_CONTRACT.md           → 4
$ grep -c "0020-http-ext-authz-http" docs/envoy-go/BEHAVIOR_CONTRACT.md               → 1
$ grep -c "Phase 18.1 forward-pointer notes" docs/envoy-go/BEHAVIOR_CONTRACT.md       → 6
$ grep -c "Per-route canonical patterns cross-reference" docs/envoy-go/BEHAVIOR_CONTRACT.md → 2
$ grep -c "HTTP outbound auth-check framework note" docs/envoy-go/BEHAVIOR_CONTRACT.md → 3
$ grep -E "77 internal names" docs/envoy-go/BEHAVIOR_CONTRACT.md → "Total: 77 internal names"
$ grep -cE '^\*\*\(xiv\)\*\*' docs/envoy-go/DECISIONS.md → 0  [NO §(xiv) — confirmed]
```

---

## 5. Acceptance checklist — SPEC §15 15-claim verification

Per SPEC §15. All 15 claims verified PASS with citations.

- [x] **Claim 1 — Package shape per ADR-0156.** **PASS.** `internal/filter/http/extauthz/{extauthz.go, check.go, attributes.go, extauthz_test.go, fuzz_test.go, doc.go}` landed; `Decoder: f, Encoder: nil` verified (DECODER-only); 6-base-counter `filterStats` (`ok`/`denied`/`errored`/`disabled`/`failureModeAllowed`/`invalid`) registered unconditionally at `New()` time; `disabled` STRUCTURALLY UNREACHABLE under MVP (publishes 0); `var _ envoyhttp.StreamDecoderFilter = (*filter)(nil)` compile-time assertion present. Evidence: Task 2 + Task 8 PROGRESS entries; Gate A.

- [x] **Claim 2 — Dual-mode envelope per ADR-0157.** **PASS.** `services`-oneof dispatch: `http_service` builds the HTTP-mode `checkFn`; `grpc_service` PARSE-REJECTs; empty `services` PARSE-REJECTs; non-V3 `transport_api_version` PARSE-REJECTs; error-classification boundary per parent §5.P10 (`200`→allow; `401/403`→deny; other→error). Group 1 tests + Task 2 PROGRESS. ADR-0157 §Decision: Accepted; §Decision not yet amended (18.2 amends for gRPC arm activation).

- [x] **Claim 3 — HTTP-outbound auth-check per ADR-0159.** **PASS.** Thin ext_authz-local `httpAuthClient` in `check.go` (disposition (b)); async POST parks the decode dispatch goroutine via `StopIteration` + goroutine + `cb.ContinueDecoding()` on completion (phase-09 async-resume primitive reuse); `OnDestroy` sets `done=true` under `mu` and calls `callCancel()` (cancels the in-flight `client.Do`); the `(a)-vs-(b)` record + oauth2-generalization forward-pointer in ADR-0159 §Decision. Evidence: Task 3 + Task 9 PROGRESS entries; ADR-0159; Group 4 + Group 9 tests.

- [x] **Claim 4 — `AuthorizationRequest` builder per ADR-0160.** **PASS.** `buildAuthRequest` in `attributes.go`: request-side header filtering through `cc.allowedHeaders` minus `cc.disallowedHeaders`; `cc.headersToAdd` appended (pre-compiled at `buildCompiledConfig` time per Task 9 review-fix Option A); path extracted from `:path` pseudo-header (Task 9 review-fix Fix 1); pseudo-headers stripped from the filtered header map (Fix 1 side-discovery); deprecated `AuthorizationRequest.allowed_headers` honored-if-present (D6 confirmed). Evidence: Task 4 + Task 9 PROGRESS entries; Group 3 tests.

- [x] **Claim 5 — Header-mutation discipline per ADR-0161.** **PASS.** Allow-path: `upstreamSet` (overwrite) via `applyUpstreamMutations`; `upstreamApp` (append). Deny-path: `allowed_client_headers`-filtered headers + `content-type: text/plain` fallback + decision-headers-first ordering. `validate_mutations` gating → `dispInvalid` → `invalid` counter + error posture. `allowed_client_headers_on_success` DEFERRED (divergence-window documented at BEHAVIOR_CONTRACT §13.4). Evidence: Task 5 PROGRESS entry; Group 8 tests; §18.P11 Task 13 closure.

- [x] **Claim 6 — Request-body inclusion per ADR-0162.** **PASS.** `with_request_body` materialized via ADR-0128 `DecodeData` accumulation; `allow_partial_message:false` over-limit → `SendLocalReply(413, "Payload Too Large", {connection: close})`, auth NOT called, NO counter increments; `allow_partial_message:true` → truncated prefix sent; `pack_as_bytes` parsed and stored (HTTP-mode no-op; 18.2 consumes). Evidence: Task 6 PROGRESS entry; Group 6 tests; fixture scenario 5.

- [x] **Claim 7 — Per-route per ADR-0163.** **PASS.** 5th-canonical REUSE (NO ADR-0125 `**(xiv)**` amendment — confirmed); `disabled` arm PGV `const: true` (PARSE-REJECT `disabled: false`); `override` oneof PGV-required (PARSE-REJECT empty); `check_settings` narrower-override merge; SHARED-stats (no per-route `*filterStats`); per-route `disabled: true` → NO counter increments (fixture scenario 6 confirms). Evidence: Task 7 + Task 8 PROGRESS entries; Group 7 tests; fixture scenarios 6 + 7.

- [x] **Claim 8 — Deny + error wire shape per §4 + parent §5.P10/§5.P11.** **PASS.** Deny → `SendLocalReply` with auth service's status (401/403) + verbatim body + `allowed_client_headers`-filtered headers + `text/plain` fallback; `content-length` synthesized by ADR-0085; `ContinueDecoding()` called after `SendLocalReply` (Task 13 fix). Error + `failure_mode_allow:false` → `status_on_error` (default 403) + empty body + `ContinueDecoding()` (Task 13 fix). Error + `failure_mode_allow:true` → `HeaderContinue` + optional `x-envoy-auth-failure-mode-allowed: true` + both `errored` AND `failureModeAllowed` increment. Evidence: Task 9 + Task 13 PROGRESS entries; Group 5 tests; fixture scenarios 2/3/4.

- [x] **Claim 9 — §11 empirical pins.** **PASS.** All 13 parent §5 pins resolved IN-SESSION per ADR-0004. 18.1-load-bearing pins reflected in SPEC §4/§5/§6/§7. 3 RATIFIED-PENDING pins CLOSED: §18.P6 + §18.P7 at Task 8 (empirical scrape); §18.P11 at Task 13 (fixture-harness differential diff). See §3 above. Evidence: parent SPEC §5; PROGRESS.md Tasks 8 + 13.

- [x] **Claim 10 — Differential fixture per §7.** **PASS.** `0020-http-ext-authz-http` with 7 scenarios; byte-exact body (allow paths backend-echo; deny path 13-byte "access denied" verbatim); cross-side counter-delta equivalence on 5 reachable counters (ok=3, denied=1, error=2, failure_mode_allowed=1, invalid=0); per-route 5th-canonical exercised on both arms (scenarios 6 + 7); three-listener topology (l_test_a/b/c); 1 NEW test-helper (`test/helpers/extauthzhttp/`). Evidence: Task 10–13 PROGRESS entries; Gate E.

- [x] **Claim 11 — BEHAVIOR_CONTRACT.md populated per Gate F.** **PASS.** §13.1 NEW `### envoy.filters.http.ext_authz` subsection (landing-chronological insertion after jwt_authn@17; D10 fallback recorded); §13.2 stat-table 71→77 (6 new `ext_authz.*` counter rows); §13.3 equivalence-matrix row for 0020; §13.4 Phase 18.1 forward-pointer notes (11 deferral items); §13.6 per-route canonical patterns cross-reference (FIRST §9 REUSE of 5th canonical; NO §(xiv)); §13.7 HTTP outbound auth-check framework note (NOT a new shared primitive; oauth2 forward-pointer). Evidence: Task 14 PROGRESS entry; Gate F.

- [x] **Claim 12 — DECISIONS.md populated per ADR-on-impl convention.** **PASS.** ADR-0156/0157/0159/0160/0161/0162/0163 §Decision + §Consequences bodies landed at their Lands-in-Task anchors (Tasks 2/2/3/4/5/6/7). ADR-0164 (split-application) landed IN FULL at the parent SPEC commit — UNCHANGED. ADR-0125: NO `**(xiv)**` amendment paragraph (`grep -cE '^\*\*(xiv)\*\*'` = 0). Evidence: DECISIONS.md; ADR acceptance-criteria greps at each task.

- [x] **Claim 13 — ROADMAP.md row 18.1 + parent row 18.** **PASS.** Row `18.1` flipped `in-progress → done` at Task 14 commit `9dd79ca` (date `2026-05-15`); summary sharpened with post-impl production LoC (2125) + final 7-ADR roster + ADR-0044-escape-valve-NOT-triggered disposition. Parent row `18` UNCHANGED at `in-progress`; row `18.2` UNCHANGED at `planned` — parent row closes only when 18.2 is `done`. Evidence: Task 14 PROGRESS entry; ROADMAP.md.

- [x] **Claim 14 — All six phase-done gates green at phase-done commit.** **PASS.** All 6 gates GREEN at `9dd79ca` — see §4 above. Evidence: Task 14 PROGRESS entry; Gate A–F outputs.

- [x] **Claim 15 — No master mutation outside the 18.1 squash-merge commit.** **PASS (pending merge).** All phase-18.1 IMPL work landed on the `phase-18.1-ext-authz-http-impl` worktree branch; master tip unchanged at `c4951ae` until the `wt-merge` squash-commit + SHA-fill follow-up. Evidence: `git log --oneline master | head -1` = `c4951ae`.

**Summary:** 15 claims PASS; 0 BLOCKED; 0 DONE_WITH_CONCERNS. The Task 13 `ContinueDecoding()` fix (deny path + error path) is an impl-time behavioral correction surfaced by the first fixture run — it is not a regression against the SPEC, which mandates the `parkDecode`-unblocking semantic; the unit tests simply had wrong expectations (`want 0` → `want 1`) because the initial Group 5 tests were written before the end-to-end wire behaviour was validated.

---

## 6. Divergence-window roster

Per BEHAVIOR_CONTRACT.md §13.4 "Phase 18.1 forward-pointer notes" + SPEC §2/§8:

**(a) `allowed_client_headers_on_success` DEFERRED.** Decode-side-only filter shape (`Encoder: nil`) cannot honor this field — it requires writing to the eventual downstream response on the allow path, which needs an encode-side leg. Field parses (silent-ignore). Documented in `doc.go` + BEHAVIOR_CONTRACT §13.4 item 1 + ADR-0161 §Consequences. Future re-activation: a stash-for-HCM mechanism or an ext_authz encode-side half.

**(b) `response_code_details` NOT emitted.** envoy-go's phase-04 HCM does not surface `response_code_details` to local-reply callers. ext_authz's deny-path `response_code_details` (`ext_authz_denied`) is a documented divergence-window, joint with phase-16 rbac + phase-17 jwt_authn (the cumulative deferred-cluster is now three phases deep). BEHAVIOR_CONTRACT §13.4 item 10.

**(c) Dynamic-metadata family silent-ignored.** `AuthorizationResponse.dynamic_metadata_from_headers`, `filter_metadata`, `enable_dynamic_metadata_ingestion`, and the four `*metadata_context_namespaces` fields all parse but have no runtime effect. ext_authz is the THIRD §9 filter blocked on the dynamic-metadata family primitive (after rbac@16, jwt_authn@17). BEHAVIOR_CONTRACT §13.4 item 9.

**(d) Cluster-scoped `cluster.<upstream>.ext_authz.{ok,denied,error}` triple DEFERRED.** A NEW stat-namespace pattern (charging into the cluster-stat-tree). envoy-go registers only the filter-level 6 counters; no cluster-scoped variant. Documented in BEHAVIOR_CONTRACT §13.2 (`disabled` counter note + §13.4 item 5). `expectations.yaml` does not exercise this triple.

**(e) `disabled` counter STRUCTURALLY UNREACHABLE under MVP.** Increments only via the deferred `filter_enabled` runtime gate (§18.P12 RATIFIED — unset = all-requests-enabled). Counter registered for scrape-stability but publishes 0 for the filter's lifetime under MVP. NOT asserted in fixture counter-delta checks (`expectations.yaml`). Documented in `doc.go` + ADR-0156 §Decision + BEHAVIOR_CONTRACT §13.2.

**(f) `grpc_service` arm PARSE-REJECTs in 18.1.** Not a divergence-window per se — it is an explicit 18.2 forward-pointer documented throughout (ADR-0157 §Decision; `doc.go`; BEHAVIOR_CONTRACT §13.1 "gRPC mode — see phase 18.2"). ADR-0157's §Decision is amended at 18.2 IMPL to activate the arm.

Fixture 0020 exercises divergence (a) implicitly (no `allowed_client_headers_on_success` config set; the field would be silent-ignored); divergences (b)–(e) are not exercised by the fixture config. `expectations.yaml` carries explicit allow-list comments for each deferred item.

---

## 7. Framework-delta impact + cross-phase reuse

Phase 18.1 introduces **ONE new framework primitive** and **REUSES four** existing ones:

**ONE NEW primitive — `httpAuthClient` HTTP-outbound auth-check (ADR-0159).** A thin ext_authz-local `httpAuthClient` in `check.go` wrapping `*http.Client` + timeout + `path_prefix`. NOT a shared `internal/httpclient/` package — ADR-0159 disposition (b). Structure mirrors the phase-17 `internal/jwks/Fetcher`'s `http.Client`/timeout discipline but without the cache/async-refresh machinery (the two consumers have structurally different lifecycles; premature to generalize with only two). The natural generalize-to-`internal/httpclient/` trigger is the THIRD outbound-HTTP consumer — a future `oauth2` phase. BEHAVIOR_CONTRACT §13.7 carries the HTTP outbound auth-check framework note.

**REUSE 1 — Phase-09 async-resume primitive.** `StopIteration` returned synchronously from `DecodeHeaders`; a goroutine performs the cancellable outbound call; `cb.ContinueDecoding()` (or deny `SendLocalReply` + `ContinueDecoding()`) on completion. ext_authz is the load-bearing re-consumer for a per-request *outbound* call (the phase-09 fault filter used it for a time-delayed response). The per-request cancellable `context.Context` (cancelled at `OnDestroy`) is the FIRST §9 row's per-request outbound-call cancellation discipline.

**REUSE 2 — Phase-13 ADR-0128 decode-side body-buffering primitive.** `with_request_body` materializes the request body via ADR-0128 `DecodeData` accumulation. ext_authz is the THIRD ADR-0128 consumer (after phase-13 buffer + phase-15 bandwidth_limit) and the FIRST to consume it for *outbound transmission* of the body (the body becomes the auth POST payload).

**REUSE 3 — Phase-17 ADR-0150 `internal/jwks/Fetcher` outbound-HTTP structure composed-against.** The thin `httpAuthClient` is structurally composed-against (NOT an import or a reuse of code from) the phase-17 JWKS fetcher's `http.Client`/timeout discipline. The composition-without-import decision is captured in ADR-0159 §Decision.

**REUSE 4 — ADR-0085 `SendLocalReply` framework primitive.** Deny-path + error-path + over-limit-413 path all use `cb.SendLocalReply(status, body, headers)` per ADR-0085. `content-length` is synthesized by the existing framework primitive. ext_authz is the most header-mutation-heavy `SendLocalReply` consumer to date (the deny path builds a filtered header set from the auth service's response and threads it through the `headerKVToOrderedHeaders` converter).

**No new ADR-0125 canonical.** ADR-0163's explicit no-amendment 5th-canonical-REUSE classification is the FIRST §9 row since phase-13 (which introduced the 5th canonical) to reuse it. The ADR-0125 roster stays at 8 entries through phase 18.1.

---

## 8. Test counts + verification surface

**Unit tests at phase-done (HEAD `6175ec5`):**
- `internal/filter/http/extauthz/`: **~184 PASS + 0 SKIP** (`extauthz_test.go` 9 test groups; `fuzz_test.go` `FuzzExtAuthzConfigParse` + 22-seed corpus).
- `test/helpers/extauthzhttp/`: **7 tests** (`extauthzhttp_test.go` — start/addr/script/path-method-dispatch/body-inspect/stop-idempotent/concurrent-client).

**Differential fixtures:** 21/21 PASS (0000–0020) at Gate E. Fixture 0020 PASS: `TestDifferential/0020-http-ext-authz-http` (1.91s inner timing at Task 13; 5.0s at Gate E standalone run).

**Fuzzers:** 22/22 PASS at Gate D @ 30s each. `FuzzExtAuthzConfigParse` is the 22nd fuzzer (3.7M+ executions in 30s, 0 crashes, 22 corpus seeds).

**h2spec:** 53/53 at ADR-0051 pin (Gate C).

**Production LoC:** **2125** — `extauthz.go` 1087 + `check.go` 411 + `attributes.go` 435 + `doc.go` 192. Test-helper `extauthzhttp.go` ~155 + `doc.go` ~25 = ~180 LoC. The production LoC exceeds the PLAN estimate range (1053–1513) due to the deeper-than-expected `DecodeHeaders`/`DecodeData` dispatch body + the multi-helper async-resume infrastructure; this is within the soft ADR-0045 threshold and does not trigger a split (the phase IS already the split half, per ADR-0164).

**Documentation deltas:** BEHAVIOR_CONTRACT.md +~520 LoC (6-edit bundle); DECISIONS.md ADR-0156..ADR-0163 §Decision + §Consequences bodies; PROGRESS.md ~2259 LoC (the 15-task ledger).

---

## 9. Divergence-window + deferred items for future phases

(None are regressions; all are auditable in the ADR-0040 deferral trail)

1. **`grpc_service` arm activation** — lands in **18.2** (the explicit next sub-phase; ADR-0157 §Decision amended at 18.2 IMPL to activate).
2. **`internal/grpcclient/` primitive (ADR-0158)** — lands in 18.2 (brand-new gRPC infrastructure; envoy-go's FIRST `google.golang.org/grpc` usage).
3. **`allowed_client_headers_on_success`** — DEFERRED per §5.P9 (decode-side-only shape; see §6(a) above).
4. **Dynamic-metadata family** (`*metadata_context_namespaces` / `dynamic_metadata_from_headers` / `filter_metadata` / `enable_dynamic_metadata_ingestion`) — DEFERRED; ext_authz is the THIRD §9 filter in the cumulative deferred-cluster (joint with rbac@16 + jwt_authn@17).
5. **Cluster-scoped `cluster.<upstream>.ext_authz.{ok,denied,error}` triple** — DEFERRED; a NEW stat-namespace pattern coupling to the cluster-stat-tree (see §6(d)).
6. **`filter_enabled`/`filter_enabled_metadata`/`deny_at_disable`** — DEFERRED (Runtime family); `disabled` counter activates when these are consumed.
7. **`response_code_details` emission** (`ext_authz_denied`) — DEFERRED; joint closure with rbac@16 + jwt_authn@17 (see §6(b)).
8. **`CheckSettings.context_extensions` HTTP-mode no-op** — field parses; HTTP-mode has no effect; 18.2 consumes it for gRPC `AttributeContext.context_extensions`.
9. **`allowed_client_headers_on_success` (already in §6(a))** — stash-for-HCM mechanism OR ext_authz encode-side half.
10. **Access-log integration** (`%EXT_AUTHZ_*%`-style formatters) — DEFERRED; access-log-extension framework (joint with rbac@16 + jwt_authn@17).
11. **`internal/httpclient/` generalization** — DEFERRED to when the THIRD outbound-HTTP consumer (likely `oauth2`) is onboarded; ADR-0159 forward-pointer.

---

## 10. Notable lessons / surprises

**`SendLocalReply` requires a subsequent `ContinueDecoding()` call to unblock `parkDecode` (Task 13 discovery).** The phase-09 fault filter is the canonical precedent (`fault.go:321-324`: `SendLocalReply` + `ContinueDecoding()` in sequence). The ext_authz `applyDisposition`/`applyErrorPosture` authors missed this for the deny and error→`status_on_error` paths — `status=0` connection errors on scenarios 2 and 3 on the first fixture run were the symptom. **Lesson:** any new filter using `SendLocalReply` from an async goroutine MUST follow the fault precedent; adding a code-review checklist item or a doc comment in `envoyhttp` at the `SendLocalReply` callsite documentation may prevent future repeats.

**`failure_mode_allow` is a TOP-LEVEL `ExtAuthz` field — per-route `CheckSettings` cannot override it (Task 11 topology reorg).** The original driver assumed scenarios 3 and 4 (distinct `failure_mode_allow` values) could be split via per-route ext_authz config overrides. Investigation confirmed that `CheckSettings` carries only `context_extensions`, `disable_request_body_buffering`, `with_request_body`, and `service_override` — none of which override `failure_mode_allow`. The fixture required a three-listener topology (l_test_a/b/c), each with a distinct listener-level `ExtAuthz` config. **Lesson:** when differentiating failure-mode scenarios in a fixture, check whether the differentiating field is per-route-overridable BEFORE designing the driver topology; `failure_mode_allow` is listener-scoped.

**Task 4 `buildAuthRequest` call-site placement — the PLAN wording was imprecise (Task 4 review-fix Option B).** The PLAN said "wire the real `buildAuthRequest` into `check.go`'s `checkFn` closure." This is architecturally unsound: the closure is mode-agnostic and runs at config-load time without the per-stream filter state needed to do header filtering. The correct call site is `DecodeHeaders` (Task 9), which has access to the resolved `activeRC`, the client request headers, and the buffered body. The review-fix recorded the justified deviation in ADR-0160 §Decision (vii). **Lesson:** PLAN descriptions of inter-task wiring are sometimes approximate; the implementer must verify the architectural fit before mechanically following the wording.

**Task 9 `path=""` and `headers_to_add` dropped bugs (Task 9 review-fix Fixes 1 + 2).** Two behavioral gaps in `dispatchOutboundCheck`: (a) the `:path` pseudo-header was not extracted, so the auth service received every request at `baseURL + pathPrefix` regardless of the actual client path; (b) `headersToAdd` was read from `hs *ext_authzv3.HttpService` which was `nil` at call time (not yet pre-compiled). Both would have been caught by Task 13's first fixture run; catching them at Task 9 review time de-risked the fixture phase. **Lesson:** the `buildAuthRequest` call-site must explicitly assert it is receiving a non-empty path AND that all pre-compiled fields are in `cc`, not in a live proto reference that may be nil at call time.

**The fixture scrape confirmed `lookupExtAuthzCounter` summation across three `hcm_local_{a,b,c}` labels works correctly.** The Task 12 concern (three listeners with distinct stat_prefix values creating three separate Prometheus label permutations) was resolved at Task 13 — the helper sums all matching metric lines regardless of label value. The total per-counter assertions (ok=3, denied=1, error=2, failure_mode_allowed=1) aggregate correctly across the multi-listener topology. **Lesson:** multi-listener fixtures with distinct stat_prefix values produce multiple label permutations in the Prometheus scrape; the counter-delta helper must not assume a unique label value per metric name.

**ADR-0044 escape-valve was NOT triggered — 0 impl-time-unanticipated ADRs.** The PLAN held ADR-0165 in reserve for the async-resume-after-`OnDestroy` race guard (planner-time decision D4). The `mu`/`done` guard + `context.WithCancel` sufficed; `TestOnDestroy_ResumeAfterDestroy_NoCallback` passes under `-race` confirming the guard. Phase-17 also had 0 unanticipated ADRs — two consecutive phases with the escape-valve unused. ADR-0165 remains the next-free ADR for phase 18.2.

---

## 11. Parent-rollup note

**Parent row `18` stays `in-progress`.** Per the parent SPEC §2.1 closure pattern ("the parent row `18` flips `in-progress → done` AT THE SAME phase-done commit as `18.2`'s phase-done — mirroring the 05/06/07/08 closure pattern"): row `18` advances to `done` only when phase 18.2 ships and its `done` flip is committed. Row `18.2` is currently `planned`; the next session is the phase-18.2 SPEC authoring session (`superpowers:brainstorming` scoped to SPEC authoring, per STATE.md `next-skill` = `superpowers:brainstorming`).

---

## 12. Sign-off

Phase 18.1 is **APPROVED and ready for master squash-merge via `wt-merge`** per the project memory `feedback_git_worktrees.md` + ADR-0003 worktree-isolation discipline. All 6 phase-done gates GREEN at HEAD `6175ec5` (verification sweep at `9dd79ca`); all 15 SPEC §15 acceptance claims verified PASS (0 BLOCKED, 0 DONE_WITH_CONCERNS); 7 SPEC-anticipated ADRs ADR-0156/0157/0159/0160/0161/0162/0163 landed cleanly; NO ADR-0125 `**(xiv)**` amendment paragraph; ADR-0044 escape-valve NOT triggered (ADR-0165 NOT authored; next-free preserved); 21 differential fixtures + 22 fuzzers GREEN; h2spec 53/53 at ADR-0051 pin; BEHAVIOR_CONTRACT 6-edit bundle landed; ROADMAP row 18.1 `done` (2026-05-15); STATE.md at lifecycle-state-4 (`phase 18.1 done; phase 18.2 SPEC pending`; next-free ADR: ADR-0165). Parent row `18` stays `in-progress` pending phase 18.2.

Phase-18.1 task chain summary: **15 tasks** at worktree HEAD including Task 9 review-fix (Fix 1: path propagation bug; Fix 2: `headers_to_add` dropped; plus 5 additional fixes), Task 11 review-fix (multi-listener topology for `failure_mode_allow` split), Task 13 critical fix (`ContinueDecoding()` after `SendLocalReply`), and Task 14 review-fix (STATE.md `next-skill` correction). Phase-done six-gate verification at `9dd79ca`; phase-closed at this Task 15 commit. The last-commit SHA-fill is deferred to the post-`wt-merge` master-side follow-up per the phase-09..17 close pattern. The next session is the phase-18.2 SPEC authoring session (`superpowers:brainstorming`).

**End of phase 18.1 review.**
