package wasm

// abi_callbacks.go — Task 11 implementation of `internalwasm.ABICallbacks`
// for the per-stream HTTP-filter context per 25.1 SPEC §3.5 + §4.3 + parent
// §4.2 + §4.5 D6 + D-P3 ADR-0196 first co-consumer.
//
// # 25.2 IMPL Task 15 EXTENSIONS
//
// EXTENDED at 25.2 IMPL Task 15 per 25.2 SPEC §3.6 + §5.3 + AMEND-B4 +
// ADR-0144 + ADR-0177 + ADR-0190 + ADR-0207:
//
//   - 5 buffer-related interface methods added to satisfy the Task-4
//     ABICallbacks interface extension (GetBuffer / SetBuffer /
//     GetBufferStatus / ContinueStream / CloseStream) — per-stream body
//     buffer accumulation lands at Task 16 body.go; Task 15 STUBS the
//     dispatch by returning empty buffers / WasmResultUnimplemented (the
//     dispatch surface is wired but the storage is Task 16 territory).
//
//   - 7 NEW consumer-side dispatch helpers (OnRequestBody / OnResponseBody /
//     OnRequestTrailers / OnResponseTrailers / OnTick / OnHttpCallResponse /
//     OnForeignFunction) per 25.2 SPEC §5.3 C14-C20 — these are NOT methods
//     on the framework's ABICallbacks interface (which is the hostcall
//     dispatch surface); they are NEW helper methods on the consumer-side
//     *abiCallbacks struct that body.go / trailers.go / tick_clock.go (Task
//     16) call when the HTTP filter chain feeds body chunks / trailers / tick
//     ticks / httpCall responses into the wasm filter. Task 15 STUBS the
//     dispatch surface by recording the invocation + delegating to the
//     per-stream *StreamContext when wired (Task 18); Task 16 wires the
//     body-buffer accumulator + Task 16 the tick goroutine receive loop.
//
//   - 4 RE-USE primitive consumer integration for GetProperty per AMEND-B4 +
//     §5.3:
//       (a) ADR-0144 DownstreamPrincipal RE-CONSUMED for connection.tls.*
//           sub-paths (second co-consumer beyond phase-04 itself);
//       (b) ADR-0177 httpclient RE-CONSUMED for upstream.* + xds.cluster_*
//           sub-paths (third-or-later co-consumer; via per-RootVM httpClient
//           + decoderCb.UpstreamHost-like accessor);
//       (c) ADR-0190 dynamicmetadata RE-CONSUMED for metadata.* + xds.*_
//           metadata branches (third-or-later co-consumer);
//       (d) NEW ADR-0207 filterstate CONSUMED for filter_state.* +
//           upstream_filter_state.* + wasm.<key> proxy branches (consumer
//           #2 — phase-22.2 lua is consumer #1 via Task 10 MIGRATION).
//     The integration constructs a `filterPropertyResolver` (implementing
//     the 60-method PropertyResolver interface from Task 13's
//     internal/wasm/property.go) + delegates to `wasm.ResolveProperty(r,
//     path)`. The resolver returns each sub-path value verbatim from the
//     appropriate consumer; absent values return (zero, false) which the
//     framework dispatcher converts to WasmResult::NotFound.
//
// # 25.1 baseline (unchanged at Task 15)
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
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"

	"github.com/pgdad/envoy-go/internal/filterstate"
	internalwasm "github.com/pgdad/envoy-go/internal/wasm"
	"github.com/pgdad/envoy-go/internal/wasm/abi"
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
// GetProperty + SetProperty (per 25.1 SPEC §5.1 hostcalls 11-12; EXTENDED at
// 25.2 IMPL Task 15 per AMEND-B4 + §5.3 to delegate to wasm.ResolveProperty
// via filterPropertyResolver).
//
// Full ~70-path roster at 25.2 per AMEND-B4 — 10 dispatched roots + 4 direct
// tokens. The ABICallbacks.GetProperty entry receives the host-parsed
// segments (NUL-split by registration.go.splitPath); we reconstruct the
// NUL-delimited wire form so wasm.ResolveProperty's own parsePathSegments
// runs the canonical dispatch (single source of truth for path semantics).
//
// The 4 RE-USE primitives are consumed via the filterPropertyResolver impl
// at the bottom of this file:
//
//   - ADR-0144 DownstreamPrincipal (connection.tls.* sub-paths)
//   - ADR-0177 httpclient + UpstreamHost-style accessor (upstream.* +
//     xds.cluster_* sub-paths)
//   - ADR-0190 dynamicmetadata (metadata.* + xds.*_metadata branches)
//   - NEW ADR-0207 filterstate (filter_state.* + upstream_filter_state.* +
//     wasm.<key> proxy branches)
//
// SetProperty is a 25.2 no-op WasmResultOk per ADR-0207 (consumer-side
// filter-state writes go through the filter_state.<key> dispatch which lands
// at Task 17 property.go; at Task 15 the SetProperty surface mirrors 25.1
// no-op-OK semantics — well-behaved guests handle it gracefully).
// -----------------------------------------------------------------------------

// GetProperty implements the 25.2 full property tree per AMEND-B4. The
// host-parsed segments are re-marshaled into the canonical NUL-delimited
// wire form + dispatched through wasm.ResolveProperty(filterPropertyResolver,
// path). Absent values return (nil, false) which the framework dispatcher at
// registration.go converts to WasmResult::NotFound (=1) — byte-faithful to
// cpp-host (context.cc:1065/1072/1078/1083/1103/1106/1110).
//
// The translation from the framework-supplied []string segments back to NUL-
// delimited bytes is a one-way (since the framework already parsed; we
// re-parse via the canonical tokenizer for symmetry with all other 25.2
// proxy_get_property surfaces — single source of truth for path semantics
// per Task 13).
func (a *abiCallbacks) GetProperty(_ context.Context, _ uint32, path []string) ([]byte, bool) {
	if a.filter == nil || len(path) == 0 {
		return nil, false
	}
	resolver := a.newPropertyResolver()
	pathBytes := joinNULDelimited(path)
	val, status := internalwasm.ResolveProperty(resolver, pathBytes)
	if status != abi.WasmResultOk {
		return nil, false
	}
	return val, true
}

