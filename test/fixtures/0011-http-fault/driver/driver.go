// Package driver registers the 0011-http-fault fixture with the differential
// runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.fault and reference Envoy v1.37.2 across the four-scenario
// matrix per phase 09 SPEC §7.1.
//
// Integration shape (SPEC §7.3 driver outline):
//
//  1. ReferenceBootstrap renders test/fixtures/0011-http-fault/envoy.yaml with
//     the backend host set to host.docker.internal (ADR-0010 STRICT_DNS) +
//     runner-allocated backend port. SubjectConfig renders envoy-go.yaml with
//     the runner-allocated subject ports + backend port (loopback).
//
//  2. DriveReference / DriveSubject issue an identical 8-probe sequence against
//     each proxy and emit a deterministic per-probe assertion-log byte stream.
//     The runner's CompareBytes pass enforces equivalence — when both proxies
//     produce equal logs, the differential gate fires.
//
//     The 8 probes cover the four scenarios per SPEC §7.1:
//     1: scenario1 → /scenario1/anything            (listener-level inheritance)
//     2: scenario2 → /scenario2/anything            (combined delay+abort override)
//     3: scenario3-wholesale → /scenario3-wholesale (wholesale-override demo)
//     4: scenario3-baseline → /scenario3-baseline   (listener-level baseline)
//     5: scenario4-a → /scenario4 (no headers)      (no match → backend 200)
//     6: scenario4-b → /scenario4 (x-fault-on: yes) (match → 503 abort)
//     7: scenario4-c → /scenario4 (X-FAULT-ON: yes) (case-insensitive name match → 503)
//     8: scenario4-d → /scenario4 (x-fault-on: YES) (case-sensitive value MISS → 200)
//
//     Per-probe log line: `probe <id> status=<code> body=<quoted> elapsed=<bucket>`
//     where bucket is "fast" (<80ms) or "delayed" (>=80ms). Status text is NOT
//     emitted (planner-time decision 7: status-text allow-listed for non-stdlib
//     codes like 418); only the numeric code is asserted.
//
//  3. AssertStats scrapes /stats/prometheus from both admin endpoints and
//     asserts the five fault.* stat values per SPEC §7.1 final-state matrix:
//     fault.aborts_injected     = 4 (scenarios 2, 3a, 4b, 4c)
//     fault.delays_injected     = 3 (scenarios 1, 2, 3b)
//     fault.faults_overflow     = 0
//     fault.active_faults       = 0 (final, after all faults complete)
//     fault.response_rl_injected = 0 (permanently zero per ADR-0107 route A)
//
//  4. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner
//     step 9.
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
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName              = "0011-http-fault"
	refContainerListenerPort = 10001
	// fastDelayThresholdMs is the timing-bucket boundary (planner-time decision
	// 11). The listener-level delay is configured at 100ms; ~80ms separates the
	// no-delay path (~5ms) from the delay path (~105ms wall-clock) with
	// comfortable margin for CI scheduling jitter (±10ms tolerance per §13.3).
	fastDelayThresholdMs = 80
	// statPrefix matches the YAML's HCM stat_prefix (ingress_http). Reference
	// Envoy + envoy-go both flatten `http.<sp>.fault.<metric>` to the
	// Prometheus form `envoy_http_fault_<metric>{envoy_http_conn_manager_prefix="<sp>"}`
	// per ADR-0061 SN2 (HCM-namespace rule: `<sp>` is extracted as the
	// `envoy_http_conn_manager_prefix` label, NOT part of the metric name) +
	// the SN2 internal-dot transform (Phase 09 / Task 14 follow-up: nested
	// rest's `.` → `_` for Prometheus name-grammar compliance).
	statPrefix = "ingress_http"
)

func init() {
	fixture.RegisterFixture(fixtureName, &faultDriver{})
}

type faultDriver struct{}

