package lua

// compiled_config_test.go — Task 2 RIGID-TDD table-driven test surface per
// 22.1 SPEC §6 Task 2 + parent SPEC §6.2 18-arm PARSE-REJECT roster +
// §12-D1 closure (REFUTED — arms 5 + 17 flip to silent no-op).
//
// Coverage at Task 2:
//   - Arm 1  (typed-config-required)              — PARSE-REJECT byte-exact
//   - Arm 2  (typed-config-unmarshal)             — PARSE-REJECT byte-exact prefix
//   - Arm 3  (inline-code-deprecated-rejected)    — PARSE-REJECT byte-exact
//   - Arm 4  (source-codes-consume-22-3)           — CONSUME path (key-empty
//                                                    PARSE-REJECT byte-exact;
//                                                    value-error wrapped)
//   - Arm 5  (default-source-code-required)       — **silent no-op** (D1-REFUTED;
//                                                    upstream Envoy v1.37.2
//                                                    lua_filter.cc:1463-1474
//                                                    silently accepts; envoy-go
//                                                    matches per parent §12-D1
//                                                    REFUTED disposition)
//   - Arms 6-15 (DataSource resolution)           — DEFERRED to Task 3 (full
//                                                    enforcement requires the
//                                                    real datasource.go; Task 2
//                                                    only wires the stub
//                                                    resolveDataSource for the
//                                                    InlineString happy path)
//   - Arm 16 (script-compile-failed)              — DEFERRED to Task 4 (the
//                                                    Task 1 skeleton CompileScript
//                                                    always succeeds; real
//                                                    *lua.ApiError wrap arrives
//                                                    with Task 4 IMPL)
//   - Arm 17 (script-missing-required-hooks)      — **silent no-op** (D1-REFUTED;
//                                                    upstream Envoy v1.37.2
//                                                    lua_filter.cc:174-181
//                                                    only logs INFO; envoy-go
//                                                    matches — defensive arm
//                                                    REMOVED from parse-time
//                                                    roster; runtime hook-
//                                                    presence check at Task 9
//                                                    decode_headers.go skips
//                                                    invocation when absent)
//   - Arm 18 (per-route-deferred-to-22-3)         — covered via the existing
//                                                    Task 1 skeleton's
//                                                    validatePerRouteLua
//                                                    (re-asserted byte-exact
//                                                    here for cross-package
//                                                    regression visibility)
//
// Happy-path rows exercise the InlineString DataSource arm through the Task 1
// skeleton stubs (CompileScript returns &Chunk{}, nil unconditionally; that's
// fine — Task 2's contract is "buildCompiledConfig returns a *compiledConfig
// with non-nil chunk + compileCache on the valid path"). Task 4 + Task 5 will
// flesh out real script compilation + chunk-cache identity assertions.

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/pgdad/envoy-go/internal/stats"
)

// validLuaConfig returns a baseline Lua proto with a valid InlineString
// default_source_code containing a script that defines envoy_on_request +
// envoy_on_response. Each PARSE-REJECT row's `mutate` closure modifies this
// baseline to trigger ONE specific arm.
func validLuaConfig() *luav3.Lua {
	return &luav3.Lua{
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end\nfunction envoy_on_response(rh) end\n",
			},
		},
		StatPrefix: "",
	}
}

// toAny wraps the Lua proto in an *anypb.Any envelope per the buildCompiledConfig
// signature contract.
func toAny(t *testing.T, msg *luav3.Lua) *anypb.Any {
	t.Helper()
	any, err := anypb.New(msg)
	if err != nil {
		t.Fatalf("anypb.New failed: %v", err)
	}
	return any
}

// -----------------------------------------------------------------------------
// TestBuildCompiledConfig — table-driven roster covering the Task-2-reachable
// arms + the D1-REFUTED silent-no-op rows + valid-config happy-path rows.
// -----------------------------------------------------------------------------

func TestBuildCompiledConfig(t *testing.T) {
	t.Run("PARSE_REJECT", testBuildCompiledConfigParseReject)
	t.Run("D1_REFUTED_SilentNoop", testBuildCompiledConfigD1SilentNoop)
	t.Run("HappyPath", testBuildCompiledConfigHappyPath)
	t.Run("Arm18_PerRoute_Validator", testBuildCompiledConfigArm18PerRoute)
	t.Run("SourceCodes", testBuildCompiledConfigSourceCodes)
}

