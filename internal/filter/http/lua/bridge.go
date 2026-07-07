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
	"crypto/tls"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	lua "github.com/yuin/gopher-lua"
	"github.com/yuin/gopher-lua/parse"

	"github.com/pgdad/envoy-go/internal/dynamicmetadata"
	envoyhttp "github.com/pgdad/envoy-go/internal/filter/http"
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
	// Task 8 (phase 22.2 IMPL) — distinct metatable registry-key for the
	// trailers userdata per SPEC §3.4 + §2.2. The trailers metatable
	// dispatches the SAME 7 mutation methods as the headers metatable +
	// the SAME __pairs alphabetical-snapshot iterator (reuses
	// installPairsShim discipline UNCHANGED from 22.1 per SPEC §11.2.2),
	// but is registered under a distinct registry key so Lua-side
	// `getmetatable(rh:trailers()) ~= getmetatable(rh:headers())` holds
	// (preserves type-distinct identity for script authors who care).
	trailersTypeName = "envoy_trailers"
	// Task 9 (phase 22.2 IMPL) — distinct metatable registry-keys for the
	// :metadata() callable-empty-userdata + :dynamicMetadata() Bucket
	// wrapper per SPEC §3.4 + §11.6 D1 closure + ADR-0192. See metadata.go
	// for the wrapper construction + the structpb.Value ↔ Lua-value
	// marshaling helpers.
	//
	//   - metadataTypeName — the :metadata() userdata. ALWAYS-CALLABLE
	//     EMPTY per §11.6 D1 closure: `:get(k)` returns lua.LNil for any
	//     key; pairs() yields zero iterations. Matches upstream
	//     MetadataMapWrapper always-non-nil pattern at v1.32.4 binding-gap.
	//   - dynamicMetadataTypeName — the :streamInfo():dynamicMetadata()
	//     userdata wrapping a *dynamicmetadata.Bucket. `:get(filterName,
	//     key)` + `:set(filterName, key, value)` round-trip via structpb.
	//     Value marshaling helpers (structpbToLua + luaToStructpb).
	metadataTypeName        = "envoy_metadata"
	dynamicMetadataTypeName = "envoy_dynamic_metadata"
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

	// DynamicMetadata returns the per-stream cross-filter dynamic-metadata
	// Bucket (per ADR-0190). Consumed by Task 9 (phase 22.2 IMPL) bridge
	// methods :streamInfo():dynamicMetadata() + :dynamicTypedMetadata() —
	// the streamInfo userdata's dispatch tables (streamInfoMethods) route
	// through this accessor to land the bucket pointer in the metadata-
	// bridge LGFunctions in metadata.go.
	//
	// Nil-tolerant per ADR-0085 — a nil *Bucket return surfaces at the
	// bridge as a no-op :set + nil :get (the bucket type itself is nil-
	// receiver-tolerant; the metadata-bridge LGFunctions additionally
	// short-circuit on nil to avoid pushing useless userdata into the Lua
	// stack — see dynamicMetadataFromUD in metadata.go).
	//
	// Task 13 (streaminfo.go extraction) extends this same dispatch surface
	// with 5 additional methods (:upstreamHost / :upstreamCluster /
	// :requestedServerName / :filterState / :downstreamSslConnection)
	// without re-touching this interface — the bridge:adapter contract is
	// stable at Task 9's addition.
	DynamicMetadata() *dynamicmetadata.Bucket

	// DownstreamTLSConnectionState returns the per-stream
	// *tls.ConnectionState observed by HCM dispatch at chain build time
	// (per Task 5 callbacks.go extension + Task 6 H1/H2 seeding). Returns
	// nil for plaintext / non-TLS / pre-handshake connections per ADR-0144
	// plumbing pattern.
	//
	// Consumed by the phase-22.2 lua bridge's :connection():ssl() surface
	// (Task 10) — the connection wrapper holds the state pointer; :ssl()
	// returns lua.LNil when the pointer is nil (plaintext) OR the 12-method
	// ssl userdata when non-nil (TLS). Task 13 additionally reuses this
	// accessor for :streamInfo():downstreamSslConnection() (symmetric
	// dispatch path).
	//
	// Per ADR-0192 §Decision body anticipation + SPEC §11.5.3 (chain-side
	// extension lives INSIDE ADR-0192 per Q13 WEAK HOLD — no separate ADR).
	// Cross-phase reusable for any future filter needing full TLS handshake
	// state.
	DownstreamTLSConnectionState() *tls.ConnectionState

	// UpstreamHost returns the resolved upstream endpoint formatted as
	// "host:port" (or just "host" when no port surface is available).
	// Empty string when no upstream has been selected (pre-router
	// dispatch / decode-only filters / local-reply paths). At phase 22.2
	// the framework adapter stubs this as "" because the project's
	// DecoderFilterCallbacks does NOT expose an upstream-host accessor
	// (framework gap; matches the RouteName pattern). Future framework
	// extension may light up the real value.
	//
	// Consumed by :streamInfo():upstreamHost() (Task 13).
	UpstreamHost() string

	// UpstreamCluster returns the cluster name from which the upstream
	// endpoint was selected. Same framework-gap disposition as
	// UpstreamHost — empty string at phase 22.2.
	//
	// Consumed by :streamInfo():upstreamCluster() (Task 13).
	UpstreamCluster() string

	// RequestedServerName returns the TLS SNI value surfaced by the
	// downstream handshake. The adapter sources this from the per-stream
	// *tls.ConnectionState.ServerName field (when the connection is TLS)
	// OR the empty string (plaintext / pre-handshake). Per SPEC §3.4 +
	// §11.5.3 ADR-0144 plumbing pattern.
	//
	// Consumed by :streamInfo():requestedServerName() (Task 13).
	RequestedServerName() string

	// FilterState returns the per-stream string-keyed `map[string]any`
	// stored on *filter (lua.go) per SPEC §11.8 D4 closure + AMEND-22.2-4.
	// May be nil at the start of a stream (lazy-allocated on first :set
	// from the bridge LGFunction). The adapter wires this to *filter's
	// filterState field via a closure capture at adapter construction
	// time (see newRequestHandleCallbacksAdapter + the *filter back-
	// pointer plumbing in decode_headers.go).
	//
	// Consumed by :streamInfo():filterState() (Task 13). The bridge
	// LGFunction (filterstate.go) wraps the returned map (via
	// filterStateRef) into a filterstate userdata exposing :get + :set
	// with the typed Lua-value marshaling table per AMEND-22.2-4.
	FilterState() map[string]any

	// SetFilterState installs a freshly-allocated map[string]any onto
	// the per-stream filterState field (lazy-allocation path from the
	// bridge LGFunction's :set on a nil map). The freshly-allocated
	// map is then observable via subsequent FilterState() calls (the
	// adapter writes it through to *filter.filterState).
	//
	// nil-tolerant on the underlying *filter back-pointer (synthetic
	// test path) — silent no-op.
	//
	// Consumed by the filterstate.go bridge LGFunction (filterStateSet)
	// for the lazy-allocation discipline.
	SetFilterState(m map[string]any)
}

