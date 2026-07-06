// Package inputs registers the 0030-http-admission-control fixture with the
// differential runner per phase-23 SPEC §7.1 + AMEND-2 + Task 9 brief.
//
// # Fixture type
//
// CROSS-SIDE (RequiresReference=true): the runner spawns reference Envoy
// v1.37.2 + envoy-go, drives both sides via DriveReference/DriveSubject,
// and performs CompareBytes on the resulting byte streams. The cross-side
// byte-exact guarantee relies on AMEND-2 RNG-independence at P_reject=0:
// when the backend always returns HTTP 200, the success-rate window
// accumulates only successes ⇒ P=0 ⇒ the integer-modulo decision
// `0 > (r % 1e4)` is false for every r ⇒ every request admitted on BOTH
// sides without any RNG dependency.
//
// # Assertion wiring (which interface asserts which scenario)
//
// Because this is a CROSS-SIDE fixture (live DriveReference + the runner's
// CompareBytes is load-bearing), the runner takes the cross-side path and
// NEVER calls SubjectAsserter.AssertSubject (that interface fires only on
// the reference-less path). The 4 scenarios are therefore wired to the
// interfaces the cross-side path actually invokes:
//
//	(a) parse_ok          — asserted by the runner's CompareBytes: both
//	                        sides emit the same "req1: status=200" line.
//	(b) all_admit_healthy — asserted by the runner's CompareBytes: the
//	                        5x status=200 lines are byte-exact on both sides.
//	(c) stat_surface      — asserted by StatsAsserter.AssertStats (step 10):
//	                        scrapes the SUBJECT admin /stats/prometheus,
//	                        asserts the 3 counters present under hcm_a,
//	                        rq_rejected==0, rq_failure==0, rq_success>0.
//	(d) pass_through_disabled — asserted by StatsAsserter.AssertStats:
//	                        dials the SUBJECT l_test_d (filter disabled),
//	                        asserts 200, then asserts hcm_d rq_rejected==0,
//	                        rq_success==0, rq_failure==0 (filter never records).
//
// # Byte stream format
//
// The probe stream emits only STATUS CODES (not headers or body) so that
// per-hop response headers added by Envoy (x-envoy-upstream-service-time,
// x-request-id, date, etc.) do not appear in the cross-side comparison.
// This mirrors fixture-0013's status-only emission discipline. The body is
// NOT emitted because reference Envoy forwards the request with extra headers
// that cause the echobackend (or any echo-style backend) to return a
// different body. The HTTPSlowStream backend (fixture-0010) returns a fixed
// body but the response headers still diverge. Status codes are the only
// comparable wire-shape element for the cross-side all-admit scenario.
//
// # 4 scenarios
//
//	(a) parse_ok          — single GET / → 200 (CROSS-SIDE byte-exact via
//	                        CompareBytes)
//	(b) all_admit_healthy — 5x GET / → all 200 (CROSS-SIDE byte-exact via
//	                        CompareBytes; P=0 RNG-independent per AMEND-2)
//	(c) stat_surface      — 3x GET / → all 200; subject admin /stats exposes
//	                        the 3 counters; rq_rejected==0, rq_failure==0,
//	                        rq_success>0 (StatsAsserter)
//	(d) pass_through_disabled — GET / on l_test_d → 200
//	                        (enabled.default_value=false; filter skipped;
//	                        hcm_d counters all stay 0) (StatsAsserter)
//
// # Listener topology
//
// Two listeners in ONE bootstrap to host the enabled config (scenarios a-c)
// and the disabled config (scenario d). A single bootstrap (no
// MultiListenerDriver) avoids the freeTCPPort combined-run flake per 22.2
// REVIEW §7.4 — a documented extension of SPEC §7.3's single-listener intent:
//
//	l_test_a — admission_control (full config, enabled=true) + router
//	           → c_backend. Scenarios (a)/(b)/(c).
//	l_test_d — admission_control (enabled.default_value=false) + router
//	           → c_backend. Scenario (d). Port = subjListenerPort + 1.
//
// # Cross-side byte-exact discipline
//
// Both DriveReference and DriveSubject emit "req_N: status=S" lines only
// (no headers, no body). Both sides admit all requests (P=0 guarantee per
// AMEND-2) and the backend returns HTTP 200 on both sides. The admin /stats
// scrape is NOT emitted in the cross-side stream — the (c)/(d) stat checks
// run subject-only via StatsAsserter. Scenario (d) emits a placeholder on
// both sides so the cross-side byte stream stays identical.
//
// # Reference container ports
//
//	refAdminPort  = 9901 (standard; harness waits on this port)
//	refLATestPort = 10030 (l_test_a in reference container)
//	refLDTestPort = 10031 (l_test_d in reference container)
//
// # Cross-references
//
//   - SPEC §7.1 (4-scenario matrix)
//   - SPEC §7.3 (single-listener intent; this fixture uses two listeners in
//     one bootstrap to host the enabled + disabled configs — see topology note)
//   - AMEND-2 (P=0 RNG-independence ⇒ all-admit ⇒ cross-side byte-exact)
//   - AMEND-3 (3-counter stat surface: rq_rejected/rq_success/rq_failure)
//   - AMEND-4 (enabled absent ⇒ true; enabled.default_value=false ⇒ filter off)
//   - ADR-0010 (host.docker.internal for reference container cluster)
//   - fixture-0013 (status-only byte stream discipline for cross-side)
//   - fixture-0029 (BootRejectFixture precedent for 0031 sibling)
package inputs

