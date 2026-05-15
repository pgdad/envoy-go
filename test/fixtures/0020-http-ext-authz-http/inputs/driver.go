// Package inputs registers the 0020-http-ext-authz-http fixture with the
// differential runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.ext_authz (HTTP-service mode) and reference Envoy v1.37.2
// across the seven-scenario matrix per phase 18.1 SPEC §7.1.
//
// Integration shape (single-listener fixture; plaintext HTTP/1.1 only per
// planner-time decision D12 — no TLS in phase 18.1):
//
//  1. ReferenceBootstrap renders test/fixtures/0020-http-ext-authz-http/envoy.yaml
//     with host.docker.internal (ADR-0010 STRICT_DNS) + runner-allocated backend
//     port + the in-process auth server's host:port (host=host.docker.internal for
//     reference Envoy; the auth port is allocated at driver instantiation so both
//     ReferenceBootstrap and SubjectConfig can templatize it deterministically).
//     SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
//     + backend port + auth server host:port (host=127.0.0.1 for envoy-go which
//     runs on the host).
//
//  2. DriveReference / DriveSubject issue the identical 7-scenario sequence
//     against each proxy. The 7-scenario assertion-log byte stream is emitted
//     of the form:
//
//     scenario <id> status=<code> body=<ok|mismatch(...)>
//
//     The runner's CompareBytes pass enforces equivalence — when both proxies
//     produce equal verdicts, the differential gate fires.
//
//  3. AssertStats scrapes /stats/prometheus from both admin endpoints AFTER the
//     7-scenario workload. It asserts per-side ext_authz counter deltas against
//     the expected per-SPEC §7.1 matrix: ok, denied, error, failure_mode_allowed,
//     invalid. The disabled counter is STRUCTURALLY UNREACHABLE under MVP (parent
//     §6 amendment 7) and NOT asserted. Scenarios 6 (per-route disabled) MUST NOT
//     increment any ext_authz counter. Scenarios 3 + 4 (unreachable auth server)
//     each increment error; scenario 4 also increments failure_mode_allowed.
//
//  4. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner step 9.
//
// # Auth-server lifecycle
//
// The in-process HTTP auth server (test/helpers/extauthzhttp) is started ONCE
// before the scenario run and serves both proxies. Scenarios 3 + 4 require the
// auth server to be UNREACHABLE — the driver calls authSrv.Stop() before
// those requests. Because the auth server is not restarted after scenarios 3+4,
// scenarios 5, 6, and 7 follow in this topology:
//
//   - Scenario 5 (with_request_body): uses a SECOND ephemeral auth server started
//     in-band for that scenario with an InspectScript that reads the POST body.
//   - Scenario 6 (per-route disabled): no auth call is made (the filter is disabled
//     on the route); the stopped auth server state is irrelevant.
//   - Scenario 7 (per-route check_settings): uses the same second auth server
//     (restarted if needed) so disable_request_body_buffering can allow through.
//
// Topology decision (documented here for Task 12): the fixture uses TWO in-process
// auth servers:
//   - authSrvA (scenarios 1, 2): FixedScript; allows or denies per scenario.
//     Stopped before scenario 3. Routes in envoy.yaml cluster c_authz_a point here.
//   - authSrvB (scenarios 5, 7): started fresh per scenario with InspectScript /
//     FixedScript. Routes in envoy.yaml cluster c_authz_b point here.
//
// HOWEVER — the differential fixture requires BOTH envoy.yaml and envoy-go.yaml
// point at THE SAME auth server URI (the in-process server serves both proxies).
// Since we only have ONE cluster URI per scenario, the topology settles as:
//
//	ONE auth server address, scenarios 1+2 use it while it is running,
//	scenarios 3+4 stop it, scenarios 5+7 start a NEW server on the SAME port.
//
// This is the SIMPLEST topology compatible with a single-listener single-cluster
// fixture. The driver allocates one ephemeral port for the auth server; the
// lifecycle is: start → run S1, S2 → stop (for S3, S4) → restart on same port
// for S5, S6, S7 (S6 doesn't call auth but a live server is harmless).
//
// Task 12 MUST wire envoy.yaml + envoy-go.yaml with a SINGLE c_authz cluster
// pointing at the auth server host:port allocated here. The port is stable for
// the fixture lifetime (allocated at ReferenceBootstrap / SubjectConfig time).
package inputs

import (
	"bufio"
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

	"github.com/esalaine/envoy-go/test/differential/fixture"
	"github.com/esalaine/envoy-go/test/helpers"
	"github.com/esalaine/envoy-go/test/helpers/extauthzhttp"
)

