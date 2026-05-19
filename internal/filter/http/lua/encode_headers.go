package lua

// encode_headers.go — EncodeHeaders dispatcher per 22.1 SPEC §4.3.
//
// Symmetric to decode_headers.go for the envoy_on_response hook. The
// per-stream VM is REUSED from DecodeHeaders (the chunk was already
// vm.Run'd at decode time + the envoy_on_response global is already
// registered), so encode just constructs a fresh response_handle
// userdata and invokes the hook.
//
// # Dispatch sequence per 22.1 SPEC §4.3 (encode-side):
//
//  1. Nil-prerequisites pass-through. If cc == nil OR cc.chunk == nil
//     OR vm == nil, return Continue. The vm-nil case occurs when
//     DecodeHeaders short-circuited at the nil-chunk pass-through
//     (no VM was constructed); same disposition on the encode side.
//
//  2. Build the response_handle context + LUserData. The
//     response_handle wraps the response-side http.Header + the same
//     RequestHandleCallbacks adapter from decode-side (the encode side
//     consumes the SAME 4-method :streamInfo() accessor surface — the
//     request-handle / response-handle interfaces both satisfy
//     RequestHandleCallbacks).
//
//  3. vm.HasGlobalFunc("envoy_on_response") — hook-presence check. If
//     absent, Continue (D1-REFUTED arm-17 silent-no-op).
//
//  4. stats.executions++ — upstream-parity (per AMEND-3 +
//     lua_filter.cc:872). Bumped per-invocation, not per-success.
//
//  5. vm.CallGlobal("envoy_on_response", respUd). Errors increment
//     stats.errors + log + Continue. The encode-side :respond()
//     SURFACES HERE as a *lua.ApiError from gopher-lua's PCall —
//     bridge.go's responseHandleRespond raises the byte-exact AMEND-8
//     wording "respond not currently supported in the response path"
//     via L.RaiseError, which propagates back through PCall as
//     ApiErrorRun. The error string is loggable but is NOT propagated
//     to the downstream consumer beyond the stats.errors bump (matches
//     upstream lua_filter.cc behavior at the encode-side respond reject
//     site).
//
//  6. Continue. There is NO respond-state check on the encode side
//     (the :respond() reject path raises an error, never captures
//     state).
//
// Note: header mutations performed by envoy_on_response via
// response_handle:headers() mutate the response-side http.Header in
// place — the framework's encode chain consumes the mutated map after
// EncodeHeaders returns Continue.

import (
	"net/http"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// EncodeHeaders implements the encode-side dispatcher per 22.1 SPEC
// §4.3. Reuses the per-stream VM constructed at DecodeHeaders, builds
// a fresh response_handle userdata + invokes the envoy_on_response
// hook against it. The encode-side :respond() reject surfaces as a
// *lua.ApiError from CallGlobal + increments stats.errors per AMEND-8.
func (f *filter) EncodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Step 1: nil-prerequisites pass-through.
	if f.cc == nil || f.cc.chunk == nil || f.vm == nil {
		return envoyhttp.Continue
	}

	// Step 2: build the response_handle context + LUserData. The
	// adapter is the same RequestHandleCallbacks (the bridge's
	// :streamInfo() consumes it identically for both sides).
	respCtx := &responseHandleContext{
		headers: headers,
		cb:      newResponseHandleCallbacksAdapter(f.ecb, f.dcb),
	}
	L := f.vm.State()
	respUd := L.NewUserData()
	respUd.Value = respCtx
	L.SetMetatable(respUd, L.GetTypeMetatable(responseHandleTypeName))

	// Step 3: hook-presence check.
	if !f.vm.HasGlobalFunc("envoy_on_response") {
		return envoyhttp.Continue
	}

	// Step 4: stats.executions++ (upstream-parity per AMEND-3).
	if f.cc.stats != nil && f.cc.stats.executions != nil {
		f.cc.stats.executions.Inc()
	}

	// Step 5: invoke the hook. The encode-side :respond() raises the
	// AMEND-8 byte-exact runtime error → PCall captures → CallGlobal
	// returns *lua.ApiError → stats.errors++ + log.
	if err := f.vm.CallGlobal("envoy_on_response", respUd); err != nil {
		if f.cc.stats != nil && f.cc.stats.errors != nil {
			f.cc.stats.errors.Inc()
		}
		logf("ERROR lua: envoy_on_response failed: %v", err)
		return envoyhttp.Continue
	}

	// Step 6: Continue. No respond-state check on encode side.
	return envoyhttp.Continue
}
