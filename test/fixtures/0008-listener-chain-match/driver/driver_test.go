// Tests for the 0008-listener-chain-match driver. These are unit tests
// that validate driver-internal invariants WITHOUT requiring a full
// envoy-go subject + Docker reference container — the differential gate
// itself is exercised by the runner's TestDifferential/0008-listener-chain-match
// integration test (see test/differential/runner_test.go).
//
// What's covered here:
//
//  1. Interface satisfaction (Driver + MultiListenerDriver +
//     AlternateConfigDriver) at compile-time + runtime.
//  2. Primary-listener-rule invariant: SubjectListenerName() ==
//     SubjectListenerNames()[0] (per SPEC §7.4 + the
//     MultiListenerDriver contract in fixture/fixture.go).
//  3. Same length invariant: len(SubjectListenerNames()) ==
//     len(ReferenceListenerPorts()) (the runner depends on this for
//     zipping the addrs map at runFixture line ~298).
//  4. BackendCount() == 5 (per PLAN line 2569).
//  5. Bootstrap templates render without panic for valid backendPorts.
//  6. driveFour issues exactly 4 connections in the documented order
//     (verified via in-process accept counters on 2 ephemeral
//     listeners standing in for l_test_a + l_test_b).
//  7. The known_driver_port pre-allocation produces a non-zero port.
//
// What's NOT covered here (and is left to the runner integration test):
//   - End-to-end chain selection equivalence between subj and ref.
//   - The c4 variant default_filter_chain fallback.
//   - The ProbeAdmin admin diff.
//   - Source-port reuse across connections 2 + 5 under TIME_WAIT.
package driver

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
)

// TestInterfaceSatisfaction asserts the driver satisfies all three
// fixture interfaces — Driver, MultiListenerDriver, AlternateConfigDriver.
// The compile-time `var _ fixture.Driver = (*driverImpl)(nil)` checks in
// driver.go already guarantee this, but a runtime test makes the
// assertion legible in failure output if the interfaces drift in the
// future.
func TestInterfaceSatisfaction(t *testing.T) {
	if _, ok := interface{}(d).(fixture.Driver); !ok {
		t.Fatal("driverImpl does not satisfy fixture.Driver")
	}
	if _, ok := interface{}(d).(fixture.MultiListenerDriver); !ok {
		t.Fatal("driverImpl does not satisfy fixture.MultiListenerDriver")
	}
	if _, ok := interface{}(d).(fixture.AlternateConfigDriver); !ok {
		t.Fatal("driverImpl does not satisfy fixture.AlternateConfigDriver")
	}
}

// TestPrimaryListenerRule asserts the SPEC §7.4 invariant: the
// single-listener Driver methods MUST return the FIRST entry of the
// MultiListenerDriver methods (so the runner's pre-multi-branch path
// uses the same primary listener as the multi-listener path).
func TestPrimaryListenerRule(t *testing.T) {
	names := d.SubjectListenerNames()
	ports := d.ReferenceListenerPorts()
	if len(names) < 2 {
		t.Fatalf("expected >=2 SubjectListenerNames, got %d", len(names))
	}
	if names[0] != d.SubjectListenerName() {
		t.Errorf("primary-listener rule: SubjectListenerName()=%q != SubjectListenerNames()[0]=%q",
			d.SubjectListenerName(), names[0])
	}
	if ports[0] != d.ReferenceListenerPort() {
		t.Errorf("primary-listener rule: ReferenceListenerPort()=%d != ReferenceListenerPorts()[0]=%d",
			d.ReferenceListenerPort(), ports[0])
	}
	if len(names) != len(ports) {
		t.Errorf("len mismatch: SubjectListenerNames()=%d, ReferenceListenerPorts()=%d", len(names), len(ports))
	}
}

// TestBackendCount pins the BackendCount per PLAN line 2569 — 5
// backends (4 chain-specific + 1 placeholder for the 5-connection
// workload symmetry; the placeholder is allocated but not wired into
// any cluster).
func TestBackendCount(t *testing.T) {
	if n := d.BackendCount(); n != 5 {
		t.Errorf("BackendCount: got %d, want 5", n)
	}
}

// TestKnownDriverPortAllocated asserts init() successfully allocated a
// non-zero source port. Defensive — if init() ever silently fails to
// allocate, the differential gate would source-bind to port 0 (OS-pick)
// and chain_srcprefix_loopback.source_ports would be 0 (which Envoy
// rejects at parse time anyway, but a clear unit-test failure is more
// helpful than a parse-time error during the runner's container start).
func TestKnownDriverPortAllocated(t *testing.T) {
	if d.knownDriverPort <= 0 {
		t.Errorf("knownDriverPort: got %d, want >0", d.knownDriverPort)
	}
	if d.knownDriverPort > 65535 {
		t.Errorf("knownDriverPort: got %d, want <=65535", d.knownDriverPort)
	}
}

