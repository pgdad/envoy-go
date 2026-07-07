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

// NewBytes returns ONLY the never-before-seen trailing bytes of the cumulative
// chain buffer (the bytes beyond *consumed), then advances *consumed to
// totalAppended — the TotalAppended high-water-mark discipline shared by the
// copying-sniffer decoders (kafkabroker/mongoproxy/zookeeperproxy). Callers
// pass chain = Buffer.Bytes() (the WHOLE undrained buffer) and totalAppended =
// Buffer.TotalAppended() (the monotonic cumulative count); the new bytes are
// always the trailing (totalAppended − consumed) bytes because bytes are only
// ever appended at the tail and drained at the head, and the runtime never
// drains bytes the filters have not yet been shown (the §3.3 soundness
// invariant). The defensive clamp never fires on that invariant.
func NewBytes(chain []byte, totalAppended int64, consumed *int64) []byte {
	newCount := totalAppended - *consumed
	if newCount <= 0 {
		return nil
	}
	if newCount > int64(len(chain)) {
		newCount = int64(len(chain)) // defensive: never slice before the buffer start
	}
	out := chain[int64(len(chain))-newCount:]
	*consumed = totalAppended
	return out
}
