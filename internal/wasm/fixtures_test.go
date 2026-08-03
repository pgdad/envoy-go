// Hand-crafted wasm binary fixtures for vm_test.go + registration_test.go.
//
// Each fixture below is a minimal, valid WebAssembly Core 1.0 binary
// constructed via the small DSL in this file. The DSL constructs section
// payloads + assembles them into a complete module — keeping the byte
// layout explicit + auditable (no external wat2wasm or precompiled blobs;
// every byte is generated at test-init time from Go-level helpers).
//
// The fixtures cover three test surfaces:
//
//	1. Module-init lifecycle (Run-step b): _initialize / _start / neither.
//	2. Host-module wiring smoke tests: a module imports an env hostcall and
//	   instantiates without error (verifies registration is wired correctly).
//	3. End-to-end ABICallbacks invocation: a module imports + CALLS an env
//	   hostcall from an exported function; the test verifies the ABICallbacks
//	   method fires + the wire-result returns to the guest.
//
// Wire reference: https://webassembly.github.io/spec/core/binary/modules.html
//
// Build constraint: this is a `_test.go`-suffixed file (filename ends with
// `_test.go`) so it is compiled ONLY for `go test`. Both vm_test.go and
// registration_test.go in the same package can reference the fixture vars.

package wasm

import (
	"encoding/binary"
)

// uleb128 returns the unsigned LEB128 encoding of v.
func uleb128(v uint32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7f)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

// sleb128 returns the signed LEB128 encoding of v (used for i32.const).
func sleb128(v int32) []byte {
	var out []byte
	more := true
	for more {
		b := byte(v & 0x7f)
		v >>= 7
		signBit := b & 0x40
		if (v == 0 && signBit == 0) || (v == -1 && signBit != 0) {
			more = false
		} else {
			b |= 0x80
		}
		out = append(out, b)
	}
	return out
}

// section builds a section: id || size || payload.
func section(id byte, payload []byte) []byte {
	out := []byte{id}
	out = append(out, uleb128(uint32(len(payload)))...)
	out = append(out, payload...)
	return out
}

// WASM value-type encodings (per binary spec).
const (
	wasmTypeI32 = 0x7f
	wasmTypeI64 = 0x7e
)

// WASM external-kind encodings.
const (
	wasmExtFunction = 0x00
	wasmExtMemory   = 0x02
)

// WASM opcode encodings used in this file.
const (
	opUnreachable = 0x00
	opEnd         = 0x0b
	opCall        = 0x10
	opLocalGet    = 0x20
	opI32Const    = 0x41
	opI32Store    = 0x36
	opDrop        = 0x1a
)

// funcType encodes a (params, results) type signature.
// Layout: 0x60 || count(params) || params_types || count(results) || result_types.
func funcType(params, results []byte) []byte {
	out := []byte{0x60}
	out = append(out, uleb128(uint32(len(params)))...)
	out = append(out, params...)
	out = append(out, uleb128(uint32(len(results)))...)
	out = append(out, results...)
	return out
}

// typeSection encodes a type section from a list of (params, results) tuples.
func typeSection(types ...[2][]byte) []byte {
	payload := uleb128(uint32(len(types)))
	for _, t := range types {
		payload = append(payload, funcType(t[0], t[1])...)
	}
	return section(0x01, payload)
}

// importSection encodes the import section. Each import is (module, name, kind, type_index).
// `imports` is a slice of [4]any: {moduleName string, exportName string, kind byte, typeIdx uint32}.
type importEntry struct {
	module string
	name   string
	kind   byte
	idx    uint32 // type index for function imports
}

func importSection(imports []importEntry) []byte {
	payload := uleb128(uint32(len(imports)))
	for _, imp := range imports {
		payload = append(payload, uleb128(uint32(len(imp.module)))...)
		payload = append(payload, []byte(imp.module)...)
		payload = append(payload, uleb128(uint32(len(imp.name)))...)
		payload = append(payload, []byte(imp.name)...)
		payload = append(payload, imp.kind)
		switch imp.kind {
		case wasmExtFunction:
			payload = append(payload, uleb128(imp.idx)...)
		default:
			// not used in our fixtures
		}
	}
	return section(0x02, payload)
}

// functionSection encodes the function-section type-index list for the
// module's own (non-imported) functions.
func functionSection(typeIndices []uint32) []byte {
	payload := uleb128(uint32(len(typeIndices)))
	for _, idx := range typeIndices {
		payload = append(payload, uleb128(idx)...)
	}
	return section(0x03, payload)
}

// memorySection encodes a memory section with a single memory of min-pages.
func memorySection(minPages uint32) []byte {
	payload := uleb128(1)           // count = 1
	payload = append(payload, 0x00) // limits flag: no max
	payload = append(payload, uleb128(minPages)...)
	return section(0x05, payload)
}

// exportEntry describes a single export.
type exportEntry struct {
	name string
	kind byte
	idx  uint32
}

// exportSection encodes the export section.
func exportSection(exports []exportEntry) []byte {
	payload := uleb128(uint32(len(exports)))
	for _, exp := range exports {
		payload = append(payload, uleb128(uint32(len(exp.name)))...)
		payload = append(payload, []byte(exp.name)...)
		payload = append(payload, exp.kind)
		payload = append(payload, uleb128(exp.idx)...)
	}
	return section(0x07, payload)
}

// codeSection encodes the code section. Each entry is a function body:
// local-count || locals || expr-bytes || 0x0b (end). We accept the body
// bytes whole (caller provides expr+end).
func codeSection(bodies [][]byte) []byte {
	payload := uleb128(uint32(len(bodies)))
	for _, body := range bodies {
		// Each entry: size || body bytes. Body bytes already include
		// local-count + expr + 0x0b end.
		payload = append(payload, uleb128(uint32(len(body)))...)
		payload = append(payload, body...)
	}
	return section(0x0a, payload)
}

