package wasm

// encodecap_park_test.go — phase-83 S8: the host-initiated parks.
//
// # What was broken
//
// body.go and trailers.go returned DataStopIterationNoBuffer /
// TrailersStopIteration from arms where the host had ALREADY decided the
// stream cannot be served and had NO resume to offer. Those statuses PARK.
// On the encode side a park is unbounded:
//
//   - all SIX of chain.go's in-iterator c.localReplyDone.Load() re-checks sit
//     in RunDecodeHeaders / RunDecodeData / RunDecodeTrailers, and none of the
//     four parkEncode sites is guarded, so setting the flag would not rescue
//     it either;
//   - no watchdog is armed, and arming the existing one would be WORSE than
//     the bug: beginEncodePause's fire path calls cb.ContinueEncoding(), which
//     unparks the chain and continues iteration — delivering the full over-cap
//     body after a stall;
//   - there is no stream idle timeout anywhere in this tree, and the ctx handed
//     to parkEncode is never canceled on downstream disconnect.
//
// Seven such arms exist. This file anchors them; each has its own subtest, and
// each subtest asserts the RETURNED ERROR rather than merely that the call
// returned, because the DataContinue variant returns (true, nil) and would
// pass a "did it return?" gate while silently disabling the cap.
//
//	(a) EncodeData, cap-exceeded            TestFilterChain_EncodeCap_TerminatesStream/cap
//	(b) EncodeData, sticky (2nd chunk)      .../encode_sticky
//	(c) DecodeData, sticky (2nd chunk)      .../decode_sticky
//	(d) DecodeData, cap + nil decoderCb     .../decode_cap_nil_cb
//	(e) EncodeData, captured local response TestFilterChain_EncodeLocalResponse_TerminatesStream/body
//	(f) EncodeTrailers, captured local resp .../trailers
//	(g) EncodeHeaders, captured local resp  .../headers  <- the LIVE one
//
// (a)/(b)/(e)/(g) are the live encode-side leaks — (g) most of all, since
// RunEncodeHeaders is driven by all three HCM dispatchers while
// RunEncodeTrailers has no production caller at this tip. (c) and (d) are LATENT: on the
// decode side beginLocalReply sets localReplyDone synchronously and
// RunDecodeData re-checks it BEFORE the status switch, so the production decode
// path with a real decoderCb is genuinely rescued — (c) needs a chain reentry
// and (d) needs a nil decoderCb, which only a test double produces. They are
// closed anyway because the cost is one word each and the failure mode is a
// held connection.
//
// # The measured reference disposition
//
// Against pinned envoyproxy/envoy:contrib-v1.37.2 (fresh container per arm,
// with a negative control), an encode-side buffer-limit breach against a
// paused encoder filter answers with an IMMEDIATE STREAM RESET: 200 + response
// headers delivered, then reset with ZERO body bytes, 1.007 s, with
// http.<prefix>.rs_too_large: 1 and downstream_rq_tx_reset: 1. The NC (same
// limit, guest Continue) served 65536 bytes in 0.003 s with rs_too_large: 0.
// Upstream NEVER stalls on a cap breach. envoy-go now answers with a
// terminate-the-stream status; the departure (upstream flushes headers first,
// envoy-go's client gets nothing) is recorded on errStreamTerminatedByFilter in
// chain.go and on the arms in body.go.

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/stats"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// p83CapProbeBudget is how long a park anchor waits for a chain iterator to
// return before declaring the arm PARKED.
//
// ⚠️ IT IS THIS PROBE'S DEADLINE, NOT A BOUND ON THE PARK. Every ctx below is
// derived from context.Background() and carries NO deadline — exactly the shape
// the production HCM connection ctx has — so a timeout here means the dispatch
// goroutine would have stayed parked FOREVER. The only thing that reaps it is
// the cancel() registered in t.Cleanup.
const p83CapProbeBudget = 2 * time.Second

// p83CapBytes is the tiny body-buffer cap the anchors configure so a 16-byte
// chunk breaches it on the first call.
const p83CapBytes = 8

