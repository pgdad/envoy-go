// Package driver registers the 0086-tracing-request-id fixture with the
// differential runner. It is the behavioral proof of the phase 46.1a HCM-native
// request-tracing HEADER engine: when an HCM carries a `tracing` block
// (OpenTelemetry provider, random_sampling 100%), envoy-go runs the
// sampling/request-id decision and injects x-request-id + W3C traceparent onto
// the UPSTREAM-forwarded request — proven cross-side EQUIVALENT against reference
// Envoy v1.37.2 (Docker, contrib-v1.37.2).
//
// Capture mechanism (NO receiver — the existing HTTPHeaderMutation Kind-9 echo
// backend): the route targets the 0012-http-header-mutation echo backend, which
// reflects every received request header into the response body as sorted
// "Canonical-Name: value" lines (Go's net/http canonicalizes header names, so
// x-request-id appears as "X-Request-Id" and traceparent as "Traceparent"). The
// driver parses each response body to recover the upstream-forwarded
// x-request-id + traceparent.
//
// Integration shape (single-listener plaintext H1; NO TLS, NO receiver):
//
//  1. ReferenceBootstrap renders envoy.yaml with host.docker.internal (ADR-0010
//     STRICT_DNS) + the runner-allocated backend port. SubjectConfig renders
//     envoy-go.yaml with the runner-allocated admin/listener ports + backend
//     port (loopback). Both carry the `tracing` block + a DUMMY (unreachable)
//     c_otlp_collector cluster that keeps the provider config valid + 46.1b-ready
//     and is never successfully dialed at 46.1a.
//
//  2. DriveReference / DriveSubject each fire, against the proxy under test:
//     - N (numPlain) PLAIN requests (fixed Host/User-Agent, query-less path) —
//     no inbound trace context. random_sampling 100% => each is a FRESH
//     locally-sampled trace (x-request-id REASON nibble '9' Sampled;
//     traceparent flags 01).
//     - M (numCont) CONTINUATION requests carrying
//     "Traceparent: 00-<fixed 32hex>-<fixed 16hex>-01" — an inbound W3C trace
//     context. envoy-go CONTINUES the trace (keeps the inbound trace-id;
//     x-request-id REASON nibble '9' Sampled — a continued+sampled trace's nibble
//     reflects the inbound sampled bit, matching the reference; the COUNTER class
//     stays not_traceable).
//     The per-request status byte stream (deterministic 200s — the random ids are
//     NOT in the stream) is returned for the runner's CompareBytes pass; the
//     echoed x-request-id + traceparent are snapshotted per side for AssertStats.
//
//  3. AssertStats asserts cross-side on the STABLE structure (the random VALUES
//     vary and are UNasserted, except the continuation trace-id):
//     - Both sides, each PLAIN request: x-request-id is 36-char UUID-shaped with
//     string index-14 == '9' (Sampled); traceparent is 00-<32hex>-<16hex>-01.
//     Both headers PRESENT (a zero-header pass is vacuous — proves injection
//     ran on BOTH sides).
//     - Both sides, each CONTINUATION request: traceparent trace-id == the FIXED
//     inbound trace-id (the cross-side EXACT continuation invariant — both sides
//     CONTINUE the trace) AND the x-request-id REASON nibble == '9' (Sampled),
//     cross-side EXACT: a continued+sampled trace (inbound flags 01) reports the
//     inbound sampled bit in its nibble, and envoy-go matches the reference (SPEC
//     §11 D-TRACE-REQUESTID probe-error correction, re-probed at the 46.1a IMPL).
//     The COUNTER class stays not_traceable (subject /stats not_traceable == M).
//     - Subject /stats: http.hcm_local.tracing.random_sampling == numPlain,
//     not_traceable == numCont, client_enabled/health_check/service_forced == 0.
//     The reference emits DIFFERENT tracing stat names — only the SUBJECT /stats
//     is asserted (mirrors 0084's subject-specific stat assertion). The CROSS-SIDE
//     assertion is on the echoed upstream HEADERS, which both sides produce.
//
// UNasserted: the x-request-id / traceparent VALUES (except the continuation
// trace-id); the span-id; the reference's default-injected extras (x-envoy-* /
// x-forwarded-*). One fixture dir = one runner branch
// (reference_differential_fixture_dispatch_constraint).
package driver

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0086-tracing-request-id"

	// In-container reference Envoy ports. Convention "100NN" for fixture "00NN";
	// fixture 0086 takes 10086 for the single plaintext listener.
	refAdminPort    = 9901
	refListenerPort = 10086

	// numPlain PLAIN requests (no inbound trace context) — each a fresh local
	// sample under random_sampling 100% (=> http.hcm_local.tracing.random_sampling).
	numPlain = 8
	// numCont CONTINUATION requests (inbound traceparent) — each continues the
	// inbound trace (=> http.hcm_local.tracing.not_traceable).
	numCont = 4

	// The fixed request shape (kept identical cross-side; query-less path).
	probePath = "/trace" // query-less.
	probeHost = "trace.example"
	probeUA   = "trace-probe/1"

	// The FIXED inbound W3C trace context for the continuation requests. NOT
	// all-zero (all-zero trace-id is treated as no-incoming-context and would
	// reset to a fresh local sample). The proxy CONTINUES this trace, so the
	// upstream-forwarded traceparent keeps contTraceID.
	contTraceID  = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 32 hex
	contParentID = "bbbbbbbbbbbbbbbb"                 // 16 hex
	contFlags    = "01"                               // inbound sampled

	// stat_prefix from the bootstraps (both sides) => the http.<prefix>.tracing.*
	// counter namespace.
	statPrefix = "hcm_local"

	// The x-request-id REASON nibble (canonical UUID string index 14). BOTH the
	// PLAIN (fresh local sample) and the CONTINUATION (continued+sampled, inbound
	// flags 01) prongs stamp '9' (Sampled) — cross-side EXACT. The continuation
	// nibble was re-probed at the 46.1a IMPL via this fixture: the reference stamps
	// the continued trace's inbound sampled bit, and envoy-go now matches it (SPEC
	// §11 D-TRACE-REQUESTID probe-error correction; the COUNTER class stays
	// not_traceable).
	reasonSampled = '9'

	reqTimeout = 10 * time.Second
)

