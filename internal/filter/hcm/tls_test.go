package hcm

import (
	stdtls "crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"net"
	"net/url"
	"testing"
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
