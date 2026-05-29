package proxywasm

// Batch-B conformance families (phase-25.3 Task 14): exports, security,
// runtime, wasm_vm, bytecode_util. Each family drives a vendored guest .wasm
// (or, for bytecode_util, hand-crafted modules at the CompileModule boundary)
// through the proxy-wasm path on an in-process RootVM and asserts host-
// observable behavior. Every family is proven LIVE via the deliberate-break
// cycle recorded in PROGRESS.md (Task 14 Batch B subsection).
//
// Registration appends to the same test-scope conformanceFamilies global the
// Batch-A init() seeds (conformance.go keeps the production literal empty).

import (
	"context"
	"encoding/binary"
	"errors"
	"testing"

	"github.com/esalaine/envoy-go/internal/wasm"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

//nolint:gochecknoinits // test-scope family registration; keeps the production global empty
func init() {
	conformanceFamilies = append(conformanceFamilies,
		conformanceFamily{name: "exports", run: runExports},
		conformanceFamily{name: "security", run: runSecurity},
		conformanceFamily{name: "runtime", run: runRuntime},
		conformanceFamily{name: "wasm_vm", run: runWasmVM},
		conformanceFamily{name: "bytecode_util", run: runBytecodeUtil},
	)
}

// ─── exports ──────────────────────────────────────────────────────────────────
//
// Guest (on_vm_start): exercises the three host-observable WASI exports —
// std::env::var (environ_get/environ_sizes_get), SystemTime::now()
// (clock_time_get), and the raw random_get import — writing each result into
// shared-data. Host-observable: the harness seeds CONFORMANCE_EXPORTS via
// WithRootEnv + reads the shared-data keys, asserting the env value round-trips
// byte-faithfully (deterministic), the clock nanos are non-zero, and the random
// buffer is the requested length + not all-zero. Proves the WASI exports are
// wired across the guest<->host boundary.
func runExports(t *testing.T) {
	const envValue = "conformance-exports-seed-value"
	cvm := newConformanceRootVM(t, loadFamilyWasm(t, "exports"),
		wasm.WithRootEnv(map[string]string{"CONFORMANCE_EXPORTS": envValue}))
	// on_vm_start ran during newConformanceRootVM's Configure.

	// environ round-trip — the deterministic observable.
	gotEnv, _, res := cvm.RootVM.GetSharedData("conformance-exports-env")
	if res != abi.WasmResultOk {
		t.Fatalf("GetSharedData(env): result %v, want Ok", res)
	}
	if string(gotEnv) != envValue {
		t.Errorf("environ round-trip: got %q, want %q (WithRootEnv -> environ_get must feed std::env::var)", gotEnv, envValue)
	}

	// clock_time_get — non-zero nanos.
	gotClock, _, res := cvm.RootVM.GetSharedData("conformance-exports-clock")
	if res != abi.WasmResultOk {
		t.Fatalf("GetSharedData(clock): result %v, want Ok", res)
	}
	if len(gotClock) != 8 {
		t.Fatalf("clock value: got %d bytes, want 8 (u64 LE nanos)", len(gotClock))
	}
	if nanos := binary.LittleEndian.Uint64(gotClock); nanos == 0 {
		t.Errorf("clock_time_get returned 0 nanos (SystemTime::now must reach the host clock shim)")
	}

	// random_get — errno success + non-zero buffer of requested length.
	gotErrno, _, res := cvm.RootVM.GetSharedData("conformance-exports-random-errno")
	if res != abi.WasmResultOk {
		t.Fatalf("GetSharedData(random-errno): result %v, want Ok", res)
	}
	if len(gotErrno) != 4 || binary.LittleEndian.Uint32(gotErrno) != 0 {
		t.Errorf("random_get errno = % x, want 0 (success)", gotErrno)
	}
	gotRandom, _, res := cvm.RootVM.GetSharedData("conformance-exports-random")
	if res != abi.WasmResultOk {
		t.Fatalf("GetSharedData(random): result %v, want Ok", res)
	}
	if len(gotRandom) != 16 {
		t.Fatalf("random buffer: got %d bytes, want 16", len(gotRandom))
	}
	allZero := true
	for _, b := range gotRandom {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Errorf("random_get returned all-zero buffer (host random_get shim must fill it)")
	}
}

// ─── security ───────────────────────────────────────────────────────────────
//
// Guest (on_request_headers): logs "log-ok" via the always-allowed proxy_log,
// then calls proxy_get_current_time and logs "time-ok" on success. The SDK
// panics (-> wasm trap) on the host deny sentinel, so a DENIED time hostcall
// traps before "time-ok". Host-observable: the harness drives the SAME guest
// under two sandboxes:
//   - permissive: CallProxyOnRequestHeaders returns nil; log shows "log-ok"
//     AND "time-ok".
//   - restricted (proxy_get_current_time_nanoseconds DENIED via a per-family
//     SandboxConfig override): CallProxyOnRequestHeaders returns a NON-NIL
//     error (the trap surfaces); "log-ok" still landed (proxy_log allowed, ran
//     first) but "time-ok" did NOT.
//
// Proves the host enforces a PER-CAPABILITY gate: one cap allowed succeeds, a
// different cap denied is rejected — not all-or-nothing.
func runSecurity(t *testing.T) {
	ctx := context.Background()

	t.Run("allowed", func(t *testing.T) {
		cvm := newConformanceRootVM(t, loadFamilyWasm(t, "security"))
		sc, err := cvm.RootVM.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext: %v", err)
		}
		if _, err := sc.CallProxyOnRequestHeaders(ctx, 0, true); err != nil {
			t.Fatalf("CallProxyOnRequestHeaders (allowed): %v", err)
		}
		assertLogContains(t, cvm.Logs, "conformance-security log-ok")
		assertLogContains(t, cvm.Logs, "conformance-security time-ok")
	})

	t.Run("denied", func(t *testing.T) {
		// Restricted sandbox: deny the time hostcall, keep proxy_log allowed.
		cvm := newConformanceRootVM(t, loadFamilyWasm(t, "security"),
			wasm.WithRootSandboxConfig(conformanceSandboxDenying("proxy_get_current_time_nanoseconds")))
		sc, err := cvm.RootVM.NewStreamContext(ctx)
		if err != nil {
			t.Fatalf("NewStreamContext: %v", err)
		}
		_, err = sc.CallProxyOnRequestHeaders(ctx, 0, true)
		if err == nil {
			t.Fatalf("denied: CallProxyOnRequestHeaders returned nil; want a surfaced trap (deny sentinel)")
		}
		// proxy_log still works (per-capability gate) — its line ran first.
		assertLogContains(t, cvm.Logs, "conformance-security log-ok")
		// The denied time hostcall trapped before reaching "time-ok".
		assertLogNotContains(t, cvm.Logs, "conformance-security time-ok")
	})
}

