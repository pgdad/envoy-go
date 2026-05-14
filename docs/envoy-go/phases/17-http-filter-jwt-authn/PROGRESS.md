# Phase 17 — PROGRESS

Append-only log. Each task lands one entry. Quote command outputs verbatim. Mirror phase-04..16 PROGRESS.md structure.

- **Phase:** 17 — HTTP filter `envoy.filters.http.jwt_authn`
- **Branch:** `phase-17-http-filter-jwt-authn-impl` (fresh worktree at `.worktrees/phase-17-http-filter-jwt-authn-impl`)
- **Base commit (master tip):** `d225bb7` (phase-17 PLAN SHA-fill follow-up; PLAN squash `a899535`; SPEC squash `9070490`; SPEC SHA-fill `75fdc2d`; BRAINSTORM squash `01315cc`; BRAINSTORM SHA-fill `89aaf1e`)

## Preamble — execution preconditions

All 18 preconditions verified green at cold-start. Worktree branch `phase-17-http-filter-jwt-authn-impl` (fresh worktree at `.worktrees/phase-17-http-filter-jwt-authn-impl`, branched from master tip `d225bb7`). Master tail shows PLAN SHA-fill follow-up at `d225bb7`, PLAN squash at `a899535`, SPEC SHA-fill follow-up at `75fdc2d`, SPEC squash at `9070490`, BRAINSTORM SHA-fill follow-up at `89aaf1e`, BRAINSTORM squash at `01315cc`, preceding phase-16 commits (`66c9ac7` impl SHA-fill / `a0bb191` impl squash / `948f6c4` PLAN SHA-fill / `40f030b` PLAN squash). Go 1.26.2, golangci-lint v1.64.8, Docker client 28.4.0 + server 28.1.1 present. ADR tail at 0155 (the highest §Context-draft anchored at SPEC commit `9070490`; per ADR-0044 ADR-on-impl convention + phase-13/15/16 pattern, the 8 phase-17 ADRs ADR-0148..ADR-0155 are anchored as §Context drafts at SPEC commit; §Decision + §Consequences bodies land at impl-time anchor Tasks 2/2/3/4/5/7/8/9 per the per-ADR table below). ADR-0125 §(xii) amendment paragraph present in DECISIONS.md (verified via `grep -nE '\(xii\)' docs/envoy-go/DECISIONS.md | head -3` returning matches at lines 5881 + 5883 — the phase-16 §(xii) amendment paragraph landed at phase-16 SPEC commit per planner-time decision 14). ADR-0125 §(xiii) amendment paragraph present in DECISIONS.md (verified via `grep -nE '\(xiii\)' docs/envoy-go/DECISIONS.md` returning matches at lines 5889 + 5891 — the phase-17 §(xiii) amendment paragraph **ALREADY LANDED at SPEC commit `9070490`** per phase-13/14/15/16 in-place-amend-at-SPEC precedent; NO PLAN-time or IMPL-time re-anchor required). SPEC at `9070490`; PLAN at `a899535`. `internal/filter/http/jwtauthn/` absent (Task 2 lands). `internal/jwks/` absent (Task 3 lands). `internal/jwt/` absent (Task 4 lands). `test/helpers/jwksbackend/` absent (Task 10 lands). `cmd/envoy-go/main.go` registers 11 `httpReg.Register` calls (`router`, `bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `localratelimit`, `rbac`) at master tip `d225bb7`; `jwtauthn` insertion alphabetical-after-`header_mutation` lands at Task 10. **Note on PLAN precondition 18 wording**: the PLAN spec says "expect `12` matches: router + 10 filters + freeze-related"; the actual `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns `11` because `Freeze()` is a SEPARATE call (`httpReg.Freeze()`) that does not contain the substring `Register` — the 11-Register baseline is consistent with the named filter list (router + 10 filters) and the PLAN's stated guardrail "If 13+, another filter has been added concurrently" remains valid at 11. **JwtAuthentication proto** present in module closure (`go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3 JwtAuthentication` returns the type's exported fields). Reference Envoy image `envoyproxy/envoy:v1.37.2` present (SHA `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`; ADR-0008 pin; unchanged through phase 17). Working tree pristine modulo `?? .wt-parent` (the worktree-parent marker is untracked-but-expected per the worktree spawn convention). All 43 `ok`-packages PASS at `go test -count=1 -short -timeout=300s ./...` with 0 failures; the differential suite passes in `-short` mode (compile-only — the Docker-subprocess scenarios skip under `-short` per `harness_test.go` + `runner_test.go` SKIP markers). **Precondition 11 short-form (per Task 1 spec)**: per the project's "fuzzer-per-package" convention, fuzzers live co-located with their packages under `internal/.../fuzz_test.go` (NOT under `./test/fuzz/`); semantic intent (≥20 fuzzers from phases 02–16) verifies via `grep -rE '^func Fuzz' --include='*.go' | wc -l` returning exactly 20 `Fuzz*` functions. The dedicated `-fuzz=… -fuzztime=30s` runs land at Task 15 phase-done Gate iv per the project's late-task gate convention (mirrors phase-13/14/15/16 PROGRESS Task 1 precedent — skipping the 30s × 20 wallclock cost at Task 1).

## Preamble — anticipated ADRs (per ADR-0044 ADR-on-impl convention; SPEC §10)

The 8 ADRs anticipated by SPEC §10 (ADR-0148..ADR-0155). **§Context drafts ALREADY LANDED at SPEC commit `9070490`** per ADR-0044 ADR-on-impl convention. **§Decision + §Consequences bodies AUTHORED AT IMPL-TIME** per the phase-13/15/16 pattern (UNLIKE phase-14 compressor's SPEC-time-pre-landing — phase-14 was the divergent precedent). **ADR-0125 §(xiii) amendment paragraph ALREADY LANDED IN FULL at SPEC commit `9070490`** per phase-13/14/15/16 in-place-amend-at-SPEC precedent; NO PLAN-time or IMPL-time re-anchor required (UNLIKE phase-16's §(xii) which deferred to Task 10 per planner-time decision 14). Per-ADR Lands-in-task anchors (reproduced verbatim from PLAN §"ADRs introduced by this plan"):

| ADR | Title | Lands-in-task |
|---|---|---|
| ADR-0148 | `internal/filter/http/jwtauthn/` package shape — single-token directory (underscore-stripped per ADR-0114) + DECODER-only `HTTPFilter` value (`Encoder: nil`; 4th §9 row to ship pure decode-side per phase-12 csrf + phase-13 buffer + phase-16 rbac precedent) + 7-base-counter `filterStats` (`allowed` + `cors_preflight_bypassed` + `denied` + `jwks_fetch_success` + `jwks_fetch_failed` + `jwt_cache_hit` + `jwt_cache_miss`; NO gauges; NO histograms) + unconditional counter allocation at `New()` time (mirrors phase-12 csrf + phase-13 buffer SHARED-stats discipline) + deny-path wire shape `SendLocalReply(401-or-403, getStatusString(reason), {www-authenticate: Bearer realm="<original_uri>"<, error="invalid_token">})` + boot-registration ordering (alphabetical-after-`header_mutation` per ADR-0100 §2.2) | Task 2 (package skeleton + types + factory + filterStats declaration) |
| ADR-0149 | `compiledConfig` shape + 5-of-6 outer-field consumed proto-faithful (`filter_state_rules` silent-ignored; proto has 6 fields per `[#next-free-field: 7]`) + 13-of-21 JwtProvider consumed (`subjects` + `require_expiration` + `max_lifetime` v1.37.x extensions silent-ignored; `clock_skew_seconds` HONORED) + 6-variant JwtRequirement evaluator (`provider_name` + `provider_and_audiences` + `requires_any` + `requires_all` + `allow_missing` + `allow_missing_or_failed`) + RS+ES algorithm allow-list (6 algorithms: RS256/384/512 + ES256/384/512) + side-effect emit-order (strip → forward_payload_header → claim_to_headers → clear_route_cache) + listener-level rules dispatch (first-match-wins; inline `requires` + named `requirement_name` BOTH HONORED; listener-level `RequirementRule.requirement_name` IS parse-reject at `buildCompiledRule` if name not in `requirement_map`) + envoy-go-side defensive PGV-mirror validation (`RemoteJwks.http_uri` REQUIRED; `RequirementRule.match` REQUIRED; `PerRouteConfig.requirement_specifier` REQUIRED; `PerRouteConfig.requirement_name` PGV min_len=1) | Task 2 (buildCompiledConfig + buildCompiledProvider + buildCompiledRequirement + buildCompiledRule + parsePerRoute + resolvePerRouteConfig land here) |
| ADR-0150 | HTTP-outbound JWKS fetcher framework primitive at NEW top-level package `internal/jwks/` (cross-phase reusable; future filters ext_authz HTTP-mode + oauth2 token-endpoint flows compose against the Fetcher type) — `Fetcher` opaque type wrapping URI + cache duration + AsyncFetch + RetryPolicy + `New(uri, cacheDuration, asyncFetch, retryPolicy)` constructor (blocking-or-non-blocking initial fetch per `fast_listener`; blocking-mode initial-fetch failure returns error from `New()`) + `Get(ctx)` returns cached JWKSet or `ErrJwksNotReady` + `Close()` lifecycle (terminates background refresh goroutine) + refresh schedule **5s-before-TTL** via `time.AfterFunc` (`RefetchBeforeExpiredSec=5s`; default 10-min cache duration) + failed-refetch **FIXED-INTERVAL 1s** (`DefaultRefetchAfterFailedSec=1s`; configurable via `JwksAsyncFetch.failed_refetch_duration`) + inner-HTTP RetryPolicy honored via NumRetries + BaseInterval + MaxInterval + `JWKSet.Lookup(kid, alg)` Envoy's `pickKeyAlgWithKid` logic + `ParseJWKSet(raw)` RFC 7517 §5 JWK Set parsing with RSA + EC key type support | Task 3 (NEW package + Fetcher + AsyncFetch + RetryPolicy + JWKSet + JWKSet.Lookup + ParseJWKSet + refresh-timer + failed-refetch + Close lifecycle) |
| ADR-0151 | JWT verifier framework primitive at NEW top-level package `internal/jwt/` (cross-phase reusable) — `Token` type with `RawHeader` + `RawPayload` + `RawSignature` + parsed Header + Payload maps + Alg + Kid + `Parse(raw)` 3-part JWT structure with `JwtBadFormat` rejection + `Token.VerifySignature(key, alg)` RS+ES algorithm allow-list (PARSE-REJECT unsupported algs via `ErrJwtHeaderNotImplementedAlg`; PS family + HS family + EdDSA + `none` all rejected) + `Token.ValidateClaims(opts)` exp + nbf + iss + aud checks with clock-skew tolerance (default 60s) + `Token.PayloadClaim(path)` dot-notation extractor with array-claim rejection + 3 silent-ignored ValidateOptions fields (`RequireExpiration` + `MaxLifetime` + `Subjects`) + ~20 canonical error sentinels mirroring jwt_verify_lib status codes + pure-Go stdlib (`crypto/rsa.VerifyPKCS1v15` + `crypto/ecdsa.Verify` + `encoding/base64.RawURLEncoding`) | Task 4 (NEW package + Token + Parse + VerifySignature + ValidateClaims + PayloadClaim + ValidateOptions + ~20 error sentinels + RS+ES algorithm dispatch via crypto/rsa + crypto/ecdsa) |
| ADR-0152 | Token extraction across all 4 sources — default Authorization Bearer (when no explicit per-provider extraction-sources set) + default access_token query param (when no explicit extraction-sources set; case-sensitive lookup) + configured from_headers (declared order; value_prefix substring-search via `strings.Index`) + configured from_params (case-sensitive exact name match; URL-decode via `url.ParseQuery`; first-value-only multi-value handling) + configured from_cookies (case-sensitive exact name match; cookie value verbatim NO URL-decode) + iteration order matches Envoy extractor.cc (headers first → params → cookies) + first-success-wins discipline + empty-extraction = `JwtMissed` failure-reason mapping to `jwt.ErrJwtMissed` + `isCorsPreflightRequest` predicate (`:method == OPTIONS && origin != "" && access-control-request-method != ""`) | Task 5 (extractTokens + parseQueryParam + parseCookies + isCorsPreflightRequest + stripNonBase64URLChars helpers + Group 2 tests) |
| ADR-0153 | Per-route 8th canonical pattern `oneof{disabled(bool) | requirement_name(string)}` (REFUTES BRAINSTORM `disabled(Empty)` hypothesis; PerRouteConfig.Disabled is varint bool NOT marker-message Empty) + delegation via listener-level requirement_map + dangling-reference RUNTIME-RESOLVE at request time (REFUTES BRAINSTORM parse-reject hypothesis; mirrors Envoy filter_config.cc `findPerRouteVerifier`; on miss emits `SendLocalReply(403, "Failed JWT authentication: Wrong requirement_name: <name>", nil)` — status 403 NOT 401; NO WWW-Authenticate header; body wraps error string) + per-route stats SHARED with listener-level (NO per-route stat_prefix; pure delegation; mirrors phase-12 csrf + phase-13 buffer + phase-14 compressor SHARED-stats discipline; DIVERGES from phase-11 / phase-15 / phase-16 INDEPENDENT) + ADR-0125 §(xiii) amendment paragraph cross-reference + per-route 3 cases at `parsePerRoute`: (a) `disabled: true`; (b) `disabled: false` falls through to listener-level rules; (c) `requirement_name: "<name>"` runtime-resolved | Task 7 (parsePerRoute + resolvePerRouteConfig + buildCompiledPerRoute + 3 cases + per-route runtime-resolve error path + Group 7 tests) |
| ADR-0154 | Stat surface 7 base counters per HCM stat_prefix scope + NO per-provider scaling (REFUTES BRAINSTORM 8-per-provider-scaling hypothesis — Envoy emits filter-wide counters; multiple providers contribute to same allowed/denied) + NO gauges + NO histograms + canonical naming `cors_preflight_bypassed` (REFUTES BRAINSTORM `bypassed_cors_preflight` hypothesis) + HCM-rooted SN2-reuse namespace `http.<HCM_stat_prefix>.jwt_authn.<counter>` (RATIFIED-PENDING-IMPL-TIME-EMPIRICAL-SCRAPE at Task 8 per phase-16 §10 lesson (c); SOLE Task-8 RATIFIED-PENDING closure pin in phase-17; if scrape divergent, amend ADR-0154 in-place per planner-time decision 10 + phase-13 ADR-0127-v2 in-place-amendment precedent) + per-route stats SHARED + unconditional non-lazy counter allocation at `New()` time | Task 8 (newFilterStats + 7-counter registration + Task-8 RATIFIED-PENDING-IMPL-TIME-EMPIRICAL-SCRAPE for §11.P7 + Group 12 tests + ADR-0154 in-place amend disposition if scrape REFUTES SN2-reuse) |
| ADR-0155 | Deny-path wire shape — 401 for most failure-reasons + 403 specifically for `JwtAudienceNotAllowed` (REFINES BRAINSTORM "always 401" hypothesis; mirrors Envoy filter.cc `code = (status == Status::JwtAudienceNotAllowed) ? Http::Code::Forbidden : Http::Code::Unauthorized`) + body = canonical jwt_verify_lib `getStatusString(status)` table (~70-string table; ~10 commonly-hit at runtime) + WWW-Authenticate header per RFC 6750 `Bearer realm="<original_uri>"` + conditional `, error="invalid_token"` append for non-JwtMissed (REFINES BRAINSTORM `realm="<issuer>"` hypothesis — realm uses request URI captured at DecodeHeaders before route mutation) + `strip_failure_response: true` strips both body AND www-authenticate + `response_code_details = "jwt_authn_access_denied{<reason_with_spaces_as_underscores>}"` divergence-window (envoy-go MVP defers field emission; joint closure with phase-16 forward-pointer item 8 at future response-code-details framework phase) + per-route runtime-resolve error case emits 403 + plain body `"Failed JWT authentication: Wrong requirement_name: <name>"` (NO www-authenticate header) + 4-header standard set lowercase wire-form + keep-alive disposition NO `connection: close` | Task 9 (DecodeHeaders body + applyResult + emitDenyResponse + mapStatusToHTTPCode + WWW-Authenticate construction + per-route runtime-resolve 403 path + Group 8 tests) |

**Plus ADR-0125 §(xiii) amendment paragraph** — **ALREADY LANDED IN FULL at SPEC commit `9070490`** per phase-13/14/15/16 in-place-amend-at-SPEC precedent. The amendment paragraph documents: phase-17 jwt_authn is the FIRST row to use the **8th canonical per-route pattern** (oneof{disabled(bool) | requirement_name(string)} with **string-reference-delegation** into a separately-named registry; explicit-disable-bool NOT Empty; runtime-resolve discipline; SHARED-stats discipline). Structurally distinct from all 7 prior canonicals — defining feature is STRING-REFERENCE-DELEGATION (per-route does NOT carry its own filter config; references-by-name into listener-level requirement_map). ADR-0125's canonical-pattern roster grows from 7 to 8. **NO PLAN-time or IMPL-time re-anchor of the §(xiii) paragraph required** — verified at Task 1 cold-start via `grep -nE '\(xiii\)' docs/envoy-go/DECISIONS.md` returning matches at lines 5889 + 5891 + 7700 + 7704 + 7760.

## Preamble — planner-time deferred-decision resolution (per PLAN §"Planner-time deferred-decision resolution")

The fourteen planner-time deferred decisions reproduced verbatim from PLAN.md so this PROGRESS.md is self-contained for any task-N reader:

1. **D1 — `internal/jwks` package location LOCKED at JWKS-specific (NOT generic `internal/httpclient`) per SPEC §12.1.** JWKS-specific scope (JWK Set parsing, kid+alg lookup, refresh-5s-before-TTL schedule) warrants a dedicated package; future outbound-HTTP-needing filters (ext_authz, oauth2) MAY introduce a sibling `internal/httpclient/` OR compose against the JWKS package's lower-level primitives. ADR-0150 §Decision documents. Group 10 + `internal/jwks/jwks_test.go` tests verify the package surface. *Anchored: SPEC §12.1 + §3.1.*

2. **D2 — `internal/jwt` package error sentinel completeness LOCKED at ~20 entries per SPEC §12.2.** Initial error sentinel set covers the most-commonly-hit entries per §11.P1 table; future impl-time additions are non-blocking; cross-phase reusability preserved as long as Parse + VerifySignature + ValidateClaims + PayloadClaim API surface is stable. Group 3 + Group 4 + `internal/jwt/jwt_test.go` tests verify. *Anchored: SPEC §12.2 + §3.2 + §11.P1.*

3. **D3 — RemoteJwks initial-fetch failure behavior LOCKED at fail-loud-at-listener-load per SPEC §12.3.** When `fast_listener: false` (default) AND initial fetch FAILS, `jwks.New()` returns an error → entire JwtAuthentication config fails listener-load. DIVERGES from Envoy's `init_target_->ready()`-on-failure-too pattern; operationally cleaner for envoy-go's static-config-at-boot world. Impl-time alternative `mirror-Envoy-activate-with-error` documented at ADR-0150 §Decision (iii); MAY flip at impl-time if integration testing reveals operator surprise. *Anchored: SPEC §12.3 + §6.10.*

4. **D4 — `allow_missing` iteration across providers LOCKED at requires_any-style first-success-wins per SPEC §12.4.** When `allow_missing` requirement fires AND request carries a token, evaluator iterates across ALL providers; any token extracted via any provider's extraction-source set triggers validation against THAT provider; first-success wins; if all providers fail to validate, deny with the last failure's status; if NO token extracted, missing-OK. Phase-17 MVP simplification mirrors Envoy's permissive disposition. Impl-time may refine per Task-N empirical scrape of authenticator.cc's `allow_missing` iteration logic; ADR-0149 §Decision documents. *Anchored: SPEC §12.4 + §6.8 + §11.P16.*

5. **D5 — Per-route runtime-resolve error wire shape LOCKED at 403 + "Failed JWT authentication: Wrong requirement_name: <name>" + NO WWW-Authenticate header per SPEC §12.5 + §1.1 amendment 6.** Mirrors Envoy filter_config.cc verbatim. Impl-time alternative (500 internal server error) NOT selected — phase-17 MVP MIRRORS Envoy's 403 verbatim per §1.1 amendment 6. ADR-0153 codifies; ADR-0155 cross-references. *Anchored: SPEC §12.5 + §1.1 amendment 6 + §11.P12.*

6. **D6 — RS-vs-PS algorithm family scope LOCKED at RS family ONLY (RS256/384/512) per SPEC §12.6.** PS family (RSASSA-PSS with SHA-256/384/512) DEFERRED via §8 deferral 5 extension; phase-17 MVP scope per BRAINSTORM Q3 = "RS + ES family (six algorithms)" interpreted strictly as RS256/384/512 + ES256/384/512. Impl-time may add PS family if Task-N reveals fixture-needed coverage; ADR-0151 §Decision documents. *Anchored: SPEC §12.6 + §2.3 + §8 deferral 5.*

7. **D7 — JWT validation order — extraction vs evaluation precedence LOCKED at first-matching-rule-wins-with-OR-short-circuit per SPEC §12.7.** When listener-level rules has multiple `rules[]` entries with different `requires` requirements, FIRST-MATCHING rule's requirement applies; within the requirement (if `requires_any`), iteration is OR-with-short-circuit. Impl-time may discover ambiguity if request matches multiple rules' RouteMatch predicates (operator-config-driven ambiguity); per phase-04 router precedent, first-match wins. *Anchored: SPEC §12.7 + §6.8.*

8. **D8 — `forward = false` stripping discipline for default Authorization Bearer LOCKED at strip-entire-header per SPEC §12.8.** When `forward = false` (proto default) AND token was extracted from the default Authorization Bearer (not configured from_headers), strip the ENTIRE `Authorization` header (not just the `Bearer <token>` value). Mirrors Envoy's behavior per JwtProvider.forward proto comment ("the JWT is removed in the request"). Impl-time may refine if Envoy preserves Authorization header presence with non-Bearer parts intact (e.g., dual-auth schemes); phase-17 MVP strips entire header for simplicity. ADR-0149 §Decision documents. *Anchored: SPEC §12.8 + §6.9.*

9. **D9 — `internal/jwks/Fetcher` cleanup at OnDestroy NO-OP per planner-time decision (NEW; surfaces at PLAN-time).** Filter `OnDestroy` does NOT close the fetcher — fetcher is owned by `factoryState.listenerRC.providers[<name>]` and SHARED across all filter instances of the listener; lifetime is listener-lifetime; fetcher closes when listener drains (future graceful-drain integration; phase-17 MVP relies on goroutine-leak-on-restart per HCM lifecycle scope). Group 10 RemoteJwks lifecycle + `TestClose_StopsRefreshGoroutine` verifies the Close path is well-formed for the listener-drain-time invocation. *Anchored: SPEC §6.10 + ADR-0150.*

10. **D10 — Stat-namespace SN2-reuse hypothesis CONFIRMED-PENDING-IMPL-TIME at Task 8 per phase-16 §10 lesson (c) + SPEC §11.P7 RATIFIED-PENDING-IMPL-TIME (NEW; surfaces at PLAN-time).** Task 8 includes the canonical RATIFIED-PENDING closure step (empirical scrape of reference Envoy v1.37.2 stats output for fixture 0019's listener config; verify `http.<HCM>.jwt_authn.<counter>` shape matches SPEC's SN2-reuse hypothesis); if divergent, amend ADR-0154 §Decision in-place at Task 8 per planner-time discipline + phase-13 ADR-0127-v2 in-place-amendment precedent. *Anchored: SPEC §11.P7 + ADR-0154 + phase-16 §10 lesson (c).*

