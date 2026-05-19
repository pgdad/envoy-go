// Package lua is the gopher-lua VM lifecycle + compile-cache + sandbox
// framework primitive at the NEW top-level package `internal/lua/` per
// ADR-0188 (§Context anchored at phase 22 parent SPEC `41ccee7`;
// §Decision + §Consequences body lands at phase 22.1 IMPL Task 16 per
// ADR-0044 in-place edit discipline).
//
// Strategically positioned for cross-phase reuse: the *VM lifecycle +
// SandboxConfig per-stdlib ALLOW/DENY discipline + *Chunk content-
// addressed compile cache compose against future consumers of the
// gopher-lua VM beyond the phase-22.1 first consumer
// `internal/filter/http/lua/` — cluster_specifier Lua at
// envoy.router.cluster_specifiers.lua, access_logger Lua at
// envoy.access_loggers.lua, and string_matcher Lua at
// envoy.string_matcher.lua. The API surface carries an EXPLICIT
// API-REVISION ALLOWANCE clause per BRAINSTORM Q4 — at consumer #2
// landing, the API shape may be revised after empirical validation.
//
// # API surface (per 22.1 SPEC §3.1 production signatures)
//
//   - VM — opaque per-stream gopher-lua execution context; NOT goroutine-safe;
//     constructed via NewVM(opts ...VMOption); released via Close at OnDestroy.
//   - VMOption — function-option configurator (idiomatic Go option pattern,
//     mirrors internal/jwks/Fetcher + internal/httpclient/ precedent).
//   - WithSandboxConfig(SandboxConfig) VMOption — sets per-stdlib ALLOW/DENY
//     posture (zero value = StrictUpstreamParity per AMEND-1).
//   - WithPanicHandler(PanicHandlerFn) VMOption — invoked after recover() in
//     the VM's panic-wrapper for genuine Go panics from bridge callbacks.
//   - WithBasePrintSink(io.Writer) VMOption — redirects Lua-script print(...)
//     output; default nil = drop (envoy-go-strict; no stdout leak).
//   - (*VM).State() *lua.LState — escape-hatch for filter consumers to set up
//     userdata + metatables (request_handle / response_handle patterns).
//   - (*VM).RegisterGlobalFunc(name, lua.LGFunction) — convenience for simple
//     globals; userdata + metatables go via State().
//   - (*VM).Run(*Chunk) error — loads the chunk's *FunctionProto onto this
//     VM's *LState (cheap; no recompilation per stream) and executes script
//     top-level (defining any global functions declared by the script).
//   - (*VM).HasGlobalFunc(name) bool — true if the named global is a callable
//     function (supports D1 script-missing-required-hooks PARSE-REJECT,
//     closed at IMPL Task 2 against upstream Envoy).
//   - (*VM).CallGlobal(name, args ...lua.LValue) error — invokes the named
//     global function with args; returns *lua.ApiError on Lua runtime error.
//   - (*VM).Close() — releases the *LState; idempotent.
//   - Chunk — opaque content-addressed compiled-script representation;
//     SHA-256(src)-keyed in the optional CompileCache.
//   - CompileCache + NewCompileCache + CompileScript([]byte, *CompileCache) —
//     content-addressed compile cache (nil-tolerant per ADR-0085).
//   - SandboxConfig — 8 Allow* fields per stdlib (per §3.1); the zero value
//     after applyZeroValueDefaults matches upstream luaL_openlibs posture
//     for coroutine + os.time/date/clock/difftime; the rest STAY DENIED.
//
// # AMEND-1 — stdlib-sandbox-strict default-deny (envoy-go-strict DEPARTURE)
//
// The zero-value SandboxConfig is STRICT-DEFAULT-DENY. NewVM does NOT call
// *lua.LState.OpenLibs() (which opens everything indiscriminately); instead
// it selectively calls per-lib OpenXxx + post-walk nil-out for the denied
// functions per the §3.3 sandbox-roster table. Rationale: gopher-lua exposes
// sandbox-breaking arms (os.execute / os.exit / io.popen / package.* /
// channel / debug.getupvalue) that upstream LuaJIT in upstream Envoy's
// per-worker deployment model tolerates by per-worker VM scoping +
// operator-trust bounds — envoy-go's per-stream goroutine dispatch model
// cannot make the same assumption. The departure record lands at
// BEHAVIOR_CONTRACT.md envoy-go-strict departures section at 22.1 IMPL
// Task 16 (per parent SPEC §14 edit #3 + §16 acceptance item 6).
//
// # AMEND-9 — gopher-lua vs LuaJIT divergences
//
// gopher-lua is Lua 5.1 (matches upstream LuaJIT 5.1 dialect). Scripts using
// 5.2+ features (`goto`, integer subtype, bit32 / utf8 stdlib) get
// Lua-compile-time errors at script-source compilation (arm 16 PARSE-REJECT
// in the filter's compiled_config.go). The runtime-error log-message
// wording for caught script errors diverges from upstream Envoy's LuaJIT-
// formatted strings; documented at BEHAVIOR_CONTRACT.md envoy-go-strict
// departures section at 22.1 IMPL Task 16.
//
// # AMEND-A4 — coroutine ALLOWED by default (luaL_openlibs parity)
//
// The applyZeroValueDefaults helper flips AllowCoroutine = true on a
// bare zero-valued SandboxConfig to match upstream luaL_openlibs opening
// the coroutine library. Phase 22.1 does NOT consume coroutines at any
// bridge entry point; the 22.2 :body() / :httpCall() async surfaces may
// (22.2 SPEC settles). The AllowOSTimeHelpers field is similarly flipped
// to preserve the read-only os.time / os.date / os.clock / os.difftime
// upstream-parity arm.
//
// # Per-stream construction (WEAK-default at 22.1 per parent §13-R6)
//
// Per-stream *lua.LState construction with a SHARED per-script-source
// *Chunk cache (one cache per compiledConfig instance; GC-driven
// eviction). The Task 12 BenchmarkPerStreamLState_Construction_Headers
// gates the WEAK-default: if per-stream ns/op exceeds 1ms threshold,
// ADR-0190 escape-valve fires at Task 16 anchoring a per-script-source
// *LState pool. Until benchmarked, hold per-stream construction.
//
// # Cross-references
//
//   - ADR-0188 (NEW internal/lua/ framework primitive; §Decision +
//     §Consequences body lands at 22.1 IMPL Task 16).
//   - 22.1 SPEC §3.1 — production API signatures.
//   - 22.1 SPEC §3.2 — file split (3 production + 3 test files).
//   - 22.1 SPEC §3.3 — sandbox roster (default-deny table).
//   - 22.1 SPEC §3.4 — per-stream *LState construction discipline.
//   - 22 parent SPEC §4.1 + §4.2 + §4.3 — framework-primitive design.
//   - 22 parent SPEC §1.1 AMEND-1 + AMEND-9 + AMEND-A4 — empirical
//     scope revisions.
package lua
