package wasm

// decode_headers.go — DecodeHeaders dispatcher per 25.1 SPEC §4.3.
//
// Lifecycle (per 25.1 SPEC §4.3 step list):
//
//  1. Lazy per-stream VM construction at first DecodeHeaders entry. The VM
//     is built via wasm.NewVM(WithSandboxConfig, WithCompilationCache),
//     registered with an *abiCallbacks bundle (Task 11) bound to this
//     *filter, then driven through the module-init lifecycle via
//     vm.Run(ctx, module, rootContextID) — re-compiles module.Source()
//     against the per-stream runtime (sub-ms cache hit via the shared
//     wazero.CompilationCache), instantiates, runs _initialize OR _start
//     (mutually exclusive per proxy-wasm v0.2.1), then proxy_on_vm_start
//     + proxy_on_configure.
//
//  2. Per-stream context-ID allocation via the package-level atomic
//     streamContextIDCounter. CallProxyOnContextCreate(streamContextID,
//     rootContextID) seeds the per-stream context on the guest side.
//
//  3. Capture headers on f.requestHeaders for abiCallbacks (Task 11
//     pattern — the *abiCallbacks back-pointer reads f.requestHeaders /
//     f.responseHeaders directly to route guest header-map hostcalls).
//
//  4. stats.executions.Inc() per AMEND-A2 Group-C envoy-go-strict
//     per-`proxy_on_request_headers`-invocation counter.
//
//  5. CallProxyOnRequestHeaders → abi.ProxyAction. Per 25.1 SPEC §4.3:
//       - ProxyActionContinue → http.Continue
//       - ProxyActionPause without captured-local-response → log +
//         http.Continue (stream-control deferred to 25.2 per parent §1
//         architectural primitive 6)
//       - sentLocalResponse non-nil → decoderCb.SendLocalReply +
//         http.StopIteration (REUSE 5 per parent §3.3)
//
//  6. On error (wazero trap, hostcall-denial chain, or panic-wrapped Go
//     panic from a bridge callback) → envoy_go.failures.Inc() + log +
//     http.Continue per ADR-0072 boot-time-fail-fast (fail-OPEN on
//     per-stream runtime errors so the request still serves).
//
// # OnDestroy lifecycle (per 25.1 SPEC §4.3 last step + parent §3.5)
//
// OnDestroy releases the per-stream VM via CallProxyOnDone (guest's
// chance to defer finalize per SPEC §3.1) + CallProxyOnLog (guest's
// last per-stream log point) + CallProxyOnDelete (context teardown) +
// vm.Close (releases the wazero.Runtime). The Group-B active gauge
// decrements at the same site. Idempotent — second OnDestroy is a
// no-op against a nil vm.
//
// # Cross-references
//
//   - 25.1 SPEC §4.3 (per-stream dispatch shape)
//   - parent §1 architectural primitive 6 (stream-control deferred to 25.2)
//   - parent §3.5 (per-stream lifecycle)
//   - ADR-0071 (single-goroutine-per-stream invariant — no synchronization
//     on f.vm / f.streamContextID / f.sentLocalResponse / f.requestHeaders)
//   - ADR-0072 (boot-time-fail-fast — per-stream fail-OPEN is the runtime
//     posture; parse-time fail-CLOSED is the New factory posture)
//   - ADR-0085 (nil-tolerance — stats nil-checks on every increment)
//   - ADR-0196 D-P3 (encoderCb seeded at SetEncoderCallbacks per Task 12)

