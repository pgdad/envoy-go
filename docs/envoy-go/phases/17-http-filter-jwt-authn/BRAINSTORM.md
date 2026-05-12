# Phase 17 Brainstorm — `envoy.filters.http.jwt_authn`

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 17 (`http-filter-jwt-authn`), the TENTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, `header_mutation` at phase 10, `local_ratelimit` at phase 11, `csrf` at phase 12, `buffer` at phase 13, `compressor` at phase 14, `bandwidth_limit` at phase 15, and `rbac` at phase 16). The next session (lifecycle-state 1 → 2 for phase 17, skill `superpowers:brainstorming` per ADR-0005 scoped to SPEC authoring per the phase 09/10/11/12/13/14/15/16 precedent) authors `docs/envoy-go/phases/17-http-filter-jwt-authn/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §11 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-17-http-filter-jwt-authn-brainstorm`, branch `phase-17-http-filter-jwt-authn-brainstorm`, branched from master tip `66c9ac7` (the phase 16 impl follow-up STATE.md SHA-fill + lifecycle-state-5 → 6 narrative commit). The phase 16 squash-merge commit `a0bb191` and its SHA-fill follow-up `66c9ac7` are the immediate predecessors on master; phase 16's PLAN squash `40f030b` + PLAN SHA-fill `948f6c4` + SPEC squash `3159811` + SPEC SHA-fill `cedf29a` are earlier still. `66c9ac7` is the current master tip.

**Brainstorm mode:** interactive with a live human. The user picked filter selection + each major design decision via 8-question dialogue (Q1 §9 family-row pick — `jwt_authn` chosen from the 9-candidate remaining list `global_ratelimit / jwt_authn / ext_authz / ext_proc / oauth2 / lua / wasm / adaptive_concurrency / admission_control`; Q2 JWKS source envelope — `Both proto-faithful` chosen from `Both / RemoteJwks-only / LocalJwks-only / Both-RemoteJwks-bounded`; Q3 algorithm subset — `RS + ES family` chosen from `RS+ES / RS-only / RS256-only / RS+ES+HS`; Q4 JwtRequirement subset — `Full 6 proto-faithful` chosen from `Full 6 / Core 4 / Small 2 / Tiny 1`; Q5 token extraction sources — `All 4 proto-faithful` chosen from `All 4 / Auth+headers+params / Auth+headers / Auth-only`; Q6 post-validation side-effects — `Full header-side proto-faithful` chosen from `Full header-side / minimal-header / gate-only / header-side+clear_route_cache`; Q7 per-route shape — `8th canonical proto-faithful` chosen from `8th-canonical / 5th-canonical-reuse / 8th-requirement_name-only / 7th-canonical-adapt`; Q8 listener-level fields — `All 3 proto-faithful` chosen from `All 3 / rules-only / per-route-only / All-3-plus-deprecated-requires`). The §9 family-row continuation is implicit per ADR-0106. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0147, where ADR-0140-ADR-0147 landed in phase 16), and the just-shipped phase 16 + phase 15 + phase 14 + phase 13 + phase 12 + phase 11 + phase 10 + phase 09 + phase 07.1 artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §10 and deferred to SPEC-drafting time per the phase 09 + 10 + 11 + 12 + 13 + 14 + 15 + 16 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/16-http-filter-rbac/BRAINSTORM.md` and `docs/envoy-go/phases/15-http-filter-bandwidth-limit/BRAINSTORM.md` section-for-section, reframed for the jwt_authn scope and adapted for its specific surface area. Phase 17 sits in a structurally important position relative to the §9 family: it is the FIRST §9 family-row whose configuration consumes a **remote network resource at boot/runtime** (the JWKS endpoint), introducing the first outbound-HTTP framework primitive in envoy-go (strategically reusable by ext_authz HTTP-mode + oauth2 token-endpoint in future phases). Per the Q2 + Q3 + Q4 user picks (Both-JWKS proto-faithful + RS+ES algorithms + Full-6 JwtRequirement), phase 17 commits to **TWO new framework primitives** — the FIRST §9 row since phase 16 to introduce non-zero framework deltas, mirroring phase-16's two-primitive precedent: (i) an outbound-HTTP fetcher with async refresh + cache + retry/backoff at a new top-level package; (ii) a JWS/JWT verifier at a new top-level package supporting the RS + ES algorithm families. Sections §§1–11 are decision-bearing prose; §10 enumerates the empirical-pin obligations the SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear. NO off-master prebrainstorm-notes branch was authored for phase 17 — this brainstorm cold-started fresh from the §9 heading + the phase 16 just-shipped artefacts per ADR-0106(e).

**Authored:** 2026-05-12.

---

## 1. Mission and scope confirmation (17 only)

ROADMAP row `17 | http-filter-jwt-authn | 16 | planned | | …` (added by this brainstorm, see §10 below) is the row this brainstorm registers as `planned`. Phase 17 is the TENTH concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading (the family heading at `ROADMAP.md` line 63 — `### HTTP filters family` — is a conceptual umbrella, not a row, per ADR-0106). The phase 16 squash-merge commit `a0bb191` (with SHA-fill at `66c9ac7`) is this row's `depends-on` anchor.

The HTTP filters family lists candidate filters at `ROADMAP.md` line 65: header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. `cors` shipped in phase 07.1 (`internal/filter/http/cors/` per ADR-0074); `fault` shipped in phase 09 (`internal/filter/http/fault/` per ADR-0100); `header_mutation` shipped in phase 10 (`internal/filter/http/header_mutation/` per ADR-0108); `local_ratelimit` shipped in phase 11 (`internal/filter/http/localratelimit/` per ADR-0114); `csrf` shipped in phase 12 (`internal/filter/http/csrf/` per ADR-0120); `buffer` shipped in phase 13 (`internal/filter/http/buffer/` per ADR-0125); `compressor` shipped in phase 14 (`internal/filter/http/compressor/` per ADR-0129–ADR-0134); `bandwidth_limit` shipped in phase 15 (`internal/filter/http/bandwidthlimit/` per ADR-0135–ADR-0139); `rbac` shipped in phase 16 (`internal/filter/http/rbac/` + `internal/matcher/` per ADR-0140–ADR-0147). Phase 17 ships **JWT (JSON Web Token) bearer-token validation as a decode-side authentication gate** as the TENTH real filter — the canonical Envoy-style "extract bearer token; validate signature + claims; allow / deny / forward payload" filter. The chosen branch + directory + Go-package identifier are aligned per phase-11 ADR-0114's underscore-stripping convention: branch `phase-17-http-filter-jwt-authn-brainstorm` (preserves underscore for human readability of the source filter type-URL), directory `internal/filter/http/jwtauthn/` (underscore-stripped per ADR-0114; matches `localratelimit/` precedent), Go package identifier `jwtauthn` (same as directory).

Phase 17 is also: (i) the FIRST §9 family-row whose configuration **consumes a remote network resource** (the JWKS endpoint of an external IdP) — distinct from all prior §9 rows which were self-contained (fault delay/abort, header mutations, local rate-limit, csrf origin-check, buffer body-buffer, compressor gzip, bandwidth_limit throttle, rbac in-memory policy evaluation). This introduces an entirely new failure-mode class — `RemoteJwks` fetch can fail at boot OR at runtime refresh — and requires a corresponding new framework primitive (the outbound-HTTP fetcher with async refresh + cache + retry/backoff at §3.1). (ii) the FIRST §9 family-row whose validation depends on **cryptographic signature verification** — JWS/JWT signature checking across RS + ES algorithm families requires a new framework primitive (the JWT verifier at §3.2; pure-Go stdlib `crypto/rsa` + `crypto/ecdsa`). (iii) the FIRST §9 family-row to introduce a **NEW canonical per-route pattern** since phase 16 codified the 7th — jwt_authn's `PerRouteConfig{oneof{requirement_name(string) | disabled(Empty)}}` with reference-by-name into listener-level `requirement_map` is structurally distinct from the 7 prior canonicals, warranting an 8th-canonical entry in ADR-0125 amendment §(xiii). (iv) the FIRST §9 family-row to use a **string-reference-delegation per-route pattern** — the 8th canonical's defining feature is that per-route doesn't carry its own provider config but DELEGATES by name to a listener-level requirement_map entry. (v) the FIRST §9 family-row with a **www-authenticate challenge header** on the deny path — RFC 6750 compliance requires emitting `www-authenticate: Bearer realm="..."` on 401 responses; this is the FIRST 401-emitting filter in envoy-go (fault/csrf/local_ratelimit/rbac all use 403 or 200+abort wire shapes). (vi) the FIRST §9 family-row whose framework primitive accumulation is **strategic for the auth-filter family** — the outbound-HTTP primitive at §3.1 is explicitly designed cross-phase-reusable for future ext_authz HTTP-mode (phase 18+) and oauth2 token-endpoint flow (phase 19+).

### 1.1 What 17 delivers as a self-contained whole

Phase 17 lands `envoy.filters.http.jwt_authn` (the canonical Envoy JWT-authentication filter, RS+ES algorithm family, Both-JWKS-source proto-faithful, Full-6 JwtRequirement variants, 8th-canonical per-route delegation) under the 07.1 framework. **Eight in-scope filter-implementation items, plus three artefact-level deliverables (11 total bullets):**

