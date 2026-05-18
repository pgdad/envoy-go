// Package inputs registers the 0024-http-oauth2 fixture with the
// differential runner. Phase-20 IMPL Task 12.
//
// Per the Task 12 scope decision documented at fixture README + PROGRESS
// Task 12 entry, this fixture is REFERENCE-LESS (`RequiresReference: false`,
// mirrors 0007b-iteration-probe). The runner short-circuits the
// reference-proxy spawn + DriveReference + byte-stream CompareBytes; only
// DriveSubject + the SubjectAsserter run.
//
// The driver:
//
//  1. Spawns an in-process oauthbackend mock per
//     test/helpers/oauthbackend/ on a free TCP port (the driver
//     pre-allocates the port via Listen+Close so the SubjectConfig
//     YAML render bakes the stable port into the c_oauth_backend
//     cluster + authorization_endpoint + token_endpoint URIs).
//  2. Materializes the SDS Secret files (hmac.json + client_secret.json)
//     by copying the canonical bytes from test/fixtures/0024-http-oauth2/
//     secrets/*.json to a per-run temp directory so the envoy-go config
//     references absolute paths the runner-spawned subject can read.
//  3. Renders envoy-go.yaml with the per-run port + path substitutions.
//  4. Drives 8 scenarios sequentially against the 2-listener topology
//     (l_test_a default-encryption + l_test_c forward_bearer_token=true);
//     captures each scenario's wire shape into a deterministic byte
//     stream (mirrors 0007b's encodeProbe).
//  5. AssertSubject performs per-scenario substring assertions against
//     the captured byte stream (mirrors 0007b's AssertSubject pattern).
//
// Per SPEC §7.1 the planner-time 9-wire-expectation matrix maps to 8
// scenarios actually landed at IMPL Task 12: (a) sign-in 302
// challenge + (b1) cookie-passthrough + (b2) tampered envelope +
// (c) pass-through bypass + (e) sign-out + (f) bad-state 401 +
// (f') POST callback PARSE-REJECT + (g) token_endpoint 5xx → 302 +
// (h) token_endpoint 4xx → 401. The 3 scenarios deferred at IMPL
// Task 12 ((d) refresh-token rotation success leg + (i)
// disable_token_encryption) are documented at fixture README +
// PROGRESS Task 12 entry as the fixture-extension forward-pointer.
package inputs

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
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
	"github.com/esalaine/envoy-go/test/helpers/oauthbackend"
)

const (
	fixtureName = "0024-http-oauth2"

	// hmacSecret matches the inline_string in secrets/hmac.json so the
	// per-test envelope HMAC validates against the same secret the
	// envoy-go subject reads from the SDS-watched file. Synchronization
	// is pinned by TestSecretsFileMatchesDriverConstant in driver_test.go
	// (to be authored alongside any future secret-rotation fixture).
	hmacSecret = "phase20-oauth2-fixture-hmac-secret-32b"
)

func init() {
	fixture.RegisterFixture(fixtureName, &oauth2Driver{})
}

// oauth2Driver carries per-driver lifecycle state.
type oauth2Driver struct {
	mu sync.Mutex

	// oauthPort: stable port for the in-process oauthbackend mock.
	// Allocated lazily by SubjectConfig.
	oauthPort int

	// oauthSrv: currently-running oauthbackend (nil before driveSubject
	// or between Stop calls and restart).
	oauthSrv *oauthbackend.Server

	// secretsDir: per-run temp directory holding the materialized
	// hmac.json + client_secret.json files.
	secretsDir string
}

// fixture.Driver interface — BackendCount + BackendKind + listener
// shapes + reference-less stubs.

func (*oauth2Driver) BackendCount() int                { return 1 }
func (*oauth2Driver) BackendKind() fixture.BackendKind { return fixture.HTTPOAuth2 }
func (*oauth2Driver) SubjectListenerName() string      { return "l_test_a" }
func (*oauth2Driver) RequiresReference() bool          { return false }

