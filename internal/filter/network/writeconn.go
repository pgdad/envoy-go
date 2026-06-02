// internal/filter/network/writeconn.go — write-chain conn for terminal handoff
// (ADR-0221). Mirrors prefixconn.go's embed-and-override-one-method shape.

package network

import "net"

// writeChainConn runs the write-filter chain over every terminal-originated
// downstream write BEFORE forwarding to the wrapped conn. Mirrors upstream
// ConnectionImpl::write → filter_manager_.onWrite() → transport (AMEND-A11).
// All non-Write methods promote from the embedded net.Conn (so Read still
// reaches a wrapped prefixConn's buffered-prefix replay).
type writeChainConn struct {
	net.Conn               // the wrapped conn (prefixConn or the raw downstream conn)
	filters  []WriteFilter // DISPATCH order (already reversed by handleTerminal)
}

func newWriteChainConn(c net.Conn, dispatch []WriteFilter) *writeChainConn {
	return &writeChainConn{Conn: c, filters: dispatch}
}

// Write runs the write chain (dispatch order) over p, then forwards the
// POST-CHAIN buffer to the wrapped conn (upstream parity: write filters may
// mutate; the transport sees the filtered buffer — at 28.x no production filter
// mutates, so the bytes are identical). Return semantics (D-P7):
//   - chain-stopped (StopIteration): (len(p), nil) — the terminal cannot
//     distinguish a stopped write from a delivered one, exactly as upstream
//     where ConnectionImpl::write returns void. A dropped write surfaces as
//     downstream silence.
//   - underlying-write error: (0, err) — the terminal's existing downstream
//     write-error handling sees it exactly as it would from the raw conn.
//     (Byte-count fidelity under mutation is moot at 28.x — no filter mutates.)
//   - success: (len(p), nil).
func (w *writeChainConn) Write(p []byte) (int, error) {
	buf := &Buffer{}
	buf.Append(p)
	for _, f := range w.filters {
		if f.OnWrite(buf, false) == StopIteration {
			// Upstream parity: the write does not proceed; nothing reaches the
			// transport. The terminal cannot distinguish (D-P7): report success.
			return len(p), nil
		}
	}
	if _, err := w.Conn.Write(buf.Bytes()); err != nil {
		return 0, err
	}
	return len(p), nil
}
