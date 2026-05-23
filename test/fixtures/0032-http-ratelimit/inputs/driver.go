// Package inputs registers the 0032-http-ratelimit fixture with the
// differential runner per phase-24.1 SPEC §7 + PLAN Task 10.
//
// # Fixture type
//
// CROSS-SIDE (default RequiresReference=true): the runner spawns reference
// Envoy v1.37.2 + envoy-go, drives both sides via DriveReference /
// DriveSubject, and CompareBytes on the resulting per-scenario byte streams
// is the differential gate. Cross-side byte-exactness on the OVER_LIMIT
// scenario (c) is load-bearing for the AMEND-6 proto-number-faithful fake
// invariant.
//
// # 6 scenarios (24.1 scope per parent SPEC §7.1)
//
//	(a) parse_ok         — GET /scenario_a → zero-descriptor short-circuit → echo 200.
//	(b) ok_admit         — GET /scenario_b → RLS OK → echo 200.
//	(c) over_limit_429   — GET /scenario_c → RLS OVER_LIMIT → 429 empty body.
//	(d) descriptor_actions — GET /scenario_d → 4-action chain → RLS OK → echo 200.
//	(e) failure_mode_open — GET /scenario_e (RLS stopped) → fail-open → echo 200.
//	(h) stat_surface     — OBSERVATIONAL: AssertStats on subject counters
//	                       accumulated by the preceding (b)/(c)/(d)/(e) probes
//	                       (no burst, no replay; proven live per
//	                       reference_differential_asserter_dispatch).
//
// # Assertion wiring
//
//   - (a)/(b)/(c)/(d)/(e) — CompareBytes on the per-scenario status+body-
//     classification line (the runner's cross-side byte gate).
//   - (h) — StatsAsserter.AssertStats on the SUBJECT admin /stats/prometheus.
//     NOT SubjectAsserter (per reference_differential_asserter_dispatch the
//     runner's runFixture dispatch only invokes SubjectAsserter on the
//     reference-less path; this fixture is cross-side ⇒ subject-side
//     assertions live in StatsAsserter).
//
// # Single-listener topology (parent SPEC §7.3)
//
// One listener (l_test_a) with the ratelimit filter + router terminator.
// Per-scenario discrimination lives at the route table (each /scenario_<id>
// path carries its own rate_limits[] policy). No multi-listener — avoids the
// freeTCPPort combined-run flake per 22.2 REVIEW §7.4.
//
// # Fake RLS lifecycle
//
// The shared in-process gRPC RateLimitService fake (test/helpers/ratelimitgrpc/)
// is allocated a free 127.0.0.1:<port> at driver instantiation (so both YAMLs
// can templatize the cluster endpoint deterministically before either proxy
// starts). The fake is started fresh ONCE at the beginning of each driveProxy
// run with all per-scenario scripts pre-populated, stopped ONCE before
// scenario (e) which requires dial failure (fail-open admit), and never
// restarted for the remainder of the run. The driver's scripts cover every
// CanonicalKey the engine is expected to emit — per the Task 9 advisory (the
// fake returns default-OK on no-match; an unscripted key would silently pass
// through OK and mask the assertion).
//
// # Reference container ports
//
//	refAdminPort  = 9901 (standard; harness waits on this port)
//	refLATestPort = 10032 (l_test_a in the reference container)
//
// # Cross-references
//
//   - parent SPEC §4.1/§4.5/§4.6/§4.7 (descriptor engine + dispositions + byte-shape)
//   - parent SPEC §7.1 (8-scenario matrix; 24.1 takes a/b/c/d-core/e/h)
//   - parent SPEC §7.3 (single-listener topology)
//   - 24.1 SPEC §7 (24.1 scope scoping)
//   - AMEND-1/3/6/8/10/11 (cluster-scoped stats; defaults; fake-encoding;
//     header order; cross-namespace; per-action key defaults)
//   - ADR-0010 / ADR-0166 (host.docker.internal + plaintext h2c)
//   - ADR-0197 (filter shape + dispositions — CORE slice)
//   - ADR-0198 (route-table accessor pair — Task 5)
//   - ADR-0200 (route-level PARSE-REJECTs — Task 3)
//   - reference_differential_asserter_dispatch
//   - 18.2 fixture-0021 (template precedent — fixed pre-allocated port +
//     host.docker.internal/127.0.0.1 templating + NewAtAddr fake-server arm)
//   - 23 fixture-0030 (StatsAsserter + statValue/requireStatIs* helpers)
package inputs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	ratelimitv3 "github.com/envoyproxy/go-control-plane/envoy/service/ratelimit/v3"

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
	"github.com/esalaine/envoy-go/test/helpers/ratelimitgrpc"
)

