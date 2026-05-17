# Phase 20 SPEC — `envoy.filters.http.oauth2`

> **Lifecycle state:** SPEC.md authored; ROADMAP row `20` already at `in-progress` (registered at the phase-20 BRAINSTORM commit per the explicit user-direction departure from the phase-09..19.2 ROADMAP-row-add-at-SPEC convention; per-cell narrative updated at this SPEC commit with the SPEC-done annotation; status stays `in-progress` until IMPL phase-done with all 6 gates GREEN). Per ADR-0045 the SPEC author settled the split disposition: **SINGLE-ROW landing** (no sub-rows `20.1`/`20.2`; precedent at phase-17 jwt_authn 3855 single-row); LoC envelope re-estimated post-empirical-scrape at ~3400-3900 (slightly tighter than BRAINSTORM's ~3500-4000 due to the S1/S2/S5 SPEC-time simplifications). Successor session's skill is `superpowers:writing-plans` to author `PLAN.md` per the phase 09–19.2 precedent. This SPEC is the authoritative input to the phase-20 PLAN.

**Predecessor:** `docs/envoy-go/phases/20-http-filter-oauth2/BRAINSTORM.md` (the 10-question Q1-Q10 settled context + the §10 12 open-pins for SPEC-time resolution + the §11 lessons applied). The §10 empirical pins are resolved in this SPEC's §11; the §1.1 amendment-block records the BRAINSTORM corrections (AMEND-1..AMEND-7) driven by the empirical scrape. NO off-master prebrainstorm-notes branch was authored for phase 20.

**Scope (per BRAINSTORM §1.1 + the SPEC-time empirical-pin scrape):** phase 20 lands `envoy.filters.http.oauth2.v3.OAuth2` (the canonical Envoy OAuth 2.0 authentication filter, Standard envelope per Q1: sign-in + refresh + sign-out) as the THIRTEENTH §9 family-row under the 07.1 framework, the FIRST §9 family-row to **CLOSE a prior-phase load-bearing forward-pointer** (ADR-0159 §Future Work — third outbound-HTTP consumer triggers `internal/httpclient/` extraction). MVP envelope: ~17 of 26 `OAuth2Config` fields consumed; 4 of 5 `OAuth2Credentials` fields consumed (no BASIC_AUTH); 5 of 7 `CookieNames` fields consumed (no `oauth_nonce`, no `code_verifier` — both deferred with PKCE). **3 load-bearing flows** (sign-in / refresh / sign-out); **5 framework primitives** committed (2 NEW top-level + 1 NEW filter-local + 2 IN-PLACE §Decision AMENDMENTs); **listener-scoped only** (REUSE-by-absence per Q5 + §20.P7 RATIFIED). **Deny-path 302+401 only — NO 500 anywhere** (envoy-go-strict simplification per AMEND-3).

**ADR continuity:** Phase 19.2 closed at ADR-0176. Phase 20 anticipates 9 NEW ADRs (ADR-0177..ADR-0185) + 2 IN-PLACE §Decision AMENDMENTs (ADR-0150, ADR-0159). §Context drafts anchor at this SPEC commit; §Decision + §Consequences bodies land at each ADR's Lands-in-Task per ADR-0044. The 2 in-place AMENDMENT-anticipation paragraphs at ADR-0150 + ADR-0159 anchor at this SPEC commit; AMENDMENT bodies + ADR-0159 §Future Work closure land at IMPL. Next-free ADR after phase 20 SPEC commit stays **ADR-0186** (9 numbers consumed: ADR-0177..ADR-0185). ADR-0044 escape-valve held in reserve for ~0-2 impl-time-unanticipated ADRs.

**Authored:** 2026-05-17.

---

## 1. Purpose

Phase 20 lands `envoy.filters.http.oauth2.v3.OAuth2` — the canonical Envoy v1.37.2 OAuth 2.0 authentication filter delegating sign-in (302 challenge to authorization_endpoint + callback handling + token_endpoint POST + cookie envelope emission) + silent refresh-token rotation (cookie-validation-driven; deferred Set-Cookie emission on CONTINUE) + sign-out (path-matched; full envelope clearing) flows under the 07.1 framework — as the THIRTEENTH §9 production HTTP filter. It establishes the entire `internal/filter/http/oauth2/` package, the 5-cookie envelope discipline (BearerToken / OauthHMAC / OauthExpires / IdToken-deferred / RefreshToken), the AES-256-CBC token-encryption helper (per AMEND-1), the 5-input HMAC composition (per AMEND-2) + dual-encoding read (per S4), the 4-field auth-code + 3-field refresh-token POST templates (per AMEND-5), the listener-scoped-only enforcement, the 6-counter wire-exact stat surface (per AMEND-4 + S5), and the deny-path 302+401-only wire shape (per AMEND-3). **It also extracts TWO NEW top-level framework primitives** (`internal/httpclient/` per Q2 EXTRACT NOW + `internal/sdsfile/` per Q7+Q8 bundled) **and amends TWO prior-phase ADRs in-place** (ADR-0150 jwks + ADR-0159 extauthz to consume the new `internal/httpclient/`).

**5 architectural primitives that make this work:**

1. **NEW `internal/filter/http/oauth2/` package** owning the filter implementation. Package directory + Go-package identifier are both `oauth2` (single token underscore-stripped per ADR-0114; matches `localratelimit/`, `jwtauthn/`, `extauthz/`, `extproc/` precedent). 16 Go files (10 production + 6 test) per §6.11. Anticipated ~2850-3110 LoC filter proper. Exposes `TypeURL` (canonical `"type.googleapis.com/envoy.extensions.filters.http.oauth2.v3.OAuth2"`) + `New` (the `HTTPFilterFactory`) + `RegisterPerRouteValidator` (the HCM-parse-time PARSE-REJECT for route-level placement per §5.2). ADR-0180 codifies the package shape + state-machine + deny-path.

2. **NEW `internal/httpclient/` top-level framework primitive** per Q2 EXTRACT NOW. ~150-250 LoC. Options struct (Timeout / RetryPolicy / TLSConfig) + `Client.Do` synchronous wrapper over `http.Client`. **3 consumers at introduction time**: jwks Fetcher (post-ADR-0150 in-place AMENDMENT), extauthz httpAuthClient (post-ADR-0159 in-place AMENDMENT), oauth2 token_endpoint POST (NEW). **CLOSES ADR-0159 §Future Work forward-pointer** ("third outbound-HTTP consumer triggers `internal/httpclient/` extraction" — the third-consumer trigger fires exactly as ADR-0159 anticipated). ADR-0177 codifies.

3. **NEW `internal/sdsfile/` top-level framework primitive** per Q7+Q8 bundled. ~160-200 LoC + NEW go.mod dependency on `github.com/fsnotify/fsnotify`. `Watcher{New, Start, Current, Close}` over fsnotify; consumes `generic_secret.inline_string` ONLY (the inner `secret_file` arm PARSE-REJECTs per §8 item 14); ~100ms debounce; atomic-swap discipline via `atomic.Pointer[[]byte]`. PARSE-REJECT for non-filesystem `core.ConfigSource` oneof arms (`ApiConfigSource` + `Ads`) + the deprecated `ConfigSource.path` field 1 per §20.P6 RATIFIED. ADR-0178 codifies.

4. **NEW filter-local AES-256-CBC token-encryption helper** at `oauth2/tokens.go` per AMEND-1 (per Q4 default-on, algorithm REVISED from BRAINSTORM-anticipated GCM to upstream-byte-exact CBC per §20.P5 REFUTED). ~150-200 LoC. `encryptToken` + `decryptToken` filter-local functions; `SHA-256(hmacSecret)[:32]` KDF → 32-byte AES-256 key; random 16-byte IV per encryption (prepended); PKCS#7 padding; Base64URL envelope. `disable_token_encryption=true` skip-path (plaintext storage; explicit MVP-CONSUMED per §2). **Decryption-failure fall-back per AMEND-3** (returns ciphertext-as-plaintext; downstream HMAC validation rejects naturally; NO `cookie_decrypt_failure` counter per §20.P11 RATIFIED-AS-ABSENT). ADR-0182 codifies.

5. **TWO IN-PLACE §Decision AMENDMENTs** to prior ADRs per ADR-0044: **ADR-0150** (`internal/jwks/Fetcher` refactored to consume `*httpclient.Client` constructor argument; ~40-60 LoC delta in `internal/jwks/`) + **ADR-0159** (`extauthz/check.go::httpAuthClient` refactored to consume `*httpclient.Client`; ~50-80 LoC delta in `internal/filter/http/extauthz/`; **§Future Work CLOSED-AT-PHASE-20** paragraph appended). Both AMENDMENT bodies land at IMPL Task 2 alongside the ADR-0177 introduction; the AMENDMENT-anticipation paragraphs anchor at this SPEC commit.

After phase 20, the project has the foundational OAuth 2.0 filter: a decoder-only filter that classifies inbound requests into 4 dispositions (signout / callback / pass_through / cookie-validate), emits 302 challenges to the configured authorization_endpoint with state-cookie HMAC protection on unauthenticated requests, exchanges authorization codes for tokens at the token_endpoint via async-resumed POST (4-field auth-code template byte-exact per AMEND-5), encrypts the access/refresh tokens via AES-256-CBC (per AMEND-1) and emits a 5-cookie envelope (per ADR-0181), validates cookie envelopes via 5-input HMAC-SHA256 with dual-encoding read (per AMEND-2 + S4), silently rotates expired bearer tokens via the refresh_token grant (per ADR-0183), clears the envelope on sign-out (per ADR-0184), and enforces listener-scoped-only placement via HCM-parse-time PARSE-REJECT (per §5.2). The **deny-path emits only 302 or 401** (per AMEND-3 — no 500 anywhere); 401 body is constant `"OAuth flow failed."` (18 bytes per §20.P9). Observable-outcomes byte-equivalent to reference Envoy v1.37.2 oauth2 on every axis except the documented envoy-go-strict departures (token_endpoint non-2xx retry-eligible → 302 challenge simplification per §4.7; POST callback method PARSE-REJECT per §20.P3).

### 1.1 Empirical-finding-driven scope (amendment block per ADR-0044)

The 12 §11 empirical pins (executed at this SPEC session via parallel-subagent fan-out against v1.37.2 reference Envoy) generated the following 7 amendment-block entries — load-bearing record of empirical-scrape-driven design revisions to the BRAINSTORM:

- **AMEND-1 (Q4 → AES-256-CBC):** algorithm swap from BRAINSTORM-anticipated AES-GCM to upstream-byte-exact **AES-256-CBC** per §20.P5 REFUTED. SHA-256(hmac_secret)[:32] key; random 16-byte IV prepended; PKCS#7 padding; Base64URL(IV ‖ CT) envelope. ADR-0182 anchors.
- **AMEND-2 (Q9 → 5-input HMAC):** composition is `HMAC-SHA256(hmac_secret, StrJoin({domain, expires, token, id_token, refresh_token}, "\n"))` per §20.P4 REFUTED; id_token + refresh_token participate as empty strings when absent; dual-encoding read per S4 (emit Base64, accept BOTH Base64 + HexBase64). ADR-0179 anchors.
- **AMEND-3 (deny-path simplification):** 302 + 401 only — **NO 500 anywhere** per §20.P9 REFUTED; constant 401 body `"OAuth flow failed."` (18 bytes per §20.P9); `addFlowCookieDeletionHeaders(headers, flow_id_)` runs on 401-when-`flow_id_`-set (flow-cookie cleanup is part of deny-path); token_endpoint non-2xx retry-eligible → 302 challenge (envoy-go-strict simplification from upstream `redirectToOAuthServer-retry`); decryption-failure fall-back returns ciphertext-as-plaintext (downstream HMAC validation rejects naturally; no `cookie_decrypt_failure` counter per §20.P11). ADR-0180 anchors.
- **AMEND-4 (stat surface):** 86 → 92 names; **6 counters wire-exact upstream** per §20.P8 REFUTED (BRAINSTORM over-counted at 94); CLOSES §20.P11 envoy-go-strict departure flag as RATIFIED-AS-ABSENT (no `signout_completed`; no `cookie_decrypt_failure`). 6 counters: `oauth_unauthorized_rq` / `oauth_failure` / `oauth_passthrough` / `oauth_success` / `oauth_refreshtoken_success` / `oauth_refreshtoken_failure`. HCM-rooted SN2-reuse per ADR-0143. ADR-0181 anchors.
- **AMEND-5 (token_endpoint POST templates):** 4-field auth-code template for MVP (PKCE-gated per S3); 3-field refresh-token template; spaces as `%20`; PercentEncoding charset includes `:/=&?` per §20.P10 RATIFIED. NEW `urlEncode` custom helper at `oauth2/oauth_client.go` (stdlib `url.PathEscape` does NOT match upstream byte-exact behavior). ADR-0185 anchors.
- **AMEND-6 (BRAINSTORM field-locus corrections):** C1 `cookie_domain` is on `OAuth2Credentials` field 5 (NOT on `OAuth2Config` as BRAINSTORM §1.1 stated); C2 `cookie_configs` is `*CookieConfigs` wrapper message (NOT a raw `map[string]CookieConfig` as BRAINSTORM §1.1 stated); C3 `forward_bearer_token` (OAuth2Config field 7) was not enumerated at BRAINSTORM §1.1 — MVP disposition: CONSUME (default-false per proto; ~10 LoC).
- **AMEND-7 (per-route + new deferral):** HCM-parse-time PARSE-REJECT for route-level placement (consistent with other listener-scoped filters per §5.2); NEW deferred surface — `Partitioned` cookie attribute (CHIPS-style; depends on `cookie_configs`; deferred per §8 item 15).

---

## 2. Non-purposes

Phase 20 is single-row per ADR-0045 (no sub-phases). It does NOT extend the framework, the listener stack, or any other subsystem beyond the minimum needed to land oauth2 under the existing 07.1 framework + the 5 new ADR-anchored primitives.

- **2.1 PKCE envelope OUT OF SCOPE.** `OAuth2Config.use_pkce` + `CookieNames.oauth_nonce` + `CookieNames.code_verifier` + `OAuth2Config.code_verifier_token_expires_in` DEFERRED per Q1; MVP emits the 4-field auth-code template per AMEND-5 (the 5th `code_verifier` field is gated per S3 and stays absent until PKCE lands). The 2-of-7 `CookieNames.oauth_nonce` + `CookieNames.code_verifier` are parsed but neither cookie is emitted nor honored at MVP.
- **2.2 id_token + JWKS-validation OUT OF SCOPE.** `OAuth2Config.id_token` envelope + the authorization-server JWKS round-trip DEFERRED per Q1. Framework REUSES: ADR-0150 jwks NOT consumed for id_token validation (the post-ADR-0150-AMENDMENT refactor refactors the EXISTING jwt_authn consumer; phase 20 does not add a NEW jwks consumer); ADR-0151 jwt verifier NOT consumed. The 6th-of-7 `CookieNames.id_token` is parsed but the cookie is neither emitted nor honored at MVP.
- **2.3 BASIC_AUTH (`OAuth2Credentials.basic_auth`) OUT OF SCOPE.** Explicit per Q1 + Q10 + AMEND-5 — MVP uses `client_secret_post` (the 4-field auth-code template embeds `client_secret` in the body). The proto's BASIC_AUTH arm PARSE-REJECTs at parse-time.
- **2.4 `end_session_endpoint` OUT OF SCOPE.** DEFERRED per Q1; couples to id_token (item 2.2). RP-initiated logout per OIDC; resurfaces when the id_token-enabling phase lands.
- **2.5 `cookie_configs` per-cookie attribute customization OUT OF SCOPE.** DEFERRED per Q1 + AMEND-6 C2 (`cookie_configs` is `*CookieConfigs` wrapper message); MVP uses listener-default Set-Cookie attributes (Secure / HttpOnly / SameSite=Lax / Path=/ per §4.5 RATIFIED-PENDING-IMPL-TIME). The `Partitioned` attribute deferred per AMEND-7 + §8 item 15.
- **2.6 `disable_id_token_set_cookie` + `disable_access_token_set_cookie` + `disable_refresh_token_set_cookie` OUT OF SCOPE.** All 3 DEFERRED per Q1. MVP always emits BearerToken + RefreshToken cookies (the latter when `use_refresh_token=true`); IdToken cookie never emitted (couples to id_token deferral at 2.2).
- **2.7 `csrf_token_expires_in` explicit field-consumption OUT OF SCOPE.** DEFERRED per §20.P12 RATIFIED. MVP uses proto-default 600s (10 minutes) via proto-default fall-through (the field is parsed but its value is ignored when zero; we never override).
- **2.8 `code_verifier_token_expires_in` OUT OF SCOPE.** DEFERRED; paired with PKCE (item 2.1).
- **2.9 `cookie_domain` OUT OF SCOPE.** DEFERRED per §20.P2 RATIFIED + AMEND-6 C1 (field is on `OAuth2Credentials` field 5, NOT `OAuth2Config` as BRAINSTORM stated). MVP emits host-only cookies (no `Domain=` attribute).
- **2.10 `OAuth2Config.retry_policy` OUT OF SCOPE.** DEFERRED per §20.P1 RATIFIED. MVP `internal/httpclient/` applies zero-retry default (matches upstream wire behavior); Options struct leaves `RetryPolicy` field present-but-unused so a future operator-ergonomics phase wires it without a Client signature break.
- **2.11 SDS non-filesystem ConfigSource variants OUT OF SCOPE + PARSE-REJECT.** `core.ConfigSource.ApiConfigSource` + `core.ConfigSource.Ads` oneof arms PARSE-REJECT per §3.2 + §20.P6. The deprecated `core.ConfigSource.path` field 1 PARSE-REJECT (envoy-go-strict). The `generic_secret.secret_file` arm PARSE-REJECT (double-indirection; framework watches the outer Secret-proto JSON/YAML file via fsnotify — the inner indirect-loading is not modeled at MVP).
- **2.12 NEVER-DEFERRED — Per-route override.** The v1.37.x oauth2 proto has NO `OAuth2PerRoute` message at all per §20.P7 RATIFIED. Listener-scoped only; HCM-parse-time PARSE-REJECT for route-level placement per §5.2. THIRD CONSECUTIVE §9 row to make this REUSE-by-absence decision (after phase 18 + phase 19). Permanent absence.
- **2.13 NEVER-DEFERRED — Runtime feature gate.** envoy-go has no runtime-features layer per S2 settled. Upstream's `envoy.reloadable_features.oauth2_encrypt_tokens` reloadable-features gate NOT modeled. MVP relies on `disable_token_encryption` proto-field default (false) as the sole switch.
- **2.14 NEVER-DEFERRED — POST callback method.** GET-only at MVP per §20.P3 PARTIAL → SPEC-decided. POST callbacks (the `response_mode=form_post` OAuth-extension variant) PARSE-REJECT at the callback-flow dispatch in `DecodeHeaders`. envoy-go-strict departure recorded at §13 + ADR-0180.
- **2.15 MVP confirmations (positive consumption assertions).** `disable_token_encryption=true` skip-path IN MVP per §3.3 + ADR-0182. `forward_bearer_token` IN MVP per AMEND-6 C3 (~10 LoC). `preserve_authorization_header` IN MVP. `pass_through_matcher` IN MVP. `deny_redirect_matcher` IN MVP (integrates with sign-out flow per §4.4 + ADR-0184). `use_refresh_token` IN MVP (gates refresh-token rotation per ADR-0183).
- **2.16 Framework REUSES NOT consumed.** ADR-0144 `DownstreamPrincipal()` NOT consumed (no TLS-principal interaction at MVP — oauth2 is browser-redirect-based, not TLS-client-cert-based). ADR-0150 jwks NOT consumed for id_token validation (id_token deferred). ADR-0151 jwt verifier NOT consumed. ADR-0125 5th canonical NOT consumed (REUSE-by-absence per §5).

---

## 3. Framework survey result (TWO NEW top-level primitives + ONE NEW filter-local helper + TWO IN-PLACE §Decision AMENDMENTs + multiple REUSES)

The framework survey evaluated REUSE of phase-04-through-19.2 primitives BEFORE proposing NEW (per the phase-16/17/18.x/19.x discipline). Findings:

### 3.1 NEW: `internal/httpclient/` framework primitive *(ADR-0177)*

NEW top-level `internal/httpclient/` package; ~150-250 LoC primitive. Public surface (settled at SPEC; the IMPL confirms the exact signature):

```go
// Options carries per-Client configuration. Zero-value Options is a no-op
// (zero timeout = no deadline; zero RetryPolicy = no retries; nil TLSConfig).
type Options struct {
    Timeout     time.Duration
    RetryPolicy RetryPolicy // zero = no retries (matches Envoy v1.37.2 wire default)
    TLSConfig   *tls.Config
}

// RetryPolicy carries the optional retry envelope. Zero-value RetryPolicy is no retries.
type RetryPolicy struct {
    Attempts        int           // 0 = no retries
    PerAttemptDelay time.Duration
    RetryOnStatus   []int         // e.g. [500, 502, 503, 504]
}

// Client wraps *http.Client with the Options envelope.
type Client struct { /* http.Client + Options */ }

// New constructs a *Client from Options.
func New(opts Options) *Client

// Do executes the request synchronously; honors ctx cancellation; applies retries per RetryPolicy.
func (c *Client) Do(req *http.Request) (*http.Response, error)
```

**3 consumers at introduction time** (the third-consumer trigger condition that fired the extraction per Q2 + ADR-0159 §Future Work):
1. **jwks Fetcher refactor** (post-ADR-0150 in-place AMENDMENT per §3.4) — REPLACES the inline `http.Client` ownership at `internal/jwks/fetcher.go` with a `*httpclient.Client` constructor argument
2. **extauthz httpAuthClient refactor** (post-ADR-0159 in-place AMENDMENT per §3.5) — REPLACES the inline `http.Client` ownership at `internal/filter/http/extauthz/check.go` with a `*httpclient.Client` constructor argument; **CLOSES the ADR-0159 §Future Work forward-pointer**
3. **oauth2 token_endpoint POST** (NEW at phase 20) — consumes the new primitive at `internal/filter/http/oauth2/oauth_client.go::postTokenEndpoint`

§Future Work plants a NEW forward-pointer for the next-cross-consumer event (anticipated future ext_authz mTLS, future jwt_authn alternative-issuer fetch, future ratelimit gRPC TLS — each at its own future trigger condition).

### 3.2 NEW: `internal/sdsfile/` framework primitive *(ADR-0178)*

NEW top-level `internal/sdsfile/` package; ~160-200 LoC primitive + NEW go.mod dependency on `github.com/fsnotify/fsnotify`. Public surface (settled at SPEC; the IMPL confirms the exact signature):

```go
// Watcher tracks an outer SDS Secret-proto JSON/YAML file (NOT the inner secret_file arm)
// via fsnotify. Concurrent reads safe via atomic.Pointer swap.
type Watcher struct { /* atomic.Pointer[[]byte] + *fsnotify.Watcher + debounce timer */ }

// New constructs a Watcher from a filesystem path pointing at the outer Secret-proto file
// (containing generic_secret.inline_string ONLY; secret_file indirection PARSE-REJECTs).
func New(path string) (*Watcher, error)

// Start begins watching; runs the fsnotify event loop in a goroutine.
func (w *Watcher) Start() error

// Current returns the current in-memory copy of the inline_string bytes (atomic load).
func (w *Watcher) Current() []byte

// Close stops the goroutine and releases the fsnotify watcher.
func (w *Watcher) Close() error
```

Honors `core.ConfigSource.PathConfigSource` (oneof arm field 8) per §20.P6 RATIFIED — the field-8 wrapper (non-deprecated; wraps `{path, watched_directory}`). The deprecated `core.ConfigSource.path` field 1 PARSE-REJECTs at compile time. The `generic_secret.secret_file` inner-indirect-arm PARSE-REJECTs per §8 item 14. **~100ms debounce** captures both atomic-rename-via-mv + in-place-write-via-truncate-and-rewrite event sequences without false-positive reloads (settled at §12 item B7). **Atomic-swap discipline** via `atomic.Pointer[[]byte]` ensures concurrent readers see consistent bytes (settled at §12 item B7). **MVP consumer: oauth2** (hmac_secret + client_secret + AES-key-derivation-source). Cross-phase-reusable for any future filesystem-SDS consumer (future jwt_authn TLS-trust-store reload, future ext_authz mTLS, future ratelimit gRPC TLS).

### 3.3 NEW: filter-local AES-256-CBC token-encryption helper *(ADR-0182)*

NEW filter-local helper at `internal/filter/http/oauth2/tokens.go`; ~150-200 LoC; stdlib `crypto/aes` + `crypto/cipher` + `crypto/sha256` + `encoding/base64`. Filter-local only — NOT extracted to a shared package at phase 20 (second-consumer-trigger deferral; no other in-tree filter needs AES-CBC at MVP). Per AMEND-1 (algorithm REVISED from BRAINSTORM Q4 AES-GCM to upstream-byte-exact CBC per §20.P5 REFUTED):

```go
// encryptToken encrypts plaintext under AES-256-CBC with a random 16-byte IV.
// Returns Base64URL(IV ‖ CT) per upstream wire shape.
func encryptToken(plaintext, hmacSecret []byte) string

// decryptToken decrypts the Base64URL(IV ‖ CT) envelope back to plaintext.
// On failure (malformed envelope, bad padding, etc.): returns the original ciphertext
// bytes (NOT an error) per AMEND-3 fall-back semantics. The downstream HMAC validation
// then rejects the cookie naturally. NO cookie_decrypt_failure counter per AMEND-4 + §20.P11.
func decryptToken(envelope string, hmacSecret []byte) []byte
```

Key derivation: `SHA-256(hmacSecret)[:32]` → 32-byte AES-256 key. IV: random 16 bytes per encryption (prepended to ciphertext). PKCS#7 padding. Wire envelope: `Base64URL(IV ‖ CT)`. **`disable_token_encryption=true` skip-path** (plaintext cookie values stored + returned-as-is per MVP-CONSUMED per §2.15 + S2 NO-runtime-gate decision).

### 3.4 IN-PLACE: ADR-0150 §Decision AMENDMENT *(per ADR-0044)*

The ADR-0150 `internal/jwks/Fetcher` framework primitive (phase 17) is refactored in-place to consume `*httpclient.Client` (per Q2 EXTRACT NOW + ADR-0177 introduction). §Decision body gains an AMENDMENT paragraph documenting the refactor (the `Fetcher` no longer owns its own `http.Client`; it takes a `*httpclient.Client` constructor argument). §Consequences body gains a paragraph documenting the cross-phase-consumer disposition (phase 17 jwks + phase 18.1 ext_authz at ADR-0159 AMENDMENT + phase 20 oauth2 token_endpoint POST — all consuming the new primitive). The third-consumer-trigger-paragraph CLOSES the **implicit forward-pointer to a future httpclient primitive** per §9 item 2. **~40-60 LoC delta** in `internal/jwks/`. Per ADR-0044 in-place edit discipline — NOT a new ADR; ADR-0150 evolves in-place with the AMENDMENT paragraph clearly dated 2026-05-17 and cross-referenced to phase 20 + ADR-0177. **AMENDMENT-anticipation paragraph anchors at this SPEC commit**; AMENDMENT body lands at IMPL Task 2.

### 3.5 IN-PLACE: ADR-0159 §Decision AMENDMENT *(per ADR-0044)*

The ADR-0159 `extauthz/check.go::httpAuthClient` framework primitive (phase 18.1) is refactored in-place to consume `*httpclient.Client` (per Q2 EXTRACT NOW + ADR-0177 introduction). §Decision body gains an AMENDMENT paragraph documenting the refactor (the `httpAuthClient` no longer owns its own `http.Client`; takes a `*httpclient.Client` constructor argument). §Future Work section gains a **CLOSED-AT-PHASE-20** closure paragraph documenting that the "third outbound-HTTP consumer triggers `internal/httpclient/` extraction" forward-pointer is **CLOSED at phase 20** per Q2 EXTRACT NOW + ADR-0177. **~50-80 LoC delta** in `internal/filter/http/extauthz/`. Per ADR-0044 in-place edit discipline — NOT a new ADR; ADR-0159 evolves in-place with the AMENDMENT + the §Future Work closure paragraph clearly dated 2026-05-17 and cross-referenced to phase 20 + ADR-0177. **This is the FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer** — the demonstration is recorded as the load-bearing milestone per §9 item 1 + BRAINSTORM §11 Lesson (d). AMENDMENT-anticipation paragraph + §Future Work closure-anticipation paragraph anchor at this SPEC commit; bodies land at IMPL Task 2.

### 3.6 Framework REUSES — 4 reuses + 4 NOT-CONSUMED items

REUSES (load-bearing):
- **Phase-04 HCM `SendLocalReply` + Location header**: REUSED for the 4-category deny-path emission per §4.1 (302 auth-challenge / 302 post-callback-success / 302 sign-out / 401 with constant body)
- **Phase-09 async-resume primitive**: REUSED for the token_endpoint POST during callback-flow handling (parks decode goroutine on resume channel per §6.8; mirrors phase-18.x + phase-19.x async-resume leg)
- **Cluster-manager for HTTP/1.1 + HTTP/2 outbound**: REUSED for the token_endpoint POST upstream (regular HTTP cluster, not gRPC)
- **ADR-0143 SN2-reuse for stat surface**: REUSED for the 6 oauth2 counters under `http.<HCM_stat_prefix>.oauth2.*` (HCM-rooted; no new SN-flattening rule; mirrors phase-16/17/18/19 stat-surface convention)

NOT-CONSUMED (documented for cross-phase audit clarity):
- **Phase-16 ADR-0144 `DownstreamPrincipal()` TLS-principal accessor**: NOT CONSUMED — oauth2 has no TLS-principal interaction at MVP
- **Phase-17 ADR-0150 `internal/jwks/Fetcher`**: NOT CONSUMED for id_token validation (id_token deferred per §2.2) — but REFACTORED in-place per §3.4 above (the refactor is a delta, not a consumption)
- **Phase-17 ADR-0151 `internal/jwt/` verifier**: NOT CONSUMED — id_token validation deferred per §2.2
- **Phase-18.2 ADR-0165 6 new `DecoderFilterCallbacks` methods**: NOT REUSED — oauth2 has no TLS/principal-attribute envelope to populate

### 3.7 Boot-registration — alphabetical insertion between `localratelimit` and `rbac`

`cmd/envoy-go/main.go` (currently registering 14 entries after phase 19.2 at lines 122-135) gains a fifteenth `httpReg.Register(oauth2.TypeURL, oauth2.New)` call. Insertion alphabetical per ADR-0100 §2.2 convention: `oauth2` inserts at line 135 (between `localratelimit` at line 134 and `rbac` which shifts to line 136). Per ADR-0072, registration order does NOT affect runtime behavior; stylistic discipline only. **NO ADR** anchored for the boot-registration insertion — straight alphabetical per the phase-09..19.2 convention. **NO filter-chain ordering surgery.**

### 3.8 Total framework footprint table

| Surface | Items | Anticipated LoC |
|---|---|---|
| NEW `internal/httpclient/` package | Options + RetryPolicy + Client + Do | ~150-250 LoC |
| NEW `internal/sdsfile/` package | Watcher + New + Start + Current + Close + fsnotify integration | ~160-200 LoC |
| NEW go.mod dep | github.com/fsnotify/fsnotify | — |
| IN-PLACE ADR-0150 refactor | jwks Fetcher consumes *httpclient.Client | ~40-60 LoC delta |
| IN-PLACE ADR-0159 refactor | extauthz httpAuthClient consumes *httpclient.Client | ~50-80 LoC delta |
| NEW filter-local AES-256-CBC helper | tokens.go (encryptToken + decryptToken + KDF) | ~150-200 LoC |
| **Subtotal framework** | | **~550-790 LoC** |
| oauth2 filter proper | 16 Go files (10 prod + 6 test) per §6.11 | ~2850-3110 LoC |
| **GRAND TOTAL phase 20** | | **~3400-3900 LoC** |

The LoC envelope tightens slightly from BRAINSTORM's ~3500-4000 estimate due to the S1/S2/S5 SPEC-time simplifications (S1 single-algorithm AES-256-CBC; S2 no runtime gate; S5 6-counter wire-exact instead of 8-counter). Below the ADR-0045 split threshold per the phase-17 jwt_authn at 3855 single-row precedent. **Single-row landing settled** per the ADR-0045 split disposition.

---

## 4. Deny-path wire shape (302+401 only; NO 500 per AMEND-3)

### 4.1 4 emission categories

| Category | Status | Body | Trigger | Counter |
|---|---|---|---|---|
| (a) 302 auth-challenge | 302 | (empty) | unauthenticated request (sign-in flow default) OR refresh-failure OR token_endpoint non-2xx retry-eligible | (none direct; oauth_refreshtoken_failure on refresh-failure leg) |
| (b) 302 post-callback-success | 302 | (empty) | callback flow successful token_endpoint POST | oauth_success |
| (c) 302 sign-out | 302 | (empty) | request matches `signout_path` | (no separate counter per AMEND-4 + S5) |
| (d) 401 with constant body | 401 | `"OAuth flow failed."` (18 bytes; no trailing newline) | bad state cookie OR token_endpoint non-2xx terminal | oauth_unauthorized_rq (bad state) OR oauth_failure (token_endpoint terminal) |

**NO 500 emissions anywhere** in phase 20 per AMEND-3 + §20.P9. Verified at §15 item 11.

### 4.2 Per-trigger emission table

| Code-site | Disposition | Wire emission | Counter |
|---|---|---|---|
| `decode_headers.go::handleUnauthenticated` | unauthenticated request → 302 challenge | (a) 302 + Location: <authorization_endpoint URL> + state-cookie Set-Cookie | (none direct) |
| `callback.go::applyTokenEndpointResponse` success path | 2xx → 302 post-callback-success | (b) 302 + Location: <redirect_uri> + 5-cookie envelope Set-Cookie | oauth_success |
| `callback.go::applyTokenEndpointResponse` non-2xx retry-eligible | 5xx → 302 challenge re-authenticate | (a) 302 + Location: <authorization_endpoint URL> + state-cookie Set-Cookie | (none direct on this leg) |
| `callback.go::applyTokenEndpointResponse` non-2xx terminal | 4xx → 401 | (d) 401 + body + flow-cookie deletion | oauth_failure |
| `callback.go::handleBadState` | mismatched state cookie | (d) 401 + body + flow-cookie deletion | oauth_unauthorized_rq |
| `signout.go::handleSignout` | signout_path hit | (c) 302 + Location: <signout-target> + Max-Age=0 for all 5 cookies | (no separate counter) |
| `decode_headers.go::handleRefreshFailure` | refresh-token POST non-2xx | (a) 302 challenge | oauth_refreshtoken_failure |
| `decode_headers.go::handlePassThrough` | pass_through_matcher hit | (no oauth2 emission; bypass) | oauth_passthrough |
| `decode_headers.go::handleValidCookies` | valid envelope → upstream proxy | (no oauth2 emission; ContinueDecoding) | (none direct; no counter) |

### 4.3 401 body byte-exact

Constant body `"OAuth flow failed."` (18 bytes ASCII; no trailing newline; sourced from upstream `UnauthorizedBodyMessage` constant). Content-Type from HCM `SendLocalReply` default for non-grpc downstream (RATIFIED-PENDING-IMPL-TIME per §12 item A1 + A4; most-likely `text/plain`). Flow-cookie deletion via `addFlowCookieDeletionHeaders(headers, flow_id_)` per AMEND-3 (cleanup is part of deny-path).

### 4.4 302 Location construction (per category)

- **Category (a) authorization_endpoint URL**: composed per RFC 6749 §4.1.1 — base `authorization_endpoint` + query params `response_type=code` + `client_id=<client_id>` + `redirect_uri=<redirect_uri>` + `state=<HMAC-protected state cookie value>` + `scope=<space-separated auth_scopes>` + `resource=<repeating per RFC 8707 resources>`. State-cookie payload byte-exact shape RATIFIED-PENDING-IMPL-TIME per §12 item A3.
- **Category (b) post-callback Location**: the operator-configured `redirect_uri` value verbatim (the registered redirect that downstream resolves to the post-sign-in landing page).
- **Category (c) sign-out Location**: per the `deny_redirect_matcher` configuration (when sign-out matches a deny-redirect; otherwise empty for browser-default).

### 4.5 Set-Cookie envelope discipline per category

MVP-default Set-Cookie attributes RATIFIED-PENDING-IMPL-TIME per §12 item A2: `Secure; HttpOnly; SameSite=Lax; Path=/`. Cookies emitted per category:

| Category | BearerToken | OauthHMAC | OauthExpires | IdToken | RefreshToken | state cookie |
|---|---|---|---|---|---|---|
| (a) auth-challenge | (clear) | (clear) | (clear) | (n/a) | (clear) | SET to HMAC(state) |
| (b) post-callback | SET to encrypted access_token | SET to HMAC | SET to expires-epoch | (n/a) | SET to encrypted refresh_token | (clear) |
| (c) sign-out | Max-Age=0 | Max-Age=0 | Max-Age=0 | Max-Age=0 | Max-Age=0 | Max-Age=0 |
| (d) 401 | (clear via addFlowCookieDeletionHeaders) | (clear) | (clear) | (n/a) | (clear) | (clear) |

### 4.6 Counter increment matrix

Per AMEND-4 + S5 (6-counter wire-exact upstream):

- `oauth_unauthorized_rq` += 1 per category-(d) bad-state-401 emission
- `oauth_failure` += 1 per category-(d) token_endpoint terminal-401 emission (NOT also on refresh-failure path; refresh-failure → 302 challenge per §4.7 + ADR-0183)
- `oauth_passthrough` += 1 per pass_through_matcher hit
- `oauth_success` += 1 per category-(b) successful sign-in completion
- `oauth_refreshtoken_success` += 1 per successful silent refresh-token rotation
- `oauth_refreshtoken_failure` += 1 per failed silent refresh-token rotation (the request then falls through to (a) 302 challenge)

### 4.7 Phase-09 async-resume integration on token_endpoint POST

The callback-flow leg parks the decode goroutine on the phase-09 async-resume primitive while the token_endpoint POST is in flight. Per AMEND-3 + §20.P9 deny-path simplification (envoy-go-strict): on non-2xx response, the dispatcher classifies the failure as **retry-eligible (5xx)** → emit (a) 302 challenge (re-authenticate from scratch) OR **terminal (4xx)** → emit (d) 401 with constant body. **NO per-call timeout at MVP** (the `RetryPolicy.Attempts=0` default + the cluster-manager's connection-level deadlines bound the wait; the operator-explicit `retry_policy` field is DEFERRED per §2.10).

---

## 5. Per-route discipline — REUSE-by-absence (NO new canonical; NO ADR-0125 amendment)

### 5.1 Proto-absence framing

The v1.37.x oauth2 proto has NO `OAuth2PerRoute` message at all per §20.P7 RATIFIED — strongest-form evidence (the proto file has no per-route-override message arm). Listener-scoped only.

### 5.2 HCM-parse-time PARSE-REJECT

Per ADR-0110 single-chokepoint discipline + the existing HCM TPFC-placement validation gate: any oauth2 `typed_per_filter_config` (TPFC) entry at route or virtualHost level PARSE-REJECTs at HCM-parse-time with byte-stable error message (consistent with the other listener-scoped filter PARSE-REJECT messages). The `RegisterPerRouteValidator` factory method (per §6.1) is the registration hook.

### 5.3 All 6 counters HCM-rooted

No per-route qualifier on any oauth2 counter per AMEND-4 + S5. Stat-name shape: `http.<HCM_stat_prefix>.oauth2.<counter>`.

### 5.4 NO ADR-0125 amendment — THIRD CONSECUTIVE §9 row to skip

After phase 18 (ADR-0163 — REUSED 5th canonical; no amendment) + phase 19 (ADR-0173 — REUSED 5th canonical; no amendment), phase 20 is the THIRD CONSECUTIVE §9 family-row to NOT extend the ADR-0125 roster. Phase 20's REUSE-by-absence is a **STRONGER form of the lesson** than the prior two phases' 5th-canonical REUSE — there is no per-route surface at all, so the listener-scoped-only enforcement is itself a parse-time PARSE-REJECT discipline rather than a roster-REUSE classification. The ADR-0125 roster does NOT grow monotonically; phase 20 strengthens the lesson WITHOUT amendment (the absence itself is the lesson). ADR-0180 records the explicit no-amendment classification.

### 5.5 Forward-pointer for hypothetical future-Envoy `OAuth2PerRoute` — NOT recorded

If a hypothetical future Envoy v1.X+ adds an `OAuth2PerRoute` message, the per-route landing mechanism is already established via ADR-0044 (ADR-on-impl convention) + ADR-0125 (canonical roster). No SPEC-time speculation paragraph needed; the ADR-0044 + ADR-0125 envelope handles it.

---

## 6. compiledConfig + code shapes (IMPL blueprint)

### 6.1 Public surface

The `oauth2` package exposes:
- `TypeURL` — constant `"type.googleapis.com/envoy.extensions.filters.http.oauth2.v3.OAuth2"`
- `New(message proto.Message) (api.HTTPFilterFactory, error)` — the factory; produces a `*filter` per stream via `buildCompiledConfig` shared across streams
- `RegisterPerRouteValidator(reg api.PerRouteValidatorRegistry)` — the HCM-parse-time PARSE-REJECT hook per §5.2

### 6.2 `compiledConfig` struct

```go
type compiledConfig struct {
    // Endpoints
    tokenEndpoint         *url.URL
    authorizationEndpoint string
    redirectURI           string

    // Matchers
    redirectPathMatcher  PathMatcher
    signoutPath          PathMatcher
    passThroughMatcher   []HeaderMatcher
    denyRedirectMatcher  []HeaderMatcher

    // Credentials (4 of 5; no basic_auth)
    clientID         string
    clientSecretSDS  *sdsfile.Watcher  // ADR-0178 consumer
    hmacSecretSDS    *sdsfile.Watcher  // ADR-0178 consumer
    cookieNames      CookieNames        // 5-of-7 consumed
    cookieDomain     string             // per AMEND-6 C1: on OAuth2Credentials (deferred; empty at MVP)

    // Behavioral knobs
    forwardBearerToken          bool   // per AMEND-6 C3: MVP CONSUMED
    preserveAuthorizationHeader bool
    disableTokenEncryption      bool   // S2: no runtime gate; proto default false
    useRefreshToken             bool
    defaultExpiresIn            time.Duration
    defaultRefreshTokenExpiry   time.Duration
    authScopes                  []string
    resources                   []string

    // Filter-local AES-256-CBC key (atomic.Pointer for sdsfile reload safety)
    aesKey atomic.Pointer[[32]byte]

    // HTTP client (per ADR-0177 consumer)
    httpClient *httpclient.Client

    // Filter-wide stats (6 counters per AMEND-4 + S5)
    stats *filterStats
}
```

Compile-time invariants enforced by `buildCompiledConfig`: token_endpoint URL valid; authorization_endpoint non-empty; redirect_uri non-empty; client_id non-empty; SDS configs reachable (file exists + readable; via `internal/sdsfile/New(path)`); `disable_token_encryption=false` AND `hmac_secret` empty → PARSE-REJECT; PKCE fields set → PARSE-REJECT per §2.1; basic_auth set → PARSE-REJECT per §2.3.

### 6.3 DecodeHeaders dispatch body

Priority order (settled at SPEC; tested at §14.1 dispatcher tests):
```
DecodeHeaders(headers, endStream):
  1. if signoutPath matches: → handleSignout (§6.9) — category (c) 302
  2. if redirectPathMatcher matches: → handleCallback (§6.8) — async token_endpoint POST
  3. if passThroughMatcher matches any: → handlePassThrough — bypass, increment oauth_passthrough
  4. else: → handleCookieValidate (§6.4)
     - if valid envelope: → ContinueDecoding (with optional Authorization header injection)
     - if expired BearerToken + valid RefreshToken: → handleRefresh — async refresh-token POST
     - else: → handleUnauthenticated — category (a) 302 challenge
```

POST callback method PARSE-REJECTs at the callback dispatch (per §2.14 envoy-go-strict departure).

### 6.4 Cookie envelope reader

```
parseAllCookies(headers) → map[string]string (the 5-cookie envelope, BearerToken/OauthHMAC/OauthExpires/IdToken/RefreshToken keys)
hmacValidate(envelope, hmacSecret) bool:
  // Per AMEND-2 + S4:
  computed = HMAC-SHA256(hmacSecret, StrJoin({domain, expires, token, id_token, refresh_token}, "\n"))
  // domain = host from request; expires = OauthExpires; token = BearerToken;
  // id_token = IdToken (empty if absent); refresh_token = RefreshToken (empty if absent)
  // Accept BOTH Base64 + HexBase64 encodings on read (dual-encoding per S4)
  return crypto/hmac.Equal(decodeDualEncoding(envelope.OauthHMAC), computed)

maybeDecrypt(value) plaintext:
  if disable_token_encryption: return value as-is
  return decryptToken(value, hmacSecret) // per ADR-0182 + AMEND-3 fall-through

expiresParse(expiresValue) time.Time:
  // Per §12 item A3 RATIFIED-PENDING-IMPL-TIME: epoch-seconds-as-decimal-string
  return time.Unix(parseInt(expiresValue), 0)
```

### 6.5 Cookie envelope writer

```
computeHMAC(domain, expires, token, idToken, refreshToken, hmacSecret) string:
  return base64.RawURLEncoding.EncodeToString(
    hmacSHA256(hmacSecret, StrJoin({domain, expires, token, idToken, refreshToken}, "\n"))
  )

maybeEncrypt(value) ciphertext:
  if disable_token_encryption: return value as-is
  return encryptToken(value, hmacSecret) // per ADR-0182

formatSetCookie(name, value, attrs) string:
  return name + "=" + value + "; Path=/; Secure; HttpOnly; SameSite=Lax"
```

### 6.6 handleUnauthenticated

Emits category (a) 302 challenge with state-cookie + cleared envelope cookies (per §4.5). Authorization-endpoint URL composed per §4.4. State-cookie payload byte-exact shape RATIFIED-PENDING-IMPL-TIME per §12 item A3.

### 6.7 oauth_client.go

`postTokenEndpoint(req)` via `httpClient.Do(req)` per ADR-0177.

`buildTokenRequestBody(grantType, params)` per AMEND-5:
- auth-code template (MVP, 4-field): `grant_type=authorization_code&code={0}&client_id={1}&client_secret={2}&redirect_uri={3}` — byte-exact per §20.P10 + ADR-0185
- refresh-token template (3-field): `grant_type=refresh_token&refresh_token={0}&client_id={1}&client_secret={2}` — byte-exact per §20.P10
- PKCE-gated 5th field for future: `&code_verifier={4}` (currently absent — gated per S3 + §2.1)

`urlEncode(value)` custom helper — percent-encodes `:/=&?` per AMEND-5 + §20.P10 + §12 item A5. NOT stdlib `url.PathEscape` (different byte-exact behavior).

### 6.8 applyTokenEndpointResponse async-resume continuation

On success (2xx response): parse JSON body → emit (b) 302 post-callback-success with new envelope per §4.1.
On refresh-success (2xx response on refresh leg): CONTINUE with deferred Set-Cookie envelope per ADR-0183.
On failure (non-2xx): retry-eligible (5xx) → emit (a) 302 challenge; terminal (4xx) → emit (d) 401 per §4.7 + AMEND-3.

### 6.9 handleSignout

Emits category (c) 302 with full envelope clearing (Max-Age=0 for all 5 cookies per §4.5). Location per `deny_redirect_matcher` integration per ADR-0184. NO separate `signout_completed` counter per AMEND-4 + S5 (the sign-out-completed event IS the 302 emission).

### 6.10 buildCompiledConfig

Parser + PARSE-REJECT path. Byte-stable error messages per ADR-0080 discipline. PARSE-REJECT cases per §14.1 group 1.

### 6.11 File layout — 16 Go files (10 production + 6 test)

**Production** (~2850-3110 LoC):
- `oauth2.go` — filter type + factory + filterStats + compile-time interface assertions
- `compiled_config.go` — `compiledConfig` + `buildCompiledConfig` + PARSE-REJECT path
- `decode_headers.go` — dispatch + handleUnauthenticated + handlePassThrough + handleValidCookies
- `callback.go` — handleCallback + applyTokenEndpointResponse + handleBadState
- `signout.go` — handleSignout
- `oauth_client.go` — postTokenEndpoint + buildTokenRequestBody + urlEncode
- `cookies.go` — parseAllCookies + formatSetCookie + Set-Cookie attribute discipline
- `hmac.go` — computeHMAC + hmacValidate + dual-encoding read per AMEND-2 + S4
- `tokens.go` — encryptToken + decryptToken + KDF per ADR-0182 + AMEND-1
- `stats.go` — 6-counter filterStats per ADR-0181 + AMEND-4 + SN2 compile-time guards per ADR-0143

**Test** (~1500-2000 LoC):
- `oauth2_test.go` — factory + dispatcher tests
- `cookies_test.go` — round-trip + Set-Cookie attribute tests
- `hmac_test.go` — vector tests per ADR-0179 + dual-encoding read
- `tokens_test.go` — AES-CBC vector tests per ADR-0182
- `oauth_client_test.go` — template byte-exact tests per ADR-0185 + urlEncode vector tests
- `fuzz_test.go` — 26th fuzzer `FuzzOAuth2ConfigParse` per §7.4

### 6.12 Compile-time invariants

- `var _ api.StreamFilter = (*filter)(nil)` interface conformance (DECODER-ONLY per §3.6 + per-phase precedent)
- `var _ api.StreamDecoderFilter = (*filter)(nil)` blank-identifier assertion
- SN2 stat-name compile-time guards per ADR-0143: each of the 6 counter names asserted at compile time via the package's stat-registration constructor pattern
- TypeURL constant assertion (string == expected proto fully-qualified name)

---

## 7. Differential fixture `0024-http-oauth2` + helpers + fuzzer

### 7.1 Per-request matrix: 9 wire-level expectations across 8 scenario directories

| # | Scenario directory | Wire expectation |
|---|---|---|
| a | `sign_in_happy_path/` | unauthenticated GET → 302 challenge → callback → token_endpoint POST (4-field auth-code template byte-exact per AMEND-5) → 302 (b) post-callback-success with 5-cookie envelope (BearerToken AES-CBC encrypted; HMAC validated); `oauth_success=1` |
| b1 | `cookie_passthrough_valid_envelope/` | valid envelope → ContinueDecoding → upstream proxy with `Authorization: Bearer <decrypted>` injection per `forward_bearer_token=true`; no counter delta |
| b2 | `cookie_passthrough_tampered_envelope/` | tampered HMAC → 302 challenge per §4.7 + AMEND-3 decryption-failure fall-back; `oauth_unauthorized_rq=1` |
| c | `pass_through_matcher/` | header match → bypass oauth2; `oauth_passthrough=1` |
| d | `refresh_token_rotation/` | expired BearerToken + valid RefreshToken → silent refresh-token POST → CONTINUE with deferred Set-Cookie; `oauth_refreshtoken_success=1` |
| e | `signout_flow/` | request to `signout_path` → 302 (c) sign-out + Max-Age=0 for all 5 cookies; no separate counter |
| f | `bad_state_401/` | callback with mismatched state cookie → 401 + `"OAuth flow failed."`; `oauth_unauthorized_rq=1`; flow-cookie cleanup |
| g | `token_endpoint_5xx_302/` | token_endpoint POST returns 5xx → 302 challenge per §4.7 envoy-go-strict simplification; `oauth_failure=1` |
| h | `token_endpoint_4xx_401/` | token_endpoint POST returns 4xx terminal → 401 with constant body; `oauth_failure=1` |
| i | `disable_token_encryption_true/` | `l_test_b` listener; cookies stored plaintext; HMAC still validates; verifies skip-path per §3.3; `oauth_success=1` |

### 7.2 Topology

2-or-3 listeners (settled by IMPL planner; the 9 wire expectations span the listener envelope):
- `l_test_a` — default-encryption (`disable_token_encryption=false`)
- `l_test_b` — `disable_token_encryption=true` (verifies the skip-path at scenario i)
- `l_test_c` — `forward_bearer_token=true` (verifies the Authorization-header injection at scenario b1)

Mock authorization-server backend via `test/helpers/oauthbackend/` per §7.3. SDS Secret files live at `test/fixtures/0024-http-oauth2/secrets/hmac.json` + `client_secret.json` as `Secret` proto JSON.

### 7.3 NEW `test/helpers/oauthbackend/` test-helper package

In-process OAuth 2.0 authorization-server backend. ~250-350 LoC. Public surface:
- `Scripted{Authz,Token}Responses` — per-scenario scripted responses keyed by request method + path
- `ValidCookieEnvelope(t, secret)` helper — produces a valid 5-cookie envelope for assertion fixtures
- `TamperedStateCookie(t, secret)` helper — produces a tampered state cookie for scenario b2 + f

Stdlib `net/http/httptest`-based server, configurable per-scenario.

### 7.4 26th fuzzer `FuzzOAuth2ConfigParse`

At `internal/filter/http/oauth2/fuzz_test.go`. Corpus seeds: each `OAuth2Config` field × valid/invalid variants (~17 consumed fields); each `OAuth2Credentials` field × valid/invalid; each `CookieNames` field × valid/invalid; `SdsSecretConfig` path variants; matcher-engine variants. Must-never-panic discipline. Clean at 30s per seed per project fuzzer-time-budget envelope. Total fuzzer count after phase 20: 26 (was 25 post-phase-19.2).

### 7.5 Six-gate checklist (A/B/C/D/E/F) + cross-package regression matrix

| Gate | Command | Pass criterion |
|---|---|---|
| A — build | `go build ./...` | Clean |
| B — vet + lint | `go vet ./...` + `golangci-lint run` | Clean; no new suppressions |
| C — race | `go test -race ./...` | Zero data-race violations (incl. fsnotify reload + refresh-token rotation + atomic.Pointer aesKey swap) |
| D — differential | `make test-differential FIXTURE=0024-http-oauth2` + regression run | Fixture 0024 GREEN at 9 expectations; fixtures 0019 + 0020 GREEN (refactor regression); 22 other fixtures GREEN |
| E — fuzz | `go test -fuzz=FuzzOAuth2ConfigParse -fuzztime=30s` | Clean; no panics |
| F — h2spec | `make test-h2spec` | 53/53 PASS at ADR-0051 pin |

Cross-package regression matrix per §12 item C8: fixture-0019 (jwt_authn) + fixture-0020 (ext_authz HTTP-mode) stay byte-exact GREEN post-ADR-0150 + ADR-0159 in-place AMENDMENTs; fixture-0021 (ext_authz gRPC-mode) untouched.

---

## 8. Deferred items (~17 items; 13 BRAINSTORM-§8 carry-forwards + 4 NEW)

For future-phase consideration (none are blockers for closing row 20 phase-done; all auditable in the ADR-0040 deferral trail).

### A. OAuth2Config field deferrals (12 items — carried from BRAINSTORM §8 with AMEND-corrections)

1. **`OAuth2Credentials.basic_auth` (BASIC_AUTH `client_secret_basic`)** — DEFERRED per Q1 + AMEND-5; MVP uses `client_secret_post`.
2. **`OAuth2Config.retry_policy`** — DEFERRED per §20.P1 RATIFIED; MVP applies zero-retry default.
3. **`OAuth2Config.end_session_endpoint`** — DEFERRED per Q1; couples to id_token (item 4).
4. **id_token consumption + validation** — DEFERRED per Q1; consumes ADR-0150 jwks + ADR-0151 jwt verifier (both NOT-consumed at phase 20).
5. **PKCE envelope** (`use_pkce` + `oauth_nonce` + `code_verifier` + `code_verifier_token_expires_in`) — DEFERRED per Q1 + AMEND-5; MVP emits 4-field auth-code template.
6. **`OAuth2Config.cookie_configs`** (`*CookieConfigs` wrapper per AMEND-6 C2) — DEFERRED per Q1; MVP uses listener-default Set-Cookie attributes.
7. **`OAuth2Config.disable_id_token_set_cookie`** — DEFERRED; couples to id_token.
8. **`OAuth2Config.disable_access_token_set_cookie`** — DEFERRED; MVP always emits BearerToken cookie.
9. **`OAuth2Config.disable_refresh_token_set_cookie`** — DEFERRED; MVP always emits RefreshToken cookie when `use_refresh_token=true`.
10. **`OAuth2Config.csrf_token_expires_in` explicit field-consumption** — DEFERRED per §20.P12 RATIFIED; MVP uses proto-default 600s.
11. **`OAuth2Config.code_verifier_token_expires_in`** — DEFERRED; paired with PKCE (item 5).
12. **`OAuth2Credentials.cookie_domain`** (per AMEND-6 C1) — DEFERRED per §20.P2 RATIFIED; MVP emits host-only cookies.

### B. SDS / proto-shape deferrals (3 items)

13. **SDS non-filesystem ConfigSource variants** (`ApiConfigSource` + `Ads` oneof arms; deprecated `path` field 1) — DEFERRED + PARSE-REJECT per §3.2 + §20.P6.
14. **`generic_secret.secret_file` arm** (filesystem alternative to `inline_string`) — DEFERRED + PARSE-REJECT at MVP.
15. **`Partitioned` cookie attribute** (CHIPS-style; per AMEND-7 — NEW deferred surface NOT anticipated by BRAINSTORM) — DEFERRED; depends on `cookie_configs` (item 6).

### C. Permanent absences (2 items — NOT deferrals)

16. **(NEVER-DEFERRED) Per-route override** — no `OAuth2PerRoute` message in v1.37.x proto per §5.1 + §20.P7. Listener-scoped only.
17. **(NEVER-DEFERRED) Runtime feature gate** — envoy-go has no runtime-features layer per S2. MVP relies on `disable_token_encryption` proto-field default.

### D. Forward-pointer to future-phase consumers

Phase-20 SPEC does NOT introduce NEW forward-pointers in §8 (the discipline is to defer-with-trail-anchor; each item above names the future-consuming phase trigger). The two ADR §Future Work forward-pointers that DO get planted at phase-20 IMPL land in ADR-0177 (cross-phase reuse footprint forward-pointer) + ADR-0178 (cross-phase reuse footprint forward-pointer), per the ADR §Future Work discipline rather than the SPEC §8 deferral discipline.

---

## 9. Cross-references against phase-17 + phase-18 + phase-19 deferred-items lists — closure pickup

### A. LOAD-BEARING closures fired at phase 20 (2 items)

1. **ADR-0159 §Future Work forward-pointer** ("third outbound-HTTP consumer triggers `internal/httpclient/` extraction"): **CLOSED at phase 20** per §3.5 IN-PLACE ADR-0159 §Decision AMENDMENT + §Future Work CLOSURE-AT-PHASE-20 paragraph. **FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer.**
2. **ADR-0150 implicit forward-pointer** (jwks Fetcher cross-phase consumer of future httpclient primitive): **CLOSED at phase 20** per §3.4 IN-PLACE ADR-0150 §Decision AMENDMENT. Minor closure (no load-bearing protocol decision was awaiting it).

### B. OPPORTUNISTIC EXTENSIONS of multi-phase deferred-clusters (3 items)

3. **Dynamic-metadata family** (phases 16+17+18+19): NO PICKUP — oauth2 has no metadata-emit surface. Cluster STAYS at 5 §9 filters blocked.
4. **`response_code_details` joint divergence-window** (phases 16+17+18+19): EXTENDED — oauth2's 401 deny-path candidate for `response_code_details` but upstream Envoy v1.37.2 also does not emit one (no candidate-emission analog upstream); phase 20 ADDS to the joint-closure forward-pointer (now 6 §9 filters).
5. **id_token-and-jwks-and-jwt-verifier NEW deferred-cluster anchored at phase 20** — 1-deep at phase 20; resurfaces at future id_token-enabling phase.

### C. CARRYFORWARD-UNTOUCHED (2 items)

6. **Carryforward M** (`subject_local_certificate` TLS-fixture hypothesis from phase 19.1 REVIEW) — CARRY FORWARD UNTOUCHED. Phase 20 has no TLS-listener-mTLS interaction.
7. **Phase-19.2 forward-pointer notes** (I), (II), (III) — body-stage `body_mutation` 500 + HCM encode-side `SendLocalReply` framework gap + decode-side body-mutation-delivery limitation: CARRY FORWARD UNTOUCHED. All three are ext_proc-body-mode-specific.

### D. Phase-18.1 + phase-19.x other deferrals — explicit non-pickup framing (4 items)

8. **Phase-18 `allowed_client_headers_on_success`**: NOT APPLICABLE (oauth2 is decoder-only).
9. **Phase-18 + phase-19 `core.GrpcService.GoogleGrpc` PARSE-REJECT**: NOT APPLICABLE (oauth2's token_endpoint POST is HTTP, not gRPC).
10. **Phase-18 + phase-19 `core.GrpcService.{initial_metadata, retry_policy}` SILENT-IGNORE**: NOT APPLICABLE (same).
11. **Phase-18.2 ADR-0165 callback-surface extension**: NOT REUSED (oauth2 has no TLS/principal-attribute envelope).

### E. Forward-pointer net change summary

| Disposition | Count |
|---|---|
| LOAD-BEARING CLOSURES at phase 20 | 2 |
| OPPORTUNISTIC EXTENSIONS | 3 |
| CARRYFORWARD-UNTOUCHED | 2 |
| NOT-APPLICABLE explicit no-op | 4 |
| NEW deferred-cluster anchored at phase 20 | 1 (SDS-non-filesystem) |

Phase 20 fires the FIRST §9 family-row to close a prior-phase forward-pointer load-bearing (item 1) — structurally important demonstration of the ADR-0044 §Future-Work forward-pointer-and-close discipline functioning across phase boundaries.

---

## 10. ADR anchor map (9 NEW + 2 IN-PLACE AMENDMENTs)

Per ADR-0044 ADR-on-impl convention: ADR-0177..ADR-0185 §Context drafts anchor at this SPEC commit; §Decision + §Consequences bodies land at each ADR's Lands-in-Task at IMPL. The 2 in-place AMENDMENT-anticipation paragraphs at ADR-0150 + ADR-0159 anchor at this SPEC commit; AMENDMENT bodies land at IMPL.

### A. 9 NEW ADRs (ADR-0177..ADR-0185)

| ADR | Subject | Anchors §§ | Lands-in-Task |
|---|---|---|---|
| **ADR-0177** | NEW `internal/httpclient/` framework primitive (Options + Client.Do); **CLOSES ADR-0159 §Future Work forward-pointer** (third-consumer trigger fired per Q2); cross-phase-reusable | §3.1; §3.4 trigger; §3.5 trigger; §3.8 footprint | Task 2 |
| **ADR-0178** | NEW `internal/sdsfile/` framework primitive (Watcher + fsnotify + atomic-swap); `inline_string` only; PARSE-REJECT non-filesystem ConfigSource arms + deprecated path field + secret_file indirect-arm | §3.2; §3.8; §8 items 13 + 14 | Task 3 |
| **ADR-0179** | oauth2 HMAC cookie composition — 5-input newline-joined per AMEND-2; dual-encoding read per S4 (emit Base64; accept BOTH Base64 + HexBase64); constant-time-compare | §6.4; §6.5; §1.1 AMEND-2 | Task 4 |
| **ADR-0180** | oauth2 state-machine + deny-path wire shape + listener-scoped enforcement; 4 emission categories per §4.1; NO 500 per AMEND-3; HCM-parse-time PARSE-REJECT per §5.2; explicit NO-ADR-0125-AMENDMENT classification per §5.4 | §4; §5; §6.3; §3.6 reuses; §9 items 4 + 5 | Task 5 |
| **ADR-0181** | oauth2 cookie envelope + stat surface — 5-of-7 `CookieNames` MVP; Set-Cookie attribute discipline RATIFIED-PENDING-IMPL-TIME; stat surface 86 → 92 per AMEND-4; 6 counters wire-exact; CLOSES §20.P11 as RATIFIED-AS-ABSENT | §6.4; §6.5; §6.11; §6.12; §1.1 AMEND-4; §11 §20.P11 | Task 6 |
| **ADR-0182** | AES-256-CBC token-encryption scheme per AMEND-1 (algorithm swap from BRAINSTORM-anticipated GCM per §20.P5 REFUTED); SHA-256(hmac_secret)[:32] KDF; random 16-byte IV; PKCS#7; Base64URL envelope; `disable_token_encryption=true` skip-path; AMEND-3 decryption-failure fall-back | §3.3; §1.1 AMEND-1; §11 §20.P5 + §20.P11 | Task 7 |
| **ADR-0183** | refresh-token rotation timing + race-vs-rotation discipline; concurrent-request disposition (no per-stream serialization; latest Set-Cookie wins via deferred Set-Cookie); counter matrix | §6.8; §4.6 | Task 8 |
| **ADR-0184** | sign-out flow — `signout_path` handling + full envelope clearing + `deny_redirect_matcher` integration; category (c) 302 per §4.1; no separate `signout_completed` counter per AMEND-4 + S5 | §6.9; §4.1 category (c); §4.5 | Task 9 |
| **ADR-0185** | token_endpoint POST body templates per AMEND-5 — 4-field auth-code template (MVP) + 3-field refresh-token template; PKCE-gated 5th field for future; `:/=&?` PercentEncoding charset; NEW urlEncode custom helper | §6.7; §1.1 AMEND-5; §11 §20.P10 | Task 10 |

### B. 2 IN-PLACE §Decision AMENDMENTs (per ADR-0044)

| ADR | AMENDMENT scope | Lands-in-Task |
|---|---|---|
| **ADR-0150** | `internal/jwks/Fetcher` refactor — §Decision body gains AMENDMENT paragraph (consumes `*httpclient.Client`); §Consequences body gains cross-phase-consumer disposition paragraph; ~40-60 LoC delta | Task 2 (paired with ADR-0177 introduction) |
| **ADR-0159** | `extauthz/check.go::httpAuthClient` refactor — §Decision body gains AMENDMENT paragraph (consumes `*httpclient.Client`); **§Future Work gains CLOSED-AT-PHASE-20 paragraph** (FIRST §9 family-row to close prior-phase load-bearing forward-pointer); ~50-80 LoC delta | Task 2 (paired with ADR-0177 introduction) |

### C. ADR-0044 escape-valve reserve

~0-2 impl-time-unanticipated ADRs per phase. Phase 20's most-likely surfaces (all SPEC-time CLOSED via §3 + §6 + §14): AES-256-CBC PKCS#7 padding-oracle hardening; fsnotify event-debounce edge-cases; urlEncode charset edge-cases for non-ASCII bytes. The escape-valve reserve is held against ORTHOGONAL surfaces.

### D. Anchor map summary

| Disposition | Count | ADR numbers |
|---|---|---|
| NEW ADR §Context drafts | 9 | ADR-0177..ADR-0185 |
| IN-PLACE §Decision AMENDMENT-anticipation | 2 | ADR-0150; ADR-0159 |
| ADR-0125 amendments | 0 | NONE — REUSE-by-absence per §5.4 |
| ADR-0044 escape-valve reserve | 0-2 | reserved at ADR-0186+ if fired |

**Next-free ADR post-SPEC commit:** ADR-0186.

---

## 11. Empirical-pin block reference (12 pins resolved at this SPEC session)

### A. Pin disposition matrix

| Pin | Disposition | Wire-level finding | ADR anchor |
|---|---|---|---|
| **§20.P1** RetryPolicy | RATIFIED | Upstream Envoy v1.37.2 applies zero-retry default at token_endpoint POST. MVP `internal/httpclient/` mirrors this. | ADR-0177 |
| **§20.P2** Cookie domain | RATIFIED | When `cookie_domain` empty, host-only cookies (no `Domain=` attribute). Field-locus correction at AMEND-6 C1. | ADR-0181 |
| **§20.P3** Callback method | PARTIAL → SPEC-decided | PARSE-REJECT POST callbacks (envoy-go-strict; GET-only matches canonical practice). | ADR-0180 |
| **§20.P4** HMAC composition | REFUTED | 5-input newline-joined `HMAC-SHA256(hmac_secret, StrJoin({domain, expires, token, id_token, refresh_token}, "\n"))`. id_token + refresh_token empty when absent. Dual-encoding read per S4. | ADR-0179; AMEND-2 |
| **§20.P5** Encryption | REFUTED | AES-256-CBC (NOT AES-GCM as BRAINSTORM hypothesized). SHA-256(hmac_secret)[:32] key. Random 16-byte IV. PKCS#7. Base64URL envelope. | ADR-0182; AMEND-1 |
| **§20.P6** SDS filesystem-path | RATIFIED | `ConfigSource.PathConfigSource` (oneof arm field 8). Deprecated `ConfigSource.path` field 1 PARSE-REJECT. | ADR-0178 |
| **§20.P7** Listener-scoped | RATIFIED | No `OAuth2PerRoute` message in v1.37.x proto. HCM-parse-time PARSE-REJECT. | ADR-0180 |
| **§20.P8** Stat roster | REFUTED | 6 counters wire-exact upstream (NOT 8 per BRAINSTORM). `signout_completed` + `cookie_decrypt_failure` ABSENT. | ADR-0181; AMEND-4 |
| **§20.P9** 401/500 body | REFUTED | 401 body constant `"OAuth flow failed."` (18 bytes; no trailing newline). NEVER 500 anywhere. | ADR-0180; AMEND-3 |
| **§20.P10** URL_ENCODED_BODY | RATIFIED | 4-field auth-code template (MVP) + 3-field refresh-token template byte-exact. `:/=&?` percent-encoded. | ADR-0185; AMEND-5 |
| **§20.P11** `cookie_decrypt_failure` | RATIFIED-AS-ABSENT | Upstream decryption-failure fall-back returns ciphertext-as-plaintext; no counter. envoy-go-strict departure flag CLOSED. | ADR-0181; AMEND-3 + AMEND-4 |
| **§20.P12** `csrf_token_expires_in` | RATIFIED | Proto default 600s = 10 minutes. MVP uses default via proto-default fall-through. | §8 item 10 |

### B. Pin disposition summary

| Disposition | Count |
|---|---|
| RATIFIED | 6 |
| REFUTED | 4 |
| PARTIAL → SPEC-decided | 1 |
| RATIFIED-AS-ABSENT | 1 |
| **TOTAL** | **12** |

All 12 pins CLOSED at SPEC time. **No RATIFIED-PENDING-IMPL-TIME pin disposition deferred to IMPL phase** at the pin-disposition level (the §12 residual byte-confirmations are SUB-PIN-LEVEL refinements, not deferred pin closures).

### C. Pin-to-AMEND-block traceability

| AMEND-N | Sources | Recipient ADRs |
|---|---|---|
| AMEND-1 | §20.P5 REFUTED | ADR-0182 |
| AMEND-2 | §20.P4 REFUTED + S4 dual-encoding | ADR-0179 |
| AMEND-3 | §20.P9 REFUTED; §20.P11 RATIFIED-AS-ABSENT | ADR-0180 |
| AMEND-4 | §20.P8 REFUTED; §20.P11 RATIFIED-AS-ABSENT | ADR-0181 |
| AMEND-5 | §20.P10 RATIFIED + S3 PKCE-gating | ADR-0185 |
| AMEND-6 | C1 (§20.P2 field-locus); C2 (wrapper shape); C3 (MVP-consume forward_bearer_token) | ADR-0180 + ADR-0181 |
| AMEND-7 | §20.P7 RATIFIED + NEW `Partitioned` deferral | ADR-0180 + §8 item 15 |

---

## 12. Deferred decisions (the planner / implementer settles these)

8 RATIFIED-PENDING-IMPL-TIME items — sub-pin-level refinements of already-closed pins, settled at IMPL Tasks 2-13 + Task 14 (six-gate verification). None block phase-20 phase-done.

### A. Wire-shape byte-confirmation items (5)

1. **401 Content-Type + no-trailing-newline** — §20.P9. Settles at IMPL Task 13 fixture-0024 scenario (f).
2. **Set-Cookie attribute byte-exact upstream defaults** — §4.5. Settles at IMPL Task 13 fixture-0024 scenario (a). Most-likely: `Path=/; Secure; HttpOnly; SameSite=Lax`.
3. **state-cookie payload byte-exact shape + OauthExpires format** — §4.4. Settles at IMPL Task 13 fixture-0024 scenario (a). Most-likely: epoch-seconds-as-decimal-string for OauthExpires.
4. **HCM `SendLocalReply` Content-Type default for non-grpc downstream** — §4.3. Settles at IMPL Task 13 cross-side Content-Type byte-comparison.
5. **urlEncode charset helper precise behavior for non-ASCII bytes** — §20.P10. Settles at IMPL Task 4 vector-tests + Task 13 fixture-0024 token_endpoint POST byte-comparison.

### B. Library-behavioral confirmation items (2)

6. **AES-256-CBC PKCS#7 padding decrypt-failure semantics** — §20.P5 + AMEND-3. Settles at IMPL Task 7 unit-tests + Task 13 fixture-0024 decrypt-failure path coverage. Most-likely: Go's `crypto/cipher.NewCBCDecrypter` surfaces padding errors as garbage; the fall-back wrap at `tokens.go::decryptToken` returns ciphertext-as-plaintext.
7. **fsnotify event-debounce window precise behavior** — ADR-0178. Settles at IMPL Task 3 unit-tests + race-tested at IMPL Task 8. ~100ms debounce captures both atomic-rename-via-mv + in-place-write-via-truncate-and-rewrite.

### C. Cross-phase regression-window items (1)

8. **Cross-package regression matrix for ADR-0150 + ADR-0159 in-place AMENDMENTs** — §3.4 + §3.5 + §9 items 1 + 2. Settles at IMPL Task 2 + Task 14 six-gate. Expected outcome: zero regression (refactor is pure thin-wrapper-substitution).

---

## 13. BEHAVIOR_CONTRACT.md additions (in-place edit per ADR-0052; lands at phase-20 phase-done)

10-edit bundle landing at the IMPL phase-done commit per ADR-0052. None at this SPEC commit. Edits:

### A. NEW top-level subsection (1)

1. **NEW `### envoy.filters.http.oauth2` subsection** inserted after `### envoy.filters.http.ext_proc`. Subsections: filter scope; populated-vs-deferred field map (~17 OAuth2Config + 4 OAuth2Credentials + 5 CookieNames consumed); sign-in flow wire shape; refresh flow wire shape; sign-out flow wire shape; pass-through wire shape; cookie envelope discipline; token_endpoint POST body template; stat-name mapping; per-route discipline; envoy-go-strict departures (2 per items 7 + 8 below). Anticipated ~250-350 LoC.

### B. Per-section additions in existing subsections (5)

2. **Stat-name mapping 86-name → 92-name table extension** — 6 new oauth2 counter rows. Table caption updated.
3. **NEW `## HTTP outbound framework primitive (per phase 20 ADR-0177)` subsection** after the existing JWKS framework primitive subsection. Documents `internal/httpclient/` + 3 consumers.
4. **NEW `## Filesystem-SDS framework primitive (per phase 20 ADR-0178)` subsection** after item B3. Documents `internal/sdsfile/` + MVP consumer + cross-phase reuse forward-pointer.
5. **CLOSURE-AT-PHASE-20 paragraph appended to `## HTTP outbound auth-check framework note (per phase 18.1 ADR-0159)`** documenting the third-consumer-trigger closure (FIRST §9 family-row to close prior-phase load-bearing forward-pointer).
6. **Per-route canonical patterns cross-reference table update** — caption "updated through phase 19.2" → "updated through phase 20"; phase-20 cross-reference paragraph added documenting the REUSE-by-absence (THIRD CONSECUTIVE §9 row; no roster extension).

### C. NEW envoy-go-strict departure records (2)

7. **`token_endpoint POST non-2xx retry-eligible → 302 challenge` simplification** per §4.7 + AMEND-3.
8. **`POST callback method PARSE-REJECT`** per §20.P3 + §2.14.

### D. Phase-20 forward-pointer notes (1)

9. **NEW `### Phase 20 forward-pointer notes` subsection** placed immediately after `### Phase 19.2 forward-pointer notes`. Documents: id_token-and-jwks-and-jwt-verifier NEW deferred-cluster; SDS-non-filesystem deferred-cluster (NEW); response_code_details joint divergence-window EXTENSION (now 6 §9 filters); dynamic-metadata family UNCHANGED (oauth2 has no metadata-emit surface); `Partitioned` cookie attribute deferred; `cookie_configs` deferred; PKCE envelope deferred; BASIC_AUTH deferred.

### E. Cross-package umbrella note (1)

10. **REFACTORED-AT-PHASE-20 paragraph appended to `## JWKS framework primitive (per phase 17 ADR-0150)`** documenting the `internal/jwks/Fetcher` refactor (consumes `*httpclient.Client` post-ADR-0150 §Decision AMENDMENT).

### F. Edit-bundle summary

| Category | Count |
|---|---|
| NEW top-level subsection | 1 |
| NEW framework-primitive umbrella subsections | 2 |
| Per-section additions | 3 |
| NEW envoy-go-strict departure records | 2 |
| Phase-20 forward-pointer notes | 1 |
| Cross-package umbrella note | 1 |
| **TOTAL** | **10** |

Anticipated total LoC delta: ~400-500 LoC added (current size ~2796; post-phase-20 ~3200-3300). All 10 edits land at the SAME IMPL commit per ADR-0052; none mutate pre-phase-20 paragraphs (in-place-by-append discipline).

---

## 14. Testing strategy

### 14.1 Unit tests

Test surface at `internal/filter/http/oauth2/*_test.go` + `internal/httpclient/*_test.go` + `internal/sdsfile/*_test.go`. Anticipated ~2000-2500 LoC test code (production-to-test ratio ~1.0-1.3). Table-driven groups:

1. **PARSE-REJECT tests** (`TestBuildCompiledConfig_PARSE_REJECT_*`) — ~30-40 table rows per §14 SPEC discipline. Byte-stable error messages per ADR-0080.
2. **HMAC composition vector tests** (`TestComputeHMAC_*` + `TestValidateHMAC_*` + `TestValidateHMAC_DualEncoding_*`) — ~15-20 vector rows per ADR-0179 + AMEND-2 + S4.
3. **AES-256-CBC encrypt/decrypt vector tests** (`TestEncryptToken_*` + `TestDecryptToken_*`) — ~20-25 vector rows per ADR-0182 + AMEND-1. Includes decrypt-failure fall-back semantics tests per AMEND-3 + §12 item B6.
4. **Cookie envelope round-trip tests** — ~15-20 rows per §6.4 + §6.5. Includes state-cookie payload shape + OauthExpires format tests per §12 item A3.
5. **token_endpoint POST body template tests** — ~15-20 rows per ADR-0185 + AMEND-5. Includes PKCE-gated 5-field template assertion + urlEncode vector tests per §12 item A5.
6. **httpclient unit tests** — ~10-12 tests per ADR-0177 (Options + Client.Do + zero-retry default per §20.P1).
7. **sdsfile unit tests** — ~12-15 tests per ADR-0178 + §12 item B7 (debounce-race coverage).
8. **dispatcher dispatch tests** — ~8-10 priority-order rows per §6.3.
9. **Compile-time invariant tests** — ~5-7 assertions per §6.12.

### 14.2 Race detector + lint

- `go test -race ./...` clean across all packages; specifically: `TestWatcher_DebounceRace_*` + `TestRefreshTokenRotation_Concurrent_*` + `TestAesKeySwap_Concurrent_*` show zero race violations
- `go vet ./...` clean
- `golangci-lint run` clean (no new suppressions)
- `go build ./...` clean

### 14.3 Fuzzer

26th fuzzer `FuzzOAuth2ConfigParse` per §7.4. Must-never-panic discipline across buildCompiledConfig + decode-path parse + cookie-parse + hmac-validate + decrypt-token + buildTokenRequestBody. Clean at 30s per seed.

### 14.4 h2spec + differential

- h2spec 53/53 PASS at ADR-0051 pin (no regression)
- Differential fixture `0024-http-oauth2` per §7 — 9 wire-level expectations across 8 scenario directories; cross-side empirical equivalence; Set-Cookie + token_endpoint POST + 401 body byte-comparisons closing §12 wire-shape items
- Cross-package regression matrix per §12 item C8 + §7.5 D-gate

### 14.5 Six-gate checklist

Per §7.5 — gates A/B/C/D/E/F as the load-bearing IMPL Task 14 verification. All MUST be GREEN for the row-20 status flip.

---

## 15. Acceptance checklist (for the reviewer)

The phase-20 phase-done reviewer (per `BOOTSTRAP_PROMPT.md` §7.6) MUST confirm the following against the landed artefacts. All 18 items MUST be GREEN for row-20 status flip from `in-progress` to `done`.

### A. Six-gate verification (6 items — atomic GREEN per gate)

1. **Gate A — build** : `go build ./...` clean across `internal/filter/http/oauth2/`, `internal/httpclient/`, `internal/sdsfile/`, all pre-existing packages
2. **Gate B — vet + lint** : `go vet ./...` + `golangci-lint run` clean; no new lint suppressions
3. **Gate C — race** : `go test -race ./...` clean; zero data-race violations across all packages including jwks + extauthz regression suites
4. **Gate D — differential** : fixture-0024 GREEN at 9 expectations; cross-side empirical equivalence; cross-package regression matrix GREEN
5. **Gate E — fuzz** : `FuzzOAuth2ConfigParse` clean at 30s per seed; no panics
6. **Gate F — h2spec** : 53/53 PASS at ADR-0051 pin

### B. Fixture-0024 9-scenario coverage (1 item — atomic GREEN over 8 scenarios + 9 wire-level expectations)

7. **Fixture-0024 scenario matrix** per §7.1 — all 8 scenario directories byte-exact-pinned; 9 wire-level expectations GREEN

### C. 6-counter stat-surface verification (1 item — atomic GREEN over 6 counters + 2 ABSENT verifications)

8. **6-counter stat-surface byte-exact** per ADR-0181 + AMEND-4 + §11 §20.P8: `oauth_unauthorized_rq` / `oauth_failure` / `oauth_passthrough` / `oauth_success` / `oauth_refreshtoken_success` / `oauth_refreshtoken_failure`; **ABSENT** `signout_completed` + `cookie_decrypt_failure` verified per S5 + AMEND-4 + §20.P11; stat surface 86 → 92 names

### D. 2-refactor regression matrix (1 item — atomic GREEN over fixtures 0019 + 0020 + 22 other fixtures)

9. **Cross-package regression matrix** per §12 item C8: fixture-0019 (jwt_authn) + fixture-0020 (ext_authz HTTP-mode) GREEN post-refactor; fixture-0021 (ext_authz gRPC-mode) untouched; 22 other pre-existing fixtures GREEN; stat surface byte-stable at 92 names

### E. ADR-0159 forward-pointer closure (1 item — atomic GREEN over ADR + BEHAVIOR_CONTRACT)

10. **ADR-0159 §Future Work CLOSURE-AT-PHASE-20 paragraph** per §3.5 + §9 item 1 + §10 item B + §13 item B5: ADR-0159 §Decision body has new AMENDMENT paragraph; §Future Work has new CLOSED-AT-PHASE-20 paragraph; BEHAVIOR_CONTRACT.md ADR-0159 subsection has CLOSURE-AT-PHASE-20 paragraph; **FIRST §9 family-row to CLOSE a prior-phase load-bearing forward-pointer**

### F. Byte-exact 401 body confirmation (1 item — atomic GREEN over body bytes + Content-Type + no-500 invariant)

11. **Byte-exact 401 body** per §11 §20.P9 + §12 item A1 + AMEND-3: 401 emissions at scenarios (f) + (h) emit `"OAuth flow failed."` (18 bytes; no trailing newline); Content-Type from HCM SendLocalReply default; **NO 500 emissions anywhere** across all 8 scenarios

### G. ADR landing (2 items)

12. **9 NEW ADR §Context drafts + §Decision + §Consequences bodies landed** at per-Task Lands-in-Tasks: ADR-0177 + ADR-0178 + ADR-0179 + ADR-0180 + ADR-0181 + ADR-0182 + ADR-0183 + ADR-0184 + ADR-0185
13. **2 IN-PLACE §Decision AMENDMENT bodies landed at IMPL Task 2**: ADR-0150 AMENDMENT (jwks refactor) + ADR-0159 AMENDMENT + §Future Work CLOSURE

### H. BEHAVIOR_CONTRACT.md edit-bundle (1 item)

14. **10-edit BEHAVIOR_CONTRACT.md bundle landed at IMPL Task 13** per §13 (atomic landing per ADR-0052)

### I. DECISIONS + STATE + ROADMAP advance (3 items)

15. **DECISIONS.md final-state alignment** at IMPL Task 11: 9 NEW ADRs + 2 IN-PLACE AMENDMENTs at final state; cross-references intact
16. **STATE.md re-advanced** at IMPL Task 14: `active-phase` updated per phase-done convention; `lifecycle-state: phase 20 IMPL done`; `next-skill: superpowers:brainstorming` (or per next-phase identity); `last-commit` SHA-fill placeholder; `next-free ADR: ADR-0186`
17. **ROADMAP.md row 20 flipped** to `done` at IMPL Task 14: per-cell IMPL-done annotation added; row stays single-row per ADR-0045

### J. Audit-trail verification (1 item)

18. **End-to-end audit-trail** at phase-done review: SPEC → PLAN → PROGRESS → REVIEW chain landed; per-task PROGRESS records map 1:1 to PLAN tasks; each §11 pin + each §12 item recorded at PROGRESS + REVIEW; cross-phase forward-pointer closures recorded; six-gate verbatim outputs at REVIEW

---

**End SPEC.**
