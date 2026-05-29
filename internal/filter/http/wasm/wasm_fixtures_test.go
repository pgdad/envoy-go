package wasm

// wasm_fixtures_test.go — minimal hand-crafted WebAssembly Core 1.0 binary
// fixtures for the Task 12 end-to-end dispatch tests. The internal/wasm
// package has a richer set of fixture helpers in `fixtures_test.go` but
// they are package-internal (test files cannot be imported across
// packages); we duplicate the strict minimum needed for the filter-side
// integration tests here.
//
// Each fixture below is a minimal, valid WebAssembly Core 1.0 binary
// constructed via the small DSL in this file — keeping the byte layout
// explicit + auditable. The fixture set is sufficient for:
//
//   - testHelloWasm — a minimal proxy_abi_version_0_2_1-exporting module
//     that has no proxy_on_request_headers / proxy_on_response_headers
//     export (lifecycle dispatch is a no-op success); also imports a tiny
//     subset of env hostcalls to verify the host module surface.
//
//   - testRespHeadersContinueWasm — exports proxy_on_request_headers +
//     proxy_on_response_headers that both return ProxyActionContinue (=0).
//     Used by EncodeHeaders integration test.
//
//   - testPauseWasm — exports proxy_on_request_headers that returns
//     ProxyActionPause (=1) without invoking proxy_send_local_response;
//     exercises the PAUSE-w/o-local-response log-and-continue arm.
//
//   - testSendLocalResponseWasm — exports proxy_on_request_headers that
//     calls proxy_send_local_response(403, msg_ptr, msg_size, body_ptr,
//     body_size, addl_ptr, addl_size, -1) before returning ProxyActionPause.
//     Used by the StopIteration + SendLocalReply integration test.
//
// Wire reference: https://webassembly.github.io/spec/core/binary/modules.html

// --- LEB128 + section + opcode primitives ---------------------------------