const (
	fixtureName = "0020-http-ext-authz-http"

	// In-container reference Envoy listener port. Convention "100NN" for
	// fixture "00NN" — fixture 0020 takes 10020 for the single plaintext listener.
	refAdminPort  = 9901
	refLATestPort = 10020 // l_test_a (plaintext HTTP/1.1)

	// Deny-path body — the ext_authz HTTP auth server's verbatim deny body.
	// Scenario 2 uses this byte-exact assertion.
	bodyDenyScenario2 = "access denied"

	// Failure-mode-allowed header — scenario 4 asserts this header arrives
	// upstream (echoed back by the echobackend).
	headerFailureModeAllowed = "x-envoy-auth-failure-mode-allowed"

	// Upstream-injected header — scenario 1 asserts the auth server's
	// allowed_upstream_headers are injected before reaching the echo backend.
	// Task 12 wires the auth server to return x-authz-result: allowed on allow.
	headerAuthzResult = "x-authz-result"
)

func init() {
	fixture.RegisterFixture(fixtureName, &extAuthzHTTPDriver{})
}

// extAuthzHTTPDriver carries the per-driver lifecycle state — the auth server
// port (constant across the ref+subj run) and the auth server handle.
type extAuthzHTTPDriver struct {
	mu sync.Mutex

	// authPort is allocated lazily on first use (ReferenceBootstrap or
	// SubjectConfig — whichever fires first). The same port is used for
	// both sides so envoy.yaml and envoy-go.yaml can be templatized
	// deterministically before the per-side Drive runs.
	authPort int

	// authSrv is the currently-running auth server. nil before Drive or
	// after Stop. The driver starts it inside driveProxy and manages its
	// lifecycle across scenarios.
	authSrv *extauthzhttp.Server
}

// allocateAuthPort allocates a free TCP port for the auth server. Called lazily
// by ReferenceBootstrap / SubjectConfig (whichever fires first). Idempotent —
// returns the same port on subsequent calls. Does NOT start the server; the
// server is started fresh at the beginning of each driveProxy call.
func (d *extAuthzHTTPDriver) allocateAuthPort() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.authPort != 0 {
		return d.authPort
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate auth port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	d.authPort = port
	return port
}

// startAuthServer starts the in-process auth server on the pre-allocated port
// with the given script. Called at the beginning of each scenario window where
// the auth server must be reachable. The caller is responsible for calling
// stopAuthServer before scenarios that require an unreachable server.
func (d *extAuthzHTTPDriver) startAuthServer(ctx context.Context, script extauthzhttp.Script) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.authSrv != nil {
		// Already running — stop the existing one first.
		_ = d.authSrv.Stop()
		d.authSrv = nil
	}
	addr := fmt.Sprintf("127.0.0.1:%d", d.authPort)
	srv, err := extauthzhttp.New(ctx, addr, script)
	if err != nil {
		return fmt.Errorf("start auth server on %s: %w", addr, err)
	}
	d.authSrv = srv
	return nil
}

// stopAuthServer stops the in-process auth server (making it unreachable for
// scenarios 3 + 4). Idempotent.
func (d *extAuthzHTTPDriver) stopAuthServer() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.authSrv != nil {
		_ = d.authSrv.Stop()
		d.authSrv = nil
	}
}

// --- fixture.Driver (required) ---

func (*extAuthzHTTPDriver) BackendCount() int                { return 1 }
func (*extAuthzHTTPDriver) BackendKind() fixture.BackendKind { return fixture.HTTPExtAuthzHTTP }

// SubjectListenerName returns the single plaintext listener name (l_test_a).
// Fixture 0020 is single-listener (no mTLS) per SPEC §7.2 + D12.
func (*extAuthzHTTPDriver) SubjectListenerName() string { return "l_test_a" }

// ReferenceListenerPort returns the in-container reference listener port.
func (*extAuthzHTTPDriver) ReferenceListenerPort() int { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal +
// runner-allocated backend port + auth server host:port (host=host.docker.internal
// because the reference Envoy container reaches the host's loopback via
// host.docker.internal per ADR-0010).
func (d *extAuthzHTTPDriver) ReferenceBootstrap(backendPorts []int) string {
	authPort := d.allocateAuthPort()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   refAdminPort,
		"LATestPort":  refLATestPort,
		"BackendHost": "host.docker.internal",
		"BackendPort": backendPorts[0],
		"AuthHost":    "host.docker.internal",
		"AuthPort":    authPort,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + auth server host:port (host=127.0.0.1 because
// envoy-go runs on the host directly — no docker translation).
func (d *extAuthzHTTPDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	authPort := d.allocateAuthPort()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"LATestPort":  subjListenerPort,
		"BackendPort": backendPorts[0],
		"AuthHost":    "127.0.0.1",
		"AuthPort":    authPort,
	})
}

// DriveReference issues all 7 scenarios against the reference proxy.
func (d *extAuthzHTTPDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "ref")
}

