// Package inputs registers the 0025-http-adaptive-concurrency fixture
// with the differential runner. Phase-21 IMPL Task 10.
//
// Per the Task 10 brief, this fixture is REFERENCE-LESS
// (`RequiresReference: false`), mirrors the phase-20 oauth2 fixture
// 0024 + phase-07.1 iteration-probe fixture 0007b single-directory
// precedent. The runner short-circuits the reference-proxy spawn +
// DriveReference + byte-stream CompareBytes; only DriveSubject + the
// SubjectAsserter run.
//
// The driver:
//
//  1. Stashes the runner-allocated subjAdminPort + subjListenerPort at
//     SubjectConfig() time so it can later scrape /stats and dial each
//     listener address by name.
//  2. Renders envoy-go.yaml with the per-run port substitutions for
//     the 3-listener topology (l_a_default + l_b_overflow +
//     l_d_disabled on consecutive ports subjListenerPort + 0/1/2).
//  3. Drives 4 scenarios sequentially per SPEC §7 + AMEND-6:
//     (a) parse_ok                — single GET / on l_a_default
//     (b) overflow_503            — 2 concurrent GET /slow on l_b_overflow
//     (c) stat_surface            — single GET / + admin /stats scrape
//     (d) pass_through_when_disabled — single GET / on l_d_disabled
//  4. SubjectAsserter validates per-scenario invariants against the
//     deterministic byte stream emitted by DriveSubject (mirrors the
//     0007b + 0024 encoded-probe pattern).
//
// The AMEND-6 cross-side byte-exact promise for scenario (b) is
// deferred to a future cross-side extension per Task 10 PROGRESS.md
// (RATIFIED-PENDING-FUTURE-CROSS-SIDE-EXTENSION); envoy-go-side byte-
// exact pinning of the 503 + 25-byte body + content-type: text/plain
// still lands here at scenario (b) per AMEND-6 wire-shape invariants —
// only the cross-side comparison is deferred.
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
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

const fixtureName = "0025-http-adaptive-concurrency"

func init() {
	fixture.RegisterFixture(fixtureName, &acDriver{})
}

// acDriver carries per-driver lifecycle state. SubjectConfig stashes
// the runner-allocated admin port so DriveSubject can later compose
// the admin /stats scrape URL (the per-listener addrs are recovered
// from the runner-supplied l_a_default addr via deriveAddrsFromBase
// per the phase-20 oauth2 deriveAddrsFromSubj precedent).
type acDriver struct {
	mu sync.Mutex

	// subjAdminPort is the admin endpoint port allocated by the runner
	// + passed to SubjectConfig. The driver scrapes /stats?format=
	// prometheus from this port at every scenario for the per-scenario
	// stat snapshot.
	subjAdminPort int
}

// --- fixture.Driver ---

func (*acDriver) BackendCount() int                { return 1 }
func (*acDriver) BackendKind() fixture.BackendKind { return fixture.HTTPAdaptiveConcurrency }
func (*acDriver) SubjectListenerName() string      { return "l_a_default" }
func (*acDriver) RequiresReference() bool          { return false }

// ReferenceListenerPort + ReferenceBootstrap are defensive stubs — the
// runner short-circuits these for reference-less fixtures. Returning
// zero / empty so any future runner refactor that inadvertently calls
// them surfaces immediately as a configuration error.
func (*acDriver) ReferenceListenerPort() int        { return 0 }
func (*acDriver) ReferenceBootstrap(_ []int) string { return "" }
func (*acDriver) DriveReference(context.Context, string) ([]byte, error) {
	return nil, nil
}

// ProbeAdmin returns nil/nil — the runner's reference-less branch does
// not invoke this hook.
func (*acDriver) ProbeAdmin(context.Context, string, string) ([]byte, []byte, error) {
	return nil, nil, nil
}

// SubjectConfig templates envoy-go.yaml with the per-run port
// substitutions for the 3-listener topology. Stashes subjAdminPort +
// subjListenerPort so DriveSubject can reach the admin endpoint and
// the +1/+2-offset listeners.
func (d *acDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.mu.Lock()
	d.subjAdminPort = subjAdminPort
	d.mu.Unlock()

	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":      subjAdminPort,
		"LADefaultPort":  subjListenerPort,
		"LBOverflowPort": subjListenerPort + 1,
		"LDDisabledPort": subjListenerPort + 2,
		"BackendPort":    backendPorts[0],
	})
}

