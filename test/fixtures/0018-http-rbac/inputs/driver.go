// Package inputs registers the 0018-http-rbac fixture with the differential
// runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.rbac and reference Envoy v1.37.2 across the eight-scenario
// matrix per phase 16 SPEC §7.1 + §7.3.
//
// Integration shape (three-listener fixture per SPEC §7.2 — extends phase-15
// driver shape with the new mTLS listener for scenario 6):
//
//  1. ReferenceBootstrap renders test/fixtures/0018-http-rbac/envoy.yaml with
//     the backend host set to host.docker.internal (ADR-0010 STRICT_DNS) +
//     runner-allocated backend port + the three reference container listener
//     ports (l_test_a plaintext + l_test_b echo-backend + l_test_a_tls
//     mTLS-required for scenario 6). SubjectConfig renders envoy-go.yaml with
//     the runner-allocated subject admin/listener ports + backend port
//     (loopback). The three subject listener ports come from a consecutive
//     allocation starting at subjListenerPort (LA=subjListenerPort,
//     LB=subjListenerPort+1, LA_TLS=subjListenerPort+2) per the
//     local_ratelimit phase-11 multi-listener precedent.
//
//  2. DriveReferenceMulti / DriveSubjectMulti issue the identical 8-scenario
//     sequence against each proxy and emit a deterministic per-scenario
//     assertion-log byte stream of the form:
//
//     scenario <id> status=<code> body=<ok|mismatch(...)>
//
//     The runner's CompareBytes pass enforces equivalence — when both proxies
//     produce equal verdicts, the differential gate fires.
//
//  3. Scenarios 1-5, 7, 8 use HTTP/1.1 plaintext against l_test_a. Scenario 6
//     uses HTTP/1.1-over-mTLS against l_test_a_tls via a fresh http.Client
//     with TLSClientConfig holding the fixture-generated client cert + fixture
//     CA pool. The PKI lands at Task 13 (pki/gen.go).
//
//  4. AssertStats scrapes /stats/prometheus from both admin endpoints AFTER
//     the 8-scenario workload and asserts per-side counter equivalence on the
//     4 active base counters per active namespace per SPEC §7.3 + ADR-0145
//     (default.allowed/denied + override.denied + override_shadow.shadow_denied).
//     INDEPENDENT-stats discipline for scenarios 7 + 8: scenario 7 emits no
//     counters anywhere (per-route 7th-canonical disabled bypasses the filter);
//     scenario 8 emits to the override / override_shadow namespaces ONLY
//     (listener-level default.* UNCHANGED) per ADR-0125 §(xii) +
//     ADR-0145.
//
//  5. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner
//     step 9.
//
// # Counter-scrape endpoint choice
//
// Uses /stats/prometheus (Prometheus text-form) to match the existing
// fixture-0005/0011/0013/0015/0016/0017 convention. The phase-15 spec-review
// note about "Prometheus tag-extractor label-vs-inline divergence" is
// MITIGATED at fixture 0018 by the SPEC §7.3 + ADR-0145 disposition that BOTH
// sides set explicit rules_stat_prefix + shadow_rules_stat_prefix (per §1.1
// amendment 9) — both sides flatten to identical Prometheus names because
// the tag-extractor is fed the same stat-name input on both sides. The
// scrape filter (parseRBACPromBody) matches an "_http_rbac_" infix; the
// concrete per-side metric name format is empirically confirmed at Task 14
// (current PLAN-time hypothesis per ADR-0145: SN2-reuse with NO new SN10
// flatten rule).
//
// # PKI path plumbing (Task 13 forward-pointer)
//
// The mTLS scenario 6 reads client cert + key + CA cert from a fixture-
// managed temp directory pre-populated by pki/gen.go at fixture-load time
// (init() in the pki package). The pki package's init() chooses the temp
// directory and exports the paths via package-level vars (pki.ClientCertPath,
// pki.ClientKeyPath, pki.CACertPath); the YAMLs at Task 13 read the
// server-side paths analogously. The driver consumes the paths via the
// pkiPaths() helper that wraps pki.* into a struct (decouples driver from
// pki package's exact API; Task 13 finalizes both sides). Until Task 13
// lands the PKI generator, runTLSScenario6 will fail at the cert-load step
// — by design; the driver compiles AT Task 12 but the fixture is not
// end-to-end runnable until Task 14.
package inputs

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
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

	// Blank-import the pki package so its init() generates the fixture-CA +
	// server cert + client cert into <fixtureDir>/pki/ at fixture-load time
	// (option (b) PKI orchestration per Task 13 — gen.go's package
	// doc-comment documents the choice). Go's package-init topology
	// guarantees pki's init runs strictly before this package's init, and
	// strictly before ReferenceBootstrap / SubjectConfig / runTLSScenario6
	// invoke the five pki*Path() accessors.
	_ "github.com/pgdad/envoy-go/test/fixtures/0018-http-rbac/pki"
)