// fixUleb128 returns the unsigned LEB128 encoding of v.
func fixUleb128(v uint32) []byte {
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

// fixSleb128 returns the signed LEB128 encoding of v (used for i32.const).
func fixSleb128(v int32) []byte {
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

// fixSection builds a section: id || size || payload.
func fixSection(id byte, payload []byte) []byte {
	out := []byte{id}
	out = append(out, fixUleb128(uint32(len(payload)))...)
	out = append(out, payload...)
	return out
}

const (
	fixTypeI32       = 0x7f
	fixExtFunction   = 0x00
	fixExtMemory     = 0x02
	fixOpUnreachable = 0x00
	fixOpEnd         = 0x0b
	fixOpCall        = 0x10
	fixOpI32Const    = 0x41
	fixOpDrop        = 0x1a
	fixOpIf          = 0x04
	fixOpGlobalGet   = 0x23
	fixOpGlobalSet   = 0x24
	fixBlockTypeVoid = 0x40
	fixGlobalMutable = 0x01
	fixSectionType   = 0x01
	fixSectionImp    = 0x02
	fixSectionFunc   = 0x03
	fixSectionMem    = 0x05
	fixSectionGlobal = 0x06
	fixSectionExp    = 0x07
	fixSectionCode   = 0x0a
)

// fixFuncType encodes a (params, results) type signature.
func fixFuncType(params, results []byte) []byte {
	out := []byte{0x60}
	out = append(out, fixUleb128(uint32(len(params)))...)
	out = append(out, params...)
	out = append(out, fixUleb128(uint32(len(results)))...)
	out = append(out, results...)
	return out
}

// fixTypeSection encodes a type section from a list of (params, results).
func fixTypeSection(types ...[2][]byte) []byte {
	payload := fixUleb128(uint32(len(types)))
	for _, t := range types {
		payload = append(payload, fixFuncType(t[0], t[1])...)
	}
	return fixSection(fixSectionType, payload)
}

type fixImport struct {
	module string
	name   string
	kind   byte
	idx    uint32
}

func fixImportSection(imports []fixImport) []byte {
	payload := fixUleb128(uint32(len(imports)))
	for _, imp := range imports {
		payload = append(payload, fixUleb128(uint32(len(imp.module)))...)
		payload = append(payload, []byte(imp.module)...)
		payload = append(payload, fixUleb128(uint32(len(imp.name)))...)
		payload = append(payload, []byte(imp.name)...)
		payload = append(payload, imp.kind)
		if imp.kind == fixExtFunction {
			payload = append(payload, fixUleb128(imp.idx)...)
		}
	}
	return fixSection(fixSectionImp, payload)
}

func fixFunctionSection(typeIndices []uint32) []byte {
	payload := fixUleb128(uint32(len(typeIndices)))
	for _, idx := range typeIndices {
		payload = append(payload, fixUleb128(idx)...)
	}
	return fixSection(fixSectionFunc, payload)
}

func fixMemorySection(minPages uint32) []byte {
	payload := fixUleb128(1)
	payload = append(payload, 0x00) // limits flag: no max
	payload = append(payload, fixUleb128(minPages)...)
	return fixSection(fixSectionMem, payload)
}

// fixGlobalSection encodes a global section of mutable i32 globals, each
// initialized to 0 via a constant init-expr (i32.const 0; end). One entry per
// `count`. Section id 0x06 must appear after the memory section (0x05) and
// before the export section (0x07) per the WebAssembly module section ordering.
func fixGlobalSection(count uint32) []byte {
	payload := fixUleb128(count)
	for i := uint32(0); i < count; i++ {
		payload = append(payload, fixTypeI32)        // valtype i32
		payload = append(payload, fixGlobalMutable)  // mutability: mutable
		payload = append(payload, fixI32Const(0)...) // init expr
		payload = append(payload, fixOpEnd)
	}
	return fixSection(fixSectionGlobal, payload)
}

func fixGlobalGet(idx uint32) []byte {
	return append([]byte{fixOpGlobalGet}, fixUleb128(idx)...)
}

func fixGlobalSet(idx uint32) []byte {
	return append([]byte{fixOpGlobalSet}, fixUleb128(idx)...)
}

// fixIfVoid wraps the supplied body instructions in `if (void) <body> end` —
// pops one i32 from the stack; executes body when non-zero. No else arm.
func fixIfVoid(body ...[]byte) []byte {
	out := []byte{fixOpIf, fixBlockTypeVoid}
	for _, b := range body {
		out = append(out, b...)
	}
	out = append(out, fixOpEnd)
	return out
}

type fixExport struct {
	name string
	kind byte
	idx  uint32
}

func fixExportSection(exports []fixExport) []byte {
	payload := fixUleb128(uint32(len(exports)))
	for _, exp := range exports {
		payload = append(payload, fixUleb128(uint32(len(exp.name)))...)
		payload = append(payload, []byte(exp.name)...)
		payload = append(payload, exp.kind)
		payload = append(payload, fixUleb128(exp.idx)...)
	}
	return fixSection(fixSectionExp, payload)
}

func fixCodeSection(bodies [][]byte) []byte {
	payload := fixUleb128(uint32(len(bodies)))
	for _, body := range bodies {
		payload = append(payload, fixUleb128(uint32(len(body)))...)
		payload = append(payload, body...)
	}
	return fixSection(fixSectionCode, payload)
}

// fixFuncBody constructs a function body: 0 locals || expr || end.
func fixFuncBody(expr ...[]byte) []byte {
	out := []byte{0x00} // local-count = 0
	for _, e := range expr {
		out = append(out, e...)
	}
	out = append(out, fixOpEnd)
	return out
}

func fixI32Const(v int32) []byte {
	return append([]byte{fixOpI32Const}, fixSleb128(v)...)
}

func fixCall(funcIdx uint32) []byte {
	return append([]byte{fixOpCall}, fixUleb128(funcIdx)...)
}

func fixDrop() []byte { return []byte{fixOpDrop} }

// fixModuleHeader is the 4-byte magic + 4-byte version 1 prefix.
var fixModuleHeader = []byte{
	0x00, 0x61, 0x73, 0x6d,
	0x01, 0x00, 0x00, 0x00,
}

func fixBuildModule(sections ...[]byte) []byte {
	out := make([]byte, 0, 64)
	out = append(out, fixModuleHeader...)
	for _, s := range sections {
		out = append(out, s...)
	}
	return out
}

// --- Fixture builders -------------------------------------------------------

// buildMinimalProxyWasm constructs a valid module that exports
// `proxy_abi_version_0_2_1` (no-op body) + `_initialize` (no-op body) +
// a 1-page memory. NO proxy_on_request_headers / proxy_on_response_headers
// — the per-callback dispatch is a no-op success (vm.CallProxyOnX with
// missing export returns Continue + nil per the Task 7 VM contract).
//
// Used by the no-export-arm lifecycle test (TestFilter_DecodeHeaders_
// MissingExports_ContinueNoOp) to verify the missing-callback no-op
// path per upstream wasm's "nullptr the function pointer" discipline.
func buildMinimalProxyWasm() []byte {
	return fixBuildModule(
		fixTypeSection(
			[2][]byte{nil, nil}, // () -> ()
		),
		fixFunctionSection([]uint32{0, 0}), // 2 local fns of type 0
		fixMemorySection(1),
		fixExportSection([]fixExport{
			{name: "_initialize", kind: fixExtFunction, idx: 0},
			{name: "proxy_abi_version_0_2_1", kind: fixExtFunction, idx: 1},
			{name: "memory", kind: fixExtMemory, idx: 0},
		}),
		fixCodeSection([][]byte{
			fixFuncBody(),
			fixFuncBody(),
		}),
	)
}

// buildContinueProxyWasm constructs a module that exports
// proxy_on_request_headers + proxy_on_response_headers, both returning
// ProxyActionContinue (=0). Includes _initialize + proxy_abi_version_0_2_1
// + a 1-page memory. NO imports — the dispatch goes through without any
// hostcall.
func buildContinueProxyWasm() []byte {
	// Types: 0 = () -> (), 1 = (i32, i32, i32) -> i32.
	return fixBuildModule(
		fixTypeSection(
			[2][]byte{nil, nil}, // type 0: () -> ()
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}}, // type 1: (i32, i32, i32) -> i32
		),
		// 4 local functions:
		// fn 0: _initialize (type 0)
		// fn 1: proxy_abi_version_0_2_1 (type 0)
		// fn 2: proxy_on_request_headers (type 1)
		// fn 3: proxy_on_response_headers (type 1)
		fixFunctionSection([]uint32{0, 0, 1, 1}),
		fixMemorySection(1),
		fixExportSection([]fixExport{
			{name: "_initialize", kind: fixExtFunction, idx: 0},
			{name: "proxy_abi_version_0_2_1", kind: fixExtFunction, idx: 1},
			{name: "proxy_on_request_headers", kind: fixExtFunction, idx: 2},
			{name: "proxy_on_response_headers", kind: fixExtFunction, idx: 3},
			{name: "memory", kind: fixExtMemory, idx: 0},
		}),
		fixCodeSection([][]byte{
			fixFuncBody(),
			fixFuncBody(),
			fixFuncBody(fixI32Const(0)), // return ProxyActionContinue
			fixFuncBody(fixI32Const(0)), // return ProxyActionContinue
		}),
	)
}