import (
	"context"
	"log"
	gohttp "net/http"
	"sync/atomic"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// streamContextIDCounter allocates fresh per-stream u32 context IDs at
// DecodeHeaders entry per 25.1 SPEC §4.3. Per-process monotonic; atomic
// load/store is safe across all per-stream-goroutine call sites (each
// stream allocates exactly one ID at its first DecodeHeaders call).
//
// Per upstream proxy-wasm convention root context IDs are typically 1
// and stream context IDs start at 2; envoy-go uses a single monotonic
// counter per kind (rootContextIDCounter in compiled_config.go for root;
// this counter for stream) — collisions between the two namespaces are
// harmless because the guest sees them via distinct proxy_on_* callbacks.
var streamContextIDCounter atomic.Uint32

// logf is the package-level logger for decode/encode-side diagnostics.
// Indirected via a package var to make test-side capture trivial.
// Mirrors the phase-22.1 lua `logf` precedent.
var logf = log.Printf

// DecodeHeaders implements envoyhttp.StreamDecoderFilter per 25.1 SPEC §4.3.
// See the file-header comment for the full step list + error-handling
// disposition.
func (f *filter) DecodeHeaders(headers gohttp.Header, endStream bool) envoyhttp.FilterHeadersStatus {
	// Defensive nil-cfg pass-through. Should-not-happen in production
	// (New factory always allocates cfg); preserved for test-double paths
	// that construct *filter{} directly.
	if f.cfg == nil {
		return envoyhttp.Continue
	}

	// Capture request headers on the *filter so the per-stream *abiCallbacks
	// (Task 11) can route guest header-map hostcalls to the request side.
	// Per ADR-0071 single-goroutine-per-stream invariant: no synchronization
	// needed; the same goroutine writes here and reads via the abiCallbacks
	// back-pointer during the CallProxyOnRequestHeaders dispatch below.
	f.requestHeaders = headers

	// Lazy per-stream VM construction at first DecodeHeaders entry. The
	// nil-check guards a defensive double-call (HCM dispatch is single-
	// shot per stream, but tests may exercise pre-construction paths).
	if f.vm == nil {
		if err := f.initVM(context.Background()); err != nil {
			if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
				f.cfg.stats.envoyGoFailures.Inc()
			}
			logf("ERROR wasm: VM construction failed: %v", err)
			// Fail-OPEN on per-stream runtime errors — the request still
			// serves, just without wasm processing. Matches the ADR-0072
			// per-stream fail-OPEN runtime posture (parse-time fail-CLOSED
			// posture is at the New factory).
			return envoyhttp.Continue
		}
	}

	ctx := context.Background()

	// stats.executions.Inc() per AMEND-A2 Group-C envoy-go-strict per-
	// `proxy_on_request_headers`-invocation counter.
	if f.cfg.stats != nil && f.cfg.stats.executions != nil {
		f.cfg.stats.executions.Inc()
	}

	// CallProxyOnContextCreate seeds the per-stream context on the guest
	// side. Idempotent — the guest may not export proxy_on_context_create
	// (VM helper returns nil + no-op in that case). On error: count it as
	// a failure but proceed to dispatch (the guest may still handle
	// proxy_on_request_headers correctly without a context-create hook).
	if err := f.vm.CallProxyOnContextCreate(ctx, f.streamContextID, f.cfg.rootContextID); err != nil {
		if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnContextCreate(stream=%d, root=%d): %v",
			f.streamContextID, f.cfg.rootContextID, err)
		return envoyhttp.Continue
	}

	// CallProxyOnRequestHeaders dispatches the guest's request-headers
	// hook. The result is a ProxyAction (Continue/Pause). On wazero trap
	// or panic-wrapped Go-side panic the error path bumps envoy_go.failures
	// and returns Continue (fail-OPEN per ADR-0072 per-stream posture).
	numHeaders := numHeaderValues(headers)
	action, err := f.vm.CallProxyOnRequestHeaders(ctx, f.streamContextID, numHeaders, endStream)
	if err != nil {
		if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: CallProxyOnRequestHeaders(stream=%d): %v",
			f.streamContextID, err)
		return envoyhttp.Continue
	}

	// REUSE 5 (parent §3.3): captured-local-response short-circuits the
	// chain via SendLocalReply + StopIteration. The capture is performed
	// inside the abiCallbacks.SendLocalResponse body (Task 11) which
	// stashes the payload on f.sentLocalResponse during the guest's
	// proxy_send_local_response hostcall invoked from within the
	// CallProxyOnRequestHeaders dispatch above.
	if f.sentLocalResponse != nil {
		captured := f.sentLocalResponse
		f.sentLocalResponse = nil // consume; idempotent against double-dispatch
		if f.decoderCb != nil {
			addl := convertHeaderPairsToOrderedHeaders(captured.additionalHeaders)
			f.decoderCb.SendLocalReply(int(captured.statusCode), captured.body, addl)
		}
		return envoyhttp.StopIteration
	}

	// ProxyAction handling per 25.1 SPEC §4.3:
	//   - Continue → http.Continue
	//   - Pause w/o captured-local-response → log + http.Continue
	//     (stream-control deferred to 25.2 per parent §1 architectural
	//     primitive 6 — at 25.1 the wasm filter has no pause/resume
	//     plumbing; the guest's request to pause is logged + ignored so
	//     the stream continues).
	switch action {
	case abi.ProxyActionPause:
		logf("INFO wasm: proxy_on_request_headers returned PAUSE without captured local response — stream-control deferred to 25.2 (parent §1 architectural primitive 6); continuing")
		return envoyhttp.Continue
	case abi.ProxyActionContinue:
		fallthrough
	default:
		return envoyhttp.Continue
	}
}

