// Package driver registers the 0124-listener-sni-longest-suffix fixture with
// the differential runner. See ../README.md for the fixture's purpose.
//
// THE PROPOSITION (phase 99, SPEC §7): when two or more filter chains'
// server_names patterns match one SNI, reference Envoy serves the chain whose
// MATCHING pattern is most specific — an exact name first, then the LONGEST
// matching "*." suffix — independent of declaration order and independent of
// any other, non-matching pattern the chain also lists. envoy-go ranked each
// chain's WHOLE pattern set (exact > suffix > universal) and, on a tie between
// two wildcard chains, CLOSED the connection; on a mixed set it served the
// chain that merely LISTED an exact name.
//
// Four TLS listeners, each with `listener_filters: [tls_inspector]`, every
// chain an HCM with a DISTINCT stat_prefix and a direct_response whose body
// names the listener and the chain:
//
//	listener       chains (declared order)                      SNI -> chain
//	l_long_first   LONG [*.b.foo.test], SHORT [*.foo.test]       a.b.foo.test -> LONG, x.foo.test -> SHORT
//	l_short_first  SHORT, LONG                                   a.b.foo.test -> LONG, x.foo.test -> SHORT
//	l_mixed        X [x.test, *.foo.test], Y [*.b.foo.test]      a.b.foo.test -> Y, q.foo.test -> X, x.test -> X
//	l_default      LONG, SHORT + default_filter_chain DEFAULT    a.b.foo.test -> LONG, nomatch.example -> DEFAULT
//
// ⚠️ l_long_first / l_short_first are the REVERSAL PAIR: identical but for
// declaration order, so a subject answering by order is red on exactly one.
//
// ⚠️ l_mixed a.b.foo.test is the ONLY row that tells "rank of the MATCHED
// pattern" (the reference, P2) from "rank of the chain's whole set + matched
// length" (P1): X's exact x.test does not match a.b.foo.test and must not help
// X. Deleting l_mixed leaves the fixture unable to see P1.
//
// ⚠️ The single-candidate rows (x.foo.test, q.foo.test, x.test,
// nomatch.example) are STRUCTURALLY GREEN at the un-fixed tip: only one chain
// is eligible, so no tie-break runs. They prove each losing chain is live and
// reachable; they are NOT evidence of precedence.
//
// ⚠️ NO ROW MAY BE A NO-MATCH CLOSE. nomatch.example reaches l_default's
// default chain. The reference books a TCP no-match close in
// no_filter_chain_match, a name envoy-go does not emit, so a close row could
// only be pinned vacuously.
//
// ⚠️ tls_inspector IS LOAD-BEARING. Without it neither side reads the SNI
// before chain selection and every server_names chain is ineligible.
package driver