// funcBody constructs a function body: 0 locals || expr || end.
func funcBody(expr ...[]byte) []byte {
	out := []byte{0x00} // local-count = 0
	for _, e := range expr {
		out = append(out, e...)
	}
	out = append(out, opEnd)
	return out
}

// i32Const emits `i32.const N`.
func i32Const(v int32) []byte {
	return append([]byte{opI32Const}, sleb128(v)...)
}

// localGet emits `local.get idx`.
func localGet(idx uint32) []byte {
	return append([]byte{opLocalGet}, uleb128(idx)...)
}

// call emits `call funcIdx`.
func call(funcIdx uint32) []byte {
	return append([]byte{opCall}, uleb128(funcIdx)...)
}

// drop emits `drop`.
func drop() []byte { return []byte{opDrop} }

// i32Store emits `i32.store offset=0 align=2`. Reserved for future
// fixtures that exercise return-by-reference hostcall paths.
func i32Store() []byte { //nolint:unused // reserved for future fixtures
	return []byte{opI32Store, 0x02, 0x00}
}

// moduleHeader is the 4-byte magic + 4-byte version 1 prefix.
var moduleHeader = []byte{
	0x00, 0x61, 0x73, 0x6d, // magic "\0asm"
	0x01, 0x00, 0x00, 0x00, // version 1
}

// buildModule concatenates header + sections.
func buildModule(sections ...[]byte) []byte {
	out := make([]byte, 0, 64)
	out = append(out, moduleHeader...)
	for _, s := range sections {
		out = append(out, s...)
	}
	return out
}

// --- Fixture: minimalInitModule -------------------------------------------
//
// Exports `_initialize` (empty body) + `proxy_abi_version_0_2_1` (empty
// body — present only so CompileModule's ABI-version gate accepts the
// module) + a 1-page memory.
//
// Used by Run-lifecycle tests + tests that need a valid Module instance
// but don't exercise hostcalls.
var minimalInitModule = buildModule(
	typeSection([2][]byte{
		nil, nil, // () -> ()
	}),
	functionSection([]uint32{0, 0}), // two local fns, both type 0
	memorySection(1),
	exportSection([]exportEntry{
		{name: "_initialize", kind: wasmExtFunction, idx: 0},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 1},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(),
		funcBody(),
	}),
)

// --- Fixture: noInitModule ------------------------------------------------
//
// Exports a 1-page memory + a non-init function + the ABI sentinel. Used
// to verify Run handles modules without _initialize / _start gracefully.
// (Held for use by future Tasks 2-22 lifecycle tests; the 25.1 vm_test.go
// consumer is gone after Task 1's vm.go DELETE per D-P-PLAN-6.)
//
//nolint:unused // retained for forward Task 2-22 lifecycle tests
var noInitModule = buildModule(
	typeSection([2][]byte{
		nil, nil, // () -> ()
	}),
	functionSection([]uint32{0, 0}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "noop", kind: wasmExtFunction, idx: 0},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 1},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(),
		funcBody(),
	}),
)

// --- Fixture: importerProxyLogModule --------------------------------------
//
// Imports `env.proxy_log(i32, i32, i32) -> i32` but does NOT call it.
// Used to verify the env host module is registered + instantiation succeeds.
var importerProxyLogModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32}, {wasmTypeI32}}, // type 0: proxy_log
		[2][]byte{nil, nil}, // type 1: () -> ()
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_log", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1}), // local function 0 has type 1
	memorySection(1),
	exportSection([]exportEntry{
		{name: "_initialize", kind: wasmExtFunction, idx: 1}, // local fn 0 = global fn idx 1 (after 1 import)
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(), // empty
	}),
)

// --- Fixture: invokeProxyLogModule ----------------------------------------
//
// Imports `env.proxy_log(i32, i32, i32) -> i32` AND exports an
// `invoke_proxy_log() -> i32` function that calls proxy_log with
// (level=2, msg_ptr=0, msg_size=0) and returns its result. Also exports
// `proxy_abi_version_0_2_1` (no-op body) for CompileModule's ABI gate.
//
// At a high level the body is:
//
//	(func $invoke_proxy_log (result i32)
//	   i32.const 2
//	   i32.const 0
//	   i32.const 0
//	   call $proxy_log)
//
// The 0-size message means the hostcall body's readString returns the
// empty string + the cb.Log path fires with msg="".
var invokeProxyLogModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32}, {wasmTypeI32}}, // type 0: proxy_log
		[2][]byte{nil, {wasmTypeI32}},                                     // type 1: () -> i32
		[2][]byte{nil, nil},                                               // type 2: () -> ()  (for the ABI sentinel)
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_log", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1, 2}), // local fn 0 (idx 1 global): invoke; local fn 1 (idx 2 global): sentinel
	memorySection(1),
	exportSection([]exportEntry{
		{name: "invoke_proxy_log", kind: wasmExtFunction, idx: 1},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(
			i32Const(2), // level = Info
			i32Const(0), // msg_ptr
			i32Const(0), // msg_size
			call(0),     // proxy_log → leaves i32 on stack (the function result)
		),
		funcBody(), // proxy_abi_version_0_2_1: no-op
	}),
)

// --- Fixture: onRequestHeadersInvokesLogModule ----------------------------
//
// Exports `proxy_on_request_headers(stream_ctx_id, num_headers, end_of_stream)
// -> i32` that internally calls proxy_log(2, 0, 0) (info, empty msg), drops
// the result, and returns 0 (ProxyActionContinue). Also exports the ABI
// sentinel for CompileModule's gate.
var onRequestHeadersInvokesLogModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32}, {wasmTypeI32}}, // type 0
		[2][]byte{nil, nil}, // type 1: () -> ()
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_log", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{0, 1}), // local fn 0 (idx 1 global): orh; local fn 1 (idx 2 global): sentinel
	memorySection(1),
	exportSection([]exportEntry{
		{name: "proxy_on_request_headers", kind: wasmExtFunction, idx: 1},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(
			i32Const(2),
			i32Const(0),
			i32Const(0),
			call(0),     // proxy_log → returns i32 on stack
			drop(),      // discard proxy_log result
			i32Const(0), // return ProxyActionContinue
		),
		funcBody(), // proxy_abi_version_0_2_1: no-op
	}),
)