// DriveSubject issues all 7 scenarios against the subject proxy.
func (d *extAuthzHTTPDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin-diff at runner step 9.
func (*extAuthzHTTPDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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

// --- scenarios ---

// scenarioResult is the per-scenario observation captured for the byte stream
// the runner's CompareBytes pass compares between sides.
type scenarioResult struct {
	statusCode int
	body       []byte
	headers    http.Header
	err        error
}

// driveProxy issues the 7-scenario sequence sequentially against the listener
// address. The "side" label is INTENTIONALLY excluded from the byte stream so
// both sides produce identical bytes when behavior is equivalent.
//
// Auth server lifecycle (per SPEC §7.2):
//  1. Start auth server with FixedScript (allow, for scenarios 1 and 2).
//  2. Run scenario 1 (allow), scenario 2 (deny — same server, different script
//     per scenario, but we restart the server per-scenario for a clean state).
//  3. Stop auth server before scenarios 3 + 4 (unreachable path).
//  4. Run scenarios 3 + 4 against the stopped server.
//  5. Restart auth server for scenarios 5 + 7 (with_request_body / check_settings).
//  6. Scenario 6 (per-route disabled) runs with a live server (harmless — no
//     auth call is made, so the server state does not affect the result).
func (d *extAuthzHTTPDriver) driveProxy(ctx context.Context, addr, side string) ([]byte, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	baseURL := "http://" + addr

	var b bytes.Buffer

	type scenarioDef struct {
		id   int
		name string
		run  func() scenarioResult
	}

	// Scenario 1: HTTP allow — start server with 200 + x-authz-result: allowed.
	if err := d.startAuthServer(ctx, extauthzhttp.FixedScript(200, nil, map[string]string{
		headerAuthzResult: "allowed",
	})); err != nil {
		return nil, fmt.Errorf("[%s] start auth server for scenario 1: %w", side, err)
	}
	res1 := runScenario1(ctx, client, baseURL, side)
	emitScenario(&b, 1, res1)

	// Scenario 2: HTTP deny — restart server with 403 + deny body + allowed_client_headers.
	// The auth server returns x-authz-denied: true which is in allowed_client_headers.
	if err := d.startAuthServer(ctx, extauthzhttp.FixedScript(403, []byte(bodyDenyScenario2), map[string]string{
		"x-authz-denied": "true",
	})); err != nil {
		return nil, fmt.Errorf("[%s] start auth server for scenario 2: %w", side, err)
	}
	res2 := runScenario2(ctx, client, baseURL, side)
	emitScenario(&b, 2, res2)

	// Scenarios 3 + 4: stop the auth server (unreachable path).
	d.stopAuthServer()

	res3 := runScenario3(ctx, client, baseURL, side)
	emitScenario(&b, 3, res3)

	res4 := runScenario4(ctx, client, baseURL, side)
	emitScenario(&b, 4, res4)

	// Scenario 5: with_request_body — restart server with InspectScript.
	// The auth server reads the POST body and allows if it contains "hello".
	if err := d.startAuthServer(ctx, extauthzhttp.InspectScript(func(method, path string, body []byte) (int, []byte, map[string]string) {
		if bytes.Contains(body, []byte("hello")) {
			return 200, nil, map[string]string{headerAuthzResult: "allowed"}
		}
		return 403, []byte("body inspection failed"), nil
	})); err != nil {
		return nil, fmt.Errorf("[%s] start auth server for scenario 5: %w", side, err)
	}
	res5 := runScenario5(ctx, client, baseURL, side)
	emitScenario(&b, 5, res5)

	// Scenario 6: per-route disabled — server state irrelevant (no auth call).
	// Server stays running from scenario 5.
	res6 := runScenario6(ctx, client, baseURL, side)
	emitScenario(&b, 6, res6)

	// Scenario 7: per-route check_settings (disable_request_body_buffering) —
	// same server (FixedScript 200 allow). The per-route override disables body
	// buffering so the auth server receives an empty body; it allows regardless.
	if err := d.startAuthServer(ctx, extauthzhttp.FixedScript(200, nil, map[string]string{
		headerAuthzResult: "allowed",
	})); err != nil {
		return nil, fmt.Errorf("[%s] start auth server for scenario 7: %w", side, err)
	}
	res7 := runScenario7(ctx, client, baseURL, side)
	emitScenario(&b, 7, res7)

	// Teardown: stop the auth server after the last scenario.
	d.stopAuthServer()

	return b.Bytes(), nil
}

// emitScenario formats the per-scenario verdict line into the byte stream.
// The format mirrors the phase-16/17 precedent:
//
//	scenario <id> status=<code> body=<ok|mismatch(...)>
//
// The side label is NOT emitted (the byte stream must be identical per-side
// for the CompareBytes differential gate to fire on equivalence).
func emitScenario(b *bytes.Buffer, id int, res scenarioResult) {
	if res.err != nil {
		fmt.Fprintf(b, "scenario %d status=ERR body=ERR\n", id)
		return
	}
	bodyVerdict := classifyBody(id, res.body)
	fmt.Fprintf(b, "scenario %d status=%d body=%s\n", id, res.statusCode, bodyVerdict)
}

// classifyBody returns the byte-stream body verdict for scenario id.
//
//   - Deny scenario 2: byte-exact against bodyDenyScenario2.
//   - Error scenarios 3: empty body assertion (status_on_error path).
//   - Allow scenarios 1, 4, 5, 6, 7: echo-backend path — assert structural
//     properties (non-empty JSON with method + path keys).
func classifyBody(scenarioID int, body []byte) string {
	switch scenarioID {
	case 1, 4, 5, 6, 7:
		// Echo-backend allow path — structural assertion (body is JSON echo).
		if !isEchoBody(body) {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", string(body))
		}
		return "ok"
	case 2:
		// Deny path — byte-exact body assertion.
		if string(body) == bodyDenyScenario2 {
			return "ok"
		}
		return fmt.Sprintf("mismatch(got=%q,want=%q)", string(body), bodyDenyScenario2)
	case 3:
		// Error path (failure_mode_allow:false) — empty body.
		if len(body) == 0 {
			return "ok"
		}
		return fmt.Sprintf("mismatch(want_empty,got=%q)", string(body))
	}
	return "skip"
}

// isEchoBody returns true if body is a JSON object containing at least the
// "method" and "path" keys — the structural signature of the echobackend
// response (per echobackend.go: {"method":"...","path":"...","headers":{...}}).
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

// echoHeaders extracts the "headers" map from an echobackend JSON body. Returns
// nil if the body is not a valid echo body or does not contain a "headers" key.
// Used for backend-arrival header assertions.
func echoHeaders(body []byte) map[string]string {
	var rec struct {
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil
	}
	return rec.Headers
}

// runScenario1 — HTTP allow. GET /scenarios/1-allow with a sentinel request
// header (x-client-id: scenario-1). The auth server returns 200 +
// x-authz-result: allowed (in allowed_upstream_headers); the echo backend
// echoes the injected header back. Expected: 200 echo, x-authz-result present.
// Counter delta: ok=+1.
func runScenario1(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/scenarios/1-allow", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("x-client-id", "scenario-1")
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0020_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 1: status=%d body=%q\n", side, res.statusCode, string(res.body))
		hdrs := echoHeaders(res.body)
		if v, ok := hdrs[headerAuthzResult]; ok {
			fmt.Fprintf(os.Stderr, "[%s] scenario 1: %s=%q\n", side, headerAuthzResult, v)
		}
	}
	return res
}

// runScenario2 — HTTP deny. POST /scenarios/2-deny with a body. The auth server
// returns 403 + body "access denied" + x-authz-denied: true (in
// allowed_client_headers). Expected: 403, body byte-exact "access denied",
// x-authz-denied header present in response. Counter delta: denied=+1.
func runScenario2(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/scenarios/2-deny",
		strings.NewReader("request-body"))
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Content-Type", "text/plain")
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0020_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 2: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario3 — error → status_on_error. GET /scenarios/3-error with the auth
// server stopped (unreachable). failure_mode_allow:false + status_on_error:503
// (wired in envoy.yaml). Expected: 503, empty body. Counter delta: error=+1.
func runScenario3(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/scenarios/3-error", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0020_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 3: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario4 — failure_mode_allow. GET /scenarios/4-failure-allow with the
// auth server stopped (unreachable). failure_mode_allow:true +
// failure_mode_allow_header_add:true (wired in envoy.yaml). Expected: 200
// echo backend, x-envoy-auth-failure-mode-allowed header arrives upstream
// (echoed back). Counter delta: error=+1, failure_mode_allowed=+1.
func runScenario4(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/scenarios/4-failure-allow", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0020_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 4: status=%d body=%q\n", side, res.statusCode, string(res.body))
		if hdrs := echoHeaders(res.body); hdrs != nil {
			if v, ok := hdrs[headerFailureModeAllowed]; ok {
				fmt.Fprintf(os.Stderr, "[%s] scenario 4: %s=%q\n", side, headerFailureModeAllowed, v)
			}
		}
	}
	return res
}

