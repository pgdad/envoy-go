package lua

// compiled_config.go — buildCompiledConfig + 18-arm PARSE-REJECT roster per
// 22.1 SPEC §4.2 + parent SPEC §6.1 + §6.2 + §12-D1 closure. Lands the
// per-listener compiledConfig struct that the Task 10 New() factory body
// will hold + the Task 9 decode/encode hot path consumes.
//
// # 18-arm PARSE-REJECT roster — Task 2 disposition
//
// The parent SPEC §6.2 catalog defines 18 arms; this file lands the 14 arms
// that the Task 2 IMPL surface can enforce directly. The remaining arms fan
// out across the broader 16-task graph per the task-decomposition discipline
// at 22.1 SPEC §6 + PLAN D-P4:
//
//   - Arm 1  (typed-config-required)           — IMPL HERE — parseRejectTypedConfigRequired
//   - Arm 2  (typed-config-unmarshal)          — IMPL HERE — wrapParseRejectTypedConfigUnmarshal
//   - Arm 3  (inline-code-deprecated-rejected) — IMPL HERE — parseRejectInlineCodeDeprecated
//   - Arm 4  (source-codes-consume-22-3)       — IMPL HERE — parseRejectSourceCodesKeyEmpty +
//                                                wrapParseRejectSourceCodesValueFmt (phase 22.3
//                                                Task 1 lifts the deferred reject to a full
//                                                consume path)
//   - Arm 5  (default-source-code-required)    — D1-REFUTED; SILENT NO-OP (see §D1 below).
//                                                The wording constant
//                                                parseRejectDefaultSourceCodeRequired is
//                                                reserved verbatim for the proto/policy
//                                                evolution path (mirrors phase-21's
//                                                parseRejectFixedValueDeferred reserved-
//                                                constant precedent for an unreachable
//                                                roster row).
//   - Arms 6-15 (DataSource resolution)        — IMPL AT TASK 3 (datasource.go).
//                                                Task 2 wires the stub
//                                                resolveDataSource (Task 3 will land the
//                                                full 10-arm DataSource gauntlet); the
//                                                stub at Task 2 handles only the
//                                                InlineString arm so the happy-path
//                                                tests can exercise the parse-through-
//                                                compile pipeline.
//   - Arm 16 (script-compile-failed)           — IMPL AT TASK 4 (internal/lua/compile.go).
//                                                The Task 1 skeleton CompileScript is a
//                                                stub returning (&Chunk{}, nil); Task 4
//                                                IMPL wires the gopher-lua LoadString
//                                                invocation + *lua.ApiError-wrap per
//                                                wrapParseRejectScriptCompileFailed.
//   - Arm 17 (script-missing-required-hooks)   — D1-REFUTED; SILENT NO-OP (see §D1 below).
//                                                Enforcement at the 22.1 SPEC §4.3
//                                                runtime hook-presence check (Task 9)
//                                                — when the script defines NEITHER
//                                                envoy_on_request NOR envoy_on_response
//                                                the runtime path returns Continue
//                                                without invoking the gopher-lua VM.
//                                                The wording constant
//                                                parseRejectScriptMissingHooks is
//                                                reserved verbatim for the same proto/
//                                                policy evolution path as arm 5.
//   - Arm 18 (per-route validation)             — IMPL AT PHASE 22.3 TASK 2 — the real
//                                                3-arm LuaPerRoute validator lives in
//                                                perroute.go::parsePerRouteLua. The
//                                                wording constants for the 4 per-route
//                                                reject arms live in perroute.go. The
//                                                deferred const parseRejectPerRouteDeferred
//                                                was retired at Task 2.
//
// # D1 closure (per 22.1 SPEC §12-D1 + parent §12-D1)
//
// The §12-D1 question — does upstream Envoy v1.37.2 PARSE-REJECT or silently
// no-op for (a) default_source_code absent and (b) script-missing-required-
// hooks — was closed empirically at Task 2 IMPL first action via WebFetch
// scrape of:
//
//   - source/extensions/filters/http/lua/config.cc (v1.37.2)
//   - source/extensions/filters/http/lua/lua_filter.cc (v1.37.2)
//
// **Disposition: REFUTED both arms 5 + 17.** Verbatim upstream evidence
// captured in this commit's PROGRESS.md entry. Per parent §12-D1 + 22.1
// SPEC §12-D1 closure clause, both arms flip to SILENT NO-OP (degraded pass-
// through) at envoy-go to match upstream's leniency:
//
//   - lua_filter.cc:1463-1474 — when proto.has_default_source_code() AND
//     proto.inline_code().empty() are both false, the constructor's
//     if/else-if chain has no terminal-else; default_lua_code_setup_ stays
//     uninitialized; the filter loads as a pass-through.
//   - lua_filter.cc:174-181 — when registerGlobal("envoy_on_request") returns
//     LUA_REFNIL, upstream emits ENVOY_LOG(info, ...) — no exception.
//
// The wording constants for arms 5 + 17 are reserved here verbatim for the
// "go-strict policy bump" migration path (if a future envoy-go policy phase
// flips back to PARSE-REJECT either arm, the constant + the rejection branch
// land here without text drift).
//
// # Byte-stable wording discipline (per parent SPEC §6.1)
//
// All `parseReject*` constants follow the format
// `"lua: <field_path>: <reason> [; <forward-pointer hint>]"`. The `lua:`
// prefix is invariant per ADR-0080 + parent §6.1. Per ADR-0044 atomic-edit
// discipline: any wording change touches both this file + the byte-exact
// roster in parent SPEC §6.2 in a single commit. The 28th-fuzzer corpus
// (Task 11 FuzzLuaConfigParse) consumes the byte-exact wording for the
// per-arm seed assertions.
//
// # Cross-references
//
//   - ADR-0072 (boot-time-fail-fast — buildCompiledConfig runs at HCM-build,
//     errors surface to the operator as boot-time config mistakes)
//   - ADR-0080 (envoy-go-strict departure discipline + byte-stable error
//     wording)
//   - ADR-0085 (nil-tolerance discipline — CompileCache lifetime tied to
//     compiledConfig; no nil-cache crash path)
//   - ADR-0188 (NEW internal/lua/ framework primitive — CompileScript +
//     CompileCache contract)
//   - ADR-0189 (NEW internal/filter/http/lua/ package shape — §Decision body
//     records the D1-REFUTED disposition at Task 16)
//   - parent SPEC §6.1 (wording discipline) + §6.2 (18-arm roster) + §12-D1
//     (D1 closure anchor)
//   - 22.1 SPEC §4.2 (compiledConfig + filterStats shape) + §12-D1 (closure
//     forward-pointer to Task 2)

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	"google.golang.org/protobuf/types/known/anypb"

	internallua "github.com/pgdad/envoy-go/internal/lua"
	"github.com/pgdad/envoy-go/internal/stats"
)

