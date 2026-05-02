package listenerfilter

import (
	"bufio"
	"net"
)

// peekerBufSize is the default peek buffer size — matches Envoy's
// tls_inspector.initial_read_buffer_size default (Decision C, ADR-0079).
// The tls_inspector proto config can override per-listener; the override
// value is clamped to [256, 65536] (Decision C).
const peekerBufSize = 4096

// peekerConn wraps a net.Conn with a bufio.Reader so listener filters can
// Peek bytes without consuming them. Subsequent Read calls drain the same
// buffer first, then transition to the underlying conn once the buffer is
// exhausted. This is the same trick crypto/tls.Conn.Handshake() uses
// internally for resumption-cookie peek-back. See SPEC §5.3.
type peekerConn struct {
	net.Conn
	br *bufio.Reader
}

// NewPeekerConn allocates a peekerConn over c with a default-size peek
// buffer. The returned value satisfies both Peeker (via Peek) and net.Conn
// (via the embedded conn + the Read override).
func NewPeekerConn(c net.Conn) net.Conn {
	return &peekerConn{Conn: c, br: bufio.NewReaderSize(c, peekerBufSize)}
}

// NewPeekerConnSize is the size-configurable variant. size is clamped to
// [256, 65536] per ADR-0079 Decision C; values outside the range are
// silently clamped (the proto-config parser at tls_inspector/proto.go
// errors at parse time on out-of-range values).
func NewPeekerConnSize(c net.Conn, size int) net.Conn {
	if size < 256 {
		size = 256
	}
	if size > 65536 {
		size = 65536
	}
	return &peekerConn{Conn: c, br: bufio.NewReaderSize(c, size)}
}

// Peek implements Peeker. Returns up to n bytes without advancing the read
// position. Returns bufio.ErrBufferFull if n > the buffer's capacity.
func (p *peekerConn) Peek(n int) ([]byte, error) {
	return p.br.Peek(n)
}

// Read drains from the buffered reader first; once the buffer is exhausted
// transitions to the underlying conn. This is the discipline that makes
// the post-listener-filter dispatch path see the same bytes the inspector
// peeked.
func (p *peekerConn) Read(b []byte) (int, error) {
	return p.br.Read(b)
}

// AsPeeker returns the Peeker view of conn if conn was constructed via
// NewPeekerConn; returns nil otherwise. Used by listener filters that need
// to access the peek buffer.
func AsPeeker(conn net.Conn) Peeker {
	if pc, ok := conn.(*peekerConn); ok {
		return pc
	}
	return nil
}