// ReferenceListenerPort + ReferenceBootstrap are defensive stubs — the
// runner short-circuits these for reference-less fixtures. Returning
// zero / empty so any future runner refactor that inadvertently calls
// them surfaces immediately as a configuration error.
func (*oauth2Driver) ReferenceListenerPort() int        { return 0 }
func (*oauth2Driver) ReferenceBootstrap(_ []int) string { return "" }
func (*oauth2Driver) DriveReference(context.Context, string) ([]byte, error) {
	return nil, nil
}

// ProbeAdmin returns nil/nil — the runner's reference-less branch does
// not invoke this hook.
func (*oauth2Driver) ProbeAdmin(context.Context, string, string) ([]byte, []byte, error) {
	return nil, nil, nil
}

// SubjectConfig templates envoy-go.yaml with the per-run port + path
// substitutions. Allocates the oauthbackend port + materializes the
// per-run SDS Secret files (the runner-spawned subject reads these
// from disk at boot via the *sdsfile.Watcher).
func (d *oauth2Driver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Allocate the oauthbackend port once per driver lifetime — the
	// fixture-runner invokes SubjectConfig exactly once per fixture
	// run.
	if d.oauthPort == 0 {
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			panic(fmt.Sprintf("driver: allocate oauthbackend port: %v", err))
		}
		d.oauthPort = ln.Addr().(*net.TCPAddr).Port
		_ = ln.Close()
	}

	// Materialize the SDS Secret files. The runner-spawned subject reads
	// these at boot via the *sdsfile.Watcher; the files must exist on
	// disk before SubjectConfig returns. The per-run temp dir is
	// cleaned up by driveSubject's defer.
	if d.secretsDir == "" {
		dir, err := os.MkdirTemp("", "fixture-0024-secrets-*")
		if err != nil {
			panic(fmt.Sprintf("driver: secrets temp dir: %v", err))
		}
		d.secretsDir = dir
		// Copy canonical secrets from fixture/secrets/*.json.
		for _, name := range []string{"hmac.json", "client_secret.json"} {
			src := filepath.Join(fixtureDir(), "secrets", name)
			data, rerr := os.ReadFile(src)
			if rerr != nil {
				panic(fmt.Sprintf("driver: read canonical %s: %v", name, rerr))
			}
			dst := filepath.Join(d.secretsDir, name)
			if werr := os.WriteFile(dst, data, 0o600); werr != nil {
				panic(fmt.Sprintf("driver: write per-run %s: %v", name, werr))
			}
		}
	}

	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"LATestPort":  subjListenerPort,
		"LCTestPort":  subjListenerPort + 1,
		"BackendPort": backendPorts[0],
		"OAuthPort":   d.oauthPort,
		"HmacPath":    filepath.Join(d.secretsDir, "hmac.json"),
		"ClientPath":  filepath.Join(d.secretsDir, "client_secret.json"),
	})
}

// SubjectListenerNames + ReferenceListenerPorts + the Multi-driver
// methods — the fixture exercises 2 listeners (l_test_a + l_test_c).
func (*oauth2Driver) SubjectListenerNames() []string {
	return []string{"l_test_a", "l_test_c"}
}

func (*oauth2Driver) ReferenceListenerPorts() []int {
	return []int{0, 0} // reference-less; never consumed
}

func (*oauth2Driver) DriveReferenceMulti(context.Context, map[string]string) ([]byte, error) {
	return nil, nil
}

// DriveSubject is a stub — the reference-less branch in the runner
// dispatches DriveSubject via the single-addr hook (not the multi
// hook). The runner short-circuits BEFORE the multi-listener
// dispatch for reference-less fixtures; the multi listener
// addrs must be resolved manually from the single subject's
// listener-name registry. See driveSubject below.
func (d *oauth2Driver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	// Derive the second listener address from the supplied first listener
	// address: the runner allocates consecutive ports for multi-listener
	// fixtures (subjListenerPort, subjListenerPort+1).
	addrs := deriveAddrsFromSubj(addr)
	return d.driveSubjectImpl(ctx, addrs)
}

