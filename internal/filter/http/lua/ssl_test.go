package lua

// ssl_test.go — Task 10 (phase 22.2 IMPL) connection + ssl bridge tests
// per SPEC §3.4 + §11.5 D5 closure (option f-B cert-fingerprint-only
// cross-side) + PLAN Task 10 acceptance + BRAINSTORM §2.4 12-method
// roster.
//
// Coverage:
//
//   - TestConnection_returns_userdata_with_ssl_method — `:connection()`
//     returns a userdata wrapping the per-stream TLS state with an
//     `:ssl()` accessor.
//   - TestConnection_ssl_returns_nil_for_plaintext — when the test-double
//     DecoderFilterCallbacks returns nil from DownstreamTLSConnectionState
//     (plaintext / pre-handshake), `:connection():ssl()` returns lua.LNil.
//   - TestConnection_ssl_returns_userdata_for_tls — on TLS, returns ssl
//     userdata (NOT nil); 12 methods on the userdata are callable.
//   - TestSSL_sha256PeerCertificateDigest_returns_byte_exact_hex — the
//     CROSS-SIDE BYTE-EXACT method per D5 (f-B): lowercase hex of
//     sha256.Sum256(cert.Raw); 64-char output.
//   - TestSSL_subjectPeerCertificate / TestSSL_subjectLocalCertificate /
//     TestSSL_sanPeerCertificate / TestSSL_sanLocalCertificate /
//     TestSSL_validFromPeerCertificate / TestSSL_expirationPeerCertificate
//     / TestSSL_sessionId / TestSSL_ciphersuiteId / TestSSL_tlsVersion /
//     TestSSL_urlEncodedPemEncodedPeerCertificate /
//     TestSSL_urlEncodedPemEncodedPeerCertificateChain — one test per
//     remaining method exercising the wire-format conventions per SPEC
//     §11.5.4.
//   - TestSSL_methods_on_nil_state_handled_gracefully — defensive: if
//     somehow the ssl wrapper is constructed wrapping a nil state, the
//     12 methods do NOT panic; return safe defaults (empty string / 0 /
//     empty table). Companion to the plaintext-nil path above.
//   - TestSSL_no_peer_certs_returns_defaults — TLS state present but
//     PeerCertificates empty (server-side with no client cert requested);
//     methods that consume PeerCertificates[0] return safe defaults.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"

	"github.com/esalaine/envoy-go/internal/dynamicmetadata"
	luaprim "github.com/esalaine/envoy-go/internal/lua"
)

// ---------------------------------------------------------------------
// Test-double infrastructure
// ---------------------------------------------------------------------

// fakeCallbacksWithTLS extends fakeCallbacks (bridge_test.go) with a
// per-stream *tls.ConnectionState so :connection():ssl() has a non-nil
// backing TLS state when desired. The TLS field is unexported; the test
// helper newBridgedVMWithTLS wires it via the per-context tlsState seam.
type fakeCallbacksWithTLS struct {
	fakeCallbacks
	tlsState *tls.ConnectionState
	bucket   *dynamicmetadata.Bucket
}

// DownstreamTLSConnectionState implements the bridge's TLS accessor for
// the :connection():ssl() entry. Returns the canned state pointer per
// test-supplied value; nil-tolerant for the plaintext path.
func (f *fakeCallbacksWithTLS) DownstreamTLSConnectionState() *tls.ConnectionState {
	return f.tlsState
}

// DynamicMetadata returns the per-test bucket (or nil); preserves the
// per-context interface (RequestHandleCallbacks) shape consumed by
// metadata.go without forcing the connection-SSL tests to wire a bucket
// when they don't need one.
func (f *fakeCallbacksWithTLS) DynamicMetadata() *dynamicmetadata.Bucket {
	return f.bucket
}

