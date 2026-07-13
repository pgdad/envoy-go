// Package sdsserver implements a minimal in-process SecretDiscoveryService
// gRPC server that delivers a configured Secret{name, tls_certificate{inline
// PEM}} on the first StreamSecrets request and records every received
// *DiscoveryRequest, for tests that exercise the internal/xds SDS provider
// against a real (fake) management server. Mirrors test/helpers/accessloggrpc
// structure.
//
// Driver-owned (reference_differential_grpc_receiver_driver_owned): a test
// DIALS this server directly — it is NOT wired into the differential runner
// as a BackendKind.
//
// API surface:
//   - New(t, opts...) *Server — bind a TCP listener on 127.0.0.1:0 (ephemeral
//     port) and start the gRPC SecretDiscoveryServiceServer. Registers
//     t.Cleanup(Stop).
//   - WithSecret(name, certPEM, keyPEM) Option — configure the delivered
//     Secret's name and inline tls_certificate PEM bytes.
//   - Silent() Option — accept the stream but never Send a response, driving
//     the client's initial_fetch_timeout path.
//   - (*Server).Addr() string — the listener's bound address; load-bearing
//     because New supplies the ephemeral port at New time.
//   - (*Server).StreamSecrets(stream) error — implements
//     secretv3.SecretDiscoveryServiceServer: records every received request
//     and, on the first one (unless Silent), Sends the configured Secret.
//   - (*Server).Requests() []*discoveryv3.DiscoveryRequest — a defensive
//     snapshot copy of the received requests, in arrival order.
//   - (*Server).Stop() — GracefulStop the *grpc.Server; idempotent via the
//     t.Cleanup registration.
//
// Test-only: this package MUST NOT be imported by any non-test production code.
package sdsserver
