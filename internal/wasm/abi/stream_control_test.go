// internal/wasm/abi/stream_control_test.go — stream-control dispatch tests
// per 25.2 SPEC §5.1 #28-29 (proxy_continue_stream + proxy_close_stream).
//
// Both shims delegate to consumer-side ABICallbacks methods (StreamContinue
// / StreamClose) — paired with the PAUSE-buffer dispatch on body callbacks
// per BRAINSTORM Q1. Stream-type discriminator: HttpRequest=0, HttpResponse=1,
// HttpUpstream=2 (per proxy-wasm v0.2.1 spec README §proxy_stream_type_t).

package abi

import (
	"context"
	"testing"
)

func TestStreamControl_ContinueStream_RoundTrip(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	mod := newHostingModule(t, ctx)

	got := ContinueStreamShim(ctx, mod, host, 1 /* HttpResponse */)
	if got != WasmResultOk {
		t.Fatalf("ContinueStreamShim = %v, want Ok", got)
	}
	if host.continueLastStreamCtx != host.ctxID {
		t.Errorf("Continue stream-ctx = %d, want %d", host.continueLastStreamCtx, host.ctxID)
	}
	if host.continueLastType != 1 {
		t.Errorf("Continue type = %d, want 1", host.continueLastType)
	}
}

func TestStreamControl_CloseStream_RoundTrip(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	mod := newHostingModule(t, ctx)

	got := CloseStreamShim(ctx, mod, host, 2 /* HttpUpstream */)
	if got != WasmResultOk {
		t.Fatalf("CloseStreamShim = %v, want Ok", got)
	}
	if host.closeLastStreamCtx != host.ctxID {
		t.Errorf("Close stream-ctx = %d, want %d", host.closeLastStreamCtx, host.ctxID)
	}
	if host.closeLastType != 2 {
		t.Errorf("Close type = %d, want 2", host.closeLastType)
	}
}

func TestStreamControl_ContinueStream_CallbackResultPropagates(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	host.continueResult = WasmResultInternalFailure
	mod := newHostingModule(t, ctx)

	got := ContinueStreamShim(ctx, mod, host, 0)
	if got != WasmResultInternalFailure {
		t.Fatalf("ContinueStreamShim(cb-result) = %v, want InternalFailure (propagated)", got)
	}
}

func TestStreamControl_CloseStream_CallbackResultPropagates(t *testing.T) {
	ctx := context.Background()
	host := newFakeBufferHost()
	host.closeResult = WasmResultInternalFailure
	mod := newHostingModule(t, ctx)

	got := CloseStreamShim(ctx, mod, host, 0)
	if got != WasmResultInternalFailure {
		t.Fatalf("CloseStreamShim(cb-result) = %v, want InternalFailure (propagated)", got)
	}
}

func TestStreamControl_ContinueStream_NonHostHostValue(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	got := ContinueStreamShim(ctx, mod, "not-a-host", 0)
	if got != WasmResultInternalFailure {
		t.Fatalf("non-host host value = %v, want WasmResultInternalFailure", got)
	}
}

func TestStreamControl_CloseStream_NonHostHostValue(t *testing.T) {
	ctx := context.Background()
	mod := newHostingModule(t, ctx)
	got := CloseStreamShim(ctx, mod, "not-a-host", 0)
	if got != WasmResultInternalFailure {
		t.Fatalf("non-host host value = %v, want WasmResultInternalFailure", got)
	}
}
