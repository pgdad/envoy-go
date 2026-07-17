package listener

import (
	"context"
	"crypto/rand"
	stdtls "crypto/tls"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	quic "github.com/quic-go/quic-go"
	http3 "github.com/quic-go/quic-go/http3"

	"github.com/pgdad/envoy-go/internal/stats"
)

// Negative-path coverage for the phase-61 QUIC/H3 listener. The committed
// phase-61 tests drive only the positive path (handshake + GET); ALPN mismatch
// existed only as a TEMPORARY deliberate-break during 61.1 (PROGRESS-61.1.md:91)
// and no test fed the UDP socket non-QUIC bytes at all. These pin two
// robustness properties a public-facing UDP listener must hold:
//
//  1. a client that cannot negotiate ALPN h3 is REFUSED at the handshake — and
//     the refusal is contained: the listener keeps serving subsequent
//     well-formed clients (the accept loop must not exit on a failed
//     handshake);
//  2. garbage datagrams (non-QUIC bytes, including a QUIC-long-header-shaped
//     prefix over junk) neither crash nor wedge the listener.
//
// Both use the existing mkQUICListenerHCM direct-response listener, ephemeral
// ports, and bounded ctx deadlines — no sleeps, no external network.

// startQUICHCMListener stands up the standard QUIC HCM direct-response
// listener and returns its bound UDP address.
func startQUICHCMListener(t *testing.T, ctx context.Context) string {
	t.Helper()
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerHCM(t, testAlphaCertPEM, testAlphaKeyPEM, hcmv3.HttpConnectionManager_HTTP3)
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(mgr.Stop)
	infos := mgr.Listeners()
	if len(infos) != 1 {
		t.Fatalf("Listeners() = %d, want 1", len(infos))
	}
	return infos[0].Addr
}

// h3GetHealth performs an H3 GET /health against addr and asserts 200 "OK\n".
// label distinguishes the before/after probes in failure output.
func h3GetHealth(t *testing.T, ctx context.Context, addr, label string) {
	t.Helper()
	rt := &http3.Transport{
		TLSClientConfig: &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true}, //nolint:gosec // local test
		QUICConfig:      &quic.Config{},
	}
	defer func() { _ = rt.Close() }()
	client := &http.Client{Transport: rt}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/health", nil)
	if err != nil {
		t.Fatalf("[%s] NewRequestWithContext: %v", label, err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("[%s] H3 GET %s: %v", label, addr, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Errorf("[%s] status = %d, want 200", label, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("[%s] read body: %v", label, err)
	}
	if string(body) != "OK\n" {
		t.Errorf("[%s] body = %q, want %q", label, string(body), "OK\n")
	}
}

// TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives: a client offering
// only ALPN h2 must fail the QUIC/TLS handshake (reference Envoy's QUIC
// listener is h3-only; crypto/tls surfaces no_application_protocol), and the
// listener must keep serving well-formed h3 clients afterwards.
func TestQUICListener_ALPNMismatch_RefusedAndListenerSurvives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	addr := startQUICHCMListener(t, ctx)

	// Control: the listener serves h3 before the mismatch attempt.
	h3GetHealth(t, ctx, addr, "control before mismatch")

	badTLS := &stdtls.Config{NextProtos: []string{"h2"}, InsecureSkipVerify: true} //nolint:gosec // local test
	dialCtx, dialCancel := context.WithTimeout(ctx, 5*time.Second)
	defer dialCancel()
	conn, err := quic.DialAddr(dialCtx, addr, badTLS, &quic.Config{})
	if err == nil {
		_ = conn.CloseWithError(0, "")
		t.Fatal("QUIC dial offering only ALPN h2 SUCCEEDED; the h3-only listener must refuse the handshake")
	}

	// The refusal must be contained: the accept loop still serves h3.
	h3GetHealth(t, ctx, addr, "after ALPN mismatch")
}

// TestQUICListener_GarbageDatagrams_ListenerSurvives: raw non-QUIC datagrams —
// short junk, max-size random noise, and a QUIC-long-header-shaped first byte
// over junk — must neither crash the process nor stop the listener from
// serving a subsequent well-formed H3 request.
func TestQUICListener_GarbageDatagrams_ListenerSurvives(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	addr := startQUICHCMListener(t, ctx)

	h3GetHealth(t, ctx, addr, "control before garbage")

	udp, err := net.Dial("udp", addr)
	if err != nil {
		t.Fatalf("net.Dial(udp %s): %v", addr, err)
	}
	defer func() { _ = udp.Close() }()

	junkLarge := make([]byte, 1200)
	if _, err := rand.Read(junkLarge); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	longHeaderJunk := append([]byte{0xC0, 0x00, 0x00, 0x00, 0x01}, junkLarge[:200]...)
	for _, datagram := range [][]byte{
		[]byte("not-quic"),
		junkLarge,
		longHeaderJunk,
		{0x00}, // single short-header-ish byte
	} {
		if _, err := udp.Write(datagram); err != nil {
			t.Fatalf("udp.Write(%d bytes): %v", len(datagram), err)
		}
	}

	// The listener must still serve a well-formed client.
	h3GetHealth(t, ctx, addr, "after garbage datagrams")
}