// requestHandleContext is the Go-side state backing a request_handle
// userdata. One per per-stream filter dispatch (decode side). Task 8
// adds the cb field consumed by :streamInfo(); Task 9 extends with
// respond-state capture consumed by decode_headers.go. Task 7 (phase
// 22.2 IMPL) adds the filterRef back-pointer consumed by the body-
// bridge LGFunctions (:body() / :bodyChunks()) to resolve the owning
// *filter for body-buffer access + counter increments + coroutine
// suspend/resume orchestration.
type requestHandleContext struct {
	headers         http.Header
	cb              RequestHandleCallbacks // Task 8 — :streamInfo() accessor surface
	respondCaptured *respondState          // Task 9 — :respond() capture; nil = no respond fired
	filterRef       *filter                // Task 7 (22.2) — body-bridge back-pointer to *filter

	// trailers holds the per-stream request-side trailers map per Task 8
	// (phase 22.2 IMPL). Nil-valued until DecodeTrailers fires (lazy-
	// availability per SPEC §2.2 + PLAN Q2 — upstream Lua filter's
	// `trailers()` returns nil pre-trailers-arrival). When non-nil,
	// `request_handle:trailers()` returns a userdata wrapping this map
	// with the 7 mutation methods + __pairs alphabetical-snapshot
	// iterator.
	trailers http.Header
}

