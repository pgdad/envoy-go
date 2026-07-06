package lua

// perroute_test.go — table-driven TDD tests for parsePerRouteLua per phase
// 22.3 Task 2 IMPL + ADR-0110 single-chokepoint + parent SPEC §6.2 arm 18.
//
// Strict TDD: tests written BEFORE the implementation. Run first to confirm
// FAIL ("parsePerRouteLua undefined"), then implement to reach PASS.
//
// Coverage (per PLAN Task 2 Step 1):
//   - disabled: true  → no error
//   - disabled: false → byte-exact wording
//   - name: "a"       → no error
//   - name: ""        → byte-exact wording
//   - source_code valid InlineString → no error (compile-to-validate)
//   - source_code ENOENT Filename   → error with prefix "lua: per-route: source_code: "
//   - source_code compile-error     → error with prefix "lua: per-route: source_code: "
//   - source_code WatchedDirectory  → reject (gauntlet reuse)
//   - nil-oneof (&LuaPerRoute{})    → byte-exact "lua: per-route: override oneof is required"
//   - wrong type (non-*LuaPerRoute) → type-assert error

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	luav3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/lua/v3"
	"google.golang.org/protobuf/proto"

	internallua "github.com/pgdad/envoy-go/internal/lua"
)

// TestParsePerRouteLua is the table-driven TDD test for parsePerRouteLua.
// Each row exercises one validation arm per PLAN Task 2 Step 1.
func TestParsePerRouteLua(t *testing.T) {
	cases := []struct {
		name             string
		input            proto.Message
		wantErrEq        string // byte-exact error match
		wantErrHasPrefix string // prefix match (variable-tail errors)
		wantNoErr        bool   // expect nil error
	}{
		// ---- disabled arm: true → ok ----
		{
			name: "Disabled_True_Ok",
			input: &luav3.LuaPerRoute{
				Override: &luav3.LuaPerRoute_Disabled{Disabled: true},
			},
			wantNoErr: true,
		},
		// ---- disabled arm: false → byte-exact reject ----
		{
			name: "Disabled_False_Rejected",
			input: &luav3.LuaPerRoute{
				Override: &luav3.LuaPerRoute_Disabled{Disabled: false},
			},
			wantErrEq: parseRejectPerRouteDisabledFalse,
		},
		// ---- name arm: non-empty → ok ----
		{
			name: "Name_NonEmpty_Ok",
			input: &luav3.LuaPerRoute{
				Override: &luav3.LuaPerRoute_Name{Name: "a"},
			},
			wantNoErr: true,
		},
		// ---- name arm: empty → byte-exact reject ----
		{
			name: "Name_Empty_Rejected",
			input: &luav3.LuaPerRoute{
				Override: &luav3.LuaPerRoute_Name{Name: ""},
			},
			wantErrEq: parseRejectPerRouteNameEmpty,
		},
		// ---- source_code arm: valid InlineString → no error (compile-to-validate) ----
		{
			name: "SourceCode_ValidInlineString_Ok",
			input: &luav3.LuaPerRoute{
				Override: &luav3.LuaPerRoute_SourceCode{
					SourceCode: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineString{
							InlineString: "function envoy_on_request(rh) end\n",
						},
					},
				},
			},
			wantNoErr: true,
		},
		// ---- source_code arm: ENOENT Filename → prefix-wrapped error ----
		{
			name: "SourceCode_FilenameEnoent_WrappedError",
			input: &luav3.LuaPerRoute{
				Override: &luav3.LuaPerRoute_SourceCode{
					SourceCode: &corev3.DataSource{
						Specifier: &corev3.DataSource_Filename{
							Filename: "/nonexistent/path/xyzzy_per_route_22_3.lua",
						},
					},
				},
			},
			wantErrHasPrefix: "lua: per-route: source_code: ",
		},
		// ---- source_code arm: compile error (malformed Lua) → prefix-wrapped error ----
		{
			name: "SourceCode_CompileError_WrappedError",
			input: &luav3.LuaPerRoute{
				Override: &luav3.LuaPerRoute_SourceCode{
					SourceCode: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineString{
							// Deliberately malformed Lua: unclosed function
							InlineString: "function not_valid ((((",
						},
					},
				},
			},
			wantErrHasPrefix: "lua: per-route: source_code: ",
		},
		// ---- source_code arm: WatchedDirectory → gauntlet rejects ----
		{
			name: "SourceCode_WatchedDirectory_Rejected",
			input: &luav3.LuaPerRoute{
				Override: &luav3.LuaPerRoute_SourceCode{
					SourceCode: &corev3.DataSource{
						Specifier: &corev3.DataSource_InlineString{
							InlineString: "function envoy_on_request(rh) end\n",
						},
						WatchedDirectory: &corev3.WatchedDirectory{Path: "/some/dir"},
					},
				},
			},
			wantErrHasPrefix: "lua: per-route: source_code: ",
		},
		// ---- nil-oneof (&LuaPerRoute{}) → byte-exact "override oneof is required" ----
		{
			name:      "NilOneof_Required",
			input:     &luav3.LuaPerRoute{},
			wantErrEq: parseRejectPerRouteOneofRequired,
		},
		// ---- wrong type (non-*LuaPerRoute proto.Message) → type-assert error ----
		{
			name:             "WrongType_TypeAssertError",
			input:            &luav3.Lua{}, // not a *LuaPerRoute
			wantErrHasPrefix: "lua: per-route: expected *luav3.LuaPerRoute, got ",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parsePerRouteLua(tc.input)
			switch {
			case tc.wantNoErr:
				if err != nil {
					t.Fatalf("parsePerRouteLua: want nil error; got %v", err)
				}
			case tc.wantErrEq != "":
				if err == nil {
					t.Fatalf("parsePerRouteLua: want error %q; got nil", tc.wantErrEq)
				}
				if err.Error() != tc.wantErrEq {
					t.Fatalf("parsePerRouteLua err = %q; want %q", err.Error(), tc.wantErrEq)
				}
			case tc.wantErrHasPrefix != "":
				if err == nil {
					t.Fatalf("parsePerRouteLua: want error with prefix %q; got nil", tc.wantErrHasPrefix)
				}
				if !strings.HasPrefix(err.Error(), tc.wantErrHasPrefix) {
					t.Fatalf("parsePerRouteLua err = %q; want prefix %q", err.Error(), tc.wantErrHasPrefix)
				}
			default:
				t.Fatalf("test case %q has neither wantNoErr, wantErrEq, nor wantErrHasPrefix", tc.name)
			}
		})
	}
}

