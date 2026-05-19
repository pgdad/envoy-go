package hcm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"sync"
	"testing"
	"time"
)

// Phase 16 Task 6 (ADR-0144): extractTLSPrincipals helper unit tests. Mirrors
// the Group 7 extraction-helper-in-isolation strategy per BOOTSTRAP_PROMPT —
// the end-to-end mTLS path is covered by fixture 0018 scenario 6 (Tasks 12-14).
// These tests exercise the extract logic against synthetic
// tls.ConnectionState values so the priority ordering URI SAN → DNS SAN →
// Subject DN CN is pinned to the production wire-extraction path.

func TestExtractTLSPrincipals_NilState_ReturnsNil(t *testing.T) {
	// nil ConnectionState (plaintext-conn signal) returns nil. Matches the
	// ADR-0144 §Decision (iii) plaintext / non-mTLS handling.
	if got := extractTLSPrincipals(nil); got != nil {
		t.Errorf("nil state: want nil; got %#v", got)
	}
}

func TestExtractTLSPrincipals_HandshakeIncomplete_ReturnsNil(t *testing.T) {
	// State with HandshakeComplete=false (pre-handshake or aborted) returns
	// nil so callers do NOT accidentally consume an unverified peer cert.
	state := &stdtls.ConnectionState{
		HandshakeComplete: false,
		PeerCertificates: []*x509.Certificate{
			{Subject: pkix.Name{CommonName: "client.example.com"}},
		},
	}
	if got := extractTLSPrincipals(state); got != nil {
		t.Errorf("handshake-incomplete: want nil; got %#v", got)
	}
}

func TestExtractTLSPrincipals_NoPeerCertificates_ReturnsNil(t *testing.T) {
	// HandshakeComplete=true + len(PeerCertificates)==0 → non-mTLS handshake
	// (server-auth-only TLS); client did not present a cert. Returns nil.
	state := &stdtls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates:  nil,
	}
	if got := extractTLSPrincipals(state); got != nil {
		t.Errorf("no peer certs: want nil; got %#v", got)
	}
}

func TestExtractTLSPrincipals_URISANs_FirstPriority(t *testing.T) {
	// Cert with URI SAN only → first slot of returned slice is the URI SAN
	// string (the URL.String() form per ADR-0144 §Decision (iii)).
	u, _ := url.Parse("spiffe://example.com/admin")
	state := &stdtls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates: []*x509.Certificate{
			{URIs: []*url.URL{u}},
		},
	}
	got := extractTLSPrincipals(state)
	if len(got) != 1 || got[0] != "spiffe://example.com/admin" {
		t.Errorf("URI SAN only: want [%q]; got %#v", "spiffe://example.com/admin", got)
	}
}

func TestExtractTLSPrincipals_DNSSANs_SecondPriority(t *testing.T) {
	// Cert with DNS SAN only (no URI SAN, no CN) → slice contains the DNS
	// SAN string in slot 0 (DNS SAN is second priority but in the absence
	// of a URI SAN, it occupies the highest available slot).
	state := &stdtls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates: []*x509.Certificate{
			{DNSNames: []string{"admin.example.com"}},
		},
	}
	got := extractTLSPrincipals(state)
	if len(got) != 1 || got[0] != "admin.example.com" {
		t.Errorf("DNS SAN only: want [%q]; got %#v", "admin.example.com", got)
	}
}

func TestExtractTLSPrincipals_SubjectCN_ThirdPriority(t *testing.T) {
	// Cert with Subject DN CN only (no URI, no DNS SAN) → slice contains
	// the CN string in slot 0. Third priority but present as the only
	// candidate when SAN extensions are absent.
	state := &stdtls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates: []*x509.Certificate{
			{Subject: pkix.Name{CommonName: "client.example.com"}},
		},
	}
	got := extractTLSPrincipals(state)
	if len(got) != 1 || got[0] != "client.example.com" {
		t.Errorf("Subject CN only: want [%q]; got %#v", "client.example.com", got)
	}
}

func TestExtractTLSPrincipals_AllThreeFields_PriorityOrdered(t *testing.T) {
	// Cert carrying all three canonical fields → returned slice is exactly
	// [URI SAN, DNS SAN, Subject CN] in priority order. Pin the byte-exact
	// ordering invariant per ADR-0144 §Decision (iii) + D11 (canonical 3
	// cert fields only; Issuer DN / Serial / fingerprints DEFERRED).
	u, _ := url.Parse("spiffe://example.com/admin")
	state := &stdtls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates: []*x509.Certificate{
			{
				URIs:     []*url.URL{u},
				DNSNames: []string{"admin.example.com"},
				Subject:  pkix.Name{CommonName: "client.example.com"},
			},
		},
	}
	got := extractTLSPrincipals(state)
	want := []string{"spiffe://example.com/admin", "admin.example.com", "client.example.com"}
	if len(got) != len(want) {
		t.Fatalf("got %d entries; want %d (%v)", len(got), len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot[%d]: got %q; want %q", i, got[i], want[i])
		}
	}
}