import (
	"bufio"
	"bytes"
	"context"
	stdtls "crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
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

const fixtureName = "0124-listener-sni-longest-suffix"

// refAdminPort is the harness-fixed in-container reference admin port.
const refAdminPort = 9901

// wantStatus is the direct_response status of every chain. NOT a 1xx (the Go
// client consumes a 1xx without surfacing it as the final status).
const wantStatus = 200

// armDeadline bounds one probe's dial + handshake + request + response.
const armDeadline = 5 * time.Second

// closedBody is what a probe records when the connection did not produce an
// HTTP response (handshake reset, EOF, timeout). It is a fixed token, NOT the
// error text: the two sides word a close differently, and the error text would
// make CompareBytes diverge on wording rather than on behavior.
const closedBody = "<CLOSED>"

// chain is one filter chain. isDefault marks the listener's
// default_filter_chain (no filter_chain_match, no server_names).
type chain struct {
	id          string   // short name used in failure text: LONG, SHORT, X, Y, DEFAULT
	prefix      string   // HCM stat_prefix — DISTINCT across the whole fixture
	serverNames []string // filter_chain_match.server_names
	isDefault   bool
}

// probe is one (SNI -> expected chain) row.
type probe struct {
	sni  string // lowercase only: case folding is out of scope
	want string // chain id
}

// listener is one listener under test. The roster order is the index-wise zip
// the runner performs between SubjectListenerNames() and
// ReferenceListenerPorts(); do not permute one accessor without the other.
type listener struct {
	name string

	// refPort is the IN-CONTAINER reference port. The runner publishes each
	// and hands back the Docker-assigned host mapping per listener.
	//
	// CENSUSED at this tip: 15124 is the `15000 + <index>` convention slot
	// (reserved for this fixture in 0123's prose); 15225-15227 read zero
	// files under `git grep -lw -- <port> -- test/ internal/ cmd/`.
	refPort int

	chains []chain
	probes []probe
}

func body(l, c string) string { return l + "/" + c + "\n" }

var (
	long = func(p string) chain { return chain{id: "LONG", prefix: p, serverNames: []string{"*.b.foo.test"}} }
	shrt = func(p string) chain { return chain{id: "SHORT", prefix: p, serverNames: []string{"*.foo.test"}} }
)

// listeners is the roster. 🔴 ALL NINE stat_prefixes MUST BE DISTINCT: two
// HCMs sharing one prefix panic envoy-go at boot with a duplicate metric
// registration.
var listeners = []listener{
	{
		name:    "l_long_first",
		refPort: 15124,
		chains:  []chain{long("lf_long"), shrt("lf_short")},
		probes:  []probe{{"a.b.foo.test", "LONG"}, {"x.foo.test", "SHORT"}},
	},
	{
		name:    "l_short_first",
		refPort: 15225,
		chains:  []chain{shrt("sf_short"), long("sf_long")},
		probes:  []probe{{"a.b.foo.test", "LONG"}, {"x.foo.test", "SHORT"}},
	},
	{
		name:    "l_mixed",
		refPort: 15226,
		chains: []chain{
			{id: "X", prefix: "mx_x", serverNames: []string{"x.test", "*.foo.test"}},
			{id: "Y", prefix: "mx_y", serverNames: []string{"*.b.foo.test"}},
		},
		probes: []probe{{"a.b.foo.test", "Y"}, {"q.foo.test", "X"}, {"x.test", "X"}},
	},
	{
		name:    "l_default",
		refPort: 15227,
		chains:  []chain{long("df_long"), shrt("df_short"), {id: "DEFAULT", prefix: "df_default", isDefault: true}},
		probes:  []probe{{"a.b.foo.test", "LONG"}, {"nomatch.example", "DEFAULT"}},
	},
}

func init() { fixture.RegisterFixture(fixtureName, &sniDriver{}) }

// sniDriver is STATEFUL: the Drive methods record each side's per-row
// observation and AssertStats asserts every row with one Errorf per property.
// A Drive that returned an error on a wrong or closed row would become
// t.Fatalf in the runner and MASK every later row's verdict.
type sniDriver struct {
	mu  sync.Mutex
	obs map[string]map[string]observation // side -> "listener sni" -> observation
}

type observation struct {
	status int
	body   string
	err    string
}

var (
	_ fixture.Driver              = (*sniDriver)(nil)
	_ fixture.MultiListenerDriver = (*sniDriver)(nil)
	_ fixture.StatsAsserter       = (*sniDriver)(nil)
)

// --- fixture.Driver ---

// BackendCount is 1: the runner rejects 0, and envoy-go boot-rejects an absent
// static_resources.clusters key, so both bootstraps carry a never-dialed
// c_unused cluster pointed at this port.
func (*sniDriver) BackendCount() int { return 1 }

// SubjectListenerName / ReferenceListenerPort return listener[0]; the runner
// computes the primary addresses before its multi-listener branch.
func (*sniDriver) SubjectListenerName() string { return listeners[0].name }
func (*sniDriver) ReferenceListenerPort() int  { return listeners[0].refPort }

func (*sniDriver) ReferenceBootstrap(backendPorts []int) string {
	return renderBootstrap("0.0.0.0", refAdminPort, func(i int) int { return listeners[i].refPort }, backendPorts[0])
}

// SubjectConfig: the subject listener ports are subjListenerPort + i. The
// runner allocates the base via freeTCPPortBlock, which probes [base, base+16)
// bindable; four of sixteen is inside the reservation (the 0123 derivation).
func (*sniDriver) SubjectConfig(_ int, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	return renderBootstrap("127.0.0.1", subjAdminPort, func(i int) int { return subjListenerPort + i }, backendPorts[0])
}

// DriveReference / DriveSubject drive listener[0] only. UNREACHABLE while
// MultiListenerDriver is implemented.
func (d *sniDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveAll(ctx, "ref", map[string]string{listeners[0].name: addr}, listeners[:1])
}

func (d *sniDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveAll(ctx, "subj", map[string]string{listeners[0].name: addr}, listeners[:1])
}

func (*sniDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.MultiListenerDriver ---

func (*sniDriver) SubjectListenerNames() []string {
	out := make([]string, len(listeners))
	for i, l := range listeners {
		out[i] = l.name
	}
	return out
}

func (*sniDriver) ReferenceListenerPorts() []int {
	out := make([]int, len(listeners))
	for i, l := range listeners {
		out[i] = l.refPort
	}
	return out
}

func (d *sniDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveAll(ctx, "ref", addrs, listeners)
}

func (d *sniDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveAll(ctx, "subj", addrs, listeners)
}

// driveAll issues ONE TLS round trip per probe row, each on a fresh
// connection with an explicit ServerName, and emits a side-label-free byte
// stream for CompareBytes. Only a missing address returns an error; a closed
// or wrong-chain row is RECORDED and asserted in AssertStats.
func (d *sniDriver) driveAll(ctx context.Context, side string, addrs map[string]string, roster []listener) ([]byte, error) {
	pool, err := serverCAPool()
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	seen := map[string]observation{}
	for _, l := range roster {
		addr := addrs[l.name]
		if addr == "" {
			return nil, fmt.Errorf("%s: no address supplied for listener %q (have %d entries)", side, l.name, len(addrs))
		}
		for _, p := range l.probes {
			o := tlsGet(ctx, pool, addr, p.sni)
			log.Printf("%s: %s %s sni=%s status=%d body=%q err=%q", fixtureName, side, l.name, p.sni, o.status, o.body, o.err)
			fmt.Fprintf(&b, "listener %s sni %s status=%d body=%q\n", l.name, p.sni, o.status, o.body)
			seen[l.name+" "+p.sni] = o
		}
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.obs == nil {
		d.obs = map[string]map[string]observation{}
	}
	d.obs[side] = seen
	return b.Bytes(), nil
}

func (d *sniDriver) recorded(side string) map[string]observation {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.obs[side]
}

// tlsGet is one row: fresh TCP connection, TLS handshake with ServerName =
// sni verified against the committed CA, one HTTP/1.1 GET, Connection: close.
// Any failure records body = closedBody and the error text.
func tlsGet(ctx context.Context, pool *x509.CertPool, addr, sni string) observation {
	fail := func(err error) observation { return observation{body: closedBody, err: err.Error()} }
	raw, err := (&net.Dialer{Timeout: armDeadline}).DialContext(ctx, "tcp", addr)
	if err != nil {
		return fail(fmt.Errorf("dial: %w", err))
	}
	defer func() { _ = raw.Close() }()
	if err := raw.SetDeadline(time.Now().Add(armDeadline)); err != nil {
		return fail(err)
	}
	conn := stdtls.Client(raw, &stdtls.Config{
		RootCAs:    pool,
		ServerName: sni,
		MinVersion: stdtls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
	})
	if err := conn.HandshakeContext(ctx); err != nil {
		return fail(fmt.Errorf("tls handshake: %w", err))
	}
	req := "GET / HTTP/1.1\r\nHost: " + sni + "\r\nConnection: close\r\n\r\n"
	if _, err := conn.Write([]byte(req)); err != nil {
		return fail(fmt.Errorf("write: %w", err))
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		return fail(fmt.Errorf("read response: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()
	bb, err := io.ReadAll(resp.Body)
	if err != nil {
		return fail(fmt.Errorf("read body: %w", err))
	}
	return observation{status: resp.StatusCode, body: string(bb)}
}

// --- fixture.StatsAsserter ---

// AssertStats asserts, per side, per row: status and the BODY (primary), then
// per chain the VALUE of http.<prefix>.downstream_rq_total, which must equal
// the number of rows expected to land on that chain (0 for a chain no row
// targets). Every counter pin is guarded by a presence check: a missing key
// reads 0 and would make a `== 0` pin vacuous.
//
// Errorf per property; Fatalf only for a failed scrape or a missing drive.
func (d *sniDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	for _, side := range []struct{ name, addr string }{{"ref", refAdminAddr}, {"subj", subjAdminAddr}} {
		st, err := scrapeHCMTotals(side.addr)
		if err != nil {
			t.Fatalf("%s: scrape: %v", side.name, err)
		}
		obs := d.recorded(side.name)
		if len(obs) == 0 {
			t.Fatalf("%s: no drive observations recorded — every assertion below would be vacuous", side.name)
			return
		}
		for _, l := range listeners {
			want := map[string]uint64{}
			for _, p := range l.probes {
				want[p.want]++
				assertRow(t, side.name, l, p, obs)
			}
			for _, c := range l.chains {
				name := "http." + c.prefix + ".downstream_rq_total"
				v, ok := st[name]
				log.Printf("%s: %s %s %s=%d(present=%t) want=%d", fixtureName, side.name, l.name, name, v, ok, want[c.id])
				if !ok {
					t.Errorf("%s %s: %s ABSENT from /stats/prometheus, want present and == %d", side.name, l.name, name, want[c.id])
					continue
				}
				if v != want[c.id] {
					t.Errorf("%s %s: %s = %d, want %d (chain %s)", side.name, l.name, name, v, want[c.id], c.id)
				}
			}
		}
	}
}

func assertRow(t fixture.TB, side string, l listener, p probe, obs map[string]observation) {
	t.Helper()
	got, ok := obs[l.name+" "+p.sni]
	if !ok {
		t.Errorf("%s %s sni=%s: no observation recorded", side, l.name, p.sni)
		return
	}
	wantBody := body(l.name, p.want)
	if got.body == closedBody {
		t.Errorf("%s %s sni=%s: connection CLOSED without a response (%s), want chain %s (body %q)",
			side, l.name, p.sni, got.err, p.want, wantBody)
		return
	}
	if got.status != wantStatus {
		t.Errorf("%s %s sni=%s: status = %d, want %d", side, l.name, p.sni, got.status, wantStatus)
	}
	if got.body != wantBody {
		t.Errorf("%s %s sni=%s: served body %q, want %q — the chain whose MATCHING server_names pattern is "+
			"most specific (exact, then longest *. suffix) must serve, regardless of declaration order",
			side, l.name, p.sni, got.body, wantBody)
	}
}

// scrapeHCMTotals fetches /stats/prometheus and RE-PROJECTS every
// envoy_http_downstream_rq_total sample onto its dotted name
// http.<envoy_http_conn_manager_prefix>.downstream_rq_total (the 0005
// precedent). Other metrics are ignored.
func scrapeHCMTotals(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", url, err)
	}
	const metric = "envoy_http_downstream_rq_total{"
	const label = `envoy_http_conn_manager_prefix="`
	out := map[string]uint64{}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, metric) {
			continue
		}
		closeIdx := strings.LastIndexByte(line, '}')
		if closeIdx < 0 {
			continue
		}
		labels := line[len(metric):closeIdx]
		i := strings.Index(labels, label)
		if i < 0 {
			continue
		}
		prefix := labels[i+len(label):]
		j := strings.IndexByte(prefix, '"')
		if j < 0 {
			continue
		}
		prefix = prefix[:j]
		val := strings.Fields(line[closeIdx+1:])
		if len(val) == 0 {
			continue
		}
		f, err := strconv.ParseFloat(val[0], 64)
		if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 {
			continue
		}
		out["http."+prefix+".downstream_rq_total"] += uint64(f)
	}
	return out, nil
}

// --- bootstrap rendering ---

// renderBootstrap builds one side's complete bootstrap. BOTH sides use this one
// renderer, differing only in bind address, admin port and listener ports, so
// the listener/chain shape is identical by construction. Certificates are
// inline_string on both sides (the reference container cannot read host
// filename: paths).
func renderBootstrap(bindAddr string, adminPort int, portFor func(i int) int, backendPort int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "admin:\n  address:\n    socket_address: { address: %s, port_value: %d }\nstatic_resources:\n  listeners:\n", bindAddr, adminPort)
	for i, l := range listeners {
		fmt.Fprintf(&b, listenerHeadTmpl, l.name, bindAddr, portFor(i))
		var def *chain
		for ci := range l.chains {
			c := l.chains[ci]
			if c.isDefault {
				def = &l.chains[ci]
				continue
			}
			quoted := make([]string, len(c.serverNames))
			for k, n := range c.serverNames {
				quoted[k] = strconv.Quote(n)
			}
			b.WriteString("        - name: fc_" + c.prefix + "\n")
			b.WriteString("          filter_chain_match:\n")
			b.WriteString("            server_names: [" + strings.Join(quoted, ", ") + "]\n")
			b.WriteString(indent(chainBody(l.name, c), 10))
		}
		if def != nil {
			b.WriteString("      default_filter_chain:\n")
			b.WriteString("        name: fc_" + def.prefix + "\n")
			b.WriteString(indent(chainBody(l.name, *def), 8))
		}
	}
	fmt.Fprintf(&b, clustersTmpl, backendPort)
	return b.String()
}

// chainBody renders one chain's transport_socket + filters at column 0; the
// caller indents it, and indent() splices the PEMs in at their final column.
func chainBody(listenerName string, c chain) string {
	return fmt.Sprintf(chainTmpl, c.prefix, strconv.Quote(body(listenerName, c.id)))
}

// indent prefixes every line with n spaces, replacing the @@CERT@@ / @@KEY@@
// placeholder lines with the PEM indented to the placeholder's own final
// column (n + 14), so the block scalar stays well-formed at any depth.
func indent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, ln := range lines {
		switch strings.TrimSpace(ln) {
		case "@@CERT@@":
			lines[i] = indentPEM(mustReadFixtureBytes("pki/server.pem"), n+14)
		case "@@KEY@@":
			lines[i] = indentPEM(mustReadFixtureBytes("pki/server.key.pem"), n+14)
		default:
			lines[i] = pad + ln
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// listenerHeadTmpl: name, bind address, port. tls_inspector is LOAD-BEARING.
const listenerHeadTmpl = `    - name: %s
      address:
        socket_address: { address: %s, port_value: %d }
      listener_filters:
        - name: envoy.filters.listener.tls_inspector
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.listener.tls_inspector.v3.TlsInspector
      filter_chains:
`

// chainTmpl: 1 stat_prefix, 2 quoted body. Rendered at column 0.
const chainTmpl = `transport_socket:
  name: envoy.transport_sockets.tls
  typed_config:
    "@type": type.googleapis.com/envoy.extensions.transport_sockets.tls.v3.DownstreamTlsContext
    common_tls_context:
      alpn_protocols: ["http/1.1"]
      tls_certificates:
        - certificate_chain:
            inline_string: |
              @@CERT@@
          private_key:
            inline_string: |
              @@KEY@@
filters:
  - name: envoy.filters.network.http_connection_manager
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
      codec_type: HTTP1
      stat_prefix: %[1]s
      route_config:
        name: rc_%[1]s
        virtual_hosts:
          - name: vh_%[1]s
            domains: ["*"]
            routes:
              - match: { prefix: "/" }
                direct_response:
                  status: 200
                  body: { inline_string: %[2]s }
      http_filters:
        - name: envoy.filters.http.router
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
`

// clustersTmpl: the never-dialed placeholder cluster. Arg: backend port.
const clustersTmpl = `  clusters:
    - name: c_unused
      type: STATIC
      connect_timeout: 0.25s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_unused
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`

// --- file helpers (the 0121 idiom) ---

func serverCAPool() (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(mustReadFixtureBytes("pki/ca.pem")) {
		return nil, errors.New("pki/ca.pem: no certificate appended")
	}
	return pool, nil
}

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureBytes(name string) []byte {
	b, err := os.ReadFile(filepath.Join(fixtureDir(), filepath.FromSlash(name))) //nolint:gosec // fixture-relative, test-only
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return b
}

// indentPEM prefixes every line of a PEM with `spaces` spaces for a YAML block
// scalar; the trailing newline is trimmed first.
func indentPEM(pemBytes []byte, spaces int) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(strings.TrimRight(string(pemBytes), "\n"), "\n")
	for i, l := range lines {
		lines[i] = pad + l
	}
	return strings.Join(lines, "\n")
}