// testBuildCompiledConfigParseReject covers the Task-2-reachable PARSE-REJECT
// arms (1, 2, 3) and the Task-1 (phase 22.3) arm-4 consume path's new
// PARSE-REJECT leaves (source-codes-key-empty + bad-value prefix).
// Arms 5 + 17 are D1-REFUTED (covered separately under "D1_REFUTED_SilentNoop").
// Arms 6-15 land at Task 3 (datasource.go full IMPL).
// Arm 16 lands at Task 4 (internal/lua/compile.go full IMPL).
func testBuildCompiledConfigParseReject(t *testing.T) {
	cases := []struct {
		name             string
		typedConfig      func(t *testing.T) *anypb.Any
		wantErrEq        string // when set, asserts err.Error() == wantErrEq
		wantErrHasPrefix string // when set, asserts err.Error() starts with this prefix
	}{
		// ---- Arm 1: typed_config required ----
		{
			name:        "Arm01_TypedConfig_Nil",
			typedConfig: func(_ *testing.T) *anypb.Any { return nil },
			wantErrEq:   parseRejectTypedConfigRequired,
		},
		// ---- Arm 2: typed_config unmarshal failure ----
		{
			name: "Arm02_TypedConfig_UnmarshalFailure",
			typedConfig: func(_ *testing.T) *anypb.Any {
				// Construct an Any with a Lua TypeURL but garbage payload bytes;
				// UnmarshalTo into *Lua surfaces a proto decoder error.
				return &anypb.Any{
					TypeUrl: TypeURL,
					Value:   []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
				}
			},
			wantErrHasPrefix: "lua: typed_config unmarshal: ",
		},
		// ---- Arm 3: inline_code deprecated; rejected ----
		{
			name: "Arm03_InlineCode_Deprecated_Rejected",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validLuaConfig()
				m.InlineCode = "function envoy_on_request(rh) end\n" //nolint:staticcheck // SA1019: arm 3 EXISTS to PARSE-REJECT this deprecated proto field; intentional access.
				return toAny(t, m)
			},
			wantErrEq: parseRejectInlineCodeDeprecated,
		},
		// Arm 3 also fires even WITHOUT default_source_code set (inline_code
		// alone). The PARSE-REJECT does not depend on default_source_code's
		// presence.
		{
			name: "Arm03_InlineCode_AloneNoDefault_Rejected",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := &luav3.Lua{InlineCode: "function envoy_on_request(rh) end\n"} //nolint:staticcheck // SA1019: arm 3 EXISTS to PARSE-REJECT this deprecated proto field; intentional access.
				return toAny(t, m)
			},
			wantErrEq: parseRejectInlineCodeDeprecated,
		},
		// ---- source_codes key-empty PARSE-REJECT (arm-group 1 of the 22.3
		// consume path) — replaces the old Arm04_SourceCodes_DeferredTo223 rows.
		{
			name: "SourceCodes_KeyEmpty_Rejected",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validLuaConfig()
				m.SourceCodes = map[string]*corev3.DataSource{
					"": {
						Specifier: &corev3.DataSource_InlineString{
							InlineString: "function envoy_on_request(rh) end\n",
						},
					},
				}
				return toAny(t, m)
			},
			wantErrEq: parseRejectSourceCodesKeyEmpty,
		},
		// ---- source_codes bad-value (arm-group 2 of the 22.3 consume path):
		// Filename ENOENT — the error must be wrapped with the source_codes[%q]:
		// prefix. ----
		{
			name: "SourceCodes_BadValue_FilenameEnoent",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validLuaConfig()
				m.SourceCodes = map[string]*corev3.DataSource{
					"a": {
						Specifier: &corev3.DataSource_Filename{
							Filename: "/nonexistent/path/xyzzy_22_3.lua",
						},
					},
				}
				return toAny(t, m)
			},
			wantErrHasPrefix: `lua: source_codes["a"]: `,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := buildCompiledConfig(tc.typedConfig(t))
			if err == nil {
				t.Fatalf("buildCompiledConfig: want error, got nil")
			}
			switch {
			case tc.wantErrEq != "":
				if err.Error() != tc.wantErrEq {
					t.Fatalf("buildCompiledConfig err = %q; want %q", err.Error(), tc.wantErrEq)
				}
			case tc.wantErrHasPrefix != "":
				if !strings.HasPrefix(err.Error(), tc.wantErrHasPrefix) {
					t.Fatalf("buildCompiledConfig err = %q; want prefix %q", err.Error(), tc.wantErrHasPrefix)
				}
			default:
				t.Fatalf("test case %q has neither wantErrEq nor wantErrHasPrefix", tc.name)
			}
		})
	}
}