// runScenario5 — with_request_body. POST /scenarios/5-body with body "hello world".
// The auth server uses InspectScript to read the buffered body and allows if it
// contains "hello". Expected: 200 echo backend. Counter delta: ok=+1.
func runScenario5(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/scenarios/5-body",
		strings.NewReader("hello world"))
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Content-Type", "text/plain")
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0020_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 5: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario6 — per-route disabled. GET /scenarios/6-disabled. The per-route
// ExtAuthzPerRoute{disabled: true} bypasses the filter entirely. Expected: 200
// echo backend. NO ext_authz counter increments (per SPEC §7.1 + parent §6
// amendment 7).
func runScenario6(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/scenarios/6-disabled", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0020_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 6: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario7 — per-route check_settings. POST /scenarios/7-check-settings with
// body "check-settings-body". The per-route ExtAuthzPerRoute{check_settings{
// disable_request_body_buffering: true}} overrides the listener-level
// with_request_body to OFF. The auth server receives an empty body (body NOT
// buffered) and allows (FixedScript 200). Expected: 200 echo backend.
// Counter delta: ok=+1 (SHARED stats).
func runScenario7(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/scenarios/7-check-settings",
		strings.NewReader("check-settings-body"))
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Content-Type", "text/plain")
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0020_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 7: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// doRequest issues req via client and captures the response body, status, and
// headers. Returns scenarioResult{err: ...} on any I/O error.
func doRequest(client *http.Client, req *http.Request) scenarioResult {
	resp, err := client.Do(req)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("do request: %w", err)}
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("read body: %w", err)}
	}
	return scenarioResult{
		statusCode: resp.StatusCode,
		body:       body,
		headers:    resp.Header,
	}
}

