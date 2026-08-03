// internal/wasm/abi/types_test.go — value-faithful + value-gap preservation
// tests for proxy-wasm v0.2.1 enum constants per 25.1 SPEC §3.1 + AMEND-A7.
//
// These tests REQUIRE that every enum constant has the exact integer value
// the proxy-wasm spec assigns, because guest modules check specific values
// directly (e.g. `if result == 10` for InternalFailure). Renumbering would
// break wire compatibility with every existing proxy-wasm guest in the wild.
//
// In particular WasmResult preserves the value-gaps at positions 5/9/11
// (BRAINSTORM hypothesized 13 contiguous values; AMEND-A7 RATIFIED 10 named
// values with gaps), and WasmBufferType value 8 binds to
// FOREIGN_FUNCTION_ARGUMENTS (NOT CallData as BRAINSTORM hypothesized;
// AMEND-A7).

package abi

import (
	"reflect"
	"testing"
)

// TestWasmResult_Values asserts every named WasmResult constant equals its
// proxy-wasm v0.2.1 wire integer value byte-exactly.
func TestWasmResult_Values(t *testing.T) {
	cases := []struct {
		name string
		got  WasmResult
		want int32
	}{
		{"Ok", WasmResultOk, 0},
		{"NotFound", WasmResultNotFound, 1},
		{"BadArgument", WasmResultBadArgument, 2},
		{"SerializationFailure", WasmResultSerializationFailure, 3},
		{"ParseFailure", WasmResultParseFailure, 4},
		// gap at 5 — verified separately in TestWasmResult_GapPreservation
		{"InvalidMemoryAccess", WasmResultInvalidMemoryAccess, 6},
		{"Empty", WasmResultEmpty, 7},
		{"CasMismatch", WasmResultCasMismatch, 8},
		// gap at 9
		{"InternalFailure", WasmResultInternalFailure, 10},
		// gap at 11
		{"Unimplemented", WasmResultUnimplemented, 12},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if int32(tc.got) != tc.want {
				t.Fatalf("WasmResult%s = %d, want %d (wire compat per AMEND-A7)",
					tc.name, int32(tc.got), tc.want)
			}
		})
	}
}

// TestWasmResult_GapPreservation asserts NO named WasmResult constant takes
// the gap positions 5, 9, 11 — these gaps are byte-faithfully preserved per
// AMEND-A7 (BRAINSTORM's contiguous-13-values hypothesis was incorrect).
func TestWasmResult_GapPreservation(t *testing.T) {
	all := []WasmResult{
		WasmResultOk,
		WasmResultNotFound,
		WasmResultBadArgument,
		WasmResultSerializationFailure,
		WasmResultParseFailure,
		WasmResultInvalidMemoryAccess,
		WasmResultEmpty,
		WasmResultCasMismatch,
		WasmResultInternalFailure,
		WasmResultUnimplemented,
	}
	if got, want := len(all), 10; got != want {
		t.Fatalf("WasmResult named-value count = %d, want %d (AMEND-A7: 10 named values)", got, want)
	}
	for _, gap := range []int32{5, 9, 11} {
		for _, v := range all {
			if int32(v) == gap {
				t.Fatalf("WasmResult value %d is RESERVED gap per AMEND-A7; got constant = %d",
					gap, int32(v))
			}
		}
	}
}

// TestWasmResult_Kind asserts WasmResult's underlying kind is int32.
func TestWasmResult_Kind(t *testing.T) {
	if k := reflect.TypeOf(WasmResultOk).Kind(); k != reflect.Int32 {
		t.Fatalf("WasmResult kind = %s, want int32", k)
	}
}

// TestWasmBufferType_Values asserts every WasmBufferType constant matches
// its proxy-wasm v0.2.1 wire value byte-exactly. Per AMEND-A7, value 8 is
// ForeignFunctionArguments (NOT CallData as BRAINSTORM hypothesized).
func TestWasmBufferType_Values(t *testing.T) {
	cases := []struct {
		name string
		got  WasmBufferType
		want int32
	}{
		{"HttpRequestBody", WasmBufferTypeHttpRequestBody, 0},
		{"HttpResponseBody", WasmBufferTypeHttpResponseBody, 1},
		{"DownstreamData", WasmBufferTypeDownstreamData, 2},
		{"UpstreamData", WasmBufferTypeUpstreamData, 3},
		{"HttpCallResponseBody", WasmBufferTypeHttpCallResponseBody, 4},
		{"GrpcReceiveBuffer", WasmBufferTypeGrpcReceiveBuffer, 5},
		{"VmConfiguration", WasmBufferTypeVmConfiguration, 6},
		{"PluginConfiguration", WasmBufferTypePluginConfiguration, 7},
		{"ForeignFunctionArguments", WasmBufferTypeForeignFunctionArguments, 8},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if int32(tc.got) != tc.want {
				t.Fatalf("WasmBufferType%s = %d, want %d", tc.name, int32(tc.got), tc.want)
			}
		})
	}
}

// TestWasmBufferType_ForeignFunctionArgumentsAt8 explicitly asserts the
// AMEND-A7 RATIFIED bind: value 8 = ForeignFunctionArguments (NOT CallData).
func TestWasmBufferType_ForeignFunctionArgumentsAt8(t *testing.T) {
	if int32(WasmBufferTypeForeignFunctionArguments) != 8 {
		t.Fatalf("WasmBufferTypeForeignFunctionArguments = %d, want 8 (AMEND-A7 RATIFIED over BRAINSTORM CallData hypothesis)",
			int32(WasmBufferTypeForeignFunctionArguments))
	}
}

