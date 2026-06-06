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
		parked := r.rt.replayRead(b[:n], false)
		if parked {
			// 29.3 post-handoff withhold (SPEC §3.1 extension 3): block the pump until
			// the async ContinueReading releases, so the delayed request's bytes are
			// withheld from the upstream pump until the fault delay elapses. Behind the
			// haltable gate inside replayRead — byte-identical when no hold (R1).
			r.rt.holdUntilResume()
		}
	}
	if err != nil && errors.Is(err, io.EOF) {
		r.rt.replayRead(nil, true)
	}
	return n, err
}

// SetCloseDirection lets the terminal (tcp_proxy) record which pump EOF'd first
// onto the chain runtime (29.3, §3.5). readChainConn is the INNERMOST wrap (it
// holds rt); the prefixConn/writeChainConn forwarding methods relay to it.
func (r *readChainConn) SetCloseDirection(d CloseDirection) { r.rt.setCloseDirection(d) }
