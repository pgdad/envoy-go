// Package driver registers the 0008-listener-chain-match fixture with the
// differential runner. This is the project's first MULTI-listener fixture
// and the first to use AlternateConfigDriver — it asserts wire-equivalent
// per-connection chain selection between envoy-go and reference Envoy
// v1.37.2 across the 5-connection workload pinned in SPEC §7.4.
//
// Integration shape (SPEC §9.2 driver outline):
//
//  1. Two listeners (l_test_a, l_test_b) per primary bootstrap. Each
//     carries the SAME chain set: chain_dstport_alpha (matches
//     destination_port = port_a), chain_srcprefix_loopback (matches
//     source_prefix_ranges + source_ports), chain_other (empty match
//     catch-all), default_filter_chain (no-match fallback). The c4
//     variant omits chain_other so connection 4 falls through to
//     default_filter_chain (the §11.1 empirical-pin demonstration).
//
//  2. Driver pre-allocates a known_driver_port in init() via
//     net.Listen("tcp","127.0.0.1:0") + close. This port is embedded in
//     BOTH bootstraps' chain_srcprefix_loopback.source_ports and
//     SOURCE-bound on connections 2 and 5 via net.Dialer{LocalAddr:...}.
//
//  3. DriveSubjectMulti / DriveReferenceMulti issue 4 sequential TCP
//     connections (1, 2, 3, 5 — the primary-variant connections) and
//     concatenate their response bodies (each backend echoes its own
//     ln.Addr().String(), so the body uniquely identifies the chain
//     selected). Connection 4 is issued by DriveAlternate against the
//     c4 variant.
//
//  4. ProbeAdmin issues GET /ready against each proxy's admin endpoint,
//     mirroring all pre-existing fixtures.
//
// Source-IP cross-side compatibility:
//
//	The reference Envoy runs in a Docker container with bridge networking;
//	the test driver runs on the host. Connections from the host to the
//	reference container's published port arrive at Envoy with a SOURCE IP
//	that is NOT 127.0.0.1 (it is the Docker bridge gateway, e.g.
//	172.17.0.1, due to Docker's bridge NAT). A literal `127.0.0.1/32`
//	source_prefix_ranges on the reference would therefore eliminate
//	chain_srcprefix_loopback for ALL connections, breaking the
//	differential against the subject (which dials 127.0.0.1 from
//	127.0.0.1 and sees a real loopback source IP).
//
//	To preserve cross-side equivalence, BOTH templates use
//	`source_prefix_ranges: 0.0.0.0/0` — universally matching on source IP.
//	The discriminator for chain_srcprefix_loopback becomes purely
//	`source_ports: [known_driver_port]`. The chain still has slot 6
//	(SourcePrefixRanges) and slot 7 (SourcePorts) specified, so the
//	specificity vector is unchanged for the §11.3 precedence test
//	(connection 5: chain_dstport_alpha at slot 0 still BEATS
//	chain_srcprefix_loopback at slots {6,7}). The static fixture YAMLs
//	at the fixture root use 127.0.0.1/32 as illustrative documentation;
//	the driver-embedded const templates are the load-bearing definition
//	per the established 0001/0007a precedent (static YAMLs are not loaded
//	by the runner — see also `test/fixtures/0007a-cors/driver/driver.go`
//	for the same pattern).
package driver

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	"github.com/pgdad/envoy-go/test/differential/fixture"
	"github.com/pgdad/envoy-go/test/helpers"
)

const fixtureName = "0008-listener-chain-match"

// Reference container in-container listener ports (l_test_a, l_test_b for
// primary; l_test_b for c4 variant). Pinned values mirror the static YAMLs
// at the fixture root (envoy.yaml uses 15008+15009; envoy-c4.yaml uses
// 15009; the c4 variant uses a separate slot so its admin and l_test_b
// don't collide with a still-running primary container — testcontainers
// publishes them to OS-picked host ports either way).
const (
	refPortLA  = 15008 // primary l_test_a
	refPortLB  = 15009 // primary l_test_b
	refAltPort = 15010 // c4 variant l_test_b — distinct so the alt
	// container doesn't collide with the primary container's exposed
	// ports if the runner starts both concurrently. Driver-rendered into
	// alternateRefTmpl.
)

