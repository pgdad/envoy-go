package lua

// streaminfo.go — Task 13 (phase 22.2 IMPL) extraction + extension of
// the :streamInfo() bridge surface per SPEC §3.4 + PLAN Task 13 +
// ADR-0192 §Decision body anticipation.
//
// # Surface
//
// 11-method streamInfo userdata at 22.2 phase-done:
//
//   - 4 methods inherited from 22.1 Task 8 (parent §11.6 pragmatic-middle):
//     :protocol() / :routeName() / :downstreamLocalAddress() /
//     :downstreamDirectRemoteAddress()
//
//   - 2 methods added at 22.2 Task 9 (metadata bridge):
//     :dynamicMetadata() / :dynamicTypedMetadata(filterName) — the
//     LGFunction bodies live at metadata.go (consumes per-stream
//     *dynamicmetadata.Bucket via the RequestHandleCallbacks adapter).
//
//   - 5 methods added at 22.2 Task 13 (THIS FILE — anchor):
//     :upstreamHost() / :upstreamCluster() / :requestedServerName() /
//     :filterState() / :downstreamSslConnection()
//
// Per 22.2 SPEC §3.4 + BRAINSTORM Q8 + AMEND-22.2-4: this set is the
// FULL bridge surface at 22.2 phase-done. Each of the 5 NEW methods
// consumes a NEW RequestHandleCallbacks accessor (see bridge.go's
// interface declaration for the extended 10-method shape — Task 13
// adds UpstreamHost / UpstreamCluster / RequestedServerName /
// FilterState; DownstreamTLSConnectionState was already added at
// Task 10 and is reused here for :downstreamSslConnection symmetric
// to :connection():ssl()).
//
// # Extraction rationale
//
// Per PLAN Task 13 + the BRAINSTORM Q8 "11-method streamInfo at 22.2
// phase-done" target: the streamInfo surface is now sufficiently large
// (11 methods + 7 LGFunctions in THIS file + 2 metadata LGFunctions in
// metadata.go) to warrant a dedicated file. The metatable install +
// dispatch table + LGFunction implementations all live here.
// bridge.go retains the RequestHandleCallbacks interface + the adapter
// types (those are the bridge:framework contract; not streamInfo-
// specific).
//
// # AMEND-22.2-4 envoy-go-strict departures
//
// :filterState() is exposed as a userdata with BOTH :get(name) AND
// :set(name, value) methods; upstream is strictly read-only. Plus the
// :get return value is typed (LString / LNumber / LBool / LTable
// recursive) per the Lua-value marshaling table; upstream returns
// serializeAsString strings always. 2 envoy-go-strict departure records
// anticipated at BEHAVIOR_CONTRACT.md §13.6 (Task 19 atomic landing).

import (
	"crypto/tls"

	lua "github.com/yuin/gopher-lua"
)