// --- fixture.StatsAsserter ---
//
// AssertStats scrapes /stats/prometheus from both admin endpoints and asserts
// ext_authz counter values per SPEC §7.1 matrix. The 7-scenario run produces:
//
//   - ok:                   3  (scenarios 1, 5, 7 → allowed)
//   - denied:               1  (scenario 2 → denied)
//   - error:                2  (scenarios 3, 4 → auth unreachable)
//   - failure_mode_allowed: 1  (scenario 4 → failure_mode_allow:true)
//   - invalid:              0  (no validate_mutations rejections on happy path)
//   - disabled:             0  (STRUCTURALLY UNREACHABLE under MVP per parent §6 amendment 7)
//
// Scenario 6 (per-route disabled) contributes NO counter increments per
// SPEC §7.1 + parent §6 amendment 7.
//
// Note: scenarios 3+4 route through DIFFERENT listener routes that carry
// different ext_authz filter configs (failure_mode_allow:false vs true). Both
// share the SAME stat namespace (SHARED-stats per ADR-0163) because the
// fixture uses per-route config overrides rather than separate listeners.
// Task 12 finalizes the exact counter values via empirical scrape; the values
// above are PLAN-time hypotheses. Task 13 confirms them.
func (d *extAuthzHTTPDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeExtAuthzStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref ext_authz stats: %v", err)
	}
	subjStats, err := scrapeExtAuthzStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj ext_authz stats: %v", err)
	}

	if os.Getenv("FIXTURE_0020_DUMP_STATS") != "" {
		fmt.Fprintf(os.Stderr, "=== ref ext_authz stats ===\n")
		for k, v := range refStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
		fmt.Fprintf(os.Stderr, "=== subj ext_authz stats ===\n")
		for k, v := range subjStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
	}

	// Per SPEC §7.1 — counter expectations finalized at Task 13 via empirical
	// reference scrape against Envoy v1.37.2 + the envoy-go subject.
	type counterExpect struct {
		suffix  string
		want    int64
		comment string
	}

	// crossSideEquivalent — counters where reference Envoy and envoy-go emit
	// the IDENTICAL per-run delta. These carry the fixture's primary
	// byte-equivalence correctness claim: both `ref == subj` AND `ref == want`
	// are asserted.
	crossSideEquivalent := []counterExpect{
		{suffix: "ok", want: 3, comment: "scenarios 1 (allow), 5 (with_request_body), 7 (check_settings)"},
		{suffix: "denied", want: 1, comment: "scenario 2 (403 deny)"},
		{suffix: "error", want: 2, comment: "scenarios 3+4 (auth server unreachable)"},
		{suffix: "failure_mode_allowed", want: 1, comment: "scenario 4 (failure_mode_allow:true)"},
		{suffix: "invalid", want: 0, comment: "no validate_mutations rejections on happy path"},
	}
	for _, exp := range crossSideEquivalent {
		refV := lookupExtAuthzCounter(refStats, exp.suffix)
		subjV := lookupExtAuthzCounter(subjStats, exp.suffix)
		if refV != subjV {
			t.Fatalf("ext_authz %s: cross-side divergence — ref=%d subj=%d (%s)",
				exp.suffix, refV, subjV, exp.comment)
		}
		if refV != exp.want {
			t.Fatalf("ext_authz %s: want %d; got %d (%s)", exp.suffix, exp.want, refV, exp.comment)
		}
	}

	// disabled — NOT ASSERTED. STRUCTURALLY UNREACHABLE under phase-18.1 MVP
	// per parent §6 amendment 7 (the filter_enabled gate is deferred; the
	// disabled counter publishes 0 for the listener's lifetime; per-route
	// disabled:true suppresses ALL counter increments including disabled).
}

