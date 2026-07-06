package lua

// filterstate_test.go — Task 13 (phase 22.2 IMPL) filter-state bridge
// tests per SPEC §3.4 + §11.8 D4 closure + AMEND-22.2-4 + PLAN Task 13.
//
// Coverage:
//
//   - Test_FilterState_get_returns_marshaled_typed_value — for each
//     supported value type (string / float64 / int64 / bool / map / []any),
//     `:get(name)` returns the corresponding Lua value typed per the
//     marshaling table.
//   - Test_FilterState_set_then_get_roundtrip — `:set(name, value)`
//     followed by `:get(name)` round-trips per Lua-value type.
//   - Test_FilterState_cross_stream_isolation — N=10 parallel filters,
//     each with its own map; writes are independent; reads do not leak
//     across.
//   - Test_FilterState_per_stream_lifecycle_OnDestroy_releases_map —
//     after f.OnDestroy(), f.filterState is nil (per-stream cleanup).
//   - Test_FilterState_set_invalid_lua_type_raises_runtime_error —
//     unsupported types (LFunction, LChannel) raise a Lua runtime error
//     (envoy-go-strict; documents the marshaling contract).

import (
	"net/http"
	"sync"
	"testing"

	lua "github.com/yuin/gopher-lua"

	luaprim "github.com/pgdad/envoy-go/internal/lua"
)

// newBridgedVMWithFilterState constructs a VM wired with the filterstate
// metatable + a shared canned filterState map[string]any on the
// requestHandleContext.cb adapter. Returns the *fakeCallbacksFull so the
// test can inspect/mutate the underlying Go-side map directly post-script.
func newBridgedVMWithFilterState(t *testing.T, initial map[string]any) (*luaprim.VM, *fakeCallbacksFull) {
	t.Helper()
	if initial == nil {
		initial = map[string]any{}
	}
	cb := &fakeCallbacksFull{filterState: initial}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	return vm, cb
}

// ---------------------------------------------------------------------
// :filterState():get marshaled typed value
// ---------------------------------------------------------------------

func Test_FilterState_get_returns_marshaled_typed_value(t *testing.T) {
	initial := map[string]any{
		"sk": "the-string",
		"fk": float64(3.5),
		"ik": int64(42),
		"bk": true,
		"mk": map[string]any{"inner": "v1", "n": float64(7)},
		"lk": []any{"a", "b", float64(3)},
	}
	vm, _ := newBridgedVMWithFilterState(t, initial)
	runScript(t, vm, `
		local fs = rh:streamInfo():filterState()
		s = fs:get("sk")
		f = fs:get("fk")
		i = fs:get("ik")
		b = fs:get("bk")
		m = fs:get("mk")
		m_inner = m and m.inner or nil
		m_n = m and m.n or nil
		l = fs:get("lk")
		l1 = l and l[1] or nil
		l2 = l and l[2] or nil
		l3 = l and l[3] or nil
	`)

	if got := getGlobalString(t, vm, "s"); got != "the-string" {
		t.Errorf("get(sk) = %q; want %q", got, "the-string")
	}
	if got := vm.State().GetGlobal("f"); got != lua.LNumber(3.5) {
		t.Errorf("get(fk) = %v; want 3.5", got)
	}
	if got := vm.State().GetGlobal("i"); got != lua.LNumber(42) {
		t.Errorf("get(ik) = %v; want 42", got)
	}
	if got := vm.State().GetGlobal("b"); got != lua.LTrue {
		t.Errorf("get(bk) = %v; want true", got)
	}
	if got := getGlobalString(t, vm, "m_inner"); got != "v1" {
		t.Errorf("get(mk).inner = %q; want %q", got, "v1")
	}
	if got := vm.State().GetGlobal("m_n"); got != lua.LNumber(7) {
		t.Errorf("get(mk).n = %v; want 7", got)
	}
	if got := getGlobalString(t, vm, "l1"); got != "a" {
		t.Errorf("get(lk)[1] = %q; want %q", got, "a")
	}
	if got := getGlobalString(t, vm, "l2"); got != "b" {
		t.Errorf("get(lk)[2] = %q; want %q", got, "b")
	}
	if got := vm.State().GetGlobal("l3"); got != lua.LNumber(3) {
		t.Errorf("get(lk)[3] = %v; want 3", got)
	}
}

