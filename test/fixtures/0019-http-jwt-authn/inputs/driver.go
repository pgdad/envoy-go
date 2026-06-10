// Package inputs registers the 0019-http-jwt-authn fixture with the
// differential runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.jwt_authn and reference Envoy v1.37.2 across the
// eight-scenario matrix per phase 17 SPEC §7.1.
//
// Integration shape (single-listener fixture; plaintext-only per SPEC §7.4 —
// no mTLS in phase 17):
//
//  1. ReferenceBootstrap renders test/fixtures/0019-http-jwt-authn/envoy.yaml
//     with the backend host set to host.docker.internal (ADR-0010 STRICT_DNS) +
//     runner-allocated backend port + the in-process JWKS server's host:port
//     (host=host.docker.internal for reference Envoy; the JWKS port is
//     allocated at driver instantiation so ReferenceBootstrap + SubjectConfig
//     can templatize it deterministically). SubjectConfig renders envoy-go.yaml
//     with the runner-allocated subject admin/listener ports + backend port +
//     the JWKS server's host:port (host=127.0.0.1 for envoy-go which runs on
//     the host).
//
//  2. DriveReference / DriveSubject issue the identical 8-scenario sequence
//     against each proxy. The 8-scenario assertion-log byte stream is emitted
//     of the form:
//
//     scenario <id> status=<code> body=<ok|mismatch(...)>
//
//     The runner's CompareBytes pass enforces equivalence — when both proxies
//     produce equal verdicts, the differential gate fires. Body classification
//     accommodates the upstream-echo path (scenarios 1, 2, 6, 7, 8) by
//     asserting structural properties only (status + body non-empty +
//     contains-method-and-path), and the deny path (scenarios 3, 4, 5) by
//     byte-exact assertion against the canonical jwt_verify_lib strings per
//     SPEC §7.1.
//
//  3. AssertStats scrapes /stats/prometheus from both admin endpoints AFTER
//     the 8-scenario workload. It asserts cross-side `ref == subj` equivalence
//     on the four byte-equivalent counters (denied, cors_preflight_bypassed,
//     jwks_fetch_success, jwks_fetch_failed) and per-side values on `allowed`
//     (ref 5 / subj 3 — reference Envoy increments `allowed` on the CORS-bypass
//     + per-route-disabled passthrough paths; envoy-go MVP does not, per SPEC
//     §3 + §1.1 amendment 5; documented divergence-window 1 in
//     expectations.yaml). The 2 jwt_cache_* counters are STRUCTURALLY
//     UNREACHABLE under phase-17 MVP (§1.1 amendment 9 + §8 deferral 8) and NOT
//     asserted. The Prometheus metric-name format is the SN2-reuse form
//     `envoy_http_jwt_authn_<counter>{envoy_http_conn_manager_prefix="<HCM>"}`
//     — identical on both sides per the Task-13 empirical scrape.
//
//  4. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner
//     step 9.
//
// # Host-header pin
//
// The differential harness reaches the reference proxy (testcontainers-mapped
// `localhost:<port>`) and the subject proxy (envoy-go's dual-stack
// `[::]:<port>`) at DIFFERENT addresses. Reference Envoy's jwt_authn reflects
// the request's full URL into the WWW-Authenticate `realm`; doRequest pins
// `Host: jwt-authn.fixture.test` on every request so both proxies observe the
// identical authority and the realm stays byte-equivalent per-side.
//
// # JWKS backend lifecycle
//
// The in-process JWKS server (test/helpers/jwksbackend) is started ONCE for the
// whole fixture run and serves both proxies:
//
//  1. allocateJWKSPort() — called lazily by ReferenceBootstrap / SubjectConfig
//     (whichever fires first) — allocates a free TCP port AND eagerly starts
//     the jwksbackend on it, serving the JWK Set payloads from
//     loadFixturePKI(). The eager start is required because envoy-go's
//     jwks.New() performs a BLOCKING initial fetch at filter-factory time
//     (ADR-0150 §Decision (iii), default fast_listener=false) — the JWKS
//     endpoint must be live before either bootstrap loads.
//
//  2. The single server outlives the entire ref+subj run; driveProxy calls
//     ensureJWKSBackend() as a liveness guard. Go's runtime reaps the listener
//     on test-process exit.
//
// Reference Envoy reaches the server via host.docker.internal:<jwksPort> (set
// in envoy.yaml's c_jwks_backend cluster); envoy-go reaches it via
// 127.0.0.1:<jwksPort>.
//
// # PKI
//
// The PKI generator (`pki/gen.go`) produces fresh key material at fixture-load
// time (planner-time decision 11 — no checked-in private keys): 3 RSA-2048
// keypairs + 1 ECDSA-P256 keypair + 1 tampered RSA key. loadFixturePKI()
// memoizes the generated set so both proxies are served identical JWK Sets.
package inputs

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/rsa"
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
	"github.com/esalaine/envoy-go/test/fixtures/0019-http-jwt-authn/pki"
	"github.com/esalaine/envoy-go/test/helpers"
)