// p83CapFilter wires a wasm *filter with a p83CapBytes body cap into a real
// FilterChain. No guest is needed: the cap check is GUEST-INDEPENDENT — the
// measured order inside EncodeData is sticky check -> accumulate (always) ->
// cap fires -> only THEN HasGlobalFunc(proxy_on_response_body) — so a filter
// with streamCtx == nil still trips the cap.
func p83CapFilter(t *testing.T, pluginName string) (*filter, *envoyhttp.FilterChain, *stats.Registry) {
	t.Helper()
	reg := stats.NewRegistry()
	cc := newBodyTestCompiledConfig(t, p83CapBytes, pluginName, reg)

	// ⚠️ A nil *stats.Counter .Inc() is a PROCESS CRASH — no recover() wraps
	// the cap arm. Assert the POINTERS before driving anything through it, or a
	// mis-wired stat scope takes the whole test binary down with a nil-deref
	// panic that reads as an unrelated failure.
	if cc.stats == nil {
		t.Fatalf("compiledConfig.stats is nil; the cap arm would nil-deref")
	}
	if cc.stats.bodyBufferCapExceeded == nil {
		t.Fatalf("stats.bodyBufferCapExceeded is nil; the cap arm's Inc() would CRASH the process")
	}
	if cc.stats.envoyGoFailures == nil {
		t.Fatalf("stats.envoyGoFailures is nil; the §2.25 co-increment would CRASH the process")
	}

	wf := &filter{cfg: cc}
	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "wasm", Decoder: wf, Encoder: wf},
	}, nil)
	t.Cleanup(chain.Destroy)
	return wf, chain, reg
}

// p83DriveChain runs fn on its own goroutine against a NO-DEADLINE ctx and
// waits p83CapProbeBudget for it to return.
//
// Returns (terminated, err, returned). When returned is false the iterator
// PARKED and the caller must not read the other two.
func p83DriveChain(t *testing.T, label string, fn func(context.Context) (bool, error)) (bool, error, bool) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	// Reap the parked goroutine on the way out. Without this, every RED run
	// leaks a blocked dispatch goroutine for the remainder of the package.
	t.Cleanup(cancel)

	type result struct {
		terminated bool
		err        error
	}
	done := make(chan result, 1)
	go func() {
		term, err := fn(ctx)
		done <- result{term, err}
	}()

	select {
	case r := <-done:
		return r.terminated, r.err, true
	case <-time.After(p83CapProbeBudget):
		t.Errorf("MEASURED HAZARD: %s PARKED: did not return within %v. ⚠️ THE %v IS THE PROBE'S DEADLINE, NOT THE PARK — the ctx carries no deadline, nothing on the encode side sets localReplyDone, no watchdog is armed on a host-initiated park, and there is no stream idle timeout in this tree. The park is UNBOUNDED; only the deferred cancel() reaps this goroutine.",
			label, p83CapProbeBudget, p83CapProbeBudget)
		return false, nil, false
	}
}

// -----------------------------------------------------------------------------
// (a) (b) (c) (d) — the body-cap family.
// -----------------------------------------------------------------------------