// initVM constructs the per-stream wazero VM + registers the per-stream
// abiCallbacks bundle + runs the module-init lifecycle per 25.1 SPEC §4.3
// step 1. Called once per stream at the first DecodeHeaders entry.
//
// The shared wazero.CompilationCache from the *compiledConfig is wired
// in via WithCompilationCache so the per-stream vm.Run re-compile of
// module.Source() against the per-stream runtime hits the shared codegen
// cache as a sub-ms cache lookup (per wazero v1.10.1's
// CompiledModule-bound-to-runtime cross-runtime fix at Task 7 follow-up).
func (f *filter) initVM(ctx context.Context) error {
	opts := []internalwasm.VMOption{
		internalwasm.WithSandboxConfig(f.cfg.sandbox),
	}
	if f.cfg.compileCache != nil {
		if wc := f.cfg.compileCache.WazeroCompilationCache(); wc != nil {
			opts = append(opts, internalwasm.WithCompilationCache(wc))
		}
	}
	f.vm = internalwasm.NewVM(ctx, opts...)

	// Register the per-stream ABICallbacks bundle (Task 11). The bundle
	// holds only a back-pointer to *filter; all per-stream state
	// (requestHeaders, responseHeaders, decoderCb, encoderCb,
	// sentLocalResponse) lives on the *filter.
	cb := &abiCallbacks{filter: f}
	f.vm.RegisterABICallbacks(cb)

	// Group-B upstream-parity counters: incr created on construction;
	// incr active gauge (decr at OnDestroy).
	if f.cfg.stats != nil {
		if f.cfg.stats.created != nil {
			f.cfg.stats.created.Inc()
		}
		if f.cfg.stats.active != nil {
			f.cfg.stats.active.Inc()
		}
	}

	// Allocate a fresh per-stream context ID. Monotonic per process; the
	// counter never wraps within a realistic process lifetime (u32 ≈ 4G).
	f.streamContextID = streamContextIDCounter.Add(1)

	// Run the module-init lifecycle: re-compile module.Source against the
	// per-stream runtime → instantiate → _initialize or _start →
	// proxy_on_vm_start → proxy_on_configure.
	if err := f.vm.Run(ctx, f.cfg.module, f.cfg.rootContextID); err != nil {
		// Release the VM on init failure — the caller's nil-vm check
		// guards against double-init attempts.
		_ = f.vm.Close()
		f.vm = nil
		// Decrement the active gauge we just incremented.
		if f.cfg.stats != nil && f.cfg.stats.active != nil {
			f.cfg.stats.active.Dec()
		}
		return err
	}

	return nil
}

// DecodeData is a no-op pass-through at 25.1 per parent SPEC §3.5 +
// 25.1 SPEC §4.3 (headers-bridge subset only). Body bridge lands at 25.2.
func (f *filter) DecodeData(_ []byte, _ bool) envoyhttp.FilterDataStatus {
	return envoyhttp.DataContinue
}

// DecodeTrailers is a no-op pass-through at 25.1 per parent SPEC §3.5 +
// 25.1 SPEC §4.3 (headers-bridge subset only). Trailers bridge lands at 25.2.
func (f *filter) DecodeTrailers(_ gohttp.Header) envoyhttp.FilterTrailersStatus {
	return envoyhttp.TrailersContinue
}

// SetDecoderCallbacks stores the framework-supplied per-stream callback
// reference for the decode-side SendLocalReply path consumed at the
// REUSE 5 short-circuit above. Per 25.1 SPEC §4.3 + ADR-0071 the callback
// reference is per-stream + per-side; the *filter holds both decoder + encoder
// callbacks because the *abiCallbacks back-pointer reads from BOTH sides
// (decoderCb for SendLocalReply; encoderCb for the ADR-0196 D-P3
// ResponseStatus accessor consumed by GetStatus).
func (f *filter) SetDecoderCallbacks(cb envoyhttp.DecoderFilterCallbacks) { f.decoderCb = cb }

// numHeaderValues returns the total value count for the header map. Multi-
// value headers contribute their full value count — matching the
// abiCallbacks.GetHeaderMapSize semantic at Task 11 + the proxy-wasm
// pair-emission shape (one wire pair per (key, value) tuple).
func numHeaderValues(h gohttp.Header) uint32 {
	var n uint32
	for _, vs := range h {
		//nolint:gosec // header count is bounded by http.Header invariants; will not exceed uint32.
		n += uint32(len(vs))
	}
	return n
}

// convertHeaderPairsToOrderedHeaders projects the captured guest-side
// proxy_send_local_response additional-headers slice ([]internalwasm.HeaderPair)
// to the framework's envoyhttp.OrderedHeaders carrier consumed by
// DecoderFilterCallbacks.SendLocalReply. Preserves caller (guest) insertion
// order — the proxy-wasm wire format pairs guest headers in the order the
// guest emitted them, matching the SPEC §11.2 verbatim ordered-headers
// discipline that drove the OrderedHeaders carrier extraction at phase-07.1.
//
// Empty input returns nil (matching the OrderedHeaders zero-value
// semantic).
func convertHeaderPairsToOrderedHeaders(pairs []internalwasm.HeaderPair) envoyhttp.OrderedHeaders {
	if len(pairs) == 0 {
		return nil
	}
	out := make(envoyhttp.OrderedHeaders, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, envoyhttp.HeaderField{Name: p.Key, Value: p.Value})
	}
	return out
}
