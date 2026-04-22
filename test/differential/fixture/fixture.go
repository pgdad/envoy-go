// Package fixture defines the FixtureDriver interface and the global registry
// that fixture driver packages populate via init(). It is a leaf package with
// no dependency on test/differential so that driver packages can import it
// without creating an import cycle.
package fixture

import (
	"context"
	"fmt"
)

// FixtureDriver is the contract a fixture under test/fixtures/NNNN-*/driver
// implements. Drivers register themselves in init(); the runner discovers
// them by name (which must match the fixture directory).
type FixtureDriver interface {
	// Drive sends fixture-specific traffic at refAddr and subjAddr (each is a
	// host:port for the listener under test in each proxy). Returns the
	// captured byte streams for diffing.
	Drive(ctx context.Context, refAddr, subjAddr string) (refBytes, subjBytes []byte, err error)

	// ReferenceBootstrap returns the YAML to feed upstream Envoy.
	ReferenceBootstrap() string

	// SubjectConfig returns the YAML to feed envoy-go.
	SubjectConfig(refListenerPort, subjListenerPort, backendPort int) string

	// ReferenceListenerPort is the in-container TCP port the reference proxy
	// must expose (the listener the driver dials).
	ReferenceListenerPort() int
}

var DriverRegistry = map[string]FixtureDriver{}

// RegisterFixture is called from a driver's init().
func RegisterFixture(name string, d FixtureDriver) {
	if _, dup := DriverRegistry[name]; dup {
		panic(fmt.Sprintf("duplicate fixture driver registration: %s", name))
	}
	DriverRegistry[name] = d
}
