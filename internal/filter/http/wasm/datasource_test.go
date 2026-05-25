package wasm

// datasource_test.go — Task 10 RIGID-TDD test surface per 25.1 PLAN Task 10 +
// parent SPEC §5.4 (AsyncDataSource.Local 4-arm resolution).
//
// # Test surface coverage
//
//   - TestResolveDataSource_Filename* — happy + empty + not-found + empty-content
//     + unreadable + isdir (6 rows; the unreadable + isdir rows surface as
//     wrapped EACCES/EISDIR via the parseRejectFilenameUnreadable %w-format).
//   - TestResolveDataSource_InlineBytes{Happy,Empty} — verbatim byte-passthrough
//     + empty-content rejection.
//   - TestResolveDataSource_InlineString{Happy,Empty} — string→bytes cast
//     + empty-content rejection.
//   - TestResolveDataSource_EnvVar{Happy,NameEmpty,Unset,EmptyValue} — t.Setenv-
//     driven success + 3 distinct failure paths.
//   - TestParseRejectDataSourceConstants_ByteStable — 8 per-arm sub-failure
//     consts byte-stable (mirrors Task 9 ByteStable discipline at the sub-arm
//     level per parent §6.1 wording-discipline).
//   - TestBuildCompiledConfig_DataSource_HappyPath_InlineString — end-to-end
//     reaches CompileModule with valid wasm bytecode via the Task 9-deferred
//     arm 16/17 path; verifies arm 17 wraps wazero compile errors (synthetic
//     non-wasm bytes surface as parseRejectModuleCompileFailed-prefixed).
//   - TestBuildCompiledConfig_DataSource_HappyPath_Filename — same but via
//     the Filename arm with a t.TempDir()-written synthetic bytecode file.
//
// # TDD discipline
//
// Tests authored FIRST; tests will FAIL with "undefined: resolveFilename" /
// "undefined: parseRejectFilenameEmpty" etc. at RED phase; Task 10 IMPL
// (datasource.go) makes them GREEN.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	wasmcommonv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/wasm/v3"

	envoyhttp "github.com/esalaine/envoy-go/internal/filter/http"
)

// -----------------------------------------------------------------------------
// TestParseRejectDataSourceConstants_ByteStable — sub-arm wording discipline.
// -----------------------------------------------------------------------------

// TestParseRejectDataSourceConstants_ByteStable pins the byte-exact wording
// for each of the 8 DataSource resolution sub-arm PARSE-REJECT constants
// landed at Task 10. These are SUB-arms of arm 8's broader "data-source-
// specifier-required" family per parent §6.2 — operator-diagnostic fidelity
// is preserved by keeping per-arm wordings distinct.
func TestParseRejectDataSourceConstants_ByteStable(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"FilenameEmpty", parseRejectFilenameEmpty, "wasm: config.vm_config.code.local.filename is empty"},
		{"FilenameNotFound", parseRejectFilenameNotFound, "wasm: config.vm_config.code.local.filename: %s: not found"},
		{"FilenameUnreadable", parseRejectFilenameUnreadable, "wasm: config.vm_config.code.local.filename: %s: read error: %w"},
		{"FilenameEmptyContent", parseRejectFilenameEmptyContent, "wasm: config.vm_config.code.local.filename: %s: file is empty"},
		{"InlineBytesEmpty", parseRejectInlineBytesEmpty, "wasm: config.vm_config.code.local.inline_bytes is empty"},
		{"InlineStringEmpty", parseRejectInlineStringEmpty, "wasm: config.vm_config.code.local.inline_string is empty"},
		{"EnvVarNameEmpty", parseRejectEnvVarNameEmpty, "wasm: config.vm_config.code.local.environment_variable is empty"},
		{"EnvVarUnset", parseRejectEnvVarUnset, "wasm: config.vm_config.code.local.environment_variable: %s: unset"},
		{"EnvVarEmptyValue", parseRejectEnvVarEmptyValue, "wasm: config.vm_config.code.local.environment_variable: %s: value is empty"},
	}

	if len(cases) != 9 {
		t.Fatalf("TestParseRejectDataSourceConstants_ByteStable: expected 9 rows; got %d", len(cases))
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("%s = %q; want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// TestResolveDataSource — table-driven 4-arm coverage + per-arm failure paths.
// -----------------------------------------------------------------------------

// dsLocal is a small helper that wraps a DataSource_Specifier in the
// minimum-required *corev3.DataSource envelope expected by resolveDataSource.
func dsLocal(spec interface{}) *corev3.DataSource {
	switch s := spec.(type) {
	case *corev3.DataSource_Filename:
		return &corev3.DataSource{Specifier: s}
	case *corev3.DataSource_InlineBytes:
		return &corev3.DataSource{Specifier: s}
	case *corev3.DataSource_InlineString:
		return &corev3.DataSource{Specifier: s}
	case *corev3.DataSource_EnvironmentVariable:
		return &corev3.DataSource{Specifier: s}
	}
	return nil
}

// ---- Filename arm: happy + 5 failure paths ----

func TestResolveDataSource_FilenameHappy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "code.wasm")
	want := []byte("synthetic-wasm-bytes-stub-not-real-wasm")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}

	got, err := resolveDataSource(dsLocal(&corev3.DataSource_Filename{Filename: path}))
	if err != nil {
		t.Fatalf("resolveDataSource returned err: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("resolveDataSource bytes = %q; want %q", string(got), string(want))
	}
}

