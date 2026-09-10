package listener

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"

	quic "github.com/quic-go/quic-go"
	http3 "github.com/quic-go/quic-go/http3"

	"github.com/pgdad/envoy-go/internal/filter/network"
	"github.com/pgdad/envoy-go/internal/listener/listenerfilter"
	"github.com/pgdad/envoy-go/internal/stats"
)

// startQUIC binds the listener's UDP socket, stands a quic-go listener over it
// (with the *stdtls.Config quicTLSConfig resolves, ALPN h3), registers the
// reused per-listener metrics on the resolved address, and launches the accept
// loop. Phase 61.1 built the handshake substrate; phase 61.2
// (serveQUICConnection) serves H3 requests on each accepted connection.
//
// Phase 97: "the single chain's" is no longer accurate and the wording is
// deliberately corrected here rather than left. A QUIC listener may carry any
// number of filter_chains[] plus a default_filter_chain, and which one supplies
// the config is decided by quicTLSConfig's documented resolution order, not by
// a one-chain precondition. Nothing enforced that precondition when it was
// written.
func (rt *listenerRuntime) startQUIC(ctx context.Context, reg *stats.Registry) error {
	udpAddr, err := net.ResolveUDPAddr("udp", rt.addr)
	if err != nil {
		return err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	// Phase 97: capture the RESOLVED address immediately after the bind, i.e.
	// before quicTLSConfig() runs. quicTLSConfig() performs the Start-time
	// chain selection, whose destination_port / prefix_ranges inputs are
	// derived from rt.addr — pre-bind that string may still read ":0"
	// (OS-pick), which would make every port-bearing filter_chain_match
	// evaluate against port 0 instead of the port the socket actually holds.
	//
	// BEHAVIOR CHANGE (deliberate): on this function's two failure paths
	// (nil tlsCfg below, and a quic.Listen error) rt.addr now holds the
	// resolved address where it previously held the configured one, so
	// Manager.Start's "listener: %q: bind %s: %w" text names the resolved
	// port for a port_value: 0 listener. The socket IS bound on both paths,
	// so the resolved address is the more accurate one.
	rt.addr = udpConn.LocalAddr().String() // resolved (port 0 → OS pick)
	tlsCfg := rt.quicTLSConfig()
	if tlsCfg == nil {
		_ = udpConn.Close()
		return errors.New("quic listener has no TLS config (mandatory TLS not built)")
	}
	ql, err := quic.Listen(udpConn, tlsCfg, &quic.Config{})
	if err != nil {
		_ = udpConn.Close()
		return err
	}
	rt.udpConn = udpConn
	rt.quicCloser = ql
	registerListenerMetrics(reg, rt)
	// Capture ql into a local (already a local here) and pass it as a parameter
	// to keep the accept goroutine off rt.quicCloser, which Stop nil-writes —
	// mirrors the TCP acceptLoop's ln-capture discipline.
	go rt.quicAcceptLoop(ctx, ql)
	return nil
}

// quicTLSConfig returns the *stdtls.Config handed to quic.Listen.
//
// 🔴 IT KEEPS ITS NULLARY SIGNATURE AND IS CONNECTION-INDEPENDENT ON PURPOSE.
// quic.Listen demands one config before any connection exists, so a repair
// that made this depend on per-connection state would be unimplementable at
// startQUIC's call site. Connection-independence is also what keeps the TWO
// call moments — startQUIC, and the &http3.Server{...} literal in
// serveQUICConnection — returning the same pointer; if they could diverge,
// a connection would be TERMINATED with one chain's TLS config and SERVED by
// another chain's filters, which is the cross-wiring this phase closes.
//
// Resolution order:
//
//  1. The Start-time chain selection (selectQUICChain(nil)) — the same
//     filter_chain_match algorithm serveQUICConnection runs, evaluated on the
//     facts that exist before a handshake: the bound destination address plus
//     the stamped transport_protocol/ALPN constants.
//  2. Failing that, the first TLS-bearing chain in rt.chainSpecs SLICE ORDER.
//  3. Failing that, the default slot.
//
// ⚠️ THAT ORDER IS NOT ARBITRARY. rt.chainSpecs is in the config's
// filter_chains[] order, so step 2 is deterministic and reproduces the
// operator's own ordering. The default slot is consulted LAST because it is
// the last-resort slot: default_filter_chain must never pre-empt an indexed
// chain. The previous shape did the opposite — it tried the default chain
// FIRST and then ranged over rt.chainByName, a MAP that CONTAINS the default
// slot, so it walked the union of indexed and default chains in
// nondeterministic order and could return the default chain by map-order
// accident while reading as if it were choosing an indexed one. The iteration
// is removed rather than documented; documenting a nondeterministic loop does
// not make it deterministic.
func (rt *listenerRuntime) quicTLSConfig() *stdtls.Config {
	if ci := rt.selectQUICChain(nil); ci != nil && ci.tlsCfg != nil {
		return ci.tlsCfg
	}
	for _, spec := range rt.chainSpecs {
		if ci := rt.chainByName[spec.Name]; ci != nil && ci.tlsCfg != nil {
			return ci.tlsCfg
		}
	}
	if rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil {
		return rt.defaultChain.tlsCfg
	}
	return nil
}

// quicTransportProtocol / quicApplicationProtocol are the two constants every
// QUIC chain-match input carries. They are stamped UNCONDITIONALLY (see
// quicChainMatchInputs) because `matches` treats the CHAIN's empty string as
// "unspecified" but has NO REVERSE WILDCARD: a chain spelling
// `transport_protocol: quic` against an UNSET input is INELIGIBLE, so leaving
// the input blank would silently disable the dimension.
const (
	quicTransportProtocol   = "quic"
	quicApplicationProtocol = "h3"
)

// quicChainMatchInputs builds the filter_chain_match inputs for a QUIC
// connection, or for the Start-time moment when conn is nil and no connection
// exists yet (quic.Listen demands a *stdtls.Config before the first
// handshake).
//
// 🔴 The UDP address extraction below deliberately does NOT reuse manager.go's
// localIP / localPort / remoteIP / remotePort helpers. Those comma-ok assert
// *net.TCPAddr and return nil / 0 for a UDP address SILENTLY — reusing them
// would compile, run, and make every destination_port dimension evaluate
// against port 0, matching only a chain that literally names port 0.
func (rt *listenerRuntime) quicChainMatchInputs(conn *quic.Conn) listenerfilter.ChainMatchInputs {
	inputs := listenerfilter.ChainMatchInputs{
		TransportProtocol:    quicTransportProtocol,
		ApplicationProtocols: []string{quicApplicationProtocol},
	}
	if conn == nil {
		// Start-time: the destination is the listener's own bound address.
		// startQUIC captures the RESOLVED address before calling
		// quicTLSConfig(), so a port_value: 0 listener presents the port the
		// socket actually holds; on a never-Started runtime rt.addr is still
		// the configured string, which ResolveUDPAddr parses just as well.
		// Source and ServerName stay unset — neither exists yet.
		if ua, err := net.ResolveUDPAddr("udp", rt.addr); err == nil && ua != nil {
			inputs.DestinationIP = ua.IP
			inputs.DestinationPort = uint32(ua.Port)
		}
		return inputs
	}
	if la, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		inputs.DestinationIP = la.IP
		inputs.DestinationPort = uint32(la.Port)
	}
	if ra, ok := conn.RemoteAddr().(*net.UDPAddr); ok {
		// source_type is DERIVED from SourceIP by IsLoopbackSource(), not
		// stored — filling SourceIP activates that dimension too.
		inputs.SourceIP = ra.IP
		inputs.SourcePort = uint32(ra.Port)
	}
	// ConnectionState() returns a VALUE (mutex-guarded); .TLS is the standard
	// library's crypto/tls.ConnectionState, whose ServerName is the SNI the
	// client actually sent in its ClientHello.
	inputs.ServerName = conn.ConnectionState().TLS.ServerName
	return inputs
}