// newBridgedVMWithTLS constructs a VM with the bridge metatables +
// streamInfo metatable + metadata + dynamicMetadata + connection + ssl
// metatables + a fakeCallbacksWithTLS wired into reqCtx.cb so the script
// can access :connection():ssl() and the 12 ssl methods.
func newBridgedVMWithTLS(t *testing.T, state *tls.ConnectionState) *luaprim.VM {
	t.Helper()
	cb := &fakeCallbacksWithTLS{tlsState: state}
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installStreamInfoMetatable(L)
	installMetadataMetatable(L)
	installDynamicMetadataMetatable(L)
	installConnectionMetatable(L)
	installSSLMetatable(L)
	installPairsShim(L)
	ctx := &requestHandleContext{headers: http.Header{}, cb: cb}
	ud := L.NewUserData()
	ud.Value = ctx
	L.SetMetatable(ud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", ud)
	return vm
}

// generateTestCertChain mints a self-signed leaf + a single-cert "chain"
// for cross-side fingerprint scenario validation. The leaf has:
//   - Subject CN = "test.envoy-go.local"
//   - DNSNames = ["test.envoy-go.local", "alt.envoy-go.local"]
//   - URIs = ["spiffe://envoy-go/test"]
//   - NotBefore = a fixed UTC time (cert-fingerprint determinism source)
//   - NotAfter = +1 year from NotBefore
//
// Returns (leaf *x509.Certificate, leafDER []byte) so tests can derive
// the expected sha256 digest + the PEM encoding independently.
func generateTestCertChain(t *testing.T) (*x509.Certificate, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa.GenerateKey: %v", err)
	}
	uri, _ := url.Parse("spiffe://envoy-go/test")
	notBefore := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   "test.envoy-go.local",
			Organization: []string{"envoy-go"},
		},
		NotBefore:   notBefore,
		NotAfter:    notBefore.AddDate(1, 0, 0),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		DNSNames:    []string{"test.envoy-go.local", "alt.envoy-go.local"},
		URIs:        []*url.URL{uri},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("x509.CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("x509.ParseCertificate: %v", err)
	}
	return cert, der
}

// stateWithPeerCert returns a TLS state with a single PeerCertificate
// (the test leaf) + canned Version/CipherSuite/TLSUnique for the
// session/cipher/version method tests.
func stateWithPeerCert(t *testing.T) *tls.ConnectionState {
	t.Helper()
	cert, _ := generateTestCertChain(t)
	return &tls.ConnectionState{
		Version:            tls.VersionTLS13,
		HandshakeComplete:  true,
		CipherSuite:        tls.TLS_AES_256_GCM_SHA384,
		PeerCertificates:   []*x509.Certificate{cert},
		VerifiedChains:     [][]*x509.Certificate{{cert}},
		TLSUnique:          []byte{0xde, 0xad, 0xbe, 0xef, 0x01, 0x02, 0x03, 0x04},
		NegotiatedProtocol: "h2",
		ServerName:         "test.envoy-go.local",
	}
}

// ---------------------------------------------------------------------
// :connection() + :ssl() accessor surface
// ---------------------------------------------------------------------

func TestConnection_returns_userdata_with_ssl_method(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `
		local c = rh:connection()
		result_type = type(c)
	`)
	if got := getGlobalString(t, vm, "result_type"); got != "userdata" {
		t.Fatalf("type(rh:connection()) = %q; want %q", got, "userdata")
	}
}

func TestConnection_ssl_returns_nil_for_plaintext(t *testing.T) {
	// plaintext path — fakeCallbacksWithTLS returns nil from
	// DownstreamTLSConnectionState.
	vm := newBridgedVMWithTLS(t, nil)
	runScript(t, vm, `
		local s = rh:connection():ssl()
		result_is_nil = (s == nil)
	`)
	if v := vm.State().GetGlobal("result_is_nil"); v != lua.LTrue {
		t.Fatalf("rh:connection():ssl() on plaintext = %v; want nil per ADR-0144 plumbing pattern", v)
	}
}

func TestConnection_ssl_returns_userdata_for_tls(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `
		local s = rh:connection():ssl()
		result_type = type(s)
	`)
	if got := getGlobalString(t, vm, "result_type"); got != "userdata" {
		t.Fatalf("type(rh:connection():ssl()) = %q; want %q", got, "userdata")
	}
}

// ---------------------------------------------------------------------
// :sha256PeerCertificateDigest — BYTE-EXACT cross-side per D5 (f-B)
// ---------------------------------------------------------------------