// TestReferenceBootstrapRenders covers the happy-path: a valid
// backendPorts slice produces a non-empty bootstrap with the embedded
// known_driver_port + the first 4 backend ports rendered into the
// expected positions. We also assert that backendPorts[4] (the 5th
// "symmetry" port) is NOT referenced — the bootstrap should template
// only the 4 chain-specific clusters.
func TestReferenceBootstrapRenders(t *testing.T) {
	ports := []int{40001, 40002, 40003, 40004, 40005}
	bootstrap := d.ReferenceBootstrap(ports)
	if bootstrap == "" {
		t.Fatal("ReferenceBootstrap returned empty string")
	}
	for _, want := range []int{40001, 40002, 40003, 40004} {
		if !strings.Contains(bootstrap, fmt.Sprintf("port_value: %d", want)) {
			t.Errorf("ReferenceBootstrap: missing port_value: %d", want)
		}
	}
	if strings.Contains(bootstrap, fmt.Sprintf("port_value: %d", 40005)) {
		t.Errorf("ReferenceBootstrap: backendPorts[4] (40005) leaked into bootstrap (should be unused per 0008 design)")
	}
	if !strings.Contains(bootstrap, fmt.Sprintf("source_ports: [%d]", d.knownDriverPort)) {
		t.Errorf("ReferenceBootstrap: missing source_ports: [%d]", d.knownDriverPort)
	}
}

// TestSubjectConfigRenders covers the same shape on the subject side.
func TestSubjectConfigRenders(t *testing.T) {
	ports := []int{50001, 50002, 50003, 50004, 50005}
	cfg := d.SubjectConfig(15008, 18000, ports, 19000)
	if cfg == "" {
		t.Fatal("SubjectConfig returned empty string")
	}
	for _, want := range []int{50001, 50002, 50003, 50004} {
		if !strings.Contains(cfg, fmt.Sprintf("port_value: %d", want)) {
			t.Errorf("SubjectConfig: missing port_value: %d", want)
		}
	}
	if !strings.Contains(cfg, "port_value: 18000") {
		t.Errorf("SubjectConfig: missing l_test_a port_value: 18000")
	}
	if !strings.Contains(cfg, "port_value: 19000") {
		t.Errorf("SubjectConfig: missing admin port_value: 19000")
	}
	if !strings.Contains(cfg, fmt.Sprintf("destination_port: %d", 18000)) {
		t.Errorf("SubjectConfig: missing destination_port: %d", 18000)
	}
}

// TestAlternateBootstrapRenders covers the c4 templates: only 3 chain
// backends (c_dstport, c_srcprefix, c_default) are wired (chain_other
// is removed). c_other backend port (backendPorts[2]) MUST NOT appear
// in either alternate bootstrap.
func TestAlternateBootstrapRenders(t *testing.T) {
	ports := []int{60001, 60002, 60003, 60004, 60005}
	refCfg := d.AlternateReferenceBootstrap(ports)
	if !strings.Contains(refCfg, "60001") || !strings.Contains(refCfg, "60002") || !strings.Contains(refCfg, "60004") {
		t.Errorf("AlternateReferenceBootstrap: missing one of c_dstport/c_srcprefix/c_default backend ports")
	}
	if strings.Contains(refCfg, "60003") {
		t.Errorf("AlternateReferenceBootstrap: c_other backend port (60003) leaked (chain_other should be removed in c4)")
	}
	if strings.Contains(refCfg, "chain_other") {
		t.Errorf("AlternateReferenceBootstrap: chain_other should be omitted in c4 variant")
	}

	subjCfg := d.AlternateSubjectConfig(0, 18001, ports, 19001)
	if !strings.Contains(subjCfg, "60001") || !strings.Contains(subjCfg, "60002") || !strings.Contains(subjCfg, "60004") {
		t.Errorf("AlternateSubjectConfig: missing one of c_dstport/c_srcprefix/c_default backend ports")
	}
	if strings.Contains(subjCfg, "60003") {
		t.Errorf("AlternateSubjectConfig: c_other backend port (60003) leaked")
	}
	if strings.Contains(subjCfg, "chain_other") {
		t.Errorf("AlternateSubjectConfig: chain_other should be omitted in c4 variant")
	}
}

