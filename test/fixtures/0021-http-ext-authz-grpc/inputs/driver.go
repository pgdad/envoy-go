// Package inputs registers the 0021-http-ext-authz-grpc fixture with the
// differential runner. Asserts per-scenario equivalence between envoy-go's
// envoy.filters.http.ext_authz (gRPC-service mode) and reference Envoy v1.37.2
// across the eight-scenario matrix per phase 18.2 SPEC §7.1.
//
// Integration shape (three-listener fixture; plaintext HTTP/1.1 downstream +
// plaintext h2c auth cluster per SPEC §7.2 + §11.P13 — no TLS in phase 18.2):
//
//  1. ReferenceBootstrap renders test/fixtures/0021-http-ext-authz-grpc/envoy.yaml
//     with host.docker.internal (ADR-0010 STRICT_DNS) + runner-allocated backend
//     port + the in-process gRPC auth server's host:port (host=host.docker.internal
//     for reference Envoy; auth port is allocated at driver instantiation so
//     both ReferenceBootstrap and SubjectConfig can templatize it
//     deterministically). SubjectConfig renders envoy-go.yaml with the
//     runner-allocated admin/listener ports + backend port (loopback) + auth
//     server host:port (host=127.0.0.1 for envoy-go which runs on the host).
//
//  2. DriveReferenceMulti / DriveSubjectMulti issue the identical 8-scenario
//     sequence against each proxy. The 8-scenario assertion-log byte stream is
//     emitted of the form:
//
//     scenario <id> status=<code> body=<ok|mismatch(...)>
//
//     The runner's CompareBytes pass enforces equivalence — when both proxies
//     produce equal verdicts, the differential gate fires.
//
//  3. AssertStats scrapes /stats/prometheus from both admin endpoints AFTER the
//     8-scenario workload. It asserts per-side ext_authz counter deltas against
//     the expected per-SPEC §7.1 matrix: ok, denied, error, failure_mode_allowed,
//     invalid. The disabled counter is STRUCTURALLY UNREACHABLE under MVP (parent
//     §6 amendment 7) and NOT asserted. Scenario 6 (per-route disabled) MUST NOT
//     increment any ext_authz counter. Scenarios 3 + 4 (auth server stopped
//     before request → gRPC dial fails) each increment error; scenario 4 also
//     increments failure_mode_allowed.
//
//  4. ProbeAdmin issues GET /ready against each proxy's admin endpoint and
//     returns the raw response bytes for the standard admin-diff at runner step 9.
//
// # Listener topology (three listeners — per SPEC §7.2 + planner-time decision D10)
//
// `failure_mode_allow` is a TOP-LEVEL `ExtAuthz` field; the proto exposes it
// ONLY at listener-config scope (`CheckSettings` cannot override it). The
// envoy-go MVP also does NOT honor `service_override`. Therefore scenario 3
// (`failure_mode_allow:false`) and scenario 4 (`failure_mode_allow:true`) MUST
// target DIFFERENT listener-level configs — mirroring the 18.1 0020 topology.
//
// Topology:
//
//   - l_test_a (ref port 10021, subj port = subjListenerPort+0) — scenarios
//     1+2+5+6+7+8. `failure_mode_allow:false` (default). Listener-level
//     `grpc_service.envoy_grpc.cluster_name: c_authz_grpc` points at the LIVE
//     auth server. Scenarios 1+2+5+7+8 hit the auth server; scenario 6 carries
//     the per-route `disabled:true` override so the filter is bypassed.
//   - l_test_b (ref port 10022, subj port = subjListenerPort+1) — scenario 3.
//     `failure_mode_allow:false`, `status_on_error:503`. Same `c_authz_grpc`
//     cluster reference. The driver STOPS the auth server before issuing the
//     scenario-3 request (and restarts it after); the gRPC dial fails →
//     dispError → 503 LocalReply.
//   - l_test_c (ref port 10023, subj port = subjListenerPort+2) — scenario 4.
//     `failure_mode_allow:true`, `failure_mode_allow_header_add:true`. Same
//     `c_authz_grpc` cluster reference. The driver STOPS the auth server
//     before issuing the scenario-4 request (and restarts it after); the gRPC
//     dial fails → dispError → request PROCEEDS to backend with the
//     `x-envoy-auth-failure-mode-allowed: true` header injected upstream.
//
// # Auth-server lifecycle (extauthzgrpc helper)
//
// The in-process gRPC auth server (test/helpers/extauthzgrpc) is started fresh
// at the beginning of each driveProxy run on a single STABLE port (allocated
// at driver instantiation), pre-populated with the 8 scripted `*CheckResponse`
// values keyed by `:path` discriminator per SPEC §7.4. The driver stops the
// server before scenarios 3+4 (to force the gRPC dial failure that exercises
// the failure-mode-allow + status-on-error paths) and restarts it before each
// subsequent scenario that needs it.
//
//   - Scenario 1 (l_test_a /scenario1): OK + empty OkHttpResponse{} → 200 echo backend.
//   - Scenario 2 (l_test_a /scenario2): DeniedResponse 403 + body + headers → verbatim 403.
//   - Scenario 3 (l_test_b /scenario3): auth server STOPPED → dispError → 503.
//   - Scenario 4 (l_test_c /scenario4): auth server STOPPED → dispError → 200 echo + failure_mode_allowed header upstream.
//   - Scenario 5 (l_test_a /scenario5): OK + empty OkHttpResponse{} with_request_body — auth sees the body.
//   - Scenario 6 (l_test_a /disabled): per-route disabled → no auth call.
//   - Scenario 7 (l_test_a /ctx): per-route context_extensions{policy:scenario7} → auth sees the extensions; OK + empty.
//   - Scenario 8 (l_test_a /scenario8): OK + OkHttpResponse with header mutations across 4 append_action arms + headers_to_remove → upstream echoes injected headers.
//
// Task 11 wires:
//   - cluster `c_authz_grpc` → 127.0.0.1:{{.AuthPort}} (live in-process gRPC auth server,
//     with `http2_protocol_options: {}` set per SPEC §11.P13 for gRPC framing).
//   - cluster `c_backend` → 127.0.0.1:{{.BackendPort}} (live echo backend).
//   - three listeners l_test_a, l_test_b, l_test_c with the three ext_authz
//     gRPC-mode configs described above.
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

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	authv3 "github.com/envoyproxy/go-control-plane/envoy/service/auth/v3"
	typev3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	rpcstatus "google.golang.org/genproto/googleapis/rpc/status"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
	"github.com/pgdad/envoy-go/test/helpers/extauthzgrpc"
)