// driverImpl implements fixture.Driver, fixture.MultiListenerDriver, and
// fixture.AlternateConfigDriver. The single-listener Driver methods are
// REQUIRED by the interface contract (the runner's pre-multi-branch path
// uses SubjectListenerName / ReferenceListenerPort for fixture-discovery
// + admin-probe steps; DriveReference / DriveSubject are NEVER invoked
// for multi-listener drivers because the runner's runFixture body
// dispatches to the Multi variants when isMulti=true) but are stubbed.
type driverImpl struct {
	// knownDriverPort is the source port the driver pre-allocates in
	// init() and embeds in BOTH the primary AND the c4 bootstraps'
	// chain_srcprefix_loopback.source_ports[0]. It is also the source
	// port the driver SOURCE-binds for connections 2 and 5.
	knownDriverPort int
}

// d is the package-level singleton. init() populates knownDriverPort
// before registering with the fixture registry.
var d = &driverImpl{}

func init() {
	port, err := allocateKnownDriverPort()
	if err != nil {
		panic(fmt.Sprintf("0008: failed to allocate known_driver_port: %v", err))
	}
	d.knownDriverPort = port
	fixture.RegisterFixture(fixtureName, d)
}

// allocateKnownDriverPort opens a TCP listener on 127.0.0.1:0, captures
// the OS-picked port, closes the listener, and returns the port number.
// The port is then EMBEDDED in chain_srcprefix_loopback.source_ports[0]
// of both bootstraps and SOURCE-bound by the driver on connections 2 + 5
// via net.Dialer{LocalAddr: &net.TCPAddr{IP: 127.0.0.1, Port: port}}.
//
// The race window between Close() and the driver's source-bind is
// theoretically non-zero; in practice the OS does not aggressively
// re-allocate the port (TIME_WAIT clears in seconds, and our use of
// SO_REUSEADDR in the source-bind also lets us reclaim it. Even without
// SO_REUSEADDR the kernel's ephemeral-port range avoids reuse in our
// time horizon).
func allocateKnownDriverPort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, fmt.Errorf("listen 127.0.0.1:0: %w", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	if err := ln.Close(); err != nil {
		return 0, fmt.Errorf("close: %w", err)
	}
	return port, nil
}

// --- fixture.Driver interface ---

func (d *driverImpl) BackendCount() int { return 5 }

// SubjectListenerName returns the PRIMARY listener name per SPEC §7.4
// (the "first" of SubjectListenerNames()). Required by the Driver
// interface; the runner uses it only at fixture-discovery / admin-probe
// steps when the driver does NOT implement MultiListenerDriver — for
// fixture-0008 (which DOES implement it) the runner dispatches via the
// Multi variants.
func (d *driverImpl) SubjectListenerName() string { return "l_test_a" }

// ReferenceListenerPort returns the PRIMARY in-container listener port
// per SPEC §7.4. Same compat-path role as SubjectListenerName.
func (d *driverImpl) ReferenceListenerPort() int { return refPortLA }

// ReferenceBootstrap renders the PRIMARY reference bootstrap (covering
// connections 1, 2, 3, 5). Uses host.docker.internal cluster endpoints
// per ADR-0010 (STRICT_DNS clusters). The %d placeholders are filled in
// declaration order; see referenceTmpl.
func (d *driverImpl) ReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) < 4 {
		panic(fmt.Sprintf("0008: expected >=4 backend ports, got %d", len(backendPorts)))
	}
	// referenceTmpl placeholders (in order of appearance):
	//   1. chain_dstport_alpha.destination_port (l_test_a in-container port)
	//   2. chain_srcprefix_loopback.source_ports[0] (known_driver_port)
	//   3. chain_dstport_alpha.destination_port (repeated on l_test_b — same value)
	//   4. chain_srcprefix_loopback.source_ports[0] (repeated)
	//   5. c_dstport endpoint port (backendPorts[0])
	//   6. c_srcprefix endpoint port (backendPorts[1])
	//   7. c_other endpoint port (backendPorts[2])
	//   8. c_default endpoint port (backendPorts[3])
	return fmt.Sprintf(referenceTmpl,
		refPortLA, d.knownDriverPort,
		refPortLA, d.knownDriverPort,
		backendPorts[0], backendPorts[1], backendPorts[2], backendPorts[3])
}

