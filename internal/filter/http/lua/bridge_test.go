package lua

// bridge_test.go — Task 6 IMPL behavioral tests for the bridge surface
// per 22.1 PLAN Task 6 + 22.1 SPEC §11.2 D7 resolution + parent §11.2
// upstream-parity headers-method semantics.
//
// Task 6 contribution: request_handle/response_handle userdata +
// metatable setup + 7 headers-object methods (:get / :getAtIndex /
// :getNumValues / :add / :append / :remove / :replace) + __pairs
// metamethod (alphabetical-snapshot per §11.2 D7) + cross-run
// determinism N=100 verification.
//
// Later tasks extend this file:
//   - Task 7 — 6 :logXxx method tests
//   - Task 8 — 4-method :streamInfo() subset tests
//   - Task 9 — :respond byte-pin + AMEND-8 encode-side runtime-reject tests

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
	luaprim "github.com/esalaine/envoy-go/internal/lua"
)

// newBridgedVM is the per-test helper that constructs a fresh *VM with
// the bridge metatables installed + a request_handle userdata wrapping
// the supplied http.Header bound to the Lua global `rh`. Test scripts
// access the headers via `rh:headers()` and observe results via Lua
// globals (then read back via vm.State().GetGlobal(name) on the Go side).
//
// The helper:
//  1. Constructs a default-sandbox VM via luaprim.NewVM.
//  2. Installs the request_handle + envoy_headers metatables.
//  3. Installs the pairs-shim that honors __pairs on userdata (since
//     gopher-lua's basePairs requires LTable and does NOT auto-dispatch
//     __pairs — see installPairsShim docstring at bridge.go).
//  4. Builds a *requestHandleContext + LUserData wrapping it.
//  5. Binds the userdata to the Lua global `rh`.
//
// t.Cleanup registers vm.Close.
func newBridgedVM(t *testing.T, h http.Header) *luaprim.VM {
	t.Helper()
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installPairsShim(L)
	ctx := &requestHandleContext{headers: h}
	ud := L.NewUserData()
	ud.Value = ctx
	L.SetMetatable(ud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", ud)
	return vm
}

// runScript compiles + runs the supplied Lua source on the VM; fails
// the test on either compile or runtime error.
func runScript(t *testing.T, vm *luaprim.VM, src string) {
	t.Helper()
	chunk, err := luaprim.CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript err = %v; src = %q", err, src)
	}
	if err := vm.Run(chunk); err != nil {
		t.Fatalf("vm.Run err = %v; src = %q", err, src)
	}
}

// getGlobalString fetches a Lua global as a string; fails on nil or
// non-string.
func getGlobalString(t *testing.T, vm *luaprim.VM, name string) string {
	t.Helper()
	v := vm.State().GetGlobal(name)
	if v == lua.LNil {
		t.Fatalf("global %q = nil; want string", name)
	}
	s, ok := v.(lua.LString)
	if !ok {
		t.Fatalf("global %q type = %s; want string (got %v)", name, v.Type(), v)
	}
	return string(s)
}

// getGlobalInt fetches a Lua global as an int; fails on non-number.
func getGlobalInt(t *testing.T, vm *luaprim.VM, name string) int {
	t.Helper()
	v := vm.State().GetGlobal(name)
	n, ok := v.(lua.LNumber)
	if !ok {
		t.Fatalf("global %q type = %s; want number", name, v.Type())
	}
	return int(n)
}

// isGlobalNil reports whether the named global is nil.
func isGlobalNil(vm *luaprim.VM, name string) bool {
	return vm.State().GetGlobal(name) == lua.LNil
}

// ----------------------------------------------------------------------
// :get — first-value / nil-on-absent
// ----------------------------------------------------------------------

