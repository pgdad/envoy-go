package network

import "testing"

func TestBufferDrainSemantics(t *testing.T) {
	b := &Buffer{}
	b.Append([]byte("hello"))
	b.Append([]byte("-world"))
	if b.Len() != 11 {
		t.Fatalf("Len=%d want 11", b.Len())
	}
	if string(b.Bytes()) != "hello-world" {
		t.Fatalf("Bytes=%q", b.Bytes())
	}
	b.Drain(6) // drop "hello-"
	if string(b.Bytes()) != "world" || b.Len() != 5 {
		t.Fatalf("after Drain(6): Bytes=%q Len=%d", b.Bytes(), b.Len())
	}
	b.Drain(b.Len())
	if b.Len() != 0 {
		t.Fatalf("after full drain: Len=%d want 0", b.Len())
	}
	b.Drain(100) // over-drain is clamped, not a panic
	if b.Len() != 0 {
		t.Fatalf("over-drain: Len=%d want 0", b.Len())
	}
}

// TotalAppended is the monotonic count of bytes ever Appended: unlike Len(),
// it is unaffected by Drain — it only grows (the 28.1b §3.3 decoder-feed
// re-base basis: filters that track novelty against TotalAppended are immune
// to WHO drains the buffer and WHEN).
func TestBufferTotalAppendedMonotonicUnderDrain(t *testing.T) {
	b := &Buffer{}
	if b.TotalAppended() != 0 {
		t.Fatalf("zero-value TotalAppended = %d, want 0", b.TotalAppended())
	}
	b.Append([]byte("hello"))
	if b.TotalAppended() != 5 {
		t.Fatalf("after Append(5): TotalAppended = %d, want 5", b.TotalAppended())
	}
	b.Drain(b.Len()) // full drain
	if b.Len() != 0 || b.TotalAppended() != 5 {
		t.Fatalf("after Drain: Len=%d (want 0), TotalAppended=%d (want 5 — Drain must NOT shrink it)", b.Len(), b.TotalAppended())
	}
	b.Append([]byte("-world"))
	if b.TotalAppended() != 11 {
		t.Fatalf("after second Append: TotalAppended = %d, want 11 (5+6, monotonic across the drain)", b.TotalAppended())
	}
	if b.Len() != 6 {
		t.Fatalf("Len = %d, want 6 (only the undrained bytes)", b.Len())
	}
	b.Append(nil) // nil/empty append is a no-op on both counters
	if b.TotalAppended() != 11 || b.Len() != 6 {
		t.Fatalf("after Append(nil): TotalAppended=%d Len=%d, want 11/6", b.TotalAppended(), b.Len())
	}
}