import (
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
	"sync"
	"text/template"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const (
	fixtureName = "0030-http-admission-control"

	// Reference-container in-container listener ports. The harness exposes
	// these via testcontainers MappedPort.
	refAdminPort  = 9901
	refLATestPort = 10030 // l_test_a — scenarios (a)/(b)/(c)
	refLDTestPort = 10031 // l_test_d — scenario (d)
)

func init() {
	fixture.RegisterFixture(fixtureName, &acDriver{})
}

// acDriver carries per-driver lifecycle state. SubjectConfig stashes the
// runner-allocated subjLDPort so StatsAsserter.AssertStats can drive
// scenario (d) against the disabled l_test_d listener before scraping.
// The subject admin address used for the /stats scrape is passed to
// AssertStats by the runner (subjAdminAddr) — no stash needed for it.
type acDriver struct {
	mu sync.Mutex

	// subjLDPort is l_test_d's port (= subjListenerPort + 1 per template).
	// Stashed here so AssertStats can drive scenario (d).
	subjLDPort int
}

// --- fixture.Driver ---

func (*acDriver) BackendCount() int                { return 1 }
func (*acDriver) BackendKind() fixture.BackendKind { return fixture.HTTPAdmissionControl }
func (*acDriver) SubjectListenerName() string      { return "l_test_a" }
func (*acDriver) ReferenceListenerPort() int       { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with reference container ports and
// host.docker.internal backend address.
func (*acDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   refAdminPort,
		"LATestPort":  refLATestPort,
		"LDTestPort":  refLDTestPort,
		"BackendHost": "host.docker.internal",
		"BackendPort": backendPorts[0],
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated ports and loopback
// backend address. Stashes subjLDPort for AssertStats scenario (d).
func (d *acDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.mu.Lock()
	d.subjLDPort = subjListenerPort + 1
	d.mu.Unlock()

	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"LATestPort":  subjListenerPort,
		"LDTestPort":  subjListenerPort + 1,
		"BackendPort": backendPorts[0],
	})
}

// DriveReference issues the 4-scenario status-only probe sequence against
// reference Envoy. Only status codes are emitted (no headers, no body) to
// avoid per-hop header divergence in the cross-side CompareBytes.
// Admin /stats NOT scraped (subject-only).
// Scenario (d) emits a placeholder (both sides emit the same placeholder).
func (*acDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return driveScenarios(ctx, addr)
}

// DriveSubject issues the 4-scenario status-only probe sequence against
// envoy-go. Mirrors DriveReference exactly. AssertStats handles the
// stats scrape + scenario (d) real dial separately.
func (d *acDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return driveScenarios(ctx, addr)
}

// driveScenarios runs the (a)/(b)/(c) status-only probe sequence on l_test_a
// and returns the byte stream. Scenario (d) is a placeholder here (the real
// l_test_d dial + counter check runs subject-only in AssertStats). Only status
// codes are emitted so cross-side CompareBytes is stable regardless of per-hop
// header differences.
func driveScenarios(ctx context.Context, addrA string) ([]byte, error) {
	var buf bytes.Buffer

	// Scenario (a) parse_ok — single GET / on l_test_a.
	{
		fmt.Fprintf(&buf, "=== scenario_a_parse_ok ===\n")
		status, err := doRequestStatus(ctx, "http://"+addrA+"/")
		if err != nil {
			fmt.Fprintf(&buf, "req1: error=%s\n", err)
		} else {
			fmt.Fprintf(&buf, "req1: status=%d\n", status)
		}
	}

	// Scenario (b) all_admit_healthy — 5x GET / on l_test_a.
	// P_reject=0 for healthy backend ⇒ all-admit, RNG-independent per AMEND-2.
	// Both sides emit identical "req_N: status=200" lines.
	{
		fmt.Fprintf(&buf, "=== scenario_b_all_admit_healthy ===\n")
		for i := 1; i <= 5; i++ {
			status, err := doRequestStatus(ctx, "http://"+addrA+"/")
			if err != nil {
				fmt.Fprintf(&buf, "req%d: error=%s\n", i, err)
			} else {
				fmt.Fprintf(&buf, "req%d: status=%d\n", i, status)
			}
		}
	}

	// Scenario (c) stat_surface — 3x GET / on l_test_a.
	// Status only; stats assertion is subject-only in AssertStats.
	{
		fmt.Fprintf(&buf, "=== scenario_c_stat_surface ===\n")
		for i := 1; i <= 3; i++ {
			status, err := doRequestStatus(ctx, "http://"+addrA+"/")
			if err != nil {
				fmt.Fprintf(&buf, "req%d: error=%s\n", i, err)
			} else {
				fmt.Fprintf(&buf, "req%d: status=%d\n", i, status)
			}
		}
	}

	// Scenario (d) pass_through_disabled — placeholder in cross-side stream.
	// The actual scenario (d) assertion (dial l_test_d, assert 200,
	// assert hcm_d counters stay 0) runs subject-only in AssertStats.
	// Both ref and subj emit the same placeholder ⇒ CompareBytes stays byte-exact.
	{
		fmt.Fprintf(&buf, "=== scenario_d_pass_through_disabled ===\n")
		fmt.Fprintf(&buf, "placeholder: subject-only via AssertStats\n")
	}

	return buf.Bytes(), nil
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin diff.
func (*acDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
	refBytes, err := helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref ready: %w", err)
	}
	subjBytes, err := helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj ready: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.StatsAsserter ---

// AssertStats performs the subject-only (c)/(d) assertions after the runner's
// cross-side CompareBytes + admin diff (which already cover (a)/(b) byte-exact
// status). The runner invokes this on the cross-side path (step 10) with both
// admin addresses; only the subject is inspected here:
//
//	(c) stat_surface: scrape the SUBJECT /stats/prometheus; assert the 3
//	    counters are present under hcm_a; rq_rejected==0, rq_failure==0,
//	    rq_success>0 (9 healthy requests were driven on l_test_a).
//	(d) pass_through_disabled: dial the SUBJECT l_test_d (filter disabled),
//	    assert 200, then assert hcm_d rq_rejected==0, rq_success==0,
//	    rq_failure==0 (a disabled filter never records).
//
// refAdminAddr is unused: the cross-side leg is the byte stream + admin diff;
// the counter checks are subject-only (the reference container's
// admission_control counters are not part of the contract this fixture pins).
func (d *acDriver) AssertStats(t fixture.TB, _ /*refAdminAddr*/, subjAdminAddr string) {
	t.Helper()

	d.mu.Lock()
	subjLDPort := d.subjLDPort
	d.mu.Unlock()

	// Scenario (d) FIRST: drive l_test_d so the disabled-pass-through path is
	// exercised before we scrape (the disabled filter must not record, so the
	// hcm_d counters must remain 0 even after a real request flows through).
	if subjLDPort != 0 {
		ctxD, cancelD := context.WithTimeout(context.Background(), 10*time.Second)
		addrD := fmt.Sprintf("127.0.0.1:%d", subjLDPort)
		status, err := doRequestStatus(ctxD, "http://"+addrD+"/")
		cancelD()
		if err != nil {
			t.Errorf("scenario d_pass_through_disabled: request error: %v", err)
		} else if status != 200 {
			t.Errorf("scenario d_pass_through_disabled: want status 200, got %d", status)
		}
	}

	// Scrape the SUBJECT admin /stats/prometheus once for (c) + (d).
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statsBody, err := scrapeStats(ctx, subjAdminAddr)
	if err != nil {
		t.Errorf("scenario c_stat_surface: scrape /stats/prometheus: %v", err)
		return
	}
	statsOut := string(statsBody)

	// (c) All 3 counters must be present per AMEND-3.
	for _, statName := range []string{"rq_rejected", "rq_success", "rq_failure"} {
		if !strings.Contains(statsOut, "envoy_http_admission_control_"+statName) {
			t.Errorf("scenario c_stat_surface: stat %q not found in /stats/prometheus output", statName)
		}
	}
	// (c) After 9 healthy requests on l_test_a: rq_rejected=0, rq_failure=0,
	// rq_success>0 (the positivity check the spec reviewer flagged as missing).
	requireStatIsZero(t, statsOut, "hcm_a", "rq_rejected")
	requireStatIsZero(t, statsOut, "hcm_a", "rq_failure")
	requireStatIsPositive(t, statsOut, "hcm_a", "rq_success")

	// (d) Disabled listener hcm_d: all 3 counters stay 0 (filter never records,
	// even though a real request flowed through l_test_d above).
	requireStatIsZero(t, statsOut, "hcm_d", "rq_rejected")
	requireStatIsZero(t, statsOut, "hcm_d", "rq_success")
	requireStatIsZero(t, statsOut, "hcm_d", "rq_failure")
}

// --- helpers ---

// doRequestStatus issues one GET request and returns only the HTTP status code.
func doRequestStatus(ctx context.Context, url string) (int, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{
		Transport: tr,
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

// scrapeStats fetches /stats/prometheus from the subject admin endpoint.
func scrapeStats(ctx context.Context, adminAddr string) ([]byte, error) {
	url := "http://" + adminAddr + "/stats/prometheus"
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("scrape /stats/prometheus: status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// statValue returns the Prometheus value for the named admission_control stat
// under the given HCM prefix, or (0, false) if no matching line is present
// (absent ≡ 0 for counters). The boolean reports presence, so callers can
// distinguish "absent" from "present and zero" when needed.
func statValue(t fixture.TB, statsOut, hcmPrefix, statName string) (float64, bool) {
	t.Helper()
	needle := fmt.Sprintf("envoy_http_admission_control_%s", statName)
	for _, line := range strings.Split(statsOut, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, needle) {
			continue
		}
		if !strings.Contains(line, fmt.Sprintf(`envoy_http_conn_manager_prefix="%s"`, hcmPrefix)) {
			continue
		}
		lastSpace := strings.LastIndex(line, " ")
		if lastSpace < 0 {
			continue
		}
		valStr := strings.TrimSpace(line[lastSpace+1:])
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			t.Errorf("stat %s/%s: could not parse value %q", hcmPrefix, statName, valStr)
			return 0, false
		}
		return val, true
	}
	return 0, false
}

// requireStatIsPositive asserts that the named stat under the given HCM prefix
// is present in the Prometheus output AND has a value > 0. Absent (≡ 0) fails.
func requireStatIsPositive(t fixture.TB, statsOut, hcmPrefix, statName string) {
	t.Helper()
	val, present := statValue(t, statsOut, hcmPrefix, statName)
	if !present {
		t.Errorf("stat %s/%s: want > 0, but stat is absent (≡ 0)", hcmPrefix, statName)
		return
	}
	if val <= 0 {
		t.Errorf("stat %s/%s: want > 0, got %v", hcmPrefix, statName, val)
	}
}

// requireStatIsZero asserts that the named stat under the given HCM prefix
// has value 0 in the Prometheus stats output, or is absent (≡ 0).
func requireStatIsZero(t fixture.TB, statsOut, hcmPrefix, statName string) {
	t.Helper()
	val, present := statValue(t, statsOut, hcmPrefix, statName)
	if present && val != 0 {
		t.Errorf("stat %s/%s: want 0, got %v", hcmPrefix, statName, val)
	}
	// Absent ≡ 0 for counters: pass.
}

// fixtureDir returns the absolute path to the test/fixtures/0030-http-
// admission-control/ directory (the parent of inputs/).
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(thisFile))
}

func mustReadFixtureFile(name string) string {
	path := filepath.Join(fixtureDir(), name)
	b, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("driver: read %s: %v", name, err))
	}
	return string(b)
}

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
	_ fixture.Driver           = (*acDriver)(nil)
	_ fixture.BackendKindAware = (*acDriver)(nil)
	_ fixture.StatsAsserter    = (*acDriver)(nil)
)
