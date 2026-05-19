package lua

// misc.go — Task 12 (phase 22.2 IMPL) miscellaneous bridge methods per
// SPEC §3.4 + AMEND-22.2-1 + AMEND-22.2-2 + D8 closure + PLAN Task 12 +
// ADR-0192 §Decision body anticipation.
//
// # Surface (2 methods on each of request_handle + response_handle)
//
//   - :fileBytes(path) — envoy-go-strict per D8 (NOT in upstream
//                        StreamHandleWrapper; absent at all upstream
//                        scopes per Pin 2 + R8 scrape). Reads the file
//                        at `path` and returns its contents as a Lua
//                        string. Subject to a 16 MiB cap (matches the
//                        22.1 Task 11 cap pattern at datasource.go via
//                        the shared maxFilenameScriptBytes constant) —
//                        files exceeding the cap raise a Lua runtime
//                        error with byte-stable wording. ENOENT / EACCES
//                        + other open/read errors return (nil, err_string)
//                        per the upstream Lua idiom.
//
//   - :timestamp(unit?) — non-deterministic wall-clock from time.Now()
//                         per BRAINSTORM §2.7. Default unit is
//                         "milliseconds". Supported units:
//                         "seconds" / "milliseconds" / "microseconds".
//                         Invalid unit raises a Lua runtime error with
//                         byte-stable wording (per PLAN Task 12).
//
// # Cap discipline (16 MiB; matches 22.1 Task 11 maxFilenameScriptBytes)
//
// Per SPEC §11.2.5 + AMEND-22.2-2: the 16 MiB cap mirrors the cap
// already in force for data-source-loaded scripts (datasource.go:117).
// `io.LimitReader(f, maxFilenameScriptBytes+1)` reads at most cap+1
// bytes — if `len(read) > cap` then the file exceeds the cap. The cap
// is anti-DoS, not a feature toggle.
//
// # Classification per D8 PLAN closure
//
//   - :fileBytes  — envoy-go-strict (NEW BEHAVIOR_CONTRACT.md departure
//                   record anticipated at Task 19 atomic landing).
//   - :timestamp  — envoy-go-strict (NOT in upstream StreamHandleWrapper;
//                   anticipated departure record at Task 19).
//
// # Cross-references
//
//   - SPEC §3.4 (misc surface)
//   - SPEC §11.2.5 (fileBytes + cap discipline)
//   - PLAN Task 12 (subagent dispatch outline)
//   - 22.1 datasource.go (16 MiB cap constant reused via
//     maxFilenameScriptBytes)