// Subject tracing decision counters (flat /stats internal names).
var (
	statRandomSampling = "http." + statPrefix + ".tracing.random_sampling"
	statNotTraceable   = "http." + statPrefix + ".tracing.not_traceable"
	statClientEnabled  = "http." + statPrefix + ".tracing.client_enabled"
	statHealthCheck    = "http." + statPrefix + ".tracing.health_check"
	statServiceForced  = "http." + statPrefix + ".tracing.service_forced"
)

// traceparentRe matches a sampled W3C traceparent: version 00, 32-hex trace-id,
// 16-hex span-id, flags 01 (sampled). The flags are 01 on BOTH the fresh-local
// (sampled) and the continued (inbound 01) prongs of this fixture.
var traceparentRe = regexp.MustCompile(`^00-([0-9a-f]{32})-[0-9a-f]{16}-01$`)

func init() {
	fixture.RegisterFixture(fixtureName, &traceDriver{})
}

// echoed captures the upstream-forwarded x-request-id + traceparent recovered
// from one HTTPHeaderMutation echo-body response.
type echoed struct {
	requestID   string
	traceparent string
}

// traceDriver snapshots each side's plain + continuation echoed headers for the
// AssertStats cross-side assertion.
type traceDriver struct {
	refPlain []echoed
	refCont  []echoed

	subjPlain []echoed
	subjCont  []echoed
}

// --- fixture.Driver (required) ---

func (*traceDriver) BackendCount() int                { return 1 }
func (*traceDriver) BackendKind() fixture.BackendKind { return fixture.HTTPHeaderMutation }
func (*traceDriver) SubjectListenerName() string      { return "l_test" }
func (*traceDriver) ReferenceListenerPort() int       { return refListenerPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal + the
// runner-allocated backend port.
func (*traceDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    refAdminPort,
		"ListenerPort": refListenerPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback).
func (*traceDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"ListenerPort": subjListenerPort,
		"BackendPort":  backendPorts[0],
	})
}

// DriveReference fires the workload against the reference proxy and snapshots the
// reference-side echoed headers.
func (d *traceDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	out, plain, cont, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.refPlain = plain
	d.refCont = cont
	return out, nil
}

// DriveSubject fires the workload against the subject proxy and snapshots the
// subject-side echoed headers.
func (d *traceDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	out, plain, cont, err := d.driveSide(ctx, addr)
	if err != nil {
		return nil, err
	}
	d.subjPlain = plain
	d.subjCont = cont
	return out, nil
}

// driveSide fires the numPlain PLAIN + numCont CONTINUATION requests against the
// proxy listener at addr, parses each echo-body response to recover the
// upstream-forwarded x-request-id + traceparent, and returns the deterministic
// per-request status byte stream (the random ids are intentionally NOT in the
// stream — they vary cross-side; the runner's CompareBytes only sees the stable
// statuses) plus the per-request echoed-header snapshots.
func (d *traceDriver) driveSide(ctx context.Context, addr string) ([]byte, []echoed, []echoed, error) {
	client := &http.Client{
		Transport: &http.Transport{DisableKeepAlives: true},
		Timeout:   reqTimeout,
	}

	var b bytes.Buffer
	var plain, cont []echoed

	for i := 0; i < numPlain; i++ {
		code, e, err := d.fireProbe(ctx, client, addr, nil)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("plain request %d: %w", i, err)
		}
		fmt.Fprintf(&b, "plain status=%d\n", code)
		plain = append(plain, e)
	}
	contTP := "00-" + contTraceID + "-" + contParentID + "-" + contFlags
	for i := 0; i < numCont; i++ {
		code, e, err := d.fireProbe(ctx, client, addr, map[string]string{"Traceparent": contTP})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("continuation request %d: %w", i, err)
		}
		fmt.Fprintf(&b, "cont status=%d\n", code)
		cont = append(cont, e)
	}
	return b.Bytes(), plain, cont, nil
}

