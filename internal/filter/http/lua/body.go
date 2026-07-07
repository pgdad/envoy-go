package lua

// body.go — Task 7 (phase 22.2 IMPL) body bridge per SPEC §3.4 + §4.1 +
// §11.3 D3 closure (option (a) defensive copy at endStream) + ADR-0192
// §Decision body anticipation.
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
// Per-side field selection (decode vs encode)
// -----------------------------------------------------------------------

// bodySide selects the per-filter fields for ONE body direction. The
// decode and encode body paths are structurally identical — they differ
// only in which per-filter fields they touch (decodedBodyBytes /
// bodyReady / pendingBodyResume vs the encoded* / resp* mirrors) and in
// which hook name the error log cites. Parameterizing accumulate /
// :body() / :bodyChunks() by side keeps each implemented ONCE so any
// fix lands on both directions.
type bodySide struct {
	// hook is the dispatcher hook name ("envoy_on_request" /
	// "envoy_on_response") cited by the inner-Resume error log —
	// matches the decode_headers.go / encode_headers.go dispatcher
	// wording.
	hook string

	bytes         *[]byte
	chunks        *[][]byte
	ready         *bool
	pendingResume **lua.LState
}

// decodeBodySide selects the request-side (DecodeData) accumulation
// fields.
func (f *filter) decodeBodySide() bodySide {
	return bodySide{
		hook:          "envoy_on_request",
		bytes:         &f.decodedBodyBytes,
		chunks:        &f.decodedBodyChunks,
		ready:         &f.bodyReady,
		pendingResume: &f.pendingBodyResume,
	}
}

// encodeBodySide selects the response-side (EncodeData) accumulation
// fields.
func (f *filter) encodeBodySide() bodySide {
	return bodySide{
		hook:          "envoy_on_response",
		bytes:         &f.encodedBodyBytes,
		chunks:        &f.encodedBodyChunks,
		ready:         &f.respBodyReady,
		pendingResume: &f.pendingRespBodyResume,
	}
}

// noteInnerResumeError increments stats.errors + logs when an inner
// (bridge-driven) vm.Resume returns a script runtime error. The outer
// dispatch site (decode_headers.go / encode_headers.go) never sees this
// error — its own Resume already returned ResumeYield when the script
// suspended — so the capture MUST happen at the inner Resume site to
// preserve the upstream-parity contract that the `errors` counter
// increments on ANY hook runtime error. Wording matches the dispatcher
// ("ERROR lua: <hook> failed: <err>"). Shared by the body accumulator
// below + the httpCall dispatch goroutine (httpcall.go).
func noteInnerResumeError(f *filter, hook string, rerr error) {
	if rerr == nil {
		return
	}
	if f != nil && f.cc != nil && f.cc.stats != nil && f.cc.stats.errors != nil {
		f.cc.stats.errors.Inc()
	}
	logf("ERROR lua: %s failed: %v", hook, rerr)
}

// -----------------------------------------------------------------------
// Body-accumulation helpers (called from DecodeData / EncodeData)
// -----------------------------------------------------------------------

// accumulateBody is the shared DecodeData / EncodeData hot-path body
// accumulator, parameterized by side. Per SPEC §4.1 + §11.3.2: each
// data call appends to the side's bytes + chunks accumulators (mirrors
// ext_authz / ext_proc per-filter accumulation patterns). On terminal
// endStream the side's ready flag fires + any suspended coroutine
// (pending :body() awaiting endStream) is resumed with the accumulated
// bytes via vm.Resume. Returns true when a suspended coroutine was
// resumed at this call — the caller (DecodeData in lua.go) uses this to
// run the post-resume respond-state check.
//
// Counter discipline:
//   - bodyBufferedBytesTotal increments by len(data) per call (cumulative
//     byte volume per SPEC §7.1).
//
// Defensive copy: the chunks accumulator captures a per-call slice of
// len(data) — copies the bytes (the framework-supplied data slice's
// backing array lifetime is NOT guaranteed beyond DecodeData/EncodeData
// return, so chunks MUST defensive-copy at append time to be safely
// re-readable later by :bodyChunks()).
func accumulateBody(f *filter, side bodySide, data []byte, endStream bool) bool {
	// Lazy-initialize the per-stream cap on first data call. Tests may
	// pre-populate to a smaller value to exercise arm-21; production
	// reaches here with zero value + sets the default.
	if f.maxBodyBufferedBytes == 0 {
		f.maxBodyBufferedBytes = defaultMaxBodyBufferedBytes
	}

	if len(data) > 0 {
		// Defensive copy of the data slice — the framework-supplied
		// []byte's backing array may be reused after the callback returns,
		// so chunks MUST own a fresh allocation for safe later iteration.
		chunk := make([]byte, len(data))
		copy(chunk, data)
		*side.bytes = append(*side.bytes, chunk...)
		*side.chunks = append(*side.chunks, chunk)
		if f.cc != nil && f.cc.stats != nil && f.cc.stats.bodyBufferedBytesTotal != nil {
			f.cc.stats.bodyBufferedBytesTotal.Add(uint64(len(data)))
		}
	}

	if !endStream {
		return false
	}
	*side.ready = true
	if *side.pendingResume == nil || f.vm == nil {
		return false
	}
	// Resume the suspended :body() coroutine with the accumulated
	// bytes. Defensive-copy into a Lua-owned LString per §11.3 D3
	// (gopher-lua's LString IS an immutable Go string — detaches Lua
	// ownership from the accumulated slice lifetime).
	child := *side.pendingResume
	*side.pendingResume = nil // clear before Resume to prevent double-dispatch
	_, rerr, _ := f.vm.Resume(child, nil, lua.LString(string(*side.bytes)))
	// Any script runtime error raised AFTER this resume is invisible to
	// the outer dispatch site (its Resume already returned ResumeYield);
	// capture it here per the upstream-parity errors-counter contract.
	noteInnerResumeError(f, side.hook, rerr)
	return true
}