// selectQUICChain runs the SAME 8-dimension filter_chain_match algorithm the
// TCP path runs in serveConnection: listenerfilter.SelectChain over
// rt.chainSpecs with rt.defaultSpec as the LAST-RESORT slot, then the
// spec -> info mapping through rt.chainByName. SelectChain returns a
// *ChainSpec, never a *chainInfo, so the map lookup is mandatory.
//
// Returns nil when no chain is selectable (SelectChain's
// (nil, ErrNoChainMatched) branch: no indexed chain eligible AND no default
// slot). A nil return is what makes serveQUICConnection close the connection,
// mirroring the reference's "no filter chain found".
func (rt *listenerRuntime) selectQUICChain(conn *quic.Conn) *chainInfo {
	spec, err := listenerfilter.SelectChain(rt.quicChainMatchInputs(conn), rt.chainSpecs, rt.defaultSpec)
	if err != nil {
		// Mirrors serveConnection's `listener %q: chain-match: %v`. A silent
		// close is indistinguishable from a healthy one in a differential run.
		// Suppressed for the Start-time (conn == nil) call only: quicTLSConfig
		// treats a nil Start-time selection as a normal fall-through to its
		// indexed/default fallbacks, and that call also runs per connection
		// (the &http3.Server{} literal below), so logging it there would emit
		// a line per connection on a perfectly healthy listener.
		if conn != nil {
			log.Printf("listener %q: chain-match: %v", rt.name, err)
		}
		return nil
	}
	return rt.chainByName[spec.Name]
}

