// Package proxywasm hosts the in-process proxy-wasm conformance harness per
// phase-25.3 Task 13 (ADR-0212; D-25.3-P4). It is a sub-package of
// test/conformance/ (mirroring the h2spec driver's package layout + doc
// style) but uses a DIFFERENT reproduction mechanism: h2spec is Docker-based
// (boots an envoy-go subprocess + runs a containerized conformance binary
// against it); THIS harness is PURE IN-PROCESS — the proxy-wasm guest runs
// in-process via the project's wazero-backed *wasm.RootVM, and the conformance
// `.wasm` blobs are prebuilt offline (Rust/C++ → wasm32) + VENDORED under
// `bytecode/`, committed to git. NO Docker, NO Rust-in-CI.
//
// # Harness surface (consumed by the Task 14 family ports)
//
// The conformance families (10 ported; see README.md) each drive a vendored
// guest `.wasm` blob through the proxy-wasm lifecycle + assert host-observable
// behavior. The shared seams are:
//
//   - loadFamilyWasm(t, name) — reads bytecode/<name>.wasm; fails if absent.
//   - newConformanceRootVM(t, wasmBytes, opts...) — compiles the bytes →
//     constructs a *wasm.RootVM with a broadly-permissive sandbox + a
//     log-capture sink + a minimal recording ABICallbacks → Configures it.
//     Returns a *conformanceVM bundling the RootVM, the captured-log buffer,
//     and the recording ABICallbacks so a family can: drive proxy_on_* via
//     the RootVM/StreamContext, then assert on captured logs (logging family),
//     shared-data state (shared_data family via rv.GetSharedData), returned
//     ProxyActions / traps (stop_iteration family), and recorded ABI calls.
//   - assertLogContains / assertNoTrap / etc. — per-family assertion utilities.
//
// # Family registry
//
// `conformanceFamilies` is the registry the driver (conformance_test.go)
// ranges over. At Task 13 it is EMPTY → the driver PASSES VACUOUSLY. Task 14
// appends the 10 families, each proven live via deliberate-break.
//
// # Exported-API sufficiency note (Task 13 finding)
//
// The harness lives in an EXTERNAL package, so it consumes only EXPORTED
// internal/wasm symbols: CompileModule, NewRootVM, WithRootSandboxConfig,
// WithRootLogSink, WithRootClock, SandboxConfig, (*RootVM).RegisterABICallbacks
// / Configure / NewStreamContext / GetSharedData / SetSharedData / Close, and
// the wasm/abi enum types. This set is SUFFICIENT to wire a working RootVM
// from outside the package — see the report. The one wrinkle: the capability-
// key constants (cap*) are package-PRIVATE; the harness rebuilds the allow-list
// from the upstream-faithful bare capability strings (the same literals the
// cap* constants hold). This is intentional — the keys ARE the upstream wire
// names, stable across the project per AMEND-A2.
//
// Cross-references:
//   - test/conformance/h2spec/ — structural precedent (Docker-based).
//   - internal/filter/http/wasm/compiled_config.go — production RootVM wiring
//     (CompileModule → NewRootVM → RegisterABICallbacks → Configure →
//     NewStreamContext) that this harness mirrors with a minimal ABICallbacks.
//   - README.md — the 16-file cpp-host roster + 10-port/6-defer rationale +
//     reproduction discipline + deliberate-break-liveness requirement.
package proxywasm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/esalaine/envoy-go/internal/wasm"
	"github.com/esalaine/envoy-go/internal/wasm/abi"
)

// conformanceFamily is one entry in the family registry. `name` is the t.Run
// subtest name; `run` drives the family's vendored guest + asserts behavior.
type conformanceFamily struct {
	name string
	run  func(t *testing.T)
}

// conformanceFamilies is the registry the driver ranges over. EMPTY at Task 13
// (the driver passes vacuously). Task 14 appends the 10 ported families:
// logging, stop_iteration, shared_data, endianness, exports, security,
// runtime, wasm_vm, bytecode_util, pairs_util. Each MUST be proven live via
// deliberate-break per README.md.
//
//nolint:gochecknoglobals // intentional package-level registry the driver ranges over; Task 14 appends families
var conformanceFamilies []conformanceFamily

