package lua

// bridge.go — Envoy↔Lua bridge surface per 22.1 SPEC §4.3 + §11.2 D7 +
// parent §11.2 + parent §11.6 + BRAINSTORM Q6 pragmatic-middle cut.
//
// # Task 6 contribution (THIS FILE — first landing slice)
//
// Lands the request_handle/response_handle userdata + metatable setup
// + the 7 headers-object methods (:get / :getAtIndex / :getNumValues /
// :add / :append / :remove / :replace) + the __pairs metamethod
// (alphabetical-snapshot per §11.2 D7) + the pairs-shim that makes
// gopher-lua honor __pairs on userdata (Lua-5.1's basePairs requires
// LTable; we override `pairs` with a Lua-5.2-style version that checks
// for __pairs first — required because the bridge userdata is NOT a
// table).
//
// # Forward-task layout
//
// The file is structured so Tasks 7-9 cleanly append:
//
//   - Task 7 — 6 :logTrace/:logDebug/:logInfo/:logWarn/:logErr/:logCritical
//     methods. New file-section + 6 LGFunctions + entries appended to
//     requestHandleMethods + responseHandleMethods maps. No re-structure
//     of Task 6 surface.
//
//   - Task 8 — :streamInfo() returning a streamInfo userdata with 4
//     methods (:protocol / :routeName / :downstreamLocalAddress /
//     :downstreamDirectRemoteAddress). New file-section: streamInfo
//     userdata + metatable + 4 LGFunctions. New entry appended to
//     requestHandleMethods + responseHandleMethods maps. Requires
//     installStreamInfoMetatable to be called alongside the existing
//     install* helpers at the per-stream-VM setup point (Task 9 wires).
//
//   - Task 9 — :respond(headers_table, body_string) on the request-side
//     (full byte-pin per parent §11.6.7 + AMEND-7 + :status [200,600)
//     validation per AMEND-8) + the response-side runtime-reject string
//     "respond not currently supported in the response path" per
//     AMEND-8 + decode_headers.go + encode_headers.go dispatch wiring.
//     Adds respondState capture to filter struct + responseHandleContext
//     + appends 1 entry to requestHandleMethods + 1 to
//     responseHandleMethods.
//
// # Architectural notes
//
// 1. Context structs (requestHandleContext / responseHandleContext)
//    are intentionally minimal at Task 6 — only the http.Header field
//    is populated. Task 8 added the cb field
//    (RequestHandleCallbacks interface) consumed by :streamInfo();
//    Task 9 adds the respondState capture pointer (request side) +
//    decoder/encoder callbacks. Each Task's growth is field-additive
//    on the same struct; no re-structure of the Task 6 fields.
//
// 2. The :append method is an ALIAS for :add per upstream Envoy
//    wrappers.cc semantics. See HeaderMapWrapper::luaAppend at
//    source/extensions/filters/http/lua/wrappers.cc — upstream wires
//    both methods to the same C++ entry (HeaderMap::addCopy). The
//    SPEC §1 + parent §11.2 enumerate them as distinct surface entries
//    (operator-visible alias) but the implementation collapses to a
//    single Go function — headers_add — registered under both names.
//
// 3. The __pairs metamethod IS set on the metatable per the §11.2 D7
//    discipline; however, **gopher-lua's basePairs does NOT honor
//    __pairs on userdata** (Lua 5.1 semantics; only 5.2+ pairs() looks
//    up the metamethod). installPairsShim overrides the global `pairs`
//    to dispatch the __pairs metamethod when present (effectively a
//    Lua-5.2 backport — necessary because fixture-0026 scenario (f)'s
//    script uses `for k,v in pairs(rh:headers()) do ... end`).
//
// 4. The headers userdata wraps `http.Header` directly (NOT a pointer-
//    indirection like *requestHandleContext does for the request_handle
//    userdata). Reason: http.Header IS a map (Go map type — passed by
//    reference under the hood); embedding it directly in LUserData.Value
//    preserves mutate-through semantics for :add/:remove/:replace
//    without an extra indirection. getHeadersFromUD just casts back.

