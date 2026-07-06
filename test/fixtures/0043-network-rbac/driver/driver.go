// Package driver registers the 0043-network-rbac cross-side differential
// fixture with the runner per phase 26.3 SPEC §8 + PLAN Task 14. It is the
// FIRST production mixed read→terminal network filter chain (R-M-LIVE): each
// listener's filter chain is [rbac_network, tcp_proxy], so a read filter
// (rbac_network) makes an L4 enforcement decision and — when it Continues —
// hands off to the terminal tcp_proxy which passes bytes through to a TCP echo
// backend. Both reference Envoy v1.37.2 (dockerized) and envoy-go boot the same
// three-listener bootstrap and the driver asserts byte-exact parity across the
// allow / deny / shadow scenarios + the four `<stat_prefix>.rbac.*` counters.
//
// # Three-listener scenario partition (mirrors 0018-http-rbac's multi-listener
// shape; L4 RBAC is connection-scoped so each scenario needs its own listener)
//
//   - l_allow  — rbac ALLOW: a Policy whose Permission is destination_port =
//     <this side's listener port> AND whose Principal is direct_remote_ip
//     0.0.0.0/0. BOTH the destination_port permission and the direct_remote_ip
//     principal are GENUINELY evaluated (the L4 accessors are plumbed at
//     Task 9). The principal matches any source IP, which is the cross-side-
//     stable choice: the reference container sees the connection's source IP as
//     the Docker bridge gateway while envoy-go (on the host) sees 127.0.0.1, so
//     a loopback-specific CIDR would DIVERGE — 0.0.0.0/0 matches identically on
//     both sides. The destination_port is templated to each side's ACTUAL
//     listener port (15043 in-container for ref; the runner-allocated random
//     port for subj), so `conn.LocalAddr().Port` matches the permission on both
//     sides. → ALLOW → tcp_proxy passthrough → byte-exact echo of the payload.
//     `<stat_prefix>.rbac.allowed` += 1.
//
//   - l_deny — rbac DENY (default-deny): an empty `rules` (action ALLOW, NO
//     policies) means nothing matches → the ALLOW-action engine denies → the
//     enforced-deny path sets response-code-details "rbac_deny_close" and closes
//     the connection (NoFlush) BEFORE tcp_proxy ever sees a byte. The downstream
//     observes a connection close with ZERO echoed bytes on BOTH sides (the
//     tcp_proxy upstream is never dialed). `<stat_prefix>.rbac.denied` += 1.
//
//   - l_shadow — rbac enforced-ALLOW + shadow-DENY: the enforced `rules` is the
//     same destination_port+direct_remote_ip ALLOW as l_allow (→ passthrough),
//     while `shadow_rules` (action ALLOW, NO policies → default-deny) denies in
//     shadow. Enforced passthrough → byte-exact echo; the shadow walk ticks
//     `<stat_prefix>.rbac.<shadow_rules_stat_prefix>.shadow_denied` += 1 AND
//     writes the shadow pair to connection dynamic-metadata (emitted-but-unread
//     at L4 — asserted indirectly via the stat here + directly by the Task-11
//     unit test). `shadow_denied` is the load-bearing cross-side counter.
//
// # Why a connection-scoped byte stream works for the cross-side diff
//
// driveProxy issues one TCP round-trip per listener (allow → deny → shadow) and
// emits a deterministic per-scenario verdict line. The "side" label (ref vs
// subj) is INTENTIONALLY excluded so both sides produce identical bytes when
// behavior is equivalent; the runner's CompareBytes pass enforces equivalence.
// The allow + shadow scenarios echo the payload back (byte-exact); the deny
// scenario yields zero echoed bytes (connection close) — all three verdicts are
// byte-identical across the two proxies, so the differential gate fires.
//
// # StatsAsserter (asserter-dispatch memory: cross-side MUST use StatsAsserter)
//
// AssertStats scrapes /stats/prometheus from BOTH admin endpoints after the
// workload and asserts the per-side `<stat_prefix>.rbac.*` counters. This is the
// LIVE subject-side assertion path (StatsAsserter runs on the cross-side path;
// SubjectAsserter would NOT — it only runs on the reference-less path). The
// deliberate-break proof (PROGRESS.md Task 14) flips an expected counter to a
// wrong value and confirms the test FAILS, proving the assertion is not vacuous.
//
// # D-26.3-4 (SNI / mTLS-authenticated scenario) — UNIT-ONLY at 26.3
//
// The PLAN's D-26.3-4 makes an SNI (requested_server_name) + mTLS-authenticated
// L4 scenario OPTIONAL, conditioned on the differential harness readily
// supporting client-cert/SNI driving for an L4 rbac scenario. Driving SNI/mTLS
// at L4 requires a downstream TLS transport_socket on the rbac listener + a
// tls_inspector + a client-cert harness — substantially heavier than the
// plaintext L4 path and not readily reusable from the HTTP-oriented 0018 PKI
// harness (which terminates TLS at the HCM, not at a raw L4 listener). Per
// D-26.3-4 this fixture stays plaintext; the RequestedServerName +
// DownstreamPrincipal accessor mapping is UNIT-covered at Task 9
// (internal/filter/network/rbac/evalctx_test.go asserts both accessors), and the
// cross-side SNI/mTLS gap is recorded honestly in PROGRESS.md Task 14.
//
// # Bootstrap discipline (cluster requirement)
//
// The tcp_proxy terminal needs an upstream cluster — c_echo (the TCP echo
// backend the runner spawns). A zero-cluster boot is rejected by both sides, so
// c_echo doubles as the boot-satisfying cluster AND the passthrough target for
// the allow + shadow scenarios. The network-filter @type URLs carry the
// `extensions.` segment (memory reference_network_filter_typeurl_extensions).
//
// # Cross-references
//
//   - parent SPEC §8 (cross-side network-rbac fixture scope)
//   - 26.3 PLAN Task 14 (R-M-LIVE + StatsAsserter + D-26.3-4)
//   - fixture-0018-http-rbac (the StatsAsserter precedent for rbac.* counters)
//   - fixture-0040-network-echo / 0001-tcp-proxy-rr (the network-filter
//     bootstrap shape + tcp_proxy upstream cluster)
package driver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0043-network-rbac"

	refAdminPort = 9901

	// In-container reference Envoy listener ports (one per scenario). Convention
	// "150NN" for fixture "00NN" — 0043 takes 15043 for the primary (l_allow)
	// and consecutive ports for l_deny + l_shadow.
	refLAllowPort  = 15043
	refLDenyPort   = 15044
	refLShadowPort = 15045

	// stat_prefix roots for each listener's rbac_network config — the four
	// `<stat_prefix>.rbac.*` counters live under these. Distinct prefixes keep
	// the per-scenario counters disjoint so AssertStats can pin each.
	statPrefixAllow  = "rbac_allow"
	statPrefixDeny   = "rbac_deny"
	statPrefixShadow = "rbac_shadow"

	// shadow_rules_stat_prefix for l_shadow — inserts a segment between `rbac.`
	// and the two shadow_* counters (SPEC §7.1). So the shadow_denied counter is
	// `rbac_shadow.rbac.shadow_ns.shadow_denied`.
	shadowRulesStatPrefix = "shadow_ns"
)

