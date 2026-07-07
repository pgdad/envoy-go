package lua

// sandbox.go — SandboxConfig + per-stdlib ALLOW/DENY discipline per
// 22.1 SPEC §3.1 + §3.3 + parent SPEC §4.3 + AMEND-1 (envoy-go-strict
// DEPARTURE from upstream's bare luaL_openlibs).
//
// Task 5 IMPL — full per-stdlib selective OpenXxx + post-walk nil-out
// of the 9 denied base globals + the 7 denied os non-time entries.
// Called exactly once per *lua.LState from NewVM after lua.NewState
// (with SkipOpenLibs: true so the catch-all luaL_openlibs equivalent
// is NEVER invoked).
//
// Roster mapping (per 22.1 SPEC §3.3 verbatim):
//
//   ALWAYS opened:  base (post-walk nil 9), table, string, math
//   AllowCoroutine: coroutine
//   AllowOS:        os (unrestricted)
//   AllowOSTimeHelpers (and not AllowOS): os (post-walk nil 7 non-time)
//   AllowIO:        io
//   AllowDebug:     debug
//   AllowPackage:   package
//   AllowChannel:   channel
//
// Denied-by-default base globals (9): dofile / loadfile / loadstring /
// load / module / require / collectgarbage / getfenv / setfenv.
// Denied-by-default os entries (7) when AllowOSTimeHelpers && !AllowOS:
// execute / exit / remove / rename / getenv / setlocale / tmpname.
// (The 4 time helpers time / date / clock / difftime REMAIN.)

import (
	lua "github.com/yuin/gopher-lua"
)

// SandboxConfig governs which stdlib modules are exposed to scripts.
// Zero value (post applyZeroValueDefaults) = StrictUpstreamParity per
// §3.3 + parent SPEC §4.3 + AMEND-1 (DENY io / os / debug / package /
// channel + base-stdlib dangerous globals dofile / loadfile / loadstring /
// load / require / module / collectgarbage / getfenv / setfenv; REDIRECT
// print to BasePrintSink with default-nil = drop; ALLOW base-core +
// table + string + math + coroutine + os.time / os.date / os.clock /
// os.difftime via AllowOSTimeHelpers).
type SandboxConfig struct {
	// AllowBaseFull exposes base stdlib's dangerous globals
	// (dofile / loadfile / loadstring / load / require / module /
	// collectgarbage / getfenv / setfenv). Default false = strict.
	AllowBaseFull bool

	// AllowIO exposes io.* wholesale (io.open / io.popen — unsandboxed
	// file + subprocess access). Default false = strict.
	AllowIO bool

	// AllowOS exposes os.* wholesale (os.execute / exit / remove /
	// rename / getenv / setlocale / tmpname — subprocess + host-Go-
	// process-exit hazards). Default false = strict; AllowOSTimeHelpers
	// preserves the upstream-parity read-only time arm.
	AllowOS bool

	// AllowOSTimeHelpers exposes only os.time / os.date / os.clock /
	// os.difftime. Independent of AllowOS — if AllowOS is true,
	// AllowOSTimeHelpers is implied. Default false; flipped to true by
	// applyZeroValueDefaults to match upstream-parity time arm at the
	// zero-value SandboxConfig posture.
	AllowOSTimeHelpers bool

	// AllowDebug exposes debug.* wholesale (debug.getupvalue /
	// setupvalue / setfenv — cross-closure tampering; sandbox-breaking).
	// Default false = strict. EXCEPTION: debug.traceback is re-exposed
	// as an INTERNAL-only global (__envoy_traceback) for the panic-
	// wrapper's use (NOT in the script's namespace) regardless of this
	// flag — wired at NewVM.
	AllowDebug bool

	// AllowPackage exposes package.* wholesale (filesystem-search
	// loader loLoaderLua reads .lua from disk). Default false = strict.
	AllowPackage bool

	// AllowChannel exposes gopher-lua's channel stdlib (Go-native chan
	// exposure to Lua). No LuaJIT counterpart; no upstream-parity
	// argument. Default false = strict.
	AllowChannel bool

	// AllowCoroutine exposes the coroutine stdlib. Default false;
	// flipped to true by applyZeroValueDefaults to match upstream
	// luaL_openlibs opening it per AMEND-A4. 22.2's :body() /
	// :httpCall() may consume internally.
	AllowCoroutine bool
}

// applyZeroValueDefaults flips AllowCoroutine + AllowOSTimeHelpers to
// true at the bare zero-valued SandboxConfig posture to match upstream
// luaL_openlibs opening these libs (per AMEND-A4 + the §3.3 sandbox-
// roster table). The strict-deny posture for the other Allow* fields
// is preserved: AllowBaseFull / AllowIO / AllowOS / AllowDebug /
// AllowPackage / AllowChannel STAY at their false default unless the
// caller explicitly set them.
//
// Consumed by NewVM (vm.go) to resolve the sandbox posture before the
// selective per-stdlib OpenXxx + post-walk nil-out application.
//
// CAVEAT: the zero-value posture means a caller CANNOT explicitly opt
// OUT of coroutine + os-time helpers via SandboxConfig alone — both
// always-become-true. This is intentional per AMEND-A4 (upstream
// luaL_openlibs parity); any future explicit-opt-out would require a
// distinct field (e.g., a `DenyCoroutine` flag), which is deliberately
// NOT introduced at 22.1 (YAGNI).
func applyZeroValueDefaults(sb SandboxConfig) SandboxConfig {
	// Unconditional: opt-out is impossible per the CAVEAT above — the
	// zero value and an explicit true both resolve to true.
	sb.AllowCoroutine = true
	sb.AllowOSTimeHelpers = true
	return sb
}

