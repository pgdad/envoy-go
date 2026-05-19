package lua

// datasource_test.go — Task 3 RIGID-TDD coverage of the full 4-arm DataSource
// resolution + the 10 PARSE-REJECT leaves per parent SPEC §6.2 arms 6-15 +
// AMEND-5 10-arm refinement. Covers:
//
//   - 4 valid arms: Filename / InlineBytes / InlineString / EnvironmentVariable
//     (each happy-path; bytes returned verbatim).
//   - 10 rejection leaves (one per parent §6.2 arms 6-15):
//        arm  6 — `data-source-specifier-required`            (empty oneof)
//        arm  7 — `data-source-watched-directory-deferred`    (WatchedDirectory set)
//        arm  8 — `data-source-filename-empty`                (Filename "")
//        arm  9 — `data-source-filename-read-failed`          (ENOENT / EACCES / EISDIR)
//        arm 10 — `data-source-filename-empty-contents`       (zero-byte file)
//        arm 11 — `data-source-inline-bytes-empty`            (zero-byte bytes)
//        arm 12 — `data-source-inline-string-empty`           (empty string)
//        arm 13 — `data-source-env-var-name-empty`            (name "")
//        arm 14 — `data-source-env-var-unset`                 (LookupEnv → false)
//        arm 15 — `data-source-env-var-empty-value`           (LookupEnv → true, "")
//
// Byte-stable wording pinned per parent SPEC §6.2 lines 352-361. Every assertion
// uses `err.Error() == <expected>` direct equality (not HasPrefix) for the
// fixed-string arms, and `strings.HasPrefix` for the wrapped-error arms (arm 9
// only — the inner os.PathError carries variable text per Go runtime version).

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// -----------------------------------------------------------------------------
// 4 valid-arm happy-path tests.
// -----------------------------------------------------------------------------

func TestResolveDataSource_ValidArms(t *testing.T) {
	t.Run("Filename", testResolveDataSourceFilenameHappy)
	t.Run("InlineBytes", testResolveDataSourceInlineBytesHappy)
	t.Run("InlineString", testResolveDataSourceInlineStringHappy)
	t.Run("EnvironmentVariable", testResolveDataSourceEnvironmentVariableHappy)
}

func testResolveDataSourceFilenameHappy(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "hello.lua")
	body := []byte("function envoy_on_request(rh) end\n")
	if err := os.WriteFile(scriptPath, body, 0o600); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}

	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_Filename{Filename: scriptPath},
	}
	got, err := resolveDataSource(ds)
	if err != nil {
		t.Fatalf("resolveDataSource(Filename): want nil error; got %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("resolveDataSource(Filename): bytes = %q; want %q", got, body)
	}
}

func testResolveDataSourceInlineBytesHappy(t *testing.T) {
	t.Parallel()
	body := []byte{0x01, 0x02, 0x03, 'a', 'b', 'c'}
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_InlineBytes{InlineBytes: body},
	}
	got, err := resolveDataSource(ds)
	if err != nil {
		t.Fatalf("resolveDataSource(InlineBytes): want nil error; got %v", err)
	}
	if string(got) != string(body) {
		t.Fatalf("resolveDataSource(InlineBytes): bytes = %v; want %v", got, body)
	}
}

func testResolveDataSourceInlineStringHappy(t *testing.T) {
	t.Parallel()
	body := "function envoy_on_response(rh) end\n"
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_InlineString{InlineString: body},
	}
	got, err := resolveDataSource(ds)
	if err != nil {
		t.Fatalf("resolveDataSource(InlineString): want nil error; got %v", err)
	}
	if string(got) != body {
		t.Fatalf("resolveDataSource(InlineString): bytes = %q; want %q", got, body)
	}
}

func testResolveDataSourceEnvironmentVariableHappy(t *testing.T) {
	// NOT parallel — t.Setenv writes process env (no t.Setenv + t.Parallel mixing
	// per stdlib testing/testing.go guidance).
	const name = "ENVOY_GO_LUA_TEST_SCRIPT_HAPPY"
	body := "function envoy_on_request(rh) end\n"
	t.Setenv(name, body)

	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_EnvironmentVariable{EnvironmentVariable: name},
	}
	got, err := resolveDataSource(ds)
	if err != nil {
		t.Fatalf("resolveDataSource(EnvironmentVariable): want nil error; got %v", err)
	}
	if string(got) != body {
		t.Fatalf("resolveDataSource(EnvironmentVariable): bytes = %q; want %q", got, body)
	}
}

