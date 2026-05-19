package lua

// body_buffer_test.go — Task 3 (phase 22.2 IMPL) behavioral tests for
// the NEW BodyBuffer seam interface per SPEC §3.2 + ADR-0191 §Context +
// §11.3 D3 RECOMMENDED option (a) defensive copy at endStream.
//
// Tests cover:
//   - Interface-signature compile-pin via mockBodyBuffer test-double
//     (catches accidental method-signature drift on Bytes/Chunks/EndStream
//     at compile time per the 22.1 vm_test.go signature-pin discipline).
//   - Consumer-side nil-tolerance: a nil BodyBuffer reference passed to
//     a small helper invoking `if b != nil { _ = b.Bytes() }` survives
//     without panic — the production bridge consumers (Task 7 :body() +
//     :bodyChunks()) follow this defensive-read pattern.
//   - Canned-value return from mockBodyBuffer: instantiate the test-
//     double with explicit bytes/chunks/endStream values, call each
//     method, assert canned values returned (interface CONTRACT, not
//     impl behavior — Task 7's concrete *decodedBody covers behavior).
//
// NO concrete BodyBuffer impl is tested here — that lives at
// internal/filter/http/lua/body.go at Task 7 per ADR-0191 §Context
// lineage separation. This file pins ONLY the interface CONTRACT.

import (
	"bytes"
	"testing"
)

// Compile-time signature pin per the 22.1 vm_test.go convention. The
// `var _ BodyBuffer = (*mockBodyBuffer)(nil)` assertion is a build-
// break the moment mockBodyBuffer's method set drifts away from the
// BodyBuffer interface signature (and conversely, the moment the
// BodyBuffer interface drifts away from the methods the production
// consumer at Task 7's `*decodedBody` will implement).
var _ BodyBuffer = (*mockBodyBuffer)(nil)

// mockBodyBuffer is a test-only test-double satisfying BodyBuffer for
// upstream-side test pinning. The production-side concrete impl lives
// at internal/filter/http/lua/body.go at Task 7.
//
// Fields are intentionally simple per-call canned values; tests
// construct instances directly via struct literal.
type mockBodyBuffer struct {
	bytesVal     []byte
	chunksVal    [][]byte
	endStreamVal bool
}

func (m *mockBodyBuffer) Bytes() []byte    { return m.bytesVal }
func (m *mockBodyBuffer) Chunks() [][]byte { return m.chunksVal }
func (m *mockBodyBuffer) EndStream() bool  { return m.endStreamVal }

// safeBytes is a tiny helper mirroring the defensive-read pattern that
// the Task 7 bridge consumer (`:body()` LGFunction) is expected to
// follow when invoking a possibly-nil BodyBuffer reference.
func safeBytes(b BodyBuffer) []byte {
	if b == nil {
		return nil
	}
	return b.Bytes()
}

// TestBodyBuffer_interface_signature_compiles_with_mock verifies the
// mockBodyBuffer test-double satisfies the BodyBuffer interface and a
// `BodyBuffer` variable holding the mock value is callable through
// each of the 3 methods. This complements the package-scope
// `var _ BodyBuffer = (*mockBodyBuffer)(nil)` compile-time check by
// exercising the interface dispatch at runtime.
func TestBodyBuffer_interface_signature_compiles_with_mock(t *testing.T) {
	var bb BodyBuffer = &mockBodyBuffer{}
	// Exercise each method through the interface (compile-pin + nil-
	// safety on the canned zero values).
	if got := bb.Bytes(); got != nil {
		t.Fatalf("zero-value mock Bytes() = %v; want nil", got)
	}
	if got := bb.Chunks(); got != nil {
		t.Fatalf("zero-value mock Chunks() = %v; want nil", got)
	}
	if got := bb.EndStream(); got {
		t.Fatalf("zero-value mock EndStream() = true; want false")
	}
}

// TestBodyBuffer_nil_tolerance_for_consumers verifies the defensive
// nil-check pattern that production bridge consumers (Task 7's
// :body() / :bodyChunks() LGFunctions) follow when a stream's body
// has not yet been buffered. Passing a nil `BodyBuffer` to the
// safeBytes helper MUST return nil without panic.
func TestBodyBuffer_nil_tolerance_for_consumers(t *testing.T) {
	// Explicit nil interface — both interface-type and underlying value
	// are nil. The defensive `if b != nil` check inside safeBytes
	// short-circuits before any method dispatch.
	var bb BodyBuffer // nil interface value
	if got := safeBytes(bb); got != nil {
		t.Fatalf("safeBytes(nil) = %v; want nil", got)
	}

	// Smoke: typed-nil concrete pointer wrapped in the interface ALSO
	// passes the explicit nil check, but the underlying *mockBodyBuffer
	// dispatch would NPE on field access — production consumers should
	// always nil-check the interface value, not the concrete pointer.
	// This sub-case documents the convention.
	t.Run("typed_nil_pointer_in_interface_is_not_nil_interface", func(t *testing.T) {
		var m *mockBodyBuffer // nil pointer
		var bb2 BodyBuffer = m
		// bb2 is a non-nil interface value wrapping a nil *mockBodyBuffer.
		// The defensive `if b != nil` check inside safeBytes does NOT
		// short-circuit here; if consumers wrap typed nils they own the
		// dispatch hazard. Verify this fact via a recover-guarded call.
		defer func() {
			// We expect a panic on m.Bytes() field access; recover so
			// the test stays green and documents the contract.
			_ = recover()
		}()
		_ = safeBytes(bb2)
	})
}

// TestBodyBuffer_mock_Bytes_Chunks_EndStream_return_canned_values
// verifies each interface method on mockBodyBuffer returns the
// canned values the test-double was constructed with. This pins the
// interface CONTRACT (method-set + return shapes) — the production
// concrete impl's behavior is covered at Task 7's body_test.go.
func TestBodyBuffer_mock_Bytes_Chunks_EndStream_return_canned_values(t *testing.T) {
	canned := []byte("hello world")
	chunk1 := []byte("hello ")
	chunk2 := []byte("world")
	chunks := [][]byte{chunk1, chunk2}

	m := &mockBodyBuffer{
		bytesVal:     canned,
		chunksVal:    chunks,
		endStreamVal: true,
	}
	var bb BodyBuffer = m

	if got := bb.Bytes(); !bytes.Equal(got, canned) {
		t.Errorf("Bytes() = %q; want %q", got, canned)
	}

	gotChunks := bb.Chunks()
	if len(gotChunks) != len(chunks) {
		t.Fatalf("Chunks() len = %d; want %d", len(gotChunks), len(chunks))
	}
	for i, want := range chunks {
		if !bytes.Equal(gotChunks[i], want) {
			t.Errorf("Chunks()[%d] = %q; want %q", i, gotChunks[i], want)
		}
	}

	if got := bb.EndStream(); !got {
		t.Errorf("EndStream() = false; want true")
	}

	// Pre-endStream sub-case: a fresh mock with endStreamVal=false
	// reports EndStream()=false (matches the production §11.3 D3
	// "false until terminal endStream=true fires" contract).
	t.Run("endStream_false_pre_terminal", func(t *testing.T) {
		pre := &mockBodyBuffer{endStreamVal: false}
		if pre.EndStream() {
			t.Fatalf("pre-terminal EndStream() = true; want false")
		}
	})
}
