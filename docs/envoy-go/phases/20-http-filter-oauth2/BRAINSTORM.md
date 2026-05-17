# Phase 20 Brainstorm — `envoy.filters.http.oauth2`

**Status:** brainstorm complete (lifecycle-state 0 → 1). This document captures the design decisions reached during the brainstorm session for phase 20 (`http-filter-oauth2`), the THIRTEENTH concrete phase under `BOOTSTRAP_PROMPT.md` §9's HTTP filters family (after `cors` at phase 07.1, `fault` at phase 09, `header_mutation` at phase 10, `local_ratelimit` at phase 11, `csrf` at phase 12, `buffer` at phase 13, `compressor` at phase 14, `bandwidth_limit` at phase 15, `rbac` at phase 16, `jwt_authn` at phase 17, `ext_authz` at phase 18 with its ADR-0045 18.1+18.2 split, and `ext_proc` at phase 19 with its ADR-0045 19.1+19.2 split). The next session (lifecycle-state 1 → 2 for phase 20, skill `superpowers:brainstorming` per ADR-0005 scoped to SPEC authoring per the phase 09/10/11/12/13/14/15/16/17/18/19 precedent) authors `docs/envoy-go/phases/20-http-filter-oauth2/SPEC.md` based on this brainstorm — that SPEC is also responsible for executing the §10 empirical-pin obligations IN-SESSION against reference Envoy v1.37.2 per ADR-0004.

**Brainstorm session:** worktree `.worktrees/phase-20-http-filter-oauth2-brainstorm`, branch `phase-20-http-filter-oauth2-brainstorm`, branched from master tip `c2c0f27` (the phase 19.2 IMPL follow-up STATE.md SHA-fill commit — `phase 19.2 IMPL follow-up: STATE.md SHA-fill (TBD → 1ddb661 post-squash)`). The phase 19.2 squash-merge commit `1ddb661` and its SHA-fill follow-up `c2c0f27` are the immediate predecessors on master. `c2c0f27` is the current master tip. Parent phase 19 and sub-phases 19.1 + 19.2 are BOTH `done` at this commit per parent SPEC §8 parent-rollup discipline (ADR-0045 split fully closed). NO row 20 exists on ROADMAP yet — this brainstorm registers it at lifecycle-state 0 → 1 (placed in the `## Feature Families (09+)` table after row 19.2) ahead of the phase-09..19.2 ROADMAP-row-add convention (rows traditionally added at SPEC time, but phase 20 lands the row at BRAINSTORM time per the explicit user request to register the in-progress §9 family-row continuation immediately for grep-traceability).

**Brainstorm mode:** interactive with a live human. The user picked filter selection + each major design decision via a 10-question dialogue (Q1 MVP envelope — Standard OAuth 2.0 sign-in + refresh + sign-out chosen from {Minimum-sign-in-only / Standard / Full-OIDC-with-PKCE}; Q2 `internal/httpclient/` generalization trigger — EXTRACT NOW chosen from {EXTRACT NOW (third outbound-HTTP consumer per ADR-0159 forward-pointer) / DEFER (fourth-consumer trigger)}; Q3 SDS posture — filesystem-path SDS only chosen from {filesystem-only / filesystem+api_config_source / full-SDS-with-ADS}; Q4 token-encryption envelope — Proto-faithful AES-GCM default-on chosen from {AES-GCM-always / AES-GCM-default-on-with-disable-flag / Plaintext-only}; Q5 per-route discipline — No per-route support listener-scoped-only chosen from {listener-scoped-only-per-proto / synthesize-per-route-anyway / new-9th-canonical}; Q6 ADR-0045 split-readiness leaning — MEDIUM-HIGH chosen from {LOW / MODERATE / MEDIUM-HIGH / HIGH}; Q7 filesystem-SDS location — NEW `internal/sdsfile/` shared package chosen from {filter-local / NEW shared `internal/sdsfile/` / fold into existing internal package}; Q8 SDS reload posture — fsnotify-based hot-reload chosen from {load-once-no-reload / polling-reload / fsnotify-hot-reload}; Q9 cookie HMAC scheme — HMAC-SHA256(hmac_secret, host + BearerToken + OauthExpires) chosen from {Envoy-default-composition / minimal-composition / custom-composition}; Q10 token-endpoint POST body format under URL_ENCODED_BODY — `client_secret_post` in body chosen from {client_secret_post / client_secret_basic (BASIC_AUTH; out of envelope per Q1)}). The §9 family-row continuation is implicit per ADR-0106. Decisions that did not require live input are self-answered against `BOOTSTRAP_PROMPT.md`, `MISSION.md`, `ROADMAP.md`, `BEHAVIOR_CONTRACT.md`, prior ADRs (ADR-0001 through ADR-0176, where ADR-0167..ADR-0176 landed in the phase-19 family), and the just-shipped phase 19.2 + phase 19.1 + phase 19 + phase 18.2 artefacts. Empirical pins requiring scrape evidence against Envoy v1.37.2 are explicitly enumerated in §10 and deferred to SPEC-drafting time per the phase 09–19.2 precedent.