const (
	fixtureName = "0021-http-ext-authz-grpc"

	// In-container reference Envoy listener ports. Convention "100NN" for
	// fixture "00NN" — fixture 0021 takes 10021/10022/10023 for the three
	// plaintext listeners (one per failure_mode_allow / auth-down variant
	// per the SPEC §7.2 + D10 topology).
	refAdminPort  = 9901
	refLATestPort = 10021 // l_test_a (S1+2+5+6+7+8; failure_mode_allow:false; c_authz_grpc live)
	refLBTestPort = 10022 // l_test_b (S3;           failure_mode_allow:false; auth-down before request)
	refLCTestPort = 10023 // l_test_c (S4;           failure_mode_allow:true;  auth-down before request)

	// Deny-path body — the ext_authz gRPC auth server's verbatim deny body for
	// scenario 2. Sent as `DeniedHttpResponse.body` (gRPC) and applied verbatim
	// by the filter per SPEC §5.P11 RATIFIED.
	bodyDenyScenario2 = "access denied"

	// Failure-mode-allowed header — scenario 4 asserts this header arrives
	// upstream (echoed back by the echobackend) when the gRPC dial fails and
	// failure_mode_allow:true + failure_mode_allow_header_add:true.
	headerFailureModeAllowed = "x-envoy-auth-failure-mode-allowed"

	// Scenario 8 OkHttpResponse mutation headers — the driver asserts these
	// arrive upstream (echoed back by the echobackend) per the 4-arm
	// append_action dispatch table (D5) + headers_to_remove.
	headerInjectedOverwrite     = "x-injected-by-authz"        // OVERWRITE_IF_EXISTS_OR_ADD
	headerInjectedAppend        = "x-also-appended"            // APPEND_IF_EXISTS_OR_ADD
	headerInjectedOverwriteOnly = "x-overwrite-only-if-exists" // OVERWRITE_IF_EXISTS (no-op when key absent upstream)
	headerInjectedAddIfAbsent   = "x-add-if-absent"            // ADD_IF_ABSENT
	// headers_to_remove — upstream should NOT see it. NOTE: cannot use "user-agent"
	// here because Go's net/http.Request.Write defaults User-Agent to
	// "Go-http-client/1.1" when the header is absent on the request, which makes
	// the upstream observe a User-Agent regardless of envoy-go's filter-level
	// headers.Del(). This is a Go net/http quirk, not an ext_authz divergence —
	// the filter DOES delete the header from the decoded request map; the Go
	// upstream-write layer re-injects a default. Pick an arbitrary client-supplied
	// header that has no special net/http write-time injection.
	headerToRemove = "x-fixture-supplied-removable"

	// Scenario 7 per-route context_extensions key/value — the auth server is
	// scripted to echo the policy back as an upstream header so the driver can
	// assert the extension reached it. (Direct CheckRequest assertion is
	// out of scope for this driver — the byte-stream diff is reference vs
	// subject; we rely on reference Envoy v1.37.2 having faithfully populated
	// `AttributeContext.context_extensions["policy"] = "scenario7"`.)
	contextExtensionPolicyKey   = "policy"
	contextExtensionPolicyValue = "scenario7"
	headerScenario7Policy       = "x-authz-policy" // echoed upstream by scenario-7 script
)

func init() {
	fixture.RegisterFixture(fixtureName, &extAuthzGRPCDriver{})
}

