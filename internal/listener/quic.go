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
	"github.com/pgdad/envoy-go/internal/stats"
)

// startQUIC binds the listener's UDP socket, stands a quic-go listener over it
// (with the single chain's *stdtls.Config, ALPN h3), registers the reused
// per-listener metrics on the resolved address, and launches the accept loop.
// Phase 61.1 built the handshake substrate; phase 61.2 (serveQUICConnection)
// serves H3 requests on each accepted connection.
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

// quicChain returns the single chain's *chainInfo for the minimal single-chain
// QUIC slice (defaultChain first, then the first chain in chainByName). Map
// iteration order over chainByName is nondeterministic in Go — that is
// harmless here because the minimal QUIC slice supports exactly one chain,
// but this must NOT be read as an implicit ordering guarantee. SNI-dispatched
// multi-chain QUIC is deferred (M-FB1/M-FB2). Phase 61.2.
func (rt *listenerRuntime) quicChain() *chainInfo {
	if rt.defaultChain != nil {
		return rt.defaultChain
	}
	for _, ci := range rt.chainByName {
		return ci
	}
	return nil
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

	ci := rt.quicChain()
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
