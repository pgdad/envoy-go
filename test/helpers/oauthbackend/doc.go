// Package oauthbackend implements a minimal in-process scriptable OAuth 2.0
// authorization-server mock for the phase-20 differential fixture
// 0024-http-oauth2 + future fixtures that exercise the
// envoy.filters.http.oauth2 filter's outbound token_endpoint POST + the
// authorization_endpoint 302-bounce.
//
// Per phase-20 SPEC §7.3 the helper hosts two route families:
//
//   - authorization_endpoint (GET /authorize) — scripted 302 → callback
//     responses for the sign-in (a) leg.
//   - token_endpoint (POST /token) — scripted JSON OAuth 2.0 token
//     responses (RFC 6749 §5.1) for the (a) success + (g) 5xx + (h) 4xx
//     scenarios, and refresh-token responses for scenario (d).
//
// # Lifecycle
//
// Spawn-per-fixture; the driver allocates a free TCP port, starts the
// server via New(t), then registers per-method+path Scripts via Script().
// The server runs httptest.NewServer-style under the hood (net.Listen on
// the requested addr; an http.Server goroutine). The driver wires the
// server's bound addr into both envoy.yaml + envoy-go.yaml token_endpoint
// / authorization_endpoint URLs.
//
// # API surface
//
//   - New(t testing.TB) *Server — bind 127.0.0.1:0 + start.
//   - NewAtAddr(addr string) (*Server, error) — caller-chosen-port arm
//     for fixture drivers that pre-allocate a stable port before
//     bootstrap rendering.
//   - (*Server).Addr() string — bound `host:port` for templating.
//   - (*Server).Script(method, path string, status int, body []byte, headers map[string]string)
//     — register the per-route scripted response.
//   - (*Server).TokenResponse(accessToken, refreshToken, idToken string, expiresIn int)
//     — convenience helper for the standard 200 OK JSON success body
//     ({"access_token":"...","token_type":"Bearer","expires_in":N,"refresh_token":"..."}).
//   - (*Server).Received() []ReceivedRequest — returns a copy of all
//     requests received so drivers can assert observability invariants
//     (which paths/methods fired + their bodies).
//   - (*Server).Stop() — graceful shutdown; idempotent via sync.Once.
//
// # Auxiliary helpers (envelope construction for ValidCookieEnvelope / TamperedStateCookie)
//
// The package also exposes pure helpers that construct the 5-cookie
// envelope (or tampered variants) given the HMAC secret + AES key
// material. These are consumed by drivers that need to seed a request
// with a pre-built envelope for the (b1) cookie-passthrough scenario or
// the (b2) tampered envelope scenario:
//
//   - ValidCookieEnvelope(hmacSecret []byte, accessToken, refreshToken, idToken, domain string, expiresEpoch int64) []*http.Cookie
//   - TamperedStateCookie(hmacSecret []byte, stateValue string) *http.Cookie
//
// # Plaintext HTTP only — no TLS
//
// Per phase-20 SPEC §7 + the in-process-helper precedent (extauthzhttp /
// jwksbackend) the mock is plaintext HTTP. The differential harness runs
// reference Envoy in a Docker container; for the container to reach the
// host-side mock, the bootstrap renders the mock URL with the
// `host.docker.internal` host per ADR-0010 (reference container reaches
// host loopback via the docker-host bridge).
package oauthbackend