// -----------------------------------------------------------------------------
// 10 PARSE-REJECT leaf tests (parent §6.2 arms 6-15).
//
// Naming: TestResolveDataSource_ParseReject_ArmNN_<short-description>.
//
// Byte-exact wordings copied from parent SPEC §6.2 (lines 352-361) verbatim.
// Drift between this file + dataSource.go + parent §6.2 is a 3-way lockstep
// edit per ADR-0044 atomic-edit discipline.
// -----------------------------------------------------------------------------

func TestResolveDataSource_ParseReject(t *testing.T) {
	t.Run("Arm06_Specifier_Required_EmptyOneof", testResolveDataSourceArm06SpecifierRequired)
	t.Run("Arm06_Specifier_Required_NilDataSource", testResolveDataSourceArm06SpecifierRequiredNilDS)
	t.Run("Arm07_WatchedDirectory_Deferred", testResolveDataSourceArm07WatchedDirectoryDeferred)
	t.Run("Arm08_Filename_Empty", testResolveDataSourceArm08FilenameEmpty)
	t.Run("Arm09_Filename_ReadFailed_ENOENT", testResolveDataSourceArm09FilenameENOENT)
	t.Run("Arm09_Filename_ReadFailed_EACCES", testResolveDataSourceArm09FilenameEACCES)
	t.Run("Arm09_Filename_ReadFailed_EISDIR", testResolveDataSourceArm09FilenameEISDIR)
	t.Run("Arm10_Filename_EmptyContents", testResolveDataSourceArm10FilenameEmptyContents)
	t.Run("Arm11_InlineBytes_Empty", testResolveDataSourceArm11InlineBytesEmpty)
	t.Run("Arm12_InlineString_Empty", testResolveDataSourceArm12InlineStringEmpty)
	t.Run("Arm13_EnvVar_NameEmpty", testResolveDataSourceArm13EnvVarNameEmpty)
	t.Run("Arm14_EnvVar_Unset", testResolveDataSourceArm14EnvVarUnset)
	t.Run("Arm15_EnvVar_EmptyValue", testResolveDataSourceArm15EnvVarEmptyValue)
}

// Arm 6 (a): bare DataSource{} with no oneof arm set → empty-oneof PARSE-REJECT.
func testResolveDataSourceArm06SpecifierRequired(t *testing.T) {
	t.Parallel()
	ds := &corev3.DataSource{} // no Specifier set
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(empty oneof): want error; got nil")
	}
	const want = "lua: default_source_code: specifier oneof required"
	if err.Error() != want {
		t.Fatalf("err = %q; want %q", err.Error(), want)
	}
}

// Arm 6 (b): nil *DataSource pointer → same arm-6 PARSE-REJECT wording.
// (defensive coverage for the call-site that passes a nil *DataSource —
// buildCompiledConfig short-circuits this at arm 5 D1-REFUTED silent-no-op,
// but resolveDataSource should still PARSE-REJECT defensively.)
func testResolveDataSourceArm06SpecifierRequiredNilDS(t *testing.T) {
	t.Parallel()
	_, err := resolveDataSource(nil)
	if err == nil {
		t.Fatalf("resolveDataSource(nil): want error; got nil")
	}
	const want = "lua: default_source_code: specifier oneof required"
	if err.Error() != want {
		t.Fatalf("err = %q; want %q", err.Error(), want)
	}
}

// Arm 7: ds.GetWatchedDirectory() != nil → deferred PARSE-REJECT.
// Per AMEND-5: WatchedDirectory is a sibling field (NOT part of the oneof).
// The proto allows BOTH a Specifier (e.g. Filename) AND a WatchedDirectory
// simultaneously — Envoy only honors WatchedDirectory when Filename is set.
// envoy-go PARSE-REJECTs any WatchedDirectory presence regardless of the
// Specifier arm (per parent §6.2 arm 7).
func testResolveDataSourceArm07WatchedDirectoryDeferred(t *testing.T) {
	t.Parallel()
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_Filename{Filename: "/tmp/whatever.lua"},
		WatchedDirectory: &corev3.WatchedDirectory{
			Path: "/tmp",
		},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(WatchedDirectory): want error; got nil")
	}
	const want = "lua: default_source_code: watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)"
	if err.Error() != want {
		t.Fatalf("err = %q; want %q", err.Error(), want)
	}
}