// DriveSubject runs the 4 scenarios sequentially against the subject
// proxy. The runner-supplied addr argument is the l_a_default listener
// addr; the driver derives l_b_overflow + l_d_disabled by +1 / +2
// port offsets per the phase-20 oauth2 deriveAddrsFromSubj precedent.
func (d *acDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	addrA := addr
	addrB, addrD, err := deriveAddrsFromBase(addr)
	if err != nil {
		return nil, fmt.Errorf("derive listener addrs: %w", err)
	}

	d.mu.Lock()
	adminPort := d.subjAdminPort
	d.mu.Unlock()
	adminAddr := fmt.Sprintf("127.0.0.1:%d", adminPort)

	var buf bytes.Buffer

	// Scenario (a) parse_ok — single GET / on l_a_default.
	{
		fmt.Fprintf(&buf, "=== scenario a_parse_ok\n")
		res := doScenario(ctx, "GET", "http://"+addrA+"/", nil)
		emitProbeBody(&buf, res)
		// Stats snapshot (rq_blocked) — the driver scrapes /stats and
		// records the post-(a) rq_blocked value across ALL three listener
		// stat prefixes so the asserter can pin rq_blocked == 0 after
		// scenario (a). Naming convention via Rule SN2: internal
		// `http.<hcm>.adaptive_concurrency.gradient_controller.<stat>`
		// flattens to prometheus
		// `envoy_http_adaptive_concurrency_gradient_controller_<stat>`
		// with label `envoy_http_conn_manager_prefix="<hcm>"`.
		statsBody, serr := scrapeStats(ctx, adminAddr)
		if serr != nil {
			emitStatsError(&buf, serr)
		} else {
			emitStatsSnapshot(&buf, statsBody)
		}
	}

	// Scenario (b) overflow_503 — 2 concurrent GET /slow on l_b_overflow.
	// The slow-stream backend takes 5 seconds to respond; the first
	// request fills the in-flight=1 slot. The second request fires
	// ~100ms after the first to ensure the first has already passed
	// through DecodeHeaders + bumped numRqOutstanding to 1; the second
	// then hits forwardingDecision's Block leg (current=1 >= limit=1)
	// → 503 + body + content-type per AMEND-6.
	{
		type slowResult struct {
			label string
			res   scenarioResult
		}
		ch := make(chan slowResult, 2)
		// Fire request 1 in a goroutine — long-running.
		go func() {
			r := doScenario(ctx, "GET", "http://"+addrB+"/slow", nil)
			ch <- slowResult{label: "req1", res: r}
		}()
		// Give request 1 a small head start so its CAS on numRqOutstanding
		// lands before request 2's check. The HTTPSlowStream backend's
		// /slow handler streams over 5 seconds; 200ms is plenty for the
		// envoy-go DecodeHeaders dispatch to record the in-flight bump
		// before the second request's forwardingDecision runs.
		time.Sleep(200 * time.Millisecond)
		go func() {
			r := doScenario(ctx, "GET", "http://"+addrB+"/slow", nil)
			ch <- slowResult{label: "req2", res: r}
		}()

		var req1, req2 scenarioResult
		var got1, got2 bool
		// Drain both responses (request 2 returns fast — 503 short-circuit;
		// request 1 returns after the 5-second slow stream completes).
		for i := 0; i < 2; i++ {
			r := <-ch
			switch r.label {
			case "req1":
				req1 = r.res
				got1 = true
			case "req2":
				req2 = r.res
				got2 = true
			}
		}
		if !got1 || !got2 {
			return nil, fmt.Errorf("scenario b: missing response label (got1=%v got2=%v)", got1, got2)
		}
		// Discriminator: among the two responses, ONE must be 200 and ONE
		// must be 503. We pin "first arrived" / "second arrived" by status
		// rather than by request-launch order so the assertions remain
		// deterministic even if the goroutine scheduler races (which it
		// will not in practice — the 200ms head-start guarantees ordering
		// — but the by-status pinning is robust against scheduler skew).
		var fast, slow scenarioResult
		switch {
		case req1.statusCode == 503 && req2.statusCode == 200:
			fast = req1
			slow = req2
		case req1.statusCode == 200 && req2.statusCode == 503:
			fast = req2
			slow = req1
		default:
			// Both 200 or both 503 — unexpected; emit both so the asserter
			// can fail with full context.
			fast = req1
			slow = req2
		}
		// Emit both response probes inside ONE scenario block so the
		// asserter can match the per-request status/body/header + the
		// per-listener stats snapshot in a single block lookup. The
		// "req200" + "req503" sub-tags partition the probe lines for
		// per-request assertions.
		fmt.Fprintf(&buf, "=== scenario b_overflow_503\n")
		fmt.Fprintf(&buf, "--- subprobe req200\n")
		emitProbeBody(&buf, slow)
		fmt.Fprintf(&buf, "--- subprobe req503\n")
		emitProbeBody(&buf, fast)
		// Stats snapshot after scenario (b) — rq_blocked should be 1 under
		// hcm_b_overflow.
		statsBody, serr := scrapeStats(ctx, adminAddr)
		if serr != nil {
			emitStatsError(&buf, serr)
		} else {
			emitStatsSnapshot(&buf, statsBody)
		}
	}

	// Scenario (c) stat_surface — single GET / on l_a_default + scrape.
	// The single fast request does not perturb the initial state
	// (concurrency_limit=3, min_rtt_calculation_active=1) because the
	// minRTT window is open at construction per AMEND-2 C4 and needs
	// 50 samples to close per request_count default.
	{
		fmt.Fprintf(&buf, "=== scenario c_stat_surface\n")
		res := doScenario(ctx, "GET", "http://"+addrA+"/", nil)
		emitProbeBody(&buf, res)
		statsBody, serr := scrapeStats(ctx, adminAddr)
		if serr != nil {
			emitStatsError(&buf, serr)
		} else {
			emitStatsSnapshot(&buf, statsBody)
		}
	}

	// Scenario (d) pass_through_when_disabled — single GET / on l_d_disabled.
	// decode_headers.go leg-1 fires; controller NEVER consulted; no
	// rq_blocked increment under hcm_d_disabled.
	{
		fmt.Fprintf(&buf, "=== scenario d_pass_through_when_disabled\n")
		res := doScenario(ctx, "GET", "http://"+addrD+"/", nil)
		emitProbeBody(&buf, res)
		statsBody, serr := scrapeStats(ctx, adminAddr)
		if serr != nil {
			emitStatsError(&buf, serr)
		} else {
			emitStatsSnapshot(&buf, statsBody)
		}
	}

	return buf.Bytes(), nil
}