// SubjectConfig renders the PRIMARY subject bootstrap. The first arg is
// the reference's listener port (unused by 0008 — the subject listener
// uses a freshly-allocated subjListenerPort, not the reference's
// fixed-port-15008-in-container shape). Following args are the subject's
// own subjListenerPort_A (= subjListenerPort, runner-allocated), the
// secondary subject listener port subjListenerPort_B (driver-allocated
// via freeTCPPort-equivalent helper), the subjAdminPort, and the 5
// backend ports. The subject listener bootstrap is fully under driver
// control (the runner does not template it), so this method allocates
// subjListenerPort_B itself.
//
// CRITICAL: this method is called only ONCE per subject startup; the
// secondary listener port allocated here is NOT the same as a
// freeTCPPort racing with a re-listen. The OS-picked port may collide
// with subsequent driver source-binds; in practice the ephemeral-port
// allocator does not collide within the same process within a few
// hundred milliseconds, but the well-known mitigation if collision were
// observed empirically would be to allocate via net.Listen on
// 127.0.0.1:0 and keep the listener open until the bootstrap is
// installed (the listener manager is what unbinds it for re-bind via
// the YAML port_value placeholder). This is the same race window that
// fixture-0006 / fixture-0007a both already accept.
func (d *driverImpl) SubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) < 4 {
		panic(fmt.Sprintf("0008: expected >=4 backend ports, got %d", len(backendPorts)))
	}
	// subjListenerPort is the runner-allocated PRIMARY (= l_test_a) port.
	// We need a SECOND listener port for l_test_b — allocate one now via
	// the same OS-pick + close pattern as freeTCPPort.
	subjPortB, err := allocateKnownDriverPort()
	if err != nil {
		panic(fmt.Sprintf("0008: failed to allocate subjPortB: %v", err))
	}
	// subjectTmpl placeholders (in order of appearance):
	//   1. admin port_value (subjAdminPort)
	//   2. l_test_a port_value (subjListenerPort = l_test_a)
	//   3. chain_dstport_alpha.destination_port (= subjListenerPort = l_test_a port)
	//   4. chain_srcprefix_loopback.source_ports[0] (known_driver_port)
	//   5. l_test_b port_value (subjPortB)
	//   6. chain_dstport_alpha.destination_port on l_test_b (= subjListenerPort = same dstport value as l_test_a)
	//   7. chain_srcprefix_loopback.source_ports[0] on l_test_b (known_driver_port)
	//   8. c_dstport endpoint port (backendPorts[0])
	//   9. c_srcprefix endpoint port (backendPorts[1])
	//  10. c_other endpoint port (backendPorts[2])
	//  11. c_default endpoint port (backendPorts[3])
	return fmt.Sprintf(subjectTmpl,
		subjAdminPort,
		subjListenerPort,
		subjListenerPort, d.knownDriverPort,
		subjPortB,
		subjListenerPort, d.knownDriverPort,
		backendPorts[0], backendPorts[1], backendPorts[2], backendPorts[3])
}