func init() {
	fixture.RegisterFixture(fixtureName, &rbacNetDriver{})
}

// rbacNetDriver carries no mutable cross-scenario state — the three-listener
// matrix is fully deterministic.
type rbacNetDriver struct{}

// --- fixture.Driver (required) ---

func (*rbacNetDriver) BackendCount() int { return 1 } // single TCP echo backend; allow + shadow passthrough to it, deny never dials it.

// SubjectListenerName returns the primary listener name (l_allow). Required by
// the Driver interface; MultiListenerDriver takes precedence at runtime.
func (*rbacNetDriver) SubjectListenerName() string { return "l_allow" }

// ReferenceListenerPort returns the primary reference listener port. Required by
// the Driver interface even though MultiListenerDriver dispatches at runtime.
func (*rbacNetDriver) ReferenceListenerPort() int { return refLAllowPort }

// ReferenceBootstrap renders the three-listener reference bootstrap. The
// destination_port permission on l_allow + l_shadow is templated to the
// IN-CONTAINER listener port (the port `conn.LocalAddr().Port` reports on the
// reference side). c_echo points at host.docker.internal:<backend> (ADR-0010
// STRICT_DNS).
func (*rbacNetDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("0.0.0.0, port_value: %d", refAdminPort),
		listenAddr:  "0.0.0.0",
		allowPort:   refLAllowPort,
		denyPort:    refLDenyPort,
		shadowPort:  refLShadowPort,
		clusterType: "STRICT_DNS",
		dnsLine:     "      dns_lookup_family: V4_ONLY\n",
		backendHost: "host.docker.internal",
		backendPort: backendPorts[0],
		nodeLine:    "",
	})
}

