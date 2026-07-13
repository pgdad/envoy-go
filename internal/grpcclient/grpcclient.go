package grpcclient

// grpcclient.go — `Dialer` + `AuthClient` real implementation per SPEC §3.1 +
// ADR-0158 §Decision. The Task-2 skeleton sentinels (`errTODOTask3`) are
// retired; the five method bodies (New, DialContext, NewAuthClient, Check,
// Close) carry their real bodies per planner-time decisions D2/D4/D7/D9.
//
// Design notes (per ADR-0158 §Decision):
//
//   - `DialContext` construction (D4 — `passthrough:///` resolver): we use
//     `grpc.NewClient("passthrough:///"+clusterName, ...)` so gRPC skips DNS
//     resolution and delegates endpoint selection to the `WithContextDialer`
//     callback. The callback re-looks-up the cluster via `mgr.Get(clusterName)`
//     (the second lookup is intentional — the first happens BEFORE the dial
//     for the PARSE-REJECT gates; the second happens INSIDE the dialer
//     callback every time the gRPC sub-channel needs a fresh net.Conn).
//
//   - `WithTransportCredentials(insecure.NewCredentials())`: TLS terminates at
//     the cluster-manager layer (the cluster's `transport_socket:
//     UpstreamTlsContext`). gRPC sees a plain `net.Conn` regardless of
//     upstream cluster TLS state — gRPC does NOT redo TLS. RATIFIED against
//     reference Envoy v1.37.2 per the §11.P13 in-session SPEC scrape.
//
//   - PARSE-REJECT gates (per SPEC §3.1):
//     1. `mgr.Get(clusterName)` returns `(_, false)` → unknown cluster.
//     2. `(*Cluster).UseH2() == false` → gRPC requires HTTP/2 framing.
//     Both errors include the cluster name to aid diagnostics.
//
//   - `AuthClient.Check` per-Check timeout discipline (D7 + D9): the
//     `timeout` captured at `NewAuthClient` construction is applied INSIDE
//     `Check` via `context.WithTimeout(ctx, timeout)` (deferred cancel). The
//     caller's `ctx` is the parent — its cancellation propagates through
//     `context.WithTimeout`'s AND-of-cancellation semantics. Transport errors
//     (`Unavailable` / `DeadlineExceeded` / `Canceled`; `context.Canceled`;
//     `context.DeadlineExceeded`) propagate VERBATIM to the caller. A zero
//     `timeout` means "no per-Check timeout" — the caller's `ctx` alone
//     bounds the call.
//
//   - `AuthClient.Close` idempotency (per SPEC §3.1): the body is guarded by
//     `sync.Once` so repeated calls return the cached error from the first
//     call. The underlying `*grpc.ClientConn.Close()` is itself idempotent
//     per the gRPC library, but the sync.Once guard normalizes the error
//     return + is concurrent-safe.
//
//   - Connection lifecycle (D2 — leaks-on-exit MVP): the `*grpc.ClientConn`
//     is owned by the caller; the filter layer captures it in the `checkFn`
//     closure and never explicitly calls `Close` in production. On process
//     exit, the OS reclaims the connection. Future hot-reload phases will
//     introduce a close-on-replacement discipline (see ADR-0158 §Consequences).

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	accesslogv3 "github.com/envoyproxy/go-control-plane/envoy/service/accesslog/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	metricsv3 "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
	secretv3 "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/pgdad/envoy-go/internal/cluster"
)

// ----------------------------------------------------------------------------
// Dialer — the cross-phase-reusable cluster-name → `*grpc.ClientConn` layer.
// ----------------------------------------------------------------------------

// Dialer constructs `*grpc.ClientConn` values for envoy-go cluster references.
// It couples to `internal/cluster.Manager` for cluster-name → endpoint + TLS
// resolution. Cross-phase-reusable for future ext_proc + global_ratelimit per
// ADR-0158 §Consequences.
//
// Concurrency: a `*Dialer` is safe for concurrent use (it holds only a
// reference to a `*cluster.Manager`, which is itself goroutine-safe per its
// own documentation).
type Dialer struct {
	mgr *cluster.Manager
}