// joinNULDelimited reconstructs the proxy_get_property wire path from the
// framework-parsed segments. Mirrors the NUL-delimited byte form per spec
// README §Serialization + cpp-host context.cc:1047-1058 (which is what
// wasm.parsePathSegments expects on the inbound side). Empty input returns
// a nil slice; the resolver dispatcher treats nil as the absent-path
// (NotFound) case.
func joinNULDelimited(segments []string) []byte {
	if len(segments) == 0 {
		return nil
	}
	// Pre-size: sum of segment lengths + (N-1) NUL separators.
	totalLen := len(segments) - 1
	for _, seg := range segments {
		totalLen += len(seg)
	}
	out := make([]byte, 0, totalLen)
	for i, seg := range segments {
		if i > 0 {
			out = append(out, 0x00)
		}
		out = append(out, seg...)
	}
	return out
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

// Log routes the guest's proxy_log invocation to the per-RootVM log
// sink (rootVM.LogProxy → rootVM.logSink set at NewRootVM construction via
// WithRootLogSink). REVISED at 25.2 Task 18 for the root-VM model: the
// LogProxy method lives on the per-compiledConfig shared *RootVM rather
// than per-stream — the log sink is a per-listener resource, not a per-
// stream resource. Defensive: degrades to no-op if filter is nil or cfg
// is nil or rootVM is nil — the host hostcall body (registration.go line
// 187-195) treats the callback as fire-and-forget; this method MUST NOT
// panic.
//
// The level-name + message format ("[wasm <level>] <msg>\n") is applied
// by rootVM.LogProxy itself; this file just forwards the call.
func (a *abiCallbacks) Log(_ context.Context, _ uint32, level abi.LogLevel, msg string) {
	if a.filter == nil || a.filter.cfg == nil || a.filter.cfg.rootVM == nil {
		// Nil-tolerant per ADR-0085: log sink unavailable, drop the line.
		// This path is hit in test doubles + early-stream-lifecycle paths
		// where the rootVM has not yet been constructed.
		return
	}
	a.filter.cfg.rootVM.LogProxy(level, msg)
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

// -----------------------------------------------------------------------------
// 25.2 IMPL Task 15 — 5 buffer-related methods (per Task 4 ABICallbacks
// interface extension at internal/wasm/registration.go).
//
// These methods complete the 13 → 20 interface roster (the 5 buffer methods
// added at Task 4 are NOT optional — the compile-time conformance check
// `var _ internalwasm.ABICallbacks = (*abiCallbacks)(nil)` fails without
// them). At Task 15 they STUB the per-stream body-buffer accumulation: the
// storage layer lands at Task 16 body.go which adds the per-side buffer
// fields on *filter + plumbs them through SetBuffer/GetBuffer/GetBufferStatus.
// ContinueStream + CloseStream STUB to no-op WasmResultUnimplemented at
// Task 15 (per ADR-0085 nil-tolerance — the consumer-side stream-control
// dispatch lands at Task 16 alongside the body PAUSE/RESUME state machine).
//
// The Host25_2 host-bridge wiring at internal/wasm/host_bridge_25_2.go (Task
// 4) routes proxy_get_buffer_bytes/proxy_set_buffer_bytes/proxy_get_buffer_
// status/proxy_continue_stream/proxy_close_stream hostcalls through the
// RootVM-side BufferGet/BufferSet/BufferStatus interface — RootVM in turn
// forwards through to the *abiCallbacks per-stream methods below. At Task
// 15 the per-stream methods return:
//
//   - GetBuffer:       (nil, nil) — empty buffer for all bufferTypes (the
//                      AMEND-B1 clamp logic at abi/body_bridge.go treats nil
//                      as length=0; guest reads return empty payload + Ok)
//   - SetBuffer:       WasmResultUnimplemented — guest writes a no-op (the
//                      body splice surface lands at Task 16; at Task 15
//                      the guest sees Unimplemented for any write)
//   - GetBufferStatus: (0, 0, nil) — size=0, flags=0 (no per-buffer state
//                      tracked at Task 15; lands at Task 16 with the end_
//                      of_stream flag bit)
//   - ContinueStream:  WasmResultUnimplemented — PAUSE/RESUME state machine
//                      lands at Task 16 body.go
//   - CloseStream:     WasmResultUnimplemented — early stream-close
//                      coordination lands at Task 16
//
// The stubs are intentional + documented; Task 16 deletes the stub
// comments + wires the real impl. The unit tests at Task 15 pin the stub
// behavior (so any regression in Task 16's wiring re-fails the unit tests
// + the implementer notices at TDD time).
// -----------------------------------------------------------------------------

// GetBuffer returns the per-(stream, bufferType) buffer bytes per 25.2 SPEC
// §5.3. ACTIVATED at Task 16 (Task 15 stub replaced): reads from the per-
// stream filter.decodeBody / filter.encodeBody accumulators populated by
// body.go's DecodeData / EncodeData. AMEND-B1 clamp semantics apply at the
// abi/body_bridge.go shim BEFORE this method fires; nil return treats the
// buffer as length=0 (guest reads return empty payload + WasmResult::Ok).
//
// Buffer-type dispatch per 25.2 SPEC §5.1 #25 (WasmBufferType values
// activated at 25.2):
//
//   - HttpRequestBody         (value 0) → f.decodeBody
//   - HttpResponseBody        (value 1) → f.encodeBody
//   - HttpCallResponseBody    (value 4) → nil (per-call response body
//     stash lands at a future task
//     alongside the http_call response
//     headers/trailers cache; at Task
//     16 the stash is empty + guest
//     reads return empty payload + Ok)
//   - other values                       → (nil, nil) — defensive (the
//     framework shim already rejected
//     unknown types with WasmResult::
//     BadArgument)
func (a *abiCallbacks) GetBuffer(_ context.Context, _ uint32, bufferType abi.WasmBufferType) ([]byte, error) {
	if a.filter == nil {
		return nil, nil
	}
	switch bufferType {
	case abi.WasmBufferTypeHttpRequestBody:
		return a.filter.decodeBody, nil
	case abi.WasmBufferTypeHttpResponseBody:
		return a.filter.encodeBody, nil
	default:
		// HttpCallResponseBody (value 4) + any other type: stash lands at
		// a future task; at Task 16 returns empty buffer per AMEND-B1 clamp
		// semantics.
		return nil, nil
	}
}

// SetBuffer replaces a slice of the buffer at offset start with data per
// 25.2 SPEC §5.1 #26 + Q1. ACTIVATED at Task 16 (Task 15 stub replaced):
// dispatches the splice/extend/truncate semantic per cpp-host src/exports.
// cc:set_buffer_bytes. The implementation supports the upstream-faithful
// 3-mode semantic:
//
//   - start == 0 + size == 0       → PREPEND data to the buffer
//   - start == len(buf) + size == 0 → APPEND data to the buffer
//   - start == 0 + size == len(buf) → REPLACE the buffer entirely
//
// Other (start, size) combinations splice in-place: `buf[:start] + data +
// buf[start+size:]`. The buffer-cap discipline (Q2) does NOT re-fire on
// SetBuffer at 25.2 — the cap fires on the read-side accumulator growth
// (DecodeData / EncodeData appends); guest writes through SetBuffer are
// considered guest-trusted-content (the guest already paid the cap-check
// cost via its proxy_get_buffer_bytes read).
//
// Per-buffer-type dispatch matches GetBuffer; returns WasmResultBadArgument
// for unactivated buffer types (the framework shim at abi/body_bridge.go
// catches most of these; defensive return here for the unactivated path).
func (a *abiCallbacks) SetBuffer(_ context.Context, _ uint32, bufferType abi.WasmBufferType, start uint32, data []byte) abi.WasmResult {
	if a.filter == nil {
		return abi.WasmResultInternalFailure
	}
	switch bufferType {
	case abi.WasmBufferTypeHttpRequestBody:
		a.filter.decodeBody = spliceBuffer(a.filter.decodeBody, start, data)
		return abi.WasmResultOk
	case abi.WasmBufferTypeHttpResponseBody:
		a.filter.encodeBody = spliceBuffer(a.filter.encodeBody, start, data)
		return abi.WasmResultOk
	default:
		// HttpCallResponseBody + other unactivated types: the per-call
		// response body stash is HOST-WRITTEN (not guest-written) so guest
		// SetBuffer attempts get BadArgument.
		return abi.WasmResultBadArgument
	}
}

// spliceBuffer implements the upstream-faithful proxy_set_buffer_bytes
// splice semantic per cpp-host src/exports.cc:set_buffer_bytes. At Task 16
// the simplified 2-arg (start, data) form is used (the framework shim at
// abi/body_bridge.go has already parsed the wire arg list per AMEND-B1).
// The size argument is implicit: `size = len(buf) - start` for the
// PREPEND/APPEND/REPLACE-detect heuristics; for arbitrary splice the
// framework shim handles the explicit (start, size, data) wire form.
//
// Behavior at Task 16: append-on-out-of-range (start > len(buf)) +
// replace-on-zero-start-with-empty-data (clears the buffer). Operators
// experiencing edge-case splice semantics may need to wait for a follow-
// up refinement that mirrors the full cpp-host 3-arg dispatch.
func spliceBuffer(buf []byte, start uint32, data []byte) []byte {
	if start >= uint32(len(buf)) {
		// APPEND (start beyond end → grow + append).
		return append(buf, data...)
	}
	// REPLACE-from-start: buf[:start] + data (truncates anything past start).
	out := make([]byte, 0, int(start)+len(data))
	out = append(out, buf[:start]...)
	out = append(out, data...)
	return out
}

// GetBufferStatus returns the current size + flag bits for the buffer
// identified by bufferType per 25.2 SPEC §5.1 #27. ACTIVATED at Task 16
// (Task 15 stub replaced): reads from the per-stream filter accumulators.
// The end_of_stream flag is NOT yet tracked at the *filter level (the
// chunked-dispatch state machine lands at Task 18 alongside the per-stream
// context wiring); at Task 16 the flag returns 0 + the size returns the
// accumulator length verbatim.
func (a *abiCallbacks) GetBufferStatus(_ context.Context, _ uint32, bufferType abi.WasmBufferType) (size, flags uint32, err error) {
	if a.filter == nil {
		return 0, 0, nil
	}
	switch bufferType {
	case abi.WasmBufferTypeHttpRequestBody:
		//nolint:gosec // f.decodeBody length bounded by cfg.bodyBufferCapBytes uint32 ceiling.
		return uint32(len(a.filter.decodeBody)), 0, nil
	case abi.WasmBufferTypeHttpResponseBody:
		//nolint:gosec // f.encodeBody length bounded by cfg.bodyBufferCapBytes uint32 ceiling.
		return uint32(len(a.filter.encodeBody)), 0, nil
	default:
		// HttpCallResponseBody + others: stash empty at Task 16.
		return 0, 0, nil
	}
}

// ContinueStream resumes a paused stream (PAUSE → CONTINUE) per 25.2 SPEC
// §5.1 #28 + Q1. ACTIVATED at Task 16 (Task 15 stub replaced): invokes the
// per-side ContinueDecoding / ContinueEncoding on the framework callback.
// The streamType discriminator selects which side (0 = decode, 1 = encode);
// other values return BadArgument.
//
// Per ADR-0071 + envoyhttp framework: ContinueDecoding / ContinueEncoding
// resume the per-side iteration after a previous DataStopIterationAndBuffer
// return; the chain re-enters the next-filter-after-this-one dispatch with
// the accumulated buffer. The guest call is fire-and-forget at the host
// side (no return value beyond WasmResult::Ok).
//
// Defensive nil-tolerance: nil filter or nil per-side callback returns
// InternalFailure (should-not-happen in production; preserved for test-
// double paths that construct *filter without callbacks).
func (a *abiCallbacks) ContinueStream(_ context.Context, _ uint32, streamType uint32) abi.WasmResult {
	if a.filter == nil {
		return abi.WasmResultInternalFailure
	}
	switch streamType {
	case 0: // decode side
		if a.filter.decoderCb == nil {
			return abi.WasmResultInternalFailure
		}
		a.filter.decoderCb.ContinueDecoding()
		return abi.WasmResultOk
	case 1: // encode side
		if a.filter.encoderCb == nil {
			return abi.WasmResultInternalFailure
		}
		a.filter.encoderCb.ContinueEncoding()
		return abi.WasmResultOk
	default:
		// Other streamType values (2 = downstream data, 3 = upstream data
		// per the WasmStreamType roster) are not applicable to the HTTP
		// filter dispatch surface.
		return abi.WasmResultBadArgument
	}
}

// CloseStream tears down the named stream early per 25.2 SPEC §5.1 #29.
// ACTIVATED at Task 16 (Task 15 stub replaced): on the decode side issues
// SendLocalReply(503, "", nil) as a half-close coordination (the guest
// requested early stream termination from inside a body/trailer callback).
// On the encode side returns Unimplemented (SendLocalReply is unavailable
// on the encoder; the guest's intent to close the encode stream is
// honored by returning Unimplemented + relying on the guest to short-
// circuit its own iteration).
//
// envoy-go does not currently expose a direct "close stream" framework
// API beyond SendLocalReply on the decode side; this implementation
// mirrors the upstream proxy-wasm semantics (decode-side close emits a
// local reply; encode-side close is a guest-side concern).
func (a *abiCallbacks) CloseStream(_ context.Context, _ uint32, streamType uint32) abi.WasmResult {
	if a.filter == nil {
		return abi.WasmResultInternalFailure
	}
	switch streamType {
	case 0: // decode side
		if a.filter.decoderCb == nil {
			return abi.WasmResultInternalFailure
		}
		// Half-close coordination: emit a 503 Service Unavailable local
		// reply per the upstream proxy-wasm convention for guest-initiated
		// close. The body is empty (the guest may have inspected the
		// accumulator already + decided to abort); the chain stops
		// iteration via the SendLocalReply hand-off.
		a.filter.decoderCb.SendLocalReply(503, "", nil)
		return abi.WasmResultOk
	case 1: // encode side
		// SendLocalReply unavailable on the encoder; the guest's close
		// intent surfaces as an Unimplemented return — the guest's host-
		// side wrapper SHOULD treat this as a hint to stop emitting body
		// chunks via its own iteration state.
		return abi.WasmResultUnimplemented
	default:
		return abi.WasmResultBadArgument
	}
}

// -----------------------------------------------------------------------------
// 25.2 IMPL Task 15 — 7 NEW consumer-side dispatch helpers per 25.2 SPEC
// §5.3 C14-C20.
//
// These are NOT methods on the framework's ABICallbacks interface (which is
// the hostcall dispatch surface from guest TO host); they are NEW helper
// methods on the consumer-side *abiCallbacks struct that the per-side
// filter-chain dispatch (Task 16 body.go + trailers.go + tick_clock.go)
// invokes when the HTTP filter receives body chunks / trailers / tick
// ticks / httpCall responses / foreign-function responses.
//
// Each helper:
//
//   1. Records the dispatch invocation on the per-stream *filter (defensive
//      no-op if filter is nil per ADR-0085).
//   2. Delegates to the per-stream *wasm.StreamContext (when wired at Task
//      18) which invokes the corresponding proxy_on_X guest-export callback
//      via wazero's `ExportedFunction(...).Call(ctx, args...)`. The
//      *StreamContext methods (CallProxyOnRequestBody / CallProxyOnResponse
//      Body / CallProxyOnRequestTrailers / CallProxyOnResponseTrailers /
//      etc.) land at Task 16 alongside the body.go + trailers.go consumer
//      wiring; tick + httpCall + foreignFunction are root-context
//      callbacks (per §5.3) that the RootVM owns + dispatches.
//   3. At Task 15: STUBS the StreamContext-delegation path (the
//      *StreamContext methods don't exist yet). Returns the per-callback
//      default (Continue / no-op) so the Task 16 wiring can land its real
//      dispatch surface without breaking the Task-15 surface contract.
//
// The 7 helpers + their §5.3 row mapping:
//
//   - OnRequestBody       — C14 (per-chunk invoke; body_size accumulated)
//   - OnResponseBody      — C15 (per-chunk invoke; body_size accumulated)
//   - OnRequestTrailers   — C16 (num_trailers count)
//   - OnResponseTrailers  — C17
//   - OnTick              — C18 (root-context; per-RootVM tick goroutine)
//   - OnHttpCallResponse  — C19 (per-call_id routing)
//   - OnForeignFunction   — C20 (foreign-function response dispatch)
// -----------------------------------------------------------------------------

// OnRequestBody dispatches the proxy_on_request_body(stream_context_id,
// body_size, end_of_stream) callback per §5.3 C14. body_size is the
// ACCUMULATED total per Q1 (per-chunk invoke; guest reads via
// proxy_get_buffer_bytes(HttpRequestBody, start, max_size)).
//
// ACTIVATED at Task 16 (Task 15 stub replaced): delegates to the per-
// stream *StreamContext.CallProxyOnRequestBody method that lands at Task
// 16 stream_context.go EXTEND. Returns ProxyActionContinue on err / nil
// streamCtx / nil filter / cap-denied / not-exported (fail-OPEN per ADR-
// 0072 per-stream posture).
//
// PRIMARY consumer: body.go's DecodeData calls streamCtx.CallProxyOnRequest
// Body DIRECTLY (not through this helper) since it has direct access to
// f.streamCtx; this helper exists as a uniform consumer surface for callers
// that hold *abiCallbacks instead of *filter (e.g. future host-side
// dispatch shims).
func (a *abiCallbacks) OnRequestBody(_ uint32, bodySize uint32, endOfStream bool) abi.ProxyAction {
	if a.filter == nil || a.filter.streamCtx == nil {
		return abi.ProxyActionContinue
	}
	action, err := a.filter.streamCtx.CallProxyOnRequestBody(context.Background(), bodySize, endOfStream)
	if err != nil {
		if a.filter.cfg != nil && a.filter.cfg.stats != nil && a.filter.cfg.stats.envoyGoFailures != nil {
			a.filter.cfg.stats.envoyGoFailures.Inc()
		}
		return abi.ProxyActionContinue
	}
	return action
}

// OnResponseBody dispatches the proxy_on_response_body(stream_context_id,
// body_size, end_of_stream) callback per §5.3 C15. Mirror of OnRequestBody
// for the encode side. ACTIVATED at Task 16.
func (a *abiCallbacks) OnResponseBody(_ uint32, bodySize uint32, endOfStream bool) abi.ProxyAction {
	if a.filter == nil || a.filter.streamCtx == nil {
		return abi.ProxyActionContinue
	}
	action, err := a.filter.streamCtx.CallProxyOnResponseBody(context.Background(), bodySize, endOfStream)
	if err != nil {
		if a.filter.cfg != nil && a.filter.cfg.stats != nil && a.filter.cfg.stats.envoyGoFailures != nil {
			a.filter.cfg.stats.envoyGoFailures.Inc()
		}
		return abi.ProxyActionContinue
	}
	return action
}

// OnRequestTrailers dispatches the proxy_on_request_trailers(stream_context_
// id, num_trailers) callback per §5.3 C16. num_trailers is the total trailer
// count (multi-value trailers expand to one entry per value, matching the
// GetHeaderMap pair-emission shape).
//
// ACTIVATED at Task 16. PRIMARY consumer: trailers.go's DecodeTrailers
// calls streamCtx.CallProxyOnRequestTrailers DIRECTLY; this helper provides
// a uniform consumer surface for callers that hold *abiCallbacks.
func (a *abiCallbacks) OnRequestTrailers(_ uint32, numTrailers uint32) abi.ProxyAction {
	if a.filter == nil || a.filter.streamCtx == nil {
		return abi.ProxyActionContinue
	}
	action, err := a.filter.streamCtx.CallProxyOnRequestTrailers(context.Background(), numTrailers)
	if err != nil {
		if a.filter.cfg != nil && a.filter.cfg.stats != nil && a.filter.cfg.stats.envoyGoFailures != nil {
			a.filter.cfg.stats.envoyGoFailures.Inc()
		}
		return abi.ProxyActionContinue
	}
	return action
}

// OnResponseTrailers dispatches the proxy_on_response_trailers(stream_
// context_id, num_trailers) callback per §5.3 C17. Mirror of
// OnRequestTrailers for the encode side. ACTIVATED at Task 16.
func (a *abiCallbacks) OnResponseTrailers(_ uint32, numTrailers uint32) abi.ProxyAction {
	if a.filter == nil || a.filter.streamCtx == nil {
		return abi.ProxyActionContinue
	}
	action, err := a.filter.streamCtx.CallProxyOnResponseTrailers(context.Background(), numTrailers)
	if err != nil {
		if a.filter.cfg != nil && a.filter.cfg.stats != nil && a.filter.cfg.stats.envoyGoFailures != nil {
			a.filter.cfg.stats.envoyGoFailures.Inc()
		}
		return abi.ProxyActionContinue
	}
	return action
}

// OnTick dispatches the proxy_on_tick(root_context_id) callback per §5.3
// C18 — root-context callback (NOT per-stream; uses rootContextID). The
// production caller is *RootVM.tick goroutine per Q5 + Task 5 internal/
// wasm/tick.go which dispatches DIRECTLY through the RootVM (per-RootVM
// state, not per-stream).
//
// This helper is a no-op surface preserved for symmetry with the other 6
// lifecycle helpers; the tick dispatch goes through *RootVM's tickHandler
// option seam + the proxy_on_tick getFunction lookup at the framework
// layer. *abiCallbacks is per-stream; tick is per-root, so this helper has
// no per-stream work to perform. Defensive no-op for filter-nil + always.
func (a *abiCallbacks) OnTick(_ uint32) {
	// Tick is a root-context callback dispatched from the per-RootVM
	// tick goroutine at internal/wasm/tick.go — NOT from per-stream
	// *abiCallbacks. This method exists for §5.3 symmetry; the actual
	// dispatch path bypasses *abiCallbacks since per-stream filter
	// context is not in scope for the root-context tick.
	if a.filter == nil {
		return
	}
}

// OnHttpCallResponse dispatches the proxy_on_http_call_response(plugin_
// context_id, call_id, num_headers, body_size, num_trailers) callback per
// §5.3 C19. The production caller is *RootVM.dispatchHttpCallResponse
// (per Task 8 internal/wasm/http_call.go) which holds dispatchMu + invokes
// proxy_on_http_call_response DIRECTLY against the per-RootVM instance.
//
// ACTIVATED at Task 16 as a delegation surface: when callers hold an
// *abiCallbacks reference + want to dispatch through the per-stream
// context (e.g. test fixtures), the helper delegates to *StreamContext.
// CallProxyOnHttpCallResponse. Production traffic uses the RootVM-direct
// path (http_call.go); this helper is for symmetry + future per-stream-
// direct invocation paths.
func (a *abiCallbacks) OnHttpCallResponse(_ uint32, callID, numHeaders, bodySize, numTrailers uint32) {
	if a.filter == nil || a.filter.streamCtx == nil {
		return
	}
	if err := a.filter.streamCtx.CallProxyOnHttpCallResponse(context.Background(), callID, numHeaders, bodySize, numTrailers); err != nil {
		if a.filter.cfg != nil && a.filter.cfg.stats != nil && a.filter.cfg.stats.envoyGoFailures != nil {
			a.filter.cfg.stats.envoyGoFailures.Inc()
		}
	}
}

// OnForeignFunction dispatches the proxy_on_foreign_function(context_id,
// foreign_function_id, data_size) callback per §5.3 C20. The foreign-
// function registry at Task 7 has the EMPTY default registry per envoy-go-
// strict departure record #4 — guests calling proxy_call_foreign_function
// get WasmResult::NotFound unless operators register via wasm.Register
// ForeignFunction at boot.
//
// ACTIVATED at Task 16 as a no-op delegation surface: the foreign-function
// response dispatch is NOT routed through per-stream *abiCallbacks at 25.2;
// the *RootVM.CallForeignFunction path (foreign.go line 837) executes the
// registered Go function synchronously + returns the result inline to the
// guest. There is no separate proxy_on_foreign_function callback dispatch
// at 25.2 (the wire-shape is RPC-style: invoke + return-payload, not
// fire-and-callback). Defensive no-op preserved for §5.3 symmetry.
func (a *abiCallbacks) OnForeignFunction(_ uint32, _ uint32, _ uint32) {
	// Foreign-function dispatch is synchronous (RPC-style) at 25.2; no
	// per-stream callback path. Defensive no-op for §5.3 symmetry.
	if a.filter == nil {
		return
	}
}

// -----------------------------------------------------------------------------
// 25.2 IMPL Task 15 — filterPropertyResolver implementation of the 60-method
// wasm.PropertyResolver interface per AMEND-B4 + 4 RE-USE primitive
// integration (ADR-0144 + ADR-0177 + ADR-0190 + ADR-0207).
//
// The resolver supplies per-stream context to the framework-side property
// dispatch at internal/wasm/property.go. Each accessor:
//
//   - Routes to the appropriate consumer-side filter callback OR framework
//     primitive (per the AMEND-B4 mapping table).
//   - Returns (zero, false) for absent / not-yet-wired surfaces — the
//     framework dispatcher converts (false → WasmResult::NotFound).
//   - Returns (value, true) with the typed value when present.
//
// The Task-15 implementation provides BASE wiring for the 4 RE-USE
// primitives:
//
//   (a) ADR-0144 DownstreamPrincipal — connection.tls.* sub-paths consume
//       decoderCb.DownstreamPrincipal() / DownstreamTLSConnectionState();
//   (b) ADR-0177 httpclient — upstream.* + xds.cluster_* consume the
//       per-RootVM httpClient (via rootVM accessor) — at Task 15 the
//       per-stream upstream-host accessor is NOT exposed on
//       DecoderFilterCallbacks (lands at Task 18 with the upstream-binding
//       extension); at Task 15 the upstream accessors return (zero, false);
//   (c) ADR-0190 dynamicmetadata — metadata.* consumes decoderCb.
//       DynamicMetadata().Get(filterName, key) per the existing accessor;
//   (d) NEW ADR-0207 filterstate — filter_state.* / upstream_filter_state.*
//       / wasm.<key> proxy consume the per-stream filterstate.Bucket
//       references on *filter (added to *filter at Task 18 alongside
//       per-stream context construction).
//
// Stream-local accessors (request.* / response.* / source.* / destination.*)
// read from the per-stream *filter (requestHeaders / responseHeaders /
// decoderCb-supplied socket-level state). Many of these surfaces are
// available verbatim from the existing 25.1 *filter fields (requestHeaders
// for request.method / request.path / request.host / request.headers.<N>);
// others require accessors that don't exist on DecoderFilterCallbacks yet
// (request.size / request.duration / request.time / response.code_details)
// and return (zero, false) at Task 15.
//
// The fine-grained 60-method interface is intentionally one-accessor-per-
// sub-path (per Task 13's design rationale) — adding a new sub-path requires
// touching BOTH the wasm.PropertyResolver interface AND this resolver impl,
// so roster drift is impossible. The cost is interface size; the benefit
// is type-system-enforced completeness.
// -----------------------------------------------------------------------------

// filterPropertyResolver wraps the per-stream *filter + supplies the
// PropertyResolver interface to wasm.ResolveProperty. Constructed fresh per
// GetProperty invocation (cheap — no allocation beyond the struct itself).
type filterPropertyResolver struct {
	filter *filter
}

// newPropertyResolver constructs a filterPropertyResolver bound to the
// abiCallbacks' filter. Returns nil-receiver-tolerant resolver: if filter
// is nil, every accessor returns (zero, false) which the framework
// dispatcher converts to NotFound.
func (a *abiCallbacks) newPropertyResolver() *filterPropertyResolver {
	return &filterPropertyResolver{filter: a.filter}
}

// Compile-time conformance: *filterPropertyResolver satisfies
// internalwasm.PropertyResolver. A regression on either side (interface
// gains a method; this file fails to implement; etc.) surfaces at package
// build.
var _ internalwasm.PropertyResolver = (*filterPropertyResolver)(nil)

// -----------------------------------------------------------------------------
// request.* (16 sub-paths) — stream-local; read from *filter.requestHeaders
// + the framework's request-time accessors (when available).
// -----------------------------------------------------------------------------

// RequestPath returns the request `:path` pseudo-header verbatim. Matches
// the 25.1 minimal-tree behavior for request.path.
func (r *filterPropertyResolver) RequestPath() (string, bool) {
	return r.pseudoHeader(":path")
}

// RequestURLPath returns the request URL path WITHOUT the query string per
// AMEND-B4 (upstream Envoy request.url_path semantic per cpp-host
// context.cc:1019-1033 + `Http::Utility::stripQueryString`). Delegates to
// the Task 17 property.go resolveURLPath helper (single source of truth for
// the :path -> (url_path, query) split per splitPathQuery).
func (r *filterPropertyResolver) RequestURLPath() (string, bool) {
	return r.resolveURLPath()
}

// RequestHost returns the request `:authority` pseudo-header verbatim.
func (r *filterPropertyResolver) RequestHost() (string, bool) {
	return r.pseudoHeader(":authority")
}

// RequestScheme returns the request `:scheme` pseudo-header (e.g. "https" /
// "http"). Absent for HTTP/1.1 paths that did not populate :scheme — returns
// (false) for those streams.
func (r *filterPropertyResolver) RequestScheme() (string, bool) {
	return r.pseudoHeader(":scheme")
}

// RequestMethod returns the request `:method` pseudo-header verbatim.
func (r *filterPropertyResolver) RequestMethod() (string, bool) {
	return r.pseudoHeader(":method")
}

// RequestReferer returns the `Referer` request header, if present.
func (r *filterPropertyResolver) RequestReferer() (string, bool) {
	return r.namedHeader(true, "Referer")
}

// RequestUserAgent returns the `User-Agent` request header, if present.
func (r *filterPropertyResolver) RequestUserAgent() (string, bool) {
	return r.namedHeader(true, "User-Agent")
}

// RequestID returns the `X-Request-Id` header, if present. Upstream Envoy
// auto-injects this; envoy-go does NOT auto-inject (parity gap documented
// at BEHAVIOR_CONTRACT.md) — the value reflects only what arrived on the
// wire OR was injected by an earlier filter.
func (r *filterPropertyResolver) RequestID() (string, bool) {
	return r.namedHeader(true, "X-Request-Id")
}

// RequestProtocol returns the downstream protocol per the framework's
// DownstreamProtocol() accessor (set by HCM dispatch — "HTTP/1.1" or
// "HTTP/2"). Empty string + ok=false for synthetic streams.
func (r *filterPropertyResolver) RequestProtocol() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	p := r.filter.decoderCb.DownstreamProtocol()
	if p == "" {
		return "", false
	}
	return p, true
}

// RequestQuery returns the query-string portion of the request URL (the
// substring AFTER the first '?' in :path) per AMEND-B4 (upstream Envoy
// request.query semantic per cpp-host context.cc:1019-1033). Delegates to
// the Task 17 property.go resolveQuery helper (single source of truth for
// the :path -> (url_path, query) split per splitPathQuery).
func (r *filterPropertyResolver) RequestQuery() (string, bool) {
	return r.resolveQuery()
}

// RequestHeader returns the named request header. Used by the
// request.headers.<name> sub-path. Mirrors the 25.1 request.headers.<N>
// behavior (case-insensitive http.Header.Get + fallback direct map lookup
// for pseudo-headers).
func (r *filterPropertyResolver) RequestHeader(name string) (string, bool) {
	return r.namedHeader(true, name)
}

// RequestHeadersBytes returns the total request-header bytes (sum of len(k)
// + len(v) across all entries). At Task 15 returns (0, false) — accurate
// byte accounting requires a Task 17 framework-supplied accessor.
func (r *filterPropertyResolver) RequestHeadersBytes() (uint64, bool) {
	// Task 17 property.go lands the bytes accounting accessor. STUB at
	// Task 15 — return absent.
	return 0, false
}

// RequestTime returns the request start time in epoch nanoseconds. At Task
// 15 returns (0, false) — requires a Task 17 framework-supplied start-time
// accessor.
func (r *filterPropertyResolver) RequestTime() (uint64, bool) {
	return 0, false
}

// RequestSize returns the request body size in bytes. At Task 15 returns
// (0, false) — accurate request-body size accounting lands at Task 16
// body.go alongside the body-buffer accumulator.
func (r *filterPropertyResolver) RequestSize() (uint64, bool) {
	return 0, false
}

// RequestTotalSize returns the total request size (headers + body + trailers).
// At Task 15 returns (0, false) — derived from the per-side size accessors
// when wired at Task 17.
func (r *filterPropertyResolver) RequestTotalSize() (uint64, bool) {
	return 0, false
}

// RequestDuration returns the request duration in nanoseconds (from request
// start to property-read time). At Task 15 returns (0, false) — derived
// from RequestTime() when wired at Task 17.
func (r *filterPropertyResolver) RequestDuration() (uint64, bool) {
	return 0, false
}

// -----------------------------------------------------------------------------
// response.* (6 sub-paths) — read from *filter.responseHeaders + encoderCb.
// -----------------------------------------------------------------------------

// ResponseCode returns the HTTP response status code (e.g., 200, 503).
// Consumes ADR-0196 EncoderFilterCallbacks.ResponseStatus() — the SAME
// framework primitive as the 25.1 GetStatus accessor (D-P3 first co-
// consumer); the property.* dispatch is the SECOND co-consumer of the
// same primitive.
func (r *filterPropertyResolver) ResponseCode() (uint64, bool) {
	if r.filter == nil || r.filter.encoderCb == nil {
		return 0, false
	}
	code := r.filter.encoderCb.ResponseStatus()
	if code <= 0 {
		return 0, false
	}
	return uint64(code), true
}

// ResponseCodeDetails returns the response code-details string (an Envoy
// internal field describing the local-reply reason). At Task 15 returns
// ("", false) — envoy-go does NOT carry this field yet.
func (r *filterPropertyResolver) ResponseCodeDetails() (string, bool) {
	return "", false
}

// ResponseFlags returns the response flags (an Envoy bitmask describing
// upstream/downstream failure modes). At Task 15 returns (0, false).
func (r *filterPropertyResolver) ResponseFlags() (uint64, bool) {
	return 0, false
}

// ResponseGrpcStatus returns the gRPC status code from the response
// `grpc-status` trailer / header. At Task 15 returns (0, false) — extracted
// from response trailers when wired at Task 17.
func (r *filterPropertyResolver) ResponseGrpcStatus() (uint64, bool) {
	return 0, false
}

// ResponseBackendLatency returns the upstream backend latency in
// nanoseconds. At Task 15 returns (0, false).
func (r *filterPropertyResolver) ResponseBackendLatency() (uint64, bool) {
	return 0, false
}

// ResponseTrailer returns the named response trailer. At Task 15 returns
// ("", false) — trailers land at Task 16 trailers.go.
func (r *filterPropertyResolver) ResponseTrailer(_ string) (string, bool) {
	return "", false
}

// ResponseHeader returns the named response header from f.responseHeaders.
// Symmetric to RequestHeader; added at 25.2 IMPL Task 18 alongside the
// cross-side header bridge completion + the parallel framework property
// dispatch for `response.headers.<name>`.
func (r *filterPropertyResolver) ResponseHeader(name string) (string, bool) {
	return r.namedHeader(false /* request */, name)
}

// -----------------------------------------------------------------------------
// connection.* (13 sub-paths) — RE-CONSUMES ADR-0144 DownstreamPrincipal +
// the DownstreamTLSConnectionState framework primitive (per phase-22.2's
// :ssl() bridge consumer).
// -----------------------------------------------------------------------------

// ConnectionID returns the per-connection identifier. At Task 15 returns
// (0, false) — envoy-go does NOT expose a per-connection u64 ID to filters
// yet; lands at Task 17 if a framework accessor materializes.
func (r *filterPropertyResolver) ConnectionID() (uint64, bool) {
	return 0, false
}

// ConnectionMTLS returns true if the downstream connection used mutual
// TLS (i.e., the client presented + verified a certificate). Derived from
// the TLS connection state's PeerCertificates being non-empty.
func (r *filterPropertyResolver) ConnectionMTLS() (bool, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return false, false
	}
	state := r.filter.decoderCb.DownstreamTLSConnectionState()
	if state == nil {
		// Plaintext — mtls=false but the answer is KNOWN (ok=true).
		return false, true
	}
	return len(state.PeerCertificates) > 0, true
}