// TestWasmBufferType_Kind asserts WasmBufferType's underlying kind is int32.
func TestWasmBufferType_Kind(t *testing.T) {
	if k := reflect.TypeOf(WasmBufferTypeHttpRequestBody).Kind(); k != reflect.Int32 {
		t.Fatalf("WasmBufferType kind = %s, want int32", k)
	}
}

// TestWasmHeaderMapType_Values asserts every WasmHeaderMapType constant
// matches its proxy-wasm v0.2.1 wire value byte-exactly.
func TestWasmHeaderMapType_Values(t *testing.T) {
	cases := []struct {
		name string
		got  WasmHeaderMapType
		want int32
	}{
		{"HttpRequestHeaders", WasmHeaderMapTypeHttpRequestHeaders, 0},
		{"HttpRequestTrailers", WasmHeaderMapTypeHttpRequestTrailers, 1},
		{"HttpResponseHeaders", WasmHeaderMapTypeHttpResponseHeaders, 2},
		{"HttpResponseTrailers", WasmHeaderMapTypeHttpResponseTrailers, 3},
		// Values 4-7 CORRECTED at phase 82; the four rows below previously
		// pinned Grpc*=6/7 + HttpCallResponse*=4/5, i.e. this golden was
		// hand-written and shared the declaration's swap, so it passed on a
		// WRONG value rather than catching it. Re-derived from the SDK the
		// guest blobs compile against: proxy-wasm-rust-sdk v0.2.4
		// src/types.rs `enum MapType` — GrpcReceiveInitialMetadata = 4,
		// GrpcReceiveTrailingMetadata = 5, HttpCallResponseHeaders = 6,
		// HttpCallResponseTrailers = 7.
		{"GrpcReceiveInitialMetadata", WasmHeaderMapTypeGrpcReceiveInitialMetadata, 4},
		{"GrpcReceiveTrailingMetadata", WasmHeaderMapTypeGrpcReceiveTrailingMetadata, 5},
		{"HttpCallResponseHeaders", WasmHeaderMapTypeHttpCallResponseHeaders, 6},
		{"HttpCallResponseTrailers", WasmHeaderMapTypeHttpCallResponseTrailers, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if int32(tc.got) != tc.want {
				t.Fatalf("WasmHeaderMapType%s = %d, want %d", tc.name, int32(tc.got), tc.want)
			}
		})
	}
}

// TestWasmHeaderMapType_Kind asserts WasmHeaderMapType's underlying kind is int32.
func TestWasmHeaderMapType_Kind(t *testing.T) {
	if k := reflect.TypeOf(WasmHeaderMapTypeHttpRequestHeaders).Kind(); k != reflect.Int32 {
		t.Fatalf("WasmHeaderMapType kind = %s, want int32", k)
	}
}

// TestLogLevel_Values asserts every LogLevel constant matches its proxy_log
// wire severity value byte-exactly.
func TestLogLevel_Values(t *testing.T) {
	cases := []struct {
		name string
		got  LogLevel
		want int32
	}{
		{"Trace", LogLevelTrace, 0},
		{"Debug", LogLevelDebug, 1},
		{"Info", LogLevelInfo, 2},
		{"Warn", LogLevelWarn, 3},
		{"Error", LogLevelError, 4},
		{"Critical", LogLevelCritical, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if int32(tc.got) != tc.want {
				t.Fatalf("LogLevel%s = %d, want %d", tc.name, int32(tc.got), tc.want)
			}
		})
	}
}

// TestLogLevel_Kind asserts LogLevel's underlying kind is int32.
func TestLogLevel_Kind(t *testing.T) {
	if k := reflect.TypeOf(LogLevelTrace).Kind(); k != reflect.Int32 {
		t.Fatalf("LogLevel kind = %s, want int32", k)
	}
}

// TestProxyAction_Values asserts every ProxyAction constant matches its
// guest→host action wire value byte-exactly.
func TestProxyAction_Values(t *testing.T) {
	cases := []struct {
		name string
		got  ProxyAction
		want int32
	}{
		{"Continue", ProxyActionContinue, 0},
		{"Pause", ProxyActionPause, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if int32(tc.got) != tc.want {
				t.Fatalf("ProxyAction%s = %d, want %d", tc.name, int32(tc.got), tc.want)
			}
		})
	}
}

// TestProxyAction_Kind asserts ProxyAction's underlying kind is int32.
func TestProxyAction_Kind(t *testing.T) {
	if k := reflect.TypeOf(ProxyActionContinue).Kind(); k != reflect.Int32 {
		t.Fatalf("ProxyAction kind = %s, want int32", k)
	}
}

// TestWasiErrno_Values asserts every WasiErrno constant matches its WASI
// errno wire value byte-exactly. Per 25.1 SPEC §3.1 this is a partial
// roster (only the 5 values the 25.1 surface uses); full roster lives in
// proxy-wasm v0.2.1 spec.
func TestWasiErrno_Values(t *testing.T) {
	cases := []struct {
		name string
		got  WasiErrno
		want int32
	}{
		{"Success", WasiErrnoSuccess, 0},
		{"Badf", WasiErrnoBadf, 8},
		{"Inval", WasiErrnoInval, 28},
		{"Notsup", WasiErrnoNotsup, 58},
		{"Notcapable", WasiErrnoNotcapable, 76},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if int32(tc.got) != tc.want {
				t.Fatalf("WasiErrno%s = %d, want %d", tc.name, int32(tc.got), tc.want)
			}
		})
	}
}

// TestWasiErrno_Kind asserts WasiErrno's underlying kind is int32.
func TestWasiErrno_Kind(t *testing.T) {
	if k := reflect.TypeOf(WasiErrnoSuccess).Kind(); k != reflect.Int32 {
		t.Fatalf("WasiErrno kind = %s, want int32", k)
	}
}