// ─── runtime ──────────────────────────────────────────────────────────────────
//
// Guest (on_request_headers): deliberately traps (panic -> wasm unreachable).
// Host-observable: CallProxyOnRequestHeaders returns a NON-NIL error (the host
// runtime catches the wazero trap rather than silently continuing). Runs on its
// OWN fresh RootVM (newConformanceRootVM builds a dedicated VM per call) so the
// poisoned instance does not leak into other families.
func runRuntime(t *testing.T) {
	ctx := context.Background()
	cvm := newConformanceRootVM(t, loadFamilyWasm(t, "runtime"))
	sc, err := cvm.RootVM.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext: %v", err)
	}
	_, err = sc.CallProxyOnRequestHeaders(ctx, 0, true)
	if err == nil {
		t.Fatalf("CallProxyOnRequestHeaders on a trapping guest returned nil error; want a surfaced trap")
	}
}

// ─── wasm_vm ────────────────────────────────────────────────────────────────
//
// Guest (on_request_headers): each HTTP stream gets its OWN Filter with a
// private call counter, written to response header x-stream-count on each call.
// Host-observable: two StreamContexts have distinct ContextID()s; driving
// stream A twice yields "1" then "2", and stream B once yields "1" (NOT "3"),
// proving per-stream context isolation + independent VM-init per-context state.
func runWasmVM(t *testing.T) {
	ctx := context.Background()
	cvm := newConformanceRootVM(t, loadFamilyWasm(t, "wasm_vm"))

	scA, err := cvm.RootVM.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext(A): %v", err)
	}
	scB, err := cvm.RootVM.NewStreamContext(ctx)
	if err != nil {
		t.Fatalf("NewStreamContext(B): %v", err)
	}
	if scA.ContextID() == scB.ContextID() {
		t.Fatalf("per-stream isolation: streams A and B share ContextID %d (want distinct)", scA.ContextID())
	}

	// Stream A, first call -> counter 1.
	cvm.ABI.ResetWrittenResponseHeaders()
	if _, err := scA.CallProxyOnRequestHeaders(ctx, 0, true); err != nil {
		t.Fatalf("A call 1: %v", err)
	}
	if v, ok := cvm.ABI.WrittenResponseHeader("x-stream-count"); !ok || v != "1" {
		t.Errorf("A call 1: x-stream-count = %q (ok=%v), want \"1\"", v, ok)
	}

	// Stream A, second call -> counter 2 (its own state advances).
	cvm.ABI.ResetWrittenResponseHeaders()
	if _, err := scA.CallProxyOnRequestHeaders(ctx, 0, true); err != nil {
		t.Fatalf("A call 2: %v", err)
	}
	if v, ok := cvm.ABI.WrittenResponseHeader("x-stream-count"); !ok || v != "2" {
		t.Errorf("A call 2: x-stream-count = %q (ok=%v), want \"2\"", v, ok)
	}

	// Stream B, first call -> counter 1 (fresh per-stream state, NOT 3).
	cvm.ABI.ResetWrittenResponseHeaders()
	if _, err := scB.CallProxyOnRequestHeaders(ctx, 0, true); err != nil {
		t.Fatalf("B call 1: %v", err)
	}
	if v, ok := cvm.ABI.WrittenResponseHeader("x-stream-count"); !ok || v != "1" {
		t.Errorf("B call 1: x-stream-count = %q (ok=%v), want \"1\" (per-stream isolation: B must not see A's count)", v, ok)
	}
}

