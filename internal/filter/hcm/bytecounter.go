package hcm

import "io"

// byteCounterWriter wraps an io.Writer to maintain a running int64 total of
// bytes written. Per SPEC §12 #3 + Decision A, short-writes account the actual
// byte count returned by the inner Write, not the request length.
type byteCounterWriter struct {
	w io.Writer
	n int64
}

func (bcw *byteCounterWriter) Write(p []byte) (int, error) {
	n, err := bcw.w.Write(p)
	bcw.n += int64(n)
	return n, err
}
