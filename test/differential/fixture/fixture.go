// Package fixture defines the Driver interface and the global registry
// that fixture driver packages populate via init(). It is a leaf package with
// no dependency on test/differential so that driver packages can import it
// without creating an import cycle.
package fixture

import (
	"context"
	"fmt"
)

// Driver is the contract a fixture under test/fixtures/NNNN-*/driver
// implements. Drivers register themselves in init(); the runner discovers
// them by name (which must match the fixture directory).
type Driver interface {
	// BackendCount is the number of host-side TCP echo backends the runner
	// allocates per fixture run. Each backend gets its own random port and
	// its own atomic.Uint64 accept counter that the runner snapshots after
	// DriveReference/DriveSubject complete.
	BackendCount() int

	// SubjectListenerName is the listener name the driver's DriveSubject
	// targets. The runner uses subj.ListenerAddr(SubjectListenerName()) to
	// look up the subject's bound address per the ADR-0026 sentinel format.
	SubjectListenerName() string

	// ReferenceBootstrap returns the YAML to feed upstream Envoy. The
	// runner passes the slice of allocated backend ports; the driver
	// templates them into its config however it wants.
	ReferenceBootstrap(backendPorts []int) string

	// SubjectConfig renders the subject's bootstrap. backendPorts is the
	// same slice the runner generated for ReferenceBootstrap.
	SubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string

	// ReferenceListenerPort is the in-container TCP port the reference
	// proxy must expose (the listener the driver dials).
	ReferenceListenerPort() int

	// DriveReference runs the fixture's driver logic against the reference
	// proxy's listener address. Returns all received bytes.
	DriveReference(ctx context.Context, addr string) ([]byte, error)

	// DriveSubject runs the fixture's driver logic against the subject
	// proxy's listener address. Returns all received bytes.
	DriveSubject(ctx context.Context, addr string) ([]byte, error)

	// ProbeAdmin issues GET /ready against each proxy's admin endpoint and
	// returns the raw response bytes (status line + headers + body) for the
	// differential diff. Phase 01.
	ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error)
}

// DistributionAsserter is an optional driver-side check the runner invokes
// after Drive when the driver implements it. The runner passes per-backend
// accept counts in the same order as backendPorts.
type DistributionAsserter interface {
	AssertDistribution(refCounts, subjCounts []uint64) error
}

// TB is the minimal testing interface that *testing.T satisfies. It is used by
// StatsAsserter so that fixture.go avoids a direct import of the "testing"
// package (which would leak into driver packages that register via init()).
type TB interface {
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Helper()
}

// StatsAsserter is an optional driver-side interface the runner invokes after
// ProbeAdmin when the driver implements it. Per SPEC §12 #6 (in-band
// assertion, no generic StatsExpectations data structure): the runner passes
// both admin addresses it already holds, and the driver performs the
// scrape-and-diff assertion in-band. Introduced by ADR-0062.
type StatsAsserter interface {
	AssertStats(t TB, refAdminAddr, subjAdminAddr string)
}

// DriverRegistry maps fixture names to their registered drivers. Drivers register
// themselves from init() via RegisterFixture.
var DriverRegistry = map[string]Driver{}

// RegisterFixture is called from a driver's init().
func RegisterFixture(name string, d Driver) {
	if _, dup := DriverRegistry[name]; dup {
		panic(fmt.Sprintf("duplicate fixture driver registration: %s", name))
	}
	DriverRegistry[name] = d
}

// HTTPRequestExpectation describes one HTTP/1.1 request the runner re-issues
// against ref and subject after Drive completes, to assert per-request status
// and body equivalence on top of the byte-stream comparison done by Drive.
//
// Phase 04 introduces this for fixture 0003-http11-routing. Phase 05 (HTTP/2)
// will reuse the shape — the path's protocol-version dimension is ignored
// here because phase 05's helpers will issue HTTP/2 round-trips via a
// different helper while populating the same struct.
//
// See ADR-0043.
type HTTPRequestExpectation struct {
	Method               string
	Path                 string
	ExpectStatus         int
	ExpectBodyEquivalent bool
}

