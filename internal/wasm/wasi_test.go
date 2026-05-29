// Tests for the custom 8-stub WASI implementation per R4 + parent §13-R4.
//
// Each shim function carries (1) a happy-path test, (2) a sandbox-deny test
// (host.IsAllowed returns false → WasiErrnoNotcapable=76 per D-P1 closure at
// Task 2 commit 511b8326), and (3) bad-input tests where the shim's semantics
// define a bad-input arm (fd_write: fd=99 → BADF=8; clock_time_get: clockID=2
// → INVAL=28). Memory writes are validated by reading back from the same
// wazero api.Memory the shim wrote into.
//
// Tests must FAIL before wasi.go lands per D-P-PLAN-4.
//
// The mock wasiHost is configurable per-test: zero-value `allowed` map ⇒
// deny-all (matches AMEND-A5 default-deny posture); set keys to true to allow.
// `logs` accumulates LogProxy invocations for fd_write log-route assertions.

package wasm

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/sys"

	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// --- test helpers --------------------------------------------------------

// logEntry records a single LogProxy invocation for assertion.
type logEntry struct {
	Level abi.LogLevel
	Msg   string
}

// mockWasiHost satisfies the wasiHost interface for shim testing.
// `allowed` is the capability allow-set; missing/false keys ⇒ DENY (per
// AMEND-A5 default-deny posture). `logs` accumulates LogProxy invocations.
type mockWasiHost struct {
	allowed map[string]bool
	logs    []logEntry
}

func (m *mockWasiHost) IsAllowed(cap string) bool { return m.allowed[cap] }
func (m *mockWasiHost) LogProxy(lvl abi.LogLevel, msg string) {
	m.logs = append(m.logs, logEntry{Level: lvl, Msg: msg})
}

// WASIEnviron returns nil (zero env entries) for the base mock. Tests that
// need non-empty environ use mockWasiHostWithEnv (env_vars_test.go).
func (m *mockWasiHost) WASIEnviron() [][]byte { return nil }

// allowAll returns a mockWasiHost with every WASI capability allowed.
func allowAll() *mockWasiHost {
	return &mockWasiHost{
		allowed: map[string]bool{
			capWasiFdWrite:         true,
			capWasiClockTimeGet:    true,
			capWasiRandomGet:       true,
			capWasiEnvironSizesGet: true,
			capWasiEnvironGet:      true,
			capWasiArgsSizesGet:    true,
			capWasiArgsGet:         true,
			capWasiProcExit:        true,
		},
	}
}

// denyAll returns a mockWasiHost with the empty allow-map (= deny everything
// per AMEND-A5).
func denyAll() *mockWasiHost {
	return &mockWasiHost{allowed: map[string]bool{}}
}

// minimalWasmMemoryModule is a hand-crafted minimal wasm binary that exports
// a single memory of 1 page (64 KiB) under the name "memory". This is the
// smallest module that lets the WASI shims exercise `mod.Memory().Read/Write`.
//
// Section layout (per the WebAssembly Core 1.0 binary format):
//
//	0x00 0x61 0x73 0x6d           — wasm magic
//	0x01 0x00 0x00 0x00           — version 1
//	0x05 0x03 0x01 0x00 0x01      — memory section (id=5, size=3, count=1, limits-flag=0 min=1)
//	0x07 0x0a 0x01                — export section (id=7, size=10, count=1)
//	0x06 'm' 'e' 'm' 'o' 'r' 'y'  — name-len=6 + "memory" (7 bytes)
//	0x02 0x00                     — export-kind=memory(2) + index=0 (2 bytes)
//
// Export-section payload after the size byte = count(1) + name-len(1) +
// name(6) + kind(1) + index(1) = 10 bytes total.
var minimalWasmMemoryModule = []byte{
	0x00, 0x61, 0x73, 0x6d, // magic
	0x01, 0x00, 0x00, 0x00, // version 1
	// memory section
	0x05, 0x03, 0x01, 0x00, 0x01,
	// export section
	0x07, 0x0a, 0x01,
	0x06, 'm', 'e', 'm', 'o', 'r', 'y',
	0x02, 0x00,
}

