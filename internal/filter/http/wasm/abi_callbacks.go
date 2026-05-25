package wasm

// abi_callbacks.go — Task 11 implementation of `internalwasm.ABICallbacks`
// for the per-stream HTTP-filter context per 25.1 SPEC §3.5 + §4.3 + parent
// §4.2 + §4.5 D6 + D-P3 ADR-0196 first co-consumer.
//
// The `abiCallbacks` struct routes the 16 ABICallbacks methods (7 header-map
// + GetProperty/SetProperty + SendLocalResponse + GetStatus + Log +
// GetLogLevel + GetCurrentTimeNanoseconds + SetEffectiveContext + Done) to
// the per-stream filter state. Header-map methods dispatch on
// `abi.WasmHeaderMapType`: only types 0 (request headers) + 2 (response
// headers) are active at 25.1; types 1/3/4/5/6/7 (trailers + http-call +
// gRPC metadata) return Unimplemented (setters) or NotFound semantic
// (getters) — those surfaces land at 25.2 (request/response trailers +
// proxy_http_call response handlers) + 25.3 (gRPC bidi-stream metadata).
//
// # D-P3 closure (ADR-0196 first co-consumer)
//
// Per PLAN Task 11 Step 1: re-read ADR-0196 (`docs/envoy-go/DECISIONS.md`
// lines 12503-12542). CONFIRMED:
//
//   - `EncoderFilterCallbacks.ResponseStatus() int` accessor returns the
//     HTTP response status code as an int.
//   - Set-once by HCM dispatch via `chain.SetEncodeResponseStatus(status)`
//     BEFORE `RunEncodeHeaders` (H1 connection.go + H2 h2dispatch.go +
//     chain.go beginLocalReply).
//   - Read by encoder filters during encode-filter iteration via the
//     per-stream `*encoderCB.ResponseStatus()` accessor.
//   - Phase-23 admission_control was the FIRST consumer (`encode.go` line
//     135: `code := f.ecb.ResponseStatus()`); Task 11 wasm's `GetStatus`
//     is the SECOND co-consumer — RATIFIES the phase-23 framework primitive
//     extraction discipline (analogous to phase-22.2's first co-consumer of
//     phase-20 `internal/httpclient/`).
//
// The `GetStatus` body re-consumes the accessor verbatim: if the per-stream
// `encoderCb` is non-nil AND `ResponseStatus() > 0`, project to
// `(uint32(code), []byte("<code>"), true)`; else return `(0, nil, false)`
// (NotFound semantic — the host hostcall wrapper at registration.go line
// 437-446 converts `!ok` to `WasmResultNotFound`).
//
// # Cross-references
//
//   - ADR-0196 (NEW EncoderFilterCallbacks.ResponseStatus() — set-once by
//     HCM dispatch; phase-23 first consumer; Task 11 first co-consumer)
//   - ADR-0202 (NEW internal/wasm/ framework primitive — the ABICallbacks
//     interface this file implements)
//   - ADR-0203 (NEW internal/filter/http/wasm/ package shape — per-stream
//     HTTP context shape for the ABICallbacks consumer)
//   - ADR-0204 (default-deny capability sandbox — Task 11 callbacks are
//     gated at registration.go's hostcall body via vm.sandbox.IsAllowed
//     BEFORE reaching this file)
//   - parent §4.2 (the 47-hostcall surface — 16 active proxy_* + 8 active
//     wasi_* + 23 deferred stubs)
//   - parent §4.5 D6 guardrail (b) (cross-side determinism — GetHeaderMap
//     returns sorted pairs so wazero/V8 byte-exact differential testing
//     holds)
//   - 25.1 SPEC §3.5 (HTTP-side implementer obligations for the headers-
//     bridge subset)
//   - 25.1 SPEC §5.1 (the 16 active proxy_* hostcalls + this file's per-
//     callback dispatch)

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"time"

	internalwasm "github.com/esalaine/envoy-go/internal/wasm"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// abiCallbacks implements internalwasm.ABICallbacks for the per-stream