const (
	fixtureName = "0032-http-ratelimit"

	// Reference-container in-container listener / admin ports. The harness
	// exposes these via testcontainers MappedPort.
	refAdminPort  = 9901
	refLATestPort = 10032

	// rlsDomain is the filter-config `domain` field; load-bearing for the
	// fake's CanonicalKey lookup (the fake key is built as
	// `domain | desc[0] | desc[1] ...`).
	rlsDomain = "domain_b"

	// rlsClusterName is the upstream RLS cluster name (matches envoy.yaml
	// + envoy-go.yaml). Used for the StatsAsserter Prometheus label query.
	rlsClusterName = "c_ratelimit"

	// tenantHeader / canaryHeader are the per-request discriminators for
	// scenario (d). Their values populate the descriptor entries the engine
	// emits; the fake script keyed on those exact values returns OK.
	tenantHeader = "x-tenant"
	tenantValue  = "tenant-x"
	canaryHeader = "x-canary"

	// loopbackIP is the value the remote_address action emits when the
	// downstream peer is the host loopback. Both reference (dockerized,
	// reaching envoy via the docker bridge) and envoy-go (loopback) see
	// "127.0.0.1" as the downstream remote_address per the ADR-0165
	// set-once-by-dispatch accessor. (Reference Envoy's downstream peer
	// from the host's perspective is the docker-bridge gateway, but the
	// reference container's view of the downstream is its localhost-mapped
	// port — both reduce to 127.0.0.1 at the descriptor.)
	loopbackIP = "127.0.0.1"
)

func init() {
	fixture.RegisterFixture(fixtureName, &rlDriver{})
}

// rlDriver carries per-driver lifecycle state — the pre-allocated RLS fake
// port + the running fake handle (toggled across scenarios that require a
// dial failure).
type rlDriver struct {
	mu sync.Mutex

	// rlsPort is the pre-allocated 127.0.0.1:<port> the RLS fake binds to;
	// shared between ReferenceBootstrap and SubjectConfig so both YAMLs
	// templatize the SAME cluster endpoint deterministically before either
	// proxy starts. Allocated lazily on first use (whichever of
	// ReferenceBootstrap / SubjectConfig fires first).
	rlsPort int

	// rlsSrv is the currently-running fake. nil before driveProxy starts it
	// and between stopRLS / setupRLS toggles. Lifecycle managed inside
	// driveProxy (Setup-at-start, toggle around e + h's fail-open arm,
	// teardown at end).
	rlsSrv *ratelimitgrpc.Server
}

// --- lazy fake-port allocation ---

// allocateRLSPort allocates a free TCP port for the RLS fake. Called lazily
// by ReferenceBootstrap / SubjectConfig (whichever fires first). Idempotent —
// returns the same port on subsequent calls. Does NOT start the server; the
// server is started fresh at the beginning of each driveProxy call.
func (d *rlDriver) allocateRLSPort() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rlsPort != 0 {
		return d.rlsPort
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate rls port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	d.rlsPort = port
	return port
}