// accumulateRequestBody is the DecodeData entry to the shared
// accumulator (request side). Returns true when a suspended :body()
// coroutine was resumed at this call — DecodeData (lua.go) re-runs the
// respond-state check in that case, since the resumed continuation may
// have called request_handle:respond().
func accumulateRequestBody(f *filter, data []byte, endStream bool) bool {
	if f == nil {
		return false
	}
	return accumulateBody(f, f.decodeBodySide(), data, endStream)
}

// accumulateResponseBody is the EncodeData entry to the shared
// accumulator (response side). The resumed return is not consumed on
// the encode side: encode-side :respond() raises the AMEND-8 runtime
// error (captured via noteInnerResumeError inside accumulateBody), so
// there is no respond state to re-check.
func accumulateResponseBody(f *filter, data []byte, endStream bool) {
	if f == nil {
		return
	}
	accumulateBody(f, f.encodeBodySide(), data, endStream)
}

// -----------------------------------------------------------------------
// Bridge methods — request_handle:body / :bodyChunks
// -----------------------------------------------------------------------

// handleBody is the shared :body() bridge body, parameterized by side
// per SPEC §3.4 + §4.1 + §11.3 D3 closure (defensive copy at
// endStream). The caller (requestHandleBody / responseHandleBody) has
// already resolved the owning non-nil *filter via the userdata's
// filterRef back-pointer.
//
// Logic:
//
//  1. If !*side.ready (endStream NOT yet fired), stash L on
//     *side.pendingResume + increment coroutineYieldsTotal + return
//     YieldFromBridge(L, lua.LNil) per §11.1 D2 closure. The
//     DecodeData / EncodeData callback at endStream resumes the
//     suspended coroutine with the accumulated bytes; the script's
//     local-binding receives the resumed bytes via the bridge
//     function's Lua-side call expression.
//  2. If len(*side.bytes) > f.maxBodyBufferedBytes, raise the arm-21
//     runtime-reject with byte-stable wording per W2.
//  3. Otherwise: push lua.LString(string(*side.bytes)) per the §11.3
//     D3 defensive-copy discipline + return 1.
func handleBody(L *lua.LState, f *filter, side bodySide) int {
	if !*side.ready {
		// Yield: stash the bridge LState on the per-filter pending slot,
		// bump the yield-event counter ONCE per yield (not per Resume),
		// and return the YieldFromBridge sentinel to gopher-lua's
		// callGFunction.
		*side.pendingResume = L
		if f.cc != nil && f.cc.stats != nil && f.cc.stats.coroutineYieldsTotal != nil {
			f.cc.stats.coroutineYieldsTotal.Inc()
		}
		return luaprim.YieldFromBridge(L, lua.LNil)
	}

	if len(*side.bytes) > f.maxBodyBufferedBytes {
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
	L.Push(lua.LString(string(*side.bytes)))
	return 1
}

// pushBodyChunksIter is the shared :bodyChunks() bridge body. Pushes a
// stateful Lua iterator function that yields each per-DecodeData /
// per-EncodeData chunk on successive invocations (defensive-copied via
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
// subsequent DecodeData / EncodeData firings AFTER :bodyChunks()
// invocation will extend the per-filter chunk slice but the iterator's
// captured len(chunks) does NOT see those extensions (snapshot-at-call-
// time semantics — matches the upstream behavior).
func pushBodyChunksIter(L *lua.LState, chunks [][]byte) int {
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

// requestHandleBody implements request_handle:body(). Resolves the
// owning *filter via the userdata's filterRef back-pointer, then
// delegates to the shared handleBody with the decode side selected.
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
	return handleBody(L, f, f.decodeBodySide())
}

// requestHandleBodyChunks implements request_handle:bodyChunks() per
// SPEC §3.4 — see pushBodyChunksIter for the iterator semantics.
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
	return pushBodyChunksIter(L, chunks)
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
	return handleBody(L, f, f.encodeBodySide())
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
	return pushBodyChunksIter(L, chunks)
}
