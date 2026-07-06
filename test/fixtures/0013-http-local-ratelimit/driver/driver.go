// Package driver registers the 0013-http-local-ratelimit fixture with the
// differential runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.local_ratelimit and reference Envoy v1.37.2 across the
// four-scenario matrix per phase 11 SPEC §7.1.
//
// Integration shape (SPEC §7.3 driver outline):
//
//  1. ReferenceBootstrap renders test/fixtures/0013-http-local-ratelimit/envoy.yaml
//     with the backend host set to host.docker.internal (ADR-0010 STRICT_DNS) +
//     runner-allocated backend port. SubjectConfig renders envoy-go.yaml with
//     the runner-allocated subject admin/listener ports + backend port (loopback).
//
//  2. The fixture exposes four listeners (l_s1, l_s2, l_s3, l_per_route) and
//     implements fixture.MultiListenerDriver so the runner allocates and publishes
//     all four reference ports and all four subject listener addresses.
//
//  3. DriveReferenceMulti / DriveSubjectMulti run all 4 scenarios in a single
//     call (planner-time decision 8 — avoids per-scenario teardown framework
//     extension; bucket isolation is achieved at boot via the 4-listener topology
//     since each listener carries its own factory-built *runtimeConfig +
//     *tokenBucket). Each scenario emits a deterministic per-request assertion-log
//     byte stream. The runner's CompareBytes pass enforces equivalence — when both
//     proxies produce equal logs, the differential gate fires.
//
//     The 4 scenarios per SPEC §7.1:
//     1: basic_allow       — 5 GETs to l_s1 (cap=10); all 200
//     2: basic_rate_limited — 5 GETs to l_s2 (cap=2, interval=60s); req3-5=429
//     3: refill_after_fill_interval — 3 GETs to l_s3 (cap=1, interval=200ms)
//     with ±10ms timing tolerance per ADR-0116 + planner-time decision 4
//     4: per_route_override — 6 interleaved GETs to l_per_route (/strict + /loose)
//
//  4. AssertStats scrapes /stats/prometheus from both admin endpoints and asserts
//     the local_ratelimit counter values per SPEC §7.1 final-state matrix.
//
//  5. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner step 9.
//
// IMPL-1 note (per PROGRESS.md): the per-route TPFC entry uses
// @type: ...LocalRateLimit (the same proto as the listener-level config; upstream
// Envoy v1.37.2 has no separate LocalRateLimitPerRoute message). The fields go
// directly under the message (no rate_limit: wrapper).
package driver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0013-http-local-ratelimit"

	// In-container reference Envoy listener ports (pre-assigned per bootstrap).
	// refAdminPort is 9901 — the harness always exposes/waits on 9901/tcp for
	// the reference container's admin interface. All other fixtures use 9901
	// for the reference admin; this fixture follows the same convention.
	// (The PLAN sketch mentioned 9913 but that was conceptually wrong.)
	refAdminPort     = 9901
	refLS1Port       = 10013
	refLS2Port       = 10014
	refLS3Port       = 10015
	refLPerRoutePort = 10016
)

func init() {
	fixture.RegisterFixture(fixtureName, &localRateLimitDriver{})
}

type localRateLimitDriver struct{}

// --- fixture.Driver (required) ---

func (localRateLimitDriver) BackendCount() int                { return 1 }
func (localRateLimitDriver) BackendKind() fixture.BackendKind { return fixture.HTTPLocalRateLimit }

// SubjectListenerName returns the primary listener name (l_s1). The runner
// uses this to look up the subject's bound address for the single-addr
// DriveSubject path. Because this fixture implements MultiListenerDriver, the
// runner dispatches DriveSubjectMulti instead — DriveSubject is never invoked
// at runtime. The method is still REQUIRED by the Driver interface contract.
func (localRateLimitDriver) SubjectListenerName() string { return "l_s1" }

// ReferenceListenerPort returns the primary reference listener port (l_s1).
// Required by the Driver interface even though MultiListenerDriver takes
// precedence for the running path.
func (localRateLimitDriver) ReferenceListenerPort() int { return refLS1Port }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal +
// runner-allocated backend port. Reference Envoy admin + listener ports are
// pre-assigned constants (9901, 10013-10016).
func (localRateLimitDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":     refAdminPort,
		"LS1Port":       refLS1Port,
		"LS2Port":       refLS2Port,
		"LS3Port":       refLS3Port,
		"LPerRoutePort": refLPerRoutePort,
		"BackendHost":   "host.docker.internal",
		"BackendPort":   backendPorts[0],
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + backend port (loopback). The four subject listeners get consecutive
// ports starting from subjListenerPort: LS1=subjListenerPort,
// LS2=subjListenerPort+1, LS3=subjListenerPort+2, LPerRoute=subjListenerPort+3.
// This mirrors the 0012-http-header-mutation pattern where the second listener
// gets LwsPort+1.
func (localRateLimitDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":     subjAdminPort,
		"LS1Port":       subjListenerPort,
		"LS2Port":       subjListenerPort + 1,
		"LS3Port":       subjListenerPort + 2,
		"LPerRoutePort": subjListenerPort + 3,
		"BackendPort":   backendPorts[0],
	})
}