// TestParsePerRouteLua_ReturnValue verifies that parsePerRouteLua returns the
// typed *LuaPerRoute on the happy path and nil on error paths.
func TestParsePerRouteLua_ReturnValue(t *testing.T) {
	t.Run("HappyPath_ReturnsProto", func(t *testing.T) {
		t.Parallel()
		pr := &luav3.LuaPerRoute{
			Override: &luav3.LuaPerRoute_Name{Name: "myscript"},
		}
		got, err := parsePerRouteLua(pr)
		if err != nil {
			t.Fatalf("parsePerRouteLua: want nil error; got %v", err)
		}
		if got == nil {
			t.Fatal("parsePerRouteLua: want non-nil *LuaPerRoute; got nil")
		}
		if got != pr {
			t.Fatal("parsePerRouteLua: want returned *LuaPerRoute to be the same pointer as input")
		}
	})

	t.Run("ErrorPath_ReturnsNil", func(t *testing.T) {
		t.Parallel()
		got, err := parsePerRouteLua(&luav3.LuaPerRoute{})
		if err == nil {
			t.Fatal("parsePerRouteLua: want error; got nil")
		}
		if got != nil {
			t.Fatalf("parsePerRouteLua: want nil *LuaPerRoute on error; got %v", got)
		}
	})
}