1. **New `internal/filter/http/jwtauthn/` package** owning the filter implementation. Package directory + Go package identifier are both `jwtauthn` (single token underscore-stripped per ADR-0114; matches `localratelimit/` precedent). Files mirror the multi-file structure of phase 16 + phase 14 + phase 15 (the precedent for larger filters): `jwtauthn.go` (filter type + factory + decode methods + filterStats struct + compiledConfig + per-route helper), `evaluator.go` (JwtRequirement evaluator — provider_name, provider_and_audiences, requires_any, requires_all, allow_missing, allow_missing_or_failed recursive combinators + RequirementRule dispatch + extraction-source iteration), `provider.go` (JwtProvider compiled-state — algorithm allow-list + JWKS reference + extraction-source set + side-effect set + jwt_cache_config no-cache no-op), `jwtauthn_test.go` (unit tests; anticipated 1500-2500 LoC given the evaluator + provider + extraction-source + verification subsurface), `fuzz_test.go` (the 21st fuzzer in the repo — `FuzzJwtAuthnConfigParse`), `doc.go` (package overview + 8-decision summary + Both-JWKS + RS+ES + Full-6-Requirement + All-4-extraction + Full-header-side + 8th-canonical-per-route summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / local_ratelimit / csrf / buffer / compressor / bandwidth_limit / rbac precedent.

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 11 entries after phase 16: `router.New`, `bandwidthlimit.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `fault.New`, `header_mutation.New`, `localratelimit.New`, `rbac.New` before the `httpReg.Freeze()` invocation) gains a twelfth `httpReg.Register(jwtauthn.TypeURL, jwtauthn.New)` call before the freeze. Insertion alphabetical-after-header_mutation per the ADR-0100 §2.2 convention: `router → bandwidthlimit → buffer → compressor → cors → csrf → envoy_go_test → fault → header_mutation → jwtauthn → localratelimit → rbac → Freeze`. `jwtauthn` inserts between `header_mutation` and `localratelimit` to maintain alphabetical-after-router ordering. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only.

3. **Proto-config parsing of `envoy.extensions.filters.http.jwt_authn.v3.JwtAuthentication`,** the canonical filter-level config message. Per `go-control-plane/envoy v1.32.4` (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), the message has 5 top-level fields; phase 17 **consumes ALL 5** in the proto-faithful posture per Q2 + Q4 + Q8 user picks. **Consumed at runtime (5 fields):**

   - `providers` (`map<string, JwtProvider>`) — provider config registry; each provider has issuer + audiences + JWKS source + algorithm allow-list + extraction-source set + side-effect set.
   - `rules` (`repeated RequirementRule`) — listener-level trigger map; each rule has `match` (`RouteMatch`; reuses phase-04 RouteMatch evaluator — zero new framework delta for the match side) and `requirement_name` (string into requirement_map).
   - `requirement_map` (`map<string, JwtRequirement>`) — named-requirement registry; referenced by both listener-level rules AND the 8th-canonical per-route.
   - `bypass_cors_preflight` (bool) — skip JWT validation for CORS preflight (OPTIONS) requests.
   - `strip_failure_response` (bool) — controls whether the failure body + WWW-Authenticate header are stripped from the 401 response.

   **Inside `JwtProvider` (consumed when a provider entry is referenced from rules + per-route):** ~13 fields consumed of ~17 total. Consumed: `issuer` (string; required-or-PARSE-REJECT per provider semantic), `audiences` (repeated string; OR-semantic match; empty-means-skip-audience-check), `remote_jwks` (RemoteJwks → URI + cache_duration + async_fetch — Q2 proto-faithful), `local_jwks` (DataSource → inline_string / inline_bytes / filename — Q2 proto-faithful; oneof exclusive-with remote_jwks), `forward` (bool; strip-on-success default), `from_headers` (repeated JwtHeader{name, value_prefix}), `from_params` (repeated string), `from_cookies` (repeated string), `forward_payload_header` (string), `pad_forward_payload_header` (bool), `claim_to_headers` (repeated ClaimToHeader{header_name, claim_name}), `clear_route_cache` (bool), `jwt_cache_config` (JwtCacheConfig — MVP no-cache no-op; documented at §8 deferral 8). DEFERRED (4 fields couple to dynamic-metadata family — phase-16 REVIEW item 9 forward-pointer): `payload_in_metadata`, `header_in_metadata`, `failed_status_in_metadata`, `normalize_payload_in_metadata`.

   **Inside `RequirementRule` (the listener-level trigger):** `match` (RouteMatch — reuses phase-04 evaluator) + `requirement_name` (string into requirement_map). Deprecated `requires` (inline JwtRequirement) field PARSE-REJECT envoy-go-only with envoy-go-only error per §8 deferral 9 (Envoy v1.37.2 honors with deprecation warning — envoy-go-strict divergence-window).

   **Inside `JwtRequirement` (the 6-variant policy graph; consumed via the 8th-canonical per-route OR via listener-level rules' requirement_name resolution):** all 6 oneof variants honored per Q4 Full-6:
   - `provider_name` (string) — references providers map by key
   - `provider_and_audiences` (ProviderWithAudiences{provider_name, audiences}) — per-rule audience override
   - `requires_any` (JwtRequirementOrList{requirements: []JwtRequirement}) — OR-semantic combinator (recursive)
   - `requires_all` (JwtRequirementAndList{requirements: []JwtRequirement}) — AND-semantic combinator (recursive)
   - `allow_missing` (Empty) — JWT absent OK; bad-JWT still rejects
   - `allow_missing_or_failed` (Empty) — JWT absent OR bad OK

   **Algorithm allow-list (six algorithms; Q3 RS + ES family):** `RS256` / `RS384` / `RS512` (RSASSA-PKCS1-v1_5 with SHA-256/384/512) + `ES256` / `ES384` / `ES512` (ECDSA with P-256/P-384/P-521 + SHA-256/384/512). Validation via Go stdlib `crypto/rsa.VerifyPKCS1v15` + `crypto/ecdsa.Verify`. JWK with `alg` claim outside the six allow-list PARSE-REJECTED at JWKS-parse time (envoy-go-strict); JWT with `alg` claim outside the six runtime-rejected as `bad-signature` failure-reason. DEFERRED algorithm families per §8 deferrals 5-7: HS family (HMAC; requires symmetric-secret config plumbing); EdDSA; `none` (intentionally never enabled — security-sensitive).

4. **Per-route TPFC: NEW 8th canonical pattern (oneof{requirement_name(string) | disabled(Empty)}; ADR-0125 amendment §(xiii)).** Per the proto message `PerRouteConfig`, per-route entries carry a oneof `requirement_specifier` with two arms:
   - (a) `PerRouteConfig{requirement_name: "<name>"}` → reference-by-name into the listener-level `requirement_map`. Filter resolves the named requirement at config-load time (PARSE-REJECT dangling references per §10 pin §17.P12) → produces a `*compiledPerRoute` referencing the resolved JwtRequirement. At request time, the filter evaluates the resolved requirement against the request's extracted JWT.
   - (b) `PerRouteConfig{disabled: <Empty>}` → the filter is wholly inactive on this route, no JWT validation, no counter increments, request forwards as-is past the gate.

   **Phase 17 is the FIRST row to use the string-reference-delegation discipline** — distinct from the 5th canonical (explicit `disabled` boolean oneof + wholesale-override sub-message; phase-13/14), the 6th (bare-message-via-TPFC + code-level-required field; phase-15), and the 7th (wrapper with reserved field + single optional sub-message, absent-implies-disabled; phase-16). ADR-0125 gains an in-place amendment paragraph §(xiii) codifying the 8th canonical pattern: the per-route does NOT carry its own provider/requirement config — it delegates by name to a listener-level requirement_map entry. The delegation pattern is symmetric with the listener-level `RequirementRule.requirement_name` (both reference the same `requirement_map`). This is a NEW canonical pattern, NOT an extension of the 7th: the 7th canonical's defining feature is the absence-implies-disabled-via-proto-comment with wholesale-override on presence; the 8th canonical's defining feature is the string-reference-delegation into a separately-named registry. Both shapes honored in MVP. Each TPFC entry runs through `parsePerRoute` at config-load time → produces a `*compiledPerRoute` value. The 3-tier `PerRouteConfig.Resolve` (Route > VirtualHost > RouteConfiguration > listener fallback) selects the most-specific per-route entry per request; that entry's shape (`requirement_name`-resolved OR `disabled(Empty)`) drives the disposition.

   **Per-route stats SHARED with listener-level** per §5: per-route just delegates by name; per-route does NOT carry its own stat_prefix (no `rules_stat_prefix`-equivalent at jwt_authn). Per-route override emits to the same stat namespace as the listener-level config. DIVERGES from phase-11 / phase-15 / phase-16 INDEPENDENT-stats; MIRRORS phase-12 / phase-13 / phase-14 SHARED-stats discipline. The stateful-policy-evaluation-per-route concern (which motivated INDEPENDENT-stats at phase-11/15/16) does NOT apply here — jwt_authn's per-route is a pure delegation, not a stateful policy-evaluator clone.

5. **Filter-callback shape: `StreamDecoderFilter` ONLY on the `*filter` instance.** Phase 17 is decode-side only — `jwt_authn` is a request-gate filter, evaluated at `DecodeHeaders` time, with the disposition (allow / deny) computed BEFORE the request body is forwarded. The filter does NOT implement `StreamEncoderFilter`. Static blank-identifier compile-time check for `StreamDecoderFilter` only. The decode-side surface: `DecodeHeaders(headers, endStream)` resolves per-route → caches `*compiledPerRoute` on filter state → runs the JwtRequirement evaluator (which iterates extraction sources, validates the JWT against the resolved provider's algorithm + JWKS, applies side-effects) → emits the appropriate counter delta → returns `HeaderContinue` (allow) OR invokes `cb.SendLocalReply(401, ...)` and returns `HeaderStopIteration` (deny). `DecodeData` + `DecodeTrailers` pass-through. `OnDestroy` no-op (no timers active per-request; mirrors phase-12 / phase-16 precedent). Listener-level RequirementRule evaluation also fires at `DecodeHeaders` when per-route is unset — the filter iterates listener-level rules, evaluates each rule's RouteMatch against the request, and applies the first-matching rule's `requirement_name` per the listener-level dispatch table.

6. **Validation algorithm (Decision #4 → ADR-0149 + ADR-0151).** Per Q3 + Q4 picks:

   **Extraction step** (per provider's extraction-source set, ordered):
   1. Authorization Bearer header (always honored unless explicitly stripped from extraction-sources by the rare absent-from-all-headers config).
   2. `from_headers[]` (in declared order; first match wins).
   3. `from_params[]` (query string; URL-decoded; case-sensitivity per §10 pin §17.P14).
   4. `from_cookies[]` (cookie name; case-sensitivity + value-parsing per §10 pin §17.P15).

   **Decoding step** (JWT structural parse):
   - Split on `.` into 3 parts (header.payload.signature); reject if not exactly 3 parts (`bad-token` failure-reason).
   - Base64url-decode header + payload; parse as JSON.
   - Validate `alg` claim in header against provider's algorithm allow-list (RS256/384/512 + ES256/384/512); reject if outside (`bad-signature` failure-reason).
   - Validate `kid` claim in header (if present) against JWKS keys; pick matching key OR fall back to first key with matching `alg` (per Envoy's pickKeyAlgWithKid logic).

   **Signature verification step** (Decision #4 → ADR-0151 framework primitive):
   - Reconstruct signed-bytes (`<base64url-header>.<base64url-payload>`).
   - Verify signature using stdlib `crypto/rsa.VerifyPKCS1v15` (for RS) or `crypto/ecdsa.Verify` (for ES). Reject if invalid (`Jwt verification fails` failure-reason).

   **Claim validation step:**
   - `exp` (expiration; required-or-fail per provider config? §10 pin §17.P9 confirms): reject if past current time (`Jwt expired` failure-reason).
   - `nbf` (not-before): reject if future (`Jwt not yet valid` failure-reason if present).
   - `iat` (issued-at): informational; not enforced for rejection.
   - `iss` (issuer): match provider's `issuer` field exactly (`Jwt issuer is not configured` failure-reason).
   - `aud` (audience; string or array): if provider's `audiences[]` is non-empty, require intersection with the JWT's `aud` (`Audiences in Jwt are not allowed` failure-reason); empty `audiences[]` means skip.

   **JwtRequirement evaluation step** (per the 6-variant evaluator):
   - `provider_name`: validate JWT against named provider; pass iff valid + claim-checks pass.
   - `provider_and_audiences`: validate against named provider with the per-rule audience override.
   - `requires_any`: recursively evaluate each sub-requirement; pass on first match.
   - `requires_all`: recursively evaluate each sub-requirement; pass iff ALL match.
   - `allow_missing`: pass iff JWT absent (no extraction matches); fail iff JWT present-and-invalid.
   - `allow_missing_or_failed`: always pass.

   **Side-effect step** (per Q6 Full-header-side; on validation success):
   - If `forward == false` (default true): strip the Authorization header (or original extraction source).
   - If `forward_payload_header` non-empty: emit `<header_name>: <base64-encoded payload>` (with or without `=` padding per `pad_forward_payload_header`).
   - For each `claim_to_headers[]` entry: extract `claim_name` from JWT payload; emit `<header_name>: <claim-value-string>`. Multi-value claims (array) handled per §10 pin §17.P10.
   - If `clear_route_cache == true`: invoke `cb.ClearRouteCache()` (HCM-side primitive that forces re-route-match after header mutation; per phase-10 ADR-0108 precedent).

   **Wire-shape conformance with reference Envoy** (deliberate; documented at BEHAVIOR_CONTRACT phase-17 forward-pointer notes): allow-decisions pass through with header-mutated request (forward / forward_payload_header / claim_to_headers writes) but BYTE-EQUIVALENT response from upstream backend. Deny-decisions emit `SendLocalReply(401, "<failure-reason body>", {Content-Type: text/plain, WWW-Authenticate: Bearer realm="<issuer>"})` mirroring the RFC 6750 401-challenge wire-shape discipline; §10 empirical pin §17.P1 confirms exact byte-content of the 401 body per failure-reason (5+ canonical bodies anticipated: `Jwt is missing`, `Jwt verification fails`, `Jwt issuer is not configured`, `Jwt expired`, `Audiences in Jwt are not allowed`); §10 pin §17.P2 confirms exact `www-authenticate` header format (with or without `error=` parameter). `strip_failure_response=true` (§10 pin §17.P3) strips both the body AND the WWW-Authenticate header — the 401 wire-bytes become 4-header-only.

7. **Stat surface — 64→~72-name minimum extension; variably larger per provider count (Decision implicit in stat-surface hypothesis → ADR-0154).** **8 base counters + per-provider counter scaling** under `BEHAVIOR_CONTRACT.md ## Stat-name mapping`, extending the phase-16 64-name table:

   Per-provider counters (analogous to phase-16's per-policy template; scales with provider count):
   - `jwt_authn.<provider>.jwt_authn_success` — counter; increments per request where the named provider's JWT validated successfully.
   - `jwt_authn.<provider>.jwt_authn_failed` — counter; increments per request where the named provider's JWT validation failed.

   Provider-level JWKS counters (one set per `RemoteJwks` provider):
   - `jwks_fetch_success` — counter; increments per successful JWKS fetch from RemoteJwks endpoint.
   - `jwks_fetch_failed` — counter; increments per failed JWKS fetch.
   - `jwks_cache_hit` — counter; increments per request that resolved JWKS from cache (no fetch needed).
   - `jwks_cache_miss` — counter; increments per request where JWKS cache miss triggered fetch.
   - `jwks_fetch_in_progress` — gauge; current count of in-flight JWKS fetches.

   Filter-wide counter (one set per HCM stat_prefix):
   - `bypassed_cors_preflight` — counter; increments per OPTIONS preflight request that bypassed validation via `bypass_cors_preflight=true`.

   §10 pin §17.P6 confirms exact stat names + scope + counter-vs-gauge disposition (e.g., the per-provider `jwt_authn_success` / `jwt_authn_failed` may be the canonical Envoy form OR a different shape — empirical scrape resolves). §10 pin §17.P7 confirms exact Prometheus tag-extractor name + namespace flattening rule (hypothesis: SN2 reuse — the existing HCM-stat-prefix tag-extractor handles `http.<HCM>.jwt_authn.<provider>.*` verbatim without amendment; SN10 introduced only if pin demands).

   **Stat surface count summary:**
   - Phase 15 (bandwidth_limit): 46 → 60 names (14 new active counter/gauge; SN2 reuse; +2 deferred-histogram via twin-series-filter divergence-window per ADR-0138).
   - Phase 16 (rbac): 60 → 64 base names (4 new active counter; per-policy lazy counter family conditional on `track_per_rule_stats: true`; SN2 reuse; ADR-0145).
   - Phase 17 (jwt_authn): 64 → **~72 names** (8 new base; per-provider counter family scales with provider count; SN2 reuse hypothesis with SN10 introduced only if §10 pin §17.P7 demands).

   **Per-route stats discipline: SHARED hypothesis** (§10 pin §17.P8 confirms). Rationale: per-route just delegates by name to a listener-level requirement_map entry; per-route does NOT carry its own provider config or its own stat_prefix (mirrors phase-12 csrf per ADR-0124 + phase-13 buffer per ADR-0125 + phase-14 compressor per ADR-0132; DIVERGES from phase-11 / phase-15 / phase-16 INDEPENDENT-stats). The pure-delegation nature of jwt_authn's per-route is the load-bearing motivator: per-route override does NOT spawn new policy-evaluation state — it simply selects which listener-level requirement applies on this route.

8. **TWO new framework primitives** — the FIRST §9 row since phase 16 to introduce non-zero framework deltas, mirroring phase-16's two-primitive precedent: (i) HTTP-outbound fetcher with async refresh + cache + retry/backoff at a new top-level package `internal/jwks/` (ADR-0150); (ii) JWS/JWT verifier at a new top-level package `internal/jwt/` (ADR-0151). See §3 for details. Both primitives are explicitly designed cross-phase-reusable at introduction time — anchoring the same cross-phase-reuse-at-introduction-time discipline phase 16 established at ADR-0142 + ADR-0144. The framework-delta accretion across §9 family-rows:

   - Phase 07.1 cors: NEW framework (the entire HTTP-filter framework). N/A baseline.
   - Phase 09 fault: introduced `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume primitives.
   - Phase 10 header_mutation: ZERO framework deltas.
   - Phase 11 local_ratelimit: ZERO framework deltas.
   - Phase 12 csrf: ZERO framework deltas.
   - Phase 13 buffer: TWO framework deltas (decode-side per ADR-0128).
   - Phase 14 compressor: ONE framework delta (`OverwriteBody` per ADR-0131).
   - Phase 15 bandwidth_limit: ZERO framework deltas (load-bearing reusability demonstration).
   - Phase 16 rbac: TWO framework deltas (matcher-engine per ADR-0142 + TLS-principal accessor per ADR-0144).
   - **Phase 17 jwt_authn: TWO framework deltas (HTTP-outbound JWKS fetcher per ADR-0150 + JWT verifier per ADR-0151).** The SECOND consecutive §9 row to ship two primitives — both genuinely cross-cutting for the auth-filter family (HTTP-outbound is reusable by ext_authz HTTP-mode + oauth2 token-endpoint; JWT verifier is reusable by any future filter consuming JWT semantics).

**Plus three artifact-level deliverables:**

9. **Differential fixture `0019-http-jwt-authn`** under `test/fixtures/0019-http-jwt-authn/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising 8 scenarios per §6 below. The fixture reuses `test/helpers/echobackend/` from phase 14/15/16 for the allow-disposition scenarios (the request passes through to backend; backend echoes; equivalence asserted on backend-arrival-time + response status + body). For the RemoteJwks scenarios, the fixture spawns a small in-process HTTP JWKS server (test-helper) serving a static JWK Set; the JWKS server's URL is wired into both `envoy.yaml` and `envoy-go.yaml` configs. The fixture asserts response status, **body byte-equivalent on allow paths AND on deny-401 paths** (jwt_authn does NOT transform response bytes; only request headers), counter deltas via `/stats/prometheus` scrape equivalence on the 8 base counters, per-route-tier-disposition (both `requirement_name`-by-string and `disabled(Empty)` per-route shapes exercised), forward-payload-header backend-arrival assertion, claim-to-headers backend-arrival assertion. PKI generation for RemoteJwks scenarios reuses phase-16's `pki/gen.go` pattern (one RSA + one ECDSA signing keypair; one JWK Set serving both public keys).

10. **`BEHAVIOR_CONTRACT.md` 6-edit bundle.** Under the existing `## HTTP filter chain` umbrella (alongside the existing 9 filter subsections): a NEW `### envoy.filters.http.jwt_authn` subsection covering the 5-consumed-listener-field map, the 13-consumed-JwtProvider-field map, the 6-variant JwtRequirement evaluator semantics, the RS+ES algorithm allow-list, the All-4-extraction-source set, the Full-header-side side-effect set, the 8th-canonical-per-route (string-reference-delegation) semantics, the SHARED-stats hypothesis for per-route, the 401-WWW-Authenticate deny-path SendLocalReply wire shape with `strip_failure_response` divergence-window. Plus the 64→~72-name stat-table extension. Plus a new equivalence-matrix row pointing at fixture 0019 with per-scenario tolerance discipline. Plus a NEW `### Phase 17 forward-pointer notes` subsection under `## Forward-pointer notes` covering the ~13-item deferral list (per §8 below). Plus a NEW `## JWKS framework primitive` top-level umbrella (per §3.1 + ADR-0150) anchoring future filters' reuse of the outbound-HTTP + JWKS-cache primitives. Plus a NEW `## JWT verifier framework primitive` top-level umbrella (per §3.2 + ADR-0151) anchoring future filters' reuse of the JWT verifier.

11. **Anticipated 8 ADRs (ADR-0148 through ADR-0155) plus ADR-0125 §(xiii) amendment paragraph** per §7 below. ADR-0147 is the highest-numbered ADR landed in phase 16; ADR-0148 is the next-free.

### 1.2 What 17 does NOT deliver (forward to §8)

The exhaustive deferral list lives in §8 under the inline-deferral discipline (no omnibus ADR per phase 11 SPEC §8.1 + phase 12/13/14/15/16 precedent; deferrals are 13 items grouped by family-coupling, comparable to phase 16's 15-item list given jwt_authn's narrower-but-still-substantial proto surface). Summary: 4 dynamic-metadata-family-coupled fields (`payload_in_metadata`, `header_in_metadata`, `failed_status_in_metadata`, `normalize_payload_in_metadata`); 3 algorithm-family deferrals (HS family, EdDSA, `none`); `jwt_cache_config` validated-JWT result LRU cache (MVP no-cache); deprecated `RequirementRule.requires` field (PARSE-REJECT envoy-go-strict divergence from Envoy's lenient honor-with-deprecation-warning); JWKS bounded retry/backoff customization beyond the canonical policy; JWKS cache-invalidation hooks; access-log integration (couples to phase-16 forward-pointer access-log family item 7); CEL-based dynamic provider selection (couples to phase-16 CEL deferral item 10). None are blockers for closing row 17 phase-done.

### 1.3 Phase-done as a §9 family-row landing

Phase 17's phase-done commit closes ROADMAP row `17` (single-row hypothesis at brainstorm time — with ADR-0045 split as the release valve per §1.4 below). It does NOT close any §9 family heading (family headings are not rows per ADR-0106) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships. Phase 17 is the TENTH §9 family-row to land (after 07.1-cors, 09-fault, 10-header_mutation, 11-local_ratelimit, 12-csrf, 13-buffer, 14-compressor, 15-bandwidth_limit, 16-rbac). The next §9 family-row will be numbered `18` per the flat-row discipline of ADR-0106. The §9 heading at `ROADMAP.md` line 63 stays unchanged across this landing.

### 1.4 ADR-0045 split-by-surface readiness — MODERATE anticipation for phase 17

The brainstorm's POSITION is that phase 17 **likely sits at or slightly above ADR-0045's 1500-LoC / 25-task split-trigger** and that the SPEC author should evaluate the split at SPEC drafting. LoC estimate: ~1800-2500 LoC (anticipated):

- `jwtauthn.go` (filter + factory + compiledConfig + decode methods + filterStats + per-route helper): ~500-700 LoC.
- `evaluator.go` (6-variant JwtRequirement evaluator + extraction-source iteration + RequirementRule dispatch): ~300-500 LoC.
- `provider.go` (JwtProvider compiled-state + algorithm allow-list + JWKS reference + side-effect set): ~300-400 LoC.
- `jwtauthn_test.go` (unit tests across all subsurfaces): ~1500-2500 LoC.
- `fuzz_test.go`: ~80 LoC.
- `doc.go`: ~80-150 LoC.
- Framework-delta plumbing at `internal/jwks/` (~250-400 LoC) + `internal/jwt/` (~200-300 LoC).
- Fixture (driver + yaml + README + PKI gen + test-helper JWKS server): ~600-900 LoC.

Total: ~3800-5900 LoC including tests + fixtures + framework primitives. Production-only ~1800-2500 LoC. This is **at or above** the ADR-0045 1500-LoC trigger; the split-by-surface release valve is RECOMMENDED at SPEC time. Two anticipated splits if the SPEC author opts to split:

- **17.1 = LocalJwks + RS+ES verifier + Full-6 JwtRequirement + 8th-canonical per-route + listener-level rules**: the foundational filter — JwtAuthentication + JwtProvider parsing with LocalJwks only (no remote fetch), RS+ES verifier, Full-6 JwtRequirement evaluator, All-4 extraction sources, Full-header-side side-effects, 8th-canonical per-route + ADR-0125 §(xiii) amendment, listener-level RequirementRule dispatch. Excludes: RemoteJwks framework primitive (ADR-0150 deferred to 17.2), JWKS cache + refresh + retry. Differential fixture covers LocalJwks + valid/invalid token scenarios. LoC estimate: ~1200-1700 LoC.
- **17.2 = RemoteJwks + JWKS cache + scheduled refresh + retry/backoff**: the secondary-engine landing — the HTTP-outbound JWKS fetcher with async refresh + LRU cache + retry policy (ADR-0150 lands). Fixture extends with RemoteJwks + cache-hit/miss scenarios. LoC estimate: ~600-900 LoC.

This split mirrors phase 05 (downstream-h2 → upstream-h2) + phase 06 (stats-prometheus → access-log) + phase 07 (http-filter-framework → listener-chain-completion) + phase 08 (admin-endpoints → graceful-drain) precedents for surface-split-with-framework-primitive-on-second-half. The brainstorm does NOT pre-commit to the split per ADR-0045 deferral-to-SPEC discipline — that decision is the SPEC author's call (mirrors phase-13/14/15/16 single-row position that ended up single-row in practice despite borderline LoC estimates). If the SPEC author concludes single-row is feasible (e.g., the JWKS framework-primitive scope is narrower than anticipated, or the JwtRequirement evaluator surface compresses), single-row is acceptable too. The split-by-engine option above is the structurally clean cut if the surface comes in heavy.

### 1.5 Seed-stub alignment

Like phases 09, 10, 11, 12, 13, 14, 15, and 16, phase 17 has NO sibling SPEC stub — phase 17 enters fresh after the phase 16 close. The §9 family-children list at ROADMAP line 65 enumerates the conceptual surface; the ROADMAP rows enumerate only filters currently in-progress or done. Per ADR-0106(b) (no-sibling-stub discipline), this brainstorm does NOT pre-author SPEC stubs for siblings (`global_ratelimit`, `ext_authz`, `ext_proc`, `oauth2`, `lua`, `wasm`, `adaptive_concurrency`, `admission_control`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts.

### 1.6 No prebrainstorm-notes branch

UNLIKE phase 11 which had an off-master prebrainstorm-notes branch (`phase-11-http-filter-local-ratelimit-prebrainstorm-notes`), phase 17 has NO such branch. The brainstorm dialogue (Q1-Q8 over the user-Claude exchange) was sufficient to settle filter pick + JWKS source envelope + algorithm subset + JwtRequirement subset + extraction sources + side-effects + per-route shape + listener-level fields without preliminary scoping notes. This matches the phase 09 / 10 / 12 / 13 / 14 / 15 / 16 cold-start precedent.

### 1.7 Phase 17's relationship to prior framework deltas + framework-delta accretion shape

Phase 17's TWO new framework deltas (HTTP-outbound JWKS fetcher + JWT verifier) are the FIRST framework introductions since phase 16's matcher-engine + TLS-principal-accessor pair. The accretion shape across §9 rows:

- Phase 13 introduced two decode-side framework primitives (ADR-0128).
- Phase 14 introduced one encode-side framework primitive (ADR-0131).
- Phase 15 introduced ZERO and demonstrated ADR-0128 + ADR-0131 reusability.
- Phase 16 introduced TWO — both designed cross-phase-reusable (matcher-engine + TLS-principal accessor).
- **Phase 17 introduces TWO** — both designed cross-phase-reusable (HTTP-outbound + JWT verifier). The SECOND consecutive two-primitive §9 row. The HTTP-outbound primitive is strategic for the auth-filter family (ext_authz HTTP-mode reuses; oauth2 token-endpoint reuses); the JWT verifier is reusable by any future filter consuming JWT semantics.

The framework-delta budget across phases 13-17 is now: ADR-0128 (decode-side; phase 13) + ADR-0131 (encode-side OverwriteBody; phase 14) + ADR-0142 (matcher-engine; phase 16) + ADR-0144 (TLS-principal accessor; phase 16) + ADR-0150 (HTTP-outbound + JWKS cache; phase 17) + ADR-0151 (JWT verifier; phase 17) = 6 framework primitive families across 5 phases. The HTTP filter framework primitive surface has accreted ~7 net new APIs over the phase-13-to-17 arc. Phase 17 marks the FIRST envoy-go phase to introduce an OUTBOUND-NETWORK framework primitive (all prior framework primitives operated on the inbound request OR on in-process state); this is the strategic foundation for the entire authentication/authorization filter family (ext_authz, oauth2, global_ratelimit) which all consume outbound-network resources.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The 8 decisions below are the phase-17-specific design choices reached during the Q-dialogue. Each cites its anticipated ADR anchor (§7); the ADRs are written by the SPEC author at lifecycle-state 1 → 2 transition.

### 2.1 Family-child selection: `jwt_authn` *(Decision #0 → ADR-0148 rationale)*

Per Q1 = "jwt_authn chosen from §9 remaining unbrainstormed list", the selection criteria per ADR-0106(e) + STATE.md.next-skill-scope four-criteria evaluation:

- **Cross-phase-reuse compatibility ★★★★★ with phase-16 framework primitives.** Phase 17 jwt_authn reuses BOTH new phase-16 primitives load-bearing: (i) ADR-0142 matcher-engine — jwt_authn's listener-level `rules` field's `match` is `RouteMatch` (NOT the xds matcher), so ADR-0142 is NOT directly reused; however, future versions of jwt_authn may extend to xds.type.matcher.v3.Matcher (future-Envoy direction); (ii) ADR-0144 TLS-principal accessor — jwt_authn's `Principal_Authenticated`-equivalent semantics for mTLS-bound JWT validation reuse the same downstream-cert principal accessor. Picks `jwt_authn` over the 8 other §9 candidates maximizes return on phase-16's framework investment per STATE.md.next-skill-scope strong-prior leaning.
- **Envelope size relative to ADR-0045 single-row threshold ★★★★.** Estimated ~1800-2500 LoC production; at or slightly above the 1500-LoC trigger; ADR-0045 split-readiness flagged (§1.4). Comparable to phase-16 rbac's ~2200-3000+ LoC estimate that ultimately landed single-row at ~2778 LoC. Among the 9 remaining candidates, only `adaptive_concurrency` and `admission_control` are smaller; the rest (ext_authz / ext_proc / oauth2 / lua / wasm / global_ratelimit) are larger.
- **Operator-visible-utility gradient ★★★★★.** JWT authentication is universal; one of the most-deployed Envoy filters in production. Higher operator-utility than every other §9 candidate except ext_authz (tie).
- **Precedent-density against phase-09..16 ★★★★★.** Decode-side gate mirrors csrf/rbac discipline; SendLocalReply 401 wire shape mirrors phase-09/11/12/16 patterns; per-route 8th-canonical builds on phase-13/14/15/16 canonical-pattern-growth precedent (ADR-0125 §(xiii) amendment lands the 8th canonical); shared-stats per-route mirrors phase-12/13/14; algorithm-allow-list discipline mirrors phase-14's compressor-library Any-dispatch pattern (PARSE-REJECT non-allowlisted types).
- **Framework-delta budget consideration.** Phase 17 introduces TWO new framework primitives (HTTP-outbound + JWT verifier) — a deliberate trade-off mirroring phase-16's TWO-primitive precedent. The HTTP-outbound primitive is strategic for the entire auth-filter family; deferring it to a later phase would block ext_authz HTTP-mode + oauth2 + global_ratelimit. Picking jwt_authn AS the introducer of HTTP-outbound maximizes long-term framework ROI.
- **Family-child rotation.** The remaining 8 §9 family-children after phase 17 lands (`global_ratelimit / ext_authz / ext_proc / oauth2 / lua / wasm / adaptive_concurrency / admission_control`) span widely-varying surface complexities. `jwt_authn` sits in the medium-large complexity band — large enough to introduce TWO strategic framework primitives but small enough to fit a phase-or-split-pair release. The LARGER remaining candidates (ext_proc with body-streaming; lua/wasm with embedded VMs) are still ahead.

**Runner-ups considered + rejected:** `ext_authz` (LARGER envelope ~1800-2500 LoC + much heavier gRPC-outbound primitive lift; near-certain ADR-0045 split; introduces gRPC primitive which is strategic but heavier than jwt_authn's HTTP primitive — gRPC could land in phase 18+); `global_ratelimit` (smaller envelope but lower phase-16 primitive reuse; the gRPC primitive is shared lift with ext_authz; not as strategically positioned as jwt_authn's HTTP primitive); `lua` / `wasm` (entirely separate world; embedded-runtime primitives are their own multi-phase sub-project per ROADMAP §9 + WASM-host-family heading at ROADMAP line 100); `adaptive_concurrency` / `admission_control` (smaller envelopes but low cross-phase-reuse + low operator-utility relative to jwt_authn).

ADR-0148 (the anticipated layout ADR for phase 17) documents the selection rationale + the TENTH §9 row + the SECOND consecutive two-primitive phase.

### 2.2 JWKS source envelope: Both proto-faithful *(Decision #1 → ADR-0149 + ADR-0150)*

Per Q2 = "Both RemoteJwks + LocalJwks proto-faithful", the proto's `JwtProvider.jwks_source_specifier` oneof has two arms; phase 17 consumes BOTH. **RemoteJwks** (with URI + cache_duration + async_fetch) requires the new framework primitive at §3.1 (HTTP-outbound + JWKS cache + scheduled refresh + retry/backoff). **LocalJwks** (DataSource: inline_string / inline_bytes / filename) is parse-once-at-config-load; no runtime network. Both flow through a common compiled `*provider` value that stores a JWKS-set (the parsed JWK keys); the difference is in the lifecycle (LocalJwks: parse-and-done; RemoteJwks: fetch-then-cache-with-refresh). The hybrid surface enables real-world IdP integration (RemoteJwks against Auth0/Okta/Google) AND testing/air-gapped paths (LocalJwks with inline JWKs). Tradeoff: framework-delta scope expands from ZERO to TWO primitives, but the HTTP-outbound primitive is strategically reusable. ADR-0149 anchors the per-arm parse discipline; ADR-0150 anchors the HTTP-outbound framework primitive shape.

### 2.3 Algorithm subset: RS + ES family *(Decision #2 → ADR-0151)*

Per Q3 = "RS + ES family (six algorithms)", phase 17 supports `RS256`, `RS384`, `RS512`, `ES256`, `ES384`, `ES512`. Algorithm allow-list enforcement happens at TWO points: (a) JWK parse time (when a JWK has an `alg` claim outside the allow-list — PARSE-REJECT with envoy-go-only error), and (b) JWT runtime (when a token's header `alg` is outside the allow-list — runtime-reject as `bad-signature` failure-reason). Go stdlib coverage: `crypto/rsa.VerifyPKCS1v15` for RS; `crypto/ecdsa.Verify` for ES; both in stdlib zero-dependency. PARSE-REJECT HS family (HMAC; requires symmetric-secret config plumbing — deferral 5), EdDSA (deferral 6), `none` (intentionally never enabled — security-sensitive; deferral 7). ADR-0151 anchors the verifier framework primitive's algorithm allow-list discipline.

### 2.4 JwtRequirement subset: Full 6 proto-faithful *(Decision #3 → ADR-0149)*

Per Q4 = "Full 6 proto-faithful", phase 17 supports all 6 oneof variants of `JwtRequirement`:
- `provider_name` (string) — references providers map by key
- `provider_and_audiences` (ProviderWithAudiences) — per-rule audience override
- `requires_any` (JwtRequirementOrList) — OR-semantic combinator (recursive)
- `requires_all` (JwtRequirementAndList) — AND-semantic combinator (recursive)
- `allow_missing` (Empty) — JWT absent OK; bad-JWT still rejects
- `allow_missing_or_failed` (Empty) — JWT absent OR bad OK

AND/OR combinators are recursive — `requires_any` + `requires_all` can be nested arbitrarily deep. Mirrors phase-16 rbac's `Permission_Set`/`Permission_And`/`Permission_Or` + `Principal_Set`/`Principal_And`/`Principal_Or` discipline; recursion-depth is unbounded at MVP (no envoy-go-only depth-cap; mirrors phase-16 §11.P11 RATIFIED-VIA-ABSENCE — Envoy also no cap). Documented as forward-pointer foot-gun per §8 deferral 14 (analogous to phase-16 deferred item 14).

### 2.5 Token extraction sources: All 4 proto-faithful *(Decision #4 → ADR-0152)*

Per Q5 = "All 4 proto-faithful", phase 17 supports extraction from:
1. `Authorization: Bearer ...` header (always honored unless explicitly stripped from extraction-sources by the rare absent-from-all-headers config).
2. `from_headers` (`repeated JwtHeader{name, value_prefix}`) — custom header names with optional value prefix.
3. `from_params` (`repeated string`) — query-string parameter names (URL-decoded value used as raw JWT; case-sensitivity per §10 pin §17.P14).
4. `from_cookies` (`repeated string`) — cookie names (Go stdlib `req.Cookie()` parser; case-sensitivity + value-parsing per §10 pin §17.P15).

Extraction-iteration order: Authorization first (highest priority), then from_headers in declared order, then from_params in declared order, then from_cookies in declared order. First match wins. Empty-extraction = "JWT missing" failure-reason. ADR-0152 anchors the extraction-source iteration discipline + the per-source resolver helpers.

### 2.6 Post-validation side-effects: Full header-side proto-faithful *(Decision #5 → ADR-0149)*

Per Q6 = "Full header-side proto-faithful", phase 17 emits 5 categories of side-effects on validation success:
- `forward` (bool; default behavior — see Q for default semantic; §10 pin §17.P9 confirms) — controls whether the Authorization header (or other extraction source) is stripped before forwarding to upstream.
- `forward_payload_header` (string) — emits `<header_name>: <base64-encoded payload>` to upstream request. Whether `=` padding is preserved per `pad_forward_payload_header`.
- `pad_forward_payload_header` (bool) — controls base64 padding shape; §10 pin §17.P13 confirms.
- `claim_to_headers` (repeated ClaimToHeader{header_name, claim_name}) — extracts named claims from JWT payload, emits as headers. Multi-value claim (array) handling per §10 pin §17.P10.
- `clear_route_cache` (bool) — invokes `cb.ClearRouteCache()` (HCM-side primitive that forces re-route-match after header mutation; per phase-10 ADR-0108 precedent).

The 4 dynamic-metadata-family-coupled fields (`payload_in_metadata`, `header_in_metadata`, `failed_status_in_metadata`, `normalize_payload_in_metadata`) DEFERRED per §8 deferrals 1-4 (couples to dynamic-metadata family blocked at phase-16 forward-pointer item 9). The side-effect emit-order: forward (strip) → forward_payload_header (write) → claim_to_headers (write) → clear_route_cache (invoke). ADR-0149 anchors the side-effect emit-order + the per-effect helper plumbing.

### 2.7 Per-route shape: NEW 8th canonical (string-reference-delegation) *(Decision #6 → ADR-0153 + ADR-0125 §(xiii))*

Per Q7 = "8th canonical proto-faithful", phase 17 introduces the 8th canonical per-route pattern: `PerRouteConfig{oneof{requirement_name(string) | disabled(Empty)}}` with reference-by-name into listener-level `JwtAuthentication.requirement_map`. Structurally distinct from the 7 prior canonicals (§4 below enumerates the full distinction matrix). ADR-0125 gains an in-place amendment paragraph §(xiii) codifying the 8th canonical; ADR-0153 anchors the per-route parsing + delegation-resolution discipline. Dangling-reference handling at parse time per §10 pin §17.P12 (Envoy v1.37.2 parse-reject vs runtime-reject — envoy-go-strict PARSE-REJECT hypothesis).

### 2.8 Listener-level fields: All 3 proto-faithful *(Decision #7 → ADR-0149)*

Per Q8 = "All 3 proto-faithful", phase 17 honors all 3 auxiliary listener-level fields:
- `rules` (repeated RequirementRule) — listener-level trigger map. Each rule's `match` is a `RouteMatch` (reuses phase-04 RouteMatch evaluator — zero new framework delta for the match side); `requirement_name` resolves via `requirement_map` at boot. Dispatch order: first-matching rule wins (per Envoy semantic). Per-route override (if set) supersedes listener-level rules per ADR-0073 wholesale-override discipline.
- `bypass_cors_preflight` (bool) — when true, OPTIONS requests bypass JWT validation entirely (`bypassed_cors_preflight` counter increments). When false (default), OPTIONS requests are validated like any other request.
- `strip_failure_response` (bool) — when true, the 401 response body AND `www-authenticate` header are both stripped (the 401 wire-bytes become the 4-header-only set with no `www-authenticate`; §10 pin §17.P3 confirms exact effect). When false (default), the 401 response carries the full failure-reason body + WWW-Authenticate challenge.

Deprecated `RequirementRule.requires` (inline JwtRequirement) field PARSE-REJECT envoy-go-only with envoy-go-only error per §8 deferral 9 (Envoy v1.37.2 honors with deprecation warning — envoy-go-strict divergence-window).

### 2.9 Stat surface anchor + per-route SHARED hypothesis *(Decision #8 → ADR-0154)*

Per §5 below + §10 pin §17.P6/P7/P8 confirms. Hypothesis: 8 new base stats (2 per-provider counters scaling with provider count + 5 per-provider JWKS counters + 1 filter-wide bypass counter) + HCM-rooted SN2-reuse namespace `http.<HCM_stat_prefix>.jwt_authn.<provider>.*` + per-route stats SHARED with listener-level (pure-delegation per-route does NOT spawn new policy-evaluation state). The per-provider counter family scales linearly with provider count (analogous to phase-16's per-policy scaling under `track_per_rule_stats: true`); operator config-time foot-gun: many-provider configs emit many-named-counters. ADR-0154 anchors the stat-surface; §10 pin §17.P6 verifies exact Envoy stat names against reference v1.37.2 at SPEC-time.

### 2.10 Deny-path wire shape — 401 + WWW-Authenticate *(Decision #9 → ADR-0155)*

Per RFC 6750 401-challenge semantics, phase 17 emits `SendLocalReply(401, "<failure-reason body>", {Content-Type: text/plain, WWW-Authenticate: Bearer realm="<issuer>"})` on validation failure. The 401 status code DIVERGES from prior §9 rows' 403 (csrf/rbac) and 429 (local_ratelimit) — 401 is the semantically-correct status for unauthenticated/missing-credentials; 403 would imply authenticated-but-unauthorized. The body byte-form varies by failure-reason; §10 pin §17.P1 enumerates the canonical set (5+ bodies expected: `Jwt is missing`, `Jwt verification fails`, `Jwt issuer is not configured`, `Jwt expired`, `Audiences in Jwt are not allowed`). The `www-authenticate: Bearer realm="<issuer>"` header is RFC 6750 standard; §10 pin §17.P2 confirms exact format (with or without `error=` parameter). `strip_failure_response=true` (§10 pin §17.P3) strips both the body AND the WWW-Authenticate header — the 401 wire-bytes become the 4-header-only set. ADR-0155 anchors the deny-path wire shape + the strip_failure_response divergence-window.

---

## 3. Framework-survey result — TWO new primitives

Phase 17 introduces TWO new framework primitives, both designed cross-phase-reusable at introduction time per phase-16's framework-primitive-introduction discipline (ADR-0142 + ADR-0144). The framework survey evaluated reuse of phase-13/14/15/16 primitives BEFORE proposing new — per phase-16 §10 lesson (a) + lesson (d). Findings:

- **Phase-09 `time.AfterFunc` + `cb.ContinueDecoding/Encoding` async-resume primitives**: NOT reused (jwt_authn's runtime validation is synchronous within DecodeHeaders; JWKS fetch happens in a background goroutine OR at boot, not in-line).
- **Phase-11 token-bucket primitive**: NOT reused (no rate-limiting at jwt_authn).
- **Phase-13 ADR-0128 decode-side body-buffering primitives**: NOT reused (jwt_authn is header-only; no body access needed).
- **Phase-14 ADR-0131 EncoderFilterCallbacks.OverwriteBody**: NOT reused (jwt_authn is decode-side only).
- **Phase-16 ADR-0142 matcher-engine at `internal/matcher/`**: NOT reused at phase-17 MVP (jwt_authn's `RequirementRule.match` is `RouteMatch`, not xds.type.matcher.v3.Matcher; future jwt_authn extensions to matcher-tree may reuse ADR-0142).
- **Phase-16 ADR-0144 TLS-principal accessor `DownstreamPrincipal()`**: NOT reused at phase-17 MVP (jwt_authn extracts principal from JWT payload, not from TLS cert; future jwt_authn extensions to mTLS-bound JWT validation will reuse ADR-0144).
- **Phase-04 RouteMatch evaluator** (existing HCM router primitive): **REUSED** for listener-level `rules[].match` evaluation — zero new framework delta on the match side.
- **ADR-0125 7 canonical per-route patterns**: NEW 8th canonical needed (§2.7 + §4).

**Zero-delta is NOT feasible** for phase 17 — the RemoteJwks proto-faithful Q2 pick requires the HTTP-outbound primitive, and the JWT signature-verification step requires the JWT verifier primitive. Both primitives are clean cross-phase-reusable lifts.

### 3.1 HTTP-outbound primitive — `internal/jwks/` package *(ADR-0150)*

A new top-level Go package `internal/jwks/` (location TBD at SPEC time; could also live at `internal/httpclient/` if the SPEC author judges the JWKS-specific scope too narrow). The package exports:

- `jwks.New(uri string, cacheDuration time.Duration, asyncFetch *RemoteJwks_AsyncFetch) (*Fetcher, error)` — constructs a fetcher for a given JWKS URI with cache duration + async-fetch policy. If async-fetch enabled, spawns a background goroutine for periodic refresh.
- `jwks.Fetcher.Get(ctx context.Context) (*JWKSet, error)` — returns the current cached JWKS, fetching if cache missed or stale. Blocks-on-fetch on first call (cache empty); subsequent calls return cached value while a background refresh runs.
- `jwks.JWKSet` opaque type — wraps the parsed JWK Set (`[]JWK`); exposes `Lookup(kid, alg string) (PublicKey, error)` for resolution by `kid` claim + algorithm.
- Retry/backoff policy: exponential 1s/2s/4s/8s/16s/30s cap (canonical; per §10 pin §17.P4 SPEC-time scrape may refine).

Cross-phase reuse intent (codified at ADR-0150 §Decision): future filters consuming outbound-HTTP-from-filter primitives reuse the same `Fetcher` pattern + extend with their own response-parsing semantics. The strategic reuse targets are: future `ext_authz` HTTP-mode (sends auth check to external HTTP service; reuses the cache-and-fetch lifecycle for connection pooling); future `oauth2` filter (fetches access tokens from token endpoint; reuses the cache-and-refresh discipline for token caching); future generic outbound-HTTP-needing filters.

The package lives OUTSIDE `internal/filter/` (mirroring phase-16's `internal/matcher/`) explicitly to anchor cross-phase reusability. The BEHAVIOR_CONTRACT §13.7 NEW top-level `## JWKS framework primitive` umbrella anchors operator-facing semantics; future filters extend the umbrella additively.

### 3.2 JWT verifier primitive — `internal/jwt/` package *(ADR-0151)*

A new top-level Go package `internal/jwt/`. The package exports:

- `jwt.Parse(raw string) (*Token, error)` — parses the 3-part JWT structure (header.payload.signature); validates JSON-encoded header + payload; rejects malformed tokens.
- `jwt.Token.VerifySignature(key crypto.PublicKey, alg string) error` — verifies signature using the named algorithm against the provided public key; PARSE-REJECT algorithms outside the RS+ES allow-list.
- `jwt.Token.ValidateClaims(opts ValidateOptions) error` — checks `exp`, `nbf`, `iat`, `iss`, `aud` per provider config; rejects with the appropriate `Jwt expired` / `Jwt not yet valid` / `Jwt issuer is not configured` / `Audiences in Jwt are not allowed` failure-reason.
- `jwt.Token.Payload() map[string]interface{}` — returns the decoded payload for claim_to_headers + forward_payload_header.

Cross-phase reuse intent (codified at ADR-0151 §Decision): future filters consuming JWT semantics (e.g., a hypothetical `jwt_claim_router` filter routing on claim values, or oauth2's token validation step) reuse `jwt.Parse` + `Token.VerifySignature` + `Token.ValidateClaims` directly. The package is algorithm-agnostic for parsing + signature verification (the algorithm allow-list is checked at signature verification time); claim validation is policy-driven via `ValidateOptions`.

The package lives OUTSIDE `internal/filter/` (mirroring phase-16's `internal/matcher/`) explicitly to anchor cross-phase reusability. The BEHAVIOR_CONTRACT §13.8 NEW top-level `## JWT verifier framework primitive` umbrella anchors operator-facing semantics.

---

## 4. Per-route shape — NEW 8th canonical

The ADR-0125 canonical per-route discipline roster after phase 16 has 7 entries:
1. cors no-per-route
2. fault / local_ratelimit / csrf data-only TPFC
3. header_mutation multi-tier all-tier
4. local_ratelimit INDEPENDENT-stats stateful
5. buffer / compressor disabled-OR-override-bool-in-oneof
6. bandwidth_limit bare-message-via-TPFC + code-level-required
7. rbac wrapper-with-reserved-field-and-single-optional-sub-message absent-implies-disabled-OR-wholesale-override

Phase 17 introduces the **8th canonical**: `PerRouteConfig{oneof{requirement_name(string) | disabled(Empty)}}` with reference-by-name delegation into listener-level `JwtAuthentication.requirement_map`. ADR-0125 gains in-place amendment §(xiii). Defining features distinct from all 7 prior canonicals:

- **vs 1st (cors no-per-route)**: 8th has explicit per-route surface.
- **vs 2nd (data-only TPFC)**: 8th has structural oneof; 2nd is bare-message.
- **vs 3rd (multi-tier all-tier)**: 8th uses 3-tier resolution (Route > VirtualHost > RouteConfig); 3rd evaluates ALL tiers (not just most-specific).
- **vs 4th (INDEPENDENT-stats stateful)**: 8th has SHARED stats (delegation-by-name spawns no new state).
- **vs 5th (disabled-bool + wholesale-override sub-message)**: 8th's disabled arm is Empty (not bool) AND the "override" arm is a STRING REFERENCE (not a sub-message); 5th's both arms are local (bool + local message).
- **vs 6th (bare-message-via-TPFC + code-level-required)**: 8th uses oneof wrapper; 6th uses bare TPFC.
- **vs 7th (wrapper-with-reserved-field + single optional sub-message; absent-implies-disabled)**: 8th explicitly distinguishes "disabled" via Empty vs "delegate via name"; 7th distinguishes "disabled" via absence vs "wholesale-override" via presence.

The 8th canonical's defining feature is the **string-reference-delegation** into a separately-named registry — per-route does NOT carry its own config; it references-by-name into the listener-level requirement_map. This is genuinely new and warrants its own canonical entry.

ADR-0125 amendment §(xiii) lands at SPEC commit time (per phase-13 §(ix) + phase-14 §(x) + phase-15 §(xi) + phase-16 §(xii) in-place-update precedent).

---

## 5. Stat surface hypothesis

Per §2.9 + Decision #8 → ADR-0154 anchor. Phase 17 grows the stat-table from 64 names (post-phase-16) to ~72 names minimum:

**Per-provider counters** (scales with provider count; `<provider>` = JwtAuthentication.providers map key):
- `jwt_authn.<provider>.jwt_authn_success` — counter
- `jwt_authn.<provider>.jwt_authn_failed` — counter

**Per-provider JWKS counters** (one set per `RemoteJwks` provider; absent for LocalJwks-only providers):
- `<provider>.jwks_fetch_success` — counter
- `<provider>.jwks_fetch_failed` — counter
- `<provider>.jwks_cache_hit` — counter
- `<provider>.jwks_cache_miss` — counter
- `<provider>.jwks_fetch_in_progress` — gauge

**Filter-wide counter** (per HCM stat_prefix):
- `bypassed_cors_preflight` — counter

**Namespace anchor**: HCM-rooted `http.<HCM_stat_prefix>.jwt_authn.<provider>.<counter>` (SN2 reuse hypothesis — the existing HCM-stat-prefix Prometheus tag-extractor handles this verbatim; NO new SN10 rule needed). §10 pin §17.P7 RATIFIED-PENDING — SPEC-time empirical scrape against reference Envoy v1.37.2 confirms or refines.

**Per-route stats discipline**: SHARED with listener-level (§10 pin §17.P8 confirms). Rationale: per-route is pure delegation; spawns no new policy-evaluation state. DIVERGES from phase-11/15/16 INDEPENDENT-stats; MIRRORS phase-12/13/14 SHARED-stats.

**Stat surface count summary table**:
| Phase | Filter | Stat surface delta |
|---|---|---|
| 11 | local_ratelimit | 22 → 26 (+4) |
| 12 | csrf | 26 → 29 (+3) |
| 13 | buffer | 29 → 29 (+0; zero stat extension) |
| 14 | compressor | 29 → 46 (+17) |
| 15 | bandwidth_limit | 46 → 60 (+14) |
| 16 | rbac | 60 → 64 (+4 base; per-policy scales) |
| **17** | **jwt_authn** | **64 → ~72 (+8 base; per-provider scales)** |

---

## 6. Differential fixture envelope — `0019-http-jwt-authn`

8 scenarios anticipated (matches phase-16's 8-scenario envelope; equivalence pattern mirrors phase-13/14/15/16):

| # | Scenario | Provider config | Token | Expected disposition | Counter delta assertion |
|---|---|---|---|---|---|
| 1 | valid-token-allow-RS256-RemoteJwks | RemoteJwks; RS256; iss=`issuer-a`; aud=`api-a` | RS256-signed; valid claims | 200 backend echo | `jwt_authn_success=1` + `jwks_fetch_success=1` |
| 2 | valid-token-allow-ES256-LocalJwks | LocalJwks; ES256; iss=`issuer-b`; aud=`api-b` | ES256-signed; valid claims | 200 backend echo | `jwt_authn_success=1` (no JWKS fetch) |
| 3 | missing-token-deny | RemoteJwks; RS256 | (no Authorization header; no extraction match) | 401 + body byte-exact `Jwt is missing` + `www-authenticate` header | `jwt_authn_failed=1` |
| 4 | expired-token-deny | RemoteJwks; RS256 | RS256-signed; exp=past | 401 + body byte-exact `Jwt expired` | `jwt_authn_failed=1` |
| 5 | bad-signature-deny | RemoteJwks; RS256 | RS256-signed but with tampered signature | 401 + body byte-exact `Jwt verification fails` | `jwt_authn_failed=1` |
| 6 | bypass-cors-preflight | RemoteJwks; RS256; `bypass_cors_preflight=true` | OPTIONS preflight request (no Authorization) | 200 backend echo | `bypassed_cors_preflight=1` (no `jwt_authn_*` increment) |
| 7 | per-route 8th-canonical delegation | RemoteJwks listener + per-route `requirement_name: "alt-req"` | RS256-signed for `alt-req`'s provider | 200 backend echo | `jwt_authn_success=1` (alt-req provider) |
| 8 | per-route 8th-canonical disabled | RemoteJwks listener + per-route `disabled: {}` | (no token) | 200 backend echo (route bypasses validation) | (no counter increments) |

Optional 9th + 10th scenarios at SPEC reshape time:
- forward_payload_header + claim_to_headers backend-arrival assertion (verify the upstream backend receives the named headers with correct payload/claim values)
- requires_any multi-provider OR scenario (rule with `requires_any: [provider_name: "a", provider_name: "b"]`; token valid against provider b)

**PKI generation**: reuses phase-16's `pki/gen.go` pattern. One RSA keypair (RS256 signing) + one ECDSA P-256 keypair (ES256 signing); one JWK Set served via test-helper JWKS server (in-process HTTP server) for RemoteJwks scenarios; inline JWKs for LocalJwks scenarios.

**Test-helper JWKS server**: new under `test/helpers/jwksbackend/` (or similar location-TBD-at-SPEC). Serves a configurable JWK Set at a known URI; both `envoy.yaml` and `envoy-go.yaml` wire to the same JWKS URI. Lifecycle: spawn-per-fixture; tear-down at fixture-done.

**21st fuzzer**: `FuzzJwtAuthnConfigParse` at `internal/filter/http/jwtauthn/fuzz_test.go`. Corpus seeds 0-12 anticipated (each-decision + boundary cases). 30s/seed @ 12-seed corpus = ~360s wallclock; under the existing fuzzer-time-budget envelope.

---

## 7. Anticipated ADRs — 8 ADRs + ADR-0125 §(xiii) amendment paragraph

ADR-0147 is the highest-numbered ADR landed in phase 16; ADR-0148 is the next-free per STATE.md.next-free. Phase 17 anticipates 8 ADRs (ADR-0148..ADR-0155). The 8-ADR roster ties the LARGEST §9 anticipated roster to date (phase-16 was 7-anticipated + 1-unanticipated = 8 landed; phase-17 hypothesizes 8 anticipated + 0-or-1 unanticipated per the ADR-0044 escape-valve precedent).

**ADR-0148** — `internal/filter/http/jwtauthn/` package shape + DECODER-only HTTPFilter + 8-stat filterStats + lazy per-provider counter allocation via `NewCounterIfAbsent` post-Freeze + deny-path wire shape `SendLocalReply(401, "<failure-reason>", {Content-Type: text/plain, WWW-Authenticate: Bearer realm="<issuer>"})` + boot-registration alphabetical-after-header_mutation. **Lands-in: Task 2.** Anchors §2.1 + §2.10.

**ADR-0149** — `compiledConfig` 5-field decomposition + 13-field JwtProvider parse + 4-deferred dynamic-metadata silent-ignore discipline + 6-variant JwtRequirement evaluator + algorithm allow-list discipline + side-effect emit-order + listener-level rules dispatch + envoy-go-strict PARSE-REJECT for deprecated `RequirementRule.requires` field. **Lands-in: Task 2.** Anchors §2.4 + §2.6 + §2.8.

**ADR-0150** — HTTP-outbound primitive at NEW `internal/jwks/` (or `internal/httpclient/` — location TBD at SPEC) package — cross-phase-reusable; async fetch + LRU cache + scheduled refresh + retry/backoff policy. **Lands-in: Task 3.** Anchors §3.1.

**ADR-0151** — JWT verifier primitive at NEW `internal/jwt/` package — RS+ES family; pure Go stdlib (`crypto/rsa.VerifyPKCS1v15` + `crypto/ecdsa.Verify`); cross-phase-reusable. **Lands-in: Task 4.** Anchors §3.2.

**ADR-0152** — Token extraction across all 4 sources (Authorization Bearer + from_headers + from_params + from_cookies) + iteration order + first-match-wins + empty-extraction = "JWT missing" failure-reason. **Lands-in: Task 5.** Anchors §2.5.

**ADR-0153** — Per-route 8th canonical: oneof{requirement_name(string) | disabled(Empty)} + delegation via listener-level requirement_map + dangling-reference PARSE-REJECT at config-load-time + per-route stats SHARED with listener-level. ADR-0125 §(xiii) amendment paragraph (NEW 8th canonical entry — ADR-0125's roster grows from 7 to 8). **Lands-in: Task 7.** Anchors §2.7 + §4.

**ADR-0154** — Stat surface (~8 new counters/gauges; per-provider scaling; HCM-rooted SN2-reuse hypothesis with §10 pin §17.P7 SPEC-time scrape confirmation). **Lands-in: Task 8.** Anchors §2.9 + §5.

**ADR-0155** — Deny-path wire shape — 401 status + WWW-Authenticate Bearer challenge header + 5+ failure-reason body variants + `strip_failure_response` divergence-window + RFC 6750 conformance. **Lands-in: Task 9.** Anchors §2.10.

ADR-0044 escape-valve held in reserve for ~1 impl-time-unanticipated ADR per phase as working estimate (phase-13 ADR-0127-v2 + phase-14 ADR-0134 + phase-16 ADR-0147 precedent).

---

## 8. Deferred items (~13 items; comparable to phase-16's 15-item list)

For future phase consideration (none are blockers for closing row 17 phase-done; all auditable in the ADR-0040 deferral trail):

1. **`payload_in_metadata` (JwtProvider field)** — encodes JWT payload as dynamic metadata under the named key. DEFERRED: couples to dynamic-metadata family (same family blocked at phase-16 forward-pointer item 9). Future dynamic-metadata-family phase lands `(FilterCallbacks).SetDynamicMetadata(key, value)` primitive; jwt_authn re-enables this field at that point.

2. **`header_in_metadata` (JwtProvider field)** — encodes JWT header as dynamic metadata. DEFERRED: couples to dynamic-metadata family (same as item 1).

3. **`failed_status_in_metadata` (JwtProvider field)** — encodes failure status as dynamic metadata on rejection. DEFERRED: couples to dynamic-metadata family.

4. **`normalize_payload_in_metadata` (JwtProvider field)** — sub-message controlling payload normalization shape for metadata emission. DEFERRED: couples to dynamic-metadata family + payload_in_metadata coupling chain.

5. **HS family algorithms (HS256/HS384/HS512)** — DEFERRED: requires symmetric-secret config plumbing in JwtProvider (security-sensitive — operators must securely provision shared secrets). Future algorithm-extension phase enables.

6. **EdDSA algorithm (Ed25519 / Ed448)** — DEFERRED: less-common; requires Go stdlib `crypto/ed25519`. Could enable as a standalone follow-on.

7. **`none` algorithm** — DEFERRED-PERMANENTLY (intentionally never enabled; security-sensitive — `alg=none` JWTs are unsigned; allowing them defeats authentication). PARSE-REJECT at JWK parse + runtime-reject at token parse; no operator-config knob to enable.

8. **`jwt_cache_config` (JwtProvider field)** — validated-JWT result LRU cache; cache-hit speedup for high-RPS deployments. envoy-go MVP no-cache (each request re-validates); foot-gun for high-RPS. DEFERRED: couples to a future caching-framework phase that introduces a generic LRU primitive (jwt_authn would reuse alongside ext_authz response cache + oauth2 token cache).

9. **Deprecated `RequirementRule.requires` field** (inline JwtRequirement) — DEFERRED: envoy-go-strict PARSE-REJECT diverges from Envoy v1.37.2's honor-with-deprecation-warning. Future Envoy releases will remove `requires` entirely (per upstream deprecation timeline); envoy-go's strict-from-day-one stance avoids migration pain. Documented at BEHAVIOR_CONTRACT phase-17 forward-pointer notes.

10. **JWKS bounded retry/backoff customization** — operator-config knob to override the canonical exponential 1s/2s/4s/8s/16s/30s-cap policy. DEFERRED: MVP picks one policy. Future operator-ergonomics phase MAY add a customization surface.

11. **JWKS cache-invalidation hooks** (cache-bust on operator signal; e.g., admin-API endpoint `POST /jwt_authn/<provider>/refresh_jwks`). DEFERRED: couples to admin-API extension.

12. **Access-log integration** for jwt_authn success/failure log fields (per-request log line emits `%JWT_PROVIDER%` / `%JWT_SUBJECT%` / `%JWT_FAILURE_REASON%` access-log formatters). DEFERRED: couples to phase-16 forward-pointer access-log item 7 + access-log formatter extension framework.

13. **CEL-based dynamic provider selection** — runtime CEL expression evaluating against request attributes to pick provider OR requirement at evaluation time. DEFERRED: couples to phase-16 CEL deferral item 10 + future CEL framework phase landing `internal/cel/` + `cel-go` dependency.

14. **JwtRequirement Set recursion-depth foot-gun** — no parse-time depth-cap on `requires_any` / `requires_all` recursion; mirrors phase-16 §11.P11 RATIFIED-VIA-ABSENCE. Documented as forward-pointer; future operator-ergonomics phase MAY add an envoy-go-only depth-cap.

---

## 9. Cross-references against phase-16 deferred-items list — forward-pointer pickup

Phase-16 REVIEW.md §9 enumerates 15 deferred items. Phase-17 evaluates which opportunistically close vs continue deferred:

- **Phase-16 item 7 — shadow access-log integration**: NO PICKUP (jwt_authn has no shadow surface analogous to rbac's shadow_rules; the parallel concept in jwt_authn would be a hypothetical `shadow_providers` or `shadow_requirements`, which is NOT part of the Envoy proto).
- **Phase-16 item 8 — `response_code_details` framework primitive**: POTENTIAL PICKUP — jwt_authn's 5+ failure-reason bodies map cleanly to `response_code_details` semantic (Envoy v1.37.2 emits `jwt_authn_status{<reason>}` as response_code_details). Decision deferred to SPEC time; if not picked up at phase 17, remains on phase-16's forward-pointer list. If picked up, would close BOTH phase-16 item 8 AND phase-17 deferred item-implicit (response_code_details propagation).
- **Phase-16 item 9 — LOG-action `access_log_hint` dynamic-metadata primitive**: NO PICKUP (jwt_authn has no LOG-action analog; the dynamic-metadata family is uniformly deferred per phase-17 deferrals 1-4).
- **Phase-16 item 10 — CEL three-field condition evaluation**: NO PICKUP at MVP (jwt_authn doesn't have CEL coupling at MVP per Q3+Q4 picks). Future jwt_authn extension to CEL-based provider/requirement selection (per-phase-17 deferral 13) would close this.
- **Phase-16 item 11 — Principal_Custom v1.32.4 binding-absent workaround**: NO PICKUP (jwt_authn doesn't consume rbac's Principal_Custom).
- **Phase-16 items 1-6, 12-15**: NO PICKUP (rbac-specific concerns + tech-debt cleanup + SPEC §13.2 housekeeping — none structurally close in phase 17).

**Forward-pointer net change for phase 17**: 0-1 closures expected (depending on SPEC-time decision on item 8 response_code_details). Phase 17 adds 13 new deferred items (§8 above) + extends the dynamic-metadata-family deferred-cluster (items 1-4) which already includes phase-16 items.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

Per phase-09..16 cadence (~5-18 questions; SPEC §11 empirical pins resolve them via reference Envoy v1.37.2 scrape per ADR-0004). Phase 17 anticipates 16 pins:

- **§17.P1 — Exact 401 body byte-form per failure-reason**: 5+ canonical bodies anticipated (`Jwt is missing`, `Jwt verification fails`, `Jwt issuer is not configured`, `Jwt expired`, `Audiences in Jwt are not allowed`). Confirms exact ASCII byte-form + LF-terminator handling. **SPEC-time scrape Task 8.**
- **§17.P2 — WWW-Authenticate header format**: hypothesizes `Bearer realm="<issuer>"` — confirms whether Envoy emits an `error=` parameter (e.g., `Bearer realm="x", error="invalid_token"`) per failure-reason.
- **§17.P3 — `strip_failure_response=true` wire-bytes effect**: confirms whether the stripped 401 has full 4-header set (just no `www-authenticate` and empty body) OR a different shape.
- **§17.P4 — RemoteJwks async-fetch retry/backoff schedule**: confirms Envoy's exponential backoff defaults vs configurable.
- **§17.P5 — JWKS cache TTL + refresh interval defaults**: confirms Envoy's default cache_duration value + refresh-relative-to-TTL discipline.
- **§17.P6 — Stat name table + counter dispositions**: confirms exact Envoy stat names + scope + counter-vs-gauge for the per-provider counters + JWKS counters + bypass counter. **SPEC-time scrape Task 8** (the canonical RATIFIED-PENDING pin closure point per phase-16 §10 lesson (c)).
- **§17.P7 — Prometheus tag-extractor + namespace flattening**: SN2-reuse-vs-new-SN-rule hypothesis; **SPEC-time scrape Task 8** RATIFIED-PENDING closure.
- **§17.P8 — Per-route stat SHARED-vs-INDEPENDENT**: hypothesis is SHARED (per-route is pure delegation; no new stateful surface). **SPEC-time scrape Task 8** RATIFIED-PENDING closure.
- **§17.P9 — PGV-required fields per JwtProvider + JwtAuthentication + RequirementRule**: confirms which fields are PGV-required (`issuer`?) vs optional vs envoy-go-defensive-mirror.
- **§17.P10 — `claim_to_headers` exact behavior**: confirms claim_name dot-notation (nested-claim lookup)? array-claim handling (multi-valued claim emitted as repeated header? joined with comma? rejected?)? non-string-claim coercion (number/boolean/object claims)?
- **§17.P11 — `jwt_cache_config` default values**: would inform MVP no-cache vs minimal-cache decision (deferral 8 may be revisited if defaults are conservative).
- **§17.P12 — Per-route `requirement_name` dangling reference handling**: Envoy parse-reject vs runtime-reject the per-route TPFC parse when `requirement_name` is not in listener-level `requirement_map`? envoy-go hypothesis: PARSE-REJECT at config-load.
- **§17.P13 — `pad_forward_payload_header` bool semantics**: confirms whether `=` padding is stripped (false) or preserved (true) in the base64-encoded payload header value.
- **§17.P14 — `from_params` resolver**: case-sensitivity of parameter-name match? URL-decode discipline? array-param handling (`?token=a&token=b`)?
- **§17.P15 — `from_cookies` resolver**: case-sensitivity of cookie-name match (per RFC 6265 cookie names ARE case-sensitive)? cookie-value parsing rules (just the value verbatim? URL-decode?)?
- **§17.P16 — `allow_missing` + `allow_missing_or_failed` exact dispositions**: hypothesis: allow_missing = JWT-absent OK / JWT-present-and-invalid FAIL; allow_missing_or_failed = both OK. Confirms vs Envoy semantic + interaction with extraction-source iteration (does "missing" mean no-extraction-source-matched? OR Authorization-Bearer-absent specifically?).

Anticipated SPEC §11 scrape time: ~3-5 hours (16 pins; phase-16 was 18 pins resolved in similar wallclock). Most pins resolved IN-SESSION at SPEC drafting; some (P4/P5/P6/P7/P8 — JWKS lifecycle + stat surface) RATIFIED-PENDING and closed at PLAN Task 3/8 impl-time empirical scrape per phase-16 §10 lesson (c).

---

## 11. Phase-16 §10 lessons applied

Per the explicit lessons-learned section of phase-16 REVIEW.md §10:

**Lesson (a) — TWO new framework primitives in a single phase as a structural data point.** Phase 17 MIRRORS this — TWO new primitives (HTTP-outbound + JWT verifier), both designed cross-phase-reusable at introduction time. The HTTP-outbound primitive is genuinely cross-cutting (strategic for the entire auth-filter family); the JWT verifier is reusable by any JWT-consuming filter. Both ADRs (ADR-0150 + ADR-0151) anchor cross-phase reuse intent explicitly in §Decision body per phase-16 ADR-0142 + ADR-0144 precedent.

**Lesson (b) — ADR-0044 escape-valve as a working estimate of ~1 impl-time-unanticipated ADR per phase.** Phase-17 budgets 0-or-1 unanticipated ADR per ADR-0044's standard discipline. Most likely surfaces: (i) JWKS-fetch failure-mode unanticipated structural lift (e.g., fetch-timeout-during-startup blocks listener-bind); (ii) HTTP-outbound primitive's TLS-config plumbing might require ADR-lift for trust-store coupling. Held in reserve.

**Lesson (c) — Task-8 empirical scrape as canonical RATIFIED-PENDING pin closure mechanism.** Phase-17 explicitly assigns Task 8 (stat-surface scrape) as the closure point for pins §17.P6/P7/P8 (stat surface + Prometheus tag-extractor + per-route SHARED-vs-INDEPENDENT). Per phase-16 §10 lesson (c) — empirical scrape against reference Envoy v1.37.2 at Task 8 is the load-bearing closure point.

**Lesson (d) — ADR-0125 canonical-pattern roster grows linearly per phase.** Phase 13 contributed the 5th canonical; phase 14 amended; phase 15 contributed the 6th; phase 16 contributed the 7th; **phase 17 contributes the 8th canonical** (string-reference-delegation via requirement_map). The §(xiii) amendment paragraph mirrors phase-13 §(ix) + phase-14 §(x) + phase-15 §(xi) + phase-16 §(xii) in-place-update precedent.

**Lesson (e) — §1.1 amendment-block channel scales linearly with §11 empirical-pin depth.** Phase 17 anticipates 16 §11 pins (comparable to phase-16's 18); SPEC-time §1.1 amendment count anticipated 10-15 amendments per the linear-scale rule. The §1.1 channel scales without operational friction; no BRAINSTORM §12 amendment cycle anticipated.

**Phase-13/14/16 ADR-0044 escape-valve precedent**: phase-13 surfaced ADR-0127 v2 at Task 12 (body-counting algorithm refutation); phase-14 surfaced ADR-0134 at Task 14 follow-up (HCM directResponseAction header plumbing); phase-16 surfaced ADR-0147 at Task 13 follow-up (TLS-layer mTLS-lift). Phase-17 budgets ONE such lift as working estimate; likely surfaces around JWKS-fetch-failure structural concerns OR HTTP-outbound TLS-trust-store plumbing.

---

## 12. Section closeout

Phase 17 brainstorm complete. Lifecycle exit: state 0 → 1 per ADR-0005 (BRAINSTORM.md authored; SPEC pending). Next session: a SPEC session (`superpowers:brainstorming` SCOPED to SPEC authoring per phase-09..16 cadence) authoring `docs/envoy-go/phases/17-http-filter-jwt-authn/SPEC.md`.

The SPEC session's defining IN-SESSION obligations:
1. **Resolve all 16 §11 empirical pins** (§10 above) via reference Envoy v1.37.2 scrape per ADR-0004.
2. **Land §1.1 amendment-block channel entries** for each pin's RATIFIED / REFUTED / PARTIAL / REFINED disposition; estimated 10-15 amendments per phase-16 §10 lesson (e).
3. **Anchor the 8 anticipated ADRs** (ADR-0148..ADR-0155) per §7 above with §Context drafts.
4. **Author ADR-0125 §(xiii) amendment paragraph** codifying the 8th canonical.
5. **Define the 8-scenario differential fixture** with per-scenario expectations YAML.
6. **Confirm or refute the ADR-0045 split-by-surface release-valve decision** per §1.4 — SPEC-time call.
7. **Assign Task-8 RATIFIED-PENDING pin closure** for §17.P6/P7/P8 per phase-16 §10 lesson (c).

Next-skill (post-SPEC session): `superpowers:writing-plans` for the phase-17 PLAN.md authoring session (lifecycle state 2 → 3).