// New returns a `*Dialer` rooted at the supplied cluster manager. The
// returned `*Dialer` does NOT own the manager — the caller (the bootstrap
// machinery) retains ownership; the dialer is a stateless wrapper.
func New(mgr *cluster.Manager) *Dialer {
	return &Dialer{mgr: mgr}
}

// DialContext returns a `*grpc.ClientConn` for the named cluster. The
// underlying transport is provided by `(*cluster.Cluster).Dial` — the cluster
// manager owns endpoint selection + TLS termination; gRPC is layered on top
// of the resulting `net.Conn` via `grpc.WithContextDialer`.
//
// PARSE-REJECTs (via error return) if:
//   - the dialer's cluster manager is nil.
//   - the cluster does not exist (`mgr.Get(clusterName)` returns `(_, false)`).
//   - the cluster's `UseH2()` returns `false` (gRPC requires HTTP/2 framing).
//
// The returned `*grpc.ClientConn` is owned by the caller and `Close()`d at
// shutdown (or, for the MVP per D2, leaked-on-exit; the OS reclaims it).
//
// `ctx` is observed during the synchronous construction phase — `grpc.NewClient`
// itself does NOT dial eagerly; the first dial fires on the first RPC. But
// `ctx.Err()` is checked up front to surface caller-cancel early.
func (d *Dialer) DialContext(ctx context.Context, clusterName string) (*grpc.ClientConn, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("grpcclient: dial %q: %w", clusterName, err)
	}
	if d == nil || d.mgr == nil {
		return nil, fmt.Errorf("grpcclient: dial %q: cluster manager is nil", clusterName)
	}
	clu, ok := d.mgr.Get(clusterName)
	if !ok {
		return nil, fmt.Errorf("grpcclient: dial %q: unknown cluster", clusterName)
	}
	if !clu.UseH2() {
		return nil, fmt.Errorf("grpcclient: dial %q: cluster does not have http2_protocol_options{} set (gRPC requires HTTP/2 framing)", clusterName)
	}

	// `grpc.NewClient` does NOT block; the underlying sub-channel state machine
	// dials lazily on first RPC (or on a `WaitForReady` call). The PARSE-REJECT
	// gates above are the only synchronous checks; everything else surfaces as
	// transport errors at `Check` time per D7.
	conn, err := grpc.NewClient(
		"passthrough:///"+clusterName,
		grpc.WithContextDialer(func(callCtx context.Context, _ string) (net.Conn, error) {
			// Re-lookup at dial time so the latest cluster manager state is
			// observed — future xDS-CDS hot-reload will mutate Manager state.
			c, ok := d.mgr.Get(clusterName)
			if !ok {
				return nil, fmt.Errorf("grpcclient: dialer callback: cluster %q vanished", clusterName)
			}
			nc, _, err := c.Dial(callCtx)
			return nc, err
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("grpcclient: dial %q: %w", clusterName, err)
	}
	return conn, nil
}

// ----------------------------------------------------------------------------
// connHolder + dialConn — the shared skeleton every typed wrapper builds on.
// ----------------------------------------------------------------------------

// connHolder is the shared `*grpc.ClientConn` + idempotent-close base embedded
// by every typed wrapper in this package (AuthClient, ALSClient,
// MetricsServiceClient, OTLPLogsClient, OTLPTracesClient, RateLimitClient,
// ProcessorClient). It consolidates the previously seven byte-identical
// `closeOnce`-guarded Close bodies into one implementation.
//
// The close body is guarded by `sync.Once` per SPEC §3.1 so repeated calls
// (or concurrent calls) return the cached error from the first call. The
// underlying `*grpc.ClientConn.Close()` is itself idempotent per the gRPC
// library, but the sync.Once guard normalizes the error return + is
// concurrent-safe.
//
// Each wrapper keeps its own nil-receiver-tolerant Close() method that
// delegates here — method promotion alone would panic on a nil outer pointer,
// and the nil-tolerance contract is part of every wrapper's documented API.
type connHolder struct {
	conn *grpc.ClientConn

	// closeOnce + closeErr guard the idempotent close per SPEC §3.1.
	closeOnce sync.Once
	closeErr  error
}

// close releases the underlying `*grpc.ClientConn`. Idempotent — repeated
// calls (or concurrent calls) return the cached error from the first call
// via the internal `sync.Once` guard.
func (h *connHolder) close() error {
	h.closeOnce.Do(func() {
		if h.conn != nil {
			h.closeErr = h.conn.Close()
		}
	})
	return h.closeErr
}

// dialConn is the shared nil-dialer-check + background-context dial performed
// by every wrapper constructor. `kind` is the constructor-specific error-noun
// (e.g. "auth client", "processor client") parameterizing the nil-dialer
// error so every constructor's existing error string stays byte-identical.
//
// On dial error the error is passed through verbatim (already includes the
// cluster name via `DialContext`'s error wrapping).
func dialConn(d *Dialer, kind, clusterName string) (*grpc.ClientConn, error) {
	if d == nil {
		return nil, fmt.Errorf("grpcclient: new %s %q: dialer is nil", kind, clusterName)
	}
	return d.DialContext(context.Background(), clusterName)
}

// ----------------------------------------------------------------------------
// AuthClient — the typed `Authorization/Check` wrapper.
// ----------------------------------------------------------------------------

// AuthClient wraps a `*grpc.ClientConn` with the typed
// `envoy.service.auth.v3.AuthorizationClient` stub from `go-control-plane
// v1.32.4`. One `*AuthClient` per (cluster_name, `*compiledConfig`) pair —
// allocated at config-load time (in `buildGRPCCheckFn`); shared across all
// per-stream `Check()` calls on that compiledConfig.
//
// Concurrency: a `*AuthClient` is safe for concurrent use — the underlying
// `*grpc.ClientConn` is goroutine-safe per the gRPC library and manages its
// own transport-level reconnect via the sub-channel state machine.
type AuthClient struct {
	connHolder
	stub authv3.AuthorizationClient

	timeout time.Duration // per-Check timeout (applied via context.WithTimeout in Check)
}

// NewAuthClient dials the named cluster via `d.DialContext` and wraps the
// resulting `*grpc.ClientConn` in a typed `AuthClient`. `timeout` is the
// per-Check timeout — applied inside `Check` via `context.WithTimeout` per
// D7/D9; a zero `timeout` means no per-Check timeout (the caller's `ctx`
// alone bounds the call).
//
// On dial error, `NewAuthClient` returns `(nil, err)` — the dial error is
// passed through verbatim (already includes the cluster name via
// `DialContext`'s error wrapping).
func NewAuthClient(d *Dialer, clusterName string, timeout time.Duration) (*AuthClient, error) {
	conn, err := dialConn(d, "auth client", clusterName)
	if err != nil {
		return nil, err
	}
	return &AuthClient{
		connHolder: connHolder{conn: conn},
		stub:       authv3.NewAuthorizationClient(conn),
		timeout:    timeout,
	}, nil
}

// Check invokes `Authorization/Check` on the wrapped `*grpc.ClientConn`.
//
// Per-Check timeout discipline (D7/D9): when `a.timeout > 0` the call is
// bounded by `context.WithTimeout(ctx, a.timeout)` (deferred cancel). The
// caller's `ctx` is the parent — `OnDestroy`-driven cancellation propagates
// through `context.WithTimeout`'s AND-of-cancellation semantics. BOTH paths
// (caller-cancel AND per-Check timeout) surface as transport errors via the
// `error` return.
//
// Transport-level errors (gRPC `Unavailable` / `DeadlineExceeded` /
// `Canceled`; `context.Canceled`; `context.DeadlineExceeded`) propagate
// VERBATIM to the caller — `Check` does NOT classify or map them. The
// filter-layer caller (the `checkFn` closure body) maps transport errors to
// `dispError` BEFORE inspecting the `*CheckResponse` per D7.
//
// On success, `Check` returns `(*CheckResponse, nil)` — the body operates on
// the response; the filter-layer caller invokes `mapGRPCResponse(resp, ...)`
// per ADR-0161.
func (a *AuthClient) Check(ctx context.Context, req *authv3.CheckRequest) (*authv3.CheckResponse, error) {
	if a == nil || a.stub == nil {
		return nil, errors.New("grpcclient: Check: nil AuthClient / stub")
	}
	if a.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.timeout)
		defer cancel()
	}
	return a.stub.Check(ctx, req)
}

