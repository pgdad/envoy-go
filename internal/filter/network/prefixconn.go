// internal/filter/network/prefixconn.go — buffered-prefix conn for terminal handoff

package network

import "net"

// prefixConn replays an undrained buffered prefix to a terminal filter BEFORE
// delegating to the live downstream conn (the bytes a preceding read filter
// inspected but did not consume). All non-Read methods promote from the
// embedded net.Conn. For an empty prefix it is a transparent passthrough →
// byte-identical to handing the raw conn to Handle (the pure-terminal case).
type prefixConn struct {
	net.Conn
	prefix []byte
}

func newPrefixConn(c net.Conn, prefix []byte) *prefixConn {
	return &prefixConn{Conn: c, prefix: prefix}
}

func (p *prefixConn) Read(b []byte) (int, error) {
	if len(p.prefix) > 0 {
		n := copy(b, p.prefix)
		p.prefix = p.prefix[n:]
		return n, nil
	}
	return p.Conn.Read(b)
}