11. **D11 — Fixture 0019 PKI fresh-generation at fixture-load time NOT pre-baked per planner-time decision (NEW; surfaces at PLAN-time).** `init()` at fixture-load generates 3 RSA-2048 + 1 ECDSA-P256 + 1 tampered key freshly via Go stdlib `crypto/rsa.GenerateKey` + `crypto/ecdsa.GenerateKey`; JWK Set JSON serialized from keys at runtime; LocalJwks inline_string content + jwksbackend-served JWK Set bodies BOTH sourced from the same per-test PKI. Mirrors phase-16 mTLS PKI generation discipline. Avoids checked-in private keys in the repo. *Anchored: SPEC §7.3 + ADR-0151 §Consequences.*

12. **D12 — Test-helper `test/helpers/jwksbackend/` location LOCKED at top-level test/helpers/ NOT per-fixture per planner-time decision (NEW; surfaces at PLAN-time).** Mirrors phase-14 `test/helpers/echobackend/` precedent — phase 17 introduces this shared helper at the well-known shared-helper location anticipating future filter fixtures (ext_authz, oauth2) needing JWKS-serving behavior. Not embedded under `test/fixtures/0019-http-jwt-authn/` since the shared-helper anticipation pattern is the cross-phase-reusable shape. *Anchored: SPEC §7.4 + planner-time decision 6 inheritance from phase-14.*

13. **D13 — Counter-delta byte-equivalence assertion convention from phase-13/14/15/16 carried forward per planner-time decision (NEW; surfaces at PLAN-time).** Fixture 0019 driver scrapes `/stats/prometheus` before + after each scenario; computes counter delta; asserts byte-equivalence against reference Envoy's expected delta per scenario in `expectations.yaml`. The 5 active counters (allowed + denied + cors_preflight_bypassed + jwks_fetch_success + jwks_fetch_failed) asserted via `counterModeAggregate` (since SHARED-stats discipline aggregates across listener-level + per-route per ADR-0154); the 2 unreachable counters (jwt_cache_hit + jwt_cache_miss) NOT asserted (STRUCTURALLY UNREACHABLE under MVP per §8 deferral 8). *Anchored: SPEC §7.5 + ADR-0154 + phase-16 ADR-0145 precedent.*

14. **D14 — BEHAVIOR_CONTRACT §13.1 insertion at ALPHABETICAL-AFTER-header_mutation position per SPEC §13.1 + ADR-0100 §2.2 (NOT landing-chronological as phase-16 used) per planner-time decision (NEW; surfaces at PLAN-time).** Phase-16's planner-time decision 19 used landing-chronological insertion per the observed file state; phase-17 reverts to the SPEC §13.1 stated alphabetical convention since the alphabetical insertion point lies BETWEEN existing subsections (`header_mutation` + `local_ratelimit`) not at the end; alphabetical insertion is cleaner here than appending at the file tail. Impl-time verifies BEHAVIOR_CONTRACT.md current state shows subsections roughly alphabetically ordered between `bandwidth_limit` and `rbac` and adjusts insertion accordingly. *Anchored: SPEC §13.1 + ADR-0100 §2.2 + planner-time decision.*

## Task 1 — Execution-precondition check + PROGRESS.md preamble