// statNameRegexLiteral is the byte-exact regex source used by
// `internal/stats/registry.go::nameRE`. Reproduced here so the arm-19
// PARSE-REJECT wording carries the operator-actionable regex literal in
// the error message. Drift between this literal + `nameRE.String()` in
// the stats package is regression-pinned at
// `TestParseRejectStatPrefixInvalid_RegexLiteralMatches` (per the
// statNameRegexLiteral assertion in compiled_config_test.go follow-up).
const statNameRegexLiteral = `^[a-zA-Z_]([a-zA-Z0-9_.]*[a-zA-Z0-9_])?$`

// -----------------------------------------------------------------------------
// compiledConfig — per-listener immutable post-parse Lua filter config per
// 22.1 SPEC §4.2.
// -----------------------------------------------------------------------------

// compiledConfig is the per-listener compiled Lua filter configuration per
// 22.1 SPEC §4.2. Populated by buildCompiledConfig at HCM-build time per
// ADR-0072 boot-time-fail-fast.
//
// Field-final at 22.1; 22.2 adds the body-buffering interaction state (likely
// a *bodyBufferConfig field); 22.3 adds the SourceCodes map (likely a
// map[string]*lua.Chunk field) + the per-route TPFC dispatch state. 22.1
// reserves no fields for future deltas; field-additive growth at 22.2 + 22.3.
type compiledConfig struct {
	// chunk is the pre-compiled default_source_code Lua chunk. NIL when the
	// proto omits default_source_code (D1-REFUTED arm 5 silent-no-op path);
	// the runtime hook-dispatch at Task 9 decode_headers.go treats nil-chunk
	// as "no script defined → pass through" without invoking the gopher-lua
	// VM. Single chunk at 22.1; SourceCodes map adds 22.3.
	chunk *internallua.Chunk

	// sourceCodes is the name → *Chunk registry populated from
	// Lua.source_codes at parse time (phase 22.3 Task 1). NIL when the
	// proto has no source_codes entries (zero-overhead nil-map read). The
	// map is immutable post-construction; no synchronization required for
	// reads from the decode/encode hot path.
	sourceCodes map[string]*internallua.Chunk

	// perRouteChunks is the D-P1 b' per-route source_code override memo,
	// keyed by the stable *LuaPerRoute proto pointer and populated lazily by
	// resolveDecodeScript (via resolvePerRouteSourceCode) on first per-route
	// dispatch. Guarded by perRouteMu. Declared at Task 1 so the
	// compiledConfig shape was settled for Tasks 2-3 without a struct-layout
	// churn; consumed by the Task 3 per-route override dispatch.
	perRouteChunks map[*luav3.LuaPerRoute]*internallua.Chunk

	// perRouteMu guards perRouteChunks (lazy allocation + concurrent
	// per-route override compilation from multiple stream goroutines).
	// Declared at Task 1; locked by Task 3. sync.Mutex zero value is valid
	// (unlocked); no init required.
	perRouteMu sync.Mutex

	// compileCache is the content-addressed chunk cache (per 22.1 SPEC §3.1).
	// Held by the compiledConfig so its lifetime matches the listener-config
	// lifetime; GC-driven eviction (no manual evict). Non-nil even when chunk
	// is nil (the cache holder still allocates so any future SourceCodes
	// additions land in the same cache).
	compileCache *internallua.CompileCache

	// sandbox carries the SandboxConfig resolved at parse time (zero-value
	// at 22.1 — no Lua proto knob; envoy-go-strict default per AMEND-1).
	// Consumed by the Task 5 NewVM call at decode/encode hot path.
	sandbox internallua.SandboxConfig

	// stats is the SHARED-across-listener 3-counter HCM-rooted stat surface
	// per 22.1 SPEC §4.2 + parent §7 + AMEND-3. Wired at Task 10 (newFilterStats
	// call inside lua.New). At Task 2 the field is declared but left nil —
	// buildCompiledConfig has no access to the stat-registry. The pointer
	// lands when Task 10's lua.New invokes newFilterStats(ctx.Stats, ...).
	stats *filterStats //nolint:unused // wired at Task 10 (lua.New → newFilterStats per 22.1 SPEC §4.2)
}