// HTTPExpectations is an OPTIONAL fixture-driver interface. Drivers that
// implement it cause the runner to issue per-request HTTP round-trips against
// ref and subject after Drive completes, asserting status equivalence and
// (when ExpectBodyEquivalent is set) body byte-equivalence. Header set
// equality is checked via helpers.HTTPHeaderDiff under the phase-04 allow-list.
//
// Drivers that do NOT implement HTTPExpectations are unaffected (the runner's
// type assertion fails-silently and the new branch does not fire).
//
// See ADR-0043.
type HTTPExpectations interface {
	HTTPExpectations() []HTTPRequestExpectation
}

// BackendKind discriminates between TCP-echo and HTTP-echo backends for the
// runner's per-fixture spawning code. Default (when a driver does NOT
// implement BackendKindAware) is TCPEcho — matches phase-02/03 fixtures.
type BackendKind int

const (
	// TCPEcho is the accept-loop that reads-until-FIN and echoes bytes back; phase-02/03 default.
	TCPEcho BackendKind = 0
	// HTTPEcho is the accept-loop that reads one http.Request and writes "backend-<idx>:<lastSeg>" canned body, then closes.
	HTTPEcho BackendKind = 1
	// HTTPSH2 is an out-of-process backend: the runner spawns
	// test/fixtures/0004-h2-routing/backends/main.go (one process per backend
	// index) on the pre-allocated port, with --cert / --key flags pointing at
	// the fixture-local PKI and BACKEND_IDX env var supplying the per-instance
	// numeric idx. The backend serves TLS+h2 with NextProtos=["h2"] +
	// http2.ConfigureServer. Because the backend is a subprocess, the runner's
	// in-process accept counter is NOT incremented by these requests; drivers
	// that use HTTPSH2 must derive distribution from response bodies instead.
	HTTPSH2 BackendKind = 2
	// HTTPStatusHeader is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0005-prometheus-stats/backends/main.go on the pre-allocated
	// port. The backend reads the X-Backend-Status request header and returns the
	// requested status code; absent or invalid → 200. No TLS. Introduced by
	// fixture 0005 for the controlled-502 path in the stats differential
	// (ADR-0062). Because the backend is a subprocess, the runner's in-process
	// accept counter is NOT incremented.
	HTTPStatusHeader BackendKind = 3
	// HTTPFixedBody is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0006-access-log/backends/main.go on the pre-allocated port.
	// The backend returns 200 OK with a fixed 17-byte body regardless of path
	// (byte-identical across all instances — ensures BYTES_SENT Tier-E equality
	// against RR endpoint-selection divergence, per SPEC §7.2). No TLS.
	// Introduced by fixture 0006 / ADR-0068. Because the backend is a subprocess,
	// the runner's in-process accept counter is NOT incremented.
	HTTPFixedBody BackendKind = 4
	// HTTPHello is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0007a-cors/backends/main.go on the pre-allocated port.
	// The backend returns 200 OK with a fixed 6-byte body "hello\n"
	// regardless of path or method. No TLS. Introduced by fixture 0007a-cors
	// (Task 21) for actual-request body byte-equivalence on the cors
	// differential. Because the backend is a subprocess, the runner's
	// in-process accept counter is NOT incremented.
	HTTPHello BackendKind = 5
	// HTTPEchoBody is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0007b-iteration-probe/backends/main.go on the pre-allocated
	// port. The backend returns 200 OK with the request body if the request had
	// a non-empty body, else with the fixed 8-byte body "backend\n" (7 chars +
	// LF). No TLS. Introduced by fixture 0007b-iteration-probe (Task 22) for
	// the iteration-protocol structural fixture: the per-mode driver issues
	// some requests with bodies (POST) and some without (GET); the echo path
	// allows the modify-encode-data mode to verify the in-place body mutation,
	// and the fixed body covers the no-body modes. Because the backend is a
	// subprocess, the runner's in-process accept counter is NOT incremented.
	HTTPEchoBody BackendKind = 6
	// HTTPSlowStream is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0010-graceful-drain/backends/backend.go on the pre-allocated
	// port. The backend serves /slow which streams 5KB at 1KB/s (5s total), and
	// / which returns 200 OK with body "backend1\n". No TLS. Introduced by
	// fixture 0010-graceful-drain (phase 08.2 Task 12) to provide a stable 5s
	// in-flight window for the graceful-drain differential. Because the backend
	// is a subprocess, the runner's in-process accept counter is NOT incremented.
	HTTPSlowStream BackendKind = 7
	// HTTPFault is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0011-http-fault/backends/backend.go on the pre-allocated
	// port. The backend serves / with body "backend\n" (8 bytes). No TLS.
	// Introduced by fixture 0011-http-fault (phase 09 Task 10) to provide the
	// deterministic-body backend the per-scenario equivalence assertions
	// expect. Because the backend is a subprocess, the runner's in-process
	// accept counter is NOT incremented.
	HTTPFault BackendKind = 8
	// HTTPHeaderMutation is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0012-http-header-mutation/backends/backend.go on the pre-
	// allocated port. The backend serves / reflecting received request headers
	// into the response body (one header per line, sorted for determinism since
	// Go map iteration is non-deterministic) plus a single-value
	// X-Resp-Test: backend-original and multi-value X-Multi: alpha, beta response
	// headers (for OVERWRITE / APPEND multi-value testing per phase 10 SPEC §11.4).
	// No TLS. Introduced by fixture 0012-http-header-mutation (phase 10 Task 11).
	// Because the backend is a subprocess, the runner's in-process accept counter
	// is NOT incremented.
	HTTPHeaderMutation BackendKind = 9

	// HTTPLocalRateLimit is an out-of-process HTTP/1.1 backend: the runner
	// spawns test/fixtures/0013-http-local-ratelimit/backends/backend.go on
	// the pre-allocated port. The backend serves / with body "backend\n"
	// (8 bytes; Content-Type: text/plain; Content-Length: 8). No TLS.
	// Introduced by fixture 0013-http-local-ratelimit (phase 11 Task 9).
	// Because the backend is a subprocess, the runner's in-process accept
	// counter is NOT incremented.
	HTTPLocalRateLimit BackendKind = 10
	// HTTPCsrf is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0014-http-csrf/backends/backend.go on the pre-allocated
	// port. The backend serves "/" with body "backend\n" (8 bytes;
	// Content-Type: text/plain; Content-Length: 8). No TLS. Introduced by
	// fixture 0014-http-csrf (phase 12 Task 7). Because the backend is a
	// subprocess, the runner's in-process accept counter is NOT incremented.
	HTTPCsrf BackendKind = 11
	// HTTPBuffer is an out-of-process HTTP/1.1 backend: the runner spawns
	// test/fixtures/0015-http-buffer/backends/backend.go on the pre-allocated
	// port. The backend echoes the inbound request method + path + headers as a
	// JSON object in the response body — load-bearing for fixture scenario 6's
	// `Content-Length: 10240` assertion at the backend boundary per the
	// `maybeAddContentLength` mirror per phase 13 SPEC §11.8-CL. Status 200;
	// Content-Type: application/json. No TLS. Introduced by fixture
	// 0015-http-buffer (phase 13 Task 7). Because the backend is a subprocess,
	// the runner's in-process accept counter is NOT incremented.
	HTTPBuffer BackendKind = 12
	// HTTPCompressor is an out-of-process HTTP/1.1 backend: the runner spawns
	// the SHARED test/helpers/echobackend/cmd/echobackend binary on the pre-
	// allocated port. The backend echoes the inbound request method + path +
	// headers (lowercased canonical keys per ADR-0072) as a JSON object in the
	// response body — load-bearing for fixture 0016 scenario 6's per-route
	// remove_accept_encoding_header assertion (driver decompresses the gzipped
	// response body and asserts no "accept-encoding" key in the echoed headers
	// map, proving the per-route override stripped AE before forwarding
	// upstream). Status 200; Content-Type: application/json. No TLS.
	// Introduced by fixture 0016-http-compressor (phase 14 Task 10) per
	// planner-time decision 12 (D7 settlement) — the FIRST consumer of the
	// shared echobackend helper at test/helpers/echobackend/ (phase-13
	// buffer's per-fixture backend at test/fixtures/0015-http-buffer/backends/
	// MAY be migrated to use the shared helper in a future cleanup; out of
	// scope for phase 14). Because the backend is a subprocess, the runner's
	// in-process accept counter is NOT incremented.
	HTTPCompressor BackendKind = 13
	// HTTPBandwidthLimit is an out-of-process HTTP/1.1 backend: the runner spawns
	// the SHARED test/helpers/echobackend/cmd/echobackend binary on the pre-
	// allocated port (the same shared helper introduced at phase 14 Task 10 per
	// planner-time decision 12 / D7 settlement). The backend echoes the inbound
	// request method + path + headers (lowercased canonical keys per ADR-0072) as
	// a JSON object in the response body. Status 200; Content-Type:
	// application/json. No TLS. Introduced by fixture 0017-http-bandwidth-limit
	// (phase 15 Task 10): scenarios 2 (decode-side throttle) + 3 (encode-side
	// throttle) need a real upstream cluster (c_backend_b) so the differential
	// can observe per-direction token-bucket pacing on a real body round-trip;
	// scenarios 1 (filter disabled by runtime), 4 (per-route override), 5
	// (per-route disable), and 6 (multi-direction REQUEST_AND_RESPONSE) use
	// direct_response and therefore do NOT touch the echo backend (the runner
	// still spawns it because BackendCount() reports the maximum across all
	// scenarios). Because the backend is a subprocess, the runner's in-process
	// accept counter is NOT incremented.
	HTTPBandwidthLimit BackendKind = 14
	// HTTPRbac reuses the existing echobackend helper at
	// test/helpers/echobackend/cmd/echobackend/main.go for scenarios that exercise
	// upstream routes (scenarios 5 + 6 + 8). Three-listener fixture (l_test_a
	// plaintext + l_test_b echo-backend + l_test_a_tls mTLS-required for
	// scenario 6). No new helper authored at phase 16 — phase-14's echobackend
	// remains the shared helper.
	HTTPRbac BackendKind = 15
	// HTTPJwtAuthn reuses the existing echobackend helper for upstream routes +
	// the NEW jwksbackend helper at test/helpers/jwksbackend/ for in-process
	// JWKS-serving. Plaintext-only (no mTLS in phase 17 per SPEC §7.4). Used by
	// fixture 0019-http-jwt-authn (phase 17 Task 10): scenarios 1 + 7 fetch JWKS
	// from the in-process jwksbackend subprocess (RemoteJwks); scenario 2
	// exercises LocalJwks inline; remaining scenarios exercise deny paths +
	// CORS preflight bypass + per-route 8th canonical. Because both helpers run
	// as subprocesses, the runner's in-process accept counter is NOT incremented.
	HTTPJwtAuthn BackendKind = 16
	// HTTPExtAuthzHTTP reuses the existing echobackend helper at
	// test/helpers/echobackend/cmd/echobackend/main.go for the upstream route +
	// the NEW extauthzhttp helper at test/helpers/extauthzhttp/ for the in-process
	// HTTP auth server. 2-cluster topology (one HCM listener l_test_a plaintext
	// with ext_authz → router filter chain + cluster c_backend → echobackend
	// subprocess + cluster c_authz → extauthzhttp subprocess). No mTLS, no TLS,
	// no PKI — phase 18.1 fixture is HTTP/1.1 plaintext-only (D12). The
	// extauthzhttp helper is lifecycle-managed BY THE DRIVER (it needs the
	// per-scenario script); this switch-case only allocates the upstream echo
	// backend. Scenarios 3+4 (auth-server-unreachable) stop the extauthzhttp
	// server before the request. Because both helpers run as subprocesses (or
	// in-process for extauthzhttp), the runner's in-process accept counter is
	// NOT incremented. Introduced by phase 18.1 Task 10.
	HTTPExtAuthzHTTP BackendKind = 17
	// HTTPExtAuthzGRPC reuses the existing echobackend helper at
	// test/helpers/echobackend/cmd/echobackend/main.go for the upstream route
	// + the NEW extauthzgrpc helper at test/helpers/extauthzgrpc/ for the
	// in-process gRPC auth server. 2-cluster topology (three HCM listeners
	// l_test_a/b/c plaintext with ext_authz → router filter chain + cluster
	// c_backend → echobackend subprocess + cluster c_authz_grpc → extauthzgrpc
	// subprocess with http2_protocol_options: {}). No TLS — phase 18.2 fixture
	// is HTTP/1.1 plaintext downstream + plaintext h2c auth cluster per SPEC
	// §7.2 + §11.P13 in-session scrape RATIFICATION of TLS-to-auth-cluster.
	// The extauthzgrpc helper is lifecycle-managed BY THE DRIVER (it needs the
	// per-scenario Script registrations); this switch-case only allocates the
	// upstream echo backend. Scenarios that exercise auth-server-unreachable
	// stop the extauthzgrpc server before the request. Because both helpers
	// run as subprocesses (echobackend) or in-process (extauthzgrpc), the
	// runner's in-process accept counter is NOT incremented. Introduced by
	// phase 18.2 Task 9 / fixture 0021.
	HTTPExtAuthzGRPC BackendKind = 18
)