// --- Fixture: invokeSetTickPeriodModule -----------------------------------
//
// Imports `env.proxy_set_tick_period_milliseconds(i32) -> i32` (a 25.2 NEW
// gated hostcall per AMEND-B5 that LANDED its real impl at Task 5) and
// exports `invoke_set_tick_period() -> i32` that calls
// proxy_set_tick_period_milliseconds(100) + returns its result.
//
// AT TASK 5: this fixture is no longer used by the panic-discipline test
// (the real impl returns Ok now); RETAINED here for use by future Task 14
// configure-time tests that exercise the per-RootVM SetTickPeriod hostcall
// via the guest. The panic-discipline coverage migrated to
// invokeGetSharedDataModule below (which targets the still-Task-6-deferred
// proxy_get_shared_data placeholder).
//
//nolint:unused // retained for forward Task 14 configure-time tests
var invokeSetTickPeriodModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32}, {wasmTypeI32}}, // type 0: (i32) -> i32 (proxy_set_tick_period_milliseconds)
		[2][]byte{nil, {wasmTypeI32}},           // type 1: () -> i32 (invoke_set_tick_period)
		[2][]byte{nil, nil},                     // type 2: () -> () (sentinel)
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_set_tick_period_milliseconds", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1, 2}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "invoke_set_tick_period", kind: wasmExtFunction, idx: 1},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(
			i32Const(100), // period_ms = 100
			call(0),       // proxy_set_tick_period_milliseconds → returns i32 on stack
		),
		funcBody(),
	}),
)

// --- Fixture: invokeGetSharedDataModule -----------------------------------
//
// Imports `env.proxy_get_shared_data(5 × i32) -> i32` (a 25.2 NEW gated
// hostcall per AMEND-B5 that LANDED its real impl at Task 6) and exports
// `invoke_get_shared_data() -> i32` that calls proxy_get_shared_data(0,0,
// 0,0,0) + returns its result.
//
// AT TASK 6: this fixture is no longer used by the panic-discipline test
// (the real impl returns NotFound on an empty/zero-key Get now); RETAINED
// here for use by future Task 14+ tests that exercise the per-RootVM
// SetSharedData / GetSharedData hostcalls via the guest. The panic-
// discipline coverage migrated to invokeCallForeignFunctionModule below
// (which targets the still-Task-7-deferred proxy_call_foreign_function
// placeholder).
//
//nolint:unused // retained for forward Task 14+ shared-data guest-side tests
var invokeGetSharedDataModule = buildModule(
	typeSection(
		[2][]byte{nI32Params(5), {wasmTypeI32}}, // type 0: (5 × i32) -> i32 (proxy_get_shared_data)
		[2][]byte{nil, {wasmTypeI32}},           // type 1: () -> i32 (invoke_get_shared_data)
		[2][]byte{nil, nil},                     // type 2: () -> () (sentinel)
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_get_shared_data", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1, 2}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "invoke_get_shared_data", kind: wasmExtFunction, idx: 1},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(
			i32Const(0), // key_data
			i32Const(0), // key_size
			i32Const(0), // ret_value_data
			i32Const(0), // ret_value_size
			i32Const(0), // ret_cas
			call(0),     // proxy_get_shared_data → returns i32 on stack
		),
		funcBody(),
	}),
)

// --- Fixture: invokeCallForeignFunctionModule -----------------------------
//
// Imports `env.proxy_call_foreign_function(6 × i32) -> i32` (a 25.2 NEW
// gated hostcall per AMEND-B5 that LANDED its real impl at Task 7) and
// exports `invoke_call_foreign_function() -> i32` that calls
// proxy_call_foreign_function(0,0,0,0,0,0) + returns its result.
//
// AT TASK 7: this fixture is no longer used by the panic-discipline test
// (the real impl returns NotFound on an empty/zero-name Get against the
// EMPTY default registry now); RETAINED here for use by future Task 14+
// tests that exercise the per-RootVM CallForeignFunction hostcall via the
// guest. The panic-discipline coverage migrated to invokeHttpCallModule
// below (which targets the still-Task-8-deferred proxy_http_call
// placeholder).
//
//nolint:unused // retained for forward Task 14+ foreign-function guest-side tests
var invokeCallForeignFunctionModule = buildModule(
	typeSection(
		[2][]byte{nI32Params(6), {wasmTypeI32}}, // type 0: (6 × i32) -> i32 (proxy_call_foreign_function)
		[2][]byte{nil, {wasmTypeI32}},           // type 1: () -> i32 (invoke_call_foreign_function)
		[2][]byte{nil, nil},                     // type 2: () -> () (sentinel)
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_call_foreign_function", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1, 2}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "invoke_call_foreign_function", kind: wasmExtFunction, idx: 1},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(
			i32Const(0), // name_data
			i32Const(0), // name_size
			i32Const(0), // args_data
			i32Const(0), // args_size
			i32Const(0), // ret_results_data
			i32Const(0), // ret_results_size
			call(0),     // proxy_call_foreign_function → returns i32 on stack
		),
		funcBody(),
	}),
)

