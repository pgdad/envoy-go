// Phase 07.2 / Task 13: type-assertion smoke tests for the two new
// optional driver-side interfaces (MultiListenerDriver, AlternateConfigDriver)
// introduced for fixture-0008 (SPEC §7.4 + Decision G). The existing Driver
// interface is UNCHANGED — multi-listener stubs still implement it with the
// FIRST listener as the primary so the runner's pre-multi-branch path
// (fixture discovery / admin probe) keeps working.
package fixture

import (
	"context"
	"testing"
)

// stubMultiAltDriver implements Driver, MultiListenerDriver, and
// AlternateConfigDriver. It is the type-assertion fixture for
// TestOptionalInterfaces — every method is a no-op or returns canned
// values; nothing here exercises real proxies. The single-addr Driver
// methods return the FIRST listener as the primary per SPEC §7.4.
type stubMultiAltDriver struct{}

// --- Driver (single-addr, required) ---

func (stubMultiAltDriver) BackendCount() int           { return 5 }
func (stubMultiAltDriver) SubjectListenerName() string { return "l_test_a" }
func (stubMultiAltDriver) ReferenceListenerPort() int  { return 15008 }
func (stubMultiAltDriver) ReferenceBootstrap(_ []int) string {
	return ""
}
func (stubMultiAltDriver) SubjectConfig(_, _ int, _ []int, _ int) string { return "" }
func (stubMultiAltDriver) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (stubMultiAltDriver) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}
func (stubMultiAltDriver) ProbeAdmin(_ context.Context, _, _ string) ([]byte, []byte, error) {
	return nil, nil, nil
}

// --- MultiListenerDriver (optional) ---

func (stubMultiAltDriver) SubjectListenerNames() []string { return []string{"l_test_a", "l_test_b"} }
func (stubMultiAltDriver) ReferenceListenerPorts() []int  { return []int{15008, 15009} }
func (stubMultiAltDriver) DriveReferenceMulti(_ context.Context, _ map[string]string) ([]byte, error) {
	return nil, nil
}
func (stubMultiAltDriver) DriveSubjectMulti(_ context.Context, _ map[string]string) ([]byte, error) {
	return nil, nil
}

// --- AlternateConfigDriver (optional) ---

func (stubMultiAltDriver) AlternateReferenceBootstrap(_ []int) string { return "" }
func (stubMultiAltDriver) AlternateSubjectConfig(_, _ int, _ []int, _ int) string {
	return ""
}
func (stubMultiAltDriver) AlternateSubjectListenerName() string { return "l_test_a" }
func (stubMultiAltDriver) AlternateReferenceListenerPort() int  { return 15010 }
func (stubMultiAltDriver) DriveAlternate(_ context.Context, _, _ string) ([]byte, error) {
	return nil, nil
}

// Compile-time interface checks: stubMultiAltDriver MUST satisfy all three
// interfaces. If any method is missing or has a wrong signature, the
// package fails to compile (Task 13 acceptance: "type-assertion happy path
// is exercised by a stub driver in the test").
var (
	_ Driver                = stubMultiAltDriver{}
	_ MultiListenerDriver   = stubMultiAltDriver{}
	_ AlternateConfigDriver = stubMultiAltDriver{}
)

// TestOptionalInterfaces verifies the runtime type-assertion path the
// runner uses (Task 14 will branch on these). Also smoke-tests the
// canned-value methods the stub returns.
func TestOptionalInterfaces(t *testing.T) {
	var d Driver = stubMultiAltDriver{}

	mld, ok := d.(MultiListenerDriver)
	if !ok {
		t.Fatalf("stubMultiAltDriver does not satisfy MultiListenerDriver; type-assertion failed")
	}
	names := mld.SubjectListenerNames()
	if len(names) < 2 {
		t.Fatalf("SubjectListenerNames(): expected >=2 entries, got %d (%v)", len(names), names)
	}
	if names[0] != "l_test_a" {
		t.Fatalf("SubjectListenerNames()[0]: expected %q (primary), got %q", "l_test_a", names[0])
	}
	if got, want := d.SubjectListenerName(), names[0]; got != want {
		t.Fatalf("Driver.SubjectListenerName()=%q must equal MultiListenerDriver.SubjectListenerNames()[0]=%q (SPEC §7.4 primary-listener rule)", got, want)
	}
	if ports := mld.ReferenceListenerPorts(); len(ports) != len(names) {
		t.Fatalf("ReferenceListenerPorts(): expected %d entries (one per listener name), got %d", len(names), len(ports))
	}

	acd, ok := d.(AlternateConfigDriver)
	if !ok {
		t.Fatalf("stubMultiAltDriver does not satisfy AlternateConfigDriver; type-assertion failed")
	}
	if got, want := acd.AlternateReferenceListenerPort(), 15010; got != want {
		t.Fatalf("AlternateReferenceListenerPort(): got %d, want %d", got, want)
	}
	if acd.AlternateSubjectListenerName() == "" {
		t.Fatalf("AlternateSubjectListenerName(): expected non-empty")
	}
}

// TestOptionalInterfaces_NotImplemented verifies the negative path: a
// driver that implements ONLY the base Driver interface MUST fail the
// type-assertion to either optional interface (so the runner's standard
// path runs unchanged for pre-existing fixtures 0000-0007b).
func TestOptionalInterfaces_NotImplemented(t *testing.T) {
	var d Driver = baseOnlyStub{}

	if _, ok := d.(MultiListenerDriver); ok {
		t.Fatalf("baseOnlyStub MUST NOT satisfy MultiListenerDriver (would break pre-existing fixtures' standard runner path)")
	}
	if _, ok := d.(AlternateConfigDriver); ok {
		t.Fatalf("baseOnlyStub MUST NOT satisfy AlternateConfigDriver (would break pre-existing fixtures' standard runner path)")
	}
}

// baseOnlyStub implements ONLY the base Driver interface — used by the
// negative type-assertion test above.
type baseOnlyStub struct{}

func (baseOnlyStub) BackendCount() int                                          { return 1 }
func (baseOnlyStub) SubjectListenerName() string                                { return "l" }
func (baseOnlyStub) ReferenceBootstrap(_ []int) string                          { return "" }
func (baseOnlyStub) SubjectConfig(_, _ int, _ []int, _ int) string              { return "" }
func (baseOnlyStub) ReferenceListenerPort() int                                 { return 0 }
func (baseOnlyStub) DriveReference(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (baseOnlyStub) DriveSubject(_ context.Context, _ string) ([]byte, error)   { return nil, nil }
func (baseOnlyStub) ProbeAdmin(_ context.Context, _, _ string) ([]byte, []byte, error) {
	return nil, nil, nil
}

// TestHostMount_DirFieldExists is the Task 12 (D-TAP-DIRMOUNT) TDD anchor:
// HostMount gains a Dir bool field so the runner can pre-create a directory
// mount (for file_per_tap's unpredictable-filename sink) instead of always
// pre-creating a single file. Dir must default to false, preserving the
// 0006-access-log single-file-mount behavior byte-for-byte.
func TestHostMount_DirFieldExists(t *testing.T) {
	m := HostMount{HostPath: "/tmp/x", ContainerPath: "/envoy-go-test/taps", Dir: true}
	if !m.Dir {
		t.Errorf("HostMount.Dir must be settable")
	}
	if (HostMount{HostPath: "/tmp/y", ContainerPath: "/c"}).Dir {
		t.Errorf("HostMount.Dir must default to false (file mount), preserving the 0006 behavior")
	}
}