import (
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// requestHandleTypeName + responseHandleTypeName + headersTypeName +
// streamInfoTypeName are the metatable registry-keys used by gopher-lua's
// NewTypeMetatable. Stable across Tasks 6-9; the Task 9 respond-state
// filter capture + Task 8 streamInfo userdata reference these names via
// L.GetTypeMetatable for cross-callback userdata identity-checks.
const (
	requestHandleTypeName  = "envoy_request_handle"
	responseHandleTypeName = "envoy_response_handle"
	headersTypeName        = "envoy_headers"
	streamInfoTypeName     = "envoy_stream_info"
)

// ---------------------------------------------------------------------
// Per-handle Go-side context structs (per 22.1 SPEC §4.3)
// ---------------------------------------------------------------------

// RequestHandleCallbacks is the small per-stream accessor interface
// consumed by the bridge's :streamInfo() 4-method subset (Task 8). It
// is decoupled from the framework's full DecoderFilterCallbacks surface
// (internal/filter/http/callbacks.go) for two reasons: (1) testability —
// a tiny test-double satisfies the 4-method shape without depending on
// the entire 11-method framework callback; (2) framework-gap insulation —
// some upstream-parity bridge methods (:routeName,
// :downstreamDirectRemoteAddress) have no exact 1:1 framework accessor
// at phase 22.1, and the adapter wiring in Task 9 can supply stubs
// (empty string + a re-use of the existing DownstreamRemoteAddr accessor)
// without contaminating the bridge IMPL.
//
// All methods return strings ready for direct push to the Lua stack —
// the adapter is responsible for formatting net.Addr → "ip:port" and for
// filling stubbed fields with sensible defaults (empty string per the
// "always-string, never-nil" contract anchored at parent §11.6).
//
// Future framework extensions (RouteName accessor on
// DecoderFilterCallbacks; explicit DownstreamDirectRemoteAddress
// distinct from DownstreamRemoteAddr) would update the Task 9 adapter
// to consume the new framework primitives; this bridge-side interface
// is stable.
type RequestHandleCallbacks interface {
	// Protocol returns the HTTP protocol version string for the
	// downstream connection: "HTTP/1.0" / "HTTP/1.1" / "HTTP/2" / "HTTP/3"
	// (upstream-parity literals). Empty string for synthetic streams that
	// did not exercise HCM dispatch (matches the framework's
	// DownstreamProtocol contract).
	Protocol() string

	// RouteName returns the resolved route name for the request, or the
	// empty string if no route is selected / the framework does not yet
	// expose the accessor (the latter is the phase-22.1 condition —
	// framework gap to be closed in a future phase).
	RouteName() string

	// DownstreamLocalAddress returns the listener's bound "ip:port" the
	// downstream connection accepted, as formatted by net.Addr.String().
	// Empty string for synthetic streams.
	DownstreamLocalAddress() string

	// DownstreamDirectRemoteAddress returns the connecting peer's
	// "ip:port" — the immediate downstream remote (not the chain-original
	// remote, which would require XFF resolution). At phase 22.1 the
	// adapter wires this to the framework's DownstreamRemoteAddr value
	// (envoy-go has no proxy-chain origin distinction yet, so "remote" ==
	// "direct remote"). Empty string for synthetic streams.
	DownstreamDirectRemoteAddress() string
}

// requestHandleContext is the Go-side state backing a request_handle
// userdata. One per per-stream filter dispatch (decode side). Task 8
// adds the cb field consumed by :streamInfo(); Task 9 extends with
// respond-state capture consumed by decode_headers.go.
type requestHandleContext struct {
	headers         http.Header
	cb              RequestHandleCallbacks // Task 8 — :streamInfo() accessor surface
	respondCaptured *respondState          // Task 9 — :respond() capture; nil = no respond fired
}

// responseHandleContext is the Go-side state backing a response_handle
// userdata. Symmetric to requestHandleContext for the encode side. Task
// 8 adds the cb field (response-side parity for :streamInfo()); Task 9
// adds NO respond-state capture (the encode-side :respond() raises a
// runtime error per AMEND-8 — no capture surface needed).
type responseHandleContext struct {
	headers http.Header
	cb      RequestHandleCallbacks // Task 8 — :streamInfo() accessor surface (encode-side parity)
}

// respondState captures the result of a successful
// request_handle:respond(headers_table, body_string) invocation per
// parent §11.6.7 + AMEND-7 + AMEND-8. The decode_headers.go dispatcher
// reads it after the envoy_on_request hook returns and emits a
// SendLocalReply if non-nil. Field-final at 22.1.
//
//   - status — the parsed [200, 600) :status integer from the headers
//     table (validated against AMEND-8's byte-exact ":status must be
//     between 200-599" wording).
//   - body — the verbatim body bytes (Go len(body) drives the auto-
//     content-length; no encoding transformations).
//   - headers — the OrderedHeaders carrier in caller-supplied order +
//     framework-injected content-type/content-length defaults per
//     upstream Utility::prepareLocalReply at utility.cc:1241,1273.
//     The decode_headers.go dispatcher passes this through verbatim
//     to dcb.SendLocalReply.
type respondState struct {
	status  int
	body    string
	headers envoyhttp.OrderedHeaders
}

// respondStatusOutOfRangeMsg is the AMEND-8 byte-exact runtime-error
// wording raised when :respond()'s :status header is outside the
// inclusive [200, 599] (== Go half-open [200, 600)) range. Pinned via
// a package-level const so a regression on the byte-stable contract
// surfaces at the TestBridge_Respond_StatusRangeValidation_* assertions.
// Matches upstream lua_filter.cc:~578-580.
const respondStatusOutOfRangeMsg = ":status must be between 200-599"

// respondNotInResponsePathMsg is the AMEND-8 byte-exact runtime-error
// wording raised when response_handle:respond(...) is invoked from
// envoy_on_response. Matches upstream lua_filter.cc:1031-1034. Pinned
// via a package-level const so a regression on the byte-stable contract
// surfaces at the TestBridge_ResponseHandleRespond_RejectsByteExact
// assertion.
const respondNotInResponsePathMsg = "respond not currently supported in the response path"

// pseudoHeaderPrefix marks decode-only HTTP pseudo-headers (:scheme,
// :authority, :path, :method) that the request_handle:respond() body
// builder skips when projecting the headers_table into the
// OrderedHeaders carrier (the operator-meaningful :status is extracted
// separately + the other 4 pseudo-headers are nonsensical on a synthetic
// local-reply). Matches upstream's split between status and other
// pseudo-headers at lua_filter.cc:~559-583.
const pseudoHeaderPrefix = ":"

// statusPseudoHeader is the canonical Lua-table key from which the
// status integer is parsed. Per parent §11.6.7 + AMEND-7: scripts
// supply `[":status"]="403"` (string-valued) and the bridge parses to
// int.
const statusPseudoHeader = ":status"

// contentTypeHeader is the canonical header name applied as default
// `text/plain` when the headers_table did not supply content-type per
// upstream Utility::prepareLocalReply at utility.cc:1241,1273.
const contentTypeHeader = "content-type"

// contentTypeDefault is the byte-exact default content-type per
// upstream Utility::prepareLocalReply at utility.cc:1241,1273. Pinned
// for the §11.6.7 byte-pin verification.
const contentTypeDefault = "text/plain"

// contentLengthHeader is the canonical header name auto-set from the
// body byte length per parent §11.6.7 byte-pin (utility.cc:1270).
const contentLengthHeader = "content-length"

// ---------------------------------------------------------------------
// Metatable installers
// ---------------------------------------------------------------------

// installRequestHandleMetatable registers the metatable for the
// request_handle userdata under requestHandleTypeName. Returns the
// metatable so callers may extend (Tasks 7-9 append additional methods
// by mutating the __index method table further; the Task 6 surface
// only registers the :headers method).
//
// Per 22.1 SPEC §4.3 the per-stream-VM setup (Task 9 decode_headers.go)
// will call this helper once at NewVM time then attach the metatable to
// a freshly-allocated LUserData wrapping *requestHandleContext.
func installRequestHandleMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(requestHandleTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), requestHandleMethods))
	return mt
}

// installResponseHandleMetatable mirrors installRequestHandleMetatable
// for the encode side. Tasks 7-9 register response-side methods (logXxx
// + streamInfo + :respond runtime-reject) by appending to
// responseHandleMethods.
func installResponseHandleMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(responseHandleTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), responseHandleMethods))
	return mt
}

// installHeadersMetatable registers the metatable for the headers
// userdata under headersTypeName. The metatable holds:
//   - __index → table of 7 headers methods
//   - __pairs → alphabetical-snapshot iterator (per §11.2 D7)
//
// Per the 22.1 SPEC §11.2 D7 discipline + parent §11.2.3, the
// __pairs metamethod snapshots http.Header into a Go slice sorted
// case-insensitively by key, then iterates by integer index — closes
// per-run map-iteration non-determinism. Required for cross-side
// fixture-0026 scenario (f) determinism.
func installHeadersMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(headersTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), headersMethods))
	L.SetField(mt, "__pairs", L.NewFunction(headersPairs))
	return mt
}

