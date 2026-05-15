// Package inputs registers the 0020-http-ext-authz-http fixture with the
// differential runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.ext_authz (HTTP-service mode) and reference Envoy v1.37.2
// across the seven-scenario matrix per phase 18.1 SPEC §7.1.
//
// Integration shape (three-listener fixture; plaintext HTTP/1.1 only per
// planner-time decision D12 — no TLS in phase 18.1):
//
//  1. ReferenceBootstrap renders test/fixtures/0020-http-ext-authz-http/envoy.yaml
//     with host.docker.internal (ADR-0010 STRICT_DNS) + runner-allocated backend
//     port + the in-process auth server's host:port (host=host.docker.internal
//     for reference Envoy; auth ports are allocated at driver instantiation so
//     both ReferenceBootstrap and SubjectConfig can templatize them
//     deterministically). SubjectConfig renders envoy-go.yaml with the
//     runner-allocated admin/listener ports + backend port (loopback) + auth
//     server host:port (host=127.0.0.1 for envoy-go which runs on the host).
//
//  2. DriveReferenceMulti / DriveSubjectMulti issue the identical 7-scenario
//     sequence against each proxy. The 7-scenario assertion-log byte stream is
//     emitted of the form:
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
//     §6 amendment 7) and NOT asserted. Scenario 6 (per-route disabled) MUST NOT
//     increment any ext_authz counter. Scenarios 3 + 4 (auth server unreachable
//     via the `c_authz_down` cluster wired to a closed port) each increment
//     error; scenario 4 also increments failure_mode_allowed.
//
//  4. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner step 9.
//
// # Listener topology (three listeners — settled at Task 11 review-fix)
//
// `failure_mode_allow` is a TOP-LEVEL `ExtAuthz` field; the proto exposes it
// ONLY at listener-config scope. `ExtAuthzPerRoute.check_settings` carries
// `context_extensions`, `disable_request_body_buffering`, `with_request_body`,
// and `service_override` — NOT `failure_mode_allow`. The envoy-go MVP also
// does NOT honor `service_override` (extauthz.go only parses the first three
// CheckSettings fields). Therefore scenario 3 (`failure_mode_allow:false`) and
// scenario 4 (`failure_mode_allow:true`) MUST target DIFFERENT
// listener-level configs.
//
// Topology:
//
//   - l_test_a (ref port 10020, subj port = subjListenerPort+0) — scenarios
//     1+2+5+6+7. `failure_mode_allow:false` (default). Listener-level
//     `http_service.server_uri` points at the LIVE auth server (cluster
//     c_authz). Scenarios 1+2+5+7 hit the auth server; scenario 6 carries the
//     per-route `disabled:true` override so the filter is bypassed.
//   - l_test_b (ref port 10021, subj port = subjListenerPort+1) — scenario 3.
//     `failure_mode_allow:false`, `status_on_error:503`. Listener-level
//     `http_service.server_uri` points at the UNREACHABLE port (cluster
//     c_authz_down — a port allocated at driver init and immediately closed,
//     so connect attempts fail fast → error disposition).
//   - l_test_c (ref port 10022, subj port = subjListenerPort+2) — scenario 4.
//     `failure_mode_allow:true`, `failure_mode_allow_header_add:true`.
//     Listener-level `http_service.server_uri` points at the same
//     UNREACHABLE port (cluster c_authz_down).
//
// Benefits of this topology:
//
//   - The auth server stays running across the ENTIRE workload — no
//     stop/restart for S3 + S4 (the bootstrap's c_authz_down is hardcoded
//     unreachable; nothing the driver does affects it).
//   - The driver only restarts the auth server between scenarios that need
//     different Scripts (S1 → S2 → S5 → S7; S6 is a no-op).
//
// # Auth-server lifecycle
//
// The in-process HTTP auth server (test/helpers/extauthzhttp) is started fresh
// per scenario-group that needs a different Script, on a single STABLE port
// (allocated at driver instantiation):
//
//   - Scenario 1 (l_test_a): FixedScript 200 + `x-authz-result: allowed`.
//   - Scenario 2 (l_test_a): FixedScript 403 + body "access denied" +
//     `x-authz-denied: true`.
//   - Scenarios 3 (l_test_b) + 4 (l_test_c): auth server stays running but
//     irrelevant — these listeners point at c_authz_down (unreachable port).
//   - Scenario 5 (l_test_a): InspectScript — allows iff body contains "hello".
//   - Scenario 6 (l_test_a): no auth call (per-route disabled).
//   - Scenario 7 (l_test_a): FixedScript 200 + `x-authz-result: allowed`.
//
// Task 12 wires:
//   - cluster `c_authz` → 127.0.0.1:{{.AuthPort}} (live auth server).
//   - cluster `c_authz_down` → 127.0.0.1:{{.AuthDownPort}} (hardcoded
//     unreachable port; nothing is bound there).
//   - cluster `c_backend` → 127.0.0.1:{{.BackendPort}} (live echo backend).
//   - three listeners l_test_a, l_test_b, l_test_c with the three
//     ext_authz configs described above.
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

	// In-container reference Envoy listener ports. Convention "100NN" for
	// fixture "00NN" — fixture 0020 takes 10020/10021/10022 for the three
	// plaintext listeners (one per failure_mode_allow / unreachable variant
	// per the Task-11-review-fix topology documented in the package doc).
	refAdminPort  = 9901
	refLATestPort = 10020 // l_test_a (S1+2+5+6+7; failure_mode_allow:false; c_authz live)
	refLBTestPort = 10021 // l_test_b (S3;       failure_mode_allow:false; c_authz_down unreachable)
	refLCTestPort = 10022 // l_test_c (S4;       failure_mode_allow:true;  c_authz_down unreachable)

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
// port (constant across the ref+subj run), the auth-down port (hardcoded
// unreachable for c_authz_down in the bootstrap; allocated and immediately
// closed at first use so it is guaranteed unbound), and the auth server handle.
type extAuthzHTTPDriver struct {
	mu sync.Mutex

	// authPort is allocated lazily on first use (ReferenceBootstrap or
	// SubjectConfig — whichever fires first). The same port is used for
	// both sides so envoy.yaml and envoy-go.yaml can be templatized
	// deterministically before the per-side Drive runs.
	authPort int

	// authDownPort is the c_authz_down cluster's target port for l_test_b
	// and l_test_c (scenarios 3+4). Allocated lazily by allocating a TCP
	// listener and immediately closing it — the port is then guaranteed
	// unbound for the fixture lifetime (subject to host-side TIME_WAIT /
	// re-allocation races, which the runner's parallel-fixture isolation
	// makes negligible for a single-fixture run). Hardcoded into the
	// bootstrap; nothing the driver does at runtime affects it.
	authDownPort int

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

// allocateAuthDownPort allocates a free TCP port and immediately closes the
// listener, leaving the port unbound. This is the address the cluster
// c_authz_down points at in the bootstrap (l_test_b + l_test_c scenarios 3+4).
// Idempotent. Distinct from authPort so a live auth server bound on authPort
// does not collide.
func (d *extAuthzHTTPDriver) allocateAuthDownPort() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.authDownPort != 0 {
		return d.authDownPort
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate auth-down port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	d.authDownPort = port
	return port
}

// startAuthServer starts the in-process auth server on the pre-allocated port
// with the given script. Called at the beginning of each scenario window where
// a different Script is required. Idempotent: if a server is already running,
// it is stopped first.
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

// stopAuthServer stops the in-process auth server. Idempotent. Called at
// teardown after the workload completes. NOT called between scenarios 2 and 5
// for the unreachable path (S3+S4) — those scenarios target l_test_b / l_test_c
// whose ext_authz config points at the hardcoded-unreachable c_authz_down
// cluster, so the live auth server state is irrelevant.
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

// SubjectListenerName returns the primary listener name (l_test_a). The
// runner uses this for the single-addr DriveSubject fallback; because this
// fixture implements MultiListenerDriver the runner dispatches
// DriveSubjectMulti instead. Method is REQUIRED by the Driver interface.
func (*extAuthzHTTPDriver) SubjectListenerName() string { return "l_test_a" }

// ReferenceListenerPort returns the primary reference listener port
// (l_test_a). Required by the Driver interface even though
// MultiListenerDriver takes precedence at runtime.
func (*extAuthzHTTPDriver) ReferenceListenerPort() int { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal +
// runner-allocated backend port + auth server host:port (host=host.docker.internal
// because the reference Envoy container reaches the host's loopback via
// host.docker.internal per ADR-0010). Three listener ports are wired
// (l_test_a/b/c).
func (d *extAuthzHTTPDriver) ReferenceBootstrap(backendPorts []int) string {
	authPort := d.allocateAuthPort()
	authDownPort := d.allocateAuthDownPort()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    refAdminPort,
		"LATestPort":   refLATestPort,
		"LBTestPort":   refLBTestPort,
		"LCTestPort":   refLCTestPort,
		"BackendHost":  "host.docker.internal",
		"BackendPort":  backendPorts[0],
		"AuthHost":     "host.docker.internal",
		"AuthPort":     authPort,
		"AuthDownHost": "host.docker.internal",
		"AuthDownPort": authDownPort,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + auth server host:port (host=127.0.0.1 because
// envoy-go runs on the host directly — no docker translation). The three
// subject listeners take consecutive ports starting at subjListenerPort:
// LA=subjListenerPort, LB=subjListenerPort+1, LC=subjListenerPort+2 — mirrors
// the phase-16 rbac (fixture-0018) port-offset pattern.
func (d *extAuthzHTTPDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	authPort := d.allocateAuthPort()
	authDownPort := d.allocateAuthDownPort()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":    subjAdminPort,
		"LATestPort":   subjListenerPort,
		"LBTestPort":   subjListenerPort + 1,
		"LCTestPort":   subjListenerPort + 2,
		"BackendPort":  backendPorts[0],
		"AuthHost":     "127.0.0.1",
		"AuthPort":     authPort,
		"AuthDownHost": "127.0.0.1",
		"AuthDownPort": authDownPort,
	})
}

// DriveReference is the single-addr Driver-interface path; never invoked at
// runtime because MultiListenerDriver is implemented. Delegates to
// DriveReferenceMulti deriving the additional addrs by reference-port
// substitution.
func (d *extAuthzHTTPDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromRef(addr)
	return d.DriveReferenceMulti(ctx, addrs)
}

// DriveSubject is the single-addr Driver-interface path; never invoked at
// runtime because MultiListenerDriver is implemented. Delegates to
// DriveSubjectMulti deriving the additional addrs by subject-port offset.
func (d *extAuthzHTTPDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromSubj(addr)
	return d.DriveSubjectMulti(ctx, addrs)
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

// --- fixture.MultiListenerDriver ---

// SubjectListenerNames returns the three subject listener names in order
// (l_test_a primary plaintext first; l_test_b second; l_test_c third). The
// order here matches ReferenceListenerPorts() — the runner zips them
// index-wise.
func (*extAuthzHTTPDriver) SubjectListenerNames() []string {
	return []string{"l_test_a", "l_test_b", "l_test_c"}
}

// ReferenceListenerPorts returns the three in-container reference listener
// ports in order matching SubjectListenerNames().
func (*extAuthzHTTPDriver) ReferenceListenerPorts() []int {
	return []int{refLATestPort, refLBTestPort, refLCTestPort}
}

// DriveReferenceMulti issues all 7 scenarios against the reference proxy.
// addrs maps listener name → "host:port" (provided by the runner).
func (d *extAuthzHTTPDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

// DriveSubjectMulti issues all 7 scenarios against the subject proxy.
// addrs maps listener name → "host:port" (provided by the runner).
func (d *extAuthzHTTPDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "subj")
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
// addresses provided in addrs. The "side" label is INTENTIONALLY excluded from
// the byte stream so both sides produce identical bytes when behavior is
// equivalent.
//
// Auth-server lifecycle (single live server on authPort, restarted only when
// the Script must change):
//  1. Start auth server with FixedScript 200 + x-authz-result (scenario 1).
//  2. Restart with FixedScript 403 + deny body (scenario 2).
//  3. Scenarios 3 + 4: NO change. Both target listeners (l_test_b, l_test_c)
//     whose ext_authz config points at c_authz_down (unreachable port). The
//     live server on authPort is untouched and irrelevant.
//  4. Restart with InspectScript (scenario 5).
//  5. Scenario 6 (per-route disabled): no auth call; server state irrelevant.
//  6. Restart with FixedScript 200 + x-authz-result (scenario 7).
//  7. Stop auth server.
func (d *extAuthzHTTPDriver) driveProxy(ctx context.Context, addrs map[string]string, side string) ([]byte, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	// Per-listener base URLs.
	baseA := "http://" + addrs["l_test_a"] // S1+2+5+6+7 (live auth)
	baseB := "http://" + addrs["l_test_b"] // S3 (auth unreachable; failure_mode_allow:false)
	baseC := "http://" + addrs["l_test_c"] // S4 (auth unreachable; failure_mode_allow:true)

	var b bytes.Buffer

	// Scenario 1: HTTP allow on l_test_a — start server with 200 + x-authz-result: allowed.
	if err := d.startAuthServer(ctx, extauthzhttp.FixedScript(200, nil, map[string]string{
		headerAuthzResult: "allowed",
	})); err != nil {
		return nil, fmt.Errorf("[%s] start auth server for scenario 1: %w", side, err)
	}
	res1 := runScenario1(ctx, client, baseA, side)
	emitScenario(&b, 1, res1)

	// Scenario 2: HTTP deny on l_test_a — restart server with 403 + deny body.
	if err := d.startAuthServer(ctx, extauthzhttp.FixedScript(403, []byte(bodyDenyScenario2), map[string]string{
		"x-authz-denied": "true",
	})); err != nil {
		return nil, fmt.Errorf("[%s] start auth server for scenario 2: %w", side, err)
	}
	res2 := runScenario2(ctx, client, baseA, side)
	emitScenario(&b, 2, res2)

	// Scenario 3: error → status_on_error on l_test_b.
	// l_test_b's ext_authz config points at c_authz_down (unreachable port);
	// the live auth server on authPort is untouched. Expected: 503 empty body.
	res3 := runScenario3(ctx, client, baseB, side)
	emitScenario(&b, 3, res3)

	// Scenario 4: failure_mode_allow on l_test_c.
	// l_test_c's ext_authz config carries failure_mode_allow:true +
	// failure_mode_allow_header_add:true. The auth call fails (unreachable);
	// the filter allows through with the marker header injected. Expected:
	// 200 echo backend, x-envoy-auth-failure-mode-allowed header arrives upstream.
	res4 := runScenario4(ctx, client, baseC, side)
	emitScenario(&b, 4, res4)

	// Scenario 5: with_request_body on l_test_a — restart server with InspectScript.
	// The auth server reads the POST body and allows if it contains "hello".
	if err := d.startAuthServer(ctx, extauthzhttp.InspectScript(func(method, path string, body []byte) (int, []byte, map[string]string) {
		if bytes.Contains(body, []byte("hello")) {
			return 200, nil, map[string]string{headerAuthzResult: "allowed"}
		}
		return 403, []byte("body inspection failed"), nil
	})); err != nil {
		return nil, fmt.Errorf("[%s] start auth server for scenario 5: %w", side, err)
	}
	res5 := runScenario5(ctx, client, baseA, side)
	emitScenario(&b, 5, res5)

	// Scenario 6: per-route disabled on l_test_a — no auth call (server state
	// irrelevant; left running from scenario 5).
	res6 := runScenario6(ctx, client, baseA, side)
	emitScenario(&b, 6, res6)

	// Scenario 7: per-route check_settings (disable_request_body_buffering) on
	// l_test_a — restart server with FixedScript 200 allow. The per-route
	// override disables body buffering so the auth server receives an empty
	// body; it allows regardless.
	if err := d.startAuthServer(ctx, extauthzhttp.FixedScript(200, nil, map[string]string{
		headerAuthzResult: "allowed",
	})); err != nil {
		return nil, fmt.Errorf("[%s] start auth server for scenario 7: %w", side, err)
	}
	res7 := runScenario7(ctx, client, baseA, side)
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

// runScenario1 — HTTP allow on l_test_a. GET /scenarios/1-allow with a
// sentinel request header (x-client-id: scenario-1). The auth server returns
// 200 + x-authz-result: allowed (in allowed_upstream_headers); the echo
// backend echoes the injected header back. Expected: 200 echo,
// x-authz-result present. Counter delta: ok=+1.
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

// runScenario2 — HTTP deny on l_test_a. POST /scenarios/2-deny with a body.
// The auth server returns 403 + body "access denied" + x-authz-denied: true
// (in allowed_client_headers). Expected: 403, body byte-exact "access denied",
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

// runScenario3 — error → status_on_error on l_test_b. GET /scenarios/3-error
// against l_test_b which carries failure_mode_allow:false + status_on_error:503
// and points at the unreachable c_authz_down cluster (Task 12). Expected: 503,
// empty body. Counter delta: error=+1.
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

// runScenario4 — failure_mode_allow on l_test_c. GET /scenarios/4-failure-allow
// against l_test_c which carries failure_mode_allow:true +
// failure_mode_allow_header_add:true and points at the unreachable
// c_authz_down cluster (Task 12). Expected: 200 echo backend, the
// x-envoy-auth-failure-mode-allowed header arrives upstream (echoed back).
// Counter delta: error=+1, failure_mode_allowed=+1.
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

// runScenario5 — with_request_body on l_test_a. POST /scenarios/5-body with
// body "hello world". The auth server uses InspectScript to read the buffered
// body and allows if it contains "hello". Expected: 200 echo backend. Counter
// delta: ok=+1.
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

// runScenario6 — per-route disabled on l_test_a. GET /scenarios/6-disabled.
// The per-route ExtAuthzPerRoute{disabled: true} bypasses the filter entirely.
// Expected: 200 echo backend. NO ext_authz counter increments (per SPEC §7.1 +
// parent §6 amendment 7).
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

// runScenario7 — per-route check_settings on l_test_a. POST
// /scenarios/7-check-settings with body "check-settings-body". The per-route
// ExtAuthzPerRoute{check_settings{disable_request_body_buffering: true}}
// overrides the listener-level with_request_body to OFF. The auth server
// receives an empty body (body NOT buffered) and allows (FixedScript 200).
// Expected: 200 echo backend. Counter delta: ok=+1 (SHARED stats).
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
// Note: scenarios 3+4 route through DIFFERENT listeners (l_test_b vs l_test_c)
// that carry different ext_authz filter configs (failure_mode_allow:false vs
// true; both point at the unreachable c_authz_down cluster). Scenarios
// 1+2+5+6+7 route through l_test_a. All three listeners share the SAME stat
// namespace (SHARED-stats per ADR-0163) because they emit to the same
// stat_prefix and the per-listener counters aggregate cluster-wide.
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

// --- address-derivation helpers (Driver-interface stubs) ---
//
// These exist for the DriveReference / DriveSubject single-addr Driver-interface
// fallbacks (never invoked at runtime because MultiListenerDriver is implemented).
// Mirrors the phase-16 fixture-0018 pattern.

// deriveAddrsFromRef derives the 2 additional listener addrs from the
// l_test_a reference container address by port substitution. The reference
// container exposes ports 10020 (l_test_a), 10021 (l_test_b), 10022
// (l_test_c). Only used by the DriveReference single-addr stub.
func deriveAddrsFromRef(s1Addr string) map[string]string {
	replace := func(addr string, fromPort, toPort int) string {
		return strings.Replace(addr,
			fmt.Sprintf(":%d", fromPort),
			fmt.Sprintf(":%d", toPort), 1)
	}
	return map[string]string{
		"l_test_a": s1Addr,
		"l_test_b": replace(s1Addr, refLATestPort, refLBTestPort),
		"l_test_c": replace(s1Addr, refLATestPort, refLCTestPort),
	}
}

// deriveAddrsFromSubj derives the 2 additional listener addrs from the
// l_test_a subject address by incrementing the port. SubjectConfig assigns
// LB=LA+1, LC=LA+2. Only used by the DriveSubject single-addr stub.
func deriveAddrsFromSubj(s1Addr string) map[string]string {
	lastColon := strings.LastIndex(s1Addr, ":")
	if lastColon < 0 {
		return map[string]string{
			"l_test_a": s1Addr,
			"l_test_b": s1Addr,
			"l_test_c": s1Addr,
		}
	}
	hostPart := s1Addr[:lastColon]
	portStr := s1Addr[lastColon+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return map[string]string{
			"l_test_a": s1Addr,
			"l_test_b": s1Addr,
			"l_test_c": s1Addr,
		}
	}
	return map[string]string{
		"l_test_a": s1Addr,
		"l_test_b": fmt.Sprintf("%s:%d", hostPart, port+1),
		"l_test_c": fmt.Sprintf("%s:%d", hostPart, port+2),
	}
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
	_ fixture.Driver              = (*extAuthzHTTPDriver)(nil)
	_ fixture.BackendKindAware    = (*extAuthzHTTPDriver)(nil)
	_ fixture.MultiListenerDriver = (*extAuthzHTTPDriver)(nil)
	_ fixture.StatsAsserter       = (*extAuthzHTTPDriver)(nil)
)
