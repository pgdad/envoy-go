package listener

import (
	"context"
	stdtls "crypto/tls"
	"net"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestNewManager_LiveHandshake_UnmatchedSNI_NoCatchAll_Aborts drives a REAL
// TLS handshake through the manager's accept loop (not just the SelectChain
// unit path exercised by TestNewManager_MultiChain_NoSNIMatch) and asserts
// that a ClientHello whose SNI matches no filter chain — with no catch-all —
// aborts the handshake instead of being served by an arbitrary chain.
//
// This mirrors reference Envoy: with no filter-chain match Envoy closes the
// connection (bumping `no_filter_chain_match`) rather than terminating TLS on a
// wrong chain. envoy-go reaches the same outcome BEFORE the handshake starts:
// since phase 07.2 Task 10 chain selection runs in serveConnection, and a
// SelectChain error closes the connection without ever handing it to
// stdtls.Server, so the client observes the handshake failing. There is no
// SNI-dispatch callback involved. (The phase-95 ALPN-mismatch fallback of
// ADR-0317 is the tree's only per-handshake callback on a downstream chain; it
// never dispatches on SNI and has no error path at all.) The negative
// live-handshake path was previously covered only at the chain-selection level.
func TestNewManager_LiveHandshake_UnmatchedSNI_NoCatchAll_Aborts(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	tsAlpha := mkDownstreamTSInline(t, testAlphaCertPEM, testAlphaKeyPEM)
	tsBeta := mkDownstreamTSInline(t, testBetaCertPEM, testBetaKeyPEM)

	// Two named SNI chains, NO catch-all (empty-match) chain.
	l := mkTLSListener("l_nosni_live", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain([]string{"alpha.envoy-go.test"}, tsAlpha, filter),
		mkTLSChain([]string{"beta.envoy-go.test"}, tsBeta, filter),
	})
	// tls_inspector is required for SNI to reach SelectChain (ADR-0079).
	l.ListenerFilters = []*listenerv3.ListenerFilter{mkTLSInspectorFilter(t)}
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, stats.NewRegistry(), nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	ls := mgr.Listeners()
	if len(ls) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(ls))
	}
	addr := ls[0].Addr
	caPool := testCAPool(t)

	// Sanity: a matched SNI DOES complete the handshake (guards against a
	// vacuous pass where every dial fails for an unrelated reason).
	okConn, err := stdtls.DialWithDialer(
		&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr,
		&stdtls.Config{ServerName: "alpha.envoy-go.test", RootCAs: caPool, MinVersion: stdtls.VersionTLS12},
	)
	if err != nil {
		t.Fatalf("control dial with matched SNI unexpectedly failed: %v", err)
	}
	_ = okConn.Close()

	// Unmatched SNI with no catch-all → the handshake must NOT succeed.
	badConn, err := stdtls.DialWithDialer(
		&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr,
		&stdtls.Config{ServerName: "gamma.envoy-go.test", RootCAs: caPool, MinVersion: stdtls.VersionTLS12},
	)
	if err == nil {
		_ = badConn.Close()
		t.Fatal("TLS handshake with unmatched SNI and no catch-all SUCCEEDED; expected an aborted handshake (reference Envoy closes on no filter chain match)")
	}
}

// mkDownstreamTSInlineALPN mirrors mkDownstreamTSInline but sets
// alpn_protocols on the CommonTlsContext, so the chain's *stdtls.Config
// ADVERTISES those protocols in NextProtos. It does NOT make the server
// enforce overlap: an offer that overlaps none of them falls back to a
// handshake with no protocol selected, matching the measured reference
// (ADR-0317).
func mkDownstreamTSInlineALPN(t *testing.T, certPEM, keyPEM string, alpn []string) *corev3.TransportSocket {
	t.Helper()
	inner := &tlsv3.DownstreamTlsContext{
		CommonTlsContext: &tlsv3.CommonTlsContext{
			AlpnProtocols: alpn,
			TlsCertificates: []*tlsv3.TlsCertificate{{
				CertificateChain: &corev3.DataSource{
					Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte(certPEM)},
				},
				PrivateKey: &corev3.DataSource{
					Specifier: &corev3.DataSource_InlineBytes{InlineBytes: []byte(keyPEM)},
				},
			}},
		},
	}
	a, err := anypb.New(inner)
	if err != nil {
		t.Fatalf("anypb.New DownstreamTlsContext (alpn): %v", err)
	}
	return &corev3.TransportSocket{
		Name:       "envoy.transport_sockets.tls",
		ConfigType: &corev3.TransportSocket_TypedConfig{TypedConfig: a},
	}
}

