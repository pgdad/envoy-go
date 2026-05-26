package wasm

// decode_headers.go — DecodeHeaders dispatcher per 25.2 SPEC §4.3 + Task 18
// closure of D-P-PLAN-6.
//
// Lifecycle (per 25.2 SPEC §4.3 step list — REVISED at Task 18 for root-VM model):
//
//  1. Lazy per-stream context construction at first DecodeHeaders entry. The
//     per-stream StreamContext is allocated via
//     `cfg.rootVM.NewStreamContext(ctx)` — the shared long-lived *RootVM
//     owned by *compiledConfig (per ADR-0205) supplies the wazero.Runtime +
//     instantiated module; the per-stream StreamContext is just bookkeeping +
//     a `proxy_on_context_create(streamCtxID, rootCtxID)` invocation. NO
//     per-stream wazero.Runtime construction (Task 1 + 18 RETIRED the 25.1
//     per-stream `wasm.NewVM` pattern; the cost moved from ~61µs/stream to
//     microseconds per stream).
//
//  2. The per-stream *abiCallbacks is constructed + registered into the
//     per-RootVM rootABICallbacks multiplexer at the streamCtxID returned by
//     NewStreamContext. Hostcall dispatch from the guest routes through
//     rv.cb (the multiplexer) → multiplexer.lookup(streamCtxID) → per-stream
//     *abiCallbacks → per-stream filter state (decoderCb, encoderCb,
//     requestHeaders, etc.).
//
//  3. Capture headers on f.requestHeaders for the per-stream abiCallbacks
//     back-pointer.
//
//  4. Per-stream filterstate.Bucket pair (downstream + upstream) lazy-init
//     for the ADR-0207 filter_state.* / upstream_filter_state.* / wasm.<key>
//     property branches consumed by abi_callbacks.GetProperty's
//     filterPropertyResolver (per 25.2 IMPL Task 15 + AMEND-B4).
//
//  5. stats.executions.Inc() per AMEND-A2 Group-C envoy-go-strict per-
//     `proxy_on_request_headers`-invocation counter.
//
//  6. CallProxyOnRequestHeaders → abi.ProxyAction. Per 25.2 SPEC §4.3:
//       - ProxyActionContinue → http.Continue
//       - ProxyActionPause without captured-local-response → log +
//         http.Continue (stream-control deferred per parent §1 architectural
//         primitive 6)
//       - sentLocalResponse non-nil → decoderCb.SendLocalReply +
//         http.StopIteration (REUSE 5 per parent §3.3)
//
//  7. On error (wazero trap, hostcall-denial chain, or panic-wrapped Go
//     panic from a bridge callback) → envoy_go.failures.Inc() + log +
//     http.Continue per ADR-0072 boot-time-fail-fast (fail-OPEN on
//     per-stream runtime errors so the request still serves).
//
// # OnDestroy lifecycle (per 25.2 SPEC §4.3 last step + parent §3.5)
//
// OnDestroy delegates to streamCtx.Close(ctx) which fires proxy_on_done +
// proxy_on_log + proxy_on_delete + cancels outstanding httpCalls per
// AMEND-B3. The per-stream entry is unregistered from the per-RootVM
// rootABICallbacks multiplexer so subsequent hostcalls (stray late
// responses, etc.) route to the no-op fallback rather than the freed
// per-stream state. The Group-B active gauge decrements at the same site.
// Idempotent — second OnDestroy is a no-op against a nil streamCtx.
// OnDestroy body lives at encode_headers.go (per the encode-side mirror
// pattern; OnDestroy + EncodeHeaders + SetEncoderCallbacks all colocate
// per the 25.1 file-split convention).
//
// # Cross-references
//
//   - 25.2 SPEC §4.3 (per-stream dispatch shape — REVISED for root-VM model)
//   - ADR-0205 (root-VM lifecycle evolution; *RootVM long-lived per
//     *compiledConfig; per-stream StreamContext children share runtime +
//     module)
//   - parent §1 architectural primitive 6 (stream-control deferred to 25.2)
//   - parent §3.5 (per-stream lifecycle)
//   - ADR-0071 (single-goroutine-per-stream invariant — no synchronization
//     on f.streamCtx / f.streamContextID / f.sentLocalResponse /
//     f.requestHeaders)
//   - ADR-0072 (boot-time-fail-fast — per-stream fail-OPEN is the runtime
//     posture; parse-time fail-CLOSED is the New factory posture)
//   - ADR-0085 (nil-tolerance — stats nil-checks on every increment)
//   - ADR-0196 D-P3 (encoderCb seeded at SetEncoderCallbacks per Task 12)
//   - ADR-0207 (NEW internal/filterstate/ — per-stream Bucket pair
//     allocated here per AMEND-B4)
//   - AMEND-B3 (cancel-at-destruction for outstanding httpCalls fires
//     inside streamCtx.Close at internal/wasm/stream_context.go Close path)

