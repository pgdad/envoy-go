package lua

// body_buffer.go — Task 3 (phase 22.2 IMPL) NEW BodyBuffer seam
// interface per 22.2 SPEC §3.2 + ADR-0191 §Context + §11.3 D3
// RECOMMENDED option (a) defensive copy at endStream.
//
// This file declares ONLY the interface CONTRACT. The concrete
// implementation lives at internal/filter/http/lua/body.go at
// Task 7 per ADR-0191 §Context lineage separation — the seam
// decouples internal/lua/ from HCM-level body-buffer accumulation
// (the upstream supplier is ADR-0128's decode-side bodyBuf primitive
// at internal/filter/hcm/connection.go:483, accumulated via append-
// per-DecodeData and passed downstream to filters via RunDecodeData).
//
// Cross-references:
//   - 22.2 SPEC §3.2 (production signature block)
//   - 22.2 SPEC §11.3 D3 RECOMMENDATION (defensive copy at endStream
//     for GC safety across coroutine yield/resume + HCM dispatch
//     goroutine lifetimes)
//   - DECISIONS.md ADR-0191 §Context (lineage-separation rationale —
//     this seam; §Decision body lands at Task 19 atomic landing per
//     ADR-0044 in-place edit discipline)
//   - DECISIONS.md ADR-0128 §Decision (HCM-level decode-side bodyBuf
//     primitive — the upstream supplier of the underlying bytes that
//     a concrete BodyBuffer impl wraps)
//   - ADR-0188 §Decision 5 API-REVISION ALLOWANCE clause STAYS scoped
//     to consumer-#2; this consumer-#1 scope-expansion lands under
//     NEW ADR-0191 (not in-place AMEND on ADR-0188).

// BodyBuffer is the seam interface consumed by the lua bridge's
// :body() + :bodyChunks() methods. Concrete implementation lives
// at internal/filter/http/lua/body.go per ADR-0191 §Context lineage
// separation (this seam decouples internal/lua/ from HCM-level body-
// buffer accumulation per ADR-0128's decode-side bodyBuf primitive).
//
// Per §11.3 D3 RECOMMENDED option (a) defensive copy at endStream:
//
//   - Bytes() returns the full accumulated body. Consumers SHOULD
//     defensive-copy this slice when passing to Lua (via
//     lua.LString(string(bytes))) so Lua owns the resulting string
//     across coroutine yield/resume + HCM dispatch goroutine
//     lifetimes (gopher-lua's LString is an interned Go string, not
//     a direct pointer to the HCM's []byte). Returns nil if body
//     not yet available; the returned slice MUST NOT be mutated by
//     consumers (treat as read-only).
//
//   - Chunks() returns per-DecodeData chunks for the :bodyChunks()
//     iterator (paralleling ext_proc's bodyStageEntry pattern per
//     §11.3.2). Returns nil if body not yet available; each inner
//     slice is read-only.
//
//   - EndStream() reports whether the terminal endStream=true signal
//     has fired (the synthetic empty-terminal RunDecodeData(ctx, nil,
//     true) at ADR-0128's connection.go:505-509). Returns false until
//     the body is fully accumulated; true once the terminal signal
//     fires.
//
// Cross-references: ADR-0191 §Context (this interface);
// ADR-0128 §Decision (HCM-level bodyBuf supplier);
// 22.2 SPEC §3.2 + §11.3 D3 (closure RECOMMENDED disposition).
type BodyBuffer interface {
	// Bytes returns the full accumulated body as one byte slice.
	// Returns nil if body not yet available; the slice MUST NOT be
	// mutated by consumers (treat as read-only). Consumers SHOULD
	// defensive-copy into a Go string via lua.LString(string(b.Bytes()))
	// when forwarding to Lua per §11.3 D3 RECOMMENDED.
	Bytes() []byte

	// Chunks returns the body as a sequence of per-DecodeData chunks.
	// Returns nil if body not yet available. Each inner slice is
	// read-only. Used by the :bodyChunks() iterator bridge.
	Chunks() [][]byte

	// EndStream reports whether the terminal endStream=true signal
	// has fired. Returns false until the body is fully accumulated.
	EndStream() bool
}