// setupRLS starts the in-process fake bound to the pre-allocated port,
// pre-populates the per-scenario scripts, and stores the handle on the
// driver. Idempotent — an already-running fake is stopped first.
//
// Scripts (all under domain_b):
//
//	scenario=b                                                  → OK
//	scenario=c                                                  → OVER_LIMIT
//	scenario=d;tenant=tenant-x;remote_address=127.0.0.1;header_match=canaried → OK
//	scenario=e                                                  → OK (defensive;
//	  scenario (e) the dial FAILS before the script is consulted — the script
//	  is present so a regression that fails to stop the fake surfaces as
//	  "scenario e admit via OK script + counter mismatch" rather than as a
//	  pass-through default-OK silent green.)
//
// Per the Task 9 advisory every expected canonical key is EXPLICITLY scripted
// so a default-OK on no-match cannot silently satisfy the test. The (a)
// scenario does NOT script anything (the engine emits zero descriptors;
// ShouldRateLimit never fires).
func (d *rlDriver) setupRLS() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rlsSrv != nil {
		d.rlsSrv.Stop()
		d.rlsSrv = nil
	}
	if d.rlsPort == 0 {
		return fmt.Errorf("driver: setupRLS called before rlsPort allocation")
	}
	addr := fmt.Sprintf("127.0.0.1:%d", d.rlsPort)
	srv, err := ratelimitgrpc.NewAtAddr(addr)
	if err != nil {
		return fmt.Errorf("driver: start rls fake on %s: %w", addr, err)
	}
	d.rlsSrv = srv

	// Script (b) — single-entry descriptor → OK.
	srv.Script(canonicalKeyFor([][2]string{{"scenario", "b"}}),
		respOKForDescriptors(1))

	// Script (c) — single-entry descriptor → OVER_LIMIT.
	// AMEND-6 / D-RL5: construct the response with ONLY the fields the
	// scenario emits — OverallCode + per-descriptor OVER_LIMIT statuses. No
	// RawBody, no Quota, no per-descriptor optionals (current_limit /
	// limit_remaining / duration_until_reset / quota). Go-protobuf's
	// zero-value/nil omission keeps the wire bytes byte-equivalent to the
	// reference Envoy v1.37.2 emit shape.
	srv.Script(canonicalKeyFor([][2]string{{"scenario", "c"}}),
		respOverLimitForDescriptors(1))

	// Script (d) — 4-entry descriptor (AMEND-6 entries-in-action-list order:
	// generic_key → request_headers → remote_address → header_value_match).
	srv.Script(canonicalKeyFor([][2]string{
		{"scenario", "d"},
		{"tenant", tenantValue},
		{"remote_address", loopbackIP},
		{"header_match", "canaried"},
	}), respOKForDescriptors(1))

	// Script (e) — defensive (the fake is STOPPED before the request so the
	// dial fails first; the script ensures a regression where the fake stays
	// up surfaces as a counter mismatch in (h) rather than as a silent pass).
	srv.Script(canonicalKeyFor([][2]string{{"scenario", "e"}}),
		respOKForDescriptors(1))

	if os.Getenv("FIXTURE_0032_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[driver] scripted keys: scenario=b, scenario=c, "+
			"scenario=d;tenant=tenant-x;remote_address=127.0.0.1;header_match=canaried, "+
			"scenario=e (all under domain %q)\n", rlsDomain)
	}
	return nil
}

// stopRLS stops the running fake. Idempotent. Called once before scenario (e)
// (which requires dial failure → fail-open admit) and never restarted within
// the same driveProxy run.
func (d *rlDriver) stopRLS() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.rlsSrv != nil {
		d.rlsSrv.Stop()
		d.rlsSrv = nil
	}
}

// --- script-key helpers ---

// canonicalKeyFor builds the CanonicalKey for a single descriptor with the
// supplied entries (in order). Mirrors the format the descriptor engine emits
// at runtime + the fake reads at ShouldRateLimit time. Per the
// ratelimitgrpc.CanonicalKey contract:
//
//	domain "|" descriptor[0]
//	where descriptor[0] = "key=value;key=value;..." in entry order.
//
// 24.1 fixture 0032 only ever produces one-descriptor requests at the engine
// (the route policies are single-policy lists); this helper builds the
// matching single-descriptor key.
func canonicalKeyFor(entries [][2]string) string {
	segs := make([]string, len(entries))
	for i, e := range entries {
		segs[i] = e[0] + "=" + e[1]
	}
	return rlsDomain + "|" + strings.Join(segs, ";")
}

