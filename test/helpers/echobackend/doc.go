// Package echobackend implements a minimal HTTP/1.1 echo backend that echoes
// inbound request method + URL.Path + Header (lowercase canonical form per
// ADR-0072 + phase-04 lowercase-header pattern) as a JSON body in its response.
//
// Used by differential fixtures whose driver needs to assert on upstream-side
// request shape (e.g., per-route remove_accept_encoding_header verification —
// phase 14 fixture 0016 scenario 6).
//
// Introduced by phase 14 per planner-time decision 6 + planner-time decision
// 12 (D7 settlement). Future filter fixtures needing echo-backend behavior MAY
// use this shared helper. Phase 13 buffer's per-fixture backend at
// test/fixtures/0015-http-buffer/backends/backend.go MAY be migrated to use
// this helper in a future cleanup (out of scope for phase 14).
//
// API surface:
//   - New() *http.Server — returns a configured *http.Server with the echo
//     handler installed; the caller binds via srv.Serve(listener).
//   - Listen(port int) (net.Listener, error) — helper that allocates the TCP
//     listener bound to the caller-specified port. Used by the cmdline wrapper
//     at cmd/echobackend/main.go.
//
// Used-by: phase 14 fixture 0016 scenario 6 (per-route rmAE assertion via
// JSON-encoded inbound-header echo).
package echobackend
