// Package abi — http_call.go: proxy_http_call hostcall dispatch shim per
// 25.2 SPEC §5.1 #37 + §11.3 D-25.2-3 + Q4 + R-25.2-3 + AMEND-B3 + ADR-0177
// RE-CONSUMER (3rd-or-later co-consumer; phase-22.2 lua :httpCall() was
// second; CLOSES parent SPEC §13-R6 RATIFIED-PENDING-IMPL anchor).
//
// # The single host shim landed at Task 8
//
//   - HttpCallShim — proxy_http_call(cluster_data, cluster_size,
//     headers_data, headers_size, body_data, body_size, trailers_data,
//     trailers_size, timeout_ms, ret_call_id_ptr) -> WasmResult. Reads
//     (cluster, headers, body, trailers) from guest memory + delegates to
//     the per-`*RootVM` httpCallHost.DispatchHttpCall via the
//     CurrentCtxID-tracked stream context; on Ok writes the allocated
//     call_id back to guest memory at ret_call_id_ptr.
//
// # Wire-shape (cpp-host include/proxy-wasm/exports.h + src/exports.cc:664-687)
//
//	Word http_call(Word uri_ptr, Word uri_size,
//	                Word header_pairs_ptr, Word header_pairs_size,
//	                Word body_ptr, Word body_size,
//	                Word trailer_pairs_ptr, Word trailer_pairs_size,
//	                Word timeout_milliseconds, Word token_ptr)
//
// All 10 args are i32 per proxy-wasm v0.2.1 spec. Return is WasmResult.
//
// # BadArgument-on-unknown-cluster per Q4 + AMEND-B3
//
// envoy-go's HTTPDispatcher.HasCluster check fires up-front; lookup miss
// returns WasmResult::BadArgument (=2) byte-faithful to upstream Envoy
// v1.37.2 `context.cc:1547-1550`. The
// `wasm.<plugin>.http_call_dispatch_unknown_cluster` envoy-go-strict counter
// increments on this path (counter wiring deferred Task 17).
//
// # Cancel-at-destruction per AMEND-B3 + cpp-host context.cc:1900-1905
//
// On StreamContext.Close, the per-*RootVM cancelOutstandingHttpCalls walks
// the httpCalls map + cancels every entry whose originating streamCtxID
// matches; the dispatch goroutines observe ctx-cancellation + their
// handleHttpCallResponse path early-returns via the token-miss guard.
// The defensive `wasm.<plugin>.http_call_response_after_close` envoy-go-
// strict counter increments on stray late-response arrivals (counter
// wiring deferred Task 17).
//
// # Concurrency contract per D-P-PLAN-9 (LOAD-BEARING)
//
// The shim is invoked from inside the per-*RootVM dispatchMu-held hostcall
// body frame (the registration.go envelope acquires dispatchMu via the per-
// *StreamContext.CallProxyOnX path). The shim delegates to
// httpCallHost.DispatchHttpCall which allocates the call_id + spawns a
// dispatch goroutine; the goroutine's handleHttpCallResponse re-acquires
// dispatchMu before invoking proxy_on_http_call_response on the originating
// stream context.
//
// # Decoupling from internal/wasm
//
// abi/ MUST NOT import internal/wasm (would induce a cycle — wasm imports
// abi). The shim accepts a `Host25_2 any` parameter (declared in
// stubs_25_2.go) carrying an opaque value the registration.go envelope
// passes through unchanged (typically `*RootVM`). The shim type-asserts to
// a package-private `httpCallHost` interface; non-satisfaction returns
// WasmResultInternalFailure.
package abi

import (
	"context"

	"github.com/tetratelabs/wazero/api"
)