// installStreamInfoMetatable registers the metatable for the streamInfo
// userdata under streamInfoTypeName. The metatable holds:
//   - __index → table of 4 streamInfo methods (:protocol / :routeName /
//     :downstreamLocalAddress / :downstreamDirectRemoteAddress)
//
// Per 22.1 SPEC §6 Task 8 + parent §11.6, the per-stream-VM setup (Task 9
// decode_headers.go) calls this helper once at NewVM time alongside
// installRequestHandleMetatable + installResponseHandleMetatable +
// installHeadersMetatable. The streamInfo userdata is allocated on-demand
// inside request_handle:streamInfo() — it wraps a RequestHandleCallbacks
// (the interface the adapter satisfies), NOT a *requestHandleContext —
// since the 4 accessor methods consume the callback shape exclusively.
func installStreamInfoMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(streamInfoTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), streamInfoMethods))
	return mt
}

// installPairsShim overrides the global `pairs` with a Lua-5.2-style
// version that honors __pairs on userdata.
//
// gopher-lua is Lua 5.1; its baselib basePairs (baselib.go:252) calls
// L.CheckTable(1) — which fails on userdata. The Lua 5.1 spec does NOT
// dispatch __pairs from pairs(); only Lua 5.2+ does. But the bridge
// userdata (envoy_headers) MUST be iterable via `for k,v in pairs(...)`
// (fixture-0026 scenario (f) script depends on it).
//
// The shim:
//  1. If arg has a __pairs metamethod, invoke it with the arg + return
//     its 3 returns (iter, state, ctrl).
//  2. Else fall back to the standard 5.1 pairs(table) semantics.
//
// The override is in-Lua to avoid the gopher-lua API overhead of
// switching between LGFunction registration + return-pushing. Bound
// directly via L.DoString of a 6-line Lua chunk. Done at install time
// (cheap one-shot; the resulting Lua function is the new `pairs`
// global for the rest of the VM's lifetime).
func installPairsShim(L *lua.LState) {
	// Stash the original pairs as __builtin_pairs (so the shim can
	// fall back when __pairs is absent — preserves stdlib pairs()
	// semantics for plain tables).
	origPairs := L.GetGlobal("pairs")
	L.SetGlobal("__builtin_pairs", origPairs)

	// Lua-side shim definition; safer than juggling LUserData wrap +
	// closure capture from Go (Lua's native 5.2-compat pairs is 4
	// lines).
	const shim = `
local _builtin = __builtin_pairs
function pairs(t)
    local mt = getmetatable(t)
    if mt and mt.__pairs then
        return mt.__pairs(t)
    end
    return _builtin(t)
end
__builtin_pairs = nil
`
	if err := L.DoString(shim); err != nil {
		// Defensive: if for some reason the shim chunk fails to compile
		// or execute, fall back to the gopher-lua default pairs (the
		// userdata path will surface "attempt to call ... (a userdata
		// value)" at script-time, which is at least debuggable). We do
		// NOT panic here — the rest of the bridge surface is still
		// usable.
		_ = err
	}
}

// ---------------------------------------------------------------------
// Method dispatch tables
// ---------------------------------------------------------------------

// requestHandleMethods is the method-name → LGFunction dispatch table
// for the request_handle userdata's __index metafield. Tasks 7-9
// append :logXxx, :streamInfo, :respond entries.
var requestHandleMethods = map[string]lua.LGFunction{
	"headers": requestHandleHeaders,
	// Task 7 — 6 log methods (request-handle side; same logAtLevel
	// helper underlies the response-handle entries below).
	"logTrace":    requestHandleLogTrace,
	"logDebug":    requestHandleLogDebug,
	"logInfo":     requestHandleLogInfo,
	"logWarn":     requestHandleLogWarn,
	"logErr":      requestHandleLogErr,
	"logCritical": requestHandleLogCritical,
	// Task 8 — :streamInfo() returns a streamInfo userdata with 4
	// accessor methods (see streamInfoMethods).
	"streamInfo": requestHandleStreamInfo,
	// Task 9 — :respond(headers_table, body_string) short-circuits to
	// SendLocalReply per parent §11.6.7 byte-pin + AMEND-7 + AMEND-8
	// :status [200, 600) validation. The captured respondState is read
	// by decode_headers.go after envoy_on_request returns.
	"respond": requestHandleRespond,
}

// responseHandleMethods mirrors requestHandleMethods for the response
// side. Same :headers method (returns response-side headers).
var responseHandleMethods = map[string]lua.LGFunction{
	"headers": responseHandleHeaders,
	// Task 7 — same 6 log methods on the encode side; script authors
	// may want to log from envoy_on_response. Per-handle separate stubs
	// (rather than reusing the request_handle functions) to keep the
	// L.CheckUserData(1) discipline crisp for future Task 8/9 extensions
	// that may consult the handle context.
	"logTrace":    responseHandleLogTrace,
	"logDebug":    responseHandleLogDebug,
	"logInfo":     responseHandleLogInfo,
	"logWarn":     responseHandleLogWarn,
	"logErr":      responseHandleLogErr,
	"logCritical": responseHandleLogCritical,
	// Task 8 — response-side :streamInfo() parity; same accessor
	// surface as the request handle (script authors writing
	// envoy_on_response will reasonably want to read stream metadata).
	"streamInfo": responseHandleStreamInfo,
	// Task 9 — :respond() at encode is REJECTED with byte-exact runtime
	// error wording "respond not currently supported in the response
	// path" per AMEND-8 (upstream lua_filter.cc:1031-1034 raises
	// luaL_error with the same wording). The error propagates back
	// through PCall + surfaces as *lua.ApiError to encode_headers.go,
	// which increments cc.stats.errors + logs.
	"respond": responseHandleRespond,
}

// streamInfoMethods is the method-name → LGFunction dispatch table for
// the streamInfo userdata's __index metafield. 4 methods per BRAINSTORM
// Q6 pragmatic-middle bridge surface + parent §11.6 + 22.1 SPEC §6 Task 8.
// Future tasks (22.2 phase) extend with the deferred methods
// (:upstreamHost / :upstreamCluster / :dynamicMetadata /
// :dynamicTypedMetadata / :requestedServerName / :filterState /
// :downstreamSslConnection) per parent §2.11.
var streamInfoMethods = map[string]lua.LGFunction{
	"protocol":                      streamInfoProtocol,
	"routeName":                     streamInfoRouteName,
	"downstreamLocalAddress":        streamInfoDownstreamLocalAddress,
	"downstreamDirectRemoteAddress": streamInfoDownstreamDirectRemoteAddress,
}