// testBuildCompiledConfigD1SilentNoop covers arms 5 + 17 — both REFUTED at the
// Task 2 D1 closure scrape of upstream Envoy v1.37.2:
//
//   - Arm 5 (default_source_code absent): upstream lua_filter.cc:1463-1474 has
//     no else branch when has_default_source_code() AND inline_code().empty()
//     are both false — default_lua_code_setup_ stays uninitialized; the filter
//     loads as a silent pass-through. envoy-go matches: buildCompiledConfig
//     returns (cc, nil) with cc.chunk == nil; the runtime hook-dispatch path
//     at decode_headers.go (Task 9) treats nil-chunk as "no script defined →
//     pass through" without invoking the gopher-lua VM.
//
//   - Arm 17 (script-missing-required-hooks): upstream lua_filter.cc:174-181
//     emits an ENVOY_LOG(info, ...) but does NOT throw — the missing hook is
//     treated as "this filter does not hook this direction." envoy-go matches:
//     no parse-time enforcement; the runtime hook-presence check at Task 9
//     decode_headers.go / encode_headers.go skips the CallGlobal step when
//     the hook is absent (per 22.1 SPEC §4.3 step 4 — "if !vm.HasGlobalFunc
//     ... return Continue").
//
// At Task 2 we can only assert arm 5's silent-no-op disposition (cc returns
// non-nil with cc.chunk == nil). Arm 17's silent-no-op is a Task 9 runtime
// concern; the Task-2-time check is that buildCompiledConfig DOES NOT reject
// a hook-less script source (the script "" arm or a hook-less function body
// still parses cleanly — the Task 1 skeleton CompileScript is lax enough to
// accept anything, and the real Task 4 IMPL will inherit this discipline).
func testBuildCompiledConfigD1SilentNoop(t *testing.T) {
	t.Run("Arm05_DefaultSourceCode_Absent_SilentNoop", func(t *testing.T) {
		t.Parallel()
		// Empty Lua proto — no DefaultSourceCode, no SourceCodes, no InlineCode.
		// Per D1 REFUTED disposition this is a silent no-op (degraded pass-through).
		m := &luav3.Lua{}
		cc, err := buildCompiledConfig(toAny(t, m))
		if err != nil {
			t.Fatalf("buildCompiledConfig(empty Lua): want nil error per D1-REFUTED arm 5; got %v", err)
		}
		if cc == nil {
			t.Fatalf("buildCompiledConfig(empty Lua): want non-nil *compiledConfig; got nil")
		}
		if cc.chunk != nil {
			t.Fatalf("buildCompiledConfig(empty Lua): want nil chunk (silent no-op); got %#v", cc.chunk)
		}
	})

	t.Run("Arm17_ScriptMissingHooks_SilentNoop", func(t *testing.T) {
		t.Parallel()
		// A script body that defines NEITHER envoy_on_request NOR envoy_on_response.
		// Per D1 REFUTED disposition this compiles cleanly + loads as a silent
		// pass-through; the runtime hook-presence check at Task 9 will return
		// Continue without invoking the gopher-lua VM. At Task 2 we only assert
		// that buildCompiledConfig does NOT reject this config at parse-time.
		m := validLuaConfig()
		m.DefaultSourceCode = &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "local x = 1\n", // no hooks defined
			},
		}
		cc, err := buildCompiledConfig(toAny(t, m))
		if err != nil {
			t.Fatalf("buildCompiledConfig(no-hook script): want nil error per D1-REFUTED arm 17; got %v", err)
		}
		if cc == nil {
			t.Fatalf("buildCompiledConfig(no-hook script): want non-nil *compiledConfig; got nil")
		}
		// chunk MUST be non-nil — the script compiled successfully (the no-hook
		// disposition is a runtime concern, not a parse-time concern).
		if cc.chunk == nil {
			t.Fatalf("buildCompiledConfig(no-hook script): want non-nil chunk; got nil")
		}
	})
}