// TestFilterChain_EncodeCap_TerminatesStream is the S8 regression for the four
// body-cap arms, driven through a REAL FilterChain.
//
// ⚠️ IT MUST BE DRIVEN THROUGH THE CHAIN. Calling f.EncodeData directly returns
// a status and returns immediately; the park is a property of RunEncodeData,
// not of the filter method, so a filter-level test cannot observe it at all.
//
// ⚠️ EVERY CELL ASSERTS THE STICKY FLAG AND THE COUNTERS AS WELL AS THE ERROR.
// A green in which the arm never fired would otherwise read as coverage: the
// cap cells require body_buffer_cap_exceeded == 1, and the STICKY cells require
// it to stay at 0, which is what distinguishes "the sticky short-circuit ran"
// from "the cap arm ran again".
func TestFilterChain_EncodeCap_TerminatesStream(t *testing.T) {
	t.Parallel()

	t.Run("cap", func(t *testing.T) {
		t.Parallel()
		wf, chain, reg := p83CapFilter(t, "p83_encode_cap")

		over := make([]byte, 16) // 16 > p83CapBytes
		terminated, err, returned := p83DriveChain(t, "chain.RunEncodeData", func(ctx context.Context) (bool, error) {
			return chain.RunEncodeData(ctx, over, true)
		})
		if !returned {
			return
		}
		if !errors.Is(err, envoyhttp.ErrStreamTerminatedByFilter) {
			t.Errorf("RunEncodeData err = %v; want ErrStreamTerminatedByFilter. ⚠️ A nil error here is the DataContinue failure mode: the chain would report terminated=true with no body override and HCM would write the over-cap body IN FULL, silently disabling the cap", err)
		}
		if terminated {
			t.Errorf("RunEncodeData terminated = true; want false")
		}
		if !wf.encodeBodyCapExceeded {
			t.Errorf("encodeBodyCapExceeded = false; want true — the cap arm never fired and this green would be vacuous")
		}
		if got := findStatCounterValue(reg, "wasm.p83_encode_cap.body_buffer_cap_exceeded"); got != 1 {
			t.Errorf("body_buffer_cap_exceeded = %d; want 1", got)
		}
		if got := findStatCounterValue(reg, "wasm.p83_encode_cap.envoy_go.failures"); got != 1 {
			t.Errorf("envoy_go.failures = %d; want 1 (§2.25 co-increment)", got)
		}
	})

	t.Run("encode_sticky", func(t *testing.T) {
		t.Parallel()
		wf, chain, reg := p83CapFilter(t, "p83_encode_sticky")

		// The state a PRIOR over-cap chunk leaves behind. Setting it directly
		// is what isolates the sticky short-circuit from the cap arm: the
		// counter assertions below would both read 1 if the cap arm ran.
		wf.encodeBodyCapExceeded = true

		terminated, err, returned := p83DriveChain(t, "chain.RunEncodeData (sticky)", func(ctx context.Context) (bool, error) {
			return chain.RunEncodeData(ctx, []byte("nxt"), false)
		})
		if !returned {
			return
		}
		if !errors.Is(err, envoyhttp.ErrStreamTerminatedByFilter) {
			t.Errorf("RunEncodeData (sticky) err = %v; want ErrStreamTerminatedByFilter", err)
		}
		if terminated {
			t.Errorf("RunEncodeData (sticky) terminated = true; want false")
		}
		if got := findStatCounterValue(reg, "wasm.p83_encode_sticky.body_buffer_cap_exceeded"); got != 0 {
			t.Errorf("body_buffer_cap_exceeded = %d; want 0 — a non-zero value means the CAP arm ran, not the sticky short-circuit, and this cell is testing the wrong arm", got)
		}
		if n := len(wf.encodeBody); n != 0 {
			t.Errorf("encodeBody grew to %d bytes; want 0 — the sticky arm returns BEFORE the accumulator append", n)
		}
	})

	t.Run("decode_sticky", func(t *testing.T) {
		t.Parallel()
		wf, chain, reg := p83CapFilter(t, "p83_decode_sticky")
		wf.decodeBodyCapExceeded = true

		terminated, err, returned := p83DriveChain(t, "chain.RunDecodeData (sticky)", func(ctx context.Context) (bool, error) {
			return chain.RunDecodeData(ctx, []byte("nxt"), false)
		})
		if !returned {
			return
		}
		if !errors.Is(err, envoyhttp.ErrStreamTerminatedByFilter) {
			t.Errorf("RunDecodeData (sticky) err = %v; want ErrStreamTerminatedByFilter", err)
		}
		if terminated {
			t.Errorf("RunDecodeData (sticky) terminated = true; want false")
		}
		if got := findStatCounterValue(reg, "wasm.p83_decode_sticky.body_buffer_cap_exceeded"); got != 0 {
			t.Errorf("body_buffer_cap_exceeded = %d; want 0 — the sticky short-circuit must not re-bump", got)
		}
		if chain.LocalReplyDone() {
			t.Errorf("localReplyDone = true; want false — the sticky arm sends NO local reply, which is exactly why chain.go's pre-switch re-check cannot rescue it")
		}
	})

	t.Run("decode_cap_nil_cb", func(t *testing.T) {
		t.Parallel()
		// The nil-decoderCb shape. NewFilterChain always wires a real
		// decoderCB, so this arm is test-double-reachable ONLY — but it is a
		// genuine unbounded park when reached, because with no SendLocalReply
		// there is no localReplyDone for RunDecodeData's pre-switch re-check to
		// find.
		reg := stats.NewRegistry()
		cc := newBodyTestCompiledConfig(t, p83CapBytes, "p83_decode_cap_nilcb", reg)
		wf := &filter{cfg: cc} // decoderCb deliberately nil
		chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
			{Name: "wasm", Decoder: wf},
		}, nil)
		t.Cleanup(chain.Destroy)
		// NewFilterChain wired a real callback; drop it back to nil to model
		// the shape under test.
		wf.decoderCb = nil

		terminated, err, returned := p83DriveChain(t, "chain.RunDecodeData (nil decoderCb)", func(ctx context.Context) (bool, error) {
			return chain.RunDecodeData(ctx, make([]byte, 16), false)
		})
		if !returned {
			return
		}
		if !errors.Is(err, envoyhttp.ErrStreamTerminatedByFilter) {
			t.Errorf("RunDecodeData (nil decoderCb) err = %v; want ErrStreamTerminatedByFilter", err)
		}
		if terminated {
			t.Errorf("RunDecodeData (nil decoderCb) terminated = true; want false")
		}
		if !wf.decodeBodyCapExceeded {
			t.Errorf("decodeBodyCapExceeded = false; want true — the cap arm never fired")
		}
		if chain.LocalReplyDone() {
			t.Errorf("localReplyDone = true; want false — with a nil decoderCb no 413 is sent, which is the whole reason this arm cannot be rescued by the pre-switch re-check")
		}
	})
}