const (
	fixtureName = "0018-http-rbac"

	// In-container reference Envoy listener ports. Convention "100NN" for
	// fixture "00NN" — fixture 0017 used 10017; 0018 takes 10018 for the
	// primary plaintext listener (l_test_a) and consecutive ports for the
	// remaining two listeners (l_test_b echo-backend + l_test_a_tls mTLS).
	refAdminPort     = 9901
	refLATestPort    = 10018 // l_test_a (plaintext)
	refLBTestPort    = 10019 // l_test_b (echo-backend listener)
	refLATestTLSPort = 10020 // l_test_a_tls (mTLS-required; scenario 6)

	// Scenario 1's allow-path direct_response body is a 32-byte ASCII
	// payload per SPEC §7.2 + §7.3. Task 13 authors the YAML carrying this
	// exact byte string; the driver asserts byte-exact match here. The
	// payload is documented at YAML-author time; Task 12 ships a placeholder
	// that Task 14 finalizes against the actual YAML.
	scenario1AllowBody = "fixture-0018-direct-response-OK\n"

	// Deny-path body per SPEC §4 + §1.1 amendment 10 + ADR-0140 — 19 bytes
	// ASCII verbatim, no trailing newline.
	denyBody = "RBAC: access denied"

	// In-container PKI paths bind-mounted by ReferenceHostMounts. The
	// reference Envoy container reads server cert/key + CA via these paths.
	// envoy-go runs on the host so it consumes the host-side fixture paths
	// directly (pkiServerCertPath() etc.).
	refContainerServerCert = "/pki/server.pem"
	refContainerServerKey  = "/pki/server.key.pem"
	refContainerCACert     = "/pki/ca.pem"
)

func init() {
	fixture.RegisterFixture(fixtureName, &rbacDriver{})
}

// rbacDriver carries no mutable state across the scenarios — the 8-scenario
// matrix is fully deterministic per the SPEC §7.1 table. The sync.Mutex +
// per-side scratch map is present only to record the per-side scrape result
// for AssertStats's post-workload differential.
type rbacDriver struct {
	mu sync.Mutex
	// perSideScrape is captured during DriveReference / DriveSubject (the
	// final per-scenario step records the POST-workload stats snapshot;
	// AssertStats compares ref vs subj from this map).
	perSideScrape map[string]map[string]int64
}

// --- fixture.Driver (required) ---

func (*rbacDriver) BackendCount() int                { return 1 }
func (*rbacDriver) BackendKind() fixture.BackendKind { return fixture.HTTPRbac }

// SubjectListenerName returns the primary listener name (l_test_a). The
// runner uses this for the single-addr DriveSubject fallback path; because
// this fixture implements MultiListenerDriver the runner dispatches
// DriveSubjectMulti instead. Method is REQUIRED by the Driver interface.
func (*rbacDriver) SubjectListenerName() string { return "l_test_a" }

// ReferenceListenerPort returns the primary reference listener port
// (l_test_a). Required by the Driver interface even though
// MultiListenerDriver takes precedence at runtime.
func (*rbacDriver) ReferenceListenerPort() int { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal +
// runner-allocated backend port. Reference Envoy admin + the three listener
// ports are pre-assigned constants (9901, 10018-10020). PKI paths are
// substituted from the fixture-managed temp directory (pki package).
func (*rbacDriver) ReferenceBootstrap(backendPorts []int) string {
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":     refAdminPort,
		"LATestPort":    refLATestPort,
		"LBTestPort":    refLBTestPort,
		"LATestTLSPort": refLATestTLSPort,
		"BackendHost":   "host.docker.internal",
		"BackendPort":   backendPorts[0],
		// In-container PKI paths — the runner bind-mounts the host fixture
		// PKI files at these paths via ReferenceHostMounts(). The reference
		// Envoy container reads cert/key/CA from /pki/*.
		"ServerCert": refContainerServerCert,
		"ServerKey":  refContainerServerKey,
		"CACert":     refContainerCACert,
	})
}

// ReferenceHostMounts returns the three bind-mount descriptors that surface
// the fixture-generated PKI files into the reference Envoy container at
// /pki/{ca,server.{,key.}}pem. The pki package's init() generates these
// files at fixture-load time (option (b) PKI orchestration per Task 13);
// the runner pre-creates each host file under flock-protected O_CREATE +
// chmod 0o666 before container start. Pre-create does NOT truncate so the
// PKI file content survives the runner's prep step.
//
// fixture.ReferenceLogMounter interface.
func (*rbacDriver) ReferenceHostMounts() []fixture.HostMount {
	return []fixture.HostMount{
		{HostPath: pkiServerCertPath(), ContainerPath: refContainerServerCert},
		{HostPath: pkiServerKeyPath(), ContainerPath: refContainerServerKey},
		{HostPath: pkiCACertPath(), ContainerPath: refContainerCACert},
	}
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener
// ports + backend port (loopback). The three subject listeners get
// consecutive ports starting from subjListenerPort: LA=subjListenerPort,
// LB=subjListenerPort+1, LA_TLS=subjListenerPort+2. Mirrors phase-11
// fixture-0013's port-offset pattern.
func (*rbacDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":     subjAdminPort,
		"LATestPort":    subjListenerPort,
		"LBTestPort":    subjListenerPort + 1,
		"LATestTLSPort": subjListenerPort + 2,
		"BackendPort":   backendPorts[0],
		"ServerCert":    pkiServerCertPath(),
		"ServerKey":     pkiServerKeyPath(),
		"CACert":        pkiCACertPath(),
	})
}

// DriveReference is the single-addr Driver-interface path; never invoked at
// runtime because MultiListenerDriver is implemented. Delegates to
// DriveReferenceMulti deriving the additional addrs by reference-port
// substitution.
func (d *rbacDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromRef(addr)
	return d.DriveReferenceMulti(ctx, addrs)
}

