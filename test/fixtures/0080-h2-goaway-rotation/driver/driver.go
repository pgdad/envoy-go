// Package driver registers the 0080-h2-goaway-rotation cross-side differential
// fixture (phase 43.2b SPEC §8.1 / PLAN Task 8).
//
// This is a CROSS-SIDE [http_connection_manager + router] fixture over an
// HTTP/2 (TLS+ALPN-h2) DOWNSTREAM listener whose requests fan out to a single
// h2c upstream H2 cluster c_h2gw. It proves the GRACEFUL GOAWAY-driven upstream
// H2 connection ROTATION: a GOAWAY'd pooled conn is admission-skipped (takes NO
// new streams), drains its in-flight stream, then closes — its replacement is a
// fresh dial (a SECOND conn; upstream_cx_http2_total → 2). Plus the two reset
// prongs: a backend RST_STREAM bumps cluster.<name>.http2.rx_reset (the conn
// survives), and a DOWNSTREAM cancel makes the upstream codec emit a CANCEL,
// bumping cluster.<name>.http2.tx_reset.
//
// # WHY an HTTP/2 DOWNSTREAM listener (reference_h2_pool_downstream_codec_gate)
//
// The HCM selects the H1-vs-H2 router action by the DOWNSTREAM listener codec
// (internal/filter/hcm/filter.go — HttpConnectionManager_HTTP2). An H1-downstream
// listener always drives the H1 upstream dial path, NEVER AcquireH2Stream — even
// when the cluster's UseH2()==true. The H2 multiplex pool (router_h2.go
// doH2ClusterAction → AcquireH2Stream) — and therefore the GOAWAY drain
// lifecycle this fixture exercises — is reached ONLY from an H2 downstream
// listener. So the 0080 listener MUST be the H2 (TLS+ALPN-h2) downstream shape
// that 0079-h2-multiplex-pool / 0004-h2-routing use (the 0004 PKI); an
// H1-downstream variant would SILENTLY never engage the pool. The decode-ran
// guard (upstream_cx_http2_total > 0) catches an accidentally-disengaged pool.
//
// # Cross-side EXACT (reference_h2_goaway_rotation_stats)
//
// The reference Envoy observes upstream H2 GOAWAY rotation via the upstream_cx_*
// LIFECYCLE counters, NOT the http2.* family: there is no goaway_received
// counter, goaway_sent stays 0 on a peer-driven drain-close, and only rx_reset +
// tx_reset of the http2.* family are live. So 0080 asserts cross-side EXACT:
//
//	cluster.c_h2gw.upstream_cx_http2_total == 2   (one rotation: orig + replacement)
//	cluster.c_h2gw.http2.streams_active    == 0   (at quiesce)
//	cluster.c_h2gw.http2.rx_reset          == 1   (one backend RST_STREAM)
//	cluster.c_h2gw.http2.tx_reset          == 1   (one downstream-cancel CANCEL)
//
// Per D-H2B-CXSTATS only the counters BOTH sides emit are asserted —
// upstream_cx_http2_total, upstream_cx_active, http2.streams_active,
// http2.rx_reset, http2.tx_reset. cx_close_notify / cx_destroy_local are NOT
// asserted (envoy-go does not emit them).
//
// # Topology: 1 H2GoawayResponder (runner-spawned, h2c, raw framer)
//
// The single backend (BackendKind 38) is a RAW-FRAMER h2c responder advertising
// SETTINGS_MAX_CONCURRENT_STREAMS=1000 >> C that HOLDS each normal request stream
// open and, on control requests recognized by :path, drives the framer:
//   - /__release — answer 200 to ALL currently-held streams (re-armable).
//   - /__goaway  — emit a peer GOAWAY(NO_ERROR) naming the highest stream id seen
//     (so NO in-flight stream is abandoned), conn STAYS open (graceful drain).
//   - /__rst     — emit a targeted RST_STREAM(INTERNAL_ERROR) on a held stream.
//
// The backend's held set + GOAWAY/RST act PER-CONN (each accepted conn has its own
// h2GoawayConn). So — unlike 0079's process-global gate — EVERY control request
// here is routed THROUGH THE PROXY (H2RoundTrip over the H2 downstream listener),
// not direct to the backend: only then does it multiplex onto the proxy's pooled
// upstream conn, so the GOAWAY/RST lands on the SAME conn that holds the in-flight
// streams. With C high (100) a single in-flight stream + the control request share
// one upstream conn — the rotation is about conn IDENTITY, not the local stream cap.
//
// # The staged drive (per side, inside AssertStats; SLEEPLESS, sequential-per-side)
//
// The runner's hooks are DriveReference/DriveSubject (the byte-equiv "READY\n"
// stream, run FIRST) then AssertStats (run LAST, holding BOTH admin addrs). The
// six-stage drive runs inside AssertStats, SUBJECT fully then REFERENCE. The Drive
// hooks stash listener addrs; the config builders stash the backend host port.
//
//	| Step | BOTH sides (cross-side EXACT)                                          |
//	|------|------------------------------------------------------------------------|
//	| 1    | Establish: fire 1 held GET / → poll upstream_cx_http2_total==1 AND     |
//	|      | http2.streams_active==1 (the pool engaged: decode-ran guard).          |
//	| 2    | Drain (in-flight): /__goaway through the proxy → the conn is admission- |
//	|      | skipped; a 2nd held GET / MISSes the drained conn → REPLACEMENT dial    |
//	|      | (upstream_cx_http2_total==2); /__release → both drain → poll            |
//	|      | streams_active==0 → the drained conn closes (upstream_cx_active==1).    |
//	| 3    | Drain (idle): fire+release 1 GET / (conn idle), /__goaway → the watcher |
//	|      | closes it PROMPTLY → poll upstream_cx_active==0 (no release drove it).  |
//	| 4    | rx_reset: fire 1 held GET /, /__rst on it → poll http2.rx_reset==1 +    |
//	|      | a DOWNSTREAM 5xx (assert the downstream class); the conn survives.      |
//	| 5    | tx_reset: fire 1 held GET / then CANCEL it downstream → the upstream     |
//	|      | codec emits CANCEL → poll http2.tx_reset==1.                            |
//	| 6    | Quiesce: /__release → all held drain → poll http2.streams_active==0 AND |
//	|      | upstream_cx_active==0.                                                  |
//
// On the rx_reset prong the DOWNSTREAM response class is asserted (the held GET's
// observed status is 5xx), NOT the upstream class (envoy-go's losers can
// over-count upstream_rq_* — reference_concurrent_attempt_downstream_class_assertion).
// All polls use convergeDeadline — NEVER a fixed sleep
// (reference_concurrency_differential_release_barrier).
//
// # Cross-references
//
//   - phase 43.2b SPEC §8.1 / PLAN Task 8 (the fixture design).
//   - reference_h2_goaway_rotation_stats (observe rotation via upstream_cx_*; only rx/tx_reset live).
//   - reference_h2_pool_downstream_codec_gate (the H2 downstream listener gate).
//   - 0079-h2-multiplex-pool (the config-as-Go-string mechanism + the H2 PKI shape).
//   - 0004-h2-routing (the H2 TLS downstream listener + PKI + helpers.H2RoundTrip).
//   - reference_concurrency_differential_release_barrier (sleepless poll-to-converge).
//   - reference_concurrent_attempt_downstream_class_assertion (downstream class on the rx prong).
//   - reference_docker_probe_bridge_network (shared bridge + STRICT_DNS hostname).
//   - reference_differential_run_selector (target -run 'TestDifferential/0080').
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

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0080-h2-goaway-rotation"

	// cluster is the single h2c upstream cluster (the GOAWAY-rotation prong target).
	cluster = "c_h2gw"

	// In-container reference Envoy listener port. Fixtures run sequentially;
	// next-free is 19169 (0079 took 19168).
	refContainerListenerPort = 19169

	refAdminPort = 9901

	// backendCount is the number of runner-spawned H2GoawayResponder hosts (1).
	backendCount = 1

	// streamCap (C) is c_h2gw's http2_protocol_options.max_concurrent_streams. It
	// is set HIGH (100) so a single held stream + the control request share ONE
	// upstream conn: the rotation is about conn IDENTITY (the GOAWAY drives the
	// multi-conn count), NOT the local stream cap. C never binds in this fixture.
	streamCap = 100

	// Convergence poll budget (NO fixed sleep — poll until the predicate holds).
	convergeDeadline = 15 * time.Second
	convergePoll     = 50 * time.Millisecond
)

