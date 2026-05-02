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