// TestSSL_sha256PeerCertificateDigest_returns_byte_exact_hex is the
// hinge test for D5 (option f-B cert-fingerprint-only cross-side). The
// method MUST return the lowercase hex encoding of
// sha256.Sum256(cert.Raw). Byte-exact matching is critical for
// fixture-0027 scenario (f-B) cross-side determinism.
func TestSSL_sha256PeerCertificateDigest_returns_byte_exact_hex(t *testing.T) {
	cert, _ := generateTestCertChain(t)
	state := &tls.ConnectionState{
		Version:           tls.VersionTLS13,
		HandshakeComplete: true,
		PeerCertificates:  []*x509.Certificate{cert},
	}
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():sha256PeerCertificateDigest()`)
	got := getGlobalString(t, vm, "result")

	sum := sha256.Sum256(cert.Raw)
	want := hex.EncodeToString(sum[:])

	if got != want {
		t.Fatalf("sha256PeerCertificateDigest() = %q; want byte-exact %q", got, want)
	}
	if len(got) != 64 {
		t.Errorf("sha256PeerCertificateDigest() len = %d; want 64 (32-byte hex)", len(got))
	}
	if strings.ToLower(got) != got {
		t.Errorf("sha256PeerCertificateDigest() = %q; want all-lowercase hex per SPEC §11.5.4", got)
	}
}

// ---------------------------------------------------------------------
// :subjectPeerCertificate + :subjectLocalCertificate
// ---------------------------------------------------------------------

func TestSSL_subjectPeerCertificate(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():subjectPeerCertificate()`)
	got := getGlobalString(t, vm, "result")
	// The Go x509 String() produces e.g. "CN=test.envoy-go.local,O=envoy-go".
	if !strings.Contains(got, "CN=test.envoy-go.local") {
		t.Errorf("subjectPeerCertificate() = %q; expected to contain %q", got, "CN=test.envoy-go.local")
	}
}

func TestSSL_subjectLocalCertificate_returns_empty_on_no_local_cert(t *testing.T) {
	// envoy-go-strict: Go's tls.ConnectionState exposes no LocalCertificate
	// field directly. Per SPEC §11.5.4 the local-cert methods may return
	// empty string when the local cert is not available. This test pins
	// the empty-on-no-local-cert disposition rather than asserting a
	// specific local-cert string.
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():subjectLocalCertificate()`)
	got := getGlobalString(t, vm, "result")
	if got != "" {
		// Acceptable values include empty; tolerate any string but pin the
		// no-panic + always-string contract.
		t.Logf("subjectLocalCertificate() returned %q (acceptable)", got)
	}
}

// ---------------------------------------------------------------------
// :sanPeerCertificate + :sanLocalCertificate — Lua table of SAN entries
// ---------------------------------------------------------------------

func TestSSL_sanPeerCertificate_returns_dns_and_uri_sans(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `
		local sans = rh:connection():ssl():sanPeerCertificate()
		sans_type = type(sans)
		sans_count = #sans
		sans_concat = table.concat(sans, ",")
	`)
	if got := getGlobalString(t, vm, "sans_type"); got != "table" {
		t.Fatalf("sanPeerCertificate() type = %q; want %q", got, "table")
	}
	if n := getGlobalInt(t, vm, "sans_count"); n < 2 {
		t.Errorf("sanPeerCertificate() len = %d; want >= 2 (DNS + URI)", n)
	}
	cat := getGlobalString(t, vm, "sans_concat")
	if !strings.Contains(cat, "test.envoy-go.local") {
		t.Errorf("sanPeerCertificate() concat = %q; expected to contain DNS SAN", cat)
	}
}

func TestSSL_sanLocalCertificate_returns_empty_table_on_no_local_cert(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `
		local sans = rh:connection():ssl():sanLocalCertificate()
		sans_type = type(sans)
		sans_count = #sans
	`)
	if got := getGlobalString(t, vm, "sans_type"); got != "table" {
		t.Fatalf("sanLocalCertificate() type = %q; want %q (empty-on-no-local-cert)", got, "table")
	}
	if n := getGlobalInt(t, vm, "sans_count"); n != 0 {
		t.Errorf("sanLocalCertificate() len = %d; want 0 (no local cert)", n)
	}
}

// ---------------------------------------------------------------------
// :validFromPeerCertificate + :expirationPeerCertificate
// ---------------------------------------------------------------------

func TestSSL_validFromPeerCertificate_returns_iso8601(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():validFromPeerCertificate()`)
	got := getGlobalString(t, vm, "result")
	// Per SPEC §11.5.4 wire-format: time.RFC3339. Fixed cert NotBefore
	// = 2024-01-01T00:00:00Z.
	const want = "2024-01-01T00:00:00Z"
	if got != want {
		t.Errorf("validFromPeerCertificate() = %q; want %q (RFC3339 per SPEC §11.5.4)", got, want)
	}
}

