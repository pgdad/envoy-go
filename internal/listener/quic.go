package listener

import (
	"context"
	stdtls "crypto/tls"
	"errors"
	"log"
	"net"

	quic "github.com/quic-go/quic-go"

	"github.com/pgdad/envoy-go/internal/stats"
)

// startQUIC binds the listener's UDP socket, stands a quic-go listener over it
// (with the single chain's *stdtls.Config, ALPN h3), registers the reused
// per-listener metrics on the resolved address, and launches the accept loop.
// Phase 61.1: handshake substrate only — no HTTP is served.
func (rt *listenerRuntime) startQUIC(ctx context.Context, reg *stats.Registry) error {
	udpAddr, err := net.ResolveUDPAddr("udp", rt.addr)
	if err != nil {
		return err
	}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
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
	rt.addr = udpConn.LocalAddr().String() // resolved (port 0 → OS pick)
	registerListenerMetrics(reg, rt)
	// Capture ql into a local (already a local here) and pass it as a parameter
	// to keep the accept goroutine off rt.quicCloser, which Stop nil-writes —
	// mirrors the TCP acceptLoop's ln-capture discipline.
	go rt.quicAcceptLoop(ctx, ql)
	return nil
}

// quicTLSConfig returns the single chain's *stdtls.Config for the minimal
// single-chain QUIC slice. SNI-dispatched multi-chain QUIC is deferred; if
// multiple chains exist, the first non-nil TLS config is used.
func (rt *listenerRuntime) quicTLSConfig() *stdtls.Config {
	if rt.defaultChain != nil && rt.defaultChain.tlsCfg != nil {
		return rt.defaultChain.tlsCfg
	}
	for _, ci := range rt.chainByName {
		if ci.tlsCfg != nil {
			return ci.tlsCfg
		}
	}
	return nil
}

// quicAcceptLoop accepts QUIC connections whose handshake has already completed
// (quic-go's Accept returns post-handshake). It mirrors acceptLoop's cx-metric
// discipline. Phase 61.1 does not serve HTTP — serveQUICConnection closes the
// connection after counting it.
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

// serveQUICConnection is the phase-61.1 handshake-only terminal: the QUIC/TLS
// handshake is complete (Accept returned), so the leg's capability is proven.
// Leg 61.2 decodes an H3 request here and dispatches it into the HCM/router
// chain. For now, count the conn (deferred Dec) and close cleanly.
func (rt *listenerRuntime) serveQUICConnection(ctx context.Context, conn *quic.Conn) {
	defer rt.downstreamCxActive.Dec()
	_ = ctx
	_ = conn.CloseWithError(0, "")
}