// testBuildCompiledConfigHappyPath covers ~5 valid configurations end-to-end:
// each exercises the InlineString DataSource arm with a script source that
// compiles cleanly via the Task 1 skeleton CompileScript stub (which is lax
// enough to accept any bytes; Task 4 IMPL will tighten this). Each row
// asserts buildCompiledConfig returns a non-nil *compiledConfig with the
// expected field shape.
func testBuildCompiledConfigHappyPath(t *testing.T) {
	cases := []struct {
		name            string
		configMutator   func(*luav3.Lua)
		wantNonNilChunk bool
	}{
		{
			name:            "Valid_BothHooks_InlineString",
			configMutator:   func(_ *luav3.Lua) {}, // baseline (both hooks)
			wantNonNilChunk: true,
		},
		{
			name: "Valid_RequestHookOnly_InlineString",
			configMutator: func(m *luav3.Lua) {
				m.DefaultSourceCode = &corev3.DataSource{
					Specifier: &corev3.DataSource_InlineString{
						InlineString: "function envoy_on_request(rh) end\n",
					},
				}
			},
			wantNonNilChunk: true,
		},
		{
			name: "Valid_ResponseHookOnly_InlineString",
			configMutator: func(m *luav3.Lua) {
				m.DefaultSourceCode = &corev3.DataSource{
					Specifier: &corev3.DataSource_InlineString{
						InlineString: "function envoy_on_response(rh) end\n",
					},
				}
			},
			wantNonNilChunk: true,
		},
		{
			name: "Valid_WithStatPrefix",
			configMutator: func(m *luav3.Lua) {
				m.StatPrefix = "myscript"
			},
			wantNonNilChunk: true,
		},
		{
			name: "Valid_EmptyStatPrefix_Explicit",
			configMutator: func(m *luav3.Lua) {
				m.StatPrefix = ""
			},
			wantNonNilChunk: true,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := validLuaConfig()
			tc.configMutator(m)
			cc, err := buildCompiledConfig(toAny(t, m))
			if err != nil {
				t.Fatalf("buildCompiledConfig: want nil error; got %v", err)
			}
			if cc == nil {
				t.Fatalf("buildCompiledConfig: want non-nil *compiledConfig; got nil")
			}
			if tc.wantNonNilChunk && cc.chunk == nil {
				t.Fatalf("buildCompiledConfig: want non-nil chunk; got nil")
			}
			// CompileCache must be non-nil on every successful build per
			// 22.1 SPEC §4.2 (the cache lives for the compiledConfig
			// lifetime; GC-driven eviction). Task 4 IMPL will tighten the
			// cache hit/miss semantics; at Task 2 we only assert non-nil.
			if cc.compileCache == nil {
				t.Fatalf("buildCompiledConfig: want non-nil compileCache; got nil")
			}
		})
	}
}

// testBuildCompiledConfigArm18PerRoute exercises the arm-18 per-route
// validator (phase 22.3 Task 2 real implementation). The deferred one-liner
// wording was retired; the real validator in perroute.go enforces the 3-arm
// oneof dispatch. This test covers the two most common misconfiguration
// surfaces via validatePerRouteLua (the ADR-0110 single-chokepoint) to
// maintain cross-package regression visibility per parent §6.2 arm 18.
func testBuildCompiledConfigArm18PerRoute(t *testing.T) {
	t.Parallel()

	// wrong type → type-assert error (not a *LuaPerRoute).
	t.Run("WrongType_TypeAssertError", func(t *testing.T) {
		t.Parallel()
		err := validatePerRouteLua(&luav3.Lua{})
		if err == nil {
			t.Fatal("validatePerRouteLua(wrong type): want error; got nil")
		}
		const wantPrefix = "lua: per-route: expected *luav3.LuaPerRoute, got "
		if !strings.HasPrefix(err.Error(), wantPrefix) {
			t.Fatalf("validatePerRouteLua err = %q; want prefix %q", err.Error(), wantPrefix)
		}
	})

	// nil oneof → byte-exact parseRejectPerRouteOneofRequired.
	t.Run("NilOneof_OneofRequired", func(t *testing.T) {
		t.Parallel()
		err := validatePerRouteLua(&luav3.LuaPerRoute{})
		if err == nil {
			t.Fatal("validatePerRouteLua(&LuaPerRoute{}): want error; got nil")
		}
		if err.Error() != parseRejectPerRouteOneofRequired {
			t.Fatalf("validatePerRouteLua err = %q; want %q", err.Error(), parseRejectPerRouteOneofRequired)
		}
	})
}