// ─── bytecode_util ──────────────────────────────────────────────────────────
//
// Asserts at the CompileModule boundary (the proxy-wasm bytecode_util custom-
// section + ABI-version parse): a module exporting proxy_abi_version_0_2_1
// compiles; a module exporting a WRONG sentinel (v0.1.0) fails CompileModule
// wrapping ErrUnsupportedAbiVersion; a module with NO abi-version sentinel also
// fails wrapping ErrUnsupportedAbiVersion. Uses hand-crafted minimal modules
// (no Rust guest needed) built with the in-file wasm DSL.
func runBytecodeUtil(t *testing.T) {
	ctx := context.Background()

	t.Run("v0_2_1_compiles", func(t *testing.T) {
		mod, err := wasm.CompileModule(ctx, craftAbiModule("proxy_abi_version_0_2_1"), nil)
		if err != nil {
			t.Fatalf("CompileModule(v0.2.1): %v, want success", err)
		}
		if mod == nil {
			t.Fatal("CompileModule(v0.2.1) returned nil module")
		}
	})

	t.Run("wrong_abi_rejected", func(t *testing.T) {
		_, err := wasm.CompileModule(ctx, craftAbiModule("proxy_abi_version_0_1_0"), nil)
		if err == nil {
			t.Fatalf("CompileModule(v0.1.0) succeeded; want ErrUnsupportedAbiVersion")
		}
		if !errors.Is(err, wasm.ErrUnsupportedAbiVersion) {
			t.Errorf("CompileModule(v0.1.0) err = %v; want wrapping ErrUnsupportedAbiVersion", err)
		}
	})

	t.Run("missing_abi_rejected", func(t *testing.T) {
		// No abi-version export at all -> AbiVersionUnknown -> rejected.
		_, err := wasm.CompileModule(ctx, craftAbiModule(""), nil)
		if err == nil {
			t.Fatalf("CompileModule(no-sentinel) succeeded; want ErrUnsupportedAbiVersion")
		}
		if !errors.Is(err, wasm.ErrUnsupportedAbiVersion) {
			t.Errorf("CompileModule(no-sentinel) err = %v; want wrapping ErrUnsupportedAbiVersion", err)
		}
	})
}

// ─── in-file wasm DSL (bytecode_util) ─────────────────────────────────────────
//
// Minimal wasm-module builders mirroring internal/wasm/compile_test.go's
// helpers (replicated here because those are package-private to internal/wasm).
// craftAbiModule produces a complete module wazero can compile: header + type +
// function + (optional named export) + code section. With exportName set to an
// abi-version sentinel the module's GetAbiVersion scan detects that version; an
// empty exportName yields AbiVersionUnknown.

// wasmHeaderBytes is the 4-byte magic + 4-byte LE version prefix.
//
//nolint:gochecknoglobals // immutable test fixture (the wasm module prefix)
var wasmHeaderBytes = []byte{
	0x00, 0x61, 0x73, 0x6d, // "\0asm" magic
	0x01, 0x00, 0x00, 0x00, // version 1
}

func crafAppendUleb128(dst []byte, v uint32) []byte {
	var buf [binary.MaxVarintLen32]byte
	n := binary.PutUvarint(buf[:], uint64(v))
	return append(dst, buf[:n]...)
}

func craftAbiModule(exportName string) []byte {
	out := append([]byte{}, wasmHeaderBytes...)
	// Type section: ID 0x01 || size || count=1 || func-form 0x60, 0 params, 0 results.
	typeBody := []byte{0x01, 0x60, 0x00, 0x00}
	out = append(out, 0x01)
	out = crafAppendUleb128(out, uint32(len(typeBody)))
	out = append(out, typeBody...)
	// Function section: ID 0x03 || size || count=1 || type-idx=0.
	fnBody := []byte{0x01, 0x00}
	out = append(out, 0x03)
	out = crafAppendUleb128(out, uint32(len(fnBody)))
	out = append(out, fnBody...)
	// Export section (optional): the named export points at function index 0.
	if exportName != "" {
		var entry []byte
		entry = crafAppendUleb128(entry, uint32(len(exportName)))
		entry = append(entry, exportName...)
		entry = append(entry, 0x00) // kind=function
		entry = crafAppendUleb128(entry, 0)
		secBody := crafAppendUleb128(nil, 1) // vector len = 1
		secBody = append(secBody, entry...)
		out = append(out, 0x07)
		out = crafAppendUleb128(out, uint32(len(secBody)))
		out = append(out, secBody...)
	}
	// Code section: ID 0x0a || size || count=1 || body(size=2 || local-count=0 || end=0x0b).
	funcBody := []byte{0x00, 0x0b}
	bodyWithSize := crafAppendUleb128(nil, uint32(len(funcBody)))
	bodyWithSize = append(bodyWithSize, funcBody...)
	codeBody := []byte{0x01}
	codeBody = append(codeBody, bodyWithSize...)
	out = append(out, 0x0a)
	out = crafAppendUleb128(out, uint32(len(codeBody)))
	out = append(out, codeBody...)
	return out
}