// conformanceVM bundles a configured *wasm.RootVM with the host-observable
// state a conformance family asserts against: the captured-log buffer (the
// logging family inspects this), and the recording ABICallbacks (families that
// assert on guest→host ABI calls inspect its call log). Shared-data state is
// observed directly via RootVM.GetSharedData / SetSharedData (exported).
//
//nolint:unused // consumed by Task 14 family ports
type conformanceVM struct {
	// RootVM is the configured, ready-to-drive proxy-wasm root VM. Families
	// call NewStreamContext on it + drive proxy_on_* via the StreamContext,
	// or read/write shared-data via GetSharedData / SetSharedData.
	RootVM *wasm.RootVM

	// Logs is the buffer wired as the RootVM's WithRootLogSink. proxy_log
	// output (and the host's structured log lines) land here; the logging
	// family asserts via assertLogContains.
	Logs *syncBuffer

	// ABI is the minimal recording ABICallbacks registered on the RootVM.
	// Families that need to assert which guest→host hostcalls fired inspect
	// ABI.Calls(). Families that need to FEED host-side state into the guest
	// (e.g. seed a header-map for a pairs_util port) can extend this stub at
	// Task 14 with seeded return values.
	ABI *recordingABICallbacks
}

// loadFamilyWasm reads the vendored guest blob bytecode/<name>.wasm (relative
// to this package directory) and fails the test if it is missing. At Task 13
// there are NO blobs yet — this helper is first exercised at Task 14 when the
// families + their vendored .wasm land. The blobs are prebuilt offline +
// committed to git per the reproduction discipline in README.md.
//
//nolint:unused // consumed by Task 14 family ports
func loadFamilyWasm(t *testing.T, name string) []byte {
	t.Helper()
	dir := packageDir(t)
	p := filepath.Join(dir, "bytecode", name+".wasm")
	data, err := os.ReadFile(p) //nolint:gosec // path derived from a test-local family name, not user input
	if err != nil {
		t.Fatalf("loadFamilyWasm(%q): vendored blob missing or unreadable at %s: %v\n"+
			"(blobs are prebuilt offline + committed to git — see README.md reproduction discipline)", name, p, err)
	}
	if len(data) == 0 {
		t.Fatalf("loadFamilyWasm(%q): vendored blob at %s is empty", name, p)
	}
	return data
}

// packageDir returns the directory containing this source file. Used to
// resolve bytecode/ relative to the package regardless of the test's cwd
// (mirrors the h2spec driver's runtime.Caller repo-root walk).
//
//nolint:unused // consumed by Task 14 family ports via loadFamilyWasm
func packageDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed — cannot locate package dir")
	}
	return filepath.Dir(thisFile)
}