// DriveSubjectMulti is the multi-listener variant — unused at the
// reference-less branch but implemented for forward-compatibility
// with a future fixture-extension to true differential.
func (d *oauth2Driver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveSubjectImpl(ctx, addrs)
}

// driveSubjectImpl is the per-scenario workload runner. Starts the
// oauthbackend mock, drives 8 scenarios, captures the wire shape
// into a deterministic byte stream, tears down.
func (d *oauth2Driver) driveSubjectImpl(ctx context.Context, addrs map[string]string) ([]byte, error) {
	if err := d.setupOAuthBackend(); err != nil {
		return nil, fmt.Errorf("setup oauthbackend: %w", err)
	}
	defer d.teardown()

	addrLA := addrs["l_test_a"]
	addrLC := addrs["l_test_c"]
	if addrLA == "" || addrLC == "" {
		return nil, fmt.Errorf("driver: missing listener addrs (l_test_a=%q l_test_c=%q)", addrLA, addrLC)
	}

	var buf bytes.Buffer

	// Scenarios fired in deterministic order; each emits a probe
	// block into buf via emitProbe.

	// (c) pass-through — X-Bypass-OAuth2: true → 200 + echo body.
	d.oauthSrv.Reset()
	res := doScenario(ctx, "GET", "http://"+addrLA+"/anything", http.Header{
		"X-Bypass-OAuth2": []string{"true"},
	}, nil)
	emitProbe(&buf, "c_pass_through", res)

	// (e) signout — GET /oauth/signout → 302 + full envelope clearing.
	d.oauthSrv.Reset()
	res = doScenario(ctx, "GET", "http://"+addrLA+"/oauth/signout", nil, nil)
	emitProbe(&buf, "e_signout", res)

	// (f) bad-state 401 — GET callback with mismatched state → 401 +
	// constant body.
	d.oauthSrv.Reset()
	res = doScenario(ctx, "GET", "http://"+addrLA+"/oauth/callback?code=c&state=mismatched", nil, nil)
	emitProbe(&buf, "f_bad_state_401", res)

	// (f') POST callback PARSE-REJECT — POST callback → 401 + constant
	// body (per SPEC §2.14 + literal D15).
	d.oauthSrv.Reset()
	res = doScenario(ctx, "POST", "http://"+addrLA+"/oauth/callback?code=c&state=s", nil, []byte(""))
	emitProbe(&buf, "fp_post_callback_parse_reject", res)

	// (a) sign-in 302-challenge wire shape — GET / with no cookies →
	// 302 + Location: authorization_endpoint + state cookie SET + 4
	// envelope cookies CLEARED.
	d.oauthSrv.Reset()
	res = doScenario(ctx, "GET", "http://"+addrLA+"/", nil, nil)
	emitProbe(&buf, "a_sign_in_challenge_wire_shape", res)

	// (g) + (h) DEFERRED per IMPL discovery: callback.go::handleCallback
	// at Task 5 ships as a SKELETON that returns StopIteration after
	// state-cookie validation succeeds; Task 10 wired the
	// oauth_client.go helpers but did NOT wire the auth-code-leg
	// outbound POST in handleCallback (the SKELETON comment at
	// callback.go:138 documents the Task 10 wire-up still pending).
	// Without the token POST firing, scenarios (g) + (h) (which
	// classify on the token POST response disposition) cannot be
	// exercised end-to-end at this fixture. Deferred to a future
	// callback-wire-up task; the auth-code-leg POST surface is
	// well-defined at compiled_config.go::tokenEndpointPoster + at
	// oauth_client.go::postTokenEndpoint, so the wire-up is a
	// dispatcher-only patch in callback.go::handleCallback.

	// (b1) cookie-passthrough + forward_bearer_token wire shape.
	// l_test_c (forward_bearer_token=true). The driver seeds a valid
	// 5-cookie envelope via oauthbackend.ValidCookieEnvelope using
	// the same hmac_secret bytes as the SDS Secret file; the
	// resulting envelope HMAC validates against the envoy-go-side
	// secret + the BearerToken AES envelope decrypts to the
	// caller-supplied plaintext.
	//
	// IMPORTANT — the filter computes HMAC with domain == the request
	// `:authority` (HTTP/2) or `Host` (HTTP/1.1) header. The Go HTTP
	// client by default sends `Host: <addr>` where addr is the
	// dialed host:port. We therefore use addrLC as the HMAC domain
	// input so the seeded envelope's HMAC matches what the filter
	// will recompute.
	{
		// expires in the FAR future so the envelope is valid.
		expires := time.Now().Add(24 * time.Hour).Unix()
		envelope := oauthbackend.ValidCookieEnvelope(
			[]byte(hmacSecret),
			"access-token-b1",
			"refresh-token-b1",
			"",     // no id_token
			addrLC, // HMAC domain = request Host
			expires,
		)
		cookieHeader := formatCookieHeader(envelope)
		res = doScenario(ctx, "GET", "http://"+addrLC+"/", http.Header{
			"Cookie": []string{cookieHeader},
		}, nil)
		emitProbe(&buf, "b1_cookie_passthrough_forward_bearer_token", res)
	}

	// (b2) cookie-passthrough tampered envelope. Seed the envelope
	// then flip a byte of the BearerToken; expected: HMAC validation
	// FAILS → category (a) 302 challenge per AMEND-3 deny-path.
	{
		expires := time.Now().Add(24 * time.Hour).Unix()
		envelope := oauthbackend.ValidCookieEnvelope(
			[]byte(hmacSecret),
			"access-token-b2",
			"refresh-token-b2",
			"",
			addrLA, // HMAC domain = request Host
			expires,
		)
		// Tamper the BearerToken cookie value: flip the first char of
		// the base64-url-encoded ciphertext envelope. The HMAC tuple
		// includes the BearerToken value; flipping a byte makes the
		// HMAC fail.
		for _, c := range envelope {
			if c.Name == "BearerToken" && len(c.Value) > 0 {
				b := []byte(c.Value)
				b[0] ^= 0x01
				// Avoid flipping into a non-base64-url byte.
				if b[0] == '=' || b[0] == '+' || b[0] == '/' {
					b[0] = 'A'
				}
				c.Value = string(b)
				break
			}
		}
		cookieHeader := formatCookieHeader(envelope)
		res = doScenario(ctx, "GET", "http://"+addrLA+"/", http.Header{
			"Cookie": []string{cookieHeader},
		}, nil)
		emitProbe(&buf, "b2_cookie_passthrough_tampered_envelope", res)
	}

	return buf.Bytes(), nil
}