func TestExtractTLSPrincipals_MultipleURIAndDNSSANs_OrderPreserved(t *testing.T) {
	// Cert with multiple URI SANs + multiple DNS SANs → returned slice
	// preserves the per-cert iteration order WITHIN each priority bucket
	// (URI SANs first in their declared order, then DNS SANs in theirs,
	// then the CN).
	u1, _ := url.Parse("spiffe://example.com/svc-a")
	u2, _ := url.Parse("spiffe://example.com/svc-b")
	state := &stdtls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates: []*x509.Certificate{
			{
				URIs:     []*url.URL{u1, u2},
				DNSNames: []string{"svc-a.example.com", "svc-b.example.com"},
				Subject:  pkix.Name{CommonName: "client.example.com"},
			},
		},
	}
	got := extractTLSPrincipals(state)
	want := []string{
		"spiffe://example.com/svc-a", "spiffe://example.com/svc-b",
		"svc-a.example.com", "svc-b.example.com",
		"client.example.com",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d; want %d (%v)", len(got), len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("slot[%d]: got %q; want %q", i, got[i], want[i])
		}
	}
}

func TestExtractTLSPrincipals_EmptyCN_Skipped(t *testing.T) {
	// Empty Subject CN is NOT appended (would otherwise pollute the slice
	// with "" candidates that match a Prefix("") matcher unexpectedly).
	u, _ := url.Parse("spiffe://example.com/admin")
	state := &stdtls.ConnectionState{
		HandshakeComplete: true,
		PeerCertificates: []*x509.Certificate{
			{
				URIs:    []*url.URL{u},
				Subject: pkix.Name{CommonName: ""}, // empty
			},
		},
	}
	got := extractTLSPrincipals(state)
	want := []string{"spiffe://example.com/admin"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("empty CN: want %v; got %#v", want, got)
	}
}

func TestDownstreamTLSPrincipals_NonTLSConn_ReturnsNil(t *testing.T) {
	// downstreamTLSPrincipals on a non-*tls.Conn (e.g. plain *net.TCPConn or
	// any net.Conn impl other than *tls.Conn) returns nil. Mirrors the H1
	// dispatch path where a plaintext listener supplies the raw conn.
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	defer func() { _ = c2.Close() }()
	if got := downstreamTLSPrincipals(c1); got != nil {
		t.Errorf("non-tls conn: want nil; got %#v", got)
	}
}

func TestDownstreamTLSPrincipals_NilConn_ReturnsNil(t *testing.T) {
	// Defensive: nil downstream (test-path or pre-handshake) returns nil.
	// The H1 dispatch test path uses nil conn for non-TLS paths
	// (chain_dispatch_test + chain_integration_test + connection_test).
	if got := downstreamTLSPrincipals(nil); got != nil {
		t.Errorf("nil conn: want nil; got %#v", got)
	}
}

// ---------------------------------------------------------------------------
// Phase 22.2 Task 6 (ADR-0192) — downstreamTLSConnectionState helper-isolation
// tests. Mirror the existing downstreamTLSPrincipals helper-in-isolation
// strategy in this file (a Group-7 extraction-helper-in-isolation suite per
// BOOTSTRAP_PROMPT). These tests exercise the new helper against the same set
// of conn-state shapes the H1 + H2 dispatch paths produce in production:
//   - nil conn               → nil
//   - non-*tls.Conn          → nil
//   - *tls.Conn pre-handshake → nil (HandshakeComplete=false guard)
//   - *tls.Conn post-handshake (server-auth-only) → non-nil pointer carrying the
//     handshake state (SNI, peer cert chain on the client side, etc.)
//
// Per SPEC §11.5.3 the helper does NOT gate on PeerCertificates length —
// server-auth-only (non-mTLS) handshakes still produce a usable
// *tls.ConnectionState (SNI / cipher suite / version are valid even without a
// client cert). That contrasts with extractTLSPrincipals which gates on
// len(PeerCertificates)==0 per ADR-0144.
// ---------------------------------------------------------------------------

func TestDownstreamTLSConnectionState_NilConn_ReturnsNil(t *testing.T) {
	if got := downstreamTLSConnectionState(nil); got != nil {
		t.Errorf("nil conn: want nil; got %#v", got)
	}
}