// newConformanceRootVM compiles wasmBytes + constructs a configured
// *wasm.RootVM ready for a conformance family to drive. It mirrors the
// production wiring in internal/filter/http/wasm/compiled_config.go:
// CompileModule → NewRootVM (with sandbox + log sink) → RegisterABICallbacks
// → Configure. The sandbox is broadly permissive (every conformance-relevant
// capability allowed) so families can exercise logging, shared-data, header-
// maps, buffers, metrics, etc. without per-family sandbox plumbing.
//
// Extra RootVMOptions (e.g. WithRootClock for a deterministic time family,
// WithRootSharedDataCaps for a shared_data cap port) are appended AFTER the
// harness defaults so a family can override them.
//
// On any failure (compile, construct, configure) the test fails immediately;
// on success a t.Cleanup closes the RootVM.
//
//nolint:unused // consumed by Task 14 family ports
func newConformanceRootVM(t *testing.T, wasmBytes []byte, extra ...wasm.RootVMOption) *conformanceVM {
	t.Helper()
	ctx := context.Background()

	// Compile via a nil cache (transient single-use runtime per the
	// ADR-0085 nil-tolerance contract). The conformance harness compiles one
	// module per family; a shared CompileCache buys nothing here.
	mod, err := wasm.CompileModule(ctx, wasmBytes, nil)
	if err != nil {
		t.Fatalf("newConformanceRootVM: CompileModule: %v", err)
	}

	logs := &syncBuffer{}
	cb := &recordingABICallbacks{logForward: logs}

	opts := append([]wasm.RootVMOption{
		wasm.WithRootSandboxConfig(conformanceSandbox()),
		wasm.WithRootLogSink(logs),
	}, extra...)

	// rootContextID 1 — a fixed non-zero per-process ID is fine for the
	// single-VM-per-family harness (production allocates monotonically).
	rv, err := wasm.NewRootVM(ctx, mod, 1, opts...)
	if err != nil {
		t.Fatalf("newConformanceRootVM: NewRootVM: %v", err)
	}
	t.Cleanup(func() { _ = rv.Close() })

	rv.RegisterABICallbacks(cb)

	// Configure with empty vm_config + plugin_config bytes. Families that
	// need a specific configuration payload re-Configure or pass it via a
	// dedicated future helper at Task 14.
	if err := rv.Configure(ctx, nil, nil); err != nil {
		t.Fatalf("newConformanceRootVM: Configure: %v", err)
	}

	return &conformanceVM{RootVM: rv, Logs: logs, ABI: cb}
}

// conformanceSandbox returns a broadly-permissive SandboxConfig allowing every
// capability the conformance families exercise. The keys are the upstream-
// faithful bare proxy_*/wasi_* names (the SAME literals the package-private
// cap* constants hold in internal/wasm/sandbox.go + wasi.go); the harness
// rebuilds the allow-list here because those constants are unexported. A
// broadly-permissive sandbox is appropriate for conformance — the families
// test guest↔host ABI behavior, not the capability gate (the gate has its own
// internal/wasm/sandbox_test.go coverage + the differential 0035/0037/0039
// fixtures).
//
//nolint:unused // consumed by Task 14 family ports
func conformanceSandbox() wasm.SandboxConfig {
	keys := []string{
		// Headers-bridge (7).
		"proxy_get_header_map_pairs", "proxy_set_header_map_pairs", "proxy_get_header_map_value",
		"proxy_add_header_map_value", "proxy_replace_header_map_value", "proxy_remove_header_map_value",
		"proxy_get_header_map_size",
		// Local-response (1).
		"proxy_send_local_response",
		// Property (2).
		"proxy_get_property", "proxy_set_property",
		// Log (2).
		"proxy_log", "proxy_get_log_level",
		// Status (1).
		"proxy_get_status",
		// Time (1).
		"proxy_get_current_time_nanoseconds",
		// Context-lifecycle (2).
		"proxy_set_effective_context", "proxy_done",
		// WASI (8).
		"fd_write", "clock_time_get", "random_get",
		"environ_sizes_get", "environ_get", "args_sizes_get", "args_get", "proc_exit",
		// Module-init / allocator (5) — informational; ungated by Configure.
		"_initialize", "_start", "main", "malloc", "proxy_on_memory_allocate",
		// Lifecycle + HTTP module-getters (8).
		"proxy_on_context_create", "proxy_on_vm_start", "proxy_on_configure", "proxy_on_done",
		"proxy_on_delete", "proxy_on_log", "proxy_on_request_headers", "proxy_on_response_headers",
		// 25.2 NEW hostcall keys (14).
		"proxy_get_buffer_bytes", "proxy_set_buffer_bytes", "proxy_get_buffer_status",
		"proxy_continue_stream", "proxy_close_stream",
		"proxy_set_tick_period_milliseconds",
		"proxy_define_metric", "proxy_increment_metric", "proxy_record_metric", "proxy_get_metric",
		"proxy_set_shared_data", "proxy_get_shared_data",
		"proxy_http_call",
		"proxy_call_foreign_function",
		// 25.2 NEW lifecycle callback keys (7).
		"proxy_on_request_body", "proxy_on_response_body",
		"proxy_on_request_trailers", "proxy_on_response_trailers",
		"proxy_on_tick",
		"proxy_on_http_call_response",
		"proxy_on_foreign_function",
	}
	allowed := make(map[string]wasm.SanitizationConfig, len(keys))
	for _, k := range keys {
		allowed[k] = wasm.SanitizationConfig{}
	}
	return wasm.SandboxConfig{AllowedCapabilities: allowed}
}

