// Package abi — stream_control.go: stream-control hostcall dispatch shims
// per 25.2 SPEC §5.1 #28-29 (proxy_continue_stream + proxy_close_stream).
//
// The stream-control hostcalls pair with the PAUSE-buffer dispatch on body
// callbacks per BRAINSTORM Q1: when a body callback returns ProxyAction::
// Pause, the host buffers the body in the per-stream accumulation; the
// guest later calls proxy_continue_stream(stream_type) to resume the
// downstream/upstream filter chain (or proxy_close_stream to abort).
//
// # Two shims landed at Task 4
//
//   - ContinueStreamShim — proxy_continue_stream(stream_type) -> WasmResult
//   - CloseStreamShim    — proxy_close_stream(stream_type)    -> WasmResult
//
// # Stream-type discriminator (per proxy-wasm v0.2.1 spec README
// §proxy_stream_type_t)
//
//   - 0 = HttpRequest (downstream → filter chain)
//   - 1 = HttpResponse (filter chain → downstream)
//   - 2 = HttpUpstream (upstream connection state)
//
// envoy-go does NOT validate the discriminator at the shim layer — the
// consumer-side ContinueStream / CloseStream impl at Task 16 decides what
// to do with unknown values (typical: no-op + emit a WARN log). This
// matches cpp-host disposition (the cpp-host shim just forwards the int).
//
// # Result-code disposition
//
// The consumer-side ContinueStream / CloseStream return WasmResult
// directly; the shim propagates the result unchanged. Typical impls
// return WasmResultOk on successful continuation and
// WasmResultInternalFailure on a tear-down error (e.g., filter chain
// already in terminal state).
package abi

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// streamHost is the package-private interface the stream-control shims
// dispatch through. Satisfied by `*wasm.RootVM` (via methods added at
// Task 4 — see internal/wasm/host_bridge_25_2.go). Non-satisfaction →
// WasmResultInternalFailure (programmer error — wrong host wired in).
type streamHost interface {
	// CurrentCtxID returns the currently-active stream context id for the
	// in-flight hostcall dispatch.
	CurrentCtxID() uint32

	// StreamContinue resumes the named stream (request/response/upstream
	// per the streamType discriminator) after a prior Pause return.
	// Returns the wire-result directly per the proxy-wasm v0.2.1
	// proxy_continue_stream return shape.
	StreamContinue(ctx context.Context, streamCtxID uint32, streamType uint32) WasmResult

	// StreamClose tears down the named stream early. Returns the
	// wire-result directly.
	StreamClose(ctx context.Context, streamCtxID uint32, streamType uint32) WasmResult
}

// ContinueStreamShim — proxy_continue_stream dispatch per §5.1 #28.
//
// Wire-shape: `Word continue_stream(Word stream_type)` per proxy-wasm
// v0.2.1 spec README §proxy_continue_stream.
//
// Forwards stream_type to the consumer-side StreamContinue impl;
// propagates the result code unchanged. The `mod api.Module` argument is
// unused (no memory access on this hostcall) but kept on the signature to
// match the registration.go envelope shape.
func ContinueStreamShim(ctx context.Context, _ api.Module, host Host25_2, streamType uint32) WasmResult {
	sh, ok := host.(streamHost)
	if !ok {
		return WasmResultInternalFailure
	}
	return sh.StreamContinue(ctx, sh.CurrentCtxID(), streamType)
}

// CloseStreamShim — proxy_close_stream dispatch per §5.1 #29.
//
// Wire-shape: `Word close_stream(Word stream_type)` per proxy-wasm
// v0.2.1 spec README §proxy_close_stream.
//
// Forwards stream_type to the consumer-side StreamClose impl; propagates
// the result code unchanged.
func CloseStreamShim(ctx context.Context, _ api.Module, host Host25_2, streamType uint32) WasmResult {
	sh, ok := host.(streamHost)
	if !ok {
		return WasmResultInternalFailure
	}
	return sh.StreamClose(ctx, sh.CurrentCtxID(), streamType)
}