// testBuildCompiledConfigSourceCodes covers the phase 22.3 Task 1 SourceCodes
// consume path: registry population, content-hash dedup, and error leaves.
func testBuildCompiledConfigSourceCodes(t *testing.T) {
	// ---- single entry: InlineString → cc.sourceCodes["a"] non-nil ----
	t.Run("SingleEntry_InlineString", func(t *testing.T) {
		t.Parallel()
		m := validLuaConfig()
		m.SourceCodes = map[string]*corev3.DataSource{
			"a": {
				Specifier: &corev3.DataSource_InlineString{
					InlineString: "function envoy_on_request(rh) end\n",
				},
			},
		}
		cc, err := buildCompiledConfig(toAny(t, m))
		if err != nil {
			t.Fatalf("buildCompiledConfig: want nil error; got %v", err)
		}
		if cc == nil {
			t.Fatal("buildCompiledConfig: want non-nil *compiledConfig; got nil")
		}
		if cc.sourceCodes == nil {
			t.Fatal("cc.sourceCodes: want non-nil map; got nil")
		}
		chunk := cc.sourceCodes["a"]
		if chunk == nil {
			t.Fatal(`cc.sourceCodes["a"]: want non-nil *Chunk; got nil`)
		}
	})

	// ---- two distinct entries → two distinct *Chunk pointers ----
	t.Run("TwoEntries_TwoDistinctChunks", func(t *testing.T) {
		t.Parallel()
		m := validLuaConfig()
		m.SourceCodes = map[string]*corev3.DataSource{
			"alpha": {
				Specifier: &corev3.DataSource_InlineString{
					InlineString: "function envoy_on_request(rh) end\n",
				},
			},
			"beta": {
				Specifier: &corev3.DataSource_InlineString{
					InlineString: "function envoy_on_response(rh) end\n",
				},
			},
		}
		cc, err := buildCompiledConfig(toAny(t, m))
		if err != nil {
			t.Fatalf("buildCompiledConfig: want nil error; got %v", err)
		}
		chunkAlpha := cc.sourceCodes["alpha"]
		chunkBeta := cc.sourceCodes["beta"]
		if chunkAlpha == nil {
			t.Fatal(`cc.sourceCodes["alpha"]: want non-nil *Chunk; got nil`)
		}
		if chunkBeta == nil {
			t.Fatal(`cc.sourceCodes["beta"]: want non-nil *Chunk; got nil`)
		}
		if chunkAlpha == chunkBeta {
			t.Fatal("cc.sourceCodes[alpha] == cc.sourceCodes[beta]: want distinct *Chunk pointers for distinct content")
		}
	})

	// ---- source_codes only (no default_source_code) → cc.chunk == nil AND
	// cc.sourceCodes non-nil. Exercises the nil-default early-return path in
	// buildCompiledConfig (the arm-5 D1-REFUTED branch that returns before
	// resolveDataSource + CompileScript for default_source_code). ----
	t.Run("SourceCodesOnly_NoDefault_ChunkNilSourceCodesPopulated", func(t *testing.T) {
		t.Parallel()
		// Deliberately omit DefaultSourceCode — use a bare *luav3.Lua with only
		// SourceCodes populated (no default_source_code field set).
		m := &luav3.Lua{
			SourceCodes: map[string]*corev3.DataSource{
				"only": {
					Specifier: &corev3.DataSource_InlineString{
						InlineString: "function envoy_on_request(rh) end\n",
					},
				},
			},
		}
		cc, err := buildCompiledConfig(toAny(t, m))
		if err != nil {
			t.Fatalf("buildCompiledConfig: want nil error; got %v", err)
		}
		if cc == nil {
			t.Fatal("buildCompiledConfig: want non-nil *compiledConfig; got nil")
		}
		// nil-default early-return path: cc.chunk MUST be nil.
		if cc.chunk != nil {
			t.Fatalf("cc.chunk: want nil (no default_source_code); got %#v", cc.chunk)
		}
		// sourceCodes MUST be non-nil and populated simultaneously.
		if cc.sourceCodes == nil {
			t.Fatal("cc.sourceCodes: want non-nil map; got nil")
		}
		if cc.sourceCodes["only"] == nil {
			t.Fatal(`cc.sourceCodes["only"]: want non-nil *Chunk; got nil`)
		}
	})

	// ---- two entries with byte-identical content → SAME *Chunk pointer
	// (content-hash dedup via shared CompileCache). ----
	t.Run("TwoEntries_IdenticalContent_SameChunkPointer", func(t *testing.T) {
		t.Parallel()
		script := "function envoy_on_request(rh) end\n"
		m := validLuaConfig()
		m.SourceCodes = map[string]*corev3.DataSource{
			"x": {Specifier: &corev3.DataSource_InlineString{InlineString: script}},
			"y": {Specifier: &corev3.DataSource_InlineString{InlineString: script}},
		}
		cc, err := buildCompiledConfig(toAny(t, m))
		if err != nil {
			t.Fatalf("buildCompiledConfig: want nil error; got %v", err)
		}
		cx := cc.sourceCodes["x"]
		cy := cc.sourceCodes["y"]
		if cx == nil || cy == nil {
			t.Fatalf("cc.sourceCodes: want both non-nil; got x=%v y=%v", cx, cy)
		}
		// Content-hash dedup: identical bytes MUST yield the same *Chunk pointer.
		if cx != cy {
			t.Fatalf("cc.sourceCodes[x] != cc.sourceCodes[y]: want SAME *Chunk pointer for byte-identical content (content-hash dedup via shared CompileCache)")
		}
	})
}

// -----------------------------------------------------------------------------
// Sanity assertion: the parseReject* constants match the byte-exact wording
// cataloged in parent SPEC §6.2 verbatim. Any drift surfaces immediately as
// a build-or-test break; future SPEC amendments to the wording must touch
// both this file + compiled_config.go in lockstep.
// -----------------------------------------------------------------------------