// --- Fixture: invokeHttpCallModule ----------------------------------------
//
// Imports `env.proxy_http_call(10 × i32) -> i32` (a 25.2 NEW gated hostcall
// per AMEND-B5 that LANDED its real impl at Task 8) and exports
// `invoke_http_call() -> i32` that calls proxy_http_call(0,...,0) +
// returns its result.
//
// AT TASK 8: this fixture is no longer used by the panic-discipline test
// (the real impl returns InternalFailure on no-dispatcher-wired now);
// RETAINED here for use by future Task 14+ tests that exercise the
// per-RootVM DispatchHttpCall hostcall via the guest. The panic-discipline
// coverage migrated to invokeDefineMetricModule below (which targets the
// still-Task-12-deferred proxy_define_metric placeholder).
//
// Re-target trail: panic-discipline test originally hit
// proxy_continue_stream (LIFTED at Task 4) → proxy_set_tick_period_milliseconds
// (LIFTED at Task 5) → proxy_get_shared_data (LIFTED at Task 6) →
// proxy_call_foreign_function (LIFTED at Task 7) → proxy_http_call
// (LIFTED at Task 8) → proxy_define_metric (still pending at Task 12 —
// see invokeDefineMetricModule below).
//
//nolint:unused // retained for forward Task 14+ http_call guest-side tests
var invokeHttpCallModule = buildModule(
	typeSection(
		[2][]byte{nI32Params(10), {wasmTypeI32}}, // type 0: (10 × i32) -> i32 (proxy_http_call)
		[2][]byte{nil, {wasmTypeI32}},            // type 1: () -> i32 (invoke_http_call)
		[2][]byte{nil, nil},                      // type 2: () -> () (sentinel)
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_http_call", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1, 2}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "invoke_http_call", kind: wasmExtFunction, idx: 1},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(
			i32Const(0), // cluster_data
			i32Const(0), // cluster_size
			i32Const(0), // headers_data
			i32Const(0), // headers_size
			i32Const(0), // body_data
			i32Const(0), // body_size
			i32Const(0), // trailers_data
			i32Const(0), // trailers_size
			i32Const(0), // timeout_ms
			i32Const(0), // ret_call_id_ptr
			call(0),     // proxy_http_call → returns i32 on stack
		),
		funcBody(),
	}),
)

// --- Fixture: invokeDefineMetricModule — RETIRED at Task 12 ---------------
//
// This fixture supported the placeholder-panic-discipline test re-targeted
// across Tasks 4-8 to point at the still-stub hostcall (last re-target
// pointed at proxy_define_metric). With Task 12 landing the real metric
// shims (abi/metrics.go), no Task-3 forward-decl placeholders remain in
// abi/stubs_25_2.go — the placeholder-panic discipline is RETIRED. The
// general panic-wrapper invariant remains covered by the per-family abi
// non-Host-value tests + stream_context_test.go panic-recovery patterns
// (see registration_test.go panic-discipline-test history doc).

// --- Fixture: invokeGrpcCancelModule --------------------------------------
//
// Imports `env.proxy_grpc_cancel(i32) -> i32` (a STILL-deferred stub at
// 25.2 per §5.4 — gRPC family deferred to WASM host family per §2.8) and
// exports `invoke_grpc_cancel() -> i32` that calls proxy_grpc_cancel(0) +
// returns its result. Used by TestRegistration_DeferredStub_Unimplemented
// to verify the still-deferred stub returns WasmResultUnimplemented (=12).
var invokeGrpcCancelModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32}, {wasmTypeI32}}, // type 0: (i32) -> i32 (proxy_grpc_cancel)
		[2][]byte{nil, {wasmTypeI32}},           // type 1: () -> i32 (invoke_grpc_cancel)
		[2][]byte{nil, nil},                     // type 2: () -> () (sentinel)
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_grpc_cancel", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1, 2}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "invoke_grpc_cancel", kind: wasmExtFunction, idx: 1},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(
			i32Const(0), // token
			call(0),     // proxy_grpc_cancel → returns i32 on stack (the function result)
		),
		funcBody(),
	}),
)

// --- Fixture: lifecycleOrderModule ----------------------------------------
//
// Exports the three module-init lifecycle callbacks that vm.Run invokes for
// the root context:
//
//	proxy_on_context_create(ctx_id, parent_ctx_id) -> ()
//	proxy_on_vm_start(ctx_id, vm_config_size)      -> i32 (success=1)
//	proxy_on_configure(ctx_id, plugin_config_size) -> i32 (success=1)
//
// Each callback calls `proxy_log` with a DISTINCT level so the test can
// recover the firing order from the recorded log levels:
//
//	proxy_on_context_create → proxy_log(level=2 = LogLevelInfo)
//	proxy_on_vm_start       → proxy_log(level=3 = LogLevelWarn)
//	proxy_on_configure      → proxy_log(level=4 = LogLevelError)
//
// Used by TestVM_Run_Lifecycle_ContextCreateBeforeVmStart to verify the
// canonical proxy-wasm host lifecycle order — proxy_on_context_create MUST
// fire BEFORE proxy_on_vm_start (matches upstream proxy-wasm-cpp-host +
// proxy-wasm-rust-sdk dispatcher expectation; panicking dispatchers like
// proxy-wasm-rust-sdk v0.2.4 require the root context to be pre-registered
// before proxy_on_vm_start is invoked).
//
// Also exports `_initialize` (empty) + `proxy_abi_version_0_2_1` (sentinel)
// + a 1-page memory.
var lifecycleOrderModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32}, {wasmTypeI32}}, // type 0: proxy_log (i32,i32,i32) -> i32
		[2][]byte{{wasmTypeI32, wasmTypeI32}, nil},                        // type 1: (i32,i32) -> ()      — proxy_on_context_create
		[2][]byte{{wasmTypeI32, wasmTypeI32}, {wasmTypeI32}},              // type 2: (i32,i32) -> i32     — proxy_on_vm_start / proxy_on_configure
		[2][]byte{nil, nil}, // type 3: () -> ()             — _initialize / sentinel
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_log", kind: wasmExtFunction, idx: 0}, // global fn idx 0
	}),
	// Local fns (after 1 import):
	//   global idx 1: proxy_on_context_create — type 1
	//   global idx 2: proxy_on_vm_start       — type 2
	//   global idx 3: proxy_on_configure      — type 2
	//   global idx 4: _initialize             — type 3
	//   global idx 5: proxy_abi_version_0_2_1 — type 3
	functionSection([]uint32{1, 2, 2, 3, 3}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "proxy_on_context_create", kind: wasmExtFunction, idx: 1},
		{name: "proxy_on_vm_start", kind: wasmExtFunction, idx: 2},
		{name: "proxy_on_configure", kind: wasmExtFunction, idx: 3},
		{name: "_initialize", kind: wasmExtFunction, idx: 4},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 5},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		// proxy_on_context_create: proxy_log(2,0,0), drop, return void
		funcBody(
			i32Const(2), // level = Info
			i32Const(0), // msg_ptr
			i32Const(0), // msg_size
			call(0),     // proxy_log -> i32 on stack
			drop(),      // discard result
		),
		// proxy_on_vm_start: proxy_log(3,0,0), drop, return 1 (success)
		funcBody(
			i32Const(3), // level = Warn
			i32Const(0),
			i32Const(0),
			call(0),
			drop(),
			i32Const(1), // return success
		),
		// proxy_on_configure: proxy_log(4,0,0), drop, return 1 (success)
		funcBody(
			i32Const(4), // level = Error
			i32Const(0),
			i32Const(0),
			call(0),
			drop(),
			i32Const(1),
		),
		funcBody(), // _initialize: no-op
		funcBody(), // proxy_abi_version_0_2_1: no-op
	}),
)