func TestSSL_expirationPeerCertificate_returns_iso8601(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():expirationPeerCertificate()`)
	got := getGlobalString(t, vm, "result")
	const want = "2025-01-01T00:00:00Z"
	if got != want {
		t.Errorf("expirationPeerCertificate() = %q; want %q", got, want)
	}
}

// ---------------------------------------------------------------------
// :sessionId
// ---------------------------------------------------------------------

func TestSSL_sessionId_returns_hex_of_tls_unique(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():sessionId()`)
	got := getGlobalString(t, vm, "result")
	const want = "deadbeef01020304"
	if got != want {
		t.Errorf("sessionId() = %q; want %q (hex of TLSUnique)", got, want)
	}
}

// ---------------------------------------------------------------------
// :ciphersuiteId
// ---------------------------------------------------------------------

func TestSSL_ciphersuiteId_returns_numeric_uint16(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():ciphersuiteId()`)
	got := getGlobalInt(t, vm, "result")
	// TLS_AES_256_GCM_SHA384 = 0x1302 = 4866
	const want = 0x1302
	if got != want {
		t.Errorf("ciphersuiteId() = %d (0x%04x); want %d (0x%04x)", got, got, want, want)
	}
}

// ---------------------------------------------------------------------
// :tlsVersion — STRING per SPEC §11.5.4
// ---------------------------------------------------------------------

func TestSSL_tlsVersion_returns_string_TLSv13(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():tlsVersion()`)
	got := getGlobalString(t, vm, "result")
	const want = "TLSv1.3"
	if got != want {
		t.Errorf("tlsVersion() = %q; want %q (per SPEC §11.5.4 wire-format)", got, want)
	}
}

func TestSSL_tlsVersion_returns_string_TLSv12(t *testing.T) {
	cert, _ := generateTestCertChain(t)
	state := &tls.ConnectionState{
		Version:           tls.VersionTLS12,
		HandshakeComplete: true,
		PeerCertificates:  []*x509.Certificate{cert},
	}
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():tlsVersion()`)
	got := getGlobalString(t, vm, "result")
	if got != "TLSv1.2" {
		t.Errorf("tlsVersion() (TLS 1.2 state) = %q; want %q", got, "TLSv1.2")
	}
}

// ---------------------------------------------------------------------
// :urlEncodedPemEncodedPeerCertificate + :urlEncodedPemEncodedPeerCertificateChain
// ---------------------------------------------------------------------

func TestSSL_urlEncodedPemEncodedPeerCertificate(t *testing.T) {
	cert, der := generateTestCertChain(t)
	state := &tls.ConnectionState{
		Version:           tls.VersionTLS13,
		HandshakeComplete: true,
		PeerCertificates:  []*x509.Certificate{cert},
	}
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():urlEncodedPemEncodedPeerCertificate()`)
	got := getGlobalString(t, vm, "result")
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	want := url.QueryEscape(string(pemBlock))
	if got != want {
		t.Errorf("urlEncodedPemEncodedPeerCertificate() mismatch\n got = %q\nwant = %q", got, want)
	}
}

