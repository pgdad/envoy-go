package lua

// misc_test.go — Task 12 (phase 22.2 IMPL) misc bridge tests per SPEC
// §3.4 + AMEND-22.2-1 + D8 + PLAN Task 12 acceptance.
//
// Coverage:
//
//   - Test_FileBytes_happy_path_returns_file_contents — synthetic file
//     contents round-trip via os.Open + io.ReadAll.
//   - Test_FileBytes_over_cap_raises_runtime_reject — 16 MiB+1 file
//     raises a Lua runtime error with byte-stable wording prefix.
//   - Test_FileBytes_ENOENT_returns_nil_plus_error — missing file
//     returns (nil, err_string) idiomatic Lua disposition.
//   - Test_FileBytes_arbitrary_path_allowed — envoy-go-strict per D8;
//     no path-restriction surface.
//   - Test_Timestamp_default_unit_milliseconds_monotonic_increasing — N
//     successive calls yield non-decreasing values.
//   - Test_Timestamp_seconds_unit_returns_approximately_milliseconds_div_1000.
//   - Test_Timestamp_microseconds_unit.
//   - Test_Timestamp_invalid_unit_raises_runtime_error — pinned wording.

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"

	luaprim "github.com/pgdad/envoy-go/internal/lua"
)

// newMiscBridgeVM mirrors newCryptoBridgeVM scaffolding for the misc
// bridge methods (:fileBytes + :timestamp).
func newMiscBridgeVM(t *testing.T) *luaprim.VM {
	t.Helper()
	vm := luaprim.NewVM()
	t.Cleanup(vm.Close)
	L := vm.State()
	installRequestHandleMetatable(L)
	installResponseHandleMetatable(L)
	installHeadersMetatable(L)
	installPairsShim(L)
	ctx := &requestHandleContext{headers: http.Header{}}
	ud := L.NewUserData()
	ud.Value = ctx
	L.SetMetatable(ud, L.GetTypeMetatable(requestHandleTypeName))
	L.SetGlobal("rh", ud)
	return vm
}

// ---------------------------------------------------------------------
// :fileBytes — envoy-go-strict per D8 (NOT in upstream)
// ---------------------------------------------------------------------

func Test_FileBytes_happy_path_returns_file_contents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "happy.bin")
	want := []byte{0x00, 0x01, 0x02, 0xff, 0x10, 0x20, 0x30, 0x40, 0x50}
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	vm := newMiscBridgeVM(t)
	L := vm.State()
	L.SetGlobal("path", lua.LString(path))
	runScript(t, vm, `result = rh:fileBytes(path)`)
	got := getGlobalString(t, vm, "result")
	if got != string(want) {
		t.Fatalf(":fileBytes(%q) = %q; want %q", path, got, want)
	}
}

