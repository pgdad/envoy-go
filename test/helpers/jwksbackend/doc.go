// Package jwksbackend implements a minimal in-process HTTP JWKS server for
// differential fixtures whose driver needs to wire a RemoteJwks endpoint into
// both envoy.yaml and envoy-go.yaml. Used by phase 17 fixture 0019-http-jwt-authn
// for scenarios 1 + 7 (RemoteJwks-served JWK Sets).
//
// Lifecycle: spawn-per-fixture; the runner (or fixture driver at Task 11)
// allocates a free TCP port, starts the JWKS backend on that port via New(),
// wires the cluster c_jwks_backend to that port in both YAML configs, runs the
// scenarios, then stops the backend at teardown via Stop().
//
// API surface:
//   - New(ctx, addr, routes) (*Server, error) — bind a TCP listener on addr
//     ("127.0.0.1:0" allocates an ephemeral port) and serve the supplied
//     URL-path → JWK-Set-JSON map. Missing routes return 404; non-GET methods
//     return 405.
//   - (*Server).Addr() string — the listener's bound address; load-bearing
//     when the caller used "127.0.0.1:0" to allocate an ephemeral port.
//   - (*Server).Stop() error — terminate the server cleanly; idempotent.
//
// Introduced by phase 17 Task 10 per planner-time decision 12 (D7 settlement)
// — the SECOND consumer of the shared-helper pattern (phase-14 echobackend is
// the FIRST). Plaintext-only (no TLS in phase 17 per SPEC §7.4).
package jwksbackend