// BackendKindAware is an OPTIONAL driver-side method. Drivers that implement
// it select an alternative backend kind (e.g., HTTP-echo for phase-04 fixture
// 0003). Drivers that do NOT implement it default to TCPEcho.
type BackendKindAware interface {
	BackendKind() BackendKind
}

// HostMount describes a file bind-mount from the test host into the reference
// container. The file at HostPath must exist on the host before the container
// starts. HostPath is the absolute host-side file path; ContainerPath is the
// absolute in-container target path.
//
// This type is defined here (in the leaf fixture package) so that driver
// packages can use it without importing testcontainers-go directly. The runner
// translates each HostMount into a testcontainers.ContainerMount before calling
// StartReferenceProxyWithMounts.
type HostMount struct {
	HostPath      string
	ContainerPath string
}

// ReferenceLogMounter is an OPTIONAL driver-side interface the runner invokes
// before starting the reference container. Drivers that need to bind-mount a
// file into the container (e.g., fixture 0006-access-log needs to bind-mount the
// reference log file) implement this interface. The runner pre-creates each host
// file and sets permissions 0o666, then passes the mounts to
// StartReferenceProxyWithMounts. Introduced by fixture 0006 / ADR-0068.
type ReferenceLogMounter interface {
	ReferenceHostMounts() []HostMount
}

