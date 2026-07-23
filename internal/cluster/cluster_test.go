package cluster

import (
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"

	"github.com/pgdad/envoy-go/internal/stats"
)

// echoConn reads bytes from c and writes them back until the connection closes.
// Uses an explicit read/write loop rather than io.Copy to avoid the Linux
// splice-on-loopback deadlock when src == dst is a *net.TCPConn.
func echoConn(c net.Conn) {
	defer func() { _ = c.Close() }()
	buf := make([]byte, 4096)
	for {
		n, err := c.Read(buf)
		if n > 0 {
			_, _ = c.Write(buf[:n])
		}
		if err != nil {
			return
		}
	}
}

// mkTestCluster builds a minimal Cluster for unit tests, bypassing manager
// validation. eps must have at least one entry. Allocates the 8 cluster-scope
// metrics on a throwaway Registry so the Dial / DialH2 hot paths (Task 9) can
// Inc/Dec their upstream-cx counters without nil-deref. Each call gets its own
// Registry — never Frozen, never observed via /stats/prometheus — matching
// the per-test isolation pattern manager_test.go uses for the cluster manager
// build path. Hyphens in `name` are normalized to underscores when forming the
// metric prefix so tests may continue to use conceptual names like "test-tls"
// (the SN-name regex in registry.go rejects hyphens; the cluster's `name`
// field itself is preserved as-is for assertion purposes).
func mkTestCluster(name string, upstreamCfg *stdtls.Config, eps ...Endpoint) *Cluster {
	c := &Cluster{
		name:           name,
		connectTimeout: time.Second,
		endpoints:      eps,
		lb:             &roundRobin{endpoints: eps},
		upstreamCfg:    upstreamCfg,
	}
	// Build a name-sanitized clone for metric registration only; the original
	// `c` keeps its hyphenated name so callers' identity assertions hold.
	tmp := &Cluster{name: strings.ReplaceAll(name, "-", "_"), endpoints: eps}
	registerClusterMetrics(stats.NewRegistry(), tmp)
	c.upstreamRqTotal = tmp.upstreamRqTotal
	c.upstreamRq2xx = tmp.upstreamRq2xx
	c.upstreamRq3xx = tmp.upstreamRq3xx
	c.upstreamRq4xx = tmp.upstreamRq4xx
	c.upstreamRq5xx = tmp.upstreamRq5xx
	c.upstreamCxTotal = tmp.upstreamCxTotal
	c.upstreamCxActive = tmp.upstreamCxActive
	c.membershipTotal = tmp.membershipTotal
	return c
}

// listenTCP starts a plaintext TCP echo server on a random loopback port and
// returns the listener. The caller is responsible for closing it.
func listenTCP(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go echoConn(c)
		}
	}()
	return ln
}

// listenTLS starts a TLS echo server on a random loopback port and returns
// the listener. The caller is responsible for closing it.
func listenTLS(t *testing.T, cfg *stdtls.Config) net.Listener {
	t.Helper()
	ln, err := stdtls.Listen("tcp", "127.0.0.1:0", cfg)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go echoConn(c)
		}
	}()
	return ln
}

// endpointFromAddr parses a "host:port" net.Addr into an Endpoint.
func endpointFromAddr(addr net.Addr) Endpoint {
	host, portStr, _ := net.SplitHostPort(addr.String())
	port := uint32(0)
	for _, b := range portStr {
		port = port*10 + uint32(b-'0')
	}
	return Endpoint{Host: host, Port: port}
}

// ---------------------------------------------------------------------------
// Dial — plaintext
// ---------------------------------------------------------------------------

func TestCluster_Dial_Plaintext(t *testing.T) {
	ln := listenTCP(t)
	defer func() { _ = ln.Close() }()

	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test", nil, ep)

	conn, _, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	_, _ = conn.Write([]byte("ping"))
	buf := make([]byte, 4)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "ping" {
		t.Errorf("got %q, want %q", buf, "ping")
	}
}

// ---------------------------------------------------------------------------
// Dial — TLS
// ---------------------------------------------------------------------------

