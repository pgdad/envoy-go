// Custom 8-stub WASI implementation per R4 + parent §13-R4.
//
// envoy-go intentionally does NOT use wazero's built-in
// `imports/wasi_snapshot_preview1` package: that package routes `fd_write`
// to the host process's stdout / stderr file descriptors, but proxy-wasm
// semantics require `fd_write` (fd=1 / fd=2) to fan in via `proxy_log` at
// the host integration. The 8 shims below replace the WASI surface entirely
// and route via a minimal `wasiHost` facade.
//
// Capability-restriction posture: each shim is gated by a sandbox capability
// key (the bare name `fd_write`, `clock_time_get`, ...). When the host's
// `IsAllowed(<key>)` returns false the shim returns `abi.WasiErrnoNotcapable`
// (=76) per D-P1 closure at Task 2 commit 511b8326. The errno value is
// byte-faithful to upstream `proxy-wasm-cpp-host@da3ce05d:include/proxy-wasm/exports.h:239`
// (`return 76; /* __WASI_ENOTCAPABLE */`).
//
// Defined in this file:
//   - `wasiHost` interface — minimal facade used by every shim; satisfied at
//     Task 7 by `*VM` composing `*SandboxConfig` (Task 6) + the configured
//     log sink. Lives in this file so Task 4 can land independently.
//   - `cap*` constants — the 8 capability keys; referenced by Task 6
//     sandbox.go for the per-key allow-set and by Task 7 registration.go
//     when wiring `wasi_snapshot_preview1` host functions to the shims.
//   - 8 shim functions: `wasiFdWrite`, `wasiClockTimeGet`, `wasiRandomGet`,
//     `wasiEnvironSizesGet`, `wasiEnvironGet`, `wasiArgsSizesGet`,
//     `wasiArgsGet`, `wasiProcExit`.
//
// Out-of-bounds memory access (an `api.Memory.Read` / `.Write` returning
// false) is reported as `abi.WasiErrnoInval` (=28). The roster fixed at
// Task 1 (AMEND-A7) provides Success/Badf/Inval/Notsup/Notcapable — Inval is
// the closest-fit existing value for an out-of-bounds memory access; adding
// WasiErrnoFault (=21 upstream) would be a roster extension outside Task 4 scope.

package wasm