// --- fixture.SubjectAsserter ---

// AssertSubject performs per-scenario assertions against the captured
// subject byte stream. Mirrors the 0007b + 0024 AssertSubject pattern
// (per-scenario block lookup via "=== scenario <id>" header + substring
// matching within the block). The stats snapshot is appended INSIDE
// the same scenario block as the request probe(s), so all assertions
// for one scenario share one block lookup.
func (d *acDriver) AssertSubject(t fixture.TB, subjBytes []byte) {
	t.Helper()
	out := string(subjBytes)

	// Scenario (a) parse_ok — single GET / → 200; rq_blocked under
	// hcm_a_default == 0.
	requireScenarioStatus(t, out, "a_parse_ok", 200)
	requireStatEquals(t, out, "a_parse_ok", "hcm_a_default", "rq_blocked", 0)

	// Scenario (b) overflow_503 — one 200, one 503 + byte-pinned wire
	// shape. The req200 + req503 subprobe tags partition the per-
	// request lines inside the single scenario block.
	requireSubprobeStatus(t, out, "b_overflow_503", "req200", 200)
	requireSubprobeStatus(t, out, "b_overflow_503", "req503", 503)
	requireSubprobeBodyExact(t, out, "b_overflow_503", "req503", "reached concurrency limit")
	requireSubprobeHeader(t, out, "b_overflow_503", "req503", "content-type", "text/plain")
	// rq_blocked under hcm_b_overflow == 1.
	requireStatEquals(t, out, "b_overflow_503", "hcm_b_overflow", "rq_blocked", 1)

	// Scenario (c) stat_surface — single GET / → 200; 7-name surface
	// present; concurrency_limit==3; min_rtt_calculation_active==1.
	requireScenarioStatus(t, out, "c_stat_surface", 200)
	statNames := []string{
		"rq_blocked",
		"concurrency_limit",
		"gradient",
		"burst_queue_size",
		"sample_rtt_msecs",
		"min_rtt_msecs",
		"min_rtt_calculation_active",
	}
	for _, n := range statNames {
		requireStatNamePresent(t, out, "c_stat_surface", "hcm_a_default", n)
	}
	requireStatEquals(t, out, "c_stat_surface", "hcm_a_default", "concurrency_limit", 3)
	requireStatEquals(t, out, "c_stat_surface", "hcm_a_default", "min_rtt_calculation_active", 1)

	// Scenario (d) pass_through_when_disabled — single GET / → 200;
	// rq_blocked under hcm_d_disabled == 0 (controller never consulted).
	requireScenarioStatus(t, out, "d_pass_through_when_disabled", 200)
	requireStatEquals(t, out, "d_pass_through_when_disabled",
		"hcm_d_disabled", "rq_blocked", 0)
}