// TestBridge_Headers_Get_Hit verifies :get returns the first value for
// an existing header (case-canonical lookup via http.Header.Get).
func TestBridge_Headers_Get_Hit(t *testing.T) {
	h := http.Header{"X-Foo": []string{"bar"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():get("X-Foo")`)
	if got := getGlobalString(t, vm, "result"); got != "bar" {
		t.Fatalf(":get(\"X-Foo\") = %q; want %q", got, "bar")
	}
}

// TestBridge_Headers_Get_Miss verifies :get returns nil for an absent
// header (vs. the empty string returned by http.Header.Get on absent).
func TestBridge_Headers_Get_Miss(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():get("X-Absent")`)
	if !isGlobalNil(vm, "result") {
		v := vm.State().GetGlobal("result")
		t.Fatalf(":get(\"X-Absent\") = %v; want nil", v)
	}
}

// TestBridge_Headers_Get_FirstValueOfMulti verifies :get returns the
// FIRST value when a header is multi-valued (matches upstream
// HeaderMapWrapper::luaGet first-value semantics).
func TestBridge_Headers_Get_FirstValueOfMulti(t *testing.T) {
	h := http.Header{"X-Multi": []string{"alpha", "beta", "gamma"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():get("X-Multi")`)
	if got := getGlobalString(t, vm, "result"); got != "alpha" {
		t.Fatalf(":get(\"X-Multi\") = %q; want %q", got, "alpha")
	}
}

// ----------------------------------------------------------------------
// :getAtIndex — 1-indexed N-th value / nil-on-out-of-range
// ----------------------------------------------------------------------

// TestBridge_Headers_GetAtIndex_FirstValue verifies the 1-indexed
// first value matches the underlying slice's [0] entry.
func TestBridge_Headers_GetAtIndex_FirstValue(t *testing.T) {
	h := http.Header{"X-Multi": []string{"alpha", "beta"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():getAtIndex("X-Multi", 1)`)
	if got := getGlobalString(t, vm, "result"); got != "alpha" {
		t.Fatalf(":getAtIndex(\"X-Multi\", 1) = %q; want %q", got, "alpha")
	}
}

// TestBridge_Headers_GetAtIndex_SecondValue verifies the 1-indexed
// second value matches the underlying slice's [1] entry.
func TestBridge_Headers_GetAtIndex_SecondValue(t *testing.T) {
	h := http.Header{"X-Multi": []string{"alpha", "beta"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():getAtIndex("X-Multi", 2)`)
	if got := getGlobalString(t, vm, "result"); got != "beta" {
		t.Fatalf(":getAtIndex(\"X-Multi\", 2) = %q; want %q", got, "beta")
	}
}

// TestBridge_Headers_GetAtIndex_OutOfRange verifies an index past the
// last value returns nil.
func TestBridge_Headers_GetAtIndex_OutOfRange(t *testing.T) {
	h := http.Header{"X-Multi": []string{"alpha"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():getAtIndex("X-Multi", 5)`)
	if !isGlobalNil(vm, "result") {
		v := vm.State().GetGlobal("result")
		t.Fatalf(":getAtIndex out-of-range = %v; want nil", v)
	}
}

// TestBridge_Headers_GetAtIndex_ZeroIndex verifies that index 0 (below
// Lua's 1-indexed convention) returns nil.
func TestBridge_Headers_GetAtIndex_ZeroIndex(t *testing.T) {
	h := http.Header{"X-Multi": []string{"alpha"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():getAtIndex("X-Multi", 0)`)
	if !isGlobalNil(vm, "result") {
		v := vm.State().GetGlobal("result")
		t.Fatalf(":getAtIndex(0) = %v; want nil (1-indexed)", v)
	}
}

// TestBridge_Headers_GetAtIndex_Absent verifies that an absent header
// name returns nil regardless of index.
func TestBridge_Headers_GetAtIndex_Absent(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():getAtIndex("X-Absent", 1)`)
	if !isGlobalNil(vm, "result") {
		v := vm.State().GetGlobal("result")
		t.Fatalf(":getAtIndex absent-header = %v; want nil", v)
	}
}

// ----------------------------------------------------------------------
// :getNumValues — count
// ----------------------------------------------------------------------

// TestBridge_Headers_GetNumValues_Multi verifies the count of values
// for a multi-valued header matches the underlying slice length.
func TestBridge_Headers_GetNumValues_Multi(t *testing.T) {
	h := http.Header{"X-Multi": []string{"a", "b", "c"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():getNumValues("X-Multi")`)
	if got := getGlobalInt(t, vm, "result"); got != 3 {
		t.Fatalf(":getNumValues(\"X-Multi\") = %d; want 3", got)
	}
}

// TestBridge_Headers_GetNumValues_Single verifies single-value count = 1.
func TestBridge_Headers_GetNumValues_Single(t *testing.T) {
	h := http.Header{"X-One": []string{"v"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():getNumValues("X-One")`)
	if got := getGlobalInt(t, vm, "result"); got != 1 {
		t.Fatalf(":getNumValues = %d; want 1", got)
	}
}

// TestBridge_Headers_GetNumValues_Absent verifies an absent header
// returns 0 (NOT nil — count semantics).
func TestBridge_Headers_GetNumValues_Absent(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `result = rh:headers():getNumValues("X-Absent")`)
	if got := getGlobalInt(t, vm, "result"); got != 0 {
		t.Fatalf(":getNumValues absent = %d; want 0", got)
	}
}

// ----------------------------------------------------------------------
// :add — appends (does NOT replace)
// ----------------------------------------------------------------------

// TestBridge_Headers_Add_Appends verifies :add appends a new value,
// preserving any existing values (multi-value preservation via
// http.Header.Add).
func TestBridge_Headers_Add_Appends(t *testing.T) {
	h := http.Header{"X-Multi": []string{"first"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:headers():add("X-Multi", "second")`)
	got := h.Values("X-Multi")
	want := []string{"first", "second"}
	if !equalSlices(got, want) {
		t.Fatalf(":add did not append; got %v; want %v", got, want)
	}
}

// TestBridge_Headers_Add_NewName verifies :add on a brand-new header
// name creates the entry with a single value.
func TestBridge_Headers_Add_NewName(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:headers():add("X-New", "hello")`)
	if got := h.Get("X-New"); got != "hello" {
		t.Fatalf(":add new-name got = %q; want %q", got, "hello")
	}
}

// ----------------------------------------------------------------------
// :append — alias for :add (per upstream Envoy wrappers.cc semantics)
// ----------------------------------------------------------------------

// TestBridge_Headers_Append_Alias verifies :append behaves identically
// to :add — both append values without replacing existing ones.
func TestBridge_Headers_Append_Alias(t *testing.T) {
	h := http.Header{"X-Multi": []string{"first"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:headers():append("X-Multi", "second")`)
	got := h.Values("X-Multi")
	want := []string{"first", "second"}
	if !equalSlices(got, want) {
		t.Fatalf(":append did not append; got %v; want %v", got, want)
	}
}

// ----------------------------------------------------------------------
// :remove — deletes all values
// ----------------------------------------------------------------------

// TestBridge_Headers_Remove_Deletes verifies :remove deletes the entire
// header entry (all values, not just one).
func TestBridge_Headers_Remove_Deletes(t *testing.T) {
	h := http.Header{"X-Multi": []string{"a", "b", "c"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:headers():remove("X-Multi")`)
	if got := h.Values("X-Multi"); len(got) != 0 {
		t.Fatalf(":remove did not delete all; got %v; want []", got)
	}
}

// TestBridge_Headers_Remove_Absent verifies removing an absent header
// is a no-op (matches http.Header.Del semantics).
func TestBridge_Headers_Remove_Absent(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:headers():remove("X-Absent")`)
	if len(h) != 0 {
		t.Fatalf(":remove absent header left state = %v; want empty", h)
	}
}

// ----------------------------------------------------------------------
// :replace — removes-then-adds (single value)
// ----------------------------------------------------------------------

// TestBridge_Headers_Replace_RemovesThenAdds verifies :replace replaces
// a multi-value header with a single value (matches http.Header.Set).
func TestBridge_Headers_Replace_RemovesThenAdds(t *testing.T) {
	h := http.Header{"User-Agent": []string{"old1", "old2"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:headers():replace("User-Agent", "envoy-go-lua/1.0")`)
	got := h.Values("User-Agent")
	want := []string{"envoy-go-lua/1.0"}
	if !equalSlices(got, want) {
		t.Fatalf(":replace got = %v; want %v", got, want)
	}
}

// TestBridge_Headers_Replace_NewName verifies :replace on a non-existent
// header name behaves like a single :add (matches http.Header.Set).
func TestBridge_Headers_Replace_NewName(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:headers():replace("X-New", "value")`)
	if got := h.Get("X-New"); got != "value" {
		t.Fatalf(":replace new-name got = %q; want %q", got, "value")
	}
}

// ----------------------------------------------------------------------
// Case-insensitive lookup (HTTP-spec convention; via http.Header
// CanonicalHeaderKey)
// ----------------------------------------------------------------------

// TestBridge_Headers_CaseInsensitiveLookup verifies :get is case-
// insensitive: "X-Foo" and "x-foo" return the same value (matches
// upstream LowerCaseString-keyed HeaderMap semantics + Go's
// http.CanonicalHeaderKey discipline).
func TestBridge_Headers_CaseInsensitiveLookup(t *testing.T) {
	h := http.Header{"X-Foo": []string{"bar"}}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `
		upper = rh:headers():get("X-Foo")
		lower = rh:headers():get("x-foo")
		mixed = rh:headers():get("X-fOo")
	`)
	for _, name := range []string{"upper", "lower", "mixed"} {
		if got := getGlobalString(t, vm, name); got != "bar" {
			t.Fatalf("case-insensitive %s = %q; want %q", name, got, "bar")
		}
	}
}

// ----------------------------------------------------------------------
// __pairs — alphabetical-snapshot iterator per §11.2 D7
// ----------------------------------------------------------------------

// TestBridge_Pairs_AlphabeticalOrder verifies that iterating the
// headers via `for k,v in pairs(rh:headers()) do ... end` walks the
// keys in alphabetical (case-insensitive) order.
func TestBridge_Pairs_AlphabeticalOrder(t *testing.T) {
	h := http.Header{
		"Zeta":  []string{"3"},
		"Alpha": []string{"1"},
		"Mango": []string{"2"},
	}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `
		order = ""
		for k, v in pairs(rh:headers()) do
			order = order .. k .. "=" .. v .. ";"
		end
	`)
	got := getGlobalString(t, vm, "order")
	want := "Alpha=1;Mango=2;Zeta=3;"
	if got != want {
		t.Fatalf("__pairs order = %q; want %q", got, want)
	}
}

// TestBridge_Pairs_CrossRunDeterminism verifies the __pairs iteration
// order is byte-identical across N=100 runs of the same headers map.
// Closes the §11 D7 cross-run-determinism RATIFIED-PENDING item.
func TestBridge_Pairs_CrossRunDeterminism(t *testing.T) {
	const N = 100
	// Use enough distinct keys + values to make a non-deterministic
	// implementation surface non-deterministic output quickly (Go's
	// map randomization is per-iterator-instance + per-process — N=100
	// samples per test run is a thorough probe).
	h := http.Header{
		"a-key": []string{"av"},
		"b-key": []string{"bv"},
		"c-key": []string{"cv"},
		"d-key": []string{"dv"},
		"e-key": []string{"ev"},
		"f-key": []string{"fv"},
		"g-key": []string{"gv"},
		"h-key": []string{"hv"},
	}
	var first string
	for i := 0; i < N; i++ {
		vm := newBridgedVM(t, h)
		runScript(t, vm, `
			out = ""
			for k, v in pairs(rh:headers()) do
				out = out .. k .. "=" .. v .. ";"
			end
		`)
		got := getGlobalString(t, vm, "out")
		if i == 0 {
			first = got
			continue
		}
		if got != first {
			t.Fatalf("__pairs iteration %d differs from first; got=%q first=%q", i, got, first)
		}
	}
	// Sanity check — ensure the iteration actually visited all keys.
	if !strings.Contains(first, "a-key=av;") || !strings.Contains(first, "h-key=hv;") {
		t.Fatalf("first iteration output missing expected entries; got %q", first)
	}
}

// TestBridge_Pairs_MultiValueSameKey verifies that multi-value headers
// surface as one (k, v) pair per value, ordered alphabetically by key
// then lexicographically by value.
func TestBridge_Pairs_MultiValueSameKey(t *testing.T) {
	h := http.Header{
		"X-Multi": []string{"v1", "v2", "v3"},
		"X-One":   []string{"only"},
	}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `
		out = ""
		for k, v in pairs(rh:headers()) do
			out = out .. k .. "=" .. v .. ";"
		end
	`)
	got := getGlobalString(t, vm, "out")
	want := "X-Multi=v1;X-Multi=v2;X-Multi=v3;X-One=only;"
	if got != want {
		t.Fatalf("__pairs multi-value = %q; want %q", got, want)
	}
}

// TestBridge_Pairs_Empty verifies iterating an empty headers map
// produces an empty output (no panic, no spurious iterations).
func TestBridge_Pairs_Empty(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `
		count = 0
		for k, v in pairs(rh:headers()) do
			count = count + 1
		end
	`)
	if got := getGlobalInt(t, vm, "count"); got != 0 {
		t.Fatalf("__pairs empty count = %d; want 0", got)
	}
}

// TestBridge_Pairs_AgainstReferenceSort verifies the bridge __pairs
// order matches a Go-side alphabetical sort of the same keys (the
// reference order used by `net/http.Header.Write`).
func TestBridge_Pairs_AgainstReferenceSort(t *testing.T) {
	h := http.Header{
		"X-Charlie": []string{"c"},
		"X-Alpha":   []string{"a"},
		"X-Bravo":   []string{"b"},
	}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `
		keys = {}
		for k, v in pairs(rh:headers()) do
			keys[#keys + 1] = k
		end
		joined = table.concat(keys, ",")
	`)
	got := getGlobalString(t, vm, "joined")
	// Reference sort: alphabetical case-insensitive.
	keys := make([]string, 0, len(h))
	for k := range h {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
	})
	want := strings.Join(keys, ",")
	if got != want {
		t.Fatalf("__pairs order vs reference sort: got %q; want %q", got, want)
	}
}

// ----------------------------------------------------------------------
// Helper: equalSlices for cmp-free deep-equal on []string
// ----------------------------------------------------------------------

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ----------------------------------------------------------------------
// Task 7 — 6 :logXxx methods (logTrace/logDebug/logInfo/logWarn/logErr/
// logCritical) per parent §7.1 + 22.1 PLAN/SPEC Task 7
// ----------------------------------------------------------------------
//
// All 6 methods wrap the Go stdlib "log" package at the corresponding
// log level. Format pin: "<LEVEL> lua: <msg>" prefix preserved across
// all 6 levels for log-greppability. Conservative coalesce of logTrace
// onto DEBUG (stdlib log has no native levels) per PLAN Task 7.
//
// Test discipline: capture log output via log.SetOutput(buf) +
// log.SetFlags(0) (suppress timestamp prefix for byte-exact assertion).
// Restore both in defer. The shared logAtLevel helper makes the request-
// handle and response-handle paths byte-identical for any given level.

// withCapturedLog runs fn with the stdlib log sink redirected to a
// buffer (timestamp flags suppressed for byte-exact assertions). Returns
// the captured bytes. Restores log.Output + log.Flags on completion.
//
// Use within a single sub-test only; the global log sink is process-
// wide so parallel sub-tests using this helper would race.
func withCapturedLog(t *testing.T, fn func()) string {
	t.Helper()
	buf := &bytes.Buffer{}
	origFlags := log.Flags()
	log.SetFlags(0)
	log.SetOutput(buf)
	defer func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(origFlags)
	}()
	fn()
	return buf.String()
}

// newBridgedVMWithResponseHandle constructs a VM with both rh
// (request_handle) AND resp (response_handle) bound as Lua globals.
// Used by the response-handle parity test to verify the same 6 :logXxx
// methods are callable from the encode-side userdata.
func newBridgedVMWithResponseHandle(t *testing.T, h http.Header) *luaprim.VM {
	t.Helper()
	vm := newBridgedVM(t, h) // installs both metatables + binds rh
	L := vm.State()
	rctx := &responseHandleContext{headers: h}
	rud := L.NewUserData()
	rud.Value = rctx
	L.SetMetatable(rud, L.GetTypeMetatable(responseHandleTypeName))
	L.SetGlobal("resp", rud)
	return vm
}

// TestBridge_Log_LevelRouting is the table-driven 6-arm verification
// that each :logXxx method emits the correct "<LEVEL> lua: <msg>" line.
// Trace coalesces onto DEBUG per PLAN Task 7 conservative mapping.
func TestBridge_Log_LevelRouting(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		msg      string
		expected string
	}{
		{"trace", "logTrace", "hi-trace", "DEBUG lua: hi-trace\n"},
		{"debug", "logDebug", "hi-debug", "DEBUG lua: hi-debug\n"},
		{"info", "logInfo", "hi-info", "INFO lua: hi-info\n"},
		{"warn", "logWarn", "hi-warn", "WARN lua: hi-warn\n"},
		{"err", "logErr", "hi-err", "ERROR lua: hi-err\n"},
		{"crit", "logCritical", "hi-crit", "CRIT lua: hi-crit\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := withCapturedLog(t, func() {
				vm := newBridgedVM(t, http.Header{})
				src := fmt.Sprintf(`rh:%s(%q)`, tc.method, tc.msg)
				runScript(t, vm, src)
			})
			if got != tc.expected {
				t.Errorf(":%s log output: got %q, want %q", tc.method, got, tc.expected)
			}
		})
	}
}

// TestBridge_Log_FromResponseHandle verifies the same 6 :logXxx methods
// are callable from the response_handle (encode-side) userdata — script
// authors may want to log from envoy_on_response.
func TestBridge_Log_FromResponseHandle(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		msg      string
		expected string
	}{
		{"trace", "logTrace", "r-trace", "DEBUG lua: r-trace\n"},
		{"debug", "logDebug", "r-debug", "DEBUG lua: r-debug\n"},
		{"info", "logInfo", "r-info", "INFO lua: r-info\n"},
		{"warn", "logWarn", "r-warn", "WARN lua: r-warn\n"},
		{"err", "logErr", "r-err", "ERROR lua: r-err\n"},
		{"crit", "logCritical", "r-crit", "CRIT lua: r-crit\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := withCapturedLog(t, func() {
				vm := newBridgedVMWithResponseHandle(t, http.Header{})
				src := fmt.Sprintf(`resp:%s(%q)`, tc.method, tc.msg)
				runScript(t, vm, src)
			})
			if got != tc.expected {
				t.Errorf("resp:%s log output: got %q, want %q", tc.method, got, tc.expected)
			}
		})
	}
}

// TestBridge_Log_MsgWithFormatSpecifier verifies that a user-supplied
// message containing Printf-style format directives (e.g. "%s %d") is
// emitted verbatim — NOT used as a format string. The bridge IMPL uses
// log.Printf("%s lua: %s", level, msg) — the user msg arrives at the
// %s arg position, so any %verbs in the msg are inert characters in
// the output (no format-string-injection attack surface).
func TestBridge_Log_MsgWithFormatSpecifier(t *testing.T) {
	got := withCapturedLog(t, func() {
		vm := newBridgedVM(t, http.Header{})
		runScript(t, vm, `rh:logInfo("%s %d %v")`)
	})
	want := "INFO lua: %s %d %v\n"
	if got != want {
		t.Errorf("format-specifier-in-msg: got %q, want %q (msg should be inert)", got, want)
	}
}

// TestBridge_Log_ArgRequired verifies that calling a log method without
// a message argument is rejected with a Lua-side error (CheckString
// fails on missing arg). The Run wrapper must surface the error.
func TestBridge_Log_ArgRequired(t *testing.T) {
	// Suppress log output during the test (in case the impl somehow
	// emits anything before the error) — we only care about err.
	vm := newBridgedVM(t, http.Header{})
	buf := &bytes.Buffer{}
	origFlags := log.Flags()
	log.SetFlags(0)
	log.SetOutput(buf)
	defer func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(origFlags)
	}()
	chunk, cerr := luaprim.CompileScript([]byte(`rh:logInfo()`), nil)
	if cerr != nil {
		t.Fatalf("CompileScript err = %v", cerr)
	}
	if err := vm.Run(chunk); err == nil {
		t.Fatalf("rh:logInfo() with no arg: got nil err; want Lua-side bad-argument error")
	}
}

// TestBridge_Log_EmptyMsg verifies an empty-string message emits
// "<LEVEL> lua: \n" — the format pin is preserved even for empty input
// (no special-casing).
func TestBridge_Log_EmptyMsg(t *testing.T) {
	got := withCapturedLog(t, func() {
		vm := newBridgedVM(t, http.Header{})
		runScript(t, vm, `rh:logWarn("")`)
	})
	want := "WARN lua: \n"
	if got != want {
		t.Errorf("empty-msg: got %q, want %q", got, want)
	}
}

// ----------------------------------------------------------------------
// Task 8 — :streamInfo() 4-method subset per parent §11.6 pragmatic-middle
// BRAINSTORM Q6 + 22.1 SPEC §6 Task 8.
// ----------------------------------------------------------------------
//
// `request_handle:streamInfo()` returns a streamInfo userdata exposing 4
// accessor methods sourced from a small `RequestHandleCallbacks`
// interface implemented by the framework's DecoderFilterCallbacks
// adapter (Task 9 wires the real adapter; tests here use a test-double
// `fakeCallbacks`).
//
// Methods:
//   :protocol()                      — "HTTP/1.0" / "HTTP/1.1" / "HTTP/2" / "HTTP/3"
//   :routeName()                     — resolved route name (may be "")
//   :downstreamLocalAddress()        — "ip:port" formatted local addr
//   :downstreamDirectRemoteAddress() — "ip:port" formatted remote addr
//
// Test discipline: a per-test `fakeCallbacks` carries canned values, the
// test-helper `newBridgedVMWithCallbacks` constructs a VM with the
// streamInfo metatable installed AND wires the test-double into the
// requestHandleContext.cb field, then the script `result = rh:streamInfo():<method>()`
// surfaces the value via the `result` Lua global.

// fakeCallbacks is a test-double satisfying RequestHandleCallbacks for
// Task 8. Each field maps 1:1 to a method on the interface; tests
// construct an instance with the canned values they want surfaced.
type fakeCallbacks struct {
	proto      string
	route      string
	localAddr  string
	remoteAddr string
}

func (f *fakeCallbacks) Protocol() string                      { return f.proto }
func (f *fakeCallbacks) RouteName() string                     { return f.route }
func (f *fakeCallbacks) DownstreamLocalAddress() string        { return f.localAddr }
func (f *fakeCallbacks) DownstreamDirectRemoteAddress() string { return f.remoteAddr }

// newBridgedVMWithCallbacks is the per-test helper that constructs a VM
// with the streamInfo metatable installed in addition to the request_handle
// + response_handle + headers metatables (per newBridgedVM), AND wires the
// supplied RequestHandleCallbacks into the requestHandleContext.cb field
// so :streamInfo() can surface the canned values via the 4 accessor methods.
func newBridgedVMWithCallbacks(t *testing.T, h http.Header, cb RequestHandleCallbacks) *luaprim.VM {
	t.Helper()
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installStreamInfoMetatable(L)
	installPairsShim(L)
	ctx := &requestHandleContext{headers: h, cb: cb}
	ud := L.NewUserData()
	ud.Value = ctx
	L.SetMetatable(ud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", ud)
	return vm
}

// TestBridge_StreamInfo_Protocol_HTTP11 verifies :protocol() surfaces the
// canned "HTTP/1.1" value verbatim. Mirrors the canonical H1 dispatch path
// per parent §11.6 (DownstreamProtocol == "HTTP/1.1" for the H1 reader).
func TestBridge_StreamInfo_Protocol_HTTP11(t *testing.T) {
	cb := &fakeCallbacks{proto: "HTTP/1.1"}
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `result = rh:streamInfo():protocol()`)
	if got := getGlobalString(t, vm, "result"); got != "HTTP/1.1" {
		t.Fatalf(":protocol() = %q; want %q", got, "HTTP/1.1")
	}
}

// TestBridge_StreamInfo_Protocol_HTTP2 verifies :protocol() surfaces the
// canned "HTTP/2" value verbatim. Mirrors the canonical H2 dispatch path.
func TestBridge_StreamInfo_Protocol_HTTP2(t *testing.T) {
	cb := &fakeCallbacks{proto: "HTTP/2"}
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `result = rh:streamInfo():protocol()`)
	if got := getGlobalString(t, vm, "result"); got != "HTTP/2" {
		t.Fatalf(":protocol() = %q; want %q", got, "HTTP/2")
	}
}

// TestBridge_StreamInfo_Protocol_HTTP10 verifies :protocol() surfaces
// the upstream-parity "HTTP/1.0" string when the dispatch path is HTTP/1.0
// (sub-version of H1; framework callback returns the literal). Pure
// pass-through — no envoy-go re-interpretation.
func TestBridge_StreamInfo_Protocol_HTTP10(t *testing.T) {
	cb := &fakeCallbacks{proto: "HTTP/1.0"}
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `result = rh:streamInfo():protocol()`)
	if got := getGlobalString(t, vm, "result"); got != "HTTP/1.0" {
		t.Fatalf(":protocol() = %q; want %q", got, "HTTP/1.0")
	}
}

// TestBridge_StreamInfo_Protocol_HTTP3 verifies :protocol() surfaces
// the upstream-parity "HTTP/3" string. envoy-go has no H3 dispatch path
// at phase 22.1 — the test pins the pass-through discipline for the day
// the framework adds H3 (no special-casing in the bridge).
func TestBridge_StreamInfo_Protocol_HTTP3(t *testing.T) {
	cb := &fakeCallbacks{proto: "HTTP/3"}
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `result = rh:streamInfo():protocol()`)
	if got := getGlobalString(t, vm, "result"); got != "HTTP/3" {
		t.Fatalf(":protocol() = %q; want %q", got, "HTTP/3")
	}
}

// TestBridge_StreamInfo_RouteName verifies :routeName() surfaces the
// canned route-name string verbatim.
//
// Framework-gap note (Task 8 implementer): the project's
// http.DecoderFilterCallbacks interface (internal/filter/http/callbacks.go)
// does NOT expose a RouteName() accessor at phase 22.1. The Task 9
// DecoderFilterCallbacks → RequestHandleCallbacks adapter stubs
// RouteName() to the empty string (TODO/FIXME: 22.2 may add the framework
// accessor + adapter wiring). The bridge interface here keeps the method
// name for forward-compatibility; test-double passes a canned string to
// prove the bridge plumbing is correct end-to-end.
func TestBridge_StreamInfo_RouteName(t *testing.T) {
	cb := &fakeCallbacks{route: "my-route"}
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `result = rh:streamInfo():routeName()`)
	if got := getGlobalString(t, vm, "result"); got != "my-route" {
		t.Fatalf(":routeName() = %q; want %q", got, "my-route")
	}
}

// TestBridge_StreamInfo_RoutenameEmpty verifies :routeName() surfaces the
// empty string when no route name is set — must NOT crash + must NOT
// surface nil (the contract is "always a string", even if empty). This
// matches the Task 9 adapter's default (route name is "" at phase 22.1
// since the framework callback doesn't expose RouteName()).
func TestBridge_StreamInfo_RoutenameEmpty(t *testing.T) {
	cb := &fakeCallbacks{} // all zero values; route == ""
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `result = rh:streamInfo():routeName()`)
	if got := getGlobalString(t, vm, "result"); got != "" {
		t.Fatalf(":routeName() empty = %q; want %q", got, "")
	}
}

// TestBridge_StreamInfo_DownstreamLocalAddress verifies the local-address
// accessor surfaces the canned "ip:port" string verbatim. The Task 9
// adapter formats from net.Addr.String() (the framework callback returns
// a net.Addr); the bridge is a pure pass-through.
func TestBridge_StreamInfo_DownstreamLocalAddress(t *testing.T) {
	cb := &fakeCallbacks{localAddr: "127.0.0.1:8080"}
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `result = rh:streamInfo():downstreamLocalAddress()`)
	if got := getGlobalString(t, vm, "result"); got != "127.0.0.1:8080" {
		t.Fatalf(":downstreamLocalAddress() = %q; want %q", got, "127.0.0.1:8080")
	}
}

// TestBridge_StreamInfo_DownstreamDirectRemoteAddress verifies the
// direct-remote-address accessor surfaces the canned "ip:port" string.
//
// Framework-gap note: the project's DecoderFilterCallbacks distinguishes
// DownstreamRemoteAddr (the connecting peer) but does NOT expose a
// separate "direct" remote address. The Task 9 adapter wires
// DownstreamDirectRemoteAddress() to the same DownstreamRemoteAddr value
// (no envoy-proxy chain at phase 22.1 means "remote" == "direct remote").
// The bridge interface keeps the upstream-parity method name; future
// framework extension may distinguish the two.
func TestBridge_StreamInfo_DownstreamDirectRemoteAddress(t *testing.T) {
	cb := &fakeCallbacks{remoteAddr: "10.0.0.1:54321"}
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `result = rh:streamInfo():downstreamDirectRemoteAddress()`)
	if got := getGlobalString(t, vm, "result"); got != "10.0.0.1:54321" {
		t.Fatalf(":downstreamDirectRemoteAddress() = %q; want %q", got, "10.0.0.1:54321")
	}
}

// TestBridge_StreamInfo_AllMethodsReturnString verifies each of the 4
// :streamInfo() methods returns a Lua-string (not nil) — even when the
// underlying canned value is the empty string. Closes the "always-string"
// contract anchored at parent §11.6 (the upstream methods return
// std::string, never nullable).
func TestBridge_StreamInfo_AllMethodsReturnString(t *testing.T) {
	cb := &fakeCallbacks{} // all empty strings; no nils
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `
		p  = rh:streamInfo():protocol()
		r  = rh:streamInfo():routeName()
		l  = rh:streamInfo():downstreamLocalAddress()
		dr = rh:streamInfo():downstreamDirectRemoteAddress()
	`)
	for _, name := range []string{"p", "r", "l", "dr"} {
		v := vm.State().GetGlobal(name)
		if v == lua.LNil {
			t.Fatalf("global %q = nil; want string (even empty)", name)
		}
		if _, ok := v.(lua.LString); !ok {
			t.Fatalf("global %q type = %s; want string", name, v.Type())
		}
	}
}

// TestBridge_StreamInfo_AllCannedValues is the comprehensive 4-arm
// table-driven verification — a single fakeCallbacks instance carries
// canned values for all 4 fields, the script invokes all 4 accessors,
// and the assertions compare each surfaced value to its canned source.
// Equivalent to the union of the 4 single-method tests above; included
// for end-to-end-shape clarity (one VM, one Lua script, 4 assertions).
func TestBridge_StreamInfo_AllCannedValues(t *testing.T) {
	cb := &fakeCallbacks{
		proto:      "HTTP/2",
		route:      "ingress-route",
		localAddr:  "192.168.1.10:443",
		remoteAddr: "203.0.113.42:60001",
	}
	vm := newBridgedVMWithCallbacks(t, http.Header{}, cb)
	runScript(t, vm, `
		si = rh:streamInfo()
		p  = si:protocol()
		r  = si:routeName()
		l  = si:downstreamLocalAddress()
		dr = si:downstreamDirectRemoteAddress()
	`)
	if got := getGlobalString(t, vm, "p"); got != "HTTP/2" {
		t.Errorf(":protocol() = %q; want %q", got, "HTTP/2")
	}
	if got := getGlobalString(t, vm, "r"); got != "ingress-route" {
		t.Errorf(":routeName() = %q; want %q", got, "ingress-route")
	}
	if got := getGlobalString(t, vm, "l"); got != "192.168.1.10:443" {
		t.Errorf(":downstreamLocalAddress() = %q; want %q", got, "192.168.1.10:443")
	}
	if got := getGlobalString(t, vm, "dr"); got != "203.0.113.42:60001" {
		t.Errorf(":downstreamDirectRemoteAddress() = %q; want %q", got, "203.0.113.42:60001")
	}
}

// ----------------------------------------------------------------------
// Task 9 — :respond byte-pin per parent §11.6.7 + AMEND-7 + AMEND-8
// ----------------------------------------------------------------------
//
// `request_handle:respond(headers_table, body_string)` captures the
// validated (status, headers, body) tuple on the requestHandleContext
// for the decode dispatcher to emit via SendLocalReply. The byte-pin
// is verified at the captured-state level (status int + OrderedHeaders
// carrier + body string), the dispatcher-level end-to-end pin (the
// captured state flowing through to a test-double SendLocalReply) lives
// at lua_test.go.
//
// `response_handle:respond(...)` raises the AMEND-8 byte-exact runtime
// error wording — verified via vm.CallGlobal returning the wrapped
// *lua.ApiError with the substring.

// findHeader returns the value of the named header in the OrderedHeaders
// carrier, case-insensitive. Returns ("", false) if absent.
func findHeader(oh envoyhttp.OrderedHeaders, name string) (string, bool) {
	target := strings.ToLower(name)
	for _, hf := range oh {
		if strings.ToLower(hf.Name) == target {
			return hf.Value, true
		}
	}
	return "", false
}

// TestBridge_Respond_FullBytePin verifies the parent §11.6.7 4-tuple
// byte-pin: status=403, content-length=6 (auto-set), content-type=
// "application/json" (operator-supplied, NOT defaulted), body="denied".
func TestBridge_Respond_FullBytePin(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="403", ["content-type"]="application/json"}, "denied")`)

	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil {
		t.Fatal("respondCaptured == nil; want non-nil after :respond()")
	}
	rs := ctx.respondCaptured
	if rs.status != 403 {
		t.Errorf("status = %d; want 403", rs.status)
	}
	if rs.body != "denied" {
		t.Errorf("body = %q; want %q", rs.body, "denied")
	}
	if got, ok := findHeader(rs.headers, "content-type"); !ok || got != "application/json" {
		t.Errorf("content-type = %q (ok=%v); want %q", got, ok, "application/json")
	}
	// content-length auto-set from len("denied") == 6.
	if got, ok := findHeader(rs.headers, "content-length"); !ok || got != "6" {
		t.Errorf("content-length = %q (ok=%v); want %q", got, ok, "6")
	}
}

// TestBridge_Respond_CanonicalParentBytePin verifies the exact parent
// §11.6.7 canonical case: script `rh:respond({[":status"]="403"}, "denied")`
// — no operator-supplied content-type; the framework's text/plain
// default fires. Reproduces the parent SPEC's 4-tuple verbatim:
// {status 403; content-length: 6; content-type: text/plain; body: "denied"}.
func TestBridge_Respond_CanonicalParentBytePin(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="403"}, "denied")`)

	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil {
		t.Fatal("respondCaptured == nil; want non-nil after :respond()")
	}
	rs := ctx.respondCaptured
	if rs.status != 403 {
		t.Errorf("status = %d; want 403", rs.status)
	}
	if rs.body != "denied" {
		t.Errorf("body = %q; want %q", rs.body, "denied")
	}
	if got, ok := findHeader(rs.headers, "content-type"); !ok || got != "text/plain" {
		t.Errorf("content-type = %q (ok=%v); want %q (default per utility.cc:1241,1273)", got, ok, "text/plain")
	}
	if got, ok := findHeader(rs.headers, "content-length"); !ok || got != "6" {
		t.Errorf("content-length = %q (ok=%v); want %q (auto-set from len(\"denied\"))", got, ok, "6")
	}
}

// TestBridge_Respond_StatusRangeValidation_Below200 verifies that a
// :status value below 200 raises the byte-exact AMEND-8 runtime error
// ":status must be between 200-599".
func TestBridge_Respond_StatusRangeValidation_Below200(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	chunk, cerr := luaprim.CompileScript([]byte(`rh:respond({[":status"]="100"}, "")`), nil)
	if cerr != nil {
		t.Fatalf("CompileScript err = %v", cerr)
	}
	err := vm.Run(chunk)
	if err == nil {
		t.Fatal("vm.Run err = nil; want byte-exact :status range error")
	}
	if !strings.Contains(err.Error(), ":status must be between 200-599") {
		t.Errorf("err = %v; want substring %q", err, ":status must be between 200-599")
	}
}

// TestBridge_Respond_StatusRangeValidation_Above599 verifies that a
// :status value at or above 600 raises the byte-exact AMEND-8 runtime
// error ":status must be between 200-599" (the range is the inclusive
// [200, 599], i.e., Go half-open [200, 600)).
func TestBridge_Respond_StatusRangeValidation_Above599(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	chunk, cerr := luaprim.CompileScript([]byte(`rh:respond({[":status"]="600"}, "")`), nil)
	if cerr != nil {
		t.Fatalf("CompileScript err = %v", cerr)
	}
	err := vm.Run(chunk)
	if err == nil {
		t.Fatal("vm.Run err = nil; want byte-exact :status range error")
	}
	if !strings.Contains(err.Error(), ":status must be between 200-599") {
		t.Errorf("err = %v; want substring %q", err, ":status must be between 200-599")
	}
}

// TestBridge_Respond_StatusBoundary_200_Accepted verifies that
// :status=200 (the inclusive low bound) is accepted (no error).
func TestBridge_Respond_StatusBoundary_200_Accepted(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="200"}, "")`)
	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil || ctx.respondCaptured.status != 200 {
		t.Fatalf("status = %d; want 200 (inclusive low bound)", ctx.respondCaptured.status)
	}
}

// TestBridge_Respond_StatusBoundary_599_Accepted verifies that
// :status=599 (the inclusive high bound) is accepted (no error).
func TestBridge_Respond_StatusBoundary_599_Accepted(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="599"}, "")`)
	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil || ctx.respondCaptured.status != 599 {
		t.Fatalf("status = %d; want 599 (inclusive high bound)", ctx.respondCaptured.status)
	}
}

// TestBridge_Respond_AutoContentLength verifies that content-length is
// auto-set from len(body) when the headers table did not supply it.
// Multi-byte body to verify the byte-length discipline (Go len() on
// strings returns bytes, not runes).
func TestBridge_Respond_AutoContentLength(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="200"}, "hello")`)
	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil {
		t.Fatal("respondCaptured == nil")
	}
	if got, ok := findHeader(ctx.respondCaptured.headers, "content-length"); !ok || got != "5" {
		t.Errorf("content-length = %q (ok=%v); want %q", got, ok, "5")
	}
}