// HTTP-filter context. Allocated fresh per-stream at Task 12's DecodeHeaders
// (one per filter; bound to the per-stream *filter). NOT goroutine-safe —
// per-stream-goroutine isolation per ADR-0071's single-goroutine-per-stream
// invariant.
//
// The struct holds ONLY a back-pointer to the *filter — all per-stream state
// (requestHeaders, responseHeaders, decoderCb, encoderCb, sentLocalResponse,
// vm) lives on the *filter. This shape keeps the call-site at Task 12
// simple: `vm.RegisterABICallbacks(&abiCallbacks{filter: f})`.
type abiCallbacks struct {
	// filter is the per-stream filter back-pointer. Non-nil at production
	// call sites; nil-tolerant per ADR-0085 — SendLocalResponse returns
	// WasmResultInternalFailure if nil; Log/other side-effecting methods
	// degrade to no-op.
	filter *filter
}

// Compile-time conformance: *abiCallbacks satisfies internalwasm.ABICallbacks.
// A regression on either side (interface gains a method; this file fails to
// implement; etc.) surfaces immediately at package build.
var _ internalwasm.ABICallbacks = (*abiCallbacks)(nil)

// -----------------------------------------------------------------------------
// 7 header-map methods (per 25.1 SPEC §5.1 hostcalls 4-10).
//
// Dispatch on abi.WasmHeaderMapType. Only types 0 (request headers) + 2
// (response headers) are active at 25.1. Types 1 (request trailers), 3
// (response trailers), 4 (http-call response headers), 5 (http-call
// response trailers), 6 (gRPC initial metadata), 7 (gRPC trailing metadata)
// return:
//
//   - Getters (GetHeaderMap/GetHeaderMapValue/GetHeaderMapSize): (nil/zero,
//     false) — the host wrapper at registration.go converts !ok to
//     WasmResultNotFound (=1) for the guest.
//   - Setters (AddHeaderMapValue/ReplaceHeaderMapValue/RemoveHeaderMapValue/
//     SetHeaderMapPairs): WasmResultUnimplemented (=12).
//
// The active surfaces land at 25.2 (request/response trailers + proxy_http_
// call response handlers) + 25.3 (gRPC bidi-stream metadata). The 25.1
// guest contract is: deferred map types behave as if the surface were
// absent — well-behaved guests handle Unimplemented + NotFound returns
// gracefully per proxy-wasm v0.2.1 ABI guidance.
// -----------------------------------------------------------------------------

// headerMapForType returns the per-side http.Header for the given map type
// + a flag indicating whether the type is ACTIVE at 25.1 (types 0 + 2) vs
// deferred-to-25.2/25.3 (types 1/3/4/5/6/7).
//
//   - active=true,  h!=nil   → ready (request or response headers captured)
//   - active=true,  h==nil   → active type but uncaptured (pre-dispatch side)
//   - active=false, h==nil   → deferred type (return Unimplemented / NotFound)
func (a *abiCallbacks) headerMapForType(mapType abi.WasmHeaderMapType) (h http.Header, active bool) {
	if a.filter == nil {
		return nil, false
	}
	switch mapType {
	case abi.WasmHeaderMapTypeHttpRequestHeaders:
		return a.filter.requestHeaders, true
	case abi.WasmHeaderMapTypeHttpResponseHeaders:
		return a.filter.responseHeaders, true
	default:
		// Deferred map types (1/3/4/5/6/7).
		return nil, false
	}
}

// GetHeaderMap returns sorted (key, value) pairs per parent §4.5 D6
// guardrail (b) cross-side determinism. Multi-value headers expand to one
// pair per value (the upstream proxy-wasm contract: pairs is a flat list
// where the same key may appear multiple times for multi-value headers).
//
// Returns (nil, false) for deferred map types or uncaptured sides — the
// host wrapper converts !ok to WasmResultNotFound for the guest.
func (a *abiCallbacks) GetHeaderMap(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType) ([]internalwasm.HeaderPair, bool) {
	headers, active := a.headerMapForType(mapType)
	if !active || headers == nil {
		return nil, false
	}

	// Collect keys, then sort, then emit pairs in sorted-key order with
	// per-key value-order preserved.
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Pre-size pairs slice for the common case (1 value per key).
	pairs := make([]internalwasm.HeaderPair, 0, len(keys))
	for _, k := range keys {
		for _, v := range headers[k] {
			pairs = append(pairs, internalwasm.HeaderPair{Key: k, Value: v})
		}
	}
	return pairs, true
}