// fireProbe issues one query-less GET probePath with the fixed Host + User-Agent
// (plus any extra inbound headers), reads the echo-body response, and recovers
// the upstream-forwarded x-request-id + traceparent from it.
func (d *traceDriver) fireProbe(ctx context.Context, client *http.Client, addr string, extra map[string]string) (int, echoed, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+probePath, nil)
	if err != nil {
		return 0, echoed{}, fmt.Errorf("build request: %w", err)
	}
	req.Host = probeHost
	req.Header.Set("User-Agent", probeUA)
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, echoed{}, fmt.Errorf("do request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, echoed{}, fmt.Errorf("read body: %w", err)
	}
	return resp.StatusCode, parseEcho(body), nil
}

// parseEcho extracts the upstream-forwarded x-request-id + traceparent from the
// HTTPHeaderMutation echo body ("Canonical-Name: value" lines, one per received
// upstream request header). Header-name matching is case-insensitive (the backend
// emits Go-canonicalized names: X-Request-Id, Traceparent).
func parseEcho(body []byte) echoed {
	var e echoed
	for _, line := range strings.Split(string(body), "\n") {
		idx := strings.IndexByte(line, ':')
		if idx < 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(line[:idx]))
		val := strings.TrimSpace(line[idx+1:])
		switch name {
		case "x-request-id":
			e.requestID = val
		case "traceparent":
			e.traceparent = val
		}
	}
	return e
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and returns
// the raw response bytes for the standard admin-diff at the runner's probe step.
func (*traceDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- fixture.StatsAsserter ---

// AssertStats asserts cross-side on the STABLE structure of the echoed
// upstream-forwarded tracing headers (the random VALUES vary and are UNasserted,
// except the continuation trace-id), plus the SUBJECT-side tracing decision
// counters.
func (d *traceDriver) AssertStats(t fixture.TB, _, subjAdminAddr string) {
	t.Helper()

	if os.Getenv("FIXTURE_0086_DUMP") != "" {
		fmt.Fprintf(os.Stderr, "=== 0086 ref plain=%d cont=%d subj plain=%d cont=%d ===\n",
			len(d.refPlain), len(d.refCont), len(d.subjPlain), len(d.subjCont))
		for i, e := range d.refPlain {
			fmt.Fprintf(os.Stderr, "ref  plain[%d] x-request-id=%q traceparent=%q\n", i, e.requestID, e.traceparent)
		}
		for i, e := range d.refCont {
			fmt.Fprintf(os.Stderr, "ref  cont[%d] x-request-id=%q traceparent=%q\n", i, e.requestID, e.traceparent)
		}
		for i, e := range d.subjPlain {
			fmt.Fprintf(os.Stderr, "subj plain[%d] x-request-id=%q traceparent=%q\n", i, e.requestID, e.traceparent)
		}
		for i, e := range d.subjCont {
			fmt.Fprintf(os.Stderr, "subj cont[%d] x-request-id=%q traceparent=%q\n", i, e.requestID, e.traceparent)
		}
	}

	// Non-vacuous counts: both sides MUST have produced the full workload.
	assertCount(t, "reference", "plain", len(d.refPlain), numPlain)
	assertCount(t, "subject", "plain", len(d.subjPlain), numPlain)
	assertCount(t, "reference", "cont", len(d.refCont), numCont)
	assertCount(t, "subject", "cont", len(d.subjCont), numCont)

	// PLAIN: fresh local sample (nibble '9', traceparent flags 01) — both sides.
	assertPlain(t, "reference", d.refPlain)
	assertPlain(t, "subject", d.subjPlain)

	// CONTINUATION: cross-side EXACT on BOTH invariants — the traceparent trace-id
	// == the fixed inbound id (both sides CONTINUE the trace) AND the x-request-id
	// REASON nibble == '9' (Sampled). A continued-AND-sampled trace (inbound flags
	// 01) reports the inbound sampled bit in the nibble; envoy-go matches the
	// reference here (SPEC §11 D-TRACE-REQUESTID probe-error correction, re-probed
	// at the 46.1a IMPL). The COUNTER class stays not_traceable (asserted below via
	// the subject /stats). Both sides pinned to '9' so a regression on EITHER side
	// bites.
	assertCont(t, "reference", d.refCont)
	assertCont(t, "subject", d.subjCont)

	// Subject-side tracing decision counters (the reference emits DIFFERENT
	// tracing stat names — subject-only, like 0084's subject-specific OTLP stat).
	subjStats, err := scrapeFlatStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subject /stats: %v", err)
	}
	assertStat(t, subjStats, statRandomSampling, numPlain)
	assertStat(t, subjStats, statNotTraceable, numCont)
	assertStat(t, subjStats, statClientEnabled, 0)
	assertStat(t, subjStats, statHealthCheck, 0)
	assertStat(t, subjStats, statServiceForced, 0)
}