// quicChain returns the *chainInfo that must serve conn. Phase 97 replaced the
// 61.2 single-chain accessor (defaultChain-first, then an arbitrary
// chainByName map entry) with the real chain-match algorithm: the old shape
// evaluated NO dimension of any filter_chain_match and let the LAST-RESORT
// default slot pre-empt an eligible indexed chain.
//
// conn == nil is the Start-time moment (see quicChainMatchInputs).
func (rt *listenerRuntime) quicChain(conn *quic.Conn) *chainInfo {
	return rt.selectQUICChain(conn)
}

// quicAcceptLoop accepts QUIC connections whose handshake has already completed
// (quic-go's Accept returns post-handshake). It mirrors acceptLoop's cx-metric
// discipline. serveQUICConnection (phase 61.2) counts the conn and serves H3
// requests into the chain terminal.
func (rt *listenerRuntime) quicAcceptLoop(ctx context.Context, ql *quic.Listener) {
	for {
		conn, err := ql.Accept(ctx)
		if err != nil {
			// Listener closed (Stop) or ctx canceled — the normal shutdown path.
			// quic-go returns ErrServerClosed after Listener.Close; Stop closes
			// the listener without necessarily canceling ctx, so match the
			// sentinel in addition to the ctx.Err() guard.
			if errors.Is(err, quic.ErrServerClosed) || ctx.Err() != nil {
				return
			}
			log.Printf("listener %q: quic accept: %v", rt.name, err)
			return
		}
		rt.downstreamCxTotal.Inc()
		rt.downstreamCxActive.Inc()
		go rt.serveQUICConnection(ctx, conn)
	}
}

// serveQUICConnection serves HTTP/3 requests on an accepted QUIC connection.
// The QUIC/TLS-1.3 handshake is complete (Accept returned). quic-go's
// http3.Server decodes H3 frames + QPACK and invokes the chain terminal
// filter's ServeH3 per request (via the network.H3Terminal seam), dispatching
// each request into the shared HCM -> router -> filter-chain path. Phase 61.2
// (ADR-0281) replaces the 61.1 handshake-only close.
//
// Honors ctx: a canceled ctx closes the connection so the blocking
// ServeQUICConn call below unblocks and Stop completes (M6-2 pickup). The
// watcher goroutine below always exits on EITHER path — ctx cancellation or
// ServeQUICConn returning on its own (the common case, e.g. the client closes
// the connection) — via the done channel, so it never leaks.
func (rt *listenerRuntime) serveQUICConnection(ctx context.Context, conn *quic.Conn) {
	defer rt.downstreamCxActive.Dec()

	ci := rt.quicChain(conn)
	if ci == nil {
		_ = conn.CloseWithError(0, "")
		return
	}
	filters := ci.netChainFactory()
	if len(filters) == 0 {
		_ = conn.CloseWithError(0, "")
		return
	}
	term, ok := filters[len(filters)-1].(network.H3Terminal)
	if !ok {
		// The chain terminal does not serve H3 (e.g. tcp_proxy on a QUIC
		// listener — out of the minimal slice). Close cleanly.
		log.Printf("listener %q: quic: chain terminal is not H3-capable (%T)", rt.name, filters[len(filters)-1])
		_ = conn.CloseWithError(0, "")
		return
	}

	srv := &http3.Server{
		Handler:    http.HandlerFunc(term.ServeH3),
		TLSConfig:  rt.quicTLSConfig(),
		QUICConfig: &quic.Config{},
	}

	// done is closed once ServeQUICConn returns, regardless of why. The
	// watcher goroutine below selects on ctx.Done() and done together so it
	// always terminates: if ctx is canceled first it closes the connection
	// (unblocking ServeQUICConn below); if ServeQUICConn returns first
	// (normal close, no ctx cancellation), done fires and the watcher exits
	// without ever having to observe ctx cancellation.
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.CloseWithError(0, "")
		case <-done:
		}
	}()

	err := srv.ServeQUICConn(conn)
	close(done)
	if err != nil && ctx.Err() == nil {
		log.Printf("listener %q: quic: serve: %v", rt.name, err)
	}
}
