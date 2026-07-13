package listener

import (
	"context"
	stdtls "crypto/tls"
	"net"
	"testing"
	"time"

	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	quic "github.com/quic-go/quic-go"

	"github.com/pgdad/envoy-go/internal/stats"
)

// TestQUICGoModuleWired is a compile-time proof the quic-go v0.54.1 module is
// wired and the API leg 61.1 depends on exists. Not behavioral — Task 6
// exercises the real bind + handshake.
func TestQUICGoModuleWired(t *testing.T) {
	_ = &quic.Config{MaxIdleTimeout: 30 * time.Second, HandshakeIdleTimeout: 5 * time.Second}
	var _ func(net.PacketConn, *stdtls.Config, *quic.Config) (*quic.Listener, error) = quic.Listen
}

// pollCounter reads the registry counter named `name`, retrying until its value
// is >= want or timeout elapses, and returns the last observed value. The
// accept path Inc's the cx counter from a goroutine, so the value may lag the
// client's handshake completion — poll, do not read once.
func pollCounter(t *testing.T, reg *stats.Registry, name string, want uint64, timeout time.Duration) uint64 {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last uint64
	for {
		last = 0
		reg.Walk(func(m stats.Metric) {
			if m.Name() != name {
				return
			}
			if c, ok := m.(*stats.Counter); ok {
				last = c.Load()
			}
		})
		if last >= want || time.Now().After(deadline) {
			return last
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestQUICListener_HandshakeALPNh3 is the leg-61.1 subject-side proof: a QUIC
// listener binds UDP, and a local quic-go client completes the QUIC/TLS-1.3
// handshake negotiating ALPN h3. NO HTTP is served (leg 61.2).
func TestQUICListener_HandshakeALPNh3(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListener(t, "c_echo", testAlphaCertPEM, testAlphaKeyPEM, []string{"h3"})
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManager(boot, cm, reg, testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	infos := mgr.Listeners()
	if len(infos) != 1 {
		t.Fatalf("Listeners() = %d, want 1 (QUIC listener must report its bound UDP addr)", len(infos))
	}
	addr := infos[0].Addr

	clientTLS := &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true} //nolint:gosec // local handshake test
	conn, err := quic.DialAddr(ctx, addr, clientTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("quic.DialAddr(%s): %v", addr, err)
	}
	defer func() { _ = conn.CloseWithError(0, "") }()

	tlsState := conn.ConnectionState().TLS
	if tlsState.NegotiatedProtocol != "h3" {
		t.Errorf("negotiated ALPN = %q, want %q", tlsState.NegotiatedProtocol, "h3")
	}
	if tlsState.Version != stdtls.VersionTLS13 {
		t.Errorf("TLS version = %#x, want TLS 1.3 (%#x)", tlsState.Version, stdtls.VersionTLS13)
	}
	// The accept path Inc'd downstream_cx_total for the completed handshake.
	// Accept runs in a goroutine so the Inc may lag the client's handshake
	// completion — poll briefly.
	if got := pollCounter(t, reg, "listener."+normalizeAddr(addr)+".downstream_cx_total", 1, 2*time.Second); got < 1 {
		t.Errorf("downstream_cx_total = %d, want >= 1", got)
	}
}