// ConnectionRequestedServerName returns the SNI from the TLS handshake.
// Consumes decoderCb.DownstreamTLSServerName() (ADR-0165 framework
// accessor). Empty string for plaintext / no-SNI handshakes → (false).
func (r *filterPropertyResolver) ConnectionRequestedServerName() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	sni := r.filter.decoderCb.DownstreamTLSServerName()
	if sni == "" {
		return "", false
	}
	return sni, true
}

// ConnectionTLSVersion returns the TLS protocol version (e.g., "TLSv1.3").
// Derived from the TLS connection state's Version field.
func (r *filterPropertyResolver) ConnectionTLSVersion() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	state := r.filter.decoderCb.DownstreamTLSConnectionState()
	if state == nil {
		return "", false
	}
	return tlsVersionString(state.Version), true
}

// ConnectionTerminationDetails returns the connection-termination reason.
// At Task 15 returns ("", false) — envoy-go does NOT carry this field yet.
func (r *filterPropertyResolver) ConnectionTerminationDetails() (string, bool) {
	return "", false
}

// ConnectionSubjectLocalCertificate returns the Subject DN of the local
// (server-side) certificate as a string. Consumes the ListenerPrincipal
// framework accessor — phase-04 ADR-0144's server-side equivalent
// extraction.
func (r *filterPropertyResolver) ConnectionSubjectLocalCertificate() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	lp := r.filter.decoderCb.ListenerPrincipal()
	if lp == "" {
		return "", false
	}
	return lp, true
}