// TestParsePerRouteConsts_ByteExactWording pins the 4 byte-exact wording
// constants for per-route validation. Mirrors TestParseRejectConstants_
// ByteExactWording in compiled_config_test.go. Any drift surfaces as a
// test break per ADR-0044 atomic-edit discipline + ADR-0080 byte-stable
// wording contract.
func TestParsePerRouteConsts_ByteExactWording(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{
			"OneofRequired",
			parseRejectPerRouteOneofRequired,
			"lua: per-route: override oneof is required",
		},
		{
			"DisabledFalse",
			parseRejectPerRouteDisabledFalse,
			"lua: per-route: disabled must be true (PGV const:true violation)",
		},
		{
			"NameEmpty",
			parseRejectPerRouteNameEmpty,
			"lua: per-route: name length must be at least 1 rune",
		},
		{
			"SourceCodeWrapFmt",
			wrapParseRejectPerRouteSourceCodeFmt,
			"lua: per-route: source_code: %w",
		},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}
}

// -----------------------------------------------------------------------------
// Task 3 — resolveDecodeScript 3-tier dispatch tests.
// -----------------------------------------------------------------------------

// perRouteDCB is a configurable test-double DecoderFilterCallbacks for the
// resolveDecodeScript dispatch tests. It embeds the package-shared recordedDCB
// (which supplies every interface method as a zero-value stub) and overrides
// RequestRouteConfig to return a canned proto.Message + count invocations.
type perRouteDCB struct {
	*recordedDCB
	routeCfg  proto.Message // returned by RequestRouteConfig; nil → no per-route
	callCount int
}

func (c *perRouteDCB) RequestRouteConfig() proto.Message {
	c.callCount++
	return c.routeCfg
}

// newResolveTestFilter builds a *filter with the supplied compiledConfig +
// the supplied per-route config wired through a perRouteDCB. A nil routeCfg
// models the no-per-route (listener-default) path.
func newResolveTestFilter(cc *compiledConfig, routeCfg proto.Message) (*filter, *perRouteDCB) {
	dcb := &perRouteDCB{recordedDCB: &recordedDCB{}, routeCfg: routeCfg}
	f := &filter{cc: cc}
	f.SetDecoderCallbacks(dcb)
	return f, dcb
}

// mustCompile compiles src into a *Chunk via the supplied cache (nil → uncached).
func mustCompile(t *testing.T, src string, cache *internallua.CompileCache) *internallua.Chunk {
	t.Helper()
	ch, err := internallua.CompileScript([]byte(src), cache)
	if err != nil {
		t.Fatalf("CompileScript(%q) err = %v", src, err)
	}
	return ch
}

// TestResolveDecodeScript exercises the 3-tier per-route dispatch.
func TestResolveDecodeScript_NoPerRoute_ListenerDefault(t *testing.T) {
	defChunk := mustCompile(t, "function envoy_on_request(rh) end\n", nil)
	cc := &compiledConfig{chunk: defChunk, compileCache: internallua.NewCompileCache()}
	f, _ := newResolveTestFilter(cc, nil)

	got, disabled := f.resolveDecodeScript()
	if disabled {
		t.Fatalf("disabled = true; want false (no per-route → listener default)")
	}
	if got != defChunk {
		t.Fatalf("chunk = %p; want listener default %p", got, defChunk)
	}
}

func TestResolveDecodeScript_NoPerRoute_NilListenerChunk(t *testing.T) {
	cc := &compiledConfig{chunk: nil, compileCache: internallua.NewCompileCache()}
	f, _ := newResolveTestFilter(cc, nil)

	got, disabled := f.resolveDecodeScript()
	if disabled {
		t.Fatalf("disabled = true; want false")
	}
	if got != nil {
		t.Fatalf("chunk = %p; want nil (no per-route + nil listener chunk)", got)
	}
}

