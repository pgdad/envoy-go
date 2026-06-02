// internal/filter/network/buffer.go — the per-connection drainable read buffer.

package network

// Buffer is the per-connection drainable read buffer. The chain owns ONE
// Buffer for all read filters (connection-level buffering per SPEC §3.3); a
// filter consumes bytes by Drain after copying them out (e.g. echo writes
// Bytes() back then Drain(Len())).
type Buffer struct {
	data []byte
	// total is the monotonic count of bytes ever Appended to this Buffer.
	// Unlike Len(), it is unaffected by Drain — it only grows. Filters that
	// need to distinguish never-before-seen bytes from re-delivered bytes (the
	// zookeeperproxy request decoder) track novelty against TotalAppended
	// instead of Len, which makes their tracking immune to WHO drains the
	// buffer and WHEN (the filter never drains — R3; the runtime drains at
	// terminal handoff and after each post-handoff replay pass — 28.1b §3.2/§3.3).
	total int64
}

// Append copies p onto the tail of the buffer.
func (b *Buffer) Append(p []byte) {
	b.data = append(b.data, p...)
	b.total += int64(len(p))
}

// TotalAppended returns the monotonic count of bytes ever Appended (int64 —
// D-S28.1b-1: a very-long-lived connection can exceed 2^31 bytes).
func (b *Buffer) TotalAppended() int64 { return b.total }

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