// conformanceSandboxDenying returns the broadly-permissive conformance sandbox
// with the named capabilities REMOVED from the allow-list. The security family
// uses it to drive the SAME guest under a restricted sandbox (e.g. denying
// proxy_get_current_time_nanoseconds while keeping proxy_log allowed) and
// assert the host enforces the per-capability gate. Passed via the `extra`
// RootVMOption to newConformanceRootVM, where it overrides the default
// WithRootSandboxConfig (extras are applied AFTER the harness defaults).
//
//nolint:unused // consumed by Task 14 security family
func conformanceSandboxDenying(deny ...string) wasm.SandboxConfig {
	sb := conformanceSandbox()
	for _, k := range deny {
		delete(sb.AllowedCapabilities, k)
	}
	return sb
}

// assertLogNotContains fails the test if the captured-log buffer contains
// substr. Consumed by the security family to assert a denied-capability path
// did NOT emit the allowed-outcome marker.
//
//nolint:unused // consumed by Task 14 security family
func assertLogNotContains(t *testing.T, logs *syncBuffer, substr string) {
	t.Helper()
	got := logs.String()
	if strings.Contains(got, substr) {
		t.Errorf("captured log unexpectedly contains %q\n--- log ---\n%s", substr, got)
	}
}

// ─── Assertion utilities ──────────────────────────────────────────────────────

// assertLogContains fails the test if the captured-log buffer does not contain
// substr. Consumed by the logging family (Task 14) to assert proxy_log output.
//
//nolint:unused // consumed by Task 14 family ports
func assertLogContains(t *testing.T, logs *syncBuffer, substr string) {
	t.Helper()
	got := logs.String()
	if !strings.Contains(got, substr) {
		t.Errorf("captured log does not contain %q\n--- log ---\n%s", substr, got)
	}
}

// ─── Minimal recording ABICallbacks ────────────────────────────────────────────

// recordingABICallbacks is the harness's minimal ABICallbacks implementation.
// It records every guest→host call (for families that assert on hostcall
// dispatch) + returns benign defaults (empty maps, WasmResultOk, zero values).
// Task 14 families that need to FEED host state into the guest (e.g. seed a
// header-map for a pairs_util port, or canned property values) extend this
// stub with configurable return fields — keeping it minimal here per the
// Task 13 scaffold scope.
//
// Compile-time guard at the bottom of this file asserts it satisfies
// wasm.ABICallbacks (the full 25+-method interface).
type recordingABICallbacks struct {
	mu    sync.Mutex
	calls []string

	// logForward, when non-nil, receives every guest proxy_log call formatted
	// as "[wasm <level>] <msg>\n" (mirroring RootVM.LogProxy's format). The
	// host bridge routes proxy_log -> ABICallbacks.Log, NOT -> the RootVM log
	// sink, so the logging family relies on this forwarder to land guest log
	// output in the captured-log buffer for assertLogContains. Set by
	// newConformanceRootVM to the same syncBuffer wired as WithRootLogSink.
	logForward io.Writer

	// reqHeaders seeds the HttpRequestHeaders map returned by GetHeaderMap
	// (the proxy_get_header_map_pairs host side). The pairs_util family sets
	// this so the guest decodes a known seed through the pairs wire format.
	// Other map types return (nil, false).
	reqHeaders []wasm.HeaderPair

	// writtenRespHeaders records every response-header write the guest makes
	// via ReplaceHeaderMapValue(HttpResponseHeaders, ...) (the host side of
	// the SDK's set_http_response_header). The pairs_util family inspects this
	// to assert what the guest derived from the decoded request pairs.
	writtenRespHeaders map[string]string
}