// AccessLogAsserter is an OPTIONAL driver-side interface the runner invokes
// after ProbeAdmin (step 10, mirroring StatsAsserter). Drivers that implement
// it receive the per-side log file paths it set up via ReferenceLogMounter and
// SubjectConfig, then assert the three-tier matrix. Introduced by ADR-0068.
type AccessLogAsserter interface {
	AssertAccessLog(t TB)
}

// ReferenceLessFixture is an OPTIONAL driver-side interface. Drivers that
// implement it returning false signal to the runner that this fixture has no
// reference Envoy counterpart — the runner SKIPS reference-proxy spawn,
// SKIPS DriveReference, SKIPS the byte-stream CompareBytes between ref/subj,
// and SKIPS the admin probe diff. Only DriveSubject + the optional
// SubjectAsserter run. Phase-07.1 fixture 0007b-iteration-probe is the first
// reference-less structural fixture (envoy-go-only iteration-protocol probe;
// no real Envoy filter equivalent exists for the envoy.filters.http.envoy_go_test
// type URL). The runner's per-fixture loop falls through the
// reference-spawn/drive/compare branches when this returns false; the
// SubjectAsserter (if also implemented) runs in-band against the subject's
// captured response stream.
//
// Drivers that do NOT implement this interface default to true (i.e., the
// pre-07.1 default — every fixture has a reference Envoy counterpart). This
// keeps the 0000–0007a fixtures untouched.
//
// Introduced by SPEC §7.4 + PLAN Task 22 / ADR-0074.
type ReferenceLessFixture interface {
	RequiresReference() bool
}