// --- Fixture: contextCreateTrapsModule ------------------------------------
//
// Exports `proxy_on_context_create(ctx_id, parent_ctx_id) -> ()` whose body
// is a single `unreachable` opcode (0x00) — the canonical wazero-trap
// instruction. Used to verify RootVM.NewStreamContext's rollback path
// (delete(rv.streamCtxs, id) on dispatch failure per SHOULD-FIX-5 review
// finding). Also exports the ABI sentinel + a 1-page memory; _initialize
// is intentionally OMITTED so Configure-time proxy_on_context_create (root
// seeding) ALSO traps — the test constructs the RootVM but does NOT call
// Configure (it exercises NewStreamContext directly).
var contextCreateTrapsModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32, wasmTypeI32}, nil}, // type 0: (i32,i32) -> ()  — proxy_on_context_create
		[2][]byte{nil, nil},                        // type 1: () -> ()         — sentinel
	),
	functionSection([]uint32{0, 1}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "proxy_on_context_create", kind: wasmExtFunction, idx: 0},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 1},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		// proxy_on_context_create body: single `unreachable` opcode -> wazero trap.
		funcBody(
			[]byte{opUnreachable},
		),
		funcBody(), // sentinel: no-op
	}),
)

// --- Fixture: requestHeadersTrapsModule -----------------------------------
//
// Exports proxy_on_request_headers(ctx_id, num_headers, eos) -> i32 whose
// body is a single `unreachable` (a wazero trap) — modeling a guest panic
// (e.g. a Rust panic! that aborts to `unreachable`). The teardown triplet
// (proxy_on_done -> i32, proxy_on_log -> (), proxy_on_delete -> ()) ALSO
// traps, so re-entering the poisoned instance during Close cascades a second
// trap (BUG-3). _initialize + proxy_abi_version_0_2_1 (no-op) + a 1-page
// memory let NewRootVM instantiate + Configure cleanly; only the per-stream
// dispatch + teardown callbacks trap.
//
// Used by TestStreamContext_TrapPoison_SkipsTeardown to verify that after a
// trap, sc.trapped is set + Close SKIPS the teardown triplet (returns nil
// instead of cascading the re-entry trap).
var requestHeadersTrapsModule = buildModule(
	typeSection(
		[2][]byte{nil, nil}, // type 0: () -> ()
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32}, {wasmTypeI32}}, // type 1: (i32,i32,i32) -> i32  (on_request_headers)
		[2][]byte{{wasmTypeI32}, {wasmTypeI32}},                           // type 2: (i32) -> i32          (on_done)
		[2][]byte{{wasmTypeI32}, nil},                                     // type 3: (i32) -> ()           (on_log / on_delete)
	),
	// fn 0: _initialize (type 0)
	// fn 1: proxy_abi_version_0_2_1 (type 0)
	// fn 2: proxy_on_request_headers (type 1) — TRAPS
	// fn 3: proxy_on_done (type 2) — TRAPS
	// fn 4: proxy_on_log (type 3) — TRAPS
	// fn 5: proxy_on_delete (type 3) — TRAPS
	functionSection([]uint32{0, 0, 1, 2, 3, 3}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "_initialize", kind: wasmExtFunction, idx: 0},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 1},
		{name: "proxy_on_request_headers", kind: wasmExtFunction, idx: 2},
		{name: "proxy_on_done", kind: wasmExtFunction, idx: 3},
		{name: "proxy_on_log", kind: wasmExtFunction, idx: 4},
		{name: "proxy_on_delete", kind: wasmExtFunction, idx: 5},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(),                      // _initialize
		funcBody(),                      // proxy_abi_version_0_2_1
		funcBody([]byte{opUnreachable}), // proxy_on_request_headers TRAPS
		funcBody([]byte{opUnreachable}), // proxy_on_done TRAPS
		funcBody([]byte{opUnreachable}), // proxy_on_log TRAPS
		funcBody([]byte{opUnreachable}), // proxy_on_delete TRAPS
	}),
)

// --- Fixture: vmStartOnlyModule -------------------------------------------
//
// Exports `proxy_on_vm_start(ctx_id, vm_config_size) -> i32` and a marker
// `proxy_abi_version_0_2_1` + a 1-page memory. Does NOT export
// `proxy_on_context_create` — used to verify that vm.Run gracefully skips
// the proxy_on_context_create seeding step when the guest doesn't export it
// (matches the "some guests omit it" comment in vm.Run).
//
// proxy_on_vm_start body calls proxy_log(level=3 Warn,...) + returns 1.
var vmStartOnlyModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32}, {wasmTypeI32}}, // type 0: proxy_log
		[2][]byte{{wasmTypeI32, wasmTypeI32}, {wasmTypeI32}},              // type 1: proxy_on_vm_start
		[2][]byte{nil, nil}, // type 2: () -> ()  (sentinel)
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_log", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1, 2, 2}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "proxy_on_vm_start", kind: wasmExtFunction, idx: 1},
		{name: "_initialize", kind: wasmExtFunction, idx: 2},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 3},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		// proxy_on_vm_start: proxy_log(3,0,0), drop, return 1
		funcBody(
			i32Const(3),
			i32Const(0),
			i32Const(0),
			call(0),
			drop(),
			i32Const(1),
		),
		funcBody(), // _initialize: no-op
		funcBody(), // sentinel: no-op
	}),
)