// responseHandleContext is the Go-side state backing a response_handle
// userdata. Symmetric to requestHandleContext for the encode side. Task
// 8 adds the cb field (response-side parity for :streamInfo()); Task 9
// adds NO respond-state capture (the encode-side :respond() raises a
// runtime error per AMEND-8 — no capture surface needed). Task 7 (phase
// 22.2 IMPL) adds the filterRef back-pointer for the body-bridge
// LGFunctions on the encode side (symmetric to requestHandleContext).
type responseHandleContext struct {
	headers   http.Header
	cb        RequestHandleCallbacks // Task 8 — :streamInfo() accessor surface (encode-side parity)
	filterRef *filter                // Task 7 (22.2) — body-bridge back-pointer to *filter (encode side)

	// trailers holds the per-stream response-side trailers map per Task 8
	// (phase 22.2 IMPL). Symmetric to requestHandleContext.trailers —
	// nil until EncodeTrailers fires. When non-nil,
	// `response_handle:trailers()` returns a userdata wrapping this map.
	trailers http.Header
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

// installTrailersMetatable registers the metatable for the trailers
// userdata under trailersTypeName per Task 8 (phase 22.2 IMPL) + SPEC
// §3.4 + §2.2. The metatable shape MIRRORS the headers metatable
// exactly (per BRAINSTORM §2.2 + SPEC §11.2.2 "trailers `pairs(udata)`
// CONFIRMED-IDENTICAL via 22.1 installPairsShim discipline"):
//
//   - __index → table of 7 trailers methods (the same headersMethods
//     dispatch table; trailers map values are http.Header so the methods
//     work identically — the type-distinct metatable just marks the
//     userdata as a trailers object at the Lua-script visible layer).
//   - __pairs → alphabetical-snapshot iterator (the same headersPairs
//     LGFunction; reads the underlying http.Header via the SAME
//     getHeadersFromUD helper).
//
// The pairs-shim (installPairsShim) is shared with headers — installed
// once per VM and dispatches __pairs for both envoy_headers AND
// envoy_trailers userdata.
func installTrailersMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(trailersTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), headersMethods))
	L.SetField(mt, "__pairs", L.NewFunction(headersPairs))
	return mt
}

// installStreamInfoMetatable lives at streaminfo.go (Task 13 extraction).

// installMetadataMetatable registers the metatable for the metadata
// userdata under metadataTypeName per Task 9 (phase 22.2 IMPL) + SPEC
// §3.4 + §11.6 D1 closure. The metatable holds:
//   - __index → table of 1 metadata method (:get)
//   - __pairs → zero-iteration iterator (empty-source per binding-gap)
//
// Per §11.6 D1 CLOSURE the metadata userdata is ALWAYS callable + ALWAYS
// returns empty (`:get(k)` returns lua.LNil for any k; pairs() yields
// zero iterations). Matches upstream MetadataMapWrapper always-non-nil
// pattern at v1.32.4 binding-gap. Source-of-data flips on at v1.32.4 →
// v1.37.x binding bump per parent AMEND-12 (future phase).
func installMetadataMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(metadataTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), metadataMethods))
	L.SetField(mt, "__pairs", L.NewFunction(metadataPairs))
	return mt
}

// installDynamicMetadataMetatable registers the metatable for the
// :streamInfo():dynamicMetadata() userdata under dynamicMetadataTypeName
// per Task 9 (phase 22.2 IMPL) + SPEC §3.4. The metatable holds:
//   - __index → table of 2 dynamicMetadata methods (:get / :set)
//
// The userdata wraps a *dynamicmetadata.Bucket. :get(filterName, key)
// returns the structpb.Value-marshaled Lua value or lua.LNil; :set(
// filterName, key, value) marshals the Lua value back to *structpb.Value
// and stores in the bucket. Both methods tolerate a nil Bucket per
// ADR-0085 (nil-receiver discipline).
func installDynamicMetadataMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(dynamicMetadataTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), dynamicMetadataMethods))
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
// switching between LGFunction registration + return-pushing. The shim
// source is a compile-time constant, so it is parsed + compiled ONCE at
// package init (pairsShimProto below); per-VM install is a cheap
// NewFunctionFromProto + PCall — no parser run per stream, identical
// Lua semantics (the same parse.Parse + lua.Compile pipeline that
// L.DoString would drive, with the same "<string>" chunk name so any
// script-observable error locations stay byte-identical).
func installPairsShim(L *lua.LState) {
	if pairsShimProto == nil {
		// Defensive: if for some reason the shim chunk failed to compile
		// at package init, fall back to the gopher-lua default pairs (the
		// userdata path will surface "attempt to call ... (a userdata
		// value)" at script-time, which is at least debuggable). We do
		// NOT panic here — the rest of the bridge surface is still
		// usable.
		return
	}

	// Stash the original pairs as __builtin_pairs (so the shim can
	// fall back when __pairs is absent — preserves stdlib pairs()
	// semantics for plain tables).
	origPairs := L.GetGlobal("pairs")
	L.SetGlobal("__builtin_pairs", origPairs)

	fn := L.NewFunctionFromProto(pairsShimProto)
	L.Push(fn)
	if err := L.PCall(0, 0, nil); err != nil {
		// Defensive: execution of the constant shim chunk cannot fail in
		// practice; mirror the historical DoString-error tolerance (keep
		// the VM usable with the default pairs).
		_ = err
	}
}