// GetHeaderMapValue returns the first value for the named key. Mirrors
// http.Header.Get (case-insensitive canonical key matching via
// CanonicalHeaderKey). Returns ("", false) for missing keys, deferred map
// types, or uncaptured sides.
//
// Note: pseudo-headers (:method/:path/:authority/:status) are NOT
// canonicalized by http.CanonicalHeaderKey (it leaves names starting with
// `:` untouched); we use direct map lookup to preserve the proxy-wasm
// expectation that guests pass keys as-stored (typically lowercase for
// pseudo-headers).
func (a *abiCallbacks) GetHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key string) (string, bool) {
	headers, active := a.headerMapForType(mapType)
	if !active || headers == nil {
		return "", false
	}
	// Try the canonical-key path (http.Header standard) first; fall back
	// to a direct map lookup for pseudo-headers (`:method` etc.) which
	// CanonicalHeaderKey does NOT canonicalize but which may be stored
	// verbatim.
	if v := headers.Get(key); v != "" {
		return v, true
	}
	if vs, ok := headers[key]; ok && len(vs) > 0 {
		return vs[0], true
	}
	return "", false
}

// AddHeaderMapValue appends a value to the named key. Multi-value semantics
// preserved (http.Header.Add). Returns WasmResultUnimplemented for deferred
// map types; WasmResultInternalFailure if the side is uncaptured (active
// type but no headers map — should-not-happen in production).
func (a *abiCallbacks) AddHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult {
	headers, active := a.headerMapForType(mapType)
	if !active {
		return abi.WasmResultUnimplemented
	}
	if headers == nil {
		return abi.WasmResultInternalFailure
	}
	headers.Add(key, value)
	return abi.WasmResultOk
}

// ReplaceHeaderMapValue replaces all values for the named key with a single
// value. Multi-value collapsed to one (http.Header.Set semantic). Returns
// WasmResultUnimplemented for deferred map types.
func (a *abiCallbacks) ReplaceHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult {
	headers, active := a.headerMapForType(mapType)
	if !active {
		return abi.WasmResultUnimplemented
	}
	if headers == nil {
		return abi.WasmResultInternalFailure
	}
	headers.Set(key, value)
	return abi.WasmResultOk
}

// RemoveHeaderMapValue removes all values for the named key. Returns
// WasmResultOk even if the key is absent (idempotent — matches upstream
// proxy-wasm semantics). WasmResultUnimplemented for deferred map types.
func (a *abiCallbacks) RemoveHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key string) abi.WasmResult {
	headers, active := a.headerMapForType(mapType)
	if !active {
		return abi.WasmResultUnimplemented
	}
	if headers == nil {
		return abi.WasmResultInternalFailure
	}
	headers.Del(key)
	return abi.WasmResultOk
}

// SetHeaderMapPairs replaces the ENTIRE header map with the given pairs.
// Existing keys are dropped; new keys added with the supplied values
// (multi-value preserved via http.Header.Add).
//
// Returns WasmResultUnimplemented for deferred map types; WasmResult
// InternalFailure for uncaptured sides (should-not-happen in production).
func (a *abiCallbacks) SetHeaderMapPairs(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, pairs []internalwasm.HeaderPair) abi.WasmResult {
	headers, active := a.headerMapForType(mapType)
	if !active {
		return abi.WasmResultUnimplemented
	}
	if headers == nil {
		return abi.WasmResultInternalFailure
	}
	// Clear the existing map IN-PLACE (the *filter holds a reference; we
	// must not swap the map pointer or the filter's view would be stale).
	for k := range headers {
		delete(headers, k)
	}
	for _, p := range pairs {
		headers.Add(p.Key, p.Value)
	}
	return abi.WasmResultOk
}

// GetHeaderMapSize returns the total VALUE count (not the unique-key
// count) — multi-value headers contribute their full value count. This
// matches the GetHeaderMap pair-emission shape (one pair per value), so
// guests using `GetHeaderMapSize` to size a buffer before `GetHeaderMap`
// get a consistent count.
//
// Returns 0 for deferred map types, uncaptured sides, or nil filter.
func (a *abiCallbacks) GetHeaderMapSize(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType) uint32 {
	headers, active := a.headerMapForType(mapType)
	if !active || headers == nil {
		return 0
	}
	var n uint32
	for _, vs := range headers {
		//nolint:gosec // header count is bounded by the http.Header invariants; will not exceed uint32.
		n += uint32(len(vs))
	}
	return n
}

