// internal/filter/network/upstreampool.go (ADR-0230)
//
// The upstream connection-pool / cluster-routing seam — the framework's SIXTH
// structural extension. It lets a network.TerminalFilter ACTIVELY manage an
// upstream connection (dial + per-downstream lifecycle + FIFO/positional reply
// correlation) rather than observe a tcp_proxy-terminated one. Redis-scoped (no
// thrift generalization; thrift reuses/extends it at its phase). It builds ON
// TerminalFilter.Handle (UNCHANGED) and reuses cluster.Cluster.Dial via a DIAL
// CLOSURE — keeping internal/filter/network free of an internal/cluster import
// (the upstreamcluster.go decoupling discipline).
//
// 32.1 MVP: one upstream conn per downstream conn (AMEND-R8); synchronous
// single-flight (a degenerate depth-1 FIFO/positional pending queue — RESP
// carries no correlation id). The shared per-host pool + a >1-depth pending queue
// + the two-goroutine pipelined model + the ADR-0223 per-conn mutex are DEFERRED
// (consume-at-second-consumer — thrift / a latency-or-fan-out sub-phase extends).

package network

import (
	"bufio"
	"context"
	"net"
)

// UpstreamDialFunc resolves + dials one upstream connection. redisproxy supplies
// a closure over the boot-resolved cluster's Cluster.Dial (Endpoint discarded) —
// keeping internal/filter/network free of an internal/cluster import.
type UpstreamDialFunc func(ctx context.Context) (net.Conn, error)

// UpstreamConn is one downstream connection's dedicated upstream connection, with
// FIFO/positional reply correlation. The MVP is one-conn-per-downstream +
// synchronous single-flight; the shared pool + deep queue + pipelined model are
// DEFERRED (see the file header). Not safe for concurrent Send (the single-flight
// pump is single-goroutine — the 32.1 contract; the ADR-0223 mutex arrives with
// the deferred pipelined model).
type UpstreamConn struct {
	dial      UpstreamDialFunc
	onRequest func() // fired per Send (the cluster.IncUpstreamRqTotal hook); nil-tolerant
	conn      net.Conn
	r         *bufio.Reader // bufio.Reader over conn; the codec decodes reply frames from it
}

// NewUpstreamConn binds a dial closure + a per-proxied-request hook (the filter
// passes cluster.IncUpstreamRqTotal — the AMEND-R6 upstream_rq_total Inc; pass
// nil for none). No dial happens until the first Send.
func NewUpstreamConn(dial UpstreamDialFunc, onRequest func()) *UpstreamConn {
	return &UpstreamConn{dial: dial, onRequest: onRequest}
}

// Send forwards one request's raw bytes upstream. On the FIRST call it lazily
// dials (that first Dial Incs cluster.upstream_cx_total/active); every call fires
// the onRequest hook (→ cluster.upstream_rq_total) then writes reqBytes.
func (u *UpstreamConn) Send(ctx context.Context, reqBytes []byte) error {
	if u.conn == nil {
		c, err := u.dial(ctx)
		if err != nil {
			return err
		}
		u.conn = c
		u.r = bufio.NewReader(c)
	}
	if u.onRequest != nil {
		u.onRequest()
	}
	_, err := u.conn.Write(reqBytes)
	return err
}

// Reader returns the buffered reader the codec decodes the reply frame from
// (stable across Sends; valid after the first Send). Positional correlation: the
// synchronous pump reads exactly one reply per Send (depth-1 FIFO).
func (u *UpstreamConn) Reader() *bufio.Reader { return u.r }

// Close closes the upstream conn (idempotent; a no-op if never dialed — a
// PING/AUTH-only downstream never dials). The Cluster.Dial connWithGauge Decs
// upstream_cx_active once. Called from the filter's Handle defer.
func (u *UpstreamConn) Close() error {
	if u.conn == nil {
		return nil
	}
	c := u.conn
	u.conn = nil
	return c.Close()
}