func TestResolveDataSource_FilenameEmpty(t *testing.T) {
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_Filename{Filename: ""}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want parseRejectFilenameEmpty")
	}
	if err.Error() != parseRejectFilenameEmpty {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), parseRejectFilenameEmpty)
	}
}

func TestResolveDataSource_FilenameNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.wasm")
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_Filename{Filename: path}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want not-found rejection")
	}
	wantPrefix := "wasm: config.vm_config.code.local.filename: " + path + ": not found"
	if err.Error() != wantPrefix {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), wantPrefix)
	}
}

func TestResolveDataSource_FilenameEmptyContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.wasm")
	if err := os.WriteFile(path, []byte{}, 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_Filename{Filename: path}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want empty-content rejection")
	}
	want := "wasm: config.vm_config.code.local.filename: " + path + ": file is empty"
	if err.Error() != want {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), want)
	}
}

func TestResolveDataSource_FilenameUnreadable(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; chmod 0000 has no effect on root reads")
	}
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0000 has different semantics on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "unreadable.wasm")
	if err := os.WriteFile(path, []byte("payload"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("os.Chmod 0000: %v", err)
	}
	// Restore mode so t.TempDir() cleanup succeeds.
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, err := resolveDataSource(dsLocal(&corev3.DataSource_Filename{Filename: path}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want unreadable (EACCES) rejection")
	}
	wantPrefix := "wasm: config.vm_config.code.local.filename: " + path + ": read error: "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err.Error() = %q; want prefix %q", err.Error(), wantPrefix)
	}
	// Verify the inner error is permission-denied via errors.Is.
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("err = %v; want errors.Is(err, os.ErrPermission) true", err)
	}
}

func TestResolveDataSource_FilenameIsDir(t *testing.T) {
	dir := t.TempDir() // dir itself is a directory we can pass.
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_Filename{Filename: dir}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want EISDIR rejection")
	}
	// EISDIR surfaces via the unreadable %w-wrap branch (os.ReadFile on a
	// directory returns an *fs.PathError wrapping syscall.EISDIR).
	wantPrefix := "wasm: config.vm_config.code.local.filename: " + dir + ": read error: "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err.Error() = %q; want prefix %q (EISDIR via read error wrap)", err.Error(), wantPrefix)
	}
}

// ---- InlineBytes arm: happy + empty ----