// newTestModule instantiates the minimalWasmMemoryModule via a real wazero
// runtime and returns the api.Module + a cleanup function. Test bodies use
// `mod.Memory()` for the shim memory-read/write assertions.
func newTestModule(t *testing.T) (api.Module, func()) {
	t.Helper()
	ctx := context.Background()
	r := wazero.NewRuntime(ctx)
	mod, err := r.InstantiateWithConfig(ctx, minimalWasmMemoryModule, wazero.NewModuleConfig().WithName(""))
	if err != nil {
		_ = r.Close(ctx)
		t.Fatalf("instantiate minimal memory module: %v", err)
	}
	return mod, func() {
		_ = mod.Close(ctx)
		_ = r.Close(ctx)
	}
}

// readU32 returns the u32 little-endian value at offset; fails the test on OOB.
func readU32(t *testing.T, mem api.Memory, offset uint32) uint32 {
	t.Helper()
	v, ok := mem.ReadUint32Le(offset)
	if !ok {
		t.Fatalf("ReadUint32Le(%d) out of bounds", offset)
	}
	return v
}

// readU64 returns the u64 little-endian value at offset; fails the test on OOB.
func readU64(t *testing.T, mem api.Memory, offset uint32) uint64 {
	t.Helper()
	v, ok := mem.ReadUint64Le(offset)
	if !ok {
		t.Fatalf("ReadUint64Le(%d) out of bounds", offset)
	}
	return v
}

// readBytes returns byteCount bytes at offset; fails the test on OOB.
func readBytes(t *testing.T, mem api.Memory, offset, byteCount uint32) []byte {
	t.Helper()
	b, ok := mem.Read(offset, byteCount)
	if !ok {
		t.Fatalf("Read(%d, %d) out of bounds", offset, byteCount)
	}
	// Defensive copy: wazero returns a view that aliases the underlying buffer.
	out := make([]byte, len(b))
	copy(out, b)
	return out
}

// writeBytes writes bytes at offset; fails the test on OOB.
func writeBytes(t *testing.T, mem api.Memory, offset uint32, b []byte) {
	t.Helper()
	if !mem.Write(offset, b) {
		t.Fatalf("Write(%d, %d-bytes) out of bounds", offset, len(b))
	}
}

// writeU32 writes a u32 LE at offset; fails the test on OOB.
func writeU32(t *testing.T, mem api.Memory, offset, v uint32) {
	t.Helper()
	if !mem.WriteUint32Le(offset, v) {
		t.Fatalf("WriteUint32Le(%d, %d) out of bounds", offset, v)
	}
}

// --- TestWasiFdWrite -----------------------------------------------------