// Close releases the underlying `*grpc.ClientConn`. Idempotent — repeated
// calls (or concurrent calls) return the cached error from the first call
// via the shared `connHolder` sync.Once guard. A nil receiver is tolerated
// (returns nil).
//
// MVP discipline (D2): in production the filter never calls `Close()` — the
// `*grpc.ClientConn` is leaked-on-exit; the OS reclaims it. `Close()` exists
// primarily for the Group 3 unit tests and for future hot-reload phases.
func (a *AuthClient) Close() error {
	if a == nil {
		return nil
	}
	return a.connHolder.close()
}

// ----------------------------------------------------------------------------
// ALSClient — the typed AccessLogService/StreamAccessLogs wrapper (ADR-0255).
// ----------------------------------------------------------------------------

// ALSClient wraps a *grpc.ClientConn with the typed
// envoy.service.accesslog.v3.AccessLogServiceClient stub. One *ALSClient per
// gRPC-ALS sink (cluster_name), owned by the GrpcAccessLogSink and Close()d at
// sink close. The AuthClient precedent (ADR-0158); StreamAccessLogs opens the
// client-streaming RPC the sink's writer goroutine drives.
//
// Concurrency: a *ALSClient is safe for concurrent use — the underlying
// *grpc.ClientConn is goroutine-safe per the gRPC library. (Each
// StreamAccessLogs call opens a distinct client stream; an individual
// AccessLogService_StreamAccessLogsClient is NOT itself concurrency-safe and is
// driven by a single writer goroutine per the sink's discipline.)
type ALSClient struct {
	connHolder
	stub accesslogv3.AccessLogServiceClient
}