// --- fixture.SubjectAsserter ---

// AssertSubject performs per-scenario substring assertions against the
// captured subject byte stream. Mirrors 0007b's AssertSubject pattern.
func (d *oauth2Driver) AssertSubject(t fixture.TB, subjBytes []byte) {
	t.Helper()
	out := string(subjBytes)

	type assertion struct {
		scenario       string
		wantStatus     int
		wantBody       string // exact body equality; empty = skip
		bodyContains   []string
		bodyNotContain []string
		setCookieAny   []string // any of these substrings must appear in a Set-Cookie header line
	}

	const (
		clearedAttrs = "; Path=/; Secure; HttpOnly; SameSite=Lax; Max-Age=0"
		baseAttrs    = "; Path=/; Secure; HttpOnly; SameSite=Lax"
	)

	assertions := []assertion{
		{
			scenario:   "c_pass_through",
			wantStatus: 200,
			// The byte stream's "body:" line is Go's %q-quoted form;
			// the echo body is a JSON map containing the request
			// `path` field. The Go-quoted substring includes the
			// backslash-escaped quote chars.
			bodyContains: []string{`\"path\"`},
		},
		{
			scenario:   "e_signout",
			wantStatus: 302,
			wantBody:   "",
			setCookieAny: []string{
				"BearerToken=" + clearedAttrs,
				"OauthHMAC=" + clearedAttrs,
				"OauthExpires=" + clearedAttrs,
				"IdToken=" + clearedAttrs,
				"RefreshToken=" + clearedAttrs,
			},
		},
		{
			scenario:   "f_bad_state_401",
			wantStatus: 401,
			wantBody:   "OAuth flow failed.",
			setCookieAny: []string{
				"BearerToken=" + clearedAttrs,
			},
		},
		{
			scenario:   "fp_post_callback_parse_reject",
			wantStatus: 401,
			wantBody:   "OAuth flow failed.",
		},
		{
			scenario:   "a_sign_in_challenge_wire_shape",
			wantStatus: 302,
			wantBody:   "",
			setCookieAny: []string{
				"BearerToken=" + clearedAttrs,
				"OauthExpires=" + baseAttrs, // state cookie SET with base attrs (no Max-Age)
			},
		},
		// (g) + (h) DEFERRED per IMPL discovery — see driveSubjectImpl
		// comment block above for the callback.go::handleCallback
		// SKELETON-not-yet-wired finding.
		{
			scenario:   "b1_cookie_passthrough_forward_bearer_token",
			wantStatus: 200,
			// echobackend reflects request headers in JSON; the
			// forward_bearer_token=true wire shape injects
			// Authorization: Bearer <plaintext>; the echobackend
			// lowercases header keys per ADR-0072. The Go-quoted
			// byte stream escapes quote chars with a backslash.
			bodyContains: []string{
				`\"path\"`,
				`\"authorization\"`,
			},
		},
		{
			scenario:   "b2_cookie_passthrough_tampered_envelope",
			wantStatus: 302, // HMAC fail → unauthenticated → category (a) 302 challenge
			wantBody:   "",
		},
	}

	for _, a := range assertions {
		header := "=== scenario " + a.scenario
		idx := strings.Index(out, header)
		if idx < 0 {
			t.Errorf("scenario %s: probe header %q not found", a.scenario, header)
			continue
		}
		blockEnd := len(out)
		if next := strings.Index(out[idx+len(header):], "\n=== scenario "); next >= 0 {
			blockEnd = idx + len(header) + next
		}
		block := out[idx:blockEnd]

		if a.wantStatus != 0 {
			want := fmt.Sprintf("status: %d", a.wantStatus)
			if !strings.Contains(block, want) {
				t.Errorf("scenario %s: status mismatch — want %q; block:\n%s",
					a.scenario, want, block)
			}
		}
		if a.wantBody != "" || (a.wantBody == "" && len(a.bodyContains) == 0) {
			// Exact body equality when wantBody is set OR (wantBody
			// empty AND no other body assertions provided).
			want := fmt.Sprintf("body: %q", a.wantBody)
			if !strings.Contains(block, want) {
				t.Errorf("scenario %s: body mismatch — want %q; block:\n%s",
					a.scenario, want, block)
			}
		}
		for _, c := range a.bodyContains {
			if !strings.Contains(block, c) {
				t.Errorf("scenario %s: body does NOT contain %q; block:\n%s",
					a.scenario, c, block)
			}
		}
		for _, c := range a.bodyNotContain {
			if strings.Contains(block, c) {
				t.Errorf("scenario %s: body unexpectedly contains %q; block:\n%s",
					a.scenario, c, block)
			}
		}
		for _, want := range a.setCookieAny {
			// Each setCookieAny entry must appear in some Set-Cookie
			// header line.
			lookup := "set-cookie: " + want
			if !strings.Contains(strings.ToLower(block), strings.ToLower(lookup)) {
				t.Errorf("scenario %s: no Set-Cookie header matches %q; block:\n%s",
					a.scenario, want, block)
			}
		}
	}
}

