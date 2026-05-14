# Phase 17 — Code review (REVIEW.md)

**Phase id:** `17` (TENTH §9 HTTP-filters family-row to land per ADR-0106; SECOND CONSECUTIVE §9 row — after phase-16 rbac — to introduce **TWO** new framework primitives simultaneously, here `internal/jwks/` per ADR-0150 + `internal/jwt/` per ADR-0151; FIRST §9 row to use the **8th canonical per-route pattern** — string-reference-delegation — per the ADR-0125 §(xiii) amendment; FIRST §9 row to land an RFC 6750-conformant Bearer-token challenge deny-path; FOURTH §9 row to ship pure decode-side after csrf + buffer + rbac; LARGEST single-row §9 production-LoC count to date at ~3882 LoC; ADR roster of 8 ADRs ADR-0148..ADR-0155 ties phase-16's record)
**Slug:** `17-http-filter-jwt-authn`
**Branch under review:** `phase-17-http-filter-jwt-authn-impl`
**Range:** branch tip `421c0c1` (Task 14 phase-done at this SHA; this REVIEW.md is the final Task 15 commit). 14 task-landing commits (Tasks 1–14) + 6 in-task follow-up commits (Task 2 / 4 / 5 / 6 / 7 / 8 follow-ups) = 20 commits at worktree HEAD; phase-done six-gate verification at `421c0c1`. Lifecycle-state advance state-3 → state-4 landed at the Task 14 STATE.md edit; the last-commit SHA-fill is deferred to the post-`wt-merge` follow-up per the phase-09..16 close pattern.
**Parent ROADMAP row:** `17 http-filter-jwt-authn` flipped `in-progress → done` at Task 14 commit `421c0c1` (date `2026-05-14`).
**Reviewer method:** Inline authoring by the implementing session — this IMPL session cold-started mid-phase (Tasks 1–12 + the in-task follow-ups were already committed in the worktree at session entry; the session resumed at Task 13's in-progress state, completed Task 13 + Task 14 + this Task 15, and made the Task-13 empirical refinements to the filter package). Inputs: SPEC §15 15-claim acceptance checklist + PLAN's 15-task structure + the branch diff + PROGRESS.md per-task entries (Tasks 1–14 with verbatim outputs) + DECISIONS.md ADR-0148..ADR-0155 + ADR-0125 §(xiii) amendment paragraph + BEHAVIOR_CONTRACT §13.1-§13.8 6-edit bundle + phase-16 REVIEW.md structural template.
**Six-gate state at HEAD:** all green per Task 14's verification sweep — outputs reproduced verbatim in §4 below.

This review covers the full phase-17 surface: the `internal/filter/http/jwtauthn/` package (`doc.go` 185 + `jwtauthn.go` 845 + `evaluator.go` 662 + `provider.go` 707 = 2399 LoC production; `jwtauthn_test.go` + `fuzz_test.go` test surface); the NEW top-level `internal/jwks/` package (`doc.go` 77 + `jwks.go` 626 = 703 LoC production; `jwks_test.go`); the NEW top-level `internal/jwt/` package (`doc.go` 98 + `jwt.go` 555 = 653 LoC production; `jwt_test.go`); the NEW `test/helpers/jwksbackend/` test-helper (`doc.go` 23 + `jwksbackend.go` 104 = 127 LoC; `jwksbackend_test.go`); boot registration at `cmd/envoy-go/main.go` (+3 LoC; alphabetical-after-header_mutation per ADR-0148); differential fixture `0019-http-jwt-authn` (8 scenarios; `envoy.yaml` + `envoy-go.yaml` + `expectations.yaml` + `README.md` + `inputs/driver.go` + `pki/gen.go`); fixture-runner wiring at `test/differential/fixture/fixture.go` + `test/differential/runner_test.go`; `FuzzJwtAuthnConfigParse` (the 21st fuzzer in the repo); the 8 anticipated ADRs ADR-0148..ADR-0155 + the ADR-0125 §(xiii) amendment paragraph; the BEHAVIOR_CONTRACT 6-edit bundle; the ROADMAP row 17 status flip + STATE.md advance to lifecycle-state-4.

This REVIEW closes phase 17's lifecycle (state-4; the last-commit SHA-fill follow-up on master is the only remaining mechanical step) and is the final task before merge to master.

---

## 1. Phase summary

**APPROVED.** All six phase-done gates are GREEN at HEAD `421c0c1` per the Task 14 verification sweep (§4 below). The implementation faithfully realizes the SPEC across all 15 PLAN tasks. jwt_authn is the TENTH §9 HTTP-filters family-row to ship under ADR-0106 and the SECOND CONSECUTIVE row (after phase-16 rbac) to introduce TWO new framework primitives in a single phase.

The architectural centerpiece is the **two cross-phase-reusable framework primitives landed outside `internal/filter/`**: (i) `internal/jwks/` — an HTTP-outbound JWKS fetcher (`Fetcher` opaque type) with a thread-safe cache, a 5s-before-TTL background refresh via `time.AfterFunc`, and a fixed-1s failed-refetch retry (NOT exponential backoff, per §11.P4 REFUTED); the initial fetch is blocking by default (`fast_listener=false`) and a failure fails listener-load loudly per ADR-0150 §Decision (iii). (ii) `internal/jwt/` — a pure-Go-stdlib JWS/JWT parser + signature verifier + claim validator (`crypto/rsa.VerifyPKCS1v15` + `crypto/ecdsa.Verify` + `encoding/base64.RawURLEncoding`; no third-party JWT library) with a 6-algorithm RS+ES allow-list, an exp→nbf→iss→aud claim-validation order with 60s default clock-skew tolerance, a dot-notation `PayloadClaim` extractor with array-claim rejection, and ~20 canonical error sentinels whose body strings are byte-exact with reference Envoy v1.37.2's deny-path bodies. The filter package consumes both primitives; the primitives are independent of each other.

The filter itself is a DECODER-only (`Encoder: nil`) pre-body request gate evaluated at `DecodeHeaders` time: it captures the request's full URL for the WWW-Authenticate realm, resolves the per-route TPFC (the 8th canonical: `oneof{disabled(bool) | requirement_name(string)}` with string-reference-delegation into the listener-level `requirement_map`), checks the CORS-preflight bypass predicate, resolves the requirement (per-route `requirement_name` runtime-resolve OR listener-level `rules` first-match-wins), runs the 6-variant `JwtRequirement` evaluator, and applies the result — on ALLOW, the 4-step side-effect emit-order (strip → forward_payload_header → claim_to_headers → clear_route_cache); on DENY, an RFC 6750-conformant `SendLocalReply` (401 default, 403 for `JwtAudienceNotAllowed`; canonical jwt_verify_lib body; `Bearer realm="<full-URL>"` + conditional `, error="invalid_token"` for non-JwtMissed). Per-route stats are SHARED with listener-level (the 8th canonical is pure delegation; spawns no new policy-evaluation state).

The differential fixture `0019-http-jwt-authn` is the phase-closing non-vacuous evidence against reference Envoy v1.37.2: 8 scenarios — valid-RS256-RemoteJwks, valid-ES256-LocalJwks, missing-token-deny, expired-token-deny, bad-signature-deny, bypass-cors-preflight, per-route `requirement_name` delegation, per-route `disabled: true`. All 8 PASS at byte-exact body/status/WWW-Authenticate + cross-side counter-delta equivalence on the 4 byte-equivalent counters.

Eight ADRs landed (ADR-0148..ADR-0155), all SPEC-anticipated per ADR-0044's standard discipline (§Context drafts at SPEC commit; §Decision + §Consequences bodies at impl-time anchor tasks). The ADR-0125 §(xiii) amendment paragraph landed at the SPEC commit. **The ADR-0044 escape-valve was NOT triggered** — 0 impl-time-unanticipated ADRs. The PLAN anticipated ~1 (the phase-13/14/16 precedent) with three named candidate surfaces (TLS-trust-store coupling for mTLS-to-JWKS; HCM framework gap for `clear_route_cache`; sync-primitive lift for RemoteJwks-fetch-during-request) — none materialized. Phase-17 ADR roster ends at ADR-0155; next-free advances to ADR-0156.

**One IMPL-session note:** this session cold-started mid-phase. Tasks 1–12 (+ 6 in-task follow-ups) were already committed in the worktree; the session resumed at Task 13's in-progress state. Task 13's end-to-end differential debugging surfaced three filter-package empirical refinements (folded into the Task 13 commit, documented at PROGRESS.md Task 13): (a) `requires_any` most-informative-error selection replacing the Task-6 last-error rule; (b) `compiledRule.matchFn` real route-match compilation (Path + Prefix arms) replacing the Task-2 wildcard placeholder; (c) `jwks_fetch_success` credited at filter-load time rather than per request-time `Get()` — ADR-0154 §Decision (vii) + §Consequences amended in-place to match. The realm divergence (reference Envoy reflects the full request URL into the realm; the harness reaches the two proxies at different addresses) was resolved by pinning a constant `Host` header in the fixture driver.

---

## 2. ADR roster

Each of the eight anticipated ADRs ADR-0148..ADR-0155 + the ADR-0125 §(xiii) in-place amendment, evaluated for whether the §Decision body held up under implementation + fixture exercise:

**ADR-0125 §(xiii) in-place amendment** (NEW **8th canonical** per-route pattern — wrapper proto `PerRouteConfig` with a REQUIRED oneof `requirement_specifier` carrying `disabled` (bool, NOT `Empty`) and `requirement_name` (string; PGV `min_len=1`); the defining feature is STRING-REFERENCE-DELEGATION into the listener-level `requirement_map`; dangling references are RUNTIME-RESOLVED at request time; SHARED-stats; landed at SPEC commit): **VALIDATED.** Fixture 0019 scenario 7 (`requirement_name: "alt-req"` → `requirement_map["alt-req"]` = provider-alt) + scenario 8 (`disabled: true` → wholesale passthrough, no counter increments) exercise the per-route arms. The 3-case `buildCompiledPerRoute` discipline (disabled-true / disabled-false-falls-through / requirement_name) landed at Task 7. ADR-0125's canonical-pattern roster grows from 7 to 8.

**ADR-0148** (`internal/filter/http/jwtauthn/` package shape — single-token directory + DECODER-only `HTTPFilter` value + 7-base-counter `filterStats` registered unconditionally at `New()` time + deny-path wire shape + boot-registration alphabetical-after-header_mutation; Lands-in: Task 2): **VALIDATED.** Single-token directory `jwtauthn/` aligns with the cors/fault/csrf/buffer/compressor/localratelimit/bandwidthlimit/rbac precedent. DECODER-only shape verified — `Encoder: nil`; the filter's full responsibility is the `DecodeHeaders` request gate. 7-counter `filterStats` unconditional allocation verified at Group 12 stats-integration tests. Boot-registration ordering: `... header_mutation → jwtauthn → localratelimit → rbac ...`.

**ADR-0149** (`compiledConfig` shape + 5-of-6 outer-envelope consumed + 13-of-21 JwtProvider consumed + 6-variant JwtRequirement evaluator interface + RS+ES algorithm allow-list + side-effect emit-order + listener-level rules dispatch with inline `requires` BOTH-ARMS-honored + `clock_skew_seconds` HONORED + defensive PGV-mirror; Lands-in: Task 2): **VALIDATED.** 5 outer fields consumed (`providers`/`rules`/`bypass_cors_preflight`/`requirement_map`/`strip_failure_response`); `filter_state_rules` silent-ignored. 13 JwtProvider fields consumed including `clock_skew_seconds` (60s default); 8 silent-ignored. Inline `RequirementRule.requires` honored proto-faithful (REFUTES the BRAINSTORM PARSE-REJECT-deprecated hypothesis). The Task-13 `compileRouteMatch` refinement (Path + Prefix arms; others rejected) replaced the Task-2 placeholder while leaving ADR-0149's `router.NewRouteMatcher`-extraction deferral standing.

**ADR-0150** (HTTP-outbound JWKS fetcher framework primitive at NEW `internal/jwks/` — `Fetcher` + `New` (blocking-or-non-blocking initial fetch per `fast_listener`) + `Get` + `Close` + `JWKSet.Lookup` (pickKeyAlgWithKid) + `ParseJWKSet` + 5s-before-TTL refresh + fixed-1s failed-refetch; Lands-in: Task 3): **VALIDATED.** 703 LoC production across `doc.go` + `jwks.go`; 35 unit tests at `jwks_test.go`. The package lives at `internal/jwks/` (outside `internal/filter/`) explicitly to anchor cross-phase reusability. Refresh-schedule + failed-refetch-fixed-interval + blocking-vs-non-blocking-init + concurrent-Get-and-refresh race-safety + `Close` goroutine-termination all verified at the unit-test surface; the RemoteJwks integration path is exercised end-to-end at fixture 0019 scenarios 1 + 7.

**ADR-0151** (JWS/JWT verifier framework primitive at NEW `internal/jwt/` — `Token` + `Parse` + `VerifySignature` (RS+ES allow-list) + `ValidateClaims` + `PayloadClaim` (dot-notation + array-claim rejection) + ~20 canonical error sentinels + pure-Go stdlib; Lands-in: Task 4): **VALIDATED.** 653 LoC production across `doc.go` + `jwt.go`; 42 unit tests at `jwt_test.go`. Algorithm allow-list RS256/384/512 + ES256/384/512; HS family + EdDSA + `none` + PS family all return `ErrJwtHeaderNotImplementedAlg`. The ~20 error sentinels' body strings are byte-exact with reference Envoy's jwt_verify_lib `getStatusString()` table — confirmed at fixture 0019 deny scenarios 3/4/5 (`Jwt is missing` 14B / `Jwt is expired` 14B / `Jwt verification fails` 22B). The 3 silent-ignored `ValidateOptions` fields (`RequireExpiration` + `MaxLifetime` + `Subjects`) accepted at the API but never enforced per §1.1 amendment 3.

**ADR-0152** (token extraction across all 4 sources + iteration order + first-success-wins + case-sensitivity + URL-decode for params + verbatim for cookies + `isCorsPreflightRequest` predicate; Lands-in: Task 5): **VALIDATED.** All 4 sources implemented at `evaluator.go`; the default Authorization Bearer + `access_token` query param apply only when no explicit per-provider extraction-sources are set. `from_params` case-sensitive + URL-decoded + first-value-only; `from_cookies` case-sensitive + verbatim. Iteration order headers → params → cookies. `isCorsPreflightRequest` (`:method == OPTIONS && origin != "" && access-control-request-method != ""`) verified at Group 9 + fixture 0019 scenario 6.

**ADR-0153** (per-route 8th canonical `oneof{disabled(bool) | requirement_name(string)}` + string-reference-delegation + runtime-resolve discipline + SHARED-stats + ADR-0125 §(xiii) cross-reference; Lands-in: Task 7): **VALIDATED.** `parsePerRoute` PGV-mirror (requirement_specifier required; requirement_name min_len=1) + `buildCompiledPerRoute` 3-case + `resolvePerRouteConfig` lazy-cache (`sync.Map` LoadOrStore, storing the lightweight `*compiledPerRoute` NOT a fresh `*compiledConfig`) all landed at Task 7. The dangling-`requirement_name` runtime-resolve path (403 + `"Failed JWT authentication: Wrong requirement_name: <name>"` + NO WWW-Authenticate) verified at Group 7. SHARED-stats discipline confirmed — no per-route stat allocation.

**ADR-0154** (stat surface — 7 base counters under HCM-rooted SN2-reuse namespace + NO per-provider scaling + NO gauges/histograms + 2 STRUCTURALLY UNREACHABLE counters + SHARED per-route + §11.P7 RATIFIED-PENDING-IMPL-TIME closure; Lands-in: Task 8): **VALIDATED with two in-place Task-13 amendments.** The 7-counter `filterStats` (allowed/denied/cors_preflight_bypassed/jwks_fetch_success/jwks_fetch_failed/jwt_cache_hit/jwt_cache_miss) registered unconditionally; the 2 `jwt_cache_*` counters STRUCTURALLY UNREACHABLE under MVP. **§Decision (vi) amended in-place at Task 13:** the §11.P7 RATIFIED-PENDING pin (deferred from Task 8 to Task 13 per Option B) was CLOSED **RATIFIED** — the fixture-0019 empirical scrape confirmed SN2-reuse verbatim (both reference Envoy v1.37.2 and envoy-go emit `envoy_http_jwt_authn_<counter>{envoy_http_conn_manager_prefix="hcm_local_a"}` identically; NO new SN10 rule; NO new tag-extractor; `baseStatPrefix` unchanged). **§Decision (vii) amended in-place at Task 13:** `jwks_fetch_success` is credited at filter-load time (once per RemoteJwks provider whose initial blocking fetch succeeded) rather than per request-time `Get()` — this matches reference Envoy's load-time-credit semantic (cross-side `jwks_fetch_success` = 2 in fixture 0019) and supersedes the Task-6 per-`Get()` disposition.

**ADR-0155** (deny-path wire shape — 401 default + 403 for `JwtAudienceNotAllowed` + canonical jwt_verify_lib body + WWW-Authenticate Bearer challenge + `strip_failure_response` strips both + `response_code_details` divergence-window + per-route runtime-resolve 403 path; Lands-in: Task 9): **VALIDATED with one Task-13 realm refinement.** The 401/403 dispatch (`mapStatusToHTTPCode`), the canonical-string body, and the conditional `, error="invalid_token"` append (non-JwtMissed) all verified at Group 8 + fixture 0019 scenarios 3/4/5. **Task-13 realm refinement:** the empirical scrape established reference Envoy emits the FULL request URL (`<scheme>://<authority><path>`) as the realm, NOT the bare path; the filter constructs the full-URL realm (authority from `:authority` ⊳ `host`; scheme from `x-forwarded-proto` ⊳ `:scheme` ⊳ `"http"`). The fixture driver pins `Host: jwt-authn.fixture.test` so both proxies observe the identical authority and the realm stays byte-equivalent per-side. `response_code_details` non-emission is a documented divergence-window (envoy-go MVP defers; Envoy emits `jwt_authn_access_denied{<reason>}`).

---

## 3. Empirical pins outcome

All 16 SPEC §11 pins were resolved IN-SESSION at SPEC drafting per ADR-0004; the SPEC §15 claim-8 disposition tally: **6 RATIFIED-AND-EXTENDED + 2 RATIFIED + 1 RATIFIED-AND-REFINED + 1 RATIFIED-PENDING-IMPL-TIME + 3 REFUTED + 2 PARTIAL + 1 REFUTED-WITH-MIRROR-DECISION** (per-pin transcripts at SPEC §11; the SPEC document is the authoritative per-pin reference). The structural design survived intact through all refutations.

**§11.P7 — the SOLE RATIFIED-PENDING-IMPL-TIME pin in phase 17 — CLOSED RATIFIED at Task 13.** The pin was deferred from its planned Task-8 closure to the Task-13 fixture-0019 end-to-end run (Option B per ADR-0154 §Decision (vi); the fixture infrastructure provides the reference-Envoy + envoy-go side-by-side scrape harness verbatim). The Task-13 scrape RATIFIED the SN2-reuse namespace hypothesis verbatim — both proxies emit `envoy_http_jwt_authn_<counter>{envoy_http_conn_manager_prefix="hcm_local_a"}` identically; the release-valve (SN10 flattening rule + `baseStatPrefix` rewrite) was NOT exercised. ADR-0154 §Decision (vi) + §Consequences amended in-place to record the closure.

**Named-pin highlights:** §11.P1 + §11.P2 (deny-path wire shape — RATIFIED-AND-EXTENDED; 401/403 dispatch + canonical body + Bearer challenge); §11.P4 (failed-refetch — REFUTED the exponential-backoff hypothesis; fixed-1s interval); §11.P5 (refresh schedule — RATIFIED-AND-EXTENDED; 5s-before-TTL); §11.P6 (stat surface — REFUTES the BRAINSTORM 8-per-provider-scaling hypothesis; 7 filter-wide counters, NO per-provider scaling); §11.P8 + §11.P12 (per-route SHARED-stats + dangling-reference runtime-resolve — RATIFIED in-SPEC-session); §11.P9 (PGV-mirror surface — PARTIAL/REFRAMED → RATIFIED); §11.P10 (dot-notation claim extraction + array-claim rejection — RATIFIED-AND-EXTENDED); §11.P14 + §11.P15 (token extraction case-sensitivity + URL-decode/verbatim — RATIFIED-AND-EXTENDED / RATIFIED); §11.P16 (6-variant JwtRequirement evaluator — RATIFIED-AND-EXTENDED).

**Task-13 empirical discovery — the `allowed`-counter divergence-window.** Not a §11 pin, but a divergence surfaced at the fixture-0019 stat scrape: reference Envoy increments `allowed` on EVERY request that clears the filter gate, including the CORS-preflight-bypass path and the per-route `disabled: true` passthrough (ref `allowed` = 5 = scenarios {1,2,7} actively-ALLOWED + {6,8} bypassed). envoy-go MVP increments `allowed` ONLY on an active-engine ALLOWED result, per SPEC §3 + §1.1 amendment 5 ("`PerRouteConfig{disabled: true}` → no counter increments"; subj `allowed` = 3 = {1,2,7}). This is SPEC-mandated envoy-go behaviour, NOT a bug — the fixture asserts `allowed` per-side (not cross-side) and the divergence is documented in expectations.yaml + BEHAVIOR_CONTRACT §13.4. **Lesson:** the differential fixture's stat scrape is the canonical place where counter-semantics divergences surface — the SPEC's "increments per request where the active engine result = ALLOWED" wording is precise, but the gap from reference Envoy's broader counting was only visible at the end-to-end scrape; future filters with bypass paths should anticipate a per-side counter-assertion split.

---

## 4. Gate-by-gate evidence

Verbatim from PROGRESS.md Task 14 outputs. All 6 gates green at HEAD `421c0c1`:

**Gate A — build + vet + lint clean:**
```
$ go build ./...        # exit 0
$ go vet ./...          # exit 0
$ golangci-lint run     # exit 0 (0 lines output)
```
(Task 13 cleared pre-existing lint debt from Tasks 2/4/10/12 — gofmt on doc.go/provider.go/jwt_test.go/pki/gen.go + 2 unused symbols + 2 misspells.)

**Gate B — race-test pass across all packages:**
```
$ go test -race -count=1 $(go list ./... | grep -v 'test/differential')   # exit 0; 45 ok packages, 0 FAIL
ok  github.com/esalaine/envoy-go/internal/filter/http/jwtauthn   1.122s
ok  github.com/esalaine/envoy-go/internal/jwks                   2.699s
ok  github.com/esalaine/envoy-go/internal/jwt                    1.153s
ok  github.com/esalaine/envoy-go/test/helpers/jwksbackend        1.015s
$ go test -race -count=1 ./test/differential/ -run TestDifferential       # exit 0
ok  github.com/esalaine/envoy-go/test/differential   57.149s
```
(An earlier full-`./...`-under-`-race` sweep flaked on timing-sensitive differential fixtures — the known phase-15/16-precedent container-startup race exacerbated by race-detector overhead + concurrent Docker contention. Split-sweep re-run came up clean on both halves — the standard phase-16 Gate-B re-run-on-flake discipline.)

**Gate C — h2spec 53/53 PASS at ADR-0051 pin:**
```
$ go test -v -count=1 ./test/conformance/h2spec/
53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec
ok  github.com/esalaine/envoy-go/test/conformance/h2spec   2.566s
```
(Phase 17 introduces no H2 wire-shape changes; the gate is unchanged at the ADR-0051 pin.)

**Gate D — 21 fuzzers green at 30s/each:**
```
$ /tmp/run_fuzz_17.sh
=== FUZZ SUMMARY: 20 passed, 1 failed (of 21) ===
```
`FuzzPromTextFormat` (a pre-existing phase-06.1 fuzzer) reported `--- FAIL: context deadline exceeded` after 25.3M clean executions / 0 new-interesting / NO crash artifact — the known Go-fuzzing coordinator↔worker grace-period flake. Re-run in isolation came up clean:
```
$ go test -run='^$' -fuzz='^FuzzPromTextFormat$' -fuzztime=30s ./internal/stats/
fuzz: elapsed: 30s, execs: 25826601, new interesting: 0 (total: 120)
PASS
ok  github.com/esalaine/envoy-go/internal/stats   30.117s
```
**Gate D: GREEN — 21/21.** The 21st fuzzer `FuzzJwtAuthnConfigParse` passed in the batch run with zero crashes.

**Gate E — 20 differential fixtures 0000-0019 PASS:**
```
$ go test -count=1 ./test/differential/ -run TestDifferential
ok  github.com/esalaine/envoy-go/test/differential   56.678s
```

**Gate F — BEHAVIOR_CONTRACT.md §13.1-§13.8 6-edit bundle landed:**
```
$ grep -c '### envoy.filters.http.jwt_authn' docs/envoy-go/BEHAVIOR_CONTRACT.md      # 2
$ grep -c '## JWKS framework primitive' docs/envoy-go/BEHAVIOR_CONTRACT.md           # 3
$ grep -c '## JWT verifier framework primitive' docs/envoy-go/BEHAVIOR_CONTRACT.md   # 3
$ grep -c '### Phase 17 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md    # 3
$ grep -c '0019-http-jwt-authn' docs/envoy-go/BEHAVIOR_CONTRACT.md                   # 1
$ grep -c 'Total: 71 internal names' docs/envoy-go/BEHAVIOR_CONTRACT.md              # 1
```
All 6 §13 patches landed.

---

## 5. Acceptance checklist — SPEC §15 15-claim verification

Per SPEC §15 lines 2349-2387. All 15 claims verified PASS with citations.

- [x] **Claim 1 — Package shape per ADR-0148.** **PASS.** `internal/filter/http/jwtauthn/{doc.go, jwtauthn.go, evaluator.go, provider.go, jwtauthn_test.go, fuzz_test.go}` landed; DECODER-only (`Encoder: nil`); 7-base-counter `filterStats` registered unconditionally at `New()` time. Evidence: Task 2 + Task 8 PROGRESS entries; Group 12 stats tests.
- [x] **Claim 2 — Field decomposition per ADR-0149 + §1.1 amendments 1-4 + 7.** **PASS.** 5 outer fields consumed + `filter_state_rules` silent-ignored; 13 JwtProvider fields consumed (incl. `clock_skew_seconds`) + 8 silent-ignored; `RequirementRule.requires` honored proto-faithful; 6 JwtRequirement variants honored; defensive PGV-mirror. Evidence: Task 2 PROGRESS entry; ADR-0149.
- [x] **Claim 3 — Framework primitives per ADR-0150 + ADR-0151.** **PASS.** `internal/jwks/` (703 LoC) + `internal/jwt/` (653 LoC) NEW top-level packages; fixed-1s failed-refetch (NOT exponential); ~20 jwt_verify_lib-mirroring error sentinels; RS+ES allow-list. SECOND CONSECUTIVE §9 row to introduce two primitives. Evidence: Task 3 + Task 4 PROGRESS entries; ADR-0150 + ADR-0151.
- [x] **Claim 4 — Token extraction per ADR-0152 + §11.P14 + §11.P15.** **PASS.** All 4 sources; case-sensitive + URL-decoded params; case-sensitive + verbatim cookies; first-value-only multi-value; iteration order headers → params → cookies; first-match-wins. Evidence: Task 5 PROGRESS entry; Group 2 + Group 9 tests.
- [x] **Claim 5 — Per-route discipline per §5 + ADR-0153 + ADR-0125 §(xiii).** **PASS.** 8th canonical `oneof{disabled(bool) | requirement_name(string)}`; string-reference-delegation; `disabled` is `bool` not `Empty`; dangling `requirement_name` RUNTIME-RESOLVED with 403 + error string; ADR-0125 §(xiii) at SPEC commit; roster 7 → 8. Evidence: Task 7 PROGRESS entry; Group 7 tests; fixture 0019 scenarios 7 + 8.
- [x] **Claim 6 — Stat surface per ADR-0154 + §1.1 amendments 9 + 10.** **PASS.** 7 base counters per HCM stat_prefix; NO per-provider scaling; NO gauges/histograms; SN2-reuse namespace RATIFIED at Task 13; SHARED per-route stats; stat-table 64 → 71. Evidence: Task 8 + Task 13 PROGRESS entries; ADR-0154 §Decision (vi) + (vii) Task-13 in-place amendments.
- [x] **Claim 7 — Wire-shape claim per ADR-0155 + §11.P1-§11.P3 + §1.1 amendments 8 + 11 + 12.** **PASS.** Byte-exact body/status/WWW-Authenticate on allow + deny paths; 401 default + 403 for JwtAudienceNotAllowed; canonical jwt_verify_lib body; `Bearer realm="<full-URL>"` + conditional error param; `strip_failure_response` strips both; `response_code_details` divergence-window documented; per-route runtime-resolve 403 path. Evidence: Task 9 + Task 13 PROGRESS entries; Group 8 tests; fixture 0019 scenarios 3/4/5.
- [x] **Claim 8 — §11 empirical pin block.** **PASS.** 16 pins resolved IN-SESSION; disposition tally per SPEC §15 claim 8; 12 §1.1 amendments authored at SPEC time. §11.P7 RATIFIED-PENDING closed RATIFIED at Task 13. Evidence: §3 above; SPEC §11.
- [x] **Claim 9 — Differential fixture per §7.** **PASS.** 8 scenarios; byte-exact body (allow = echo; deny = canonical strings); cross-side counter-delta equivalence on the 4 byte-equivalent counters; per-route 8th-canonical on both arms (7 + 8); RemoteJwks lifecycle (1 + 7); LocalJwks (2); CORS-bypass (6). Evidence: Task 11–13 PROGRESS entries; Gate E.
- [x] **Claim 10 — BEHAVIOR_CONTRACT.md populated per Gate F.** **PASS.** §13.1 jwt_authn subsection (landing-chronological per planner-time decision 14 fallback) + §13.2 64→71 + §13.3 equivalence-matrix row + §13.4 Phase 17 forward-pointer notes + §13.7 NEW JWKS section + §13.8 NEW JWT verifier section. Evidence: Task 14 PROGRESS entry; Gate F.
- [x] **Claim 11 — DECISIONS.md populated per ADR-on-impl convention.** **PASS.** ADR-0148..ADR-0155 §Decision + §Consequences bodies landed at their Lands-in-Task anchors (Task 2/2/3/4/5/7/8/9); ADR-0125 §(xiii) amendment paragraph landed in full at the SPEC commit. ADR-0154 §Decision (vi) + (vii) carry Task-13 in-place amendments. Evidence: DECISIONS.md ADR-0148..ADR-0155.
- [x] **Claim 12 — ROADMAP.md row 17 summary refinement.** **PASS.** Row 17 `in-progress → done` (2026-05-14); summary opening rewritten with post-impl LoC counts + the ADR-0044-escape-valve-NOT-triggered disposition + the §11.P7 RATIFIED closure + the two Task-13 counter divergence-windows. Evidence: Task 14 PROGRESS entry; ROADMAP row 17.
- [x] **Claim 13 — All six phase-done gates green at phase-done commit.** **PASS.** All 6 gates GREEN at `421c0c1` — see §4 above.
- [x] **Claim 14 — TENTH §9 family-row landed per ADR-0106.** **PASS.** jwt_authn is the 10th concrete §9 HTTP-filter (cors / fault / header_mutation / local_ratelimit / csrf / buffer / compressor / bandwidth_limit / rbac / jwt_authn); lands as a single flat ROADMAP row 17. Evidence: ROADMAP row 17; SPEC §10 single-row decision; PLAN §Scope check.
- [x] **Claim 15 — No master mutation outside the phase-17 squash-merge commit.** **PASS (pending merge).** All phase-17 IMPL work landed on the `phase-17-http-filter-jwt-authn-impl` worktree branch (20 commits at HEAD `421c0c1`); master tip is unchanged at `d225bb7` until the `wt-merge` squash-commit + SHA-fill follow-up. The BRAINSTORM / SPEC / PLAN stages landed on their own worktree branches per ADR-0005 §Decision 4.

**Summary:** 15 claims PASS; 0 BLOCKED; 0 DONE_WITH_CONCERNS. The Task-13 in-place amendments to ADR-0154 (§Decision (vi) §11.P7 closure + §Decision (vii) jwks_fetch_success load-time credit) and ADR-0155 (full-URL realm) are SPEC-anticipated impl-time refinements (the SPEC explicitly carried §11.P7 as RATIFIED-PENDING and ADR-0155's realm as `<original_uri>`), not regressions.

---

## 6. Divergence-window roster

Per BEHAVIOR_CONTRACT.md §13.4 "Phase 17 forward-pointer notes" + the PLAN's §1 enumeration:

**(a) `allowed` counter on bypass paths (Task-13 empirical discovery).** Reference Envoy increments `allowed` on CORS-preflight-bypass + per-route-disabled passthrough; envoy-go MVP does not (SPEC §3 + §1.1 amendment 5). Asserted per-side (ref 5 / subj 3), not cross-side. SPEC-mandated, not a bug.

**(b) `jwt_cache_hit` / `jwt_cache_miss` structurally unreachable.** envoy-go MVP silent-ignores `jwt_cache_config` (§8 deferral 8); the validated-JWT LRU cache is never constructed. Both counters registered (71-name stat-table completeness) but never incremented. Reference Envoy's cache is always active.

**(c) `response_code_details` field-emission divergence-window** (§1.1 amendment 11 + §8 deferral 13). envoy-go MVP silent; Envoy emits `jwt_authn_access_denied{<failure_reason_underscored>}`. Joint closure with phase-16 rbac forward-pointer item 8 at a future response-code-details framework phase.

**(d) `filter_state_rules` silent-ignored** (§1.1 amendment 1 + §8 deferral 12). envoy-go has no filter-state primitive. Couples to the filter-state-family phase (joint with phase-16 rbac `Principal_FilterState`).

**(e) dynamic-metadata family silent-ignored** (§8 deferrals 1-4). `payload_in_metadata` + `header_in_metadata` + `failed_status_in_metadata` + `normalize_payload_in_metadata`. Couples to the dynamic-metadata family phase.

**(f) v1.37.x claim-coverage extensions silent-ignored** (§1.1 amendments 2-3 + §8 deferrals 15-17). `subjects` + `require_expiration` + `max_lifetime`. `jwt.ValidateOptions` carries the 3 fields but never enforces them.

**(g) `jwt_cache_config` no-cache** (§8 deferral 8). High-RPS performance divergence — envoy-go re-validates every request; Envoy caches. Same root as (b).

**(h) `clear_route_cache` implicit-on-side-effect trigger** (§8 deferral 18). envoy-go MVP honours only an explicit `clear_route_cache: true`; Envoy also clears when `claim_to_headers` adds ≥1 header OR `payload_in_metadata` is set.

**(i) `strip_failure_response` strips both body AND WWW-Authenticate** (§11.P3). Not a divergence — envoy-go mirrors Envoy verbatim — but a documented operator foot-gun (operators relying on the WWW-Authenticate challenge must leave `strip_failure_response` unset).

Fixture 0019 surfaces (a) + (b) empirically; (c)–(h) are not exercised by the fixture config (the fixture does not set those fields).

---

## 7. Framework-delta impact + cross-phase reuse intent

Phase 17 introduces TWO new framework primitives — the SECOND CONSECUTIVE §9 row to do so (after phase-16's matcher-engine + TLS-principal accessor). Both deltas are explicitly designed for cross-phase reuse and live OUTSIDE `internal/filter/`:

**JWKS fetcher primitive at `internal/jwks/` (ADR-0150).** An HTTP-outbound JWKS fetcher with a thread-safe cache, background refresh, and a retry policy. Future filters consuming outbound-HTTP-from-filter primitives — `ext_authz` HTTP-mode (POST to an external auth service), `oauth2` token-endpoint flows (exchange + refresh) — compose against the `Fetcher` type's `http.Client` + retry-policy structure. The package is the FIRST envoy-go outbound-network-from-filter primitive.

**JWT verifier primitive at `internal/jwt/` (ADR-0151).** A pure-Go-stdlib JWS/JWT parser + signature verifier + claim validator. Future filters consuming JWT semantics — `jwt_claim_router` (routing on claim values), `oauth2` (token validation) — compose against `Parse` + `Token.VerifySignature` + `Token.ValidateClaims` + `Token.PayloadClaim` directly. The ~20 canonical error sentinels are reusable for any filter that needs jwt_verify_lib-compatible failure-reason strings.

**Cross-phase reuse intent is load-bearing.** Both ADR §Decision bodies are explicit about cross-phase reuse — NOT a deferred consideration. The BEHAVIOR_CONTRACT §13.7 + §13.8 NEW top-level sections are the operator-facing anchors. This continues the phase-13/14/15/16 lesson: framework primitives that are explicitly cross-phase-reusable at introduction time enable faster downstream filter onboarding.

---

## 8. Test counts + verification surface

**Unit tests at phase-done (HEAD `421c0c1`):**
- `internal/filter/http/jwtauthn/`: **~138 PASS + 0 SKIP** (`jwtauthn_test.go` 12 test groups; `fuzz_test.go` `FuzzJwtAuthnConfigParse` + 13-seed corpus).
- `internal/jwt/`: **42 tests** (`jwt_test.go` — Parse / VerifySignature RS+ES / ValidateClaims / PayloadClaim / silent-ignored-field coverage).
- `internal/jwks/`: **35 tests** (`jwks_test.go` — blocking/non-blocking init / refresh schedule / failed-refetch / JWKSet.Lookup / ParseJWKSet RSA+EC / Close lifecycle / concurrent-Get race-safety).
- `test/helpers/jwksbackend/`: **6 tests** (`jwksbackend_test.go` — multi-route dispatch / byte-exact serving / 404 / teardown / concurrent-client).

**Differential fixtures:** 20/20 PASS (0000-0019) at Gate E (56.7s wallclock; re-confirmed under `-race` at 57.1s).

**Fuzzers:** 21/21 PASS at Gate D @ 30s each. `FuzzJwtAuthnConfigParse` is the 21st fuzzer; `FuzzPromTextFormat` flaked on the coordinator grace-period and passed clean on isolated re-run.

**h2spec:** 53/53 at ADR-0051 pin (Gate C; 2.6s wallclock).

**Production LoC:** ~3882 — jwtauthn package 2399 (`jwtauthn.go` 845 + `evaluator.go` 662 + `provider.go` 707 + `doc.go` 185); `internal/jwks/` 703 (`jwks.go` 626 + `doc.go` 77); `internal/jwt/` 653 (`jwt.go` 555 + `doc.go` 98); `test/helpers/jwksbackend/` 127 (`jwksbackend.go` 104 + `doc.go` 23). The LARGEST single-row §9 production-LoC count to date — the bulk in the two structurally-isolated framework primitives (1356 LoC across `internal/jwks/` + `internal/jwt/`).

**Documentation deltas:** BEHAVIOR_CONTRACT.md +~520 LoC (6-edit bundle); DECISIONS.md ADR-0148..ADR-0155 §Decision + §Consequences bodies + ADR-0154 Task-13 amendments; PROGRESS.md ~1738 LoC (the 15-task ledger).

---

## 9. Deferred items + open follow-ups

For future phase consideration (none are regressions; all are auditable in the trail):

1. **`router.NewRouteMatcher` extraction (ADR-0149 deferral).** `compiledRule.matchFn` is an interim inline route-match predicate covering the Path + Prefix `path_specifier` arms (the two arms fixture 0019 exercises); SafeRegex / PathSeparatedPrefix / ConnectMatcher + the headers / query_parameters / runtime_fraction / dynamic_metadata / tls_context predicates are REJECTED at compile time. Once the shared `router.NewRouteMatcher` constructor lands, `matchFn` can be retired in favor of the shared matcher type.
2. **`jwt_cache_config` validated-JWT LRU cache DEFERRED** (§8 deferral 8). Couples to a caching-framework phase; the 2 `jwt_cache_*` counters re-activate with no registration-shape change.
3. **`response_code_details` framework primitive DEFERRED** (§1.1 amendment 11 + §8 deferral 13). Joint pickup with phase-16 rbac at a future HCM response-code-details phase.
4. **dynamic-metadata family DEFERRED** (§8 deferrals 1-4). Couples to a dynamic-metadata-emit framework phase (joint with phase-16 rbac `access_log_hint`).
5. **`filter_state_rules` DEFERRED** (§8 deferral 12). Couples to a filter-state-family phase.
6. **v1.37.x claim-coverage extensions DEFERRED** (`subjects` + `require_expiration` + `max_lifetime`; §8 deferrals 15-17). Couples to a claim-coverage-extension phase.
7. **HS / EdDSA / `none` / PS algorithm families DEFERRED** (§8 deferrals 5-7 + planner-time decision 6). `none` is PERMANENT-DEFERRED (security anti-pattern); HS couples to a shared-secret config-surface phase; EdDSA + PS couple to an algorithm-extension phase.
8. **CEL-based dynamic provider selection DEFERRED** (§8). Joint with phase-16 rbac's CEL three-field deferral; couples to a CEL framework phase.
9. **`clear_route_cache` implicit-on-side-effect trigger + exponential-backoff customization DEFERRED** (§8 deferral 18). Couples to an operator-ergonomics phase.
10. **JwtRequirement Set recursion-depth foot-gun.** No parse-time recursion-depth cap on nested `requires_any` / `requires_all` (mirrors Envoy's permissive disposition; mirrors the phase-16 rbac `Principal_Set` / `Permission_Set` foot-gun). Future operator-ergonomics phase MAY add an envoy-go-only depth-cap.
11. **`jwt_authn` access-log operators DEFERRED** (`%JWT_PROVIDER%` + `%JWT_SUBJECT%` + `%JWT_FAILURE_REASON%`; §8 deferral 14). Couples to an access-log-extension phase.
12. **The 6 in-task follow-up commits in the worktree history** (Task 2/4/5/6/7/8 follow-ups) are squashed into the single phase-17 IMPL commit at `wt-merge` time — no action needed, noted for the SHA-fill follow-up's commit-count accounting.

---

## 10. Phase-done lessons learned

**The ADR-0044 escape-valve was NOT triggered — 0 impl-time-unanticipated ADRs.** Phase-13 (ADR-0127-v2), phase-14 (ADR-0134), and phase-16 (ADR-0147) each landed ~1 impl-time-unanticipated ADR; the phase-17 PLAN explicitly held the escape-valve in reserve with three named candidate surfaces (TLS-trust-store coupling for mTLS-to-JWKS; HCM framework gap for `clear_route_cache`; sync-primitive lift for RemoteJwks-fetch-during-request). None materialized — the JWKS endpoint in fixture 0019 is plaintext (no mTLS-to-JWKS), the fixture does not exercise `clear_route_cache`, and `jwks.Fetcher.Get` from the per-stream filter path needed no sync-primitive lift (the `Fetcher`'s internal `sync.Mutex` + the `context.Background()` Get call sufficed). **Lesson:** the ~1-unanticipated-ADR working estimate is an estimate, not a rule — a phase whose framework primitives are cleanly stdlib-composable (here `internal/jwks/` + `internal/jwt/` both build against pure Go stdlib) can land with 0 unanticipated ADRs.

**Resumed-mid-phase IMPL sessions inherit in-progress uncommitted work.** This session cold-started at Task 13's in-progress state — Tasks 1–12 + 6 in-task follow-ups were committed, but Task 13's filter-package empirical refinements + the fixture YAML/driver edits were uncommitted. The session's first job was to assess the uncommitted state (does it build? do tests pass? does the fixture pass?) before continuing. **Lesson:** a resumed IMPL session must run the full build/test/differential sweep BEFORE assuming the uncommitted state is coherent — here the differential was FAILING on the realm divergence, and the fix (Host-header pin) + the AssertStats finalization + the lint-debt cleanup were the actual remaining Task-13 work.

**The differential fixture's stat scrape is the canonical place where counter-semantics divergences surface.** The `allowed`-counter-on-bypass divergence (ref 5 / subj 3) was invisible at the unit-test level — the SPEC's "increments per request where the active engine result = ALLOWED" wording is precise and the committed Group 9 + Group 12 tests assert envoy-go's behaviour correctly — but the gap from reference Envoy's broader counting only surfaced at the fixture-0019 end-to-end scrape. **Lesson:** filters with bypass paths (CORS-preflight, per-route-disabled) should anticipate a per-side counter-assertion split in the differential driver; the cross-side-equivalence assertion is correct only for the counters that ARE byte-equivalent.

**Reference Envoy's WWW-Authenticate realm is the full request URL, and the differential harness reaches the two proxies at different addresses.** The pre-Task-13 driver assumed a bare-path realm; the empirical scrape showed reference Envoy emits `<scheme>://<authority><path>`. Because the harness reaches the reference proxy at `localhost:<mapped-port>` and the subject at `[::]:<port>`, the realm — faithfully reflecting the Host header — diverges per-side by construction. **Lesson:** when a filter reflects request-environment values (Host, port, scheme) into its wire output, the differential driver must pin those values to a constant so both proxies observe the identical input — otherwise the byte stream diverges on environment, not on filter behaviour.

**§11.P7 RATIFIED-PENDING closure deferred from Task 8 to Task 13 worked cleanly.** ADR-0154 §Decision (vi) chose Option B (defer the empirical scrape to the fixture-0019 end-to-end run rather than spin up a one-off probe Envoy at Task 8). The fixture infrastructure provided the reference-Envoy + envoy-go side-by-side scrape harness verbatim; the scrape RATIFIED SN2-reuse with zero release-valve usage. **Lesson:** when a RATIFIED-PENDING pin's closure needs a reference-Envoy scrape AND a later PLAN task already builds a reference-Envoy harness, deferring the closure to that task avoids reinventing the harness — but the deferral must be explicitly recorded in the ADR §Decision (here §Decision (vi)) so the closure is not lost.

---

## 11. Sign-off

Phase 17 is **ready for master squash-merge via `wt-merge`** per the project memory `feedback_git_worktrees.md` + ADR-0003 worktree-isolation discipline. All 6 phase-done gates green at HEAD `421c0c1`; all 15 SPEC §15 acceptance claims verified PASS (0 BLOCKED, 0 DONE_WITH_CONCERNS); 8 SPEC-anticipated ADRs ADR-0148..ADR-0155 landed cleanly + ADR-0125 §(xiii) amendment paragraph at the SPEC commit; ADR-0044 escape-valve NOT triggered (0 impl-time-unanticipated ADRs); 20 differential fixtures + 21 fuzzers green; h2spec 53/53 at ADR-0051 pin; BEHAVIOR_CONTRACT §13.1-§13.8 6-edit bundle landed; ROADMAP row 17 `done` (`2026-05-14`); STATE.md at lifecycle-state-4 (`phase 17 done; awaiting next planning`). next-free ADR advances to ADR-0156.

Phase-17 task chain summary: **15 tasks × 20 commits at worktree HEAD** (14 task-landing commits Tasks 1–14 + 6 in-task follow-up commits + this Task 15 main commit = 21 at the post-commit HEAD). Phase-done six-gate verification at `421c0c1`; phase-closed at this Task 15 commit. The last-commit SHA-fill is deferred to the post-`wt-merge` master-side follow-up per the phase-09..16 close pattern. The next session is a BRAINSTORM session for the next §9 HTTP-filters family-row per `superpowers:brainstorming` cadence (ROADMAP has no row 18 — the next planning session selects the next §9 filter from the ext_authz / ext_proc / oauth2 / lua / wasm / adaptive_concurrency / admission_control / global_ratelimit roster).

**End of phase 17 review.**