// DriveReference is the single-addr path; never called at runtime because
// MultiListenerDriver is implemented. Delegates to DriveReferenceMulti with
// the single addr mapped to l_s1, deriving the remaining addrs by port offset.
func (d localRateLimitDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromRef(addr)
	return d.DriveReferenceMulti(ctx, addrs)
}

// DriveSubject is the single-addr path; never called at runtime because
// MultiListenerDriver is implemented. Delegates to DriveSubjectMulti with
// the single addr mapped to l_s1, deriving the remaining addrs by port offset.
func (d localRateLimitDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromSubj(addr)
	return d.DriveSubjectMulti(ctx, addrs)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin-diff at runner step 9.
func (localRateLimitDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- fixture.MultiListenerDriver ---

// SubjectListenerNames returns all four listener names in order (primary first).
func (localRateLimitDriver) SubjectListenerNames() []string {
	return []string{"l_s1", "l_s2", "l_s3", "l_per_route"}
}

// ReferenceListenerPorts returns all four in-container listener ports.
func (localRateLimitDriver) ReferenceListenerPorts() []int {
	return []int{refLS1Port, refLS2Port, refLS3Port, refLPerRoutePort}
}

// DriveReferenceMulti issues all 4 scenario probes against the reference proxy.
// addrs maps listener names to "host:port" strings (provided by the runner).
func (d localRateLimitDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return driveAll(ctx, addrs)
}

// DriveSubjectMulti issues all 4 scenario probes against the subject proxy.
// addrs maps listener names to "host:port" strings (provided by the runner).
func (d localRateLimitDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return driveAll(ctx, addrs)
}

// --- fixture.StatsAsserter ---

// AssertStats scrapes /stats/prometheus from both admin endpoints and asserts
// the local_ratelimit counter values per SPEC §7.1 final-state matrix.
//
// Stat names: per ADR-0118 + ADR-0061 SN9, the Envoy-domain stat name
// `local_ratelimit.<stat_prefix>.<n>` flattens to Prometheus name
// `envoy_local_ratelimit_<n>{envoy_local_http_ratelimit_prefix="<stat_prefix>"}`.
// We assert that for each stat_prefix (foo, bar, baz, strict, qux) the
// expected counter values match on both sides.
func (localRateLimitDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	// Per SPEC §7.1 final-state matrix after the 4-scenario run:
	//
	// foo (l_s1, cap=10):  5 requests all allowed → ok=5, rate_limited=0
	// bar (l_s2, cap=2, interval=60s): req1+2 allowed, req3+4+5 limited → ok=2, rate_limited=3
	// baz (l_s3, cap=1, interval=200ms): req1 allowed, req2 limited, req3 allowed (refill) → ok=2, rate_limited=1
	// strict (l_per_route /strict, cap=1, interval=60s): req1 allowed, req3+5 limited → ok=1, rate_limited=2
	// qux (l_per_route listener, cap=10): only /loose requests reach the listener-level
	//   bucket (per-route override for /strict resolves a per-route *runtimeConfig that
	//   uses strict stats, NOT the listener-level qux stats). 3 /loose requests → ok=3.
	type statExpect struct {
		prefix      string
		ok          int64
		rateLimited int64
	}
	expectations := []statExpect{
		{"foo", 5, 0},
		{"bar", 2, 3},
		{"baz", 2, 1},
		{"strict", 1, 2},
		{"qux", 3, 0},
	}

	refStats, err := scrapeRateLimitStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref ratelimit stats: %v", err)
	}
	subjStats, err := scrapeRateLimitStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj ratelimit stats: %v", err)
	}

	for _, exp := range expectations {
		okKey := fmt.Sprintf("envoy_http_local_rate_limit_ok|%s", exp.prefix)
		rlKey := fmt.Sprintf("envoy_http_local_rate_limit_rate_limited|%s", exp.prefix)

		if got := refStats[okKey]; got != exp.ok {
			t.Errorf("ref %s ok: got %d, want %d", exp.prefix, got, exp.ok)
		}
		if got := subjStats[okKey]; got != exp.ok {
			t.Errorf("subj %s ok: got %d, want %d", exp.prefix, got, exp.ok)
		}
		if got := refStats[rlKey]; got != exp.rateLimited {
			t.Errorf("ref %s rate_limited: got %d, want %d", exp.prefix, got, exp.rateLimited)
		}
		if got := subjStats[rlKey]; got != exp.rateLimited {
			t.Errorf("subj %s rate_limited: got %d, want %d", exp.prefix, got, exp.rateLimited)
		}
	}
}