// --- helpers ---

// setupOAuthBackend starts the oauthbackend mock on d.oauthPort with
// default scripts pre-installed (the per-scenario re-scripts happen
// at driveSubject).
func (d *oauth2Driver) setupOAuthBackend() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopOAuthBackendLocked()

	if d.oauthPort == 0 {
		return fmt.Errorf("oauthbackend port not allocated")
	}
	srv, err := oauthbackend.NewAtAddr(fmt.Sprintf("127.0.0.1:%d", d.oauthPort))
	if err != nil {
		return err
	}
	d.oauthSrv = srv

	// Default scripts:
	//   - /authorize: 302 → callback (sign-in success leg leg seed; not
	//     exercised end-to-end at fixture scope).
	//   - /token: 200 with a stub token response (default — per-scenario
	//     re-scripts override to 5xx / 4xx).
	srv.Script("GET", "/authorize", http.StatusFound, nil, map[string]string{
		"Location": fmt.Sprintf("http://127.0.0.1:%d/oauth/callback?code=stub&state=stub", d.oauthPort),
	})
	srv.TokenResponse("/token", "stub-access-token", "stub-refresh-token", "", 3600)
	return nil
}

func (d *oauth2Driver) stopOAuthBackendLocked() {
	if d.oauthSrv != nil {
		_ = d.oauthSrv.Stop()
		d.oauthSrv = nil
	}
}

