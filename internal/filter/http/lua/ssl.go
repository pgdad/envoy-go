package lua

// ssl.go — Task 10 (phase 22.2 IMPL) ssl wrapper 12-method surface per
// SPEC §3.4 + §11.5 D5 closure (option (f-B) cert-fingerprint-only
// cross-side) + PLAN Task 10 + BRAINSTORM §2.4 + ADR-0192 §Decision body
// anticipation.
//
// # Surface — 12 methods consuming *tls.ConnectionState directly
//
//   1. :subjectPeerCertificate()    — peer cert Subject DN string
//      (Go x509 pkix.Name.String() shape: "CN=...,O=...").
//   2. :subjectLocalCertificate()   — empty string (Go's
//      tls.ConnectionState exposes no symmetric LocalCertificate field;
//      envoy-go-strict per BEHAVIOR_CONTRACT departure record at Task 19
//      atomic landing — the "always-string, never-nil" contract holds).
//   3. :sanLocalCertificate()       — empty Lua table (symmetric to
//      :subjectLocalCertificate — Go tls.ConnectionState has no local
//      cert surface).
//   4. :sanPeerCertificate()        — Lua table of SAN strings from
//      PeerCertificates[0].DNSNames + URIs.String() + IPAddresses.String()
//      + EmailAddresses (combined; matches upstream
//      uriSanPeerCertificate + dnsSansPeerCertificate semantically).
//   5. :validFromPeerCertificate()  — RFC3339 string from
//      PeerCertificates[0].NotBefore.
//   6. :expirationPeerCertificate() — RFC3339 string from
//      PeerCertificates[0].NotAfter.
//   7. :sessionId()                 — lowercase hex of
//      tls.ConnectionState.TLSUnique (RFC 5929 channel binding; Go's
//      idiomatic session-identity surface).
//   8. :ciphersuiteId()             — uint16 numeric of
//      tls.ConnectionState.CipherSuite (upstream-parity wire shape).
//   9. :tlsVersion()                — string "TLSv1.0"/"TLSv1.1"/
//      "TLSv1.2"/"TLSv1.3" mapped from tls.ConnectionState.Version per
//      SPEC §11.5.4 wire-format convention.
//  10. :urlEncodedPemEncodedPeerCertificate()
//                                   — url.QueryEscape of pem.Encode(
//      &pem.Block{Type:"CERTIFICATE", Bytes: PeerCertificates[0].Raw}).
//  11. :urlEncodedPemEncodedPeerCertificateChain()
//                                   — url.QueryEscape of the
//      concatenated PEM-encoded full PeerCertificates chain.
//  12. :sha256PeerCertificateDigest() — CROSS-SIDE BYTE-EXACT per D5
//      (f-B) — lowercase hex of sha256.Sum256(PeerCertificates[0].Raw)
//      via fmt.Sprintf("%x", sum). Matches SPEC §11.5.4 wire-format
//      convention verbatim.
//
// # Cross-side byte-exact discipline (D5 closure option f-B)
//
// Only :sha256PeerCertificateDigest is byte-exact cross-side reference-
// Envoy ↔ envoy-go (fixture-0027 scenario (f-B)). The other 11 methods
// are subject-only test surface — implementation-specifically formatted
// (Go's x509 pkix.Name.String() shape differs from OpenSSL's DN print;
// Go's RFC3339 timezone differs from OpenSSL's UTC-Z; Go's CipherSuite
// uint16 IDs match upstream IANA but Go's preferred suite enumeration
// differs). Per D5 closure option (f-B): cross-side fixture compares
// ONLY the sha256 digest; the other methods are exercised subject-side
// for shape/wire-format correctness without cross-side byte-comparison.
//
// # nil-tolerance discipline
//
// The 12 methods are all NIL-TOLERANT on the wrapped *tls.ConnectionState
// pointer + on the underlying PeerCertificates slice. The connection.go
// caller (connectionSSL) guards against nil-state and short-circuits to
// lua.LNil BEFORE constructing the ssl userdata — so under normal flow
// the ssl userdata's Value is non-nil. The defensive nil-checks in this
// file cover synthetic test paths that might construct the wrapper
// directly with a nil state, plus the "TLS state present but no peer
// cert" path (server-side without client auth — PeerCertificates is
// empty; subject/san/validFrom/expiration/sha/pem/chain return empty
// defaults).

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"net/url"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// sslMethods is the method-name → LGFunction dispatch table for the ssl
// userdata's __index metafield. 12 methods per SPEC §3.4 + BRAINSTORM
// §2.4 + PLAN Task 10. Each method consumes the wrapped
// *tls.ConnectionState directly; all are nil-tolerant on the state
// pointer + the PeerCertificates slice + the individual cert fields.
var sslMethods = map[string]lua.LGFunction{
	"subjectPeerCertificate":                   sslSubjectPeerCertificate,
	"subjectLocalCertificate":                  sslSubjectLocalCertificate,
	"sanPeerCertificate":                       sslSanPeerCertificate,
	"sanLocalCertificate":                      sslSanLocalCertificate,
	"validFromPeerCertificate":                 sslValidFromPeerCertificate,
	"expirationPeerCertificate":                sslExpirationPeerCertificate,
	"sessionId":                                sslSessionID,
	"ciphersuiteId":                            sslCiphersuiteID,
	"tlsVersion":                               sslTLSVersion,
	"urlEncodedPemEncodedPeerCertificate":      sslURLEncodedPemEncodedPeerCertificate,
	"urlEncodedPemEncodedPeerCertificateChain": sslURLEncodedPemEncodedPeerCertificateChain,
	"sha256PeerCertificateDigest":              sslSHA256PeerCertificateDigest,
}