// headersMethods is the method-name → LGFunction dispatch table for
// the headers userdata's __index metafield. 7 methods per BRAINSTORM
// Q6 + parent §11.2 + 22.1 SPEC §1. The :append method is registered
// as an ALIAS for :add — both point to the same Go function (matches
// upstream Envoy wrappers.cc HeaderMapWrapper::luaAdd / luaAppend
// collapse to HeaderMap::addCopy).
var headersMethods = map[string]lua.LGFunction{
	"get":          headersGet,
	"getAtIndex":   headersGetAtIndex,
	"getNumValues": headersGetNumValues,
	"add":          headersAdd,
	"append":       headersAdd, // alias per upstream wrappers.cc
	"remove":       headersRemove,
	"replace":      headersReplace,
}

// ---------------------------------------------------------------------
// request_handle :headers + response_handle :headers
// ---------------------------------------------------------------------

// requestHandleHeaders is the Lua-callable :headers() method on the
// request_handle userdata. Returns a new headers userdata wrapping
// the request-side http.Header. The returned userdata shares the same
// underlying map — mutations via :add / :remove / :replace are
// observable on the filter's request-headers state.
func requestHandleHeaders(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*requestHandleContext)
	if !ok {
		L.ArgError(1, "expected request_handle")
		return 0
	}
	return pushHeadersUD(L, ctx.headers)
}

// responseHandleHeaders is the symmetric :headers() method on
// response_handle. Returns a headers userdata wrapping the response-
// side http.Header.
func responseHandleHeaders(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*responseHandleContext)
	if !ok {
		L.ArgError(1, "expected response_handle")
		return 0
	}
	return pushHeadersUD(L, ctx.headers)
}

// pushHeadersUD allocates a fresh LUserData wrapping the supplied
// http.Header + attaches the envoy_headers metatable + pushes it onto
// the stack. Returns 1 (number of pushed return values per LGFunction
// convention).
func pushHeadersUD(L *lua.LState, h http.Header) int {
	ud := L.NewUserData()
	ud.Value = h
	L.SetMetatable(ud, L.GetTypeMetatable(headersTypeName))
	L.Push(ud)
	return 1
}

// getHeadersFromUD type-checks + extracts the http.Header from the
// userdata at the supplied stack index. ArgError + nil-return on type
// mismatch (matches gopher-lua's L.Check* convention — ArgError
// raises a Lua-side error, the caller's Go function effectively
// returns 0).
func getHeadersFromUD(L *lua.LState, idx int) http.Header {
	ud := L.CheckUserData(idx)
	h, ok := ud.Value.(http.Header)
	if !ok {
		L.ArgError(idx, "expected headers")
		return nil
	}
	return h
}

// ---------------------------------------------------------------------
// 7 headers methods (Task 6)
// ---------------------------------------------------------------------

// headersGet implements :get(name) — returns the first value for the
// header (case-insensitive lookup per Go http.Header / CanonicalHeader
// Key) or nil if absent.
//
// Distinguishes "header present with empty-string value" from "header
// absent" — http.Header.Get returns "" for both cases, so we have to
// look at the underlying map to disambiguate. Upstream Envoy's
// HeaderMapWrapper::luaGet (wrappers.cc) returns nil only when the
// header entry is absent; a present-but-empty-value header returns the
// empty string. We match that semantics.
func headersGet(L *lua.LState) int {
	h := getHeadersFromUD(L, 1)
	name := L.CheckString(2)
	vs := h.Values(name)
	if len(vs) == 0 {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(vs[0]))
	return 1
}

// headersGetAtIndex implements :getAtIndex(name, idx) — returns the
// 1-indexed N-th value for the header or nil if out of range.
//
// Per Lua convention indices are 1-based; index 0 and negative indices
// return nil (matches upstream luaGetAtIndex behavior of treating
// out-of-range as the absent case).
func headersGetAtIndex(L *lua.LState) int {
	h := getHeadersFromUD(L, 1)
	name := L.CheckString(2)
	idx := L.CheckInt(3)
	vs := h.Values(name)
	if idx < 1 || idx > len(vs) {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(vs[idx-1]))
	return 1
}

// headersGetNumValues implements :getNumValues(name) — returns the
// count of values for the header (0 if absent). Matches upstream
// HeaderMapWrapper::luaGetNumValues which returns size_t (0 for
// absent).
func headersGetNumValues(L *lua.LState) int {
	h := getHeadersFromUD(L, 1)
	name := L.CheckString(2)
	L.Push(lua.LNumber(len(h.Values(name))))
	return 1
}

// headersAdd implements :add(name, val) AND :append(name, val) (alias).
// Appends a new value to the header (does NOT replace existing values).
// Matches upstream HeaderMapWrapper::luaAdd / luaAppend which both
// wire to HeaderMap::addCopy (the "append-without-replace" semantics).
func headersAdd(L *lua.LState) int {
	h := getHeadersFromUD(L, 1)
	name := L.CheckString(2)
	val := L.CheckString(3)
	h.Add(name, val)
	return 0
}

// headersRemove implements :remove(name) — deletes the entire header
// entry (all values, not just the first). Matches upstream
// HeaderMapWrapper::luaRemove → HeaderMap::remove(LowerCaseString).
func headersRemove(L *lua.LState) int {
	h := getHeadersFromUD(L, 1)
	name := L.CheckString(2)
	h.Del(name)
	return 0
}

// headersReplace implements :replace(name, val) — removes-then-adds
// (single-value replace). Matches upstream HeaderMapWrapper::luaReplace
// → HeaderMap::setCopy (remove existing + addCopy new).
func headersReplace(L *lua.LState) int {
	h := getHeadersFromUD(L, 1)
	name := L.CheckString(2)
	val := L.CheckString(3)
	h.Set(name, val)
	return 0
}

// ---------------------------------------------------------------------
// Task 7 — 6 :logXxx methods (logTrace/logDebug/logInfo/logWarn/logErr/
// logCritical) per parent §7.1 + 22.1 PLAN/SPEC Task 7.
// ---------------------------------------------------------------------
//
// Wraps the Go stdlib "log" package — the canonical project log sink
// per extauthz.go:18 + extproc.go:26 + rbac.go:6 + router_h2.go:7 +
// extproc/processor.go:52. Format pin: "<LEVEL> lua: <msg>" prefix
// preserved across all 6 levels for log-greppability.
//
// Conservative coalesce: stdlib `log` has no native levels, so
// :logTrace and :logDebug both emit the "DEBUG" level prefix. If/when
// a structured log-leveling primitive is introduced cross-project,
// the per-method wiring here is replaced verbatim per its own ADR.
//
// Format-string safety: logAtLevel uses log.Printf("%s lua: %s",
// level, msg) — the user-supplied msg arrives at a %s arg position,
// NOT as a format string. Any %verbs in the msg are inert characters
// in the output (no format-string-injection attack surface).
//
// Per-handle separate stubs (rather than a single Go function
// registered under both maps) keep the L.CheckUserData(1) discipline
// crisp for future Tasks 8-9 extensions that may consult per-handle
// context (e.g. log-prefix scoping, request-id stamping).