// -----------------------------------------------------------------------------
// (e) (f) — the captured-local-response family, encode side.
// -----------------------------------------------------------------------------

// buildEncodeLocalResponseProxyWasm constructs a guest that calls
// proxy_send_local_response from BOTH proxy_on_response_body and
// proxy_on_response_trailers, then returns ProxyActionContinue (0) from each.
//
// ⚠️ RETURNING CONTINUE IS THE DISCRIMINATOR, NOT AN OVERSIGHT. Both arms under
// test read f.sentLocalResponse BEFORE the ProxyAction switch. If the capture
// arm did not fire, the switch would take the Continue path and the chain would
// sail through — so a Continue-returning guest makes "the arm ran" and "the arm
// did not run" produce different observable outcomes. A Pause-returning guest
// would park on the S1/S2 arm instead and the cell would pass for the wrong
// reason.
//
// proxy_on_request_headers must be exported and must Continue: the per-stream
// StreamContext is built ONLY in DecodeHeaders, and both body.go and
// trailers.go short-circuit on a nil f.streamCtx.
func buildEncodeLocalResponseProxyWasm() []byte {
	sendLocalResponse := func() []byte {
		return fixFuncBody(
			// proxy_send_local_response(403, 0, 0, 0, 0, 0, 0, -1)
			fixI32Const(403), // status_code
			fixI32Const(0),   // status_msg_ptr
			fixI32Const(0),   // status_msg_size
			fixI32Const(0),   // body_ptr
			fixI32Const(0),   // body_size
			fixI32Const(0),   // additional_headers_ptr
			fixI32Const(0),   // additional_headers_size
			fixI32Const(-1),  // grpc_status
			fixCall(0),       // import 0 = proxy_send_local_response
			fixDrop(),        // discard the WasmResult
			fixI32Const(0),   // return ProxyActionContinue
		)
	}
	return fixBuildModule(
		fixTypeSection(
			[2][]byte{nil, nil}, // type 0: () -> ()
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}}, // type 1: (i32,i32,i32) -> i32
			fixTypeTrailersCallback(),                                     // type 2: (i32,i32) -> i32
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}}, // type 3
		),
		fixImportSection([]fixImport{
			{module: "env", name: "proxy_send_local_response", kind: fixExtFunction, idx: 3},
		}),
		// Function space after 1 import (idx 0):
		//   fn 1: _initialize                 (type 0)
		//   fn 2: proxy_abi_version_0_2_1     (type 0)
		//   fn 3: proxy_on_request_headers    (type 1) -> Continue
		//   fn 4: proxy_on_response_headers   (type 1) -> Continue
		//   fn 5: proxy_on_response_body      (type 1) -> send_local_response + Continue
		//   fn 6: proxy_on_response_trailers  (type 2) -> send_local_response + Continue
		fixFunctionSection([]uint32{0, 0, 1, 1, 1, 2}),
		fixMemorySection(1),
		fixExportSection([]fixExport{
			{name: "_initialize", kind: fixExtFunction, idx: 1},
			{name: "proxy_abi_version_0_2_1", kind: fixExtFunction, idx: 2},
			{name: "proxy_on_request_headers", kind: fixExtFunction, idx: 3},
			{name: "proxy_on_response_headers", kind: fixExtFunction, idx: 4},
			{name: "proxy_on_response_body", kind: fixExtFunction, idx: 5},
			{name: "proxy_on_response_trailers", kind: fixExtFunction, idx: 6},
			{name: "memory", kind: fixExtMemory, idx: 0},
		}),
		fixCodeSection([][]byte{
			fixFuncBody(),
			fixFuncBody(),
			fixFuncBody(fixI32Const(0)), // request headers  -> Continue
			fixFuncBody(fixI32Const(0)), // response headers -> Continue
			sendLocalResponse(),         // response body
			sendLocalResponse(),         // response trailers
		}),
	)
}