func TestWasiFdWrite(t *testing.T) {
	ctx := context.Background()

	t.Run("fd=1 routes to INFO log + writes nwritten", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		// Layout in guest memory:
		//   iovec[0] @ 0x100 = (buf_ptr=0x200, buf_len=5)
		//   iovec[1] @ 0x108 = (buf_ptr=0x210, buf_len=6)
		//   buf @ 0x200 = "hello"
		//   buf @ 0x210 = " world"
		//   nwritten @ 0x300
		writeU32(t, mod.Memory(), 0x100, 0x200)
		writeU32(t, mod.Memory(), 0x104, 5)
		writeU32(t, mod.Memory(), 0x108, 0x210)
		writeU32(t, mod.Memory(), 0x10C, 6)
		writeBytes(t, mod.Memory(), 0x200, []byte("hello"))
		writeBytes(t, mod.Memory(), 0x210, []byte(" world"))

		errno := wasiFdWrite(ctx, mod, host, 1, 0x100, 2, 0x300)
		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("errno = %d, want %d (success)", errno, abi.WasiErrnoSuccess)
		}
		if got := readU32(t, mod.Memory(), 0x300); got != 11 {
			t.Errorf("nwritten = %d, want 11", got)
		}
		if len(host.logs) != 1 {
			t.Fatalf("logs len = %d, want 1; logs = %+v", len(host.logs), host.logs)
		}
		if host.logs[0].Level != abi.LogLevelInfo {
			t.Errorf("log level = %d, want %d (Info)", host.logs[0].Level, abi.LogLevelInfo)
		}
		if host.logs[0].Msg != "hello world" {
			t.Errorf("log msg = %q, want %q", host.logs[0].Msg, "hello world")
		}
	})

	t.Run("fd=2 routes to ERROR log", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		writeU32(t, mod.Memory(), 0x100, 0x200)
		writeU32(t, mod.Memory(), 0x104, 3)
		writeBytes(t, mod.Memory(), 0x200, []byte("err"))

		errno := wasiFdWrite(ctx, mod, host, 2, 0x100, 1, 0x300)
		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("errno = %d, want %d", errno, abi.WasiErrnoSuccess)
		}
		if got := readU32(t, mod.Memory(), 0x300); got != 3 {
			t.Errorf("nwritten = %d, want 3", got)
		}
		if len(host.logs) != 1 || host.logs[0].Level != abi.LogLevelError || host.logs[0].Msg != "err" {
			t.Errorf("logs = %+v, want [{Error,\"err\"}]", host.logs)
		}
	})

	t.Run("fd=99 returns BADF + no log + no nwritten", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		writeU32(t, mod.Memory(), 0x100, 0x200)
		writeU32(t, mod.Memory(), 0x104, 3)
		writeBytes(t, mod.Memory(), 0x200, []byte("abc"))
		writeU32(t, mod.Memory(), 0x300, 0xDEADBEEF) // sentinel

		errno := wasiFdWrite(ctx, mod, host, 99, 0x100, 1, 0x300)
		if errno != abi.WasiErrnoBadf {
			t.Errorf("errno = %d, want %d (Badf)", errno, abi.WasiErrnoBadf)
		}
		if got := readU32(t, mod.Memory(), 0x300); got != 0xDEADBEEF {
			t.Errorf("nwritten clobbered: got %#x, want sentinel %#x", got, uint32(0xDEADBEEF))
		}
		if len(host.logs) != 0 {
			t.Errorf("logs = %+v, want empty (BADF should not log)", host.logs)
		}
	})

	t.Run("sandbox deny → Notcapable + no log + no nwritten", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := denyAll()

		writeU32(t, mod.Memory(), 0x300, 0xCAFEBABE) // sentinel

		errno := wasiFdWrite(ctx, mod, host, 1, 0x100, 0, 0x300)
		if errno != abi.WasiErrnoNotcapable {
			t.Errorf("errno = %d, want %d (Notcapable)", errno, abi.WasiErrnoNotcapable)
		}
		if got := readU32(t, mod.Memory(), 0x300); got != 0xCAFEBABE {
			t.Errorf("nwritten clobbered on deny: got %#x, want sentinel %#x", got, uint32(0xCAFEBABE))
		}
		if len(host.logs) != 0 {
			t.Errorf("logs = %+v, want empty on deny", host.logs)
		}
	})

	t.Run("zero iovecs → success + nwritten=0 + empty-string log", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		errno := wasiFdWrite(ctx, mod, host, 1, 0x100, 0, 0x300)
		if errno != abi.WasiErrnoSuccess {
			t.Errorf("errno = %d, want success", errno)
		}
		if got := readU32(t, mod.Memory(), 0x300); got != 0 {
			t.Errorf("nwritten = %d, want 0", got)
		}
	})
}

// --- TestWasiClockTimeGet ------------------------------------------------

