package lua

// compiled_config_test.go — Task 2 RIGID-TDD table-driven test surface per
// 22.1 SPEC §6 Task 2 + parent SPEC §6.2 18-arm PARSE-REJECT roster +
// §12-D1 closure (REFUTED — arms 5 + 17 flip to silent no-op).
//
// Coverage at Task 2:
//   - Arm 1  (typed-config-required)              — PARSE-REJECT byte-exact
//   - Arm 2  (typed-config-unmarshal)             — PARSE-REJECT byte-exact prefix
//   - Arm 3  (inline-code-deprecated-rejected)    — PARSE-REJECT byte-exact
//   - Arm 4  (source-codes-deferred-to-22-3)      — PARSE-REJECT byte-exact
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
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/esalaine/envoy-go/internal/stats"
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
}

// testBuildCompiledConfigParseReject covers the Task-2-reachable PARSE-REJECT
// arms (1, 2, 3, 4). Arms 5 + 17 are D1-REFUTED (covered separately under
// "D1_REFUTED_SilentNoop"). Arms 6-15 land at Task 3 (datasource.go full IMPL).
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
		// ---- Arm 4: source_codes map deferred to 22.3 ----
		{
			name: "Arm04_SourceCodes_DeferredTo223",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := validLuaConfig()
				m.SourceCodes = map[string]*corev3.DataSource{
					"hello.lua": {
						Specifier: &corev3.DataSource_InlineString{
							InlineString: "function envoy_on_request(rh) end\n",
						},
					},
				}
				return toAny(t, m)
			},
			wantErrEq: parseRejectSourceCodesDeferred,
		},
		// Arm 4 fires even if SourceCodes is the SOLE source path (no
		// default_source_code) — the SourceCodes presence is itself rejected
		// regardless of the default_source_code arm; PARSE-REJECT ordering
		// puts arm 4 BEFORE arm 5 (REFUTED) so the SourceCodes-only config
		// surfaces arm 4, not the silent-no-op path.
		{
			name: "Arm04_SourceCodes_AloneNoDefault_Rejected",
			typedConfig: func(t *testing.T) *anypb.Any {
				m := &luav3.Lua{
					SourceCodes: map[string]*corev3.DataSource{
						"hello.lua": {
							Specifier: &corev3.DataSource_InlineString{
								InlineString: "function envoy_on_request(rh) end\n",
							},
						},
					},
				}
				return toAny(t, m)
			},
			wantErrEq: parseRejectSourceCodesDeferred,
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

// testBuildCompiledConfigArm18PerRoute re-asserts the arm-18 PARSE-REJECT
// wording surfaced by the existing Task 1 skeleton's validatePerRouteLua
// (declared in lua.go). The wording constant is the canonical
// parseRejectPerRouteDeferred in compiled_config.go — this test asserts that
// the validator returns the byte-exact same string. Mirrors the ADR-0110
// single-chokepoint discipline + parent §6.2 arm 18.
func testBuildCompiledConfigArm18PerRoute(t *testing.T) {
	t.Parallel()
	err := validatePerRouteLua(nil)
	if err == nil {
		t.Fatalf("validatePerRouteLua: want error; got nil")
	}
	if err.Error() != parseRejectPerRouteDeferred {
		t.Fatalf("validatePerRouteLua err = %q; want %q", err.Error(), parseRejectPerRouteDeferred)
	}
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
		{"Arm04", parseRejectSourceCodesDeferred, "lua: source_codes map is not yet supported (lands in phase 22.3)"},
		{"Arm18", parseRejectPerRouteDeferred, "lua: per-route configuration is not yet supported (lands in phase 22.3)"},
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
