// Package accessloggrpc implements a minimal in-process AccessLogService gRPC
// server that accumulates streamed *HTTPAccessLogEntry records for differential
// fixtures whose driver wires a grpc_access_log sink endpoint into both
// envoy.yaml and envoy-go.yaml. Used by phase 44.1 fixture
// 0081-grpc-access-log.
//
// Lifecycle: spawn-per-fixture (or per-side); the runner allocates a free TCP
// port, starts the server, wires the ALS grpc_service cluster to that port in
// both yaml configs, runs the scenarios, polls Count() to converge, reads
// Entries() to assert, then stops via Stop(). Plaintext h2c (no TLS) per
// D-ALS-RECEIVER. Mirrors test/helpers/extauthzgrpc/ structure.
//
// Accumulation semantics (AMEND-ALS-3): entries are appended across BOTH
// messages within a stream AND across successive streams onto the same slice —
// the differential asserts the aggregated entries, never the stream/message
// framing (which legitimately varies side-to-side).
//
// API surface:
//   - New(t) *Server — bind a TCP listener on 127.0.0.1:0 (ephemeral port) and
//     start the gRPC AccessLogServiceServer. Registers t.Cleanup(Stop).
//   - NewAtAddr(addr) (*Server, error) — bind a caller-supplied `host:port`
//     (e.g. "0.0.0.0:<port>" for Docker reachability). No t.Cleanup — the
//     caller manages lifecycle via Stop.
//   - (*Server).Addr() string — the listener's bound address; load-bearing
//     because New supplies the ephemeral port at New time.
//   - (*Server).StreamAccessLogs(stream) error — implements
//     accesslogv3.AccessLogServiceServer: drains the client stream, appending
//     every message's HTTP log entries to the accumulator, and SendAndClose on
//     io.EOF.
//   - (*Server).Entries() []*HTTPAccessLogEntry — a defensive snapshot copy of
//     the accumulated entries, in arrival order.
//   - (*Server).Count() int — the accumulated entry count (the driver's
//     converge poll).
//   - (*Server).Reset() — drop all accumulated entries (per-side snapshot
//     separation when one server is reused across sides).
//   - (*Server).Stop() — GracefulStop the *grpc.Server; idempotent via the
//     t.Cleanup registration.
//
// Test-only: this package MUST NOT be imported by any non-test production code.
package accessloggrpc