func TestWasiClockTimeGet(t *testing.T) {
	ctx := context.Background()

	t.Run("CLOCK_REALTIME=0 writes wall time", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		before := time.Now().UnixNano()
		errno := wasiClockTimeGet(ctx, mod, host, 0, 0, 0x400)
		after := time.Now().UnixNano()

		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("errno = %d, want success", errno)
		}
		got := readU64(t, mod.Memory(), 0x400)
		if int64(got) < before || int64(got) > after {
			t.Errorf("clock value %d outside [%d, %d]", got, before, after)
		}
	})

	t.Run("CLOCK_MONOTONIC=1 writes monotonic time + monotonically advances", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		errno := wasiClockTimeGet(ctx, mod, host, 1, 0, 0x400)
		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("first errno = %d, want success", errno)
		}
		first := readU64(t, mod.Memory(), 0x400)

		// A tiny sleep + second sample guarantees the monotonic clock advanced.
		time.Sleep(2 * time.Millisecond)
		errno = wasiClockTimeGet(ctx, mod, host, 1, 0, 0x408)
		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("second errno = %d, want success", errno)
		}
		second := readU64(t, mod.Memory(), 0x408)

		if second <= first {
			t.Errorf("monotonic clock did not advance: first=%d second=%d", first, second)
		}
	})

	t.Run("unsupported clock_id=2 returns INVAL + no write", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		// Sentinel u64 at 0x400 = 0xDEADBEEFCAFEBABE
		mod.Memory().WriteUint64Le(0x400, 0xDEADBEEFCAFEBABE)

		errno := wasiClockTimeGet(ctx, mod, host, 2, 0, 0x400)
		if errno != abi.WasiErrnoInval {
			t.Errorf("errno = %d, want %d (Inval)", errno, abi.WasiErrnoInval)
		}
		if got := readU64(t, mod.Memory(), 0x400); got != 0xDEADBEEFCAFEBABE {
			t.Errorf("time clobbered on INVAL: got %#x, want sentinel", got)
		}
	})

	t.Run("sandbox deny → Notcapable + no write", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := denyAll()

		mod.Memory().WriteUint64Le(0x400, 0xCAFEBABEDEADBEEF)

		errno := wasiClockTimeGet(ctx, mod, host, 0, 0, 0x400)
		if errno != abi.WasiErrnoNotcapable {
			t.Errorf("errno = %d, want %d (Notcapable)", errno, abi.WasiErrnoNotcapable)
		}
		if got := readU64(t, mod.Memory(), 0x400); got != 0xCAFEBABEDEADBEEF {
			t.Errorf("time clobbered on deny: got %#x", got)
		}
	})
}

// --- TestWasiRandomGet ---------------------------------------------------

func TestWasiRandomGet(t *testing.T) {
	ctx := context.Background()

	t.Run("fills buffer with random bytes (non-zero, varying)", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		// Pre-fill with sentinel so we can verify rand-write happened.
		sentinel := make([]byte, 32)
		for i := range sentinel {
			sentinel[i] = 0xAA
		}
		writeBytes(t, mod.Memory(), 0x500, sentinel)

		errno := wasiRandomGet(ctx, mod, host, 0x500, 32)
		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("errno = %d, want success", errno)
		}
		got := readBytes(t, mod.Memory(), 0x500, 32)
		// Crypto-rand: probability of equaling sentinel is ~2^-256; reject.
		allAA := true
		for _, b := range got {
			if b != 0xAA {
				allAA = false
				break
			}
		}
		if allAA {
			t.Error("random buffer equals sentinel; crypto/rand did not write")
		}
	})

	t.Run("zero-sized buffer returns success with no write", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		// Sentinel byte at 0x500.
		writeBytes(t, mod.Memory(), 0x500, []byte{0xAA})

		errno := wasiRandomGet(ctx, mod, host, 0x500, 0)
		if errno != abi.WasiErrnoSuccess {
			t.Errorf("errno = %d, want success", errno)
		}
		got := readBytes(t, mod.Memory(), 0x500, 1)
		if got[0] != 0xAA {
			t.Errorf("sentinel clobbered: got %#x, want 0xAA", got[0])
		}
	})

	t.Run("sandbox deny → Notcapable + no write", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := denyAll()

		sentinel := make([]byte, 8)
		for i := range sentinel {
			sentinel[i] = 0xBB
		}
		writeBytes(t, mod.Memory(), 0x500, sentinel)

		errno := wasiRandomGet(ctx, mod, host, 0x500, 8)
		if errno != abi.WasiErrnoNotcapable {
			t.Errorf("errno = %d, want %d (Notcapable)", errno, abi.WasiErrnoNotcapable)
		}
		got := readBytes(t, mod.Memory(), 0x500, 8)
		for i, b := range got {
			if b != 0xBB {
				t.Errorf("buffer[%d] = %#x, want sentinel 0xBB", i, b)
			}
		}
	})
}

// --- TestWasiEnvironSizesGet / TestWasiEnvironGet ----------------------