// TestBridge_Respond_AutoContentLength_EmptyBody verifies that
// content-length=0 is auto-set for an empty-body respond.
func TestBridge_Respond_AutoContentLength_EmptyBody(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="204"}, "")`)
	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil {
		t.Fatal("respondCaptured == nil")
	}
	if got, ok := findHeader(ctx.respondCaptured.headers, "content-length"); !ok || got != "0" {
		t.Errorf("content-length = %q (ok=%v); want %q (auto-set len(\"\") == 0)", got, ok, "0")
	}
}

// TestBridge_Respond_ContentTypeDefault verifies content-type defaults
// to "text/plain" when absent from the headers table (per upstream
// Utility::prepareLocalReply at utility.cc:1241,1273).
func TestBridge_Respond_ContentTypeDefault(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="200"}, "")`)
	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil {
		t.Fatal("respondCaptured == nil")
	}
	if got, ok := findHeader(ctx.respondCaptured.headers, "content-type"); !ok || got != "text/plain" {
		t.Errorf("content-type = %q (ok=%v); want %q (default)", got, ok, "text/plain")
	}
}

// TestBridge_Respond_UserContentTypeRespected verifies the user-supplied
// content-type is preserved verbatim (NOT overwritten with the default).
func TestBridge_Respond_UserContentTypeRespected(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="200", ["content-type"]="application/xml"}, "<x/>")`)
	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil {
		t.Fatal("respondCaptured == nil")
	}
	// Count content-type entries: must be exactly 1 (the user-supplied
	// one); the default branch must NOT have fired (which would produce
	// 2 entries — the user-supplied + the "text/plain" default).
	count := 0
	for _, hf := range ctx.respondCaptured.headers {
		if strings.ToLower(hf.Name) == "content-type" {
			count++
			if hf.Value != "application/xml" {
				t.Errorf("content-type value = %q; want %q (user-supplied)", hf.Value, "application/xml")
			}
		}
	}
	if count != 1 {
		t.Errorf("content-type entry count = %d; want 1 (default branch must NOT fire when user-supplied)", count)
	}
}

// TestBridge_Respond_UserContentLengthRespected verifies the user-supplied
// content-length is preserved verbatim (NOT overwritten with the
// auto-set len(body)). Operators may override (e.g., for chunked
// transfer-encoding setups).
func TestBridge_Respond_UserContentLengthRespected(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="200", ["content-length"]="999"}, "hello")`)
	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil {
		t.Fatal("respondCaptured == nil")
	}
	count := 0
	for _, hf := range ctx.respondCaptured.headers {
		if strings.ToLower(hf.Name) == "content-length" {
			count++
			if hf.Value != "999" {
				t.Errorf("content-length value = %q; want %q (user-supplied)", hf.Value, "999")
			}
		}
	}
	if count != 1 {
		t.Errorf("content-length entry count = %d; want 1 (auto-set must NOT fire when user-supplied)", count)
	}
}