func TestResolveDecodeScript_NilDCB_ListenerDefault(t *testing.T) {
	defChunk := mustCompile(t, "function envoy_on_request(rh) end\n", nil)
	cc := &compiledConfig{chunk: defChunk, compileCache: internallua.NewCompileCache()}
	f := &filter{cc: cc} // no SetDecoderCallbacks → f.dcb == nil

	got, disabled := f.resolveDecodeScript()
	if disabled {
		t.Fatalf("disabled = true; want false (nil dcb → listener default)")
	}
	if got != defChunk {
		t.Fatalf("chunk = %p; want listener default %p", got, defChunk)
	}
}

func TestResolveDecodeScript_NamedHit_RegistryChunk(t *testing.T) {
	cache := internallua.NewCompileCache()
	namedA := mustCompile(t, "function envoy_on_request(rh) end\n", cache)
	defChunk := mustCompile(t, "function envoy_on_response(rh) end\n", cache)
	cc := &compiledConfig{
		chunk:        defChunk,
		sourceCodes:  map[string]*internallua.Chunk{"named_a": namedA},
		compileCache: cache,
	}
	pr := &luav3.LuaPerRoute{Override: &luav3.LuaPerRoute_Name{Name: "named_a"}}
	f, _ := newResolveTestFilter(cc, pr)

	got, disabled := f.resolveDecodeScript()
	if disabled {
		t.Fatalf("disabled = true; want false (name arm)")
	}
	if got != namedA {
		t.Fatalf("chunk = %p; want registry chunk %p", got, namedA)
	}
}

func TestResolveDecodeScript_DanglingName_SilentNoOp(t *testing.T) {
	// AMEND-22.3-1: a name absent from the registry returns (nil, false) —
	// silent no-op (NOT an error), so decode early-returns Continue.
	defChunk := mustCompile(t, "function envoy_on_request(rh) end\n", nil)
	cc := &compiledConfig{
		chunk:        defChunk,
		sourceCodes:  map[string]*internallua.Chunk{"named_a": defChunk},
		compileCache: internallua.NewCompileCache(),
	}
	pr := &luav3.LuaPerRoute{Override: &luav3.LuaPerRoute_Name{Name: "ghost"}}
	f, _ := newResolveTestFilter(cc, pr)

	got, disabled := f.resolveDecodeScript()
	if disabled {
		t.Fatalf("disabled = true; want false (dangling name → silent no-op)")
	}
	if got != nil {
		t.Fatalf("chunk = %p; want nil (registry miss → silent no-op)", got)
	}
}

func TestResolveDecodeScript_Disabled_ReturnsDisabled(t *testing.T) {
	defChunk := mustCompile(t, "function envoy_on_request(rh) end\n", nil)
	cc := &compiledConfig{chunk: defChunk, compileCache: internallua.NewCompileCache()}
	pr := &luav3.LuaPerRoute{Override: &luav3.LuaPerRoute_Disabled{Disabled: true}}
	f, _ := newResolveTestFilter(cc, pr)

	got, disabled := f.resolveDecodeScript()
	if !disabled {
		t.Fatalf("disabled = false; want true (disabled arm)")
	}
	if got != nil {
		t.Fatalf("chunk = %p; want nil (disabled arm)", got)
	}
}

func TestResolveDecodeScript_SourceCodeOverride_CompiledChunk(t *testing.T) {
	cc := &compiledConfig{chunk: nil, compileCache: internallua.NewCompileCache()}
	pr := &luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{
					InlineString: "function envoy_on_response(rh) end\n",
				},
			},
		},
	}
	f, _ := newResolveTestFilter(cc, pr)

	got, disabled := f.resolveDecodeScript()
	if disabled {
		t.Fatalf("disabled = true; want false (source_code arm)")
	}
	if got == nil {
		t.Fatalf("chunk = nil; want compiled override chunk")
	}
}