// Arm 8: Filename "" → name-empty PARSE-REJECT.
func testResolveDataSourceArm08FilenameEmpty(t *testing.T) {
	t.Parallel()
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_Filename{Filename: ""},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(Filename=\"\"): want error; got nil")
	}
	const want = "lua: default_source_code: filename empty"
	if err.Error() != want {
		t.Fatalf("err = %q; want %q", err.Error(), want)
	}
}

// Arm 9 (a): ENOENT — file does not exist.
func testResolveDataSourceArm09FilenameENOENT(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.lua")
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_Filename{Filename: missing},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(ENOENT): want error; got nil")
	}
	// Byte-stable PREFIX: arm-9 wording wraps the inner os.PathError; assert
	// the structural prefix + that the wrapped error matches os.ErrNotExist.
	wantPrefix := fmt.Sprintf("lua: default_source_code: read file %q: ", missing)
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err = %q; want prefix %q", err.Error(), wantPrefix)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v; want errors.Is(_, os.ErrNotExist)", err)
	}
}

// Arm 9 (b): EACCES — file unreadable. Uses t.TempDir + 0o000 permission
// on the file (NOT the directory — chmodding the dir to 0o000 would make
// cleanup fail).
func testResolveDataSourceArm09FilenameEACCES(t *testing.T) {
	// Skip under root — root bypasses POSIX read-permission checks.
	if os.Geteuid() == 0 {
		t.Skip("skipping EACCES test under root (POSIX permissions bypassed)")
	}
	t.Parallel()
	dir := t.TempDir()
	unreadable := filepath.Join(dir, "no-read.lua")
	if err := os.WriteFile(unreadable, []byte("x"), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod 000: %v", err)
	}
	// Restore permissions BEFORE t.TempDir's cleanup runs so cleanup doesn't
	// fail to remove the file. Use t.Cleanup to ensure ordering.
	t.Cleanup(func() {
		_ = os.Chmod(unreadable, 0o600)
	})

	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_Filename{Filename: unreadable},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(EACCES): want error; got nil")
	}
	wantPrefix := fmt.Sprintf("lua: default_source_code: read file %q: ", unreadable)
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err = %q; want prefix %q", err.Error(), wantPrefix)
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("err = %v; want errors.Is(_, os.ErrPermission)", err)
	}
}

// Arm 9 (c): EISDIR — Filename points at a directory.
func testResolveDataSourceArm09FilenameEISDIR(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // exists + is a directory
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_Filename{Filename: dir},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(EISDIR): want error; got nil")
	}
	wantPrefix := fmt.Sprintf("lua: default_source_code: read file %q: ", dir)
	if !strings.HasPrefix(err.Error(), wantPrefix) {
		t.Fatalf("err = %q; want prefix %q", err.Error(), wantPrefix)
	}
	// EISDIR surfaces as an *os.PathError wrapping syscall.EISDIR or
	// "is a directory" depending on Go version + OS; we don't pin the
	// inner sentinel beyond the prefix-match.
}

// Arm 10: file is readable but zero-byte.
func testResolveDataSourceArm10FilenameEmptyContents(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.lua")
	if err := os.WriteFile(empty, nil, 0o600); err != nil {
		t.Fatalf("write empty fixture: %v", err)
	}
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_Filename{Filename: empty},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(zero-byte): want error; got nil")
	}
	want := fmt.Sprintf("lua: default_source_code: file %q is empty", empty)
	if err.Error() != want {
		t.Fatalf("err = %q; want %q", err.Error(), want)
	}
}

// Arm 11: InlineBytes len == 0.
func testResolveDataSourceArm11InlineBytesEmpty(t *testing.T) {
	t.Parallel()
	for _, body := range [][]byte{nil, {}} {
		body := body
		t.Run(fmt.Sprintf("len%d", len(body)), func(t *testing.T) {
			t.Parallel()
			ds := &corev3.DataSource{
				Specifier: &corev3.DataSource_InlineBytes{InlineBytes: body},
			}
			_, err := resolveDataSource(ds)
			if err == nil {
				t.Fatalf("resolveDataSource(InlineBytes=%v): want error; got nil", body)
			}
			const want = "lua: default_source_code: inline_bytes empty"
			if err.Error() != want {
				t.Fatalf("err = %q; want %q", err.Error(), want)
			}
		})
	}
}