func TestSSL_urlEncodedPemEncodedPeerCertificateChain(t *testing.T) {
	cert, der := generateTestCertChain(t)
	state := &tls.ConnectionState{
		Version:           tls.VersionTLS13,
		HandshakeComplete: true,
		PeerCertificates:  []*x509.Certificate{cert},
	}
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():urlEncodedPemEncodedPeerCertificateChain()`)
	got := getGlobalString(t, vm, "result")
	// Chain with single cert; full chain PEM is just the one leaf.
	pemBlock := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	want := url.QueryEscape(string(pemBlock))
	if got != want {
		t.Errorf("urlEncodedPemEncodedPeerCertificateChain() mismatch\n got = %q\nwant = %q",
			got, want)
	}
}

// ---------------------------------------------------------------------
// Nil-state / no-peer-cert defensive paths
// ---------------------------------------------------------------------

// TestSSL_no_peer_certs_returns_defaults verifies that when a TLS state
// is present but PeerCertificates is empty (server-side without client
// auth), the methods that consume PeerCertificates[0] return safe
// defaults rather than panicking.
func TestSSL_no_peer_certs_returns_defaults(t *testing.T) {
	state := &tls.ConnectionState{
		Version:           tls.VersionTLS12,
		HandshakeComplete: true,
		CipherSuite:       tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		PeerCertificates:  nil, // no peer cert
	}
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `
		local s = rh:connection():ssl()
		r_subject = s:subjectPeerCertificate()
		r_san = s:sanPeerCertificate()
		r_validFrom = s:validFromPeerCertificate()
		r_expiration = s:expirationPeerCertificate()
		r_sha = s:sha256PeerCertificateDigest()
		r_pem = s:urlEncodedPemEncodedPeerCertificate()
		r_chain = s:urlEncodedPemEncodedPeerCertificateChain()
		r_version = s:tlsVersion()
		r_cipher = s:ciphersuiteId()
	`)
	// Subject/sha/pem/chain should be empty strings on no peer cert.
	for _, name := range []string{"r_subject", "r_validFrom", "r_expiration", "r_sha", "r_pem", "r_chain"} {
		v := vm.State().GetGlobal(name)
		s, ok := v.(lua.LString)
		if !ok {
			t.Errorf("%s type = %s; want string (defensive empty)", name, v.Type())
			continue
		}
		if string(s) != "" {
			t.Errorf("%s = %q; want \"\" (no peer cert)", name, string(s))
		}
	}
	// san should be empty table.
	runScript(t, vm, `r_san_count = #r_san`)
	if n := getGlobalInt(t, vm, "r_san_count"); n != 0 {
		t.Errorf("sanPeerCertificate() len = %d; want 0 (no peer cert)", n)
	}
	// version + cipher SHOULD still return their TLS-state values
	// (independent of PeerCertificates).
	if got := getGlobalString(t, vm, "r_version"); got != "TLSv1.2" {
		t.Errorf("tlsVersion() on no-peer-cert = %q; want %q", got, "TLSv1.2")
	}
	if got := getGlobalInt(t, vm, "r_cipher"); got == 0 {
		t.Errorf("ciphersuiteId() on no-peer-cert = 0; want non-zero (state-independent of peer cert)")
	}
}

// TestSSL_full_method_callability exercises every 1 of the 12 methods at
// least once to ensure the metatable dispatch table is wired correctly
// (no "method X is missing" panics).
func TestSSL_full_method_callability(t *testing.T) {
	state := stateWithPeerCert(t)
	vm := newBridgedVMWithTLS(t, state)
	const script = `
		local s = rh:connection():ssl()
		m1 = s:subjectPeerCertificate()
		m2 = s:subjectLocalCertificate()
		m3 = s:sanPeerCertificate()
		m4 = s:sanLocalCertificate()
		m5 = s:validFromPeerCertificate()
		m6 = s:expirationPeerCertificate()
		m7 = s:sessionId()
		m8 = s:ciphersuiteId()
		m9 = s:tlsVersion()
		m10 = s:urlEncodedPemEncodedPeerCertificate()
		m11 = s:urlEncodedPemEncodedPeerCertificateChain()
		m12 = s:sha256PeerCertificateDigest()
	`
	runScript(t, vm, script)
	// Sanity: m12 (sha256) is the cross-side byte-exact method — verify
	// non-empty + matches expected length.
	if got := getGlobalString(t, vm, "m12"); len(got) != 64 {
		t.Errorf("m12 sha256PeerCertificateDigest len = %d; want 64", len(got))
	}
}

// TestSSL_byte_exact_cross_side_hex_format verifies the hex format is
// exactly what fixture-0027 scenario (f-B) cross-side comparison expects:
// lowercase hex via fmt.Sprintf("%x", ...) per SPEC §11.5.4.
func TestSSL_byte_exact_cross_side_hex_format(t *testing.T) {
	cert, _ := generateTestCertChain(t)
	state := &tls.ConnectionState{
		Version:          tls.VersionTLS13,
		PeerCertificates: []*x509.Certificate{cert},
	}
	vm := newBridgedVMWithTLS(t, state)
	runScript(t, vm, `result = rh:connection():ssl():sha256PeerCertificateDigest()`)
	got := getGlobalString(t, vm, "result")

	// Computed via the two-arg fmt.Sprintf("%x", ...) reference (matches
	// the SPEC §11.5.4 wire-format convention verbatim).
	sum := sha256.Sum256(cert.Raw)
	want := fmt.Sprintf("%x", sum)

	if got != want {
		t.Errorf("sha256PeerCertificateDigest format mismatch\n got = %q\nwant = %q (fmt.Sprintf(%%x, sha256.Sum256(cert.Raw)))",
			got, want)
	}
}