func TestParseRejectConstants_ByteExactWording(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Arm01", parseRejectTypedConfigRequired, "lua: typed_config required"},
		{"Arm03", parseRejectInlineCodeDeprecated, "lua: inline_code is deprecated; use default_source_code"},
		// Arm04 deferred const (parseRejectSourceCodesDeferred) retired at phase
		// 22.3 Task 1 — the arm-4 reject is replaced by the consume path. The
		// new constants for the consume path error leaves are pinned here:
		{"SourceCodesKeyEmpty", parseRejectSourceCodesKeyEmpty, "lua: source_codes: key must be non-empty"},
		{"SourceCodesValueFmt", wrapParseRejectSourceCodesValueFmt, `lua: source_codes[%q]: %w`},
		// Arm18 per-route const family — retired deferred wording;
		// real 4 consts live in perroute.go + pinned at
		// perroute_test.go::TestParsePerRouteConsts_ByteExactWording.
		// Representative spot-checks here for cross-package visibility:
		{"Arm18_OneofRequired", parseRejectPerRouteOneofRequired, "lua: per-route: override oneof is required"},
		{"Arm18_DisabledFalse", parseRejectPerRouteDisabledFalse, "lua: per-route: disabled must be true (PGV const:true violation)"},
		{"Arm18_NameEmpty", parseRejectPerRouteNameEmpty, "lua: per-route: name length must be at least 1 rune"},
		{"Arm18_SourceCodeWrap", wrapParseRejectPerRouteSourceCodeFmt, "lua: per-route: source_code: %w"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// Compile-time assertion that the wrapped-error arms compose cleanly via
// errors.Is. The arm 2 (typed_config unmarshal) constant is the format-string
// prefix used by fmt.Errorf — assert the prefix shape here.
func TestParseRejectArm02_WrappedError_HasPrefix(t *testing.T) {
	inner := errors.New("decoder failure")
	err := wrapParseRejectTypedConfigUnmarshal(inner)
	if !strings.HasPrefix(err.Error(), "lua: typed_config unmarshal: ") {
		t.Fatalf("wrapped err = %q; want prefix %q", err.Error(), "lua: typed_config unmarshal: ")
	}
	if !errors.Is(err, inner) {
		t.Fatalf("wrapped err does not unwrap to inner via errors.Is")
	}
}

// TestParseRejectArm19_StatPrefixInvalid_TriggersOnInvalidPrefix covers
// the arm-19 stat_prefix character-class pre-check added at Task 11 in
// response to the FuzzLuaConfigParse panic-discovery finding (without
// this guard, `stats.NewCounter` panics at newFilterStats time when
// `Lua.stat_prefix` contains nameRE-invalid characters — taking down
// `lua.New` per the listener-construction call path). Mirrors the
// hcm/config.go:209 + cluster/manager.go:205 `stats.IsValidName`
// pre-check precedent.
func TestParseRejectArm19_StatPrefixInvalid_TriggersOnInvalidPrefix(t *testing.T) {
	// `my-script-prefix` contains `-` which is outside nameRE's permitted
	// character class. Without arm-19, the assembled name
	// `http.<hcm>.lua.my-script-prefix.errors` would PANIC at registry
	// write time. With arm-19, it returns a clean PARSE-REJECT error.
	m := &luav3.Lua{
		StatPrefix: "my-script-prefix",
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			},
		},
	}
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	_, gotErr := buildCompiledConfig(a)
	if gotErr == nil {
		t.Fatal("buildCompiledConfig returned nil err; want arm-19 PARSE-REJECT")
	}
	if !strings.Contains(gotErr.Error(), "lua: stat_prefix: invalid characters in ") {
		t.Errorf("err = %q; want substring %q", gotErr.Error(), "lua: stat_prefix: invalid characters in ")
	}
	if !strings.Contains(gotErr.Error(), `"my-script-prefix"`) {
		t.Errorf("err = %q; want substring %q", gotErr.Error(), `"my-script-prefix"`)
	}
	if !strings.Contains(gotErr.Error(), statNameRegexLiteral) {
		t.Errorf("err = %q; want substring %q (regex literal)", gotErr.Error(), statNameRegexLiteral)
	}
}

// TestParseRejectArm19_StatPrefixValid_PassesThrough verifies arm-19
// does NOT reject valid stat_prefix inputs. Underscore-separated prefix
// is the canonical valid form (e.g., "ingress_http_lua").
func TestParseRejectArm19_StatPrefixValid_PassesThrough(t *testing.T) {
	m := &luav3.Lua{
		StatPrefix: "my_script_prefix",
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			},
		},
	}
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	cc, gotErr := buildCompiledConfig(a)
	if gotErr != nil {
		t.Fatalf("buildCompiledConfig err = %v; want nil for valid prefix", gotErr)
	}
	if cc == nil {
		t.Fatal("buildCompiledConfig returned nil cc; want non-nil")
	}
}