// extAuthzGRPCDriver carries the per-driver lifecycle state — the auth server
// port (constant across the ref+subj run; allocated lazily at templatize time)
// and the running auth server handle. The auth server is spawned at the start
// of each driveProxy call (with all 8 scripts pre-populated), stopped before
// scenarios 3+4, and restarted before subsequent scenarios that need it.
type extAuthzGRPCDriver struct {
	mu sync.Mutex

	// authPort is allocated lazily on first use (ReferenceBootstrap or
	// SubjectConfig — whichever fires first). The same port is used for both
	// ref and subj runs so envoy.yaml and envoy-go.yaml can be templatized
	// deterministically before the per-side Drive runs. The cluster
	// c_authz_grpc points at this port (with http2_protocol_options: {}).
	authPort int

	// authSrv is the currently-running auth server. nil before Drive or
	// between Stop and the next Start. The driver starts it inside driveProxy
	// via the shared test/helpers/extauthzgrpc helper (caller-chosen-port arm
	// extauthzgrpc.NewAtAddr — pre-allocated port baked into bootstrap YAMLs
	// per the 18.1 fixture-0020 idiom), pre-populates the 8 scripts, and
	// toggles it across scenarios 3+4.
	authSrv *extauthzgrpc.Server
}

// allocateAuthPort allocates a free TCP port for the auth server. Called lazily
// by ReferenceBootstrap / SubjectConfig (whichever fires first). Idempotent —
// returns the same port on subsequent calls. Does NOT start the server; the
// server is started fresh at the beginning of each driveProxy call.
func (d *extAuthzGRPCDriver) allocateAuthPort() int {
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

// setupAuthGRPC starts the in-process gRPC auth server bound to the
// pre-allocated authPort, pre-populates the 8 scripted CheckResponse values
// keyed by `:path` discriminator (per SPEC §7.4), and stores the handle on
// the driver. Mirrors the 18.1 setupAuthHTTP shape. Idempotent: an
// already-running server is stopped first.
//
// The 8 scripts (keyed by req.Attributes.Request.Http.Path):
//
//   - /scenario1 → OK + empty OkHttpResponse{} (bare allow; backend echo).
//   - /scenario2 → DeniedResponse{status:403, body:"access denied", headers:[x-authz-denied-reason:scenario2]}.
//   - /scenario5 → OK + empty OkHttpResponse{} (with_request_body — auth still allows).
//   - /disabled  → NOT registered (scenario 6 bypasses the filter; no auth call fires).
//   - /ctx       → OK + OkHttpResponse with `x-authz-policy: scenario7` (the auth server
//     received `AttributeContext.context_extensions["policy"] = "scenario7"`;
//     we cannot directly assert that from this driver — we rely on the byte-stream
//     equivalence between reference and subject + the echo of the upstream header
//     as a coarse confirmation that the script fired).
//   - /scenario8 → OK + OkHttpResponse with 4 header mutations (one per
//     append_action arm) plus headers_to_remove:[user-agent].
//
// Scenarios 3 + 4 do NOT register any script — those scenarios STOP the auth
// server before issuing the request so the gRPC dial fails → dispError. No
// CheckRequest reaches the server.
func (d *extAuthzGRPCDriver) setupAuthGRPC() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.authSrv != nil {
		d.authSrv.Stop()
		d.authSrv = nil
	}
	if d.authPort == 0 {
		// Defensive — allocateAuthPort should have fired by now.
		return fmt.Errorf("driver: setupAuthGRPC called before authPort allocation")
	}

	// Mirrors 18.1's allocate-then-rebind pattern: allocateAuthPort reserves
	// a free port via Listen+Close; setupAuthGRPC re-binds the gRPC server to
	// that exact port via extauthzgrpc.NewAtAddr — the caller-chosen-port arm
	// of the shared SPEC §7.4 helper. If the port has been recycled (TIME_WAIT
	// / parallel-fixture race — negligible for the single-fixture runner), the
	// bind will fail and the run errors out loudly.
	//
	// Bind all interfaces so the reference Envoy container can reach the
	// service via host.docker.internal (bridge gateway) on plain Linux Docker;
	// loopback-only binds are unreachable from containers outside Docker Desktop.
	addr := fmt.Sprintf("0.0.0.0:%d", d.authPort)
	srv, err := extauthzgrpc.NewAtAddr(addr)
	if err != nil {
		return fmt.Errorf("driver: start gRPC auth server on %s: %w", addr, err)
	}
	d.authSrv = srv

	// Pre-populate the 8 scripts per SPEC §7.1.
	// Scenario 1 — bare allow.
	srv.Script("/scenario1", &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 0 /* OK */},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{},
		},
	})

	// Scenario 2 — DeniedResponse 403 + body + one custom header.
	srv.Script("/scenario2", &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 7 /* PERMISSION_DENIED */},
		HttpResponse: &authv3.CheckResponse_DeniedResponse{
			DeniedResponse: &authv3.DeniedHttpResponse{
				Status: &typev3.HttpStatus{Code: typev3.StatusCode_Forbidden},
				Body:   bodyDenyScenario2,
				Headers: []*corev3.HeaderValueOption{
					{
						Header: &corev3.HeaderValue{
							Key:   "x-authz-denied-reason",
							Value: "scenario2",
						},
					},
				},
			},
		},
	})

	// Scenario 5 — with_request_body allow. Auth server sees the body in
	// AttributeContext.request.http.body (populated by the filter via
	// ADR-0128 buffering); we don't inspect it from the driver — the
	// reference vs subject byte-stream diff is the assertion.
	srv.Script("/scenario5", &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 0},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{},
		},
	})

	// Scenario 7 — per-route context_extensions{policy:scenario7}. Auth server
	// sees the extension in AttributeContext.context_extensions; we cannot
	// directly assert that from this driver, but we echo the policy back as an
	// upstream header so the echobackend reflects it in the response body. The
	// reference vs subject byte-stream diff plus the upstream header echo
	// gives us coarse confirmation that the script fired.
	srv.Script("/ctx", &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 0},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: []*corev3.HeaderValueOption{
					{
						Header: &corev3.HeaderValue{
							Key:   headerScenario7Policy,
							Value: contextExtensionPolicyValue,
						},
						AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
					},
				},
			},
		},
	})

	// Scenario 8 — OkHttpResponse mutation across 4 append_action arms +
	// headers_to_remove. The echobackend reflects upstream-arrived headers
	// back in the response body; the driver asserts the injected keys are
	// present (overwrite + append + add-if-absent) and that the to-remove key
	// is absent.
	srv.Script("/scenario8", &authv3.CheckResponse{
		Status: &rpcstatus.Status{Code: 0},
		HttpResponse: &authv3.CheckResponse_OkResponse{
			OkResponse: &authv3.OkHttpResponse{
				Headers: []*corev3.HeaderValueOption{
					{
						Header:       &corev3.HeaderValue{Key: headerInjectedOverwrite, Value: "scenario8"},
						AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS_OR_ADD,
					},
					{
						Header:       &corev3.HeaderValue{Key: headerInjectedAppend, Value: "append1"},
						AppendAction: corev3.HeaderValueOption_APPEND_IF_EXISTS_OR_ADD,
					},
					{
						Header:       &corev3.HeaderValue{Key: headerInjectedOverwriteOnly, Value: "v3"},
						AppendAction: corev3.HeaderValueOption_OVERWRITE_IF_EXISTS,
					},
					{
						Header:       &corev3.HeaderValue{Key: headerInjectedAddIfAbsent, Value: "v4"},
						AppendAction: corev3.HeaderValueOption_ADD_IF_ABSENT,
					},
				},
				HeadersToRemove: []string{headerToRemove},
			},
		},
	})

	return nil
}