import (
	"context"
	cryptorand "crypto/rand"
	"strings"
	"time"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// wasiHost is the minimal interface the WASI shims need from the calling VM.
// Implemented by *VM at Task 7 (which composes *SandboxConfig from Task 6
// + the WithLogSink-configured log sink). Defined here so Task 4 can land
// without forward-depending on the Task 6 / Task 7 surfaces.
type wasiHost interface {
	// IsAllowed reports whether the named capability is permitted by the
	// sandbox. Per AMEND-A5 the zero-value (empty-set) SandboxConfig denies
	// all capabilities; explicit opt-in is required.
	IsAllowed(capabilityName string) bool

	// LogProxy routes a log message at the named level. Used by `fd_write`
	// to forward fd=1 / fd=2 stream-writes through the proxy-wasm
	// integration logger (NOT the host process's stdout / stderr).
	LogProxy(level abi.LogLevel, msg string)
}

// Capability keys for the 8 WASI shims. Bare names (no `wasi_` prefix —
// the per-group `wasi_snapshot_preview1_` prefix is applied at the wazero
// host-module registration boundary at Task 7 per AMEND-A2). Byte-stability
// is asserted by `TestWasiCapabilityKeys` so Task 6's sandbox roster and
// Task 7's registration table can rely on these constants without copying
// the strings.
const (
	capWasiFdWrite         = "fd_write"
	capWasiClockTimeGet    = "clock_time_get"
	capWasiRandomGet       = "random_get"
	capWasiEnvironSizesGet = "environ_sizes_get"
	capWasiEnvironGet      = "environ_get"
	capWasiArgsSizesGet    = "args_sizes_get"
	capWasiArgsGet         = "args_get"
	capWasiProcExit        = "proc_exit"
)

// WASI clock_id sentinel values per WASI snapshot-preview1.
const (
	wasiClockRealtime  uint32 = 0 // wall clock — `time.Now().UnixNano()`
	wasiClockMonotonic uint32 = 1 // monotonic — `time.Now().UnixNano()` (host-time accuracy per SPEC)
)

// wasiFdWrite implements the `wasi_snapshot_preview1.fd_write` shim. Routes
// fd=1 / fd=2 stream-writes through `host.LogProxy` rather than the host
// stdout / stderr.
//
// Wire shape: each iovec is 8 bytes — u32 `buf_ptr` + u32 `buf_len`.
// Concatenates all iovec buffers into a single UTF-8 best-effort message
// and routes via `host.LogProxy(level, msg)` where:
//
//	fd == 1 → abi.LogLevelInfo
//	fd == 2 → abi.LogLevelError
//	else    → return abi.WasiErrnoBadf without writing nwritten or logging
//
// On success writes the total byte-count (sum of all `buf_len` values) to
// `nwrittenPtr` as u32 little-endian + returns `abi.WasiErrnoSuccess`.
// Sandbox-denied (`!host.IsAllowed(capWasiFdWrite)`) → returns
// `abi.WasiErrnoNotcapable` with no memory write + no log invocation.
func wasiFdWrite(_ context.Context, mod api.Module, host wasiHost, fd, iovsPtr, iovsLen, nwrittenPtr uint32) abi.WasiErrno {
	if !host.IsAllowed(capWasiFdWrite) {
		return abi.WasiErrnoNotcapable
	}

	// Bad-fd path: gate BEFORE reading iovecs so the caller observes BADF
	// even if the iovec pointer is bogus. Matches upstream proxy-wasm
	// behavior of treating non-{1,2} fds as not-our-business.
	var level abi.LogLevel
	switch fd {
	case 1:
		level = abi.LogLevelInfo
	case 2:
		level = abi.LogLevelError
	default:
		return abi.WasiErrnoBadf
	}

	mem := mod.Memory()

	// Walk the iovec array. Each entry is 8 bytes: u32 buf_ptr + u32 buf_len.
	// Accumulate buffers into a single strings.Builder to produce the final
	// log message in a single allocation.
	var msg strings.Builder
	var nwritten uint32
	for i := uint32(0); i < iovsLen; i++ {
		entryOffset := iovsPtr + i*8
		bufPtr, ok := mem.ReadUint32Le(entryOffset)
		if !ok {
			return abi.WasiErrnoInval
		}
		bufLen, ok := mem.ReadUint32Le(entryOffset + 4)
		if !ok {
			return abi.WasiErrnoInval
		}
		if bufLen == 0 {
			continue
		}
		buf, ok := mem.Read(bufPtr, bufLen)
		if !ok {
			return abi.WasiErrnoInval
		}
		msg.Write(buf)
		nwritten += bufLen
	}

	host.LogProxy(level, msg.String())

	if !mem.WriteUint32Le(nwrittenPtr, nwritten) {
		return abi.WasiErrnoInval
	}
	return abi.WasiErrnoSuccess
}

// wasiClockTimeGet implements `wasi_snapshot_preview1.clock_time_get`.
//
// Supports two clock IDs: CLOCK_REALTIME=0 (wall via `time.Now().UnixNano()`)
// and CLOCK_MONOTONIC=1 (Go's `time.Now()` carries a monotonic reading by
// default; UnixNano() returns an absolute nanosecond count which is
// monotonically increasing within a process — acceptable for proxy-wasm
// guests at 25.1 per SPEC's "host-time accuracy; no fake-time seam"
// disposition). Other clock IDs return `abi.WasiErrnoInval`.
//
// The `precision` argument is accepted and IGNORED — the host clock's
// native precision is used regardless.
//
// Writes the u64 little-endian nanosecond count to `timePtr` on success.
func wasiClockTimeGet(_ context.Context, mod api.Module, host wasiHost, clockID uint32, _ uint64, timePtr uint32) abi.WasiErrno {
	if !host.IsAllowed(capWasiClockTimeGet) {
		return abi.WasiErrnoNotcapable
	}

	switch clockID {
	case wasiClockRealtime, wasiClockMonotonic:
		// fall through
	default:
		return abi.WasiErrnoInval
	}

	now := uint64(time.Now().UnixNano())
	if !mod.Memory().WriteUint64Le(timePtr, now) {
		return abi.WasiErrnoInval
	}
	return abi.WasiErrnoSuccess
}

// wasiRandomGet implements `wasi_snapshot_preview1.random_get`. Fills
// `[bufferPtr, bufferPtr+bufferSize)` with cryptographically secure random
// bytes via `crypto/rand.Read`. On a successful single read writes the bytes
// into the guest memory + returns `abi.WasiErrnoSuccess`. A short read from
// `crypto/rand` (extremely unlikely on a healthy system) maps to
// `abi.WasiErrnoInval` — no `WasiErrnoIo` exists in the 25.1 roster + Inval
// is the closest-fit existing value.
func wasiRandomGet(_ context.Context, mod api.Module, host wasiHost, bufferPtr, bufferSize uint32) abi.WasiErrno {
	if !host.IsAllowed(capWasiRandomGet) {
		return abi.WasiErrnoNotcapable
	}
	if bufferSize == 0 {
		return abi.WasiErrnoSuccess
	}

	buf := make([]byte, bufferSize)
	if _, err := cryptorand.Read(buf); err != nil {
		return abi.WasiErrnoInval
	}
	if !mod.Memory().Write(bufferPtr, buf) {
		return abi.WasiErrnoInval
	}
	return abi.WasiErrnoSuccess
}

// wasiEnvironSizesGet implements `wasi_snapshot_preview1.environ_sizes_get`.
// At 25.1 + 25.2 the proxy-wasm host advertises zero environment variables;
// the 25.3 sub-phase will populate from `VmConfig.environment_variables`.
// Writes 0 (u32 LE) to both `numElementsPtr` and `bufferSizePtr`.
func wasiEnvironSizesGet(_ context.Context, mod api.Module, host wasiHost, numElementsPtr, bufferSizePtr uint32) abi.WasiErrno {
	if !host.IsAllowed(capWasiEnvironSizesGet) {
		return abi.WasiErrnoNotcapable
	}
	mem := mod.Memory()
	if !mem.WriteUint32Le(numElementsPtr, 0) {
		return abi.WasiErrnoInval
	}
	if !mem.WriteUint32Le(bufferSizePtr, 0) {
		return abi.WasiErrnoInval
	}
	return abi.WasiErrnoSuccess
}

// wasiEnvironGet implements `wasi_snapshot_preview1.environ_get`. Pairs
// with `wasiEnvironSizesGet` (which advertises 0 environ entries at 25.1);
// writes nothing and returns `abi.WasiErrnoSuccess` on allow.
func wasiEnvironGet(_ context.Context, _ api.Module, host wasiHost, _, _ uint32) abi.WasiErrno {
	if !host.IsAllowed(capWasiEnvironGet) {
		return abi.WasiErrnoNotcapable
	}
	return abi.WasiErrnoSuccess
}

// wasiArgsSizesGet implements `wasi_snapshot_preview1.args_sizes_get`. The
// proxy-wasm host NEVER passes command-line args to guest modules — writes
// 0/0 to both `argcPtr` and `argvBufSizePtr`.
func wasiArgsSizesGet(_ context.Context, mod api.Module, host wasiHost, argcPtr, argvBufSizePtr uint32) abi.WasiErrno {
	if !host.IsAllowed(capWasiArgsSizesGet) {
		return abi.WasiErrnoNotcapable
	}
	mem := mod.Memory()
	if !mem.WriteUint32Le(argcPtr, 0) {
		return abi.WasiErrnoInval
	}
	if !mem.WriteUint32Le(argvBufSizePtr, 0) {
		return abi.WasiErrnoInval
	}
	return abi.WasiErrnoSuccess
}

// wasiArgsGet implements `wasi_snapshot_preview1.args_get`. Pairs with
// `wasiArgsSizesGet` (0 args advertised); writes nothing and returns
// `abi.WasiErrnoSuccess` on allow.
func wasiArgsGet(_ context.Context, _ api.Module, host wasiHost, _, _ uint32) abi.WasiErrno {
	if !host.IsAllowed(capWasiArgsGet) {
		return abi.WasiErrnoNotcapable
	}
	return abi.WasiErrnoSuccess
}

// wasiProcExit implements `wasi_snapshot_preview1.proc_exit`. Well-behaved
// proxy-wasm guests MUST NOT call proc_exit, but if invoked the shim must
// terminate the VM cleanly. Returns a `*sys.ExitError` (from
// `github.com/tetratelabs/wazero/sys`) which wazero treats as a trap;
// the per-callback caller at Task 7 propagates the error as a VM-aborted
// signal.
//
// Sandbox-denied: still returns an `*sys.ExitError` (proc_exit by definition
// never returns to the guest, even on capability-deny) but conveys
// `abi.WasiErrnoNotcapable` as the exit code so the caller can distinguish
// a capability-deny from a guest-initiated exit. This is byte-stable for
// downstream test assertions.
func wasiProcExit(_ context.Context, _ api.Module, host wasiHost, exitCode uint32) error {
	if !host.IsAllowed(capWasiProcExit) {
		return sys.NewExitError(uint32(abi.WasiErrnoNotcapable))
	}
	return sys.NewExitError(exitCode)
}
