// Package ratelimitgrpc implements a minimal in-process scriptable
// RateLimitService gRPC server for differential fixtures whose driver needs
// to wire a rate_limit_service grpc_service endpoint into both envoy.yaml
// and envoy-go.yaml. Used by phase 24.1 fixture 0032-http-ratelimit and
// 0033-http-ratelimit-boot-reject. Clones the test/helpers/extauthzgrpc/
// (fixture 0021) shape — the SECOND in-process gRPC server in envoy-go's
// test tree.
//
// Lifecycle: spawn-per-fixture; the runner allocates a free TCP port,
// starts the server via NewAtAddr(addr), wires the EnvoyGrpc.cluster_name
// to a cluster pointing at that port in both yaml configs, runs the
// scenarios, then stops via Stop(). Plaintext h2c (no TLS) per parent
// SPEC §7.2.
//
// API surface:
//   - New(t) *Server — bind a TCP listener on 127.0.0.1:0 (ephemeral port)
//     and start the gRPC RateLimitServiceServer. Registers t.Cleanup(Stop).
//   - NewAtAddr(addr) (*Server, error) — bind the caller-supplied
//     "host:port" (allocated upstream via Listen+Close + rebind, the same
//     idiom fixture-0021 uses to pin a stable rls-cluster endpoint before
//     bootstrap YAMLs render); no t.Cleanup registration — caller manages
//     lifecycle via Server.Stop.
//   - (*Server).Addr() string — the listener's bound address; load-bearing
//     because t supplies the ephemeral port at New time.
//   - (*Server).Script(key, resp) — register a scripted
//     *ratelimitv3.RateLimitResponse for the canonical descriptor-list key
//     (one entry's "key=value" segments joined with ';' per descriptor;
//     descriptors joined with '|'). Drivers use CanonicalKey(req) to derive
//     the same key the fake computes at ShouldRateLimit time.
//   - (*Server).ShouldRateLimit(ctx, req) (*RateLimitResponse, error) —
//     implements ratelimitv3.RateLimitServiceServer: looks up the scripted
//     response by the canonical descriptor-list key; on unregistered key
//     returns a default OK response (RateLimitResponse_OK + per-descriptor
//     OK statuses) so unscripted scenarios pass through cleanly.
//   - (*Server).Stop() — GracefulStop the *grpc.Server; idempotent via
//     sync.Once. Registered as t.Cleanup at New time.
//
// CRITICAL — D-RL5 / AMEND-6 proto-number-faithful encoding (parent §1.1
// + §7.1): the fake builds RateLimitResponse via Go-protobuf struct
// literals — setting ONLY the fields the scenario explicitly wants.
// Unset optionals (raw_body, dynamic_metadata, quota, per-descriptor
// current_limit / limit_remaining / duration_until_reset / quota) are
// omitted from the wire because Go-protobuf elides zero-value scalars
// and nil-pointer messages by default. Cross-side byte-exactness between
// the reference Envoy + envoy-go OVER_LIMIT replies depends on this:
// both sides emit byte-equivalent local replies only when the fake's
// response bytes are deterministic + minimal.
//
// Introduced by phase 24.1 Task 9 per planner-time decisions D-RL5 +
// AMEND-6 — mirrors test/helpers/extauthzgrpc/ structure verbatim
// (RegisterRateLimitServiceServer + ShouldRateLimit replacing
// RegisterAuthorizationServer + Check). Plaintext h2c only — no TLS —
// per parent SPEC §7.2.
package ratelimitgrpc
