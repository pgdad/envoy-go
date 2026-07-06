// Shared test helpers for the internal/wasm/ package. Migrated from
// 25.1's vm_test.go at 25.2 Task 1 (vm.go + vm_test.go DELETED per
// D-P-PLAN-6); the helpers serve both the new root_vm_test.go +
// stream_context_test.go as well as the pre-existing registration_test.go
// (which still relies on fakeABICallbacks + allowAllSandbox).

package wasm

import (
	"context"
	"fmt"
	"sync"

	"github.com/pgdad/envoy-go/internal/wasm/abi"
)

// fakeABICallbacks records every method invocation for test assertions.
// Each method's behavior is configurable via the corresponding response field.
type fakeABICallbacks struct {
	mu sync.Mutex

	// Call log: every invocation appends an entry for later assertions.
	calls []string

	// Configurable returns.
	getHeaderMapReturn      []HeaderPair
	getHeaderMapOK          bool
	getHeaderMapValueReturn string
	getHeaderMapValueOK     bool
	addReturn               abi.WasmResult
	replaceReturn           abi.WasmResult
	removeReturn            abi.WasmResult
	setPairsReturn          abi.WasmResult
	getHeaderMapSizeReturn  uint32
	getPropertyReturn       []byte
	getPropertyOK           bool
	setPropertyReturn       abi.WasmResult
	sendLocalReturn         abi.WasmResult
	getStatusCode           uint32
	getStatusValue          []byte
	getStatusOK             bool
	getLogLevelReturn       abi.LogLevel
	getCurrentTimeReturn    uint64
	setEffectiveCtxReturn   abi.WasmResult
	doneReturn              abi.WasmResult
	panicOnLog              bool   // if set, Log panics on invocation
	panicOnLogMsg           string // the panic value (if panicOnLog)
	logLastMsg              string
	logLastLevel            abi.LogLevel
	addHeaderLastKey        string
	addHeaderLastValue      string

	// 25.2 Task 4 body+buffer + stream-control fields.
	getBufferReturn      []byte
	getBufferErr         error
	setBufferReturn      abi.WasmResult
	setBufferLastData    []byte
	setBufferLastStart   uint32
	setBufferLastType    abi.WasmBufferType
	getBufferStatusSize  uint32
	getBufferStatusFlags uint32
	getBufferStatusErr   error
	continueStreamReturn abi.WasmResult
	closeStreamReturn    abi.WasmResult
}

func (f *fakeABICallbacks) record(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, s)
}

func (f *fakeABICallbacks) callsSnapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