// respOKForDescriptors builds an OK RateLimitResponse with per-descriptor OK
// statuses (n entries; matches the request's descriptor count). AMEND-6: only
// OverallCode + Statuses[i].Code are set; all other optionals (RawBody,
// DynamicMetadata, Quota, per-descriptor CurrentLimit / LimitRemaining /
// DurationUntilReset / Quota) are zero-value / nil and elided by Go-protobuf.
func respOKForDescriptors(n int) *ratelimitv3.RateLimitResponse {
	statuses := make([]*ratelimitv3.RateLimitResponse_DescriptorStatus, n)
	for i := range statuses {
		statuses[i] = &ratelimitv3.RateLimitResponse_DescriptorStatus{
			Code: ratelimitv3.RateLimitResponse_OK,
		}
	}
	return &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OK,
		Statuses:    statuses,
	}
}

// respOverLimitForDescriptors builds an OVER_LIMIT RateLimitResponse with
// per-descriptor OVER_LIMIT statuses. AMEND-6: only OverallCode + Statuses[i].Code
// are set; no RawBody, no per-descriptor optionals.
func respOverLimitForDescriptors(n int) *ratelimitv3.RateLimitResponse {
	statuses := make([]*ratelimitv3.RateLimitResponse_DescriptorStatus, n)
	for i := range statuses {
		statuses[i] = &ratelimitv3.RateLimitResponse_DescriptorStatus{
			Code: ratelimitv3.RateLimitResponse_OVER_LIMIT,
		}
	}
	return &ratelimitv3.RateLimitResponse{
		OverallCode: ratelimitv3.RateLimitResponse_OVER_LIMIT,
		Statuses:    statuses,
	}
}

// --- fixture.Driver (required) ---

func (*rlDriver) BackendCount() int                { return 1 }
func (*rlDriver) BackendKind() fixture.BackendKind { return fixture.HTTPGlobalRateLimitGRPC }
func (*rlDriver) SubjectListenerName() string      { return "l_test_a" }
func (*rlDriver) ReferenceListenerPort() int       { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with the reference container ports
// + host.docker.internal backend / RLS-fake hosts (the reference container
// reaches the host's loopback via host.docker.internal per ADR-0010).
func (d *rlDriver) ReferenceBootstrap(backendPorts []int) string {
	rlsPort := d.allocateRLSPort()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   refAdminPort,
		"LATestPort":  refLATestPort,
		"BackendHost": "host.docker.internal",
		"BackendPort": backendPorts[0],
		"RLSHost":     "host.docker.internal",
		"RLSPort":     rlsPort,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated ports + the
// loopback backend / RLS-fake hosts (envoy-go runs on the host directly —
// no docker translation).
func (d *rlDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	rlsPort := d.allocateRLSPort()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"LATestPort":  subjListenerPort,
		"BackendPort": backendPorts[0],
		"RLSHost":     "127.0.0.1",
		"RLSPort":     rlsPort,
	})
}

// DriveReference issues the 5-scenario probe sequence (a/b/c/d/e) against the
// reference proxy. Scenario (h) is OBSERVATIONAL — it issues NO additional
// requests; AssertStats reads the counters accumulated by b/c/d/e
// subject-side.
func (d *rlDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "ref")
}