// SubjectConfig renders the three-listener subject bootstrap. The three subject
// listeners get consecutive ports starting from subjListenerPort
// (allow=subjListenerPort, deny=+1, shadow=+2) per the fixture-0018 /
// local-ratelimit multi-listener port-offset precedent. The destination_port
// permission is templated to each side's ACTUAL listener port.
func (*rbacNetDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("%s: expected 1 backend port, got %d", fixtureName, len(backendPorts)))
	}
	return renderBootstrap(bootstrapParams{
		adminAddr:   fmt.Sprintf("127.0.0.1, port_value: %d", subjAdminPort),
		listenAddr:  "127.0.0.1",
		allowPort:   subjListenerPort,
		denyPort:    subjListenerPort + 1,
		shadowPort:  subjListenerPort + 2,
		clusterType: "STATIC",
		dnsLine:     "",
		backendHost: "127.0.0.1",
		backendPort: backendPorts[0],
		nodeLine:    "node: { id: envoy-go-subject-0043, cluster: envoy-go-differential }\n",
	})
}

// --- fixture.MultiListenerDriver ---

func (*rbacNetDriver) SubjectListenerNames() []string {
	return []string{"l_allow", "l_deny", "l_shadow"}
}

func (*rbacNetDriver) ReferenceListenerPorts() []int {
	return []int{refLAllowPort, refLDenyPort, refLShadowPort}
}

func (d *rbacNetDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveReferenceMulti(ctx, map[string]string{"l_allow": addr})
}

func (d *rbacNetDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.DriveSubjectMulti(ctx, map[string]string{"l_allow": addr})
}

func (d *rbacNetDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

func (d *rbacNetDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "subj")
}

func (*rbacNetDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- scenario driving ---

// echoPayload is the deterministic payload sent on each scenario's TCP round-
// trip. The tcp_proxy passes it through to the echo backend which reflects it
// verbatim on the allow + shadow scenarios; the deny scenario closes the
// connection before any byte reaches tcp_proxy → zero echoed bytes.
func echoPayload() []byte {
	var p []byte
	for n := 0; n < 5; n++ {
		p = append(p, []byte(fmt.Sprintf("rbac-net-%d\n", n))...)
	}
	return p
}

// driveProxy issues one TCP round-trip per listener (allow → deny → shadow) and
// returns a deterministic per-scenario verdict byte stream. The side label is
// EXCLUDED so both sides produce identical bytes when behavior is equivalent.
//
// We do NOT half-close (TCPRoundTripNoHalfClose): a downstream FIN co-arriving
// with the payload can tear the connection down on reference Envoy before the
// passthrough echo flushes (characterized in fixture 0040); draining on the
// idle timeout is byte-exact on both sides. For the deny scenario the connection
// is closed by rbac immediately, so the read returns zero bytes promptly either
// way.
func (d *rbacNetDriver) driveProxy(ctx context.Context, addrs map[string]string, side string) ([]byte, error) {
	var b bytes.Buffer
	payload := echoPayload()

	scenarios := []struct {
		name       string
		listener   string
		expectEcho bool // true = expect payload reflected (allow/shadow); false = expect zero bytes (deny)
	}{
		{"allow", "l_allow", true},
		{"deny", "l_deny", false},
		{"shadow", "l_shadow", true},
	}

	for _, s := range scenarios {
		addr, ok := addrs[s.listener]
		if !ok {
			return nil, fmt.Errorf("%s: missing addr for listener %s", fixtureName, s.listener)
		}
		got, err := helpers.TCPRoundTripNoHalfClose(ctx, addr, payload, time.Second)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[fixture 0043 %s] scenario %s: round-trip error: %v\n", side, s.name, err)
			fmt.Fprintf(&b, "scenario %s verdict=ERR\n", s.name)
			continue
		}
		fmt.Fprintf(&b, "scenario %s verdict=%s\n", s.name, classify(got, payload, s.expectEcho))
	}
	return b.Bytes(), nil
}