// --- core drive logic ---

// driveAll orchestrates all 4 scenarios sequentially against the 4
// pre-configured listeners. The "side" (ref vs subj) is INTENTIONALLY excluded
// from the log so both sides produce identical byte streams when behavior is
// equivalent.
func driveAll(ctx context.Context, addrs map[string]string) ([]byte, error) {
	var b bytes.Buffer
	driveScenario1(ctx, &b, addrs["l_s1"])
	driveScenario2(ctx, &b, addrs["l_s2"])
	driveScenario3(ctx, &b, addrs["l_s3"])
	driveScenario4(ctx, &b, addrs["l_per_route"])
	return b.Bytes(), nil
}

// driveScenario1 sends 5 sequential GETs to l_s1 (cap=10, interval=1s).
// All 5 requests should be allowed (status 200) since cap exceeds request count.
func driveScenario1(ctx context.Context, b *bytes.Buffer, addr string) {
	fmt.Fprintln(b, "=== scenario_1_basic_allow ===")
	for i := 1; i <= 5; i++ {
		status, _, _ := probe(ctx, addr, "/")
		fmt.Fprintf(b, "req%d: status=%d\n", i, status)
	}
}

// driveScenario2 sends 5 sequential GETs to l_s2 (cap=2, interval=60s).
// req1+req2 are allowed; req3-req5 are rate-limited (429). For 429 responses,
// captures body (byte-exact "local_rate_limited") + 4 sorted response headers
// (content-length, content-type, date allow-listed, server).
func driveScenario2(ctx context.Context, b *bytes.Buffer, addr string) {
	fmt.Fprintln(b, "=== scenario_2_basic_rate_limited ===")
	for i := 1; i <= 5; i++ {
		status, headers, body := probe(ctx, addr, "/")
		fmt.Fprintf(b, "req%d: status=%d\n", i, status)
		if status == 429 {
			fmt.Fprintf(b, "  body: %q\n", strings.TrimRight(string(body), "\n"))
			emitRateLimitedHeaders(b, headers)
		}
	}
}

// driveScenario3 is the timing-sensitive refill scenario. l_s3 has cap=1 and
// fill_interval=200ms. Issues 3 GETs:
//   - req1 at t=0: consumes the single token → 200
//   - req2 at t≈10ms: bucket empty → 429
//   - req3 at t≈250ms: bucket refilled (>200ms elapsed) → 200
//
// Per ADR-0116 + planner-time decision 4: simple time.Sleep with post-hoc band
// assertion at [200, 260]ms. If the assertion fires TOLERANCE_FAIL, the byte
// stream includes the sentinel so CompareBytes surfaces it as a difference
// (both sides should either pass or fail together — a failure indicates a
// fundamental timing problem on this test host).
func driveScenario3(ctx context.Context, b *bytes.Buffer, addr string) {
	fmt.Fprintln(b, "=== scenario_3_refill_after_fill_interval ===")
	t0 := time.Now()

	status1, _, _ := probe(ctx, addr, "/")
	fmt.Fprintf(b, "req1 (t=0): status=%d\n", status1)

	// Sleep until ~10ms from t0 before issuing req2.
	if remaining := 10*time.Millisecond - time.Since(t0); remaining > 0 {
		time.Sleep(remaining)
	}
	status2, _, _ := probe(ctx, addr, "/")
	fmt.Fprintf(b, "req2 (t=10ms): status=%d\n", status2)

	// Sleep until ~250ms from t0 before issuing req3 (fill_interval=200ms).
	if remaining := 250*time.Millisecond - time.Since(t0); remaining > 0 {
		time.Sleep(remaining)
	}
	// Capture delay before issuing req3 for post-hoc tolerance assertion.
	actualDelayMs := time.Since(t0).Milliseconds()
	status3, _, _ := probe(ctx, addr, "/")
	fmt.Fprintf(b, "req3 (t=250ms): status=%d\n", status3)

	// Post-hoc tolerance assertion: delay must be within [200, 260]ms.
	// Per ADR-0116: ±10ms band relative to the fill_interval=200ms boundary.
	// The upper bound 260ms = 250ms target + 10ms CI scheduling jitter.
	if actualDelayMs < 200 || actualDelayMs > 260 {
		fmt.Fprintf(b, "TOLERANCE_FAIL: req3 delay %dms outside [200, 260] band\n", actualDelayMs)
	} else {
		fmt.Fprintln(b, "tolerance: req3 delay within [200, 260]ms band")
	}
}