func (f *fakeABICallbacks) GetHeaderMap(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType) ([]HeaderPair, bool) {
	f.record(fmt.Sprintf("GetHeaderMap(%d)", mapType))
	return f.getHeaderMapReturn, f.getHeaderMapOK
}
func (f *fakeABICallbacks) GetHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key string) (string, bool) {
	f.record(fmt.Sprintf("GetHeaderMapValue(%d,%q)", mapType, key))
	return f.getHeaderMapValueReturn, f.getHeaderMapValueOK
}
func (f *fakeABICallbacks) AddHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult {
	f.mu.Lock()
	f.addHeaderLastKey = key
	f.addHeaderLastValue = value
	f.mu.Unlock()
	f.record(fmt.Sprintf("AddHeaderMapValue(%d,%q,%q)", mapType, key, value))
	return f.addReturn
}
func (f *fakeABICallbacks) ReplaceHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult {
	f.record(fmt.Sprintf("ReplaceHeaderMapValue(%d,%q,%q)", mapType, key, value))
	return f.replaceReturn
}
func (f *fakeABICallbacks) RemoveHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key string) abi.WasmResult {
	f.record(fmt.Sprintf("RemoveHeaderMapValue(%d,%q)", mapType, key))
	return f.removeReturn
}
func (f *fakeABICallbacks) SetHeaderMapPairs(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, pairs []HeaderPair) abi.WasmResult {
	f.record(fmt.Sprintf("SetHeaderMapPairs(%d,%d-pairs)", mapType, len(pairs)))
	return f.setPairsReturn
}
func (f *fakeABICallbacks) GetHeaderMapSize(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType) uint32 {
	f.record(fmt.Sprintf("GetHeaderMapSize(%d)", mapType))
	return f.getHeaderMapSizeReturn
}
func (f *fakeABICallbacks) GetProperty(_ context.Context, _ uint32, path []string) ([]byte, bool) {
	f.record(fmt.Sprintf("GetProperty(%v)", path))
	return f.getPropertyReturn, f.getPropertyOK
}
func (f *fakeABICallbacks) SetProperty(_ context.Context, _ uint32, path []string, value []byte) abi.WasmResult {
	f.record(fmt.Sprintf("SetProperty(%v,%d-bytes)", path, len(value)))
	return f.setPropertyReturn
}
func (f *fakeABICallbacks) SendLocalResponse(_ context.Context, _ uint32, statusCode uint32, statusMsg, body string, addl []HeaderPair, grpcStatus int32) abi.WasmResult {
	f.record(fmt.Sprintf("SendLocalResponse(%d,%q,%q,%d-hdrs,%d)", statusCode, statusMsg, body, len(addl), grpcStatus))
	return f.sendLocalReturn
}
func (f *fakeABICallbacks) GetStatus(_ context.Context, _ uint32) (uint32, []byte, bool) {
	f.record("GetStatus")
	return f.getStatusCode, f.getStatusValue, f.getStatusOK
}
func (f *fakeABICallbacks) Log(_ context.Context, _ uint32, level abi.LogLevel, msg string) {
	f.mu.Lock()
	f.logLastLevel = level
	f.logLastMsg = msg
	pan := f.panicOnLog
	panMsg := f.panicOnLogMsg
	f.mu.Unlock()
	f.record(fmt.Sprintf("Log(%d,%q)", level, msg))
	if pan {
		panic(panMsg)
	}
}
func (f *fakeABICallbacks) GetLogLevel(_ context.Context) abi.LogLevel {
	f.record("GetLogLevel")
	return f.getLogLevelReturn
}
func (f *fakeABICallbacks) GetCurrentTimeNanoseconds(_ context.Context) uint64 {
	f.record("GetCurrentTimeNanoseconds")
	return f.getCurrentTimeReturn
}
func (f *fakeABICallbacks) SetEffectiveContext(_ context.Context, contextID uint32) abi.WasmResult {
	f.record(fmt.Sprintf("SetEffectiveContext(%d)", contextID))
	return f.setEffectiveCtxReturn
}
func (f *fakeABICallbacks) Done(_ context.Context, contextID uint32) abi.WasmResult {
	f.record(fmt.Sprintf("Done(%d)", contextID))
	return f.doneReturn
}

// --- 25.2 Task 4 body+buffer + stream-control surface (test fake stubs).
//
// Each method records a call-log entry and returns either a canned value
// (for tests that exercise the dispatch) or the zero value. Tests that
// need to drive the bridge supply seeded values via the per-method
// configurable return fields below.

func (f *fakeABICallbacks) GetBuffer(_ context.Context, streamContextID uint32, bufferType abi.WasmBufferType) ([]byte, error) {
	f.record(fmt.Sprintf("GetBuffer(%d,%d)", streamContextID, bufferType))
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getBufferReturn, f.getBufferErr
}

func (f *fakeABICallbacks) SetBuffer(_ context.Context, streamContextID uint32, bufferType abi.WasmBufferType, start uint32, data []byte) abi.WasmResult {
	f.mu.Lock()
	f.setBufferLastData = append([]byte(nil), data...)
	f.setBufferLastStart = start
	f.setBufferLastType = bufferType
	r := f.setBufferReturn
	f.mu.Unlock()
	f.record(fmt.Sprintf("SetBuffer(%d,%d,start=%d,len=%d)", streamContextID, bufferType, start, len(data)))
	return r
}

func (f *fakeABICallbacks) GetBufferStatus(_ context.Context, streamContextID uint32, bufferType abi.WasmBufferType) (uint32, uint32, error) {
	f.record(fmt.Sprintf("GetBufferStatus(%d,%d)", streamContextID, bufferType))
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.getBufferStatusSize, f.getBufferStatusFlags, f.getBufferStatusErr
}