import (
	"fmt"
	"io"
	"os"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// miscFileBytesOverCapPrefix is the byte-stable prefix raised when
// :fileBytes(path) reads a file exceeding the 16 MiB cap. Pinned via a
// package-level const so a regression on the byte-stable contract
// surfaces at the Test_FileBytes_over_cap_raises_runtime_reject
// assertion. The full template is
// `lua: fileBytes: file size exceeds maximum of <N> bytes`.
const miscFileBytesOverCapPrefix = "lua: fileBytes: file size exceeds maximum of"

// miscTimestampInvalidUnitPrefix is the byte-stable prefix raised when
// :timestamp(unit) is called with an unsupported unit. The full template
// is `lua: timestamp: unsupported unit <unit>`.
const miscTimestampInvalidUnitPrefix = "lua: timestamp: unsupported unit"

// timestampUnit* names match upstream Envoy Lua filter's timestamp()
// argument conventions (per public-facing Envoy Lua API docs).
const (
	timestampUnitMilliseconds = "milliseconds"
	timestampUnitMicroseconds = "microseconds"
	timestampUnitSeconds      = "seconds"
)

// ---------------------------------------------------------------------
// :fileBytes — envoy-go-strict per D8
// ---------------------------------------------------------------------

// requestHandleFileBytes implements request_handle:fileBytes(path) per
// SPEC §3.4 + D8 (envoy-go-strict). Reads the file at `path` subject to
// the 16 MiB cap (maxFilenameScriptBytes — shared with the 22.1 Task 11
// data-source loader at datasource.go).
//
// Disposition table:
//
//   - happy path:   returns the file contents as a Lua string.
//   - over-cap:     raises a Lua runtime error with the byte-stable
//     wording prefix `lua: fileBytes: file size exceeds
//     maximum of <N> bytes`.
//   - ENOENT/etc:   returns (nil, err_string) per upstream Lua idiom;
//     scripts may check `local b, err = rh:fileBytes(p);
//     if b == nil then ... end`.
//
// arbitrary paths allowed (envoy-go-strict per D8 — no chroot, no
// allow-list; this method is NOT in upstream at all). Operators are
// expected to restrict at the configuration layer (sandbox via OS-level
// confinement of the envoy-go process).
func requestHandleFileBytes(L *lua.LState) int {
	_ = L.CheckUserData(1)
	path := L.CheckString(2)
	body, err := readFileBytesCapped(path)
	if err != nil {
		// Over-cap is a runtime-reject; surface as a Lua runtime error
		// (script-stopping). Other errors (ENOENT, EACCES, EISDIR, etc.)
		// are reported via the Lua idiom (nil, err_string) so scripts can
		// recover.
		if isOverCapError(err) {
			L.RaiseError("%s", err.Error())
			return 0
		}
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(body))
	return 1
}

// fileBytesOverCapError is the sentinel error type signaling that a
// :fileBytes(path) read exceeded the 16 MiB cap. Distinguished from
// open/read errors so the LGFunction can dispatch to the runtime-reject
// path (raise) vs. the soft-error path (nil + err string).
type fileBytesOverCapError struct {
	msg string
}

func (e *fileBytesOverCapError) Error() string { return e.msg }

// isOverCapError reports whether err is a fileBytesOverCapError.
func isOverCapError(err error) bool {
	_, ok := err.(*fileBytesOverCapError)
	return ok
}

// readFileBytesCapped opens `path` + reads at most cap+1 bytes via
// io.LimitReader. Returns the bytes on success. Returns a
// *fileBytesOverCapError if the file size exceeds the cap. Returns a
// generic error for ENOENT / EACCES / etc.
//
// Matches the 22.1 datasource.go cap discipline (16 MiB via the shared
// maxFilenameScriptBytes constant).
func readFileBytesCapped(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	body, readErr := io.ReadAll(io.LimitReader(f, maxFilenameScriptBytes+1))
	if readErr != nil {
		return nil, readErr
	}
	if len(body) > maxFilenameScriptBytes {
		return nil, &fileBytesOverCapError{
			msg: fmt.Sprintf("%s %d bytes", miscFileBytesOverCapPrefix, maxFilenameScriptBytes),
		}
	}
	return body, nil
}

// ---------------------------------------------------------------------
// :timestamp(unit?) — non-deterministic wall-clock per BRAINSTORM §2.7
// ---------------------------------------------------------------------

// requestHandleTimestamp implements request_handle:timestamp(unit?) per
// SPEC §3.4 + BRAINSTORM §2.7. Reads time.Now() + converts to the
// requested unit. Default unit is "milliseconds". Supported units:
// "seconds" / "milliseconds" / "microseconds". Invalid unit raises a
// Lua runtime error with the byte-stable wording prefix per PLAN
// Task 12.
//
// Per SPEC §11.2 + ADR-0186: this consumer does NOT use the Clock seam
// — wall-clock time.Now() is fine here because the surface is
// EXPLICITLY non-deterministic (script authors who want testable time
// use other Lua patterns or framework-injected fixtures).
func requestHandleTimestamp(L *lua.LState) int {
	_ = L.CheckUserData(1)
	unit := L.OptString(2, timestampUnitMilliseconds)
	now := time.Now()
	switch unit {
	case timestampUnitMilliseconds:
		L.Push(lua.LNumber(now.UnixMilli()))
		return 1
	case timestampUnitMicroseconds:
		L.Push(lua.LNumber(now.UnixMicro()))
		return 1
	case timestampUnitSeconds:
		L.Push(lua.LNumber(now.Unix()))
		return 1
	}
	L.RaiseError("%s %q", miscTimestampInvalidUnitPrefix, unit)
	return 0
}

// ---------------------------------------------------------------------
// Response-handle parity stubs — symmetric to request-handle for the
// encode side. Scripts writing envoy_on_response may equivalently call
// :fileBytes / :timestamp on the response_handle. Same stateless
// dispatch as the crypto methods (the receiver type-check at index 1
// is only a sanity-check).
// ---------------------------------------------------------------------

func responseHandleFileBytes(L *lua.LState) int { return requestHandleFileBytes(L) }
func responseHandleTimestamp(L *lua.LState) int { return requestHandleTimestamp(L) }
