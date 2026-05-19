package lua

// datasource.go — full 4-arm DataSource resolution per 22.1 SPEC §6 Task 3 +
// parent SPEC §5.3 + §6.2 arms 6-15 + AMEND-5 10-arm refinement. REPLACES the
// Task 2 stub from scratch.
//
// # Arm dispatch (per parent §5.3 + §6.2 + AMEND-5)
//
// The envoy.config.core.v3.DataSource message carries a 4-arm `specifier`
// oneof (`Filename` / `InlineBytes` / `InlineString` / `EnvironmentVariable`)
// plus a 5th sibling field `WatchedDirectory` (NOT part of the oneof). The
// 22.1 surface honors the 4 oneof arms + PARSE-REJECTs WatchedDirectory + the
// empty-oneof case, expanding to 10 distinct rejection leaves per AMEND-5:
//
//   - Arm  6 — `data-source-specifier-required`          (ds == nil OR
//                                                          ds.Specifier == nil)
//   - Arm  7 — `data-source-watched-directory-deferred`  (ds.WatchedDirectory != nil;
//                                                          checked BEFORE the
//                                                          oneof dispatch so a
//                                                          combined Filename+
//                                                          WatchedDirectory
//                                                          config rejects on
//                                                          arm 7 first)
//   - Arm  8 — `data-source-filename-empty`              (Filename "")
//   - Arm  9 — `data-source-filename-read-failed`        (os.ReadFile error:
//                                                          ENOENT / EACCES /
//                                                          EISDIR / etc.;
//                                                          wraps inner error
//                                                          via fmt.Errorf %w)
//   - Arm 10 — `data-source-filename-empty-contents`     (file readable, len 0)
//   - Arm 11 — `data-source-inline-bytes-empty`          (len(InlineBytes) == 0)
//   - Arm 12 — `data-source-inline-string-empty`         (InlineString "")
//   - Arm 13 — `data-source-env-var-name-empty`          (EnvironmentVariable "")
//   - Arm 14 — `data-source-env-var-unset`               (os.LookupEnv → false)
//   - Arm 15 — `data-source-env-var-empty-value`         (os.LookupEnv → true,
//                                                          empty-string value)
//
// # Byte-stable wording discipline (per parent §6.1)
//
// All wording constants in this file begin with the `lua:` prefix per parent
// §6.1 + ADR-0080. Drift between this file + parent SPEC §6.2 lines 352-361 +
// the datasource_test.go::TestResolveDataSource_ByteExactWording assertions is
// a 3-way lockstep edit per ADR-0044 atomic-edit discipline.
//
// # WatchedDirectory deferred (per parent §6.2 arm 7 + §2.1)
//
// WatchedDirectory is a Runtime/RTDS/hot-reload concern (Envoy reloads the
// file referenced by `Filename` when a relevant move event fires inside the
// watched directory). The 22.1 surface PARSE-REJECTs any WatchedDirectory
// presence; deferred to a future Runtime/RTDS family phase. NOT linked to
// per-route configuration (which is the 22.3 concern, distinct surface).
//
// # WatchedDirectory ordering (deliberate; pin)
//
// The arm-7 check fires BEFORE the oneof dispatch. The proto allows a
// DataSource with BOTH a Specifier (e.g. Filename) AND a WatchedDirectory
// simultaneously (the proto field-7 sibling); the upstream Envoy semantics
// only honor WatchedDirectory when Filename is set. envoy-go PARSE-REJECTs
// any WatchedDirectory presence regardless of the Specifier arm — choosing
// arm 7 over arm 8/9/10 when both apply gives the operator the most
// actionable error message (the WatchedDirectory feature is the load-bearing
// "this won't work" signal; the Filename arm validity is a downstream
// concern).
//
// # nil-tolerance (per ADR-0085)
//
// resolveDataSource(nil) returns the arm-6 PARSE-REJECT wording (not a panic).
// The call site at compiled_config.go::buildCompiledConfig short-circuits
// nil-DefaultSourceCode at the D1-REFUTED arm 5 silent-no-op path BEFORE
// invoking resolveDataSource — so the nil branch here is defensive coverage
// per ADR-0085 nil-tolerance discipline.
//
// # Cross-references
//
//   - ADR-0080 (byte-stable error wording)
//   - ADR-0085 (nil-tolerance)
//   - ADR-0188 (NEW internal/lua/ framework primitive — resolveDataSource
//     feeds CompileScript)
//   - ADR-0189 (NEW internal/filter/http/lua/ package shape — datasource.go
//     is one of the 8 production files per parent §4.4 + 22.1 SPEC §3.5)
//   - parent SPEC §5.3 (DataSource 5-arm structural reference)
//   - parent SPEC §6.2 arms 6-15 (byte-stable wording roster)
//   - parent SPEC AMEND-5 (10-arm rejection-leaf refinement)
//   - 22.1 SPEC §6 Task 3 (this file's task spec)