// NewALSClient dials the named cluster via d.DialContext and wraps the
// resulting *grpc.ClientConn in a typed ALSClient. Unlike NewAuthClient there
// is no per-call timeout param — the ALS streaming RPC is long-lived and bounded
// by the caller's context per the sink's lifecycle.
//
// On dial error, NewALSClient returns (nil, err) — the dial error is passed
// through verbatim (already includes the cluster name via DialContext's error
// wrapping).
func NewALSClient(d *Dialer, clusterName string) (*ALSClient, error) {
	conn, err := dialConn(d, "ALS client", clusterName)
	if err != nil {
		return nil, err
	}
	return &ALSClient{
		connHolder: connHolder{conn: conn},
		stub:       accesslogv3.NewAccessLogServiceClient(conn),
	}, nil
}

// StreamAccessLogs opens the client-streaming AccessLogService/StreamAccessLogs
// RPC. The returned stream's lifetime is bounded by ctx; the sink's writer
// goroutine drives Send + CloseAndRecv.
func (a *ALSClient) StreamAccessLogs(ctx context.Context) (accesslogv3.AccessLogService_StreamAccessLogsClient, error) {
	if a == nil || a.stub == nil {
		return nil, errors.New("grpcclient: StreamAccessLogs: nil ALSClient / stub")
	}
	return a.stub.StreamAccessLogs(ctx)
}

// Close releases the underlying *grpc.ClientConn. Idempotent — repeated (or
// concurrent) calls return the cached error from the first call via the shared
// connHolder sync.Once guard (the AuthClient precedent). A nil receiver is
// tolerated (returns nil).
func (a *ALSClient) Close() error {
	if a == nil {
		return nil
	}
	return a.connHolder.close()
}

// ----------------------------------------------------------------------------
// SDSClient — the typed SecretDiscoveryService/StreamSecrets BIDI wrapper (ADR-0278).
// ----------------------------------------------------------------------------

// SDSClient wraps a *grpc.ClientConn with the typed
// secretv3.SecretDiscoveryServiceClient stub. One *SDSClient per SDS secret
// config (cluster_name), owned by the internal/xds SecretProvider and Close()d at
// shutdown. The ALSClient precedent (ADR-0158) but BIDI — StreamSecrets is a
// bidirectional stream (Send *DiscoveryRequest / Recv *DiscoveryResponse), unlike
// ALS's client-streaming.
//
// Concurrency: a *SDSClient is safe for concurrent use — the underlying
// *grpc.ClientConn is goroutine-safe. Each StreamSecrets call opens a distinct
// bidi stream; an individual stream is NOT itself concurrency-safe and is driven
// by a single caller.
type SDSClient struct {
	connHolder
	stub secretv3.SecretDiscoveryServiceClient
}

