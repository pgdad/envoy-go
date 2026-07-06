// Package driver registers the 0079-h2-multiplex-pool cross-side differential
// fixture (phase 43.2a SPEC §8.1 / PLAN Task 9).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over an
// HTTP/2 (TLS+ALPN-h2) downstream listener whose concurrent streams fan out to
// an H2 (h2c) upstream multiplex pool. It proves the H2 multiplex connection
// pool: many concurrent upstream H2 STREAMS multiplexed onto a SMALL number of
// upstream connections — exactly ceil(K/C) of them, where C is the cluster's OWN
// http2_protocol_options.max_concurrent_streams (the LOCAL cap, AMEND-H2-1). The
// single backend is an H2HoldResponder (BackendKind 37) that HOLDS each
// "GET /<seg>" stream open until the driver releases the batch.
//
// # WHY an HTTP/2 DOWNSTREAM listener (departure from SPEC §8.1's wording)
//
// SPEC §8.1 described an "HTTP/1.1 downstream listener → H2-upstream cluster".
// That does NOT exercise envoy-go's H2 upstream pool: the HCM selects the H1 vs
// H2 router action by the DOWNSTREAM listener codec (internal/filter/hcm/
// filter.go:120 — HttpConnectionManager_HTTP2). An H1-downstream listener always
// drives the H1 upstream dial path (Cluster.AcquireH1/Dial), NEVER
// Cluster.AcquireH2Stream — even when the cluster's UseH2()==true. The H2
// multiplex pool (router_h2.go doH2ClusterAction → AcquireH2Stream) is reached
// ONLY from an H2 downstream listener. Real Envoy bridges protocols (H1-down →
// H2-up) so the reference would pool either way; envoy-go does not. To keep the
// differential CROSS-SIDE EXACT, BOTH sides therefore use an H2 (TLS+ALPN-h2)
// downstream listener — the 0004-h2-routing PKI/TLS shape — so both engage their
// H2 upstream pool identically. (Recorded as the binding topology finding for
// Task 9; the pool's conn/stream semantics are unchanged.)
//
// # Cross-side EXACT — NOT a 43.1-style robust departure (reference_h2_pool_local_cap_driven)
//
// The 2026-06-23 live probe (SPEC §11 D-H2-EXACT) confirmed the reference Envoy
// grows its H2 pool off the cluster's OWN max_concurrent_streams: with C set and
// K fully-overlapping held streams, the reference opens EXACTLY ceil(K/C)
// upstream connections — deterministic, zero errors, all 200s. This is CLEAN
// flow-control enforcement (contrast 0078/43.1's SOFT max_connections, which
// forced an exact-vs-robust prong split). So the H2 conn/stream COUNTS are
// asserted cross-side EXACT on BOTH sides:
//
//	cluster.c_h2mp.upstream_cx_total       == ceil(K/C)   (few conns)
//	cluster.c_h2mp.upstream_cx_http2_total == ceil(K/C)   (all H2)
//	cluster.c_h2mp.http2.streams_active    == K           (many streams)
//
// # The TWO-cluster topology (the design choice; D-H2-BACKEND)
//
// The fixture runs TWO prongs with IRRECONCILABLE single-cluster budgets:
//   - the ceil prong wants max_connections HIGH (>= ceil(K/C)) so the cap does
//     NOT bind — only the stream cap C drives multi-conn growth.
//   - the overflow prong wants max_connections=1 + max_pending_requests=1 (tight)
//     so a 3rd request OVERFLOWS the pending queue.
//
// So 0079 uses TWO h2c clusters BOTH pointing at the SAME backend host:
//   - c_h2mp: C=2, max_connections=16, max_pending_requests=16 — the EXACT ceil
//     prong (the cap never binds; conn growth is C-driven). K=6 ⇒ ceil(6/2)=3.
//   - c_h2of: C=1, max_connections=1, max_pending_requests=1 — the overflow prong
//     (1 held stream fills the 1 conn (C=1), a 2nd PENDS, a 3rd OVERFLOWS → 503).
//
// Two routes select the clusters by path prefix (/mp → c_h2mp, /of → c_h2of).
// The single backend's gate holds ALL streams from both clusters; the driver
// drives the prongs SEQUENTIALLY per side (ceil prong fully — incl. drain — then
// the overflow prong), so the shared gate stays clean.
//
// # Topology: 1 H2HoldResponder (runner-spawned, h2c)
//
//   - backend0 → c_h2mp endpoint 0 AND c_h2of endpoint 0 (the same host; both
//     clusters dial it). Advertises SETTINGS_MAX_CONCURRENT_STREAMS=1000 >> C so
//     the LOCAL cluster cap binds, not the peer (AMEND-H2-1/H2-5; no REFUSED_STREAM).
//
// BackendCount() is 1; the uniform BackendKind() is H2HoldResponder (BackendKind 37).
//
// # The staged drive (per side, inside AssertStats; SLEEPLESS)
//
// The runner's hooks are DriveReference/DriveSubject (the byte-equiv "READY\n"
// stream, run FIRST) then AssertStats (run LAST, holding BOTH admin addrs). All
// fill / multiplex-prove / pend / oversub / drain runs inside AssertStats. The
// Drive hooks stash listener addrs; the config builders stash the backend host
// port so AssertStats can hit 127.0.0.1:<backendPort>/__release (loopback) to
// drain. The held streams are concurrent HTTP/2 GETs (one fresh downstream
// ClientConn per held stream via helpers.H2RoundTrip; the upstream pool sees K
// concurrent streams regardless of downstream conn fan-out).
//
//	| Step | BOTH sides (cross-side EXACT)                                          |
//	|------|------------------------------------------------------------------------|
//	| 1    | Fire K held H2 GET /mp/<i> → poll upstream_cx_total==ceil(K/C) AND      |
//	|      | http2.streams_active==K AND upstream_cx_http2_total==ceil(K/C)          |
//	| 2    | multiplex proof: upstream_cx_total (== ceil(K/C)) << K                  |
//	| 3    | /__release → all K drain to 200 → poll http2.streams_active==0          |
//	| 4    | overflow prong: fire 1 held GET /of; fire a 2nd → poll                  |
//	|      | upstream_rq_pending_active==1; fire a 3rd → DOWNSTREAM 503 +             |
//	|      | upstream_rq_pending_overflow delta>=1                                   |
//	| 5    | /__release → drain → poll http2.streams_active==0 AND                   |
//	|      | upstream_rq_pending_active==0                                           |
//
// The overflow prong asserts the DOWNSTREAM response class (503 from the fired
// request's status, NOT upstream_rq_5xx —
// reference_concurrent_attempt_downstream_class_assertion). All polls use
// convergeDeadline — NEVER a fixed sleep (reference_concurrency_differential_release_barrier).
//
// # Cross-references
//
//   - phase 43.2a SPEC §8.1 / PLAN Task 9 (the fixture design).
//   - reference_h2_pool_local_cap_driven (ceil(K/C) keyed off the LOCAL cap; cross-side EXACT).
//   - 0004-h2-routing (the H2 TLS downstream listener + PKI + helpers.H2RoundTrip shape).
//   - 0078-connection-pool-max-connections (the cross-side fill/poll/release shape).
//   - reference_concurrent_attempt_downstream_class_assertion (count downstream 503s).
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS hostname).
//   - reference_differential_run_selector (target -run 'TestDifferential/0079').
//   - reference_fixture_workload_constant_desync (constants single-sourced).
//   - reference_differential_asserter_dispatch (StatsAsserter — NOT SubjectAsserter).
package driver

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0079-h2-multiplex-pool"

	// clusterMP is the EXACT ceil prong cluster (cap never binds; C drives growth).
	clusterMP = "c_h2mp"
	// clusterOF is the overflow prong cluster (tight budgets).
	clusterOF = "c_h2of"

	// In-container reference Envoy listener port. Fixtures run sequentially;
	// next-free is 19168 (0078 took 19167).
	refContainerListenerPort = 19168

	refAdminPort = 9901

	// backendCount is the number of runner-spawned H2HoldResponder hosts (1).
	backendCount = 1

	// streamCapMP (C) is c_h2mp's http2_protocol_options.max_concurrent_streams —
	// the per-conn stream budget. The pool opens a new conn only when every
	// existing conn holds C in-flight streams.
	streamCapMP = 2

	// heldK (K) is the number of fully-overlapping held streams fired at c_h2mp.
	// ceil(K/C) = ceil(6/2) = 3 connections (expectedConnsMP).
	heldK = 6

	// expectedConnsMP = ceil(heldK / streamCapMP) — the EXACT conn count both
	// sides open (cross-side EXACT; the clean local-cap-driven ceil).
	expectedConnsMP = (heldK + streamCapMP - 1) / streamCapMP // = 3

	// maxConnectionsMP / maxPendingMP are c_h2mp's circuit_breakers budgets, set
	// HIGH so they do NOT bind in the ceil prong (only C drives growth).
	maxConnectionsMP = 16
	maxPendingMP     = 16

	// The overflow prong (c_h2of): C=1, max_connections=1, max_pending_requests=1.
	// 1 held stream fills the 1 conn (C=1); a 2nd PENDS; a 3rd OVERFLOWS → 503.
	streamCapOF      = 1
	maxConnectionsOF = 1
	maxPendingOF     = 1

	// Convergence poll budget (NO fixed sleep — poll until the predicate holds).
	convergeDeadline = 15 * time.Second
	convergePoll     = 50 * time.Millisecond
)