// DriveReference is a stub; the runner's multi-listener branch dispatches
// to DriveReferenceMulti instead. See SPEC §7.4 + the MultiListenerDriver
// interface comment in fixture/fixture.go: multi-listener drivers must
// still implement Driver methods for compat, but the multi-listener
// runner branch supersedes them at drive time.
func (d *driverImpl) DriveReference(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

// DriveSubject is a stub; see DriveReference comment.
func (d *driverImpl) DriveSubject(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

// ProbeAdmin reuses the phase-01 raw-socket /ready probe shape (mirrors
// every other fixture's implementation).
func (d *driverImpl) ProbeAdmin(ctx context.Context, refAdminAddr, subjAdminAddr string) (refBytes, subjBytes []byte, err error) {
	refBytes, err = helpers.HTTPGetReadyRaw(ctx, refAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("ref admin: %w", err)
	}
	subjBytes, err = helpers.HTTPGetReadyRaw(ctx, subjAdminAddr)
	if err != nil {
		return nil, nil, fmt.Errorf("subj admin: %w", err)
	}
	return refBytes, subjBytes, nil
}

// --- fixture.MultiListenerDriver interface ---

// SubjectListenerNames returns the two primary listener names. Per the
// MultiListenerDriver contract, names[0] MUST equal SubjectListenerName().
func (d *driverImpl) SubjectListenerNames() []string {
	return []string{"l_test_a", "l_test_b"}
}

// ReferenceListenerPorts returns the two primary in-container listener
// ports. Length matches SubjectListenerNames() length per the contract.
func (d *driverImpl) ReferenceListenerPorts() []int {
	return []int{refPortLA, refPortLB}
}

// DriveReferenceMulti issues the 4 primary-variant connections against
// the reference proxy. Connections 1, 2, 3, 5 — connection 4 is the c4
// variant handled by DriveAlternate.
func (d *driverImpl) DriveReferenceMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveFour(ctx, addrs)
}

// DriveSubjectMulti issues the 4 primary-variant connections against
// the subject proxy. Logic identical to DriveReferenceMulti — both sides
// see the same byte stream when chain selection is equivalent.
func (d *driverImpl) DriveSubjectMulti(ctx context.Context, addrs map[string]string) ([]byte, error) {
	return d.driveFour(ctx, addrs)
}

// driveFour executes the 4 primary-variant connections (per SPEC §7.4)
// against the addrs map. Connection numbering follows SPEC §7.4 (1, 2,
// 3, 5 — connection 4 is c4-variant only, not driven here).
//
//	conn 1: dial l_test_a from default source            → expect chain_dstport_alpha → P_dstport
//	conn 2: dial l_test_b from 127.0.0.1:knownDriverPort → expect chain_srcprefix_loopback → P_srcprefix
//	conn 3: dial l_test_b from default source            → expect chain_other → P_other
//	conn 5: dial l_test_a from 127.0.0.1:knownDriverPort → expect chain_dstport_alpha (precedence) → P_dstport
//
// Each connection's response is the backend's ln.Addr().String() echoed
// once (newline-terminated). The concatenated stream is the differential
// surface — byte-equality across subj and ref verifies chain selection
// equivalence.
//
// Per-connection separator is the connection ordinal as ASCII to make
// debug output legible if the byte-stream differential fails — the
// underlying body bytes already differ per backend (each backend echoes
// its own port), so the separator is purely a debug aid; the runner's
// CompareBytes hexdump will surface mismatches with full per-line
// context.
func (d *driverImpl) driveFour(ctx context.Context, addrs map[string]string) ([]byte, error) {
	addrA, ok := addrs["l_test_a"]
	if !ok {
		return nil, fmt.Errorf("driveFour: missing l_test_a in addrs map")
	}
	addrB, ok := addrs["l_test_b"]
	if !ok {
		return nil, fmt.Errorf("driveFour: missing l_test_b in addrs map")
	}

	type connSpec struct {
		num     int
		addr    string
		bindLB  bool // if true, source-bind 127.0.0.1:knownDriverPort
		comment string
	}
	conns := []connSpec{
		{num: 1, addr: addrA, bindLB: false, comment: "chain_dstport_alpha"},
		{num: 2, addr: addrB, bindLB: true, comment: "chain_srcprefix_loopback"},
		{num: 3, addr: addrB, bindLB: false, comment: "chain_other"},
		{num: 5, addr: addrA, bindLB: true, comment: "chain_dstport_alpha (precedence)"},
	}

	var out bytes.Buffer
	for _, c := range conns {
		body, err := d.dialAndEcho(ctx, c.addr, c.bindLB)
		if err != nil {
			return nil, fmt.Errorf("conn %d (%s, bindLB=%v): %w", c.num, c.comment, c.bindLB, err)
		}
		fmt.Fprintf(&out, "=== conn %d (%s)\n", c.num, c.comment)
		out.Write(body)
	}
	return out.Bytes(), nil
}

// dialAndEcho dials addr (with optional source-bind to
// 127.0.0.1:knownDriverPort), writes a single trigger byte, half-closes
// the write side, and reads the response until EOF or read deadline.
// Returns the response bytes (the backend's ln.Addr().String() + "\n").
//
// The trigger byte is 't' (arbitrary, non-empty so the backend's
// io.Copy(io.Discard, conn) returns when the half-close arrives — empty
// payload would also work given the half-close, but a single-byte payload
// makes wireshark traces self-documenting).
//
// Source-bind uses net.Dialer{LocalAddr: &net.TCPAddr{IP: 127.0.0.1,
// Port: knownDriverPort}}. If the kernel rejects the source-bind (e.g.,
// a stray TIME_WAIT on the port) the dial fails — no graceful retry,
// the differential test surfaces the failure deterministically.
func (d *driverImpl) dialAndEcho(ctx context.Context, addr string, bindLB bool) ([]byte, error) {
	dialer := net.Dialer{}
	if bindLB {
		dialer.LocalAddr = &net.TCPAddr{
			IP:   net.ParseIP("127.0.0.1"),
			Port: d.knownDriverPort,
		}
		// Allow port reuse so the source-bind succeeds even if a stray
		// TIME_WAIT lingers from a previous run. SO_REUSEADDR + SO_REUSEPORT
		// are set via Control.
		dialer.Control = setReuseSockopts
	}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.Write([]byte("t")); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.CloseWrite()
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	var resp []byte
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if n > 0 {
			resp = append(resp, buf[:n]...)
		}
		if err != nil {
			break
		}
	}
	return resp, nil
}