func (r *recordingABICallbacks) record(s string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, s)
}

// Calls returns a snapshot of the recorded guest→host call log.
//
//nolint:unused // consumed by Task 14 family ports
func (r *recordingABICallbacks) Calls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.calls))
	copy(out, r.calls)
	return out
}

func (r *recordingABICallbacks) GetHeaderMap(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType) ([]wasm.HeaderPair, bool) {
	r.record("GetHeaderMap")
	if mapType == abi.WasmHeaderMapTypeHttpRequestHeaders {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.reqHeaders == nil {
			return nil, false
		}
		// Return a copy so the host bridge's encode cannot mutate the seed.
		out := make([]wasm.HeaderPair, len(r.reqHeaders))
		copy(out, r.reqHeaders)
		return out, true
	}
	return nil, false
}

// SeedRequestHeaders installs the request-header map the guest will read via
// proxy_get_header_map_pairs (decoded through the pairs wire format). Consumed
// by the pairs_util family.
//
//nolint:unused // consumed by Task 14 pairs_util family
func (r *recordingABICallbacks) SeedRequestHeaders(pairs []wasm.HeaderPair) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reqHeaders = pairs
}

// WrittenResponseHeader returns the value the guest wrote to the response-
// header `key` via ReplaceHeaderMapValue (set_http_response_header), and
// whether it was written. Consumed by the pairs_util family.
//
//nolint:unused // consumed by Task 14 pairs_util family
func (r *recordingABICallbacks) WrittenResponseHeader(key string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.writtenRespHeaders[key]
	return v, ok
}

// ResetWrittenResponseHeaders clears the recorded response-header writes. The
// wasm_vm family drives several streams against the SAME RootVM (and thus the
// same recording callbacks) and reads x-stream-count after EACH drive — it
// resets between drives so each read reflects only the latest stream's write
// rather than a stale value from a prior stream.
//
//nolint:unused // consumed by Task 14 wasm_vm family
func (r *recordingABICallbacks) ResetWrittenResponseHeaders() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writtenRespHeaders = nil
}

func (r *recordingABICallbacks) GetHeaderMapValue(_ context.Context, _ uint32, _ abi.WasmHeaderMapType, _ string) (string, bool) {
	r.record("GetHeaderMapValue")
	return "", false
}

func (r *recordingABICallbacks) AddHeaderMapValue(_ context.Context, _ uint32, _ abi.WasmHeaderMapType, _, _ string) abi.WasmResult {
	r.record("AddHeaderMapValue")
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) ReplaceHeaderMapValue(_ context.Context, _ uint32, mapType abi.WasmHeaderMapType, key, value string) abi.WasmResult {
	r.record("ReplaceHeaderMapValue")
	if mapType == abi.WasmHeaderMapTypeHttpResponseHeaders {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.writtenRespHeaders == nil {
			r.writtenRespHeaders = make(map[string]string)
		}
		r.writtenRespHeaders[key] = value
	}
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) RemoveHeaderMapValue(_ context.Context, _ uint32, _ abi.WasmHeaderMapType, _ string) abi.WasmResult {
	r.record("RemoveHeaderMapValue")
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) SetHeaderMapPairs(_ context.Context, _ uint32, _ abi.WasmHeaderMapType, _ []wasm.HeaderPair) abi.WasmResult {
	r.record("SetHeaderMapPairs")
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) GetHeaderMapSize(_ context.Context, _ uint32, _ abi.WasmHeaderMapType) uint32 {
	r.record("GetHeaderMapSize")
	return 0
}

func (r *recordingABICallbacks) GetProperty(_ context.Context, _ uint32, _ []string) ([]byte, bool) {
	r.record("GetProperty")
	return nil, false
}