**Files changed:** `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` (new)
**Commit SHA:** <filled at commit time; capture via `git log -1 --format=%H -- docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` post-commit, or via a follow-up SHA-fill commit per phase-13/14/15/16 precedent>
**Notes:** Created PROGRESS.md; verified all 18 preconditions per PLAN §"Execution preconditions"; phase-17 SPEC + PLAN confirmed present in HEAD; SPEC at `9070490`, PLAN at `a899535`; ADR tail at 0155 (the 8 phase-17 ADRs ADR-0148..ADR-0155 §Context drafts ALREADY landed at SPEC commit `9070490` per ADR-0044 ADR-on-impl convention; §Decision + §Consequences bodies land at impl-time anchor Tasks 2/2/3/4/5/7/8/9 per the per-ADR table — mirroring phase-13/15/16 pattern); `internal/filter/http/jwtauthn/` absent (Task 2 lands); `internal/jwks/` absent (Task 3 lands); `internal/jwt/` absent (Task 4 lands); `test/helpers/jwksbackend/` absent (Task 10 lands). ADR-0125 §(xiii) amendment paragraph ALREADY LANDED IN FULL at SPEC commit `9070490` (`grep -nE '\(xiii\)' docs/envoy-go/DECISIONS.md` returns 5 matches at lines 5889 + 5891 + 7700 + 7704 + 7760); NO further amendment action required at PLAN-time or IMPL-time per phase-13/14/15/16 in-place-amend-at-SPEC precedent (UNLIKE phase-16's §(xii) which deferred to Task 10). No ADR landed in Task 1 (ADR-0044 ADR-on-impl convention). Pre-existing fuzzers (20 fuzzers from phases 02–16 across co-located `fuzz_test.go` files; PLAN's literal `find ./test/fuzz` path does not match envoy-go's per-package co-located fuzzer layout — verbatim count captured under the precondition 11 block below) deferred to Task 15 phase-done Gate iv per PLAN. PLAN-precondition-18-wording note: the PLAN's expected count of `12` `httpReg.Register` matches mismatched the actual count of `11` because `httpReg.Freeze()` does not contain the substring `Register`; the 11-Register baseline is consistent with the named filter list at master `d225bb7` (router + 10 filters: bandwidthlimit, buffer, compressor, cors, csrf, envoygotest, fault, header_mutation, localratelimit, rbac); the PLAN's stated guardrail "If 13+, another filter has been added concurrently" remains valid at 11 — proceed.

**Outputs:**

### Precondition 1 — branch name

```
$ git rev-parse --abbrev-ref HEAD
phase-17-http-filter-jwt-authn-impl
```

### Precondition 2 — master tail

```
$ git log --oneline master | head -10
d225bb7 phase 17 plan follow-up: STATE.md SHA-fill (TBD → a899535 post-squash)
a899535 Squash merge phase-17-http-filter-jwt-authn-plan
75fdc2d phase 17 spec follow-up: STATE.md SHA-fill (TBD → 9070490 post-squash)
9070490 Squash merge phase-17-http-filter-jwt-authn-spec
89aaf1e phase 17 brainstorm follow-up: STATE.md SHA-fill (TBD → 01315cc post-squash)
01315cc Squash merge phase-17-http-filter-jwt-authn-brainstorm
66c9ac7 phase 16 impl follow-up: STATE.md SHA-fill (TBD → a0bb191 post-squash) + state-5 → state-6 advance
a0bb191 Squash merge phase-16-http-filter-rbac-impl
948f6c4 phase 16 plan follow-up: STATE.md SHA-fill (TBD → 40f030b post-squash)
40f030b Squash merge phase-16-http-filter-rbac-plan
```

### Precondition 3 — toolchain (go + golangci-lint + docker)

```
$ go version && golangci-lint version && docker version 2>&1 | head -30
go version go1.26.2 linux/amd64
golangci-lint has version v1.64.8 built with go1.26.2 from (unknown, modified: ?, mod sum: "h1:y5TdeVidMtBGG32zgSC7ZXTFNHrsJkDnpO4ItB3Am+I=") on (unknown)
Client: Docker Engine - Community
 Version:           28.4.0
 API version:       1.49 (downgraded from 1.51)
 Go version:        go1.24.7
 Git commit:        d8eb465
 Built:             Wed Sep  3 20:57:32 2025
 OS/Arch:           linux/amd64
 Context:           desktop-linux

Server: Docker Desktop 4.41.2 (191736)
 Engine:
  Version:          28.1.1
  API version:      1.49 (minimum version 1.24)
  Go version:       go1.23.8
  Git commit:       01f442b
  Built:            Fri Apr 18 09:52:57 2025
  OS/Arch:          linux/amd64
  Experimental:     false
 containerd:
  Version:          1.7.27
  GitCommit:        05044ec0a9a75232cad458027ca83437aae3f4da
 runc:
  Version:          1.2.5
  GitCommit:        v1.2.5-0-g59923ef
 docker-init:
  Version:          0.19.0
  GitCommit:        de40ad0
```

### Precondition 4 — DECISIONS.md ADR tail

```
$ grep '^## ADR-' docs/envoy-go/DECISIONS.md | sed 's/.*ADR-0*\([0-9]*\):.*/\1/' | sort -n | tail -1
155
```

### Precondition 5 — ADR-0125 amendment paragraphs (xii) and (xiii) present

```
$ grep -nE '\(xii\)' docs/envoy-go/DECISIONS.md | head -3
5881:Phase 16 rbac is the FIRST row to use the 7th canonical per-route pattern. The empirical surface verbatim from Envoy v1.37.2 `envoy.extensions.filters.http.rbac.v3.RBACPerRoute`: a wrapper proto with reserved field 1 (legacy) + a single optional sub-message field `rbac` at field 2; the proto comment on the `rbac` field reads `"If absent, RBAC policy will be disabled for this route."`. The structural shape is distinct from the 5th canonical (no oneof; no explicit `disabled` bool) AND from the 6th canonical (a wrapper IS present; the per-route proto is NOT the listener-level proto re-used). ADR-0125's canonical-pattern roster grows from 6 to 7 via in-place amendment paragraph §(xii) (mirrors phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) precedent for in-place ADR amendments):
5883:**(xii)** Phase 16 rbac is the FIRST row to use the **7th canonical per-route pattern**: a wrapper proto (`RBACPerRoute`) with reserved field 1 + a single optional sub-message field (`rbac` at field 2); ABSENCE-of-the-sub-message-field implies disabled-via-proto-comment (per Envoy v1.37.2 proto comment `"If absent, RBAC policy will be disabled for this route."`); PRESENCE-of-the-sub-message-field implies wholesale-override of the listener-level config (mirrors ADR-0073 wholesale-not-merge). Structurally distinct from the 5th canonical (explicit `disabled` bool in oneof; phase-13 + phase-14) and the 6th canonical (bare-message-via-TPFC + code-level-required field; phase-15). The 7th canonical's stat-discipline is INDEPENDENT (per ADR-0145; mirrors phase-11 + phase-15 stateful-override-implies-INDEPENDENT discipline). Future §9 family-rows whose per-route proto follows the same "wrapper-with-reserved-field-and-single-optional-sub-message; absent-means-disabled; presence-means-override" shape compose against this canonical. ADR-0125's canonical-pattern roster grows from 6 to 7.
5889:Phase 17 jwt_authn is the FIRST row to use the 8th canonical per-route pattern. The empirical surface verbatim from Envoy v1.37.2 `envoy.extensions.filters.http.jwt_authn.v3.PerRouteConfig` (`config.pb.go:1595-1679`): a wrapper proto with NO reserved field + a REQUIRED oneof `RequirementSpecifier` (PGV `required = true` per `config.pb.validate.go:2472-2481`) carrying two arms — `disabled` (**bool**; varint at field 1; NOT `Empty` as phase-17 BRAINSTORM §2.7 hypothesized — REFUTED at SPEC time per §1.1 amendment 5) and `requirement_name` (string; bytes at field 2; PGV `min_len=1` per `config.pb.validate.go:2460-2462`). The structural shape is distinct from all 7 prior canonicals — the defining feature is **string-reference-delegation** into a separately-named registry: the per-route proto does NOT embed a filter config; it embeds a string name that resolves at REQUEST TIME (per §1.1 amendment 6 — Envoy filter_config.cc `findPerRouteVerifier()` runtime-resolves; on miss emits 403 + "Failed JWT authentication: Wrong requirement_name: <name>"; envoy-go MIRRORS) against the listener-level `JwtAuthentication.requirement_map`. ADR-0125's canonical-pattern roster grows from 7 to 8 via in-place amendment paragraph §(xiii) (mirrors phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) + phase-16 ADR-0125 §(xii) precedent for in-place ADR amendments at SPEC commit):

$ grep -nE '\(xiii\)' docs/envoy-go/DECISIONS.md
5889:Phase 17 jwt_authn is the FIRST row to use the 8th canonical per-route pattern. The empirical surface verbatim from Envoy v1.37.2 `envoy.extensions.filters.http.jwt_authn.v3.PerRouteConfig` (`config.pb.go:1595-1679`): a wrapper proto with NO reserved field + a REQUIRED oneof `RequirementSpecifier` (PGV `required = true` per `config.pb.validate.go:2472-2481`) carrying two arms — `disabled` (**bool**; varint at field 1; NOT `Empty` as phase-17 BRAINSTORM §2.7 hypothesized — REFUTED at SPEC time per §1.1 amendment 5) and `requirement_name` (string; bytes at field 2; PGV `min_len=1` per `config.pb.validate.go:2460-2462`). The structural shape is distinct from all 7 prior canonicals — the defining feature is **string-reference-delegation** into a separately-named registry: the per-route proto does NOT embed a filter config; it embeds a string name that resolves at REQUEST TIME (per §1.1 amendment 6 — Envoy filter_config.cc `findPerRouteVerifier()` runtime-resolves; on miss emits 403 + "Failed JWT authentication: Wrong requirement_name: <name>"; envoy-go MIRRORS) against the listener-level `JwtAuthentication.requirement_map`. ADR-0125's canonical-pattern roster grows from 7 to 8 via in-place amendment paragraph §(xiii) (mirrors phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) + phase-16 ADR-0125 §(xii) precedent for in-place ADR amendments at SPEC commit):
5891:**(xiii)** Phase 17 jwt_authn is the FIRST row to use the **8th canonical per-route pattern**: a wrapper proto (`PerRouteConfig`) with a REQUIRED oneof `requirement_specifier` containing two arms — `disabled` (**bool**; NOT Empty; the `true` value disables JWT validation on this route; the `false` value indicates the oneof is set to disabled-arm but does not actually disable, falling through to listener-level rules dispatch) and `requirement_name` (string; reference-by-name into the listener-level `JwtAuthentication.requirement_map`). Structurally distinct from all 7 prior canonicals: vs 1st (cors no-per-route) — 8th has explicit per-route surface; vs 2nd (data-only TPFC, cors / fault) — 8th has structural oneof, 2nd is bare-message; vs 3rd (multi-tier all-tier, header_mutation) — 8th uses 3-tier resolution, 3rd evaluates ALL tiers; vs 4th (INDEPENDENT-stats stateful, local_ratelimit) — 8th has SHARED stats (delegation-by-name spawns no new state); vs 5th (disabled-bool + wholesale-override sub-message, buffer + compressor) — 8th's "override" arm is a STRING REFERENCE (NOT a sub-message); both have disabled-as-bool but 5th's other arm is a local sub-message, 8th's other arm is a string-reference into a separate registry; vs 6th (bare-message-via-TPFC + code-level-required, bandwidth_limit) — 8th uses oneof wrapper; vs 7th (wrapper-with-reserved-field + single optional sub-message, absent-implies-disabled, rbac) — 8th has EXPLICIT disable-bool (NOT absence-implies-disabled), AND the "non-disabled" arm is a STRING REFERENCE (NOT a sub-message). The 8th canonical's defining feature: STRING-REFERENCE-DELEGATION into a separately-named registry — per-route does NOT carry its own config; it references-by-name into the listener-level requirement_map (per §1.1 amendment 6 — runtime-resolved at request time; dangling reference yields 403 + error string mirroring Envoy filter_config.cc `findPerRouteVerifier`). The 8th canonical's stat-discipline is SHARED with listener-level (per ADR-0154; mirrors phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125 + phase-14 compressor ADR-0132 SHARED-stats discipline; DIVERGES from phase-11 / phase-15 / phase-16 INDEPENDENT-stats; rationale: pure delegation by name does NOT spawn new policy-evaluation state, so a shared stat namespace is operationally correct). Future §9 family-rows whose per-route proto follows the same "oneof with string-reference-delegation OR explicit-disable-bool" shape compose against this canonical. ADR-0125's canonical-pattern roster grows from 7 to 8.
7700:## ADR-0153: Per-route 8th canonical pattern `oneof{disabled(bool) | requirement_name(string)}` (REFUTES BRAINSTORM `disabled(Empty)` hypothesis per SPEC §1.1 amendment 5 + §11.P9) + delegation via listener-level requirement_map + runtime-resolve at request time for dangling references (REFUTES BRAINSTORM parse-reject hypothesis per §1.1 amendment 6 + §11.P12 — Envoy filter_config.cc `findPerRouteVerifier` runtime-resolves; on miss emits 403 + "Failed JWT authentication: Wrong requirement_name: <name>"; envoy-go MIRRORS) + per-route stats SHARED with listener-level (mirrors phase-12/13/14 SHARED; DIVERGES from phase-11/15/16 INDEPENDENT) + ADR-0125 §(xiii) amendment paragraph at SPEC commit (NEW 8th canonical; ADR-0125 roster grows from 7 to 8)
7704:**Doctrine:** Phase 17 §9 family-row. ADR-0044 ADR-on-impl convention. Co-anchors with ADR-0125 §(xiii) amendment paragraph (in-place amendment lands at SPEC commit) + ADR-0148 + ADR-0149 + ADR-0154.
7760:**ADR-0125 §(xiii) in-place amendment paragraph**: lands at SPEC commit (NOT IMPL commit) per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) + phase-16 ADR-0125 §(xii) in-place-update precedent. The amendment paragraph documents the 8th canonical's defining feature (string-reference-delegation; explicit-disable-bool NOT Empty; runtime-resolve discipline; SHARED-stats discipline) and grows ADR-0125's canonical-pattern roster from 7 to 8. See DECISIONS.md ADR-0125 §(xiii) for the verbatim text.
```

### Precondition 6 — phase-17 SPEC SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/17-http-filter-jwt-authn/SPEC.md
9070490ba98a46d644d26138f347106303ac3719
```

### Precondition 7 — phase-17 PLAN SHA

```
$ git log -1 --format=%H -- docs/envoy-go/phases/17-http-filter-jwt-authn/PLAN.md
a89953527ae12c04c0d49f0a01accdb46b2ea5d3
```

### Precondition 8 — pristine tree (modulo worktree-parent marker)

```
$ git status --porcelain
?? .wt-parent
```

(The `.wt-parent` file is the worktree-spawn parent-tracking marker; untracked-but-expected per the worktree convention; not part of any commit.)

### Precondition 9 — short `go test ./...` pass

```
$ go test -count=1 -short -timeout=300s ./... 2>&1 | grep -cE '^ok'
43

$ go test -count=1 -short -timeout=300s ./... 2>&1 | grep -cE '^(FAIL|---\s+FAIL)'
0

$ go test -count=1 -short -timeout=300s ./... 2>&1 | tail -20
?   	github.com/esalaine/envoy-go/test/fixtures/0009-admin-config-dump/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0010-graceful-drain/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0011-http-fault/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0011-http-fault/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0012-http-header-mutation/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0012-http-header-mutation/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0013-http-local-ratelimit/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0013-http-local-ratelimit/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0014-http-csrf/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0014-http-csrf/driver	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0015-http-buffer/backends	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0015-http-buffer/driver	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0016-http-compressor/inputs	0.005s
?   	github.com/esalaine/envoy-go/test/fixtures/0017-http-bandwidth-limit/inputs	[no test files]
?   	github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/inputs	[no test files]
ok  	github.com/esalaine/envoy-go/test/fixtures/0018-http-rbac/pki	0.006s
ok  	github.com/esalaine/envoy-go/test/helpers	0.013s
ok  	github.com/esalaine/envoy-go/test/helpers/echobackend	0.007s
?   	github.com/esalaine/envoy-go/test/helpers/echobackend/cmd/echobackend	[no test files]
```

(43 packages PASS; 0 FAIL; remaining packages are `[no test files]`. Full output elided for brevity; canonical pass / fail counters captured via `grep -c`.)

### Precondition 10 — differential suite `-short` clean

```
$ go test -count=1 -short -timeout=300s ./test/differential/ -v 2>&1 | tail -22
=== RUN   TestCompareBytes_Equal
--- PASS: TestCompareBytes_Equal (0.00s)
=== RUN   TestCompareBytes_DivergesAtFirstByte
--- PASS: TestCompareBytes_DivergesAtFirstByte (0.00s)
=== RUN   TestCompareBytes_DifferentLengths
--- PASS: TestCompareBytes_DifferentLengths (0.00s)
=== RUN   TestParseEnvoyTarget_PullsTagAndDigest
--- PASS: TestParseEnvoyTarget_PullsTagAndDigest (0.00s)
=== RUN   TestParseEnvoyTarget_RejectsMissingTag
--- PASS: TestParseEnvoyTarget_RejectsMissingTag (0.00s)
=== RUN   TestReferenceProxy_Starts
    harness_test.go:41: differential test; skipped under -short
--- SKIP: TestReferenceProxy_Starts (0.00s)
=== RUN   TestSubjectProxy_StartsAndReports
    harness_test.go:96: subject subprocess test; skipped under -short
--- SKIP: TestSubjectProxy_StartsAndReports (0.00s)
=== RUN   TestDifferential
    runner_test.go:52: differential suite; skipped under -short
--- SKIP: TestDifferential (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/test/differential	0.084s
```

(`-short` mode verifies the differential test infrastructure compiles and the registered fixtures load; Docker-subprocess scenarios skip per `harness_test.go` + `runner_test.go` SKIP markers. The 19 pre-existing fixtures 0000–0018 register and load successfully; full differential runs without `-short` exercise Docker subprocesses and land at Task 15 phase-done Gate iv per project precedent.)

### Precondition 11 — fuzzer count (per-package co-located)

```
$ grep -rE '^func Fuzz' --include='*.go' | wc -l
20
```

(20 fuzzers from phases 02–16 across co-located `fuzz_test.go` files under the per-package "fuzzer-per-package" convention. Phase 17 adds the 21st (`FuzzJwtAuthnConfigParse` per Task 10). The seed-corpus tests already passed under `go test -count=1 -short ./...` (each fuzzer's `f.Add` seed inputs execute as normal subtests, so the no-panic / no-`(nil,nil)` invariants are baseline-verified). The dedicated `-fuzz=… -fuzztime=30s` runs land at Task 15 phase-done Gate iv per project precedent — skipping the 30s × 20 wallclock cost at Task 1.)

### Precondition 12 — Envoy reference image v1.37.2 present

```
$ docker image inspect envoyproxy/envoy:v1.37.2 2>/dev/null | grep -E '"Id":' | head -1
        "Id": "sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd",
```

(SHA matches ADR-0008 pin `sha256:c5e8a68e52f4d4697a9adb280dbe415d77fedf1257e183dcb86205bd438f18bd`; unchanged through phase 17.)

### Precondition 13 — `envoy.extensions.filters.http.jwt_authn.v3` proto present in module closure

```
$ go doc github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3 JwtAuthentication 2>&1 | head -20
package jwt_authnv3 // import "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/jwt_authn/v3"

type JwtAuthentication struct {

	// Map of provider names to JwtProviders.
	//
	// .. code-block:: yaml
	//
	//	providers:
	//	  provider1:
	//	     issuer: issuer1
	//	     audiences:
	//	     - audience1
	//	     - audience2
	//	     remote_jwks:
	//	       http_uri:
	//	         uri: https://example.com/.well-known/jwks.json
	//	         cluster: example_jwks_cluster
	//	         timeout: 1s
	//	   provider2:
```

(No `import path failed` error; `JwtAuthentication` proto type's exported fields visible via `go doc`. No `go mod tidy` required at Task 1 cold-start.)

### Precondition 14 — `internal/filter/http/jwtauthn/` absent

```
$ test ! -d internal/filter/http/jwtauthn && echo "ok: jwtauthn absent"
ok: jwtauthn absent
```

### Precondition 15 — `internal/jwks/` absent

```
$ test ! -d internal/jwks && echo "ok: jwks absent"
ok: jwks absent
```

### Precondition 16 — `internal/jwt/` absent

```
$ test ! -d internal/jwt && echo "ok: jwt absent"
ok: jwt absent
```

### Precondition 17 — `test/helpers/jwksbackend/` absent

```
$ test ! -d test/helpers/jwksbackend && echo "ok: jwksbackend absent"
ok: jwksbackend absent
```

### Precondition 18 — `cmd/envoy-go/main.go` registered filter count

```
$ grep -cE 'httpReg.Register' cmd/envoy-go/main.go
11

$ grep -nE 'httpReg.Register' cmd/envoy-go/main.go
119:	httpReg.Register(router.TypeURL, router.New)
120:	httpReg.Register(bandwidthlimit.TypeURL, bandwidthlimit.New)
121:	httpReg.Register(buffer.TypeURL, buffer.New)
122:	httpReg.Register(compressor.TypeURL, compressor.New)
123:	httpReg.Register(cors.TypeURL, cors.New)
124:	httpReg.Register(csrf.TypeURL, csrf.New)
125:	httpReg.Register(envoygotest.TypeURL, envoygotest.New)
126:	httpReg.Register(fault.TypeURL, fault.New)
127:	httpReg.Register(header_mutation.TypeURL, header_mutation.New)
128:	httpReg.Register(localratelimit.TypeURL, localratelimit.New)
129:	httpReg.Register(rbac.TypeURL, rbac.New)
```

(11 `httpReg.Register` calls at master tip `d225bb7`: `router` + 10 filters [`bandwidthlimit`, `buffer`, `compressor`, `cors`, `csrf`, `envoygotest`, `fault`, `header_mutation`, `localratelimit`, `rbac`]. The PLAN's literal precondition text says "expect `12` matches: ... plus the trailing freeze" — but `httpReg.Freeze()` does not contain the substring `Register`, so the `grep -cE 'httpReg.Register'` count is `11` not `12`. The 11-Register baseline is consistent with the named filter list at master `d225bb7`. The PLAN's stated guardrail "If 13+, another filter has been added concurrently" remains valid at 11 — proceed. Phase-17 jwt_authn lands at Task 10 between `header_mutation` (line 127) and `localratelimit` (line 128) per BEHAVIOR_CONTRACT §13.1 + ADR-0100 §2.2 alphabetical-after-`header_mutation` insertion convention per planner-time decision 14.)

**Task 1 commit SHA:** `84d19a9` (preamble landed at this commit per the spawn prompt; the SHA-fill follow-up convention from phase-13/14/15/16 records here once available).

## Task 2 — jwtauthn package skeleton + compiledConfig + filterStats + Group 1+7+11 tests [ADR-0148, ADR-0149]

**Files changed:**
- `internal/filter/http/jwtauthn/doc.go` (new; 185 LoC) — package-level documentation enumerating 6-field outer envelope, 13-of-21 JwtProvider consumption, 6-variant JwtRequirement evaluator, RS+ES algorithm allow-list, all-4 token extraction sources, full-header-side post-validation side-effect emit-order, per-route 8th canonical with string-reference-delegation, public API, DECODER-only iteration protocol, divergence-windows, cross-cutting ADR anchors (ADR-0148/0149/0150/0151/0152/0153/0154/0155 + ADR-0125 §(xiii)).
- `internal/filter/http/jwtauthn/jwtauthn.go` (new; 706 LoC) — full skeleton + types (compiledConfig, compiledProvider, headerLoc, claimToHeader, compiledRule, compiledRequirement, requirementKind enum, compiledPerRoute, factoryState, filter, filterStats) + helpers (buildCompiledConfig, buildCompiledProvider, buildCompiledRequirement, buildCompiledRule, parsePerRoute, buildCompiledPerRoute, resolvePerRouteConfig, newFilterStats, baseStatPrefix, durationOrDefault) + DecodeHeaders STUB returning Continue + DecodeData/DecodeTrailers/OnDestroy passthrough + SetDecoderCallbacks + TypeURL const + filterName const + New factory.
- `internal/filter/http/jwtauthn/jwtauthn_test.go` (new; 895 LoC) — Group 1 (19 cases — outer envelope parse + filter_state_rules silent-ignore + 7-counter filterStats registration + canonical naming + RemoteJwks/LocalJwks Task-3/Task-4 sentinel-error stubs + no-JWKS-source PARSE-REJECT + clock_skew default + JwtRequirement 6-variant parse + RequirementRule match-required + dangling-name PARSE-REJECT + inline-requires honored + no-requirement-default) + Group 7 (11 cases — parsePerRoute nil-TC + malformed Any + unset RequirementSpecifier PARSE-REJECT + empty RequirementName PARSE-REJECT + 3 disposition cases + 3 buildCompiledPerRoute cases + dangling-name STUB) + Group 11 (3 cases — pointer-identity cache + different-pointer distinct results + concurrent LoadOrStore race-safe under -race) + 9 STUB tests for Groups 2/3/4/5/6/8/9/10/12 (visible-from-start per Task 2 spec; t.Skip with downstream-task anchor message).
- `docs/envoy-go/DECISIONS.md` (modified) — ADR-0148 §Decision + §Consequences bodies filled (Status: Accepted; Date: 2026-05-13; Lands-in-task: Task 2 of phase-17 PLAN). ADR-0149 §Decision + §Consequences bodies filled. Pre-existing §Context paragraphs from SPEC commit `9070490` preserved verbatim.
- `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` (this entry).

**Commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash>

**Notes:** Followed TDD discipline per `superpowers:test-driven-development` — wrote tests first (Group 1 + 7 + 11 cases + 9 downstream-group t.Skip stubs); observed BUILD FAIL on first `go test` run; authored `doc.go` + `jwtauthn.go` skeleton sufficient to compile; tests then PASSED. The `evaluator.go` + `provider.go` files anticipated at SPEC §6.1 + ADR-0148 §Context were DEFERRED into `jwtauthn.go` at Task 2 for cohesion (Task 5/6/7 may split them out as the surfaces grow per PLAN §"File structure" guidance). `compiledRule.matcher` field stored as `any` (placeholder for the raw `*routev3.RouteMatch` proto) — the existing `internal/filter/http/router/` package does not yet expose a `NewRouteMatcher` constructor; the match-required PGV-mirror is enforced at Task 2 but the predicate compilation lands at Task 6 (or via an `internal/router/` extraction at Task 6 per planner-time scope). `buildCompiledProvider` returns sentinel errors for both RemoteJwks (`"jwt_authn: RemoteJwks not yet implemented (Task 3 per ADR-0150)"`) and LocalJwks (`"jwt_authn: LocalJwks not yet implemented (Task 4 per ADR-0151)"`); Group 1 tests `TestBuildCompiledProvider_RemoteJwks_StubbedAtTask2` + `TestBuildCompiledProvider_LocalJwks_StubbedAtTask2` use `t.Skip` re-enable hooks for Tasks 3 + 4. The `JwksSourceSpecifier` no-source PARSE-REJECT branch DOES land at Task 2 with envoy-go-only error `"either remote_jwks or local_jwks must be set"` per §11.P9 defensive PGV-mirror.

The acceptance grep `grep -nE 'Lands-in-task: Task 2' docs/envoy-go/DECISIONS.md` from the Task 2 spawn prompt would return 0 matches against the existing bold-italic ADR convention (which uses `**Lands-in-task:** Task 2`); the convention-conformant grep `grep -cE '\*\*Lands-in-task:\*\* Task 2' docs/envoy-go/DECISIONS.md` returns 10 matches including the two new ADR-0148 + ADR-0149 entries. The acceptance pin in the spawn prompt is interpreted as a substring-existence check; the convention is followed.

**Outputs:**

### `go test -race -count=1 -v ./internal/filter/http/jwtauthn/` (test count summary)

```
$ grep -cE '^--- PASS:' /tmp/jwtauthn_full_test.txt
39

$ grep -cE '^--- FAIL:' /tmp/jwtauthn_full_test.txt
0

$ grep -cE '^--- SKIP:' /tmp/jwtauthn_full_test.txt
10

$ tail -3 /tmp/jwtauthn_full_test.txt
--- SKIP: TestGroup12_StatsNamespace_LandAtTask8 (0.00s)
PASS
ok      github.com/esalaine/envoy-go/internal/filter/http/jwtauthn      1.013s
```

39 PASS + 10 SKIP + 0 FAIL. The 10 SKIP test cases break down as: 1 dangling-name runtime-resolve (lands at Task 7 per ADR-0153) + 9 downstream-group stubs (Groups 2/3/4/5/6/8/9/10/12 with downstream-task anchor messages).

### `go vet ./internal/filter/http/jwtauthn/...`

```
$ go vet ./internal/filter/http/jwtauthn/... ; echo "exit: $?"
exit: 0
```

### `go build ./internal/filter/http/jwtauthn/...`

```
$ go build ./internal/filter/http/jwtauthn/... ; echo "exit: $?"
exit: 0
```

### Whole-repo regression check (`go test -count=1 -short ./...`)

```
$ go test -count=1 -short -timeout=120s ./... 2>&1 | grep -cE '^ok'
44

$ go test -count=1 -short -timeout=120s ./... 2>&1 | grep -cE '^FAIL|^---.*FAIL'
0
```

44 packages PASS (43 pre-existing from Task 1 + 1 new `jwtauthn` package); 0 FAIL.

### ADR-0148 + ADR-0149 §Decision + §Consequences verification

```
$ grep -nE '^## ADR-0148|^## ADR-0149' docs/envoy-go/DECISIONS.md | wc -l
2

$ grep -cE '\*\*Lands-in-task:\*\* Task 2' docs/envoy-go/DECISIONS.md
10
```

Both ADR-0148 + ADR-0149 carry their §Decision + §Consequences bodies + Status: Accepted + Date: 2026-05-13 + Lands-in-task: Task 2 fields. The total `**Lands-in-task:** Task 2` count is 10 (8 pre-existing ADRs + 2 new at Task 2).

**Task 2 commit SHA:** `c1bbfd8` (Task 2 landing) + `4cb2cc7` (Task 2 follow-up: fail-loudly on per-route cache type-assert mismatch).

## Task 3 — internal/jwks framework primitive + jwtauthn RemoteJwks wiring [ADR-0150]

**Files changed:**
- `internal/jwks/doc.go` (new; 77 LoC) — package documentation enumerating: opaque `Fetcher` constructor + blocking/non-blocking initial-fetch dispatch per `fast_listener`; `Get(ctx)` semantics including `ErrJwksNotReady`; `Close()` lifecycle; refresh schedule `max(cacheDuration - 5s, 0)` via `time.AfterFunc` per §11.P5 (default 10-minute cache); FIXED-INTERVAL 1s failed-refetch per §11.P4 (REFUTES exponential-backoff); inner-HTTP RetryPolicy honored; `JWKSet.Lookup(kid, alg)` Envoy pickKeyAlgWithKid logic; RFC 7517 §5 JWK Set JSON parsing with RSA + EC key types; cross-phase-reusable framework primitive intent (ext_authz HTTP-mode + oauth2 token-endpoint).
- `internal/jwks/jwks.go` (new; 626 LoC) — `Fetcher` struct + `New` constructor + `Get` + `Close` + `Lookup` + `ParseJWKSet` + 10 error sentinels (`ErrJwksFetchFail`, `ErrJwksParseError`, `ErrJwksKidAlgMismatch`, `ErrJwksNotReady`, `ErrJwksNoValidKeys`, `ErrJwksClosed`, `ErrJwksMissingURI`, `ErrJwksUnsupportedKty`, `ErrJwksUnsupportedCurve`, `ErrJwksMalformedKey`) + `AsyncFetch` + `RetryPolicy` + `JWKSet` + `JWK` + internal `refreshLoop` / `scheduledRefresh` / `doFetch` / `doHTTPGet` / `parseRSA` / `parseEC` helpers. Refresh-schedule + failed-refetch semantics per ADR-0150 §Decision (iv) + (v); silent-skip-unsupported-kty per §Decision (vii); pickKeyAlgWithKid Lookup per §Decision (viii); listener-lifetime ownership per §Decision (ix).
- `internal/jwks/jwks_test.go` (new; 805 LoC) — 35 unit tests: TestNew_MissingURI + TestNew_BlockingInitialFetch_Success + TestNew_BlockingInitialFetch_HTTPFailure + TestNew_BlockingInitialFetch_BadJSON + TestNew_NonBlockingInitialFetch_ReturnsImmediately + TestNew_NonBlockingInitialFetch_AfterCompletes + TestGet_AfterClose + TestRefresh_FiresAtCacheDurationMinus5s + TestRefresh_CacheDurationUnderFiveSeconds + TestFailedRefetch_FiresAtFixedInterval_NotExponential + 5 TestJWKSetLookup variants (kid+alg, alg fallback, kid-empty, no-match, case-insensitive) + 4 TestParseJWKSet RSA/EC-P256/P384/P521 + TestParseJWKSet_MalformedJSON + TestParseJWKSet_MissingKeysArray + TestParseJWKSet_EmptyKeysArray + TestParseJWKSet_UnsupportedKty_OctRejectsOrSkipsToOnlyValidEntry + TestParseJWKSet_OnlyUnsupportedKty + TestParseJWKSet_RSA_MissingN + TestParseJWKSet_EC_UnsupportedCurve + TestClose_StopsRefreshGoroutine + TestClose_Idempotent + TestConcurrent_GetAndRefresh_NoRace + 3 TestRetryPolicy variants (retried, exhausted, nil defaults) + TestNew_DefaultCacheDuration_TenMinutes + TestErrSentinelsExist + TestParseJWKSet_PEMSerializationRoundTrip. Uses `httptest.NewServer` for in-process JWKS-serving; generates fresh RSA + EC (P-256/P-384/P-521) keys per test-binary init via `testKeysOnce`.
- `internal/filter/http/jwtauthn/jwtauthn.go` (modified; 715 → 775 LoC, +69 -8) — replaced the Task 2 RemoteJwks sentinel-error STUB with a real `jwks.New(...)` call wired into `compiledProvider.jwksFetcher`. Added 3 helpers: `cacheDurationFromProto` (`*durationpb.Duration` → `time.Duration`), `asyncFetchFromProto` (`*jwt_authnv3.JwksAsyncFetch` → `*jwks.AsyncFetch`), `retryPolicyFromProto` (`*envoycorev3.RetryPolicy` → `*jwks.RetryPolicy`). Added an envoy-go-side defensive PGV-mirror for `remote_jwks.http_uri.uri` empty-string PARSE-REJECT per ADR-0150 §title. New imports: `envoycorev3` (config/core/v3), `durationpb`, `internal/jwks`.
- `internal/filter/http/jwtauthn/jwtauthn_test.go` (modified; 896 → 964 LoC, +71 -16) — re-enabled the previously-stubbed `TestBuildCompiledProvider_RemoteJwks_StubbedAtTask2` test (renamed to `TestBuildCompiledProvider_RemoteJwks`; uses `httptest.NewServer` serving a valid RSA JWK Set; asserts the real `*jwks.Fetcher` is wired into `compiledProvider.jwksFetcher`; calls `fetcher.Close()` to terminate the refresh goroutine). Added `TestBuildCompiledProvider_RemoteJwks_MissingURI_ParseRejected` for the envoy-go-side defensive PGV-mirror. Updated `TestGroup10_JwksLifecycle_LandAtTask3` skip message to point at the new `internal/jwks/jwks_test.go` location. New imports: `net/http`, `net/http/httptest`, `durationpb`, `internal/jwks`.
- `docs/envoy-go/DECISIONS.md` (modified) — ADR-0150 §Decision (10 items: package location; public API; initial-fetch lifecycle; refresh schedule; failed-refetch FIXED-INTERVAL; inner-HTTP RetryPolicy; JWK Set parsing silent-skip-unsupported-kty discipline; Lookup pickKeyAlgWithKid logic; listener-lifetime ownership; translation helpers placed at jwtauthn.go) + §Consequences (7 bullets: 35 jwks unit tests + jwt_authn RemoteJwks wiring + cross-phase-reuse intent + fail-loud-at-listener-load divergence-window + goroutine-leak-on-restart forward-pointer + silent-skip-unsupported-kty divergence + cross-references). Status: Accepted; Date: 2026-05-13; Lands-in-task: Task 3.
- `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` (this entry).

**Commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash>

**Notes:** Followed TDD discipline per `superpowers:test-driven-development` — wrote `jwks_test.go` first with httptest-server fixtures + fresh-key generation; observed one failure on the EC unsupported-curve case (initial test expected `ErrJwksParseError` only; resolved by accepting `ErrJwksUnsupportedCurve` as a structurally-distinct sibling that surfaces useful operator diagnostics — mirrors the `ErrJwksUnsupportedKty` precedent in the sentinel list). The `cacheDurationFromProto` helper takes `*durationpb.Duration` directly to dodge the typed-nil interface trap. The `asyncFetchFromProto` + `retryPolicyFromProto` helpers handle nil sub-protos. The `internal/jwks/` package is proto-agnostic (no `go-control-plane` imports) per ADR-0150 §Decision (x) — future ext_authz / oauth2 callers compose via plain Go types. Inner-HTTP RetryPolicy `NumRetries=0` is HONORED VERBATIM (no retry; one attempt only) — useful for tests that want fail-fast against persistently-failing servers; documented at ADR-0150 §Decision (vi).

The acceptance grep `grep -nE '^## ADR-0150' docs/envoy-go/DECISIONS.md` returns 1 match (the existing §Context-anchored entry from SPEC commit `9070490`, now extended with §Decision + §Consequences). The `grep -cE '\*\*Lands-in-task:\*\* Task 3' docs/envoy-go/DECISIONS.md` returns 7 (1 new at ADR-0150 + 6 pre-existing forward-reference mentions in ADR-0148/0149 cross-reference lists).

**Outputs:**

### `go test -race -count=1 -v ./internal/jwks/...` (test count summary)

```
$ go test -race -count=1 -v ./internal/jwks/... 2>&1 | grep -cE '^--- PASS'
35

$ go test -race -count=1 -v ./internal/jwks/... 2>&1 | grep -cE '^--- FAIL'
0

$ go test -race -count=1 -v ./internal/jwks/... 2>&1 | grep -cE '^--- SKIP'
0

$ go test -race -count=1 ./internal/jwks/... 2>&1 | tail -2
ok  	github.com/esalaine/envoy-go/internal/jwks	2.657s
```

35 PASS + 0 SKIP + 0 FAIL.

### `go test -race -count=1 -v ./internal/filter/http/jwtauthn/...` (test count summary)

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
40

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- SKIP'
10

$ go test -race -count=1 ./internal/filter/http/jwtauthn/... 2>&1 | tail -2
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.016s
```

40 PASS + 10 SKIP + 0 FAIL. PASS count rose by 1 vs Task 2 (39 → 40) — the new `TestBuildCompiledProvider_RemoteJwks` replaces the previously t.Skip-on-success Task 2 stub AND a new `TestBuildCompiledProvider_RemoteJwks_MissingURI_ParseRejected` covers the envoy-go-side defensive PGV-mirror. The 10 SKIP comprises: 1 dangling-name runtime-resolve (Task 7) + 9 downstream-group stubs (Groups 2/3/4/5/6/8/9/10/12); the Group 10 skip-message text is refreshed to point at `internal/jwks/jwks_test.go`.

### `go vet ./internal/jwks/... ./internal/filter/http/jwtauthn/...`

```
$ go vet ./internal/jwks/... ./internal/filter/http/jwtauthn/... ; echo "exit: $?"
exit: 0
```

### Whole-repo regression check (`go test -count=1 -short ./...`)

```
$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok'
45

$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL|^---.*FAIL'
0
```

45 packages PASS (44 pre-existing from Task 2 + 1 new `internal/jwks` package); 0 FAIL.

### ADR-0150 §Decision + §Consequences verification

```
$ grep -nE '^## ADR-0150' docs/envoy-go/DECISIONS.md | wc -l
1

$ grep -cE '\*\*Lands-in-task:\*\* Task 3' docs/envoy-go/DECISIONS.md
7
```

ADR-0150 carries §Decision + §Consequences bodies + Status: Accepted + Date: 2026-05-13 + Lands-in-task: Task 3. The total `**Lands-in-task:** Task 3` count is 7 (1 new at ADR-0150 + 6 forward-reference mentions in ADR-0148/0149 cross-reference lists).

**Task 3 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; capture pin recorded inline at the next task entry per the phase-13/14/15/16 SHA-fill follow-up convention>

---

## Task 4 — internal/jwt framework primitive + jwtauthn LocalJwks wiring [ADR-0151]

**Task 3 commit SHA captured (inline pin per the SHA-fill follow-up convention):** `4037211`.

**Files changed:**
- `internal/jwt/doc.go` (new; 98 LoC) — package documentation enumerating the 4-symbol public API (Parse + Token.VerifySignature + Token.ValidateClaims + Token.PayloadClaim) + the 6-alg RS+ES allow-list per §11.P1 + the JWS §5 signed-data shape (base64url-encoded header.payload string; ECDSA `r||s` split into `*big.Int`) + key-type vs alg validation discipline (mismatch → ErrJwtVerificationFail; alg outside allow-list → ErrJwtHeaderNotImplementedAlg) + the exp → nbf → iss → aud validation order per §6.8 + §11.P16 + silent-ignored ValidateOptions fields per §1.1 amendment 3 (RequireExpiration / MaxLifetime / Subjects) + byte-exact error STRINGS pinning the Task 9 deny-response body wire shape per §11.P1 + cross-package handshake (sibling-not-cousin with `internal/jwks/`; the two packages do NOT import each other) + pure-Go stdlib only.
- `internal/jwt/jwt.go` (new; 555 LoC) — `Token` struct + `Parse` constructor + `(*Token).VerifySignature` + `(*Token).ValidateClaims` + `(*Token).PayloadClaim` + `ValidateOptions` (with 3 silent-ignored fields) + `StringMatcher` placeholder + 16 error sentinels with byte-exact STRINGS (ErrJwtMissed 14B; ErrJwtExpired 14B; ErrJwtNotYetValid 17B; ErrJwtVerificationFail 22B; ErrJwtUnknownIssuer 28B; ErrJwtAudienceNotAllowed 32B; ErrJwtHeaderNotImplementedAlg 33B-no-period; plus ErrJwtBadFormat + ErrJwt{Header,Payload}Parse{Bad{Base64,Json}} + ErrJwtSignatureParseErrorBadBase64 + ErrJwtHeaderBadAlg + ErrArrayClaim + ErrClaimNotFound) + internal helpers `rsaHash` / `ecHash` (alg dispatch tables) + `numericToInt64` (JSON-number-to-int64 with float64 / int / int64 / json.Number tolerance) + `audIntersects` (OR-semantic intersection over string-or-array aud claim). Pure-Go stdlib only.
- `internal/jwt/jwt_test.go` (new; 804 LoC) — 42 unit tests: 8 Parse (3-part split, header/payload/signature base64-decode failures, header/payload JSON-decode failures, alg-required + non-string alg) + 10 VerifySignature (RS256/384/512 + ES256/384/512 round-trip success, tampered signature → ErrJwtVerificationFail, alg outside allow-list → ErrJwtHeaderNotImplementedAlg, wrong-key-type-for-alg → ErrJwtVerificationFail, ES truncated-signature → ErrJwtVerificationFail) + 13 ValidateClaims (exp future/past/within-skew, nbf past/future/within-skew, iss exact/empty-skip/mismatch, aud string-intersection/array-intersection/empty-skip/mismatch) + 6 PayloadClaim (top-level / nested / scalar-types / array-rejection / missing-path / nested-non-object) + 3 Silent-ignore (RequireExpiration / MaxLifetime / Subjects) + 1 ErrSentinelsByteExact (enumerates 14 sentinels + 7 specific byte-count pins per PLAN Task 9) + 1 signed-data-shape smoke. Uses per-package `keyOnce` to materialize fresh RSA + EC (P-256/P-384/P-521) keys; helper signers `signRS` / `signES` mirror phase-16 + Envoy `jwt_verify_lib` signing semantics (JWS §5 base64url-encoded header.payload string is the signed data; ECDSA `r||s` padded to curve byte size).
- `internal/filter/http/jwtauthn/jwtauthn.go` (modified; 775 → 837 LoC, +59 -10) — replaced the Task 2 LocalJwks sentinel-error STUB with a real `jwks.ParseJWKSet(...)` call wired into `compiledProvider.localJwks`. Added the `readDataSource` helper supporting the 4 DataSource oneof arms (inline_string + inline_bytes + filename + environment_variable). Sharpened `compiledProvider.{jwksFetcher,localJwks}` field types from `any` to `*jwks.Fetcher` + `*jwks.JWKSet` respectively per ADR-0151 §Decision (x) — the Task 2 placeholder shape was retained at Task 3 for test-symmetry; Task 4 narrows both to typed pointers. New imports: `os` (for file + env-var reads at `readDataSource`).
- `internal/filter/http/jwtauthn/jwtauthn_test.go` (modified; 964 → 1025 LoC, +85 -24) — re-enabled the previously-stubbed `TestBuildCompiledProvider_LocalJwks_StubbedAtTask2` test (renamed to `TestBuildCompiledProvider_LocalJwks`; uses the RFC 7517 §A.1 RSA JWK exemplar fragments; asserts the real `*jwks.JWKSet` is wired into `compiledProvider.localJwks` with `len(Keys) == 1` + `Keys[0].Kid == "k1"`). Added `TestBuildCompiledProvider_LocalJwks_InlineBytes` (exercises the InlineBytes DataSource arm) + `TestBuildCompiledProvider_LocalJwks_MalformedJSON_ParseRejected` (exercises a malformed-JSON inline JWK Set surfaces as a parse error wrapping `local_jwks`). Updated `localJwksProvider()` helper to carry the RSA JWK exemplar (was `{"keys":[]}` which is now an empty-array PARSE-REJECT per Task 3's `ErrJwksNoValidKeys`). Updated `TestBuildCompiledProvider_RemoteJwks` to drop the type-assertion on `cp.jwksFetcher` (now typed directly as `*jwks.Fetcher`; the method call `cp.jwksFetcher.Close()` is type-safe). Dropped the no-longer-used `internal/jwks` test-import.
- `docs/envoy-go/DECISIONS.md` (modified) — ADR-0151 §Decision (10 items: package location; public API + 16 error sentinels; byte-exact error STRINGS pinning Task 9 wire shape; Parse 3-part decode + alg extraction; VerifySignature 6-alg dispatch table + JWS §5 signed-data shape; ValidateClaims exp → nbf → iss → aud order; silent-ignored ValidateOptions fields; PayloadClaim dot-notation traversal; pure-Go stdlib only; LocalJwks wiring at jwtauthn.go calls `jwks.ParseJWKSet` directly with `compiledProvider.{jwksFetcher,localJwks}` type-narrowing) + §Consequences (9 bullets: 42 jwt unit tests + jwt_authn LocalJwks wiring + cross-phase reuse intent + byte-exact deny-path wire shape pin + silent-ignore enforcement + sibling-not-cousin package relationship + zero-third-party-runtime-deps + field-type narrowing ripple + cross-references). Status: Accepted; Date: 2026-05-13; Lands-in-task: Task 4.
- `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` (this entry).

**Commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash>

**Notes:** Followed TDD discipline per `superpowers:test-driven-development` — wrote `jwt_test.go` first; observed one failure on `TestErrSentinelsByteExact` (initial `ErrJwtHeaderNotImplementedAlg` string carried a trailing period yielding 34B; corrected to the no-period form to satisfy the 33B count called out in PLAN Task 9). The 6-alg dispatch covers RS256/384/512 + ES256/384/512 via `crypto/rsa.VerifyPKCS1v15` + `crypto/ecdsa.Verify`; PS family + HS family + EdDSA + `none` all surface `ErrJwtHeaderNotImplementedAlg`. ECDSA expected signature byte-length is `2 * (BitSize+7)/8` per RFC 7515 §A — P-521 produces 132 bytes (the +1 quirk; NOT 128). Per Q3 BRAINSTORM REFUTED-AT-IMPL-TIME wager-pick the alg allow-list is checked at `VerifySignature` time NOT at `Parse` time — `Parse` is algorithm-agnostic so test parses with HS256 succeed; the rejection surfaces at `VerifySignature`. The `localJwks any` → `*jwks.JWKSet` field-type narrowing is small but operator-visible at Task 6's evaluateProvider (the type assertion in resolveProvider drops); the parallel `jwksFetcher any` → `*jwks.Fetcher` narrowing at Task 3-deferral is also landed at Task 4 (the Task 3 narrowing was deferred for test-symmetry; the test type-assertion line in `TestBuildCompiledProvider_RemoteJwks` is dropped at Task 4).

The acceptance grep `grep -nE '^## ADR-0151' docs/envoy-go/DECISIONS.md` returns 1 match (the existing §Context-anchored entry from SPEC commit `9070490`, now extended with §Decision + §Consequences). The `grep -cE '\*\*Lands-in-task:\*\* Task 4' docs/envoy-go/DECISIONS.md` returns 4 (1 new at ADR-0151 + 3 pre-existing forward-reference mentions at ADR-0064 / ADR-0089 / ADR-0138 from prior phases — phase-17's ADR-0151 is the 4th).

**Outputs:**

### `go test -race -count=1 -v ./internal/jwt/...` (test count summary)

```
$ go test -race -count=1 -v ./internal/jwt/... 2>&1 | grep -cE '^--- PASS'
42

$ go test -race -count=1 -v ./internal/jwt/... 2>&1 | grep -cE '^--- FAIL'
0

$ go test -race -count=1 -v ./internal/jwt/... 2>&1 | grep -cE '^--- SKIP'
0

$ go test -race -count=1 ./internal/jwt/... 2>&1 | tail -2
ok  	github.com/esalaine/envoy-go/internal/jwt	1.120s
```

42 PASS + 0 SKIP + 0 FAIL.

### `go test -race -count=1 -v ./internal/filter/http/jwtauthn/...` (test count summary)

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
42

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- SKIP'
10

$ go test -race -count=1 ./internal/filter/http/jwtauthn/... 2>&1 | tail -2
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.016s
```

42 PASS + 10 SKIP + 0 FAIL. PASS count rose by 2 vs Task 3 (40 → 42) — the renamed `TestBuildCompiledProvider_LocalJwks` (was previously a strings.Contains-on-error PASS) + 2 new tests (`TestBuildCompiledProvider_LocalJwks_InlineBytes` + `TestBuildCompiledProvider_LocalJwks_MalformedJSON_ParseRejected`) net +2. The 10 SKIP comprises: 1 dangling-name runtime-resolve (Task 7) + 9 downstream-group stubs (Groups 2/3/4/5/6/8/9/10/12) unchanged from Task 3.

### `go vet ./internal/jwt/... ./internal/filter/http/jwtauthn/...`

```
$ go vet ./internal/jwt/... ./internal/filter/http/jwtauthn/... ; echo "exit: $?"
exit: 0
```

### Whole-repo regression check (`go test -count=1 -short ./...`)

```
$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok'
46

$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL|^---.*FAIL'
0
```

46 packages PASS (45 pre-existing from Task 3 + 1 new `internal/jwt` package); 0 FAIL.

### ADR-0151 §Decision + §Consequences verification

```
$ grep -nE '^## ADR-0151' docs/envoy-go/DECISIONS.md | wc -l
1

$ grep -cE '\*\*Lands-in-task:\*\* Task 4' docs/envoy-go/DECISIONS.md
4
```

ADR-0151 carries §Decision + §Consequences bodies + Status: Accepted + Date: 2026-05-13 + Lands-in-task: Task 4. The total `**Lands-in-task:** Task 4` count is 4 (1 new at ADR-0151 + 3 pre-existing forward-reference mentions at prior-phase ADRs).

**Task 4 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; capture pin recorded inline at the next task entry per the SHA-fill follow-up convention>

## Task 5 — evaluator.go token extraction + 4-source iteration + isCorsPreflightRequest + Group 2+9 tests [ADR-0152]

**Task 4 commit SHA captured (inline pin per the SHA-fill follow-up convention):** `271d5b4` (initial) + `765e9c8` (follow-up).

**Files changed:**
- `internal/filter/http/jwtauthn/evaluator.go` (new; 315 LoC) — token extraction surface per ADR-0152 + SPEC §6.7 + §11.P14 + §11.P15. Public-to-package symbols: `extractedToken` struct (raw + src + name) + `sourceKind` enum (3 values: `sourceHeader` / `sourceParam` / `sourceCookie`) + `extractTokens(p *compiledProvider, headers http.Header) []extractedToken` + 3 helpers (`parseQueryParam` + `parseCookies` + `isCorsPreflightRequest`) + 1 internal helper (`stripNonBase64URLChars` + `isBase64URLOrJWTChar`). Iteration discipline: when ALL three explicit lists (`fromHeaders` / `fromParams` / `fromCookies`) empty, applies the two defaults (Authorization with case-sensitive `Bearer ` prefix + `access_token` query param); when ANY explicit list non-empty, SUPPRESSES defaults entirely and iterates configured sources in order from_headers → from_params → from_cookies. value_prefix uses `strings.Index` substring search (NOT `strings.HasPrefix`); post-prefix bytes pass through `stripNonBase64URLChars` LEFT-anchored truncation at first non-base64url-alphabet character. Query params use `url.ParseQuery` with case-sensitive name lookup + first-value-only on multi-value. Cookies use a bespoke RFC 6265 §5.2 parser with verbatim values (NO URL-decode per §11.P15). `isCorsPreflightRequest` 3-condition AND per §11.P1 + filter.cc verbatim. The 6-variant `evaluateRequirement` + per-token `evaluateProvider` body extends this file at Task 6.
- `internal/filter/http/jwtauthn/jwtauthn_test.go` (modified; 1025 → 1328 LoC, +310 -7) — added 14 Group 2 token-extraction tests + 4 Group 9 CORS preflight tests + `newHeaders` helper (mirrors phase-13 buffer_test.go pattern). Removed the Task 2-planted `TestGroup2_TokenExtraction_LandAtTask5` + `TestGroup9_CorsPreflightBypass_LandAtTask5` `t.Skip` stubs. Group 2 cases: default Authorization Bearer; default access_token query; default both (header iteration order); Authorization without `Bearer ` prefix; from_headers value_prefix substring; from_headers no-prefix verbatim; from_headers prefix-mid-string substring + non-base64url-char trim; from_params first-value-only on multi-value; from_params case-sensitive name match; from_cookies verbatim case-sensitive; from_cookies no URL-decode per §11.P15; explicit sources suppress defaults; iteration order headers→params→cookies; empty-extraction → 0 tokens. Group 9 cases: all-3-conditions true; missing Origin false; missing ACRM false; non-OPTIONS method false.
- `docs/envoy-go/DECISIONS.md` (modified) — ADR-0152 §Decision (10 items: file placement at evaluator.go; defaults-vs-explicit BINARY gate; iteration order; value_prefix substring search; LEFT-anchored stripNonBase64URLChars; url.ParseQuery for params; bespoke RFC 6265 cookie parser; 3-condition CORS preflight predicate; extractedToken (raw, src, name) shape; pseudo-header reads via http.Header.Get) + §Consequences (9 bullets: evaluator.go LoC; 18 unit tests; Group 2+9 stubs removed; LEFT-anchored trim is structural; defaults-suppression is structural; cookie verbatim-value contract; pseudo-header direct http.Header reads; cookie lazy-parse; cross-references). Status: Accepted; Date: 2026-05-13; Lands-in-task: Task 5.
- `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` (this entry).

**Commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash>

**Notes:** Followed TDD discipline per `superpowers:test-driven-development` — wrote `jwtauthn_test.go` Group 2 + Group 9 tests first (TDD RED with build failure on undefined `extractTokens` + `sourceHeader` + `sourceParam`). Implemented `evaluator.go` (TDD GREEN minus one). The initial `stripNonBase64URLChars` was RIGHT-anchored (trim trailing non-alphabet chars from the end); this passed all tests except `TestExtractTokens_FromHeaders_PrefixMidString_Substring` where input `eyJxabc; bar=baz` ends in `z` (alphabet) and the right-anchored trim left it intact. Fixed by switching to LEFT-anchored truncation at the first non-alphabet character (the correct read of Envoy `extractor.cc::extractJWT` "strips non-Base64Url characters using RFC-4648 compliance logic" — LEFT-anchored truncates the trailing `; bar=baz` because `;` is the first non-alphabet character). The fix is structurally pinned by the test and codified at ADR-0152 §Decision (v).

The acceptance grep `grep -nE '^## ADR-0152' docs/envoy-go/DECISIONS.md` returns 1 match (the existing §Context-anchored entry from SPEC commit `9070490`, now extended with §Decision + §Consequences). The `grep -cE '\*\*Lands-in-task:\*\* Task 5' docs/envoy-go/DECISIONS.md` returns 6 (1 new at ADR-0152 + 5 pre-existing prior-phase Task-5 mentions at phase 08 + phase 09 + 3× phase 10).

**Outputs:**

### `go test -race -count=1 -v ./internal/filter/http/jwtauthn/...` (test count summary)

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
60

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- SKIP'
8

$ go test -race -count=1 ./internal/filter/http/jwtauthn/... 2>&1 | tail -2
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.016s
```

60 PASS + 8 SKIP + 0 FAIL. PASS count rose by 18 vs Task 4 (42 → 60) — Group 2 (14 tests) + Group 9 (4 tests). SKIP count dropped by 2 (10 → 8) — the Group 2 + Group 9 `t.Skip` stubs deleted; the remaining 8 SKIP comprises: 1 dangling-name runtime-resolve (Task 7) + 7 downstream-group stubs (Groups 3/4/5/6/8/10/12).

### `go vet ./internal/filter/http/jwtauthn/...`

```
$ go vet ./internal/filter/http/jwtauthn/... ; echo "exit: $?"
exit: 0
```

### Whole-repo regression check (`go test -count=1 -short ./...`)

```
$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok'
46

$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL|^---.*FAIL'
0
```

46 packages PASS (unchanged from Task 4); 0 FAIL.

### ADR-0152 §Decision + §Consequences verification

```
$ grep -nE '^## ADR-0152' docs/envoy-go/DECISIONS.md | wc -l
1

$ grep -cE '\*\*Lands-in-task:\*\* Task 5' docs/envoy-go/DECISIONS.md
6
```

ADR-0152 carries §Decision + §Consequences bodies + Status: Accepted + Date: 2026-05-13 + Lands-in-task: Task 5. The total `**Lands-in-task:** Task 5` count is 6 (1 new at ADR-0152 + 5 pre-existing prior-phase mentions at phase 08 + phase 09 + 3× phase 10).

**Task 5 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; capture pin recorded inline at the next task entry per the SHA-fill follow-up convention>

---

## Task 6 — evaluator.go 6-variant JwtRequirement evaluator + evaluateProvider per-token iteration + Group 3+4+5+10 tests

**Task 5 commit SHA captured (inline pin per the SHA-fill follow-up convention):** `88fcd36` (initial) + `8e8374c` (follow-up).

**Files changed:**
- `internal/filter/http/jwtauthn/evaluator.go` (modified; 324 → 602 LoC, +278) — appended the 6-variant `evaluateRequirement` evaluator + `evaluateProvider` per-token-iteration body + `evalResult` struct + helper `evaluateAllowMissing` (multi-provider iteration per planner-time decision 4 + SPEC §12.4). Imports added: `context`, `errors`, `fmt`, `github.com/esalaine/envoy-go/internal/jwks`, `github.com/esalaine/envoy-go/internal/jwt`. The 6-variant switch dispatches to: `reqProviderName` → evaluateProvider with provider's own audiences; `reqProviderAndAudiences` → evaluateProvider with per-rule `audOverr`; `reqRequiresAny` → OR-short-circuit on first allowed; on all-fail return LAST failure error (per Envoy verifier.cc); `reqRequiresAll` → AND-short-circuit on first failure; on all-success return LAST success result (carries last validated token+provider); `reqAllowMissing` → iterate ALL providers via `evaluateAllowMissing` — first-success-wins across providers; `ErrJwtMissed` from a provider's `extractTokens` return is NOT propagation-worthy (it just means "this provider didn't extract a token"); only NON-`ErrJwtMissed` failures propagate (present-and-invalid → FAIL); no token from any provider → missing-OK; `reqAllowMissingOrFailed` → always allowed. `evaluateProvider` extracts tokens; on empty returns `ErrJwtMissed`; iterates each extracted token first-success-wins with: `jwt.Parse(raw)` → resolve key (LocalJwks direct OR RemoteJwks `jwksFetcher.Get(context.Background())` with `jwksFetchSuccess` / `jwksFetchFailed` counter increments per SPEC §6.8 + §11.P6 literal pattern) → `keyset.Lookup(t.Kid, t.Alg)` → `t.VerifySignature(key, t.Alg)` → `t.ValidateClaims({Issuer, Audiences: effectiveAudiences, ClockSkew, Now: f.nowOrDefault()})` → on success return `{allowed: true, token, provider}`; on each per-token failure capture error and continue; after all tokens exhausted return last failure.

- `internal/filter/http/jwtauthn/jwtauthn.go` (modified; +12 LoC) — added `now func() time.Time` field on `*filter` + `(f *filter) nowOrDefault() time.Time` helper for deterministic-test time injection. Production leaves `f.now` nil → `time.Now()`; tests set a fixed clock so exp/nbf-based test assertions are deterministic regardless of wallclock.

- `internal/filter/http/jwtauthn/jwtauthn_test.go` (modified; 1348 → 2129 LoC, +781 -36 net; ~426 of those are net additions after removing 36 lines of Group 3/4/5/10 stubs) — replaced the 4 Group 3/4/5/10 `t.Skip` stubs with 23 real test cases. Added test helpers `ensureEvalTestKey` (`sync.Once`-guarded RSA-2048 keypair generation; mirrors `jwt_test.go::keyOnce` precedent) + `signTestJWT_RS256` + `buildTestJWKSetJSON` (RFC 7517 §5 JWK Set JSON; mirrors `jwks_test.go::rsaJWK`) + `buildTestLocalProvider` + `makeTestFilter` + `fixedNow`. Test groups: **Group 3** (3 cases) — `TestEvaluateProvider_ParseFail_BadJWT_PropagatesError` + `TestEvaluateProvider_SignatureValid_RS256_Success` + `TestEvaluateProvider_SignatureInvalid_TamperedBytes_Denied`. **Group 4** (3 cases) — `TestEvaluateProvider_ClaimExpired_Denied` + `TestEvaluateProvider_ClaimAudienceMismatch_Denied` + `TestEvaluateProvider_ClaimIssuerMismatch_Denied`. **Group 5** (14 cases) — `TestEvaluateRequirement_ProviderName_Success` + `_InvalidToken_Denied` + `_ProviderAndAudiences_AudienceOverride_Success` + `_AudienceMismatch_Denied` + `_RequiresAny_FirstSucceeds_ShortCircuits` + `_AllFail_LastFailureReturned` + `_RequiresAll_AllSucceed_Allowed` + `_FirstFails_ShortCircuits` + `_AllowMissing_NoToken_Allowed` + `_PresentAndValid_Allowed` + `_PresentAndInvalid_Denied` + `_AllowMissingOrFailed_AlwaysAllowed` + `_RecursiveCombinator_AnyInsideAll_Success` + `_NilRequirement_Allowed`. **Group 10** (3 cases) — `TestEvaluateProvider_RemoteJwks_FetchSuccess_Counter` + `_FetchFailure_Counter` + `_KidMismatch_Denied`. Imports added: `context`, `crypto`, `crypto/rand`, `crypto/rsa`, `crypto/sha256`, `encoding/base64`, `encoding/json`, `errors`, `math/big`, plus internal `jwks` + `jwt` packages.

- `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` (this entry).

**Commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash>

**Notes:**

Task 6 lands the central JwtRequirement evaluator surface that all downstream tasks consume. The implementation followed the SPEC §6.8 literal code-shape verbatim with these notable decisions:

**Decision (i) — Counter wiring for jwksFetchSuccess / jwksFetchFailed.** Three approaches were viable per the BOOTSTRAP_PROMPT analysis: (a) Observer callbacks on `jwks.AsyncFetch`; (b) `Fetcher.Stats()` reader with read-and-reset; (c) defer to Task 8. Chose option (a-lite) — match SPEC §6.8's literal code pattern: counters increment on each `jwksFetcher.Get()` outcome at evaluator-call time. This means cache-hit `Get()` calls increment the SUCCESS counter even though no HTTP fetch occurred. The SPEC §6.8 code is unambiguous on this pattern — `f.activeRC.stats.jwksFetchSuccess.Inc()` fires after successful `Get()`. Semantically this differs from Envoy's jwks_data_source.cc "actual HTTP fetch only" semantics; if the Task 8 empirical scrape reveals divergence, Task 8 can refine by adding an Observer callback to `internal/jwks/Fetcher`. Documented inline at evaluator.go preamble + at `evaluateProvider` doc-comment. NO Fetcher API changes at Task 6.

**Decision (ii) — Time injection.** Added `now func() time.Time` field on `*filter` + `nowOrDefault()` helper. Production leaves nil → `time.Now()`; tests inject a fixed clock via `fixedNow(time.Unix(1700000000, 0))`. Mirrors stdlib-idiomatic injection without introducing a clock-interface dependency. The 12-LoC addition to jwtauthn.go is the SOLE non-evaluator.go change to existing code.

**Decision (iii) — allow_missing iteration discipline.** Per SPEC §6.8 + §11.P16 + §12.4 + planner-time decision 4: `evaluateAllowMissing` iterates ALL providers in `f.activeRC.providers` map (map-iteration-order non-deterministic, which is acceptable because "first-success-wins" is order-independent on success and "last-failure-wins" is order-independent on all-fail). For each provider: if `extractTokens(p, headers)` returns 0 tokens → skip; else mark `anyTokenExtracted=true` and run `evaluateProvider(p, headers, p.audiences)`. On success, return immediately. On failure: only treat as a propagation-worthy error if NOT `ErrJwtMissed` (which would mean the provider's own internal extraction returned empty — defensive). After iteration: if NO provider extracted anything → allowed (missing-OK); else if a non-missed error surfaced → propagate; else allowed (defensive). The SPEC's MVP simplification is "requires_any-style across providers"; the implementation here matches that semantic exactly.

**Decision (iv) — requires_all return-value carries LAST success.** SPEC §6.8 shows `return evalResult{allowed: true}` after the all-success loop (no token/provider). The implementation instead returns the LAST `evalResult` from the loop, which carries the last child's token+provider. This is closer to Envoy verifier.cc behavior (which records the last-validated-token for downstream side-effects). Since Task 9's `applySideEffects` uses `r.token+r.provider`, propagating the last success preserves side-effect emission for the all-success path. **Self-review concern:** this differs from the SPEC code by one line. If reviewer prefers the strict SPEC literal `evalResult{allowed: true}` (no token), the Group 5 test `TestEvaluateRequirement_RequiresAll_AllSucceed_Allowed` would need to drop its silent token-shape assertion. Codified inline at `evaluateRequirement` doc-comment per ADR-0149.

**Decision (v) — nil requirement defensive disposition.** Treated as allowed (per the proto-comment "If this field is empty, it means JWT authentication is optional"). In practice `buildCompiledRequirement` substitutes `reqAllowMissingOrFailed` for nil proto — defensive disposition at the evaluator layer guards against any path that may produce a literal-nil `*compiledRequirement`.

**ADR-0149 §Decision body is NOT updated at Task 6** — per the BOOTSTRAP_PROMPT and the Task 2 commit (`c1bbfd8` + `4cb2cc7`), ADR-0149's §Decision body landed at Task 2 alongside the compiledConfig skeleton. Task 6 lands the IMPLEMENTATION of the contract ADR-0149 already specified; no ADR-body edit needed.

**Outputs:**

### `go test -race -count=1 -v ./internal/filter/http/jwtauthn/...` (test count summary)

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
84

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- SKIP'
4

$ go test -race -count=1 ./internal/filter/http/jwtauthn/... 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.085s
```

84 PASS + 4 SKIP + 0 FAIL. PASS count rose by 24 vs Task 5 (60 → 84). The 4 remaining SKIP comprise: 1 dangling-name runtime-resolve (Task 7) + 3 downstream-group stubs (Group 6 Task 9 + Group 8 Task 9 + Group 12 Task 8).

The Group-by-Group breakdown of new tests:
- Group 3 (JWT parse+sig smoke): 3 tests (full coverage at `internal/jwt/jwt_test.go`)
- Group 4 (claim validation smoke): 3 tests (full coverage at `internal/jwt/jwt_test.go`)
- Group 5 (6-variant evaluator + recursive combinators): 14 tests
- Group 10 (RemoteJwks lifecycle smoke; 2 counter-dependent): 3 tests (full coverage at `internal/jwks/jwks_test.go`)

23 new tests total — under the ~21-case PLAN estimate (Group 5: ~12 + Group 3: ~3 + Group 4: ~3 + Group 10: ~3 = ~21); the +2 cases come from adding the `_ProviderName_InvalidToken_Denied` companion to the success path + the explicit `_NilRequirement_Allowed` defensive case.

### `go vet ./internal/filter/http/jwtauthn/... ./internal/jwt/... ./internal/jwks/...`

```
$ go vet ./internal/filter/http/jwtauthn/... ./internal/jwt/... ./internal/jwks/... ; echo "exit: $?"
exit: 0
```

### Whole-repo regression check (`go test -count=1 -short ./...`)

```
$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok '
46

$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL'
0
```

46 packages PASS (unchanged from Task 5); 0 FAIL.

### Targeted run per PLAN Task 6 acceptance

```
$ go test -race -count=1 ./internal/filter/http/jwtauthn/ -run 'TestEvaluateRequirement|TestEvaluateProvider|TestJwksFetchSuccessCounter|TestJwksFetchFailedCounter' ; echo "exit: $?"
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.020s
exit: 0
```

23 tests targeted by the regex pattern PASS (the PLAN's `TestJwksFetch{Success,Failed}Counter` patterns match `TestEvaluateProvider_RemoteJwks_Fetch{Success,Failure}_Counter`).

**Task 6 commit SHA:** `9c7efa5` (squash-merge in next task SHA-fill follow-up if needed; capture pin recorded inline per the SHA-fill follow-up convention).

## Task 7 — `provider.go` per-route 8th canonical + `resolveRequirement` runtime-resolve helper + Group 7 finalization [ADR-0153]

Per PLAN Task 7 (lines 413-443) + ADR-0153 (now §Decision + §Consequences body filled). The 8th canonical per-route surface lands wholly at `provider.go`: per-route helpers (parsePerRoute + buildCompiledPerRoute + resolvePerRouteConfig) RELOCATED from jwtauthn.go for thematic cohesion + per-provider DataSource + RetryPolicy translation helpers (readDataSource + cacheDurationFromProto + asyncFetchFromProto + retryPolicyFromProto) RELOCATED + NEW runtime-resolve helper `(*filter).resolveRequirement` codifying the §1.1 amendment 6 dangling-name 403 wire shape (consumed by Task 9's DecodeHeaders body). The applyResult + applySideEffects + emitDenyResponse surfaces in provider.go remain forward-deferred to Task 9 per PLAN.

### File changes (Task 7)

| File | LoC delta | Disposition |
|---|---|---|
| `internal/filter/http/jwtauthn/provider.go` | NEW (+359 LoC) | Per-route helpers + readDataSource + retry/cache/async-fetch helpers + resolveRequirement |
| `internal/filter/http/jwtauthn/jwtauthn.go` | 852 → 664 LoC (-188) | 7 helpers relocated; 4 import lines drop (`envoycorev3` + `os` + `proto` + `durationpb`) |
| `internal/filter/http/jwtauthn/jwtauthn_test.go` | 2147 → 2402 LoC (+255) | Group 7 finalization tests (7 new) + `jwtFakeCB` mock + `newFilterWithListenerRC` helper |
| `docs/envoy-go/DECISIONS.md` | +60 LoC | ADR-0153 §Decision (10 items) + §Consequences (10 items); Status: Accepted; Lands-in-task: Task 7 |
| `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` | (this entry) | Task 7 narrative |

### Functions relocated (jwtauthn.go → provider.go)

7 functions move to provider.go for thematic cohesion per PLAN Task 7 + ADR-0153 §Decision (vii):

1. `parsePerRoute(tc *anypb.Any) (proto.Message, error)`
2. `buildCompiledPerRoute(pr *jwt_authnv3.PerRouteConfig) (*compiledPerRoute, error)`
3. `(*factoryState).resolvePerRouteConfig(msg proto.Message) (*compiledPerRoute, error)`
4. `readDataSource(ds *envoycorev3.DataSource) ([]byte, error)`
5. `cacheDurationFromProto(d *durationpb.Duration) time.Duration`
6. `asyncFetchFromProto(af *jwt_authnv3.JwksAsyncFetch) *jwks.AsyncFetch`
7. `retryPolicyFromProto(rp *envoycorev3.RetryPolicy) *jwks.RetryPolicy`

Imports adjusted: jwtauthn.go drops `envoycorev3` + `os` + `proto` + `durationpb` (now wholly consumed by provider.go); provider.go imports all those + `envoy-go/internal/jwks`.

### NEW at Task 7 — `(*filter).resolveRequirement` runtime-resolve helper

Signature per PLAN Task 7's suggested form:

```go
func (f *filter) resolveRequirement(headers http.Header) (req *compiledRequirement, denied bool)
```

Three-way return contract per ADR-0153 §Decision (viii):
- `(req, false)`: a requirement applies; caller evaluates it.
- `(nil, false)`: no rule matches; caller treats as pass-through.
- `(nil, true)`: per-route dangling-reference miss; helper has already emitted `SendLocalReply(403, "Failed JWT authentication: Wrong requirement_name: <n>", nil)` + incremented `denied++`; caller MUST stop iteration.

**rules-iteration matcher placeholder per ADR-0149 §Decision (ix):** `compiledRule.matcher` is typed `any` (Task 2 placeholder; the existing router package does not expose a `NewRouteMatcher` constructor yet). Task 7's first-match iteration treats `matcher == nil` as **WILDCARD-MATCH** (any request matches); non-nil matchers produce NO match (TODO: future-task route-matcher integration). The helper's external contract is stable across this dimension — future route-matcher integration replaces the `matcher == nil` branch without changing the three-way return contract. Documented at ADR-0153 §Decision (ix) + §Consequences.

**Counter-increment nil-tolerance per ADR-0085:** the `f.activeRC.stats != nil` guard wraps the `denied.Inc()` call; SendLocalReply still fires even when stats is nil. Verified at `TestResolveRequirement_DanglingName_NilStats_NoPanic`.

### Group 7 finalization tests (7 new)

Replaces the Task 2-planted `TestResolvePerRouteConfig_DanglingName_RuntimeResolve` t.Skip stub with 7 real tests:

1. `TestResolvePerRouteConfig_DanglingName_RuntimeResolve` — dangling-name 403 wire shape verification: status=403, body=`"Failed JWT authentication: Wrong requirement_name: missing-req"`, headers=nil, `denied++` counter increment via fs.denied.Load() == 1.
2. `TestResolveRequirement_DanglingName_NilStats_NoPanic` — nil-tolerance per ADR-0085: when listener-level stats is nil, resolveRequirement still emits SendLocalReply without panicking on the denied counter increment.
3. `TestResolveRequirement_PerRouteRequirementName_Success` — case (c) happy path: per-route requirement_name resolves; returns (req, false); no SendLocalReply.
4. `TestResolveRequirement_PerRouteCaseB_FallsThroughToListenerRules` — case (b) disabled:false / requirementName=="" → falls through to listener-level rules iteration; matched rule's requirement returned.
5. `TestResolveRequirement_NoPerRoute_FallsThroughToListenerRules` — f.perRoute==nil → identical to case (b).
6. `TestResolveRequirement_NoRulesNoMatch_PassThrough` — empty rules slice → (nil, false); no SendLocalReply.
7. `TestResolveRequirement_FirstMatchWins` — listener-level rules iteration first-match-wins (with `matcher==nil` wildcard).

Mock dcb pattern: `jwtFakeCB` implements `envoyhttp.DecoderFilterCallbacks` with mutex-guarded `localReply *jwtLocalReplyArgs` capture (mirrors phase-16 rbac `rbacFakeCB` precedent). Test helper `newFilterWithListenerRC(t, rc, pr)` wires a `*filter` against a supplied listener-level `*compiledConfig` + per-route `*compiledPerRoute` + fresh `jwtFakeCB`.

### Test results

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
91

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- SKIP'
3

$ go test -race -count=1 ./internal/filter/http/jwtauthn/... 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.117s
```

91 PASS + 3 SKIP + 0 FAIL. PASS count rose by 7 vs Task 6 (84 → 91); SKIP count dropped by 1 (4 → 3) as the dangling-name runtime-resolve case landed. The 3 remaining SKIPs are: Group 6 (Task 9) + Group 8 (Task 9) + Group 12 (Task 8).

### Targeted run per PLAN Task 7 acceptance

```
$ go test -race -count=1 ./internal/filter/http/jwtauthn/ -run 'TestParsePerRoute|TestBuildCompiledPerRoute|TestResolvePerRouteConfig|TestPerRouteRuntimeResolve|TestResolveRequirement' -v ; echo "exit: $?"
... (all PASS) ...
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.020s
exit: 0
```

All per-route + runtime-resolve tests PASS under the PLAN Task 7 acceptance regex pattern.

### `go vet ./internal/filter/http/jwtauthn/...`

```
$ go vet ./internal/filter/http/jwtauthn/... ; echo "exit: $?"
exit: 0
```

### Whole-repo regression check (`go test -count=1 -short ./...`)

```
$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok '
46

$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL'
0
```

46 packages PASS (unchanged from Task 6); 0 FAIL.

### ADR-0153 verification

```
$ grep -nE '^## ADR-0153' docs/envoy-go/DECISIONS.md | wc -l
1

$ grep -cE '\*\*Lands-in-task:\*\* Task 7' docs/envoy-go/DECISIONS.md
3
```

1 ADR-0153 anchor match (per phase-13/14/15/16 ADR-anchor pattern); 3 `**Lands-in-task:** Task 7` matches across the ADR header + §Decision (vi) cross-reference + §Decision (i) preamble. Status flipped from `Anticipated` → `Accepted`. §Decision body authored with 10 numbered items per phase-13/14/15/16 §Decision-shape precedent. §Consequences body authored with 10 items including: file landings (LoC deltas); test count (7 Group 7 finalization tests); t.Skip stub replacement note; impl-time divergence from SPEC §6.6's `(req, errMsg)` sketch (consolidated to `(req, denied bool)` per §Decision (viii)); matcher==nil wildcard placeholder per ADR-0149 §Decision (ix); ClearRouteCache forward-defer to Task 9; cross-reference roster.

**Task 7 commit SHA:** `b9cde4d` (squash-base before Task 7 follow-up `1d92934`).

---

## Task 8 — Stat surface finalization + §11.P7 RATIFIED-PENDING closure (DEFERRED to Task 13) + Group 12 tests [ADR-0154]

Per PLAN Task 8 (lines 447-478) + ADR-0154 (§Context landed at SPEC commit; §Decision + §Consequences body filled at this commit). The stat-surface machinery — `newFilterStats` + the 7-counter `filterStats` struct + `baseStatPrefix` helper — landed wholly at Task 2 (commit `c1bbfd8`) per ADR-0148 + ADR-0149; Task 8's substantive deliverables are:
1. The Group 12 stat-surface integration tests (5 new cases) pinning the wiring at the stat-API level.
2. The ADR-0154 §Decision + §Consequences body fill in DECISIONS.md (8 §Decision items + 11 §Consequences items).
3. The §11.P7 RATIFIED-PENDING-IMPL-TIME-EMPIRICAL-SCRAPE closure disposition: **DEFERRED to Task 13** fixture 0019 end-to-end driver run, per planner-time decision 10 + phase-13 ADR-0127-v2 in-place-amend precedent (PLAN Task 8 Step 5 Option B — "defer to Task 13 fixture 0019 end-to-end scrape where the full fixture infrastructure exists"; documented in ADR-0154 §Decision (vi)).

NO source-code amendments to `internal/filter/http/jwtauthn/jwtauthn.go` at Task 8 — the wiring landed correctly at Task 2 + was verified against the §Decision items via the Group 12 tests at this commit.

### File changes (Task 8)

| File | LoC delta | Disposition |
|---|---|---|
| `internal/filter/http/jwtauthn/jwtauthn_test.go` | 2402 → 2680 LoC (+278) | Group 12 stats-namespace tests (5 new) + `collectMetricNames` + `containsString` helpers |
| `docs/envoy-go/DECISIONS.md` | +50 LoC | ADR-0154 §Decision (8 items) + §Consequences (11 items); Status: Accepted; Lands-in-task: Task 8; ADR-0154 header (was missing) inserted between ADR-0153 §Consequences final bullet and the ADR-0154 §Context block landed at SPEC |
| `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` | (this entry) | Task 8 narrative |

### §11.P7 RATIFIED-PENDING-IMPL-TIME-EMPIRICAL-SCRAPE closure disposition — Option B DEFERRED to Task 13

PLAN Task 8 Step 5 offered two paths:
- **Option A:** Start reference Envoy v1.37.2 at Task 8 with a probe yaml (simplified single-provider config); scrape `/stats/prometheus`; verify shape against `http.<HCM_stat_prefix>.jwt_authn.<counter>` SN2-reuse hypothesis.
- **Option B:** Defer to Task 13 fixture 0019 end-to-end run, where the fixture infrastructure already provides reference-Envoy + envoy-go side-by-side scrape.

**Task 8 elects Option B** for two structural reasons (documented at ADR-0154 §Decision (vi)):
1. Tasks 11-13 build the fixture infrastructure that Option A would otherwise reinvent — the 0019 fixture driver (Task 11) + envoy.yaml + jwksbackend test-helper (Tasks 10/12) provide the reference-Envoy + envoy-go side-by-side scrape harness verbatim. Running a one-off reference Envoy spin-up at Task 8 would reinvent that harness pre-fixture-infrastructure.
2. The hypothesis is CARRIED by ADR-0148 + ADR-0154 + SPEC §11.P7 with high pre-impl confidence — phase-14 compressor + phase-15 bandwidth_limit + phase-16 rbac all RATIFIED SN2-reuse for their own counter surfaces; the precedent body is strong. The §11.P7 deferral does NOT introduce additional risk vs Option A — the in-place-amend release-valve closes any divergence at Task 13 with an SN10 rule + `baseStatPrefix` rewrite if needed.

**Deferral path at Task 13:** when the fixture 0019 driver scrapes `/stats/prometheus` against reference Envoy v1.37.2 with the actual listener config, the counter-name lines are captured verbatim into the fixture's expectations + PROGRESS.md Task 13 entry. If the actual shape matches `http.<HCM_stat_prefix>.jwt_authn.<counter>`, ADR-0154 §Decision (vi) amends in-place to flip the RATIFIED-PENDING marker to RATIFIED (phase-16 ADR-0145 §11.P7 closure precedent — landed at impl-time-scrape with empirical evidence quoted inline). If the shape DIVERGES, ADR-0154 §Decision (ii) + (vi) amend in-place to introduce a new SN10 flattening rule + the `baseStatPrefix` helper rewrites to match. Either outcome lands at Task 13 with in-place amendment per planner-time decision 10 + phase-13 ADR-0127-v2 in-place-amend precedent.

### Group 12 tests (5 new)

Replaces the Task-2-planted `TestGroup12_StatsNamespace_LandAtTask8` t.Skip stub with 5 real tests, per PLAN Task 8 Step 1 + ADR-0154 §Decision:

1. `TestFilterStats_AllSevenCountersRegistered` — verifies the 7 SN2-reuse counter names land on the Registry verbatim at New() time + Registry size==7 (no per-provider scaling per §1.1 amendment 9). The 7 names: `http.ingress_http.jwt_authn.{allowed, denied, cors_preflight_bypassed, jwks_fetch_success, jwks_fetch_failed, jwt_cache_hit, jwt_cache_miss}`.
2. `TestFilterStats_NilRegistry_NoPanic` — ADR-0085 nil-tolerance verification at the filter-instance level: under nil `ctx.Stats` the `cc.stats` field is nil; the `resolveRequirement` dangling-name path's `f.activeRC.stats != nil` guard fires; SendLocalReply still emits 403 + body without panicking.
3. `TestFilterStats_CanonicalCorsPreflightBypassedName` — pins canonical naming `cors_preflight_bypassed` per §1.1 amendment 10 + asserts the REFUTED inverse BRAINSTORM hypothesis name `bypassed_cors_preflight` MUST NOT appear in the Registry. Re-anchors the Group 1 `TestBuildCompiledConfig_CorsPreflightBypassed_CanonicalNaming` at the Group 12 anchor — load-bearing for the §11.P7 empirical-scrape closure.
4. `TestFilterStats_JwksFetchCountersWired` — re-anchors the Group 10 Task-6 evaluator-side counter-increment assertions at the stat-API level: counter-handle Name() asserts the SN2-reuse shape verbatim; Inc() → Load() observable + Walk-output set membership confirmed.
5. `TestFilterStats_CacheCountersRegisteredButUnreachable` — pins ADR-0154 §Decision (iii) + §8 deferral 8: `jwt_cache_hit` + `jwt_cache_miss` counters are STRUCTURALLY UNREACHABLE under MVP — registered (Walk-observable; handles non-nil) yet NEVER incremented post deny-path exercise (Load() stays 0).

Test helpers added: `collectMetricNames(reg)` returns the Registry's full registered-name set via Walk (mirrors phase-16 rbac/rbac_test.go's `collectMetricNames` precedent); `containsString(haystack, needle)` exact-match contains check. Both helpers package-scoped to the test file — no production-code dependency.

### Test results

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
97

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- SKIP'
2

$ go test -race -count=1 ./internal/filter/http/jwtauthn/... 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.142s
```

97 PASS + 2 SKIP + 0 FAIL. PASS count rose by 6 vs Task 7 (91 → 97; 5 new Group 12 tests + 1 anchor); SKIP count dropped by 1 (3 → 2) as the Group 12 stub was replaced. The 2 remaining SKIPs are: Group 6 (Task 9) + Group 8 (Task 9).

### Targeted run per PLAN Task 8 acceptance

```
$ go test -race -count=1 ./internal/filter/http/jwtauthn/ -run 'TestFilterStats_' -v ; echo "exit: $?"
=== RUN   TestFilterStats_AllSevenCountersRegistered
--- PASS: TestFilterStats_AllSevenCountersRegistered (0.00s)
=== RUN   TestFilterStats_NilRegistry_NoPanic
--- PASS: TestFilterStats_NilRegistry_NoPanic (0.00s)
=== RUN   TestFilterStats_CanonicalCorsPreflightBypassedName
--- PASS: TestFilterStats_CanonicalCorsPreflightBypassedName (0.00s)
=== RUN   TestFilterStats_JwksFetchCountersWired
--- PASS: TestFilterStats_JwksFetchCountersWired (0.00s)
=== RUN   TestFilterStats_CacheCountersRegisteredButUnreachable
--- PASS: TestFilterStats_CacheCountersRegisteredButUnreachable (0.00s)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.011s
exit: 0
```

All 5 Group 12 tests PASS under the PLAN Task 8 acceptance regex pattern.

### `go vet ./internal/filter/http/jwtauthn/...`

```
$ go vet ./internal/filter/http/jwtauthn/... ; echo "exit: $?"
exit: 0
```

### Whole-repo regression check (`go test -count=1 -short ./...`)

```
$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok '
46

$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL'
0
```

46 packages PASS (unchanged from Task 7); 0 FAIL.

### ADR-0154 verification

```
$ grep -nE '^## ADR-0154' docs/envoy-go/DECISIONS.md | wc -l
1

$ grep -cE '\*\*Lands-in-task:\*\* Task 8' docs/envoy-go/DECISIONS.md
4
```

1 ADR-0154 anchor match (per phase-13/14/15/16 ADR-anchor pattern); 4 `**Lands-in-task:** Task 8` matches across the ADR header (Lands-in-task), the §Decision preamble, and 2 cross-reference sites. Status flipped from `Anticipated` → `Accepted`. §Decision body authored with 8 numbered items per ADR-0153 §Decision-shape precedent (covering: 7-counter unconditional registration; SN2-reuse namespace; 2 STRUCTURALLY UNREACHABLE counters; canonical naming; SHARED-stats per ADR-0153; §11.P7 RATIFIED-PENDING deferral to Task 13; jwks_fetch counter semantic disposition; cross-reference roster). §Consequences body authored with 11 items including: stat-surface count delta (64 → 71 names); §11.P7 deferral disposition; STRUCTURALLY UNREACHABLE allocation cost; divergence-window from Envoy; factoryState shape simplification; `NewCounter` vs `NewCounterIfAbsent` discipline; Group 12 test count (5 new tests); no source-code amendments to jwtauthn.go; BEHAVIOR_CONTRACT forward-pointer; Prometheus surface semantics; forward-compat for Task 9.

### §11.P7 RATIFIED-PENDING-IMPL-TIME-EMPIRICAL-SCRAPE — DISPOSITION RECORD

**Disposition:** DEFERRED to Task 13 fixture 0019 end-to-end run (Option B per PLAN Task 8 Step 5).

**Rationale:** Tasks 11-13 build the fixture infrastructure that Option A would otherwise reinvent. The hypothesis is CARRIED by ADR-0148 + ADR-0154 + SPEC §11.P7 with high pre-impl confidence (phase-14/15/16 SN2-reuse precedent body is strong). The in-place-amend release-valve closes any divergence at Task 13 with no additional risk vs Option A.

**Carry-forward to Task 13:** the fixture 0019 driver scrapes `/stats/prometheus` against reference Envoy v1.37.2 with the actual listener config; the counter-name lines are captured verbatim into PROGRESS.md Task 13 entry + the fixture's expectations file. At Task 13, ADR-0154 §Decision (vi) amends in-place (per planner-time decision 10 + phase-13 ADR-0127-v2 precedent):
- RATIFIES: flip RATIFIED-PENDING → RATIFIED inline.
- REFUTES: introduce SN10 flattening rule + rewrite `baseStatPrefix` helper to match empirical shape.

**Empirical-scrape probe disposition at Task 13:** no Task-8-time probe yaml was executed; the fixture 0019 listener config IS the probe at Task 13 (single-config produces the empirical evidence + the end-to-end differential test simultaneously — eliminates the dual-config maintenance burden Option A would have imposed).

**Task 8 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; capture pin recorded inline at the next task entry per the SHA-fill follow-up convention>

---

## Task 9 — DecodeHeaders body + deny-path wire shape + side-effect emit-order + Groups 5+6+8 tests [ADR-0155]

Per PLAN Task 9 (lines 482-514) + ADR-0155 (§Context landed at SPEC commit; §Decision + §Consequences body filled at this commit). Task 9 materializes the request-time dispatch surface — the `DecodeHeaders` body that composes Tasks 5/6/7's helpers — plus the deny-path wire shape (401/403 + canonical jwt_verify_lib body + RFC 6750 `WWW-Authenticate` Bearer challenge) plus the 4-step success-side-effect emit-order (strip → forward_payload_header → claim_to_headers → clear_route_cache).

### File changes (Task 9)

| File | LoC delta | Disposition |
|---|---|---|
| `internal/filter/http/jwtauthn/jwtauthn.go` | +97 / -16 | DecodeHeaders body replaces Task-2 STUB; `clearRouteCacheRequested` TRACKING field added on `*filter` |
| `internal/filter/http/jwtauthn/provider.go` | +328 / -16 | 8 new helpers: `applyResult` + `applySideEffects` + `emitDenyResponse` + `mapStatusToHTTPCode` + `encodeBase64URL` + `stripExtractionSources` + `stripQueryParam` + `stringify`; imports widened with `net/url` + `strconv` + `strings` + `envoyhttp` + `jwt` |
| `internal/filter/http/jwtauthn/jwtauthn_test.go` | +773 / -7 | Group 5 (14) + Group 6 (12) + Group 8 (15) tests; 2 t.Skip stubs removed; new helpers `buildTestFilterForDispatch` + `dispatchFixture` + `dispatchFixtureNoStats` + `validToken` + `buildBareFilterForEmit` |
| `docs/envoy-go/DECISIONS.md` | +106 LoC | ADR-0155 §Decision (9 items) + §Consequences (11 items); Status: Accepted; 3 `**Lands-in-task:** Task 9` anchors |
| `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` | (this entry) | Task 9 narrative |

### Implementation summary

**`DecodeHeaders` body in jwtauthn.go** — 6-step composition per SPEC §6.6 + §1.1 amendment 12:

1. Bind `f.activeRC = f.state.listenerRC` (per ADR-0153 SHARED-stats discipline — listener-level is the SOLE config instance; per-route delegates).
2. Capture `:path` → `f.originalURI` for the WWW-Authenticate realm (BEFORE any route mutation per §1.1 amendment 12).
3. Resolve per-route TPFC via `f.dcb.RequestRouteConfig()` + `factoryState.resolvePerRouteConfig`. Case (a) disabled:true → `passthrough=true` + `HeaderContinue` + NO counter increments per §5.3.
4. CORS preflight bypass per §11.P1: `bypass_cors_preflight=true` + `isCorsPreflightRequest(headers)` → `cors_preflight_bypassed++` + `HeaderContinue`.
5. Resolve requirement via `f.resolveRequirement(headers)` — provider.go Task 7's helper handles per-route case (c) dangling-name 403 (per ADR-0153 + §1.1 amendment 6) + listener-level rules first-match iteration.
6. Evaluate requirement via Task 6's `evaluateRequirement` → apply result via Task 9's `applyResult`.

**`applyResult` in provider.go** — 3-branch disposition per SPEC §6.9:

- DENIED → `denied++` + `emitDenyResponse(r.err)` + `HeaderStopIteration`.
- ALLOWED with token+provider populated → `allowed++` + `applySideEffects` (strip + forward_payload_header + claim_to_headers + clear_route_cache).
- ALLOWED with NO token (e.g., reqAllowMissing without extracted token) → `allowed++` + `HeaderContinue` (no side-effects without a surviving token to mutate against).

**`applySideEffects` 4-step emit-order** per SPEC §6.9 + §11.P10 + §11.P13:

1. Strip-on-success: when `!p.forward`, `stripExtractionSources` strips Authorization + access_token (defaults path) OR the configured from_headers/from_params (explicit path); from_cookies UNTOUCHED per proto caveat.
2. `forward_payload_header`: `encodeBase64URL(t.RawPayload, p.padForwardPayloadHdr)` + `headers.Set`.
3. `claim_to_headers`: `t.PayloadClaim(claimName)` dot-notation + `stringify(val)` + `headers.Set` with SILENT-SKIP on err (array claim per §11.P10 OR missing claim) OR non-stringifiable.
4. `clear_route_cache`: TRACKING flag flip on `f.clearRouteCacheRequested`. The framework primitive `cb.ClearRouteCache()` is deferred to a future HCM phase per ADR-0155 §Consequences item 8.

**`emitDenyResponse` + `mapStatusToHTTPCode`** per SPEC §4 + §6.9 + §1.1 amendments 8 + 11 + 12:

- `mapStatusToHTTPCode(reason)`: `errors.Is(reason, jwt.ErrJwtAudienceNotAllowed)` → 403; else (including nil) → 401.
- `strip_failure_response: true` → `SendLocalReply(code, "", nil)` per §11.P3.
- Else: body = `reason.Error()` (canonical jwt_verify_lib `getStatusString`); headers = `[www-authenticate: Bearer realm="<originalURI>"<, error="invalid_token">, content-type: text/plain]`. Realm fallback `"/"` when `originalURI == ""` (RFC 6750 §3 requires realm parameter).

**`encodeBase64URL` + `stripExtractionSources` + `stripQueryParam` + `stringify`** support helpers per §11.P10 + §11.P13 + planner-time decision 8:

- `encodeBase64URL(raw, pad)`: `pad=true` → append `=` until len % 4 == 0; `pad=false` → strip trailing `=` (RFC 7515 §2 unpadded form).
- `stripExtractionSources(headers, p)`: defaults path strips Authorization + `access_token` query; explicit path strips from_headers + from_params; from_cookies UNTOUCHED per proto caveat.
- `stripQueryParam(path, name)`: `url.ParseQuery` + `delete` + `Values.Encode` reassembly with fragment preservation.
- `stringify(val)`: scalar string/bool/float64/nil → ok; array/map → false (silent-skip).

### Group 5 tests (14 — DecodeHeaders dispatch integration)

1. `TestDecodeHeaders_PerRouteDisabled_Passthrough_NoCounters` — case (a) → HeaderContinue + NO counter increments.
2. `TestDecodeHeaders_CorsPreflight_Bypassed_CounterIncremented` — bypass_cors_preflight + OPTIONS+Origin+ACR-M → cors_preflight_bypassed++.
3. `TestDecodeHeaders_CorsPreflightDisabled_NotBypassed` — bypass false → CORS reaches validation; absent JWT → 401.
4. `TestDecodeHeaders_DanglingPerRouteName_403_Denied` — per-route requirement_name miss → 403 + canonical body via resolveRequirement.
5. `TestDecodeHeaders_NoRuleMatch_NoPerRoute_Passthrough_NoCounters` — empty rules + no per-route → HeaderContinue + NO counters.
6. `TestDecodeHeaders_ValidToken_RouteMatch_Allowed_HeaderContinue` — happy path → HeaderContinue + allowed++.
7. `TestDecodeHeaders_ExpiredToken_Denied_401` — expired → 401 + body "Jwt is expired" + denied++.
8. `TestDecodeHeaders_AudienceMismatch_Denied_403` — wrong aud → 403 + body "Audiences in Jwt are not allowed" per amendment 8.
9. `TestDecodeHeaders_MissingToken_Denied_401_NoErrorParam_WWWAuth` — no Authorization → 401 + WWW-Authenticate `Bearer realm="..."` (NO error param per §11.P2).
10. `TestDecodeHeaders_BadSignature_Denied_401_WithErrorParam_WWWAuth` — tampered sig → 401 + WWW-Authenticate with `, error="invalid_token"`.
11. `TestDecodeHeaders_OriginalURICapturedBeforeRouteMutation` — :path captured at entry → f.originalURI == :path.
12. `TestDecodeHeaders_StripFailureResponse_EmptyBody_NoWWWAuth` — strip_failure_response → 401 + body "" + nil headers.
13. `TestDecodeHeaders_PerRouteRequirementName_ValidToken_Allowed` — case (c) → named requirement → allowed.
14. `TestDecodeHeaders_PathMissing_OriginalURIEmpty_RealmDefaultsToSlash` — empty :path → defensive realm="/".

### Group 6 tests (12 — Side-effect emit-order)

1. `TestApplySideEffects_StripAuthorizationHeader_OnForwardFalse` — defaults path Authorization strip.
2. `TestApplySideEffects_AuthorizationRetained_OnForwardTrue` — forward=true → no strip.
3. `TestApplySideEffects_StripFromHeaders` — explicit from_headers stripped; defaults suppressed.
4. `TestApplySideEffects_StripFromParams_PathRewritten` — :path rewritten without the configured param.
5. `TestApplySideEffects_FromCookiesUntouched_PerProtoCaveat` — Cookie header preserved even on forward=false.
6. `TestApplySideEffects_ForwardPayloadHeader_PaddingTrue` — pad=true → trailing `=` retained; length % 4 == 0.
7. `TestApplySideEffects_ForwardPayloadHeader_PaddingFalse` — pad=false → no trailing `=`.
8. `TestApplySideEffects_ClaimToHeaders_StringClaim_Emitted` — string claim → header value verbatim.
9. `TestApplySideEffects_ClaimToHeaders_NumericClaim_Stringified` — integral float64 → integer string.
10. `TestApplySideEffects_ClaimToHeaders_ArrayClaim_SilentSkip` — array claim → header NOT emitted; loop continues.
11. `TestApplySideEffects_ClaimToHeaders_NestedDotNotation` — `user.email` → nested map traversal.
12. `TestApplySideEffects_ClearRouteCache_TriggersDcbInvocation` — `clear_route_cache:true` → `f.clearRouteCacheRequested` flips true (framework primitive deferred).
13. `TestApplySideEffects_EmitOrder_StripBeforeForwardBeforeClaimBeforeClear` — combined-action smoke verifying all 4 steps.

### Group 8 tests (15 — Deny-path wire shape)

1. `TestEmitDenyResponse_JwtMissed_401_BodyByteExact_NoErrorParam` — 401 + body "Jwt is missing" (14B) + realm-only WWW-Authenticate.
2. `TestEmitDenyResponse_JwtExpired_401_BodyByteExact_WithErrorParam` — 401 + body "Jwt is expired" (14B) + WWW-Authenticate with error param.
3. `TestEmitDenyResponse_JwtVerificationFail_401_BodyByteExact` — body "Jwt verification fails".
4. `TestEmitDenyResponse_JwtUnknownIssuer_401_BodyByteExact` — body "Jwt issuer is not configured".
5. `TestEmitDenyResponse_JwtAudienceNotAllowed_403_BodyByteExact` — **403** + body "Audiences in Jwt are not allowed" (32B).
6. `TestEmitDenyResponse_JwtBadFormat_401_BodyByteExact` — body matches jwt.ErrJwtBadFormat sentinel.
7. `TestEmitDenyResponse_JwtHeaderNotImplementedAlg_401_BodyByteExact` — body "Jwt header [alg] is not supported".
8. `TestEmitDenyResponse_StripFailureResponse_EmptyBody_NoHeaders` — body="" + nil headers; status preserved.
9. `TestEmitDenyResponse_RealmCapturedFromOriginalURI` — long :path with `?with=query` → realm captures verbatim.
10. `TestEmitDenyResponse_RealmDefaultsToSlash_WhenURIEmpty` — empty originalURI → realm="/" defensive default.
11. `TestMapStatusToHTTPCode_AudienceNotAllowed_403` — ErrJwtAudienceNotAllowed → 403.
12. `TestMapStatusToHTTPCode_OtherErrors_401` — 7 other sentinels → 401.
13. `TestMapStatusToHTTPCode_NilReason_401` — nil → 401 defensive default.
14. `TestEmitDenyResponse_ContentTypeTextPlain` — content-type: text/plain on non-stripped path.

### Test results

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
138

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | awk '/^--- SKIP/{s++} END{print s+0}'
0

$ go test -race -count=1 ./internal/filter/http/jwtauthn/... 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	1.112s
```

138 PASS + 0 FAIL + 0 SKIP. PASS count rose by 41 vs Task 8 (97 → 138; 14 Group 5 + 13 Group 6 + 14 Group 8 = 41 new tests); SKIP count dropped from 2 → 0 (Group 6 + Group 8 stubs replaced).

### Targeted run per PLAN Task 9 acceptance

```
$ go test -race -count=1 ./internal/filter/http/jwtauthn/ -run 'TestEvaluateRequirement|TestApplyResult|TestApplySideEffects|TestDecodeHeaders|TestEmitDenyResponse|TestMapStatusToHTTPCode|TestStripExtractionSources|TestEncodeBase64URL|TestStringify' -v ; echo "exit: $?"
... (all PASS) ...
exit: 0
```

### `go vet ./internal/filter/http/jwtauthn/...`

```
$ go vet ./internal/filter/http/jwtauthn/... ; echo "exit: $?"
exit: 0
```

### Whole-repo regression check (`go test -count=1 -short ./...`)

```
$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok '
46

$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL'
0
```

46 packages PASS (unchanged from Task 8); 0 FAIL.

### ADR-0155 verification

```
$ grep -nE '^## ADR-0155' docs/envoy-go/DECISIONS.md | wc -l
1

$ grep -cE '\*\*Lands-in-task:\*\* Task 9' docs/envoy-go/DECISIONS.md
3
```

1 ADR-0155 anchor match; 3 `**Lands-in-task:** Task 9` matches (ADR header + §Decision preamble + §Consequences preamble). Status flipped from `Anticipated` → `Accepted`. §Decision body authored with 9 numbered items covering: `mapStatusToHTTPCode` dispatch; canonical jwt_verify_lib body; WWW-Authenticate Bearer challenge; strip_failure_response semantic; response_code_details divergence-window; per-route runtime-resolve 403 path; 4-header standard set; keep-alive disposition; clear_route_cache framework primitive deferral. §Consequences body authored with 11 items including: byte-exact body parity; WWW-Authenticate byte-exact; strip_failure_response parity; per-route distinguished from validation-deny; Group 5+6+8 test coverage; 2 divergence-windows (response_code_details + clear_route_cache joint-closure pointer); operator-subtle realm-after-mutation; empty-:path defensive default; keep-alive convergence; 8 cross-references (ADR-0085 / ADR-0102 / ADR-0146 / ADR-0148 / ADR-0149 / ADR-0151 / ADR-0153 / ADR-0154).

### Framework deferrals carried by Task 9

Two divergence-windows are documented at ADR-0155 §Consequences as forward-pointers, joint-closure expected at a future HCM response-code-details framework primitive phase:

1. **`response_code_details` field emission** — Envoy emits `jwt_authn_access_denied{<sanitized_failure_reason>}` on the local-reply path; envoy-go's 3-arg `SendLocalReply(status, body, headers)` has no slot. Joint-closure with phase-16 ADR-0146 `rbac_access_denied_matched_policy[<id>]` divergence-window (a SINGLE primitive serves both filter families).

2. **`cb.ClearRouteCache()` framework primitive** — phase-17 MVP materializes `clear_route_cache: true` as the TRACKING flag `f.clearRouteCacheRequested`. The actual route-cache flush is deferred. The TRACKING flag IS the test-introspection anchor (Group 6 case 12); production sees a no-op until the framework primitive lands.

### Task 9 commit SHA

**Task 9 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; capture pin recorded inline at the next task entry per the SHA-fill follow-up convention>

## Task 10 — main.go register + fixture infrastructure + FuzzJwtAuthnConfigParse (21st fuzzer) + jwksbackend test-helper

Wires the jwt_authn filter into the boot registry, lands the fixture-runner infrastructure for fixture 0019 (BackendKind enum + switch-case stub), authors the 21st repo-wide fuzzer (`FuzzJwtAuthnConfigParse`), and creates the NEW shared `test/helpers/jwksbackend/` test-helper per planner-time decision 12 (D7 settlement) — the SECOND consumer of the shared-helper pattern.

### Files changed

- `cmd/envoy-go/main.go` (modified; +2 lines) — added `internal/filter/http/jwtauthn` import alphabetical-after `header_mutation` and the `httpReg.Register(jwtauthn.TypeURL, jwtauthn.New)` line alphabetical-after `header_mutation.Register` and before `localratelimit.Register`. The post-edit registration block reads `router → bandwidthlimit → buffer → compressor → cors → csrf → envoygotest → fault → header_mutation → jwtauthn → localratelimit → rbac → header_mutation.RegisterPerRouteValidator → httpReg.Freeze()`. Per ADR-0100 §2.2 + ADR-0148 §Decision alphabetical discipline. Verified: `grep -cE 'httpReg.Register' cmd/envoy-go/main.go` returns 12.

- `test/differential/fixture/fixture.go` (modified; +9 lines) — added `HTTPJwtAuthn BackendKind = 16` enum value after `HTTPRbac BackendKind = 15` with the documented "reuses echobackend helper + NEW jwksbackend helper; plaintext-only per SPEC §7.4" doc-comment. Verified: `grep -nE 'HTTPJwtAuthn' test/differential/fixture/fixture.go` returns 1.

- `test/differential/runner_test.go` (modified; ~40 lines) — added the commented-out `_ "github.com/esalaine/envoy-go/test/fixtures/0019-http-jwt-authn/inputs"` blank-import with a TODO pointing to Task 11 (the inputs package doesn't exist yet — the blank-import flips on when Task 11 lands); added the `case fixture.HTTPJwtAuthn:` switch-arm reusing the existing `startEchoBackend` helper (mirrors the phase-16 HTTPRbac precedent). The full jwksbackend lifecycle invocation is deferred to Task 11 where the driver authors `setupJWKSBackend` directly per the per-scenario JWK-Set payload requirement.

- `internal/filter/http/jwtauthn/fuzz_test.go` (created; ~280 LoC) — 21st repo-wide fuzzer. 13-seed corpus covering the six JwtAuthentication outer fields, the 13 consumed JwtProvider fields, the 8 silent-ignored fields, all 4 extraction sources, claim_to_headers dot-notation, inline `requires` + named requirement_map resolution, all 6 JwtRequirement variants with recursive RequiresAny/RequiresAll nesting, filter_state_rules silent-ignore, both outer-flags combined, empty config, and clear_route_cache + claim_to_headers combination. The fuzz body asserts the (factory, nil) | (nil, err) contract per ADR-0018 + ADR-0148. Mirrors phase-16 rbac fuzzer structural shape.

- `test/helpers/jwksbackend/doc.go` (created; ~22 LoC) — package doc enumerating the in-process HTTP JWKS server's purpose, lifecycle (spawn-per-fixture, runner-allocated free port, Stop at teardown), and API surface (New / Addr / Stop). Plaintext-only per SPEC §7.4.

- `test/helpers/jwksbackend/jwksbackend.go` (created; ~95 LoC) — `Server` struct with `listener + srv + addr + mu/closed` fields. `New(ctx, addr, routes)` binds a TCP listener, builds a route → JWK-Set-JSON body `http.ServeMux`, plumbs the supplied ctx via BaseContext, and starts `srv.Serve(ln)` in a goroutine. `Addr()` returns the resolved bound address (load-bearing for "127.0.0.1:0" ephemeral allocation). `Stop()` is idempotent — first call invokes `Shutdown` with a 5-second deadline; subsequent calls are no-ops. Non-GET methods return 405; missing paths return 404 (Go's default `http.ServeMux` behavior).

- `test/helpers/jwksbackend/jwksbackend_test.go` (created; ~155 LoC) — 6 unit tests: (1) `TestNew_StartsServerOnConfiguredAddr_ServesRoutes` — verifies bind on 127.0.0.1:0 + ephemeral port allocation + route serves byte-exact body; (2) `TestServer_RouteServesJWKSetJSON_ByteExact` — byte-exact body + Content-Type: application/json; (3) `TestServer_MissingRoute_Returns404` — unconfigured path → 404; (4) `TestServer_MultiRouteDispatch` — 3 routes; per-path correct body; (5) `TestServer_Stop_ClosesListener` — pre-Stop GET succeeds; Stop returns nil; post-Stop GET fails (connection refused / EOF / reset accepted); (6) `TestServer_Stop_Idempotent` — second Stop() returns nil.

- `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` (modified; this entry).

### Verification

```
$ go build ./cmd/envoy-go/ ; echo "exit: $?"
exit: 0

$ grep -cE 'httpReg.Register' cmd/envoy-go/main.go
12

$ grep -nE 'HTTPJwtAuthn' test/differential/fixture/fixture.go | head -2
270:	// HTTPJwtAuthn reuses the existing echobackend helper for upstream routes +
278:	HTTPJwtAuthn BackendKind = 16
```

Build succeeds; Register count is 12 (was 11 pre-Task-10: router + 10 filters; +1 for jwtauthn); HTTPJwtAuthn enum value lands at the expected position.

### jwksbackend unit-test results

```
$ go test -race -count=1 ./test/helpers/jwksbackend/... 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/test/helpers/jwksbackend	1.010s
```

6 PASS / 0 FAIL / 0 SKIP.

### Full jwtauthn suite (regression — no change beyond +1 fuzzer seed-corpus entry)

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
139

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0
```

139 PASS (was 138 at Task 9; +1 for `FuzzJwtAuthnConfigParse` seed-corpus regression entry).

### Fuzzer 30s targeted run per PLAN Task 10 acceptance

```
$ go test -fuzz=FuzzJwtAuthnConfigParse -fuzztime=30s ./internal/filter/http/jwtauthn/ 2>&1 | tail -5
fuzz: elapsed: 27s, execs: 6052032 (232288/sec), new interesting: 366 (total: 379)
fuzz: elapsed: 30s, execs: 6613504 (187161/sec), new interesting: 387 (total: 400)
fuzz: elapsed: 31s, execs: 6613504 (0/sec), new interesting: 387 (total: 400)
PASS
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn	31.149s
```

6.6M executions; 387 new-interesting inputs derived from the 13-seed corpus; clean exit (PASS). No invariant violations found.

### Pre-existing 20 fuzzers seed-corpus regression

```
$ go test -count=1 -run Fuzz ./... 2>&1 | grep -cE '^FAIL'
0
```

All 21 fuzzers (20 pre-existing + 1 new) PASS at seed-corpus regression level.

### Whole-repo regression check (`go test -count=1 -short ./...`)

```
$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok '
47

$ go test -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL'
0
```

47 packages PASS (was 46 at Task 9; +1 for `test/helpers/jwksbackend`); 0 FAIL.

### `go vet` regression

```
$ go vet ./... ; echo "exit: $?"
exit: 0
```

### Task 10 commit SHA

**Task 10 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; capture pin recorded inline at the next task entry per the SHA-fill follow-up convention>

---

## Task 11 — Fixture 0019 driver — 8 scenarios incl. RS256-RemoteJwks + ES256-LocalJwks + CORS preflight + per-route 8th canonical

Lands the 8-scenario fixture driver for `0019-http-jwt-authn` mirroring the phase-15/16 driver shape. All scenarios use HTTP/1.1 plaintext (no H2 differential coverage per SPEC §7.4); no mTLS in phase 17. The driver compiles + the fixture is registered; the runner's blank-import for `0019-http-jwt-authn/inputs/` is flipped on. End-to-end execution lands at Task 13 (needs YAMLs from Task 12 + PKI generator from Task 12 + expectations from Task 13).

### Files changed

- `test/fixtures/0019-http-jwt-authn/inputs/driver.go` (created; ~946 LoC) — the 8-scenario fixture driver. Implements `fixture.Driver` + `fixture.BackendKindAware` + `fixture.StatsAsserter`. Single-listener (l_test_a) plaintext fixture; reuses the shared echobackend helper for upstream-echo routes + the NEW jwksbackend helper (phase-17 Task 10) for in-process JWKS-serving. JWKS port allocated lazily at first `ReferenceBootstrap`/`SubjectConfig` call so the bootstrap templates can templatize the host:port deterministically; per-side jwksbackend server spawn + teardown inside `DriveReference`/`DriveSubject` (per-side lifecycle isolates the ref + subj windows). Scenarios:

  1. `runScenario1_valid_RS256_RemoteJwks_allow` — RS256 token signed by RS256Key1 for provider-rs256; first request triggers JWKS fetch; expect 200 echo + counter `allowed +1`, `jwks_fetch_success +1`.
  2. `runScenario2_valid_ES256_LocalJwks_allow` — ES256 token signed by ES256Key for provider-es256 (LocalJwks inline; no fetch); expect 200 echo + `allowed +1`.
  3. `runScenario3_missing_token_deny` — no Authorization header; expect 401 + body byte-exact `"Jwt is missing"` (14B) + WWW-Authenticate `Bearer realm="/"` (no error param per §1.1 amendment 12 JwtMissed case) + `denied +1`.
  4. `runScenario4_expired_token_deny` — token with exp in the past; expect 401 + body byte-exact `"Jwt is expired"` (14B) + WWW-Authenticate `Bearer realm="/", error="invalid_token"` + `denied +1`.
  5. `runScenario5_bad_signature_deny` — token signed with TamperedKey (signature mismatches the public JWK); expect 401 + body byte-exact `"Jwt verification fails"` (22B) + WWW-Authenticate `Bearer realm="/", error="invalid_token"` + `denied +1`.
  6. `runScenario6_bypass_cors_preflight` — OPTIONS / with Origin + Access-Control-Request-Method (NO Authorization); expect 200 echo + `cors_preflight_bypassed +1` (no allowed/denied increments).
  7. `runScenario7_per_route_requirement_name` — GET /alt-req with token for provider-alt; per-route TPFC `requirement_name: "alt-req"` resolves via listener `requirement_map`; expect 200 echo + `allowed +1`.
  8. `runScenario8_per_route_disabled` — GET /per-route-disabled with NO Authorization; per-route TPFC `disabled: true` passthrough; expect 200 echo; NO counter increments per SPEC §7.1 row 8 + §1.1 amendment 5.

  Byte-stream emit shape (the runner CompareBytes pass enforces ref/subj equivalence):

  ```
  scenario 1 status=200 body=ok
  scenario 2 status=200 body=ok
  scenario 3 status=401 body=ok www-authenticate=ok
  scenario 4 status=401 body=ok www-authenticate=ok
  scenario 5 status=401 body=ok www-authenticate=ok
  scenario 6 status=200 body=ok
  scenario 7 status=200 body=ok
  scenario 8 status=200 body=ok
  ```

  Body classification: allow scenarios (1, 2, 6, 7, 8) assert structural property (body non-empty); deny scenarios (3, 4, 5) assert byte-exact against the canonical jwt_verify_lib strings. WWW-Authenticate classification: scenarios 3-5 byte-exact against the canonical form (scenario 3 omits the error param; 4 + 5 include it).

  `AssertStats` scrapes `/stats/prometheus` from both admin endpoints and is plumbed for the 5 active base counters (allowed, denied, cors_preflight_bypassed, jwks_fetch_success, jwks_fetch_failed) per SPEC §7.5. Per-side expected values + cross-side equivalence assertions are STUBBED at Task 11 (silent at zero-counter state) — Task 13 finalizes the empirical Prometheus stat-name format + actual deltas. The 2 jwt_cache_* counters are STRUCTURALLY UNREACHABLE under phase-17 MVP per §1.1 amendment 9 + §8 deferral 8 and NOT asserted.

  `loadFixturePKI` returns a Task 11 PLACEHOLDER zero-valued `fixturePKI{}`; Task 12 will land `test/fixtures/0019-http-jwt-authn/pki/gen.go` whose init() populates the fields (RSA-2048 + ECDSA-P256 keypairs + JWK Set JSON serialization). Until Task 12 lands the generator, the per-scenario token signing fails at the `pki.<KeyField> == nil` guard with a clear "PKI not ready (Task 12 lands pki/gen.go)" error message — by design at Task 11 (driver compiles + fixture is registered; full execution gated on Task 12 + 13).

- `test/fixtures/0019-http-jwt-authn/inputs/sign.go` (created; ~91 LoC) — `signTokenRS256` + `signTokenES256` JWT signing helpers + `jwksbackendNewImpl` wrapper around the shared `test/helpers/jwksbackend` package. The sign helpers mirror the test pattern at `internal/jwt/jwt_test.go`'s `signRS` + `signES` (header → base64url + payload → base64url + signature → base64url; ECDSA r||s padded to curve byte size per JWS §A); fixture-local copies so the driver does not depend on internal/jwt test code.

- `test/differential/runner_test.go` (modified; -5/+1 lines) — flipped on the `_ "github.com/esalaine/envoy-go/test/fixtures/0019-http-jwt-authn/inputs"` blank-import (was commented out at Task 10 with the TODO pointing at Task 11). The blank-import's init() triggers the driver's `fixture.RegisterFixture("0019-http-jwt-authn", &jwtAuthnDriver{})` so the runner discovers the fixture at suite startup. The HTTPJwtAuthn switch-case (lands at Task 10) now exercises end-to-end with the registered driver.

- `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` (modified; this entry).

### Verification

```
$ go build ./test/fixtures/0019-http-jwt-authn/inputs/... ; echo "exit: $?"
exit: 0

$ grep -cE 'RegisterFixture\("0019-http-jwt-authn"' test/fixtures/0019-http-jwt-authn/inputs/driver.go
1

$ go build ./... ; echo "exit: $?"
exit: 0
```

Build succeeds; the literal RegisterFixture call grep returns 1 per the acceptance.

### Whole-repo regression (`go test -count=1 -short ./...`)

```
$ go test -race -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok '
47

$ go test -race -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL'
0
```

47 packages PASS (unchanged from Task 10 — fixture inputs has no test files of its own; the driver IS the test code); 0 FAIL.

### jwtauthn package regression

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
139

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0
```

139 PASS / 0 FAIL — unchanged from Task 10.

### `go vet` regression

```
$ go vet ./... ; echo "exit: $?"
exit: 0
```

### Task 11 commit SHA

**Task 11 commit SHA:** `4bd686a` — `phase 17 Task 11: Fixture 0019 driver — 8 scenarios incl. RS256-RemoteJwks + ES256-LocalJwks + CORS preflight + per-route 8th canonical` (filled at Task 12 entry-time per the SHA-fill follow-up convention).

---

## Task 12 — Fixture 0019 YAMLs + RSA/ECDSA PKI gen + JWK Set serialization

Lands the fixture-0019 bootstrap YAMLs (reference Envoy + envoy-go) + the PKI generator that produces fresh RSA-2048 + ECDSA-P256 keypairs + RFC 7517 §5 JWK Set JSON serializations at fixture-load time. The driver's `loadFixturePKI()` placeholder from Task 11 is replaced with the real `pki.Generate()` invocation, populating the per-test PKI material that the 8-scenario token signing path consumes.

### Files changed

- `test/fixtures/0019-http-jwt-authn/pki/gen.go` (created; ~252 LoC). Stdlib-only PKI generator per planner-time decision 11 + SPEC §7.3. Exports `FixturePKI` (5 keypairs + 4 kids + 3 JWK Set JSON strings) + `Generate() (*FixturePKI, error)`. Five keypairs:
  - `RS256Key1` (rs256-1) → provider-rs256, scenarios 1, 4.
  - `RS256Key2` (rs256-2) → spare per SPEC §7.3 (added to provider-rs256's JWK Set so the multi-key kid-lookup path is exercised).
  - `RS256Key3` (alt-1) → provider-alt, scenario 7.
  - `ES256Key` (es256-1) → provider-es256, scenario 2 (LocalJwks; no JWKS fetch).
  - `TamperedKey` (no kid; reuses RS256Kid1 in the JWS header) → scenario 5 bad-signature. Signing with a different private key than the public modulus in the JWK Set deterministically hits the bad-signature path rather than the kid-mismatch path.

  JWK Set serialization (RFC 7517 §5):
  - `RS256JWKSet` = `{"keys":[<RS256Key1 pub>, <RS256Key2 pub>]}` (served at `/.well-known/jwks-rs.json`).
  - `AltProviderJWKSet` = `{"keys":[<RS256Key3 pub>]}` (served at `/.well-known/jwks-alt.json`).
  - `ES256JWKSet` = `{"keys":[<ES256Key pub>]}` (substituted into envoy.yaml + envoy-go.yaml `LocalJwks.inline_string` at driver bootstrap-render time).

  RSA modulus n + exponent e base64url-encoded per RFC 7518 §6.3.1.1. ECDSA P-256 x + y coordinates padded with leading zeros to the curve byte size (32 bytes) per JWS §A then base64url-encoded per RFC 7518 §6.2.1.2. Stdlib-only: `crypto/rsa` + `crypto/ecdsa` + `crypto/elliptic` + `crypto/rand` + `encoding/base64` + `encoding/json` + `math/big` (no third-party JWT/JWK libraries per ADR-0151 framework-primitive discipline). Wallclock cost ~250-500ms for the 4 RSA-2048 keygens; driver memoizes via `sync.Once` so the cost is amortized across both proxy sides + all 8 scenarios.

- `test/fixtures/0019-http-jwt-authn/envoy.yaml` (created; ~164 LoC). Reference Envoy bootstrap per SPEC §7.2 + Task 11 driver wiring. Single plaintext listener `l_test_a` on port 10019 with HCM filter chain `envoy.filters.http.jwt_authn → envoy.filters.http.router`. Listener-level `JwtAuthentication` config carries:
  - 3 providers: `provider-rs256` (RemoteJwks via `c_jwks_backend` at `/.well-known/jwks-rs.json`, cache_duration 300s) + `provider-es256` (LocalJwks inline_string `{{.LocalJWKSES256Inline}}`) + `provider-alt` (RemoteJwks via `c_jwks_backend` at `/.well-known/jwks-alt.json`, cache_duration 300s).
  - 1 rule: `prefix: /` → `requires_any: [provider-rs256, provider-es256]` (covers scenarios 1-6).
  - 1 requirement_map entry: `alt-req: {provider_name: provider-alt}` (referenced by scenario 7's per-route TPFC).
  - `bypass_cors_preflight: true` per SPEC §7.1 row 6.

  Three routes:
  - `/` → cluster `c_backend` (scenarios 1-6 hit the default listener-level rule).
  - `/alt-req` → cluster `c_backend` + per-route TPFC `PerRouteConfig{requirement_name: "alt-req"}` per ADR-0125 §(xiii) case (b) (scenario 7).
  - `/per-route-disabled` → cluster `c_backend` + per-route TPFC `PerRouteConfig{disabled: true}` per ADR-0125 §(xiii) case (a) (scenario 8).

  Two clusters:
  - `c_backend`: STRICT_DNS to `{{.BackendHost}}:{{.BackendPort}}` (host.docker.internal per ADR-0010); `dns_lookup_family: V4_ONLY` per phase-14/15/16 precedent.
  - `c_jwks_backend`: STRICT_DNS to `{{.JWKSHost}}:{{.JWKSPort}}` (host.docker.internal); same family + timeout.

  Template substitution keys: `AdminPort`, `LATestPort`, `BackendHost`, `BackendPort`, `JWKSHost`, `JWKSPort`, `LocalJWKSES256Inline`.

- `test/fixtures/0019-http-jwt-authn/envoy-go.yaml` (created; ~140 LoC). Functionally equivalent to envoy.yaml modulo cluster type `STATIC` (vs STRICT_DNS) + cluster endpoint addresses `127.0.0.1` (vs host.docker.internal — envoy-go runs in-process on the host so no docker-bridge translation is needed). Listener/route/JwtAuthentication shape is byte-identical to envoy.yaml so the differential gate has a clean equivalence claim across the 8-scenario matrix.

- `test/fixtures/0019-http-jwt-authn/inputs/driver.go` (modified; -8/+22 lines). Replaces the Task 11 `loadFixturePKI()` placeholder with the real `pki.Generate()` invocation. The driver's `fixturePKI` struct is populated field-by-field from the returned `*pki.FixturePKI` via the `sync.Once`-protected memoization path. Panics on keygen failure (no error-return surface in the driver's lifecycle hooks).

- `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` (modified; this entry).

### YAML lint verification — reference Envoy v1.37.2 `--mode validate`

Both rendered YAMLs (after `mustRender` substitution with concrete port/host values) validate clean against `envoyproxy/envoy:v1.37.2`:

```
$ docker run --rm -v /tmp:/data envoyproxy/envoy:v1.37.2 -c /data/envoy_test_0019.yaml --mode validate 2>&1 | tail -3
[info][config] [...configuration_impl.cc:148] loading 1 listener(s)
[info][config] [...configuration_impl.cc:164] loading stats configuration
configuration '/data/envoy_test_0019.yaml' OK
```

Same exit-OK for the envoy-go.yaml rendered counterpart. The bootstrap structure (admin + 1 listener + 2 clusters + 3 providers + 1 requires_any rule + 1 requirement_map entry + 2 per-route TPFC entries) parses + type-resolves clean under reference Envoy's protobuf schema.

Note: the validation uses the rendered output (template placeholders substituted with concrete values); the raw YAML files contain Go-template `{{...}}` directives that reference Envoy cannot parse directly. Production validation runs through the driver's `mustRender` pass first.

### Build + vet regression

```
$ go build ./... ; echo "exit: $?"
exit: 0

$ go vet ./... ; echo "exit: $?"
exit: 0
```

### jwtauthn package regression

```
$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- PASS'
139

$ go test -race -count=1 -v ./internal/filter/http/jwtauthn/... 2>&1 | grep -cE '^--- FAIL'
0
```

139 PASS / 0 FAIL — unchanged from Task 11 (Task 12 adds no test files to internal/filter/http/jwtauthn).

### Whole-repo regression (`go test -race -count=1 -short ./...`)

```
$ go test -race -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^ok '
47

$ go test -race -count=1 -short -timeout=180s ./... 2>&1 | grep -cE '^FAIL'
0
```

47 packages PASS / 0 FAIL. The new `test/fixtures/0019-http-jwt-authn/pki/` package carries no test files of its own (the generator's correctness is exercised at Task 13's end-to-end differential fixture run); it appears in the test output as `? ... [no test files]`.

### `pki.Generate()` smoke-check

Standalone smoke-test (rendered output truncated for the log):

```
Kids: rs256-1 rs256-2 alt-1 es256-1
RS256 JWKS: {"keys":[{"kty":"RSA","kid":"rs256-1","alg":"RS256","use":"sig","n":"<…2048-bit modulus…>","e":"AQAB"},
                     {"kty":"RSA","kid":"rs256-2","alg":"RS256","use":"sig","n":"<…>","e":"AQAB"}]}
Alt JWKS:   {"keys":[{"kty":"RSA","kid":"alt-1","alg":"RS256","use":"sig","n":"<…>","e":"AQAB"}]}
ES256 JWKS: {"keys":[{"kty":"EC","kid":"es256-1","alg":"ES256","use":"sig","crv":"P-256",
                     "x":"<32-byte base64url>","y":"<32-byte base64url>"}]}
```

JWK Set JSON shape matches the RFC 7517 §5 + RFC 7518 §6.{2,3} grammar; both `mustRender` template passes (envoy.yaml + envoy-go.yaml) succeed with the substituted PKI + port values.

### Task 12 acceptance grep

```
$ grep -nE 'pki\.Generate' test/fixtures/0019-http-jwt-authn/inputs/driver.go | wc -l
1

$ ls test/fixtures/0019-http-jwt-authn/{envoy.yaml,envoy-go.yaml,pki/gen.go} 2>&1 | wc -l
3
```

All three required artifacts present; the driver's `pki.Generate()` invocation grep returns 1 per the acceptance.

### Task 12 commit SHA

**Task 12 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; capture pin recorded inline at the next task entry per the SHA-fill follow-up convention>

---

## Task 13 — Fixture 0019 expectations + README + end-to-end differential pass (20 fixtures green) + §11.P7 RATIFIED closure

Lands the fixture-0019 `expectations.yaml` + `README.md`, finalizes the driver's `AssertStats` counter assertions against the empirical reference scrape, closes the §11.P7 RATIFIED-PENDING pin deferred from Task 8, and brings all 20 differential fixtures (0000–0019) to green. This task also folds in the filter-package empirical refinements that the end-to-end debugging surfaced (the differential gate is the first place the full envoy-go ↔ reference-Envoy wire comparison runs).

### The realm divergence + the Host-header pin

The first end-to-end run failed on the WWW-Authenticate header byte stream:

```
first divergence at offset 143
ref  [...]: realm=\"http://localhost:44495/...
subj [...]: realm=\"http://[::]:37425/...
```

Root cause: reference Envoy v1.37.2's jwt_authn reflects the request's FULL URL into the WWW-Authenticate `realm` (`<scheme>://<authority><path>`), NOT the bare path the pre-Task-13 driver constants assumed. The differential harness reaches the reference proxy at the testcontainers-mapped `localhost:<port>` and the envoy-go subject at its dual-stack `[::]:<port>` — so the Go HTTP client stamps a DIFFERENT `Host` header on each side, and the realm (faithfully reflecting Host) diverges per-side by construction.

Fix (driver-side, `inputs/driver.go`): `doRequest` now pins `req.Host = "jwt-authn.fixture.test"` (the new `fixtureHostHeader` const) on every scenario request. Both proxies then observe the identical authority; the realm is byte-equivalent. The `wwwAuthMissing` / `wwwAuthInvalid` constants are updated to the full-URL form `Bearer realm="http://jwt-authn.fixture.test/"` (+ `, error="invalid_token"` for the non-JwtMissed scenarios 4/5). The filter's full-URL realm construction (`<scheme>://<authority><path>`, authority from `:authority` ⊳ `host`, scheme from `x-forwarded-proto` ⊳ `:scheme` ⊳ `"http"`) was authored by the prior session's Task-13 work and is the correct shape per the empirical scrape — only the driver needed the Host pin.

### Filter-package empirical refinements (folded into this commit)

The end-to-end debugging surfaced three filter-package refinements (all in `internal/filter/http/jwtauthn/`):

1. **`requires_any` error selection — most-informative-error (evaluator.go).** The Task-6 "last failure error" rule could let a later kid-mismatch from a non-matching provider overwrite an earlier expired/verify-fail from the matching provider — producing a misleading body string. Replaced with `mergeRequiresAnyErr` + `requiresAnyErrPriority`: JWT-semantic errors (expired / verify-fail / audience / issuer …) outrank JWKS lookup misses, which outrank `ErrJwtMissed`; on tie the EARLIER error wins (mirrors Envoy verifier.cc's first-encountered-preserve discipline).
2. **`compiledRule.matcher any` → `compiledRule.matchFn func(string) bool` (jwtauthn.go + provider.go).** The Task-2 placeholder (`matcher any`, treated as wildcard) is replaced with a real `compileRouteMatch` covering the two `path_specifier` arms fixture 0019 exercises — `Path` (exact) + `Prefix` — mirroring phase-04's `internal/filter/hcm/config.go` `buildMatch`. SafeRegex / PathSeparatedPrefix / ConnectMatcher + the headers / query_parameters / runtime_fraction / dynamic_metadata / tls_context predicates are REJECTED at compile time with descriptive errors (fail-loud rather than silent-bypass). `resolveRequirement` now evaluates `matchFn(path)` for first-match-wins instead of treating every rule as wildcard. ADR-0149's `router.NewRouteMatcher` extraction deferral stands — `matchFn` is the interim shape.
3. **`jwks_fetch_success` credited at filter-load time (evaluator.go + jwtauthn.go).** Per the empirical scrape (see §11.P7 below): reference Envoy increments `jwks_fetch_success` once per RemoteJwks provider at filter-load time, not per request-time `Get()`. `buildCompiledConfig` now credits one increment per RemoteJwks provider whose initial blocking fetch succeeded; `evaluateProvider` increments only `jwks_fetch_failed` (on a `Get()` error). ADR-0154 §Decision (vii) + §Consequences amended in-place to record this; the Task-6 per-`Get()` disposition is superseded.

### §11.P7 RATIFIED-PENDING-IMPL-TIME-EMPIRICAL-SCRAPE — CLOSED RATIFIED

Per ADR-0154 §Decision (vi) Option-B deferral from Task 8. The fixture 0019 end-to-end run scrapes `/stats/prometheus` from both proxies. Verbatim scrape evidence:

```
=== ref jwt_authn stats ===
  envoy_http_jwt_authn_jwks_fetch_success{envoy_http_conn_manager_prefix="hcm_local_a"} = 2
  envoy_http_jwt_authn_jwt_cache_hit{envoy_http_conn_manager_prefix="hcm_local_a"} = 0
  envoy_http_jwt_authn_jwt_cache_miss{envoy_http_conn_manager_prefix="hcm_local_a"} = 9
  envoy_http_jwt_authn_allowed{envoy_http_conn_manager_prefix="hcm_local_a"} = 5
  envoy_http_jwt_authn_cors_preflight_bypassed{envoy_http_conn_manager_prefix="hcm_local_a"} = 1
  envoy_http_jwt_authn_denied{envoy_http_conn_manager_prefix="hcm_local_a"} = 3
  envoy_http_jwt_authn_jwks_fetch_failed{envoy_http_conn_manager_prefix="hcm_local_a"} = 0
=== subj jwt_authn stats ===
  envoy_http_jwt_authn_jwt_cache_miss{envoy_http_conn_manager_prefix="hcm_local_a"} = 0
  envoy_http_jwt_authn_allowed{envoy_http_conn_manager_prefix="hcm_local_a"} = 3
  envoy_http_jwt_authn_cors_preflight_bypassed{envoy_http_conn_manager_prefix="hcm_local_a"} = 1
  envoy_http_jwt_authn_denied{envoy_http_conn_manager_prefix="hcm_local_a"} = 3
  envoy_http_jwt_authn_jwks_fetch_failed{envoy_http_conn_manager_prefix="hcm_local_a"} = 0
  envoy_http_jwt_authn_jwks_fetch_success{envoy_http_conn_manager_prefix="hcm_local_a"} = 2
  envoy_http_jwt_authn_jwt_cache_hit{envoy_http_conn_manager_prefix="hcm_local_a"} = 0
```

**SN2-reuse RATIFIED verbatim.** Both proxies emit the IDENTICAL Prometheus form `envoy_http_jwt_authn_<counter>{envoy_http_conn_manager_prefix="hcm_local_a"}` — the internal name `http.hcm_local_a.jwt_authn.<counter>` flattening through the existing SN2 `http.*` default-branch route with the `envoy_http_conn_manager_prefix` label promoted. NO new SN10 rule, NO new tag-extractor, `baseStatPrefix` unchanged. ADR-0154 §Decision (vi) + §Consequences amended in-place: RATIFIED-PENDING → RATIFIED. The §11.P7 pin — the SOLE Task-8 RATIFIED-PENDING closure pin in phase 17 — is CLOSED.

### Counter divergence-windows surfaced empirically

The scrape surfaced two counter divergences, both documented (NOT bugs):

1. **`allowed` — ref 5 / subj 3.** Reference Envoy increments `allowed` on EVERY request that clears the filter gate, including CORS-preflight bypass (scenario 6) and per-route `disabled: true` passthrough (scenario 8): ref 5 = scenarios {1,2,7} actively-ALLOWED + {6,8} bypassed. envoy-go MVP increments `allowed` ONLY on an active-engine ALLOWED result per SPEC §3 + §1.1 amendment 5 ("PerRouteConfig{disabled: true} → no counter increments"): subj 3 = {1,2,7}. SPEC-mandated envoy-go behavior. `AssertStats` asserts `allowed` per-side (ref 5, subj 3), NOT cross-side.
2. **`jwt_cache_miss` — ref 9 / subj 0.** STRUCTURALLY UNREACHABLE under envoy-go MVP (`jwt_cache_config` silent-ignored per §8 deferral 8); reference's validated-JWT LRU cache is always active. `jwt_cache_hit` + `jwt_cache_miss` are NOT asserted.

The four byte-equivalent counters — `denied` (3=3), `cors_preflight_bypassed` (1=1), `jwks_fetch_success` (2=2), `jwks_fetch_failed` (0=0) — ARE cross-side-equivalence-asserted by `AssertStats`. Both divergence-windows are recorded in `expectations.yaml` + `README.md`.

### Files changed (Task 13)

| File | Disposition |
|---|---|
| `test/fixtures/0019-http-jwt-authn/expectations.yaml` | NEW — prose expectations + 8-scenario matrix + per-side counter-delta map + 7 documented divergence-windows (ADR-0019 — driver enforces). |
| `test/fixtures/0019-http-jwt-authn/README.md` | NEW — fixture overview + PKI notes + listener config + Host-header pin + 8-scenario narrative + counter-expectation table + divergence-window roster. |
| `test/fixtures/0019-http-jwt-authn/inputs/driver.go` | Host-header pin (`fixtureHostHeader` const + `req.Host` in `doRequest`); `wwwAuthMissing`/`wwwAuthInvalid` updated to the full-URL realm form; `AssertStats` finalized (cross-side equivalence on 4 counters + per-side on `allowed`; `jwt_cache_*` not asserted); package-doc refreshed for the eager-JWKS-start + Host-pin lifecycle; `fixtureName` const now consumed in `init()`. |
| `test/fixtures/0019-http-jwt-authn/{envoy.yaml,envoy-go.yaml}` | `cache_duration: { seconds: 300 }` → `cache_duration: 300s` (the prior session's YAML-syntax fix). |
| `internal/filter/http/jwtauthn/evaluator.go` | `requires_any` most-informative-error selection (`mergeRequiresAnyErr` + `requiresAnyErrPriority`); `jwks_fetch_success` per-`Get()` increment retired (load-time credit moves to `buildCompiledConfig`). |
| `internal/filter/http/jwtauthn/jwtauthn.go` | `compiledRule.matchFn` + `compileRouteMatch` (Path + Prefix arms; others rejected); `jwks_fetch_success` load-time credit in `buildCompiledConfig`; `originalURI` full-URL realm construction. |
| `internal/filter/http/jwtauthn/provider.go` | `resolveRequirement` evaluates `matchFn(path)` for first-match-wins. |
| `internal/filter/http/jwtauthn/jwtauthn_test.go` | `matcher:` → `matchFn:` field rename across tests; `TestEvaluateProvider_RemoteJwks_FetchSuccess_Counter` updated for the load-time-credit semantic; unused `remoteJwksProvider` helper removed. |
| `docs/envoy-go/DECISIONS.md` | ADR-0154 §Decision (vi) + (vii) + §Consequences amended in-place — §11.P7 RATIFIED; `jwks_fetch_success` load-time-credit semantic. |
| `internal/filter/http/jwtauthn/doc.go`, `internal/filter/http/jwtauthn/provider.go`, `internal/jwt/jwt_test.go`, `test/fixtures/0019-http-jwt-authn/pki/gen.go` | `gofmt -w` — pre-existing lint debt from Tasks 2/4/12. |
| `test/helpers/jwksbackend/jwksbackend.go` | `cancelling` → `canceling` (misspell — pre-existing lint debt from Task 10). |
| scratch artifacts removed | `cmd/__test_jwks/` (prior-session debugging program) + `envoy-go` (stray build binary). |
| `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` | this entry. |

### Gate A — build + vet + lint

```
$ go build ./... ; echo "exit: $?"
exit: 0
$ go vet ./... ; echo "exit: $?"
exit: 0
$ golangci-lint run ; echo "exit: $?"
exit: 0
```

Lint exit 0 — the pre-existing gofmt + unused + misspell debt from Tasks 2/4/10/12 is cleared (Gate A at Task 14 confirms).

### jwtauthn package regression

```
$ go test -race -count=1 ./internal/filter/http/jwtauthn/... ./internal/jwks/... ./internal/jwt/... ./test/helpers/jwksbackend/... 2>&1 | tail -4
ok  	github.com/esalaine/envoy-go/internal/filter/http/jwtauthn
ok  	github.com/esalaine/envoy-go/internal/jwks
ok  	github.com/esalaine/envoy-go/internal/jwt
ok  	github.com/esalaine/envoy-go/test/helpers/jwksbackend
```

### Gate E — full 20-fixture differential regression

```
$ go test -count=1 ./test/differential/ -run 'TestDifferential' 2>&1 | tail -1
ok  	github.com/esalaine/envoy-go/test/differential	56.678s
```

All 20 differential fixtures (0000–0019) PASS, including `0019-http-jwt-authn` with all 8 scenarios + the finalized `AssertStats` counter checks.

### Task 13 acceptance grep

```
$ ls test/fixtures/0019-http-jwt-authn/{expectations.yaml,README.md} 2>&1 | wc -l
2
```

Both Task 13 artifacts present; end-to-end differential green.

**Task 13 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; capture pin recorded inline at the next task entry per the SHA-fill follow-up convention>
---

## Task 14 — BEHAVIOR_CONTRACT.md 6-edit bundle + ROADMAP row 17 in-progress→done + STATE.md advance + 6-gate phase-done verification

Lands the SPEC §13 6-edit bundle into BEHAVIOR_CONTRACT.md, flips ROADMAP row 17 to `done`, advances STATE.md to lifecycle-state-4, and runs the phase-done six-gate verification per BOOTSTRAP_PROMPT.md §7.5.

### BEHAVIOR_CONTRACT.md 6-edit bundle (per SPEC §13)

- **§13.1 — NEW `### envoy.filters.http.jwt_authn` subsection.** Inserted LANDING-CHRONOLOGICAL (after `### envoy.filters.http.rbac`, before the `## HTTP filter chain` closing `### Applies to` block), NOT alphabetical. **Planner-time decision 14 fallback invoked:** PLAN Task 14 Step 1 + planner-time decision 14 anticipated an alphabetical-after-header_mutation insertion BUT instructed "if the existing subsection ordering is landing-chronological rather than alphabetical, fall back to landing-chronological insertion". BEHAVIOR_CONTRACT.md's `## HTTP filter chain` subsections are ordered landing-chronologically (fault@09 → header_mutation@10 → local_ratelimit@11 → csrf@12 → buffer@13 → compressor@14 → bandwidth_limit@15 → rbac@16), so the fallback fires: jwt_authn lands after rbac. The subsection covers the 6-listener-field map (5 consumed + filter_state_rules silent-ignored), the 13-of-21 JwtProvider consumed-field map + 8 silent-ignored, the 6-variant JwtRequirement evaluator, the RS+ES algorithm allow-list, the 4 token-extraction sources + iteration order, the 4-step side-effect emit-order, the deny-path wire shape (401/403 dispatch + canonical jwt_verify_lib body + WWW-Authenticate Bearer challenge + strip_failure_response), the 8th-canonical per-route + SHARED-stats discipline, and the stat surface.
- **§13.2 — Stat-name mapping 64 → 71.** NEW "jwt_authn filter — 7 names" block (5 active + 2 STRUCTURALLY UNREACHABLE) inserted before the total line; total updated 64 → 71 with the per-phase tally extended (`+ 7 from 17`).
- **§13.3 — Equivalence Matrix row.** NEW `0019-http-jwt-authn` row (3-cell rbac-row format) covering byte-exact body/status/WWW-Authenticate, cross-side counter equivalence on the 4 byte-equivalent counters, per-side `allowed` divergence-window, `jwt_cache_*` non-assertion, SHARED per-route stats.
- **§13.4 — NEW `### Phase 17 forward-pointer notes` subsection.** Appended under `## Forward-pointer notes` after the Phase 16 notes; enumerates the 17-deferral + 1-foot-gun list organized by 9 deferred-cluster families (dynamic-metadata / algorithm-extension / caching-framework / claim-coverage-extension / filter-state / response-code-details / access-log / CEL / operator-ergonomics) + the `allowed`-on-bypass counter divergence-window + the `strip_failure_response` divergence-window + the JwtRequirement-Set recursion-depth foot-gun + the TWO-new-framework-primitives note + the 8th-canonical note + the no-new-tag-extractor note.
- **§13.7 — NEW `## JWKS framework primitive` top-level section.** Placed after `## Matcher engine framework primitive`, before `## Forward-pointer notes`. Covers the `internal/jwks/` package shape, the `Fetcher` lifecycle (New / Get / Close), the 5s-before-TTL refresh schedule + fixed-1s failed-refetch, observability (load-time `jwks_fetch_success` credit per ADR-0154 §Decision (vii)), and the cross-phase reuse intent.
- **§13.8 — NEW `## JWT verifier framework primitive` top-level section.** Covers the `internal/jwt/` package shape, the `Token` lifecycle (Parse / VerifySignature / ValidateClaims / PayloadClaim), the RS+ES algorithm allow-list, the exp→nbf→iss→aud claim-validation order, the ~20 canonical error sentinels, and the cross-phase reuse intent.

§13.5 (no HTTPFilterCallbacks extensions) + §13.6 (ADR-0125 §(xiii) cross-reference) require no standalone edits — the §(xiii) cross-reference is carried inline in the §13.1 jwt_authn subsection + the §13.4 forward-pointer notes.

### ROADMAP row 17

Status `in-progress → done`; date `2026-05-14`; summary opening rewritten with post-impl LoC counts (`jwtauthn` 2399 + `internal/jwks` 703 + `internal/jwt` 653 + `jwksbackend` 127) + the ADR-0044-escape-valve-NOT-triggered disposition + the §11.P7 RATIFIED closure + the two Task-13 counter divergence-windows.

### STATE.md advance

Rewritten to lifecycle-state-4: lifecycle-state `phase 17 done; awaiting next planning`; next-skill `(none — phase complete)`; next-free ADR `ADR-0156`; last-updated `2026-05-14`; next-skill-scope rewritten for the next BRAINSTORM session (ROADMAP has no row 18 — the next session selects the next §9 family-row). last-commit left as a `<TBD>` placeholder for the post-`wt-merge` SHA-fill follow-up.

### Six-gate phase-done verification (per BOOTSTRAP_PROMPT.md §7.5 + SPEC §14.7)

**Gate A — build + vet + lint:**

```
$ go build ./...           # exit 0
$ go vet ./...             # exit 0
$ golangci-lint run        # exit 0 (0 lines output)
```

**Gate B — race tests across all packages:**

```
$ go test -race -count=1 $(go list ./... | grep -v 'test/differential')   # exit 0; 45 ok packages, 0 FAIL/panic/DATA RACE
ok  github.com/esalaine/envoy-go/internal/filter/http/jwtauthn   1.122s
ok  github.com/esalaine/envoy-go/internal/jwks                   2.699s
ok  github.com/esalaine/envoy-go/internal/jwt                    1.153s
ok  github.com/esalaine/envoy-go/test/helpers/jwksbackend        1.015s
$ go test -race -count=1 ./test/differential/ -run TestDifferential   # exit 0
ok  github.com/esalaine/envoy-go/test/differential   57.149s
```

All 45 non-differential packages (including the 4 new phase-17 packages) + the differential package pass clean under `-race`. NOTE: an earlier full-`./...`-under-`-race` sweep flaked on timing-sensitive differential fixtures (0013 / 0017 / 0018 — the known phase-15/16-precedent container-startup race exacerbated by race-detector overhead + concurrent Docker contention). Splitting the sweep into the non-differential set + the differential set run separately came up clean on both — the standard phase-16 Gate-B re-run-on-flake discipline.

**Gate C — h2spec 53/53 at ADR-0051 pin:**

```
$ go test -v -count=1 ./test/conformance/h2spec/
53 tests, 53 passed, 0 skipped, 0 failed
--- PASS: TestH2Spec
ok  github.com/esalaine/envoy-go/test/conformance/h2spec   2.566s
```

Phase 17 introduces no H2 wire-shape changes; the gate is unchanged at the ADR-0051 pin.

**Gate D — 21 fuzzers green at 30s each:**

```
$ /tmp/run_fuzz_17.sh   # 21 fuzzers sequenced, 30s each
=== FUZZ SUMMARY: 20 passed, 1 failed (of 21) ===
```

`FuzzPromTextFormat` (a pre-existing phase-06.1 fuzzer) reported `--- FAIL: context deadline exceeded` after 25.3M clean executions / 0 new-interesting / NO crash artifact written — the known Go-fuzzing coordinator↔worker grace-period flake (the coordinator's stop-and-flush deadline expired before the worker acknowledged, NOT a fuzz-found bug). Re-run in isolation came up clean:

```
$ go test -run='^$' -fuzz='^FuzzPromTextFormat$' -fuzztime=30s ./internal/stats/
fuzz: elapsed: 30s, execs: 25826601, new interesting: 0 (total: 120)
PASS
ok  github.com/esalaine/envoy-go/internal/stats   30.117s
```

**Gate D: GREEN — 21/21 fuzzers pass at 30s each.** The 21st fuzzer `FuzzJwtAuthnConfigParse` (phase 17, `internal/filter/http/jwtauthn/fuzz_test.go`) passed in the batch run with zero crashes across its 13-seed corpus + derived inputs.

**Gate E — 20 differential fixtures (0000–0019) PASS:**

```
$ go test -count=1 ./test/differential/ -run TestDifferential
ok  github.com/esalaine/envoy-go/test/differential   56.678s
```

(Re-confirmed under `-race` at Gate B: `ok ... 57.149s`.) All 20 fixtures including `0019-http-jwt-authn` (8 scenarios + the finalized AssertStats counter checks) pass.

**Gate F — BEHAVIOR_CONTRACT.md 6-edit bundle populated:**

```
$ grep -c '### envoy.filters.http.jwt_authn' docs/envoy-go/BEHAVIOR_CONTRACT.md      # 2 (1 header + 1 cross-ref)
$ grep -c '## JWKS framework primitive' docs/envoy-go/BEHAVIOR_CONTRACT.md           # 3 (1 header + 2 cross-refs)
$ grep -c '## JWT verifier framework primitive' docs/envoy-go/BEHAVIOR_CONTRACT.md   # 3 (1 header + 2 cross-refs)
$ grep -c '### Phase 17 forward-pointer notes' docs/envoy-go/BEHAVIOR_CONTRACT.md    # 3 (1 header + 2 cross-refs)
$ grep -c '0019-http-jwt-authn' docs/envoy-go/BEHAVIOR_CONTRACT.md                   # 1 (equivalence-matrix row)
$ grep -c 'Total: 71 internal names' docs/envoy-go/BEHAVIOR_CONTRACT.md              # 1 (stat-table total)
```

All 6 §13 patches landed.

### Six-gate summary

| Gate | Check | State | Evidence |
|---|---|---|---|
| A | build + vet + lint | GREEN | all exit 0 |
| B | race tests all packages | GREEN | 45 non-diff packages + differential clean under `-race` (split-sweep re-run discipline per phase-16 precedent) |
| C | h2spec 53/53 | GREEN | 53 tests, 53 passed, 0 failed |
| D | 21 fuzzers @ 30s | GREEN | 21/21 pass (FuzzPromTextFormat clean on isolated re-run) |
| E | 20 differential fixtures | GREEN | 0000–0019 PASS, 56.7s |
| F | BEHAVIOR_CONTRACT 6-edit bundle | GREEN | all 6 §13 patches landed |

### Files changed (Task 14)

| File | Disposition |
|---|---|
| `docs/envoy-go/BEHAVIOR_CONTRACT.md` | 6-edit bundle (§13.1 jwt_authn subsection + §13.2 stat-table 64→71 + §13.3 equivalence-matrix row + §13.4 Phase 17 forward-pointer notes + §13.7 NEW JWKS framework primitive section + §13.8 NEW JWT verifier framework primitive section). |
| `docs/envoy-go/ROADMAP.md` | row 17 `in-progress → done`; date `2026-05-14`; summary opening sharpened with post-impl counts. |
| `docs/envoy-go/STATE.md` | advanced to lifecycle-state-4 (`phase 17 done; awaiting next planning`); next-free ADR `ADR-0156`. |
| `docs/envoy-go/phases/17-http-filter-jwt-authn/PROGRESS.md` | this entry. |

**Task 14 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; capture pin recorded inline at the next task entry per the SHA-fill follow-up convention>
---

## Task 15 — REVIEW.md end-of-phase review

Authors `docs/envoy-go/phases/17-http-filter-jwt-authn/REVIEW.md` per the `superpowers:requesting-code-review` output template + the phase-13/14/15/16 REVIEW.md structural precedent. The review is inline-authored by the implementing session (this IMPL session resumed mid-phase at Task 13's in-progress state and carried through Task 15).

### REVIEW.md structure (11 sections, ~290 LoC)

1. **Phase summary** — APPROVED; the two cross-phase-reusable framework primitives as the architectural centerpiece; the DECODER-only pre-body request gate; the mid-phase-resume note + the three Task-13 filter-package empirical refinements.
2. **ADR roster** — ADR-0148..ADR-0155 + ADR-0125 §(xiii), each evaluated VALIDATED; ADR-0154 + ADR-0155 carry the Task-13 in-place amendments (§11.P7 RATIFIED closure; jwks_fetch_success load-time credit; full-URL realm).
3. **Empirical pins outcome** — 16 §11 pins (disposition tally per SPEC §15 claim 8); §11.P7 the SOLE RATIFIED-PENDING pin CLOSED RATIFIED at Task 13; the Task-13-discovered `allowed`-counter divergence-window.
4. **Gate-by-gate evidence** — all 6 gates GREEN at `421c0c1`, verbatim from the Task 14 PROGRESS entry.
5. **Acceptance checklist** — SPEC §15 15-claim verification; 15 PASS, 0 BLOCKED, 0 DONE_WITH_CONCERNS.
6. **Divergence-window roster** — 9 windows (a)-(i): `allowed`-on-bypass + `jwt_cache_*` unreachable + `response_code_details` + `filter_state_rules` + dynamic-metadata family + v1.37.x claim-coverage + `jwt_cache_config` no-cache + `clear_route_cache` implicit-trigger + `strip_failure_response` foot-gun.
7. **Framework-delta impact** — the two new primitives + cross-phase reuse intent (ext_authz / oauth2 / jwt_claim_router).
8. **Test counts** — ~138 jwtauthn + 42 jwt + 35 jwks + 6 jwksbackend unit tests; 20 fixtures; 21 fuzzers; h2spec 53/53; ~3882 production LoC.
9. **Deferred items** — 12 follow-ups (router.NewRouteMatcher extraction; jwt_cache_config; response_code_details; dynamic-metadata; filter_state_rules; claim-coverage extensions; HS/EdDSA/none/PS; CEL; clear_route_cache implicit-trigger; recursion-depth foot-gun; access-log operators; the 6 in-task follow-up commits squash-fold note).
10. **Phase-done lessons learned** — 5 lessons (ADR-0044 escape-valve NOT triggered; resumed-mid-phase sessions inherit in-progress work; the stat scrape surfaces counter-semantics divergences; the realm/Host-pin lesson; the §11.P7 deferral-to-Task-13 worked cleanly).
11. **Sign-off** — ready for `wt-merge`; 15 tasks × 20 commits at worktree HEAD; next session is a BRAINSTORM for the next §9 family-row.

### Verification

```
$ go build ./... ; echo "exit: $?"        # exit 0
$ go vet ./... ; echo "exit: $?"          # exit 0
$ golangci-lint run ; echo "exit: $?"     # exit 0
```

REVIEW.md is documentation-only (no source-code change); the Gate A sweep confirms no regression.

**Task 15 commit SHA:** <filled at commit time via `git log -1 --format=%H` post-commit; expected 40-char hash; the last-commit SHA-fill for STATE.md lands at the post-`wt-merge` master-side follow-up per the phase-09..16 close pattern>

---

## Phase 17 close

All 15 tasks complete. 20 commits at worktree HEAD (14 task-landing commits Tasks 1–14 + 6 in-task follow-ups) + this Task 15 commit = 21 at the post-commit HEAD. Phase-done six-gate verification GREEN at `421c0c1` (Task 14). REVIEW.md authored (Task 15). Ready for squash-merge to master via `wt-merge` + the post-merge STATE.md last-commit SHA-fill follow-up.
