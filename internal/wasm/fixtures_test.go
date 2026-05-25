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
	opEnd      = 0x0b
	opCall     = 0x10
	opLocalGet = 0x20 // not used yet — reserved for future fixtures
	opI32Const = 0x41
	opI32Store = 0x36
	opDrop     = 0x1a
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

// --- Fixture: invokeContinueStreamModule ----------------------------------
//
// Imports `env.proxy_continue_stream(i32) -> i32` (a deferred-stub hostcall)
// and exports `invoke_continue_stream() -> i32` that calls
// proxy_continue_stream(0) + returns its result. Used to verify the
// deferred-stub returns WasmResultUnimplemented (=12).
var invokeContinueStreamModule = buildModule(
	typeSection(
		[2][]byte{{wasmTypeI32}, {wasmTypeI32}}, // type 0: (i32) -> i32 (proxy_continue_stream)
		[2][]byte{nil, {wasmTypeI32}},           // type 1: () -> i32 (invoke_continue_stream)
		[2][]byte{nil, nil},                     // type 2: () -> () (sentinel)
	),
	importSection([]importEntry{
		{module: "env", name: "proxy_continue_stream", kind: wasmExtFunction, idx: 0},
	}),
	functionSection([]uint32{1, 2}),
	memorySection(1),
	exportSection([]exportEntry{
		{name: "invoke_continue_stream", kind: wasmExtFunction, idx: 1},
		{name: "proxy_abi_version_0_2_1", kind: wasmExtFunction, idx: 2},
		{name: "memory", kind: wasmExtMemory, idx: 0},
	}),
	codeSection([][]byte{
		funcBody(
			i32Const(0), // stream_type
			call(0),     // proxy_continue_stream → returns i32 on stack (the function result)
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

// --- silence-unused guards ------------------------------------------------

// Keep encoding/binary referenced (the helpers use it via wazero's memory
// methods downstream; the fixture builder hand-rolls LEB128 here, so no
// direct dep — but the import keeps the build clean for shared test deps).
var _ = binary.LittleEndian
