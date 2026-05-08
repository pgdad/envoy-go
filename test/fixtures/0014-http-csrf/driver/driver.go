// Package driver registers the 0014-http-csrf fixture with the differential
// runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.csrf and reference Envoy v1.37.2 across the six-scenario
// matrix per phase 12 SPEC §7.1.
//
// Integration shape (single-listener fixture.Driver — planner-time decision 7,
// mirrors the cors / fault / header_mutation precedent rather than phase 11's
// MultiListenerDriver fan-out):
//
//  1. ReferenceBootstrap renders test/fixtures/0014-http-csrf/envoy.yaml with
//     the backend host set to host.docker.internal (ADR-0010 STRICT_DNS) +
//     runner-allocated backend port. SubjectConfig renders envoy-go.yaml with
//     the runner-allocated subject admin/listener ports + backend port (loopback).
//
//  2. DriveReference / DriveSubject issue an identical 7-probe sequence (the 6
//     public scenarios; scenario 7 is split into 7a + 7b sub-requests that
//     share the same listener) against each proxy and emit a deterministic
//     per-probe assertion-log byte stream. The runner's CompareBytes pass
//     enforces equivalence — when both proxies produce equal logs, the
//     differential gate fires.
//
//     The 7 probes per SPEC §7.1:
//     1: 1-same-origin              → POST / Origin: http://<addr>          → 200 backend
//     2: 2-cross-origin             → POST / Origin: https://evil.test     → 403 "Invalid origin"
//     3: 3-additional-origins-match → POST / Origin: https://app.example.test → 200 backend
//     4: 4-no-source                → POST / (no Origin, no Referer)        → 403 "Invalid origin"
//     5: 5-referer-fallback         → POST / Referer: http://<addr>/somepage → 200 backend
//     6: 7a-per-route-allow         → POST /route-only Origin: https://route-only.test → 200 backend
//     7: 7b-per-route-listener-reject → POST / Origin: https://route-only.test → 403 "Invalid origin"
//
//     Per-probe log line: `probe <id> status=<code> body=<quoted>` followed by
//     the 4-header set (lowercase wire-form, sorted) for 403 responses only.
//     The Date header value is allow-listed (rendered as `<allow-listed>`)
//     since it varies per run. 200 responses skip the header dump because
//     backend-set headers (content-type/content-length/date) are uninteresting
//     for the differential.
//
//  3. AssertStats scrapes /stats/prometheus from both admin endpoints and
//     asserts the three csrf.* counter values per SPEC §7.1 final-state matrix:
//     csrf.request_valid          = 4 (scenarios 1, 3, 5, 7a)
//     csrf.request_invalid        = 2 (scenarios 2, 7b)
//     csrf.missing_source_origin  = 1 (scenario 4)
//
//     Per §11.9 amendment: per-route counter increments AGGREGATE under the
//     SAME *filterStats pointer (one counter series per HCM scope) — diverges
//     from phase 11 ADR-0117 INDEPENDENT-stats precedent. The driver asserts
//     a single set of three counter values, NOT a per-route split.
//
//  4. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner step 9.
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

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0014-http-csrf"

	// In-container reference Envoy listener port (pre-assigned per bootstrap).
	// Convention `100NN` for fixture `00NN`: phase 11 uses 10013-10016, phase 10
	// uses 10012, phase 09 uses 10011 — phase 12 follows with 10014 for the
	// single l_main listener.
	refContainerListenerPort = 10014

	// statPrefix matches the YAML's HCM stat_prefix (ingress_csrf). Reference
	// Envoy + envoy-go both flatten `http.<sp>.csrf.<metric>` to the
	// Prometheus form `envoy_http_csrf_<metric>{envoy_http_conn_manager_prefix="<sp>"}`
	// per ADR-0061 SN2 (HCM-namespace rule: `<sp>` is extracted as the
	// `envoy_http_conn_manager_prefix` label, NOT part of the metric name).
	statPrefix = "ingress_csrf"
)

func init() {
	fixture.RegisterFixture(fixtureName, &csrfDriver{})
}

type csrfDriver struct{}

// --- fixture.Driver (required) ---