func TestResolveDataSource_InlineBytesHappy(t *testing.T) {
	want := []byte{0x00, 0x61, 0x73, 0x6d} // wasm magic-bytes prefix (not a real module).
	got, err := resolveDataSource(dsLocal(&corev3.DataSource_InlineBytes{InlineBytes: want}))
	if err != nil {
		t.Fatalf("resolveDataSource returned err: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("resolveDataSource bytes = %v; want %v", got, want)
	}
}

func TestResolveDataSource_InlineBytesEmpty(t *testing.T) {
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_InlineBytes{InlineBytes: []byte{}}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want parseRejectInlineBytesEmpty")
	}
	if err.Error() != parseRejectInlineBytesEmpty {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), parseRejectInlineBytesEmpty)
	}
}

func TestResolveDataSource_InlineBytesNilSlice(t *testing.T) {
	// nil-slice path: same len-0 trigger; verifies len() over nil-check.
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_InlineBytes{InlineBytes: nil}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want parseRejectInlineBytesEmpty for nil slice")
	}
	if err.Error() != parseRejectInlineBytesEmpty {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), parseRejectInlineBytesEmpty)
	}
}

// ---- InlineString arm: happy + empty ----

func TestResolveDataSource_InlineStringHappy(t *testing.T) {
	const want = "synthetic-non-wasm-but-non-empty"
	got, err := resolveDataSource(dsLocal(&corev3.DataSource_InlineString{InlineString: want}))
	if err != nil {
		t.Fatalf("resolveDataSource returned err: %v", err)
	}
	if string(got) != want {
		t.Fatalf("resolveDataSource bytes = %q; want %q", string(got), want)
	}
}

func TestResolveDataSource_InlineStringEmpty(t *testing.T) {
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_InlineString{InlineString: ""}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want parseRejectInlineStringEmpty")
	}
	if err.Error() != parseRejectInlineStringEmpty {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), parseRejectInlineStringEmpty)
	}
}

// ---- EnvironmentVariable arm: happy + 3 failure paths ----

func TestResolveDataSource_EnvVarHappy(t *testing.T) {
	const name = "ENVOY_GO_WASM_TEST_VAR_HAPPY"
	const value = "synthetic-env-bytes-payload"
	t.Setenv(name, value)
	got, err := resolveDataSource(dsLocal(&corev3.DataSource_EnvironmentVariable{EnvironmentVariable: name}))
	if err != nil {
		t.Fatalf("resolveDataSource returned err: %v", err)
	}
	if string(got) != value {
		t.Fatalf("resolveDataSource bytes = %q; want %q", string(got), value)
	}
}

func TestResolveDataSource_EnvVarNameEmpty(t *testing.T) {
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_EnvironmentVariable{EnvironmentVariable: ""}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want parseRejectEnvVarNameEmpty")
	}
	if err.Error() != parseRejectEnvVarNameEmpty {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), parseRejectEnvVarNameEmpty)
	}
}

func TestResolveDataSource_EnvVarUnset(t *testing.T) {
	// Use a name very unlikely to be set in the env. We do NOT call
	// t.Setenv → so LookupEnv returns ok=false.
	const name = "ENVOY_GO_WASM_TEST_VAR_DEFINITELY_UNSET_XYZZY"
	// Defensively unset in case some weird env-pollution exists.
	_ = os.Unsetenv(name)
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_EnvironmentVariable{EnvironmentVariable: name}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want env-var-unset rejection")
	}
	want := "wasm: config.vm_config.code.local.environment_variable: " + name + ": unset"
	if err.Error() != want {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), want)
	}
}

func TestResolveDataSource_EnvVarEmptyValue(t *testing.T) {
	const name = "ENVOY_GO_WASM_TEST_VAR_EMPTY"
	t.Setenv(name, "") // set, but value is empty.
	_, err := resolveDataSource(dsLocal(&corev3.DataSource_EnvironmentVariable{EnvironmentVariable: name}))
	if err == nil {
		t.Fatal("resolveDataSource returned nil; want env-var-empty-value rejection")
	}
	want := "wasm: config.vm_config.code.local.environment_variable: " + name + ": value is empty"
	if err.Error() != want {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), want)
	}
}