func TestResolveDecodeScript_SourceCodeOverride_MemoHit_SamePointer(t *testing.T) {
	// Second resolve on the SAME *LuaPerRoute pointer must return the SAME
	// *Chunk pointer (memo hit, keyed by the per-route proto pointer).
	cc := &compiledConfig{chunk: nil, compileCache: internallua.NewCompileCache()}
	pr := &luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineString{
					InlineString: "function envoy_on_response(rh) end\n",
				},
			},
		},
	}
	f, _ := newResolveTestFilter(cc, pr)

	first, _ := f.resolveDecodeScript()
	second, _ := f.resolveDecodeScript()
	if first == nil || second == nil {
		t.Fatalf("got nil chunk(s): first=%p second=%p", first, second)
	}
	if first != second {
		t.Fatalf("memo miss: first=%p second=%p; want same *Chunk pointer", first, second)
	}
}

// TestResolveDecodeScript_SourceCodeOverride_MemoHit_FilenameNotReread pins the
// D-P1(b') no-re-read guarantee at the filesystem level. The memo exists
// precisely so a Filename DataSource is read+compiled ONCE per *LuaPerRoute and
// never re-read on subsequent streams.
//
// Approach (b): resolveDataSource has no injectable filesystem hook (it reads
// via os.ReadFile), so we prove the no-re-read property directly. We write a
// temp file, resolve the source_code override ONCE (populating the memo), then
// DELETE the file, then resolve a SECOND time on the SAME *LuaPerRoute pointer.
// If the second resolve returned the same *Chunk, it could NOT have re-read the
// now-absent file (a re-read would hit ENOENT → nil) — it was a genuine memo
// hit. With an InlineString arm a CompileCache content-hash hit could mask a
// re-read; the deleted Filename makes a re-read observable as a nil chunk.
func TestResolveDecodeScript_SourceCodeOverride_MemoHit_FilenameNotReread(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "per_route_22_3.lua")
	if err := os.WriteFile(scriptPath, []byte("function envoy_on_response(rh) end\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q) err = %v", scriptPath, err)
	}

	cc := &compiledConfig{chunk: nil, compileCache: internallua.NewCompileCache()}
	pr := &luav3.LuaPerRoute{
		Override: &luav3.LuaPerRoute_SourceCode{
			SourceCode: &corev3.DataSource{
				Specifier: &corev3.DataSource_Filename{
					Filename: scriptPath,
				},
			},
		},
	}
	f, _ := newResolveTestFilter(cc, pr)

	// First resolve: reads + compiles the file, populating the memo.
	first, disabled := f.resolveDecodeScript()
	if disabled {
		t.Fatalf("disabled = true; want false (source_code arm)")
	}
	if first == nil {
		t.Fatalf("first chunk = nil; want compiled override chunk")
	}

	// Delete the file BETWEEN resolves. A second resolve that re-read the
	// DataSource would now hit ENOENT and return nil (silent no-op).
	if err := os.Remove(scriptPath); err != nil {
		t.Fatalf("Remove(%q) err = %v", scriptPath, err)
	}
	if _, statErr := os.Stat(scriptPath); !os.IsNotExist(statErr) {
		t.Fatalf("temp file still present after Remove; cannot prove no-re-read")
	}

	// Second resolve on the SAME pointer: must be a memo hit (no filesystem
	// read), returning the SAME *Chunk despite the file now being absent.
	second, _ := f.resolveDecodeScript()
	if second == nil {
		t.Fatalf("second chunk = nil after file delete: the memo re-read the " +
			"now-absent file instead of serving the cached chunk (D-P1(b') re-read bug)")
	}
	if first != second {
		t.Fatalf("memo miss: first=%p second=%p; want same *Chunk pointer "+
			"(second resolve must NOT re-read the file)", first, second)
	}
}
