package h2

import (
	"io"
)

// clientPrefaceBytes is the 24-byte HTTP/2 connection preface (RFC 9113 §3.4).
// Every h2 connection (cleartext OR after TLS+ALPN) MUST begin with these
// bytes; mismatch is a connection-level PROTOCOL_ERROR.
var clientPrefaceBytes = []byte("PRI * HTTP/2.0\r\n\r\nSM\r\n\r\n")

// readClientPreface reads exactly len(clientPrefaceBytes) bytes from r and
// compares them against the canonical preface. Returns nil on match;
// returns *Error{Code: PROTOCOL_ERROR} on truncation or mismatch.
func readClientPreface(r io.Reader) error {
	buf := make([]byte, len(clientPrefaceBytes))
	if _, err := io.ReadFull(r, buf); err != nil {
		return &Error{
			Code:       ErrProtocolError,
			Msg:        "short preface",
			Underlying: err,
		}
	}
	for i, b := range clientPrefaceBytes {
		if buf[i] != b {
			return &Error{
				Code: ErrProtocolError,
				Msg:  "bad preface bytes",
			}
		}
	}
	return nil
}