// buildEncodeHeadersLocalResponseProxyWasm is the HEADERS-axis fixture: the
// guest calls proxy_send_local_response from proxy_on_response_headers and
// then returns ProxyActionContinue.
//
// It is separate from buildEncodeLocalResponseProxyWasm on purpose. If the same
// module also fired the hostcall from response-headers, RunEncodeHeaders would
// terminate the stream before the body / trailers cells ever reached their arm,
// and those two cells would pass for the wrong reason.
func buildEncodeHeadersLocalResponseProxyWasm() []byte {
	return fixBuildModule(
		fixTypeSection(
			[2][]byte{nil, nil},
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}},
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}},
		),
		fixImportSection([]fixImport{
			{module: "env", name: "proxy_send_local_response", kind: fixExtFunction, idx: 2},
		}),
		fixFunctionSection([]uint32{0, 0, 1, 1}),
		fixMemorySection(1),
		fixExportSection([]fixExport{
			{name: "_initialize", kind: fixExtFunction, idx: 1},
			{name: "proxy_abi_version_0_2_1", kind: fixExtFunction, idx: 2},
			{name: "proxy_on_request_headers", kind: fixExtFunction, idx: 3},
			{name: "proxy_on_response_headers", kind: fixExtFunction, idx: 4},
			{name: "memory", kind: fixExtMemory, idx: 0},
		}),
		fixCodeSection([][]byte{
			fixFuncBody(),
			fixFuncBody(),
			fixFuncBody(fixI32Const(0)), // request headers -> Continue
			fixFuncBody(
				fixI32Const(403), fixI32Const(0), fixI32Const(0), fixI32Const(0),
				fixI32Const(0), fixI32Const(0), fixI32Const(0), fixI32Const(-1),
				fixCall(0), fixDrop(), fixI32Const(0),
			),
		}),
	)
}