func TestDownstreamTLSConnectionState_NonTLSConn_ReturnsNil(t *testing.T) {
	// Non-*tls.Conn (plain net.Pipe pair) → nil. Plaintext listener / test
	// conn-pair path produces this shape.
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	defer func() { _ = c2.Close() }()
	if got := downstreamTLSConnectionState(c1); got != nil {
		t.Errorf("non-tls conn: want nil; got %#v", got)
	}
}

func TestDownstreamTLSConnectionState_TLSConn_HandshakeIncomplete_ReturnsNil(t *testing.T) {
	// *tls.Conn that has not yet completed its handshake → nil. Mirrors the
	// extractTLSPrincipals HandshakeComplete guard so the bridge surface does
	// NOT observe an unverified state.
	c1, c2 := net.Pipe()
	defer func() { _ = c1.Close() }()
	defer func() { _ = c2.Close() }()
	tlsConn := stdtls.Server(c1, &stdtls.Config{})
	// No Handshake() invocation; tlsConn.ConnectionState().HandshakeComplete
	// is false.
	if got := downstreamTLSConnectionState(tlsConn); got != nil {
		t.Errorf("pre-handshake *tls.Conn: want nil; got HandshakeComplete=%v", got.HandshakeComplete)
	}
}

// runInProcessTLSHandshake spins up a *tls.Server / *tls.Client over a
// net.Pipe pair with a self-signed RSA / ECDSA leaf cert and drives the
// handshake to completion. Returns the server-side *tls.Conn (post-handshake)
// + a cleanup function. The server certificate's Subject.CommonName is
// `serverCN`, the SNI presented by the client is `clientSNI`. Used by the
// dispatchRequest end-to-end seeding tests.
//
// The helper is intentionally self-contained — no shared PKI fixtures — so
// each test runs in <10ms with no external file dependencies.
func runInProcessTLSHandshake(t *testing.T, serverCN string, clientSNI string) (*stdtls.Conn, func()) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ecdsa: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: serverCN},
		DNSNames:     []string{clientSNI},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("self-signed cert: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	cert := stdtls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        leaf,
	}

	serverConn, clientConn := net.Pipe()
	serverTLS := stdtls.Server(serverConn, &stdtls.Config{
		Certificates: []stdtls.Certificate{cert},
		MinVersion:   stdtls.VersionTLS12,
	})
	clientTLS := stdtls.Client(clientConn, &stdtls.Config{
		ServerName:         clientSNI,
		InsecureSkipVerify: true, // self-signed test cert
		MinVersion:         stdtls.VersionTLS12,
	})

	var wg sync.WaitGroup
	var clientErr, serverErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		clientErr = clientTLS.Handshake()
	}()
	go func() {
		defer wg.Done()
		serverErr = serverTLS.Handshake()
	}()
	wg.Wait()
	if clientErr != nil {
		t.Fatalf("client handshake: %v", clientErr)
	}
	if serverErr != nil {
		t.Fatalf("server handshake: %v", serverErr)
	}
	// Cleanup discipline: close the raw net.Pipe conns directly (NOT the
	// stdtls.Conn wrappers). The tls.Conn.Close() path writes a close_notify
	// alert and blocks waiting for the peer's close_notify; on a synchronous
	// net.Pipe pair both sides block simultaneously and the test stalls 5s
	// per dispatch waiting on the read deadline. Closing the raw pipe forces
	// EOF on both readers immediately so the tls.Conn writers/readers
	// terminate without waiting for the alert exchange.
	cleanup := func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
	}
	return serverTLS, cleanup
}

func TestDownstreamTLSConnectionState_TLSConn_HandshakeComplete_ReturnsState(t *testing.T) {
	// *tls.Conn post-handshake → non-nil pointer; ServerName carries the SNI
	// presented by the client; HandshakeComplete is true. Per SPEC §11.5.3 the
	// state is non-nil even for server-auth-only TLS (no client cert).
	serverTLS, cleanup := runInProcessTLSHandshake(t, "server-cn-test", "sni.envoy-go.test")
	defer cleanup()

	got := downstreamTLSConnectionState(serverTLS)
	if got == nil {
		t.Fatal("post-handshake *tls.Conn: want non-nil *tls.ConnectionState; got nil")
	}
	if !got.HandshakeComplete {
		t.Errorf("HandshakeComplete = false; want true")
	}
	if got.ServerName != "sni.envoy-go.test" {
		t.Errorf("ServerName = %q; want %q", got.ServerName, "sni.envoy-go.test")
	}
}