// stopAuthGRPC stops the running auth server. Idempotent. Called between
// scenarios 2→3 (to force dispError on scenario 3) and 3→4 (auth stays down
// for scenario 4 too) and at teardown.
func (d *extAuthzGRPCDriver) stopAuthGRPC() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.authSrv != nil {
		d.authSrv.Stop()
		d.authSrv = nil
	}
}

// --- fixture.Driver (required) ---

func (*extAuthzGRPCDriver) BackendCount() int                { return 1 }
func (*extAuthzGRPCDriver) BackendKind() fixture.BackendKind { return fixture.HTTPExtAuthzGRPC }

// SubjectListenerName returns the primary listener name (l_test_a). The
// runner uses this for the single-addr DriveSubject fallback; because this
// fixture implements MultiListenerDriver the runner dispatches
// DriveSubjectMulti instead. Method is REQUIRED by the Driver interface.
func (*extAuthzGRPCDriver) SubjectListenerName() string { return "l_test_a" }

// ReferenceListenerPort returns the primary reference listener port
// (l_test_a). Required by the Driver interface even though
// MultiListenerDriver takes precedence at runtime.
func (*extAuthzGRPCDriver) ReferenceListenerPort() int { return refLATestPort }

// ReferenceBootstrap renders envoy.yaml with host.docker.internal +
// runner-allocated backend port + auth server host:port (host=host.docker.internal
// because the reference Envoy container reaches host-side services via
// host.docker.internal per ADR-0010). Three listener ports are wired
// (l_test_a/b/c). The auth port is allocated here if not already; Task 11
// authors envoy.yaml.
func (d *extAuthzGRPCDriver) ReferenceBootstrap(backendPorts []int) string {
	authPort := d.allocateAuthPort()
	tpl := mustReadFixtureFile("envoy.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   refAdminPort,
		"LATestPort":  refLATestPort,
		"LBTestPort":  refLBTestPort,
		"LCTestPort":  refLCTestPort,
		"BackendHost": "host.docker.internal",
		"BackendPort": backendPorts[0],
		"AuthHost":    "host.docker.internal",
		"AuthPort":    authPort,
	})
}