// installSSLMetatable registers the metatable for the ssl userdata under
// sslTypeName per Task 10 (phase 22.2 IMPL) + SPEC §3.4 + §11.5. The
// metatable holds:
//   - __index → table of 12 ssl methods (see sslMethods).
//
// Per SPEC §11.5.3 + ADR-0144 plumbing pattern, the ssl userdata is
// constructed ONLY when the per-stream *tls.ConnectionState is non-nil
// (plaintext path short-circuits to lua.LNil at connection:ssl()).
func installSSLMetatable(L *lua.LState) *lua.LTable {
	mt := L.NewTypeMetatable(sslTypeName)
	L.SetField(mt, "__index", L.SetFuncs(L.NewTable(), sslMethods))
	return mt
}

// pushSSLUD allocates a fresh LUserData wrapping the supplied
// *tls.ConnectionState + attaches the envoy_ssl metatable + pushes it
// onto the stack. Returns 1.
//
// The state pointer SHOULD be non-nil at the normal flow (the caller
// connectionSSL guards against nil). Defensive nil-handling in the 12
// methods covers synthetic test paths.
func pushSSLUD(L *lua.LState, state *tls.ConnectionState) int {
	ud := L.NewUserData()
	ud.Value = state
	L.SetMetatable(ud, L.GetTypeMetatable(sslTypeName))
	L.Push(ud)
	return 1
}

// sslStateFromUD type-checks + extracts the *tls.ConnectionState from
// the userdata at the supplied stack index. Returns nil on:
//   - ud.Value is nil (nil-state-wrapping userdata — synthetic test
//     path; the normal flow short-circuits at connection:ssl() before
//     constructing this wrapper).
//   - ud.Value is a typed nil *tls.ConnectionState.
//
// ArgError + nil-return on type mismatch (e.g., script passed a
// connection userdata where an ssl userdata was expected).
func sslStateFromUD(L *lua.LState, idx int) *tls.ConnectionState {
	ud := L.CheckUserData(idx)
	if ud.Value == nil {
		return nil
	}
	s, ok := ud.Value.(*tls.ConnectionState)
	if !ok {
		L.ArgError(idx, "expected ssl")
		return nil
	}
	return s
}

