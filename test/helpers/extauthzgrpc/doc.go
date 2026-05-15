// Package extauthzgrpc implements a minimal in-process scriptable
// Authorization/Check gRPC server for differential fixtures whose driver
// needs to wire an ext_authz grpc_service endpoint into both envoy.yaml
// and envoy-go.yaml. Used by phase 18.2 fixture 0021-http-ext-authz-grpc.
// THE FIRST in-process gRPC server in envoy-go's test tree.
//
// Lifecycle: spawn-per-fixture; the runner allocates a free TCP port,
// starts the server via New(t), wires the EnvoyGrpc.cluster_name to a
// cluster pointing at that port in both yaml configs, runs the scenarios,
// then stops via Stop(). Plaintext h2c (no TLS) per SPEC §7.2; per-:path
// scriptable CheckResponse per planner-time decision D1.
//
// API surface:
//   - New(t) *Server — bind a TCP listener on 127.0.0.1:0 (ephemeral port)
//     and start the gRPC AuthorizationServer. Registers t.Cleanup(Stop).
//   - (*Server).Addr() string — the listener's bound address; load-bearing
//     because t supplies the ephemeral port at New time.
//   - (*Server).Script(path, resp) — register a scripted *authv3.CheckResponse
//     for the discriminator key (the `:path` from req.Attributes.Request.Http.Path).
//   - (*Server).Check(ctx, req) (*authv3.CheckResponse, error) — implements
//     authv3.AuthorizationServer: looks up the scripted response by `:path`;
//     returns it; returns codes.Unavailable when no script matches (the
//     ext_authz filter maps codes.Unavailable transport errors to dispError).
//   - (*Server).Stop() — GracefulStop the *grpc.Server; idempotent via the
//     t.Cleanup registration.
//
// Introduced by phase 18.2 Task 9 per planner-time decision D1 — mirrors
// test/helpers/extauthzhttp/ + test/helpers/jwksbackend/ structure. Plaintext
// h2c only — no TLS — per SPEC §7.2 + §11.P13 in-session scrape ratification
// (the fixture's auth-cluster is also plaintext h2c).
package extauthzgrpc