// NewSDSClient dials the named cluster via d.DialContext and wraps the resulting
// *grpc.ClientConn in a typed SDSClient. On dial error returns (nil, err) verbatim
// (already cluster-named via DialContext's wrapping). The NewALSClient shape.
func NewSDSClient(d *Dialer, clusterName string) (*SDSClient, error) {
	conn, err := dialConn(d, "SDS client", clusterName)
	if err != nil {
		return nil, err
	}
	return &SDSClient{
		connHolder: connHolder{conn: conn},
		stub:       secretv3.NewSecretDiscoveryServiceClient(conn),
	}, nil
}

// StreamSecrets opens the bidirectional StreamSecrets RPC. The caller (the SDS
// provider) drives Send(*DiscoveryRequest) + Recv(*DiscoveryResponse); ctx bounds
// the stream lifetime (the provider applies initial_fetch_timeout via ctx).
func (c *SDSClient) StreamSecrets(ctx context.Context) (secretv3.SecretDiscoveryService_StreamSecretsClient, error) {
	if c == nil || c.stub == nil {
		return nil, errors.New("grpcclient: StreamSecrets: nil SDSClient / stub")
	}
	return c.stub.StreamSecrets(ctx)
}

// Close releases the underlying *grpc.ClientConn. Idempotent (shared connHolder
// sync.Once), the ALSClient.Close shape. A nil receiver is tolerated (returns nil).
func (c *SDSClient) Close() error {
	if c == nil {
		return nil
	}
	return c.connHolder.close()
}

// ----------------------------------------------------------------------------
// MetricsServiceClient — the typed MetricsService/StreamMetrics CLIENT-STREAMING
// wrapper (ADR-0262). StreamMetrics is client-streaming exactly like ALS
// StreamAccessLogs (Send many -> CloseAndRecv one), so this mirrors ALSClient
// (NOT the unary OTLP wrappers). One *MetricsServiceClient per metrics_service
// sink (cluster_name), owned by the MetricsServiceSink and Close()d at sink close.
// ----------------------------------------------------------------------------

// MetricsServiceClient wraps a *grpc.ClientConn with the typed
// metricsv3.MetricsServiceClient stub. The ALSClient precedent (ADR-0158) —
// CLIENT-streaming (StreamMetrics: Send many -> CloseAndRecv one), NOT the unary
// OTLP wrappers.
//
// Concurrency: a *MetricsServiceClient is safe for concurrent use — the
// underlying *grpc.ClientConn is goroutine-safe per the gRPC library. (Each
// StreamMetrics call opens a distinct client stream; an individual
// MetricsService_StreamMetricsClient is NOT itself concurrency-safe and is
// driven by a single writer goroutine per the sink's discipline.)
type MetricsServiceClient struct {
	connHolder
	stub metricsv3.MetricsServiceClient
}

// NewMetricsServiceClient dials the named cluster via d.DialContext and wraps the
// resulting *grpc.ClientConn in a typed MetricsServiceClient. On dial error
// returns (nil, err) verbatim (already cluster-named via DialContext). The
// NewALSClient shape.
func NewMetricsServiceClient(d *Dialer, clusterName string) (*MetricsServiceClient, error) {
	conn, err := dialConn(d, "metrics service client", clusterName)
	if err != nil {
		return nil, err
	}
	return &MetricsServiceClient{
		connHolder: connHolder{conn: conn},
		stub:       metricsv3.NewMetricsServiceClient(conn),
	}, nil
}

// StreamMetrics opens the client-streaming StreamMetrics RPC. The sink's writer
// goroutine bounds ctx; the returned stream is Send()-many then CloseAndRecv()-once.
func (c *MetricsServiceClient) StreamMetrics(ctx context.Context) (metricsv3.MetricsService_StreamMetricsClient, error) {
	if c == nil || c.stub == nil {
		return nil, errors.New("grpcclient: StreamMetrics: nil MetricsServiceClient / stub")
	}
	return c.stub.StreamMetrics(ctx)
}