// -----------------------------------------------------------------------------
// PARSE-REJECT byte-stable wordings per parent SPEC §6.2.
//
// EVERY constant prefix is `lua:` per parent §6.1 + ADR-0080. Byte-exact
// wording pinned at compiled_config_test.go::TestParseRejectConstants_
// ByteExactWording. Any drift requires a parent-SPEC §6.2 + ADR-0189 lockstep
// edit per ADR-0044 atomic-edit discipline.
// -----------------------------------------------------------------------------

const (
	// Arm 1: typed_config required.
	parseRejectTypedConfigRequired = "lua: typed_config required"

	// Arm 3: inline_code is deprecated; reject (envoy-go-strict departure
	// from upstream's "honor with deprecation-warn-log" disposition;
	// mirrors phase-20 §20.P6 deprecated-ConfigSource.path PARSE-REJECT
	// precedent).
	parseRejectInlineCodeDeprecated = "lua: inline_code is deprecated; use default_source_code"

	// source_codes key-empty (arm-group 1 of the phase 22.3 consume path).
	// Fires when a source_codes map entry has an empty-string key.
	parseRejectSourceCodesKeyEmpty = "lua: source_codes: key must be non-empty"

	// Arm 5: default_source_code required.
	//
	// **D1-REFUTED — reserved-verbatim wording for the proto/policy bump
	// migration path.** Upstream Envoy v1.37.2 (lua_filter.cc:1463-1474)
	// silently accepts absent-default_source_code as a no-op; envoy-go
	// matches per parent §12-D1 REFUTED disposition. The constant is
	// preserved verbatim so that any future envoy-go policy phase flipping
	// arm 5 back to PARSE-REJECT can land the rejection branch without text
	// drift. Pattern mirrors phase-21 adaptive_concurrency's
	// parseRejectFixedValueDeferred reserved-constant precedent for an arm
	// that is enforced in source but unreachable at the current proto
	// revision.
	//
	//nolint:unused // reserved verbatim for D1-REFUTED policy-bump migration path
	parseRejectDefaultSourceCodeRequired = "lua: default_source_code required"

	// Arm 17: script defines neither envoy_on_request nor envoy_on_response.
	//
	// **D1-REFUTED — reserved-verbatim wording for the proto/policy bump
	// migration path.** Upstream Envoy v1.37.2 (lua_filter.cc:174-181) only
	// emits ENVOY_LOG(info, ...) when registerGlobal returns LUA_REFNIL —
	// no exception. envoy-go matches: the runtime hook-presence check at
	// Task 9 decode_headers.go / encode_headers.go skips the CallGlobal step
	// when the hook is absent (per 22.1 SPEC §4.3 step 4). The constant is
	// preserved verbatim for the same proto/policy evolution path as arm 5
	// per ADR-0044 in-place edit discipline.
	//
	//nolint:unused // reserved verbatim for D1-REFUTED policy-bump migration path
	parseRejectScriptMissingHooks = "lua: default_source_code: script defines neither envoy_on_request nor envoy_on_response"

	// source_codes value error wrap format (arm-group 2 of the phase 22.3
	// consume path). Applied when resolveDataSource or CompileScript fails
	// for a named entry. Verbs: %q (key name) + %w (wrapped inner error).
	wrapParseRejectSourceCodesValueFmt = "lua: source_codes[%q]: %w"

	// Arm 19: stat_prefix contains characters outside the
	// `internal/stats/registry.go::nameRE` permitted class. Without this
	// pre-check, `stats.NewCounter` PANICS at `newFilterStats` time when
	// the operator supplies a `Lua.stat_prefix` containing `-`, ` `,
	// `/`, etc. — taking down `lua.New` per ADR-0072 boot-time-fail-fast
	// (the panic propagates to listener construction, killing the
	// proxy).
	//
	// Surfaced by Task 11 fuzzer FuzzLuaConfigParse first-run: random
	// stat_prefix bytes (e.g. `"0 "`, `"my-script-prefix"`) decoded
	// cleanly as a Lua proto + cleared arms 1-17 + reached
	// newFilterStats with an invalid `Lua.stat_prefix`, triggering the
	// `stats: invalid metric name` panic. Pattern mirrors the
	// hcm/config.go:209 + cluster/manager.go:205 `stats.IsValidName`
	// pre-check precedent — reject at the user-input boundary so the
	// failure surfaces as a clean PARSE-REJECT per ADR-0072 instead of
	// a panic per ADR-0059 (Registry.NewCounter boot-time panic
	// discipline).
	//
	// Wording template carries the operator-supplied stat_prefix +
	// the regex itself for actionable diagnostics. The %q applies to
	// the prefix; the byte-exact regex literal is appended for the
	// "must match" tail.
	parseRejectStatPrefixInvalidFmt = "lua: stat_prefix: invalid characters in %q (must match %s)"
)

