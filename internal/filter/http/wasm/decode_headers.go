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
//  6. CallProxyOnRequestHeaders → abi.ProxyAction. Per 25.2 SPEC §4.3, as
//     REVISED at phase-82 S1 + S9:
//       - ProxyActionContinue → http.Continue
//       - ProxyActionPause without captured-local-response →
//         filter.beginDecodePause (records the owed resume + arms the S9
//         watchdog) + http.StopIteration. ⚠️ This step formerly read "log +
//         http.Continue (stream-control deferred per parent §1 architectural
//         primitive 6)" — that deferral ENDED at phase 82 and the sentence
//         outlived it by a full phase. Pause is HONORED here.
//       - sentLocalResponse non-nil → decoderCb.SendLocalReply +
//         http.StopIteration, or envoyhttp.TerminateStream when decoderCb is
//         nil (REUSE 5 per parent §3.3)
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
//   - parent §1 architectural primitive 6 (stream-control deferred to 25.2 —
//     that deferral is DISCHARGED: phase-82 S1 honors Pause on the headers
//     arms, phase-83 S1 + S2 on the body and trailers arms)
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

	"google.golang.org/protobuf/proto"

	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
	"github.com/pgdad/envoy-go/internal/filterstate"
	internalwasm "github.com/pgdad/envoy-go/internal/wasm"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// logf is the package-level logger for decode/encode-side diagnostics.
// Indirected via a package var to make test-side capture trivial.
// Mirrors the phase-22.1 lua `logf` precedent.
var logf = log.Printf