// tlsPairForTest builds a server/client *stdtls.Config pair against the
// 0002-tls-tcp fixture's PKI (upstream-alpha cert/key + its CA), for tests
// that need a TLS echo server and a matching upstream client config without
// duplicating cert-loading. Shared by TestCluster_Dial_TLS and
// TestDialSink_TLS.
func tlsPairForTest(t *testing.T) (srv, cli *stdtls.Config) {
	t.Helper()
	caPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/ca.pem")
	if err != nil {
		t.Fatalf("read ca.pem: %v", err)
	}
	certPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.pem")
	if err != nil {
		t.Fatalf("read upstream-alpha.pem: %v", err)
	}
	keyPEM, err := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.key.pem")
	if err != nil {
		t.Fatalf("read upstream-alpha.key.pem: %v", err)
	}

	pair, err := stdtls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		t.Fatal(err)
	}
	srv = &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		MinVersion:   stdtls.VersionTLS12,
	}

	// Build upstream *stdtls.Config against this server.
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	cli = &stdtls.Config{
		ServerName: "alpha.envoy-go.test",
		RootCAs:    pool,
		MinVersion: stdtls.VersionTLS12,
		MaxVersion: stdtls.VersionTLS13,
	}
	return srv, cli
}

func TestCluster_Dial_TLS(t *testing.T) {
	srvCfg, upCfg := tlsPairForTest(t)
	ln := listenTLS(t, srvCfg)
	defer func() { _ = ln.Close() }()

	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-tls", upCfg, ep)

	conn, _, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// Phase 06.1 Task 9: Cluster.Dial wraps every successful conn in a
	// *connWithGauge for upstream_cx_active gauge bookkeeping; the inner
	// conn for a TLS cluster is *stdtls.Conn. Assert through the wrapper.
	wrapped, ok := conn.(*connWithGauge)
	if !ok {
		t.Fatalf("want *connWithGauge, got %T", conn)
	}
	if _, ok := wrapped.Conn.(*stdtls.Conn); !ok {
		t.Errorf("want *stdtls.Conn under *connWithGauge, got %T", wrapped.Conn)
	}

	_, _ = conn.Write([]byte("secret"))
	buf := make([]byte, 6)
	if _, err := io.ReadFull(conn, buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != "secret" {
		t.Errorf("got %q, want %q", buf, "secret")
	}
}

// ---------------------------------------------------------------------------
// Dial — TLS handshake failure
// ---------------------------------------------------------------------------

func TestCluster_Dial_TLS_HandshakeFailure(t *testing.T) {
	certPEM, _ := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.pem")
	keyPEM, _ := os.ReadFile("../../test/fixtures/0002-tls-tcp/pki/upstream-alpha.key.pem")
	pair, _ := stdtls.X509KeyPair(certPEM, keyPEM)

	ln := listenTLS(t, &stdtls.Config{
		Certificates: []stdtls.Certificate{pair},
		MinVersion:   stdtls.VersionTLS12,
	})
	defer func() { _ = ln.Close() }()

	// Upstream config with an empty cert pool — handshake fails.
	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-bad-ca", &stdtls.Config{
		ServerName: "alpha.envoy-go.test",
		RootCAs:    x509.NewCertPool(),
		MinVersion: stdtls.VersionTLS12,
	}, ep)

	_, _, err := c.Dial(context.Background())
	if err == nil || !strings.HasPrefix(err.Error(), "cluster: tls: handshake:") {
		t.Errorf("want cluster: tls: handshake: prefix, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Dial — context already canceled
// ---------------------------------------------------------------------------

func TestCluster_Dial_CtxCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ep := Endpoint{Host: "127.0.0.1", Port: 1} // unreachable
	c := mkTestCluster("test", nil, ep)

	_, _, err := c.Dial(ctx)
	if err == nil {
		t.Error("want ctx error")
	}
}

// ---------------------------------------------------------------------------
// UseH2 — accessor coverage (phase 05.2 Task 9)
// ---------------------------------------------------------------------------

// TestCluster_UseH2_DefaultsFalse verifies the zero-value Cluster reports
// useH2 == false, matching every existing cluster build path that doesn't
// set the field. Task 10 wires up the actual setter from HttpProtocolOptions.
func TestCluster_UseH2_DefaultsFalse(t *testing.T) {
	c := &Cluster{}
	if c.UseH2() {
		t.Errorf("zero-value Cluster.UseH2() = true, want false")
	}
}

// TestCluster_UseH2_True verifies a Cluster constructed with useH2: true
// reports UseH2() == true. This is the path Task 10's HttpProtocolOptions
// parser will exercise.
func TestCluster_UseH2_True(t *testing.T) {
	c := &Cluster{useH2: true}
	if !c.UseH2() {
		t.Errorf("Cluster.UseH2() = false, want true (with useH2: true)")
	}
}

// ---------------------------------------------------------------------------
// statusClassCounter — Phase 06.1 Task 8 [ADR-0063]
// ---------------------------------------------------------------------------

// TestCluster_StatusClassCounter_Buckets verifies the integer-divide code/100
// dispatch returns the matching counter for codes in [200, 599] and nil for
// codes outside that range (per Rule SN4 of SPEC §10.1; 1xx informationals
// are NOT bucketed in the upstream_rq_<Nxx> family).
func TestCluster_StatusClassCounter_Buckets(t *testing.T) {
	r := stats.NewRegistry()
	bs := mkBootstrap(mkStaticCluster("c_status", mkLbEndpoint("127.0.0.1", 9001)))
	m, err := NewManager(bs, r)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	c, ok := m.Get("c_status")
	if !ok {
		t.Fatal("cluster c_status not found")
	}
	cases := []struct {
		code int
		want *stats.Counter
	}{
		{200, c.upstreamRq2xx},
		{204, c.upstreamRq2xx},
		{299, c.upstreamRq2xx},
		{301, c.upstreamRq3xx},
		{304, c.upstreamRq3xx},
		{400, c.upstreamRq4xx},
		{404, c.upstreamRq4xx},
		{499, c.upstreamRq4xx},
		{500, c.upstreamRq5xx},
		{502, c.upstreamRq5xx},
		{599, c.upstreamRq5xx},
		// Outside [200, 599] → nil.
		{0, nil},
		{99, nil},
		{100, nil},
		{199, nil},
		{600, nil},
		{999, nil},
	}
	for _, tc := range cases {
		if got := c.statusClassCounter(tc.code); got != tc.want {
			t.Errorf("statusClassCounter(%d) = %p, want %p", tc.code, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Dial — upstream_cx metric wiring (phase 06.1 Task 9)
// ---------------------------------------------------------------------------

// TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose verifies the upstream-cx
// metric wiring for plaintext Dial: pre-Dial both metrics are zero; a
// successful Dial Incs upstream_cx_total AND upstream_cx_active by 1; the
// returned conn's Close() Decs upstream_cx_active back to 0 (and leaves
// upstream_cx_total monotonic at 1). Per ADR-0063: the connWithGauge wrapper
// is Cluster.Dial's responsibility — every successful Dial must Inc the
// counter once and Inc/Dec the gauge symmetrically.
func TestDial_IncsCxMetricsAndWrapsForActiveDecOnClose(t *testing.T) {
	ln := listenTCP(t)
	defer func() { _ = ln.Close() }()

	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-cx", nil, ep)

	// Pre-Dial: both zero.
	if got := c.upstreamCxTotal.Load(); got != 0 {
		t.Errorf("pre-Dial upstream_cx_total = %d, want 0", got)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Errorf("pre-Dial upstream_cx_active = %d, want 0", got)
	}

	conn, _, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Post-Dial: total=1, active=1.
	if got := c.upstreamCxTotal.Load(); got != 1 {
		t.Errorf("post-Dial upstream_cx_total = %d, want 1", got)
	}
	if got := c.upstreamCxActive.Load(); got != 1 {
		t.Errorf("post-Dial upstream_cx_active = %d, want 1", got)
	}

	// Close: active back to 0; total still at 1 (counter is monotonic).
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Errorf("post-Close upstream_cx_active = %d, want 0", got)
	}
	if got := c.upstreamCxTotal.Load(); got != 1 {
		t.Errorf("post-Close upstream_cx_total = %d, want 1 (monotonic)", got)
	}
}

// TestDial_CloseIdempotent verifies the connWithGauge wrapper's sync.Once
// guard: calling Close twice on the returned conn must Dec the active gauge
// exactly once (so it lands at 0, never -1). Defends against a future
// caller that double-closes through both an explicit Close and a defer
// Close, or against the Go HTTP-stack pattern where (*http.Response).Body
// closes the underlying conn after the caller has already done so.
func TestDial_CloseIdempotent(t *testing.T) {
	ln := listenTCP(t)
	defer func() { _ = ln.Close() }()

	ep := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-cx-idem", nil, ep)

	conn, _, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	_ = conn.Close()
	_ = conn.Close() // second Close MUST be a no-op for the gauge.

	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Errorf("after double-Close upstream_cx_active = %d, want 0 (sync.Once must guard the Dec)", got)
	}
}

// ---------------------------------------------------------------------------
// Dial — surfaced endpoint (phase 06.2 Task 8)
// ---------------------------------------------------------------------------

// TestCluster_Dial_ReturnsPickedEndpoint verifies that Dial surfaces the
// picked Endpoint in the second return value. The returned ep.Host must be
// non-empty and must match the single endpoint the test cluster was built
// with (round-robin over one endpoint always returns the same one).
func TestCluster_Dial_ReturnsPickedEndpoint(t *testing.T) {
	ln := listenTCP(t)
	defer func() { _ = ln.Close() }()

	want := endpointFromAddr(ln.Addr())
	c := mkTestCluster("test-ep-surface", nil, want)

	conn, got, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if got.Host == "" {
		t.Errorf("returned Endpoint.Host is empty; want non-empty")
	}
	if got.Host != want.Host || got.Port != want.Port {
		t.Errorf("returned Endpoint = %v, want %v", got, want)
	}
}

// ---------------------------------------------------------------------------
// LB release threading — phase 34 Task 2 (ADR-0232 OPTION C)
// ---------------------------------------------------------------------------

// stubLB is a test-only loadBalancer whose pick increments an observable
// counter and whose release decrements it, so the cluster.go release threading
// (Dial / AcquireH1 / dial-failure / double-Close / closePool drain) is
// testable in isolation from leastRequest (which does not exist yet at Task 2).
type stubLB struct {
	ep     Endpoint
	active atomic.Int64
}

func (s *stubLB) Pick(_ uint64, _ bool, _ SubsetMatch, _ bool) (Endpoint, func(), error) {
	s.active.Add(1)
	var once sync.Once
	return s.ep, func() { once.Do(func() { s.active.Add(-1) }) }, nil
}

// newTestClusterLB builds a *Cluster wired to the supplied loadBalancer with the
// 8 cluster-scope metrics registered (so the Dial/AcquireH1 hot paths can
// Inc/Dec their upstream-cx counters without nil-deref). Reuses mkTestCluster's
// metric registration, then swaps in the custom LB and endpoint set.
func newTestClusterLB(t *testing.T, lb loadBalancer, eps ...Endpoint) *Cluster {
	t.Helper()
	c := mkTestCluster("test-stublb", nil, eps...)
	c.lb = lb
	c.endpoints = eps
	return c
}

// pickErrLB is a loadBalancer whose Pick always fails — models an empty /
// all-unavailable cluster (the LB-pick-failure path). Used to prove AcquireH1
// surfaces a ZERO Endpoint when no host was picked.
type pickErrLB struct{}

func (pickErrLB) Pick(_ uint64, _ bool, _ SubsetMatch, _ bool) (Endpoint, func(), error) {
	return Endpoint{}, func() {}, errNoEndpoints
}

func TestAcquireH1_SurfacesPickedEndpointOnDialFailure(t *testing.T) {
	// A host IS picked but the dial fails (port 1 refused). AcquireH1's new ep
	// return MUST carry the picked endpoint (non-zero) alongside the error so
	// the router can attribute the local-origin connect failure to that host.
	ep := Endpoint{Host: "127.0.0.1", Port: 1} // port 1: refused
	stub := &stubLB{ep: ep}
	c := newTestClusterLB(t, stub, ep)
	p, got, err := c.AcquireH1(context.Background())
	if err == nil {
		t.Fatal("expected dial error")
	}
	if p != nil {
		t.Fatalf("expected nil PooledH1Conn on failure, got %v", p)
	}
	if got.IsZero() {
		t.Fatalf("expected non-zero picked endpoint on dial failure, got zero")
	}
	if got.Host != ep.Host || got.Port != ep.Port {
		t.Fatalf("surfaced endpoint = %s, want %s", got.Addr(), ep.Addr())
	}
}

func TestAcquireH1_SurfacesZeroEndpointOnPickFailure(t *testing.T) {
	// An empty / all-unavailable cluster: the LB Pick fails before any host is
	// chosen. AcquireH1 MUST return a ZERO Endpoint (IsZero) so the router skips
	// the local-origin attribution (no host to blame).
	c := newTestClusterLB(t, pickErrLB{})
	p, got, err := c.AcquireH1(context.Background())
	if err == nil {
		t.Fatal("expected pick error")
	}
	if p != nil {
		t.Fatalf("expected nil PooledH1Conn on failure, got %v", p)
	}
	if !got.IsZero() {
		t.Fatalf("expected zero picked endpoint on pick failure, got %s", got.Addr())
	}
}

func TestDial_SurfacesPickedEndpointOnDialFailure(t *testing.T) {
	// A host IS picked but the dial fails (port 1 refused). Dial's ep return MUST
	// carry the picked endpoint (non-zero) alongside the error so the router can
	// attribute the local-origin connect failure to that host. Mirrors the
	// AcquireH1 surfacing lock; the existing Dial-failure tests discard ep with _.
	ep := Endpoint{Host: "127.0.0.1", Port: 1} // port 1: refused
	stub := &stubLB{ep: ep}
	c := newTestClusterLB(t, stub, ep)
	conn, got, err := c.Dial(context.Background())
	if err == nil {
		t.Fatal("expected dial error")
	}
	if conn != nil {
		t.Fatalf("expected nil net.Conn on failure, got %v", conn)
	}
	if got.IsZero() {
		t.Fatalf("expected non-zero picked endpoint on dial failure, got zero")
	}
	if got.Host != ep.Host || got.Port != ep.Port {
		t.Fatalf("surfaced endpoint = %s, want %s", got.Addr(), ep.Addr())
	}
}

func TestDial_ReleasesOnConnClose(t *testing.T) {
	// Listener that accepts and immediately closes — Dial succeeds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()

	stub := &stubLB{ep: endpointFromAddr(ln.Addr())}
	c := newTestClusterLB(t, stub, stub.ep)
	conn, _, err := c.Dial(context.Background())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	if got := stub.active.Load(); got != 1 {
		t.Fatalf("after Dial: active = %d, want 1 (held until Close)", got)
	}
	_ = conn.Close()
	if got := stub.active.Load(); got != 0 {
		t.Fatalf("after Close: active = %d, want 0", got)
	}
	_ = conn.Close() // double-Close must NOT underflow
	if got := stub.active.Load(); got != 0 {
		t.Fatalf("after double-Close: active = %d, want 0", got)
	}
}

func TestDial_ReleasesOnDialFailure(t *testing.T) {
	// Point at a port nothing listens on → dial fails → release MUST fire.
	stub := &stubLB{ep: Endpoint{Host: "127.0.0.1", Port: 1}} // port 1: refused
	c := newTestClusterLB(t, stub, stub.ep)
	_, _, err := c.Dial(context.Background())
	if err == nil {
		t.Fatal("expected dial error")
	}
	if got := stub.active.Load(); got != 0 {
		t.Errorf("after dial failure: active = %d, want 0 (release-on-failure)", got)
	}
}

func TestAcquireH1_PoolHitReleasesImmediately(t *testing.T) {
	// First AcquireH1 dials fresh (active=1 held by the conn). PutIdleH1 returns
	// it to the pool (still active=1 — cx-as-rq). Second AcquireH1 is a POOL HIT:
	// its fresh pick releases immediately, so active stays 1 (the pooled conn's
	// dial-time hold persists). Final Close → active 0.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() { _, _ = io.Copy(io.Discard, c) }()
		}
	}()
	stub := &stubLB{ep: endpointFromAddr(ln.Addr())}
	c := newTestClusterLB(t, stub, stub.ep)

	p1, _, err := c.AcquireH1(context.Background())
	if err != nil {
		t.Fatalf("AcquireH1 miss: %v", err)
	}
	if got := stub.active.Load(); got != 1 {
		t.Fatalf("after dial: active=%d want 1", got)
	}
	c.PutIdleH1(p1)
	if got := stub.active.Load(); got != 1 {
		t.Fatalf("after PutIdle: active=%d want 1 (cx-as-rq hold persists)", got)
	}

	p2, _, err := c.AcquireH1(context.Background())
	if err != nil {
		t.Fatalf("AcquireH1 hit: %v", err)
	}
	if got := stub.active.Load(); got != 1 {
		t.Fatalf("after pool hit: active=%d want 1 (fresh pick released immediately)", got)
	}
	_ = p2.Conn.Close()
	if got := stub.active.Load(); got != 0 {
		t.Fatalf("after Close: active=%d want 0", got)
	}
}

// ---------------------------------------------------------------------------
// EnsureRetryStats — phase 42.2a Task 5 (ADR-0249 +6th counter)
// ---------------------------------------------------------------------------

// mkRetryCluster builds a minimal Cluster with statsReg + statsPrefix stashed
// (via registerClusterMetrics) so EnsureRetryStats can be called on it.
// Returns both the Cluster and the Registry for Walk-based introspection.
func mkRetryCluster(t *testing.T, name string) (*Cluster, *stats.Registry) {
	t.Helper()
	r := stats.NewRegistry()
	ep := Endpoint{Host: "127.0.0.1", Port: 9001}
	c := &Cluster{
		name:           name,
		connectTimeout: time.Second,
		endpoints:      []Endpoint{ep},
		lb:             &roundRobin{endpoints: []Endpoint{ep}},
	}
	registerClusterMetrics(r, c)
	return c, r
}

// counterNames returns the set of counter names visible in the registry.
func counterNames(r *stats.Registry) map[string]bool {
	names := make(map[string]bool)
	r.Walk(func(m stats.Metric) {
		if m.Type() == stats.MetricCounter {
			names[m.Name()] = true
		}
	})
	return names
}

// TestEnsureRetryStats_RegistersSixCounters verifies that after
// EnsureRetryStats() the cluster registers exactly the 6 retry counters
// (the existing 5 + upstream_rq_per_try_timeout), that
// IncUpstreamRqPerTryTimeout increments the new counter from 0→1, and
// that a second EnsureRetryStats() call is idempotent (no panic, same
// handles). (ADR-0249; phase 42.2a Task 5)
func TestEnsureRetryStats_RegistersSixCounters(t *testing.T) {
	c, r := mkRetryCluster(t, "rc_six")
	p := "cluster.rc_six."

	// Before EnsureRetryStats: retry counters must NOT be present.
	before := counterNames(r)
	retryNames := []string{
		p + "upstream_rq_retry",
		p + "upstream_rq_retry_success",
		p + "upstream_rq_retry_limit_exceeded",
		p + "upstream_rq_retry_backoff_exponential",
		p + "upstream_rq_retry_backoff_ratelimited",
		p + "upstream_rq_per_try_timeout",
	}
	for _, n := range retryNames {
		if before[n] {
			t.Errorf("counter %q present before EnsureRetryStats, want absent", n)
		}
	}

	// After EnsureRetryStats: all 6 must appear.
	c.EnsureRetryStats()
	after := counterNames(r)
	for _, n := range retryNames {
		if !after[n] {
			t.Errorf("counter %q absent after EnsureRetryStats, want present", n)
		}
	}

	// IncUpstreamRqPerTryTimeout: 0 → 1.
	c.IncUpstreamRqPerTryTimeout()
	// Read the value via Walk to avoid reaching into unexported fields.
	var got uint64
	r.Walk(func(m stats.Metric) {
		if m.Name() == p+"upstream_rq_per_try_timeout" {
			if cnt, ok := m.(*stats.Counter); ok {
				got = cnt.Load()
			}
		}
	})
	if got != 1 {
		t.Errorf("after IncUpstreamRqPerTryTimeout: value = %d, want 1", got)
	}

	// Idempotency: second call must not panic and must keep the same handle.
	c.EnsureRetryStats()
	after2 := counterNames(r)
	for _, n := range retryNames {
		if !after2[n] {
			t.Errorf("counter %q absent after second EnsureRetryStats, want present", n)
		}
	}
	// Value must still be 1 (same handle — no double-registration).
	r.Walk(func(m stats.Metric) {
		if m.Name() == p+"upstream_rq_per_try_timeout" {
			if cnt, ok := m.(*stats.Counter); ok {
				got = cnt.Load()
			}
		}
	})
	if got != 1 {
		t.Errorf("after second EnsureRetryStats: counter value = %d, want 1 (same handle)", got)
	}
}

// TestEnsureRetryStats_NilGuardIncIsNoop verifies that a Cluster without
// EnsureRetryStats (no retry policy) treats IncUpstreamRqPerTryTimeout as a
// no-op and registers NONE of the 6 retry counters. (ADR-0249)
func TestEnsureRetryStats_NilGuardIncIsNoop(t *testing.T) {
	c, r := mkRetryCluster(t, "rc_noop")
	p := "cluster.rc_noop."

	// Never call EnsureRetryStats — IncUpstreamRqPerTryTimeout must not panic.
	c.IncUpstreamRqPerTryTimeout()

	// None of the 6 retry counters must be registered.
	names := counterNames(r)
	retryNames := []string{
		p + "upstream_rq_retry",
		p + "upstream_rq_retry_success",
		p + "upstream_rq_retry_limit_exceeded",
		p + "upstream_rq_retry_backoff_exponential",
		p + "upstream_rq_retry_backoff_ratelimited",
		p + "upstream_rq_per_try_timeout",
	}
	for _, n := range retryNames {
		if names[n] {
			t.Errorf("counter %q present on non-retry cluster, want absent", n)
		}
	}
}

// ---------------------------------------------------------------------------
// DialSink (phase 55 Task 1) — the unaccounted stats-sink dial
// ---------------------------------------------------------------------------

// TestDialSink_NoCxAccounting pins AMEND-TCP-CXSTATS: the reference's statsd TCP
// connection reports upstream_cx_total: 0 / upstream_cx_active: 0. DialSink must
// leave BOTH counters untouched, before and after the conn's Close.
func TestDialSink_NoCxAccounting(t *testing.T) {
	ln := listenTCP(t)
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(ln.Addr()))

	conn, err := c.DialSink(context.Background())
	if err != nil {
		t.Fatalf("DialSink: %v", err)
	}
	if got := c.upstreamCxTotal.Load(); got != 0 {
		t.Errorf("upstream_cx_total after DialSink = %d, want 0", got)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Errorf("upstream_cx_active after DialSink = %d, want 0", got)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := c.upstreamCxActive.Load(); got != 0 {
		t.Errorf("upstream_cx_active after Close = %d, want 0 (never Inc'd, must not go negative)", got)
	}
	if got := c.upstreamCxTotal.Load(); got != 0 {
		t.Errorf("upstream_cx_total after Close = %d, want 0", got)
	}
}

// TestDialSink_TakesNoConnPermit is the decisive AMEND-TCP-CXSTATS test. The probe
// showed the reference STILL connects and flushes with max_connections: 0 on the
// stats cluster. envoy-go's connPool returns errConnPoolOverflow on the FIRST
// acquire at maxConnections=0/maxPending=0 (connpool_test.go:295 "(f)"), so Dial
// FAILS on such a cluster while DialSink MUST succeed. This test would pass
// vacuously if DialSink merely skipped the Inc but still took the permit.
func TestDialSink_TakesNoConnPermit(t *testing.T) {
	ln := listenTCP(t)
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(ln.Addr()))
	attachConnPool(c, 0, 0) // max_connections: 0, max_pending_requests: 0

	// Control: Dial must be refused by the permit.
	if _, _, err := c.Dial(context.Background()); !errors.Is(err, errConnPoolOverflow) {
		t.Fatalf("Dial with max_connections=0: got %v, want errConnPoolOverflow", err)
	}
	// DialSink bypasses the permit entirely.
	conn, err := c.DialSink(context.Background())
	if err != nil {
		t.Fatalf("DialSink with max_connections=0: %v (must bypass the permit)", err)
	}
	_ = conn.Close()
}