// classify returns the byte-stream verdict for one scenario. The verdict is
// side-independent (no ref/subj label) so equivalent behavior yields identical
// bytes for CompareBytes.
func classify(got, payload []byte, expectEcho bool) string {
	if expectEcho {
		if bytes.Equal(got, payload) {
			return "echo_ok"
		}
		return fmt.Sprintf("echo_mismatch(got_len=%d,want_len=%d)", len(got), len(payload))
	}
	// Deny path: rbac closes the connection before tcp_proxy → zero echoed bytes.
	if len(got) == 0 {
		return "closed_no_bytes"
	}
	return fmt.Sprintf("deny_leak(got_len=%d)", len(got))
}

// --- fixture.StatsAsserter (asserter-dispatch memory: cross-side MUST use
// StatsAsserter; SubjectAsserter would be a dead vacuous assertion) ---

// AssertStats scrapes /stats/prometheus from both admin endpoints and asserts
// the per-side `<stat_prefix>.rbac.*` counters per SPEC §7. The three listeners
// use disjoint stat_prefixes so each scenario's counter is independently
// pinned. The assertion runs on BOTH sides (ref + subj) — this is the LIVE
// subject-side counter assertion the asserter-dispatch memory mandates.
//
// Expected counters after the one-round-trip-per-listener workload:
//
//   - rbac_allow.rbac.allowed   = 1 (allow scenario ALLOW)
//   - rbac_allow.rbac.denied    = 0
//   - rbac_deny.rbac.denied     = 1 (deny scenario default-deny)
//   - rbac_deny.rbac.allowed    = 0
//   - rbac_shadow.rbac.allowed  = 1 (shadow scenario enforced-ALLOW)
//   - rbac_shadow.rbac.shadow_ns.shadow_denied = 1 (shadow scenario shadow-DENY)
//   - rbac_shadow.rbac.shadow_ns.shadow_allowed = 0
func (d *rbacNetDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeRBACStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref rbac stats: %v", err)
	}
	subjStats, err := scrapeRBACStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj rbac stats: %v", err)
	}

	if os.Getenv("FIXTURE_0043_DUMP_STATS") != "" {
		dump := func(label string, m map[string]int64) {
			fmt.Fprintf(os.Stderr, "=== %s rbac stats ===\n", label)
			for k, v := range m {
				fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
			}
		}
		dump("ref", refStats)
		dump("subj", subjStats)
	}

	type counterExpect struct {
		metric string // the rbac counter metric stem (after the `envoy_` prefix is normalized away by lookupCounter)
		want   int64
	}
	expectations := []counterExpect{
		{"rbac_allow.rbac.allowed", 1},
		{"rbac_allow.rbac.denied", 0},
		{"rbac_deny.rbac.denied", 1},
		{"rbac_deny.rbac.allowed", 0},
		{"rbac_shadow.rbac.allowed", 1},
		{"rbac_shadow.rbac.shadow_ns.shadow_denied", 1},
		{"rbac_shadow.rbac.shadow_ns.shadow_allowed", 0},
	}

	for _, side := range []struct {
		label string
		stats map[string]int64
	}{{"ref", refStats}, {"subj", subjStats}} {
		for _, exp := range expectations {
			got := lookupCounter(side.stats, exp.metric)
			if got != exp.want {
				t.Errorf("%s %s = %d, want %d", side.label, exp.metric, got, exp.want)
			}
		}
	}
}

// --- stats scraping ---

// scrapeRBACStats issues GET /stats/prometheus against adminAddr and returns a
// map of rbac-related counter values keyed by the dotted internal stat name
// reconstructed from the Prometheus metric name. Both reference Envoy and
// envoy-go flatten `<stat_prefix>.rbac.<counter>` to a Prometheus name of the
// form `envoy_<flattened>` (dots → underscores); we retain only lines whose
// name contains the `_rbac_` infix and re-key by the dotted form so the
// AssertStats table can look up by the SPEC §7 stat name without committing to
// either side's exact Prometheus formatting.
func scrapeRBACStats(adminAddr string) (map[string]int64, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return parseRBACPromBody(resp.Body)
}

