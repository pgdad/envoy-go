package grpcclient

// processor_client.go — `*ProcessorClient` bidi-stream wrapper per ADR-0169
// landed at Task 4 of phase 19.1 IMPL. NEW file alongside the existing
// `grpcclient.go` per planner-time decision D11. Co-located in the same
// package so the bidi-stream wrapper EXTENDS the existing `*Dialer` (NO
// `Dialer` API changes per ADR-0158 §Consequences — the cross-phase-reuse
// shape was anchored at phase-18.2 introduction time).
//
// Design notes (per ADR-0169 §Decision):
//
//   - Composition over `*Dialer`: `NewProcessorClient` calls
//     `d.DialContext(context.Background(), clusterName)` mirroring
//     `NewAuthClient` (per ADR-0158 §Decision (iv)). The dial-time
//     PARSE-REJECT chain (unknown cluster + `UseH2()==false`) is INHERITED
//     unchanged — no new error paths at this wrapper.
//
//   - Bidi-stream stub: the typed `extprocv3.ExternalProcessorClient` stub
//     from `go-control-plane v1.32.4` is constructed via
//     `extprocv3.NewExternalProcessorClient(conn)` on the dialed
//     `*grpc.ClientConn`. NO codegen; the bindings ship in
//     `go-control-plane v1.32.4`.
//
//   - Per-message timeout discipline (load-bearing per ADR-0169 §Decision):
//     the `perMessageTimeout` argument is STORED on `c.perMessageTimeout`
//     for the FILTER (`extproc/processor.go.dispatchStage`, lands at
//     Task 7) to read — `Process()` itself does NOT apply any
//     `context.WithTimeout` around stream operations. The filter wraps
//     each `Recv` call with `context.WithTimeout(streamCtx,
//     pc.PerMessageTimeout())` (deferred cancel) per ADR-0169 §Decision.
//     Rationale: bidi-stream Send/Recv have asymmetric blocking semantics
//     (Send is non-blocking on the gRPC client stream; Recv blocks until
//     the processor responds), so a wrapper-level per-CALL timeout would
//     not capture the right granularity. The filter is the right layer to
//     own the per-MESSAGE timer because (a) the timer must reset between
//     `override_message_timeout` ProcessingResponse arrivals (per parent
//     §5.P10), and (b) the filter owns the per-stream cancellation hook
//     via `OnDestroy`.
//
//   - One `*grpc.ClientConn` per (cluster_name, `*compiledConfig`) pair —
//     allocated at config-load time (in `buildGRPCProcessorClient`, lands
//     at Task 11); shared across all per-stream `Process()` invocations on
//     that compiledConfig. Mirrors phase-18.2 ADR-0158 §Decision (v) for
//     `*AuthClient`.
//
//   - Leaks-on-exit MVP per ADR-0158 §Decision (vi) + ADR-0169 §Decision:
//     production callers NEVER invoke `Close()` — the `*grpc.ClientConn`
//     is leaked-on-exit; the OS reclaims it. `Close()` is authored for
//     unit tests + future hot-reload phases (sync.Once-guarded idempotency).

import (
	"context"
	"fmt"
	"sync"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	"google.golang.org/grpc"
)

// ProcessStream is the bidi-stream surface returned by
// `(*ProcessorClient).Process`. The interface is intentionally a 3-method
// minimum that the filter's `dispatchStage` needs — Send, Recv, CloseSend.
// (The underlying `grpc.ClientStream` methods like `Header`/`Trailer`/
// `Context` are NOT exposed because the filter does not need them; if a
// future consumer needs them the interface can be extended without
// breaking existing callers.)
type ProcessStream interface {
	Send(*extprocv3.ProcessingRequest) error
	Recv() (*extprocv3.ProcessingResponse, error)
	CloseSend() error
}

// ProcessorClient wraps a `*grpc.ClientConn` with the typed
// `envoy.service.ext_proc.v3.ExternalProcessorClient` stub from
// `go-control-plane v1.32.4`. One `*ProcessorClient` per (cluster_name,
// `*compiledConfig`) pair — allocated at config-load time (in
// `buildGRPCProcessorClient`, lands at Task 11); shared across all
// per-stream `Process()` invocations on that compiledConfig.
//
// Concurrency: a `*ProcessorClient` is safe for concurrent use — the
// underlying `*grpc.ClientConn` is goroutine-safe per the gRPC library and
// manages its own transport-level reconnect via the sub-channel state
// machine. Concurrent `Process` calls multiplex over the underlying HTTP/2
// transport per the gRPC concurrency model.
type ProcessorClient struct {
	conn   *grpc.ClientConn
	stub   extprocv3.ExternalProcessorClient
	target string // cluster_name — for logs/errors

	// perMessageTimeout is the per-MESSAGE timeout captured at
	// `NewProcessorClient` time. It is STORED here for the filter's
	// `dispatchStage` to read (via `PerMessageTimeout()`); `Process()`
	// itself does NOT apply any timeout around stream operations.
	// See file-level design notes above + ADR-0169 §Decision.
	perMessageTimeout time.Duration

	// closeOnce + closeErr guard the idempotent Close per SPEC §3.1.
	closeOnce sync.Once
	closeErr  error
}

