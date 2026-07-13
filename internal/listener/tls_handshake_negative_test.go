package listener

import (
	"context"
	stdtls "crypto/tls"
	"net"
	"testing"
	"time"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"

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
// wrong chain. envoy-go reaches the same outcome via SelectChain returning an
// error from the GetConfigForClient callback, which the Go TLS server turns
// into a handshake abort. The negative live-handshake path was previously
// covered only at the chain-selection level.
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