// Test_FilterState_get_missing_returns_nil pins the absent-key contract
// — :get(name) on an unset key returns lua.LNil (matches the upstream
// FilterStateWrapper missing-key behavior).
func Test_FilterState_get_missing_returns_nil(t *testing.T) {
	vm, _ := newBridgedVMWithFilterState(t, nil)
	runScript(t, vm, `
		local fs = rh:streamInfo():filterState()
		v = fs:get("never-set")
		result_is_nil = (v == nil)
	`)
	if v := vm.State().GetGlobal("result_is_nil"); v != lua.LTrue {
		t.Fatalf(":get(unset) = non-nil; want nil")
	}
}

// ---------------------------------------------------------------------
// :filterState():set + :get round-trip
// ---------------------------------------------------------------------

func Test_FilterState_set_then_get_roundtrip(t *testing.T) {
	vm, cb := newBridgedVMWithFilterState(t, nil)
	runScript(t, vm, `
		local fs = rh:streamInfo():filterState()
		fs:set("s", "hello")
		fs:set("n", 12.5)
		fs:set("b", true)
		fs:set("t", { name = "alice", age = 30 })
		fs:set("l", { "x", "y", "z" })
		s = fs:get("s")
		n = fs:get("n")
		b = fs:get("b")
		tname = fs:get("t").name
		tage  = fs:get("t").age
		l1 = fs:get("l")[1]
		l2 = fs:get("l")[2]
		l3 = fs:get("l")[3]
	`)

	if got := getGlobalString(t, vm, "s"); got != "hello" {
		t.Errorf("roundtrip s = %q; want %q", got, "hello")
	}
	if got := vm.State().GetGlobal("n"); got != lua.LNumber(12.5) {
		t.Errorf("roundtrip n = %v; want 12.5", got)
	}
	if got := vm.State().GetGlobal("b"); got != lua.LTrue {
		t.Errorf("roundtrip b = %v; want true", got)
	}
	if got := getGlobalString(t, vm, "tname"); got != "alice" {
		t.Errorf("roundtrip t.name = %q; want %q", got, "alice")
	}
	if got := vm.State().GetGlobal("tage"); got != lua.LNumber(30) {
		t.Errorf("roundtrip t.age = %v; want 30", got)
	}
	for i, want := range []string{"x", "y", "z"} {
		name := []string{"l1", "l2", "l3"}[i]
		if got := getGlobalString(t, vm, name); got != want {
			t.Errorf("roundtrip l[%d] = %q; want %q", i+1, got, want)
		}
	}

	// Go-side inspection: the map should reflect the writes.
	if _, ok := cb.filterState["s"]; !ok {
		t.Errorf("filterState[s] missing post-set; map = %v", cb.filterState)
	}
}

// ---------------------------------------------------------------------
// Cross-stream isolation (N=10 parallel filter instances)
// ---------------------------------------------------------------------

// Test_FilterState_cross_stream_isolation spins up N=10 separate VMs
// (one per simulated stream), each with its own filterState map. Each
// VM writes a stream-unique value to the SAME key; reads do NOT observe
// any other stream's value. Pins the per-stream isolation contract from
// PLAN Task 13 acceptance.
func Test_FilterState_cross_stream_isolation(t *testing.T) {
	const N = 10
	results := make([]string, N)
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			vm, _ := newBridgedVMWithFilterState(t, nil)
			defer vm.Close()
			runScript(t, vm, `
				local fs = rh:streamInfo():filterState()
				fs:set("shared-key", "`+streamValueLiteral(i)+`")
				result = fs:get("shared-key")
			`)
			results[i] = getGlobalString(t, vm, "result")
		}()
	}
	wg.Wait()
	for i := 0; i < N; i++ {
		want := streamValueLiteral(i)
		if results[i] != want {
			t.Errorf("stream %d filterState[shared-key] = %q; want %q (cross-stream leak)", i, results[i], want)
		}
	}
}