// buildPauseProxyWasm constructs a module whose proxy_on_request_headers
// returns ProxyActionPause (=1) without invoking proxy_send_local_response.
// Exercises the PAUSE-w/o-local-response log+continue arm.
func buildPauseProxyWasm() []byte {
	return fixBuildModule(
		fixTypeSection(
			[2][]byte{nil, nil},
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}},
		),
		fixFunctionSection([]uint32{0, 0, 1}),
		fixMemorySection(1),
		fixExportSection([]fixExport{
			{name: "_initialize", kind: fixExtFunction, idx: 0},
			{name: "proxy_abi_version_0_2_1", kind: fixExtFunction, idx: 1},
			{name: "proxy_on_request_headers", kind: fixExtFunction, idx: 2},
			{name: "memory", kind: fixExtMemory, idx: 0},
		}),
		fixCodeSection([][]byte{
			fixFuncBody(),
			fixFuncBody(),
			fixFuncBody(fixI32Const(1)), // return ProxyActionPause
		}),
	)
}

// buildTrappingProxyWasm constructs a module whose proxy_on_request_headers
// executes the `unreachable` instruction (opcode 0x00) — a guest TRAP. wazero
// surfaces this as a RuntimeError from fn.Call, which the host catches as a
// non-nil error on the CallProxyOnRequestHeaders return. This is the REAL
// guest-trap condition the differential test surfaced (a Rust panic! in the
// proxy-wasm-rust-sdk dispatcher leaves the RefCell borrowed; the host catches
// the trap but must NOT re-enter the poisoned instance on teardown — see
// BUG-3). proxy_on_response_headers + proxy_on_done/log/delete are ALSO
// trapping so any re-entry of the poisoned instance would cascade (which the
// trapped-instance guard must prevent).
//
// Exports _initialize + proxy_abi_version_0_2_1 (no-op) + a 1-page memory +
// the five trapping callbacks. No imports.
func buildTrappingProxyWasm() []byte {
	// trap body: a single `unreachable` then `end`.
	trapBody := fixFuncBody([]byte{fixOpUnreachable})
	return fixBuildModule(
		fixTypeSection(
			[2][]byte{nil, nil}, // type 0: () -> ()
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}}, // type 1: (i32,i32,i32) -> i32
			[2][]byte{{fixTypeI32}, {fixTypeI32}},                         // type 2: (i32) -> i32  (proxy_on_done)
			[2][]byte{{fixTypeI32}, nil},                                  // type 3: (i32) -> ()   (proxy_on_log / proxy_on_delete)
		),
		// fn 0: _initialize (type 0)
		// fn 1: proxy_abi_version_0_2_1 (type 0)
		// fn 2: proxy_on_request_headers (type 1) — TRAPS
		// fn 3: proxy_on_response_headers (type 1) — TRAPS
		// fn 4: proxy_on_done (type 2) — TRAPS
		// fn 5: proxy_on_log (type 3) — TRAPS
		// fn 6: proxy_on_delete (type 3) — TRAPS
		fixFunctionSection([]uint32{0, 0, 1, 1, 2, 3, 3}),
		fixMemorySection(1),
		fixExportSection([]fixExport{
			{name: "_initialize", kind: fixExtFunction, idx: 0},
			{name: "proxy_abi_version_0_2_1", kind: fixExtFunction, idx: 1},
			{name: "proxy_on_request_headers", kind: fixExtFunction, idx: 2},
			{name: "proxy_on_response_headers", kind: fixExtFunction, idx: 3},
			{name: "proxy_on_done", kind: fixExtFunction, idx: 4},
			{name: "proxy_on_log", kind: fixExtFunction, idx: 5},
			{name: "proxy_on_delete", kind: fixExtFunction, idx: 6},
			{name: "memory", kind: fixExtMemory, idx: 0},
		}),
		fixCodeSection([][]byte{
			fixFuncBody(), // _initialize
			fixFuncBody(), // proxy_abi_version_0_2_1
			trapBody,      // proxy_on_request_headers TRAPS
			trapBody,      // proxy_on_response_headers TRAPS
			trapBody,      // proxy_on_done TRAPS
			trapBody,      // proxy_on_log TRAPS
			trapBody,      // proxy_on_delete TRAPS
		}),
	)
}