// --- fixture.AlternateConfigDriver interface ---

// AlternateReferenceBootstrap renders the c4 reference bootstrap (only
// connection 4: chain_other removed; chain_default is the no-match
// fallback). Uses host.docker.internal cluster endpoints per ADR-0010.
func (d *driverImpl) AlternateReferenceBootstrap(backendPorts []int) string {
	if len(backendPorts) < 4 {
		panic(fmt.Sprintf("0008: expected >=4 backend ports, got %d", len(backendPorts)))
	}
	// alternateRefTmpl placeholders (in order):
	//   1. chain_dstport_alpha.destination_port (= primary l_test_a port,
	//      preserved for shape symmetry — no connection in c4 matches it)
	//   2. chain_srcprefix_loopback.source_ports[0] (known_driver_port)
	//   3. c_dstport endpoint port (backendPorts[0])
	//   4. c_srcprefix endpoint port (backendPorts[1])
	//   5. c_default endpoint port (backendPorts[3])
	return fmt.Sprintf(alternateRefTmpl,
		refPortLA, d.knownDriverPort,
		backendPorts[0], backendPorts[1], backendPorts[3])
}

// AlternateSubjectConfig renders the c4 subject bootstrap. refListenerPort
// is unused here (the c4 variant uses its own freshly-allocated alt
// subject port + admin port).
func (d *driverImpl) AlternateSubjectConfig(_, subjListenerPort int, backendPorts []int, subjAdminPort int) string {
	if len(backendPorts) < 4 {
		panic(fmt.Sprintf("0008: expected >=4 backend ports, got %d", len(backendPorts)))
	}
	// alternateSubjectTmpl placeholders (in order):
	//   1. admin port_value (subjAdminPort)
	//   2. l_test_b port_value (subjListenerPort)
	//   3. chain_dstport_alpha.destination_port (= primary l_test_a port — preserved)
	//   4. chain_srcprefix_loopback.source_ports[0] (known_driver_port)
	//   5. c_dstport endpoint port (backendPorts[0])
	//   6. c_srcprefix endpoint port (backendPorts[1])
	//   7. c_default endpoint port (backendPorts[3])
	return fmt.Sprintf(alternateSubjectTmpl,
		subjAdminPort,
		subjListenerPort,
		// c4 chain_dstport_alpha keeps the primary l_test_a port value (a
		// fixed sentinel that no c4 connection matches; preserves chain
		// shape across the two variants for SPEC narrative symmetry).
		// We re-use refPortLA's value for both sides since the actual
		// match is irrelevant — c4 connection 4 source has no source-bind
		// and dials port_b (subjListenerPort), so chain_dstport_alpha is
		// eliminated by destination_port mismatch regardless of the value
		// here. Using a fixed sentinel (refPortLA = 15008) on both subj
		// and ref keeps the shape identical.
		refPortLA, d.knownDriverPort,
		backendPorts[0], backendPorts[1], backendPorts[3])
}