// -----------------------------------------------------------------------------
// Integration via buildCompiledConfig — verifies arms 16/17 reachable now that
// resolveDataSource is real (per Task 10 PROGRESS forward-reference).
// -----------------------------------------------------------------------------

// TestBuildCompiledConfig_DataSource_Arm17_CompileFailed_InlineString verifies
// that with the resolveDataSource real body in place, non-wasm bytes flow
// THROUGH resolveDataSource and surface arm 17 (compile-failed %w-wrapped).
// This is the Task 9-deferred arm-17 reachability test, now enabled.
func TestBuildCompiledConfig_DataSource_Arm17_CompileFailed_InlineString(t *testing.T) {
	ctx := context.Background()
	m := validWasmConfig() // InlineString "some-non-wasm-bytes-stub"
	_, err := buildCompiledConfig(ctx, toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("buildCompiledConfig returned nil error; want arm-17 compile-failed wrap")
	}
	// Arm 17 wording: "wasm: config.vm_config.code: compile: %w"
	const wantPrefix = "wasm: config.vm_config.code: compile: "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err.Error() = %q; want prefix %q (arm-17 compile-failed)", err.Error(), wantPrefix)
	}
}

// TestBuildCompiledConfig_DataSource_Arm17_CompileFailed_Filename verifies
// the Filename arm flows through resolveDataSource to CompileModule and
// surfaces arm 17 for non-wasm bytes. End-to-end Filename-arm integration.
func TestBuildCompiledConfig_DataSource_Arm17_CompileFailed_Filename(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	path := filepath.Join(dir, "fake.wasm")
	if err := os.WriteFile(path, []byte("not-real-wasm-bytecode"), 0o644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	m := validWasmConfig()
	m.Config.GetVmConfig().Code = &corev3.AsyncDataSource{
		Specifier: &corev3.AsyncDataSource_Local{
			Local: &corev3.DataSource{
				Specifier: &corev3.DataSource_Filename{Filename: path},
			},
		},
	}
	_, err := buildCompiledConfig(ctx, toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("buildCompiledConfig returned nil error; want arm-17 via Filename")
	}
	const wantPrefix = "wasm: config.vm_config.code: compile: "
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err.Error() = %q; want prefix %q (arm-17 compile-failed via Filename)", err.Error(), wantPrefix)
	}
}

// TestBuildCompiledConfig_DataSource_FilenameEmpty_PropagatesArm verifies
// that a Filename-arm empty-name failure path propagates the byte-stable
// parseRejectFilenameEmpty wording up through buildCompiledConfig (no
// arm-17 wrap layered on top — the resolveDataSource error is returned
// verbatim per Task 9 buildCompiledConfig contract).
func TestBuildCompiledConfig_DataSource_FilenameEmpty_PropagatesArm(t *testing.T) {
	ctx := context.Background()
	m := validWasmConfig()
	m.Config.GetVmConfig().Code = &corev3.AsyncDataSource{
		Specifier: &corev3.AsyncDataSource_Local{
			Local: &corev3.DataSource{
				Specifier: &corev3.DataSource_Filename{Filename: ""},
			},
		},
	}
	_, err := buildCompiledConfig(ctx, toAny(t, m), envoyhttp.FactoryCtx{})
	if err == nil {
		t.Fatal("buildCompiledConfig returned nil error; want parseRejectFilenameEmpty")
	}
	if err.Error() != parseRejectFilenameEmpty {
		t.Fatalf("err.Error() = %q; want %q", err.Error(), parseRejectFilenameEmpty)
	}
}

// -----------------------------------------------------------------------------
// CapabilityRestrictionConfig — Task 9 surface kept-untouched at Task 10.
// The dsLocal helper exercises *corev3.DataSource directly; this comment-only
// row documents that buildSandboxConfig is OUT-OF-SCOPE for Task 10.
// -----------------------------------------------------------------------------

// _ wasmcommonv3 import-anchor (keeps the import block stable across edits;
// removed if buildSandboxConfig moves to its own file at a later task).
var _ wasmcommonv3.CapabilityRestrictionConfig