// statKey builds a cluster-scoped stat name "cluster.<cluster>.<suffix>".
func statKey(cluster, suffix string) string { return "cluster." + cluster + "." + suffix }

func init() {
	fixture.RegisterFixture(fixtureName, &h2Driver{})
}

// h2Driver is STATEFUL: the Drive hooks stash the per-side listener addrs and the
// config builders stash the backend port, so AssertStats — the only hook holding
// BOTH admin addrs — can run the staged drive (fill / multiplex / pend / oversub
// / drain) and hit the backend's /__release control path to drain.
type h2Driver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_h2 listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_h2 listener (from SubjectConfig)
	backendPort  int    // the single backend's host port (for the /__release hit)
	rootCAs      *x509.CertPool
}

func (*h2Driver) BackendCount() int                { return backendCount }
func (*h2Driver) BackendKind() fixture.BackendKind { return fixture.H2HoldResponder }
func (*h2Driver) SubjectListenerName() string      { return "l_h2" }
func (*h2Driver) ReferenceListenerPort() int       { return refContainerListenerPort }

// fixtureDir resolves the absolute path to the fixture directory (the parent of
// this driver/ package), regardless of the test's working directory.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	return filepath.Dir(filepath.Dir(thisFile))
}

// readPEM reads a PEM file from the fixture's pki/ directory (the 0004 PKI,
// copied — listener.pem/listener.key.pem/ca.pem; SANs localhost +
// host.docker.internal + 127.0.0.1).
func readPEM(name string) string {
	path := filepath.Join(fixtureDir(), "pki", name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read pki/%s: %v", name, err))
	}
	return string(b)
}