// SubjectConfig renders envoy-go.yaml with runner-allocated admin/listener ports
// + backend port (loopback) + auth server host:port (host=127.0.0.1 because
// envoy-go runs on the host directly — no docker translation). The three
// subject listeners take consecutive ports starting at subjListenerPort:
// LA=subjListenerPort, LB=subjListenerPort+1, LC=subjListenerPort+2 — mirrors
// the phase-18.1 fixture-0020 port-offset pattern.
func (d *extAuthzGRPCDriver) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	authPort := d.allocateAuthPort()
	tpl := mustReadFixtureFile("envoy-go.yaml")
	return mustRender(tpl, map[string]any{
		"AdminPort":   subjAdminPort,
		"LATestPort":  subjListenerPort,
		"LBTestPort":  subjListenerPort + 1,
		"LCTestPort":  subjListenerPort + 2,
		"BackendPort": backendPorts[0],
		"AuthHost":    "127.0.0.1",
		"AuthPort":    authPort,
	})
}

// DriveReference is the single-addr Driver-interface path; never invoked at
// runtime because MultiListenerDriver is implemented. Delegates to
// DriveReferenceMulti deriving the additional addrs by reference-port
// substitution.
func (d *extAuthzGRPCDriver) DriveReference(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromRef(addr)
	return d.DriveReferenceMulti(ctx, addrs)
}

// DriveSubject is the single-addr Driver-interface path; never invoked at
// runtime because MultiListenerDriver is implemented. Delegates to
// DriveSubjectMulti deriving the additional addrs by subject-port offset.
func (d *extAuthzGRPCDriver) DriveSubject(ctx context.Context, addr string) ([]byte, error) {
	addrs := deriveAddrsFromSubj(addr)
	return d.DriveSubjectMulti(ctx, addrs)
}

// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
// returns the raw response bytes for the standard admin-diff at runner step 9.
func (*extAuthzGRPCDriver) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
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
func (*extAuthzGRPCDriver) SubjectListenerNames() []string {
	return []string{"l_test_a", "l_test_b", "l_test_c"}
}

// ReferenceListenerPorts returns the three in-container reference listener
// ports in order matching SubjectListenerNames().
func (*extAuthzGRPCDriver) ReferenceListenerPorts() []int {
	return []int{refLATestPort, refLBTestPort, refLCTestPort}
}

// DriveReferenceMulti issues all 8 scenarios against the reference proxy.
// addrs maps listener name → "host:port" (provided by the runner).
func (d *extAuthzGRPCDriver) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveProxy(ctx, addrs, "ref")
}

// DriveSubjectMulti issues all 8 scenarios against the subject proxy.
// addrs maps listener name → "host:port" (provided by the runner).
func (d *extAuthzGRPCDriver) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
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

// driveProxy issues the 8-scenario sequence sequentially against the listener
// addresses provided in addrs. The "side" label is INTENTIONALLY excluded from
// the byte stream so both sides produce identical bytes when behavior is
// equivalent.
//
// Auth-server lifecycle (single live server on authPort with all 8 scripts
// pre-populated; toggled across scenarios 3+4 to force dispError):
//  1. Start auth server + register all 8 scripts (scenarios 1, 2, 5, 7, 8 fire).
//  2. Scenarios 1+2+5: run against l_test_a (live auth).
//  3. STOP auth server before scenarios 3+4.
//  4. Scenario 3: l_test_b (failure_mode_allow:false) → dispError → 503.
//  5. Scenario 4: l_test_c (failure_mode_allow:true) → dispError → 200 echo + header.
//  6. RESTART auth server before scenarios 6+7+8 (with same 8 scripts).
//  7. Scenarios 6+7+8: run against l_test_a (live auth; S6 bypasses filter).
//  8. Stop auth server at teardown.
func (d *extAuthzGRPCDriver) driveProxy(ctx context.Context, addrs map[string]string, side string) ([]byte, error) {
	tr := &http.Transport{DisableKeepAlives: true}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}

	// Per-listener base URLs.
	baseA := "http://" + addrs["l_test_a"] // S1+2+5+6+7+8 (live auth)
	baseB := "http://" + addrs["l_test_b"] // S3 (auth stopped; failure_mode_allow:false)
	baseC := "http://" + addrs["l_test_c"] // S4 (auth stopped; failure_mode_allow:true)

	var b bytes.Buffer

	// Start auth server with all 8 scripts pre-populated.
	if err := d.setupAuthGRPC(); err != nil {
		return nil, fmt.Errorf("[%s] setup gRPC auth server: %w", side, err)
	}

	// Scenario 1: bare allow on l_test_a.
	res1 := runScenario1(ctx, client, baseA, side)
	emitScenario(&b, 1, res1)

	// Scenario 2: deny on l_test_a — verbatim body + 403.
	res2 := runScenario2(ctx, client, baseA, side)
	emitScenario(&b, 2, res2)

	// Scenario 5: with_request_body allow on l_test_a (run before S3 to keep
	// the auth-live group contiguous).
	res5 := runScenario5(ctx, client, baseA, side)
	emitScenario(&b, 5, res5)

	// STOP auth server before scenarios 3+4 to force gRPC dial failures.
	d.stopAuthGRPC()

	// Scenario 3: error → status_on_error on l_test_b.
	// l_test_b carries failure_mode_allow:false + status_on_error:503.
	// The auth server is stopped → gRPC dial fails → dispError → 503.
	res3 := runScenario3(ctx, client, baseB, side)
	emitScenario(&b, 3, res3)

	// Scenario 4: failure_mode_allow on l_test_c.
	// l_test_c carries failure_mode_allow:true + failure_mode_allow_header_add:true.
	// The auth server is stopped → gRPC dial fails → dispError → request
	// PROCEEDS to backend with x-envoy-auth-failure-mode-allowed: true injected
	// upstream.
	res4 := runScenario4(ctx, client, baseC, side)
	emitScenario(&b, 4, res4)

	// RESTART auth server for scenarios 6+7+8 (S6 bypasses the filter so the
	// server state is irrelevant, but S7 + S8 need it back).
	if err := d.setupAuthGRPC(); err != nil {
		return nil, fmt.Errorf("[%s] restart gRPC auth server before scenarios 6-8: %w", side, err)
	}

	// Scenario 6: per-route disabled on l_test_a — no auth call.
	res6 := runScenario6(ctx, client, baseA, side)
	emitScenario(&b, 6, res6)

	// Scenario 7: per-route context_extensions on l_test_a.
	res7 := runScenario7(ctx, client, baseA, side)
	emitScenario(&b, 7, res7)

	// Scenario 8: OkHttpResponse mutation on l_test_a.
	res8 := runScenario8(ctx, client, baseA, side)
	emitScenario(&b, 8, res8)

	// Teardown.
	d.stopAuthGRPC()

	return b.Bytes(), nil
}