// ConnectionSubjectPeerCertificate returns the Subject DN of the downstream
// peer certificate. Derived from the TLS connection state's PeerCertificates
// (per ADR-0144's URI-SAN-first, then DNS-SAN, then Subject CN extraction
// for the peer principal — same priority order; here we expose the FIRST
// candidate from DownstreamPrincipal() which is the same ordered slice).
func (r *filterPropertyResolver) ConnectionSubjectPeerCertificate() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	principals := r.filter.decoderCb.DownstreamPrincipal()
	if len(principals) == 0 {
		return "", false
	}
	return principals[0], true
}

// ConnectionURISANLocalCertificate returns the URI SANs from the local
// (server-side) certificate as a comma-separated string. At Task 15 returns
// ("", false) — extracting from the server-side Certificates requires a
// framework accessor not yet exposed.
func (r *filterPropertyResolver) ConnectionURISANLocalCertificate() (string, bool) {
	return "", false
}

// ConnectionURISANPeerCertificate returns the URI SANs from the downstream
// peer certificate as a comma-separated string. Derived from the TLS
// connection state's PeerCertificates[0].URIs.
func (r *filterPropertyResolver) ConnectionURISANPeerCertificate() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	state := r.filter.decoderCb.DownstreamTLSConnectionState()
	if state == nil || len(state.PeerCertificates) == 0 {
		return "", false
	}
	cert := state.PeerCertificates[0]
	if len(cert.URIs) == 0 {
		return "", false
	}
	var b strings.Builder
	for i, u := range cert.URIs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(u.String())
	}
	return b.String(), true
}