// streamInfoMethods is the method-name → LGFunction dispatch table for
// the streamInfo userdata's __index metafield. 11 methods at 22.2
// phase-done:
//
//   - 4 inherited from 22.1 Task 8 (:protocol / :routeName /
//     :downstreamLocalAddress / :downstreamDirectRemoteAddress).
//   - 2 added at 22.2 Task 9 (:dynamicMetadata / :dynamicTypedMetadata —
//     LGFunction bodies in metadata.go).
//   - 5 added at 22.2 Task 13 (:upstreamHost / :upstreamCluster /
//     :requestedServerName / :filterState / :downstreamSslConnection).
//
// All LGFunctions tolerate a nil-cb receiver per the "always-string,
// never-nil" contract anchored at parent §11.6 (string-returning
// methods push the empty string on nil-cb).
var streamInfoMethods = map[string]lua.LGFunction{
	// 22.1 Task 8 — pragmatic-middle 4-method subset. All four are
	// plain string accessors built via streamInfoStringAccessor:
	//   - :protocol() — HTTP protocol version string ("HTTP/1.0" /
	//     "HTTP/1.1" / "HTTP/2" / "HTTP/3" per upstream-parity literals;
	//     "" for synthetic streams that did not exercise HCM dispatch).
	//   - :routeName() — resolved route name or "" if no route is
	//     selected / the framework adapter has no RouteName accessor.
	//   - :downstreamLocalAddress() — the listener's bound "ip:port"
	//     formatted via net.Addr.String(); "" for synthetic streams.
	//   - :downstreamDirectRemoteAddress() — the connecting peer's
	//     "ip:port" (envoy-go has no proxy-chain origin distinction yet,
	//     so "remote" == "direct remote"); "" for synthetic streams.
	"protocol":                      streamInfoStringAccessor(RequestHandleCallbacks.Protocol),
	"routeName":                     streamInfoStringAccessor(RequestHandleCallbacks.RouteName),
	"downstreamLocalAddress":        streamInfoStringAccessor(RequestHandleCallbacks.DownstreamLocalAddress),
	"downstreamDirectRemoteAddress": streamInfoStringAccessor(RequestHandleCallbacks.DownstreamDirectRemoteAddress),
	// 22.2 Task 9 — metadata bridge (bodies in metadata.go)
	"dynamicMetadata":      streamInfoDynamicMetadata,
	"dynamicTypedMetadata": streamInfoDynamicTypedMetadata,
	// 22.2 Task 13 — full bridge surface completion. The three string
	// accessors follow the same factory pattern:
	//   - :upstreamHost() — the resolved upstream endpoint as
	//     "host:port"; "" when no upstream has been selected. At phase
	//     22.2 the framework adapter stubs this as "" (framework gap;
	//     matches the RouteName pattern).
	//   - :upstreamCluster() — the cluster name from which the upstream
	//     endpoint was selected; same framework-gap disposition.
	//   - :requestedServerName() — the TLS SNI server name surfaced by
	//     the downstream handshake (per-stream
	//     *tls.ConnectionState.ServerName) or "" on plaintext /
	//     pre-handshake per SPEC §3.4 + §11.5.3 ADR-0144 plumbing.
	"upstreamHost":            streamInfoStringAccessor(RequestHandleCallbacks.UpstreamHost),
	"upstreamCluster":         streamInfoStringAccessor(RequestHandleCallbacks.UpstreamCluster),
	"requestedServerName":     streamInfoStringAccessor(RequestHandleCallbacks.RequestedServerName),
	"filterState":             streamInfoFilterState,
	"downstreamSslConnection": streamInfoDownstreamSslConnection,
}

// installStreamInfoMetatable registers the metatable for the streamInfo
// userdata under streamInfoTypeName. The metatable holds:
//   - __index → table of 11 streamInfo methods (see streamInfoMethods)
//
// Per 22.1 SPEC §6 Task 8 + parent §11.6 + 22.2 SPEC §3.4: the per-
// stream-VM setup (decode_headers.go) calls this helper once at NewVM
// time alongside the other install* helpers. The streamInfo userdata
// is allocated on-demand inside request_handle:streamInfo() — it wraps
// a RequestHandleCallbacks (the interface the adapter satisfies in
// bridge.go), NOT a *requestHandleContext — since all 11 accessor
// methods consume the callback shape exclusively.
func installStreamInfoMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(streamInfoTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), streamInfoMethods))
	return mt
}