func (faultDriver) BackendCount() int                { return 1 }
func (faultDriver) BackendKind() fixture.BackendKind { return fixture.HTTPFault }
func (faultDriver) SubjectListenerName() string      { return "l_main" }
func (faultDriver) ReferenceListenerPort() int       { return refContainerListenerPort }

// ReferenceBootstrap renders test/fixtures/0011-http-fault/envoy.yaml with the
// backend host (= host.docker.internal per ADR-0010) and the runner-allocated
// port.
func (faultDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"BackendHost": "host.docker.internal",
		"BackendPort": backendPorts[0],
	})
}

// SubjectConfig renders test/fixtures/0011-http-fault/envoy-go.yaml with the
// runner-allocated subject admin/listener ports + backend port (loopback).
func (faultDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendHost":  "127.0.0.1",
		"BackendPort":  backendPorts[0],
	})
}

// DriveReference + DriveSubject issue the identical 8-probe sequence and
// return the per-probe assertion-log byte stream. CompareBytes passes when
// both sides produce identical logs.
func (d faultDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr)
}

func (d faultDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr)
}

// driveProxy issues the 8 probes against addr and returns deterministic-format
// assertion-log lines. The "side" (ref vs subj) is INTENTIONALLY excluded from
// the log lines so the two sides produce identical byte streams when behavior
// is equivalent.
func (faultDriver) driveProxy(ctx context.Context, addr string) ([]byte, error) {
	type probe struct {
		id      string
		path    string
		headers map[string]string
	}
	probes := []probe{
		{"scenario1", "/scenario1/anything", nil},
		{"scenario2", "/scenario2/anything", nil},
		{"scenario3-wholesale", "/scenario3-wholesale/anything", nil},
		{"scenario3-baseline", "/scenario3-baseline/anything", nil},
		{"scenario4-a", "/scenario4/anything", nil},
		{"scenario4-b", "/scenario4/anything", map[string]string{"x-fault-on": "yes"}},
		{"scenario4-c", "/scenario4/anything", map[string]string{"X-FAULT-ON": "yes"}},
		{"scenario4-d", "/scenario4/anything", map[string]string{"x-fault-on": "YES"}},
	}
	var out bytes.Buffer
	for _, p := range probes {
		status, body, elapsed, err := httpProbe(ctx, addr, p.path, p.headers)
		if err != nil {
			fmt.Fprintf(&out, "probe %s ERROR: %v\n", p.id, err)
			continue
		}
		// Per planner-time decision 7: status-text allow-list — log status code
		// only (status TEXT diverges between proxies for non-stdlib codes like
		// 418, where the abort-injected response synthesizes its own status
		// text). Body is asserted byte-equal across sides; elapsed is bucketed
		// (fast vs delayed) at the 80ms midpoint between the no-delay path
		// (~5ms) and the inherited-delay path (~105ms wall-clock). A timing
		// flake here would surface as a CompareBytes diff with the bucket
		// strings differing — clearer than a missed assertion.
		bucket := "fast"
		if elapsed >= fastDelayThresholdMs*time.Millisecond {
			bucket = "delayed"
		}
		fmt.Fprintf(&out, "probe %s status=%d body=%q elapsed=%s\n", p.id, status, body, bucket)
	}
	return out.Bytes(), nil
}

