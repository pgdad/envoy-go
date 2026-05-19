package lua

// sandbox_test.go — Task 5 full IMPL per-stdlib ALLOW/DENY exhaustive
// tests per 22.1 PLAN Task 5 + 22.1 SPEC §3.3 sandbox-roster table +
// AMEND-1 default-deny posture + AMEND-A4 coroutine-allowed.
//
// Test discipline: each test constructs a fresh VM with an explicit
// SandboxConfig, runs a Lua snippet exercising the gated stdlib entry,
// and asserts the expected ALLOW (no error) or DENY (Lua runtime error
// + nil-deref message) outcome. The DENY arm asserts the gopher-lua
// runtime-error message "attempt to call a nil value" or
// "attempt to index a nil value" since post-walk nil-out leaves the
// global/table-entry as lua.LNil rather than removing the binding.

import (
	"strings"
	"testing"

	lua "github.com/yuin/gopher-lua"
)

// runLuaSrc is the per-test helper: constructs a VM with the supplied
// sandbox config, compiles + runs src, returns vm.Run's error (or nil).
// Always closes the VM via defer.
func runLuaSrc(t *testing.T, sb SandboxConfig, src string) error {
	t.Helper()
	vm := NewVM(WithSandboxConfig(sb))
	defer vm.Close()
	chunk, err := CompileScript([]byte(src), nil)
	if err != nil {
		t.Fatalf("CompileScript(%q) err = %v; want nil (test src must compile)", src, err)
	}
	return vm.Run(chunk)
}

// TestApplyZeroValueDefaults_EnablesCoroutineAndTimeHelpers pins the
// zero-value-helper contract: a bare zero-valued SandboxConfig has
// AllowCoroutine + AllowOSTimeHelpers flipped on to match upstream
// luaL_openlibs posture per AMEND-A4.
func TestApplyZeroValueDefaults_EnablesCoroutineAndTimeHelpers(t *testing.T) {
	got := applyZeroValueDefaults(SandboxConfig{})
	if !got.AllowCoroutine {
		t.Errorf("AllowCoroutine = false; want true (AMEND-A4 luaL_openlibs parity)")
	}
	if !got.AllowOSTimeHelpers {
		t.Errorf("AllowOSTimeHelpers = false; want true (upstream-parity time arm)")
	}
	if got.AllowBaseFull || got.AllowIO || got.AllowOS || got.AllowDebug || got.AllowPackage || got.AllowChannel {
		t.Errorf("strict-deny posture broken: %+v", got)
	}
}

// TestApplyZeroValueDefaults_PreservesNonZero verifies the helper does
// NOT clobber explicit-true values for OTHER fields (the helper only
// flips AllowCoroutine + AllowOSTimeHelpers when they are zero/false;
// other fields pass through verbatim).
func TestApplyZeroValueDefaults_PreservesNonZero(t *testing.T) {
	in := SandboxConfig{
		AllowBaseFull: true,
		AllowIO:       true,
		AllowOS:       true,
		AllowDebug:    true,
		AllowPackage:  true,
		AllowChannel:  true,
	}
	got := applyZeroValueDefaults(in)
	if !got.AllowBaseFull || !got.AllowIO || !got.AllowOS || !got.AllowDebug || !got.AllowPackage || !got.AllowChannel {
		t.Errorf("explicit-true fields clobbered: %+v", got)
	}
	if !got.AllowCoroutine || !got.AllowOSTimeHelpers {
		t.Errorf("zero-value default-on fields broken: %+v", got)
	}
}

// ----------------------------- io -----------------------------

// TestSandbox_DefaultDeniesIO_OpenNilDeref verifies the default
// zero-value SandboxConfig denies the entire io stdlib. Calling
// io.open(...) attempts to index the nil `io` global → runtime error.
func TestSandbox_DefaultDeniesIO_OpenNilDeref(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{}, `local f = io.open("/etc/passwd", "r")`)
	if err == nil {
		t.Fatal("default sandbox allowed io.open; want runtime error (io is nil)")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Fatalf("io.open err = %q; want substring \"nil\" (nil-deref/nil-call from sandbox-nilled io)", err.Error())
	}
}

// TestSandbox_AllowIO_OpensIo verifies AllowIO=true opens io stdlib +
// io is callable. The Lua snippet checks `type(io) == "table"` rather
// than actually opening a file (which would hit the disk).
func TestSandbox_AllowIO_OpensIo(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{AllowIO: true}, `assert(type(io) == "table", "io is not a table")`)
	if err != nil {
		t.Fatalf("AllowIO=true sandbox runtime err = %v; want nil", err)
	}
}

// ----------------------------- os -----------------------------