import (
	"errors"
	"fmt"
	"io"
	"os"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// maxFilenameScriptBytes is the upper bound on bytes read from a
// `DefaultSourceCode.Filename` script. 16 MiB is generous enough to
// accommodate any realistic Lua script (the largest scripts in the wild
// are ~100 KiB; the upstream Envoy v1.37.2 lua_filter.cc reads the whole
// file into memory but does not enforce a hard cap; envoy-go's cap is
// a defense-in-depth limit per ADR-0085 input-bounds discipline).
//
// Surfaced by Task 11 fuzzer FuzzLuaConfigParse: an operator-supplied
// `Filename: "/dev/full"` (or any infinite-read device / pipe) caused
// `os.ReadFile` to allocate unboundedly until the Go runtime OOM-kills
// the process. The Filename path is operator-supplied — an outage
// vector from a YAML typo or a hostile config-management bug.
//
// We use `io.LimitReader` rather than `os.Stat`-then-`os.ReadFile`
// because special files (`/dev/full`, named pipes) return Size 0 from
// Stat regardless of the actual readable length; LimitReader is the
// universal-correct enforcement layer.
//
// The limit is byte-exact: a script EXACTLY at the cap is accepted; a
// script ONE BYTE OVER the cap is rejected with a clear error message.
// We read cap+1 bytes; if the result exceeds cap, we know the file is
// larger than the cap (or infinite).
const maxFilenameScriptBytes = 16 * 1024 * 1024 // 16 MiB

// parseRejectDataSourceFilenameTooLargeFmt is the format template for
// the arm-9-extension PARSE-REJECT wording when a Filename-arm script
// exceeds maxFilenameScriptBytes. The verb-bearing template carries
// the filename + the cap-size for actionable operator diagnostics.
const parseRejectDataSourceFilenameTooLargeFmt = "lua: default_source_code: file %q exceeds the maximum script size of %d bytes"

// -----------------------------------------------------------------------------
// PARSE-REJECT byte-stable wordings per parent SPEC §6.2 arms 6-15.
//
// Fixed-string arms (6, 7, 8, 11, 12, 13) are `const string`.
// Format-template arms (9, 10, 14, 15) carry the verb-bearing template; the
// helpers below invoke fmt.Errorf against the template.
//
// Every wording starts with `lua:` per parent §6.1 + ADR-0080.
// -----------------------------------------------------------------------------

const (
	// Arm 6: ds.GetSpecifier() == nil (no oneof arm set; bare DataSource{}).
	// Also fires for nil *DataSource per ADR-0085 nil-tolerance.
	parseRejectDataSourceSpecifierRequired = "lua: default_source_code: specifier oneof required"

	// Arm 7: ds.GetWatchedDirectory() != nil (sibling field present).
	// Deferred to future Runtime/RTDS/hot-reload family phase per parent §2.1.
	parseRejectDataSourceWatchedDirectoryDeferred = "lua: default_source_code: watched_directory is not yet supported (lands in a future Runtime/hot-reload phase)"

	// Arm 8: Filename arm with empty Filename.
	parseRejectDataSourceFilenameEmpty = "lua: default_source_code: filename empty"

	// Arm 9 format template: os.ReadFile error wrapper.
	// Verbs: %q (filename) + %w (wrapped inner error).
	parseRejectDataSourceFilenameReadFailedFmt = "lua: default_source_code: read file %q: %w"

	// Arm 10 format template: file readable but zero-byte.
	// Verb: %q (filename).
	parseRejectDataSourceFilenameEmptyContentsFmt = "lua: default_source_code: file %q is empty"

	// Arm 11: InlineBytes arm with zero-byte contents.
	parseRejectDataSourceInlineBytesEmpty = "lua: default_source_code: inline_bytes empty"

	// Arm 12: InlineString arm with empty-string contents.
	parseRejectDataSourceInlineStringEmpty = "lua: default_source_code: inline_string empty"

	// Arm 13: EnvironmentVariable arm with empty env-var name.
	parseRejectDataSourceEnvVarNameEmpty = "lua: default_source_code: environment_variable name empty"

	// Arm 14 format template: env var name set but LookupEnv returns false.
	// Verb: %q (env var name).
	parseRejectDataSourceEnvVarUnsetFmt = "lua: default_source_code: environment_variable %q not set"

	// Arm 15 format template: env var name set, LookupEnv returns (true, "").
	// Verb: %q (env var name).
	parseRejectDataSourceEnvVarEmptyValueFmt = "lua: default_source_code: environment_variable %q is empty"
)

// -----------------------------------------------------------------------------
// resolveDataSource — single-chokepoint 4-arm dispatch per 22.1 SPEC §6 Task 3.
// -----------------------------------------------------------------------------

// resolveDataSource resolves a *corev3.DataSource oneof into the in-memory
// script bytes per parent §5.3 + §6.2 arms 6-15 + AMEND-5 10-arm refinement.
//
// Return contract:
//   - (bytes, nil) on success (one of the 4 valid arms with non-empty contents)
//   - (nil, error) on PARSE-REJECT — byte-stable error wording per parent §6.2.
//     errors.Is exposes the underlying os.PathError for arm 9 (read-failed).
//
// Arm ordering (deliberate):
//  1. nil *DataSource → arm 6 (defensive per ADR-0085)
//  2. WatchedDirectory presence → arm 7 (BEFORE oneof dispatch; see file-level
//     "WatchedDirectory ordering" comment for the rationale)
//  3. Specifier nil → arm 6 (empty-oneof)
//  4. Oneof dispatch:
//     - Filename → resolveDataSourceFilename (arms 8/9/10)
//     - InlineBytes → resolveDataSourceInlineBytes (arm 11)
//     - InlineString → resolveDataSourceInlineString (arm 12)
//     - EnvironmentVariable → resolveDataSourceEnvVar (arms 13/14/15)
func resolveDataSource(ds *corev3.DataSource) ([]byte, error) {
	if ds == nil {
		return nil, errors.New(parseRejectDataSourceSpecifierRequired)
	}

	// Arm 7 — WatchedDirectory PARSE-REJECT BEFORE the oneof dispatch. The
	// proto allows a DataSource with both a Specifier and a WatchedDirectory
	// simultaneously; envoy-go PARSE-REJECTs the WatchedDirectory presence
	// regardless of the Specifier arm. See file-level "WatchedDirectory
	// ordering" comment for the rationale.
	if ds.GetWatchedDirectory() != nil {
		return nil, errors.New(parseRejectDataSourceWatchedDirectoryDeferred)
	}

	switch s := ds.GetSpecifier().(type) {
	case *corev3.DataSource_Filename:
		return resolveDataSourceFilename(s.Filename)
	case *corev3.DataSource_InlineBytes:
		return resolveDataSourceInlineBytes(s.InlineBytes)
	case *corev3.DataSource_InlineString:
		return resolveDataSourceInlineString(s.InlineString)
	case *corev3.DataSource_EnvironmentVariable:
		return resolveDataSourceEnvVar(s.EnvironmentVariable)
	default:
		// Specifier == nil (bare DataSource{} with no oneof arm set). Per
		// parent §6.2 arm 6, returns the empty-oneof PARSE-REJECT. This is
		// the same wording as the nil-*DataSource defensive branch above.
		return nil, errors.New(parseRejectDataSourceSpecifierRequired)
	}
}

// -----------------------------------------------------------------------------
// Per-arm helpers.
// -----------------------------------------------------------------------------

// resolveDataSourceFilename handles the Filename arm per parent §6.2 arms 8/9/10.
//
// PARSE-REJECT leaves:
//   - arm 8  — name "" → fixed-string wording
//   - arm 9  — os.ReadFile error (ENOENT / EACCES / EISDIR / etc.) → fmt.Errorf
//     wrap with %q (name) + %w (inner). errors.Is exposes the inner
//     os.PathError to callers (e.g. for differential test classification).
//   - arm 10 — read OK, zero-byte contents → fmt.Errorf wrap with %q (name)
func resolveDataSourceFilename(name string) ([]byte, error) {
	if name == "" {
		return nil, errors.New(parseRejectDataSourceFilenameEmpty)
	}
	// Defense-in-depth bounded read per maxFilenameScriptBytes. Replaces
	// the naive `os.ReadFile` to defend against infinite-read special
	// files (`/dev/full`, named pipes) + against accidentally-huge
	// scripts (the operator-side YAML typo / config-management mistake
	// could otherwise OOM the proxy at boot). Surfaced by Task 11
	// fuzzer; pattern matches the upstream defense-in-depth input-cap
	// discipline.
	//
	//nolint:gosec // G304: operator-supplied path is the contract (lua filter loads its script from operator-specified file)
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf(parseRejectDataSourceFilenameReadFailedFmt, name, err)
	}
	defer func() { _ = f.Close() }()
	// Read cap+1 bytes: if the result exceeds cap, we know the file is
	// larger than the cap (or unbounded). We allocate up-front with the
	// cap+1 bound so the slice never grows past the limit.
	body, err := io.ReadAll(io.LimitReader(f, maxFilenameScriptBytes+1))
	if err != nil {
		return nil, fmt.Errorf(parseRejectDataSourceFilenameReadFailedFmt, name, err)
	}
	if len(body) > maxFilenameScriptBytes {
		return nil, fmt.Errorf(parseRejectDataSourceFilenameTooLargeFmt, name, maxFilenameScriptBytes)
	}
	if len(body) == 0 {
		return nil, fmt.Errorf(parseRejectDataSourceFilenameEmptyContentsFmt, name)
	}
	return body, nil
}

