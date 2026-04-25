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