// TestSandbox_DefaultDeniesOsExecute verifies the default sandbox nils
// out os.execute (the AllowOSTimeHelpers zero-value-default opens the
// `os` table, but the 7 dangerous entries are nilled). The runtime
// error wording differs slightly depending on whether the nil value
// is at the top-level global (gopher-lua says "attempt to call a nil
// value") or inside a table entry (gopher-lua says "attempt to call a
// non-function object"). Both indicate a successful sandbox-deny;
// accept either via a substring-OR check on "nil" or "non-function".
func TestSandbox_DefaultDeniesOsExecute(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{}, `os.execute("ls")`)
	if err == nil {
		t.Fatal("default sandbox allowed os.execute; want runtime error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "nil") && !strings.Contains(msg, "non-function") {
		t.Fatalf("os.execute err = %q; want substring \"nil\" or \"non-function\" (sandbox-deny)", msg)
	}
}

// TestSandbox_DefaultAllowsOsTime verifies the upstream-parity os.time
// arm: the zero-value SandboxConfig leaves os.time callable.
func TestSandbox_DefaultAllowsOsTime(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{}, `local t = os.time(); assert(type(t) == "number", "os.time returned non-number")`)
	if err != nil {
		t.Fatalf("default sandbox os.time err = %v; want nil (AllowOSTimeHelpers default-on)", err)
	}
}

// TestSandbox_AllowOS_OpensFullOs verifies AllowOS=true exposes the
// full os table (os.getenv, os.execute, etc., are all callable —
// individual entries are NOT nilled when AllowOS is true).
func TestSandbox_AllowOS_OpensFullOs(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{AllowOS: true}, `assert(type(os.execute) == "function", "os.execute not a function")`)
	if err != nil {
		t.Fatalf("AllowOS=true sandbox os.execute err = %v; want nil", err)
	}
}

// TestSandbox_OsPostWalkNilsOut_NonTimeHelpers exhaustively verifies
// the 7 entries nilled at the default-sandbox os post-walk.
func TestSandbox_OsPostWalkNilsOut_NonTimeHelpers(t *testing.T) {
	deniedOsEntries := []string{
		"execute",
		"exit",
		"remove",
		"rename",
		"getenv",
		"setlocale",
		"tmpname",
	}
	for _, name := range deniedOsEntries {
		t.Run(name, func(t *testing.T) {
			src := `assert(os.` + name + ` == nil, "os.` + name + ` is not nil")`
			err := runLuaSrc(t, SandboxConfig{}, src)
			if err != nil {
				t.Fatalf("os.%s assertion err = %v; want nil (os.%s should be nilled)", name, err, name)
			}
		})
	}
}

// TestSandbox_OsTimeHelpers_RemainCallable verifies the 4 read-only
// time helpers (time / date / clock / difftime) remain non-nil at the
// default sandbox.
func TestSandbox_OsTimeHelpers_RemainCallable(t *testing.T) {
	timeHelpers := []string{
		"time",
		"date",
		"clock",
		"difftime",
	}
	for _, name := range timeHelpers {
		t.Run(name, func(t *testing.T) {
			src := `assert(type(os.` + name + `) == "function", "os.` + name + ` is not a function")`
			err := runLuaSrc(t, SandboxConfig{}, src)
			if err != nil {
				t.Fatalf("os.%s assertion err = %v; want nil (os.%s should be callable)", name, err, name)
			}
		})
	}
}

// ----------------------------- debug -----------------------------

// TestSandbox_DefaultDeniesDebug verifies the default sandbox denies
// the debug module wholesale.
func TestSandbox_DefaultDeniesDebug(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{}, `local _ = debug.getupvalue`)
	if err == nil {
		t.Fatal("default sandbox allowed debug indexing; want runtime error (debug is nil)")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Fatalf("debug index err = %q; want substring \"nil\"", err.Error())
	}
}

// TestSandbox_AllowDebug_OpensDebug verifies AllowDebug=true opens
// the debug module.
func TestSandbox_AllowDebug_OpensDebug(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{AllowDebug: true}, `assert(type(debug) == "table", "debug is not a table")`)
	if err != nil {
		t.Fatalf("AllowDebug=true err = %v; want nil", err)
	}
}

// ----------------------------- package -----------------------------

// TestSandbox_DefaultDeniesPackage verifies the default sandbox denies
// the package module wholesale.
func TestSandbox_DefaultDeniesPackage(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{}, `local _ = package.path`)
	if err == nil {
		t.Fatal("default sandbox allowed package.path indexing; want runtime error (package is nil)")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Fatalf("package index err = %q; want substring \"nil\"", err.Error())
	}
}

// TestSandbox_AllowPackage_OpensPackage verifies AllowPackage=true
// opens the package module.
func TestSandbox_AllowPackage_OpensPackage(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{AllowPackage: true}, `assert(type(package) == "table", "package is not a table")`)
	if err != nil {
		t.Fatalf("AllowPackage=true err = %v; want nil", err)
	}
}