// Arm 12: InlineString == "".
func testResolveDataSourceArm12InlineStringEmpty(t *testing.T) {
	t.Parallel()
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_InlineString{InlineString: ""},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(InlineString=\"\"): want error; got nil")
	}
	const want = "lua: default_source_code: inline_string empty"
	if err.Error() != want {
		t.Fatalf("err = %q; want %q", err.Error(), want)
	}
}

// Arm 13: EnvironmentVariable name "".
func testResolveDataSourceArm13EnvVarNameEmpty(t *testing.T) {
	t.Parallel()
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_EnvironmentVariable{EnvironmentVariable: ""},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(env name=\"\"): want error; got nil")
	}
	const want = "lua: default_source_code: environment_variable name empty"
	if err.Error() != want {
		t.Fatalf("err = %q; want %q", err.Error(), want)
	}
}

// Arm 14: env var name set but LookupEnv returns false.
// Use a name unlikely to exist in any CI environment, and explicitly Unsetenv
// for hermeticity.
func testResolveDataSourceArm14EnvVarUnset(t *testing.T) {
	// NOT parallel — Unsetenv writes process env.
	const name = "ENVOY_GO_LUA_TEST_UNSET_VAR_DOES_NOT_EXIST_XYZZY"
	if err := os.Unsetenv(name); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_EnvironmentVariable{EnvironmentVariable: name},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(env unset): want error; got nil")
	}
	want := fmt.Sprintf("lua: default_source_code: environment_variable %q not set", name)
	if err.Error() != want {
		t.Fatalf("err = %q; want %q", err.Error(), want)
	}
}

// Arm 15: env var name set, LookupEnv returns (true, ""). Uses t.Setenv with
// the empty string.
func testResolveDataSourceArm15EnvVarEmptyValue(t *testing.T) {
	// NOT parallel — t.Setenv writes process env.
	const name = "ENVOY_GO_LUA_TEST_EMPTY_VALUE"
	t.Setenv(name, "")
	ds := &corev3.DataSource{
		Specifier: &corev3.DataSource_EnvironmentVariable{EnvironmentVariable: name},
	}
	_, err := resolveDataSource(ds)
	if err == nil {
		t.Fatalf("resolveDataSource(env empty value): want error; got nil")
	}
	want := fmt.Sprintf("lua: default_source_code: environment_variable %q is empty", name)
	if err.Error() != want {
		t.Fatalf("err = %q; want %q", err.Error(), want)
	}
}

// -----------------------------------------------------------------------------
// Byte-exact wording pin test — the fixed-string PARSE-REJECT constants for
// arms 6, 7, 8, 10 (fmt-template), 11, 12, 13, 14 (fmt-template), 15
// (fmt-template) match the parent SPEC §6.2 verbatim. Any drift surfaces here
// as a build/test break per ADR-0044 atomic-edit discipline.
//
// This is the SECOND wording-pin test in this package (the first lives at
// compiled_config_test.go::TestParseRejectConstants_ByteExactWording covering
// arms 1, 3, 4, 18). Together they cover the full Task-2-Task-3 byte-stable
// surface for arms 1, 3, 4, 6-15, 18 (arm 2 covered by a separate prefix-test;
// arms 5 + 17 are D1-REFUTED reserved-verbatim).
// -----------------------------------------------------------------------------

func TestResolveDataSource_ByteExactWording(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"Arm06", parseRejectDataSourceSpecifierRequired, "lua: default_source_code: specifier oneof required"},
		{"Arm07", parseRejectDataSourceWatchedDirectoryDeferred, "lua: default_source_code: watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)"},
		{"Arm08", parseRejectDataSourceFilenameEmpty, "lua: default_source_code: filename empty"},
		{"Arm09_tmpl", parseRejectDataSourceFilenameReadFailedFmt, "lua: default_source_code: read file %q: %w"},
		{"Arm10_tmpl", parseRejectDataSourceFilenameEmptyContentsFmt, "lua: default_source_code: file %q is empty"},
		{"Arm11", parseRejectDataSourceInlineBytesEmpty, "lua: default_source_code: inline_bytes empty"},
		{"Arm12", parseRejectDataSourceInlineStringEmpty, "lua: default_source_code: inline_string empty"},
		{"Arm13", parseRejectDataSourceEnvVarNameEmpty, "lua: default_source_code: environment_variable name empty"},
		{"Arm14_tmpl", parseRejectDataSourceEnvVarUnsetFmt, "lua: default_source_code: environment_variable %q not set"},
		{"Arm15_tmpl", parseRejectDataSourceEnvVarEmptyValueFmt, "lua: default_source_code: environment_variable %q is empty"},
		{"Arm09Ext_TooLarge_tmpl", parseRejectDataSourceFilenameTooLargeFmt, "lua: default_source_code: file %q exceeds the maximum script size of %d bytes"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q; want %q", tc.name, tc.got, tc.want)
		}
	}
}