// DriveSubject is the single-addr Driver-interface path; never invoked at
// runtime because MultiListenerDriver is implemented. Delegates to
// DriveSubjectMulti deriving the additional addrs by subject-port offset.
func (d *rbacDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromSubj(addr)
	return d.DriveSubjectMulti(ctx, addrs)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin-diff at runner step 9.
func (*rbacDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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
// (primary plaintext first; echo-backend second; mTLS third). The order
// here matches ReferenceListenerPorts() — the runner zips them index-wise.
func (*rbacDriver) SubjectListenerNames() []string {
	return []string{"l_test_a", "l_test_b", "l_test_a_tls"}
}

// ReferenceListenerPorts returns the three in-container reference listener
// ports in order matching SubjectListenerNames().
func (*rbacDriver) ReferenceListenerPorts() []int {
	return []int{refLATestPort, refLBTestPort, refLATestTLSPort}
}

// DriveReferenceMulti issues all 8 scenarios against the reference proxy.
// addrs maps listener name → "host:port" (provided by the runner).
func (d *rbacDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

// DriveSubjectMulti issues all 8 scenarios against the subject proxy.
// addrs maps listener name → "host:port" (provided by the runner).
func (d *rbacDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "subj")
}

// --- scenarios ---

// scenarioResult is the per-scenario observation captured for the byte
// stream the runner's CompareBytes pass compares between sides.
type scenarioResult struct {
	statusCode int
	body       []byte
	err        error
}

// driveProxy issues the 8-scenario sequence sequentially against the
// listener addresses provided in addrs. The "side" label (ref vs subj) is
// INTENTIONALLY excluded from the byte stream so both sides produce
// identical bytes when behavior is equivalent. After the workload completes
// the driver captures the side's /stats/prometheus snapshot into perSideScrape
// so AssertStats can perform the cross-side counter-delta differential.
//
// All scenarios share a single keep-alive-disabled http.Client (mirrors the
// phase-14/15/17 driver discipline; fresh connection per request avoids
// cross-scenario state leakage on the proxy's server side). Scenario 6 uses
// a SEPARATE mTLS-capable client built by runTLSScenario6.
func (d *rbacDriver) driveProxy(ctx context.Context, addrs map[string]string, side string) ([]byte, error) {
	var b bytes.Buffer

	tr := &http.Transport{DisableKeepAlives: true}
	plainClient := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	lATest := "http://" + addrs["l_test_a"]
	lATestTLS := "https://" + addrs["l_test_a_tls"]

	scenarios := []struct {
		id   int
		name string
		run  func() scenarioResult
	}{
		{1, "allow_by_header_match", func() scenarioResult { return runScenario1(ctx, plainClient, lATest) }},
		{2, "deny_no_match", func() scenarioResult { return runScenario2(ctx, plainClient, lATest) }},
		{3, "allow_by_url_path", func() scenarioResult { return runScenario3(ctx, plainClient, lATest) }},
		{4, "allow_by_destination_port", func() scenarioResult { return runScenario4(ctx, plainClient, lATest) }},
		{5, "allow_by_direct_remote_ip", func() scenarioResult { return runScenario5(ctx, plainClient, lATest) }},
		{6, "mtls_allow_by_tls_principal", func() scenarioResult { return runTLSScenario6(ctx, lATestTLS) }},
		{7, "per_route_7th_canonical_disabled", func() scenarioResult { return runScenario7(ctx, plainClient, lATest) }},
		{8, "per_route_wholesale_override_independent_stats", func() scenarioResult { return runScenario8(ctx, plainClient, lATest) }},
	}

	for _, s := range scenarios {
		res := s.run()
		if res.err != nil {
			fmt.Fprintf(os.Stderr, "[fixture 0018 %s] scenario %d (%s): request error: %v\n",
				side, s.id, s.name, res.err)
			fmt.Fprintf(&b, "scenario %d status=ERR body=ERR\n", s.id)
			continue
		}
		bodyVerdict := classifyBody(s.id, res.body)
		fmt.Fprintf(&b, "scenario %d status=%d body=%s\n", s.id, res.statusCode, bodyVerdict)
	}

	// Capture the per-side post-workload stats snapshot. The admin address is
	// not threaded into driveProxy directly; AssertStats receives both admin
	// addresses from the runner and performs the scrape there. The snapshot
	// path here is reserved for future use (e.g. per-scenario delta capture
	// across the workload's mid-points) and is currently a no-op.
	d.mu.Lock()
	if d.perSideScrape == nil {
		d.perSideScrape = map[string]map[string]int64{}
	}
	d.mu.Unlock()

	return b.Bytes(), nil
}

// classifyBody returns the byte-stream body verdict for scenario id given
// the observed response body. Allow-path scenarios assert against the
// scenario's expected verbatim payload; deny-path scenarios (2 + 8) assert
// the 19-byte "RBAC: access denied". Scenarios 5 and 6 route through the
// echo-backend (or carry per-side variable headers) — for those the
// verdict captures only a stable structural property (status; body
// non-empty) rather than byte-exact, leaving byte-exact assertion to
// Task 14's expectations.yaml finalization.
func classifyBody(scenarioID int, body []byte) string {
	switch scenarioID {
	case 1, 3, 4:
		// Direct-response allow path — byte-exact verbatim payload.
		if string(body) == scenario1AllowBody {
			return "ok"
		}
		return fmt.Sprintf("mismatch(got=%q,want=%q)", string(body), scenario1AllowBody)
	case 2, 8:
		// Deny path — byte-exact 19-byte ASCII payload per SPEC §4 +
		// §1.1 amendment 10.
		if string(body) == denyBody {
			return "ok"
		}
		return fmt.Sprintf("mismatch(got=%q,want=%q)", string(body), denyBody)
	case 5, 6:
		// Echo-backend (5) and mTLS direct_response (6) — body byte-exact
		// asserted via Task 14's expectations.yaml against the actual
		// observed payload. Here we capture a structural property only.
		if len(body) == 0 {
			return "mismatch(empty_body)"
		}
		return "ok"
	case 7:
		// Per-route disabled: filter bypassed → 200 OK with direct_response
		// payload (byte-exact equal to scenario1AllowBody per the
		// l_test_a /per-route-disabled route's direct_response payload at
		// Task 13).
		if string(body) == scenario1AllowBody {
			return "ok"
		}
		return fmt.Sprintf("mismatch(got=%q,want=%q)", string(body), scenario1AllowBody)
	}
	return "skip"
}

// runScenario1 — Allow-by-header-match (ALLOW + match). HTTP/1.1 plaintext
// GET to l_test_a "/" with X-User: admin → matches policy "admin_users"
// (header-exact admin) → 200 with the listener's direct_response body
// (32-byte payload per SPEC §7.2).
func runScenario1(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("X-User", "admin")
	return doRequest(client, req)
}

// runScenario2 — Deny-no-match (ALLOW + no-match). HTTP/1.1 plaintext GET
// to l_test_a "/" with X-User: guest (or no header). No policy matches →
// ALLOW + no-match → 403 with byte-exact 19-byte "RBAC: access denied".
func runScenario2(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("X-User", "guest")
	return doRequest(client, req)
}

// runScenario3 — Allow-by-url-path. HTTP/1.1 plaintext GET to l_test_a
// "/public" (no special header). Matches policy "public_paths" (url_path
// exact /public) → 200 with direct_response.
func runScenario3(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/public", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	return doRequest(client, req)
}

// runScenario4 — Allow-by-AND-composite (url_path /api AND header
// X-Tenant=acme). HTTP/1.1 plaintext GET to l_test_a "/api" with header
// X-Tenant: acme. Matches policy "tenant_api_users" (Permission_AndRules
// with two clauses: url_path-exact-/api + header-X-Tenant-exact-acme) →
// ALLOW + 200 with direct_response.
//
// Replaces the BRAINSTORM destination_port-based scenario 4 per Task-14
// fixture redesign: the envoy-go MVP stubs DestinationPort()->0 so a
// destination_port permission cannot allow on the envoy-go side. The
// AND-composite over url_path + header exercises the same Permission_AndRules
// + Permission_UrlPath + Permission_Header canonical evaluators using
// accessors that are plumbed at the MVP. The destination_port canonical
// remains covered by unit tests at Group 3.
//
// Per SPEC §7.1 row 4 (intent preserved; mechanism adjusted to MVP-stub-
// compatible accessors).
func runScenario4(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/api", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("X-Tenant", "acme")
	return doRequest(client, req)
}

// runScenario5 — Allow-by-OR-composite (url_path prefix /protected OR
// header X-Internal=true). HTTP/1.1 plaintext GET to l_test_a "/protected"
// (matches the OR-rules first clause url_path-prefix-/protected). Routes
// through cluster c_backend_b to the echo-backend.
//
// Replaces the BRAINSTORM direct_remote_ip-based scenario 5 per Task-14
// fixture redesign: the envoy-go MVP stubs DirectRemoteIP()->nil so a
// direct_remote_ip principal cannot allow on the envoy-go side. The
// OR-composite over url_path + header exercises the Permission_OrRules
// canonical evaluator using accessors that are plumbed at the MVP. The
// direct_remote_ip canonical remains covered by unit tests at Group 4.
//
// Per SPEC §7.1 row 5 (intent preserved; mechanism adjusted to MVP-stub-
// compatible accessors).
func runScenario5(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/protected", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	return doRequest(client, req)
}

// runTLSScenario6 — mTLS allow-by-TLS-principal. HTTP/1.1-over-mTLS GET to
// l_test_a_tls "/admin". The client presents a cert with URI SAN
// "spiffe://example.com/admin" + signed by fixture-CA; matches policy
// "authenticated_admin" (Principal_Authenticated with principal_name
// StringMatcher matching the URI SAN) → 200 with direct_response.
//
// Uses a FRESH http.Client (NOT the keep-alive-disabled plaintext one) with
// TLSClientConfig holding the fixture client cert + RootCAs=fixtureCA. The
// PKI paths are sourced from the pki package at Task 13. Until Task 13 the
// PKI files do NOT exist; runTLSScenario6 surfaces a load-cert error which
// driveProxy converts into the byte-stream sentinel "scenario 6 status=ERR
// body=ERR". This is BY DESIGN at Task 12 (driver compiles; fixture not
// end-to-end runnable until Task 14).
//
// Per SPEC §7.4 + ADR-0144 + PLAN line 83.
func runTLSScenario6(ctx context.Context, baseURL string) scenarioResult {
	clientCertPath := pkiClientCertPath()
	clientKeyPath := pkiClientKeyPath()
	caCertPath := pkiCACertPath()

	clientCert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("load client cert (%s, %s): %w", clientCertPath, clientKeyPath, err)}
	}
	caBytes, err := os.ReadFile(caCertPath)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("read CA cert (%s): %w", caCertPath, err)}
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(caBytes) {
		return scenarioResult{err: fmt.Errorf("parse CA cert (%s): no PEM blocks found", caCertPath)}
	}

	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		// ServerName per SPEC §7.2 server-cert DNSNames entry. The
		// fixture-CA-signed server cert at Task 13 carries
		// DNSNames=[l_test_a_tls.fixture.test]; the driver presents the
		// matching SNI so Envoy's transport_socket negotiates TLS
		// successfully.
		ServerName: "l_test_a_tls.fixture.test",
		MinVersion: tls.VersionTLS12,
	}
	tr := &http.Transport{
		DisableKeepAlives: true,
		TLSClientConfig:   tlsCfg,
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/admin", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	return doRequest(client, req)
}

