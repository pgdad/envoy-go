package hcm

import "io"

// byteCounterWriter wraps an io.Writer to maintain a running int64 total of
// bytes written. Used by routerAction.do to capture BYTES_SENT for the
// access-log Record. Per SPEC §12 #3 + Decision A, the total reflects bytes
// written to the downstream (response body + status-line + headers in the H1
// path); short-writes account the actual byte count returned by the inner
// Write, not the request length.
type byteCounterWriter struct {
	w io.Writer
	n int64
}

func (bcw *byteCounterWriter) Write(p []byte) (int, error) {
	n, err := bcw.w.Write(p)
	bcw.n += int64(n)
	return n, err
}
