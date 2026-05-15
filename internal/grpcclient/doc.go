// Package grpcclient implements the outbound-gRPC framework primitive at the
// NEW top-level package `internal/grpcclient/` per ADR-0158. The package is
// strategically positioned for cross-phase reuse: a thin generic `Dialer`
// (cluster-name → `*grpc.ClientConn` via `internal/cluster.Manager` coupling)
// + a typed `AuthClient` wrapper (`envoy.service.auth.v3.AuthorizationClient`
// stub from `go-control-plane v1.32.4` — no codegen). Future ext_proc +
// global_ratelimit phases reuse the `Dialer` and add their own typed wrappers
// per ADR-0158 §Consequences. Mirrors the phase-17 ADR-0150 discipline
// (`internal/jwks/Fetcher` typed-wrapper-around-stdlib-`http.Client`) — the
// outbound-HTTP framework-primitive precedent for outbound-from-filter
// transport-aware framework primitives.
//
// API surface (per ADR-0158 §Decision + SPEC §3.1):
//
//   - `Dialer` opaque type — couples to `internal/cluster.Manager` for
//     cluster-name → endpoint + TLS resolution.
//   - `New(mgr *cluster.Manager) *Dialer` — constructs a dialer rooted at the
//     supplied cluster manager.
//   - `(*Dialer).DialContext(ctx, clusterName) (*grpc.ClientConn, error)` —
//     PARSE-REJECTs (via error return) when the cluster does not exist OR
//     `UseH2()==false` (gRPC requires HTTP/2 framing). Internally uses
//     `grpc.NewClient("passthrough:///"+clusterName, grpc.WithContextDialer(...),
//     grpc.WithTransportCredentials(insecure.NewCredentials()))` per D4.
//   - `AuthClient` typed wrapper — pairs a `*grpc.ClientConn` with the
//     `envoy.service.auth.v3.AuthorizationClient` stub.
//   - `NewAuthClient(d *Dialer, clusterName, timeout) (*AuthClient, error)` —
//     dials the named cluster and wraps it in the typed stub. `timeout` is
//     per-Check (applied via `context.WithTimeout` inside `Check`) per D7/D9.
//   - `(*AuthClient).Check(ctx, *CheckRequest) (*CheckResponse, error)` —
//     invokes `Authorization/Check`. Per-Check `context.WithTimeout(ctx,
//     timeout)` applied INSIDE `Check` per SPEC §3.1 + D7. Transport-level
//     errors (`Unavailable` / `DeadlineExceeded` / `Canceled`) propagate
//     VERBATIM to the caller — `mapGRPCResponse` (the filter-layer mapper)
//     never sees a transport error.
//   - `(*AuthClient).Close() error` — releases the underlying
//     `*grpc.ClientConn`; idempotent (sync.Once-guarded).
//
// Cluster-manager coupling (per SPEC §3.1 + §11.P13 in-session RATIFICATION):
//
//   - Endpoint resolution: `mgr.Get(clusterName) (*Cluster, bool)` — the
//     PARSE-REJECT-on-false gate at `DialContext` time.
//   - HTTP/2 gate: `(*Cluster).UseH2() bool` — the PARSE-REJECT-on-false
//     gate at `DialContext` time (gRPC framing requires HTTP/2 upstream
//     origination via `http2_protocol_options{}`).
//   - Per-dial endpoint pick: `(*Cluster).Dial(ctx) (net.Conn, Endpoint, error)`
//     — wrapped in `grpc.WithContextDialer` so gRPC layers framing on top of
//     the cluster-manager-owned `net.Conn`.
//   - TLS handling: TLS terminates at the cluster-manager layer (the cluster's
//     `transport_socket: UpstreamTlsContext` parsing). gRPC uses
//     `WithTransportCredentials(insecure.NewCredentials())` because we hand
//     it a TLS-wrapped `net.Conn` from the cluster manager — gRPC does NOT
//     redo TLS. RATIFIED against reference Envoy v1.37.2 per the §11.P13
//     in-session scrape.
//
// Connection lifecycle (per ADR-0158 §Decision + D2):
//
//   - One `*grpc.ClientConn` per (cluster_name, `*compiledConfig`) pair —
//     allocated at config-load time (the filter's `buildGRPCCheckFn` closure
//     captures a `*AuthClient`); shared across all per-stream `Check()` calls
//     on that compiledConfig. The `*grpc.ClientConn` is goroutine-safe per
//     the gRPC library and manages its own transport-level reconnect via the
//     sub-channel state machine.
//   - On process exit, the `*AuthClient` is GC'd; `Close()` is NOT explicitly
//     called for MVP — leaks-on-exit per D2 (envoy-go has no config hot-reload
//     yet; the process lifecycle bounds the connection). A future hot-reload
//     phase will land a close-on-replacement discipline per a new ADR (NOT
//     18.2).
//
// Cross-phase reuse intent (ADR-0158 §Consequences):
//
//   - ext_proc reuses `Dialer.DialContext` and composes its own
//     `*ProcessorClient` wrapping
//     `envoy.service.ext_proc.v3.ExternalProcessor.Process` (bidi-stream —
//     extends the unary Check pattern).
//   - global_ratelimit reuses `Dialer.DialContext` and composes a
//     `*RateLimitClient` wrapping
//     `envoy.service.ratelimit.v3.RateLimitService.ShouldRateLimit` (unary —
//     structurally identical to ext_authz's Check).
//   - The `Dialer` surface is intentionally minimal; no future client
//     coupling is anticipated to require `Dialer` API changes.
//
// ADR anchors: ADR-0158 (this package), ADR-0157 (the `checkFn` closure
// signature that consumes `AuthClient` from the filter layer), parent
// phase-18 SPEC §5.P1 (the proto-package origin of the
// `AuthorizationClient` stub).
package grpcclient