// TestDialSink_ReturnsBareConn: no connWithGauge wrapper — there is no gauge to
// Dec and no permit to release, so wrapping would be a lie (and connDec would
// underflow the pool).
func TestDialSink_ReturnsBareConn(t *testing.T) {
	ln := listenTCP(t)
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(ln.Addr()))
	conn, err := c.DialSink(context.Background())
	if err != nil {
		t.Fatalf("DialSink: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, wrapped := conn.(*connWithGauge); wrapped {
		t.Fatal("DialSink returned a *connWithGauge; want the bare net.Conn")
	}
}

// TestDialSink_TLS: DialSink honors upstream TLS, like Dial.
func TestDialSink_TLS(t *testing.T) {
	srvCfg, cliCfg := tlsPairForTest(t)
	ln := listenTLS(t, srvCfg)
	c := mkTestCluster("c_statsd", cliCfg, endpointFromAddr(ln.Addr()))
	conn, err := c.DialSink(context.Background())
	if err != nil {
		t.Fatalf("DialSink over TLS: %v", err)
	}
	defer func() { _ = conn.Close() }()
	if _, ok := conn.(*stdtls.Conn); !ok {
		t.Fatalf("DialSink over TLS returned %T, want *tls.Conn", conn)
	}
}

// TestDialSink_CtxCanceled: a canceled ctx short-circuits before the pick.
func TestDialSink_CtxCanceled(t *testing.T) {
	ln := listenTCP(t)
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(ln.Addr()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.DialSink(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("DialSink with canceled ctx: got %v, want context.Canceled", err)
	}
}

// TestDialSink_DialFailure surfaces the same wrapped error string as Dial.
func TestDialSink_DialFailure(t *testing.T) {
	ln := listenTCP(t)
	// endpointFromAddr takes a net.Addr (cluster_test.go:110), NOT a string.
	// The net.Addr VALUE stays usable after the listener closes.
	addr := ln.Addr()
	_ = ln.Close() // nothing is listening now
	c := mkTestCluster("c_statsd", nil, endpointFromAddr(addr))
	_, err := c.DialSink(context.Background())
	if err == nil {
		t.Fatal("DialSink to a closed port: want error, got nil")
	}
	if !strings.Contains(err.Error(), "cluster: dial: ") {
		t.Errorf("error %q should carry the byte-stable prefix %q", err, "cluster: dial: ")
	}
}

// ---------------------------------------------------------------------------
// Phase 72 Task 1 — Endpoint.filterMetadata retention + the MetaLookup accessor
// ---------------------------------------------------------------------------

// TestEndpoint_MetaLookup pins the accessor's own return contract (NOT a span
// attribute — a span-routed assertion would be vacuous, PLAN F5b): a present
// namespace yields (non-nil, true) wrapping the WHOLE namespace struct; an
// absent namespace, the ZERO Endpoint and a nil-valued namespace all yield
// (nil, false) without panicking.
func TestEndpoint_MetaLookup(t *testing.T) {
	st, err := structpb.NewStruct(map[string]any{"k": "v"})
	if err != nil {
		t.Fatalf("structpb.NewStruct: %v", err)
	}
	e := Endpoint{Host: "127.0.0.1", Port: 9001, filterMetadata: map[string]*structpb.Struct{"ns": st}}

	// (a) present namespace → the whole namespace struct, wrapped.
	v, ok := e.MetaLookup("ns")
	if !ok {
		t.Errorf("MetaLookup(\"ns\") ok = false, want true")
	}
	if v == nil {
		t.Errorf("MetaLookup(\"ns\") value = nil, want a non-nil StructValue wrapper")
	} else if got := v.GetStructValue().GetFields()["k"].GetStringValue(); got != "v" {
		t.Errorf("MetaLookup(\"ns\") wrapped fields[k] = %q, want %q", got, "v")
	}

	// (b) absent namespace → (nil, false).
	if v, ok := e.MetaLookup("absent"); v != nil || ok {
		t.Errorf("MetaLookup(\"absent\") = (%v, %v), want (nil, false)", v, ok)
	}

	// (c) the ZERO Endpoint (the 5 span-capable local-reply sites carry it) →
	// (nil, false), no panic on the nil map.
	var zero Endpoint
	if v, ok := zero.MetaLookup("ns"); v != nil || ok {
		t.Errorf("zero Endpoint MetaLookup(\"ns\") = (%v, %v), want (nil, false)", v, ok)
	}

	// (d) a namespace mapped to a nil *structpb.Struct → (nil, false) (the
	// guard's own contract: structpb.NewStructValue(nil) is NON-nil).
	nilNS := Endpoint{filterMetadata: map[string]*structpb.Struct{"ns": nil}}
	if v, ok := nilNS.MetaLookup("ns"); v != nil || ok {
		t.Errorf("nil-struct namespace MetaLookup(\"ns\") = (%v, %v), want (nil, false)", v, ok)
	}
}
