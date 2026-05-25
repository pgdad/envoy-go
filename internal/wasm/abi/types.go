// Package abi defines proxy-wasm v0.2.1 wire-protocol enum types
// (value-faithful encoding per AMEND-A7).
//
// Guest modules compiled against the proxy-wasm spec check specific integer
// values directly (e.g. `if result == 10` for InternalFailure). Renumbering
// any constant in this file would break wire compatibility with every
// existing proxy-wasm guest module. The value-gaps in WasmResult at
// positions 5, 9, 11 are PRESERVED INTENTIONALLY per AMEND-A7 (BRAINSTORM
// hypothesized 13 contiguous values; that hypothesis was incorrect).
//
// Test coverage lives in types_test.go (value-faithful + value-gap
// preservation + reflect.Kind == int32 assertions).
package abi

// WasmResult is the host→guest result code. 10 named values with value-gaps
// at positions 5, 9, 11 (per AMEND-A7 — the gaps MUST be preserved
// byte-faithfully because guest modules check specific integer values and
// remapping would break wire compatibility).
type WasmResult int32

// WasmResult* constants per proxy-wasm v0.2.1 + AMEND-A7. Gaps at 5/9/11
// (BadExpression / ResultMismatch / BrokenConnection in BRAINSTORM
// hypothesis) do NOT exist in the v0.2.1 wire protocol and are reserved.
const (
	WasmResultOk                   WasmResult = 0
	WasmResultNotFound             WasmResult = 1
	WasmResultBadArgument          WasmResult = 2
	WasmResultSerializationFailure WasmResult = 3
	WasmResultParseFailure         WasmResult = 4
	// value 5 reserved (gap — does NOT exist in v0.2.1)
	WasmResultInvalidMemoryAccess WasmResult = 6
	WasmResultEmpty               WasmResult = 7
	WasmResultCasMismatch         WasmResult = 8
	// value 9 reserved (gap — does NOT exist in v0.2.1)
	WasmResultInternalFailure WasmResult = 10
	// value 11 reserved (gap — does NOT exist in v0.2.1)
	WasmResultUnimplemented WasmResult = 12
)

// WasmBufferType identifies the buffer the guest is reading/writing.
// Value 8 = FOREIGN_FUNCTION_ARGUMENTS (NOT CallData as BRAINSTORM
// hypothesized; per AMEND-A7).
type WasmBufferType int32

// WasmBufferType* constants per proxy-wasm v0.2.1 + AMEND-A7. Value 8 is
// ForeignFunctionArguments (RATIFIED over BRAINSTORM's CallData hypothesis).
const (
	WasmBufferTypeHttpRequestBody          WasmBufferType = 0
	WasmBufferTypeHttpResponseBody         WasmBufferType = 1
	WasmBufferTypeDownstreamData           WasmBufferType = 2
	WasmBufferTypeUpstreamData             WasmBufferType = 3
	WasmBufferTypeHttpCallResponseBody     WasmBufferType = 4
	WasmBufferTypeGrpcReceiveBuffer        WasmBufferType = 5
	WasmBufferTypeVmConfiguration          WasmBufferType = 6
	WasmBufferTypePluginConfiguration      WasmBufferType = 7
	WasmBufferTypeForeignFunctionArguments WasmBufferType = 8 // NOT CallData
)

// WasmHeaderMapType identifies the header-map the guest is reading/writing.
type WasmHeaderMapType int32

// WasmHeaderMapType* constants per proxy-wasm v0.2.1.
const (
	WasmHeaderMapTypeHttpRequestHeaders          WasmHeaderMapType = 0
	WasmHeaderMapTypeHttpRequestTrailers         WasmHeaderMapType = 1
	WasmHeaderMapTypeHttpResponseHeaders         WasmHeaderMapType = 2
	WasmHeaderMapTypeHttpResponseTrailers        WasmHeaderMapType = 3
	WasmHeaderMapTypeHttpCallResponseHeaders     WasmHeaderMapType = 4
	WasmHeaderMapTypeHttpCallResponseTrailers    WasmHeaderMapType = 5
	WasmHeaderMapTypeGrpcReceiveInitialMetadata  WasmHeaderMapType = 6
	WasmHeaderMapTypeGrpcReceiveTrailingMetadata WasmHeaderMapType = 7
)

// LogLevel is the proxy_log severity.
type LogLevel int32

// LogLevel* constants per proxy-wasm v0.2.1 proxy_log severity wire values.
const (
	LogLevelTrace    LogLevel = 0
	LogLevelDebug    LogLevel = 1
	LogLevelInfo     LogLevel = 2
	LogLevelWarn     LogLevel = 3
	LogLevelError    LogLevel = 4
	LogLevelCritical LogLevel = 5
)

// ProxyAction is the guest→host action on proxy_on_request_headers /
// proxy_on_response_headers callbacks.
type ProxyAction int32

// ProxyAction* constants per proxy-wasm v0.2.1 callback return wire values.
const (
	ProxyActionContinue ProxyAction = 0
	ProxyActionPause    ProxyAction = 1
)

// WasiErrno is the WASI errno return code for the 8 WASI shim hostcalls.
// Partial roster (only the values 25.1 surface actually uses); full
// roster lives in proxy-wasm spec v0.2.1.
type WasiErrno int32

// WasiErrno* constants — partial roster (5 values used by 25.1 surface).
// WasiErrnoNotcapable (76) is upstream's choice for capability denial; see D-P1.
const (
	WasiErrnoSuccess    WasiErrno = 0
	WasiErrnoBadf       WasiErrno = 8
	WasiErrnoInval      WasiErrno = 28
	WasiErrnoNotsup     WasiErrno = 58
	WasiErrnoNotcapable WasiErrno = 76
)