// httpCallHost is the package-private interface the http_call shim
// dispatches through. Satisfied by `*wasm.RootVM` (via the DispatchHttpCall
// method on the RootVM type — see internal/wasm/http_call.go + the adapter
// methods at host_bridge_25_2.go) so the registration.go envelope can
// forward its `vm` argument unchanged. The shim type-asserts the opaque
// Host25_2 argument to this interface; non-satisfaction returns
// WasmResultInternalFailure.
//
// D-P-PLAN-9 contract: DispatchHttpCall is invoked synchronously inside
// the per-stream dispatchMu-held call frame; it MUST NOT re-acquire
// dispatchMu (sync.Mutex is non-reentrant; would deadlock). The host-side
// httpCalls-map mutation acquires a SEPARATE mutex (httpCallsMu); lock
// order is dispatchMu → httpCallsMu.
type httpCallHost interface {
	// CurrentCtxID returns the in-flight stream context id for the
	// hostcall dispatch (the same atomic Uint32 the 25.1 hostcalls read
	// inline as `vm.currentCtxID.Load()`). Adapter shared with the
	// bufferHost + streamHost + foreignHost + sharedDataHost interfaces.
	CurrentCtxID() uint32

	// DispatchHttpCall25_2 starts an outbound HTTP request via the
	// configured HTTPDispatcher and returns the allocated call_id. Response
	// routes asynchronously to proxy_on_http_call_response on the
	// originating stream context. Returns:
	//   - (callID, WasmResultOk) on successful dispatch start;
	//   - (0, WasmResultBadArgument) on unknown cluster (Q4 + AMEND-B3);
	//   - (0, WasmResultInternalFailure) on no-dispatcher-wired (host
	//     misconfiguration) or buildHttpCallRequest failure.
	//
	// The "25_2" suffix on the method name distinguishes it from the
	// plain `DispatchHttpCall` that *RootVM defines at http_call.go (which
	// takes the wasm-package HeaderPair). The adapter on *RootVM at
	// host_bridge_25_2.go converts between the two header-pair shapes via
	// per-pair iteration.
	DispatchHttpCall25_2(ctx context.Context, streamCtxID uint32, cluster string, headers []HeaderPair25_2, body []byte, trailers []HeaderPair25_2, timeoutMs uint32) (uint32, WasmResult)
}

// HeaderPair25_2 is the abi-package-local mirror of the wasm-package
// HeaderPair struct. Defined here (rather than imported from internal/wasm)
// to keep the abi → wasm decoupling invariant (abi MUST NOT import wasm).
// The structural-match between HeaderPair25_2 + wasm.HeaderPair lets the
// httpCallHost interface satisfy *RootVM via a thin pairs-conversion
// adapter at host_bridge_25_2.go.
//
// The struct shape is byte-identical to wasm.HeaderPair (Key + Value
// string fields; alphabetical order; no tags). Cross-package conversion
// is via per-pair iteration (we cannot direct-cast because Go strict-typing
// treats the two structs as distinct types even with identical layout).
type HeaderPair25_2 struct {
	Key   string
	Value string
}