// buildContextCreatePoisonProxyWasm constructs a module that replicates the
// BUG-4 "poisoned-instance context-create trap" condition self-contained (no
// Rust-SDK blob). It carries ONE mutable i32 wasm global `poisoned` (global 0,
// init 0):
//
//   - proxy_on_context_create (global 0 == 0 on a fresh instance) → NO trap;
//     when global 0 != 0 (the instance has been poisoned by a prior header
//     trap) → `unreachable` (TRAP). This is the BUG-4 condition: once the shared
//     instance is poisoned, EVERY subsequent proxy_on_context_create traps.
//   - proxy_on_request_headers → `global.set 0 = 1` (POISON the instance — the
//     write persists in wazero linear/global state across the trap unwind) then
//     `unreachable` (TRAP). Mirrors a Rust-SDK guest that leaves a RefCell
//     borrowed on panic so any subsequent entry into the instance re-traps.
//
// A `reinstantiate` (fresh InstantiateModule) resets global 0 back to its init
// value (0), so proxy_on_context_create on the FRESH instance succeeds — which
// is exactly what the reload machine must reach (BUG-4 fix: ReloadDispatch must
// run BEFORE initStreamContext so context-create lands on the fresh instance).
//
// Exports _initialize + proxy_abi_version_0_2_1 (no-op) + a 1-page memory +
// proxy_on_context_create + proxy_on_request_headers + proxy_on_response_headers
// + the teardown triplet (proxy_on_done/log/delete are NO-OP here so OnDestroy's
// trapped-instance guard is not the subject under test). No imports.
func buildContextCreatePoisonProxyWasm() []byte {
	return fixBuildModule(
		fixTypeSection(
			[2][]byte{nil, nil}, // type 0: () -> ()
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}}, // type 1: (i32,i32,i32) -> i32
			[2][]byte{{fixTypeI32, fixTypeI32}, nil},                      // type 2: (i32,i32) -> ()  proxy_on_context_create
			[2][]byte{{fixTypeI32}, {fixTypeI32}},                         // type 3: (i32) -> i32     proxy_on_done
			[2][]byte{{fixTypeI32}, nil},                                  // type 4: (i32) -> ()      proxy_on_log / proxy_on_delete
		),
		// fn 0: _initialize (type 0)
		// fn 1: proxy_abi_version_0_2_1 (type 0)
		// fn 2: proxy_on_context_create (type 2) — TRAPS IFF poisoned
		// fn 3: proxy_on_request_headers (type 1) — POISONS then TRAPS
		// fn 4: proxy_on_response_headers (type 1) — returns Continue
		// fn 5: proxy_on_done (type 3) — no-op (returns 0)
		// fn 6: proxy_on_log (type 4) — no-op
		// fn 7: proxy_on_delete (type 4) — no-op
		fixFunctionSection([]uint32{0, 0, 2, 1, 1, 3, 4, 4}),
		fixMemorySection(1),
		fixGlobalSection(1), // global 0: mutable i32 `poisoned`, init 0
		fixExportSection([]fixExport{
			{name: "_initialize", kind: fixExtFunction, idx: 0},
			{name: "proxy_abi_version_0_2_1", kind: fixExtFunction, idx: 1},
			{name: "proxy_on_context_create", kind: fixExtFunction, idx: 2},
			{name: "proxy_on_request_headers", kind: fixExtFunction, idx: 3},
			{name: "proxy_on_response_headers", kind: fixExtFunction, idx: 4},
			{name: "proxy_on_done", kind: fixExtFunction, idx: 5},
			{name: "proxy_on_log", kind: fixExtFunction, idx: 6},
			{name: "proxy_on_delete", kind: fixExtFunction, idx: 7},
			{name: "memory", kind: fixExtMemory, idx: 0},
		}),
		fixCodeSection([][]byte{
			fixFuncBody(), // _initialize
			fixFuncBody(), // proxy_abi_version_0_2_1
			// proxy_on_context_create: if poisoned (global 0 != 0) → unreachable.
			fixFuncBody(
				fixGlobalGet(0),
				fixIfVoid([]byte{fixOpUnreachable}),
			),
			// proxy_on_request_headers: poison (global 0 = 1) then trap.
			fixFuncBody(
				fixI32Const(1),
				fixGlobalSet(0),
				[]byte{fixOpUnreachable},
			),
			fixFuncBody(fixI32Const(0)), // proxy_on_response_headers → Continue
			fixFuncBody(fixI32Const(0)), // proxy_on_done → 0
			fixFuncBody(),               // proxy_on_log
			fixFuncBody(),               // proxy_on_delete
		}),
	)
}