func (r *recordingABICallbacks) SetProperty(_ context.Context, _ uint32, _ []string, _ []byte) abi.WasmResult {
	r.record("SetProperty")
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) SendLocalResponse(_ context.Context, _ uint32, _ uint32, _, _ string, _ []wasm.HeaderPair, _ int32) abi.WasmResult {
	r.record("SendLocalResponse")
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) GetStatus(_ context.Context, _ uint32) (uint32, []byte, bool) {
	r.record("GetStatus")
	return 0, nil, false
}

func (r *recordingABICallbacks) Log(_ context.Context, _ uint32, level abi.LogLevel, msg string) {
	r.record("Log")
	if r.logForward != nil {
		// Mirror RootVM.LogProxy's "[wasm <level>] <msg>\n" format so the
		// logging family's assertLogContains sees the same shape whether the
		// line came from a guest proxy_log or a host-side LogProxy call.
		_, _ = fmt.Fprintf(r.logForward, "[wasm %s] %s\n", logLevelName(level), msg)
	}
}

// logLevelName maps an abi.LogLevel to the upstream proxy-wasm level-name
// string (the SAME labels RootVM.logLevelString emits). Kept local to the
// harness because logLevelString is package-private to internal/wasm.
//
//nolint:unused // consumed by Task 14 logging family
func logLevelName(l abi.LogLevel) string {
	switch l {
	case abi.LogLevelTrace:
		return "trace"
	case abi.LogLevelDebug:
		return "debug"
	case abi.LogLevelInfo:
		return "info"
	case abi.LogLevelWarn:
		return "warn"
	case abi.LogLevelError:
		return "error"
	case abi.LogLevelCritical:
		return "critical"
	default:
		return "unknown"
	}
}

func (r *recordingABICallbacks) GetLogLevel(_ context.Context) abi.LogLevel {
	r.record("GetLogLevel")
	return abi.LogLevelInfo
}

func (r *recordingABICallbacks) GetCurrentTimeNanoseconds(_ context.Context) uint64 {
	r.record("GetCurrentTimeNanoseconds")
	return 0
}

func (r *recordingABICallbacks) SetEffectiveContext(_ context.Context, _ uint32) abi.WasmResult {
	r.record("SetEffectiveContext")
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) Done(_ context.Context, _ uint32) abi.WasmResult {
	r.record("Done")
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) GetBuffer(_ context.Context, _ uint32, _ abi.WasmBufferType) ([]byte, error) {
	r.record("GetBuffer")
	return nil, nil
}

func (r *recordingABICallbacks) SetBuffer(_ context.Context, _ uint32, _ abi.WasmBufferType, _ uint32, _ []byte) abi.WasmResult {
	r.record("SetBuffer")
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) GetBufferStatus(_ context.Context, _ uint32, _ abi.WasmBufferType) (uint32, uint32, error) {
	r.record("GetBufferStatus")
	return 0, 0, nil
}

func (r *recordingABICallbacks) ContinueStream(_ context.Context, _ uint32, _ uint32) abi.WasmResult {
	r.record("ContinueStream")
	return abi.WasmResultOk
}

func (r *recordingABICallbacks) CloseStream(_ context.Context, _ uint32, _ uint32) abi.WasmResult {
	r.record("CloseStream")
	return abi.WasmResultOk
}

// Compile-time guard: recordingABICallbacks satisfies the full ABICallbacks
// interface. If internal/wasm grows the interface, this line breaks the
// harness build (a deliberate signal that the stub needs the new method).
var _ wasm.ABICallbacks = (*recordingABICallbacks)(nil)

// ─── syncBuffer ────────────────────────────────────────────────────────────────

// syncBuffer is a goroutine-safe bytes.Buffer wrapper used as the RootVM's
// WithRootLogSink. The RootVM's tick goroutine + per-stream dispatch may write
// concurrently, so the sink must be safe for concurrent Write + read.
//
//nolint:unused // consumed by Task 14 family ports
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

//nolint:unused // consumed by Task 14 family ports (WithRootLogSink target)
func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

// String returns the accumulated log output.
//
//nolint:unused // consumed by Task 14 family ports via assertLogContains
func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}