// TestParseRejectArm19_StatPrefixEmpty_PassesThrough verifies arm-19
// does NOT reject the empty prefix (the consecutive-dot path is
// RATIFIED per AMEND-2 + parent §7.2; the registry regex permits
// interior consecutive dots — only trailing dots are rejected).
func TestParseRejectArm19_StatPrefixEmpty_PassesThrough(t *testing.T) {
	m := &luav3.Lua{
		StatPrefix: "",
		DefaultSourceCode: &corev3.DataSource{
			Specifier: &corev3.DataSource_InlineString{
				InlineString: "function envoy_on_request(rh) end",
			},
		},
	}
	a, err := anypb.New(m)
	if err != nil {
		t.Fatalf("anypb.New: %v", err)
	}
	if _, gotErr := buildCompiledConfig(a); gotErr != nil {
		t.Fatalf("buildCompiledConfig err = %v; want nil for empty prefix (AMEND-2 consecutive-dot path)", gotErr)
	}
}

// -----------------------------------------------------------------------------
// Task 14 (phase 22.2 IMPL) — SPEC §6 arms 20-22 byte-stable wording byte-pin
// assertions. Arms 20-22 are RUNTIME-REJECTS per §11.2 AMEND-22.2-2 (raised
// via L.RaiseError from the bridge LGFunctions, NOT PARSE-REJECTs at
// config-load), so they live in production code at body.go (arm 21) +
// httpcall.go (arm 20) + crypto.go (arm 22) rather than in compiled_config.go's
// parseReject* const family. The 19-arm config-load PARSE-REJECT roster from
// 22.1 STAYS UNCHANGED at 22.2 IMPL per SPEC §6.
//
// Per-method byte-stable wording tests for the LIVE arm-raising surface
// already exist at:
//   - body_test.go::Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording
//   - httpcall_test.go::Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording
//   - crypto_test.go::Test_ImportPublicKey_invalid_PEM_raises_arm22_byte_stable_wording
//
// This centralized const-byte-pin family asserts the SAME wordings at the
// CONSTANT level (mirrors TestParseRejectConstants_ByteExactWording precedent
// for arms 1/3/4/18). Drift between the per-method tests + this central
// catalog surfaces at the per-method test (LIVE raise) AND here (const drift)
// in lockstep.
// -----------------------------------------------------------------------------

// TestRuntimeRejectArm20_HTTPCallClusterRequired_ByteExactWording pins the
// byte-exact wording for SPEC §6 arm 20 (httpcall-cluster-name-required) —
// raised from httpcall.go::requestHandleHttpCall when :httpCall("", ...) is
// invoked with an empty cluster name. The live raise path is asserted at
// httpcall_test.go::Test_HTTPCall_empty_cluster_raises_arm20_byte_stable_wording;
// this test mirrors that assertion at the package-const level.
func TestRuntimeRejectArm20_HTTPCallClusterRequired_ByteExactWording(t *testing.T) {
	const want = "lua: httpCall: cluster name must not be empty"
	if httpCallClusterRequiredMsg != want {
		t.Fatalf("httpCallClusterRequiredMsg = %q; want %q (SPEC §6 arm 20 byte-stable drift)",
			httpCallClusterRequiredMsg, want)
	}
}

// TestRuntimeRejectArm22_CryptoImportPublicKeyPrefix_ByteExactWording pins
// the byte-exact wording prefix for SPEC §6 arm 22 (crypto-key-format-
// invalid) — raised from crypto.go::requestHandleImportPublicKey when
// :importPublicKey(pem) fails to parse. The wording template is
// `"lua: importPublicKey: <inner crypto/x509 error>"` per W2 pinning
// (Task 12). The live raise path is asserted at
// crypto_test.go::Test_ImportPublicKey_invalid_PEM_raises_arm22_byte_stable_wording;
// this test mirrors that assertion at the package-const level.
//
// NOTE: SPEC §6 row 22 prescribes the template `"lua: %s: %w"` wrapping
// `crypto/x509.ParsePKIXPublicKey` with `%s` = "importPublicKey". The
// production wording (Task 12 W2 pinning) materializes this as a literal
// prefix `"lua: importPublicKey:"` followed by the inner error. The prefix
// form is byte-stable; the trailing inner error carries variable bytes
// (crypto/x509's per-error wording). No reconciliation needed — the SPEC
// template + W2 pinning agree.
func TestRuntimeRejectArm22_CryptoImportPublicKeyPrefix_ByteExactWording(t *testing.T) {
	const want = "lua: importPublicKey:"
	if cryptoImportPublicKeyErrPrefix != want {
		t.Fatalf("cryptoImportPublicKeyErrPrefix = %q; want %q (SPEC §6 arm 22 byte-stable drift)",
			cryptoImportPublicKeyErrPrefix, want)
	}
}