// statKey builds a cluster-scoped stat name "cluster.<cluster>.<suffix>".
func statKey(suffix string) string { return "cluster." + cluster + "." + suffix }

func init() {
	fixture.RegisterFixture(fixtureName, &h2Driver{})
}

// h2Driver is STATEFUL: the Drive hooks stash the per-side listener addrs and the
// config builders stash the backend port, so AssertStats — the only hook holding
// BOTH admin addrs — can run the six-stage drive (establish / drain-in-flight /
// drain-idle / rx_reset / tx_reset / quiesce). Control requests (/__release,
// /__goaway, /__rst) are routed THROUGH THE PROXY so they land on the pooled
// upstream conn that holds the in-flight streams.
type h2Driver struct {
	mu           sync.Mutex
	refListener  string // host:port of the reference l_h2 listener (from DriveReference)
	subjListener string // 127.0.0.1:<port> of the subject l_h2 listener (from SubjectConfig)
	backendPort  int    // the single backend's host port (stashed; the proxy dials it)
	rootCAs      *x509.CertPool
}

func (*h2Driver) BackendCount() int                { return backendCount }
func (*h2Driver) BackendKind() fixture.BackendKind { return fixture.H2GoawayResponder }
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

// h2ClusterBlock renders the single H2 (h2c) upstream cluster with the given
// endpoint type/address and the H2 typed_extension_protocol_options carrying
// max_concurrent_streams: C. h2c (no transport_socket) per ADR-0166; the backend
// advertises SETTINGS=1000 >> C so the LOCAL cap binds. No circuit_breakers block
// (C never binds; the rotation is conn-identity-driven, not budget-driven).
func h2ClusterBlock(clusterType, endpointAddr string, endpointPort int) string {
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
      load_assignment:
        cluster_name: %s
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: %s, port_value: %d } } }
`, cluster, clusterType, streamCap, cluster, endpointAddr, endpointPort)
}

// routeTable routes /tx → c_h2gw WITH a per_try_timeout retry_policy (the
// tx_reset prong: a held /tx request times out per-try → the router cancels the
// upstream H2 stream → the codec emits RST_STREAM(CANCEL) → http2.tx_reset++),
// and everything else (incl. the control paths /__release, /__goaway, /__rst) to
// c_h2gw with NO timeout. Both routes target the SAME cluster so tx_reset accrues
// on c_h2gw. num_retries:0 so the per-try-timeout does not loop. Identical on both
// sides.
//
// WHY per_try_timeout drives tx_reset (NOT a raw downstream cancel): envoy-go's
// H2 downstream codec dispatches each stream on the CONNECTION-level ctx (no
// per-stream cancelable ctx; internal/filter/hcm/h2/conn.go), so a downstream
// stream RST / conn-close does NOT propagate to cancel the in-flight UPSTREAM
// RoundTrip ctx — the codec's CANCEL site (client.go RoundTrip ctx.Done) never
// fires from a downstream cancel alone. A per_try_timeout is the faithful trigger
// that DOES cancel the upstream attempt ctx (retry.go retryExecutorH2's
// attemptCtx WithTimeout), so the codec emits the CANCEL the SPEC's tx_reset
// prong observes. The reference emits the same upstream RST_STREAM(CANCEL) on a
// per-try-timeout, so http2.tx_reset == 1 stays cross-side EXACT.
const routeTable = `                      routes:
                        - match: { prefix: "/tx" }
                          route:
                            cluster: c_h2gw
                            retry_policy:
                              num_retries: 0
                              per_try_timeout: 0.5s
                        - match: { prefix: "/" }
                          route: { cluster: c_h2gw }`

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
	// STRICT_DNS + host.docker.internal (the 0079 reference shape; shared bridge).
	c := h2ClusterBlock("STRICT_DNS", "host.docker.internal", backendPorts[0])
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
%s`, refAdminPort, refContainerListenerPort, h2ListenerFilterChain(), c)
}