// resolveDataSourceInlineBytes handles the InlineBytes arm per parent §6.2
// arm 11. Returns the bytes verbatim (no copy — caller treats as read-only).
func resolveDataSourceInlineBytes(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, errors.New(parseRejectDataSourceInlineBytesEmpty)
	}
	return b, nil
}

// resolveDataSourceInlineString handles the InlineString arm per parent §6.2
// arm 12. Converts string → []byte via a copy (Go's string→[]byte cast
// semantics — the returned slice is independent of any subsequent string GC).
func resolveDataSourceInlineString(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New(parseRejectDataSourceInlineStringEmpty)
	}
	return []byte(s), nil
}

// resolveDataSourceEnvVar handles the EnvironmentVariable arm per parent §6.2
// arms 13/14/15.
//
// PARSE-REJECT leaves:
//   - arm 13 — name "" → fixed-string wording
//   - arm 14 — name set, LookupEnv returns false → fmt.Errorf wrap with %q
//   - arm 15 — name set, LookupEnv returns (true, "") → fmt.Errorf wrap with %q
//
// The distinction between arms 14 + 15 matters operationally: "unset" means
// the env var is not present at all (typo? deployment forgot to set?);
// "empty value" means the operator explicitly set it to the empty string
// (more likely a config-management bug — an upstream substitution dropped).
// The byte-stable wording disambiguates per parent §6.2.
func resolveDataSourceEnvVar(name string) ([]byte, error) {
	if name == "" {
		return nil, errors.New(parseRejectDataSourceEnvVarNameEmpty)
	}
	v, ok := os.LookupEnv(name)
	if !ok {
		return nil, fmt.Errorf(parseRejectDataSourceEnvVarUnsetFmt, name)
	}
	if v == "" {
		return nil, fmt.Errorf(parseRejectDataSourceEnvVarEmptyValueFmt, name)
	}
	return []byte(v), nil
}
