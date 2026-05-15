// Package extauthzhttp implements a minimal in-process scriptable HTTP
// authorization server for differential fixtures whose driver needs to wire an
// ext_authz http_service endpoint into both envoy.yaml and envoy-go.yaml.
// Used by phase 18.1 fixture 0020-http-ext-authz-http.
//
// Lifecycle: spawn-per-fixture; the runner allocates a free TCP port, starts
// the auth server via New(), wires the http_service.server_uri to that port in
// both yaml configs, runs the scenarios, then stops via Stop(). Scenario 3+4
// (auth-server-unreachable) stop the server before the request.
//
// API surface:
//   - New(ctx, addr, script) (*Server, error) — bind a TCP listener on addr
//     ("127.0.0.1:0" allocates an ephemeral port) and start the auth server
//     with the supplied script.
//   - (*Server).Addr() string — the listener's bound address; load-bearing
//     when the caller supplied "127.0.0.1:0" to allocate an ephemeral port.
//   - (*Server).Stop() error — terminate the server cleanly; idempotent.
//   - FixedScript(status, body, headers) Script — returns a fixed response.
//   - RouteScript(routes, defaultStatus, defaultBody, defaultHeaders) Script
//     — dispatches by path+method; falls back to defaults for unknown routes.
//   - InspectScript(fn) Script — body-inspecting predicate (for scenario 5
//     with_request_body: the auth service receives the buffered body).
//
// Introduced by phase 18.1 Task 10 per planner-time decision D1 — mirrors
// test/helpers/jwksbackend/ structure. Plaintext-only (no TLS in phase 18.1
// per SPEC §7.2 + D12).
package extauthzhttp