// driveScenario4 issues 6 interleaved GETs to l_per_route: /strict (per-route
// override, cap=1, interval=60s) and /loose (falls through to listener qux,
// cap=10). Captures status only (body is irrelevant for per-route test).
//
//	req1 /strict/x  → 200 (per-route strict bucket: first token consumed)
//	req2 /loose/x   → 200 (listener qux: token 1 of 10)
//	req3 /strict/x  → 429 (per-route strict bucket empty)
//	req4 /loose/x   → 200 (listener qux: token 2 of 10)
//	req5 /strict/x  → 429 (per-route strict bucket still empty)
//	req6 /loose/x   → 200 (listener qux: token 3 of 10)
//
// Per-route override: for /strict requests the per-route *runtimeConfig is
// used (with strict stats). For /loose requests no per-route TPFC applies, so
// the listener-level qux config is used (with qux stats).
func driveScenario4(ctx context.Context, b *bytes.Buffer, addr string) {
	fmt.Fprintln(b, "=== scenario_4_per_route_override ===")
	paths := []string{"/strict/x", "/loose/x", "/strict/x", "/loose/x", "/strict/x", "/loose/x"}
	for i, p := range paths {
		status, _, _ := probe(ctx, addr, p)
		fmt.Fprintf(b, "req%d (%s): status=%d\n", i+1, p, status)
	}
}

// emitRateLimitedHeaders emits the 4 rate-limit response headers in sorted
// order. The Date header value is allow-listed (varies per run) and rendered
// as "<allow-listed>". All other values are emitted verbatim.
//
// The 4 headers are: content-length, content-type, date, server.
// (x-envoy-ratelimited is also emitted by reference Envoy but may differ from
// envoy-go; we include it if present so any divergence surfaces in CompareBytes.)
func emitRateLimitedHeaders(b *bytes.Buffer, headers http.Header) {
	// Canonicalize header names to lowercase for determinism; sort alphabetically.
	type kv struct{ name, value string }
	var pairs []kv
	for name, values := range headers {
		lname := strings.ToLower(name)
		// Include rate-limit-relevant headers; skip connection-level noise.
		switch lname {
		case "connection", "transfer-encoding":
			continue
		}
		for _, v := range values {
			if lname == "date" {
				pairs = append(pairs, kv{name: lname, value: "<allow-listed>"})
			} else {
				pairs = append(pairs, kv{name: lname, value: v})
			}
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].name != pairs[j].name {
			return pairs[i].name < pairs[j].name
		}
		return pairs[i].value < pairs[j].value
	})
	for _, p := range pairs {
		fmt.Fprintf(b, "  header: %s: %s\n", p.name, p.value)
	}
}

// probe issues GET path against addr, reads the full response body, and
// returns (status, headers, body). Uses http.Client with a 5s timeout.
func probe(ctx context.Context, addr, path string) (int, http.Header, []byte) {
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", "http://"+addr+path, nil)
	if err != nil {
		return -1, nil, []byte(fmt.Sprintf("ERROR: %v", err))
	}
	resp, err := client.Do(req)
	if err != nil {
		return -1, nil, []byte(fmt.Sprintf("ERROR: %v", err))
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, body
}

// --- stats scraping ---

// scrapeRateLimitStats issues GET /stats/prometheus against adminAddr and
// parses the body into a map keyed by "metricname|statprefix" for all
// envoy_http_local_rate_limit_* metric values. The stat_prefix is extracted
// from the envoy_local_http_ratelimit_prefix label per ADR-0118 SN9.
func scrapeRateLimitStats(adminAddr string) (map[string]int64, error) {
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
	return parseRateLimitPromBody(resp.Body)
}

// parseRateLimitPromBody parses Prometheus text-format body and returns a map
// keyed by "metricname|statprefix" for all envoy_http_local_rate_limit_* metrics.
// The envoy_local_http_ratelimit_prefix label carries the stat_prefix per SN9.
// The metric prefix envoy_http_local_rate_limit_ matches both reference Envoy
// and envoy-go (per ADR-0118 empirical pin from the local ratelimit fixture run).
func parseRateLimitPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantPrefix = "envoy_http_local_rate_limit_"
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
		if !strings.HasPrefix(name, wantPrefix) {
			continue
		}
		// Extract stat_prefix from the envoy_local_http_ratelimit_prefix label.
		statPrefix := extractLabel(labelStr, "envoy_local_http_ratelimit_prefix")
		if statPrefix == "" {
			// Also try envoy_local_ratelimit_prefix (alternate label name).
			statPrefix = extractLabel(labelStr, "envoy_local_ratelimit_prefix")
		}
		// Strip optional timestamp.
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		f, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		key := name + "|" + statPrefix
		out[key] = int64(f)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

// extractLabel returns the value of the named label from a Prometheus label
// string (the contents of {...} in an exposition line), or "" if not found.
func extractLabel(labelStr, key string) string {
	prefix := key + `="`
	for _, part := range strings.Split(labelStr, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, prefix) && strings.HasSuffix(part, `"`) {
			return part[len(prefix) : len(part)-1]
		}
	}
	return ""
}