// pairsShimSrc is the Lua-side shim definition; safer than juggling
// LUserData wrap + closure capture from Go (Lua's native 5.2-compat
// pairs is 4 lines).
const pairsShimSrc = `
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

// pairsShimProto is the once-compiled bytecode for pairsShimSrc,
// produced at package init. A *lua.FunctionProto is bytecode-only
// (no LState-specific state) and is safe to share across N concurrent
// per-stream *lua.LStates via NewFunctionFromProto — same discipline
// as internal/lua's CompileScript chunk sharing. Nil only if the
// constant source failed to compile (never in practice; installPairsShim
// degrades to the gopher-lua default pairs in that case).
var pairsShimProto = func() *lua.FunctionProto {
	// "<string>" matches the chunk name L.DoString would stamp
	// (gopher-lua LoadString), keeping error-location strings from the
	// shim chunk byte-identical to the historical DoString path.
	ast, err := parse.Parse(strings.NewReader(pairsShimSrc), "<string>")
	if err != nil {
		return nil
	}
	proto, err := lua.Compile(ast, "<string>")
	if err != nil {
		return nil
	}
	return proto
}()

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
	// Task 7 (phase 22.2 IMPL) — :body() whole-buffer + :bodyChunks()
	// chunked iterator per SPEC §3.4 + §4.1 + §11.3 D3 closure (option
	// (a) defensive copy at endStream). Pre-endStream :body() suspends
	// via YieldFromBridge per §11.1 D2 closure; DecodeData at endStream
	// resumes with the accumulated bytes. Over-cap raises arm-21
	// runtime-reject byte-stable wording per W2.
	"body":       requestHandleBody,
	"bodyChunks": requestHandleBodyChunks,
	// Task 8 (phase 22.2 IMPL) — :trailers() returns a userdata wrapping
	// the request-side trailers map (envoy_trailers metatable) or nil if
	// trailers have not yet been received (lazy-availability per SPEC
	// §2.2 + PLAN Q2). The userdata exposes the SAME 7 mutation methods
	// + __pairs alphabetical-snapshot as the headers metatable, but is a
	// distinct Lua-side type. See trailers.go for the LGFunction body +
	// the trailersMethods dispatch table.
	"trailers": requestHandleTrailers,
	// Task 9 (phase 22.2 IMPL) — :metadata() returns ALWAYS-CALLABLE
	// EMPTY USERDATA per SPEC §11.6 D1 closure (NEVER nil; matches
	// upstream MetadataMapWrapper always-non-nil pattern at v1.32.4
	// binding-gap). The wrapper's :get(k) returns lua.LNil for any key;
	// pairs() yields zero iterations. See metadata.go.
	"metadata": requestHandleMetadata,
	// Task 10 (phase 22.2 IMPL) — :connection() returns a connection
	// userdata wrapping the per-stream *tls.ConnectionState (may be nil
	// for plaintext). The wrapper exposes a single primary :ssl()
	// method that returns lua.LNil on plaintext OR the 12-method ssl
	// userdata on TLS. Per BRAINSTORM §2.4 Q4 closure: :connection() is
	// scoped to the SSL accessor only (per-connection address surface
	// lives on :streamInfo() per SPEC §2.11). See connection.go + ssl.go.
	"connection": requestHandleConnection,
	// Task 11 (phase 22.2 IMPL) — :httpCall(cluster, headers, body,
	// timeout_ms, asynchronous?) cluster-based outbound HTTP dispatch
	// per SPEC §3.4 + §11.7 D6 closure + AMEND-22.2-3. FIRST CO-CONSUMER
	// of Task 4's ADR-0177 IN-PLACE AMENDMENT ClusterDispatch (R5
	// RATIFIED at Task 4 + actualized here). Sync path: coroutine yield +
	// Go-side dispatch + resume with (headers_table, body_string) on
	// success OR (nil, err_string) on failure. Async path (asynchronous=
	// true): PURE FIRE-AND-FORGET — spawns goroutine, returns 0 values
	// immediately, NO yield, response/error discarded per upstream
	// lua_filter.cc:400-416 noopCallbacks parity. Counters: httpcall_total
	// increments on every dispatch (sync + async); httpcall_failures +
	// httpcall_timeouts SYNC-ONLY per AMEND-22.2-3 (async failures
	// invisible at filter-stats per upstream parity). Arm-20 runtime-
	// reject byte-stable wording if cluster == "" per SPEC §6 + W2.
	// See httpcall.go.
	"httpCall": requestHandleHttpCall,
	// Task 12 (phase 22.2 IMPL) — 6 crypto methods + 2 misc methods
	// (:fileBytes + :timestamp) per SPEC §3.4 + §6 arm-22 + AMEND-22.2-1
	// + AMEND-22.2-2 + D8 closure at PLAN session. See crypto.go + misc.go.
	//
	// Classification per D8 PLAN closure:
	//   - :base64Escape    — upstream-parity (per AMEND-22.2-1)
	//   - :base64Decode    — envoy-go-strict (per D8 PLAN scrape)
	//   - :sha256          — envoy-go-strict (per D8 PLAN scrape)
	//   - :sha512          — envoy-go-strict (per D8 PLAN scrape)
	//   - :importPublicKey — upstream-parity (MIMICKING wrappers.h:415-427)
	//   - :verifySignature — upstream-parity (calling convention pinned
	//                        to upstream lua_filter.cc:611; 4 args ordered
	//                        as hash_algo, pubkey_wrapper, sig, text)
	//   - :fileBytes       — envoy-go-strict (per D8; NOT in upstream)
	//   - :timestamp       — envoy-go-strict (NOT in upstream
	//                        StreamHandleWrapper)
	//
	// 4 NEW envoy-go-strict BEHAVIOR_CONTRACT.md departure records
	// anticipated at Task 19 atomic landing for :base64Decode + :sha256 +
	// :sha512 + :fileBytes (bundle 7 → 11; :timestamp may also warrant a
	// record per Task 19 final scrape — PLAN settles).
	"base64Escape":    requestHandleBase64Escape,
	"base64Decode":    requestHandleBase64Decode,
	"sha256":          requestHandleSha256,
	"sha512":          requestHandleSha512,
	"importPublicKey": requestHandleImportPublicKey,
	"verifySignature": requestHandleVerifySignature,
	"fileBytes":       requestHandleFileBytes,
	"timestamp":       requestHandleTimestamp,
}

// responseHandleMethods mirrors requestHandleMethods for the response
// side. Same :headers method (returns response-side headers).
var responseHandleMethods = map[string]lua.LGFunction{
	"headers": responseHandleHeaders,
	// Task 7 — same 6 log methods on the encode side; script authors
	// may want to log from envoy_on_response. The request-side Go
	// functions are registered directly (the `append`→`add` alias below
	// sets the precedent, as do the crypto/misc delegating stubs):
	// logAtLevel only checks that the receiver IS a userdata, never its
	// concrete type, so dispatch is identical for both handles.
	"logTrace":    requestHandleLogTrace,
	"logDebug":    requestHandleLogDebug,
	"logInfo":     requestHandleLogInfo,
	"logWarn":     requestHandleLogWarn,
	"logErr":      requestHandleLogErr,
	"logCritical": requestHandleLogCritical,
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
	// Task 7 (phase 22.2 IMPL) — encode-side :body() + :bodyChunks()
	// symmetric to the request side. EncodeData accumulates response-
	// body bytes; pre-endStream :body() suspends via YieldFromBridge;
	// EncodeData at endStream resumes.
	"body":       responseHandleBody,
	"bodyChunks": responseHandleBodyChunks,
	// Task 8 (phase 22.2 IMPL) — encode-side :trailers() parity. Returns
	// the response-side trailers userdata or nil pre-EncodeTrailers
	// firing.
	"trailers": responseHandleTrailers,
	// Task 9 (phase 22.2 IMPL) — encode-side :metadata() parity. Symmetric
	// to the request-side; returns the same ALWAYS-CALLABLE EMPTY USERDATA
	// per §11.6 D1 closure.
	"metadata": responseHandleMetadata,
	// Task 10 (phase 22.2 IMPL) — encode-side :connection() parity.
	// Symmetric to the request-side; returns the same connection userdata
	// wrapping per-stream *tls.ConnectionState; :ssl() exposes the same 12
	// method surface. See connection.go + ssl.go.
	"connection": responseHandleConnection,
	// Task 11 (phase 22.2 IMPL) — encode-side :httpCall() parity. Symmetric
	// to the request-side; same cluster-based dispatch + sync/async
	// semantics + counter discipline. See httpcall.go.
	"httpCall": responseHandleHttpCall,
	// Task 12 (phase 22.2 IMPL) — encode-side crypto + misc parity per
	// SPEC §3.4. Symmetric to the request-side; same stateless dispatch
	// (the receiver type-check at index 1 is only a sanity-check — these
	// methods do not consult the handle context). See crypto.go + misc.go.
	"base64Escape":    responseHandleBase64Escape,
	"base64Decode":    responseHandleBase64Decode,
	"sha256":          responseHandleSha256,
	"sha512":          responseHandleSha512,
	"importPublicKey": responseHandleImportPublicKey,
	"verifySignature": responseHandleVerifySignature,
	"fileBytes":       responseHandleFileBytes,
	"timestamp":       responseHandleTimestamp,
}

// streamInfoMethods + installStreamInfoMetatable + the streamInfo
// userdata helpers (requestHandleStreamInfo / responseHandleStreamInfo
// / pushStreamInfoUD / streamInfoCallbacksFromUD) + the 11 LGFunction
// implementations all live at streaminfo.go (Task 13 phase 22.2 IMPL —
// extracted from this file as the streamInfo surface grew from 4
// methods (22.1) to 11 methods (22.2 phase-done) per SPEC §3.4 + PLAN
// Task 13). The metadata-bridge LGFunctions
// (streamInfoDynamicMetadata + streamInfoDynamicTypedMetadata) live at
// metadata.go since they consume the *dynamicmetadata.Bucket primitive
// from Task 1.

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

// pushHeaderLikeUD allocates a fresh LUserData wrapping the supplied
// http.Header + attaches the metatable registered under typeName +
// pushes it onto the stack. Returns 1 (number of pushed return values
// per LGFunction convention). Shared by pushHeadersUD (envoy_headers)
// and pushTrailersUD (envoy_trailers) — the two userdata kinds differ
// ONLY in the metatable registry key (a Lua-side type marker); the Go-
// side value is http.Header in both cases.
func pushHeaderLikeUD(L *lua.LState, h http.Header, typeName string) int {
	ud := L.NewUserData()
	ud.Value = h
	L.SetMetatable(ud, L.GetTypeMetatable(typeName))
	L.Push(ud)
	return 1
}

// pushHeadersUD allocates a fresh LUserData wrapping the supplied
// http.Header + attaches the envoy_headers metatable + pushes it onto
// the stack. Returns 1 (number of pushed return values per LGFunction
// convention).
func pushHeadersUD(L *lua.LState, h http.Header) int {
	return pushHeaderLikeUD(L, h, headersTypeName)
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
// The same 6 Go functions are registered under BOTH dispatch maps —
// log emission is context-free, so per-handle stubs would be pure
// duplication. A future extension that consults per-handle context
// (e.g. log-prefix scoping, request-id stamping) would split
// per-handle helpers at that point.

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

// The response_handle :logXxx entries reuse the request-side functions
// directly (registered in responseHandleMethods) — logAtLevel is
// receiver-type-agnostic, so no encode-side stubs are needed.

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
// Task 8 — :streamInfo() bridge extracted to streaminfo.go (Task 13)
// ---------------------------------------------------------------------
//
// requestHandleStreamInfo + responseHandleStreamInfo +
// pushStreamInfoUD + streamInfoCallbacksFromUD + the 11 per-method
// LGFunctions all live at streaminfo.go (per PLAN Task 13 extraction).
// The RequestHandleCallbacks interface definition (the bridge contract
// consumed by those LGFunctions) stays in bridge.go since it doubles as
// the framework:bridge seam (consumed by the adapters below).

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
//   - UpstreamHost / UpstreamCluster — no DecoderFilterCallbacks
//     accessor at phase 22.2; return "" (Task 13 framework gap; matches
//     the RouteName pattern).
//   - RequestedServerName — derived from
//     DownstreamTLSConnectionState().ServerName when non-nil; empty
//     for plaintext.
//   - FilterState — per-stream string-keyed `map[string]any` on the
//     *filter back-pointer (Task 13). Lazily-allocated on first :set
//     from the bridge LGFunction.
type requestHandleCallbacksAdapter struct {
	dcb envoyhttp.DecoderFilterCallbacks
	// filter is the per-stream *filter back-pointer for FilterState
	// access (Task 13). May be nil under ADR-0085 nil-tolerance at the
	// synthetic test path (the adapter's FilterState accessor returns
	// nil on nil filter — the bridge LGFunction tolerates the nil map).
	filter *filter
}

// newRequestHandleCallbacksAdapter constructs an adapter wrapping the
// supplied DecoderFilterCallbacks + *filter back-pointer for FilterState
// access. The adapter satisfies the bridge's RequestHandleCallbacks
// interface; nil-tolerant — a nil dcb produces an adapter whose
// accessors all return the empty string (matches the "always-string,
// never-nil" contract at parent §11.6). A nil *filter produces an
// adapter whose FilterState returns nil (defensive — the bridge
// LGFunction tolerates the nil map via filterStateRef.get returning
// nil).
func newRequestHandleCallbacksAdapter(dcb envoyhttp.DecoderFilterCallbacks, f *filter) RequestHandleCallbacks {
	return &requestHandleCallbacksAdapter{dcb: dcb, filter: f}
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

// DynamicMetadata returns the per-stream cross-filter Bucket via the
// framework's DecoderFilterCallbacks.DynamicMetadata accessor (per Task 5
// callbacks.go extension + ADR-0190). Returns nil when the underlying
// dcb is nil (synthetic test path) — the bridge metadata.go LGFunctions
// tolerate the nil-bucket case per ADR-0085 nil-receiver discipline.
func (a *requestHandleCallbacksAdapter) DynamicMetadata() *dynamicmetadata.Bucket {
	if a.dcb == nil {
		return nil
	}
	return a.dcb.DynamicMetadata()
}

// DownstreamTLSConnectionState returns the per-stream *tls.ConnectionState
// via the framework's DecoderFilterCallbacks.DownstreamTLSConnectionState
// accessor (per Task 5 callbacks.go extension + Task 6 H1/H2 seeding).
// Returns nil for plaintext / non-TLS / pre-handshake connections AND
// when the underlying dcb is nil (synthetic test path) — the bridge
// connection.go + ssl.go LGFunctions short-circuit to lua.LNil at
// connection:ssl() on nil-state.
func (a *requestHandleCallbacksAdapter) DownstreamTLSConnectionState() *tls.ConnectionState {
	if a.dcb == nil {
		return nil
	}
	return a.dcb.DownstreamTLSConnectionState()
}

// UpstreamHost returns "" — Task 13 framework gap. The project's
// DecoderFilterCallbacks does NOT expose an upstream-host accessor at
// phase 22.2. Future framework extension may add it.
func (a *requestHandleCallbacksAdapter) UpstreamHost() string { return "" }

// UpstreamCluster returns "" — Task 13 framework gap (same as
// UpstreamHost).
func (a *requestHandleCallbacksAdapter) UpstreamCluster() string { return "" }

// RequestedServerName derives the TLS SNI value from the per-stream
// *tls.ConnectionState.ServerName field. Returns "" for plaintext /
// pre-handshake connections per ADR-0144 plumbing pattern.
func (a *requestHandleCallbacksAdapter) RequestedServerName() string {
	if a.dcb == nil {
		return ""
	}
	state := a.dcb.DownstreamTLSConnectionState()
	if state == nil {
		return ""
	}
	return state.ServerName
}

// FilterState returns the per-stream `map[string]any` from the *filter
// back-pointer. May be nil (lazy-allocation discipline — the bridge
// LGFunction's :set lazy-init path makes the freshly-allocated map
// observable via subsequent :get calls on the same filterstate
// userdata). Returns nil if the back-pointer is nil (synthetic test
// path).
func (a *requestHandleCallbacksAdapter) FilterState() map[string]any {
	if a.filter == nil {
		return nil
	}
	return a.filter.filterState
}

// SetFilterState installs a freshly-allocated map onto the *filter's
// filterState field (lazy-allocation path from the bridge :set on a
// previously-nil map). Silent no-op on nil back-pointer.
func (a *requestHandleCallbacksAdapter) SetFilterState(m map[string]any) {
	if a.filter == nil {
		return
	}
	a.filter.filterState = m
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
//
// Task 13 adds a *filter back-pointer for FilterState access — same
// per-stream map shared across both encode + decode adapters (the
// *filter struct holds a single filterState field).
type responseHandleCallbacksAdapter struct {
	ecb envoyhttp.EncoderFilterCallbacks
	dcb envoyhttp.DecoderFilterCallbacks // fallback for framework gaps
	// filter is the per-stream *filter back-pointer for FilterState
	// access (Task 13). May be nil under ADR-0085 nil-tolerance at the
	// synthetic test path.
	filter *filter
}

// newResponseHandleCallbacksAdapter constructs an adapter wrapping the
// supplied EncoderFilterCallbacks (+ optional DecoderFilterCallbacks
// fallback for framework gaps + *filter back-pointer for FilterState).
// Nil-tolerant on all three arguments.
func newResponseHandleCallbacksAdapter(ecb envoyhttp.EncoderFilterCallbacks, dcb envoyhttp.DecoderFilterCallbacks, f *filter) RequestHandleCallbacks {
	return &responseHandleCallbacksAdapter{ecb: ecb, dcb: dcb, filter: f}
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

// DynamicMetadata returns the per-stream cross-filter Bucket. Symmetric
// to the decode-side adapter; ecb preferred, dcb fallback (both return
// the SAME per-stream bucket per ADR-0190 §Decision body — per-stream
// shared across decode + encode).
func (a *responseHandleCallbacksAdapter) DynamicMetadata() *dynamicmetadata.Bucket {
	if a.ecb != nil {
		if b := a.ecb.DynamicMetadata(); b != nil {
			return b
		}
	}
	if a.dcb != nil {
		return a.dcb.DynamicMetadata()
	}
	return nil
}

// DownstreamTLSConnectionState returns the per-stream *tls.ConnectionState.
// Symmetric to the decode-side adapter; ecb preferred, dcb fallback (both
// return the SAME per-stream state per ADR-0144 plumbing pattern — the
// chain-side tlsConnectionState field is set once at chain build time
// before either DecodeHeaders or EncodeHeaders fires). nil for plaintext.
func (a *responseHandleCallbacksAdapter) DownstreamTLSConnectionState() *tls.ConnectionState {
	if a.ecb != nil {
		if s := a.ecb.DownstreamTLSConnectionState(); s != nil {
			return s
		}
	}
	if a.dcb != nil {
		return a.dcb.DownstreamTLSConnectionState()
	}
	return nil
}

// UpstreamHost returns "" — Task 13 framework gap (encode-side parity).
func (a *responseHandleCallbacksAdapter) UpstreamHost() string { return "" }

// UpstreamCluster returns "" — Task 13 framework gap (encode-side parity).
func (a *responseHandleCallbacksAdapter) UpstreamCluster() string { return "" }

// RequestedServerName derives the TLS SNI value from the per-stream
// *tls.ConnectionState.ServerName field. Symmetric to the decode-side
// adapter — same plumbing per ADR-0144. Returns "" for plaintext.
func (a *responseHandleCallbacksAdapter) RequestedServerName() string {
	state := a.DownstreamTLSConnectionState()
	if state == nil {
		return ""
	}
	return state.ServerName
}

// FilterState returns the per-stream `map[string]any` from the *filter
// back-pointer. Same map shared across decode + encode adapters per the
// per-stream *filter discipline. Returns nil on nil back-pointer
// (synthetic test path).
func (a *responseHandleCallbacksAdapter) FilterState() map[string]any {
	if a.filter == nil {
		return nil
	}
	return a.filter.filterState
}

// SetFilterState installs a freshly-allocated map onto the *filter's
// filterState field (lazy-allocation path symmetric to the decode-side
// adapter). Silent no-op on nil back-pointer.
func (a *responseHandleCallbacksAdapter) SetFilterState(m map[string]any) {
	if a.filter == nil {
		return
	}
	a.filter.filterState = m
}