// parseRBACPromBody parses a Prometheus text-format body and returns a map keyed
// by the FULL metric name (with the `envoy_` prefix stripped) of int64 values
// for all lines whose metric name contains the `_rbac_` infix. The value is the
// last whitespace-separated token (the metric value; optional timestamp
// stripped).
func parseRBACPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantInfix = "_rbac_"
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var name, valueStr, labelStr string
		if idx := strings.IndexByte(line, '{'); idx >= 0 {
			name = line[:idx]
			closeIdx := strings.LastIndexByte(line, '}')
			if closeIdx < 0 || closeIdx+1 >= len(line) {
				continue
			}
			labelStr = line[idx+1 : closeIdx]
			valueStr = strings.TrimSpace(line[closeIdx+1:])
		} else {
			sp := strings.LastIndexByte(line, ' ')
			if sp < 0 {
				continue
			}
			name = line[:sp]
			valueStr = strings.TrimSpace(line[sp+1:])
		}
		if !strings.Contains(name, wantInfix) {
			continue
		}
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		f, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		name = strings.TrimPrefix(name, "envoy_")
		// Retain the full label set in the key so distinct stat_prefixes (which
		// reference Envoy surfaces as a tag-extractor LABEL rather than inlining
		// into the metric name) do not collapse onto the same bare-name key.
		key := name
		if labelStr != "" {
			key = name + "{" + labelStr + "}"
		}
		out[key] = int64(f)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

// lookupCounter resolves a SPEC §7 dotted stat name (e.g.
// "rbac_allow.rbac.allowed") against the scraped map. BOTH reference Envoy
// v1.37.2 AND envoy-go (after the phase-26.3 flattenToProm rbac tag-extractor
// rule) surface the network rbac counters in the tag-extractor shape:
//
//	<stat_prefix>.rbac.<rest>  →  envoy_rbac_<rest_flat>{envoy_rbac_prefix="<stat_prefix>"}
//
// where the `envoy_` prefix is stripped by parseRBACPromBody (leaving
// `rbac_<rest_flat>`) and the stat_prefix is promoted to the
// `envoy_rbac_prefix` label. So a dotted name `rbac_allow.rbac.allowed` splits
// into stat_prefix `rbac_allow` + rest `rbac.allowed` → metric name
// `rbac_allowed` + label `envoy_rbac_prefix="rbac_allow"`. We match the bare
// name AND require the label needle. Returns 0 when absent (absent-as-zero
// discipline per the phase-13/14/15/18 precedent).
func lookupCounter(stats map[string]int64, dotted string) int64 {
	// Split into <stat_prefix>.rbac.<rest>.
	const seg = ".rbac."
	idx := strings.Index(dotted, seg)
	if idx < 0 {
		return 0
	}
	statPrefix := dotted[:idx]
	rest := dotted[idx+1:] // "rbac.<rest>"
	wantName := strings.ReplaceAll(rest, ".", "_")
	wantLabel := `envoy_rbac_prefix="` + statPrefix + `"`
	for k, v := range stats {
		name, labelStr := k, ""
		if i := strings.IndexByte(k, '{'); i >= 0 {
			name = k[:i]
			if j := strings.LastIndexByte(k, '}'); j > i {
				labelStr = k[i+1 : j]
			}
		}
		if name == wantName && strings.Contains(labelStr, wantLabel) {
			return v
		}
	}
	return 0
}

// --- bootstrap rendering ---

type bootstrapParams struct {
	adminAddr   string // the "<ip>, port_value: <n>" fragment for the admin socket_address
	listenAddr  string // listener bind address (0.0.0.0 for ref container; 127.0.0.1 for subj)
	allowPort   int
	denyPort    int
	shadowPort  int
	clusterType string // STRICT_DNS (ref) | STATIC (subj)
	dnsLine     string // "      dns_lookup_family: V4_ONLY\n" for STRICT_DNS, else ""
	backendHost string
	backendPort int
	nodeLine    string // "node: {...}\n" for subj, "" for ref
}

// rbacNetworkType is the rbac_network typed_config @type URL — the network-
// filter type URLs carry the `extensions.` segment (memory
// reference_network_filter_typeurl_extensions); the proto FQN is
// envoy.extensions.filters.network.rbac.v3.RBAC.
const rbacNetworkType = "type.googleapis.com/envoy.extensions.filters.network.rbac.v3.RBAC"
const tcpProxyType = "type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy"