// teardown is invoked at the end of driveSubject; releases the
// oauthbackend port + cleans up the per-run secrets temp dir.
func (d *oauth2Driver) teardown() {
	d.mu.Lock()
	d.stopOAuthBackendLocked()
	dir := d.secretsDir
	d.secretsDir = ""
	d.mu.Unlock()
	if dir != "" {
		_ = os.RemoveAll(dir)
	}
}

// scenarioResult captures one HTTP round-trip.
type scenarioResult struct {
	statusCode int
	body       []byte
	headers    http.Header
	err        error
}

func doScenario(ctx context.Context, method, url string, headers http.Header, body []byte) scenarioResult {
	// Use a non-redirect-following client so 302 challenges surface as
	// observable status codes + Location headers.
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{
		Transport: tr,
		Timeout:   15 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
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

// emitProbe renders one scenario's result into the byte stream.
// Mirrors 0007b's encodeProbe format.
func emitProbe(buf *bytes.Buffer, scenario string, res scenarioResult) {
	fmt.Fprintf(buf, "=== scenario %s\n", scenario)
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

// formatCookieHeader joins cookies into a single Cookie request-header
// value per RFC 6265 §5.4.
func formatCookieHeader(cookies []*http.Cookie) string {
	parts := make([]string, 0, len(cookies))
	for _, c := range cookies {
		parts = append(parts, c.Name+"="+c.Value)
	}
	return strings.Join(parts, "; ")
}

// deriveAddrsFromSubj reconstructs the l_test_c addr from the
// runner-supplied l_test_a addr by incrementing the port. The runner
// allocates consecutive ports for multi-listener fixtures.
func deriveAddrsFromSubj(s1Addr string) map[string]string {
	lastColon := strings.LastIndex(s1Addr, ":")
	if lastColon < 0 {
		return map[string]string{"l_test_a": s1Addr, "l_test_c": s1Addr}
	}
	hostPart := s1Addr[:lastColon]
	portStr := s1Addr[lastColon+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return map[string]string{"l_test_a": s1Addr, "l_test_c": s1Addr}
	}
	return map[string]string{
		"l_test_a": s1Addr,
		"l_test_c": fmt.Sprintf("%s:%d", hostPart, port+1),
	}
}

// fixtureDir returns the absolute path to the test/fixtures/0024-http-
// oauth2/ directory (the parent of inputs/).
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
	_ fixture.Driver               = (*oauth2Driver)(nil)
	_ fixture.BackendKindAware     = (*oauth2Driver)(nil)
	_ fixture.MultiListenerDriver  = (*oauth2Driver)(nil)
	_ fixture.ReferenceLessFixture = (*oauth2Driver)(nil)
	_ fixture.SubjectAsserter      = (*oauth2Driver)(nil)
)
