// Package driver registers the 0005-prometheus-stats fixture with the
// differential runner. This is the project's first observability-surface
// differential — it asserts per-counter delta-equality and per-gauge
// snapshot-equality between envoy-go and reference Envoy v1.37.2 on the
// 17 stat names enumerated in SPEC §6 (ADR-0062).
//
// Integration shape (per SPEC §12 #6 in-band discipline, ADR-0062):
//
//  1. DriveReference(ctx, refListenerAddr) issues the 5-request workload
//     [200, 200, 404, 200, 502] for the runner's byte-comparison step, and
//     saves refListenerAddr in the driver struct for AssertStats.
//
//  2. DriveSubject(ctx, subjListenerAddr) does the same and saves the addr.
//
//  3. AssertStats(t, refAdminAddr, subjAdminAddr) — invoked by the runner
//     after ProbeAdmin (step 10) — performs a SECOND scrape-before / 5-request
//     load / scrape-after cycle at both proxies using the saved listener addrs.
//     The second load produces the same expected deltas as the first; the stats
//     differential validates those deltas per ADR-0062.
//
// The two-pass design (Drive pass for bytes + AssertStats pass for stats) is
// intentional: the runner's byte-comparison step needs DriveReference /
// DriveSubject to produce deterministic output, while the stats assertion
// needs admin addresses that only become available to the runner after
// StartReferenceProxy + StartSubjectProxy have completed.
package driver

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

const fixtureName = "0005-prometheus-stats"
const refContainerListenerPort = 15005

func init() {
	fixture.RegisterFixture(fixtureName, &statsDriver{})
}

// statsDriver implements fixture.Driver, fixture.BackendKindAware, and
// fixture.StatsAsserter.
type statsDriver struct {
	mu              sync.Mutex
	refListenerAddr string
	subjListenerAddr string
}

// Snapshot holds the 17-name allow-listed metric values scraped from
// /stats/prometheus. Counters are uint64; gauges are int64. Absent names
// (metric not yet emitted) are treated as 0 per the allow-list discipline.
type Snapshot struct {
	// HCM counters (5)
	HCMRqTotal uint64
	HCMRq2xx   uint64
	HCMRq3xx   uint64
	HCMRq4xx   uint64
	HCMRq5xx   uint64

	// Cluster counters (6)
	ClusterRqTotal uint64
	ClusterRq2xx   uint64
	ClusterRq3xx   uint64
	ClusterRq4xx   uint64
	ClusterRq5xx   uint64
	ClusterCxTotal uint64

	// Listener counter (1)
	ListenerCxTotal uint64

	// Gauges (5)
	ListenerCxActive  int64
	ClusterCxActive   int64
	ClusterMembership int64
	ServerLive        int64
}

func (*statsDriver) BackendCount() int                { return 1 }
func (*statsDriver) BackendKind() fixture.BackendKind { return fixture.HTTPStatusHeader }
func (*statsDriver) SubjectListenerName() string      { return "l_h1" }
func (*statsDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ReferenceBootstrap returns the reference Envoy bootstrap YAML with the
// backend port filled in.
func (*statsDriver) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("0005: expected 1 backend port, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(referenceTmpl, backendPorts[0])
}

// SubjectConfig returns the envoy-go bootstrap YAML.
func (*statsDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) != 1 {
		panic(fmt.Sprintf("0005: expected 1 backend port, got %d", len(backendPorts)))
	}
	return fmt.Sprintf(subjectTmpl, subjAdminPort, subjListenerPort, backendPorts[0])
}

// DriveReference runs the 5-request workload against the reference proxy listener,
// saves the addr for AssertStats, and returns the concatenated 200 response bodies.
// Uses DisableKeepAlives=true so connections are closed after each response; this
// ensures the downstream cx_active gauge returns to 0 before the AssertStats
// scrape-before, enabling the gauge snapshot assertion.
func (d *statsDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.refListenerAddr = addr
	d.mu.Unlock()
	return drive(ctx, addr, true)
}

// DriveSubject runs the 5-request workload against the subject proxy listener,
// saves the addr for AssertStats, and returns the concatenated 200 response bodies.
func (d *statsDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	d.mu.Lock()
	d.subjListenerAddr = addr
	d.mu.Unlock()
	return drive(ctx, addr, true)
}