// DriveSubject issues the same sequence against envoy-go.
func (d *rlDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin diff.
func (*rlDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) ([]byte, []byte, error) {
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

// --- core probe sequence ---

// driveProxy issues the 5-scenario probe sequence against the listener
// address provided in addr. Scenario (h) is OBSERVATIONAL — it does NOT
// re-burst requests; it asserts the counter deltas accumulated by the
// preceding b/c/d/e probes via StatsAsserter.AssertStats. This keeps the
// fake-lifecycle minimal (one stop before e + tear-down at the end) and
// avoids the gRPC-client reconnect-after-restart edge — the subject's
// gRPC sub-channel manages reconnect state per ADR-0158, but a
// stop+restart-on-the-same-port within a single fixture run is NOT a
// pattern other fixtures exercise (0021's auth-server toggle puts the
// STOPPED state at the END of the live-batch, never mid-stream restart).
//
// The byte stream emitted MUST be identical on both sides (status +
// body-classification per scenario; no per-hop header divergence). The
// "side" label is included only in debug-dump output
// (FIXTURE_0032_DUMP_BYTES=1).
//
// Lifecycle (single stop, no mid-stream restart):
//
//  1. Start the fake with all per-scenario scripts pre-populated.
//  2. Scenarios a (no RLS) + b (OK) + c (OVER_LIMIT) + d (4-action OK).
//  3. STOP the fake.
//  4. Scenario e (RLS unreachable → fail-open admit).
//  5. Teardown — fake stays stopped (the next fixture run starts a fresh one).
//
// After this returns the runner's CompareBytes pass enforces cross-side
// byte-equivalence + the StatsAsserter runs against the subject admin and
// asserts the four cluster-scoped counter values accumulated by b/c/d/e:
//
//	ok                   = 2 (scenarios b + d)
//	over_limit           = 1 (scenario c)
//	error                = 1 (scenario e)
//	failure_mode_allowed = 1 (scenario e)
func (d *rlDriver) driveProxy(ctx context.Context, addr, side string) ([]byte, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	baseURL := "http://" + addr

	if err := d.setupRLS(); err != nil {
		return nil, fmt.Errorf("[%s] setup rls: %w", side, err)
	}

	var b bytes.Buffer

	// (a) parse_ok — no rate_limits → echo 200.
	emitScenario(&b, "a", runGet(ctx, client, baseURL+"/scenario_a", nil, side, "a"))

	// (b) ok_admit — RLS scripted OK → echo 200.
	emitScenario(&b, "b", runGet(ctx, client, baseURL+"/scenario_b", nil, side, "b"))

	// (c) over_limit_429 — RLS scripted OVER_LIMIT → 429 empty body.
	emitScenario(&b, "c", runGet(ctx, client, baseURL+"/scenario_c", nil, side, "c"))

	// (d) descriptor_actions — 4-action chain (generic_key + request_headers
	// + remote_address + header_value_match). The fake matches the
	// 4-entry CanonicalKey → OK → echo 200.
	emitScenario(&b, "d", runGet(ctx, client, baseURL+"/scenario_d",
		http.Header{
			tenantHeader: []string{tenantValue},
			canaryHeader: []string{"true"},
		}, side, "d"))

	// STOP the fake before scenario (e) — forces the gRPC dial to fail fast.
	d.stopRLS()

	// (e) failure_mode_open — RLS unreachable → fail-open admit → echo 200.
	emitScenario(&b, "e", runGet(ctx, client, baseURL+"/scenario_e", nil, side, "e"))

	// Teardown — fake stays stopped (the next fixture run starts a fresh one).
	return b.Bytes(), nil
}

// --- scenario probe primitives ---

// scenarioResult is the per-scenario observation captured for the byte stream
// the runner's CompareBytes pass compares between sides.
type scenarioResult struct {
	statusCode int
	body       []byte
	err        error
}

// runGet issues a single GET against the supplied URL with optional headers
// and returns the response status + body. Errors are folded into the
// scenarioResult; the caller's emitScenario maps them into the byte stream
// as a stable "status=ERR body=ERR" line (both sides see the same on a
// shared transport failure).
func runGet(ctx context.Context, client *http.Client, url string, hdr http.Header, side, label string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	for k, vs := range hdr {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("do request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("read body: %w", err)}
	}
	res := scenarioResult{statusCode: resp.StatusCode, body: body}
	if os.Getenv("FIXTURE_0032_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] %s: status=%d body=%q\n", side, label, res.statusCode, string(res.body))
	}
	return res
}

// emitScenario formats the per-scenario verdict line into the byte stream.
//
//	scenario <id> status=<code> body=<ok|mismatch(...)>
//
// The side label is NOT emitted (the byte stream must be identical per-side
// for the CompareBytes differential gate to fire on equivalence). On request
// error both sides emit "status=ERR body=ERR" — symmetrical and stable.
func emitScenario(b *bytes.Buffer, id string, res scenarioResult) {
	if res.err != nil {
		fmt.Fprintf(b, "scenario %s status=ERR body=ERR\n", id)
		return
	}
	bodyVerdict := classifyBody(id, res.body)
	fmt.Fprintf(b, "scenario %s status=%d body=%s\n", id, res.statusCode, bodyVerdict)
}

// classifyBody returns the per-scenario body verdict. The body is NOT
// emitted byte-for-byte because Envoy adds per-hop headers (x-forwarded-for,
// x-request-id, x-envoy-*) that the echobackend reflects into its JSON body
// — those headers diverge across the two sides. Instead each scenario
// classifies the body structurally:
//
//	allow scenarios (a/b/d/e):
//	  ⇒ body is the echobackend echo JSON (object with method+path keys).
//
//	over_limit scenario (c):
//	  ⇒ body is empty (no RawBody on the scripted OVER_LIMIT response).
func classifyBody(id string, body []byte) string {
	switch id {
	case "a", "b", "d", "e":
		if !isEchoBody(body) {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", string(body))
		}
		return "ok"
	case "c":
		if len(body) == 0 {
			return "ok"
		}
		return fmt.Sprintf("mismatch(want_empty,got=%q)", string(body))
	}
	return "skip"
}

// isEchoBody returns true iff body is a JSON object containing at least the
// "method" and "path" keys — the structural signature of the echobackend
// response (test/helpers/echobackend/echobackend.go::buildEcho).
func isEchoBody(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		return false
	}
	_, hasMethod := m["method"]
	_, hasPath := m["path"]
	return hasMethod && hasPath
}

// --- fixture.StatsAsserter ---

// AssertStats performs the subject-only scenario (h) counter assertion
// after the runner's cross-side CompareBytes + admin diff. Reads the SUBJECT
// admin /stats/prometheus (refAdminAddr is unused — the reference counters
// are NOT asserted at 24.1) and asserts the four
// `cluster.c_ratelimit.ratelimit.*` counters at the deterministic deltas
// accumulated by the b/c/d/e probe sequence:
//
//	ok                   = 2  (scenarios (b) admit + (d) admit)
//	over_limit           = 1  (scenario  (c) OVER_LIMIT)
//	error                = 1  (scenario  (e) RLS unreachable → applyError arm)
//	failure_mode_allowed = 1  (scenario  (e) failure_mode_deny:false fail-open
//	                           — incremented INSIDE the applyError arm per
//	                           dispositions.go::applyError)
//
// Per reference_differential_asserter_dispatch the subject-side counter
// assertions MUST live in StatsAsserter (NOT SubjectAsserter; the latter
// fires only on the reference-less path). The deliberate-break recipe in
// the Task 10 PROGRESS entry proves AssertStats is LIVE by temporarily
// asserting a wrong value (must FAIL), then reverting (must GREEN).
func (d *rlDriver) AssertStats(t fixture.TB, _ /*refAdminAddr*/, subjAdminAddr string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statsBody, err := scrapeStats(ctx, subjAdminAddr)
	if err != nil {
		t.Errorf("scenario h stat_surface: scrape /stats/prometheus: %v", err)
		return
	}
	statsOut := string(statsBody)

	if os.Getenv("FIXTURE_0032_DUMP_STATS") != "" {
		for _, line := range strings.Split(statsOut, "\n") {
			if strings.Contains(line, "ratelimit") || strings.Contains(line, "c_ratelimit") {
				fmt.Fprintf(os.Stderr, "[subj] %s\n", line)
			}
		}
	}

	// 4 cluster-scoped counter expectations per the b/c/d/e probe sequence.
	type expect struct {
		stat    string
		want    int64
		comment string
	}
	expectations := []expect{
		{stat: "ok", want: 2, comment: "scenarios (b) + (d) admit"},
		{stat: "over_limit", want: 1, comment: "scenario (c)"},
		{stat: "error", want: 1, comment: "scenario (e) RLS unreachable"},
		{stat: "failure_mode_allowed", want: 1, comment: "scenario (e) failure_mode_deny:false"},
	}
	for _, exp := range expectations {
		got, present := clusterRatelimitCounter(statsOut, rlsClusterName, exp.stat)
		if !present {
			t.Errorf("scenario h: cluster.%s.ratelimit.%s absent from /stats/prometheus (%s)",
				rlsClusterName, exp.stat, exp.comment)
			continue
		}
		if got != exp.want {
			t.Errorf("scenario h: cluster.%s.ratelimit.%s = %d; want %d (%s)",
				rlsClusterName, exp.stat, got, exp.want, exp.comment)
		}
	}
}

// scrapeStats fetches /stats/prometheus from the supplied admin endpoint.
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

// clusterRatelimitCounter returns the value of the
// `envoy_cluster_ratelimit.<stat>{envoy_cluster_name="<cluster>"}` Prometheus
// counter, or (0, false) if no matching line is found. The Prometheus form
// reflects the absolute name `cluster.<cluster>.ratelimit.<stat>` via the
// SN1 flattening rule (internal/stats/name.go:42-51):
//
//	cluster.c_ratelimit.ratelimit.ok
//	  → tail="c_ratelimit.ratelimit.ok"
//	  → label envoy_cluster_name="c_ratelimit", rest="ratelimit.ok"
//	  → base = "envoy_cluster_" + rest = "envoy_cluster_ratelimit.ok"
//
// NOTE: unlike SN2 (which applies a dot→underscore transform on the `rest`
// segment per the phase-09 follow-up), SN1 does NOT apply the transform.
// The emitted Prometheus name therefore contains a literal '.' between
// "ratelimit" and the leaf — non-standard but functional (Prometheus
// rejects metric names with dots in general; envoy-go's exposition emits
// the line as-is). The lookup helper matches the literal form. A future
// stats-cleanup phase MAY extend SN1 with the dot→underscore transform;
// this fixture is the first cross-namespace cluster-scoped stat consumer
// per AMEND-1/AMEND-10 + ADR-0197 and the empirical Prometheus shape
// surfaces here.
//
// Absent ≡ 0 semantics for counters is honored at the call-site via the
// boolean: callers that expect a non-zero value treat absent as a failure
// (the counter MUST be registered + scraped after the probe sequence).
func clusterRatelimitCounter(statsOut, cluster, stat string) (int64, bool) {
	// Match both the literal-dot form (SN1 as-implemented) and the
	// underscore-normalized form (SN1 after a future dot→underscore
	// extension) so the helper survives a stats-cleanup phase without
	// fixture churn.
	needlePrefixes := []string{
		"envoy_cluster_ratelimit." + stat,
		"envoy_cluster_ratelimit_" + stat,
	}
	labelNeedle := fmt.Sprintf(`envoy_cluster_name="%s"`, cluster)
	for _, line := range strings.Split(statsOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		matched := false
		for _, p := range needlePrefixes {
			// Match prefix + delimiter (space OR `{`) so we don't match
			// e.g., "envoy_cluster_ratelimit.over_limit" when looking up
			// "envoy_cluster_ratelimit.over" (defensive — both stat suffixes
			// share a common prefix in the over_limit/over case).
			if strings.HasPrefix(line, p+" ") || strings.HasPrefix(line, p+"{") {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if !strings.Contains(line, labelNeedle) {
			continue
		}
		lastSpace := strings.LastIndex(line, " ")
		if lastSpace < 0 {
			continue
		}
		valStr := strings.TrimSpace(line[lastSpace+1:])
		val, err := strconv.ParseFloat(valStr, 64)
		if err != nil {
			continue
		}
		return int64(val), true
	}
	return 0, false
}

// --- file/template helpers ---

// fixtureDir returns the absolute path to the test/fixtures/0032-http-
// ratelimit/ directory (the parent of inputs/).
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
	_ fixture.Driver           = (*rlDriver)(nil)
	_ fixture.BackendKindAware = (*rlDriver)(nil)
	_ fixture.StatsAsserter    = (*rlDriver)(nil)
)