// AlternateSubjectListenerName is the c4 subject's only listener — the
// c4 variant configures only l_test_b (per the c4 envoy-go-c4.yaml).
// Connection 4 dials this listener.
func (d *driverImpl) AlternateSubjectListenerName() string { return "l_test_b" }

// AlternateReferenceListenerPort is the c4 reference's only listener
// in-container port. Distinct from the primary's refPortLB (15009) so
// that the runner's two reference containers (primary + alternate) do
// not collide on exposed-port allocation if started concurrently. In
// practice the primary container starts first and is fully started
// before the alternate spawns, but the distinct value keeps the design
// resilient to future concurrency.
func (d *driverImpl) AlternateReferenceListenerPort() int { return refAltPort }

// DriveAlternate issues connection 4 against both ref and subj. Per SPEC
// §7.4 row 4: target l_test_b, no source-bind, expect chain_default →
// backend P_default. Returns the concatenated bytes (driver-internal
// in-band assertion mirrors SubjectAsserter / StatsAsserter / AccessLog
// Asserter precedent — the runner accepts the bytes; the byte-equality
// is what matters at the differential level).
//
// Both refAddr and subjAddr point at l_test_b on their respective sides
// (the runner zips the alt addrs from AlternateSubjectListenerName /
// AlternateReferenceListenerPort).
func (d *driverImpl) DriveAlternate(ctx context.Context, refAddr, subjAddr string) ([]byte, error) {
	// Connection 4: ref side first, then subj side. No source-bind.
	refBody, err := d.dialAndEcho(ctx, refAddr, false)
	if err != nil {
		return nil, fmt.Errorf("alt conn 4 ref: %w", err)
	}
	subjBody, err := d.dialAndEcho(ctx, subjAddr, false)
	if err != nil {
		return nil, fmt.Errorf("alt conn 4 subj: %w", err)
	}
	// In-band byte-equality check (per AlternateConfigDriver contract:
	// the runner accepts whatever bytes we return; the in-band check
	// surfaces divergence as an error from DriveAlternate, which the
	// runner converts to t.Fatalf). This mirrors the SubjectAsserter /
	// StatsAsserter precedent for in-band assertions.
	if !bytes.Equal(refBody, subjBody) {
		return nil, fmt.Errorf("alt conn 4 byte mismatch:\n  ref:  %q\n  subj: %q", refBody, subjBody)
	}
	var out bytes.Buffer
	out.WriteString("=== alt conn 4 (chain_default fallback)\n")
	out.Write(refBody)
	out.WriteString("--- subj ---\n")
	out.Write(subjBody)
	return out.Bytes(), nil
}

// Compile-time interface satisfaction checks. If any signature drifts
// from fixture/fixture.go, this package fails to compile — the canonical
// shape is enforced at the package boundary (mirrors 0001/0007a/0007b
// precedent).
var (
	_ fixture.Driver                = (*driverImpl)(nil)
	_ fixture.MultiListenerDriver   = (*driverImpl)(nil)
	_ fixture.AlternateConfigDriver = (*driverImpl)(nil)
)