// -----------------------------------------------------------------------------
// GetProperty + SetProperty (per 25.1 SPEC §5.1 hostcalls 11-12).
//
// Minimal property tree at 25.1 per the 25.1 SPEC §3.5 obligations:
//
//   - request.path        → request headers' `:path` pseudo-header
//   - request.method      → request headers' `:method` pseudo-header
//   - request.host        → request headers' `:authority` pseudo-header
//   - request.headers.<N> → request headers' named header (case-insensitive)
//   - response.headers.<N>→ response headers' named header (case-insensitive)
//
// All other paths return (nil, false) → WasmResultNotFound. The full CEL
// surface (connection.*, source.*, destination.*, request.body.*, route.*,
// xds.*, etc.) lands at 25.2 alongside the body/trailer state.
//
// SetProperty is a no-op WasmResultOk at 25.1 — the minimal tree is
// read-only; guests setting properties get silent success but the values
// are not persisted (25.2 adds the cross-filter property bucket).
// -----------------------------------------------------------------------------

// GetProperty implements the 25.1 minimal property-tree per the obligations
// above. Path-element matching is byte-exact (no case-folding); guests
// constructing paths follow the proxy-wasm spec which uses canonical lower-
// case path segments.
func (a *abiCallbacks) GetProperty(_ context.Context, _ uint32, path []string) ([]byte, bool) {
	if a.filter == nil || len(path) == 0 {
		return nil, false
	}

	switch path[0] {
	case "request":
		return a.getRequestProperty(path[1:])
	case "response":
		return a.getResponseProperty(path[1:])
	}
	return nil, false
}

// getRequestProperty handles the request.* subtree.
func (a *abiCallbacks) getRequestProperty(sub []string) ([]byte, bool) {
	if len(sub) == 0 {
		return nil, false
	}
	reqHeaders := a.filter.requestHeaders
	if reqHeaders == nil {
		return nil, false
	}

	switch sub[0] {
	case "path":
		return pseudoHeaderBytes(reqHeaders, ":path")
	case "method":
		return pseudoHeaderBytes(reqHeaders, ":method")
	case "host":
		return pseudoHeaderBytes(reqHeaders, ":authority")
	case "headers":
		if len(sub) < 2 {
			return nil, false
		}
		// Try both case-folded http.Header.Get and direct map lookup
		// (for non-canonical keys like pseudo-headers).
		if v := reqHeaders.Get(sub[1]); v != "" {
			return []byte(v), true
		}
		if vs, ok := reqHeaders[sub[1]]; ok && len(vs) > 0 {
			return []byte(vs[0]), true
		}
		return nil, false
	}
	return nil, false
}

// getResponseProperty handles the response.* subtree. At 25.1 only
// response.headers.<N> is supported; response.code is NOT exposed via
// property-tree at 25.1 (guests use proxy_get_status instead — which goes
// through GetStatus / the ADR-0196 D-P3 accessor).
func (a *abiCallbacks) getResponseProperty(sub []string) ([]byte, bool) {
	if len(sub) < 2 || sub[0] != "headers" {
		return nil, false
	}
	respHeaders := a.filter.responseHeaders
	if respHeaders == nil {
		return nil, false
	}
	if v := respHeaders.Get(sub[1]); v != "" {
		return []byte(v), true
	}
	if vs, ok := respHeaders[sub[1]]; ok && len(vs) > 0 {
		return []byte(vs[0]), true
	}
	return nil, false
}

// pseudoHeaderBytes reads a pseudo-header (`:method`/`:path`/`:authority`/
// `:status`) from an http.Header. http.CanonicalHeaderKey does NOT
// canonicalize names starting with `:`, so we use direct map lookup.
// Returns (nil, false) when the pseudo-header is absent.
func pseudoHeaderBytes(h http.Header, key string) ([]byte, bool) {
	if vs, ok := h[key]; ok && len(vs) > 0 {
		return []byte(vs[0]), true
	}
	return nil, false
}