// emitScenario formats the per-scenario verdict line into the byte stream.
// The format mirrors the phase-18.1 0020 precedent:
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
//   - Error scenario 3: empty body assertion (status_on_error path).
//   - Allow scenarios 1, 4, 5, 6, 7, 8: echo-backend path — assert structural
//     properties (non-empty JSON with method + path keys). Scenarios 4 and 8
//     additionally assert specific upstream-arrival header keys.
func classifyBody(scenarioID int, body []byte) string {
	switch scenarioID {
	case 1, 5, 6:
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
	case 4:
		// failure_mode_allow:true path — echo backend body PLUS the
		// x-envoy-auth-failure-mode-allowed header reflected upstream.
		if !isEchoBody(body) {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", string(body))
		}
		hdrs := echoHeaders(body)
		if _, ok := hdrs[headerFailureModeAllowed]; !ok {
			return fmt.Sprintf("mismatch(missing_failure_mode_header,hdrs=%v)", hdrs)
		}
		return "ok"
	case 7:
		// context_extensions allow — echo backend body PLUS the upstream-echo
		// of the per-scenario auth-server-injected x-authz-policy header
		// (coarse confirmation that the script fired; the AttributeContext
		// content assertion is byte-stream-equivalence-based, not direct).
		if !isEchoBody(body) {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", string(body))
		}
		hdrs := echoHeaders(body)
		if v, ok := hdrs[headerScenario7Policy]; !ok || v != contextExtensionPolicyValue {
			return fmt.Sprintf("mismatch(scenario7_policy_header,got=%q,want=%q)", v, contextExtensionPolicyValue)
		}
		return "ok"
	case 8:
		// OkHttpResponse mutation allow — echo backend body PLUS the 4
		// injected upstream headers (overwrite + append + add-if-absent;
		// OVERWRITE_IF_EXISTS is a no-op when the key is absent upstream, so
		// we do NOT assert headerInjectedOverwriteOnly arrives). The
		// to-remove header (user-agent) MUST NOT appear upstream.
		if !isEchoBody(body) {
			return fmt.Sprintf("mismatch(not_echo_json,got=%q)", string(body))
		}
		hdrs := echoHeaders(body)
		if _, ok := hdrs[headerInjectedOverwrite]; !ok {
			return fmt.Sprintf("mismatch(missing_overwrite_header,hdrs=%v)", hdrs)
		}
		if _, ok := hdrs[headerInjectedAppend]; !ok {
			return fmt.Sprintf("mismatch(missing_append_header,hdrs=%v)", hdrs)
		}
		if _, ok := hdrs[headerInjectedAddIfAbsent]; !ok {
			return fmt.Sprintf("mismatch(missing_add_if_absent_header,hdrs=%v)", hdrs)
		}
		if _, ok := hdrs[headerToRemove]; ok {
			return fmt.Sprintf("mismatch(unexpected_to_remove_header_present,hdrs=%v)", hdrs)
		}
		return "ok"
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
// Used for backend-arrival header assertions (scenarios 4, 7, 8).
func echoHeaders(body []byte) map[string]string {
	var rec struct {
		Headers map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil
	}
	return rec.Headers
}

// runScenario1 — gRPC allow on l_test_a, path /scenario1. The auth server
// returns OK + empty OkHttpResponse{}; the filter allows the request through
// to the echo backend. Expected: 200 echo, no special header assertion.
// Counter delta: ok=+1.
func runScenario1(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/scenario1", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("x-client-id", "scenario-1")
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0021_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 1: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario2 — gRPC deny on l_test_a, path /scenario2. The auth server
// returns DeniedResponse{status:403, body:"access denied", headers:[x-authz-denied-reason:scenario2]};
// the filter sends a LocalReply 403 with the body + headers verbatim (per
// SPEC §5.P11 — gRPC-mode deny headers are NOT filtered through
// allowed_client_headers; they're applied verbatim). Counter delta: denied=+1.
func runScenario2(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/scenario2",
		strings.NewReader("request-body"))
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Content-Type", "text/plain")
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0021_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 2: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario3 — error → status_on_error on l_test_b, path /scenario3.
// l_test_b carries failure_mode_allow:false + status_on_error:503; the auth
// server has been STOPPED by the driver before this request. The gRPC dial
// fails → dispError → 503 LocalReply with empty body. Counter delta: error=+1.
func runScenario3(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/scenario3", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0021_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 3: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario4 — failure_mode_allow on l_test_c, path /scenario4. l_test_c
// carries failure_mode_allow:true + failure_mode_allow_header_add:true; the
// auth server has been STOPPED. The gRPC dial fails → dispError → request
// PROCEEDS to backend with x-envoy-auth-failure-mode-allowed: true injected
// upstream. Expected: 200 echo with the marker header arriving upstream.
// Counter delta: error=+1, failure_mode_allowed=+1.
func runScenario4(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/scenario4", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0021_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 4: status=%d body=%q\n", side, res.statusCode, string(res.body))
		if hdrs := echoHeaders(res.body); hdrs != nil {
			if v, ok := hdrs[headerFailureModeAllowed]; ok {
				fmt.Fprintf(os.Stderr, "[%s] scenario 4: %s=%q\n", side, headerFailureModeAllowed, v)
			}
		}
	}
	return res
}

// runScenario5 — with_request_body allow on l_test_a, path /scenario5. POSTs
// a body that the filter buffers (per ADR-0128) and attaches to
// AttributeContext.request.http.body. The auth server is scripted to allow.
// Expected: 200 echo backend. Counter delta: ok=+1.
func runScenario5(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/scenario5",
		strings.NewReader("hello world"))
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	req.Header.Set("Content-Type", "text/plain")
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0021_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 5: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario6 — per-route disabled on l_test_a, path /disabled. The
// per-route ExtAuthzPerRoute{disabled: true} bypasses the filter entirely.
// Expected: 200 echo backend. NO ext_authz counter increments (per SPEC §7.1 +
// parent §6 amendment 7).
func runScenario6(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/disabled", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0021_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 6: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario7 — per-route check_settings.context_extensions on l_test_a,
// path /ctx. The per-route ExtAuthzPerRoute{check_settings{context_extensions:
// {policy:"scenario7"}}} populates AttributeContext.context_extensions; the
// auth server's /ctx script echoes the policy back as the
// x-authz-policy upstream header for coarse confirmation. Expected: 200 echo
// backend + the upstream-arrival x-authz-policy header.
// Counter delta: ok=+1 (SHARED stats).
func runScenario7(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/ctx", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0021_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 7: status=%d body=%q\n", side, res.statusCode, string(res.body))
	}
	return res
}

// runScenario8 — OkHttpResponse upstream mutation on l_test_a, path
// /scenario8. The auth server returns OK + OkHttpResponse with 4 header
// mutations (one per append_action arm) + headers_to_remove:[user-agent].
// The filter injects (overwrite + append + add-if-absent) and strips
// (user-agent) before forwarding upstream. The echobackend reflects the
// upstream-arrival headers in the response body. Expected: 200 echo backend
// + the 3 reachable injected keys present + user-agent absent.
// Counter delta: ok=+1.
func runScenario8(ctx context.Context, client *http.Client, baseURL, side string) scenarioResult {
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/scenario8", nil)
	if err != nil {
		return scenarioResult{err: fmt.Errorf("build request: %w", err)}
	}
	// Inject a synthetic client-supplied header so the headers_to_remove case
	// is non-trivial — the auth server scripts headers_to_remove:[<headerToRemove>]
	// and the driver asserts it is absent upstream. We deliberately avoid
	// "user-agent" because Go's net/http.Request.Write re-injects a default
	// "Go-http-client/1.1" when User-Agent is absent on r.Header, which would
	// mask the filter's headers.Del at upstream-write time (a Go net/http
	// quirk, not an ext_authz divergence).
	req.Header.Set(headerToRemove, "client-supplied-to-be-removed")
	res := doRequest(client, req)
	if os.Getenv("FIXTURE_0021_DUMP_BYTES") != "" {
		fmt.Fprintf(os.Stderr, "[%s] scenario 8: status=%d body=%q\n", side, res.statusCode, string(res.body))
		if hdrs := echoHeaders(res.body); hdrs != nil {
			fmt.Fprintf(os.Stderr, "[%s] scenario 8 upstream-headers: %v\n", side, hdrs)
		}
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
// ext_authz counter values per SPEC §7.1 matrix. The 8-scenario run produces:
//
//   - ok:                   4  (scenarios 1, 5, 7, 8 → allowed)
//   - denied:               1  (scenario 2 → denied)
//   - error:                2  (scenarios 3, 4 → auth dial fails)
//   - failure_mode_allowed: 1  (scenario 4 → failure_mode_allow:true)
//   - invalid:              0  (no validate_mutations rejections on happy path)
//   - disabled:             0  (STRUCTURALLY UNREACHABLE under MVP per parent §6 amendment 7)
//
// Scenario 6 (per-route disabled) contributes NO counter increments per
// SPEC §7.1 + parent §6 amendment 7.
//
// Note: scenarios 3+4 route through DIFFERENT listeners (l_test_b vs l_test_c)
// that carry different ext_authz filter configs (failure_mode_allow:false vs
// true). Scenarios 1+2+5+6+7+8 route through l_test_a. All three listeners
// share the SAME stat namespace (SHARED-stats per ADR-0163) because they emit
// to the same stat_prefix and the per-listener counters aggregate cluster-wide.
// Task 12 finalizes the exact counter values via empirical scrape; the values
// above are PLAN-time hypotheses.
func (d *extAuthzGRPCDriver) AssertStats(t fixture.TB, refAdminAddr, subjAdminAddr string) {
	t.Helper()

	refStats, err := scrapeExtAuthzStats(refAdminAddr)
	if err != nil {
		t.Fatalf("scrape ref ext_authz stats: %v", err)
	}
	subjStats, err := scrapeExtAuthzStats(subjAdminAddr)
	if err != nil {
		t.Fatalf("scrape subj ext_authz stats: %v", err)
	}

	if os.Getenv("FIXTURE_0021_DUMP_STATS") != "" {
		fmt.Fprintf(os.Stderr, "=== ref ext_authz stats ===\n")
		for k, v := range refStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
		fmt.Fprintf(os.Stderr, "=== subj ext_authz stats ===\n")
		for k, v := range subjStats {
			fmt.Fprintf(os.Stderr, "  %s = %d\n", k, v)
		}
	}

	// Per SPEC §7.1 — counter expectations finalized at Task 12 via empirical
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
		{suffix: "ok", want: 4, comment: "scenarios 1 (allow), 5 (with_request_body), 7 (context_extensions), 8 (OkHttpResponse mutation)"},
		{suffix: "denied", want: 1, comment: "scenario 2 (403 deny verbatim)"},
		{suffix: "error", want: 2, comment: "scenarios 3+4 (gRPC dial fails: auth server stopped)"},
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
// at query time. Reused verbatim from 18.1 fixture 0020.
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
// phase-16 rbac fixture-0018 precedent. Reused verbatim from 18.1 fixture 0020.
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
// suffix across all observed name+label permutations. Reused verbatim from
// 18.1 fixture 0020; accommodates the two observed Prometheus naming
// conventions (reference Envoy v1.37.2 + envoy-go SN2-reuse form).
//
// Returns 0 when no metric matches (absent-as-zero discipline per phase-13/
// 14/15/16/17/18.1 precedent).
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
// Mirrors the phase-18.1 fixture-0020 pattern.

// deriveAddrsFromRef derives the 2 additional listener addrs from the
// l_test_a reference container address by port substitution. The reference
// container exposes ports 10021 (l_test_a), 10022 (l_test_b), 10023
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

// fixtureDir returns the absolute path to the 0021-http-ext-authz-grpc fixture
// root (one directory above this file's inputs/ parent), derived from
// runtime.Caller — works regardless of the caller's cwd.
func fixtureDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		panic("driver: runtime.Caller failed — cannot locate fixture directory")
	}
	// thisFile is .../test/fixtures/0021-http-ext-authz-grpc/inputs/driver.go
	return filepath.Dir(filepath.Dir(thisFile))
}

// mustReadFixtureFile reads name from the fixture root directory. Used to
// load envoy.yaml + envoy-go.yaml templates at Task 12 + Task 13. At Task 10
// the YAMLs do NOT exist; ReferenceBootstrap + SubjectConfig will panic if
// invoked before Task 11 lands the templates — BY DESIGN at Task 10
// (fixture compiles + registers; runtime invocation gated on Task 11).
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
	_ fixture.Driver              = (*extAuthzGRPCDriver)(nil)
	_ fixture.BackendKindAware    = (*extAuthzGRPCDriver)(nil)
	_ fixture.MultiListenerDriver = (*extAuthzGRPCDriver)(nil)
	_ fixture.StatsAsserter       = (*extAuthzGRPCDriver)(nil)
)