// logAtLevel is the shared body for all 12 :logXxx methods (6 levels
// × {request, response} handle). Type-checks the receiver userdata,
// extracts the msg string arg, emits via log.Printf with the pinned
// "<LEVEL> lua: <msg>" format.
//
// The receiver is type-checked only for argument-position validity
// (L.CheckUserData raises a Lua-side error on non-userdata receiver);
// the value's concrete type is intentionally NOT asserted — log
// emission is context-free and works identically for both request and
// response handles. Future per-handle extensions (e.g. stamping the
// stream-id from a *requestHandleContext) would split per-handle
// helpers.
func logAtLevel(L *lua.LState, level string) int {
	_ = L.CheckUserData(1) // receiver — ArgError on type mismatch
	msg := L.CheckString(2)
	log.Printf("%s lua: %s", level, msg)
	return 0
}

// requestHandleLogTrace implements request_handle:logTrace(msg).
// Coalesces onto DEBUG level (stdlib log has no native trace level).
func requestHandleLogTrace(L *lua.LState) int { return logAtLevel(L, "DEBUG") }

// requestHandleLogDebug implements request_handle:logDebug(msg).
func requestHandleLogDebug(L *lua.LState) int { return logAtLevel(L, "DEBUG") }

// requestHandleLogInfo implements request_handle:logInfo(msg).
func requestHandleLogInfo(L *lua.LState) int { return logAtLevel(L, "INFO") }

// requestHandleLogWarn implements request_handle:logWarn(msg).
func requestHandleLogWarn(L *lua.LState) int { return logAtLevel(L, "WARN") }

// requestHandleLogErr implements request_handle:logErr(msg).
func requestHandleLogErr(L *lua.LState) int { return logAtLevel(L, "ERROR") }

// requestHandleLogCritical implements request_handle:logCritical(msg).
func requestHandleLogCritical(L *lua.LState) int { return logAtLevel(L, "CRIT") }

// responseHandleLogTrace implements response_handle:logTrace(msg).
// Encode-side parity for the 6 :logXxx methods — script authors may
// want to log from envoy_on_response.
func responseHandleLogTrace(L *lua.LState) int { return logAtLevel(L, "DEBUG") }

// responseHandleLogDebug implements response_handle:logDebug(msg).
func responseHandleLogDebug(L *lua.LState) int { return logAtLevel(L, "DEBUG") }

// responseHandleLogInfo implements response_handle:logInfo(msg).
func responseHandleLogInfo(L *lua.LState) int { return logAtLevel(L, "INFO") }

// responseHandleLogWarn implements response_handle:logWarn(msg).
func responseHandleLogWarn(L *lua.LState) int { return logAtLevel(L, "WARN") }

// responseHandleLogErr implements response_handle:logErr(msg).
func responseHandleLogErr(L *lua.LState) int { return logAtLevel(L, "ERROR") }

// responseHandleLogCritical implements response_handle:logCritical(msg).
func responseHandleLogCritical(L *lua.LState) int { return logAtLevel(L, "CRIT") }

// ---------------------------------------------------------------------
// __pairs metamethod — alphabetical-snapshot iterator (per §11.2 D7)
// ---------------------------------------------------------------------

// headersPairs implements the __pairs metamethod for the envoy_headers
// userdata. Snapshots the http.Header map into a slice of (k, v) pairs
// sorted case-insensitively by key (ties broken by lexicographic value
// order) at snapshot time, then returns a stateful iterator that walks
// the slice by integer index.
//
// Lua __pairs protocol (per Lua 5.2 spec + LuaJIT 5.2-compat + our
// installPairsShim): returns 3 values — (iterator_fn, state, init_ctrl).
// The iterator is repeatedly called with (state, last_ctrl) until it
// returns nil (or 0 values per gopher-lua's pairsaux pattern).
//
// Discipline per 22.1 SPEC §11.2 D7 resolution:
//   - Snapshot taken at __pairs invocation (NOT lazily during
//     iteration) — closes per-run non-determinism.
//   - Sort case-insensitively (strings.ToLower) — matches
//     net/http.Header.Write emit-order discipline.
//   - Ties broken by value lexicographic order — produces a stable
//     multi-value ordering even when http.Header.Values' slice ordering
//     is dependent on Add-insertion order.
//   - Walk by integer index — neither gopher-lua's nor LuaJIT's table
//     hash-iteration is in play.
//
// Determinism is verified at cross-run-determinism test (N=100 runs
// produce byte-identical iteration order).
func headersPairs(L *lua.LState) int {
	h := getHeadersFromUD(L, 1)

	// Snapshot the map into a (k, v) slice. Pre-size for typical
	// per-stream header counts to avoid reallocation in the common
	// case.
	type kv struct{ k, v string }
	snap := make([]kv, 0, len(h)*2)
	for k, vs := range h {
		for _, v := range vs {
			snap = append(snap, kv{k, v})
		}
	}

	// Sort alphabetically case-insensitive by key; tie-break by value
	// lexicographic to make multi-value ordering stable across runs.
	sort.Slice(snap, func(i, j int) bool {
		ki := strings.ToLower(snap[i].k)
		kj := strings.ToLower(snap[j].k)
		if ki != kj {
			return ki < kj
		}
		return snap[i].v < snap[j].v
	})

	// Closure-captured iterator state. The Lua-side pairs() shim
	// invokes us with the userdata; we hand back an iterator fn +
	// state + initial control. The state + control are not consulted
	// by our iterator (we close over i), but Lua's __pairs protocol
	// requires the 3-return shape so we still return state + ctrl
	// (LNil suffices for both).
	i := 0
	iter := L.NewFunction(func(L2 *lua.LState) int {
		if i >= len(snap) {
			L2.Push(lua.LNil)
			return 1
		}
		e := snap[i]
		i++
		L2.Push(lua.LString(e.k))
		L2.Push(lua.LString(e.v))
		return 2
	})
	L.Push(iter)
	L.Push(lua.LNil) // state (unused; closure captures snap + i)
	L.Push(lua.LNil) // initial control
	return 3
}

// ---------------------------------------------------------------------
// Task 8 — :streamInfo() 4-method subset per BRAINSTORM Q6 + parent §11.6
// + 22.1 SPEC §6 Task 8
// ---------------------------------------------------------------------
//
// `request_handle:streamInfo()` (and the symmetric
// `response_handle:streamInfo()`) returns a streamInfo userdata exposing
// 4 accessor methods sourced from the per-context cb field
// (RequestHandleCallbacks interface — see the interface definition at the
// top of this file for the framework-gap insulation rationale).
//
// The 4 methods return strings ready to push onto the Lua stack (the
// callback interface handles net.Addr → "ip:port" formatting + stubbing
// of framework gaps). Per the "always-string, never-nil" contract
// anchored at parent §11.6 (matching upstream's std::string return
// shape), each method emits an empty Lua-string when the underlying
// callback returns "" — never lua.LNil.
//
// Defensive nil-cb handling: if the context's cb field is nil (e.g.
// synthetic test invocation that did not wire the adapter), each accessor
// pushes the empty string rather than panicking on a nil-interface
// dereference. The 2 helper functions
// (streamInfoCallbacksFromUD + pushStreamInfoString) centralize the
// nil-cb fallback and the string-push convention.