// --- Fixture: fullRosterImporterModule ------------------------------------
//
// Imports EVERY hostcall registered by registerHostModules (47 total = 16
// active proxy_* + 8 active wasi_* + 23 deferred stubs). The module does
// NOT call any of them — instantiation alone verifies the registration is
// complete (wazero would error with "unknown import" if any were missing).
//
// Each import type signature must match the host-side registration; we
// reuse a small set of type indices since many hostcalls share the same
// (uint32, uint32, ...) -> uint32 shape.
//
// The fixture is generated by allHostcallImports below; the type-index
// table is built such that:
//
//	type 0: (i32) -> i32                     — 1 param + result; proc_exit-style
//	type 1: (i32, i32) -> i32
//	type 2: (i32, i32, i32) -> i32           — proxy_log
//	type 3: (i32, i32, i32, i32) -> i32
//	type 4: (i32, i32, i32, i32, i32) -> i32
//	type 5: (i32, i32, i32, i32, i32, i32) -> i32
//	type 6: () -> i32                        — proxy_done
//	type 7: (i32, i64, i32) -> i32           — clock_time_get
//	type 8: (i32) -> ()                      — proc_exit (no result)
//	type 9: (i32, i64) -> i32                — proxy_increment_metric / proxy_record_metric
//	type 10: 8 i32 params -> i32             — proxy_send_local_response
//	type 11: 9 i32 params -> i32             — proxy_grpc_stream
//	type 12: 10 i32 params -> i32            — proxy_http_call
//	type 13: 12 i32 params -> i32            — proxy_grpc_call
//	type 14: () -> ()                        — sentinel
type rosterImport struct {
	module  string
	name    string
	typeIdx uint32
}

var rosterImports = []rosterImport{
	// 16 proxy_* active (env namespace) per §5.1
	{"env", "proxy_log", 2},                          // (i32, i32, i32) -> i32
	{"env", "proxy_get_log_level", 0},                // (i32) -> i32
	{"env", "proxy_send_local_response", 10},         // (8 × i32) -> i32
	{"env", "proxy_get_header_map_pairs", 2},         // (i32, i32, i32) -> i32
	{"env", "proxy_set_header_map_pairs", 2},         // (i32, i32, i32) -> i32
	{"env", "proxy_get_header_map_value", 4},         // (5 × i32) -> i32
	{"env", "proxy_add_header_map_value", 4},         // (5 × i32) -> i32
	{"env", "proxy_replace_header_map_value", 4},     // (5 × i32) -> i32
	{"env", "proxy_remove_header_map_value", 2},      // (3 × i32) -> i32
	{"env", "proxy_get_header_map_size", 1},          // (i32, i32) -> i32
	{"env", "proxy_get_property", 3},                 // (4 × i32) -> i32
	{"env", "proxy_set_property", 3},                 // (4 × i32) -> i32
	{"env", "proxy_get_status", 2},                   // (i32, i32, i32) -> i32
	{"env", "proxy_set_effective_context", 0},        // (i32) -> i32
	{"env", "proxy_done", 6},                         // () -> i32
	{"env", "proxy_get_current_time_nanoseconds", 0}, // (i32) -> i32
	// 23 deferred stubs (env namespace) per §5.4
	{"env", "proxy_get_buffer_bytes", 4},             // (5 × i32) -> i32
	{"env", "proxy_set_buffer_bytes", 4},             // (5 × i32) -> i32
	{"env", "proxy_get_buffer_status", 2},            // (3 × i32) -> i32
	{"env", "proxy_continue_stream", 0},              // (i32) -> i32
	{"env", "proxy_close_stream", 0},                 // (i32) -> i32
	{"env", "proxy_set_tick_period_milliseconds", 0}, // (i32) -> i32
	{"env", "proxy_define_metric", 3},                // (4 × i32) -> i32
	{"env", "proxy_get_metric", 1},                   // (2 × i32) -> i32
	{"env", "proxy_increment_metric", 9},             // (i32, i64) -> i32
	{"env", "proxy_record_metric", 9},                // (i32, i64) -> i32
	{"env", "proxy_get_shared_data", 4},              // (5 × i32) -> i32
	{"env", "proxy_set_shared_data", 4},              // (5 × i32) -> i32
	{"env", "proxy_register_shared_queue", 2},        // (3 × i32) -> i32
	{"env", "proxy_resolve_shared_queue", 4},         // (5 × i32) -> i32
	{"env", "proxy_dequeue_shared_queue", 2},         // (3 × i32) -> i32
	{"env", "proxy_enqueue_shared_queue", 2},         // (3 × i32) -> i32
	{"env", "proxy_http_call", 12},                   // (10 × i32) -> i32
	{"env", "proxy_grpc_call", 13},                   // (12 × i32) -> i32
	{"env", "proxy_grpc_stream", 11},                 // (9 × i32) -> i32
	{"env", "proxy_grpc_send", 3},                    // (4 × i32) -> i32
	{"env", "proxy_grpc_cancel", 0},                  // (i32) -> i32
	{"env", "proxy_grpc_close", 0},                   // (i32) -> i32
	{"env", "proxy_call_foreign_function", 5},        // (6 × i32) -> i32
	// 8 wasi_* shims (wasi_snapshot_preview1 namespace) per §5.2
	{"wasi_snapshot_preview1", "fd_write", 3},          // (4 × i32) -> i32
	{"wasi_snapshot_preview1", "clock_time_get", 7},    // (i32, i64, i32) -> i32
	{"wasi_snapshot_preview1", "random_get", 1},        // (i32, i32) -> i32
	{"wasi_snapshot_preview1", "environ_sizes_get", 1}, // (i32, i32) -> i32
	{"wasi_snapshot_preview1", "environ_get", 1},       // (i32, i32) -> i32
	{"wasi_snapshot_preview1", "args_sizes_get", 1},    // (i32, i32) -> i32
	{"wasi_snapshot_preview1", "args_get", 1},          // (i32, i32) -> i32
	{"wasi_snapshot_preview1", "proc_exit", 8},         // (i32) -> () (no result)
}

