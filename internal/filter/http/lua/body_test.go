package lua

// body_test.go — Task 7 IMPL behavioral tests for the body bridge per
// 22.2 PLAN Task 7 + 22.2 SPEC §3.4 + §4.1 + §11.3 D3 closure (defensive
// copy at endStream) + §6 arm-21 (body-size-cap-exceeded byte-stable
// wording per W2).
//
// 8 test functions per the PLAN Task 7 Step 1 enumeration:
//   - Test_RequestHandleBody_returns_full_bytes
//   - Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording
//   - Test_RequestHandleBodyChunks_iterator_yields_chunks_then_nil
//   - Test_RequestHandleBody_coroutine_yield_before_endStream_then_resume
//   - Test_RequestHandleBody_defensive_copy_verified
//   - Test_body_buffered_bytes_total_counter_increments
//   - Test_coroutine_yields_total_counter_increments
//   - Test_ResponseHandleBody_symmetric

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"

	luaprim "github.com/esalaine/envoy-go/internal/lua"
	"github.com/esalaine/envoy-go/internal/stats"
)

// newBodyBridgeFilter constructs a per-test *filter with the body-bridge
// scaffolding wired: a VM + the bridge metatables installed + a
// request_handle context bound to global `rh` + a response_handle context
// bound to global `resp` + a stats registry providing the 2 NEW Task 7
// counters. Returns the filter so tests can drive DecodeData / EncodeData
// directly + the VM for script execution.
func newBodyBridgeFilter(t *testing.T) *filter {
	t.Helper()
	reg := stats.NewRegistry()
	cc := &compiledConfig{
		stats: &filterStats{
			errors:                 reg.NewCounter("test.errors"),
			executions:             reg.NewCounter("test.executions"),
			respondCalls:           reg.NewCounter("test.respond_calls"),
			bodyBufferedBytesTotal: reg.NewCounter("test.body_buffered_bytes_total"),
			coroutineYieldsTotal:   reg.NewCounter("test.coroutine_yields_total"),
		},
	}
	f := &filter{cc: cc}
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	f.vm = vm
	// Attach a context to the parent LState so that coroutine NewThread
	// returns a non-nil CancelFunc per gopher-lua state.go:1614 — tests
	// that drive coroutine paths need a cancellable child loop. Tests
	// that don't drive coroutines pay the trivial overhead of an unused
	// context.
	ctx, cancelCtx := context.WithCancel(context.Background())
	t.Cleanup(cancelCtx)
	vm.State().SetContext(ctx)
	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installPairsShim(L)

	// request_handle userdata bound to global `rh`.
	f.reqCtx = &requestHandleContext{headers: nil, filterRef: f}
	rud := L.NewUserData()
	rud.Value = f.reqCtx
	L.SetMetatable(rud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", rud)

	// response_handle userdata bound to global `resp`.
	f.respCtx = &responseHandleContext{headers: nil, filterRef: f}
	pud := L.NewUserData()
	pud.Value = f.respCtx
	L.SetMetatable(pud, L.GetTypeMetatable(responseHandleTypeName))
	L.SetGlobal("resp", pud)
	return f
}

// runBodyScript compiles + runs the supplied Lua source on the filter's
// VM; fails on either compile or runtime error.
func runBodyScript(t *testing.T, f *filter, src string) {
	t.Helper()
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; src = %q", err, src)
	}
	if err := f.vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; src = %q", err, src)
	}
}

// getStrGlobal fetches a Lua string global; fails on absent / type
// mismatch.
func getStrGlobal(t *testing.T, f *filter, name string) string {
	t.Helper()
	v := f.vm.State().GetGlobal(name)
	s, ok := v.(lua.LString)
	if !ok {
		t.Fatalf("global %q type = %s; want string (got %v)", name, v.Type(), v)
	}
	return string(s)
}

// -----------------------------------------------------------------------
// Test 1: :body() returns full accumulated bytes when body already ready.
// -----------------------------------------------------------------------

func Test_RequestHandleBody_returns_full_bytes(t *testing.T) {
	f := newBodyBridgeFilter(t)
	// Drive DecodeData with two chunks then endStream to fully buffer
	// the body. The bridge's :body() should return the concatenated
	// bytes verbatim.
	f.DecodeData([]byte("hello "), false)
	f.DecodeData([]byte("world"), true)
	runBodyScript(t, f, `result = rh:body()`)
	got := getStrGlobal(t, f, "result")
	if got != "hello world" {
		t.Fatalf("rh:body() = %q; want %q", got, "hello world")
	}
}

