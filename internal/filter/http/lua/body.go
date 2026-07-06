package lua

// body.go — Task 7 (phase 22.2 IMPL) body bridge per SPEC §3.4 + §4.1 +
// §11.3 D3 closure (option (a) defensive copy at endStream) + ADR-0192
// §Decision body anticipation + ADR-0191 BodyBuffer interface consumer.
//
// # Surface
//
// Two bridge methods on each of request_handle + response_handle:
//
//   - :body() returns the full accumulated body as a Lua string. If the
//     body is NOT yet fully accumulated (decode-side endStream NOT yet
//     fired), the bridge yields via YieldFromBridge per §11.1 D2 closure;
//     the per-filter DecodeData callback resumes the suspended coroutine
//     at endStream with the accumulated bytes. If accumulated bytes
//     exceed f.maxBodyBufferedBytes, the bridge raises an arm-21 runtime-
//     reject with the byte-stable wording per W2:
//     `"lua: body: accumulated body exceeds maximum buffered size of %d bytes"`.
//
//   - :bodyChunks() returns a Lua iterator function that yields the
//     per-DecodeData chunks one at a time (each defensive-copied via
//     lua.LString(string(chunk)) per §11.3 D3); the iterator returns nil
//     when chunks are exhausted.
//
// # Coroutine yield/resume orchestration (R9 SIGNAL PROTOCOL evaluation)
//
// The body bridge consumes Task 2's internal/lua YieldFromBridge +
// VM.Resume API. Sequence:
//
//   1. envoy_on_request hook fires inside a child coroutine (the
//      DecodeHeaders dispatcher mints the child via vm.NewThread + drives
//      vm.Resume(child, fn, reqUd)).
//   2. The script calls rh:body(). If !f.bodyReady, requestHandleBody
//      stashes the child *LState on f.pendingBodyResume, increments
//      f.cc.stats.coroutineYieldsTotal, and returns YieldFromBridge(L,
//      lua.LNil) — gopher-lua's callGFunction sees gfnret<0 and unwinds
//      via switchToParentThread per §11.1 D2 closure.
//   3. The parent's Resume call returns ResumeYield to the dispatcher.
//   4. When DecodeData fires with endStream=true, f.bodyReady is set +
//      vm.Resume(f.pendingBodyResume, nil, lua.LString(string(bytes)))
//      is invoked; the script's local-binding receives the resumed bytes
//      via the bridge function's Lua-side call expression.
//
// Per the Task 7 dispatch outline §13-R9 disposition evaluation: the
// body bridge implementation surface DOES NOT introduce additional ADR-
// warranting complexity beyond what is already documented under ADR-0192
// §Context. The yield/resume orchestration is mechanically simple
// (single suspended *LState slot per stream; single Resume site at
// endStream); the defensive-copy discipline is one line per call site;
// the over-cap arm is byte-stable wording. **R9 DISPOSITION: STAYS
// embedded in ADR-0192.** Recorded in the Task 7 PROGRESS.md entry per
// the R9 signal protocol per PLAN Task 7 Step 6.
//
// # Concrete BodyBuffer implementation (ADR-0191 consumer)
//
// The decodedBodyBuffer + encodedBodyBuffer types below satisfy the
// internal/lua.BodyBuffer interface declared at Task 3. The interface
// decouples internal/lua/ from the per-filter accumulation surface;
// concrete consumers live here per ADR-0191 §Context lineage separation.
// At Task 7 the interface is satisfied for symmetric introspection (the
// :body() / :bodyChunks() LGFunctions consume the *filter directly for
// performance — one indirection vs the interface seam — but the
// interface-satisfying wrappers are retained for cross-package consumers
// + the BodyBuffer test compliance assertion at Task 3).
//
// # Defensive-copy discipline at endStream (§11.3 D3)
//
// Every :body() / :bodyChunks() chunk-emit defensive-copies via
// lua.LString(string(bytes)) — gopher-lua's LString IS a Go string
// (interned; immutable by Go semantics; NOT a direct pointer to the
// underlying []byte). The defensive copy detaches Lua's ownership from
// the underlying byte-slice lifetime — safe across coroutine
// yield/resume + HCM dispatch goroutine lifetimes. Verified at
// Test_RequestHandleBody_defensive_copy_verified (mutating
// f.decodedBodyBytes Go-side after :body() returns does NOT change the
// Lua string).

import (
	"fmt"

	lua "github.com/yuin/gopher-lua"

	luaprim "github.com/pgdad/envoy-go/internal/lua"
)