// HttpCallShim — proxy_http_call dispatch per §5.1 #37 + §11.3 D-25.2-3 +
// AMEND-B3 + Q4 + R-25.2-3 + ADR-0177 RE-CONSUMER.
//
// Wire-shape (cpp-host src/exports.cc:664-687):
//
//	Word http_call(Word uri_ptr, Word uri_size,
//	                Word header_pairs_ptr, Word header_pairs_size,
//	                Word body_ptr, Word body_size,
//	                Word trailer_pairs_ptr, Word trailer_pairs_size,
//	                Word timeout_milliseconds, Word token_ptr)
//
// Reads (cluster, headers, body, trailers) from guest memory; delegates to
// httpCallHost.DispatchHttpCall with the current stream context id; on Ok
// writes the allocated call_id back to guest memory at ret_call_id_ptr.
//
// Semantic per Q4 + AMEND-B3 + cpp-host byte-faithful:
//
//   - cluster read failure → WasmResult::InvalidMemoryAccess (=6).
//   - headers read failure → InvalidMemoryAccess.
//   - headers decode failure (malformed pairs wire format) → BadArgument.
//   - body read failure → InvalidMemoryAccess.
//   - trailers read failure → InvalidMemoryAccess.
//   - trailers decode failure → BadArgument.
//   - unknown cluster (host returns BadArgument) → propagate
//     BadArgument; write 0 to ret_call_id_ptr (NO call_id allocated).
//     http_call_dispatch_unknown_cluster counter increments (host-side;
//     deferred Task 17).
//   - no dispatcher wired (host returns InternalFailure) → propagate
//     InternalFailure; write 0 to ret_call_id_ptr.
//   - Ok with allocated call_id → write call_id to ret_call_id_ptr;
//     return Ok. http_call_dispatched counter increments (host-side;
//     deferred Task 17).
//
// Pseudo-headers in the headers list (:method / :path / :scheme /
// :authority) are extracted by buildHttpCallRequest at the host side; the
// shim passes the headers list through unchanged.
//
// timeoutMs == 0 is interpreted by the host as "use the conservative
// default" (5 seconds at 25.2; see http_call.go defaultHttpCallTimeout).
//
// retCallIDPtr write-failure ⇒ WasmResultInvalidMemoryAccess (the host has
// already allocated the call_id; the guest cannot recover its handle, which
// is a defensive impossibility for well-formed guests but a correctness
// guard for malformed guests).
func HttpCallShim(ctx context.Context, mod api.Module, host Host25_2, clusterData, clusterSize, headersData, headersSize, bodyData, bodySize, trailersData, trailersSize, timeoutMs, retCallIDPtr uint32) WasmResult {
	hh, ok := host.(httpCallHost)
	if !ok {
		return WasmResultInternalFailure
	}

	mem := mod.Memory()

	// Read cluster bytes. clusterSize == 0 → empty string (valid; the host
	// rejects empty cluster with HasCluster miss → BadArgument).
	var clusterStr string
	if clusterSize > 0 {
		raw, ok := mem.Read(clusterData, clusterSize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		// Defensive copy: mem.Read returns a view into live guest memory.
		buf := make([]byte, len(raw))
		copy(buf, raw)
		clusterStr = string(buf)
	}

	// Read + decode headers pairs. headersSize == 0 → empty headers.
	var headers []HeaderPair25_2
	if headersSize > 0 {
		raw, ok := mem.Read(headersData, headersSize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		decoded, err := decodePairs25_2(raw)
		if err != nil {
			return WasmResultBadArgument
		}
		headers = decoded
	}

	// Read body bytes. bodySize == 0 → empty body (valid; many guests
	// dispatch GET requests with no body).
	var bodyBytes []byte
	if bodySize > 0 {
		raw, ok := mem.Read(bodyData, bodySize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		bodyBytes = make([]byte, len(raw))
		copy(bodyBytes, raw)
	}

	// Read + decode trailers pairs. trailersSize == 0 → empty trailers.
	var trailers []HeaderPair25_2
	if trailersSize > 0 {
		raw, ok := mem.Read(trailersData, trailersSize)
		if !ok {
			return WasmResultInvalidMemoryAccess
		}
		decoded, err := decodePairs25_2(raw)
		if err != nil {
			return WasmResultBadArgument
		}
		trailers = decoded
	}

	// Dispatch via the host. The host:
	//   (a) validates the cluster (BadArgument on miss);
	//   (b) builds the *http.Request from the pseudo-headers + body;
	//   (c) allocates the monotonic call_id;
	//   (d) spawns the dispatch goroutine;
	//   (e) returns (callID, Ok) for the guest to consume.
	streamCtxID := hh.CurrentCtxID()
	callID, status := hh.DispatchHttpCall25_2(ctx, streamCtxID, clusterStr, headers, bodyBytes, trailers, timeoutMs)

	if status != WasmResultOk {
		// On non-Ok: write 0 to ret_call_id_ptr (defensive; the guest
		// should not consume the call_id on non-Ok status, but writing 0
		// is harmless + matches the "zero return slot on miss" pattern
		// from the other Task 4-7 shims).
		if !mem.WriteUint32Le(retCallIDPtr, 0) {
			return WasmResultInvalidMemoryAccess
		}
		return status
	}

	// Ok: write the allocated call_id to ret_call_id_ptr.
	if !mem.WriteUint32Le(retCallIDPtr, callID) {
		return WasmResultInvalidMemoryAccess
	}
	return WasmResultOk
}

// decodePairs25_2 is the abi-package-local mirror of the wasm-package
// DecodePairs helper. Decodes the proxy-wasm pairs wire format into a
// []HeaderPair25_2 slice. Wire format (little-endian throughout):
//
//	u32 num_pairs
//	(u32 key_len, u32 value_len) * num_pairs
//	(key_bytes, NUL, value_bytes, NUL) * num_pairs
//
// Mirrors the wasm-package implementation byte-for-byte; defined here to
// keep the abi → wasm decoupling invariant. On any malformed input (truncated,
// missing NUL terminators, declared lengths overrunning the buffer, trailing
// garbage), returns a nil slice + non-nil error so the shim caller can
// translate to WasmResultBadArgument.
//
// num_pairs == 0 returns (nil, nil) — a valid empty pairs frame is the 4-byte
// little-endian u32(0) prefix followed by no further bytes.
func decodePairs25_2(buf []byte) ([]HeaderPair25_2, error) {
	if len(buf) < 4 {
		return nil, errPairsTruncated25_2
	}
	numPairs := uint32(buf[0]) | uint32(buf[1])<<8 | uint32(buf[2])<<16 | uint32(buf[3])<<24
	pos := 4
	remaining := len(buf) - pos
	if uint64(numPairs)*10 > uint64(remaining) && uint64(numPairs) > uint64(remaining/10)+1 {
		return nil, errPairsTruncated25_2
	}
	if uint64(numPairs)*8 > uint64(remaining) {
		return nil, errPairsTruncated25_2
	}

	type sizes struct{ keyLen, valueLen uint32 }
	table := make([]sizes, numPairs)
	for i := uint32(0); i < numPairs; i++ {
		table[i].keyLen = uint32(buf[pos]) | uint32(buf[pos+1])<<8 | uint32(buf[pos+2])<<16 | uint32(buf[pos+3])<<24
		pos += 4
		table[i].valueLen = uint32(buf[pos]) | uint32(buf[pos+1])<<8 | uint32(buf[pos+2])<<16 | uint32(buf[pos+3])<<24
		pos += 4
	}

	pairs := make([]HeaderPair25_2, numPairs)
	for i := uint32(0); i < numPairs; i++ {
		keyLen := table[i].keyLen
		valueLen := table[i].valueLen

		if uint64(pos)+uint64(keyLen)+1 > uint64(len(buf)) {
			return nil, errPairsTruncated25_2
		}
		key := string(buf[pos : pos+int(keyLen)])
		pos += int(keyLen)
		if buf[pos] != 0x00 {
			return nil, errPairsMissingNUL25_2
		}
		pos++

		if uint64(pos)+uint64(valueLen)+1 > uint64(len(buf)) {
			return nil, errPairsTruncated25_2
		}
		value := string(buf[pos : pos+int(valueLen)])
		pos += int(valueLen)
		if buf[pos] != 0x00 {
			return nil, errPairsMissingNUL25_2
		}
		pos++

		pairs[i] = HeaderPair25_2{Key: key, Value: value}
	}

	if pos != len(buf) {
		return nil, errPairsTrailingBytes25_2
	}

	if numPairs == 0 {
		return nil, nil
	}
	return pairs, nil
}

// Package-private sentinel errors for the abi-side pairs decode. Mirror
// the wasm-package internal/wasm/pairs.go sentinels structurally; we keep
// them separate to honor the abi → wasm decoupling invariant.
var (
	errPairsTruncated25_2     = errPairsErr("pairs: truncated buffer")
	errPairsTrailingBytes25_2 = errPairsErr("pairs: trailing bytes after well-formed frame")
	errPairsMissingNUL25_2    = errPairsErr("pairs: missing NUL terminator")
)

// errPairsErr is the minimal error type for the abi-side pairs decode
// sentinels. Keeps the package free of an external errors-package import
// (we already use stdlib `context` + `wazero/api` only).
type errPairsErr string

func (e errPairsErr) Error() string { return string(e) }