func (d *h2Driver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != backendCount {
		panic(fmt.Sprintf("%s: expected %d backend ports, got %d", fixtureName, backendCount, len(backendPorts)))
	}
	d.stashBackendPort(backendPorts)
	d.mu.Lock()
	d.subjListener = fmt.Sprintf("127.0.0.1:%d", subjListenerPort)
	d.mu.Unlock()
	// STATIC + 127.0.0.1 (the 0079 subject shape).
	c := h2ClusterBlock("STATIC", "127.0.0.1", backendPorts[0])
	return fmt.Sprintf(`
node: { id: envoy-go-subject-0080, cluster: envoy-go-differential }
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
%s`, subjAdminPort, subjListenerPort, h2ListenerFilterChain(), c)
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

// ProbeAdmin issues GET /ready against each proxy's admin endpoint (the 0079 raw
// /ready probe, verbatim).
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

// heldResult captures one H2 GET outcome (status + error) for a held/control request.
type heldResult struct {
	status int
	err    error
}

// fireHeld launches ONE HTTP/2 GET request against the given path through the
// proxy listener (it BLOCKS at the responder until a release / RST / cancel). A
// fresh downstream H2 ClientConn is used per call (helpers.H2RoundTrip
// discipline); the upstream pool multiplexes the resulting stream onto a pooled
// upstream conn. The result lands in *res when the request returns. Returns a
// cancel func (for the tx_reset prong's downstream cancel) and the goroutine's
// done channel.
func (d *h2Driver) fireHeld(parent context.Context, listenerAddr, path string, res *heldResult) (context.CancelFunc, <-chan struct{}) {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		status, _, _, err := helpers.H2RoundTrip(ctx, listenerAddr, d.tlsConfig(), "GET", path, nil, nil)
		*res = heldResult{status: status, err: err}
	}()
	return cancel, done
}