func Test_FileBytes_over_cap_raises_runtime_reject(t *testing.T) {
	// Write a file 1 byte over the 16 MiB cap (matching 22.1 Task 11
	// pattern); expect a Lua runtime error with byte-stable wording.
	dir := t.TempDir()
	path := filepath.Join(dir, "over.bin")
	over := make([]byte, maxFilenameScriptBytes+1)
	if err := os.WriteFile(path, over, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	vm := newMiscBridgeVM(t)
	L := vm.State()
	L.SetGlobal("path", lua.LString(path))
	chunk, err := luaprim.CompileScript([]byte(`result = rh:fileBytes(path)`), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}
	runErr := vm.Run(chunk)
	if runErr == nil {
		t.Fatalf("vm.Run = nil; want runtime error")
	}
	if !strings.Contains(runErr.Error(), miscFileBytesOverCapPrefix) {
		t.Fatalf("vm.Run error = %q; want substring %q",
			runErr.Error(), miscFileBytesOverCapPrefix)
	}
}

func Test_FileBytes_ENOENT_returns_nil_plus_error(t *testing.T) {
	// Idiomatic Lua disposition: (nil, err_string) so scripts may check
	// `local b, err = rh:fileBytes(path); if b == nil then ... end`.
	vm := newMiscBridgeVM(t)
	L := vm.State()
	L.SetGlobal("path", lua.LString("/nonexistent/path/should/not/exist"))
	runScript(t, vm, `b, err = rh:fileBytes(path)`)
	if !isGlobalNil(vm, "b") {
		t.Fatalf("b = %v; want nil", vm.State().GetGlobal("b"))
	}
	errGlobal := L.GetGlobal("err")
	if errGlobal == lua.LNil {
		t.Fatalf("err = nil; want a non-empty error string")
	}
	if _, ok := errGlobal.(lua.LString); !ok {
		t.Fatalf("err type = %s; want string", errGlobal.Type())
	}
}

func Test_FileBytes_arbitrary_path_allowed(t *testing.T) {
	// envoy-go-strict per D8: no path-restriction surface (no chroot,
	// no allow-list). Verify a path entirely outside t.TempDir() (e.g.
	// /etc/hostname OR a separately-created location) still works. To
	// avoid system-dependent files, create two t.TempDirs and assert a
	// cross-dir read works.
	dirA := t.TempDir()
	dirB := t.TempDir()
	if dirA == dirB {
		t.Fatalf("t.TempDir() returned identical dirs; cannot test cross-dir")
	}
	path := filepath.Join(dirB, "arbitrary.bin")
	want := []byte("arbitrary path contents")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	vm := newMiscBridgeVM(t)
	L := vm.State()
	L.SetGlobal("path", lua.LString(path))
	runScript(t, vm, `result = rh:fileBytes(path)`)
	got := getGlobalString(t, vm, "result")
	if got != string(want) {
		t.Fatalf("arbitrary-path :fileBytes() = %q; want %q", got, want)
	}
}

// ---------------------------------------------------------------------
// :timestamp — non-deterministic wall-clock from time.Now() per BRAINSTORM §2.7
// ---------------------------------------------------------------------

func Test_Timestamp_default_unit_milliseconds_monotonic_increasing(t *testing.T) {
	// Default unit is "milliseconds". N successive calls must yield
	// non-decreasing values (we don't assert STRICTLY-increasing because
	// the system clock has finite resolution).
	vm := newMiscBridgeVM(t)
	runScript(t, vm, `
results = {}
for i = 1, 10 do
  results[i] = rh:timestamp()
end
`)
	tbl, ok := vm.State().GetGlobal("results").(*lua.LTable)
	if !ok {
		t.Fatalf("results not a table")
	}
	var prev float64
	for i := 1; i <= 10; i++ {
		v := tbl.RawGetInt(i)
		n, isNum := v.(lua.LNumber)
		if !isNum {
			t.Fatalf("results[%d] type = %s; want number", i, v.Type())
		}
		f := float64(n)
		if i > 1 && f < prev {
			t.Fatalf("results[%d] = %v < prev %v; want non-decreasing", i, f, prev)
		}
		prev = f
	}

	// Sanity: the millisecond timestamp is roughly current Unix time
	// in ms (within 5 seconds — accounts for slow CI / scheduling).
	now := float64(time.Now().UnixMilli())
	if prev < now-5000 || prev > now+5000 {
		t.Fatalf("timestamp = %v; want ~%v", prev, now)
	}
}

func Test_Timestamp_seconds_unit_returns_approximately_milliseconds_div_1000(t *testing.T) {
	// :timestamp("seconds") returns the Unix time in seconds. Cross-check
	// vs :timestamp("milliseconds") taken roughly contemporaneously
	// (must agree to within a 5-second window for fuzz tolerance).
	vm := newMiscBridgeVM(t)
	runScript(t, vm, `
ms = rh:timestamp("milliseconds")
s = rh:timestamp("seconds")
`)
	ms, ok := vm.State().GetGlobal("ms").(lua.LNumber)
	if !ok {
		t.Fatalf("ms not a number")
	}
	s, ok := vm.State().GetGlobal("s").(lua.LNumber)
	if !ok {
		t.Fatalf("s not a number")
	}
	delta := float64(ms)/1000 - float64(s)
	if delta < -5 || delta > 5 {
		t.Fatalf("delta = %v; ms=%v s=%v want |delta| < 5", delta, ms, s)
	}
}

func Test_Timestamp_microseconds_unit(t *testing.T) {
	// :timestamp("microseconds") returns Unix time in microseconds.
	vm := newMiscBridgeVM(t)
	runScript(t, vm, `result = rh:timestamp("microseconds")`)
	v, ok := vm.State().GetGlobal("result").(lua.LNumber)
	if !ok {
		t.Fatalf("result not a number")
	}
	// Sanity-check vs current Unix microseconds (within 5 seconds = 5e6 us).
	now := float64(time.Now().UnixMicro())
	if float64(v) < now-5e6 || float64(v) > now+5e6 {
		t.Fatalf("microseconds timestamp = %v; want ~%v", v, now)
	}
}

func Test_Timestamp_invalid_unit_raises_runtime_error(t *testing.T) {
	// Invalid unit raises a Lua runtime error per SPEC §3.4 +
	// PLAN Task 12 acceptance.
	vm := newMiscBridgeVM(t)
	chunk, err := luaprim.CompileScript([]byte(`result = rh:timestamp("fortnight")`), nil)
	if err != nil {
		t.Fatalf("CompileScript: %v", err)
	}
	runErr := vm.Run(chunk)
	if runErr == nil {
		t.Fatalf("vm.Run = nil; want runtime error for invalid unit")
	}
	if !strings.Contains(runErr.Error(), miscTimestampInvalidUnitPrefix) {
		t.Fatalf("vm.Run error = %q; want substring %q",
			runErr.Error(), miscTimestampInvalidUnitPrefix)
	}
}
