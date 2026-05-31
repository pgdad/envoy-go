// internal/filter/network/rbac/evalctx.go — the L4 rbacengine.EvalContext
// implementation built from a network.Connection (the §4.2 accessor mapping).

package rbac

import (
	"net"
	"strconv"

	"github.com/esalaine/envoy-go/internal/filter/network"
	rbacengine "github.com/esalaine/envoy-go/internal/rbac"
)

// l4EvalContext adapts a network.Connection to the shared rbacengine.EvalContext
// the Permission/Principal evaluators consume (SPEC §4.2). The L4 accessor
// mapping is direct:
//
//   - DestinationIP/Port  ← conn.LocalAddr()   (listener-bound local addr)
//   - DirectRemoteIP      ← conn.RemoteAddr()  (peer source addr, pre-XFF)
//   - RemoteIP            ← DirectRemoteIP      (no XFF at L4 — see below)
//   - RequestedServerName ← conn.RequestedServerName() (tls_inspector SNI)
//   - DownstreamPrincipal ← conn.DownstreamPrincipals() (mTLS peer-cert SANs)
//
// The HTTP-only accessors (Header / URLPath / Method) are present-but-empty:
// the L4 consumer compiles under ProfileL4, which PARSE-REJECTs the HTTP-only
// Permission/Principal arms (header / url_path) at config-build time, so no
// surviving evaluator ever calls them. They satisfy the interface; they are
// unreachable at runtime.
//
// SourcedMetadata / FilterState are forward-compat placeholders (always-FALSE
// evaluator runtimes per SPEC §2.5 + §8.10); they return nil, matching the
// HTTP consumer's *filter.
type l4EvalContext struct{ conn network.Connection }

// Compile-time assertion: *l4EvalContext implements rbacengine.EvalContext.
var _ rbacengine.EvalContext = (*l4EvalContext)(nil)

// newL4EvalContext wraps a network.Connection as an rbacengine.EvalContext.
func newL4EvalContext(c network.Connection) *l4EvalContext { return &l4EvalContext{conn: c} }

// addrParts splits a net.Addr into (IP, port). It fast-paths *net.TCPAddr and
// *net.UDPAddr (reading the struct fields directly) and falls back to
// net.SplitHostPort + net.ParseIP for any other Stringer-shaped addr. A nil
// addr, an unparseable host, or a non-numeric port yields (nil, 0) — never a
// panic. permDestIP / prinDirectRemoteIP treat a nil IP as no-match (matchCidr
// nil-IP → false), and permDestPort treats 0 as the literal port 0.
func addrParts(addr net.Addr) (net.IP, uint32) {
	switch a := addr.(type) {
	case nil:
		return nil, 0
	case *net.TCPAddr:
		if a == nil {
			return nil, 0
		}
		return a.IP, uint32(a.Port)
	case *net.UDPAddr:
		if a == nil {
			return nil, 0
		}
		return a.IP, uint32(a.Port)
	default:
		host, portStr, err := net.SplitHostPort(addr.String())
		if err != nil {
			return nil, 0
		}
		ip := net.ParseIP(host)
		port, err := strconv.ParseUint(portStr, 10, 16)
		if err != nil {
			return ip, 0
		}
		return ip, uint32(port)
	}
}

// DestinationIP returns the listener-bound (local) IP. nil for non-IP / nil addr.
func (e *l4EvalContext) DestinationIP() net.IP {
	ip, _ := addrParts(e.conn.LocalAddr())
	return ip
}

// DestinationPort returns the listener-bound (local) port. 0 when unavailable.
func (e *l4EvalContext) DestinationPort() uint32 {
	_, port := addrParts(e.conn.LocalAddr())
	return port
}

// DirectRemoteIP returns the peer connection's source IP (pre-XFF).
func (e *l4EvalContext) DirectRemoteIP() net.IP {
	ip, _ := addrParts(e.conn.RemoteAddr())
	return ip
}

// RemoteIP returns the XFF-resolved remote IP. At L4 there is no XFF layer on
// the raw connection, so it equals DirectRemoteIP (peer source IP) per SPEC
// §4.2 — the distinction with DirectRemoteIP is interface-level only here.
func (e *l4EvalContext) RemoteIP() net.IP { return e.DirectRemoteIP() }

// RequestedServerName returns the tls_inspector SNI ("" for plaintext).
func (e *l4EvalContext) RequestedServerName() string { return e.conn.RequestedServerName() }

// DownstreamPrincipal returns the mTLS peer-cert principal candidates (URI/DNS
// SANs) from the connection, or nil for non-mTLS connections.
func (e *l4EvalContext) DownstreamPrincipal() []string { return e.conn.DownstreamPrincipals() }

// Header is HTTP-only — unreachable under ProfileL4 (the header arm
// PARSE-REJECTs at compile). Present-but-empty to satisfy the interface.
func (e *l4EvalContext) Header(string) (string, bool) { return "", false }

// URLPath is HTTP-only — unreachable under ProfileL4 (the url_path arm
// PARSE-REJECTs at compile). Present-but-empty to satisfy the interface.
func (e *l4EvalContext) URLPath() string { return "" }

// Method is HTTP-only — surfaced solely for the matcher-engine bridge in the
// HTTP consumer. No L4 evaluator consumes it; present-but-empty here.
func (e *l4EvalContext) Method() string { return "" }

// SourcedMetadata is a forward-compat placeholder (always-FALSE evaluator
// runtime per SPEC §2.5 + §8.10); nil at L4 MVP.
func (e *l4EvalContext) SourcedMetadata() any { return nil }

// FilterState is a forward-compat placeholder (always-FALSE evaluator runtime
// per SPEC §2.5 + §8.10); nil at L4 MVP.
func (e *l4EvalContext) FilterState() any { return nil }