// -----------------------------------------------------------------------
// Test 2: arm-21 byte-stable wording when body exceeds cap.
// Per SPEC §6 + W2: "lua: body: accumulated body exceeds maximum
// buffered size of %d bytes".
// -----------------------------------------------------------------------

func Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording(t *testing.T) {
	f := newBodyBridgeFilter(t)
	// Lower the cap to a small testable value to avoid a 17 MiB allocation
	// in CI. The byte-stable wording substitutes the configured cap value.
	const testCap = 1024
	f.maxBodyBufferedBytes = testCap
	// Drive DecodeData with cap+1 bytes (1025 bytes) → :body() must
	// raise arm-21 runtime-reject.
	big := bytes.Repeat([]byte("A"), testCap+1)
	f.DecodeData(big, true)

	chunk, err := luaprim.CompileScript([]byte(`rh:body()`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v", err)
	}
	err = f.vm.Run(chunk)
	if err == nil {
		t.Fatalf("vm.Run err = nil; want error (over-cap arm-21)")
	}
	want := fmt.Sprintf("lua: body: accumulated body exceeds maximum buffered size of %d bytes", testCap)
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("err = %q; want substring %q", err.Error(), want)
	}
}

// -----------------------------------------------------------------------
// Test 3: :bodyChunks() iterator yields chunks then nil.
// -----------------------------------------------------------------------

func Test_RequestHandleBodyChunks_iterator_yields_chunks_then_nil(t *testing.T) {
	f := newBodyBridgeFilter(t)
	// 3 chunks → 3 yields then nil-terminator.
	f.DecodeData([]byte("aaa"), false)
	f.DecodeData([]byte("bbb"), false)
	f.DecodeData([]byte("ccc"), true)

	// Iterate via the closure-iterator the bridge returns. Concatenate
	// chunks into a global; assert correct ordering + nil-terminator.
	runBodyScript(t, f, `
		local iter = rh:bodyChunks()
		seq = ""
		local terminated = false
		while true do
			local c = iter()
			if c == nil then terminated = true; break end
			seq = seq .. c
		end
		term = terminated
	`)
	seq := getStrGlobal(t, f, "seq")
	if seq != "aaabbbccc" {
		t.Fatalf("seq = %q; want %q", seq, "aaabbbccc")
	}
	term := f.vm.State().GetGlobal("term")
	if term != lua.LTrue {
		t.Fatalf("iterator did not terminate via nil; term=%v", term)
	}
}

// -----------------------------------------------------------------------
// Test 4: :body() BEFORE endStream → suspends via coroutine yield;
// DecodeData(endStream=true) resumes → script sees full bytes.
// -----------------------------------------------------------------------

func Test_RequestHandleBody_coroutine_yield_before_endStream_then_resume(t *testing.T) {
	f := newBodyBridgeFilter(t)

	// Compile a chunk that defines a top-level function which calls
	// rh:body() then captures the result into a global. Run the chunk
	// inside a fresh coroutine child *LState (mirroring the production
	// dispatch path).
	src := `function consume() captured = rh:body() end`
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v", err)
	}
	if err := f.vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v", err)
	}

	// Mint a child thread + Resume into `consume`. Because no DecodeData
	// has fired yet, :body() yields via YieldFromBridge; Resume returns
	// ResumeYield with the bridge's nil-sentinel.
	child, cancel := f.vm.NewThread()
	if cancel != nil {
		defer cancel()
	}
	fn := f.vm.State().GetGlobal("consume").(*lua.LFunction)

	// Stash the child *LState in the filter's pending-body-resume slot.
	// The production dispatch path does this inside :body() before yielding;
	// our test pre-stashes to mirror.
	f.pendingBodyResume = child

	state, rerr, _ := f.vm.Resume(child, fn)
	if rerr != nil {
		t.Fatalf("Resume[1] err = %v; want nil", rerr)
	}
	if state != lua.ResumeYield {
		t.Fatalf("Resume[1] state = %v; want ResumeYield (body not yet ready)", state)
	}

	// Drive DecodeData with the body bytes + endStream=true. The
	// production filter callback resumes the pending coroutine with the
	// accumulated bytes; the script's `local body = rh:body()` evaluates
	// to that resumed value.
	f.DecodeData([]byte("late-body"), true)

	// Verify the script captured the resumed body bytes.
	captured := f.vm.State().GetGlobal("captured")
	if captured.String() != "late-body" {
		t.Fatalf("captured = %q; want %q", captured.String(), "late-body")
	}
}