// control issues ONE control request (/__release, /__goaway, /__rst) THROUGH THE
// PROXY so it multiplexes onto the pooled upstream conn that holds the in-flight
// streams (the backend's held set + GOAWAY/RST are PER-CONN). It returns 200 (the
// backend answers the control stream immediately) — a non-200 / transport error
// is a hard failure.
func (d *h2Driver) control(t fixture.TB, side, listenerAddr, path string) {
	t.Helper()
	status, _, _, err := helpers.H2RoundTrip(context.Background(), listenerAddr, d.tlsConfig(), "GET", path, nil, nil)
	if err != nil {
		t.Fatalf("%s: control %s: transport error through proxy %s: %v", side, path, listenerAddr, err)
	}
	if status != 200 {
		t.Fatalf("%s: control %s: status %d, want 200", side, path, status)
	}
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

// assertSide runs the six-stage GOAWAY-rotation drive against one side (cross-side
// EXACT on both — the reference and the subject agree on the rotation conn count +
// the reset counters).
func (d *h2Driver) assertSide(t fixture.TB, side, listenerAddr, adminAddr string, backendPort int) {
	t.Helper()
	_ = backendPort // the proxy dials the backend; control goes through the proxy.
	ctx := context.Background()

	cxHTTP2Total := statKey("upstream_cx_http2_total")
	cxActive := statKey("upstream_cx_active")
	streamsActive := statKey("http2.streams_active")
	rxReset := statKey("http2.rx_reset")
	txReset := statKey("http2.tx_reset")

	// ===== Step 1: Establish =====
	// Fire 1 held GET / → poll the pool engaged: ONE H2 conn + ONE active stream.
	var r1 heldResult
	_, done1 := d.fireHeld(ctx, listenerAddr, "/h1", &r1)
	if err := pollStats(side, adminAddr, map[string]uint64{
		cxHTTP2Total:  1,
		streamsActive: 1,
	}); err != nil {
		t.Fatalf("%s: step 1 establish: %v (the H2 upstream pool did not engage — is the H2 downstream listener gate active? did the backend hold the stream?)", side, err)
	}

	// ===== Step 2: Drain (in-flight) =====
	// Trigger the backend GOAWAY through the proxy (it multiplexes onto the conn
	// holding the in-flight stream r1). The conn is admission-skipped: a 2nd held
	// GET MISSes it → a REPLACEMENT dial (upstream_cx_http2_total → 2).
	d.control(t, side, listenerAddr, "/__goaway")
	var r2 heldResult
	_, done2 := d.fireHeld(ctx, listenerAddr, "/h2", &r2)
	if err := pollStats(side, adminAddr, map[string]uint64{
		cxHTTP2Total:  2,
		streamsActive: 2,
	}); err != nil {
		t.Fatalf("%s: step 2 drain-in-flight: %v (the GOAWAY'd conn should take NO new stream — the 2nd request should MISS it and dial a REPLACEMENT, upstream_cx_http2_total→2)", side, err)
	}
	// The headline cross-side EXACT rotation value: ONE GOAWAY rotation opened
	// EXACTLY a second conn (orig + replacement). Captured here (absolute, from a
	// 0 baseline) — the later prongs (idle-drain, rx, tx) accumulate further dials,
	// so the END total is NOT re-asserted as 2.
	if rot, serr := scrapeStats(adminAddr); serr != nil {
		t.Fatalf("%s: step 2 rotation scrape: %v", side, serr)
	} else if rot[cxHTTP2Total] != 2 {
		t.Errorf("%s: %s = %d, want 2 (exactly one GOAWAY rotation: orig + replacement)", side, cxHTTP2Total, rot[cxHTTP2Total])
	}
	// Release both held streams; the drained (GOAWAY'd) conn closes once its
	// in-flight stream releases (last-release eager-close), so upstream_cx_active
	// settles to 1 (the replacement survives).
	d.control(t, side, listenerAddr, "/__release")
	<-done1
	<-done2
	if r1.err != nil || r1.status != 200 {
		t.Errorf("%s: step 2: held r1 status=%d err=%v, want 200/nil (the in-flight stream must drain to 200 on the GOAWAY'd conn)", side, r1.status, r1.err)
	}
	if r2.err != nil || r2.status != 200 {
		t.Errorf("%s: step 2: held r2 status=%d err=%v, want 200/nil (the replacement-conn stream must drain to 200)", side, r2.status, r2.err)
	}
	if err := pollStats(side, adminAddr, map[string]uint64{
		streamsActive: 0,
		cxActive:      1,
	}); err != nil {
		t.Fatalf("%s: step 2 drain settle: %v (after release the drained conn closes; the replacement survives → upstream_cx_active==1)", side, err)
	}

	// ===== Step 3: Drain (idle) =====
	// Establish a conn via a fired+released request (it goes idle), then GOAWAY it:
	// the per-conn drain-watcher closes it PROMPTLY (no in-flight release drives
	// it) → upstream_cx_active drops to 0.
	var r3 heldResult
	_, done3 := d.fireHeld(ctx, listenerAddr, "/h3", &r3)
	if err := pollStats(side, adminAddr, map[string]uint64{streamsActive: 1}); err != nil {
		t.Fatalf("%s: step 3 establish-idle: %v (the held stream did not register)", side, err)
	}
	d.control(t, side, listenerAddr, "/__release")
	<-done3
	if r3.err != nil || r3.status != 200 {
		t.Errorf("%s: step 3: held r3 status=%d err=%v, want 200/nil", side, r3.status, r3.err)
	}
	// The conn is now idle (1 conn, 0 streams). GOAWAY it through the proxy (the
	// control request itself is a transient stream on that same idle conn).
	if err := pollStats(side, adminAddr, map[string]uint64{streamsActive: 0, cxActive: 1}); err != nil {
		t.Fatalf("%s: step 3 idle-precondition: %v (expected 1 idle conn, 0 active streams before the GOAWAY)", side, err)
	}
	d.control(t, side, listenerAddr, "/__goaway")
	if err := pollStats(side, adminAddr, map[string]uint64{cxActive: 0}); err != nil {
		t.Fatalf("%s: step 3 drain-idle: %v (the idle GOAWAY'd conn must be closed PROMPTLY by the per-conn watcher → upstream_cx_active==0)", side, err)
	}

	// ===== Step 4: rx_reset prong =====
	// Fire 1 held GET / (a fresh conn dials), trigger a backend RST_STREAM on it →
	// the upstream codec RECEIVES the RST → http2.rx_reset++; the DOWNSTREAM request
	// observes a 502 local reply (a RoundTrip protocol error → bad-gateway). The
	// backend's TCP conn stays open (it RST'd one stream, not the conn); envoy-go
	// evicts the poisoned pooled conn (EvictH2ConnOnError) — an internal pool
	// detail, not asserted cross-side.
	var r4 heldResult
	_, done4 := d.fireHeld(ctx, listenerAddr, "/h4", &r4)
	if err := pollStats(side, adminAddr, map[string]uint64{streamsActive: 1}); err != nil {
		t.Fatalf("%s: step 4 establish: %v (the rx_reset prong's held stream did not register)", side, err)
	}
	d.control(t, side, listenerAddr, "/__rst")
	<-done4
	if err := pollStats(side, adminAddr, map[string]uint64{rxReset: 1}); err != nil {
		t.Fatalf("%s: step 4 rx_reset: %v (a backend RST_STREAM should bump cluster.%s.http2.rx_reset to 1)", side, err, cluster)
	}
	// Assert the DOWNSTREAM response class (NOT upstream_rq_5xx): the RST'd stream
	// surfaces a 5xx (502/503) to the downstream client, or a transport error
	// (the downstream H2 stream is reset). Either is a non-200 downstream outcome.
	if r4.err == nil && r4.status >= 200 && r4.status < 500 {
		t.Errorf("%s: step 4 rx_reset: downstream status=%d err=%v, want a 5xx or a reset error (the upstream RST_STREAM must surface a downstream failure)", side, r4.status, r4.err)
	}

	// ===== Step 5: tx_reset prong =====
	// Fire 1 held GET /tx/ (the per_try_timeout route). The backend holds it; the
	// route's per_try_timeout (0.5s, num_retries:0) fires → the router cancels the
	// upstream H2 attempt ctx → the codec emits RST_STREAM(CANCEL) on the upstream
	// stream → http2.tx_reset++ and a synthesized 504 downstream. (See routeTable:
	// envoy-go has no downstream-cancel→upstream-cancel propagation, so the
	// per_try_timeout is the faithful tx_reset trigger; cross-side EXACT.)
	var r5 heldResult
	_, done5 := d.fireHeld(ctx, listenerAddr, "/tx/5", &r5)
	if err := pollStats(side, adminAddr, map[string]uint64{txReset: 1}); err != nil {
		t.Fatalf("%s: step 5 tx_reset: %v (the per_try_timeout should make the upstream codec emit a CANCEL → cluster.%s.http2.tx_reset==1)", side, err, cluster)
	}
	<-done5
	// The per-try-timeout synthesizes a 504 downstream (assert the DOWNSTREAM class,
	// not the upstream class). A transport reset of the downstream stream is also
	// acceptable (non-200 downstream outcome).
	if r5.err == nil && r5.status != 504 && (r5.status >= 200 && r5.status < 500) {
		t.Errorf("%s: step 5 tx_reset: downstream status=%d err=%v, want 504 (per_try_timeout) or a reset error", side, r5.status, r5.err)
	}

	// ===== Step 6: Quiesce =====
	// Release any remaining held streams; poll the stream gauge back to 0. The
	// /__release control request itself rides a freshly-dialed conn that stays
	// pooled+idle afterward (no idle-timeout configured), so upstream_cx_active
	// settles to 1 (one idle conn), NOT 0 — the cx_active==0 close was already
	// observed at the idle-drain prong (Step 3). streams_active==0 is the
	// deterministic quiesce gauge asserted here (cross-side EXACT).
	d.control(t, side, listenerAddr, "/__release")
	if err := pollStats(side, adminAddr, map[string]uint64{
		streamsActive: 0,
	}); err != nil {
		t.Fatalf("%s: step 6 quiesce: %v (all held streams should drain → http2.streams_active==0)", side, err)
	}

	// Final cross-side EXACT assertions (the reset counters; the rotation total
	// was pinned at the Step-2 boundary above).
	final, err := scrapeStats(adminAddr)
	if err != nil {
		t.Fatalf("%s: final scrape: %v", side, err)
	}
	if final[rxReset] != 1 {
		t.Errorf("%s: %s = %d, want 1 (one backend RST_STREAM)", side, rxReset, final[rxReset])
	}
	if final[txReset] != 1 {
		t.Errorf("%s: %s = %d, want 1 (one downstream-cancel CANCEL)", side, txReset, final[txReset])
	}
}

// AssertStats runs the six-stage GOAWAY-rotation drive SEQUENTIALLY per side
// (subject FULLY, then reference). The shared in-process backend is idle between
// sides (each side's held requests all drain before the next side fires), so there
// is no cross-side interference. Both sides assert cross-side EXACT.
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
// "name: value" lines into a map[name]uint64. (The 0079 driver scrapeStats.)
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