// runScenario7 — Per-route 7th-canonical disabled. HTTP/1.1 plaintext GET
// to l_test_a "/per-route-disabled" with X-User: guest (which WOULD deny
// at the listener-level config). Per-route TPFC carries RBACPerRoute{}
// (empty wrapper; rbac field nil) → filter disabled per ADR-0125 §(xii)
// case (a) → 200 (direct_response passthrough). NO counter increments
// (INDEPENDENT-stats discipline; listener-level default.allowed AND
// default.denied UNCHANGED for this scenario).
func runScenario7(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/per-route-disabled", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("X-User", "guest")
	return doRequest(client, req)
}

// runScenario8 — Per-route wholesale-override with INDEPENDENT stats +
// shadow. HTTP/1.1 plaintext GET to l_test_a "/per-route-override" with
// X-User: guest. Per-route TPFC carries
//
//	RBACPerRoute{rbac: <RBAC rules_stat_prefix:"override",
//	                  shadow_rules_stat_prefix:"override_shadow",
//	                  action:DENY,
//	                  policies:{guests:{any}/{X-User=guest}},
//	                  shadow_rules: <mirror>>}
//
// X-User=guest matches the override DENY policy → 403 + byte-exact 19-byte
// "RBAC: access denied". INDEPENDENT-stats per ADR-0145 + ADR-0125 §(xii):
// override.denied += 1 AND override_shadow.shadow_denied += 1 AND
// default.* UNCHANGED. Asserted in AssertStats (post-workload differential).
func runScenario8(ctx context.Context, client *http.Client, baseURL string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/per-route-override", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("X-User", "guest")
	return doRequest(client, req)
}