// NewProcessorClient dials the named cluster via `d.DialContext` and wraps
// the resulting `*grpc.ClientConn` in a typed `ProcessorClient`. The
// `perMessageTimeout` is the per-MESSAGE timeout — captured here for the
// filter's `dispatchStage` to read; NO timeout is applied inside `Process`.
//
// PARSE-REJECTs (via error return):
//   - `d == nil` → `"grpcclient: new processor client %q: dialer is nil"`.
//   - INHERITED from `d.DialContext`: unknown cluster + `UseH2()==false`.
//     The dial error is passed through verbatim (already includes the
//     cluster name via `DialContext`'s error wrapping).
//
// Mirrors `NewAuthClient`'s shape (per ADR-0158 §Decision (iv) + ADR-0169
// §Decision (i)) — the only structural difference is the typed stub
// (`extprocv3.NewExternalProcessorClient` vs `authv3.NewAuthorizationClient`)
// and the timeout semantics (per-MESSAGE here vs per-CALL there).
func NewProcessorClient(d *Dialer, clusterName string, perMessageTimeout time.Duration) (*ProcessorClient, error) {
	if d == nil {
		return nil, fmt.Errorf("grpcclient: new processor client %q: dialer is nil", clusterName)
	}
	conn, err := d.DialContext(context.Background(), clusterName)
	if err != nil {
		return nil, err
	}
	return &ProcessorClient{
		conn:              conn,
		stub:              extprocv3.NewExternalProcessorClient(conn),
		target:            clusterName,
		perMessageTimeout: perMessageTimeout,
	}, nil
}

// PerMessageTimeout returns the per-message timeout captured at construction
// time. The filter's `dispatchStage` (Task 7) reads this value and wraps each
// `Recv` with `context.WithTimeout` per ADR-0169 §Decision. A zero return
// means "no per-message timeout" — the caller's `ctx` alone bounds the call.
//
// A nil receiver returns 0 (mirrors the nil-tolerance discipline of `Close`).
func (c *ProcessorClient) PerMessageTimeout() time.Duration {
	if c == nil {
		return 0
	}
	return c.perMessageTimeout
}

// Process opens a bidi-stream `Process` RPC on the wrapped
// `*grpc.ClientConn`. The caller's `ctx` is threaded into
// `c.stub.Process(ctx)` so cancellation (from `OnDestroy` via the per-stream
// cancel hook) aborts in-flight `Send`/`Recv` calls promptly.
//
// `Process` does NOT apply `c.perMessageTimeout` — see file-level design
// notes + ADR-0169 §Decision. The filter's `dispatchStage` (Task 7) wraps
// each `Recv` with `context.WithTimeout(ctx, c.PerMessageTimeout())`.
//
// On error (server unreachable / dial failure / closed conn) returns
// `(nil, err)` with the underlying gRPC error verbatim — `Process` does NOT
// classify or map transport errors; the filter is responsible for
// `dispError`-style classification per the SPEC §6.5 error-classification
// boundary.
func (c *ProcessorClient) Process(ctx context.Context) (ProcessStream, error) {
	if c == nil || c.stub == nil {
		return nil, fmt.Errorf("grpcclient: Process: nil ProcessorClient / stub")
	}
	stream, err := c.stub.Process(ctx)
	if err != nil {
		return nil, err
	}
	return stream, nil
}

// Close releases the underlying `*grpc.ClientConn`. Idempotent — repeated
// calls (or concurrent calls) return the cached error from the first call
// via an internal `sync.Once` guard.
//
// MVP discipline (per ADR-0158 §Decision (vi) + ADR-0169 §Decision): in
// production the filter NEVER calls `Close()` — the `*grpc.ClientConn` is
// leaked-on-exit; the OS reclaims it. `Close()` exists primarily for the
// Group 3 unit tests and for future hot-reload phases.
//
// A nil receiver is tolerated (returns nil) — mirrors `AuthClient.Close`.
func (c *ProcessorClient) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		if c.conn != nil {
			c.closeErr = c.conn.Close()
		}
	})
	return c.closeErr
}
