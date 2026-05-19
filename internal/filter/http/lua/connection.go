package lua

// connection.go — Task 10 (phase 22.2 IMPL) connection bridge per SPEC
// §3.4 + §11.5 D5 closure (option (f-B) cert-fingerprint-only cross-side)
// + PLAN Task 10 + ADR-0192 §Decision body anticipation.
//
// # Surface
//
//   - request_handle:connection() — returns a connection userdata wrapping
//     the per-stream *tls.ConnectionState sourced from the
//     RequestHandleCallbacks adapter's DownstreamTLSConnectionState()
//     accessor (Task 10 interface extension). The connection userdata
//     exposes a single primary method :ssl() per BRAINSTORM §2.4 scope
//     (Q4 closure: :connection() non-SSL methods like :remoteAddress are
//     OUT OF SCOPE; live on :streamInfo() already at 22.1).
//
//   - connection:ssl() — returns the ssl userdata wrapping the same
//     *tls.ConnectionState, OR lua.LNil for plaintext / non-TLS /
//     non-handshake-complete connections per ADR-0144 plumbing pattern.
//     The 12 ssl methods consume *tls.ConnectionState directly — see
//     ssl.go.
//
// # Plaintext / nil-state discipline
//
// Per SPEC §11.5.3 + ADR-0144 plumbing pattern, the chain-side
// tlsConnectionState field is non-nil ONLY on TLS connections with
// completed handshake. Plaintext + pre-handshake connections surface as
// nil from DownstreamTLSConnectionState(). The bridge:
//
//   - :connection() ALWAYS returns a callable userdata (matches upstream
//     ConnectionWrapper always-non-nil pattern — operator scripts may
//     check `if rh:connection() then ...` reliably; the wrapper itself is
//     non-nil even on plaintext).
//
//   - :ssl() returns lua.LNil when DownstreamTLSConnectionState is nil.
//     Matches upstream wrappers.cc connection:ssl() conditional push.
//
// # ADR-0192 cross-references
//
// ADR-0192 §Decision body anticipation: this file lands the connection-
// SSL bridge entry point per SPEC §3.4. The :sha256PeerCertificateDigest
// method on the ssl userdata is the CROSS-SIDE BYTE-EXACT method per
// §11.5 D5 closure (option f-B); the OTHER 11 ssl methods are subject-
// only test surface (implementation-specifically formatted: ISO-8601
// timezone, URL-encoded PEM ordering, cipher-suite-ID format).

import (
	"crypto/tls"

	lua "github.com/yuin/gopher-lua"
)

// connectionTypeName + sslTypeName are the metatable registry-keys used
// by gopher-lua's NewTypeMetatable for the connection + ssl userdata.
// Distinct registry keys preserve type-distinct identity for script
// authors who consult getmetatable(rh:connection()) or getmetatable(
// rh:connection():ssl()).
const (
	connectionTypeName = "envoy_connection"
	sslTypeName        = "envoy_ssl"
)

// connectionMethods is the method-name → LGFunction dispatch table for
// the connection userdata's __index metafield. Single primary method
// (:ssl) per BRAINSTORM §2.4 (Q4 closure scoped :connection() to the
// SSL accessor only; per-connection address surface lives on
// :streamInfo() per SPEC §2.11).
var connectionMethods = map[string]lua.LGFunction{
	"ssl": connectionSSL,
}

// installConnectionMetatable registers the metatable for the connection
// userdata under connectionTypeName per Task 10 (phase 22.2 IMPL) + SPEC
// §3.4 + §11.5. The metatable holds:
//   - __index → table of 1 connection method (:ssl)
//
// Per SPEC §11.5.3 + ADR-0144 plumbing pattern, :ssl() returns lua.LNil
// for plaintext / non-TLS / non-handshake-complete connections; on TLS
// returns the ssl userdata wrapping *tls.ConnectionState with the 12-
// method surface from ssl.go.
func installConnectionMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(connectionTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), connectionMethods))
	return mt
}