// p83LocalResponseFilter builds the guest above into a real chain and runs
// decode headers to completion so the per-stream StreamContext exists.
func p83LocalResponseFilter(t *testing.T, pluginName string, modBytes []byte) (*filter, *envoyhttp.FilterChain) {
	t.Helper()
	reg := stats.NewRegistry()
	cc := newTestCompiledConfigWithCaps(t, modBytes, pluginName, reg,
		"proxy_send_local_response")
	t.Cleanup(func() { _ = cc.Close() })

	wf := &filter{cfg: cc}
	chain := envoyhttp.NewFilterChain([]envoyhttp.HTTPFilter{
		{Name: "wasm", Decoder: wf, Encoder: wf},
	}, nil)
	t.Cleanup(chain.Destroy)

	h := http.Header{}
	h.Set(":method", "GET")
	terminated, err := chain.RunDecodeHeaders(context.Background(), h, true)
	if err != nil || !terminated {
		t.Fatalf("RunDecodeHeaders(terminated=%v, err=%v); want (true, nil)", terminated, err)
	}
	if wf.streamCtx == nil {
		t.Fatalf("streamCtx is nil after RunDecodeHeaders; every encode dispatch would short-circuit")
	}
	return wf, chain
}

// TestFixture_EncodeLocalResponse_Liveness asserts the guest actually reaches
// the hostcall and leaves a capture on f.sentLocalResponse with a NIL error.
//
// ⚠️ THIS IS NOT PARANOIA. A wasm module with a wrong type-section entry
// INSTANTIATES FINE and fails only at call time inside wazero, where the host
// FAIL-OPENS and returns Continue — and under a denied capability the guest is
// not dispatched at all, with err == nil and no failures bump. Either way every
// cell below would silently skip the arm under test and read GREEN.
func TestFixture_EncodeLocalResponse_Liveness(t *testing.T) {
	t.Parallel()
	wf, _ := p83LocalResponseFilter(t, "p83_lr_live", buildEncodeLocalResponseProxyWasm())

	for _, fn := range []string{proxyOnResponseBody, proxyOnResponseTrailers} {
		if !wf.streamCtx.HasGlobalFunc(fn) {
			t.Errorf("HasGlobalFunc(%q) = false; the guest does not export the callback under test", fn)
		}
	}

	act, err := wf.streamCtx.CallProxyOnResponseBody(context.Background(), 4, true)
	if err != nil {
		t.Errorf("CallProxyOnResponseBody err = %v; want nil (non-nil means the host FAIL-OPENED and every cell below is dead code)", err)
	}
	if act != abi.ProxyActionContinue {
		t.Errorf("CallProxyOnResponseBody = %v; want ProxyActionContinue", act)
	}
	if wf.sentLocalResponse == nil {
		t.Errorf("sentLocalResponse = nil after the guest called proxy_send_local_response; the capture never landed and the arms under test are unreachable")
	} else if wf.sentLocalResponse.statusCode != 403 {
		t.Errorf("sentLocalResponse.statusCode = %d; want 403", wf.sentLocalResponse.statusCode)
	}
}

