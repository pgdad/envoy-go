// internal/filter/network/buffer.go — the per-connection drainable read buffer.

package network

// Buffer is the per-connection drainable read buffer. The chain owns ONE
// Buffer for all read filters (connection-level buffering per SPEC §3.3); a
// filter consumes bytes by Drain after copying them out (e.g. echo writes
// Bytes() back then Drain(Len())).
type Buffer struct {
	data []byte
}

// Append copies p onto the tail of the buffer.
func (b *Buffer) Append(p []byte) { b.data = append(b.data, p...) }

// Bytes returns the current undrained bytes. The slice aliases the buffer's
// backing array; callers must copy before Drain if they need the bytes after.
func (b *Buffer) Bytes() []byte { return b.data }

// Len returns the number of undrained bytes.
func (b *Buffer) Len() int { return len(b.data) }

// Drain drops the first n bytes (clamped to Len). It re-slices in place;
// callers must copy Bytes() before Drain if they need the bytes after.
func (b *Buffer) Drain(n int) {
	if n >= len(b.data) {
		b.data = b.data[:0]
		return
	}
	b.data = b.data[n:]
}