// deniedBaseGlobals lists the 9 base-stdlib globals nilled out at the
// default sandbox per 22.1 SPEC §3.3 (when AllowBaseFull == false).
// Order is alphabetical; mirrors the SPEC table reading order so a
// reviewer can scan the SPEC + this slice side-by-side without
// reordering noise.
var deniedBaseGlobals = []string{
	"collectgarbage",
	"dofile",
	"getfenv",
	"load",
	"loadfile",
	"loadstring",
	"module",
	"require",
	"setfenv",
}

// deniedOsNonTimeHelpers lists the 7 os.* entries nilled out at the
// default sandbox per 22.1 SPEC §3.3 (when AllowOSTimeHelpers is true
// AND AllowOS is false — i.e., we opened the os table for the read-
// only time helpers but must strip the dangerous entries). The 4
// preserved time helpers (time / date / clock / difftime) are NOT in
// this list.
var deniedOsNonTimeHelpers = []string{
	"execute",
	"exit",
	"getenv",
	"remove",
	"rename",
	"setlocale",
	"tmpname",
}

// applySandbox configures the *lua.LState per the SandboxConfig roster.
// MUST be called exactly once per *lua.LState (immediately after
// lua.NewState(lua.Options{SkipOpenLibs: true})); otherwise re-opens
// would leak globals back into the environment + violate the §3.3
// strict-deny posture.
//
// The function is called by NewVM (vm.go) with the resolved (post
// applyZeroValueDefaults) SandboxConfig.
func applySandbox(state *lua.LState, sb SandboxConfig) {
	// Always-opened core libs per §3.3 (base / table / string / math
	// are safe in all postures). lua.OpenBase MUST run BEFORE the
	// post-walk nil-out because the nil-out targets the freshly-bound
	// base globals.
	lua.OpenBase(state)
	lua.OpenTable(state)
	lua.OpenString(state)
	lua.OpenMath(state)

	// Conditional opens. Order is independent (each open registers its
	// own module table without cross-deps for the libs listed below);
	// holding the order stable for cross-reviewer scan-friendliness.
	if sb.AllowCoroutine {
		lua.OpenCoroutine(state)
	}
	if sb.AllowOS || sb.AllowOSTimeHelpers {
		lua.OpenOs(state)
		// When AllowOS is false (so we got here via AllowOSTimeHelpers),
		// the os table is open for the 4 read-only time helpers only; the
		// 7 dangerous entries get nilled. When AllowOS is true the os
		// table is left as-opened with no post-walk strip.
		if !sb.AllowOS {
			nilOutOsNonTimeHelpers(state)
		}
	}
	if sb.AllowIO {
		lua.OpenIo(state)
	}
	if sb.AllowDebug {
		lua.OpenDebug(state)
	}
	if sb.AllowPackage {
		lua.OpenPackage(state)
	}
	if sb.AllowChannel {
		lua.OpenChannel(state)
	}

	// Base post-walk nil-out. Runs unless AllowBaseFull preserves the
	// full base posture (then NONE of the 9 dangerous base globals
	// are stripped).
	if !sb.AllowBaseFull {
		nilOutDeniedBaseGlobals(state)
	}
}

// nilOutDeniedBaseGlobals walks the 9 denied base globals listed in
// deniedBaseGlobals and replaces each with lua.LNil at the global
// scope. Scripts indexing or calling these names get a runtime error
// ("attempt to call a nil value" / "attempt to index a nil value")
// — sandbox-strict departure per AMEND-1.
func nilOutDeniedBaseGlobals(state *lua.LState) {
	for _, name := range deniedBaseGlobals {
		state.SetGlobal(name, lua.LNil)
	}
}

// nilOutOsNonTimeHelpers walks the 7 denied os entries listed in
// deniedOsNonTimeHelpers and replaces each entry in the `os` module
// table with lua.LNil. Preserves the 4 time helpers (time / date /
// clock / difftime) per the upstream-parity time arm. Scripts indexing
// these stripped entries get a runtime error.
//
// If the os global is not a *lua.LTable (defensive — should not happen
// after lua.OpenOs), the function is a no-op rather than panicking.
func nilOutOsNonTimeHelpers(state *lua.LState) {
	osTable, ok := state.GetGlobal("os").(*lua.LTable)
	if !ok {
		return
	}
	for _, name := range deniedOsNonTimeHelpers {
		osTable.RawSetString(name, lua.LNil)
	}
}