// TestFilterChain_EncodeLocalResponse_TerminatesStream anchors arms (e) and (f)
// — the encode-side captured-local-response short-circuits in EncodeData and
// EncodeTrailers.
//
// Both returned a PARKING status while the host had already decided the
// response would not be served, and neither side has any escape:
// RunEncodeData / RunEncodeTrailers carry ZERO localReplyDone re-checks and no
// watchdog is armed.
//
// ⚠️ RunEncodeTrailers IS DEAD PRODUCTION CODE at this tip — every non-test hit
// of it is a comment or the definition — so (f) is a latent leak, not a live
// one. It is closed anyway: the arm is one word, and "unreachable today" is not
// a property a filter framework should rely on.
func TestFilterChain_EncodeLocalResponse_TerminatesStream(t *testing.T) {
	t.Parallel()

	t.Run("body", func(t *testing.T) {
		t.Parallel()
		wf, chain := p83LocalResponseFilter(t, "p83_lr_body", buildEncodeLocalResponseProxyWasm())

		respHeaders := http.Header{}
		respHeaders.Set("Content-Type", "text/plain")
		if term, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil || !term {
			t.Fatalf("RunEncodeHeaders(terminated=%v, err=%v); want (true, nil)", term, err)
		}

		terminated, err, returned := p83DriveChain(t, "chain.RunEncodeData (captured local response)", func(ctx context.Context) (bool, error) {
			return chain.RunEncodeData(ctx, []byte("body"), true)
		})
		if !returned {
			return
		}
		if !errors.Is(err, envoyhttp.ErrStreamTerminatedByFilter) {
			t.Errorf("RunEncodeData err = %v; want ErrStreamTerminatedByFilter", err)
		}
		if terminated {
			t.Errorf("RunEncodeData terminated = true; want false")
		}
		if wf.sentLocalResponse != nil {
			t.Errorf("sentLocalResponse = %+v; want nil — the arm must CONSUME the capture so a later callback cannot re-dispatch it", wf.sentLocalResponse)
		}
	})

	// ⚠️ THE ONLY LIVE PARK IN THIS FAMILY. RunEncodeHeaders is driven by ALL
	// THREE HCM dispatchers (connection.go, h2dispatch.go, h3dispatch.go),
	// unlike RunEncodeTrailers, which no production caller reaches. Pre-fix this
	// cell measured a 3 s non-return with nothing but a ctx-cancel to reap the
	// dispatch goroutine.
	t.Run("headers", func(t *testing.T) {
		t.Parallel()
		wf, chain := p83LocalResponseFilter(t, "p83_lr_headers", buildEncodeHeadersLocalResponseProxyWasm())

		respHeaders := http.Header{}
		respHeaders.Set("Content-Type", "text/plain")
		terminated, err, returned := p83DriveChain(t, "chain.RunEncodeHeaders (captured local response)", func(ctx context.Context) (bool, error) {
			return chain.RunEncodeHeaders(ctx, respHeaders, true)
		})
		if !returned {
			return
		}
		if !errors.Is(err, envoyhttp.ErrStreamTerminatedByFilter) {
			t.Errorf("RunEncodeHeaders err = %v; want ErrStreamTerminatedByFilter", err)
		}
		if terminated {
			t.Errorf("RunEncodeHeaders terminated = true; want false")
		}
		if wf.sentLocalResponse != nil {
			t.Errorf("sentLocalResponse = %+v; want nil — the arm must consume the capture", wf.sentLocalResponse)
		}
	})

	t.Run("trailers", func(t *testing.T) {
		t.Parallel()
		wf, chain := p83LocalResponseFilter(t, "p83_lr_trailers", buildEncodeLocalResponseProxyWasm())

		respHeaders := http.Header{}
		respHeaders.Set("Content-Type", "text/plain")
		if term, err := chain.RunEncodeHeaders(context.Background(), respHeaders, false); err != nil || !term {
			t.Fatalf("RunEncodeHeaders(terminated=%v, err=%v); want (true, nil)", term, err)
		}

		trailers := http.Header{}
		trailers.Set("X-Trailer", "1")
		terminated, err, returned := p83DriveChain(t, "chain.RunEncodeTrailers (captured local response)", func(ctx context.Context) (bool, error) {
			return chain.RunEncodeTrailers(ctx, trailers)
		})
		if !returned {
			return
		}
		if !errors.Is(err, envoyhttp.ErrStreamTerminatedByFilter) {
			t.Errorf("RunEncodeTrailers err = %v; want ErrStreamTerminatedByFilter", err)
		}
		if terminated {
			t.Errorf("RunEncodeTrailers terminated = true; want false")
		}
		if wf.sentLocalResponse != nil {
			t.Errorf("sentLocalResponse = %+v; want nil — the arm must consume the capture", wf.sentLocalResponse)
		}
	})
}