// peerCertFromState returns the first peer certificate from the supplied
// *tls.ConnectionState or nil if state is nil OR PeerCertificates is
// empty. Centralizes the nil-tolerant peer-cert lookup so the 9 methods
// that consume PeerCertificates[0] share the same guard.
func peerCertFromState(state *tls.ConnectionState) *x509.Certificate {
	if state == nil {
		return nil
	}
	if len(state.PeerCertificates) == 0 {
		return nil
	}
	return state.PeerCertificates[0]
}

// ---------------------------------------------------------------------
// Method 1: subjectPeerCertificate — peer cert Subject DN string
// ---------------------------------------------------------------------

// sslSubjectPeerCertificate returns the Subject DN of
// PeerCertificates[0] formatted via Go's x509 pkix.Name.String() (e.g.,
// "CN=test.local,O=envoy-go"). Empty string when no peer cert is
// available (plaintext / no-client-auth / synthetic test).
//
// envoy-go-strict note: Go's x509 pkix.Name.String() formats the DN in
// reverse-RDN order (most-specific-first) with comma separation;
// upstream Envoy uses OpenSSL's X509_NAME_oneline which may produce a
// different ordering / separator. Per D5 (f-B) closure this method is
// subject-only test surface — NOT cross-side byte-exact.
func sslSubjectPeerCertificate(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	cert := peerCertFromState(state)
	if cert == nil {
		L.Push(lua.LString(""))
		return 1
	}
	L.Push(lua.LString(cert.Subject.String()))
	return 1
}

// ---------------------------------------------------------------------
// Method 2: subjectLocalCertificate — empty per Go tls.ConnectionState
// ---------------------------------------------------------------------

// sslSubjectLocalCertificate returns the empty string. Go's
// tls.ConnectionState exposes no LocalCertificate symmetric to
// PeerCertificates — the local cert is held in the underlying tls.Conn's
// Config.Certificates slice but is NOT exposed via ConnectionState
// (server-side perspective: the server's own cert is config-known to
// operators; client-side perspective: there's no Go API to surface it
// post-handshake without re-reading the Config).
//
// envoy-go-strict per BEHAVIOR_CONTRACT departure record at Task 19
// atomic landing — the "always-string, never-nil" contract holds (empty
// string, never lua.LNil). Future framework extension may surface the
// local cert via a new accessor.
func sslSubjectLocalCertificate(L *lua.LState) int {
	_ = sslStateFromUD(L, 1) // receiver — ArgError on type mismatch
	L.Push(lua.LString(""))
	return 1
}

// ---------------------------------------------------------------------
// Method 3: sanPeerCertificate — Lua table of combined DNS+URI+IP+email SANs
// ---------------------------------------------------------------------

// sslSanPeerCertificate returns a Lua table containing the SAN entries
// from PeerCertificates[0]: DNSNames + URIs.String() + IPAddresses.String()
// + EmailAddresses (combined into a single 1-indexed list).
//
// Upstream Envoy splits SANs into uriSan* + dnsSans* + (no email/IP
// variants); envoy-go-strict combines them under :sanPeerCertificate
// per BRAINSTORM §2.4 method-name convention. Returns an empty table
// when no peer cert is available.
func sslSanPeerCertificate(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	cert := peerCertFromState(state)
	tbl := L.NewTable()
	if cert == nil {
		L.Push(tbl)
		return 1
	}
	for _, dns := range cert.DNSNames {
		tbl.Append(lua.LString(dns))
	}
	for _, uri := range cert.URIs {
		if uri == nil {
			continue
		}
		tbl.Append(lua.LString(uri.String()))
	}
	for _, ip := range cert.IPAddresses {
		tbl.Append(lua.LString(ip.String()))
	}
	for _, email := range cert.EmailAddresses {
		tbl.Append(lua.LString(email))
	}
	L.Push(tbl)
	return 1
}

// ---------------------------------------------------------------------
// Method 4: sanLocalCertificate — empty Lua table per Go tls.ConnectionState
// ---------------------------------------------------------------------