// allowRBAC renders an rbac_network typed_config with an ALLOW policy gated on
// destination_port == listenPort AND direct_remote_ip 0.0.0.0/0. statPrefix
// roots the four counters. Indented to sit under `typed_config:` at 14 spaces.
func allowRBAC(statPrefix string, listenPort int) string {
	return fmt.Sprintf(`                "@type": %s
                stat_prefix: %s
                rules:
                  action: ALLOW
                  policies:
                    p_allow:
                      permissions:
                        - destination_port: %d
                      principals:
                        - direct_remote_ip: { address_prefix: 0.0.0.0, prefix_len: 0 }
`, rbacNetworkType, statPrefix, listenPort)
}

// denyRBAC renders an rbac_network typed_config with an ALLOW action and NO
// policies — a default-deny: nothing matches so the enforced engine denies and
// closes the connection. statPrefix roots the four counters.
func denyRBAC(statPrefix string) string {
	return fmt.Sprintf(`                "@type": %s
                stat_prefix: %s
                rules:
                  action: ALLOW
                  policies: {}
`, rbacNetworkType, statPrefix)
}

// shadowRBAC renders an rbac_network typed_config with an enforced-ALLOW rules
// (destination_port + direct_remote_ip, same as allowRBAC) AND a shadow_rules
// default-deny (ALLOW action, no policies). The enforced engine allows
// (passthrough); the shadow engine denies → shadow_denied increments + the
// shadow pair is written to connection dynamic-metadata.
func shadowRBAC(statPrefix, shadowPrefix string, listenPort int) string {
	return fmt.Sprintf(`                "@type": %s
                stat_prefix: %s
                shadow_rules_stat_prefix: %s
                rules:
                  action: ALLOW
                  policies:
                    p_allow:
                      permissions:
                        - destination_port: %d
                      principals:
                        - direct_remote_ip: { address_prefix: 0.0.0.0, prefix_len: 0 }
                shadow_rules:
                  action: ALLOW
                  policies: {}
`, rbacNetworkType, statPrefix, shadowPrefix, listenPort)
}

// tcpProxyFilter renders the terminal tcp_proxy filter routing to c_echo.
func tcpProxyFilter(statPrefix string) string {
	return fmt.Sprintf(`                "@type": %s
                stat_prefix: %s
                cluster: c_echo
`, tcpProxyType, statPrefix)
}

// renderBootstrap assembles the full three-listener bootstrap. Each listener's
// filter chain is [rbac_network, tcp_proxy] — the R-M-LIVE mixed read→terminal
// chain. c_echo is the tcp_proxy upstream (the runner's TCP echo backend) AND
// the boot-satisfying cluster (a zero-cluster boot is rejected by both sides).
func renderBootstrap(p bootstrapParams) string {
	listener := func(name string, port int, rbacCfg string, tcpStat string) string {
		return fmt.Sprintf(`    - name: %s
      address:
        socket_address: { address: %s, port_value: %d }
      filter_chains:
        - filters:
            - name: envoy.filters.network.rbac
              typed_config:
%s            - name: envoy.filters.network.tcp_proxy
              typed_config:
%s`, name, p.listenAddr, port, rbacCfg, tcpProxyFilter(tcpStat))
	}

	return fmt.Sprintf(`%sadmin:
  address:
    socket_address: { address: %s }
static_resources:
  listeners:
%s%s%s  clusters:
    - name: c_echo
      type: %s
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
%s      load_assignment:
        cluster_name: c_echo
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address:
                    socket_address: { address: %s, port_value: %d }
`,
		p.nodeLine,
		p.adminAddr,
		listener("l_allow", p.allowPort, allowRBAC(statPrefixAllow, p.allowPort), "tcp_allow"),
		listener("l_deny", p.denyPort, denyRBAC(statPrefixDeny), "tcp_deny"),
		listener("l_shadow", p.shadowPort, shadowRBAC(statPrefixShadow, shadowRulesStatPrefix, p.shadowPort), "tcp_shadow"),
		p.clusterType,
		p.dnsLine,
		p.backendHost, p.backendPort,
	)
}

// Compile-time interface assertions.
var (
	_ fixture.Driver              = (*rbacNetDriver)(nil)
	_ fixture.MultiListenerDriver = (*rbacNetDriver)(nil)
	_ fixture.StatsAsserter       = (*rbacNetDriver)(nil)
)