// SubjectAsserter is an OPTIONAL driver-side interface the runner invokes after
// DriveSubject when the driver implements it. The runner passes the bytes
// returned by DriveSubject; the driver performs in-band assertions against
// them (e.g., a structural fixture's per-mode embedded-expectation table). For
// reference-less fixtures (RequiresReference() == false) this is the only
// assertion path the runner exercises after Drive — the byte-stream
// CompareBytes against a reference is skipped.
//
// Introduced by fixture 0007b-iteration-probe (Task 22) so the driver's 8-mode
// assertion logic lives in-band per SPEC §12 #8 (mirrors the StatsAsserter +
// AccessLogAsserter precedent). Drivers that do NOT implement this interface
// experience a no-op runner branch.
type SubjectAsserter interface {
	AssertSubject(t TB, subjBytes []byte)
}

// MultiListenerDriver is an OPTIONAL driver-side interface for fixtures
// that target >1 listener simultaneously. Drivers that implement it return
// >=2 listener names (and matching reference ports); the runner allocates
// additional subject ports and exposes additional reference ports, then
// dispatches DriveReferenceMulti / DriveSubjectMulti instead of the
// single-addr Drive variants. The single-addr Driver methods
// (SubjectListenerName / ReferenceListenerPort / DriveReference /
// DriveSubject) MUST still be implemented (returning the first listener as
// the primary) so the runner's pre-multi-branch path still works for the
// fixture-discovery / admin-probe steps.
//
// Introduced by phase 07.2 / fixture-0008 per SPEC §7.4.
type MultiListenerDriver interface {
	SubjectListenerNames() []string
	ReferenceListenerPorts() []int
	DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error)
	DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error)
}

// AlternateConfigDriver is an OPTIONAL driver-side interface for fixtures
// that need to spawn a SECOND ref+subj pair with an alternate bootstrap
// (e.g., to exercise a code path the primary bootstrap cannot reach
// without removing one of its chains). The runner spawns the alternate
// pair AFTER the primary diff completes, runs DriveAlternate against the
// alternate addrs, and diffs the resulting bytes. fixture-0008 uses this
// for the c4 variant (chain_other removed) which exercises the
// default_filter_chain fallback.
//
// Introduced by phase 07.2 / fixture-0008 per SPEC §7.4 + Decision G.
type AlternateConfigDriver interface {
	AlternateReferenceBootstrap(backendPorts []int) string
	AlternateSubjectConfig(refListenerPort, subjListenerPort int, backendPorts []int, subjAdminPort int) string
	AlternateSubjectListenerName() string
	AlternateReferenceListenerPort() int
	DriveAlternate(ctx context.Context, refAddr, subjAddr string) ([]byte, error)
}