// requestHandleStreamInfo implements request_handle:streamInfo().
// Returns a streamInfo userdata that wraps the per-context
// RequestHandleCallbacks. The returned userdata shares the same cb
// pointer — calling :protocol() / :routeName() / etc. on the userdata
// is observationally equivalent to calling the corresponding callback
// method on the request_handle's adapter.
func requestHandleStreamInfo(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*requestHandleContext)
	if !ok {
		L.ArgError(1, "expected request_handle")
		return 0
	}
	return pushStreamInfoUD(L, ctx.cb)
}

// responseHandleStreamInfo implements response_handle:streamInfo().
// Symmetric to requestHandleStreamInfo — wraps the response-handle's
// per-context callbacks. Script authors writing envoy_on_response may
// reasonably want to read stream metadata; the encode-side parity keeps
// the API symmetric. Both sides expose the same 4-method surface; the
// adapter in Task 9 wires the same per-stream callbacks for both
// handles.
func responseHandleStreamInfo(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*responseHandleContext)
	if !ok {
		L.ArgError(1, "expected response_handle")
		return 0
	}
	return pushStreamInfoUD(L, ctx.cb)
}

// pushStreamInfoUD allocates a fresh LUserData wrapping the supplied
// RequestHandleCallbacks + attaches the envoy_stream_info metatable +
// pushes it onto the stack. Returns 1 (number of pushed return values
// per LGFunction convention).
//
// The cb argument may be nil — the per-method accessor helpers
// (streamInfoCallbacksFromUD) tolerate a nil interface and surface
// empty strings for all 4 methods (per the defensive-nil discipline
// described in the section header).
func pushStreamInfoUD(L *lua.LState, cb RequestHandleCallbacks) int {
	ud := L.NewUserData()
	ud.Value = cb
	L.SetMetatable(ud, L.GetTypeMetatable(streamInfoTypeName))
	L.Push(ud)
	return 1
}

// streamInfoCallbacksFromUD type-checks + extracts the
// RequestHandleCallbacks from the userdata at the supplied stack index.
// Returns nil on nil cb (the wrapping userdata exists but holds a nil
// callbacks interface — synthetic test path). ArgError + nil-return on
// type mismatch (e.g. caller passed a request_handle userdata where a
// stream_info userdata was expected).
//
// Per the "always-string, never-nil" contract anchored at parent §11.6,
// the 4 accessor methods unconditionally push a string onto the stack —
// either the callback's value or the empty string when cb is nil.
func streamInfoCallbacksFromUD(L *lua.LState, idx int) RequestHandleCallbacks {
	ud := L.CheckUserData(idx)
	// nil interface case: the userdata was constructed with cb=nil
	// (e.g. test that did not wire the adapter). Return nil so the
	// per-method accessor pushes the empty string.
	if ud.Value == nil {
		return nil
	}
	cb, ok := ud.Value.(RequestHandleCallbacks)
	if !ok {
		L.ArgError(idx, "expected stream_info")
		return nil
	}
	return cb
}

// streamInfoProtocol implements streamInfo:protocol() — returns the
// HTTP protocol version string ("HTTP/1.0" / "HTTP/1.1" / "HTTP/2" /
// "HTTP/3" per upstream-parity literals; "" for synthetic streams that
// did not exercise HCM dispatch).
func streamInfoProtocol(L *lua.LState) int {
	cb := streamInfoCallbacksFromUD(L, 1)
	if cb == nil {
		L.Push(lua.LString(""))
		return 1
	}
	L.Push(lua.LString(cb.Protocol()))
	return 1
}

// streamInfoRouteName implements streamInfo:routeName() — returns the
// resolved route name string or "" if no route is selected / the
// framework adapter has no RouteName accessor (the phase-22.1
// condition — framework gap to be closed in a future phase per the
// RequestHandleCallbacks interface docstring).
func streamInfoRouteName(L *lua.LState) int {
	cb := streamInfoCallbacksFromUD(L, 1)
	if cb == nil {
		L.Push(lua.LString(""))
		return 1
	}
	L.Push(lua.LString(cb.RouteName()))
	return 1
}

// streamInfoDownstreamLocalAddress implements
// streamInfo:downstreamLocalAddress() — returns the listener's bound
// "ip:port" formatted via net.Addr.String() (the Task 9 adapter handles
// the formatting); "" for synthetic streams.
func streamInfoDownstreamLocalAddress(L *lua.LState) int {
	cb := streamInfoCallbacksFromUD(L, 1)
	if cb == nil {
		L.Push(lua.LString(""))
		return 1
	}
	L.Push(lua.LString(cb.DownstreamLocalAddress()))
	return 1
}

// streamInfoDownstreamDirectRemoteAddress implements
// streamInfo:downstreamDirectRemoteAddress() — returns the connecting
// peer's "ip:port" formatted via net.Addr.String() (the Task 9 adapter
// wires this to DownstreamRemoteAddr at phase 22.1; envoy-go has no
// proxy-chain origin distinction yet, so "remote" == "direct remote").
// "" for synthetic streams.
func streamInfoDownstreamDirectRemoteAddress(L *lua.LState) int {
	cb := streamInfoCallbacksFromUD(L, 1)
	if cb == nil {
		L.Push(lua.LString(""))
		return 1
	}
	L.Push(lua.LString(cb.DownstreamDirectRemoteAddress()))
	return 1
}