// --- per-scenario assertion helpers ---

func scenarioBlock(out, scenario string) (string, bool) {
	header := "=== scenario " + scenario + "\n"
	idx := strings.Index(out, header)
	if idx < 0 {
		return "", false
	}
	blockStart := idx
	blockEnd := len(out)
	if next := strings.Index(out[blockStart+len(header):], "=== scenario "); next >= 0 {
		blockEnd = blockStart + len(header) + next
	}
	return out[blockStart:blockEnd], true
}

// subprobeBlock returns the portion of the scenario block delimited by
// "--- subprobe <tag>" + the next "--- subprobe" OR "stat:" / EOF
// (so the per-subprobe assertions don't accidentally match stats
// lines that follow the subprobes inside the same scenario block).
func subprobeBlock(out, scenario, subprobe string) (string, bool) {
	block, ok := scenarioBlock(out, scenario)
	if !ok {
		return "", false
	}
	header := "--- subprobe " + subprobe + "\n"
	idx := strings.Index(block, header)
	if idx < 0 {
		return "", false
	}
	sub := block[idx+len(header):]
	// End at next "--- subprobe" boundary OR the first "stat:" line
	// (stats are emitted AFTER all subprobes in the scenario block).
	end := len(sub)
	if next := strings.Index(sub, "--- subprobe "); next >= 0 && next < end {
		end = next
	}
	if statIdx := strings.Index(sub, "\nstat: "); statIdx >= 0 && statIdx+1 < end {
		end = statIdx + 1
	}
	return sub[:end], true
}

func requireScenarioStatus(t fixture.TB, out, scenario string, want int) {
	t.Helper()
	block, ok := scenarioBlock(out, scenario)
	if !ok {
		t.Errorf("scenario %s: probe header not found", scenario)
		return
	}
	want1 := fmt.Sprintf("status: %d", want)
	if !strings.Contains(block, want1) {
		t.Errorf("scenario %s: status mismatch — want %q; block:\n%s", scenario, want1, block)
	}
}

func requireSubprobeStatus(t fixture.TB, out, scenario, subprobe string, want int) {
	t.Helper()
	block, ok := subprobeBlock(out, scenario, subprobe)
	if !ok {
		t.Errorf("scenario %s subprobe %s: header not found", scenario, subprobe)
		return
	}
	wantLine := fmt.Sprintf("status: %d", want)
	if !strings.Contains(block, wantLine) {
		t.Errorf("scenario %s subprobe %s: status mismatch — want %q; subblock:\n%s",
			scenario, subprobe, wantLine, block)
	}
}

func requireSubprobeBodyExact(t fixture.TB, out, scenario, subprobe, want string) {
	t.Helper()
	block, ok := subprobeBlock(out, scenario, subprobe)
	if !ok {
		t.Errorf("scenario %s subprobe %s: header not found", scenario, subprobe)
		return
	}
	wantLine := fmt.Sprintf("body: %q", want)
	if !strings.Contains(block, wantLine) {
		t.Errorf("scenario %s subprobe %s: body mismatch — want %q; subblock:\n%s",
			scenario, subprobe, wantLine, block)
	}
}

