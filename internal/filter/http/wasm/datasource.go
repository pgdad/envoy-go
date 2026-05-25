package wasm

// datasource.go — Task 10 `resolveDataSource` 4-arm AsyncDataSource.Local
// resolution per parent SPEC §5.4. Replaces the Task 9 forward-stub in
// compiled_config.go with the real body covering the 4 supported arms
// (Filename / InlineBytes / InlineString / EnvironmentVariable) + the
// per-arm content-empty PARSE-REJECT sub-failure paths.
//
// # Pre-conditions (enforced at buildCompiledConfig BEFORE this is called)
//
//   - `local != nil` (arm 5 `vm-config-code-required` rejects the
//     AsyncDataSource.Local-unset path; arm 8 then verifies the inner
//     specifier oneof is present).
//   - `local.Specifier != nil` (arm 8 `data-source-specifier-required` —
//     enforced at buildCompiledConfig BEFORE resolveDataSource).
//   - `local.WatchedDirectory == nil` (arm 7 `data-source-watched-directory-
//     deferred` — enforced at buildCompiledConfig BEFORE resolveDataSource).
//
// So resolveDataSource ONLY handles the 4 valid arms (Filename / InlineBytes
// / InlineString / EnvironmentVariable) + their per-arm content-empty
// PARSE-REJECT sub-failure paths. The `default:` switch branch is a
// defensive return of the arm-8 wording (unreachable in production per
// the pre-conditions above; covers any future oneof extension).
//
// # Per-arm sub-failure PARSE-REJECT consts (D-P5 sub-arm wording-discipline)
//
// Each per-arm failure path carries its own byte-stable wording for operator
// diagnostic fidelity (a NotFound vs Unreadable vs EmptyContent surface that
// the operator can read at a glance to know which knob to twist). These are
// SUB-arms of the parent SPEC §6.2 arm-8 family (data-source-specifier-
// required broadens to "data-source-specifier-resolution-failed"). The 9
// consts below are byte-pinned at `TestParseRejectDataSourceConstants_
// ByteStable` per Task 9's wording-discipline pattern.
//
// # Cross-references
//
//   - parent SPEC §5.4 (AsyncDataSource.Local 4-arm resolution surface)
//   - parent SPEC §6.1 (wording-discipline) + §6.2 (18-arm roster — arm 8
//     family broadens to per-arm sub-failure wordings landed here)
//   - 25.1 PLAN Task 10 (this file)
//   - 25.1 SPEC §6 Task 10 (4-arm resolution body anchor)
//   - ADR-0072 (boot-time-fail-fast — resolveDataSource errors surface as
//     boot-time config mistakes via buildCompiledConfig's error return)
//   - ADR-0080 (envoy-go-strict departure discipline + byte-stable wording)

import (
	"errors"
	"fmt"
	"os"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
)

// -----------------------------------------------------------------------------
// PARSE-REJECT sub-arm wordings — Task 10 D-P5 extension at the sub-arm level.
//
// EVERY constant prefix is `wasm:` per parent §6.1 + ADR-0080. Byte-exact
// wording pinned at datasource_test.go::TestParseRejectDataSourceConstants_
// ByteStable. Any drift requires a parent-SPEC §6.2 + ADR-0203 lockstep edit
// per ADR-0044 atomic-edit discipline.
// -----------------------------------------------------------------------------

const (
	// Filename arm: name-string empty (operator set `local.filename: ""`).
	parseRejectFilenameEmpty = "wasm: config.vm_config.code.local.filename is empty"

	// Filename arm: ENOENT (file does not exist). %s-formatted with the path
	// for operator diagnostic context (the operator may have typo'd the path).
	parseRejectFilenameNotFound = "wasm: config.vm_config.code.local.filename: %s: not found"

	// Filename arm: any other os.ReadFile error (EACCES, EISDIR, EIO, etc.).
	// %s-formatted with the path + %w-wraps the inner *fs.PathError so the
	// caller can drill in via errors.Is/errors.As for the underlying syscall.
	parseRejectFilenameUnreadable = "wasm: config.vm_config.code.local.filename: %s: read error: %w"

	// Filename arm: file exists + readable but zero-byte content.
	parseRejectFilenameEmptyContent = "wasm: config.vm_config.code.local.filename: %s: file is empty"

	// InlineBytes arm: nil or zero-length byte slice.
	parseRejectInlineBytesEmpty = "wasm: config.vm_config.code.local.inline_bytes is empty"

	// InlineString arm: empty string.
	parseRejectInlineStringEmpty = "wasm: config.vm_config.code.local.inline_string is empty"

	// EnvironmentVariable arm: variable-name string empty (operator set
	// `local.environment_variable: ""`).
	parseRejectEnvVarNameEmpty = "wasm: config.vm_config.code.local.environment_variable is empty"

	// EnvironmentVariable arm: variable not set in the process environment.
	// %s-formatted with the variable name for diagnostic context.
	parseRejectEnvVarUnset = "wasm: config.vm_config.code.local.environment_variable: %s: unset"

	// EnvironmentVariable arm: variable IS set but value is the empty string.
	parseRejectEnvVarEmptyValue = "wasm: config.vm_config.code.local.environment_variable: %s: value is empty"
)