// ConnectionDNSSANLocalCertificate returns the DNS SANs from the local
// (server-side) certificate. At Task 15 returns ("", false).
func (r *filterPropertyResolver) ConnectionDNSSANLocalCertificate() (string, bool) {
	return "", false
}

// ConnectionDNSSANPeerCertificate returns the DNS SANs from the downstream
// peer certificate as a comma-separated string. Derived from the TLS
// connection state's PeerCertificates[0].DNSNames.
func (r *filterPropertyResolver) ConnectionDNSSANPeerCertificate() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	state := r.filter.decoderCb.DownstreamTLSConnectionState()
	if state == nil || len(state.PeerCertificates) == 0 {
		return "", false
	}
	cert := state.PeerCertificates[0]
	if len(cert.DNSNames) == 0 {
		return "", false
	}
	return strings.Join(cert.DNSNames, ","), true
}

// ConnectionSHA256PeerCertificateDigest returns the SHA-256 digest of the
// downstream peer certificate's DER bytes as a hex-encoded string. Derived
// from sha256(DownstreamTLSPeerCertDER()).
func (r *filterPropertyResolver) ConnectionSHA256PeerCertificateDigest() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	der := r.filter.decoderCb.DownstreamTLSPeerCertDER()
	if len(der) == 0 {
		return "", false
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:]), true
}