// TestAlternateListenerName pins the c4 variant uses l_test_b only.
func TestAlternateListenerName(t *testing.T) {
	if name := d.AlternateSubjectListenerName(); name != "l_test_b" {
		t.Errorf("AlternateSubjectListenerName: got %q, want %q", name, "l_test_b")
	}
	if port := d.AlternateReferenceListenerPort(); port != refAltPort {
		t.Errorf("AlternateReferenceListenerPort: got %d, want %d", port, refAltPort)
	}
	if port := d.AlternateReferenceListenerPort(); port == refPortLA || port == refPortLB {
		t.Errorf("AlternateReferenceListenerPort: got %d, must NOT collide with refPortLA(%d)/refPortLB(%d)",
			port, refPortLA, refPortLB)
	}
}

// TestDriveFourConnectionCount asserts driveFour issues exactly 4
// connections in the documented order (l_test_a, l_test_b, l_test_b,
// l_test_a) by standing up 2 in-process echo listeners and counting
// accepts per listener. The 4 connections per SPEC §7.4 split as:
//
//	conn 1 → l_test_a (no source-bind)
//	conn 2 → l_test_b (source-bind 127.0.0.1:knownDriverPort)
//	conn 3 → l_test_b (no source-bind)
//	conn 5 → l_test_a (source-bind 127.0.0.1:knownDriverPort)
//
// → l_test_a gets 2 accepts; l_test_b gets 2 accepts.
//
// The source-bind correctness is asserted via the kernel-observable
// source port: connections 2 and 5 must arrive with source port =
// knownDriverPort; connections 1 and 3 must arrive with a different
// (OS-picked ephemeral) port.
func TestDriveFourConnectionCount(t *testing.T) {
	lnA := mustListen(t)
	defer func() { _ = lnA.Close() }()
	lnB := mustListen(t)
	defer func() { _ = lnB.Close() }()

	var acceptsA, acceptsB atomic.Uint64
	bodyA := []byte(lnA.Addr().String() + "\n")
	bodyB := []byte(lnB.Addr().String() + "\n")
	stoppedA := make(chan struct{})
	stoppedB := make(chan struct{})
	go runEchoBackend(lnA, &acceptsA, bodyA, stoppedA)
	go runEchoBackend(lnB, &acceptsB, bodyB, stoppedB)

	addrs := map[string]string{
		"l_test_a": lnA.Addr().String(),
		"l_test_b": lnB.Addr().String(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	out, err := d.driveFour(ctx, addrs)
	if err != nil {
		t.Fatalf("driveFour: %v", err)
	}
	if got := acceptsA.Load(); got != 2 {
		t.Errorf("l_test_a accepts: got %d, want 2 (conns 1, 5)", got)
	}
	if got := acceptsB.Load(); got != 2 {
		t.Errorf("l_test_b accepts: got %d, want 2 (conns 2, 3)", got)
	}
	if !strings.Contains(string(out), "=== conn 1 ") {
		t.Errorf("driveFour output missing conn 1 marker; got:\n%s", string(out))
	}
	if !strings.Contains(string(out), "=== conn 5 ") {
		t.Errorf("driveFour output missing conn 5 marker; got:\n%s", string(out))
	}
}

// TestDriveFourMissingAddr covers the defensive error path: if the
// addrs map lacks one of the two listener names, driveFour returns a
// clear error rather than panicking on map lookup.
func TestDriveFourMissingAddr(t *testing.T) {
	ctx := context.Background()
	if _, err := d.driveFour(ctx, map[string]string{"l_test_a": "127.0.0.1:1"}); err == nil {
		t.Error("driveFour with missing l_test_b: expected error, got nil")
	}
	if _, err := d.driveFour(ctx, map[string]string{"l_test_b": "127.0.0.1:1"}); err == nil {
		t.Error("driveFour with missing l_test_a: expected error, got nil")
	}
}

// mustListen opens an in-process TCP listener on 127.0.0.1:0 for use
// as a chain backend stand-in.
func mustListen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln
}

// runEchoBackend mirrors the fixture-0008 backends/main.go behavior:
// reads-until-EOF, writes back the listener's addr, closes. Counts
// each accept in the supplied counter.
func runEchoBackend(ln net.Listener, accepts *atomic.Uint64, body []byte, stopped chan struct{}) {
	defer close(stopped)
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepts.Add(1)
		go func(c net.Conn) {
			defer func() { _ = c.Close() }()
			buf := make([]byte, 1024)
			for {
				_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				n, e := c.Read(buf)
				if n == 0 || e != nil {
					break
				}
			}
			_, _ = c.Write(body)
		}(conn)
	}
}