// -----------------------------------------------------------------------------
// resolveDataSource — 4-arm dispatch entry point.
// -----------------------------------------------------------------------------

// resolveDataSource dispatches the AsyncDataSource.Local oneof across the
// 4 supported arms (Filename / InlineBytes / InlineString / EnvironmentVariable)
// per parent §5.4. Returns the resolved bytes on success OR a wrapped error
// using the per-arm parseRejectDataSource* consts when the resolved content
// is empty or the named file/env-var is unavailable.
//
// Pre-conditions (enforced at buildCompiledConfig BEFORE this is called):
//   - local != nil
//   - local.Specifier != nil
//   - local.WatchedDirectory == nil
//
// The function only handles the 4 valid arms + per-arm content-empty failure
// paths. The default switch branch is defensive (unreachable in production).
func resolveDataSource(local *corev3.DataSource) ([]byte, error) {
	switch spec := local.Specifier.(type) {
	case *corev3.DataSource_Filename:
		return resolveFilename(spec.Filename)
	case *corev3.DataSource_InlineBytes:
		return resolveInlineBytes(spec.InlineBytes)
	case *corev3.DataSource_InlineString:
		return resolveInlineString(spec.InlineString)
	case *corev3.DataSource_EnvironmentVariable:
		return resolveEnvironmentVariable(spec.EnvironmentVariable)
	default:
		// Unreachable per pre-conditions (arm 8 enforces Specifier != nil
		// + the 4-arm oneof is closed at the proto level). Return the
		// arm-8 byte-stable wording as a defensive fallthrough.
		return nil, errors.New(parseRejectDataSourceSpecifierRequired)
	}
}

// -----------------------------------------------------------------------------
// Per-arm resolvers — each handles its arm's content-empty failure paths.
// -----------------------------------------------------------------------------

// resolveFilename handles the Filename arm: empty-name → name-empty reject;
// ENOENT → not-found reject; other errors → unreadable %w-wrap; zero-byte
// file → empty-content reject; non-empty content → byte slice returned.
func resolveFilename(name string) ([]byte, error) {
	if name == "" {
		return nil, errors.New(parseRejectFilenameEmpty)
	}
	bytes, err := os.ReadFile(name) //nolint:gosec // operator-supplied wasm bytecode path; boot-time fail-fast on any read error.
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf(parseRejectFilenameNotFound, name)
		}
		// EACCES, EISDIR, EIO, etc. surface via the wrap-with-inner-err
		// path so callers can drill in via errors.Is/errors.As.
		return nil, fmt.Errorf(parseRejectFilenameUnreadable, name, err)
	}
	if len(bytes) == 0 {
		return nil, fmt.Errorf(parseRejectFilenameEmptyContent, name)
	}
	return bytes, nil
}

// resolveInlineBytes handles the InlineBytes arm: nil/zero-length → empty
// reject; non-empty → verbatim byte slice returned.
func resolveInlineBytes(bytes []byte) ([]byte, error) {
	if len(bytes) == 0 {
		return nil, errors.New(parseRejectInlineBytesEmpty)
	}
	return bytes, nil
}

// resolveInlineString handles the InlineString arm: empty → empty reject;
// non-empty → string→[]byte cast.
func resolveInlineString(s string) ([]byte, error) {
	if s == "" {
		return nil, errors.New(parseRejectInlineStringEmpty)
	}
	return []byte(s), nil
}

// resolveEnvironmentVariable handles the EnvironmentVariable arm: empty
// name → name-empty reject; unset env var → unset reject; set but empty
// value → empty-value reject; non-empty value → string→[]byte cast.
func resolveEnvironmentVariable(name string) ([]byte, error) {
	if name == "" {
		return nil, errors.New(parseRejectEnvVarNameEmpty)
	}
	val, ok := os.LookupEnv(name)
	if !ok {
		return nil, fmt.Errorf(parseRejectEnvVarUnset, name)
	}
	if val == "" {
		return nil, fmt.Errorf(parseRejectEnvVarEmptyValue, name)
	}
	return []byte(val), nil
}