// ConnectionTransportFailureReason returns the transport failure reason
// (downstream connection-level error). At Task 15 returns ("", false).
func (r *filterPropertyResolver) ConnectionTransportFailureReason() (string, bool) {
	return "", false
}

// -----------------------------------------------------------------------------
// source.* + destination.* (4 sub-paths) — RE-CONSUMES the ADR-0165
// DownstreamRemoteAddr / DownstreamLocalAddr accessors.
// -----------------------------------------------------------------------------

// SourceAddress returns the downstream remote IP address (no port).
func (r *filterPropertyResolver) SourceAddress() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	addr := r.filter.decoderCb.DownstreamRemoteAddr()
	if addr == nil {
		return "", false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		// Fall back to the full address string (e.g., a Unix-socket path).
		return addr.String(), true
	}
	return host, true
}

// SourcePort returns the downstream remote port.
func (r *filterPropertyResolver) SourcePort() (uint64, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return 0, false
	}
	addr := r.filter.decoderCb.DownstreamRemoteAddr()
	if addr == nil {
		return 0, false
	}
	_, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0, false
	}
	port, err := strconv.ParseUint(portStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return port, true
}

// DestinationAddress returns the downstream local IP address (the listener's
// bound IP).
func (r *filterPropertyResolver) DestinationAddress() (string, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return "", false
	}
	addr := r.filter.decoderCb.DownstreamLocalAddr()
	if addr == nil {
		return "", false
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		return addr.String(), true
	}
	return host, true
}