func (csrfDriver) BackendCount() int                { return 1 }
func (csrfDriver) BackendKind() fixture.BackendKind { return fixture.HTTPCsrf }
func (csrfDriver) SubjectListenerName() string      { return "l_main" }
func (csrfDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal +
// runner-allocated backend port. Reference Envoy admin + listener ports are
// pre-assigned constants (9901, 10014).
func (csrfDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    9901,
		"ListenerPort": refContainerListenerPort,
		"BackendPort":  backendPorts[0],
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + backend port (loopback).
func (csrfDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
	})
}

// DriveReference + DriveSubject issue the identical 7-probe sequence and
// return the per-probe assertion-log byte stream. CompareBytes passes when
// both sides produce identical logs.
func (d csrfDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return driveProxy(ctx, addr)
}

func (d csrfDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return driveProxy(ctx, addr)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin-diff at runner step 9.
func (csrfDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- core drive logic ---

// scenario describes one differential probe.
type scenario struct {
	id         string
	method     string
	path       string
	originHdr  string // "" → header omitted
	refererHdr string // "" → header omitted
}

// scenarios returns the 7 probe descriptors. listenerHost is the proxy's
// listener address (host:port) — used to construct same-origin Origin /
// Referer header values for scenarios 1 and 5. Per SPEC §7.1, the csrf
// filter's "same-origin" check compares the source's host[:port] form
// against the request's Host header — using the dialed listener address as
// the Origin guarantees a same-origin match on both proxies.
func scenarios(listenerHost string) []scenario {
	return []scenario{
		{id: "1-same-origin", method: "POST", path: "/", originHdr: "http://" + listenerHost},
		{id: "2-cross-origin", method: "POST", path: "/", originHdr: "https://evil.test"},
		{id: "3-additional-origins-match", method: "POST", path: "/", originHdr: "https://app.example.test"},
		{id: "4-no-source", method: "POST", path: "/"},
		{id: "5-referer-fallback", method: "POST", path: "/", refererHdr: "http://" + listenerHost + "/somepage"},
		{id: "7a-per-route-allow", method: "POST", path: "/route-only", originHdr: "https://route-only.test"},
		{id: "7b-per-route-listener-reject", method: "POST", path: "/", originHdr: "https://route-only.test"},
	}
}

// driveProxy issues the 7 probes against addr and returns deterministic-format
// assertion-log lines. The "side" (ref vs subj) is INTENTIONALLY excluded from
// the log so both sides produce identical byte streams when behavior is
// equivalent.
//
// Per-probe encoding:
//
//	probe <id> status=<code> body=<quoted>
//	  header: <name>: <value>           (only on non-200 responses; sorted)
//	  ...
//
// Headers emitted on non-200 responses: lowercase wire-form, sorted; the
// 4-header set per SPEC §7.1 scenario 2 (content-length, content-type,
// date, server). The `date` value is scrubbed to `<allow-listed>` (varies
// per run); other values are emitted verbatim.
func driveProxy(ctx context.Context, addr string) ([]byte, error) {
	var b bytes.Buffer
	for _, s := range scenarios(addr) {
		var hdrs http.Header
		if s.originHdr != "" || s.refererHdr != "" {
			hdrs = http.Header{}
			if s.originHdr != "" {
				hdrs.Set("Origin", s.originHdr)
			}
			if s.refererHdr != "" {
				hdrs.Set("Referer", s.refererHdr)
			}
		}
		resp, body, err := helpers.HTTPRoundTrip(ctx, addr, s.method, s.path, hdrs, nil)
		if err != nil {
			fmt.Fprintf(&b, "probe %s ERROR: %v\n", s.id, err)
			continue
		}
		fmt.Fprintf(&b, "probe %s status=%d body=%q\n", s.id, resp.StatusCode, string(body))
		// Emit the response-header allow-list ONLY on non-200 responses
		// (scenarios 2, 4, 7b). 200 responses (scenarios 1, 3, 5, 7a) come
		// from the backend with run-varying Date / Content-Length-of-fixed
		// body / Server: backend headers — uninteresting for the differential
		// and asserted via the body byte-equality on its own.
		if resp.StatusCode != 200 {
			emitInvalidOriginHeaders(&b, resp.Header)
		}
	}
	return b.Bytes(), nil
}

// emitInvalidOriginHeaders emits the 4-header response set on a 403 "Invalid
// origin" reply per SPEC §7.1 scenario 2. Headers are emitted in sorted
// order, lowercased; the Date header value is allow-listed (rendered as
// `<allow-listed>`) since it varies per run. All other values are emitted
// verbatim.
//
// Connection / Transfer-Encoding / X-* headers are skipped to keep the
// assertion stable across the two proxies (reference Envoy may emit
// connection: close, x-envoy-upstream-service-time, etc., that envoy-go
// may not — those would surface as a CompareBytes diff that is NOT a real
// behavioral divergence).
func emitInvalidOriginHeaders(b *bytes.Buffer, headers http.Header) {
	type kv struct{ name, value string }
	var pairs []kv
	for name, values := range headers {
		lname := strings.ToLower(name)
		switch lname {
		case "content-length", "content-type", "date", "server":
			// in allow-list
		default:
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

// --- fixture.StatsAsserter ---

// AssertStats scrapes /stats/prometheus from both admin endpoints and asserts
// the csrf counter values per SPEC §7.1 final-state matrix:
//
//	envoy_http_csrf_request_valid         = 4 (scenarios 1, 3, 5, 7a)
//	envoy_http_csrf_request_invalid       = 2 (scenarios 2, 7b)
//	envoy_http_csrf_missing_source_origin = 1 (scenario 4)
//
// Per §11.9 amendment + ADR-0124: per-route counter increments AGGREGATE under
// the SAME *filterStats pointer (one counter series per HCM scope) — diverges
// from phase 11 ADR-0117 INDEPENDENT-stats precedent. We assert a single set
// of three counter values, NOT a per-route split.
func (csrfDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	expected := map[string]int64{
		"envoy_http_csrf_request_valid":         4,
		"envoy_http_csrf_request_invalid":       2,
		"envoy_http_csrf_missing_source_origin": 1,
	}
	refStats, err := scrapeCsrfStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref csrf stats: %v", err)
	}
	subjStats, err := scrapeCsrfStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj csrf stats: %v", err)
	}
	for name, want := range expected {
		if got := refStats[name]; got != want {
			t.Errorf("ref %s = %d, want %d", name, got, want)
		}
		if got := subjStats[name]; got != want {
			t.Errorf("subj %s = %d, want %d", name, got, want)
		}
	}
}

// --- stats scraping ---

// scrapeCsrfStats issues GET /stats/prometheus against adminAddr and parses the
// body into a map[name]int64 of all envoy_http_csrf_* metric values whose
// envoy_http_conn_manager_prefix label matches the fixture's configured
// stat_prefix (= "ingress_csrf"). Counters are int64. Names absent from the
// response are absent from the map (caller treats absent as 0).
func scrapeCsrfStats(adminAddr string) (map[string]int64, error) {
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
	return parseCsrfPromBody(resp.Body)
}

// parseCsrfPromBody parses a Prometheus text-format body and returns a map of
// all metric names beginning with envoy_http_csrf_ whose
// envoy_http_conn_manager_prefix label matches statPrefix (= "ingress_csrf").
// The label-bearing form `name{k="v",...} value` and the bare form
// `name value` are both supported. Non-csrf lines and lines with mismatched
// stat_prefix labels are silently ignored.
func parseCsrfPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantPrefix = "envoy_http_csrf_"
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
		// stat_prefix discrimination: only accept lines whose
		// envoy_http_conn_manager_prefix label matches the fixture's
		// configured stat_prefix. Lines without labels are also accepted (in
		// case envoy-go's stats SN2 transform emits a flat-name form).
		if labelStr != "" && !labelMatches(labelStr, "envoy_http_conn_manager_prefix", statPrefix) {
			continue
		}
		// Strip optional timestamp.
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		f, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		out[name] = int64(f)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("scan: %w", err)
	}
	return out, nil
}

// labelMatches reports whether labelStr (the contents of {...} in a
// Prometheus exposition line) contains key="value" exactly.
func labelMatches(labelStr, key, value string) bool {
	want := key + `="` + value + `"`
	for _, part := range strings.Split(labelStr, ",") {
		if strings.TrimSpace(part) == want {
			return true
		}
	}
	return false
}

// --- helpers ---

// fixtureDir returns the absolute path to the 0014-http-csrf fixture root,
// derived from runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0014-http-csrf/driver/driver.go
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
	_ fixture.Driver           = (*csrfDriver)(nil)
	_ fixture.BackendKindAware = (*csrfDriver)(nil)
	_ fixture.StatsAsserter    = (*csrfDriver)(nil)
)
