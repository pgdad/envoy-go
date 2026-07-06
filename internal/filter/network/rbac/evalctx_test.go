package rbac

import (
	"net"
	"testing"

	"github.com/pgdad/envoy-go/internal/filter/network"
)

// fakeConn implements network.Connection with scriptable L4 facts. The L4
// EvalContext consumes the read-side accessors (LocalAddr / RemoteAddr /
// RequestedServerName / DownstreamPrincipals); the Close arm records the
// requested close so the Task-10 OnData decision tests can assert the
// enforced-deny NoFlush close.
type fakeConn struct {
	local, remote net.Addr
	sni           string
	principals    []string

	closed    bool              // set true by Close
	closeType network.CloseType // the CloseType passed to Close
}

func (c *fakeConn) Write([]byte, bool) {}
func (c *fakeConn) Close(ct network.CloseType) {
	c.closed = true
	c.closeType = ct
}
func (c *fakeConn) LocalAddr() net.Addr            { return c.local }
func (c *fakeConn) RemoteAddr() net.Addr           { return c.remote }
func (c *fakeConn) RequestedServerName() string    { return c.sni }
func (c *fakeConn) DownstreamPrincipals() []string { return c.principals }

// Statically assert fakeConn satisfies the full network.Connection surface so
// a future accessor addition fails the test build rather than silently drifting.
var _ network.Connection = (*fakeConn)(nil)

func TestL4EvalContext_MapsConnectionFacts(t *testing.T) {
	conn := &fakeConn{
		local:      &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 8443},
		remote:     &net.TCPAddr{IP: net.ParseIP("192.168.1.5"), Port: 51000},
		sni:        "svc.internal",
		principals: []string{"spiffe://td/a"},
	}
	ec := newL4EvalContext(conn)

	if got := ec.DestinationIP(); !got.Equal(net.ParseIP("10.0.0.1")) {
		t.Errorf("DestinationIP = %v, want 10.0.0.1", got)
	}
	if ec.DestinationPort() != 8443 {
		t.Errorf("DestinationPort = %d, want 8443", ec.DestinationPort())
	}
	if !ec.DirectRemoteIP().Equal(net.ParseIP("192.168.1.5")) {
		t.Errorf("DirectRemoteIP = %v, want 192.168.1.5", ec.DirectRemoteIP())
	}
	// RemoteIP == DirectRemoteIP at L4 (no XFF resolver on a raw connection).
	if !ec.RemoteIP().Equal(net.ParseIP("192.168.1.5")) {
		t.Errorf("RemoteIP = %v, want 192.168.1.5", ec.RemoteIP())
	}
	if ec.RequestedServerName() != "svc.internal" {
		t.Errorf("RequestedServerName = %q, want %q", ec.RequestedServerName(), "svc.internal")
	}
	if got := ec.DownstreamPrincipal(); len(got) != 1 || got[0] != "spiffe://td/a" {
		t.Errorf("DownstreamPrincipal = %v, want [spiffe://td/a]", got)
	}

	// HTTP-only accessors are present-but-empty (unreachable under ProfileL4;
	// the HTTP-only Permission/Principal arms PARSE-REJECT at compile).
	if v, present := ec.Header("x"); present || v != "" {
		t.Errorf("Header = (%q, %v), want (\"\", false) at L4", v, present)
	}
	if ec.URLPath() != "" {
		t.Errorf("URLPath = %q, want \"\" at L4", ec.URLPath())
	}
	if ec.Method() != "" {
		t.Errorf("Method = %q, want \"\" at L4", ec.Method())
	}
	// Forward-compat placeholders: nil at L4 MVP (always-FALSE evaluators).
	if ec.SourcedMetadata() != nil {
		t.Errorf("SourcedMetadata = %v, want nil", ec.SourcedMetadata())
	}
	if ec.FilterState() != nil {
		t.Errorf("FilterState = %v, want nil", ec.FilterState())
	}
}

// customAddr is a minimal net.Addr implementation whose String() returns an
// arbitrary host:port string, exercising addrParts's default (SplitHostPort)
// fallback branch (not *net.TCPAddr and not *net.UDPAddr).
type customAddr struct{ s string }

func (a customAddr) Network() string { return "custom" }
func (a customAddr) String() string  { return a.s }

