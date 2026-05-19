package lua

// decode_headers.go — DecodeHeaders dispatcher per 22.1 SPEC §4.3.
//
// Constructs a per-stream gopher-lua VM at request-headers arrival,
// installs the bridge metatables + the request_handle userdata,
// executes the operator-supplied script top-level (to define
// envoy_on_request / envoy_on_response globals), checks for hook
// presence + invokes envoy_on_request against the request_handle, then
// inspects the respondCaptured state to either short-circuit to
// SendLocalReply or continue dispatch.
//
// # Dispatch sequence per 22.1 SPEC §4.3:
//
//  1. Nil-chunk pass-through. If cc == nil OR cc.chunk == nil, return
//     Continue without VM construction. Matches the D1-REFUTED arm-5
//     silent-no-op disposition at compiled_config.go (the proto's
//     default_source_code is absent → no script defined → no Lua-side
//     behavior, pass-through to the next filter).
//
//  2. Construct per-stream VM via luaprim.NewVM with the SHARED
//     cc.sandbox config. The VM is fresh per stream — no cross-stream
//     contamination (matches upstream's per-request VM construction
//     model at lua_filter.cc).
//
//  3. Install the bridge metatables (request_handle + response_handle +
//     headers + streamInfo) + the __pairs shim per Task 6-8 IMPL. Done
//     ONCE per VM here, not lazily, to keep the bridge-method first-call
//     latency predictable.
//
//  4. Build the *requestHandleContext + LUserData + bind to global `rh`?
//     The hook signature is `envoy_on_request(request_handle)` — the
//     userdata is PASSED AS ARG, not bound to a global. So we construct
//     the userdata and pass it as the CallGlobal arg below.
//
//  5. vm.Run(cc.chunk) — executes script top-level. Errors increment
//     stats.errors + log + Continue (per 22.1 SPEC §4.3 step 3 + the
//     BRAINSTORM §2.9 "continue dispatch despite script error"
//     discipline).
//
//  6. vm.HasGlobalFunc("envoy_on_request") — hook-presence check. If
//     absent, Continue (per 22.1 SPEC §4.3 step 4 — D1-REFUTED arm-17:
//     scripts that define neither hook degrade to pass-through silently,
//     matching upstream's INFO-log-only disposition at
//     lua_filter.cc:174-181).
//
//  7. stats.executions++ — upstream-parity: increments PER INVOCATION,
//     not per-success (per AMEND-3 + lua_filter.cc:872). Bumped BEFORE
//     CallGlobal so a hook crash still increments executions (matches
//     upstream's instrumentation order).
//
//  8. vm.CallGlobal("envoy_on_request", reqUd). Errors increment
//     stats.errors + log + fall through to respond-state check (even on
//     hook error, a respond-state captured BEFORE the error still
//     triggers SendLocalReply — defensive against partial-execution
//     scripts).
//
//  9. respondCaptured check. If non-nil: stats.respondCalls++ +
//     SendLocalReply(status, body, headers) + StopIteration. The
//     SendLocalReply hand-off carries the OrderedHeaders carrier built
//     by bridge.go's requestHandleRespond per parent §11.6.7 byte-pin.
//
// 10. Otherwise: Continue. The VM stays alive for EncodeHeaders to
//     reuse (avoids re-Run of the chunk on the encode side; the
//     envoy_on_response global is already registered from step 5).

import (
	"net/http"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	luaprim "github.com/esalaine/envoy-go/internal/lua"
)

// DecodeHeaders implements the decode-side dispatcher per 22.1 SPEC
// §4.3 + parent §11.6.7. Constructs the per-stream gopher-lua VM,
// executes the operator-supplied script's envoy_on_request hook
// against the request_handle bridge userdata, and either short-
// circuits to SendLocalReply (if :respond() fired) or continues
// dispatch.
func (f *filter) DecodeHeaders(headers http.Header, _ bool) envoyhttp.FilterHeadersStatus {
	// Step 1: nil-chunk pass-through (D1-REFUTED arm-5 silent-no-op).
	if f.cc == nil || f.cc.chunk == nil {
		return envoyhttp.Continue
	}

	// Step 2: construct per-stream VM with the SHARED sandbox config.
	f.vm = luaprim.NewVM(
		luaprim.WithSandboxConfig(f.cc.sandbox),
		// WithPanicHandler + WithBasePrintSink default to nil (drop
		// per envoy-go-strict default; no Lua-print-to-stdout leak).
	)

	// Step 3: install bridge metatables + pairs shim. Idempotent —
	// gopher-lua's NewTypeMetatable returns the existing metatable on
	// second call, but the per-stream VM is fresh so each call is the
	// FIRST install for this VM.
	L := f.vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installStreamInfoMetatable(L)
	installPairsShim(L)

	// Step 4: build the request_handle context + LUserData. The
	// requestHandleCallbacksAdapter bridges the framework's
	// DecoderFilterCallbacks to the bridge's RequestHandleCallbacks
	// interface (the small 4-method subset consumed by :streamInfo()).
	f.reqCtx = &requestHandleContext{
		headers: headers,
		cb:      newRequestHandleCallbacksAdapter(f.dcb),
	}
	reqUd := L.NewUserData()
	reqUd.Value = f.reqCtx
	L.SetMetatable(reqUd, L.GetTypeMetatable(requestHandleTypeName))

	// Step 5: execute script top-level. Errors → stats.errors + log +
	// Continue.
	if err := f.vm.Run(f.cc.chunk); err != nil {
		if f.cc.stats != nil && f.cc.stats.errors != nil {
			f.cc.stats.errors.Inc()
		}
		logf("ERROR lua: script run failed: %v", err)
		return envoyhttp.Continue
	}

	// Step 6: hook-presence check (D1-REFUTED arm-17 silent-no-op).
	if !f.vm.HasGlobalFunc("envoy_on_request") {
		return envoyhttp.Continue
	}

	// Step 7: stats.executions++ (upstream-parity per AMEND-3; bumped
	// BEFORE CallGlobal so hook crashes still count as executions).
	if f.cc.stats != nil && f.cc.stats.executions != nil {
		f.cc.stats.executions.Inc()
	}

	// Step 8: invoke the hook with the request_handle userdata. Errors
	// → stats.errors + log + fall through to respond-state check.
	if err := f.vm.CallGlobal("envoy_on_request", reqUd); err != nil {
		if f.cc.stats != nil && f.cc.stats.errors != nil {
			f.cc.stats.errors.Inc()
		}
		logf("ERROR lua: envoy_on_request failed: %v", err)
		// Fall through to respond-state check; a respond-state captured
		// BEFORE the error still fires (defensive against partial-
		// execution scripts).
	}

	// Step 9: respond-state check.
	if f.reqCtx.respondCaptured != nil {
		if f.cc.stats != nil && f.cc.stats.respondCalls != nil {
			f.cc.stats.respondCalls.Inc()
		}
		rs := f.reqCtx.respondCaptured
		if f.dcb != nil {
			f.dcb.SendLocalReply(rs.status, rs.body, rs.headers)
		}
		return envoyhttp.StopIteration
	}

	// Step 10: no respond; pass through to next filter.
	return envoyhttp.Continue
}