// nI32Params returns N i32-type bytes (for type-section param list construction).
func nI32Params(n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = wasmTypeI32
	}
	return out
}

// fullRosterImporterModule imports every hostcall registered by
// registerHostModules. Built lazily via init() because the type-section
// list is non-trivial.
var fullRosterImporterModule = func() []byte {
	imports := make([]importEntry, len(rosterImports))
	for i, ri := range rosterImports {
		imports[i] = importEntry{module: ri.module, name: ri.name, kind: wasmExtFunction, idx: ri.typeIdx}
	}

	// Build the type-section list. Index → (params, results) tuple.
	types := [][2][]byte{
		/* 0 */ {nI32Params(1), []byte{wasmTypeI32}},
		/* 1 */ {nI32Params(2), []byte{wasmTypeI32}},
		/* 2 */ {nI32Params(3), []byte{wasmTypeI32}},
		/* 3 */ {nI32Params(4), []byte{wasmTypeI32}},
		/* 4 */ {nI32Params(5), []byte{wasmTypeI32}},
		/* 5 */ {nI32Params(6), []byte{wasmTypeI32}},
		/* 6 */ {nil, []byte{wasmTypeI32}},
		/* 7 */ {{wasmTypeI32, wasmTypeI64, wasmTypeI32}, []byte{wasmTypeI32}},
		/* 8 */ {nI32Params(1), nil}, // proc_exit
		/* 9 */ {{wasmTypeI32, wasmTypeI64}, []byte{wasmTypeI32}},
		/* 10 */ {nI32Params(8), []byte{wasmTypeI32}},
		/* 11 */ {nI32Params(9), []byte{wasmTypeI32}},
		/* 12 */ {nI32Params(10), []byte{wasmTypeI32}},
		/* 13 */ {nI32Params(12), []byte{wasmTypeI32}},
		/* 14 */ {nil, nil}, // sentinel
	}

	// Local function 0 (global idx = len(imports)): proxy_abi_version_0_2_1 (sentinel)
	abiSentinelGlobalIdx := uint32(len(imports))

	return buildModule(
		typeSection(types...),
		importSection(imports),
		functionSection([]uint32{14}), // one local fn of type 14 (() -> ())
		memorySection(1),
		exportSection([]exportEntry{
			{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: abiSentinelGlobalIdx},
			{name: "memory", kind: wasmExtMemory, idx: 0},
		}),
		codeSection([][]byte{
			funcBody(), // empty body
		}),
	)
}()

// --- Fixture: importerSetTickPeriodModule ---------------------------------
//
// Imports `env.proxy_set_tick_period_milliseconds(i32) -> i32` but does
// NOT call it. Used by TestRegistration_NewHostcall_DenyRejectsGuest-
// Instantiation_25_2 to verify the gate-at-registration deny-path
// observable from a guest: under a sandbox that denies
// `proxy_set_tick_period_milliseconds`, `runtime.Instantiate` of this
// fixture FAILS with an "unknown import" error per AMEND-B5.
var importerSetTickPeriodModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32}, {wasmTypeI32}}, // type 0: (i32) -> i32 — proxy_set_tick_period_milliseconds
		[2][]byte{nil, nil},                     // type 1: () -> () — sentinel + _initialize
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_set_tick_period_milliseconds", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1, 1}), // local fn 0 = _initialize; local fn 1 = sentinel
	memorySection(1),
	exportSection([]exportEntry{
		{name: "_initialize", kind: wasmExtFunction, idx: 1},             // global fn idx 1 (after 1 import)
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2}, // global fn idx 2
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(), // _initialize: no-op
		funcBody(), // sentinel: no-op
	}),
)