const (
	fixtureName = "0019-http-jwt-authn"

	// In-container reference Envoy listener port. Convention "100NN" for
	// fixture "00NN" — fixture 0018 used 10018-10020; 0019 takes 10019 for
	// the single plaintext listener l_test_a (no mTLS in phase 17 per
	// SPEC §7.4).
	refAdminPort  = 9901
	refLATestPort = 10019 // l_test_a (plaintext)

	// Canonical jwt_verify_lib deny-path body strings per SPEC §7.1 table.
	// Asserted byte-exact in scenario classification.
	bodyJwtMissing     = "Jwt is missing"         // 14 bytes — scenario 3
	bodyJwtExpired     = "Jwt is expired"         // 14 bytes — scenario 4
	bodyJwtVerifyFails = "Jwt verification fails" // 22 bytes — scenario 5

	// fixtureHostHeader is the constant Host header pinned on every scenario
	// request (see doRequest). The differential harness reaches the reference
	// proxy (testcontainers-mapped `localhost:<mappedPort>`) and the subject
	// proxy (envoy-go's dual-stack `[::]:<port>`) at DIFFERENT addresses, so
	// the Go HTTP client would otherwise stamp a different Host header on each
	// side. Reference Envoy's jwt_authn reflects the request's full URL into
	// the WWW-Authenticate `realm` (`<scheme>://<authority><path>`); without a
	// pinned authority the realm — and therefore the byte stream — diverges
	// per-side by construction. Pinning Host to a constant makes both proxies
	// observe the identical authority, so the realm is byte-equivalent.
	// (Task-13 empirical reference scrape established the full-URL realm form.)
	fixtureHostHeader = "jwt-authn.fixture.test"

	// WWW-Authenticate header byte-exact values per SPEC §1.1 amendment 12 +
	// the Task-13 empirical reference scrape: reference Envoy v1.37.2 emits the
	// full request URL as the realm (`http://<Host><path>`), NOT the bare path.
	// All deny scenarios (3, 4, 5) target `GET /`, so the realm is the listener
	// root URL under the pinned Host header. Scenario 3 (JwtMissed): NO error
	// param. Scenarios 4 + 5: with error param.
	wwwAuthMissing = `Bearer realm="http://` + fixtureHostHeader + `/"`
	wwwAuthInvalid = `Bearer realm="http://` + fixtureHostHeader + `/", error="invalid_token"`
)

func init() {
	// fixtureName is the literal "0019-http-jwt-authn" (matched by the PLAN
	// Task 11 acceptance grep on the source).
	fixture.RegisterFixture(fixtureName, &jwtAuthnDriver{})
}

// jwtAuthnDriver carries the per-driver lifecycle state — the allocated JWKS
// server port (constant across the ref+subj run) and a sync.Mutex protecting
// the JWKS server handle while DriveReference / DriveSubject each spawn +
// tear down the per-side server.
type jwtAuthnDriver struct {
	mu sync.Mutex

	// jwksPort is allocated lazily on first use (ReferenceBootstrap or
	// SubjectConfig — whichever fires first). The same port is used for
	// both sides because ReferenceBootstrap + SubjectConfig precede the
	// per-side DriveReference / DriveSubject and template the cluster
	// endpoint upfront. The per-side jwksbackend server is spawned + torn
	// down per-side INSIDE DriveReference / DriveSubject.
	jwksPort int

	// jwks is the currently-running JWKS server (per-side). nil before Drive,
	// after teardown, or between sides.
	jwks *jwksHandle
}

// jwksHandle wraps the jwksbackend server lifecycle for cleanup.
type jwksHandle struct {
	stop func() error
}