func TestWasiEnvironSizesGet(t *testing.T) {
	ctx := context.Background()

	t.Run("writes 0/0 on allow", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		writeU32(t, mod.Memory(), 0x600, 0xDEADBEEF)
		writeU32(t, mod.Memory(), 0x604, 0xCAFEBABE)

		errno := wasiEnvironSizesGet(ctx, mod, host, 0x600, 0x604)
		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("errno = %d, want success", errno)
		}
		if got := readU32(t, mod.Memory(), 0x600); got != 0 {
			t.Errorf("num_elements = %d, want 0", got)
		}
		if got := readU32(t, mod.Memory(), 0x604); got != 0 {
			t.Errorf("buffer_size = %d, want 0", got)
		}
	})

	t.Run("sandbox deny → Notcapable + no write", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := denyAll()

		writeU32(t, mod.Memory(), 0x600, 0xDEADBEEF)
		writeU32(t, mod.Memory(), 0x604, 0xCAFEBABE)

		errno := wasiEnvironSizesGet(ctx, mod, host, 0x600, 0x604)
		if errno != abi.WasiErrnoNotcapable {
			t.Errorf("errno = %d, want %d", errno, abi.WasiErrnoNotcapable)
		}
		if got := readU32(t, mod.Memory(), 0x600); got != 0xDEADBEEF {
			t.Errorf("num_elements clobbered: %#x", got)
		}
		if got := readU32(t, mod.Memory(), 0x604); got != 0xCAFEBABE {
			t.Errorf("buffer_size clobbered: %#x", got)
		}
	})
}

func TestWasiEnvironGet(t *testing.T) {
	ctx := context.Background()

	t.Run("returns success without writing anything", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		// Sentinel; with an empty environ the shim writes nothing, so the sentinel should survive.
		writeU32(t, mod.Memory(), 0x700, 0x11111111)
		writeU32(t, mod.Memory(), 0x710, 0x22222222)

		errno := wasiEnvironGet(ctx, mod, host, 0x700, 0x710)
		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("errno = %d, want success", errno)
		}
		if got := readU32(t, mod.Memory(), 0x700); got != 0x11111111 {
			t.Errorf("environ_ptr region clobbered: %#x", got)
		}
		if got := readU32(t, mod.Memory(), 0x710); got != 0x22222222 {
			t.Errorf("buffer_ptr region clobbered: %#x", got)
		}
	})

	t.Run("sandbox deny → Notcapable", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := denyAll()

		errno := wasiEnvironGet(ctx, mod, host, 0x700, 0x710)
		if errno != abi.WasiErrnoNotcapable {
			t.Errorf("errno = %d, want %d", errno, abi.WasiErrnoNotcapable)
		}
	})
}

// --- TestWasiArgsSizesGet / TestWasiArgsGet ----------------------------

func TestWasiArgsSizesGet(t *testing.T) {
	ctx := context.Background()

	t.Run("writes 0/0 on allow", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		writeU32(t, mod.Memory(), 0x800, 0xAAAAAAAA)
		writeU32(t, mod.Memory(), 0x804, 0xBBBBBBBB)

		errno := wasiArgsSizesGet(ctx, mod, host, 0x800, 0x804)
		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("errno = %d, want success", errno)
		}
		if got := readU32(t, mod.Memory(), 0x800); got != 0 {
			t.Errorf("argc = %d, want 0", got)
		}
		if got := readU32(t, mod.Memory(), 0x804); got != 0 {
			t.Errorf("argv_buf_size = %d, want 0", got)
		}
	})

	t.Run("sandbox deny → Notcapable + no write", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := denyAll()

		writeU32(t, mod.Memory(), 0x800, 0xAAAAAAAA)
		writeU32(t, mod.Memory(), 0x804, 0xBBBBBBBB)

		errno := wasiArgsSizesGet(ctx, mod, host, 0x800, 0x804)
		if errno != abi.WasiErrnoNotcapable {
			t.Errorf("errno = %d, want %d", errno, abi.WasiErrnoNotcapable)
		}
		if got := readU32(t, mod.Memory(), 0x800); got != 0xAAAAAAAA {
			t.Errorf("argc clobbered: %#x", got)
		}
	})
}

