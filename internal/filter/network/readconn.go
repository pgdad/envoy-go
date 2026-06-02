// internal/filter/network/readconn.go — read-side seam conn for terminal
// handoff (ADR-0221 §AMEND — the read-direction half of the terminal-handoff
// conn-wrap seam). Mirrors prefixconn.go / writeconn.go's embed-and-override-
// one-method shape.

package network

import (
	"errors"
	"io"
	"net"
)

// readChainConn re-feeds every post-handoff downstream socket read through the
// read-filter chain (rt.replayRead) BEFORE returning the bytes to the terminal,
// restoring upstream FilterManagerImpl::onRead re-iteration parity for chains
// that need it (the §3.4 predicate). All non-Read methods promote from the
// embedded net.Conn.
type readChainConn struct {
	net.Conn               // the RAW downstream conn (innermost wrap — §3.1 composition)
	rt       *chainRuntime // the per-connection runtime whose read filters get the replay
}

func newReadChainConn(c net.Conn, rt *chainRuntime) *readChainConn {
	return &readChainConn{Conn: c, rt: rt}
}

// Read reads from the wrapped conn, replays any received bytes through the
// read-filter chain (observational — §3.5), then returns them to the terminal.
// Replay-before-return makes stat increments visible BEFORE the bytes are
// forwarded upstream (deterministic ordering for the 0046 scrape — §5.1).
// io.EOF additionally delivers a final endStream replay (pre-handoff read-loop
// symmetry, internal/listener/manager.go:1073-1077 (serveNetworkChain)).
func (r *readChainConn) Read(b []byte) (int, error) {
	n, err := r.Conn.Read(b)
	if n > 0 {
		r.rt.replayRead(b[:n], false)
	}
	if err != nil && errors.Is(err, io.EOF) {
		r.rt.replayRead(nil, true)
	}
	return n, err
}
