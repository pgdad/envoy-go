// Package jwks implements the HTTP-outbound JWKS fetcher framework primitive
// at the NEW top-level package `internal/jwks/` per ADR-0150. This package is
// strategically positioned for cross-phase reuse: the `Fetcher` opaque type +
// retry-policy structure compose against future auth-filter family flows —
// `ext_authz` HTTP-mode (POST to external HTTP auth service) and `oauth2`
// token-endpoint flows (fetches access tokens; refresh on 401) both reuse the
// same outbound-HTTP-from-filter primitive. Mirrors phase-16 ADR-0142
// (`internal/matcher/`) + ADR-0144 (DownstreamPrincipal accessor) cross-phase-
// reusable-framework-primitive discipline.
//
// API surface (per ADR-0150 §Decision):
//
//   - `Fetcher` opaque type — one per RemoteJwks JwtProvider; lifetime is
//     listener-lifetime; owns a background-refresh goroutine.
//   - `New(uri, cacheDuration, asyncFetch, retryPolicy) (*Fetcher, error)` —
//     constructs a fetcher. (i) `fast_listener=false` (DEFAULT): performs a
//     BLOCKING initial fetch; returns error from New on failure (fail-loud-at-
//     listener-load discipline per planner-time decision 3). (ii)
//     `fast_listener=true`: returns immediately; spawns goroutine for initial
//     fetch; subsequent `Get` returns `ErrJwksNotReady` until ready channel
//     closes.
//   - `(*Fetcher).Get(ctx)` — returns the cached `*JWKSet` OR `ErrJwksNotReady`
//     in fast_listener-mode-before-completion; thread-safe under refresh.
//   - `(*Fetcher).Close()` — terminates the background refresh goroutine
//     (`refreshTimer.Stop()` + atomic-closed flag); idempotent.
//   - `(*JWKSet).Lookup(kid, alg)` — Envoy pickKeyAlgWithKid logic: if kid
//     non-empty find kid-match; among kid-matched prefer alg-match (case-
//     insensitive); fall back to first kid-match. If kid empty, prefer first
//     alg-match; fall back to first key with empty Alg.
//   - `ParseJWKSet(raw []byte) (*JWKSet, error)` — RFC 7517 §5 JWK Set JSON
//     parser; supports `kty: "RSA"` (n + e base64url-decoded) + `kty: "EC"`
//     (P-256 / P-384 / P-521 via `crypto/elliptic`). Used both internally by
//     Fetcher (for RemoteJwks responses) AND by Task 4's `internal/jwt/`
//     LocalJwks wiring per the PLAN's File-structure table.
//
// Refresh schedule semantics (per §11.P5 RATIFIED via Envoy
// `jwks_async_fetcher.cc` scrape + ADR-0150 §Decision):
//
//   - Default `cacheDuration`: 10 minutes (`DefaultCacheExpirationSec=600s`).
//   - Refresh fires `max(cacheDuration - 5s, 0)` BEFORE TTL expiry via
//     `time.AfterFunc` (mirrors Envoy's `RefetchBeforeExpiredSec=5s`).
//   - Clamped to 0 if `cacheDuration < 5s` (no negative lead-time; refresh
//     immediately).
//
// Failed-refetch semantics (per §11.P4 REFUTES BRAINSTORM exponential-backoff
// hypothesis):
//
//   - FIXED-INTERVAL 1s default per `DefaultRefetchAfterFailedSec`; configurable
//     via `AsyncFetch.FailedRefetchDuration`.
//   - NO exponential backoff; NO max-retries cap; NO jitter at the outer-refetch
//     level (REFUTES BRAINSTORM hypothesis).
//
// Inner-HTTP-request retry policy (separate from outer refetch):
//
//   - `RetryPolicy.NumRetries` (default 1 per Envoy proto comment).
//   - `RetryPolicy.BaseInterval` (default 1s).
//   - `RetryPolicy.MaxInterval` (default 10*BaseInterval).
//   - Applied per HTTP request inside one refresh cycle; the outer refresh
//     schedule is unaffected by inner retry exhaustion.
//
// JWK Set JSON parsing (RFC 7517 §5):
//
//   - RSA: decode `n` and `e` via `base64.RawURLEncoding`; convert to
//     `*rsa.PublicKey` via `big.Int.SetBytes`.
//   - EC: decode `x` and `y`; lookup curve from `crv` field (P-256 →
//     `elliptic.P256()`, P-384 → `elliptic.P384()`, P-521 → `elliptic.P521()`);
//     construct `*ecdsa.PublicKey`.
//   - Unsupported `kty` (anything other than `RSA` or `EC`): the offending key
//     is SKIPPED silently; the JWK Set is still considered parsed iff at least
//     one valid key remains. All-keys-unsupported yields `ErrJwksNoValidKeys`.
//
// Lifecycle ownership (per ADR-0150 §Decision + planner-time decision 9): the
// `*Fetcher` is owned by `compiledProvider.jwksFetcher` (created at
// `buildCompiledProvider` time); lifetime is listener-lifetime; shared across
// all filter instances of the listener. Filter `OnDestroy` does NOT close the
// fetcher.
package jwks