func TestWasiArgsGet(t *testing.T) {
	ctx := context.Background()

	t.Run("returns success without writing", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		writeU32(t, mod.Memory(), 0x900, 0x33333333)
		writeU32(t, mod.Memory(), 0x910, 0x44444444)

		errno := wasiArgsGet(ctx, mod, host, 0x900, 0x910)
		if errno != abi.WasiErrnoSuccess {
			t.Fatalf("errno = %d, want success", errno)
		}
		if got := readU32(t, mod.Memory(), 0x900); got != 0x33333333 {
			t.Errorf("argv_ptr region clobbered: %#x", got)
		}
		if got := readU32(t, mod.Memory(), 0x910); got != 0x44444444 {
			t.Errorf("argv_buf region clobbered: %#x", got)
		}
	})

	t.Run("sandbox deny → Notcapable", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := denyAll()

		errno := wasiArgsGet(ctx, mod, host, 0x900, 0x910)
		if errno != abi.WasiErrnoNotcapable {
			t.Errorf("errno = %d, want %d", errno, abi.WasiErrnoNotcapable)
		}
	})
}

// --- TestWasiProcExit ----------------------------------------------------

func TestWasiProcExit(t *testing.T) {
	ctx := context.Background()

	t.Run("returns sys.ExitError carrying exit_code on allow", func(t *testing.T) {
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := allowAll()

		err := wasiProcExit(ctx, mod, host, 42)
		if err == nil {
			t.Fatal("err = nil, want *sys.ExitError")
		}
		var exitErr *sys.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("err type = %T, want *sys.ExitError; err = %v", err, err)
		}
		if exitErr.ExitCode() != 42 {
			t.Errorf("exit code = %d, want 42", exitErr.ExitCode())
		}
	})

	t.Run("sandbox deny → ExitError with Notcapable-as-exit-code", func(t *testing.T) {
		// proc_exit on deny still propagates a trap (it never returns to the
		// guest, by definition); the WASI Notcapable=76 errno is conveyed as
		// the exit code so callers can distinguish a capability-deny from a
		// guest-initiated exit. This documents the chosen disposition.
		mod, cleanup := newTestModule(t)
		defer cleanup()
		host := denyAll()

		err := wasiProcExit(ctx, mod, host, 0)
		if err == nil {
			t.Fatal("err = nil on deny, want *sys.ExitError")
		}
		var exitErr *sys.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("err type = %T, want *sys.ExitError; err = %v", err, err)
		}
		if exitErr.ExitCode() != uint32(abi.WasiErrnoNotcapable) {
			t.Errorf("exit code = %d, want %d (Notcapable conveyed via exit code)",
				exitErr.ExitCode(), uint32(abi.WasiErrnoNotcapable))
		}
	})
}

// --- TestWasiCapabilityKeys: byte-stable capability-key constants ---------

// The constants are consumed by Task 6 sandbox.go for the per-key roster +
// by Task 7 registration.go for the per-shim wiring. Byte-stable string
// values are required for the AMEND-A5 default-deny capability sandbox.
func TestWasiCapabilityKeys(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"fd_write", capWasiFdWrite, "fd_write"},
		{"clock_time_get", capWasiClockTimeGet, "clock_time_get"},
		{"random_get", capWasiRandomGet, "random_get"},
		{"environ_sizes_get", capWasiEnvironSizesGet, "environ_sizes_get"},
		{"environ_get", capWasiEnvironGet, "environ_get"},
		{"args_sizes_get", capWasiArgsSizesGet, "args_sizes_get"},
		{"args_get", capWasiArgsGet, "args_get"},
		{"proc_exit", capWasiProcExit, "proc_exit"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Errorf("cap key = %q, want %q", tc.got, tc.want)
			}
		})
	}
}

// --- helper: ensure encoding/binary is exercised for u32-LE helpers ------

// Compile-time use guard for the encoding/binary import even if no test body
// uses it directly (the readU32/writeU32 helpers go via the wazero memory
// methods). This keeps `goimports`-style cleanliness without imposing a
// per-edit ritual.
var _ = binary.LittleEndian