// indentPEM returns the PEM body with every line after the first prefixed by
// `indent` spaces (for inline_string block-scalar embedding at a fixed depth).
func indentPEM(pem string, indent int) string {
	pad := strings.Repeat(" ", indent)
	var b strings.Builder
	for i, line := range strings.Split(strings.TrimRight(pem, "\n"), "\n") {
		if i > 0 {
			b.WriteByte('\n')
			b.WriteString(pad)
		}
		b.WriteString(line)
	}
	return b.String()
}

// stashBackendPort memoizes the single backend's host port. Both builders receive
// the same backendPorts slice and must agree on the SAME port (shared backend).
func (d *h2Driver) stashBackendPort(backendPorts []int) {
	d.mu.Lock()
	d.backendPort = backendPorts[0]
	d.mu.Unlock()
}

// h2ClusterBlock renders one H2 (h2c) upstream cluster with the given name,
// endpoint type/address, circuit_breakers budgets, and the H2
// typed_extension_protocol_options carrying max_concurrent_streams. h2c (no
// transport_socket) per ADR-0166; the backend advertises SETTINGS=1000 >> C so
// the LOCAL cap binds.
func h2ClusterBlock(name, clusterType, endpointAddr string, endpointPort, maxConns, maxPending, streamCap int) string {
	return fmt.Sprintf(`    - name: %s
      type: %s
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      typed_extension_protocol_options:
        envoy.extensions.upstreams.http.v3.HttpProtocolOptions:
          "@type": type.googleapis.com/envoy.extensions.upstreams.http.v3.HttpProtocolOptions
          explicit_http_config:
            http2_protocol_options:
              max_concurrent_streams: %d
      circuit_breakers:
        thresholds:
          - priority: DEFAULT
            max_connections: %d
            max_pending_requests: %d
      load_assignment:
        cluster_name: %s
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }
`, name, clusterType, streamCap, maxConns, maxPending, name, endpointAddr, endpointPort)
}