// drive issues the 5-request workload [200, 200, 404, 200, 502] against addr.
// Returns the concatenated bytes of 200 response bodies.
// disableKeepAlives=true forces a fresh TCP connection per request; used in the
// AssertStats pass to ensure cx_total counters increment as expected.
func drive(ctx context.Context, addr string, disableKeepAlives bool) ([]byte, error) {
	type req struct {
		path   string
		status int
		header map[string]string
	}
	plan := []req{
		{path: "/", status: 200},
		{path: "/", status: 200},
		{path: "/missing", status: 404},
		{path: "/", status: 200},
		{path: "/", status: 502, header: map[string]string{"X-Backend-Status": "502"}},
	}

	transport := &http.Transport{DisableKeepAlives: disableKeepAlives}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: transport,
	}

	var out strings.Builder
	for i, r := range plan {
		url := "http://" + addr + r.path
		httpReq, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, fmt.Errorf("req[%d]: build: %w", i, err)
		}
		for k, v := range r.header {
			httpReq.Header.Set(k, v)
		}
		resp, err := client.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("req[%d]: do: %w", i, err)
		}
		body, rerr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if rerr != nil {
			return nil, fmt.Errorf("req[%d]: read body: %w", i, rerr)
		}
		if resp.StatusCode != r.status {
			return nil, fmt.Errorf("req[%d]: got status %d, want %d (body=%q)", i, resp.StatusCode, r.status, string(body))
		}
		if resp.StatusCode == 200 {
			out.Write(body)
		}
	}
	return []byte(out.String()), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// raw HTTP response bytes for the runner's byte-comparison step.