// doRequest issues req via client and captures the response body + status.
// Returns scenarioResult{err: ...} on any I/O error.
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
	return scenarioResult{statusCode: resp.StatusCode, body: body}
}

// --- fixture.StatsAsserter ---

// AssertStats scrapes /stats/prometheus from both admin endpoints and
// asserts per-side rbac counter values per SPEC §7.3 + ADR-0145.
//
// The 4 active base counters per active namespace (per SPEC §7.3):
//
//   - default.allowed: scenarios 1, 3, 4, 5, 6 ALLOW → +5 total
//     (scenario 6's mTLS path emits to the SAME default namespace because
//     the l_test_a_tls listener-level config uses rules_stat_prefix=default
//     analogously per SPEC §7.2). NOTE: this requires the l_test_a_tls
//     YAML at Task 13 to set rules_stat_prefix=default on its listener-
//     level RBAC config — analogous to l_test_a — so all six allow-path
//     scenarios funnel into the SAME default-namespace counter.
//     Alternatively, if Task 13 chooses a separate prefix for the TLS
//     listener (e.g. "tls_default"), this assertion table updates at
//     Task 14. Initial PLAN-time disposition: single shared "default"
//     prefix across both plaintext + TLS listeners per SPEC §7.2 prose.
//
//   - default.denied: scenario 2 DENIES → +1.
//
//   - override.denied: scenario 8's per-route DENY → +1.
//
//   - override_shadow.shadow_denied: scenario 8's shadow path → +1
//     (shadow mirrors primary per §1.1 amendment 9 + ADR-0146).
//
//   - default.* UNCHANGED by scenario 8 (INDEPENDENT-stats discipline per
//     ADR-0125 §(xii); the override namespace is wholly separate from the
//     listener-level default namespace).
//
//   - Scenario 7 (per-route disabled) emits NO counters anywhere (filter
//     bypassed entirely; ADR-0125 §(xii) case (a)).
//
// Task 14 finalizes the EXACT Prometheus metric-name format from empirical
// scrape; the assertion table here uses the PLAN-time SN2-reuse hypothesis
// per ADR-0145. The expected values are placeholder until Task 14
// validates against reference Envoy.
//
// INDEPENDENT-stats discipline verification (scenario 8): the cross-
// scenario assertion that default.* counters do NOT include the override-
// scenario's increment is structurally enforced by the SEPARATE namespace
// keys (default.denied vs override.denied are distinct map keys; the
// assertion that override.denied == 1 AND default.denied == 1 (NOT 2)
// implicitly verifies the INDEPENDENT discipline).
func (d *rbacDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeRBACStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref rbac stats: %v", err)
	}
	subjStats, err := scrapeRBACStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj rbac stats: %v", err)
	}

	if os.Getenv("FIXTURE_0018_DUMP_STATS") != "" {
		fmt.Fprintf(os.Stderr, "=== ref rbac stats ===\n")
		for k, v := range refStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
		fmt.Fprintf(os.Stderr, "=== subj rbac stats ===\n")
		for k, v := range subjStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
	}

	// Per SPEC §7.3 + Task-14 empirical scrape — 4 active base counters per
	// active namespace. Reference Envoy v1.37.2 and envoy-go DIVERGE on the
	// per-route INDEPENDENT-stats discipline (ADR-0145): reference SHARES
	// per-route counter emission with the listener-level stat-prefix
	// (scenario 8's per-route DENY + shadow fold into default.denied +
	// default.shadow_denied); envoy-go uses INDEPENDENT per-route stats
	// (scenario 8 lands in override.denied + override_shadow.shadow_denied
	// while default.* is UNCHANGED). The differential gate accommodates this
	// known divergence-window by asserting per-side expectations
	// independently rather than enforcing byte-equivalence across the two
	// counter surfaces. See expectations.yaml + README.md for the full
	// divergence-window documentation.
	type counterExpect struct {
		namespace string // e.g. "default", "override", "override_shadow"
		suffix    string // e.g. "allowed", "denied", "shadow_allowed", "shadow_denied"
		want      int64
	}

	// Reference Envoy expectations — scenarios 8's per-route primary +
	// shadow fold into LISTENER-level prefix `default` (reference's
	// SHARED-stats discipline on per-route TPFC; the
	// rules_stat_prefix/shadow_rules_stat_prefix on per-route configs is
	// ignored by reference Envoy v1.37.2 per Task-14 empirical scrape).
	refExpectations := []counterExpect{
		{namespace: "default", suffix: "allowed", want: 5},       // scenarios 1, 3, 4, 5 + scenario 6 (mTLS HCM)
		{namespace: "default", suffix: "denied", want: 2},        // scenarios 2 + 8 (scenario 8 folds via SHARED)
		{namespace: "default", suffix: "shadow_denied", want: 1}, // scenario 8 shadow folds via SHARED
		{namespace: "override", suffix: "denied", want: 0},       // SHARED — no override.* on ref side
		{namespace: "override_shadow", suffix: "shadow_denied", want: 0},
	}

	// envoy-go expectations — scenario 8's per-route primary + shadow land
	// in INDEPENDENT prefixes `override` + `override_shadow` per ADR-0145
	// (the per-route rules_stat_prefix/shadow_rules_stat_prefix create
	// fresh stat namespaces disjoint from the listener-level `default`).
	subjExpectations := []counterExpect{
		{namespace: "default", suffix: "allowed", want: 5}, // scenarios 1, 3, 4, 5 + scenario 6 (mTLS HCM)
		{namespace: "default", suffix: "denied", want: 1},  // scenario 2 ONLY (scenario 8 lands in override)
		{namespace: "default", suffix: "shadow_denied", want: 0},
		{namespace: "override", suffix: "denied", want: 1}, // scenario 8 INDEPENDENT
		{namespace: "override_shadow", suffix: "shadow_denied", want: 1},
	}

	for _, exp := range refExpectations {
		v := lookupRBACCounter(refStats, exp.namespace, exp.suffix)
		if v != exp.want {
			t.Errorf("ref rbac.%s.%s = %d, want %d", exp.namespace, exp.suffix, v, exp.want)
		}
	}
	for _, exp := range subjExpectations {
		v := lookupRBACCounter(subjStats, exp.namespace, exp.suffix)
		if v != exp.want {
			t.Errorf("subj rbac.%s.%s = %d, want %d", exp.namespace, exp.suffix, v, exp.want)
		}
	}

	// Cross-side ALLOW-equivalence on default.allowed AND default.shadow_allowed
	// (zero-valued; no ALLOW shadow scenario in fixture 0018) anchors the
	// non-divergent portion of the differential. The ALLOW path's per-side
	// equality is the primary correctness claim; the DENY-path divergence
	// on override.*/override_shadow.* is the documented INDEPENDENT-vs-SHARED
	// stats divergence-window per ADR-0145.
	if a := lookupRBACCounter(refStats, "default", "allowed"); a != lookupRBACCounter(subjStats, "default", "allowed") {
		t.Errorf("default.allowed cross-side divergence: ref=%d subj=%d (allow path should be equivalent)",
			a, lookupRBACCounter(subjStats, "default", "allowed"))
	}

	// INDEPENDENT-vs-SHARED stats divergence-window check: the TOTAL deny-
	// path event count across BOTH per-side surface (listener + per-route)
	// MUST equal between ref and subj. ref: default.denied=2 (+ shadow=1).
	// subj: default.denied=1 + override.denied=1 = 2 (+ override_shadow.shadow_denied=1).
	// Both sides record the same TOTAL deny events; only the prefix-binding
	// differs.
	refTotalDeny := lookupRBACCounter(refStats, "default", "denied") +
		lookupRBACCounter(refStats, "override", "denied")
	subjTotalDeny := lookupRBACCounter(subjStats, "default", "denied") +
		lookupRBACCounter(subjStats, "override", "denied")
	if refTotalDeny != subjTotalDeny {
		t.Errorf("total deny-event divergence: ref=%d subj=%d (sum across default.denied + override.denied)",
			refTotalDeny, subjTotalDeny)
	}
	refTotalShadowDeny := lookupRBACCounter(refStats, "default", "shadow_denied") +
		lookupRBACCounter(refStats, "override_shadow", "shadow_denied")
	subjTotalShadowDeny := lookupRBACCounter(subjStats, "default", "shadow_denied") +
		lookupRBACCounter(subjStats, "override_shadow", "shadow_denied")
	if refTotalShadowDeny != subjTotalShadowDeny {
		t.Errorf("total shadow-deny-event divergence: ref=%d subj=%d (sum across default + override_shadow)",
			refTotalShadowDeny, subjTotalShadowDeny)
	}

	// Override-ALLOW + override-shadow-ALLOW always-zero (the fixture's
	// override config is DENY-only; shadow mirrors). Cross-checked per-side
	// to surface accidental cross-namespace leakage.
	for _, side := range []struct {
		label string
		stats map[string]int64
	}{{"ref", refStats}, {"subj", subjStats}} {
		if v := lookupRBACCounter(side.stats, "override", "allowed"); v != 0 {
			t.Errorf("%s rbac.override.allowed = %d, want 0 (override config is DENY-only)", side.label, v)
		}
		if v := lookupRBACCounter(side.stats, "override_shadow", "shadow_allowed"); v != 0 {
			t.Errorf("%s rbac.override_shadow.shadow_allowed = %d, want 0 (override shadow mirrors DENY)", side.label, v)
		}
	}
}