// TestNewManager_LiveHandshake_ALPNMismatch_CompletesWithNoProtocol pins the
// MEASURED reference behavior for a TCP downstream chain whose
// DownstreamTlsContext advertises alpn_protocols. Booted against the
// digest-pinned reference (envoyproxy/envoy:contrib-v1.37.2), a client that
// offers ONLY a non-overlapping protocol gets a COMPLETED handshake with
// ConnectionState().NegotiatedProtocol == "", the connection is SERVED (the
// reference finished a full echo round trip, not merely a handshake), and the
// listener books ssl.handshake +1 with ssl.connection_error +0. envoy-go
// reaches the same outcome through the ALPN-mismatch fallback installed on the
// per-chain *stdtls.Config by internal/tls (ADR-0317); it does NOT abort and
// it does NOT alert no_application_protocol.
//
// The overlapping-offer control runs FIRST and asserts the negotiated protocol
// by exact equality: it is the over-firing guard (a fallback that fired when an
// overlap DOES exist would read ""), and running it first makes the
// ssl.handshake == 2 arithmetic unambiguous.
func TestNewManager_LiveHandshake_ALPNMismatch_CompletesWithNoProtocol(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	filter := mkTcpProxyFilter(t, "c_echo")
	ts := mkDownstreamTSInlineALPN(t, testAlphaCertPEM, testAlphaKeyPEM, []string{"h2", "http/1.1"})

	l := mkTLSListener("l_alpn_live", "127.0.0.1", 0, []*listenerv3.FilterChain{
		mkTLSChain(nil, ts, filter), // single catch-all TLS chain
	})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManagerWithBaseDirAndAllowH2C(boot, cm, "", false, reg, nil, testHTTPRegistry(), testLFRegistry(), nil, nil, testNetRegistryWithTerminals(t, cm), nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	ls := mgr.Listeners()
	if len(ls) != 1 {
		t.Fatalf("expected 1 listener, got %d", len(ls))
	}
	addr := ls[0].Addr
	caPool := testCAPool(t)

	// Control (FIRST, and the over-firing guard): an overlapping ALPN offer
	// must still complete AND negotiate the overlap.
	okConn, err := stdtls.DialWithDialer(
		&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr,
		&stdtls.Config{ServerName: "alpha.envoy-go.test", RootCAs: caPool, MinVersion: stdtls.VersionTLS12, NextProtos: []string{"http/1.1"}},
	)
	if err != nil {
		t.Fatalf("control dial with overlapping ALPN unexpectedly failed: %v", err)
	}
	if got := okConn.ConnectionState().NegotiatedProtocol; got != "http/1.1" {
		t.Errorf("negotiated ALPN = %q, want %q", got, "http/1.1")
	}
	_ = okConn.Close()

	// Mismatch arm: a NON-overlapping ALPN offer must still COMPLETE, with no
	// protocol selected. This is the reference behavior measured in §0.7.
	mismatchConn, err := stdtls.DialWithDialer(
		&net.Dialer{Timeout: 2 * time.Second}, "tcp", addr,
		&stdtls.Config{ServerName: "alpha.envoy-go.test", RootCAs: caPool, MinVersion: stdtls.VersionTLS12, NextProtos: []string{"bogus/9"}},
	)
	if err != nil {
		t.Fatalf("dial with a non-overlapping ALPN offer failed: %v — the reference COMPLETES this handshake (ADR-0317)", err)
	}
	if got := mismatchConn.ConnectionState().NegotiatedProtocol; got != "" {
		t.Errorf("negotiated ALPN on a non-overlapping offer = %q, want %q", got, "")
	}
	_ = mismatchConn.Close()

	// Both arms drained, the counters must read the reference's arithmetic:
	// two completed handshakes, zero connection errors. Poll the gauge — NO
	// SLEEPS — and read with counterValue, which int64-types the result and
	// t.Errorf's on an ABSENT counter rather than reading a vacuous 0.
	awaitDrained(t, reg, addr, 2)
	prefix := "listener." + normalizeAddr(addr) + ".ssl."
	if got := counterValue(t, reg, prefix+"connection_error"); got != int64(0) {
		t.Errorf("ssl.connection_error = %d, want 0 — an ALPN mismatch is not a connection error", got)
	}
	if got := counterValue(t, reg, prefix+"handshake"); got != int64(2) {
		t.Errorf("ssl.handshake = %d, want 2 — both the overlapping and the non-overlapping offer complete", got)
	}
}