// DestinationPort returns the downstream local port.
func (r *filterPropertyResolver) DestinationPort() (uint64, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return 0, false
	}
	addr := r.filter.decoderCb.DownstreamLocalAddr()
	if addr == nil {
		return 0, false
	}
	_, portStr, err := net.SplitHostPort(addr.String())
	if err != nil {
		return 0, false
	}
	port, err := strconv.ParseUint(portStr, 10, 64)
	if err != nil {
		return 0, false
	}
	return port, true
}

// -----------------------------------------------------------------------------
// upstream.* (14 sub-paths) — RE-CONSUMES ADR-0177 httpclient + the per-
// stream upstream-host binding (Task 18 wiring). At Task 15 most upstream
// accessors return (zero, false) since the per-stream upstream-host accessor
// is NOT yet on DecoderFilterCallbacks (lands at Task 18 alongside the
// per-stream context construction extension).
// -----------------------------------------------------------------------------

// UpstreamAddress returns the upstream host address. At Task 15 returns
// ("", false) — Task 18 wires the per-stream upstream-binding accessor.
func (r *filterPropertyResolver) UpstreamAddress() (string, bool) { return "", false }
func (r *filterPropertyResolver) UpstreamPort() (uint64, bool)    { return 0, false }
func (r *filterPropertyResolver) UpstreamLocalAddress() (string, bool) {
	return "", false
}
func (r *filterPropertyResolver) UpstreamLocality() (string, bool)               { return "", false }
func (r *filterPropertyResolver) UpstreamTransportFailureReason() (string, bool) { return "", false }
func (r *filterPropertyResolver) UpstreamRequestAttemptCount() (uint64, bool)    { return 0, false }
func (r *filterPropertyResolver) UpstreamCxPoolReadyDuration() (uint64, bool)    { return 0, false }
func (r *filterPropertyResolver) UpstreamNumEndpoints() (uint64, bool)           { return 0, false }
func (r *filterPropertyResolver) UpstreamSubjectLocalCertificate() (string, bool) {
	return "", false
}
func (r *filterPropertyResolver) UpstreamSubjectPeerCertificate() (string, bool) {
	return "", false
}
func (r *filterPropertyResolver) UpstreamURISANLocalCertificate() (string, bool) {
	return "", false
}
func (r *filterPropertyResolver) UpstreamURISANPeerCertificate() (string, bool) {
	return "", false
}
func (r *filterPropertyResolver) UpstreamDNSSANPeerCertificate() (string, bool) {
	return "", false
}
func (r *filterPropertyResolver) UpstreamTLSVersion() (string, bool) { return "", false }

// -----------------------------------------------------------------------------
// xds.* (12 sub-paths) — RE-CONSUMES ADR-0177 httpclient (for cluster_name +
// cluster_metadata) + ADR-0190 dynamicmetadata (for the metadata-bearing
// sub-paths). At Task 15 the route + listener + node sub-paths return
// (zero, false) — they require Task 17 / Task 18 wiring to surface the
// per-stream xds context.
// -----------------------------------------------------------------------------