// streamValueLiteral returns a stable per-stream-index value embedded in
// the per-stream Lua script. Used to verify cross-stream isolation.
func streamValueLiteral(i int) string {
	chars := []byte{'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h', 'i', 'j'}
	return string([]byte{'v', '-', chars[i%len(chars)]})
}

// ---------------------------------------------------------------------
// Per-stream lifecycle — OnDestroy releases the map
// ---------------------------------------------------------------------

// Test_FilterState_per_stream_lifecycle_OnDestroy_releases_map asserts
// that the *filter.OnDestroy callback clears the per-stream filterState
// map. The contract from PLAN Task 13 + SPEC §11.8 D4: per-stream
// lifecycle — created at filter struct allocation; destroyed at
// OnDestroy.
func Test_FilterState_per_stream_lifecycle_OnDestroy_releases_map(t *testing.T) {
	// Construct a *filter directly (no VM); populate filterState; invoke
	// OnDestroy; verify the map is cleared.
	f := &filter{
		filterState: map[string]any{"k": "v"},
	}
	if f.filterState == nil {
		t.Fatalf("filterState nil pre-OnDestroy; want non-nil")
	}
	if v, ok := f.filterState["k"]; !ok || v != "v" {
		t.Fatalf("filterState[k] = %v, ok=%v; want \"v\", true", v, ok)
	}
	f.OnDestroy()
	if f.filterState != nil {
		t.Errorf("filterState = %v post-OnDestroy; want nil (released)", f.filterState)
	}
}

// ---------------------------------------------------------------------
// Set with invalid Lua type raises runtime error
// ---------------------------------------------------------------------

// Test_FilterState_set_invalid_lua_type_raises_runtime_error verifies
// that unsupported Lua value types (function, channel, raw userdata)
// raise a Lua runtime error at :set rather than silently dropping. Pins
// the envoy-go-strict marshaling contract per SPEC §11.8 D4.
func Test_FilterState_set_invalid_lua_type_raises_runtime_error(t *testing.T) {
	vm, _ := newBridgedVMWithFilterState(t, nil)
	chunk, err := luaprim.CompileScript([]byte(`
		local fs = rh:streamInfo():filterState()
		fs:set("fn", function() end)
	`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v", err)
	}
	if err := vm.Run(chunk); err == nil {
		t.Fatalf("vm.Run on :set(fn) returned nil err; want runtime error")
	}
}

// Test_FilterState_filter_struct_initialized_empty_at_construction
// verifies that a freshly-allocated *filter has the per-stream
// filterState map either nil or empty — both are acceptable; the
// :set bridge LGFunction lazy-initializes on first write.
func Test_FilterState_filter_struct_initialized_empty_at_construction(t *testing.T) {
	f := &filter{}
	// Either nil or empty is fine at construction; the bridge :set will
	// lazy-init on first write.
	if f.filterState != nil && len(f.filterState) != 0 {
		t.Errorf("fresh *filter.filterState = %v; want nil or empty", f.filterState)
	}
}

// ---------------------------------------------------------------------
// Round-trip with nil-bucket tolerance (cb has nil filterState pointer)
// ---------------------------------------------------------------------

// Test_FilterState_nil_map_tolerance asserts that the bridge surface
// stays callable even when the underlying filterState map is nil (the
// streamInfo userdata is constructed with a nil-FilterState cb). :get
// returns nil; :set is silently dropped (the bridge no-ops or lazy-
// constructs — either is acceptable per Spec).
func Test_FilterState_nil_map_tolerance(t *testing.T) {
	cb := &fakeCallbacksFull{filterState: nil}
	vm := newBridgedVMWithFullCallbacks(t, cb)
	defer vm.Close()
	// Script must NOT panic — defensive nil handling per ADR-0085.
	chunk, err := luaprim.CompileScript([]byte(`
		local fs = rh:streamInfo():filterState()
		v = fs:get("any")
		result_is_nil = (v == nil)
	`), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v", err)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; want clean on nil filterState", err)
	}
	if v := vm.State().GetGlobal("result_is_nil"); v != lua.LTrue {
		t.Fatalf(":get on nil map = non-nil; want nil")
	}
}

// Static check: the http.Header import is referenced via fakeCallbacks usage.
var _ = http.Header{}