// proxyOnRequestHeaders / proxyOnResponseHeaders are the byte-stable
// proxy-wasm v0.2.1 guest-export names for the two headers callbacks, in the
// same shape as body.go's proxyOnRequestBody / proxyOnResponseBody.
//
// Added at phase-83 S1 for the pause-watchdog attribution parameter: the
// watchdog's operator-facing warning names the guest callback the pause came
// from, and before S1 that name was a hard-coded literal inside pause.go —
// correct for the headers arms, a misattribution for every other arm.
const (
	proxyOnRequestHeaders  = "proxy_on_request_headers"
	proxyOnResponseHeaders = "proxy_on_response_headers"
)

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

	ctx := context.Background()

	// Per-route resolution per phase-25.3 Task 9 + AMEND-C1. Resolve the
	// EFFECTIVE compiledConfig ONCE here (per-route override > listener
	// default) + reuse it for the WHOLE per-stream lifecycle (this dispatch +
	// EncodeHeaders + OnDestroy) so a per-route override swaps the entire VM
	// consistently. The framework's RequestRouteConfig returns the 3-tier-
	// resolved per-route proto (or nil); decoderCb may be nil in test-double
	// paths (guard). A dispatch-time per-route build error is UNEXPECTED (the
	// proto was shape-validated at HCM-build); fail-OPEN per the ADR-0072 per-
	// stream runtime posture.
	var routeProto proto.Message
	if f.decoderCb != nil {
		routeProto = f.decoderCb.RequestRouteConfig()
	}
	eff, err := f.cfg.resolveEffective(routeProto)
	if err != nil {
		if f.cfg.stats != nil && f.cfg.stats.envoyGoFailures != nil {
			f.cfg.stats.envoyGoFailures.Inc()
		}
		logf("ERROR wasm: per-route resolveEffective failed: %v", err)
		return envoyhttp.Continue
	}
	f.eff = eff

	// Reload integration per phase-25.3 Task 9 (FAIL_RELOAD) + BUG-4 reorder.
	// BEFORE constructing the per-stream StreamContext (which fires
	// proxy_on_context_create on the shared instance), consult the reload state
	// machine of eff.rootVM. ReloadDispatch is serialized (dispatchMu) + advances
	// the machine: it serves Running, attempts a reload past the backoff window,
	// or reports Backoff within the window. The vm_reload triplet counters are
	// incremented INSIDE ReloadDispatch (via the WithReloadStats seam wired at
	// Task 8) — we do NOT double-increment. nil rootVM (test-double) →
	// ReloadAvailable benign pass-through.
	//
	// BUG-4: this MUST run BEFORE initStreamContext. When a prior guest trap has
	// POISONED the shared proxy-wasm instance (a Rust-SDK RefCell left borrowed
	// on panic — ANY subsequent entry into the instance re-traps), constructing
	// the per-stream context FIRST would fire proxy_on_context_create on the
	// poisoned instance → that traps → initStreamContext returns an error →
	// DecodeHeaders fail-OPENs at the initStreamContext branch → ReloadDispatch
	// is NEVER reached → the Failed VM never reinstantiates (vm_reload_backoff /
	// vm_reload_success stay 0 forever). Running ReloadDispatch first means a
	// Failed VM within backoff → 503/bypass WITHOUT touching the poisoned
	// instance, and a Failed VM past backoff → reinstantiate (FRESH, un-poisoned
	// instance) so the subsequent context-create lands on clean state.
	if eff.rootVM != nil {
		if avail := eff.rootVM.ReloadDispatch(ctx); avail == internalwasm.ReloadUnavailable {
			// The VM is unavailable (Failed within backoff OR a reload attempt
			// just failed). Apply the failure-policy disposition WITHOUT
			// constructing a per-stream context on / dispatching the guest of
			// the poisoned instance.
			return f.applyFailureDisposition(eff)
		}
	}

	// Lazy per-stream StreamContext construction at first DecodeHeaders entry,
	// bound to the EFFECTIVE config's *RootVM. The nil-check guards a defensive
	// double-call (HCM dispatch is single-shot per stream, but tests may
	// exercise pre-construction paths). Per the BUG-4 reorder above this now runs
	// AFTER ReloadDispatch, so proxy_on_context_create fires on a VM that is
	// confirmed available (Running, or freshly reinstantiated post-backoff) —
	// never on a poisoned-but-Failed instance.
	if f.streamCtx == nil {
		if err := f.initStreamContext(ctx, eff); err != nil {
			if eff.stats != nil && eff.stats.envoyGoFailures != nil {
				eff.stats.envoyGoFailures.Inc()
			}
			logf("ERROR wasm: StreamContext construction failed: %v", err)
			// Fail-OPEN on per-stream runtime errors — the request still
			// serves, just without wasm processing. Matches the ADR-0072
			// per-stream fail-OPEN runtime posture (parse-time fail-CLOSED
			// posture is at the New factory).
			return envoyhttp.Continue
		}
	}

	// stats.executions.Inc() per AMEND-A2 Group-C envoy-go-strict per-
	// `proxy_on_request_headers`-invocation counter. Counts every dispatch
	// ATTEMPT that reached the guest, INCLUDING ones where the guest subsequently
	// traps — incremented BEFORE CallProxyOnRequestHeaders returns.
	if eff.stats != nil && eff.stats.executions != nil {
		eff.stats.executions.Inc()
	}

	// CallProxyOnRequestHeaders dispatches the guest's request-headers
	// hook. The result is a ProxyAction (Continue/Pause). On wazero trap
	// or panic-wrapped Go-side panic err is non-nil.
	//
	// Per 25.2 ADR-0205: the per-stream context_create dispatch already
	// fired inside RootVM.NewStreamContext above — no separate
	// CallProxyOnContextCreate call here (the 25.1 separate-call pattern
	// is RETIRED).
	numHeaders := numHeaderValues(headers)
	action, err := f.streamCtx.CallProxyOnRequestHeaders(ctx, numHeaders, endStream)
	if err != nil {
		logf("ERROR wasm: CallProxyOnRequestHeaders(stream=%d): %v",
			f.streamContextID, err)
		// Reload-on-RuntimeError per phase-25.3 Task 9: the guest TRAPPED. If
		// the effective policy is FAIL_RELOAD (reload-eligible for a runtime
		// error), arm the reload machine so the NEXT request drives a reload-
		// or-backoff. Then apply the failure-policy disposition for THIS
		// request (FAIL_CLOSED → 503; FAIL_OPEN → bypass; FAIL_RELOAD serves
		// FAIL_CLOSED for the trapping request itself + reloads next time).
		//
		// ALL non-nil dispatch errors arm the reload machine under FAIL_RELOAD,
		// not only confirmed wazero RuntimeErrors — host-side errors are rare
		// and a spurious reload is benign (matches the pre-25.3 "any error =
		// failure" stance). applyFailureDisposition is the SINGLE
		// envoy_go.failures increment site for this path (no pre-call inc here).
		if eff.rootVM != nil && internalwasm.ReloadEligible(eff.failurePolicy, internalwasm.FailStateRuntimeError) {
			eff.rootVM.NoteReloadRuntimeError()
		}
		return f.applyFailureDisposition(eff)
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
		// Phase-83 S8: with a decoderCb, beginLocalReply sets localReplyDone
		// synchronously and RunDecodeHeaders re-checks it BEFORE the status
		// switch, so StopIteration is never read. With a nil one nothing sets
		// the flag and the status parks with no resumer.
		if f.decoderCb == nil {
			logf("WARN wasm: captured local response from proxy_on_request_headers (stream=%d, status=%d) but decoderCb is nil — terminating the stream",
				f.streamContextID, captured.statusCode)
			return envoyhttp.TerminateStream
		}
		addl := convertHeaderPairsToOrderedHeaders(captured.additionalHeaders)
		f.decoderCb.SendLocalReply(int(captured.statusCode), captured.body, addl)
		return envoyhttp.StopIteration
	}

	// ProxyAction handling per 25.2 SPEC §4.3:
	//   - Continue → http.Continue
	//   - Pause w/o captured-local-response → StopIteration (phase-82 S1).
	//
	// Pause is now HONORED. Before phase-82 this arm logged "stream-control
	// deferred" and returned Continue, i.e. the guest's pause was silently
	// discarded. Returning StopIteration makes FilterChain.RunDecodeHeaders
	// park the stream on the chain-scoped decodeResumeCh; the guest resumes it
	// with proxy_continue_stream(0), which lands in abiCallbacks.ContinueStream
	// -> filter.resumeDecode.
	//
	// ⚠️ THE PHASE-82 CONSOLATION IN THIS PARAGRAPH WAS FALSE AND IS DELETED.
	// It said the other "FOUR of the six ProxyActionPause arms in this package
	// (body.go x2, trailers.go x2) had honored Pause all along." They returned
	// a PARKING STATUS all along, which is not the same thing: none of them
	// set decodePaused/encodePaused, so resumeDecode's CompareAndSwap failed
	// and the guest's own proxy_continue_stream fired NOTHING while still
	// returning WasmResult::Ok, and none of them armed a watchdog, so the park
	// had no bound either. Those four arms were the WORSE half of the defect,
	// not the honored half; phase-83 S1 + S2 close them.
	//
	// beginDecodePause records that THIS filter owes exactly one resume (the
	// CAS gate against spuriously un-parking a sibling filter on the same
	// chain) and arms the S9 watchdog that bounds the park — see pause.go for
	// both, including the measured connection+goroutine leak an unbounded
	// park produces.
	switch action {
	case abi.ProxyActionPause:
		f.beginDecodePause(proxyOnRequestHeaders)
		return envoyhttp.StopIteration
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
func (f *filter) initStreamContext(ctx context.Context, eff *compiledConfig) error {
	if eff.rootVM == nil {
		// Defensive: production callers always populate rootVM at
		// buildCompiledConfig (Task 14). Test-double paths that bypass
		// buildCompiledConfig (e.g. body_test.go's newBodyTestCompiledConfig)
		// may leave it nil; in that case the per-stream callbacks short-
		// circuit per the gate-at-getFunction / nil-streamCtx defensive
		// guards in body.go / trailers.go.
		return errNilRootVM
	}

	streamCtx, err := eff.rootVM.NewStreamContext(ctx)
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
	if eff.rootCB != nil {
		eff.rootCB.register(f.streamContextID, cb)
	}

	// Group-B upstream-parity counters: incr created on construction;
	// incr active gauge (decr at OnDestroy).
	if eff.stats != nil {
		if eff.stats.created != nil {
			eff.stats.created.Inc()
		}
		if eff.stats.active != nil {
			eff.stats.active.Inc()
		}
	}

	return nil
}

// applyFailureDisposition returns the per-request FilterHeadersStatus when the
// guest is unavailable (the reload machine reported the VM unavailable) OR the
// guest dispatch TRAPPED, per the effective failure policy (phase-25.3 Task 9 +
// AMEND-C3 + R-25.3-3):
//
//   - FAIL_OPEN → bypass the filter: Continue, no local reply. The stream
//     proceeds as if the wasm filter weren't present. NOT treated as an error.
//   - FAIL_CLOSED / FAIL_RELOAD (and the proto-default UNSPECIFIED→FailClosed)
//     → fail the stream: send a 503 local reply via decoderCb.SendLocalReply
//     (matching the REUSE-5 captured-local-response SendLocalReply path) +
//     StopIteration. envoy_go.failures is bumped (the request did not serve the
//     guest's intended processing — per §2.25 failure-path stat discipline).
//
// The vm_reload triplet counters are NOT touched here (ReloadDispatch owns
// them). decoderCb may be nil on test-double / encode-only paths — the 503
// branch then returns TerminateStream instead (phase-83 S8: it used to degrade
// to StopIteration, which PARKS with no resumer because nothing sets
// localReplyDone when no reply is sent). FAIL_RELOAD shares the FAIL_CLOSED
// disposition for the unavailable /
// trapping request itself; the reload-or-backoff happens on the NEXT request.
func (f *filter) applyFailureDisposition(eff *compiledConfig) envoyhttp.FilterHeadersStatus {
	if eff.failurePolicy == internalwasm.FailurePolicyFailOpen {
		// Bypass — the stream proceeds without wasm processing. No failure
		// counter (an intentional bypass is not a failure).
		logf("INFO wasm: FAIL_OPEN bypass (stream=%d) — VM unavailable/trapped; continuing without wasm processing",
			f.streamContextID)
		return envoyhttp.Continue
	}

	// FAIL_CLOSED / FAIL_RELOAD-within-backoff / UNSPECIFIED → 503.
	if eff.stats != nil && eff.stats.envoyGoFailures != nil {
		eff.stats.envoyGoFailures.Inc()
	}
	logf("INFO wasm: FAIL_CLOSED 503 (stream=%d) — VM unavailable/trapped under failure_policy=%d",
		f.streamContextID, eff.failurePolicy)
	// Phase-83 S8: same asymmetry as the captured-local-response arm above.
	if f.decoderCb == nil {
		logf("WARN wasm: FAIL_CLOSED (stream=%d) but decoderCb is nil — cannot emit 503; terminating the stream", f.streamContextID)
		return envoyhttp.TerminateStream
	}
	f.decoderCb.SendLocalReply(gohttp.StatusServiceUnavailable, "", nil)
	return envoyhttp.StopIteration
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
//
// DELIBERATE TWIN: internal/wasm/http_call.go has an identical unexported
// headerValueCount, used to size the proxy_on_http_call_response num_headers /
// num_trailers args. The duplication is intentional: internal/wasm cannot
// import this package (the dependency runs THIS package → internal/wasm, and
// not the reverse — see `go list -deps`). Keep the two in sync.
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