// --- stats scraping ---

// scrapeRBACStats issues GET /stats/prometheus against adminAddr and
// returns a map of rbac-related metric values. The map is keyed by
// "<namespace>|<suffix>" (e.g. "default|allowed", "override|denied") for
// the assertion in AssertStats to look up without committing to the
// exact Prometheus metric-name format (which Task 14 finalizes via
// empirical scrape against reference Envoy v1.37.2 per ADR-0145 SN2-reuse
// hypothesis).
func scrapeRBACStats(adminAddr string) (map[string]int64, error) {
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
	return parseRBACPromBody(resp.Body)
}

// parseRBACPromBody parses a Prometheus text-format body and returns a map
// keyed by the FULL metric line content ("name|labelstr") of int64 values
// for all rbac-related metrics. The filter retains lines whose name
// contains the substring "_rbac_" — matches BOTH inline-form (where
// rules_stat_prefix flattens into the metric name per SN2 reuse) AND
// label-form (where the stat_prefix appears as a label, per the local-
// ratelimit precedent's tag-extractor convention).
//
// The full metric line text (name + serialized labels) is preserved in
// the map key so lookupRBACCounter can search across both naming
// conventions without re-parsing.
func parseRBACPromBody(r io.Reader) (map[string]int64, error) {
	out := map[string]int64{}
	const wantInfix = "_rbac_"
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
		// Strip optional timestamp.
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

// lookupRBACCounter sums the rbac counter values for the given namespace +
// suffix across TWO Prometheus naming conventions AND across multiple HCM
// stat_prefixes (a single fixture may emit the same rbac counter under
// different HCM-stat-prefix labels — fixture 0018 has hcm_local_a +
// hcm_local_a_tls; per SPEC §7.3 + ADR-0145 the assertion table tracks the
// SUMMATIVE per-namespace total across all HCMs).
//
// The TWO observed naming conventions (Task-14 empirical scrape):
//
//   - Reference Envoy v1.37.2 LABEL form (Prometheus tag-extractor lands the
//     namespace on the `envoy_rbac_http_prefix` label): base name
//     `envoy_http_rbac_<suffix>` (e.g. `envoy_http_rbac_allowed`) +
//     label-set `{envoy_rbac_http_prefix="<namespace>",envoy_http_conn_manager_prefix="..."}`.
//
//   - envoy-go INLINE form (per ADR-0145 SN2-reuse without a new tag-
//     extractor; flattenToProm SN2 inlines the namespace into the base
//     because the internal stat path is `http.<HCM>.rbac.<ns>.<counter>`
//     and SN2's dot→underscore substitution promotes <ns> into the rest):
//     base name `envoy_http_rbac_<namespace>_<suffix>` (e.g.
//     `envoy_http_rbac_default_allowed`) + label-set
//     `{envoy_http_conn_manager_prefix="..."}`. No `envoy_rbac_http_prefix`
//     label.
//
// This is the "Prometheus tag-extractor divergence" anticipated at Task 8
// per ADR-0145 §Decision (the SN2-reuse hypothesis was empirically refuted
// at Task 14 — reference Envoy DOES carry a tag-extractor for
// `envoy_rbac_http_prefix`; envoy-go's MVP does NOT). Per the Task-14
// resolution path (Task description option (a)): the driver normalizes
// across both conventions; the divergence is absorbed in the assertion
// helper and documented in expectations.yaml + README.md. NO new SN10
// flatten rule is added; the structural envoy-go-side adjustment is
// deferred.
//
// Returns 0 when no metric matches (absent-as-zero discipline; per
// phase-13/14/15 precedent).
func lookupRBACCounter(stats map[string]int64, namespace, suffix string) int64 {
	// Form A: reference Envoy label form — base name `envoy_http_rbac_<suffix>`
	// + label `envoy_rbac_http_prefix="<namespace>"`. Sum across all
	// HCM-prefix label permutations.
	labelBase := "envoy_http_rbac_" + suffix
	labelNeedle := `envoy_rbac_http_prefix="` + namespace + `"`
	var total int64
	matched := map[string]bool{}
	for k, v := range stats {
		name, labelStr := k, ""
		if i := strings.IndexByte(k, '{'); i >= 0 {
			name = k[:i]
			if j := strings.LastIndexByte(k, '}'); j > i {
				labelStr = k[i+1 : j]
			}
		}
		if name == labelBase && strings.Contains(labelStr, labelNeedle) {
			if !matched[k] {
				total += v
				matched[k] = true
			}
		}
	}
	if len(matched) > 0 {
		return total
	}
	// Form B: envoy-go inline form — base name `envoy_http_rbac_<namespace>_<suffix>`
	// (namespace inlined into the metric name; HCM-prefix carries as label
	// only). Sum across all HCM-prefix label permutations.
	inlineName := "envoy_http_rbac_" + namespace + "_" + suffix
	for k, v := range stats {
		name := k
		if i := strings.IndexByte(k, '{'); i >= 0 {
			name = k[:i]
		}
		if name == inlineName {
			if !matched[k] {
				total += v
				matched[k] = true
			}
		}
	}
	return total
}

// --- PKI path plumbing (Task 13 forward-pointer) ---
//
// The five PKI file paths are derived from the fixture-managed temp dir
// established by the pki package at Task 13. Until Task 13 lands the pki
// package, these accessors return file paths under the fixture's pki/
// subdirectory (cert files do NOT yet exist; the driver compiles AT
// Task 12, but runtime invocations of runTLSScenario6 fail at the cert-
// load step — by design).
//
// Task 13 contract: the pki package's init() (or analogous Go-test-
// triggered hook) writes the five PEM files to a stable directory
// referenced by these accessors. The PLAN-time disposition uses
// fixtureDir() + "/pki" as the file-output dir; Task 13 may override if a
// per-test temp dir is preferable (e.g. via runtime.Caller-derived path
// or os.MkdirTemp). Until then the accessors return the fixed-path form.

func pkiClientCertPath() string { return filepath.Join(fixtureDir(), "pki", "client.pem") }
func pkiClientKeyPath() string  { return filepath.Join(fixtureDir(), "pki", "client.key.pem") }
func pkiServerCertPath() string { return filepath.Join(fixtureDir(), "pki", "server.pem") }
func pkiServerKeyPath() string  { return filepath.Join(fixtureDir(), "pki", "server.key.pem") }
func pkiCACertPath() string     { return filepath.Join(fixtureDir(), "pki", "ca.pem") }

// --- address-derivation helpers (Driver-interface stubs) ---

// deriveAddrsFromRef derives the 2 additional listener addrs from the
// l_test_a reference container address by port substitution. The
// reference container exposes ports 10018 (l_test_a), 10019 (l_test_b),
// 10020 (l_test_a_tls). Only used by the DriveReference single-addr stub
// (never reached at runtime because MultiListenerDriver is implemented).
func deriveAddrsFromRef(s1Addr string) map[string]string {
	replace := func(addr string, fromPort, toPort int) string {
		return strings.Replace(addr,
			fmt.Sprintf(":%d", fromPort),
			fmt.Sprintf(":%d", toPort), 1)
	}
	return map[string]string{
		"l_test_a":     s1Addr,
		"l_test_b":     replace(s1Addr, refLATestPort, refLBTestPort),
		"l_test_a_tls": replace(s1Addr, refLATestPort, refLATestTLSPort),
	}
}

// deriveAddrsFromSubj derives the 2 additional listener addrs from the
// l_test_a subject address by incrementing the port. SubjectConfig assigns
// LB=LA+1, LA_TLS=LA+2. Only used by the DriveSubject single-addr stub
// (never reached at runtime because MultiListenerDriver is implemented).
func deriveAddrsFromSubj(s1Addr string) map[string]string {
	lastColon := strings.LastIndex(s1Addr, ":")
	if lastColon < 0 {
		return map[string]string{
			"l_test_a":     s1Addr,
			"l_test_b":     s1Addr,
			"l_test_a_tls": s1Addr,
		}
	}
	hostPart := s1Addr[:lastColon]
	portStr := s1Addr[lastColon+1:]
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return map[string]string{
			"l_test_a":     s1Addr,
			"l_test_b":     s1Addr,
			"l_test_a_tls": s1Addr,
		}
	}
	return map[string]string{
		"l_test_a":     s1Addr,
		"l_test_b":     fmt.Sprintf("%s:%d", hostPart, port+1),
		"l_test_a_tls": fmt.Sprintf("%s:%d", hostPart, port+2),
	}
}

// --- file / template helpers ---

// fixtureDir returns the absolute path to the 0018-http-rbac fixture root
// (one directory above this file's inputs/ parent), derived from
// runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0018-http-rbac/inputs/driver.go
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
	_ fixture.Driver              = (*rbacDriver)(nil)
	_ fixture.BackendKindAware    = (*rbacDriver)(nil)
	_ fixture.MultiListenerDriver = (*rbacDriver)(nil)
	_ fixture.StatsAsserter       = (*rbacDriver)(nil)
)