**Document shape:** mirrors `docs/envoy-go/phases/19-http-filter-ext-proc/BRAINSTORM.md` section-for-section, reframed for the oauth2 scope and adapted for its specific surface area. Phase 20 sits in a structurally important position relative to the §9 family: it is the FIRST §9 family-row to **CLOSE the ADR-0159 §Future Work forward-pointer** — "third outbound-HTTP consumer triggers `internal/httpclient/` extraction" — via the Q2 EXTRACT NOW decision; the THIRD CONSECUTIVE §9 family-row to **REUSE-or-skip** the ADR-0125 canonical-roster per the Q5 listener-scoped-only decision (after phase 18 + phase 19 both REUSED the 5th canonical; phase 20's REUSE-by-absence — the v1.37.x oauth2 proto has no `OAuth2PerRoute` message at all — is a STRONGER form of the lesson). Per the Q1 + Q2 + Q4 + Q7 + Q8 user picks, phase 20 commits to **TWO NEW package-level framework primitives** (`internal/httpclient/` + `internal/sdsfile/`) **PLUS ONE NEW filter-local AES-GCM token-encryption helper** **PLUS TWO IN-PLACE §Decision AMENDMENTs to prior ADRs** (ADR-0150 jwks framework primitive + ADR-0159 extauthz `httpAuthClient`) per ADR-0044 in-place edit discipline. Sections §§1–12 are decision-bearing prose; §10 enumerates the empirical-pin obligations the SPEC author resolves against Envoy v1.37.2. Per D-3.4 (context isolation), every load-bearing fact cited here lives on disk in the named files; no "see prior conversation" references appear. NO off-master prebrainstorm-notes branch was authored for phase 20 — this brainstorm cold-started fresh from the §9 heading + the phase 19.2 just-shipped artefacts per ADR-0106(e).

**Authored:** 2026-05-17.

---

## 1. Mission and scope confirmation (20 only)

ROADMAP row `20 | http-filter-oauth2 | 19 | in-progress | | …` (added by this brainstorm at the explicit user-direction departure from the phase-09..19.2 ROADMAP-row-add-at-SPEC convention; the row registers in `## Feature Families (09+)` after row 19.2 — see §1.3) is the row this brainstorm registers as the next family-row landing. Phase 20 is the THIRTEENTH concrete phase to enter the BOOTSTRAP_PROMPT.md §9 HTTP filters family heading. The phase 19.2 squash-merge commit `1ddb661` (with SHA-fill at `c2c0f27`) is this row's `depends-on` anchor.

The HTTP filters family lists candidate filters at `ROADMAP.md` line 66: header manipulation, cors, compression, fault, local + global rate limit, jwt_authn, rbac, ext_authz, ext_proc, oauth2, csrf, buffer, lua, wasm, adaptive concurrency, admission control, bandwidth limit. The 12 prior landings: `cors` (phase 07.1 ADR-0074), `fault` (phase 09 ADR-0100), `header_mutation` (phase 10 ADR-0108), `local_ratelimit` (phase 11 ADR-0114), `csrf` (phase 12 ADR-0120), `buffer` (phase 13 ADR-0125), `compressor` (phase 14 ADR-0129–0134), `bandwidth_limit` (phase 15 ADR-0135–0139), `rbac` (phase 16 ADR-0140–0147), `jwt_authn` (phase 17 ADR-0148–0155), `ext_authz` (phase 18 with 18.1+18.2 split per ADR-0045 + ADR-0164; ADR-0156..ADR-0166 anchor), `ext_proc` (phase 19 with 19.1+19.2 split per ADR-0045 + ADR-0176; ADR-0167..ADR-0176 anchor). Phase 20 ships **OAuth 2.0 sign-in / refresh / sign-out (browser-facing redirect + cookie-session) as a downstream-authentication filter** as the THIRTEENTH real filter — the canonical Envoy-style "intercept an unauthenticated request, redirect to the authorization_endpoint, handle the callback, exchange the code for tokens at the token_endpoint, encrypt the tokens into Set-Cookie envelope" filter. The chosen branch + directory + Go-package identifier are aligned per phase-11 ADR-0114's underscore-stripping convention: branch `phase-20-http-filter-oauth2-brainstorm` (matches the type-URL minus the v3 suffix), directory `internal/filter/http/oauth2/` (no underscore — matches `localratelimit/` + `jwtauthn/` + `extauthz/` + `extproc/` precedent for single-token-after-strip filter names), Go package identifier `oauth2` (same as directory).

Phase 20 is also: (i) the FIRST §9 family-row to **CLOSE a phase-N-1 forward-pointer load-bearing** — the ADR-0159 §Future Work forward-pointer ("third outbound-HTTP consumer triggers `internal/httpclient/` extraction") fires at phase 20 per Q2's EXTRACT NOW decision (the third outbound-HTTP consumer is the oauth2 filter's token_endpoint POST + future-id_token-jwks-discovery; the first two consumers — `internal/jwks/Fetcher` at phase 17 and `extauthz/check.go`'s `httpAuthClient` at phase 18.1 — get refactored to consume the new `internal/httpclient/` primitive in-place per ADR-0044). (ii) the FIRST §9 family-row whose MVP envelope **includes BOTH outbound HTTP I/O (token_endpoint POST) AND browser-facing redirect mechanics (302 to authorization_endpoint + callback handling + Set-Cookie envelope)** — distinct from all prior §9 filters' upstream-only or local-reply-only modes. (iii) the FIRST §9 family-row whose configuration **requires filesystem-watched SDS secret-material loading** (per Q3 filesystem-only + Q7 NEW `internal/sdsfile/` + Q8 fsnotify reload) — distinct from prior §9 filters' inline-Secret consumption (jwt_authn local_jwks inline_bytes; ext_authz no secret material). (iv) the FIRST §9 family-row whose MVP envelope **includes AES-GCM cookie-payload encryption** (per Q4 default-on AES-GCM with KDF from `hmac_secret`) — distinct from all prior §9 filters' plaintext payloads (jwt_authn JWT signature verification is the closest analog, but JWT is JSON-payload-signed-not-encrypted). (v) the FIRST §9 family-row to **REUSE-by-absence the ADR-0125 canonical roster** — per Q5 the v1.37.x oauth2 proto has NO `OAuth2PerRoute` message at all, so phase 20 is the THIRD CONSECUTIVE §9 row to skip an ADR-0125 amendment (after phase 18 + phase 19 both REUSED the 5th canonical), but phase 20's REUSE-by-absence is a stronger form of the lesson — there is no per-route surface at all, so the listener-scoped-only enforcement is itself a parse-time PARSE-REJECT for any oauth2 TPFC placed under a route/virtualHost.

### 1.1 What 20 delivers as a self-contained whole

Phase 20 lands `envoy.filters.http.oauth2` (the canonical Envoy OAuth 2.0 authentication filter, Standard envelope per Q1, sign-in + refresh + sign-out, AES-GCM cookie encryption default-on per Q4, filesystem-path SDS per Q3 + fsnotify hot-reload per Q8, listener-scoped-only per Q5, `client_secret_post` token_endpoint POST body format per Q10) under the 07.1 framework. **Nine in-scope filter-implementation items, plus three artefact-level deliverables (12 total bullets):**

1. **New `internal/filter/http/oauth2/` package** owning the filter implementation. Package directory + Go package identifier are both `oauth2` (single token underscore-stripped per ADR-0114; matches `localratelimit/` + `jwtauthn/` + `extauthz/` + `extproc/` precedent). Files mirror the multi-file structure of phase 19.x (the precedent for larger session-state filters): `oauth2.go` (filter type + factory + decode methods + filterStats struct + compiledConfig + listener-scoped-only enforcement helper), `flow.go` (the three-flow dispatcher — sign-in flow path-matching + callback flow path-matching + sign-out flow path-matching + the request-classification per the configured `redirect_path_matcher` / `signout_path` / `pass_through_matcher`), `cookies.go` (the 5-of-7 CookieNames envelope: `BearerToken` / `OauthHMAC` / `OauthExpires` / `IdToken` (deferred) / `RefreshToken` — Set-Cookie attribute discipline: `Secure` / `HttpOnly` / `SameSite=Strict` / `Path=/`; HMAC composition per Q9; cookie parse + validate on inbound), `tokens.go` (the AES-GCM token-encryption helper — `Encrypt(plaintext []byte) []byte` + `Decrypt(ciphertext []byte) ([]byte, error)` + KDF from `hmac_secret` + nonce-derivation per Q4 + `disable_token_encryption=true` skip-path), `endpoint.go` (the token_endpoint POST + the authorization_endpoint redirect URL construction + the `client_secret_post` body format per Q10 + the JSON-response parser for the `access_token` / `refresh_token` / `expires_in` / `id_token` (deferred) fields), `refresh.go` (the refresh-token rotation timing + `default_refresh_token_expires_in` semantics + race-vs-rotation discipline), `signout.go` (the sign-out flow: `signout_path` handling + cookie clearing + `deny_redirect_matcher` integration + redirect-to-origin semantics), `oauth2_test.go` (unit tests; anticipated 3500–4000 LoC given the three-flow surface + cookie/token discipline + AES-GCM encryption + refresh-token rotation + sign-out), `fuzz_test.go` (the 26th fuzzer in the repo — `FuzzOAuth2ConfigParse`), `doc.go` (package overview + Q1-Q10 decision summary). The package exposes `TypeURL` (the canonical type-URL constant `"type.googleapis.com/envoy.extensions.filters.http.oauth2.v3.OAuth2"`) + `New` (the `HTTPFilterFactory`) per the cors / fault / header_mutation / local_ratelimit / csrf / buffer / compressor / bandwidth_limit / rbac / jwt_authn / ext_authz / ext_proc precedent. Per ADR-0045 the SPEC author may split this package across two sub-phases (§1.4).

2. **Extension-registry registration** at boot, per ADR-0072. `cmd/envoy-go/main.go` (currently registering 14 entries after phase 19.2 at lines 122–135: `router.New`, `bandwidthlimit.New`, `buffer.New`, `compressor.New`, `cors.New`, `csrf.New`, `envoygotest.New`, `extauthz.New`, `extproc.New`, `fault.New`, `header_mutation.New`, `jwtauthn.New`, `localratelimit.New`, `rbac.New` before the `httpReg.Freeze()` invocation) gains a fifteenth `httpReg.Register(oauth2.TypeURL, oauth2.New)` call before the freeze. Insertion alphabetical per the ADR-0100 §2.2 convention: `router → bandwidthlimit → buffer → compressor → cors → csrf → envoy_go_test → extauthz → extproc → fault → header_mutation → jwtauthn → localratelimit → oauth2 → rbac → Freeze`. `oauth2` inserts between `localratelimit` (current line 134) and `rbac` (current line 135) to maintain alphabetical-after-router ordering; rbac shifts down one line. Per ADR-0072, registration order does NOT affect runtime behavior; this is a stylistic discipline only. NO ADR is anchored for this — straight alphabetical insertion per the phase-09..19.2 convention.

3. **Proto-config parsing of `envoy.extensions.filters.http.oauth2.v3.OAuth2`,** the canonical filter-level config message. Per `go-control-plane/envoy v1.32.4` (proto pin via ADR-0008 → Envoy v1.37.2 → proto v3), phase 20 consumes **~17 of 26 `OAuth2Config` fields** + **4 of 5 `OAuth2Credentials` fields** (no BASIC_AUTH per Q1 + Q10) + **5 of 7 `CookieNames` fields** (no `oauth_nonce`, no `code_verifier` — both deferred with PKCE per Q1). **Consumed at runtime (the MVP-envelope fields; the SPEC §10 empirical-pin block does the exhaustive proto-field roster against reference Envoy v1.37.2):**

   - `OAuth2Config.token_endpoint` (`*core.HttpUri`) — the upstream OAuth 2.0 token_endpoint URI; consumed for the POST during callback flow per Q10's `client_secret_post` body format.
   - `OAuth2Config.authorization_endpoint` (string) — the upstream OAuth 2.0 authorization_endpoint URI; consumed for the 302-redirect URL construction during sign-in flow.
   - `OAuth2Config.redirect_uri` (string) — the registered redirect_uri to embed in the authorization_endpoint redirect + the token_endpoint POST.
   - `OAuth2Config.redirect_path_matcher` (`*matcherv3.PathMatcher`) — the path-matcher classifying inbound requests as callback-flow vs sign-in-flow.
   - `OAuth2Config.signout_path` (`*matcherv3.PathMatcher`) — the path-matcher classifying inbound requests as sign-out-flow (per Q1's Standard envelope; consumed).
   - `OAuth2Config.credentials` (`*OAuth2Credentials`) — the client credentials envelope; consumed fields per the 4-of-5 split below.
   - `OAuth2Config.forward_bearer_token` (bool) — when true, inject `Authorization: Bearer <decrypted-BearerToken>` on upstream proxy.
   - `OAuth2Config.preserve_authorization_header` (bool) — when true, preserve any existing `Authorization` header on the proxied request (per Q1 explicit MVP-consumed).
   - `OAuth2Config.pass_through_matcher` ([]*HeaderMatcher) — request-header matchers that bypass oauth2 entirely (consumed proto-faithful; envoy-go-strict on matcher type).
   - `OAuth2Config.auth_scopes` ([]string) — the OAuth 2.0 `scope` parameter embedded in the authorization_endpoint redirect URL.
   - `OAuth2Config.resources` ([]string) — the OAuth 2.0 `resource` parameter(s) embedded in the authorization_endpoint redirect URL.
   - `OAuth2Config.use_refresh_token` (`*wrappers.BoolValue`) — gates whether refresh-token rotation is enabled (per Q1 Standard envelope; MVP-consumed).
   - `OAuth2Config.default_expires_in` (`*durationpb.Duration`) — the fallback BearerToken expiry when the token_endpoint response omits `expires_in` (per Q1 MVP-consumed).
   - `OAuth2Config.default_refresh_token_expires_in` (`*durationpb.Duration`) — the RefreshToken cookie expiry (per Q1 MVP-consumed).
   - `OAuth2Config.deny_redirect_matcher` ([]*HeaderMatcher) — on a denied request matching the matcher, emit a 302 instead of a 401 (per Q1 MVP-consumed; integrates with sign-out flow per B4 deny-path wire shape).
   - `OAuth2Config.disable_token_encryption` (bool) — default `false` per Q4; when explicit `true`, skip the AES-GCM encryption layer (cookies stored plaintext).
   - `OAuth2Credentials.client_id` (string) — embedded in the authorization_endpoint redirect URL + the token_endpoint POST body per Q10.
   - `OAuth2Credentials.token_secret` (`*v3.SdsSecretConfig`) — the client_secret via SDS; embedded in the token_endpoint POST body per Q10's `client_secret_post`.
   - `OAuth2Credentials.hmac_secret` (`*v3.SdsSecretConfig`) — the HMAC + AES-GCM-KDF secret via SDS; consumed for cookie HMAC per Q9 + token-encryption KDF per Q4.
   - `OAuth2Credentials.cookie_names` (`*CookieNames`) — the 5-of-7 cookie-name customization envelope per the deferral list below.
   - `CookieNames.bearer_token` / `oauth_hmac` / `oauth_expires` / `id_token` (deferred) / `refresh_token` — the 5 of 7 customizable names; MVP defaults `BearerToken` / `OauthHMAC` / `OauthExpires` / `IdToken` / `RefreshToken`.

   **Deferred fields** (silent-ignore under the inline-deferral discipline; full roster at §8): `OAuth2Credentials.basic_auth` (BASIC_AUTH out of envelope per Q1 + Q10); `OAuth2Config.retry_policy` (`*RetryPolicy`; deferred — httpclient applies SPEC-time-pinned default per §10 open question §20.P1); `OAuth2Config.end_session_endpoint` (deferred per Q1); `OAuth2Config.use_pkce` + `CookieNames.oauth_nonce` + `CookieNames.code_verifier` + `OAuth2Config.code_verifier_token_expires_in` (PKCE deferred per Q1); `OAuth2Config.cookie_configs` (`map[string]CookieConfig`) — per-cookie attribute customization (deferred per Q1); `OAuth2Config.disable_id_token_set_cookie` / `OAuth2Config.disable_access_token_set_cookie` / `OAuth2Config.disable_refresh_token_set_cookie` (all deferred — couples to PKCE + id_token); `OAuth2Config.csrf_token_expires_in` (deferred); `OAuth2Config.cookie_domain` (deferred — Set-Cookie Domain attribute defaulting to request Host per §10 open question §20.P2); id_token + id_token-validation (consumes ADR-0150 jwks + ADR-0151 jwt verifier — deferred future phase).

4. **Three-flow dispatcher (Decision #1 → ADR-0180).** Per Q1 Standard envelope (sign-in + refresh + sign-out): the filter's `DecodeHeaders` classifies the inbound request into one of FOUR dispositions: (a) `pass_through_matcher` hit → bypass oauth2 entirely (counter `oauth_passthrough`); (b) `redirect_path_matcher` hit (callback flow) → invoke the token_endpoint POST via async-resume per Phase-09; (c) `signout_path` hit (sign-out flow) → clear cookies + 302 redirect; (d) default (sign-in flow) → if valid cookies present, proxy upstream with `Authorization: Bearer <decrypted>` injection; if cookies missing/expired AND refresh-token cookie present, attempt silent refresh-token rotation; otherwise 302 redirect to authorization_endpoint with `client_id` + `redirect_uri` + `scope=<auth_scopes>` + `resource=<resources>` + `state=<HMAC-protected-cookie-state>`. The dispatcher is async on the callback-flow leg only: the token_endpoint POST parks the decode goroutine on a resume channel (mirrors the phase-09 fault + phase-18.x ext_authz async-resume primitive). The other three flows (pass_through / sign-out / sign-in-with-valid-cookies / sign-in-redirect) complete synchronously from header-decode via `SendLocalReply` + Location header (302) or via `cb.ContinueDecoding()` + Authorization-header injection.

5. **Filter-callback shape: DECODER-ONLY** (matches phase 09/10/11/12/13/16/17/18 precedent; differs from phase 14's encode-only + phase 19's both-sides). Phase 20 has NO encode-side participation in the MVP — Set-Cookie headers are injected on the SendLocalReply path (decoder-side, via the 302 + Set-Cookie envelope) + via response-header mutation on the callback-flow's 302-to-origin step (still decoder-side via SendLocalReply). Static blank-identifier compile-time check for `StreamDecoderFilter` only (NOT `StreamEncoderFilter`). The decode-side surface: `DecodeHeaders(headers, endStream)` resolves the three-flow classification (§4) + returns `HeaderStopIteration` for the async callback-flow leg or `HeaderContinue` / `HeaderStopIteration`+`SendLocalReply` for the other three flows. `DecodeData` / `DecodeTrailers` participate only on the callback-flow leg (where the OAuth 2.0 callback's body MAY carry `code` as a query param — typically `code` arrives in the GET query string, but POST callback variants need request-body parsing; SPEC author confirms via §10 open question §20.P3).

6. **AES-GCM token-encryption envelope (Decision #2 → ADR-0182).** Per Q4 Proto-faithful AES-GCM default-on: encrypt `BearerToken` + `RefreshToken` cookie values (the `IdToken` cookie value would also be encrypted but is deferred per Q1). Algorithm: AES-256-GCM with a 12-byte random nonce per encryption (carried in-cookie as the leading 12 bytes; mirrors stdlib `crypto/cipher.NewGCM` defaults per §10 open question §20.P5). KDF: derive a 32-byte AES-256 key from `hmac_secret` via HKDF-SHA256 (or HMAC-SHA256 single-pass — SPEC author pins against reference Envoy v1.37.2 at §10 open question §20.P5). The `disable_token_encryption=true` override skips the encryption layer entirely (plaintext cookie values; mirrors reference Envoy semantics). ~150-200 LoC stdlib `crypto/aes` + `crypto/cipher` + `crypto/hmac` + `crypto/sha256`. The encryption helper lives filter-local at `oauth2/tokens.go` (NOT extracted to a shared package at phase 20 — second-consumer trigger defers per phase-18.1 ADR-0159 (b)-disposition rationale). ADR-0182 anchors the AES-GCM mode + nonce-derivation + KDF discipline + the `disable_token_encryption` skip behavior.

7. **Cookie HMAC scheme (Decision #3 → ADR-0179).** Per Q9 HMAC-SHA256 composition: each Set-Cookie response's `OauthHMAC` cookie value is computed as `base64(HMAC-SHA256(hmac_secret, host + BearerToken + OauthExpires))` where `host` is the request Host header (request-host-binding rationale — prevents cross-host cookie replay per Envoy default discipline) + `BearerToken` is the (encrypted-or-plaintext-depending-on-Q4) BearerToken cookie value + `OauthExpires` is the unix-timestamp string. Standard stdlib `crypto/hmac` + `crypto/sha256`. On inbound cookie validation: recompute the HMAC and compare via `subtle.ConstantTimeCompare`; mismatch → treat as no-cookie + 302 redirect to authorization_endpoint. SPEC author confirms (a) byte-exact composition ordering against reference Envoy v1.37.2 (§10 open question §20.P4 — may include additional fields like CookieAttributeNames) and (b) the `subtle.ConstantTimeCompare` constant-time discipline. ADR-0179 anchors the composition order + host-binding rationale + the constant-time-compare discipline.

8. **Filesystem-path SDS (Decision #4 → ADR-0178).** Per Q3 filesystem-path SDS only: honor `SdsSecretConfig.SdsConfig.ConfigSource.Path` (or equivalent path-selector — SPEC author pins the exact ConfigSource oneof variant via empirical scrape at §10 open question §20.P6) pointing at a `Secret` proto JSON/YAML file containing a `generic_secret.secret.inline_string` (or `inline_bytes`). Other `ConfigSource` variants (`api_config_source`, `ads`, `path_config_source`, the resource_locator variants) PARSE-REJECT envoy-go-strict (no xDS control-plane in envoy-go; deferred per §8 item 13). Per Q7 the loader lives in a NEW `internal/sdsfile/` top-level package (~80-120 LoC primitive; anticipates future consumers — jwt_authn TLS-trust-store reload, future ext_authz mTLS, future ratelimit) anchored at ADR-0178. Per Q8 the loader watches the secret file via `fsnotify` for Write events + reload + atomic-swap the in-memory secret value on change; ~80 LoC extra (likely bundled into ADR-0178 same anchor). The atomic-swap discipline: the loader exposes `Load() []byte` returning the latest in-memory copy under a `sync.RWMutex` (or `atomic.Pointer[[]byte]`); cookie HMAC + AES-GCM-KDF + client_secret consumers call `Load()` per-request, so a mid-stream secret rotation takes effect on the NEXT request without filter restart.

9. **TWO NEW package-level framework primitives + ONE NEW filter-local helper + TWO IN-PLACE §Decision AMENDMENTs (per Q2 + Q7 + Q8 + Q4)** + FIVE REUSES per §3. The accretion shape: phase 20 INTRODUCES `internal/httpclient/` (NEW top-level package, ~150-250 LoC primitive wrapping `http.Client` + a `RetryPolicy` + a `Timeout` envelope) per Q2 EXTRACT NOW + closes the ADR-0159 §Future Work forward-pointer; INTRODUCES `internal/sdsfile/` (NEW top-level package, ~160-200 LoC primitive — filesystem-path Secret reader + fsnotify hot-reload) per Q7 + Q8; INTRODUCES a filter-local AES-GCM token-encryption helper at `oauth2/tokens.go` (NOT a shared package — second-consumer-trigger defers per phase-18.1 (b)-disposition rationale); AMENDS ADR-0150 in-place to refactor `internal/jwks/Fetcher` to consume `internal/httpclient/` rather than its own `http.Client` (per ADR-0044 in-place §Decision AMENDMENT — closes the ADR-0150 implicit forward-pointer to the future httpclient primitive); AMENDS ADR-0159 in-place to refactor `extauthz/check.go`'s `httpAuthClient` similarly (per ADR-0044 in-place §Decision AMENDMENT — closes the explicit ADR-0159 §Future Work forward-pointer). Cross-phase refactor delta ~200-300 LoC across extauthz + jwks. See §3 for framework-survey details. PLUS FIVE REUSES per §3.6.

**Plus three artifact-level deliverables:**

10. **Differential fixture `0024-http-oauth2`** under `test/fixtures/0024-http-oauth2/`: `envoy.yaml` + `envoy-go.yaml` + a Go driver in `inputs/driver.go` exercising ~6-8 scenarios per §6 below across the three flows (sign-in + callback + sign-out) + the pass-through-matcher leg + the refresh-token rotation leg + the AES-GCM-disabled skip-path leg. The fixture requires **ONE new test-helper** under `test/helpers/oauthbackend/`: an in-process OAuth 2.0 authorization-server backend (mock `authorization_endpoint` issuing redirect-with-code + mock `token_endpoint` accepting POSTs + returning JSON with `access_token` / `refresh_token` / `expires_in`; optional-stub JWKS endpoint for future id_token reuse). Likely shape: stdlib `net/http/httptest`-based server, configurable per-scenario with scripted responses keyed by request method + path. The HTTP-service test-helper for the filesystem-SDS Secret file may be a simple per-fixture `Secret` JSON file in `test/fixtures/0024-http-oauth2/secrets/`.

11. **`BEHAVIOR_CONTRACT.md` ~8-edit bundle (lands at IMPL phase-done, NOT this brainstorm).** Under the existing `## HTTP filter chain` umbrella (alongside the existing 12 filter subsections through phase 19.2): a NEW `### envoy.filters.http.oauth2` subsection covering the Standard envelope, the three-flow dispatcher, the AES-GCM token-encryption envelope, the filesystem-SDS + fsnotify reload disciplines, the HMAC composition + host-binding, the deny-path wire shape (302 / 401 / 500), the listener-scoped-only enforcement (no per-route surface at all). Plus the 86 → ~94-name stat-table extension. Plus a new equivalence-matrix row pointing at fixture 0024 with per-scenario tolerance discipline. Plus a NEW `### Phase 20 forward-pointer notes` subsection under `## Forward-pointer notes` covering the deferral list (per §8 below). Plus an additive NEW `## HTTP outbound client framework primitive (per phase 20 ADR-0177)` umbrella covering the `internal/httpclient/` primitive + the ADR-0150 jwks consumer + the ADR-0159 extauthz consumer refactor. Plus an additive NEW `## SDS filesystem secret loader framework primitive (per phase 20 ADR-0178)` umbrella covering the `internal/sdsfile/` primitive + the fsnotify hot-reload discipline. Plus an extension to `### Phase 19.2 forward-pointer notes` to add the THREE phase-19.2 forward-pointers as CARRIED-FORWARD-UNTOUCHED at phase 20 (ext_proc streaming-mode, ext_proc REQUEST_RESPONSE merge, ext_proc message_timeout — none touched by phase-20 per B7).

12. **Anticipated 7-9 ADRs (ADR-0177 through ADR-0185)** per §7 below. ADR-0176 is the highest-numbered ADR landed in phase 19.2; ADR-0177 is the next-free per STATE.md. **Phase 20 lands NO ADR-0125 amendment paragraph** — the per-route shape is REUSE-by-absence (§4); this is the THIRD CONSECUTIVE §9 row to not extend the ADR-0125 canonical roster (phase 18 + phase 19 both REUSED the 5th canonical per ADR-0163 + ADR-0173; phase 20's REUSE-by-absence is a stronger form). ADR-0044 escape-valve held in reserve at IMPL-time for ~0-2 impl-time-unanticipated ADRs if scope balloons.

### 1.2 What 20 does NOT deliver (forward to §8)

The exhaustive deferral list lives in §8 under the inline-deferral discipline (no omnibus ADR per phase 11 SPEC §8.1 + phase 12/13/14/15/16/17/18/19 precedent; deferrals grouped by family-coupling). Summary: BASIC_AUTH (`OAuth2Credentials.basic_auth`); `retry_policy`; `end_session_endpoint`; id_token + validation (consumes ADR-0150 jwks + ADR-0151 jwt verifier — future phase); PKCE (`use_pkce` + `code_verifier` cookie + `oauth_nonce` cookie); `cookie_configs` (`map[string]CookieConfig`); `disable_id_token_set_cookie` / `disable_access_token_set_cookie` / `disable_refresh_token_set_cookie`; `csrf_token_expires_in`; `code_verifier_token_expires_in` (paired with PKCE); `cookie_domain` (listener-default-host semantics — Set-Cookie Domain attribute defaulting to request Host); SDS non-filesystem ConfigSource variants (`api_config_source` / `ads` — PARSE-REJECT for now; future phase if/when xDS-control-plane lands); per-route override — NEVER (no `OAuth2PerRoute` message in v1.37.x proto). None are blockers for closing row 20 phase-done.

### 1.3 Phase-done as a §9 family-row landing

Phase 20's phase-done commit closes ROADMAP row `20` (single-row hypothesis at brainstorm time — with ADR-0045 split as the release valve per §1.4 below; MEDIUM-HIGH leaning per Q6, so split is plausible but not as likely as phase-19's HIGH). It does NOT close any §9 family heading (family headings are not rows per ADR-0106) — the HTTP filters family stays "in-progress" implicitly until the last filter under the family ships. Phase 20 is the THIRTEENTH §9 family-row to land (after 07.1-cors, 09-fault, 10-header_mutation, 11-local_ratelimit, 12-csrf, 13-buffer, 14-compressor, 15-bandwidth_limit, 16-rbac, 17-jwt_authn, 18-ext_authz with its 18.1+18.2 split, 19-ext_proc with its 19.1+19.2 split). The next §9 family-row will be numbered `21` (or `21`+ if phase 20 splits) per the flat-row discipline of ADR-0106. The §9 heading at `ROADMAP.md` line 66 stays unchanged across this landing. The 5 remaining unbrainstormed §9 candidates after phase 20 lands: `lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `global_ratelimit`.

**Row-registration timing departure:** Phase 20 lands the ROADMAP row at BRAINSTORM time (this commit) per the explicit user-direction departure from the phase-09..19.2 ROADMAP-row-add-at-SPEC convention. The row is registered `in-progress` with empty sub-phases; the SPEC author may add sub-phase rows `20.1` / `20.2` at SPEC time if the ADR-0045 split fires per Q6 MEDIUM-HIGH leaning.

### 1.4 ADR-0045 split-by-surface readiness — MEDIUM-HIGH anticipation for phase 20

The brainstorm's POSITION per Q6 is **MEDIUM-HIGH** — phase 20's LoC envelope sits between phase-17 jwt_authn (3855 single-row) and phase-19 ext_proc (5000+ split). The SPEC author makes the final call after the empirical-pin scrape closes the LoC envelope. The drivers:

- **Standard MVP envelope (Q1) — sign-in + refresh + sign-out.** Three distinct flow surfaces, each with its own state-machine + cookie-handling + endpoint-mechanic.
- **Cross-phase httpclient refactor (Q2 EXTRACT NOW).** The `internal/httpclient/` extraction + the in-place ADR-0150 + ADR-0159 §Decision AMENDMENTs + the extauthz/check.go + jwks/Fetcher refactor adds ~200-300 LoC of cross-phase delta beyond the filter package itself.
- **AES-GCM token-encryption envelope (Q4) — default-on.** ~150-200 LoC stdlib `crypto/aes` + `crypto/cipher` + KDF + nonce-derivation discipline.
- **Filesystem-path SDS + fsnotify reload (Q3 + Q7 + Q8).** ~160-200 LoC NEW `internal/sdsfile/` primitive + atomic-swap discipline.
- **HMAC cookie composition + host-binding (Q9).** Comparatively small (~80 LoC) but distinct sub-surface.

LoC estimate: ~3500-4000 LoC production (filter package ~2200-2600 + framework deltas — `internal/httpclient/` ~150-250 + `internal/sdsfile/` ~160-200 + extauthz + jwks refactor ~200-300 + tests ~600-900). This is **MEDIUM-HIGH** relative to the ADR-0045 1500-LoC trigger — well above the trigger but well below phase-19's 5000+. Two split candidates surveyed (the SPEC author owns the disposition per ADR-0045 deferral-to-SPEC discipline):

- **Recommended: split-by-feature-class.** 20.1 = framework refactor (`internal/httpclient/` + `internal/sdsfile/` + ADR-0150/0159 in-place AMENDMENTs + extauthz/jwks refactor) + oauth2 sign-in flow (HMAC cookie + redirects + token_endpoint POST per Q10 + URL_ENCODED_BODY + pass_through_matcher + auth_scopes + resources). ~2000-2400 LoC. 20.2 = AES-GCM token encryption + refresh-token rotation + sign-out + `deny_redirect_matcher` + `preserve_authorization_header`. ~1500-1800 LoC. The sign-in baseline lands first; the AES-GCM + rotation + sign-out extends.
- **Alternative: single-row landing.** Viable if the SPEC author's empirical scrape lands the LoC closer to the lower end of the estimate (~3500). Phase-17 jwt_authn at 3855 single-row is the precedent; phase 20 sits in the same band.

The brainstorm does NOT pre-commit to the split per ADR-0045 deferral-to-SPEC discipline. UNLIKE phase 19 (which flagged HIGH and DID split per ADR-0045 + ADR-0176), phase 20's MEDIUM-HIGH leaning is genuinely borderline — the SPEC author may rationally either-way per the empirical scrape closing the LoC envelope.

### 1.5 Seed-stub alignment

Like phases 09 through 19.2, phase 20 has NO sibling SPEC stub — phase 20 enters fresh after the phase 19.2 close. The §9 family-children list at ROADMAP line 66 enumerates the conceptual surface; the ROADMAP rows enumerate only filters currently in-progress or done. Per ADR-0106(b) (no-sibling-stub discipline), this brainstorm does NOT pre-author SPEC stubs for siblings (`lua`, `wasm`, `adaptive_concurrency`, `admission_control`, `global_ratelimit`). Each next-filter phase brainstorms cold from the §9 heading + the just-shipped artefacts. NO `envoy.filters.http.oauth2`-typed seed-stub exists in the existing fixture tree or in `cmd/envoy-go/main.go` (verified at this brainstorm's worktree-checkout time).

### 1.6 No prebrainstorm-notes branch

UNLIKE phase 11 which had an off-master prebrainstorm-notes branch (`phase-11-http-filter-local-ratelimit-prebrainstorm-notes`), phase 20 has NO such branch. The brainstorm dialogue (Q1–Q10 over the user-Claude exchange) was sufficient to settle MVP envelope + httpclient extraction trigger + SDS posture + token encryption + per-route shape + split-readiness + sdsfile location + reload discipline + HMAC composition + token-endpoint body format without preliminary scoping notes. This matches the phase 09 / 10 / 12 / 13 / 14 / 15 / 16 / 17 / 18 / 19 cold-start precedent.

### 1.7 Phase 20's relationship to prior framework deltas + framework-delta accretion shape

Phase 20's framework deltas are SUBSTANTIAL and STRATEGIC: TWO NEW package-level primitives (`internal/httpclient/` + `internal/sdsfile/`) + ONE NEW filter-local helper (AES-GCM token-encryption) + TWO IN-PLACE §Decision AMENDMENTs (ADR-0150 + ADR-0159). The accretion shape across §9 rows:

- Phase 13 introduced two decode-side framework primitives (ADR-0128).
- Phase 14 introduced one encode-side framework primitive (ADR-0131).
- Phase 15 introduced ZERO and demonstrated ADR-0128 + ADR-0131 reusability.
- Phase 16 introduced TWO — both designed cross-phase-reusable (matcher-engine + TLS-principal accessor).
- Phase 17 introduced TWO — both designed cross-phase-reusable (HTTP-outbound JWKS fetcher + JWT verifier).
- Phase 18 introduced FOUR — gRPC-client outbound (ADR-0158) + HTTP-outbound auth-check (ADR-0159) + callback-surface extension (ADR-0165) + plaintext h2c upstream relaxation (ADR-0166).
- Phase 19 introduced ONE NEW primitive (`*ProcessorClient` extending ADR-0158) + ONE NEW JSON codec (ADR-0170) + TWO CONDITIONAL deltas that both fired (ADR-0174 encode-side callback symmetry + ADR-0175 encode-side body-buffering primitive).
- **Phase 20 introduces TWO NEW package-level primitives (`internal/httpclient/` ADR-0177 + `internal/sdsfile/` ADR-0178) + ONE NEW filter-local AES-GCM helper (ADR-0182) + TWO IN-PLACE §Decision AMENDMENTs (ADR-0150 + ADR-0159 — refactored to consume `internal/httpclient/` in-place per ADR-0044).** Phase 20 is the FIRST §9 row to CLOSE a phase-N-1 forward-pointer load-bearing (ADR-0159 §Future Work) via Q2's EXTRACT NOW + the in-place §Decision AMENDMENT discipline. AND phase 20 demonstrates the reusability of FIVE prior primitives (§3.6) — Phase-04 HCM `SendLocalReply` + Phase-09 async-resume + ADR-0144 NOT-consumed + ADR-0150 jwks NOT-consumed (id_token deferred) + ADR-0151 jwt verifier NOT-consumed (id_token deferred).

The framework-delta budget across phases 13–20 is now: ADR-0128 (phase 13) + ADR-0131 (phase 14) + ADR-0142 (phase 16) + ADR-0144 (phase 16) + ADR-0150 (phase 17; **AMENDED in-place at phase 20**) + ADR-0151 (phase 17) + ADR-0158 (phase 18.2) + ADR-0159 (phase 18.1; **AMENDED in-place at phase 20**) + ADR-0165 (phase 18.2) + ADR-0166 (phase 18.2) + ADR-0169 (phase 19.1) + ADR-0170 (phase 19.1) + ADR-0174 (phase 19.1) + ADR-0175 (phase 19.2) + ADR-0177 (phase 20 NEW — `internal/httpclient/`) + ADR-0178 (phase 20 NEW — `internal/sdsfile/`) + ADR-0182 (phase 20 NEW — AES-GCM token-encryption helper, filter-local) = 17 framework primitive families across 8 phases (with TWO in-place AMENDMENTs to existing ADRs at phase 20). Phase 20 marks the FIRST envoy-go phase to ship a **browser-facing redirect + cookie-session** consumer of the HTTP-filter framework — the OAuth 2.0 sign-in / refresh / sign-out pattern.

---

## 2. Design decisions (per topic; each cites BRAINSTORM-style rationale + consequences anchor)

The 10 user-picked decisions below (Q1–Q10) plus the self-answered decisions are the phase-20-specific design choices reached during the Q-dialogue. Each cites its anticipated ADR anchor (§7); the ADRs are written by the SPEC author at lifecycle-state 1 → 2 transition.

### 2.1 MVP envelope: Standard OAuth 2.0 (sign-in + refresh + sign-out) *(Q1 → ADR-0180 + ADR-0181)*

Per Q1 (MVP envelope) decision: Standard OAuth 2.0 — sign-in + refresh + sign-out. NO id_token / NO PKCE / NO end_session_endpoint / NO cookie_configs / NO BASIC_AUTH. Consumes ~17 of 26 `OAuth2Config` fields (full list at §1.1 item 3); 4-of-5 `OAuth2Credentials` (no `basic_auth`); 5-of-7 `CookieNames` (no `oauth_nonce`, no `code_verifier` — both deferred with PKCE). Defers ~10 of 26 `OAuth2Config` fields per §8. Estimated 3500-4000 LoC including cross-phase httpclient refactor — single-row borderline (MEDIUM landing). Includes `signout_path`, `use_refresh_token`, `default_expires_in`, `default_refresh_token_expires_in`, `deny_redirect_matcher`, `preserve_authorization_header`, `disable_token_encryption=false` default AES-GCM encryption ON. ADR-0180 anchors the three-flow state-machine + the 302-redirect / 401 / 500 deny-path wire shape; ADR-0181 anchors the 5-of-7 cookie-name envelope + the 2-deferred-with-PKCE cookie subset + Set-Cookie attribute discipline.

### 2.2 `internal/httpclient/` generalization trigger: EXTRACT NOW *(Q2 → ADR-0177 + ADR-0150 + ADR-0159 in-place AMENDMENTs)*

Per Q2 (httpclient extraction trigger) decision: EXTRACT NOW. The oauth2 filter is the THIRD outbound-HTTP consumer per the ADR-0159 §Future Work forward-pointer (after `internal/jwks/Fetcher` at phase 17 + `extauthz/check.go`'s `httpAuthClient` at phase 18.1). NEW top-level `internal/httpclient/` package (~150-250 LoC primitive wrapping `http.Client` + a `RetryPolicy` envelope + a `Timeout` envelope). Refactor `extauthz/check.go` + `internal/jwks/Fetcher` to consume it (~200-300 LoC cross-phase delta). TWO IN-PLACE §Decision AMENDMENTs per ADR-0044: ADR-0150 (jwks framework primitive — refactored to consume `internal/httpclient/`) + ADR-0159 (extauthz `httpAuthClient` — refactored to consume `internal/httpclient/`). ONE NEW ADR for the httpclient primitive itself (ADR-0177). Closes the ADR-0159 §Future Work forward-pointer load-bearing. Rationale: the third-consumer trigger is the canonical generalization-trigger per ADR-0044 deferral-to-second-consumer discipline; oauth2 is the third; extract is the right call. The phase-18.1 (b)-disposition rationale (defer-until-second-consumer) does NOT apply here — that rationale is for one-off primitives that may not have a second consumer; for HTTP-outbound, the third consumer demonstrates the pattern.

### 2.3 SDS posture: Filesystem-path SDS only *(Q3 → ADR-0178)*

Per Q3 (SDS posture) decision: filesystem-path SDS only. Honor `SdsSecretConfig.SdsConfig.ConfigSource.Path` (or equivalent path-selector — SPEC author pins exact variant via §10 open question §20.P6) pointing at a `Secret` proto JSON/YAML file containing `generic_secret.secret.inline_string` (or `inline_bytes`). Other `ConfigSource` variants — `api_config_source` (xDS-style SDS-over-gRPC) + `ads` (aggregated discovery service) + `path_config_source` (path-watched ConfigSource pointer) + the various resource_locator variants — all PARSE-REJECT envoy-go-strict. envoy-go has no xDS control-plane (deferred per §8 item 13); the filesystem-path SDS is the only path-of-least-resistance for secret-material loading. Loader implementation per Q7 + Q8 — see §2.7 + §2.8. ADR-0178 anchors the filesystem-path SDS + the PARSE-REJECT for non-filesystem ConfigSource variants.

### 2.4 Token-encryption envelope: AES-GCM default-on, honor `disable_token_encryption=true` *(Q4 → ADR-0182)*

Per Q4 (token-encryption envelope) decision: Proto-faithful AES-GCM default-on, honor `disable_token_encryption=true` to skip. Encrypt `BearerToken` + `IdToken` (deferred) + `RefreshToken` cookie values using AES-GCM with KDF derived from `hmac_secret`. ~150-200 LoC stdlib `crypto/aes` + `crypto/cipher`. Algorithm: AES-256-GCM with a 12-byte random nonce per encryption (carried in-cookie as the leading 12 bytes; mirrors stdlib `crypto/cipher.NewGCM` defaults — SPEC author pins against reference Envoy v1.37.2 at §10 open question §20.P5). KDF: HKDF-SHA256 (or HMAC-SHA256 single-pass — SPEC author pins via §20.P5). The `disable_token_encryption=true` override skips encryption entirely (plaintext cookie values; honors the proto-faithful skip-path; mirrors reference Envoy semantics). The helper lives filter-local at `oauth2/tokens.go` (NOT extracted to a shared package — second-consumer trigger defers per phase-18.1 (b)-disposition rationale). ONE NEW ADR (ADR-0182) for the AES-GCM mode + nonce-derivation + KDF discipline.

### 2.5 Per-route discipline: No per-route support (listener-scoped only) *(Q5 → NO new canonical, NO ADR-0125 amendment)*

Per Q5 (per-route discipline) decision: No per-route support — listener-scoped only. The v1.37.x oauth2 proto has NO `OAuth2PerRoute` message at all (confirmed at the proto reference `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.37.0/extensions/filters/http/oauth2/v3/oauth.pb.go`). PARSE-REJECT if anyone places an oauth2 TPFC under a route/virtualHost (enforcement point TBD per §10 open question §20.P7 — HCM parse-time vs `oauth2.New` factory-time; lean is HCM parse-time consistent with other listener-scoped filters). NO new ADR-0125 canonical; NO ADR-0125 amendment. THIRD CONSECUTIVE §9 family-row to REUSE-or-skip the ADR-0125 roster (after phase 18 + phase 19 both REUSED the 5th canonical; phase 20's REUSE-by-absence is a STRONGER form — there is no per-route surface at all). Strengthens the ADR-0125 "roster is not monotonic" lesson WITHOUT amendment (the absence itself is the lesson).

### 2.6 ADR-0045 split-readiness leaning: MEDIUM-HIGH *(Q6 → SPEC author's call)*

Per Q6 (split-readiness leaning) decision: MEDIUM-HIGH. LoC envelope sits between phase-17 jwt_authn (3855 single-row) and phase-19 ext_proc (5000+ split). SPEC author makes the final call after the empirical-pin scrape closes the LoC envelope. Most-likely split-by-feature-class shape per §1.4. The brainstorm does NOT pre-commit per ADR-0045 deferral-to-SPEC discipline.

### 2.7 Filesystem-SDS location: NEW `internal/sdsfile/` shared package *(Q7 → ADR-0178 anchor)*

Per Q7 (filesystem-SDS location) decision: NEW `internal/sdsfile/` (shared). Anticipates future filters needing the same shape (jwt_authn TLS-trust-store reload; future ext_authz mTLS; future ratelimit gRPC TLS). Small primitive (~80-120 LoC). NOT folded into an existing internal package (no natural home — `internal/jwks/` is JWKS-specific; `internal/grpcclient/` is gRPC-specific). NOT filter-local (oauth2 is the first consumer but anticipates ≥1 future). ONE NEW ADR (ADR-0178; bundles with Q8 fsnotify reload — see §2.8). The package exposes `Load(path string) (*Loader, error)` returning a `*Loader` with `Bytes() []byte` + `OnChange(callback func([]byte))` (or atomic-swap discipline — SPEC author pins).

### 2.8 SDS reload posture: fsnotify-based hot-reload *(Q8 → ADR-0178 same anchor)*

Per Q8 (SDS reload posture) decision: fsnotify-based hot-reload. Watch the secret file via `fsnotify` for changes; reload + atomic-swap the in-memory secret on Write events. ~80 LoC extra. Likely bundled into the `internal/sdsfile/` ADR (ADR-0178) rather than a separate ADR. The atomic-swap discipline: a `sync.RWMutex` or `atomic.Pointer[[]byte]` wrapping the in-memory copy; readers (HMAC + AES-GCM-KDF + client_secret consumers) call `Bytes()` per-request, so a mid-stream secret rotation takes effect on the NEXT request without filter restart. Robust against partial-write races (the typical write-then-rename atomic-replace pattern from operators; fsnotify Write event is the canonical trigger per the fsnotify upstream documentation).

### 2.9 Cookie HMAC scheme: HMAC-SHA256(hmac_secret, host + BearerToken + OauthExpires) *(Q9 → ADR-0179)*

Per Q9 (cookie HMAC scheme) decision: HMAC-SHA256(hmac_secret, host + BearerToken + OauthExpires). Reference Envoy default composition. Standard stdlib `crypto/hmac` + `crypto/sha256`. The Set-Cookie response's `OauthHMAC` cookie value is computed as `base64(HMAC-SHA256(hmac_secret, host + BearerToken + OauthExpires))` where `host` is the request Host header + `BearerToken` is the (encrypted-or-plaintext-depending-on-Q4) BearerToken cookie value + `OauthExpires` is the unix-timestamp string. On inbound: recompute the HMAC and compare via `subtle.ConstantTimeCompare`; mismatch → treat as no-cookie + 302 redirect to authorization_endpoint. ONE NEW ADR (ADR-0179) canonicalizing the composition order + the host-binding rationale + the constant-time-compare discipline. SPEC author pins the byte-exact composition ordering against reference Envoy v1.37.2 (§10 open question §20.P4 — may include `CookieAttributeNames` fields beyond the three above).

### 2.10 Token-endpoint POST body format under URL_ENCODED_BODY: `client_secret_post` *(Q10 → ADR-0185)*

Per Q10 (token-endpoint POST body format) decision: `client_secret_post` (in body). `client_id` + `client_secret` embedded in `application/x-www-form-urlencoded` body alongside `grant_type=authorization_code` + `code=<code>` + `redirect_uri=<redirect_uri>`. Standard OAuth 2.0 `client_secret_post` auth method. NOT `client_secret_basic` (BASIC_AUTH — out of envelope per Q1). The exact field-ordering byte-exact against reference Envoy v1.37.2 is a SPEC §10 empirical-pin (§20.P10 — likely surfaced as ADR-0185 if the ordering is load-bearing for upstream-fidelity). ONE NEW ADR (ADR-0185 likely; reserve ADR-0044 escape-valve at IMPL-time if scope balloons) for the `client_secret_post` body format + field-ordering canonicalization.

### 2.11 Error-handling posture *(self-answered → ADR-0180)*

NOT requiring live input — core oauth2 semantics, consumed proto-faithful. On token_endpoint POST failure (transport error, non-2xx status, malformed JSON response): return 500 to the downstream via `SendLocalReply` (counter `oauth_failure`). On callback with bad/missing/mismatched state cookie: return 401 to the downstream (counter `oauth_unauthorized_rq` — CSRF defense). On AES-GCM cookie decryption failure (corruption, key rotation race): treat as no-cookie + 302 to authorization_endpoint (counter `cookie_decrypt_failure` — envoy-go-strict departure from Envoy reference; flag for ADR-0080 discipline review at SPEC time). On valid cookies with expired BearerToken but valid RefreshToken: attempt silent refresh-token rotation (counter `oauth_refreshtoken_success` or `oauth_refreshtoken_failure`). On `pass_through_matcher` hit: bypass oauth2 entirely (counter `oauth_passthrough`). On successful sign-in completion (callback flow + token_endpoint POST returns 2xx): 302 to origin with Set-Cookie envelope (counter `oauth_success`). On sign-out flow completion (`signout_path` hit): clear cookies + 302 to origin (counter `signout_completed`).

### 2.12 Stat surface anchor + listener-scoped-only stats hypothesis *(self-answered → ADR-0181 or ADR-0180)*

Per §5 below + SPEC §10 pin §20.P8. Hypothesis: ~8 new base counters under the HCM-rooted SN2-reuse namespace `http.<HCM_stat_prefix>.oauth2.<counter>` (the canonical Envoy oauth2_stats_macro family per reference Envoy v1.37.2 scrape). Names mirror upstream Envoy `oauth2_stats_macro` where possible. Stat-surface growth estimate: 86 → ~94 names (claim approximate — SPEC-time empirical pin confirms). Per-route stats N/A (no per-route surface per Q5). ADR-0181 or ADR-0180 anchors the stat surface.

### 2.13 Deny-path wire shape *(self-answered → ADR-0180)*

On a 302 redirect to authorization_endpoint (the dominant "deny" — really a "challenge"): `SendLocalReply(302, "", {Location: <authorization_endpoint URL with client_id + redirect_uri + scope + resource + state>, Set-Cookie: <state-cookie>})`. On a 302 redirect after `signout_path` cookie-clear: `SendLocalReply(302, "", {Location: <origin or deny_redirect_matcher target>, Set-Cookie: <cookie-clearing Set-Cookie headers with Max-Age=0>})`. On 401 (bad state cookie, CSRF defense): `SendLocalReply(401, <body>, <headers>)` — body shape per §10 open question §20.P9 (likely "Unauthorized" or similar; pin against reference Envoy). On 500 (token_endpoint POST failure): `SendLocalReply(500, <body>, <headers>)` — body shape per §20.P9. All redirects use Phase-04 HCM `SendLocalReply` + Location header (no async; the 302 is synchronous from header-decode). The token_endpoint POST during callback handling uses Phase-09 async-resume to avoid blocking the worker. `deny_redirect_matcher` integration: when set, a denied request matching the matcher gets the 302 instead of being permitted. ADR-0180 anchors the four-disposition deny-path wire shape.

---

## 3. Framework-survey result — TWO NEW package-level + ONE NEW filter-local + TWO IN-PLACE AMENDMENTs + 5 REUSES

Phase 20 introduces TWO NEW package-level primitives + ONE NEW filter-local helper + TWO IN-PLACE §Decision AMENDMENTs to prior ADRs per ADR-0044 in-place edit discipline. The framework survey evaluated reuse of phase-09-through-19.2 primitives BEFORE proposing new — per phase-16 §10 lesson (a) + lesson (d) + phase-17/18/19 §3 discipline. Findings:

- **Phase-04 HCM `SendLocalReply` + Location header**: **REUSED** — for the 302 redirects to authorization_endpoint + sign-out + 401 + 500 deny-path emission (§2.13).
- **Phase-09 `time.AfterFunc` + `cb.ContinueDecoding` async-resume primitives**: **REUSED** — for the token_endpoint POST during callback-flow path handling. The other three flows (sign-in / sign-out / pass-through) complete synchronously from header-decode.
- **Phase-13 ADR-0128 decode-side body-buffering**: NOT REUSED (oauth2 has no request-body inspection; the OAuth 2.0 callback's `code` arrives via the GET query string in standard usage).
- **Phase-14 ADR-0131 `EncoderFilterCallbacks.OverwriteBody`**: NOT REUSED (oauth2 is decoder-only; no encode-side participation).
- **Phase-16 ADR-0142 matcher-engine at `internal/matcher/`**: NOT REUSED at MVP (oauth2 uses simpler `*matcherv3.PathMatcher` + `[]*HeaderMatcher`, not `xds.type.matcher.v3.Matcher`).
- **Phase-16 ADR-0144 TLS-principal accessor `DownstreamPrincipal()`**: NOT REUSED — oauth2 has no TLS-principal interaction.
- **Phase-17 ADR-0150 `internal/jwks/Fetcher` outbound-HTTP structure**: NOT CONSUMED for id_token verification (id_token is deferred per Q1) — but REFACTORED in-place per ADR-0044 §Decision AMENDMENT to consume `internal/httpclient/` (per Q2 EXTRACT NOW). See §3.4.
- **Phase-17 ADR-0151 `internal/jwt/` verifier**: NOT REUSED — id_token verification is deferred per Q1.
- **Phase-18.2 ADR-0158 `internal/grpcclient/Dialer`**: NOT REUSED — oauth2 has no gRPC consumer.
- **Phase-18.1 ADR-0159 `extauthz/check.go` `httpAuthClient`**: REFACTORED in-place per ADR-0044 §Decision AMENDMENT to consume `internal/httpclient/` (per Q2 EXTRACT NOW; closes the ADR-0159 §Future Work forward-pointer). See §3.5.
- **Phase-18.2 ADR-0165 6 new `DecoderFilterCallbacks` methods**: NOT REUSED — oauth2 has no TLS/principal attribute envelope to populate.
- **Phase-18.2 ADR-0166 plaintext h2c upstream relaxation**: NOT REUSED — oauth2's token_endpoint POST is HTTP (not gRPC); the cluster-manager defaults handle it without h2c-specific relaxation.
- **Phase-19.1 ADR-0169 bidi-stream wrapper**: NOT REUSED — oauth2 has no bidi-stream consumer.
- **Phase-19.1 ADR-0170 JSON codec**: NOT REUSED — oauth2's token_endpoint response parsing uses stdlib `encoding/json` directly (not protojson).
- **Phase-19.1 ADR-0174 encode-side callback symmetry**: NOT REUSED — oauth2 is decoder-only.
- **Phase-19.2 ADR-0175 encode-side body-buffering primitive**: NOT REUSED — oauth2 is decoder-only.
- **ADR-0125 8 canonical per-route patterns**: NO NEW canonical needed — REUSE-by-absence per Q5 (§4).

**Zero-delta is NOT feasible** for phase 20 — the `internal/httpclient/` extraction fires per the ADR-0159 §Future Work third-consumer trigger; the `internal/sdsfile/` primitive is required for filesystem-watched Secret loading; the AES-GCM token-encryption helper is required for Q4's default-on encryption.

### 3.1 NEW: `internal/httpclient/` framework primitive *(ADR-0177)*

A new top-level `internal/httpclient/` package (~150-250 LoC primitive). Public surface (the SPEC author confirms the exact signature at §10 open question §20.P1):

```go
type Client struct { /* http.Client + RetryPolicy + Timeout envelope */ }
type RetryPolicy struct { /* attempts; backoff; retry-on-status-set */ }
func NewClient(timeout time.Duration, retry RetryPolicy) *Client
func (c *Client) Do(req *http.Request) (*http.Response, error)
```

Wraps `http.Client` + a `RetryPolicy` envelope + a `Timeout` envelope. Consumed by oauth2's `endpoint.go` (token_endpoint POST + future authorization_endpoint discovery), refactored-jwks (`internal/jwks/Fetcher`), refactored-extauthz (`extauthz/check.go`'s `httpAuthClient`). Closes the ADR-0159 §Future Work forward-pointer ("third outbound-HTTP consumer triggers `internal/httpclient/` extraction"). Cross-phase-reusable for any future outbound-HTTP consumer (future ext_authz mTLS, future jwt_authn alternative-issuer fetch, future ratelimit). ADR-0177 anchors the httpclient primitive + the third-consumer-trigger rationale + the cross-phase reuse intent.

### 3.2 NEW: `internal/sdsfile/` framework primitive *(ADR-0178)*

A new top-level `internal/sdsfile/` package (~160-200 LoC primitive — filesystem-path Secret reader + fsnotify hot-reload bundled per Q7 + Q8). Public surface (the SPEC author confirms the exact signature at §10 open question §20.P6):

```go
type Loader struct { /* atomic.Pointer[[]byte] + *fsnotify.Watcher */ }
func NewLoader(path string) (*Loader, error)
func (l *Loader) Bytes() []byte
func (l *Loader) Close() error
```

Honors `SdsSecretConfig.SdsConfig.ConfigSource.Path` pointing at a `Secret` proto JSON/YAML file containing `generic_secret.secret.inline_string`. Watches via `fsnotify` for Write events + reloads + atomic-swaps the in-memory copy. Consumed by oauth2's `cookies.go` (hmac_secret) + `tokens.go` (AES-GCM KDF) + `endpoint.go` (client_secret). Cross-phase-reusable for any future filesystem-SDS consumer (jwt_authn TLS-trust-store reload, future ext_authz mTLS, future ratelimit gRPC TLS). ADR-0178 anchors the filesystem-path SDS + the fsnotify hot-reload + the atomic-swap discipline + the PARSE-REJECT for non-filesystem ConfigSource variants.

### 3.3 NEW: filter-local AES-GCM token-encryption helper *(ADR-0182)*

A filter-local helper at `oauth2/tokens.go` (~150-200 LoC stdlib `crypto/aes` + `crypto/cipher` + KDF + nonce-derivation). Public surface (filter-local only — not extracted to a shared package at phase 20 per second-consumer-trigger deferral):

```go
func encrypt(plaintext, key []byte) ([]byte, error)
func decrypt(ciphertext, key []byte) ([]byte, error)
func deriveKey(hmacSecret []byte) []byte // HKDF-SHA256 or HMAC-SHA256 single-pass
```

AES-256-GCM with a 12-byte random nonce per encryption (carried in-cookie as leading 12 bytes). KDF derives the 32-byte AES-256 key from `hmac_secret` via HKDF-SHA256 (or HMAC-SHA256 single-pass — SPEC author pins at §20.P5). The `disable_token_encryption=true` config flag bypasses the encrypt/decrypt path entirely (plaintext cookie values). ADR-0182 anchors the AES-GCM mode + nonce-derivation + KDF discipline + the skip-path semantics.

### 3.4 IN-PLACE: ADR-0150 §Decision AMENDMENT *(per ADR-0044)*

The ADR-0150 `internal/jwks/Fetcher` framework primitive (phase 17) is refactored in-place to consume `internal/httpclient/` (per Q2 EXTRACT NOW). The §Decision section of ADR-0150 gains an AMENDMENT paragraph documenting the refactor (the `Fetcher` no longer owns its own `http.Client`; instead it takes a `*httpclient.Client` constructor argument). The §Consequences section gains a paragraph documenting the cross-phase-consumer disposition. Per ADR-0044 in-place edit discipline — NOT a new ADR; the existing ADR-0150 evolves in-place with the AMENDMENT paragraph clearly dated and cross-referenced to phase 20 + ADR-0177. ~50-100 LoC refactor delta in `internal/jwks/`.

### 3.5 IN-PLACE: ADR-0159 §Decision AMENDMENT *(per ADR-0044)*

The ADR-0159 `extauthz/check.go` `httpAuthClient` framework primitive (phase 18.1) is refactored in-place to consume `internal/httpclient/` (per Q2 EXTRACT NOW). The §Decision section of ADR-0159 gains an AMENDMENT paragraph documenting the refactor (the `httpAuthClient` no longer owns its own `http.Client`; instead it takes a `*httpclient.Client` constructor argument). The §Future Work section gains a closure paragraph documenting that the "third outbound-HTTP consumer triggers `internal/httpclient/` extraction" forward-pointer is CLOSED at phase 20. Per ADR-0044 in-place edit discipline — NOT a new ADR; the existing ADR-0159 evolves in-place with the AMENDMENT + the §Future Work closure paragraph clearly dated and cross-referenced to phase 20 + ADR-0177. ~100-150 LoC refactor delta in `internal/filter/http/extauthz/`.

### 3.6 Framework reuses — 5 items

Phase 20 demonstrates the reusability of FIVE prior framework primitives (or absence-of-consumption for primitives whose consumption is deferred), mirroring the phase-15 bandwidth_limit + phase-18 ext_authz + phase-19 ext_proc reusability-demonstration discipline:

- **Phase-04 HCM `SendLocalReply` + Location header**: REUSED for the four-disposition deny-path emission (302 redirect to authorization_endpoint / 302 redirect after sign-out / 401 bad-state-cookie / 500 token_endpoint-failure).
- **Phase-09 async-resume primitive**: REUSED for the token_endpoint POST during callback-flow handling (parks decode goroutine on resume channel; mirrors phase-18.x + phase-19.x async-resume leg).
- **Phase-16 ADR-0144 `DownstreamPrincipal()` TLS-principal accessor**: NOT CONSUMED — oauth2 has no TLS-principal interaction (the OAuth 2.0 authentication is browser-facing redirect-based, not TLS-client-cert-based). Documented as NOT-consumed for cross-phase audit clarity.
- **Phase-17 ADR-0150 `internal/jwks/Fetcher`**: NOT CONSUMED for id_token validation (id_token deferred per Q1) — but REFACTORED in-place per ADR-0044 §Decision AMENDMENT to consume `internal/httpclient/`. The REFACTOR is a delta, not a consumption.
- **Phase-17 ADR-0151 `internal/jwt/` verifier**: NOT CONSUMED — id_token validation deferred per Q1. Documented as NOT-consumed for cross-phase audit clarity (load-bearing for the future id_token-enabling phase).

No ADR is anchored for the reuses themselves (they are reuses, not deltas); ADR-0180 (the phase-20 layout ADR) cites all five reuses + their cross-phase-consumer framing.

---

## 4. Per-route shape — REUSE-by-absence (NO new canonical, NO ADR-0125 amendment)

The ADR-0125 canonical per-route discipline roster after phase 19 has 8 entries (§(i) through §(xiii) amendment paragraphs — phase 18 + phase 19 added NONE):
1. cors no-per-route
2. fault / local_ratelimit / csrf data-only TPFC
3. header_mutation multi-tier all-tier
4. local_ratelimit INDEPENDENT-stats stateful
5. buffer / compressor / ext_authz / ext_proc disabled-OR-override-bool-in-oneof
6. bandwidth_limit bare-message-via-TPFC + code-level-required
7. rbac wrapper-with-reserved-field-and-single-optional-sub-message absent-implies-disabled
8. jwt_authn oneof{disabled(bool) | requirement_name(string)} string-reference-delegation

Phase 20's per-route shape is **REUSE-by-absence**: the v1.37.x oauth2 proto has NO `OAuth2PerRoute` message at all. Listener-scoped only. PARSE-REJECT if anyone places an oauth2 TPFC under a route/virtualHost (enforcement point TBD per §10 open question §20.P7 — lean is HCM parse-time consistent with other listener-scoped filters; alternative is `oauth2.New` factory-time rejection).

**Phase 20 lands NO ADR-0125 amendment paragraph.** This is the THIRD CONSECUTIVE §9 family-row (after phase 18 per ADR-0163 + phase 19 per ADR-0173) to NOT extend the ADR-0125 roster. Phase 18 + phase 19 REUSED the 5th canonical (compressor/ext_authz/ext_proc-style disabled-OR-overrides-oneof); phase 20's REUSE-by-absence is a STRONGER form of the lesson — there is no per-route surface at all, so the listener-scoped-only enforcement is itself a parse-time PARSE-REJECT discipline. The ADR-0125 roster does NOT grow monotonically; phase 20 strengthens this lesson WITHOUT amendment (the absence itself is the lesson — mirrors phase-18 ADR-0163 + phase-19 ADR-0173 "explicit no-amendment-paragraph" decision).

SPEC §10 pin §20.P7 confirms the exact enforcement point (HCM parse-time vs `oauth2.New` factory-time) — the brainstorm's hypothesis is HCM parse-time consistent with the existing listener-scoped-filter precedent (the SPEC author may confirm by inspecting the existing HCM TPFC-placement validation gate).

---

## 5. Stat surface hypothesis

Per §2.12 + the self-answered Decision → ADR-0180 / ADR-0181 anchor. Phase 20 grows the stat-table from 86 names (post-phase-19.2) to ~94 names:

**Filter-wide counters** (per HCM stat_prefix; the canonical Envoy `oauth2_stats_macro` family per reference Envoy v1.37.2 scrape — exact roster + dispositions are SPEC §10 pin §20.P8):

- `oauth2.oauth_unauthorized_rq` — counter; per bad/missing state cookie at callback path → 401.
- `oauth2.oauth_failure` — counter; per token_endpoint POST failure → 500.
- `oauth2.oauth_passthrough` — counter; per `pass_through_matcher` hit (bypass).
- `oauth2.oauth_success` — counter; per successful sign-in completion (callback-flow + token_endpoint POST returns 2xx).
- `oauth2.oauth_refreshtoken_success` — counter; per refresh-token rotation OK.
- `oauth2.oauth_refreshtoken_failure` — counter; per refresh-token rotation failed.
- `oauth2.signout_completed` — counter; per sign-out flow completed.
- `oauth2.cookie_decrypt_failure` — counter; per AES-GCM token decryption failed (envoy-go-strict departure from Envoy reference; FLAG FOR ADR-0080 DISCIPLINE REVIEW at SPEC time per §10 open question §20.P11).

**Namespace anchor**: HCM-rooted `http.<HCM_stat_prefix>.oauth2.<counter>` (SN2-reuse hypothesis — the existing HCM-stat-prefix Prometheus tag-extractor handles this verbatim; NO new SN-flattening rule; mirrors the phase-16 rbac + phase-17 jwt_authn + phase-18.2 ext_authz + phase-19.x ext_proc SN2-reuse RATIFICATIONS). SPEC §10 pin §20.P8 RATIFIED-PENDING — SPEC-time / impl-time empirical scrape against reference Envoy v1.37.2 confirms or refines, per phase-16 §10 lesson (c).

**Per-route stats discipline**: N/A (no per-route surface per Q5 + §4). All stats are listener-level filter-wide.

**Stat surface count summary table**:
| Phase | Filter | Stat surface delta |
|---|---|---|
| 13 | buffer | 29 → 29 (+0; zero stat extension) |
| 14 | compressor | 29 → 46 (+17) |
| 15 | bandwidth_limit | 46 → 60 (+14) |
| 16 | rbac | 60 → 64 (+4 base; per-policy scales) |
| 17 | jwt_authn | 64 → 71 (+7 base) |
| 18 | ext_authz | 71 → 77 (+6 base; no per-* scaling) |
| 19 | ext_proc | 77 → 86 (+9 base; no per-* scaling) |
| **20** | **oauth2** | **86 → ~94 (+~8 base; no per-* scaling)** |

---

## 6. Differential fixture envelope — `0024-http-oauth2`

~6-8 scenarios anticipated (matches the phase-13/14/15/16/17 baseline; lighter than phase-18/19's 8-10 because phase-20 has no service-mode axis). The fixture exercises the three flows + the pass-through + the refresh-token rotation + the disable-encryption skip-path + the deny-path variants:

| # | Scenario | Service mode | Backend script | Expected disposition | Counter delta assertion |
|---|---|---|---|---|---|
| 1 | `sign_in_happy_path` | OAuth backend up | authorization_endpoint redirects with `code`; token_endpoint returns 2xx with access_token + refresh_token + expires_in | unauthenticated GET → 302 to authorization_endpoint → callback → token_endpoint POST → 302 to origin + Set-Cookie envelope (5 cookies) | `oauth_success=1` |
| 2 | `cookie_present_passthrough` | (no oauth backend call) | (n/a) | authenticated GET with valid 5-cookie envelope → proxied to upstream with `Authorization: Bearer <decrypted>` header injection | (no counter deltas; passthrough) |
| 3 | `pass_through_matcher` | (no oauth backend call) | (n/a) | request matching `pass_through_matcher` → bypass oauth2 entirely | `oauth_passthrough=1` |
| 4 | `refresh_token_rotation` | OAuth backend up | token_endpoint returns 2xx for refresh request | cookie present but expired BearerToken + valid RefreshToken → silent refresh + retry → proxied upstream | `oauth_refreshtoken_success=1` |
| 5 | `signout_flow` | (no oauth backend call) | (n/a) | request to `signout_path` → cookie clearing (Max-Age=0) + 302 to origin | `signout_completed=1` |
| 6 | `bad_state_401` | (no oauth backend call) | (n/a) | callback with mismatched/invalid state cookie → 401 | `oauth_unauthorized_rq=1` |
| 7 | `token_endpoint_failure_500` | OAuth backend up but returns 500 | token_endpoint returns 5xx | callback → token_endpoint POST → 500 SendLocalReply | `oauth_failure=1` |
| 8 | `disable_token_encryption_true` | OAuth backend up | (as scenario 1 but `disable_token_encryption=true` set) | cookies stored in plaintext (no AES-GCM); verifies skip-path | `oauth_success=1` |

**Test-helpers**: ONE new helper under `test/helpers/oauthbackend/` — an in-process OAuth 2.0 authorization-server backend (mock `authorization_endpoint` issuing the redirect-with-code; mock `token_endpoint` accepting POSTs + returning JSON with `access_token` / `refresh_token` / `expires_in`; optional-stub JWKS endpoint for future id_token reuse). Likely shape: stdlib `net/http/httptest`-based server, configurable per-scenario with scripted responses keyed by request method + path. The filesystem-SDS Secret file lives at `test/fixtures/0024-http-oauth2/secrets/hmac.json` + `client_secret.json` as `Secret` proto JSON.

**26th fuzzer**: `FuzzOAuth2ConfigParse` at `internal/filter/http/oauth2/fuzz_test.go` (current count 25 post-phase-19.2 → 26 post-phase-20). Corpus seeds anticipated (each `OAuth2Config` field × valid/invalid variants; cookie-name customization × valid/invalid; `SdsSecretConfig` path × valid/invalid; matcher-engine variants × valid/invalid). 30s/seed under the existing fuzzer-time-budget envelope.

**If phase 20 splits per ADR-0045** (§1.4 + Q6 MEDIUM-HIGH): the fixture splits correspondingly — the framework-refactor + sign-in sub-phase covers scenarios 1-3 + 6 + (partially) 8; the AES-GCM + refresh + sign-out sub-phase covers scenarios 4 + 5 + 7 + (fully) 8; the fuzzer + test-helpers split correspondingly. The SPEC author owns this disposition.

---

## 7. Anticipated ADRs — 7-9 ADRs (ADR-0177..ADR-0185; ADR-0044 escape-valve reserve)

ADR-0176 is the highest-numbered ADR landed in phase 19.2; ADR-0177 is the next-free per STATE.md. Phase 20 anticipates 7-9 ADRs (ADR-0177..ADR-0185) plus the TWO IN-PLACE §Decision AMENDMENTs to ADR-0150 + ADR-0159 (per ADR-0044 in-place edit discipline — not new ADR numbers). **Phase 20 anticipates NO ADR-0125 amendment paragraph** — the per-route shape is REUSE-by-absence (§4); THIRD CONSECUTIVE §9 row to do so (after phase 18 per ADR-0163 + phase 19 per ADR-0173).

**ADR-0177** — `internal/httpclient/` framework primitive. Closes ADR-0159 §Future Work forward-pointer ("third outbound-HTTP consumer triggers `internal/httpclient/` extraction") per Q2 EXTRACT NOW. AMENDS ADR-0150 in-place (jwks framework primitive refactored to consume) + AMENDS ADR-0159 in-place (extauthz `httpAuthClient` refactored to consume). NEW top-level `internal/httpclient/` package (~150-250 LoC). **Lands-in: SPEC §Context draft + IMPL Task 2.** Anchors §2.2 + §3.1 + §3.4 + §3.5.

**ADR-0178** — `internal/sdsfile/` filesystem-path Secret reader + fsnotify hot-reload (Q7 + Q8 bundled). NEW top-level `internal/sdsfile/` package (~160-200 LoC). PARSE-REJECT for non-filesystem ConfigSource variants. atomic-swap discipline. **Lands-in: SPEC §Context draft + IMPL Task 3.** Anchors §2.3 + §2.7 + §2.8 + §3.2.

**ADR-0179** — oauth2 HMAC cookie composition: HMAC-SHA256(hmac_secret, host + BearerToken + OauthExpires) canonicalization + host-binding rationale + constant-time-compare discipline (Q9). **Lands-in: SPEC §Context draft + IMPL Task 4.** Anchors §2.9.

**ADR-0180** — oauth2 authorization_endpoint / callback / token_endpoint state-machine + the 302-redirect / 401 / 500 deny-path wire shape + the three-flow dispatcher + the listener-scoped-only enforcement. Records the cross-phase NOT-CONSUMED dispositions for ADR-0144 + ADR-0150 + ADR-0151. **Lands-in: SPEC §Context draft + IMPL Task 5.** Anchors §2.1 + §2.5 + §2.11 + §2.13 + §3.6 + §4.

**ADR-0181** — oauth2 cookie envelope: 5-of-7 CookieNames consumed (BearerToken / OauthHMAC / OauthExpires / IdToken (deferred) / RefreshToken); 2-deferred-with-PKCE (oauth_nonce / code_verifier); Set-Cookie attribute discipline (Secure / HttpOnly / SameSite / Path); stat surface anchor (§5). **Lands-in: SPEC §Context draft + IMPL Task 6.** Anchors §1.1 item 3 + §2.12 + §5.

**ADR-0182** — AES-GCM token-encryption scheme: KDF from `hmac_secret` (HKDF-SHA256 vs single-pass HMAC pinned at §10), 12-byte random nonce per encryption, AES-256-GCM mode discipline, `disable_token_encryption=true` skip behavior (Q4). Filter-local helper at `oauth2/tokens.go`. **Lands-in: SPEC §Context draft + IMPL Task 7.** Anchors §2.4 + §3.3.

**ADR-0183** — refresh-token rotation timing + `default_refresh_token_expires_in` semantics + race-vs-rotation discipline (concurrent requests with same expired BearerToken + valid RefreshToken). **Lands-in: SPEC §Context draft + IMPL Task 8.** Anchors §4 + §5 (`oauth_refreshtoken_*` counters).

**ADR-0184** — sign-out flow: `signout_path` handling + cookie clearing (Set-Cookie Max-Age=0) + redirect-to-origin semantics + `deny_redirect_matcher` integration. **Lands-in: SPEC §Context draft + IMPL Task 9.** Anchors §5 (`signout_completed`).

**ADR-0185 (likely; reserve ADR-0044 escape-valve at IMPL-time if scope balloons)** — token_endpoint POST body format under URL_ENCODED_BODY: `client_secret_post` field-ordering canonicalization (Q10). Pins byte-exact field ordering against reference Envoy v1.37.2 for upstream-fidelity. **Lands-in: SPEC §Context draft + IMPL Task 10.** Anchors §2.10.

Likely landing split (per Q6 MEDIUM-HIGH lean): ADR-0177..ADR-0181 anchor at 20.1 SPEC; ADR-0182..ADR-0185 anchor at 20.2 SPEC.

ADR-0044 escape-valve held in reserve for ~0-2 impl-time-unanticipated ADRs per phase as working estimate. The MOST LIKELY phase-20 unanticipated-ADR surfaces: (i) the AES-GCM nonce-vs-deterministic choice if §20.P5 surfaces a divergence between random-per-encrypt (cookie carries IV) and KDF-deterministic (cookie omits IV); (ii) the HMAC composition byte-exact ordering against reference Envoy if §20.P4 surfaces additional fields; (iii) the listener-scoped-only enforcement point if §20.P7 surfaces a need for both HCM parse-time AND `oauth2.New` factory-time rejection.

---

## 8. Deferred items (~13-14 items; comparable to phase-19's 13-item + phase-17's 13-item)

For future phase consideration (none are blockers for closing row 20 phase-done; all auditable in the ADR-0040 deferral trail):

1. **BASIC_AUTH (`OAuth2Credentials.basic_auth`)** — OAuth 2.0 `client_secret_basic` auth method. DEFERRED: explicitly out of envelope per Q1 + Q10 (we picked `client_secret_post` in the body); a future operator-ergonomics phase MAY add the BASIC_AUTH alternative if operators request it.

2. **`OAuth2Config.retry_policy` (RetryPolicy proto)** — token_endpoint POST retry customization. DEFERRED: `internal/httpclient/` applies a SPEC-time-pinned default (likely zero-retry MVP per §10 open question §20.P1); future operator-ergonomics phase MAY add the retry surface.

3. **`OAuth2Config.end_session_endpoint`** — RP-initiated logout per OIDC. DEFERRED: explicitly out of envelope per Q1; couples to id_token (also deferred).

4. **id_token + validation** — OIDC id_token consumed + validated against the authorization-server JWKS. DEFERRED: out of envelope per Q1; consumes ADR-0150 jwks framework primitive + ADR-0151 jwt verifier (both NOT-consumed at phase 20). Future id_token-enabling phase reactivates them as load-bearing.

5. **PKCE (`use_pkce` + `code_verifier` cookie + `oauth_nonce` cookie)** — Proof Key for Code Exchange (RFC 7636). DEFERRED: out of envelope per Q1; the two cookies (`oauth_nonce`, `code_verifier`) are the 2-of-7 CookieNames not consumed at phase 20.

6. **`OAuth2Config.cookie_configs` (map[string]CookieConfig)** — per-cookie Set-Cookie attribute customization. DEFERRED: explicitly out of envelope per Q1; MVP uses listener-default Set-Cookie attributes (Secure / HttpOnly / SameSite=Strict / Path=/).

7. **`OAuth2Config.disable_id_token_set_cookie`** — gates whether the IdToken cookie is set. DEFERRED: couples to id_token (also deferred).

8. **`OAuth2Config.disable_access_token_set_cookie`** — gates whether the BearerToken cookie is set. DEFERRED: MVP always sets the BearerToken cookie; the disable surface is a future operator-ergonomics extension.

9. **`OAuth2Config.disable_refresh_token_set_cookie`** — gates whether the RefreshToken cookie is set. DEFERRED: MVP always sets the RefreshToken cookie when `use_refresh_token=true`; the disable surface is a future operator-ergonomics extension.

10. **`OAuth2Config.csrf_token_expires_in`** — CSRF-state-cookie expiry. DEFERRED: MVP uses a hard-coded reasonable default (likely 10 minutes — SPEC author pins against reference Envoy v1.37.2 default at §10 open question §20.P12); the operator-customization surface is future operator-ergonomics.

11. **`OAuth2Config.code_verifier_token_expires_in`** — code_verifier cookie expiry. DEFERRED: paired with PKCE (item 5).

12. **`OAuth2Config.cookie_domain`** — Set-Cookie Domain attribute. DEFERRED: MVP uses request Host (no Domain attribute = host-only cookie). The listener-default-host customization surface is future operator-ergonomics. Confirmed at §10 open question §20.P2.

13. **SDS non-filesystem ConfigSource variants (`api_config_source` / `ads`)** — xDS-control-plane SDS-over-gRPC + aggregated discovery. DEFERRED: PARSE-REJECT for now per Q3; future phase if/when xDS-control-plane lands in envoy-go.

14. **(Implicit, NEVER) per-route override** — the v1.37.x oauth2 proto has NO `OAuth2PerRoute` message at all. Listener-scoped only per Q5. NEVER deferred; this is a permanent absence not a deferral. PARSE-REJECT if anyone places oauth2 TPFC under a route/virtualHost.

The SPEC §10 exhaustive proto-field scrape may surface additional oauth2 fields not enumerated here — each gets a consumed-or-deferred disposition at SPEC time per the empirical-pin discipline.

---

## 9. Cross-references against phase-17 + phase-18 + phase-19 deferred-items lists — closure pickup

Phase-17 REVIEW.md §6 + phase-18.2 REVIEW.md §6 + phase-19 + phase-19.2 phase-done deferred lists enumerate the prior-phase deferred items. Phase 20 evaluates which opportunistically close vs continue deferred:

- **Phase-18.1 ADR-0159 §Future Work** — "third outbound-HTTP consumer triggers `internal/httpclient/` extraction": **CLOSED at phase 20** via Q2 EXTRACT NOW + ADR-0177 NEW + ADR-0150 + ADR-0159 in-place §Decision AMENDMENTs. This is the FIRST §9 family-row to CLOSE a phase-N-1 forward-pointer load-bearing.
- **Phase-17 ADR-0150 implicit forward-pointer to future httpclient primitive** — analogous to ADR-0159's explicit one: **CLOSED at phase 20** via the in-place ADR-0150 §Decision AMENDMENT.
- **Phase-17 dynamic-metadata family** (`payload_in_metadata`): CONTINUED DEFERRAL — oauth2 has no metadata-emit surface in the MVP envelope. NO PICKUP.
- **Phase-18 dynamic-metadata family**: CONTINUED DEFERRAL — oauth2 has no metadata-emit surface. NO PICKUP.
- **Phase-19 dynamic-metadata family** (ext_proc `metadata_options` + `filter_metadata` + `CommonResponse.dynamic_metadata`-style emit): CONTINUED DEFERRAL — oauth2 has no metadata-emit surface. NO PICKUP.
- **Phase-19.2 forward-pointer notes** (ext_proc streaming-mode body-mode + REQUEST_RESPONSE merge + message_timeout): CARRY FORWARD UNTOUCHED — none touched by phase-20 per B7. Documented in `BEHAVIOR_CONTRACT.md ### Phase 19.2 forward-pointer notes` continues without amendment.
- **Carryforward M (TLS-fixture phase deferral)** — unrelated to oauth2 sign-in flow: CARRY FORWARD UNTOUCHED. Remains documented in `BEHAVIOR_CONTRACT.md` §13.4.
- **Phase-16/17/18 `response_code_details` framework primitive joint forward-pointer**: NO PICKUP at MVP — oauth2's deny-path `response_code_details` would be `oauth_unauthorized` / `oauth_failure` / `oauth_passthrough` (or similar); ADDS to the joint-closure forward-pointer rather than closing it. Now blocked across phases 16 + 17 + 18 + 19 + 20.

**Forward-pointer net change for phase 20**: ONE major closure (ADR-0159 §Future Work — the load-bearing closure) + ONE minor closure (ADR-0150 implicit forward-pointer to future httpclient primitive). Phase 20 adds ~13-14 new deferred items (§8 above) + EXTENDS the id_token-and-jwks-and-jwt-verifier deferred-cluster (now blocked across phase 20 + any future id_token-enabling phase) + EXTENDS the SDS-non-filesystem deferred-cluster (new at phase 20 — item 13) + ADDS to the `response_code_details` joint-closure forward-pointer (now phases 16 + 17 + 18 + 19 + 20). The accumulating cross-phase deferred-clusters (dynamic-metadata, id_token-and-jwks, SDS-non-filesystem, response_code_details) are increasingly strong signals that dedicated framework phases for each are warranted.

---

## 10. BRAINSTORM-time open questions for SPEC-time resolution

Per phase-09..19 cadence (~5–18 questions; SPEC §11 empirical pins resolve them via reference Envoy v1.37.2 scrape per ADR-0004). Phase 20 anticipates 12 pins:

- **§20.P1 — Default RetryPolicy attempts for token_endpoint POST**: `RetryPolicy` proto field deferred per §8 item 2. Should `internal/httpclient/` apply a small default (e.g., 1 retry on 5xx) or zero-retry? Lean: zero-retry MVP, document at SPEC.
- **§20.P2 — Cookie domain default scoping**: without `cookie_domain` consumed (deferred per §8 item 12), what's the Set-Cookie Domain attribute? Lean: request Host (no Domain attribute = host-only cookie). Confirm at SPEC against reference Envoy v1.37.2.
- **§20.P3 — Callback request method**: typically `code` arrives in the GET query string. Does reference Envoy v1.37.2 support POST callbacks (body-carrying)? If yes, decode-side body-buffering via ADR-0128 may be needed.
- **§20.P4 — CookieAttributeNames in HMAC composition**: Envoy reference may include MORE than `host + BearerToken + OauthExpires` in some builds (e.g., `CookieAttributeNames`-derived suffixes). Pin against v1.37.2 source for byte-exact composition.
- **§20.P5 — AES-GCM nonce-derivation**: random 12 bytes per encryption (write to cookie as leading 12 bytes) vs deterministic from `hmac_secret` (no IV in cookie)? Lean: random (standard; cookie carries IV; mirrors stdlib `crypto/cipher.NewGCM` defaults). ADR-0182 pins.
- **§20.P6 — Exact SDS ConfigSource variant for filesystem-path**: which oneof arm of `core.ConfigSource` carries the path? Lean: `ConfigSource.Path` (or `PathConfigSource`); SPEC author scrapes proto reference at `/home/esa/go/pkg/mod/github.com/envoyproxy/go-control-plane/envoy@v1.37.0/config/core/v3/config_source.pb.go`.
- **§20.P7 — Listener-scoped only enforcement point**: at HCM parse-time (rejects route-level placement) or at `oauth2.New` factory-time (factory-time rejection)? Lean: HCM parse-time (consistent with other listener-scoped filters).
- **§20.P8 — Stat-counter roster + exact canonical names**: confirms the exact oauth2 stat names + scope + counter-vs-gauge dispositions. **SPEC-time / impl-time scrape** — the canonical RATIFIED-PENDING pin closure point per phase-16 §10 lesson (c).
- **§20.P9 — 401 + 500 SendLocalReply body shape**: byte-exact body content for the 401 (CSRF defense) + 500 (token_endpoint POST failure) emissions. Pin against reference Envoy v1.37.2.
- **§20.P10 — URL_ENCODED_BODY exact field-ordering under `client_secret_post`**: Lean: reproduce Envoy v1.37.2 ordering byte-for-byte for upstream-fidelity. ADR-0185 pins.
- **§20.P11 — `cookie_decrypt_failure` counter envoy-go-strict departure**: phase-20 introduces a counter that does not exist in reference Envoy (envoy-go-strict departure). Flag for ADR-0080 discipline review at SPEC time (per phase-18 §11 lesson).
- **§20.P12 — `csrf_token_expires_in` MVP default**: SPEC author pins against reference Envoy v1.37.2 default (likely 10 minutes per the proto's typical-default discipline).

Anticipated SPEC §11 scrape time: ~2-4 hours (12 pins; phase-19 was 13 pins resolved in similar wallclock). Most pins resolved IN-SESSION at SPEC drafting; some (P8 — stat surface; P11 — envoy-go-strict departure; P5 — AES-GCM nonce-derivation) RATIFIED-PENDING and closed at PLAN impl-time empirical scrape per phase-16 §10 lesson (c) + phase-19.2 §11.P-style closure precedent.

---

## 11. Phase-17 + phase-18 + phase-19 §10/§11 lessons applied

Per the explicit lessons-learned sections of phase-17 REVIEW.md §10 + phase-18.2 REVIEW.md §12 + phase-19 BRAINSTORM.md §11 + phase-19.2 phase-done REVIEW:

**Lesson (a) — Phase-17 `internal/jwks/` framework-extract precedent.** Phase-17 introduced `internal/jwks/Fetcher` as a NEW top-level package primitive — thin (~200 LoC), single-purpose, cross-phase-reusable at introduction time. Phase-20 directly templates this shape for the TWO NEW package-level primitives: `internal/httpclient/` (Q2 EXTRACT NOW) + `internal/sdsfile/` (Q7 + Q8). Same shape — thin primitive package, +1 ADR, multi-consumer at introduction or anticipated-multi-consumer.

**Lesson (b) — Phase-18 ext_authz envoy-go-strict departures.** Phase-18's `denied_with_reason` envoy-go-strict departure (counter that does not exist in reference Envoy) was an explicit ADR-0080 discipline review at SPEC time. Phase-20 has an analogous case at the `cookie_decrypt_failure` counter (per §5 + §20.P11 open question) — flag for ADR-0080 discipline review at SPEC time.

**Lesson (c) — Phase-19 D10-hypothesis-style D-series.** Phase-19 SPEC adopted D-series hypothesis discipline (D1..D12 at parent SPEC, refined at sub-phase SPECs) for empirical pins that could not be definitively settled at BRAINSTORM. Phase-20 SPEC should adopt the D-series hypothesis discipline for the AES-GCM scheme + fsnotify reload semantics + the HMAC composition byte-exact ordering (where empirical scrape against Envoy reference disambiguates).

**Lesson (d) — Phase-18 ADR-0159 forward-pointer-and-close discipline.** Phase-18 introduced ADR-0159 with an explicit §Future Work forward-pointer ("third outbound-HTTP consumer triggers `internal/httpclient/` extraction"); phase-20 CLOSES that forward-pointer via the in-place AMENDMENT pattern Q2 relies on. This demonstrates the forward-pointer-and-close discipline working as designed across two phases — the explicit forward-pointer at ADR-introduction time + the closure at the trigger-event.

**Lesson (e) — Phase-19.1/19.2 split templates the 20.1/20.2 split-by-feature-class line.** Phase-19 split by feature-class (19.1 = headers stages; 19.2 = body-stage activation) per parent SPEC §11.x. Phase-20's most-likely split (per Q6 MEDIUM-HIGH + §1.4) is similarly by feature-class (20.1 = framework refactor + sign-in flow; 20.2 = AES-GCM + refresh + sign-out). Direct template reuse.

**Lesson (f) — Phase-16/18/19 listener-scoped-only.** Phases 16/18/19 all REUSED the ADR-0125 5th canonical for per-route (or REUSED-by-absence in spirit). Phase-20 is the 3rd CONSECUTIVE §9 row to REUSE-or-skip ADR-0125 → strengthens the "ADR-0125 roster is not monotonic" lesson WITHOUT amendment (the absence itself is the lesson). Phase-20's REUSE-by-absence is a STRONGER form (no per-route surface at all, per Q5).

**Lesson (g) — Phase-19.2 SHA-fill follow-up discipline.** Phase-19.2 introduced the post-squash-merge STATE.md SHA-fill follow-up commit (`phase 19.2 IMPL follow-up: STATE.md SHA-fill (TBD → 1ddb661 post-squash)`). Phase-20 STATE.md re-advances now with `last-commit: <TBD>` placeholder + the parent session fills it post-squash-merge in the same session. Direct discipline reuse.

**Lesson (h) — subagent-driven IMPL discipline** (phase-16/17/18.x/19.x convention per project memory `feedback_execution_style.md`). Phase-20 IMPL session (later lifecycle stage) MUST use subagent-driven task execution.

**Lesson (i) — ADR-0045 split-readiness leaning is a brainstorm POSITION, not a SPEC mandate.** Phases 13–17 all flagged MODERATE and all landed single-row; phases 18 + 19 flagged HIGH and DID split; **phase 20 flags MEDIUM-HIGH** (§1.4 + Q6) — genuinely borderline. The SPEC author may rationally either-way; single-row landing at the lower end of the LoC estimate is viable per phase-17 jwt_authn at 3855 single-row.

**Lesson (j) — in-session SPEC scrape closure of empirical pins** (phase-18.2 §11.P13 + phase-19.x precedent). The SPEC author MUST run §20.P5 + §20.P7 + §20.P8 + §20.P10 empirical scrapes IN-SESSION at SPEC drafting (not at IMPL time) to determine the AES-GCM nonce + listener-scoped enforcement point + stat surface + token_endpoint body-format dispositions. The pin-closure protects against KNOWN escape-valve surfaces; the ADR-0044 escape-valve discipline remains in reserve for ORTHOGONAL surfaces.

---

## 12. Section closeout

Phase 20 brainstorm complete. Lifecycle exit: state 0 → 1 per ADR-0005 (BRAINSTORM.md authored; SPEC pending). Next session: a SPEC session (`superpowers:brainstorming` SCOPED to SPEC authoring per phase-09..19.2 cadence) authoring `docs/envoy-go/phases/20-http-filter-oauth2/SPEC.md`.

The SPEC session's defining IN-SESSION obligations:
1. **Resolve all 12 §10 empirical pins** (§10 above) via reference Envoy v1.37.2 scrape per ADR-0004 — including the exhaustive `OAuth2Config` / `OAuth2Credentials` / `CookieNames` proto-field roster.
2. **Land §1.1 amendment-block channel entries** for each pin's RATIFIED / REFUTED / PARTIAL / REFINED disposition; estimated 6-10 amendments per phase-16 §10 lesson (e).
3. **Anchor the 7-9 anticipated ADRs** (ADR-0177..ADR-0185 per §7 above) with §Context drafts. Plus the TWO IN-PLACE §Decision AMENDMENTs to ADR-0150 + ADR-0159 per ADR-0044.
4. **Confirm the REUSE-by-absence per-route classification** (§4) against the reference-Envoy v1.37.x oauth2 proto (verify no `OAuth2PerRoute` message exists) — and record the explicit no-ADR-0125-amendment decision at ADR-0180 (or analogous anchor).
5. **Define the ~6-8-scenario differential fixture** (`0024-http-oauth2`) with per-scenario expectations YAML, spanning the three flows + pass-through + refresh + sign-out + disable-encryption + deny-path variants + the ONE new test-helper.
6. **Confirm or refute the ADR-0045 split-by-surface release-valve decision** per §1.4 — SPEC-time call; phase 20's MEDIUM-HIGH split-readiness leaning is genuinely borderline. TWO split candidates surveyed (split-by-feature-class / single-row); SPEC author chooses.
7. **Assign the RATIFIED-PENDING pin closures** for §20.P5 (AES-GCM nonce-derivation) + §20.P7 (listener-scoped enforcement point) + §20.P8 (stat surface) + §20.P10 (token_endpoint body-format ordering) per phase-16 §10 lesson (c).
8. **Note that ROADMAP row 20 has ALREADY been added** at this BRAINSTORM commit (departure from the phase-09..19.2 ROADMAP-row-add-at-SPEC convention per explicit user-direction). SPEC author may add sub-phase rows `20.1` / `20.2` at SPEC time if the ADR-0045 split fires.

Next-skill (post-SPEC session): `superpowers:writing-plans` for the phase-20 PLAN.md authoring session (lifecycle state 2 → 3) — OR, if the SPEC author invokes the ADR-0045 split, the SPEC session exits with the sub-phase rows registered and the first sub-phase's PLAN as the next-skill.