func (f *fakeABICallbacks) ContinueStream(_ context.Context, streamContextID uint32, streamType uint32) abi.WasmResult {
	f.record(fmt.Sprintf("ContinueStream(%d,%d)", streamContextID, streamType))
	return f.continueStreamReturn
}

func (f *fakeABICallbacks) CloseStream(_ context.Context, streamContextID uint32, streamType uint32) abi.WasmResult {
	f.record(fmt.Sprintf("CloseStream(%d,%d)", streamContextID, streamType))
	return f.closeStreamReturn
}

// Compile-time guard: fakeABICallbacks satisfies ABICallbacks.
var _ ABICallbacks = (*fakeABICallbacks)(nil)

// allowAllSandbox returns a SandboxConfig with every capability key allowed.
// Useful for tests that want the sandbox out of the way + focus on behavior.
// At 25.2 Task 3 this helper covers the full 58-key roster (37 25.1 + 21
// NEW 25.2 per AMEND-B5: 14 hostcall + 7 lifecycle callback) — necessary
// for tests exercising the 14 NEW gated hostcalls + the 7 NEW gated
// callbacks at registration time + lookup time.
func allowAllSandbox() SandboxConfig {
	keys := []string{
		// Headers-bridge (7) — 25.1
		capProxyGetHeaderMapPairs, capProxySetHeaderMapPairs, capProxyGetHeaderMapValue,
		capProxyAddHeaderMapValue, capProxyReplaceHeaderMapValue, capProxyRemoveHeaderMapValue,
		capProxyGetHeaderMapSize,
		// Local-response (1) — 25.1
		capProxySendLocalResponse,
		// Property (2) — 25.1
		capProxyGetProperty, capProxySetProperty,
		// Log (2) — 25.1
		capProxyLog, capProxyGetLogLevel,
		// Status (1) — 25.1
		capProxyGetStatus,
		// Time (1) — 25.1
		capProxyGetCurrentTimeNanoseconds,
		// Context-lifecycle (2) — 25.1
		capProxySetEffectiveContext, capProxyDone,
		// WASI (8) — 25.1
		capWasiFdWrite, capWasiClockTimeGet, capWasiRandomGet,
		capWasiEnvironSizesGet, capWasiEnvironGet, capWasiArgsSizesGet, capWasiArgsGet, capWasiProcExit,
		// Module-init / allocator (5) — 25.1; informational; not consulted by Configure
		capModuleInitialize, capModuleStart, capModuleMain, capAllocatorMalloc, capAllocatorProxyOnMemoryAllocate,
		// Lifecycle + HTTP module-getters (8) — 25.1
		capProxyOnContextCreate, capProxyOnVmStart, capProxyOnConfigure, capProxyOnDone,
		capProxyOnDelete, capProxyOnLog, capProxyOnRequestHeaders, capProxyOnResponseHeaders,
		// 25.2 NEW hostcall keys (14) per AMEND-B5 §11.5
		capProxyGetBufferBytes, capProxySetBufferBytes, capProxyGetBufferStatus,
		capProxyContinueStream, capProxyCloseStream,
		capProxySetTickPeriodMilliseconds,
		capProxyDefineMetric, capProxyIncrementMetric, capProxyRecordMetric, capProxyGetMetric,
		capProxySetSharedData, capProxyGetSharedData,
		capProxyHttpCall,
		capProxyCallForeignFunction,
		// 25.2 NEW lifecycle callback keys (7) per AMEND-B5 §11.5
		capProxyOnRequestBody, capProxyOnResponseBody,
		capProxyOnRequestTrailers, capProxyOnResponseTrailers,
		capProxyOnTick,
		capProxyOnHttpCallResponse,
		capProxyOnForeignFunction,
	}
	allowed := make(map[string]SanitizationConfig, len(keys))
	for _, k := range keys {
		allowed[k] = SanitizationConfig{}
	}
	return SandboxConfig{AllowedCapabilities: allowed}
}