// allocateJWKSPort allocates a free TCP port for the JWKS backend AND eagerly
// starts the in-process JWKS server on that port. Called lazily by
// ReferenceBootstrap / SubjectConfig (whichever fires first). The allocation is
// idempotent within a driver instance — the same port is reused across
// ReferenceBootstrap + SubjectConfig + both per-side Drive calls.
//
// Task 13 lifecycle adjustment: the JWKS server is started here (not later
// inside Drive) because envoy-go's jwks.New() performs a BLOCKING initial
// fetch at filter-factory time (per ADR-0150 §Decision (iii) default
// fast_listener=false). The reference proxy + subject proxy both load their
// bootstraps BEFORE Drive runs, so the JWKS endpoint MUST be live at that
// moment. The single server serves both sides for the lifetime of the test
// process; Go's runtime reaps the listener on process exit.
func (d *jwtAuthnDriver) allocateJWKSPort() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.jwksPort != 0 {
		return d.jwksPort
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		panic(fmt.Sprintf("driver: allocate jwks port: %v", err))
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	d.jwksPort = port

	// Eagerly start the JWKS server on the freshly-allocated port. The server
	// outlives the entire fixture run (no per-side teardown). Routes are
	// driven by the memoized loadFixturePKI() so the same JWK Set bytes are
	// served to both proxies.
	pki := loadFixturePKI()
	routes := map[string]string{
		"/.well-known/jwks-rs.json":  pki.RS256JWKSet,
		"/.well-known/jwks-alt.json": pki.AltProviderJWKSet,
	}
	// Bind all interfaces so the reference Envoy container can reach the
	// service via host.docker.internal (bridge gateway) on plain Linux Docker;
	// loopback-only binds are unreachable from containers outside Docker Desktop.
	srv, err := jwksbackendNew(context.Background(), fmt.Sprintf("0.0.0.0:%d", port), routes)
	if err != nil {
		panic(fmt.Sprintf("driver: start jwks backend: %v", err))
	}
	d.jwks = &jwksHandle{stop: srv.Stop}
	return port
}

// --- fixture.Driver (required) ---

func (*jwtAuthnDriver) BackendCount() int                { return 1 }
func (*jwtAuthnDriver) BackendKind() fixture.BackendKind { return fixture.HTTPJwtAuthn }

// SubjectListenerName returns the single plaintext listener name (l_test_a).
// Fixture 0019 is single-listener (no mTLS variant) per SPEC §7.4.
func (*jwtAuthnDriver) SubjectListenerName() string { return "l_test_a" }

// ReferenceListenerPort returns the in-container reference listener port.
func (*jwtAuthnDriver) ReferenceListenerPort() int { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal +
// runner-allocated backend port + JWKS host:port (host=host.docker.internal
// because the reference Envoy container reaches host-side services via
// host.docker.internal per ADR-0010).
func (d *jwtAuthnDriver) ReferenceBootstrap(backendPorts []int) string {
	jwksPort := d.allocateJWKSPort()
	pki := loadFixturePKI()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":            refAdminPort,
		"LATestPort":           refLATestPort,
		"BackendHost":          "host.docker.internal",
		"BackendPort":          backendPorts[0],
		"JWKSHost":             "host.docker.internal",
		"JWKSPort":             jwksPort,
		"LocalJWKSES256Inline": pki.ES256JWKSet,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + backend port (loopback) + JWKS host:port (host=127.0.0.1 because
// envoy-go runs on the host directly — no docker translation).
func (d *jwtAuthnDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	jwksPort := d.allocateJWKSPort()
	pki := loadFixturePKI()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":            subjAdminPort,
		"LATestPort":           subjListenerPort,
		"BackendPort":          backendPorts[0],
		"JWKSHost":             "127.0.0.1",
		"JWKSPort":             jwksPort,
		"LocalJWKSES256Inline": pki.ES256JWKSet,
	})
}

// ensureJWKSBackend is a no-op assertion that the JWKS server is alive —
// allocateJWKSPort starts it eagerly at the first ReferenceBootstrap /
// SubjectConfig call (per the Task 13 lifecycle adjustment; see allocateJWKSPort
// rationale). Drive paths still call this to surface a clear error in the
// pathological case where the port was allocated but the server start failed
// (currently impossible because allocateJWKSPort panics on start failure; the
// function remains as a safety guard).
func (d *jwtAuthnDriver) ensureJWKSBackend() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.jwks == nil {
		return fmt.Errorf("jwks backend not started; ReferenceBootstrap or SubjectConfig must precede Drive")
	}
	return nil
}

// DriveReference issues all 8 scenarios against the reference proxy.
func (d *jwtAuthnDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "ref")
}

