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
	// Drive completes.
	BackendCount() int

	// SubjectListenerName is the listener name the driver's Drive targets.
	// The runner uses subj.ListenerAddr(SubjectListenerName()) to look up
	// the subject's bound address per the ADR-0026 sentinel format.
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

	// Drive sends fixture-specific traffic at refAddr and subjAddr (each is
	// a host:port for the listener under test in each proxy). Returns the
	// captured byte streams for diffing. Runners may pass "" for either
	// side to signal "don't drive this side"; drivers must no-op the
	// corresponding side in that case.
	Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error)

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