func requireSubprobeHeader(t fixture.TB, out, scenario, subprobe, headerKey, headerVal string) {
	t.Helper()
	block, ok := subprobeBlock(out, scenario, subprobe)
	if !ok {
		t.Errorf("scenario %s subprobe %s: header not found", scenario, subprobe)
		return
	}
	wantLine := "header: " + strings.ToLower(headerKey) + ": " + headerVal
	if !strings.Contains(block, wantLine) {
		t.Errorf("scenario %s subprobe %s: header %s: %s not found; subblock:\n%s",
			scenario, subprobe, headerKey, headerVal, block)
	}
}

// requireStatNamePresent verifies that the named stat appears in the
// scenario's stats snapshot block under the given HCM prefix
// (emitStatsSnapshot writes lines of the form "stat: <hcm_prefix>
// <bare_name> <value>").
func requireStatNamePresent(t fixture.TB, out, scenario, hcmPrefix, statName string) {
	t.Helper()
	block, ok := scenarioBlock(out, scenario)
	if !ok {
		t.Errorf("scenario %s: probe header not found", scenario)
		return
	}
	want := "stat: " + hcmPrefix + " " + statName + " "
	if !strings.Contains(block, want) {
		t.Errorf("scenario %s: stat %s/%s not present; block:\n%s",
			scenario, hcmPrefix, statName, block)
	}
}

// requireStatEquals verifies that the named stat appears with the
// expected integer value in the scenario's stats snapshot block under
// the given HCM prefix.
func requireStatEquals(t fixture.TB, out, scenario, hcmPrefix, statName string, want int64) {
	t.Helper()
	block, ok := scenarioBlock(out, scenario)
	if !ok {
		t.Errorf("scenario %s: probe header not found", scenario)
		return
	}
	wantLine := fmt.Sprintf("stat: %s %s %d", hcmPrefix, statName, want)
	if !strings.Contains(block, wantLine) {
		t.Errorf("scenario %s: stat %s/%s != %d; block:\n%s",
			scenario, hcmPrefix, statName, want, block)
	}
}

// --- helpers ---

// scenarioResult captures one HTTP round-trip.
type scenarioResult struct {
	statusCode int
	body       []byte
	headers    http.Header
	err        error
}