// sslSanLocalCertificate returns an empty Lua table. Symmetric to
// :subjectLocalCertificate — Go's tls.ConnectionState has no local cert
// surface. envoy-go-strict per BEHAVIOR_CONTRACT departure record at
// Task 19 atomic landing.
func sslSanLocalCertificate(L *lua.LState) int {
	_ = sslStateFromUD(L, 1) // receiver — ArgError on type mismatch
	tbl := L.NewTable()
	L.Push(tbl)
	return 1
}

// ---------------------------------------------------------------------
// Method 5: validFromPeerCertificate — RFC3339 string
// ---------------------------------------------------------------------

// sslValidFromPeerCertificate returns the NotBefore timestamp of
// PeerCertificates[0] formatted via time.RFC3339 (e.g.,
// "2024-01-01T00:00:00Z") per SPEC §11.5.4 wire-format convention.
// Empty string when no peer cert is available.
func sslValidFromPeerCertificate(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	cert := peerCertFromState(state)
	if cert == nil {
		L.Push(lua.LString(""))
		return 1
	}
	L.Push(lua.LString(cert.NotBefore.UTC().Format(time.RFC3339)))
	return 1
}

// ---------------------------------------------------------------------
// Method 6: expirationPeerCertificate — RFC3339 string
// ---------------------------------------------------------------------

// sslExpirationPeerCertificate returns the NotAfter timestamp of
// PeerCertificates[0] formatted via time.RFC3339. Empty string when no
// peer cert is available.
func sslExpirationPeerCertificate(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	cert := peerCertFromState(state)
	if cert == nil {
		L.Push(lua.LString(""))
		return 1
	}
	L.Push(lua.LString(cert.NotAfter.UTC().Format(time.RFC3339)))
	return 1
}

// ---------------------------------------------------------------------
// Method 7: sessionId — lowercase hex of TLSUnique
// ---------------------------------------------------------------------

// sslSessionID returns the lowercase hex encoding of
// tls.ConnectionState.TLSUnique (RFC 5929 channel-binding bytes — Go's
// idiomatic session-identity surface; the closest analog to OpenSSL's
// SSL_get_session_id). Empty string when state is nil or TLSUnique is
// empty.
//
// envoy-go-strict note: upstream Envoy's :sessionId() returns the raw
// SSL session ID (OpenSSL SSL_get_session_id) which is a different byte
// sequence than Go's TLSUnique. Per D5 (f-B) closure this method is
// subject-only test surface — NOT cross-side byte-exact.
func sslSessionID(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	if state == nil || len(state.TLSUnique) == 0 {
		L.Push(lua.LString(""))
		return 1
	}
	L.Push(lua.LString(hex.EncodeToString(state.TLSUnique)))
	return 1
}

// ---------------------------------------------------------------------
// Method 8: ciphersuiteId — uint16 numeric
// ---------------------------------------------------------------------

// sslCiphersuiteID returns the uint16 cipher-suite ID from
// tls.ConnectionState.CipherSuite as a Lua number. Pushed via
// lua.LNumber (gopher-lua's numeric type); the value is the IANA
// cipher-suite ID (matches upstream Envoy's :ciphersuiteId numeric
// shape).
//
// Returns 0 when state is nil. Per D5 (f-B) closure: subject-only test
// surface (Go and OpenSSL use the same IANA IDs but the enumerated set
// differs — Go's preferred suite set is a subset of OpenSSL's).
func sslCiphersuiteID(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	if state == nil {
		L.Push(lua.LNumber(0))
		return 1
	}
	L.Push(lua.LNumber(state.CipherSuite))
	return 1
}

// ---------------------------------------------------------------------
// Method 9: tlsVersion — string per SPEC §11.5.4
// ---------------------------------------------------------------------