// -----------------------------------------------------------------------
// Test 5: defensive copy verified — mutating f.decodedBodyBytes after
// :body() returns must NOT affect the Lua string previously returned.
// -----------------------------------------------------------------------

func Test_RequestHandleBody_defensive_copy_verified(t *testing.T) {
	f := newBodyBridgeFilter(t)
	f.DecodeData([]byte("original-bytes"), true)
	runBodyScript(t, f, `result = rh:body()`)

	// Now mutate the Go-side byte slice in place (zero it out).
	for i := range f.decodedBodyBytes {
		f.decodedBodyBytes[i] = 'X'
	}

	// The Lua string captured earlier MUST still read the original bytes
	// per the §11.3 D3 defensive-copy discipline (gopher-lua's LString is
	// a Go string — immutable by Go semantics — so the defensive copy at
	// LString(string(b)) site detaches Lua from the underlying byte slice
	// lifetime).
	got := getStrGlobal(t, f, "result")
	if got != "original-bytes" {
		t.Fatalf("post-mutation Lua string = %q; want %q (defensive copy contract violated)", got, "original-bytes")
	}
}

// -----------------------------------------------------------------------
// Test 6: body_buffered_bytes_total counter increments correctly.
// -----------------------------------------------------------------------

func Test_body_buffered_bytes_total_counter_increments(t *testing.T) {
	f := newBodyBridgeFilter(t)
	before := f.cc.stats.bodyBufferedBytesTotal.Load()
	f.DecodeData([]byte("12345"), false)      // 5 bytes
	f.DecodeData([]byte("67890ABCDEF"), true) // 11 bytes → total 16
	after := f.cc.stats.bodyBufferedBytesTotal.Load()
	delta := after - before
	const wantDelta = 16
	if delta != wantDelta {
		t.Fatalf("body_buffered_bytes_total delta = %d; want %d", delta, wantDelta)
	}
}

// -----------------------------------------------------------------------
// Test 7: coroutine_yields_total counter increments ONCE per yield event
// (NOT per Resume) per Task 7 semantics.
// -----------------------------------------------------------------------

func Test_coroutine_yields_total_counter_increments(t *testing.T) {
	f := newBodyBridgeFilter(t)

	src := `function consume() captured = rh:body() end`
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v", err)
	}
	if err := f.vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v", err)
	}

	before := f.cc.stats.coroutineYieldsTotal.Load()

	child, cancel := f.vm.NewThread()
	if cancel != nil {
		defer cancel()
	}
	fn := f.vm.State().GetGlobal("consume").(*lua.LFunction)
	f.pendingBodyResume = child
	state, rerr, _ := f.vm.Resume(child, fn)
	if rerr != nil || state != lua.ResumeYield {
		t.Fatalf("Resume[1] state=%v err=%v; want (ResumeYield, nil)", state, rerr)
	}
	// Resume to completion; counter should NOT increment again on resume.
	f.DecodeData([]byte("body-payload"), true)

	after := f.cc.stats.coroutineYieldsTotal.Load()
	delta := after - before
	if delta != 1 {
		t.Fatalf("coroutine_yields_total delta = %d; want 1 (ONCE per yield event)", delta)
	}
}

// -----------------------------------------------------------------------
// Test 8: response-side :body() symmetric.
// -----------------------------------------------------------------------

func Test_ResponseHandleBody_symmetric(t *testing.T) {
	f := newBodyBridgeFilter(t)
	f.EncodeData([]byte("encoded-"), false)
	f.EncodeData([]byte("response"), true)
	runBodyScript(t, f, `result = resp:body()`)
	got := getStrGlobal(t, f, "result")
	if got != "encoded-response" {
		t.Fatalf("resp:body() = %q; want %q", got, "encoded-response")
	}
}