// TestResolveDataSourceFilename_DevFull_TooLarge_NoHang verifies the
// arm-9-extension PARSE-REJECT for infinite-read special files like
// `/dev/full`. Surfaced by Task 11 fuzzer FuzzLuaConfigParse: without
// `io.LimitReader` + maxFilenameScriptBytes, `os.ReadFile("/dev/full")`
// allocates unboundedly until OOM-kill. With the bounded read, the
// rejection surfaces as a clean PARSE-REJECT within milliseconds.
//
// Skipped on systems without /dev/full (Linux-only special device).
func TestResolveDataSourceFilename_DevFull_TooLarge_NoHang(t *testing.T) {
	t.Parallel()
	if _, err := os.Stat("/dev/full"); err != nil {
		t.Skipf("/dev/full unavailable: %v (Linux-only test)", err)
	}
	// The /dev/full read returns infinite NUL bytes; LimitReader caps
	// at maxFilenameScriptBytes+1 so this completes in O(cap) time
	// without OOM.
	got, err := resolveDataSourceFilename("/dev/full")
	if err == nil {
		t.Fatal("resolveDataSourceFilename(/dev/full) returned nil err; want too-large PARSE-REJECT")
	}
	if got != nil {
		t.Errorf("resolveDataSourceFilename(/dev/full) returned non-nil body: len=%d; want nil", len(got))
	}
	want := "lua: default_source_code: file \"/dev/full\" exceeds the maximum script size of 16777216 bytes"
	if err.Error() != want {
		t.Errorf("err = %q; want %q", err.Error(), want)
	}
}

// TestResolveDataSourceFilename_MaxSize_Boundary verifies the boundary
// behavior of the maxFilenameScriptBytes cap: a script EXACTLY at the
// cap is accepted; a script ONE BYTE OVER the cap is rejected. Uses
// t.TempDir to author the boundary fixtures inside the test.
func TestResolveDataSourceFilename_MaxSize_Boundary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// At-the-cap: maxFilenameScriptBytes bytes — accepted.
	atCapPath := dir + "/at_cap.lua"
	atCap := make([]byte, maxFilenameScriptBytes)
	for i := range atCap {
		atCap[i] = 'a'
	}
	if err := os.WriteFile(atCapPath, atCap, 0o600); err != nil {
		t.Fatalf("WriteFile at_cap: %v", err)
	}
	got, err := resolveDataSourceFilename(atCapPath)
	if err != nil {
		t.Errorf("at-cap: err = %v; want nil", err)
	}
	if len(got) != maxFilenameScriptBytes {
		t.Errorf("at-cap: len(body) = %d; want %d", len(got), maxFilenameScriptBytes)
	}

	// One-byte-over: maxFilenameScriptBytes+1 bytes — rejected.
	overPath := dir + "/over_cap.lua"
	over := make([]byte, maxFilenameScriptBytes+1)
	for i := range over {
		over[i] = 'b'
	}
	if err := os.WriteFile(overPath, over, 0o600); err != nil {
		t.Fatalf("WriteFile over_cap: %v", err)
	}
	gotOver, errOver := resolveDataSourceFilename(overPath)
	if errOver == nil {
		t.Fatal("over-cap: err = nil; want too-large PARSE-REJECT")
	}
	if gotOver != nil {
		t.Errorf("over-cap: body = non-nil (len=%d); want nil", len(gotOver))
	}
	wantPrefix := "lua: default_source_code: file "
	if !strings.HasPrefix(errOver.Error(), wantPrefix) {
		t.Errorf("over-cap: err = %q; want prefix %q", errOver.Error(), wantPrefix)
	}
	wantSuffix := " exceeds the maximum script size of 16777216 bytes"
	if !strings.HasSuffix(errOver.Error(), wantSuffix) {
		t.Errorf("over-cap: err = %q; want suffix %q", errOver.Error(), wantSuffix)
	}
}
