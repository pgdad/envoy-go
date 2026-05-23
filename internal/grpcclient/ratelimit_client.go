package grpcclient

// ratelimit_client.go — DELTA-1 RateLimitClient per phase-24.1 PLAN §Task-2
// + parent SPEC §3.1 + ADR-0197. The THIRD ADR-0158 two-tier typed wrapper.
//
// `ShouldRateLimit` is a UNARY RPC (request/response), so this wrapper
// clones the AuthClient shape (`grpcclient.go` AuthClient struct +
// NewAuthClient + Check + Close) verbatim — NOT the bidi ProcessorClient
// shape. The per-stream OnDestroy cancellation lives at the FILTER layer
// (Task 7), NOT in the wrapper.
//
// Design notes (mirror the AuthClient §Design notes; reproduced for the
// reader who lands on this file first):
//
//   - Per-call timeout discipline (mirrors AuthClient D7 + D9): the
//     `timeout` captured at `NewRateLimitClient` construction is applied
//     INSIDE `ShouldRateLimit` via `context.WithTimeout(ctx, timeout)`
//     (deferred cancel). The caller's `ctx` is the parent — its
//     cancellation propagates through `context.WithTimeout`'s
//     AND-of-cancellation semantics. Transport errors (gRPC `Unavailable`
//     / `DeadlineExceeded` / `Canceled`; `context.Canceled`;
//     `context.DeadlineExceeded`) propagate VERBATIM to the caller. A zero
//     `timeout` means "no per-call timeout" — the caller's `ctx` alone
//     bounds the call.
//
//   - `Close` idempotency (per SPEC §3.1): the body is guarded by
//     `sync.Once` so repeated calls return the cached error from the first
//     call. The underlying `*grpc.ClientConn.Close()` is itself idempotent
//     per the gRPC library, but the sync.Once guard normalizes the error
//     return + is concurrent-safe.
//
//   - Connection lifecycle (per ADR-0158 D2 — leaks-on-exit MVP): the
//     `*grpc.ClientConn` is owned by the caller; the filter layer captures
//     it in the descriptor-dispatch closure and never explicitly calls
//     `Close` in production. On process exit, the OS reclaims the
//     connection. Future hot-reload phases will introduce a
//     close-on-replacement discipline.

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"
	"google.golang.org/grpc"
)

// ----------------------------------------------------------------------------
// RateLimitClient — the typed `RateLimitService/ShouldRateLimit` wrapper.
// ----------------------------------------------------------------------------

// RateLimitClient wraps a `*grpc.ClientConn` with the typed
// `envoy.service.ratelimit.v3.RateLimitServiceClient` stub from
// `go-control-plane v1.32.4`. One `*RateLimitClient` per
// (cluster_name, `*compiledConfig`) pair — allocated at config-load time
// (in the filter's `New` boot path; Task 7); shared across all per-stream
// `ShouldRateLimit()` calls on that compiledConfig.
//
// Concurrency: a `*RateLimitClient` is safe for concurrent use — the
// underlying `*grpc.ClientConn` is goroutine-safe per the gRPC library and
// manages its own transport-level reconnect via the sub-channel state
// machine.
type RateLimitClient struct {
	conn   *grpc.ClientConn
	stub   ratelimitv3.RateLimitServiceClient
	target string // cluster_name — for logs/errors

	timeout time.Duration // per-call timeout (applied via context.WithTimeout in ShouldRateLimit)

	// closeOnce + closeErr guard the idempotent Close per SPEC §3.1.
	closeOnce sync.Once
	closeErr  error
}

// NewRateLimitClient dials the named cluster via `d.DialContext` and wraps
// the resulting `*grpc.ClientConn` in a typed `RateLimitClient`. `timeout`
// is the per-call timeout — applied inside `ShouldRateLimit` via
// `context.WithTimeout`; a zero `timeout` means no per-call timeout (the
// caller's `ctx` alone bounds the call).
//
// On dial error, `NewRateLimitClient` returns `(nil, err)` — the dial error
// is passed through verbatim (already includes the cluster name via
// `DialContext`'s error wrapping).
//
// No `Dialer` API change: reuses the existing `(*Dialer).DialContext`
// surface that `NewAuthClient` consumes.
func NewRateLimitClient(d *Dialer, clusterName string, timeout time.Duration) (*RateLimitClient, error) {
	if d == nil {
		return nil, fmt.Errorf("grpcclient: new ratelimit client %q: dialer is nil", clusterName)
	}
	conn, err := d.DialContext(context.Background(), clusterName)
	if err != nil {
		return nil, err
	}
	return &RateLimitClient{
		conn:    conn,
		stub:    ratelimitv3.NewRateLimitServiceClient(conn),
		target:  clusterName,
		timeout: timeout,
	}, nil
}

// ShouldRateLimit invokes `RateLimitService/ShouldRateLimit` on the wrapped
// `*grpc.ClientConn`.
//
// Per-call timeout discipline (mirrors AuthClient D7/D9): when
// `r.timeout > 0` the call is bounded by `context.WithTimeout(ctx, r.timeout)`
// (deferred cancel). The caller's `ctx` is the parent — `OnDestroy`-driven
// cancellation (wired at the filter layer per Task 7) propagates through
// `context.WithTimeout`'s AND-of-cancellation semantics. BOTH paths
// (caller-cancel AND per-call timeout) surface as transport errors via the
// `error` return.
//
// Transport-level errors (gRPC `Unavailable` / `DeadlineExceeded` /
// `Canceled`; `context.Canceled`; `context.DeadlineExceeded`) propagate
// VERBATIM to the caller — `ShouldRateLimit` does NOT classify or map them.
// The filter-layer caller (Task 7's dispositions module) maps transport
// errors to its error disposition BEFORE inspecting the
// `*RateLimitResponse`.
//
// On success, `ShouldRateLimit` returns `(*RateLimitResponse, nil)` — the
// caller inspects `resp.GetOverallCode()` + per-descriptor statuses.
func (r *RateLimitClient) ShouldRateLimit(ctx context.Context, req *ratelimitv3.RateLimitRequest) (*ratelimitv3.RateLimitResponse, error) {
	if r == nil || r.stub == nil {
		return nil, errors.New("grpcclient: ShouldRateLimit: nil RateLimitClient / stub")
	}
	if r.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.timeout)
		defer cancel()
	}
	return r.stub.ShouldRateLimit(ctx, req)
}

// Close releases the underlying `*grpc.ClientConn`. Idempotent — repeated
// calls (or concurrent calls) return the cached error from the first call
// via an internal `sync.Once` guard.
//
// MVP discipline (per ADR-0158 D2): in production the filter never calls
// `Close()` — the `*grpc.ClientConn` is leaked-on-exit; the OS reclaims
// it. `Close()` exists primarily for unit tests and for future hot-reload
// phases.
func (r *RateLimitClient) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		if r.conn != nil {
			r.closeErr = r.conn.Close()
		}
	})
	return r.closeErr
}