// routeTable routes /mp → c_h2mp and /of → c_h2of (the two prongs). Identical on
// both sides.
const routeTable = `                      routes:
                        - match: { prefix: "/mp" }
                          route: { cluster: c_h2mp }
                        - match: { prefix: "/of" }
                          route: { cluster: c_h2of }`

// h2ListenerFilterChain renders the TLS+ALPN-h2 downstream filter chain for the
// l_h2 listener (the 0004 shape: DownstreamTlsContext, alpn h2/http1.1,
// codec_type AUTO so ALPN drives per-conn selection — the driver advertises h2
// only). PEMs are embedded inline at the listener depth.
func h2ListenerFilterChain() string {
	certIndent := 24 // inline_string body depth under tls_certificates
	cert := indentPEM(readPEM("listener.pem"), certIndent)
	key := indentPEM(readPEM("listener.key.pem"), certIndent)
	return fmt.Sprintf(`        - transport_socket:
            name: envoy.transport_sockets.tls
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
              common_tls_context:
                alpn_protocols: ["h2", "http/1.1"]
                tls_certificates:
                  - certificate_chain:
                      inline_string: |
                        %s
                    private_key:
                      inline_string: |
                        %s
          filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: AUTO
                stat_prefix: ingress_http
                route_config:
                  name: local_route
                  virtual_hosts:
                    - name: vh_default
                      domains: ["*"]
%s
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router`, cert, key, routeTable)
}

func (d *h2Driver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	// STRICT_DNS + host.docker.internal (the 0066/0069/0074/0078 reference shape).
	mp := h2ClusterBlock(clusterMP, "STRICT_DNS", "host.docker.internal", backendPorts[0], maxConnectionsMP, maxPendingMP, streamCapMP)
	of := h2ClusterBlock(clusterOF, "STRICT_DNS", "host.docker.internal", backendPorts[0], maxConnectionsOF, maxPendingOF, streamCapOF)
	return fmt.Sprintf(`admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: %d }
static_resources:
  listeners:
    - name: l_h2
      address:
        socket_address: { address: 0.0.0.0, port_value: %d }
      filter_chains:
%s
  clusters:
%s%s`, refAdminPort, refContainerListenerPort, h2ListenerFilterChain(), mp, of)
}

func (d *h2Driver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0066/0069/0074/0078 subject shape).
	mp := h2ClusterBlock(clusterMP, "STATIC", "127.0.0.1", backendPorts[0], maxConnectionsMP, maxPendingMP, streamCapMP)
	of := h2ClusterBlock(clusterOF, "STATIC", "127.0.0.1", backendPorts[0], maxConnectionsOF, maxPendingOF, streamCapOF)
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0079, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_h2
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
%s
  clusters:
%s%s`, subjAdminPort, subjListenerPort, h2ListenerFilterChain(), mp, of)
}

// DriveReference stashes the reference listener addr and returns the fixed
// byte-equiv stream. The real work runs in AssertStats.
func (d *h2Driver) DriveReference(_ context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListener = addr
	d.mu.Unlock()
	return []byte("READY\n"), nil
}

// DriveSubject returns the fixed byte-equiv stream (the subject listener addr was
// already stashed in SubjectConfig). The real work runs in AssertStats.
func (d *h2Driver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return []byte("READY\n"), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0074/0078
// raw /ready probe, verbatim).
func (*h2Driver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref probe: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj probe: %w", err)
	}
	return refBytes, subjBytes, nil
}

// ensureCertPool builds d.rootCAs from the committed CA PEM on the first call.
func (d *h2Driver) ensureCertPool() *x509.CertPool {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rootCAs != nil {
		return d.rootCAs
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(readPEM("ca.pem"))) {
		panic("driver: failed to parse CA PEM from pki/ca.pem")
	}
	d.rootCAs = pool
	return d.rootCAs
}

// tlsConfig trusts the fixture-local CA and advertises NextProtos=["h2"] so the
// listener (offering ["h2","http/1.1"]) negotiates h2 via ALPN. ServerName
// "localhost" matches the listener cert's DNS SAN (the 0004 discipline).
func (d *h2Driver) tlsConfig() *tls.Config {
	return &tls.Config{
		RootCAs:    d.ensureCertPool(),
		NextProtos: []string{"h2"},
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
	}
}

// backendIdxFromBody parses the H2HoldResponder canned body "backend-<idx>:<seg>"
// and returns the embedded backend idx (host attribution).
func backendIdxFromBody(body []byte) (int, error) {
	s := string(body)
	const pfx = "backend-"
	if !strings.HasPrefix(s, pfx) {
		return 0, fmt.Errorf("body %q has no %q prefix", s, pfx)
	}
	rest := s[len(pfx):]
	colon := strings.IndexByte(rest, ':')
	if colon < 0 {
		return 0, fmt.Errorf("body %q has no ':' after idx", s)
	}
	idx, err := strconv.Atoi(rest[:colon])
	if err != nil {
		return 0, fmt.Errorf("body %q: bad idx: %w", s, err)
	}
	return idx, nil
}

// pollStats scrapes until ALL key==want pairs hold simultaneously (or deadline).
func pollStats(side, adminAddr string, want map[string]uint64) error {
	deadline := time.Now().Add(convergeDeadline)
	last := map[string]int64{}
	for {
		st, err := scrapeStats(adminAddr)
		if err == nil {
			ok := true
			for k, w := range want {
				v := st[k]
				last[k] = int64(v)
				if v != w {
					ok = false
				}
			}
			if ok {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: stats did not converge to %v within %s (last seen %v)",
				side, want, convergeDeadline, last)
		}
		time.Sleep(convergePoll)
	}
}

// heldResult captures one H2 GET outcome (status + body) for the post-release tally.
type heldResult struct {
	status int
	body   []byte
	err    error
}

// fireHeld launches n concurrent HTTP/2 GET requests against the given path
// prefix (each BLOCKS at the responder until a release). Each request gets a
// UNIQUE path "<prefix>/<base+i>" + a fresh downstream H2 ClientConn (the
// helpers.H2RoundTrip discipline). The upstream pool sees n concurrent STREAMS
// regardless of the downstream conn fan-out. Results land in res[base:base+n].
func (d *h2Driver) fireHeld(ctx context.Context, listenerAddr, prefix string, n, base int, res []heldResult, wg *sync.WaitGroup) {
	tlsConf := d.tlsConfig()
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(slot int) {
			defer wg.Done()
			path := fmt.Sprintf("%s/%d", prefix, slot)
			status, _, body, err := helpers.H2RoundTrip(ctx, listenerAddr, tlsConf, "GET", path, nil, nil)
			res[slot] = heldResult{status: status, body: body, err: err}
		}(base + i)
	}
}

// release hits the BACKEND control port /__release (NOT the proxy listener),
// re-armably freeing the current held batch. Always loopback (the backend is
// in-process on this machine for both sides). The backend speaks h2c; a plain
// HTTP/1.1 control request would not parse — so the release control request is
// an HTTP/2 (h2c prior-knowledge) GET via helpers.H2CRoundTrip.
func (d *h2Driver) release(t fixture.TB, side string, backendPort int) {
	t.Helper()
	releaseAddr := "127.0.0.1:" + strconv.Itoa(backendPort)
	status, _, _, err := helpers.H2CRoundTrip(context.Background(), releaseAddr, "GET", "/__release", nil, nil)
	if err != nil {
		t.Fatalf("%s: /__release: transport error to backend %s: %v", side, releaseAddr, err)
	}
	if status != 200 {
		t.Fatalf("%s: /__release: status %d, want 200", side, status)
	}
}

// assertSide runs BOTH prongs against one side (cross-side EXACT on both). The
// reference and the subject behave identically for the H2 conn/stream counts
// (D-H2-EXACT — clean local-cap-driven flow control).
func (d *h2Driver) assertSide(t fixture.TB, side, listenerAddr, adminAddr string, backendPort int) {
	t.Helper()
	ctx := context.Background()

	// ===== Prong 1: the EXACT ceil prong (c_h2mp) =====
	cxTotalMP := statKey(clusterMP, "upstream_cx_total")
	cxHTTP2TotalMP := statKey(clusterMP, "upstream_cx_http2_total")
	streamsActiveMP := statKey(clusterMP, "http2.streams_active")

	mpRes := make([]heldResult, heldK)
	var mpWG sync.WaitGroup
	d.fireHeld(ctx, listenerAddr, "/mp", heldK, 0, mpRes, &mpWG)

	if err := pollStats(side, adminAddr, map[string]uint64{
		cxTotalMP:       uint64(expectedConnsMP),
		cxHTTP2TotalMP:  uint64(expectedConnsMP),
		streamsActiveMP: uint64(heldK),
	}); err != nil {
		t.Fatalf("%s: ceil prong: %v (the %d held streams did not converge to ceil(%d/%d)=%d conns + %d active streams — is the LOCAL cap C=%d driving multi-conn growth? is the backend holding all streams?)",
			side, err, heldK, heldK, streamCapMP, expectedConnsMP, heldK, streamCapMP)
	}

	// Step 2: multiplex proof — ceil(K/C) conns must be << K (few conns, many streams).
	if expectedConnsMP >= heldK {
		t.Errorf("%s: multiplex proof vacuous: expectedConnsMP=%d not << heldK=%d", side, expectedConnsMP, heldK)
	}

	// Step 3: release → all K drain to 200 → poll streams_active back to 0.
	d.release(t, side, backendPort)
	mpWG.Wait()
	for i := 0; i < heldK; i++ {
		r := mpRes[i]
		if r.err != nil {
			t.Errorf("%s: ceil[%d]: transport error: %v", side, i, r.err)
			continue
		}
		if r.status != 200 {
			t.Errorf("%s: ceil[%d]: status %d, want 200 (held stream not served after drain)", side, i, r.status)
			continue
		}
		if idx, perr := backendIdxFromBody(r.body); perr != nil {
			t.Errorf("%s: ceil[%d]: %v", side, i, perr)
		} else if idx != 0 {
			t.Errorf("%s: ceil[%d]: backend idx %d, want 0", side, i, idx)
		}
	}
	if err := pollStats(side, adminAddr, map[string]uint64{streamsActiveMP: 0}); err != nil {
		t.Fatalf("%s: ceil prong did not drain: %v (http2.streams_active should return to 0 after release)", side, err)
	}

	// ===== Prong 2: the overflow prong (c_h2of: C=1, max_connections=1, max_pending=1) =====
	pendingActiveOF := statKey(clusterOF, "upstream_rq_pending_active")
	pendingOverflowOF := statKey(clusterOF, "upstream_rq_pending_overflow")
	streamsActiveOF := statKey(clusterOF, "http2.streams_active")
	cxTotalOF := statKey(clusterOF, "upstream_cx_total")

	beforeOF, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape overflow baseline: %v", side, err)
	}
	baseOverflow := beforeOF[pendingOverflowOF]

	// Step 4a: fire 1 held GET /of/0 → fills the 1 conn (C=1) → poll streams_active==1.
	ofRes := make([]heldResult, 2) // [0] the held filler, [1] the pending request
	var ofWG sync.WaitGroup
	d.fireHeld(ctx, listenerAddr, "/of", 1, 0, ofRes, &ofWG)
	if err := pollStats(side, adminAddr, map[string]uint64{streamsActiveOF: 1}); err != nil {
		t.Fatalf("%s: overflow prong fill: %v (the 1 held stream did not occupy the C=1 conn)", side, err)
	}

	// Step 4b: fire a 2nd held GET /of/1 → both the conn (C=1) and max_connections=1
	// are exhausted → it PENDS → poll upstream_rq_pending_active==1.
	d.fireHeld(ctx, listenerAddr, "/of", 1, 1, ofRes, &ofWG)
	if err := pollStats(side, adminAddr, map[string]uint64{pendingActiveOF: 1}); err != nil {
		t.Fatalf("%s: overflow prong pend: %v (the 2nd request did not PEND at depth 1)", side, err)
	}

	// Step 4c: fire a 3rd GET /of/2 SYNCHRONOUSLY → the pending queue is full
	// (max_pending_requests=1) → it OVERFLOWS → DOWNSTREAM 503 (returns immediately).
	// Assert the DOWNSTREAM status code (not upstream_rq_5xx).
	status, _, _, oerr := helpers.H2RoundTrip(ctx, listenerAddr, d.tlsConfig(), "GET", "/of/2", nil, nil)
	if oerr != nil {
		t.Errorf("%s: overflow oversub: transport error: %v (should be a 503 local reply, not a transport failure)", side, oerr)
	} else if status != 503 {
		t.Errorf("%s: overflow oversub: status %d, want 503 (the pending queue was full — should overflow)", side, status)
	}

	afterOF, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: scrape after oversub: %v", side, err)
	}
	if delta := afterOF[pendingOverflowOF] - baseOverflow; delta < 1 {
		t.Errorf("%s: %s delta = %d, want >= 1 (the bounded pending queue overflowed; after %d base %d)", side, pendingOverflowOF, delta, afterOF[pendingOverflowOF], baseOverflow)
	}
	if afterOF[cxTotalOF] == 0 {
		t.Fatalf("%s: overflow prong did NOT decode: %s == 0 (could not reach the backend over H2?)", side, cxTotalOF)
	}

	// Step 5: drain → the held filler AND the woken pending both reach 200, then
	// poll streams_active==0 AND pending_active==0 (gauges settle).
	//
	// The held filler is blocked at the backend on the CURRENT gate; the pending
	// request is queued at the PROXY (no upstream stream yet). A single
	// re-armable /__release frees ONLY the filler — the pending request is woken
	// onto the same conn and sent upstream AFTER the gate re-armed, so it would
	// re-block on the fresh gate. So drain with a RE-ARM LOOP: release repeatedly
	// (each frees whatever batch is currently held) until BOTH ofWG requests have
	// returned. A sticky release is NOT usable here — the backend is SHARED with
	// the not-yet-run side, and a permanently-open gate would let that side's
	// holds sail through. The loop's cadence is a control-plane re-arm, not an
	// assertion sleep (the gauge convergence is still polled, release-barrier).
	drained := make(chan struct{})
	go func() { ofWG.Wait(); close(drained) }()
	d.release(t, side, backendPort)
	for {
		select {
		case <-drained:
		case <-time.After(convergePoll):
			d.release(t, side, backendPort) // re-arm: free the woken pending's stream
			continue
		}
		break
	}
	for i := 0; i < 2; i++ {
		r := ofRes[i]
		if r.err != nil {
			t.Errorf("%s: overflow held/pending[%d]: transport error: %v", side, i, r.err)
			continue
		}
		if r.status != 200 {
			t.Errorf("%s: overflow held/pending[%d]: status %d, want 200 (in-budget request not served after drain)", side, i, r.status)
		}
	}
	if err := pollStats(side, adminAddr, map[string]uint64{
		streamsActiveOF: 0,
		pendingActiveOF: 0,
	}); err != nil {
		t.Fatalf("%s: overflow prong did not drain: %v (streams_active + pending_active should settle to 0)", side, err)
	}
}

// AssertStats runs the staged drive SEQUENTIALLY per side (subject FULLY, then
// reference). The shared in-process backend is idle between sides (each side's
// held + woken requests all drain before the next side fires), so there is no
// cross-side release interference. Both sides assert cross-side EXACT (D-H2-EXACT).
func (d *h2Driver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	d.mu.Lock()
	refListener := d.refListener
	subjListener := d.subjListener
	backendPort := d.backendPort
	d.mu.Unlock()
	if refListener == "" || subjListener == "" {
		t.Fatalf("listener addrs not stashed (ref=%q subj=%q) — Drive hooks did not run?", refListener, subjListener)
	}
	if backendPort == 0 {
		t.Fatalf("backend port not stashed — config builders did not run?")
	}

	d.assertSide(t, "subject", subjListener, subjAdminAddr, backendPort)
	d.assertSide(t, "reference", refListener, refAdminAddr, backendPort)
}

// scrapeStats issues GET http://<addr>/stats (the FLAT admin text) and parses
// "name: value" lines into a map[name]uint64. (The 0074/0078 driver scrapeStats.)
func scrapeStats(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	out := make(map[string]uint64)
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	for {
		nn, rerr := resp.Body.Read(tmp)
		if nn > 0 {
			buf = append(buf, tmp[:nn]...)
		}
		if rerr != nil {
			break
		}
	}
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ": ")
		if idx < 0 {
			continue
		}
		name := line[:idx]
		valStr := strings.TrimSpace(line[idx+2:])
		v, err := strconv.ParseUint(valStr, 10, 64)
		if err != nil {
			continue // skip non-numeric (histograms, special formats)
		}
		out[name] = v
	}
	return out, nil
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*h2Driver)(nil)
	_ fixture.StatsAsserter    = (*h2Driver)(nil)
	_ fixture.BackendKindAware = (*h2Driver)(nil)
)