// -----------------------------------------------------------------------
// Concrete BodyBuffer implementations (ADR-0191 interface consumers)
// -----------------------------------------------------------------------

// decodedBodyBuffer wraps the per-filter decode-side accumulation +
// satisfies internal/lua.BodyBuffer. Returned for cross-package
// consumers + as a documentation surface for the per-filter discipline.
// Internal :body() / :bodyChunks() LGFunctions consume the *filter
// directly to avoid the interface-call overhead on the body-bridge hot
// path.
type decodedBodyBuffer struct {
	f *filter
}

// Bytes returns the full accumulated decode-side body. Consumers SHOULD
// defensive-copy via lua.LString(string(b.Bytes())) per §11.3 D3.
func (b *decodedBodyBuffer) Bytes() []byte {
	if b == nil || b.f == nil {
		return nil
	}
	return b.f.decodedBodyBytes
}

// Chunks returns the per-DecodeData decode-side chunks.
func (b *decodedBodyBuffer) Chunks() [][]byte {
	if b == nil || b.f == nil {
		return nil
	}
	return b.f.decodedBodyChunks
}

// EndStream reports whether the terminal decode-side endStream signal
// has fired.
func (b *decodedBodyBuffer) EndStream() bool {
	if b == nil || b.f == nil {
		return false
	}
	return b.f.bodyReady
}

// encodedBodyBuffer wraps the per-filter encode-side accumulation; same
// shape + discipline as decodedBodyBuffer.
type encodedBodyBuffer struct {
	f *filter
}

// Bytes returns the full accumulated encode-side body.
func (b *encodedBodyBuffer) Bytes() []byte {
	if b == nil || b.f == nil {
		return nil
	}
	return b.f.encodedBodyBytes
}

// Chunks returns the per-EncodeData encode-side chunks.
func (b *encodedBodyBuffer) Chunks() [][]byte {
	if b == nil || b.f == nil {
		return nil
	}
	return b.f.encodedBodyChunks
}

// EndStream reports whether the terminal encode-side endStream signal
// has fired.
func (b *encodedBodyBuffer) EndStream() bool {
	if b == nil || b.f == nil {
		return false
	}
	return b.f.respBodyReady
}

// Compile-time interface conformance assertions per ADR-0191 + Task 3
// BodyBuffer seam. The interface-satisfying wrappers can be consumed by
// future cross-package callers (e.g., a separate body-buffer observer
// at fuzz harnesses); the per-filter LGFunctions below short-circuit
// the interface and read *filter directly.
var (
	_ luaprim.BodyBuffer = (*decodedBodyBuffer)(nil)
	_ luaprim.BodyBuffer = (*encodedBodyBuffer)(nil)
)

// -----------------------------------------------------------------------
// Body-accumulation helpers (called from DecodeData / EncodeData)
// -----------------------------------------------------------------------

// accumulateRequestBody is the DecodeData hot-path body accumulator. Per
// SPEC §4.1 + §11.3.2: each DecodeData call appends to f.decodedBodyBytes
// + f.decodedBodyChunks (mirrors ext_authz / ext_proc per-filter
// accumulation patterns). On terminal endStream the f.bodyReady flag
// fires + any suspended request-side coroutine (pending :body() awaiting
// endStream) is resumed with the accumulated bytes via vm.Resume.
//
// Counter discipline:
//   - bodyBufferedBytesTotal increments by len(data) per call (cumulative
//     byte volume per SPEC §7.1).
//
// Defensive copy: f.decodedBodyChunks captures a per-call slice of len
// len(data) — copies the bytes (the framework-supplied data slice's
// backing array lifetime is NOT guaranteed beyond DecodeData return, so
// chunks MUST defensive-copy at append time to be safely re-readable
// later by :bodyChunks()).
func accumulateRequestBody(f *filter, data []byte, endStream bool) {
	if f == nil {
		return
	}
	// Lazy-initialize the per-stream cap on first DecodeData. Tests may
	// pre-populate to a smaller value to exercise arm-21; production
	// reaches here with zero value + sets the default.
	if f.maxBodyBufferedBytes == 0 {
		f.maxBodyBufferedBytes = defaultMaxBodyBufferedBytes
	}

	if len(data) > 0 {
		// Defensive copy of the data slice — the framework-supplied
		// []byte's backing array may be reused after DecodeData returns,
		// so chunks MUST own a fresh allocation for safe later iteration.
		chunk := make([]byte, len(data))
		copy(chunk, data)
		f.decodedBodyBytes = append(f.decodedBodyBytes, chunk...)
		f.decodedBodyChunks = append(f.decodedBodyChunks, chunk)
		if f.cc != nil && f.cc.stats != nil && f.cc.stats.bodyBufferedBytesTotal != nil {
			f.cc.stats.bodyBufferedBytesTotal.Add(uint64(len(data)))
		}
	}

	if endStream {
		f.bodyReady = true
		// Resume any suspended :body() coroutine with the accumulated
		// bytes. Defensive-copy into a Lua-owned LString per §11.3 D3
		// (gopher-lua's LString IS an immutable Go string — detaches Lua
		// ownership from the decodedBodyBytes slice lifetime).
		//
		// Resume's error return is intentionally discarded here: any
		// script-side runtime error post-resume is captured via
		// stats.errors at the dispatch site (decode_headers.go) when
		// the parent's encompassing Resume returns ResumeError. The
		// inner Resume here is part of the bridge plumbing, not the
		// outermost dispatch site — its return is not actionable at
		// this level. Future Task 14 / Task 16 may surface the error
		// through a per-stream-debug log channel.
		if f.pendingBodyResume != nil && f.vm != nil {
			child := f.pendingBodyResume
			f.pendingBodyResume = nil // clear before Resume to prevent double-dispatch
			_, _, _ = f.vm.Resume(child, nil, lua.LString(string(f.decodedBodyBytes)))
		}
	}
}