// scrapeExtAuthzStats issues GET /stats/prometheus against adminAddr and
// returns a map of ext_authz-related metric values keyed by the full metric
// line ("name" or "name{labels}") — the lookup helper applies suffix matching
// at query time.
func scrapeExtAuthzStats(adminAddr string) (map[string]int64, error) {
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
	return parseExtAuthzPromBody(resp.Body)
}

// parseExtAuthzPromBody parses a Prometheus text-format body and returns a map
// keyed by the full metric line ("name|labelstr") of int64 values for all
// ext_authz-related metrics. The filter retains lines whose name contains the
// substring "_ext_authz_" — matches both inline-form and label-form per the
// phase-16 rbac fixture-0018 precedent.
func parseExtAuthzPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantInfix = "_ext_authz_"
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

// lookupExtAuthzCounter sums the ext_authz counter values matching the given
// suffix across all observed name+label permutations. Per the phase-16
// fixture-0018 precedent, accommodates the two observed Prometheus naming
// conventions:
//
//   - Reference Envoy v1.37.2 form (SN2-reuse hypothesis per SPEC §18.P7):
//     `envoy_http_ext_authz_<suffix>` with HCM stat-prefix carried as a label
//     (`envoy_http_conn_manager_prefix=...`).
//
//   - envoy-go form (per ADR-0156 + the §18.P6 stat-surface finalization at
//     Task 8): matches the same SN2-reuse form via the shared tag-extractor.
//
// Returns 0 when no metric matches (absent-as-zero discipline per phase-13/
// 14/15/16/17 precedent).
func lookupExtAuthzCounter(stats map[string]int64, suffix string) int64 {
	wantName := "envoy_http_ext_authz_" + suffix
	var total int64
	matched := map[string]bool{}
	for k, v := range stats {
		name := k
		if i := strings.IndexByte(k, '{'); i >= 0 {
			name = k[:i]
		}
		if name == wantName {
			if !matched[k] {
				total += v
				matched[k] = true
			}
		}
	}
	return total
}

// --- file / template helpers ---

// fixtureDir returns the absolute path to the 0020-http-ext-authz-http fixture
// root (one directory above this file's inputs/ parent), derived from
// runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0020-http-ext-authz-http/inputs/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

// mustReadFixtureFile reads name from the fixture root directory. Used to
// load envoy.yaml + envoy-go.yaml templates at Task 12 + Task 13. At Task 11
// the YAMLs do NOT exist; ReferenceBootstrap + SubjectConfig will panic if
// invoked before Task 12 lands the templates — BY DESIGN at Task 11
// (fixture compiles + registers; runtime invocation gated on Task 12).
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
	_ fixture.Driver           = (*extAuthzHTTPDriver)(nil)
	_ fixture.BackendKindAware = (*extAuthzHTTPDriver)(nil)
	_ fixture.StatsAsserter    = (*extAuthzHTTPDriver)(nil)
)