// SetProperty is a 25.1 no-op returning WasmResultOk per 25.1 SPEC §3.5 —
// the minimal property tree is read-only; guests setting properties get
// silent success but the values are NOT persisted. The full property-tree
// surface (incl. set-side) lands at 25.2 with the cross-filter property
// bucket (mirrors lua's filter-state surface).
//
// Returning Ok (rather than Unimplemented) matches upstream proxy-wasm
// semantics for guests that do best-effort property writes; returning
// Unimplemented would surface as a guest-visible error on every set, which
// would break otherwise-working guests that opportunistically set
// properties without checking the result.
func (a *abiCallbacks) SetProperty(_ context.Context, _ uint32, _ []string, _ []byte) abi.WasmResult {
	// Intentional no-op at 25.1; 25.2 wires the property bucket.
	return abi.WasmResultOk
}

// -----------------------------------------------------------------------------
// SendLocalResponse (per 25.1 SPEC §5.1 hostcall 3).
//
// Captures the guest's send-local-response payload on the *filter; consumed
// at Task 12's post-CallProxyOnRequestHeaders / post-CallProxyOnResponse
// Headers check (non-nil sentLocalResponse → return StopIteration + invoke
// decoderCb.SendLocalReply / encoderCb-equivalent).
// -----------------------------------------------------------------------------

// SendLocalResponse captures the guest's local-response payload. Returns
// WasmResultInternalFailure if the filter is nil (defensive — should-not-
// happen in production). Otherwise WasmResultOk + populates the filter's
// sentLocalResponse field with the verbatim args.
//
// The captured payload is consumed at Task 12 after the CallProxyOn*
// dispatch returns; the dispatcher checks `f.sentLocalResponse != nil` and
// short-circuits to SendLocalReply per the REUSE 5 contract (parent §3.3).
func (a *abiCallbacks) SendLocalResponse(_ context.Context, _ uint32, statusCode uint32, statusMsg, body string, additionalHeaders []internalwasm.HeaderPair, grpcStatus int32) abi.WasmResult {
	if a.filter == nil {
		return abi.WasmResultInternalFailure
	}
	a.filter.sentLocalResponse = &capturedLocalResponse{
		statusCode:        statusCode,
		statusMsg:         statusMsg,
		body:              body,
		additionalHeaders: additionalHeaders,
		grpcStatus:        grpcStatus,
	}
	return abi.WasmResultOk
}

// -----------------------------------------------------------------------------
// GetStatus — D-P3 ADR-0196 first co-consumer (per 25.1 SPEC §5.1 hostcall 13).
//
// RE-CONSUMES EncoderFilterCallbacks.ResponseStatus() per ADR-0196 +
// D-P3 + R7. This is the SECOND co-consumer of the phase-23 framework
// primitive (phase-23 admission_control's encode.go is the FIRST consumer);
// the co-consumption RATIFIES the extraction discipline analogous to
// phase-22.2's first co-consumer of phase-20 internal/httpclient/.
//
// Semantic per upstream proxy-wasm v0.2.1:
//
//   - On the encode side with a known response status code: returns
//     (uint32(code), []byte("<code>"), true). The value-bytes is the
//     status code formatted as a decimal string (e.g. "200", "503") —
//     upstream guests treat the value-bytes as the "status string"
//     representation; envoy-go follows the conservative numeric format.
//
//   - On the decode side (encoderCb == nil) OR with an unset status
//     (code == 0): returns (0, nil, false) — the host wrapper at
//     registration.go converts to WasmResultNotFound (=1).
// -----------------------------------------------------------------------------

func (a *abiCallbacks) GetStatus(_ context.Context, _ uint32) (uint32, []byte, bool) {
	if a.filter == nil || a.filter.encoderCb == nil {
		// Decode-path or pre-EncodeHeaders dispatch — no encode-side
		// callback yet, so the status is unavailable. NotFound semantic.
		return 0, nil, false
	}
	// ADR-0196 first co-consumer: read via the framework accessor
	// (HCM dispatch seeded the value via chain.SetEncodeResponseStatus
	// BEFORE RunEncodeHeaders per the set-once-by-dispatch / read-via-
	// accessor discipline).
	statusCode := a.filter.encoderCb.ResponseStatus()
	if statusCode <= 0 {
		return 0, nil, false
	}
	//nolint:gosec // statusCode is bounded by the HTTP-status int range (100-599); will not exceed uint32.
	code := uint32(statusCode)
	statusBytes := []byte(strconv.FormatUint(uint64(code), 10))
	return code, statusBytes, true
}