// wrapParseRejectTypedConfigUnmarshal wraps the proto-decoder error from
// anypb.UnmarshalTo with the byte-stable arm-2 prefix per parent §6.2 arm 2.
// The inner error carries proto-level context (e.g. failing field's wire tag)
// useful for operator first-pass diagnostics — mirrors phase-21 adaptive_
// concurrency + phase-20 oauth2 typed_config-unmarshal wrap precedent.
func wrapParseRejectTypedConfigUnmarshal(inner error) error {
	return fmt.Errorf("lua: typed_config unmarshal: %w", inner)
}

// wrapParseRejectScriptCompileFailed wraps the gopher-lua *lua.ApiError
// returned by internal/lua/CompileScript (Task 4 IMPL) with the byte-stable
// arm-16 prefix per parent §6.2 arm 16. The structural prefix is byte-stable;
// the wrapped error carries variable bytes (file name + line number from
// gopher-lua's parser). At Task 2 the Task 1 skeleton CompileScript always
// returns nil error, so this wrap helper has no live call path; Task 4 IMPL
// flips the call path live by returning real *lua.ApiError values.
func wrapParseRejectScriptCompileFailed(inner error) error {
	return fmt.Errorf("lua: default_source_code: compile: %w", inner)
}

// -----------------------------------------------------------------------------
// buildCompiledConfig — full proto-parser body per 22.1 SPEC §6 Task 2 +
// parent §6.1.
// -----------------------------------------------------------------------------