// Close releases the underlying *grpc.ClientConn. Idempotent (shared connHolder
// sync.Once), the ALSClient.Close shape. A nil receiver is tolerated (returns nil).
func (c *MetricsServiceClient) Close() error {
	if c == nil {
		return nil
	}
	return c.connHolder.close()
}

// ----------------------------------------------------------------------------
// OTLPLogsClient — the typed LogsService/Export UNARY wrapper (ADR-0258).
// ----------------------------------------------------------------------------

// OTLPLogsClient wraps a *grpc.ClientConn with the typed
// go.opentelemetry.io/proto/otlp collector LogsServiceClient stub. One
// *OTLPLogsClient per OTLP access-log sink (cluster_name), owned by the
// OTLPAccessLogSink and Close()d at sink close. The ALSClient precedent
// (ADR-0255) but UNARY — Export is a plain unary RPC (no stream lifecycle).
type OTLPLogsClient struct {
	connHolder
	stub collogspb.LogsServiceClient
}

// NewOTLPLogsClient dials the named cluster via d.DialContext and wraps the
// resulting *grpc.ClientConn in a typed OTLPLogsClient. On dial error returns
// (nil, err) verbatim (already cluster-named via DialContext's wrapping).
func NewOTLPLogsClient(d *Dialer, clusterName string) (*OTLPLogsClient, error) {
	conn, err := dialConn(d, "OTLP logs client", clusterName)
	if err != nil {
		return nil, err
	}
	return &OTLPLogsClient{
		connHolder: connHolder{conn: conn},
		stub:       collogspb.NewLogsServiceClient(conn),
	}, nil
}

// Export sends one ExportLogsServiceRequest over the unary LogsService/Export
// RPC. The sink's writer goroutine bounds ctx; on error the sink retries once.
func (c *OTLPLogsClient) Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
	if c == nil || c.stub == nil {
		return nil, errors.New("grpcclient: Export: nil OTLPLogsClient / stub")
	}
	return c.stub.Export(ctx, req)
}

// Close releases the underlying *grpc.ClientConn. Idempotent (shared connHolder
// sync.Once). A nil receiver is tolerated (returns nil).
func (c *OTLPLogsClient) Close() error {
	if c == nil {
		return nil
	}
	return c.connHolder.close()
}

// ----------------------------------------------------------------------------
// OTLPTracesClient — the typed TraceService/Export UNARY wrapper (ADR-0260).
// ----------------------------------------------------------------------------

// OTLPTracesClient wraps a *grpc.ClientConn with the typed
// go.opentelemetry.io/proto/otlp collector TraceServiceClient stub. One
// *OTLPTracesClient per OTLP tracing sink (cluster_name), owned by the
// OTLPExporter and Close()d at sink close. The OTLPLogsClient precedent
// (ADR-0258) but for traces — Export is a plain unary RPC (no stream
// lifecycle).
type OTLPTracesClient struct {
	connHolder
	stub coltracepb.TraceServiceClient
}

// NewOTLPTracesClient dials the named cluster via d.DialContext and wraps the
// resulting *grpc.ClientConn in a typed OTLPTracesClient. On dial error returns
// (nil, err) verbatim (already cluster-named via DialContext's wrapping).
func NewOTLPTracesClient(d *Dialer, clusterName string) (*OTLPTracesClient, error) {
	conn, err := dialConn(d, "OTLP traces client", clusterName)
	if err != nil {
		return nil, err
	}
	return &OTLPTracesClient{
		connHolder: connHolder{conn: conn},
		stub:       coltracepb.NewTraceServiceClient(conn),
	}, nil
}

// Export sends one ExportTraceServiceRequest over the unary TraceService/Export
// RPC. The exporter's writer goroutine bounds ctx; on error the exporter retries
// once.
func (c *OTLPTracesClient) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	if c == nil || c.stub == nil {
		return nil, errors.New("grpcclient: Export: nil OTLPTracesClient / stub")
	}
	return c.stub.Export(ctx, req)
}

// Close releases the underlying *grpc.ClientConn. Idempotent (shared connHolder
// sync.Once). A nil receiver is tolerated (returns nil).
func (c *OTLPTracesClient) Close() error {
	if c == nil {
		return nil
	}
	return c.connHolder.close()
}