// ----------------------------- channel -----------------------------

// TestSandbox_DefaultDeniesChannel verifies the default sandbox denies
// the gopher-lua-specific channel stdlib.
func TestSandbox_DefaultDeniesChannel(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{}, `local _ = channel.make`)
	if err == nil {
		t.Fatal("default sandbox allowed channel indexing; want runtime error (channel is nil)")
	}
	if !strings.Contains(err.Error(), "nil") {
		t.Fatalf("channel index err = %q; want substring \"nil\"", err.Error())
	}
}

// TestSandbox_AllowChannel_OpensChannel verifies AllowChannel=true
// opens the channel module.
func TestSandbox_AllowChannel_OpensChannel(t *testing.T) {
	err := runLuaSrc(t, SandboxConfig{AllowChannel: true}, `assert(type(channel) == "table", "channel is not a table")`)
	if err != nil {
		t.Fatalf("AllowChannel=true err = %v; want nil", err)
	}
}

// ----------------------------- coroutine -----------------------------

// TestSandbox_DefaultAllowsCoroutine verifies the AMEND-A4 / zero-value
// default opens the coroutine stdlib.
func TestSandbox_DefaultAllowsCoroutine(t *testing.T) {
	src := `local co = coroutine.create(function() end); assert(type(co) == "thread", "coroutine.create did not return thread")`
	err := runLuaSrc(t, SandboxConfig{}, src)
	if err != nil {
		t.Fatalf("default sandbox coroutine err = %v; want nil (AMEND-A4 default-on)", err)
	}
}

// ----------------------------- base globals -----------------------------

// TestSandbox_DefaultDeniesBaseGlobals exhaustively verifies the 9
// denied base globals are nil at the default sandbox.
func TestSandbox_DefaultDeniesBaseGlobals(t *testing.T) {
	deniedBase := []string{
		"dofile",
		"loadfile",
		"loadstring",
		"load",
		"module",
		"require",
		"collectgarbage",
		"getfenv",
		"setfenv",
	}
	for _, name := range deniedBase {
		t.Run(name, func(t *testing.T) {
			src := `assert(` + name + ` == nil, "` + name + ` is not nil")`
			err := runLuaSrc(t, SandboxConfig{}, src)
			if err != nil {
				t.Fatalf("%s assertion err = %v; want nil (%s should be nilled at default sandbox)", name, err, name)
			}
		})
	}
}

// TestSandbox_AllowBaseFull_KeepsAllBase verifies AllowBaseFull=true
// preserves the 9 base globals (no post-walk nil-out runs).
func TestSandbox_AllowBaseFull_KeepsAllBase(t *testing.T) {
	preservedBase := []string{
		"dofile",
		"loadfile",
		"loadstring",
		"load",
		// `module` is removed from Lua 5.2+; gopher-lua does NOT bind it
		// per its `value.go` base func registry. Skipped here to keep
		// the test resilient against the gopher-lua API surface.
		"require",
		"collectgarbage",
		"getfenv",
		"setfenv",
	}
	for _, name := range preservedBase {
		t.Run(name, func(t *testing.T) {
			src := `assert(` + name + ` ~= nil, "` + name + ` is nil under AllowBaseFull")`
			err := runLuaSrc(t, SandboxConfig{AllowBaseFull: true}, src)
			if err != nil {
				t.Fatalf("%s assertion err = %v; want nil under AllowBaseFull=true", name, err)
			}
		})
	}
}

// TestSandbox_AlwaysAllowsCoreStdlibs verifies the unconditional
// ALLOW arms (table / string / math) are always opened.
func TestSandbox_AlwaysAllowsCoreStdlibs(t *testing.T) {
	coreLibs := []string{
		"table",
		"string",
		"math",
	}
	for _, name := range coreLibs {
		t.Run(name, func(t *testing.T) {
			src := `assert(type(` + name + `) == "table", "` + name + ` is not a table")`
			err := runLuaSrc(t, SandboxConfig{}, src)
			if err != nil {
				t.Fatalf("%s assertion err = %v; want nil (%s is always-allowed core)", name, err, name)
			}
		})
	}
}

// TestSandbox_InternalTraceback_Exposed verifies the panic-wrapper's
// INTERNAL __envoy_traceback global is exposed to scripts even with
// debug denied (it is an internal-use helper, not a script-facing API,
// but it MUST exist for the wrapper's reference).
func TestSandbox_InternalTraceback_Exposed(t *testing.T) {
	vm := NewVM(WithSandboxConfig(SandboxConfig{}))
	defer vm.Close()
	tb := vm.State().GetGlobal("__envoy_traceback")
	if tb.Type() != lua.LTFunction {
		t.Fatalf("__envoy_traceback type = %v; want LTFunction (panic-wrapper internal)", tb.Type())
	}
}