// buildCompiledConfig parses the Lua proto envelope (wrapped in *anypb.Any) +
// constructs the per-listener compiledConfig per 22.1 SPEC §4.2 + parent §6.2
// 18-arm PARSE-REJECT roster. Per ADR-0072 boot-time-fail-fast: any validation
// failure returns the byte-stable error string verbatim.
//
// Arm ordering (parent §6.2 + D1-REFUTED disposition for arms 5 + 17):
//
//  1. arm 1  — typedConfig nil (PARSE-REJECT)
//  2. arm 2  — typedConfig.UnmarshalTo(*Lua) fails (PARSE-REJECT, wrapped)
//  3. arm 3  — InlineCode != "" (PARSE-REJECT)
//  4. arm 4  — source_codes consume path (phase 22.3 Task 1): sorted-key
//     iteration; reject empty keys (arm-group 1); resolveDataSource +
//     CompileScript each value (arm-group 2, prefix source_codes[%q]:);
//     populate cc.sourceCodes registry into SHARED compileCache.
//  5. arm 5  — DefaultSourceCode == nil → silent no-op (D1-REFUTED). Returns
//     *compiledConfig with chunk == nil; runtime hot path treats
//     nil-chunk as "no script defined → pass through."
//  6. arms 6-15 — DataSource resolution via resolveDataSource (datasource.go).
//     At Task 2 the stub handles only InlineString; Task 3 IMPL
//     lands the full 10-arm DataSource gauntlet.
//  7. arm 16 — internal/lua.CompileScript wraps *lua.ApiError (Task 4 IMPL;
//     Task 1 skeleton stub always succeeds).
//  8. arm 17 — silent no-op (D1-REFUTED; no parse-time check).
//
// arm 18 (per-route) is enforced via the separate HCM RegisterPerRouteValidator
// path in lua.go::validatePerRouteLua per ADR-0110 single-chokepoint; not a
// buildCompiledConfig concern.
func buildCompiledConfig(typedConfig *anypb.Any) (*compiledConfig, error) {
	// Arm 1: typed_config required.
	if typedConfig == nil {
		return nil, errors.New(parseRejectTypedConfigRequired)
	}

	// Arm 2: typed_config.UnmarshalTo(*Lua) failure.
	var m luav3.Lua
	if err := typedConfig.UnmarshalTo(&m); err != nil {
		return nil, wrapParseRejectTypedConfigUnmarshal(err)
	}

	// Arm 19: stat_prefix character-class pre-check. Surfaced by Task 11
	// fuzzer (FuzzLuaConfigParse first-run): without this guard,
	// `stats.NewCounter` PANICS via `nameRE` mismatch when the operator
	// supplies a `Lua.stat_prefix` containing `-`, ` `, `/`, etc. —
	// taking down `lua.New` per the listener-construction call path.
	// Mirrors the hcm/config.go:209 + cluster/manager.go:205 pre-check
	// precedent. Empty-prefix passes through (the consecutive-dot path
	// is RATIFIED per AMEND-2 + parent §7.2; the registry regex permits
	// interior consecutive dots — only trailing dots are rejected). The
	// check uses the assembled-name probe pattern (mirrors HCM)
	// because nameRE is whole-string anchored; a bare-prefix check
	// would mis-accept inputs like "" (valid as prefix → consecutive-
	// dot literal) or single-char inputs that pass in isolation but
	// fail when prefixed by `http.<hcm>.lua.`.
	if sp := m.GetStatPrefix(); sp != "" {
		probe := "http.lua." + sp + ".errors"
		if !stats.IsValidName(probe) {
			return nil, fmt.Errorf(parseRejectStatPrefixInvalidFmt, sp, statNameRegexLiteral)
		}
	}

	// Arm 3: inline_code is deprecated; reject (envoy-go-strict departure).
	// SA1019 deliberate: this arm EXISTS to enforce rejection of the
	// deprecated proto field per parent §6.2 arm 3. The `nolint` suppresses
	// the staticcheck noise without losing the contract: accessing a
	// deprecated proto field is intentional here.
	if m.GetInlineCode() != "" { //nolint:staticcheck // SA1019: arm 3 EXISTS to PARSE-REJECT this deprecated proto field; intentional access.
		return nil, errors.New(parseRejectInlineCodeDeprecated)
	}

	// Compile cache lives for the compiledConfig lifetime per 22.1 SPEC §4.2
	// + PLAN D-P5 (GC-driven eviction; no cross-listener / cross-process
	// global cache). Allocated even on the silent-no-op path so the cache
	// holder shape is uniform; the (zero-content) cache costs ~few bytes.
	compileCache := internallua.NewCompileCache()

	// Phase 22.3 Task 1: consume source_codes → name→*Chunk registry.
	// Iterate in sorted-key order for deterministic compile ordering +
	// deterministic error reporting. Reject empty-string keys (arm-group 1).
	// Wrap DataSource resolution + compile errors with the source_codes[%q]:
	// prefix (arm-group 2). Compile into the SHARED compileCache so
	// byte-identical entries across source_codes AND default_source_code
	// dedup to the same *Chunk pointer via content-hash.
	var sourceCodes map[string]*internallua.Chunk
	if rawMap := m.GetSourceCodes(); len(rawMap) > 0 {
		// Sort keys for deterministic iteration.
		keys := make([]string, 0, len(rawMap))
		for k := range rawMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		sourceCodes = make(map[string]*internallua.Chunk, len(rawMap))
		for _, name := range keys {
			// Arm-group 1: empty key is a misconfiguration — reject immediately.
			if name == "" {
				return nil, errors.New(parseRejectSourceCodesKeyEmpty)
			}
			// Arms 6-15: DataSource resolution for this entry.
			src, dsErr := resolveDataSource(rawMap[name])
			if dsErr != nil {
				return nil, fmt.Errorf(wrapParseRejectSourceCodesValueFmt, name, dsErr)
			}
			// Arm 16: compile into the shared cache.
			ch, compErr := internallua.CompileScript(src, compileCache)
			if compErr != nil {
				return nil, fmt.Errorf(wrapParseRejectSourceCodesValueFmt, name, compErr)
			}
			sourceCodes[name] = ch
		}
	}

	// Arm 5 (D1-REFUTED): default_source_code absent → silent no-op (degraded
	// pass-through). Returns *compiledConfig with chunk == nil; the runtime
	// hook-dispatch at Task 9 decode_headers.go treats nil-chunk as "no
	// script defined → pass through." Per parent §12-D1 + the upstream
	// lua_filter.cc:1463-1474 silent-accept evidence captured in
	// PROGRESS.md.
	if m.GetDefaultSourceCode() == nil {
		return &compiledConfig{
			chunk:        nil,
			sourceCodes:  sourceCodes,
			compileCache: compileCache,
			sandbox:      internallua.SandboxConfig{}, // zero-value = StrictUpstreamParity per AMEND-1
		}, nil
	}

	// Arms 6-15: DataSource resolution. resolveDataSource is the
	// single-chokepoint dispatch into datasource.go's 10-arm gauntlet (Task 3
	// IMPL). At Task 2 the stub handles only the InlineString arm so the
	// happy-path tests can exercise the parse-through-compile pipeline.
	src, err := resolveDataSource(m.GetDefaultSourceCode())
	if err != nil {
		return nil, err
	}

	// Arm 16: gopher-lua compile failure. The Task 1 skeleton CompileScript
	// always returns (&Chunk{}, nil); Task 4 IMPL wires the real *lua.ApiError
	// surface. The wrap helper wrapParseRejectScriptCompileFailed is invoked
	// here so the call path lands ready for Task 4.
	chunk, err := internallua.CompileScript(src, compileCache)
	if err != nil {
		return nil, wrapParseRejectScriptCompileFailed(err)
	}

	// Arm 17 (D1-REFUTED): no parse-time check. The 22.1 SPEC §4.3 runtime
	// hook-presence check at Task 9 decode_headers.go / encode_headers.go
	// inspects vm.HasGlobalFunc post-script-run and skips CallGlobal when
	// the hook is absent. Per parent §12-D1 + the upstream
	// lua_filter.cc:174-181 INFO-log-only evidence captured in PROGRESS.md.

	return &compiledConfig{
		chunk:        chunk,
		sourceCodes:  sourceCodes,
		compileCache: compileCache,
		sandbox:      internallua.SandboxConfig{}, // zero-value = StrictUpstreamParity per AMEND-1
	}, nil
}