// --- Fixture: exportsAll25_2CallbacksModule -------------------------------
//
// Exports all 7 NEW 25.2 lifecycle callbacks per §5.3 (rows C14-C20). Each
// body is empty (returns the default zero — ProxyActionContinue for body+
// trailer callbacks; void for tick + httpCallResponse + foreignFunction).
// Used by TestGateAtRegistration_NewCallback_HasGlobalFunc_{Allow,Deny}
// to verify the gate-at-getFunction discipline per AMEND-B5: when the
// capability is denied, HasGlobalFunc returns false EVEN THOUGH the guest
// exports the function.
//
// Type indices:
//
//	0: (i32, i32, i32) -> i32   — proxy_on_request_body / response_body
//	1: (i32, i32) -> i32        — proxy_on_request_trailers / response_trailers
//	2: (i32) -> ()              — proxy_on_tick
//	3: (i32, i32, i32, i32, i32) -> ()  — proxy_on_http_call_response
//	4: (i32, i32, i32) -> ()    — proxy_on_foreign_function
//	5: () -> ()                 — _initialize + sentinel
var exportsAll25_2CallbacksModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32}, {wasmTypeI32}},                 // type 0: request_body / response_body
		[2][]byte{{wasmTypeI32, wasmTypeI32}, {wasmTypeI32}},                              // type 1: request_trailers / response_trailers
		[2][]byte{{wasmTypeI32}, nil},                                                     // type 2: proxy_on_tick (void)
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32}, nil}, // type 3: proxy_on_http_call_response (void, 5 args)
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32}, nil},                           // type 4: proxy_on_foreign_function (void, 3 args)
		[2][]byte{nil, nil}, // type 5: () -> () — _initialize + sentinel
	),
	functionSection([]uint32{
		0, // fn 0: proxy_on_request_body  (type 0)
		0, // fn 1: proxy_on_response_body (type 0)
		1, // fn 2: proxy_on_request_trailers  (type 1)
		1, // fn 3: proxy_on_response_trailers (type 1)
		2, // fn 4: proxy_on_tick           (type 2)
		3, // fn 5: proxy_on_http_call_response (type 3)
		4, // fn 6: proxy_on_foreign_function   (type 4)
		5, // fn 7: _initialize             (type 5)
		5, // fn 8: proxy_abi_version_0_2_1 (type 5)
	}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "proxy_on_request_body", kind: wasmExtFunction, idx: 0},
		{name: "proxy_on_response_body", kind: wasmExtFunction, idx: 1},
		{name: "proxy_on_request_trailers", kind: wasmExtFunction, idx: 2},
		{name: "proxy_on_response_trailers", kind: wasmExtFunction, idx: 3},
		{name: "proxy_on_tick", kind: wasmExtFunction, idx: 4},
		{name: "proxy_on_http_call_response", kind: wasmExtFunction, idx: 5},
		{name: "proxy_on_foreign_function", kind: wasmExtFunction, idx: 6},
		{name: "_initialize", kind: wasmExtFunction, idx: 7},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 8},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		// proxy_on_request_body: return 0 (Continue)
		funcBody(i32Const(0)),
		// proxy_on_response_body: return 0
		funcBody(i32Const(0)),
		// proxy_on_request_trailers: return 0
		funcBody(i32Const(0)),
		// proxy_on_response_trailers: return 0
		funcBody(i32Const(0)),
		// proxy_on_tick: void
		funcBody(),
		// proxy_on_http_call_response: void
		funcBody(),
		// proxy_on_foreign_function: void
		funcBody(),
		// _initialize: no-op
		funcBody(),
		// sentinel: no-op
		funcBody(),
	}),
)

// --- Fixture: httpCallResponseTrapsModule ---------------------------------
//
// Exports `proxy_on_http_call_response(ctx_id, call_id, num_headers,
// body_size, num_trailers) -> ()` whose body is a single `unreachable` (a
// wazero trap) — modeling a guest panic inside the http-call response
// callback (e.g. a Rust panic! that aborts to `unreachable`).
//
// Unlike requestHeadersTrapsModule the teardown triplet is NOT exported, so
// StreamContext.Close is a clean no-op and the test isolates the
// proxy_on_http_call_response trap itself.
//
// Used by TestA4_HttpCallResponseTrap_PoisonsStreamContext (phase 82 Task 3 +
// Task 4) to verify that a trapping http-call-response callback poisons the
// StreamContext (sc.trapped), co-increments envoy_go.failures, and does NOT
// increment http_call_response.
var httpCallResponseTrapsModule = buildModule(
	typeSection(
		[2][]byte{nil, nil}, // type 0: () -> ()
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32}, nil}, // type 1: proxy_on_http_call_response (void, 5 args)
	),
	// fn 0: _initialize (type 0)
	// fn 1: proxy_abi_version_0_2_1 (type 0)
	// fn 2: proxy_on_http_call_response (type 1) — TRAPS
	functionSection([]uint32{0, 0, 1}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "_initialize", kind: wasmExtFunction, idx: 0},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 1},
		{name: "proxy_on_http_call_response", kind: wasmExtFunction, idx: 2},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(),                      // _initialize
		funcBody(),                      // proxy_abi_version_0_2_1
		funcBody([]byte{opUnreachable}), // proxy_on_http_call_response TRAPS
	}),
)

// --- Fixture: httpCallResponseLogsNumHeadersModule ------------------------
//
// Exports `proxy_on_http_call_response(ctx_id, call_id, num_headers,
// body_size, num_trailers) -> ()` whose body calls the imported
// `proxy_log(level, ptr, size)` with level := num_headers (local index 2) and
// an empty message, then drops the result.
//
// This makes the num_headers argument the guest actually received OBSERVABLE
// from the host side: the ABICallbacks.Log hook receives it as the LogLevel.
// It also gives the test a re-entrancy point INSIDE the callback frame, so a
// Log hook can snapshot rv.HTTPCallResponse() while the cache is published
// (the clear only runs after the guest returns).
//
// Used by TestHttpCallResponse_CachePublishedDuringCallback (phase 82 Task 2).
var httpCallResponseLogsNumHeadersModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32}, {wasmTypeI32}}, // type 0: proxy_log
		[2][]byte{nil, nil}, // type 1: () -> ()
		[2][]byte{{wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32, wasmTypeI32}, nil}, // type 2: proxy_on_http_call_response
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_log", kind: wasmExtFunction, idx: 0}, // fn 0
	}),
	// fn 1: _initialize (type 1)
	// fn 2: proxy_abi_version_0_2_1 (type 1)
	// fn 3: proxy_on_http_call_response (type 2)
	functionSection([]uint32{1, 1, 2}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "_initialize", kind: wasmExtFunction, idx: 1},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2},
		{name: "proxy_on_http_call_response", kind: wasmExtFunction, idx: 3},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(), // _initialize
		funcBody(), // proxy_abi_version_0_2_1
		// proxy_on_http_call_response: proxy_log(num_headers, 0, 0); drop
		funcBody(
			localGet(2), // num_headers → level
			i32Const(0), // msg_ptr
			i32Const(0), // msg_size
			call(0),     // proxy_log
			[]byte{opDrop},
		),
	}),
)

// --- silence-unused guards ------------------------------------------------

// Keep encoding/binary referenced (the helpers use it via wazero's memory
// methods downstream; the fixture builder hand-rolls LEB128 here, so no
// direct dep — but the import keeps the build clean for shared test deps).
var _ = binary.LittleEndian
