// Package driver registers the 0015-http-buffer differential fixture.
//
// The full driver body lands in Task 11; this Task-7 stub registers the fixture
// with the runner so the BackendKind plumbing can be wired without breaking
// the runner's switch. The stub returns no scenarios; the runner's per-fixture
// test will SKIP until Task 11 populates the body.
package driver

import (
	"context"

	"github.com/esalaine/envoy-go/test/differential/fixture"
)

func init() {
	fixture.RegisterFixture("0015-http-buffer", &bufferDriver{})
}

type bufferDriver struct{}

// --- fixture.BackendKindAware ---

func (bufferDriver) BackendKind() fixture.BackendKind { return fixture.HTTPBuffer }

// --- fixture.Driver (required) ---

func (bufferDriver) BackendCount() int           { return 1 }
func (bufferDriver) SubjectListenerName() string { return "" }
func (bufferDriver) ReferenceListenerPort() int  { return 0 }

func (bufferDriver) ReferenceBootstrap(_ []int) string { return "" }

func (bufferDriver) SubjectConfig(_, _ int, _ []int, _ int) string { return "" }

func (bufferDriver) DriveReference(_ context.Context, _ string) ([]byte, error) { return nil, nil }

func (bufferDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) { return nil, nil }

func (bufferDriver) ProbeAdmin(_ context.Context, _, _ string) ([]byte, []byte, error) {
	return nil, nil, nil
}

// Compile-time interface assertions.
var (
	_ fixture.Driver           = (*bufferDriver)(nil)
	_ fixture.BackendKindAware = (*bufferDriver)(nil)
)