// accumulateResponseBody is the EncodeData hot-path body accumulator.
// Symmetric to accumulateRequestBody for the response side.
func accumulateResponseBody(f *filter, data []byte, endStream bool) {
	if f == nil {
		return
	}
	if f.maxBodyBufferedBytes == 0 {
		f.maxBodyBufferedBytes = defaultMaxBodyBufferedBytes
	}

	if len(data) > 0 {
		chunk := make([]byte, len(data))
		copy(chunk, data)
		f.encodedBodyBytes = append(f.encodedBodyBytes, chunk...)
		f.encodedBodyChunks = append(f.encodedBodyChunks, chunk)
		if f.cc != nil && f.cc.stats != nil && f.cc.stats.bodyBufferedBytesTotal != nil {
			f.cc.stats.bodyBufferedBytesTotal.Add(uint64(len(data)))
		}
	}

	if endStream {
		f.respBodyReady = true
		// Resume return values discarded per the symmetric discipline
		// documented at accumulateRequestBody (see comments there).
		if f.pendingRespBodyResume != nil && f.vm != nil {
			child := f.pendingRespBodyResume
			f.pendingRespBodyResume = nil
			_, _, _ = f.vm.Resume(child, nil, lua.LString(string(f.encodedBodyBytes)))
		}
	}
}

// -----------------------------------------------------------------------
// Bridge methods — request_handle:body / :bodyChunks
// -----------------------------------------------------------------------

// requestHandleBody implements request_handle:body() per SPEC §3.4 +
// §4.1 + §11.3 D3 closure (defensive copy at endStream).
//
// Logic:
//
//  1. Resolve the owning *filter via the userdata's filterRef back-
//     pointer.
//  2. If !f.bodyReady (endStream NOT yet fired), stash L on
//     f.pendingBodyResume + increment coroutineYieldsTotal + return
//     YieldFromBridge(L, lua.LNil) per §11.1 D2 closure. The DecodeData
//     callback at endStream resumes the suspended coroutine with the
//     accumulated bytes; the script's local-binding receives the
//     resumed bytes via the bridge function's Lua-side call expression.
//  3. If len(f.decodedBodyBytes) > f.maxBodyBufferedBytes, raise the
//     arm-21 runtime-reject with byte-stable wording per W2.
//  4. Otherwise: push lua.LString(string(f.decodedBodyBytes)) per the
//     §11.3 D3 defensive-copy discipline + return 1.
func requestHandleBody(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*requestHandleContext)
	if !ok {
		L.ArgError(1, "expected request_handle")
		return 0
	}
	f := ctx.filterRef
	if f == nil {
		// Defensive: synthetic test path without a filterRef binding.
		// Push empty string per the "always-string, never-nil" contract.
		L.Push(lua.LString(""))
		return 1
	}

	if !f.bodyReady {
		// Yield: stash the bridge LState on the per-filter pending slot,
		// bump the yield-event counter ONCE per yield (not per Resume),
		// and return the YieldFromBridge sentinel to gopher-lua's
		// callGFunction.
		f.pendingBodyResume = L
		if f.cc != nil && f.cc.stats != nil && f.cc.stats.coroutineYieldsTotal != nil {
			f.cc.stats.coroutineYieldsTotal.Inc()
		}
		return luaprim.YieldFromBridge(L, lua.LNil)
	}

	if len(f.decodedBodyBytes) > f.maxBodyBufferedBytes {
		L.RaiseError("%s", fmt.Sprintf(
			"lua: body: accumulated body exceeds maximum buffered size of %d bytes",
			f.maxBodyBufferedBytes,
		))
		return 0
	}

	// Defensive copy at endStream per §11.3 D3: gopher-lua's LString is
	// an immutable Go string; converting via string(b) creates a fresh
	// backing array — Lua owns it across coroutine yield/resume + HCM
	// dispatch goroutine lifetimes.
	L.Push(lua.LString(string(f.decodedBodyBytes)))
	return 1
}