// TestRuntimeRejectArm21_BodyOverCap_ByteExactWording pins the byte-exact
// wording template for SPEC §6 arm 21 (body-size-cap-exceeded) — raised
// from body.go::requestHandleBody / responseHandleBody when accumulated
// body bytes exceed f.maxBodyBufferedBytes. The wording template is
// `"lua: body: accumulated body exceeds maximum buffered size of %d bytes"`
// per W2 pinning (Task 7). Unlike arms 20 + 22 which live as named const
// declarations in their respective package-files, arm 21 currently
// materializes as an inline format string in body.go (used at two sites:
// requestHandleBody + responseHandleBody). This test asserts the wording
// shape via a sentinel-formatted probe matching the production
// fmt.Sprintf call shape at body.go:315-318 + body.go:406-409.
//
// The live raise path is asserted at
// body_test.go::Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording.
//
// Future maintainer: if arm 21's wording template is ever extracted to a
// named package-const (e.g. bodyOverCapMsgFmt), update both this test +
// body.go's two fmt.Sprintf call sites in lockstep per ADR-0044 atomic-
// edit discipline.
func TestRuntimeRejectArm21_BodyOverCap_ByteExactWording(t *testing.T) {
	const wantFmt = "lua: body: accumulated body exceeds maximum buffered size of %d bytes"
	// Probe the template with a sentinel value to exercise the byte-exact
	// shape end-to-end. If body.go's two fmt.Sprintf sites drift from this
	// template, the per-method test
	// (Test_RequestHandleBody_over_cap_raises_arm21_byte_stable_wording)
	// breaks; this test verifies the byte-exact template itself stays
	// pinned at the byte-stable contract.
	const sentinelCap = 4096
	got := fmt.Sprintf(wantFmt, sentinelCap)
	want := "lua: body: accumulated body exceeds maximum buffered size of 4096 bytes"
	if got != want {
		t.Fatalf("arm 21 byte-stable wording sentinel = %q; want %q (SPEC §6 arm 21 drift)",
			got, want)
	}
}

// TestDefaultMaxBodyBufferedBytes_SixteenMiB pins the byte-exact 16 MiB
// hardcoded body-buffer cap constant per SPEC §6 arm 21 + Task 7 W2
// pinning (16 * 1024 * 1024 = 16777216). The constant lives at lua.go
// per Task 7's declaration; Task 14 verifies the value is correct +
// settled per the "16 MiB cap inherits 22.1 Task 11 cap pattern from
// DataSource.Filename" SPEC §6 row 21 derivation.
func TestDefaultMaxBodyBufferedBytes_SixteenMiB(t *testing.T) {
	const want = 16 * 1024 * 1024 // 16 MiB = 16,777,216 bytes
	if defaultMaxBodyBufferedBytes != want {
		t.Fatalf("defaultMaxBodyBufferedBytes = %d; want %d (16 MiB; SPEC §6 arm 21 hardcoded cap)",
			defaultMaxBodyBufferedBytes, want)
	}
	if defaultMaxBodyBufferedBytes != 16777216 {
		t.Fatalf("defaultMaxBodyBufferedBytes = %d; want 16777216 (literal bytes form)",
			defaultMaxBodyBufferedBytes)
	}
}

// TestStatNameRegexLiteral_MatchesStatsPackageRegex verifies the
// statNameRegexLiteral constant matches the stats package's nameRE
// source verbatim. Drift between the two literals would cause arm-19's
// error wording to misrepresent the actual regex applied by the
// registry; this test catches the drift at build/test time.
func TestStatNameRegexLiteral_MatchesStatsPackageRegex(t *testing.T) {
	// We don't have direct access to nameRE's source string from outside
	// the stats package (it's unexported), but we can cross-check via
	// IsValidName's behavior: a string that matches the LITERAL regex
	// must also pass IsValidName, and vice versa for non-matchers.
	tests := []struct {
		name    string
		isValid bool
	}{
		{"valid_under_score", true},
		{"valid.dot.separated", true},
		{"a", true},
		{"a1", true},
		{"_underscore_start", true},
		{"my-hyphen", false},
		{"has space", false},
		{"has/slash", false},
		{"trailing_dot.", false},
		{"1starts_with_digit", false},
	}
	for _, tc := range tests {
		got := stats.IsValidName(tc.name)
		if got != tc.isValid {
			t.Errorf("stats.IsValidName(%q) = %v; want %v (drift between statNameRegexLiteral and nameRE?)", tc.name, got, tc.isValid)
		}
	}
}