func assertCount(t fixture.TB, side, kind string, got, want int) {
	t.Helper()
	if got != want {
		t.Fatalf("%s %s requests: got %d echoed snapshots, want %d (vacuous?)", side, kind, got, want)
	}
}

// assertPlain asserts every PLAIN request's echoed headers are PRESENT and shaped
// as a fresh local sample: x-request-id is a 36-char UUID with index-14 == '9'
// (Sampled); traceparent is 00-<32hex>-<16hex>-01.
func assertPlain(t fixture.TB, side string, es []echoed) {
	t.Helper()
	for i, e := range es {
		assertRequestID(t, side, "plain", i, e.requestID, reasonSampled)
		if !traceparentRe.MatchString(e.traceparent) {
			t.Fatalf("%s plain[%d]: traceparent %q does not match 00-<32hex>-<16hex>-01", side, i, e.traceparent)
		}
	}
}

// assertCont asserts every CONTINUATION request's echoed traceparent keeps the
// FIXED inbound trace-id (the cross-side EXACT continuation invariant) and that
// its x-request-id carries the '9' (Sampled) REASON nibble — cross-side EXACT
// (a continued+sampled trace reflects the inbound sampled bit; both sides match;
// see the call site for the SPEC §11 probe-error correction rationale).
func assertCont(t fixture.TB, side string, es []echoed) {
	t.Helper()
	for i, e := range es {
		assertRequestID(t, side, "cont", i, e.requestID, reasonSampled)
		m := traceparentRe.FindStringSubmatch(e.traceparent)
		if m == nil {
			t.Fatalf("%s cont[%d]: traceparent %q does not match 00-<32hex>-<16hex>-01", side, i, e.traceparent)
		}
		if m[1] != contTraceID {
			t.Fatalf("%s cont[%d]: continued trace-id = %q, want fixed inbound %q", side, i, m[1], contTraceID)
		}
	}
}

// assertRequestID asserts presence + 36-char UUID shape + the expected REASON
// nibble at canonical string index 14.
func assertRequestID(t fixture.TB, side, kind string, i int, id string, wantNibble byte) {
	t.Helper()
	if id == "" {
		t.Fatalf("%s %s[%d]: x-request-id ABSENT (injection did not run — vacuous)", side, kind, i)
	}
	if len(id) != 36 {
		t.Fatalf("%s %s[%d]: x-request-id %q len %d, want 36 (UUID-shaped)", side, kind, i, id, len(id))
	}
	if got := id[14]; got != wantNibble {
		t.Fatalf("%s %s[%d]: x-request-id index-14 nibble = %q, want %q (id=%q)", side, kind, i, string(got), string(wantNibble), id)
	}
}

func assertStat(t fixture.TB, stats map[string]uint64, name string, want uint64) {
	t.Helper()
	if got := stats[name]; got != want {
		t.Fatalf("subject %s: got %d, want %d", name, got, want)
	}
}

// scrapeFlatStats issues GET /stats against adminAddr and returns the flat
// "name: value" lines parsed into a map (mirrors the 0084 StatsAsserter scraper —
// the http.<prefix>.tracing.* counters surface on the flat /stats admin endpoint).
func scrapeFlatStats(adminAddr string) (map[string]uint64, error) {
	url := "http://" + adminAddr + "/stats"
	resp, err := http.Get(url) //nolint:gosec // fixed admin URL, test-only
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s body: %w", url, err)
	}
	out := make(map[string]uint64)
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.LastIndex(line, ": ")
		if idx < 0 {
			continue
		}
		name := line[:idx]
		v, err := strconv.ParseUint(strings.TrimSpace(line[idx+2:]), 10, 64)
		if err != nil {
			continue
		}
		out[name] = v
	}
	return out, nil
}

// --- file / template helpers (the 0084 idiom) ---

func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0086-tracing-request-id/driver/driver.go
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
	_ fixture.Driver        = (*traceDriver)(nil)
	_ fixture.StatsAsserter = (*traceDriver)(nil)
)