import (
	"context"
	"log"
	gohttp "net/http"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	"github.com/esalaine/envoy-go/internal/filterstate"
	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// logf is the package-level logger for decode/encode-side diagnostics.
// Indirected via a package var to make test-side capture trivial.
// Mirrors the phase-22.1 lua `logf` precedent.
var logf = log.Printf

// DecodeHeaders implements envoyhttp.StreamDecoderFilter per 25.2 SPEC §4.3.
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
	// (Task 11/15) can route guest header-map hostcalls to the request side.
	// Per ADR-0071 single-goroutine-per-stream invariant: no synchronization
	// needed; the same goroutine writes here and reads via the abiCallbacks
	// back-pointer during the CallProxyOnRequestHeaders dispatch below.
	f.requestHeaders = headers

	// Lazy per-stream StreamContext construction at first DecodeHeaders entry.
	// The nil-check guards a defensive double-call (HCM dispatch is single-
	// shot per stream, but tests may exercise pre-construction paths).
	if f.streamCtx == nil {
		if err := f.initStreamContext(context.Background()); err != nil {
			if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
				f.cfg.stats.envoyGoFailures.Inc()
			}
			logf("ERROR wasm: StreamContext construction failed: %v", err)
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

	// CallProxyOnRequestHeaders dispatches the guest's request-headers
	// hook. The result is a ProxyAction (Continue/Pause). On wazero trap
	// or panic-wrapped Go-side panic the error path bumps envoy_go.failures
	// and returns Continue (fail-OPEN per ADR-0072 per-stream posture).
	//
	// Per 25.2 ADR-0205: the per-stream context_create dispatch already
	// fired inside RootVM.NewStreamContext above — no separate
	// CallProxyOnContextCreate call here (the 25.1 separate-call pattern
	// is RETIRED).
	numHeaders := numHeaderValues(headers)
	action, err := f.streamCtx.CallProxyOnRequestHeaders(ctx, numHeaders, endStream)
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

	// ProxyAction handling per 25.2 SPEC §4.3:
	//   - Continue → http.Continue
	//   - Pause w/o captured-local-response → log + http.Continue
	//     (stream-control deferred to 25.2 per parent §1 architectural
	//     primitive 6 — at 25.2 the wasm filter has no pause/resume
	//     plumbing on headers; the guest's request to pause is logged +
	//     ignored so the stream continues).
	switch action {
	case abi.ProxyActionPause:
		logf("INFO wasm: proxy_on_request_headers returned PAUSE without captured local response — stream-control deferred (parent §1 architectural primitive 6); continuing")
		return envoyhttp.Continue
	case abi.ProxyActionContinue:
		fallthrough
	default:
		return envoyhttp.Continue
	}
}

// initStreamContext constructs the per-stream StreamContext via
// `cfg.rootVM.NewStreamContext` + registers the per-stream *abiCallbacks
// into the rootABICallbacks multiplexer per 25.2 SPEC §4.3 + Task 18
// closure of D-P-PLAN-6. Called once per stream at the first DecodeHeaders
// entry.
//
// REPLACES the 25.1 initVM body (deleted at Task 18 alongside the obsolete
// per-stream wasm.NewVM call). The shared *RootVM in cfg.rootVM was
// constructed at buildCompiledConfig (Task 14); the wazero.Runtime + the
// instantiated module live on the *RootVM, not on the per-stream
// StreamContext. Per-stream-context construction cost is microseconds.
func (f *filter) initStreamContext(ctx context.Context) error {
	if f.cfg.rootVM == nil {
		// Defensive: production callers always populate cfg.rootVM at
		// buildCompiledConfig (Task 14). Test-double paths that bypass
		// buildCompiledConfig (e.g. body_test.go's newBodyTestCompiledConfig)
		// may leave it nil; in that case the per-stream callbacks short-
		// circuit per the gate-at-getFunction / nil-streamCtx defensive
		// guards in body.go / trailers.go.
		return errNilRootVM
	}

	streamCtx, err := f.cfg.rootVM.NewStreamContext(ctx)
	if err != nil {
		return err
	}

	// Allocate per-stream filterstate.Bucket pair for the ADR-0207
	// filter_state.* + upstream_filter_state.* + wasm.<key> property
	// branches consumed by abi_callbacks.GetProperty's
	// filterPropertyResolver (per 25.2 IMPL Task 15 + AMEND-B4).
	f.downstreamFilterState = filterstate.NewBucket()
	f.upstreamFilterState = filterstate.NewBucket()

	f.streamCtx = streamCtx
	f.streamContextID = streamCtx.ContextID()

	// Allocate the per-stream *abiCallbacks bound to *filter + register it
	// into the rootABICallbacks multiplexer so hostcalls fired from the
	// guest dispatch route back to this per-stream filter state. The
	// multiplexer's lookup-by-streamCtxID covers cross-stream isolation
	// (N parallel streams + N independent *abiCallbacks bound to N
	// distinct *filters; each guest hostcall routes to the originating
	// stream).
	cb := &abiCallbacks{filter: f}
	if f.cfg.rootCB != nil {
		f.cfg.rootCB.register(f.streamContextID, cb)
	}

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

	return nil
}

// errNilRootVM signals a *filter constructed against a *compiledConfig
// whose rootVM field is nil (test-double path). The filter dispatch fails
// open per ADR-0072 per-stream posture; the production New factory always
// populates rootVM via buildCompiledConfig's NewRootVM at Task 14.
var errNilRootVM = errStaticString("wasm: cfg.rootVM is nil (test-double path)")

// errStaticString is a minimal error type that wraps a constant string
// without depending on errors.New (which would force the package to
// import "errors" again in this file).
type errStaticString string

func (e errStaticString) Error() string { return string(e) }

// DecodeData + DecodeTrailers landed at 25.2 IMPL Task 16 — see body.go +
// trailers.go for the per-stream body-buffer accumulator + cap-enforcement
// + trailer-dispatch shape per 25.2 SPEC §4.3 + Q1 + Q2.

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