// sslTLSVersion returns the TLS version string per SPEC §11.5.4 wire-
// format convention:
//   - tls.VersionTLS10 → "TLSv1.0"
//   - tls.VersionTLS11 → "TLSv1.1"
//   - tls.VersionTLS12 → "TLSv1.2"
//   - tls.VersionTLS13 → "TLSv1.3"
//   - other / unknown → "" (defensive — covers SSL30 + future versions)
//
// Returns "" when state is nil.
func sslTLSVersion(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	if state == nil {
		L.Push(lua.LString(""))
		return 1
	}
	var s string
	switch state.Version {
	case tls.VersionTLS10:
		s = "TLSv1.0"
	case tls.VersionTLS11:
		s = "TLSv1.1"
	case tls.VersionTLS12:
		s = "TLSv1.2"
	case tls.VersionTLS13:
		s = "TLSv1.3"
	default:
		s = ""
	}
	L.Push(lua.LString(s))
	return 1
}

// ---------------------------------------------------------------------
// Method 10: urlEncodedPemEncodedPeerCertificate
// ---------------------------------------------------------------------

// sslURLEncodedPemEncodedPeerCertificate returns the URL-encoded PEM
// encoding of PeerCertificates[0].Raw per SPEC §11.5.4 wire-format
// convention:
//   - PEM block: pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE",
//     Bytes: cert.Raw}).
//   - URL-encoding: url.QueryEscape on the PEM string.
//
// Returns "" when no peer cert is available.
func sslURLEncodedPemEncodedPeerCertificate(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	cert := peerCertFromState(state)
	if cert == nil {
		L.Push(lua.LString(""))
		return 1
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})
	L.Push(lua.LString(url.QueryEscape(string(pemBytes))))
	return 1
}

// ---------------------------------------------------------------------
// Method 11: urlEncodedPemEncodedPeerCertificateChain
// ---------------------------------------------------------------------

// sslURLEncodedPemEncodedPeerCertificateChain returns the URL-encoded
// PEM encoding of the full PeerCertificates chain per SPEC §11.5.4:
//   - For each cert in PeerCertificates: emit a CERTIFICATE PEM block.
//   - Concatenate all blocks (no separator beyond the standard PEM
//     "-----END CERTIFICATE-----\n" trailing newline).
//   - URL-encode the full concatenation via url.QueryEscape.
//
// Returns "" when state is nil or PeerCertificates is empty.
func sslURLEncodedPemEncodedPeerCertificateChain(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	if state == nil || len(state.PeerCertificates) == 0 {
		L.Push(lua.LString(""))
		return 1
	}
	var buf []byte
	for _, c := range state.PeerCertificates {
		if c == nil {
			continue
		}
		buf = append(buf, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})...)
	}
	L.Push(lua.LString(url.QueryEscape(string(buf))))
	return 1
}

// ---------------------------------------------------------------------
// Method 12: sha256PeerCertificateDigest — CROSS-SIDE BYTE-EXACT per D5
// ---------------------------------------------------------------------

// sslSHA256PeerCertificateDigest returns the lowercase hex encoding of
// sha256.Sum256(PeerCertificates[0].Raw) per SPEC §11.5.4 wire-format
// convention verbatim (`fmt.Sprintf("%x", sha256.Sum256(cert.Raw))`).
//
// This is the CROSS-SIDE BYTE-EXACT method per §11.5 D5 closure (option
// f-B cert-fingerprint-only cross-side). Fixture-0027 scenario (f-B)
// compares this method's output between reference Envoy and envoy-go;
// the byte-exact match is the load-bearing acceptance criterion for D5
// closure.
//
// Returns "" when no peer cert is available.
func sslSHA256PeerCertificateDigest(L *lua.LState) int {
	state := sslStateFromUD(L, 1)
	cert := peerCertFromState(state)
	if cert == nil {
		L.Push(lua.LString(""))
		return 1
	}
	sum := sha256.Sum256(cert.Raw)
	// Per SPEC §11.5.4: fmt.Sprintf("%x", sha256.Sum256(cert.Raw)) —
	// lowercase hex of the 32-byte digest (64-char output).
	L.Push(lua.LString(fmt.Sprintf("%x", sum)))
	return 1
}