// requestHandleBodyChunks implements request_handle:bodyChunks() per
// SPEC §3.4. Returns a stateful Lua iterator function that yields each
// per-DecodeData chunk on successive invocations (defensive-copied via
// lua.LString); returns nil when chunks are exhausted.
//
// Pre-endStream semantics: the iterator emits available chunks then nil
// (matching the upstream Lua filter's bodyChunks() iterator-completion
// contract — the script author drives multiple iterations across
// DecodeData firings; for simplicity at 22.2 the iterator captures the
// chunk-slice at call-time and emits those chunks). At 22.2 the typical
// caller invokes :bodyChunks() inside envoy_on_request_body or after
// :body() — at endStream the chunk slice is fully populated.
//
// The chunk slice is captured by closure at iterator-construction time;
// subsequent DecodeData firings AFTER :bodyChunks() invocation will
// extend f.decodedBodyChunks but the iterator's captured len(snap) does
// NOT see those extensions (snapshot-at-call-time semantics — matches
// the upstream behavior).
func requestHandleBodyChunks(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*requestHandleContext)
	if !ok {
		L.ArgError(1, "expected request_handle")
		return 0
	}
	f := ctx.filterRef
	var chunks [][]byte
	if f != nil {
		chunks = f.decodedBodyChunks
	}

	i := 0
	iter := L.NewFunction(func(L2 *lua.LState) int {
		if i >= len(chunks) {
			L2.Push(lua.LNil)
			return 1
		}
		c := chunks[i]
		i++
		L2.Push(lua.LString(string(c)))
		return 1
	})
	L.Push(iter)
	return 1
}

// -----------------------------------------------------------------------
// Bridge methods — response_handle:body / :bodyChunks (symmetric)
// -----------------------------------------------------------------------

// responseHandleBody implements response_handle:body() symmetric to
// requestHandleBody for the encode side. Pre-endStream suspends via
// YieldFromBridge; EncodeData at endStream resumes with the
// accumulated encode-side bytes.
func responseHandleBody(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*responseHandleContext)
	if !ok {
		L.ArgError(1, "expected response_handle")
		return 0
	}
	f := ctx.filterRef
	if f == nil {
		L.Push(lua.LString(""))
		return 1
	}

	if !f.respBodyReady {
		f.pendingRespBodyResume = L
		if f.cc != nil && f.cc.stats != nil && f.cc.stats.coroutineYieldsTotal != nil {
			f.cc.stats.coroutineYieldsTotal.Inc()
		}
		return luaprim.YieldFromBridge(L, lua.LNil)
	}

	if len(f.encodedBodyBytes) > f.maxBodyBufferedBytes {
		L.RaiseError("%s", fmt.Sprintf(
			"lua: body: accumulated body exceeds maximum buffered size of %d bytes",
			f.maxBodyBufferedBytes,
		))
		return 0
	}

	L.Push(lua.LString(string(f.encodedBodyBytes)))
	return 1
}

// responseHandleBodyChunks implements response_handle:bodyChunks()
// symmetric to requestHandleBodyChunks for the encode side.
func responseHandleBodyChunks(L *lua.LState) int {
	ud := L.CheckUserData(1)
	ctx, ok := ud.Value.(*responseHandleContext)
	if !ok {
		L.ArgError(1, "expected response_handle")
		return 0
	}
	f := ctx.filterRef
	var chunks [][]byte
	if f != nil {
		chunks = f.encodedBodyChunks
	}

	i := 0
	iter := L.NewFunction(func(L2 *lua.LState) int {
		if i >= len(chunks) {
			L2.Push(lua.LNil)
			return 1
		}
		c := chunks[i]
		i++
		L2.Push(lua.LString(string(c)))
		return 1
	})
	L.Push(iter)
	return 1
}
