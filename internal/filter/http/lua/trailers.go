package lua

// trailers.go — Task 8 (phase 22.2 IMPL) trailers bridge per SPEC §3.4 +
// §2.2 + PLAN Task 8 + ADR-0192 §Decision body anticipation.
//
// # Surface
//
// One bridge method on each of request_handle + response_handle:
//
//   - :trailers() returns a userdata wrapping the per-stream trailers
//     http.Header, OR nil if no trailers have been received yet (lazy-
//     availability per SPEC §2.2 + PLAN Q2 — upstream Lua filter's
//     `trailers()` returns nil pre-trailers-arrival).
//
// The returned userdata's metatable (envoy_trailers, distinct registry
// key from envoy_headers per BRAINSTORM §2.2) holds the same 7 mutation
// methods + __pairs alphabetical-snapshot iterator as the headers
// metatable — reuses headersMethods + headersPairs UNCHANGED (the
// dispatch is identical; trailers and headers are both backed by
// http.Header so a single dispatch surface is correct).
//
// # 8 operator-visible surface entries (per PLAN Task 8 acceptance)
//
// The 22.1 headersMethods map registers 7 Go LGFunctions under 8
// operator-visible method names (`:append` is an ALIAS for `:add` per
// upstream wrappers.cc HeaderMapWrapper::luaAdd / luaAppend collapse).
// The trailers metatable mirrors this exactly:
//
//   1. :get(name)            → headersGet
//   2. :getAtIndex(name, i)  → headersGetAtIndex
//   3. :getNumValues(name)   → headersGetNumValues
//   4. :add(name, value)     → headersAdd
//   5. :append(name, value)  → headersAdd   (alias per upstream wrappers.cc)
//   6. :remove(name)         → headersRemove
//   7. :replace(name, value) → headersReplace
//   8. __pairs iterator      → headersPairs (alphabetical-snapshot per §11.2.2)
//
// Operator-visible-surface count = 8 (7 named methods + the __pairs
// metamethod). The PLAN Task 8 brief documents this as "8 mutation
// methods + 1 inherited" — the inheritance refers to the :append alias
// being the 8th operator-visible name backed by the same Go function as
// :add.
//
// # Lazy-availability discipline (SPEC §2.2 + PLAN Q2)
//
// Pre-trailers-arrival, the per-stream context's `trailers` field is
// nil — the bridge :trailers() method pushes lua.LNil rather than a
// userdata wrapping an empty map. This matches upstream Envoy's
// behavior: HeaderMapWrapper for trailers is constructed only after
// HTTP/2 (or chunked H1) trailer-frames are received; Lua scripts that
// call `request_handle:trailers()` early get nil and must handle the
// nil case explicitly (`local t = rh:trailers(); if t then ... end`).
//
// # Wiring at terminal-state (DecodeTrailers / EncodeTrailers)
//
// Per PLAN Task 8 step 3, the framework callbacks DecodeTrailers /
// EncodeTrailers (defined at lua.go) attach the received trailers map
// onto the per-stream context (f.reqCtx.trailers or
// f.respCtx.trailers). If the per-stream context is nil (e.g. nil-chunk
// pass-through path where DecodeHeaders never constructed it), the
// callbacks no-op gracefully — defensive against the synthetic test
// path + the framework's TrailersContinue contract.
//
// # No __pairs divergence from headers
//
// Per SPEC §11.2.2 "Trailers `pairs(udata)` CONFIRMED-IDENTICAL via 22.1
// installPairsShim discipline" — the shim installed by
// installPairsShim() honors __pairs on userdata for BOTH envoy_headers
// AND envoy_trailers. No NEW shim code; no BEHAVIOR_CONTRACT departure.

import (
	"net/http"

	lua "github.com/yuin/gopher-lua"
)

// requestHandleTrailers implements request_handle:trailers() per SPEC
// §3.4 + §2.2 + PLAN Task 8. Returns a userdata wrapping the per-stream
// request-side trailers http.Header, or nil if trailers have not been
// received yet (lazy-availability discipline).
//
// Argument shape:
//
//	rh:trailers() -> userdata | nil
//
// The returned userdata's metatable is envoy_trailers (distinct from
// envoy_headers but with the same 7 mutation methods + __pairs).
// Mutations via :add / :remove / :replace on the returned userdata
// mutate the underlying http.Header map IN-PLACE — observable on the
// filter's request-side trailers state for subsequent filters in the
// chain.
func requestHandleTrailers(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*requestHandleContext)
	if !ok {
		L.ArgError(1, "expected request_handle")
		return 0
	}
	if ctx.trailers == nil {
		L.Push(lua.LNil)
		return 1
	}
	return pushTrailersUD(L, ctx.trailers)
}

// responseHandleTrailers implements response_handle:trailers() —
// symmetric to requestHandleTrailers for the encode side. Returns a
// userdata wrapping the per-stream response-side trailers http.Header,
// or nil if trailers have not been received yet.
func responseHandleTrailers(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*responseHandleContext)
	if !ok {
		L.ArgError(1, "expected response_handle")
		return 0
	}
	if ctx.trailers == nil {
		L.Push(lua.LNil)
		return 1
	}
	return pushTrailersUD(L, ctx.trailers)
}

// pushTrailersUD allocates a fresh LUserData wrapping the supplied
// http.Header + attaches the envoy_trailers metatable + pushes it onto
// the Lua stack. Returns 1 (number of pushed return values per
// LGFunction convention).
//
// Delegates to pushHeaderLikeUD (bridge.go) with the metatable registry
// key swapped — the underlying value type is http.Header in both cases
// (Go map type — passed by reference; embedding directly in
// LUserData.Value preserves mutate-through semantics for :add /
// :remove / :replace without an extra indirection). The trailers
// methods dispatch via getHeadersFromUD UNCHANGED (it casts the
// LUserData.Value to http.Header without consulting the metatable's
// registry key — the type-distinct metatable is purely a Lua-side
// marker, not a Go-side dispatch discriminator).
func pushTrailersUD(L *lua.LState, h http.Header) int {
	return pushHeaderLikeUD(L, h, trailersTypeName)
}