// -----------------------------------------------------------------------------
// Log + GetLogLevel + GetCurrentTimeNanoseconds + SetEffectiveContext + Done.
// -----------------------------------------------------------------------------

// Log routes the guest's proxy_log invocation to the per-stream VM's log
// sink (vm.LogProxy → vm.logSink set at NewVM construction via WithLogSink).
// Defensive: degrades to no-op if filter is nil or vm is nil — the host
// hostcall body (registration.go line 187-195) treats the callback as
// fire-and-forget; this method MUST NOT panic.
//
// The level-name + message format ("[wasm <level>] <msg>\n") is applied
// by vm.LogProxy itself; this file just forwards the call.
func (a *abiCallbacks) Log(_ context.Context, _ uint32, level abi.LogLevel, msg string) {
	if a.filter == nil || a.filter.vm == nil {
		// Nil-tolerant per ADR-0085: log sink unavailable, drop the line.
		// This path is hit in test doubles + early-stream-lifecycle paths
		// where the vm has not yet been constructed.
		return
	}
	a.filter.vm.LogProxy(level, msg)
}

// GetLogLevel returns the active log level for the per-stream VM. At 25.1
// the simple default is LogLevelInfo — the wasm filter config does not (yet)
// expose a per-plugin log-level knob; 25.2 may add a `vm_config.log_level`
// field surfacing here. Guests use this to gate expensive log construction
// (formatting overhead for messages that would be dropped).
func (a *abiCallbacks) GetLogLevel(_ context.Context) abi.LogLevel {
	// Returning Info means the guest treats Trace/Debug as "would be
	// dropped" + skips expensive formatting; Info/Warn/Error/Critical
	// pass through to the host sink.
	return abi.LogLevelInfo
}

// GetCurrentTimeNanoseconds returns the wall-clock time in nanoseconds.
// DEPRECATED in proxy-wasm v0.2.1 (guests are encouraged to use
// wasi_snapshot_preview1.clock_time_get); implemented at 25.1 for upstream
// byte-faithfulness — guests built against older proxy-wasm SDKs still call
// this and would fail on a NotFound return.
//
// Returns `time.Now().UnixNano()` cast to uint64. The signed-to-unsigned
// conversion is well-defined for all times after 1970-01-01 (UnixNano is
// non-negative for present + future times).
func (a *abiCallbacks) GetCurrentTimeNanoseconds(_ context.Context) uint64 {
	//nolint:gosec // UnixNano is non-negative for post-1970 wall-clock times; the conversion is intentional.
	return uint64(time.Now().UnixNano())
}

// SetEffectiveContext is a 25.1 no-op acknowledgment returning WasmResultOk.
// The actual context-switching is performed at the VM level (registration.go
// line 458-461: vm.currentCtxID.Store(contextID) is called by the host
// hostcall wrapper AFTER this method returns Ok). Used by timer + httpCall
// callbacks (25.2 territory); at 25.1 the per-stream-only model has no
// alternative contexts to switch to, but the callback must succeed for
// guests that defensively call it before every cross-context interaction.
func (a *abiCallbacks) SetEffectiveContext(_ context.Context, _ uint32) abi.WasmResult {
	return abi.WasmResultOk
}

// Done is a 25.1 no-op acknowledgment returning WasmResultOk. Signals the
// guest has finished with the named context; the framework reaps the
// context at Task 12's OnDestroy via CallProxyOnDone. 25.2 may use this
// for httpCall + timer context teardown (the guest calls proxy_done() inside
// an httpCall response callback to release the deferred-context state).
func (a *abiCallbacks) Done(_ context.Context, _ uint32) abi.WasmResult {
	return abi.WasmResultOk
}

// (No additional internal helpers at 25.1 — the http.Header standard
// methods cover all per-side dispatch needs.)