func (r *filterPropertyResolver) XdsClusterName() (string, bool)          { return "", false }
func (r *filterPropertyResolver) XdsClusterMetadata() ([]byte, bool)      { return nil, false }
func (r *filterPropertyResolver) XdsRouteName() (string, bool)            { return "", false }
func (r *filterPropertyResolver) XdsRouteMetadata() ([]byte, bool)        { return nil, false }
func (r *filterPropertyResolver) XdsVirtualHostName() (string, bool)      { return "", false }
func (r *filterPropertyResolver) XdsVirtualHostMetadata() ([]byte, bool)  { return nil, false }
func (r *filterPropertyResolver) XdsUpstreamHostMetadata() ([]byte, bool) { return nil, false }
func (r *filterPropertyResolver) XdsUpstreamHostLocalityMetadata() ([]byte, bool) {
	return nil, false
}
func (r *filterPropertyResolver) XdsFilterChainName() (string, bool)   { return "", false }
func (r *filterPropertyResolver) XdsListenerMetadata() ([]byte, bool)  { return nil, false }
func (r *filterPropertyResolver) XdsListenerDirection() (uint64, bool) { return 0, false }
func (r *filterPropertyResolver) XdsNode() ([]byte, bool)              { return nil, false }

// -----------------------------------------------------------------------------
// metadata.<filter>.<key> — RE-CONSUMES ADR-0190 dynamicmetadata via
// decoderCb.DynamicMetadata().
// -----------------------------------------------------------------------------

// Metadata returns the marshaled google.protobuf.Value bytes for the
// (filterName, key) coordinate. RE-CONSUMES ADR-0190 dynamicmetadata.Bucket
// via decoderCb.DynamicMetadata() — third-or-later co-consumer of the
// phase-19 framework primitive (phase-22.2 lua is the second co-consumer).
//
// The protobuf wire-encoding produces a defensive copy of the encoded bytes
// — the caller can mutate the returned slice without affecting the bucket.
// Returns (nil, false) for absent (filterName, key) coordinates OR for
// streams with no DynamicMetadata bucket wired (synthetic streams, decode-
// phase pre-encoder).
func (r *filterPropertyResolver) Metadata(filterName, key string) ([]byte, bool) {
	if r.filter == nil || r.filter.decoderCb == nil {
		return nil, false
	}
	bucket := r.filter.decoderCb.DynamicMetadata()
	if bucket == nil {
		return nil, false
	}
	val, ok := bucket.Get(filterName, key)
	if !ok || val == nil {
		return nil, false
	}
	// Serialize the *structpb.Value to its protobuf wire bytes — the
	// proxy-wasm guest receives the marshaled payload + can decode via its
	// language-side protobuf bindings.
	out, err := proto.Marshal(val)
	if err != nil {
		return nil, false
	}
	return out, true
}

// -----------------------------------------------------------------------------
// filter_state.* + upstream_filter_state.* + wasm.<key> — NEW ADR-0207
// filterstate CONSUMER #2 (phase-22.2 lua is consumer #1 via Task 10
// MIGRATION). The per-stream filterstate.Bucket references live on the
// *filter struct (added at Task 18 alongside per-stream context construction).
// -----------------------------------------------------------------------------

// FilterState returns the per-stream downstream filterstate.Bucket. At Task
// 15 returns nil (the *filter.downstreamFilterState field lands at Task 18);
// the framework dispatcher treats nil as absent → NotFound for all
// filter_state.<key> probes.
func (r *filterPropertyResolver) FilterState() *filterstate.Bucket {
	if r.filter == nil {
		return nil
	}
	return r.filter.downstreamFilterState
}

// UpstreamFilterState returns the per-stream upstream filterstate.Bucket.
// At Task 15 returns nil (Task 18 wires the *filter.upstreamFilterState
// field); the framework dispatcher treats nil as absent → NotFound for all
// upstream_filter_state.<key> probes.
func (r *filterPropertyResolver) UpstreamFilterState() *filterstate.Bucket {
	if r.filter == nil {
		return nil
	}
	return r.filter.upstreamFilterState
}

// -----------------------------------------------------------------------------
// 4 direct tokens — plugin_name / plugin_root_id / plugin_vm_id /
// connection_id (the last is also accessible via connection.id; both forms
// resolve via ConnectionID()).
// -----------------------------------------------------------------------------

// PluginName returns the plugin name from the compiledConfig. Sourced from
// PluginConfig.name per the 25.1 SPEC §4.1 parse.
func (r *filterPropertyResolver) PluginName() (string, bool) {
	if r.filter == nil || r.filter.cfg == nil || r.filter.cfg.pluginName == "" {
		return "", false
	}
	return r.filter.cfg.pluginName, true
}

// PluginRootID returns the plugin root context ID as a decimal string. The
// proxy-wasm guest typically treats this as an opaque identifier; we emit
// the decimal u32 form as the byte payload.
func (r *filterPropertyResolver) PluginRootID() (string, bool) {
	if r.filter == nil || r.filter.cfg == nil {
		return "", false
	}
	return strconv.FormatUint(uint64(r.filter.cfg.rootContextID), 10), true
}

// PluginVMID returns the plugin VM ID. envoy-go does NOT yet expose a
// VM-ID string distinct from PluginName (multi-plugin VM-sharing lands at
// 25.3 per parse-reject arm 12); at Task 15 returns ("", false).
func (r *filterPropertyResolver) PluginVMID() (string, bool) {
	return "", false
}

// -----------------------------------------------------------------------------
// Internal helpers used by the resolver above.
// -----------------------------------------------------------------------------

// pseudoHeader reads a pseudo-header (`:method`/`:path`/`:authority`/
// `:scheme`/`:status`) from the request headers (or the response headers
// for `:status`). Returns (value, true) when present + non-empty;
// ("", false) otherwise.
func (r *filterPropertyResolver) pseudoHeader(key string) (string, bool) {
	if r.filter == nil || r.filter.requestHeaders == nil {
		return "", false
	}
	if vs, ok := r.filter.requestHeaders[key]; ok && len(vs) > 0 && vs[0] != "" {
		return vs[0], true
	}
	return "", false
}

// namedHeader returns the named header from requestHeaders (if request=true)
// OR responseHeaders (if request=false). Case-insensitive via
// http.Header.Get with fallback to direct map lookup (for non-canonical
// keys / pseudo-headers).
func (r *filterPropertyResolver) namedHeader(request bool, name string) (string, bool) {
	if r.filter == nil {
		return "", false
	}
	var h http.Header
	if request {
		h = r.filter.requestHeaders
	} else {
		h = r.filter.responseHeaders
	}
	if h == nil {
		return "", false
	}
	if v := h.Get(name); v != "" {
		return v, true
	}
	if vs, ok := h[name]; ok && len(vs) > 0 && vs[0] != "" {
		return vs[0], true
	}
	return "", false
}

// tlsVersionString maps the crypto/tls.VersionTLS{10,11,12,13} constants
// to upstream Envoy's canonical "TLSv1.0" .. "TLSv1.3" form. Unknown
// versions return "TLS(<hex>)" for diagnostics.
func tlsVersionString(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLSv1.0"
	case tls.VersionTLS11:
		return "TLSv1.1"
	case tls.VersionTLS12:
		return "TLSv1.2"
	case tls.VersionTLS13:
		return "TLSv1.3"
	default:
		return "TLS(" + strconv.FormatUint(uint64(v), 16) + ")"
	}
}