// --- address derivation helpers (for the single-addr stub paths) ---

// deriveAddrsFromRef derives the 3 additional listener addrs from the l_s1
// reference container address by replacing the port. The reference container
// exposes ports 10013 (l_s1), 10014 (l_s2), 10015 (l_s3), 10016 (l_per_route).
// This helper is only called by the fallback DriveReference stub (never reached
// at runtime when MultiListenerDriver is implemented).
func deriveAddrsFromRef(s1Addr string) map[string]string {
	replace := func(addr string, fromPort, toPort int) string {
		return strings.Replace(addr,
			fmt.Sprintf(":%d", fromPort),
			fmt.Sprintf(":%d", toPort), 1)
	}
	return map[string]string{
		"l_s1":        s1Addr,
		"l_s2":        replace(s1Addr, refLS1Port, refLS2Port),
		"l_s3":        replace(s1Addr, refLS1Port, refLS3Port),
		"l_per_route": replace(s1Addr, refLS1Port, refLPerRoutePort),
	}
}

// deriveAddrsFromSubj derives the 3 additional listener addrs from the l_s1
// subject address by incrementing the port. SubjectConfig assigns
// LS2=LS1+1, LS3=LS1+2, LPerRoute=LS1+3.
// This helper is only called by the fallback DriveSubject stub (never reached
// at runtime when MultiListenerDriver is implemented).
func deriveAddrsFromSubj(s1Addr string) map[string]string {
	lastColon := strings.LastIndex(s1Addr, ":")
	if lastColon < 0 {
		return map[string]string{"l_s1": s1Addr, "l_s2": s1Addr, "l_s3": s1Addr, "l_per_route": s1Addr}
	}
	host := s1Addr[:lastColon]
	port := s1Addr[lastColon+1:]
	var portNum int
	if _, err := fmt.Sscanf(port, "%d", &portNum); err != nil {
		return map[string]string{"l_s1": s1Addr, "l_s2": s1Addr, "l_s3": s1Addr, "l_per_route": s1Addr}
	}
	return map[string]string{
		"l_s1":        s1Addr,
		"l_s2":        fmt.Sprintf("%s:%d", host, portNum+1),
		"l_s3":        fmt.Sprintf("%s:%d", host, portNum+2),
		"l_per_route": fmt.Sprintf("%s:%d", host, portNum+3),
	}
}

// --- helpers ---

// fixtureDir returns the absolute path to the 0013-http-local-ratelimit fixture
// root, derived from runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0013-http-local-ratelimit/driver/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

// mustReadFixtureFile reads name from the fixture root directory.
func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

// mustRender renders a text/template body with data; panics on parse/exec
// errors (driver-time misconfiguration is non-recoverable).
func mustRender(tpl string, data map[string]any) string {
	t, err := template.New("bootstrap").Parse(tpl)
	if err != nil {
		panic(fmt.Sprintf("driver: template parse: %v", err))
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		panic(fmt.Sprintf("driver: template execute: %v", err))
	}
	return buf.String()
}

// Compile-time interface assertions.
var (
	_ fixture.Driver              = (*localRateLimitDriver)(nil)
	_ fixture.BackendKindAware    = (*localRateLimitDriver)(nil)
	_ fixture.MultiListenerDriver = (*localRateLimitDriver)(nil)
	_ fixture.StatsAsserter       = (*localRateLimitDriver)(nil)
)