// TestBridge_Respond_PseudoHeadersSkipped verifies that pseudo-headers
// other than :status (i.e., :scheme, :authority, :path, :method) are
// SKIPPED from the output OrderedHeaders carrier. They are decode-only
// and nonsensical on a synthetic local-reply.
func TestBridge_Respond_PseudoHeadersSkipped(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]="200", [":authority"]="example.com", [":path"]="/x", ["x-good"]="yes"}, "")`)
	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil {
		t.Fatal("respondCaptured == nil")
	}
	// Walk all headers; none should start with ":".
	for _, hf := range ctx.respondCaptured.headers {
		if strings.HasPrefix(hf.Name, ":") {
			t.Errorf("pseudo-header %q leaked into output; want skipped", hf.Name)
		}
	}
	// x-good MUST be present.
	if got, ok := findHeader(ctx.respondCaptured.headers, "x-good"); !ok || got != "yes" {
		t.Errorf("x-good = %q (ok=%v); want %q", got, ok, "yes")
	}
}

// TestBridge_Respond_NumericStatus_Accepted verifies that callers can
// supply :status as a number (not a string) per gopher-lua's loose
// typing. Both `[":status"]=403` and `[":status"]="403"` accepted.
func TestBridge_Respond_NumericStatus_Accepted(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVM(t, h)
	runScript(t, vm, `rh:respond({[":status"]=403}, "")`)
	ctx := getRequestCtx(t, vm)
	if ctx.respondCaptured == nil || ctx.respondCaptured.status != 403 {
		t.Fatalf("status = %v; want 403", ctx.respondCaptured)
	}
}

// TestBridge_ResponseHandleRespond_RejectsByteExact verifies that
// invoking :respond() from the response_handle (encode side) raises
// the byte-exact AMEND-8 runtime error
// "respond not currently supported in the response path".
func TestBridge_ResponseHandleRespond_RejectsByteExact(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVMWithResponseHandle(t, h)
	chunk, cerr := luaprim.CompileScript([]byte(`resp:respond({[":status"]="200"}, "")`), nil)
	if cerr != nil {
		t.Fatalf("CompileScript err = %v", cerr)
	}
	err := vm.Run(chunk)
	if err == nil {
		t.Fatal("vm.Run err = nil; want byte-exact AMEND-8 reject")
	}
	if !strings.Contains(err.Error(), "respond not currently supported in the response path") {
		t.Errorf("err = %v; want substring %q", err, "respond not currently supported in the response path")
	}
}

// TestBridge_ResponseHandleRespond_FromEnvoyOnResponseHook verifies the
// reject also fires when :respond() is invoked from within an
// envoy_on_response hook function (the canonical script-author shape).
func TestBridge_ResponseHandleRespond_FromEnvoyOnResponseHook(t *testing.T) {
	h := http.Header{}
	vm := newBridgedVMWithResponseHandle(t, h)
	// Define the hook then invoke it directly via CallGlobal so we can
	// observe the error surface.
	runScript(t, vm, `function envoy_on_response(rh) rh:respond({[":status"]="200"}, "") end`)
	if !vm.HasGlobalFunc("envoy_on_response") {
		t.Fatal("envoy_on_response not defined after Run")
	}
	// Build the response_handle userdata + invoke via CallGlobal.
	L := vm.State()
	respUd := L.GetGlobal("resp") // already bound by newBridgedVMWithResponseHandle
	luaUd, ok := respUd.(*lua.LUserData)
	if !ok {
		t.Fatalf("global resp not userdata: %v", respUd)
	}
	err := vm.CallGlobal("envoy_on_response", luaUd)
	if err == nil {
		t.Fatal("CallGlobal err = nil; want AMEND-8 reject")
	}
	if !strings.Contains(err.Error(), "respond not currently supported in the response path") {
		t.Errorf("err = %v; want substring %q", err, "respond not currently supported in the response path")
	}
}

// getRequestCtx fetches the *requestHandleContext from the Lua global
// "rh" (bound by newBridgedVM). The cast is by Go-side type assertion
// after extracting the LUserData.Value.
func getRequestCtx(t *testing.T, vm *luaprim.VM) *requestHandleContext {
	t.Helper()
	L := vm.State()
	rh := L.GetGlobal("rh")
	ud, ok := rh.(*lua.LUserData)
	if !ok {
		t.Fatalf("global rh not userdata: %T", rh)
	}
	ctx, ok := ud.Value.(*requestHandleContext)
	if !ok {
		t.Fatalf("rh.Value not *requestHandleContext: %T", ud.Value)
	}
	return ctx
}

// ----------------------------------------------------------------------
// Compile-time signature pins for the bridge surface that Tasks 7-9
// extend. Pins the Task 6 contribution; later tasks add more.
// ----------------------------------------------------------------------

var (
	_ func(*lua.LState) *lua.LTable = installRequestHandleMetatable
	_ func(*lua.LState) *lua.LTable = installResponseHandleMetatable
	_ func(*lua.LState) *lua.LTable = installHeadersMetatable
	_ func(*lua.LState) *lua.LTable = installStreamInfoMetatable
	_ func(*lua.LState)             = installPairsShim
)

// Avoid unused-import warning for fmt — used in the test descriptor
// strings for sub-test enumeration (see future Task 7/8/9 tests).
var _ = fmt.Sprintf