// buildSendLocalResponseProxyWasm constructs a module whose
// proxy_on_request_headers invokes
// proxy_send_local_response(403, 0, 0, 0, 0, 0, 0, -1) and then returns
// ProxyActionPause. Exercises the REUSE-5 captured-local-response →
// StopIteration + SendLocalReply path.
//
// The 8 hostcall args are all-zero / minus-one (no msg, no body, no addl);
// the test asserts SendLocalReply was invoked with status=403.
func buildSendLocalResponseProxyWasm() []byte {
	// Types:
	//   type 0: () -> ()
	//   type 1: (i32, i32, i32) -> i32 (proxy_on_request_headers)
	//   type 2: (i32, i32, i32, i32, i32, i32, i32, i32) -> i32
	//           (proxy_send_local_response)
	return fixBuildModule(
		fixTypeSection(
			[2][]byte{nil, nil},
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}},
			[2][]byte{{fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32, fixTypeI32}, {fixTypeI32}},
		),
		fixImportSection([]fixImport{
			{module: "env", name: "proxy_send_local_response", kind: fixExtFunction, idx: 2},
		}),
		// Function space layout after 1 import (idx 0):
		//   fn 1 (local 0): _initialize (type 0)
		//   fn 2 (local 1): proxy_abi_version_0_2_1 (type 0)
		//   fn 3 (local 2): proxy_on_request_headers (type 1)
		fixFunctionSection([]uint32{0, 0, 1}),
		fixMemorySection(1),
		fixExportSection([]fixExport{
			{name: "_initialize", kind: fixExtFunction, idx: 1},
			{name: "proxy_abi_version_0_2_1", kind: fixExtFunction, idx: 2},
			{name: "proxy_on_request_headers", kind: fixExtFunction, idx: 3},
			{name: "memory", kind: fixExtMemory, idx: 0},
		}),
		fixCodeSection([][]byte{
			fixFuncBody(), // _initialize
			fixFuncBody(), // proxy_abi_version_0_2_1
			fixFuncBody(
				// proxy_send_local_response(403, 0, 0, 0, 0, 0, 0, -1)
				fixI32Const(403), // status_code
				fixI32Const(0),   // status_msg_ptr
				fixI32Const(0),   // status_msg_size
				fixI32Const(0),   // body_ptr
				fixI32Const(0),   // body_size
				fixI32Const(0),   // additional_headers_ptr
				fixI32Const(0),   // additional_headers_size
				fixI32Const(-1),  // grpc_status
				fixCall(0),       // call import 0 (proxy_send_local_response)
				fixDrop(),        // discard the WasmResult return value
				fixI32Const(1),   // return ProxyActionPause
			),
		}),
	)
}