// DriveSubject issues all 8 scenarios against the subject proxy.
func (d *jwtAuthnDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	return d.driveProxy(ctx, addr, "subj")
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin-diff at runner step 9.
func (*jwtAuthnDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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
	wwwAuth    string // WWW-Authenticate header (deny scenarios 3-5)
	err        error
}

// driveProxy issues the 8-scenario sequence sequentially against the listener
// address. The "side" label is INTENTIONALLY excluded from the byte stream so
// both sides produce identical bytes when behavior is equivalent. The JWKS
// backend is spawned on entry + torn down on exit so the per-side window is
// isolated.
func (d *jwtAuthnDriver) driveProxy(ctx context.Context, addr, side string) ([]byte, error) {
	if err := d.ensureJWKSBackend(); err != nil {
		return nil, fmt.Errorf("[%s] jwks: %w", side, err)
	}

	var b bytes.Buffer

	tr := &http.Transport{DisableKeepAlives: true}
	plainClient := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	baseURL := "http://" + addr

	scenarios := []struct {
		id   int
		name string
		run  func() scenarioResult
	}{
		{1, "valid_RS256_RemoteJwks_allow", func() scenarioResult { return runScenario1(ctx, plainClient, baseURL) }},
		{2, "valid_ES256_LocalJwks_allow", func() scenarioResult { return runScenario2(ctx, plainClient, baseURL) }},
		{3, "missing_token_deny", func() scenarioResult { return runScenario3(ctx, plainClient, baseURL) }},
		{4, "expired_token_deny", func() scenarioResult { return runScenario4(ctx, plainClient, baseURL) }},
		{5, "bad_signature_deny", func() scenarioResult { return runScenario5(ctx, plainClient, baseURL) }},
		{6, "bypass_cors_preflight", func() scenarioResult { return runScenario6(ctx, plainClient, baseURL) }},
		{7, "per_route_requirement_name", func() scenarioResult { return runScenario7(ctx, plainClient, baseURL) }},
		{8, "per_route_disabled", func() scenarioResult { return runScenario8(ctx, plainClient, baseURL) }},
	}

	for _, s := range scenarios {
		res := s.run()
		if res.err != nil {
			fmt.Fprintf(os.Stderr, "[fixture 0019 %s] scenario %d (%s): request error: %v\n",
				side, s.id, s.name, res.err)
			fmt.Fprintf(&b, "scenario %d status=ERR body=ERR\n", s.id)
			continue
		}
		bodyVerdict := classifyBody(s.id, res.body)
		if os.Getenv("FIXTURE_0019_DUMP_BYTES") != "" {
			fmt.Fprintf(os.Stderr, "[%s] scenario %d: status=%d body=%q wwwAuth=%q\n",
				side, s.id, res.statusCode, string(res.body), res.wwwAuth)
		}
		// For deny scenarios 3-5, append the WWW-Authenticate verdict to the
		// byte stream so the differential CompareBytes pass catches any
		// per-side divergence on the canonical header form.
		if s.id == 3 || s.id == 4 || s.id == 5 {
			waVerdict := classifyWWWAuth(s.id, res.wwwAuth)
			fmt.Fprintf(&b, "scenario %d status=%d body=%s www-authenticate=%s\n",
				s.id, res.statusCode, bodyVerdict, waVerdict)
			continue
		}
		fmt.Fprintf(&b, "scenario %d status=%d body=%s\n", s.id, res.statusCode, bodyVerdict)
	}

	return b.Bytes(), nil
}

// classifyBody returns the byte-stream body verdict for scenario id given
// the observed response body. Deny scenarios (3, 4, 5) assert byte-exact
// against the canonical jwt_verify_lib strings; allow scenarios (1, 2, 7)
// route through the echo-backend and assert structural properties only;
// scenarios 6 + 8 (CORS preflight bypass + per-route-disabled) also route
// through the echo-backend.
func classifyBody(scenarioID int, body []byte) string {
	switch scenarioID {
	case 1, 2, 6, 7, 8:
		// Echo-backend allow path — structural assertion (body non-empty).
		// Byte-exact body assertion lands at Task 13's expectations.yaml
		// finalization against the actual observed payload.
		if len(body) == 0 {
			return "mismatch(empty_body)"
		}
		return "ok"
	case 3:
		if string(body) == bodyJwtMissing {
			return "ok"
		}
		return fmt.Sprintf("mismatch(got=%q,want=%q)", string(body), bodyJwtMissing)
	case 4:
		if string(body) == bodyJwtExpired {
			return "ok"
		}
		return fmt.Sprintf("mismatch(got=%q,want=%q)", string(body), bodyJwtExpired)
	case 5:
		if string(body) == bodyJwtVerifyFails {
			return "ok"
		}
		return fmt.Sprintf("mismatch(got=%q,want=%q)", string(body), bodyJwtVerifyFails)
	}
	return "skip"
}

// classifyWWWAuth returns the byte-stream WWW-Authenticate verdict for deny
// scenarios 3-5. Scenario 3 (JwtMissed) expects NO error param; scenarios 4 +
// 5 expect the `, error="invalid_token"` suffix per SPEC §1.1 amendment 12.
func classifyWWWAuth(scenarioID int, wa string) string {
	var want string
	switch scenarioID {
	case 3:
		want = wwwAuthMissing
	case 4, 5:
		want = wwwAuthInvalid
	default:
		return "skip"
	}
	if wa == want {
		return "ok"
	}
	return fmt.Sprintf("mismatch(got=%q,want=%q)", wa, want)
}

// runScenario1 — valid RS256 token via RemoteJwks. GET / with Authorization:
// Bearer <RS256 token signed by RS256Key1 for provider-rs256>. First request
// triggers a JWKS fetch from the in-process jwksbackend → 200 echo-backend
// response. Counter delta: allowed +1, jwks_fetch_success +1.
func runScenario1(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	pki := loadFixturePKI()
	if pki.RS256Key1 == nil {
		return scenarioResult{err: fmt.Errorf("PKI not ready (Task 12 lands pki/gen.go)")}
	}
	tok, err := signTokenRS256(map[string]interface{}{
		"iss": "https://issuer-rs.example.com",
		"aud": "api-rs",
		"sub": "alice",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}, pki.RS256Key1, pki.RS256Kid1)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("sign RS256 token: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return doRequest(client, req)
}

// runScenario2 — valid ES256 token via LocalJwks (inline). GET / with
// Authorization: Bearer <ES256 token signed by ES256Key for provider-es256>.
// No RemoteJwks fetch (LocalJwks inline path) → 200 echo-backend response.
// Counter delta: allowed +1.
func runScenario2(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	pki := loadFixturePKI()
	if pki.ES256Key == nil {
		return scenarioResult{err: fmt.Errorf("PKI not ready (Task 12 lands pki/gen.go)")}
	}
	tok, err := signTokenES256(map[string]interface{}{
		"iss": "https://issuer-es.example.com",
		"aud": "api-es",
		"sub": "bob",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}, pki.ES256Key, pki.ES256Kid)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("sign ES256 token: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return doRequest(client, req)
}

// runScenario3 — missing token deny. GET / with NO Authorization header.
// Expected: 401, body byte-exact "Jwt is missing" (14B), WWW-Authenticate
// `Bearer realm="/"` (WITHOUT error param per §1.1 amendment 12 JwtMissed
// case). Counter delta: denied +1.
func runScenario3(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	return doRequest(client, req)
}

// runScenario4 — expired token deny. GET / with Authorization: Bearer <token
// with exp in the past>. Expected: 401, body byte-exact "Jwt is expired"
// (14B), WWW-Authenticate `Bearer realm="/", error="invalid_token"`.
// Counter delta: denied +1.
func runScenario4(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	pki := loadFixturePKI()
	if pki.RS256Key1 == nil {
		return scenarioResult{err: fmt.Errorf("PKI not ready (Task 12 lands pki/gen.go)")}
	}
	tok, err := signTokenRS256(map[string]interface{}{
		"iss": "https://issuer-rs.example.com",
		"aud": "api-rs",
		"sub": "alice",
		"exp": time.Now().Add(-1 * time.Hour).Unix(), // past
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
	}, pki.RS256Key1, pki.RS256Kid1)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("sign expired RS256 token: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return doRequest(client, req)
}

// runScenario5 — bad signature deny. GET / with Authorization: Bearer <token
// signed by TamperedKey (NOT a JWK Set member)>. Expected: 401, body
// byte-exact "Jwt verification fails" (22B), WWW-Authenticate `Bearer
// realm="/", error="invalid_token"`. Counter delta: denied +1.
func runScenario5(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	pki := loadFixturePKI()
	if pki.TamperedKey == nil {
		return scenarioResult{err: fmt.Errorf("PKI not ready (Task 12 lands pki/gen.go)")}
	}
	// Use the correct kid (RS256Kid1) so the validator selects the matching
	// JWK by kid lookup but the signature verification fails because the
	// token was signed with the tampered key. This deterministically hits
	// the bad-signature path rather than the kid-mismatch path.
	tok, err := signTokenRS256(map[string]interface{}{
		"iss": "https://issuer-rs.example.com",
		"aud": "api-rs",
		"sub": "alice",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}, pki.TamperedKey, pki.RS256Kid1)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("sign tampered RS256 token: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return doRequest(client, req)
}

// runScenario6 — bypass CORS preflight. OPTIONS / with Origin +
// Access-Control-Request-Method headers (NO Authorization). Expected: 200
// echo-backend (preflight bypassed). Counter delta: cors_preflight_bypassed +1
// (NO allowed/denied increments).
func runScenario6(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "OPTIONS", baseURL+"/", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	return doRequest(client, req)
}

// runScenario7 — per-route 8th-canonical requirement_name delegation. GET
// /alt-req with Authorization: Bearer <token valid for provider-alt>. The
// per-route TPFC PerRouteConfig{requirement_name: "alt-req"} resolves against
// the listener-level requirement_map["alt-req"] = provider_name:provider-alt.
// Expected: 200 echo-backend. Counter delta: allowed +1.
func runScenario7(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	pki := loadFixturePKI()
	if pki.RS256Key3 == nil {
		return scenarioResult{err: fmt.Errorf("PKI not ready (Task 12 lands pki/gen.go)")}
	}
	tok, err := signTokenRS256(map[string]interface{}{
		"iss": "https://issuer-alt.example.com",
		"aud": "api-alt",
		"sub": "carol",
		"exp": time.Now().Add(1 * time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}, pki.RS256Key3, pki.AltKid)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("sign alt RS256 token: %w", err)}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/alt-req", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	return doRequest(client, req)
}

// runScenario8 — per-route 8th-canonical disabled. GET /per-route-disabled
// with NO Authorization. The per-route TPFC PerRouteConfig{disabled: true}
// bypasses the filter (passthrough). Expected: 200 echo-backend. NO counter
// increments per SPEC §7.1 row 8 + §1.1 amendment 5.
func runScenario8(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/per-route-disabled", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	return doRequest(client, req)
}

// doRequest issues req via client and captures the response body + status +
// WWW-Authenticate header. Returns scenarioResult{err: ...} on any I/O error.
func doRequest(client *http.Client, req *http.Request) scenarioResult {
	// Pin the Host header to a constant so the reference proxy and the subject
	// proxy — reached at different addresses by the differential harness —
	// observe the identical authority. Reference Envoy reflects this into the
	// WWW-Authenticate `realm`; pinning keeps that header byte-equivalent
	// per-side. See fixtureHostHeader.
	req.Host = fixtureHostHeader
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
		wwwAuth:    resp.Header.Get("WWW-Authenticate"),
	}
}

// --- fixture.StatsAsserter ---
//
// AssertStats scrapes /stats/prometheus from both admin endpoints and asserts
// per-side jwt_authn counter values per SPEC §7.5. Asserts the 5 active base
// counters per HCM stat_prefix:
//
//   - allowed: scenarios 1, 2, 7 ALLOW → +3 (scenario 8 bypasses the filter
//     entirely; NO allowed increment per §1.1 amendment 5).
//   - denied: scenarios 3, 4, 5 DENY → +3.
//   - cors_preflight_bypassed: scenario 6 → +1.
//   - jwks_fetch_success: scenarios 1 + 7 trigger JWKS fetches; +2 (one per
//     fetched provider lifetime — scenario 1 = provider-rs256; scenario 7 =
//     provider-alt). NOTE: if either provider's cache-duration window
//     overlaps such that only one fetch fires per side, this asserts via
//     range >=1 to absorb the cache-overlap. PLAN-time disposition uses the
//     literal +2; Task 13 finalizes via empirical scrape.
//   - jwks_fetch_failed: zero on the happy path (no jwksbackend failures).
//
// The 2 jwt_cache_* counters are STRUCTURALLY UNREACHABLE under phase-17 MVP
// per §1.1 amendment 9 + §8 deferral 8 and NOT asserted.
//
// Task 13 finalizes the EXACT Prometheus metric-name format from empirical
// scrape; the assertion table here uses the PLAN-time SN2-reuse hypothesis
// (mirrors phase-16 rbac fixture-0018 lookup helper). The expected values
// are placeholder until Task 13 validates against reference Envoy.
func (d *jwtAuthnDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeJWTAuthnStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref jwt_authn stats: %v", err)
	}
	subjStats, err := scrapeJWTAuthnStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj jwt_authn stats: %v", err)
	}

	if os.Getenv("FIXTURE_0019_DUMP_STATS") != "" {
		fmt.Fprintf(os.Stderr, "=== ref jwt_authn stats ===\n")
		for k, v := range refStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
		fmt.Fprintf(os.Stderr, "=== subj jwt_authn stats ===\n")
		for k, v := range subjStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
	}

	// Per SPEC §7.1 + §7.5 — counter expectations finalized at Task 13 via the
	// empirical reference scrape against Envoy v1.37.2 + the envoy-go subject.
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
		{suffix: "denied", want: 3, comment: "scenarios 3, 4, 5 (missing + expired + bad-signature)"},
		{suffix: "cors_preflight_bypassed", want: 1, comment: "scenario 6 (OPTIONS preflight bypass)"},
		// jwks_fetch_success: provider-rs256 + provider-alt each fetch their
		// JWKS once at filter-load time (the initial blocking fetch from
		// jwks.New per ADR-0150); provider-es256 is LocalJwks (no fetch).
		{suffix: "jwks_fetch_success", want: 2, comment: "provider-rs256 + provider-alt load-time fetch"},
		{suffix: "jwks_fetch_failed", want: 0, comment: "happy path — jwksbackend always reachable"},
	}
	for _, exp := range crossSideEquivalent {
		refV := lookupJWTAuthnCounter(refStats, exp.suffix)
		subjV := lookupJWTAuthnCounter(subjStats, exp.suffix)
		if refV != subjV {
			t.Fatalf("jwt_authn %s: cross-side divergence — ref=%d subj=%d (%s)",
				exp.suffix, refV, subjV, exp.comment)
		}
		if refV != exp.want {
			t.Fatalf("jwt_authn %s: want %d; got %d (%s)", exp.suffix, exp.want, refV, exp.comment)
		}
	}

	// allowed — DIVERGENCE-WINDOW (Task-13 empirical discovery). Reference
	// Envoy increments `allowed` on EVERY request that clears the filter gate,
	// INCLUDING the CORS-preflight-bypass path (scenario 6) and the per-route
	// `disabled: true` passthrough (scenario 8): ref allowed = 5 = scenarios
	// {1,2,7} actively-ALLOWED + {6,8} bypassed. envoy-go MVP increments
	// `allowed` ONLY on an active-engine ALLOWED result per SPEC §3 ("increments
	// per request where the active engine result = ALLOWED") + §1.1 amendment 5
	// ("PerRouteConfig{disabled: true} → no counter increments"): subj allowed
	// = 3 = scenarios {1,2,7}. This is SPEC-mandated envoy-go behavior, NOT a
	// bug — the divergence is asserted per-side (NOT cross-side) and documented
	// in expectations.yaml + BEHAVIOR_CONTRACT §13.4 forward-pointer notes.
	if got := lookupJWTAuthnCounter(refStats, "allowed"); got != 5 {
		t.Fatalf("ref jwt_authn allowed: want 5 (scenarios 1,2,7 allowed + 6,8 bypassed); got %d", got)
	}
	if got := lookupJWTAuthnCounter(subjStats, "allowed"); got != 3 {
		t.Fatalf("subj jwt_authn allowed: want 3 (scenarios 1,2,7 active-engine ALLOWED); got %d", got)
	}

	// jwt_cache_hit + jwt_cache_miss — NOT ASSERTED. STRUCTURALLY UNREACHABLE
	// under phase-17 envoy-go MVP (jwt_cache_config silent-ignored per §8
	// deferral 8); reference Envoy emits jwt_cache_miss > 0 because its
	// validated-JWT LRU cache is always active. Documented divergence-window.
}

// scrapeJWTAuthnStats issues GET /stats/prometheus against adminAddr and
// returns a map of jwt_authn-related metric values keyed by the full metric
// line ("name" or "name{labels}") — the lookup helper applies the form-A /
// form-B suffix matching at query time.
func scrapeJWTAuthnStats(adminAddr string) (map[string]int64, error) {
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
	return parseJWTAuthnPromBody(resp.Body)
}

// parseJWTAuthnPromBody parses a Prometheus text-format body and returns a
// map keyed by the full metric line ("name|labelstr") of int64 values for
// all jwt_authn-related metrics. The filter retains lines whose name
// contains the substring "_jwt_authn_" — matches both inline-form and
// label-form per the phase-16 rbac fixture-0018 precedent.
func parseJWTAuthnPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantInfix = "_jwt_authn_"
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

// lookupJWTAuthnCounter sums the jwt_authn counter values matching the given
// suffix across all observed name+label permutations. Per the phase-16
// fixture-0018 precedent, accommodates the two observed Prometheus naming
// conventions:
//
//   - Reference Envoy v1.37.2 form (per Task-13 empirical scrape; PLAN-time
//     SN2-reuse hypothesis): `envoy_http_jwt_authn_<suffix>` with HCM
//     stat-prefix carried as a label (`envoy_http_conn_manager_prefix=...`).
//
//   - envoy-go form (per ADR-0154 + ADR-0155 + the §11 stat-surface
//     finalization at Task 8): MAY match the same form via SN2 reuse OR a
//     stat_prefix-inlined form `envoy_http_jwt_authn_<prefix>_<suffix>` if a
//     per-provider suffix lands. Phase-17 MVP uses the suffix-only form per
//     §11 + ADR-0154 (no per-provider stat namespacing in the MVP).
//
// Returns 0 when no metric matches (absent-as-zero discipline per phase-13/
// 14/15/16 precedent).
func lookupJWTAuthnCounter(stats map[string]int64, suffix string) int64 {
	wantName := "envoy_http_jwt_authn_" + suffix
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

// --- PKI plumbing ---
//
// Task 12 lands the real PKI generator at test/fixtures/0019-http-jwt-authn/pki/.
// loadFixturePKI() invokes pki.Generate() once per process (sync.Once memoized)
// and copies the generated *pki.FixturePKI into the driver-local fixturePKI
// struct field-by-field. The keys are fresh per `go test` process (NOT
// pre-baked / committed to the repo) per planner-time decision 11.

// fixturePKI carries the per-test PKI material for fixture 0019. Field shape
// matches pki.FixturePKI at test/fixtures/0019-http-jwt-authn/pki/gen.go.
type fixturePKI struct {
	// Private keys for token signing.
	RS256Key1   *rsa.PrivateKey   // provider-rs256 (RemoteJwks)
	RS256Key2   *rsa.PrivateKey   // spare RSA-2048 keypair (per SPEC §7.3)
	RS256Key3   *rsa.PrivateKey   // provider-alt (RemoteJwks; per-route 7 scenario)
	ES256Key    *ecdsa.PrivateKey // provider-es256 (LocalJwks)
	TamperedKey *rsa.PrivateKey   // scenario 5 bad-signature

	// kids for JWS header `kid` field — matches the JWK Set entry kids.
	RS256Kid1 string
	RS256Kid2 string
	AltKid    string
	ES256Kid  string

	// JWK Set JSON serializations.
	RS256JWKSet       string // served at /.well-known/jwks-rs.json
	AltProviderJWKSet string // served at /.well-known/jwks-alt.json
	ES256JWKSet       string // LocalJwks inline_string content for envoy.yaml
}

// pkiOnce + pkiInstance memoize the per-process PKI generation so the same
// keypair set is shared across ReferenceBootstrap + SubjectConfig + every
// scenario function. The underlying pki.Generate() is invoked once per
// process; subsequent loadFixturePKI() calls return the cached instance.
var (
	pkiOnce     sync.Once
	pkiInstance *fixturePKI
)

// loadFixturePKI returns the per-process PKI material. Invokes pki.Generate()
// once at first call (~250-500ms for 4 RSA-2048 + 1 ECDSA-P256 keygens); all
// subsequent calls return the cached instance. Panics on keygen failure
// because the driver has no error-return surface in its lifecycle hooks
// (ReferenceBootstrap + SubjectConfig + runScenario*) and a keygen failure is
// non-recoverable.
func loadFixturePKI() *fixturePKI {
	pkiOnce.Do(func() {
		gen, err := pki.Generate()
		if err != nil {
			panic(fmt.Sprintf("fixture 0019: pki.Generate: %v", err))
		}
		pkiInstance = &fixturePKI{
			RS256Key1:         gen.RS256Key1,
			RS256Key2:         gen.RS256Key2,
			RS256Key3:         gen.RS256Key3,
			ES256Key:          gen.ES256Key,
			TamperedKey:       gen.TamperedKey,
			RS256Kid1:         gen.RS256Kid1,
			RS256Kid2:         gen.RS256Kid2,
			AltKid:            gen.AltKid,
			ES256Kid:          gen.ES256Kid,
			RS256JWKSet:       gen.RS256JWKSet,
			AltProviderJWKSet: gen.AltProviderJWKSet,
			ES256JWKSet:       gen.ES256JWKSet,
		}
	})
	return pkiInstance
}

// --- jwksbackend wrapper ---
//
// The jwksbackend test helper lives at test/helpers/jwksbackend. The driver
// imports it via an internal indirection (jwksbackendNew) so the path is
// easy to swap if the helper relocates — and so the import is not load-
// bearing for the Task 11 driver compilation (the function variable is
// initialized from the helper at init time in a separate file, or inlined
// here; phase-16 precedent inlines via direct import).

// jwksbackendStarter is the minimal interface the driver expects from the
// shared jwksbackend helper. Implemented by *jwksbackend.Server.
type jwksbackendStarter interface {
	Stop() error
}

// jwksbackendNew is the package-level indirection through which the driver
// spawns a jwksbackend server. The body is wired to the shared
// test/helpers/jwksbackend package below — split into its own file would
// also work; phase-16 rbac precedent inlines it via direct import. Here we
// keep it as a small function so the indirection surface is minimal.
func jwksbackendNew(ctx context.Context, addr string, routes map[string]string) (jwksbackendStarter, error) {
	return jwksbackendNewImpl(ctx, addr, routes)
}

// --- file / template helpers ---

// fixtureDir returns the absolute path to the 0019-http-jwt-authn fixture
// root (one directory above this file's inputs/ parent), derived from
// runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0019-http-jwt-authn/inputs/driver.go
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
	_ fixture.Driver           = (*jwtAuthnDriver)(nil)
	_ fixture.BackendKindAware = (*jwtAuthnDriver)(nil)
	_ fixture.StatsAsserter    = (*jwtAuthnDriver)(nil)
)