// ---------------------------------------------------------------------
// Task 9 — :respond byte-pin per parent §11.6.7 + AMEND-7 + AMEND-8
// ---------------------------------------------------------------------
//
// `request_handle:respond(headers_table, body_string)` short-circuits
// the decode chain to a SendLocalReply per parent §11.6.7. Semantics:
//
//  1. Extract `headers_table[":status"]`, parse as int. Validate
//     [200, 600) (i.e. inclusive [200, 599]). Out of range → byte-exact
//     ":status must be between 200-599" Lua error per AMEND-8.
//  2. Walk headers_table; collect non-`:` (i.e. non-pseudo-header)
//     entries into an OrderedHeaders carrier preserving Lua-table-walk
//     order (gopher-lua's L.ForEach walks a table's hash part; pure-Lua
//     tables have no stable iteration order per Lua 5.1 §2.5.7 — the
//     bridge does NOT impose ordering on the operator-supplied headers,
//     matching upstream's pure-pass-through behavior). Pseudo-headers
//     (`:scheme`, `:authority`, `:path`, `:method`) are SKIPPED (decode-
//     only; nonsensical on a synthetic local-reply).
//  3. If headers_table did NOT supply content-type, append
//     "content-type: text/plain" per upstream Utility::prepareLocalReply
//     at utility.cc:1241,1273.
//  4. If headers_table did NOT supply content-length, append
//     "content-length: <len(body)>" per parent §11.6.7 (utility.cc:1270).
//     Operator-supplied content-length is honored verbatim (no override).
//  5. Capture the (status, body, headers) tuple onto
//     requestHandleContext.respondCaptured. The decode_headers.go
//     dispatcher reads this AFTER the envoy_on_request hook returns and
//     emits dcb.SendLocalReply if non-nil.
//
// The function returns 0 to Lua (no return values); the operator's
// script CAN continue executing additional code after :respond() — the
// short-circuit happens at the Go-side dispatcher level, not by halting
// the VM. This matches upstream's behavior at lua_filter.cc:583 (the
// CallGlobal returns normally; the dispatcher inspects state.respond_
// captured before continuing).
//
// `response_handle:respond(...)` raises the byte-exact AMEND-8 wording
// "respond not currently supported in the response path" via
// L.RaiseError. The error propagates back through PCall and surfaces as
// *lua.ApiError to encode_headers.go.

// requestHandleRespond implements request_handle:respond(headers_table,
// body_string) per parent §11.6.7 + AMEND-7 + AMEND-8. Captures the
// validated (status, headers, body) tuple onto
// requestHandleContext.respondCaptured for the decode dispatcher to
// emit via SendLocalReply.
//
// Argument shape (matches upstream wrappers.cc respond signature):
//
//	rh:respond(headers_table, body_string)
//
// Where headers_table is a Lua table with the `[":status"]` pseudo-
// header set to a stringly-typed integer in [200, 599], plus any
// additional response headers. body_string is the body byte string
// (Go len() drives the auto-content-length).
func requestHandleRespond(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*requestHandleContext)
	if !ok {
		L.ArgError(1, "expected request_handle")
		return 0
	}
	headersTable := L.CheckTable(2)
	body := L.CheckString(3)

	// Step 1: extract + validate :status. The :status header value is
	// stringly-typed in upstream Lua scripts (per parent §11.6.7
	// example `[":status"]="403"`); we accept BOTH LString and LNumber
	// for flexibility (some script authors omit the quotes).
	statusLV := headersTable.RawGetString(statusPseudoHeader)
	statusInt, ok := parseRespondStatus(statusLV)
	if !ok || statusInt < 200 || statusInt >= 600 {
		L.RaiseError("%s", respondStatusOutOfRangeMsg)
		return 0
	}

	// Step 2: walk headers_table; collect non-pseudo-header entries into
	// an OrderedHeaders carrier. Track whether content-type +
	// content-length were operator-supplied so the defaults at steps 3+4
	// only fire when absent.
	out := make(envoyhttp.OrderedHeaders, 0, headersTable.Len())
	var sawContentType, sawContentLength bool
	headersTable.ForEach(func(k, v lua.LValue) {
		keyStr, ok := k.(lua.LString)
		if !ok {
			return // non-string key; skip silently (pure-Lua-table tolerance)
		}
		name := string(keyStr)
		// Skip ALL pseudo-headers (decode-only; :status was already
		// extracted above; the other 4 are nonsensical on a local-reply).
		if strings.HasPrefix(name, pseudoHeaderPrefix) {
			return
		}
		// Coerce value to string (Lua's tostring semantics — gopher-lua
		// LValue.String() handles LString, LNumber, LBool uniformly).
		val := v.String()
		// Track presence of content-type / content-length case-
		// insensitively per HTTP-spec discipline.
		switch strings.ToLower(name) {
		case contentTypeHeader:
			sawContentType = true
		case contentLengthHeader:
			sawContentLength = true
		}
		out = append(out, envoyhttp.HeaderField{Name: name, Value: val})
	})

	// Step 3: default content-type if absent. Per upstream
	// Utility::prepareLocalReply at utility.cc:1241,1273.
	if !sawContentType {
		out = append(out, envoyhttp.HeaderField{
			Name:  contentTypeHeader,
			Value: contentTypeDefault,
		})
	}

	// Step 4: auto-set content-length from len(body) if not operator-
	// supplied. Per parent §11.6.7 (utility.cc:1270). len(body) is the
	// BYTE length (Go strings are UTF-8 byte sequences; len() returns
	// bytes not runes — matches upstream's body_size).
	if !sawContentLength {
		out = append(out, envoyhttp.HeaderField{
			Name:  contentLengthHeader,
			Value: strconv.Itoa(len(body)),
		})
	}

	// Step 5: capture the respond state for the decode dispatcher.
	ctx.respondCaptured = &respondState{
		status:  statusInt,
		body:    body,
		headers: out,
	}
	return 0
}

// responseHandleRespond implements response_handle:respond(...) per
// AMEND-8: ALWAYS raises the byte-exact runtime-error wording "respond
// not currently supported in the response path". Matches upstream
// lua_filter.cc:1031-1034 luaL_error wording verbatim. The error
// propagates back through PCall and surfaces as *lua.ApiError to
// encode_headers.go, which increments cc.stats.errors + logs.
func responseHandleRespond(L *lua.LState) int {
	// Type-check the receiver for argument-position validity (ArgError
	// raises a Lua-side error on non-userdata receiver). Defensive: if
	// somehow the receiver is the wrong type, ArgError fires before
	// RaiseError — both surface as Lua errors so the encode dispatcher
	// catches either path.
	_ = L.CheckUserData(1)
	L.RaiseError("%s", respondNotInResponsePathMsg)
	return 0
}

// parseRespondStatus extracts an int from the :status header value.
// Accepts both lua.LString (the upstream-canonical shape per the
// example `[":status"]="403"`) AND lua.LNumber (for script authors
// who omit the quotes). Returns (statusInt, true) on success; (0,
// false) on type mismatch or unparseable string.
//
// The returned int may be out-of-range; the caller validates the
// [200, 600) constraint after parsing.
func parseRespondStatus(lv lua.LValue) (int, bool) {
	switch v := lv.(type) {
	case lua.LString:
		n, err := strconv.Atoi(string(v))
		if err != nil {
			return 0, false
		}
		return n, true
	case lua.LNumber:
		// Truncate fractional part (matches Lua's number-to-int idiom).
		return int(v), true
	default:
		return 0, false
	}
}

