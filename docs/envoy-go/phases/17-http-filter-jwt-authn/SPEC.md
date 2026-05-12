# Phase 17 SPEC — `envoy.filters.http.jwt_authn`

> **Lifecycle state:** SPEC.md authored; ROADMAP row 17 status stays `in-progress` (set during BRAINSTORM at phase-17 brainstorm commit) per `BOOTSTRAP_PROMPT.md` §4.1 invariant 3. Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09 / 10 / 11 / 12 / 13 / 14 / 15 / 16 precedent (BRAINSTORM → SPEC → PLAN → impl → review). This SPEC is the authoritative input to PLAN.

**Predecessors:** `BRAINSTORM.md` (this directory; 544 lines). §§1–11 are the pre-§9-empirical-pin design sketch (PRESERVED VERBATIM per D-3.5); the §11 empirical-pin block in this SPEC re-runs all 16 BRAINSTORM §10 pins (§17.P1–§17.P16) against reference Envoy v1.37.2 IN-SESSION per ADR-0004. NO post-landing BRAINSTORM §12 amendment cycle is authored — the empirical re-frame is structured for the §1.1 amendment-block channel (mirrors phase-12 csrf 4-amendment + phase-14 compressor 6-amendment + phase-15 bandwidth_limit 10-amendment + phase-16 rbac 12-amendment precedents rather than the phase-13 buffer §12 amendment-cycle precedent). NO off-master prebrainstorm-notes branch.

**ADR continuity:** Phase 16 closed at ADR-0147. Phase 17 anticipates ADR-0148..ADR-0155 (8 ADRs per BRAINSTORM §7 — tied with phase-16's 7+1=8 landed ADRs for LARGEST §9-row roster to date) + ADR-0125 amendment paragraph §(xiii) (for the NEW 8th canonical per-route pattern). Phase 17 ships these ADRs **anticipated** at SPEC time per ADR-0044 ADR-on-impl convention; impl session anchors each at the task it first lands in (mirrors phase-13 + phase-15 + phase-16 precedent; phase-14's SPEC-time-pre-landing of ADR-0129..ADR-0133 is the divergent precedent). Next-free ADR after phase 17 is ADR-0156.

**§3 framework-survey result up front (locks §3 TWO-framework-deltas claim):** Phase 17 is the SECOND CONSECUTIVE §9 family-row (after phase 16) to introduce TWO framework primitives simultaneously: (i) HTTP-outbound JWKS fetcher at NEW top-level `internal/jwks/` package — async fetch with `fast_listener` mode + LRU/per-thread JWK Set cache + scheduled refresh 5s before TTL expiry + fixed-interval failed-refetch retry (NOT exponential backoff; §11.P4 REFUTED BRAINSTORM hypothesis) + RetryPolicy support for the inner outbound HTTP request; cross-phase-reusable for future `ext_authz` HTTP-mode + `oauth2` token-endpoint flow; anchored at ADR-0150. (ii) JWS/JWT verifier at NEW top-level `internal/jwt/` package — parser + signature verifier for RS256/384/512 + ES256/384/512 + claims validator + nested-claim dot-notation extractor; cross-phase-reusable; anchored at ADR-0151. Both primitives are landed-in-phase-17 but explicitly CROSS-PHASE-REUSABLE. The §1.7 framework-delta accretion shape gains two new entries; ADR-0125 + ADR-0117 + ADR-0142 NOT amended on framework grounds.

---

## 1. Purpose

Phase 17 lands `envoy.filters.http.jwt_authn` — Envoy's canonical JWT bearer-token validation filter, RS+ES algorithm family, Both-JWKS-source proto-faithful, Full-6 JwtRequirement variants, All-4 token extraction sources, Full-header-side post-validation side-effects, 8th-canonical per-route delegation via requirement_map — as the TENTH production HTTP filter in envoy-go after cors (07.1), fault (09), header_mutation (10), local_ratelimit (11), csrf (12), buffer (13), compressor (14), bandwidth_limit (15), and rbac (16), and the TENTH top-level row under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family. Phase 17 is the SECOND CONSECUTIVE §9 family-row to introduce TWO framework primitives in a single phase per §3 framework-survey result above. The eight architectural primitives:

1. **New `internal/filter/http/jwtauthn/` package** owning the filter implementation. Package directory + Go-package identifier are both `jwtauthn` (single token underscore-stripped per ADR-0114 precedent; matches `localratelimit/` + `bandwidthlimit/` precedent for compound proto type-names; the proto type-URL preserves the underscore as `envoy.filters.http.jwt_authn` but the Go-package identifier strips it). Files mirror the phase-14 + phase-15 + phase-16 multi-file split (the precedent for larger filters): `jwtauthn.go` (filter type + factory + decode methods + filterStats struct + compiledConfig + per-route helper), `evaluator.go` (6-variant JwtRequirement evaluator + extraction-source iteration + RequirementRule dispatch), `provider.go` (JwtProvider compiled-state + algorithm allow-list + JWKS reference + extraction-source set + side-effect set), `jwtauthn_test.go` (unit tests; anticipated 1500-2500 LoC given the evaluator + provider + extraction-source + verification subsurface), `fuzz_test.go` (the 21st fuzzer in the repo — `FuzzJwtAuthnConfigParse`), `doc.go` (package overview + 8-decision summary + Both-JWKS + RS+ES + Full-6-Requirement + All-4-extraction + Full-header-side + 8th-canonical-per-route summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / localratelimit / csrf / buffer / compressor / bandwidthlimit / rbac precedent. ADR-0148 codifies.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 12 entries after phase 16: `router.New`, `bandwidthlimit.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `localratelimit.New`, `rbac.New` before the `httpReg.Freeze()` invocation) gains a thirteenth `httpReg.Register(jwtauthn.TypeURL, jwtauthn.New)` call before the freeze. Insertion alphabetical-after-router per the ADR-0100 §2.2 convention: `router → bandwidthlimit → buffer → compressor → cors → csrf → envoygotest → fault → header_mutation → jwtauthn → localratelimit → rbac → Freeze`. `jwtauthn` inserts between `header_mutation` and `localratelimit` to maintain alphabetical-after-router ordering. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

3. **MVP envelope: 6 listener-level consumed + 1 silent-ignored (REFINED from BRAINSTORM "5 consumed" claim per §1.1 amendment 1).** `envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication` (per `[#next-free-field: 7]` at `config.pb.go:1446`) has **6 top-level fields, not 5** as BRAINSTORM §1.1 hypothesized — `filter_state_rules` is the 6th field, SILENT-IGNORED in phase-17 MVP (couples to future filter-state-family phase). The 5 actively consumed fields per Q2 + Q4 + Q8 user picks:

   - **`providers`** (`map<string, JwtProvider>`; field 1) — provider config registry; each provider has issuer + audiences + JWKS source + algorithm allow-list + extraction-source set + side-effect set. Up to 21 fields per provider per `[#next-free-field: 22]` annotation (REFINED from BRAINSTORM "17 fields" claim per §1.1 amendment 2).
   - **`rules`** (`repeated RequirementRule`; field 2) — listener-level trigger map; each rule has `match` (`RouteMatch`; PGV REQUIRED per `config.pb.validate.go:1792-1801`; reuses phase-04 RouteMatch evaluator) and `requirement_type` oneof (`requires` inline JwtRequirement OR `requirement_name` string into requirement_map).
   - **`bypass_cors_preflight`** (bool; field 4) — skip JWT validation for CORS preflight (OPTIONS) requests; predicate per §11.P1 verbatim Envoy filter source: method == OPTIONS AND `Origin` header present AND `Access-Control-Request-Method` header present.
   - **`requirement_map`** (`map<string, JwtRequirement>`; field 5) — named-requirement registry; referenced by both listener-level rules AND the 8th-canonical per-route.
   - **`strip_failure_response`** (bool; field 6) — controls whether the failure body + WWW-Authenticate header are stripped from the 401 response. §11.P3 RATIFIES exact effect: body becomes empty string; www-authenticate header NOT set; `response_code_details` still emitted.

   **SILENT-IGNORED at outer level (1 field):**
   - `filter_state_rules` (`FilterStateRule`; field 3) — runtime CEL-like requirement selection based on `StreamInfo.FilterState` string accessor. SILENT-IGNORED in phase-17 MVP (couples to a future filter-state-family phase per §8 deferral 12). The proto parser accepts the field; the runtime evaluator never consults it. Operator divergence-window: configs setting `filter_state_rules` against Envoy use that path FIRST when `rules`-side matches yield no match, but envoy-go falls through to the no-match disposition (no JWT verification required). §1.1 amendment 1 documents.

   **Inside `JwtProvider` (per `[#next-free-field: 22]` at `config.pb.go:60`):** **13 fields consumed of 21 total** (REFINED from BRAINSTORM "13 of 17" framing per §1.1 amendment 2 — proto has 21 fields, not 17). Consumed in MVP: `issuer` (string; OPTIONAL per §11.P9 `config.pb.validate.go:61` `// no validation rules for Issuer`; if set, JWT's `iss` claim must match), `audiences` (repeated string; OR-semantic match; empty-means-skip-audience-check), `remote_jwks` (RemoteJwks → http_uri PGV REQUIRED per `config.pb.validate.go:577-586` + cache_duration PGV `[1ms, 2500000h)` per `config.pb.validate.go:631-637` + async_fetch + retry_policy), `local_jwks` (DataSource → inline_string / inline_bytes / filename; oneof exclusive-with remote_jwks via `JwksSourceSpecifier`), `forward` (bool; default `false` per proto comment "if false, JWT is removed in the request after a success verification"), `from_headers` (repeated JwtHeader{name, value_prefix}), `from_params` (repeated string), `from_cookies` (repeated string), `forward_payload_header` (string), `pad_forward_payload_header` (bool), `claim_to_headers` (repeated ClaimToHeader{header_name, claim_name}), `clear_route_cache` (bool), `clock_skew_seconds` (uint32; default 60s per proto comment line 281 — REFINED in §1.1 amendment 7; previously absent from BRAINSTORM consumed-list).

   **SILENT-IGNORED at JwtProvider (8 fields):** (i) `payload_in_metadata` — couples to dynamic-metadata family per §8 deferral 1; (ii) `normalize_payload_in_metadata` — couples to dynamic-metadata family per §8 deferral 4; (iii) `header_in_metadata` — couples to dynamic-metadata family per §8 deferral 2; (iv) `failed_status_in_metadata` — couples to dynamic-metadata family per §8 deferral 3; (v) `jwt_cache_config` — validated-JWT result LRU cache; MVP no-cache per §8 deferral 8 (default 100-entry cache per proto comment "default to 100"); (vi) `subjects` — StringMatcher on JWT `sub` claim; SILENT-IGNORED per §1.1 amendment 3 + §8 deferral 15 (NEW deferral introduced by amendment 3 — BRAINSTORM hypothesized claim validation honored sub; refuted by Envoy proto comment treating subjects as an optional v1.37.x addition); (vii) `require_expiration` — bool; if true, JWT MUST carry `exp` claim; SILENT-IGNORED per §1.1 amendment 3 + §8 deferral 16 (NEW deferral); (viii) `max_lifetime` — Duration; rejects JWTs whose `exp - now` exceeds this; SILENT-IGNORED per §1.1 amendment 3 + §8 deferral 17 (NEW deferral).

   **Inside `RequirementRule` (per `config.pb.go:1223`):** `match` (RouteMatch — reuses phase-04 evaluator; PGV REQUIRED per §11.P9) + `requirement_type` oneof: `requires` (inline JwtRequirement; DEPRECATED per Envoy filter source comment at filter_config.cc — actually NOT deprecated per scrape REFUTATION; honored at runtime per §1.1 amendment 4 + §11.P12 REFINED) OR `requirement_name` (string into requirement_map; PARSE-VALIDATED at envoy-go-side, lazy-RESOLVED at request-time per §11.P12). **§1.1 amendment 4 REFUTES BRAINSTORM §1.1 item 3 + §2.8 hypothesis** that deprecated `requires` is PARSE-REJECT envoy-go-only — empirical scrape of Envoy v1.37.2 filter_config.cc reveals `requires` is NOT marked deprecated in v1.37.2 proto annotations (the field carries no `[deprecated = true]` flag; Envoy filter source treats both arms equivalently). envoy-go MVP HONORS both arms proto-faithful. The §8 deferral 9 originally framed as "deprecated requires PARSE-REJECT" is WITHDRAWN.

   **Inside `JwtRequirement` (per `[#next-free-field: 7]` at `config.pb.go:950`; 6-variant oneof; consumed via the 8th-canonical per-route OR via listener-level rules' requires/requirement_name resolution):** all 6 oneof variants honored per Q4 Full-6:
   - `provider_name` (string) — references providers map by key
   - `provider_and_audiences` (ProviderWithAudiences{provider_name, audiences}) — per-rule audience override
   - `requires_any` (JwtRequirementOrList{requirements: []JwtRequirement}) — OR-semantic combinator (recursive)
   - `requires_all` (JwtRequirementAndList{requirements: []JwtRequirement}) — AND-semantic combinator (recursive)
   - `allow_missing_or_failed` (Empty) — JWT absent OR bad OK
   - `allow_missing` (Empty) — JWT absent OK; bad-JWT still rejects

   **Algorithm allow-list (six algorithms; Q3 RS + ES family):** `RS256` / `RS384` / `RS512` (RSASSA-PKCS1-v1_5 with SHA-256/384/512) + `ES256` / `ES384` / `ES512` (ECDSA with P-256/P-384/P-521 + SHA-256/384/512). Validation via Go stdlib `crypto/rsa.VerifyPKCS1v15` + `crypto/ecdsa.Verify`. JWK with `alg` claim outside the six allow-list PARSE-REJECTED at JWKS-parse time (envoy-go-strict); JWT with `alg` claim outside the six runtime-rejected as `JwtHeaderNotImplementedAlg` failure-reason per §11.P1 canonical string `"Jwt header [alg] is not supported"`. DEFERRED algorithm families per §8 deferrals 5-7: HS family (HMAC; requires symmetric-secret config plumbing); EdDSA; `none` (intentionally never enabled — security-sensitive).

4. **Per-route TPFC: NEW 8th canonical pattern (oneof{disabled(bool) | requirement_name(string)}; ADR-0125 amendment §(xiii) at SPEC commit).** **§1.1 amendment 5 REFUTES BRAINSTORM §2.7 + §4 hypothesis** that the 8th canonical uses `oneof{requirement_name(string) | disabled(Empty)}` — empirical scrape of `config.pb.go:1595-1679` reveals `PerRouteConfig.RequirementSpecifier` oneof has TWO arms: `disabled` of type **`bool`** (NOT `Empty` as hypothesized; varint at field 1) and `requirement_name` of type `string` (field 2; PGV `min_len=1` per `config.pb.validate.go:2460-2462`). The 8th canonical's correct form is:

   - (a) `PerRouteConfig{disabled: true}` → the filter is wholly inactive on this route, no JWT validation, no counter increments, request forwards as-is past the gate.
   - (b) `PerRouteConfig{requirement_name: "<name>"}` → reference-by-name into the listener-level `requirement_map`. Per §11.P12 REFINED: Envoy v1.37.2 RUNTIME-RESOLVES the named reference at request time via `findPerRouteVerifier()` map lookup; if name absent from map, returns 403 + error string (NOT parse-reject). envoy-go MVP **mirrors Envoy's runtime-resolve** (PARSE-REJECT divergence withdrawn per §1.1 amendment 6; the original BRAINSTORM PARSE-REJECT hypothesis would diverge needlessly from upstream's behaviour).
   - (c) `PerRouteConfig{disabled: false}` (the variant set but bool unset, defaulting to false) — PGV requires `oneof` to be set, so this case is structurally `PerRouteConfig{disabled: false}` with the oneof IS present (just carrying false). Operator intent here is ambiguous; envoy-go treats `disabled: false` as "the per-route override exists but explicitly does NOT disable — fall through to listener-level rules evaluation as if no per-route override were set". This matches Envoy's filter_config.cc behaviour per §11.P12.

   **Phase 17 is the FIRST row to use the string-reference-delegation discipline** — distinct from the 5th canonical (explicit `disabled` boolean oneof + wholesale-override sub-message; phase-13/14), the 6th (bare-message-via-TPFC + code-level-required field; phase-15), and the 7th (wrapper with reserved field + single optional sub-message, absent-implies-disabled; phase-16). ADR-0125 gains an in-place amendment paragraph §(xiii) codifying the 8th canonical pattern: the per-route does NOT carry its own provider/requirement config — it delegates by name to a listener-level requirement_map entry. The delegation pattern is symmetric with the listener-level `RequirementRule.requirement_name` (both reference the same `requirement_map`). This is a NEW canonical pattern, NOT an extension of the 7th: the 7th canonical's defining feature is the absence-implies-disabled-via-proto-comment with wholesale-override on presence; the 8th canonical's defining feature is the string-reference-delegation into a separately-named registry. Both shapes honored in MVP. Each TPFC entry runs through `parsePerRoute` at config-load time → produces a `*compiledPerRoute` value. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific per-route entry per request; that entry's shape (`disabled=true` OR `requirement_name`-resolved) drives the disposition.

   **Per-route stats SHARED with listener-level** per §5 + §11.P8 RATIFIED: per-route just delegates by name (or disables); per-route does NOT carry its own stat_prefix (no `rules_stat_prefix`-equivalent at jwt_authn; empirically confirmed by filter_config.cc scrape — `findPerRouteVerifier()` contains no stats-related logic). Per-route override emits to the same stat namespace as the listener-level config. DIVERGES from phase-11 / phase-15 / phase-16 INDEPENDENT-stats; MIRRORS phase-12 / phase-13 / phase-14 SHARED-stats discipline. The stateful-policy-evaluation-per-route concern (which motivated INDEPENDENT-stats at phase-11/15/16) does NOT apply here — jwt_authn's per-route is a pure delegation, not a stateful policy-evaluator clone.

5. **Filter-callback shape: `StreamDecoderFilter` ONLY on the `*filter` instance.** Phase 17 is decode-side only — `jwt_authn` is a request-gate filter, evaluated at `DecodeHeaders` time, with the disposition (allow / deny) computed BEFORE the request body is forwarded. The filter does NOT implement `StreamEncoderFilter`. Static blank-identifier compile-time check for `StreamDecoderFilter` only. The decode-side surface: `DecodeHeaders(headers, endStream)` resolves per-route → caches `*compiledPerRoute` on filter state → runs the JwtRequirement evaluator (which iterates extraction sources, validates the JWT against the resolved provider's algorithm + JWKS, applies side-effects) → emits the appropriate counter delta → returns `HeaderContinue` (allow) OR invokes `cb.SendLocalReply(<status>, <body>, <www-authenticate header>)` and returns `HeaderStopIteration` (deny). `DecodeData` + `DecodeTrailers` pass-through. `OnDestroy` no-op for synchronous resolutions; for `RemoteJwks` on-demand fetches that arrive after request close, the background goroutine writing to the per-stream callback must be guarded (per §6.8 RemoteJwks lifecycle discipline). Listener-level RequirementRule evaluation also fires at `DecodeHeaders` when per-route is unset — the filter iterates listener-level rules, evaluates each rule's RouteMatch against the request, and applies the first-matching rule's `requires` (inline) OR `requirement_name` (via map) per the listener-level dispatch table.

6. **Validation algorithm (Decision #4 → ADR-0149 + ADR-0151).** Per Q3 + Q4 picks:

   **Extraction step** (per provider's extraction-source set; §11.P14 + §11.P15 + extractor.cc scrape RATIFIES proto-faithful behaviour):
   1. Authorization Bearer header (always honored when no explicit per-provider extraction-sources set, OR honored alongside explicit sources per Envoy's "all defaults plus configured" extractor design).
   2. `from_headers[]` (in declared order; first match wins within the source set; `value_prefix` substring-search per extractor.cc line `value_str.find(location_spec->value_prefix_)`).
   3. `from_params[]` (query string; URL-decoded value via `QueryParamsMulti::parseAndDecodeQueryString` — §11.P14 RATIFIES URL-decode; case-sensitive param-name match; multi-value query `?token=a&token=b` extracts ONLY first value via `getFirstValue` per §11.P14 REFINED).
   4. `from_cookies[]` (cookie name; case-sensitive exact match per §11.P15 RATIFIES; cookie value used verbatim — no URL-decode).

   Note (§11.P14 + §11.P15 RATIFIED): default extraction sources (Authorization Bearer + `access_token` query param) apply when the provider's extraction-source set is wholly empty.

   **Decoding step** (JWT structural parse; delegated to `internal/jwt` framework primitive per ADR-0151):
   - Split on `.` into 3 parts (header.payload.signature); reject if not exactly 3 parts (`JwtBadFormat` failure-reason; canonical string `"Jwt is not in the form of Header.Payload.Signature..."`).
   - Base64url-decode header + payload; parse as JSON.
   - Validate `alg` claim in header against provider's algorithm allow-list (RS256/384/512 + ES256/384/512); reject if outside (`JwtHeaderNotImplementedAlg` failure-reason).
   - Validate `kid` claim in header (if present) against JWKS keys; pick matching key OR fall back to first key with matching `alg` (per Envoy's pickKeyAlgWithKid logic; matches `JwksKidAlgMismatch` reject-path if no match).

   **Signature verification step** (Decision #4 → ADR-0151 framework primitive):
   - Reconstruct signed-bytes (`<base64url-header>.<base64url-payload>`).
   - Verify signature using stdlib `crypto/rsa.VerifyPKCS1v15` (for RS) or `crypto/ecdsa.Verify` (for ES). Reject if invalid (`JwtVerificationFail` failure-reason; canonical string `"Jwt verification fails"`).

   **Claim validation step** (per §11.P1 canonical failure-reason strings):
   - `exp` (expiration): reject if past current time + `clock_skew_seconds` (default 60s per proto comment) (`JwtExpired` failure-reason; canonical string `"Jwt is expired"`).
   - `nbf` (not-before): reject if `now + clock_skew_seconds < nbf` (`JwtNotYetValid` failure-reason; canonical string `"Jwt not yet valid"`).
   - `iat` (issued-at): informational; not enforced for rejection.
   - `iss` (issuer): if provider's `issuer` field is non-empty, JWT's `iss` claim MUST match exactly (`JwtUnknownIssuer` failure-reason; canonical string `"Jwt issuer is not configured"`). Empty provider issuer = skip iss check.
   - `aud` (audience; string or array): if provider's `audiences[]` is non-empty, require intersection with the JWT's `aud` (`JwtAudienceNotAllowed` failure-reason; canonical string `"Audiences in Jwt are not allowed"`; per §11.P1 REFINED this status maps to **HTTP 403, not 401** — Envoy filter source's `code == Status::JwtAudienceNotAllowed ? Forbidden : Unauthorized` mapping); empty `audiences[]` means skip.

   **JwtRequirement evaluation step** (per the 6-variant evaluator + §11.P16 RATIFIED-AND-EXTENDED):
   - `provider_name`: validate JWT against named provider; pass iff valid + claim-checks pass.
   - `provider_and_audiences`: validate against named provider with the per-rule audience override (per ProviderWithAudiences.audiences[] OVERRIDES provider's audiences[]).
   - `requires_any`: recursively evaluate each sub-requirement; pass on first match (short-circuit OR).
   - `requires_all`: recursively evaluate each sub-requirement; pass iff ALL match.
   - `allow_missing`: pass iff JWT absent (no extraction matches; status = `JwtMissed`); fail iff JWT present-and-invalid.
   - `allow_missing_or_failed`: always pass (any verification outcome accepted; useful for validate-and-forward-for-downstream patterns).

   **Side-effect step** (per Q6 Full-header-side; on validation success):
   - If `forward == false` (proto default `false`): strip the Authorization header (or original extraction source). Per JwtProvider.forward proto comment: "If false, the JWT is removed in the request after a success verification. If true, the JWT is not removed in the request." Note proto comment caveat: `forward` only works for from_header/from_params; from_cookies extraction does NOT strip.
   - If `forward_payload_header` non-empty: emit `<header_name>: <base64url-encoded payload>` (with or without `=` padding per `pad_forward_payload_header`). §11.P13 RATIFIES: when `pad_forward_payload_header == true`, the base64url encoding preserves `=` padding; when `false` (default), padding is stripped.
   - For each `claim_to_headers[]` entry: extract `claim_name` from JWT payload (dot-notation for nested claims per §11.P10 + proto comment line 1690 — `"claim.nested.key"` traverses `payload[claim][nested][key]`); emit `<header_name>: <claim-value-string>`. Per §11.P10 REFINED + proto comment line 287: **array-claims are NOT supported** — `"The claim must be of type; string, int, double, bool. Array type claims are not supported"`. Non-string scalar claims are coerced via stdlib JSON-marshal of the value.
   - If `clear_route_cache == true`: invoke `cb.ClearRouteCache()` (HCM-side primitive that forces re-route-match after header mutation; per phase-10 ADR-0108 precedent). Per proto comment lines 299-305, clear is also TRIGGERED when at least one `claim_to_headers` header is added OR if `payload_in_metadata` is set; envoy-go MVP only honors the explicit `clear_route_cache: true` setting (the implicit-on-claim-side-effect trigger is DEFERRED per §8 deferral 18 NEW).

   **Wire-shape conformance with reference Envoy** (deliberate; documented at BEHAVIOR_CONTRACT phase-17 forward-pointer notes): allow-decisions pass through with header-mutated request (forward / forward_payload_header / claim_to_headers writes) but BYTE-EQUIVALENT response from upstream backend. Deny-decisions emit `SendLocalReply(<status>, "<failure-reason canonical string>", <www-authenticate header>)` mirroring the RFC 6750 401-challenge wire-shape discipline; §11.P1 + §11.P2 + §11.P3 RATIFIED via filter.cc scrape:
   - Status: 403 for `JwtAudienceNotAllowed`, 401 for ALL other failure-reasons (REFINED from BRAINSTORM "always 401" hypothesis per §1.1 amendment 8).
   - Body: byte-exact failure-reason string from `getStatusString(status)` jwt_verify_lib mapping (~70 canonical strings; ~10 commonly-hit in practice per §11.P1 table). When `strip_failure_response == true`, body becomes empty string.
   - WWW-Authenticate header: `Bearer realm="<original_request_uri>"` always added; if status != `JwtMissed`, additionally appends `, error="invalid_token"` (per §11.P2 RATIFIED-AND-EXTENDED; verbatim quote `absl::StrAppend(&value, InvalidTokenErrorString)` where `InvalidTokenErrorString` = `, error="invalid_token"`). When `strip_failure_response == true`, www-authenticate header NOT set.
   - `response_code_details` (NEW finding per §1.1 amendment 11): format `"jwt_authn_access_denied{<failure_reason_with_spaces_as_underscores>}"`. envoy-go MVP DEFERS the field emission per phase-04 HCM scope (current HCM does not surface response_code_details to local-reply callers; mirrors phase-16 ADR-0146 `response_code_details` divergence-window discipline); documented as divergence-window per §8 deferral 13 NEW.

7. **Stat surface — 64→**71** names (REFINED from BRAINSTORM "~72 names" hypothesis per §1.1 amendment 9 + §11.P6 REFUTED).** **§11.P6 REFUTES BRAINSTORM §1.1 item 7 + §5 hypothesis** that the stat surface contains per-provider counter scaling — empirical scrape of Envoy v1.37.2 `source/extensions/filters/http/jwt_authn/stats.h` reveals the `ALL_JWT_AUTHN_FILTER_STATS` macro defines EXACTLY **7 filter-wide counters, NO per-provider scaling, NO `jwks_fetch_in_progress` gauge** (the BRAINSTORM-hypothesized 5-per-provider JWKS family + 2-per-provider success/fail + 1 bypass = 8 base scaling per-provider is REFUTED; Envoy emits a single filter-wide counter set keyed at the HCM stat_prefix scope). The 7 base counters per active HCM stat_prefix:

   - `allowed` — counter; increments per request where the active engine result = ALLOWED (mirrors phase-16 rbac's `allowed` counter naming convention — Envoy reuses the same counter name across multiple filters).
   - `denied` — counter; increments per request where the engine result = DENIED.
   - `cors_preflight_bypassed` — counter; increments per OPTIONS preflight request that bypassed validation via `bypass_cors_preflight=true` (REFINED from BRAINSTORM-hypothesized `bypassed_cors_preflight`; the canonical Envoy name is `cors_preflight_bypassed` per §1.1 amendment 10).
   - `jwks_fetch_success` — counter; increments per successful JWKS fetch from RemoteJwks endpoint (filter-wide counter; NOT per-provider).
   - `jwks_fetch_failed` — counter; increments per failed JWKS fetch.
   - `jwt_cache_hit` — counter; increments per request that hit the validated-JWT LRU cache (NEW finding per §1.1 amendment 9 — BRAINSTORM hypothesized `jwks_cache_hit/miss` for the JWKS cache; the actual counter family `jwt_cache_hit/miss` tracks the validated-JWT LRU cache, gated on `jwt_cache_config` being set).
   - `jwt_cache_miss` — counter; increments per request that missed the validated-JWT LRU cache.

   §11.P6 + §11.P7 confirm exact stat names + scope + counter-vs-gauge disposition (the 7 stats are ALL counters; NO gauges; NO histograms in jwt_authn — counter-only filter mirroring phase-16 rbac's counter-only discipline). §11.P7 confirms Prometheus tag-extractor + namespace flattening: SN2 reuse (the existing HCM-stat-prefix tag-extractor handles `http.<HCM_stat_prefix>.jwt_authn.*` verbatim without amendment; NO new SN10 rule needed).

   **Stat surface count summary:**
   - Phase 15 (bandwidth_limit): 46 → 60 names (14 new active counter/gauge; SN2 reuse; +2 deferred-histogram via twin-series-filter divergence-window per ADR-0138).
   - Phase 16 (rbac): 60 → 64 base names (4 new active counter; per-policy lazy counter family conditional on `track_per_rule_stats: true`; SN2 reuse; ADR-0145).
   - **Phase 17 (jwt_authn): 64 → 71 names (7 new active counter; NO per-provider scaling; NO gauges; NO histograms; SN2 reuse).**

   Note: phase-17's MVP gates `jwt_cache_hit` and `jwt_cache_miss` behind `jwt_cache_config` proto being set; since MVP defers `jwt_cache_config` per §8 deferral 8, these two counters are STRUCTURALLY UNREACHABLE under MVP configs (the lazy-allocate Registry never registers them). Operators wanting to scrape these counters need `jwt_cache_config` re-activation (future phase). The fixed-table extension is 71 names INCLUSIVE of the 2 cache counters for the operator-facing BEHAVIOR_CONTRACT stat-table; the runtime emission count is 5 of 7 (allowed + denied + cors_preflight_bypassed + jwks_fetch_success + jwks_fetch_failed) under phase-17 MVP.

   **Per-route stats discipline: SHARED hypothesis RATIFIED** (§11.P8 RATIFIES via filter_config.cc scrape — `findPerRouteVerifier()` carries no stats-related logic; no per-route stat namespacing exists in Envoy). Rationale: per-route is pure delegation; spawns no new policy-evaluation state. DIVERGES from phase-11 / phase-15 / phase-16 INDEPENDENT-stats; MIRRORS phase-12 / phase-13 / phase-14 SHARED-stats discipline.

8. **TWO new framework primitives** — the SECOND CONSECUTIVE §9 row to ship two primitives per phase-16 precedent: (i) HTTP-outbound fetcher with async refresh + per-thread cache + scheduled refresh + retry/backoff at a new top-level package `internal/jwks/` (ADR-0150); (ii) JWS/JWT verifier at a new top-level package `internal/jwt/` (ADR-0151). See §3 for details. Both primitives are explicitly designed cross-phase-reusable at introduction time — anchoring the same cross-phase-reuse-at-introduction-time discipline phase 16 established at ADR-0142 + ADR-0144. The framework-delta accretion across §9 family-rows:

   - Phase 07.1 cors: NEW framework (the entire HTTP-filter framework). N/A baseline.
   - Phase 09 fault: introduced `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume primitives.
   - Phase 10 header_mutation: ZERO framework deltas (`ResolveAllTiers` does not count as new — accessor variant).
   - Phase 11 local_ratelimit: ZERO framework deltas.
   - Phase 12 csrf: ZERO framework deltas.
   - Phase 13 buffer: TWO framework deltas (decode-side per ADR-0128).
   - Phase 14 compressor: ONE framework delta (`OverwriteBody` per ADR-0131).
   - Phase 15 bandwidth_limit: ZERO framework deltas (load-bearing reusability demonstration).
   - Phase 16 rbac: TWO framework deltas (matcher-engine per ADR-0142 + TLS-principal accessor per ADR-0144).
   - **Phase 17 jwt_authn: TWO framework deltas (HTTP-outbound JWKS fetcher per ADR-0150 + JWT verifier per ADR-0151).** The SECOND consecutive §9 row to ship two primitives — both genuinely cross-cutting for the auth-filter family (HTTP-outbound is reusable by ext_authz HTTP-mode + oauth2 token-endpoint; JWT verifier is reusable by any future filter consuming JWT semantics).

After phase 17, the project has proven the §9 HTTP filters family-expansion pattern carries through a TENTH filter under: the cors / fault / header_mutation / localratelimit / csrf / buffer / compressor / bandwidthlimit / rbac precedent's package-shape discipline (single-token directory matching the proto type-name modulo underscore-strip); the FIRST §9 row to use the **8th canonical per-route discipline** codified at ADR-0125 §(xiii) in-place amendment; TWO new framework primitives (the SECOND consecutive single phase to introduce two; both cross-phase-reusable); a deliberate divergence-window from reference Envoy on the `response_code_details` field-emission axis (envoy-go: silent; Envoy: emits `jwt_authn_access_denied{<reason>}`) AND on the `filter_state_rules` axis (envoy-go: silent-ignored; Envoy: full filter-state-driven requirement selection) AND on the dynamic-metadata family (envoy-go: silent; Envoy: emits payload_in_metadata + header_in_metadata + failed_status_in_metadata + normalize_payload_in_metadata) AND on the validated-JWT cache axis (envoy-go: no cache; Envoy: 100-entry default LRU when jwt_cache_config set). *envoy-go's HTTP filter framework hosts a decode-side JWT-validating filter that parses 5 proto-faithful top-level fields plus 1 silent-ignored field, walks all 6 JwtRequirement evaluator variants recursively, fetches JWKS from RemoteJwks endpoints with async refresh + per-thread cache + scheduled-refresh-5s-before-TTL + fixed-interval failed-refetch retry, verifies JWT signatures against RS+ES algorithm family, performs canonical claim validation (exp + nbf + iss + aud + clock-skew), applies side-effects on success (strip / forward-payload-header / claim-to-headers / clear-route-cache), and gates the request via SendLocalReply(401-or-403, "<failure-reason canonical string>", {WWW-Authenticate: Bearer realm="<uri>"}) on a verification failure; the OBSERVABLE-OUTCOMES are byte-equivalent to reference Envoy on every axis EXCEPT the response_code_details field-emission axis AND the dynamic-metadata family AND filter_state_rules AND validated-JWT cache hit-rate observability.* This is the TENTH §9 family-row to land; subsequent filters (ext_authz, ext_proc, oauth2, lua, wasm, adaptive_concurrency, admission_control, global_ratelimit) follow the same row-as-its-own-phase pattern per ADR-0106.

### 1.1 Empirical-finding-driven scope revisions (per §11)

The §11 empirical-pin block executed in this SPEC's drafting session (2026-05-12) ratifies, refines, or refutes load-bearing BRAINSTORM hypotheses. **Twelve** amendments below are self-contained corrections; collectively they revise:

- **Field decomposition framing (amendments 1–4):** structural — JwtAuthentication has 6 top-level fields not 5 (filter_state_rules is the 6th, SILENT-IGNORED); JwtProvider has 21 fields not 17 (3 new v1.37.x additions: subjects + require_expiration + max_lifetime, all SILENT-IGNORED in MVP); deprecated `requires` is NOT deprecated in v1.37.2 (BRAINSTORM PARSE-REJECT hypothesis WITHDRAWN; honored proto-faithful).
- **8th canonical refinement (amendments 5 + 6):** structural — `PerRouteConfig.disabled` is `bool` (varint), NOT `Empty` as BRAINSTORM hypothesized; `requirement_name` dangling references are RUNTIME-RESOLVED at request time (Envoy returns 403 + error string), NOT parse-rejected; envoy-go MVP mirrors Envoy's runtime-resolve discipline (PARSE-REJECT divergence withdrawn).
- **Clock-skew + claim coverage refinement (amendments 3 + 7):** structural — `clock_skew_seconds` (default 60s per proto comment) is consumed in MVP claim validation; `subjects` + `require_expiration` + `max_lifetime` claim-coverage extensions are SILENT-IGNORED in MVP (couples to v1.37.x-specific extension family).
- **Stat surface refinement (amendments 9 + 10):** structural — 7 base counters, NO per-provider scaling, NO gauges; cors counter name is `cors_preflight_bypassed` (NOT `bypassed_cors_preflight` as BRAINSTORM hypothesized); the actual counter set is `{allowed, cors_preflight_bypassed, denied, jwks_fetch_success, jwks_fetch_failed, jwt_cache_hit, jwt_cache_miss}` (NOT BRAINSTORM's 5-per-provider-JWKS + 2-per-provider-success/fail + 1-bypass = 8-per-provider scaling hypothesis).
- **Deny-path wire shape refinement (amendments 8 + 11 + 12):** structural — status is 403 (not 401) for `JwtAudienceNotAllowed`; WWW-Authenticate body appends `, error="invalid_token"` for non-`JwtMissed` failure-reasons (NOT just `Bearer realm="<issuer>"`); `response_code_details` emits `jwt_authn_access_denied{<reason>}` per filter.cc scrape (envoy-go MVP DEFERS the field emission per phase-04 HCM scope; mirrors phase-16 rbac §1.1 amendment 11 precedent); 401-body literal is the canonical jwt_verify_lib `getStatusString(status)` mapping (~70 strings; ~10 hit at runtime); the original-uri in `Bearer realm="..."` comes from `original_uri_` filter-state, captured at DecodeHeaders time before any route mutation.

Mirrors the phase-12 csrf 4-amendment + phase-14 compressor 6-amendment + phase-15 bandwidth_limit 10-amendment + phase-16 rbac 12-amendment pattern; phase-17 lands 12 amendments aligned with phase-16. The structural design (TWO new framework primitives, Both-JWKS proto-faithful, Full-6 JwtRequirement, RS+ES algorithm allow-list, All-4 extraction sources, Full-header-side side-effects, 8th canonical per-route + SHARED-stats discipline, 401-WWW-Authenticate deny-path) survives intact despite the magnitude of the refinements — all amendments fit within the §1.1 self-contained-prose-block channel without requiring a BRAINSTORM §12 amendment cycle.

#### 1.1 Amendment 1 — JwtAuthentication outer-proto has 6 fields not 5; `filter_state_rules` SILENT-IGNORED (BRAINSTORM §1.1 item 3 + §17.P9)

BRAINSTORM §1.1 item 3 enumerated "5 fields all consumed proto-faithful" — implying the JwtAuthentication outer envelope had exactly 5 top-level fields. **§11.P9 + scrape of `config.pb.go:1446-1517` empirically REFUTES** the 5-field framing — the proto has SIX top-level fields per `[#next-free-field: 7]`:

- Field 1: `providers map<string, JwtProvider>` (consumed)
- Field 2: `rules repeated RequirementRule` (consumed)
- Field 3: **`filter_state_rules FilterStateRule`** (silent-ignored in phase-17 MVP — BRAINSTORM-missed)
- Field 4: `bypass_cors_preflight bool` (consumed)
- Field 5: `requirement_map map<string, JwtRequirement>` (consumed)
- Field 6: `strip_failure_response bool` (consumed)

The `filter_state_rules` field couples to a runtime-mutable JwtRequirement-selection mechanism where another HTTP filter can write a string to `StreamInfo.FilterState[<name>]` and jwt_authn picks the requirement keyed by that string from `FilterStateRule.requires` map. Phase-17 envoy-go MVP SILENT-IGNORES — the parse accepts the field, the evaluator never consults it. Couples to future filter-state-family phase (related to but distinct from dynamic-metadata family). §8 deferral 12 NEW added.

#### 1.1 Amendment 2 — JwtProvider has 21 fields not 17; 3 v1.37.x claim-coverage additions SILENT-IGNORED (BRAINSTORM §1.1 item 3 + §17.P9)

BRAINSTORM §1.1 item 3 enumerated "13 of 17 JwtProvider fields consumed" — implying a 17-field surface. **§11.P9 + scrape of `config.pb.go:60-307` empirically REFUTES** — the proto has 21 fields per `[#next-free-field: 22]`. The 4 BRAINSTORM-missed fields are claim-coverage extensions added in v1.37.x:

- Field 19: `subjects StringMatcher` — restricts JWT `sub` claim per provider; SILENT-IGNORED in phase-17 MVP (§8 deferral 15 NEW; couples to JWT-SVID-class restriction-extensions; v1.37.x-specific).
- Field 20: `require_expiration bool` — requires JWT to carry `exp` claim; SILENT-IGNORED (§8 deferral 16 NEW; v1.37.x-specific).
- Field 21: `max_lifetime Duration` — rejects JWTs with `exp - now > max_lifetime`; SILENT-IGNORED (§8 deferral 17 NEW; v1.37.x-specific).

Plus `clock_skew_seconds` (field 10) which BRAINSTORM treated as silent-ignored coupling-to-claim-validation but per §1.1 amendment 7 is HONORED in MVP (consumed as the canonical 60-second default for `exp` + `nbf` claim validation tolerance). Net MVP consumed count: **13 of 21** (issuer, audiences, remote_jwks, local_jwks, forward, from_headers, from_params, from_cookies, forward_payload_header, pad_forward_payload_header, claim_to_headers, clear_route_cache, clock_skew_seconds). Net silent-ignored count: **8** (payload_in_metadata, normalize_payload_in_metadata, header_in_metadata, failed_status_in_metadata, jwt_cache_config, subjects, require_expiration, max_lifetime). The 13/8 split matches phase-16 rbac's 11+11/3+3 Large-MVP framing pattern.

#### 1.1 Amendment 3 — Subject + require_expiration + max_lifetime claim-coverage SILENT-IGNORED (BRAINSTORM §2.4 + §17.P9)

BRAINSTORM §2.4 framed "Full 6 JwtRequirement proto-faithful" and §1.1 item 3 noted claim validation as `exp + nbf + iat + iss + aud`. Per Amendment 2 above, the v1.37.x additions `subjects` + `require_expiration` + `max_lifetime` extend the per-provider claim-coverage surface in ways orthogonal to the JwtRequirement evaluator. Phase-17 MVP SILENT-IGNORES all three:

- `subjects` (StringMatcher on `sub` claim): MVP claim validation skips `sub` matching entirely. Operators relying on subject-based authorization see envoy-go-vs-Envoy DIVERGENCE — envoy-go admits JWTs with any sub; Envoy rejects sub-mismatch.
- `require_expiration`: MVP claim validation does NOT enforce mandatory-exp. JWTs without `exp` are admitted under envoy-go (subject to other claim checks); Envoy with `require_expiration: true` rejects unexpired-claimed JWTs.
- `max_lifetime`: MVP claim validation does NOT enforce max-lifetime ceilings. JWTs with `exp - now > max_lifetime` are admitted; Envoy rejects.

Couples to a future JWT-SVID-class extension phase that re-enables the three fields together (they form a coherent claim-coverage sub-surface).

#### 1.1 Amendment 4 — `RequirementRule.requires` is NOT deprecated in v1.37.2; PARSE-REJECT discipline WITHDRAWN (BRAINSTORM §1.1 item 3 + §8 deferral 9 + §17.P12)

BRAINSTORM §1.1 item 3 + §8 deferral 9 framed the `RequirementRule.requires` (inline JwtRequirement) field as DEPRECATED in v1.37.2 with envoy-go-strict PARSE-REJECT discipline. **§11.P12 + scrape of `config.pb.go:1294-1325` + filter_config.cc empirically REFUTES** — the v1.37.2 proto carries NO `[deprecated = true]` annotation on either oneof arm (`Requires` at line 1312 or `RequirementName` at line 1317). Envoy filter_config.cc treats both arms equivalently at the dispatch table construction:

```cpp
switch(rule.requirement_type_case()) {
  case kRequires:
    rule_pairs_.emplace_back(Matcher::create(rule, ...), Verifier::create(rule.requires_(), ...));
    break;
  case kRequirementName:
    auto it = name_verifiers_.find(rule.requirement_name());
    if (it == name_verifiers_.end()) throw EnvoyException("Wrong requirement_name ...");
    rule_pairs_.emplace_back(Matcher::create(rule, ...), it->second.get());
    break;
}
```

envoy-go MVP HONORS both arms proto-faithful (mirrors Envoy's permissive disposition). §8 deferral 9 originally framed as "deprecated requires PARSE-REJECT" is WITHDRAWN. The §1.1 amendment 4 refutes BRAINSTORM §1.1 item 3 + §2.8 + §8 deferral 9 PARSE-REJECT framing.

#### 1.1 Amendment 5 — `PerRouteConfig.disabled` is `bool` not `Empty` (BRAINSTORM §2.7 + §4 + §17.P9)

BRAINSTORM §2.7 + §4 framed the 8th canonical per-route as `oneof{requirement_name(string) | disabled(Empty)}` — implying the disable-arm carries a marker message (`google.protobuf.Empty`). **§11.P9 + scrape of `config.pb.go:1595-1679` empirically REFUTES** — the actual `PerRouteConfig.RequirementSpecifier` oneof has TWO arms:

```go
type PerRouteConfig_Disabled struct {
    Disabled bool `protobuf:"varint,1,opt,name=disabled,proto3,oneof"`
}
type PerRouteConfig_RequirementName struct {
    RequirementName string `protobuf:"bytes,2,opt,name=requirement_name,json=requirementName,proto3,oneof"`
}
```

`disabled` is a `bool` (varint at field 1), NOT `Empty`. The structural ramifications:
- The marker-message-Empty form (BRAINSTORM hypothesis) would be unambiguous: presence of `disabled: {}` means "disabled".
- The actual `bool` form admits THREE wire states: `disabled: true` (clearly disabled), `disabled: false` (oneof set; explicit not-disabled), and oneof unset entirely (PGV `required = true` per `config.pb.validate.go:2472-2481` REJECTS this — `PerRouteConfig.RequirementSpecifier` MUST be set).

PGV constraint (REFINED per §11.P9): `PerRouteConfig.requirement_specifier` is REQUIRED at the oneof level; `PerRouteConfig.requirement_name` (when chosen) has PGV `min_len=1` per `config.pb.validate.go:2460-2462`. The `disabled` arm has no value-level PGV (any bool value valid).

Phase-17 envoy-go disposition:
- `PerRouteConfig{disabled: true}` → wholly disabled on route per 8th canonical's disable semantic.
- `PerRouteConfig{disabled: false}` → oneof set but disable-bool false; treated as "passthrough to listener-level rules" (no per-route override; matches Envoy's filter_config.cc behaviour — the oneof IS set so the per-route resolution returns this entry, but the disable bool is false so the resolver falls through). Documented at ADR-0153 §Decision.
- `PerRouteConfig{requirement_name: "<name>"}` → reference-by-name into listener-level requirement_map; resolved at request time per §1.1 amendment 6.

ADR-0153 codifies. ADR-0125 §(xiii) amendment paragraph reflects the `bool` (not `Empty`) form.

#### 1.1 Amendment 6 — Per-route `requirement_name` dangling reference is RUNTIME-RESOLVED, not parse-rejected (BRAINSTORM §2.7 + §8 deferral + §17.P12)

BRAINSTORM §2.7 + §17.P12 hypothesized that envoy-go-strict would PARSE-REJECT per-route `requirement_name` entries whose name does not exist in the listener-level requirement_map. **§11.P12 + scrape of Envoy filter_config.cc REFUTES** the parse-reject framing — Envoy's `findPerRouteVerifier()` returns an error string at request time:

```cpp
const auto& it = name_verifiers_.find(per_route.config().requirement_name());
if (it != name_verifiers_.end()) {
  return std::make_pair(it->second.get(), EMPTY_STRING);
}
return std::make_pair(nullptr, absl::StrCat("Wrong requirement_name: ...", name));
```

When the name is absent, the filter returns 403 + body `"Failed JWT authentication: Wrong requirement_name: <name>"` (NOT 401; per filter.cc per-route error path uses `Http::Code::Forbidden`). The mistake at parse time is structurally IMPOSSIBLE to detect without listener-level requirement_map context, AND deferred-config-loading patterns (RDS-served route configs evaluated lazily against listener-level provider configs) require runtime-resolve semantics.

Phase-17 envoy-go disposition: MIRROR Envoy's runtime-resolve. `parsePerRoute` accepts `requirement_name: "<any-string>"` without consulting the listener-level requirement_map; `resolvePerRouteConfig` performs the map lookup at request time; on miss, the filter emits `SendLocalReply(403, "Failed JWT authentication: Wrong requirement_name: <name>", {})` (mirrors Envoy's wire shape; NO WWW-Authenticate header for the per-route error case per filter.cc). The original BRAINSTORM PARSE-REJECT hypothesis is WITHDRAWN. ADR-0153 codifies.

#### 1.1 Amendment 7 — `clock_skew_seconds` HONORED in MVP claim validation (BRAINSTORM §1.1 item 3 silent + §17.P9)

BRAINSTORM §1.1 item 3 did NOT enumerate `clock_skew_seconds` (field 10 per `config.pb.go:281`) in the 13-consumed list. Per the JwtProvider proto comment line 281: `"Specify the clock skew in seconds when verifying JWT time constraint, such as 'exp', and 'nbf'. If not specified, default is 60 seconds."` Envoy's authenticator applies this tolerance to both `exp` (rejected when `now > exp + clock_skew_seconds`) and `nbf` (rejected when `now + clock_skew_seconds < nbf`). The default-60-second clock-skew tolerance is operationally important — JWT issuers + Envoy proxies routinely have ~5-30 second clock drift; rejecting tokens that are ~30 seconds expired-or-pending punishes legitimate clients on clock-drift alone.

Phase-17 envoy-go disposition: HONOR `clock_skew_seconds` in MVP. compiledProvider carries the tolerance value (default 60 if unset; per-provider per proto field 10). `exp` + `nbf` claim validation applies the tolerance. Net consumed list grows to 13 fields (per §1.1 amendment 2). ADR-0149 codifies.

#### 1.1 Amendment 8 — Deny-path status is 401 for most failures + 403 for `JwtAudienceNotAllowed` (BRAINSTORM §4 + §17.P1)

BRAINSTORM §4 framed the deny-path as "always 401 + WWW-Authenticate Bearer challenge". **§11.P1 + scrape of Envoy filter.cc REFINES** — the actual status mapping per the filter source:

```cpp
auto code = (status == Status::JwtAudienceNotAllowed) ? Http::Code::Forbidden : Http::Code::Unauthorized;
```

Status 401 for `JwtMissed`, `JwtVerificationFail`, `JwtUnknownIssuer`, `JwtExpired`, `JwtNotYetValid`, `JwtBadFormat`, and the ~60 other failure-reasons.
Status 403 for `JwtAudienceNotAllowed` ONLY (RFC 6750 semantic: 401 = unauthenticated/missing-credentials; 403 = authenticated-but-not-authorized; audience-mismatch is the canonical "authenticated but not for this audience" case).

Phase-17 envoy-go disposition: mirror the status mapping verbatim. ADR-0155 codifies the 401-or-403 dispatch table; the WWW-Authenticate Bearer challenge fires on ALL deny paths (both 401 and 403) per §11.P2 RATIFIED — Envoy's filter.cc adds the WWW-Authenticate header inside the `if (!stripFailureResponse)` block regardless of which status fired. The body literal is `getStatusString(status)` (canonical jwt_verify_lib mapping; see §11.P1 for the ~70-string table).

#### 1.1 Amendment 9 — Stat surface is 7 base counters, NO per-provider scaling, NO gauges (BRAINSTORM §2.9 + §1.1 item 7 + §17.P6)

BRAINSTORM §2.9 + §1.1 item 7 hypothesized "8 new base counters with per-provider scaling: 2 per-provider counters (jwt_authn_success/failed) scaling with provider count + 5 per-provider JWKS counters + 1 filter-wide bypass counter". **§11.P6 + scrape of Envoy v1.37.2 `source/extensions/filters/http/jwt_authn/stats.h` REFUTES** via the `ALL_JWT_AUTHN_FILTER_STATS` macro:

```
COUNTER(allowed)
COUNTER(cors_preflight_bypassed)
COUNTER(denied)
COUNTER(jwks_fetch_success)
COUNTER(jwks_fetch_failed)
COUNTER(jwt_cache_hit)
COUNTER(jwt_cache_miss)
```

**7 base counters, NO per-provider scaling, NO `jwks_fetch_in_progress` gauge.** All 7 are FILTER-WIDE counters keyed at the HCM stat_prefix scope (mirrors phase-16 rbac's filter-wide `allowed`/`denied` counters; multiple providers + multiple RemoteJwks endpoints all contribute to the same 7-counter set). The BRAINSTORM-hypothesized per-provider `jwt_authn.<provider>.jwt_authn_success` / `jwt_authn.<provider>.jwt_authn_failed` template is REFUTED — Envoy emits a single filter-wide `allowed` + `denied` pair, NOT per-provider counters. Operators wanting per-provider visibility need access-log scraping (which Envoy emits with provider-name annotation, not stats-scrape).

Phase-17 envoy-go disposition: filterStats struct carries EXACTLY 7 counters (allowed + denied + cors_preflight_bypassed + jwks_fetch_success + jwks_fetch_failed + jwt_cache_hit + jwt_cache_miss); the latter 2 (`jwt_cache_*`) are gated on `jwt_cache_config` proto being set — under phase-17 MVP they are STRUCTURALLY UNREACHABLE (per §8 deferral 8 the jwt_cache_config is silent-ignored). Stat-table 64 → 71 names (7 new active counters; NO gauges; NO histograms). ADR-0154 codifies.

#### 1.1 Amendment 10 — CORS bypass counter name is `cors_preflight_bypassed`, not `bypassed_cors_preflight` (BRAINSTORM §1.1 item 7 + §17.P6)

BRAINSTORM §1.1 item 7 hypothesized the bypass counter name as `bypassed_cors_preflight`. **§11.P6 + the stats.h macro REFINES** — the actual canonical name per `COUNTER(cors_preflight_bypassed)` is `cors_preflight_bypassed` (noun-noun-verb-past-participle ordering, NOT verb-past-participle-noun-noun). Phase-17 envoy-go disposition: use the canonical Envoy name `cors_preflight_bypassed` in the filterStats struct + BEHAVIOR_CONTRACT stat-table. ADR-0154 + BEHAVIOR_CONTRACT §13.2 update.

#### 1.1 Amendment 11 — `response_code_details` emit format discovered; envoy-go MVP DEFERS (BRAINSTORM § silent + §17.P1)

BRAINSTORM §4 did not address `response_code_details` field emission on the deny path. **§11.P1 + scrape of Envoy filter.cc REVEALS** — Envoy emits `response_code_details = "jwt_authn_access_denied{<failure_reason_with_spaces_as_underscores>}"` per the filter's `generateRcDetails()` helper:

```cpp
constexpr absl::string_view kRcDetailJwtAuthnPrefix = "jwt_authn_access_denied";
std::string generateRcDetails(absl::string_view error_msg) {
  return absl::StrCat(kRcDetailJwtAuthnPrefix, "{",
      StringUtil::replaceAllEmptySpace(error_msg), "}");
}
```

E.g., `"Jwt is missing"` becomes `"jwt_authn_access_denied{Jwt_is_missing}"`. This lands in Envoy's HCM `response_flag_details` accessor + access-log `RESPONSE_CODE_DETAILS` operator.

Phase-17 envoy-go disposition: MIRROR phase-16 rbac §1.1 amendment 11 + ADR-0146 precedent — envoy-go MVP DEFERS the field emission (current phase-04 HCM does not surface response_code_details to local-reply callers; threading the value through HCM to access-log output is a phase-04 framework primitive concern). Divergence-window documented at §8 deferral 13 NEW + BEHAVIOR_CONTRACT phase-17 forward-pointer notes. Operator dashboards inspecting access-log `RESPONSE_CODE_DETAILS` see Envoy-side emit but envoy-go absent on jwt_authn denials. Future re-activation: same response-code-details framework phase phase-16 rbac §8.12 forward-pointed to. ADR-0155 documents the divergence-window.

#### 1.1 Amendment 12 — WWW-Authenticate body appends `, error="invalid_token"` for non-JwtMissed; realm uses request URI (BRAINSTORM §4 + §17.P2)

BRAINSTORM §4 hypothesized the WWW-Authenticate header as `Bearer realm="<issuer>"`. **§11.P2 + scrape of Envoy filter.cc REFINES** — the actual header value generation per the filter source:

```cpp
std::string value = absl::StrCat("Bearer realm=\"", uri, "\"");
if (status != Status::JwtMissed) {
  absl::StrAppend(&value, InvalidTokenErrorString);
}
headers.setCopy(Http::Headers::get().WWWAuthenticate, value);
```

Two structural differences from the BRAINSTORM hypothesis:
- **(a)** The `realm` field uses the **original request URI** captured at DecodeHeaders time (`original_uri_` filter-state), NOT the JWT provider's `issuer` field. This is operationally significant — the realm value reflects the WHAT-WAS-ACCESSED (`/api/v1/foo`), not the WHO-SHOULD-HAVE-AUTHENTICATED (`https://issuer.example.com`).
- **(b)** For all failure-reasons EXCEPT `JwtMissed`, the header value additionally appends `, error="invalid_token"` (per RFC 6750 §3 challenge syntax). `JwtMissed` case (no JWT extracted) omits the error parameter (no credential was rejected; only absent). The constant `InvalidTokenErrorString` = `, error="invalid_token"` (with leading comma + space).

Phase-17 envoy-go disposition: capture the original request URI at DecodeHeaders entry; thread into the SendLocalReply callsite. WWW-Authenticate header construction follows the verbatim Envoy logic — `Bearer realm="<original_uri>"` + conditional `, error="invalid_token"` append. ADR-0155 codifies. NOTE: this amendment carries an operationally subtle implication — if the JWT validation fires after a header-mutation filter has rewritten the `:path` pseudo-header, the realm reflects the MUTATED path, not the original. Operators with `:path` rewriters upstream of jwt_authn see WWW-Authenticate realms that may diverge from their dashboard expectations.

---

## 2. Non-purposes

Phase 17 is a single-filter slice. It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land `envoy.filters.http.jwt_authn` (Both-JWKS Full-6-Requirement RS+ES All-4-extraction Full-header-side 8th-canonical MVP) under the existing 07.1 framework + the TWO new framework primitives anchored at ADR-0150 + ADR-0151 (which ARE part of phase 17's deliverable).

### 2.1 `JwtAuthentication` outer-proto non-goals (per BRAINSTORM §8 + §1.1 amendment 1)

The outer filter proto `envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication` consumes 5 of 6 fields proto-faithful (per §1.1 amendment 1). The silent-ignore set at the outer level:

#### 2.1.1 Out of scope: `JwtAuthentication.filter_state_rules` (silent-ignored at parse + runtime)

Per §1.1 amendment 1 — runtime requirement-selection based on `StreamInfo.FilterState[<name>]` string accessor. envoy-go MVP silent-ignores; couples to a future filter-state-family phase (kindred to dynamic-metadata family). §8 deferral 12.

### 2.2 Per-route override surface non-goals (per §1.1 amendments 5 + 6 + §11.P9 + §11.P12)

The per-route TPFC entry is the `PerRouteConfig` wrapper proto with a single REQUIRED oneof `RequirementSpecifier` (per §1.1 amendment 5; PGV `required = true` at the oneof level). Two cases per §5:

- **NOT honored:** there is NO per-route override on `providers` map or `bypass_cors_preflight` or `strip_failure_response` or `requirement_map` or any subset of the listener-level fields — the only knobs are `disabled` (bool) and `requirement_name` (string-reference into listener-level map).
- **To DISABLE jwt_authn on a specific route:** operators set per-route TPFC entry `PerRouteConfig{disabled: true}`. The route bypasses JWT entirely.
- **To OVERRIDE which requirement applies on a specific route:** operators set `PerRouteConfig{requirement_name: "<name>"}` referencing a name in listener-level `requirement_map`. The named requirement replaces the listener-level rules-dispatch result for this route.
- **NOT supported:** per-route override of a `JwtRequirement` inline (the `requires` shape from `RequirementRule` does NOT exist at the per-route level — Envoy's proto designers intentionally enforced the name-reference indirection to keep per-route TPFC entries minimal).

### 2.3 Algorithm scope-out (HS family + EdDSA + `none` deferred; per BRAINSTORM §2.3 + §8 deferrals 5-7)

Algorithm DEFERRED set (3 algorithm families):
- **HS family (HS256/HS384/HS512)** — DEFERRED per §8 deferral 5: requires symmetric-secret config plumbing in JwtProvider (security-sensitive — operators must securely provision shared secrets). Future algorithm-extension phase enables.
- **EdDSA algorithm (Ed25519 / Ed448)** — DEFERRED per §8 deferral 6: less-common; requires Go stdlib `crypto/ed25519`. Could enable as a standalone follow-on.
- **`none` algorithm** — DEFERRED-PERMANENTLY per §8 deferral 7 (intentionally never enabled; security-sensitive — `alg=none` JWTs are unsigned; allowing them defeats authentication). PARSE-REJECT at JWK parse + runtime-reject at token parse; no operator-config knob to enable.

Future codec-extension phase MAY re-activate HS / EdDSA by amending ADR-0151.

### 2.4 JwtProvider field scope-out (4 dynamic-metadata-coupled + 3 v1.37.x claim-coverage; per §1.1 amendments 2 + 3 + §8 deferrals 1-4 + 15-17)

JwtProvider DEFERRED set (8 fields):
- **`payload_in_metadata`** (field 9; DEFERRED per §8 deferral 1) — encodes JWT payload as dynamic metadata. Couples to dynamic-metadata family (same family blocked at phase-16 forward-pointer item 9).
- **`header_in_metadata`** (field 14; DEFERRED per §8 deferral 2) — encodes JWT header as dynamic metadata. Couples to dynamic-metadata family.
- **`failed_status_in_metadata`** (field 16; DEFERRED per §8 deferral 3) — encodes failure status as dynamic metadata. Couples to dynamic-metadata family.
- **`normalize_payload_in_metadata`** (field 18; DEFERRED per §8 deferral 4) — sub-message controlling payload normalization shape for metadata emission. Couples to dynamic-metadata family + payload_in_metadata coupling chain.
- **`jwt_cache_config`** (field 12; DEFERRED per §8 deferral 8) — validated-JWT result LRU cache; default 100-entry size per proto. MVP no-cache (each request re-validates); foot-gun for high-RPS. Couples to a future caching-framework phase.
- **`subjects`** (field 19; DEFERRED per §8 deferral 15 + §1.1 amendment 3) — StringMatcher on JWT `sub` claim; v1.37.x JWT-SVID-class extension. Couples to claim-coverage-extension phase.
- **`require_expiration`** (field 20; DEFERRED per §8 deferral 16 + §1.1 amendment 3) — mandates JWT `exp` presence. v1.37.x extension.
- **`max_lifetime`** (field 21; DEFERRED per §8 deferral 17 + §1.1 amendment 3) — ceiling on JWT remaining lifetime. v1.37.x extension.

### 2.5 `jwt_cache_config` validated-JWT LRU cache (MVP no-cache; per §8 deferral 8)

Validated-JWT result LRU cache. Cache-hit speedup for high-RPS deployments (each repeated identical JWT skips signature verification + claim validation). envoy-go MVP no-cache (each request re-validates; correctness invariant: cached vs uncached produce identical authorization outcomes). Foot-gun: high-RPS deployments with identical JWTs see ~10-100x performance gap vs Envoy. Documented at BEHAVIOR_CONTRACT phase-17 forward-pointer notes. Future caching-framework phase introduces a generic LRU primitive (jwt_authn + ext_authz response cache + oauth2 token cache co-design).

### 2.6 Deprecated `RequirementRule.requires` field — NOT actually deprecated; HONORED proto-faithful (per §1.1 amendment 4)

Per §1.1 amendment 4: the BRAINSTORM-hypothesized PARSE-REJECT discipline for the inline `requires` oneof arm is WITHDRAWN. v1.37.2 proto carries no deprecation annotation; envoy-go MVP honors both `RequirementRule.requires` (inline JwtRequirement) AND `RequirementRule.requirement_name` (map reference) arms. No divergence-window.

### 2.7 JWKS retry/backoff customization beyond canonical default (per BRAINSTORM §8 deferral 10)

Operator-config knob to override the canonical 1-second `failed_refetch_duration` default (§11.P4 RATIFIES the 1s constant `DefaultRefetchAfterFailedSec`). DEFERRED: MVP picks the canonical default; honoring `JwksAsyncFetch.failed_refetch_duration` proto field is in scope at MVP (the field IS consumed when set; only the further customization of base-interval-vs-cap-vs-jitter is deferred). Per §11.P4 REFUTED hypothesis: Envoy does NOT implement exponential backoff (BRAINSTORM hypothesized 1s/2s/4s/8s/16s/30s-cap). Fixed-interval retry. Future operator-ergonomics phase MAY add an exponential backoff customization surface; §8 deferral 10 reframed.

### 2.8 JWKS cache-invalidation hooks (admin-API integration; per BRAINSTORM §8 deferral 11)

Cache-bust on operator signal (e.g., admin-API endpoint `POST /jwt_authn/<provider>/refresh_jwks`). DEFERRED per §8 deferral 11: couples to admin-API extension. Operationally useful for IdP key-rotation flows where waiting for natural cache expiry (10 minutes default per §11.P5) is too slow. Future phase introduces.

### 2.9 Access-log integration for jwt_authn fields (per BRAINSTORM §8 deferral 12 / now §8 deferral 14)

Per-request log line emits `%JWT_PROVIDER%` / `%JWT_SUBJECT%` / `%JWT_FAILURE_REASON%` access-log formatters. DEFERRED per §8 deferral 14: couples to phase-16 forward-pointer access-log item 7 + access-log formatter extension framework.

### 2.10 CEL-based dynamic provider selection (per BRAINSTORM §8 deferral 13 / now §8 deferral 11)

Runtime CEL expression evaluating against request attributes to pick provider OR requirement at evaluation time. DEFERRED per §8 deferral 11 (renumbered): couples to phase-16 CEL deferral item 10 + future CEL framework phase landing `internal/cel/` + `cel-go` dependency.

### 2.11 No filter-chain ordering surgery (per BRAINSTORM §3.4)

Phase 17 jwt_authn's filter-chain position is up to the operator. Recommended ordering: jwt_authn AFTER cors + rbac, BEFORE header_mutation/buffer/compressor (so jwt_authn can apply claim-to-headers writes that downstream filters consume). Fixture 0019 pins jwt_authn between rbac and header_mutation. Operators wanting custom ordering (e.g., jwt_authn before rbac so RBAC can match on the validated JWT principal) have full flexibility per the operator's filter-chain order. SPEC documents the trade-off without prescribing.

### 2.12 JwtRequirement Set recursion-depth (per BRAINSTORM §8 deferral 14 / now foot-gun)

No envoy-go-only depth-cap on `requires_any` / `requires_all` recursion at parse-time; mirrors Envoy's permissive disposition (no proto-level PGV; no documented hard cap). The recursive `buildOneRequirement` calls form a natural Go-stack-depth bound (~10K-frame default before SIGSEGV). Operators writing deeply-nested requires configs may hit Go-stack-depth issues at config-load; documented foot-gun at BEHAVIOR_CONTRACT phase-17 forward-pointer notes. Future operator-ergonomics phase MAY add an envoy-go-only depth-cap (e.g., max 32 levels of nesting); deferred from MVP.

---

## 3. Framework survey result (TWO new framework deltas)

Phase 17 introduces **TWO framework deltas** — SECOND CONSECUTIVE §9 row to ship two simultaneously (after phase 16). The two primitives:

| Primitive | Location | Source |
|---|---|---|
| HTTP-outbound JWKS fetcher + per-thread cache + scheduled refresh + retry | NEW package `internal/jwks/` (~300-500 LoC) | ADR-0150 (NEW) |
| JWS/JWT parser + signature verifier + claim validator | NEW package `internal/jwt/` (~250-400 LoC) | ADR-0151 (NEW) |

Reused (no framework changes):

| Primitive | Source | Phase-17 usage |
|---|---|---|
| `cb.SendLocalReply(status, body, headers)` | Phase-09 fault per `internal/filter/http/fault/fault.go:319,335` | DENY-path 401/403 emission with WWW-Authenticate header |
| 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) | Phase 07.1 per `internal/filter/http/registry.go` | Per-route TPFC most-specific override |
| `internal/filter/http/extension/HTTPRegistry` per ADR-0072 | Phase 07.1 | Boot-registration discipline |
| `stats.Registry.NewCounter` post-Freeze | Phase-06.1 per `internal/stats/registry.go` | 7 base counters allocated at New() time (NO lazy-allocation; NO `NewCounterIfAbsent` needed for filter-wide counters) |
| Existing RouteMatch evaluator | Phase 04 HCM router | `RequirementRule.match` evaluation (listener-level rules dispatch) |
| `cb.ClearRouteCache()` HCM-side primitive | Phase 10 header_mutation per ADR-0108 | Side-effect on validation success when `clear_route_cache: true` |
| Go stdlib `net/http.Client` | stdlib | Outbound HTTP for JWKS fetch (wrapped by `internal/jwks` primitive) |
| Go stdlib `crypto/rsa.VerifyPKCS1v15` + `crypto/ecdsa.Verify` | stdlib | Signature verification (wrapped by `internal/jwt` primitive) |
| Go stdlib `encoding/base64` (URL-encoding variant) | stdlib | JWT base64url-decode for header + payload + signature |
| Go stdlib `time.AfterFunc` + `time.Ticker` | stdlib | JWKS refresh scheduling (wrapped by `internal/jwks` primitive) |

### 3.1 HTTP-outbound JWKS fetcher primitive at NEW `internal/jwks/` package (ADR-0150)

A new top-level Go package `internal/jwks/` (location LOCKED at SPEC time per §11.P5 + §11.P6 framework-survey: the JWKS-specific scope is narrow enough to warrant a dedicated package, NOT a generic `internal/httpclient/`; future outbound-HTTP-needing filters compose against this OR introduce a sibling package). The package exports:

```go
package jwks

import (
    "context"
    "crypto"
    "errors"
    "fmt"
    "io"
    "net/http"
    "sync"
    "sync/atomic"
    "time"
)

// Fetcher manages a JWKS endpoint with async background refresh + thread-safe cache.
// One Fetcher per RemoteJwks configured in a JwtProvider.
type Fetcher struct {
    uri              string
    cacheDuration    time.Duration  // default 10 minutes per §11.P5 (DefaultCacheExpirationSec)
    refetchAfterFail time.Duration  // default 1 second per §11.P4 (DefaultRefetchAfterFailedSec)
    refreshLead      time.Duration  // fixed 5 seconds per §11.P5 (RefetchBeforeExpiredSec)
    fastListener     bool           // when true, listener activation does not block on initial fetch
    retryPolicy      *RetryPolicy   // optional inner-HTTP-request retry policy (envoy.config.core.v3.RetryPolicy)
    client           *http.Client
    // ... opaque state (mu, current keys, refresh timer, ...)
}

// New constructs a Fetcher for a given JWKS URI with cache duration + async-fetch policy.
// If asyncFetch is non-nil, spawns a background goroutine for periodic refresh.
// Initial fetch is blocking if fastListener=false; non-blocking if fastListener=true.
func New(uri string, cacheDuration time.Duration, asyncFetch *AsyncFetch, retryPolicy *RetryPolicy) (*Fetcher, error)

// AsyncFetch mirrors envoy.extensions.filters.http.jwt_authn.v3.JwksAsyncFetch.
type AsyncFetch struct {
    FastListener           bool
    FailedRefetchDuration  time.Duration  // default 1s if zero
}

// RetryPolicy mirrors a subset of envoy.config.core.v3.RetryPolicy for inner-HTTP-request retries.
type RetryPolicy struct {
    NumRetries  int
    BaseInterval time.Duration  // default 1s when num_retries set + base unset per §11.P4 RetryPolicy proto comment
    MaxInterval  time.Duration  // default 10*base when unset
}

// Get returns the current cached JWK Set, fetching on-demand if cache empty or stale.
// Blocks on first call when cache is empty; subsequent calls return cached value while
// background refresh runs. Returns ErrJwksNotReady if fastListener=true and the initial
// fetch has not yet completed (callers handle as "Jwks not available" error).
func (f *Fetcher) Get(ctx context.Context) (*JWKSet, error)

// Close stops the background refresh goroutine + releases resources.
func (f *Fetcher) Close() error

// JWKSet wraps a parsed JWK Set (RFC 7517 §5).
type JWKSet struct {
    // ... opaque
}

// Lookup resolves a key by kid (key-id) + alg (algorithm); falls back to first key
// with matching alg if kid not present. Mirrors Envoy's pickKeyAlgWithKid logic.
// Returns ErrJwksKidAlgMismatch if no match.
func (s *JWKSet) Lookup(kid, alg string) (crypto.PublicKey, error)

// Errors mirror jwt_verify_lib status codes for the JWKS-fetch + key-lookup sub-domain.
var (
    ErrJwksFetchFail        = errors.New("Jwks remote fetch is failed")
    ErrJwksParseError       = errors.New("Jwks is an invalid JSON")
    ErrJwksKidAlgMismatch   = errors.New("Jwks doesn't have key to match kid or alg")
    ErrJwksNotReady         = errors.New("Jwks fetch not yet completed")
    ErrJwksNoValidKeys      = errors.New("Jwks doesn't have any valid public key")
    // ... more per the JWKS subset of §11.P1 canonical strings
)
```

**Refresh schedule** (per §11.P4 + §11.P5 RATIFIED):
- Successful fetch → schedule next refresh at `cacheDuration - 5s` (default 10min - 5s = 9min55s) via `time.AfterFunc`.
- Failed fetch → schedule next retry at `failedRefetchDuration` (default 1 second) via `time.AfterFunc`.
- `fastListener: true` → initial fetch is non-blocking (listener activates immediately; first few requests may return `ErrJwksNotReady` until fetch completes).
- `fastListener: false` (DEFAULT) → initial fetch is blocking at New() time; New() returns only after the initial fetch succeeds-or-fails (failures still return New() error per §11.P4 + Envoy `init_target_->ready()` semantic).

**Cross-phase reuse intent** (codified at ADR-0150 §Decision): future filters consuming outbound-HTTP-from-filter primitives MAY reuse the same `Fetcher` pattern as a structural template:
- Future `ext_authz` HTTP-mode (sends auth check to external HTTP service): reuses the `http.Client` + retry-policy structure; differs on the request-shape (POST with body, not GET) and the response-handling (auth-decision propagation, not JSON-decode-and-cache).
- Future `oauth2` filter (fetches access tokens from token endpoint): reuses the cache-and-refresh discipline; differs on the token-refresh-on-401-from-upstream pattern.

The package lives OUTSIDE `internal/filter/` (mirroring phase-16's `internal/matcher/`) explicitly to anchor cross-phase reusability. The BEHAVIOR_CONTRACT §13.7 NEW top-level `## JWKS framework primitive` umbrella anchors operator-facing semantics; future filters extend the umbrella additively.

### 3.2 JWS/JWT verifier primitive at NEW `internal/jwt/` package (ADR-0151)

A new top-level Go package `internal/jwt/`. The package exports:

```go
package jwt

import (
    "crypto"
    "errors"
    "time"
)

// Token represents a parsed JWT (header + payload + signature).
type Token struct {
    RawHeader     string  // base64url-encoded header (the first segment before the first '.')
    RawPayload    string  // base64url-encoded payload (the second segment)
    RawSignature  string  // base64url-encoded signature (the third segment)
    Header        map[string]interface{}  // decoded header (alg, kid, typ, ...)
    Payload       map[string]interface{}  // decoded payload (iss, sub, aud, exp, nbf, iat, ...)
    Alg           string  // header.alg
    Kid           string  // header.kid (may be "")
}

// Parse parses the 3-part JWT structure (header.payload.signature). Validates JSON
// decoding of header + payload; rejects malformed tokens. Does NOT verify signature
// or claims at parse time — that is the caller's two-step responsibility.
// Status return mirrors jwt_verify_lib's canonical status codes per §11.P1.
func Parse(raw string) (*Token, error)

// VerifySignature verifies the JWT signature against the given public key + algorithm.
// Algorithm allow-list per ADR-0151: RS256/384/512 + ES256/384/512.
// PARSE-REJECT (returns ErrJwtHeaderNotImplementedAlg) for algorithms outside the allow-list.
func (t *Token) VerifySignature(key crypto.PublicKey, alg string) error

// ValidateOptions controls claim validation.
type ValidateOptions struct {
    Issuer            string         // if non-empty, JWT.iss must match exactly
    Audiences         []string       // if non-empty, intersection with JWT.aud required (OR-semantic)
    ClockSkew         time.Duration  // tolerance for exp + nbf checks (default 60s per JwtProvider.clock_skew_seconds)
    Now               time.Time      // injection point for clock; defaults to time.Now()
    RequireExpiration bool           // SILENT-IGNORED in phase-17 MVP per §1.1 amendment 3
    MaxLifetime       time.Duration  // SILENT-IGNORED in phase-17 MVP per §1.1 amendment 3
    Subjects          *StringMatcher // SILENT-IGNORED in phase-17 MVP per §1.1 amendment 3
}

// ValidateClaims checks exp, nbf, iss, aud per the options.
// Returns the appropriate canonical error (ErrJwtExpired, ErrJwtNotYetValid, ErrJwtUnknownIssuer,
// ErrJwtAudienceNotAllowed) on rejection.
func (t *Token) ValidateClaims(opts ValidateOptions) error

// PayloadClaim extracts a claim by dot-notation path (e.g., "groups", "sub", "user.email").
// Returns the typed value (string, float64, bool, nil); array-valued claims return ErrArrayClaim
// per §11.P10 + JwtProvider.claim_to_headers proto comment "Array type claims are not supported".
func (t *Token) PayloadClaim(path string) (interface{}, error)

// Errors mirror jwt_verify_lib status codes for the JWT-parse + claim-validation sub-domain.
var (
    ErrJwtMissed                       = errors.New("Jwt is missing")
    ErrJwtBadFormat                    = errors.New("Jwt is not in the form of Header.Payload.Signature with two dots and 3 sections")
    ErrJwtHeaderBadAlg                 = errors.New("Jwt header [alg] field is required and must be a string")
    ErrJwtHeaderNotImplementedAlg      = errors.New("Jwt header [alg] is not supported")
    ErrJwtHeaderParseErrorBadBase64    = errors.New("Jwt header is an invalid Base64url encoded")
    ErrJwtHeaderParseErrorBadJson      = errors.New("Jwt header is an invalid JSON")
    ErrJwtPayloadParseErrorBadBase64   = errors.New("Jwt payload is an invalid Base64url encoded")
    ErrJwtPayloadParseErrorBadJson     = errors.New("Jwt payload is an invalid JSON")
    ErrJwtSignatureParseErrorBadBase64 = errors.New("Jwt signature is an invalid Base64url encoded")
    ErrJwtUnknownIssuer                = errors.New("Jwt issuer is not configured")
    ErrJwtAudienceNotAllowed           = errors.New("Audiences in Jwt are not allowed")
    ErrJwtVerificationFail             = errors.New("Jwt verification fails")
    ErrJwtExpired                      = errors.New("Jwt is expired")
    ErrJwtNotYetValid                  = errors.New("Jwt not yet valid")
    ErrArrayClaim                      = errors.New("claim is array-valued; not supported")
    // ... more per §11.P1 canonical strings table
)
```

**Cross-phase reuse intent** (codified at ADR-0151 §Decision): future filters consuming JWT semantics (e.g., a hypothetical `jwt_claim_router` filter routing on claim values, or oauth2's token validation step) reuse `jwt.Parse` + `Token.VerifySignature` + `Token.ValidateClaims` + `Token.PayloadClaim` directly. The package is algorithm-agnostic for parsing + signature verification (the algorithm allow-list is checked at signature verification time, not parse time — `Parse` accepts any header.alg string; `VerifySignature` enforces the allow-list); claim validation is policy-driven via `ValidateOptions`.

The package lives OUTSIDE `internal/filter/` (mirroring phase-16's `internal/matcher/`) explicitly to anchor cross-phase reusability. The BEHAVIOR_CONTRACT §13.8 NEW top-level `## JWT verifier framework primitive` umbrella anchors operator-facing semantics.

### 3.3 What else is reused (already-on-disk primitives)

(See table at §3 top.) No further amendments to existing primitives.

### 3.4 No filter-chain ordering surgery

Per BRAINSTORM §3.4 + §2.11 above. Phase 17 fixture pins jwt_authn between rbac and header_mutation in the chain; operators have full flexibility.

---

## 4. Rejection-path wire shape (deny disposition)

Phase 17's deny-path is RFC 6750-conformant Bearer-token challenge. The wire-shape composition (per §1.1 amendments 8 + 11 + 12 + §11.P1 + §11.P2 + §11.P3 RATIFIED):

- **Status code:** 401 (Unauthorized) for ALL failure-reasons EXCEPT `JwtAudienceNotAllowed`, which gets 403 (Forbidden). Per §1.1 amendment 8 + filter.cc verbatim `code = (status == Status::JwtAudienceNotAllowed) ? Http::Code::Forbidden : Http::Code::Unauthorized`. The 401-vs-403 dispatch is structurally semantic — 401 means "no valid authentication provided, please authenticate"; 403 means "you ARE authenticated but not authorized for this audience".
- **Body:** byte-exact failure-reason string from `getStatusString(status)` jwt_verify_lib mapping (per §11.P1 RATIFIES). The ~70 canonical strings include common ones like `"Jwt is missing"` (14 bytes), `"Jwt verification fails"` (22 bytes), `"Jwt issuer is not configured"` (28 bytes), `"Jwt is expired"` (14 bytes), `"Audiences in Jwt are not allowed"` (32 bytes), `"Jwt not yet valid"` (17 bytes), `"Jwt header [alg] is not supported"` (33 bytes), `"Jwt is not in the form of Header.Payload.Signature..."` (variable; full string truncated in source quote). When `strip_failure_response: true`, body becomes empty string (0 bytes; per §11.P3 RATIFIED — filter.cc uses `""` literal in the strip-branch SendLocalReply call).
- **WWW-Authenticate header** (lowercase wire-form `www-authenticate`; added on ALL deny paths regardless of 401-vs-403):
  - Always: `Bearer realm="<original_request_uri>"` (per §1.1 amendment 12 — realm uses request URI captured at DecodeHeaders, NOT JWT issuer).
  - For non-`JwtMissed` statuses: additionally appends `, error="invalid_token"` (per §11.P2 RATIFIED-AND-EXTENDED via `InvalidTokenErrorString` constant).
  - When `strip_failure_response: true`: header NOT set (the `headers.setCopy` call sits inside the non-strip branch of filter.cc).
- **4-header standard set (lowercase wire-form):** `content-length: <body length>` (0 when stripped), `content-type: text/plain` (when body non-empty; may be absent when stripped — empirical at §11.P3 PARTIAL), `date: <RFC1123>`, `server: envoy` (mirrors phase-09/11/12/13/16 4-header discipline). Plus the WWW-Authenticate when not stripped.
- **Connection disposition:** keep-alive (NO `connection: close`). Unlike phase-13 buffer 413 which closes the connection, jwt_authn's deny is a pre-body decision (the body has not started yet at validation time at DecodeHeaders).
- **Response-code-details (NEW finding per §1.1 amendment 11):** Envoy emits `jwt_authn_access_denied{<failure_reason_with_spaces_as_underscores>}`. envoy-go MVP DEFERS the field emission per phase-04 HCM scope; divergence-window documented at §8 deferral 13.
- **Per-route runtime-resolve error case** (per §1.1 amendment 6 + §11.P12): when per-route `requirement_name` is unresolved against listener-level requirement_map, filter emits `SendLocalReply(403, "Failed JWT authentication: Wrong requirement_name: <name>", {})` — status 403 (NOT 401; per filter.cc `Http::Code::Forbidden`); NO WWW-Authenticate header; body wraps the error string. envoy-go MVP MIRRORS this verbatim.

`cb.SendLocalReply(status, body, headers)` mechanism (mirrors fault.abort, local_ratelimit 429, csrf 403, buffer 413, rbac 403). The WWW-Authenticate header addition uses a header-callback closure form mirroring Envoy's filter.cc:

```go
// Pseudocode for the deny callsite:
status := mapStatusToHTTPCode(reason)  // 401 or 403 per §1.1 amendment 8
body := getStatusString(reason)        // canonical string per §11.P1
hdrs := []envoyhttp.Header{}
if !cc.stripFailureResponse {
    wwwAuth := fmt.Sprintf(`Bearer realm=%q`, f.originalURI)
    if reason != jwt.ErrJwtMissed {
        wwwAuth += `, error="invalid_token"`
    }
    hdrs = append(hdrs, envoyhttp.Header{Name: "www-authenticate", Value: wwwAuth})
    hdrs = append(hdrs, envoyhttp.Header{Name: "content-type", Value: "text/plain"})
} else {
    body = ""
}
f.dcb.SendLocalReply(status, body, hdrs)
```

ADR-0155 codifies the wire-shape claim.

---

## 5. Per-route discipline — NEW 8th canonical (oneof{disabled(bool) | requirement_name(string)}) + SHARED-stats

Per §1.1 amendments 5 + 6 + §11.P9 + §11.P12: the per-route TPFC entry is the `PerRouteConfig` wrapper proto with a REQUIRED oneof `RequirementSpecifier` carrying two arms — `disabled` (bool; varint at field 1) and `requirement_name` (string; bytes at field 2; PGV `min_len=1`). The 8th canonical pattern (per ADR-0125 §(xiii) amendment).

### 5.1 8th canonical: oneof{disabled(bool) | requirement_name(string)} with string-reference-delegation

Phase 17 is the FIRST row to use the **string-reference-delegation** per-route discipline. Structurally distinct from all 7 prior canonicals (§5.4 amendment §(xiii) enumerates the distinction matrix verbatim). The 8th canonical's defining feature: the per-route does NOT carry its own filter config; it carries either a disable-bool OR a string-reference into a listener-level registry.

Three cases at `parsePerRoute`:
- **(a) `PerRouteConfig{disabled: true}`:** produce `*compiledPerRoute{disabled: true, requirementName: ""}`. The route bypasses jwt_authn entirely.
- **(b) `PerRouteConfig{disabled: false}` (the oneof is set to disabled-arm with false value):** produce `*compiledPerRoute{disabled: false, requirementName: ""}`. The filter falls through to listener-level rules-dispatch as if no per-route override existed (mirrors Envoy filter_config.cc behaviour — the oneof set arm IS the disabled arm but it explicitly does not disable; envoy-go treats this as "passthrough to listener-level rules").
- **(c) `PerRouteConfig{requirement_name: "<name>"}`:** produce `*compiledPerRoute{disabled: false, requirementName: "<name>"}`. The compiled per-route carries the name; the LISTENER-level `*compiledConfig.requirementMap[<name>]` is consulted at request time.

The `disabled` boolean inside `*compiledPerRoute` is set to TRUE only for case (a). Cases (b) + (c) carry `disabled: false`. Documented at ADR-0153.

### 5.2 SHARED-stats discipline (per ADR-0154; mirrors phase-12 csrf + phase-13 buffer + phase-14 compressor)

Per §11.P8 RATIFIED + filter_config.cc empirical scrape: per-route override emits to the SAME stat namespace as the listener-level config. No per-route `stat_prefix` field exists at jwt_authn (NO per-route override of HCM stat_prefix). Per-route emits `allowed` / `denied` / `cors_preflight_bypassed` / `jwks_fetch_success` / `jwks_fetch_failed` etc. to the same `http.<HCM_stat_prefix>.jwt_authn.<counter>` namespace as listener-level requests.

Rationale: per-route is pure delegation (string-reference resolution against listener-level requirement_map). It does NOT spawn new policy-evaluation state — it merely selects WHICH listener-level requirement applies to this route. The SHARED-stats discipline is operationally correct (operators see aggregated allow/deny counts per HCM regardless of which route fired).

DIVERGES from phase-11 / phase-15 / phase-16 INDEPENDENT-stats (those filters' per-route overrides own NEW stateful config — token bucket / throttle config / policy set). MIRRORS phase-12 csrf ADR-0124 + phase-13 buffer ADR-0125 + phase-14 compressor ADR-0132 SHARED-stats discipline. The decisive factor: jwt_authn's per-route DOES NOT spawn new state.

### 5.3 Resolution flow at request time

`PerRouteConfig.Resolve(ctx)` → most-specific `*compiledPerRoute` for this route.
1. If `compiledPerRoute.disabled == true` → set `f.passthrough = true`; `DecodeHeaders` short-circuits to `HeaderContinue` without engine evaluation; NO counter increments (passthrough does NOT increment `allowed`).
2. If `compiledPerRoute.disabled == false` AND `compiledPerRoute.requirementName != ""` → consult `f.state.listenerRC.requirementMap[requirementName]`. If found, evaluate that requirement against the request's extracted JWT; emit `allowed` or `denied` per the result. If NOT found, emit `SendLocalReply(403, "Failed JWT authentication: Wrong requirement_name: <name>", {})` + `denied` counter increment.
3. If `compiledPerRoute.disabled == false` AND `compiledPerRoute.requirementName == ""` (case (b) above) → fall through to listener-level `rules` dispatch (evaluate `rules[i].match` against the request; first-match wins; apply the matched rule's `requires` OR `requirement_name`-resolved JwtRequirement; default no-match disposition is "no JWT verification required, pass through").
4. If `PerRouteConfig.Resolve(ctx)` returns nil (no per-route TPFC entry at any tier) → identical to case (b): fall through to listener-level `rules` dispatch.

ADR-0153 codifies the resolution flow.

### 5.4 ADR-0125 in-place amendment paragraph §(xiii) authored at this SPEC commit

ADR-0125's canonical-pattern roster grows from 7 to 8 via in-place amendment paragraph §(xiii) (mirrors phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) + phase-16 ADR-0125 §(xii) in-place-update precedent). The amendment paragraph is authored verbatim at this SPEC commit and lands in DECISIONS.md after the existing §(xii) paragraph. See DECISIONS.md ADR-0125 §(xiii) for the full text.

---

## 6. compiledConfig + code shapes

### 6.1 Public surface

`internal/filter/http/jwtauthn/jwtauthn.go` exports:
- `TypeURL` const = `"type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication"`.
- `New` (the `HTTPFilterFactory` registered at boot per ADR-0072).
- `filterName` package-private const = `"envoy.filters.http.jwt_authn"`.

### 6.2 `compiledConfig` + `compiledProvider` + `compiledRequirement` + `filterStats` shape

```go
// compiledConfig is the parsed + validated runtime config for one jwt_authn filter
// instance (listener-level OR per-route).
type compiledConfig struct {
    // Providers parsed from JwtAuthentication.providers map.
    providers map[string]*compiledProvider

    // Rules list parsed from JwtAuthentication.rules (listener-level dispatch).
    rules []*compiledRule

    // Requirement map parsed from JwtAuthentication.requirement_map (referenced by
    // both listener-level rules AND per-route TPFC).
    requirementMap map[string]*compiledRequirement

    // bypass_cors_preflight + strip_failure_response flags.
    bypassCorsPreflight  bool
    stripFailureResponse bool

    // Stats — nil when ctx.Stats is nil (test path).
    stats *filterStats
}

// compiledProvider carries the parsed + validated JwtProvider.
type compiledProvider struct {
    issuer               string             // "" means skip iss check
    audiences            []string           // empty means skip aud check
    jwksFetcher          *jwks.Fetcher      // for RemoteJwks (non-nil exclusive with localJwks)
    localJwks            *jwt.JWKSet        // for LocalJwks (non-nil exclusive with jwksFetcher)
    forward              bool               // false (default) → strip JWT from request on success
    fromHeaders          []headerLoc        // parsed JwtHeader entries
    fromParams           []string           // case-sensitive query param names
    fromCookies          []string           // case-sensitive cookie names
    forwardPayloadHeader string             // header name to emit base64url(payload); "" → skip
    padForwardPayloadHdr bool               // preserve = padding when true
    claimToHeaders       []claimToHeader    // claim_name → header_name mapping
    clearRouteCache      bool               // invoke cb.ClearRouteCache() on success
    clockSkew            time.Duration      // default 60s per §1.1 amendment 7
    // Silent-ignored fields not stored: payload_in_metadata, header_in_metadata,
    // failed_status_in_metadata, normalize_payload_in_metadata, jwt_cache_config,
    // subjects, require_expiration, max_lifetime.
}

type headerLoc struct {
    name        string  // header name (case-insensitive HTTP header lookup)
    valuePrefix string  // e.g., "Bearer " — substring-searched in value
}

type claimToHeader struct {
    headerName string  // destination request header
    claimName  string  // JWT payload claim path (dot-notation for nested)
}

// compiledRule carries one parsed RequirementRule from JwtAuthentication.rules.
type compiledRule struct {
    matcher     *router.RouteMatcher   // existing phase-04 RouteMatch evaluator
    requirement *compiledRequirement   // resolved either from inline `requires` or from requirement_map[name]
}

// compiledRequirement is the parsed + recursively-built JwtRequirement evaluator tree.
type compiledRequirement struct {
    kind     requirementKind   // providerName / providerAndAudiences / requiresAny / requiresAll / allowMissing / allowMissingOrFailed
    provider *compiledProvider // for providerName + providerAndAudiences (nil otherwise)
    audOverr []string          // for providerAndAudiences (per-rule audience override)
    children []*compiledRequirement // for requiresAny + requiresAll (recursive)
}

type requirementKind int
const (
    reqProviderName requirementKind = iota
    reqProviderAndAudiences
    reqRequiresAny
    reqRequiresAll
    reqAllowMissing
    reqAllowMissingOrFailed
)

// filterStats is the 7-counter base set (per §1.1 amendment 9 + §11.P6).
type filterStats struct {
    allowed              *stats.Counter
    denied               *stats.Counter
    corsPreflightBypassed *stats.Counter
    jwksFetchSuccess     *stats.Counter
    jwksFetchFailed      *stats.Counter
    jwtCacheHit          *stats.Counter  // structurally unreachable in phase-17 MVP per §8 deferral 8
    jwtCacheMiss         *stats.Counter  // structurally unreachable in phase-17 MVP per §8 deferral 8
}
```

### 6.3 `factoryState` + `filter` shape

```go
// factoryState is the closure-captured shared state per factory invocation.
// Mirrors phase-11 ADR-0117 + phase-15 ADR-0139 + phase-16 ADR-0145 IMPL-1 pattern,
// SIMPLIFIED for SHARED-stats discipline (no per-route sync.Map needed for stats).
type factoryState struct {
    listenerRC *compiledConfig
    perRoute   sync.Map // map[*jwt_authnv3.PerRouteConfig]*compiledPerRoute — keyed by per-route TPFC proto pointer
}

// compiledPerRoute wraps the per-route disposition per §5.
type compiledPerRoute struct {
    disabled        bool   // true when PerRouteConfig{disabled: true} per §5.1 (a)
    requirementName string // non-empty when PerRouteConfig{requirement_name: "<name>"} per §5.1 (c); "" for cases (a) + (b)
}

// filter is the per-stream filter instance allocated by the factory closure.
// Decoder-only; no encode-side state.
type filter struct {
    state *factoryState
    dcb   envoyhttp.DecoderFilterCallbacks

    // Per-stream state (cached at DecodeHeaders).
    activeRC    *compiledConfig    // always listener-level config (per-route is delegation-only; no override config)
    perRoute    *compiledPerRoute  // resolved at DecodeHeaders; nil if no per-route entry
    passthrough bool               // true when per-route disabled=true
    originalURI string             // captured at DecodeHeaders for WWW-Authenticate realm per §1.1 amendment 12
}
```

### 6.4 `New` factory

```go
func New(tc *anypb.Any, ctx envoyhttp.FactoryCtx) (envoyhttp.FilterInstanceFactory, error) {
    if tc == nil {
        return nil, errors.New("jwt_authn: typed_config required")
    }
    var c jwt_authnv3.JwtAuthentication
    if err := tc.UnmarshalTo(&c); err != nil {
        return nil, fmt.Errorf("jwt_authn: unmarshal: %w", err)
    }
    rc, err := buildCompiledConfig(&c, ctx)
    if err != nil {
        return nil, err
    }
    state := &factoryState{listenerRC: rc}
    return func() envoyhttp.HTTPFilter {
        f := &filter{state: state}
        return envoyhttp.HTTPFilter{
            Name:     filterName,
            Decoder:  f,
            Encoder:  nil, // decoder-only per §1 item 5
            PerRoute: parsePerRoute,
        }
    }, nil
}
```

### 6.5 `buildCompiledConfig` — parse + validate JwtAuthentication

```go
func buildCompiledConfig(c *jwt_authnv3.JwtAuthentication, ctx envoyhttp.FactoryCtx) (*compiledConfig, error) {
    cc := &compiledConfig{
        bypassCorsPreflight:  c.GetBypassCorsPreflight(),
        stripFailureResponse: c.GetStripFailureResponse(),
    }

    // Parse providers map.
    providers := make(map[string]*compiledProvider, len(c.GetProviders()))
    for name, p := range c.GetProviders() {
        cp, err := buildCompiledProvider(name, p)
        if err != nil {
            return nil, fmt.Errorf("jwt_authn: provider %q: %w", name, err)
        }
        providers[name] = cp
    }
    cc.providers = providers

    // Parse requirement_map.
    reqMap := make(map[string]*compiledRequirement, len(c.GetRequirementMap()))
    for name, r := range c.GetRequirementMap() {
        cr, err := buildCompiledRequirement(r, providers)
        if err != nil {
            return nil, fmt.Errorf("jwt_authn: requirement %q: %w", name, err)
        }
        reqMap[name] = cr
    }
    cc.requirementMap = reqMap

    // Parse rules list (listener-level RequirementRule entries).
    rules := make([]*compiledRule, 0, len(c.GetRules()))
    for i, r := range c.GetRules() {
        rule, err := buildCompiledRule(r, providers, reqMap)
        if err != nil {
            return nil, fmt.Errorf("jwt_authn: rule[%d]: %w", i, err)
        }
        rules = append(rules, rule)
    }
    cc.rules = rules

    // filter_state_rules SILENT-IGNORED per §1.1 amendment 1 + §8 deferral 12.

    // Stats — register 7 counters at HCM stat_prefix scope.
    if ctx.Stats != nil {
        cc.stats = newFilterStats(ctx.Stats)
    }

    return cc, nil
}

// buildCompiledProvider parses one JwtProvider with envoy-go-side defensive PGV-mirror.
func buildCompiledProvider(name string, p *jwt_authnv3.JwtProvider) (*compiledProvider, error) {
    cp := &compiledProvider{
        issuer:               p.GetIssuer(),
        audiences:            p.GetAudiences(),
        forward:              p.GetForward(),
        forwardPayloadHeader: p.GetForwardPayloadHeader(),
        padForwardPayloadHdr: p.GetPadForwardPayloadHeader(),
        clearRouteCache:      p.GetClearRouteCache(),
        clockSkew:            durationOrDefault(p.GetClockSkewSeconds(), 60*time.Second),
    }

    // Parse from_headers, from_params, from_cookies.
    for _, h := range p.GetFromHeaders() {
        cp.fromHeaders = append(cp.fromHeaders, headerLoc{
            name:        h.GetName(),
            valuePrefix: h.GetValuePrefix(),
        })
    }
    cp.fromParams = p.GetFromParams()
    cp.fromCookies = p.GetFromCookies()

    // Parse claim_to_headers.
    for _, ch := range p.GetClaimToHeaders() {
        cp.claimToHeaders = append(cp.claimToHeaders, claimToHeader{
            headerName: ch.GetHeaderName(),
            claimName:  ch.GetClaimName(),
        })
    }

    // Parse JWKS source (oneof).
    switch src := p.GetJwksSourceSpecifier().(type) {
    case *jwt_authnv3.JwtProvider_RemoteJwks:
        rj := src.RemoteJwks
        if rj.GetHttpUri() == nil {
            return nil, errors.New("remote_jwks.http_uri is required")
        }
        cd := rj.GetCacheDuration()
        cacheDur := 10 * time.Minute // default per §11.P5
        if cd != nil {
            cacheDur = cd.AsDuration()
        }
        af := &jwks.AsyncFetch{}
        if rj.GetAsyncFetch() != nil {
            af.FastListener = rj.GetAsyncFetch().GetFastListener()
            if fr := rj.GetAsyncFetch().GetFailedRefetchDuration(); fr != nil {
                af.FailedRefetchDuration = fr.AsDuration()
            }
        }
        fetcher, err := jwks.New(rj.GetHttpUri().GetUri(), cacheDur, af, parseRetryPolicy(rj.GetRetryPolicy()))
        if err != nil {
            return nil, fmt.Errorf("remote_jwks: %w", err)
        }
        cp.jwksFetcher = fetcher
    case *jwt_authnv3.JwtProvider_LocalJwks:
        ds := src.LocalJwks
        raw, err := readDataSource(ds)
        if err != nil {
            return nil, fmt.Errorf("local_jwks: %w", err)
        }
        keyset, err := jwt.ParseJWKSet(raw)
        if err != nil {
            return nil, fmt.Errorf("local_jwks: %w", err)
        }
        cp.localJwks = keyset
    default:
        // Neither set: provider has no JWKS source — PARSE-REJECT.
        return nil, errors.New("either remote_jwks or local_jwks must be set")
    }

    // Silent-ignored fields not parsed: payload_in_metadata, header_in_metadata,
    // failed_status_in_metadata, normalize_payload_in_metadata, jwt_cache_config,
    // subjects, require_expiration, max_lifetime.

    return cp, nil
}

// buildCompiledRequirement recursively parses a JwtRequirement (6-variant evaluator).
func buildCompiledRequirement(r *jwt_authnv3.JwtRequirement, providers map[string]*compiledProvider) (*compiledRequirement, error) {
    if r == nil || r.GetRequiresType() == nil {
        // Empty requirement = "no JWT required" per proto comment.
        return &compiledRequirement{kind: reqAllowMissingOrFailed}, nil
    }
    switch t := r.GetRequiresType().(type) {
    case *jwt_authnv3.JwtRequirement_ProviderName:
        p, ok := providers[t.ProviderName]
        if !ok {
            return nil, fmt.Errorf("provider_name %q not in providers map", t.ProviderName)
        }
        return &compiledRequirement{kind: reqProviderName, provider: p}, nil
    case *jwt_authnv3.JwtRequirement_ProviderAndAudiences:
        p, ok := providers[t.ProviderAndAudiences.GetProviderName()]
        if !ok {
            return nil, fmt.Errorf("provider_and_audiences.provider_name %q not in providers map", t.ProviderAndAudiences.GetProviderName())
        }
        return &compiledRequirement{
            kind:     reqProviderAndAudiences,
            provider: p,
            audOverr: t.ProviderAndAudiences.GetAudiences(),
        }, nil
    case *jwt_authnv3.JwtRequirement_RequiresAny:
        cr := &compiledRequirement{kind: reqRequiresAny}
        for i, sub := range t.RequiresAny.GetRequirements() {
            sc, err := buildCompiledRequirement(sub, providers)
            if err != nil {
                return nil, fmt.Errorf("requires_any[%d]: %w", i, err)
            }
            cr.children = append(cr.children, sc)
        }
        return cr, nil
    case *jwt_authnv3.JwtRequirement_RequiresAll:
        cr := &compiledRequirement{kind: reqRequiresAll}
        for i, sub := range t.RequiresAll.GetRequirements() {
            sc, err := buildCompiledRequirement(sub, providers)
            if err != nil {
                return nil, fmt.Errorf("requires_all[%d]: %w", i, err)
            }
            cr.children = append(cr.children, sc)
        }
        return cr, nil
    case *jwt_authnv3.JwtRequirement_AllowMissing:
        return &compiledRequirement{kind: reqAllowMissing}, nil
    case *jwt_authnv3.JwtRequirement_AllowMissingOrFailed:
        return &compiledRequirement{kind: reqAllowMissingOrFailed}, nil
    default:
        return nil, fmt.Errorf("unknown requires_type %T", t)
    }
}

// buildCompiledRule parses one RequirementRule (match + requirement).
func buildCompiledRule(r *jwt_authnv3.RequirementRule, providers map[string]*compiledProvider, reqMap map[string]*compiledRequirement) (*compiledRule, error) {
    if r.GetMatch() == nil {
        return nil, errors.New("match is required")
    }
    matcher, err := router.NewRouteMatcher(r.GetMatch())
    if err != nil {
        return nil, fmt.Errorf("match: %w", err)
    }
    var req *compiledRequirement
    switch t := r.GetRequirementType().(type) {
    case *jwt_authnv3.RequirementRule_Requires:
        req, err = buildCompiledRequirement(t.Requires, providers)
        if err != nil {
            return nil, fmt.Errorf("requires: %w", err)
        }
    case *jwt_authnv3.RequirementRule_RequirementName:
        r2, ok := reqMap[t.RequirementName]
        if !ok {
            return nil, fmt.Errorf("requirement_name %q not in requirement_map", t.RequirementName)
        }
        req = r2
    case nil:
        // No requirement set — proto comment: "If not specified, Jwt verification is disabled."
        req = &compiledRequirement{kind: reqAllowMissingOrFailed}
    default:
        return nil, fmt.Errorf("unknown requirement_type %T", t)
    }
    return &compiledRule{matcher: matcher, requirement: req}, nil
}
```

### 6.6 `DecodeHeaders` body — top-level dispatch

```go
func (f *filter) DecodeHeaders(headers http.Header, endStream bool) envoyhttp.FilterHeadersStatus {
    // Capture original URI for WWW-Authenticate realm per §1.1 amendment 12.
    f.originalURI = headers.Get(":path")

    // Resolve per-route TPFC.
    var perRouteMsg proto.Message
    if f.dcb != nil {
        perRouteMsg = f.dcb.RequestRouteConfig()
    }
    f.perRoute = f.state.resolvePerRouteConfig(perRouteMsg)
    f.activeRC = f.state.listenerRC

    // CASE (a): per-route disabled → passthrough.
    if f.perRoute != nil && f.perRoute.disabled {
        f.passthrough = true
        return envoyhttp.HeaderContinue
    }

    // CORS preflight bypass per §11.P1 + filter.cc verbatim.
    if f.activeRC.bypassCorsPreflight && isCorsPreflightRequest(headers) {
        f.activeRC.stats.corsPreflightBypassed.Inc()
        return envoyhttp.HeaderContinue
    }

    // Resolve the requirement to apply.
    req, perRouteErr := f.resolveRequirement(headers)
    if perRouteErr != "" {
        // Per-route runtime-resolve error case per §1.1 amendment 6.
        f.activeRC.stats.denied.Inc()
        f.dcb.SendLocalReply(403, "Failed JWT authentication: " + perRouteErr, nil)
        return envoyhttp.HeaderStopIteration
    }
    if req == nil {
        // No requirement applies → pass through (no JWT verification needed).
        return envoyhttp.HeaderContinue
    }

    // Evaluate the requirement.
    result := f.evaluateRequirement(req, headers)
    return f.applyResult(result, headers, req)
}

func isCorsPreflightRequest(headers http.Header) bool {
    // Per filter.cc isCorsPreflightRequest predicate:
    return headers.Get(":method") == "OPTIONS" &&
        headers.Get("origin") != "" &&
        headers.Get("access-control-request-method") != ""
}

// resolveRequirement determines which compiledRequirement applies to this request.
// Returns (requirement, "") for normal cases; ("", errMsg) for per-route name-resolution failure.
func (f *filter) resolveRequirement(headers http.Header) (*compiledRequirement, string) {
    // Per-route requirement_name case (c).
    if f.perRoute != nil && f.perRoute.requirementName != "" {
        req, ok := f.activeRC.requirementMap[f.perRoute.requirementName]
        if !ok {
            return nil, fmt.Sprintf("Wrong requirement_name: %s", f.perRoute.requirementName)
        }
        return req, ""
    }

    // Listener-level rules dispatch (case (b) or no per-route).
    for _, rule := range f.activeRC.rules {
        if rule.matcher.Match(headers) {
            return rule.requirement, ""
        }
    }

    // No rule matched → no JWT verification required.
    return nil, ""
}
```

### 6.7 Extraction-source iteration (all 4 sources per §11.P14 + §11.P15)

```go
// extractTokens iterates a provider's configured extraction sources + the defaults
// (Authorization Bearer + access_token query param) per extractor.cc + §11.P14/P15.
// Returns the list of (rawToken, sourceKind) tuples; empty when no extraction matched.
func extractTokens(provider *compiledProvider, headers http.Header) []extractedToken {
    var out []extractedToken

    // 1. Configured from_headers (in declared order).
    for _, hl := range provider.fromHeaders {
        if v := headers.Get(hl.name); v != "" {
            if hl.valuePrefix != "" {
                if i := strings.Index(v, hl.valuePrefix); i >= 0 {
                    out = append(out, extractedToken{raw: stripNonBase64URLChars(v[i+len(hl.valuePrefix):]), src: sourceHeader, name: hl.name})
                }
            } else {
                out = append(out, extractedToken{raw: v, src: sourceHeader, name: hl.name})
            }
        }
    }

    // 2. Default Authorization Bearer (only when no from_headers configured OR Envoy applies defaults additively).
    //    Per §11.P14 + extractor.cc: defaults apply when provider.fromHeaders == nil AND provider.fromParams == nil.
    if len(provider.fromHeaders) == 0 && len(provider.fromParams) == 0 && len(provider.fromCookies) == 0 {
        if v := headers.Get("authorization"); strings.HasPrefix(v, "Bearer ") {
            out = append(out, extractedToken{raw: v[len("Bearer "):], src: sourceHeader, name: "authorization"})
        }
        // Default access_token query param.
        path := headers.Get(":path")
        if vals, ok := parseQueryParam(path, "access_token"); ok && len(vals) > 0 {
            out = append(out, extractedToken{raw: vals[0], src: sourceParam, name: "access_token"})
        }
    }

    // 3. Configured from_params (case-sensitive; URL-decoded; first-value-only per §11.P14).
    for _, paramName := range provider.fromParams {
        path := headers.Get(":path")
        if vals, ok := parseQueryParam(path, paramName); ok && len(vals) > 0 {
            out = append(out, extractedToken{raw: vals[0], src: sourceParam, name: paramName})
        }
    }

    // 4. Configured from_cookies (case-sensitive; verbatim value per §11.P15).
    cookies := parseCookies(headers.Get("cookie"))
    for _, cookieName := range provider.fromCookies {
        if v, ok := cookies[cookieName]; ok {
            out = append(out, extractedToken{raw: v, src: sourceCookie, name: cookieName})
        }
    }

    return out
}

type extractedToken struct {
    raw  string
    src  sourceKind
    name string  // header/param/cookie name (used for forward=false stripping)
}

type sourceKind int
const (
    sourceHeader sourceKind = iota
    sourceParam
    sourceCookie
)
```

### 6.8 JwtRequirement evaluator (6-variant per §11.P16)

```go
type evalResult struct {
    allowed bool
    err     error          // canonical failure-reason error per §11.P1
    token   *jwt.Token     // non-nil on successful validation (used for side-effects)
    provider *compiledProvider // non-nil on successful validation
}

func (f *filter) evaluateRequirement(req *compiledRequirement, headers http.Header) evalResult {
    switch req.kind {
    case reqProviderName:
        return f.evaluateProvider(req.provider, headers, req.provider.audiences)
    case reqProviderAndAudiences:
        return f.evaluateProvider(req.provider, headers, req.audOverr)
    case reqRequiresAny:
        // OR-semantic: short-circuit on first match. Per §11.P16 + filter.cc semantic:
        // requires_any returns the FIRST successful evaluation's status; if all fail,
        // returns the LAST failure status (per Envoy's verifier.cc).
        var lastErr error
        for _, sub := range req.children {
            r := f.evaluateRequirement(sub, headers)
            if r.allowed {
                return r
            }
            lastErr = r.err
        }
        return evalResult{allowed: false, err: lastErr}
    case reqRequiresAll:
        // AND-semantic: short-circuit on first failure.
        for _, sub := range req.children {
            r := f.evaluateRequirement(sub, headers)
            if !r.allowed {
                return r
            }
        }
        return evalResult{allowed: true}
    case reqAllowMissing:
        // Iterate extraction sources across all providers (per Envoy's allow_missing semantic).
        // If any token extracted, it must validate against SOME provider; else missing-OK.
        // Phase-17 MVP simplification: requires_any-style across providers to mirror Envoy.
        // (TODO: ADR-0149 §Decision may refine this iteration per Task-N empirical scrape.)
        for _, p := range f.activeRC.providers {
            if len(extractTokens(p, headers)) > 0 {
                r := f.evaluateProvider(p, headers, p.audiences)
                if !r.allowed {
                    return r // present-and-invalid → fail
                }
                return r
            }
        }
        return evalResult{allowed: true} // missing → OK
    case reqAllowMissingOrFailed:
        return evalResult{allowed: true}
    default:
        return evalResult{allowed: false, err: errors.New("unknown requirement kind")}
    }
}

func (f *filter) evaluateProvider(p *compiledProvider, headers http.Header, effectiveAudiences []string) evalResult {
    tokens := extractTokens(p, headers)
    if len(tokens) == 0 {
        return evalResult{allowed: false, err: jwt.ErrJwtMissed}
    }

    // For each extracted token, try to validate; first-success wins.
    var lastErr error = jwt.ErrJwtMissed
    for _, et := range tokens {
        t, err := jwt.Parse(et.raw)
        if err != nil {
            lastErr = err
            continue
        }
        // Resolve key from JWKS.
        var keyset *jwt.JWKSet
        if p.localJwks != nil {
            keyset = p.localJwks
        } else if p.jwksFetcher != nil {
            keyset, err = p.jwksFetcher.Get(f.dcb.Context())
            if err != nil {
                f.activeRC.stats.jwksFetchFailed.Inc()
                lastErr = err
                continue
            }
            f.activeRC.stats.jwksFetchSuccess.Inc()
        }
        key, err := keyset.Lookup(t.Kid, t.Alg)
        if err != nil {
            lastErr = err
            continue
        }
        // Verify signature.
        if err := t.VerifySignature(key, t.Alg); err != nil {
            lastErr = err
            continue
        }
        // Validate claims.
        if err := t.ValidateClaims(jwt.ValidateOptions{
            Issuer:    p.issuer,
            Audiences: effectiveAudiences,
            ClockSkew: p.clockSkew,
            Now:       time.Now(),
        }); err != nil {
            lastErr = err
            continue
        }
        // SUCCESS.
        return evalResult{allowed: true, token: t, provider: p}
    }
    return evalResult{allowed: false, err: lastErr}
}
```

### 6.9 Side-effect emit-order (per §1 item 6 + §11.P10 + §11.P13)

```go
// applyResult applies the evaluation result to the request + emits counter delta.
func (f *filter) applyResult(r evalResult, headers http.Header, req *compiledRequirement) envoyhttp.FilterHeadersStatus {
    if !r.allowed {
        // Deny path.
        f.activeRC.stats.denied.Inc()
        f.emitDenyResponse(r.err)
        return envoyhttp.HeaderStopIteration
    }

    // Allow path: emit counter + side-effects + continue.
    f.activeRC.stats.allowed.Inc()
    if r.token != nil && r.provider != nil {
        f.applySideEffects(r.token, r.provider, headers)
    }
    return envoyhttp.HeaderContinue
}

// applySideEffects executes the 4-step side-effect emit-order on validation success.
// Order: (1) strip-on-success → (2) forward_payload_header → (3) claim_to_headers → (4) clear_route_cache.
func (f *filter) applySideEffects(t *jwt.Token, p *compiledProvider, headers http.Header) {
    // 1. Strip (forward=false) — strip from Authorization/from_headers/from_params; from_cookies untouched per proto caveat.
    if !p.forward {
        // Strip Authorization Bearer if it was used; strip from_headers values; strip from_params query params.
        // Cookies are NOT stripped per proto field 5 comment "caveat: only works for from_header/from_params".
        stripExtractionSources(headers, p, t)
    }

    // 2. forward_payload_header.
    if p.forwardPayloadHeader != "" {
        encoded := encodeBase64URL(t.RawPayload, p.padForwardPayloadHdr)
        headers.Set(p.forwardPayloadHeader, encoded)
    }

    // 3. claim_to_headers (dot-notation; array claims rejected per §11.P10).
    for _, ch := range p.claimToHeaders {
        val, err := t.PayloadClaim(ch.claimName)
        if err != nil {
            continue // silent skip per Envoy's claim_to_headers tolerance
        }
        if s, ok := stringify(val); ok {
            headers.Set(ch.headerName, s)
        }
    }

    // 4. clear_route_cache.
    if p.clearRouteCache {
        f.dcb.ClearRouteCache()
    }
}

// emitDenyResponse maps the failure-reason to status + body + headers per §4.
func (f *filter) emitDenyResponse(reason error) {
    code := mapStatusToHTTPCode(reason)  // 401 for most; 403 for ErrJwtAudienceNotAllowed
    var body string
    var hdrs []envoyhttp.Header
    if f.activeRC.stripFailureResponse {
        body = ""
    } else {
        body = reason.Error()
        wwwAuth := fmt.Sprintf(`Bearer realm=%q`, f.originalURI)
        if reason != jwt.ErrJwtMissed {
            wwwAuth += `, error="invalid_token"`
        }
        hdrs = append(hdrs, envoyhttp.Header{Name: "www-authenticate", Value: wwwAuth})
        hdrs = append(hdrs, envoyhttp.Header{Name: "content-type", Value: "text/plain"})
    }
    f.dcb.SendLocalReply(code, body, hdrs)
}

func mapStatusToHTTPCode(reason error) int {
    if errors.Is(reason, jwt.ErrJwtAudienceNotAllowed) {
        return 403
    }
    return 401
}
```

### 6.10 RemoteJwks async-fetch lifecycle (per §11.P4 + §11.P5)

The `compiledProvider.jwksFetcher` field holds a `*jwks.Fetcher` constructed at `buildCompiledProvider` time (configure-once-at-listener-init):

- **Initial fetch** (per §11.P4): if `async_fetch.fast_listener: false` (default), `jwks.New()` blocks until the initial fetch completes (success or failure). Failure returns an error from `New()`, which causes the entire JwtAuthentication config to fail listener-load — operationally correct (an invalid JWKS endpoint at boot should fail loud).
- **Initial fetch with `fast_listener: true`**: `jwks.New()` returns immediately; first few requests against the provider see `ErrJwksNotReady` (mapped to `ErrJwksFetchFail` + 401 denied) until the background goroutine completes the initial fetch.
- **Successful refresh schedule** (per §11.P5): after each successful fetch, schedule the next refresh at `cacheDuration - 5s` via `time.AfterFunc`. Default `cacheDuration = 10 minutes`, refresh fires at 9 minutes 55 seconds.
- **Failed refresh schedule** (per §11.P4): after each failed fetch, schedule the next attempt at `failedRefetchDuration` (default 1s, configurable via `JwksAsyncFetch.failed_refetch_duration`). No exponential backoff; fixed-interval retry.
- **On-demand fetch** (per §11.P4): when `async_fetch` is nil entirely (RemoteJwks without async_fetch sub-message), Envoy fetches JWKS on the FIRST request that needs it; first few requests block until fetch completes (Envoy's "on-demand fetch" mode per `async_fetch` proto comment). Phase-17 envoy-go MVP — if `async_fetch` is nil, treats as `fast_listener: false` with `cacheDuration` honored (mirrors blocking-initial-fetch but at first-request rather than at New()-time; ADR-0150 §Decision documents the simplification).
- **Cleanup**: filter `OnDestroy` does NOT close the fetcher — the fetcher is owned by `factoryState.listenerRC.providers[<name>]` and shared across all filter instances of the listener; lifetime is listener-lifetime. The fetcher is closed when the listener drains (future graceful-drain integration; phase-17 MVP relies on goroutine-leak-on-restart per HCM lifecycle scope).

ADR-0150 codifies the lifecycle discipline.

### 6.11 `DecodeData` + `DecodeTrailers` + `OnDestroy` + `SetDecoderCallbacks`

```go
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus { return envoyhttp.DataContinue }
func (f *filter) DecodeTrailers(_ http.Header) envoyhttp.FilterTrailersStatus { return envoyhttp.TrailersContinue }
func (f *filter) OnDestroy() {} // no per-stream resources to release
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.dcb = cb }
```

### 6.12 `parsePerRoute` + `resolvePerRouteConfig`

```go
// parsePerRoute parses one PerRouteConfig TPFC entry per §5.1.
func parsePerRoute(any *anypb.Any) (proto.Message, error) {
    var pr jwt_authnv3.PerRouteConfig
    if err := any.UnmarshalTo(&pr); err != nil {
        return nil, fmt.Errorf("jwt_authn: per-route unmarshal: %w", err)
    }
    // PGV-mirror: RequirementSpecifier oneof must be set per §11.P9.
    if pr.GetRequirementSpecifier() == nil {
        return nil, errors.New("jwt_authn: per_route requirement_specifier is required")
    }
    if rn := pr.GetRequirementName(); rn != "" || pr.GetRequirementSpecifier().(*jwt_authnv3.PerRouteConfig_RequirementName) != nil {
        if rn == "" {
            return nil, errors.New("jwt_authn: per_route requirement_name must be at least 1 rune")
        }
    }
    return &pr, nil
}

func (s *factoryState) resolvePerRouteConfig(msg proto.Message) *compiledPerRoute {
    if msg == nil {
        return nil
    }
    pr, ok := msg.(*jwt_authnv3.PerRouteConfig)
    if !ok {
        return nil
    }
    if cached, ok := s.perRoute.Load(pr); ok {
        return cached.(*compiledPerRoute)
    }
    fresh := buildCompiledPerRoute(pr)
    actual, _ := s.perRoute.LoadOrStore(pr, fresh)
    return actual.(*compiledPerRoute)
}

func buildCompiledPerRoute(pr *jwt_authnv3.PerRouteConfig) *compiledPerRoute {
    switch t := pr.GetRequirementSpecifier().(type) {
    case *jwt_authnv3.PerRouteConfig_Disabled:
        return &compiledPerRoute{disabled: t.Disabled, requirementName: ""}
    case *jwt_authnv3.PerRouteConfig_RequirementName:
        return &compiledPerRoute{disabled: false, requirementName: t.RequirementName}
    default:
        return &compiledPerRoute{disabled: false, requirementName: ""}
    }
}
```

---

## 7. Differential fixture `0019-http-jwt-authn`

### 7.1 Per-request matrix (8 scenarios per BRAINSTORM §6 with refinements per §1.1 amendments)

| # | Scenario | Request | Expected response | Counter delta (envoy-go side) | §11 cross-ref |
|---|---|---|---|---|---|
| 1 | valid-token-allow-RS256-RemoteJwks | `GET /` with `Authorization: Bearer <RS256-signed valid token>` | 200; backend echoes; `allowed +1`; `jwks_fetch_success +1` on first request | `http.<HCM>.jwt_authn.allowed +1` + `jwks_fetch_success +1` (one per provider lifetime) | §11.P1 + §11.P5 + §11.P6 |
| 2 | valid-token-allow-ES256-LocalJwks | `GET /` with `Authorization: Bearer <ES256-signed valid token>` | 200; backend echoes; `allowed +1`; NO `jwks_fetch_*` (LocalJwks path) | `allowed +1` | §11.P1 + §11.P6 |
| 3 | missing-token-deny | `GET /` (no Authorization header) | 401; body byte-exact `Jwt is missing` (14 bytes); `www-authenticate: Bearer realm="/"` (NO `, error="invalid_token"` per §11.P2 — `JwtMissed` case omits error param); `denied +1` | `denied +1` | §11.P1 + §11.P2 + §1.1 amendments 8 + 12 |
| 4 | expired-token-deny | `GET /` with `Authorization: Bearer <token with exp in the past>` | 401; body byte-exact `Jwt is expired` (14 bytes); `www-authenticate: Bearer realm="/", error="invalid_token"`; `denied +1` | `denied +1` | §11.P1 + §1.1 amendments 8 + 12 |
| 5 | bad-signature-deny | `GET /` with `Authorization: Bearer <token with tampered signature>` | 401; body byte-exact `Jwt verification fails` (22 bytes); `www-authenticate: Bearer realm="/", error="invalid_token"`; `denied +1` | `denied +1` | §11.P1 |
| 6 | bypass-cors-preflight | `OPTIONS /` with `Origin: http://example.com` + `Access-Control-Request-Method: POST` (no Authorization) | 200; backend echoes (preflight bypassed); `cors_preflight_bypassed +1` (NO `allowed/denied` increments) | `cors_preflight_bypassed +1` | §11.P1 + §1.1 amendment 10 |
| 7 | per-route 8th-canonical delegation | `GET /alt-req` with `Authorization: Bearer <token valid for alt-req's referenced provider>`; per-route `requirement_name: "alt-req"` resolves against listener's requirement_map | 200; backend echoes; `allowed +1` | `allowed +1` | §11.P9 + §11.P12 |
| 8 | per-route 8th-canonical disabled | `GET /per-route-disabled` (no Authorization); per-route `disabled: true` per §5.1 (a) | 200; backend echoes (route bypasses validation); NO counter increments | (no counter increments) | §11.P9 + §1.1 amendment 5 |

Optional 9th + 10th scenarios at SPEC reshape time (deferred from MVP fixture set; impl-time MAY add):
- forward_payload_header + claim_to_headers backend-arrival assertion (verify the upstream backend receives the named headers with correct payload/claim values).
- requires_any multi-provider OR scenario (rule with `requires_any: [provider_name: "a", provider_name: "b"]`; token valid against provider b).

### 7.2 Topology

`test/fixtures/0019-http-jwt-authn/`:
- `envoy.yaml` — reference Envoy config.
- `envoy-go.yaml` — equivalent envoy-go config.
- `inputs/driver.go` — Go driver issuing the 8 scenarios; byte-exact body comparison; per-counter delta scrape; in-process JWKS server.
- `expectations.yaml` — per-scenario allow-list / counter-delta map.
- `README.md` — fixture overview + scenario list + reference config citations + JWKS server lifecycle notes + RS256/ES256 keypair generation notes.

Three listeners + one cluster + one in-process JWKS server (extends phase-11/12/13/14/15/16 fixture topology with the new JWKS backend):

- **Listener `l_test_a`** (TCP plaintext): HCM with one filter-chain `jwt_authn → router`. Listener-level config:
  ```yaml
  envoy.filters.http.jwt_authn:
    providers:
      provider-rs256:
        issuer: https://issuer-rs.example.com
        audiences: [api-rs]
        remote_jwks:
          http_uri:
            uri: http://<jwks-server>/.well-known/jwks-rs.json
            cluster: c_jwks_backend
            timeout: 1s
          cache_duration: { seconds: 300 }
      provider-es256:
        issuer: https://issuer-es.example.com
        audiences: [api-es]
        local_jwks:
          inline_string: '{"keys":[<ES256 JWK>]}'
      provider-alt:
        issuer: https://issuer-alt.example.com
        audiences: [api-alt]
        remote_jwks: { http_uri: { uri: http://<jwks-server>/.well-known/jwks-alt.json, cluster: c_jwks_backend, timeout: 1s }, cache_duration: { seconds: 300 } }
    rules:
      - match: { prefix: / }
        requires:
          requires_any:
            requirements:
              - provider_name: provider-rs256
              - provider_name: provider-es256
    requirement_map:
      alt-req:
        provider_name: provider-alt
    bypass_cors_preflight: true
  ```
  Routes:
  - `/` → cluster `c_backend` (scenarios 1-5; rules-dispatch matches all paths).
  - `/alt-req` → cluster `c_backend`; per-route TPFC `PerRouteConfig{requirement_name: "alt-req"}`. Scenario 7.
  - `/per-route-disabled` → cluster `c_backend`; per-route TPFC `PerRouteConfig{disabled: true}`. Scenario 8.
- **Listener `l_jwks_backend`** + cluster `c_jwks_backend`: in-process JWKS server serving `jwks-rs.json` + `jwks-alt.json` at fixed URIs.
- **Cluster `c_backend`**: echo-backend cluster (reuses `test/helpers/echobackend/` from phase 14/15/16).

### 7.3 PKI generation (reuses phase-16 `pki/gen.go` pattern)

Three RSA-2048 keypairs + one ECDSA P-256 keypair generated at fixture-test-setup time:
- `keys/rs256-private.pem` + `keys/rs256-public.pem` — for provider-rs256 (RemoteJwks via in-process JWKS server).
- `keys/es256-private.pem` + `keys/es256-public.pem` — for provider-es256 (LocalJwks; inline JWK).
- `keys/alt-private.pem` + `keys/alt-public.pem` — for provider-alt (RemoteJwks; per-route 7 scenario).
- `keys/tampered-public.pem` — NOT a real signing key; used to seed a 4th JWK for scenario 5's "bad-signature" path where the signing private key does NOT match any public JWK.

JWK Set serialization happens inside the test helper at fixture setup; both inline JWKs (for LocalJwks) and JWKS-server-served JWKs (for RemoteJwks) share the same generation discipline.

### 7.4 Test-helper JWKS server (NEW under `test/helpers/jwksbackend/`)

NEW directory `test/helpers/jwksbackend/` (or similar location at SPEC author's discretion; locked to `jwksbackend/` for phase-17). Serves a configurable JWK Set at a known URI; both `envoy.yaml` and `envoy-go.yaml` wire to the same JWKS URI via `c_jwks_backend` cluster. Lifecycle: spawn-per-fixture; tear-down at fixture-done. The package exposes:

```go
package jwksbackend

import (
    "context"
    "net"
    "net/http"
)

// Server is an in-process HTTP server serving JWKS responses at configurable paths.
type Server struct {
    Listener net.Listener
    URLs     map[string]string  // path → JWK Set JSON content
    // ... opaque state
}

// New starts a JWKS server on the given address; routes serve the JWK Set JSON.
func New(ctx context.Context, addr string, routes map[string]string) (*Server, error)

// Stop terminates the server.
func (s *Server) Stop()
```

The fixture driver allocates a free TCP port at setup, starts the JWKS backend on that port, wires the cluster `c_jwks_backend` to that port in both yaml configs, runs the scenarios, then stops the backend at teardown.

### 7.5 Per-scenario expectations YAML

`expectations.yaml` per scenario:
- **Response status:** byte-exact between Envoy and envoy-go for every scenario (200 on allow paths; 401 on JWT-related denies; 200 on cors-bypass + per-route-disabled).
- **Response headers:** lowercase wire-form, set-equal modulo the existing allow-list (`date`, `server`). For deny scenarios 3-5: `www-authenticate` header asserted byte-exact (including the conditional `, error="invalid_token"` suffix per §1.1 amendment 12).
- **Response body:** byte-exact on ALL scenarios:
  - Allow paths (1, 2, 6, 7, 8): backend echo bytes or preflight-passthrough bytes.
  - Deny paths (3, 4, 5): byte-exact canonical jwt_verify_lib string (e.g., 14-byte `"Jwt is missing"`; 22-byte `"Jwt verification fails"`).
- **Counter deltas:** `/stats/prometheus` scrape equivalence on the 5 active base counters (`allowed`, `denied`, `cors_preflight_bypassed`, `jwks_fetch_success`, `jwks_fetch_failed`); the 2 jwt_cache_* counters are STRUCTURALLY UNREACHABLE under phase-17 MVP per §1.1 amendment 9 + §8 deferral 8.
- **Per-route fixture-config disposition:** scenarios 7 + 8 exercise BOTH per-route 8th-canonical arms (`requirement_name` + `disabled: true`).
- **`response_code_details` field:** NOT asserted (envoy-go MVP defers emission per §1.1 amendment 11 + §8 deferral 13; documented divergence-window).
- **`filter_state_rules` field:** NOT exercised (silent-ignored per §1.1 amendment 1; envoy-go-Envoy parity achievable only when filter_state_rules is empty across both sides).

### 7.6 Optional 9th/10th scenarios (deferred to impl-time)

- **9th — `forward_payload_header` + `claim_to_headers` backend-arrival assertion**: configure `provider-rs256` with `forward_payload_header: x-jwt-payload` + `claim_to_headers: [{header_name: x-jwt-sub, claim_name: sub}, {header_name: x-jwt-groups, claim_name: groups}]`; backend asserts the named headers arrive with the correct values; tests both string claim (sub) and nested-claim-dot-notation (groups.0 if applicable; or just sub for simplicity).
- **10th — `requires_any` multi-provider OR with token-valid-against-second**: configure rule with `requires_any: [provider_name: "a", provider_name: "b"]`; issue token valid only against b; expect allow disposition + `allowed +1`.

Both scenarios are STRUCTURALLY SUPPORTED by the MVP filter implementation; their addition to fixture 0019 is impl-time discretion.

### 7.7 Driver shape

`inputs/driver.go` mirrors the phase-16 driver shape:
- 8 scenarios; each a function `runScenarioN(ctx, baseURL) error` returning the assertion result.
- Per-scenario assertion helper for status + body + counter-delta.
- JWKS backend setup-helper at TestMain entry; teardown at exit.
- RSA-2048 + ECDSA-P256 keypair generation at TestMain entry via `pki/gen.go` (NEW for jwt_authn; mirrors phase-16's pattern but produces JWK Sets instead of X.509 cert chains).
- Stats scrape per scenario; counter-delta computation against pre-scrape baseline.

Total estimated driver size: ~300-400 LoC (similar to phase-16 driver + the additional JWKS-backend lifecycle helpers).

**No H2 differential coverage.** Phase 17 fixture 0019 is HTTP/1.1-only per the existing §9 family-row convention.

---

## 8. Deferred items (~13 items + 1 foot-gun; per BRAINSTORM §8 with §11 refinements)

For future phase consideration (none are blockers for closing row 17 phase-done; all auditable in the ADR-0040 deferral trail):

1. **`payload_in_metadata` (JwtProvider field 9)** — encodes JWT payload as dynamic metadata. DEFERRED: couples to dynamic-metadata family (same family blocked at phase-16 forward-pointer item 9). Future dynamic-metadata-family phase lands `(FilterCallbacks).SetDynamicMetadata(key, value)` primitive; jwt_authn re-enables this field at that point.

2. **`header_in_metadata` (JwtProvider field 14)** — encodes JWT header as dynamic metadata. DEFERRED: couples to dynamic-metadata family (same as item 1).

3. **`failed_status_in_metadata` (JwtProvider field 16)** — encodes failure status as dynamic metadata on rejection. DEFERRED: couples to dynamic-metadata family.

4. **`normalize_payload_in_metadata` (JwtProvider field 18)** — sub-message controlling payload normalization shape for metadata emission. DEFERRED: couples to dynamic-metadata family + payload_in_metadata coupling chain.

5. **HS family algorithms (HS256/HS384/HS512)** — DEFERRED: requires symmetric-secret config plumbing in JwtProvider (security-sensitive — operators must securely provision shared secrets). Future algorithm-extension phase enables.

6. **EdDSA algorithm (Ed25519 / Ed448)** — DEFERRED: less-common; requires Go stdlib `crypto/ed25519`. Could enable as a standalone follow-on.

7. **`none` algorithm** — DEFERRED-PERMANENTLY (intentionally never enabled; security-sensitive — `alg=none` JWTs are unsigned; allowing them defeats authentication). PARSE-REJECT at JWK parse + runtime-reject at token parse; no operator-config knob to enable.

8. **`jwt_cache_config` (JwtProvider field 12)** — validated-JWT result LRU cache (default 100-entry per proto). DEFERRED: couples to a future caching-framework phase that introduces a generic LRU primitive. Net impact: the `jwt_cache_hit` + `jwt_cache_miss` counters are STRUCTURALLY UNREACHABLE under phase-17 MVP.

9. **~~Deprecated `RequirementRule.requires` field~~** — WITHDRAWN per §1.1 amendment 4. The field is NOT deprecated in v1.37.2; envoy-go MVP honors both oneof arms proto-faithful. No deferral entry; entry preserved here as placeholder for §8 numbering continuity.

10. **JWKS bounded retry/backoff customization beyond canonical default** — operator-config knob to override the canonical fixed-1-second `failed_refetch_duration` policy (per §11.P4 REFUTED exponential-backoff hypothesis). DEFERRED: MVP picks the fixed-interval default; honoring `failed_refetch_duration` proto field IS in scope at MVP (the field is consumed when set). Future operator-ergonomics phase MAY add exponential backoff or jitter.

11. **JWKS cache-invalidation hooks** (cache-bust on operator signal; e.g., admin-API endpoint `POST /jwt_authn/<provider>/refresh_jwks`). DEFERRED: couples to admin-API extension.

12. **`filter_state_rules` (JwtAuthentication field 3; NEW deferral per §1.1 amendment 1)** — runtime requirement-selection driven by `StreamInfo.FilterState[<name>]` string. DEFERRED: couples to future filter-state-family phase (kindred to dynamic-metadata family).

13. **`response_code_details` field-emission divergence-window (NEW deferral per §1.1 amendment 11)** — Envoy emits `jwt_authn_access_denied{<reason>}` per filter.cc; envoy-go MVP defers (current phase-04 HCM does not surface response_code_details to local-reply callers). Couples to future response-code-details framework phase (same phase as phase-16 rbac §8.12 forward-points to).

14. **Access-log integration** for jwt_authn success/failure log fields (per-request log line emits `%JWT_PROVIDER%` / `%JWT_SUBJECT%` / `%JWT_FAILURE_REASON%` access-log formatters). DEFERRED: couples to phase-16 forward-pointer access-log item 7 + access-log formatter extension framework.

15. **`subjects` (JwtProvider field 19; NEW deferral per §1.1 amendments 2 + 3)** — StringMatcher on JWT `sub` claim. v1.37.x JWT-SVID-class extension. DEFERRED: couples to claim-coverage-extension family.

16. **`require_expiration` (JwtProvider field 20; NEW deferral per §1.1 amendments 2 + 3)** — bool; if true, JWT MUST carry `exp` claim. v1.37.x extension. DEFERRED: couples to claim-coverage-extension family.

17. **`max_lifetime` (JwtProvider field 21; NEW deferral per §1.1 amendments 2 + 3)** — Duration; rejects JWTs with `exp - now > max_lifetime`. v1.37.x extension. DEFERRED: couples to claim-coverage-extension family.

18. **`clear_route_cache` implicit-on-claim-side-effect trigger (NEW deferral)** — per JwtProvider.clear_route_cache proto comment lines 299-305, Envoy clears the route cache when `clear_route_cache: true` OR (claim_to_headers added at least one header OR payload_in_metadata is set). envoy-go MVP honors only the explicit `clear_route_cache: true`; the implicit-on-side-effect trigger is DEFERRED. Operators relying on the implicit trigger see envoy-go-Envoy DIVERGENCE.

**Foot-gun (not numbered)**: JwtRequirement Set recursion-depth — no parse-time depth-cap on `requires_any` / `requires_all` recursion; mirrors Envoy's permissive disposition. Operators writing deeply-nested rules may hit Go-stack-depth issues at config-load time; documented forward-pointer foot-gun per §2.12 + BEHAVIOR_CONTRACT phase-17 forward-pointer notes.

---

## 9. Cross-references against phase-16 deferred-items list — forward-pointer pickup

Phase-16 REVIEW.md §9 enumerates 15 deferred items. Phase-17 evaluates which opportunistically close vs continue deferred:

### 9.1 NO PICKUP items

- **Phase-16 item 7 — shadow access-log integration**: NO PICKUP (jwt_authn has no shadow surface analogous to rbac's shadow_rules; the parallel concept would be a hypothetical `shadow_providers` or `shadow_requirements`, which is NOT part of the Envoy proto).
- **Phase-16 item 9 — LOG-action `access_log_hint` dynamic-metadata primitive**: NO PICKUP (jwt_authn has no LOG-action analog; the dynamic-metadata family is uniformly deferred per phase-17 deferrals 1-4).
- **Phase-16 item 10 — CEL three-field condition evaluation**: NO PICKUP at MVP (jwt_authn doesn't have CEL coupling at MVP per Q3+Q4 picks). Future jwt_authn extension to CEL-based provider/requirement selection (per phase-17 deferral 11 — renumbered from BRAINSTORM §8 deferral 13) would close this.
- **Phase-16 item 11 — Principal_Custom v1.32.4 binding-absent workaround**: NO PICKUP (jwt_authn doesn't consume rbac's Principal_Custom).
- **Phase-16 items 1-6, 12-15**: NO PICKUP (rbac-specific concerns + tech-debt cleanup + SPEC §13.2 housekeeping — none structurally close in phase 17).

### 9.2 Potential pickup of phase-16 item 8 (response_code_details framework primitive)

Phase-16 item 8 — `response_code_details` framework primitive: **POTENTIAL PICKUP NOT TAKEN at phase-17 MVP**. jwt_authn's deny-path emits a `response_code_details` value (per §1.1 amendment 11 — `jwt_authn_access_denied{<reason>}`) but envoy-go MVP defers the field emission identically to how phase-16 rbac deferred its `rbac_access_denied_matched_policy[<id>]` emission. The phase-17 deferral (item 13) mirrors phase-16 item 8 verbatim — both filters' deny paths have a canonical Envoy response_code_details string but neither envoy-go MVP threads it through HCM to access-log. **Joint closure** at a future response-code-details framework phase would close BOTH phase-16 item 8 AND phase-17 item 13 simultaneously (the HCM-side primitive serves both filter families).

### 9.3 Forward-pointer net change for phase 17

0 closures expected at phase-17 phase-done. Phase 17 adds 13+ new deferred items (§8 above) + extends the dynamic-metadata-family deferred-cluster (items 1-4) which already includes phase-16 items + extends the response_code_details deferred-cluster (item 13) by ONE new emitter (jwt_authn alongside rbac).

---

## 10. ADR anchor map (Lands-in-Task hypothesis)

8 ADRs anticipated per BRAINSTORM §7 — tied with phase-16's 7+1=8 landed ADRs for LARGEST §9-row roster to date. ADR-0147 is the highest-numbered ADR landed in phase 16; ADR-0148 is the next-free.

| ADR | Subject | Lands-in-Task | Anchors §§ |
|---|---|---|---|
| **ADR-0148** | `internal/filter/http/jwtauthn/` package shape — single-token directory matching cors/fault/csrf/buffer/compressor/localratelimit/bandwidthlimit/rbac precedent + boot registration ordering (alphabetical-after-header_mutation) + DECODER-only `HTTPFilter` value (`Encoder: nil`; 4th §9 row to ship pure decode-side per phase-12 csrf + phase-13 buffer + phase-16 rbac precedent) + 7-base-counter `filterStats` (per §1.1 amendment 9 — REFUTES BRAINSTORM 8-counter-per-provider-scaling hypothesis; NO gauges; NO histograms) + non-lazy unconditional counter allocation at New() time + deny-path wire shape (401 default, 403 for JwtAudienceNotAllowed; SendLocalReply with body + WWW-Authenticate header per §4 + §1.1 amendments 8 + 12) | Task 2 | §1 items 1 + 2 + 5 + 7 + §4 |
| **ADR-0149** | `compiledConfig` shape + 5-of-6 outer-field consumed + 1-outer-silent-ignored decomposition (filter_state_rules) + 13-of-21 JwtProvider consumed + 8-silent-ignored decomposition + 6-variant JwtRequirement evaluator + algorithm allow-list discipline (RS256/384/512 + ES256/384/512) + side-effect emit-order (strip → forward_payload_header → claim_to_headers → clear_route_cache) + listener-level rules dispatch + envoy-go-side defensive PGV-mirror validation + clock_skew_seconds 60s default consumed | Task 2 | §1 item 3 + §1 item 6 + §1.1 amendments 1-4 + 7 + §6.2 + §6.5-§6.9 |
| **ADR-0150** | HTTP-outbound JWKS fetcher primitive at NEW top-level package `internal/jwks/` — `Fetcher` opaque type wrapping URI + cache duration + AsyncFetch + RetryPolicy; `New(uri, cacheDuration, asyncFetch, retryPolicy)` constructor (blocking or non-blocking initial fetch per `fast_listener`); `Get(ctx)` returns cached JWKSet or ErrJwksNotReady; refresh schedule 5s-before-TTL via `time.AfterFunc` per §11.P5; failed-refetch fixed-interval (NOT exponential backoff per §11.P4 REFUTED); cross-phase-reusable for future ext_authz HTTP-mode + oauth2 token-endpoint flows | Task 3 | §3.1 + §1 item 8 |
| **ADR-0151** | JWT verifier framework primitive at NEW top-level package `internal/jwt/` — `Parse(raw)` parses 3-part JWT structure; `Token.VerifySignature(key, alg)` with RS+ES algorithm allow-list (PARSE-REJECT unsupported algs); `Token.ValidateClaims(opts)` checks exp + nbf + iss + aud per ValidateOptions with clock-skew tolerance; `Token.PayloadClaim(path)` dot-notation extractor (array claims rejected per §11.P10); ~20 canonical error sentinels mirroring jwt_verify_lib status codes; pure-Go stdlib (`crypto/rsa.VerifyPKCS1v15` + `crypto/ecdsa.Verify` + `encoding/base64.RawURLEncoding`); cross-phase-reusable | Task 4 | §3.2 + §1 item 8 |
| **ADR-0152** | Token extraction across all 4 sources — default Authorization Bearer + access_token query param (when no explicit per-provider extraction-sources set) + configured from_headers (declared order; value_prefix substring-search) + configured from_params (case-sensitive; URL-decoded; first-value-only per §11.P14) + configured from_cookies (case-sensitive; verbatim value per §11.P15); iteration order matches Envoy extractor.cc (headers first, then params, then cookies); first-success-wins discipline; empty-extraction = `JwtMissed` failure-reason | Task 5 | §1 item 6 + §6.7 + §1.1 amendment 7 |
| **ADR-0153** | Per-route 8th canonical: `oneof{disabled(bool) | requirement_name(string)}` per §1.1 amendment 5 (REFUTES BRAINSTORM `Empty`-form hypothesis; PerRouteConfig.Disabled is varint bool) + delegation via listener-level requirement_map + dangling-reference RUNTIME-RESOLVE at request time per §1.1 amendment 6 (REFUTES parse-reject hypothesis; mirrors Envoy filter_config.cc `findPerRouteVerifier` runtime-resolve discipline; on miss emits 403 + "Failed JWT authentication: Wrong requirement_name: <name>") + per-route stats SHARED with listener-level per §5.2 + §11.P8 (NO per-route stat_prefix; pure delegation; mirrors phase-12 csrf + phase-13 buffer + phase-14 compressor SHARED-stats discipline) + ADR-0125 §(xiii) amendment paragraph (NEW 8th canonical entry — ADR-0125's roster grows from 7 to 8) | Task 7 | §5 + ADR-0125 §(xiii) + §1.1 amendments 5 + 6 |
| **ADR-0154** | Stat surface 7 base counters (allowed + denied + cors_preflight_bypassed + jwks_fetch_success + jwks_fetch_failed + jwt_cache_hit + jwt_cache_miss; the last 2 structurally unreachable under MVP per §8 deferral 8) + NO per-provider scaling per §1.1 amendment 9 (REFUTES BRAINSTORM 8-per-provider hypothesis) + NO gauges + NO histograms + HCM-rooted SN2-reuse namespace `http.<HCM_stat_prefix>.jwt_authn.<counter>` per §11.P7 (RATIFIED-PENDING-IMPL-TIME-EMPIRICAL-SCRAPE; phase-16 ADR-0145 SN2-reuse precedent applies) + per-route stats SHARED per §5.2 (NO INDEPENDENT-stats discipline; pure delegation; mirrors phase-12/13/14) | Task 8 | §1 item 7 + §5.2 + §1.1 amendments 9 + 10 + §13.2 |
| **ADR-0155** | Deny-path wire shape — 401 for most failure-reasons + 403 for `JwtAudienceNotAllowed` per §1.1 amendment 8 + §11.P1 (REFINES BRAINSTORM "always 401" hypothesis) + body = canonical jwt_verify_lib `getStatusString(status)` per §11.P1 (~70-string table; 5+ commonly-hit) + WWW-Authenticate header per RFC 6750 `Bearer realm="<original_uri>"` + conditional `, error="invalid_token"` append for non-`JwtMissed` per §1.1 amendment 12 + §11.P2 (REFINES BRAINSTORM `realm="<issuer>"` hypothesis — realm uses request URI) + `strip_failure_response: true` strips both body AND www-authenticate per §11.P3 + `response_code_details = "jwt_authn_access_denied{<reason>}"` divergence-window per §1.1 amendment 11 + §8 deferral 13 (envoy-go MVP defers field emission per phase-04 HCM scope; mirrors phase-16 ADR-0146 response_code_details discipline) + per-route runtime-resolve error case emits 403 + plain body (NO www-authenticate header) per §1.1 amendment 6 | Task 9 | §1 item 6 + §4 + §1.1 amendments 6 + 8 + 11 + 12 + §11.P1 + §11.P2 + §11.P3 |

**Plus an ADR-0125 in-place amendment paragraph §(xiii)** (NOT a new ADR; authored at this SPEC commit per phase-13 ADR-0127-v2 + phase-14 ADR-0125 §(viii)-(x) + phase-15 ADR-0125 §(xi) + phase-16 ADR-0125 §(xii) in-place-update precedent): documents phase 17 jwt_authn as the FIRST row to use the **8th canonical per-route pattern** (oneof{disabled(bool) | requirement_name(string)} with string-reference-delegation via listener-level requirement_map; SHARED-stats discipline). ADR-0125's canonical-pattern roster grows from 7 to 8. The 8th canonical's stat-discipline returns to SHARED (after 6th + 7th's INDEPENDENT), reflecting the absence of stateful per-route policy-evaluation state. See §5.4 for the verbatim amendment paragraph.

**ADR-0045 split decision:** Phase-17 SPEC author's call is **single-row, NOT INVOKED**. LoC estimate refined post-§11 empirical-pin resolution: production ~1800-2200 LoC (jwtauthn package ~1000-1300 + internal/jwks ~300-450 + internal/jwt ~250-400 + fixture driver ~300-400). The SPEC §11-pin resolutions slightly NARROWED the framework primitive scope (`internal/jwks` is JWKS-specific, NOT a generic httpclient; `internal/jwt` has clearer error-set after §11.P1 + §11.P10 + §11.P14 + §11.P15 RATIFICATIONS); the production envelope is borderline ADR-0045 1500-LoC trigger but mirrors phase-13/14/15/16 single-row-borderline-LoC precedent. Anticipated PLAN runs single-row at ~14-16 tasks well-under ADR-0045 25-task / 1500-LoC split-trigger.

ADR-0044 escape-valve held in reserve for ~1 impl-time-unanticipated ADR per phase as working estimate (phase-13 ADR-0127-v2 + phase-14 ADR-0134 + phase-16 ADR-0147 precedent). Most likely surfaces: (i) HTTP-outbound primitive's TLS-config plumbing might require ADR-lift for trust-store coupling (mTLS to JWKS endpoint); (ii) `clear_route_cache` interaction with phase-04 HCM might surface an HCM framework gap; (iii) RemoteJwks fetch failure during request-time may require a synchronization-primitive lift on the per-stream callback path.

SPEC-time may revise the 8-ADR count per ADR-0044 SPEC-time-anticipation discipline.

NO new SN10 flattening rule unless impl-time §11.P7 scrape confirmation refutes the SN2-reuse hypothesis. NO new framework primitives beyond ADR-0150 + ADR-0151.

---

## 11. Empirical-pin block (per BRAINSTORM §10 — all 16 pins resolved IN-SESSION)

This block contains the verbatim Envoy v1.37.2 scrape evidence executed during this SPEC drafting session, per ADR-0004's hard-gate discipline. Mirrors phase 09 / 10 / 11 / 12 / 13 / 14 / 15 / 16 SPEC §11's structure precisely. Probe date: **2026-05-12**.

**Reference source corpus:** Multiple verification axes used in this session (mirrors phase-15 + phase-16 multi-axis verification discipline):

1. v1.32.4 go-control-plane bindings: `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.32.4/extensions/filters/http/jwt_authn/v3/config.{pb.go, pb.validate.go}` (2466 + 2797 LoC).
2. Upstream Envoy v1.37.2 source via WebFetch:
   - `source/extensions/filters/http/jwt_authn/filter.cc` — DecodeHeaders dispatch + SendLocalReply + WWW-Authenticate header construction + response_code_details + cors_preflight bypass.
   - `source/extensions/filters/http/jwt_authn/authenticator.cc` — JWT validation pipeline + claim validation order + allow_missing/allow_missing_or_failed semantics.
   - `source/extensions/filters/http/jwt_authn/jwks_async_fetcher.cc` — async fetch + retry/backoff schedule.
   - `source/extensions/filters/http/jwt_authn/jwks_cache.cc` — JWKS cache lifecycle.
   - `source/extensions/filters/http/jwt_authn/stats.h` — ALL_JWT_AUTHN_FILTER_STATS macro.
   - `source/extensions/filters/http/jwt_authn/filter_config.cc` — per-route TPFC + listener-level rules dispatch table construction.
   - `source/extensions/filters/http/jwt_authn/extractor.cc` — extraction-source iteration.
3. jwt_verify_lib upstream source via WebFetch:
   - `src/status.cc` — `getStatusString()` canonical status → message string mapping (~70 entries).

Verbatim file-line citations are durable on this SPEC drafting session machine.

### Summary disposition table (16 pins; tally below)

| Pin | Topic | Disposition | Amendment cross-ref |
|---|---|---|---|
| **§17.P1** | Exact 401 body byte-form per failure-reason | **RATIFIED-AND-EXTENDED** — body = `getStatusString(status)` jwt_verify_lib mapping (~70 strings); status mapping 401/403 dispatch | §1.1 amendments 8 + 11 + §4 |
| **§17.P2** | WWW-Authenticate header format | **RATIFIED-AND-REFINED** — `Bearer realm="<original_uri>"` + conditional `, error="invalid_token"` for non-JwtMissed; realm uses request URI NOT issuer | §1.1 amendment 12 |
| **§17.P3** | `strip_failure_response=true` wire-bytes effect | **RATIFIED** — body becomes empty string; www-authenticate header NOT set; response_code_details still emitted | §4 |
| **§17.P4** | RemoteJwks async-fetch retry/backoff schedule | **REFUTED** — NO exponential backoff; fixed 1-second `DefaultRefetchAfterFailedSec`; configurable via `failed_refetch_duration` | §2.7 + §8 deferral 10 |
| **§17.P5** | JWKS cache TTL + refresh interval defaults | **RATIFIED-AND-EXTENDED** — default 10min cache; refresh fires 5s before expiry via `RefetchBeforeExpiredSec=5s` | ADR-0150 |
| **§17.P6** | Stat name table + counter dispositions | **REFUTED** — 7 filter-wide counters, NO per-provider scaling; canonical names per ALL_JWT_AUTHN_FILTER_STATS macro | §1.1 amendments 9 + 10 + ADR-0154 |
| **§17.P7** | Prometheus tag-extractor + namespace flattening | **RATIFIED-PENDING-IMPL-TIME** — SN2-reuse hypothesis intact (mirrors phase-14 + phase-15 + phase-16 SN2 reuse) | §1 item 7 + ADR-0154 |
| **§17.P8** | Per-route stat SHARED-vs-INDEPENDENT | **RATIFIED** — SHARED; filter_config.cc `findPerRouteVerifier` has NO stats logic | §5.2 + ADR-0153 |
| **§17.P9** | PGV-required fields per JwtAuthentication + JwtProvider + RequirementRule + PerRouteConfig | **PARTIAL/REFRAMED** — JwtAuthentication has 6 fields (NOT 5); JwtProvider has 21 fields (NOT 17); few PGV constraints; documented | §1.1 amendments 1 + 2 + 5 |
| **§17.P10** | `claim_to_headers` exact behavior | **RATIFIED-AND-EXTENDED** — dot-notation for nested claims; ARRAY claims NOT supported per proto comment | §6.9 + ADR-0149 |
| **§17.P11** | `jwt_cache_config` default values | **RATIFIED** — default jwt_cache_size = 100 per proto; MVP defers entire feature | §8 deferral 8 |
| **§17.P12** | Per-route `requirement_name` dangling reference handling | **REFUTED** — Envoy RUNTIME-resolves (returns 403 + error string); NOT parse-rejected; envoy-go MIRRORS Envoy | §1.1 amendment 6 |
| **§17.P13** | `pad_forward_payload_header` bool semantics | **RATIFIED** — `true` preserves base64 padding; `false` strips | §6.9 + ADR-0149 |
| **§17.P14** | `from_params` resolver | **RATIFIED-AND-REFINED** — case-sensitive; URL-decoded; multi-value array extracts ONLY first value | §6.7 + ADR-0152 |
| **§17.P15** | `from_cookies` resolver | **RATIFIED** — case-sensitive exact match; cookie value used verbatim (no URL-decode) | §6.7 + ADR-0152 |
| **§17.P16** | `allow_missing` + `allow_missing_or_failed` exact dispositions | **RATIFIED-AND-EXTENDED** — `allow_failed`: any status → Ok; `allow_missing`: JwtMissed → Ok else propagate; iteration via requires_any/all evaluator | §6.8 + ADR-0149 |

**Tally:** 6 RATIFIED-AND-EXTENDED + 2 RATIFIED + 1 RATIFIED-AND-REFINED + 1 RATIFIED-PENDING-IMPL-TIME + 3 REFUTED + 2 PARTIAL + 1 REFUTED-WITH-MIRROR-DECISION = **16 pins** all resolved IN-SESSION.

**Structural amendments (re-frame §2.x Decisions):** **12** — JwtAuthentication 6-field outer-envelope with filter_state_rules silent-ignored (§17.P9 → amendment 1); JwtProvider 21-field surface with 3 v1.37.x additions silent-ignored (§17.P9 → amendments 2 + 3); deprecated `requires` is NOT deprecated, honored proto-faithful (§17.P12 → amendment 4); PerRouteConfig.disabled is `bool` not `Empty` (§17.P9 → amendment 5); per-route `requirement_name` dangling references are RUNTIME-RESOLVED not parse-rejected (§17.P12 → amendment 6); clock_skew_seconds HONORED in MVP claim validation (§17.P9 → amendment 7); 401-vs-403 dispatch (§17.P1 → amendment 8); 7-counter stat surface NO per-provider scaling (§17.P6 → amendment 9); `cors_preflight_bypassed` canonical name (§17.P6 → amendment 10); `response_code_details` field-emission divergence-window (§17.P1 → amendment 11); WWW-Authenticate `Bearer realm="<original_uri>"` + conditional error param (§17.P2 → amendment 12). All handled via §1.1 amendment-block channel per phase-12 csrf + phase-14 compressor + phase-15 bandwidth_limit + phase-16 rbac precedent. NO BRAINSTORM §12 amendment cycle required.

### 11.1 Empirical pin §17.P1 — Exact 401 body byte-form per failure-reason (RATIFIED-AND-EXTENDED)

**Probe configuration:** WebFetch of `source/extensions/filters/http/jwt_authn/filter.cc` + WebFetch of `source/extensions/filters/http/jwt_authn/authenticator.cc` + WebFetch of jwt_verify_lib `src/status.cc` for `getStatusString()` canonical mapping.

**Verbatim filter.cc evidence** (via WebFetch):

> *"On verification failure (non-Ok status): When `stripFailureResponse()` is true:
> `decoder_callbacks_->sendLocalReply(code, "", nullptr, absl::nullopt, generateRcDetails(::google::jwt_verify::getStatusString(status)));`
> When `stripFailureResponse()` is false:
> `decoder_callbacks_->sendLocalReply(code, ::google::jwt_verify::getStatusString(status), [uri = this->original_uri_, status](Http::ResponseHeaderMap& headers) { std::string value = absl::StrCat("Bearer realm=\"", uri, "\""); if (status != Status::JwtMissed) { absl::StrAppend(&value, InvalidTokenErrorString); } headers.setCopy(Http::Headers::get().WWWAuthenticate, value); }, absl::nullopt, generateRcDetails(...));`
> HTTP Status Codes: `Http::Code::Forbidden` (403) when `status == Status::JwtAudienceNotAllowed`; `Http::Code::Unauthorized` (401) for all other failures."*

**Verbatim jwt_verify_lib `getStatusString()` table** (via WebFetch; partial subset of ~70-entry mapping):

| Status | Canonical message string |
|---|---|
| `Ok` | `"OK"` |
| `JwtMissed` | `"Jwt is missing"` |
| `JwtNotYetValid` | `"Jwt not yet valid"` |
| `JwtExpired` | `"Jwt is expired"` |
| `JwtBadFormat` | `"Jwt is not in the form of Header.Payload.Signature..."` |
| `JwtHeaderParseErrorBadBase64` | `"Jwt header is an invalid Base64url encoded"` |
| `JwtHeaderParseErrorBadJson` | `"Jwt header is an invalid JSON"` |
| `JwtHeaderBadAlg` | `"Jwt header [alg] field is required and must be..."` |
| `JwtHeaderNotImplementedAlg` | `"Jwt header [alg] is not supported"` |
| `JwtUnknownIssuer` | `"Jwt issuer is not configured"` |
| `JwtAudienceNotAllowed` | `"Audiences in Jwt are not allowed"` |
| `JwtVerificationFail` | `"Jwt verification fails"` |
| `JwksFetchFail` | `"Jwks remote fetch is failed"` |
| `JwksNoValidKeys` | `"Jwks doesn't have any valid public key"` |
| `JwksKidAlgMismatch` | `"Jwks doesn't have key to match kid or alg..."` |
| (~55 more) | (per `src/status.cc` table; ~70 total) |

**Conclusions (pinned) — RATIFIED-AND-EXTENDED:**

- (a) Body bytes = `getStatusString(status)` canonical string (variable length; 14-33 bytes for commonly-hit reasons). NO trailing newline.
- (b) Status: 401 for ALL failure-reasons EXCEPT `JwtAudienceNotAllowed` (403). REFINES BRAINSTORM §4 "always 401" hypothesis per §1.1 amendment 8.
- (c) Phase-17 envoy-go disposition: import jwt_verify_lib status table verbatim into `internal/jwt` error sentinels (ADR-0151 §Decision lists ~20 most-commonly-hit entries; impl-time may add more). Status-code dispatch table per `mapStatusToHTTPCode(reason)` per §6.9.
- (d) **NEW finding** — `response_code_details = "jwt_authn_access_denied{<reason_with_spaces_as_underscores>}"` per `generateRcDetails()` helper. Phase-17 envoy-go MVP DEFERS field emission per §1.1 amendment 11 + §8 deferral 13.

### 11.2 Empirical pin §17.P2 — WWW-Authenticate header format (RATIFIED-AND-REFINED)

**Probe configuration:** Same WebFetch as §17.P1; specific focus on the header-callback closure in filter.cc.

**Verbatim filter.cc evidence:**

> *"`std::string value = absl::StrCat(\"Bearer realm=\\\"\", uri, \"\\\"\"); if (status != Status::JwtMissed) { absl::StrAppend(&value, InvalidTokenErrorString); } headers.setCopy(Http::Headers::get().WWWAuthenticate, value);`
> WWW-Authenticate Header Format: `\"Bearer realm=\\\"{original_uri}\\\"\"` — appends `\", error=\\\"invalid_token\\\"\"` if status is not JwtMissed."*

**Conclusions (pinned) — RATIFIED-AND-REFINED BRAINSTORM §4 hypothesis:**

- (a) Header value format: `Bearer realm="<original_uri>"` (where `<original_uri>` is the request `:path` captured at DecodeHeaders before any route mutation).
- (b) For non-`JwtMissed` statuses, the value additionally appends `, error="invalid_token"` (the `InvalidTokenErrorString` constant; per RFC 6750 §3 challenge syntax).
- (c) **REFINES** BRAINSTORM hypothesis `Bearer realm="<issuer>"` — Envoy uses the REQUEST URI as the realm, NOT the JWT provider's `issuer` field. Operationally meaningful: realm reflects WHAT-WAS-ACCESSED, not WHO-SHOULD-AUTHENTICATE.
- (d) Phase-17 envoy-go disposition: capture `:path` at DecodeHeaders entry → `filter.originalURI`; thread into the SendLocalReply callsite. Conditional append for non-`JwtMissed` statuses. §1.1 amendment 12 documents.

### 11.3 Empirical pin §17.P3 — `strip_failure_response=true` wire-bytes effect (RATIFIED)

**Probe configuration:** Same WebFetch as §17.P1; specific focus on the strip-branch SendLocalReply.

**Verbatim filter.cc evidence:**

> *"When `stripFailureResponse()` is true: `decoder_callbacks_->sendLocalReply(code, \"\", nullptr, absl::nullopt, generateRcDetails(...));`"*

The strip-branch:
- Body = `""` (empty string; 0 bytes).
- Headers-callback = `nullptr` (no header-mutation callback; WWW-Authenticate header NOT added).
- `code` = same 401/403 dispatch as non-strip branch.
- `generateRcDetails` still called (response_code_details preserved even when body stripped).

**Conclusions (pinned) — RATIFIED:**

- (a) `strip_failure_response: true` → body becomes empty string; www-authenticate header NOT set.
- (b) Status code unchanged (401 or 403 per the same dispatch).
- (c) `response_code_details` still emitted (but envoy-go MVP defers field emission entirely per §1.1 amendment 11).
- (d) The 4-header set (lowercase wire-form: `content-length: 0`, `content-type` absent or text/plain, `date`, `server: envoy`) preserves the framework's standard SendLocalReply headers. The strip removes the body + the per-filter WWW-Authenticate header only.
- (e) Phase-17 envoy-go disposition: `cc.stripFailureResponse` branch in `emitDenyResponse` per §6.9; SendLocalReply with `body=""` + no www-authenticate header.

### 11.4 Empirical pin §17.P4 — RemoteJwks async-fetch retry/backoff schedule (REFUTED)

**Probe configuration:** WebFetch of `source/extensions/filters/http/jwt_authn/jwks_async_fetcher.cc`.

**Verbatim jwks_async_fetcher.cc evidence:**

> *"The code does not implement exponential backoff. Instead, it uses a fixed interval: Default failed refetch interval: `constexpr std::chrono::seconds DefaultRefetchAfterFailedSec{1};` — a flat 1-second delay. Configurable via: `async_fetch.failed_refetch_duration()` field in the RemoteJwks config. No exponential backoff, max retries, or base/max interval parameters are defined in this implementation."*

**Conclusions (pinned) — REFUTES BRAINSTORM §10 §17.P4 hypothesis "exponential 1s/2s/4s/8s/16s/30s cap":**

- (a) Envoy implements FIXED-INTERVAL retry; default 1 second; configurable via `JwksAsyncFetch.failed_refetch_duration` proto field.
- (b) NO exponential backoff. NO max-retries cap. NO jitter.
- (c) Per `async_fetch.retry_policy` proto field (RemoteJwks.retry_policy at field 4 in v1.37.x; introduced post-1.32.x) — applies to the INNER HTTP request retries (NOT the outer refetch schedule). Two distinct retry surfaces: outer = fixed-interval failed refetch; inner = HTTP-request RetryPolicy (default 1 retry per proto comment "if num_retries is omitted, the default is to allow only one retry").
- (d) Initial fetch behavior: `fast_listener: true` → non-blocking; `fast_listener: false` (DEFAULT) → blocks listener startup until init completes (success OR failure both unblock).
- (e) Phase-17 envoy-go disposition: `internal/jwks/Fetcher` implements fixed-interval failed-refetch via `time.AfterFunc` (default 1s; configurable via AsyncFetch.FailedRefetchDuration). NO exponential backoff. The RetryPolicy proto field is HONORED in MVP at the inner HTTP-request level (mirrors Envoy's `retry_policy_` member). ADR-0150 codifies.

### 11.5 Empirical pin §17.P5 — JWKS cache TTL + refresh interval defaults (RATIFIED-AND-EXTENDED)

**Probe configuration:** WebFetch of `source/extensions/filters/http/jwt_authn/jwks_async_fetcher.cc` + `source/extensions/filters/http/jwt_authn/jwks_cache.cc`.

**Verbatim evidence:**

> *"Default cache duration: `constexpr std::chrono::seconds DefaultCacheExpirationSec{600};` (10 minutes). Refetch lead time: `constexpr std::chrono::seconds RefetchBeforeExpiredSec{5};` (5 seconds early). Effective refresh interval: `good_refetch_duration_ = cache_duration - 5 seconds`. Code: \"if (good_refetch_duration_ > RefetchBeforeExpiredSec) { good_refetch_duration_ = good_refetch_duration_ - RefetchBeforeExpiredSec; }\""*

**Conclusions (pinned) — RATIFIED-AND-EXTENDED:**

- (a) Default cache duration: **10 minutes (600 seconds)** when `RemoteJwks.cache_duration` is unset.
- (b) Refresh fires `cache_duration - 5 seconds` before TTL expiry (i.e., 9m55s with default 10min cache).
- (c) The `RefetchBeforeExpiredSec=5s` constant is the FIXED refresh-lead time across all cache_duration values; for cache_duration < 5s the refresh fires at cache_duration (no lead).
- (d) PGV constraint: `RemoteJwks.cache_duration` PGV `[1ms, 2500000h)` per `config.pb.validate.go:631-637`.
- (e) Phase-17 envoy-go disposition: `internal/jwks/Fetcher` defaults `cacheDuration` to 10 minutes when unset; refresh schedule = `cacheDuration - 5 seconds` (clamped to `0` if cacheDuration < 5s). ADR-0150 codifies.

### 11.6 Empirical pin §17.P6 — Stat name table + counter dispositions (REFUTES BRAINSTORM hypothesis)

**Probe configuration:** WebFetch of `source/extensions/filters/http/jwt_authn/stats.h`.

**Verbatim stats.h evidence:**

> *"COUNTER(allowed) COUNTER(cors_preflight_bypassed) COUNTER(denied) COUNTER(jwks_fetch_success) COUNTER(jwks_fetch_failed) COUNTER(jwt_cache_hit) COUNTER(jwt_cache_miss)"*

**Conclusions (pinned) — REFUTES BRAINSTORM §1.1 item 7 + §5 hypothesis:**

- (a) **7 base counters** in Envoy v1.37.2: `allowed`, `cors_preflight_bypassed`, `denied`, `jwks_fetch_success`, `jwks_fetch_failed`, `jwt_cache_hit`, `jwt_cache_miss`.
- (b) **NO per-provider scaling.** All counters are FILTER-WIDE at the HCM stat_prefix scope. The BRAINSTORM hypothesis of `jwt_authn.<provider>.jwt_authn_success` per-provider counters is REFUTED.
- (c) **NO `jwks_fetch_in_progress` gauge.** BRAINSTORM hypothesized this gauge; the stats.h macro defines counters ONLY. NO gauges; NO histograms in jwt_authn (counter-only filter).
- (d) **Counter naming** — `cors_preflight_bypassed` (NOT `bypassed_cors_preflight` as BRAINSTORM hypothesized). §1.1 amendment 10 documents.
- (e) `jwt_cache_hit` + `jwt_cache_miss` track the validated-JWT LRU cache (controlled by `jwt_cache_config` proto). Phase-17 MVP defers `jwt_cache_config` entirely (§8 deferral 8); these 2 counters are STRUCTURALLY UNREACHABLE under MVP configs.
- (f) Phase-17 envoy-go disposition: `filterStats` struct carries EXACTLY 7 counters per §6.2; the 5 active under MVP (`allowed`, `denied`, `cors_preflight_bypassed`, `jwks_fetch_success`, `jwks_fetch_failed`) + 2 structurally-unreachable cache counters. Stat-table 64 → 71 names per §1 item 7. ADR-0154 codifies.

### 11.7 Empirical pin §17.P7 — Prometheus tag-extractor + namespace flattening (RATIFIED-PENDING-IMPL-TIME)

**Probe configuration:** Inferred from stats.h + phase-14/15/16 SN2-reuse precedent.

**Conclusions (pinned) — RATIFIED-PENDING-IMPL-TIME:**

- (a) Stat namespace likely follows `<HCM_stat_prefix>.jwt_authn.<counter>` per Envoy's filter-stat-prefix convention (mirrors `<HCM_stat_prefix>.rbac.<rules_stat_prefix>.<counter>` for rbac, simplified for jwt_authn's lack of per-provider stat_prefix surface).
- (b) Phase-17 SPEC's position per §1 item 7 + ADR-0154: SN2-reuse hypothesis (no new SN10 rule); namespace shape `http.<HCM_stat_prefix>.jwt_authn.<counter>` (existing HCM-stat-prefix Prometheus tag-extractor handles verbatim). Mirrors phase-14 + phase-15 + phase-16 SN2-reuse positions.
- (c) **Impl-time empirical scrape against reference Envoy v1.37.2 with a probe yaml confirms the exact shape + amends ADR-0154 accordingly.** Per phase-16 §10 lesson (c), Task-8 is the canonical RATIFIED-PENDING closure point.

### 11.8 Empirical pin §17.P8 — Per-route stat SHARED-vs-INDEPENDENT (RATIFIED)

**Probe configuration:** WebFetch of `source/extensions/filters/http/jwt_authn/filter_config.cc`.

**Verbatim filter_config.cc evidence:**

> *"`No per-route stat prefix exists.` Stats are wired only at the listener level: `stats_(generateStats(stats_prefix, proto_config_.stat_prefix(), context.scope()))`. The `findPerRouteVerifier()` method contains no stats-related logic."*

**Conclusions (pinned) — RATIFIED:**

- (a) NO per-route stat prefix surface in Envoy's jwt_authn filter. The proto `PerRouteConfig` has no `stat_prefix`-equivalent field.
- (b) Per-route emits to the SAME stat namespace as listener-level (allowed/denied/jwks_*/etc. increments aggregate across per-route + listener-level requests under the same HCM stat_prefix).
- (c) Mirrors phase-12 csrf + phase-13 buffer + phase-14 compressor SHARED-stats discipline. DIVERGES from phase-11 + phase-15 + phase-16 INDEPENDENT-stats.
- (d) Rationale: per-route is pure delegation (string-reference resolution against listener-level requirement_map). It does NOT spawn new policy-evaluation state — it merely selects WHICH listener-level requirement applies to this route. SHARED is operationally correct.
- (e) Phase-17 envoy-go disposition: per §5.2 + ADR-0153. factoryState carries ONE filterStats (at listenerRC.stats); per-route resolution does NOT allocate new stats. Eliminates the `sync.Map` lazy-cache + `NewCounterIfAbsent` post-Freeze plumbing that phase-11/15/16 required.

### 11.9 Empirical pin §17.P9 — PGV-required fields per JwtAuthentication + JwtProvider + RequirementRule + PerRouteConfig (PARTIAL/REFRAMED)

**Probe configuration:** Direct read of `config.pb.validate.go` + `config.pb.go` raw descriptor scrape.

**JwtAuthentication PGV findings** (`config.pb.validate.go:2138-2317`):

- `Providers`: line 2170 — `// no validation rules for Providers[key]` (map entries recursive embedded-message validation).
- `Rules`: embedded-message validation only.
- `FilterStateRules`: embedded-message validation only (lines 2239-2266); silent-ignored at runtime per §1.1 amendment 1.
- `BypassCorsPreflight`: line 2267 — `// no validation rules for BypassCorsPreflight`.
- `RequirementMap`: line 2281 — `// no validation rules for RequirementMap[key]`.
- `StripFailureResponse`: line 2315 — `// no validation rules for StripFailureResponse`.

**JwtProvider PGV findings** (`config.pb.validate.go:42-535`):

- `Issuer`: line 61 — `// no validation rules for Issuer` (OPTIONAL).
- `Audiences`: embedded-string validation only.
- `Subjects`: embedded-message validation only (StringMatcher).
- `RequireExpiration`: line 92 — `// no validation rules for RequireExpiration`.
- `MaxLifetime`: embedded-duration validation only.
- `Forward`: line 123 — `// no validation rules for Forward`.
- `PadForwardPayloadHeader`: line 170 — `// no validation rules for PadForwardPayloadHeader`.
- `PayloadInMetadata`: line 172 — `// no validation rules for PayloadInMetadata`.
- `HeaderInMetadata`: line 203 — `// no validation rules for HeaderInMetadata`.
- `FailedStatusInMetadata`: line 205 — `// no validation rules for FailedStatusInMetadata`.
- `ClockSkewSeconds`: line 207 — `// no validation rules for ClockSkewSeconds`.
- `ClearRouteCache`: line 272 — `// no validation rules for ClearRouteCache`.
- `JwksSourceSpecifier` oneof: PARSE-REJECTS if neither `RemoteJwks` nor `LocalJwks` set (per the oneof-required pattern).

**RemoteJwks PGV findings** (`config.pb.validate.go:570-700`):

- `HttpUri`: lines 577-586 — `if m.GetHttpUri() == nil { err := RemoteJwksValidationError{ field: \"HttpUri\", reason: \"value is required\" } }`. **REQUIRED**.
- `CacheDuration`: lines 631-637 — PGV `[1ms, 2500000h)`.
- `AsyncFetch`: embedded-message validation only.
- `RetryPolicy`: embedded-message validation only.

**RequirementRule PGV findings** (`config.pb.validate.go:1780-1830`):

- `Match`: lines 1792-1801 — **REQUIRED** (`value is required`).
- `RequirementType` oneof: oneof not required (proto comment "If not specified, Jwt verification is disabled.").

**PerRouteConfig PGV findings** (`config.pb.validate.go:2410-2487`):

- `RequirementSpecifier` oneof: lines 2472-2481 — **REQUIRED** (`value is required`).
- `Disabled` arm: no value-level PGV (any bool valid).
- `RequirementName` arm: lines 2460-2462 — PGV `min_len=1` (`value length must be at least 1 runes`).

**Conclusions (pinned) — PARTIAL/REFRAMED:**

- (a) Outer JwtAuthentication: NO PGV-required fields; all 6 outer fields optional.
- (b) JwtProvider: NO field-level PGV-required at the proto level; the `JwksSourceSpecifier` oneof is structurally-required (no JWKS source = unworkable).
- (c) RemoteJwks: `HttpUri` REQUIRED; `CacheDuration` PGV `[1ms, 2500000h)`.
- (d) RequirementRule: `Match` REQUIRED; `RequirementType` oneof optional.
- (e) PerRouteConfig: `RequirementSpecifier` oneof REQUIRED; `RequirementName` (when chosen) PGV `min_len=1`.
- (f) Phase-17 envoy-go-side defensive PGV-mirror validation: enforce the above constraints with envoy-go-own error wording per phase-11 ADR-0115 + phase-15 ADR-0136 + phase-16 ADR-0141 precedent. ADR-0149 codifies.
- (g) **NEW finding** — `JwtAuthentication` has 6 top-level fields per `[#next-free-field: 7]`, NOT 5 as BRAINSTORM §1.1 hypothesized. The 6th is `filter_state_rules` (silent-ignored per §1.1 amendment 1).
- (h) **NEW finding** — `JwtProvider` has 21 fields per `[#next-free-field: 22]`, NOT 17 as BRAINSTORM §1.1 hypothesized. The 4 new fields are `subjects` + `require_expiration` + `max_lifetime` (claim-coverage extensions; silent-ignored per §1.1 amendment 3) + (the count drops to 4 net since BRAINSTORM also missed `clock_skew_seconds` per §1.1 amendment 7 which is HONORED in MVP).
- (i) **NEW finding** — `PerRouteConfig.disabled` is `bool` (varint), NOT `Empty` as BRAINSTORM §2.7 + §4 hypothesized. §1.1 amendment 5 documents.

### 11.10 Empirical pin §17.P10 — `claim_to_headers` exact behavior (RATIFIED-AND-EXTENDED)

**Probe configuration:** Direct read of `config.pb.go:1681-1740` (JwtClaimToHeader proto comment) + JwtProvider.claim_to_headers proto comment.

**Verbatim proto evidence:**

JwtProvider.claim_to_headers proto comment (line 287):
> *"Add JWT claim to HTTP Header. Specify the claim name you want to copy in which HTTP header. The claim must be of type; string, int, double, bool. Array type claims are not supported"*

JwtClaimToHeader.claim_name proto comment (line 1690):
> *"The field name for the JWT Claim : it can be a nested claim of type (eg. \"claim.nested.key\", \"sub\"). String separated with \".\" in case of nested claims. The nested claim name must use dot \".\" to separate the JSON name path."*

**Conclusions (pinned) — RATIFIED-AND-EXTENDED:**

- (a) Dot-notation supported for nested claim extraction: `"claim.nested.key"` traverses `payload[claim][nested][key]`.
- (b) **Array claims NOT supported.** Per proto comment line 287 explicit: `"Array type claims are not supported"`. Operator config that wires `claim_to_headers` to an array-valued claim sees the header SKIPPED at runtime (no error; silent skip per Envoy's claim_to_headers tolerance).
- (c) Supported claim types: string, int, double, bool. Non-string types are coerced via JSON-marshal semantics (e.g., bool true → `"true"`, int 42 → `"42"`).
- (d) Phase-17 envoy-go disposition: `internal/jwt.Token.PayloadClaim(path)` returns `interface{}` typed values; the filter's `applySideEffects` coerces scalars to string via `stringify(val)` helper; array-claims return `ErrArrayClaim` and are silently skipped at header emission. §6.9 + ADR-0149 codify.

### 11.11 Empirical pin §17.P11 — `jwt_cache_config` default values (RATIFIED)

**Probe configuration:** Direct read of `config.pb.go:541-588` (`JwtCacheConfig` type + proto comment).

**Verbatim proto comment** (line 547):
> *"The unit is number of JWT tokens, default to 100."*

**Conclusions (pinned) — RATIFIED:**

- (a) `JwtCacheConfig.jwt_cache_size` default = 100 entries (when proto unset OR set to 0).
- (b) `JwtCacheConfig` proto carries no PGV constraints.
- (c) Phase-17 envoy-go disposition: SILENT-IGNORE the entire `jwt_cache_config` field per §8 deferral 8 (couples to future caching-framework phase). The `jwt_cache_hit` + `jwt_cache_miss` counters are STRUCTURALLY UNREACHABLE under MVP. Operator divergence-window: high-RPS deployments with identical-JWT-load see Envoy hit the cache and skip signature verification + claim validation; envoy-go re-validates each request, incurring CPU-cycle cost per repeated JWT.

### 11.12 Empirical pin §17.P12 — Per-route `requirement_name` dangling reference handling (REFUTED + MIRROR DECISION)

**Probe configuration:** WebFetch of `source/extensions/filters/http/jwt_authn/filter_config.cc`.

**Verbatim filter_config.cc evidence:**

> *"Parse-time vs. Runtime Rejection: The code uses runtime rejection. `findPerRouteVerifier()` performs a map lookup:
> ```cpp
> const auto& it = name_verifiers_.find(per_route.config().requirement_name());
> if (it != name_verifiers_.end()) { return std::make_pair(it->second.get(), EMPTY_STRING); }
> return std::make_pair(nullptr, absl::StrCat(\"Wrong requirement_name: ...\"));
> ```
> If the requirement name doesn't exist in `name_verifiers_`, it returns an error string rather than throwing an exception. The filter resolves the name at request time, not during configuration parsing."*

Also from filter.cc per-route error path:
> *"On per-route config error: `decoder_callbacks_->sendLocalReply(Http::Code::Forbidden, config_.get()->stripFailureResponse() ? \"\" : absl::StrCat(\"Failed JWT authentication: \", error_msg), nullptr, absl::nullopt, generateRcDetails(error_msg));`"*

**Conclusions (pinned) — REFUTES BRAINSTORM PARSE-REJECT hypothesis:**

- (a) Envoy RUNTIME-resolves dangling `requirement_name` at request time, NOT parse time.
- (b) Dangling-resolution emit: `SendLocalReply(403, "Failed JWT authentication: Wrong requirement_name: <name>", nullptr)`. Status 403 (NOT 401). NO WWW-Authenticate header. Body wraps the error string.
- (c) Listener-level `requires` vs `requirement_name` resolution at filter_config.cc (CTOR time): the `kRequirementName` arm THROWS `EnvoyException` if the name is not in the listener-level requirement_map. This IS parse-reject at listener-level rules. BUT the per-route `PerRouteConfig.requirement_name` resolution happens via `findPerRouteVerifier()` at REQUEST time, NOT at parse time. The two surfaces have different parse-vs-runtime semantics.
- (d) Phase-17 envoy-go disposition: MIRROR Envoy's split semantic. Listener-level `RequirementRule.requirement_name` is PARSE-REJECTED at `buildCompiledRule` if name not in `requirement_map`. Per-route `PerRouteConfig.requirement_name` is RUNTIME-RESOLVED at `resolveRequirement` (per §6.6 + §6.12); on miss emits `SendLocalReply(403, "Failed JWT authentication: Wrong requirement_name: <name>", {})`. §1.1 amendment 6 documents. ADR-0153 codifies.

### 11.13 Empirical pin §17.P13 — `pad_forward_payload_header` bool semantics (RATIFIED)

**Probe configuration:** Direct read of `config.pb.go:200-207` (JwtProvider.pad_forward_payload_header proto comment).

**Verbatim proto comment** (lines 200-206):
> *"When `forward_payload_header` is specified, the base64 encoded payload will be added to the headers. Normally JWT based64 encode doesn't add padding. If this field is true, the header will be padded."*

**Conclusions (pinned) — RATIFIED:**

- (a) `pad_forward_payload_header: false` (DEFAULT): base64url encoding strips trailing `=` padding (the canonical RFC 7519 §3 JWT format). The forward-payload-header value carries the unpadded base64url.
- (b) `pad_forward_payload_header: true`: base64url encoding preserves trailing `=` padding (per standard base64 encoding RFC 4648).
- (c) Phase-17 envoy-go disposition: `internal/jwt.Token.RawPayload` carries the original base64url-encoded payload (unpadded, per JWT canonical form). The filter's `applySideEffects` re-pads if `pad_forward_payload_header == true` (appends `=` characters until length is multiple of 4). §6.9 + ADR-0149 codify.

### 11.14 Empirical pin §17.P14 — `from_params` resolver (RATIFIED-AND-REFINED)

**Probe configuration:** WebFetch of `source/extensions/filters/http/jwt_authn/extractor.cc`.

**Verbatim extractor.cc evidence:**

> *"Case-sensitivity: Parameters are case-sensitive. The code uses `params.getFirstValue(param_key)` with exact string matching against `param_locations_` map keys.
> URL Decoding: Enabled via `QueryParamsMulti::parseAndDecodeQueryString(path)` - the parse function explicitly decodes.
> Array Handling: Only first value extracted: `const auto& it = params.getFirstValue(param_key)` returns single optional value, not all matching params."*

**Conclusions (pinned) — RATIFIED-AND-REFINED:**

- (a) Parameter name matching: case-sensitive exact match (e.g., `from_params: ["access_token"]` matches `?access_token=foo` but NOT `?Access_Token=foo`).
- (b) URL decoding: ENABLED via `parseAndDecodeQueryString` — query parameter values are URL-decoded BEFORE being used as the raw JWT.
- (c) Multi-value query handling: Envoy extracts ONLY the FIRST value (`getFirstValue`). For `?token=a&token=b`, only `a` is treated as the JWT. **REFINES** BRAINSTORM §17.P14 hypothesis of "array-param handling unclear" — Envoy's behavior is documented as first-value-only.
- (d) Phase-17 envoy-go disposition: `parseQueryParam(path, name)` returns the slice of values for the name (case-sensitive lookup); the filter's `extractTokens` uses only `vals[0]` (first value). URL-decode happens at parse-time via Go's `url.ParseQuery` (stdlib). ADR-0152 codifies.

### 11.15 Empirical pin §17.P15 — `from_cookies` resolver (RATIFIED)

**Probe configuration:** Same extractor.cc WebFetch as §17.P14.

**Verbatim extractor.cc evidence:**

> *"Cookie Name Matching & Parsing: Case-sensitivity: Exact case match. Uses `cookie_locations_.contains(k)` predicate during parsing, then `cookies.find(cookie_key)` for lookup. Parsing Rules: Delegates to `Http::Utility::parseCookies()` with callback filter. Returns map of name→value pairs; extracts the value directly."*

**Conclusions (pinned) — RATIFIED:**

- (a) Cookie name matching: case-sensitive exact match (per RFC 6265 §4.1.1 — cookie names ARE case-sensitive).
- (b) Cookie value parsing: Envoy's `Http::Utility::parseCookies()` returns the value verbatim (NO URL-decode; per RFC 6265 cookie values are raw bytes).
- (c) Phase-17 envoy-go disposition: `parseCookies(cookieHeader)` returns `map[string]string` of name→value pairs (Go stdlib `net/http.Request.Cookies()` semantic; case-sensitive name keys). Filter's `extractTokens` uses cookies[name] directly. ADR-0152 codifies.

### 11.16 Empirical pin §17.P16 — `allow_missing` + `allow_missing_or_failed` exact dispositions (RATIFIED-AND-EXTENDED)

**Probe configuration:** WebFetch of `source/extensions/filters/http/jwt_authn/authenticator.cc`.

**Verbatim authenticator.cc evidence:**

> *"allow_failed disposition: 'Unless allowing failed or missing, all tokens must be verified successfully.' (line ~280). When `is_allow_failed_` is true, the authenticator calls `callback_(Status::Ok)` even after non-Ok status, masking verification failures.
> allow_missing disposition: 'is_allow_missing_ && status == Status::JwtMissed' returns Ok (line ~289). Missing tokens pass only if `is_allow_missing_` is set."*

**Conclusions (pinned) — RATIFIED-AND-EXTENDED BRAINSTORM §17.P16 hypothesis:**

- (a) `allow_missing_or_failed`: ANY verification outcome treated as OK. JWT absent → OK; JWT present-and-invalid → OK. Useful for validate-and-forward-for-downstream patterns where downstream consumers make the auth decision.
- (b) `allow_missing`: JWT absent (`Status::JwtMissed`) → OK; JWT present-and-invalid → FAIL (propagates the canonical failure-reason). The distinguishing feature vs `allow_missing_or_failed`: `allow_missing` is STRICT on present-but-invalid tokens.
- (c) Within `requires_any` / `requires_all` recursive combinators: the 6-variant evaluator delegates to sub-requirements recursively. Per Envoy's verifier.cc, `requires_any` returns the FIRST successful evaluation's status; if all fail, returns the LAST failure status. `requires_all` short-circuits on first failure.
- (d) Phase-17 envoy-go disposition: per §6.8 evaluator code. The `allow_missing` case iterates extraction across all providers (looking for any token); if any token is extracted, validation MUST succeed (else fail-with-status); else missing-OK. The `allow_missing_or_failed` case always returns allowed. ADR-0149 codifies.

### 11.17 Summary

All 16 pins resolved IN-SESSION. 12 §1.1 amendments authored covering the empirical refinements + structural discoveries (mirrors phase-16 rbac's 12-amendment volume). The structural design (TWO new framework primitives, Both-JWKS proto-faithful, Full-6 JwtRequirement, RS+ES algorithm allow-list, All-4 extraction sources, Full-header-side side-effects, 8th canonical per-route + SHARED-stats discipline, 401-WWW-Authenticate deny-path) survives intact despite the magnitude of the refinements — all amendments fit within the §1.1 self-contained-prose-block channel without requiring a BRAINSTORM §12 amendment cycle. Phase-16 §10 lesson (c) Task-8 RATIFIED-PENDING closure mechanism applies to §17.P7 only (Prometheus tag-extractor namespace).

---

## 12. Deferred decisions (the planner / implementer settles these)

The following decisions are explicit IMPL-time discretion. Each is bounded — none affects the BEHAVIOR_CONTRACT bundle landing at phase-done.

### 12.1 `internal/jwks` package location vs alternative `internal/httpclient`

SPEC author's call per §3.1: LOCK to `internal/jwks/` (JWKS-specific package). The alternative of a generic `internal/httpclient/` is rejected at SPEC time — the JWKS sub-domain has enough framework-primitive-specific concerns (JWK Set parsing, kid+alg lookup, refresh-5s-before-TTL schedule) that a JWKS-specific package is structurally cleaner. Future outbound-HTTP-needing filters (ext_authz, oauth2) MAY introduce a sibling `internal/httpclient/` (extract the request-shape + retry-policy plumbing) OR compose directly against the JWKS package's lower-level primitives. ADR-0150 §Decision documents.

### 12.2 `internal/jwt` package error sentinel completeness

SPEC author's call per §3.2: LOCK initial error sentinel set at ~20 most-commonly-hit entries (per §11.P1 table). Future impl-time additions are non-blocking; the cross-phase reusability is preserved as long as the `Parse` + `VerifySignature` + `ValidateClaims` + `PayloadClaim` API surface is stable.

### 12.3 RemoteJwks initial-fetch failure behavior

SPEC author's call per §6.10: when `fast_listener: false` (default) AND initial fetch FAILS, `jwks.New()` returns an error, which causes the entire `JwtAuthentication` config to fail listener-load. This DIVERGES from Envoy's behavior — Envoy's `init_target_->ready()` is called on failure too, allowing the listener to activate (with the JWKS endpoint marked as failed; on-demand fetches retry per the failed-refetch schedule). Phase-17 envoy-go MVP's "fail-loud-at-listener-load" approach is operationally cleaner for envoy-go's static-config-at-boot world but DIVERGES from Envoy's xDS-served deferred-listener-activation pattern.

Impl-time alternative: mirror Envoy's "activate-listener-anyway-after-init-target-ready-with-error". Documented at ADR-0150 §Decision (iii); impl-time may flip to the Envoy-mirroring behavior if integration testing reveals operator surprise.

### 12.4 `allow_missing` iteration discipline across providers

SPEC author's call per §6.8: when `allow_missing` requirement fires AND the request carries a token, the evaluator iterates across ALL providers (any token extracted via any provider's extraction-source set triggers validation against THAT provider). The phase-17 MVP simplification treats this as a requires_any-style across providers — first-success wins; if any provider validates the token, allow; if all providers fail to validate, deny with the last failure's status.

Impl-time may refine this discipline per a Task-N empirical scrape of authenticator.cc's `allow_missing` iteration logic; ADR-0149 §Decision documents.

### 12.5 Per-route runtime-resolve error wire shape

SPEC author's call per §1.1 amendment 6: on per-route `requirement_name` runtime-resolve failure, emit `SendLocalReply(403, "Failed JWT authentication: Wrong requirement_name: <name>", {})`. Status 403 (not 401); body wraps error message; NO WWW-Authenticate header.

Impl-time alternative: emit 500 (internal server error) — the per-route config IS technically invalid (operator error). Phase-17 MVP MIRRORS Envoy's 403 verbatim. ADR-0153 codifies; ADR-0155 cross-references.

### 12.6 RS-vs-PS algorithm family inclusion scope

SPEC author's call per §1 item 3: RS family ONLY (RSASSA-PKCS1-v1_5 with SHA-256/384/512). The PS family (RSASSA-PSS with SHA-256/384/512) is NOT explicitly mentioned in BRAINSTORM Q3 algorithm subset. Per jwt_verify_lib status table (e.g., `JwksRSAKeyBadAlg = "[alg] is not started with [RS] or [PS]..."`), Envoy v1.37.2 DOES support PS family. Phase-17 MVP scope per BRAINSTORM Q3 = "RS + ES family (six algorithms)" — interpreted strictly as RS256/384/512 + ES256/384/512 (6 algorithms total). PS family DEFERRED via §8 deferral 5 extension (HS family + EdDSA + PS family co-deferred to future algorithm-extension phase).

Impl-time may add PS family if Task-N reveals fixture-needed coverage; ADR-0151 §Decision documents.

### 12.7 JWT validation order — extraction vs evaluation precedence

SPEC author's call per §6.8: when listener-level rules has multiple `rules[]` entries with different `requires` requirements, the FIRST-MATCHING rule's requirement applies. Within the requirement (if `requires_any`), iteration is OR-with-short-circuit.

Impl-time may discover ambiguity if a request matches multiple rules' RouteMatch predicates (operator-config-driven ambiguity); per phase-04 router precedent, first-match wins.

### 12.8 `forward = false` stripping discipline for default Authorization Bearer

SPEC author's call per §6.9: when `forward = false` (proto default) AND the token was extracted from the default Authorization Bearer (not configured from_headers), strip the entire `Authorization` header (not just the `Bearer <token>` value). This mirrors Envoy's behavior per JwtProvider.forward proto comment ("the JWT is removed in the request").

Impl-time may refine — Envoy MAY preserve `Authorization` header presence with non-Bearer parts intact (e.g., dual-auth schemes where the same header carries multiple credentials). Phase-17 MVP strips the entire header for simplicity; ADR-0149 §Decision documents.

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052, lands at phase-done commit)

The phase-17 phase-done commit lands a 6-edit bundle into `BEHAVIOR_CONTRACT.md` per `BOOTSTRAP_PROMPT.md` §7.5 Gate F. The bundle SHAPE locks here; the bundle CONTENTS land at IMPL phase-done commit.

### 13.1 `## HTTP filter chain` → NEW `### envoy.filters.http.jwt_authn` subsection (insertion alphabetical-after-header_mutation per ADR-0100 §2.2)

Under the existing `## HTTP filter chain` umbrella (currently carrying 10 filter subsections: bandwidth_limit, buffer, compressor, cors, csrf, envoy_go_test, fault, header_mutation, local_ratelimit, rbac, router): a NEW `### envoy.filters.http.jwt_authn` subsection covering:
- 5-consumed-listener-field map (providers + rules + bypass_cors_preflight + requirement_map + strip_failure_response) + 1-silent-ignored (filter_state_rules).
- 13-consumed-JwtProvider-field map (per §1 item 3 enumeration) + 8-silent-ignored.
- 6-variant JwtRequirement evaluator semantics (provider_name + provider_and_audiences + requires_any + requires_all + allow_missing + allow_missing_or_failed).
- RS+ES algorithm allow-list (6 algorithms).
- All-4-extraction-source set + iteration order + first-match-wins + default Authorization Bearer + access_token query param.
- Full-header-side side-effect set + emit-order (strip → forward_payload_header → claim_to_headers → clear_route_cache).
- 8th-canonical-per-route (string-reference-delegation) semantics + SHARED-stats discipline.
- 401-WWW-Authenticate deny-path SendLocalReply wire shape (401 default, 403 for JwtAudienceNotAllowed; body = canonical jwt_verify_lib string; WWW-Authenticate Bearer challenge per RFC 6750).
- `strip_failure_response` divergence-window (strip strips both body AND www-authenticate).
- `response_code_details` envoy-go MVP defer + future-pickup notes.
- `filter_state_rules` silent-ignored divergence-window.
- `subjects` + `require_expiration` + `max_lifetime` v1.37.x silent-ignored divergence-window.
- dynamic-metadata family silent-ignored divergence-window.
- `jwt_cache_config` no-cache divergence-window.

Anticipated subsection size: ~200-350 lines (mirrors phase-16 rbac subsection).

### 13.2 `## Stat-name mapping` → 64 → 71-name table extension

The existing stat-name mapping table (currently 64 names post-phase-16) extends by 7 entries:

```
| `http.<conn_mgr>.jwt_authn.allowed`                    | counter | `envoy_http_conn_manager_prefix` (SN2) | jwt_authn auth-success counter per HCM stat_prefix |
| `http.<conn_mgr>.jwt_authn.denied`                     | counter | `envoy_http_conn_manager_prefix` (SN2) | jwt_authn auth-failure counter |
| `http.<conn_mgr>.jwt_authn.cors_preflight_bypassed`    | counter | `envoy_http_conn_manager_prefix` (SN2) | OPTIONS preflight bypassed via bypass_cors_preflight |
| `http.<conn_mgr>.jwt_authn.jwks_fetch_success`         | counter | `envoy_http_conn_manager_prefix` (SN2) | Successful JWKS fetches across all RemoteJwks providers |
| `http.<conn_mgr>.jwt_authn.jwks_fetch_failed`          | counter | `envoy_http_conn_manager_prefix` (SN2) | Failed JWKS fetches |
| `http.<conn_mgr>.jwt_authn.jwt_cache_hit`              | counter | `envoy_http_conn_manager_prefix` (SN2) | (UNREACHABLE under MVP; jwt_cache_config deferred per §8 deferral 8) |
| `http.<conn_mgr>.jwt_authn.jwt_cache_miss`             | counter | `envoy_http_conn_manager_prefix` (SN2) | (UNREACHABLE under MVP) |
```

Total stat-table: 71 names; 5 actively-emitted under MVP; 2 structurally-unreachable.

### 13.3 `## Equivalence Matrix` → NEW row pointing at fixture 0019

A new row in the equivalence matrix:

```
| 0019-http-jwt-authn | byte-exact body on allow paths AND deny paths; status byte-exact (401/403 dispatch per §17.P1); www-authenticate header byte-exact including conditional ", error=\"invalid_token\"" append per §17.P2; 5 active counter delta equivalence (allowed/denied/cors_preflight_bypassed/jwks_fetch_success/jwks_fetch_failed); 2 unreachable counters not asserted; response_code_details NOT asserted (envoy-go MVP defers; per §1.1 amendment 11) |
```

### 13.4 NEW `### Phase 17 forward-pointer notes` subsection (under `## Forward-pointer notes`)

A new `### Phase 17 forward-pointer notes` subsection enumerates the 17 deferred items (§8 above) + 1 foot-gun, organized by deferred-cluster:

- **dynamic-metadata family** (items 1-4): payload_in_metadata + header_in_metadata + failed_status_in_metadata + normalize_payload_in_metadata.
- **algorithm-extension family** (items 5-7): HS family + EdDSA + `none` (the latter PERMANENT-DEFERRED).
- **caching-framework family** (items 8 + 11): jwt_cache_config + JWKS cache-invalidation hooks.
- **claim-coverage-extension family** (items 15-17): subjects + require_expiration + max_lifetime.
- **filter-state-family** (item 12): filter_state_rules.
- **response-code-details family** (item 13): joint pickup with phase-16 rbac item 8.
- **access-log family** (item 14): %JWT_PROVIDER% + %JWT_SUBJECT% + %JWT_FAILURE_REASON%.
- **CEL family** (item 10 renumbered): CEL-based dynamic provider selection.
- **operator-ergonomics family** (item 10 prev / item 18): exponential-backoff customization + clear_route_cache implicit-on-side-effect trigger.
- **foot-gun**: JwtRequirement Set recursion-depth (no parse-time cap; mirrors Envoy permissive disposition).

### 13.5 `## HTTPFilterCallbacks` — NO extensions

Phase-17 introduces NO new HTTPFilterCallbacks methods. The existing `cb.SendLocalReply(status, body, headers)` + `cb.ClearRouteCache()` + `cb.RequestRouteConfig()` + (optionally) `cb.Context()` cover jwt_authn's needs. The `internal/jwks` + `internal/jwt` framework primitives live OUTSIDE the HTTPFilterCallbacks surface (mirrors phase-16's `internal/matcher` package — cross-cutting primitives go in dedicated top-level packages, NOT on the filter-callback interface).

### 13.6 `## Per-route canonical patterns` — ADR-0125 §(xiii) cross-reference

A reference under the existing ADR-0125 canonical-pattern roster section (if present in BEHAVIOR_CONTRACT) noting that ADR-0125 §(xiii) lands the 8th canonical pattern at the phase-17 SPEC commit. The actual amendment paragraph text lives in DECISIONS.md ADR-0125 §(xiii); BEHAVIOR_CONTRACT cross-references.

### 13.7 NEW `## JWKS framework primitive` top-level section (per ADR-0150)

A new top-level section (`## JWKS framework primitive`) under the operator-facing portion of BEHAVIOR_CONTRACT.md (alongside the existing `## HTTPFilterCallbacks` + `## Matcher engine framework primitive` umbrellas added in prior phases). Covers:
- Package shape: `internal/jwks/` (location + exported API surface).
- `Fetcher` lifecycle: New (blocking or non-blocking init per fast_listener) → Get (returns cached JWK Set or ErrJwksNotReady) → Close.
- Refresh schedule: 5s before TTL expiry via `time.AfterFunc`; default 10-minute cache duration.
- Failed-refetch schedule: fixed 1s interval (configurable); NO exponential backoff.
- Cross-phase reuse intent: future ext_authz HTTP-mode + oauth2 token-endpoint flows compose against this primitive.
- Operator-facing semantics: how to wire up RemoteJwks endpoints + how to interpret jwks_fetch_success/failed counters + how to handle initial-fetch failures.

### 13.8 NEW `## JWT verifier framework primitive` top-level section (per ADR-0151)

A new top-level section (`## JWT verifier framework primitive`). Covers:
- Package shape: `internal/jwt/` (location + exported API surface).
- `Token` lifecycle: Parse → VerifySignature → ValidateClaims → PayloadClaim.
- Algorithm allow-list: RS256/384/512 + ES256/384/512 (6 algorithms); HS family + EdDSA + `none` + PS family DEFERRED.
- Claim validation order: exp + nbf + iss + aud (with clock-skew tolerance).
- Cross-phase reuse intent: future filters consuming JWT semantics (jwt_claim_router, oauth2 token validation) compose against this primitive.
- ~20 canonical error sentinels mirroring jwt_verify_lib status codes.

---

## 14. Testing strategy

### 14.1 Unit tests (test groups in `jwtauthn_test.go`)

Anticipated test groups (~1500-2500 LoC; phase-17 SPEC author's call locked the upper estimate per §1 item 1 + the 12-amendment empirical-pin refinement scope):

- **Group 1 — Config parsing.** Each of the 12 §1.1 amendments has a dedicated test fixture validating its empirical claim:
  - JwtAuthentication 6-field outer-envelope (filter_state_rules silent-ignored at parse).
  - JwtProvider 21-field surface (subjects + require_expiration + max_lifetime silent-ignored).
  - PerRouteConfig.disabled bool form (NOT Empty).
  - clock_skew_seconds default 60s.
  - 7-counter filterStats; cors_preflight_bypassed naming.
  - PGV-mirror validation per §11.P9 enumerated constraints.
- **Group 2 — Token extraction.** Per §6.7 + §11.P14 + §11.P15:
  - Default Authorization Bearer (when no explicit extraction-sources).
  - Default access_token query param.
  - Configured from_headers with value_prefix substring-search.
  - Configured from_params case-sensitive + URL-decode + first-value-only.
  - Configured from_cookies case-sensitive + verbatim value.
  - Iteration order (headers → params → cookies) + first-match-wins.
- **Group 3 — JWT parse + signature verify.** Per `internal/jwt` API:
  - Parse 3-part JWT structure + reject malformed (`JwtBadFormat`).
  - Base64url-decode header + payload + signature.
  - VerifySignature with RS256/384/512 + ES256/384/512 (positive cases via fixture-PKI keypairs).
  - VerifySignature rejection on tampered signature (`JwtVerificationFail`).
  - VerifySignature rejection on alg outside allow-list (`JwtHeaderNotImplementedAlg`).
  - PayloadClaim dot-notation extraction (nested claims).
  - PayloadClaim array-claim rejection (`ErrArrayClaim`).
- **Group 4 — Claim validation.** Per §6.8 + §11.P16:
  - exp + nbf + clock_skew_seconds tolerance (±60s default).
  - iss exact match (when provider.issuer non-empty).
  - aud OR-semantic intersection (when provider.audiences non-empty).
  - exp in past → `JwtExpired` (with clock-skew tolerance applied).
  - nbf in future → `JwtNotYetValid`.
  - iss mismatch → `JwtUnknownIssuer`.
  - aud mismatch → `JwtAudienceNotAllowed` (note: this status maps to HTTP 403, not 401, per §1.1 amendment 8).
- **Group 5 — Requirement evaluator.** Per §6.8 + §11.P16:
  - provider_name simple case.
  - provider_and_audiences with per-rule audience override.
  - requires_any with short-circuit OR.
  - requires_all with short-circuit AND.
  - allow_missing: JWT absent → OK; JWT present-and-invalid → FAIL.
  - allow_missing_or_failed: any outcome → OK.
  - Recursive combinators (requires_any inside requires_all + vice versa; ~3 levels deep).
- **Group 6 — Side-effect emit-order.** Per §6.9:
  - strip-on-success (forward=false) → Authorization header removed.
  - forward_payload_header → base64url-encoded payload emitted (with + without pad).
  - claim_to_headers → string + numeric + bool claims emitted; array claims silently skipped.
  - clear_route_cache → cb.ClearRouteCache() invoked.
- **Group 7 — Per-route (8th canonical).** Per §5 + §6.12:
  - `PerRouteConfig{disabled: true}` → wholesale passthrough; NO counters.
  - `PerRouteConfig{disabled: false}` → falls through to listener-level rules dispatch.
  - `PerRouteConfig{requirement_name: "<valid-name>"}` → requirement_map resolved; named requirement applied.
  - `PerRouteConfig{requirement_name: "<dangling-name>"}` → runtime-resolve fail; 403 + "Failed JWT authentication: Wrong requirement_name: <name>" + denied counter increment.
- **Group 8 — Deny-path wire shape.** Per §4 + §1.1 amendments 8 + 11 + 12:
  - 401 for JwtMissed (body byte-exact `"Jwt is missing"`; WWW-Authenticate `Bearer realm="<original_uri>"` WITHOUT error param).
  - 401 for JwtExpired (body byte-exact `"Jwt is expired"`; WWW-Authenticate with `, error="invalid_token"`).
  - 403 for JwtAudienceNotAllowed (body byte-exact `"Audiences in Jwt are not allowed"`; same WWW-Authenticate format).
  - `strip_failure_response: true` → body empty; NO www-authenticate header.
  - Per-route runtime-resolve fail → 403 + "Failed JWT authentication: Wrong requirement_name: ...".
- **Group 9 — CORS preflight bypass.** Per §11.P1:
  - OPTIONS + Origin + Access-Control-Request-Method → 200 passthrough + cors_preflight_bypassed counter.
  - OPTIONS without Origin → NOT bypassed (falls through to JWT validation).
- **Group 10 — RemoteJwks lifecycle** (`internal/jwks` integration). Per §6.10 + §11.P4 + §11.P5:
  - Initial fetch blocking (fast_listener=false) → New() returns after fetch completes.
  - Initial fetch non-blocking (fast_listener=true) → New() returns immediately; first call to Get returns ErrJwksNotReady.
  - Successful refresh fires 5s before TTL.
  - Failed fetch retry at 1s fixed interval.
- **Group 11 — `compiledPerRoute` lazy-cache** (sync.Map LoadOrStore semantics):
  - Identity-keyed by `*PerRouteConfig` proto pointer.
  - Concurrent resolvePerRouteConfig calls produce ONE compiledPerRoute per proto pointer.

### 14.2 Race detector + lint

All test groups run under `go test -race`. The factoryState `sync.Map` is the primary concurrency point; `internal/jwks/Fetcher` has its own `sync.Mutex` guarding cache state. Race tests must cover concurrent extraction + validation across goroutines per stream + concurrent JWKS refresh on background goroutine vs request-path Get.

Lint: `go vet ./...` clean; `staticcheck ./...` clean per phase-09..16 precedent.

### 14.3 Fuzzers (21st fuzzer `FuzzJwtAuthnConfigParse`)

NEW fuzzer at `internal/filter/http/jwtauthn/fuzz_test.go`. Corpus seeds 0-12 anticipated (mirrors phase-16's fuzzer corpus size):
- Seed 0: minimal valid JwtAuthentication (one provider with LocalJwks + one rule).
- Seed 1: provider with all 13 consumed fields populated.
- Seed 2: provider with all 8 silent-ignored fields populated (parser must accept-and-ignore).
- Seed 3: provider with RemoteJwks + async_fetch + retry_policy.
- Seed 4: provider with all 4 extraction sources.
- Seed 5: provider with claim_to_headers (nested + dot-notation).
- Seed 6: rule with inline `requires` (deprecated arm honored per §1.1 amendment 4).
- Seed 7: rule with requirement_name → requirement_map resolution.
- Seed 8: requirement_map with all 6 JwtRequirement variants (including recursive requires_any/all).
- Seed 9: filter_state_rules set (silent-ignored).
- Seed 10: bypass_cors_preflight + strip_failure_response both true.
- Seed 11: per-route TPFC with disabled: true.
- Seed 12: per-route TPFC with requirement_name (valid name).

30s/seed @ 13-seed corpus = ~390s wallclock; under the existing fuzzer-time-budget envelope.

### 14.4 Existing fuzzers re-run

The 20 existing fuzzers (post-phase-16) re-run at 30s each. Expected: all green. Phase-17 introduces no proto-decode-path divergence beyond the new jwtauthn parser.

### 14.5 h2spec re-run (53/53 PASS unchanged through this SPEC commit)

Phase 17 introduces no H2 wire-shape changes; h2spec gate 53/53 PASS at the ADR-0051 pin unchanged. Confirmed at phase-done Gate C.

### 14.6 Differential 0000-0018 + 0019

The 19 differential fixtures (0000-0018) re-run on phase-17 phase-done. Expected: 19/19 green; no observable wire-shape change from phase-16. NEW fixture 0019-http-jwt-authn lands per §7; 8 scenarios. Total 20 fixtures at phase-done.

### 14.7 Six-gate checklist (A/B/C/D/E/F per BOOTSTRAP_PROMPT.md §7.5)

- **Gate A** (build + vet + lint): all green; new `jwtauthn` + `internal/jwks` + `internal/jwt` packages compile clean.
- **Gate B** (race tests): all green; `go test -race ./internal/filter/http/jwtauthn/... ./internal/jwks/... ./internal/jwt/...` + repo-wide race clean.
- **Gate C** (h2spec): 53/53 PASS at ADR-0051 pin.
- **Gate D** (fuzzers): 21 fuzzers green at 30s each; ~10.5 minutes wallclock for the full suite.
- **Gate E** (differential): 20/20 fixtures green (0000-0019).
- **Gate F** (BEHAVIOR_CONTRACT): 6-edit bundle (§13.1-§13.8) landed; tools/check_behavior_contract.sh (or analog) green.

---

## 15. Acceptance checklist (for the reviewer)

The phase-17 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following claims against the landed artefacts:

1. **Package shape per ADR-0148:** `internal/filter/http/jwtauthn/{jwtauthn.go, evaluator.go, provider.go, jwtauthn_test.go, fuzz_test.go, doc.go}` with `Decoder: f, Encoder: nil` (decoder-only per §1 item 5; mirrors csrf + buffer + rbac precedent); 7-base-counter `filterStats` registered (allowed/denied/cors_preflight_bypassed/jwks_fetch_success/jwks_fetch_failed/jwt_cache_hit/jwt_cache_miss per §1.1 amendments 9 + 10); unconditional counter allocation at New() time (NO lazy-allocation for filter-wide counters).

2. **Field decomposition per ADR-0149 + §1.1 amendments 1-4 + 7:** 5 outer-envelope fields consumed proto-faithful + 1 silent-ignored (filter_state_rules); 13 JwtProvider fields consumed (including clock_skew_seconds per amendment 7) + 8 silent-ignored (4 dynamic-metadata family + jwt_cache_config + 3 v1.37.x claim-coverage extensions); `RequirementRule.requires` honored proto-faithful (NOT parse-rejected per amendment 4); 6 JwtRequirement variants all honored; envoy-go-side defensive PGV-mirror validation with envoy-go-own error wording per §11.P9.

3. **Framework primitives per ADR-0150 + ADR-0151:** (i) `internal/jwks/` NEW top-level package with Fetcher API + async refresh + per-thread cache + 5s-before-TTL refresh schedule + fixed-1s failed-refetch retry (NO exponential backoff per §11.P4 REFUTED); (ii) `internal/jwt/` NEW top-level package with Parse + VerifySignature + ValidateClaims + PayloadClaim API + ~20 canonical error sentinels mirroring jwt_verify_lib + RS+ES algorithm allow-list. Phase 17 is the SECOND CONSECUTIVE §9 row to introduce TWO framework primitives in a single phase (after phase-16's matcher + TLS-principal).

4. **Token extraction per ADR-0152 + §11.P14 + §11.P15:** All 4 sources implemented (default Authorization Bearer + access_token query param + configured from_headers + configured from_params + configured from_cookies); case-sensitive + URL-decoded for params; case-sensitive + verbatim for cookies; first-value-only for multi-value query params; iteration order headers → params → cookies; first-match-wins.

5. **Per-route discipline per §5 + ADR-0153 + ADR-0125 §(xiii) amendment:** phase-17 is the FIRST row to use the **8th canonical per-route pattern** (oneof{disabled(bool) | requirement_name(string)} with string-reference-delegation via listener-level requirement_map; SHARED-stats discipline); structurally distinct from 5th-7th canonicals (per §5.4 amendment §(xiii) distinction matrix); PerRouteConfig.disabled is `bool` not `Empty` per §1.1 amendment 5; per-route `requirement_name` dangling reference is RUNTIME-RESOLVED with 403 + error string per §1.1 amendment 6 (mirrors Envoy filter_config.cc); ADR-0125 in-place §(xiii) amendment paragraph lands at SPEC time per phase-13/14/15/16 precedent; ADR-0125's canonical-pattern roster grows from 7 to 8.

6. **Stat surface per ADR-0154 + §1.1 amendments 9 + 10:** 7 base counters (allowed/denied/cors_preflight_bypassed/jwks_fetch_success/jwks_fetch_failed/jwt_cache_hit/jwt_cache_miss) per HCM stat_prefix; NO per-provider scaling (REFUTES BRAINSTORM hypothesis per §11.P6); NO gauges; NO histograms; namespace `http.<HCM_stat_prefix>.jwt_authn.<counter>` (SN2-reuse hypothesis RATIFIED-PENDING-IMPL-TIME per §11.P7); SHARED per-route stats discipline (per §5.2 + §11.P8 RATIFIED); stat-table 64 → 71 names (7 new active counters; 2 structurally-unreachable under MVP per §8 deferral 8).

7. **Wire-shape claim per ADR-0155 + §11.P1-§11.P3 + §1.1 amendments 8 + 11 + 12:** byte-exact response (status + body + 4-header set + conditional WWW-Authenticate) on allow paths AND deny paths; deny status 401 for most failure-reasons + 403 for JwtAudienceNotAllowed (per §1.1 amendment 8); body = canonical jwt_verify_lib `getStatusString(status)` (~70 strings; ~5 hit in fixture 0019); WWW-Authenticate `Bearer realm="<original_uri>"` + conditional `, error="invalid_token"` for non-JwtMissed (per §1.1 amendment 12); `strip_failure_response: true` strips both body AND WWW-Authenticate (per §11.P3); `response_code_details = "jwt_authn_access_denied{<reason>}"` divergence-window documented (envoy-go MVP defers field emission per §1.1 amendment 11 + §8 deferral 13); per-route runtime-resolve error case emits 403 + "Failed JWT authentication: Wrong requirement_name: <name>" with NO WWW-Authenticate header (per §1.1 amendment 6).

8. **§11 empirical pin block:** all 16 pins resolved IN-SESSION per ADR-0004; disposition tally captured at §11 summary table (6 RATIFIED-AND-EXTENDED + 2 RATIFIED + 1 RATIFIED-AND-REFINED + 1 RATIFIED-PENDING-IMPL-TIME + 3 REFUTED + 2 PARTIAL + 1 REFUTED-WITH-MIRROR-DECISION); verbatim filter.cc + authenticator.cc + jwks_async_fetcher.cc + jwks_cache.cc + stats.h + filter_config.cc + extractor.cc scrape evidence + jwt_verify_lib `getStatusString()` table; **12 §1.1 amendments** authored covering the empirical refinements + structural discoveries.

9. **Differential fixture per §7:** 8 scenarios; byte-exact body assertion (allow paths backend-echo + deny paths canonical jwt_verify_lib strings); per-counter delta byte-equivalence on the 5 active base counters; per-route 8th-canonical exercised on both arms (scenarios 7 + 8); RemoteJwks fetch lifecycle exercised on scenarios 1 + 7; LocalJwks path exercised on scenario 2; cors-bypass exercised on scenario 6.

10. **BEHAVIOR_CONTRACT.md populated** per Gate F:
    - §13.1 new `### envoy.filters.http.jwt_authn` subsection (~200-350 lines incorporating field-decomposition + 6-variant evaluator + RS+ES allow-list + 4-extraction-sources + Full-header-side-effects + 8th-canonical-per-route + SHARED-stats discipline + wire-shape).
    - §13.2 stat-table 64 → 71 names extension (7 new active counters; 2 structurally-unreachable noted).
    - §13.3 NEW equivalence-matrix row pointing at fixture 0019 with byte-exact body + status + www-authenticate discipline.
    - §13.4 NEW `### Phase 17 forward-pointer notes` subsection covering the 17-deferral + 1-foot-gun list.
    - §13.7 NEW `## JWKS framework primitive` top-level section per ADR-0150.
    - §13.8 NEW `## JWT verifier framework primitive` top-level section per ADR-0151.

11. **DECISIONS.md populated** per ADR-on-impl convention: ADR-0148..ADR-0155 §Context drafts anchored at SPEC commit; §Decision + §Consequences bodies LAND at each ADR's Lands-in-Task per ADR-0044. ADR-0125 §(xiii) amendment paragraph LANDS IN FULL at SPEC commit per phase-13/14/15/16 in-place-amend-at-SPEC precedent.

12. **ROADMAP.md row 17 summary refinement** (optional): SPEC author may revise the in-progress row 17 summary to reflect §11-pin RATIFICATION/REFUTATION dispositions + final 8-ADR + 12-amendment count + ADR-0045 split-decision NOT-INVOKED.

13. **All six phase-done gates green at phase-done commit:** build/vet/lint clean; race-test clean across 40+ packages (39 pre-phase-17 baseline from phase-16 + new `internal/jwks/` + new `internal/jwt/` + new `internal/filter/http/jwtauthn/`); h2spec 53/53 PASS at ADR-0051 pin (phase 17 introduces no H2 wire-shape changes); 21 fuzzers green at 30s budget (~10.5 minutes wallclock); 20 differential fixtures green (19 pre-phase-17 + new 0019); BEHAVIOR_CONTRACT.md populated per Gate F (6-edit bundle landed).

14. **TENTH §9 family-row landed** per ADR-0106 — phase 17 jwt_authn extends the row-as-its-own-phase pattern through a 10th concrete filter under the §9 HTTP filters family.

15. **No master mutation outside the phase-17 squash-merge commit** — all work landed on the phase-17 worktree branches (brainstorm + spec + plan + impl + review per ADR-0005 §Decision 4); master tip advances only at the squash-merge commits + SHA-fill follow-ups.

End of phase 17 SPEC.