// referenceTmpl is the PRIMARY reference Envoy bootstrap. Two listeners
// (l_test_a on 15008, l_test_b on 15009) carry the same 3 chains +
// default_filter_chain. STRICT_DNS clusters reach host backends via
// host.docker.internal per ADR-0010. The chain_srcprefix_loopback
// source_prefix_ranges is `0.0.0.0/0` (universally-matching) per the
// driver doc-comment's "Source-IP cross-side compatibility" section —
// the discriminator is purely source_ports.
//
// 8 sprintf placeholders (in order — see ReferenceBootstrap):
//
//	%[1]d = chain_dstport_alpha.destination_port (l_test_a port = 15008)
//	%[2]d = chain_srcprefix_loopback.source_ports[0] (known_driver_port)
//	%[3]d = chain_dstport_alpha.destination_port on l_test_b (same value as %[1]d)
//	%[4]d = chain_srcprefix_loopback.source_ports[0] on l_test_b (same value as %[2]d)
//	%[5]d = c_dstport endpoint port (backendPorts[0])
//	%[6]d = c_srcprefix endpoint port (backendPorts[1])
//	%[7]d = c_other endpoint port (backendPorts[2])
//	%[8]d = c_default endpoint port (backendPorts[3])
const referenceTmpl = `admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_test_a
      address:
        socket_address: { address: 0.0.0.0, port_value: 15008 }
      filter_chains:
        - name: chain_dstport_alpha
          filter_chain_match:
            destination_port: %d
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_dstport
                cluster: c_dstport
        - name: chain_srcprefix_loopback
          filter_chain_match:
            source_prefix_ranges:
              - { address_prefix: 0.0.0.0, prefix_len: 0 }
            source_ports: [%d]
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_srcprefix
                cluster: c_srcprefix
        - name: chain_other
          filter_chain_match: {}
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_other
                cluster: c_other
      default_filter_chain:
        name: chain_default
        filters:
          - name: envoy.filters.network.tcp_proxy
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
              stat_prefix: tcp_default
              cluster: c_default
    - name: l_test_b
      address:
        socket_address: { address: 0.0.0.0, port_value: 15009 }
      filter_chains:
        - name: chain_dstport_alpha
          filter_chain_match:
            destination_port: %d
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_dstport
                cluster: c_dstport
        - name: chain_srcprefix_loopback
          filter_chain_match:
            source_prefix_ranges:
              - { address_prefix: 0.0.0.0, prefix_len: 0 }
            source_ports: [%d]
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_srcprefix
                cluster: c_srcprefix
        - name: chain_other
          filter_chain_match: {}
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_other
                cluster: c_other
      default_filter_chain:
        name: chain_default
        filters:
          - name: envoy.filters.network.tcp_proxy
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
              stat_prefix: tcp_default
              cluster: c_default
  clusters:
    - name: c_dstport
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_dstport
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
    - name: c_srcprefix
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_srcprefix
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
    - name: c_other
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_other
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
    - name: c_default
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_default
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`

// subjectTmpl is the PRIMARY subject envoy-go bootstrap. Two listeners
// (l_test_a, l_test_b) on 127.0.0.1:OS-picked carry the same 3 chains +
// default. STATIC clusters at 127.0.0.1 per ADR-0010. Same 0.0.0.0/0
// source_prefix as the reference for cross-side equivalence.
//
// 11 sprintf placeholders (see SubjectConfig for the list).
const subjectTmpl = `node: { id: envoy-go-subject-0008, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_test_a
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - name: chain_dstport_alpha
          filter_chain_match:
            destination_port: %d
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_dstport
                cluster: c_dstport
        - name: chain_srcprefix_loopback
          filter_chain_match:
            source_prefix_ranges:
              - { address_prefix: 0.0.0.0, prefix_len: 0 }
            source_ports: [%d]
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_srcprefix
                cluster: c_srcprefix
        - name: chain_other
          filter_chain_match: {}
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_other
                cluster: c_other
      default_filter_chain:
        name: chain_default
        filters:
          - name: envoy.filters.network.tcp_proxy
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
              stat_prefix: tcp_default
              cluster: c_default
    - name: l_test_b
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - name: chain_dstport_alpha
          filter_chain_match:
            destination_port: %d
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_dstport
                cluster: c_dstport
        - name: chain_srcprefix_loopback
          filter_chain_match:
            source_prefix_ranges:
              - { address_prefix: 0.0.0.0, prefix_len: 0 }
            source_ports: [%d]
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_srcprefix
                cluster: c_srcprefix
        - name: chain_other
          filter_chain_match: {}
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_other
                cluster: c_other
      default_filter_chain:
        name: chain_default
        filters:
          - name: envoy.filters.network.tcp_proxy
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
              stat_prefix: tcp_default
              cluster: c_default
  clusters:
    - name: c_dstport
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_dstport
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
    - name: c_srcprefix
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_srcprefix
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
    - name: c_other
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_other
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
    - name: c_default
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_default
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`