func doScenario(ctx context.Context, method, url string, headers http.Header) scenarioResult {
	// Non-redirect-following client; KeepAlive disabled so each request
	// uses a fresh connection (the slow-stream concurrency trap relies
	// on the two requests occupying two distinct downstream connections
	// so HCM-level connection coalescing does not serialize them).
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{
		Transport: tr,
		Timeout:   30 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	req, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return scenarioResult{err: err}
	}
	for k, vv := range headers {
		for _, v := range vv {
			req.Header.Add(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return scenarioResult{err: err}
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return scenarioResult{err: err}
	}
	return scenarioResult{
		statusCode: resp.StatusCode,
		body:       respBody,
		headers:    resp.Header,
	}
}

// emitProbeBody renders one scenario response's wire shape into the
// byte stream WITHOUT a "=== scenario" header (the caller emits the
// header so multiple sub-probes can share one scenario block —
// scenario (b) overflow_503 uses one block with two sub-probes for
// the req200 + req503 responses). Mirrors 0024's emitProbe format
// minus the header: "status: N", "header: k: v", "body: %q" lines.
func emitProbeBody(buf *bytes.Buffer, res scenarioResult) {
	if res.err != nil {
		fmt.Fprintf(buf, "error: %s\n", res.err)
		return
	}
	fmt.Fprintf(buf, "status: %d\n", res.statusCode)
	// Headers sorted for determinism.
	var keys []string
	for k := range res.headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		for _, v := range res.headers[k] {
			fmt.Fprintf(buf, "header: %s: %s\n", strings.ToLower(k), v)
		}
	}
	fmt.Fprintf(buf, "body: %q\n", string(res.body))
}

// scrapeStats fetches /stats/prometheus from the subject admin endpoint
// (the only stats endpoint exposed by internal/admin/admin.go — per
// the phase-01 ADR-0061 admin-route roster: `/stats/prometheus` per
// `mux.HandleFunc("/stats/prometheus", handlePrometheus(...))`).
// Returns the raw body bytes.
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

// emitStatsSnapshot parses the prometheus-format stats body into a
// per-stat "stat: <hcm_prefix> <bare_name> <value>" line shape
// consumed by the requireStat* helpers. The prometheus format is one
// stat per line:
//
//	<name>{labels} <value>
//
// or
//
//	<name> <value>
//
// The adaptive_concurrency 7-name surface uses Rule SN2 internal-name
// flattening at internal/stats/name.go — the internal name
// `http.<hcm>.adaptive_concurrency.gradient_controller.<stat>`
// flattens to prometheus base
// `envoy_http_adaptive_concurrency_gradient_controller_<stat>` with
// label `envoy_http_conn_manager_prefix="<hcm>"`. The driver re-
// extracts the (hcm_prefix, bare_name, value) tuple so the asserter
// can pin per-listener stats.
//
// Multi-line scrape emissions (TYPE/HELP comments) starting with "#"
// are skipped. Lines NOT matching the adaptive_concurrency surface
// are skipped to keep the snapshot small + deterministic.
//
// Emits lines APPENDED to the current scenario probe block (no new
// "=== scenario" header — the scenario block contains BOTH the request
// probe AND the stats snapshot under one header so the per-scenario
// asserter can match both shapes in one block lookup).
func emitStatsSnapshot(buf *bytes.Buffer, body []byte) {
	const bareNamePrefix = "envoy_http_adaptive_concurrency_gradient_controller_"
	const labelPrefix = `envoy_http_conn_manager_prefix="`
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split into <name>[{labels}] <value>; value is the last token.
		lastSpace := strings.LastIndex(line, " ")
		if lastSpace < 0 {
			continue
		}
		nameAndLabels := line[:lastSpace]
		value := line[lastSpace+1:]
		// Extract <bare_name>: name without labels.
		bareName := nameAndLabels
		labelsBlock := ""
		if br := strings.Index(nameAndLabels, "{"); br >= 0 {
			bareName = nameAndLabels[:br]
			labelsBlock = nameAndLabels[br:]
		}
		// Filter to adaptive_concurrency surface only.
		if !strings.HasPrefix(bareName, bareNamePrefix) {
			continue
		}
		stat := strings.TrimPrefix(bareName, bareNamePrefix)
		// Extract HCM prefix from the conn-manager-prefix label so the
		// asserter can pin per-listener stat values.
		hcmPrefix := ""
		if idx := strings.Index(labelsBlock, labelPrefix); idx >= 0 {
			tail := labelsBlock[idx+len(labelPrefix):]
			if end := strings.Index(tail, `"`); end >= 0 {
				hcmPrefix = tail[:end]
			}
		}
		// Value normalization: prometheus integer values may appear as
		// "0" or "0.000000" depending on the writer; the
		// internal/stats writer emits bare integers per ADR-0059. We
		// pass the value verbatim and let the asserter use substring
		// match (which tolerates the bare-int form).
		fmt.Fprintf(buf, "stat: %s %s %s\n", hcmPrefix, stat, value)
	}
}

func emitStatsError(buf *bytes.Buffer, err error) {
	fmt.Fprintf(buf, "stats_error: %s\n", err)
}

// deriveAddrsFromBase reconstructs the l_b_overflow + l_d_disabled
// addrs from the runner-supplied l_a_default addr by incrementing the
// port by 1 and 2 respectively. The runner allocates ONE port; the
// driver's SubjectConfig template assumes +1 / +2 are also free.
// Mirrors the phase-20 oauth2 deriveAddrsFromSubj pattern.
func deriveAddrsFromBase(s1Addr string) (addrB, addrD string, err error) {
	lastColon := strings.LastIndex(s1Addr, ":")
	if lastColon < 0 {
		return "", "", fmt.Errorf("malformed addr %q", s1Addr)
	}
	hostPart := s1Addr[:lastColon]
	portStr := s1Addr[lastColon+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", "", fmt.Errorf("parse port from %q: %w", s1Addr, err)
	}
	return fmt.Sprintf("%s:%d", hostPart, port+1), fmt.Sprintf("%s:%d", hostPart, port+2), nil
}

// fixtureDir returns the absolute path to the test/fixtures/0025-http-
// adaptive-concurrency/ directory (the parent of inputs/).
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
	_ fixture.Driver               = (*acDriver)(nil)
	_ fixture.BackendKindAware     = (*acDriver)(nil)
	_ fixture.ReferenceLessFixture = (*acDriver)(nil)
	_ fixture.SubjectAsserter      = (*acDriver)(nil)
)