// ---------------------------------------------------------------------
// Task 9 — RequestHandleCallbacks adapter for the framework callbacks
// ---------------------------------------------------------------------
//
// The bridge's RequestHandleCallbacks interface (declared at the top of
// this file) is a small 4-method subset consumed by :streamInfo(). The
// framework's DecoderFilterCallbacks / EncoderFilterCallbacks have a
// LARGER surface that includes the 4 underlying primitives — but with
// some framework gaps (no RouteName accessor; no separate
// DownstreamDirectRemoteAddress vs DownstreamRemoteAddr). The adapters
// below project the framework callback surface onto the bridge
// interface, supplying stubs for the framework gaps per the docstring
// notes at parent §11.6 + 22.1 SPEC §6 Task 8.
//
// Both adapters live in bridge.go (not a separate adapter.go) because
// they are intimately bound to the RequestHandleCallbacks interface
// declaration; co-location keeps the contract + the satisfier in one
// reading-context.

// requestHandleCallbacksAdapter projects an envoyhttp.DecoderFilterCallbacks
// onto the RequestHandleCallbacks bridge interface. Framework gaps:
//   - RouteName — no DecoderFilterCallbacks accessor at phase 22.1;
//     returns "" (matches the per-method docstring at the interface
//     declaration). Future framework extension may add the accessor.
//   - DownstreamDirectRemoteAddress — re-uses DownstreamRemoteAddr (no
//     proxy-chain origin distinction in envoy-go yet, so "remote" ==
//     "direct remote").
type requestHandleCallbacksAdapter struct {
	dcb envoyhttp.DecoderFilterCallbacks
}

// newRequestHandleCallbacksAdapter constructs an adapter wrapping the
// supplied DecoderFilterCallbacks. The adapter satisfies the bridge's
// RequestHandleCallbacks interface; nil-tolerant — a nil dcb produces
// an adapter whose accessors all return the empty string (matches the
// "always-string, never-nil" contract at parent §11.6).
func newRequestHandleCallbacksAdapter(dcb envoyhttp.DecoderFilterCallbacks) RequestHandleCallbacks {
	return &requestHandleCallbacksAdapter{dcb: dcb}
}

// Protocol returns the framework's DownstreamProtocol verbatim.
func (a *requestHandleCallbacksAdapter) Protocol() string {
	if a.dcb == nil {
		return ""
	}
	return a.dcb.DownstreamProtocol()
}

// RouteName returns "" — framework gap at phase 22.1.
func (a *requestHandleCallbacksAdapter) RouteName() string { return "" }

// DownstreamLocalAddress returns the framework's DownstreamLocalAddr
// formatted via net.Addr.String(). Empty for nil-addr.
func (a *requestHandleCallbacksAdapter) DownstreamLocalAddress() string {
	if a.dcb == nil {
		return ""
	}
	addr := a.dcb.DownstreamLocalAddr()
	if addr == nil {
		return ""
	}
	return addr.String()
}

// DownstreamDirectRemoteAddress returns the framework's
// DownstreamRemoteAddr formatted via net.Addr.String(). Empty for
// nil-addr. Re-uses DownstreamRemoteAddr per the docstring note at
// the RequestHandleCallbacks interface declaration.
func (a *requestHandleCallbacksAdapter) DownstreamDirectRemoteAddress() string {
	if a.dcb == nil {
		return ""
	}
	addr := a.dcb.DownstreamRemoteAddr()
	if addr == nil {
		return ""
	}
	return addr.String()
}

// responseHandleCallbacksAdapter projects an envoyhttp.EncoderFilterCallbacks
// (with a fallback DecoderFilterCallbacks for RouteName-style accessors
// not present on the encoder side) onto the RequestHandleCallbacks
// bridge interface. Same framework-gap shape as the decode-side adapter.
//
// The two-arg constructor exists because the framework's encoder-side
// callbacks have FEWER methods than the decoder-side at some phase-
// boundaries (notably RouteName / per-route accessors), so the adapter
// can consult the decoder-side dcb (set during chain wiring per ADR-
// 0071 even on encode-only firing) for fields the encoder side doesn't
// expose. At 22.1 the 4 :streamInfo() accessors all live on BOTH dcb
// and ecb (the ADR-0174 callback-surface extension landed at phase-19.1),
// so the dcb fallback is purely defensive for the (rare) test case
// where dcb is wired but ecb is not.
type responseHandleCallbacksAdapter struct {
	ecb envoyhttp.EncoderFilterCallbacks
	dcb envoyhttp.DecoderFilterCallbacks // fallback for framework gaps
}

// newResponseHandleCallbacksAdapter constructs an adapter wrapping the
// supplied EncoderFilterCallbacks (+ optional DecoderFilterCallbacks
// fallback for framework gaps). Nil-tolerant on both arguments.
func newResponseHandleCallbacksAdapter(ecb envoyhttp.EncoderFilterCallbacks, dcb envoyhttp.DecoderFilterCallbacks) RequestHandleCallbacks {
	return &responseHandleCallbacksAdapter{ecb: ecb, dcb: dcb}
}

// Protocol returns the framework's DownstreamProtocol (encoder-side per
// ADR-0174 extension). Falls back to dcb on nil ecb.
func (a *responseHandleCallbacksAdapter) Protocol() string {
	if a.ecb != nil {
		return a.ecb.DownstreamProtocol()
	}
	if a.dcb != nil {
		return a.dcb.DownstreamProtocol()
	}
	return ""
}

// RouteName returns "" — framework gap at phase 22.1 (same as decode-
// side; no encoder-side or decoder-side RouteName accessor).
func (a *responseHandleCallbacksAdapter) RouteName() string { return "" }

// DownstreamLocalAddress returns the formatted local-addr from ecb
// (preferred) or dcb (fallback). Empty for nil-addr.
func (a *responseHandleCallbacksAdapter) DownstreamLocalAddress() string {
	if a.ecb != nil {
		if addr := a.ecb.DownstreamLocalAddr(); addr != nil {
			return addr.String()
		}
	}
	if a.dcb != nil {
		if addr := a.dcb.DownstreamLocalAddr(); addr != nil {
			return addr.String()
		}
	}
	return ""
}

// DownstreamDirectRemoteAddress returns the formatted remote-addr (re-
// used per the docstring note at the RequestHandleCallbacks interface
// declaration). Empty for nil-addr.
func (a *responseHandleCallbacksAdapter) DownstreamDirectRemoteAddress() string {
	if a.ecb != nil {
		if addr := a.ecb.DownstreamRemoteAddr(); addr != nil {
			return addr.String()
		}
	}
	if a.dcb != nil {
		if addr := a.dcb.DownstreamRemoteAddr(); addr != nil {
			return addr.String()
		}
	}
	return ""
}
