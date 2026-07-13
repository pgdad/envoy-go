package listener

import (
	"context"
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

// TestQUICListener_ServesH3GET is the leg-61.2 subject-side proof: a QUIC
// listener whose HCM has codec_type HTTP3 and a direct_response /health route
// (mkQUICListenerHCM) serves an H3 GET to a local quic-go http3.Transport
// client, returning 200 + "OK\n" over HTTP/3. NO differential (that is 61.3).
func TestQUICListener_ServesH3GET(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerHCM(t, testAlphaCertPEM, testAlphaKeyPEM, hcmv3.HttpConnectionManager_HTTP3)
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	mgr, err := NewManager(boot, cm, stats.NewRegistry(), testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer mgr.Stop()

	addr := mgr.Listeners()[0].Addr

	rt := &http3.Transport{
		TLSClientConfig: &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true}, //nolint:gosec // local test
		QUICConfig:      &quic.Config{},
	}
	defer func() { _ = rt.Close() }()
	client := &http.Client{Transport: rt}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/health", nil)
	if err != nil {
		t.Fatalf("NewRequestWithContext: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("H3 GET %s: %v", addr, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if resp.ProtoMajor != 3 {
		t.Errorf("proto major = %d, want 3 (HTTP/3)", resp.ProtoMajor)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if string(body) != "OK\n" {
		t.Errorf("body = %q, want %q", string(body), "OK\n")
	}
}