func TestAddrParts_SplitHostPortFallback(t *testing.T) {
	// Happy path: well-formed "host:port" string → parsed IP + port.
	conn := &fakeConn{
		local:  customAddr{"1.2.3.4:9000"},
		remote: customAddr{"5.6.7.8:1234"},
	}
	ec := newL4EvalContext(conn)

	if got := ec.DestinationIP(); !got.Equal(net.ParseIP("1.2.3.4")) {
		t.Errorf("DestinationIP(custom) = %v, want 1.2.3.4", got)
	}
	if ec.DestinationPort() != 9000 {
		t.Errorf("DestinationPort(custom) = %d, want 9000", ec.DestinationPort())
	}
	if !ec.DirectRemoteIP().Equal(net.ParseIP("5.6.7.8")) {
		t.Errorf("DirectRemoteIP(custom) = %v, want 5.6.7.8", ec.DirectRemoteIP())
	}

	// Partial-failure: ParseIP fails (non-numeric host) → nil IP, but port
	// still parses successfully — addrParts returns (nil, port).
	badHost := &fakeConn{local: customAddr{"badhost:9000"}}
	ecBad := newL4EvalContext(badHost)
	if ip := ecBad.DestinationIP(); ip != nil {
		t.Errorf("DestinationIP(badhost) = %v, want nil", ip)
	}
	if ecBad.DestinationPort() != 9000 {
		t.Errorf("DestinationPort(badhost) = %d, want 9000", ecBad.DestinationPort())
	}

	// Partial-failure: port non-numeric → (parsed IP, 0).
	badPort := &fakeConn{local: customAddr{"1.2.3.4:notaport"}}
	ecBadPort := newL4EvalContext(badPort)
	if got := ecBadPort.DestinationIP(); !got.Equal(net.ParseIP("1.2.3.4")) {
		t.Errorf("DestinationIP(badport) = %v, want 1.2.3.4", got)
	}
	if ecBadPort.DestinationPort() != 0 {
		t.Errorf("DestinationPort(badport) = %d, want 0", ecBadPort.DestinationPort())
	}

	// Total failure: SplitHostPort fails (no colon) → (nil, 0).
	noColon := &fakeConn{local: customAddr{"nocolon"}}
	ecNoColon := newL4EvalContext(noColon)
	if ecNoColon.DestinationIP() != nil {
		t.Errorf("DestinationIP(nocolon) = %v, want nil", ecNoColon.DestinationIP())
	}
	if ecNoColon.DestinationPort() != 0 {
		t.Errorf("DestinationPort(nocolon) = %d, want 0", ecNoColon.DestinationPort())
	}
}

func TestL4EvalContext_NilAndNonTCPAddrs(t *testing.T) {
	// Nil addrs → nil IP / 0 port (no panic).
	ec := newL4EvalContext(&fakeConn{})
	if ec.DestinationIP() != nil {
		t.Errorf("DestinationIP(nil addr) = %v, want nil", ec.DestinationIP())
	}
	if ec.DestinationPort() != 0 {
		t.Errorf("DestinationPort(nil addr) = %d, want 0", ec.DestinationPort())
	}
	if ec.DirectRemoteIP() != nil {
		t.Errorf("DirectRemoteIP(nil addr) = %v, want nil", ec.DirectRemoteIP())
	}

	// *net.UDPAddr exercises the UDP fast-path (struct-field read, not SplitHostPort).
	udp := &fakeConn{
		local:  &net.UDPAddr{IP: net.ParseIP("172.16.0.9"), Port: 4242},
		remote: &net.UDPAddr{IP: net.ParseIP("172.16.0.10"), Port: 4243},
	}
	ecu := newL4EvalContext(udp)
	if !ecu.DestinationIP().Equal(net.ParseIP("172.16.0.9")) {
		t.Errorf("DestinationIP(UDP) = %v, want 172.16.0.9", ecu.DestinationIP())
	}
	if ecu.DestinationPort() != 4242 {
		t.Errorf("DestinationPort(UDP) = %d, want 4242", ecu.DestinationPort())
	}
	if !ecu.DirectRemoteIP().Equal(net.ParseIP("172.16.0.10")) {
		t.Errorf("DirectRemoteIP(UDP) = %v, want 172.16.0.10", ecu.DirectRemoteIP())
	}
}
