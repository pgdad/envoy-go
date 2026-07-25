package listener

import (
	"context"
	stdtls "crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"reflect"
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

// counterValue reads the registry counter named `name` ONCE and returns its
// value. Unlike pollCounter it asserts an EXACT value at the call site rather
// than polling for a minimum, which is what an "is still zero" assertion needs
// — pollCounter would return 0 both for "registered and zero" and for "not
// registered at all".
//
// ⚠️ It t.Errorf's when `name` is ABSENT from the registry and MUST NOT be
// changed to return a silent 0. The zero-assertion in
// TestQUICListener_RegistersSSLNamesAtZero would otherwise pass VACUOUSLY under
// a break that stops registering the ssl.* counters entirely: nothing
// registered ⇒ every read is 0 ⇒ "all three are zero" is trivially true.
func counterValue(t *testing.T, reg *stats.Registry, name string) int64 {
	t.Helper()
	var (
		val   uint64
		found bool
	)
	reg.Walk(func(m stats.Metric) {
		if m.Name() != name {
			return
		}
		if c, ok := m.(*stats.Counter); ok {
			val = c.Load()
			found = true
		}
	})
	if !found {
		t.Errorf("counter %q is not registered", name)
		return -1
	}
	return int64(val)
}

// driveH3 performs one real HTTP/3 GET against a live QUIC listener's bound UDP
// address, using the same http3.Transport shape as
// TestQUICListener_ServesH3GET, and returns a non-nil error if any leg of the
// round trip fails. Callers treat a failure as a PRECONDITION break, not a
// property: without a completed H3 handshake, an "the ssl.* counters are still
// zero" assertion is vacuous.
func driveH3(t *testing.T, addr string) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	rt := &http3.Transport{
		TLSClientConfig: &stdtls.Config{NextProtos: []string{"h3"}, InsecureSkipVerify: true}, //nolint:gosec // local test
		QUICConfig:      &quic.Config{},
	}
	defer func() { _ = rt.Close() }()
	client := &http.Client{Transport: rt}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+addr+"/health", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return fmt.Errorf("H3 GET %s: status = %d, want 200", addr, resp.StatusCode)
	}
	if resp.ProtoMajor != 3 {
		return fmt.Errorf("H3 GET %s: proto major = %d, want 3", addr, resp.ProtoMajor)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		return fmt.Errorf("H3 GET %s: read body: %w", addr, err)
	}
	return nil
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

// TestQUICListener_RegistersSSLNamesAtZero pins SPEC §3.4: a QUIC listener
// registers all THREE ssl.* counters (because startQUIC hard-errors without a
// TLS config, so rt.tlsMode is necessarily true) and they stay permanently ZERO
// across a COMPLETED HTTP/3 handshake — because quic-go's Accept returns
// post-handshake, so a QUIC handshake never surfaces as a per-connection event
// on the TCP serveConnection path that could increment them.
//
// ⚠️ This is PARITY, not a departure. The reference behaves IDENTICALLY:
// ssl.handshake: 0 after five successful H3 connections, and connection_error: 0
// on a failure arm where the TCP comparator in the SAME process and the SAME
// scrape fired (phase-74 SPEC §3.4, EXECUTED). Gating QUIC out — which this
// SPEC's own first reading did — would have been the departure. Break D adds
// that refuted `rt.kind != kindQUIC` gate and this test's assertion (1) is the
// only thing in the suite that separates it from the landed behavior.
//
// ⚠️ Assertion order is load-bearing: (1) is the NAME-SET comparison and must be
// the discriminating failure under Break D. (4)'s zero check cannot discriminate
// on its own — with nothing registered every read is 0 — which is why
// counterValue errors on an absent name rather than returning a silent 0.
//
// ⚠️ Only the counter half of the pre-existing cx pin is asserted here.
// downstream_cx_active is a gauge; pollCounter type-asserts *stats.Counter and
// has no gauge equivalent, and no test in this file asserts the gauge. The gauge
// half is deliberately UNASSERTED.
func TestQUICListener_RegistersSSLNamesAtZero(t *testing.T) {
	cm := mkClusterMgr(t, "c_echo", "127.0.0.1", 9999)
	l := mkQUICListenerHCM(t, testAlphaCertPEM, testAlphaKeyPEM, hcmv3.HttpConnectionManager_HTTP3)
	boot := mkBoot(0, []*listenerv3.Listener{l}, nil)
	reg := stats.NewRegistry()
	mgr, err := NewManager(boot, cm, reg, testHTTPRegistry())
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
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
	prefix := "listener." + normalizeAddr(addr) + "."

	// (1) REGISTRATION: all three names present, spelled exactly. A cardinality
	//     assertion would pass with all three misspelled — compare the set.
	want := []string{
		prefix + "ssl.fail_verify_error",
		prefix + "ssl.fail_verify_no_cert",
		prefix + "ssl.handshake",
	}
	if got := listenerSSLNames(reg, addr); !reflect.DeepEqual(got, want) {
		t.Errorf("QUIC listener ssl name set = %v, want %v", got, want)
	}

	// (2) drive a REAL H3 request. A failed drive would make (3) and (4)
	//     vacuous, so this is a PRECONDITION, not a property.
	if err := driveH3(t, addr); err != nil {
		t.Fatalf("precondition: H3 round trip failed, so the zero-assertion below would be vacuous: %v", err)
	}

	// (3) the cx counter DID move — proving the connection was accounted, so the
	//     zeros in (4) are a real observation and not "nothing happened".
	if got := pollCounter(t, reg, prefix+"downstream_cx_total", 1, 2*time.Second); got < 1 {
		t.Errorf("downstream_cx_total = %d, want >= 1", got)
	}

	// (4) ...and all three ssl.* counters are STILL ZERO.
	for _, n := range want {
		if v := counterValue(t, reg, n); v != 0 {
			t.Errorf("%s = %d after a completed H3 handshake, want 0", n, v)
		}
	}

	// (5) and nothing panicked — implicit, but state it: a nil-Counter Inc from
	//     the QUIC accept goroutine would crash the binary, not fail this test
	//     (reference_nil_stats_counter_inc_crashes_goroutine).
}