// AssertStats per SPEC §7.1 stat-delta matrix. Scrapes both proxies'
// /stats/prometheus and asserts the 5 fault.* counters match the expected
// final-state values (sum across the 8-probe sequence).
//
// Stat names: per SPEC §11.6 empirical pin + ADR-0061 SN2, the Envoy-domain
// stat name `http.<stat_prefix>.fault.<n>` flattens to Prometheus name
// `envoy_http_fault_<n>{envoy_http_conn_manager_prefix="<stat_prefix>"}` —
// the stat_prefix is extracted as a label, not embedded in the metric name.
// scrapeFaultStats keys by the bare metric name (already filtered to lines
// whose label set matches the configured stat_prefix).
func (faultDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()
	expected := map[string]int64{
		"envoy_http_fault_aborts_injected":      4, // scenarios 2, 3a, 4b, 4c
		"envoy_http_fault_delays_injected":      3, // scenarios 1, 2, 3b
		"envoy_http_fault_faults_overflow":      0,
		"envoy_http_fault_active_faults":        0, // final
		"envoy_http_fault_response_rl_injected": 0, // permanently zero per ADR-0107 route A
	}
	refStats, err := scrapeFaultStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref fault stats: %v", err)
	}
	subjStats, err := scrapeFaultStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj fault stats: %v", err)
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

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the standard admin-diff at runner step 9.
func (faultDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// ----------------------------------------------------------------------------
// Helpers (mirror the 0007a-cors / 0010-graceful-drain / 0004-h2 driver patterns).
// ----------------------------------------------------------------------------

// fixtureDir returns the absolute path to the 0011-http-fault fixture root,
// derived from runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0011-http-fault/driver/driver.go
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

// httpProbe issues GET path against addr (with optional headers), reads the
// full response, and returns (status, body, elapsed, error). Uses
// helpers.HTTPRoundTrip for the wire-level round-trip (handcrafted bufio
// + Connection: close per the existing 0007a-cors precedent).
func httpProbe(ctx context.Context, addr, path string, hdrs map[string]string) (int, []byte, time.Duration, error) {
	var h http.Header
	if len(hdrs) > 0 {
		h = http.Header{}
		for k, v := range hdrs {
			h.Set(k, v)
		}
	}
	start := time.Now()
	resp, body, err := helpers.HTTPRoundTrip(ctx, addr, "GET", path, h, nil)
	elapsed := time.Since(start)
	if err != nil {
		return 0, nil, elapsed, err
	}
	return resp.StatusCode, body, elapsed, nil
}

// scrapeFaultStats issues GET /stats/prometheus against adminAddr and parses
// the body into a map[name]int64 of all envoy_http_fault_* metric values
// whose envoy_http_conn_manager_prefix label matches the fixture's
// configured stat_prefix (= "ingress_http"). Counters and the active_faults
// gauge are both treated as int64. Names absent from the response are absent
// from the map (caller treats absent as 0 via zero-value lookup).
func scrapeFaultStats(adminAddr string) (map[string]int64, error) {
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
	return parseFaultPromBody(resp.Body)
}

// parseFaultPromBody parses a Prometheus text-format body and returns a map
// of all metric names beginning with envoy_http_fault_ whose
// envoy_http_conn_manager_prefix label matches statPrefix (= "ingress_http").
// The label-bearing form `name{k="v",...} value` and the bare form
// `name value` are both supported. Non-fault lines and lines with mismatched
// stat_prefix labels are silently ignored.
func parseFaultPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantPrefix = "envoy_http_fault_"
	sc := bufio.NewScanner(r)
	// Increase buffer size — Prometheus exposition lines can be long when
	// histograms are present (not in our 5-name set, but defensive).
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Determine where the name ends: at the first '{' (label form) or at
		// the last ' ' before the value (bare form). For the label-bearing
		// form, also extract the label string for stat_prefix matching.
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
		// case reference Envoy emits a flat-name form for some stats — see
		// 0005-prometheus-stats precedent).
		if labelStr != "" && !labelMatches(labelStr, "envoy_http_conn_manager_prefix", statPrefix) {
			continue
		}
		// Strip optional timestamp (Prometheus exposition allows `value timestamp`).
		if sp := strings.IndexByte(valueStr, ' '); sp >= 0 {
			valueStr = valueStr[:sp]
		}
		// Parse as float (Prometheus exposition encodes integers as floats),
		// then truncate to int64. The five fault metrics are integer-valued.
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

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*faultDriver)(nil)
	_ fixture.BackendKindAware = (*faultDriver)(nil)
	_ fixture.StatsAsserter    = (*faultDriver)(nil)
)