// requestHandleConnection implements request_handle:connection() per
// SPEC §3.4. Returns a connection userdata wrapping the per-stream
// *tls.ConnectionState sourced from the RequestHandleCallbacks adapter
// (DownstreamTLSConnectionState accessor — Task 10 interface extension).
//
// The wrapper's Value is the *tls.ConnectionState pointer (may be nil
// for plaintext — :ssl() consumes the nil and returns lua.LNil
// downstream). The wrapper itself is ALWAYS non-nil (matches upstream
// ConnectionWrapper always-non-nil pattern at v1.37.2 wrappers.cc).
func requestHandleConnection(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*requestHandleContext)
	if !ok {
		L.ArgError(1, "expected request_handle")
		return 0
	}
	var state *tls.ConnectionState
	if ctx.cb != nil {
		state = ctx.cb.DownstreamTLSConnectionState()
	}
	return pushConnectionUD(L, state)
}

// responseHandleConnection implements response_handle:connection()
// symmetric to the request side. Same wrapper construction + nil-tolerant
// :ssl() downstream. Per SPEC §3.4 the encode-side parity is maintained
// (script authors writing envoy_on_response may want to inspect the same
// TLS state).
func responseHandleConnection(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*responseHandleContext)
	if !ok {
		L.ArgError(1, "expected response_handle")
		return 0
	}
	var state *tls.ConnectionState
	if ctx.cb != nil {
		state = ctx.cb.DownstreamTLSConnectionState()
	}
	return pushConnectionUD(L, state)
}

// pushConnectionUD allocates a fresh LUserData wrapping the supplied
// *tls.ConnectionState (may be nil for plaintext) + attaches the
// envoy_connection metatable + pushes it onto the stack. Returns 1.
//
// The userdata's Value is the *tls.ConnectionState pointer (typed-nil
// possible on plaintext; the connectionSSL LGFunction checks for nil
// before constructing the ssl wrapper).
func pushConnectionUD(L *lua.LState, state *tls.ConnectionState) int {
	ud := L.NewUserData()
	ud.Value = state
	L.SetMetatable(ud, L.GetTypeMetatable(connectionTypeName))
	L.Push(ud)
	return 1
}

// connectionSSL implements connection:ssl() per SPEC §3.4 + §11.5
// D5 closure + ADR-0144 plumbing pattern. Returns lua.LNil when the
// wrapped *tls.ConnectionState is nil (plaintext / non-TLS / non-
// handshake-complete connection per ADR-0144 plumbing pattern; matches
// upstream wrappers.cc connection:ssl() conditional push). Returns the
// ssl userdata when state is non-nil — the ssl userdata exposes the
// 12-method surface from ssl.go (subjectPeerCertificate /
// subjectLocalCertificate / sanPeerCertificate / sanLocalCertificate /
// validFromPeerCertificate / expirationPeerCertificate / sessionId /
// ciphersuiteId / tlsVersion / urlEncodedPemEncodedPeerCertificate /
// urlEncodedPemEncodedPeerCertificateChain / sha256PeerCertificateDigest).
//
// nil-tolerant on the wrapper itself: type-mismatched userdata raises
// ArgError; nil Value is the documented plaintext signal.
func connectionSSL(L *lua.LState) int {
	ud := L.CheckUserData(1)
	state, ok := ud.Value.(*tls.ConnectionState)
	if !ok {
		L.ArgError(1, "expected connection")
		return 0
	}
	if state == nil {
		// Plaintext / non-TLS / non-handshake-complete per ADR-0144 plumbing
		// pattern. Returns lua.LNil to the Lua side; operator script idiom
		// `if rh:connection():ssl() then ... end` enters the false branch.
		L.Push(lua.LNil)
		return 1
	}
	return pushSSLUD(L, state)
}