func (*statsDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
	refBytes, err := httpGetRaw(ctx, "http://"+refAdminAddr+"/ready")
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err := httpGetRaw(ctx, "http://"+subjAdminAddr+"/ready")
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// httpGetRaw issues GET and returns a raw HTTP/1.1 response (status line +
// headers + body) as bytes, mirroring helpers.HTTPGetReadyRaw for admin /ready.
func httpGetRaw(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var buf strings.Builder
	fmt.Fprintf(&buf, "HTTP/1.1 %s\r\n", resp.Status)
	for k, vs := range resp.Header {
		for _, v := range vs {
			fmt.Fprintf(&buf, "%s: %s\r\n", k, v)
		}
	}
	buf.WriteString("\r\n")
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	buf.Write(body)
	return []byte(buf.String()), nil
}

// AssertStats implements fixture.StatsAsserter (SPEC §12 #6 in-band discipline,
// ADR-0062). Invoked by the runner at step 10 after ProbeAdmin. It performs a
// second scrape-before / 5-request load / scrape-after cycle at both proxies
// using the listener addresses saved during DriveReference/DriveSubject, then
// asserts per-counter delta-equality and per-gauge snapshot-equality on the
// 17-name allow-list.
func (d *statsDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	d.mu.Lock()
	refAddr := d.refListenerAddr
	subjAddr := d.subjListenerAddr
	d.mu.Unlock()

	if refAddr == "" || subjAddr == "" {
		t.Fatalf("stats: listener addresses not set (AssertStats called before Drive?)")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Wait for the Drive pass connections to drain before scraping "before".
	// Without this wait, cx_active gauges may be non-zero (stale from Drive).
	time.Sleep(200 * time.Millisecond)

	// Scrape-before, drive (no keepalive = fresh connections for reliable
	// cx_total counting), drain wait, scrape-after — for the reference proxy.
	refBefore, err := scrapeAndParse(ctx, refAdminAddr)
	if err != nil {
		t.Fatalf("stats: ref scrape-before: %v", err)
	}
	if _, err := drive(ctx, refAddr, true); err != nil {
		t.Fatalf("stats: ref drive: %v", err)
	}
	// Drain wait: active-connection gauges must reach 0 post-drive.
	time.Sleep(200 * time.Millisecond)
	refAfter, err := scrapeAndParse(ctx, refAdminAddr)
	if err != nil {
		t.Fatalf("stats: ref scrape-after: %v", err)
	}

	// Scrape-before, drive, drain wait, scrape-after — for the subject proxy.
	subjBefore, err := scrapeAndParse(ctx, subjAdminAddr)
	if err != nil {
		t.Fatalf("stats: subj scrape-before: %v", err)
	}
	if _, err := drive(ctx, subjAddr, true); err != nil {
		t.Fatalf("stats: subj drive: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	subjAfter, err := scrapeAndParse(ctx, subjAdminAddr)
	if err != nil {
		t.Fatalf("stats: subj scrape-after: %v", err)
	}

	// Assert per ADR-0062.
	AssertStatsEquivalence(t,
		refBefore, refAfter,
		subjBefore, subjAfter,
	)
}

// AssertStatsEquivalence is the in-band assertion entry-point per ADR-0062:
// per-counter delta_envoy_go == delta_envoy; per-gauge after_envoy_go == after_envoy.
// HELP text ignored; non-listed names ignored. cx_total counters use delta_min >= 1.
// Exported so driver_test.go can exercise it directly with synthetic snapshots.
func AssertStatsEquivalence(t fixture.TB, refBefore, refAfter, subjBefore, subjAfter Snapshot) {
	t.Helper()

	type counterRow struct {
		name      string
		refDelta  uint64
		subjDelta uint64
		deltaMin  bool
	}
	rows := []counterRow{
		{"http.ingress_http.downstream_rq_total",
			refAfter.HCMRqTotal - refBefore.HCMRqTotal,
			subjAfter.HCMRqTotal - subjBefore.HCMRqTotal, false},
		{"http.ingress_http.downstream_rq_2xx",
			refAfter.HCMRq2xx - refBefore.HCMRq2xx,
			subjAfter.HCMRq2xx - subjBefore.HCMRq2xx, false},
		{"http.ingress_http.downstream_rq_3xx",
			refAfter.HCMRq3xx - refBefore.HCMRq3xx,
			subjAfter.HCMRq3xx - subjBefore.HCMRq3xx, false},
		{"http.ingress_http.downstream_rq_4xx",
			refAfter.HCMRq4xx - refBefore.HCMRq4xx,
			subjAfter.HCMRq4xx - subjBefore.HCMRq4xx, false},
		{"http.ingress_http.downstream_rq_5xx",
			refAfter.HCMRq5xx - refBefore.HCMRq5xx,
			subjAfter.HCMRq5xx - subjBefore.HCMRq5xx, false},
		{"cluster.c0.upstream_rq_total",
			refAfter.ClusterRqTotal - refBefore.ClusterRqTotal,
			subjAfter.ClusterRqTotal - subjBefore.ClusterRqTotal, false},
		{"cluster.c0.upstream_rq_2xx",
			refAfter.ClusterRq2xx - refBefore.ClusterRq2xx,
			subjAfter.ClusterRq2xx - subjBefore.ClusterRq2xx, false},
		{"cluster.c0.upstream_rq_3xx",
			refAfter.ClusterRq3xx - refBefore.ClusterRq3xx,
			subjAfter.ClusterRq3xx - subjBefore.ClusterRq3xx, false},
		{"cluster.c0.upstream_rq_4xx",
			refAfter.ClusterRq4xx - refBefore.ClusterRq4xx,
			subjAfter.ClusterRq4xx - subjBefore.ClusterRq4xx, false},
		{"cluster.c0.upstream_rq_5xx",
			refAfter.ClusterRq5xx - refBefore.ClusterRq5xx,
			subjAfter.ClusterRq5xx - subjBefore.ClusterRq5xx, false},
		{"cluster.c0.upstream_cx_total",
			refAfter.ClusterCxTotal - refBefore.ClusterCxTotal,
			subjAfter.ClusterCxTotal - subjBefore.ClusterCxTotal, true},
		{"listener.<addr>.downstream_cx_total",
			refAfter.ListenerCxTotal - refBefore.ListenerCxTotal,
			subjAfter.ListenerCxTotal - subjBefore.ListenerCxTotal, true},
	}

	for _, r := range rows {
		if r.deltaMin {
			// delta_min >= 1: both sides must have incremented at least once.
			// Equality is NOT asserted for cx_total counters — keepalive may
			// collapse multiple requests onto fewer connections, and envoy-go
			// (no conn pooling per ADR-0056) and reference Envoy (pooling) will
			// produce different absolute counts. The contract is that BOTH sides
			// produced at least one connection (ADR-0062, SPEC §7.3).
			if r.refDelta < 1 {
				t.Errorf("stats counter %q: ref delta %d < delta_min 1", r.name, r.refDelta)
			}
			if r.subjDelta < 1 {
				t.Errorf("stats counter %q: subj delta %d < delta_min 1", r.name, r.subjDelta)
			}
		} else {
			if r.refDelta != r.subjDelta {
				t.Errorf("stats counter %q: delta mismatch: ref=%d subj=%d", r.name, r.refDelta, r.subjDelta)
			}
		}
	}

	// Gauge snapshot-equality: after_envoy_go == after_envoy.
	type gaugeRow struct {
		name    string
		refVal  int64
		subjVal int64
	}
	gauges := []gaugeRow{
		{"listener.<addr>.downstream_cx_active", refAfter.ListenerCxActive, subjAfter.ListenerCxActive},
		{"cluster.c0.upstream_cx_active", refAfter.ClusterCxActive, subjAfter.ClusterCxActive},
		{"cluster.c0.membership_total", refAfter.ClusterMembership, subjAfter.ClusterMembership},
		{"server.live", refAfter.ServerLive, subjAfter.ServerLive},
	}
	for _, g := range gauges {
		if g.refVal != g.subjVal {
			t.Errorf("stats gauge %q: snapshot mismatch: ref=%d subj=%d", g.name, g.refVal, g.subjVal)
		}
	}
}

// scrapeAndParse fetches /stats/prometheus from adminAddr (host:port) and
// parses the 17-name allow-list into a Snapshot. Names absent in the
// exposition output are treated as 0 (metric not yet emitted = 0).
func scrapeAndParse(ctx context.Context, adminAddr string) (Snapshot, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return Snapshot{}, fmt.Errorf("build request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Snapshot{}, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return Snapshot{}, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	return parsePromSnapshot(resp.Body)
}

// parsePromSnapshot parses a Prometheus text-format body and populates a
// Snapshot with the 17-name allow-list values. Lines not matching the
// allow-list are silently ignored (Rule SN6 + §7.4 allow-list discipline).
func parsePromSnapshot(r io.Reader) (Snapshot, error) {
	var s Snapshot
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, labels, val, err := parseMetricLine(line)
		if err != nil {
			// Non-fatal: skip malformed lines (reference Envoy may emit
			// metric families we don't understand; the allow-list ignores them).
			continue
		}
		applyToSnapshot(&s, name, labels, val)
	}
	if err := scanner.Err(); err != nil {
		return Snapshot{}, fmt.Errorf("scan: %w", err)
	}
	return s, nil
}

// parseMetricLine parses one Prometheus text-format metric line of the form
// `name{k="v",...} value` or `name value` into its component parts.
func parseMetricLine(line string) (name string, labels map[string]string, val float64, err error) {
	if idx := strings.IndexByte(line, '{'); idx >= 0 {
		name = line[:idx]
		rest := line[idx+1:]
		closeIdx := strings.LastIndexByte(rest, '}')
		if closeIdx < 0 {
			return "", nil, 0, fmt.Errorf("malformed: missing '}'")
		}
		labelStr := rest[:closeIdx]
		valueStr := strings.TrimSpace(rest[closeIdx+1:])
		// Strip optional timestamp.
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		val, err = strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return "", nil, 0, fmt.Errorf("parse value %q: %w", valueStr, err)
		}
		labels = parseLabels(labelStr)
	} else {
		sp := strings.LastIndexByte(line, ' ')
		if sp < 0 {
			return "", nil, 0, fmt.Errorf("malformed: no space separator")
		}
		name = line[:sp]
		valueStr := line[sp+1:]
		val, err = strconv.ParseFloat(valueStr, 64)
		if err != nil {
			return "", nil, 0, fmt.Errorf("parse value %q: %w", valueStr, err)
		}
		labels = map[string]string{}
	}
	return name, labels, val, nil
}

// parseLabels parses a `key="value",key2="value2"` label string into a map.
func parseLabels(s string) map[string]string {
	m := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		eq := strings.IndexByte(part, '=')
		if eq < 0 {
			continue
		}
		k := part[:eq]
		v := part[eq+1:]
		if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
			v = v[1 : len(v)-1]
		}
		m[k] = v
	}
	return m
}

// applyToSnapshot maps a parsed Prometheus metric line to the Snapshot's fields.
// Lines whose names/labels don't match the 17-name allow-list are silently
// skipped per §7.4 allow-list discipline.
func applyToSnapshot(s *Snapshot, name string, labels map[string]string, val float64) {
	ival := int64(val)
	uval := uint64(val)

	switch name {
	case "envoy_http_downstream_rq_total":
		if labels["envoy_http_conn_manager_prefix"] == "ingress_http" {
			s.HCMRqTotal = uval
		}
	case "envoy_http_downstream_rq_xx":
		if labels["envoy_http_conn_manager_prefix"] != "ingress_http" {
			return
		}
		switch labels["envoy_response_code_class"] {
		case "2":
			s.HCMRq2xx = uval
		case "3":
			s.HCMRq3xx = uval
		case "4":
			s.HCMRq4xx = uval
		case "5":
			s.HCMRq5xx = uval
		}
	case "envoy_cluster_upstream_rq_total":
		if labels["envoy_cluster_name"] == "c0" {
			s.ClusterRqTotal = uval
		}
	case "envoy_cluster_upstream_rq_xx":
		if labels["envoy_cluster_name"] != "c0" {
			return
		}
		switch labels["envoy_response_code_class"] {
		case "2":
			s.ClusterRq2xx = uval
		case "3":
			s.ClusterRq3xx = uval
		case "4":
			s.ClusterRq4xx = uval
		case "5":
			s.ClusterRq5xx = uval
		}
	case "envoy_cluster_upstream_cx_total":
		if labels["envoy_cluster_name"] == "c0" {
			s.ClusterCxTotal = uval
		}
	case "envoy_cluster_upstream_cx_active":
		if labels["envoy_cluster_name"] == "c0" {
			s.ClusterCxActive = ival
		}
	case "envoy_cluster_membership_total":
		if labels["envoy_cluster_name"] == "c0" {
			s.ClusterMembership = ival
		}
	case "envoy_listener_downstream_cx_total":
		// Any listener address (the addr token varies per run).
		s.ListenerCxTotal = uval
	case "envoy_listener_downstream_cx_active":
		s.ListenerCxActive = ival
	case "envoy_server_live":
		s.ServerLive = ival
	}
}

// subjectTmpl is the envoy-go bootstrap. Parameters: adminPort, listenerPort, backendPort.
var subjectTmpl = `admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_h1
      address: { socket_address: { address: 127.0.0.1, port_value: %d } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { path: /missing }
                          direct_response: { status: 404, body: { inline_string: "not found\n" } }
                        - match: { prefix: / }
                          route: { cluster: c0 }
  clusters:
    - name: c0
      type: STATIC
      connect_timeout: 1s
      load_assignment:
        cluster_name: c0
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address: { socket_address: { address: 127.0.0.1, port_value: %d } }
`

// referenceTmpl is the reference Envoy bootstrap. Parameter: backendPort.
var referenceTmpl = `admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_h1
      address: { socket_address: { address: 0.0.0.0, port_value: 15005 } }
      filter_chains:
        - filters:
            - name: envoy.filters.network.http_connection_manager
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
                codec_type: HTTP1
                stat_prefix: ingress_http
                http_filters:
                  - name: envoy.filters.http.router
                    typed_config:
                      "@type": type.googleapis.com/envoy.extensions.filters.http.router.v3.Router
                route_config:
                  virtual_hosts:
                    - name: vh
                      domains: ["*"]
                      routes:
                        - match: { path: /missing }
                          direct_response: { status: 404, body: { inline_string: "not found\n" } }
                        - match: { prefix: / }
                          route: { cluster: c0 }
  clusters:
    - name: c0
      type: STRICT_DNS
      dns_lookup_family: V4_ONLY
      connect_timeout: 1s
      load_assignment:
        cluster_name: c0
        endpoints:
          - lb_endpoints:
              - endpoint:
                  address: { socket_address: { address: host.docker.internal, port_value: %d } }
`

// Compile-time interface checks.
var (
	_ fixture.Driver           = (*statsDriver)(nil)
	_ fixture.BackendKindAware = (*statsDriver)(nil)
	_ fixture.StatsAsserter    = (*statsDriver)(nil)
)