// ---------------------------------------------------------------------
// streamInfo userdata construction helpers
// ---------------------------------------------------------------------

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
// the API symmetric. Both sides expose the same 11-method surface; the
// adapter wires the same per-stream callbacks for both handles.
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
// empty strings for all string-returning methods (per the defensive-nil
// discipline anchored at parent §11.6).
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
// the string-returning accessor methods unconditionally push a string
// onto the stack — either the callback's value or the empty string when
// cb is nil.
func streamInfoCallbacksFromUD(L *lua.LState, idx int) RequestHandleCallbacks {
	ud := L.CheckUserData(idx)
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

// ---------------------------------------------------------------------
// String-accessor factory (4 inherited 22.1 Task 8 methods + 3 of the
// 5 NEW Task 13 methods — per-method semantics documented at the
// streamInfoMethods dispatch table above)
// ---------------------------------------------------------------------

// streamInfoStringAccessor builds the LGFunction for a string-returning
// streamInfo accessor. All seven string accessors share the identical
// nil-cb-then-push-string body, differing only in which
// RequestHandleCallbacks method supplies the value — the get parameter
// is typically a method expression (e.g.
// RequestHandleCallbacks.Protocol).
//
// Per the "always-string, never-nil" contract anchored at parent §11.6:
// a nil cb (synthetic test path) pushes the empty string; otherwise the
// callback's value is pushed verbatim.
func streamInfoStringAccessor(get func(RequestHandleCallbacks) string) lua.LGFunction {
	return func(L *lua.LState) int {
		cb := streamInfoCallbacksFromUD(L, 1)
		if cb == nil {
			L.Push(lua.LString(""))
			return 1
		}
		L.Push(lua.LString(get(cb)))
		return 1
	}
}

// ---------------------------------------------------------------------
// 2 non-string Task 13 methods
// ---------------------------------------------------------------------

// streamInfoFilterState implements streamInfo:filterState() — returns
// a filterstate userdata wrapping the per-stream `map[string]any` per
// SPEC §3.4 + §11.8 D4 closure + AMEND-22.2-4. The userdata exposes
// 2 methods:
//
//   - :get(name)        → marshaled Lua value (typed per AMEND-22.2-4)
//   - :set(name, value) → inverse marshal + store (envoy-go-strict per
//     AMEND-22.2-4 — upstream is strictly read-only)
//
// Per-stream lifecycle: the underlying map[string]any lives on *filter
// (lua.go); accessed via the cb's FilterState() accessor. Cleared at
// *filter.OnDestroy. Cross-stream isolation is via the per-stream
// *filter — no shared state.
//
// Lazy-allocation: the bridge LGFunction's :set lazy-init path requires
// a writeback channel to the *filter struct. The filterStateRef
// constructed here uses a setter that consults the *requestHandleContext
// (or *responseHandleContext) via the userdata's filterRef back-pointer
// — but we cannot reach the *filter from the streamInfo userdata's
// RequestHandleCallbacks alone. Workaround: pass the cb adapter to a
// filterStateRef whose get() always re-reads from cb.FilterState() AND
// whose set() needs a writeback — handled by *filter back-pointer
// adapters (requestHandleCallbacksAdapter has a filter field; we cast
// up via a small interface in the helper below).
//
// nil-cb tolerance: a nil cb yields a userdata wrapping a nil map; the
// filterstate :get short-circuits to lua.LNil, :set is silently dropped
// (matches ADR-0085 nil-receiver discipline).
func streamInfoFilterState(L *lua.LState) int {
	cb := streamInfoCallbacksFromUD(L, 1)
	return pushFilterStateUDFromCb(L, cb)
}

// streamInfoDownstreamSslConnection implements
// streamInfo:downstreamSslConnection() — symmetric to
// :connection():ssl() (connection.go). Returns the ssl userdata
// wrapping the per-stream *tls.ConnectionState OR lua.LNil for
// plaintext / non-TLS / non-handshake-complete connections per ADR-0144
// plumbing pattern.
//
// Per SPEC §3.4 + §11.5.3: the SAME 12 ssl methods (subjectPeerCertificate
// / sanPeerCertificate / sha256PeerCertificateDigest / etc.) are
// callable on the returned userdata. The two access paths
// (:connection():ssl() and :streamInfo():downstreamSslConnection())
// route through the SAME pushSSLUD helper and produce
// observationally-identical results.
func streamInfoDownstreamSslConnection(L *lua.LState) int {
	cb := streamInfoCallbacksFromUD(L, 1)
	var state *tls.ConnectionState
	if cb != nil {
		state = cb.DownstreamTLSConnectionState()
	}
	if state == nil {
		L.Push(lua.LNil)
		return 1
	}
	return pushSSLUD(L, state)
}
