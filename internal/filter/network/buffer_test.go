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