// alternateRefTmpl is the c4 reference bootstrap. Only l_test_b is
// configured; chain_other is REMOVED so connection 4 falls through to
// default_filter_chain (the §11.1 demonstration).
//
// 5 sprintf placeholders (see AlternateReferenceBootstrap):
//
//	%[1]d = chain_dstport_alpha.destination_port (= refPortLA = 15008, sentinel)
//	%[2]d = chain_srcprefix_loopback.source_ports[0] (known_driver_port)
//	%[3]d = c_dstport endpoint port (backendPorts[0])
//	%[4]d = c_srcprefix endpoint port (backendPorts[1])
//	%[5]d = c_default endpoint port (backendPorts[3])
const alternateRefTmpl = `admin:
  address:
    socket_address: { address: 0.0.0.0, port_value: 9901 }
static_resources:
  listeners:
    - name: l_test_b
      address:
        socket_address: { address: 0.0.0.0, port_value: 15010 }
      filter_chains:
        - name: chain_dstport_alpha
          filter_chain_match:
            destination_port: %d
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_dstport
                cluster: c_dstport
        - name: chain_srcprefix_loopback
          filter_chain_match:
            source_prefix_ranges:
              - { address_prefix: 0.0.0.0, prefix_len: 0 }
            source_ports: [%d]
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_srcprefix
                cluster: c_srcprefix
      default_filter_chain:
        name: chain_default
        filters:
          - name: envoy.filters.network.tcp_proxy
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
              stat_prefix: tcp_default
              cluster: c_default
  clusters:
    - name: c_dstport
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_dstport
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
    - name: c_srcprefix
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_srcprefix
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
    - name: c_default
      type: STRICT_DNS
      connect_timeout: 1s
      dns_lookup_family: V4_ONLY
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_default
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: host.docker.internal, port_value: %d } } }
`

// alternateSubjectTmpl is the c4 subject envoy-go bootstrap. Only
// l_test_b is configured; chain_other is removed (same as
// alternateRefTmpl).
//
// 7 sprintf placeholders (see AlternateSubjectConfig).
const alternateSubjectTmpl = `node: { id: envoy-go-subject-0008-c4, cluster: envoy-go-differential }
admin:
  address:
    socket_address: { address: 127.0.0.1, port_value: %d }
static_resources:
  listeners:
    - name: l_test_b
      address:
        socket_address: { address: 127.0.0.1, port_value: %d }
      filter_chains:
        - name: chain_dstport_alpha
          filter_chain_match:
            destination_port: %d
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_dstport
                cluster: c_dstport
        - name: chain_srcprefix_loopback
          filter_chain_match:
            source_prefix_ranges:
              - { address_prefix: 0.0.0.0, prefix_len: 0 }
            source_ports: [%d]
          filters:
            - name: envoy.filters.network.tcp_proxy
              typed_config:
                "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
                stat_prefix: tcp_srcprefix
                cluster: c_srcprefix
      default_filter_chain:
        name: chain_default
        filters:
          - name: envoy.filters.network.tcp_proxy
            typed_config:
              "@type": type.googleapis.com/envoy.extensions.filters.network.tcp_proxy.v3.TcpProxy
              stat_prefix: tcp_default
              cluster: c_default
  clusters:
    - name: c_dstport
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_dstport
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
    - name: c_srcprefix
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_srcprefix
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
    - name: c_default
      type: STATIC
      connect_timeout: 1s
      lb_policy: ROUND_ROBIN
      load_assignment:
        cluster_name: c_default
        endpoints:
          - lb_endpoints:
              - endpoint: { address: { socket_address: { address: 127.0.0.1, port_value: %d } } }
`
